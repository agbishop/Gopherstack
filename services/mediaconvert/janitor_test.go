package mediaconvert_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/mediaconvert"
)

// TestAdvanceJobPhase_ReachesComplete verifies the COMPLETE state is reachable.
func TestAdvanceJobPhase_ReachesComplete(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)

	// Advance through the full phase pipeline to COMPLETE.
	// SUBMITTED → PROGRESSING (phase=PROBING)
	b.AdvanceJobPhase()
	assert.Equal(t, "PROGRESSING", mediaconvert.JobStatusOf(b, j.ID))

	// PROBING → TRANSCODING
	b.AdvanceJobPhase()
	assert.Equal(t, "TRANSCODING", mediaconvert.JobPhaseOf(b, j.ID))

	// TRANSCODING → UPLOADING
	b.AdvanceJobPhase()
	assert.Equal(t, "UPLOADING", mediaconvert.JobPhaseOf(b, j.ID))

	// UPLOADING → COMPLETE
	b.AdvanceJobPhase()
	assert.Equal(t, "COMPLETE", mediaconvert.JobStatusOf(b, j.ID))
}

// TestAdvanceJobPhase_FullProgression tests the full phase advancement.
func TestAdvanceJobPhase_FullProgression(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)

	require.Equal(t, "SUBMITTED", mediaconvert.JobStatusOf(b, j.ID))

	// Tick 1: SUBMITTED → PROGRESSING/PROBING
	b.AdvanceJobPhase()
	assert.Equal(t, "PROGRESSING", mediaconvert.JobStatusOf(b, j.ID))
	assert.Equal(t, "PROBING", mediaconvert.JobPhaseOf(b, j.ID))

	// Tick 2: PROBING → TRANSCODING
	b.AdvanceJobPhase()
	assert.Equal(t, "PROGRESSING", mediaconvert.JobStatusOf(b, j.ID))
	assert.Equal(t, "TRANSCODING", mediaconvert.JobPhaseOf(b, j.ID))

	// Tick 3: TRANSCODING → UPLOADING
	b.AdvanceJobPhase()
	assert.Equal(t, "PROGRESSING", mediaconvert.JobStatusOf(b, j.ID))
	assert.Equal(t, "UPLOADING", mediaconvert.JobPhaseOf(b, j.ID))

	// Tick 4: UPLOADING → COMPLETE
	b.AdvanceJobPhase()
	assert.Equal(t, "COMPLETE", mediaconvert.JobStatusOf(b, j.ID))
	assert.Empty(t, mediaconvert.JobPhaseOf(b, j.ID))
}

// TestAdvanceJobPhase_AdvancesAllEligibleJobs ensures all jobs in the backend advance.
func TestAdvanceJobPhase_AdvancesAllEligibleJobs(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j1 := createTestJobDirect(t, b)
	j2 := createTestJobDirect(t, b)

	b.AdvanceJobPhase()
	assert.Equal(t, "PROGRESSING", mediaconvert.JobStatusOf(b, j1.ID))
	assert.Equal(t, "PROGRESSING", mediaconvert.JobStatusOf(b, j2.ID))
}

// TestAdvanceJobPhase_OutputGroupDetailsOnComplete verifies output details set when job completes.
func TestAdvanceJobPhase_OutputGroupDetailsOnComplete(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)

	// Advance all the way to COMPLETE.
	for range 4 {
		b.AdvanceJobPhase()
	}

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	require.Equal(t, "COMPLETE", got.Status)
	require.Len(t, got.OutputGroupDetails, 1)
	require.Len(t, got.OutputGroupDetails[0].OutputDetails, 1)
	od := got.OutputGroupDetails[0].OutputDetails[0]
	assert.Equal(t, 60000, od.DurationInMs)
	require.NotNil(t, od.VideoDetails)
	assert.Equal(t, 1920, od.VideoDetails.WidthInPx)
	assert.Equal(t, 1080, od.VideoDetails.HeightInPx)
}

