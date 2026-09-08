// Package kms provides a mock AWS Key Management Service (KMS) implementation.
package kms

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
)

const (
	// nanoToSeconds converts nanoseconds to seconds.
	nanoToSeconds = 1e9
	// MockAccountID is the mock AWS account ID.
	MockAccountID = config.DefaultAccountID
	// MockRegion is the mock AWS region.
	MockRegion = config.DefaultRegion
)

// KeyStateEnabled is the string constant for an enabled key.
const KeyStateEnabled = "Enabled"

// KeyStateDisabled is the string constant for a disabled key.
const KeyStateDisabled = "Disabled"

// KeyUsageEncryptDecrypt is the string constant for the default key usage.
const KeyUsageEncryptDecrypt = "ENCRYPT_DECRYPT"

// KeyUsageSignVerify is the string constant for sign/verify-only keys.
const KeyUsageSignVerify = "SIGN_VERIFY"

// Note: Go fields use KeyID (Go convention) while JSON tags use KeyId (AWS API wire format).
// This intentional difference matches both Go naming best practices and AWS API compatibility.

// RotationRecord records a single key material rotation with its type.
type RotationRecord struct {
	RotationType string  `json:"RotationType"`
	Date         float64 `json:"Date"`
}

// Key represents a KMS customer-managed key.
type Key struct {
	Origin           string `json:"Origin,omitempty"`
	PrimaryRegion    string `json:"PrimaryRegion,omitempty"`
	Description      string `json:"Description,omitempty"`
	KeyState         string `json:"KeyState"`
	KeyUsage         string `json:"KeyUsage"`
	KeySpec          string `json:"KeySpec,omitempty"`
	KeyID            string `json:"KeyId"`
	Arn              string `json:"Arn"`
	ExpirationModel  string `json:"ExpirationModel,omitempty"`
	CustomKeyStoreID string `json:"CustomKeyStoreId,omitempty"`
	// Rotations stores all rotation events with their types. The separate
	// RotationDates and OnDemandRotationDates slices are kept for JSON
	// backwards-compatibility with existing snapshots.
	Rotations             []RotationRecord `json:"Rotations,omitempty"`
	RotationDates         []float64        `json:"RotationDates,omitempty"`
	OnDemandRotationDates []float64        `json:"OnDemandRotationDates,omitempty"`
	// ReplicaKeyIDs stores the key IDs of replica keys created from this primary.
	ReplicaKeyIDs        []string `json:"ReplicaKeyIds,omitempty"`
	CreationDate         float64  `json:"CreationDate"`
	DeletionDate         float64  `json:"DeletionDate,omitempty"`
	ValidTo              float64  `json:"ValidTo,omitempty"`
	PendingWindowInDays  int      `json:"PendingWindowInDays,omitempty"`
	RotationPeriodInDays int32    `json:"RotationPeriodInDays,omitempty"`
	Enabled              bool     `json:"Enabled"`
	MultiRegion          bool     `json:"MultiRegion,omitempty"`
	RotationEnabled      bool     `json:"RotationEnabled"`
}

// MultiRegionKeyRef is a reference to a primary or replica key in a multi-region set.
type MultiRegionKeyRef struct {
	// Arn is the ARN of the multi-region key.
	Arn string `json:"Arn"`
	// Region is the AWS region of the multi-region key.
	Region string `json:"Region"`
}

// MultiRegionConfiguration describes the multi-region key topology.
type MultiRegionConfiguration struct {
	// MultiRegionKeyType is either PRIMARY or REPLICA.
	MultiRegionKeyType string `json:"MultiRegionKeyType,omitempty"`
	// PrimaryKey references the primary key in the multi-region set.
	PrimaryKey *MultiRegionKeyRef `json:"PrimaryKey,omitempty"`
	// ReplicaKeys lists the replica keys associated with the primary.
	ReplicaKeys []MultiRegionKeyRef `json:"ReplicaKeys,omitempty"`
}

// KeyMetadata is the metadata for a KMS key returned in API responses.
type KeyMetadata struct {
	MultiRegionConfiguration    *MultiRegionConfiguration `json:"MultiRegionConfiguration,omitempty"`
	PrimaryRegion               string                    `json:"PrimaryRegion,omitempty"`
	Arn                         string                    `json:"Arn"`
	Description                 string                    `json:"Description,omitempty"`
	KeyState                    string                    `json:"KeyState"`
	KeyUsage                    string                    `json:"KeyUsage"`
	KeyManager                  string                    `json:"KeyManager,omitempty"`
	Origin                      string                    `json:"Origin,omitempty"`
	KeySpec                     string                    `json:"KeySpec,omitempty"`
	KeyID                       string                    `json:"KeyId"`
	AWSAccountID                string                    `json:"AWSAccountId,omitempty"`
	CustomKeyStoreID            string                    `json:"CustomKeyStoreId,omitempty"`
	CustomerMasterKeySpec       string                    `json:"CustomerMasterKeySpec,omitempty"`
	MultiRegionKeyType          string                    `json:"MultiRegionKeyType,omitempty"`
	ExpirationModel             string                    `json:"ExpirationModel,omitempty"`
	MacAlgorithms               []string                  `json:"MacAlgorithms,omitempty"`
	SigningAlgorithms           []string                  `json:"SigningAlgorithms,omitempty"`
	KeyAgreementAlgorithms      []string                  `json:"KeyAgreementAlgorithms,omitempty"`
	EncryptionAlgorithms        []string                  `json:"EncryptionAlgorithms,omitempty"`
	CreationDate                float64                   `json:"CreationDate"`
	DeletionDate                float64                   `json:"DeletionDate,omitempty"`
	ValidTo                     float64                   `json:"ValidTo,omitempty"`
	PendingDeletionWindowInDays int                       `json:"PendingDeletionWindowInDays,omitempty"`
	MultiRegion                 bool                      `json:"MultiRegion"`
	Enabled                     bool                      `json:"Enabled"`
}

