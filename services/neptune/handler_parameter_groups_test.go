package neptune_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/neptune"
)

func TestHandler_DBParameterGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_pg_success",
			vals: url.Values{
				"Action":                 {"CreateDBParameterGroup"},
				"Version":                {"2014-10-31"},
				"DBParameterGroupName":   {"test-param-group"},
				"DBParameterGroupFamily": {"neptune1.3"},
				"Description":            {"test parameter group"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "test-param-group",
		},
		{
			name: "create_pg_missing_name",
			vals: url.Values{
				"Action":                 {"CreateDBParameterGroup"},
				"Version":                {"2014-10-31"},
				"DBParameterGroupFamily": {"neptune1.3"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
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

func TestHandler_CopyDBParameterGroup(t *testing.T) {
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
					"Action":                 {"CreateDBParameterGroup"},
					"Version":                {"2014-10-31"},
					"DBParameterGroupName":   {"src-param-group"},
					"DBParameterGroupFamily": {"neptune1.3"},
					"Description":            {"source param group"},
				})
			},
			vals: url.Values{
				"Action":                            {"CopyDBParameterGroup"},
				"Version":                           {"2014-10-31"},
				"SourceDBParameterGroupIdentifier":  {"src-param-group"},
				"TargetDBParameterGroupIdentifier":  {"dst-param-group"},
				"TargetDBParameterGroupDescription": {"copy"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "dst-param-group",
		},
		{
			name: "copy_pg_source_not_found",
			vals: url.Values{
				"Action":                           {"CopyDBParameterGroup"},
				"Version":                          {"2014-10-31"},
				"SourceDBParameterGroupIdentifier": {"no-such-pg"},
				"TargetDBParameterGroupIdentifier": {"dst-pg"},
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

func TestCreateDBParameterGroup_InvalidFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		family       string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "invalid_family",
			family:       "aurora-mysql5.7",
			wantStatus:   http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "valid_neptune13",
			family:       "neptune1.3",
			wantStatus:   http.StatusOK,
			wantContains: "CreateDBParameterGroupResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, url.Values{
				"Action":                 {"CreateDBParameterGroup"},
				"Version":                {"2014-10-31"},
				"DBParameterGroupName":   {"pg-" + strings.ReplaceAll(tt.name, "_", "-")},
				"DBParameterGroupFamily": {tt.family},
				"Description":            {"test"},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// ---- DBParameterGroup comprehensive coverage ----

func TestDBParameterGroup_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create
	rr := doRequest(t, h, url.Values{
		"Action":                 {"CreateDBParameterGroup"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupName":   {"param-pg-1"},
		"DBParameterGroupFamily": {"neptune1.3"},
		"Description":            {"parameter group test"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "param-pg-1")
	assert.Contains(t, rr.Body.String(), "neptune1.3")

	// Describe
	rr = doRequest(t, h, url.Values{
		"Action":               {"DescribeDBParameterGroups"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"param-pg-1"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "param-pg-1")

	// Modify
	rr = doRequest(t, h, url.Values{
		"Action":               {"ModifyDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"param-pg-1"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "param-pg-1")

	// Reset
	rr = doRequest(t, h, url.Values{
		"Action":               {"ResetDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"param-pg-1"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "param-pg-1")

	// Describe parameters
	rr = doRequest(t, h, url.Values{
		"Action":               {"DescribeDBParameters"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"param-pg-1"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "DescribeDBParametersResponse")

	// Delete
	rr = doRequest(t, h, url.Values{
		"Action":               {"DeleteDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"param-pg-1"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify gone
	rr = doRequest(t, h, url.Values{
		"Action":               {"DescribeDBParameterGroups"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"param-pg-1"},
	})
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

func TestCopyDBParameterGroup_FieldsPreserved(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                 {"CreateDBParameterGroup"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupName":   {"src-pg"},
		"DBParameterGroupFamily": {"neptune1.2"},
		"Description":            {"source param group"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                            {"CopyDBParameterGroup"},
		"Version":                           {"2014-10-31"},
		"SourceDBParameterGroupIdentifier":  {"src-pg"},
		"TargetDBParameterGroupIdentifier":  {"dst-pg"},
		"TargetDBParameterGroupDescription": {"copy of source"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "dst-pg")
	assert.Contains(t, body, "neptune1.2")
	assert.Contains(t, body, "copy of source")
}

// ---- DescribeEngineDefaultClusterParameters ----

func TestDescribeEngineDefaultClusterParameters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                 {"DescribeEngineDefaultClusterParameters"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupFamily": {"neptune1.3"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "neptune1.3")
	assert.Contains(t, body, "DescribeEngineDefaultClusterParametersResponse")
}

// ---- Backend unit tests for DBInstance options ----

func TestBackend_CreateDBInstance_AllOptions(t *testing.T) {
	t.Parallel()

	b := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDBCluster(
		context.Background(),
		"inst-opts-cluster",
		"",
		0,
		neptune.DBClusterCreateOptions{},
	)
	require.NoError(t, err)

	opts := neptune.DBInstanceCreateOptions{
		DBParameterGroupName:            "custom-pg",
		PreferredMaintenanceWindow:      "wed:04:00-wed:05:00",
		PreferredBackupWindow:           "01:00-02:00",
		AvailabilityZone:                "us-east-1b",
		CopyTagsToSnapshot:              true,
		EnableIAMDatabaseAuthentication: true,
		PromotionTier:                   5,
		StorageEncrypted:                true,
	}
	inst, err := b.CreateDBInstance(
		context.Background(),
		"inst-opts",
		"inst-opts-cluster",
		"db.r5.xlarge",
		opts,
	)
	require.NoError(t, err)
	assert.Equal(t, "custom-pg", inst.DBParameterGroupName)
	assert.Equal(t, "wed:04:00-wed:05:00", inst.PreferredMaintenanceWindow)
	assert.Equal(t, "01:00-02:00", inst.PreferredBackupWindow)
	assert.Equal(t, "us-east-1b", inst.AvailabilityZone)
	assert.True(t, inst.CopyTagsToSnapshot)
	assert.True(t, inst.EnableIAMDatabaseAuthentication)
	assert.Equal(t, 5, inst.PromotionTier)
	assert.True(t, inst.StorageEncrypted)
}

func TestBackend_ModifyDBInstance_AllOptions(t *testing.T) {
	t.Parallel()

	b := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDBCluster(
		context.Background(),
		"mod-opts-cluster",
		"",
		0,
		neptune.DBClusterCreateOptions{},
	)
	require.NoError(t, err)
	_, err = b.CreateDBInstance(
		context.Background(),
		"mod-opts-inst",
		"mod-opts-cluster",
		"",
		neptune.DBInstanceCreateOptions{},
	)
	require.NoError(t, err)

	opts := neptune.DBInstanceModifyOptions{
		DBParameterGroupName:            "new-pg",
		PreferredMaintenanceWindow:      "fri:06:00-fri:07:00",
		PreferredBackupWindow:           "03:00-04:00",
		AutoMinorVersionUpgrade:         false,
		AutoMinorVersionUpgradeSet:      true,
		CopyTagsToSnapshot:              true,
		CopyTagsToSnapshotSet:           true,
		EnableIAMDatabaseAuthentication: true,
		IamAuthSet:                      true,
		PromotionTier:                   7,
		PromotionTierSet:                true,
	}
	inst, err := b.ModifyDBInstance(context.Background(), "mod-opts-inst", "db.r6g.4xlarge", opts)
	require.NoError(t, err)
	assert.Equal(t, "db.r6g.4xlarge", inst.DBInstanceClass)
	assert.Equal(t, "new-pg", inst.DBParameterGroupName)
	assert.Equal(t, "fri:06:00-fri:07:00", inst.PreferredMaintenanceWindow)
	assert.Equal(t, "03:00-04:00", inst.PreferredBackupWindow)
	assert.False(t, inst.AutoMinorVersionUpgrade)
	assert.True(t, inst.CopyTagsToSnapshot)
	assert.True(t, inst.EnableIAMDatabaseAuthentication)
	assert.Equal(t, 7, inst.PromotionTier)
}

// TestCopyDBParameterGroup_MissingSource verifies error on missing source param group.
func TestCopyDBParameterGroup_MissingSource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                            {"CopyDBParameterGroup"},
		"Version":                           {"2014-10-31"},
		"SourceDBParameterGroupIdentifier":  {"nonexistent"},
		"TargetDBParameterGroupIdentifier":  {"target-dpg"},
		"TargetDBParameterGroupDescription": {"test"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

// TestCreateDBParameterGroup_AlreadyExists verifies duplicate error.
func TestCreateDBParameterGroup_AlreadyExists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, url.Values{
		"Action":                 {"CreateDBParameterGroup"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupName":   {"dup-dpg"},
		"DBParameterGroupFamily": {"neptune1.3"},
		"Description":            {"test"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":                 {"CreateDBParameterGroup"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupName":   {"dup-dpg"},
		"DBParameterGroupFamily": {"neptune1.3"},
		"Description":            {"test"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupAlreadyExists")
}

// TestDescribeDBParameters_MissingGroup returns error.
func TestDescribeDBParameters_MissingGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBParameters"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"nonexistent-pg"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

// TestDescribeEngineDefaultClusterParameters_Family verifies requested family is reflected.
func TestDescribeEngineDefaultClusterParameters_Family(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                 {"DescribeEngineDefaultClusterParameters"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupFamily": {"neptune1.2"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "neptune1.2")
}

// TestDescribeEngineDefaultParameters_Family verifies requested family is reflected.
func TestDescribeEngineDefaultParameters_Family(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":                 {"DescribeEngineDefaultParameters"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupFamily": {"neptune1.4"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "neptune1.4")
}

// TestDescribeDBParameters_ExistingGroup succeeds.
func TestDescribeDBParameters_ExistingGroup(t *testing.T) {
	t.Parallel()

	backend := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	h := neptune.NewHandler(backend)
	backend.AddParameterGroupInternal("my-pg", "neptune1.3")

	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBParameters"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"my-pg"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
}

// --- DB parameter group lifecycle ---

func TestDBParameterGroup_FullLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// create
	rr := doRequest(t, h, url.Values{
		"Action":                 {"CreateDBParameterGroup"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupName":   {"pg-full"},
		"DBParameterGroupFamily": {"neptune1.3"},
		"Description":            {"test group"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "pg-full")

	// describe all
	rr = doRequest(t, h, url.Values{
		"Action":  {"DescribeDBParameterGroups"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "pg-full")

	// describe by name
	rr = doRequest(t, h, url.Values{
		"Action":               {"DescribeDBParameterGroups"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-full"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "pg-full")

	// describe parameters
	rr = doRequest(t, h, url.Values{
		"Action":               {"DescribeDBParameters"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-full"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "DescribeDBParametersResponse")

	// modify
	rr = doRequest(t, h, url.Values{
		"Action":               {"ModifyDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-full"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "pg-full")

	// reset
	rr = doRequest(t, h, url.Values{
		"Action":               {"ResetDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-full"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "pg-full")

	// delete
	rr = doRequest(t, h, url.Values{
		"Action":               {"DeleteDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-full"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	// delete again must fail
	rr = doRequest(t, h, url.Values{
		"Action":               {"DeleteDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-full"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBParameterGroupNotFound")
}

func TestDBParameterGroup_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		vals         url.Values
		wantContains string
	}{
		{
			name: "describe_not_found",
			vals: url.Values{
				"Action":               {"DescribeDBParameterGroups"},
				"Version":              {"2014-10-31"},
				"DBParameterGroupName": {"no-such"},
			},
			wantContains: "DBParameterGroupNotFound",
		},
		{
			name: "describe_params_not_found",
			vals: url.Values{
				"Action":               {"DescribeDBParameters"},
				"Version":              {"2014-10-31"},
				"DBParameterGroupName": {"no-such"},
			},
			wantContains: "DBParameterGroupNotFound",
		},
		{
			name: "modify_not_found",
			vals: url.Values{
				"Action":               {"ModifyDBParameterGroup"},
				"Version":              {"2014-10-31"},
				"DBParameterGroupName": {"no-such"},
			},
			wantContains: "DBParameterGroupNotFound",
		},
		{
			name: "reset_not_found",
			vals: url.Values{
				"Action":               {"ResetDBParameterGroup"},
				"Version":              {"2014-10-31"},
				"DBParameterGroupName": {"no-such"},
			},
			wantContains: "DBParameterGroupNotFound",
		},
		{
			name: "delete_not_found",
			vals: url.Values{
				"Action":               {"DeleteDBParameterGroup"},
				"Version":              {"2014-10-31"},
				"DBParameterGroupName": {"no-such"},
			},
			wantContains: "DBParameterGroupNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestDescribeEngineDefaultParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		family       string
		action       string
		wantFamily   string
		wantContains string
	}{
		{
			name:         "cluster_params_default_family",
			action:       "DescribeEngineDefaultClusterParameters",
			family:       "",
			wantContains: "DescribeEngineDefaultClusterParametersResponse",
			wantFamily:   "neptune1.3",
		},
		{
			name:         "cluster_params_explicit_family",
			action:       "DescribeEngineDefaultClusterParameters",
			family:       "neptune1.2",
			wantContains: "neptune1.2",
			wantFamily:   "neptune1.2",
		},
		{
			name:         "instance_params_default_family",
			action:       "DescribeEngineDefaultParameters",
			family:       "",
			wantContains: "DescribeEngineDefaultParametersResponse",
			wantFamily:   "neptune1.3",
		},
		{
			name:         "instance_params_explicit_family",
			action:       "DescribeEngineDefaultParameters",
			family:       "neptune1.1",
			wantContains: "neptune1.1",
			wantFamily:   "neptune1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			vals := url.Values{
				"Action":  {tt.action},
				"Version": {"2014-10-31"},
			}
			if tt.family != "" {
				vals["DBParameterGroupFamily"] = []string{tt.family}
			}
			rr := doRequest(t, h, vals)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), tt.wantContains)
			assert.Contains(t, rr.Body.String(), tt.wantFamily)
		})
	}
}

// --- DB Parameter Group operations ---

// TestCreateDescribeDeleteDBParameterGroup tests CRUD for parameter groups.
func TestCreateDescribeDeleteDBParameterGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create
	rr := doRequest(t, h, url.Values{
		"Action":                 {"CreateDBParameterGroup"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupName":   {"pg-01"},
		"DBParameterGroupFamily": {"neptune1.3"},
		"Description":            {"test"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "pg-01")

	// Describe
	rr = doRequest(t, h, url.Values{
		"Action":  {"DescribeDBParameterGroups"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "pg-01")

	// Modify
	rr = doRequest(t, h, url.Values{
		"Action":               {"ModifyDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-01"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	// Reset
	rr = doRequest(t, h, url.Values{
		"Action":               {"ResetDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-01"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	// Delete
	rr = doRequest(t, h, url.Values{
		"Action":               {"DeleteDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"pg-01"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
}

// TestHandler_DeleteDBParameterGroup_InUse asserts DeleteDBParameterGroup
// rejects a parameter group still associated with a DB instance
// (api_op_DeleteDBParameterGroup.go: "The DBParameterGroup to be deleted
// can't be associated with any DB instances.").
func TestHandler_DeleteDBParameterGroup_InUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, url.Values{
		"Action":                 {"CreateDBParameterGroup"},
		"Version":                {"2014-10-31"},
		"DBParameterGroupName":   {"used-pg"},
		"DBParameterGroupFamily": {"neptune1.3"},
		"Description":            {"in use"},
	})
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"pg-user-cluster"},
	})
	doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"pg-user-instance"},
		"DBClusterIdentifier":  {"pg-user-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
		"DBParameterGroupName": {"used-pg"},
	})
	// A second cluster member so deleting pg-user-instance below doesn't trip
	// the "can't delete the only instance in a cluster" guard.
	doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"pg-user-instance-sibling"},
		"DBClusterIdentifier":  {"pg-user-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
	})

	recEarly := doRequest(t, h, url.Values{
		"Action":               {"DeleteDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"used-pg"},
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)
	assert.Contains(t, recEarly.Body.String(), "InvalidDBParameterGroupState")

	recDeleteInstance := doRequest(t, h, url.Values{
		"Action":               {"DeleteDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"pg-user-instance"},
	})
	require.Equal(t, http.StatusOK, recDeleteInstance.Code)

	recDelete := doRequest(t, h, url.Values{
		"Action":               {"DeleteDBParameterGroup"},
		"Version":              {"2014-10-31"},
		"DBParameterGroupName": {"used-pg"},
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}
