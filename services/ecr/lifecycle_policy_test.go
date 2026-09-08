package ecr_test

// lifecycle_policy_test.go — verifies lifecycle_policy.go's CRUD surface:
// Get/Put/Delete LifecyclePolicy and Start/GetLifecyclePolicyPreview, as
// distinct from the expiry-evaluation semantics covered in lifecycle_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecr"
)

func TestLifecyclePolicy_CRUD(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	makeRepo(t, b, "lp-repo")

	policy := `{"rules":[{"rulePriority":1,"action":{"type":"expire"},"selection":{"tagStatus":"untagged","countType":"imageCountMoreThan","countNumber":10}}]}` //nolint:lll // JSON policy exceeds 120 chars; splitting worsens readability

	out, err := b.PutLifecyclePolicy(context.Background(), "lp-repo", policy)
	require.NoError(t, err)
	assert.Equal(t, policy, out.LifecyclePolicyText)

	got, err := b.GetLifecyclePolicy(context.Background(), "lp-repo")
	require.NoError(t, err)
	assert.Equal(t, policy, got.LifecyclePolicyText)
	assert.Equal(t, 1, b.LifecyclePolicyCount())

	_, err = b.DeleteLifecyclePolicy(context.Background(), "lp-repo")
	require.NoError(t, err)
	assert.Equal(t, 0, b.LifecyclePolicyCount())

	_, err = b.GetLifecyclePolicy(context.Background(), "lp-repo")
	assert.ErrorIs(t, err, ecr.ErrLifecyclePolicyNotFound)
}

