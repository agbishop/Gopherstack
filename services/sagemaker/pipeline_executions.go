package sagemaker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Additional pipeline execution operations
// ---------------------------------------------------------------------------

// PipelineExecutionStep represents a step within a pipeline execution.
type PipelineExecutionStep struct {
	StartTime        time.Time           `json:"StartTime"`
	EndTime          time.Time           `json:"EndTime"`
	StepName         string              `json:"StepName"`
	StepType         string              `json:"StepType"`
	StepStatus       string              `json:"StepStatus"`
	FailureReason    string              `json:"FailureReason,omitempty"`
	CallbackToken    string              `json:"CallbackToken,omitempty"`
	ExecutionArn     string              `json:"executionArn"`
	OutputParameters []PipelineParameter `json:"OutputParameters,omitempty"`
}

const (
	startTransitionDelay = 200 * time.Millisecond // delay for started execution to succeed
	retryTransitionDelay = 200 * time.Millisecond // delay for retry execution to succeed
	stopTransitionDelay  = 100 * time.Millisecond // delay for execution to stop
)

const (
	pipelineStatusExecuting = "Executing"
	pipelineStatusSucceeded = "Succeeded"
	pipelineStatusStopping  = "Stopping"
	pipelineStatusStopped   = "Stopped"
	stepTypeCallback        = "Callback"
	stepStatusSucceeded     = "Succeeded"
	stepStatusFailed        = "Failed"
	// pipelineCallbackStepName is the fixed step name used to record a
	// callback step. The real SendPipelineExecutionStepSuccess/Failure API
	// (api_op_SendPipelineExecutionStepSuccess.go:29-43, api_op_
	// SendPipelineExecutionStepFailure.go:29-42) carries only CallbackToken,
	// never a step name or execution ARN — AWS resolves the step from the
	// opaque token it generated when the callback step started. This backend
	// has no pipeline-definition step graph to generate or track distinct
	// per-step tokens, so by disclosed convention it treats the caller-
	// supplied CallbackToken as the target execution's ARN and records at
	// most one trackable callback step per execution under this fixed name.
	pipelineCallbackStepName = "callback-step"
)

// pipelineExecutionStepsKey builds the map key for step records.
func pipelineExecutionStepsKey(execArn, stepName string) string {
	return execArn + "|" + stepName
}

// RetryPipelineExecution creates a new execution from a failed pipeline
// execution. parallelismConfig, if non-nil, overrides the parent pipeline's
// parallelism configuration for the new execution only; if nil, the parent
// pipeline's current configuration applies unchanged (api_op_
// RetryPipelineExecution.go:34-38, sagemaker@v1.263.2 — "if specified,
// overrides the parallelism configuration of the parent pipeline").
func (b *InMemoryBackend) RetryPipelineExecution(
	ctx context.Context,
	execArn string,
	parallelismConfig *ParallelismConfiguration,
) (*PipelineExecution, error) {
	b.mu.Lock("RetryPipelineExecution")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	pe, ok := b.pipelineExecutionsStore(region).Get(execArn)
	if !ok {
		return nil, fmt.Errorf(
			"%w: pipeline execution %q not found",
			ErrPipelineExecutionNotFound,
			execArn,
		)
	}

	effectiveParallelism := parallelismConfig
	if effectiveParallelism == nil {
		if p, found := b.findPipelineByARNLocked(region, pe.PipelineArn); found {
			effectiveParallelism = p.ParallelismConfiguration
		}
	}

	newID := generateID()
	newArn := pe.PipelineArn + "/execution/" + newID
	now := time.Now()

	newExec := &PipelineExecution{
		PipelineArn:              pe.PipelineArn,
		PipelineExecutionArn:     newArn,
		PipelineExecutionStatus:  pipelineStatusExecuting,
		ParallelismConfiguration: effectiveParallelism,
		StartTime:                now,
	}
	b.pipelineExecutionsStore(region).Put(newExec)

	// Transition to Succeeded after a short delay.
	b.runDelayed(b.lifecycleCtx, retryTransitionDelay, func() {
		b.mu.Lock("RetryPipelineExecution.goroutine")
		defer b.mu.Unlock()

		if exec, exists := b.pipelineExecutionsStore(region).Get(newArn); exists {
			exec.PipelineExecutionStatus = pipelineStatusSucceeded
		}
	})

	return clonePipelineExecution(newExec), nil
}

