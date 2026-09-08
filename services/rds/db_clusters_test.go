package rds_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

func TestDBCluster_ModifyFields(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBCluster(
		"mod-cluster",
		"aurora-postgresql",
		"admin",
		"",
		"",
		5432,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	updated, err := b.ModifyDBCluster("mod-cluster", "", rds.DBClusterOptions{
		DeletionProtection:    true,
		DeletionProtectionSet: true,
	})
	require.NoError(t, err)
	assert.True(t, updated.DeletionProtection)

	updated, err = b.ModifyDBCluster("mod-cluster", "", rds.DBClusterOptions{
		DeletionProtection:    false,
		DeletionProtectionSet: true,
	})
	require.NoError(t, err)
	assert.False(t, updated.DeletionProtection)
}

func TestDBCluster_FailoverUpdatesState(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBCluster(
		"failover-cluster",
		"aurora-mysql",
		"admin",
		"",
		"",
		3306,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	result, err := b.FailoverDBCluster("failover-cluster", "")
	require.NoError(t, err)
	assert.Equal(t, "failover-cluster", result.DBClusterIdentifier)
}

func TestDBCluster_RebootTransitions(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBCluster(
		"reboot-cluster",
		"aurora-postgresql",
		"admin",
		"",
		"",
		5432,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	result, err := b.RebootDBCluster("reboot-cluster")
	require.NoError(t, err)
	assert.Equal(t, "reboot-cluster", result.DBClusterIdentifier)
}

func TestCreateDBCluster_RejectsInvalidStorageType(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBCluster(
		"bad-storage-cluster",
		"aurora-postgresql",
		"admin",
		"",
		"",
		5432,
		nil,
		rds.DBClusterOptions{StorageType: "gp3"},
	)
	require.ErrorIs(t, err, rds.ErrInvalidParameter)

	_, descErr := b.DescribeDBClusters("bad-storage-cluster")
	assert.Error(t, descErr)
}

func TestDBCluster_HTTP_Modify(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()

	rec := postRDSForm(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"http-mod-cluster"},
		"Engine":              {"aurora-postgresql"},
		"MasterUsername":      {"admin"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":              {"ModifyDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"http-mod-cluster"},
		"DeletionProtection":  {"true"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":              {"FailoverDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"http-mod-cluster"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
}

// Test_DeleteDBCluster_FinalSnapshotContract verifies AWS's
// SkipFinalSnapshot/FinalDBSnapshotIdentifier contract for DeleteDBCluster:
// a final snapshot must be requested unless explicitly skipped, the two
// parameters are mutually exclusive, and a real manual cluster snapshot is
// created and persisted before the cluster is removed.
func Test_DeleteDBCluster_FinalSnapshotContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		finalSnapshotID   string
		wantErrContains   string
		skipFinalSnapshot bool
	}{
		{
			name:            "missing both SkipFinalSnapshot and FinalDBSnapshotIdentifier is rejected",
			wantErrContains: "FinalDBSnapshotIdentifier",
		},
		{
			name:              "SkipFinalSnapshot with FinalDBSnapshotIdentifier is rejected",
			skipFinalSnapshot: true,
			finalSnapshotID:   "final-csnap",
			wantErrContains:   "FinalDBSnapshotIdentifier",
		},
		{
			name:              "SkipFinalSnapshot alone deletes without a snapshot",
			skipFinalSnapshot: true,
		},
		{
			name:            "FinalDBSnapshotIdentifier alone takes a final snapshot",
			finalSnapshotID: "final-csnap",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rds.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateDBCluster(
				"del-cluster", "aurora-postgresql", "admin", "", "", 0, nil, rds.DBClusterOptions{},
			)
			require.NoError(t, err)

			_, err = b.DeleteDBClusterWithOptions("del-cluster", tt.skipFinalSnapshot, tt.finalSnapshotID)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.Contains(t, err.Error(), "InvalidParameterCombination")
				// The cluster must NOT have been deleted on a rejected request.
				_, describeErr := b.DescribeDBClusters("del-cluster")
				assert.NoError(t, describeErr, "cluster should still exist after a rejected delete")

				return
			}

			require.NoError(t, err)

			_, describeErr := b.DescribeDBClusters("del-cluster")
			require.Error(t, describeErr, "cluster should be removed after delete")

			snaps, _ := b.DescribeDBClusterSnapshots("", "del-cluster")
			if tt.finalSnapshotID == "" {
				assert.Empty(t, snaps, "no final snapshot should be created when SkipFinalSnapshot is set")

				return
			}

			require.Len(t, snaps, 1, "exactly one final cluster snapshot should be created")
			snap := snaps[0]
			assert.Equal(t, tt.finalSnapshotID, snap.DBClusterSnapshotIdentifier)
			assert.Equal(t, "del-cluster", snap.DBClusterIdentifier)
			assert.Equal(t, "aurora-postgresql", snap.Engine)
		})
	}
}

