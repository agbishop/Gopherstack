package efs

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// Package-local sentinels used as the inner error for wrapped error types.
// They are not exported; callers should match via the exported Err* vars.
var (
	errTokenIdentical = errors.New("creation token exists with identical parameters")
	errThrottled      = errors.New("too many requests")
)

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = awserr.New("FileSystemNotFound", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource with the same token already exists but args differ.
	ErrAlreadyExists = awserr.New("FileSystemAlreadyExists", awserr.ErrConflict)
	// ErrCreationTokenExists is returned when the same creation token with identical args is reused.
	ErrCreationTokenExists = awserr.New("FileSystemAlreadyExists", errTokenIdentical)
	// ErrReplicationConfigExists is returned by CreateReplicationConfiguration when a
	// replication configuration already exists for the source file system.
	// CreateReplicationConfiguration's own deserializeOpError (efs@v1.44.4
	// deserializers.go) has no FileSystemAlreadyExists case -- unlike CreateFileSystem,
	// it has ConflictException instead.
	ErrReplicationConfigExists = awserr.New("ConflictException", awserr.ErrConflict)
	// ErrMountTargetNotFound is returned when a requested mount target does not exist.
	ErrMountTargetNotFound = awserr.New("MountTargetNotFound", awserr.ErrNotFound)
	// ErrAccessPointNotFound is returned when a requested access point does not exist.
	ErrAccessPointNotFound = awserr.New("AccessPointNotFound", awserr.ErrNotFound)
	// ErrPolicyNotFound is returned when no resource policy is configured for a file system.
	ErrPolicyNotFound = awserr.New("PolicyNotFound", awserr.ErrNotFound)
	// ErrInvalidPolicy is returned when a file system policy document is malformed or too large.
	ErrInvalidPolicy = awserr.New("InvalidPolicyException", awserr.ErrInvalidParameter)
	// ErrValidation is returned when input validation fails.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrBadRequest is returned for malformed input on ops whose declared error model has
	// BadRequest instead of ValidationException -- efs@v1.44.4 declares ValidationException
	// on only 4 of its 31 ops (CreateReplicationConfiguration, DescribeBackupPolicy,
	// DescribeReplicationConfigurations, PutBackupPolicy); every other op's generic bad-input
	// code is BadRequest ("Returned if the request is malformed or contains an error such as
	// an invalid parameter value or a missing required parameter" -- types/errors.go).
	ErrBadRequest = awserr.New("BadRequest", awserr.ErrInvalidParameter)
	// ErrFileSystemInUse is returned when attempting to delete a file system that has mount targets.
	ErrFileSystemInUse = awserr.New("FileSystemInUse", awserr.ErrConflict)
	// ErrMountTargetConflict is returned when a duplicate mount target is created in the same
	// subnet, or (with an EC2Resolver wired, see crossservice.go) when the requested subnet
	// violates the "one VPC, one mount target per Availability Zone" rule (efs@v1.44.4
	// types/errors.go: "Returned if the mount target would violate one of the specified
	// restrictions based on the file system's existing mount targets").
	ErrMountTargetConflict = awserr.New("MountTargetConflict", awserr.ErrConflict)
	// ErrSubnetNotFound is returned by CreateMountTarget, with an EC2Resolver wired, when
	// SubnetId does not exist (efs@v1.44.4 types/errors.go: "Returned if there is no subnet
	// with ID SubnetId provided in the request").
	ErrSubnetNotFound = awserr.New("SubnetNotFound", awserr.ErrNotFound)
	// ErrIncorrectFileSystemLifeCycleState is returned when an operation requires the
	// file system to be in the "available" lifecycle state (botocore efs/service-2.json:
	// "Returned if the file system's lifecycle state is not \"available\"").
	ErrIncorrectFileSystemLifeCycleState = awserr.New("IncorrectFileSystemLifeCycleState", awserr.ErrConflict)
	// ErrSecurityGroupLimitExceeded is returned when too many security groups are specified.
	ErrSecurityGroupLimitExceeded = awserr.New("SecurityGroupLimitExceeded", awserr.ErrConflict)
	// ErrTooManyRequests is returned when a throughput change cooldown is violated.
	ErrTooManyRequests = awserr.New("TooManyRequests", errThrottled)
)
