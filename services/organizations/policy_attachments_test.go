package organizations_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestBackend_PolicyAttachment tests attaching and detaching policies.
func TestBackend_PolicyAttachment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name: "attach_and_detach",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// Create a policy.
			policy, err := b.CreatePolicy(
				"test-scp",
				"test",
				`{"Version":"2012-10-17","Statement":[]}`,
				"SERVICE_CONTROL_POLICY",
				nil,
			)
			require.NoError(t, err)

			// AttachPolicy.
			err = b.AttachPolicy(policy.PolicySummary.ID, rootID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Duplicate attach should fail.
			err = b.AttachPolicy(policy.PolicySummary.ID, rootID)
			require.Error(t, err)

			// ListPoliciesForTarget.
			policies, err := b.ListPoliciesForTarget(rootID, "SERVICE_CONTROL_POLICY")
			require.NoError(t, err)

			found := false

			for _, p := range policies {
				if p.PolicySummary.ID == policy.PolicySummary.ID {
					found = true

					break
				}
			}

			assert.True(t, found, "attached policy should appear in ListPoliciesForTarget")

			// ListTargetsForPolicy.
			targets, err := b.ListTargetsForPolicy(policy.PolicySummary.ID)
			require.NoError(t, err)
			assert.NotEmpty(t, targets)

			// DetachPolicy.
			err = b.DetachPolicy(policy.PolicySummary.ID, rootID)
			require.NoError(t, err)

			// Double detach should fail.
			err = b.DetachPolicy(policy.PolicySummary.ID, rootID)
			require.Error(t, err)
		})
	}
}

// TestAttachPolicy_Targets tests attaching policies to root, OU, and account.
func TestAttachPolicy_Targets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		targetKind string // "root", "ou", "account"
		wantInList bool
	}{
		{name: "attach_to_root", targetKind: "root", wantInList: true},
		{name: "attach_to_ou", targetKind: "ou", wantInList: true},
		{name: "attach_to_account", targetKind: "account", wantInList: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			p, err := b.CreatePolicy("test-scp", "", `{"Version":"2012-10-17"}`,
				"SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			var targetID string

			switch tt.targetKind {
			case "root":
				targetID = rootID
			case "ou":
				ou, ouErr := b.CreateOrganizationalUnit(rootID, "policy-target-ou", nil)
				require.NoError(t, ouErr)
				targetID = ou.ID
			case "account":
				s, acctErr := b.CreateAccount("policy-target", "pt@example.com", "", "", nil)
				require.NoError(t, acctErr)
				targetID = s.AccountID
			}

			require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, targetID))

			// ListPoliciesForTarget.
			policies, err := b.ListPoliciesForTarget(targetID, "SERVICE_CONTROL_POLICY")
			require.NoError(t, err)
			found := false
			for _, lp := range policies {
				if lp.PolicySummary.ID == p.PolicySummary.ID {
					found = true
				}
			}
			assert.Equal(t, tt.wantInList, found)

			// ListTargetsForPolicy.
			targets, err := b.ListTargetsForPolicy(p.PolicySummary.ID)
			require.NoError(t, err)
			assert.Len(t, targets, 1)
			assert.Equal(t, targetID, targets[0].TargetID)
		})
	}
}

// TestAttachPolicy_Limits tests the 5-policy-per-target-per-type limit.
func TestAttachPolicy_Limits(t *testing.T) {
	t.Parallel()

	// attachCount is new SCPs on top of the default FullAWSAccess SCP that
	// every organization is created with (already attached to root), so the
	// 5-per-target-per-type limit is hit one new attach earlier than a bare
	// count of 5 would suggest.
	tests := []struct {
		name        string
		attachCount int
		wantLastErr bool
	}{
		{name: "exactly_at_limit", attachCount: 4, wantLastErr: false},
		{name: "exceeds_limit", attachCount: 5, wantLastErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			var lastErr error

			for i := range tt.attachCount {
				p, err := b.CreatePolicy(
					"scp-"+string(rune('a'+i)),
					"",
					`{"Version":"2012-10-17"}`,
					"SERVICE_CONTROL_POLICY",
					nil,
				)
				require.NoError(t, err)
				lastErr = b.AttachPolicy(p.PolicySummary.ID, rootID)
				if lastErr != nil {
					break
				}
			}

			if tt.wantLastErr {
				require.Error(t, lastErr)
			} else {
				require.NoError(t, lastErr)
			}
		})
	}
}

