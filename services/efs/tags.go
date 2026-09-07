package efs

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// validateTags returns an error if any tag key/value violates AWS constraints.
// Callers (CreateAccessPoint, CreateFileSystem, TagResource, CreateTags) all declare
// BadRequest, never ValidationException, for a malformed request (efs@v1.44.4
// deserializers.go).
func validateTags(kv map[string]string) error {
	if len(kv) > maxTagsPerResource {
		return fmt.Errorf(
			"%w: too many tags: %d (max %d)",
			ErrBadRequest,
			len(kv),
			maxTagsPerResource,
		)
	}

	for k, v := range kv {
		if len(k) == 0 || len(k) > maxTagKeyLen {
			return fmt.Errorf(
				"%w: tag key length must be 1-%d, got %d",
				ErrBadRequest,
				maxTagKeyLen,
				len(k),
			)
		}
		if strings.HasPrefix(k, "aws:") {
			return fmt.Errorf("%w: tag key must not start with 'aws:'", ErrBadRequest)
		}
		if len(v) > maxTagValueLen {
			return fmt.Errorf(
				"%w: tag value length must be 0-%d, got %d",
				ErrBadRequest,
				maxTagValueLen,
				len(v),
			)
		}
	}

	return nil
}

// TagResource adds or updates tags on a resource (file system or access point) by ARN or ID.
func (b *InMemoryBackend) TagResource(ctx context.Context, resourceID string, kv map[string]string) error {
	if err := validateTags(kv); err != nil {
		return err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	if fs, ok := b.fileSystems.Get(regionKey(region, resourceID)); ok {
		fs.Tags.Merge(kv)

		return nil
	}
	if fs, ok := b.fileSystemsByARN.Get(regionKey(region, resourceID)); ok {
		fs.Tags.Merge(kv)

		return nil
	}

	if ap, ok := b.accessPoints.Get(regionKey(region, resourceID)); ok {
		ap.Tags.Merge(kv)

		return nil
	}
	if ap, ok := b.accessPointsByARN.Get(regionKey(region, resourceID)); ok {
		ap.Tags.Merge(kv)

		return nil
	}

	return fmt.Errorf("%w: resource %s not found", ErrNotFound, resourceID)
}

// UntagResource removes tags from a resource (file system or access point) by ARN or ID.
func (b *InMemoryBackend) UntagResource(ctx context.Context, resourceID string, tagKeys []string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	if fs, ok := b.fileSystems.Get(regionKey(region, resourceID)); ok {
		fs.Tags.DeleteKeys(tagKeys)

		return nil
	}
	if fs, ok := b.fileSystemsByARN.Get(regionKey(region, resourceID)); ok {
		fs.Tags.DeleteKeys(tagKeys)

		return nil
	}

	if ap, ok := b.accessPoints.Get(regionKey(region, resourceID)); ok {
		ap.Tags.DeleteKeys(tagKeys)

		return nil
	}
	if ap, ok := b.accessPointsByARN.Get(regionKey(region, resourceID)); ok {
		ap.Tags.DeleteKeys(tagKeys)

		return nil
	}

	return fmt.Errorf("%w: resource %s not found", ErrNotFound, resourceID)
}

// ListTagsForResource lists tags for a resource by ID or ARN.
func (b *InMemoryBackend) ListTagsForResource(
	ctx context.Context,
	resourceID string,
) (map[string]string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListTagsForResource")
	defer b.mu.RUnlock()

	if fs, ok := b.fileSystems.Get(regionKey(region, resourceID)); ok {
		return fs.Tags.Clone(), nil
	}
	if fs, ok := b.fileSystemsByARN.Get(regionKey(region, resourceID)); ok {
		return fs.Tags.Clone(), nil
	}

	if ap, ok := b.accessPoints.Get(regionKey(region, resourceID)); ok {
		return ap.Tags.Clone(), nil
	}
	if ap, ok := b.accessPointsByARN.Get(regionKey(region, resourceID)); ok {
		return ap.Tags.Clone(), nil
	}

	return nil, fmt.Errorf("%w: resource %s not found", ErrNotFound, resourceID)
}

// CreateTags adds tags to a file system (legacy operation, delegates to TagResource).
func (b *InMemoryBackend) CreateTags(ctx context.Context, fileSystemID string, kv map[string]string) error {
	if err := validateTags(kv); err != nil {
		return err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateTags")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}

	fs.Tags.Merge(kv)

	return nil
}

// TaggedEntry pairs a resource ARN with its tag map, for cross-service tag
// enumeration by the Resource Groups Tagging API (see cli.go's
// wireTaggingEFS).
type TaggedEntry struct {
	Tags map[string]string
	ARN  string
}

// appendEFSTaggedEntry appends a TaggedEntry for arn/t to entries when t
// holds at least one tag.
func appendEFSTaggedEntry(entries []TaggedEntry, arn string, t *tags.Tags) []TaggedEntry {
	if t == nil || t.Len() == 0 {
		return entries
	}

	return append(entries, TaggedEntry{ARN: arn, Tags: t.Clone()})
}

// TaggedResources returns every EFS resource ARN that currently has at
// least one tag, across every taggable EFS resource kind (file systems,
// access points).
func (b *InMemoryBackend) TaggedResources() []TaggedEntry {
	b.mu.RLock("TaggedResources")
	defer b.mu.RUnlock()

	var out []TaggedEntry

	for _, fs := range b.fileSystems.All() {
		out = appendEFSTaggedEntry(out, fs.FileSystemArn, fs.Tags)
	}

	for _, ap := range b.accessPoints.All() {
		out = appendEFSTaggedEntry(out, ap.AccessPointArn, ap.Tags)
	}

	return out
}

// DeleteTags removes tags from a file system by key (legacy operation).
func (b *InMemoryBackend) DeleteTags(ctx context.Context, fileSystemID string, tagKeys []string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteTags")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}

	fs.Tags.DeleteKeys(tagKeys)

	return nil
}
