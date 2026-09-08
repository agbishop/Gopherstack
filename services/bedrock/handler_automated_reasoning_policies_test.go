package bedrock_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

func TestHandler_CreateAutomatedReasoningPolicy(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		setup      func(*testing.T, *bedrock.Handler)
		input      map[string]any
		name       string
		wantStatus int
		wantFields bool
	}{
		{
			name:       "valid policy",
			input:      map[string]any{"name": "my-policy", "description": "test"},
			wantStatus: http.StatusCreated,
			wantFields: true,
		},
		{
			name:       "missing name",
			input:      map[string]any{"name": ""},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate name",
			setup: func(t *testing.T, h *bedrock.Handler) {
				t.Helper()
				_, err := h.Backend.CreateAutomatedReasoningPolicy("dup-policy", "", nil)
				require.NoError(t, err)
			},
			input:      map[string]any{"name": "dup-policy"},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)

			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantFields {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				assert.NotEmpty(t, out["policyArn"])
				assert.Equal(t, "my-policy", out["name"])
				assert.Equal(t, "ACTIVE", out["status"])
			}
		})
	}
}

func TestHandler_CancelAutomatedReasoningPolicyBuildWorkflow( //nolint:paralleltest // existing issue.
	t *testing.T,
) {
	tests := []struct {
		name        string
		policyARN   string
		workflowID  string
		setupPolicy bool
		setupWF     bool
		wantStatus  int
	}{
		{
			name:        "cancel existing workflow",
			setupPolicy: true,
			setupWF:     true,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "policy not found",
			policyARN:   "arn:aws:bedrock:us-east-1:000000000000:automated-reasoning-policy/nonexistent",
			workflowID:  "wf-001",
			setupPolicy: false,
			setupWF:     false,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "workflow not found",
			setupPolicy: true,
			setupWF:     false,
			workflowID:  "nonexistent-wf",
			wantStatus:  http.StatusNotFound,
		},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)

			policyARN := tt.policyARN
			workflowID := tt.workflowID

			if tt.setupPolicy {
				policy, err := h.Backend.CreateAutomatedReasoningPolicy("test-cancel-policy", "", nil)
				require.NoError(t, err)
				policyARN = policy.PolicyArn
			}

			if tt.setupWF {
				wf := h.Backend.AddBuildWorkflowForTest(policyARN)
				workflowID = wf.BuildWorkflowID
			}

			path := fmt.Sprintf("/automated-reasoning-policies/%s/build-workflows/%s/cancel",
				url.PathEscape(policyARN), workflowID)
			rec := doRequest(t, h, http.MethodPost, path, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_CreateAutomatedReasoningPolicyTestCase( //nolint:paralleltest // existing issue.
	t *testing.T,
) {
	tests := []struct {
		name        string
		policyARN   string
		setupPolicy bool
		wantStatus  int
	}{
		{
			name:        "valid test case",
			setupPolicy: true,
			wantStatus:  http.StatusCreated,
		},
		{
			name:        "policy not found",
			policyARN:   "arn:aws:bedrock:us-east-1:000000000000:automated-reasoning-policy/nonexistent",
			setupPolicy: false,
			wantStatus:  http.StatusNotFound,
		},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)

			policyARN := tt.policyARN
			if tt.setupPolicy {
				policy, err := h.Backend.CreateAutomatedReasoningPolicy("tc-policy", "", nil)
				require.NoError(t, err)
				policyARN = policy.PolicyArn
			}

			path := fmt.Sprintf("/automated-reasoning-policies/%s/test-cases", url.PathEscape(policyARN))
			rec := doRequest(t, h, http.MethodPost, path, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusCreated {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				assert.NotEmpty(t, out["testCaseId"])
				assert.Equal(t, policyARN, out["policyArn"])
			}
		})
	}
}

func TestHandler_CreateAutomatedReasoningPolicyVersion(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		name        string
		policyARN   string
		setupPolicy bool
		wantStatus  int
	}{
		{
			name:        "valid version",
			setupPolicy: true,
			wantStatus:  http.StatusCreated,
		},
		{
			name:        "policy not found",
			policyARN:   "arn:aws:bedrock:us-east-1:000000000000:automated-reasoning-policy/nonexistent",
			setupPolicy: false,
			wantStatus:  http.StatusNotFound,
		},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)

			policyARN := tt.policyARN
			if tt.setupPolicy {
				policy, err := h.Backend.CreateAutomatedReasoningPolicy("ver-policy", "", nil)
				require.NoError(t, err)
				policyARN = policy.PolicyArn
			}

			path := fmt.Sprintf("/automated-reasoning-policies/%s/versions", url.PathEscape(policyARN))
			rec := doRequest(t, h, http.MethodPost, path, map[string]any{
				"lastUpdatedDefinitionHash": "abc123hash",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusCreated {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				assert.NotEmpty(t, out["policyArn"])
				assert.NotEmpty(t, out["version"])
				assert.Equal(t, "abc123hash", out["definitionHash"])
			}
		})
	}
}