func TestPutLifecyclePolicy_HTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repoName   string
		policy     string
		wantStatus int
	}{
		{
			name:       "creates_policy",
			repoName:   "policy-repo",
			policy:     `{"rules":[{"rulePriority":1,"description":"test"}]}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "repo_not_found",
			repoName:   "no-such-repo",
			policy:     `{}`,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyHandler()

			if tt.wantStatus == http.StatusOK {
				doAccuracy(t, h, "CreateRepository", map[string]any{"repositoryName": tt.repoName})
			}

			rec := doAccuracy(t, h, "PutLifecyclePolicy", map[string]any{
				"repositoryName":      tt.repoName,
				"lifecyclePolicyText": tt.policy,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestDeleteLifecyclePolicy_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repoName   string
		wantStatus int
	}{
		{
			name:       "repo_exists_but_no_policy",
			repoName:   "no-policy-repo",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "repo_not_found",
			repoName:   "ghost-repo",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyHandler()

			if tt.repoName == "no-policy-repo" {
				doAccuracy(t, h, "CreateRepository", map[string]any{"repositoryName": tt.repoName})
			}

			rec := doAccuracy(t, h, "DeleteLifecyclePolicy", map[string]any{
				"repositoryName": tt.repoName,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestECR_DeleteLifecyclePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(*ecr.Handler)
		repositoryName string
		wantStatus     int
	}{
		{
			name: "deletes lifecycle policy for existing repo",
			setup: func(h *ecr.Handler) {
				rec := doECRRequest(
					t,
					h,
					"CreateRepository",
					map[string]any{"repositoryName": "policy-repo"},
				)
				require.Equal(t, http.StatusOK, rec.Code)

				rec2 := doECRRequest(t, h, "PutLifecyclePolicy", map[string]any{
					"repositoryName":      "policy-repo",
					"lifecyclePolicyText": `{"rules":[]}`,
				})
				require.Equal(t, http.StatusOK, rec2.Code)
			},
			repositoryName: "policy-repo",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "nonexistent repository returns not found",
			repositoryName: "nonexistent",
			wantStatus:     http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doECRRequest(t, h, "DeleteLifecyclePolicy", map[string]any{
				"repositoryName": tt.repositoryName,
			})
			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, tt.repositoryName, out["repositoryName"])
			}
		})
	}
}

func TestPutLifecyclePolicy_And_GetLifecyclePolicy_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lc-repo")

	policy := `{"rules":[{"rulePriority":1,"description":"keep 10",` +
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":10},` +
		`"action":{"type":"expire"}}]}`

	putRec := doAccuracy(t, h, "PutLifecyclePolicy", map[string]any{
		"repositoryName":      "lc-repo",
		"lifecyclePolicyText": policy,
	})
	require.Equal(t, http.StatusOK, putRec.Code)

	getRec := doAccuracy(t, h, "GetLifecyclePolicy", map[string]any{
		"repositoryName": "lc-repo",
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	out := parseAccuracy(t, getRec)
	assert.Equal(t, policy, out["lifecyclePolicyText"],
		"GetLifecyclePolicy must return the exact policy set by PutLifecyclePolicy")
}

func TestGetLifecyclePolicy_NonExistentRepo_Returns404(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "GetLifecyclePolicy", map[string]any{
		"repositoryName": "ghost-repo",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetLifecyclePolicy_NonExistentPolicy_Returns404(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "no-policy")

	rec := doAccuracy(t, h, "GetLifecyclePolicy", map[string]any{
		"repositoryName": "no-policy",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestLifecyclePolicy_Put_Delete_Gone(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lc-del")

	policy := `{"rules":[{"rulePriority":1,` +
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":5},` +
		`"action":{"type":"expire"}}]}`
	doAccuracy(t, h, "PutLifecyclePolicy", map[string]any{
		"repositoryName":      "lc-del",
		"lifecyclePolicyText": policy,
	})

	delRec := doAccuracy(t, h, "DeleteLifecyclePolicy", map[string]any{
		"repositoryName": "lc-del",
	})
	require.Equal(t, http.StatusOK, delRec.Code)

	// After deletion, Get must return 404.
	getRec := doAccuracy(t, h, "GetLifecyclePolicy", map[string]any{
		"repositoryName": "lc-del",
	})
	assert.Equal(t, http.StatusNotFound, getRec.Code)
}

func TestLifecyclePolicy_Delete_NonExistent_Returns404(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lc-noexist")

	rec := doAccuracy(t, h, "DeleteLifecyclePolicy", map[string]any{
		"repositoryName": "lc-noexist",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestLifecyclePolicyPreview_WithPolicy(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lc-preview")
	mustPutImage(t, h, "lc-preview", "v1", `{"schemaVersion":2,"v":1}`)

	policy := `{"rules":[{"rulePriority":1,"description":"keep none",` +
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":0},` +
		`"action":{"type":"expire"}}]}`

	previewRec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName":      "lc-preview",
		"lifecyclePolicyText": policy,
	})
	require.Equal(t, http.StatusOK, previewRec.Code)
	previewOut := parseAccuracy(t, previewRec)
	assert.NotEmpty(t, previewOut["status"])

	getPreview := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "lc-preview",
	})
	require.Equal(t, http.StatusOK, getPreview.Code)
	pOut := parseAccuracy(t, getPreview)
	assert.Equal(t, "COMPLETE", pOut["status"])
}

func TestLifecyclePolicyPreview_UsesExistingPolicy(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lc-use-existing")

	policy := `{"rules":[{"rulePriority":1,` +
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":10},` +
		`"action":{"type":"expire"}}]}`
	doAccuracy(t, h, "PutLifecyclePolicy", map[string]any{
		"repositoryName":      "lc-use-existing",
		"lifecyclePolicyText": policy,
	})

	// StartLifecyclePolicyPreview without policyText → uses stored policy.
	previewRec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName": "lc-use-existing",
	})
	require.Equal(t, http.StatusOK, previewRec.Code)
	pOut := parseAccuracy(t, previewRec)
	assert.Equal(t, policy, pOut["lifecyclePolicyText"],
		"preview must use the stored lifecycle policy when policyText not provided")
}

func TestGetLifecyclePolicyPreview_NoPreview_Returns404(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "no-preview")

	rec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "no-preview",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetLifecyclePolicyPreview_NoPreview_ErrorType verifies the wire __type
// for a missing preview is LifecyclePolicyPreviewNotFoundException, not
// LifecyclePolicyNotFoundException.
//
// GetLifecyclePolicyPreview's own deserializeOpError (aws-sdk-go-v2/service/
// ecr@v1.60.4 deserializers.go) recognizes InvalidParameterException,
// LifecyclePolicyPreviewNotFoundException, RepositoryNotFoundException,
// ServerException and ValidationException -- LifecyclePolicyNotFoundException
// (the code GetLifecyclePolicy/DeleteLifecyclePolicy/PutLifecyclePolicy use)
// is a distinct, real exception type this op does not model.
func TestGetLifecyclePolicyPreview_NoPreview_ErrorType(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "no-preview-errtype")

	rec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "no-preview-errtype",
	})
	require.Equal(t, http.StatusNotFound, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "LifecyclePolicyPreviewNotFoundException", out["__type"])
	assert.NotEqual(t, "LifecyclePolicyNotFoundException", out["__type"])
}

func TestLifecyclePolicyPreview_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "preview-repo")
	policyText := `{"rules":[{"rulePriority":1,` +
		`"selection":{"tagStatus":"untagged","countType":"imageCountMoreThan","countNumber":1},` +
		`"action":{"type":"expire"}}]}`
	doAccuracy(t, h, "PutLifecyclePolicy", map[string]any{
		"repositoryName":      "preview-repo",
		"lifecyclePolicyText": policyText,
	})

	startRec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName": "preview-repo",
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	startOut := parseAccuracy(t, startRec)
	assert.Equal(t, "preview-repo", startOut["repositoryName"])
	assert.Equal(t, "COMPLETE", startOut["status"])

	getRec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "preview-repo",
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	getOut := parseAccuracy(t, getRec)
	assert.Equal(t, "preview-repo", getOut["repositoryName"])
	assert.Equal(t, "COMPLETE", getOut["status"])
	assert.Equal(t, policyText, getOut["lifecyclePolicyText"],
		"GetLifecyclePolicyPreview must return the policy text used for the preview")
}

func TestStartLifecyclePolicyPreview_WithInlinePolicy(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "inline-preview")

	inlinePolicy := `{"rules":[]}`
	rec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName":      "inline-preview",
		"lifecyclePolicyText": inlinePolicy,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, inlinePolicy, out["lifecyclePolicyText"])
}