// Test_DeleteDBCluster_NotFoundBeforeParamValidation verifies that a
// nonexistent cluster yields DBClusterNotFound even when the
// SkipFinalSnapshot/FinalDBSnapshotIdentifier combination is also invalid.
func Test_DeleteDBCluster_NotFoundBeforeParamValidation(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	rec := postRDSForm(t, h, "Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=missing-cluster")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "DBClusterNotFound")
	assert.NotContains(t, rec.Body.String(), "InvalidParameterCombination")
}

// TestDeleteDBClusterCascadeClusterRoles verifies that deleting a cluster
// also removes its IAM role associations.
func TestDeleteDBClusterCascadeClusterRoles(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddClusterInternal("my-cluster", "aurora-mysql")

	err := b.AddRoleToDBCluster("my-cluster", "arn:aws:iam::000:role/R1", "")
	require.NoError(t, err)
	require.Equal(t, 1, rds.ClusterRoleCount(b, "my-cluster"))

	_, err = b.DeleteDBCluster("my-cluster")
	require.NoError(t, err)

	assert.Equal(t, 0, rds.ClusterRoleCount(b, "my-cluster"))
}

// TestBacktrackDBCluster_UniqueIDs verifies each call returns a different BacktrackIdentifier.
func TestBacktrackDBCluster_UniqueIDs(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddClusterInternal("aurora-cluster", "aurora-mysql")

	bt1, err := b.BacktrackDBCluster("aurora-cluster", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	bt2, err := b.BacktrackDBCluster("aurora-cluster", "2024-01-02T00:00:00Z")
	require.NoError(t, err)

	assert.NotEqual(t, bt1.BacktrackIdentifier, bt2.BacktrackIdentifier,
		"each BacktrackDBCluster call should return a unique BacktrackIdentifier")
	assert.True(t, strings.HasPrefix(bt1.BacktrackIdentifier, "backtrack-"))
}

// TestBacktrackDBCluster_EmptyBacktrackTo validates that an empty BacktrackTo returns error.
func TestBacktrackDBCluster_EmptyBacktrackTo(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddClusterInternal("aurora-cluster", "aurora-mysql")

	_, err := b.BacktrackDBCluster("aurora-cluster", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrInvalidParameter)
}

// TestHTTP_BacktrackDBCluster_EmptyBacktrackTo validates HTTP error for missing BacktrackTo.
func TestHTTP_BacktrackDBCluster_EmptyBacktrackTo(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddClusterInternal("aurora-cluster", "aurora-mysql")
	h := rds.NewHandler(b)

	rec := postRDSForm(t, h, "Action=BacktrackDBCluster&Version=2014-10-31&DBClusterIdentifier=aurora-cluster")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
}

// TestRDSBackend_BacktrackDBCluster tests BacktrackDBCluster.
func TestRDSBackend_BacktrackDBCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs   error
		setup       func(b *rds.InMemoryBackend)
		name        string
		clusterID   string
		backtrackTo string
		wantErr     bool
	}{
		{
			name: "success",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBCluster("my-cluster", "aurora-postgresql", "", "", "", 0, nil, rds.DBClusterOptions{})
			},
			clusterID:   "my-cluster",
			backtrackTo: "2024-01-01T00:00:00Z",
		},
		{
			name:        "cluster_not_found",
			setup:       func(_ *rds.InMemoryBackend) {},
			clusterID:   "no-such-cluster",
			backtrackTo: "2024-01-01T00:00:00Z",
			wantErr:     true,
			wantErrIs:   rds.ErrClusterNotFound,
		},
		{
			name:        "empty_cluster_id",
			setup:       func(_ *rds.InMemoryBackend) {},
			clusterID:   "",
			backtrackTo: "2024-01-01T00:00:00Z",
			wantErr:     true,
			wantErrIs:   rds.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rds.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)

			bt, err := b.BacktrackDBCluster(tt.clusterID, tt.backtrackTo)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.clusterID, bt.DBClusterIdentifier)
			assert.Equal(t, "applying", bt.Status)
		})
	}
}

