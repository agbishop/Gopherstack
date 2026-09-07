package sagemaker_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sagemaker"
)

const (
	lifecycleAccountID = "123456789012"
	lifecycleRegion    = "us-east-1"
)

// seedPipelineExecution creates a pipeline and an initial execution, returning the
// execution ARN.
func seedPipelineExecution(t *testing.T, b *sagemaker.InMemoryBackend) string {
	t.Helper()

	_, err := b.CreatePipeline(context.Background(), "pipe", "{}", "role", nil)
	require.NoError(t, err)

	exec, err := b.StartPipelineExecution(context.Background(), "pipe")
	require.NoError(t, err)

	return exec.PipelineExecutionArn
}

// statusOf returns the current status of a pipeline execution.
func statusOf(t *testing.T, b *sagemaker.InMemoryBackend, execArn string) string {
	t.Helper()

	pe, err := b.DescribePipelineExecution(context.Background(), execArn)
	require.NoError(t, err)

	return pe.PipelineExecutionStatus
}

// TestPipelineExecutionTransitionsFire verifies that the delayed status
// transitions still fire after their delay once scheduled via runDelayed.
func TestPipelineExecutionTransitionsFire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		act        func(t *testing.T, b *sagemaker.InMemoryBackend, execArn string) string
		wantStatus string
	}{
		{
			name: "retry transitions to Succeeded",
			act: func(t *testing.T, b *sagemaker.InMemoryBackend, execArn string) string {
				t.Helper()
				retried, err := b.RetryPipelineExecution(context.Background(), execArn, nil)
				require.NoError(t, err)

				return retried.PipelineExecutionArn
			},
			wantStatus: "Succeeded",
		},
		{
			// Stopped must survive past startTransitionDelay: the Start callback
			// fires 100ms after this Stop lands and used to overwrite it
			// (gopherstack-7lrq). assert.Eventually hid that by returning the
			// moment Stopped first appeared, before the clobber.
			name: "stop transitions to Stopped",
			act: func(t *testing.T, b *sagemaker.InMemoryBackend, execArn string) string {
				t.Helper()
				_, err := b.StopPipelineExecution(context.Background(), execArn)
				require.NoError(t, err)

				return execArn
			},
			wantStatus: "Stopped",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := sagemaker.NewInMemoryBackendWithContext(
					t.Context(),
					lifecycleAccountID,
					lifecycleRegion,
				)
				execArn := seedPipelineExecution(t, b)
				target := tc.act(t, b, execArn)

				time.Sleep(time.Second)
				synctest.Wait()

				assert.Equal(t, tc.wantStatus, statusOf(t, b, target), "transition did not fire")
			})
		})
	}
}

// TestShutdownCancelsPendingTransitions verifies that Shutdown cancels in-flight
// lifecycle goroutines so they do not mutate state after the backend is discarded,
// and that Shutdown returns promptly (no leaked goroutines blocking the wait).
func TestShutdownCancelsPendingTransitions(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := sagemaker.NewInMemoryBackend(lifecycleAccountID, lifecycleRegion)
		execArn := seedPipelineExecution(t, b)

		// Schedule a Stopping -> Stopped transition (100ms delay), then immediately
		// shut down. Shutdown cancels the lifecycle context before the timer fires.
		_, err := b.StopPipelineExecution(context.Background(), execArn)
		require.NoError(t, err)

		shutdownStart := time.Now()
		b.Shutdown(t.Context())
		require.Less(t, time.Since(shutdownStart), time.Second, "Shutdown must return promptly")

		// The cancelled goroutine must not advance the status past Stopping.
		// Wait past the transition delay to confirm it never fires.
		time.Sleep(150 * time.Millisecond)
		assert.Equal(t, "Stopping", statusOf(t, b, execArn),
			"cancelled transition must not mutate state after Shutdown")
	})
}

// TestShutdownRespectsContextDeadline verifies Shutdown returns when its ctx
// expires even though it always completes quickly here with no work pending.
func TestShutdownRespectsContextDeadline(t *testing.T) {
	t.Parallel()

	b := sagemaker.NewInMemoryBackend(lifecycleAccountID, lifecycleRegion)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	b.Shutdown(ctx)
	assert.Less(t, time.Since(start), time.Second)
}
