package ecr

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// CreateRepositoryCreationTemplate creates a new repository creation template.
func (b *InMemoryBackend) CreateRepositoryCreationTemplate(
	ctx context.Context, //nolint:revive // existing issue.
	req *RepositoryCreationTemplate,
) (*RepositoryCreationTemplate, error) {
	if req.Prefix == "" {
		return nil, fmt.Errorf("%w: prefix is required", ErrInvalidRepositoryName)
	}

	b.mu.Lock("CreateRepositoryCreationTemplate")
	defer b.mu.Unlock()

	if b.repositoryCreationTemplates.Has(req.Prefix) {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryCreationTemplateAlreadyExists, req.Prefix)
	}

	now := time.Now()
	tmpl := &RepositoryCreationTemplate{
		Prefix:                             req.Prefix,
		Description:                        req.Description,
		EncryptionType:                     req.EncryptionType,
		KMSKey:                             req.KMSKey,
		ImageTagMutability:                 req.ImageTagMutability,
		ImageTagMutabilityExclusionFilters: req.ImageTagMutabilityExclusionFilters,
		RepositoryPolicy:                   req.RepositoryPolicy,
		LifecyclePolicy:                    req.LifecyclePolicy,
		AppliedFor:                         req.AppliedFor,
		CustomRoleArn:                      req.CustomRoleArn,
		ResourceTags:                       copyStringMap(req.ResourceTags),
		CreatedAt:                          now,
		UpdatedAt:                          now,
	}
	b.repositoryCreationTemplates.Put(tmpl)

	cp := *tmpl

	return &cp, nil
}

// DeleteRepositoryCreationTemplate deletes a repository creation template.
func (b *InMemoryBackend) DeleteRepositoryCreationTemplate(
	ctx context.Context, //nolint:revive // existing issue.
	prefix string,
) (*RepositoryCreationTemplate, error) {
	b.mu.Lock("DeleteRepositoryCreationTemplate")
	defer b.mu.Unlock()

	tmpl, ok := b.repositoryCreationTemplates.Get(prefix)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryCreationTemplateNotFound, prefix)
	}

	b.repositoryCreationTemplates.Delete(prefix)
	cp := copyRepositoryCreationTemplate(tmpl)

	return &cp, nil
}

// DescribeRepositoryCreationTemplates lists repository creation templates.
func (b *InMemoryBackend) DescribeRepositoryCreationTemplates(
	ctx context.Context, //nolint:revive // existing issue.
	prefixes []string,
) ([]RepositoryCreationTemplate, error) {
	b.mu.RLock("DescribeRepositoryCreationTemplates")
	defer b.mu.RUnlock()

	out := make([]RepositoryCreationTemplate, 0, b.repositoryCreationTemplates.Len())
	if len(prefixes) == 0 {
		for _, tmpl := range b.repositoryCreationTemplates.All() {
			out = append(out, copyRepositoryCreationTemplate(tmpl))
		}
	} else {
		for _, prefix := range prefixes {
			// Real DescribeRepositoryCreationTemplates declares no
			// TemplateNotFoundException (per deserializeOpErrorDescribeRepositoryCreationTemplates,
			// unlike DeleteRepositoryCreationTemplate/UpdateRepositoryCreationTemplate) --
			// an unmatched prefix is silently omitted, not an error.
			tmpl, ok := b.repositoryCreationTemplates.Get(prefix)
			if !ok {
				continue
			}

			out = append(out, copyRepositoryCreationTemplate(tmpl))
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Prefix < out[j].Prefix })

	return out, nil
}

// UpdateRepositoryCreationTemplate updates a repository creation template.
func (b *InMemoryBackend) UpdateRepositoryCreationTemplate(
	ctx context.Context, //nolint:revive // existing issue.
	req *RepositoryCreationTemplate,
) (*RepositoryCreationTemplate, error) {
	b.mu.Lock("UpdateRepositoryCreationTemplate")
	defer b.mu.Unlock()

	tmpl, ok := b.repositoryCreationTemplates.Get(req.Prefix)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryCreationTemplateNotFound, req.Prefix)
	}

	tmpl.Description = req.Description
	tmpl.EncryptionType = req.EncryptionType
	tmpl.KMSKey = req.KMSKey
	tmpl.ImageTagMutability = req.ImageTagMutability
	tmpl.ImageTagMutabilityExclusionFilters = append(
		[]ImageTagMutabilityExclusionFilter(nil),
		req.ImageTagMutabilityExclusionFilters...,
	)
	tmpl.RepositoryPolicy = req.RepositoryPolicy
	tmpl.LifecyclePolicy = req.LifecyclePolicy
	tmpl.CustomRoleArn = req.CustomRoleArn
	tmpl.AppliedFor = append([]string(nil), req.AppliedFor...)
	tmpl.ResourceTags = copyStringMap(req.ResourceTags)
	tmpl.UpdatedAt = time.Now()
	cp := copyRepositoryCreationTemplate(tmpl)

	return &cp, nil
}

func copyRepositoryCreationTemplate(in *RepositoryCreationTemplate) RepositoryCreationTemplate {
	cp := *in
	cp.AppliedFor = append([]string(nil), in.AppliedFor...)
	cp.ImageTagMutabilityExclusionFilters = append(
		[]ImageTagMutabilityExclusionFilter(nil),
		in.ImageTagMutabilityExclusionFilters...,
	)
	cp.ResourceTags = copyStringMap(in.ResourceTags)

	return cp
}