// StopPipelineExecution stops a running pipeline execution.
func (b *InMemoryBackend) StopPipelineExecution(ctx context.Context, execArn string) (*PipelineExecution, error) {
	b.mu.Lock("StopPipelineExecution")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	pe, ok := b.pipelineExecutionsStore(region).Get(execArn)
	if !ok {
		return nil, fmt.Errorf(
			"%w: pipeline execution %q not found",
			ErrPipelineExecutionNotFound,
			execArn,
		)
	}

	pe.PipelineExecutionStatus = pipelineStatusStopping
	cp := clonePipelineExecution(pe)

	// Transition to Stopped after a short delay.
	b.runDelayed(b.lifecycleCtx, stopTransitionDelay, func() {
		b.mu.Lock("StopPipelineExecution.goroutine")
		defer b.mu.Unlock()

		if exec, exists := b.pipelineExecutionsStore(region).Get(execArn); exists {
			exec.PipelineExecutionStatus = pipelineStatusStopped
		}
	})

	return cp, nil
}

// SendPipelineExecutionStepSuccess records a step success for a callback
// step. callbackToken is treated as the owning execution's ARN — see
// pipelineCallbackStepName's doc comment for why.
func (b *InMemoryBackend) SendPipelineExecutionStepSuccess(
	ctx context.Context,
	callbackToken string,
	outputParameters []PipelineParameter,
) error {
	b.mu.Lock("SendPipelineExecutionStepSuccess")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.pipelineExecutionsStore(region).Get(callbackToken); !ok {
		return fmt.Errorf(
			"%w: pipeline execution %q not found",
			ErrPipelineExecutionNotFound,
			callbackToken,
		)
	}

	now := time.Now()

	b.pipelineExecStepsStore(region).Put(&PipelineExecutionStep{
		StartTime:        now,
		EndTime:          now,
		StepName:         pipelineCallbackStepName,
		StepType:         stepTypeCallback,
		StepStatus:       stepStatusSucceeded,
		CallbackToken:    callbackToken,
		OutputParameters: outputParameters,
		ExecutionArn:     callbackToken,
	})

	return nil
}

// SendPipelineExecutionStepFailure records a step failure for a callback
// step. callbackToken is treated as the owning execution's ARN — see
// pipelineCallbackStepName's doc comment for why.
func (b *InMemoryBackend) SendPipelineExecutionStepFailure(
	ctx context.Context, callbackToken, failureReason string,
) error {
	b.mu.Lock("SendPipelineExecutionStepFailure")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.pipelineExecutionsStore(region).Get(callbackToken); !ok {
		return fmt.Errorf(
			"%w: pipeline execution %q not found",
			ErrPipelineExecutionNotFound,
			callbackToken,
		)
	}

	now := time.Now()

	b.pipelineExecStepsStore(region).Put(&PipelineExecutionStep{
		StartTime:     now,
		EndTime:       now,
		StepName:      pipelineCallbackStepName,
		StepType:      stepTypeCallback,
		StepStatus:    stepStatusFailed,
		FailureReason: failureReason,
		CallbackToken: callbackToken,
		ExecutionArn:  callbackToken,
	})

	return nil
}

// ListPipelineExecutionStepsParams bundles ListPipelineExecutionSteps'
// pagination/sort input (api_op_ListPipelineExecutionSteps.go:29-43,
// sagemaker@v1.263.2 — the op has no SortBy, only SortOrder, so results
// always sort by StartTime, the real API's stated default sort field).
type ListPipelineExecutionStepsParams struct {
	ExecutionArn string
	NextToken    string
	SortOrder    string
	MaxResults   int32
}

// ListPipelineExecutionSteps lists the steps for a pipeline execution.
func (b *InMemoryBackend) ListPipelineExecutionSteps(
	ctx context.Context, params ListPipelineExecutionStepsParams,
) ([]*PipelineExecutionStep, string) {
	b.mu.RLock("ListPipelineExecutionSteps")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	list := make([]*PipelineExecutionStep, 0, b.pipelineExecStepsStoreRO(region).Len())

	for _, step := range b.pipelineExecStepsStoreRO(region).All() {
		if step.ExecutionArn == params.ExecutionArn {
			cp := *step
			list = append(list, &cp)
		}
	}

	desc := strings.EqualFold(params.SortOrder, sortOrderDescending)
	sort.Slice(list, func(i, j int) bool {
		less := list[i].StartTime.Before(list[j].StartTime)
		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, params.NextToken, params.MaxResults)
}
