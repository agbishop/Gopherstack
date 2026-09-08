package organizations_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestDescribeEffectivePolicy_AllPolicyTypes tests DescribeEffectivePolicy
// works for all six policy types when a policy is attached to the root.
func TestDescribeEffectivePolicy_AllPolicyTypes(t *testing.T) {
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

			b, rootID := newOrgBackend(t)

			p, err := b.CreatePolicy("p-"+pt, "", `{"Version":"2012-10-17"}`, pt, nil)
			require.NoError(t, err)

			// Attach to root so hierarchy walk finds it from any child.
			err = b.AttachPolicy(p.PolicySummary.ID, rootID)
			require.NoError(t, err)

			// Look up effective policy from root directly.
			ep, err := b.DescribeEffectivePolicy(pt, rootID)
			require.NoError(t, err)
			assert.Equal(t, pt, ep.PolicyType)
			assert.Equal(t, rootID, ep.TargetID)
		})
	}
}

// TestListEffectivePolicyValidationErrors_AllTypes tests all six policy types.
func TestListEffectivePolicyValidationErrors_AllTypes(t *testing.T) {
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

			b, _ := newOrgBackend(t)

			errs, err := b.ListEffectivePolicyValidationErrors(pt, "")
			require.NoError(t, err)
			assert.Empty(t, errs)
		})
	}
}

// TestDescribeEffectivePolicy_FullChain verifies that DescribeEffectivePolicy
// returns the effective policy even when the policy is attached to a parent (OU or root),
// not just the direct target.
func TestDescribeEffectivePolicy_FullChain(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	// Enable TAG_POLICY so we can use it.
	_, err := b.EnablePolicyType(rootID, "TAG_POLICY")
	require.NoError(t, err)

	// Create a TAG_POLICY attached at root level.
	tagContent := `{"tags":{"env":{"tag_value":{"@@assign":"prod"}}}}`
	rootPolicy, err := b.CreatePolicy("root-tag", "", tagContent, "TAG_POLICY", nil)
	require.NoError(t, err)
	require.NoError(t, b.AttachPolicy(rootPolicy.PolicySummary.ID, rootID))

	// Create an OU with no direct policy.
	ou, err := b.CreateOrganizationalUnit(rootID, "prod-ou", nil)
	require.NoError(t, err)

	// Create an account inside the OU with no direct policy.
	status, err := b.CreateAccount("leaf-acct", "leaf@example.com", "", "", nil)
	require.NoError(t, err)
	require.NoError(t, b.MoveAccount(status.AccountID, rootID, ou.ID))

	// DescribeEffectivePolicy on the account should find the root-level policy via chain walk.
	ep, err := b.DescribeEffectivePolicy("TAG_POLICY", status.AccountID)
	require.NoError(t, err, "effective policy should be found via hierarchy chain")
	assert.NotEmpty(t, ep.PolicyContent)
	assert.Equal(t, "TAG_POLICY", ep.PolicyType)
}

