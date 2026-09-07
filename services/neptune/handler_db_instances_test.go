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

func TestHandler_DBInstances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "create_instance",
			vals: url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"test-instance"},
				"DBClusterIdentifier":  {"inst-cluster"},
				"DBInstanceClass":      {"db.r5.large"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "test-instance",
		},
		{
			name: "describe_instances",
			vals: url.Values{
				"Action":  {"DescribeDBInstances"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DescribeDBInstancesResponse",
		},
		{
			name: "modify_instance",
			vals: url.Values{
				"Action":               {"ModifyDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"test-instance"},
				"DBInstanceClass":      {"db.r5.xlarge"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "db.r5.xlarge",
		},
		{
			name: "reboot_instance",
			vals: url.Values{
				"Action":               {"RebootDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"test-instance"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "RebootDBInstanceResponse",
		},
		{
			name: "delete_instance",
			vals: url.Values{
				"Action":               {"DeleteDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"test-instance"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "DeleteDBInstanceResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			createCluster(t, h, "inst-cluster")
			if tt.name != "create_instance" {
				createInstance(t, h, "test-instance", "inst-cluster")
			}
			if tt.name == "delete_instance" {
				// A cluster's last instance can't be deleted, so give it a sibling.
				createInstance(t, h, "test-instance-sibling", "inst-cluster")
			}
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestHandler_DeleteDBInstance_OnlyInstanceInCluster asserts DeleteDBInstance
// rejects deleting the only instance in a DB cluster (api_op_DeleteDBInstance.go:
// "You can't delete a DB instance if it is the only instance in the DB
// cluster").
func TestHandler_DeleteDBInstance_OnlyInstanceInCluster(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "solo-cluster")
	createInstance(t, h, "solo-inst", "solo-cluster")

	recEarly := doRequest(t, h, url.Values{
		"Action":               {"DeleteDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"solo-inst"},
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)
	assert.Contains(t, recEarly.Body.String(), "InvalidDBInstanceState")

	createInstance(t, h, "solo-inst-2", "solo-cluster")

	recDelete := doRequest(t, h, url.Values{
		"Action":               {"DeleteDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"solo-inst"},
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

// TestHandler_DeleteDBInstance_DeletionProtectionEnabled asserts DeleteDBInstance
// rejects deleting an instance with deletion protection enabled
// (api_op_DeleteDBInstance.go: "You can't delete a DB instance ... if it has
// deletion protection enabled", and types.DBInstance.DeletionProtection's own
// doc comment: "The instance can't be deleted when deletion protection is
// enabled.").
func TestHandler_DeleteDBInstance_DeletionProtectionEnabled(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "dp-cluster")
	// A sibling instance keeps the only-instance-in-cluster rule out of play.
	createInstance(t, h, "dp-inst-sibling", "dp-cluster")
	doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dp-inst"},
		"DBClusterIdentifier":  {"dp-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
		"DeletionProtection":   {"true"},
	})

	recProtected := doRequest(t, h, url.Values{
		"Action":               {"DeleteDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dp-inst"},
	})
	assert.Equal(t, http.StatusBadRequest, recProtected.Code)
	assert.Contains(t, recProtected.Body.String(), "InvalidDBInstanceState")

	doRequest(t, h, url.Values{
		"Action":               {"ModifyDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dp-inst"},
		"DeletionProtection":   {"false"},
	})

	recDelete := doRequest(t, h, url.Values{
		"Action":               {"DeleteDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dp-inst"},
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_DescribeEngineVersions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "describe_engine_versions",
			vals: url.Values{
				"Action":  {"DescribeDBEngineVersions"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "neptune",
		},
		{
			name: "describe_orderable_options",
			vals: url.Values{
				"Action":  {"DescribeOrderableDBInstanceOptions"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "db.r5.large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_DescribeDBInstances_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "test-cluster")

	for _, id := range []string{"inst-1", "inst-2"} {
		createInstance(t, h, id, "test-cluster")
	}

	tests := []struct {
		vals       url.Values
		name       string
		wantCode   int
		wantMarker bool
	}{
		{
			name: "all instances",
			vals: url.Values{
				"Action":  {"DescribeDBInstances"},
				"Version": {"2014-10-31"},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "paginated with MaxRecords=1",
			vals: url.Values{
				"Action":     {"DescribeDBInstances"},
				"Version":    {"2014-10-31"},
				"MaxRecords": {"1"},
			},
			wantCode:   http.StatusOK,
			wantMarker: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantCode, rr.Code)

			if tt.wantMarker {
				assert.Contains(t, rr.Body.String(), "<Marker>")
			}
		})
	}
}

func TestHandler_ModifyReboot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *neptune.Handler)
		vals     url.Values
		name     string
		wantBody string
		wantCode int
	}{
		{
			name: "modify cluster",
			vals: url.Values{
				"Action":              {"ModifyDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"mod-cluster"},
			},
			wantCode: http.StatusOK,
			wantBody: "ModifyDBClusterResponse",
		},
		{
			name: "stop cluster",
			vals: url.Values{
				"Action":              {"StopDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"mod-cluster"},
			},
			wantCode: http.StatusOK,
			wantBody: "StopDBClusterResponse",
		},
		{
			name: "start cluster",
			setup: func(h *neptune.Handler) {
				doRequest(t, h, url.Values{
					"Action":              {"StopDBCluster"},
					"Version":             {"2014-10-31"},
					"DBClusterIdentifier": {"mod-cluster"},
				})
			},
			vals: url.Values{
				"Action":              {"StartDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"mod-cluster"},
			},
			wantCode: http.StatusOK,
			wantBody: "StartDBClusterResponse",
		},
		{
			name: "failover cluster",
			setup: func(h *neptune.Handler) {
				createInstance(t, h, "mod-inst-2", "mod-cluster")
			},
			vals: url.Values{
				"Action":              {"FailoverDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"mod-cluster"},
			},
			wantCode: http.StatusOK,
			wantBody: "FailoverDBClusterResponse",
		},
		{
			name: "modify instance",
			vals: url.Values{
				"Action":               {"ModifyDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"mod-inst"},
				"DBInstanceClass":      {"db.r5.large"},
			},
			wantCode: http.StatusOK,
			wantBody: "ModifyDBInstanceResponse",
		},
		{
			name: "reboot instance",
			vals: url.Values{
				"Action":               {"RebootDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"mod-inst"},
			},
			wantCode: http.StatusOK,
			wantBody: "RebootDBInstanceResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createCluster(t, h, "mod-cluster")
			createInstance(t, h, "mod-inst", "mod-cluster")
			if tt.setup != nil {
				tt.setup(h)
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantCode, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantBody)
		})
	}
}

func TestHandler_DeleteClusterAndInstance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals     url.Values
		name     string
		wantBody string
		wantCode int
	}{
		{
			name: "delete instance",
			vals: url.Values{
				"Action":               {"DeleteDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"del-inst"},
			},
			wantCode: http.StatusOK,
			wantBody: "DeleteDBInstanceResponse",
		},
		{
			name: "delete cluster",
			vals: url.Values{
				"Action":              {"DeleteDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"del-cluster"},
				"SkipFinalSnapshot":   {"true"},
			},
			wantCode: http.StatusOK,
			wantBody: "DeleteDBClusterResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createCluster(t, h, "del-cluster")
			createInstance(t, h, "del-inst", "del-cluster")
			if tt.name == "delete instance" {
				// A cluster's last instance can't be deleted, so give it a sibling.
				createInstance(t, h, "del-inst-sibling", "del-cluster")
			}

			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, tt.wantCode, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantBody)
		})
	}
}

func TestHandler_ApplyPendingMaintenanceAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "apply_action_success",
			vals: url.Values{
				"Action":             {"ApplyPendingMaintenanceAction"},
				"Version":            {"2014-10-31"},
				"ResourceIdentifier": {"arn:aws:rds:us-east-1:000000000000:cluster:test"},
				"ApplyAction":        {"system-update"},
				"OptInType":          {"immediate"},
			},
			wantStatus:   http.StatusOK,
			wantContains: "ApplyPendingMaintenanceActionResponse",
		},
		{
			name: "apply_action_missing_resource",
			vals: url.Values{
				"Action":      {"ApplyPendingMaintenanceAction"},
				"Version":     {"2014-10-31"},
				"ApplyAction": {"system-update"},
				"OptInType":   {"immediate"},
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

// --- CreateDBInstance: DBClusterIdentifier required ---

func TestCreateDBInstance_MissingClusterID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"no-cluster-inst"},
		"DBInstanceClass":      {"db.r5.large"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "InvalidParameterValue")
}

// --- PromotionTier range validation ---

func TestPromotionTier_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		tier         string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "valid_tier_0",
			tier:         "0",
			wantStatus:   http.StatusOK,
			wantContains: "CreateDBInstanceResponse",
		},
		{
			name:         "valid_tier_15",
			tier:         "15",
			wantStatus:   http.StatusOK,
			wantContains: "CreateDBInstanceResponse",
		},
		{
			name:         "invalid_tier_16",
			tier:         "16",
			wantStatus:   http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
		{
			name:         "invalid_tier_negative",
			tier:         "-1",
			wantStatus:   http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createCluster(t, h, "tier-cluster")
			rr := doRequest(t, h, url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"tier-inst-" + tt.tier},
				"DBClusterIdentifier":  {"tier-cluster"},
				"DBInstanceClass":      {"db.r5.large"},
				"PromotionTier":        {tt.tier},
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

func TestModifyDBInstance_PromotionTierInvalid(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "mod-tier-cluster")
	createInstance(t, h, "mod-tier-inst", "mod-tier-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":               {"ModifyDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"mod-tier-inst"},
		"PromotionTier":        {"20"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "InvalidParameterValue")
}

// ---- DBInstance comprehensive coverage ----

func TestCreateDBInstance_AllFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "inst-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":                          {"CreateDBInstance"},
		"Version":                         {"2014-10-31"},
		"DBInstanceIdentifier":            {"inst-full"},
		"DBClusterIdentifier":             {"inst-cluster"},
		"DBInstanceClass":                 {"db.r6g.xlarge"},
		"Engine":                          {"neptune"},
		"PreferredMaintenanceWindow":      {"mon:05:00-mon:06:00"},
		"PreferredBackupWindow":           {"02:00-03:00"},
		"AvailabilityZone":                {"us-east-1a"},
		"AutoMinorVersionUpgrade":         {"true"},
		"CopyTagsToSnapshot":              {"true"},
		"EnableIAMDatabaseAuthentication": {"true"},
		"StorageEncrypted":                {"true"},
		"PromotionTier":                   {"2"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "inst-full")
	assert.Contains(t, body, "db.r6g.xlarge")
	assert.Contains(t, body, "mon:05:00-mon:06:00")
	assert.Contains(t, body, "us-east-1a")
	assert.Contains(t, body, "IAMDatabaseAuthenticationEnabled")
	assert.Contains(t, body, "CopyTagsToSnapshot")
}

func TestCreateDBInstance_FirstIsWriter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "writer-cluster")
	createInstance(t, h, "writer-inst", "writer-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":              {"DescribeDBClusters"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"writer-cluster"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "writer-inst")
	assert.Contains(t, body, "<IsClusterWriter>true</IsClusterWriter>")
}

func TestCreateDBInstance_SecondIsNotWriter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "multi-inst-cluster")
	createInstance(t, h, "inst-w", "multi-inst-cluster")
	createInstance(t, h, "inst-r", "multi-inst-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":              {"DescribeDBClusters"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"multi-inst-cluster"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "<IsClusterWriter>false</IsClusterWriter>")
}

func TestModifyDBInstance_AllFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "mod-inst-cluster")
	createInstance(t, h, "mod-inst", "mod-inst-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":                          {"ModifyDBInstance"},
		"Version":                         {"2014-10-31"},
		"DBInstanceIdentifier":            {"mod-inst"},
		"DBInstanceClass":                 {"db.r6g.2xlarge"},
		"PreferredMaintenanceWindow":      {"tue:06:00-tue:07:00"},
		"PreferredBackupWindow":           {"04:00-05:00"},
		"AutoMinorVersionUpgrade":         {"false"},
		"CopyTagsToSnapshot":              {"true"},
		"EnableIAMDatabaseAuthentication": {"true"},
		"PromotionTier":                   {"3"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "db.r6g.2xlarge")
	assert.Contains(t, body, "tue:06:00-tue:07:00")
	assert.Contains(t, body, "04:00-05:00")
}

func TestModifyDBInstance_IamAuth_Persist(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "iam-inst-cluster")
	createInstance(t, h, "iam-inst", "iam-inst-cluster")

	doRequest(t, h, url.Values{
		"Action":                          {"ModifyDBInstance"},
		"Version":                         {"2014-10-31"},
		"DBInstanceIdentifier":            {"iam-inst"},
		"EnableIAMDatabaseAuthentication": {"true"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBInstances"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"iam-inst"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "true")
}

func TestDBInstance_EnginVersionInheritsFromCluster(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	// Create cluster with specific version
	doRequest(t, h, url.Values{
		"Action":              {"CreateDBCluster"},
		"Version":             {"2014-10-31"},
		"DBClusterIdentifier": {"ver-cluster"},
		"EngineVersion":       {"1.2.0.0"},
	})

	createInstance(t, h, "ver-inst", "ver-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBInstances"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"ver-inst"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "1.2.0.0")
}

func TestDBInstance_EndpointFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "ep-cluster-inst")
	createInstance(t, h, "ep-inst", "ep-cluster-inst")

	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBInstances"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"ep-inst"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "ep-inst.neptune.us-east-1.amazonaws.com")
	assert.Contains(t, body, "8182")
}

// ---- ApplyPendingMaintenanceAction coverage ----

func TestApplyPendingMaintenanceAction_AllOptInTypes(t *testing.T) {
	t.Parallel()

	for _, optIn := range []string{"immediate", "next-maintenance", "undo-opt-in"} {
		t.Run(optIn, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, url.Values{
				"Action":             {"ApplyPendingMaintenanceAction"},
				"Version":            {"2014-10-31"},
				"ResourceIdentifier": {"arn:aws:rds:us-east-1:000000000000:cluster:test"},
				"ApplyAction":        {"system-update"},
				"OptInType":          {optIn},
			})
			require.Equal(t, http.StatusOK, rr.Code)
		})
	}
}

// ---- DescribeEngineVersions and DescribeOrderableOptions ----

func TestDescribeDBEngineVersions_AllFamilies(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribeDBEngineVersions"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "1.2.0.0")
	assert.Contains(t, body, "1.2.0.1")
	assert.Contains(t, body, "1.2.0.2")
	assert.Contains(t, body, "1.2.1.0")
	assert.Contains(t, body, "1.3.0.0")
	assert.Contains(t, body, "1.3.1.0")
	assert.Contains(t, body, "1.3.2.0")
	assert.Contains(t, body, "1.4.0.0")
	assert.Contains(t, body, "neptune1.2")
	assert.Contains(t, body, "neptune1.3")
	assert.Contains(t, body, "neptune1.4")
	assert.Contains(t, body, "Amazon Neptune")
}

func TestDescribeOrderableDBInstanceOptions_AllClasses(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribeOrderableDBInstanceOptions"},
		"Version": {"2014-10-31"},
		"Engine":  {"neptune"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "db.r5.large")
	assert.Contains(t, body, "db.r5.xlarge")
	assert.Contains(t, body, "db.r6g.large")
	assert.Contains(t, body, "db.t3.medium")
}

func TestBackend_ModifyDBInstance_IamNotSet_NoChange(t *testing.T) {
	t.Parallel()

	b := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDBCluster(
		context.Background(),
		"iam-noset-cluster",
		"",
		0,
		neptune.DBClusterCreateOptions{},
	)
	require.NoError(t, err)
	_, err = b.CreateDBInstance(
		context.Background(),
		"iam-noset-inst",
		"iam-noset-cluster",
		"",
		neptune.DBInstanceCreateOptions{
			EnableIAMDatabaseAuthentication: true,
		},
	)
	require.NoError(t, err)

	// Modify without IamAuthSet — should not change
	inst, err := b.ModifyDBInstance(
		context.Background(),
		"iam-noset-inst",
		"",
		neptune.DBInstanceModifyOptions{
			EnableIAMDatabaseAuthentication: false,
			IamAuthSet:                      false,
		},
	)
	require.NoError(t, err)
	assert.True(t, inst.EnableIAMDatabaseAuthentication)
}

func TestInstanceARN_Format(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "arn-inst-cluster")
	createInstance(t, h, "arn-inst", "arn-inst-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBInstances"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"arn-inst"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "arn:aws:neptune:us-east-1:000000000000:db:arn-inst")
}

func TestDeleteCluster_CascadesInstances(t *testing.T) {
	t.Parallel()

	b := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDBCluster(
		context.Background(),
		"cascade-inst-cluster",
		"",
		0,
		neptune.DBClusterCreateOptions{},
	)
	require.NoError(t, err)
	_, err = b.CreateDBInstance(
		context.Background(),
		"cascade-inst-1",
		"cascade-inst-cluster",
		"",
		neptune.DBInstanceCreateOptions{},
	)
	require.NoError(t, err)
	_, err = b.CreateDBInstance(
		context.Background(),
		"cascade-inst-2",
		"cascade-inst-cluster",
		"",
		neptune.DBInstanceCreateOptions{},
	)
	require.NoError(t, err)

	require.Equal(t, 2, neptune.InstanceCount(b))

	_, err = b.DeleteDBCluster(
		context.Background(),
		"cascade-inst-cluster",
		neptune.DBClusterDeleteOptions{SkipFinalSnapshot: true},
	)
	require.NoError(t, err)

	require.Equal(t, 0, neptune.InstanceCount(b))
	require.Equal(t, 0, neptune.ClusterCount(b))
}

// TestCloneCluster_NoSharedSlice verifies deep copy of DBClusterMembers.
func TestCloneCluster_NoSharedSlice(t *testing.T) {
	t.Parallel()

	backend := neptune.NewInMemoryBackend("000000000000", "us-east-1")
	h := neptune.NewHandler(backend)

	createCluster(t, h, "member-cluster")
	createInstance(t, h, "member-inst", "member-cluster")

	clusters, err := backend.DescribeDBClusters(context.Background(), "member-cluster", neptune.DBClusterFilters{})
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	require.Len(t, clusters[0].DBClusterMembers, 1)

	// Mutate the returned copy — should not affect stored state.
	clusters[0].DBClusterMembers[0].DBInstanceIdentifier = "mutated"

	clusters2, err := backend.DescribeDBClusters(context.Background(), "member-cluster", neptune.DBClusterFilters{})
	require.NoError(t, err)
	assert.NotEqual(t, "mutated", clusters2[0].DBClusterMembers[0].DBInstanceIdentifier)
}

// TestApplyPendingMaintenanceAction_MissingFields verifies validation.
func TestApplyPendingMaintenanceAction_MissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		vals        url.Values
		wantContain string
	}{
		{
			name: "missing_resource_id",
			vals: url.Values{
				"Action":      {"ApplyPendingMaintenanceAction"},
				"Version":     {"2014-10-31"},
				"ApplyAction": {"system-update"},
				"OptInType":   {"immediate"},
			},
			wantContain: "InvalidParameterValue",
		},
		{
			name: "missing_apply_action",
			vals: url.Values{
				"Action":             {"ApplyPendingMaintenanceAction"},
				"Version":            {"2014-10-31"},
				"ResourceIdentifier": {"arn:aws:rds:us-east-1:000000000000:cluster:test"},
				"OptInType":          {"immediate"},
			},
			wantContain: "InvalidParameterValue",
		},
		{
			name: "missing_opt_in_type",
			vals: url.Values{
				"Action":             {"ApplyPendingMaintenanceAction"},
				"Version":            {"2014-10-31"},
				"ResourceIdentifier": {"arn:aws:rds:us-east-1:000000000000:cluster:test"},
				"ApplyAction":        {"system-update"},
			},
			wantContain: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContain)
		})
	}
}

// TestInstanceHasArn verifies CreateDBInstance response includes DBInstanceArn.
func TestInstanceHasArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "inst-arn-cluster")
	rr := doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"inst-arn-instance"},
		"DBClusterIdentifier":  {"inst-arn-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "<DBInstanceArn>")
}

// TestCreateDBInstance_ClusterNotFound verifies error when cluster not found.
func TestCreateDBInstance_ClusterNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"inst-nocluster"},
		"DBClusterIdentifier":  {"nonexistent-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBClusterNotFoundFault")
}

// TestDescribeDBEngineVersions_MoreVersions verifies more engine versions returned.
func TestDescribeDBEngineVersions_MoreVersions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribeDBEngineVersions"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "1.2.0.0")
	assert.Contains(t, body, "1.3.0.0")
	assert.Contains(t, body, "1.3.1.0")
	assert.Contains(t, body, "1.4.0.0")
	assert.Contains(t, body, "neptune1.3")
}

// TestDescribeOrderableDBInstanceOptions_MoreOptions verifies comprehensive instance classes.
func TestDescribeOrderableDBInstanceOptions_MoreOptions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribeOrderableDBInstanceOptions"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "db.r5.large")
	assert.Contains(t, body, "db.r6g.large")
	assert.Contains(t, body, "db.t3.medium")
	assert.Contains(t, body, "db.r5.4xlarge")
}

// TestDescribeOrderableDBInstanceOptions_Filters proves the Engine/EngineVersion/
// DBInstanceClass request filters actually narrow the static catalog instead of
// always returning every row regardless of what was asked for.
func TestDescribeOrderableDBInstanceOptions_Filters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals            url.Values
		name            string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "instance_class_filter_narrows",
			vals: url.Values{
				"Action":          {"DescribeOrderableDBInstanceOptions"},
				"Version":         {"2014-10-31"},
				"DBInstanceClass": {"db.t3.medium"},
			},
			wantContains:    []string{"db.t3.medium"},
			wantNotContains: []string{"db.r5.large", "db.r6g.large"},
		},
		{
			name: "engine_version_filter_narrows",
			vals: url.Values{
				"Action":        {"DescribeOrderableDBInstanceOptions"},
				"Version":       {"2014-10-31"},
				"EngineVersion": {"1.4.0.0"},
			},
			wantContains:    []string{"1.4.0.0"},
			wantNotContains: []string{"1.2.0.0", "1.3.0.0"},
		},
		{
			name: "unknown_engine_returns_empty",
			vals: url.Values{
				"Action":  {"DescribeOrderableDBInstanceOptions"},
				"Version": {"2014-10-31"},
				"Engine":  {"mysql"},
			},
			wantNotContains: []string{"db.t3.medium", "db.r5.large", "OrderableDBInstanceOption>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			body := rr.Body.String()
			for _, want := range tt.wantContains {
				assert.Contains(t, body, want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, body, notWant)
			}
		})
	}
}

