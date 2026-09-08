package docdb_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/docdb"
)

// TestHandler_DeleteGlobalCluster_HasMembers asserts DeleteGlobalCluster
// rejects a global cluster that still has an attached member cluster
// (api_op_DeleteGlobalCluster.go: "The primary and secondary clusters must
// already be detached or deleted before attempting to delete a global
// cluster.").
func TestHandler_DeleteGlobalCluster_HasMembers(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"gc-member-cluster"},
		"Engine":              {"docdb"},
		"MasterUsername":      {"admin"},
	})
	doRequest(t, h, url.Values{
		"Action":                    {"CreateGlobalCluster"},
		"Version":                   {"2014-10-31"},
		"GlobalClusterIdentifier":   {"gc-with-member"},
		"SourceDBClusterIdentifier": {"gc-member-cluster"},
	})

	recEarly := doRequest(t, h, url.Values{
		"Action":                  {"DeleteGlobalCluster"},
		"Version":                 {"2014-10-31"},
		"GlobalClusterIdentifier": {"gc-with-member"},
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)
	assert.Contains(t, recEarly.Body.String(), "InvalidGlobalClusterStateFault")

	recRemove := doRequest(t, h, url.Values{
		"Action":                  {"RemoveFromGlobalCluster"},
		"Version":                 {"2014-10-31"},
		"GlobalClusterIdentifier": {"gc-with-member"},
		"DbClusterIdentifier":     {"gc-member-cluster"},
	})
	require.Equal(t, http.StatusOK, recRemove.Code)

	recDelete := doRequest(t, h, url.Values{
		"Action":                  {"DeleteGlobalCluster"},
		"Version":                 {"2014-10-31"},
		"GlobalClusterIdentifier": {"gc-with-member"},
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_GlobalClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_global_cluster",
			vals: url.Values{
				"Action":                  {"CreateGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"my-global"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-global",
		},
		{
			name: "delete_global_cluster",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"my-global"},
				})
			},
			vals: url.Values{
				"Action":                  {"DeleteGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"my-global"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DeleteGlobalClusterResponse",
		},
		{
			name: "create_duplicate_global_cluster",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"dup-global"},
				})
			},
			vals: url.Values{
				"Action":                  {"CreateGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"dup-global"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "GlobalClusterAlreadyExistsFault",
		},
		{
			name: "delete_nonexistent_global_cluster",
			vals: url.Values{
				"Action":                  {"DeleteGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"nonexistent"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "GlobalClusterNotFoundFault",
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

func TestSortedDescribeGlobalClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{
			name: "sorted_order",
			ids:  []string{"gc-z", "gc-a", "gc-m"},
			want: []string{"gc-a", "gc-m", "gc-z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := docdb.NewInMemoryBackend("000000000000", "us-east-1")
			for _, id := range tt.ids {
				b.AddGlobalClusterInternal(&docdb.GlobalCluster{GlobalClusterIdentifier: id})
			}

			got := b.DescribeGlobalClusters(context.Background(), "")

			gotIDs := make([]string, len(got))
			for i, gc := range got {
				gotIDs[i] = gc.GlobalClusterIdentifier
			}

			assert.Equal(t, tt.want, gotIDs)
		})
	}
}

func TestDescribeGlobalClusters_RealData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filterID  string
		gcIDs     []string
		wantCount int
	}{
		{
			name:      "all_clusters",
			gcIDs:     []string{"gc-1", "gc-2"},
			filterID:  "",
			wantCount: 2,
		},
		{
			name:      "filtered_by_id",
			gcIDs:     []string{"gc-1", "gc-2"},
			filterID:  "gc-1",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			for _, id := range tt.gcIDs {
				h.Backend.AddGlobalClusterInternal(&docdb.GlobalCluster{
					GlobalClusterIdentifier: id,
					Status:                  "available",
				})
			}

			vals := url.Values{
				"Action":  []string{"DescribeGlobalClusters"},
				"Version": []string{"2014-10-31"},
			}
			if tt.filterID != "" {
				vals.Set("GlobalClusterIdentifier", tt.filterID)
			}

			resp := doRequest(t, h, vals)
			require.Equal(t, http.StatusOK, resp.Code)

			body := resp.Body.String()
			for _, id := range tt.gcIDs[:tt.wantCount] {
				assert.Contains(t, body, id)
			}
		})
	}
}