// TestAdvanceJobPhase_SetsStartTime verifies StartTime set when PROGRESSING.
func TestAdvanceJobPhase_SetsStartTime(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)
	require.Zero(t, j.Timing.StartTime)

	b.AdvanceJobPhase() // SUBMITTED → PROGRESSING

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Greater(t, got.Timing.StartTime, float64(0))
}

// TestAdvanceJobPhase_SetsFinishTimeOnComplete verifies FinishTime set on COMPLETE.
func TestAdvanceJobPhase_SetsFinishTimeOnComplete(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)

	for range 4 {
		b.AdvanceJobPhase()
	}

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Greater(t, got.Timing.FinishTime, float64(0))
}

// TestAdvanceJobPhase_PreservesSubmitTime verifies SubmitTime is not overwritten during phases.
func TestAdvanceJobPhase_PreservesSubmitTime(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)
	submitTime := j.Timing.SubmitTime
	require.Greater(t, submitTime, float64(0))

	// Advance all the way to COMPLETE.
	for range 4 {
		b.AdvanceJobPhase()
	}

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.InDelta(t, submitTime, got.Timing.SubmitTime, 0.001)
}

// TestAdvanceJobPhase_FinishTimeAfterStartTime verifies time ordering.
func TestAdvanceJobPhase_FinishTimeAfterStartTime(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)

	for range 4 {
		b.AdvanceJobPhase()
		time.Sleep(1 * time.Millisecond)
	}

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, got.Timing.FinishTime, got.Timing.StartTime)
}

// TestAdvanceJobPhase_JobPercentComplete100OnComplete verifies 100% on COMPLETE.
func TestAdvanceJobPhase_JobPercentComplete100OnComplete(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)
	require.Equal(t, 0, j.JobPercentComplete)

	for range 4 {
		b.AdvanceJobPhase()
	}

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, got.JobPercentComplete)
}

// TestAdvanceJobPhase_CompletedJobIsTerminal verifies COMPLETE is terminal.
func TestAdvanceJobPhase_CompletedJobIsTerminal(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)

	for range 4 {
		b.AdvanceJobPhase()
	}

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	require.Equal(t, "COMPLETE", got.Status)
	finishTime := got.Timing.FinishTime

	// Extra advance should not change state.
	b.AdvanceJobPhase()

	got2, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", got2.Status)
	assert.InDelta(t, finishTime, got2.Timing.FinishTime, 0.001)
}

// TestAdvanceJobPhase_CanceledJobIsTerminal verifies CANCELED is terminal.
func TestAdvanceJobPhase_CanceledJobIsTerminal(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j := createTestJobDirect(t, b)
	require.NoError(t, b.CancelJob(j.ID))

	b.AdvanceJobPhase()

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, "CANCELED", got.Status)
}

// TestAdvanceJobPhase_PausedQueueBlocksSubmittedJob verifies a SUBMITTED job
// assigned to a PAUSED queue does not begin processing. Queue.Status doc
// (aws-sdk-go-v2 mediaconvert types.go, Queue.Status field) says: if you
// pause a queue, the service won't begin processing jobs in that queue.
func TestAdvanceJobPhase_PausedQueueBlocksSubmittedJob(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	_, err := b.CreateQueueFull("paused-queue", "", "", "PAUSED", nil, nil, nil)
	require.NoError(t, err)

	j, err := b.CreateJob("arn:aws:iam::123:role/role", "paused-queue", "", nil, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, "SUBMITTED", j.Status)

	advanced := b.AdvanceJobPhase()
	assert.False(t, advanced)

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, "SUBMITTED", got.Status, "job on a PAUSED queue must not begin processing")

	// Reactivating the queue lets the job proceed on the next tick.
	_, err = b.UpdateQueue("paused-queue", "", "ACTIVE", nil, nil, nil)
	require.NoError(t, err)

	advanced = b.AdvanceJobPhase()
	assert.True(t, advanced)

	got, err = b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, "PROGRESSING", got.Status)
}

