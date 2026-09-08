package redshift_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *redshift.InMemoryBackend) string
		verify func(t *testing.T, b *redshift.InMemoryBackend, id string)
		name   string
	}{
		{
			name: "round_trip_preserves_state",
			setup: func(b *redshift.InMemoryBackend) string {
				cluster, err := b.CreateCluster("test-cluster", "ra3.xlplus", "admin", "Pass1234!", nil, "")
				if err != nil {
					return ""
				}

				return cluster.ClusterIdentifier
			},
			verify: func(t *testing.T, b *redshift.InMemoryBackend, id string) {
				t.Helper()

				clusters, _, err := b.DescribeClusters(id, "", 0, nil, nil)
				require.NoError(t, err)
				require.Len(t, clusters, 1)
				assert.Equal(t, id, clusters[0].ClusterIdentifier)
			},
		},
		{
			name:  "empty_backend_round_trip",
			setup: func(_ *redshift.InMemoryBackend) string { return "" },
			verify: func(t *testing.T, b *redshift.InMemoryBackend, _ string) {
				t.Helper()

				clusters, _, err := b.DescribeClusters("", "", 0, nil, nil)
				require.NoError(t, err)
				assert.Empty(t, clusters)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			id := tt.setup(original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh, id)
		})
	}
}

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

func TestRedshiftHandler_Persistence(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	h := redshift.NewHandler(backend)

	_, err := backend.CreateCluster("snap-cluster", "ra3.xlplus", "admin", "Pass1234!", nil, "")
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	freshH := redshift.NewHandler(fresh)
	require.NoError(t, freshH.Restore(t.Context(), snap))

	clusters, _, err := fresh.DescribeClusters("", "", 0, nil, nil)
	require.NoError(t, err)
	assert.Len(t, clusters, 1)
}

func TestRedshiftHandler_Routing(t *testing.T) {
	t.Parallel()

	h := redshift.NewHandler(redshift.NewInMemoryBackend("000000000000", "us-east-1"))

	assert.Equal(t, "Redshift", h.Name())
	assert.Positive(t, h.MatchPriority())

	e := echo.New()

	tests := []struct {
		name      string
		ct        string
		wantMatch bool
	}{
		{"redshift form", "application/x-www-form-urlencoded", true},
		{"json type", "application/json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(
				http.MethodPost,
				"/",
				strings.NewReader("Action=CreateCluster&Version=2012-12-01"),
			)
			req.Header.Set("Content-Type", tt.ct)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.wantMatch, h.RouteMatcher()(c))
		})
	}

	// Test ExtractOperation
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("Action=DescribeClusters"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := e.NewContext(req, httptest.NewRecorder())
	assert.Equal(t, "DescribeClusters", h.ExtractOperation(c))

	// Test ExtractResource
	req2 := httptest.NewRequest(
		http.MethodPost,
		"/",
		strings.NewReader("Action=DescribeClusters&ClusterIdentifier=my-cluster"),
	)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c2 := e.NewContext(req2, httptest.NewRecorder())
	assert.Equal(t, "my-cluster", h.ExtractResource(c2))
}

