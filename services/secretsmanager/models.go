// Package secretsmanager provides a mock AWS Secrets Manager implementation.
package secretsmanager

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	// nanoToSeconds converts nanoseconds to seconds.
	nanoToSeconds = 1e9
	// MockAccountID is the mock AWS account ID.
	MockAccountID = config.DefaultAccountID
	// MockRegion is the mock AWS region.
	MockRegion = config.DefaultRegion
	// StagingLabelCurrent is the staging label for the current secret version.
	StagingLabelCurrent = "AWSCURRENT"
	// StagingLabelPrevious is the staging label for the previous secret version.
	StagingLabelPrevious = "AWSPREVIOUS"
)

// SecretVersion represents a single version of a secret.
type SecretVersion struct {
	LastAccessedDate *float64 `json:"LastAccessedDate,omitempty"`
	VersionID        string   `json:"VersionId"`
	SecretString     string   `json:"SecretString,omitempty"`
	SecretBinary     []byte   `json:"SecretBinary,omitempty"`
	StagingLabels    []string `json:"VersionStages,omitempty"`
	KmsKeyIDs        []string `json:"KmsKeyIds,omitempty"`
	// Ciphertext, when non-nil, holds the KMS-encrypted secret value produced
	// by InMemoryBackend.sealVersion; SecretString and SecretBinary are then
	// both empty and the plaintext must be obtained via
	// InMemoryBackend.openVersion. Populated only when the backend has a
	// KMSEncryptor wired (see SetKMSEncryptor in kms.go) -- backends without
	// one continue to store SecretString/SecretBinary directly, exactly as
	// before KMS integration.
	Ciphertext []byte `json:"Ciphertext,omitempty"`
	// WasString records whether the plaintext sealed into Ciphertext came
	// from SecretString (true) or SecretBinary (false), so openVersion
	// restores it to the correct output field. Meaningless when Ciphertext
	// is nil.
	WasString   bool    `json:"WasStringValue,omitempty"`
	CreatedDate float64 `json:"CreatedDate"`
}

// Secret represents a stored secret including all versions.
type Secret struct {
	RotationRules         *RotationRulesType        `json:"-"`
	Tags                  *tags.Tags                `json:"Tags,omitempty"`
	DeletedDate           *float64                  `json:"DeletedDate,omitempty"`
	ScheduledDeletionDate *float64                  `json:"ScheduledDeletionDate,omitempty"`
	Versions              map[string]*SecretVersion `json:"-"`
	LastChangedDate       *float64                  `json:"-"`
	LastRotatedDate       *float64                  `json:"-"`
	LastAccessedDate      *float64                  `json:"-"`
	CreatedDate           *float64                  `json:"-"`
	Name                  string                    `json:"Name"`
	region                string
	// PrimaryRegion is the origin region of a replica secret, or "" for a
	// primary/standalone secret (whose own region is its primary region).
	// Set by upsertReplicaSecretLocked when mirroring a primary's current
	// version into a replica region's store.
	PrimaryRegion                  string                               `json:"-"`
	Description                    string                               `json:"Description,omitempty"`
	KmsKeyID                       string                               `json:"-"`
	RotationLambdaARN              string                               `json:"-"`
	ARN                            string                               `json:"ARN"`
	CurrentVersionID               string                               `json:"-"`
	Type                           string                               `json:"-"`
	ExternalSecretRotationRoleArn  string                               `json:"-"`
	ExternalSecretRotationMetadata []ExternalSecretRotationMetadataItem `json:"-"`
	RotationEnabled                bool                                 `json:"-"`
}

// ExternalSecretRotationMetadataItem is a key/value pair of managed
// external secret rotation metadata, specified by the third-party partner.
type ExternalSecretRotationMetadataItem struct {
	Key   string `json:"Key,omitempty"`
	Value string `json:"Value,omitempty"`
}

