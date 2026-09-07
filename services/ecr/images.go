package ecr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// deleteByDigestLocked removes an image by digest, deletes all tag bindings for
// that digest, and returns true if the image was found.
func deleteByDigestLocked(
	images *store.Table[Image],
	repoTags map[string]string,
	repositoryName, digest string,
) bool {
	key := imageTableKey(repositoryName, digest)
	if !images.Has(key) {
		return false
	}

	// Remove all tag bindings for this digest.
	for tag, d := range repoTags {
		if d == digest {
			delete(repoTags, tag)
		}
	}

	images.Delete(key)

	return true
}

// deleteByTagLocked removes a tag binding, clears the image's tag field if it
// matches, and falls back to a linear scan for legacy images. Returns true if found.
func deleteByTagLocked(
	images *store.Table[Image],
	repoTags map[string]string,
	repositoryName, tag string,
) bool {
	digest, ok := repoTags[tag]
	if !ok {
		return false
	}

	delete(repoTags, tag)
	if img, exists := images.Get(imageTableKey(repositoryName, digest)); exists {
		img.ImageID.ImageTag = ""
	}

	return true
}

// BatchDeleteImage deletes the specified images from a repository.
// When deleting by digest, all associated tags are removed and the image is deleted.
// When deleting by tag, only that tag binding is removed; the image remains accessible
// by digest (it becomes untagged if it had no other tags).
func (b *InMemoryBackend) BatchDeleteImage(ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
	imageIDs []ImageIdentifier,
) ([]ImageIdentifier, []ImageFailure, error) {
	b.mu.Lock("BatchDeleteImage")
	defer b.mu.Unlock()

	if !b.repos.Has(repositoryName) {
		return nil, nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	deleted := make([]ImageIdentifier, 0, len(imageIDs))
	failures := make([]ImageFailure, 0, len(imageIDs))

	repoTags := b.tagIndex[repositoryName]

	for _, id := range imageIDs {
		var found bool

		if id.ImageDigest != "" {
			found = deleteByDigestLocked(b.images, repoTags, repositoryName, id.ImageDigest)
			if found {
				b.clearDigestTagsLocked(repositoryName, id.ImageDigest)
			}
		} else if id.ImageTag != "" {
			// Snapshot the digest before deletion so we can update the reverse index.
			oldDigest := repoTags[id.ImageTag]
			found = deleteByTagLocked(b.images, repoTags, repositoryName, id.ImageTag)
			if found && oldDigest != "" {
				b.removeDigestTagLocked(repositoryName, oldDigest, id.ImageTag)
			}
		}

		if found {
			deleted = append(deleted, id)
		} else {
			failures = append(failures, ImageFailure{
				ImageID:       id,
				FailureCode:   "ImageNotFound",
				FailureReason: "requested image not found",
			})
		}
	}

	return deleted, failures, nil
}

// BatchGetImage retrieves details for the specified images. Fetching an
// image's manifest this way is how a client pulls it, so this also stamps
// lastRecordedPullTime (surfaced via DescribeImages) on every found image.
func (b *InMemoryBackend) BatchGetImage(ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
	imageIDs []ImageIdentifier,
) ([]Image, []ImageFailure, error) {
	b.mu.Lock("BatchGetImage")
	defer b.mu.Unlock()

	if !b.repos.Has(repositoryName) {
		return nil, nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	imgs := make([]Image, 0, len(imageIDs))
	failures := make([]ImageFailure, 0, len(imageIDs))

	repoTagIdx := b.tagIndex[repositoryName]

	for _, id := range imageIDs {
		img, ok := findImageLocked(b.images, b.imagesByRepo, repositoryName, repoTagIdx, id)
		if ok {
			img.LastRecordedPullTime = time.Now()

			cp := *img
			// Preserve requested tag in imageId for the response.
			if id.ImageTag != "" {
				cp.ImageID.ImageTag = id.ImageTag
			}
			imgs = append(imgs, cp)
		} else {
			failures = append(failures, ImageFailure{
				ImageID:       id,
				FailureCode:   "ImageNotFound",
				FailureReason: "requested image not found",
			})
		}
	}

	return imgs, failures, nil
}

// buildDigestTagsLocked builds a reverse digest→[]tag map from a tag index.
// Ranging over a nil map is safe in Go, so no nil check is needed.
// tagMatchesAnyExclusionFilter reports whether tag is exempted from immutability
// by any of the configured exclusion filters.
// AWS supports filterType "WILDCARD" with patterns like "v*" or exact names.
func tagMatchesAnyExclusionFilter(tag string, filters []ImageTagMutabilityExclusionFilter) bool {
	for _, f := range filters {
		switch strings.ToUpper(f.FilterType) {
		case "WILDCARD":
			if wildcardMatch(f.Filter, tag) {
				return true
			}
		default:
			if tag == f.Filter {
				return true
			}
		}
	}

	return false
}

func buildDigestTagsLocked(repoTagIdx map[string]string) map[string][]string {
	digestTags := make(map[string][]string)
	for tag, digest := range repoTagIdx {
		digestTags[digest] = append(digestTags[digest], tag)
	}

	return digestTags
}

// addDigestTagLocked records tag→digest in both tagIndex and digestTagsIndex.
// Caller must hold the write lock.
func (b *InMemoryBackend) addDigestTagLocked(repo, digest, tag string) {
	if b.digestTagsIndex[repo] == nil {
		b.digestTagsIndex[repo] = make(map[string][]string)
	}
	b.digestTagsIndex[repo][digest] = append(b.digestTagsIndex[repo][digest], tag)
}

// removeDigestTagLocked removes a single tag from digestTagsIndex[repo][digest].
// Caller must hold the write lock.
func (b *InMemoryBackend) removeDigestTagLocked(repo, digest, tag string) {
	if b.digestTagsIndex[repo] == nil {
		return
	}
	tags := b.digestTagsIndex[repo][digest]
	for i, t := range tags {
		if t == tag {
			b.digestTagsIndex[repo][digest] = append(tags[:i], tags[i+1:]...)

			return
		}
	}
}

// clearDigestTagsLocked removes all tag entries for a digest (used on image delete by digest).
// Caller must hold the write lock.
func (b *InMemoryBackend) clearDigestTagsLocked(repo, digest string) {
	if b.digestTagsIndex[repo] != nil {
		delete(b.digestTagsIndex[repo], digest)
	}
}

// DescribeImages returns image details for a repository.
func (b *InMemoryBackend) DescribeImages(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
	imageIDs []ImageIdentifier,
) ([]Image, error) {
	b.mu.RLock("DescribeImages")
	defer b.mu.RUnlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	repoImages := b.imagesByRepo.Get(repositoryName)
	repoTagIdx := b.tagIndex[repositoryName]
	digestTags := b.digestTagsIndex[repositoryName]

	annotate := func(img Image) Image {
		tags := digestTags[img.ImageDigest]
		if len(tags) == 0 && img.ImageID.ImageTag != "" {
			tags = []string{img.ImageID.ImageTag}
		}
		// Sort for stable output.
		sort.Strings(tags)
		img.Tags = tags

		// imageScanFindingsSummary/imageScanStatus are derived fresh from the
		// scan store on every DescribeImages call (transient annotation, not
		// persisted on Image); an image that was never scanned simply has
		// neither field set, matching AWS (both are optional).
		if findings, ok := b.imageScanFindings.Get(findingsTableKey(repositoryName, img.ImageDigest)); ok {
			img.ScanFindingsSummary = &ImageScanFindingsSummaryInfo{
				FindingSeverityCounts:        findings.FindingSeverityCounts,
				ImageScanCompletedAt:         findings.ImageScanCompletedAt,
				VulnerabilitySourceUpdatedAt: findings.VulnerabilitySourceUpdatedAt,
			}
			img.ScanStatus = &ImageScanStatusInfo{
				Status:      findings.Status,
				Description: findings.Description,
			}
		}

		return img
	}

	out := make([]Image, 0, len(repoImages))
	if len(imageIDs) == 0 {
		for _, img := range repoImages {
			out = append(out, annotate(*img))
		}
	} else {
		for _, id := range imageIDs {
			img, ok := findImageLocked(b.images, b.imagesByRepo, repositoryName, repoTagIdx, id)
			if !ok {
				return nil, fmt.Errorf("%w: image not found", ErrImageNotFound)
			}

			out = append(out, annotate(*img))
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ImageDigest < out[j].ImageDigest })

	return out, nil
}

// imageTagsLocked returns the tags for an image, falling back to the legacy
// per-image tag field if the digest is not in the tag index.
func imageTagsLocked(img *Image, digestTags map[string][]string) []string {
	tags := digestTags[img.ImageDigest]
	if len(tags) == 0 && img.ImageID.ImageTag != "" {
		// Legacy: tag stored on image directly (before tagIndex).
		return []string{img.ImageID.ImageTag}
	}

	return tags
}

// passesTagFilter reports whether an image with the given tagged state should be
// included given tagStatusFilter ("TAGGED", "UNTAGGED", or anything else for all).
func passesTagFilter(isTagged bool, tagStatusFilter string) bool {
	switch tagStatusFilter {
	case "TAGGED":
		return isTagged
	case "UNTAGGED":
		return !isTagged
	default:
		return true
	}
}

// passesImageStatusFilter reports whether an image with the given status should be
// included given imageStatusFilter. An empty filter means "not specified", which per
// AWS docs (DescribeImages/ListImages) means only ACTIVE images are returned; "ANY"
// disables the status filter entirely.
func passesImageStatusFilter(status, imageStatusFilter string) bool {
	switch imageStatusFilter {
	case "", imageStatusActive:
		return status == imageStatusActive
	case "ANY":
		return true
	default:
		return status == imageStatusFilter
	}
}

// ListImages lists image identifiers for a repository.
// tagStatusFilter controls which images to return: "TAGGED", "UNTAGGED", or "ANY" (default).
// imageStatusFilter controls status filtering: "" defaults to ACTIVE-only per AWS docs,
// "ANY" disables it, or an explicit status ("ACTIVE"/"ARCHIVED"/"ACTIVATING").
func (b *InMemoryBackend) ListImages(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName, tagStatusFilter, imageStatusFilter string,
) ([]ImageIdentifier, error) {
	b.mu.RLock("ListImages")
	defer b.mu.RUnlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	repoTagIdx := b.tagIndex[repositoryName]

	// Build a reverse map: digest → []tag from tagIndex.
	digestTags := buildDigestTagsLocked(repoTagIdx)

	repoImages := b.imagesByRepo.Get(repositoryName)

	out := make([]ImageIdentifier, 0, len(repoImages))
	for _, img := range repoImages {
		tags := imageTagsLocked(img, digestTags)
		isTagged := len(tags) > 0

		if !passesTagFilter(isTagged, tagStatusFilter) || !passesImageStatusFilter(img.ImageStatus, imageStatusFilter) {
			continue
		}

		if isTagged {
			// Emit one identifier per tag (matching AWS ListImages behaviour).
			for _, tag := range tags {
				out = append(out, ImageIdentifier{ImageDigest: img.ImageDigest, ImageTag: tag})
			}
		} else {
			out = append(out, ImageIdentifier{ImageDigest: img.ImageDigest})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ImageDigest == out[j].ImageDigest {
			return out[i].ImageTag < out[j].ImageTag
		}

		return out[i].ImageDigest < out[j].ImageDigest
	})

	return out, nil
}

// ListImageReferrers lists image referrers for a subject image.
func (b *InMemoryBackend) ListImageReferrers(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
	_ ImageIdentifier,
) ([]ImageReferrer, error) {
	b.mu.RLock("ListImageReferrers")
	defer b.mu.RUnlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	// Real ListImageReferrers declares no not-found error for the subject
	// (only RepositoryNotFoundException/UnableToListUpstreamImageReferrersException/etc,
	// per deserializeOpErrorListImageReferrers) -- an unknown subject digest
	// returns an empty list, not ImageNotFoundException.
	return []ImageReferrer{}, nil
}

// retagImageLocked moves a tag to a new digest: if the tag already maps to a
// different digest, it clears the old image's ImageTag field so it becomes untagged.
func retagImageLocked(
	images *store.Table[Image],
	repoTags map[string]string,
	repositoryName, tag, newDigest string,
) {
	oldDigest, has := repoTags[tag]
	if !has || oldDigest == newDigest {
		return
	}

	if oldImg, exists := images.Get(imageTableKey(repositoryName, oldDigest)); exists {
		if oldImg.ImageID.ImageTag == tag {
			oldImg.ImageID.ImageTag = ""
		}
	}
}

// PutImage creates or replaces an image manifest.
// Digest is computed from the manifest content only (not the tag), matching AWS behaviour.
// When the repository imageTagMutability is IMMUTABLE, attempts to retag an
// existing image (same tag, different digest) are rejected with ErrImageTagAlreadyExists.
// When pushing to a MUTABLE repo with a tag that already exists on a different digest,
// the tag is moved to the new digest (the old image becomes untagged).
// normalizeImageFields fills in any zero-valued metadata fields on an image
// using the repository name and account ID.
func normalizeImageFields(image *Image, repositoryName, accountID string) {
	if image.ImageID.ImageDigest == "" {
		image.ImageID.ImageDigest = image.ImageDigest
	}

	if image.RepositoryName == "" {
		image.RepositoryName = repositoryName
	}

	if image.RegistryID == "" {
		image.RegistryID = accountID
	}

	if image.ImagePushedAt.IsZero() {
		image.ImagePushedAt = time.Now()
	}

	if image.ImageStatus == "" {
		image.ImageStatus = imageStatusActive
	}
}

func (b *InMemoryBackend) PutImage(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
	image Image,
) (*Image, error) {
	b.mu.Lock("PutImage")
	defer b.mu.Unlock()

	repo, ok := b.repos.Get(repositoryName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	// Compute digest from manifest only (not tag), matching AWS ECR behaviour.
	if image.ImageDigest == "" {
		sum := sha256.Sum256([]byte(image.ImageManifest))
		image.ImageDigest = "sha256:" + hex.EncodeToString(sum[:])
	}

	tag := image.ImageID.ImageTag

	// Ensure per-repo tagIndex exists.
	if b.tagIndex[repositoryName] == nil {
		b.tagIndex[repositoryName] = make(map[string]string)
	}
	repoTags := b.tagIndex[repositoryName]

	if existingDigest, has := repoTags[tag]; tag != "" && has {
		switch {
		case existingDigest == image.ImageDigest:
			// ImageAlreadyExistsException: re-pushing a manifest that is
			// already registered under this exact tag is a complete no-op
			// push ("no changes to the manifest or image tag after the last
			// push", per the real API doc) and is rejected -- independent of
			// repository tag mutability, which instead governs the
			// different-digest retag case below. Confirmed against the moto
			// ECR emulator's put_image logic (existing_images_with_matching_manifest
			// + tag already in image.image_tags raises this same exception).
			return nil, fmt.Errorf("%w: image already exists with tag %s in repository %s",
				ErrImageAlreadyExists, tag, repositoryName)

		case repo.ImageTagMutability == mutabilityImmutable:
			// IMMUTABLE enforcement: reject retagging to a different digest
			// unless the tag matches an exclusion filter (which exempts
			// specific tag patterns from immutability).
			if !tagMatchesAnyExclusionFilter(tag, repo.ImageTagMutabilityExclusionFilters) {
				return nil, fmt.Errorf("%w: tag %s already exists in immutable repository %s",
					ErrImageTagAlreadyExists, tag, repositoryName)
			}
		}
	}

	// If tag already points to a different digest, untag the old image.
	if tag != "" {
		retagImageLocked(b.images, repoTags, repositoryName, tag, image.ImageDigest)
	}

	normalizeImageFields(&image, repositoryName, b.accountID)

	stored := image
	b.images.Put(&stored)

	// Update tag index and keep digestTagsIndex in sync.
	if tag != "" {
		oldDigest, hadTag := repoTags[tag]
		repoTags[tag] = image.ImageDigest
		if hadTag && oldDigest != image.ImageDigest {
			b.removeDigestTagLocked(repositoryName, oldDigest, tag)
		}
		if !hadTag || oldDigest != image.ImageDigest {
			b.addDigestTagLocked(repositoryName, image.ImageDigest, tag)
		}
	}

	ret := stored

	return &ret, nil
}

// PutImageTagMutability updates per-repository tag mutability.
func (b *InMemoryBackend) PutImageTagMutability(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName, imageTagMutability string,
	exclusionFilters []ImageTagMutabilityExclusionFilter,
) (*Repository, error) {
	b.mu.Lock("PutImageTagMutability")
	defer b.mu.Unlock()

	repo, ok := b.repos.Get(repositoryName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	if imageTagMutability == "" {
		imageTagMutability = mutabilityMutable
	}

	repo.ImageTagMutability = imageTagMutability
	repo.ImageTagMutabilityExclusionFilters = append(
		[]ImageTagMutabilityExclusionFilter(nil),
		exclusionFilters...)
	cp := *repo

	return &cp, nil
}

// UpdateImageStorageClass updates the storage class for an image.
func (b *InMemoryBackend) UpdateImageStorageClass(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
	imageID ImageIdentifier,
	target string,
) (*ImageStorageClassResult, error) {
	b.mu.Lock("UpdateImageStorageClass")
	defer b.mu.Unlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	img, ok := findImageLocked(b.images, b.imagesByRepo, repositoryName, b.tagIndex[repositoryName], imageID)
	if !ok {
		return nil, fmt.Errorf("%w: image not found", ErrImageNotFound)
	}

	if target == storageClassArchive {
		img.StorageClass = target
		img.ImageStatus = imageStatusArchived
		img.LastArchivedAt = time.Now()
	} else {
		img.StorageClass = "STANDARD"
		img.ImageStatus = imageStatusActive
		img.LastActivatedAt = time.Now()
	}

	return &ImageStorageClassResult{
		ImageID:        img.ImageID,
		ImageStatus:    img.ImageStatus,
		RegistryID:     b.accountID,
		RepositoryName: repositoryName,
	}, nil
}

// findImageLocked looks up an image by digest or tag within repositoryName.
// tagIdx is the per-repository tag→digest index; it may be nil for older callers.
// imagesByRepo is only consulted by the fallback linear scan (images that
// predate the tag index).
func findImageLocked(
	images *store.Table[Image],
	imagesByRepo *store.Index[Image],
	repositoryName string,
	tagIdx map[string]string,
	id ImageIdentifier,
) (*Image, bool) {
	if id.ImageDigest != "" {
		return images.Get(imageTableKey(repositoryName, id.ImageDigest))
	}

	if id.ImageTag != "" {
		// Fast path via tag index.
		if tagIdx != nil {
			if digest, ok := tagIdx[id.ImageTag]; ok {
				return images.Get(imageTableKey(repositoryName, digest))
			}
		}
		// Fallback: linear scan for images without tag index entry.
		for _, img := range imagesByRepo.Get(repositoryName) {
			if img.ImageID.ImageTag == id.ImageTag {
				return img, true
			}
		}
	}

	return nil, false
}

// AddImageInternal seeds an image directly into the backend for testing.
// repositoryName is the repository to add the image to; img is the image to add.
func (b *InMemoryBackend) AddImageInternal(repositoryName string, img Image) {
	b.mu.Lock("AddImageInternal")
	defer b.mu.Unlock()

	cp := img
	// RepositoryName is part of the store.Table composite key (see
	// imageTableKey); normalize it to the map-scoping repositoryName argument
	// exactly as PutImage's normalizeImageFields does, so a test-seeded image
	// is found by repo-scoped lookups the same way a PutImage-created one is.
	cp.RepositoryName = repositoryName
	if cp.ImageStatus == "" {
		cp.ImageStatus = imageStatusActive
	}

	b.images.Put(&cp)

	if img.ImageID.ImageTag != "" {
		if b.tagIndex[repositoryName] == nil {
			b.tagIndex[repositoryName] = make(map[string]string)
		}
		b.tagIndex[repositoryName][img.ImageID.ImageTag] = img.ImageDigest
	}
}
