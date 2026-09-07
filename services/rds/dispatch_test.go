package rds_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rdssdk "github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/rds"
)

func TestHandlerOpsLen(t *testing.T) {
	t.Parallel()

	h := newBatch3Handler()
	assert.Equal(t, 165, rds.HandlerOpsLen(h))
}

func TestSupportedOperations_ContainsExtendedOps(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	h := rds.NewHandler(b)

	ops := h.GetSupportedOperations()
	assert.Contains(t, ops, "DescribeServerlessV2PlatformVersions")
	assert.Contains(t, ops, "GetPerformanceInsightsMetrics")
	assert.Contains(t, ops, "CreateBlueGreenDeployment")
	assert.Contains(t, ops, "CreateDBShardGroup")
	assert.Contains(t, ops, "CreateIntegration")
}

// TestAccountIDAndRegion verifies AccountID() and Region() on the backend.
func TestAccountIDAndRegion(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("123456789012", "eu-west-1")

	assert.Equal(t, "123456789012", b.AccountID())
	assert.Equal(t, "eu-west-1", b.Region())
}

// TestHandlerReset verifies that Handler.Reset() clears all backend state.
func TestHandlerReset(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	h := rds.NewHandler(b)
	_, err := b.CreateDBInstance("inst-1", "mysql", "", "", "", "", 0, rds.DBInstanceOptions{})
	require.NoError(t, err)

	require.Equal(t, 1, rds.InstanceCount(b))

	h.Reset()

	assert.Equal(t, 0, rds.InstanceCount(b))
}

// TestHandlerOpsLen_FreshBackend verifies GetSupportedOperations has the expected count.
func TestHandlerOpsLen_FreshBackend(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	h := rds.NewHandler(b)

	assert.Equal(t, 165, rds.HandlerOpsLen(h))
}

// TestGetSupportedOperationsSorted verifies that GetSupportedOperations is alphabetically sorted.
func TestGetSupportedOperationsSorted(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("000000000000", "us-east-1")
	h := rds.NewHandler(b)
	ops := h.GetSupportedOperations()

	for i := 1; i < len(ops); i++ {
		assert.LessOrEqual(t, ops[i-1], ops[i],
			"GetSupportedOperations not sorted: %s > %s", ops[i-1], ops[i])
	}
}

// TestStorageBackendInterface verifies InMemoryBackend implements StorageBackend.
func TestStorageBackendInterface(t *testing.T) {
	t.Parallel()

	var _ rds.StorageBackend = (*rds.InMemoryBackend)(nil)
}

// TestProviderNilAppContext verifies Init returns error for nil AppContext.
func TestProviderNilAppContext(t *testing.T) {
	t.Parallel()

	p := &rds.Provider{}
	_, err := p.Init(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, rds.ErrNilAppContext)
}

func TestRDSHandler_Name(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	assert.Equal(t, "RDS", h.Name())
}

func TestRDSHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	ops := h.GetSupportedOperations()
	assert.Contains(t, ops, "CreateDBInstance")
	assert.Contains(t, ops, "DeleteDBInstance")
	assert.Contains(t, ops, "DescribeDBInstances")
	assert.Contains(t, ops, "ModifyDBInstance")
	assert.Contains(t, ops, "CreateDBSnapshot")
	assert.Contains(t, ops, "DescribeDBSnapshots")
	assert.Contains(t, ops, "DeleteDBSnapshot")
	assert.Contains(t, ops, "CreateDBSubnetGroup")
	assert.Contains(t, ops, "DescribeDBSubnetGroups")
	assert.Contains(t, ops, "DeleteDBSubnetGroup")
	assert.Contains(t, ops, "CreateDBParameterGroup")
	assert.Contains(t, ops, "CreateOptionGroup")
	assert.Contains(t, ops, "CreateDBCluster")
	assert.Contains(t, ops, "CreateDBInstanceReadReplica")
	assert.Contains(t, ops, "RebootDBInstance")
	assert.Contains(t, ops, "DescribeDBEngineVersions")
}

func TestRDSHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	assert.Equal(t, 84, h.MatchPriority())
}

func TestRDSHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	matcher := h.RouteMatcher()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   bool
	}{
		{
			name:   "valid RDS request",
			method: http.MethodPost,
			path:   "/",
			body:   "Version=2014-10-31&Action=DescribeDBInstances",
			want:   true,
		},
		{
			name:   "wrong version",
			method: http.MethodPost,
			path:   "/",
			body:   "Version=2012-12-01&Action=DescribeDBInstances",
			want:   false,
		},
		{
			name:   "GET request",
			method: http.MethodGet,
			path:   "/",
			want:   false,
		},
		{
			name:   "dashboard path",
			method: http.MethodPost,
			path:   "/dashboard/rds",
			body:   "Version=2014-10-31",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.want, matcher(c))
		})
	}
}

func TestRDSHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Action=CreateDBInstance&Version=2014-10-31"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := e.NewContext(req, httptest.NewRecorder())

	assert.Equal(t, "CreateDBInstance", h.ExtractOperation(c))
}

func TestRDSHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader("DBInstanceIdentifier=my-db&Version=2014-10-31"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := e.NewContext(req, httptest.NewRecorder())

	assert.Equal(t, "my-db", h.ExtractResource(c))
}

// mockDNSRegistrar is a test double for rds.DNSRegistrar.
type mockDNSRegistrar struct {
	registered map[string]bool
}

func (m *mockDNSRegistrar) Register(hostname string) {
	m.registered[hostname] = true
}

func (m *mockDNSRegistrar) Deregister(hostname string) {
	delete(m.registered, hostname)
}

func TestRDSBackend_DNSRegistrar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		instanceID     string
		wantRegistered bool
		deleteAfter    bool
	}{
		{
			name:           "registers_on_create",
			instanceID:     "my-db",
			wantRegistered: true,
		},
		{
			name:           "deregisters_on_delete",
			instanceID:     "del-db",
			deleteAfter:    true,
			wantRegistered: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registrar := &mockDNSRegistrar{registered: make(map[string]bool)}
			b := rds.NewInMemoryBackend("000000000000", "us-east-1")
			b.SetDNSRegistrar(registrar)

			inst, err := b.CreateDBInstance(tt.instanceID, "postgres", "", "", "", "", 0, rds.DBInstanceOptions{})
			require.NoError(t, err)

			if tt.deleteAfter {
				_, err = b.DeleteDBInstance(tt.instanceID)
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantRegistered, registrar.registered[inst.Endpoint])
		})
	}
}

