package docdb_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/docdb"
)

func TestHandler_ClusterSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_snapshot",
			vals: url.Values{
				"Action":                      {"CreateDBClusterSnapshot"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
				"DBClusterIdentifier":         {"my-cluster"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-snap",
		},
		{
			name: "describe_snapshots_all",
			vals: url.Values{
				"Action":  {"DescribeDBClusterSnapshots"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeDBClusterSnapshotsResponse",
		},
		{
			name: "describe_snapshot_by_id",
			vals: url.Values{
				"Action":                      {"DescribeDBClusterSnapshots"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-snap",
		},
		{
			name: "delete_snapshot",
			vals: url.Values{
				"Action":                      {"DeleteDBClusterSnapshot"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DeleteDBClusterSnapshotResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":              {"CreateDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"my-cluster"},
				"Engine":              {"docdb"},
			})
			if tt.name != "create_snapshot" {
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterSnapshot"},
					"Version":                     {"2014-10-31"},
					"DBClusterSnapshotIdentifier": {"my-snap"},
					"DBClusterIdentifier":         {"my-cluster"},
				})
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestDescribeDBClusterSnapshots_NoFabricatedDBClusterArn confirms the
// snapshot wire response no longer includes a bare <DBClusterArn> element.
// The real types.DBClusterSnapshot has no such member (only
// DBClusterSnapshotArn) -- confirmed against
// awsAwsquery_deserializeDocumentDBClusterSnapshot,
// docdb@v1.51.4/deserializers.go. A real client's generated deserializer
// silently ignores unknown elements, so this is not independently
// observable via the typed SDK client; it can only be caught by inspecting
// the raw XML body directly, as this test does.
func TestDescribeDBClusterSnapshots_NoFabricatedDBClusterArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"arn-check-cluster"},
		"Engine":              {"docdb"},
	})
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterSnapshot"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"arn-check-snap"},
		"DBClusterIdentifier":         {"arn-check-cluster"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterSnapshots"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"arn-check-snap"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	body := rr.Body.String()
	assert.Contains(t, body, "<DBClusterSnapshotArn>", "the real member must still be present")
	assert.NotContains(t, body, "<DBClusterArn>",
		"types.DBClusterSnapshot has no DBClusterArn member; emitting one is fabricated wire content")
}

func TestSortedDescribeSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{
			name: "sorted_order",
			ids:  []string{"snap-z", "snap-a", "snap-m"},
			want: []string{"snap-a", "snap-m", "snap-z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := docdb.NewInMemoryBackend("000000000000", "us-east-1")
			for _, id := range tt.ids {
				b.AddDBClusterSnapshotInternal(&docdb.DBClusterSnapshot{DBClusterSnapshotIdentifier: id})
			}

			got, err := b.DescribeDBClusterSnapshots(context.Background(), "", "", "")
			require.NoError(t, err)

			gotIDs := make([]string, len(got))
			for i, s := range got {
				gotIDs[i] = s.DBClusterSnapshotIdentifier
			}

			assert.Equal(t, tt.want, gotIDs)
		})
	}
}

func TestSnapshotClusterIdFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		clusterID string
		wantCount int
	}{
		{
			name:      "filter_by_cluster",
			clusterID: "cluster-a",
			wantCount: 2,
		},
		{
			name:      "no_filter",
			clusterID: "",
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := docdb.NewInMemoryBackend("000000000000", "us-east-1")
			b.AddDBClusterSnapshotInternal(&docdb.DBClusterSnapshot{
				DBClusterSnapshotIdentifier: "snap-1",
				DBClusterIdentifier:         "cluster-a",
			})
			b.AddDBClusterSnapshotInternal(&docdb.DBClusterSnapshot{
				DBClusterSnapshotIdentifier: "snap-2",
				DBClusterIdentifier:         "cluster-a",
			})
			b.AddDBClusterSnapshotInternal(&docdb.DBClusterSnapshot{
				DBClusterSnapshotIdentifier: "snap-3",
				DBClusterIdentifier:         "cluster-b",
			})

			got, err := b.DescribeDBClusterSnapshots(context.Background(), "", tt.clusterID, "")
			require.NoError(t, err)

			assert.Len(t, got, tt.wantCount)
		})
	}
}

