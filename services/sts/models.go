package sts

import (
	"encoding/xml"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
)

const (
	// STSNamespace is the XML namespace for STS wire responses.
	STSNamespace = "https://sts.amazonaws.com/doc/2011-06-15/"

	// MockAccountID is the default mock AWS account ID returned by GetCallerIdentity.
	MockAccountID = config.DefaultAccountID

	// MockUserID is the fixed user ID returned by GetCallerIdentity.
	MockUserID = "AKIAIOSFODNN7EXAMPLE" //nolint:gosec // well-known AWS example key, not real credentials

	// MockUserArn is the default ARN returned by GetCallerIdentity.
	MockUserArn = "arn:aws:iam::" + config.DefaultAccountID + ":root"

	// DefaultDurationSeconds is the default credential lifetime (1 hour).
	DefaultDurationSeconds = 3600

	// MinDurationSeconds is the minimum allowed credential lifetime.
	MinDurationSeconds = 900

	// MaxDurationSeconds is the maximum allowed credential lifetime for AssumeRole (12 hours).
	MaxDurationSeconds = 43200

	// DefaultSessionTokenDurationSeconds is the default lifetime for GetSessionToken (12 hours).
	DefaultSessionTokenDurationSeconds = 43200

	// MinSessionTokenDurationSeconds is the minimum allowed lifetime (15 minutes).
	MinSessionTokenDurationSeconds = 900

	// MaxSessionTokenDurationSeconds is the maximum allowed lifetime for GetSessionToken (36 hours for IAM users).
	MaxSessionTokenDurationSeconds = 129600

	// MaxTagCount is the maximum number of session tags allowed per AssumeRole call.
	MaxTagCount = 50

	// MaxFederationTokenDurationSeconds is the maximum allowed lifetime for GetFederationToken (36 hours).
	MaxFederationTokenDurationSeconds = 129600

	// MaxRootDurationSeconds is the maximum allowed lifetime for AssumeRoot (15 minutes).
	MaxRootDurationSeconds = 900

	// MaxRoleChainDurationSeconds is the AWS cap on session duration when the caller
	// uses temporary credentials (role chaining). AWS enforces 1 hour regardless of
	// the target role's MaxSessionDuration.
	MaxRoleChainDurationSeconds = 3600

	// DefaultWebIdentityTokenDurationSeconds is the default lifetime for GetWebIdentityToken (5 minutes).
	DefaultWebIdentityTokenDurationSeconds = 300

	// MinWebIdentityTokenDurationSeconds is the minimum allowed lifetime for GetWebIdentityToken (1 minute).
	MinWebIdentityTokenDurationSeconds = 60

	// MaxWebIdentityTokenDurationSeconds is the maximum allowed lifetime for GetWebIdentityToken (1 hour).
	MaxWebIdentityTokenDurationSeconds = 3600

	// MaxAudienceCount is the maximum number of audience entries for GetWebIdentityToken.
	MaxAudienceCount = 10

	// MinRoleSessionNameLen is the minimum allowed session name length per AWS.
	MinRoleSessionNameLen = 2

	// MaxRoleSessionNameLen is the maximum allowed session name length per AWS.
	MaxRoleSessionNameLen = 64

	// MaxFederationTokenNameLen is the maximum allowed federation token name length per AWS.
	MaxFederationTokenNameLen = 32

	// MinFederationTokenNameLen is the minimum allowed federation token name length per AWS.
	MinFederationTokenNameLen = 2

	// MaxPolicyArnsCount is the maximum number of managed policy ARNs allowed per operation.
	MaxPolicyArnsCount = 10

	// MaxProvidedContextsCount is the maximum number of provided contexts per operation.
	MaxProvidedContextsCount = 5

	// MaxTagKeyLen is the maximum allowed length for a session tag key.
	MaxTagKeyLen = 128

	// MaxTagValueLen is the maximum allowed length for a session tag value.
	MaxTagValueLen = 256

	// MinTagKeyLen is the minimum allowed length for a session tag key.
	MinTagKeyLen = 1

	// MinSourceIdentityLen is the minimum allowed length for SourceIdentity.
	MinSourceIdentityLen = 2

	// MaxSourceIdentityLen is the maximum allowed length for SourceIdentity.
	MaxSourceIdentityLen = 64

	// MaxProvidedContextLen is the maximum allowed length for a ProvidedContext assertion or ARN.
	MaxProvidedContextLen = 2048

	// MFATokenCodeLen is the required length for MFA token codes.
	MFATokenCodeLen = 6
)