// RotationRulesType configures automatic secret rotation scheduling.
type RotationRulesType struct {
	// AutomaticallyAfterDays rotates the secret after this many days.
	AutomaticallyAfterDays *int64 `json:"AutomaticallyAfterDays,omitempty"`
	// Duration is an optional ISO-8601 duration window for rotation.
	Duration string `json:"Duration,omitempty"`
	// ScheduleExpression is an optional cron/rate expression for rotation scheduling.
	ScheduleExpression string `json:"ScheduleExpression,omitempty"`
}

// CreateSecretInput is the request payload for CreateSecret.
type CreateSecretInput struct {
	Name                        string          `json:"Name"`
	Description                 string          `json:"Description,omitempty"`
	SecretString                string          `json:"SecretString,omitempty"`
	ClientRequestToken          string          `json:"ClientRequestToken,omitempty"`
	KmsKeyID                    string          `json:"KmsKeyId,omitempty"`
	Region                      string          `json:"-"`
	Type                        string          `json:"Type,omitempty"`
	SecretBinary                []byte          `json:"SecretBinary,omitempty"`
	Tags                        []Tag           `json:"Tags,omitempty"`
	AddReplicaRegions           []ReplicaRegion `json:"AddReplicaRegions,omitempty"`
	ForceOverwriteReplicaSecret bool            `json:"ForceOverwriteReplicaSecret,omitempty"`
}

// Tag represents a key/value tag pair in the Secrets Manager wire format.
type Tag struct {
	// Key is the tag key.
	Key string `json:"Key"`
	// Value is the tag value.
	Value string `json:"Value"`
}

// CreateSecretOutput is the response payload for CreateSecret.
type CreateSecretOutput struct {
	// ARN is the full ARN of the created secret.
	ARN string `json:"ARN"`
	// Name is the name of the created secret.
	Name string `json:"Name"`
	// VersionId is the initial version UUID.
	VersionID string `json:"VersionId,omitempty"`
	// ReplicationStatus is the initial replication status for AddReplicaRegions.
	ReplicationStatus []ReplicationStatusType `json:"ReplicationStatus,omitempty"`
}

// GetSecretValueInput is the request payload for GetSecretValue.
type GetSecretValueInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
	// VersionId retrieves a specific version (default: AWSCURRENT).
	VersionID string `json:"VersionId,omitempty"`
	// VersionStage retrieves the version with this staging label (default: AWSCURRENT).
	VersionStage string `json:"VersionStage,omitempty"`
}

// GetSecretValueOutput is the response payload for GetSecretValue.
type GetSecretValueOutput struct {
	// LastAccessedDate is the Unix timestamp (day granularity) of the most recent access.
	LastAccessedDate *float64 `json:"LastAccessedDate,omitempty"`
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
	// VersionId is the UUID of the version returned.
	VersionID string `json:"VersionId"`
	// SecretString is the string value (when the secret stores a string).
	SecretString string `json:"SecretString,omitempty"`
	// SecretBinary is the binary value (when the secret stores binary data).
	SecretBinary []byte `json:"SecretBinary,omitempty"`
	// VersionStages are the staging labels attached to this version.
	VersionStages []string `json:"VersionStages,omitempty"`
	// CreatedDate is the Unix timestamp when this version was created.
	CreatedDate float64 `json:"CreatedDate"`
}

// PutSecretValueInput is the request payload for PutSecretValue.
type PutSecretValueInput struct {
	SecretID           string `json:"SecretId"`
	SecretString       string `json:"SecretString,omitempty"`
	ClientRequestToken string `json:"ClientRequestToken,omitempty"`
	SecretBinary       []byte `json:"SecretBinary,omitempty"`
	RotationToken      string `json:"RotationToken,omitempty"`
	// VersionStages are the staging labels to attach to the new version.
	// AWSCURRENT is always added; AWSPENDING is a common value during rotation.
	VersionStages []string `json:"VersionStages,omitempty"`
}

// PutSecretValueOutput is the response payload for PutSecretValue.
type PutSecretValueOutput struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
	// VersionId is the UUID of the new version.
	VersionID string `json:"VersionId"`
	// VersionStages are the staging labels attached to the new version.
	VersionStages []string `json:"VersionStages"`
}

