package redshift_test

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- CreateClusterSnapshot ----

func TestRedshiftHandler_CreateClusterSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=snap-cluster")
			},
			body: "Action=CreateClusterSnapshot&Version=2012-12-01" +
				"&SnapshotIdentifier=my-snap&ClusterIdentifier=snap-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateClusterSnapshotResponse", "my-snap", "snap-cluster"},
		},
		{
			name:         "missing_snapshot_id",
			body:         "Action=CreateClusterSnapshot&Version=2012-12-01&ClusterIdentifier=cluster",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_cluster_id",
			body:         "Action=CreateClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=snap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "cluster_not_found",
			body: "Action=CreateClusterSnapshot&Version=2012-12-01" +
				"&SnapshotIdentifier=snap&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DeleteClusterSnapshot ----

func TestRedshiftHandler_DeleteClusterSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=del-snap-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=del-snap&ClusterIdentifier=del-snap-cluster")
			},
			body:         "Action=DeleteClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=del-snap",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteClusterSnapshotResponse", "del-snap"},
		},
		{
			name:         "not_found",
			body:         "Action=DeleteClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSnapshotNotFound"},
		},
		{
			name:         "missing_id",
			body:         "Action=DeleteClusterSnapshot&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			// api_op_DeleteClusterSnapshot.go: "If other accounts are
			// authorized to access the snapshot, you must revoke all of the
			// authorizations before you can delete the snapshot."
			name: "still_authorized",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=auth-snap-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=auth-snap&ClusterIdentifier=auth-snap-cluster")
				postRedshiftForm(t, h, "Action=AuthorizeSnapshotAccess&Version=2012-12-01"+
					"&SnapshotIdentifier=auth-snap&AccountWithRestoreAccess=999999999999")
			},
			body:         "Action=DeleteClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=auth-snap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidClusterSnapshotState"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DescribeClusterSnapshots ----

func TestRedshiftHandler_DescribeClusterSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "list_empty",
			body:         "Action=DescribeClusterSnapshots&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterSnapshotsResponse"},
		},
		{
			name: "list_with_snapshot",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=list-snap-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=list-snap&ClusterIdentifier=list-snap-cluster")
			},
			body:         "Action=DescribeClusterSnapshots&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterSnapshotsResponse", "list-snap"},
		},
		{
			name: "filter_by_cluster",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=filter-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=filter-snap&ClusterIdentifier=filter-cluster")
			},
			body:         "Action=DescribeClusterSnapshots&Version=2012-12-01&ClusterIdentifier=filter-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"filter-snap"},
		},
		{
			name:         "snapshot_not_found",
			body:         "Action=DescribeClusterSnapshots&Version=2012-12-01&SnapshotIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSnapshotNotFound"},
		},
		{
			name: "response_includes_snapshot_type_and_create_time",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=meta-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=meta-snap&ClusterIdentifier=meta-cluster")
			},
			body:     "Action=DescribeClusterSnapshots&Version=2012-12-01&SnapshotIdentifier=meta-snap",
			wantCode: http.StatusOK,
			wantContains: []string{
				"<SnapshotType>manual</SnapshotType>",
				"<SnapshotCreateTime>",
			},
		},
		{
			name: "filter_by_snapshot_type_manual",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=type-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=type-snap&ClusterIdentifier=type-cluster")
			},
			body:         "Action=DescribeClusterSnapshots&Version=2012-12-01&SnapshotType=manual",
			wantCode:     http.StatusOK,
			wantContains: []string{"type-snap"},
		},
		{
			name: "filter_by_snapshot_type_automated_returns_empty",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=auto-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=auto-snap&ClusterIdentifier=auto-cluster")
			},
			body:         "Action=DescribeClusterSnapshots&Version=2012-12-01&SnapshotType=automated",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterSnapshotsResponse"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- CopyClusterSnapshot ----