// TestRDSHandler_NewOperations2 tests the HTTP handlers for the 10 new operations.
func TestRDSHandler_NewOperations2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            string
		setupBodies     []string
		wantContains    []string
		wantNotContains []string
		wantCode        int
	}{
		// AddRoleToDBCluster
		{
			name: "AddRoleToDBCluster_success",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=role-cluster&Engine=aurora-postgresql",
			},
			body: "Action=AddRoleToDBCluster&Version=2014-10-31" +
				"&DBClusterIdentifier=role-cluster&RoleArn=arn:aws:iam::000000000000:role/MyRole",
			wantCode:     http.StatusOK,
			wantContains: []string{"AddRoleToDBClusterResponse"},
		},
		{
			name: "AddRoleToDBCluster_cluster_not_found",
			body: "Action=AddRoleToDBCluster&Version=2014-10-31" +
				"&DBClusterIdentifier=no-such-cluster&RoleArn=arn:aws:iam::000000000000:role/MyRole",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name: "AddRoleToDBCluster_missing_role_arn",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=role-cluster2",
			},
			body:         "Action=AddRoleToDBCluster&Version=2014-10-31&DBClusterIdentifier=role-cluster2&RoleArn=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// AddRoleToDBInstance
		{
			name: "AddRoleToDBInstance_success",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=role-inst&Engine=postgres",
			},
			body: "Action=AddRoleToDBInstance&Version=2014-10-31" +
				"&DBInstanceIdentifier=role-inst&RoleArn=arn:aws:iam::000000000000:role/MyRole" +
				"&FeatureName=S3_INTEGRATION",
			wantCode:     http.StatusOK,
			wantContains: []string{"AddRoleToDBInstanceResponse"},
		},
		{
			name: "AddRoleToDBInstance_not_found",
			body: "Action=AddRoleToDBInstance&Version=2014-10-31" +
				"&DBInstanceIdentifier=no-such-db&RoleArn=arn:aws:iam::000000000000:role/MyRole" +
				"&FeatureName=S3_INTEGRATION",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		// AddSourceIdentifierToSubscription
		{
			name: "AddSourceIdentifierToSubscription_success",
			body: "Action=AddSourceIdentifierToSubscription&Version=2014-10-31" +
				"&SubscriptionName=my-sub&SourceIdentifier=my-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"AddSourceIdentifierToSubscriptionResponse", "my-sub"},
		},
		{
			name: "AddSourceIdentifierToSubscription_empty_subscription",
			body: "Action=AddSourceIdentifierToSubscription&Version=2014-10-31" +
				"&SubscriptionName=&SourceIdentifier=my-db",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// ApplyPendingMaintenanceAction
		{
			name: "ApplyPendingMaintenanceAction_success_instance",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=maint-db&Engine=postgres",
			},
			body: "Action=ApplyPendingMaintenanceAction&Version=2014-10-31" +
				"&ResourceIdentifier=maint-db&ApplyAction=system-update&OptInType=immediate",
			wantCode:     http.StatusOK,
			wantContains: []string{"ApplyPendingMaintenanceActionResponse"},
		},
		{
			name: "ApplyPendingMaintenanceAction_not_found",
			body: "Action=ApplyPendingMaintenanceAction&Version=2014-10-31" +
				"&ResourceIdentifier=no-such-resource&ApplyAction=system-update&OptInType=immediate",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ResourceNotFoundFault"},
		},
		{
			name:         "ApplyPendingMaintenanceAction_empty_resource_id",
			body:         "Action=ApplyPendingMaintenanceAction&Version=2014-10-31&ApplyAction=system-update",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "ApplyPendingMaintenanceAction_missing_opt_in_type",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=maint-db-noopt&Engine=postgres",
			},
			body: "Action=ApplyPendingMaintenanceAction&Version=2014-10-31" +
				"&ResourceIdentifier=maint-db-noopt&ApplyAction=system-update",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "ApplyPendingMaintenanceAction_invalid_opt_in_type",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=maint-db-badopt&Engine=postgres",
			},
			body: "Action=ApplyPendingMaintenanceAction&Version=2014-10-31" +
				"&ResourceIdentifier=maint-db-badopt&ApplyAction=system-update&OptInType=whenever",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// AuthorizeDBSecurityGroupIngress
		{
			name: "AuthorizeDBSecurityGroupIngress_success",
			body: "Action=AuthorizeDBSecurityGroupIngress&Version=2014-10-31" +
				"&DBSecurityGroupName=my-sg&CIDRIP=10.0.0.0/8",
			wantCode:     http.StatusOK,
			wantContains: []string{"AuthorizeDBSecurityGroupIngressResponse", "my-sg", "10.0.0.0/8"},
		},
		{
			name: "AuthorizeDBSecurityGroupIngress_empty_group",
			body: "Action=AuthorizeDBSecurityGroupIngress&Version=2014-10-31" +
				"&DBSecurityGroupName=&CIDRIP=10.0.0.0/8",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// BacktrackDBCluster
		{
			name: "BacktrackDBCluster_success",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=bt-cluster&Engine=aurora-mysql",
			},
			body: "Action=BacktrackDBCluster&Version=2014-10-31" +
				"&DBClusterIdentifier=bt-cluster&BacktrackTo=2024-01-01T00%3A00%3A00Z",
			wantCode:     http.StatusOK,
			wantContains: []string{"BacktrackDBClusterResponse", "bt-cluster"},
		},
		{
			name: "BacktrackDBCluster_cluster_not_found",
			body: "Action=BacktrackDBCluster&Version=2014-10-31" +
				"&DBClusterIdentifier=no-such-cluster&BacktrackTo=2024-01-01T00:00:00Z",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		// CopyDBClusterParameterGroup
		{
			name: "CopyDBClusterParameterGroup_success",
			setupBodies: []string{
				"Action=CreateDBClusterParameterGroup&Version=2014-10-31" +
					"&DBClusterParameterGroupName=src-cpg&DBParameterGroupFamily=aurora-postgresql14&Description=Source",
			},
			body: "Action=CopyDBClusterParameterGroup&Version=2014-10-31" +
				"&SourceDBClusterParameterGroupIdentifier=src-cpg" +
				"&TargetDBClusterParameterGroupIdentifier=dst-cpg" +
				"&TargetDBClusterParameterGroupDescription=Copied",
			wantCode:     http.StatusOK,
			wantContains: []string{"CopyDBClusterParameterGroupResponse", "dst-cpg"},
		},
		{
			name: "CopyDBClusterParameterGroup_source_not_found",
			body: "Action=CopyDBClusterParameterGroup&Version=2014-10-31" +
				"&SourceDBClusterParameterGroupIdentifier=no-such-cpg" +
				"&TargetDBClusterParameterGroupIdentifier=dst-cpg",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBParameterGroupNotFound"},
		},
		// CopyDBParameterGroup
		{
			name: "CopyDBParameterGroup_success",
			setupBodies: []string{
				"Action=CreateDBParameterGroup&Version=2014-10-31" +
					"&DBParameterGroupName=src-pg&DBParameterGroupFamily=postgres14&Description=Source",
			},
			body: "Action=CopyDBParameterGroup&Version=2014-10-31" +
				"&SourceDBParameterGroupIdentifier=src-pg" +
				"&TargetDBParameterGroupIdentifier=dst-pg" +
				"&TargetDBParameterGroupDescription=Copied",
			wantCode:     http.StatusOK,
			wantContains: []string{"CopyDBParameterGroupResponse", "dst-pg"},
		},
		{
			name: "CopyDBParameterGroup_source_not_found",
			body: "Action=CopyDBParameterGroup&Version=2014-10-31" +
				"&SourceDBParameterGroupIdentifier=no-such-pg" +
				"&TargetDBParameterGroupIdentifier=dst-pg",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBParameterGroupNotFound"},
		},
		// CopyOptionGroup
		{
			name: "CopyOptionGroup_success",
			setupBodies: []string{
				"Action=CreateOptionGroup&Version=2014-10-31" +
					"&OptionGroupName=src-og&EngineName=mysql&MajorEngineVersion=8.0&OptionGroupDescription=Source",
			},
			body: "Action=CopyOptionGroup&Version=2014-10-31" +
				"&SourceOptionGroupIdentifier=src-og" +
				"&TargetOptionGroupIdentifier=dst-og" +
				"&TargetOptionGroupDescription=Copied",
			wantCode:     http.StatusOK,
			wantContains: []string{"CopyOptionGroupResponse", "dst-og"},
		},
		{
			name: "CopyOptionGroup_source_not_found",
			body: "Action=CopyOptionGroup&Version=2014-10-31" +
				"&SourceOptionGroupIdentifier=no-such-og" +
				"&TargetOptionGroupIdentifier=dst-og",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"OptionGroupNotFound"},
		},
		// CreateBlueGreenDeployment
		{
			name: "CreateBlueGreenDeployment_success",
			body: "Action=CreateBlueGreenDeployment&Version=2014-10-31" +
				"&BlueGreenDeploymentName=my-deployment" +
				"&Source=arn:aws:rds:us-east-1:000000000000:db:my-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateBlueGreenDeploymentResponse", "my-deployment"},
		},
		{
			name: "CreateBlueGreenDeployment_empty_name",
			body: "Action=CreateBlueGreenDeployment&Version=2014-10-31" +
				"&BlueGreenDeploymentName=" +
				"&Source=arn:aws:rds:us-east-1:000000000000:db:my-db",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "CreateBlueGreenDeployment_empty_source",
			body: "Action=CreateBlueGreenDeployment&Version=2014-10-31" +
				"&BlueGreenDeploymentName=my-deployment" +
				"&Source=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "CreateBlueGreenDeployment_duplicate",
			setupBodies: []string{
				"Action=CreateBlueGreenDeployment&Version=2014-10-31" +
					"&BlueGreenDeploymentName=my-deployment" +
					"&Source=arn:aws:rds:us-east-1:000000000000:db:my-db",
			},
			body: "Action=CreateBlueGreenDeployment&Version=2014-10-31" +
				"&BlueGreenDeploymentName=my-deployment" +
				"&Source=arn:aws:rds:us-east-1:000000000000:db:my-db",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"BlueGreenDeploymentAlreadyExists"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()

			for _, setup := range tt.setupBodies {
				postRDSForm(t, h, setup)
			}

			rec := postRDSForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)

			body := rec.Body.String()
			for _, s := range tt.wantContains {
				assert.Contains(t, body, s)
			}

			for _, s := range tt.wantNotContains {
				assert.NotContains(t, body, s)
			}
		})
	}
}

