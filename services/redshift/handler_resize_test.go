package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- CancelResize ----

func TestHandler_CancelResize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *redshift.Handler, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *redshift.Handler, b *redshift.InMemoryBackend) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=resize-cluster")
				b.AddActiveResizeInternal("resize-cluster", &redshift.ResizeProgress{
					TargetNodeType:      "ra3.xlplus",
					TargetNumberOfNodes: 4,
					TargetClusterType:   "multi-node",
					Status:              "IN_PROGRESS",
					ResizeType:          "ClassicResize",
					AllowCancelResize:   true,
				})
			},
			body:         "Action=CancelResize&Version=2012-12-01&ClusterIdentifier=resize-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"CancelResizeResponse", "CANCELLED"},
		},
		{
			name:         "missing_cluster_identifier",
			body:         "Action=CancelResize&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "cluster_not_found",
			body:         "Action=CancelResize&Version=2012-12-01&ClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterNotFound"},
		},
		{
			name: "no_active_resize",
			setup: func(t *testing.T, h *redshift.Handler, _ *redshift.InMemoryBackend) {
				t.Helper()
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=no-resize-cluster")
			},
			body:         "Action=CancelResize&Version=2012-12-01&ClusterIdentifier=no-resize-cluster",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ResizeNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postRedshiftForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestBackend_CancelResize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		setup     func(b *redshift.InMemoryBackend)
		name      string
		clusterID string
	}{
		{
			name: "success",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("cr-cluster", "", "", "", nil, "")
				b.AddActiveResizeInternal("cr-cluster", &redshift.ResizeProgress{
					Status:            "IN_PROGRESS",
					AllowCancelResize: true,
				})
			},
			clusterID: "cr-cluster",
		},
		{
			name:      "empty_cluster_id",
			clusterID: "",
			wantErr:   redshift.ErrInvalidParameter,
		},
		{
			name:      "cluster_not_found",
			clusterID: "nonexistent",
			wantErr:   redshift.ErrClusterNotFound,
		},
		{
			name: "no_active_resize",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("no-resize", "", "", "", nil, "")
			},
			clusterID: "no-resize",
			wantErr:   redshift.ErrResizeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			result, err := b.CancelResize(tt.clusterID)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "CANCELLED", result.Status)
		})
	}
}

// ---- CancelResize: AllowCancelResize=false is rejected ----

func TestCancelResize_AllowCancelResizeFalse(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	_, err := b.CreateCluster("cr-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	b.AddActiveResizeInternal("cr-cluster", &redshift.ResizeProgress{
		Status:            "IN_PROGRESS",
		AllowCancelResize: false, // not cancellable
	})

	rec := postRedshiftForm(t, h,
		"Action=CancelResize&Version=2012-12-01&ClusterIdentifier=cr-cluster")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidClusterState")
}

// ---- CancelResize: cluster not found ----

func TestCancelResize_ClusterNotFound(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	rec := postRedshiftForm(t, h,
		"Action=CancelResize&Version=2012-12-01&ClusterIdentifier=nonexistent")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ClusterNotFound")
}

// ---- CancelResize: no active resize ----

func TestCancelResize_NoActiveResize(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	_, err := b.CreateCluster("nr-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	rec := postRedshiftForm(t, h,
		"Action=CancelResize&Version=2012-12-01&ClusterIdentifier=nr-cluster")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ResizeNotFound")
}

// ---- Resize progress XML fields ----

func TestCancelResize_ReturnsAllXMLFields(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(b)

	_, err := b.CreateCluster("rp-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	b.AddActiveResizeInternal("rp-cluster", &redshift.ResizeProgress{
		Status:                 "IN_PROGRESS",
		TargetNodeType:         "ra3.xlplus",
		TargetClusterType:      "multi-node",
		TargetNumberOfNodes:    4,
		ResizeType:             "ClassicResize",
		AllowCancelResize:      true,
		ImportTablesCompleted:  []string{"table1"},
		ImportTablesInProgress: []string{"table2"},
		ImportTablesNotStarted: []string{"table3", "table4"},
	})

	rec := postRedshiftForm(t, h,
		"Action=CancelResize&Version=2012-12-01&ClusterIdentifier=rp-cluster")

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "CancelResizeResponse")
	assert.Contains(t, body, "CANCELLED")
	assert.Contains(t, body, "ra3.xlplus")
	assert.Contains(t, body, "table1")
	assert.Contains(t, body, "table2")
	assert.Contains(t, body, "table3")
}

// ---- DescribeResize ----

func TestHandler_DescribeResize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, b *redshift.InMemoryBackend)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateCluster("dr-cluster", "dc2.large", "dev", "admin", nil, "")
				require.NoError(t, err)
				b.AddActiveResizeInternal("dr-cluster", &redshift.ResizeProgress{
					Status:         "IN_PROGRESS",
					TargetNodeType: "dc2.large",
				})
			},
			body:         "Action=DescribeResize&Version=2012-12-01&ClusterIdentifier=dr-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeResizeResponse", "IN_PROGRESS"},
		},
		{
			name:     "missing_cluster_id",
			body:     "Action=DescribeResize&Version=2012-12-01&ClusterIdentifier=",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "not_found",
			body:     "Action=DescribeResize&Version=2012-12-01&ClusterIdentifier=missing",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.setup != nil {
				tt.setup(t, b)
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestBackend_DescribeResize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(b *redshift.InMemoryBackend)
		name      string
		clusterID string
		wantErr   bool
	}{
		{
			name: "success",
			setup: func(b *redshift.InMemoryBackend) {
				_, _ = b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
				b.AddActiveResizeInternal("c1", &redshift.ResizeProgress{Status: "IN_PROGRESS"})
			},
			clusterID: "c1",
			wantErr:   false,
		},
		{
			name:      "missing_cluster_id",
			clusterID: "",
			wantErr:   true,
		},
		{
			name:      "not_found",
			clusterID: "missing",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			if tt.setup != nil {
				tt.setup(b)
			}

			rp, err := b.DescribeResize(tt.clusterID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "IN_PROGRESS", rp.Status)
		})
	}
}

// TestHandler_ResizeCluster_ObservableViaDescribeResize locks in that a resize
// triggered through the real ResizeCluster API op is observable afterward via
// DescribeResize. Before this fix, ResizeCluster mutated the cluster's node
// type/count synchronously but never recorded anything in activeResizes, so
// DescribeResize always reported ResizeNotFound immediately after a resize --
// the only way to populate activeResizes was the AddActiveResizeInternal
// test-seed helper, which no real client traffic ever calls.
func TestHandler_ResizeCluster_ObservableViaDescribeResize(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=resize-cluster")

	rec := postRedshiftForm(t, h, "Action=ResizeCluster&Version=2012-12-01"+
		"&ClusterIdentifier=resize-cluster&NodeType=ra3.4xlarge&NumberOfNodes=4")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = postRedshiftForm(t, h, "Action=DescribeResize&Version=2012-12-01&ClusterIdentifier=resize-cluster")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, "<TargetNodeType>ra3.4xlarge</TargetNodeType>")
	assert.Contains(t, body, "<TargetNumberOfNodes>4</TargetNumberOfNodes>")
	assert.Contains(t, body, "<Status>SUCCEEDED</Status>")

	// The resize is already complete, so it should not be cancellable.
	rec = postRedshiftForm(t, h, "Action=CancelResize&Version=2012-12-01&ClusterIdentifier=resize-cluster")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidClusterState")
}