// TestDescribeEffectivePolicy_MergeTagPolicy verifies that TAG_POLICY
// content from multiple levels in the chain is deep-merged, with child overriding parent.
func TestDescribeEffectivePolicy_MergeTagPolicy(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	_, err := b.EnablePolicyType(rootID, "TAG_POLICY")
	require.NoError(t, err)

	// Attach a TAG_POLICY at root that sets "env" and "owner".
	parentContent := `{"tags":{` +
		`"env":{"tag_value":{"@@assign":"prod"}},` +
		`"owner":{"tag_value":{"@@assign":"platform"}}}}`
	parentPolicy, err := b.CreatePolicy("parent-tag", "", parentContent, "TAG_POLICY", nil)
	require.NoError(t, err)
	require.NoError(t, b.AttachPolicy(parentPolicy.PolicySummary.ID, rootID))

	// Create account and attach a TAG_POLICY that overrides "env" but keeps "owner" from parent.
	childContent := `{"tags":{"env":{"tag_value":{"@@assign":"dev"}}}}`
	childPolicy, err := b.CreatePolicy("child-tag", "", childContent, "TAG_POLICY", nil)
	require.NoError(t, err)

	status, err := b.CreateAccount("merge-acct", "merge@example.com", "", "", nil)
	require.NoError(t, err)
	require.NoError(t, b.AttachPolicy(childPolicy.PolicySummary.ID, status.AccountID))

	ep, err := b.DescribeEffectivePolicy("TAG_POLICY", status.AccountID)
	require.NoError(t, err)

	// Merged content should have both "env" (from child) and "owner" (from parent).
	var merged map[string]any
	require.NoError(t, json.Unmarshal([]byte(ep.PolicyContent), &merged))

	tags, hasTags := merged["tags"].(map[string]any)
	require.True(t, hasTags, "merged policy should have 'tags' object")
	assert.Contains(t, tags, "env", "merged policy should contain child 'env' key")
	assert.Contains(t, tags, "owner", "merged policy should contain parent 'owner' key")

	// Child's "env" value should override parent's.
	envTag, hasEnvTag := tags["env"].(map[string]any)
	if hasEnvTag {
		tagVal, hasTagVal := envTag["tag_value"].(map[string]any)
		if hasTagVal {
			assert.Equal(t, "dev", tagVal["@@assign"], "child 'env' should override parent")
		}
	}
}

// TestDescribeEffectivePolicy_OUChain verifies the chain walk through multiple OU levels.
func TestDescribeEffectivePolicy_OUChain(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	_, err := b.EnablePolicyType(rootID, "TAG_POLICY")
	require.NoError(t, err)

	// Attach policy at root.
	rootPolicy, err := b.CreatePolicy("chain-tag", "", `{"Version":"2012-10-17"}`, "TAG_POLICY", nil)
	require.NoError(t, err)
	require.NoError(t, b.AttachPolicy(rootPolicy.PolicySummary.ID, rootID))

	// Build OU1 > OU2 chain.
	ou1, err := b.CreateOrganizationalUnit(rootID, "ou1", nil)
	require.NoError(t, err)
	ou2, err := b.CreateOrganizationalUnit(ou1.ID, "ou2", nil)
	require.NoError(t, err)

	// Account inside OU2 with no direct policy.
	status, err := b.CreateAccount("deep-acct", "deep@example.com", "", "", nil)
	require.NoError(t, err)
	require.NoError(t, b.MoveAccount(status.AccountID, rootID, ou2.ID))

	ep, err := b.DescribeEffectivePolicy("TAG_POLICY", status.AccountID)
	require.NoError(t, err, "root-level policy should be found via deep OU chain")
	assert.NotEmpty(t, ep.PolicyContent)
}

// ---------------------------------------------------------------------------
// Item 22: CreateAccount response must NOT include GovCloudAccountId
// ---------------------------------------------------------------------------