func TestGetLifecyclePolicyPreview_NotStarted_Errors(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "no-preview-2")

	rec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "no-preview-2",
	})
	assert.NotEqual(t, http.StatusOK, rec.Code,
		"GetLifecyclePolicyPreview without StartLifecyclePolicyPreview must error")
}

// TestLifecyclePolicyResult_LastEvaluatedAt_IsEpochNumber locks the
// GetLifecyclePolicy/PutLifecyclePolicy/DeleteLifecyclePolicy wire shape: AWS's
// awsAwsjson11_deserializeDocumentGetLifecyclePolicyOutput parses lastEvaluatedAt
// via smithytime.ParseEpochSeconds(json.Number) — a bare time.Time json.Marshal
// would instead emit an RFC3339 string, which the real SDK client rejects.
func TestLifecyclePolicyResult_LastEvaluatedAt_IsEpochNumber(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "lea-repo")

	policy := `{"rules":[{"rulePriority":1,` +
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":10},` +
		`"action":{"type":"expire"}}]}`

	putRec := doAccuracy(t, h, "PutLifecyclePolicy", map[string]any{
		"repositoryName":      "lea-repo",
		"lifecyclePolicyText": policy,
	})
	require.Equal(t, http.StatusOK, putRec.Code)
	putOut := parseAccuracy(t, putRec)
	putLEA, ok := putOut["lastEvaluatedAt"].(float64)
	require.True(t, ok, "PutLifecyclePolicy lastEvaluatedAt must be a JSON number, got %T", putOut["lastEvaluatedAt"])
	assert.Positive(t, putLEA, "lastEvaluatedAt must reflect the immediate post-Put evaluation")

	getRec := doAccuracy(t, h, "GetLifecyclePolicy", map[string]any{"repositoryName": "lea-repo"})
	require.Equal(t, http.StatusOK, getRec.Code)
	getOut := parseAccuracy(t, getRec)
	getLEA, ok := getOut["lastEvaluatedAt"].(float64)
	require.True(t, ok, "GetLifecyclePolicy lastEvaluatedAt must be a JSON number, got %T", getOut["lastEvaluatedAt"])
	assert.InDelta(t, putLEA, getLEA, 0, "Get must echo the same evaluation timestamp recorded by Put")

	deleteRec := doAccuracy(t, h, "DeleteLifecyclePolicy", map[string]any{"repositoryName": "lea-repo"})
	require.Equal(t, http.StatusOK, deleteRec.Code)
	deleteOut := parseAccuracy(t, deleteRec)
	_, ok = deleteOut["lastEvaluatedAt"].(float64)
	assert.True(t, ok,
		"DeleteLifecyclePolicy lastEvaluatedAt must be a JSON number, got %T", deleteOut["lastEvaluatedAt"])
}

