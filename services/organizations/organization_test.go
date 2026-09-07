package organizations_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestBackend_OrgLifecycle tests organization creation, describe, and deletion.
func TestBackend_OrgLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		featureSet   string
		wantErr      bool
		doubleCreate bool
	}{
		{
			name:       "create_ALL",
			featureSet: "ALL",
		},
		{
			name:       "create_CONSOLIDATED_BILLING",
			featureSet: "CONSOLIDATED_BILLING",
		},
		{
			name:         "duplicate_create_fails",
			featureSet:   "ALL",
			doubleCreate: true,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			org, root, err := b.CreateOrganization(tt.featureSet)

			if tt.wantErr && !tt.doubleCreate {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, org)
			require.NotNil(t, root)
			assert.NotEmpty(t, org.ID)
			assert.NotEmpty(t, org.ARN)
			assert.Equal(t, tt.featureSet, org.FeatureSet)
			assert.NotEmpty(t, root.ID)
			// The account that calls CreateOrganization becomes the management
			// account, matching real AWS: its ID must equal the caller identity
			// this backend reports elsewhere (e.g. to STS/IAM), not a synthetic
			// counter-derived account ID.
			assert.Equal(t, b.AccountID(), org.MasterAccountID)

			if tt.doubleCreate {
				_, _, err2 := b.CreateOrganization(tt.featureSet)
				require.Error(t, err2)

				return
			}

			// DescribeOrganization.
			descOrg, err := b.DescribeOrganization()
			require.NoError(t, err)
			assert.Equal(t, org.ID, descOrg.ID)

			// DeleteOrganization.
			err = b.DeleteOrganization()
			require.NoError(t, err)

			// After deletion, describe should fail.
			_, err = b.DescribeOrganization()
			require.Error(t, err)
		})
	}
}

// TestHandler_CreateOrganization tests the HTTP handler for CreateOrganization.
func TestHandler_CreateOrganization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantOrgID  bool
	}{
		{
			name:       "creates_org_ALL",
			body:       map[string]any{"FeatureSet": "ALL"},
			wantStatus: http.StatusOK,
			wantOrgID:  true,
		},
		{
			name:       "creates_org_no_feature_set",
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantOrgID:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateOrganization", tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantOrgID {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				org, ok := resp["Organization"].(map[string]any)
				require.True(t, ok, "response should contain Organization")
				assert.NotEmpty(t, org["Id"])
			}
		})
	}
}