// TestHandler_DescribeEffectivePolicy tests the DescribeEffectivePolicy operation.
func TestHandler_DescribeEffectivePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		policyType  string
		targetID    string
		wantStatus  int
		seedPolicy  bool
		wantContent bool
	}{
		{
			// SCP always resolves via the default FullAWSAccess SCP attached
			// to root, even with no custom policy anywhere in the hierarchy.
			name:        "no_custom_policy_inherits_default_scp",
			policyType:  "SERVICE_CONTROL_POLICY",
			wantStatus:  http.StatusOK,
			wantContent: true,
		},
		{
			name:        "effective_policy_found_on_target",
			policyType:  "SERVICE_CONTROL_POLICY",
			seedPolicy:  true,
			wantStatus:  http.StatusOK,
			wantContent: true,
		},
		{
			name:       "missing_policy_type",
			policyType: "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			h := organizations.NewHandler(b)

			doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

			var targetID string

			if tt.seedPolicy {
				// Create an account and attach a policy of the given type to it.
				acctRec := doRequest(t, h, "CreateAccount", map[string]any{
					"AccountName": "effective-test",
					"Email":       "eff@example.com",
				})
				require.Equal(t, http.StatusOK, acctRec.Code)

				var acctResp map[string]any
				require.NoError(t, json.NewDecoder(acctRec.Body).Decode(&acctResp))
				status := acctResp["CreateAccountStatus"].(map[string]any)
				targetID = status["AccountId"].(string)

				// Create a policy of the given type.
				policyRec := doRequest(t, h, "CreatePolicy", map[string]any{
					"Name":    "test-scp",
					"Content": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
					"Type":    tt.policyType,
				})
				require.Equal(t, http.StatusOK, policyRec.Code)

				var policyResp map[string]any
				require.NoError(t, json.NewDecoder(policyRec.Body).Decode(&policyResp))
				policyObj := policyResp["Policy"].(map[string]any)
				summary := policyObj["PolicySummary"].(map[string]any)
				policyID := summary["Id"].(string)

				// Attach the policy to the account.
				attachRec := doRequest(t, h, "AttachPolicy", map[string]any{
					"PolicyId": policyID,
					"TargetId": targetID,
				})
				require.Equal(t, http.StatusOK, attachRec.Code)
			}

			body := map[string]any{}
			if tt.policyType != "" {
				body["PolicyType"] = tt.policyType
			}

			if targetID != "" {
				body["TargetId"] = targetID
			}

			rec := doRequest(t, h, "DescribeEffectivePolicy", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantContent {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				ep, ok := resp["EffectivePolicy"].(map[string]any)
				require.True(t, ok, "response must have EffectivePolicy")
				assert.NotEmpty(t, ep["PolicyContent"])
				assert.NotEmpty(t, ep["PolicyId"])
				assert.Equal(t, tt.policyType, ep["PolicyType"])
				assert.NotZero(t, ep["LastUpdatedTimestamp"])

				if tt.name == "no_custom_policy_inherits_default_scp" {
					assert.Equal(t, "p-FullAWSAccess", ep["PolicyId"])
				}
			}
		})
	}
}

// TestBackend_DescribeEffectivePolicy tests the DescribeEffectivePolicy backend method.
func TestBackend_DescribeEffectivePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyType string
		targetID   string
		seedPolicy bool
		noOrg      bool
		wantErr    bool
	}{
		{
			// SCP always resolves via the default FullAWSAccess SCP attached
			// to root, even with no custom policy anywhere in the hierarchy.
			name:       "no_custom_policy_inherits_default_scp",
			policyType: "SERVICE_CONTROL_POLICY",
		},
		{
			name:       "effective_policy_found",
			policyType: "SERVICE_CONTROL_POLICY",
			seedPolicy: true,
		},
		{
			name:       "no_org_returns_error",
			policyType: "SERVICE_CONTROL_POLICY",
			noOrg:      true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if !tt.noOrg {
				_, _, err := b.CreateOrganization("ALL")
				require.NoError(t, err)
			}

			var targetID string

			if tt.seedPolicy {
				// Create an account.
				status, acctErr := b.CreateAccount("ep-test", "ep@example.com", "", "", nil)
				require.NoError(t, acctErr)
				targetID = status.AccountID

				// Create a policy.
				p, pErr := b.CreatePolicy(
					"ep-scp",
					"desc",
					`{"Version":"2012-10-17","Statement":[]}`,
					tt.policyType,
					nil,
				)
				require.NoError(t, pErr)

				// Attach policy to account.
				require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, targetID))
			}

			ep, err := b.DescribeEffectivePolicy(tt.policyType, targetID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, ep)
			assert.Equal(t, tt.policyType, ep.PolicyType)
			assert.NotEmpty(t, ep.PolicyContent)
			assert.NotEmpty(t, ep.PolicyID)

			if tt.name == "no_custom_policy_inherits_default_scp" {
				assert.Equal(t, "p-FullAWSAccess", ep.PolicyID)
				assert.JSONEq(t,
					`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
					ep.PolicyContent)
			}
		})
	}
}
