package iotwireless

import "errors"

// Sentinel errors for IoT Wireless backend operations.
var (
	// ErrDeviceNotFound is returned when a wireless device does not exist.
	ErrDeviceNotFound = errors.New("ResourceNotFoundException: Wireless device not found")
	// ErrGatewayNotFound is returned when a wireless gateway does not exist.
	ErrGatewayNotFound = errors.New("ResourceNotFoundException: Wireless gateway not found")
	// ErrServiceProfileNotFound is returned when a service profile does not exist.
	ErrServiceProfileNotFound = errors.New("ResourceNotFoundException: Service profile not found")
	// ErrDestinationNotFound is returned when a destination does not exist.
	ErrDestinationNotFound = errors.New("ResourceNotFoundException: Destination not found")
	// ErrDeviceProfileNotFound is returned when a device profile does not exist.
	ErrDeviceProfileNotFound = errors.New("ResourceNotFoundException: Device profile not found")
	// ErrFuotaTaskNotFound is returned when a FUOTA task does not exist.
	ErrFuotaTaskNotFound = errors.New("ResourceNotFoundException: FUOTA task not found")
	// ErrMulticastGroupNotFound is returned when a multicast group does not exist.
	ErrMulticastGroupNotFound = errors.New("ResourceNotFoundException: Multicast group not found")
	// ErrNetworkAnalyzerConfigNotFound is returned when a network analyzer configuration does not exist.
	ErrNetworkAnalyzerConfigNotFound = errors.New("ResourceNotFoundException: Network analyzer configuration not found")
	// ErrValidation is returned when a request contains invalid parameters.
	ErrValidation = errors.New("ValidationException: invalid request parameters")
)

// Sentinel errors for new backend operations.
var (
	// ErrPartnerAccountNotFound is returned when a partner account does not exist.
	ErrPartnerAccountNotFound = errors.New("ResourceNotFoundException: Partner account not found")
	// ErrImportTaskNotFound is returned when a wireless device import task does not exist.
	ErrImportTaskNotFound = errors.New("ResourceNotFoundException: Wireless device import task not found")
	// ErrGatewayTaskNotFound is returned when a wireless gateway task does not exist.
	ErrGatewayTaskNotFound = errors.New(
		"ResourceNotFoundException: Wireless gateway task not found",
	)
	// ErrGatewayTaskDefNotFound is returned when a wireless gateway task definition does not exist.
	ErrGatewayTaskDefNotFound = errors.New(
		"ResourceNotFoundException: Wireless gateway task definition not found",
	)
	// ErrMulticastGroupSessionNotFound is returned when a multicast group has no active session.
	ErrMulticastGroupSessionNotFound = errors.New(
		"ResourceNotFoundException: Multicast group session not found",
	)
)

// Sentinel errors for delete preconditions.
var (
	// ErrDeviceProfileInUse is returned when a device profile is still
	// referenced by a wireless device.
	ErrDeviceProfileInUse = errors.New("ConflictException: Device profile is in use")
	// ErrServiceProfileInUse is returned when a service profile is still
	// referenced by a wireless device.
	ErrServiceProfileInUse = errors.New("ConflictException: Service profile is in use")
	// ErrDestinationInUse is returned when a destination is still referenced
	// by a wireless device.
	ErrDestinationInUse = errors.New("ConflictException: Destination is in use")
	// ErrMulticastGroupInUse is returned when a multicast group is still
	// associated with a FUOTA task (api_op_DeleteMulticastGroup.go: "Deletes
	// a multicast group if it is not in use by a FUOTA task").
	ErrMulticastGroupInUse = errors.New("ConflictException: Multicast group is in use by a FUOTA task")
)
