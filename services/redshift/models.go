package redshift

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// ReservedNode represents an in-memory Redshift reserved node.
type ReservedNode struct {
	StartTime              time.Time `json:"startTime"`
	ReservedNodeID         string    `json:"reservedNodeId"`
	ReservedNodeOfferingID string    `json:"reservedNodeOfferingId"`
	NodeType               string    `json:"nodeType"`
	CurrencyCode           string    `json:"currencyCode"`
	State                  string    `json:"state"`
	OfferingType           string    `json:"offeringType"`
	Duration               int       `json:"duration"`
	FixedPrice             float64   `json:"fixedPrice"`
	UsagePrice             float64   `json:"usagePrice"`
	NodeCount              int       `json:"nodeCount"`
}

// Partner represents a partner integration for a Redshift cluster.
type Partner struct {
	AccountID         string `json:"accountId"`
	ClusterIdentifier string `json:"clusterIdentifier"`
	DatabaseName      string `json:"databaseName"`
	PartnerName       string `json:"partnerName"`
	Status            string `json:"status"`
	StatusMessage     string `json:"statusMessage"`
}

// DataShareAssociation represents an association between a data share and a consumer.
type DataShareAssociation struct {
	ConsumerIdentifier string    `json:"consumerIdentifier"`
	ConsumerRegion     string    `json:"consumerRegion"`
	CreatedDate        time.Time `json:"createdDate"`
	StatusChangeDate   time.Time `json:"statusChangeDate"`
	Status             string    `json:"status"`
	Type               string    `json:"type"`
}

// DataShare represents a Redshift data share.
type DataShare struct {
	DataShareArn string `json:"dataShareArn"`
	ProducerArn  string `json:"producerArn"`
	ManagedBy    string `json:"managedBy"`
	// DataShareType mirrors types.DataShareType from the real SDK. "INTERNAL" is
	// currently the only defined enum value (types.DataShareTypeInternal) --
	// namespace-to-namespace shares created via RegisterNamespace would be the
	// other case, which this backend does not yet model.
	DataShareType                    string                 `json:"dataShareType"`
	DataShareAssociations            []DataShareAssociation `json:"dataShareAssociations"`
	AllowPubliclyAccessibleConsumers bool                   `json:"allowPubliclyAccessibleConsumers"`
}

// IPRange represents an IP CIDR range within a cluster security group.
type IPRange struct {
	CIDRIP string `json:"cidrip"`
	Status string `json:"status"`
}

// EC2SecurityGroup represents an EC2 security group within a cluster security group.
type EC2SecurityGroup struct {
	EC2SecurityGroupName    string `json:"ec2SecurityGroupName"`
	EC2SecurityGroupOwnerID string `json:"ec2SecurityGroupOwnerId"`
	Status                  string `json:"status"`
}

// ClusterSecurityGroup represents a Redshift cluster security group.
type ClusterSecurityGroup struct {
	Tags                     map[string]string  `json:"tags,omitempty"`
	ClusterSecurityGroupName string             `json:"clusterSecurityGroupName"`
	Description              string             `json:"description"`
	IPRanges                 []IPRange          `json:"ipRanges"`
	EC2SecurityGroups        []EC2SecurityGroup `json:"ec2SecurityGroups"`
}

// AccountWithRestoreAccess represents an account permitted to restore from a snapshot.
type AccountWithRestoreAccess struct {
	AccountID    string `json:"accountId"`
	AccountAlias string `json:"accountAlias"`
}

// Snapshot represents a Redshift cluster snapshot.
type Snapshot struct {
	SnapshotCreateTime            time.Time                  `json:"snapshotCreateTime"`
	SnapshotIdentifier            string                     `json:"snapshotIdentifier"`
	ClusterIdentifier             string                     `json:"clusterIdentifier"`
	SnapshotType                  string                     `json:"snapshotType"`
	Status                        string                     `json:"status"`
	NodeType                      string                     `json:"nodeType,omitempty"`
	DBName                        string                     `json:"dbName,omitempty"`
	MasterUsername                string                     `json:"masterUsername,omitempty"`
	AccountsWithRestoreAccess     []AccountWithRestoreAccess `json:"accountsWithRestoreAccess"`
	ManualSnapshotRetentionPeriod int                        `json:"manualSnapshotRetentionPeriod"`
	NumberOfNodes                 int                        `json:"numberOfNodes,omitempty"`
}

// EndpointAuthorization represents authorization for a VPC endpoint to a cluster.
type EndpointAuthorization struct {
	AuthorizeTime     time.Time `json:"authorizeTime"`
	Grantor           string    `json:"grantor"`
	Grantee           string    `json:"grantee"`
	ClusterIdentifier string    `json:"clusterIdentifier"`
	ClusterStatus     string    `json:"clusterStatus"`
	Status            string    `json:"status"`
	AllowedVPCs       []string  `json:"allowedVPCs"`
	EndpointCount     int       `json:"endpointCount"`
	AllowedAllVPCs    bool      `json:"allowedAllVPCs"`
}

