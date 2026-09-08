package emr

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var ErrValidation = awserr.New(
	"ValidationException: required field is missing",
	awserr.ErrInvalidParameter,
)

var (
	ErrNotFound      = awserr.New("ClientException", awserr.ErrNotFound)
	ErrAlreadyExists = awserr.New("ClientException", awserr.ErrAlreadyExists)
)

var errTerminationProtected = awserr.New(
	"ValidationException: cluster has termination protection enabled",
	awserr.ErrInvalidParameter,
)

// errSessionClusterNotReady is returned by StartSession when the target
// cluster is not in a state that can host a session (real StartSession's
// doc: "The cluster must be in the RUNNING or WAITING state").
var errSessionClusterNotReady = awserr.New(
	"ValidationException: cluster is not in a state that can host a session",
	awserr.ErrInvalidParameter,
)

// errSessionsNotEnabled is returned by StartSession when the target cluster
// was not launched with SessionEnabled=true (the other half of real
// StartSession's precondition alongside errSessionClusterNotReady).
var errSessionsNotEnabled = awserr.New(
	"ValidationException: cluster does not have sessions enabled",
	awserr.ErrInvalidParameter,
)

// errClusterNotAcceptingSteps is returned by AddJobFlowSteps when the target
// cluster is not in a state that accepts new steps (real AddJobFlowSteps'
// doc: "You can only add steps to a cluster that is in one of the following
// states: STARTING, BOOTSTRAPPING, RUNNING, or WAITING").
var errClusterNotAcceptingSteps = awserr.New(
	"ValidationException: cluster is not in a state that accepts new steps",
	awserr.ErrInvalidParameter,
)