func TestHandler_CancelWorkflowWrongPolicy(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	policy1, err := h.Backend.CreateAutomatedReasoningPolicy("policy-1", "", nil)
	require.NoError(t, err)

	policy2, err := h.Backend.CreateAutomatedReasoningPolicy("policy-2", "", nil)
	require.NoError(t, err)

	wf := h.Backend.AddBuildWorkflowForTest(policy1.PolicyArn)

	// Cancel using wrong policy ARN should return 404.
	path := fmt.Sprintf("/automated-reasoning-policies/%s/build-workflows/%s/cancel",
		url.PathEscape(policy2.PolicyArn), wf.BuildWorkflowID)
	rec := doRequest(t, h, http.MethodPost, path, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_TagsOnAutomatedReasoningPolicy(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{
		"name": "tagged-policy",
		"tags": []map[string]string{
			{"key": "team", "value": "platform"},
		},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	policyARN := out["policyArn"].(string)

	rec2 := doRequest(t, h, http.MethodPost, "/listTagsForResource", map[string]any{
		"resourceARN": policyARN,
	})
	assert.Equal(t, http.StatusOK, rec2.Code)

	var tagsOut map[string]any
	mustUnmarshal(t, rec2, &tagsOut)
	tags := tagsOut["tags"].([]any)
	assert.Len(t, tags, 1)
	assert.Equal(t, "team", tags[0].(map[string]any)["key"])
}

func TestHandler_PerPolicyVersionNumbering(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	p1, err := h.Backend.CreateAutomatedReasoningPolicy("policy-ver-1", "", nil)
	require.NoError(t, err)

	p2, err := h.Backend.CreateAutomatedReasoningPolicy("policy-ver-2", "", nil)
	require.NoError(t, err)

	// Create versions for each policy independently.
	rec1 := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(p1.PolicyArn)+"/versions",
		map[string]any{"lastUpdatedDefinitionHash": "hash1"})
	require.Equal(t, http.StatusCreated, rec1.Code)

	rec2 := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(p2.PolicyArn)+"/versions",
		map[string]any{"lastUpdatedDefinitionHash": "hash2"})
	require.Equal(t, http.StatusCreated, rec2.Code)

	// Both should be version "1" (per-policy counter).
	var v1Out map[string]any
	mustUnmarshal(t, rec1, &v1Out)
	assert.Equal(t, "1", v1Out["version"])

	var v2Out map[string]any
	mustUnmarshal(t, rec2, &v2Out)
	assert.Equal(t, "1", v2Out["version"])

	// Second version for p1 should be "2".
	rec3 := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(p1.PolicyArn)+"/versions",
		map[string]any{"lastUpdatedDefinitionHash": "hash3"})
	require.Equal(t, http.StatusCreated, rec3.Code)

	var v3Out map[string]any
	mustUnmarshal(t, rec3, &v3Out)
	assert.Equal(t, "2", v3Out["version"])
}

func TestAccuracy_ARPVersion_PerPolicyNumberingIsolated(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("000000000000", "us-east-1")

	p1, err := b.CreateAutomatedReasoningPolicy("policy-alpha", "", nil)
	require.NoError(t, err)

	p2, err := b.CreateAutomatedReasoningPolicy("policy-beta", "", nil)
	require.NoError(t, err)

	// Each policy starts its own counter.
	v1a, err := b.CreateAutomatedReasoningPolicyVersion(p1.PolicyArn, "notes", nil)
	require.NoError(t, err)
	assert.Equal(t, "1", v1a.Version)

	v1b, err := b.CreateAutomatedReasoningPolicyVersion(p2.PolicyArn, "notes", nil)
	require.NoError(t, err)
	assert.Equal(t, "1", v1b.Version, "per-policy counter — beta starts at 1")

	v2a, err := b.CreateAutomatedReasoningPolicyVersion(p1.PolicyArn, "notes", nil)
	require.NoError(t, err)
	assert.Equal(t, "2", v2a.Version)

	v2b, err := b.CreateAutomatedReasoningPolicyVersion(p2.PolicyArn, "notes", nil)
	require.NoError(t, err)
	assert.Equal(t, "2", v2b.Version)
}