// ResizeProgress represents in-progress resize information for a cluster.
type ResizeProgress struct {
	TargetNodeType         string   `json:"targetNodeType"`
	TargetClusterType      string   `json:"targetClusterType"`
	Status                 string   `json:"status"`
	Message                string   `json:"message"`
	ResizeType             string   `json:"resizeType"`
	ImportTablesCompleted  []string `json:"importTablesCompleted"`
	ImportTablesInProgress []string `json:"importTablesInProgress"`
	ImportTablesNotStarted []string `json:"importTablesNotStarted"`
	TargetNumberOfNodes    int      `json:"targetNumberOfNodes"`
	AllowCancelResize      bool     `json:"allowCancelResize"`
}

// SnapshotBatchError represents an error when deleting a snapshot in a batch operation.
type SnapshotBatchError struct {
	SnapshotIdentifier        string `json:"snapshotIdentifier"`
	SnapshotClusterIdentifier string `json:"snapshotClusterIdentifier"`
	FailureCode               string `json:"failureCode"`
	FailureReason             string `json:"failureReason"`
}

// DNSRegistrar can register and deregister hostnames with an embedded DNS server.
type DNSRegistrar interface {
	Register(hostname string)
	Deregister(hostname string)
}

// SnapshotCopyGrant represents a KMS key grant for cross-region snapshot copy.
type SnapshotCopyGrant struct {
	Tags                  map[string]string `json:"tags"`
	SnapshotCopyGrantName string            `json:"snapshotCopyGrantName"`
	KMSKeyID              string            `json:"kmsKeyId"`
}

// SnapshotSchedule represents a snapshot schedule for automated snapshots.
type SnapshotSchedule struct {
	Tags                map[string]string `json:"tags"`
	ScheduleIdentifier  string            `json:"scheduleIdentifier"`
	Description         string            `json:"description"`
	ScheduleDefinitions []string          `json:"scheduleDefinitions"`
	// AssociatedClusters is derived at read time (see
	// InMemoryBackend.snapshotScheduleAssociatedClusters) from the clusters whose
	// SnapshotScheduleIdentifier matches this schedule; it is never persisted
	// directly on the schedule itself.
	AssociatedClusters []string `json:"-"`
}

// UsageLimit represents a usage limit for a Redshift feature.
type UsageLimit struct {
	Tags              map[string]string `json:"tags"`
	UsageLimitID      string            `json:"usageLimitId"`
	ClusterIdentifier string            `json:"clusterIdentifier"`
	FeatureType       string            `json:"featureType"`
	LimitType         string            `json:"limitType"`
	BreachAction      string            `json:"breachAction"`
	Amount            int64             `json:"amount"`
}

// AuthenticationProfile represents an authentication profile for Redshift.
type AuthenticationProfile struct {
	AuthenticationProfileName    string `json:"authenticationProfileName"`
	AuthenticationProfileContent string `json:"authenticationProfileContent"`
}

// ResourcePolicy represents a resource-based policy attached to a Redshift resource.
type ResourcePolicy struct {
	ResourceArn string `json:"resourceArn"`
	Policy      string `json:"policy"`
}

// TableRestoreStatus represents the status of a table-level restore operation.
type TableRestoreStatus struct {
	RequestTime           time.Time `json:"requestTime"`
	TableRestoreRequestID string    `json:"tableRestoreRequestId"`
	ClusterIdentifier     string    `json:"clusterIdentifier"`
	SnapshotIdentifier    string    `json:"snapshotIdentifier,omitempty"`
	Status                string    `json:"status"`
	Message               string    `json:"message"`
	SourceDatabaseName    string    `json:"sourceDatabaseName"`
	SourceTableName       string    `json:"sourceTableName"`
	TargetDatabaseName    string    `json:"targetDatabaseName"`
	TargetTableName       string    `json:"targetTableName"`
}

// HsmClientCertificate represents a Redshift HSM client certificate.
type HsmClientCertificate struct {
	Tags                           map[string]string `json:"tags"`
	HsmClientCertificateIdentifier string            `json:"hsmClientCertificateIdentifier"`
	HsmClientCertificatePublicKey  string            `json:"hsmClientCertificatePublicKey"`
}

// HsmConfiguration represents a Redshift HSM configuration.
type HsmConfiguration struct {
	Tags                       map[string]string `json:"tags"`
	HsmConfigurationIdentifier string            `json:"hsmConfigurationIdentifier"`
	Description                string            `json:"description"`
	HsmIPAddress               string            `json:"hsmIpAddress"`
	HsmPartitionName           string            `json:"hsmPartitionName"`
}