// TestInMemoryBackend_FullStateRoundTrip is the Phase 3.3 store.Table
// conversion's persistence-safety proof: it seeds one entry in every
// resource collection the backend owns -- every store.Table registered in
// store_setup.go, plus the three resource maps deliberately left raw
// (activeResizes, snapshotCopyConfigs, loggingStatuses) -- snapshots, restores
// into a fresh backend, and asserts every entry survived. Nothing here should
// be dropped; if it is, this test fails.
//
// events is intentionally NOT seeded: no public API populates it (see
// store_setup.go / DescribeEvents), matching pre-conversion behaviour.
func TestInMemoryBackend_FullStateRoundTrip(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateCluster("rt-cluster", "ra3.xlplus", "rtdb", "admin", nil, "")
	require.NoError(t, err)

	b.AddReservedNodeInternal(&redshift.ReservedNode{ReservedNodeID: "rn-1"})

	_, err = b.AddPartner("000000000000", "rt-cluster", "rtdb", "partner-1")
	require.NoError(t, err)

	b.AddDataShareInternal(&redshift.DataShare{DataShareArn: "arn:aws:redshift:us-east-1:000000000000:datashare:ds-1"})

	_, err = b.CreateClusterSecurityGroup("rt-secgroup", "desc", nil)
	require.NoError(t, err)

	_, err = b.CreateClusterSnapshot("rt-snapshot", "rt-cluster")
	require.NoError(t, err)

	_, err = b.AuthorizeEndpointAccess("rt-cluster", "111111111111", nil)
	require.NoError(t, err)

	_, err = b.CreateClusterParameterGroup("rt-paramgroup", "redshift-1.0", "desc")
	require.NoError(t, err)

	_, err = b.CreateClusterSubnetGroup("rt-subnetgroup", "desc", "vpc-1", []string{"subnet-1"}, nil)
	require.NoError(t, err)

	_, err = b.CreateEventSubscription(
		"rt-eventsub", "arn:aws:sns:us-east-1:000000000000:topic", "cluster", "INFO", nil, nil, true,
	)
	require.NoError(t, err)

	_, err = b.CreateSnapshotCopyGrant("rt-copygrant", "kms-key-1", nil)
	require.NoError(t, err)

	_, err = b.CreateSnapshotSchedule("rt-copyschedule", "desc", []string{"rate(12 hours)"}, nil)
	require.NoError(t, err)

	_, err = b.CreateUsageLimit("rt-cluster", "spectrum", "data-scanned", "log", 1000, nil)
	require.NoError(t, err)

	_, err = b.CreateAuthenticationProfile("rt-authprofile", `{"AllowDBUserOverride":"1"}`)
	require.NoError(t, err)

	_, err = b.PutResourcePolicy(
		"arn:aws:redshift:us-east-1:000000000000:cluster:rt-cluster", `{"Version":"2012-10-17"}`,
	)
	require.NoError(t, err)

	_, err = b.CreateTableRestoreStatus("rt-cluster", "rt-snapshot", "srcdb", "srctbl", "dstdb", "dsttbl")
	require.NoError(t, err)

	_, err = b.CreateHsmClientCertificate("rt-hsmcert", nil)
	require.NoError(t, err)

	_, err = b.CreateHsmConfiguration("rt-hsmconfig", "desc", "10.0.0.1", "partition-1", nil)
	require.NoError(t, err)

	_, err = b.CreateScheduledAction(
		"rt-scheduledaction", "at(2030-01-01T00:00:00)", "arn:aws:iam::000000000000:role/r", "desc", nil, nil,
	)
	require.NoError(t, err)

	_, err = b.CreateCustomDomainAssociation(
		"rt-cluster", "db.example.com", "arn:aws:acm:us-east-1:000000000000:certificate/1",
	)
	require.NoError(t, err)

	_, err = b.CreateEndpointAccess("rt-cluster", "rt-endpointaccess", "", "", nil)
	require.NoError(t, err)

	_, err = b.CreateIntegration(
		"rt-integration",
		"arn:aws:redshift:us-east-1:000000000000:cluster:rt-cluster",
		"arn:aws:redshift:us-east-1:000000000000:namespace/1",
		"", "desc", nil,
	)
	require.NoError(t, err)

	_, err = b.CreateIdcApplication(
		"rt-idcapp", "arn:aws:sso::000000000000:instance/1", "display", "arn:aws:iam::000000000000:role/r", "None",
	)
	require.NoError(t, err)

	ns, err := b.CreateNamespace(redshift.CreateNamespaceParams{
		NamespaceName: "rt-namespace",
		AdminUsername: "admin",
		DBName:        "rtdb",
		Tags:          map[string]string{"env": "rt"},
	})
	require.NoError(t, err)

	_, err = b.UpdateLakehouseConfigurationSL(redshift.UpdateLakehouseConfigParams{
		NamespaceName:              "rt-namespace",
		CatalogName:                "rt-catalog",
		LakehouseRegistration:      "Register",
		LakehouseIdcApplicationArn: "arn:aws:sso::000000000000:application/rt-idc-app",
		LakehouseIdcRegistration:   "Associate",
	})
	require.NoError(t, err)

	_, err = b.CreateWorkgroup(
		"rt-workgroup", "rt-namespace", redshift.WorkgroupParams{BaseCapacity: 32}, nil,
	)
	require.NoError(t, err)

	_, err = b.CreateServerlessSnapshot("rt-slsnapshot", "rt-namespace", 0, nil)
	require.NoError(t, err)

	rtRetention := 45
	_, err = b.UpdateServerlessSnapshot("rt-slsnapshot", &rtRetention)
	require.NoError(t, err)

	rtRecoveryPoints, _ := b.ListRecoveryPointsSL(redshift.ListRecoveryPointsParams{NamespaceName: "rt-namespace"})
	require.Len(t, rtRecoveryPoints, 1, "CreateWorkgroup must have generated exactly one recovery point")
	rtRecoveryPointID := rtRecoveryPoints[0].RecoveryPointID

	rtTableRestore, err := b.RestoreTableFromSnapshotSL(redshift.RestoreTableFromSnapshotParams{
		NamespaceName:      "rt-namespace",
		WorkgroupName:      "rt-workgroup",
		NewTableName:       "rt-newtable",
		SnapshotName:       "rt-slsnapshot",
		SourceDatabaseName: "rt-srcdb",
		SourceTableName:    "rt-srctable",
	})
	require.NoError(t, err)

	_, err = b.CreateCustomDomainAssociationSL(
		"rt.example.com", "arn:aws:acm:us-east-1:000000000000:certificate/rt-1", "rt-workgroup",
	)
	require.NoError(t, err)

	_, err = b.PutResourcePolicySL(ns.NamespaceArn, `{"Version":"2012-10-17"}`)
	require.NoError(t, err)

	scc, err := b.CreateSnapshotCopyConfigurationSL("rt-namespace", "us-west-2", "", 7)
	require.NoError(t, err)

	_, err = b.CreateEndpointAccessSL("rt-slendpoint", "rt-workgroup", "", []string{"subnet-1"}, nil)
	require.NoError(t, err)

	_, err = b.CreateServerlessUsageLimit(
		"arn:aws:redshift-serverless:us-east-1:000000000000:workgroup/rt-workgroup",
		"ComputeCapacity", "daily", "log", 100,
	)
	require.NoError(t, err)

	_, err = b.CreateServerlessScheduledAction(redshift.CreateScheduledActionParams{
		ScheduledActionName: "rt-slscheduledaction",
		NamespaceName:       "rt-namespace",
		RoleArn:             "arn:aws:iam::000000000000:role/scheduler",
		Schedule:            json.RawMessage(`{"at":1893456000}`),
		TargetAction: json.RawMessage(
			`{"createSnapshot":{"namespaceName":"rt-namespace","snapshotName":"rt-slsnapshot2"}}`,
		),
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	// The three raw (unconverted) maps.
	b.AddActiveResizeInternal("rt-cluster", &redshift.ResizeProgress{Status: "IN_PROGRESS", AllowCancelResize: true})

	_, err = b.EnableSnapshotCopy("rt-cluster", "us-west-2", "rt-copygrant", 7)
	require.NoError(t, err)

	_, err = b.EnableLogging("rt-cluster", "rt-bucket", "prefix/")
	require.NoError(t, err)

	// Snapshot and restore into a fresh backend.
	data := b.Snapshot(t.Context())
	require.NotNil(t, data)

	fresh := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), data))

	// Verify every seeded resource survived the round trip.
	assert.Equal(t, 1, redshift.ClusterCount(fresh))
	assert.Equal(t, 1, redshift.ReservedNodeCount(fresh))
	assert.Equal(t, 1, redshift.PartnerCount(fresh))
	assert.Equal(t, 1, redshift.DataShareCount(fresh))
	assert.Equal(t, 1, redshift.SecurityGroupCount(fresh))
	assert.Equal(t, 1, redshift.SnapshotCount(fresh))
	assert.Equal(t, 1, redshift.EndpointAuthCount(fresh))
	assert.Equal(t, 1, redshift.ParameterGroupCount(fresh))
	assert.Equal(t, 1, redshift.SubnetGroupCount(fresh))
	assert.Equal(t, 1, redshift.EventSubscriptionCount(fresh))
	assert.Equal(t, 1, redshift.HsmClientCertCount(fresh))
	assert.Equal(t, 1, redshift.HsmConfigCount(fresh))
	assert.Equal(t, 1, redshift.ScheduledActionCount(fresh))
	assert.Equal(t, 1, redshift.CustomDomainCount(fresh))
	assert.Equal(t, 1, redshift.EndpointAccessCount(fresh))
	assert.Equal(t, 1, redshift.IntegrationCount(fresh))
	assert.Equal(t, 1, redshift.IdcApplicationCount(fresh))

	// The three deliberately-raw maps (see store_setup.go).
	assert.Equal(t, 1, redshift.ActiveResizeCount(fresh))
	assert.Equal(t, 1, redshift.SnapshotCopyConfigCount(fresh))
	assert.Equal(t, 1, redshift.LoggingStatusCount(fresh))

	// Resources without an exported Count helper: verify via the public
	// Describe/Get surface instead.
	grants, err := fresh.DescribeSnapshotCopyGrants("")
	require.NoError(t, err)
	assert.Len(t, grants, 1)

	schedules, err := fresh.DescribeSnapshotSchedules("")
	require.NoError(t, err)
	assert.Len(t, schedules, 1)

	limits, err := fresh.DescribeUsageLimits("", "")
	require.NoError(t, err)
	assert.Len(t, limits, 1)

	profiles, err := fresh.DescribeAuthenticationProfiles("")
	require.NoError(t, err)
	assert.Len(t, profiles, 1)

	_, err = fresh.GetResourcePolicy("arn:aws:redshift:us-east-1:000000000000:cluster:rt-cluster")
	require.NoError(t, err)

	restores, err := fresh.DescribeTableRestoreStatus("")
	require.NoError(t, err)
	assert.Len(t, restores, 1)

	freshNS, err := fresh.GetNamespace("rt-namespace")
	require.NoError(t, err)
	assert.Contains(t, freshNS.CatalogArn, "rt-catalog")
	assert.Equal(t, "Registered", freshNS.LakehouseRegistrationStatus)

	_, err = fresh.GetWorkgroup("rt-workgroup")
	require.NoError(t, err)

	slSnap, err := fresh.GetServerlessSnapshot("rt-slsnapshot", "")
	require.NoError(t, err)
	assert.Equal(t, 45, slSnap.SnapshotRetentionPeriod)

	// A follow-up UpdateLakehouseConfiguration call that changes only the
	// registration status must still see the LakehouseIdcApplicationArn set
	// before the round trip -- proving slLakehouseConfig (a table with no
	// Namespace field of its own) survived Restore, not just Namespace's own
	// CatalogArn/LakehouseRegistrationStatus fields.
	lhResult, err := fresh.UpdateLakehouseConfigurationSL(redshift.UpdateLakehouseConfigParams{
		NamespaceName:         "rt-namespace",
		LakehouseRegistration: "Deregister",
	})
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:sso::000000000000:application/rt-idc-app", lhResult.LakehouseIdcApplicationArn)

	slLimits, _ := fresh.ListServerlessUsageLimits("", "", 0, "")
	assert.Len(t, slLimits, 1)

	_, err = fresh.GetServerlessScheduledAction("rt-slscheduledaction")
	require.NoError(t, err)

	nsTags, err := fresh.ListServerlessResourceTags(ns.NamespaceArn)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "rt"}, nsTags)

	assoc, err := fresh.GetCustomDomainAssociationSL("rt.example.com", "rt-workgroup")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:acm:us-east-1:000000000000:certificate/rt-1", assoc.CustomDomainCertificateArn)

	rp, err := fresh.GetResourcePolicySL(ns.NamespaceArn)
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":"2012-10-17"}`, rp.Policy)

	sccList, _ := fresh.ListSnapshotCopyConfigurationsSL("rt-namespace", 0, "")
	require.Len(t, sccList, 1)
	assert.Equal(t, scc.SnapshotCopyConfigurationID, sccList[0].SnapshotCopyConfigurationID)

	recoveryPoint, err := fresh.GetRecoveryPointSL(rtRecoveryPointID)
	require.NoError(t, err)
	assert.Equal(t, "rt-namespace", recoveryPoint.NamespaceName)

	recoveryPointList, _ := fresh.ListRecoveryPointsSL(redshift.ListRecoveryPointsParams{NamespaceName: "rt-namespace"})
	require.Len(t, recoveryPointList, 1, "sorted index must survive the round trip, not just the underlying table")

	tr, err := fresh.GetTableRestoreStatusSL(rtTableRestore.TableRestoreRequestID)
	require.NoError(t, err)
	assert.Equal(t, "rt-newtable", tr.NewTableName)

	tableRestoreList, _ := fresh.ListTableRestoreStatusSL("rt-namespace", "", 0, "")
	require.Len(t, tableRestoreList, 1, "sorted index must survive the round trip, not just the underlying table")

	_, err = fresh.GetEndpointAccessSL("rt-slendpoint")
	require.NoError(t, err)

	endpointList, _ := fresh.ListEndpointAccessSL("rt-workgroup", "", 0, "")
	require.Len(t, endpointList, 1, "sorted index must survive the round trip, not just the underlying table")

	// Disabling snapshot copy only succeeds if snapshotCopyConfigs (raw map)
	// survived the round trip with the rt-cluster entry intact.
	_, err = fresh.DisableSnapshotCopy("rt-cluster")
	require.NoError(t, err)
}

