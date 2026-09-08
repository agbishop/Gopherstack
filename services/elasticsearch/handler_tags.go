package elasticsearch

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

type listTagsOutput struct {
	TagList []svcTags.KV `json:"TagList"`
}

func (h *Handler) handleListTags(w http.ResponseWriter, r *http.Request) {
	domainARN := r.URL.Query().Get("arn")

	tags, err := h.Backend.ListTags(h.reqContext(r), domainARN)
	if err != nil {
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", err.Error())

		return
	}

	tagList := make([]svcTags.KV, 0, len(tags))
	for k, v := range tags {
		tagList = append(tagList, svcTags.KV{Key: k, Value: v})
	}

	slices.SortFunc(tagList, func(a, b svcTags.KV) int {
		return strings.Compare(a.Key, b.Key)
	})

	h.writeJSON(r, w, &listTagsOutput{TagList: tagList})
}

type addTagsInput struct {
	ARN     string       `json:"ARN"`
	TagList []svcTags.KV `json:"TagList"`
}

func (h *Handler) handleAddTags(w http.ResponseWriter, r *http.Request) {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return
	}

	var req addTagsInput
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "invalid JSON body")

		return
	}

	seen := make(map[string]bool, len(req.TagList))
	for _, t := range req.TagList {
		if len(t.Key) == 0 || len(t.Key) > maxTagKeyLen {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException",
				fmt.Sprintf("tag key must be 1-%d characters", maxTagKeyLen))

			return
		}

		if len(t.Value) > maxTagValueLen {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException",
				fmt.Sprintf("tag value must be 0-%d characters", maxTagValueLen))

			return
		}

		if seen[t.Key] {
			h.writeError(r, w, http.StatusBadRequest, "ValidationException",
				fmt.Sprintf("Duplicate tag key: %s", t.Key))

			return
		}

		seen[t.Key] = true
	}

	tagMap := make(map[string]string, len(req.TagList))
	for _, t := range req.TagList {
		tagMap[t.Key] = t.Value
	}

	ctx := h.reqContext(r)
	existing, _ := h.Backend.ListTags(ctx, req.ARN)

	merged := make(map[string]string, len(existing)+len(tagMap))
	maps.Copy(merged, existing)
	maps.Copy(merged, tagMap)

	if len(merged) > maxTagsPerResource {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException",
			fmt.Sprintf("resource cannot have more than %d tags", maxTagsPerResource))

		return
	}

	// AddTags's own deserializer (elasticsearchservice@v1.45.4 deserializers.go,
	// awsRestjson1_deserializeOpErrorAddTags) has no ResourceNotFoundException
	// case -- an unrecognized ARN here is ValidationException, matching
	// services/opensearch's identical fix for the same sibling API.
	if addErr := h.Backend.AddTags(ctx, req.ARN, tagMap); addErr != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", addErr.Error())

		return
	}

	w.WriteHeader(http.StatusOK)
}

type removeTagsInput struct {
	ARN     string   `json:"ARN"`
	TagKeys []string `json:"TagKeys"`
}

func (h *Handler) handleRemoveTags(w http.ResponseWriter, r *http.Request) {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return
	}

	var req removeTagsInput
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "invalid JSON body")

		return
	}

	// RemoveTags's own deserializer (elasticsearchservice@v1.45.4 deserializers.go,
	// awsRestjson1_deserializeOpErrorRemoveTags) has no ResourceNotFoundException
	// case -- an unrecognized ARN here is ValidationException, matching AddTags above.
	if removeErr := h.Backend.RemoveTags(h.reqContext(r), req.ARN, req.TagKeys); removeErr != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", removeErr.Error())

		return
	}

	w.WriteHeader(http.StatusOK)
}
