package elbv2_test

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/elbv2"
)

// fakeEC2Resolver is a test double for elbv2.EC2Resolver backed by static sets,
// standing in for the real services/ec2 backend cli.go wires in.
type fakeEC2Resolver struct {
	securityGroups map[string]bool
	subnets        map[string]bool
}

func (f *fakeEC2Resolver) SecurityGroupExists(id string) bool { return f.securityGroups[id] }
func (f *fakeEC2Resolver) SubnetExists(id string) bool        { return f.subnets[id] }

// fakeCertResolver is a test double for elbv2.CertificateResolver backed by a
// static set, standing in for the real services/acm and services/iam backends
// cli.go wires in. It also records AddInUseBy/RemoveInUseBy calls so tests can
// assert the usage-reporting half independently of the validation half.
type fakeCertResolver struct {
	certs map[string]bool
	// inUse maps certARN -> resourceARN -> attached.
	inUse map[string]map[string]bool
}

func newFakeCertResolver(known ...string) *fakeCertResolver {
	certs := make(map[string]bool, len(known))
	for _, c := range known {
		certs[c] = true
	}

	return &fakeCertResolver{certs: certs, inUse: make(map[string]map[string]bool)}
}

func (f *fakeCertResolver) ResolveCertificate(certARN string) bool { return f.certs[certARN] }

func (f *fakeCertResolver) AddInUseBy(certARN, resourceARN string) {
	if f.inUse[certARN] == nil {
		f.inUse[certARN] = make(map[string]bool)
	}

	f.inUse[certARN][resourceARN] = true
}

func (f *fakeCertResolver) RemoveInUseBy(certARN, resourceARN string) {
	delete(f.inUse[certARN], resourceARN)
}

func (f *fakeCertResolver) isInUseBy(certARN, resourceARN string) bool {
	return f.inUse[certARN][resourceARN]
}

func newCrossServiceHandler() *elbv2.InMemoryBackend {
	return elbv2.NewInMemoryBackend("123456789012", config.DefaultRegion)
}

func requireAPIErrorCode(t *testing.T, err error, code string) {
	t.Helper()

	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, code, apiErr.ErrorCode())
}

func TestCreateLoadBalancer_EC2Resolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		resolver  elbv2.EC2Resolver
		name      string
		sg        string
		subnet    string
		wantCode  string
		wantError bool
	}{
		{
			name:     "no_resolver_wired_accepts_any_id",
			resolver: nil,
			sg:       "sg-unknown",
			subnet:   "subnet-unknown",
		},
		{
			name: "known_sg_and_subnet_accepted",
			resolver: &fakeEC2Resolver{
				securityGroups: map[string]bool{"sg-real": true},
				subnets:        map[string]bool{"subnet-real": true},
			},
			sg:     "sg-real",
			subnet: "subnet-real",
		},
		{
			name: "unknown_security_group_rejected",
			resolver: &fakeEC2Resolver{
				securityGroups: map[string]bool{},
				subnets:        map[string]bool{"subnet-real": true},
			},
			sg:        "sg-unknown",
			subnet:    "subnet-real",
			wantError: true,
			wantCode:  "InvalidSecurityGroup",
		},
		{
			name: "unknown_subnet_rejected",
			resolver: &fakeEC2Resolver{
				securityGroups: map[string]bool{"sg-real": true},
				subnets:        map[string]bool{},
			},
			sg:        "sg-real",
			subnet:    "subnet-unknown",
			wantError: true,
			wantCode:  "SubnetNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newCrossServiceHandler()
			if tt.resolver != nil {
				backend.SetEC2Resolver(tt.resolver)
			}

			h := elbv2.NewHandler(backend)
			client := newTestELBv2Client(t, h)

			_, err := client.CreateLoadBalancer(t.Context(), &elbv2sdk.CreateLoadBalancerInput{
				Name:           aws.String(strings.ReplaceAll(tt.name, "_", "-")),
				Subnets:        []string{tt.subnet},
				SecurityGroups: []string{tt.sg},
			})

			if tt.wantError {
				requireAPIErrorCode(t, err, tt.wantCode)

				return
			}

			require.NoError(t, err)
		})
	}
}