// TestStartLifecyclePolicyPreview_ResponseShape locks that
// StartLifecyclePolicyPreview's response contains ONLY
// lifecyclePolicyText/repositoryName/registryId/status. Confirmed by direct
// diff of StartLifecyclePolicyPreviewOutput /
// awsAwsjson11_deserializeOpDocumentStartLifecyclePolicyPreviewOutput in
// aws-sdk-go-v2/service/ecr@v1.60.4: unlike GetLifecyclePolicyPreview, Start
// does NOT return previewResults or summary -- those must be retrieved
// separately via GetLifecyclePolicyPreview (see
// TestLifecyclePolicyPreview_EntryShape below). The previous implementation
// leaked previewResults/summary into Start's response via a shared view
// type; this test previously asserted that invented shape.
func TestStartLifecyclePolicyPreview_ResponseShape(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "start-shape-repo")
	mustPutImage(t, h, "start-shape-repo", "v1", `{"schemaVersion":2,"v":1}`)

	policy := `{"rules":[{"rulePriority":7,"description":"expire all",` +
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":0},` +
		`"action":{"type":"expire"}}]}`

	rec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName":      "start-shape-repo",
		"lifecyclePolicyText": policy,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "start-shape-repo", out["repositoryName"])
	assert.Equal(t, policy, out["lifecyclePolicyText"])
	assert.Equal(t, "COMPLETE", out["status"])
	assert.NotContains(t, out, "previewResults",
		"StartLifecyclePolicyPreview must not return previewResults; see GetLifecyclePolicyPreview")
	assert.NotContains(t, out, "summary",
		"StartLifecyclePolicyPreview must not return summary; see GetLifecyclePolicyPreview")
}

// TestLifecyclePolicyPreview_EntryShape locks the full per-image preview wire
// shape returned by GetLifecyclePolicyPreview: real AWS's
// GetLifecyclePolicyPreviewOutput.previewResults is a list of {action,
// appliedRulePriority, imageDigest, imagePushedAt, imageTags, storageClass}
// objects (types.LifecyclePolicyPreviewResult), not the bare {imageDigest,
// imageTag} ImageIdentifier shape. It also locks the top-level
// "summary.expiringImageTotalCount" field (types.LifecyclePolicyPreviewSummary).
func TestLifecyclePolicyPreview_EntryShape(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "entry-shape-repo")
	digest := mustPutImage(t, h, "entry-shape-repo", "v1", `{"schemaVersion":2,"v":1}`)

	policy := `{"rules":[{"rulePriority":7,"description":"expire all",` +
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":0},` +
		`"action":{"type":"expire"}}]}`

	startRec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName":      "entry-shape-repo",
		"lifecyclePolicyText": policy,
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	rec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "entry-shape-repo",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)

	summary, ok := out["summary"].(map[string]any)
	require.True(t, ok, "summary must be an object")
	assert.InDelta(t, float64(1), summary["expiringImageTotalCount"], 0,
		"summary.expiringImageTotalCount must count the one previewed image")

	results, ok := out["previewResults"].([]any)
	require.True(t, ok)
	require.Len(t, results, 1)

	entry, ok := results[0].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, digest, entry["imageDigest"], "imageDigest must be the pushed image's digest key")
	assert.Contains(t, entry["imageTags"], "v1", "imageTags must include the image's tag")
	assert.Equal(t, "STANDARD", entry["storageClass"], "default storage class must be STANDARD")
	assert.InDelta(t, float64(7), entry["appliedRulePriority"], 0,
		"appliedRulePriority must reflect the matched rule's rulePriority")

	action, ok := entry["action"].(map[string]any)
	require.True(t, ok, "action must be an object, not a bare string")
	assert.Equal(t, "EXPIRE", action["type"])

	pushedAt, ok := entry["imagePushedAt"].(float64)
	require.True(t, ok, "imagePushedAt must be a JSON number, got %T", entry["imagePushedAt"])
	assert.Positive(t, pushedAt)
}

// expireAllPolicy is a lifecycle policy that matches every image regardless
// of tag status, used by the GetLifecyclePolicyPreview request-param tests
// below so every pushed image appears in the preview.
const expireAllPolicy = `{"rules":[{"rulePriority":1,"description":"expire all",` +
	`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":0},` +
	`"action":{"type":"expire"}}]}`

