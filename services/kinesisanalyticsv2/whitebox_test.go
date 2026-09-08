package kinesisanalyticsv2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackend_ListApplicationSnapshots_TieBreak proves ListApplicationSnapshots's
// sort.Slice on SnapshotCreation alone is not a total order: sort.Slice is not
// guaranteed stable, so two snapshots with an identical creation timestamp can
// come back in either relative order depending on the pre-sort order of the
// underlying byApp index group -- which an unrelated Delete on a third
// snapshot in the same group can silently change, via Index.remove's
// swap-with-last-element removal (pkgs/store/index.go). A client paginating
// this listing can then see a tied pair swap sides of a page boundary with
// nothing about either snapshot itself having changed.
func TestBackend_ListApplicationSnapshots_TieBreak(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := NewInMemoryBackend("000000000000", "us-east-1")

	app, err := b.CreateApplication(ctx, "tie-snap-app", "FLINK-1_18", "", "", "", nil)
	require.NoError(t, err)

	tie := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snapA := &Snapshot{
		ApplicationARN: app.ApplicationARN, SnapshotName: "snap-a", SnapshotStatus: ApplicationStatusReady,
		Region: b.defaultRegion, AppName: "tie-snap-app", SnapshotCreation: tie,
	}
	snapB := &Snapshot{
		ApplicationARN: app.ApplicationARN, SnapshotName: "snap-b", SnapshotStatus: ApplicationStatusReady,
		Region: b.defaultRegion, AppName: "tie-snap-app", SnapshotCreation: tie,
	}
	padding := &Snapshot{
		ApplicationARN: app.ApplicationARN, SnapshotName: "snap-padding", SnapshotStatus: ApplicationStatusReady,
		Region: b.defaultRegion, AppName: "tie-snap-app", SnapshotCreation: tie.Add(-time.Hour),
	}

	// Insertion order into the byApp index group: padding, A, B.
	b.snapshots.Put(padding)
	b.snapshots.Put(snapA)
	b.snapshots.Put(snapB)

	before, _, err := b.ListApplicationSnapshots(ctx, "tie-snap-app", "")
	require.NoError(t, err)
	require.Len(t, before, 3)
	require.Equal(t, "snap-a", before[1].SnapshotName)
	require.Equal(t, "snap-b", before[2].SnapshotName)

	// Deleting padding (index 0 of [padding, A, B]) swaps the last element
	// (B) into its slot, leaving the group as [B, A] -- reordering A and B
	// relative to each other despite neither being touched.
	require.True(t, b.snapshots.Delete(snapshotKey(b.defaultRegion, "tie-snap-app", "snap-padding")))

	after, _, err := b.ListApplicationSnapshots(ctx, "tie-snap-app", "")
	require.NoError(t, err)
	require.Len(t, after, 2)

	const wantMsg = "tied snapshots must sort in a stable order (by SnapshotName) " +
		"regardless of an unrelated delete elsewhere in the same application"
	assert.Equal(t, "snap-a", after[0].SnapshotName, wantMsg)
	assert.Equal(t, "snap-b", after[1].SnapshotName)
}

// TestBackend_UpdateApplication_ConditionalToken verifies that
// UpdateApplication's ConditionalToken implements the same
// optimistic-concurrency check as CurrentApplicationVersionId (real AWS: "you
// must provide the CurrentApplicationVersionId or the ConditionalToken"),
// and that a mismatched token is rejected with ErrConcurrentModification
// without mutating the application or bumping its version.
func TestBackend_UpdateApplication_ConditionalToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("valid token succeeds and rotates", func(t *testing.T) {
		t.Parallel()

		b := NewInMemoryBackend("000000000000", "us-east-1")
		app, err := b.CreateApplication(ctx, "token-app", "FLINK-1_18", "", "", "", nil)
		require.NoError(t, err)

		tok := conditionalToken(app)

		updated, opID, err := b.UpdateApplication(ctx, UpdateApplicationParams{
			Name:                       "token-app",
			ConditionalToken:           tok,
			ServiceExecutionRoleUpdate: "arn:aws:iam::000000000000:role/updated-via-token",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, opID)
		assert.Equal(t, int64(2), updated.ApplicationVersionID)
		assert.NotEqual(t, tok, conditionalToken(updated), "token must rotate on version bump")
	})

	t.Run("stale token rejected", func(t *testing.T) {
		t.Parallel()

		b := NewInMemoryBackend("000000000000", "us-east-1")
		app, err := b.CreateApplication(ctx, "stale-token-app", "FLINK-1_18", "", "orig", "", nil)
		require.NoError(t, err)

		staleTok := conditionalToken(app)

		// Bump the version once via a normal update so staleTok no longer matches.
		_, _, err = b.UpdateApplication(ctx, UpdateApplicationParams{
			Name:                       "stale-token-app",
			ServiceExecutionRoleUpdate: "arn:aws:iam::000000000000:role/first-update",
		})
		require.NoError(t, err)

		_, _, err = b.UpdateApplication(ctx, UpdateApplicationParams{
			Name:                       "stale-token-app",
			ConditionalToken:           staleTok,
			ServiceExecutionRoleUpdate: "arn:aws:iam::000000000000:role/should-not-apply",
		})
		require.ErrorIs(t, err, ErrConcurrentModification)

		current, err := b.DescribeApplication(ctx, "stale-token-app")
		require.NoError(t, err)
		assert.Equal(t, "arn:aws:iam::000000000000:role/first-update", current.ServiceExecutionRole,
			"rejected update must not mutate state")
	})
}
