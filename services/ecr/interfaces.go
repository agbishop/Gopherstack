package ecr

import "context"

// Backend defines the interface for ECR control-plane operations.
// InMemoryBackend implements this interface; alternative backends (e.g. one
// that delegates to a real Docker daemon registry, or a test double) can
// implement it too, keeping the Handler backend-agnostic.
//
// Data-plane methods take a context.Context as their first argument so that
// per-request metadata (notably the AWS region, via pkgs/awsmeta) can flow
// through to ARN construction and region-scoped behaviour.
type Backend interface {
	// CreateRepository creates a new ECR repository and returns its metadata.
	// Returns ErrRepositoryAlreadyExists if a repository with that name already
	// exists, or ErrInvalidRepositoryName when name is empty.
	// imageTagMutability defaults to "MUTABLE" when empty.
	// encryptionType defaults to "AES256" when empty; kmsKey is only used with "KMS".
	CreateRepository(
		ctx context.Context,
		name, imageTagMutability string,
		scanOnPush bool,
		encryptionType, kmsKey string,
	) (*Repository, error)

	// DescribeRepositories returns repository metadata, optionally filtered by
	// the provided names. Passing an empty slice returns all repositories.
	// Returns ErrRepositoryNotFound if any requested name does not exist.
	DescribeRepositories(ctx context.Context, names []string) ([]Repository, error)

	// DeleteRepository removes the named repository and returns its metadata.
	// Returns ErrRepositoryNotFound if the repository does not exist.
	// Returns ErrRepositoryNotEmpty if the repository contains images and
	// force is false.
	DeleteRepository(ctx context.Context, name string, force bool) (*Repository, error)

	// ProxyEndpoint returns the registry endpoint embedded in repository URIs
	// and returned by GetAuthorizationToken.
	ProxyEndpoint() string

	// SetEndpoint updates the registry endpoint used in new repository URIs.
	// It should be called once the server's listening address is known.
	SetEndpoint(endpoint string)

	// BatchCheckLayerAvailability checks the availability of image layers.
	BatchCheckLayerAvailability(
		ctx context.Context,
		repositoryName string,
		layerDigests []string,
	) ([]LayerAvailability, []LayerFailure, error)

	// BatchDeleteImage deletes specified images from a repository.
	BatchDeleteImage(
		ctx context.Context,
		repositoryName string,
		imageIDs []ImageIdentifier,
	) ([]ImageIdentifier, []ImageFailure, error)

	// BatchGetImage gets details for specified images.
	BatchGetImage(
		ctx context.Context,
		repositoryName string,
		imageIDs []ImageIdentifier,
	) ([]Image, []ImageFailure, error)

	// DescribeImages returns image details for a repository.
	DescribeImages(ctx context.Context, repositoryName string, imageIDs []ImageIdentifier) ([]Image, error)

	// BatchGetRepositoryScanningConfiguration gets scanning config for repositories.
	BatchGetRepositoryScanningConfiguration(
		ctx context.Context,
		repositoryNames []string,
	) ([]RepositoryScanningConfiguration, []RepositoryScanningConfigurationFailure, error)

	// CompleteLayerUpload completes the upload of an image layer.
	CompleteLayerUpload(
		ctx context.Context,
		repositoryName, uploadID string,
		layerDigests []string,
	) (*CompleteLayerUploadResult, error)

	// GetDownloadURLForLayer resolves a download URL for a repository layer.
	GetDownloadURLForLayer(ctx context.Context, repositoryName, layerDigest string) (string, error)

	// InitiateLayerUpload starts a layer upload session.
	InitiateLayerUpload(ctx context.Context, repositoryName string) (*LayerUploadInitiation, error)

	// UploadLayerPart records uploaded bytes for an existing session.
	UploadLayerPart(
		ctx context.Context,
		repositoryName, uploadID string,
		firstByte, lastByte int64,
		blob []byte,
	) (*LayerUploadPartResult, error)

	// CreatePullThroughCacheRule creates a pull-through cache rule.
	CreatePullThroughCacheRule(
		ctx context.Context,
		prefix, upstreamURL, credentialArn, upstreamRegistry, customRoleArn, upstreamRepositoryPrefix string,
	) (*PullThroughCacheRule, error)

	// DescribePullThroughCacheRules lists pull-through cache rules.
	DescribePullThroughCacheRules(ctx context.Context, prefixes []string) ([]PullThroughCacheRule, error)

	// CreateRepositoryCreationTemplate creates a repository creation template.
	CreateRepositoryCreationTemplate(
		ctx context.Context,
		req *RepositoryCreationTemplate,
	) (*RepositoryCreationTemplate, error)

	// DeleteRepositoryCreationTemplate deletes a repository creation template.
	DeleteRepositoryCreationTemplate(ctx context.Context, prefix string) (*RepositoryCreationTemplate, error)

	// DescribeRepositoryCreationTemplates lists repository creation templates.
	DescribeRepositoryCreationTemplates(ctx context.Context, prefixes []string) ([]RepositoryCreationTemplate, error)

	// DeleteLifecyclePolicy deletes the lifecycle policy for a repository.
	DeleteLifecyclePolicy(ctx context.Context, repositoryName string) (*LifecyclePolicyResult, error)

	// GetLifecyclePolicy returns the lifecycle policy for a repository.
	GetLifecyclePolicy(ctx context.Context, repositoryName string) (*LifecyclePolicyResult, error)

	// GetLifecyclePolicyPreview returns the current lifecycle policy preview.
	GetLifecyclePolicyPreview(ctx context.Context, repositoryName string) (*LifecyclePolicyPreviewResult, error)

	// PutLifecyclePolicy creates or replaces the lifecycle policy for a repository.
	PutLifecyclePolicy(ctx context.Context, repositoryName, policyText string) (*LifecyclePolicyResult, error)

	// StartLifecyclePolicyPreview starts or refreshes a lifecycle policy preview.
	StartLifecyclePolicyPreview(
		ctx context.Context,
		repositoryName, policyText string,
	) (*LifecyclePolicyPreviewResult, error)

	// DeletePullThroughCacheRule deletes a pull-through cache rule by prefix.
	DeletePullThroughCacheRule(ctx context.Context, prefix string) (*PullThroughCacheRule, error)

	// UpdatePullThroughCacheRule updates a pull-through cache rule by prefix.
	UpdatePullThroughCacheRule(
		ctx context.Context,
		prefix, credentialArn, customRoleArn string,
	) (*PullThroughCacheRule, error)

	// ValidatePullThroughCacheRule validates a pull-through cache rule by prefix.
	ValidatePullThroughCacheRule(ctx context.Context, prefix string) (*ValidatePullThroughCacheRuleResult, error)

	// DeleteRegistryPolicy deletes the registry-level policy.
	DeleteRegistryPolicy(ctx context.Context) (*RegistryPolicyResult, error)

	// DescribeRegistry returns registry-wide metadata.
	DescribeRegistry(ctx context.Context) (*RegistryDescription, error)

	// GetRegistryPolicy returns the registry-level policy.
	GetRegistryPolicy(ctx context.Context) (*RegistryPolicyResult, error)

	// GetRegistryScanningConfiguration returns the registry scanning configuration.
	GetRegistryScanningConfiguration(ctx context.Context) (*RegistryScanningSettings, error)

	// PutRegistryPolicy creates or replaces the registry-level IAM policy.
	PutRegistryPolicy(ctx context.Context, policyText string) (*RegistryPolicyResult, error)

	// PutRegistryScanningConfiguration updates the registry scanning configuration.
	PutRegistryScanningConfiguration(
		ctx context.Context,
		settings *RegistryScanningSettings,
	) (*RegistryScanningSettings, error)

	// PutReplicationConfiguration updates the registry replication configuration.
	PutReplicationConfiguration(ctx context.Context, cfg *ReplicationConfig) (*ReplicationConfig, error)

	// GetRepositoryPolicy returns the repository-level policy.
	GetRepositoryPolicy(ctx context.Context, repositoryName string) (*RepositoryPolicyResult, error)

	// SetRepositoryPolicy creates or replaces the repository-level IAM policy.
	SetRepositoryPolicy(ctx context.Context, repositoryName, policyText string) (*RepositoryPolicyResult, error)

	// DeleteRepositoryPolicy deletes the repository-level policy.
	DeleteRepositoryPolicy(ctx context.Context, repositoryName string) (*RepositoryPolicyResult, error)

	// GetSigningConfiguration returns the current registry signing configuration.
	GetSigningConfiguration(ctx context.Context) (*SigningSettings, error)

	// PutSigningConfiguration updates the registry signing configuration.
	PutSigningConfiguration(ctx context.Context, settings *SigningSettings) (*SigningSettings, error)

	// DeleteSigningConfiguration removes the registry signing configuration.
	DeleteSigningConfiguration(ctx context.Context) (*SigningSettings, error)

	// DescribeImageSigningStatus returns signing status for an image.
	DescribeImageSigningStatus(
		ctx context.Context,
		repositoryName string,
		imageID ImageIdentifier,
	) (*ImageSigningStatusResult, error)

	// DescribeImageScanFindings returns scan findings for an image.
	DescribeImageScanFindings(
		ctx context.Context,
		repositoryName string,
		imageID ImageIdentifier,
		maxResults int,
		nextToken string,
	) (*ImageScanFindingsResult, string, error)

	// StartImageScan starts an image scan and returns the scan status.
	StartImageScan(ctx context.Context, repositoryName string, imageID ImageIdentifier) (*ImageScanStartResult, error)

	// ListImages lists image identifiers for a repository, filtered by tag and image status.
	// tagStatusFilter can be "TAGGED", "UNTAGGED", or "" / "ANY" for all images.
	// imageStatusFilter can be "ACTIVE"/"ARCHIVED"/"ACTIVATING", or "" (defaults to ACTIVE)/"ANY".
	ListImages(
		ctx context.Context, repositoryName, tagStatusFilter, imageStatusFilter string,
	) ([]ImageIdentifier, error)

	// ListImageReferrers lists image referrers for a subject image.
	ListImageReferrers(ctx context.Context, repositoryName string, subject ImageIdentifier) ([]ImageReferrer, error)

	// PutImage creates or replaces an image manifest.
	PutImage(ctx context.Context, repositoryName string, image Image) (*Image, error)

	// PutImageScanningConfiguration updates per-repository scan-on-push config.
	PutImageScanningConfiguration(
		ctx context.Context,
		repositoryName string,
		scanOnPush bool,
	) (*RepositoryScanningConfiguration, error)

	// PutImageTagMutability updates per-repository tag mutability.
	PutImageTagMutability(
		ctx context.Context,
		repositoryName, imageTagMutability string,
		exclusionFilters []ImageTagMutabilityExclusionFilter,
	) (*Repository, error)

	// DescribeImageReplicationStatus returns the current replication status for an image.
	DescribeImageReplicationStatus(
		ctx context.Context,
		repositoryName string,
		imageID ImageIdentifier,
	) (*ImageReplicationStatusResult, error)

	// UpdateImageStorageClass updates the storage class for an image.
	UpdateImageStorageClass(
		ctx context.Context,
		repositoryName string,
		imageID ImageIdentifier,
		target string,
	) (*ImageStorageClassResult, error)

	// GetAccountSetting returns a registry account setting.
	GetAccountSetting(ctx context.Context, name string) (string, error)

	// PutAccountSetting updates a registry account setting.
	PutAccountSetting(ctx context.Context, name, value string) (string, error)

	// RegisterPullTimeUpdateExclusion creates a pull time update exclusion.
	RegisterPullTimeUpdateExclusion(ctx context.Context, principalArn string) (*PullTimeUpdateExclusion, error)

	// DeregisterPullTimeUpdateExclusion deletes a pull time update exclusion.
	DeregisterPullTimeUpdateExclusion(ctx context.Context, principalArn string) (*PullTimeUpdateExclusion, error)

	// ListPullTimeUpdateExclusions lists pull time update exclusions.
	ListPullTimeUpdateExclusions(ctx context.Context) ([]PullTimeUpdateExclusion, error)

	// UpdateRepositoryCreationTemplate updates a repository creation template.
	UpdateRepositoryCreationTemplate(
		ctx context.Context,
		req *RepositoryCreationTemplate,
	) (*RepositoryCreationTemplate, error)

	// TagResource associates tags with a resource identified by ARN.
	TagResource(ctx context.Context, resourceArn string, tags map[string]string) error

	// UntagResource removes tags from a resource identified by ARN.
	UntagResource(ctx context.Context, resourceArn string, tagKeys []string) error

	// ListTagsForResource returns all tags for a resource identified by ARN.
	ListTagsForResource(ctx context.Context, resourceArn string) (map[string]string, error)

	// Reset clears all backend state.
	Reset()

	// Region returns the AWS region this backend is configured for.
	Region() string

	// AccountID returns the AWS account ID associated with this registry.
	AccountID() string
}

// Snapshottable is an optional interface that a Backend may implement to
// support state serialisation and restoration (e.g. for --persist mode).
// Backends that do not implement it are silently skipped during snapshot/restore.
type Snapshottable interface {
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}