// TestPersistence_FullRoundTrip verifies snapshot+restore preserves all fields.
func TestPersistence_FullRoundTrip(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=persist-cluster")
	postRedshiftForm(t, h,
		"Action=ModifyClusterIamRoles&Version=2012-12-01&ClusterIdentifier=persist-cluster"+
			"&AddIamRoles.IamRoleArn.1=arn:aws:iam::123456789012:role/Role1")
	postRedshiftForm(t, h,
		"Action=CreateSnapshotCopyGrant&Version=2012-12-01&SnapshotCopyGrantName=my-grant")
	postRedshiftForm(t, h,
		"Action=CreateUsageLimit&Version=2012-12-01&ClusterIdentifier=persist-cluster"+
			"&FeatureType=spectrum&LimitType=time&Amount=1000&BreachAction=log")

	ctx := t.Context()
	data := h.Backend.Snapshot(ctx)
	require.NotNil(t, data)

	h2 := newRedshiftHandler()
	err := h2.Backend.Restore(ctx, data)
	require.NoError(t, err)

	// Verify clusters restored.
	rec := postRedshiftForm(t, h2, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=persist-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "persist-cluster")
	assert.Contains(t, rec.Body.String(), "Role1")

	// Verify snapshot copy grant restored.
	rec2 := postRedshiftForm(t, h2,
		"Action=DescribeSnapshotCopyGrants&Version=2012-12-01&SnapshotCopyGrantName=my-grant")
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "my-grant")

	// Verify usage limits restored.
	rec3 := postRedshiftForm(t, h2,
		"Action=DescribeUsageLimits&Version=2012-12-01&ClusterIdentifier=persist-cluster")
	require.Equal(t, http.StatusOK, rec3.Code)
	assert.Contains(t, rec3.Body.String(), "spectrum")
}

