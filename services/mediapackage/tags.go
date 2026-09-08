package mediapackage

import (
	"maps"
	"strings"
)

// arnFieldCount is the number of colon-separated fields in an ARN:
// arn:<partition>:<service>:<region>:<account>:<resource>.
const arnFieldCount = 6

// TagResource adds or updates tags on a resource.
func (b *InMemoryBackend) TagResource(resourceARN string, tags map[string]string) error {
	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	if _, ok := b.tags[resourceARN]; !ok {
		b.tags[resourceARN] = make(map[string]string)
	}

	maps.Copy(b.tags[resourceARN], tags)

	// Keep resource-level Tags fields in sync so Describe* responses reflect tag updates.
	if ch := b.findChannelByARN(resourceARN); ch != nil {
		if ch.Tags == nil {
			ch.Tags = make(map[string]string)
		}

		maps.Copy(ch.Tags, tags)
	} else if ep := b.findOriginEndpointByARN(resourceARN); ep != nil {
		if ep.Tags == nil {
			ep.Tags = make(map[string]string)
		}

		maps.Copy(ep.Tags, tags)
	}

	return nil
}

// UntagResource removes tags from a resource.
func (b *InMemoryBackend) UntagResource(resourceARN string, keys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	if existing, ok := b.tags[resourceARN]; ok {
		for _, k := range keys {
			delete(existing, k)
		}
	}

	// Keep resource-level Tags fields in sync so Describe* responses reflect tag removals.
	if ch := b.findChannelByARN(resourceARN); ch != nil {
		for _, k := range keys {
			delete(ch.Tags, k)
		}
	} else if ep := b.findOriginEndpointByARN(resourceARN); ep != nil {
		for _, k := range keys {
			delete(ep.Tags, k)
		}
	}

	return nil
}

// splitMediaPackageResourceARN extracts the resource-type segment
// ("channels"/"origin_endpoints") and ID from a MediaPackage ARN's trailing
// "<resourceType>/<id>" component: arn:<partition>:mediapackage:<region>:
// <account>:<resourceType>/<id> (see buildChannelARN/buildOriginEndpointARN,
// store.go).
func splitMediaPackageResourceARN(resourceARN string) (string, string, bool) {
	// Split to exactly 6 fields so a colon inside the ID stays in the
	// resource segment rather than being mistaken for the segment boundary.
	parts := strings.SplitN(resourceARN, ":", arnFieldCount)
	if len(parts) != arnFieldCount {
		return "", "", false
	}

	resourceType, id, found := strings.Cut(parts[arnFieldCount-1], "/")
	if !found {
		return "", "", false
	}

	return resourceType, id, true
}

// findChannelByARN returns the channel with the given ARN, or nil. O(1): the
// ID is read directly out of the ARN's "channels/<id>" resource segment
// rather than scanning every channel. Must be called with lock held.
func (b *InMemoryBackend) findChannelByARN(resourceARN string) *storedChannel {
	resourceType, id, ok := splitMediaPackageResourceARN(resourceARN)
	if !ok || resourceType != resourceTypeChannel {
		return nil
	}

	ch, found := b.channels.Get(id)
	if !found || ch.ARN != resourceARN {
		return nil
	}

	return ch
}

// findOriginEndpointByARN returns the origin endpoint with the given ARN, or
// nil. O(1): the ID is read directly out of the ARN's "origin_endpoints/<id>"
// resource segment rather than scanning every origin endpoint. Must be
// called with lock held.
func (b *InMemoryBackend) findOriginEndpointByARN(resourceARN string) *storedOriginEndpoint {
	resourceType, id, ok := splitMediaPackageResourceARN(resourceARN)
	if !ok || resourceType != resourceTypeOriginEndpoint {
		return nil
	}

	ep, found := b.originEndpoints.Get(id)
	if !found || ep.ARN != resourceARN {
		return nil
	}

	return ep
}

// ListTagsForResource returns all tags for a resource.
func (b *InMemoryBackend) ListTagsForResource(resourceARN string) (map[string]string, error) {
	b.mu.RLock("ListTagsForResource")
	defer b.mu.RUnlock()

	result := make(map[string]string)

	if existing, ok := b.tags[resourceARN]; ok {
		maps.Copy(result, existing)
	}

	return result, nil
}

// TaggedEntry pairs a resource ARN with its tags.
type TaggedEntry struct {
	Tags map[string]string
	ARN  string
}

// TaggedResources returns every MediaPackage resource ARN (channels and origin
// endpoints) that currently has at least one tag applied via TagResource.
func (b *InMemoryBackend) TaggedResources() []TaggedEntry {
	b.mu.RLock("TaggedResources")
	defer b.mu.RUnlock()

	out := make([]TaggedEntry, 0, len(b.tags))

	for resourceARN, tags := range b.tags {
		if len(tags) == 0 {
			continue
		}

		result := make(map[string]string, len(tags))
		maps.Copy(result, tags)
		out = append(out, TaggedEntry{ARN: resourceARN, Tags: result})
	}

	return out
}
