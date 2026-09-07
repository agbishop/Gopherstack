package mediaconvert_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/mediaconvert"
)

// TestInMemoryBackend_RestoreInvalidData verifies that malformed JSON is
// reported as an error rather than silently discarded or partially applied.
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "not_valid_json", data: "not-valid-json"},
		{name: "not_json", data: "not-json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
			err := b.Restore(t.Context(), []byte(tt.data))
			require.Error(t, err)
		})
	}
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateQueue("seed-queue", "", "", "", nil)
	require.NoError(t, err)

	// A syntactically valid but version-mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, mediaconvert.QueueCount(b))
}

// TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero verifies that a
// snapshot with no version field at all (the pre-Phase-3.3 shape: plain
// resource maps keyed directly by "queues", "jobs", etc., no "tables"/
// "version" wrapper) decodes with Version == 0, which mismatches
// mediaconvertSnapshotVersion and is discarded the same way any other
// incompatible version is -- not partially applied.
func TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateQueue("seed-queue", "", "", "", nil)
	require.NoError(t, err)

	oldShape := `{"queues":{"q1":{"name":"q1","arn":"arn:old","pricingPlan":"ON_DEMAND","status":"ACTIVE"}},` +
		`"accountID":"000000000000","region":"us-east-1"}`
	err = b.Restore(t.Context(), []byte(oldShape))
	require.NoError(t, err)

	assert.Equal(t, 0, mediaconvert.QueueCount(b))
}

