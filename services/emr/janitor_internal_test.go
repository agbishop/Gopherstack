package emr // needs unexported Cluster.steps/clusterGet; *_internal_test.go per house convention.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
)

// TestJanitor_AutoTerminateSweep covers the janitor's ALL_STEPS_COMPLETED
// auto-termination path (KeepJobFlowAliveWhenNoSteps=false, the default):
// a cluster whose steps have all reached a terminal status must be
// terminated by the sweep, but only when AutoTerminate is set and only once
// there is at least one completed step.
func TestJanitor_AutoTerminateSweep(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		keepAlive      bool
		backdateStep   bool
		wantTerminated bool
	}{
		{
			name:           "auto_terminates_after_step_completes",
			keepAlive:      false,
			backdateStep:   true,
			wantTerminated: true,
		},
		{
			name:           "kept_alive_cluster_not_terminated",
			keepAlive:      true,
			backdateStep:   true,
			wantTerminated: false,
		},
		{
			name:           "still_pending_step_not_terminated",
			keepAlive:      false,
			backdateStep:   false,
			wantTerminated: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewInMemoryBackend("123456789012", "us-east-1")

			cluster, err := b.RunJobFlow(context.Background(), RunJobFlowParams{
				Name:         "auto-terminate-test",
				ReleaseLabel: testReleaseLabel6,
				Steps: []StepSpec{{
					Name:          "step-1",
					HadoopJarStep: StepHadoopJarStepInput{Jar: "s3://bucket/jar"},
				}},
				Instances: RunJobFlowInstances{KeepJobFlowAliveWhenNoSteps: tt.keepAlive},
			})
			require.NoError(t, err)

			if tt.backdateStep {
				stored, ok := b.clusterGet("us-east-1", cluster.ID)
				require.True(t, ok)
				stored.steps[0].Status.Timeline.CreationDateTime = awstime.Epoch(
					time.Now().Add(-2 * stepCompletionDelay),
				)
			}

			NewJanitor(b, time.Minute, time.Hour).SweepOnce(context.Background())

			got, err := b.DescribeCluster(context.Background(), cluster.ID)
			require.NoError(t, err)

			if tt.wantTerminated {
				assert.Equal(t, StateTerminated, got.Status.State)
				assert.Equal(t, "ALL_STEPS_COMPLETED", got.Status.StateChangeReason["Code"])
			} else {
				assert.Equal(t, StateWaiting, got.Status.State)
			}
		})
	}
}

// TestJanitor_AutoTerminateSweep_NoStepsNeverTerminates covers the
// zero-step case: real KeepJobFlowAliveWhenNoSteps=false only fires "after
// completing all steps" (JobFlowInstancesConfig, emr@v1.64.4
// types/types.go:1861-1866) -- a cluster that was never given any steps has
// nothing to have completed, so the sweep must leave it alone.
func TestJanitor_AutoTerminateSweep_NoStepsNeverTerminates(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")

	cluster, err := b.RunJobFlow(context.Background(), RunJobFlowParams{
		Name:         "no-steps-test",
		ReleaseLabel: testReleaseLabel6,
	})
	require.NoError(t, err)

	NewJanitor(b, time.Minute, time.Hour).SweepOnce(context.Background())

	got, err := b.DescribeCluster(context.Background(), cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, StateWaiting, got.Status.State)
}

// TestJanitor_AutoTerminateSweep_SkipsAlreadyTerminated guards against the
// sweep re-firing on an already-TERMINATED cluster every tick (it would
// otherwise call terminateSingle repeatedly and misreport idempotent
// no-ops as freshly auto-terminated clusters).
func TestJanitor_AutoTerminateSweep_SkipsAlreadyTerminated(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")

	cluster, err := b.RunJobFlow(context.Background(), RunJobFlowParams{
		Name:         "already-terminated-test",
		ReleaseLabel: testReleaseLabel6,
		Steps: []StepSpec{{
			Name:          "step-1",
			HadoopJarStep: StepHadoopJarStepInput{Jar: "s3://bucket/jar"},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, b.TerminateJobFlows(context.Background(), []string{cluster.ID}))

	j := NewJanitor(b, time.Minute, time.Hour)
	j.SweepOnce(context.Background())

	got, err := b.DescribeCluster(context.Background(), cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, StateTerminated, got.Status.State)
	assert.Equal(t, "USER_REQUEST", got.Status.StateChangeReason["Code"],
		"already-terminated cluster's reason must not be overwritten by the auto-terminate sweep")
}
