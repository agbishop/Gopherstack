package rds_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

func TestEnableDisableHttpEndpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs   error
		name        string
		clusterID   string
		resourceARN string
		enable      bool
		wantErr     bool
	}{
		{
			name:        "enable success",
			clusterID:   "my-cluster",
			resourceARN: "arn:aws:rds:us-east-1:123456789012:cluster:my-cluster",
			enable:      true,
		},
		{
			name:        "disable success",
			clusterID:   "my-cluster",
			resourceARN: "arn:aws:rds:us-east-1:123456789012:cluster:my-cluster",
			enable:      false,
		},
		{
			name:        "not found enable",
			resourceARN: "arn:aws:rds:us-east-1:123456789012:cluster:missing",
			enable:      true,
			wantErr:     true,
			wantErrIs:   rds.ErrResourceNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if tt.clusterID != "" {
				_, err := b.CreateDBCluster(
					tt.clusterID,
					"aurora-mysql",
					"admin",
					"",
					"",
					0,
					nil,
					rds.DBClusterOptions{},
				)
				require.NoError(t, err)
			}
			var err error
			if tt.enable {
				err = b.EnableHTTPEndpoint(tt.resourceARN)
			} else {
				err = b.DisableHTTPEndpoint(tt.resourceARN)
			}
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
		})
	}
}

// TestEnableHTTPEndpointByID verifies EnableHTTPEndpoint works with cluster identifier.
func TestEnableHTTPEndpointByID(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	_, err := b.CreateDBCluster("http-cluster", "aurora-postgresql", "admin", "", "", 0, nil, rds.DBClusterOptions{})
	require.NoError(t, err)
	// Enable by cluster ID directly.
	err = b.EnableHTTPEndpoint("http-cluster")
	require.NoError(t, err)
	got, err := b.DescribeDBClusters("http-cluster")
	require.NoError(t, err)
	assert.True(t, got[0].HTTPEndpointEnabled)
	// Disable by ARN.
	err = b.DisableHTTPEndpoint("arn:aws:rds:us-east-1:123456789012:cluster:http-cluster")
	require.NoError(t, err)
	got, err = b.DescribeDBClusters("http-cluster")
	require.NoError(t, err)
	assert.False(t, got[0].HTTPEndpointEnabled)
}
