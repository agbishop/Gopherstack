package dlm_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dlm"
)

// createPolicyWithDetails is like createPolicy (handler_test.go) but lets
// the caller supply PolicyDetails, needed to exercise the
// resourceTypes/targetTags/tagsToAdd GetLifecyclePolicies filters.
func createPolicyWithDetails(t *testing.T, h *dlm.Handler, description string, details map[string]any) string {
	t.Helper()

	rec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
		"Description":      description,
		"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
		"State":            "ENABLED",
		"PolicyDetails":    details,
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	policyID, ok := resp["PolicyId"].(string)
	require.True(t, ok, "PolicyId missing from response")

	return policyID
}

func getPoliciesResp(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	raw, ok := resp["Policies"].([]any)
	require.True(t, ok)

	out := make([]map[string]any, len(raw))
	for i, p := range raw {
		out[i] = p.(map[string]any)
	}

	return out
}

// TestHandler_GetLifecyclePolicies_MultiValuePolicyIDs verifies a real SDK
// client's wire format for the policyIds filter: the same query key repeated
// once per ID (policyIds=a&policyIds=b), never comma-joined. A prior version
// of handleGetLifecyclePolicies used url.Values.Get, which only observes the
// FIRST occurrence of a repeated key, silently dropping every id but the
// first from the filter.
func TestHandler_GetLifecyclePolicies_MultiValuePolicyIDs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	id1 := createPolicy(t, h)
	id2 := createPolicy(t, h)
	_ = createPolicy(t, h) // a third, unfiltered policy

	path := fmt.Sprintf("/policies?policyIds=%s&policyIds=%s", id1, id2)

	rec := doRequest(t, h, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	policies := getPoliciesResp(t, rec)
	require.Len(t, policies, 2, "both repeated policyIds query values must be honored")

	gotIDs := map[string]bool{}
	for _, p := range policies {
		gotIDs[p["PolicyId"].(string)] = true
	}

	assert.True(t, gotIDs[id1])
	assert.True(t, gotIDs[id2])
}

// TestHandler_GetLifecyclePolicies_PolicyTypeInResponse verifies the
// LifecyclePolicySummary wire shape includes PolicyType (previously absent
// from this handler's summary JSON).
func TestHandler_GetLifecyclePolicies_PolicyTypeInResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createPolicyWithDetails(t, h, "img", map[string]any{"PolicyType": "IMAGE_MANAGEMENT"})

	rec := doRequest(t, h, http.MethodGet, "/policies", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	policies := getPoliciesResp(t, rec)
	require.Len(t, policies, 1)
	assert.Equal(t, "IMAGE_MANAGEMENT", policies[0]["PolicyType"])
}

// TestHandler_GetLifecyclePolicies_ResourceTypesQueryFilter drives the
// resourceTypes filter through the full HTTP path (query string -> handler
// -> backend), matching the real client's repeated-query-key wire format.
func TestHandler_GetLifecyclePolicies_ResourceTypesQueryFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createPolicyWithDetails(t, h, "volume", map[string]any{"ResourceTypes": []any{"VOLUME"}})
	createPolicyWithDetails(t, h, "instance", map[string]any{"ResourceTypes": []any{"INSTANCE"}})

	rec := doRequest(t, h, http.MethodGet, "/policies?resourceTypes=INSTANCE", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	policies := getPoliciesResp(t, rec)
	require.Len(t, policies, 1)
	assert.Equal(t, "instance", policies[0]["Description"])
}

// TestHandler_GetLifecyclePolicies_DefaultPolicyTypeQueryFilter drives the
// defaultPolicyType filter through the full HTTP path. Before this fix,
// handleGetLifecyclePolicies never read the "defaultPolicyType" query
// parameter, so it was silently dropped and every policy matched regardless
// of the filter.
func TestHandler_GetLifecyclePolicies_DefaultPolicyTypeQueryFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createPolicyWithDetails(t, h, "custom", map[string]any{"ResourceTypes": []any{"VOLUME"}})

	volRec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
		"Description":      "volume-default",
		"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
		"State":            "ENABLED",
		"DefaultPolicy":    "VOLUME",
	})
	require.Equal(t, http.StatusCreated, volRec.Code)

	instRec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
		"Description":      "instance-default",
		"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
		"State":            "ENABLED",
		"DefaultPolicy":    "INSTANCE",
	})
	require.Equal(t, http.StatusCreated, instRec.Code)

	rec := doRequest(t, h, http.MethodGet, "/policies?defaultPolicyType=VOLUME", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	policies := getPoliciesResp(t, rec)
	require.Len(t, policies, 1)
	assert.Equal(t, "volume-default", policies[0]["Description"])
}