// TestDescribeValidDBInstanceModifications_RequiresRealInstance proves
// DBInstanceIdentifier -- a required input on this op -- is now genuinely
// validated: missing or unknown identifiers are rejected instead of silently
// answering 200 with fabricated data (reverting the handler's existence check
// makes this fail).
func TestDescribeValidDBInstanceModifications_RequiresRealInstance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals         url.Values
		name         string
		wantContains string
		wantStatus   int
	}{
		{
			name: "missing_identifier",
			vals: url.Values{
				"Action":  {"DescribeValidDBInstanceModifications"},
				"Version": {"2014-10-31"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBInstanceNotFound",
		},
		{
			name: "unknown_identifier",
			vals: url.Values{
				"Action":               {"DescribeValidDBInstanceModifications"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"does-not-exist"},
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "DBInstanceNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rr := doRequest(t, h, tt.vals)
			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestDescribeValidDBInstanceModifications_RealInstance proves a real
// instance's identifier is accepted and the response uses the real
// ValidDBInstanceModificationsMessage.Storage wire shape, not the previous
// fabricated ValidProcessorFeatures/DB-instance-class list (which used an
// XML element name the real SDK deserializer never recognizes).
func TestDescribeValidDBInstanceModifications_RealInstance(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "vdim-cluster")
	createInstance(t, h, "vdim-instance", "vdim-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeValidDBInstanceModifications"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"vdim-instance"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	body := rr.Body.String()
	assert.Contains(t, body, "DescribeValidDBInstanceModificationsResponse")
	assert.Contains(t, body, "ValidDBInstanceModificationsMessage")
	assert.NotContains(t, body, "ValidProcessorFeatures")
	assert.NotContains(t, body, "db.r5.large")
}

// TestInstanceEngineVersionInheritsFromCluster verifies instance engine version from cluster.
func TestInstanceEngineVersionInheritsFromCluster(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "ev-inherit-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"ev-inherit-inst"},
		"DBClusterIdentifier":  {"ev-inherit-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "<EngineVersion>")
	assert.Contains(t, rr.Body.String(), "1.3.0.0")
}

// TestCreateDBInstance_Engine proves a non-"neptune" Engine value is rejected
// instead of being silently dropped and ignored (reverting the handler's
// Engine check makes the "wrong_engine_rejected" case fail).
func TestCreateDBInstance_Engine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		id           string
		engine       string
		wantContains string
		wantStatus   int
	}{
		{
			name:         "omitted_engine_accepted",
			id:           "engine-inst-omitted",
			engine:       "",
			wantStatus:   http.StatusOK,
			wantContains: "<Engine>neptune</Engine>",
		},
		{
			name:         "correct_engine_accepted",
			id:           "engine-inst-correct",
			engine:       "neptune",
			wantStatus:   http.StatusOK,
			wantContains: "<Engine>neptune</Engine>",
		},
		{
			name:         "wrong_engine_rejected",
			id:           "engine-inst-wrong",
			engine:       "mysql",
			wantStatus:   http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createCluster(t, h, "engine-cluster-"+tt.id)

			vals := url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {tt.id},
				"DBClusterIdentifier":  {"engine-cluster-" + tt.id},
				"DBInstanceClass":      {"db.r5.large"},
			}
			if tt.engine != "" {
				vals.Set("Engine", tt.engine)
			}

			rr := doRequest(t, h, vals)
			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestInstanceHasMaintenanceWindow verifies PreferredMaintenanceWindow is returned.
func TestInstanceHasMaintenanceWindow(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "mw-cluster")

	rr := doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"mw-inst"},
		"DBClusterIdentifier":  {"mw-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "<PreferredMaintenanceWindow>")
}

// --- Stub ops ---

func TestDescribePendingMaintenanceActions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":  {"DescribePendingMaintenanceActions"},
		"Version": {"2014-10-31"},
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "DescribePendingMaintenanceActionsResponse")
}

// TestDescribeDBInstances_ByID tests single-instance lookup.
func TestDescribeDBInstances_ByID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "inst-byid-cluster")
	doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"inst-byid"},
		"DBClusterIdentifier":  {"inst-byid-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
	})

	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBInstances"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"inst-byid"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "inst-byid")
}

