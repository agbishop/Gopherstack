package main

import (
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	acmbackend "github.com/blackbirdworks/gopherstack/services/acm"
	ec2backend "github.com/blackbirdworks/gopherstack/services/ec2"
	elbv2backend "github.com/blackbirdworks/gopherstack/services/elbv2"
)

// TestInitializeServices_ELBv2EC2ACMWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than
// invoking wireELBv2CrossService directly, so that deleting the wiring call
// from wireComputeAndObservabilityIntegrations -- not just breaking the
// helper function itself -- is what this test is sensitive to. Mirrors
// TestInitializeServices_ELBEC2ACMWiring (cli_elb_ec2_acm_wiring_test.go) for
// ELBv2. Covers three things wired by the same cli.go call site
// (gopherstack-t74c/gopherstack-v7ns): EC2 (SecurityGroups/Subnets
// existence, CreateLoadBalancer), ACM (CertificateArn existence on
// CreateListener), and usage reporting (a certificate attached to a live
// listener resists ACM DeleteCertificate; detaching -- here via
// DeleteListener -- clears it).
func TestInitializeServices_ELBv2EC2ACMWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000"}
	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	elbv2H, ok := byName["ELBv2"].(*elbv2backend.Handler)
	require.True(t, ok, "ELBv2 handler must be registered")

	ec2H, ok := byName["EC2"].(*ec2backend.Handler)
	require.True(t, ok, "EC2 handler must be registered")

	acmH, ok := byName["ACM"].(*acmbackend.Handler)
	require.True(t, ok, "ACM handler must be registered")

	ctx := t.Context()

	vpc, err := ec2H.Backend.CreateVpc("10.0.0.0/16", "default")
	require.NoError(t, err)

	sg, err := ec2H.Backend.CreateSecurityGroup("elbv2-wiring-test-sg", "wiring test", vpc.ID)
	require.NoError(t, err)

	subnet, err := ec2H.Backend.CreateSubnet(vpc.ID, "10.0.1.0/24", "us-east-1a")
	require.NoError(t, err)

	cert, err := acmH.Backend.RequestCertificate(
		ctx, "wired-elbv2.example.com", "AMAZON_ISSUED", "DNS", "", "", "", "", nil,
	)
	require.NoError(t, err)

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(elbv2H))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		ctx,
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	client := elbv2sdk.NewFromConfig(cfg, func(o *elbv2sdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})

	_, err = client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:           aws.String("wired-elbv2-lb"),
		Type:           elbv2types.LoadBalancerTypeEnumApplication,
		Subnets:        []string{"subnet-does-not-exist"},
		SecurityGroups: []string{sg.ID},
	})
	require.Error(t, err,
		"an unknown subnet must be rejected through the actual cli.go composition root's EC2 wiring")

	_, err = client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:           aws.String("wired-elbv2-lb"),
		Type:           elbv2types.LoadBalancerTypeEnumApplication,
		Subnets:        []string{subnet.ID},
		SecurityGroups: []string{"sg-does-not-exist"},
	})
	require.Error(t, err,
		"an unknown security group must be rejected through the actual cli.go composition root's EC2 wiring")

	lbOut, err := client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:           aws.String("wired-elbv2-lb"),
		Type:           elbv2types.LoadBalancerTypeEnumApplication,
		Subnets:        []string{subnet.ID},
		SecurityGroups: []string{sg.ID},
	})
	require.NoError(t, err, "a real EC2-backed subnet and security group must be accepted")
	lbArn := aws.ToString(lbOut.LoadBalancers[0].LoadBalancerArn)

	fixedResponse200 := []elbv2types.Action{
		{
			Type: elbv2types.ActionTypeEnumFixedResponse,
			FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
				StatusCode: aws.String("200"),
			},
		},
	}

	_, err = client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumHttps,
		Port:            aws.Int32(443),
		Certificates: []elbv2types.Certificate{
			{CertificateArn: aws.String("arn:aws:acm:us-east-1:000000000000:certificate/does-not-exist")},
		},
		DefaultActions: fixedResponse200,
	})
	require.Error(t, err,
		"an unknown certificate ARN must be rejected through the actual cli.go composition root's ACM wiring")

	listenerOut, err := client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumHttps,
		Port:            aws.Int32(443),
		Certificates: []elbv2types.Certificate{
			{CertificateArn: aws.String(cert.ARN)},
		},
		DefaultActions: fixedResponse200,
	})
	require.NoError(t, err, "a real ACM certificate must be accepted")
	listenerArn := aws.ToString(listenerOut.Listeners[0].ListenerArn)

	err = acmH.Backend.DeleteCertificate(ctx, cert.ARN)
	require.Error(t, err, "a certificate attached to a live ELBv2 listener must not be deletable")

	_, err = client.DeleteListener(ctx, &elbv2sdk.DeleteListenerInput{ListenerArn: aws.String(listenerArn)})
	require.NoError(t, err)

	err = acmH.Backend.DeleteCertificate(ctx, cert.ARN)
	require.NoError(t, err, "deleting the listener must unmark the certificate's usage, making it deletable")
}