// DeleteSecretInput is the request payload for DeleteSecret.
type DeleteSecretInput struct {
	// RecoveryWindowInDays is the number of days before the secret can be deleted.
	// Must be between 7 and 30 inclusive. Defaults to 30 when not set.
	RecoveryWindowInDays *int64 `json:"RecoveryWindowInDays,omitempty"`
	// SecretId is the name or ARN of the secret to delete.
	SecretID string `json:"SecretId"`
	// ForceDeleteWithoutRecovery deletes immediately when true.
	ForceDeleteWithoutRecovery bool `json:"ForceDeleteWithoutRecovery,omitempty"`
}

// DeleteSecretOutput is the response payload for DeleteSecret.
type DeleteSecretOutput struct {
	// ARN is the full ARN of the deleted secret.
	ARN string `json:"ARN"`
	// Name is the name of the deleted secret.
	Name string `json:"Name"`
	// DeletionDate is the Unix timestamp when the secret was deleted.
	DeletionDate float64 `json:"DeletionDate"`
}

// SecretListEntry is a brief secret descriptor used in ListSecrets.
type SecretListEntry struct {
	SecretVersionsToStages         map[string][]string                  `json:"SecretVersionsToStages,omitempty"`
	LastChangedDate                *float64                             `json:"LastChangedDate,omitempty"`
	LastAccessedDate               *float64                             `json:"LastAccessedDate,omitempty"`
	LastRotatedDate                *float64                             `json:"LastRotatedDate,omitempty"`
	CreatedDate                    *float64                             `json:"CreatedDate,omitempty"`
	NextRotationDate               *float64                             `json:"NextRotationDate,omitempty"`
	DeletedDate                    *float64                             `json:"DeletedDate,omitempty"`
	RotationRules                  *RotationRulesType                   `json:"RotationRules,omitempty"`
	ARN                            string                               `json:"ARN"`
	Name                           string                               `json:"Name"`
	Description                    string                               `json:"Description,omitempty"`
	KmsKeyID                       string                               `json:"KmsKeyId,omitempty"`
	RotationLambdaARN              string                               `json:"RotationLambdaARN,omitempty"`
	PrimaryRegion                  string                               `json:"PrimaryRegion,omitempty"`
	Type                           string                               `json:"Type,omitempty"`
	ExternalSecretRotationRoleArn  string                               `json:"ExternalSecretRotationRoleArn,omitempty"`
	Tags                           []Tag                                `json:"Tags,omitempty"`
	ExternalSecretRotationMetadata []ExternalSecretRotationMetadataItem `json:"ExternalSecretRotationMetadata,omitempty"`
	RotationEnabled                bool                                 `json:"RotationEnabled,omitempty"`
}

// ListSecretsInput is the request payload for ListSecrets.
type ListSecretsInput struct {
	MaxResults *int64 `json:"MaxResults,omitempty"`
	NextToken  string `json:"NextToken,omitempty"`
	SortOrder  string `json:"SortOrder,omitempty"` // "asc" or "desc"
	// SortBy selects the field secrets are ordered by: "name" (default),
	// "created-date", "last-changed-date", or "last-accessed-date".
	SortBy  string         `json:"SortBy,omitempty"`
	Filters []SecretFilter `json:"Filters,omitempty"`
	// IncludePlannedDeletion specifies whether to include secrets scheduled for
	// deletion. By default, secrets scheduled for deletion aren't included.
	// This is the real AWS wire field name (previously misnamed IncludeDeleted,
	// which real SDK clients never send, silently defeating the filter).
	IncludePlannedDeletion bool `json:"IncludePlannedDeletion,omitempty"`
}

// SecretFilter is a filter criterion for ListSecrets and BatchGetSecretValue.
type SecretFilter struct {
	// Key is the filter key (e.g. "name", "description", "tag-key", "tag-value", "all").
	Key string `json:"Key,omitempty"`
	// Values is the list of filter values.
	Values []string `json:"Values,omitempty"`
}

// ListSecretsOutput is the response payload for ListSecrets.
type ListSecretsOutput struct {
	NextToken  string            `json:"NextToken,omitempty"`
	SecretList []SecretListEntry `json:"SecretList"`
}