// PauseClusterAction mirrors types.PauseClusterMessage: the ResizeCluster/
// PauseCluster/ResumeCluster payload a ScheduledAction can target.
type PauseClusterAction struct {
	ClusterIdentifier string `json:"clusterIdentifier"`
}

// ResumeClusterAction mirrors types.ResumeClusterMessage.
type ResumeClusterAction struct {
	ClusterIdentifier string `json:"clusterIdentifier"`
}

// ResizeClusterAction mirrors types.ResizeClusterMessage.
type ResizeClusterAction struct {
	ClusterIdentifier            string `json:"clusterIdentifier"`
	ClusterType                  string `json:"clusterType,omitempty"`
	NodeType                     string `json:"nodeType,omitempty"`
	ReservedNodeID               string `json:"reservedNodeId,omitempty"`
	TargetReservedNodeOfferingID string `json:"targetReservedNodeOfferingId,omitempty"`
	NumberOfNodes                int    `json:"numberOfNodes,omitempty"`
	Classic                      bool   `json:"classic,omitempty"`
}

// ScheduledActionTarget mirrors types.ScheduledActionType: exactly one of these
// three should be set, matching which Redshift API operation the schedule invokes.
type ScheduledActionTarget struct {
	PauseCluster  *PauseClusterAction  `json:"pauseCluster,omitempty"`
	ResumeCluster *ResumeClusterAction `json:"resumeCluster,omitempty"`
	ResizeCluster *ResizeClusterAction `json:"resizeCluster,omitempty"`
}

// ScheduledAction represents a Redshift scheduled action.
type ScheduledAction struct {
	TargetAction               *ScheduledActionTarget `json:"targetAction,omitempty"`
	ScheduledActionName        string                 `json:"scheduledActionName"`
	Schedule                   string                 `json:"schedule"`
	IamRole                    string                 `json:"iamRole"`
	ScheduledActionDescription string                 `json:"scheduledActionDescription"`
	State                      string                 `json:"state"`
}

// CustomDomainAssociation represents a custom domain name associated with a Redshift cluster.
type CustomDomainAssociation struct {
	ClusterIdentifier          string `json:"clusterIdentifier"`
	CustomDomainName           string `json:"customDomainName"`
	CustomDomainCertificateArn string `json:"customDomainCertificateArn"`
	CustomDomainCertExpiryTime string `json:"customDomainCertExpiryTime"`
}

// EndpointAccess represents a Redshift managed VPC endpoint.
type EndpointAccess struct {
	ClusterIdentifier   string   `json:"clusterIdentifier"`
	EndpointName        string   `json:"endpointName"`
	EndpointStatus      string   `json:"endpointStatus"`
	EndpointCreateTime  string   `json:"endpointCreateTime"`
	VpcID               string   `json:"vpcId"`
	SubnetGroupName     string   `json:"subnetGroupName,omitempty"`
	ResourceOwner       string   `json:"resourceOwner,omitempty"`
	VpcSecurityGroupIDs []string `json:"vpcSecurityGroupIds,omitempty"`
	Port                int      `json:"port"`
}

// Integration represents a zero-ETL integration from Redshift.
type Integration struct {
	CreateTime       time.Time         `json:"createTime"`
	Tags             map[string]string `json:"tags"`
	IntegrationArn   string            `json:"integrationArn"`
	IntegrationName  string            `json:"integrationName"`
	SourceArn        string            `json:"sourceArn"`
	TargetArn        string            `json:"targetArn"`
	Status           string            `json:"status"`
	Description      string            `json:"description"`
	AdditionalEncKey string            `json:"additionalEncryptionContext,omitempty"`
	KmsKeyID         string            `json:"kmsKeyId,omitempty"`
}

// IdcApplication represents a Redshift IDC application.
type IdcApplication struct {
	IdcApplicationArn  string `json:"redshiftIdcApplicationArn"`
	IdcApplicationName string `json:"redshiftIdcApplicationName"`
	IdcInstanceArn     string `json:"idcInstanceArn"`
	IdcDisplayName     string `json:"idcDisplayName"`
	IamRoleArn         string `json:"iamRoleArn"`
	// ApplicationType mirrors types.ApplicationType ("None" or "Lakehouse"); it is
	// set only on create -- real ModifyRedshiftIdcApplicationInput has no field
	// for it (confirmed against aws-sdk-go-v2/service/redshift@v1.65.4/serializers.go
	// awsAwsquery_serializeOpDocumentModifyRedshiftIdcApplicationInput).
	ApplicationType string `json:"applicationType,omitempty"`
}

