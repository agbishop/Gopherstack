package rds_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/rds"
)

func newOverlayTestCluster(t *testing.T, h *rds.Handler, id string) {
	t.Helper()

	_, err := h.Backend.CreateDBCluster(
		id,
		"aurora-postgresql",
		"admin",
		"",
		"",
		0,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)
}

func TestRDS_DescribeDBClusters_FailoverStatusOverlay(t *testing.T) {
	t.Parallel()

	t.Run("active_fault_overlays_status", func(t *testing.T) {
		t.Parallel()

		h := newFISRDSHandler()
		const clusterID = "overlay-active-cluster"
		newOverlayTestCluster(t, h, clusterID)

		err := h.ExecuteFISAction(t.Context(), service.FISActionExecution{
			ActionID: "aws:rds:failover-db-cluster",
			Targets:  []string{"arn:aws:rds:us-east-1:000000000000:cluster/" + clusterID},
			Duration: 0,
		})
		require.NoError(t, err)

		clusters, err := h.Backend.DescribeDBClusters(clusterID)
		require.NoError(t, err)
		require.Len(t, clusters, 1)
		assert.Equal(t, "failing-over", clusters[0].Status)
	})

	t.Run("no_fault_reports_normal_status", func(t *testing.T) {
		t.Parallel()

		h := newFISRDSHandler()
		const clusterID = "overlay-none-cluster"
		newOverlayTestCluster(t, h, clusterID)

		clusters, err := h.Backend.DescribeDBClusters(clusterID)
		require.NoError(t, err)
		require.Len(t, clusters, 1)
		assert.Equal(t, "available", clusters[0].Status)
	})

	t.Run("expired_fault_does_not_overlay", func(t *testing.T) {
		t.Parallel()

		h := newFISRDSHandler()
		const clusterID = "overlay-expired-cluster"
		newOverlayTestCluster(t, h, clusterID)

		h.Backend.InjectExpiredFaultForTest(clusterID)

		clusters, err := h.Backend.DescribeDBClusters(clusterID)
		require.NoError(t, err)
		require.Len(t, clusters, 1)
		assert.Equal(t, "available", clusters[0].Status)
	})

	t.Run("overlay_applies_when_listing_all_clusters", func(t *testing.T) {
		t.Parallel()

		h := newFISRDSHandler()
		const clusterID = "overlay-list-cluster"
		newOverlayTestCluster(t, h, clusterID)

		err := h.ExecuteFISAction(t.Context(), service.FISActionExecution{
			ActionID: "aws:rds:failover-db-cluster",
			Targets:  []string{"arn:aws:rds:us-east-1:000000000000:cluster/" + clusterID},
			Duration: 0,
		})
		require.NoError(t, err)

		clusters, err := h.Backend.DescribeDBClusters("")
		require.NoError(t, err)

		idx := -1
		for i, c := range clusters {
			if c.DBClusterIdentifier == clusterID {
				idx = i
			}
		}
		require.NotEqual(t, -1, idx, "expected cluster to be present in list-all Describe")
		assert.Equal(t, "failing-over", clusters[idx].Status)
	})
}

func TestRDS_FailoverDBCluster_FinalStatusUnchanged(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDBCluster(
		"failover-final-cluster",
		"aurora-postgresql",
		"admin",
		"",
		"",
		0,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	updated, err := b.FailoverDBCluster("failover-final-cluster", "")
	require.NoError(t, err)
	assert.Equal(t, "available", updated.Status)
}