// TestHandler_GetLifecyclePolicies_TargetTagsQueryFilter drives the
// targetTags filter (format key=value) through the full HTTP path.
func TestHandler_GetLifecyclePolicies_TargetTagsQueryFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createPolicyWithDetails(t, h, "prod", map[string]any{
		"TargetTags": []any{map[string]any{"Key": "env", "Value": "prod"}},
	})
	createPolicyWithDetails(t, h, "dev", map[string]any{
		"TargetTags": []any{map[string]any{"Key": "env", "Value": "dev"}},
	})

	rec := doRequest(t, h, http.MethodGet, "/policies?targetTags=env%3Dprod", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	policies := getPoliciesResp(t, rec)
	require.Len(t, policies, 1)
	assert.Equal(t, "prod", policies[0]["Description"])
}

// TestHandler_GetLifecyclePolicies_MatcherRouted proves the RouteMatcher and
// the query-parameter parsing work together end-to-end: a request is first
// accepted by RouteMatcher (the way the real service router would decide
// whether DLM should handle it), then dispatched through Handler(), so the
// policyIds fix above is proven under the same routing path a real request
// takes rather than only via a direct Handler() call.
func TestHandler_GetLifecyclePolicies_MatcherRouted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id1 := createPolicy(t, h)
	id2 := createPolicy(t, h)

	path := fmt.Sprintf("/policies?policyIds=%s&policyIds=%s", id1, id2)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	require.True(t, h.RouteMatcher()(c), "RouteMatcher must accept GET /policies?policyIds=...")
	require.Equal(t, "GetLifecyclePolicies", h.ExtractOperation(c))

	require.NoError(t, h.Handler()(c))
	require.Equal(t, http.StatusOK, rec.Code)

	policies := getPoliciesResp(t, rec)
	assert.Len(t, policies, 2)
}

// TestHandler_GetLifecyclePolicies_DeterministicOrder verifies repeat calls
// against the same backend state return an identical, ascending-by-PolicyID
// ordered response.
func TestHandler_GetLifecyclePolicies_DeterministicOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
	}{
		{name: "five_policies", count: 5},
		{name: "three_policies", count: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for range tc.count {
				createPolicy(t, h)
			}

			firstRec := doRequest(t, h, http.MethodGet, "/policies", nil)
			require.Equal(t, http.StatusOK, firstRec.Code)

			secondRec := doRequest(t, h, http.MethodGet, "/policies", nil)
			require.Equal(t, http.StatusOK, secondRec.Code)

			assert.JSONEq(t, firstRec.Body.String(), secondRec.Body.String())

			var resp map[string]any
			require.NoError(t, json.Unmarshal(firstRec.Body.Bytes(), &resp))
			policies, ok := resp["Policies"].([]any)
			require.True(t, ok)
			require.Len(t, policies, tc.count)

			// Verify ascending PolicyID order.
			ids := make([]string, len(policies))
			for i, p := range policies {
				pm := p.(map[string]any)
				ids[i] = pm["PolicyId"].(string)
			}

			for i := 1; i < len(ids); i++ {
				assert.LessOrEqual(t, ids[i-1], ids[i], "policies not sorted ascending by PolicyID")
			}
		})
	}
}

