package shield

import (
	"fmt"
	"maps"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	maxTagsPerResource = 50
	maxTagKeyLen       = 128
	maxTagValueLen     = 256
)

// TagResource adds tags to a protection, keyed by Shield protection ARN or resource ARN.
// Requires an active subscription. Enforces 50-tag cap and key/value length limits.
func (b *InMemoryBackend) TagResource(resourceARN string, tags map[string]string) error {
	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	if b.subscription == nil {
		// TagResource's declared error catalog has no InvalidOperationException
		// (deserializers.go's deserializeOpErrorTagResource); use ErrSubscriptionNotFound
		// (-> ResourceNotFoundException) instead, same as DescribeSubscription's own no-subscription case.
		return fmt.Errorf("%w: Shield Advanced subscription is required", ErrSubscriptionNotFound)
	}

	for k, v := range tags {
		if len(k) > maxTagKeyLen {
			return fmt.Errorf("%w: tag key %q exceeds 128 characters", ErrValidation, k)
		}

		if len(v) > maxTagValueLen {
			return fmt.Errorf("%w: tag value for key %q exceeds 256 characters", ErrValidation, k)
		}
	}

	p, err := b.resolveTaggableProtection(resourceARN)
	if err != nil {
		return err
	}

	if p.Tags == nil {
		p.Tags = make(map[string]string)
	}

	if len(p.Tags)+len(tags) > maxTagsPerResource {
		return fmt.Errorf("%w: resource would exceed the 50-tag limit", ErrValidation)
	}

	maps.Copy(p.Tags, tags)

	return nil
}

// ListTagsForResource returns the tags for a protection.
func (b *InMemoryBackend) ListTagsForResource(resourceARN string) (map[string]string, error) {
	b.mu.RLock("ListTagsForResource")
	defer b.mu.RUnlock()

	p, err := b.resolveTaggableProtection(resourceARN)
	if err != nil {
		return nil, err
	}

	return maps.Clone(p.Tags), nil
}

// UntagResource removes tags from a protection.
func (b *InMemoryBackend) UntagResource(resourceARN string, tagKeys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	p, err := b.resolveTaggableProtection(resourceARN)
	if err != nil {
		return err
	}

	for _, k := range tagKeys {
		delete(p.Tags, k)
	}

	return nil
}

// resolveTaggableProtection resolves a Shield protection ARN or resource ARN to a Protection.
// AWS TagResource accepts:
//   - arn:aws:shield::<acct>:protection/<id>  (Shield protection ARN)
//   - any other ARN treated as a resource ARN lookup
//
// Must be called with b.mu held.
func (b *InMemoryBackend) resolveTaggableProtection(resourceARN string) (*Protection, error) {
	// Shield protection ARN: arn:aws:shield::<acct>:protection/<id>
	if p := b.resolveShieldProtectionARN(resourceARN); p != nil {
		return p, nil
	}

	// Fall back to resource ARN index.
	if matches := b.protectionsByResourceARN.Get(resourceARN); len(matches) > 0 {
		return matches[0], nil
	}

	return nil, fmt.Errorf("%w: protection %q not found", ErrProtectionNotFound, resourceARN)
}

// resolveShieldProtectionARN resolves a Shield protection ARN (arn:{partition}:shield::*:protection/<id>)
// to a protection, or returns nil if the ARN is not a Shield protection ARN or not found.
// The ID is extracted directly from the ARN path — no O(n) scan needed. The partition prefix is
// derived from the backend's configured region (via arn.PartitionForRegion) rather than
// hardcoded to "aws", matching how protectionARN/protectionGroupARN build ARNs elsewhere in this
// package -- otherwise GovCloud/China/ISO region backends could never resolve their own
// protection ARNs back to a Protection.
func (b *InMemoryBackend) resolveShieldProtectionARN(resourceARN string) *Protection {
	prefix := fmt.Sprintf("arn:%s:shield::", arn.PartitionForRegion(b.region))

	if !strings.HasPrefix(resourceARN, prefix) || !strings.Contains(resourceARN, ":protection/") {
		return nil
	}

	parts := strings.SplitN(resourceARN, ":protection/", 2) //nolint:mnd // split into prefix and ID
	if len(parts) < 2 {                                     //nolint:mnd // require 2 parts
		return nil
	}

	p, _ := b.protections.Get(parts[1])

	return p
}

// TaggedEntry pairs a resource ARN with its tags.
type TaggedEntry struct {
	Tags map[string]string
	ARN  string
}

// TaggedResources returns every Shield protection ARN that currently has at
// least one tag applied via TagResource.
func (b *InMemoryBackend) TaggedResources() []TaggedEntry {
	b.mu.RLock("TaggedResources")
	defer b.mu.RUnlock()

	out := make([]TaggedEntry, 0, b.protections.Len())

	for _, p := range b.protections.All() {
		if len(p.Tags) == 0 {
			continue
		}

		out = append(out, TaggedEntry{ARN: p.ProtectionArn, Tags: maps.Clone(p.Tags)})
	}

	return out
}

// cloneTags returns a deep copy of the given tag map.
func cloneTags(tags map[string]string) map[string]string {
	if tags == nil {
		return make(map[string]string)
	}

	return maps.Clone(tags)
}