// TestInMemoryBackend_RestoreV1JobLastShareDetailsDiscarded proves
// gopherstack-hjdd's fix: a v1 snapshot holding Job.LastShareDetails in the
// pre-d83f4b5d3 object shape must be discarded cleanly now that
// mediaconvertSnapshotVersion is 2, rather than erroring Restore outright
// when the registered "jobs" table's custom UnmarshalJSON can't decode a
// JSON object into the new bare-string field.
func TestInMemoryBackend_RestoreV1JobLastShareDetailsDiscarded(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	v1Snapshot := `{
		"version": 1,
		"accountID": "` + testAccountID + `",
		"region": "` + testRegion + `",
		"tables": {
			"jobs": [{
				"id": "job-1",
				"arn": "arn:aws:mediaconvert:us-east-1:000000000000:jobs/job-1",
				"role": "arn:aws:iam::000000000000:role/Role",
				"status": "COMPLETE",
				"lastShareDetails": {"shareToken": "tok-1", "sharedAt": 1700000000}
			}]
		}
	}`

	require.NoError(t, b.Restore(t.Context(), []byte(v1Snapshot)),
		"a v1 snapshot must be discarded via the version guard, not error out of RestoreAll")

	_, err := b.GetJob("job-1")
	require.ErrorIs(t, err, mediaconvert.ErrNotFound,
		"incompatible-version snapshot must reset to empty, not partially decode")
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every store.Table-backed resource family the Phase 3.3
// conversion touched (queues incl. the byARN secondary index, jobTemplates,
// jobs, presets, plus the "dirty" tokenIndex table), every raw map left
// un-converted (tags, certificates), and the scalar policy field.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := mediaconvert.NewInMemoryBackend("111122223333", "us-west-2")

	rp := &mediaconvert.ReservationPlan{Status: "ACTIVE", Commitment: "ONE_YEAR", ReservedSlots: 3}
	concurrentJobs := 5
	queue, err := original.CreateQueueFull(
		"queue-1", "primary queue", "RESERVED", "ACTIVE",
		map[string]string{"team": "media"}, &concurrentJobs, rp,
	)
	require.NoError(t, err)

	// A second queue exists purely to prove the byARN secondary index
	// survives Restore: jobs below are attached to it by ARN, not name.
	queue2, err := original.CreateQueue("queue-2", "secondary queue", "", "", nil)
	require.NoError(t, err)

	jobTemplate, err := original.CreateJobTemplateFull(
		"template-1", "desc", "category-a", "queue-1", 10, map[string]any{"k": "v"}, map[string]string{"env": "prod"},
		"PREFERRED", "SECONDS_30",
		[]mediaconvert.HopDestination{{Queue: "queue-2", WaitMinutes: 20}},
	)
	require.NoError(t, err)

	preset, err := original.CreatePreset("preset-1", "desc", "category-b", map[string]any{"p": 1}, nil)
	require.NoError(t, err)

	job, err := original.CreateJobFull(
		"arn:aws:iam::111122223333:role/mc-role", "queue-1", "template-1",
		map[string]any{"s": "v"}, map[string]string{"owner": "team"}, map[string]string{"m": "v"},
		"JOB", "req-token-1", "ENABLED", "2017-08-29", 7,
		[]mediaconvert.HopDestination{{Queue: "queue-2", WaitMinutes: 5}},
		mediaconvert.JobCreateExtras{StatusUpdateInterval: "SECONDS_10", SimulateReservedQueue: "ENABLED"},
	)
	require.NoError(t, err)

	require.NoError(t, original.AssociateCertificate("arn:aws:acm:us-west-2:111122223333:cert/abc"))
	original.PutPolicy("ALLOWED", "ALLOWED", "DISALLOWED")
	original.TagResource(queue.Arn, map[string]string{"extra": "tag"})

	require.Equal(t, 1, mediaconvert.TokenIndexSize(original))

	snap := original.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	fresh := mediaconvert.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// Scalars.
	assert.Equal(t, "111122223333", fresh.AccountID())
	assert.Equal(t, "us-west-2", fresh.Region())

	// Direct tables.
	gotQueue, err := fresh.GetQueue(queue.Name)
	require.NoError(t, err)
	assert.Equal(t, "primary queue", gotQueue.Description)
	assert.Equal(t, "RESERVED", gotQueue.PricingPlan)
	assert.Equal(t, "media", gotQueue.Tags["team"])
	require.NotNil(t, gotQueue.ReservationPlan)
	assert.Equal(t, 3, gotQueue.ReservationPlan.ReservedSlots)

	gotJobTemplate, err := fresh.GetJobTemplate(jobTemplate.Name)
	require.NoError(t, err)
	assert.Equal(t, "category-a", gotJobTemplate.Category)
	assert.Equal(t, 10, gotJobTemplate.Priority)
	require.NotNil(t, gotJobTemplate.AccelerationSettings)
	assert.Equal(t, "PREFERRED", gotJobTemplate.AccelerationSettings.Mode)
	assert.Equal(t, "SECONDS_30", gotJobTemplate.StatusUpdateInterval)
	require.Len(t, gotJobTemplate.HopDestinations, 1)
	assert.Equal(t, "queue-2", gotJobTemplate.HopDestinations[0].Queue)

	gotPreset, err := fresh.GetPreset(preset.Name)
	require.NoError(t, err)
	assert.Equal(t, "category-b", gotPreset.Category)

	gotJob, err := fresh.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, "req-token-1", gotJob.ClientRequestToken)
	require.Len(t, gotJob.HopDestinations, 1)
	assert.Equal(t, "queue-2", gotJob.HopDestinations[0].Queue)
	assert.Equal(t, "SECONDS_10", gotJob.StatusUpdateInterval)
	assert.Equal(t, "ENABLED", gotJob.SimulateReservedQueue)

	// byARN secondary index on the queues table: resolving a queue by ARN
	// (rather than name) only works if AddIndex's registered index was
	// correctly repopulated by Table.Restore, not left empty.
	jobByArn, err := fresh.CreateJob(
		"arn:aws:iam::111122223333:role/mc-role", queue2.Arn, "", nil, nil, nil, "",
	)
	require.NoError(t, err)
	assert.Equal(t, queue2.Arn, jobByArn.QueueArn)

	// Deleting the queue afterward must also clear the restored index entry
	// -- proves Table.Delete (not just Table.Restore) still maintains the
	// index correctly on a backend that came from Restore.
	require.NoError(t, fresh.DeleteQueue(queue2.Name))
	_, err = fresh.CreateJob("arn:aws:iam::111122223333:role/mc-role", queue2.Arn, "", nil, nil, nil, "")
	require.ErrorIs(t, err, mediaconvert.ErrNotFound)

	// Plain (un-converted) maps/scalars that were persisted before Phase 3.3.
	assert.Equal(t, 1, mediaconvert.CertificateCount(fresh))

	pol, err := fresh.GetPolicy()
	require.NoError(t, err)
	assert.Equal(t, "ALLOWED", pol.HTTPInputs)
	assert.Equal(t, "DISALLOWED", pol.S3Inputs)

	tags := fresh.GetTags(queue.Arn)
	assert.Equal(t, "tag", tags["extra"])

	// "Dirty" tokenIndex table: the identity (token) key set survives the
	// round trip via the DTO registry, matching pre-Phase-3.3 behavior where
	// the map's key set (but not its unexported-field values) survived a
	// Snapshot/Restore round trip.
	assert.Equal(t, 1, mediaconvert.TokenIndexSize(fresh))
}

// TestHandler_SnapshotRestoreDelegate verifies the Handler-level Snapshot and
// Restore -- which cli.go's setupPersistence actually calls, since it
// type-asserts the service.Registerable value returned by Provider.Init
// (the Handler, not InMemoryBackend) -- correctly delegate to the backend.
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	h := mediaconvert.NewHandler(mediaconvert.NewInMemoryBackend(testAccountID, testRegion))

	_, err := h.Backend.CreateQueue("delegate-queue", "", "", "", nil)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	h2 := mediaconvert.NewHandler(mediaconvert.NewInMemoryBackend(testAccountID, testRegion))
	require.NoError(t, h2.Restore(t.Context(), snap))

	assert.Equal(t, 1, mediaconvert.QueueCount(h2.Backend.(*mediaconvert.InMemoryBackend)))
}

