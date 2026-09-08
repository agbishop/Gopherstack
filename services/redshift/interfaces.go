package redshift

import "context"

// StorageBackend defines the interface for Redshift backend implementations.
// All mutating methods must be safe for concurrent use.
type StorageBackend interface {
	// Cluster operations
	CreateCluster(
		id, nodeType, dbName, masterUser string,
		clusterSecurityGroups []string,
		clusterParameterGroupName string,
	) (*Cluster, error)
	DeleteCluster(id string) (*Cluster, error)
	DescribeClusters(id, marker string, maxRecords int, tagKeys, tagValues []string) ([]Cluster, string, error)
	ModifyCluster(id string, opts ModifyClusterOptions) (*Cluster, error)
	RebootCluster(id string) (*Cluster, error)
	PauseCluster(id string) (*Cluster, error)
	ResumeCluster(id string) (*Cluster, error)
	ResizeCluster(id, nodeType, clusterType string, numberOfNodes int, classic bool) (*Cluster, error)
	RotateEncryptionKey(id string) (*Cluster, error)
	ModifyClusterIamRoles(id string, addRoles, removeRoles []string) (*Cluster, error)
	ModifyClusterMaintenance(id, maintenanceTrack string, deferMaintenance bool) (*Cluster, error)
	ModifyAquaConfiguration(id string) (*Cluster, error)
	ModifyLakehouseConfiguration(p ModifyLakehouseConfigParams) (*ClusterLakehouseConfigResult, error)

	// Tag operations
	DescribeTags() map[string]map[string]string
	CreateTags(clusterID string, kv map[string]string) error
	DeleteTags(clusterID string, keys []string) error

	// Parameter group operations
	CreateClusterParameterGroup(name, family, description string) (*ClusterParameterGroup, error)
	DeleteClusterParameterGroup(name string) error
	DescribeClusterParameterGroups(name string) ([]ClusterParameterGroup, error)
	DescribeClusterParameters(groupName string) ([]ClusterParameter, error)
	ModifyClusterParameterGroup(groupName string, params []ClusterParameter) (*ClusterParameterGroup, error)
	ResetClusterParameterGroup(
		groupName string,
		resetAllParameters bool,
		params []ClusterParameter,
	) (*ClusterParameterGroup, error)
	DescribeDefaultClusterParameters(family string) ([]ClusterParameter, error)

	// Reserved node operations
	AcceptReservedNodeExchange(reservedNodeID, targetOfferingID string) (*ReservedNode, error)
	DescribeReservedNodes(reservedNodeID string) ([]ReservedNode, error)
	DescribeReservedNodeOfferings(offeringID string) ([]ReservedNodeOffering, error)
	PurchaseReservedNodeOffering(offeringID, reservedNodeID string, nodeCount int) (*ReservedNode, error)
	DescribeReservedNodeExchangeStatus(reservedNodeID string) (string, error)
	GetReservedNodeExchangeOfferings(reservedNodeID string) ([]ReservedNodeOffering, error)

	// Security group operations
	AuthorizeClusterSecurityGroupIngress(
		groupName, cidrIP, ec2GroupName, ec2GroupOwnerID string,
	) (*ClusterSecurityGroup, error)
	CreateClusterSecurityGroup(name, description string, tags map[string]string) (*ClusterSecurityGroup, error)
	DeleteClusterSecurityGroup(name string) error
	DescribeClusterSecurityGroups(
		name, marker string, maxRecords int, tagKeys, tagValues []string,
	) ([]ClusterSecurityGroup, string, error)
	RevokeClusterSecurityGroupIngress(
		groupName, cidrIP, ec2GroupName, ec2GroupOwnerID string,
	) (*ClusterSecurityGroup, error)

	// Snapshot operations
	CreateClusterSnapshot(snapshotID, clusterID string) (*Snapshot, error)
	DeleteClusterSnapshot(snapshotID string) (*Snapshot, error)
	DescribeClusterSnapshots(snapshotID, clusterID, snapshotType string, clusterExists *bool) ([]Snapshot, error)
	CopyClusterSnapshot(sourceSnapshotID, destinationSnapshotID string) (*Snapshot, error)
	RestoreFromClusterSnapshot(clusterID, snapshotID string) (*Cluster, error)
	AuthorizeSnapshotAccess(snapshotID, accountWithRestoreAccess string) (*Snapshot, error)
	BatchDeleteClusterSnapshots(identifiers []string) ([]SnapshotBatchError, []string)
	BatchModifyClusterSnapshots(identifiers []string, retentionPeriod *int, force bool) ([]SnapshotBatchError, []string)

	// Subnet group operations
	CreateClusterSubnetGroup(
		name, description, vpcID string, subnetIDs []string, tags map[string]string,
	) (*ClusterSubnetGroup, error)
	DeleteClusterSubnetGroup(name string) error
	DescribeClusterSubnetGroups(
		name, marker string, maxRecords int, tagKeys, tagValues []string,
	) ([]ClusterSubnetGroup, string, error)
	ModifyClusterSubnetGroup(name, description string, subnetIDs []string) (*ClusterSubnetGroup, error)

	// Endpoint operations
	AuthorizeEndpointAccess(clusterID, grantee string, vpcIDs []string) (*EndpointAuthorization, error)
	DescribeEndpointAuthorization(clusterID, account string, grantee bool) ([]EndpointAuthorization, error)
	RevokeEndpointAccess(clusterID, account string, vpcIDs []string, force bool) (*EndpointAuthorization, error)

	// Data share operations
	AddPartner(accountID, clusterID, databaseName, partnerName string) (*Partner, error)
	AssociateDataShareConsumer(
		dataShareArn, consumerArn, consumerRegion string,
		associateEntireAccount bool,
	) (*DataShare, error)
	AuthorizeDataShare(dataShareArn, consumerIdentifier string) (*DataShare, error)
	DeauthorizeDataShare(dataShareArn, consumerIdentifier string) (*DataShare, error)
	DescribeDataShares(dataShareArn string) ([]DataShare, error)
	DescribeDataSharesForConsumer(consumerArn, status string) ([]DataShare, error)
	DescribeDataSharesForProducer(producerArn, status string) ([]DataShare, error)
	DisassociateDataShareConsumer(
		dataShareArn, consumerArn, consumerRegion string,
		disassociateEntireAccount bool,
	) (*DataShare, error)
	RejectDataShare(dataShareArn string) (*DataShare, error)

	// Partner operations
	DeletePartner(accountID, clusterID, databaseName, partnerName string) error
	DescribePartners(accountID, clusterID, databaseName, partnerName string) ([]Partner, error)
	UpdatePartnerStatus(accountID, clusterID, databaseName, partnerName, status, statusMessage string) (*Partner, error)

	// Resize operations
	CancelResize(clusterID string) (*ResizeProgress, error)
	DescribeResize(clusterID string) (*ResizeProgress, error)

	// Snapshot management
	ModifyClusterSnapshot(snapshotID string, retentionPeriod *int, force bool) (*Snapshot, error)
	RevokeSnapshotAccess(snapshotID, accountWithRestoreAccess string) (*Snapshot, error)

	// Credentials
	GetClusterCredentials(clusterID, dbUser string, autoCreate bool) (*ClusterCredentials, error)

	// Logging operations
	EnableLogging(clusterID, bucketName, s3KeyPrefix string) (*LoggingStatus, error)
	DisableLogging(clusterID string) (*LoggingStatus, error)
	GetLoggingStatus(clusterID string) (*LoggingStatus, error)

	// Event operations
	DescribeEvents(sourceIdentifier, sourceType string) ([]Event, error)
	CreateEventSubscription(
		subscriptionName, snsTopicArn, sourceType, severity string,
		sourceIDs, eventCategories []string,
		enabled bool,
	) (*EventSubscription, error)
	DeleteEventSubscription(subscriptionName string) error
	DescribeEventSubscriptions(subscriptionName string) ([]EventSubscription, error)
	ModifyEventSubscription(
		subscriptionName, snsTopicArn, sourceType, severity string,
		sourceIDs, eventCategories []string,
		enabled *bool,
	) (*EventSubscription, error)

	// Seed helpers for tests
	AddReservedNodeInternal(node *ReservedNode)
	AddDataShareInternal(ds *DataShare)
	AddSecurityGroupInternal(sg *ClusterSecurityGroup)
	AddSnapshotInternal(snap *Snapshot)
	AddActiveResizeInternal(clusterID string, resize *ResizeProgress)
	AddParameterGroupInternal(pg *ClusterParameterGroup)
	AddSubnetGroupInternal(sg *ClusterSubnetGroup)

	// Snapshot copy grant operations
	CreateSnapshotCopyGrant(name, kmsKeyID string, tags map[string]string) (*SnapshotCopyGrant, error)
	DeleteSnapshotCopyGrant(name string) error
	DescribeSnapshotCopyGrants(name string) ([]SnapshotCopyGrant, error)

	// Snapshot copy operations
	EnableSnapshotCopy(clusterID, destinationRegion, grantName string, retentionPeriod int) (*Cluster, error)
	DisableSnapshotCopy(clusterID string) (*Cluster, error)
	ModifySnapshotCopyRetentionPeriod(clusterID string, retentionPeriod int) (*Cluster, error)

	// Snapshot schedule operations
	CreateSnapshotSchedule(
		scheduleID, description string,
		definitions []string,
		tags map[string]string,
	) (*SnapshotSchedule, error)
	DeleteSnapshotSchedule(scheduleID string) error
	DescribeSnapshotSchedules(scheduleID string) ([]SnapshotSchedule, error)
	ModifySnapshotSchedule(scheduleID string, definitions []string) (*SnapshotSchedule, error)
	ModifyClusterSnapshotSchedule(clusterID, scheduleID string, disassociate bool) error

	// Usage limit operations
	CreateUsageLimit(
		clusterID, featureType, limitType, breachAction string,
		amount int64,
		tags map[string]string,
	) (*UsageLimit, error)
	DeleteUsageLimit(usageLimitID string) error
	DescribeUsageLimits(clusterID, featureType string) ([]UsageLimit, error)
	ModifyUsageLimit(usageLimitID, breachAction string, amount int64) (*UsageLimit, error)

	// Authentication profile operations
	CreateAuthenticationProfile(name, content string) (*AuthenticationProfile, error)
	DeleteAuthenticationProfile(name string) error
	DescribeAuthenticationProfiles(name string) ([]AuthenticationProfile, error)
	ModifyAuthenticationProfile(name, content string) (*AuthenticationProfile, error)

	// Resource policy operations
	GetResourcePolicy(resourceArn string) (*ResourcePolicy, error)
	PutResourcePolicy(resourceArn, policy string) (*ResourcePolicy, error)
	DeleteResourcePolicy(resourceArn string) error

	// Additional operations
	GetClusterCredentialsWithIAM(clusterID, dbName string) (*ClusterCredentials, error)
	FailoverPrimaryCompute(clusterID string) (*Cluster, error)
	DescribeTableRestoreStatus(clusterID string) ([]TableRestoreStatus, error)
	CreateTableRestoreStatus(
		clusterID, snapshotID, sourceDatabaseName, sourceTableName, targetDatabaseName, targetTableName string,
	) (*TableRestoreStatus, error)

	// HSM client certificate operations
	CreateHsmClientCertificate(id string, tags map[string]string) (*HsmClientCertificate, error)
	DeleteHsmClientCertificate(id string) error
	DescribeHsmClientCertificates(id string) ([]HsmClientCertificate, error)

	// HSM configuration operations
	CreateHsmConfiguration(
		id, description, hsmIPAddress, hsmPartitionName string, tags map[string]string,
	) (*HsmConfiguration, error)
	DeleteHsmConfiguration(id string) error
	DescribeHsmConfigurations(id string) ([]HsmConfiguration, error)

	// Scheduled action operations (regular Redshift)
	CreateScheduledAction(
		name, schedule, iamRole, description string,
		target *ScheduledActionTarget,
		enable *bool,
	) (*ScheduledAction, error)
	DeleteScheduledAction(name string) error
	DescribeScheduledActions(name string) ([]ScheduledAction, error)
	ModifyScheduledAction(
		name, schedule, iamRole, description string,
		target *ScheduledActionTarget,
		enable *bool,
	) (*ScheduledAction, error)

	// Custom domain association operations
	CreateCustomDomainAssociation(
		clusterID, customDomainName, customDomainCertificateArn string,
	) (*CustomDomainAssociation, error)
	DeleteCustomDomainAssociation(clusterID, customDomainName string) error
	DescribeCustomDomainAssociations(clusterID, customDomainName string) ([]CustomDomainAssociation, error)
	ModifyCustomDomainAssociation(
		clusterID, customDomainName, customDomainCertificateArn string,
	) (*CustomDomainAssociation, error)

	// Endpoint access operations
	CreateEndpointAccess(
		clusterID, endpointName, subnetGroupName, resourceOwner string, vpcSecurityGroupIDs []string,
	) (*EndpointAccess, error)
	DeleteEndpointAccess(endpointName string) (*EndpointAccess, error)
	DescribeEndpointAccess(clusterID, endpointName string) ([]EndpointAccess, error)
	ModifyEndpointAccess(endpointName string, vpcSecurityGroupIDs []string) (*EndpointAccess, error)

	// Integration operations
	CreateIntegration(
		integrationName, sourceArn, targetArn, kmsKeyID, description string, tags map[string]string,
	) (*Integration, error)
	DeleteIntegration(integrationArn string) (*Integration, error)
	DescribeIntegrations(integrationArn string) ([]Integration, error)
	ModifyIntegration(integrationArn, description, newIntegrationName string) (*Integration, error)

	// IDC application operations
	CreateIdcApplication(
		appName, idcInstanceArn, idcDisplayName, iamRoleArn, applicationType string,
	) (*IdcApplication, error)
	DeleteIdcApplication(appArn string) error
	DescribeIdcApplications(appArn string) ([]IdcApplication, error)
	ModifyIdcApplication(appArn, idcDisplayName, iamRoleArn string) (*IdcApplication, error)

	// Query Editor V2 (QEV2) IDC application operations. Qev2IdcApplication is
	// a distinct resource from IdcApplication above -- see the doc comment on
	// the Qev2IdcApplication type in models.go.
	CreateQev2IdcApplication(
		appName, idcInstanceArn, idcDisplayName string, tags map[string]string,
	) (*Qev2IdcApplication, error)
	DeleteQev2IdcApplication(appArn string) error
	DescribeQev2IdcApplications(appArn, marker string, maxRecords int) ([]Qev2IdcApplication, string, error)
	ModifyQev2IdcApplication(appArn, idcDisplayName string) (*Qev2IdcApplication, error)

	// Glue Data Catalog namespace registration operations. clusterIdentifier
	// is set for the ProvisionedIdentifier union variant; serverlessNamespace/
	// serverlessWorkgroup are both set for the ServerlessIdentifier variant --
	// see namespace_registration.go's doc comment.
	RegisterNamespace(
		consumerIdentifiers []string, clusterIdentifier, serverlessNamespace, serverlessWorkgroup string,
	) (*NamespaceRegistration, error)
	DeregisterNamespace(
		consumerIdentifiers []string, clusterIdentifier, serverlessNamespace, serverlessWorkgroup string,
	) (*NamespaceRegistration, error)

	// Lifecycle
	Reset()
	Region() string
	AccountID() string
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
	SetDNSRegistrar(dns DNSRegistrar)

	// StartReconciler starts the managed background reconciler that advances
	// cluster lifecycle state (creating→available, deleting→gone).
	StartReconciler(ctx context.Context)
	// StopReconciler stops the reconciler and waits for its goroutine to exit.
	StopReconciler()
}

// compile-time assertion that InMemoryBackend satisfies StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)
