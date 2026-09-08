package kms

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// defaultKMSTagsLimit is the default maximum number of tags returned by ListResourceTags.
const defaultKMSTagsLimit int32 = 50

type listResourceTagsInput struct {
	Limit  *int32 `json:"Limit,omitempty"`
	KeyID  string `json:"KeyId"` //nolint:tagliatelle // AWS API uses KeyId
	Marker string `json:"Marker,omitempty"`
}

type kmsTagEntry struct {
	TagKey   string `json:"TagKey"`
	TagValue string `json:"TagValue"`
}

type tagResourceInput struct {
	KeyID string        `json:"KeyId"` //nolint:tagliatelle // AWS API uses KeyId
	Tags  []kmsTagEntry `json:"Tags"`
}

type listResourceTagsOutput struct {
	NextMarker string        `json:"NextMarker,omitempty"`
	Tags       []kmsTagEntry `json:"Tags"`
	Truncated  bool          `json:"Truncated"`
}

type untagResourceInput struct {
	KeyID   string   `json:"KeyId"` //nolint:tagliatelle // AWS API uses KeyId
	TagKeys []string `json:"TagKeys"`
}

func (h *Handler) setTags(resourceID string, kv map[string]string) {
	h.tagsMu.Lock("setTags")
	defer h.tagsMu.Unlock()
	if h.tags[resourceID] == nil {
		h.tags[resourceID] = tags.New("kms." + resourceID + ".tags")
	}
	h.tags[resourceID].Merge(kv)
}

// copyTagsToReplica copies the source key's tags to the replica and overlays any input tags.
func (h *Handler) copyTagsToReplica(sourceKeyID, replicaKeyID string, inputTags []Tag) {
	if sourceKeyID != "" {
		if srcTags := h.getTags(sourceKeyID); len(srcTags) > 0 {
			h.setTags(replicaKeyID, srcTags)
		}
	}

	if len(inputTags) > 0 {
		kv := make(map[string]string, len(inputTags))
		for _, t := range inputTags {
			kv[t.TagKey] = t.TagValue
		}

		h.setTags(replicaKeyID, kv)
	}
}

// validateTags checks every tag in inputTags against validateTag, returning the first
// failure. Callers should invoke this BEFORE creating any backend resource so that a
// malformed tag rejects the whole request instead of leaving an orphaned, untagged
// resource behind (e.g. a KMS key with no reachable KeyId in the caller's response).
func validateTags(inputTags []Tag) error {
	for _, t := range inputTags {
		if err := validateTag(t.TagKey, t.TagValue); err != nil {
			return err
		}
	}

	return nil
}

// applyInputTags converts a []Tag slice to a map and stores it for the given resource ID.
// Returns an error if any tag key/value fails validation. Callers that create a new
// resource in the same request should call validateTags first and only create the
// resource after validation succeeds; see createKeyAction.
func (h *Handler) applyInputTags(resourceID string, inputTags []Tag) error {
	if len(inputTags) == 0 {
		return nil
	}

	if err := validateTags(inputTags); err != nil {
		return err
	}

	kv := make(map[string]string, len(inputTags))
	for _, t := range inputTags {
		kv[t.TagKey] = t.TagValue
	}

	h.setTags(resourceID, kv)

	return nil
}

// purgeTags permanently removes and closes the tag collection for a key that
// the janitor has just permanently purged (see Janitor.OnKeyPurged, wired in
// WithJanitor). Handler.tags lives entirely outside InMemoryBackend, so
// nothing else ever removes an entry from it once set: without this hook a
// key's tags -- and the lockmetrics/Prometheus registration each *tags.Tags
// instance owns (see pkgs/tags.Tags.Close's doc comment) -- would leak for
// the remaining lifetime of the process, since KMS key IDs are UUIDs and are
// never reused. Safe to call with the backend's write lock held: it only
// ever touches tagsMu, never Backend.
func (h *Handler) purgeTags(_, keyID string) {
	h.tagsMu.Lock("purgeTags")
	defer h.tagsMu.Unlock()

	if t := h.tags[keyID]; t != nil {
		t.Close()
		delete(h.tags, keyID)
	}
}

