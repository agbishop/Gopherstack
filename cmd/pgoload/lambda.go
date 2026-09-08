package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

const (
	lambdaFunctionName = "pgoload-fn"
	lambdaWaitTimeout  = 30 * time.Second
)

// ensureLambdaFunction creates lambdaFunctionName from an in-memory zip,
// tolerating one that already exists, and waits (best-effort, bounded) for
// it to leave the Pending state. Returns the function name.
func ensureLambdaFunction(ctx context.Context, cl *lambda.Client, roleArn string, log *slog.Logger) (string, error) {
	_, err := cl.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(lambdaFunctionName),
		Runtime:      lambdatypes.RuntimeNodejs20x,
		Handler:      aws.String("index.handler"),
		Role:         aws.String(roleArn),
		PackageType:  lambdatypes.PackageTypeZip,
		Code:         &lambdatypes.FunctionCode{ZipFile: lambdaDummyZip()},
	})
	if err != nil {
		if _, ok := errors.AsType[*lambdatypes.ResourceConflictException](err); !ok {
			return "", fmt.Errorf("create function %s: %w", lambdaFunctionName, err)
		}

		log.InfoContext(ctx, "lambda function already exists", "function", lambdaFunctionName)
	}

	waitCtx, cancel := context.WithTimeout(ctx, lambdaWaitTimeout)
	defer cancel()

	for {
		out, getErr := cl.GetFunction(waitCtx, &lambda.GetFunctionInput{FunctionName: aws.String(lambdaFunctionName)})
		if getErr == nil && out.Configuration != nil && out.Configuration.State != lambdatypes.StatePending {
			return lambdaFunctionName, nil
		}

		t := time.NewTimer(pollInterval)
		select {
		case <-waitCtx.Done():
			t.Stop()
			log.WarnContext(
				ctx, "gave up waiting for lambda function to leave Pending state", "function", lambdaFunctionName,
			)

			return lambdaFunctionName, nil
		case <-t.C:
		}
	}
}

func lambdaDummyZip() []byte {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	f, _ := w.Create("index.js")
	_, _ = f.Write([]byte("exports.handler = async () => ({statusCode: 200, body: 'pgoload'});"))
	_ = w.Close()

	return buf.Bytes()
}

// lambdaWorker repeatedly runs a mix of Lambda operations, staggered by
// workerID, until ctx is done.
func lambdaWorker(
	ctx context.Context,
	cl *lambda.Client,
	res *resources,
	workerID int,
	c *opCounter,
	log *slog.Logger,
) {
	ops := []opFunc{
		func(ctx context.Context, _, _ int) error { return lambdaInvokeOp(ctx, cl, res.functionName) },
		func(ctx context.Context, _, _ int) error { return lambdaInvokeOp(ctx, cl, res.functionName) },
		func(ctx context.Context, _, _ int) error {
			return lambdaGetFunctionOp(ctx, cl, res.functionName)
		},
		func(ctx context.Context, _, _ int) error { return lambdaListFunctionsOp(ctx, cl) },
	}

	runOpLoop(ctx, workerID, ops, c, "lambda", log)
}

func lambdaInvokeOp(ctx context.Context, cl *lambda.Client, functionName string) error {
	_, err := cl.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(functionName),
		Payload:      []byte(`{"pgoload":true}`),
	})

	return err
}

func lambdaGetFunctionOp(ctx context.Context, cl *lambda.Client, functionName string) error {
	_, err := cl.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: aws.String(functionName)})

	return err
}

func lambdaListFunctionsOp(ctx context.Context, cl *lambda.Client) error {
	_, err := cl.ListFunctions(ctx, &lambda.ListFunctionsInput{})

	return err
}
