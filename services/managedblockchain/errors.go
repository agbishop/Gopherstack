package managedblockchain

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrNetworkNotFound is returned when a network does not exist.
	ErrNetworkNotFound = awserr.New("ResourceNotFoundException: network not found", awserr.ErrNotFound)
	// ErrMemberNotFound is returned when a member does not exist.
	ErrMemberNotFound = awserr.New("ResourceNotFoundException: member not found", awserr.ErrNotFound)
	// ErrResourceNotFound is returned when a resource (network or member) cannot be found by ARN.
	ErrResourceNotFound = awserr.New("ResourceNotFoundException: resource not found", awserr.ErrNotFound)
	// ErrNetworkAlreadyExists is returned when a network already exists.
	ErrNetworkAlreadyExists = awserr.New(
		"ResourceAlreadyExistsException: network already exists",
		awserr.ErrAlreadyExists,
	)
	// ErrMissingNetworkName is returned when the network name is missing.
	ErrMissingNetworkName = errors.New("Name is required for CreateNetwork")
	// ErrMissingClientRequestToken is returned when ClientRequestToken is missing from
	// CreateNetwork/CreateMember/CreateNode/CreateProposal/CreateAccessor. The real
	// aws-sdk-go-v2 client-side validator (validators.go, all 5 ops, v1.34.4) marks it
	// required and the SDK's idempotency-token middleware always fills it in when a caller
	// leaves it unset, so a real SDK client never omits it; a raw HTTP caller bypassing that
	// middleware can, and real AWS rejects that request.
	ErrMissingClientRequestToken = errors.New("ClientRequestToken is required")
	// ErrMissingMemberName is returned when the member name is missing.
	ErrMissingMemberName = errors.New("Name is required for member configuration")
	// ErrMissingNetworkID is returned when the network ID is missing from a path.
	ErrMissingNetworkID = errors.New("networkId is required")
	// ErrNodeNotFound is returned when a node does not exist.
	ErrNodeNotFound = awserr.New("ResourceNotFoundException: node not found", awserr.ErrNotFound)
	// ErrAccessorNotFound is returned when an accessor does not exist.
	ErrAccessorNotFound = awserr.New("ResourceNotFoundException: accessor not found", awserr.ErrNotFound)
	// ErrProposalNotFound is returned when a proposal does not exist.
	ErrProposalNotFound = awserr.New("ResourceNotFoundException: proposal not found", awserr.ErrNotFound)
	// ErrInvitationNotFound is returned when an invitation does not exist.
	ErrInvitationNotFound = awserr.New("ResourceNotFoundException: invitation not found", awserr.ErrNotFound)
	// ErrMissingMemberID is returned when the member ID is missing for a proposal.
	ErrMissingMemberID = errors.New("MemberId is required for CreateProposal")
	// ErrMissingVoterMemberID is returned when the voter member ID is missing for VoteOnProposal.
	ErrMissingVoterMemberID = errors.New("VoterMemberId is required for VoteOnProposal")
	// ErrMissingNodeMemberID is returned when MemberId is missing for a node operation. Real AWS
	// documents MemberId as "required for Hyperledger Fabric" on every node op (CreateNode's body
	// field, GetNode/ListNodes/DeleteNode/UpdateNode's "memberId" query parameter); gopherstack
	// only emulates Hyperledger Fabric networks, so it is always required here.
	ErrMissingNodeMemberID = errors.New("MemberId is required for Hyperledger Fabric node operations")
	// ErrMissingMemberFrameworkConfig is returned when MemberConfiguration.FrameworkConfiguration
	// is missing. The real aws-sdk-go-v2 client-side validator (validateMemberConfiguration)
	// requires this field on both CreateNetwork's and CreateMember's MemberConfiguration.
	ErrMissingMemberFrameworkConfig = errors.New("FrameworkConfiguration is required for member configuration")
	// ErrMissingMemberFabricConfig is returned when MemberConfiguration.FrameworkConfiguration.Fabric
	// is missing. gopherstack only emulates Hyperledger Fabric networks (see ErrMissingNodeMemberID),
	// so unlike the real API (which permits an empty FrameworkConfiguration for future frameworks),
	// Fabric is always required here.
	ErrMissingMemberFabricConfig = errors.New(
		"FrameworkConfiguration.Fabric is required for Hyperledger Fabric member configuration",
	)
	// ErrMissingMemberAdminUsername is returned when Fabric.AdminUsername is missing. Real AWS's
	// client-side validator (validateMemberFabricConfiguration) requires this field.
	ErrMissingMemberAdminUsername = errors.New("AdminUsername is required for member's Fabric configuration")
	// ErrMissingMemberAdminPassword is returned when Fabric.AdminPassword is missing. Real AWS's
	// client-side validator (validateMemberFabricConfiguration) requires this field.
	ErrMissingMemberAdminPassword = errors.New("AdminPassword is required for member's Fabric configuration")
	// ErrInvalidMemberAdminPassword is returned when Fabric.AdminPassword does not meet the real
	// API's documented length constraint (8-32 characters).
	ErrInvalidMemberAdminPassword = errors.New("AdminPassword must be between 8 and 32 characters")
	// ErrMissingNetworkFabricEdition is returned when FrameworkConfiguration.Fabric is present but
	// Edition is missing. Real AWS's client-side validator (validateNetworkFabricConfiguration)
	// requires this field whenever a Fabric configuration object is supplied.
	ErrMissingNetworkFabricEdition = errors.New(
		"FrameworkConfiguration.Fabric.Edition is required when Fabric is specified",
	)
	// ErrUnsupportedNetworkFramework is returned when Framework is set to a value other than
	// HYPERLEDGER_FABRIC. Real AWS's CreateNetwork documents itself as "Applies only to
	// Hyperledger Fabric" -- new networks (including ETHEREUM, still a valid Framework enum
	// member used elsewhere, e.g. accessors) can no longer be created through this operation.
	ErrUnsupportedNetworkFramework = errors.New("CreateNetworkInput.Framework must be HYPERLEDGER_FABRIC")
	// ErrMissingInvitationID is returned when CreateMemberInput.InvitationId is missing.
	// The real aws-sdk-go-v2 client-side validator (validateOpCreateMemberInput) marks
	// this field required and refuses to send the request without it.
	ErrMissingInvitationID = errors.New("InvitationId is required for CreateMember")
	// ErrInvitationNetworkMismatch is returned when CreateMember's InvitationId names an
	// invitation issued for a different network than the one in the request path.
	ErrInvitationNetworkMismatch = awserr.New(
		"InvalidRequestException: invitation is not for this network",
		awserr.ErrInvalidParameter,
	)
	// ErrInvitationNotPending is returned when CreateMember's InvitationId names an
	// invitation that is not PENDING -- already used to join (ACCEPTED), rejected, or
	// expired. Real AWS's Invitation.Status doc documents ACCEPTED as "The invitee created
	// a member and joined the network using the InvitationId", implying an invitation is
	// consumed exactly once.
	ErrInvitationNotPending = awserr.New(
		"InvalidRequestException: invitation is not pending",
		awserr.ErrInvalidParameter,
	)
	// ErrValidation is returned when input validation fails.
	ErrValidation = awserr.New("InvalidRequestException: validation error", awserr.ErrInvalidParameter)
	// ErrTooManyTags is returned when tagging a resource would push its total
	// tag count above the 50-tag-per-resource limit (botocore
	// managedblockchain/2018-09-24 service-2.json.gz InputTagMap: max 50;
	// TagResourceRequest.Tags doc: "an overall maximum of 50 tags added to
	// each resource" -- the cap is on the resource's resulting tag count,
	// not the request's tag count). TagResource and every Create* op that
	// accepts Tags (CreateNetwork, CreateMember via MemberConfiguration.Tags,
	// CreateNode, CreateProposal, CreateAccessor) declare TooManyTagsException
	// (aws-sdk-go-v2 managedblockchain@v1.34.4 deserializers.go, each op's own
	// awsRestjson1_deserializeOpError<Op> switch).
	ErrTooManyTags = awserr.New("TooManyTagsException: too many tags", awserr.ErrInvalidParameter)
)
