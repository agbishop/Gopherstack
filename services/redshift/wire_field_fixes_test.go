package redshift_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	redshiftsdk "github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// TestDescribeReservedNodeExchangeStatus_StatusIsLegalEnumMember drives
// DescribeReservedNodeExchangeStatus through the real aws-sdk-go-v2 client.
// ReservedNodeExchangeStatus.Status is types.ReservedNodeExchangeStatusType
// (REQUESTED/PENDING/IN_PROGRESS/RETRYING/SUCCEEDED/FAILED --
// redshift@v1.65.4 types/enums.go:468); the backend previously returned the
// bare string "Active" (borrowed from an unrelated PartnerIntegrationStatus
// constant), which is not a member of ReservedNodeExchangeStatusType, so a
// real client's waiter for an exchange request would never match any case
// and poll until timeout.
func TestDescribeReservedNodeExchangeStatus_StatusIsLegalEnumMember(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	backend.AddReservedNodeInternal(&redshift.ReservedNode{
		ReservedNodeID: "rn-exchange",
		State:          "active",
	})
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	out, err := client.DescribeReservedNodeExchangeStatus(ctx, &redshiftsdk.DescribeReservedNodeExchangeStatusInput{
		ReservedNodeId: aws.String("rn-exchange"),
	})
	require.NoError(t, err)
	require.Len(t, out.ReservedNodeExchangeStatusDetails, 1)
	assert.Equal(t, types.ReservedNodeExchangeStatusTypeSucceeded, out.ReservedNodeExchangeStatusDetails[0].Status)
}

