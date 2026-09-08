package redshift_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

func TestRedshiftHandler_CreateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			body: "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=test-cluster&" +
				"NodeType=dc2.large&DBName=mydb&MasterUsername=admin",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateClusterResponse", "test-cluster"},
		},
		{
			name:         "empty_id",
			body:         "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newRedshiftHandler()
			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestRedshiftHandler_DeleteCluster(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=del-cluster")

	rec := postRedshiftForm(t, h, "Action=DeleteCluster&Version=2012-12-01&ClusterIdentifier=del-cluster")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DeleteClusterResponse")
}

func TestRedshiftHandler_DescribeClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "list_all",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=desc-cluster")
			},
			body:         "Action=DescribeClusters&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClustersResponse", "desc-cluster"},
		},
		{
			name:     "not_found",
			body:     "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}
			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestRedshiftHandler_DeleteCluster_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=del-cluster")
			},
			body:         "Action=DeleteCluster&Version=2012-12-01&ClusterIdentifier=del-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteClusterResponse", "del-cluster"},
		},
		{
			name:     "not_found",
			body:     "Action=DeleteCluster&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty_id",
			body:     "Action=DeleteCluster&Version=2012-12-01&ClusterIdentifier=",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}
			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestRedshiftHandler_DescribeLoggingStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "logging_never_enabled",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=dls-cluster")
			},
			body:         "Action=DescribeLoggingStatus&Version=2012-12-01&ClusterIdentifier=dls-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeLoggingStatusResponse", "<LoggingEnabled>false</LoggingEnabled>"},
		},
		{
			name: "reflects_enabled_state",
			setup: func(t *testing.T, h *redshift.Handler) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=dls-cluster2")
				postRedshiftForm(t, h, "Action=EnableLogging&Version=2012-12-01"+
					"&ClusterIdentifier=dls-cluster2&BucketName=dls-bucket")
			},
			body:     "Action=DescribeLoggingStatus&Version=2012-12-01&ClusterIdentifier=dls-cluster2",
			wantCode: http.StatusOK,
			wantContains: []string{
				"DescribeLoggingStatusResponse", "<LoggingEnabled>true</LoggingEnabled>", "dls-bucket",
			},
		},
		{
			name:         "missing_cluster_id",
			body:         "Action=DescribeLoggingStatus&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "cluster_not_found",
			body:         "Action=DescribeLoggingStatus&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// mockDNSRegistrar is a test double for redshift.DNSRegistrar.
type mockDNSRegistrar struct {
	registered map[string]bool
}

func (m *mockDNSRegistrar) Register(hostname string) {
	m.registered[hostname] = true
}

func (m *mockDNSRegistrar) Deregister(hostname string) {
	delete(m.registered, hostname)
}

func TestRedshiftBackend_DNSRegistrar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		clusterID      string
		wantRegistered bool
		deleteAfter    bool
	}{
		{
			name:           "registers_on_create",
			clusterID:      "my-cluster",
			wantRegistered: true,
		},
		{
			name:           "deregisters_on_delete",
			clusterID:      "del-cluster",
			deleteAfter:    true,
			wantRegistered: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registrar := &mockDNSRegistrar{registered: make(map[string]bool)}
			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			b.SetDNSRegistrar(registrar)

			cluster, err := b.CreateCluster(tt.clusterID, "dc2.large", "dev", "admin", nil, "")
			require.NoError(t, err)

			if tt.deleteAfter {
				_, err = b.DeleteCluster(tt.clusterID)
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantRegistered, registrar.registered[cluster.Endpoint])
		})
	}
}

// ----- DeleteCluster SkipFinalClusterSnapshot -----