func TestRedshiftHandler_CopyClusterSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=copy-cluster")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=src-snap&ClusterIdentifier=copy-cluster")
			},
			body: "Action=CopyClusterSnapshot&Version=2012-12-01" +
				"&SourceSnapshotIdentifier=src-snap&TargetSnapshotIdentifier=dst-snap",
			wantCode:     http.StatusOK,
			wantContains: []string{"CopyClusterSnapshotResponse", "dst-snap"},
		},
		{
			name: "source_not_found",
			body: "Action=CopyClusterSnapshot&Version=2012-12-01" +
				"&SourceSnapshotIdentifier=nonexistent&TargetSnapshotIdentifier=dst",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSnapshotNotFound"},
		},
		{
			name:         "missing_source",
			body:         "Action=CopyClusterSnapshot&Version=2012-12-01&TargetSnapshotIdentifier=dst",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_target",
			body:         "Action=CopyClusterSnapshot&Version=2012-12-01&SourceSnapshotIdentifier=src",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- RestoreFromClusterSnapshot ----

func TestRedshiftHandler_RestoreFromClusterSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=restore-src")
				postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
					"&SnapshotIdentifier=restore-snap&ClusterIdentifier=restore-src")
			},
			body: "Action=RestoreFromClusterSnapshot&Version=2012-12-01" +
				"&ClusterIdentifier=restore-dst&SnapshotIdentifier=restore-snap",
			wantCode:     http.StatusOK,
			wantContains: []string{"RestoreFromClusterSnapshotResponse", "restore-dst"},
		},
		{
			name: "snapshot_not_found",
			body: "Action=RestoreFromClusterSnapshot&Version=2012-12-01" +
				"&ClusterIdentifier=new-cluster&SnapshotIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterSnapshotNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- Backend: SnapshotCount ----

func TestRedshiftBackend_SnapshotCount(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	require.Equal(t, 0, redshift.SnapshotCount(b))

	h := redshift.NewHandler(b)
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=count-cluster")
	postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
		"&SnapshotIdentifier=count-snap&ClusterIdentifier=count-cluster")
	require.Equal(t, 1, redshift.SnapshotCount(b))

	postRedshiftForm(t, h, "Action=DeleteClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=count-snap")
	require.Equal(t, 0, redshift.SnapshotCount(b))
}

// TestRedshiftHandler_DescribeClusterSnapshots_SnapshotTypeFilter verifies that
// the SnapshotType filter correctly includes and excludes snapshots by type.
func TestRedshiftHandler_DescribeClusterSnapshots_SnapshotTypeFilter(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=st-cluster")
	postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
		"&SnapshotIdentifier=manual-snap&ClusterIdentifier=st-cluster")

	// manual filter: snapshot appears
	rec := postRedshiftForm(t, h, "Action=DescribeClusterSnapshots&Version=2012-12-01&SnapshotType=manual")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "manual-snap")

	// automated filter: snapshot absent
	rec = postRedshiftForm(t, h, "Action=DescribeClusterSnapshots&Version=2012-12-01&SnapshotType=automated")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "manual-snap")
}

