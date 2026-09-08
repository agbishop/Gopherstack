package ecr

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrRepositoryNotFound is returned when a repository does not exist.
	ErrRepositoryNotFound = awserr.New("RepositoryNotFoundException", awserr.ErrNotFound)
	// ErrRepositoryAlreadyExists is returned when a repository already exists.
	ErrRepositoryAlreadyExists = awserr.New(
		"RepositoryAlreadyExistsException",
		awserr.ErrAlreadyExists,
	)
	// ErrRepositoryNotEmpty is returned when deleting a non-empty repository without the force flag.
	ErrRepositoryNotEmpty = awserr.New("RepositoryNotEmptyException", awserr.ErrConflict)
	// ErrImageTagAlreadyExists is returned when re-tagging an image in an IMMUTABLE repository.
	ErrImageTagAlreadyExists = awserr.New("ImageTagAlreadyExistsException", awserr.ErrConflict)
	// ErrInvalidRepositoryName is returned when the repository name is invalid.
	ErrInvalidRepositoryName = errors.New("InvalidParameterException")
	// ErrPullThroughCacheRuleNotFound is returned when a pull-through cache rule does not exist.
	ErrPullThroughCacheRuleNotFound = awserr.New(
		"PullThroughCacheRuleNotFoundException",
		awserr.ErrNotFound,
	)
	// ErrPullThroughCacheRuleAlreadyExists is returned when a pull-through cache rule already exists.
	ErrPullThroughCacheRuleAlreadyExists = awserr.New(
		"PullThroughCacheRuleAlreadyExistsException",
		awserr.ErrAlreadyExists,
	)
	// ErrLifecyclePolicyNotFound is returned when a lifecycle policy does not exist.
	ErrLifecyclePolicyNotFound = awserr.New("LifecyclePolicyNotFoundException", awserr.ErrNotFound)
	// ErrLifecyclePolicyPreviewNotFound is returned by GetLifecyclePolicyPreview when
	// no preview (dry run) has been started for the repository. Distinct from
	// ErrLifecyclePolicyNotFound: GetLifecyclePolicyPreview's own deserializeOpError
	// (ecr@v1.60.4 deserializers.go) recognizes LifecyclePolicyPreviewNotFoundException
	// ("There is no dry run for this repository."), not LifecyclePolicyNotFoundException.
	ErrLifecyclePolicyPreviewNotFound = awserr.New(
		"LifecyclePolicyPreviewNotFoundException",
		awserr.ErrNotFound,
	)
	// ErrRepositoryCreationTemplateNotFound is returned when a creation template does not exist.
	ErrRepositoryCreationTemplateNotFound = awserr.New(
		"TemplateNotFoundException",
		awserr.ErrNotFound,
	)
	// ErrRepositoryCreationTemplateAlreadyExists is returned when a creation template prefix already exists.
	ErrRepositoryCreationTemplateAlreadyExists = awserr.New(
		"TemplateAlreadyExistsException",
		awserr.ErrAlreadyExists,
	)
	// ErrRegistryPolicyNotFound is returned when the registry policy does not exist.
	ErrRegistryPolicyNotFound = awserr.New("RegistryPolicyNotFoundException", awserr.ErrNotFound)
	// ErrRepositoryPolicyNotFound is returned when a repository-level IAM policy does not exist.
	ErrRepositoryPolicyNotFound = awserr.New(
		"RepositoryPolicyNotFoundException",
		awserr.ErrNotFound,
	)
	// ErrImageNotFound is returned when a requested image does not exist in a repository.
	ErrImageNotFound = awserr.New("ImageNotFoundException", awserr.ErrNotFound)
	// ErrScanNotFoundException is returned when DescribeImageScanFindings is called
	// on an image that has never had a scan started.
	ErrScanNotFoundException = awserr.New("ScanNotFoundException", awserr.ErrNotFound)
)

var (
	// ErrLayerInaccessible is returned when a layer exists but is not accessible.
	ErrLayerInaccessible = awserr.New("LayerInaccessibleException", awserr.ErrNotFound)
	// ErrLayersNotFound is returned when requested layers do not exist in the repository.
	ErrLayersNotFound = awserr.New("LayersNotFoundException", awserr.ErrNotFound)
	// ErrLayerAlreadyExists is returned when CompleteLayerUpload is called with a
	// digest that has already been registered as an available layer in the
	// repository (matches AWS: "The image layer already exists in the associated
	// repository.").
	ErrLayerAlreadyExists = awserr.New("LayerAlreadyExistsException", awserr.ErrAlreadyExists)
	// ErrInvalidLayerPart is returned when an UploadLayerPart's first byte is not
	// consecutive to the last byte received by a previous part in the same
	// upload session (matches AWS InvalidLayerPartException).
	ErrInvalidLayerPart = awserr.New("InvalidLayerPartException", awserr.ErrInvalidParameter)
	// ErrImageDigestDoesNotMatch is returned when a caller-supplied imageDigest on
	// PutImage does not match the digest ECR computes from the image manifest.
	ErrImageDigestDoesNotMatch = awserr.New(
		"ImageDigestDoesNotMatchException",
		awserr.ErrInvalidParameter,
	)
	// ErrUploadNotFound is returned when CompleteLayerUpload or UploadLayerPart is
	// called with an uploadId that matches no live InitiateLayerUpload session for
	// the given repository (matches AWS UploadNotFoundException: "The upload could
	// not be found, or the specified upload ID is not valid for this repository.").
	ErrUploadNotFound = awserr.New("UploadNotFoundException", awserr.ErrNotFound)
	// ErrEmptyUpload is returned when CompleteLayerUpload is called on a live
	// upload session that never received any UploadLayerPart data (matches AWS
	// EmptyUploadException: "The specified layer upload does not contain any
	// layer parts.").
	ErrEmptyUpload = awserr.New("EmptyUploadException", awserr.ErrInvalidParameter)
	// ErrLayerPartTooSmall is returned when CompleteLayerUpload finds a non-final
	// uploaded part smaller than the 5MiB minimum (matches AWS
	// LayerPartTooSmallException: "Layer parts must be at least 5 MiB in size.").
	ErrLayerPartTooSmall = awserr.New("LayerPartTooSmallException", awserr.ErrInvalidParameter)
	// ErrImageAlreadyExists is returned when PutImage re-pushes a manifest that is
	// already registered under the exact same tag -- a complete no-op push (matches
	// AWS ImageAlreadyExistsException: "The specified image has already been
	// pushed, and there were no changes to the manifest or image tag after the
	// last push."). Independent of repository tag mutability, which instead
	// governs the distinct ImageTagAlreadyExistsException case (retagging to a
	// different digest).
	ErrImageAlreadyExists = awserr.New("ImageAlreadyExistsException", awserr.ErrConflict)
)

// ErrLayerDigestMismatch is returned when the provided digest does not match the uploaded bytes.
var ErrLayerDigestMismatch = awserr.New("InvalidLayerException", awserr.ErrInvalidParameter)
