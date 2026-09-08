package batch_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/batch"
)

// TestBatchJanitor_TaskTimeout_WithJanitor verifies that the variadic taskTimeout
// parameter is stored in the janitor's TaskTimeout field when passed to WithJanitor.
func TestBatchJanitor_TaskTimeout_WithJanitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		inactiveJobDefTTL time.Duration
		completedJobTTL   time.Duration
		taskTimeout       time.Duration
		wantJobDefTTL     time.Duration
		wantCompletedTTL  time.Duration
		wantTimeout       time.Duration
	}{
		{
			name:              "zero_timeout",
			inactiveJobDefTTL: time.Hour,
			completedJobTTL:   12 * time.Hour,
			taskTimeout:       0,
			wantJobDefTTL:     time.Hour,
			wantCompletedTTL:  12 * time.Hour,
			wantTimeout:       0,
		},
		{
			name:              "30s_timeout",
			inactiveJobDefTTL: 24 * time.Hour,
			completedJobTTL:   24 * time.Hour,
			taskTimeout:       30 * time.Second,
			wantJobDefTTL:     24 * time.Hour,
			wantCompletedTTL:  24 * time.Hour,
			wantTimeout:       30 * time.Second,
		},
		{
			name:              "zero_ttls_use_defaults",
			inactiveJobDefTTL: 0,
			completedJobTTL:   0,
			taskTimeout:       0,
			wantJobDefTTL:     24 * time.Hour,
			wantCompletedTTL:  24 * time.Hour,
			wantTimeout:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := batch.NewHandler(batch.NewInMemoryBackend("000000000000", "us-east-1"))
			h.WithJanitor(time.Minute, tt.inactiveJobDefTTL, tt.completedJobTTL, tt.taskTimeout)

			assert.Equal(t, tt.wantJobDefTTL, h.GetJanitorInactiveJobDefTTL())
			assert.Equal(t, tt.wantCompletedTTL, h.GetJanitorCompletedJobTTL())
			assert.Equal(t, tt.wantTimeout, h.GetJanitorTaskTimeout())
		})
	}
}

// TestBatchJanitor_Run_ExitsOnCancel verifies that the janitor loop exits
// promptly when the parent context is cancelled.
func TestBatchJanitor_Run_ExitsOnCancel(t *testing.T) {
	t.Parallel()

	b := batch.NewInMemoryBackend("000000000000", "us-east-1")
	j := batch.NewJanitor(b, 10*time.Millisecond, time.Hour, time.Hour)
	j.TaskTimeout = 30 * time.Second

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})

	go func() {
		j.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "janitor did not exit after context cancellation")
	}
}

// TestBatchJanitor_SweepOnce_WithTaskTimeout verifies that SweepOnce works correctly
// even when a TaskTimeout is configured on the janitor.
func TestBatchJanitor_SweepOnce_WithTaskTimeout(t *testing.T) {
	t.Parallel()

	b := batch.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.RegisterJobDefinition(
		context.Background(),
		"sweep-timeout-test",
		"container",
		nil,
		nil,
		0,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		false,
		nil,
	)
	require.NoError(t, err)

	require.NoError(t, b.DeregisterJobDefinition(context.Background(), "sweep-timeout-test:1"))

	// Set DeregisteredAt in the past so it will be swept.
	b.SetJobDefinitionDeregisteredAt("sweep-timeout-test:1", time.Now().Add(-25*time.Hour))

	j := batch.NewJanitor(b, time.Minute, 24*time.Hour, 24*time.Hour)
	j.TaskTimeout = 30 * time.Second

	require.Equal(t, 1, b.JobDefinitionCount())

	j.SweepOnce(t.Context())

	assert.Equal(t, 0, b.JobDefinitionCount())
}

// TestBatchJanitor_SweepCompletedJobs verifies that the janitor sweeps
// completed and failed jobs past their CompletedJobTTL.
func TestBatchJanitor_SweepCompletedJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      string
		stoppedAge  time.Duration // how old StoppedAt is (positive = past)
		ttl         time.Duration
		wantEvicted bool
	}{
		{
			name:        "evict_succeeded_past_ttl",
			status:      "SUCCEEDED",
			stoppedAge:  25 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: true,
		},
		{
			name:        "evict_failed_past_ttl",
			status:      "FAILED",
			stoppedAge:  25 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: true,
		},
		{
			name:        "keep_succeeded_within_ttl",
			status:      "SUCCEEDED",
			stoppedAge:  1 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: false,
		},
		{
			name:        "keep_running_job",
			status:      "RUNNING",
			stoppedAge:  25 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := batch.NewInMemoryBackend("000000000000", "us-east-1")

			queue, err := b.CreateJobQueue(context.Background(), "test-queue", 1, "ENABLED", nil, nil, "", nil, "", nil)
			require.NoError(t, err)

			_, err = b.RegisterJobDefinition(
				context.Background(),
				"test-jd",
				"container",
				nil,
				nil,
				0,
				0,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				false,
				nil,
			)
			require.NoError(t, err)

			job, err := b.SubmitJob(
				context.Background(),
				"test-job",
				queue.JobQueueName,
				"test-jd:1",
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				"",
				0,
				false,
			)
			require.NoError(t, err)

			// Set the job status and a back-dated StoppedAt.
			b.SetJobStoppedAtForTest(job.JobID, tt.status, time.Now().Add(-tt.stoppedAge))

			j := batch.NewJanitor(b, time.Minute, 24*time.Hour, tt.ttl)
			j.SweepOnce(t.Context())

			jobs, _, err := b.ListJobs(context.Background(), queue.JobQueueName, tt.status, "", 0, nil)
			require.NoError(t, err)

			if tt.wantEvicted {
				assert.Empty(t, jobs, "job should have been evicted")
			} else {
				assert.NotEmpty(t, jobs, "job should not have been evicted")
			}
		})
	}
}

