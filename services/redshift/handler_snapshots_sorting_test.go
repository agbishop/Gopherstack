package redshift_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	redshiftsdk "github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// gopherstack-igsa gap 2: DescribeClusterSnapshots ignored SortingEntities
// entirely. Real DescribeClusterSnapshotsInput.SortingEntities
// (redshift@v1.65.4 api_op_DescribeClusterSnapshots.go) is
// []types.SnapshotSortingEntity{Attribute, SortOrder}, wire-encoded as
// "SortingEntities.SnapshotSortingEntity.N.Attribute"/".SortOrder"
// (awsAwsquery_serializeDocumentSnapshotSortingEntityList, serializers.go).
// These tests drive the real SDK client to prove the wire shape and the
// sort itself. TOTAL_SIZE is deliberately not covered: this backend has no
// snapshot-size field to sort by.

func snapshotIdentifiers(out *redshiftsdk.DescribeClusterSnapshotsOutput) []string {
	ids := make([]string, 0, len(out.Snapshots))
	for _, s := range out.Snapshots {
		ids = append(ids, aws.ToString(s.SnapshotIdentifier))
	}

	return ids
}

// TestDescribeClusterSnapshots_SortingEntities_SourceType verifies
// SOURCE_TYPE sorting (types.SnapshotAttributeToSortBySourceType), backed by
// the real Snapshot.SnapshotType field.
func TestDescribeClusterSnapshots_SortingEntities_SourceType(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	now := time.Now()
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "snap-manual", SnapshotType: "manual", SnapshotCreateTime: now,
	})
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "snap-automated", SnapshotType: "automated", SnapshotCreateTime: now,
	})

	out, err := client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		SortingEntities: []types.SnapshotSortingEntity{
			{Attribute: types.SnapshotAttributeToSortBySourceType, SortOrder: types.SortByOrderAscending},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap-automated", "snap-manual"}, snapshotIdentifiers(out))

	out, err = client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		SortingEntities: []types.SnapshotSortingEntity{
			{Attribute: types.SnapshotAttributeToSortBySourceType, SortOrder: types.SortByOrderDescending},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap-manual", "snap-automated"}, snapshotIdentifiers(out))
}

// TestDescribeClusterSnapshots_SortingEntities_CreateTime verifies
// CREATE_TIME sorting (types.SnapshotAttributeToSortByCreateTime), backed by
// the real Snapshot.SnapshotCreateTime field.
func TestDescribeClusterSnapshots_SortingEntities_CreateTime(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "snap-2022", SnapshotType: "manual", SnapshotCreateTime: base.AddDate(-2, 0, 0),
	})
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "snap-2024", SnapshotType: "manual", SnapshotCreateTime: base,
	})
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "snap-2023", SnapshotType: "manual", SnapshotCreateTime: base.AddDate(-1, 0, 0),
	})

	out, err := client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		SortingEntities: []types.SnapshotSortingEntity{
			{Attribute: types.SnapshotAttributeToSortByCreateTime, SortOrder: types.SortByOrderAscending},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap-2022", "snap-2023", "snap-2024"}, snapshotIdentifiers(out))

	out, err = client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		SortingEntities: []types.SnapshotSortingEntity{
			{Attribute: types.SnapshotAttributeToSortByCreateTime, SortOrder: types.SortByOrderDescending},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap-2024", "snap-2023", "snap-2022"}, snapshotIdentifiers(out))
}

// TestDescribeClusterSnapshots_SortingEntities_Unset verifies that omitting
// SortingEntities leaves the pre-existing (identifier-ascending) order
// unchanged. The two snapshots are seeded so identifier order deliberately
// disagrees with both type order and create-time order: an implementation
// that defaults to sorting by SOURCE_TYPE or CREATE_TIME even when
// SortingEntities is absent would produce the opposite order and fail here.
func TestDescribeClusterSnapshots_SortingEntities_Unset(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	now := time.Now()
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "aaa-late", SnapshotType: "manual", SnapshotCreateTime: now,
	})
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "zzz-early", SnapshotType: "automated", SnapshotCreateTime: now.AddDate(-1, 0, 0),
	})

	out, err := client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{})
	require.NoError(t, err)
	assert.Equal(t, []string{"aaa-late", "zzz-early"}, snapshotIdentifiers(out))
}