// TestHandler_UpdateLifecyclePolicy_PolicyDetails verifies PATCH updates
// PolicyDetails and the change is visible via a subsequent GET.
func TestHandler_UpdateLifecyclePolicy_PolicyDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		createDetails map[string]any
		updateDetails map[string]any
		wantType      string
	}{
		{
			name:          "update_policy_type",
			createDetails: map[string]any{"PolicyType": "EBS_SNAPSHOT_MANAGEMENT"},
			updateDetails: map[string]any{"PolicyType": "IMAGE_MANAGEMENT"},
			wantType:      "IMAGE_MANAGEMENT",
		},
		{
			name:          "update_with_schedules",
			createDetails: map[string]any{"PolicyType": "EBS_SNAPSHOT_MANAGEMENT"},
			updateDetails: map[string]any{
				"PolicyType": "EBS_SNAPSHOT_MANAGEMENT",
				"Schedules":  []any{map[string]any{"Name": "daily"}},
			},
			wantType: "EBS_SNAPSHOT_MANAGEMENT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create with initial PolicyDetails.
			createRec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
				"Description":      "test",
				"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
				"State":            "ENABLED",
				"PolicyDetails":    tc.createDetails,
			})
			require.Equal(t, http.StatusCreated, createRec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			policyID := createResp["PolicyId"].(string)

			// Update with new PolicyDetails.
			patchRec := doRequest(t, h, http.MethodPatch, fmt.Sprintf("/policies/%s", policyID), map[string]any{
				"PolicyDetails": tc.updateDetails,
			})
			assert.Equal(t, http.StatusOK, patchRec.Code)

			// GET and verify PolicyDetails updated.
			getRec := doRequest(t, h, http.MethodGet, fmt.Sprintf("/policies/%s", policyID), nil)
			require.Equal(t, http.StatusOK, getRec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
			policy := getResp["Policy"].(map[string]any)
			details := policy["PolicyDetails"].(map[string]any)
			assert.Equal(t, tc.wantType, details["PolicyType"])
		})
	}
}

// TestHandler_UpdateLifecyclePolicy_ExplicitEmptyVsOmitted verifies PATCH
// distinguishes an explicit empty-string Description/ExecutionRoleArn
// (clears the field) from an omitted key (leaves it unchanged) -- the real
// UpdateLifecyclePolicyInput carries both as *string, so the two are
// distinct request bodies on the wire, not synonyms.
func TestHandler_UpdateLifecyclePolicy_ExplicitEmptyVsOmitted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		patchBody   map[string]any
		name        string
		wantDesc    string
		wantRoleArn string
	}{
		{
			name:        "omitted Description/ExecutionRoleArn leaves both unchanged",
			patchBody:   map[string]any{"State": "DISABLED"},
			wantDesc:    "original",
			wantRoleArn: "arn:aws:iam::000000000000:role/original",
		},
		{
			name:        "explicit empty Description clears it",
			patchBody:   map[string]any{"Description": ""},
			wantDesc:    "",
			wantRoleArn: "arn:aws:iam::000000000000:role/original",
		},
		{
			name:        "explicit empty ExecutionRoleArn clears it",
			patchBody:   map[string]any{"ExecutionRoleArn": ""},
			wantDesc:    "original",
			wantRoleArn: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createRec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
				"Description":      "original",
				"ExecutionRoleArn": "arn:aws:iam::000000000000:role/original",
				"State":            "ENABLED",
			})
			require.Equal(t, http.StatusCreated, createRec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			policyID := createResp["PolicyId"].(string)

			patchRec := doRequest(t, h, http.MethodPatch, fmt.Sprintf("/policies/%s", policyID), tc.patchBody)
			require.Equal(t, http.StatusOK, patchRec.Code)

			getRec := doRequest(t, h, http.MethodGet, fmt.Sprintf("/policies/%s", policyID), nil)
			require.Equal(t, http.StatusOK, getRec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
			policy := getResp["Policy"].(map[string]any)
			assert.Equal(t, tc.wantDesc, policy["Description"])
			assert.Equal(t, tc.wantRoleArn, policy["ExecutionRoleArn"])
		})
	}
}

// ---------------------------------------------------------------------------
// CreateLifecyclePolicy / UpdateLifecyclePolicy: top-level default-policy
// fields (CopyTags, CreateInterval, RetainInterval, ExtendDeletion,
// CrossRegionCopyTargets, Exclusions, DefaultPolicy)
// ---------------------------------------------------------------------------