// Qev2IdcApplication represents an Amazon Redshift Query Editor (QEV2) IAM
// Identity Center application. This is a DISTINCT resource from
// IdcApplication (RedshiftIdcApplication) above, not a sub-resource of it:
// RedshiftIdcApplication backs cluster-level federated authentication into
// IAM Identity Center and carries an IamRoleArn used to invoke the IDC
// Identity Center API, while Qev2IdcApplication is the separate IdC-managed
// application that powers the standalone Query Editor V2 web console and has
// no IamRoleArn or cluster reference at all (confirmed field-by-field against
// aws-sdk-go-v2/service/redshift@v1.65.0/types.Qev2IdcApplication, which
// declares no IamRoleArn field, and against CreateQev2IdcApplicationInput /
// ModifyQev2IdcApplicationInput, neither of which accepts one). The two
// families share only the IdC-instance/display-name/onboard-status shape and
// are otherwise independently keyed and stored.
type Qev2IdcApplication struct {
	Tags                     map[string]string `json:"tags,omitempty"`
	Qev2IdcApplicationArn    string            `json:"qev2IdcApplicationArn"`
	Qev2IdcApplicationName   string            `json:"qev2IdcApplicationName"`
	IdcInstanceArn           string            `json:"idcInstanceArn"`
	IdcDisplayName           string            `json:"idcDisplayName"`
	IdcManagedApplicationArn string            `json:"idcManagedApplicationArn"`
	IdcOnboardStatus         string            `json:"idcOnboardStatus"`
}

// SnapshotCopyConfig holds the cross-region snapshot copy configuration for a cluster.
type SnapshotCopyConfig struct {
	DestinationRegion     string `json:"destinationRegion"`
	SnapshotCopyGrantName string `json:"snapshotCopyGrantName"`
	RetentionPeriod       int    `json:"retentionPeriod"`
}

// ClusterPendingModifiedValues holds changes queued for the next maintenance
// window. Fields mirror the real types.PendingModifiedValues (redshift@v1.65.4
// types/types.go:1491) subset this backend models.
type ClusterPendingModifiedValues struct {
	NodeType           string `json:"nodeType,omitempty"`
	ClusterVersion     string `json:"clusterVersion,omitempty"`
	NumberOfNodes      int    `json:"numberOfNodes,omitempty"`
	Encrypted          bool   `json:"encrypted,omitempty"`
	PubliclyAccessible bool   `json:"publiclyAccessible,omitempty"`
}

// Cluster represents a Redshift cluster.
type Cluster struct {
	Tags                        *tags.Tags                    `json:"tags,omitempty"`
	PendingModifiedValues       *ClusterPendingModifiedValues `json:"pendingModifiedValues,omitempty"`
	SnapshotScheduleState       string                        `json:"snapshotScheduleState,omitempty"`
	ClusterIdentifier           string                        `json:"clusterIdentifier"`
	ClusterType                 string                        `json:"clusterType"`
	Endpoint                    string                        `json:"endpoint"`
	Status                      string                        `json:"status"`
	DBName                      string                        `json:"dbName"`
	PreferredMaintenanceWindow  string                        `json:"preferredMaintenanceWindow,omitempty"`
	VpcID                       string                        `json:"vpcId,omitempty"`
	MasterUsername              string                        `json:"masterUsername"`
	NodeType                    string                        `json:"nodeType"`
	SnapshotScheduleIdentifier  string                        `json:"snapshotScheduleIdentifier,omitempty"`
	KmsKeyID                    string                        `json:"kmsKeyId,omitempty"`
	ClusterVersion              string                        `json:"clusterVersion,omitempty"`
	LakehouseRegistrationStatus string                        `json:"lakehouseRegistrationStatus,omitempty"`
	ClusterParameterGroupName   string                        `json:"clusterParameterGroupName,omitempty"`
	CatalogArn                  string                        `json:"catalogArn,omitempty"`
	ClusterSecurityGroups       []string                      `json:"clusterSecurityGroups,omitempty"`
	VpcSecurityGroupIDs         []string                      `json:"vpcSecurityGroupIds,omitempty"`
	IamRoles                    []string                      `json:"iamRoles,omitempty"`
	Port                        int                           `json:"port"`
	NumberOfNodes               int                           `json:"numberOfNodes"`
	Encrypted                   bool                          `json:"encrypted"`
	EnhancedVpcRouting          bool                          `json:"enhancedVpcRouting"`
	PubliclyAccessible          bool                          `json:"publiclyAccessible,omitempty"`
}

// ClusterCredentials holds temporary cluster credentials.
type ClusterCredentials struct {
	Expiration time.Time
	DBUser     string
	DBPassword string
}