// TestBatchJanitor_DefaultInterval verifies that a zero interval in WithJanitor
// results in the default interval being used.
func TestBatchJanitor_DefaultInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{
			name:     "zero_uses_default",
			interval: 0,
			want:     batch.DefaultJanitorInterval,
		},
		{
			name:     "custom_interval_propagated",
			interval: 5 * time.Minute,
			want:     5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := batch.NewHandler(batch.NewInMemoryBackend("123456789012", "us-east-1"))
			h.WithJanitor(tt.interval, 0, 0)

			assert.Equal(t, tt.want, h.GetJanitorInterval())
		})
	}
}

// TestBatchJanitor_DependsOn covers SubmitJobInput.DependsOn
// (api_op_SubmitJob.go), which was previously accepted, stored, and echoed
// by DescribeJobs but never evaluated: a dependent job advanced straight to
// RUNNING regardless of whether its dependency had completed.
func TestBatchJanitor_DependsOn(t *testing.T) {
	t.Parallel()

	newBackendWithQueue := func(t *testing.T) (*batch.InMemoryBackend, *batch.JobQueue) {
		t.Helper()

		b := batch.NewInMemoryBackend("000000000000", "us-east-1")

		queue, err := b.CreateJobQueue(context.Background(), "test-queue", 1, "ENABLED", nil, nil, "", nil, "", nil)
		require.NoError(t, err)

		_, err = b.RegisterJobDefinition(
			context.Background(), "test-jd", "container",
			nil, nil, 0, 0, nil, nil, nil, nil, nil, nil, false, nil,
		)
		require.NoError(t, err)

		return b, queue
	}

	submit := func(
		t *testing.T, b *batch.InMemoryBackend, q *batch.JobQueue, name string, dependsOn []batch.JobDependency,
	) *batch.Job {
		t.Helper()

		job, err := b.SubmitJob(
			context.Background(), name, q.JobQueueName, "test-jd:1",
			nil, nil, dependsOn, nil, nil, nil, nil, nil, "", 0, false,
		)
		require.NoError(t, err)

		return job
	}

	jobStatus := func(t *testing.T, b *batch.InMemoryBackend, jobID string) string {
		t.Helper()

		jobs := b.DescribeJobs(context.Background(), []string{jobID})
		require.Len(t, jobs, 1)

		return jobs[0].Status
	}

	t.Run("blocks_until_dependency_succeeds", func(t *testing.T) {
		t.Parallel()

		b, q := newBackendWithQueue(t)
		upstream := submit(t, b, q, "upstream", nil)
		downstream := submit(t, b, q, "downstream", []batch.JobDependency{{JobID: upstream.JobID}})

		j := batch.NewJanitor(b, time.Minute, 24*time.Hour, 24*time.Hour)

		j.SweepOnce(t.Context()) // upstream: SUBMITTED -> RUNNING
		assert.Equal(t, "RUNNING", jobStatus(t, b, upstream.JobID))
		assert.Equal(t, "SUBMITTED", jobStatus(t, b, downstream.JobID),
			"must not advance while its dependency is unresolved")

		j.SweepOnce(t.Context()) // upstream: RUNNING -> SUCCEEDED
		assert.Equal(t, "SUCCEEDED", jobStatus(t, b, upstream.JobID))
		assert.Equal(t, "SUBMITTED", jobStatus(t, b, downstream.JobID),
			"must not advance in the same sweep its dependency completes")

		j.SweepOnce(t.Context()) // downstream: SUBMITTED -> RUNNING, now that upstream is SUCCEEDED
		assert.Equal(t, "RUNNING", jobStatus(t, b, downstream.JobID))
	})

	t.Run("propagates_dependency_failure", func(t *testing.T) {
		t.Parallel()

		b, q := newBackendWithQueue(t)
		upstream := submit(t, b, q, "upstream", nil)
		downstream := submit(t, b, q, "downstream", []batch.JobDependency{{JobID: upstream.JobID}})

		b.ForceJobStatus(upstream.JobID, "FAILED")

		j := batch.NewJanitor(b, time.Minute, 24*time.Hour, 24*time.Hour)
		j.SweepOnce(t.Context())

		assert.Equal(t, "FAILED", jobStatus(t, b, downstream.JobID))
	})
}