// TestAttachPolicy_CrossTypes tests that the per-type limit is per-type
// (attaching 5 SCPs and 5 TAGs to the same target should succeed).
func TestAttachPolicy_CrossTypes(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	// The default FullAWSAccess SCP is already attached to root, occupying
	// one of the 5 SCP slots, so only 4 new SCPs fit; TAG_POLICY is
	// unaffected and still fits 5.
	counts := map[string]int{"SERVICE_CONTROL_POLICY": 4, "TAG_POLICY": 5}

	for _, pt := range []string{"SERVICE_CONTROL_POLICY", "TAG_POLICY"} {
		for i := range counts[pt] {
			p, err := b.CreatePolicy(
				pt+"-"+string(rune('a'+i)),
				"",
				`{"Version":"2012-10-17"}`,
				pt,
				nil,
			)
			require.NoError(t, err)
			require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, rootID),
				"attaching policy %d of type %s should succeed", i+1, pt)
		}
	}
}

// TestDetachPolicy_Scenarios tests detach lifecycle.
func TestDetachPolicy_Scenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		attachFirst bool
		wantErr     bool
	}{
		{name: "detach_attached", attachFirst: true},
		{name: "detach_not_attached_fails", attachFirst: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			p, err := b.CreatePolicy("p", "", `{}`, "SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			if tt.attachFirst {
				require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, rootID))
			}

			err = b.DetachPolicy(p.PolicySummary.ID, rootID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// After detach, ListTargetsForPolicy should be empty.
			targets, err := b.ListTargetsForPolicy(p.PolicySummary.ID)
			require.NoError(t, err)
			assert.Empty(t, targets)
		})
	}
}

// TestListPoliciesForTarget_Filter tests filtering by policy type.
func TestListPoliciesForTarget_Filter(t *testing.T) {
	t.Parallel()

	t.Run("empty_filter_rejected", func(t *testing.T) {
		t.Parallel()

		b, rootID := newOrgBackend(t)
		_, err := b.ListPoliciesForTarget(rootID, "")
		require.Error(t, err, "empty filter must be rejected (AWS requires Filter)")
	})

	tests := []struct {
		name      string
		filter    string
		wantCount int
	}{
		// +1 for the default FullAWSAccess SCP already attached to root.
		{name: "filter_scp", filter: "SERVICE_CONTROL_POLICY", wantCount: 3},
		{name: "filter_tag", filter: "TAG_POLICY", wantCount: 1},
		{name: "filter_backup", filter: "BACKUP_POLICY", wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			attach := func(name, pt string) {
				p, err := b.CreatePolicy(name, "", `{}`, pt, nil)
				require.NoError(t, err)
				require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, rootID))
			}

			attach("scp1", "SERVICE_CONTROL_POLICY")
			attach("scp2", "SERVICE_CONTROL_POLICY")
			attach("tag1", "TAG_POLICY")

			policies, err := b.ListPoliciesForTarget(rootID, tt.filter)
			require.NoError(t, err)
			assert.Len(t, policies, tt.wantCount)
		})
	}
}

// ---------------------------------------------------------------------------
// EnableAWSServiceAccess
// ---------------------------------------------------------------------------

