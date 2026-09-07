// White-box: needs startTransitionDelay directly, mirroring
// instance_refreshes_async_test.go (autoscaling).
package sagemaker //nolint:testpackage // see comment above

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// TestStartPipelineExecution_TransitionsThroughExecuting verifies that
// StartPipelineExecution and StartPipelineExecutionFull start an execution
// in Executing and only reach Succeeded after startTransitionDelay, matching
// RetryPipelineExecution/StopPipelineExecution's async-transition pattern
// (gopherstack-z5hj).
func TestStartPipelineExecution_TransitionsThroughExecuting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		start func(t *testing.T, b *InMemoryBackend, pipelineName string) *PipelineExecution
		name  string
	}{
		{
			name: "plain",
			start: func(t *testing.T, b *InMemoryBackend, pipelineName string) *PipelineExecution {
				t.Helper()

				exec, err := b.StartPipelineExecution(context.Background(), pipelineName)
				if err != nil {
					t.Fatalf("StartPipelineExecution: %v", err)
				}

				return exec
			},
		},
		{
			name: "full",
			start: func(t *testing.T, b *InMemoryBackend, pipelineName string) *PipelineExecution {
				t.Helper()

				exec, err := b.StartPipelineExecutionFull(context.Background(), StartPipelineExecutionOptions{
					PipelineName: pipelineName,
				})
				if err != nil {
					t.Fatalf("StartPipelineExecutionFull: %v", err)
				}

				return exec
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := NewInMemoryBackend("000000000000", "us-east-1")
				defer b.Shutdown(context.Background())

				pipelineName := "start-pipe-" + tc.name
				if _, err := b.CreatePipeline(context.Background(), pipelineName, "{}", "role", nil); err != nil {
					t.Fatalf("CreatePipeline: %v", err)
				}

				exec := tc.start(t, b, pipelineName)
				if exec.PipelineExecutionStatus != pipelineStatusExecuting {
					t.Fatalf("status returned by start = %q, want %q",
						exec.PipelineExecutionStatus, pipelineStatusExecuting)
				}

				got, err := b.DescribePipelineExecution(context.Background(), exec.PipelineExecutionArn)
				if err != nil {
					t.Fatalf("DescribePipelineExecution: %v", err)
				}
				if got.PipelineExecutionStatus != pipelineStatusExecuting {
					t.Fatalf("status before delay = %q, want %q (must not report a terminal status early)",
						got.PipelineExecutionStatus, pipelineStatusExecuting)
				}

				time.Sleep(startTransitionDelay + time.Millisecond)
				synctest.Wait()

				got, err = b.DescribePipelineExecution(context.Background(), exec.PipelineExecutionArn)
				if err != nil {
					t.Fatalf("DescribePipelineExecution after delay: %v", err)
				}
				if got.PipelineExecutionStatus != pipelineStatusSucceeded {
					t.Fatalf("status after delay = %q, want %q", got.PipelineExecutionStatus, pipelineStatusSucceeded)
				}
			})
		})
	}
}