// DescribeSecretInput is the request payload for DescribeSecret.
type DescribeSecretInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
}

// DescribeSecretOutput is the response payload for DescribeSecret.
type DescribeSecretOutput struct {
	RotationRules                  *RotationRulesType                   `json:"RotationRules,omitempty"`
	DeletedDate                    *float64                             `json:"DeletedDate,omitempty"`
	CreatedDate                    *float64                             `json:"CreatedDate,omitempty"`
	LastChangedDate                *float64                             `json:"LastChangedDate,omitempty"`
	LastRotatedDate                *float64                             `json:"LastRotatedDate,omitempty"`
	LastAccessedDate               *float64                             `json:"LastAccessedDate,omitempty"`
	NextRotationDate               *float64                             `json:"NextRotationDate,omitempty"`
	VersionIDsToStages             map[string][]string                  `json:"VersionIdsToStages,omitempty"`
	Description                    string                               `json:"Description,omitempty"`
	Name                           string                               `json:"Name"`
	KmsKeyID                       string                               `json:"KmsKeyId,omitempty"`
	RotationLambdaARN              string                               `json:"RotationLambdaARN,omitempty"`
	PrimaryRegion                  string                               `json:"PrimaryRegion,omitempty"`
	ARN                            string                               `json:"ARN"`
	Type                           string                               `json:"Type,omitempty"`
	ExternalSecretRotationRoleArn  string                               `json:"ExternalSecretRotationRoleArn,omitempty"`
	Tags                           []Tag                                `json:"Tags,omitempty"`
	ReplicationStatus              []ReplicationStatusType              `json:"ReplicationStatus,omitempty"`
	ExternalSecretRotationMetadata []ExternalSecretRotationMetadataItem `json:"ExternalSecretRotationMetadata,omitempty"`
	RotationEnabled                bool                                 `json:"RotationEnabled"`
}

// UpdateSecretInput is the request payload for UpdateSecret.
type UpdateSecretInput struct {
	KmsKeyID           *string `json:"KmsKeyId,omitempty"`
	SecretID           string  `json:"SecretId"`
	Description        string  `json:"Description,omitempty"`
	SecretString       string  `json:"SecretString,omitempty"`
	ClientRequestToken string  `json:"ClientRequestToken,omitempty"`
	Type               string  `json:"Type,omitempty"`
	SecretBinary       []byte  `json:"SecretBinary,omitempty"`
}

// UpdateSecretOutput is the response payload for UpdateSecret.
type UpdateSecretOutput struct {
	// ARN is the full ARN of the updated secret.
	ARN string `json:"ARN"`
	// Name is the name of the updated secret.
	Name string `json:"Name"`
	// VersionId is the new version UUID when a value was also updated.
	VersionID string `json:"VersionId,omitempty"`
}

// RestoreSecretInput is the request payload for RestoreSecret.
type RestoreSecretInput struct {
	// SecretId is the name or ARN of the secret to restore.
	SecretID string `json:"SecretId"`
}

// RestoreSecretOutput is the response payload for RestoreSecret.
type RestoreSecretOutput struct {
	// ARN is the full ARN of the restored secret.
	ARN string `json:"ARN"`
	// Name is the name of the restored secret.
	Name string `json:"Name"`
}

// TagResourceInput is the request payload for TagResource.
type TagResourceInput struct {
	SecretID string `json:"SecretId"`
	Tags     []Tag  `json:"Tags"`
}

// UntagResourceInput is the request payload for UntagResource.
type UntagResourceInput struct {
	SecretID string   `json:"SecretId"`
	TagKeys  []string `json:"TagKeys"`
}

