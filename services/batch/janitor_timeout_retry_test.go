package batch_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/batch"
)

// TestBatchJanitor_TimeoutRetriesThenFails proves JobTimeout.AttemptDurationSeconds
// and RetryStrategy.Attempts actually drive job state instead of only being
// echoed back on Describe -- gopherstack-3nik. A RUNNING job whose attempt runs
// longer than its timeout is retried (back to RUNNABLE) while attempts remain,
// then fails for good once RetryStrategy.Attempts is exhausted.
func TestBatchJanitor_TimeoutRetriesThenFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := batch.NewInMemoryBackend("000000000000", "us-east-1")

	queue, err := b.CreateJobQueue(ctx, "test-queue", 1, "ENABLED", nil, nil, "", nil, "", nil)
	require.NoError(t, err)

	_, err = b.RegisterJobDefinition(
		ctx, "test-jd", "container", nil, nil, 0, 0, nil, nil, nil, nil, nil, nil, false, nil,
	)
	require.NoError(t, err)

	job, err := b.SubmitJob(
		ctx, "job", queue.JobQueueName, "test-jd:1", nil, nil, nil,
		&batch.RetryStrategy{Attempts: 2},
		&batch.JobTimeout{AttemptDurationSeconds: 1},
		nil, nil, nil, "", 0, false,
	)
	require.NoError(t, err)

	status := func() *batch.Job {
		jobs := b.DescribeJobs(ctx, []string{job.JobID})
		require.Len(t, jobs, 1)

		return jobs[0]
	}

	j := batch.NewJanitor(b, time.Minute, 24*time.Hour, 24*time.Hour)

	j.SweepOnce(ctx) // SUBMITTED -> RUNNING
	require.Equal(t, "RUNNING", status().Status)

	// Simulate the first attempt running longer than its 1-second timeout.
	b.SetJobStartedAtForTest(job.JobID, time.Now().Add(-2*time.Second))
	j.SweepOnce(ctx) // attempt 1 times out; one retry attempt remains -> RUNNABLE
	first := status()
	assert.Equal(t, "RUNNABLE", first.Status)
	assert.Contains(t, first.StatusReason, "retrying")

	j.SweepOnce(ctx) // RUNNABLE -> RUNNING (second attempt)
	require.Equal(t, "RUNNING", status().Status)

	// Simulate the second attempt also running longer than its timeout.
	b.SetJobStartedAtForTest(job.JobID, time.Now().Add(-2*time.Second))
	j.SweepOnce(ctx) // attempt 2 times out; attempts exhausted -> FAILED
	final := status()
	assert.Equal(t, "FAILED", final.Status)
	assert.Equal(t, "job attempt duration exceeded timeout", final.StatusReason)
	assert.NotNil(t, final.StoppedAt)
}
