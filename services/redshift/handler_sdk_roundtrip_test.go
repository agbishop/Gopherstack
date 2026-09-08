package redshift_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	redshiftsdk "github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	smithy "github.com/aws/smithy-go"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// newTestRedshiftClient stands up the real aws-sdk-go-v2 Redshift client
// against an httptest server running this package's Handler, wired through
// the same pkgs/service registry/router used in production. Round-tripping
// through the genuine SDK deserializer (rather than string-matching the raw
// XML body) is what proves a response is wire-compatible: unrecognized list
// wrapper/element names are skipped silently by the deserializer rather than
// erroring, so a plausible-looking response can still decode to an empty
// slice.
func newTestRedshiftClient(t *testing.T, h *redshift.Handler) *redshiftsdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion(rtTestRegion),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return redshiftsdk.NewFromConfig(cfg, func(o *redshiftsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

const rtTestRegion = "us-east-1"

// TestPurchaseReservedNodeOffering_State proves the newly purchased
// ReservedNode.State value matches real AWS's documented wire string. Real
// ReservedNode.State is a plain *string (redshift@v1.65.4 types/types.go),
// not an enum, but its doc comment enumerates the legal values, and the
// pending-payment one is "pending-payment" (word order: state then reason);
// pre-fix, gopherstack emitted "payment-pending" (reason then state).
func TestPurchaseReservedNodeOffering_State(t *testing.T) {
	t.Parallel()

	h := redshift.NewHandler(redshift.NewInMemoryBackend("000000000000", rtTestRegion))
	client := newTestRedshiftClient(t, h)

	offerings, err := client.DescribeReservedNodeOfferings(
		t.Context(), &redshiftsdk.DescribeReservedNodeOfferingsInput{},
	)
	require.NoError(t, err)
	require.NotEmpty(t, offerings.ReservedNodeOfferings)

	out, err := client.PurchaseReservedNodeOffering(t.Context(), &redshiftsdk.PurchaseReservedNodeOfferingInput{
		ReservedNodeOfferingId: offerings.ReservedNodeOfferings[0].ReservedNodeOfferingId,
	})
	require.NoError(t, err)
	require.NotNil(t, out.ReservedNode)
	assert.Equal(t, "pending-payment", aws.ToString(out.ReservedNode.State))
}

// TestSDKRoundTrip_ListWrapperFixes covers six independent list-decoding
// bugs found by diffing every gopherstack redshift XML list tag against the
// pinned SDK's deserializer (redshift@v1.65.4): each handler wrapped list
// entries in the wrong element name, so a real client always saw an empty
// slice regardless of what the backend actually stored.
func TestSDKRoundTrip_ListWrapperFixes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		run  func(t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client)
		name string
	}{
		{testDescribeCustomDomainAssociations, "describe custom domain associations"},
		{testDescribeSnapshotSchedulesAssociatedClusters, "describe snapshot schedules associated clusters"},
		{testDescribeAuthenticationProfiles, "describe authentication profiles"},
		{testDescribeDataShares, "describe data shares"},
		{testDescribeEndpointAuthorization, "describe endpoint authorization"},
		{testDescribeUsageLimits, "describe usage limits"},
		{testDescribeEventCategories, "describe event categories"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
			h := redshift.NewHandler(backend)
			client := newTestRedshiftClient(t, h)

			tc.run(t, backend, client)
		})
	}
}

// testDescribeCustomDomainAssociations: the handler wrapped entries in
// <CustomDomainAssociations><CustomDomainAssociation>, but the real
// deserializer (redshift@v1.65.4 deserializers.go:49708,23893) names the
// field Associations and wraps each entry in <Association>.
func testDescribeCustomDomainAssociations(t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-cd-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = backend.CreateCustomDomainAssociation(
		"rt-cd-cluster", "db.example.com", "arn:aws:acm:us-east-1:000000000000:certificate/rt",
	)
	require.NoError(t, err)

	out, err := client.DescribeCustomDomainAssociations(ctx, &redshiftsdk.DescribeCustomDomainAssociationsInput{
		CustomDomainName: aws.String("db.example.com"),
	})
	require.NoError(t, err)
	require.Len(t, out.Associations, 1)
	assert.Equal(
		t,
		"arn:aws:acm:us-east-1:000000000000:certificate/rt",
		aws.ToString(out.Associations[0].CustomDomainCertificateArn),
	)
}

