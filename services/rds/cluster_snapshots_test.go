package rds_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

func TestClusterSnapshot_CRUD(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()

	_, err := b.CreateDBCluster(
		"cluster-a",
		"aurora-postgresql",
		"admin",
		"",
		"",
		5432,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	snap, err := b.CreateDBClusterSnapshot("snap-a", "cluster-a")
	require.NoError(t, err)
	assert.Equal(t, "snap-a", snap.DBClusterSnapshotIdentifier)
	assert.Equal(t, "aurora-postgresql", snap.Engine)

	snaps, err := b.DescribeDBClusterSnapshots("snap-a", "")
	require.NoError(t, err)
	require.Len(t, snaps, 1)

	_, err = b.DeleteDBClusterSnapshot("snap-a")
	require.NoError(t, err)

	_, err = b.DescribeDBClusterSnapshots("snap-a", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrClusterSnapshotNotFound)
}

func TestClusterSnapshot_Copy(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBCluster(
		"cluster-b",
		"aurora-mysql",
		"admin",
		"",
		"",
		3306,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	_, err = b.CreateDBClusterSnapshot("src-snap", "cluster-b")
	require.NoError(t, err)

	dst, err := b.CopyDBClusterSnapshot("src-snap", "dst-snap")
	require.NoError(t, err)
	assert.Equal(t, "dst-snap", dst.DBClusterSnapshotIdentifier)

	snaps, err := b.DescribeDBClusterSnapshots("", "")
	require.NoError(t, err)
	assert.Len(t, snaps, 2)
}

// TestClusterSnapshot_Duplicate covers the exact-duplicate-identifier case,
// plus (DBClusterSnapshotIdentifier is a case-insensitive AWS identifier,
// matching real AWS, which lower-cases identifiers internally) the
// case-varying duplicate collisions and the corresponding case-insensitive
// Describe/Delete lookups.
func TestClusterSnapshot_Duplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		setupID   string
		actionID  string
		action    string
		wantErr   bool
	}{
		{
			name:      "exact_duplicate_collides",
			setupID:   "dup-snap",
			actionID:  "dup-snap",
			action:    "create",
			wantErr:   true,
			wantErrIs: rds.ErrClusterSnapshotAlreadyExists,
		},
		{
			name:      "lowercased_duplicate_collides",
			setupID:   "MyClusterSnap",
			actionID:  "myclustersnap",
			action:    "create",
			wantErr:   true,
			wantErrIs: rds.ErrClusterSnapshotAlreadyExists,
		},
		{
			name:      "uppercased_duplicate_collides",
			setupID:   "lower-cluster-snap",
			actionID:  "LOWER-CLUSTER-SNAP",
			action:    "create",
			wantErr:   true,
			wantErrIs: rds.ErrClusterSnapshotAlreadyExists,
		},
		{
			name:     "distinct_id_does_not_collide",
			setupID:  "some-cluster-snap",
			actionID: "some-other-cluster-snap",
			action:   "create",
		},
		{
			name:     "describe_finds_resource_under_different_case",
			setupID:  "FindMe-ClusterSnap",
			actionID: "findme-clustersnap",
			action:   "describe",
		},
		{
			name:     "delete_removes_resource_under_different_case",
			setupID:  "DeleteMe-ClusterSnap",
			actionID: "deleteme-clustersnap",
			action:   "delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBatch2Backend()
			_, err := b.CreateDBCluster(
				"cluster-c-"+tt.name,
				"aurora-postgresql",
				"admin",
				"",
				"",
				5432,
				nil,
				rds.DBClusterOptions{},
			)
			require.NoError(t, err)

			_, err = b.CreateDBClusterSnapshot(tt.setupID, "cluster-c-"+tt.name)
			require.NoError(t, err)

			switch tt.action {
			case "create":
				_, err = b.CreateDBClusterSnapshot(tt.actionID, "cluster-c-"+tt.name)
			case "describe":
				var snaps []rds.DBClusterSnapshot
				snaps, err = b.DescribeDBClusterSnapshots(tt.actionID, "")
				if err == nil {
					require.Len(t, snaps, 1)
					// The wire response must keep echoing the ORIGINAL
					// caller-supplied casing from creation time.
					assert.Equal(t, tt.setupID, snaps[0].DBClusterSnapshotIdentifier)
				}
			case "delete":
				_, err = b.DeleteDBClusterSnapshot(tt.actionID)
				if err == nil {
					_, describeErr := b.DescribeDBClusterSnapshots(tt.setupID, "")
					require.ErrorIs(t, describeErr, rds.ErrClusterSnapshotNotFound)
				}
			}

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestClusterSnapshot_DeleteNotFound(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.DeleteDBClusterSnapshot("noexist")
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrClusterSnapshotNotFound)
}

func TestClusterSnapshot_HTTP(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()

	rec := postRDSForm(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"cluster-snap-test"},
		"Engine":              {"aurora-postgresql"},
		"MasterUsername":      {"admin"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":                      {"CreateDBClusterSnapshot"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"http-cluster-snap"},
		"DBClusterIdentifier":         {"cluster-snap-test"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "http-cluster-snap")

	rec = postRDSForm(t, h, url.Values{
		"Action":                      {"DescribeDBClusterSnapshots"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"http-cluster-snap"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":                            {"CopyDBClusterSnapshot"},
		"Version":                           {"2014-10-31"},
		"SourceDBClusterSnapshotIdentifier": {"http-cluster-snap"},
		"TargetDBClusterSnapshotIdentifier": {"http-cluster-snap-copy"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":                      {"DeleteDBClusterSnapshot"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"http-cluster-snap"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestConcurrent_ClusterSnapshot(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBCluster(
		"conc-cluster",
		"aurora-postgresql",
		"admin",
		"",
		"",
		5432,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			snapID := fmt.Sprintf("conc-snap-%d", n)
			_, _ = b.CreateDBClusterSnapshot(snapID, "conc-cluster")
			_, _ = b.DescribeDBClusterSnapshots(snapID, "")
		}(i)
	}
	wg.Wait()

	snaps, err := b.DescribeDBClusterSnapshots("", "")
	require.NoError(t, err)
	assert.Len(t, snaps, 10)
}

func TestDescribeDBClusterSnapshots_FilterByCluster_ReturnsMatching(t *testing.T) {
	t.Parallel()

	h := newAccOps2Handler(t)
	mustCreateAccOps2Cluster(t, h, "cluster-1")
	mustCreateAccOps2Cluster(t, h, "cluster-2")

	for _, pair := range [][2]string{
		{"csnap-1a", "cluster-1"},
		{"csnap-1b", "cluster-1"},
		{"csnap-2a", "cluster-2"},
	} {
		rec := doAccOps2(t, h, url.Values{
			"Action": {"CreateDBClusterSnapshot"}, "Version": {"2014-10-31"},
			"DBClusterSnapshotIdentifier": {pair[0]},
			"DBClusterIdentifier":         {pair[1]},
		}.Encode())
		require.Equal(t, http.StatusOK, rec.Code, "create cluster snapshot %s: %s", pair[0], rec.Body)
	}

	rec := doAccOps2(t, h, url.Values{
		"Action":              {"DescribeDBClusterSnapshots"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"cluster-1"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			Snapshots []struct {
				DBClusterSnapshotIdentifier string `xml:"DBClusterSnapshotIdentifier"`
				DBClusterIdentifier         string `xml:"DBClusterIdentifier"`
			} `xml:"DBClusterSnapshots>DBClusterSnapshot"`
		} `xml:"DescribeDBClusterSnapshotsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	ids := make([]string, 0, len(resp.Result.Snapshots))
	for _, s := range resp.Result.Snapshots {
		assert.Equal(t, "cluster-1", s.DBClusterIdentifier)
		ids = append(ids, s.DBClusterSnapshotIdentifier)
	}
	assert.ElementsMatch(t, []string{"csnap-1a", "csnap-1b"}, ids)
}

func TestDescribeDBClusterSnapshots_FilterByCluster_NoMatch_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newAccOps2Handler(t)
	mustCreateAccOps2Cluster(t, h, "cluster-x")
	doAccOps2(t, h, url.Values{
		"Action": {"CreateDBClusterSnapshot"}, "Version": {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"csnap-x"},
		"DBClusterIdentifier":         {"cluster-x"},
	}.Encode())

	rec := doAccOps2(t, h, url.Values{
		"Action":              {"DescribeDBClusterSnapshots"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"no-such-cluster"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			Snapshots []struct {
				DBClusterSnapshotIdentifier string `xml:"DBClusterSnapshotIdentifier"`
			} `xml:"DBClusterSnapshots>DBClusterSnapshot"`
		} `xml:"DescribeDBClusterSnapshotsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Result.Snapshots)
}

func TestDescribeDBClusterSnapshots_NoFilter_ReturnsAll(t *testing.T) {
	t.Parallel()

	h := newAccOps2Handler(t)
	mustCreateAccOps2Cluster(t, h, "cluster-m")
	mustCreateAccOps2Cluster(t, h, "cluster-n")

	for _, pair := range [][2]string{
		{"csnap-m1", "cluster-m"}, {"csnap-n1", "cluster-n"}, {"csnap-n2", "cluster-n"},
	} {
		doAccOps2(t, h, url.Values{
			"Action": {"CreateDBClusterSnapshot"}, "Version": {"2014-10-31"},
			"DBClusterSnapshotIdentifier": {pair[0]},
			"DBClusterIdentifier":         {pair[1]},
		}.Encode())
	}

	rec := doAccOps2(t, h, "Action=DescribeDBClusterSnapshots&Version=2014-10-31")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Result struct {
			Snapshots []struct {
				DBClusterSnapshotIdentifier string `xml:"DBClusterSnapshotIdentifier"`
			} `xml:"DBClusterSnapshots>DBClusterSnapshot"`
		} `xml:"DescribeDBClusterSnapshotsResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Result.Snapshots, 3)
}

func TestDescribeDBClusterSnapshotAttributes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs  error
		name       string
		snapshotID string
		wantErr    bool
	}{
		{name: "success returns empty attrs", snapshotID: "cluster-snap-1"},
		{name: "not found", snapshotID: "missing", wantErr: true, wantErrIs: rds.ErrClusterSnapshotNotFound},
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
				_, err = b.CreateDBClusterSnapshot(tt.snapshotID, "my-cluster")
				require.NoError(t, err)
			}
			got, err := b.DescribeDBClusterSnapshotAttributes(tt.snapshotID)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.snapshotID, got.DBClusterSnapshotIdentifier)
		})
	}
}

func TestModifyDBClusterSnapshotAttribute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErrIs      error
		name           string
		snapshotID     string
		attributeName  string
		valuesToAdd    []string
		valuesToRemove []string
		wantCount      int
		wantErr        bool
	}{
		{
			name:          "add values",
			snapshotID:    "cluster-snap-1",
			attributeName: "restore",
			valuesToAdd:   []string{"123456789012"},
			wantCount:     1,
		},
		{
			name:          "not found",
			snapshotID:    "missing",
			attributeName: "restore",
			wantErr:       true,
			wantErrIs:     rds.ErrClusterSnapshotNotFound,
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
				_, err = b.CreateDBClusterSnapshot(tt.snapshotID, "my-cluster")
				require.NoError(t, err)
			}
			got, err := b.ModifyDBClusterSnapshotAttribute(
				tt.snapshotID,
				tt.attributeName,
				tt.valuesToAdd,
				tt.valuesToRemove,
			)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErrIs)

				return
			}
			require.NoError(t, err)
			require.Len(t, got.DBClusterSnapshotAttributes, 1)
			assert.Equal(t, tt.attributeName, got.DBClusterSnapshotAttributes[0].AttributeName)
			assert.Len(t, got.DBClusterSnapshotAttributes[0].AttributeValues, tt.wantCount)
		})
	}
}

// TestClusterSnapshotCreateTime verifies new fields on CreateDBClusterSnapshot.
func TestClusterSnapshotCreateTime(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	_, err := b.CreateDBCluster("cluster-snap", "aurora-postgresql", "admin", "", "", 0, nil,
		rds.DBClusterOptions{EngineVersion: "15.3", StorageEncrypted: true})
	require.NoError(t, err)
	snap, err := b.CreateDBClusterSnapshot("cluster-snap-1", "cluster-snap")
	require.NoError(t, err)
	assert.False(t, snap.SnapshotCreateTime.IsZero(), "SnapshotCreateTime should be set")
	assert.Equal(t, 100, snap.PercentProgress)
	assert.Equal(t, "15.3", snap.EngineVersion, "EngineVersion should be copied from cluster")
	assert.True(t, snap.StorageEncrypted, "StorageEncrypted should be copied from cluster")
}

// TestDescribeDBClusterSnapshotsSorted verifies deterministic sort order.
func TestDescribeDBClusterSnapshotsSorted(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	_, err := b.CreateDBCluster("cluster-sort", "aurora-postgresql", "admin", "", "", 0, nil, rds.DBClusterOptions{})
	require.NoError(t, err)
	for _, id := range []string{"csnap-z", "csnap-a", "csnap-m"} {
		_, snapErr := b.CreateDBClusterSnapshot(id, "cluster-sort")
		require.NoError(t, snapErr)
	}
	got, err := b.DescribeDBClusterSnapshots("", "")
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "csnap-a", got[0].DBClusterSnapshotIdentifier)
	assert.Equal(t, "csnap-m", got[1].DBClusterSnapshotIdentifier)
	assert.Equal(t, "csnap-z", got[2].DBClusterSnapshotIdentifier)
}