// Tag represents a session tag key-value pair passed to AssumeRole.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ProvidedContext carries a federated identity context assertion.
type ProvidedContext struct {
	ProviderArn      string
	ContextAssertion string
}

// AssumeRoleInput holds the parameters for an AssumeRole call.
type AssumeRoleInput struct {
	CallerSession     *SessionInfo
	RoleArn           string
	RoleSessionName   string
	ExternalID        string
	Policy            string
	SourceIdentity    string
	CallerAccessKeyID string
	// SerialNumber and TokenCode carry MFA proof, mirroring GetSessionToken's
	// identically named/validated parameters. Present iff the caller supplied
	// both; checked against any aws:MultiFactorAuthPresent trust-policy condition.
	SerialNumber string
	TokenCode    string
	// CallerArn is the resolved ARN of the calling principal (e.g. an assumed-role
	// ARN during role chaining). When set, the target role's trust policy is
	// evaluated against it; when empty, trust-policy Principal evaluation is
	// skipped (the emulator only enforces constraints it can positively determine).
	CallerArn         string
	Tags              []Tag
	TransitiveTagKeys []string
	PolicyArns        []string
	ProvidedContexts  []ProvidedContext
	DurationSeconds   int32
}

// AssumedRoleUser contains the ARN and ID of the resulting assumed-role principal.
type AssumedRoleUser struct {
	Arn           string `xml:"Arn"`
	AssumedRoleID string `xml:"AssumedRoleId"`
}

// Credentials holds a set of temporary AWS security credentials.
type Credentials struct {
	AccessKeyID     string `xml:"AccessKeyId"`
	SecretAccessKey string `xml:"SecretAccessKey"`
	SessionToken    string `xml:"SessionToken"`
	Expiration      string `xml:"Expiration"`
}

// AssumeRoleResult wraps the assumed-role user and credentials.
type AssumeRoleResult struct {
	AssumedRoleUser AssumedRoleUser `xml:"AssumedRoleUser"`
	Credentials     Credentials     `xml:"Credentials"`
	// SourceIdentity is the source identity set when the role was assumed.
	SourceIdentity string `xml:"SourceIdentity,omitempty"`
	// PackedPolicySize is the percentage of session policy size used (informational).
	PackedPolicySize int32 `xml:"PackedPolicySize,omitempty"`
}

// ResponseMetadata carries the per-request identifier.
type ResponseMetadata struct {
	RequestID string `xml:"RequestId"`
}

