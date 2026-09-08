package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- CreateClusterParameterGroup ----

func TestRedshiftHandler_CreateClusterParameterGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			body: "Action=CreateClusterParameterGroup&Version=2012-12-01" +
				"&ParameterGroupName=my-group&ParameterGroupFamily=redshift-1.0" +
				"&Description=my+description",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateClusterParameterGroupResponse", "my-group", "redshift-1.0"},
		},
		{
			name:         "missing_name",
			body:         "Action=CreateClusterParameterGroup&Version=2012-12-01&ParameterGroupFamily=redshift-1.0",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "missing_family",
			body:         "Action=CreateClusterParameterGroup&Version=2012-12-01&ParameterGroupName=my-group",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "duplicate_name",
			body: "Action=CreateClusterParameterGroup&Version=2012-12-01" +
				"&ParameterGroupName=existing-group&ParameterGroupFamily=redshift-1.0",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterParameterGroupAlreadyExists"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			h := redshift.NewHandler(b)

			if tt.name == "duplicate_name" {
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=existing-group&ParameterGroupFamily=redshift-1.0")
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DeleteClusterParameterGroup ----

func TestRedshiftHandler_DeleteClusterParameterGroup(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=del-group&ParameterGroupFamily=redshift-1.0")
			},
			body:         "Action=DeleteClusterParameterGroup&Version=2012-12-01&ParameterGroupName=del-group",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteClusterParameterGroupResponse"},
		},
		{
			name:         "not_found",
			body:         "Action=DeleteClusterParameterGroup&Version=2012-12-01&ParameterGroupName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterParameterGroupNotFound"},
		},
		{
			name:         "missing_name",
			body:         "Action=DeleteClusterParameterGroup&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			// Real AWS: "Cannot delete a default cluster parameter group."
			name:         "default_parameter_group_rejected",
			body:         "Action=DeleteClusterParameterGroup&Version=2012-12-01&ParameterGroupName=default.redshift-1.0",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidClusterParameterGroupState"},
		},
		{
			// Real AWS: "You cannot delete a parameter group if it is
			// associated with a cluster" (api_op_DeleteClusterParameterGroup.go).
			name: "associated_with_cluster_rejected",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=assoc-pg&ParameterGroupFamily=redshift-1.0")
				postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01"+
					"&ClusterIdentifier=assoc-pg-cluster&NodeType=dc2.large"+
					"&MasterUsername=admin&MasterUserPassword=Password1"+
					"&ClusterParameterGroupName=assoc-pg")
			},
			body:         "Action=DeleteClusterParameterGroup&Version=2012-12-01&ParameterGroupName=assoc-pg",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidClusterParameterGroupState"},
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

// ---- DescribeClusterParameterGroups ----

func TestRedshiftHandler_DescribeClusterParameterGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "list_all_empty",
			body:         "Action=DescribeClusterParameterGroups&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterParameterGroupsResponse"},
		},
		{
			name: "list_with_group",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=my-pg&ParameterGroupFamily=redshift-1.0")
			},
			body:         "Action=DescribeClusterParameterGroups&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterParameterGroupsResponse", "my-pg"},
		},
		{
			name: "describe_specific_group",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=specific-pg&ParameterGroupFamily=redshift-1.0")
			},
			body:         "Action=DescribeClusterParameterGroups&Version=2012-12-01&ParameterGroupName=specific-pg",
			wantCode:     http.StatusOK,
			wantContains: []string{"specific-pg"},
		},
		{
			name:         "not_found",
			body:         "Action=DescribeClusterParameterGroups&Version=2012-12-01&ParameterGroupName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterParameterGroupNotFound"},
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

// ---- DescribeClusterParameters ----

func TestRedshiftHandler_DescribeClusterParameters(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=params-pg&ParameterGroupFamily=redshift-1.0")
			},
			body:         "Action=DescribeClusterParameters&Version=2012-12-01&ParameterGroupName=params-pg",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeClusterParametersResponse", "enable_user_activity_logging"},
		},
		{
			name:         "missing_group_name",
			body:         "Action=DescribeClusterParameters&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "group_not_found",
			body:         "Action=DescribeClusterParameters&Version=2012-12-01&ParameterGroupName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterParameterGroupNotFound"},
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

// ---- ModifyClusterParameterGroup ----

func TestRedshiftHandler_ModifyClusterParameterGroup(t *testing.T) {
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
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=mod-pg&ParameterGroupFamily=redshift-1.0")
			},
			body: "Action=ModifyClusterParameterGroup&Version=2012-12-01&ParameterGroupName=mod-pg" +
				"&Parameters.Parameter.1.ParameterName=enable_user_activity_logging" +
				"&Parameters.Parameter.1.ParameterValue=true",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyClusterParameterGroupResponse", "mod-pg"},
		},
		{
			name:         "not_found",
			body:         "Action=ModifyClusterParameterGroup&Version=2012-12-01&ParameterGroupName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterParameterGroupNotFound"},
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

// ---- ResetClusterParameterGroup ----

func TestRedshiftHandler_ResetClusterParameterGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *redshift.Handler)
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "reset_all",
			setup: func(h *redshift.Handler) {
				postRedshiftForm(t, h, "Action=CreateClusterParameterGroup&Version=2012-12-01"+
					"&ParameterGroupName=reset-pg&ParameterGroupFamily=redshift-1.0")
			},
			body: "Action=ResetClusterParameterGroup&Version=2012-12-01" +
				"&ParameterGroupName=reset-pg&ResetAllParameters=true",
			wantCode:     http.StatusOK,
			wantContains: []string{"ResetClusterParameterGroupResponse", "reset-pg"},
		},
		{
			name:         "not_found",
			body:         "Action=ResetClusterParameterGroup&Version=2012-12-01&ParameterGroupName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ClusterParameterGroupNotFound"},
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

// ---- DescribeDefaultClusterParameters ----

func TestRedshiftHandler_DescribeDefaultClusterParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "success",
			body:         "Action=DescribeDefaultClusterParameters&Version=2012-12-01&ParameterGroupFamily=redshift-1.0",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeDefaultClusterParametersResponse", "enable_user_activity_logging"},
		},
		{
			name:         "missing_family",
			body:         "Action=DescribeDefaultClusterParameters&Version=2012-12-01",
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

// ---- Backend: ParameterGroupCount ----

func TestRedshiftBackend_ParameterGroupCount(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	require.Equal(t, 0, redshift.ParameterGroupCount(b))

	b.AddParameterGroupInternal(&redshift.ClusterParameterGroup{
		ParameterGroupName:   "test-pg",
		ParameterGroupFamily: "redshift-1.0",
	})

	require.Equal(t, 1, redshift.ParameterGroupCount(b))
}