// TestDescribeUsageLimits_FiltersByTagKeys drives DescribeUsageLimits through the
// real client with TagKeys set. DescribeUsageLimitsInput.TagKeys/TagValues are real,
// documented request fields (api_op_DescribeUsageLimits.go) that the handler
// previously never read at all, so any TagKeys/TagValues filter was silently
// ignored and every usage limit was returned regardless.
func TestDescribeUsageLimits_FiltersByTagKeys(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:  aws.String("ul-cluster"),
		NodeType:           aws.String("dc2.large"),
		MasterUsername:     aws.String("admin"),
		MasterUserPassword: aws.String("Password1"),
	})
	require.NoError(t, err)

	_, err = client.CreateUsageLimit(ctx, &redshiftsdk.CreateUsageLimitInput{
		ClusterIdentifier: aws.String("ul-cluster"),
		FeatureType:       types.UsageLimitFeatureTypeConcurrencyScaling,
		LimitType:         types.UsageLimitLimitTypeTime,
		Amount:            aws.Int64(60),
		Tags:              []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	_, err = client.CreateUsageLimit(ctx, &redshiftsdk.CreateUsageLimitInput{
		ClusterIdentifier: aws.String("ul-cluster"),
		FeatureType:       types.UsageLimitFeatureTypeSpectrum,
		LimitType:         types.UsageLimitLimitTypeDataScanned,
		Amount:            aws.Int64(10),
		Tags:              []types.Tag{{Key: aws.String("env"), Value: aws.String("staging")}},
	})
	require.NoError(t, err)

	out, err := client.DescribeUsageLimits(ctx, &redshiftsdk.DescribeUsageLimitsInput{
		TagValues: []string{"prod"},
	})
	require.NoError(t, err)
	require.Len(t, out.UsageLimits, 1)
	assert.Equal(t, types.UsageLimitFeatureTypeConcurrencyScaling, out.UsageLimits[0].FeatureType)
}

// TestDescribeHsmClientCertificates_FiltersByTagKeys drives
// DescribeHsmClientCertificates through the real client with TagKeys set.
// DescribeHsmClientCertificatesInput.TagKeys/TagValues (api_op_DescribeHsmClientCertificates.go)
// were previously never read by the handler, so the filter was a silent no-op.
func TestDescribeHsmClientCertificates_FiltersByTagKeys(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateHsmClientCertificate(ctx, &redshiftsdk.CreateHsmClientCertificateInput{
		HsmClientCertificateIdentifier: aws.String("cert-a"),
		Tags:                           []types.Tag{{Key: aws.String("team"), Value: aws.String("data")}},
	})
	require.NoError(t, err)

	_, err = client.CreateHsmClientCertificate(ctx, &redshiftsdk.CreateHsmClientCertificateInput{
		HsmClientCertificateIdentifier: aws.String("cert-b"),
		Tags:                           []types.Tag{{Key: aws.String("team"), Value: aws.String("platform")}},
	})
	require.NoError(t, err)

	out, err := client.DescribeHsmClientCertificates(ctx, &redshiftsdk.DescribeHsmClientCertificatesInput{
		TagKeys: []string{"nonexistent"},
	})
	require.NoError(t, err)
	assert.Empty(t, out.HsmClientCertificates)

	out, err = client.DescribeHsmClientCertificates(ctx, &redshiftsdk.DescribeHsmClientCertificatesInput{
		TagValues: []string{"data"},
	})
	require.NoError(t, err)
	require.Len(t, out.HsmClientCertificates, 1)
	assert.Equal(t, "cert-a", aws.ToString(out.HsmClientCertificates[0].HsmClientCertificateIdentifier))
}

// TestDescribeHsmConfigurations_FiltersByTagKeys mirrors the HsmClientCertificates
// case for DescribeHsmConfigurationsInput.TagKeys/TagValues.
func TestDescribeHsmConfigurations_FiltersByTagKeys(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateHsmConfiguration(ctx, &redshiftsdk.CreateHsmConfigurationInput{
		HsmConfigurationIdentifier: aws.String("cfg-a"),
		Description:                aws.String("d"),
		HsmIpAddress:               aws.String("10.0.0.1"),
		HsmPartitionName:           aws.String("p1"),
		HsmPartitionPassword:       aws.String("pw"),
		HsmServerPublicCertificate: aws.String("cert"),
		Tags:                       []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	_, err = client.CreateHsmConfiguration(ctx, &redshiftsdk.CreateHsmConfigurationInput{
		HsmConfigurationIdentifier: aws.String("cfg-b"),
		Description:                aws.String("d"),
		HsmIpAddress:               aws.String("10.0.0.2"),
		HsmPartitionName:           aws.String("p2"),
		HsmPartitionPassword:       aws.String("pw"),
		HsmServerPublicCertificate: aws.String("cert"),
		Tags:                       []types.Tag{{Key: aws.String("env"), Value: aws.String("staging")}},
	})
	require.NoError(t, err)

	out, err := client.DescribeHsmConfigurations(ctx, &redshiftsdk.DescribeHsmConfigurationsInput{
		TagValues: []string{"prod"},
	})
	require.NoError(t, err)
	require.Len(t, out.HsmConfigurations, 1)
	assert.Equal(t, "cfg-a", aws.ToString(out.HsmConfigurations[0].HsmConfigurationIdentifier))
}

// TestDescribeEndpointAccess_FiltersByResourceOwner drives DescribeEndpointAccess
// through the real client with ResourceOwner set. DescribeEndpointAccessInput.
// ResourceOwner (api_op_DescribeEndpointAccess.go) was previously never read by
// the handler, even though EndpointAccess.ResourceOwner is real backend data
// populated directly from CreateEndpointAccessInput.ResourceOwner.
func TestDescribeEndpointAccess_FiltersByResourceOwner(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:  aws.String("ep-cluster"),
		NodeType:           aws.String("dc2.large"),
		MasterUsername:     aws.String("admin"),
		MasterUserPassword: aws.String("Password1"),
	})
	require.NoError(t, err)

	_, err = client.CreateEndpointAccess(ctx, &redshiftsdk.CreateEndpointAccessInput{
		ClusterIdentifier: aws.String("ep-cluster"),
		EndpointName:      aws.String("ep-owner-a"),
		SubnetGroupName:   aws.String("default"),
		ResourceOwner:     aws.String("111111111111"),
	})
	require.NoError(t, err)

	_, err = client.CreateEndpointAccess(ctx, &redshiftsdk.CreateEndpointAccessInput{
		ClusterIdentifier: aws.String("ep-cluster"),
		EndpointName:      aws.String("ep-owner-b"),
		SubnetGroupName:   aws.String("default"),
		ResourceOwner:     aws.String("222222222222"),
	})
	require.NoError(t, err)

	out, err := client.DescribeEndpointAccess(ctx, &redshiftsdk.DescribeEndpointAccessInput{
		ResourceOwner: aws.String("111111111111"),
	})
	require.NoError(t, err)
	require.Len(t, out.EndpointAccessList, 1)
	assert.Equal(t, "ep-owner-a", aws.ToString(out.EndpointAccessList[0].EndpointName))
}

// TestDescribeScheduledActions_FiltersByActive drives DescribeScheduledActions
// through the real client with Active set. DescribeScheduledActionsInput.Active
// (api_op_DescribeScheduledActions.go) was previously never read, so it never
// excluded disabled scheduled actions from the response.
func TestDescribeScheduledActions_FiltersByActive(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateScheduledAction(ctx, &redshiftsdk.CreateScheduledActionInput{
		ScheduledActionName: aws.String("active-action"),
		Schedule:            aws.String("rate(1 day)"),
		IamRole:             aws.String("arn:aws:iam::000000000000:role/r"),
		Enable:              aws.Bool(true),
		TargetAction: &types.ScheduledActionType{
			PauseCluster: &types.PauseClusterMessage{ClusterIdentifier: aws.String("c1")},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateScheduledAction(ctx, &redshiftsdk.CreateScheduledActionInput{
		ScheduledActionName: aws.String("disabled-action"),
		Schedule:            aws.String("rate(1 day)"),
		IamRole:             aws.String("arn:aws:iam::000000000000:role/r"),
		Enable:              aws.Bool(false),
		TargetAction: &types.ScheduledActionType{
			PauseCluster: &types.PauseClusterMessage{ClusterIdentifier: aws.String("c2")},
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeScheduledActions(ctx, &redshiftsdk.DescribeScheduledActionsInput{
		Active: aws.Bool(true),
	})
	require.NoError(t, err)
	require.Len(t, out.ScheduledActions, 1)
	assert.Equal(t, "active-action", aws.ToString(out.ScheduledActions[0].ScheduledActionName))

	out, err = client.DescribeScheduledActions(ctx, &redshiftsdk.DescribeScheduledActionsInput{
		Active: aws.Bool(false),
	})
	require.NoError(t, err)
	require.Len(t, out.ScheduledActions, 1)
	assert.Equal(t, "disabled-action", aws.ToString(out.ScheduledActions[0].ScheduledActionName))
}

// TestDescribeClusterSnapshots_FiltersByStartTime drives DescribeClusterSnapshots
// through the real client with StartTime set. DescribeClusterSnapshotsInput.
// StartTime/EndTime (api_op_DescribeClusterSnapshots.go) were previously never
// read, even though Snapshot.SnapshotCreateTime is real backend data set at
// CreateClusterSnapshot time.
func TestDescribeClusterSnapshots_FiltersByStartTime(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:  aws.String("snap-cluster"),
		NodeType:           aws.String("dc2.large"),
		MasterUsername:     aws.String("admin"),
		MasterUserPassword: aws.String("Password1"),
	})
	require.NoError(t, err)

	before := time.Now().UTC()

	_, err = client.CreateClusterSnapshot(ctx, &redshiftsdk.CreateClusterSnapshotInput{
		SnapshotIdentifier: aws.String("snap-1"),
		ClusterIdentifier:  aws.String("snap-cluster"),
	})
	require.NoError(t, err)

	after := time.Now().UTC()

	out, err := client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		StartTime: aws.Time(after.Add(time.Hour)),
	})
	require.NoError(t, err)
	assert.Empty(t, out.Snapshots, "StartTime after snapshot creation must exclude it")

	out, err = client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		StartTime: aws.Time(before.Add(-time.Hour)),
	})
	require.NoError(t, err)
	require.Len(t, out.Snapshots, 1)
	assert.Equal(t, "snap-1", aws.ToString(out.Snapshots[0].SnapshotIdentifier))

	out, err = client.DescribeClusterSnapshots(ctx, &redshiftsdk.DescribeClusterSnapshotsInput{
		EndTime: aws.Time(before.Add(-time.Hour)),
	})
	require.NoError(t, err)
	assert.Empty(t, out.Snapshots, "EndTime before snapshot creation must exclude it")
}

// TestDescribeClusterParameters_FiltersBySource drives DescribeClusterParameters
// through the real client with Source set. DescribeClusterParametersInput.Source
// (api_op_DescribeClusterParameters.go) is a real request field the handler
// previously never read, so a real client's Source=user filter (only
// user-modified parameters) silently returned every parameter, engine-default
// included.
func TestDescribeClusterParameters_FiltersBySource(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateClusterParameterGroup(ctx, &redshiftsdk.CreateClusterParameterGroupInput{
		ParameterGroupName:   aws.String("src-pg"),
		ParameterGroupFamily: aws.String("redshift-1.0"),
		Description:          aws.String("d"),
	})
	require.NoError(t, err)

	_, err = client.ModifyClusterParameterGroup(ctx, &redshiftsdk.ModifyClusterParameterGroupInput{
		ParameterGroupName: aws.String("src-pg"),
		Parameters: []types.Parameter{
			{ParameterName: aws.String("enable_user_activity_logging"), ParameterValue: aws.String("true")},
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeClusterParameters(ctx, &redshiftsdk.DescribeClusterParametersInput{
		ParameterGroupName: aws.String("src-pg"),
		Source:             aws.String("user"),
	})
	require.NoError(t, err)
	require.Len(t, out.Parameters, 1)
	assert.Equal(t, "enable_user_activity_logging", aws.ToString(out.Parameters[0].ParameterName))

	all, err := client.DescribeClusterParameters(ctx, &redshiftsdk.DescribeClusterParametersInput{
		ParameterGroupName: aws.String("src-pg"),
	})
	require.NoError(t, err)
	assert.Greater(t, len(all.Parameters), 1, "unfiltered call must still return engine-default parameters")
}

// TestDescribeEventCategories_FiltersBySourceType drives DescribeEventCategories
// through the real client with SourceType set. DescribeEventCategoriesInput.SourceType
// is a real request field (5 legal values: cluster, cluster-snapshot,
// cluster-parameter-group, cluster-security-group, scheduled-action) the handler
// previously never read, so every SourceType request returned all 4 modeled groups.
func TestDescribeEventCategories_FiltersBySourceType(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	out, err := client.DescribeEventCategories(ctx, &redshiftsdk.DescribeEventCategoriesInput{
		SourceType: aws.String("cluster-snapshot"),
	})
	require.NoError(t, err)
	require.Len(t, out.EventCategoriesMapList, 1)
	assert.Equal(t, "cluster-snapshot", aws.ToString(out.EventCategoriesMapList[0].SourceType))

	all, err := client.DescribeEventCategories(ctx, &redshiftsdk.DescribeEventCategoriesInput{})
	require.NoError(t, err)
	assert.Greater(t, len(all.EventCategoriesMapList), 1, "unfiltered call must still return every source type")
}

// TestDescribeCustomDomainAssociations_FiltersByCertificateArn drives
// DescribeCustomDomainAssociations through the real client with
// CustomDomainCertificateArn set. DescribeCustomDomainAssociationsInput.
// CustomDomainCertificateArn is a real, populated backend field
// (CustomDomainAssociation.CustomDomainCertificateArn) the handler previously
// never read.
func TestDescribeCustomDomainAssociations_FiltersByCertificateArn(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:  aws.String("cd-cluster"),
		NodeType:           aws.String("dc2.large"),
		MasterUsername:     aws.String("admin"),
		MasterUserPassword: aws.String("Password1"),
	})
	require.NoError(t, err)

	_, err = client.CreateCustomDomainAssociation(ctx, &redshiftsdk.CreateCustomDomainAssociationInput{
		ClusterIdentifier:          aws.String("cd-cluster"),
		CustomDomainName:           aws.String("a.example.com"),
		CustomDomainCertificateArn: aws.String("arn:aws:acm:us-east-1:000000000000:certificate/aaa"),
	})
	require.NoError(t, err)

	_, err = client.CreateCustomDomainAssociation(ctx, &redshiftsdk.CreateCustomDomainAssociationInput{
		ClusterIdentifier:          aws.String("cd-cluster"),
		CustomDomainName:           aws.String("b.example.com"),
		CustomDomainCertificateArn: aws.String("arn:aws:acm:us-east-1:000000000000:certificate/bbb"),
	})
	require.NoError(t, err)

	out, err := client.DescribeCustomDomainAssociations(ctx, &redshiftsdk.DescribeCustomDomainAssociationsInput{
		CustomDomainCertificateArn: aws.String("arn:aws:acm:us-east-1:000000000000:certificate/aaa"),
	})
	require.NoError(t, err)
	require.Len(t, out.Associations, 1)
	assert.Equal(t,
		"arn:aws:acm:us-east-1:000000000000:certificate/aaa",
		aws.ToString(out.Associations[0].CustomDomainCertificateArn),
	)

	all, err := client.DescribeCustomDomainAssociations(ctx, &redshiftsdk.DescribeCustomDomainAssociationsInput{})
	require.NoError(t, err)
	assert.Len(t, all.Associations, 2, "unfiltered call must still return both associations")
}

// TestDescribeInboundIntegrations_ReturnsRealData drives DescribeInboundIntegrations
// through the real client. The handler previously ignored the request entirely and
// always returned an empty list, even though the backend's integrations store
// (populated by CreateIntegration) has real TargetArn/IntegrationArn data to serve
// this op from -- a full no-stub violation, not merely a dropped filter.
func TestDescribeInboundIntegrations_ReturnsRealData(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	created, err := client.CreateIntegration(ctx, &redshiftsdk.CreateIntegrationInput{
		IntegrationName: aws.String("inbound-ig"),
		SourceArn:       aws.String("arn:aws:rds:us-east-1:000000000000:cluster:src"),
		TargetArn:       aws.String("arn:aws:redshift-serverless:us-east-1:000000000000:namespace/ns-1"),
	})
	require.NoError(t, err)

	out, err := client.DescribeInboundIntegrations(ctx, &redshiftsdk.DescribeInboundIntegrationsInput{
		TargetArn: aws.String("arn:aws:redshift-serverless:us-east-1:000000000000:namespace/ns-1"),
	})
	require.NoError(t, err)
	require.Len(t, out.InboundIntegrations, 1)
	assert.Equal(t, aws.ToString(created.IntegrationArn), aws.ToString(out.InboundIntegrations[0].IntegrationArn))

	miss, err := client.DescribeInboundIntegrations(ctx, &redshiftsdk.DescribeInboundIntegrationsInput{
		TargetArn: aws.String("arn:aws:redshift-serverless:us-east-1:000000000000:namespace/no-such"),
	})
	require.NoError(t, err)
	assert.Empty(t, miss.InboundIntegrations)
}

// TestDescribeIntegrations_FiltersBySourceArn drives DescribeIntegrations through the
// real client with a source-arn Filters entry. DescribeIntegrationsInput.Filters is a
// real request field the handler previously never read at all.
func TestDescribeIntegrations_FiltersBySourceArn(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateIntegration(ctx, &redshiftsdk.CreateIntegrationInput{
		IntegrationName: aws.String("ig-a"),
		SourceArn:       aws.String("arn:aws:rds:us-east-1:000000000000:cluster:a"),
		TargetArn:       aws.String("arn:aws:redshift:us-east-1:000000000000:namespace:ns"),
	})
	require.NoError(t, err)

	_, err = client.CreateIntegration(ctx, &redshiftsdk.CreateIntegrationInput{
		IntegrationName: aws.String("ig-b"),
		SourceArn:       aws.String("arn:aws:rds:us-east-1:000000000000:cluster:b"),
		TargetArn:       aws.String("arn:aws:redshift:us-east-1:000000000000:namespace:ns"),
	})
	require.NoError(t, err)

	out, err := client.DescribeIntegrations(ctx, &redshiftsdk.DescribeIntegrationsInput{
		Filters: []types.DescribeIntegrationsFilter{
			{
				Name:   types.DescribeIntegrationsFilterNameSourceArn,
				Values: []string{"arn:aws:rds:us-east-1:000000000000:cluster:a"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Integrations, 1)
	assert.Equal(t, "ig-a", aws.ToString(out.Integrations[0].IntegrationName))
}

// TestDescribeSnapshotCopyGrants_FiltersByTagKeys drives DescribeSnapshotCopyGrants
// through the real client with TagValues set. DescribeSnapshotCopyGrantsInput.
// TagKeys/TagValues are real request fields the handler previously never read, even
// though SnapshotCopyGrant.Tags is real, populated backend data.
func TestDescribeSnapshotCopyGrants_FiltersByTagKeys(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateSnapshotCopyGrant(ctx, &redshiftsdk.CreateSnapshotCopyGrantInput{
		SnapshotCopyGrantName: aws.String("grant-prod"),
		Tags:                  []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	_, err = client.CreateSnapshotCopyGrant(ctx, &redshiftsdk.CreateSnapshotCopyGrantInput{
		SnapshotCopyGrantName: aws.String("grant-staging"),
		Tags:                  []types.Tag{{Key: aws.String("env"), Value: aws.String("staging")}},
	})
	require.NoError(t, err)

	out, err := client.DescribeSnapshotCopyGrants(ctx, &redshiftsdk.DescribeSnapshotCopyGrantsInput{
		TagValues: []string{"prod"},
	})
	require.NoError(t, err)
	require.Len(t, out.SnapshotCopyGrants, 1)
	assert.Equal(t, "grant-prod", aws.ToString(out.SnapshotCopyGrants[0].SnapshotCopyGrantName))
}

// TestDescribeSnapshotSchedules_FiltersByClusterIdentifier drives
// DescribeSnapshotSchedules through the real client with ClusterIdentifier set.
// DescribeSnapshotSchedulesInput.ClusterIdentifier is a real request field the
// handler previously never read, even though SnapshotSchedule.AssociatedClusters
// is real, derived backend data (see ModifyClusterSnapshotSchedule).
func TestDescribeSnapshotSchedules_FiltersByClusterIdentifier(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:  aws.String("sched-cluster"),
		NodeType:           aws.String("dc2.large"),
		MasterUsername:     aws.String("admin"),
		MasterUserPassword: aws.String("Password1"),
	})
	require.NoError(t, err)

	_, err = client.CreateSnapshotSchedule(ctx, &redshiftsdk.CreateSnapshotScheduleInput{
		ScheduleIdentifier:  aws.String("sched-a"),
		ScheduleDefinitions: []string{"rate(12 hours)"},
	})
	require.NoError(t, err)

	_, err = client.CreateSnapshotSchedule(ctx, &redshiftsdk.CreateSnapshotScheduleInput{
		ScheduleIdentifier:  aws.String("sched-b"),
		ScheduleDefinitions: []string{"rate(6 hours)"},
	})
	require.NoError(t, err)

	_, err = client.ModifyClusterSnapshotSchedule(ctx, &redshiftsdk.ModifyClusterSnapshotScheduleInput{
		ClusterIdentifier:  aws.String("sched-cluster"),
		ScheduleIdentifier: aws.String("sched-a"),
	})
	require.NoError(t, err)

	out, err := client.DescribeSnapshotSchedules(ctx, &redshiftsdk.DescribeSnapshotSchedulesInput{
		ClusterIdentifier: aws.String("sched-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, out.SnapshotSchedules, 1)
	assert.Equal(t, "sched-a", aws.ToString(out.SnapshotSchedules[0].ScheduleIdentifier))
}

// TestDescribeTableRestoreStatus_FiltersByRequestId drives DescribeTableRestoreStatus
// through the real client with TableRestoreRequestId set.
// DescribeTableRestoreStatusInput.TableRestoreRequestId is a real, populated backend
// field (TableRestoreStatus.TableRestoreRequestID) the handler previously never read.
func TestDescribeTableRestoreStatus_FiltersByRequestId(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:  aws.String("tr-cluster"),
		NodeType:           aws.String("dc2.large"),
		MasterUsername:     aws.String("admin"),
		MasterUserPassword: aws.String("Password1"),
	})
	require.NoError(t, err)

	first, err := client.RestoreTableFromClusterSnapshot(ctx, &redshiftsdk.RestoreTableFromClusterSnapshotInput{
		ClusterIdentifier:  aws.String("tr-cluster"),
		SnapshotIdentifier: aws.String("snap-1"),
		SourceDatabaseName: aws.String("db"),
		SourceTableName:    aws.String("t1"),
		TargetDatabaseName: aws.String("db"),
		NewTableName:       aws.String("t1_restored"),
	})
	require.NoError(t, err)

	_, err = client.RestoreTableFromClusterSnapshot(ctx, &redshiftsdk.RestoreTableFromClusterSnapshotInput{
		ClusterIdentifier:  aws.String("tr-cluster"),
		SnapshotIdentifier: aws.String("snap-1"),
		SourceDatabaseName: aws.String("db"),
		SourceTableName:    aws.String("t2"),
		TargetDatabaseName: aws.String("db"),
		NewTableName:       aws.String("t2_restored"),
	})
	require.NoError(t, err)

	wantID := aws.ToString(first.TableRestoreStatus.TableRestoreRequestId)
	require.NotEmpty(t, wantID)

	out, err := client.DescribeTableRestoreStatus(ctx, &redshiftsdk.DescribeTableRestoreStatusInput{
		TableRestoreRequestId: aws.String(wantID),
	})
	require.NoError(t, err)
	require.Len(t, out.TableRestoreStatusDetails, 1)
	assert.Equal(t, "t1", aws.ToString(out.TableRestoreStatusDetails[0].SourceTableName))
}

// TestCreateCluster_ClusterSecurityGroupsRoundTrip drives CreateCluster
// through the real client with ClusterSecurityGroups set and proves the
// association actually reaches the backend and reads back out through
// DescribeClusters, rather than being parsed and silently dropped.
// CreateClusterInput.ClusterSecurityGroups is a real, documented field
// (redshift@v1.65.4 api_op_CreateCluster.go: "A list of security groups to
// be associated with this cluster") that the backend previously had no
// field to store at all.
func TestCreateCluster_ClusterSecurityGroupsRoundTrip(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateClusterSecurityGroup(ctx, &redshiftsdk.CreateClusterSecurityGroupInput{
		ClusterSecurityGroupName: aws.String("csg-assoc"),
		Description:              aws.String("assoc test group"),
	})
	require.NoError(t, err)

	_, err = client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:     aws.String("csg-rt-cluster"),
		NodeType:              aws.String("dc2.large"),
		MasterUsername:        aws.String("admin"),
		MasterUserPassword:    aws.String("Password1"),
		ClusterSecurityGroups: []string{"csg-assoc"},
	})
	require.NoError(t, err)

	out, err := client.DescribeClusters(ctx, &redshiftsdk.DescribeClustersInput{
		ClusterIdentifier: aws.String("csg-rt-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, out.Clusters, 1)
	require.Len(t, out.Clusters[0].ClusterSecurityGroups, 1)
	assert.Equal(t, "csg-assoc", aws.ToString(out.Clusters[0].ClusterSecurityGroups[0].ClusterSecurityGroupName))

	// Real AWS: CreateCluster's own declared error switch includes
	// ClusterSecurityGroupNotFound for a group that doesn't exist.
	_, err = client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:     aws.String("csg-rt-cluster-2"),
		NodeType:              aws.String("dc2.large"),
		MasterUsername:        aws.String("admin"),
		MasterUserPassword:    aws.String("Password1"),
		ClusterSecurityGroups: []string{"no-such-group"},
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "ClusterSecurityGroupNotFound", apiErr.ErrorCode())
}

// TestCreateCluster_ClusterParameterGroupNameRoundTrip is the
// ClusterParameterGroupName analogue of
// TestCreateCluster_ClusterSecurityGroupsRoundTrip.
func TestCreateCluster_ClusterParameterGroupNameRoundTrip(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	client := newTestRedshiftClient(t, redshift.NewHandler(backend))
	ctx := t.Context()

	_, err := client.CreateClusterParameterGroup(ctx, &redshiftsdk.CreateClusterParameterGroupInput{
		ParameterGroupName:   aws.String("cpg-assoc"),
		ParameterGroupFamily: aws.String("redshift-1.0"),
		Description:          aws.String("assoc test group"),
	})
	require.NoError(t, err)

	_, err = client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:         aws.String("cpg-rt-cluster"),
		NodeType:                  aws.String("dc2.large"),
		MasterUsername:            aws.String("admin"),
		MasterUserPassword:        aws.String("Password1"),
		ClusterParameterGroupName: aws.String("cpg-assoc"),
	})
	require.NoError(t, err)

	out, err := client.DescribeClusters(ctx, &redshiftsdk.DescribeClustersInput{
		ClusterIdentifier: aws.String("cpg-rt-cluster"),
	})
	require.NoError(t, err)
	require.Len(t, out.Clusters, 1)
	require.Len(t, out.Clusters[0].ClusterParameterGroups, 1)
	assert.Equal(t, "cpg-assoc", aws.ToString(out.Clusters[0].ClusterParameterGroups[0].ParameterGroupName))

	_, err = client.CreateCluster(ctx, &redshiftsdk.CreateClusterInput{
		ClusterIdentifier:         aws.String("cpg-rt-cluster-2"),
		NodeType:                  aws.String("dc2.large"),
		MasterUsername:            aws.String("admin"),
		MasterUserPassword:        aws.String("Password1"),
		ClusterParameterGroupName: aws.String("no-such-group"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "ClusterParameterGroupNotFound", apiErr.ErrorCode())
}