// ---- Persistence round-trip for reserved nodes, data shares, security groups, snapshots, resizes ----

func TestBackend_SnapshotRestore_NewMaps(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")

	b.AddReservedNodeInternal(&redshift.ReservedNode{
		ReservedNodeID:         "rn-persist",
		ReservedNodeOfferingID: "offering-persist",
		NodeType:               "ra3.xlplus",
		NodeCount:              2,
		State:                  "active",
	})
	b.AddDataShareInternal(&redshift.DataShare{
		DataShareArn: "arn:aws:redshift:us-east-1:000000000000:datashare:ds-persist",
		ProducerArn:  "producer-arn",
	})
	b.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{
		ClusterSecurityGroupName: "sg-persist",
		Description:              "persist test",
	})
	b.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "snap-persist",
		ClusterIdentifier:  "c-persist",
		Status:             "available",
	})

	_, err := b.CreateCluster("resize-persist-cluster", "", "", "", nil, "")
	require.NoError(t, err)
	b.AddActiveResizeInternal("resize-persist-cluster", &redshift.ResizeProgress{
		Status:            "IN_PROGRESS",
		AllowCancelResize: true,
	})

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// Verify reserved node round-trip
	_, err = fresh.AcceptReservedNodeExchange("rn-persist", "new-offering")
	require.NoError(t, err)

	// Verify data share round-trip
	_, err = fresh.AuthorizeDataShare("arn:aws:redshift:us-east-1:000000000000:datashare:ds-persist", "consumer-id")
	require.NoError(t, err)

	// Verify security group round-trip
	_, err = fresh.AuthorizeClusterSecurityGroupIngress("sg-persist", "192.168.1.0/24", "", "")
	require.NoError(t, err)

	// Verify snapshot round-trip
	_, err = fresh.AuthorizeSnapshotAccess("snap-persist", "111111111111")
	require.NoError(t, err)

	// Verify resize round-trip
	_, err = fresh.CancelResize("resize-persist-cluster")
	require.NoError(t, err)
}