// Tag is a key-value pair attached to a KMS resource.
type Tag struct {
	// TagKey is the tag key.
	TagKey string `json:"TagKey"`
	// TagValue is the tag value.
	TagValue string `json:"TagValue"`
}

// Alias represents a KMS alias pointing to a key.
type Alias struct {
	// AliasName is the alias name (e.g., alias/my-key).
	AliasName string `json:"AliasName"`
	// AliasArn is the full ARN of the alias.
	AliasArn string `json:"AliasArn"`
	// TargetKeyId is the key ID that this alias points to.
	TargetKeyID string `json:"TargetKeyId,omitempty"`
	// CreationDate is the Unix timestamp when the alias was created.
	CreationDate float64 `json:"CreationDate,omitempty"`
	// LastUpdatedDate is the Unix timestamp when the alias was last updated.
	LastUpdatedDate float64 `json:"LastUpdatedDate,omitempty"`
}

// CreateKeyInput is the request payload for CreateKey.
type CreateKeyInput struct {
	Description                    string `json:"Description,omitempty"`
	KeyUsage                       string `json:"KeyUsage,omitempty"`
	KeySpec                        string `json:"KeySpec,omitempty"`
	Origin                         string `json:"Origin,omitempty"`
	Policy                         string `json:"Policy,omitempty"`
	Region                         string `json:"-"`
	CustomKeyStoreID               string `json:"CustomKeyStoreId,omitempty"`
	Tags                           []Tag  `json:"Tags,omitempty"`
	MultiRegion                    bool   `json:"MultiRegion,omitempty"`
	BypassPolicyLockoutSafetyCheck bool   `json:"BypassPolicyLockoutSafetyCheck,omitempty"`
}

// CreateKeyOutput is the response payload for CreateKey.
type CreateKeyOutput struct {
	// KeyMetadata contains the newly created key metadata.
	KeyMetadata KeyMetadata `json:"KeyMetadata"`
}

