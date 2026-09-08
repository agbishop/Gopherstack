package elasticache

import (
	"context"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// Cluster represents an ElastiCache cluster.
type Cluster struct {
	CreatedAt                  time.Time
	Tags                       *tags.Tags
	mini                       *miniredis.Miniredis
	ClusterID                  string
	Engine                     string
	EngineVersion              string
	Status                     string
	Endpoint                   string
	NodeType                   string
	ARN                        string
	Region                     string
	CacheParameterGroupName    string
	PreferredMaintenanceWindow string
	SnapshotWindow             string
	ReplicationGroupID         string
	KmsKeyID                   string
	TransitEncryptionMode      string
	ConnectAddress             string
	PendingStatus              string
	SubnetGroupName            string
	AvailableAt                time.Time
	Members                    []CacheNodeMember
	Port                       int
	AllocatedPort              int
	NumCacheNodes              int
	SnapshotRetentionLimit     int
	TransitEncryptionEnabled   bool
	AtRestEncryptionEnabled    bool
}

// ReplicationGroup represents an ElastiCache replication group.
type ReplicationGroup struct {
	CreatedAt                  time.Time                `json:"createdAt"`
	AuthTokenLastModifiedDate  *time.Time               `json:"authTokenLastModifiedDate,omitempty"`
	AvailableAt                time.Time                `json:"availableAt,omitzero"`
	PendingModifiedValues      *RGPendingModifiedValues `json:"pendingModifiedValues,omitempty"`
	Tags                       *tags.Tags               `json:"tags,omitempty"`
	ReplicationGroupID         string                   `json:"replicationGroupID"`
	Description                string                   `json:"description"`
	Status                     string                   `json:"status"`
	PendingStatus              string                   `json:"pendingStatus,omitempty"`
	ARN                        string                   `json:"arn"`
	Engine                     string                   `json:"engine,omitempty"`
	CacheParameterGroupName    string                   `json:"cacheParameterGroupName,omitempty"`
	AutomaticFailover          string                   `json:"automaticFailover,omitempty"`
	EngineVersion              string                   `json:"engineVersion,omitempty"`
	CacheNodeType              string                   `json:"cacheNodeType,omitempty"`
	PreferredMaintenanceWindow string                   `json:"preferredMaintenanceWindow,omitempty"`
	SnapshotWindow             string                   `json:"snapshotWindow,omitempty"`
	AuthToken                  string                   `json:"authToken,omitempty"`
	KmsKeyID                   string                   `json:"kmsKeyId,omitempty"`
	NotificationTopicArn       string                   `json:"notificationTopicArn,omitempty"`
	TransitEncryptionMode      string                   `json:"transitEncryptionMode,omitempty"`
	Durability                 string                   `json:"durability,omitempty"`
	NodeGroups                 []NodeGroup              `json:"nodeGroups,omitempty"`
	LogDeliveryConfigurations  []LogDeliveryConfig      `json:"logDeliveryConfigurations,omitempty"`
	UserGroupIDs               []string                 `json:"userGroupIds,omitempty"`
	SnapshotRetentionLimit     int                      `json:"snapshotRetentionLimit,omitempty"`
	ReplicaCount               int32                    `json:"replicaCount,omitempty"`
	ClusterModeEnabled         bool                     `json:"clusterModeEnabled,omitempty"`
	AuthTokenEnabled           bool                     `json:"authTokenEnabled,omitempty"`
	AtRestEncryptionEnabled    bool                     `json:"atRestEncryptionEnabled,omitempty"`
	TransitEncryptionEnabled   bool                     `json:"transitEncryptionEnabled,omitempty"`
	DataTieringEnabled         bool                     `json:"dataTieringEnabled,omitempty"`
	MultiAZEnabled             bool                     `json:"multiAZEnabled,omitempty"`
}

// CacheParameterGroup represents an ElastiCache parameter group.
type CacheParameterGroup struct {
	Tags        *tags.Tags        `json:"tags,omitempty"`
	Parameters  map[string]string `json:"parameters"`
	Name        string            `json:"name"`
	Family      string            `json:"family"`
	Description string            `json:"description"`
	ARN         string            `json:"arn"`
	IsGlobal    bool              `json:"isGlobal"`
}

// CacheSubnetGroup represents an ElastiCache subnet group.
type CacheSubnetGroup struct {
	Tags        *tags.Tags `json:"tags,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	VpcID       string     `json:"vpcId"`
	ARN         string     `json:"arn"`
	SubnetIDs   []string   `json:"subnetIds"`
}

// CacheSnapshot represents an ElastiCache snapshot.
type CacheSnapshot struct {
	CreatedAt   time.Time `json:"createdAt"`
	AvailableAt time.Time `json:"availableAt,omitzero"`
	// SourceClusterCreatedAt is the source CacheCluster's own CreatedAt,
	// captured at snapshot time. Zero when the snapshot was taken from a
	// replication group instead (member-cluster creation time isn't tracked
	// there). This backs the wire's CacheClusterCreateTime member, which is
	// the source cluster's creation time -- not the snapshot's.
	SourceClusterCreatedAt time.Time  `json:"sourceClusterCreatedAt,omitzero"`
	Tags                   *tags.Tags `json:"tags,omitempty"`
	SnapshotName           string     `json:"snapshotName"`
	CacheClusterID         string     `json:"cacheClusterId"`
	ReplicationGroupID     string     `json:"replicationGroupId"`
	Status                 string     `json:"status"`
	PendingStatus          string     `json:"pendingStatus,omitempty"`
	ARN                    string     `json:"arn"`
	Engine                 string     `json:"engine"`
	EngineVersion          string     `json:"engineVersion"`
	NodeType               string     `json:"nodeType"`
	KmsKeyID               string     `json:"kmsKeyId,omitempty"`
	SnapshotSource         string     `json:"snapshotSource"` // "manual" or "automated"
}

// StorageBackend defines the interface for the ElastiCache in-memory store.
type StorageBackend interface {
	CreateCluster(ctx context.Context, id, engine, nodeType string, port int) (*Cluster, error)
	CreateClusterWithOptions(
		ctx context.Context,
		id, engine, nodeType, paramGroupName, maintenanceWindow, snapshotWindow string,
		numCacheNodes, port int,
	) (*Cluster, error)
	DeleteCluster(ctx context.Context, id string) error
	SetClusterSubnetGroupName(ctx context.Context, id, subnetGroupName string) error
	SetClusterSnapshotRetentionLimit(ctx context.Context, id string, limit *int) error
	SetClusterReplicationGroupID(ctx context.Context, id, replicationGroupID string) error
	DescribeClusters(ctx context.Context, id, marker string, maxRecords int, notInRG bool) (page.Page[Cluster], error)
	ModifyCluster(
		ctx context.Context,
		id, nodeType, paramGroupName, engineVersion, maintenanceWindow, snapshotWindow string,
		numCacheNodes int,
	) (*Cluster, error)
	ListTagsForResource(ctx context.Context, arn string) (map[string]string, error)
	AddTagsToResource(ctx context.Context, arn string, newTags map[string]string) error
	RemoveTagsFromResource(ctx context.Context, arn string, tagKeys []string) error
	CreateReplicationGroup(ctx context.Context, id, description string) (*ReplicationGroup, error)
	CreateReplicationGroupWithOptions(
		ctx context.Context,
		id, description, paramGroupName, maintenanceWindow, snapshotWindow string,
	) (*ReplicationGroup, error)
	DeleteReplicationGroup(ctx context.Context, id string) error
	DescribeReplicationGroups(
		ctx context.Context,
		id, marker string,
		maxRecords int,
	) (page.Page[ReplicationGroup], error)
	ModifyReplicationGroup(
		ctx context.Context,
		id, description, paramGroupName, engineVersion, cacheNodeType, maintenanceWindow, snapshotWindow string,
		automaticFailoverEnabled, multiAZEnabled *bool,
	) (*ReplicationGroup, error)
	FailoverReplicationGroup(ctx context.Context, id, nodeGroupID string) (*ReplicationGroup, error)
	CreateParameterGroup(ctx context.Context, name, family, description string) (*CacheParameterGroup, error)
	DeleteParameterGroup(ctx context.Context, name string) error
	DescribeParameterGroups(
		ctx context.Context,
		name, marker string,
		maxRecords int,
	) (page.Page[CacheParameterGroup], error)
	ModifyParameterGroup(ctx context.Context, name string, params map[string]string) (*CacheParameterGroup, error)
	ResetParameterGroup(
		ctx context.Context,
		name string,
		paramNames []string,
		resetAll bool,
	) (*CacheParameterGroup, error)
	DescribeParameters(ctx context.Context, name, marker string, maxRecords int) (page.Page[CacheParameter], error)
	CreateSubnetGroup(ctx context.Context, name, description string, subnetIDs []string) (*CacheSubnetGroup, error)
	CreateSubnetGroupFull(
		ctx context.Context,
		name, description, vpcID string,
		subnetIDs []string,
	) (*CacheSubnetGroup, error)
	DeleteSubnetGroup(ctx context.Context, name string) error
	DescribeSubnetGroups(ctx context.Context, name, marker string, maxRecords int) (page.Page[CacheSubnetGroup], error)
	ModifySubnetGroup(ctx context.Context, name, description string, subnetIDs []string) (*CacheSubnetGroup, error)
	CreateSnapshot(ctx context.Context, snapshotName, clusterID, replicationGroupID string) (*CacheSnapshot, error)
	DeleteSnapshot(ctx context.Context, snapshotName string) (*CacheSnapshot, error)
	DescribeSnapshots(
		ctx context.Context,
		snapshotName, clusterID, replicationGroupID, snapshotSource, marker string,
		maxRecords int,
	) (page.Page[CacheSnapshot], error)
	CopySnapshot(ctx context.Context, sourceSnapshotName, targetSnapshotName string) (*CacheSnapshot, error)
	CopySnapshotFull(
		ctx context.Context,
		sourceSnapshotName, targetSnapshotName, kmsKeyID string,
	) (*CacheSnapshot, error)
	DescribeEvents(
		ctx context.Context,
		sourceIdentifier, sourceType, marker string,
		startTime, endTime time.Time,
		duration, maxRecords int,
	) (page.Page[CacheEvent], error)
	// New ops
	CreateCacheSecurityGroup(ctx context.Context, name, description string) (*CacheSecurityGroup, error)
	AuthorizeCacheSecurityGroupIngress(
		ctx context.Context,
		name, ec2SecurityGroupName, ec2SecurityGroupOwnerID string,
	) (*CacheSecurityGroup, error)
	CreateGlobalReplicationGroup(
		ctx context.Context,
		globalReplicationGroupIDSuffix, description, primaryReplicationGroupID string,
	) (*GlobalReplicationGroup, error)
	CreateServerlessCache(ctx context.Context, name, description, engine string) (*ServerlessCache, error)
	CreateServerlessCacheSnapshot(
		ctx context.Context,
		snapshotName, serverlessCacheName, kmsKeyID string,
	) (*ServerlessCacheSnapshot, error)
	CopyServerlessCacheSnapshot(
		ctx context.Context,
		sourceSnapshotName, targetSnapshotName string,
	) (*ServerlessCacheSnapshot, error)
	CreateUser(
		ctx context.Context,
		userID, userName, accessString, engine string,
		noPasswordRequired bool,
	) (*User, error)
	CreateUserWithAuth(
		ctx context.Context,
		userID, userName, accessString, engine, authType string,
		passwordCount int,
	) (*User, error)
	BatchApplyUpdateAction(
		ctx context.Context,
		replicationGroupIDs, cacheClusterIDs []string,
		serviceUpdateName string,
	) (*BatchUpdateResult, error)
	BatchStopUpdateAction(
		ctx context.Context,
		replicationGroupIDs, cacheClusterIDs []string,
		serviceUpdateName string,
	) (*BatchUpdateResult, error)
	CompleteMigration(ctx context.Context, replicationGroupID string, force bool) (*ReplicationGroup, error)
	// User operations
	DeleteUser(ctx context.Context, userID string) (*User, error)
	DescribeUsers(
		ctx context.Context,
		userID, marker, engine string,
		maxRecords int,
		filterUserIDs []string,
	) (page.Page[User], error)
	ModifyUser(ctx context.Context, userID, accessString string, noPasswordRequired bool) (*User, error)
	ModifyUserWithAuth(
		ctx context.Context,
		userID, accessString, appendAccessString, engine, authType string,
		passwordCount *int,
	) (*User, error)
	// UserGroup operations
	CreateUserGroup(ctx context.Context, groupID, engine string, userIDs []string) (*UserGroup, error)
	CreateUserGroupValidated(
		ctx context.Context,
		groupID, engine string,
		userIDs []string,
	) (*UserGroup, error)
	DeleteUserGroup(ctx context.Context, groupID string) (*UserGroup, error)
	DescribeUserGroups(ctx context.Context, groupID, marker string, maxRecords int) (page.Page[UserGroup], error)
	ModifyUserGroup(ctx context.Context, groupID string, userIDsToAdd, userIDsToRemove []string) (*UserGroup, error)
	// GlobalReplicationGroup operations
	DeleteGlobalReplicationGroup(
		ctx context.Context,
		id string,
		retainPrimaryReplicationGroup bool,
	) (*GlobalReplicationGroup, error)
	DescribeGlobalReplicationGroups(
		ctx context.Context,
		id, marker string,
		maxRecords int,
	) (page.Page[GlobalReplicationGroup], error)
	DisassociateGlobalReplicationGroup(
		ctx context.Context,
		id, replicationGroupID, replicationGroupRegion string,
	) (*GlobalReplicationGroup, error)
	FailoverGlobalReplicationGroup(
		ctx context.Context,
		id, primaryRegion, primaryReplicationGroupID string,
	) (*GlobalReplicationGroup, error)
	IncreaseNodeGroupsInGlobalReplicationGroup(
		ctx context.Context,
		id string,
		nodeGroupCount int32,
		applyImmediately bool,
	) (*GlobalReplicationGroup, error)
	DecreaseNodeGroupsInGlobalReplicationGroup(
		ctx context.Context,
		id string,
		nodeGroupCount int32,
		applyImmediately bool,
	) (*GlobalReplicationGroup, error)
	ModifyGlobalReplicationGroup(
		ctx context.Context,
		id, description, engineVersion string,
		automaticFailoverEnabled, applyImmediately bool,
	) (*GlobalReplicationGroup, error)
	RebalanceSlotsInGlobalReplicationGroup(
		ctx context.Context,
		id string,
		applyImmediately bool,
	) (*GlobalReplicationGroup, error)
	// ReservedCacheNodes operations
	DescribeReservedCacheNodes(
		ctx context.Context,
		id, cacheNodeType, offeringType, duration, productDescription, marker string,
		maxRecords int,
	) (page.Page[ReservedCacheNode], error)
	DescribeReservedCacheNodesOfferings(
		ctx context.Context,
		offeringID, cacheNodeType, offeringType, duration, productDescription, marker string,
		maxRecords int,
	) (page.Page[ReservedCacheNodesOffering], error)
	PurchaseReservedCacheNodesOffering(
		ctx context.Context,
		offeringID, reservedCacheNodeID string,
		cacheNodeCount int32,
	) (*ReservedCacheNode, error)
	// ServerlessCache operations
	DeleteServerlessCache(ctx context.Context, name string) (*ServerlessCache, error)
	DeleteServerlessCacheSnapshot(ctx context.Context, name string) (*ServerlessCacheSnapshot, error)
	DescribeServerlessCaches(
		ctx context.Context,
		name, marker string,
		maxRecords int,
	) (page.Page[ServerlessCache], error)
	DescribeServerlessCacheSnapshots(
		ctx context.Context,
		serverlessCacheName, snapshotName, marker string,
		maxRecords int,
	) (page.Page[ServerlessCacheSnapshot], error)
	ExportServerlessCacheSnapshot(
		ctx context.Context,
		snapshotName, s3BucketName string,
	) (*ServerlessCacheSnapshot, error)
	ModifyServerlessCache(ctx context.Context, name, description string) (*ServerlessCache, error)
	CreateServerlessCacheFull(ctx context.Context, opts ServerlessCreateOpts) (*ServerlessCache, error)
	ModifyServerlessCacheFull(ctx context.Context, name string, opts ServerlessModifyOpts) (*ServerlessCache, error)
	// Migration operations
	StartMigration(
		ctx context.Context,
		replicationGroupID string,
		customerNodeEndpoints []CustomerNodeEndpoint,
	) (*ReplicationGroup, error)
	TestMigration(
		ctx context.Context,
		replicationGroupID string,
		customerNodeEndpoints []CustomerNodeEndpoint,
	) (*ReplicationGroup, error)
	IncreaseReplicaCount(
		ctx context.Context,
		replicationGroupID string,
		newReplicaCount int32,
		applyImmediately bool,
	) (*ReplicationGroup, error)
	DecreaseReplicaCount(
		ctx context.Context,
		replicationGroupID string,
		newReplicaCount int32,
		applyImmediately bool,
	) (*ReplicationGroup, error)
	ModifyReplicationGroupShardConfiguration(
		ctx context.Context,
		replicationGroupID string,
		nodeGroupCount int32,
		applyImmediately bool,
	) (*ReplicationGroup, error)
	// Cache info operations
	DescribeCacheEngineVersions(
		ctx context.Context,
		engine, family, engineVersion, marker string,
		maxRecords int,
	) (page.Page[CacheEngineVersion], error)
	RebootCacheCluster(ctx context.Context, clusterID string, nodeIDs []string) (*Cluster, error)
	DeleteCacheSecurityGroup(ctx context.Context, name string) error
	DescribeCacheSecurityGroups(
		ctx context.Context,
		name, marker string,
		maxRecords int,
	) (page.Page[CacheSecurityGroup], error)
	RevokeCacheSecurityGroupIngress(
		ctx context.Context,
		name, ec2SecurityGroupName, ec2SecurityGroupOwnerID string,
	) (*CacheSecurityGroup, error)
	DescribeEngineDefaultParameters(
		ctx context.Context,
		cacheParameterGroupFamily, marker string,
		maxRecords int,
	) (page.Page[CacheParameter], error)
	DescribeServiceUpdates(
		ctx context.Context,
		serviceUpdateName, marker string,
		maxRecords int,
		status []string,
	) (page.Page[ServiceUpdate], error)
	DescribeUpdateActions(
		ctx context.Context,
		serviceUpdateName, marker string,
		maxRecords int,
		cacheClusterIDs, replicationGroupIDs, updateActionStatus []string,
	) (page.Page[UpdateAction], error)
	ListAllowedNodeTypeModifications(ctx context.Context, clusterID, replicationGroupID string) ([]string, error)
	// Audit1: extended create/modify with new fields
	CreateReplicationGroupFull(ctx context.Context, opts ReplicationGroupCreateOpts) (*ReplicationGroup, error)
	ModifyReplicationGroupFull(
		ctx context.Context,
		id string,
		opts ReplicationGroupModifyOpts,
	) (*ReplicationGroup, error)
	// Audit1: auto snapshot scheduling
	TriggerAutoSnapshot(ctx context.Context, replicationGroupID string) (*CacheSnapshot, error)
	// Batch-2: update action tracking
	AppendUpdateActions(actions []*UpdateAction)
	ListUpdateActionsByServiceUpdate(serviceUpdateName string) []*UpdateAction
	// Region returns the backend's default AWS region.
	Region() string
}

// CacheParameter represents a single cache parameter (for DescribeParameters response).
type CacheParameter struct {
	Name          string
	Value         string
	Description   string
	DataType      string
	AllowedValues string
	IsModifiable  bool
}

// DNSRegistrar can register and deregister hostnames with an embedded DNS server.
type DNSRegistrar interface {
	Register(hostname string)
	Deregister(hostname string)
}

// dnsRecordRegistrar is an optional capability: a registrar that can bind a
// hostname to a specific record value (e.g. an A record to the real bound IP of
// the embedded redis). The concrete embedded DNS server implements this, so the
// published synthetic endpoint resolves to a genuinely connectable address.
type dnsRecordRegistrar interface {
	RegisterRecord(hostname, recordType string, values []string)
}

// InMemoryBackend is an in-memory ElastiCache backend.
// All regional resource maps are nested by region (outer key = region) so that
// same-named resources in different regions are fully isolated. GlobalReplicationGroups
// are global/partition-scoped (like AWS) and therefore are NOT region-nested.
type InMemoryBackend struct {
	dnsRegistrar              DNSRegistrar
	registry                  *store.Registry
	globalReplicationGroups   *store.Table[GlobalReplicationGroup]
	serverlessCaches          map[string]*store.Table[ServerlessCache]
	userGroups                map[string]*store.Table[UserGroup]
	parameterGroups           map[string]*store.Table[CacheParameterGroup]
	snapshots                 map[string]*store.Table[CacheSnapshot]
	cacheSecurityGroups       map[string]*store.Table[CacheSecurityGroup]
	cacheSecurityGroupIngress map[string]map[string][]EC2SecurityGroupMembership
	clusters                  map[string]*store.Table[Cluster]
	replicationGroups         map[string]*store.Table[ReplicationGroup]
	reservedCacheNodes        map[string]*store.Table[ReservedCacheNode]
	serverlessCacheSnapshots  map[string]*store.Table[ServerlessCacheSnapshot]
	subnetGroups              map[string]*store.Table[CacheSubnetGroup]
	users                     map[string]*store.Table[User]
	events                    *eventRing
	mu                        *lockmetrics.RWMutex
	allocator                 *portalloc.Allocator
	clock                     func() time.Time
	region                    string
	engineMode                string
	accountID                 string
	updateActions             []*UpdateAction
	lifecycleDelay            time.Duration
}

// CacheNodeMember represents a Memcached cluster node member (gap #3).
type CacheNodeMember struct {
	CacheClusterID            string    `json:"cacheClusterId"`
	CacheNodeID               string    `json:"cacheNodeId"`
	CacheNodeStatus           string    `json:"cacheNodeStatus"`
	PreferredAvailabilityZone string    `json:"preferredAvailabilityZone"`
	CacheNodeCreateTime       time.Time `json:"cacheNodeCreateTime"`
	Endpoint                  struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
	} `json:"endpoint"`
}

// NodeGroupNode represents a single node within a node group (gap #2).
type NodeGroupNode struct {
	ReadEndpointAddress       string `json:"readEndpointAddress,omitempty"`
	CacheClusterID            string `json:"cacheClusterId"`
	CacheNodeID               string `json:"cacheNodeId"`
	CurrentRole               string `json:"currentRole"` // "primary" or "replica"
	PreferredAvailabilityZone string `json:"preferredAvailabilityZone"`
	ReadEndpointPort          int    `json:"readEndpointPort,omitempty"`
}

// NodeGroup represents a shard / node group in a cluster-mode-enabled replication group (gap #2).
type NodeGroup struct {
	PrimaryNode *NodeGroupNode  `json:"primaryNode,omitempty"`
	NodeGroupID string          `json:"nodeGroupId"`
	Status      string          `json:"status"`
	Slots       string          `json:"slots"`
	Replicas    []NodeGroupNode `json:"replicas,omitempty"`
}

// RGPendingModifiedValues holds modifications queued for the next maintenance window (gap #7).
type RGPendingModifiedValues struct {
	ReplicaCount            *int32 `json:"replicaCount,omitempty"`
	CacheNodeType           string `json:"cacheNodeType,omitempty"`
	EngineVersion           string `json:"engineVersion,omitempty"`
	AuthTokenStatus         string `json:"authTokenStatus,omitempty"`
	AutomaticFailoverStatus string `json:"automaticFailoverStatus,omitempty"`
}

// LogDeliveryConfig holds log delivery configuration for slow-log or engine-log (gap #6).
type LogDeliveryConfig struct {
	DestinationDetails string `json:"destinationDetails"`
	LogType            string `json:"logType"`         // "slow-log" or "engine-log"
	DestinationType    string `json:"destinationType"` // "cloudwatch-logs" or "kinesis-firehose"
	LogFormat          string `json:"logFormat"`       // "text" or "json"
	Status             string `json:"status"`
	Message            string `json:"message,omitempty"`
}

// ----------------------------------------
// CreateReplicationGroupFull options (all gaps)
// ----------------------------------------

// ReplicationGroupCreateOpts carries all fields for full replication-group creation.
type ReplicationGroupCreateOpts struct {
	Tags               map[string]string
	Engine             string
	EngineVersion      string
	ID                 string
	Description        string
	ParameterGroupName string
	// SnapshotName, when set, restores the new replication group from an
	// existing snapshot: the snapshot must exist, and its engine/node type
	// become defaults for any field the caller didn't explicitly set.
	SnapshotName              string
	MaintenanceWindow         string
	TransitEncryptionMode     string
	AuthToken                 string
	KmsKeyID                  string
	NotificationTopicArn      string
	CacheNodeType             string
	SnapshotWindow            string
	Durability                string
	UserGroupIDs              []string
	LogDeliveryConfigurations []LogDeliveryConfig
	SnapshotRetentionLimit    int
	ReplicasPerNodeGroup      int32
	NumNodeGroups             int32
	ClusterModeEnabled        bool
	AuthTokenEnabled          bool
	AtRestEncryptionEnabled   bool
	TransitEncryptionEnabled  bool
	DataTieringEnabled        bool
	AutomaticFailoverEnabled  bool
	MultiAZEnabled            bool
}

// ReplicationGroupModifyOpts carries all fields for full replication-group modification.
type ReplicationGroupModifyOpts struct {
	SnapshotRetentionLimit    *int
	ReplicaCount              *int32
	AutomaticFailoverEnabled  *bool
	MultiAZEnabled            *bool
	Description               string
	ParameterGroupName        string
	EngineVersion             string
	CacheNodeType             string
	MaintenanceWindow         string
	SnapshotWindow            string
	AuthToken                 string
	AuthTokenUpdateStrategy   string
	NotificationTopicArn      string
	TransitEncryptionMode     string
	Durability                string
	LogDeliveryConfigurations []LogDeliveryConfig
	UserGroupIDsToAdd         []string
	UserGroupIDsToRemove      []string
	ApplyImmediately          bool
}

// CustomerNodeEndpoint is a source endpoint for StartMigration/TestMigration
// (aws-sdk-go-v2/service/elasticache/types.CustomerNodeEndpoint). It has no
// output-side wire counterpart -- AWS's ReplicationGroup response never
// echoes it back -- so it exists purely as an input shape.
type CustomerNodeEndpoint struct {
	Address string
	Port    int32
}

// ----------------------------------------
// resizeNodeGroups helper (gap #2)
// ----------------------------------------

// CacheSecurityGroup represents an ElastiCache cache security group (EC2-Classic).
type CacheSecurityGroup struct {
	Tags        *tags.Tags `json:"tags,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	ARN         string     `json:"arn"`
	OwnerID     string     `json:"ownerId"`
}

// EC2SecurityGroupMembership is a single EC2 security group authorization on a cache security group.
type EC2SecurityGroupMembership struct {
	EC2SecurityGroupName    string `json:"ec2SecurityGroupName"`
	EC2SecurityGroupOwnerID string `json:"ec2SecurityGroupOwnerId"`
	Status                  string `json:"status"`
}

// GlobalReplicationGroup represents an ElastiCache global replication group.
type GlobalReplicationGroup struct {
	CreatedAt                     time.Time         `json:"createdAt"`
	AvailableAt                   time.Time         `json:"availableAt,omitzero"`
	Tags                          *tags.Tags        `json:"tags,omitempty"`
	SecondaryReplicationGroups    map[string]string `json:"secondaryReplicationGroups,omitempty"`
	GlobalReplicationGroupID      string            `json:"globalReplicationGroupId"`
	Description                   string            `json:"description"`
	Status                        string            `json:"status"`
	PendingStatus                 string            `json:"pendingStatus,omitempty"`
	ARN                           string            `json:"arn"`
	Engine                        string            `json:"engine"`
	EngineVersion                 string            `json:"engineVersion"`
	PrimaryReplicationGroupRegion string            `json:"primaryReplicationGroupRegion,omitempty"`
	NodeGroupCount                int32             `json:"nodeGroupCount,omitempty"`
}

// ServerlessCacheEndpoint holds the address and port for a serverless cache endpoint.
type ServerlessCacheEndpoint struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// DataStorageLimit models the data-storage half of a serverless cache's usage
// limits (aws-sdk-go-v2/service/elasticache/types.DataStorage). Unit is
// always "GB" on real AWS (the only value DataStorageUnit currently has).
type DataStorageLimit struct {
	Unit    string `json:"unit,omitempty"`
	Maximum int32  `json:"maximum,omitempty"`
	Minimum int32  `json:"minimum,omitempty"`
}

// ECPUPerSecondLimit models the ElastiCache-Processing-Unit half of a
// serverless cache's usage limits (aws-sdk-go-v2/service/elasticache/types.
// ECPUPerSecond).
type ECPUPerSecondLimit struct {
	Maximum int32 `json:"maximum,omitempty"`
	Minimum int32 `json:"minimum,omitempty"`
}

// CacheUsageLimits models a serverless cache's data/ECPU usage limits
// (aws-sdk-go-v2/service/elasticache/types.CacheUsageLimits).
type CacheUsageLimits struct {
	DataStorage   *DataStorageLimit   `json:"dataStorage,omitempty"`
	ECPUPerSecond *ECPUPerSecondLimit `json:"ecpuPerSecond,omitempty"`
}

// ServerlessCache represents an ElastiCache serverless cache.
type ServerlessCache struct {
	CreatedAt              time.Time                `json:"createdAt"`
	AvailableAt            time.Time                `json:"availableAt,omitzero"`
	Tags                   *tags.Tags               `json:"tags,omitempty"`
	Endpoint               *ServerlessCacheEndpoint `json:"endpoint,omitempty"`
	ReaderEndpoint         *ServerlessCacheEndpoint `json:"readerEndpoint,omitempty"`
	CacheUsageLimits       *CacheUsageLimits        `json:"cacheUsageLimits,omitempty"`
	Name                   string                   `json:"name"`
	Description            string                   `json:"description"`
	Status                 string                   `json:"status"`
	PendingStatus          string                   `json:"pendingStatus,omitempty"`
	ARN                    string                   `json:"arn"`
	Engine                 string                   `json:"engine"`
	KmsKeyID               string                   `json:"kmsKeyId,omitempty"`
	UserGroupID            string                   `json:"userGroupId,omitempty"`
	SubnetGroupName        string                   `json:"subnetGroupName,omitempty"`
	DailySnapshotTime      string                   `json:"dailySnapshotTime,omitempty"`
	MajorEngineVersion     string                   `json:"majorEngineVersion,omitempty"`
	NetworkType            string                   `json:"networkType,omitempty"`
	SubnetIDs              []string                 `json:"subnetIds,omitempty"`
	SecurityGroupIDs       []string                 `json:"securityGroupIds,omitempty"`
	SnapshotRetentionLimit int32                    `json:"snapshotRetentionLimit,omitempty"`
}

// ServerlessCacheConfigSnapshot mirrors a serverless cache's configuration as
// it existed at the moment a snapshot was taken (aws-sdk-go-v2/service/
// elasticache/types.ServerlessCacheConfiguration).
type ServerlessCacheConfigSnapshot struct {
	ServerlessCacheName string `json:"serverlessCacheName,omitempty"`
	Engine              string `json:"engine,omitempty"`
	MajorEngineVersion  string `json:"majorEngineVersion,omitempty"`
}

// ServerlessCacheSnapshot represents a snapshot of a serverless cache.
//
// ExpiryTime is deliberately left zero for every snapshot this emulator
// creates: real AWS only sets it for automatically-created snapshots (expiry
// driven by the source cache's SnapshotRetentionLimit), never for manual or
// copied ones -- and CreateServerlessCacheSnapshot/CopyServerlessCacheSnapshot
// only ever produce "manual"-type snapshots here (see SnapshotType), matching
// the already-documented deferred gap that this emulator has no background
// automated-snapshot scheduler. BytesUsedForCache is always "0": serverless
// caches in this emulator have no real backing data-plane engine (unlike
// Cluster, which uses an embedded miniredis instance) to compute an accurate
// byte count from, so "0" is the literal, non-fabricated true size of the
// (empty) data this emulator actually holds for it.
type ServerlessCacheSnapshot struct {
	CreatedAt                    time.Time                      `json:"createdAt"`
	ExpiryTime                   time.Time                      `json:"expiryTime,omitzero"`
	Tags                         *tags.Tags                     `json:"tags,omitempty"`
	ServerlessCacheConfiguration *ServerlessCacheConfigSnapshot `json:"serverlessCacheConfiguration,omitempty"`
	Name                         string                         `json:"name"`
	Status                       string                         `json:"status"`
	ARN                          string                         `json:"arn"`
	ServerlessCacheName          string                         `json:"serverlessCacheName"`
	SnapshotType                 string                         `json:"snapshotType"` // "manual" or "automated"
	KmsKeyID                     string                         `json:"kmsKeyId,omitempty"`
	BytesUsedForCache            string                         `json:"bytesUsedForCache,omitempty"`
}

// User represents an ElastiCache user.
//
// AuthType holds the wire-accurate OUTPUT authentication type -- one of
// "password", "no-password", or "iam" (types.AuthenticationType). Note this
// differs from the INPUT enum accepted on Create/ModifyUser
// (types.InputAuthenticationType), which spells the no-password case
// "no-password-required"; the two must not be confused when field-diffing
// against the SDK. PasswordCount reflects len(Passwords) (max 2, enforced at
// the handler); the plaintext passwords themselves are never echoed back on
// the wire, matching AWS. UserGroupIDs is derived (the reverse of
// UserGroup.UserIDs) and populated fresh on every response, not persisted.
type User struct {
	CreatedAt     time.Time  `json:"createdAt"`
	Tags          *tags.Tags `json:"tags,omitempty"`
	UserID        string     `json:"userId"`
	UserName      string     `json:"userName"`
	Status        string     `json:"status"`
	ARN           string     `json:"arn"`
	Engine        string     `json:"engine"`
	AccessString  string     `json:"accessString"`
	AuthType      string     `json:"authType"`
	UserGroupIDs  []string   `json:"userGroupIds,omitempty"`
	PasswordCount int        `json:"passwordCount"`
}

// UpdateActionResult represents the outcome of a single update action.
type UpdateActionResult struct {
	ReplicationGroupID string `json:"replicationGroupId,omitempty"`
	CacheClusterID     string `json:"cacheClusterId,omitempty"`
	ServiceUpdateName  string `json:"serviceUpdateName"`
	UpdateActionStatus string `json:"updateActionStatus"`
}

// BatchUpdateResult holds the results of a BatchApplyUpdateAction / BatchStopUpdateAction call.
type BatchUpdateResult struct {
	ProcessedUpdateActions   []UpdateActionResult `json:"processedUpdateActions"`
	UnprocessedUpdateActions []UpdateActionResult `json:"unprocessedUpdateActions"`
}

// ----------------------------------------
// ARN builders
// ----------------------------------------

// UserGroup represents an ElastiCache user group.
//
// AssignedReplicationGroupIDs mirrors the wire ReplicationGroups field
// (types.UserGroup.ReplicationGroups): the reverse of
// ReplicationGroup.UserGroupIDs, computed fresh on every response rather
// than persisted (see userGroupReplicationGroupIDsLocked), matching how
// User.UserGroupIDs is derived. AssignedServerlessCacheIDs mirrors the wire
// ServerlessCaches field (types.UserGroup.ServerlessCaches) the same way:
// the reverse of ServerlessCache.UserGroupID (see
// userGroupServerlessCacheIDsLocked). Note: the real SDK's UserGroup type
// has NO Description field -- a prior pass invented one and serialized it
// on the wire; do not re-add it.
type UserGroup struct {
	CreatedAt                   time.Time  `json:"createdAt"`
	Tags                        *tags.Tags `json:"tags,omitempty"`
	UserGroupID                 string     `json:"userGroupID"`
	Status                      string     `json:"status"`
	ARN                         string     `json:"arn"`
	Engine                      string     `json:"engine"`
	UserIDs                     []string   `json:"userIDs,omitempty"`
	AssignedReplicationGroupIDs []string   `json:"assignedReplicationGroupIDs,omitempty"`
	AssignedServerlessCacheIDs  []string   `json:"assignedServerlessCacheIDs,omitempty"`
}

// ReservedCacheNode represents a purchased reserved cache node.
type ReservedCacheNode struct {
	StartTime           time.Time `json:"startTime"`
	ReservationID       string    `json:"reservationID,omitempty"`
	ReservedCacheNodeID string    `json:"reservedCacheNodeID"`
	ARN                 string    `json:"arn"`
	CacheNodeType       string    `json:"cacheNodeType"`
	OfferingType        string    `json:"offeringType"`
	ProductDescription  string    `json:"productDescription"`
	State               string    `json:"state"`
	OfferingID          string    `json:"offeringID"`
	FixedPrice          float64   `json:"fixedPrice"`
	UsagePrice          float64   `json:"usagePrice"`
	Duration            int32     `json:"duration"`
	CacheNodeCount      int32     `json:"cacheNodeCount"`
}

// ReservedCacheNodesOffering represents a reserved node offering.
type ReservedCacheNodesOffering struct {
	OfferingID         string
	CacheNodeType      string
	ProductDescription string
	OfferingType       string
	FixedPrice         float64
	UsagePrice         float64
	Duration           int32
}

// CacheEngineVersion represents an engine version.
type CacheEngineVersion struct {
	Engine                        string
	EngineVersion                 string
	CacheParameterGroupFamily     string
	CacheEngineDescription        string
	CacheEngineVersionDescription string
}

// ServiceUpdate represents a service update.
type ServiceUpdate struct {
	ServiceUpdateName string
	Status            string
}

// UpdateAction represents an update action.
type UpdateAction struct {
	ReplicationGroupID string
	CacheClusterID     string
	ServiceUpdateName  string
	UpdateActionStatus string
}

// ServerlessCreateOpts carries all fields for serverless cache creation.
type ServerlessCreateOpts struct {
	Tags                   map[string]string
	CacheUsageLimits       *CacheUsageLimits
	Name                   string
	Description            string
	Engine                 string
	KmsKeyID               string
	UserGroupID            string
	SubnetGroupName        string
	DailySnapshotTime      string
	MajorEngineVersion     string
	NetworkType            string
	SecurityGroupIDs       []string
	SubnetIDs              []string
	SnapshotRetentionLimit int32
}

// ServerlessModifyOpts carries all fields for serverless cache modification.
type ServerlessModifyOpts struct {
	SnapshotRetentionLimit *int32
	CacheUsageLimits       *CacheUsageLimits
	Description            string
	UserGroupID            string
	DailySnapshotTime      string
	SecurityGroupIDs       []string
	RemoveUserGroup        bool
}

// ----------------------------------------
// CreateServerlessCacheFull
// ----------------------------------------
