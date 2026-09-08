package verifiedpermissions

import (
	"time"

	cedar "github.com/cedar-policy/cedar-go"
)

// ValidationMode constants for policy store validation settings.
const (
	ValidationModeOff    = "OFF"
	ValidationModeStrict = "STRICT"
)

// DeletionProtection constants for policy store deletion protection.
const (
	DeletionProtectionEnabled  = "ENABLED"
	DeletionProtectionDisabled = "DISABLED"
)

// AliasState constants for policy store alias state (real SDK:
// types.AliasState).
const (
	AliasStateActive          = "Active"
	AliasStatePendingDeletion = "PendingDeletion"
)

// DeletionMode constants for DeletePolicyStoreAlias (real SDK:
// types.DeletionMode).
const (
	DeletionModeSoftDelete = "SoftDelete"
	DeletionModeHardDelete = "HardDelete"
)

// policyStoreAliasPrefix is the mandatory prefix for every policy store
// alias name (real SDK doc, repeated on every alias-name field: "The alias
// name must always be prefixed with policy-store-alias/").
const policyStoreAliasPrefix = "policy-store-alias/"

// PolicyStoreAlias represents a Verified Permissions policy store alias -- a
// human-readable, account/region-unique name that (once Active) resolves to
// a policy store ID and can be used in place of that ID in the policyStoreId
// field of nearly every other operation (see Handler.resolvePolicyStoreID).
// Not itself taggable: the real SDK's TagResource doc says "In Verified
// Permissions, policy stores can be tagged" and there is no alias
// ResourceType, so -- unlike PolicyStore/Policy/PolicyTemplate/
// IdentitySource -- aliases are never registered in InMemoryBackend's
// arnIndex/resourceTags.
type PolicyStoreAlias struct {
	CreatedAt     time.Time `json:"createdAt"`
	AliasName     string    `json:"aliasName"`
	Arn           string    `json:"arn"`
	PolicyStoreID string    `json:"policyStoreID"`
	State         string    `json:"state"`
}

// PolicyStore represents an Amazon Verified Permissions policy store.
type PolicyStore struct {
	CreatedDate        time.Time         `json:"createdDate"`
	LastUpdated        time.Time         `json:"lastUpdated"`
	Tags               map[string]string `json:"tags,omitempty"`
	PolicyStoreID      string            `json:"policyStoreID"`
	Arn                string            `json:"arn"`
	Description        string            `json:"description"`
	AccountID          string            `json:"accountID"`
	Region             string            `json:"region"`
	ValidationMode     string            `json:"validationMode"`
	DeletionProtection string            `json:"deletionProtection"`
}

// Policy represents a policy in a Verified Permissions policy store.
type Policy struct {
	CreatedDate         time.Time `json:"createdDate"`
	LastUpdated         time.Time `json:"lastUpdated"`
	PolicyStoreID       string    `json:"policyStoreID"`
	PolicyID            string    `json:"policyID"`
	PolicyType          string    `json:"policyType"` // STATIC | TEMPLATE_LINKED
	Statement           string    `json:"statement"`
	Description         string    `json:"description,omitempty"`
	PolicyTemplateID    string    `json:"policyTemplateID,omitempty"`
	PrincipalEntityType string    `json:"principalEntityType,omitempty"`
	PrincipalEntityID   string    `json:"principalEntityID,omitempty"`
	ResourceEntityType  string    `json:"resourceEntityType,omitempty"`
	ResourceEntityID    string    `json:"resourceEntityID,omitempty"`
}

// PolicyTemplate represents a policy template in a Verified Permissions policy store.
type PolicyTemplate struct {
	CreatedDate      time.Time `json:"createdDate"`
	LastUpdated      time.Time `json:"lastUpdated"`
	PolicyStoreID    string    `json:"policyStoreID"`
	PolicyTemplateID string    `json:"policyTemplateID"`
	Description      string    `json:"description"`
	Statement        string    `json:"statement"`
	Name             string    `json:"name,omitempty"`
}

// CognitoGroupConfig holds Cognito group-to-Cedar-entity mapping configuration.
type CognitoGroupConfig struct {
	GroupEntityType string `json:"groupEntityType,omitempty"`
}

// OIDCGroupConfig holds OIDC group claim to Cedar entity mapping configuration.
type OIDCGroupConfig struct {
	GroupClaim      string `json:"groupClaim,omitempty"`
	GroupEntityType string `json:"groupEntityType,omitempty"`
}

// OIDCTokenSelection holds configuration for which OIDC token to use for authorization.
type OIDCTokenSelection struct {
	TokenType        string   `json:"tokenType,omitempty"` // IDENTITY | ACCESS
	PrincipalIDClaim string   `json:"principalIdClaim,omitempty"`
	Audiences        []string `json:"audiences,omitempty"`
}

// IdentitySource represents an Amazon Verified Permissions identity source.
type IdentitySource struct {
	CreatedDate         time.Time           `json:"createdDate"`
	LastUpdated         time.Time           `json:"lastUpdated"`
	CognitoGroupConfig  *CognitoGroupConfig `json:"cognitoGroupConfig,omitempty"`
	OIDCGroupConfig     *OIDCGroupConfig    `json:"oidcGroupConfig,omitempty"`
	OIDCTokenSelection  *OIDCTokenSelection `json:"oidcTokenSelection,omitempty"`
	IdentitySourceID    string              `json:"identitySourceId"`
	PolicyStoreID       string              `json:"policyStoreId"`
	PrincipalEntityType string              `json:"principalEntityType"`
	UserPoolArn         string              `json:"userPoolArn,omitempty"`
	OpenIDIssuer        string              `json:"openIdIssuer,omitempty"`
	EntityIDPrefix      string              `json:"entityIdPrefix,omitempty"`
	ClientIDs           []string            `json:"clientIds,omitempty"`
}