// testDescribeSnapshotSchedulesAssociatedClusters: the handler wrapped
// AssociatedClusters entries in <member>, but the real deserializer
// (redshift@v1.65.4 deserializers.go:23753) wraps each entry in
// <ClusterAssociatedToSchedule>.
func testDescribeSnapshotSchedulesAssociatedClusters(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-sched-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = backend.CreateSnapshotSchedule("rt-sched", "roundtrip test", []string{"rate(12 hours)"}, nil)
	require.NoError(t, err)

	require.NoError(t, backend.ModifyClusterSnapshotSchedule("rt-sched-cluster", "rt-sched", false))

	out, err := client.DescribeSnapshotSchedules(ctx, &redshiftsdk.DescribeSnapshotSchedulesInput{
		ScheduleIdentifier: aws.String("rt-sched"),
	})
	require.NoError(t, err)
	require.Len(t, out.SnapshotSchedules, 1)
	require.Len(t, out.SnapshotSchedules[0].AssociatedClusters, 1)
	assert.Equal(t, "rt-sched-cluster", aws.ToString(out.SnapshotSchedules[0].AssociatedClusters[0].ClusterIdentifier))
}

// testDescribeAuthenticationProfiles: the handler wrapped entries in
// <AuthenticationProfile>, but the real deserializer (redshift@v1.65.4
// deserializers.go:24257) wraps each entry in <member>.
func testDescribeAuthenticationProfiles(t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateAuthenticationProfile("rt-profile", `{"AllowedAllVPCs":true}`)
	require.NoError(t, err)

	out, err := client.DescribeAuthenticationProfiles(ctx, &redshiftsdk.DescribeAuthenticationProfilesInput{
		AuthenticationProfileName: aws.String("rt-profile"),
	})
	require.NoError(t, err)
	require.Len(t, out.AuthenticationProfiles, 1)
	assert.Equal(t, "rt-profile", aws.ToString(out.AuthenticationProfiles[0].AuthenticationProfileName))
}

// testDescribeDataShares: the handler wrapped entries in <DataShare>, but
// the real deserializer (redshift@v1.65.4 deserializers.go:29079) wraps
// each entry in <member>.
func testDescribeDataShares(t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client) {
	t.Helper()
	ctx := t.Context()

	const dataShareArn = "arn:aws:redshift:us-east-1:000000000000:datashare:rt-namespace/rt-share"
	backend.AddDataShareInternal(&redshift.DataShare{
		DataShareArn:  dataShareArn,
		ProducerArn:   "arn:aws:redshift:us-east-1:000000000000:namespace:rt-namespace",
		DataShareType: "INTERNAL",
	})

	out, err := client.DescribeDataShares(ctx, &redshiftsdk.DescribeDataSharesInput{
		DataShareArn: aws.String(dataShareArn),
	})
	require.NoError(t, err)
	require.Len(t, out.DataShares, 1)
	assert.Equal(t, dataShareArn, aws.ToString(out.DataShares[0].DataShareArn))
}