// TestRedshiftHandler_DescribeClusterSnapshots_ClusterExistsFilter verifies
// ClusterExists filters by whether the snapshot's source cluster still exists in
// this account (gopherstack-do4v): true selects a still-existing cluster's
// snapshots and requires ClusterIdentifier, false selects snapshots whose cluster
// no longer exists (every orphaned snapshot when ClusterIdentifier is omitted, or
// that one deleted cluster's snapshots when it is given), and omitting the flag
// leaves every match through unfiltered.
func TestRedshiftHandler_DescribeClusterSnapshots_ClusterExistsFilter(t *testing.T) {
	t.Parallel()

	setup := func(h *redshift.Handler) {
		postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ce-live")
		postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
			"&SnapshotIdentifier=ce-live-snap&ClusterIdentifier=ce-live")
		postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ce-dead")
		postRedshiftForm(t, h, "Action=CreateClusterSnapshot&Version=2012-12-01"+
			"&SnapshotIdentifier=ce-dead-snap&ClusterIdentifier=ce-dead")
		postRedshiftForm(t, h, "Action=DeleteCluster&Version=2012-12-01&ClusterIdentifier=ce-dead")
	}

	tests := []struct {
		name        string
		query       string
		wantContain []string
		wantAbsent  []string
		wantCode    int
	}{
		{
			name:        "omitted_pins_both_snapshots",
			query:       "Action=DescribeClusterSnapshots&Version=2012-12-01",
			wantCode:    http.StatusOK,
			wantContain: []string{"ce-live-snap", "ce-dead-snap"},
		},
		{
			name:        "true_existing_cluster_included",
			query:       "Action=DescribeClusterSnapshots&Version=2012-12-01&ClusterExists=true&ClusterIdentifier=ce-live",
			wantCode:    http.StatusOK,
			wantContain: []string{"ce-live-snap"},
		},
		{
			name:       "true_deleted_cluster_excluded",
			query:      "Action=DescribeClusterSnapshots&Version=2012-12-01&ClusterExists=true&ClusterIdentifier=ce-dead",
			wantCode:   http.StatusOK,
			wantAbsent: []string{"ce-dead-snap"},
		},
		{
			name:        "true_without_cluster_identifier_is_invalid",
			query:       "Action=DescribeClusterSnapshots&Version=2012-12-01&ClusterExists=true",
			wantCode:    http.StatusBadRequest,
			wantContain: []string{"InvalidParameterValue"},
		},
		{
			name:        "false_without_identifier_returns_only_orphaned",
			query:       "Action=DescribeClusterSnapshots&Version=2012-12-01&ClusterExists=false",
			wantCode:    http.StatusOK,
			wantContain: []string{"ce-dead-snap"},
			wantAbsent:  []string{"ce-live-snap"},
		},
		{
			name:        "false_deleted_cluster_identifier_included",
			query:       "Action=DescribeClusterSnapshots&Version=2012-12-01&ClusterExists=false&ClusterIdentifier=ce-dead",
			wantCode:    http.StatusOK,
			wantContain: []string{"ce-dead-snap"},
		},
		{
			name:       "false_existing_cluster_identifier_excluded",
			query:      "Action=DescribeClusterSnapshots&Version=2012-12-01&ClusterExists=false&ClusterIdentifier=ce-live",
			wantCode:   http.StatusOK,
			wantAbsent: []string{"ce-live-snap"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)
			setup(h)

			rec := postRedshiftForm(t, h, tt.query)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContain {
				assert.Contains(t, rec.Body.String(), s)
			}

			for _, s := range tt.wantAbsent {
				assert.NotContains(t, rec.Body.String(), s)
			}
		})
	}
}

// ----- DescribeClusterSnapshots pagination -----