func (h *Handler) removeTags(resourceID string, keys []string) {
	h.tagsMu.RLock("removeTags")
	t := h.tags[resourceID]
	h.tagsMu.RUnlock()
	if t != nil {
		t.DeleteKeys(keys)
	}
}

func (h *Handler) getTags(resourceID string) map[string]string {
	h.tagsMu.RLock("getTags")
	t := h.tags[resourceID]
	h.tagsMu.RUnlock()
	if t == nil {
		return map[string]string{}
	}

	return t.Clone()
}

// listResourceTags handles the ListResourceTags operation.
func (h *Handler) listResourceTags(ctx context.Context, b []byte) (any, error) {
	var input listResourceTagsInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	if _, descErr := h.Backend.DescribeKey(
		ctx, &DescribeKeyInput{KeyID: input.KeyID},
	); descErr != nil {
		return nil, descErr
	}

	tagMap := h.getTags(input.KeyID)
	tagList := make([]kmsTagEntry, 0, len(tagMap))

	for k, v := range tagMap {
		tagList = append(tagList, kmsTagEntry{TagKey: k, TagValue: v})
	}

	sort.Slice(tagList, func(i, j int) bool { return tagList[i].TagKey < tagList[j].TagKey })

	return paginateTagList(tagList, input.Marker, input.Limit), nil
}

// paginateTagList returns a paginated listResourceTagsOutput slice from a full tag list.
func paginateTagList(tagList []kmsTagEntry, marker string, limit *int32) *listResourceTagsOutput {
	startIdx := parseMarker(marker)
	pageLimit := defaultKMSTagsLimit

	if limit != nil && *limit > 0 {
		pageLimit = *limit
	}

	if startIdx >= len(tagList) {
		return &listResourceTagsOutput{Tags: []kmsTagEntry{}, Truncated: false}
	}

	end := startIdx + int(pageLimit)

	var nextMarker string

	if end < len(tagList) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(tagList)
	}

	return &listResourceTagsOutput{
		Tags:       tagList[startIdx:end],
		NextMarker: nextMarker,
		Truncated:  nextMarker != "",
	}
}

// tagResource handles the TagResource operation, validating key existence and tag count.
func (h *Handler) tagResource(ctx context.Context, b []byte) (any, error) {
	var input tagResourceInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	desc, descErr := h.Backend.DescribeKey(ctx, &DescribeKeyInput{KeyID: input.KeyID})
	if descErr != nil {
		return nil, descErr
	}

	if desc.KeyMetadata.KeyState == KeyStatePendingDeletion {
		return nil, fmt.Errorf("%w: key %q is pending deletion", ErrKeyInvalidState, desc.KeyMetadata.KeyID)
	}

	newTags := make(map[string]string, len(input.Tags))
	for _, t := range input.Tags {
		if err := validateTag(t.TagKey, t.TagValue); err != nil {
			return nil, err
		}

		newTags[t.TagKey] = t.TagValue
	}

	if err := h.validateTagCount(input.KeyID, newTags); err != nil {
		return nil, err
	}

	h.setTags(input.KeyID, newTags)

	return struct{}{}, nil
}

// validateTag checks that a tag key and value satisfy AWS KMS length and content constraints.
func validateTag(key, value string) error {
	if key == "" {
		return fmt.Errorf("%w: tag key must not be empty", ErrInvalidTag)
	}

	if len(key) > maxTagKeyLength {
		return fmt.Errorf(
			"%w: tag key %q exceeds maximum length of %d characters",
			ErrInvalidTag, key, maxTagKeyLength,
		)
	}

	if strings.HasPrefix(key, "aws:") {
		return fmt.Errorf(
			"%w: tag key %q must not start with the reserved prefix 'aws:'",
			ErrInvalidTag, key,
		)
	}

	if len(value) > maxTagValueLength {
		return fmt.Errorf(
			"%w: tag value for key %q exceeds maximum length of %d characters",
			ErrInvalidTag, key, maxTagValueLength,
		)
	}

	return nil
}