// RotateSecretInput is the request payload for RotateSecret.
type RotateSecretInput struct {
	SecretID           string             `json:"SecretId"`
	RotationLambdaARN  string             `json:"RotationLambdaARN,omitempty"`
	RotationRules      *RotationRulesType `json:"RotationRules,omitempty"`
	RotateImmediately  *bool              `json:"RotateImmediately,omitempty"`
	ClientRequestToken string             `json:"ClientRequestToken,omitempty"`
	// ExternalSecretRotationRoleArn is the role Secrets Manager assumes to rotate
	// a managed external secret.
	ExternalSecretRotationRoleArn string `json:"ExternalSecretRotationRoleArn,omitempty"`
	// ExternalSecretRotationMetadata is partner-specified rotation metadata.
	ExternalSecretRotationMetadata []ExternalSecretRotationMetadataItem `json:"ExternalSecretRotationMetadata,omitempty"`
}

// RotateSecretOutput is the response payload for RotateSecret.
type RotateSecretOutput struct {
	ARN       string `json:"ARN"`
	Name      string `json:"Name"`
	VersionID string `json:"VersionId,omitempty"`
}

// GetRandomPasswordInput is the request payload for GetRandomPassword.
type GetRandomPasswordInput struct {
	PasswordLength          *int64 `json:"PasswordLength,omitempty"`
	ExcludeCharacters       string `json:"ExcludeCharacters,omitempty"`
	ExcludeNumbers          bool   `json:"ExcludeNumbers,omitempty"`
	ExcludePunctuation      bool   `json:"ExcludePunctuation,omitempty"`
	ExcludeUppercase        bool   `json:"ExcludeUppercase,omitempty"`
	ExcludeLowercase        bool   `json:"ExcludeLowercase,omitempty"`
	IncludeSpace            bool   `json:"IncludeSpace,omitempty"`
	RequireEachIncludedType bool   `json:"RequireEachIncludedType,omitempty"`
}

// GetRandomPasswordOutput is the response payload for GetRandomPassword.
type GetRandomPasswordOutput struct {
	// RandomPassword is the generated password string.
	RandomPassword string `json:"RandomPassword"`
}

// ErrorResponse is the Secrets Manager JSON error response format.
type ErrorResponse struct {
	// Type is the error type string.
	Type string `json:"__type"`
	// Message is the human-readable error message.
	Message string `json:"message"`
}

// SecretVersionEntry is a brief descriptor for a single secret version, used in ListSecretVersionIDs.
type SecretVersionEntry struct {
	LastAccessedDate *float64 `json:"LastAccessedDate,omitempty"`
	VersionID        string   `json:"VersionId"`
	StagingLabels    []string `json:"VersionStages,omitempty"`
	KmsKeyIDs        []string `json:"KmsKeyIds,omitempty"`
	CreatedDate      float64  `json:"CreatedDate"`
}

// ListSecretVersionIDsInput is the request payload for ListSecretVersionIDs.
type ListSecretVersionIDsInput struct {
	MaxResults        *int64 `json:"MaxResults,omitempty"`
	SecretID          string `json:"SecretId"`
	NextToken         string `json:"NextToken,omitempty"`
	IncludeDeprecated bool   `json:"IncludeDeprecated,omitempty"`
}

// ListSecretVersionIDsOutput is the response payload for ListSecretVersionIDs.
type ListSecretVersionIDsOutput struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
	// NextToken is the pagination cursor for the next page.
	NextToken string `json:"NextToken,omitempty"`
	// Versions is the list of version entries.
	Versions []SecretVersionEntry `json:"Versions"`
}

// UnixTimeFloat converts a time value to a Unix timestamp float.
func UnixTimeFloat(t time.Time) float64 {
	return float64(t.UnixNano()) / nanoToSeconds
}

// BatchGetSecretValueFilter is a filter for BatchGetSecretValue.
type BatchGetSecretValueFilter struct {
	// Key is the filter key (e.g. "name", "tag-key", "tag-value", "description").
	Key string `json:"Key,omitempty"`
	// Values is the list of filter values.
	Values []string `json:"Values,omitempty"`
}