// TestHandler_DescribeOrganization tests the HTTP handler for DescribeOrganization.
func TestHandler_DescribeOrganization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantStatus  int
		createFirst bool
	}{
		{
			name:        "no_org_returns_error",
			wantStatus:  http.StatusBadRequest,
			createFirst: false,
		},
		{
			name:        "describes_existing_org",
			wantStatus:  http.StatusOK,
			createFirst: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.createFirst {
				rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(t, h, "DescribeOrganization", map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestBackend_EnsureOrgExists tests the EnsureOrgExists helper.
func TestBackend_EnsureOrgExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		hasOrg  bool
		wantErr bool
	}{
		{
			name:   "org_exists",
			hasOrg: true,
		},
		{
			name:    "no_org",
			hasOrg:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.hasOrg {
				_, _, err := b.CreateOrganization("ALL")
				require.NoError(t, err)
			}

			err := b.EnsureOrgExists()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDescribeOrganization_AvailablePolicyTypes verifies that DescribeOrganization
// returns all 8 valid policy types in AvailablePolicyTypes, with correct ENABLED/DISABLED status.
func TestDescribeOrganization_AvailablePolicyTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		enableTypes  []string
		wantEnabled  []string
		wantDisabled []string
		wantTotalLen int
	}{
		{
			name:         "all_disabled_initially",
			enableTypes:  nil,
			wantEnabled:  nil,
			wantTotalLen: 8,
			wantDisabled: []string{
				"SERVICE_CONTROL_POLICY",
				"RESOURCE_CONTROL_POLICY",
				"TAG_POLICY",
				"BACKUP_POLICY",
				"AISERVICES_OPT_OUT_POLICY",
				"CHATBOT_POLICY",
				"DECLARATIVE_POLICY_EC2",
				"SECURITYHUB_POLICY",
			},
		},
		{
			name:        "one_enabled",
			enableTypes: []string{"SERVICE_CONTROL_POLICY"},
			wantEnabled: []string{"SERVICE_CONTROL_POLICY"},
			wantDisabled: []string{
				"RESOURCE_CONTROL_POLICY",
				"TAG_POLICY",
				"BACKUP_POLICY",
				"AISERVICES_OPT_OUT_POLICY",
				"CHATBOT_POLICY",
				"DECLARATIVE_POLICY_EC2",
				"SECURITYHUB_POLICY",
			},
			wantTotalLen: 8,
		},
		{
			name: "multiple_enabled",
			enableTypes: []string{
				"SERVICE_CONTROL_POLICY",
				"RESOURCE_CONTROL_POLICY",
				"SECURITYHUB_POLICY",
			},
			wantEnabled: []string{
				"SERVICE_CONTROL_POLICY",
				"RESOURCE_CONTROL_POLICY",
				"SECURITYHUB_POLICY",
			},
			wantTotalLen: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			for _, pt := range tt.enableTypes {
				_, err := b.EnablePolicyType(rootID, pt)
				require.NoError(t, err)
			}

			org, err := b.DescribeOrganization()
			require.NoError(t, err)

			assert.Len(t, org.AvailablePolicyTypes, tt.wantTotalLen, "should have 8 policy types")

			// Build a map for easy lookup.
			statusMap := make(map[string]string)
			for _, pt := range org.AvailablePolicyTypes {
				statusMap[pt.Type] = pt.Status
			}

			for _, enabled := range tt.wantEnabled {
				assert.Equal(t, "ENABLED", statusMap[enabled], "type %s should be ENABLED", enabled)
			}

			for _, disabled := range tt.wantDisabled {
				assert.Equal(t, "DISABLED", statusMap[disabled], "type %s should be DISABLED", disabled)
			}
		})
	}
}

// TestAvailablePolicyTypes_ViaHandler verifies DescribeOrganization via HTTP handler.
func TestAvailablePolicyTypes_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create org.
	rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})
	require.Equal(t, http.StatusOK, rec.Code)

	// Describe org.
	rec = doRequest(t, h, "DescribeOrganization", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	org := resp["Organization"].(map[string]any)
	pts, ok := org["AvailablePolicyTypes"].([]any)
	require.True(t, ok, "AvailablePolicyTypes should be present")
	assert.Len(t, pts, 8, "should have 8 available policy types")
}

// ---------------------------------------------------------------------------
// Item 2: Validate all 8 policy types in validPolicyTypes
// ---------------------------------------------------------------------------