// TestAdvanceJobPhase_ConcurrentJobsLimitBlocksExcessJobs verifies a queue's
// ConcurrentJobs limit caps how many jobs run PROGRESSING at once (gopherstack-7bxb):
// with a limit of 1 and 2 SUBMITTED jobs, only 1 may advance -- the other stays
// SUBMITTED until a slot frees up.
func TestAdvanceJobPhase_ConcurrentJobsLimitBlocksExcessJobs(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	limit := 1
	q, err := b.CreateQueueFull("cj-limit-queue", "", "", "", nil, &limit, nil)
	require.NoError(t, err)

	j1, err := b.CreateJob("arn:aws:iam::123:role/role", q.Name, "", nil, nil, nil, "")
	require.NoError(t, err)
	j2, err := b.CreateJob("arn:aws:iam::123:role/role", q.Name, "", nil, nil, nil, "")
	require.NoError(t, err)

	advanced := b.AdvanceJobPhase()
	assert.True(t, advanced)

	statuses := map[string]int{}
	for _, id := range []string{j1.ID, j2.ID} {
		gotJob, jobErr := b.GetJob(id)
		require.NoError(t, jobErr)
		statuses[gotJob.Status]++
	}
	assert.Equal(t, 1, statuses["PROGRESSING"], "only ConcurrentJobs=1 job may run at once")
	assert.Equal(t, 1, statuses["SUBMITTED"], "the excess job must wait for a free slot")
	assert.Equal(t, 1, mediaconvert.QueueCounterProgressing(b, q.Arn))
	assert.Equal(t, 1, mediaconvert.QueueCounterSubmitted(b, q.Arn))

	// A further tick must not advance the waiting job while the slot is taken.
	b.AdvanceJobPhase()
	stillWaiting := 0
	for _, id := range []string{j1.ID, j2.ID} {
		gotJob, jobErr := b.GetJob(id)
		require.NoError(t, jobErr)
		if gotJob.Status == "SUBMITTED" {
			stillWaiting++
		}
	}
	assert.Equal(t, 1, stillWaiting, "the excess job must keep waiting until the running job finishes")
}

// TestAdvanceJobPhase_NilConcurrentJobsIsUnlimited verifies that a queue
// created without an explicit ConcurrentJobs value (nil, matching AWS's
// *int32 "not set") does not throttle admission at all.
func TestAdvanceJobPhase_NilConcurrentJobsIsUnlimited(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	q, err := b.CreateQueue("unlimited-queue", "", "", "", nil)
	require.NoError(t, err)
	require.Nil(t, q.ConcurrentJobs)

	j1, err := b.CreateJob("arn:aws:iam::123:role/role", q.Name, "", nil, nil, nil, "")
	require.NoError(t, err)
	j2, err := b.CreateJob("arn:aws:iam::123:role/role", q.Name, "", nil, nil, nil, "")
	require.NoError(t, err)

	b.AdvanceJobPhase()

	for _, id := range []string{j1.ID, j2.ID} {
		gotJob, jobErr := b.GetJob(id)
		require.NoError(t, jobErr)
		assert.Equal(t, "PROGRESSING", gotJob.Status)
	}
}

// TestAdvanceJobPhase_ReturnsFalseWhenNoEligibleJobs verifies the return value.
func TestAdvanceJobPhase_ReturnsFalseWhenNoEligibleJobs(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	advanced := b.AdvanceJobPhase()
	assert.False(t, advanced)
}

// TestAdvanceJobPhase_ReturnsTrueWhenJobsAdvanced verifies the return value.
func TestAdvanceJobPhase_ReturnsTrueWhenJobsAdvanced(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	createTestJobDirect(t, b)

	advanced := b.AdvanceJobPhase()
	assert.True(t, advanced)
}