func TestHandler_SnapshotAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "describe_snapshot_attributes",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":              {"CreateDBCluster"},
					"Version":             {"2014-10-31"},
					"DBClusterIdentifier": {"my-cluster"},
					"Engine":              {"docdb"},
				})
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterSnapshot"},
					"Version":                     {"2014-10-31"},
					"DBClusterSnapshotIdentifier": {"my-snap"},
					"DBClusterIdentifier":         {"my-cluster"},
				})
			},
			vals: url.Values{
				"Action":                      {"DescribeDBClusterSnapshotAttributes"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeDBClusterSnapshotAttributesResponse",
		},
		{
			name: "describe_snapshot_attributes_not_found",
			vals: url.Values{
				"Action":                      {"DescribeDBClusterSnapshotAttributes"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"nonexistent"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBClusterSnapshotNotFoundFault",
		},
		{
			name: "modify_snapshot_attribute",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":              {"CreateDBCluster"},
					"Version":             {"2014-10-31"},
					"DBClusterIdentifier": {"my-cluster"},
					"Engine":              {"docdb"},
				})
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterSnapshot"},
					"Version":                     {"2014-10-31"},
					"DBClusterSnapshotIdentifier": {"my-snap"},
					"DBClusterIdentifier":         {"my-cluster"},
				})
			},
			vals: url.Values{
				"Action":                      {"ModifyDBClusterSnapshotAttribute"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
				"AttributeName":               {"restore"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "ModifyDBClusterSnapshotAttributeResponse",
		},
		{
			name: "modify_snapshot_attribute_not_found",
			vals: url.Values{
				"Action":                      {"ModifyDBClusterSnapshotAttribute"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"nonexistent"},
				"AttributeName":               {"restore"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBClusterSnapshotNotFoundFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestSnapshotAttributePersistence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "modify_and_describe_reflects_change",
			wantContains: "restore",
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":              {"CreateDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"my-cluster"},
			})
			doRequest(t, h, url.Values{
				"Action":                      {"CreateDBClusterSnapshot"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
				"DBClusterIdentifier":         {"my-cluster"},
			})
			modResp := doRequest(t, h, url.Values{
				"Action":                       {"ModifyDBClusterSnapshotAttribute"},
				"Version":                      {"2014-10-31"},
				"DBClusterSnapshotIdentifier":  {"my-snap"},
				"AttributeName":                {"restore"},
				"ValuesToAdd.AttributeValue.1": {"123456789012"},
			})
			require.Equal(t, http.StatusOK, modResp.Code)

			descResp := doRequest(t, h, url.Values{
				"Action":                      {"DescribeDBClusterSnapshotAttributes"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
			})
			assert.Equal(t, tt.wantStatus, descResp.Code)
			assert.Contains(t, descResp.Body.String(), tt.wantContains)
		})
	}
}

// TestDeleteDBClusterSnapshot_ClearsAttributes asserts that deleting a
// snapshot removes its restore-attribute grants, so a later snapshot
// recreated under the same identifier does not inherit a stale grant it was
// never given. snapshotAttributes is keyed by "region|DBClusterSnapshotIdentifier"
// (store_setup.go:snapshotAttributesKeyFn), independent of the clusterSnapshots
// table's own delete -- a ghost row left behind here is an access-control
// artefact (ModifyDBClusterSnapshotAttribute's "restore" attribute grants an
// AWS account ID cross-account restore access to the snapshot).
func TestDeleteDBClusterSnapshot_ClearsAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"my-cluster"},
	})
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterSnapshot"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"my-snap"},
		"DBClusterIdentifier":         {"my-cluster"},
	})
	modResp := doRequest(t, h, url.Values{
		"Action":                       {"ModifyDBClusterSnapshotAttribute"},
		"Version":                      {"2014-10-31"},
		"DBClusterSnapshotIdentifier":  {"my-snap"},
		"AttributeName":                {"restore"},
		"ValuesToAdd.AttributeValue.1": {"123456789012"},
	})
	require.Equal(t, http.StatusOK, modResp.Code)

	delResp := doRequest(t, h, url.Values{
		"Action":                      {"DeleteDBClusterSnapshot"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"my-snap"},
	})
	require.Equal(t, http.StatusOK, delResp.Code)

	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterSnapshot"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"my-snap"},
		"DBClusterIdentifier":         {"my-cluster"},
	})

	descResp := doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterSnapshotAttributes"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"my-snap"},
	})
	require.Equal(t, http.StatusOK, descResp.Code)
	assert.NotContains(t, descResp.Body.String(), "123456789012",
		"recreated snapshot must not inherit the deleted snapshot's restore grant")
}

