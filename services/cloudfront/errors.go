package cloudfront

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// codeEntityNotFound is the generic not-found code several unrelated CloudFront operations
// declare in place of a dedicated per-resource-type code (KVS, resource policies,
// ListDomainConflicts's DomainControlValidationResource).
const codeEntityNotFound = "EntityNotFound"

var (
	// ErrNotFound is returned when a requested distribution does not exist.
	ErrNotFound = awserr.New("NoSuchDistribution", awserr.ErrNotFound)
	// ErrDistributionNotDisabled is returned when attempting to delete an enabled distribution.
	ErrDistributionNotDisabled = awserr.New("DistributionNotDisabled", awserr.ErrConflict)
	// ErrOAINotFound is returned when a requested OAI does not exist.
	ErrOAINotFound = awserr.New("NoSuchCloudFrontOriginAccessIdentity", awserr.ErrNotFound)
	// ErrCachePolicyNotFound is returned when a requested cache policy does not exist.
	ErrCachePolicyNotFound = awserr.New("NoSuchCachePolicy", awserr.ErrNotFound)
	// ErrAnycastIPListNotFound is returned when a requested anycast IP list does not exist.
	// Code is EntityNotFound -- see ErrConnectionFunctionNotFound; "NoSuchAnycastIPList"
	// does not exist anywhere in the pinned SDK (Get/Update/DeleteAnycastIpList all model
	// EntityNotFound, cloudfront@v1.67.4 deserializers.go).
	ErrAnycastIPListNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)
	// ErrConnectionFunctionNotFound is returned when a connection function does not exist.
	// Code is EntityNotFound: every connection-function op's own deserializer
	// (cloudfront@v1.67.4 deserializers.go) models EntityNotFound, never a
	// dedicated "NoSuchConnectionFunction" -- that code does not exist
	// anywhere in the pinned SDK.
	ErrConnectionFunctionNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)
	// ErrConnectionGroupNotFound is returned when a connection group does not exist.
	// Code is EntityNotFound -- see ErrConnectionFunctionNotFound; "NoSuchConnectionGroup"
	// does not exist anywhere in the pinned SDK either.
	ErrConnectionGroupNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)
	// ErrConnectionGroupAlreadyExists is returned when a connection group name is already in use.
	ErrConnectionGroupAlreadyExists = awserr.New("EntityAlreadyExists", awserr.ErrAlreadyExists)
	// ErrContinuousDeploymentPolicyNotFound is returned when a continuous deployment policy does not exist.
	ErrContinuousDeploymentPolicyNotFound = awserr.New(
		"NoSuchContinuousDeploymentPolicy",
		awserr.ErrNotFound,
	)
	// ErrInvalidationNotFound is returned when a requested invalidation does not exist.
	ErrInvalidationNotFound = awserr.New("NoSuchInvalidation", awserr.ErrNotFound)
	// ErrOACNotFound is returned when a requested origin access control does not exist.
	ErrOACNotFound = awserr.New("NoSuchOriginAccessControl", awserr.ErrNotFound)
	// ErrResponseHeadersPolicyNotFound is returned when a requested response headers policy does not exist.
	ErrResponseHeadersPolicyNotFound = awserr.New("NoSuchResponseHeadersPolicy", awserr.ErrNotFound)
	// ErrFunctionNotFound is returned when a requested CloudFront function does not exist.
	ErrFunctionNotFound = awserr.New("NoSuchFunctionExists", awserr.ErrNotFound)
	// ErrOriginRequestPolicyNotFound is returned when a requested origin request policy does not exist.
	ErrOriginRequestPolicyNotFound = awserr.New("NoSuchOriginRequestPolicy", awserr.ErrNotFound)
	// ErrValidation is returned when request parameters fail validation.
	ErrValidation = awserr.New("InvalidArgument", awserr.ErrInvalidParameter)
	// ErrAlreadyExists is the generic fallback for a resource whose identifier already
	// exists but which has no dedicated AlreadyExists error type in the CloudFront API
	// (e.g. Anycast IP lists, key value stores). AWS itself falls back to this same
	// generic code for such resources.
	ErrAlreadyExists = awserr.New("EntityAlreadyExists", awserr.ErrAlreadyExists)
	// ErrCachePolicyAlreadyExists is returned when a cache policy name is already in use.
	ErrCachePolicyAlreadyExists = awserr.New("CachePolicyAlreadyExists", awserr.ErrAlreadyExists)
	// ErrOriginRequestPolicyAlreadyExists is returned when an origin request policy name
	// is already in use.
	ErrOriginRequestPolicyAlreadyExists = awserr.New("OriginRequestPolicyAlreadyExists", awserr.ErrAlreadyExists)
	// ErrResponseHeadersPolicyAlreadyExists is returned when a response headers policy
	// name is already in use.
	ErrResponseHeadersPolicyAlreadyExists = awserr.New(
		"ResponseHeadersPolicyAlreadyExists",
		awserr.ErrAlreadyExists,
	)
	// ErrOriginAccessControlAlreadyExists is returned when an origin access control name
	// is already in use.
	ErrOriginAccessControlAlreadyExists = awserr.New("OriginAccessControlAlreadyExists", awserr.ErrAlreadyExists)
	// ErrFunctionAlreadyExists is returned when a CloudFront function name is already in use.
	ErrFunctionAlreadyExists = awserr.New("FunctionAlreadyExists", awserr.ErrAlreadyExists)
	// ErrFLEAlreadyExists is returned when a field-level-encryption config's CallerReference
	// collides with an existing config of a different shape.
	ErrFLEAlreadyExists = awserr.New("FieldLevelEncryptionConfigAlreadyExists", awserr.ErrAlreadyExists)
	// ErrFLEProfileAlreadyExists is returned when a field-level-encryption profile name is
	// already in use.
	ErrFLEProfileAlreadyExists = awserr.New("FieldLevelEncryptionProfileAlreadyExists", awserr.ErrAlreadyExists)
	// ErrPublicKeyAlreadyExists is returned when a public key name is already in use.
	ErrPublicKeyAlreadyExists = awserr.New("PublicKeyAlreadyExists", awserr.ErrAlreadyExists)
	// ErrKeyGroupAlreadyExists is returned when a key group name is already in use.
	ErrKeyGroupAlreadyExists = awserr.New("KeyGroupAlreadyExists", awserr.ErrAlreadyExists)
	// ErrRealtimeLogConfigAlreadyExists is returned when a realtime log config name is
	// already in use.
	ErrRealtimeLogConfigAlreadyExists = awserr.New("RealtimeLogConfigAlreadyExists", awserr.ErrAlreadyExists)
	// ErrCachePolicyInUse is returned when attempting to delete a cache policy that is
	// still referenced by a distribution's default or ordered cache behavior.
	ErrCachePolicyInUse = awserr.New("CachePolicyInUse", awserr.ErrConflict)
	// ErrOriginRequestPolicyInUse is returned when attempting to delete an origin
	// request policy that is still referenced by a distribution's cache behavior.
	ErrOriginRequestPolicyInUse = awserr.New("OriginRequestPolicyInUse", awserr.ErrConflict)
	// ErrResponseHeadersPolicyInUse is returned when attempting to delete a response
	// headers policy that is still referenced by a distribution's cache behavior.
	ErrResponseHeadersPolicyInUse = awserr.New("ResponseHeadersPolicyInUse", awserr.ErrConflict)
	// ErrFunctionInUse is returned when attempting to delete a CloudFront function that
	// is still associated with a distribution's cache behavior.
	ErrFunctionInUse = awserr.New("FunctionInUse", awserr.ErrConflict)
	// ErrFLENotFound is returned when a requested field level encryption config does not exist.
	ErrFLENotFound = awserr.New("NoSuchFieldLevelEncryptionConfig", awserr.ErrNotFound)
	// ErrFLEProfileNotFound is returned when a requested field level encryption profile does not exist.
	ErrFLEProfileNotFound = awserr.New("NoSuchFieldLevelEncryptionProfile", awserr.ErrNotFound)
	// ErrPublicKeyNotFound is returned when a requested public key does not exist.
	ErrPublicKeyNotFound = awserr.New("NoSuchPublicKey", awserr.ErrNotFound)
	// ErrKeyGroupNotFound is returned when a requested key group does not exist.
	ErrKeyGroupNotFound = awserr.New("NoSuchResource", awserr.ErrNotFound)
	// ErrRealtimeLogConfigNotFound is returned when a requested realtime log config does not exist.
	ErrRealtimeLogConfigNotFound = awserr.New("NoSuchRealtimeLogConfig", awserr.ErrNotFound)
	// ErrKeyValueStoreNotFound is returned when a requested key value store does not exist.
	ErrKeyValueStoreNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)
	// ErrVpcOriginNotFound is returned when a requested VPC origin does not exist.
	// Code is EntityNotFound -- see ErrConnectionFunctionNotFound; "NoSuchVpcOrigin"
	// does not exist anywhere in the pinned SDK.
	ErrVpcOriginNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)
	// ErrResourcePolicyNotFound is returned when no resource policy has been put for a
	// resource ARN. Get/Put/DeleteResourcePolicy all declare EntityNotFound, not
	// NoSuchResourcePolicy, in their deserializeOpError switch (deserializers.go).
	ErrResourcePolicyNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)
	// ErrMonitoringSubscriptionNotFound is returned when no monitoring subscription exists for a
	// distribution.
	ErrMonitoringSubscriptionNotFound = awserr.New("NoSuchMonitoringSubscription", awserr.ErrNotFound)
	// ErrPublicKeyInUse is returned when a public key is still referenced by a key
	// group or a field-level-encryption profile and therefore cannot be deleted.
	ErrPublicKeyInUse = awserr.New("PublicKeyInUse", awserr.ErrConflict)
	// ErrFLEProfileInUse is returned when a field-level-encryption profile is still
	// referenced by a field-level-encryption config and therefore cannot be deleted.
	ErrFLEProfileInUse = awserr.New("FieldLevelEncryptionProfileInUse", awserr.ErrConflict)
	// ErrInconsistentQuantities is returned when a config payload declares a Quantity
	// for a list that does not match the number of Items actually provided. AWS
	// validates this pervasively across DistributionConfig and policy configs.
	ErrInconsistentQuantities = awserr.New("InconsistentQuantities", awserr.ErrInvalidParameter)
	// ErrOAIInUse is returned when attempting to delete an origin access identity that
	// is still referenced by a distribution's S3OriginConfig.
	ErrOAIInUse = awserr.New("CloudFrontOriginAccessIdentityInUse", awserr.ErrConflict)
	// ErrOACInUse is returned when attempting to delete an origin access control that
	// is still referenced by a distribution's origin.
	ErrOACInUse = awserr.New("OriginAccessControlInUse", awserr.ErrConflict)
	// ErrKeyGroupInUse is returned when attempting to delete a key group that is still
	// referenced by a distribution's cache behavior TrustedKeyGroups. Real AWS has no
	// dedicated KeyGroupInUse type; DeleteKeyGroup documents ResourceInUse for this case.
	ErrKeyGroupInUse = awserr.New("ResourceInUse", awserr.ErrConflict)
	// ErrOAIAlreadyExists is returned when CreateOAI reuses a CallerReference whose
	// stored Comment differs from the request (content-comparison idempotency; see
	// the real CloudFrontOriginAccessIdentityConfig.CallerReference doc).
	ErrOAIAlreadyExists = awserr.New("CloudFrontOriginAccessIdentityAlreadyExists", awserr.ErrAlreadyExists)
	// ErrDistributionAlreadyExists is returned when CreateDistribution/CopyDistribution
	// is called with a CallerReference that already identifies another distribution.
	// Unlike OAI/PublicKey/KeyGroup/FLE-profile, real AWS does NOT compare config content
	// here: per the CreateDistribution API docs, reuse of CallerReference always returns
	// this error, "regardless of the content of the DistributionConfig object".
	ErrDistributionAlreadyExists = awserr.New("DistributionAlreadyExists", awserr.ErrAlreadyExists)
	// ErrStreamingDistributionAlreadyExists is returned when
	// CreateStreamingDistribution reuses a CallerReference that already identifies
	// another streaming distribution. Same "always conflicts, content is irrelevant"
	// rule as ErrDistributionAlreadyExists (see CreateStreamingDistribution API docs).
	ErrStreamingDistributionAlreadyExists = awserr.New("StreamingDistributionAlreadyExists", awserr.ErrAlreadyExists)
	// ErrIllegalDelete is returned when attempting to delete an AWS-managed cache
	// policy, origin request policy, or response headers policy. Managed policies are
	// read-only; real AWS returns this instead of actually deleting them.
	ErrIllegalDelete = awserr.New("IllegalDelete", awserr.ErrInvalidParameter)
	// ErrIllegalUpdate is returned when attempting to update an AWS-managed cache
	// policy, origin request policy, or response headers policy.
	ErrIllegalUpdate = awserr.New("IllegalUpdate", awserr.ErrInvalidParameter)
)

