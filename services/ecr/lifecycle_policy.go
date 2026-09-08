package ecr

import (
	"context"
	"fmt"
)

// DeleteLifecyclePolicy deletes the lifecycle policy for a repository.
func (b *InMemoryBackend) DeleteLifecyclePolicy(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
) (*LifecyclePolicyResult, error) {
	b.mu.Lock("DeleteLifecyclePolicy")
	defer b.mu.Unlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	entry, ok := b.lifecyclePolicies.Get(repositoryName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrLifecyclePolicyNotFound, repositoryName)
	}

	policyText := entry.PolicyText
	lastEvaluated := b.lifecycleLastEvaluated[repositoryName]
	b.lifecyclePolicies.Delete(repositoryName)
	delete(b.lifecycleLastEvaluated, repositoryName)

	return &LifecyclePolicyResult{
		LifecyclePolicyText: policyText,
		LastEvaluatedAt:     lastEvaluated,
		RepositoryName:      repositoryName,
		RegistryID:          b.accountID,
	}, nil
}

// GetLifecyclePolicy returns the lifecycle policy for a repository.
func (b *InMemoryBackend) GetLifecyclePolicy(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
) (*LifecyclePolicyResult, error) {
	b.mu.RLock("GetLifecyclePolicy")
	defer b.mu.RUnlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	entry, ok := b.lifecyclePolicies.Get(repositoryName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrLifecyclePolicyNotFound, repositoryName)
	}

	return &LifecyclePolicyResult{
		LifecyclePolicyText: entry.PolicyText,
		LastEvaluatedAt:     b.lifecycleLastEvaluated[repositoryName],
		RepositoryName:      repositoryName,
		RegistryID:          b.accountID,
	}, nil
}

// GetLifecyclePolicyPreview returns the current lifecycle policy preview.
func (b *InMemoryBackend) GetLifecyclePolicyPreview(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName string,
) (*LifecyclePolicyPreviewResult, error) {
	b.mu.RLock("GetLifecyclePolicyPreview")
	defer b.mu.RUnlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	preview, ok := b.lifecyclePolicyPreviews.Get(repositoryName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrLifecyclePolicyPreviewNotFound, repositoryName)
	}

	cp := *preview
	cp.PreviewResults = append([]LifecyclePolicyPreviewEntry(nil), preview.PreviewResults...)

	return &cp, nil
}

// PutLifecyclePolicy creates or replaces the lifecycle policy for a repository.
func (b *InMemoryBackend) PutLifecyclePolicy(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName, policyText string,
) (*LifecyclePolicyResult, error) {
	b.mu.Lock("PutLifecyclePolicy")
	defer b.mu.Unlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	b.lifecyclePolicies.Put(&lifecyclePolicyEntry{RepositoryName: repositoryName, PolicyText: policyText})

	// AWS evaluates a newly applied lifecycle policy right away; matched images
	// are expired and deleted rather than merely stored.
	b.applyLifecyclePolicyLocked(repositoryName)

	return &LifecyclePolicyResult{
		LifecyclePolicyText: policyText,
		LastEvaluatedAt:     b.lifecycleLastEvaluated[repositoryName],
		RepositoryName:      repositoryName,
		RegistryID:          b.accountID,
	}, nil
}

// StartLifecyclePolicyPreview starts or refreshes a lifecycle policy preview.
func (b *InMemoryBackend) StartLifecyclePolicyPreview(
	ctx context.Context, //nolint:revive // existing issue.
	repositoryName, policyText string,
) (*LifecyclePolicyPreviewResult, error) {
	b.mu.Lock("StartLifecyclePolicyPreview")
	defer b.mu.Unlock()

	if !b.repos.Has(repositoryName) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repositoryName)
	}

	if policyText == "" {
		if entry, ok := b.lifecyclePolicies.Get(repositoryName); ok {
			policyText = entry.PolicyText
		}
	}

	expired := evaluateLifecyclePolicy(
		policyText, b.imagesByRepo.Get(repositoryName), b.digestTagsIndex[repositoryName])

	preview := &LifecyclePolicyPreviewResult{
		LifecyclePolicyText: policyText,
		PreviewResults:      expired,
		RepositoryName:      repositoryName,
		RegistryID:          b.accountID,
		Status:              scanStatusComplete,
	}
	b.lifecyclePolicyPreviews.Put(preview)

	cp := *preview
	cp.PreviewResults = append([]LifecyclePolicyPreviewEntry(nil), preview.PreviewResults...)

	return &cp, nil
}

// AddLifecyclePolicyInternal seeds a lifecycle policy directly into the backend for testing.
func (b *InMemoryBackend) AddLifecyclePolicyInternal(repositoryName, policy string) {
	b.mu.Lock("AddLifecyclePolicyInternal")
	defer b.mu.Unlock()

	b.lifecyclePolicies.Put(&lifecyclePolicyEntry{RepositoryName: repositoryName, PolicyText: policy})
}