func TestDescribeDBClusterSnapshotsByType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "filter_by_manual_snapshot_type",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":              {"CreateDBCluster"},
					"Version":             {"2014-10-31"},
					"DBClusterIdentifier": {"my-cluster"},
				})
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterSnapshot"},
					"Version":                     {"2014-10-31"},
					"DBClusterSnapshotIdentifier": {"my-snap"},
					"DBClusterIdentifier":         {"my-cluster"},
				})
			},
			vals: url.Values{
				"Action":       {"DescribeDBClusterSnapshots"},
				"Version":      {"2014-10-31"},
				"SnapshotType": {"manual"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-snap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestSnapshotHasSnapshotType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "snapshot_has_manual_type",
			wantContains: "manual",
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":              {"CreateDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"my-cluster"},
			})
			rr := doRequest(t, h, url.Values{
				"Action":                      {"CreateDBClusterSnapshot"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"my-snap"},
				"DBClusterIdentifier":         {"my-cluster"},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestCopySnapshotDuplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "duplicate_snapshot_copy_rejected",
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBClusterSnapshotAlreadyExistsFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":              {"CreateDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"src-cluster"},
			})
			doRequest(t, h, url.Values{
				"Action":                      {"CreateDBClusterSnapshot"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"src-snap"},
				"DBClusterIdentifier":         {"src-cluster"},
			})
			doRequest(t, h, url.Values{
				"Action":                            {"CopyDBClusterSnapshot"},
				"Version":                           {"2014-10-31"},
				"SourceDBClusterSnapshotIdentifier": {"src-snap"},
				"TargetDBClusterSnapshotIdentifier": {"dst-snap"},
			})
			rr := doRequest(t, h, url.Values{
				"Action":                            {"CopyDBClusterSnapshot"},
				"Version":                           {"2014-10-31"},
				"SourceDBClusterSnapshotIdentifier": {"src-snap"},
				"TargetDBClusterSnapshotIdentifier": {"dst-snap"},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestCopyCluster_SnapshotRetainsMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "copy_snapshot_retains_engine_version",
			wantContains: "4.0.0",
			wantStatus:   200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":              {"CreateDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"snap-src-cluster"},
				"EngineVersion":       {"4.0.0"},
			})
			doRequest(t, h, url.Values{
				"Action":                      {"CreateDBClusterSnapshot"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"src-snap"},
				"DBClusterIdentifier":         {"snap-src-cluster"},
			})
			rr := doRequest(t, h, url.Values{
				"Action":                            {"CopyDBClusterSnapshot"},
				"Version":                           {"2014-10-31"},
				"SourceDBClusterSnapshotIdentifier": {"src-snap"},
				"TargetDBClusterSnapshotIdentifier": {"dst-snap"},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestSnapshotAttributes_AddRemove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		operation    string
		attrName     string
		wantContains string
		valuesToAdd  []string
		valuesToRm   []string
		wantStatus   int
	}{
		{
			name:         "add_restore_attribute",
			operation:    "ModifyDBClusterSnapshotAttribute",
			attrName:     "restore",
			valuesToAdd:  []string{"111111111111"},
			wantContains: "111111111111",
			wantStatus:   200,
		},
		{
			name:         "remove_restore_attribute",
			operation:    "ModifyDBClusterSnapshotAttribute",
			attrName:     "restore",
			valuesToAdd:  []string{"111111111111"},
			valuesToRm:   []string{"111111111111"},
			wantContains: "ModifyDBClusterSnapshotAttributeResponse",
			wantStatus:   200,
		},
		{
			name:         "describe_snapshot_attributes",
			operation:    "DescribeDBClusterSnapshotAttributes",
			attrName:     "",
			wantContains: "DescribeDBClusterSnapshotAttributesResponse",
			wantStatus:   200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":              {"CreateDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"attr-cluster"},
			})
			doRequest(t, h, url.Values{
				"Action":                      {"CreateDBClusterSnapshot"},
				"Version":                     {"2014-10-31"},
				"DBClusterSnapshotIdentifier": {"attr-snap"},
				"DBClusterIdentifier":         {"attr-cluster"},
			})

			if tt.operation == "ModifyDBClusterSnapshotAttribute" {
				vals := url.Values{
					"Action":                      {"ModifyDBClusterSnapshotAttribute"},
					"Version":                     {"2014-10-31"},
					"DBClusterSnapshotIdentifier": {"attr-snap"},
					"AttributeName":               {tt.attrName},
				}
				for i, v := range tt.valuesToAdd {
					vals.Set(fmt.Sprintf("ValuesToAdd.AttributeValue.%d", i+1), v)
				}
				for i, v := range tt.valuesToRm {
					vals.Set(fmt.Sprintf("ValuesToRemove.AttributeValue.%d", i+1), v)
				}
				rr := doRequest(t, h, vals)
				assert.Equal(t, tt.wantStatus, rr.Code)
				assert.Contains(t, rr.Body.String(), tt.wantContains)
			} else {
				rr := doRequest(t, h, url.Values{
					"Action":                      {"DescribeDBClusterSnapshotAttributes"},
					"Version":                     {"2014-10-31"},
					"DBClusterSnapshotIdentifier": {"attr-snap"},
				})
				assert.Equal(t, tt.wantStatus, rr.Code)
				assert.Contains(t, rr.Body.String(), tt.wantContains)
			}
		})
	}
}

