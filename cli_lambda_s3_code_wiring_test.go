package main

import (
	"archive/zip"
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	lambdabackend "github.com/blackbirdworks/gopherstack/services/lambda"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
)

// makeLambdaS3WiringZip builds a minimal in-memory zip containing a Node.js handler.
func makeLambdaS3WiringZip(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	f, err := w.Create("index.js")
	require.NoError(t, err)

	_, err = f.Write([]byte(`exports.handler = async () => ({ statusCode: 200, body: "ok" });`))
	require.NoError(t, err)

	require.NoError(t, w.Close())

	return buf.Bytes()
}

// TestInitializeServices_LambdaS3CodeWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireLambdaS3 directly, so that deleting the wiring call from wireCrossServiceDependencies
// -- not just breaking the helper function itself -- is what this test is sensitive to.
//
// Regression test for a function deployed from Code.S3Bucket/S3Key: without S3CodeFetcher
// wired, startZipContainer always returns "ErrLambdaUnavailable: S3 code delivery requires S3
// integration" for such a function, before ever touching Docker. This asserts the real
// production Invoke path (InvokeFunction, Event/async) gets past that gate and actually
// fetches the zip from the real S3 backend and starts a real container from it -- it does not
// wait for the container's Lambda Runtime Interface Client to respond, since that is
// orthogonal to the S3-wiring bug this covers.
func TestInitializeServices_LambdaS3CodeWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	portAlloc, err := portalloc.New(19000, 19100)
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

	s3H, ok := byName["S3"].(*s3backend.S3Handler)
	require.True(t, ok, "S3 handler must be registered")

	s3Bk, ok := s3H.Backend.(*s3backend.InMemoryBackend)
	require.True(t, ok, "S3 backend must be an InMemoryBackend")

	ctx := t.Context()

	bucketName := "lambda-s3-code-wiring-bucket"
	_, err = s3Bk.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)

	key := "functions/lambda-s3-wiring-fn.zip"
	_, err = s3Bk.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
		Body:   bytes.NewReader(makeLambdaS3WiringZip(t)),
	})
	require.NoError(t, err)

	fn := &lambdabackend.FunctionConfiguration{
		FunctionName: "lambda-s3-wiring-fn",
		PackageType:  lambdabackend.PackageTypeZip,
		Runtime:      "nodejs20.x",
		Handler:      "index.handler",
		Role:         "arn:aws:iam::000000000000:role/r",
		S3BucketCode: bucketName,
		S3KeyCode:    key,
	}
	require.NoError(t, lambdaBk.CreateFunction(fn))

	_, status, invokeErr := lambdaBk.InvokeFunction(
		ctx, fn.FunctionName, lambdabackend.InvocationTypeEvent, []byte("{}"),
	)
	if invokeErr != nil && strings.Contains(invokeErr.Error(), "container runtime unavailable") {
		t.Skip("container runtime unavailable in this environment")
	}

	require.NoError(t, invokeErr,
		"a function deployed from Code.S3Bucket/S3Key must actually fetch its code and start "+
			"through the real cli.go composition root's S3 wiring (wireLambdaS3), not fail with "+
			"ErrLambdaUnavailable: S3 code delivery requires S3 integration")
	require.Equal(t, 202, status)
}