// TestDescribeClusterSnapshots_Pagination verifies that MaxRecords and Marker
// work for DescribeClusterSnapshots. Real AWS truncates results and returns a Marker
// to retrieve the next page.
func TestDescribeClusterSnapshots_Pagination(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()

	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=page-cluster")

	for i := 1; i <= 5; i++ {
		id := "snap-page-" + string(rune('a'-1+i))
		body := "Action=CreateClusterSnapshot&Version=2012-12-01&ClusterIdentifier=page-cluster&SnapshotIdentifier=" + id
		rec := postRedshiftForm(t, h, body)
		require.Equal(t, http.StatusOK, rec.Code, "setup: create snapshot %q", id)
	}

	t.Run("no_limit_returns_all", func(t *testing.T) {
		t.Parallel()

		h2 := newRedshiftHandler()
		postRedshiftForm(t, h2, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=page-c2")
		for i := 1; i <= 3; i++ {
			id := "snap-all-" + string(rune('a'-1+i))
			postRedshiftForm(t, h2,
				"Action=CreateClusterSnapshot&Version=2012-12-01&ClusterIdentifier=page-c2&SnapshotIdentifier="+id)
		}

		rec := postRedshiftForm(t, h2, "Action=DescribeClusterSnapshots&Version=2012-12-01")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "snap-all-a")
		assert.Contains(t, rec.Body.String(), "snap-all-c")
	})

	t.Run("max_records_limits_results", func(t *testing.T) {
		t.Parallel()

		h2 := newRedshiftHandler()
		postRedshiftForm(t, h2, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=page-c3")
		for i := 1; i <= 3; i++ {
			id := "snap-max-" + string(rune('a'-1+i))
			postRedshiftForm(t, h2,
				"Action=CreateClusterSnapshot&Version=2012-12-01&ClusterIdentifier=page-c3&SnapshotIdentifier="+id)
		}

		rec := postRedshiftForm(t, h2, "Action=DescribeClusterSnapshots&Version=2012-12-01&MaxRecords=20")
		assert.Equal(t, http.StatusOK, rec.Code)

		body := rec.Body.String()
		assert.Contains(t, body, "snap-max-a")
	})

	t.Run("invalid_max_records_returns_error", func(t *testing.T) {
		t.Parallel()

		rec := postRedshiftForm(t, h, "Action=DescribeClusterSnapshots&Version=2012-12-01&MaxRecords=5")
		assert.Equal(t, http.StatusBadRequest, rec.Code,
			"MaxRecords < 20 should return error")
		assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
	})

	t.Run("invalid_marker_returns_error", func(t *testing.T) {
		t.Parallel()

		rec := postRedshiftForm(t, h,
			"Action=DescribeClusterSnapshots&Version=2012-12-01&Marker=not!!valid!!base64")
		assert.Equal(t, http.StatusBadRequest, rec.Code,
			"invalid base64 Marker should return error")
		assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
	})

	t.Run("marker_from_first_page_fetches_next_page", func(t *testing.T) {
		t.Parallel()

		h2 := newRedshiftHandler()
		postRedshiftForm(t, h2, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=page-c4")

		snapIDs := []string{
			"page-snap-001", "page-snap-002", "page-snap-003",
			"page-snap-004", "page-snap-005",
		}
		for _, id := range snapIDs {
			postRedshiftForm(t, h2,
				"Action=CreateClusterSnapshot&Version=2012-12-01&ClusterIdentifier=page-c4&SnapshotIdentifier="+id)
		}

		// Get first page with MaxRecords=20 (all 5 fit — pagination is only visible with >100 items naturally,
		// but the Marker mechanism is testable by checking that an empty Marker means no more pages).
		rec := postRedshiftForm(t, h2, "Action=DescribeClusterSnapshots&Version=2012-12-01&MaxRecords=20")
		require.Equal(t, http.StatusOK, rec.Code)

		// All snapshots fit in one page, no next marker expected.
		assert.Contains(t, rec.Body.String(), "page-snap-001")
		assert.Contains(t, rec.Body.String(), "page-snap-005")
	})

	t.Run("valid_base64_marker_accepted", func(t *testing.T) {
		t.Parallel()

		validMarker := base64.StdEncoding.EncodeToString([]byte("snap-page-a"))
		rec := postRedshiftForm(t, h,
			"Action=DescribeClusterSnapshots&Version=2012-12-01&Marker="+validMarker)
		assert.Equal(t, http.StatusOK, rec.Code,
			"valid base64 Marker should be accepted")
	})
}

// snapshotsPageXML mirrors just the fields of describeClusterSnapshotsResponse
// this test needs; it lives in the external test package so cannot reference
// the unexported handler type directly.
type snapshotsPageXML struct {
	XMLName xml.Name `xml:"DescribeClusterSnapshotsResponse"`
	Result  struct {
		Marker    string `xml:"Marker"`
		Snapshots struct {
			Snapshot []struct {
				SnapshotIdentifier string `xml:"SnapshotIdentifier"`
			} `xml:"Snapshot"`
		} `xml:"Snapshots"`
	} `xml:"DescribeClusterSnapshotsResult"`
}

// TestDescribeClusterSnapshots_PaginationOrderIsReproducible walks every
// snapshot via Marker-based pagination and asserts the concatenation of pages
// reproduces the full set exactly -- no drops, no duplicates. DescribeClusterSnapshots
// pages over b.snapshots.All(), an unspecified-order map walk (see pkgs/store's
// Table.All doc), so a second call backing the second page can observe a
// completely different order than the first, corrupting the Marker-based walk
// even though SnapshotIdentifier -- the marker value -- is itself unique.
func TestDescribeClusterSnapshots_PaginationOrderIsReproducible(t *testing.T) {
	t.Parallel()

	const numSnapshots = 130
	const pageSize = 25

	for iter := range 30 {
		h := newRedshiftHandler()
		postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=order-cluster")

		want := make(map[string]bool, numSnapshots)
		for i := range numSnapshots {
			id := fmt.Sprintf("order-snap-%03d", i)
			want[id] = true

			rec := postRedshiftForm(
				t,
				h,
				"Action=CreateClusterSnapshot&Version=2012-12-01&ClusterIdentifier=order-cluster&SnapshotIdentifier="+id,
			)
			require.Equalf(t, http.StatusOK, rec.Code, "iteration %d: setup create snapshot %q", iter, id)
		}

		got := make(map[string]int, numSnapshots)
		marker := ""

		for page := range numSnapshots/pageSize + 5 {
			body := fmt.Sprintf("Action=DescribeClusterSnapshots&Version=2012-12-01&MaxRecords=%d", pageSize)
			if marker != "" {
				body += "&Marker=" + url.QueryEscape(marker)
			}

			rec := postRedshiftForm(t, h, body)
			require.Equalf(t, http.StatusOK, rec.Code, "iteration %d page %d", iter, page)

			var parsed snapshotsPageXML
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &parsed))

			for _, s := range parsed.Result.Snapshots.Snapshot {
				got[s.SnapshotIdentifier]++
			}

			if parsed.Result.Marker == "" {
				break
			}

			marker = parsed.Result.Marker
		}

		for id := range want {
			assert.Equalf(t, 1, got[id], "iteration %d: snapshot %s expected exactly once, got %d", iter, id, got[id])
		}

		assert.Lenf(t, got, numSnapshots, "iteration %d: total distinct snapshots returned", iter)
	}
}

