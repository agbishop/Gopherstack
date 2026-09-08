package main

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	lambdaclientsdk "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	ecrbackend "github.com/blackbirdworks/gopherstack/services/ecr"
	lambdabackend "github.com/blackbirdworks/gopherstack/services/lambda"
)

// TestInitializeServices_LambdaECRWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than
// invoking wireLambdaECR directly, so that deleting the wiring call from
// wireComputeAndObservabilityIntegrations -- not just breaking the helper
// function itself -- is what this test is sensitive to.
//
// Regression test for gopherstack-zkp9: without an ECRResolver wired,
// CreateFunction's Code.ImageUri validation is a no-op and accepts any
// image reference, including one that does not exist in ECR. This proves
// the real cli.go wiring rejects a nonexistent image and accepts a real one
// pushed to a real ECR repository.
func TestInitializeServices_LambdaECRWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	portAlloc, err := portalloc.New(19200, 19300)
	require.NoError(t, err)

	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
		PortAlloc:  portAlloc,
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	lambdaH, ok := byName["Lambda"].(*lambdabackend.Handler)
	require.True(t, ok, "Lambda handler must be registered")

	lambdaBk, ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend)
	require.True(t, ok, "Lambda backend must be an InMemoryBackend")

	t.Cleanup(func() { lambdaBk.Close(context.Background()) })

	ecrH, ok := byName["ECR"].(*ecrbackend.Handler)
	require.True(t, ok, "ECR handler must be registered")

	ecrBk, ok := ecrH.Backend.(*ecrbackend.InMemoryBackend)
	require.True(t, ok, "ECR backend must be an InMemoryBackend")

	ctx := t.Context()

	repo, err := ecrBk.CreateRepository(ctx, "lambda-ecr-wiring-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	_, err = ecrBk.PutImage(ctx, repo.RepositoryName, ecrbackend.Image{
		ImageID:       ecrbackend.ImageIdentifier{ImageTag: "v1"},
		ImageManifest: `{"schemaVersion":2}`,
	})
	require.NoError(t, err)

	realURI := repo.RepositoryURI + ":v1"
	missingURI := repo.RepositoryURI + ":does-not-exist"

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(lambdaH))
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

	client := lambdaclientsdk.NewFromConfig(cfg, func(o *lambdaclientsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})

	_, err = client.CreateFunction(ctx, &lambdaclientsdk.CreateFunctionInput{
		FunctionName: aws.String("lambda-ecr-wiring-missing-fn"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String(missingURI)},
		Role:         aws.String("arn:aws:iam::000000000000:role/r"),
	})
	require.Error(t, err,
		"a nonexistent ECR image must be rejected through the actual cli.go composition root's ECR wiring")

	_, err = client.CreateFunction(ctx, &lambdaclientsdk.CreateFunctionInput{
		FunctionName: aws.String("lambda-ecr-wiring-real-fn"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String(realURI)},
		Role:         aws.String("arn:aws:iam::000000000000:role/r"),
	})
	require.NoError(t, err, "a real ECR image pushed via the real ECR backend must be accepted")
}