// TestDeleteCluster_FinalSnapshot verifies that DeleteCluster respects
// SkipFinalClusterSnapshot and FinalClusterSnapshotIdentifier. Real AWS:
//   - Requires FinalClusterSnapshotIdentifier when SkipFinalClusterSnapshot=false
//   - Creates a snapshot before deletion when FinalClusterSnapshotIdentifier is provided
func TestDeleteCluster_FinalSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		skipFinal         string
		finalSnapshotID   string
		wantErr           string
		wantCode          int
		wantSnapshotAfter bool
	}{
		{
			name:              "skip_true_no_snapshot_id_succeeds",
			skipFinal:         "true",
			finalSnapshotID:   "",
			wantCode:          http.StatusOK,
			wantSnapshotAfter: false,
		},
		{
			name:              "skip_false_with_snapshot_id_creates_snapshot",
			skipFinal:         "false",
			finalSnapshotID:   "final-snap-1",
			wantCode:          http.StatusOK,
			wantSnapshotAfter: true,
		},
		{
			name:            "skip_false_without_snapshot_id_returns_error",
			skipFinal:       "false",
			finalSnapshotID: "",
			wantCode:        http.StatusBadRequest,
			wantErr:         "FinalClusterSnapshotIdentifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			clusterID := "del-" + strings.ReplaceAll(tt.name, "_", "-")

			postRedshiftForm(t, h,
				"Action=CreateCluster&Version=2012-12-01&ClusterIdentifier="+clusterID)

			body := "Action=DeleteCluster&Version=2012-12-01&ClusterIdentifier=" + clusterID +
				"&SkipFinalClusterSnapshot=" + tt.skipFinal
			if tt.finalSnapshotID != "" {
				body += "&FinalClusterSnapshotIdentifier=" + tt.finalSnapshotID
			}

			rec := postRedshiftForm(t, h, body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"DeleteCluster skip=%s snapshotID=%q", tt.skipFinal, tt.finalSnapshotID)

			if tt.wantErr != "" {
				assert.Contains(t, rec.Body.String(), tt.wantErr)
			}

			if tt.wantSnapshotAfter {
				snapRec := postRedshiftForm(t, h,
					"Action=DescribeClusterSnapshots&Version=2012-12-01&SnapshotIdentifier="+tt.finalSnapshotID)
				assert.Equal(t, http.StatusOK, snapRec.Code,
					"final snapshot %q should exist after deletion", tt.finalSnapshotID)
				assert.Contains(t, snapRec.Body.String(), tt.finalSnapshotID)
			}
		})
	}
}

// ----- DescribeClusters tag filtering -----
//
// Tag-filter coverage (real TagKeys/TagValues param names, and the
// filter-before-paginate ordering) lives in
// TestDescribeClusters_TagKeysFilter and
// TestDescribeClusters_TagKeysFilter_PaginationOrdering
// (handler_cluster_tagkeys_test.go), driven through the real SDK client.
// A prior version of this file tested singular "TagKey"/"TagValue" query
// params that do not exist on the real DescribeClustersInput
// (redshift@v1.65.4 defines TagKeys/TagValues as []string) and so asserted
// behavior no real client can produce.

// TestDescribeClusters_Pagination verifies Marker/MaxRecords pagination.
func TestDescribeClusters_Pagination(t *testing.T) {
	t.Parallel()

	type pageTC struct {
		name       string
		query      string
		wantAbsent []string
		wantInBody []string
		wantCode   int
		wantMarker bool
	}

	tests := []pageTC{
		{
			name:       "no_params_returns_all",
			query:      "Action=DescribeClusters&Version=2012-12-01",
			wantCode:   http.StatusOK,
			wantInBody: []string{"alpha", "beta", "gamma"},
			wantMarker: false,
		},
		{
			name:       "max_records_1_returns_first",
			query:      "Action=DescribeClusters&Version=2012-12-01&MaxRecords=1",
			wantCode:   http.StatusOK,
			wantInBody: []string{"alpha", "Marker"},
			wantAbsent: []string{"beta", "gamma"},
			wantMarker: true,
		},
		{
			name:       "marker_advances_to_second_page",
			query:      "Action=DescribeClusters&Version=2012-12-01&MaxRecords=1&Marker=alpha",
			wantCode:   http.StatusOK,
			wantInBody: []string{"beta"},
			wantAbsent: []string{"alpha", "gamma"},
			wantMarker: true,
		},
		{
			name:       "marker_advances_to_last_page",
			query:      "Action=DescribeClusters&Version=2012-12-01&MaxRecords=1&Marker=beta",
			wantCode:   http.StatusOK,
			wantInBody: []string{"gamma"},
			wantAbsent: []string{"alpha", "beta"},
			wantMarker: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			for _, id := range []string{"alpha", "beta", "gamma"} {
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier="+id)
			}

			rec := postRedshiftForm(t, h, tt.query)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantInBody {
				assert.Contains(t, rec.Body.String(), s, "expected %q in response", s)
			}

			for _, s := range tt.wantAbsent {
				assert.NotContains(t, rec.Body.String(), s, "unexpected %q in response", s)
			}
		})
	}
}

// ---- CreateCluster returns NumberOfNodes and Port ----

func TestCreateCluster_ReturnsExpectedFields(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	rec := postRedshiftForm(t, h,
		"Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=fields-cluster&NodeType=dc2.8xlarge")

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "CreateClusterResponse")
	assert.Contains(t, body, "fields-cluster")
	assert.Contains(t, body, "dc2.8xlarge")
	// Port 5439 is default
	assert.Contains(t, body, "5439")
}

// ---- DescribeClusters: deep copy check ----

func TestDescribeClusters_DeepCopy(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	clusters, _, err := b.DescribeClusters("", "", 0, nil, nil)
	require.NoError(t, err)
	require.Len(t, clusters, 1)

	// Modifying the returned slice should not affect the backend
	clusters[0].ClusterIdentifier = "mutated"

	clusters2, _, err := b.DescribeClusters("", "", 0, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "c1", clusters2[0].ClusterIdentifier, "backend should not be mutated by caller")
}
