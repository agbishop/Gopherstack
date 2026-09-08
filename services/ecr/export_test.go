package ecr

import "time"

// CreateRepoInternal creates a minimal repository entry directly for testing.
// Use this alongside AddImageInternal when a test needs images in a repo that
// also satisfies the repo-existence check.
func (b *InMemoryBackend) CreateRepoInternal(repositoryName string) {
	b.mu.Lock("CreateRepoInternal")
	defer b.mu.Unlock()

	if !b.repos.Has(repositoryName) {
		b.repos.Put(&Repository{
			RepositoryName:     repositoryName,
			RegistryID:         b.accountID,
			ImageTagMutability: "MUTABLE",
		})
	}
}

// ImageCount returns the total number of images stored across all repositories.
// Used only in tests to verify backend state without going through the HTTP handler.
func (b *InMemoryBackend) ImageCount() int {
	b.mu.RLock("ImageCount")
	defer b.mu.RUnlock()

	return b.images.Len()
}

// PullThroughCacheRuleCount returns the number of pull-through cache rules stored.
func (b *InMemoryBackend) PullThroughCacheRuleCount() int {
	b.mu.RLock("PullThroughCacheRuleCount")
	defer b.mu.RUnlock()

	return b.pullThroughCacheRules.Len()
}

// RepositoryCreationTemplateCount returns the number of repository creation templates stored.
func (b *InMemoryBackend) RepositoryCreationTemplateCount() int {
	b.mu.RLock("RepositoryCreationTemplateCount")
	defer b.mu.RUnlock()

	return b.repositoryCreationTemplates.Len()
}

// RepositoryCount returns the number of repositories.
// Used only in tests to verify backend state without going through the HTTP handler.
func (b *InMemoryBackend) RepositoryCount() int {
	b.mu.RLock("RepositoryCount")
	defer b.mu.RUnlock()

	return b.repos.Len()
}

// LifecyclePolicyCount returns the number of lifecycle policies stored.
func (b *InMemoryBackend) LifecyclePolicyCount() int {
	b.mu.RLock("LifecyclePolicyCount")
	defer b.mu.RUnlock()

	return b.lifecyclePolicies.Len()
}

// UploadedLayerCount returns the total number of uploaded layers.
// Used only in tests to verify backend state without going through the HTTP handler.
func (b *InMemoryBackend) UploadedLayerCount() int {
	b.mu.RLock("UploadedLayerCount")
	defer b.mu.RUnlock()

	total := 0
	for _, layers := range b.uploadedLayers {
		total += len(layers)
	}

	return total
}

// LayerUploadCount returns the number of in-progress layer upload sessions.
// Used only in tests to verify that DeleteRepository cleans up layer uploads.
func (b *InMemoryBackend) LayerUploadCount() int {
	b.mu.RLock("LayerUploadCount")
	defer b.mu.RUnlock()

	return len(b.layerUploads)
}

// RepoUploadIndexCount returns the total number of repo->uploadID entries
// tracked across all repositories. Used only in tests to verify that Reset
// clears this index the same way Restore does.
func (b *InMemoryBackend) RepoUploadIndexCount() int {
	b.mu.RLock("RepoUploadIndexCount")
	defer b.mu.RUnlock()

	total := 0
	for _, ids := range b.repoUploadIndex {
		total += len(ids)
	}

	return total
}

// AgeAllLayerUploadsForTest backdates every in-progress layer upload's
// CreatedAt by the given duration so abandoned-upload pruning can be exercised
// without waiting for the real TTL.
func (b *InMemoryBackend) AgeAllLayerUploadsForTest(d time.Duration) {
	b.mu.Lock("AgeAllLayerUploadsForTest")
	defer b.mu.Unlock()

	for _, upload := range b.layerUploads {
		upload.CreatedAt = upload.CreatedAt.Add(-d)
	}
}

// LayerUploadTTLForTest exposes the abandoned-upload TTL for tests.
const LayerUploadTTLForTest = layerUploadTTL

// TagIndexCount returns the total number of tag→digest entries across all repos.
func (b *InMemoryBackend) TagIndexCount() int {
	b.mu.RLock("TagIndexCount")
	defer b.mu.RUnlock()

	total := 0
	for _, idx := range b.tagIndex {
		total += len(idx)
	}

	return total
}

// RepoTagCount returns the number of tag→digest entries for a specific repository.
func (b *InMemoryBackend) RepoTagCount(repositoryName string) int {
	b.mu.RLock("RepoTagCount")
	defer b.mu.RUnlock()

	return len(b.tagIndex[repositoryName])
}

// TagDigest returns the digest a tag resolves to, or "" if not found.
func (b *InMemoryBackend) TagDigest(repositoryName, tag string) string {
	b.mu.RLock("TagDigest")
	defer b.mu.RUnlock()

	if idx, ok := b.tagIndex[repositoryName]; ok {
		return idx[tag]
	}

	return ""
}

// RepoImageCount returns the number of images stored for a single repository.
func (b *InMemoryBackend) RepoImageCount(repositoryName string) int {
	b.mu.RLock("RepoImageCount")
	defer b.mu.RUnlock()

	return len(b.imagesByRepo.Get(repositoryName))
}

// HasImageDigest reports whether an image with the given digest exists in the repo.
func (b *InMemoryBackend) HasImageDigest(repositoryName, digest string) bool {
	b.mu.RLock("HasImageDigest")
	defer b.mu.RUnlock()

	return b.images.Has(imageTableKey(repositoryName, digest))
}

// LifecycleLastEvaluatedForTest returns the recorded lifecycle evaluation time
// for a repository, or the zero time if none.
func (b *InMemoryBackend) LifecycleLastEvaluatedForTest(repositoryName string) time.Time {
	b.mu.RLock("LifecycleLastEvaluatedForTest")
	defer b.mu.RUnlock()

	return b.lifecycleLastEvaluated[repositoryName]
}

// SetReplicationSettleDelayForTest sets how long replication is modeled to take.
// A freshly pushed image reports IN_PROGRESS until this delay elapses.
func (b *InMemoryBackend) SetReplicationSettleDelayForTest(d time.Duration) {
	b.mu.Lock("SetReplicationSettleDelayForTest")
	defer b.mu.Unlock()

	b.replicationSettleDelay = d
}

// AgeImageForTest backdates an image's ImagePushedAt by d so age-based lifecycle
// expiry and replication settling can be exercised without real elapsed time.
func (b *InMemoryBackend) AgeImageForTest(repositoryName, digest string, d time.Duration) {
	b.mu.Lock("AgeImageForTest")
	defer b.mu.Unlock()

	if img, ok := b.images.Get(imageTableKey(repositoryName, digest)); ok {
		img.ImagePushedAt = img.ImagePushedAt.Add(-d)
	}
}