// ---- Persistence round-trip (reserved nodes, data shares, security groups, snapshots, resizes) ----

func TestPersistence_RoundTrip(t *testing.T) {
	t.Parallel()

	b1 := redshift.NewInMemoryBackend("123456789012", "us-west-2")
	_, err := b1.CreateCluster("p-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	b1.AddReservedNodeInternal(&redshift.ReservedNode{ReservedNodeID: "rn-p1", NodeType: "dc2.large", State: "active"})
	b1.AddDataShareInternal(&redshift.DataShare{DataShareArn: "arn:aws:redshift::123:datashare:ds-p1"})
	b1.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{ClusterSecurityGroupName: "sg-p1"})
	b1.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "snap-p1", ClusterIdentifier: "p-cluster", Status: "available"},
	)
	b1.AddActiveResizeInternal("p-cluster", &redshift.ResizeProgress{Status: "IN_PROGRESS", AllowCancelResize: true})

	data := b1.Snapshot(t.Context())
	require.NotNil(t, data)

	b2 := redshift.NewInMemoryBackend("", "")
	err = b2.Restore(t.Context(), data)
	require.NoError(t, err)

	assert.Equal(t, "123456789012", b2.AccountID())
	assert.Equal(t, "us-west-2", b2.Region())
	assert.Equal(t, 1, redshift.ClusterCount(b2))
	assert.Equal(t, 1, redshift.ReservedNodeCount(b2))
	assert.Equal(t, 1, redshift.DataShareCount(b2))
	assert.Equal(t, 1, redshift.SecurityGroupCount(b2))
	assert.Equal(t, 1, redshift.SnapshotCount(b2))
	assert.Equal(t, 1, redshift.ActiveResizeCount(b2))
}