// TestHandler_CreateLifecyclePolicy_DefaultPolicyFields verifies the
// top-level [Default policies only] request members documented on
// CreateLifecyclePolicyInput (aws-sdk-go-v2/service/dlm@v1.39.4/
// api_op_CreateLifecyclePolicy.go:65-138) are accepted and survive a
// Create->Get round trip nested inside PolicyDetails under the same field
// names the real types.PolicyDetails documents for them (types/types.go:
// 512-648), matching real AWS's behavior of folding these convenience
// fields into the stored PolicyDetails document.
func TestHandler_CreateLifecyclePolicy_DefaultPolicyFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody map[string]any
		wantDetail map[string]any
		name       string
	}{
		{
			name: "default policy volume maps to resourcetype and policylanguage",
			createBody: map[string]any{
				"DefaultPolicy": "VOLUME",
			},
			wantDetail: map[string]any{
				"ResourceType":   "VOLUME",
				"PolicyLanguage": "SIMPLIFIED",
			},
		},
		{
			name: "copytags createinterval retaininterval extenddeletion",
			createBody: map[string]any{
				"CopyTags":       true,
				"CreateInterval": float64(3),
				"RetainInterval": float64(10),
				"ExtendDeletion": true,
			},
			wantDetail: map[string]any{
				"CopyTags":       true,
				"CreateInterval": float64(3),
				"RetainInterval": float64(10),
				"ExtendDeletion": true,
			},
		},
		{
			name: "crossregioncopytargets and exclusions",
			createBody: map[string]any{
				"CrossRegionCopyTargets": []any{
					map[string]any{"TargetRegion": "us-west-2"},
				},
				"Exclusions": map[string]any{"ExcludeBootVolumes": true},
			},
			wantDetail: map[string]any{
				"CrossRegionCopyTargets": []any{
					map[string]any{"TargetRegion": "us-west-2"},
				},
				"Exclusions": map[string]any{"ExcludeBootVolumes": true},
			},
		},
		{
			name: "top-level field overwrites the same key sent in PolicyDetails",
			createBody: map[string]any{
				"CopyTags":      true,
				"PolicyDetails": map[string]any{"CopyTags": false},
			},
			wantDetail: map[string]any{
				"CopyTags": true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := map[string]any{
				"Description":      "default policy test",
				"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
				"State":            "ENABLED",
			}
			maps.Copy(body, tc.createBody)

			createRec := doRequest(t, h, http.MethodPost, "/policies", body)
			require.Equal(t, http.StatusCreated, createRec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			policyID := createResp["PolicyId"].(string)

			getRec := doRequest(t, h, http.MethodGet, fmt.Sprintf("/policies/%s", policyID), nil)
			require.Equal(t, http.StatusOK, getRec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
			policy := getResp["Policy"].(map[string]any)
			details := policy["PolicyDetails"].(map[string]any)

			for k, want := range tc.wantDetail {
				assert.Equal(t, want, details[k], "PolicyDetails[%q]", k)
			}
		})
	}
}

// TestHandler_UpdateLifecyclePolicy_DefaultPolicyFields verifies the
// top-level [Default policies only] request members documented on
// UpdateLifecyclePolicyInput (aws-sdk-go-v2/service/dlm@v1.39.4/
// api_op_UpdateLifecyclePolicy.go:39-97, notably no DefaultPolicy member --
// the policy type cannot be changed after creation) are merged into the
// existing stored PolicyDetails when the PATCH body omits PolicyDetails
// entirely, preserving unrelated nested fields like Schedules.
func TestHandler_UpdateLifecyclePolicy_DefaultPolicyFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
		"Description":      "update default policy fields",
		"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
		"State":            "ENABLED",
		"PolicyDetails": map[string]any{
			"PolicyType": "EBS_SNAPSHOT_MANAGEMENT",
			"Schedules":  []any{map[string]any{"Name": "daily"}},
			"CopyTags":   true,
		},
	})
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	policyID := createResp["PolicyId"].(string)

	patchRec := doRequest(t, h, http.MethodPatch, fmt.Sprintf("/policies/%s", policyID), map[string]any{
		"CopyTags":       false,
		"RetainInterval": float64(14),
	})
	require.Equal(t, http.StatusOK, patchRec.Code)

	getRec := doRequest(t, h, http.MethodGet, fmt.Sprintf("/policies/%s", policyID), nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	policy := getResp["Policy"].(map[string]any)
	details := policy["PolicyDetails"].(map[string]any)

	assert.Equal(t, "EBS_SNAPSHOT_MANAGEMENT", details["PolicyType"], "unrelated PolicyDetails fields must survive")
	assert.NotEmpty(t, details["Schedules"], "Schedules must survive an update that only touches top-level fields")
	assert.Equal(t, false, details["CopyTags"], "top-level CopyTags must overwrite the previously stored value")
	assert.InDelta(t, float64(14), details["RetainInterval"], 0)
}