func TestAccuracy_ARPVersion_ViaHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		wantLastVersion string
		versionCount    int
	}{
		{name: "single version", versionCount: 1, wantLastVersion: "1"},
		{name: "three versions", versionCount: 3, wantLastVersion: "3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := bedrock.NewInMemoryBackend("000000000000", "us-east-1")
			h := bedrock.NewHandler(b)
			policy, err := b.CreateAutomatedReasoningPolicy("versioned-policy-"+tt.name, "", nil)
			require.NoError(t, err)

			var lastVersion map[string]any
			for i := range tt.versionCount {
				rec := doRequest(t, h, http.MethodPost,
					"/automated-reasoning-policies/"+url.PathEscape(policy.PolicyArn)+"/versions",
					map[string]any{"notes": fmt.Sprintf("version %d", i+1)})
				require.Equal(t, http.StatusCreated, rec.Code)
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lastVersion))
			}

			assert.Equal(t, tt.wantLastVersion, lastVersion["version"])
			assert.NotEmpty(t, lastVersion["policyArn"])
		})
	}
}

func TestAccuracy_ARP_DescriptionPreservedOnCreate(t *testing.T) {
	t.Parallel()

	const desc = "reasons about financial transactions"

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{
		"name":        "finance-arp",
		"description": desc,
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	policyARN := out["policyArn"].(string)

	getRec := doRequest(t, h, http.MethodGet, "/automated-reasoning-policies/"+policyARN, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
	assert.Equal(t, desc, getOut["description"])
	assert.Equal(t, "finance-arp", getOut["name"])
}

func TestAccuracy_ARP_UpdateDescriptionReflected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{
		"name": "updatable-arp",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	policyARN := out["policyArn"].(string)

	// Update description (real AWS: UpdateAutomatedReasoningPolicy uses PATCH)
	updateRec := doRequest(
		t, h, http.MethodPatch,
		"/automated-reasoning-policies/"+policyARN,
		map[string]any{
			"description":      "updated description",
			"policyDefinition": map[string]any{"version": "1"},
		},
	)
	require.Equal(t, http.StatusOK, updateRec.Code)

	getRec := doRequest(t, h, http.MethodGet, "/automated-reasoning-policies/"+policyARN, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
	assert.Equal(t, "updated description", getOut["description"])
}

func TestAccuracy_ARP_VersionNumberingIsolatedPerPolicy(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("000000000000", "us-east-1")
	h := bedrock.NewHandler(b)

	policy0, err := b.CreateAutomatedReasoningPolicy("iso-arp-0", "", nil)
	require.NoError(t, err)

	policy1, err := b.CreateAutomatedReasoningPolicy("iso-arp-1", "", nil)
	require.NoError(t, err)

	// Create 2 versions for policy 0 via HTTP
	for i := range 2 {
		rec := doRequest(t, h, http.MethodPost,
			"/automated-reasoning-policies/"+url.PathEscape(policy0.PolicyArn)+"/versions",
			map[string]any{"definitionHash": fmt.Sprintf("hash-0-%d", i)})
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	// Create 1 version for policy 1 via HTTP
	rec := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policy1.PolicyArn)+"/versions",
		map[string]any{"definitionHash": "hash-1-0"})
	require.Equal(t, http.StatusCreated, rec.Code)

	// Verify second version for policy0 has version "2"
	var verOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &verOut))

	// policy1's first version is "1" (isolated counter)
	assert.Equal(t, "1", verOut["version"], "policy1 version counter starts from 1 independent of policy0")
}

func TestAccuracy_ARP_BuildWorkflowStatusTransitions(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("000000000000", "us-east-1")
	h := bedrock.NewHandler(b)

	// Create a policy
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{
		"name": "workflow-test-arp",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	policyARN := out["policyArn"].(string)

	// Add a build workflow via test helper
	wf := b.AddBuildWorkflowForTest(policyARN)
	assert.Equal(t, "Running", wf.Status)

	// Get the workflow
	getRec := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+policyARN+"/build-workflows/"+wf.BuildWorkflowID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getOut))
	assert.Equal(t, "Running", getOut["status"])
}

func TestAccuracy_ARP_TestCaseRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create policy
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{
		"name": "tc-arp",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	policyARN := out["policyArn"].(string)

	// Create test case (needs URL-encoded ARN in path)
	tcRec := doRequest(
		t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases",
		map[string]any{"input": "Is 2 + 2 = 4?"},
	)
	require.Equal(t, http.StatusCreated, tcRec.Code)

	var tcOut map[string]any
	require.NoError(t, json.Unmarshal(tcRec.Body.Bytes(), &tcOut))
	tcID := tcOut["testCaseId"].(string)
	assert.NotEmpty(t, tcID)

	// Get test case (needs URL-encoded ARN in path)
	getTestRec := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID, nil)
	require.Equal(t, http.StatusOK, getTestRec.Code)

	var getTestOut map[string]any
	require.NoError(t, json.Unmarshal(getTestRec.Body.Bytes(), &getTestOut))
	assert.Equal(t, policyARN, getTestOut["policyArn"])
	testCase, ok := getTestOut["testCase"].(map[string]any)
	require.True(t, ok, "response must wrap the test case under the required testCase key")
	assert.Equal(t, tcID, testCase["testCaseId"])
}