// TestRDSHandler_NewOperations tests new HTTP handler operations.
func TestRDSHandler_NewOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		setupBodies  []string
		wantContains []string
		wantCode     int
	}{
		{
			name: "CopyDBSnapshot_success",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=snap-copy-db&Engine=postgres",
				"Action=CreateDBSnapshot&Version=2014-10-31&DBSnapshotIdentifier=src-snap&DBInstanceIdentifier=snap-copy-db",
			},
			body: "Action=CopyDBSnapshot&Version=2014-10-31" +
				"&SourceDBSnapshotIdentifier=src-snap&TargetDBSnapshotIdentifier=dst-snap",
			wantCode:     http.StatusOK,
			wantContains: []string{"CopyDBSnapshotResponse", "dst-snap"},
		},
		{
			name: "CopyDBSnapshot_source_not_found",
			body: "Action=CopyDBSnapshot&Version=2014-10-31" +
				"&SourceDBSnapshotIdentifier=no-snap&TargetDBSnapshotIdentifier=dst-snap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBSnapshotNotFound"},
		},
		{
			name: "RestoreDBInstanceFromDBSnapshot_success",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=orig-db&Engine=postgres",
				"Action=CreateDBSnapshot&Version=2014-10-31&DBSnapshotIdentifier=restore-snap&DBInstanceIdentifier=orig-db",
			},
			body: "Action=RestoreDBInstanceFromDBSnapshot&Version=2014-10-31" +
				"&DBInstanceIdentifier=restored-db&DBSnapshotIdentifier=restore-snap",
			wantCode:     http.StatusOK,
			wantContains: []string{"RestoreDBInstanceFromDBSnapshotResponse", "restored-db"},
		},
		{
			name: "RestoreDBInstanceFromDBSnapshot_snap_not_found",
			body: "Action=RestoreDBInstanceFromDBSnapshot&Version=2014-10-31" +
				"&DBInstanceIdentifier=restored-db&DBSnapshotIdentifier=no-snap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBSnapshotNotFound"},
		},
		{
			name: "RestoreDBInstanceToPointInTime_success",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=pit-src-db&Engine=mysql",
			},
			body: "Action=RestoreDBInstanceToPointInTime&Version=2014-10-31" +
				"&TargetDBInstanceIdentifier=pit-restored-db&SourceDBInstanceIdentifier=pit-src-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"RestoreDBInstanceToPointInTimeResponse", "pit-restored-db"},
		},
		{
			name: "RestoreDBInstanceToPointInTime_source_not_found",
			body: "Action=RestoreDBInstanceToPointInTime&Version=2014-10-31" +
				"&TargetDBInstanceIdentifier=pit-db&SourceDBInstanceIdentifier=no-db",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		{
			name: "StartDBInstance_success",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=start-db",
				"Action=StopDBInstance&Version=2014-10-31&DBInstanceIdentifier=start-db",
			},
			body:         "Action=StartDBInstance&Version=2014-10-31&DBInstanceIdentifier=start-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"StartDBInstanceResponse", "start-db", "available"},
		},
		{
			name: "StopDBInstance_success",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=stop-db",
			},
			body:         "Action=StopDBInstance&Version=2014-10-31&DBInstanceIdentifier=stop-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"StopDBInstanceResponse", "stop-db", "stopped"},
		},
		{
			name:         "StopDBInstance_not_found",
			body:         "Action=StopDBInstance&Version=2014-10-31&DBInstanceIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		{
			name: "StartDBInstance_wrong_state",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=running-db",
			},
			body:         "Action=StartDBInstance&Version=2014-10-31&DBInstanceIdentifier=running-db",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidDBInstanceState"},
		},
		{
			name: "CreateGlobalCluster_success",
			body: "Action=CreateGlobalCluster&Version=2014-10-31" +
				"&GlobalClusterIdentifier=gc1&Engine=aurora-postgresql&EngineVersion=14.3",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateGlobalClusterResponse", "gc1", "aurora-postgresql"},
		},
		{
			name: "DescribeGlobalClusters_all",
			setupBodies: []string{
				"Action=CreateGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=gc-list1",
				"Action=CreateGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=gc-list2",
			},
			body:         "Action=DescribeGlobalClusters&Version=2014-10-31",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeGlobalClustersResponse", "gc-list1", "gc-list2"},
		},
		{
			name: "DeleteGlobalCluster_success",
			setupBodies: []string{
				"Action=CreateGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=gc-del",
			},
			body:         "Action=DeleteGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=gc-del",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteGlobalClusterResponse", "gc-del"},
		},
		{
			name:         "DeleteGlobalCluster_not_found",
			body:         "Action=DeleteGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"GlobalClusterNotFound"},
		},
		{
			name: "ModifyGlobalCluster_success",
			setupBodies: []string{
				"Action=CreateGlobalCluster&Version=2014-10-31&GlobalClusterIdentifier=gc-mod",
			},
			body: "Action=ModifyGlobalCluster&Version=2014-10-31" +
				"&GlobalClusterIdentifier=gc-mod&EngineVersion=15.0&DeletionProtection=true",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyGlobalClusterResponse", "gc-mod", "15.0"},
		},
		{
			name: "CreateDBInstance_with_new_fields",
			body: "Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=rich-db&Engine=postgres" +
				"&MultiAZ=true&StorageType=io1&BackupRetentionPeriod=7&StorageEncrypted=true",
			wantCode: http.StatusOK,
			wantContains: []string{
				"CreateDBInstanceResponse",
				"rich-db",
				"<MultiAZ>true</MultiAZ>",
				"<StorageType>io1</StorageType>",
				"<StorageEncrypted>true</StorageEncrypted>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()

			for _, setupBody := range tt.setupBodies {
				rds.FlushInstanceLifecycle(h.Backend) // ensure prior instances are available
				rec := postRDSForm(t, h, setupBody)
				require.Equal(t, http.StatusOK, rec.Code, "setup body %q failed: %s", setupBody, rec.Body.String())
			}

			rds.FlushInstanceLifecycle(h.Backend) // advance creating→available before action

			rec := postRDSForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, want := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), want)
			}
		})
	}
}