// ---------------------------------------------------------------------------
// GetLifecyclePolicy / GetLifecyclePolicies: top-level DefaultPolicy bool
// ---------------------------------------------------------------------------

// TestHandler_GetLifecyclePolicy_DefaultPolicyEcho verifies the top-level
// DefaultPolicy bool (aws-sdk-go-v2/service/dlm@v1.39.4/types/types.go:411)
// on the GetLifecyclePolicy response, derived from the stored
// PolicyLanguage that a default-policy Create request sets to SIMPLIFIED
// (see defaultPolicyFields.applyTo in models.go). A custom (STANDARD)
// policy must echo false.
func TestHandler_GetLifecyclePolicy_DefaultPolicyEcho(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody map[string]any
		name       string
		want       bool
	}{
		{
			name:       "default policy echoes DefaultPolicy true",
			createBody: map[string]any{"DefaultPolicy": "VOLUME"},
			want:       true,
		},
		{
			name:       "custom policy echoes DefaultPolicy false",
			createBody: map[string]any{"PolicyDetails": map[string]any{"PolicyType": "EBS_SNAPSHOT_MANAGEMENT"}},
			want:       false,
		},
		{
			name:       "no PolicyDetails at all echoes DefaultPolicy false",
			createBody: map[string]any{},
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := map[string]any{
				"Description":      "default policy echo test",
				"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
				"State":            "ENABLED",
			}
			maps.Copy(body, tc.createBody)

			createRec := doRequest(t, h, http.MethodPost, "/policies", body)
			require.Equal(t, http.StatusCreated, createRec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			policyID := createResp["PolicyId"].(string)

			getRec := doRequest(t, h, http.MethodGet, fmt.Sprintf("/policies/%s", policyID), nil)
			require.Equal(t, http.StatusOK, getRec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
			policy := getResp["Policy"].(map[string]any)

			assert.Equal(t, tc.want, policy["DefaultPolicy"])
		})
	}
}

// TestHandler_GetLifecyclePolicies_DefaultPolicyEcho verifies the summary
// shape (LifecyclePolicySummary, types.go:449) also carries the top-level
// DefaultPolicy bool.
func TestHandler_GetLifecyclePolicies_DefaultPolicyEcho(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createPolicyWithDetails(t, h, "custom", map[string]any{"PolicyType": "EBS_SNAPSHOT_MANAGEMENT"})

	defaultRec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
		"Description":      "default",
		"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
		"State":            "ENABLED",
		"DefaultPolicy":    "INSTANCE",
	})
	require.Equal(t, http.StatusCreated, defaultRec.Code)

	rec := doRequest(t, h, http.MethodGet, "/policies", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	policies := getPoliciesResp(t, rec)
	require.Len(t, policies, 2)

	got := map[string]bool{}
	for _, p := range policies {
		got[p["Description"].(string)] = p["DefaultPolicy"].(bool)
	}

	assert.False(t, got["custom"])
	assert.True(t, got["default"])
}

// ---------------------------------------------------------------------------
// CreateLifecyclePolicy: LimitExceededException quota
// ---------------------------------------------------------------------------

