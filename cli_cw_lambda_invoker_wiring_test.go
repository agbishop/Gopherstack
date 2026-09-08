package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	lambdabackend "github.com/blackbirdworks/gopherstack/services/lambda"
)

// TestCWLambdaInvokerAdapter_ThreadsInvocationType is the regression test for
// gopherstack-pr1r: cwLambdaInvokerAdapter used to drop the invocationType
// argument and hardcode InvocationTypeEvent, silently ignoring whatever a
// caller passed. DryRun distinguishes the two without needing a running
// container: InvokeFunctionWithQualifier returns 204 No Content for DryRun
// but 202 Accepted for Event (services/lambda/invocation.go). Before the
// fix, passing "DryRun" here still got 202 because the value was discarded.
func TestCWLambdaInvokerAdapter_ThreadsInvocationType(t *testing.T) {
	t.Parallel()

	bk := lambdabackend.NewInMemoryBackend(
		nil, nil, lambdabackend.DefaultSettings(), "000000000000", "us-east-1",
	)
	t.Cleanup(func() { bk.Close(context.Background()) })

	require.NoError(t, bk.CreateFunction(&lambdabackend.FunctionConfiguration{
		FunctionName: "cw-invoker-fn",
		PackageType:  lambdabackend.PackageTypeImage,
		Role:         "arn:aws:iam::000000000000:role/r",
		ImageURI:     "x",
	}))

	adapter := &cwLambdaInvokerAdapter{backend: bk}

	payload, status, err := adapter.InvokeFunction(
		context.Background(), "cw-invoker-fn", lambdabackend.InvocationTypeDryRun, []byte("{}"),
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status,
		"invocationType must reach the backend unchanged, not be hardcoded to Event")
	require.Nil(t, payload)
}