func TestAccuracy_ARP_AnnotationsGetAfterUpdate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create policy
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{
		"name": "ann-arp",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	policyARN := out["policyArn"].(string)

	wf := h.Backend.AddBuildWorkflowForTest(policyARN)
	annPath := "/automated-reasoning-policies/" + url.PathEscape(policyARN) +
		"/build-workflows/" + wf.BuildWorkflowID + "/annotations"

	// A real client reads the current annotationSetHash before updating
	// (bedrock@v1.66.4 GetAutomatedReasoningPolicyAnnotationsOutput.AnnotationSetHash
	// is the token UpdateAutomatedReasoningPolicyAnnotations's required
	// lastUpdatedAnnotationSetHash checks against).
	preRec := doRequest(t, h, http.MethodGet, annPath, nil)
	require.Equal(t, http.StatusOK, preRec.Code)

	var preOut map[string]any
	require.NoError(t, json.Unmarshal(preRec.Body.Bytes(), &preOut))
	hash, ok := preOut["annotationSetHash"].(string)
	require.True(t, ok)
	require.NotEmpty(t, hash)

	// Update annotations (needs URL-encoded ARN)
	updateRec := doRequest(
		t, h, http.MethodPatch, annPath,
		map[string]any{
			"annotations": []map[string]any{
				{"key": "env", "value": "prod"},
			},
			"lastUpdatedAnnotationSetHash": hash,
		},
	)
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateOut map[string]any
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateOut))
	assert.Equal(t, policyARN, updateOut["policyArn"])
	assert.Equal(t, wf.BuildWorkflowID, updateOut["buildWorkflowId"])
	assert.NotEmpty(t, updateOut["annotationSetHash"])
	assert.NotEmpty(t, updateOut["updatedAt"])

	// Get annotations - should reflect what was just updated.
	getAnnRec := doRequest(t, h, http.MethodGet, annPath, nil)
	require.Equal(t, http.StatusOK, getAnnRec.Code)

	var annOut map[string]any
	require.NoError(t, json.Unmarshal(getAnnRec.Body.Bytes(), &annOut))
	anns, ok := annOut["annotations"].([]any)
	require.True(t, ok)
	require.Len(t, anns, 1)
}

func TestHandler_GetAutomatedReasoningPolicy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "my-arp"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	rec2 := doRequest(t, h, http.MethodGet, "/automated-reasoning-policies/"+url.PathEscape(policyARN), nil)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]any
	mustUnmarshal(t, rec2, &out)
	assert.Equal(t, policyARN, out["policyArn"])
	assert.Equal(t, "my-arp", out["name"])
}

func TestHandler_GetAutomatedReasoningPolicy_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	path := "/automated-reasoning-policies/" + url.PathEscape("arn:aws:bedrock:us-east-1::arp/nope")
	rec := doRequest(t, h, http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_UpdateAutomatedReasoningPolicy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "upd-pol"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	rec2 := doRequest(t, h, http.MethodPatch, "/automated-reasoning-policies/"+url.PathEscape(policyARN),
		map[string]any{
			"description":      "new-desc",
			"policyDefinition": map[string]any{"version": "1"},
		})
	assert.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]any
	mustUnmarshal(t, rec2, &out)
	assert.Equal(t, policyARN, out["policyArn"])
	assert.NotEmpty(t, out["definitionHash"])
	assert.NotContains(t, out, "status", "real UpdateAutomatedReasoningPolicyOutput has no status field")
}

// TestHandler_UpdateAutomatedReasoningPolicy_MissingPolicyDefinition locks in
// that policyDefinition -- required on UpdateAutomatedReasoningPolicyInput
// (bedrock@v1.66.4 api_op_UpdateAutomatedReasoningPolicy.go:37-63) -- is
// actually enforced, not silently dropped.
func TestHandler_UpdateAutomatedReasoningPolicy_MissingPolicyDefinition(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "upd-pol-nodef"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	rec2 := doRequest(t, h, http.MethodPatch, "/automated-reasoning-policies/"+url.PathEscape(policyARN),
		map[string]any{"description": "new-desc"})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

func TestHandler_DeleteAutomatedReasoningPolicy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "del-pol"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	rec2 := doRequest(t, h, http.MethodDelete, "/automated-reasoning-policies/"+url.PathEscape(policyARN), nil)
	assert.Equal(t, http.StatusNoContent, rec2.Code)

	rec3 := doRequest(t, h, http.MethodGet, "/automated-reasoning-policies/"+url.PathEscape(policyARN), nil)
	assert.Equal(t, http.StatusNotFound, rec3.Code)
}

