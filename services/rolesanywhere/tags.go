package rolesanywhere

import "context"

// maxResourceTags is the maximum number of tags a single resource may carry,
// matching the real SDK's shared "TagList" shape (max: 200) that bounds every
// tags list in the service model.
const maxResourceTags = 200

// resourceExistsLocked reports whether resourceARN identifies a trust
// anchor, profile, or CRL currently stored in region. Callers must already
// hold b.mu (for read or write). Tag operations address resources purely by
// ARN, with no dedicated per-family lookup path, so this scans the three
// region-indexed resource groups directly.
func (b *InMemoryBackend) resourceExistsLocked(region, resourceARN string) bool {
	for _, ta := range b.trustAnchorsByRegion.Get(region) {
		if ta.TrustAnchorArn == resourceARN {
			return true
		}
	}

	for _, p := range b.profilesByRegion.Get(region) {
		if p.ProfileArn == resourceARN {
			return true
		}
	}

	for _, c := range b.crlsByRegion.Get(region) {
		if c.CrlArn == resourceARN {
			return true
		}
	}

	return false
}

// TagResource adds tags to a resource. Region is resolved from the resource
// ARN. Real AWS's TagResource models ResourceNotFoundException (returned
// when resourceARN matches no trust anchor/profile/CRL) and
// TooManyTagsException (returned when the resulting tag count on the
// resource would exceed the service's per-resource limit).
func (b *InMemoryBackend) TagResource(ctx context.Context, resourceARN string, tags []TagEntry) error {
	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	region := regionFromARN(resourceARN, getRegion(ctx, b.defaultRegion))

	if !b.resourceExistsLocked(region, resourceARN) {
		return ErrResourceNotFound
	}

	tagStore := b.tagsStore(region)
	existing := cloneTags(tagStore[resourceARN])

	for _, newTag := range tags {
		updated := false

		for i, t := range existing {
			if t.Key == newTag.Key {
				existing[i].Value = newTag.Value
				updated = true

				break
			}
		}

		if !updated {
			existing = append(existing, newTag)
		}
	}

	if len(existing) > maxResourceTags {
		return ErrTooManyTags
	}

	tagStore[resourceARN] = existing

	return nil
}

// UntagResource removes tags from a resource. Region is resolved from the
// resource ARN. Real AWS's UntagResource models ResourceNotFoundException,
// returned when resourceARN matches no trust anchor/profile/CRL (same as
// TagResource/ListTagsForResource; see
// aws-sdk-go-v2/service/rolesanywhere/deserializers.go's
// awsRestjson1_deserializeOpErrorUntagResource).
func (b *InMemoryBackend) UntagResource(ctx context.Context, resourceARN string, tagKeys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	region := regionFromARN(resourceARN, getRegion(ctx, b.defaultRegion))

	if !b.resourceExistsLocked(region, resourceARN) {
		return ErrResourceNotFound
	}

	tagStore := b.tagsStore(region)
	existing := tagStore[resourceARN]
	keySet := make(map[string]bool, len(tagKeys))

	for _, k := range tagKeys {
		keySet[k] = true
	}

	filtered := existing[:0]

	for _, t := range existing {
		if !keySet[t.Key] {
			filtered = append(filtered, t)
		}
	}

	tagStore[resourceARN] = filtered

	return nil
}

// ListTagsForResource returns tags for a resource. Region is resolved from
// the resource ARN. Real AWS's ListTagsForResource models
// ResourceNotFoundException, returned when resourceARN matches no trust
// anchor/profile/CRL (same as TagResource/UntagResource).
func (b *InMemoryBackend) ListTagsForResource(ctx context.Context, resourceARN string) ([]TagEntry, error) {
	b.mu.RLock("ListTagsForResource")
	defer b.mu.RUnlock()

	region := regionFromARN(resourceARN, getRegion(ctx, b.defaultRegion))

	if !b.resourceExistsLocked(region, resourceARN) {
		return nil, ErrResourceNotFound
	}

	return cloneTags(b.tags[region][resourceARN]), nil
}