// testDescribeEndpointAuthorization: the handler wrapped entries in
// <EndpointAuthorization>, but the real deserializer (redshift@v1.65.4
// deserializers.go:30628) wraps each entry in <member>.
func testDescribeEndpointAuthorization(t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-epauth-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = backend.AuthorizeEndpointAccess("rt-epauth-cluster", "111122223333", nil)
	require.NoError(t, err)

	out, err := client.DescribeEndpointAuthorization(ctx, &redshiftsdk.DescribeEndpointAuthorizationInput{
		ClusterIdentifier: aws.String("rt-epauth-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, out.EndpointAuthorizationList, 1)
	assert.Equal(t, "111122223333", aws.ToString(out.EndpointAuthorizationList[0].Grantee))
}

// testDescribeUsageLimits: the handler wrapped entries in <UsageLimit>, but
// the real deserializer (redshift@v1.65.4 deserializers.go:45683) wraps
// each entry in <member>.
func testDescribeUsageLimits(t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-ul-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = backend.CreateUsageLimit("rt-ul-cluster", "concurrency-scaling", "time", "log", 60, nil)
	require.NoError(t, err)

	out, err := client.DescribeUsageLimits(ctx, &redshiftsdk.DescribeUsageLimitsInput{
		ClusterIdentifier: aws.String("rt-ul-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, out.UsageLimits, 1)
	assert.Equal(t, "rt-ul-cluster", aws.ToString(out.UsageLimits[0].ClusterIdentifier))
}

// testDescribeEventCategories: the handler emitted a flat EventCategory
// string on each EventCategoriesMap entry, but the real deserializer
// (redshift@v1.65.4 deserializers.go:31075) has no such field -- category
// names live in a nested Events list of EventInfoMap, each carrying its own
// EventCategories. A real client always saw SourceType populated but Events
// permanently empty.
func testDescribeEventCategories(t *testing.T, _ *redshift.InMemoryBackend, client *redshiftsdk.Client) {
	t.Helper()
	ctx := t.Context()

	out, err := client.DescribeEventCategories(ctx, &redshiftsdk.DescribeEventCategoriesInput{})
	require.NoError(t, err)
	require.NotEmpty(t, out.EventCategoriesMapList)

	var clusterEvents []string
	for _, m := range out.EventCategoriesMapList {
		if aws.ToString(m.SourceType) != "cluster" {
			continue
		}

		for _, ev := range m.Events {
			clusterEvents = append(clusterEvents, ev.EventCategories...)
		}
	}

	assert.Contains(t, clusterEvents, "maintenance")
}

// TestSDKRoundTrip_MutatingOpFixes covers gopherstack-7185: Create/Delete/Modify
// response shapes on classic Redshift, the class the List/Describe sweep never
// checked. Each case fails against the pre-fix code (verified by hand-reverting
// the corresponding source change and rerunning).
func TestSDKRoundTrip_MutatingOpFixes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		run  func(t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client)
		name string
	}{
		{testCreateCustomDomainAssociationCertExpiryTime, "create custom domain association cert expiry time"},
		{testModifyCustomDomainAssociationCertExpiryTime, "modify custom domain association cert expiry time"},
		{testCreateSnapshotScheduleTags, "create snapshot schedule tags"},
		{testBatchDeleteClusterSnapshotsRealWireShape, "batch delete cluster snapshots real wire shape"},
		{testModifyClusterDBRevisionClusterWrapper, "modify cluster db revision cluster wrapper"},
		{testListRecommendationsRecommendationType, "list recommendations recommendation type"},
		{testDescribeLoggingStatusReflectsRealState, "describe logging status reflects real state"},
		{testModifyClusterSnapshotOmittedRetentionPreserved, "modify cluster snapshot omitted retention preserved"},
		{
			testBatchModifyClusterSnapshotsOmittedRetentionPreserved,
			"batch modify cluster snapshots omitted retention preserved",
		},
		{
			testRevokeSnapshotAccessAuthorizationNotFoundErrorCode,
			"revoke snapshot access authorization not found error code",
		},
		{
			testRevokeClusterSecurityGroupIngressAuthorizationNotFoundErrorCode,
			"revoke cluster security group ingress authorization not found error code",
		},
		{
			testAuthorizeSnapshotAccessAlreadyExistsErrorCode,
			"authorize snapshot access already exists error code",
		},
		{
			testAuthorizeClusterSecurityGroupIngressAlreadyExistsErrorCode,
			"authorize cluster security group ingress already exists error code",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
			h := redshift.NewHandler(backend)
			client := newTestRedshiftClient(t, h)

			tc.run(t, backend, client)
		})
	}
}

// testCreateCustomDomainAssociationCertExpiryTime: CreateCustomDomainAssociationOutput
// carries CustomDomainCertExpiryTime (confirmed against
// aws-sdk-go-v2/service/redshift@v1.65.4/api_op_CreateCustomDomainAssociation.go),
// but the handler's response struct never had a field for it at all, so a real
// client always decoded an empty string.
func testCreateCustomDomainAssociationCertExpiryTime(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-cdexp-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	out, err := client.CreateCustomDomainAssociation(ctx, &redshiftsdk.CreateCustomDomainAssociationInput{
		ClusterIdentifier:          aws.String("rt-cdexp-cluster"),
		CustomDomainName:           aws.String("cdexp.example.com"),
		CustomDomainCertificateArn: aws.String("arn:aws:acm:us-east-1:000000000000:certificate/rt-cdexp"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, aws.ToString(out.CustomDomainCertExpiryTime))
}

// testModifyCustomDomainAssociationCertExpiryTime: same missing member on
// ModifyCustomDomainAssociationOutput.
func testModifyCustomDomainAssociationCertExpiryTime(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-cdexp-mod-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = backend.CreateCustomDomainAssociation(
		"rt-cdexp-mod-cluster", "cdexp-mod.example.com", "arn:aws:acm:us-east-1:000000000000:certificate/rt-old",
	)
	require.NoError(t, err)

	out, err := client.ModifyCustomDomainAssociation(ctx, &redshiftsdk.ModifyCustomDomainAssociationInput{
		ClusterIdentifier:          aws.String("rt-cdexp-mod-cluster"),
		CustomDomainName:           aws.String("cdexp-mod.example.com"),
		CustomDomainCertificateArn: aws.String("arn:aws:acm:us-east-1:000000000000:certificate/rt-new"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, aws.ToString(out.CustomDomainCertExpiryTime))
}

// testCreateSnapshotScheduleTags: CreateSnapshotScheduleOutput carries Tags
// (redshift@v1.65.4 deserializers.go:43027), and this backend already tracks
// SnapshotSchedule.Tags, but xmlSnapshotSchedule had no field for it, so a real
// client's tags on a newly created schedule always decoded to an empty slice.
func testCreateSnapshotScheduleTags(
	t *testing.T, _ *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	out, err := client.CreateSnapshotSchedule(ctx, &redshiftsdk.CreateSnapshotScheduleInput{
		ScheduleIdentifier:  aws.String("rt-sched-tags"),
		ScheduleDefinitions: []string{"rate(12 hours)"},
		Tags: []types.Tag{
			{Key: aws.String("env"), Value: aws.String("prod")},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Tags, 1)
	assert.Equal(t, "env", aws.ToString(out.Tags[0].Key))
	assert.Equal(t, "prod", aws.ToString(out.Tags[0].Value))
}

// testBatchDeleteClusterSnapshotsRealWireShape: BatchDeleteClusterSnapshotsInput.
// Identifiers is a list of DeleteClusterSnapshotMessage structs, serialized as
// "Identifiers.DeleteClusterSnapshotMessage.N.SnapshotIdentifier" (confirmed
// against aws-sdk-go-v2/service/redshift@v1.65.4/serializers.go). The handler
// previously read "Identifiers.DeleteClusterSnapshotMessage.N" and, failing
// that, "Identifiers.SnapshotIdentifier.N" -- neither is a key any real client
// ever sends, so every real BatchDeleteClusterSnapshots call silently deleted
// nothing while still returning 200 OK.
func testBatchDeleteClusterSnapshotsRealWireShape(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	backend.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "rt-batch-del-1", ClusterIdentifier: "c1", Status: "available"},
	)
	backend.AddSnapshotInternal(
		&redshift.Snapshot{SnapshotIdentifier: "rt-batch-del-2", ClusterIdentifier: "c1", Status: "available"},
	)

	out, err := client.BatchDeleteClusterSnapshots(ctx, &redshiftsdk.BatchDeleteClusterSnapshotsInput{
		Identifiers: []types.DeleteClusterSnapshotMessage{
			{SnapshotIdentifier: aws.String("rt-batch-del-1")},
			{SnapshotIdentifier: aws.String("rt-batch-del-2")},
		},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"rt-batch-del-1", "rt-batch-del-2"}, out.Resources)
	assert.Equal(t, 0, redshift.SnapshotCount(backend))
}

// testModifyClusterDBRevisionClusterWrapper: ModifyClusterDbRevisionOutput
// (redshift@v1.65.4 deserializers.go:52728) looks for a nested <Cluster>
// element inside ModifyClusterDbRevisionResult -- every other Cluster-
// returning op (ModifyCluster, RebootCluster, ...) wraps that way -- but the
// handler flattened xmlCluster's own fields directly under
// ModifyClusterDbRevisionResult with no <Cluster> wrapper, so a real client
// always decoded a nil Cluster.
func testModifyClusterDBRevisionClusterWrapper(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-dbrev-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	out, err := client.ModifyClusterDbRevision(ctx, &redshiftsdk.ModifyClusterDbRevisionInput{
		ClusterIdentifier: aws.String("rt-dbrev-cluster"),
		RevisionTarget:    aws.String("1"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Cluster)
	assert.Equal(t, "rt-dbrev-cluster", aws.ToString(out.Cluster.ClusterIdentifier))
}

// testListRecommendationsRecommendationType: Recommendation.RecommendationType
// (redshift@v1.65.4 deserializers.go around the Recommendation document
// deserializer) is the wire name for a recommendation's type, but the
// handler tagged that field "Type" -- a name the real deserializer never
// matches -- so a real client's RecommendationType always decoded nil even
// though the backend always populates a value (e.g. "Security").
func testListRecommendationsRecommendationType(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-rec-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	out, err := client.ListRecommendations(ctx, &redshiftsdk.ListRecommendationsInput{
		ClusterIdentifier: aws.String("rt-rec-cluster"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Recommendations)
	assert.Equal(t, "Security", aws.ToString(out.Recommendations[0].RecommendationType))
}

// testDescribeLoggingStatusReflectsRealState: DescribeLoggingStatus never
// read ClusterIdentifier and never consulted the backend's loggingStatuses
// map (which EnableLogging/DisableLogging already populate) -- it always
// returned a hardcoded LoggingEnabled=false, so a real client could never
// observe logging state it had itself just enabled.
func testDescribeLoggingStatusReflectsRealState(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-logstatus-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = client.EnableLogging(ctx, &redshiftsdk.EnableLoggingInput{
		ClusterIdentifier: aws.String("rt-logstatus-cluster"),
		BucketName:        aws.String("rt-log-bucket"),
	})
	require.NoError(t, err)

	out, err := client.DescribeLoggingStatus(ctx, &redshiftsdk.DescribeLoggingStatusInput{
		ClusterIdentifier: aws.String("rt-logstatus-cluster"),
	})
	require.NoError(t, err)
	assert.True(t, aws.ToBool(out.LoggingEnabled))
	assert.Equal(t, "rt-log-bucket", aws.ToString(out.BucketName))

	_, err = client.DisableLogging(ctx, &redshiftsdk.DisableLoggingInput{
		ClusterIdentifier: aws.String("rt-logstatus-cluster"),
	})
	require.NoError(t, err)

	out, err = client.DescribeLoggingStatus(ctx, &redshiftsdk.DescribeLoggingStatusInput{
		ClusterIdentifier: aws.String("rt-logstatus-cluster"),
	})
	require.NoError(t, err)
	assert.False(t, aws.ToBool(out.LoggingEnabled))
}

// testModifyClusterSnapshotOmittedRetentionPreserved: ManualSnapshotRetentionPeriod
// is optional on ModifyClusterSnapshotInput (*int32, no "required" doc comment,
// confirmed against aws-sdk-go-v2/service/redshift@v1.65.4/api_op_ModifyClusterSnapshot.go).
// The handler used an int sentinel of -1 for "omitted", indistinguishable from
// a real, explicit ManualSnapshotRetentionPeriod=-1 ("retain indefinitely"), so
// a Force-only call silently reset every snapshot's real retention period.
func testModifyClusterSnapshotOmittedRetentionPreserved(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-modsnap-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier:            "rt-modsnap-1",
		ClusterIdentifier:             "rt-modsnap-cluster",
		Status:                        "available",
		ManualSnapshotRetentionPeriod: 30,
	})

	out, err := client.ModifyClusterSnapshot(ctx, &redshiftsdk.ModifyClusterSnapshotInput{
		SnapshotIdentifier: aws.String("rt-modsnap-1"),
		Force:              aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Snapshot)
	assert.EqualValues(t, 30, aws.ToInt32(out.Snapshot.ManualSnapshotRetentionPeriod))
}

// testBatchModifyClusterSnapshotsOmittedRetentionPreserved: same optional-field
// bug as ModifyClusterSnapshot, present in the sibling batch op sharing the
// exact same request field (BatchModifyClusterSnapshotsInput.ManualSnapshotRetentionPeriod,
// also *int32, also undocumented as required).
func testBatchModifyClusterSnapshotsOmittedRetentionPreserved(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-batchmodsnap-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier:            "rt-batchmodsnap-1",
		ClusterIdentifier:             "rt-batchmodsnap-cluster",
		Status:                        "available",
		ManualSnapshotRetentionPeriod: 45,
	})

	out, err := client.BatchModifyClusterSnapshots(ctx, &redshiftsdk.BatchModifyClusterSnapshotsInput{
		SnapshotIdentifierList: []string{"rt-batchmodsnap-1"},
		Force:                  aws.Bool(true),
	})
	require.NoError(t, err)
	assert.Empty(t, out.Errors)
	assert.Contains(t, out.Resources, "rt-batchmodsnap-1")

	describeOut, err := client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		SnapshotIdentifier: aws.String("rt-batchmodsnap-1"),
	})
	require.NoError(t, err)
	require.Len(t, describeOut.Snapshots, 1)
	assert.EqualValues(t, 45, aws.ToInt32(describeOut.Snapshots[0].ManualSnapshotRetentionPeriod))
}

// testRevokeSnapshotAccessAuthorizationNotFoundErrorCode: RevokeSnapshotAccess's
// own declared error switch (redshift@v1.65.4 deserializers.go,
// awsAwsquery_deserializeOpErrorRevokeSnapshotAccess) lists
// AccessToSnapshotDenied/AuthorizationNotFound/ClusterSnapshotNotFound/
// UnsupportedOperation -- no InvalidParameterValue-shaped fault at all -- so
// revoking access for an account that was never granted it must surface
// AuthorizationNotFound, not the handler's previous generic
// InvalidParameterValue, which a real client's errors.As(*types.AuthorizationNotFoundFault)
// would never match.
func testRevokeSnapshotAccessAuthorizationNotFoundErrorCode(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	_, err := backend.CreateCluster("rt-revoke-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)
	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "rt-revoke-snap",
		ClusterIdentifier:  "rt-revoke-cluster",
		Status:             "available",
	})

	_, err = client.RevokeSnapshotAccess(ctx, &redshiftsdk.RevokeSnapshotAccessInput{
		SnapshotIdentifier:       aws.String("rt-revoke-snap"),
		AccountWithRestoreAccess: aws.String("999999999999"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "AuthorizationNotFound", apiErr.ErrorCode())
}

// testAuthorizeSnapshotAccessAlreadyExistsErrorCode: this op's own declared
// error switch (redshift@v1.65.4 deserializers.go,
// awsAwsquery_deserializeOpErrorAuthorizeSnapshotAccess) lists
// AuthorizationAlreadyExists, the same fault AuthorizeEndpointAccess's sibling
// grant-list op already enforces for an identical re-grant -- so authorizing
// an account that already has restore access must error, not silently add a
// second entry.
func testAuthorizeSnapshotAccessAlreadyExistsErrorCode(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	backend.AddSnapshotInternal(&redshift.Snapshot{
		SnapshotIdentifier: "rt-authz-dup-snap",
		ClusterIdentifier:  "rt-authz-dup-cluster",
		Status:             "available",
	})

	_, err := client.AuthorizeSnapshotAccess(ctx, &redshiftsdk.AuthorizeSnapshotAccessInput{
		SnapshotIdentifier:       aws.String("rt-authz-dup-snap"),
		AccountWithRestoreAccess: aws.String("999999999999"),
	})
	require.NoError(t, err)

	_, err = client.AuthorizeSnapshotAccess(ctx, &redshiftsdk.AuthorizeSnapshotAccessInput{
		SnapshotIdentifier:       aws.String("rt-authz-dup-snap"),
		AccountWithRestoreAccess: aws.String("999999999999"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "AuthorizationAlreadyExists", apiErr.ErrorCode())
}

// testAuthorizeClusterSecurityGroupIngressAlreadyExistsErrorCode: this op's
// own declared error switch (redshift@v1.65.4 deserializers.go,
// awsAwsquery_deserializeOpErrorAuthorizeClusterSecurityGroupIngress) lists
// AuthorizationAlreadyExists -- same fault family and reasoning as
// AuthorizeSnapshotAccess above.
func testAuthorizeClusterSecurityGroupIngressAlreadyExistsErrorCode(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	backend.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{
		ClusterSecurityGroupName: "rt-authz-dup-sg",
	})

	_, err := client.AuthorizeClusterSecurityGroupIngress(
		ctx, &redshiftsdk.AuthorizeClusterSecurityGroupIngressInput{
			ClusterSecurityGroupName: aws.String("rt-authz-dup-sg"),
			CIDRIP:                   aws.String("10.0.0.0/8"),
		})
	require.NoError(t, err)

	_, err = client.AuthorizeClusterSecurityGroupIngress(
		ctx, &redshiftsdk.AuthorizeClusterSecurityGroupIngressInput{
			ClusterSecurityGroupName: aws.String("rt-authz-dup-sg"),
			CIDRIP:                   aws.String("10.0.0.0/8"),
		})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "AuthorizationAlreadyExists", apiErr.ErrorCode())
}

// testRevokeClusterSecurityGroupIngressAuthorizationNotFoundErrorCode: this op's
// own declared error switch (redshift@v1.65.4 deserializers.go,
// awsAwsquery_deserializeOpErrorRevokeClusterSecurityGroupIngress) lists
// AuthorizationNotFound/ClusterSecurityGroupNotFound/InvalidClusterSecurityGroupState
// -- same fault family RevokeSnapshotAccess declares for the identical
// nothing-to-revoke condition -- so revoking a CIDR that was never authorized
// must surface AuthorizationNotFound, not silently succeed with the group
// unchanged.
func testRevokeClusterSecurityGroupIngressAuthorizationNotFoundErrorCode(
	t *testing.T, backend *redshift.InMemoryBackend, client *redshiftsdk.Client,
) {
	t.Helper()
	ctx := t.Context()

	backend.AddSecurityGroupInternal(&redshift.ClusterSecurityGroup{
		ClusterSecurityGroupName: "rt-revoke-sg",
		IPRanges:                 []redshift.IPRange{{CIDRIP: "10.0.0.0/8", Status: "authorized"}},
	})

	_, err := client.RevokeClusterSecurityGroupIngress(ctx, &redshiftsdk.RevokeClusterSecurityGroupIngressInput{
		ClusterSecurityGroupName: aws.String("rt-revoke-sg"),
		CIDRIP:                   aws.String("192.168.0.0/16"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "AuthorizationNotFound", apiErr.ErrorCode())
}

// TestDescribeNodeConfigurationOptions_FilterWireKey drives a real
// aws-sdk-go-v2 client with typed Filters. redshift@v1.65.4 serializers.go
// (awsAwsquery_serializeOpDocumentDescribeNodeConfigurationOptionsInput,
// awsAwsquery_serializeDocumentNodeConfigurationOptionsFilter,
// awsAwsquery_serializeDocumentValueStringList) puts each filter value on
// the wire as "Filter.NodeConfigurationOptionsFilter.N.Value.item.M" --
// singular "Value" wrapping an "item" list, not the plural
// "...Values.M" the handler's nodeConfigFilterValue looked for. A real
// client's filters were silently ignored entirely.
func TestDescribeNodeConfigurationOptions_FilterWireKey(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	t.Run("NodeType filter selects the requested target", func(t *testing.T) {
		t.Parallel()

		out, err := client.DescribeNodeConfigurationOptions(ctx, &redshiftsdk.DescribeNodeConfigurationOptionsInput{
			ActionType: types.ActionTypeRecommendNodeConfig,
			Filters: []types.NodeConfigurationOptionsFilter{
				{
					Name:     types.NodeConfigurationOptionsFilterNameNodeType,
					Operator: types.OperatorTypeEq,
					Values:   []string{"ra3.4xlarge"},
				},
			},
		})
		require.NoError(t, err)
		require.NotEmpty(t, out.NodeConfigurationOptionList)
		assert.Equal(t, "ra3.4xlarge", aws.ToString(out.NodeConfigurationOptionList[0].NodeType))
	})

	t.Run("NumberOfNodes filter narrows the result set", func(t *testing.T) {
		t.Parallel()

		out, err := client.DescribeNodeConfigurationOptions(ctx, &redshiftsdk.DescribeNodeConfigurationOptionsInput{
			ActionType: types.ActionTypeRecommendNodeConfig,
			Filters: []types.NodeConfigurationOptionsFilter{
				{
					Name:     types.NodeConfigurationOptionsFilterNameNumNodes,
					Operator: types.OperatorTypeEq,
					Values:   []string{"4"},
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, out.NodeConfigurationOptionList, 1)
		assert.EqualValues(t, 4, aws.ToInt32(out.NodeConfigurationOptionList[0].NumberOfNodes))
	})

	// NodeConfigurationOptionsFilter.Operator (types/types.go:1379-1388) documents
	// gt/lt/le/ge/between/in alongside eq -- "Provide one value to evaluate for
	// 'eq', 'lt', 'le', 'gt', and 'ge'. Provide two values to evaluate for
	// 'between'." The Eq-only subtest above cannot see an operator that is parsed
	// and then ignored (every filter always compared with ==), because Eq's
	// wrong-in-every-way and Eq's right-by-coincidence result are identical.
	t.Run("NumberOfNodes filter honours the gt operator", func(t *testing.T) {
		t.Parallel()

		out, err := client.DescribeNodeConfigurationOptions(ctx, &redshiftsdk.DescribeNodeConfigurationOptionsInput{
			ActionType: types.ActionTypeRecommendNodeConfig,
			Filters: []types.NodeConfigurationOptionsFilter{
				{
					Name:     types.NodeConfigurationOptionsFilterNameNumNodes,
					Operator: types.OperatorTypeGt,
					Values:   []string{"4"},
				},
			},
		})
		require.NoError(t, err)

		got := make([]int32, 0, len(out.NodeConfigurationOptionList))
		for _, o := range out.NodeConfigurationOptionList {
			got = append(got, aws.ToInt32(o.NumberOfNodes))
		}

		assert.ElementsMatch(t, []int32{8}, got, "gt 4 must return only the 8-node option")
	})

	t.Run("NumberOfNodes filter honours the between operator", func(t *testing.T) {
		t.Parallel()

		out, err := client.DescribeNodeConfigurationOptions(ctx, &redshiftsdk.DescribeNodeConfigurationOptionsInput{
			ActionType: types.ActionTypeRecommendNodeConfig,
			Filters: []types.NodeConfigurationOptionsFilter{
				{
					Name:     types.NodeConfigurationOptionsFilterNameNumNodes,
					Operator: types.OperatorTypeBetween,
					Values:   []string{"2", "4"},
				},
			},
		})
		require.NoError(t, err)

		got := make([]int32, 0, len(out.NodeConfigurationOptionList))
		for _, o := range out.NodeConfigurationOptionList {
			got = append(got, aws.ToInt32(o.NumberOfNodes))
		}

		assert.ElementsMatch(t, []int32{2, 4}, got, "between 2 and 4 must include both inclusive bounds, exclude 8")
	})
}

// TestCreateSnapshotSchedule_ScheduleDefinitionsWireKey drives a real
// aws-sdk-go-v2 client. redshift@v1.65.4 serializers.go
// (awsAwsquery_serializeDocumentScheduleDefinitionList) puts each entry on
// the wire as "ScheduleDefinitions.ScheduleDefinition.N" -- the handler's
// parseStringList call was missing the separating "." before the index, so
// the key it looked for ("ScheduleDefinitions.ScheduleDefinition" + N, with
// no dot) never matched anything a real client sent.
func TestCreateSnapshotSchedule_ScheduleDefinitionsWireKey(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
	h := redshift.NewHandler(backend)
	client := newTestRedshiftClient(t, h)
	ctx := t.Context()

	out, err := client.CreateSnapshotSchedule(ctx, &redshiftsdk.CreateSnapshotScheduleInput{
		ScheduleIdentifier: aws.String("rt-sched-wire"),
		ScheduleDefinitions: []string{
			"rate(12 hours)",
			"cron(30 4 * * ? *)",
		},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"rate(12 hours)", "cron(30 4 * * ? *)"}, out.ScheduleDefinitions)
}