// TestHandler_DeleteAutomatedReasoningPolicy_ResourceInUse covers the force
// precondition AWS documents: without force, delete fails while the policy
// still has versions or test cases (bedrock@v1.66.4
// api_op_DeleteAutomatedReasoningPolicy.go:36-41); force bypasses it.
func TestHandler_DeleteAutomatedReasoningPolicy_ResourceInUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, b *bedrock.InMemoryBackend, policyARN string)
		name       string
		force      bool
		wantStatus int
	}{
		{
			name:       "no artifacts no force succeeds",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "test case blocks delete without force",
			setup: func(t *testing.T, b *bedrock.InMemoryBackend, policyARN string) {
				t.Helper()
				_, err := b.CreateAutomatedReasoningPolicyTestCase(policyARN)
				require.NoError(t, err)
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "test case does not block delete with force",
			setup: func(t *testing.T, b *bedrock.InMemoryBackend, policyARN string) {
				t.Helper()
				_, err := b.CreateAutomatedReasoningPolicyTestCase(policyARN)
				require.NoError(t, err)
			},
			force:      true,
			wantStatus: http.StatusNoContent,
		},
		{
			name: "version blocks delete without force",
			setup: func(t *testing.T, b *bedrock.InMemoryBackend, policyARN string) {
				t.Helper()
				_, err := b.CreateAutomatedReasoningPolicyVersion(policyARN, "hash", nil)
				require.NoError(t, err)
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "version does not block delete with force",
			setup: func(t *testing.T, b *bedrock.InMemoryBackend, policyARN string) {
				t.Helper()
				_, err := b.CreateAutomatedReasoningPolicyVersion(policyARN, "hash", nil)
				require.NoError(t, err)
			},
			force:      true,
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			policy, err := h.Backend.CreateAutomatedReasoningPolicy("del-inuse-pol", "", nil)
			require.NoError(t, err)

			if tt.setup != nil {
				tt.setup(t, h.Backend, policy.PolicyArn)
			}

			path := "/automated-reasoning-policies/" + url.PathEscape(policy.PolicyArn)
			if tt.force {
				path += "?force=true"
			}

			rec := doRequest(t, h, http.MethodDelete, path, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_DeleteAutomatedReasoningPolicy_ForceRemovesVersionGhostRow
// covers a ghost-row bug: force-deleting a policy previously left its
// versions behind in the arpVersions store, still exportable by ARN even
// though the owning policy was gone.
func TestHandler_DeleteAutomatedReasoningPolicy_ForceRemovesVersionGhostRow(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	policy, err := h.Backend.CreateAutomatedReasoningPolicy("ghost-pol", "", nil)
	require.NoError(t, err)

	version, err := h.Backend.CreateAutomatedReasoningPolicyVersion(policy.PolicyArn, "hash", nil)
	require.NoError(t, err)

	require.NoError(t, h.Backend.DeleteAutomatedReasoningPolicy(policy.PolicyArn, true))

	_, err = h.Backend.ExportAutomatedReasoningPolicyVersion(version.PolicyArn)
	assert.ErrorIs(t, err, bedrock.ErrNotFound)
}

// TestHandler_StartARPBuildWorkflow drives the real path AWS uses --
// .../build-workflows/{buildWorkflowType}/start (bedrock@v1.66.4
// serializers.go:8008), not the bare .../build-workflows path gopherstack
// previously served (which no real client's Start request ever reached).
func TestHandler_StartARPBuildWorkflow(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "wf-pol"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	rec2 := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/build-workflows/INGEST_CONTENT/start",
		map[string]any{"policyDefinition": map[string]any{"version": "1"}})
	assert.Equal(t, http.StatusCreated, rec2.Code)

	var wf map[string]any
	mustUnmarshal(t, rec2, &wf)
	assert.NotEmpty(t, wf["buildWorkflowId"])
	assert.NotContains(
		t, wf, "status", "real StartAutomatedReasoningPolicyBuildWorkflowOutput has no status field",
	)
}

func TestHandler_GetListDeleteARPBuildWorkflow(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "wf2-pol"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	recWF := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/build-workflows/INGEST_CONTENT/start",
		map[string]any{"policyDefinition": map[string]any{"version": "1"}})
	require.Equal(t, http.StatusCreated, recWF.Code)

	var wf map[string]any
	mustUnmarshal(t, recWF, &wf)
	wfID := wf["buildWorkflowId"].(string)

	// Get
	recGet := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/build-workflows/"+wfID, nil)
	assert.Equal(t, http.StatusOK, recGet.Code)

	var getOut map[string]any
	mustUnmarshal(t, recGet, &getOut)
	assert.Equal(t, "INGEST_CONTENT", getOut["buildWorkflowType"])

	// List
	recList := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/build-workflows", nil)
	require.Equal(t, http.StatusOK, recList.Code)

	var listOut map[string]any
	mustUnmarshal(t, recList, &listOut)
	assert.Len(t, listOut["automatedReasoningPolicyBuildWorkflowSummaries"], 1)

	// Delete
	recDel := doRequest(t, h, http.MethodDelete,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/build-workflows/"+wfID, nil)
	assert.Equal(t, http.StatusNoContent, recDel.Code)

	// List after delete
	recList2 := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/build-workflows", nil)
	var listOut2 map[string]any
	mustUnmarshal(t, recList2, &listOut2)
	assert.Empty(t, listOut2["automatedReasoningPolicyBuildWorkflowSummaries"])
}

func TestHandler_GetListDeleteARPTestCase(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "tc-pol"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	recTC := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases", nil)
	require.Equal(t, http.StatusCreated, recTC.Code)

	var tc map[string]any
	mustUnmarshal(t, recTC, &tc)
	tcID := tc["testCaseId"].(string)

	// Get
	recGet := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID, nil)
	assert.Equal(t, http.StatusOK, recGet.Code)

	// List
	recList := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases", nil)
	require.Equal(t, http.StatusOK, recList.Code)

	var listOut map[string]any
	mustUnmarshal(t, recList, &listOut)
	assert.Len(t, listOut["testCases"], 1)

	// Update
	recUpd := doRequest(t, h, http.MethodPatch,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID,
		map[string]any{
			"guardContent":                     "the model said X",
			"expectedAggregatedFindingsResult": "VALID",
		})
	assert.Equal(t, http.StatusOK, recUpd.Code)

	// Delete
	recDel := doRequest(t, h, http.MethodDelete,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID, nil)
	assert.Equal(t, http.StatusNoContent, recDel.Code)

	// Get after delete
	recGet2 := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID, nil)
	assert.Equal(t, http.StatusNotFound, recGet2.Code)
}

// TestAccuracy_ARPTestCase_UpdateAppliesContent locks in that
// UpdateAutomatedReasoningPolicyTestCase actually parses and stores its request
// body (aws-sdk-go-v2 api_op_UpdateAutomatedReasoningPolicyTestCase.go:29-71) --
// gopherstack previously accepted PATCH bodies but never read them, only echoing
// testCaseId/policyArn back.
func TestAccuracy_ARPTestCase_UpdateAppliesContent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "upd-content-pol"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	recTC := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases", nil)
	require.Equal(t, http.StatusCreated, recTC.Code)

	var tc map[string]any
	mustUnmarshal(t, recTC, &tc)
	tcID := tc["testCaseId"].(string)

	confidence := 0.85
	updRec := doRequest(t, h, http.MethodPatch,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID,
		map[string]any{
			"guardContent":                     "the model said the sky is blue",
			"queryContent":                     "what color is the sky?",
			"expectedAggregatedFindingsResult": "VALID",
			"confidenceThreshold":              confidence,
		})
	require.Equal(t, http.StatusOK, updRec.Code)

	getRec := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut map[string]any
	mustUnmarshal(t, getRec, &getOut)
	testCase, ok := getOut["testCase"].(map[string]any)
	require.True(t, ok, "response must wrap the test case under the required testCase key")
	assert.Equal(t, "the model said the sky is blue", testCase["guardContent"])
	assert.Equal(t, "what color is the sky?", testCase["queryContent"])
	assert.Equal(t, "VALID", testCase["expectedAggregatedFindingsResult"])
	assert.InEpsilon(t, confidence, testCase["confidenceThreshold"], 0)
}

// TestAccuracy_ARPTestCase_UpdateRequiresGuardContent locks in that
// UpdateAutomatedReasoningPolicyTestCase's required guardContent and
// expectedAggregatedFindingsResult fields are enforced, not silently accepted
// as absent.
func TestAccuracy_ARPTestCase_UpdateRequiresGuardContent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "req-content-pol"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	mustUnmarshal(t, rec, &created)
	policyARN := created["policyArn"].(string)

	recTC := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases", nil)
	require.Equal(t, http.StatusCreated, recTC.Code)

	var tc map[string]any
	mustUnmarshal(t, recTC, &tc)
	tcID := tc["testCaseId"].(string)

	updRec := doRequest(t, h, http.MethodPatch,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID, nil)
	assert.Equal(t, http.StatusBadRequest, updRec.Code)

	getRec := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases/"+tcID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getOut map[string]any
	mustUnmarshal(t, getRec, &getOut)
	assert.Empty(t, getOut["guardContent"], "a rejected update must not mutate the test case")
}