func TestDescribeDBClusterSnapshots_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxRecords  string
		wantCount   int
		wantHasMore bool
	}{
		{
			name:        "no_limit_returns_all",
			wantCount:   3,
			wantHasMore: false,
		},
		{
			name:        "limit_to_1",
			maxRecords:  "1",
			wantCount:   1,
			wantHasMore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			b2CreateCluster(t, h, "snap-cluster")
			for i := range 3 {
				b2CreateSnapshot(t, h, fmt.Sprintf("snap-%d", i), "snap-cluster")
			}

			vals := url.Values{
				"Action":  {"DescribeDBClusterSnapshots"},
				"Version": {"2014-10-31"},
			}
			if tt.maxRecords != "" {
				vals.Set("MaxRecords", tt.maxRecords)
			}
			rr := doRequest(t, h, vals)
			require.Equal(t, http.StatusOK, rr.Code)
			body := rr.Body.String()
			count := strings.Count(body, "<DBClusterSnapshotIdentifier>")
			assert.Equal(t, tt.wantCount, count)
			if tt.wantHasMore {
				assert.Contains(t, body, "<Marker>")
			} else {
				assert.NotContains(t, body, "<Marker>")
			}
		})
	}
}

func TestCopyDBClusterSnapshot_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		source       string
		target       string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "not_found_source_returns_error",
			source:       "no-such-snap",
			target:       "new-snap",
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBClusterSnapshotNotFoundFault",
		},
		{
			name:         "duplicate_target_returns_error",
			source:       "src-snap",
			target:       "dst-snap",
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBClusterSnapshotAlreadyExistsFault",
		},
		{
			name:         "valid_copy_succeeds",
			source:       "src-snap2",
			target:       "dst-snap2",
			wantStatus:   http.StatusOK,
			wantContains: "CopyDBClusterSnapshotResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			b2CreateCluster(t, h, "copy-snap-cluster")

			// Create source snapshots where needed.
			if tt.source == "src-snap" || tt.source == "src-snap2" {
				b2CreateSnapshot(t, h, tt.source, "copy-snap-cluster")
			}
			// Create target snapshot to trigger duplicate error.
			if tt.name == "duplicate_target_returns_error" {
				b2CreateSnapshot(t, h, tt.target, "copy-snap-cluster")
			}

			rr := doRequest(t, h, url.Values{
				"Action":                            {"CopyDBClusterSnapshot"},
				"Version":                           {"2014-10-31"},
				"SourceDBClusterSnapshotIdentifier": {tt.source},
				"TargetDBClusterSnapshotIdentifier": {tt.target},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestCreateDBClusterSnapshot_ClusterNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rr := doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterSnapshot"},
		"Version":                     {"2014-10-31"},
		"DBClusterSnapshotIdentifier": {"orphan-snap"},
		"DBClusterIdentifier":         {"no-such-cluster"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBClusterNotFoundFault")
}

func TestCopyDBClusterSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "copy_snapshot",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":              {"CreateDBCluster"},
					"Version":             {"2014-10-31"},
					"DBClusterIdentifier": {"my-cluster"},
					"Engine":              {"docdb"},
				})
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterSnapshot"},
					"Version":                     {"2014-10-31"},
					"DBClusterSnapshotIdentifier": {"source-snap"},
					"DBClusterIdentifier":         {"my-cluster"},
				})
			},
			vals: url.Values{
				"Action":                            {"CopyDBClusterSnapshot"},
				"Version":                           {"2014-10-31"},
				"SourceDBClusterSnapshotIdentifier": {"source-snap"},
				"TargetDBClusterSnapshotIdentifier": {"target-snap"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "target-snap",
		},
		{
			name: "copy_snapshot_source_not_found",
			vals: url.Values{
				"Action":                            {"CopyDBClusterSnapshot"},
				"Version":                           {"2014-10-31"},
				"SourceDBClusterSnapshotIdentifier": {"nonexistent"},
				"TargetDBClusterSnapshotIdentifier": {"target-snap"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBClusterSnapshotNotFoundFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}