// mustCreateALBListenerFixture creates an ALB and returns its ARN, for tests
// that only care about listener-level certificate validation/reporting.
func mustCreateALBListenerFixture(t *testing.T, client *elbv2sdk.Client, name string) string {
	t.Helper()

	out, err := client.CreateLoadBalancer(t.Context(), &elbv2sdk.CreateLoadBalancerInput{
		Name: aws.String(name),
		Type: elbv2types.LoadBalancerTypeEnumApplication,
	})
	require.NoError(t, err)
	require.Len(t, out.LoadBalancers, 1)

	return aws.ToString(out.LoadBalancers[0].LoadBalancerArn)
}

func TestCreateListener_CertificateResolver(t *testing.T) {
	t.Parallel()

	const knownCert = "arn:aws:acm:us-east-1:123456789012:certificate/known"

	tests := []struct {
		resolver  elbv2.CertificateResolver
		name      string
		lbName    string
		certARN   string
		wantError bool
	}{
		{
			name:     "no_resolver_wired_accepts_any_cert",
			lbName:   "lb-cl-unwired",
			resolver: nil,
			certARN:  "arn:aws:acm:us-east-1:123456789012:certificate/unknown",
		},
		{
			name:     "known_certificate_accepted",
			lbName:   "lb-cl-known",
			resolver: newFakeCertResolver(knownCert),
			certARN:  knownCert,
		},
		{
			name:      "unknown_certificate_rejected",
			lbName:    "lb-cl-unknown",
			resolver:  newFakeCertResolver(knownCert),
			certARN:   "arn:aws:acm:us-east-1:123456789012:certificate/unknown",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newCrossServiceHandler()
			if tt.resolver != nil {
				backend.SetCertificateResolver(tt.resolver)
			}

			h := elbv2.NewHandler(backend)
			client := newTestELBv2Client(t, h)
			lbArn := mustCreateALBListenerFixture(t, client, tt.lbName)

			out, err := client.CreateListener(t.Context(), &elbv2sdk.CreateListenerInput{
				LoadBalancerArn: aws.String(lbArn),
				Protocol:        elbv2types.ProtocolEnumHttps,
				Port:            aws.Int32(443),
				Certificates:    []elbv2types.Certificate{{CertificateArn: aws.String(tt.certARN)}},
				DefaultActions: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnumFixedResponse,
						FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
							StatusCode: aws.String("200"),
						},
					},
				},
			})

			if tt.wantError {
				requireAPIErrorCode(t, err, "CertificateNotFound")

				return
			}

			require.NoError(t, err)
			require.Len(t, out.Listeners, 1)

			listenerArn := aws.ToString(out.Listeners[0].ListenerArn)
			if fake, ok := tt.resolver.(*fakeCertResolver); ok {
				assert.True(t, fake.isInUseBy(tt.certARN, listenerArn),
					"CreateListener must report the attached certificate as in-use")
			}
		})
	}
}

func TestModifyListener_CertificateResolver(t *testing.T) {
	t.Parallel()

	const (
		certA = "arn:aws:acm:us-east-1:123456789012:certificate/a"
		certB = "arn:aws:acm:us-east-1:123456789012:certificate/b"
		certC = "arn:aws:acm:us-east-1:123456789012:certificate/unknown"
	)

	resolver := newFakeCertResolver(certA, certB)
	backend := newCrossServiceHandler()
	backend.SetCertificateResolver(resolver)
	h := elbv2.NewHandler(backend)
	client := newTestELBv2Client(t, h)
	ctx := t.Context()

	lbArn := mustCreateALBListenerFixture(t, client, "lb-modify-listener")

	createOut, err := client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumHttps,
		Port:            aws.Int32(443),
		Certificates:    []elbv2types.Certificate{{CertificateArn: aws.String(certA)}},
		DefaultActions: []elbv2types.Action{
			{Type: elbv2types.ActionTypeEnumFixedResponse, FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
				StatusCode: aws.String("200"),
			}},
		},
	})
	require.NoError(t, err)
	listenerArn := aws.ToString(createOut.Listeners[0].ListenerArn)
	require.True(t, resolver.isInUseBy(certA, listenerArn))

	// Reject an unknown certificate; the old certificate must remain in use.
	_, err = client.ModifyListener(ctx, &elbv2sdk.ModifyListenerInput{
		ListenerArn:  aws.String(listenerArn),
		Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(certC)}},
	})
	requireAPIErrorCode(t, err, "CertificateNotFound")
	assert.True(t, resolver.isInUseBy(certA, listenerArn), "rejected swap must not unmark the existing certificate")

	// Swap to a known certificate: old must be unmarked, new marked.
	_, err = client.ModifyListener(ctx, &elbv2sdk.ModifyListenerInput{
		ListenerArn:  aws.String(listenerArn),
		Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(certB)}},
	})
	require.NoError(t, err)
	assert.False(t, resolver.isInUseBy(certA, listenerArn), "swapping away from certA must unmark it")
	assert.True(t, resolver.isInUseBy(certB, listenerArn), "swapping to certB must mark it")
}