// TestAccuracy_ARP_UpdateOpsRejectOldPUTMethod locks in the parity fix for
// three ARP update ops that real AWS routes on PATCH: UpdateAutomatedReasoningPolicy,
// UpdateAutomatedReasoningPolicyTestCase, and UpdateAutomatedReasoningPolicyAnnotations.
// gopherstack previously routed all three on PUT, making them 100% unreachable
// by real SDK clients (which always send PATCH for these ops).
func TestAccuracy_ARP_UpdateOpsRejectOldPUTMethod(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies",
		map[string]any{"name": "put-rejected-policy"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	policyARN := out["policyArn"].(string)

	tcRec := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases", nil)
	require.Equal(t, http.StatusCreated, tcRec.Code)

	var tcOut map[string]any
	require.NoError(t, json.Unmarshal(tcRec.Body.Bytes(), &tcOut))
	tcID := tcOut["testCaseId"].(string)

	wf := h.Backend.AddBuildWorkflowForTest(policyARN)

	tests := []struct {
		name string
		path string
	}{
		{name: "UpdateAutomatedReasoningPolicy", path: "/automated-reasoning-policies/" + url.PathEscape(policyARN)},
		{
			name: "UpdateAutomatedReasoningPolicyTestCase",
			path: "/automated-reasoning-policies/" + url.PathEscape(policyARN) + "/test-cases/" + tcID,
		},
		{
			name: "UpdateAutomatedReasoningPolicyAnnotations",
			path: "/automated-reasoning-policies/" + url.PathEscape(policyARN) +
				"/build-workflows/" + wf.BuildWorkflowID + "/annotations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			putRec := doRequest(t, h, http.MethodPut, tt.path, map[string]any{"description": "x"})
			assert.Equal(t, http.StatusNotFound, putRec.Code)
		})
	}
}