// TestHandler_CreateLifecyclePolicy_LimitExceeded covers gopherstack-x009's
// quota gap: AWS's documented default "Policies per Region" quota is 100
// (adjustable; quota code L-5407D8DA,
// docs.aws.amazon.com/general/latest/gr/dlm.html), and
// CreateLifecyclePolicy's modeled error catalog includes
// LimitExceededException (aws-sdk-go-v2/service/dlm@v1.39.4/types/errors.go:70).
func TestHandler_CreateLifecyclePolicy_LimitExceeded(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	const maxPoliciesPerRegion = 100

	for i := range maxPoliciesPerRegion {
		rec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
			"Description":      fmt.Sprintf("policy-%d", i),
			"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
			"State":            "ENABLED",
		})
		require.Equal(t, http.StatusCreated, rec.Code, "policy %d should succeed under the quota", i)
	}

	rec := doRequest(t, h, http.MethodPost, "/policies", map[string]any{
		"Description":      "one-too-many",
		"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
		"State":            "ENABLED",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var out map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "LimitExceededException", out["__type"])
}

// ---------------------------------------------------------------------------
// CreateLifecyclePolicy: required-field validation over HTTP
// ---------------------------------------------------------------------------

// TestHandler_CreateLifecyclePolicy_MissingRequiredFields verifies that
// omitting ExecutionRoleArn or Description -- both required members of the
// real CreateLifecyclePolicyInput -- surfaces as a 400 InvalidRequestException
// rather than silently fabricating a policy the real API could never produce.
func TestHandler_CreateLifecyclePolicy_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "missing Description",
			body: map[string]any{
				"ExecutionRoleArn": "arn:aws:iam::000000000000:role/dlm-role",
				"State":            "ENABLED",
			},
		},
		{
			name: "missing ExecutionRoleArn",
			body: map[string]any{
				"Description": "a policy",
				"State":       "ENABLED",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/policies", tc.body)

			require.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "InvalidRequestException", resp["__type"])
		})
	}
}

// ---------------------------------------------------------------------------
// handleCreateLifecyclePolicy / handleUpdateLifecyclePolicy: invalid body
// ---------------------------------------------------------------------------

func TestHandler_CreateLifecyclePolicy_InvalidBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     []byte
		wantCode int
	}{
		{
			name:     "malformed JSON returns 400",
			body:     []byte(`{not valid json`),
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e := echo.New()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestHandler_UpdateLifecyclePolicy_InvalidBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     []byte
		wantCode int
	}{
		{
			name:     "malformed JSON on PATCH returns 400",
			body:     []byte(`not json`),
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			policyID := createPolicy(t, h)
			path := fmt.Sprintf("/policies/%s", policyID)

			req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e := echo.New()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

// ---------------------------------------------------------------------------
// handleGetLifecyclePolicies: filter by policyIds and state
// ---------------------------------------------------------------------------

func TestHandler_GetLifecyclePolicies_Filters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		setupCount int
		wantMinLen int
		wantMaxLen int
	}{
		{
			name:       "filter by policyIds returns matching policies",
			setupCount: 2,
			// will be set dynamically — tested via check in subtests below
		},
		{
			name:       "filter by state=ENABLED returns only enabled",
			query:      "state=ENABLED",
			setupCount: 1,
			wantMinLen: 1,
			wantMaxLen: 1,
		},
		{
			name:       "filter by state=DISABLED returns empty when all ENABLED",
			query:      "state=DISABLED",
			setupCount: 1,
			wantMinLen: 0,
			wantMaxLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			var ids []string
			for range tc.setupCount {
				ids = append(ids, createPolicy(t, h))
			}

			query := tc.query
			if query == "" && len(ids) > 0 {
				// Filter by first ID only.
				query = fmt.Sprintf("policyIds=%s", ids[0])
			}

			path := "/policies"
			if query != "" {
				path = path + "?" + query
			}

			rec := doRequest(t, h, http.MethodGet, path, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			policies := resp["Policies"].([]any)

			if tc.wantMaxLen > 0 || tc.wantMinLen > 0 {
				assert.GreaterOrEqual(t, len(policies), tc.wantMinLen)
				assert.LessOrEqual(t, len(policies), tc.wantMaxLen)
			} else if tc.query == "" {
				// policyIds filter: exactly 1 result.
				assert.Len(t, policies, 1)
			}
		})
	}
}
