package servicediscovery

import (
	"fmt"
	"maps"
)

// ListTagsForResource returns tags for a resource ARN (namespace or service).
func (b *InMemoryBackend) ListTagsForResource(arn string) (map[string]string, error) {
	b.mu.RLock("ListTagsForResource")
	defer b.mu.RUnlock()

	if nsMatches := b.namespacesByARN.Get(arn); len(nsMatches) > 0 {
		return copyTags(nsMatches[0].Tags), nil
	}

	if svcMatches := b.servicesByARN.Get(arn); len(svcMatches) > 0 {
		return copyTags(svcMatches[0].Tags), nil
	}

	return nil, fmt.Errorf("%w: resource %s not found", ErrResourceNotFound, arn)
}

// mergedTagCount returns how many distinct keys existing and incoming would
// have combined -- used to enforce the 50-tag-per-resource quota on the
// post-merge total, not the incoming request alone (TooManyTagsException:
// "the maximum number of tags that can be applied to a resource is 50").
func mergedTagCount(existing, incoming map[string]string) int {
	count := len(existing)

	for k := range incoming {
		if _, ok := existing[k]; !ok {
			count++
		}
	}

	return count
}

// TagResource adds tags to a resource (namespace or service).
func (b *InMemoryBackend) TagResource(arn string, tags map[string]string) error {
	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	if nsMatches := b.namespacesByARN.Get(arn); len(nsMatches) > 0 {
		ns := nsMatches[0]
		if mergedTagCount(ns.Tags, tags) > maxTagCount {
			return fmt.Errorf("%w: resource %s would have more than %d tags", ErrTooManyTags, arn, maxTagCount)
		}

		if ns.Tags == nil {
			ns.Tags = make(map[string]string)
		}

		maps.Copy(ns.Tags, tags)

		return nil
	}

	if svcMatches := b.servicesByARN.Get(arn); len(svcMatches) > 0 {
		svc := svcMatches[0]
		if mergedTagCount(svc.Tags, tags) > maxTagCount {
			return fmt.Errorf("%w: resource %s would have more than %d tags", ErrTooManyTags, arn, maxTagCount)
		}

		if svc.Tags == nil {
			svc.Tags = make(map[string]string)
		}

		maps.Copy(svc.Tags, tags)

		return nil
	}

	return fmt.Errorf("%w: resource %s not found", ErrResourceNotFound, arn)
}

// TaggedEntry pairs a resource ARN with its tag map, for cross-service tag
// enumeration by the Resource Groups Tagging API (see cli.go's
// wireTaggingServiceDiscovery).
type TaggedEntry struct {
	Tags map[string]string
	ARN  string
}

// TaggedResources returns every Cloud Map namespace and service ARN that
// currently has at least one tag applied via TagResource.
func (b *InMemoryBackend) TaggedResources() []TaggedEntry {
	b.mu.RLock("TaggedResources")
	defer b.mu.RUnlock()

	namespaces := b.namespaces.All()
	services := b.services.All()
	out := make([]TaggedEntry, 0, len(namespaces)+len(services))

	for _, ns := range namespaces {
		if len(ns.Tags) == 0 {
			continue
		}

		out = append(out, TaggedEntry{ARN: ns.ARN, Tags: copyTags(ns.Tags)})
	}

	for _, svc := range services {
		if len(svc.Tags) == 0 {
			continue
		}

		out = append(out, TaggedEntry{ARN: svc.ARN, Tags: copyTags(svc.Tags)})
	}

	return out
}

// UntagResource removes tags from a resource (namespace or service).
func (b *InMemoryBackend) UntagResource(arn string, tagKeys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	if nsMatches := b.namespacesByARN.Get(arn); len(nsMatches) > 0 {
		ns := nsMatches[0]
		for _, k := range tagKeys {
			delete(ns.Tags, k)
		}

		return nil
	}

	if svcMatches := b.servicesByARN.Get(arn); len(svcMatches) > 0 {
		svc := svcMatches[0]
		for _, k := range tagKeys {
			delete(svc.Tags, k)
		}

		return nil
	}

	return fmt.Errorf("%w: resource %s not found", ErrResourceNotFound, arn)
}