// PolicyStoreSchema holds the Cedar schema for a policy store.
type PolicyStoreSchema struct {
	CreatedDate time.Time `json:"createdDate"`
	LastUpdated time.Time `json:"lastUpdated"`
	Schema      string    `json:"schema"`
	// policyStoreID is the store.Table primary key (one schema per policy
	// store). It is never part of the wire API, so it carries no json tag
	// and is round-tripped separately through a DTO in persistence.go.
	policyStoreID string
	Namespaces    []string `json:"namespaces,omitempty"`
}

// AuthorizationRequest represents a single authorization evaluation request.
// Context is internal-only (never wire-marshaled directly -- the JSON echo in
// AuthDecision.Request is built field-by-field by the handler layer, see
// toBatchRequestEcho), so it carries no json tag.
type AuthorizationRequest struct {
	PrincipalEntityType string       `json:"principalEntityType,omitempty"`
	PrincipalEntityID   string       `json:"principalEntityId,omitempty"`
	ActionType          string       `json:"actionType,omitempty"`
	ActionID            string       `json:"actionId,omitempty"`
	ResourceEntityType  string       `json:"resourceEntityType,omitempty"`
	ResourceEntityID    string       `json:"resourceEntityId,omitempty"`
	Context             cedar.Record `json:"-"`
}

// AuthDecision is the result of a single authorization evaluation.
type AuthDecision struct {
	Request             AuthorizationRequest `json:"request"`
	Decision            string               `json:"decision"`
	DeterminingPolicies []string             `json:"determiningPolicies"`
	Errors              []string             `json:"errors"`
}

// BatchGetPolicyItem identifies a policy to retrieve in a batch request.
type BatchGetPolicyItem struct {
	PolicyStoreID string `json:"policyStoreId"`
	PolicyID      string `json:"policyId"`
}

// BatchGetPolicyResult holds the results of a BatchGetPolicy call.
type BatchGetPolicyResult struct {
	Results []Policy                  `json:"results"`
	Errors  []batchGetPolicyErrorItem `json:"errors"`
}

type batchGetPolicyErrorItem struct {
	PolicyStoreID string `json:"policyStoreId"`
	PolicyID      string `json:"policyId"`
	Code          string `json:"code"`
	Message       string `json:"message"`
}

// CreatePolicyParams holds parameters for creating a policy.
type CreatePolicyParams struct {
	PolicyType          string // "STATIC" or "TEMPLATE_LINKED"
	Statement           string // STATIC only
	Description         string // STATIC only
	PolicyTemplateID    string // TEMPLATE_LINKED only
	PrincipalEntityType string // TEMPLATE_LINKED only
	PrincipalEntityID   string // TEMPLATE_LINKED only
	ResourceEntityType  string // TEMPLATE_LINKED only
	ResourceEntityID    string // TEMPLATE_LINKED only
	ClientToken         string // idempotency token, see InMemoryBackend.checkClientToken
}

// UpdatePolicyParams holds parameters for updating a static policy. Real
// AWS's UpdatePolicy can only update static policies; there is no
// TEMPLATE_LINKED variant.
type UpdatePolicyParams struct {
	Statement   string
	Description string
}

// IdentitySourceConfig holds full identity source configuration for create/update.
type IdentitySourceConfig struct {
	// Cognito
	UserPoolArn            string
	ClientIDs              []string
	CognitoGroupEntityType string
	// OIDC
	Issuer              string
	EntityIDPrefix      string
	OIDCGroupClaim      string
	OIDCGroupEntityType string
	// Token selection
	TokenType        string // "IDENTITY" or "ACCESS"
	PrincipalIDClaim string
	Audiences        []string
}

// ListPoliciesFilter holds filter params for ListPolicies. PrincipalUnspecified
// / ResourceUnspecified mirror the wire filter's EntityReference "unspecified"
// variant: when set, only policies with no principal/resource scope match.
type ListPoliciesFilter struct {
	PolicyType           string
	PolicyTemplateID     string
	PrincipalEntityType  string
	PrincipalEntityID    string
	ResourceEntityType   string
	ResourceEntityID     string
	PrincipalUnspecified bool
	ResourceUnspecified  bool
}

// decisionAllow / decisionDeny are the decision strings returned by authorization evaluations.
const (
	decisionAllow = "ALLOW"
	decisionDeny  = "DENY"
)

const (
	policyTypeStatic         = "STATIC"
	policyTypeTemplateLinked = "TEMPLATE_LINKED"
	arnKindPolicyStore       = "policyStore"
	arnKindPolicy            = "policy"
	arnKindPolicyTemplate    = "policyTemplate"
	arnKindIdentitySource    = "identitySource"
)

// OIDCTokenSelection.TokenType values (real SDK: the identityTokenOnly /
// accessTokenOnly union members of OpenIdConnectTokenSelection).
const (
	tokenTypeIdentity = "IDENTITY"
	tokenTypeAccess   = "ACCESS"
)
