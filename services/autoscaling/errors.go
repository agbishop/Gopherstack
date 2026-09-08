package autoscaling

import "errors"

var (
	// ErrGroupNotFound is returned when the requested Auto Scaling group does not exist.
	ErrGroupNotFound = errors.New("AutoScalingGroupNotFound")
	// ErrGroupAlreadyExists is returned when an Auto Scaling group with that name already exists.
	ErrGroupAlreadyExists = errors.New("AlreadyExists")
	// ErrLaunchConfigurationNotFound is returned when a launch configuration does not exist.
	ErrLaunchConfigurationNotFound = errors.New("LaunchConfigurationNotFound")
	// ErrLaunchConfigurationAlreadyExists is returned when a launch configuration already exists.
	ErrLaunchConfigurationAlreadyExists = errors.New("AlreadyExists")
	// ErrUnknownAction is returned when the requested action is not recognized.
	ErrUnknownAction = errors.New("InvalidAction")
	// ErrInvalidParameter is returned when a request parameter is invalid.
	ErrInvalidParameter = errors.New("ValidationError")
	// ErrActiveInstanceRefreshNotFound is returned when no active instance refresh exists.
	ErrActiveInstanceRefreshNotFound = errors.New("ActiveInstanceRefreshNotFound")
	// ErrLifecycleHookNotFound is returned when the specified lifecycle hook does not exist.
	ErrLifecycleHookNotFound = errors.New("ValidationError")
	// ErrScalingActivityInProgress is returned when a delete is attempted on a group with active instances
	// and ForceDelete is not set.
	ErrScalingActivityInProgress = errors.New("ScalingActivityInProgress")
	// ErrInstanceNotFound is returned when a specific instance ID is not found in an ASG.
	ErrInstanceNotFound = errors.New("ValidationError")
	// ErrWarmPoolNotFound is returned when no warm pool exists for the group.
	ErrWarmPoolNotFound = errors.New("ValidationError")
	// ErrPolicyNotFound is returned when the specified scaling policy does not exist.
	ErrPolicyNotFound = errors.New("ValidationError")
	// ErrInstanceRefreshInProgress is returned when StartInstanceRefresh is called
	// while another instance refresh is already in progress for the group.
	// Matches the real SDK's InstanceRefreshInProgressFault, whose ErrorCode() is
	// "InstanceRefreshInProgress" (autoscaling@v1.70.4 types/errors.go).
	ErrInstanceRefreshInProgress = errors.New("InstanceRefreshInProgress")
	// ErrDeletionProtected is returned when DeleteAutoScalingGroup is called on a
	// group whose DeletionProtection setting forbids the requested delete.
	// Matches the real SDK's ResourceInUseFault, whose ErrorCode() is "ResourceInUse".
	ErrDeletionProtected = errors.New("ResourceInUse")
	// ErrLaunchConfigurationInUse is returned when DeleteLaunchConfiguration is
	// called on a launch configuration still attached to an Auto Scaling group.
	// api_op_DeleteLaunchConfiguration.go: "The launch configuration must not be
	// attached to an Auto Scaling group." Same wire code as ErrDeletionProtected
	// ("ResourceInUse" -- ResourceInUseFault).
	ErrLaunchConfigurationInUse = errors.New("ResourceInUse")
)
