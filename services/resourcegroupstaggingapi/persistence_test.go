package resourcegroupstaggingapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/resourcegroupstaggingapi"
)

// TestInMemoryBackend_RestoreInvalidData verifies that malformed JSON is
// reported as an error rather than silently discarded or partially applied.
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)

	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend is discarded cleanly rather than
// partially decoded: the reportStates table resets to empty and Restore
// returns no error. Account/region, providers, taggers, and untaggers are
// unaffected by the mismatch branch specifically (account/region simply
// aren't touched; providers/taggers/untaggers/the resource cache are always
// cleared by Restore regardless of version, matching pre-conversion
// behavior).
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)
	resourcegroupstaggingapi.AddReportStateInternal(b, "RUNNING", "s3://bucket/key.csv", "2025-06-01T00:00:00Z")
	require.True(t, resourcegroupstaggingapi.HasReportState(b))

	// A syntactically valid but version-mismatched snapshot.
	err := b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.False(t, resourcegroupstaggingapi.HasReportState(b))
	assert.Empty(t, resourcegroupstaggingapi.ReportStateRegions(b))
}

// TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero verifies that a
// snapshot with no version field at all decodes with Version == 0, which
// mismatches resourcegroupstaggingapiSnapshotVersion and is discarded the
// same way any other incompatible version is -- covering the pre-Phase-3.3
// on-disk shape (a flat {"reportStates": {...}, "accountID": ..., "region":
// ...} object with no version field), which must not be partially decoded
// as the new store.Registry-backed shape.
func TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero(t *testing.T) {
	t.Parallel()

	b := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)
	resourcegroupstaggingapi.AddReportStateInternal(b, "RUNNING", "s3://bucket/key.csv", "2025-06-01T00:00:00Z")
	require.True(t, resourcegroupstaggingapi.HasReportState(b))

	oldShape := `{"reportStates":{"us-east-1":{"s3Location":"s3://old/report.csv",` +
		`"startDate":"2024-01-01T00:00:00Z","status":"SUCCEEDED"}},` +
		`"accountID":"000000000000","region":"us-east-1"}`

	err := b.Restore(t.Context(), []byte(oldShape))
	require.NoError(t, err)

	assert.False(t, resourcegroupstaggingapi.HasReportState(b))
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every piece of state this backend persists: the
// store.Table-backed reportStates collection (populated across multiple
// regions, proving the region-keyed identity round-trips correctly through
// the reportStateSnapshot DTO) plus the scalar accountID/region fields.
// caches and the providers/filteredProviders/taggers/untaggers callback
// slices are deliberately NOT part of this assertion: they were never
// persisted before this conversion (ephemeral cache, unserializable runtime
// callbacks respectively) and remain so.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := resourcegroupstaggingapi.NewInMemoryBackend("111122223333", "us-west-2")

	resourcegroupstaggingapi.AddReportStateForRegion(
		original, "us-west-2", "SUCCEEDED", "s3://bucket-west/report.csv", "2025-06-01T00:00:00Z",
	)
	resourcegroupstaggingapi.AddReportStateForRegion(
		original, "us-east-1", "RUNNING", "s3://bucket-east/report.csv", "2025-06-02T00:00:00Z",
	)
	resourcegroupstaggingapi.AddReportStateForRegion(
		original, "eu-west-1", "FAILED", "s3://bucket-eu/report.csv", "2025-06-03T00:00:00Z",
	)

	snap := original.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	restored := resourcegroupstaggingapi.NewInMemoryBackend("000000000000", "ap-southeast-1")
	require.NoError(t, restored.Restore(t.Context(), snap))

	assert.Equal(t, "111122223333", restored.AccountID())
	assert.Equal(t, "us-west-2", restored.Region())

	assert.ElementsMatch(t, []string{"us-west-2", "us-east-1", "eu-west-1"},
		resourcegroupstaggingapi.ReportStateRegions(restored))
	assert.Equal(t, "SUCCEEDED", resourcegroupstaggingapi.ReportStatusForRegion(restored, "us-west-2"))
	assert.Equal(t, "RUNNING", resourcegroupstaggingapi.ReportStatusForRegion(restored, "us-east-1"))
	assert.Equal(t, "FAILED", resourcegroupstaggingapi.ReportStatusForRegion(restored, "eu-west-1"))
}

// TestInMemoryBackend_Snapshot_EmptyBackend verifies that snapshotting and
// restoring a backend with no report state at all round-trips to an empty
// (not nil-panicking, not erroring) reportStates table.
func TestInMemoryBackend_Snapshot_EmptyBackend(t *testing.T) {
	t.Parallel()

	original := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)

	snap := original.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	restored := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, restored.Restore(t.Context(), snap))

	assert.Empty(t, resourcegroupstaggingapi.ReportStateRegions(restored))
}

// TestInMemoryBackend_SnapshotRestore_PreservesReportStartTime verifies that a
// report's real start time survives a Snapshot->Restore round trip. Before this
// fix, reportCreationState.startedAt (an unexported, unpersisted time.Time used
// only for the RUNNING->SUCCEEDED and 90-day-staleness checks in
// DescribeReportCreation) was silently dropped by the reportStateSnapshot DTO,
// so it decoded as time.Time{} (year 1) on every restore. That made both checks
// fire immediately regardless of the report's actual age: a just-started RUNNING
// report would flip to SUCCEEDED and then immediately past-90-days to NO REPORT
// on the very next DescribeReportCreation call after a restore, even though no
// real time had elapsed.
func TestInMemoryBackend_SnapshotRestore_PreservesReportStartTime(t *testing.T) {
	t.Parallel()

	start := time.Now()

	original := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)
	resourcegroupstaggingapi.SetClockFunc(original, func() time.Time { return start })

	_, err := original.StartReportCreation(context.Background(), &resourcegroupstaggingapi.StartReportCreationInput{
		S3Bucket: "bkt",
	})
	require.NoError(t, err)

	snap := original.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	restored := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)
	// Only a second of real time passes across the restore -- nowhere near
	// reportRunningDuration (30s) or the 90-day staleness window.
	afterRestore := start.Add(time.Second)
	resourcegroupstaggingapi.SetClockFunc(restored, func() time.Time { return afterRestore })

	require.NoError(t, restored.Restore(t.Context(), snap))

	out := restored.DescribeReportCreation(context.Background())
	require.NotNil(t, out.Status)
	assert.Equal(t, "RUNNING", *out.Status,
		"a freshly restored RUNNING report must still read RUNNING when no real time has passed")
}

// TestHandler_SnapshotRestoreDelegate verifies the dead-wiring fix: Handler
// now delegates Snapshot/Restore to its backend (it did not before this
// Phase 3.3 conversion, even though the backend already implemented both),
// so the persistence manager -- which only ever holds a Handler, registered
// via the service registry -- can actually reach the backend's state.
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	backend := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)
	resourcegroupstaggingapi.AddReportStateInternal(backend, "SUCCEEDED", "s3://bucket/key.csv", "2025-06-01T00:00:00Z")

	h := resourcegroupstaggingapi.NewHandler(backend)

	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	backend2 := resourcegroupstaggingapi.NewInMemoryBackend(testAccountID, testRegion)
	h2 := resourcegroupstaggingapi.NewHandler(backend2)

	require.NoError(t, h2.Restore(t.Context(), snap))
	assert.True(t, resourcegroupstaggingapi.HasReportState(backend2))
	assert.Equal(t, "SUCCEEDED", resourcegroupstaggingapi.ReportStatus(backend2))
}