// TestRDSBackend_DeletionProtection tests that resources with deletion protection cannot be deleted.
func TestRDSBackend_DeletionProtection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errIs   error
		setup   func(b *rds.InMemoryBackend)
		action  func(b *rds.InMemoryBackend) error
		name    string
		wantErr bool
	}{
		{
			name: "delete_protected_instance_blocked",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBInstance(
					"prot-db", "postgres", "", "", "", "", 20,
					rds.DBInstanceOptions{DeletionProtection: true},
				)
			},
			action: func(b *rds.InMemoryBackend) error {
				_, err := b.DeleteDBInstance("prot-db")

				return err
			},
			wantErr: true,
			errIs:   rds.ErrInvalidDBInstanceState,
		},
		{
			name: "delete_unprotected_instance_allowed",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateDBInstance(
					"unprot-db", "postgres", "", "", "", "", 20,
					rds.DBInstanceOptions{DeletionProtection: false},
				)
			},
			action: func(b *rds.InMemoryBackend) error {
				_, err := b.DeleteDBInstance("unprot-db")

				return err
			},
			wantErr: false,
		},
		{
			name: "delete_protected_global_cluster_blocked",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateGlobalCluster("gc-prot", "aurora-postgresql", "14.3", false, true)
			},
			action: func(b *rds.InMemoryBackend) error {
				_, err := b.DeleteGlobalCluster("gc-prot")

				return err
			},
			wantErr: true,
			errIs:   rds.ErrInvalidGlobalClusterState,
		},
		{
			name: "delete_unprotected_global_cluster_allowed",
			setup: func(b *rds.InMemoryBackend) {
				_, _ = b.CreateGlobalCluster("gc-unprot", "aurora-postgresql", "14.3", false, false)
			},
			action: func(b *rds.InMemoryBackend) error {
				_, err := b.DeleteGlobalCluster("gc-unprot")

				return err
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rds.NewInMemoryBackend("000000000000", "us-east-1")

			if tt.setup != nil {
				tt.setup(b)
			}

			err := tt.action(b)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errIs)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestCreateDBCluster_BackupRetentionPeriodBounds verifies that
// CreateDBCluster validates BackupRetentionPeriod within the AWS-allowed
// range [1, 35]. Real AWS returns InvalidParameterValue for out-of-range values
// and defaults to 1 when the parameter is omitted.
func TestCreateDBCluster_BackupRetentionPeriodBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		retention string
		wantCode  int
	}{
		{
			name:      "zero_rejected",
			retention: "0",
			wantCode:  http.StatusBadRequest,
		},
		{
			name:      "above_maximum_rejected",
			retention: "36",
			wantCode:  http.StatusBadRequest,
		},
		{
			name:      "minimum_boundary_accepted",
			retention: "1",
			wantCode:  http.StatusOK,
		},
		{
			name:      "maximum_boundary_accepted",
			retention: "35",
			wantCode:  http.StatusOK,
		},
		{
			name:      "mid_range_accepted",
			retention: "7",
			wantCode:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()
			body := "Action=CreateDBCluster" +
				"&DBClusterIdentifier=test-cluster-" + tt.name +
				"&Engine=aurora-mysql" +
				"&BackupRetentionPeriod=" + tt.retention

			rec := postRDSForm(t, h, body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateDBCluster BackupRetentionPeriod=%s", tt.retention)

			if tt.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "Error",
					"expected error response for BackupRetentionPeriod=%s", tt.retention)
			}
		})
	}
}

// TestCreateDBCluster_BackupRetentionPeriodDefault verifies that
// CreateDBCluster defaults BackupRetentionPeriod to 1 when omitted.
// Real AWS documents this default and includes it in DescribeDBClusters output.
func TestCreateDBCluster_BackupRetentionPeriodDefault(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	body := "Action=CreateDBCluster" +
		"&DBClusterIdentifier=default-retention-cluster" +
		"&Engine=aurora-postgresql"

	rec := postRDSForm(t, h, body)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<BackupRetentionPeriod>1</BackupRetentionPeriod>",
		"default BackupRetentionPeriod should be 1")
}

