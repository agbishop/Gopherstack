package rds_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestDeleteDBCluster_CascadeDeletesClusterEndpoints is a regression test for
// a ghost-row leak: DeleteDBClusterWithOptions must remove every custom
// cluster endpoint belonging to the deleted cluster, not just the cluster
// itself. Before the fix, DescribeDBClusterEndpoints kept returning
// endpoints pointing at a deleted cluster forever (the clusterEndpoints map
// only shrank when a client explicitly called DeleteDBClusterEndpoint).
func TestDeleteDBCluster_CascadeDeletesClusterEndpoints(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	_, err := b.CreateDBCluster(
		"leak-cluster", "aurora-mysql", "admin", "", "", 0, nil, rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	_, err = b.CreateDBClusterEndpoint("leak-endpoint-1", "leak-cluster", "READER")
	require.NoError(t, err)
	_, err = b.CreateDBClusterEndpoint("leak-endpoint-2", "leak-cluster", "WRITER")
	require.NoError(t, err)

	// Sanity check: both endpoints exist before delete.
	before, err := b.DescribeDBClusterEndpoints("leak-cluster", "")
	require.NoError(t, err)
	assert.Len(t, before, 2)

	_, err = b.DeleteDBClusterWithOptions("leak-cluster", true, "")
	require.NoError(t, err)

	// The endpoints must be gone, not just the cluster. DBClusterEndpointIdentifier
	// is a filter (see DescribeDBClusterEndpoints), so a gone endpoint yields an
	// empty result, not a not-found error.
	got1, err := b.DescribeDBClusterEndpoints("", "leak-endpoint-1")
	require.NoError(t, err)
	assert.Empty(t, got1)
	got2, err := b.DescribeDBClusterEndpoints("", "leak-endpoint-2")
	require.NoError(t, err)
	assert.Empty(t, got2)

	after, err := b.DescribeDBClusterEndpoints("leak-cluster", "")
	require.NoError(t, err)
	assert.Empty(t, after)
}

func TestModifyDBClusterEndpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs    error
		name         string
		endpointID   string
		endpointType string
		wantErr      bool
	}{
		{
			name:         "success updates type",
			endpointID:   "my-endpoint",
			endpointType: "READER",
		},
		{
			name:       "not found",
			endpointID: "missing",
			wantErr:    true,
			wantErrIs:  rds.ErrClusterEndpointNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if !tt.wantErr {
				_, err := b.CreateDBCluster(
					"my-cluster",
					"aurora-mysql",
					"admin",
					"",
					"",
					0,
					nil,
					rds.DBClusterOptions{},
				)
				require.NoError(t, err)
				_, err = b.CreateDBClusterEndpoint(tt.endpointID, "my-cluster", "WRITER")
				require.NoError(t, err)
			}
			got, err := b.ModifyDBClusterEndpoint(tt.endpointID, tt.endpointType)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			if tt.endpointType != "" {
				assert.Equal(t, tt.endpointType, got.EndpointType)
			}
		})
	}
}