// BatchGetSecretValueInput is the request payload for BatchGetSecretValue.
type BatchGetSecretValueInput struct {
	// Filters specifies filter criteria for secrets to retrieve.
	Filters []BatchGetSecretValueFilter `json:"Filters,omitempty"`
	// MaxResults limits the number of results returned.
	MaxResults *int32 `json:"MaxResults,omitempty"`
	// NextToken is the pagination cursor from a previous call.
	NextToken string `json:"NextToken,omitempty"`
	// SecretIDList is the list of secret names or ARNs to retrieve.
	SecretIDList []string `json:"SecretIdList,omitempty"`
}

// SecretValueEntry is a single secret value entry returned by BatchGetSecretValue.
type SecretValueEntry struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
	// VersionID is the UUID of the version returned.
	VersionID string `json:"VersionId,omitempty"`
	// SecretString is the string value.
	SecretString string `json:"SecretString,omitempty"`
	// SecretBinary is the binary value.
	SecretBinary []byte `json:"SecretBinary,omitempty"`
	// VersionStages are the staging labels attached to this version.
	VersionStages []string `json:"VersionStages,omitempty"`
	// CreatedDate is the Unix timestamp when this version was created.
	CreatedDate float64 `json:"CreatedDate,omitempty"`
}

// APIErrorType is an error entry returned by BatchGetSecretValue for a single secret.
type APIErrorType struct {
	// ErrorCode is the AWS error type string.
	ErrorCode string `json:"ErrorCode"`
	// Message is the error message.
	Message string `json:"Message"`
	// SecretID is the identifier of the secret that caused the error.
	SecretID string `json:"SecretId"`
}

// BatchGetSecretValueOutput is the response payload for BatchGetSecretValue.
type BatchGetSecretValueOutput struct {
	// Errors contains per-secret errors.
	Errors []APIErrorType `json:"Errors,omitempty"`
	// NextToken is the pagination cursor for the next page.
	NextToken string `json:"NextToken,omitempty"`
	// SecretValues contains the successfully retrieved secret values.
	SecretValues []SecretValueEntry `json:"SecretValues"`
}

// CancelRotateSecretInput is the request payload for CancelRotateSecret.
type CancelRotateSecretInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
}

// CancelRotateSecretOutput is the response payload for CancelRotateSecret.
type CancelRotateSecretOutput struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
	// VersionID is the version ID affected.
	VersionID string `json:"VersionId,omitempty"`
}

// GetResourcePolicyInput is the request payload for GetResourcePolicy.
type GetResourcePolicyInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
}

// GetResourcePolicyOutput is the response payload for GetResourcePolicy.
type GetResourcePolicyOutput struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
	// ResourcePolicy is the resource-based policy document.
	ResourcePolicy string `json:"ResourcePolicy,omitempty"`
}

// PutResourcePolicyInput is the request payload for PutResourcePolicy.
type PutResourcePolicyInput struct {
	BlockPublicPolicy *bool  `json:"BlockPublicPolicy,omitempty"`
	SecretID          string `json:"SecretId"`
	ResourcePolicy    string `json:"ResourcePolicy"`
}

// PutResourcePolicyOutput is the response payload for PutResourcePolicy.
type PutResourcePolicyOutput struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
}

// DeleteResourcePolicyInput is the request payload for DeleteResourcePolicy.
type DeleteResourcePolicyInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
}

// DeleteResourcePolicyOutput is the response payload for DeleteResourcePolicy.
type DeleteResourcePolicyOutput struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
}

// ReplicaRegion specifies a target region and optional KMS key for secret replication.
type ReplicaRegion struct {
	// KmsKeyId is the ARN or alias of the KMS key to use for encryption.
	KmsKeyID string `json:"KmsKeyId,omitempty"`
	// Region is the AWS region to replicate to.
	Region string `json:"Region"`
}

// ReplicationStatusType describes the replication status for a replica region.
type ReplicationStatusType struct {
	// KmsKeyId is the ARN or alias of the KMS key used for encryption.
	KmsKeyID string `json:"KmsKeyId,omitempty"`
	// Region is the replica region.
	Region string `json:"Region,omitempty"`
	// Status is the replication status (e.g. "InSync", "Failed").
	Status string `json:"Status,omitempty"`
	// StatusMessage is an optional message describing the status.
	StatusMessage string `json:"StatusMessage,omitempty"`
}