// TestAttachPolicy_TargetValidation verifies that AttachPolicy validates the target.
func TestAttachPolicy_TargetValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		targetFn func(b *organizations.InMemoryBackend, rootID string) string
		name     string
		wantErr  bool
	}{
		{
			name: "root_target_valid",
			targetFn: func(_ *organizations.InMemoryBackend, rootID string) string {
				return rootID
			},
			wantErr: false,
		},
		{
			name: "ou_target_valid",
			targetFn: func(b *organizations.InMemoryBackend, rootID string) string {
				ou, err := b.CreateOrganizationalUnit(rootID, "TestOU", nil)
				if err != nil {
					panic(err)
				}

				return ou.ID
			},
			wantErr: false,
		},
		{
			name: "account_target_valid",
			targetFn: func(b *organizations.InMemoryBackend, _ string) string {
				s, err := b.CreateAccount("target-acct", "target@example.com", "", "", nil)
				if err != nil {
					panic(err)
				}

				return s.AccountID
			},
			wantErr: false,
		},
		{
			name: "unknown_target_rejected",
			targetFn: func(_ *organizations.InMemoryBackend, _ string) string {
				return "ou-nonexistent-12345678"
			},
			wantErr: true,
		},
		{
			name: "random_string_rejected",
			targetFn: func(_ *organizations.InMemoryBackend, _ string) string {
				return "not-a-real-target"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			p, err := b.CreatePolicy("test-pol", "desc", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			targetID := tt.targetFn(b, rootID)

			err = b.AttachPolicy(p.PolicySummary.ID, targetID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Item 12: Tag resource existence validation
// ---------------------------------------------------------------------------

// TestAttachPolicy_UnknownTarget_ViaHandler tests AttachPolicy target validation via handler.
func TestAttachPolicy_UnknownTarget_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "CreatePolicy", map[string]any{
		"Name":    "test-pol",
		"Type":    "SERVICE_CONTROL_POLICY",
		"Content": `{"Version":"2012-10-17","Statement":[]}`,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var polResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&polResp))
	policyID := polResp["Policy"].(map[string]any)["PolicySummary"].(map[string]any)["Id"].(string)

	// Attempt to attach to a non-existent target.
	rec = doRequest(t, h, "AttachPolicy", map[string]any{
		"PolicyId": policyID,
		"TargetId": "ou-nonexistent-12345678",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestListPoliciesForTarget_EmptyFilter verifies that ListPoliciesForTarget rejects empty Filter.
func TestListPoliciesForTarget_EmptyFilter(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	_, err := b.ListPoliciesForTarget(rootID, "")
	require.Error(t, err, "empty Filter must be rejected")

	// Valid filter works fine.
	_, err = b.ListPoliciesForTarget(rootID, "TAG_POLICY")
	require.NoError(t, err)
}

// TestListTargetsForPolicy_Sorted verifies sorted output by target ID.
func TestListTargetsForPolicy_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "sorted_targets"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)

			p, err := b.CreatePolicy("sorted-pol", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
			require.NoError(t, err)

			// Attach to root + a couple of accounts.
			err = b.AttachPolicy(p.PolicySummary.ID, roots[0].ID)
			require.NoError(t, err)

			for i := range 2 {
				s, err2 := b.CreateAccount("t-acct", fmt.Sprintf("t%d@example.com", i), "", "", nil)
				require.NoError(t, err2)
				require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, s.AccountID))
			}

			targets, err := b.ListTargetsForPolicy(p.PolicySummary.ID)
			require.NoError(t, err)
			require.NotEmpty(t, targets)

			for i := 1; i < len(targets); i++ {
				assert.LessOrEqual(t, targets[i-1].TargetID, targets[i].TargetID)
			}
		})
	}
}

// TestResolveTargetSummary_Root verifies the ROOT target type is returned correctly.
func TestResolveTargetSummary_Root(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantType string
	}{
		{name: "root_target", wantType: "ROOT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)
			require.Len(t, roots, 1)

			summary := organizations.ResolveTargetSummaryExported(b, roots[0].ID)
			assert.Equal(t, tt.wantType, summary.Type)
			assert.Equal(t, roots[0].ID, summary.TargetID)
		})
	}
}

// TestResolveTargetSummary_OU verifies the ORGANIZATIONAL_UNIT target type.
func TestResolveTargetSummary_OU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ouName   string
		wantType string
	}{
		{name: "ou_target", ouName: "test-ou", wantType: "ORGANIZATIONAL_UNIT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			roots, err := b.ListRoots()
			require.NoError(t, err)

			ou, err := b.CreateOrganizationalUnit(roots[0].ID, tt.ouName, nil)
			require.NoError(t, err)

			summary := organizations.ResolveTargetSummaryExported(b, ou.ID)
			assert.Equal(t, tt.wantType, summary.Type)
			assert.Equal(t, tt.ouName, summary.Name)
		})
	}
}