// TestCreateGlobalCluster_NoFabricatedSourceDBClusterIdentifier confirms the
// GlobalCluster wire response no longer includes a bare
// <SourceDBClusterIdentifier> element. That name is a real
// CreateGlobalClusterInput REQUEST member only -- the real response type
// types.GlobalCluster has no such member (confirmed against
// awsAwsquery_deserializeDocumentGlobalCluster, docdb@v1.51.4/deserializers.go).
// A real client's generated deserializer silently ignores unknown elements,
// so this is not independently observable via the typed SDK client; it can
// only be caught by inspecting the raw XML body directly, as this test does.
func TestCreateGlobalCluster_NoFabricatedSourceDBClusterIdentifier(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"gc-source-check"},
		"Engine":              {"docdb"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                    {"CreateGlobalCluster"},
		"Version":                   {"2014-10-31"},
		"GlobalClusterIdentifier":   {"gc-arn-check"},
		"SourceDBClusterIdentifier": {"gc-source-check"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	body := rr.Body.String()
	assert.Contains(t, body, "<GlobalClusterIdentifier>gc-arn-check</GlobalClusterIdentifier>")
	assert.NotContains(t, body, "<SourceDBClusterIdentifier>",
		"types.GlobalCluster has no SourceDBClusterIdentifier member; emitting one is fabricated wire content")
}

func TestHandler_GlobalClusterMutations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "modify_global_cluster",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"my-global"},
				})
			},
			vals: url.Values{
				"Action":                  {"ModifyGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"my-global"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-global",
		},
		{
			name: "failover_global_cluster",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"my-global"},
				})
			},
			vals: url.Values{
				"Action":                    {"FailoverGlobalCluster"},
				"Version":                   {"2014-10-31"},
				"GlobalClusterIdentifier":   {"my-global"},
				"TargetDbClusterIdentifier": {"secondary-cluster"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "failing-over",
		},
		{
			name: "switchover_global_cluster",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"my-global"},
				})
			},
			vals: url.Values{
				"Action":                    {"SwitchoverGlobalCluster"},
				"Version":                   {"2014-10-31"},
				"GlobalClusterIdentifier":   {"my-global"},
				"TargetDbClusterIdentifier": {"secondary-cluster"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "switching-over",
		},
		{
			name: "remove_from_global_cluster",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"my-global"},
				})
			},
			vals: url.Values{
				"Action":                  {"RemoveFromGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"my-global"},
				"DbClusterIdentifier":     {"secondary-cluster"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "RemoveFromGlobalClusterResponse",
		},
		{
			name: "modify_global_cluster_not_found",
			vals: url.Values{
				"Action":                  {"ModifyGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"nonexistent"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "GlobalClusterNotFoundFault",
		},
		{
			name: "failover_global_cluster_not_found",
			vals: url.Values{
				"Action":                    {"FailoverGlobalCluster"},
				"Version":                   {"2014-10-31"},
				"GlobalClusterIdentifier":   {"nonexistent"},
				"TargetDbClusterIdentifier": {"secondary-cluster"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "GlobalClusterNotFoundFault",
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

func TestModifyGlobalClusterRename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*testing.T, *docdb.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "rename_global_cluster",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"old-global"},
				})
			},
			vals: url.Values{
				"Action":                     {"ModifyGlobalCluster"},
				"Version":                    {"2014-10-31"},
				"GlobalClusterIdentifier":    {"old-global"},
				"NewGlobalClusterIdentifier": {"new-global"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "new-global",
		},
		{
			name: "modify_global_cluster_deletion_protection",
			setup: func(t *testing.T, h *docdb.Handler) {
				t.Helper()
				doRequest(t, h, url.Values{
					"Action":                  {"CreateGlobalCluster"},
					"Version":                 {"2014-10-31"},
					"GlobalClusterIdentifier": {"my-global"},
				})
			},
			vals: url.Values{
				"Action":                  {"ModifyGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"my-global"},
				"DeletionProtection":      {"true"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "my-global",
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

func TestGlobalClusterWithEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_global_cluster_with_engine",
			vals: url.Values{
				"Action":                  {"CreateGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"my-global"},
				"Engine":                  {"docdb"},
				"EngineVersion":           {"5.0.0"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "5.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestGlobalCluster_FailoverSwitchover(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		action       string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "failover_sets_failing_over_status",
			action:       "FailoverGlobalCluster",
			wantStatus:   200,
			wantContains: "failing-over",
		},
		{
			name:         "switchover_sets_switching_over_status",
			action:       "SwitchoverGlobalCluster",
			wantStatus:   200,
			wantContains: "switching-over",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":                  {"CreateGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"fo-gc"},
			})
			rr := doRequest(t, h, url.Values{
				"Action":                    {tt.action},
				"Version":                   {"2014-10-31"},
				"GlobalClusterIdentifier":   {"fo-gc"},
				"TargetDbClusterIdentifier": {"some-cluster"},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestGlobalCluster_ModifyRenameAndDeletionProtection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		newID              string
		deletionProtection string
		wantContains       string
		wantStatus         int
	}{
		{
			name:         "rename_global_cluster",
			newID:        "renamed-gc",
			wantContains: "renamed-gc",
			wantStatus:   200,
		},
		{
			name:               "set_deletion_protection",
			newID:              "",
			deletionProtection: "true",
			wantContains:       "ModifyGlobalClusterResponse",
			wantStatus:         200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":                  {"CreateGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"mod-gc"},
			})
			vals := url.Values{
				"Action":                  {"ModifyGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"mod-gc"},
			}
			if tt.newID != "" {
				vals.Set("NewGlobalClusterIdentifier", tt.newID)
			}
			if tt.deletionProtection != "" {
				vals.Set("DeletionProtection", tt.deletionProtection)
			}
			rr := doRequest(t, h, vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestDescribeGlobalClusters_Filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filterID     string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "no_filter_returns_all",
			wantStatus:   http.StatusOK,
			wantContains: "DescribeGlobalClustersResponse",
		},
		{
			name:         "filter_by_known_id",
			filterID:     "global-a",
			wantStatus:   http.StatusOK,
			wantContains: "global-a",
		},
		{
			name:         "filter_by_unknown_id_returns_empty",
			filterID:     "no-such-global",
			wantStatus:   http.StatusOK,
			wantContains: "DescribeGlobalClustersResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":                  {"CreateGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"global-a"},
			})
			doRequest(t, h, url.Values{
				"Action":                  {"CreateGlobalCluster"},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"global-b"},
			})

			vals := url.Values{
				"Action":  {"DescribeGlobalClusters"},
				"Version": {"2014-10-31"},
			}
			if tt.filterID != "" {
				vals.Set("GlobalClusterIdentifier", tt.filterID)
			}
			rr := doRequest(t, h, vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
			if tt.filterID == "global-a" {
				assert.NotContains(t, rr.Body.String(), "global-b")
			}
		})
	}
}

func TestCreateGlobalCluster_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rr := doRequest(t, h, url.Values{
		"Action":                  {"CreateGlobalCluster"},
		"Version":                 {"2014-10-31"},
		"GlobalClusterIdentifier": {"dup-global"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	rr = doRequest(t, h, url.Values{
		"Action":                  {"CreateGlobalCluster"},
		"Version":                 {"2014-10-31"},
		"GlobalClusterIdentifier": {"dup-global"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "GlobalClusterAlreadyExistsFault")
}

// ---- Global cluster not-found paths ----

func TestGlobalCluster_NotFound_Paths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		action       string
		wantContains string
	}{
		{
			name:         "RemoveFromGlobalCluster_not_found",
			action:       "RemoveFromGlobalCluster",
			wantContains: "GlobalClusterNotFoundFault",
		},
		{
			name:         "FailoverGlobalCluster_not_found",
			action:       "FailoverGlobalCluster",
			wantContains: "GlobalClusterNotFoundFault",
		},
		{
			name:         "SwitchoverGlobalCluster_not_found",
			action:       "SwitchoverGlobalCluster",
			wantContains: "GlobalClusterNotFoundFault",
		},
		{
			name:         "DeleteGlobalCluster_not_found",
			action:       "DeleteGlobalCluster",
			wantContains: "GlobalClusterNotFoundFault",
		},
		{
			name:         "ModifyGlobalCluster_not_found",
			action:       "ModifyGlobalCluster",
			wantContains: "GlobalClusterNotFoundFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			vals := url.Values{
				"Action":                  {tt.action},
				"Version":                 {"2014-10-31"},
				"GlobalClusterIdentifier": {"no-such-global"},
			}
			if tt.action == "RemoveFromGlobalCluster" {
				vals.Set("DbClusterIdentifier", "dummy-cluster")
			}
			if tt.action == "FailoverGlobalCluster" || tt.action == "SwitchoverGlobalCluster" {
				vals.Set("TargetDbClusterIdentifier", "dummy-cluster")
			}
			rr := doRequest(t, h, vals)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestGlobalCluster_MemberTracking locks in the fix for the
// "GlobalCluster has no GlobalClusterMembers subresource" gap identified in
// PARITY.md: CreateGlobalCluster previously never added the source cluster
// as a member, DescribeGlobalClusters always reported an empty member list,
// and RemoveFromGlobalCluster was a pure no-op with respect to membership.
func TestGlobalCluster_MemberTracking(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"member-src-cluster"},
	})

	// CreateGlobalCluster with a resolvable SourceDBClusterIdentifier must
	// add that cluster as the initial writer member -- not an empty list.
	createRR := doRequest(t, h, url.Values{
		"Action":                    {"CreateGlobalCluster"},
		"Version":                   {"2014-10-31"},
		"GlobalClusterIdentifier":   {"member-gc"},
		"SourceDBClusterIdentifier": {"member-src-cluster"},
	})
	require.Equal(t, http.StatusOK, createRR.Code)
	assert.Contains(t, createRR.Body.String(), "<GlobalClusterMember>")
	assert.Contains(t, createRR.Body.String(), "member-src-cluster")
	assert.Contains(t, createRR.Body.String(), "<IsWriter>true</IsWriter>")

	// DescribeGlobalClusters must reflect the same real member, not an
	// always-empty GlobalClusterMembers list.
	describeRR := doRequest(t, h, url.Values{
		"Action":                  {"DescribeGlobalClusters"},
		"Version":                 {"2014-10-31"},
		"GlobalClusterIdentifier": {"member-gc"},
	})
	require.Equal(t, http.StatusOK, describeRR.Code)
	assert.Contains(t, describeRR.Body.String(), "<GlobalClusterMember>")
	assert.Contains(t, describeRR.Body.String(), "member-src-cluster")

	// FailoverGlobalCluster promoting a second real cluster must attach it
	// as a new writer member and demote the prior writer, not silently no-op.
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"member-target-cluster"},
	})
	failoverRR := doRequest(t, h, url.Values{
		"Action":                    {"FailoverGlobalCluster"},
		"Version":                   {"2014-10-31"},
		"GlobalClusterIdentifier":   {"member-gc"},
		"TargetDbClusterIdentifier": {"member-target-cluster"},
	})
	require.Equal(t, http.StatusOK, failoverRR.Code)
	body := failoverRR.Body.String()
	assert.Contains(t, body, "member-target-cluster")
	// Two members now: the original source (demoted) and the newly
	// promoted target (writer).
	assert.Equal(t, 2, strings.Count(body, "<GlobalClusterMember>"))

	// RemoveFromGlobalCluster must genuinely delete the matching member,
	// not leave the member list unchanged.
	removeRR := doRequest(t, h, url.Values{
		"Action":                  {"RemoveFromGlobalCluster"},
		"Version":                 {"2014-10-31"},
		"GlobalClusterIdentifier": {"member-gc"},
		"DbClusterIdentifier":     {"member-target-cluster"},
	})
	require.Equal(t, http.StatusOK, removeRR.Code)
	assert.NotContains(t, removeRR.Body.String(), "member-target-cluster")
	assert.Equal(t, 1, strings.Count(removeRR.Body.String(), "<GlobalClusterMember>"))
}

// TestGlobalCluster_MemberTracking_UnresolvableSourceIsEmpty locks in that
// an unresolvable SourceDBClusterIdentifier (no matching cluster in this
// account/region) leaves GlobalClusterMembers empty rather than fabricating
// a member entry -- matching this backend's existing leniency for a target
// it cannot validate, while still doing real work for a target it can.
func TestGlobalCluster_MemberTracking_UnresolvableSourceIsEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                    {"CreateGlobalCluster"},
		"Version":                   {"2014-10-31"},
		"GlobalClusterIdentifier":   {"unresolved-gc"},
		"SourceDBClusterIdentifier": {"does-not-exist"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.NotContains(t, rr.Body.String(), "<GlobalClusterMember>")
}
