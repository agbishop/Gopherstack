package awsconfig

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrNotFound is returned when a configuration recorder is not found.
	ErrNotFound = awserr.New("NoSuchConfigurationRecorder", awserr.ErrNotFound)
	// ErrNoSuchDeliveryChannel is returned when a delivery channel is not found.
	ErrNoSuchDeliveryChannel = awserr.New("NoSuchDeliveryChannelException", awserr.ErrNotFound)
	// ErrNoSuchConfigRule is returned when a config rule is not found.
	ErrNoSuchConfigRule = awserr.New("NoSuchConfigRuleException", awserr.ErrNotFound)
	// ErrNoSuchAggregator is returned when a configuration aggregator is not found.
	ErrNoSuchAggregator = awserr.New("NoSuchConfigurationAggregatorException", awserr.ErrNotFound)
	// ErrNoSuchConformancePack is returned when a conformance pack is not found.
	ErrNoSuchConformancePack = awserr.New("NoSuchConformancePackException", awserr.ErrNotFound)
	// ErrNoSuchOrganizationConfigRule is returned when an organization config rule is not found.
	ErrNoSuchOrganizationConfigRule = awserr.New("NoSuchOrganizationConfigRuleException", awserr.ErrNotFound)
	// ErrNoSuchOrganizationConformancePack is returned when an org conformance pack is not found.
	// The wire error type is NoSuchOrganizationConformancePackException (verified against
	// aws-sdk-go-v2/service/configservice's DeleteOrganizationConformancePack deserializer).
	ErrNoSuchOrganizationConformancePack = awserr.New(
		"NoSuchOrganizationConformancePackException",
		awserr.ErrNotFound,
	)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New("MaxNumberOfConfigurationRecordersExceededException", awserr.ErrAlreadyExists)
	// ErrNoDeliveryChannel is returned when starting a recorder with no delivery channel configured.
	ErrNoDeliveryChannel = awserr.New("NoAvailableDeliveryChannelException", awserr.ErrInvalidParameter)
	// ErrValidation is returned when a required field is missing or invalid.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrInvalidParameterValue is returned for a missing/invalid required field
	// on operations whose declared error model has no ValidationException --
	// e.g. PutRemediationExceptions (verified against aws-sdk-go-v2/service/
	// configservice's awsAwsjson11_deserializeOpErrorPutRemediationExceptions,
	// which declares InsufficientPermissionsException/
	// InvalidParameterValueException only).
	ErrInvalidParameterValue = awserr.New("InvalidParameterValueException", awserr.ErrInvalidParameter)
	// ErrInvalidNextToken is returned for a malformed pagination token on an op whose
	// declared error model has InvalidNextTokenException instead of ValidationException --
	// e.g. DescribeConfigRules (verified against aws-sdk-go-v2/service/configservice's
	// awsAwsjson11_deserializeOpErrorDescribeConfigRules, which declares
	// InvalidNextTokenException/InvalidParameterValueException, never ValidationException).
	ErrInvalidNextToken = awserr.New("InvalidNextTokenException", awserr.ErrInvalidParameter)
	// ErrResourceNotFound is returned when a referenced resource evaluation does not exist.
	ErrResourceNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrResourceNotDiscovered is returned when GetAggregateResourceConfig's
	// ResourceIdentifier matches no discovered resource (verified against
	// aws-sdk-go-v2/service/configservice's GetAggregateResourceConfig
	// deserializer, which declares ResourceNotDiscoveredException).
	ErrResourceNotDiscovered = awserr.New("ResourceNotDiscoveredException", awserr.ErrNotFound)
	// ErrNoSuchConfigRuleInConformancePack is returned when a conformance pack
	// filter/lookup references a config rule name that the pack did not deploy
	// (verified against aws-sdk-go-v2/service/configservice's
	// DescribeConformancePackCompliance/GetConformancePackComplianceDetails
	// deserializers).
	ErrNoSuchConfigRuleInConformancePack = awserr.New(
		"NoSuchConfigRuleInConformancePackException",
		awserr.ErrNotFound,
	)
	// ErrNoSuchRemediationConfiguration is returned when a remediation-execution op
	// targets a config rule with no remediation configuration (verified against
	// aws-sdk-go-v2/service/configservice's StartRemediationExecution/
	// DescribeRemediationExecutionStatus deserializers).
	ErrNoSuchRemediationConfiguration = awserr.New("NoSuchRemediationConfigurationException", awserr.ErrNotFound)
	// ErrNoAvailableConfigurationRecorder is returned when an op that needs a
	// configuration recorder (e.g. DeliverConfigSnapshot) finds none configured.
	ErrNoAvailableConfigurationRecorder = awserr.New(
		"NoAvailableConfigurationRecorderException",
		awserr.ErrInvalidParameter,
	)
	// ErrNoRunningConfigurationRecorder is returned when an op that needs an active
	// configuration recorder (e.g. DeliverConfigSnapshot) finds recorders configured
	// but none running.
	ErrNoRunningConfigurationRecorder = awserr.New(
		"NoRunningConfigurationRecorderException",
		awserr.ErrInvalidParameter,
	)
	// ErrInvalidConfigurationRecorderName is returned when a configuration recorder
	// name fails validation (verified against aws-sdk-go-v2/service/configservice's
	// PutConfigurationRecorder deserializer, which declares
	// InvalidConfigurationRecorderNameException).
	ErrInvalidConfigurationRecorderName = awserr.New(
		"InvalidConfigurationRecorderNameException",
		awserr.ErrInvalidParameter,
	)
	// ErrInvalidRole is returned when a configuration recorder's IAM role ARN fails
	// validation (verified against aws-sdk-go-v2/service/configservice's
	// PutConfigurationRecorder deserializer, which declares InvalidRoleException).
	ErrInvalidRole = awserr.New("InvalidRoleException", awserr.ErrInvalidParameter)
	// ErrInvalidDeliveryChannelName is returned when a delivery channel name fails
	// validation (verified against aws-sdk-go-v2/service/configservice's
	// PutDeliveryChannel deserializer, which declares
	// InvalidDeliveryChannelNameException).
	ErrInvalidDeliveryChannelName = awserr.New("InvalidDeliveryChannelNameException", awserr.ErrInvalidParameter)
	// ErrConflict is returned when a connector or third-party service-linked
	// recorder request conflicts with existing state: PutConnector with a
	// ConnectorConfiguration matching an already-existing connector, or
	// PutThirdPartyServiceLinkedConfigurationRecorder for a ServicePrincipal
	// that already owns a service-linked recorder tied to a different
	// connector (verified against the AWS Config API reference's PutConnector/
	// PutThirdPartyServiceLinkedConfigurationRecorder error lists, which
	// declare ConflictException at HTTP status 400 -- not 409, unlike this
	// package's other conflict-shaped errors).
	ErrConflict = awserr.New("ConflictException", awserr.ErrConflict)
	// ErrLastDeliveryChannelDeleteFailed is returned by DeleteDeliveryChannel
	// when the customer managed configuration recorder is still recording.
	// Real AWS: "Before you can delete the delivery channel, you must stop
	// the customer managed configuration recorder" (verified against
	// aws-sdk-go-v2/service/configservice's DeleteDeliveryChannel
	// deserializer, which declares LastDeliveryChannelDeleteFailedException).
	ErrLastDeliveryChannelDeleteFailed = awserr.New(
		"LastDeliveryChannelDeleteFailedException",
		awserr.ErrConflict,
	)
)