// TestDescribeDBInstances_NotFound tests not found error.
func TestDescribeDBInstances_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":               {"DescribeDBInstances"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"nonexistent-inst"},
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "DBInstanceNotFound")
}

// TestModifyRebootDBInstance tests instance modify and reboot.
func TestModifyRebootDBInstance(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCluster(t, h, "reboot-cluster")
	doRequest(t, h, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"reboot-inst"},
		"DBClusterIdentifier":  {"reboot-cluster"},
		"DBInstanceClass":      {"db.r5.large"},
	})

	// Modify
	rr := doRequest(t, h, url.Values{
		"Action":               {"ModifyDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"reboot-inst"},
		"DBInstanceClass":      {"db.r5.xlarge"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	// Reboot
	rr = doRequest(t, h, url.Values{
		"Action":               {"RebootDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"reboot-inst"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
}

// TestApplyPendingMaintenanceAction tests apply pending maintenance.
func TestApplyPendingMaintenanceAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rr := doRequest(t, h, url.Values{
		"Action":             {"ApplyPendingMaintenanceAction"},
		"Version":            {"2014-10-31"},
		"ResourceIdentifier": {"arn:aws:rds:us-east-1:000000000000:cluster:test"},
		"ApplyAction":        {"system-update"},
		"OptInType":          {"immediate"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
}

// TestDescribeDBInstances_SortedDeterministically verifies instances are returned sorted.
func TestDescribeDBInstances_SortedDeterministically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		instanceIDs   []string
		wantOrderedIn []string
	}{
		{
			name:          "sorted_alphabetically",
			instanceIDs:   []string{"inst-z", "inst-a", "inst-m"},
			wantOrderedIn: []string{"inst-a", "inst-m", "inst-z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			doRequest(t, h, url.Values{
				"Action":              {"CreateDBCluster"},
				"Version":             {"2014-10-31"},
				"DBClusterIdentifier": {"sort-cluster"},
			})
			for _, id := range tt.instanceIDs {
				doRequest(t, h, url.Values{
					"Action":               {"CreateDBInstance"},
					"Version":              {"2014-10-31"},
					"DBInstanceIdentifier": {id},
					"DBClusterIdentifier":  {"sort-cluster"},
					"DBInstanceClass":      {"db.r5.large"},
				})
			}
			rr := doRequest(t, h, url.Values{
				"Action":  {"DescribeDBInstances"},
				"Version": {"2014-10-31"},
			})
			require.Equal(t, http.StatusOK, rr.Code)
			body := rr.Body.String()
			prevPos := -1
			for _, want := range tt.wantOrderedIn {
				pos := strings.Index(body, want)
				assert.Greater(t, pos, prevPos, "instance %q should appear in order", want)
				prevPos = pos
			}
		})
	}
}

// TestErrorCodes_DBInstanceFaultSuffix verifies that DBInstance error codes use the Fault suffix.
func TestErrorCodes_DBInstanceFaultSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		vals         url.Values
		wantContains string
	}{
		{
			name: "instance_not_found_has_fault_suffix",
			vals: url.Values{
				"Action":               {"DescribeDBInstances"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"nonexistent-instance"},
			},
			wantContains: "DBInstanceNotFound",
		},
		{
			name: "instance_already_exists_has_fault_suffix",
			vals: url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {"dup-inst"},
				"DBClusterIdentifier":  {"dup-cluster"},
				"DBInstanceClass":      {"db.r5.large"},
			},
			wantContains: "DBInstanceAlreadyExists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tt.name == "instance_already_exists_has_fault_suffix" {
				doRequest(t, h, url.Values{
					"Action":              {"CreateDBCluster"},
					"Version":             {"2014-10-31"},
					"DBClusterIdentifier": {"dup-cluster"},
				})
				// Create the instance first
				doRequest(t, h, url.Values{
					"Action":               {"CreateDBInstance"},
					"Version":              {"2014-10-31"},
					"DBInstanceIdentifier": {"dup-inst"},
					"DBClusterIdentifier":  {"dup-cluster"},
					"DBInstanceClass":      {"db.r5.large"},
				})
			}
			rr := doRequest(t, h, tt.vals)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantContains)
		})
	}
}