// TestGetLifecyclePolicyPreview_FilterTagStatus locks the real API's
// filter.tagStatus request parameter, previously ignored entirely (gopherstack
// always returned the full unfiltered preview result set).
func TestGetLifecyclePolicyPreview_FilterTagStatus(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "preview-filter-repo")
	mustPutImage(t, h, "preview-filter-repo", "v1", `{"schemaVersion":2,"v":1}`)
	mustPutImage(t, h, "preview-filter-repo", "v2", `{"schemaVersion":2,"v":2}`)
	mustPutImage(t, h, "preview-filter-repo", "", `{"schemaVersion":2,"v":3}`)

	startRec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName":      "preview-filter-repo",
		"lifecyclePolicyText": expireAllPolicy,
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	taggedRec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "preview-filter-repo",
		"filter":         map[string]any{"tagStatus": "TAGGED"},
	})
	require.Equal(t, http.StatusOK, taggedRec.Code)
	taggedResults := parseAccuracy(t, taggedRec)["previewResults"].([]any)
	assert.Len(t, taggedResults, 2, "TAGGED filter must return only the two tagged images")

	untaggedRec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "preview-filter-repo",
		"filter":         map[string]any{"tagStatus": "UNTAGGED"},
	})
	require.Equal(t, http.StatusOK, untaggedRec.Code)
	untaggedResults := parseAccuracy(t, untaggedRec)["previewResults"].([]any)
	assert.Len(t, untaggedResults, 1, "UNTAGGED filter must return only the one untagged image")
}

// TestGetLifecyclePolicyPreview_ImageIds locks the real API's imageIds
// request parameter: when supplied, the preview is restricted to exactly
// those images (and MaxResults/NextToken are ignored, per the real API doc).
func TestGetLifecyclePolicyPreview_ImageIds(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "preview-imageids-repo")
	digest1 := mustPutImage(t, h, "preview-imageids-repo", "v1", `{"schemaVersion":2,"v":1}`)
	mustPutImage(t, h, "preview-imageids-repo", "v2", `{"schemaVersion":2,"v":2}`)

	startRec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName":      "preview-imageids-repo",
		"lifecyclePolicyText": expireAllPolicy,
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	rec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "preview-imageids-repo",
		"imageIds":       []map[string]any{{"imageDigest": digest1}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	results := out["previewResults"].([]any)
	require.Len(t, results, 1)
	assert.Equal(t, digest1, results[0].(map[string]any)["imageDigest"])
}

// TestGetLifecyclePolicyPreview_Pagination locks the real API's
// maxResults/nextToken request parameters, previously ignored (gopherstack
// always returned the full unpaginated preview result set).
func TestGetLifecyclePolicyPreview_Pagination(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "preview-page-repo")
	for i := range 3 {
		mustPutImage(t, h, "preview-page-repo", "",
			`{"schemaVersion":2,"idx":`+string(rune('0'+i))+`}`)
	}

	startRec := doAccuracy(t, h, "StartLifecyclePolicyPreview", map[string]any{
		"repositoryName":      "preview-page-repo",
		"lifecyclePolicyText": expireAllPolicy,
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	firstRec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
		"repositoryName": "preview-page-repo",
		"maxResults":     1,
	})
	require.Equal(t, http.StatusOK, firstRec.Code)
	firstOut := parseAccuracy(t, firstRec)
	firstResults := firstOut["previewResults"].([]any)
	require.Len(t, firstResults, 1, "maxResults=1 must return exactly one entry")
	nextToken, ok := firstOut["nextToken"].(string)
	require.True(t, ok, "nextToken must be present when more results remain")
	require.NotEmpty(t, nextToken)

	seen := map[string]bool{firstResults[0].(map[string]any)["imageDigest"].(string): true}

	for range 2 {
		rec := doAccuracy(t, h, "GetLifecyclePolicyPreview", map[string]any{
			"repositoryName": "preview-page-repo",
			"maxResults":     1,
			"nextToken":      nextToken,
		})
		require.Equal(t, http.StatusOK, rec.Code)
		out := parseAccuracy(t, rec)
		results := out["previewResults"].([]any)
		require.Len(t, results, 1)
		seen[results[0].(map[string]any)["imageDigest"].(string)] = true
		nextToken, _ = out["nextToken"].(string)
	}

	assert.Len(t, seen, 3, "paginating through maxResults=1 pages must cover all 3 images exactly once")
	assert.Empty(t, nextToken, "nextToken must be empty once the last page has been returned")
}