// TestAdvanceJobPhase_TwoPhaseLockingPattern verifies the RLock+Lock two-phase
// pattern used by AdvanceJobPhase advances a job created via HTTP through
// every phase to COMPLETE.
func TestAdvanceJobPhase_TwoPhaseLockingPattern(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	id := createTestJob(t, mediaconvert.NewHandler(b))

	// Initially SUBMITTED.
	assert.Equal(t, "SUBMITTED", mediaconvert.JobStatusOf(b, id))

	advanced := b.AdvanceJobPhase()
	assert.True(t, advanced)
	assert.Equal(t, "PROGRESSING", mediaconvert.JobStatusOf(b, id))
	assert.Equal(t, "PROBING", mediaconvert.JobPhaseOf(b, id))

	// Advance through phases.
	advanced = b.AdvanceJobPhase()
	assert.True(t, advanced)
	assert.Equal(t, "TRANSCODING", mediaconvert.JobPhaseOf(b, id))

	advanced = b.AdvanceJobPhase()
	assert.True(t, advanced)
	assert.Equal(t, "UPLOADING", mediaconvert.JobPhaseOf(b, id))

	advanced = b.AdvanceJobPhase()
	assert.True(t, advanced)
	assert.Equal(t, "COMPLETE", mediaconvert.JobStatusOf(b, id))

	// No more advancement once COMPLETE.
	advanced = b.AdvanceJobPhase()
	assert.False(t, advanced)
}

// TestFullJobLifecycle_ViaHTTP tests create→advance→complete via HTTP.
func TestFullJobLifecycle_ViaHTTP(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	h := mediaconvert.NewHandler(b)

	// Create job.
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/jobs",
		map[string]any{"role": "arn:aws:iam::123:role/r"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	jobData := out["job"].(map[string]any)
	jobID := jobData["id"].(string)
	assert.Equal(t, "SUBMITTED", jobData["status"])

	// Advance to COMPLETE.
	for range 4 {
		b.AdvanceJobPhase()
	}

	rec = doRequest(t, h, http.MethodGet, "/2017-08-29/jobs/"+jobID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out2 map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out2))
	j2 := out2["job"].(map[string]any)
	assert.Equal(t, "COMPLETE", j2["status"])
	timing := j2["timing"].(map[string]any)
	assert.Greater(t, timing["startTime"], float64(0))
	assert.Greater(t, timing["finishTime"], float64(0))
	outputGroups := j2["outputGroupDetails"].([]any)
	require.Len(t, outputGroups, 1)
}

// TestSweepExpiredTokens_NoOpOnEmptyIndex verifies sweeping an empty token
// index is a safe no-op, and that a freshly-added token survives a sweep
// within the TTL window.
func TestSweepExpiredTokens_NoOpOnEmptyIndex(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	// No tokens — sweep is a no-op.
	b.SweepExpiredTokens()
	assert.Equal(t, 0, mediaconvert.TokenIndexSize(b))

	// Add a token via job creation.
	_, err := b.CreateJobFull(
		"arn:aws:iam::"+testAccountID+":role/Role",
		"", "", map[string]any{}, nil, nil,
		"", "sweep-tok", "", "", 0, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, mediaconvert.TokenIndexSize(b))

	// TTL is 1 minute; fresh token should survive sweep.
	b.SweepExpiredTokens()
	assert.Equal(t, 1, mediaconvert.TokenIndexSize(b))
}

// TestSweepExpiredTokens_DoesNotRemoveFreshTokens verifies a token created
// just now (well within the TTL window) is never removed by a sweep,
// regardless of how the job was created.
func TestSweepExpiredTokens_DoesNotRemoveFreshTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
	}{
		{name: "created_via_create_job_full", token: "old-token"},
		{name: "created_via_create_job_full_alt", token: "fresh-token"},
		{name: "created_via_create_job_full_unique", token: "unique-token-001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

			_, err := b.CreateJobFull("arn:aws:iam::123:role/r", "", "", nil, nil, nil,
				"", tt.token, "", "", 0, nil)
			require.NoError(t, err)
			assert.Equal(t, 1, mediaconvert.TokenIndexSize(b))

			b.SweepExpiredTokens()
			assert.Equal(t, 1, mediaconvert.TokenIndexSize(b), "fresh token should not be swept")
		})
	}
}