// TestRDS_FormParseFail_Returns400 asserts that a malformed form body returns 400 ValidationException.
func TestRDS_FormParseFail_Returns400(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()

	// Send a body that causes ParseForm to fail by using a semicolon-only content type
	// that triggers an invalid percent-encoding error.
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("%zz=bad"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ValidationException")
}

// TestExtendedOperations_DoNotError covers all the stub RDS operations.
func TestExtendedOperations_DoNotError(t *testing.T) {
	t.Parallel()

	stubOps := []struct {
		action string
		params string
	}{
		{"CreateCustomDBEngineVersion", "Engine=custom-oracle-ee&EngineVersion=19.0.0.0"},
		{"DeleteCustomDBEngineVersion", "Engine=custom-oracle-ee&EngineVersion=19.0.0.0"},
		{"ModifyCustomDBEngineVersion", "Engine=custom-oracle-ee&EngineVersion=19.0.0.0"},
		{"CreateDBShardGroup", "DBShardGroupIdentifier=test-shard&DBClusterIdentifier=test-cluster&MaxACU=1"},
		{"DeleteDBShardGroup", "DBShardGroupIdentifier=test-shard"},
		{"DescribeDBShardGroups", ""},
		{"ModifyDBShardGroup", "DBShardGroupIdentifier=test-shard"},
		{"RebootDBShardGroup", "DBShardGroupIdentifier=test-shard"},
		{
			"CreateIntegration",
			"IntegrationName=test-integration" +
				"&SourceArn=arn:aws:rds:us-east-1:000000000000:db:test" +
				"&TargetArn=arn:aws:redshift:us-east-1:000000000000:namespace:test",
		},
		{"DeleteIntegration", "IntegrationIdentifier=test-integration"},
		{"DescribeIntegrations", ""},
		{"ModifyIntegration", "IntegrationIdentifier=test-integration"},
		{"CreateTenantDatabase", "DBInstanceIdentifier=test-db&TenantDBName=tenantdb&MasterUsername=admin"},
		{"DeleteTenantDatabase", "DBInstanceIdentifier=test-db&TenantDBName=tenantdb"},
		{"DescribeTenantDatabases", ""},
		{"ModifyTenantDatabase", "DBInstanceIdentifier=test-db&TenantDBName=tenantdb"},
		{"DeleteDBClusterAutomatedBackup", "DbClusterResourceId=cluster-1"},
		{"DescribeDBClusterAutomatedBackups", ""},
		{"DeleteDBInstanceAutomatedBackup", "DbiResourceId=db-1"},
		{"DescribeDBInstanceAutomatedBackups", ""},
		{
			"StartDBInstanceAutomatedBackupsReplication",
			"SourceDBInstanceArn=arn:aws:rds:us-east-1:000000000000:db:test",
		},
		{"StopDBInstanceAutomatedBackupsReplication", "SourceDBInstanceArn=arn:aws:rds:us-east-1:000000000000:db:test"},
		{"DescribeDBSnapshotTenantDatabases", ""},
		{
			"GetPerformanceInsightsMetrics",
			"ServiceType=RDS&Identifier=db-test&MetricQueries.member.1.Metric=db.load.avg",
		},
	}

	h := newRDSHandler()
	h.Backend.CreateDBInstance(
		"db-test",
		"mysql",
		"db.t3.micro",
		"",
		"admin",
		"",
		20,
		rds.DBInstanceOptions{PerformanceInsightsEnabled: true},
	)

	for _, op := range stubOps {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()
			body := "Action=" + op.action + "&Version=2014-10-31"
			if op.params != "" {
				body += "&" + op.params
			}

			rec := postRDSForm(t, h, body)
			assert.NotEqual(t, http.StatusInternalServerError, rec.Code,
				"action %s should not 500", op.action)
		})
	}
}