func TestAddRemoveListenerCertificates_CertificateResolver(t *testing.T) {
	t.Parallel()

	const (
		certA = "arn:aws:acm:us-east-1:123456789012:certificate/a"
		certB = "arn:aws:acm:us-east-1:123456789012:certificate/b"
		certC = "arn:aws:acm:us-east-1:123456789012:certificate/unknown"
	)

	resolver := newFakeCertResolver(certA, certB)
	backend := newCrossServiceHandler()
	backend.SetCertificateResolver(resolver)
	h := elbv2.NewHandler(backend)
	client := newTestELBv2Client(t, h)
	ctx := t.Context()

	lbArn := mustCreateALBListenerFixture(t, client, "lb-add-remove-certs")

	createOut, err := client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumHttps,
		Port:            aws.Int32(443),
		Certificates:    []elbv2types.Certificate{{CertificateArn: aws.String(certA)}},
		DefaultActions: []elbv2types.Action{
			{Type: elbv2types.ActionTypeEnumFixedResponse, FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
				StatusCode: aws.String("200"),
			}},
		},
	})
	require.NoError(t, err)
	listenerArn := aws.ToString(createOut.Listeners[0].ListenerArn)

	// AddListenerCertificates rejects an unknown certificate.
	_, err = client.AddListenerCertificates(ctx, &elbv2sdk.AddListenerCertificatesInput{
		ListenerArn:  aws.String(listenerArn),
		Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(certC)}},
	})
	requireAPIErrorCode(t, err, "CertificateNotFound")

	// AddListenerCertificates accepts and marks a known certificate.
	_, err = client.AddListenerCertificates(ctx, &elbv2sdk.AddListenerCertificatesInput{
		ListenerArn:  aws.String(listenerArn),
		Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(certB)}},
	})
	require.NoError(t, err)
	assert.True(t, resolver.isInUseBy(certB, listenerArn))

	// RemoveListenerCertificates unmarks the removed certificate; certA stays in use.
	_, err = client.RemoveListenerCertificates(ctx, &elbv2sdk.RemoveListenerCertificatesInput{
		ListenerArn:  aws.String(listenerArn),
		Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(certB)}},
	})
	require.NoError(t, err)
	assert.False(t, resolver.isInUseBy(certB, listenerArn), "removed certificate must be unmarked")
	assert.True(t, resolver.isInUseBy(certA, listenerArn), "untouched certificate must remain marked")
}

func TestDeleteListener_UnmarksCertificatesInUse(t *testing.T) {
	t.Parallel()

	const certA = "arn:aws:acm:us-east-1:123456789012:certificate/a"

	resolver := newFakeCertResolver(certA)
	backend := newCrossServiceHandler()
	backend.SetCertificateResolver(resolver)
	h := elbv2.NewHandler(backend)
	client := newTestELBv2Client(t, h)
	ctx := t.Context()

	lbArn := mustCreateALBListenerFixture(t, client, "lb-delete-listener")

	createOut, err := client.CreateListener(ctx, &elbv2sdk.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Protocol:        elbv2types.ProtocolEnumHttps,
		Port:            aws.Int32(443),
		Certificates:    []elbv2types.Certificate{{CertificateArn: aws.String(certA)}},
		DefaultActions: []elbv2types.Action{
			{Type: elbv2types.ActionTypeEnumFixedResponse, FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
				StatusCode: aws.String("200"),
			}},
		},
	})
	require.NoError(t, err)
	listenerArn := aws.ToString(createOut.Listeners[0].ListenerArn)
	require.True(t, resolver.isInUseBy(certA, listenerArn))

	_, err = client.DeleteListener(ctx, &elbv2sdk.DeleteListenerInput{
		ListenerArn: aws.String(listenerArn),
	})
	require.NoError(t, err)
	assert.False(t, resolver.isInUseBy(certA, listenerArn), "deleting the listener must unmark its certificates")
}
