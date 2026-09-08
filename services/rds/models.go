package rds

import (
	"regexp"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// dbInstanceIDRegex matches valid RDS DBInstanceIdentifier values.
// Must start with a letter, contain only alphanumeric and hyphens, 1–63 chars.
var dbInstanceIDRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,62}$`)

const (
	defaultPort             = 5432
	mysqlPort               = 3306
	defaultInstanceClass    = "db.t3.micro"
	defaultAllocatedStorage = 20

	instanceStatusCreating  = "creating"
	instanceStatusModifying = "modifying"
	instanceStatusDeleting  = "deleting"
	instanceStatusAvailable = "available"
	instanceStatusStopped   = "stopped"
	instanceStatusRebooting = "rebooting"

	clusterStatusFailingOver = "failing-over"

	subscriptionStatusActive           = "active"
	backtrackStatusApplying            = "applying"
	blueGreenDeploymentStatusAvailable = "available"
	ipRangeStatusAuthorized            = "authorized"
	instanceTransitionDelay            = 250 * time.Millisecond
	reconcilerDivisor                  = 5
	maxEvents                          = 512
	percentProgressComplete            = 100

	// seededLogFileCount is the number of synthetic DB log files generated per instance.
	seededLogFileCount = 3
	// seededLogBasePID is the base process ID used in synthetic DB log lines.
	seededLogBasePID = 1000

	engineMySQL            = "mysql"
	engineMariaDB          = "mariadb"
	enginePostgres         = "postgres"
	engineAuroraMySQL      = "aurora-mysql"
	engineAuroraPostgresql = "aurora-postgresql"

	currencyUSD       = "USD"
	reservedValidFrom = "2021-05-25T00:00:00Z"
	// defaultCACertificateID is the CA certificate identifier RDS reports as the
	// account default until ModifyCertificates overrides it.
	defaultCACertificateID   = "rds-ca-rsa2048-g1"
	reservedAllUpfront       = "All Upfront"
	clusterEndpointReadWrite = "READ_WRITE"
	opDescribeGlobalClusters = "DescribeGlobalClusters"
)

// VpcSecurityGroupMembership represents a VPC security group association.
type VpcSecurityGroupMembership struct {
	VpcSecurityGroupID string `json:"vpcSecurityGroupId"`
	Status             string `json:"status"`
}

// DBSecurityGroupMembership represents a classic (EC2-Classic) DB security
// group association on a DB instance.
type DBSecurityGroupMembership struct {
	DBSecurityGroupName string `json:"dbSecurityGroupName"`
	Status              string `json:"status"`
}

// DBClusterMember represents a member instance in a DB cluster.
type DBClusterMember struct {
	DBInstanceIdentifier        string `json:"dbInstanceIdentifier"`
	DBClusterParameterGroupName string `json:"dbClusterParameterGroupName"`
	PromotionTier               int    `json:"promotionTier"`
	IsClusterWriter             bool   `json:"isClusterWriter"`
}

// DBClusterRole associates an IAM role with a DB cluster for a feature slot
// (rds@v1.124.1 types.go:1511, AssociatedRoles on DBCluster). Unlike the
// instance-side FeatureName (required, fixed in gopherstack-i101),
// FeatureName is optional here (api_op_AddRoleToDBCluster.go:39-43), so an
// empty FeatureName means the client omitted it, not that AWS reported one
// as empty -- see db_clusters.go's upsertClusterRole/removeClusterRole for how the
// omitted case is handled. That handling is a documented placeholder, not
// verified against real AWS; see gopherstack-1jkv and PARITY.md.
type DBClusterRole struct {
	RoleArn     string `json:"roleArn"`
	FeatureName string `json:"featureName"`
	Status      string `json:"status"`
}

// GlobalClusterMember represents a member cluster in a global cluster.
type GlobalClusterMember struct {
	DBClusterArn          string `json:"dbClusterArn"`
	GlobalWriteForwarding bool   `json:"globalWriteForwarding"`
	IsWriter              bool   `json:"isWriter"`
}

// CustomDBEngineVersion represents a custom engine version for RDS.
type CustomDBEngineVersion struct {
	Engine             string `json:"engine"`
	EngineVersion      string `json:"engineVersion"`
	DBEngineVersionArn string `json:"dbEngineVersionArn,omitempty"`
	Status             string `json:"status"`
	Description        string `json:"description"`
}

// DBInstance represents an RDS database instance.
type DBInstance struct {
	InstanceCreateTime                time.Time                    `json:"instanceCreateTime"`
	DBInstanceArn                     string                       `json:"dbInstanceArn,omitempty"`
	EnhancedMonitoringResourceArn     string                       `json:"enhancedMonitoringResourceArn,omitempty"`
	PreferredBackupWindow             string                       `json:"preferredBackupWindow,omitempty"`
	KmsKeyID                          string                       `json:"kmsKeyID,omitempty"`
	DBClusterIdentifier               string                       `json:"dbClusterIdentifier,omitempty"`
	Engine                            string                       `json:"engine"`
	EngineVersion                     string                       `json:"engineVersion"`
	DBInstanceStatus                  string                       `json:"dbInstanceStatus"`
	MasterUsername                    string                       `json:"masterUsername"`
	DBName                            string                       `json:"dbName"`
	Endpoint                          string                       `json:"endpoint"`
	VpcID                             string                       `json:"vpcID"`
	DBSubnetGroupName                 string                       `json:"dbSubnetGroupName"`
	DBParameterGroupName              string                       `json:"dbParameterGroupName"`
	OptionGroupName                   string                       `json:"optionGroupName,omitempty"`
	ReplicaSourceDBInstanceIdentifier string                       `json:"replicaSourceDBInstanceIdentifier"`
	AvailabilityZone                  string                       `json:"availabilityZone"`
	StorageType                       string                       `json:"storageType"`
	LicenseModel                      string                       `json:"licenseModel,omitempty"`
	MonitoringRoleArn                 string                       `json:"monitoringRoleArn,omitempty"`
	DBInstanceIdentifier              string                       `json:"dbInstanceIdentifier"`
	DbiResourceID                     string                       `json:"dbiResourceID"`
	PreferredMaintenanceWindow        string                       `json:"preferredMaintenanceWindow,omitempty"`
	DBInstanceClass                   string                       `json:"dbInstanceClass"`
	EngineLifecycleSupport            string                       `json:"engineLifecycleSupport,omitempty"`
	EnabledCloudwatchLogsExports      []string                     `json:"enabledCloudwatchLogsExports,omitempty"`
	VpcSecurityGroups                 []VpcSecurityGroupMembership `json:"vpcSecurityGroups,omitempty"`
	DBSecurityGroups                  []DBSecurityGroupMembership  `json:"dbSecurityGroups,omitempty"`
	PendingModifiedValues             *PendingModifiedValues       `json:"pendingModifiedValues,omitempty"`
	ReadReplicaIdentifiers            []string                     `json:"readReplicaIdentifiers,omitempty"`
	Port                              int                          `json:"port"`
	AllocatedStorage                  int                          `json:"allocatedStorage"`
	Iops                              int                          `json:"iops,omitempty"`
	StorageThroughput                 int                          `json:"storageThroughput,omitempty"`
	BackupRetentionPeriod             int                          `json:"backupRetentionPeriod"`
	MonitoringInterval                int                          `json:"monitoringInterval,omitempty"`
	MultiAZ                           bool                         `json:"multiAZ"`
	StorageEncrypted                  bool                         `json:"storageEncrypted"`
	IAMDatabaseAuthenticationEnabled  bool                         `json:"iamDatabaseAuthenticationEnabled"`
	DeletionProtection                bool                         `json:"deletionProtection"`
	CopyTagsToSnapshot                bool                         `json:"copyTagsToSnapshot,omitempty"`
	PubliclyAccessible                bool                         `json:"publiclyAccessible,omitempty"`
	PerformanceInsightsEnabled        bool                         `json:"performanceInsightsEnabled,omitempty"`
	StorageOptimized                  bool                         `json:"storageOptimized,omitempty"`
	OptimizedWrites                   bool                         `json:"optimizedWrites,omitempty"`
}

// PendingModifiedValues holds deferred instance changes (ApplyImmediately=false).
// A non-nil pointer means at least one field is pending. Nil means nothing pending.
type PendingModifiedValues struct {
	MultiAZChange    *bool  `json:"multiAZChange,omitempty"`
	DBInstanceClass  string `json:"dbInstanceClass,omitempty"`
	EngineVersion    string `json:"engineVersion,omitempty"`
	StorageType      string `json:"storageType,omitempty"`
	AllocatedStorage int    `json:"allocatedStorage,omitempty"`
	Iops             int    `json:"iops,omitempty"`
}

// DBSnapshot represents an RDS database snapshot.
type DBSnapshot struct {
	SnapshotCreateTime   time.Time `json:"snapshotCreateTime"`
	DBSnapshotIdentifier string    `json:"dbSnapshotIdentifier"`
	DBSnapshotArn        string    `json:"dbSnapshotArn,omitempty"`
	DBInstanceIdentifier string    `json:"dbInstanceIdentifier"`
	DbiResourceID        string    `json:"dbiResourceId,omitempty"`
	Engine               string    `json:"engine"`
	EngineVersion        string    `json:"engineVersion"`
	Status               string    `json:"status"`
	StorageType          string    `json:"storageType"`
	OptionGroupName      string    `json:"optionGroupName"`
	KmsKeyID             string    `json:"kmsKeyID,omitempty"`
	SourceRegion         string    `json:"sourceRegion,omitempty"`
	SnapshotType         string    `json:"snapshotType,omitempty"`
	AllocatedStorage     int       `json:"allocatedStorage"`
	Port                 int       `json:"port"`
	PercentProgress      int       `json:"percentProgress"`
	StorageEncrypted     bool      `json:"storageEncrypted"`
	CopyTagsToSnapshot   bool      `json:"copyTagsToSnapshot,omitempty"`
}

// DBSubnetGroup represents an RDS DB subnet group.
type DBSubnetGroup struct {
	DBSubnetGroupName        string   `json:"dbSubnetGroupName"`
	DBSubnetGroupDescription string   `json:"dbSubnetGroupDescription"`
	DBSubnetGroupArn         string   `json:"dbSubnetGroupArn,omitempty"`
	VpcID                    string   `json:"vpcID"`
	Status                   string   `json:"status"`
	SubnetIDs                []string `json:"subnetIDs"`
}

// Tag is a key/value tag attached to an RDS resource.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DBParameter represents a single RDS parameter.
type DBParameter struct {
	ParameterName  string `json:"parameterName"`
	ParameterValue string `json:"parameterValue"`
	Description    string `json:"description"`
	ApplyType      string `json:"applyType"`
	DataType       string `json:"dataType"`
	Source         string `json:"source"`
	ApplyMethod    string `json:"applyMethod"`
	IsModifiable   bool   `json:"isModifiable"`
}

// DBParameterGroup represents an RDS DB parameter group.
type DBParameterGroup struct {
	Parameters             map[string]DBParameter `json:"parameters"`
	DBParameterGroupName   string                 `json:"dbParameterGroupName"`
	DBParameterGroupArn    string                 `json:"dbParameterGroupArn,omitempty"`
	DBParameterGroupFamily string                 `json:"dbParameterGroupFamily"`
	Description            string                 `json:"description"`
}

// OptionGroupOption represents an option within an option group.
type OptionGroupOption struct {
	OptionName    string `json:"optionName"`
	OptionVersion string `json:"optionVersion"`
}

// OptionGroup represents an RDS option group.
type OptionGroup struct {
	OptionGroupName        string              `json:"optionGroupName"`
	OptionGroupDescription string              `json:"optionGroupDescription"`
	OptionGroupArn         string              `json:"optionGroupArn,omitempty"`
	EngineName             string              `json:"engineName"`
	MajorEngineVersion     string              `json:"majorEngineVersion"`
	Options                []OptionGroupOption `json:"options"`
}

// ServerlessV2ScalingConfiguration holds Aurora Serverless v2 capacity settings.
type ServerlessV2ScalingConfiguration struct {
	MinCapacity float64 `json:"minCapacity"`
	MaxCapacity float64 `json:"maxCapacity"`
}

// DBCluster represents an Aurora-style RDS cluster.
type DBCluster struct {
	ClusterCreateTime               time.Time                         `json:"clusterCreateTime"`
	ServerlessV2ScalingConfig       *ServerlessV2ScalingConfiguration `json:"serverlessV2ScalingConfiguration,omitempty"`
	MonitoringRoleArn               string                            `json:"monitoringRoleArn,omitempty"`
	StorageType                     string                            `json:"storageType,omitempty"`
	Status                          string                            `json:"status"`
	MasterUsername                  string                            `json:"masterUsername"`
	DatabaseName                    string                            `json:"databaseName"`
	DBClusterParameterGroupName     string                            `json:"dbClusterParameterGroupName"`
	DBClusterArn                    string                            `json:"dbClusterArn,omitempty"`
	DBClusterResourceID             string                            `json:"dbClusterResourceId,omitempty"`
	Engine                          string                            `json:"engine"`
	EngineVersion                   string                            `json:"engineVersion,omitempty"`
	ActivityStreamAuditPolicy       string                            `json:"activityStreamAuditPolicy"`
	DBClusterIdentifier             string                            `json:"dbClusterIdentifier"`
	ActivityStreamKinesisStreamName string                            `json:"activityStreamKinesisStreamName"`
	ActivityStreamKMSKeyID          string                            `json:"activityStreamKmsKeyId"`
	ActivityStreamMode              string                            `json:"activityStreamMode"`
	PreferredBackupWindow           string                            `json:"preferredBackupWindow,omitempty"`
	PreferredMaintenanceWindow      string                            `json:"preferredMaintenanceWindow,omitempty"`
	KmsKeyID                        string                            `json:"kmsKeyID,omitempty"`
	ActivityStreamStatus            string                            `json:"activityStreamStatus"`
	EngineLifecycleSupport          string                            `json:"engineLifecycleSupport,omitempty"`
	NetworkType                     string                            `json:"networkType,omitempty"`
	ReaderEndpoint                  string                            `json:"readerEndpoint,omitempty"`
	Endpoint                        string                            `json:"endpoint"`
	ReplicationSourceIdentifier     string                            `json:"replicationSourceIdentifier,omitempty"`
	EnabledCloudwatchLogsExports    []string                          `json:"enabledCloudwatchLogsExports,omitempty"`
	ReaderAvailabilityZones         []string                          `json:"readerAvailabilityZones,omitempty"`
	AvailabilityZones               []string                          `json:"availabilityZones,omitempty"`
	DBClusterMembers                []DBClusterMember                 `json:"dbClusterMembers,omitempty"`
	ReadReplicaIdentifiers          []string                          `json:"readReplicaIdentifiers,omitempty"`
	BacktrackWindow                 int64                             `json:"backtrackWindow,omitempty"`
	Port                            int                               `json:"port"`
	BackupRetentionPeriod           int                               `json:"backupRetentionPeriod"`
	MonitoringInterval              int                               `json:"monitoringInterval,omitempty"`
	ServerlessCapacity              int                               `json:"serverlessCapacity"`
	MultiAZ                         bool                              `json:"multiAZ,omitempty"`
	HTTPEndpointEnabled             bool                              `json:"httpEndpointEnabled"`
	StorageEncrypted                bool                              `json:"storageEncrypted,omitempty"`
	CopyTagsToSnapshot              bool                              `json:"copyTagsToSnapshot,omitempty"`
	DeletionProtection              bool                              `json:"deletionProtection,omitempty"`
	OptimizedWrites                 bool                              `json:"optimizedWrites,omitempty"`
}

// DBClusterSnapshot represents an RDS cluster snapshot.
type DBClusterSnapshot struct {
	SnapshotCreateTime          time.Time `json:"snapshotCreateTime"`
	DBClusterSnapshotIdentifier string    `json:"dbClusterSnapshotIdentifier"`
	DBClusterSnapshotArn        string    `json:"dbClusterSnapshotArn,omitempty"`
	DBClusterIdentifier         string    `json:"dbClusterIdentifier"`
	DBClusterResourceID         string    `json:"dbClusterResourceId,omitempty"`
	Engine                      string    `json:"engine"`
	EngineVersion               string    `json:"engineVersion,omitempty"`
	Status                      string    `json:"status"`
	SnapshotType                string    `json:"snapshotType,omitempty"`
	PercentProgress             int       `json:"percentProgress"`
	StorageEncrypted            bool      `json:"storageEncrypted,omitempty"`
}

// DBClusterEndpoint represents a custom endpoint for an RDS cluster.
type DBClusterEndpoint struct {
	DBClusterEndpointIdentifier string `json:"dbClusterEndpointIdentifier"`
	DBClusterIdentifier         string `json:"dbClusterIdentifier"`
	DBClusterEndpointArn        string `json:"dbClusterEndpointArn,omitempty"`
	EndpointType                string `json:"endpointType"`
	Status                      string `json:"status"`
	Endpoint                    string `json:"endpoint"`
}

// ExportTask represents an RDS export task.
type ExportTask struct {
	ExportTaskIdentifier string `json:"exportTaskIdentifier"`
	SourceArn            string `json:"sourceArn"`
	Status               string `json:"status"`
	S3Bucket             string `json:"s3Bucket"`
	IamRoleArn           string `json:"iamRoleArn"`
	KmsKeyID             string `json:"kmsKeyId"`
}

// GlobalCluster represents an RDS global cluster.
type GlobalCluster struct {
	GlobalClusterIdentifier string                `json:"globalClusterIdentifier"`
	GlobalClusterArn        string                `json:"globalClusterArn,omitempty"`
	Engine                  string                `json:"engine"`
	EngineVersion           string                `json:"engineVersion"`
	Status                  string                `json:"status"`
	PrimaryRegion           string                `json:"primaryRegion,omitempty"`
	GlobalClusterMembers    []GlobalClusterMember `json:"globalClusterMembers,omitempty"`
	ClusterARNs             []string              `json:"clusterARNs"`
	StorageEncrypted        bool                  `json:"storageEncrypted"`
	DeletionProtection      bool                  `json:"deletionProtection"`
}

// DBEngineVersion represents an available RDS engine version.
type DBEngineVersion struct {
	Engine              string `json:"engine"`
	EngineVersion       string `json:"engineVersion"`
	DBEngineDescription string `json:"dbEngineDescription"`
}

// DBSnapshotAttribute represents an attribute of a DB snapshot.
type DBSnapshotAttribute struct {
	AttributeName   string   `json:"attributeName"`
	AttributeValues []string `json:"attributeValues"`
}

// DBSnapshotAttributesResult holds attributes for a DB snapshot.
type DBSnapshotAttributesResult struct {
	DBSnapshotIdentifier string                `json:"dbSnapshotIdentifier"`
	DBSnapshotAttributes []DBSnapshotAttribute `json:"dbSnapshotAttributes"`
}

// DBClusterSnapshotAttributesResult holds attributes for a cluster snapshot.
type DBClusterSnapshotAttributesResult struct {
	DBClusterSnapshotIdentifier string                `json:"dbClusterSnapshotIdentifier"`
	DBClusterSnapshotAttributes []DBSnapshotAttribute `json:"dbClusterSnapshotAttributes"`
}

// Certificate represents an RDS CA certificate.
type Certificate struct {
	CertificateIdentifier string `json:"certificateIdentifier"`
	CertificateType       string `json:"certificateType"`
	ValidTill             string `json:"validTill"`
	ValidFrom             string `json:"validFrom"`
	Thumbprint            string `json:"thumbprint"`
	CustomerOverride      bool   `json:"customerOverride"`
}

// AccountAttribute represents an RDS account quota attribute.
type AccountAttribute struct {
	AttributeName string `json:"attributeName"`
	Used          int    `json:"used"`
	Max           int    `json:"max"`
}

// PendingMaintenanceAction represents a pending maintenance action for a resource.
type PendingMaintenanceAction struct {
	ResourceIdentifier   string `json:"resourceIdentifier"`
	Action               string `json:"action"`
	AutoAppliedAfterDate string `json:"autoAppliedAfterDate"`
	CurrentApplyDate     string `json:"currentApplyDate"`
	Description          string `json:"description"`
}

// SourceRegion represents an available source region for cross-region operations.
type SourceRegion struct {
	RegionName string `json:"regionName"`
	Endpoint   string `json:"endpoint"`
	Status     string `json:"status"`
}

// DBMajorEngineVersion represents a major engine version.
type DBMajorEngineVersion struct {
	Engine             string `json:"engine"`
	MajorEngineVersion string `json:"majorEngineVersion"`
	Status             string `json:"status"`
}

// ServerlessV2FeaturesSupport describes Aurora Serverless v2 capacity limits that
// differ between engine/platform versions (aws-sdk-go-v2/service/rds/types's
// ServerlessV2FeaturesSupport: MinCapacity/MaxCapacity are both *float64 on the wire).
type ServerlessV2FeaturesSupport struct {
	MinCapacity float64 `json:"minCapacity"`
	MaxCapacity float64 `json:"maxCapacity"`
}

// ServerlessV2PlatformVersionInfo represents a single Aurora Serverless v2 platform
// version, as returned by DescribeServerlessV2PlatformVersions (aws-sdk-go-v2/service/
// rds/types's ServerlessV2PlatformVersionInfo). See DescribeServerlessV2PlatformVersions
// in reference_data.go for why this backend's catalog is always empty.
type ServerlessV2PlatformVersionInfo struct {
	ServerlessV2FeaturesSupport            *ServerlessV2FeaturesSupport `json:"serverlessV2FeaturesSupport,omitempty"`
	Engine                                 string                       `json:"engine"`
	ServerlessV2PlatformVersion            string                       `json:"serverlessV2PlatformVersion"`
	ServerlessV2PlatformVersionDescription string                       `json:"serverlessV2PlatformVersionDescription"`
	Status                                 string                       `json:"status"`
	IsDefault                              bool                         `json:"isDefault"`
}

// ReservedDBInstance represents a purchased reserved DB instance.
type ReservedDBInstance struct {
	ReservedDBInstanceID          string  `json:"reservedDBInstanceId"`
	ReservedDBInstancesOfferingID string  `json:"reservedDBInstancesOfferingId"`
	DBInstanceClass               string  `json:"dbInstanceClass"`
	StartTime                     string  `json:"startTime"`
	ProductDescription            string  `json:"productDescription"`
	OfferingType                  string  `json:"offeringType"`
	State                         string  `json:"state"`
	CurrencyCode                  string  `json:"currencyCode"`
	FixedPrice                    float64 `json:"fixedPrice"`
	UsagePrice                    float64 `json:"usagePrice"`
	Duration                      int     `json:"duration"`
	DBInstanceCount               int     `json:"dbInstanceCount"`
	MultiAZ                       bool    `json:"multiAZ"`
}

// ReservedDBInstancesOffering represents an available reserved DB instance offering.
type ReservedDBInstancesOffering struct {
	ReservedDBInstancesOfferingID string  `json:"reservedDBInstancesOfferingId"`
	DBInstanceClass               string  `json:"dbInstanceClass"`
	ProductDescription            string  `json:"productDescription"`
	OfferingType                  string  `json:"offeringType"`
	CurrencyCode                  string  `json:"currencyCode"`
	FixedPrice                    float64 `json:"fixedPrice"`
	UsagePrice                    float64 `json:"usagePrice"`
	Duration                      int     `json:"duration"`
	MultiAZ                       bool    `json:"multiAZ"`
}

// DBRecommendation represents an RDS performance recommendation.
type DBRecommendation struct {
	RecommendationID string `json:"recommendationId"`
	TypeID           string `json:"typeId"`
	Severity         string `json:"severity"`
	Status           string `json:"status"`
	Description      string `json:"description"`
	Reason           string `json:"reason"`
	ResourceARN      string `json:"resourceArn"`
	UpdatedTime      string `json:"updatedTime"`
	CreatedTime      string `json:"createdTime"`
}

// OrderableDBInstanceOption represents an orderable DB instance option.
type OrderableDBInstanceOption struct {
	Engine          string `json:"engine"`
	EngineVersion   string `json:"engineVersion"`
	DBInstanceClass string `json:"dbInstanceClass"`
	MultiAZCapable  bool   `json:"multiAZCapable"`
}

// DBLogFile represents a log file for a DB instance.
type DBLogFile struct {
	LogFileName string `json:"logFileName"`
	// LastWritten is the epoch-millisecond timestamp at which the log file was
	// last written, matching the RDS DescribeDBLogFilesDetails.LastWritten field.
	LastWritten int64 `json:"lastWritten"`
	Size        int64 `json:"size"`
}

// EventSubscription represents an RDS event notification subscription.
type EventSubscription struct {
	SubscriptionName     string   `json:"subscriptionName"`
	SnsTopicArn          string   `json:"snsTopicArn"`
	EventSubscriptionArn string   `json:"eventSubscriptionArn,omitempty"`
	Status               string   `json:"status"`
	SourceType           string   `json:"sourceType"`
	SourceIDs            []string `json:"sourceIds"`
	EventCategories      []string `json:"eventCategories,omitempty"`
	Enabled              bool     `json:"enabled"`
}

// Event represents a published RDS lifecycle event.
type Event struct {
	CreatedAt        time.Time `json:"createdAt"`
	Message          string    `json:"message"`
	SourceIdentifier string    `json:"sourceIdentifier"`
	SourceType       string    `json:"sourceType"`
	SourceArn        string    `json:"sourceArn,omitempty"`
}

// IPRange represents a CIDR IP range authorized for a DB security group.
type IPRange struct {
	CIDRIP string `json:"cidrip"`
	Status string `json:"status"`
}

// DBSecurityGroup represents an RDS DB security group.
type DBSecurityGroup struct {
	DBSecurityGroupName        string    `json:"dbSecurityGroupName"`
	DBSecurityGroupDescription string    `json:"dbSecurityGroupDescription"`
	DBSecurityGroupArn         string    `json:"dbSecurityGroupArn,omitempty"`
	IPRanges                   []IPRange `json:"ipRanges"`
}

// BlueGreenDeployment represents an RDS Blue/Green Deployment.
type BlueGreenDeployment struct {
	BlueGreenDeploymentIdentifier string `json:"blueGreenDeploymentIdentifier"`
	BlueGreenDeploymentName       string `json:"blueGreenDeploymentName"`
	Source                        string `json:"source"`
	Target                        string `json:"target,omitempty"`
	Status                        string `json:"status"`
}

// DBClusterBacktrack represents backtrack information for an Aurora cluster.
type DBClusterBacktrack struct {
	DBClusterIdentifier string `json:"dbClusterIdentifier"`
	BacktrackIdentifier string `json:"backtrackIdentifier"`
	BacktrackTo         string `json:"backtrackTo"`
	Status              string `json:"status"`
}

// DNSRegistrar can register and deregister hostnames with an embedded DNS server.
type DNSRegistrar interface {
	Register(hostname string)
	Deregister(hostname string)
}

// DBShardGroup represents an Aurora Limitless DB shard group.
type DBShardGroup struct {
	DBShardGroupIdentifier string  `json:"dbShardGroupIdentifier"`
	DBClusterIdentifier    string  `json:"dbClusterIdentifier"`
	DBShardGroupArn        string  `json:"dbShardGroupArn,omitempty"`
	DBShardGroupResourceID string  `json:"dbShardGroupResourceId,omitempty"`
	Status                 string  `json:"status"`
	Endpoint               string  `json:"endpoint,omitempty"`
	MaxACU                 float64 `json:"maxACU,omitempty"`
	MinACU                 float64 `json:"minACU,omitempty"`
	ComputeRedundancy      int     `json:"computeRedundancy,omitempty"`
	PubliclyAccessible     bool    `json:"publiclyAccessible,omitempty"`
}

// IntegrationError describes a single error associated with a zero-ETL
// integration, matching aws-sdk-go-v2's types.IntegrationError.
type IntegrationError struct {
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// Integration represents an RDS zero-ETL integration to Amazon Redshift.
type Integration struct {
	CreatedAt              time.Time          `json:"createdAt"`
	IntegrationArn         string             `json:"integrationArn"`
	IntegrationName        string             `json:"integrationName"`
	SourceArn              string             `json:"sourceArn"`
	TargetArn              string             `json:"targetArn"`
	KmsKeyID               string             `json:"kmsKeyId,omitempty"`
	DataFilter             string             `json:"dataFilter,omitempty"`
	IntegrationDescription string             `json:"integrationDescription,omitempty"`
	Status                 string             `json:"status"`
	Errors                 []IntegrationError `json:"errors,omitempty"`
}

// TenantDatabase represents a tenant database within a multi-tenant RDS instance.
type TenantDatabase struct {
	CreatedAt            time.Time `json:"createdAt"`
	DBInstanceIdentifier string    `json:"dbInstanceIdentifier"`
	TenantDBName         string    `json:"tenantDBName"`
	MasterUsername       string    `json:"masterUsername"`
	TenantDatabaseARN    string    `json:"tenantDatabaseArn"`
	DbiResourceID        string    `json:"dbiResourceId"`
	Status               string    `json:"status"`
}

// DBClusterAutomatedBackup represents an automated backup record for an RDS cluster.
type DBClusterAutomatedBackup struct {
	DBClusterIdentifier   string `json:"dbClusterIdentifier"`
	DBClusterResourceID   string `json:"dbClusterResourceId"`
	Engine                string `json:"engine"`
	EngineVersion         string `json:"engineVersion"`
	Region                string `json:"region"`
	Status                string `json:"status"`
	BackupRetentionPeriod int    `json:"backupRetentionPeriod"`
	StorageEncrypted      bool   `json:"storageEncrypted"`
}

// DBSnapshotTenantDatabase represents a tenant database within a DB snapshot.
type DBSnapshotTenantDatabase struct {
	DBSnapshotIdentifier string `json:"dbSnapshotIdentifier"`
	DBInstanceIdentifier string `json:"dbInstanceIdentifier"`
	TenantDatabaseName   string `json:"tenantDatabaseName"`
	Engine               string `json:"engine"`
	Status               string `json:"status"`
}

// DBInstanceAutomatedBackup represents an automated backup record for an RDS instance.
type DBInstanceAutomatedBackup struct {
	DBInstanceIdentifier  string `json:"dbInstanceIdentifier"`
	DbiResourceID         string `json:"dbiResourceId"`
	Engine                string `json:"engine"`
	EngineVersion         string `json:"engineVersion"`
	DBInstanceArn         string `json:"dbInstanceArn"`
	Region                string `json:"region"`
	Status                string `json:"status"`
	AllocatedStorage      int    `json:"allocatedStorage"`
	Port                  int    `json:"port"`
	BackupRetentionPeriod int    `json:"backupRetentionPeriod"`
	Encrypted             bool   `json:"encrypted"`
}

// DBInstanceOptions holds optional fields for CreateDBInstance and ModifyDBInstance.
type DBInstanceOptions struct {
	EngineVersion                    string
	StorageType                      string
	AvailabilityZone                 string
	DBParameterGroupName             string
	OptionGroupName                  string
	SourceRegion                     string
	LicenseModel                     string
	MonitoringRoleArn                string
	PreferredMaintenanceWindow       string
	PreferredBackupWindow            string
	KmsKeyID                         string
	DBClusterIdentifier              string
	EngineLifecycleSupport           string
	DBSubnetGroupName                string
	VpcSecurityGroupIDs              []string
	DBSecurityGroupNames             []string
	EnabledCloudwatchLogsExports     []string
	BackupRetentionPeriod            int
	Iops                             int
	StorageThroughput                int
	MonitoringInterval               int
	MultiAZ                          bool
	MultiAZSet                       bool
	StorageEncrypted                 bool
	IAMDatabaseAuthenticationEnabled bool
	IAMDatabaseAuthSet               bool
	DeletionProtection               bool
	DeletionProtectionSet            bool
	CopyTagsToSnapshot               bool
	AllowMajorVersionUpgrade         bool
	ApplyImmediately                 bool
	PubliclyAccessible               bool
	PerformanceInsightsEnabled       bool
	StorageOptimized                 bool
	OptimizedWrites                  bool
}

// CopyDBSnapshotOptions holds optional fields for CopyDBSnapshot.
type CopyDBSnapshotOptions struct {
	KmsKeyID     string
	SourceRegion string
	CopyTags     bool
}

// DBClusterOptions holds optional fields for CreateDBCluster and ModifyDBCluster.
type DBClusterOptions struct {
	EngineVersion                string
	KmsKeyID                     string
	PreferredBackupWindow        string
	PreferredMaintenanceWindow   string
	MonitoringRoleArn            string
	StorageType                  string
	NetworkType                  string
	EngineLifecycleSupport       string
	ReplicationSourceIdentifier  string
	EnabledCloudwatchLogsExports []string
	AvailabilityZones            []string
	BacktrackWindow              int64
	BackupRetentionPeriod        int
	MonitoringInterval           int
	MultiAZ                      bool
	StorageEncrypted             bool
	StorageEncryptedChanged      bool
	CopyTagsToSnapshot           bool
	DeletionProtection           bool
	DeletionProtectionSet        bool
	OptimizedWrites              bool
}

// InMemoryBackend is the in-memory store for RDS resources.
type InMemoryBackend struct {
	dnsRegistrar              DNSRegistrar
	registry                  *store.Registry
	snapshotAttributes        *store.Table[DBSnapshotAttributesResult]
	reservedInstances         *store.Table[ReservedDBInstance]
	snapshots                 *store.Table[DBSnapshot]
	subnetGroups              *store.Table[DBSubnetGroup]
	tags                      map[string][]Tag
	instances                 *store.Table[DBInstance]
	clusterParameterGroups    *store.Table[DBParameterGroup]
	optionGroups              *store.Table[OptionGroup]
	clusters                  *store.Table[DBCluster]
	instanceReadyAt           map[string]time.Time
	clusterSnapshots          *store.Table[DBClusterSnapshot]
	eventSubscriptions        *store.Table[EventSubscription]
	globalClusters            *store.Table[GlobalCluster]
	clusterRoles              map[string][]DBClusterRole
	instanceRoles             map[string]map[string]string
	exportTasks               *store.Table[ExportTask]
	mu                        *lockmetrics.RWMutex
	dbSecurityGroups          *store.Table[DBSecurityGroup]
	blueGreenDeployments      *store.Table[BlueGreenDeployment]
	clusterEndpoints          *store.Table[DBClusterEndpoint]
	parameterGroups           *store.Table[DBParameterGroup]
	recommendations           *store.Table[DBRecommendation]
	clusterSnapshotAttributes *store.Table[DBClusterSnapshotAttributesResult]
	proxies                   *store.Table[DBProxy]
	proxyTargetGroups         *store.Table[DBProxyTargetGroup]
	proxyTargets              map[string][]DBProxyTarget
	proxyEndpoints            *store.Table[DBProxyEndpoint]
	customEngineVersions      *store.Table[CustomDBEngineVersion]
	fisFailoverFaults         map[string]time.Time
	automatedBackups          map[string]*DBInstanceAutomatedBackup
	shardGroups               *store.Table[DBShardGroup]
	integrations              *store.Table[Integration]
	tenantDatabases           *store.Table[TenantDatabase]
	clusterAutomatedBackups   *store.Table[DBClusterAutomatedBackup]
	snapshotTenantDatabases   map[string][]*DBSnapshotTenantDatabase
	clusterReadyAt            map[string]time.Time
	piMetrics                 map[string]map[string][]PIDataPoint
	instanceLogFiles          map[string][]DBLogFile
	instanceLogContent        map[string]map[string]string
	accountID                 string
	region                    string
	defaultCACertificateID    string
	events                    []Event
	reconcilerRunning         bool
}
