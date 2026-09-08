package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
)

const sfnStateMachineName = "pgoload-sm"

const sfnPassState = "Pass"

// errStateMachineNotFound is returned when a state machine that
// CreateStateMachine reported as already existing can't be located by a
// follow-up ListStateMachines call.
var errStateMachineNotFound = errors.New("state machine exists but was not found by ListStateMachines")

// sfnDefinition is a minimal one-state Pass machine — enough to exercise
// CreateStateMachine/StartExecution/DescribeExecution without needing real
// task integrations.
func sfnDefinition() string {
	def, _ := json.Marshal(map[string]any{
		"StartAt": sfnPassState,
		"States": map[string]any{
			sfnPassState: map[string]any{"Type": "Pass", "End": true},
		},
	})

	return string(def)
}

// ensureStateMachine creates sfnStateMachineName, tolerating one that
// already exists, and returns its ARN.
func ensureStateMachine(ctx context.Context, cl *sfn.Client, roleArn string, log *slog.Logger) (string, error) {
	out, err := cl.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name:       aws.String(sfnStateMachineName),
		Definition: aws.String(sfnDefinition()),
		RoleArn:    aws.String(roleArn),
	})
	if err == nil {
		return aws.ToString(out.StateMachineArn), nil
	}

	if _, ok := errors.AsType[*sfntypes.StateMachineAlreadyExists](err); !ok {
		return "", fmt.Errorf("create state machine %s: %w", sfnStateMachineName, err)
	}

	log.InfoContext(ctx, "step functions state machine already exists", "stateMachine", sfnStateMachineName)

	list, err := cl.ListStateMachines(ctx, &sfn.ListStateMachinesInput{})
	if err != nil {
		return "", fmt.Errorf("list state machines: %w", err)
	}

	for _, sm := range list.StateMachines {
		if aws.ToString(sm.Name) == sfnStateMachineName {
			return aws.ToString(sm.StateMachineArn), nil
		}
	}

	return "", fmt.Errorf("%w: stateMachine=%s", errStateMachineNotFound, sfnStateMachineName)
}

// stepFunctionsWorker repeatedly runs a mix of Step Functions operations,
// staggered by workerID, until ctx is done.
func stepFunctionsWorker(
	ctx context.Context,
	cl *sfn.Client,
	res *resources,
	workerID int,
	c *opCounter,
	log *slog.Logger,
) {
	ops := []opFunc{
		func(ctx context.Context, workerID, i int) error {
			return sfnStartDescribeExecutionOp(ctx, cl, res.stateMachineArn, workerID, i)
		},
		func(ctx context.Context, _, _ int) error {
			return sfnListExecutionsOp(ctx, cl, res.stateMachineArn)
		},
		func(ctx context.Context, _, _ int) error { return sfnListStateMachinesOp(ctx, cl) },
	}

	runOpLoop(ctx, workerID, ops, c, "stepfunctions", log)
}

// sfnStartDescribeExecutionOp starts an execution and immediately describes
// it, exercising both wire paths without needing shared execution state.
func sfnStartDescribeExecutionOp(ctx context.Context, cl *sfn.Client, stateMachineArn string, workerID, i int) error {
	started, err := cl.StartExecution(ctx, &sfn.StartExecutionInput{
		StateMachineArn: aws.String(stateMachineArn),
		Name:            aws.String(fmt.Sprintf("pgoload-exec-%d-%d", workerID, i)),
	})
	if err != nil {
		return fmt.Errorf("start execution: %w", err)
	}

	if _, descErr := cl.DescribeExecution(ctx, &sfn.DescribeExecutionInput{
		ExecutionArn: started.ExecutionArn,
	}); descErr != nil {
		return fmt.Errorf("describe execution: %w", descErr)
	}

	return nil
}

func sfnListExecutionsOp(ctx context.Context, cl *sfn.Client, stateMachineArn string) error {
	_, err := cl.ListExecutions(ctx, &sfn.ListExecutionsInput{StateMachineArn: aws.String(stateMachineArn)})

	return err
}

func sfnListStateMachinesOp(ctx context.Context, cl *sfn.Client) error {
	_, err := cl.ListStateMachines(ctx, &sfn.ListStateMachinesInput{})

	return err
}
