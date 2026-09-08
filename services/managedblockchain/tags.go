package managedblockchain

import (
	"fmt"
	"maps"
)

// maxTagsPerResource is the real API's documented per-resource tag cap
// (botocore managedblockchain/2018-09-24 service-2.json.gz InputTagMap: max
// 50).
const maxTagsPerResource = 50

// checkTagLimit returns ErrTooManyTags if applying additions on top of
// existing would push the resource's resulting tag count above
// maxTagsPerResource. The check counts only keys in additions that are not
// already present in existing, matching TagResource's own semantics (an
// update to an existing key does not add a new tag).
func checkTagLimit(existing, additions map[string]string) error {
	newKeys := 0

	for k := range additions {
		if _, ok := existing[k]; !ok {
			newKeys++
		}
	}

	if total := len(existing) + newKeys; total > maxTagsPerResource {
		return fmt.Errorf("%w: %d tag(s) would exceed the %d-tag limit", ErrTooManyTags, total, maxTagsPerResource)
	}

	return nil
}

// TaggedEntry pairs a resource ARN with its tags.
type TaggedEntry struct {
	Tags map[string]string
	ARN  string
}

// resourceTags returns res's Tags field for the resource kinds TagResource
// supports (Network, Member, Node, Accessor, Proposal -- CreateProposal
// accepts Tags in the real CreateProposalInput, aws-sdk-go-v2
// managedblockchain@v1.34.4 api_op_CreateProposal.go), or nil for anything
// else (e.g. Invitation, which carries no tags).
func resourceTags(res any) map[string]string {
	switch r := res.(type) {
	case *Network:
		return r.Tags
	case *Member:
		return r.Tags
	case *Node:
		return r.Tags
	case *Accessor:
		return r.Tags
	case *Proposal:
		return r.Tags
	default:
		return nil
	}
}

// TaggedResources returns every Managed Blockchain resource ARN that
// currently has at least one tag applied via TagResource.
func (b *InMemoryBackend) TaggedResources() []TaggedEntry {
	b.mu.RLock("TaggedResources")
	defer b.mu.RUnlock()

	out := make([]TaggedEntry, 0, len(b.arnToResource))

	for resourceARN, res := range b.arnToResource {
		tags := resourceTags(res)
		if len(tags) == 0 {
			continue
		}

		out = append(out, TaggedEntry{ARN: resourceARN, Tags: maps.Clone(tags)})
	}

	return out
}

// ListTagsForResource returns tags for a resource identified by ARN.
func (b *InMemoryBackend) ListTagsForResource(resourceARN string) (map[string]string, error) {
	b.mu.RLock("ListTagsForResource")
	defer b.mu.RUnlock()

	res, ok := b.arnToResource[resourceARN]
	if !ok {
		return nil, ErrResourceNotFound
	}

	switch r := res.(type) {
	case *Network:
		result := make(map[string]string, len(r.Tags))
		maps.Copy(result, r.Tags)

		return result, nil
	case *Member:
		result := make(map[string]string, len(r.Tags))
		maps.Copy(result, r.Tags)

		return result, nil
	case *Node:
		result := make(map[string]string, len(r.Tags))
		maps.Copy(result, r.Tags)

		return result, nil
	case *Accessor:
		result := make(map[string]string, len(r.Tags))
		maps.Copy(result, r.Tags)

		return result, nil
	case *Proposal:
		result := make(map[string]string, len(r.Tags))
		maps.Copy(result, r.Tags)

		return result, nil
	}

	return nil, ErrResourceNotFound
}

// TagResource adds or updates tags on a resource.
func (b *InMemoryBackend) TagResource(resourceARN string, tags map[string]string) error {
	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	res, ok := b.arnToResource[resourceARN]
	if !ok {
		return ErrResourceNotFound
	}

	if err := checkTagLimit(resourceTags(res), tags); err != nil {
		return err
	}

	switch r := res.(type) {
	case *Network:
		if r.Tags == nil {
			r.Tags = make(map[string]string)
		}

		maps.Copy(r.Tags, tags)

		return nil
	case *Member:
		if r.Tags == nil {
			r.Tags = make(map[string]string)
		}

		maps.Copy(r.Tags, tags)

		return nil
	case *Node:
		if r.Tags == nil {
			r.Tags = make(map[string]string)
		}

		maps.Copy(r.Tags, tags)

		return nil
	case *Accessor:
		if r.Tags == nil {
			r.Tags = make(map[string]string)
		}

		maps.Copy(r.Tags, tags)

		return nil
	case *Proposal:
		if r.Tags == nil {
			r.Tags = make(map[string]string)
		}

		maps.Copy(r.Tags, tags)

		return nil
	}

	return ErrResourceNotFound
}

// UntagResource removes tags from a resource.
func (b *InMemoryBackend) UntagResource(resourceARN string, tagKeys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	res, ok := b.arnToResource[resourceARN]
	if !ok {
		return ErrResourceNotFound
	}

	switch r := res.(type) {
	case *Network:
		for _, k := range tagKeys {
			delete(r.Tags, k)
		}

		return nil
	case *Member:
		for _, k := range tagKeys {
			delete(r.Tags, k)
		}

		return nil
	case *Node:
		for _, k := range tagKeys {
			delete(r.Tags, k)
		}

		return nil
	case *Accessor:
		for _, k := range tagKeys {
			delete(r.Tags, k)
		}

		return nil
	case *Proposal:
		for _, k := range tagKeys {
			delete(r.Tags, k)
		}

		return nil
	}

	return ErrResourceNotFound
}