// TestBackendOps covers DescribeDBInstanceAutomatedBackups backend.
func TestBackendOps(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()

	tests := []struct {
		name string
		body string
	}{
		{"DescribeDBInstanceAutomatedBackups", "Action=DescribeDBInstanceAutomatedBackups&Version=2014-10-31"},
		{"DescribeDBClusterAutomatedBackups", "Action=DescribeDBClusterAutomatedBackups&Version=2014-10-31"},
		{"DescribeDBSnapshotTenantDatabases", "Action=DescribeDBSnapshotTenantDatabases&Version=2014-10-31"},
		{"DescribeTenantDatabases", "Action=DescribeTenantDatabases&Version=2014-10-31"},
		{"DescribeIntegrations", "Action=DescribeIntegrations&Version=2014-10-31"},
		{"DescribeDBShardGroups", "Action=DescribeDBShardGroups&Version=2014-10-31"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := postRDSForm(t, h, tt.body)
			assert.GreaterOrEqual(t, rec.Code, 200, "expected non-negative status")
		})
	}
}

// TestCloseIdempotent verifies Close can be called more than once without
// panicking on a double-close of stopCh (sync.Once guard).
func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	b := rds.NewInMemoryBackend("123456789012", "us-east-1")

	require.NotPanics(t, func() {
		b.Close()
		b.Close()
	})
}

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// rds client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.  The test fails when the upstream
// SDK adds a new operation that gopherstack has not yet handled.
func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	backend := rds.NewInMemoryBackend("000000000000", "us-east-1")
	h := rds.NewHandler(backend)
	sdkcheck.CheckCompleteness(t, &rdssdk.Client{}, h.GetSupportedOperations(), []string{})
}