// DescribeKeyInput is the request payload for DescribeKey.
type DescribeKeyInput struct {
	// KeyId is the key ID or alias to describe.
	KeyID string `json:"KeyId"`
	// GrantTokens is an optional list of grant tokens used to make a just-created
	// grant that permits DescribeKey immediately effective. DescribeKey is a valid
	// grant operation (see isValidGrantOperation) and the real DescribeKey op
	// declares InvalidGrantTokenException in its error set, so a supplied token
	// must resolve to an existing, unexpired grant.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// DescribeKeyOutput is the response payload for DescribeKey.
type DescribeKeyOutput struct {
	// KeyMetadata contains the key metadata.
	KeyMetadata KeyMetadata `json:"KeyMetadata"`
}

// KeyListEntry is a brief key reference used in ListKeys.
type KeyListEntry struct {
	// KeyId is the UUID of the key.
	KeyID string `json:"KeyId"`
	// KeyArn is the full ARN of the key.
	KeyArn string `json:"KeyArn"`
	// Description is the optional human-readable description of the key.
	Description string `json:"Description,omitempty"`
}

// ListKeysInput is the request payload for ListKeys.
type ListKeysInput struct {
	// Limit caps the number of results returned.
	Limit *int32 `json:"Limit,omitempty"`
	// Marker is the pagination cursor from a previous call.
	Marker string `json:"Marker,omitempty"`
}

// ListKeysOutput is the response payload for ListKeys.
type ListKeysOutput struct {
	NextMarker string         `json:"NextMarker,omitempty"`
	Keys       []KeyListEntry `json:"Keys"`
	Truncated  bool           `json:"Truncated"`
}

// EncryptInput is the request payload for Encrypt.
type EncryptInput struct {
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens []string `json:"GrantTokens,omitempty"`
	KeyID       string   `json:"KeyId"`
	Plaintext   []byte   `json:"Plaintext"`
}

// EncryptOutput is the response payload for Encrypt.
type EncryptOutput struct {
	KeyID               string `json:"KeyId"`
	EncryptionAlgorithm string `json:"EncryptionAlgorithm,omitempty"`
	CiphertextBlob      []byte `json:"CiphertextBlob"`
}

// DecryptInput is the request payload for Decrypt.
type DecryptInput struct {
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens    []string `json:"GrantTokens,omitempty"`
	KeyID          string   `json:"KeyId,omitempty"`
	CiphertextBlob []byte   `json:"CiphertextBlob"`
}

// DecryptOutput is the response payload for Decrypt.
type DecryptOutput struct {
	KeyID               string `json:"KeyId"`
	EncryptionAlgorithm string `json:"EncryptionAlgorithm,omitempty"`
	Plaintext           []byte `json:"Plaintext"`
}

// GenerateDataKeyInput is the request payload for GenerateDataKey.
type GenerateDataKeyInput struct {
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
	NumberOfBytes     *int32            `json:"NumberOfBytes,omitempty"`
	KeyID             string            `json:"KeyId"`
	KeySpec           string            `json:"KeySpec,omitempty"`
	GrantTokens       []string          `json:"GrantTokens,omitempty"`
}

// GenerateDataKeyOutput is the response payload for GenerateDataKey.
type GenerateDataKeyOutput struct {
	KeyID          string `json:"KeyId"`
	CiphertextBlob []byte `json:"CiphertextBlob"`
	Plaintext      []byte `json:"Plaintext"`
}

// ReEncryptInput is the request payload for ReEncrypt.
type ReEncryptInput struct {
	SourceEncryptionContext      map[string]string `json:"SourceEncryptionContext,omitempty"`
	DestinationEncryptionContext map[string]string `json:"DestinationEncryptionContext,omitempty"`
	DestinationKeyID             string            `json:"DestinationKeyId"`
	SourceKeyID                  string            `json:"SourceKeyId,omitempty"`
	CiphertextBlob               []byte            `json:"CiphertextBlob"`
}

// ReEncryptOutput is the response payload for ReEncrypt.
type ReEncryptOutput struct {
	KeyID                          string `json:"KeyId"`
	SourceKeyID                    string `json:"SourceKeyId"`
	SourceEncryptionAlgorithm      string `json:"SourceEncryptionAlgorithm,omitempty"`
	DestinationEncryptionAlgorithm string `json:"DestinationEncryptionAlgorithm,omitempty"`
	CiphertextBlob                 []byte `json:"CiphertextBlob"`
}

// CreateAliasInput is the request payload for CreateAlias.
type CreateAliasInput struct {
	// AliasName is the name of the alias (must begin with alias/).
	AliasName string `json:"AliasName"`
	// TargetKeyId is the key ID the alias should point to.
	TargetKeyID string `json:"TargetKeyId"`
}

// UpdateAliasInput is the request payload for UpdateAlias.
type UpdateAliasInput struct {
	// AliasName is the existing alias to redirect.
	AliasName string `json:"AliasName"`
	// TargetKeyId is the new key ID the alias should point to.
	TargetKeyID string `json:"TargetKeyId"`
}

// DeleteAliasInput is the request payload for DeleteAlias.
type DeleteAliasInput struct {
	// AliasName is the name of the alias to delete.
	AliasName string `json:"AliasName"`
}

// ListAliasesInput is the request payload for ListAliases.
type ListAliasesInput struct {
	// KeyId optionally filters aliases to those pointing to this key.
	KeyID string `json:"KeyId,omitempty"`
	// Limit caps the number of results returned.
	Limit *int32 `json:"Limit,omitempty"`
	// Marker is the pagination cursor from a previous call.
	Marker string `json:"Marker,omitempty"`
}

// ListAliasesOutput is the response payload for ListAliases.
type ListAliasesOutput struct {
	NextMarker string  `json:"NextMarker,omitempty"`
	Aliases    []Alias `json:"Aliases"`
	Truncated  bool    `json:"Truncated"`
}

// EnableKeyRotationInput is the request payload for EnableKeyRotation.
type EnableKeyRotationInput struct {
	RotationPeriodInDays *int32 `json:"RotationPeriodInDays,omitempty"`
	KeyID                string `json:"KeyId"`
}

// DisableKeyRotationInput is the request payload for DisableKeyRotation.
type DisableKeyRotationInput struct {
	// KeyId is the key to disable rotation for.
	KeyID string `json:"KeyId"`
}

// GetKeyRotationStatusInput is the request payload for GetKeyRotationStatus.
type GetKeyRotationStatusInput struct {
	// KeyId is the key to query rotation status for.
	KeyID string `json:"KeyId"`
}

// GetKeyRotationStatusOutput is the response payload for GetKeyRotationStatus.
type GetKeyRotationStatusOutput struct {
	KeyID                     string  `json:"KeyId"`
	NextRotationDate          float64 `json:"NextRotationDate,omitempty"`
	OnDemandRotationStartDate float64 `json:"OnDemandRotationStartDate,omitempty"`
	RotationPeriodInDays      int32   `json:"RotationPeriodInDays,omitempty"`
	KeyRotationEnabled        bool    `json:"KeyRotationEnabled"`
}

// KeyStatePendingDeletion is the string constant for a key pending deletion.
const KeyStatePendingDeletion = "PendingDeletion"

// KeyStatePendingReplicaDeletion is the string constant for a multi-Region
// primary key whose deletion was scheduled while it still has replica keys.
// It stays in this non-final state indefinitely until the last replica is
// actually deleted, at which point it moves to KeyStatePendingDeletion and
// its waiting period begins.
const KeyStatePendingReplicaDeletion = "PendingReplicaDeletion"

// KeyStatePendingImport is the string constant for a key awaiting imported key material.
const KeyStatePendingImport = "PendingImport"

// KeyOriginAWSKMS is the origin for keys whose material is generated by AWS KMS.
const KeyOriginAWSKMS = "AWS_KMS"

// KeyOriginExternal is the origin for keys whose material is imported by the customer.
const KeyOriginExternal = "EXTERNAL"

// DisableKeyInput is the request payload for DisableKey.
type DisableKeyInput struct {
	KeyID string `json:"KeyId"`
}

// EnableKeyInput is the request payload for EnableKey.
type EnableKeyInput struct {
	KeyID string `json:"KeyId"`
}

// ScheduleKeyDeletionInput is the request payload for ScheduleKeyDeletion.
type ScheduleKeyDeletionInput struct {
	KeyID               string `json:"KeyId"`
	PendingWindowInDays int    `json:"PendingWindowInDays,omitempty"`
}

// ScheduleKeyDeletionOutput is the response payload for ScheduleKeyDeletion.
type ScheduleKeyDeletionOutput struct {
	KeyID    string `json:"KeyId"`
	KeyState string `json:"KeyState"`
	// DeletionDate is absent for a multi-Region primary key with replicas:
	// real AWS doesn't know the deletion date until the last replica is
	// deleted, so the key is in KeyStatePendingReplicaDeletion instead.
	DeletionDate        float64 `json:"DeletionDate,omitempty"`
	PendingWindowInDays int     `json:"PendingWindowInDays,omitempty"`
}

// CancelKeyDeletionInput is the request payload for CancelKeyDeletion.
type CancelKeyDeletionInput struct {
	KeyID string `json:"KeyId"`
}

// CancelKeyDeletionOutput is the response payload for CancelKeyDeletion.
type CancelKeyDeletionOutput struct {
	KeyID    string `json:"KeyId"`
	KeyState string `json:"KeyState"`
}

// ErrorResponse is the KMS JSON error response format.
type ErrorResponse struct {
	// Type is the error type string.
	Type string `json:"__type"`
	// Message is the human-readable error message.
	Message string `json:"message"`
}

// UnixTimeFloat converts a time value to a Unix timestamp float.
func UnixTimeFloat(t time.Time) float64 {
	return float64(t.UnixNano()) / nanoToSeconds
}

// GrantConstraints holds the constraints for a grant.
// When set, cryptographic operations using this grant's token must supply
// an encryption context that satisfies the constraint.
type GrantConstraints struct {
	// EncryptionContextEquals requires the caller's encryption context to be
	// an exact match of this map (same keys and values).
	EncryptionContextEquals map[string]string `json:"EncryptionContextEquals,omitempty"`
	// EncryptionContextSubset requires the caller's encryption context to
	// contain at least all key-value pairs present in this map.
	EncryptionContextSubset map[string]string `json:"EncryptionContextSubset,omitempty"`
	// SourceArn restricts grant use to requests made on behalf of the named
	// AWS resource (effectively the aws:SourceArn condition key). This mock
	// has no cross-service request-context plumbing to carry a "made on
	// behalf of" resource ARN through crypto calls, so the constraint is
	// stored and round-tripped (CreateGrant -> ListGrants/ListRetirableGrants)
	// for wire parity but is NOT enforced -- consistent with the scope
	// boundary already documented for grant-token authorization elsewhere in
	// this package (see isValidGrantOperation's doc and CreateGrantInput.
	// GrantTokens below).
	SourceArn string `json:"SourceArn,omitempty"`
}

// Grant represents a KMS key grant.
type Grant struct {
	// Constraints holds optional constraints for the grant.
	Constraints *GrantConstraints `json:"Constraints,omitempty"`
	// GrantID is the unique identifier for the grant.
	GrantID string `json:"GrantId"`
	// KeyID is the ID of the KMS key.
	KeyID string `json:"KeyId"`
	// GranteePrincipal is the principal that receives the grant. Mutually
	// exclusive with GranteeServicePrincipal; exactly one must be set.
	GranteePrincipal string `json:"GranteePrincipal,omitempty"`
	// GranteeServicePrincipal is the AWS service principal that receives the
	// grant. Mutually exclusive with GranteePrincipal; exactly one must be
	// set. No AWS-service-principal simulation exists in this mock (no
	// IAM/authorization layer at all -- see CreateGrantInput.GrantTokens), so
	// this is stored/round-tripped for wire parity, matching real AWS's
	// GrantListEntry shape, without any behavioral effect.
	GranteeServicePrincipal string `json:"GranteeServicePrincipal,omitempty"`
	// RetiringPrincipal is the principal that can retire the grant. Mutually
	// exclusive with RetiringServicePrincipal.
	RetiringPrincipal string `json:"RetiringPrincipal,omitempty"`
	// RetiringServicePrincipal is the AWS service principal that can retire
	// the grant. Mutually exclusive with RetiringPrincipal. Same no-IAM-layer
	// scope boundary as GranteeServicePrincipal.
	RetiringServicePrincipal string `json:"RetiringServicePrincipal,omitempty"`
	// GrantToken is a token that can be used to identify this grant.
	GrantToken string `json:"GrantToken"`
	// TokenIssuedAt records when the grant token was issued, enabling expiry checks.
	TokenIssuedAt time.Time `json:"TokenIssuedAt"`
	// Name is an optional name for the grant.
	Name string `json:"Name,omitempty"`
	// Operations is the list of cryptographic operations the grantee can perform.
	Operations []string `json:"Operations"`
	// CreationDate is the Unix timestamp when the grant was created.
	CreationDate float64 `json:"CreationDate"`
	// IssuingAccount is the AWS account ID under which the grant was issued.
	IssuingAccount string `json:"IssuingAccount,omitempty"`
}

// CreateGrantInput is the request payload for CreateGrant.
type CreateGrantInput struct {
	Constraints              *GrantConstraints `json:"Constraints,omitempty"`
	KeyID                    string            `json:"KeyId"`
	GranteePrincipal         string            `json:"GranteePrincipal,omitempty"`
	GranteeServicePrincipal  string            `json:"GranteeServicePrincipal,omitempty"`
	RetiringPrincipal        string            `json:"RetiringPrincipal,omitempty"`
	RetiringServicePrincipal string            `json:"RetiringServicePrincipal,omitempty"`
	Name                     string            `json:"Name,omitempty"`
	Operations               []string          `json:"Operations"`
	// GrantTokens authorizes the CreateGrant call itself via an existing,
	// not-yet-eventually-consistent grant. There is no IAM/authorization
	// layer anywhere in this mock, so this field is accepted for wire parity
	// and otherwise a no-op -- the same documented scope boundary as
	// CreateKeyInput/ReplicateKeyInput's BypassPolicyLockoutSafetyCheck.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// CreateGrantOutput is the response payload for CreateGrant.
type CreateGrantOutput struct {
	GrantID    string `json:"GrantId"`
	GrantToken string `json:"GrantToken"`
}

// ListGrantsInput is the request payload for ListGrants.
type ListGrantsInput struct {
	Limit   *int32 `json:"Limit,omitempty"`
	KeyID   string `json:"KeyId"`
	GrantID string `json:"GrantId,omitempty"`
	Marker  string `json:"Marker,omitempty"`
}

// ListGrantsOutput is the response payload for ListGrants.
type ListGrantsOutput struct {
	NextMarker string           `json:"NextMarker,omitempty"`
	Grants     []GrantListEntry `json:"Grants"`
	Truncated  bool             `json:"Truncated"`
}

// GrantListEntry is the wire shape of a single ListGrants/ListRetirableGrants
// result entry, matching real AWS's types.GrantListEntry field-for-field.
// It deliberately excludes GrantToken and TokenIssuedAt: a grant token is
// returned exactly once, in the CreateGrant response, and is never
// retrievable from a List call -- see kms.Grant for the internal storage
// representation that does carry both.
type GrantListEntry struct {
	Constraints              *GrantConstraints `json:"Constraints,omitempty"`
	GrantID                  string            `json:"GrantId"`
	KeyID                    string            `json:"KeyId"`
	GranteePrincipal         string            `json:"GranteePrincipal,omitempty"`
	GranteeServicePrincipal  string            `json:"GranteeServicePrincipal,omitempty"`
	RetiringPrincipal        string            `json:"RetiringPrincipal,omitempty"`
	RetiringServicePrincipal string            `json:"RetiringServicePrincipal,omitempty"`
	Name                     string            `json:"Name,omitempty"`
	Operations               []string          `json:"Operations"`
	CreationDate             float64           `json:"CreationDate"`
	IssuingAccount           string            `json:"IssuingAccount,omitempty"`
}

// toGrantListEntry converts a stored Grant into its wire-safe ListGrants shape,
// stripping GrantToken and TokenIssuedAt.
func toGrantListEntry(g *Grant) GrantListEntry {
	return GrantListEntry{
		Constraints:              g.Constraints,
		GrantID:                  g.GrantID,
		KeyID:                    g.KeyID,
		GranteePrincipal:         g.GranteePrincipal,
		GranteeServicePrincipal:  g.GranteeServicePrincipal,
		RetiringPrincipal:        g.RetiringPrincipal,
		RetiringServicePrincipal: g.RetiringServicePrincipal,
		Name:                     g.Name,
		Operations:               g.Operations,
		CreationDate:             g.CreationDate,
		IssuingAccount:           g.IssuingAccount,
	}
}

// RevokeGrantInput is the request payload for RevokeGrant.
type RevokeGrantInput struct {
	KeyID   string `json:"KeyId"`
	GrantID string `json:"GrantId"`
}

// RetireGrantInput is the request payload for RetireGrant.
type RetireGrantInput struct {
	GrantToken string `json:"GrantToken,omitempty"`
	GrantID    string `json:"GrantId,omitempty"`
	KeyID      string `json:"KeyId,omitempty"`
}

// ListRetirableGrantsInput is the request payload for ListRetirableGrants.
// Real AWS requires exactly one of RetiringPrincipal/RetiringServicePrincipal
// (aws-sdk-go-v2/service/kms@v1.55.4 api_op_ListRetirableGrants.go).
type ListRetirableGrantsInput struct {
	Limit                    *int32 `json:"Limit,omitempty"`
	RetiringPrincipal        string `json:"RetiringPrincipal,omitempty"`
	RetiringServicePrincipal string `json:"RetiringServicePrincipal,omitempty"`
	Marker                   string `json:"Marker,omitempty"`
}

// GenerateDataKeyWithoutPlaintextInput is the request payload for GenerateDataKeyWithoutPlaintext.
type GenerateDataKeyWithoutPlaintextInput struct {
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
	NumberOfBytes     *int32            `json:"NumberOfBytes,omitempty"`
	KeyID             string            `json:"KeyId"`
	KeySpec           string            `json:"KeySpec,omitempty"`
	GrantTokens       []string          `json:"GrantTokens,omitempty"`
}

// GenerateDataKeyWithoutPlaintextOutput is the response payload for GenerateDataKeyWithoutPlaintext.
type GenerateDataKeyWithoutPlaintextOutput struct {
	KeyID          string `json:"KeyId"`
	CiphertextBlob []byte `json:"CiphertextBlob"`
}

// PutKeyPolicyInput is the request payload for PutKeyPolicy.
type PutKeyPolicyInput struct {
	KeyID      string `json:"KeyId"`
	PolicyName string `json:"PolicyName"`
	Policy     string `json:"Policy"`
}

// GetKeyPolicyInput is the request payload for GetKeyPolicy.
type GetKeyPolicyInput struct {
	KeyID      string `json:"KeyId"`
	PolicyName string `json:"PolicyName"`
}

// GetKeyPolicyOutput is the response payload for GetKeyPolicy.
type GetKeyPolicyOutput struct {
	Policy     string `json:"Policy"`
	PolicyName string `json:"PolicyName"`
}

// SignInput is the request payload for Sign.
type SignInput struct {
	KeyID            string   `json:"KeyId"`
	MessageType      string   `json:"MessageType,omitempty"`
	SigningAlgorithm string   `json:"SigningAlgorithm"`
	Message          []byte   `json:"Message"`
	GrantTokens      []string `json:"GrantTokens,omitempty"`
}

// SignOutput is the response payload for Sign.
type SignOutput struct {
	KeyID            string `json:"KeyId"`
	SigningAlgorithm string `json:"SigningAlgorithm"`
	Signature        []byte `json:"Signature"`
}

// VerifyInput is the request payload for Verify.
type VerifyInput struct {
	KeyID            string   `json:"KeyId"`
	MessageType      string   `json:"MessageType,omitempty"`
	SigningAlgorithm string   `json:"SigningAlgorithm"`
	Message          []byte   `json:"Message"`
	Signature        []byte   `json:"Signature"`
	GrantTokens      []string `json:"GrantTokens,omitempty"`
}

// VerifyOutput is the response payload for Verify.
type VerifyOutput struct {
	KeyID            string `json:"KeyId"`
	SigningAlgorithm string `json:"SigningAlgorithm"`
	SignatureValid   bool   `json:"SignatureValid"`
}

// GetPublicKeyInput is the request payload for GetPublicKey.
type GetPublicKeyInput struct {
	// KeyId identifies the asymmetric KMS key whose public key to retrieve.
	KeyID string `json:"KeyId"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// GetPublicKeyOutput is the response payload for GetPublicKey.
type GetPublicKeyOutput struct {
	// KeyId is the ID of the asymmetric KMS key.
	KeyID string `json:"KeyId"`
	// PublicKey is the DER-encoded public key.
	PublicKey []byte `json:"PublicKey"`
	// KeySpec is the key spec of the key.
	KeySpec string `json:"KeySpec"`
	// KeyUsage is the intended use of the key.
	KeyUsage string `json:"KeyUsage"`
	// SigningAlgorithms lists the signing algorithms supported by this key.
	SigningAlgorithms []string `json:"SigningAlgorithms,omitempty"`
	// EncryptionAlgorithms lists the encryption algorithms (empty for sign keys).
	EncryptionAlgorithms []string `json:"EncryptionAlgorithms,omitempty"`
	// KeyAgreementAlgorithms lists the key agreement algorithms (e.g. ECDH).
	KeyAgreementAlgorithms []string `json:"KeyAgreementAlgorithms,omitempty"`
}

// ImportKeyMaterialInput is the request payload for ImportKeyMaterial.
type ImportKeyMaterialInput struct {
	KeyID           string  `json:"KeyId"`
	ExpirationModel string  `json:"ExpirationModel,omitempty"`
	KeyMaterial     []byte  `json:"EncryptedKeyMaterial"`
	ValidTo         float64 `json:"ValidTo,omitempty"`
}

// ImportKeyMaterialOutput is the response payload for ImportKeyMaterial.
// The real output also declares KeyMaterialId, part of the multi-key-material
// rotation feature (aws-sdk-go-v2 kms@v1.55.4 api_op_ImportKeyMaterial.go);
// this backend has no concept of multiple key-material generations per key,
// so only KeyId — always present on the real wire response — is echoed back.
type ImportKeyMaterialOutput struct {
	KeyID string `json:"KeyId"`
}

// DeleteImportedKeyMaterialInput is the request payload for DeleteImportedKeyMaterial.
type DeleteImportedKeyMaterialInput struct {
	// KeyId identifies the EXTERNAL-origin key whose material should be deleted.
	KeyID string `json:"KeyId"`
}

// DeleteImportedKeyMaterialOutput is the response payload for DeleteImportedKeyMaterial.
// See ImportKeyMaterialOutput for why KeyMaterialId is not modeled.
type DeleteImportedKeyMaterialOutput struct {
	KeyID string `json:"KeyId"`
}

// GetParametersForImportInput is the request payload for GetParametersForImport.
type GetParametersForImportInput struct {
	KeyID             string `json:"KeyId"`
	WrappingAlgorithm string `json:"WrappingAlgorithm,omitempty"`
	WrappingKeySpec   string `json:"WrappingKeySpec,omitempty"`
}

// GetParametersForImportOutput is the response payload for GetParametersForImport.
type GetParametersForImportOutput struct {
	KeyID             string  `json:"KeyId"`
	ImportToken       []byte  `json:"ImportToken"`
	PublicKey         []byte  `json:"PublicKey"`
	ParametersValidTo float64 `json:"ParametersValidTo"`
}

// ListKeyPoliciesInput is the request payload for ListKeyPolicies.
type ListKeyPoliciesInput struct {
	Limit  *int32 `json:"Limit,omitempty"`
	KeyID  string `json:"KeyId"`
	Marker string `json:"Marker,omitempty"`
}

// ListKeyPoliciesOutput is the response payload for ListKeyPolicies.
type ListKeyPoliciesOutput struct {
	NextMarker  string   `json:"NextMarker,omitempty"`
	PolicyNames []string `json:"PolicyNames"`
	Truncated   bool     `json:"Truncated"`
}

// KeyRotationEntry describes one key rotation event.
type KeyRotationEntry struct {
	KeyID        string  `json:"KeyId,omitempty"`
	RotationType string  `json:"RotationType,omitempty"`
	RotationDate float64 `json:"RotationDate"`
}

// ListKeyRotationsInput is the request payload for ListKeyRotations.
type ListKeyRotationsInput struct {
	Limit  *int32 `json:"Limit,omitempty"`
	KeyID  string `json:"KeyId"`
	Marker string `json:"Marker,omitempty"`
}

// ListKeyRotationsOutput is the response payload for ListKeyRotations.
type ListKeyRotationsOutput struct {
	NextMarker string             `json:"NextMarker,omitempty"`
	Rotations  []KeyRotationEntry `json:"Rotations"`
	Truncated  bool               `json:"Truncated"`
}

// ReplicateKeyInput is the request payload for ReplicateKey.
type ReplicateKeyInput struct {
	KeyID         string `json:"KeyId"`
	ReplicaRegion string `json:"ReplicaRegion"`
	Description   string `json:"Description,omitempty"`
	// Policy is the key policy to attach to the replica. If omitted, KMS
	// attaches the default key policy (matches CreateKey's Policy field).
	// The key policy is NOT a shared property of multi-region keys: the
	// replica gets its own independent policy rather than inheriting the
	// primary's.
	Policy string `json:"Policy,omitempty"`
	// Tags are optional tags to apply to the replica key.
	Tags                           []Tag `json:"Tags,omitempty"`
	BypassPolicyLockoutSafetyCheck bool  `json:"BypassPolicyLockoutSafetyCheck,omitempty"`
}

// ReplicateKeyOutput is the response payload for ReplicateKey.
type ReplicateKeyOutput struct {
	ReplicaKeyMetadata KeyMetadata `json:"ReplicaKeyMetadata"`
	ReplicaPolicy      string      `json:"ReplicaPolicy,omitempty"`
	ReplicaTags        []Tag       `json:"ReplicaTags,omitempty"`
}

// RotateKeyOnDemandInput is the request payload for RotateKeyOnDemand.
type RotateKeyOnDemandInput struct {
	KeyID string `json:"KeyId"`
}

// RotateKeyOnDemandOutput is the response payload for RotateKeyOnDemand.
type RotateKeyOnDemandOutput struct {
	KeyID string `json:"KeyId"`
}

// UpdateCustomKeyStoreInput is the request payload for UpdateCustomKeyStore.
type UpdateCustomKeyStoreInput struct {
	CustomKeyStoreID      string `json:"CustomKeyStoreId"`
	NewCustomKeyStoreName string `json:"NewCustomKeyStoreName,omitempty"`
}

// UpdateKeyDescriptionInput is the request payload for UpdateKeyDescription.
type UpdateKeyDescriptionInput struct {
	KeyID       string `json:"KeyId"`
	Description string `json:"Description"`
}

// UpdatePrimaryRegionInput is the request payload for UpdatePrimaryRegion.
type UpdatePrimaryRegionInput struct {
	KeyID         string `json:"KeyId"`
	PrimaryRegion string `json:"PrimaryRegion"`
}

// KeyUsageGenerateMac is the key usage for HMAC keys.
const KeyUsageGenerateMac = "GENERATE_VERIFY_MAC"

// KeyUsageKeyAgreement is the key usage for ECDH key agreement keys.
const KeyUsageKeyAgreement = "KEY_AGREEMENT"

// maxDescriptionLength is the maximum length of a KMS key description (matches AWS).
const maxDescriptionLength = 8192

// maxTagsPerKey is the maximum number of tags that can be applied to a KMS key (matches AWS).
const maxTagsPerKey = 50

// maxAliasNameLength is the maximum byte length of an alias name including the "alias/" prefix.
const maxAliasNameLength = 256

// rotationTypeAWSKMS is the rotation type for automatic AWS-managed rotations.
const rotationTypeAWSKMS = "AWS_KMS"

// rotationTypeImported is the rotation type for customer-triggered (on-demand) rotations.
const rotationTypeImported = "IMPORTED"

// encryptionAlgorithmSymmetric is the encryption algorithm string for symmetric (AES-256-GCM) keys.
const encryptionAlgorithmSymmetric = "SYMMETRIC_DEFAULT"

// encryptionAlgorithmRSAOAEP is the encryption algorithm string for RSA OAEP-SHA-256 keys.
const encryptionAlgorithmRSAOAEP = "RSAES_OAEP_SHA_256"

// ConnectionStateConnected indicates a custom key store is connected.
const ConnectionStateConnected = "CONNECTED"

// ConnectionStateDisconnected indicates a custom key store is disconnected.
const ConnectionStateDisconnected = "DISCONNECTED"

// CustomKeyStore represents an AWS KMS custom key store entry.
type CustomKeyStore struct {
	CustomKeyStoreID   string  `json:"CustomKeyStoreId"`
	CustomKeyStoreName string  `json:"CustomKeyStoreName"`
	ConnectionState    string  `json:"ConnectionState"`
	CustomKeyStoreType string  `json:"CustomKeyStoreType"`
	CreationDate       float64 `json:"CreationDate"`
}

// CreateCustomKeyStoreInput is the request payload for CreateCustomKeyStore.
type CreateCustomKeyStoreInput struct {
	// CustomKeyStoreName is the name of the custom key store to create.
	CustomKeyStoreName string `json:"CustomKeyStoreName"`
	// CustomKeyStoreType is the type of custom key store (default AWS_CLOUDHSM).
	CustomKeyStoreType string `json:"CustomKeyStoreType,omitempty"`
}

// CreateCustomKeyStoreOutput is the response payload for CreateCustomKeyStore.
type CreateCustomKeyStoreOutput struct {
	// CustomKeyStoreId is the ID of the newly created custom key store.
	CustomKeyStoreID string `json:"CustomKeyStoreId"`
}

// DeleteCustomKeyStoreInput is the request payload for DeleteCustomKeyStore.
type DeleteCustomKeyStoreInput struct {
	// CustomKeyStoreId identifies the custom key store to delete.
	CustomKeyStoreID string `json:"CustomKeyStoreId"`
}

// DescribeCustomKeyStoresInput is the request payload for DescribeCustomKeyStores.
type DescribeCustomKeyStoresInput struct {
	// CustomKeyStoreId filters results to a single custom key store by ID.
	CustomKeyStoreID string `json:"CustomKeyStoreId,omitempty"`
	// CustomKeyStoreName filters results to a single custom key store by name.
	CustomKeyStoreName string `json:"CustomKeyStoreName,omitempty"`
	// Limit caps the number of results returned.
	Limit *int32 `json:"Limit,omitempty"`
	// Marker is the pagination cursor from a previous call.
	Marker string `json:"Marker,omitempty"`
}

// DescribeCustomKeyStoresOutput is the response payload for DescribeCustomKeyStores.
type DescribeCustomKeyStoresOutput struct {
	NextMarker      string           `json:"NextMarker,omitempty"`
	CustomKeyStores []CustomKeyStore `json:"CustomKeyStores"`
	Truncated       bool             `json:"Truncated"`
}

// ConnectCustomKeyStoreInput is the request payload for ConnectCustomKeyStore.
type ConnectCustomKeyStoreInput struct {
	// CustomKeyStoreId identifies the custom key store to connect.
	CustomKeyStoreID string `json:"CustomKeyStoreId"`
}

// DisconnectCustomKeyStoreInput is the request payload for DisconnectCustomKeyStore.
type DisconnectCustomKeyStoreInput struct {
	// CustomKeyStoreId identifies the custom key store to disconnect.
	CustomKeyStoreID string `json:"CustomKeyStoreId"`
}

// DeriveSharedSecretInput is the request payload for DeriveSharedSecret.
type DeriveSharedSecretInput struct {
	// KeyId is the ECC key (KEY_AGREEMENT usage) used to derive the shared secret.
	KeyID string `json:"KeyId"`
	// KeyAgreementAlgorithm is the key agreement algorithm (always ECDH).
	KeyAgreementAlgorithm string `json:"KeyAgreementAlgorithm"`
	// PublicKey is the DER-encoded public key of the other party.
	PublicKey []byte `json:"PublicKey"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// DeriveSharedSecretOutput is the response payload for DeriveSharedSecret.
type DeriveSharedSecretOutput struct {
	KeyID                 string `json:"KeyId"`
	KeyAgreementAlgorithm string `json:"KeyAgreementAlgorithm"`
	SharedSecret          []byte `json:"SharedSecret"`
}

// GenerateDataKeyPairInput is the request payload for GenerateDataKeyPair.
type GenerateDataKeyPairInput struct {
	// EncryptionContext is the optional encryption context for the wrapping key.
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
	// KeyId is the KMS symmetric key used to encrypt the private key.
	KeyID string `json:"KeyId"`
	// KeyPairSpec specifies the asymmetric key spec (e.g. RSA_2048, ECC_NIST_P256).
	KeyPairSpec string `json:"KeyPairSpec"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// GenerateDataKeyPairOutput is the response payload for GenerateDataKeyPair.
type GenerateDataKeyPairOutput struct {
	// KeyId is the ARN of the wrapping KMS key.
	KeyID string `json:"KeyId"`
	// KeyPairSpec is the key pair spec used.
	KeyPairSpec string `json:"KeyPairSpec"`
	// PrivateKeyCiphertextBlob is the DER-encoded private key encrypted under KeyId.
	PrivateKeyCiphertextBlob []byte `json:"PrivateKeyCiphertextBlob"`
	// PrivateKeyPlaintext is the DER-encoded PKCS#8 private key.
	PrivateKeyPlaintext []byte `json:"PrivateKeyPlaintext"`
	// PublicKey is the DER-encoded SubjectPublicKeyInfo public key.
	PublicKey []byte `json:"PublicKey"`
}

// GenerateDataKeyPairWithoutPlaintextInput is the request payload for GenerateDataKeyPairWithoutPlaintext.
type GenerateDataKeyPairWithoutPlaintextInput struct {
	// EncryptionContext is the optional encryption context for the wrapping key.
	EncryptionContext map[string]string `json:"EncryptionContext,omitempty"`
	// KeyId is the KMS symmetric key used to encrypt the private key.
	KeyID string `json:"KeyId"`
	// KeyPairSpec specifies the asymmetric key spec (e.g. RSA_2048, ECC_NIST_P256).
	KeyPairSpec string `json:"KeyPairSpec"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// GenerateDataKeyPairWithoutPlaintextOutput is the response payload for GenerateDataKeyPairWithoutPlaintext.
type GenerateDataKeyPairWithoutPlaintextOutput struct {
	// KeyId is the ARN of the wrapping KMS key.
	KeyID string `json:"KeyId"`
	// KeyPairSpec is the key pair spec used.
	KeyPairSpec string `json:"KeyPairSpec"`
	// PrivateKeyCiphertextBlob is the DER-encoded private key encrypted under KeyId.
	PrivateKeyCiphertextBlob []byte `json:"PrivateKeyCiphertextBlob"`
	// PublicKey is the DER-encoded SubjectPublicKeyInfo public key.
	PublicKey []byte `json:"PublicKey"`
}

// GenerateMacInput is the request payload for GenerateMac.
type GenerateMacInput struct {
	// KeyId is the HMAC KMS key used to generate the MAC.
	KeyID string `json:"KeyId"`
	// MacAlgorithm specifies the MAC algorithm (e.g. HMAC_SHA_256).
	MacAlgorithm string `json:"MacAlgorithm"`
	// Message is the data over which to compute the MAC.
	Message []byte `json:"Message"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// GenerateMacOutput is the response payload for GenerateMac.
type GenerateMacOutput struct {
	KeyID        string `json:"KeyId"`
	MacAlgorithm string `json:"MacAlgorithm"`
	Mac          []byte `json:"Mac"`
}

// GenerateRandomInput is the request payload for GenerateRandom.
type GenerateRandomInput struct {
	// NumberOfBytes specifies how many random bytes to generate (default 32, max 1024).
	NumberOfBytes *int32 `json:"NumberOfBytes,omitempty"`
}

// GenerateRandomOutput is the response payload for GenerateRandom.
type GenerateRandomOutput struct {
	// Plaintext contains the generated random bytes.
	Plaintext []byte `json:"Plaintext"`
}

// VerifyMacInput is the request payload for VerifyMac.
type VerifyMacInput struct {
	// KeyId is the HMAC KMS key used to verify the MAC.
	KeyID string `json:"KeyId"`
	// MacAlgorithm specifies the MAC algorithm (e.g. HMAC_SHA_256).
	MacAlgorithm string `json:"MacAlgorithm"`
	// Message is the data over which to verify the MAC.
	Message []byte `json:"Message"`
	// Mac is the MAC tag to verify.
	Mac []byte `json:"Mac"`
	// GrantTokens is an optional list of grant tokens used to authorize the operation.
	GrantTokens []string `json:"GrantTokens,omitempty"`
}

// VerifyMacOutput is the response payload for VerifyMac.
type VerifyMacOutput struct {
	KeyID        string `json:"KeyId"`
	MacAlgorithm string `json:"MacAlgorithm"`
	MacValid     bool   `json:"MacValid"`
}

// KeyLastUsageData contains information about the last successful cryptographic operation on a KMS key.
type KeyLastUsageData struct {
	CloudTrailEventID string  `json:"CloudTrailEventId,omitempty"`
	KmsRequestID      string  `json:"KmsRequestId,omitempty"`
	Operation         string  `json:"Operation,omitempty"`
	Timestamp         float64 `json:"Timestamp,omitempty"`
}

// GetKeyLastUsageInput is the request payload for GetKeyLastUsage.
type GetKeyLastUsageInput struct {
	KeyID string `json:"KeyId"` //nolint:tagliatelle // AWS API uses KeyId
}

// GetKeyLastUsageOutput is the response payload for GetKeyLastUsage.
type GetKeyLastUsageOutput struct {
	KeyLastUsage      *KeyLastUsageData `json:"KeyLastUsage,omitempty"`
	KeyID             string            `json:"KeyId,omitempty"`
	KeyCreationDate   float64           `json:"KeyCreationDate,omitempty"`
	TrackingStartDate float64           `json:"TrackingStartDate,omitempty"`
}