// validateTagCount returns an error if adding newTags to keyID would exceed the max tag limit.
func (h *Handler) validateTagCount(keyID string, newTags map[string]string) error {
	existing := h.getTags(keyID)

	netNew := 0
	for k := range newTags {
		if _, alreadyPresent := existing[k]; !alreadyPresent {
			netNew++
		}
	}

	if len(existing)+netNew > maxTagsPerKey {
		return fmt.Errorf(
			"%w: tagging key %q would exceed the maximum of %d tags",
			ErrLimitExceeded, keyID, maxTagsPerKey,
		)
	}

	return nil
}

// untagResource handles the UntagResource operation.
func (h *Handler) untagResource(ctx context.Context, b []byte) (any, error) {
	var input untagResourceInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	desc, descErr := h.Backend.DescribeKey(ctx, &DescribeKeyInput{KeyID: input.KeyID})
	if descErr != nil {
		return nil, descErr
	}

	if desc.KeyMetadata.KeyState == KeyStatePendingDeletion {
		return nil, fmt.Errorf("%w: key %q is pending deletion", ErrKeyInvalidState, desc.KeyMetadata.KeyID)
	}

	h.removeTags(input.KeyID, input.TagKeys)

	return struct{}{}, nil
}

// buildTagActions returns dispatch entries for KMS resource tag operations.
func (h *Handler) buildTagActions() map[string]kmsActionFn {
	return map[string]kmsActionFn{
		"ListResourceTags": func(ctx context.Context, b []byte) (any, error) {
			return h.listResourceTags(ctx, b)
		},
		"TagResource": func(ctx context.Context, b []byte) (any, error) {
			return h.tagResource(ctx, b)
		},
		"UntagResource": func(ctx context.Context, b []byte) (any, error) {
			return h.untagResource(ctx, b)
		},
	}
}

// TaggedKeyInfo contains a KMS key's ARN and tag snapshot.
// Used by the Resource Groups Tagging API cross-service listing.
type TaggedKeyInfo struct {
	Tags map[string]string
	ARN  string
}

// TaggedKeys returns a snapshot of all KMS keys with their ARNs and tags.
// Intended for use by the Resource Groups Tagging API provider.
func (h *Handler) TaggedKeys(ctx context.Context) []TaggedKeyInfo {
	out, err := h.Backend.ListKeys(ctx, &ListKeysInput{})
	if err != nil {
		return nil
	}

	h.tagsMu.RLock("TaggedKeys")
	defer h.tagsMu.RUnlock()

	result := make([]TaggedKeyInfo, 0, len(out.Keys))

	for _, k := range out.Keys {
		var tagMap map[string]string
		if t := h.tags[k.KeyID]; t != nil {
			tagMap = t.Clone()
		}

		result = append(result, TaggedKeyInfo{ARN: k.KeyArn, Tags: tagMap})
	}

	return result
}

// TagKeyByARN applies tags to the KMS key identified by its ARN.
func (h *Handler) TagKeyByARN(ctx context.Context, keyARN string, newTags map[string]string) error {
	out, err := h.Backend.ListKeys(ctx, &ListKeysInput{})
	if err != nil {
		return err
	}

	for _, k := range out.Keys {
		if k.KeyArn == keyARN {
			h.setTags(k.KeyID, newTags)

			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrKeyNotFound, keyARN)
}

// UntagKeyByARN removes the specified tag keys from the KMS key identified by its ARN.
func (h *Handler) UntagKeyByARN(ctx context.Context, keyARN string, tagKeys []string) error {
	out, err := h.Backend.ListKeys(ctx, &ListKeysInput{})
	if err != nil {
		return err
	}

	for _, k := range out.Keys {
		if k.KeyArn == keyARN {
			h.removeTags(k.KeyID, tagKeys)

			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrKeyNotFound, keyARN)
}
