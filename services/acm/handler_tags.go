package acm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

type listTagsForCertificateOutput struct {
	Tags []map[string]string `json:"Tags"`
}

type addTagsToCertificateOutput struct{}

type removeTagsFromCertificateOutput struct{}

type listTagsForCertificateInput struct {
	CertificateArn string `json:"CertificateArn"`
}

type addTagsToCertificateInput struct {
	CertificateArn string       `json:"CertificateArn"`
	Tags           []svcTags.KV `json:"Tags"`
}

type acmTagKey struct {
	Key string `json:"Key"`
}

type removeTagsFromCertificateInput struct {
	CertificateArn string      `json:"CertificateArn"`
	Tags           []acmTagKey `json:"Tags"`
}

// certTagsEntry associates a live *tags.Tags collection with the certificate
// ARN it annotates. svcTags.Tags carries no identity of its own (see
// pkgs/tags), so ResourceID is a hidden identity field purely for
// store.Table's key -- the same "identity-less" technique used elsewhere in
// this rollout for values with no field of their own to key on. It also
// makes h.tags a "dirty" table: a live *tags.Tags pointer cannot round-trip
// through plain json.Marshal, so persistence.go handles its Snapshot/Restore
// via a plain-map DTO instead of registering it on any store.Registry.
type certTagsEntry struct {
	Tags       *svcTags.Tags
	ResourceID string
}

func certTagsKeyFn(v *certTagsEntry) string { return v.ResourceID }

// reservedTagPrefix is the AWS-reserved tag key/value prefix; keys or values
// beginning with it (case-insensitively) are rejected with
// InvalidTagException, matching real AWS tagging behavior.
const reservedTagPrefix = "aws:"

// setTags validates and merges kv into resourceID's tag set. invalidTagErr
// and tooManyTagsErr let callers supply the sentinel their op actually
// declares: the legacy certificate-tag ops (AddTagsToCertificate,
// ImportCertificate, RequestCertificate) declare InvalidTagException/
// TooManyTagsException, but the ACME resource families and TagResource
// declare ValidationException/ServiceQuotaExceededException instead --
// gopherstack-ftkd.
func (h *Handler) setTags(resourceID string, kv map[string]string, invalidTagErr, tooManyTagsErr error) error {
	const maxTagKeyLength = 128
	const maxTagValueLength = 256
	for k, v := range kv {
		if k == "" {
			return fmt.Errorf("%w: tag key must not be empty", invalidTagErr)
		}
		if strings.HasPrefix(strings.ToLower(k), reservedTagPrefix) ||
			strings.HasPrefix(strings.ToLower(v), reservedTagPrefix) {
			return fmt.Errorf("%w: tag keys/values must not begin with %q", invalidTagErr, reservedTagPrefix)
		}
		if len(k) > maxTagKeyLength {
			return fmt.Errorf("%w: tag key exceeds 128 characters", invalidTagErr)
		}
		if len(v) > maxTagValueLength {
			return fmt.Errorf("%w: tag value exceeds 256 characters", invalidTagErr)
		}
	}

	h.tagsMu.Lock("setTags")
	defer h.tagsMu.Unlock()

	const maxTagsPerCertificate = 50

	entry, exists := h.tags.Get(resourceID)
	newKeys := 0

	if exists {
		for k := range kv {
			if !entry.Tags.HasTag(k) {
				newKeys++
			}
		}
		if entry.Tags.Len()+newKeys > maxTagsPerCertificate {
			return fmt.Errorf("%w: maximum of 50 tags allowed", tooManyTagsErr)
		}
	} else if len(kv) > maxTagsPerCertificate {
		return fmt.Errorf("%w: maximum of 50 tags allowed", tooManyTagsErr)
	}

	if !exists {
		entry = &certTagsEntry{ResourceID: resourceID, Tags: svcTags.New("acm." + resourceID + ".tags")}
		h.tags.Put(entry)
	}
	entry.Tags.Merge(kv)

	return nil
}

func (h *Handler) removeTags(resourceID string, keys []string) {
	h.tagsMu.Lock("removeTags")
	defer h.tagsMu.Unlock()
	if entry, ok := h.tags.Get(resourceID); ok {
		entry.Tags.DeleteKeys(keys)
	}
}

func (h *Handler) cleanupTags(resourceID string) {
	h.tagsMu.Lock("cleanupTags")
	defer h.tagsMu.Unlock()
	if entry, ok := h.tags.Get(resourceID); ok {
		entry.Tags.Close()
		h.tags.Delete(resourceID)
	}
}

func (h *Handler) getTags(resourceID string) []map[string]string {
	h.tagsMu.RLock("getTags")
	entry, ok := h.tags.Get(resourceID)
	h.tagsMu.RUnlock()
	if !ok {
		return []map[string]string{}
	}
	tagMap := entry.Tags.Clone()
	result := make([]map[string]string, 0, len(tagMap))
	for k, v := range tagMap {
		result = append(result, map[string]string{"Key": k, "Value": v})
	}

	return result
}

func (h *Handler) jsonListTagsForCertificate(ctx context.Context, body []byte) (any, error) {
	var input listTagsForCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}

	if !h.Backend.CertExists(ctx, input.CertificateArn) {
		return nil, fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, input.CertificateArn)
	}

	return &listTagsForCertificateOutput{Tags: h.getTags(input.CertificateArn)}, nil
}

func (h *Handler) jsonAddTagsToCertificate(ctx context.Context, body []byte) (any, error) {
	var input addTagsToCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}

	if !h.Backend.CertExists(ctx, input.CertificateArn) {
		return nil, fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, input.CertificateArn)
	}

	kv := make(map[string]string, len(input.Tags))
	for _, t := range input.Tags {
		kv[t.Key] = t.Value
	}
	if err := h.setTags(input.CertificateArn, kv, ErrInvalidTag, ErrTooManyTags); err != nil {
		return nil, err
	}

	return &addTagsToCertificateOutput{}, nil
}

func (h *Handler) jsonRemoveTagsFromCertificate(ctx context.Context, body []byte) (any, error) {
	var input removeTagsFromCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}

	if !h.Backend.CertExists(ctx, input.CertificateArn) {
		return nil, fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, input.CertificateArn)
	}

	keys := make([]string, 0, len(input.Tags))
	for _, t := range input.Tags {
		keys = append(keys, t.Key)
	}
	h.removeTags(input.CertificateArn, keys)

	return &removeTagsFromCertificateOutput{}, nil
}