// TestHandler_CreateDBInstance_DBSubnetGroupName covers gopherstack-ucus: an
// explicit DBSubnetGroupName on CreateDBInstance must be honored rather than
// silently overwritten by the cluster's subnet group, and an omitted one
// must still default to the cluster's (api_op_CreateDBInstance.go's
// DBSubnetGroupName doc comment does not spell out a default; the cluster
// inheritance below is this backend's pre-existing interpretation, since
// every Neptune instance belongs to a cluster).
func TestHandler_CreateDBInstance_DBSubnetGroupName(t *testing.T) {
	t.Parallel()

	t.Run("explicit_name_overrides_cluster", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		doRequest(t, h, url.Values{
			"Action":            {"CreateDBSubnetGroup"},
			"Version":           {"2014-10-31"},
			"DBSubnetGroupName": {"cluster-sg"},
		})
		doRequest(t, h, url.Values{
			"Action":            {"CreateDBSubnetGroup"},
			"Version":           {"2014-10-31"},
			"DBSubnetGroupName": {"instance-sg"},
		})
		doRequest(t, h, url.Values{
			"Action":              {"CreateDBCluster"},
			"Version":             {"2014-10-31"},
			"DBClusterIdentifier": {"explicit-sg-cluster"},
			"DBSubnetGroupName":   {"cluster-sg"},
		})
		rr := doRequest(t, h, url.Values{
			"Action":               {"CreateDBInstance"},
			"Version":              {"2014-10-31"},
			"DBInstanceIdentifier": {"explicit-sg-inst"},
			"DBClusterIdentifier":  {"explicit-sg-cluster"},
			"DBInstanceClass":      {"db.r5.large"},
			"DBSubnetGroupName":    {"instance-sg"},
		})
		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, "<DBSubnetGroup>instance-sg</DBSubnetGroup>")
		assert.NotContains(t, body, "<DBSubnetGroup>cluster-sg</DBSubnetGroup>")
	})

	t.Run("omitted_defaults_to_cluster", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		doRequest(t, h, url.Values{
			"Action":            {"CreateDBSubnetGroup"},
			"Version":           {"2014-10-31"},
			"DBSubnetGroupName": {"default-sg"},
		})
		doRequest(t, h, url.Values{
			"Action":              {"CreateDBCluster"},
			"Version":             {"2014-10-31"},
			"DBClusterIdentifier": {"default-sg-cluster"},
			"DBSubnetGroupName":   {"default-sg"},
		})
		rr := doRequest(t, h, url.Values{
			"Action":               {"CreateDBInstance"},
			"Version":              {"2014-10-31"},
			"DBInstanceIdentifier": {"default-sg-inst"},
			"DBClusterIdentifier":  {"default-sg-cluster"},
			"DBInstanceClass":      {"db.r5.large"},
		})
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "<DBSubnetGroup>default-sg</DBSubnetGroup>")
	})

	t.Run("nonexistent_name_rejected", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		createCluster(t, h, "sg-nf-cluster")
		rr := doRequest(t, h, url.Values{
			"Action":               {"CreateDBInstance"},
			"Version":              {"2014-10-31"},
			"DBInstanceIdentifier": {"sg-nf-inst"},
			"DBClusterIdentifier":  {"sg-nf-cluster"},
			"DBInstanceClass":      {"db.r5.large"},
			"DBSubnetGroupName":    {"does-not-exist-sg"},
		})
		require.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "DBSubnetGroupNotFoundFault")
	})
}
