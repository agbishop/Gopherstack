package main

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
	s3controlbackend "github.com/blackbirdworks/gopherstack/services/s3control"
)

// TestInitializeServices_S3ControlObjectLambdaWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireS3ControlObjectLambda directly, so that deleting the wiring call from
// wireStorageAndSecretsIntegrations -- not just breaking the helper function itself -- is
// what this test is sensitive to.
//
// Regression test for gopherstack-6o0r: S3Control's
// PutAccessPointConfigurationForObjectLambda accepted and stored an Object Lambda
// configuration but never reached the S3 backend, so h.objectLambdaConfigs stayed empty and
// GetObject on the underlying bucket always served the plain object -- Object Lambda Access
// Points were a complete no-op. This asserts the real behavioral difference: once a
// SupportingAccessPoint's underlying bucket has an Object Lambda config, GetObject on that
// bucket must route through the (real, composition-root) Lambda invocation path instead of
// serving the object directly -- observed here by pointing the config at a Lambda function
// that does not exist, so the real InvokeFunction call fails and GetObject errors, instead of
// returning 200 with the plain object body.
func TestInitializeServices_S3ControlObjectLambdaWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	s3H, ok := byName["S3"].(*s3backend.S3Handler)
	require.True(t, ok, "S3 handler must be registered")

	s3Bk, ok := s3H.Backend.(*s3backend.InMemoryBackend)
	require.True(t, ok, "S3 backend must be an InMemoryBackend")

	s3cH, ok := byName["S3Control"].(*s3controlbackend.Handler)
	require.True(t, ok, "S3Control handler must be registered")
	require.NotNil(t, s3cH.Backend, "S3Control backend must be set")

	ctx := t.Context()
	accountID := "000000000000"
	bucketName := "object-lambda-wiring-bucket"
	objectKey := "wiring-object.txt"
	objectBody := "plain object body"

	_, err = s3Bk.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)

	_, err = s3Bk.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   strings.NewReader(objectBody),
	})
	require.NoError(t, err)

	ap := s3cH.Backend.CreateAccessPoint(accountID, "object-lambda-wiring-ap", bucketName)
	require.NotEmpty(t, ap.AccessPointArn)

	olap := s3cH.Backend.CreateAccessPointForObjectLambda(accountID, "object-lambda-wiring-olap")
	require.NotEmpty(t, olap.ObjectLambdaAccessPointArn)

	configXML := `<SupportingAccessPoint>` + ap.AccessPointArn + `</SupportingAccessPoint>` +
		`<TransformationConfigurations><TransformationConfiguration><Actions><Action>GetObject</Action></Actions>` +
		`<ContentTransformation><AwsLambda><FunctionArn>` +
		`arn:aws:lambda:us-east-1:000000000000:function:object-lambda-wiring-missing-fn` +
		`</FunctionArn></AwsLambda></ContentTransformation></TransformationConfiguration></TransformationConfigurations>`

	require.NoError(
		t,
		s3cH.Backend.PutAccessPointConfigurationForObjectLambda(accountID, "object-lambda-wiring-olap", configXML),
	)

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(s3H))
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

	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(srv.URL)
	})

	_, err = client.GetObject(context.Background(), &awss3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	require.Error(
		t,
		err,
		"once s3control's PutAccessPointConfigurationForObjectLambda resolves to this bucket "+
			"through the real cli.go composition root's wiring (wireS3ControlObjectLambda), "+
			"GetObject must route through the (here: failing, nonexistent-function) Lambda "+
			"transform instead of serving the plain object",
	)
}
