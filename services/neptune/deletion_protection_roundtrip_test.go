package neptune_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	neptunesdk "github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/aws/aws-sdk-go-v2/service/neptune/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/neptune"
)

// TestDeleteGlobalCluster_DeletionProtectionRoundTrip proves ModifyGlobalCluster's
// DeletionProtection has an effect on DeleteGlobalCluster, mirroring the sibling fix
// already present in rds and the identical fix just made in docdb for the same
// concept. DeleteGlobalCluster's own deserializer (neptune@v1.48.4
// deserializers.go:2905-2911) models InvalidGlobalClusterStateFault as a typed error
// for this op -- before the fix, the field was stored on the global cluster and read
// only by Describe/serialization code, so DeleteGlobalCluster always succeeded
// regardless of the setting.
func TestDeleteGlobalCluster_DeletionProtectionRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		id        string
		protected bool
		wantErr   bool
	}{
		{"protected blocks delete", "dp-rt-protected", true, true},
		{"unprotected allows delete", "dp-rt-unprotected", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := neptune.NewInMemoryBackend("000000000000", testRegion)
			h := neptune.NewHandler(backend)
			client := newTestNeptuneClient(t, h)
			ctx := t.Context()

			_, err := client.CreateGlobalCluster(ctx, &neptunesdk.CreateGlobalClusterInput{
				GlobalClusterIdentifier: aws.String(tt.id),
			})
			require.NoError(t, err)

			_, err = client.ModifyGlobalCluster(ctx, &neptunesdk.ModifyGlobalClusterInput{
				GlobalClusterIdentifier: aws.String(tt.id),
				DeletionProtection:      aws.Bool(tt.protected),
			})
			require.NoError(t, err)

			_, err = client.DeleteGlobalCluster(ctx, &neptunesdk.DeleteGlobalClusterInput{
				GlobalClusterIdentifier: aws.String(tt.id),
			})

			if tt.wantErr {
				require.Error(t, err)

				var invalidState *types.InvalidGlobalClusterStateFault
				require.ErrorAs(t, err, &invalidState,
					"expected a typed InvalidGlobalClusterStateFault, got %v", err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestDeleteGlobalCluster_MembersAttachedRoundTrip proves DeleteGlobalCluster
// rejects a global cluster that still has attached member DB clusters
// (api_op_DeleteGlobalCluster.go: "Deletes a global database. The primary and
// all secondary clusters must already be detached or deleted first.") --
// before the fix, DeleteGlobalCluster never inspected GlobalClusterMembers at
// all, so it always succeeded and silently orphaned the attached DB cluster's
// own GlobalClusterIdentifier back-reference.
func TestDeleteGlobalCluster_MembersAttachedRoundTrip(t *testing.T) {
	t.Parallel()

	backend := neptune.NewInMemoryBackend("000000000000", testRegion)
	h := neptune.NewHandler(backend)
	client := newTestNeptuneClient(t, h)
	ctx := t.Context()

	_, err := client.CreateGlobalCluster(ctx, &neptunesdk.CreateGlobalClusterInput{
		GlobalClusterIdentifier: aws.String("dp-rt-members-global"),
	})
	require.NoError(t, err)

	created, err := client.CreateDBCluster(ctx, &neptunesdk.CreateDBClusterInput{
		DBClusterIdentifier:     aws.String("dp-rt-members-cluster"),
		Engine:                  aws.String("neptune"),
		GlobalClusterIdentifier: aws.String("dp-rt-members-global"),
	})
	require.NoError(t, err)

	_, err = client.DeleteGlobalCluster(ctx, &neptunesdk.DeleteGlobalClusterInput{
		GlobalClusterIdentifier: aws.String("dp-rt-members-global"),
	})
	require.Error(t, err)

	var invalidState *types.InvalidGlobalClusterStateFault
	require.ErrorAs(t, err, &invalidState,
		"expected a typed InvalidGlobalClusterStateFault, got %v", err)

	_, err = client.RemoveFromGlobalCluster(ctx, &neptunesdk.RemoveFromGlobalClusterInput{
		GlobalClusterIdentifier: aws.String("dp-rt-members-global"),
		DbClusterIdentifier:     created.DBCluster.DBClusterArn,
	})
	require.NoError(t, err)

	_, err = client.DeleteGlobalCluster(ctx, &neptunesdk.DeleteGlobalClusterInput{
		GlobalClusterIdentifier: aws.String("dp-rt-members-global"),
	})
	require.NoError(t, err)
}

// TestDeleteGlobalCluster_MemberDeletedDirectlyRoundTrip proves deleting a
// global cluster's member DB cluster directly (DeleteDBCluster, not
// RemoveFromGlobalCluster) also clears the membership -- otherwise the fix in
// TestDeleteGlobalCluster_MembersAttachedRoundTrip would leave a ghost member
// entry that blocks DeleteGlobalCluster forever, contradicting
// api_op_DeleteGlobalCluster.go's own doc comment ("must already be detached
// or deleted first" -- "deleted" is a named, sufficient alternative to
// detaching).
func TestDeleteGlobalCluster_MemberDeletedDirectlyRoundTrip(t *testing.T) {
	t.Parallel()

	backend := neptune.NewInMemoryBackend("000000000000", testRegion)
	h := neptune.NewHandler(backend)
	client := newTestNeptuneClient(t, h)
	ctx := t.Context()

	_, err := client.CreateGlobalCluster(ctx, &neptunesdk.CreateGlobalClusterInput{
		GlobalClusterIdentifier: aws.String("dp-rt-direct-global"),
	})
	require.NoError(t, err)

	_, err = client.CreateDBCluster(ctx, &neptunesdk.CreateDBClusterInput{
		DBClusterIdentifier:     aws.String("dp-rt-direct-cluster"),
		Engine:                  aws.String("neptune"),
		GlobalClusterIdentifier: aws.String("dp-rt-direct-global"),
	})
	require.NoError(t, err)

	_, err = client.DeleteDBCluster(ctx, &neptunesdk.DeleteDBClusterInput{
		DBClusterIdentifier: aws.String("dp-rt-direct-cluster"),
		SkipFinalSnapshot:   aws.Bool(true),
	})
	require.NoError(t, err)

	described, err := client.DescribeGlobalClusters(ctx, &neptunesdk.DescribeGlobalClustersInput{
		GlobalClusterIdentifier: aws.String("dp-rt-direct-global"),
	})
	require.NoError(t, err)
	require.Len(t, described.GlobalClusters, 1)
	assert.Empty(t, described.GlobalClusters[0].GlobalClusterMembers,
		"deleted DB cluster should no longer be reported as an attached global cluster member")

	_, err = client.DeleteGlobalCluster(ctx, &neptunesdk.DeleteGlobalClusterInput{
		GlobalClusterIdentifier: aws.String("dp-rt-direct-global"),
	})
	require.NoError(t, err)
}