// TestCreateDBCluster_BackupRetentionPeriodPersisted verifies that an
// explicitly set BackupRetentionPeriod is round-tripped through DescribeDBClusters.
func TestCreateDBCluster_BackupRetentionPeriodPersisted(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()

	createBody := "Action=CreateDBCluster" +
		"&DBClusterIdentifier=ret-cluster" +
		"&Engine=aurora-mysql" +
		"&BackupRetentionPeriod=14"

	createRec := postRDSForm(t, h, createBody)
	require.Equal(t, http.StatusOK, createRec.Code)

	describeBody := "Action=DescribeDBClusters&DBClusterIdentifier=ret-cluster"
	describeRec := postRDSForm(t, h, describeBody)
	require.Equal(t, http.StatusOK, describeRec.Code)
	assert.Contains(t, describeRec.Body.String(), "<BackupRetentionPeriod>14</BackupRetentionPeriod>",
		"BackupRetentionPeriod=14 should be returned by DescribeDBClusters")
}

func TestFailoverDBCluster(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs error
		name      string
		clusterID string
		wantErr   bool
	}{
		{name: "success", clusterID: "my-cluster"},
		{name: "not found", clusterID: "missing", wantErr: true, wantErrIs: rds.ErrClusterNotFound},
		{
			name:      "wrong status",
			clusterID: "stopped-cluster",
			wantErr:   true,
			wantErrIs: rds.ErrInvalidDBClusterStateFault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend(t)
			if tt.name == "success" {
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
			if tt.name == "wrong status" {
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
				_, err = b.StopDBCluster(tt.clusterID)
				require.NoError(t, err)
			}
			got, err := b.FailoverDBCluster(tt.clusterID, "")
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.clusterID, got.DBClusterIdentifier)
		})
	}
}

// TestFailoverDBCluster_PromotesTarget verifies that FailoverDBCluster
// actually promotes a cluster member to writer (real AWS behavior: "promotes
// one of the Aurora Replicas... to be the primary DB instance, the cluster
// writer" -- rds@v1.124.1 api_op_FailoverDBCluster.go:13-14), instead of only
// flickering the cluster's Status.
func TestFailoverDBCluster_PromotesTarget(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	_, err := b.CreateDBCluster("fo-cluster", "aurora-mysql", "admin", "", "", 0, nil, rds.DBClusterOptions{})
	require.NoError(t, err)

	_, err = b.CreateDBInstance("fo-writer", "aurora-mysql", "db.r5.large", "", "", "", 20,
		rds.DBInstanceOptions{DBClusterIdentifier: "fo-cluster"})
	require.NoError(t, err)
	_, err = b.CreateDBInstance("fo-reader", "aurora-mysql", "db.r5.large", "", "", "", 20,
		rds.DBInstanceOptions{DBClusterIdentifier: "fo-cluster"})
	require.NoError(t, err)

	clusters, err := b.DescribeDBClusters("fo-cluster")
	require.NoError(t, err)
	require.Len(t, clusters[0].DBClusterMembers, 2)
	writerIdx := 0
	if clusters[0].DBClusterMembers[1].IsClusterWriter {
		writerIdx = 1
	}
	require.True(t, clusters[0].DBClusterMembers[writerIdx].IsClusterWriter)
	require.Equal(t, "fo-writer", clusters[0].DBClusterMembers[writerIdx].DBInstanceIdentifier)

	_, err = b.FailoverDBCluster("fo-cluster", "fo-reader")
	require.NoError(t, err)

	clusters, err = b.DescribeDBClusters("fo-cluster")
	require.NoError(t, err)
	for _, m := range clusters[0].DBClusterMembers {
		switch m.DBInstanceIdentifier {
		case "fo-reader":
			assert.True(t, m.IsClusterWriter, "target of FailoverDBCluster should be promoted to writer")
		case "fo-writer":
			assert.False(t, m.IsClusterWriter, "previous writer should no longer be writer after failover")
		}
	}
}
