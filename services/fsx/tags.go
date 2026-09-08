package fsx

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/blackbirdworks/gopherstack/pkgs/collections"
)

// TagResource adds or updates tags on a resource.
//
// The tag-limit check below uses ErrValidation (BadRequest), not
// ErrTagLimitExceeded: TagResource's own switch (fsx@v1.68.4
// deserializers.go deserializeOpErrorTagResource) is [BadRequest,
// InternalServerError, NotServiceResourceError, ResourceDoesNotSupportTagging,
// ResourceNotFound] -- ServiceLimitExceeded is not declared here, unlike its
// legitimate declarers CopyBackup/CopySnapshotAndUpdateVolume/CreateBackup/
// CreateDataRepositoryAssociation/CreateDataRepositoryTask. Neither
// NotServiceResourceError nor ResourceDoesNotSupportTagging fit "too many
// tags" by their own doc comments (types/errors.go), so BadRequest -- this
// op's own declared generic-client-error type -- is the correct
// substitution (gopherstack-6flj/uox6 error-envelope sweep).
func (b *InMemoryBackend) TagResource(resourceARN string, tags []Tag) error {
	if err := validateTags(tags); err != nil {
		return err
	}

	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	if !b.arnExists(resourceARN) {
		return ErrResourceNotFound
	}

	if b.tags[resourceARN] == nil {
		b.tags[resourceARN] = make(map[string]string)
	}

	existing := b.tags[resourceARN]
	newKeys := 0
	for _, t := range tags {
		if _, ok := existing[t.Key]; !ok {
			newKeys++
		}
	}

	if len(existing)+newKeys > maxTagsPerResource {
		return fmt.Errorf("%w: adding %d tag(s) would exceed the %d-tag limit",
			ErrValidation, newKeys, maxTagsPerResource)
	}

	for _, t := range tags {
		existing[t.Key] = t.Value
	}

	return nil
}

// UntagResource removes tags from a resource.
func (b *InMemoryBackend) UntagResource(resourceARN string, tagKeys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	if !b.arnExists(resourceARN) {
		return ErrResourceNotFound
	}

	for _, k := range tagKeys {
		delete(b.tags[resourceARN], k)
	}

	return nil
}

// ListTagsForResource returns the tags on a resource.
func (b *InMemoryBackend) ListTagsForResource(resourceARN string) ([]Tag, error) {
	b.mu.RLock("ListTagsForResource")
	defer b.mu.RUnlock()

	if !b.arnExists(resourceARN) {
		return nil, ErrResourceNotFound
	}

	return tagsMapToSlice(b.tags[resourceARN]), nil
}

// validateCreateTags is validateTags plus the shared "Tags" shape's own
// list-length constraint (fsx@v1.68.4 botocore service-2.json.gz, "A list
// of Tag values, with a maximum of 50 elements", max: 50) -- for every
// Create* op's initial Tags input, not TagResource's separate
// cumulative-existing-plus-new check (tags.go's maxTagsPerResource above,
// which already has its own op-specific wire code). The real SDK's own
// client-side validator (validateOpCreate<Op>Input -> validateTags,
// validators.go) never checks this length, only each tag's own key/value
// constraints, so a real client can put more than 50 tags on the wire.
// ServiceLimitExceeded is legitimately declared by every caller of this
// function (CopyBackup, CreateBackup, CreateDataRepositoryAssociation,
// CreateDataRepositoryTask, CreateFileCache, CreateFileSystem,
// CreateFileSystemFromBackup, CreateSnapshot, CreateStorageVirtualMachine,
// CreateVolume, CreateVolumeFromBackup -- each op's own
// deserializeOpError<Op> switch confirmed), unlike TagResource which does
// not declare it (see the doc comment on TagResource's own check above).
func validateCreateTags(tags []Tag) error {
	if err := validateTags(tags); err != nil {
		return err
	}

	if len(tags) > maxTagsPerResource {
		return fmt.Errorf("%w: %d tag(s) exceeds the %d-tag limit", ErrTagLimitExceeded, len(tags), maxTagsPerResource)
	}

	return nil
}

// validateTags returns ErrTagInvalid if any tag key or value violates FSx constraints:
// key must be 1–128 chars and must not start with "aws:"; value must be 0–256 chars.
func validateTags(tags []Tag) error {
	for _, t := range tags {
		klen := utf8.RuneCountInString(t.Key)
		if klen == 0 || klen > maxTagKeyLen {
			return fmt.Errorf("%w: tag key must be 1–%d chars, got %d", ErrTagInvalid, maxTagKeyLen, klen)
		}

		if strings.HasPrefix(strings.ToLower(t.Key), "aws:") {
			return fmt.Errorf("%w: tag key must not start with reserved prefix \"aws:\"", ErrTagInvalid)
		}

		if vlen := utf8.RuneCountInString(t.Value); vlen > maxTagValueLen {
			return fmt.Errorf("%w: tag value must be 0–%d chars, got %d", ErrTagInvalid, maxTagValueLen, vlen)
		}
	}

	return nil
}

func tagsSliceToMap(tags []Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[t.Key] = t.Value
	}

	return m
}

func tagsMapToSlice(m map[string]string) []Tag {
	if len(m) == 0 {
		return nil
	}

	keys := collections.SortedKeys(m)

	tags := make([]Tag, len(keys))
	for i, k := range keys {
		tags[i] = Tag{Key: k, Value: m[k]}
	}

	return tags
}
