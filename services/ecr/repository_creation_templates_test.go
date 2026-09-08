package ecr_test

// repository_creation_templates_test.go — verifies
// repository_creation_templates.go: Create/Describe/Update/Delete templates,
// prefix filtering, and duplicate-prefix rejection.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECR_CreateRepositoryCreationTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prefix     string
		wantStatus int
	}{
		{
			name:       "creates template successfully",
			prefix:     "my-org",
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty prefix returns error",
			prefix:     "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doECRRequest(t, h, "CreateRepositoryCreationTemplate", map[string]any{
				"prefix": tt.prefix,
				"resourceTags": []map[string]any{
					{"Key": "env", "Value": "test"},
				},
			})
			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				out := parseAccuracy(t, rec)
				tmpl, ok := out["repositoryCreationTemplate"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.prefix, tmpl["prefix"])
				require.IsType(t, []any{}, tmpl["resourceTags"])
			}
		})
	}
}

func TestRepositoryCreationTemplate_DuplicatePrefixRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prefix     string
		wantFirst  int
		wantSecond int
	}{
		{
			name:       "duplicate_prefix",
			prefix:     "my-prefix",
			wantFirst:  http.StatusOK,
			wantSecond: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAccuracyHandler()

			rec1 := doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
				"prefix": tt.prefix,
			})
			assert.Equal(t, tt.wantFirst, rec1.Code)

			rec2 := doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
				"prefix": tt.prefix,
			})
			assert.Equal(t, tt.wantSecond, rec2.Code)
		})
	}
}

func TestRepositoryCreationTemplate_CreateDescribeDelete(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	createRec := doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
		"prefix":             "prod/",
		"description":        "Production repos",
		"imageTagMutability": "IMMUTABLE",
		"appliedFor":         []string{"REPLICATION", "PULL_THROUGH_CACHE"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	descRec := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{})
	require.Equal(t, http.StatusOK, descRec.Code)
	out := parseAccuracy(t, descRec)
	tmpls, _ := out["repositoryCreationTemplates"].([]any)
	assert.Len(t, tmpls, 1)
	tmpl := tmpls[0].(map[string]any)
	assert.Equal(t, "prod/", tmpl["prefix"])
	assert.Equal(t, "IMMUTABLE", tmpl["imageTagMutability"])

	delRec := doAccuracy(t, h, "DeleteRepositoryCreationTemplate", map[string]any{
		"prefix": "prod/",
	})
	require.Equal(t, http.StatusOK, delRec.Code)

	descRec2 := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{})
	require.Equal(t, http.StatusOK, descRec2.Code)
	out2 := parseAccuracy(t, descRec2)
	tmpls2, _ := out2["repositoryCreationTemplates"].([]any)
	assert.Empty(t, tmpls2)
}

func TestRepositoryCreationTemplate_Update(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
		"prefix":             "staging/",
		"imageTagMutability": "MUTABLE",
	})

	updateRec := doAccuracy(t, h, "UpdateRepositoryCreationTemplate", map[string]any{
		"prefix":             "staging/",
		"imageTagMutability": "IMMUTABLE",
		"description":        "updated staging",
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	descRec := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{
		"prefixes": []string{"staging/"},
	})
	require.Equal(t, http.StatusOK, descRec.Code)
	out := parseAccuracy(t, descRec)
	tmpls, _ := out["repositoryCreationTemplates"].([]any)
	require.Len(t, tmpls, 1)
	tmpl := tmpls[0].(map[string]any)
	assert.Equal(t, "IMMUTABLE", tmpl["imageTagMutability"])
	assert.Equal(t, "updated staging", tmpl["description"])
}

func TestDescribeRepositoryCreationTemplates_FilterByPrefix(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for _, prefix := range []string{"app/", "infra/", "data/"} {
		doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
			"prefix": prefix,
		})
	}

	rec := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{
		"prefixes": []string{"infra/"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	tmpls, _ := out["repositoryCreationTemplates"].([]any)
	require.Len(t, tmpls, 1, "filter by prefix must return only the matching template")
	tmpl := tmpls[0].(map[string]any)
	assert.Equal(t, "infra/", tmpl["prefix"])
}

func TestDescribeRepositoryCreationTemplates_UnmatchedPrefix_FilteredNotError(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
		"prefix": "exists/",
	})

	// DescribeRepositoryCreationTemplates declares no TemplateNotFoundException
	// (unlike Delete/UpdateRepositoryCreationTemplate, per
	// deserializeOpErrorDescribeRepositoryCreationTemplates); an unmatched
	// prefix must be silently omitted, not rejected.
	rec := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{
		"prefixes": []string{"exists/", "does-not-exist/"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.NotEqual(t, "TemplateNotFoundException", out["__type"])
	tmpls, _ := out["repositoryCreationTemplates"].([]any)
	require.Len(t, tmpls, 1, "unmatched prefix must be filtered out, matching prefix must still be returned")
	tmpl := tmpls[0].(map[string]any)
	assert.Equal(t, "exists/", tmpl["prefix"])
}

func TestDescribeRepositoryCreationTemplates_AllPrefixesUnmatched_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	rec := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{
		"prefixes": []string{"does-not-exist/"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	tmpls, _ := out["repositoryCreationTemplates"].([]any)
	assert.Empty(t, tmpls)
}

func TestDescribeRepositoryCreationTemplates_NoFilter_ReturnsAll(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for _, prefix := range []string{"team-a/", "team-b/"} {
		doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
			"prefix": prefix,
		})
	}

	rec := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	tmpls, _ := out["repositoryCreationTemplates"].([]any)
	assert.Len(t, tmpls, 2)
}

func TestDescribeRepositoryCreationTemplates_RegistryID_Present(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
		"prefix": "check-registry/",
	})

	rec := doAccuracy(t, h, "DescribeRepositoryCreationTemplates", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	assert.Equal(t, "123456789012", out["registryId"],
		"DescribeRepositoryCreationTemplates must include registryId")
}

func TestUpdateRepositoryCreationTemplate_DescriptionUpdated(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
		"prefix":      "update-tmpl/",
		"description": "original",
	})

	rec := doAccuracy(t, h, "UpdateRepositoryCreationTemplate", map[string]any{
		"prefix":      "update-tmpl/",
		"description": "updated",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	tmpl, _ := out["repositoryCreationTemplate"].(map[string]any)
	assert.Equal(t, "updated", tmpl["description"])
	assert.Equal(t, "update-tmpl/", tmpl["prefix"])
}

func TestUpdateRepositoryCreationTemplate_NotFound_Errors(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	rec := doAccuracy(t, h, "UpdateRepositoryCreationTemplate", map[string]any{
		"prefix":      "no-such-template/",
		"description": "new desc",
	})
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestUpdateRepositoryCreationTemplate_AppliedForPreserved(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	doAccuracy(t, h, "CreateRepositoryCreationTemplate", map[string]any{
		"prefix":     "applied-tmpl/",
		"appliedFor": []string{"REPLICATION", "PULL_THROUGH_CACHE"},
	})

	rec := doAccuracy(t, h, "UpdateRepositoryCreationTemplate", map[string]any{
		"prefix":      "applied-tmpl/",
		"description": "changed desc",
		"appliedFor":  []string{"REPLICATION", "PULL_THROUGH_CACHE"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	tmpl, _ := out["repositoryCreationTemplate"].(map[string]any)
	appliedFor, _ := tmpl["appliedFor"].([]any)
	assert.Len(t, appliedFor, 2)
}