// TestCreateOrganization_FeatureSetValidation verifies that invalid FeatureSets are rejected.
func TestCreateOrganization_FeatureSetValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		featureSet string
		wantErr    bool
	}{
		{name: "ALL_valid", featureSet: "ALL", wantErr: false},
		{name: "CONSOLIDATED_BILLING_valid", featureSet: "CONSOLIDATED_BILLING", wantErr: false},
		{name: "empty_defaults_to_ALL", featureSet: "", wantErr: false},
		{name: "invalid_FULL", featureSet: "FULL", wantErr: true},
		{name: "invalid_NONE", featureSet: "NONE", wantErr: true},
		{name: "invalid_lowercase_all", featureSet: "all", wantErr: true},
		{name: "invalid_gibberish", featureSet: "SOMETHING_ELSE", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			_, _, err := b.CreateOrganization(tt.featureSet)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateOrganization_FeatureSetViaHandler tests FeatureSet validation via handler.
func TestCreateOrganization_FeatureSetViaHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		featureSet string
		wantStatus int
	}{
		{name: "ALL", featureSet: "ALL", wantStatus: http.StatusOK},
		{name: "CONSOLIDATED_BILLING", featureSet: "CONSOLIDATED_BILLING", wantStatus: http.StatusOK},
		{name: "invalid", featureSet: "BOGUS", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": tt.featureSet})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// ---------------------------------------------------------------------------
// Item 4: RoleName and IamUserAccessToBilling in CreateAccount
// ---------------------------------------------------------------------------

// TestAudit2_DescribeOrganization_AllTypesPresent verifies that all 8 policy types
// appear in AvailablePolicyTypes even after enabling/disabling some.
func TestDescribeOrganization_AllTypesAfterEnableDisable(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	// Enable two types.
	_, err := b.EnablePolicyType(rootID, "SERVICE_CONTROL_POLICY")
	require.NoError(t, err)

	_, err = b.EnablePolicyType(rootID, "TAG_POLICY")
	require.NoError(t, err)

	// The default FullAWSAccess SCP is attached to root; detach it so
	// disabling SCP isn't rejected by the still-attached guard.
	require.NoError(t, b.DetachPolicy("p-FullAWSAccess", rootID))

	// Disable one.
	_, err = b.DisablePolicyType(rootID, "SERVICE_CONTROL_POLICY")
	require.NoError(t, err)

	org, err := b.DescribeOrganization()
	require.NoError(t, err)

	assert.Len(t, org.AvailablePolicyTypes, 8)

	statusMap := make(map[string]string)
	for _, pt := range org.AvailablePolicyTypes {
		statusMap[pt.Type] = pt.Status
	}

	assert.Equal(t, "DISABLED", statusMap["SERVICE_CONTROL_POLICY"])
	assert.Equal(t, "ENABLED", statusMap["TAG_POLICY"])
	assert.Equal(t, "DISABLED", statusMap["RESOURCE_CONTROL_POLICY"])
	assert.Equal(t, "DISABLED", statusMap["SECURITYHUB_POLICY"])
}

// TestDeleteOrganization_MemberConstraints verifies that DeleteOrganization
// rejects a still-populated organization (OrganizationNotEmptyException) and
// succeeds once only the management account remains.
func TestDeleteOrganization_MemberConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, b *organizations.InMemoryBackend)
		name    string
		wantErr bool
	}{
		{
			name: "with_members_rejected",
			setup: func(t *testing.T, b *organizations.InMemoryBackend) {
				t.Helper()

				_, err := b.CreateAccount("member", "member@example.com", "", "", nil)
				require.NoError(t, err)
			},
			wantErr: true,
		},
		{
			name:    "only_master_ok",
			setup:   func(_ *testing.T, _ *organizations.InMemoryBackend) {},
			wantErr: false,
		},
		{
			name: "after_removing_members_ok",
			setup: func(t *testing.T, b *organizations.InMemoryBackend) {
				t.Helper()

				status, err := b.CreateAccount("member", "member2@example.com", "", "", nil)
				require.NoError(t, err)
				require.NoError(t, b.RemoveAccountFromOrganization(status.AccountID))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)
			tt.setup(t, b)

			err := b.DeleteOrganization()
			if tt.wantErr {
				require.Error(t, err, "DeleteOrganization must fail while member accounts exist")

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestDeleteOrganization_WithMembers_ViaHandler verifies HTTP 400 +
// OrganizationNotEmptyException.
func TestDeleteOrganization_WithMembers_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

	ca := doRequest(t, h, "CreateAccount", map[string]any{
		"AccountName": "member",
		"Email":       "member@example.com",
	})
	require.Equal(t, http.StatusOK, ca.Code)

	rec := doRequest(t, h, "DeleteOrganization", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "OrganizationNotEmptyException", errResp["__type"])
}

// ---------------------------------------------------------------------------
// DisablePolicyType: ConstraintViolationException when policies of that type are attached
// ---------------------------------------------------------------------------

// TestHandler_DeleteOrganization tests the HTTP handler for DeleteOrganization.
func TestHandler_DeleteOrganization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "deletes_org",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			rec := doRequest(t, h, "DeleteOrganization", map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