// ReplicateSecretToRegionsInput is the request payload for ReplicateSecretToRegions.
type ReplicateSecretToRegionsInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
	// AddReplicaRegions is the list of regions to replicate to.
	AddReplicaRegions []ReplicaRegion `json:"AddReplicaRegions"`
	// ForceOverwriteReplicaSecret controls whether to overwrite existing replicas.
	ForceOverwriteReplicaSecret bool `json:"ForceOverwriteReplicaSecret,omitempty"`
}

// ReplicateSecretToRegionsOutput is the response payload for ReplicateSecretToRegions.
type ReplicateSecretToRegionsOutput struct {
	// ARN is the full ARN of the primary secret.
	ARN string `json:"ARN"`
	// ReplicationStatus contains the status for each replica region.
	ReplicationStatus []ReplicationStatusType `json:"ReplicationStatus"`
}

// RemoveRegionsFromReplicationInput is the request payload for RemoveRegionsFromReplication.
type RemoveRegionsFromReplicationInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
	// RemoveReplicaRegions is the list of regions to remove replicas from.
	RemoveReplicaRegions []string `json:"RemoveReplicaRegions"`
}

// RemoveRegionsFromReplicationOutput is the response payload for RemoveRegionsFromReplication.
type RemoveRegionsFromReplicationOutput struct {
	// ARN is the full ARN of the primary secret.
	ARN string `json:"ARN"`
	// ReplicationStatus contains the remaining replica statuses.
	ReplicationStatus []ReplicationStatusType `json:"ReplicationStatus"`
}

// StopReplicationToReplicaInput is the request payload for StopReplicationToReplica.
type StopReplicationToReplicaInput struct {
	// SecretId is the name or ARN of the secret replica to promote.
	SecretID string `json:"SecretId"`
}

// StopReplicationToReplicaOutput is the response payload for StopReplicationToReplica.
type StopReplicationToReplicaOutput struct {
	// ARN is the full ARN of the promoted replica secret.
	ARN string `json:"ARN"`
}

// UpdateSecretVersionStageInput is the request payload for UpdateSecretVersionStage.
type UpdateSecretVersionStageInput struct {
	// SecretId is the name or ARN of the secret.
	SecretID string `json:"SecretId"`
	// VersionStage is the staging label to add or move.
	VersionStage string `json:"VersionStage"`
	// MoveToVersionID is the version to move the label to.
	MoveToVersionID string `json:"MoveToVersionId,omitempty"`
	// RemoveFromVersionID is the version to remove the label from.
	RemoveFromVersionID string `json:"RemoveFromVersionId,omitempty"`
}

// UpdateSecretVersionStageOutput is the response payload for UpdateSecretVersionStage.
type UpdateSecretVersionStageOutput struct {
	// ARN is the full ARN of the secret.
	ARN string `json:"ARN"`
	// Name is the name of the secret.
	Name string `json:"Name"`
}

// ValidateResourcePolicyInput is the request payload for ValidateResourcePolicy.
type ValidateResourcePolicyInput struct {
	// SecretId is the optional name or ARN of the secret to validate the policy against.
	SecretID string `json:"SecretId,omitempty"`
	// ResourcePolicy is the resource-based policy document to validate.
	ResourcePolicy string `json:"ResourcePolicy"`
}

// PolicyValidationException represents a single policy validation failure.
type PolicyValidationException struct {
	// CheckName identifies the validation rule.
	CheckName string `json:"CheckName"`
	// ErrorMessage describes the issue.
	ErrorMessage string `json:"ErrorMessage"`
}

// ValidateResourcePolicyOutput is the response payload for ValidateResourcePolicy.
type ValidateResourcePolicyOutput struct {
	// ValidationErrors is the list of validation errors (empty when PolicyValidationPassed is true).
	ValidationErrors []PolicyValidationException `json:"ValidationErrors,omitempty"`
	// PolicyValidationPassed is true when no validation errors were found.
	PolicyValidationPassed bool `json:"PolicyValidationPassed"`
}
