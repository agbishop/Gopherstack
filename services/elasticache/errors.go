package elasticache

import "errors"

var (
	ErrClusterNotFound                    = errors.New("CacheClusterNotFound")
	ErrClusterAlreadyExists               = errors.New("CacheClusterAlreadyExists")
	ErrReplicationGroupNotFound           = errors.New("ReplicationGroupNotFound")
	ErrReplicationGroupAlreadyExists      = errors.New("ReplicationGroupAlreadyExists")
	ErrResourceNotFound                   = errors.New("resource not found")
	ErrParameterGroupNotFound             = errors.New("CacheParameterGroupNotFound")
	ErrParameterGroupAlreadyExists        = errors.New("CacheParameterGroupAlreadyExists")
	ErrParameterGroupDefaultNotModifiable = errors.New("default parameter group cannot be deleted or modified")
	ErrInvalidParameterGroupFamily        = errors.New("InvalidParameterGroupFamily")
	ErrSubnetGroupNotFound                = errors.New("CacheSubnetGroupNotFound")
	ErrSubnetGroupAlreadyExists           = errors.New("CacheSubnetGroupAlreadyExists")
	ErrSubnetGroupInUse                   = errors.New("CacheSubnetGroupInUse")
	ErrSnapshotNotFound                   = errors.New("SnapshotNotFound")
	ErrSnapshotAlreadyExists              = errors.New("SnapshotAlreadyExistsFault")
	ErrInvalidSnapshotSource              = errors.New(
		"exactly one of CacheClusterId or ReplicationGroupId must be specified",
	)
)

var (
	ErrClusterModeRequired          = errors.New("cluster mode must be enabled for shard configuration changes")
	ErrDataTieringInvalid           = errors.New("data tiering requires r7g node type and Redis/Valkey 7.0+")
	ErrTransitEncryptionModeInvalid = errors.New("transit encryption mode 'required' requires an auth token")
	ErrAuthTokenRequiredForMode     = errors.New(
		"auth token must be provided when transit encryption mode is 'required'",
	)
	// ErrApplyImmediatelyRequired matches AWS's own documented constraint on
	// IncreaseReplicaCount/DecreaseReplicaCount/ModifyReplicationGroupShardConfiguration/
	// IncreaseNodeGroupsInGlobalReplicationGroup/DecreaseNodeGroupsInGlobalReplicationGroup:
	// ApplyImmediately is required, and false is documented as "not currently
	// supported" / "the only permitted value for this parameter is true".
	ErrApplyImmediatelyRequired = errors.New("ApplyImmediately=false is not currently supported for this operation")
	// ErrCustomerNodeEndpointsRequired guards StartMigration/TestMigration's
	// required CustomerNodeEndpointList member.
	ErrCustomerNodeEndpointsRequired = errors.New("CustomerNodeEndpointList must contain at least one endpoint")
)

// State-transition guard sentinels: a resource must be "available" before it
// can accept a new Modify/Delete/failover-style mutation. AWS models these as
// Invalid<Resource>State(Fault) on every mutating op (verified against
// aws-sdk-go-v2's per-operation error deserializers), returned e.g. while a
// resource is still "creating", "modifying", or "deleting" from a prior call.
var (
	ErrClusterNotAvailable                = errors.New("cache cluster is not in the available state")
	ErrReplicationGroupNotAvailable       = errors.New("replication group is not in the available state")
	ErrServerlessCacheNotAvailable        = errors.New("serverless cache is not in the available state")
	ErrGlobalReplicationGroupNotAvailable = errors.New("global replication group is not in the available state")
	ErrClusterInReplicationGroup          = errors.New(
		"cannot delete a cache cluster that is the last member of a replication group",
	)
)

// ----------------------------------------
// New model types (gaps #1–#15)
// ----------------------------------------

var (
	ErrUserGroupNotFound                  = errors.New("UserGroupNotFound")
	ErrUserGroupAlreadyExists             = errors.New("UserGroupAlreadyExistsFault")
	ErrReservedCacheNodeNotFound          = errors.New("ReservedCacheNodeNotFound")
	ErrReservedCacheNodeAlreadyExists     = errors.New("ReservedCacheNodeAlreadyExists")
	ErrReservedCacheNodesOfferingNotFound = errors.New("ReservedCacheNodesOfferingNotFound")
)

// Quota-exceeded sentinels: AWS's documented default per-Region/per-resource
// quotas (docs.aws.amazon.com/AmazonElastiCache/latest/dg/quota-limits.html),
// enforced with the matching Fault the real API recognizes for each op
// (verified against aws-sdk-go-v2/service/elasticache@v1.56.4/
// deserializers.go's per-operation error-deserializer switch). See
// maxCacheSubnetGroupsPerRegion/maxSubnetsPerCacheSubnetGroup/
// maxServerlessCachesPerRegion for the specific published values.
var (
	ErrCacheSubnetGroupQuotaExceeded = errors.New("CacheSubnetGroupQuotaExceeded")
	ErrCacheSubnetQuotaExceeded      = errors.New("CacheSubnetQuotaExceededFault")
	ErrServerlessCacheQuotaExceeded  = errors.New("ServerlessCacheQuotaForCustomerExceededFault")
)

var (
	ErrGroupUserNotFound = errors.New("one or more specified user IDs do not exist")
)

// ----------------------------------------
// ServerlessCreateOpts holds all fields for full serverless cache creation.
// ----------------------------------------

var (
	ErrCacheSecurityGroupNotFound      = errors.New("CacheSecurityGroupNotFound")
	ErrCacheSecurityGroupAlreadyExists = errors.New("CacheSecurityGroupAlreadyExists")
	ErrGlobalReplicationGroupNotFound  = errors.New("GlobalReplicationGroupNotFound")
	ErrGlobalReplicationGroupExists    = errors.New("GlobalReplicationGroupAlreadyExistsFault")
	ErrServerlessCacheNotFound         = errors.New("ServerlessCacheNotFound")
	ErrServerlessCacheAlreadyExists    = errors.New("ServerlessCacheAlreadyExistsFault")
	ErrServerlessCacheSnapshotNotFound = errors.New("ServerlessCacheSnapshotNotFoundFault")
	ErrServerlessCacheSnapshotExists   = errors.New("ServerlessCacheSnapshotAlreadyExistsFault")
	ErrUserNotFound                    = errors.New("UserNotFound")
	ErrUserAlreadyExists               = errors.New("UserAlreadyExists")
)

// ----------------------------------------
// New model types
// ----------------------------------------