// TestPersistenceRoundTrip verifies Snapshot/Restore round-trip across every
// resource family via the top-level backend constructors.
func TestPersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	b1 := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b1.CreateQueue("q1", "desc", "", "", map[string]string{"env": "test"})
	require.NoError(t, err)
	_, err = b1.CreatePreset("p1", "preset1", "", nil, nil)
	require.NoError(t, err)
	_, err = b1.CreateJob("arn:aws:iam::123:role/r", "", "", nil, nil, nil, "")
	require.NoError(t, err)
	_, err = b1.CreateJobTemplate("jt1", "tmpl", "", "", 0, nil, nil)
	require.NoError(t, err)
	require.NoError(t, b1.AssociateCertificate("arn:aws:acm:us-east-1:123:cert/abc"))
	b1.PutPolicy("ALLOWED", "ALLOWED", "DISALLOWED")

	snap := b1.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := mediaconvert.NewInMemoryBackend("", "")
	require.NoError(t, b2.Restore(t.Context(), snap))

	assert.Equal(t, testAccountID, b2.AccountID())
	assert.Equal(t, testRegion, b2.Region())
	assert.Equal(t, 1, mediaconvert.QueueCount(b2))
	assert.Equal(t, 1, mediaconvert.PresetCount(b2))
	assert.Equal(t, 1, mediaconvert.JobCount(b2))
	assert.Equal(t, 1, mediaconvert.JobTemplateCount(b2))
	assert.Equal(t, 1, mediaconvert.CertificateCount(b2))

	p, err := b2.GetPolicy()
	require.NoError(t, err)
	assert.Equal(t, "ALLOWED", p.HTTPInputs)
	assert.Equal(t, "DISALLOWED", p.S3Inputs)
}

// TestRestoreEmptySnapshot verifies an empty snapshot is handled gracefully.
func TestRestoreEmptySnapshot(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	err := b.Restore(t.Context(), []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, 0, mediaconvert.QueueCount(b))
}

// TestSnapshotThenDeleteThenRestore_DataPreserved verifies that mutating the
// backend after taking a snapshot does not corrupt the previously-taken
// snapshot: restoring it elsewhere still reflects the pre-mutation state.
func TestSnapshotThenDeleteThenRestore_DataPreserved(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateQueue("snap-q", "", "", "", map[string]string{"k": "v"})
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	err = b.DeleteQueue("snap-q")
	require.NoError(t, err)

	b2 := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	err = b2.Restore(t.Context(), snap)
	require.NoError(t, err)

	q, err := b2.GetQueue("snap-q")
	require.NoError(t, err)
	assert.Equal(t, "snap-q", q.Name)
}

// TestPersistence_NewFieldsRoundTrip verifies newer Job/Queue fields survive
// a snapshot/restore round trip.
func TestPersistence_NewFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	b1 := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	// Create queue with new fields.
	rp := &mediaconvert.ReservationPlan{ReservedSlots: 2, Status: "ACTIVE"}
	maxFeeds := 6
	concurrentJobs := 4
	q, err := b1.CreateQueueFull(
		"snap-q2", "", "", "", nil, &concurrentJobs, rp,
		mediaconvert.QueueCreateExtras{MaximumConcurrentFeeds: &maxFeeds},
	)
	require.NoError(t, err)

	// Create job with new fields.
	j, err := b1.CreateJobFull("arn:aws:iam::123:role/r", "snap-q2", "", nil, nil, nil,
		"", "snap-token", "ENABLED", "2017-08-29", 5,
		[]mediaconvert.HopDestination{{Queue: "fallback", WaitMinutes: 3}})
	require.NoError(t, err)

	snap := b1.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, b2.Restore(t.Context(), snap))

	// Verify queue.
	got, err := b2.GetQueue(q.Name)
	require.NoError(t, err)
	require.NotNil(t, got.ConcurrentJobs)
	assert.Equal(t, 4, *got.ConcurrentJobs)
	require.NotNil(t, got.ReservationPlan)
	assert.Equal(t, 2, got.ReservationPlan.ReservedSlots)
	require.NotNil(t, got.MaximumConcurrentFeeds)
	assert.Equal(t, 6, *got.MaximumConcurrentFeeds)

	// Verify job.
	gotJ, err := b2.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, "2017-08-29", gotJ.JobEngineVersionUsed)
	assert.Equal(t, "snap-token", gotJ.ClientRequestToken)
	assert.Equal(t, "PREFERRED", gotJ.AccelerationStatus)
	assert.Equal(t, 5, gotJ.Priority)
	require.Len(t, gotJ.HopDestinations, 1)
	assert.Equal(t, "fallback", gotJ.HopDestinations[0].Queue)
	assert.NotNil(t, gotJ.Messages)
	assert.Equal(t, "NOT_SHARED", gotJ.ShareStatus)
}