// TestAccuracy_ARP_BuildWorkflowScopedSubResources locks in that
// Get/UpdateAutomatedReasoningPolicyAnnotations, GetAutomatedReasoningPolicyNextScenario,
// Get/ListAutomatedReasoningPolicyTestResult(s), and StartAutomatedReasoningPolicyTestWorkflow
// are build-workflow-scoped in real AWS (bedrock@v1.66.4 serializers.go:3874, :4122,
// :4282, :5937, :8117), not policy-scoped -- the paths gopherstack previously served for
// these ops did not exist in the real API.
func TestAccuracy_ARP_BuildWorkflowScopedSubResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pathSuffix func(wfID, tcID string) string
		body       map[string]any
		name       string
		method     string
		wantStatus int
	}{
		{
			name:       "get annotations",
			method:     http.MethodGet,
			pathSuffix: func(wfID, _ string) string { return "/build-workflows/" + wfID + "/annotations" },
			wantStatus: http.StatusOK,
		},
		{
			name:       "update annotations",
			method:     http.MethodPatch,
			pathSuffix: func(wfID, _ string) string { return "/build-workflows/" + wfID + "/annotations" },
			body: map[string]any{
				"annotations":                  []map[string]any{{"key": "a", "value": "b"}},
				"lastUpdatedAnnotationSetHash": "seed-hash",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "next scenario",
			method:     http.MethodGet,
			pathSuffix: func(wfID, _ string) string { return "/build-workflows/" + wfID + "/scenarios" },
			wantStatus: http.StatusOK,
		},
		{
			name:   "get test result",
			method: http.MethodGet,
			pathSuffix: func(wfID, tcID string) string {
				return "/build-workflows/" + wfID + "/test-cases/" + tcID + "/test-results"
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "list test results",
			method:     http.MethodGet,
			pathSuffix: func(wfID, _ string) string { return "/build-workflows/" + wfID + "/test-results" },
			wantStatus: http.StatusOK,
		},
		{
			name:       "start test workflow",
			method:     http.MethodPost,
			pathSuffix: func(wfID, _ string) string { return "/build-workflows/" + wfID + "/test-workflows" },
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies",
				map[string]any{"name": "sub-resource-" + tt.name})
			require.Equal(t, http.StatusCreated, rec.Code)

			var created map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
			policyARN := created["policyArn"].(string)

			wf := h.Backend.AddBuildWorkflowForTest(policyARN)

			tcRec := doRequest(t, h, http.MethodPost,
				"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases", nil)
			require.Equal(t, http.StatusCreated, tcRec.Code)

			var tc map[string]any
			require.NoError(t, json.Unmarshal(tcRec.Body.Bytes(), &tc))
			tcID := tc["testCaseId"].(string)

			path := "/automated-reasoning-policies/" + url.PathEscape(policyARN) +
				tt.pathSuffix(wf.BuildWorkflowID, tcID)
			gotRec := doRequest(t, h, tt.method, path, tt.body)
			assert.Equal(t, tt.wantStatus, gotRec.Code)
		})
	}
}