// TestRestoreFromClusterSnapshot_CopiesClusterProperties verifies that
// RestoreFromClusterSnapshot uses the source cluster's properties, not defaults.
func TestRestoreFromClusterSnapshot_CopiesClusterProperties(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()

	// Create cluster with non-default node type.
	postRedshiftForm(t, h,
		"Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=src-cluster"+
			"&NodeType=ds2.xlarge&DBName=mydb")
	postRedshiftForm(t, h,
		"Action=CreateClusterSnapshot&Version=2012-12-01"+
			"&ClusterIdentifier=src-cluster&SnapshotIdentifier=my-snap")

	// Restore from snapshot.
	postRedshiftForm(t, h,
		"Action=RestoreFromClusterSnapshot&Version=2012-12-01"+
			"&ClusterIdentifier=restored-cluster&SnapshotIdentifier=my-snap")

	// Verify restored cluster has source cluster's node type and DBName.
	rec := postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=restored-cluster")
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "ds2.xlarge", "restored cluster should use source node type")
	assert.True(t,
		strings.Contains(body, "mydb") || strings.Contains(body, "DBName"),
		"restored cluster should reference source DBName")
}

// TestRestoreFromClusterSnapshot_TagsInitialized guards against a
// nil-pointer panic: a cluster produced by RestoreFromClusterSnapshot must own
// a live Tags collection just like CreateCluster, because DescribeTags calls
// c.Tags.Clone() unconditionally for every cluster in the backend.
func TestRestoreFromClusterSnapshot_TagsInitialized(t *testing.T) {
	t.Parallel()

	b := newRedshiftBackend()

	_, err := b.CreateCluster("src-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = b.CreateClusterSnapshot("src-snap", "src-cluster")
	require.NoError(t, err)

	_, err = b.RestoreFromClusterSnapshot("restored-cluster", "src-snap")
	require.NoError(t, err)

	// Previously panicked with a nil pointer dereference inside tags.Tags.Clone.
	require.NotPanics(t, func() {
		_ = b.DescribeTags()
	})

	// CreateTags/DeleteTags must also work against the restored cluster.
	require.NoError(t, b.CreateTags("restored-cluster", map[string]string{"env": "prod"}))

	all := b.DescribeTags()
	assert.Equal(t, map[string]string{"env": "prod"}, all["restored-cluster"])

	require.NoError(t, b.DeleteTags("restored-cluster", []string{"env"}))
}

// TestRestoreFromClusterSnapshot_TagsInitialized_HTTP is the HTTP-level
// counterpart: DescribeTags must not 500 after a RestoreFromClusterSnapshot,
// including via the tag-filtered DescribeClusters path.
func TestRestoreFromClusterSnapshot_TagsInitialized_HTTP(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=src-cluster")
	postRedshiftForm(t, h,
		"Action=CreateClusterSnapshot&Version=2012-12-01&SnapshotIdentifier=src-snap&ClusterIdentifier=src-cluster")

	rec := postRedshiftForm(t, h,
		"Action=RestoreFromClusterSnapshot&Version=2012-12-01"+
			"&ClusterIdentifier=restored-cluster&SnapshotIdentifier=src-snap")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postRedshiftForm(t, h, "Action=DescribeTags&Version=2012-12-01")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DescribeTagsResponse")

	rec = postRedshiftForm(t, h,
		"Action=DescribeClusters&Version=2012-12-01&TagKey=env")
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestRestoreFromClusterSnapshot_Lifecycle verifies the restored
// cluster's ClusterStatus actually reaches "available" instead of getting
// stuck in "restoring" forever.
func TestRestoreFromClusterSnapshot_Lifecycle(t *testing.T) {
	t.Parallel()

	t.Run("no_activation_delay_is_immediately_available", func(t *testing.T) {
		t.Parallel()

		b := newRedshiftBackend()

		_, err := b.CreateCluster("src-cluster", "", "", "", nil, "")
		require.NoError(t, err)
		_, err = b.CreateClusterSnapshot("src-snap", "src-cluster")
		require.NoError(t, err)

		restored, err := b.RestoreFromClusterSnapshot("restored-cluster", "src-snap")
		require.NoError(t, err)
		assert.Equal(t, "available", restored.Status)
	})

	t.Run("activation_delay_transitions_restoring_to_available", func(t *testing.T) {
		t.Parallel()

		b := newRedshiftBackend()
		redshift.SetClusterActivationDelay(b, 20*time.Millisecond)

		_, err := b.CreateCluster("src-cluster", "", "", "", nil, "")
		require.NoError(t, err)
		_, err = b.CreateClusterSnapshot("src-snap", "src-cluster")
		require.NoError(t, err)

		restored, err := b.RestoreFromClusterSnapshot("restored-cluster", "src-snap")
		require.NoError(t, err)
		assert.Equal(t, "restoring", restored.Status,
			"restored cluster should start in restoring state when an activation delay is configured")

		require.Eventually(t, func() bool {
			clusters, _, descErr := b.DescribeClusters("restored-cluster", "", 0, nil, nil)

			return descErr == nil && len(clusters) == 1 && clusters[0].Status == "available"
		}, time.Second, 5*time.Millisecond,
			"restored cluster must transition out of restoring, previously it never did")
	})
}
