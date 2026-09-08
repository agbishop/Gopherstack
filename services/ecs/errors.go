package ecs

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrClusterNotFound is returned when a cluster does not exist.
	ErrClusterNotFound = awserr.New("ClusterNotFoundException", awserr.ErrNotFound)
	// ErrServiceNotFound is returned when a service does not exist.
	ErrServiceNotFound = awserr.New("ServiceNotFoundException", awserr.ErrNotFound)
	// ErrResourceNotFound is returned when a generic resource does not exist
	// (e.g. DescribeExpressGatewayService's not-found case: unlike its
	// Delete/Update siblings, which model plain ServiceNotFoundException,
	// this op's own deserializer models ResourceNotFoundException instead).
	ErrResourceNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrInvalidParameter is returned when a required parameter is missing or invalid.
	ErrInvalidParameter = awserr.New("InvalidParameterException", awserr.ErrInvalidParameter)
	// ErrClient is returned when a request is structurally invalid in a way that
	// AWS ECS reports as a ClientException (for example, malformed container
	// definitions or an unsupported network mode / launch-type combination).
	ErrClient = awserr.New("ClientException", awserr.ErrInvalidParameter)
	// ErrCapacityProviderInUse is returned when deleting a capacity provider
	// still associated with a cluster. Uses the ErrAlreadyExists kind (like
	// ErrDaemonAlreadyExists in daemon.go) since handleError only maps
	// ErrNotFound/ErrAlreadyExists/ErrInvalidParameter kinds to 400; the wire
	// code is still the real SDK's ResourceInUseException.
	ErrCapacityProviderInUse = awserr.New("ResourceInUseException", awserr.ErrAlreadyExists)
	// ErrClusterContainsServices is returned by DeleteCluster when the cluster
	// still has services; they must be deleted first.
	ErrClusterContainsServices = awserr.New("ClusterContainsServicesException", awserr.ErrInvalidParameter)
	// ErrClusterContainsTasks is returned by DeleteCluster when the cluster
	// still has active (non-STOPPED) tasks.
	ErrClusterContainsTasks = awserr.New("ClusterContainsTasksException", awserr.ErrInvalidParameter)
	// ErrClusterContainsContainerInstances is returned by DeleteCluster when the
	// cluster still has registered container instances; they must be
	// deregistered first.
	ErrClusterContainsContainerInstances = awserr.New(
		"ClusterContainsContainerInstancesException", awserr.ErrInvalidParameter,
	)
)

// errServiceDeploymentAlreadyStopped is returned by StopServiceDeployment when
// the deployment is already STOPPED. ecs models no "AlreadyStopped" exception
// (absent from ecs@v1.90.0/types/errors.go and from
// awsAwsjson11_deserializeOpErrorStopServiceDeployment's switch) --
// ConflictException ("conflict in the current state of the resource") is the
// code that switch actually models for this condition (gopherstack-101r).
var errServiceDeploymentAlreadyStopped = awserr.New(
	"ConflictException", awserr.ErrInvalidParameter,
)

// errNoLifecycleHook is returned by ContinueServiceDeployment: this backend
// never pauses a deployment at a lifecycle hook (blue/green PAUSE stages
// aren't modeled — every deployment either runs to completion or trips the
// circuit breaker), so there is never a paused hookId to act on. Matching AWS
// behavior for "act on a hook that isn't currently paused" as closely as
// possible without modeling the full lifecycle-hook state machine.
var errNoLifecycleHook = awserr.New("ClientException", awserr.ErrInvalidParameter)
