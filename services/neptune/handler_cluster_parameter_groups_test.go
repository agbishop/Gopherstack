package neptune_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/neptune"
)

func TestHandler_DBClusterParameterGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_parameter_group",
			vals: url.Values{
				"Action":                      {"CreateDBClusterParameterGroup"},
				"Version":                     {"2014-10-31"},
				"DBClusterParameterGroupName": {"test-pg"},
				"DBParameterGroupFamily":      {"neptune1.3"},
				"Description":                 {"test parameter group"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "test-pg",
		},
		{
			name: "describe_parameter_groups",
			vals: url.Values{
				"Action":  {"DescribeDBClusterParameterGroups"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeDBClusterParameterGroupsResponse",
		},
		{
			name: "modify_parameter_group",
			vals: url.Values{
				"Action":                      {"ModifyDBClusterParameterGroup"},
				"Version":                     {"2014-10-31"},
				"DBClusterParameterGroupName": {"test-pg"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "test-pg",
		},
		{
			name: "delete_parameter_group",
			vals: url.Values{
				"Action":                      {"DeleteDBClusterParameterGroup"},
				"Version":                     {"2014-10-31"},
				"DBClusterParameterGroupName": {"test-pg"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DeleteDBClusterParameterGroupResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tt.name != "create_parameter_group" {
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterParameterGroup"},
					"Version":                     {"2014-10-31"},
					"DBClusterParameterGroupName": {"test-pg"},
					"DBParameterGroupFamily":      {"neptune1.3"},
					"Description":                 {"test pg"},
				})
			}
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestHandler_DeleteDBClusterParameterGroup_InUse asserts
// DeleteDBClusterParameterGroup rejects a cluster parameter group still
// associated with a DB cluster (api_op_DeleteDBClusterParameterGroup.go:
// "The DB cluster parameter group to be deleted can't be associated with any
// DB clusters.").
func TestHandler_DeleteDBClusterParameterGroup_InUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"used-cpg"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"in use"},
	})
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBCluster"},
		"Version":                     {"2014-10-31"},
		"DBClusterIdentifier":         {"cpg-user-cluster"},
		"DBClusterParameterGroupName": {"used-cpg"},
	})

	recEarly := doRequest(t, h, url.Values{
		"Action":                      {"DeleteDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"used-cpg"},
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)
	assert.Contains(t, recEarly.Body.String(), "InvalidDBParameterGroupState")

	recDeleteCluster := doRequest(t, h, url.Values{
		"Action":              {"DeleteDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"cpg-user-cluster"},
		"SkipFinalSnapshot":   {"true"},
	})
	require.Equal(t, http.StatusOK, recDeleteCluster.Code)

	recDelete := doRequest(t, h, url.Values{
		"Action":                      {"DeleteDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"used-cpg"},
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_CopyDBClusterParameterGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*neptune.Handler)
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "copy_pg_success",
			setup: func(h *neptune.Handler) {
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterParameterGroup"},
					"Version":                     {"2014-10-31"},
					"DBClusterParameterGroupName": {"src-pg"},
					"DBParameterGroupFamily":      {"neptune1.3"},
					"Description":                 {"source group"},
				})
			},
			vals: url.Values{
				"Action":  {"CopyDBClusterParameterGroup"},
				"Version": {"2014-10-31"},
				"SourceDBClusterParameterGroupIdentifier":  {"src-pg"},
				"TargetDBClusterParameterGroupIdentifier":  {"dst-pg"},
				"TargetDBClusterParameterGroupDescription": {"copy of src-pg"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "dst-pg",
		},
		{
			name: "copy_pg_source_not_found",
			vals: url.Values{
				"Action":  {"CopyDBClusterParameterGroup"},
				"Version": {"2014-10-31"},
				"SourceDBClusterParameterGroupIdentifier": {"no-such-pg"},
				"TargetDBClusterParameterGroupIdentifier": {"new-pg"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBParameterGroupNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// --- Parameter group family validation ---

func TestCreateDBClusterParameterGroup_InvalidFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		family       string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "invalid_family",
			family:       "mysql5.7",
			wantStatus:   http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "empty_family",
			family:       "",
			wantStatus:   http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "valid_neptune12",
			family:       "neptune1.2",
			wantStatus:   http.StatusOK,
			wantContains: "CreateDBClusterParameterGroupResponse",
		},
		{
			name:         "valid_neptune13",
			family:       "neptune1.3",
			wantStatus:   http.StatusOK,
			wantContains: "CreateDBClusterParameterGroupResponse",
		},
		{
			name:         "valid_neptune14",
			family:       "neptune1.4",
			wantStatus:   http.StatusOK,
			wantContains: "CreateDBClusterParameterGroupResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, url.Values{
				"Action":                      {"CreateDBClusterParameterGroup"},
				"Version":                     {"2014-10-31"},
				"DBClusterParameterGroupName": {"pg-" + strings.ReplaceAll(tt.name, "_", "-")},
				"DBParameterGroupFamily":      {tt.family},
				"Description":                 {"test"},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// ---- DBClusterParameterGroup comprehensive coverage ----

func TestDBClusterParameterGroup_CreateAllFamilies(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, family := range []string{"neptune1.2", "neptune1.3", "neptune1.4"} {
		rr := doRequest(t, h, url.Values{
			"Action":                      {"CreateDBClusterParameterGroup"},
			"Version":                     {"2014-10-31"},
			"DBClusterParameterGroupName": {family + "-pg"},
			"DBParameterGroupFamily":      {family},
			"Description":                 {"group for " + family},
		})
		require.Equal(t, http.StatusOK, rr.Code, "family %s", family)
		assert.Contains(t, rr.Body.String(), family+"-pg")
		assert.Contains(t, rr.Body.String(), family)
	}
}

func TestDBClusterParameterGroup_DescribeByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-find-me"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"find me"},
	})
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-not-me"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"not me"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterParameterGroups"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-find-me"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "pg-find-me")
	assert.NotContains(t, body, "pg-not-me")
}

func TestDBClusterParameterGroup_ModifyReturnsName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-mod-test"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"test"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                      {"ModifyDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-mod-test"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "pg-mod-test")
}

func TestDBClusterParameterGroup_ResetReturnsName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-reset-test"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"test"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                      {"ResetDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-reset-test"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "pg-reset-test")
}

func TestDBClusterParameterGroup_DeleteNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                      {"DeleteDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"nonexistent-pg"},
	})
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

func TestDBClusterParameterGroup_DescribeParameters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-params"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"test"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterParameters"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-params"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "DescribeDBClusterParametersResponse")
}

func TestDBClusterParameterGroup_CopyPreservesFamily(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"pg-src-copy"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"source"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":  {"CopyDBClusterParameterGroup"},
		"Version": {"2014-10-31"},
		"SourceDBClusterParameterGroupIdentifier":  {"pg-src-copy"},
		"TargetDBClusterParameterGroupIdentifier":  {"pg-dst-copy"},
		"TargetDBClusterParameterGroupDescription": {"copy of source"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "pg-dst-copy")
	assert.Contains(t, body, "neptune1.3")
	assert.Contains(t, body, "copy of source")
}

func TestTags_OnParameterGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"tagged-cpg"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"test"},
		"Tags.Tag.1.Key":              {"Team"},
		"Tags.Tag.1.Value":            {"platform"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestDescribeEngineDefaultClusterParameters_DefaultFamily(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribeEngineDefaultClusterParameters"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "neptune1.3")
}

// TestCopyDBClusterParameterGroup_MissingSource verifies error on missing source.
func TestCopyDBClusterParameterGroup_MissingSource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"CopyDBClusterParameterGroup"},
		"Version": {"2014-10-31"},
		"SourceDBClusterParameterGroupIdentifier":  {"nonexistent"},
		"TargetDBClusterParameterGroupIdentifier":  {"target-pg"},
		"TargetDBClusterParameterGroupDescription": {"test"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

// TestCopyDBClusterParameterGroup_TargetAlreadyExists verifies duplicate error.
func TestCopyDBClusterParameterGroup_TargetAlreadyExists(t *testing.T) {
	t.Parallel()

	backend := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	hb := neptune.NewHandler(backend)

	backend.AddClusterParameterGroupInternal("src-cpg", "neptune1.3")
	backend.AddClusterParameterGroupInternal("existing-cpg", "neptune1.3")

	rr := doRequest(t, hb, url.Values{
		"Action":  {"CopyDBClusterParameterGroup"},
		"Version": {"2014-10-31"},
		"SourceDBClusterParameterGroupIdentifier":  {"src-cpg"},
		"TargetDBClusterParameterGroupIdentifier":  {"existing-cpg"},
		"TargetDBClusterParameterGroupDescription": {"test"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupAlreadyExists")
}

// TestDescribeDBClusterParameters_MissingGroup returns error.
func TestDescribeDBClusterParameters_MissingGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterParameters"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"nonexistent-cpg"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

// TestDescribeDBClusterParameters_Empty verifies that an empty group name is
// rejected: DBClusterParameterGroupName is a required field on the real
// DescribeDBClusterParametersInput, and this backend now actually looks the
// group up (see the parameter-value-store fix in parameter_catalog.go)
// rather than always answering with an empty parameter list regardless of
// the name given.
func TestDescribeDBClusterParameters_Empty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribeDBClusterParameters"},
		"Version": {"2014-10-31"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

// --- Cluster parameter group extended ops ---

func TestClusterParameterGroup_ModifyResetDescribeParams(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                      {"CreateDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"cpg-ext"},
		"DBParameterGroupFamily":      {"neptune1.3"},
		"Description":                 {"extended test"},
	})

	// modify
	rr := doRequest(t, h, url.Values{
		"Action":                      {"ModifyDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"cpg-ext"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "cpg-ext")

	// reset
	rr = doRequest(t, h, url.Values{
		"Action":                      {"ResetDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"cpg-ext"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "cpg-ext")

	// describe parameters
	rr = doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterParameters"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"cpg-ext"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "DescribeDBClusterParametersResponse")

	// describe parameters with unknown group returns error
	rr = doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterParameters"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"no-such-cpg"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

// TestResetDBClusterParameterGroup tests reset cluster parameter group.
func TestResetDBClusterParameterGroup(t *testing.T) {
	t.Parallel()

	backend := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	h := neptune.NewHandler(backend)
	backend.AddClusterParameterGroupInternal("cpg-reset", "neptune1.3")

	rr := doRequest(t, h, url.Values{
		"Action":                      {"ResetDBClusterParameterGroup"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"cpg-reset"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "cpg-reset")
}

// TestDescribeDBClusterParameterGroups_ByName tests describe by name.
func TestDescribeDBClusterParameterGroups_ByName(t *testing.T) {
	t.Parallel()

	backend := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	h := neptune.NewHandler(backend)
	backend.AddClusterParameterGroupInternal("my-cpg", "neptune1.3")

	rr := doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterParameterGroups"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"my-cpg"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "my-cpg")
}

// TestDescribeDBClusterParameterGroups_NotFound tests not found.
func TestDescribeDBClusterParameterGroups_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                      {"DescribeDBClusterParameterGroups"},
		"Version":                     {"2014-10-31"},
		"DBClusterParameterGroupName": {"nonexistent-cpg"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

// TestDBClusterParameterGroups_Pagination verifies Marker pagination works for cluster parameter groups.
func TestDBClusterParameterGroups_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxRecords string
		wantMarker bool
		wantCount  int
	}{
		{
			name:       "all_results_no_pagination",
			maxRecords: "10",
			wantMarker: false,
		},
		{
			name:       "paginate_with_max_records_1",
			maxRecords: "1",
			wantMarker: true,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			// Create 3 cluster parameter groups
			for i, name := range []string{"pg-alpha", "pg-beta", "pg-gamma"} {
				_ = i
				doRequest(t, h, url.Values{
					"Action":                      {"CreateDBClusterParameterGroup"},
					"Version":                     {"2014-10-31"},
					"DBClusterParameterGroupName": {name},
					"DBParameterGroupFamily":      {"neptune1.3"},
					"Description":                 {"test"},
				})
			}
			vals := url.Values{
				"Action":     {"DescribeDBClusterParameterGroups"},
				"Version":    {"2014-10-31"},
				"MaxRecords": {tt.maxRecords},
			}
			rr := doRequest(t, h, vals)
			require.Equal(t, http.StatusOK, rr.Code)
			body := rr.Body.String()
			if tt.wantMarker {
				assert.Contains(t, body, "<Marker>")
			} else {
				assert.NotContains(t, body, "<Marker>")
			}
		})
	}
}
