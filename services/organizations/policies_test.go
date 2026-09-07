package organizations_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestBackend_PolicyLifecycle tests policy CRUD and attachment.
func TestBackend_PolicyLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyName string
		policyType string
		content    string
	}{
		{
			name:       "scp_lifecycle",
			policyName: "deny-all",
			policyType: "SERVICE_CONTROL_POLICY",
			content:    `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			_, _, err := b.CreateOrganization("ALL")
			require.NoError(t, err)

			// CreatePolicy.
			policy, err := b.CreatePolicy(tt.policyName, "test policy", tt.content, tt.policyType, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, policy.PolicySummary.ID)
			assert.Equal(t, tt.policyName, policy.PolicySummary.Name)

			// DescribePolicy.
			desc, err := b.DescribePolicy(policy.PolicySummary.ID)
			require.NoError(t, err)
			assert.Equal(t, policy.PolicySummary.ID, desc.PolicySummary.ID)

			// ListPolicies.
			policies, err := b.ListPolicies(tt.policyType)
			require.NoError(t, err)
			assert.NotEmpty(t, policies)

			// DeletePolicy.
			err = b.DeletePolicy(policy.PolicySummary.ID)
			require.NoError(t, err)

			// After deletion, describe should fail.
			_, err = b.DescribePolicy(policy.PolicySummary.ID)
			require.Error(t, err)
		})
	}
}

// TestHandler_EnablePolicyType tests the HTTP handler for EnablePolicyType.
func TestHandler_EnablePolicyType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "enables_policy_type",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

			rootsRec := doRequest(t, h, "ListRoots", map[string]any{})
			var rootsResp map[string]any
			require.NoError(t, json.NewDecoder(rootsRec.Body).Decode(&rootsResp))
			roots := rootsResp["Roots"].([]any)
			rootID := roots[0].(map[string]any)["Id"].(string)

			// TAG_POLICY, not SERVICE_CONTROL_POLICY: SCP is auto-enabled by
			// CreateOrganization under FeatureSet=ALL (gopherstack-inmm), so
			// enabling it here would already return AlreadyEnabled.
			rec := doRequest(t, h, "EnablePolicyType", map[string]any{
				"RootId":     rootID,
				"PolicyType": "TAG_POLICY",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestBackend_UpdatePolicy tests updating a policy.
func TestBackend_UpdatePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name: "updates_policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			policy, err := b.CreatePolicy("original", "desc", `{}`, "SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			updated, err := b.UpdatePolicy(
				policy.PolicySummary.ID,
				"updated-name",
				"updated-desc",
				`{"Version":"2012-10-17"}`,
			)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "updated-name", updated.PolicySummary.Name)
		})
	}
}

// TestBackend_DisablePolicyType tests disabling a policy type.
func TestBackend_DisablePolicyType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name: "enable_then_disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// TAG_POLICY, not SERVICE_CONTROL_POLICY: SCP is auto-enabled by
			// CreateOrganization under FeatureSet=ALL (gopherstack-inmm), so
			// TAG_POLICY is what starts genuinely disabled here.
			_, err := b.EnablePolicyType(rootID, "TAG_POLICY")
			require.NoError(t, err)

			// Now disable.
			_, err = b.DisablePolicyType(rootID, "TAG_POLICY")

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestPolicyTypes tests all six supported policy types for CRUD + enable/disable.
func TestPolicyTypes(t *testing.T) {
	t.Parallel()

	policyTypes := []string{
		"SERVICE_CONTROL_POLICY",
		"TAG_POLICY",
		"BACKUP_POLICY",
		"AISERVICES_OPT_OUT_POLICY",
		"CHATBOT_POLICY",
		"DECLARATIVE_POLICY_EC2",
	}

	for _, pt := range policyTypes {
		t.Run(pt, func(t *testing.T) {
			t.Parallel()

			// CONSOLIDATED_BILLING seeds the same default FullAWSAccess SCP as
			// ALL but leaves every policy type disabled on root (SDK doc:
			// "no policy types are enabled by default"), so EnablePolicyType
			// below always starts from a genuinely disabled state -- SCP
			// included, since ALL auto-enables it (gopherstack-inmm).
			b := newTestBackend()

			_, orgRoot, err := b.CreateOrganization("CONSOLIDATED_BILLING")
			require.NoError(t, err)

			rootID := orgRoot.ID

			p, err := b.CreatePolicy("test-"+pt, "desc", `{"Version":"2012-10-17"}`, pt, nil)
			require.NoError(t, err, "CreatePolicy")
			assert.Equal(t, pt, p.PolicySummary.Type)

			// SCP starts with 1 extra entry: the default FullAWSAccess policy
			// every organization is created with.
			wantListed := 1
			if pt == "SERVICE_CONTROL_POLICY" {
				wantListed = 2
			}

			listed, err := b.ListPolicies(pt)
			require.NoError(t, err, "ListPolicies")
			assert.Len(t, listed, wantListed)

			_, err = b.EnablePolicyType(rootID, pt)
			require.NoError(t, err, "EnablePolicyType")

			root, err := b.DisablePolicyType(rootID, pt)
			require.NoError(t, err, "DisablePolicyType")
			assert.Empty(t, root.PolicyTypes)

			err = b.DeletePolicy(p.PolicySummary.ID)
			require.NoError(t, err, "DeletePolicy")
		})
	}
}

// TestPolicyTypes_InvalidType tests that invalid policy types are rejected.
func TestPolicyTypes_InvalidType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyType string
	}{
		{name: "empty", policyType: ""},
		{name: "wrong_type", policyType: "NOT_A_REAL_TYPE"},
		{name: "lowercase", policyType: "service_control_policy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			_, err := b.CreatePolicy("p", "d", `{}`, tt.policyType, nil)
			require.Error(t, err)
		})
	}
}

// TestPolicyTypes_Handler tests the HTTP handler accepts all six policy types.
func TestPolicyTypes_Handler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyType string
		wantStatus int
	}{
		{name: "SCP", policyType: "SERVICE_CONTROL_POLICY", wantStatus: http.StatusOK},
		{name: "TAG", policyType: "TAG_POLICY", wantStatus: http.StatusOK},
		{name: "BACKUP", policyType: "BACKUP_POLICY", wantStatus: http.StatusOK},
		{name: "AISERVICES", policyType: "AISERVICES_OPT_OUT_POLICY", wantStatus: http.StatusOK},
		{name: "CHATBOT", policyType: "CHATBOT_POLICY", wantStatus: http.StatusOK},
		{name: "DECLARATIVE_EC2", policyType: "DECLARATIVE_POLICY_EC2", wantStatus: http.StatusOK},
		{name: "invalid", policyType: "INVALID_TYPE", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			rec := doRequest(t, h, "CreatePolicy", map[string]any{
				"Name":        "test-policy",
				"Description": "desc",
				"Content":     `{"Version":"2012-10-17"}`,
				"Type":        tt.policyType,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_PolicyErrors tests policy handler error paths.
func TestHandler_PolicyErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		op         string
		wantStatus int
	}{
		{
			name:       "create_policy_missing_name",
			op:         "CreatePolicy",
			body:       map[string]any{"Type": "SERVICE_CONTROL_POLICY", "Content": "{}"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "describe_policy_not_found",
			op:         "DescribePolicy",
			body:       map[string]any{"PolicyId": "p-notexist"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "delete_policy_not_found",
			op:         "DeletePolicy",
			body:       map[string]any{"PolicyId": "p-notexist"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "attach_policy_not_found",
			op:         "AttachPolicy",
			body:       map[string]any{"PolicyId": "p-notexist", "TargetId": "r-rootid"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "enable_policy_type_invalid_root",
			op:         "EnablePolicyType",
			body:       map[string]any{"RootId": "r-notexist", "PolicyType": "SERVICE_CONTROL_POLICY"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			rec := doRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestValidPolicyTypes_AllEight ensures all 8 policy types can be used in CreatePolicy.
func TestValidPolicyTypes_AllEight(t *testing.T) {
	t.Parallel()

	allTypes := []string{
		"SERVICE_CONTROL_POLICY",
		"RESOURCE_CONTROL_POLICY",
		"TAG_POLICY",
		"BACKUP_POLICY",
		"AISERVICES_OPT_OUT_POLICY",
		"CHATBOT_POLICY",
		"DECLARATIVE_POLICY_EC2",
		"SECURITYHUB_POLICY",
	}

	for _, pt := range allTypes {
		t.Run(pt, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			p, err := b.CreatePolicy("test", "desc", `{"Version":"2012-10-17"}`, pt, nil)
			require.NoError(t, err, "CreatePolicy should succeed for %s", pt)
			assert.Equal(t, pt, p.PolicySummary.Type)
		})
	}
}

// TestNewPolicyTypes_ResourceControl_SecurityHub specifically tests the two new ones.
func TestNewPolicyTypes_ResourceControl_SecurityHub(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyType string
	}{
		{name: "resource_control_policy", policyType: "RESOURCE_CONTROL_POLICY"},
		{name: "securityhub_policy", policyType: "SECURITYHUB_POLICY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// CreatePolicy.
			p, err := b.CreatePolicy("test", "desc", `{"Version":"2012-10-17"}`, tt.policyType, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.policyType, p.PolicySummary.Type)

			// EnablePolicyType.
			root, err := b.EnablePolicyType(rootID, tt.policyType)
			require.NoError(t, err)

			found := false
			for _, pt := range root.PolicyTypes {
				if pt.Type == tt.policyType && pt.Status == "ENABLED" {
					found = true
				}
			}
			assert.True(t, found, "%s should be ENABLED", tt.policyType)
		})
	}
}

// ---------------------------------------------------------------------------
// Item 3: FeatureSet validation in CreateOrganization
// ---------------------------------------------------------------------------

// TestListPolicies_EmptyFilter verifies that ListPolicies rejects empty Filter.
func TestListPolicies_EmptyFilter(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	_, err := b.ListPolicies("")
	require.Error(t, err, "empty Filter must be rejected")

	// Valid filter works fine (empty result is OK, nil error is required).
	_, err = b.ListPolicies("SERVICE_CONTROL_POLICY")
	require.NoError(t, err)
}

// TestListPolicies_EmptyFilter_ViaHandler verifies HTTP 400 when Filter is empty.
func TestListPolicies_EmptyFilter_ViaHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		op         string
		wantStatus int
	}{
		{
			name:       "ListPolicies_empty_filter",
			op:         "ListPolicies",
			body:       map[string]any{"Filter": ""},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ListPolicies_missing_filter",
			op:         "ListPolicies",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ListPoliciesForTarget_empty_filter",
			op:         "ListPoliciesForTarget",
			body:       map[string]any{"TargetId": "r-test", "Filter": ""},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

			rec := doRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code, tt.name)
		})
	}
}

// ---------------------------------------------------------------------------
// Item 26: Policy IDs use lowercase hex chars only
// ---------------------------------------------------------------------------

var policyIDHexRe = regexp.MustCompile(`^p-[0-9a-f]{8}$`)

// TestPolicyID_HexFormat verifies that generated policy IDs use only
// lowercase hex chars (0-9 a-f) — matching real AWS policy ID format.
func TestPolicyID_HexFormat(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	for i := range 20 {
		p, err := b.CreatePolicy(
			"hex-test-"+string(rune('a'+i%26)),
			"",
			`{"Version":"2012-10-17"}`,
			"SERVICE_CONTROL_POLICY",
			nil,
		)
		require.NoError(t, err)
		assert.Regexp(t, policyIDHexRe, p.PolicySummary.ID,
			"policy ID must match p-[0-9a-f]{8}, got %s", p.PolicySummary.ID)
	}
}

// ---------------------------------------------------------------------------
// Item 8: RemoveAccountFromOrganization cleans policyTargets reverse mapping
// and generates LEAVE_ORGANIZATION handshake for INVITED accounts
// ---------------------------------------------------------------------------

// TestDeletePolicy_Attached_Rejected verifies that deleting a policy
// that is still attached to a target returns PolicyInUseException (HTTP 400).
func TestDeletePolicy_Attached_Rejected(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	p, err := b.CreatePolicy("deny-all", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
	require.NoError(t, err)

	err = b.AttachPolicy(p.PolicySummary.ID, rootID)
	require.NoError(t, err)

	err = b.DeletePolicy(p.PolicySummary.ID)
	require.Error(t, err, "DeletePolicy must fail while policy is attached")
}

// TestDeletePolicy_Attached_ViaHandler verifies HTTP 400 + PolicyInUseException.
func TestDeletePolicy_Attached_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

	roots := doRequest(t, h, "ListRoots", nil)
	var rootsResp map[string]any
	require.NoError(t, json.Unmarshal(roots.Body.Bytes(), &rootsResp))
	rootID := rootsResp["Roots"].([]any)[0].(map[string]any)["Id"].(string)

	doRequest(t, h, "EnablePolicyType", map[string]any{"RootId": rootID, "PolicyType": "SERVICE_CONTROL_POLICY"})

	cp := doRequest(t, h, "CreatePolicy", map[string]any{
		"Name":        "deny-all",
		"Content":     `{"Version":"2012-10-17"}`,
		"Type":        "SERVICE_CONTROL_POLICY",
		"Description": "",
	})
	require.Equal(t, http.StatusOK, cp.Code)

	var cpResp map[string]any
	require.NoError(t, json.Unmarshal(cp.Body.Bytes(), &cpResp))
	policyID := cpResp["Policy"].(map[string]any)["PolicySummary"].(map[string]any)["Id"].(string)

	doRequest(t, h, "AttachPolicy", map[string]any{"PolicyId": policyID, "TargetId": rootID})

	rec := doRequest(t, h, "DeletePolicy", map[string]any{"PolicyId": policyID})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "PolicyInUseException", errResp["__type"])
}

// TestDeletePolicy_Succeeds verifies that DeletePolicy succeeds once a policy
// is no longer attached to any target, whether it was previously attached and
// detached, or never attached at all.
func TestDeletePolicy_Succeeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		attachThenDetach bool
	}{
		{name: "after_detach_ok", attachThenDetach: true},
		{name: "not_attached_ok", attachThenDetach: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			p, err := b.CreatePolicy("deny-all", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			if tt.attachThenDetach {
				require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, rootID))
				require.NoError(t, b.DetachPolicy(p.PolicySummary.ID, rootID))
			}

			require.NoError(t, b.DeletePolicy(p.PolicySummary.ID))
		})
	}
}

// ---------------------------------------------------------------------------
// DeleteOrganization: OrganizationNotEmptyException when member accounts exist
// ---------------------------------------------------------------------------

// TestDisablePolicyType_WithAttached_Rejected verifies that disabling
// a policy type while policies of that type are attached returns ConstraintViolationException.
func TestDisablePolicyType_WithAttached_Rejected(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	p, err := b.CreatePolicy("scp", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
	require.NoError(t, err)

	require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, rootID))

	_, err = b.DisablePolicyType(rootID, "SERVICE_CONTROL_POLICY")
	require.Error(t, err, "DisablePolicyType must fail while policies of that type are attached")
}

// TestEnablePolicyType_UnknownType_Rejected verifies EnablePolicyType rejects
// a PolicyType value outside AWS's enum instead of silently enabling it.
func TestEnablePolicyType_UnknownType_Rejected(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	_, err := b.EnablePolicyType(rootID, "NOT_A_REAL_POLICY_TYPE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidInputException")
}

// TestDisablePolicyType_UnknownType_Rejected verifies DisablePolicyType
// rejects a PolicyType value outside AWS's enum.
func TestDisablePolicyType_UnknownType_Rejected(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	_, err := b.DisablePolicyType(rootID, "NOT_A_REAL_POLICY_TYPE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidInputException")
}

// TestDisablePolicyType_WithAttached_ViaHandler verifies HTTP 400 +
// ConstraintViolationException when policies are still attached.
func TestDisablePolicyType_WithAttached_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

	roots := doRequest(t, h, "ListRoots", nil)
	var rootsResp map[string]any
	require.NoError(t, json.Unmarshal(roots.Body.Bytes(), &rootsResp))
	rootID := rootsResp["Roots"].([]any)[0].(map[string]any)["Id"].(string)

	doRequest(t, h, "EnablePolicyType", map[string]any{"RootId": rootID, "PolicyType": "TAG_POLICY"})

	cp := doRequest(t, h, "CreatePolicy", map[string]any{
		"Name":        "tag-p",
		"Content":     `{"tags":{}}`,
		"Type":        "TAG_POLICY",
		"Description": "",
	})
	require.Equal(t, http.StatusOK, cp.Code)

	var cpResp map[string]any
	require.NoError(t, json.Unmarshal(cp.Body.Bytes(), &cpResp))
	policyID := cpResp["Policy"].(map[string]any)["PolicySummary"].(map[string]any)["Id"].(string)

	doRequest(t, h, "AttachPolicy", map[string]any{"PolicyId": policyID, "TargetId": rootID})

	rec := doRequest(t, h, "DisablePolicyType", map[string]any{"RootId": rootID, "PolicyType": "TAG_POLICY"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ConstraintViolationException", errResp["__type"])
}

// TestDisablePolicyType_AfterDetach_OK verifies that detaching all policies
// of a type before disabling it succeeds.
func TestDisablePolicyType_AfterDetach_OK(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	p, err := b.CreatePolicy("scp", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
	require.NoError(t, err)

	require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, rootID))
	require.NoError(t, b.DetachPolicy(p.PolicySummary.ID, rootID))

	// The default FullAWSAccess SCP is also attached to root; detach it too
	// so disabling isn't rejected by the still-attached guard.
	require.NoError(t, b.DetachPolicy("p-FullAWSAccess", rootID))

	_, err = b.DisablePolicyType(rootID, "SERVICE_CONTROL_POLICY")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// InviteAccountToOrganization: DuplicateHandshakeException for duplicate open invites
// ---------------------------------------------------------------------------

// TestHandler_PolicyOperations tests create, describe, update, delete, list, attach, detach.
func TestHandler_PolicyOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "full_policy_lifecycle",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, rootID := newHandlerWithOrg(t)
			policyID := createPolicyViaHandler(t, h, "test-policy")

			// DescribePolicy.
			rec := doRequest(t, h, "DescribePolicy", map[string]any{"PolicyId": policyID})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// UpdatePolicy.
			rec = doRequest(t, h, "UpdatePolicy", map[string]any{
				"PolicyId":    policyID,
				"Name":        "updated-policy",
				"Description": "updated",
				"Content":     `{"Version":"2012-10-17"}`,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// ListPolicies.
			rec = doRequest(t, h, "ListPolicies", map[string]any{"Filter": "SERVICE_CONTROL_POLICY"})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// AttachPolicy.
			rec = doRequest(t, h, "AttachPolicy", map[string]any{
				"PolicyId": policyID,
				"TargetId": rootID,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// ListPoliciesForTarget.
			rec = doRequest(t, h, "ListPoliciesForTarget", map[string]any{
				"TargetId": rootID,
				"Filter":   "SERVICE_CONTROL_POLICY",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// ListTargetsForPolicy.
			rec = doRequest(t, h, "ListTargetsForPolicy", map[string]any{"PolicyId": policyID})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// DetachPolicy.
			rec = doRequest(t, h, "DetachPolicy", map[string]any{
				"PolicyId": policyID,
				"TargetId": rootID,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// DeletePolicy.
			rec = doRequest(t, h, "DeletePolicy", map[string]any{"PolicyId": policyID})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_DisablePolicyType tests the HTTP handler.
func TestHandler_DisablePolicyType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "disables_policy_type",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, rootID := newHandlerWithOrg(t)

			// Enable first.
			doRequest(t, h, "EnablePolicyType", map[string]any{
				"RootId":     rootID,
				"PolicyType": "SERVICE_CONTROL_POLICY",
			})

			// The default FullAWSAccess SCP is attached to root; detach it so
			// disabling isn't rejected by the still-attached guard.
			detachRec := doRequest(t, h, "DetachPolicy", map[string]any{
				"PolicyId": "p-FullAWSAccess",
				"TargetId": rootID,
			})
			require.Equal(t, http.StatusOK, detachRec.Code)

			rec := doRequest(t, h, "DisablePolicyType", map[string]any{
				"RootId":     rootID,
				"PolicyType": "SERVICE_CONTROL_POLICY",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestListPolicies_Sorted verifies deterministic sorted order by policy name.
func TestListPolicies_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		names   []string
		wantLen int
	}{
		{
			// wantLen includes the default FullAWSAccess SCP every organization
			// is created with.
			name:    "two_policies_sorted",
			names:   []string{"ZPolicy", "APolicy"},
			wantLen: 3,
		},
		{
			name:    "three_policies_sorted",
			names:   []string{"CPolicy", "APolicy", "BPolicy"},
			wantLen: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			for _, name := range tt.names {
				_, err := b.CreatePolicy(name, "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
				require.NoError(t, err)
			}

			policies, err := b.ListPolicies("SERVICE_CONTROL_POLICY")
			require.NoError(t, err)
			require.Len(t, policies, tt.wantLen)

			for i := 1; i < len(policies); i++ {
				assert.LessOrEqual(t, policies[i-1].PolicySummary.Name, policies[i].PolicySummary.Name)
			}
		})
	}
}

// TestCreatePolicy_InvalidType verifies CreatePolicy rejects invalid policy types.
func TestCreatePolicy_InvalidType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyType string
		wantErr    bool
	}{
		{name: "invalid_type", policyType: "INVALID_POLICY_TYPE", wantErr: true},
		{name: "empty_type", policyType: "", wantErr: true},
		{name: "valid_scp", policyType: "SERVICE_CONTROL_POLICY", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			_, err := b.CreatePolicy("p", "", `{"Version":"2012-10-17"}`, tt.policyType, nil)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreatePolicy_ValidTypes verifies all 6 valid policy types are accepted.
func TestCreatePolicy_ValidTypes(t *testing.T) {
	t.Parallel()

	validTypes := []string{
		"SERVICE_CONTROL_POLICY",
		"TAG_POLICY",
		"BACKUP_POLICY",
		"AISERVICES_OPT_OUT_POLICY",
		"CHATBOT_POLICY",
		"DECLARATIVE_POLICY_EC2",
	}

	for _, pt := range validTypes {
		t.Run(pt, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			p, err := b.CreatePolicy("mypol", "", `{"Version":"2012-10-17"}`, pt, nil)
			require.NoError(t, err)
			assert.Equal(t, pt, p.PolicySummary.Type)
		})
	}
}

// TestCreatePolicy_MalformedContent verifies CreatePolicy rejects non-JSON
// content with MalformedPolicyDocumentException, matching real AWS behavior.
func TestCreatePolicy_MalformedContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "not_json_at_all", content: "this is not json"},
		{name: "truncated_json", content: `{"Version":"2012-10-17"`},
		{name: "trailing_garbage", content: `{}garbage`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			_, err := b.CreatePolicy("bad-content", "", tt.content, "SERVICE_CONTROL_POLICY", nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MalformedPolicyDocumentException")
		})
	}
}

// TestCreatePolicy_MalformedContent_ViaHandler verifies the HTTP wire error
// for a non-JSON policy document.
func TestCreatePolicy_MalformedContent_ViaHandler(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithOrg(t)

	rec := doRequest(t, h, "CreatePolicy", map[string]any{
		"Name":        "bad",
		"Description": "",
		"Content":     "not json",
		"Type":        "SERVICE_CONTROL_POLICY",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "MalformedPolicyDocumentException", errResp["__type"])
}

// TestCreatePolicy_ContentSizeLimit verifies CreatePolicy enforces the
// per-policy-type maximum document size with ConstraintViolationException
// (reason POLICY_CONTENT_LIMIT_EXCEEDED), matching real AWS quotas.
func TestCreatePolicy_ContentSizeLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyType string
		limit      int
	}{
		{name: "scp", policyType: "SERVICE_CONTROL_POLICY", limit: 10240},
		{name: "resource_control", policyType: "RESOURCE_CONTROL_POLICY", limit: 5120},
		{name: "tag", policyType: "TAG_POLICY", limit: 10000},
		{name: "backup", policyType: "BACKUP_POLICY", limit: 10000},
		{name: "ai_opt_out", policyType: "AISERVICES_OPT_OUT_POLICY", limit: 2500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			// Build oversized-but-valid JSON: a big string value padded past the limit.
			pad := strings.Repeat("a", tt.limit)
			oversized := `{"k":"` + pad + `"}`

			_, err := b.CreatePolicy("too-big", "", oversized, tt.policyType, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ConstraintViolationException")
			assert.Contains(t, err.Error(), "POLICY_CONTENT_LIMIT_EXCEEDED")

			// A document right at the limit is accepted.
			atLimit := `{"k":"` + strings.Repeat("a", tt.limit-8) + `"}`
			require.Len(t, atLimit, tt.limit)

			_, err = b.CreatePolicy("just-right", "", atLimit, tt.policyType, nil)
			require.NoError(t, err)
		})
	}
}

// TestUpdatePolicy_MalformedContent verifies UpdatePolicy rejects non-JSON
// content and leaves the existing policy untouched (name/description are not
// partially applied either), matching AWS's "the entire request fails" semantics.
func TestUpdatePolicy_MalformedContent(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	p, err := b.CreatePolicy("orig-name", "orig-desc", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
	require.NoError(t, err)

	_, err = b.UpdatePolicy(p.PolicySummary.ID, "new-name", "new-desc", "not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MalformedPolicyDocumentException")

	desc, err := b.DescribePolicy(p.PolicySummary.ID)
	require.NoError(t, err)
	assert.Equal(t, "orig-name", desc.PolicySummary.Name, "name must not be partially applied")
	assert.Equal(t, "orig-desc", desc.PolicySummary.Description, "description must not be partially applied")
	assert.JSONEq(t, `{"Version":"2012-10-17"}`, desc.Content, "content must not be partially applied")
}

// TestUpdatePolicy_ContentSizeLimit verifies UpdatePolicy enforces the same
// per-type size quota as CreatePolicy.
func TestUpdatePolicy_ContentSizeLimit(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	p, err := b.CreatePolicy("scp", "", `{}`, "SERVICE_CONTROL_POLICY", nil)
	require.NoError(t, err)

	oversized := `{"k":"` + strings.Repeat("a", 10240) + `"}`
	_, err = b.UpdatePolicy(p.PolicySummary.ID, "", "", oversized)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "POLICY_CONTENT_LIMIT_EXCEEDED")
}

// TestBackend_AddPolicyInternal verifies policy seed helper.
func TestBackend_AddPolicyInternal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policyID  string
		wantCount int
	}{
		{name: "adds_policy", policyID: "p-12345678", wantCount: 2}, // +1 for the default FullAWSAccess SCP
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			b.AddPolicyInternal(&organizations.Policy{
				PolicySummary: organizations.PolicySummary{
					ID:   tt.policyID,
					Name: "seed-policy",
					Type: "SERVICE_CONTROL_POLICY",
				},
				Content: `{}`,
			})

			assert.Equal(t, tt.wantCount, organizations.PolicyCount(b))
		})
	}
}
