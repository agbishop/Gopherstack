package memorydb

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// ErrValidation is returned when input validation fails.
var ErrValidation = awserr.New("InvalidParameterValueException", awserr.ErrInvalidParameter)

// Errors used by the backend.
var (
	// ErrClusterNotFound is returned when a cluster does not exist.
	ErrClusterNotFound = awserr.New("ClusterNotFoundFault: cluster not found", awserr.ErrNotFound)
	// ErrClusterAlreadyExists is returned when a cluster already exists.
	ErrClusterAlreadyExists = awserr.New("ClusterAlreadyExistsFault: cluster already exists", awserr.ErrAlreadyExists)
	// ErrACLNotFound is returned when an ACL does not exist.
	ErrACLNotFound = awserr.New("ACLNotFoundFault: ACL not found", awserr.ErrNotFound)
	// ErrACLAlreadyExists is returned when an ACL already exists.
	ErrACLAlreadyExists = awserr.New("ACLAlreadyExistsFault: ACL already exists", awserr.ErrAlreadyExists)
	// ErrSubnetGroupNotFound is returned when a subnet group does not exist.
	ErrSubnetGroupNotFound = awserr.New("SubnetGroupNotFoundFault: subnet group not found", awserr.ErrNotFound)
	// ErrSubnetGroupAlreadyExists is returned when a subnet group already exists.
	ErrSubnetGroupAlreadyExists = awserr.New(
		"SubnetGroupAlreadyExistsFault: subnet group already exists",
		awserr.ErrAlreadyExists,
	)
	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = awserr.New("UserNotFoundFault: user not found", awserr.ErrNotFound)
	// ErrUserAlreadyExists is returned when a user already exists.
	ErrUserAlreadyExists = awserr.New("UserAlreadyExistsFault: user already exists", awserr.ErrAlreadyExists)
	// ErrParameterGroupNotFound is returned when a parameter group does not exist.
	ErrParameterGroupNotFound = awserr.New("ParameterGroupNotFoundFault: parameter group not found", awserr.ErrNotFound)
	// ErrParameterGroupAlreadyExists is returned when a parameter group already exists.
	ErrParameterGroupAlreadyExists = awserr.New(
		"ParameterGroupAlreadyExistsFault: parameter group already exists",
		awserr.ErrAlreadyExists,
	)
	// ErrSnapshotNotFound is returned when a snapshot does not exist.
	ErrSnapshotNotFound = awserr.New("SnapshotNotFoundFault: snapshot not found", awserr.ErrNotFound)
	// ErrSnapshotAlreadyExists is returned when a snapshot already exists.
	ErrSnapshotAlreadyExists = awserr.New(
		"SnapshotAlreadyExistsFault: snapshot already exists",
		awserr.ErrAlreadyExists,
	)
	// ErrMultiRegionClusterNotFound is returned when a multi-region cluster does not exist.
	ErrMultiRegionClusterNotFound = awserr.New(
		"MultiRegionClusterNotFoundFault: multi-region cluster not found",
		awserr.ErrNotFound,
	)
	// ErrMultiRegionClusterAlreadyExists is returned when a multi-region cluster already exists.
	ErrMultiRegionClusterAlreadyExists = awserr.New(
		"MultiRegionClusterAlreadyExistsFault: multi-region cluster already exists",
		awserr.ErrAlreadyExists,
	)
	// ErrMultiRegionParameterGroupNotFound is returned when a multi-region parameter group does not exist.
	ErrMultiRegionParameterGroupNotFound = awserr.New(
		"MultiRegionParameterGroupNotFoundFault: multi-region parameter group not found",
		awserr.ErrNotFound,
	)
	// ErrSubnetGroupInUse is returned when a subnet group cannot be deleted because it
	// is assigned to a cluster (real AWS fault: SubnetGroupInUseFault).
	ErrSubnetGroupInUse = awserr.New(
		"SubnetGroupInUseFault: subnet group is currently associated with a cluster",
		awserr.ErrConflict,
	)
	// ErrACLInUse is returned when an ACL cannot be deleted because it is assigned to a
	// cluster (real AWS fault: InvalidACLStateFault).
	ErrACLInUse = awserr.New(
		"InvalidACLStateFault: ACL is currently associated with a cluster",
		awserr.ErrConflict,
	)
	// ErrParameterGroupInUse is returned when a parameter group cannot be deleted
	// because it is assigned to a cluster (real AWS fault: InvalidParameterGroupStateFault).
	ErrParameterGroupInUse = awserr.New(
		"InvalidParameterGroupStateFault: parameter group is currently associated with a cluster",
		awserr.ErrConflict,
	)
	// ErrInvalidARN is returned by the tagging operations (ListTags/TagResource/
	// UntagResource) when the given ResourceArn does not match any known
	// resource (real AWS fault: InvalidARNFault -- the only NotFound-family
	// fault those three operations' models actually define for an unmatched ARN).
	ErrInvalidARN = awserr.New("InvalidARNFault: resource not found for the given ARN", awserr.ErrNotFound)
	// ErrReservationAlreadyExists is returned when a reserved node with the same ID already exists.
	ErrReservationAlreadyExists = awserr.New(
		"ReservedNodeAlreadyExistsFault: reserved node already exists",
		awserr.ErrAlreadyExists,
	)
	// ErrServiceUpdateNotFound is returned when BatchUpdateCluster names a service
	// update that doesn't exist (real AWS fault: ServiceUpdateNotFoundFault, confirmed
	// in botocore's memorydb/2021-01-01/service-2.json BatchUpdateCluster.errors).
	ErrServiceUpdateNotFound = awserr.New(
		"ServiceUpdateNotFoundFault: service update not found",
		awserr.ErrNotFound,
	)
)
