package redshift

import "errors"

const (
	errClusterSnapshotNotFound = "ClusterSnapshotNotFound"
)

// Error code strings below are verified verbatim against the ErrorCode() method of
// each corresponding fault type in aws-sdk-go-v2/service/redshift@v1.65.4/types/errors.go
// (re-checked at v1.65.4, the version currently pinned in go.mod; unchanged from v1.62.3).
// Real AWS is NOT consistent about the "Fault" suffix -- some fault ErrorCode() values
// include it (e.g. "HsmConfigurationNotFoundFault") and some strip it (e.g.
// "ClusterNotFound" for ClusterNotFoundFault) -- so each entry here was checked
// individually rather than assumed from a pattern. Do not "clean up" a suffix without
// re-checking the SDK source.
var (
	ErrClusterNotFound                = errors.New("ClusterNotFound")
	ErrClusterAlreadyExists           = errors.New("ClusterAlreadyExists")
	ErrInvalidParameter               = errors.New("InvalidParameterValue")
	ErrReservedNodeNotFound           = errors.New("ReservedNodeNotFound")
	ErrReservedNodeAlreadyExists      = errors.New("ReservedNodeAlreadyExists")
	ErrReservedNodeOfferingNotFound   = errors.New("ReservedNodeOfferingNotFound")
	ErrPartnerNotFound                = errors.New("PartnerNotFound")
	ErrDataShareNotFound              = errors.New("InvalidDataShareFault")
	ErrSecurityGroupNotFound          = errors.New("ClusterSecurityGroupNotFound")
	ErrSecurityGroupAlreadyExists     = errors.New("ClusterSecurityGroupAlreadyExists")
	ErrSnapshotNotFound               = errors.New(errClusterSnapshotNotFound)
	ErrSnapshotAlreadyExists          = errors.New("ClusterSnapshotAlreadyExists")
	ErrEndpointAuthNotFound           = errors.New("EndpointAuthorizationNotFound")
	ErrEndpointAuthAlreadyExists      = errors.New("EndpointAuthorizationAlreadyExists")
	ErrResizeNotFound                 = errors.New("ResizeNotFound")
	ErrResizeNotCancellable           = errors.New("InvalidClusterState")
	ErrParameterGroupNotFound         = errors.New("ClusterParameterGroupNotFound")
	ErrParameterGroupAlreadyExists    = errors.New("ClusterParameterGroupAlreadyExists")
	ErrSubnetGroupNotFound            = errors.New("ClusterSubnetGroupNotFoundFault")
	ErrSubnetGroupAlreadyExists       = errors.New("ClusterSubnetGroupAlreadyExists")
	ErrEventSubscriptionNotFound      = errors.New("SubscriptionNotFound")
	ErrEventSubscriptionAlreadyExists = errors.New("SubscriptionAlreadyExist")
	ErrSnapshotCopyGrantNotFound      = errors.New("SnapshotCopyGrantNotFoundFault")
	ErrSnapshotCopyGrantAlreadyExists = errors.New("SnapshotCopyGrantAlreadyExistsFault")
	ErrSnapshotScheduleNotFound       = errors.New("SnapshotScheduleNotFound")
	ErrSnapshotScheduleAlreadyExists  = errors.New("SnapshotScheduleAlreadyExists")
	ErrUsageLimitNotFound             = errors.New("UsageLimitNotFound")
	ErrAuthProfileNotFound            = errors.New("AuthenticationProfileNotFoundFault")
	ErrAuthProfileAlreadyExists       = errors.New("AuthenticationProfileAlreadyExistsFault")
	ErrResourcePolicyNotFound         = errors.New("ResourceNotFoundFault")
	ErrSnapshotCopyAlreadyEnabled     = errors.New("SnapshotCopyAlreadyEnabledFault")
	ErrSnapshotCopyNotEnabled         = errors.New("CopyToRegionDisabledFault")
	ErrHsmClientCertNotFound          = errors.New("HsmClientCertificateNotFoundFault")
	ErrHsmClientCertAlreadyExists     = errors.New("HsmClientCertificateAlreadyExistsFault")
	ErrHsmConfigNotFound              = errors.New("HsmConfigurationNotFoundFault")
	ErrHsmConfigAlreadyExists         = errors.New("HsmConfigurationAlreadyExistsFault")
	ErrScheduledActionNotFound        = errors.New("ScheduledActionNotFound")
	ErrScheduledActionAlreadyExists   = errors.New("ScheduledActionAlreadyExists")
	ErrCustomDomainNotFound           = errors.New("CustomDomainAssociationNotFoundFault")
	ErrCustomDomainAlreadyExists      = errors.New("CustomCnameAssociationFault")
	ErrEndpointAccessNotFound         = errors.New("EndpointNotFound")
	ErrEndpointAccessAlreadyExists    = errors.New("EndpointAlreadyExists")
	ErrIntegrationNotFound            = errors.New("IntegrationNotFoundFault")
	ErrIntegrationAlreadyExists       = errors.New("IntegrationAlreadyExistsFault")
	ErrIdcApplicationNotFound         = errors.New("RedshiftIdcApplicationNotExists")
	ErrIdcApplicationAlreadyExists    = errors.New("RedshiftIdcApplicationAlreadyExists")
	// Qev2IdcApplication fault codes verified against
	// aws-sdk-go-v2/service/redshift@v1.65.0/types/errors.go
	// Qev2IdcApplicationNotExistsFault.ErrorCode() and
	// Qev2IdcApplicationAlreadyExistsFault.ErrorCode() -- these are a distinct
	// fault family from RedshiftIdcApplication's above (no shared "Redshift"
	// prefix), matching Qev2IdcApplication being a distinct resource.
	ErrQev2IdcApplicationNotFound      = errors.New("Qev2IdcApplicationNotExists")
	ErrQev2IdcApplicationAlreadyExists = errors.New("Qev2IdcApplicationAlreadyExists")
	// ErrSnapshotAccessNotFound is returned by RevokeSnapshotAccess when the
	// target account does not currently have restore access to revoke
	// (ErrorCode() "AuthorizationNotFound", verified against
	// AuthorizationNotFoundFault in types/errors.go and RevokeSnapshotAccess's
	// own declared error switch in deserializers.go -- that op has no
	// InvalidParameterValue-shaped fault at all).
	ErrSnapshotAccessNotFound = errors.New("AuthorizationNotFound")
	// ErrSecurityGroupIngressNotFound is returned by RevokeClusterSecurityGroupIngress
	// when the given CIDRIP/EC2SecurityGroupName matched no existing rule
	// (ErrorCode() "AuthorizationNotFound", verified against
	// AuthorizationNotFoundFault in types/errors.go and this op's own declared
	// error switch, awsAwsquery_deserializeOpErrorRevokeClusterSecurityGroupIngress
	// in deserializers.go -- same fault family as ErrSnapshotAccessNotFound above,
	// a distinct sentinel per errCodeSentinels' same-text-different-call-site
	// convention).
	ErrSecurityGroupIngressNotFound = errors.New("AuthorizationNotFound")
	// ErrAuthorizationAlreadyExists is returned by AuthorizeClusterSecurityGroupIngress
	// and AuthorizeSnapshotAccess when the given CIDRIP/EC2SecurityGroupName or
	// AccountWithRestoreAccess is already authorized (ErrorCode()
	// "AuthorizationAlreadyExists", verified against AuthorizationAlreadyExistsFault
	// in types/errors.go and both ops' own declared error switches,
	// awsAwsquery_deserializeOpErrorAuthorizeClusterSecurityGroupIngress /
	// awsAwsquery_deserializeOpErrorAuthorizeSnapshotAccess in deserializers.go).
	ErrAuthorizationAlreadyExists = errors.New("AuthorizationAlreadyExists")
	// ErrNamespaceRegistrationInvalidClusterState is returned by RegisterNamespace/
	// DeregisterNamespace when the target cluster exists but isn't in a
	// registerable state (ErrorCode() "InvalidClusterState", verified against
	// InvalidClusterStateFault in types/errors.go). Deliberately distinct from
	// ErrResizeNotCancellable, which carries the same wire text for an
	// unrelated resize-cancellation meaning -- see errCodeSentinels, where
	// resolveErrCode only needs the sentinel's Error() text to match, so two
	// same-text sentinels for different call sites are fine.
	ErrNamespaceRegistrationInvalidClusterState = errors.New("InvalidClusterState")
	// ErrInvalidNamespace is returned by RegisterNamespace/DeregisterNamespace
	// when NamespaceIdentifier doesn't resolve to a real cluster or Redshift
	// Serverless namespace/workgroup (ErrorCode() "InvalidNamespaceFault",
	// verified against InvalidNamespaceFault in types/errors.go).
	ErrInvalidNamespace = errors.New("InvalidNamespaceFault")
	// ErrSecurityGroupInvalidState is returned by DeleteClusterSecurityGroup
	// when the target is the default security group (ErrorCode()
	// "InvalidClusterSecurityGroupState", verified against
	// InvalidClusterSecurityGroupStateFault in types/errors.go and this op's
	// own declared error switch, awsAwsquery_deserializeOpErrorDeleteClusterSecurityGroup
	// in deserializers.go).
	ErrSecurityGroupInvalidState = errors.New("InvalidClusterSecurityGroupState")
	// ErrParameterGroupInvalidState is returned by DeleteClusterParameterGroup
	// when the target is a default parameter group (ErrorCode()
	// "InvalidClusterParameterGroupState", verified against
	// InvalidClusterParameterGroupStateFault in types/errors.go and this op's
	// own declared error switch, awsAwsquery_deserializeOpErrorDeleteClusterParameterGroup
	// in deserializers.go).
	ErrParameterGroupInvalidState = errors.New("InvalidClusterParameterGroupState")
	// ErrSnapshotHasAuthorizedAccounts is returned by DeleteClusterSnapshot
	// when other accounts still have restore access to the snapshot
	// (ErrorCode() "InvalidClusterSnapshotState", verified against
	// InvalidClusterSnapshotStateFault in types/errors.go and this op's own
	// declared error switch, awsAwsquery_deserializeOpErrorDeleteClusterSnapshot
	// in deserializers.go). Per api_op_DeleteClusterSnapshot.go, the snapshot
	// must be in the available state with no other users authorized to
	// access it, and other accounts' authorizations must be revoked before
	// the snapshot can be deleted.
	ErrSnapshotHasAuthorizedAccounts = errors.New("InvalidClusterSnapshotState")
	// ErrClusterInvalidState is returned by ModifyCluster when the target
	// cluster is not in the "available" state (ErrorCode() "InvalidClusterState",
	// verified against InvalidClusterStateFault in types/errors.go -- "The
	// specified cluster is not in the available state" -- and this op's own
	// declared error switch, awsAwsquery_deserializeOpErrorModifyCluster in
	// deserializers.go). Deliberately distinct from ErrResizeNotCancellable and
	// ErrNamespaceRegistrationInvalidClusterState, which carry the same wire
	// text for unrelated call sites -- see errCodeSentinels, where resolveErrCode
	// only needs the sentinel's Error() text to match.
	ErrClusterInvalidState = errors.New("InvalidClusterState")
	// ErrInvalidS3KeyPrefix is returned by EnableLogging when S3KeyPrefix
	// contains a character outside the set documented on
	// EnableLoggingInput.S3KeyPrefix (ErrorCode() "InvalidS3KeyPrefixFault",
	// verified against InvalidS3KeyPrefixFault in types/errors.go).
	ErrInvalidS3KeyPrefix = errors.New("InvalidS3KeyPrefixFault")
)