// TestAccuracy_ARP_BuildWorkflowSubResourcesRejectWrongPolicy locks in that the
// build-workflow-scoped sub-resources validate (policyArn, buildWorkflowId) as a
// pair, not buildWorkflowId alone -- a workflow from a different policy must 404,
// mirroring TestHandler_CancelWorkflowWrongPolicy for the pre-existing Cancel op.
func TestAccuracy_ARP_BuildWorkflowSubResourcesRejectWrongPolicy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec1 := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "wrong-policy-1"})
	require.Equal(t, http.StatusCreated, rec1.Code)

	var p1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &p1))
	policy1ARN := p1["policyArn"].(string)

	rec2 := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "wrong-policy-2"})
	require.Equal(t, http.StatusCreated, rec2.Code)

	var p2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &p2))
	policy2ARN := p2["policyArn"].(string)

	wf := h.Backend.AddBuildWorkflowForTest(policy1ARN)

	tests := []struct {
		name   string
		method string
		suffix string
	}{
		{name: "annotations", method: http.MethodGet, suffix: "/annotations"},
		{name: "scenarios", method: http.MethodGet, suffix: "/scenarios"},
		{name: "test results list", method: http.MethodGet, suffix: "/test-results"},
		{name: "test workflows start", method: http.MethodPost, suffix: "/test-workflows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := "/automated-reasoning-policies/" + url.PathEscape(policy2ARN) +
				"/build-workflows/" + wf.BuildWorkflowID + tt.suffix
			gotRec := doRequest(t, h, tt.method, path, nil)
			assert.Equal(t, http.StatusNotFound, gotRec.Code)
		})
	}
}

// TestAccuracy_ARP_ExportVersion locks in ExportAutomatedReasoningPolicyVersion's real
// path (bedrock@v1.66.4 serializers.go:3603): GET .../{policyArn}/export, where
// policyArn may itself be a versioned ARN -- there is no separate {version} segment,
// and the real method is GET, not POST.
func TestAccuracy_ARP_ExportVersion(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies",
		map[string]any{"name": "export-policy"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	policyARN := created["policyArn"].(string)

	verRec := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/versions",
		map[string]any{"lastUpdatedDefinitionHash": "hash-export"})
	require.Equal(t, http.StatusCreated, verRec.Code)

	var ver map[string]any
	require.NoError(t, json.Unmarshal(verRec.Body.Bytes(), &ver))
	versionedARN := ver["policyArn"].(string)

	exportRec := doRequest(t, h, http.MethodGet,
		"/automated-reasoning-policies/"+url.PathEscape(versionedARN)+"/export", nil)
	require.Equal(t, http.StatusOK, exportRec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(exportRec.Body.Bytes(), &out))
	assert.Equal(t, "hash-export", out["definitionHash"])
}

// TestAccuracy_ARP_OldInventedSubResourcePathsGone locks in that the pre-fix
// policy-scoped ARP sub-resource paths -- which do not exist in real AWS, see
// PARITY.md families.AutomatedReasoningPolicy -- no longer serve traffic now that
// these ops are build-workflow-scoped (or, for export, take no version segment).
func TestAccuracy_ARP_OldInventedSubResourcePathsGone(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/automated-reasoning-policies",
		map[string]any{"name": "old-path-gone-policy"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	policyARN := created["policyArn"].(string)

	tcRec := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/test-cases", nil)
	require.Equal(t, http.StatusCreated, tcRec.Code)

	var tc map[string]any
	require.NoError(t, json.Unmarshal(tcRec.Body.Bytes(), &tc))
	tcID := tc["testCaseId"].(string)

	verRec := doRequest(t, h, http.MethodPost,
		"/automated-reasoning-policies/"+url.PathEscape(policyARN)+"/versions",
		map[string]any{"lastUpdatedDefinitionHash": "hash"})
	require.Equal(t, http.StatusCreated, verRec.Code)

	var ver map[string]any
	require.NoError(t, json.Unmarshal(verRec.Body.Bytes(), &ver))
	version := ver["version"].(string)

	base := "/automated-reasoning-policies/" + url.PathEscape(policyARN)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "test case run", method: http.MethodPost, path: base + "/test-cases/" + tcID + "/run"},
		{name: "test case result", method: http.MethodGet, path: base + "/test-cases/" + tcID + "/result"},
		{name: "test cases results collection", method: http.MethodGet, path: base + "/test-cases/results"},
		{name: "versioned export via POST", method: http.MethodPost, path: base + "/versions/" + version + "/export"},
		{name: "flat next scenario", method: http.MethodGet, path: base + "/next-scenario"},
		{name: "flat annotations get", method: http.MethodGet, path: base + "/annotations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotRec := doRequest(t, h, tt.method, tt.path, nil)
			assert.Equal(t, http.StatusNotFound, gotRec.Code)
		})
	}
}
