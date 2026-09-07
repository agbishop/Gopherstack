package rds

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrInstanceNotFound is returned when an RDS instance does not exist.
	ErrInstanceNotFound = awserr.New("DBInstanceNotFound", awserr.ErrNotFound)
	// ErrInstanceAlreadyExists is returned when an RDS instance already exists.
	ErrInstanceAlreadyExists = awserr.New("DBInstanceAlreadyExists", awserr.ErrAlreadyExists)
	// ErrSnapshotNotFound is returned when a snapshot does not exist.
	ErrSnapshotNotFound = awserr.New("DBSnapshotNotFound", awserr.ErrNotFound)
	// ErrSnapshotAlreadyExists is returned when a snapshot already exists.
	ErrSnapshotAlreadyExists = awserr.New("DBSnapshotAlreadyExists", awserr.ErrAlreadyExists)
	// ErrSubnetGroupNotFound is returned when a subnet group does not exist.
	ErrSubnetGroupNotFound = awserr.New("DBSubnetGroupNotFound", awserr.ErrNotFound)
	// ErrSubnetGroupAlreadyExists is returned when a subnet group already exists.
	ErrSubnetGroupAlreadyExists = awserr.New("DBSubnetGroupAlreadyExists", awserr.ErrAlreadyExists)
	// ErrSubnetGroupInUse is returned when a subnet group is still associated with a DB instance.
	ErrSubnetGroupInUse = awserr.New("InvalidDBSubnetGroupStateFault", awserr.ErrConflict)
	// ErrInvalidParameter is returned for invalid input.
	ErrInvalidParameter = awserr.New("InvalidParameterValue", awserr.ErrInvalidParameter)
	// ErrInvalidParameterCombination is returned when a set of otherwise-valid
	// parameters cannot be used together (e.g. MonitoringInterval>0 without a
	// MonitoringRoleArn). AWS returns the InvalidParameterCombination error code.
	ErrInvalidParameterCombination = awserr.New("InvalidParameterCombination", awserr.ErrInvalidParameter)
	// ErrUnknownAction is returned for unrecognized RDS actions.
	ErrUnknownAction = awserr.New("InvalidAction", awserr.ErrInvalidParameter)
	// ErrInvalidDBInstanceState is returned when an instance operation is invalid given its current state.
	ErrInvalidDBInstanceState = awserr.New("InvalidDBInstanceState", awserr.ErrConflict)
	// ErrResourceNotFound is the generic not-found error for operations whose
	// ResourceArn/ResourceIdentifier can name more than one resource type
	// (verified against aws-sdk-go-v2/service/rds@v1.124.1's deserializers.go
	// declared error sets for ApplyPendingMaintenanceAction, EnableHttpEndpoint,
	// and DisableHttpEndpoint, none of which declare a resource-type-specific
	// not-found code).
	ErrResourceNotFound = awserr.New("ResourceNotFoundFault", awserr.ErrNotFound)

	// ErrParameterGroupNotFound is returned when a DB parameter group does not exist.
	ErrParameterGroupNotFound = awserr.New("DBParameterGroupNotFound", awserr.ErrNotFound)
	// ErrParameterGroupAlreadyExists is returned when a DB parameter group already exists.
	ErrParameterGroupAlreadyExists = awserr.New("DBParameterGroupAlreadyExists", awserr.ErrAlreadyExists)
	// ErrOptionGroupNotFound is returned when an option group does not exist.
	ErrOptionGroupNotFound = awserr.New("OptionGroupNotFound", awserr.ErrNotFound)
	// ErrOptionGroupAlreadyExists is returned when an option group already exists.
	ErrOptionGroupAlreadyExists = awserr.New("OptionGroupAlreadyExists", awserr.ErrAlreadyExists)
	// ErrClusterNotFound is returned when a DB cluster does not exist.
	ErrClusterNotFound = awserr.New("DBClusterNotFound", awserr.ErrNotFound)
	// ErrClusterAlreadyExists is returned when a DB cluster already exists.
	ErrClusterAlreadyExists = awserr.New("DBClusterAlreadyExists", awserr.ErrAlreadyExists)
	// ErrClusterSnapshotNotFound is returned when a DB cluster snapshot does not exist.
	ErrClusterSnapshotNotFound = awserr.New("DBClusterSnapshotNotFound", awserr.ErrNotFound)
	// ErrClusterSnapshotAlreadyExists is returned when a DB cluster snapshot already exists.
	ErrClusterSnapshotAlreadyExists = awserr.New("DBClusterSnapshotAlreadyExists", awserr.ErrAlreadyExists)
	// ErrClusterEndpointNotFound is returned when a DB cluster endpoint does not exist.
	ErrClusterEndpointNotFound = awserr.New("DBClusterEndpointNotFound", awserr.ErrNotFound)
	// ErrClusterEndpointAlreadyExists is returned when a DB cluster endpoint already exists.
	ErrClusterEndpointAlreadyExists = awserr.New("DBClusterEndpointAlreadyExists", awserr.ErrAlreadyExists)
	// ErrExportTaskNotFound is returned when an export task does not exist.
	ErrExportTaskNotFound = awserr.New("ExportTaskNotFound", awserr.ErrNotFound)
	// ErrExportTaskAlreadyExists is returned when an export task already exists.
	ErrExportTaskAlreadyExists = awserr.New("ExportTaskAlreadyExists", awserr.ErrAlreadyExists)
	// ErrGlobalClusterNotFound is returned when a global cluster does not exist.
	ErrGlobalClusterNotFound = awserr.New("GlobalClusterNotFound", awserr.ErrNotFound)
	// ErrGlobalClusterAlreadyExists is returned when a global cluster already exists.
	ErrGlobalClusterAlreadyExists = awserr.New("GlobalClusterAlreadyExists", awserr.ErrAlreadyExists)
	// ErrInvalidDBClusterStateFault is returned when a cluster operation is invalid given its current state.
	ErrInvalidDBClusterStateFault = awserr.New("InvalidDBClusterStateFault", awserr.ErrConflict)
	// ErrInvalidGlobalClusterState is returned when a global cluster operation is invalid given its current state.
	ErrInvalidGlobalClusterState = awserr.New("InvalidGlobalClusterStateFault", awserr.ErrConflict)
	// ErrEventSubscriptionNotFound is returned when an event subscription does not exist.
	ErrEventSubscriptionNotFound = awserr.New("SubscriptionNotFound", awserr.ErrNotFound)
	// ErrEventSubscriptionAlreadyExists is returned when an event subscription already exists.
	ErrEventSubscriptionAlreadyExists = awserr.New("SubscriptionAlreadyExist", awserr.ErrAlreadyExists)
	// ErrDBSecurityGroupNotFound is returned when a DB security group does not exist.
	ErrDBSecurityGroupNotFound = awserr.New("DBSecurityGroupNotFound", awserr.ErrNotFound)
	// ErrDBSecurityGroupAlreadyExists is returned when a DB security group already exists.
	ErrDBSecurityGroupAlreadyExists = awserr.New("DBSecurityGroupAlreadyExists", awserr.ErrAlreadyExists)
	// ErrDBSecurityGroupInvalidState is returned by DeleteDBSecurityGroup for
	// the default DB security group. Real AWS: DeleteDBSecurityGroupInput.
	// DBSecurityGroupName's own doc comment, "You can't delete the default
	// DB security group" ("Must not be \"Default\"").
	ErrDBSecurityGroupInvalidState = awserr.New("InvalidDBSecurityGroupState", awserr.ErrConflict)
	// ErrBlueGreenDeploymentNotFound is returned when a Blue/Green Deployment does not exist.
	ErrBlueGreenDeploymentNotFound = awserr.New("BlueGreenDeploymentNotFound", awserr.ErrNotFound)
	// ErrBlueGreenDeploymentAlreadyExists is returned when a Blue/Green Deployment already exists.
	ErrBlueGreenDeploymentAlreadyExists = awserr.New("BlueGreenDeploymentAlreadyExists", awserr.ErrAlreadyExists)
	// ErrNoServerlessV2Config is a sentinel indicating no ServerlessV2ScalingConfiguration was provided.
	ErrNoServerlessV2Config = errors.New("noServerlessV2Config")

	// ErrDBShardGroupNotFound is returned when a DB shard group does not exist.
	ErrDBShardGroupNotFound = awserr.New("DBShardGroupNotFound", awserr.ErrNotFound)
	// ErrDBShardGroupAlreadyExists is returned when a DB shard group already exists.
	ErrDBShardGroupAlreadyExists = awserr.New("DBShardGroupAlreadyExists", awserr.ErrAlreadyExists)

	// ErrIntegrationNotFound is returned when an integration does not exist.
	ErrIntegrationNotFound = awserr.New("IntegrationNotFound", awserr.ErrNotFound)
	// ErrIntegrationAlreadyExists is returned when an integration already exists.
	ErrIntegrationAlreadyExists = awserr.New("IntegrationAlreadyExists", awserr.ErrAlreadyExists)

	// ErrTenantDatabaseNotFound is returned when a tenant database does not exist.
	ErrTenantDatabaseNotFound = awserr.New("TenantDatabaseNotFound", awserr.ErrNotFound)
	// ErrTenantDatabaseAlreadyExists is returned when a tenant database already exists.
	ErrTenantDatabaseAlreadyExists = awserr.New("TenantDatabaseAlreadyExists", awserr.ErrAlreadyExists)

	// ErrCustomDBEngineVersionNotFound is returned when a custom DB engine version does not exist.
	ErrCustomDBEngineVersionNotFound = awserr.New("CustomDBEngineVersionNotFoundFault", awserr.ErrNotFound)
	// ErrCustomDBEngineVersionAlreadyExists is returned when a custom DB engine version already exists.
	ErrCustomDBEngineVersionAlreadyExists = awserr.New(
		"CustomDBEngineVersionAlreadyExistsFault",
		awserr.ErrAlreadyExists,
	)

	// ErrDBClusterAutomatedBackupNotFound is returned when a cluster automated backup does not exist.
	ErrDBClusterAutomatedBackupNotFound = awserr.New("DBClusterAutomatedBackupNotFound", awserr.ErrNotFound)
	// ErrDBInstanceAutomatedBackupNotFound is returned when an instance automated backup does not exist.
	ErrDBInstanceAutomatedBackupNotFound = awserr.New("DBInstanceAutomatedBackupNotFound", awserr.ErrNotFound)
)

var (
	// ErrDBProxyAlreadyExists is returned when a DB proxy with the same name already exists.
	ErrDBProxyAlreadyExists             = awserr.New("DBProxyAlreadyExists", awserr.ErrAlreadyExists)
	ErrDBProxyEndpointAlreadyExists     = awserr.New("DBProxyEndpointAlreadyExists", awserr.ErrAlreadyExists)
	ErrCannotDeleteDefaultProxyEndpoint = awserr.New("InvalidDBProxyEndpointStateFault", awserr.ErrConflict)
	ErrActivityStreamAlreadyStarted     = awserr.New("InvalidDBClusterStateFault", awserr.ErrConflict)
	ErrActivityStreamNotStarted         = awserr.New("InvalidDBClusterStateFault", awserr.ErrConflict)
)

// ErrRebootFailed is returned when one or more FIS reboot-instance actions fail.
var ErrRebootFailed = errors.New("aws:rds:reboot-db-instances failed")
