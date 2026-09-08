package quicksight_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/quicksight"
)

// TestQuickSight_Phase3_3_StoreRoundTrip verifies that the Phase 3.3
// pkgs/store conversion (store.go, store_setup.go: every map[string]*T
// resource field with a pure identity registered as a *store.Table[T] on the
// backend's registry) preserves full backend state across a Snapshot/Restore
// round-trip. It seeds at least one entry in every converted table plus a
// representative sample of the maps deliberately left raw (tags,
// groupMembers, publicSharing -- see store_setup.go's doc comment), then
// asserts every seeded resource is independently readable from a brand-new
// backend restored from the snapshot.
func TestQuickSight_Phase3_3_StoreRoundTrip(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	const ns = "roundtrip-ns"

	// ---- seed one entry per converted store.Table ----

	_, err := b.CreateNamespace(testAccountID, ns, testRegion, nil)
	require.NoError(t, err)

	_, err = b.CreateGroup(testAccountID, ns, "group1", "a group")
	require.NoError(t, err)

	_, err = b.CreateGroupMembership(testAccountID, ns, "group1", "user1")
	require.NoError(t, err)

	_, err = b.RegisterUser(testAccountID, ns, "user1", "user1@example.com", "READER", "QUICKSIGHT", "", nil)
	require.NoError(t, err)

	_, err = b.CreateDataSource(testAccountID, "ds1", "DataSource1", "MYSQL", nil, nil)
	require.NoError(t, err)

	_, _, err = b.CreateDataSet(
		testAccountID,
		"dset1",
		"DataSet1",
		"SPICE",
		nil,
		nil,
		map[string]quicksight.PhysicalTable{
			"pt1": {RelationalTable: &quicksight.RelationalTable{DataSourceArn: "ds1", Name: "table1"}},
		},
		nil,
	)
	require.NoError(t, err)

	_, err = b.CreateIngestion(testAccountID, "dset1", "ingest1")
	require.NoError(t, err)

	dash, err := b.CreateDashboard(testAccountID, "dash1", "Dashboard1", "", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAnalysis(testAccountID, "an1", "Analysis1", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateFolder(testAccountID, "folder1", "Folder1", "", "", "", nil, nil)
	require.NoError(t, err)

	_, err = b.CreateFolderMembership(testAccountID, "folder1", "dash1", "DASHBOARD")
	require.NoError(t, err)

	_, err = b.CreateTemplate(testAccountID, "tmpl1", "Template1", "", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateTheme(testAccountID, "theme1", "Theme1", "", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateTopic(testAccountID, "topic1", "Topic1", "", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateVPCConnection(testAccountID, "vpc1", "VPCConn1", "vpc-1", nil, nil, nil, "", nil)
	require.NoError(t, err)

	_, err = b.CreateIAMPolicyAssignment(testAccountID, ns, "assign1", "", "arn:aws:iam::000000000000:policy/p1", nil)
	require.NoError(t, err)

	_, err = b.CreateAccountCustomization(testAccountID, ns, "theme1", "")
	require.NoError(t, err)

	_, err = b.CreateBrand(testAccountID, "brand1", map[string]any{"BrandName": "Brand1"}, nil)
	require.NoError(t, err)

	_, err = b.CreateCustomPermissions(testAccountID, "cp1", nil, nil)
	require.NoError(t, err)

	_, err = b.CreateOAuthClientApplication(testAccountID, "client1", "App1", nil, nil)
	require.NoError(t, err)

	err = b.UpdateIdentityPropagationConfig(
		testAccountID, "REDSHIFT", []string{"arn:aws:sso::000000000000:application/foo"},
	)
	require.NoError(t, err)

	_, err = b.StartAssetBundleExportJob(
		testAccountID, "exportjob1", "", "", []string{dash.Arn}, false, false, false, false,
	)
	require.NoError(t, err)

	_, err = b.StartAssetBundleImportJob(testAccountID, "importjob1", "")
	require.NoError(t, err)

	_, err = b.StartDashboardSnapshotJob(testAccountID, "dash1", "snapjob1", nil)
	require.NoError(t, err)

	_, err = b.CreateActionConnector(
		testAccountID, "ac1", "AC1", "GENERIC_HTTP", "", "",
		map[string]any{"AuthenticationType": "NO_AUTH"}, nil, nil,
	)
	require.NoError(t, err)

	autoJob, err := b.StartAutomationJob(testAccountID, "group1", "automation1", "")
	require.NoError(t, err)

	quicksight.SeedFlow(b, testAccountID, &quicksight.Flow{
		CreatedTime:     time.Now().UTC(),
		LastUpdatedTime: time.Now().UTC(),
		FlowID:          "flow1",
		Name:            "Flow1",
		PublishState:    "PUBLISHED",
	})

	flow2, err := b.CreateFlow(testAccountID, "Flow2", "", map[string]any{"steps": []any{}}, nil)
	require.NoError(t, err)

	agentCustomPrompt := &quicksight.CustomPromptProfile{
		ModelProfileID:  "profile1",
		QbsAwsAccountID: "999999999999",
		SubscriptionID:  "sub1",
	}
	_, err = b.CreateAgent(
		testAccountID, "agent1", "Agent1", "", "", "", "", nil, nil, nil, nil, nil, agentCustomPrompt,
	)
	require.NoError(t, err)

	_, err = b.CreateKnowledgeBase(
		testAccountID, "kb1", "KB1", "", "arn:aws:s3:::b", "", nil, nil, nil, nil, nil,
	)
	require.NoError(t, err)

	_, err = b.CreateSpace(testAccountID, "space1", "Space1", "")
	require.NoError(t, err)

	quicksight.SeedSelfUpgradeRequest(b, testAccountID, ns, &quicksight.SelfUpgradeRequestDetail{
		CreationTime:     time.Now().UTC().Unix(),
		UpgradeRequestID: "req1",
		OriginalRole:     "READER",
		RequestedRole:    "AUTHOR",
		RequestStatus:    "PENDING",
	})

	// ---- seed a representative sample of the maps left raw ----

	require.NoError(t, b.TagResource(dash.Arn, map[string]string{"env": "test"}))
	require.NoError(t, b.UpdatePublicSharingSettings(testAccountID, true))

	// ---- snapshot, then restore onto a brand-new backend ----

	snapshot := b.Snapshot(t.Context())
	require.NotNil(t, snapshot)

	restored := quicksight.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, restored.Restore(t.Context(), snapshot))

	// ---- verify every converted table's seeded entry survived ----

	_, err = restored.DescribeNamespace(testAccountID, ns)
	require.NoError(t, err)

	_, err = restored.DescribeGroup(testAccountID, ns, "group1")
	require.NoError(t, err)

	_, err = restored.DescribeGroupMembership(testAccountID, ns, "group1", "user1")
	require.NoError(t, err)

	_, err = restored.DescribeUser(testAccountID, ns, "user1")
	require.NoError(t, err)

	_, err = restored.DescribeDataSource(testAccountID, "ds1")
	require.NoError(t, err)

	_, err = restored.DescribeDataSet(testAccountID, "dset1")
	require.NoError(t, err)

	_, err = restored.DescribeIngestion(testAccountID, "dset1", "ingest1")
	require.NoError(t, err)

	restoredDash, err := restored.DescribeDashboard(testAccountID, "dash1")
	require.NoError(t, err)

	_, err = restored.DescribeAnalysis(testAccountID, "an1")
	require.NoError(t, err)

	_, err = restored.DescribeFolder(testAccountID, "folder1")
	require.NoError(t, err)

	members, _, err := restored.ListFolderMembers(testAccountID, "folder1", 0, "")
	require.NoError(t, err)
	assert.Len(t, members, 1)

	_, err = restored.DescribeTemplate(testAccountID, "tmpl1", 0)
	require.NoError(t, err)

	_, err = restored.DescribeTheme(testAccountID, "theme1", 0)
	require.NoError(t, err)

	_, err = restored.DescribeTopic(testAccountID, "topic1")
	require.NoError(t, err)

	_, err = restored.DescribeVPCConnection(testAccountID, "vpc1")
	require.NoError(t, err)

	_, err = restored.DescribeIAMPolicyAssignment(testAccountID, ns, "assign1")
	require.NoError(t, err)

	_, err = restored.DescribeAccountCustomization(testAccountID, ns, false)
	require.NoError(t, err)

	_, err = restored.DescribeBrand(testAccountID, "brand1", "")
	require.NoError(t, err)

	_, err = restored.DescribeCustomPermissions(testAccountID, "cp1")
	require.NoError(t, err)

	_, err = restored.DescribeOAuthClientApplication(testAccountID, "client1")
	require.NoError(t, err)

	idPropConfigs, err := restored.ListIdentityPropagationConfigs(testAccountID)
	require.NoError(t, err)
	assert.Len(t, idPropConfigs, 1)

	_, err = restored.DescribeAssetBundleExportJob(testAccountID, "exportjob1")
	require.NoError(t, err)

	_, err = restored.DescribeAssetBundleImportJob(testAccountID, "importjob1")
	require.NoError(t, err)

	_, err = restored.DescribeDashboardSnapshotJob(testAccountID, "dash1", "snapjob1")
	require.NoError(t, err)

	_, err = restored.DescribeActionConnector(testAccountID, "ac1")
	require.NoError(t, err)

	_, err = restored.DescribeAutomationJob(testAccountID, "group1", "automation1", autoJob.JobID)
	require.NoError(t, err)

	_, err = restored.GetFlowMetadata(testAccountID, "flow1")
	require.NoError(t, err)

	restoredFlow2, err := restored.DescribeFlow(testAccountID, flow2.FlowID)
	require.NoError(t, err)
	assert.Equal(t, "Flow2", restoredFlow2.Name)

	restoredAgent, err := restored.DescribeAgent(testAccountID, "agent1")
	require.NoError(t, err)
	assert.Equal(t, "Agent1", restoredAgent.Name)
	require.NotNil(t, restoredAgent.CustomPrompt)
	assert.Equal(t, "profile1", restoredAgent.CustomPrompt.ModelProfileID)
	assert.Equal(t, "999999999999", restoredAgent.CustomPrompt.QbsAwsAccountID)
	assert.Equal(t, "sub1", restoredAgent.CustomPrompt.SubscriptionID)

	restoredKB, err := restored.DescribeKnowledgeBase(testAccountID, "kb1")
	require.NoError(t, err)
	assert.Equal(t, "KB1", restoredKB.Name)

	restoredSpace, err := restored.DescribeSpace(testAccountID, "space1")
	require.NoError(t, err)
	assert.Equal(t, "Space1", restoredSpace.Name)

	selfUpgrades, _, err := restored.ListSelfUpgrades(testAccountID, ns, 0, "")
	require.NoError(t, err)
	assert.Len(t, selfUpgrades, 1)

	// ---- verify the raw-left maps survived too ----

	tags, err := restored.ListTagsForResource(restoredDash.Arn)
	require.NoError(t, err)
	assert.Equal(t, "test", tags["env"])

	settings, err := restored.DescribeAccountSettings(testAccountID)
	require.NoError(t, err)
	assert.True(t, settings.PublicSharingEnabled)

	// ---- sanity-check table counts via the export_test.go accessors ----
	assert.Equal(t, 2, quicksight.NamespaceCount(restored)) // default + ns
	assert.Equal(t, 1, quicksight.GroupCount(restored))
	assert.Equal(t, 1, quicksight.UserCount(restored))
	assert.Equal(t, 1, quicksight.DataSourceCount(restored))
	assert.Equal(t, 1, quicksight.DataSetCount(restored))
	assert.Equal(t, 1, quicksight.DashboardCount(restored))
	assert.Equal(t, 1, quicksight.AnalysisCount(restored))
	assert.Equal(t, 1, quicksight.FolderCount(restored))
	assert.Equal(t, 1, quicksight.TemplateCount(restored))
	assert.Equal(t, 1, quicksight.ThemeCount(restored))
	assert.Equal(t, 1, quicksight.VPCConnectionCount(restored))
	assert.Equal(t, 1, quicksight.BrandCount(restored))
}