// ErrPreconditionFailed is returned when an If-Match ETag check fails in a data-plane operation.
var ErrPreconditionFailed = errors.New("PreconditionFailed")

// ErrDistributionTenantNotFound is returned when a distribution tenant does not exist.
// Code is EntityNotFound -- see ErrConnectionFunctionNotFound; "NoSuchDistributionTenant"
// does not exist anywhere in the pinned SDK.
var ErrDistributionTenantNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)

// ErrInvalidTagging is returned when tag key/value constraints are violated.
var ErrInvalidTagging = awserr.New("InvalidTagging", awserr.ErrInvalidParameter)

// ErrCNAMEAlreadyExists is returned by CreateDistributionTenant/UpdateDistributionTenant
// when a domain is already associated with another distribution tenant or distribution.
// "DomainConflictException" does not exist anywhere in the pinned SDK; both ops' own
// deserializers (cloudfront@v1.67.4 deserializers.go) model CNAMEAlreadyExists for
// this case -- the same code CreateDistribution/UpdateDistribution use for an alias
// collision.
var ErrCNAMEAlreadyExists = awserr.New("CNAMEAlreadyExists", awserr.ErrConflict)

// ErrDomainControlValidationResourceNotFound is returned when ListDomainConflicts is given a
// DomainControlValidationResource that does not identify an existing distribution or
// distribution tenant. ListDomainConflicts's own error switch declares "EntityNotFound" for this
// case (deserializers.go), not the per-resource-type codes (NoSuchDistribution /
// NoSuchDistributionTenant) other operations use for the same underlying lookup failure.
var ErrDomainControlValidationResourceNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)

// ErrTrustStoreNotFound is returned when a trust store does not exist.
// Code is EntityNotFound -- see ErrConnectionFunctionNotFound; "NoSuchTrustStore"
// does not exist anywhere in the pinned SDK.
var ErrTrustStoreNotFound = awserr.New(codeEntityNotFound, awserr.ErrNotFound)

// ErrStreamingDistributionNotFound is returned when a streaming distribution does not exist.
var ErrStreamingDistributionNotFound = awserr.New("NoSuchStreamingDistribution", awserr.ErrNotFound)

// ErrStreamingDistributionNotDisabled is returned when deleting a streaming distribution
// that is still enabled.
var ErrStreamingDistributionNotDisabled = awserr.New("StreamingDistributionNotDisabled", awserr.ErrConflict)

// ---------------------------------------------------------------------------
// TrustStore
// ---------------------------------------------------------------------------