// AssumeRoleResponse is the top-level XML envelope returned by AssumeRole.
type AssumeRoleResponse struct {
	XMLName          xml.Name         `xml:"AssumeRoleResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	AssumeRoleResult AssumeRoleResult `xml:"AssumeRoleResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// GetCallerIdentityResult carries the caller's account, ARN, and user-ID.
type GetCallerIdentityResult struct {
	Account string `xml:"Account"`
	Arn     string `xml:"Arn"`
	UserID  string `xml:"UserId"`
}

// GetCallerIdentityResponse is the top-level XML envelope returned by GetCallerIdentity.
type GetCallerIdentityResponse struct {
	XMLName                 xml.Name                `xml:"GetCallerIdentityResponse"`
	Xmlns                   string                  `xml:"xmlns,attr"`
	GetCallerIdentityResult GetCallerIdentityResult `xml:"GetCallerIdentityResult"`
	ResponseMetadata        ResponseMetadata        `xml:"ResponseMetadata"`
}

// ErrorDetail carries the STS error code and message.
type ErrorDetail struct {
	Type    string `xml:"Type"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// GetSessionTokenInput holds the parameters for a GetSessionToken call.
type GetSessionTokenInput struct {
	SerialNumber    string
	TokenCode       string
	DurationSeconds int32
}

// GetSessionTokenResult wraps the credentials.
type GetSessionTokenResult struct {
	Credentials Credentials `xml:"Credentials"`
}

// GetSessionTokenResponse is the top-level XML envelope returned by GetSessionToken.
type GetSessionTokenResponse struct {
	XMLName               xml.Name              `xml:"GetSessionTokenResponse"`
	Xmlns                 string                `xml:"xmlns,attr"`
	GetSessionTokenResult GetSessionTokenResult `xml:"GetSessionTokenResult"`
	ResponseMetadata      ResponseMetadata      `xml:"ResponseMetadata"`
}

// ErrorResponse is the XML error envelope returned on failed STS operations.
type ErrorResponse struct {
	XMLName   xml.Name    `xml:"ErrorResponse"`
	Xmlns     string      `xml:"xmlns,attr"`
	Error     ErrorDetail `xml:"Error"`
	RequestID string      `xml:"RequestId"`
}

// GetAccessKeyInfoResult carries the account for the given access key.
type GetAccessKeyInfoResult struct {
	Account string `xml:"Account"`
}

// GetAccessKeyInfoResponse is the top-level XML envelope returned by GetAccessKeyInfo.
type GetAccessKeyInfoResponse struct {
	XMLName                xml.Name               `xml:"GetAccessKeyInfoResponse"`
	Xmlns                  string                 `xml:"xmlns,attr"`
	GetAccessKeyInfoResult GetAccessKeyInfoResult `xml:"GetAccessKeyInfoResult"`
	ResponseMetadata       ResponseMetadata       `xml:"ResponseMetadata"`
}

// DecodeAuthorizationMessageResult carries the decoded message.
type DecodeAuthorizationMessageResult struct {
	DecodedMessage string `xml:"DecodedMessage"`
}

// DecodeAuthorizationMessageResponse is the top-level XML envelope returned by DecodeAuthorizationMessage.
type DecodeAuthorizationMessageResponse struct {
	XMLName                          xml.Name                         `xml:"DecodeAuthorizationMessageResponse"`
	Xmlns                            string                           `xml:"xmlns,attr"`
	DecodeAuthorizationMessageResult DecodeAuthorizationMessageResult `xml:"DecodeAuthorizationMessageResult"`
	ResponseMetadata                 ResponseMetadata                 `xml:"ResponseMetadata"`
}

// SessionInfo stores metadata about an issued assumed-role session for GetCallerIdentity lookups.
type SessionInfo struct {
	// Expiration is the time at which this session expires and should be evicted.
	Expiration     time.Time `json:"expiration"`
	AssumedRoleArn string    `json:"assumed_role_arn"`
	AccountID      string    `json:"account_id"`
	SessionName    string    `json:"session_name"`
	AccessKeyID    string    `json:"access_key_id"`
	// SecretAccessKey is the secret key for this session, stored for in-process SigV4 validation.
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	// SessionToken is the session token for this credential set, used to match X-Amz-Security-Token.
	SessionToken string `json:"session_token,omitempty"`
	// AssumedRoleID is the AROA-prefixed role ID + session name (e.g. "AROATESTROLEID:session").
	// It is the value returned by GetCallerIdentity as the UserId for assumed-role credentials.
	AssumedRoleID     string   `json:"assumed_role_id"`
	SourceIdentity    string   `json:"source_identity,omitempty"`
	Tags              []Tag    `json:"tags,omitempty"`
	TransitiveTagKeys []string `json:"transitive_tag_keys,omitempty"`
	// IsAssumedRole reports whether this session was minted by an actual role
	// assumption (AssumeRole/AssumeRoleWithSAML/AssumeRoleWithWebIdentity/
	// AssumeRoot). False for GetSessionToken/GetFederationToken/
	// GetDelegatedAccessToken sessions, which keep the caller's own identity
	// rather than assuming a role. ResolvePrincipal uses this to report the
	// correct awsmeta.PrincipalKind to IAM's cross-service enforcement path.
	IsAssumedRole bool `json:"is_assumed_role,omitempty"`
}

// SessionMetrics represents STS session and janitor sweep metrics for dashboard views.
type SessionMetrics struct {
	ActiveSessions         int   `json:"activeSessions"`
	ExpiredSessions        int   `json:"expiredSessions"`
	SweepCount             int64 `json:"sweepCount"`
	ExpiredEvictions       int64 `json:"expiredEvictions"`
	TotalSessionsCreated   int64 `json:"totalSessionsCreated"`
	OpsAssumeRole          int64 `json:"opsAssumeRole"`
	OpsAssumeRoleWithSAML  int64 `json:"opsAssumeRoleWithSAML"`
	OpsAssumeRoleWithWI    int64 `json:"opsAssumeRoleWithWebIdentity"`
	OpsAssumeRoot          int64 `json:"opsAssumeRoot"`
	OpsGetCallerIdentity   int64 `json:"opsGetCallerIdentity"`
	OpsGetFederationToken  int64 `json:"opsGetFederationToken"`
	OpsGetSessionToken     int64 `json:"opsGetSessionToken"`
	OpsGetWebIdentityToken int64 `json:"opsGetWebIdentityToken"`
	OpsGetAccessKeyInfo    int64 `json:"opsGetAccessKeyInfo"`
	OpsDecodeAuthMessage   int64 `json:"opsDecodeAuthorizationMessage"`
	OpsGetDelegatedToken   int64 `json:"opsGetDelegatedAccessToken"`
}

// GetFederationTokenInput holds the parameters for a GetFederationToken call.
type GetFederationTokenInput struct {
	Name            string
	Policy          string
	Tags            []Tag
	PolicyArns      []string
	DurationSeconds int32
}

// FederatedUser contains the ARN and ID of the resulting federated-user principal.
type FederatedUser struct {
	Arn             string `xml:"Arn"`
	FederatedUserID string `xml:"FederatedUserId"`
}

// GetFederationTokenResult wraps the federated user and credentials.
type GetFederationTokenResult struct {
	FederatedUser    FederatedUser `xml:"FederatedUser"`
	Credentials      Credentials   `xml:"Credentials"`
	PackedPolicySize int32         `xml:"PackedPolicySize,omitempty"`
}

// GetFederationTokenResponse is the top-level XML envelope returned by GetFederationToken.
type GetFederationTokenResponse struct {
	XMLName                  xml.Name                 `xml:"GetFederationTokenResponse"`
	Xmlns                    string                   `xml:"xmlns,attr"`
	GetFederationTokenResult GetFederationTokenResult `xml:"GetFederationTokenResult"`
	ResponseMetadata         ResponseMetadata         `xml:"ResponseMetadata"`
}

// AssumeRoleWithWebIdentityInput holds the parameters for an
// AssumeRoleWithWebIdentity call. Per aws-sdk-go-v2/service/sts's
// AssumeRoleWithWebIdentityInput, there is no SourceIdentity or Tags request
// member for this operation — unlike AssumeRole, AWS derives both from custom
// claims added to the WebIdentityToken by the identity provider (see
// jwtClaimSourceIdentity / jwtClaimTags in token_validation.go).
type AssumeRoleWithWebIdentityInput struct {
	RoleArn          string
	RoleSessionName  string
	WebIdentityToken string
	ProviderID       string
	Policy           string
	PolicyArns       []string
	DurationSeconds  int32
}

// AssumeRoleWithSAMLInput holds the parameters for an AssumeRoleWithSAML call.
// Per aws-sdk-go-v2/service/sts's AssumeRoleWithSAMLInput, there is no
// RoleSessionName, SourceIdentity, or Tags request member for this operation —
// unlike AssumeRole/AssumeRoleWithWebIdentity, AWS derives the session name,
// source identity, and session tags from named <Attribute> elements inside the
// SAMLAssertion itself (see saml_attributes.go's extractSAMLAssertionData).
type AssumeRoleWithSAMLInput struct {
	RoleArn         string
	PrincipalArn    string
	SAMLAssertion   string
	Policy          string
	PolicyArns      []string
	DurationSeconds int32
}

// AssumeRoleWithSAMLResult wraps the assumed-role user, credentials, and SAML provider details.
type AssumeRoleWithSAMLResult struct {
	AssumedRoleUser  AssumedRoleUser `xml:"AssumedRoleUser"`
	Credentials      Credentials     `xml:"Credentials"`
	Audience         string          `xml:"Audience,omitempty"`
	Issuer           string          `xml:"Issuer,omitempty"`
	NameQualifier    string          `xml:"NameQualifier,omitempty"`
	Subject          string          `xml:"Subject,omitempty"`
	SubjectType      string          `xml:"SubjectType,omitempty"`
	SourceIdentity   string          `xml:"SourceIdentity,omitempty"`
	PackedPolicySize int32           `xml:"PackedPolicySize,omitempty"`
}

// AssumeRoleWithSAMLResponse is the top-level XML envelope returned by AssumeRoleWithSAML.
type AssumeRoleWithSAMLResponse struct {
	XMLName                  xml.Name                 `xml:"AssumeRoleWithSAMLResponse"`
	Xmlns                    string                   `xml:"xmlns,attr"`
	AssumeRoleWithSAMLResult AssumeRoleWithSAMLResult `xml:"AssumeRoleWithSAMLResult"`
	ResponseMetadata         ResponseMetadata         `xml:"ResponseMetadata"`
}

// AssumeRootInput holds the parameters for an AssumeRoot call.
type AssumeRootInput struct {
	// CallerSession is the caller's own STS session, when the request was made
	// using temporary security credentials. There is no SourceIdentity request
	// parameter for AssumeRoot; AWS documents AssumeRootOutput.SourceIdentity as
	// "the source identity specified by the principal that is calling the
	// AssumeRoot operation" and that source identity "persists across chained
	// role sessions" — so it is inherited from the caller's own session here,
	// mirroring AssumeRole's role-chaining SourceIdentity propagation.
	CallerSession   *SessionInfo
	TargetPrincipal string
	TaskPolicyArn   string
	DurationSeconds int32
}

// AssumeRootResult wraps the credentials returned by AssumeRoot.
type AssumeRootResult struct {
	Credentials    Credentials `xml:"Credentials"`
	SourceIdentity string      `xml:"SourceIdentity,omitempty"`
}

// AssumeRootResponse is the top-level XML envelope returned by AssumeRoot.
type AssumeRootResponse struct {
	XMLName          xml.Name         `xml:"AssumeRootResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	AssumeRootResult AssumeRootResult `xml:"AssumeRootResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// GetDelegatedAccessTokenInput holds the parameters for a GetDelegatedAccessToken call.
// Per aws-sdk-go-v2/service/sts's GetDelegatedAccessTokenInput, TradeInToken is the
// only request member — there is no DurationSeconds parameter for this operation.
type GetDelegatedAccessTokenInput struct {
	TradeInToken string
}

// GetDelegatedAccessTokenResult wraps the principal and credentials returned by GetDelegatedAccessToken.
type GetDelegatedAccessTokenResult struct {
	AssumedPrincipal string      `xml:"AssumedPrincipal,omitempty"`
	Credentials      Credentials `xml:"Credentials"`
	PackedPolicySize int32       `xml:"PackedPolicySize,omitempty"`
}

// GetDelegatedAccessTokenResponse is the top-level XML envelope returned by GetDelegatedAccessToken.
type GetDelegatedAccessTokenResponse struct {
	XMLName                       xml.Name                      `xml:"GetDelegatedAccessTokenResponse"`
	Xmlns                         string                        `xml:"xmlns,attr"`
	GetDelegatedAccessTokenResult GetDelegatedAccessTokenResult `xml:"GetDelegatedAccessTokenResult"`
	ResponseMetadata              ResponseMetadata              `xml:"ResponseMetadata"`
}

// GetWebIdentityTokenInput holds the parameters for a GetWebIdentityToken call.
type GetWebIdentityTokenInput struct {
	// CallerSession is the caller's own STS session, when the request was made
	// using temporary security credentials (looked up by access key ID from the
	// SigV4 Authorization header / X-Amz-Security-Token). It is used to enforce
	// that the issued JWT's expiration does not exceed the calling session's own
	// expiration (AWS SessionDurationEscalationException). Nil when the caller
	// used long-lived (non-STS) credentials, in which case no such cap applies.
	CallerSession    *SessionInfo
	SigningAlgorithm string
	Audience         []string
	Tags             []Tag
	DurationSeconds  int32
}

// GetWebIdentityTokenResult wraps the token and expiration returned by GetWebIdentityToken.
type GetWebIdentityTokenResult struct {
	WebIdentityToken string `xml:"WebIdentityToken"`
	Expiration       string `xml:"Expiration"`
}

// GetWebIdentityTokenResponse is the top-level XML envelope returned by GetWebIdentityToken.
type GetWebIdentityTokenResponse struct {
	XMLName                   xml.Name                  `xml:"GetWebIdentityTokenResponse"`
	Xmlns                     string                    `xml:"xmlns,attr"`
	GetWebIdentityTokenResult GetWebIdentityTokenResult `xml:"GetWebIdentityTokenResult"`
	ResponseMetadata          ResponseMetadata          `xml:"ResponseMetadata"`
}

// AssumeRoleWithWebIdentityResult wraps the assumed-role user, credentials, and OIDC provider details.
type AssumeRoleWithWebIdentityResult struct {
	AssumedRoleUser             AssumedRoleUser `xml:"AssumedRoleUser"`
	Credentials                 Credentials     `xml:"Credentials"`
	SubjectFromWebIdentityToken string          `xml:"SubjectFromWebIdentityToken,omitempty"`
	Audience                    string          `xml:"Audience,omitempty"`
	Provider                    string          `xml:"Provider,omitempty"`
	SourceIdentity              string          `xml:"SourceIdentity,omitempty"`
	PackedPolicySize            int32           `xml:"PackedPolicySize,omitempty"`
}

// AssumeRoleWithWebIdentityResponse is the top-level XML envelope returned by AssumeRoleWithWebIdentity.
type AssumeRoleWithWebIdentityResponse struct {
	XMLName                         xml.Name                        `xml:"AssumeRoleWithWebIdentityResponse"`
	Xmlns                           string                          `xml:"xmlns,attr"`
	AssumeRoleWithWebIdentityResult AssumeRoleWithWebIdentityResult `xml:"AssumeRoleWithWebIdentityResult"`
	ResponseMetadata                ResponseMetadata                `xml:"ResponseMetadata"`
}
