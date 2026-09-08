package ssm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

// TestCreatePatchBaseline_ApprovalRulesValidation covers validateApprovalRules,
// which previously didn't exist -- any ApprovalRules shape, valid or not, was
// accepted and stored verbatim.
func TestCreatePatchBaseline_ApprovalRulesValidation(t *testing.T) {
	t.Parallel()

	filterGroup := &ssm.PatchFilterGroup{
		PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"AmazonLinux2"}}},
	}

	cases := []struct {
		rules   *ssm.PatchRuleGroup
		name    string
		wantErr bool
	}{
		{
			name:    "nil_rules_ok",
			rules:   nil,
			wantErr: false,
		},
		{
			name: "approve_after_days_ok",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: filterGroup,
				ApproveAfterDays: aws.Int32(7),
			}}},
			wantErr: false,
		},
		{
			name: "approve_until_date_ok",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: filterGroup,
				ApproveUntilDate: "2025-01-01",
			}}},
			wantErr: false,
		},
		{
			name:    "empty_rules_list_rejected",
			rules:   &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{}},
			wantErr: true,
		},
		{
			name: "missing_filter_group_rejected",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				ApproveAfterDays: aws.Int32(7),
			}}},
			wantErr: true,
		},
		{
			name: "filter_missing_key_rejected",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{PatchFilters: []ssm.PatchFilter{{Values: []string{"x"}}}},
				ApproveAfterDays: aws.Int32(7),
			}}},
			wantErr: true,
		},
		{
			name: "neither_after_days_nor_until_date_rejected",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: filterGroup,
			}}},
			wantErr: true,
		},
		{
			// Neither the AWS docs nor validatePatchRule forbid setting both --
			// see validateApprovalRule and ruleOutcomeForPatch for the tie-break.
			name: "both_after_days_and_until_date_ok",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: filterGroup,
				ApproveAfterDays: aws.Int32(7),
				ApproveUntilDate: "2025-01-01",
			}}},
			wantErr: false,
		},
		{
			name: "approve_after_days_negative_rejected",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: filterGroup,
				ApproveAfterDays: aws.Int32(-1),
			}}},
			wantErr: true,
		},
		{
			name: "approve_after_days_over_360_rejected",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: filterGroup,
				ApproveAfterDays: aws.Int32(361),
			}}},
			wantErr: true,
		},
		{
			name: "malformed_until_date_rejected",
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: filterGroup,
				ApproveUntilDate: "01/01/2025",
			}}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			_, err := b.CreatePatchBaseline(context.Background(), &ssm.CreatePatchBaselineInput{
				Name:          "pb-" + tc.name,
				ApprovalRules: tc.rules,
			})

			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ssm.ErrValidationException)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreatePatchBaseline_RejectedPatchesActionValidation covers
// validateRejectedPatchesAction, which previously accepted any string.
func TestCreatePatchBaseline_RejectedPatchesActionValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		action  string
		name    string
		wantErr bool
	}{
		{name: "empty_ok", action: "", wantErr: false},
		{name: "block_ok", action: "BLOCK", wantErr: false},
		{name: "allow_as_dependency_ok", action: "ALLOW_AS_DEPENDENCY", wantErr: false},
		{name: "garbage_rejected", action: "DELETE_EVERYTHING", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			_, err := b.CreatePatchBaseline(context.Background(), &ssm.CreatePatchBaselineInput{
				Name:                  "pb-" + tc.name,
				RejectedPatchesAction: tc.action,
			})

			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ssm.ErrValidationException)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDescribeEffectivePatches_ApprovalRules exercises rule-based approval
// evaluation directly against DescribeEffectivePatchesForPatchBaseline, the
// observable surface effectivePatchesForBaseline backs.
func TestDescribeEffectivePatches_ApprovalRules(t *testing.T) {
	t.Parallel()

	oldRelease := ssm.UnixTimeFloat(time.Now().AddDate(0, 0, -400))
	recentRelease := ssm.UnixTimeFloat(time.Now().AddDate(0, 0, -1))

	cases := []struct {
		rules          *ssm.PatchRuleGroup
		name           string
		wantStatus     string
		wantCompliance string
		releaseDate    float64
	}{
		{
			name:        "approve_after_days_elapsed",
			releaseDate: oldRelease,
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{
					PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"AmazonLinux2"}}},
				},
				ApproveAfterDays: aws.Int32(30),
				ComplianceLevel:  "CRITICAL",
			}}},
			wantStatus:     "APPROVED",
			wantCompliance: "CRITICAL",
		},
		{
			name:        "approve_after_days_not_elapsed",
			releaseDate: recentRelease,
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{
					PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"AmazonLinux2"}}},
				},
				ApproveAfterDays: aws.Int32(30),
			}}},
			wantStatus: "PENDING_APPROVAL",
		},
		{
			name:        "approve_until_date_past_release",
			releaseDate: ssm.UnixTimeFloat(time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)),
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{
					PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"AmazonLinux2"}}},
				},
				ApproveUntilDate: "2024-06-01",
			}}},
			wantStatus: "APPROVED",
		},
		{
			name:        "approve_until_date_before_release",
			releaseDate: ssm.UnixTimeFloat(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)),
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{
					PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"AmazonLinux2"}}},
				},
				ApproveUntilDate: "2024-06-01",
			}}},
			wantStatus: "PENDING_APPROVAL",
		},
		{
			// Both set is accepted (not rejected -- see
			// TestCreatePatchBaseline_ApprovalRulesValidation/both_after_days_and_until_date_ok).
			// ApproveUntilDate must win: alone, a 30-day ApproveAfterDays rule
			// would NOT yet approve a patch released yesterday, but the
			// ApproveUntilDate cutoff (today) does.
			name:        "both_set_until_date_wins",
			releaseDate: recentRelease,
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{
					PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"AmazonLinux2"}}},
				},
				ApproveAfterDays: aws.Int32(30),
				ApproveUntilDate: time.Now().UTC().Format(time.DateOnly),
			}}},
			wantStatus: "APPROVED",
		},
		{
			name:        "unsupported_filter_key_fails_closed",
			releaseDate: oldRelease,
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{
					PatchFilters: []ssm.PatchFilter{{Key: "ARCH", Values: []string{"x86_64"}}},
				},
				ApproveAfterDays: aws.Int32(30),
			}}},
			wantStatus: "PENDING_APPROVAL",
		},
		{
			name:        "non_matching_product_stays_pending",
			releaseDate: oldRelease,
			rules: &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
				PatchFilterGroup: &ssm.PatchFilterGroup{
					PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"WindowsServer2022"}}},
				},
				ApproveAfterDays: aws.Int32(30),
			}}},
			wantStatus: "PENDING_APPROVAL",
		},
		{
			name:           "no_approval_rules_unchanged",
			releaseDate:    oldRelease,
			rules:          nil,
			wantStatus:     "PENDING_APPROVAL",
			wantCompliance: "UNSPECIFIED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			b.AddAvailablePatchInternal(ssm.Patch{
				Name: "TEST-PATCH-1", Product: "AmazonLinux2",
				Classification: "Security", Severity: "Critical",
				ReleaseDate: tc.releaseDate,
			})

			created, err := b.CreatePatchBaseline(context.Background(), &ssm.CreatePatchBaselineInput{
				Name:            "pb-" + tc.name,
				OperatingSystem: "AMAZON_LINUX_2",
				ApprovalRules:   tc.rules,
			})
			require.NoError(t, err)

			out, err := b.DescribeEffectivePatchesForPatchBaseline(
				context.Background(),
				&ssm.DescribeEffectivePatchesForPatchBaselineInput{BaselineID: created.BaselineID},
			)
			require.NoError(t, err)
			require.Len(t, out.EffectivePatches, 1)

			ep := out.EffectivePatches[0]
			require.NotNil(t, ep.PatchStatus)
			assert.Equal(t, tc.wantStatus, ep.PatchStatus.DeploymentStatus)

			if tc.wantCompliance != "" {
				assert.Equal(t, tc.wantCompliance, ep.PatchStatus.ComplianceLevel)
			}
		})
	}
}

// TestDescribeEffectivePatches_ExplicitListsOverrideApprovalRules verifies the
// documented precedence (RejectedPatches > ApprovedPatches > ApprovalRules,
// per the AWS Systems Manager User Guide's "How security patches are
// selected"): explicit approval/rejection must win even when ApprovalRules
// would otherwise decide the same patch differently.
func TestDescribeEffectivePatches_ExplicitListsOverrideApprovalRules(t *testing.T) {
	t.Parallel()

	oldRelease := ssm.UnixTimeFloat(time.Now().AddDate(0, 0, -400))

	rules := &ssm.PatchRuleGroup{PatchRules: []ssm.PatchRule{{
		PatchFilterGroup: &ssm.PatchFilterGroup{
			PatchFilters: []ssm.PatchFilter{{Key: "PRODUCT", Values: []string{"AmazonLinux2"}}},
		},
		ApproveAfterDays: aws.Int32(30),
	}}}

	b := ssm.NewInMemoryBackend()
	b.AddAvailablePatchInternal(ssm.Patch{
		Name: "EXPLICIT-APPROVED-1", Product: "AmazonLinux2", ReleaseDate: oldRelease,
	})
	b.AddAvailablePatchInternal(ssm.Patch{
		Name: "EXPLICIT-REJECTED-1", Product: "AmazonLinux2", ReleaseDate: oldRelease,
	})

	created, err := b.CreatePatchBaseline(context.Background(), &ssm.CreatePatchBaselineInput{
		Name:            "pb-explicit-precedence",
		OperatingSystem: "AMAZON_LINUX_2",
		ApprovedPatches: []string{"EXPLICIT-APPROVED-1"},
		RejectedPatches: []string{"EXPLICIT-REJECTED-1"},
		ApprovalRules:   rules,
	})
	require.NoError(t, err)

	out, err := b.DescribeEffectivePatchesForPatchBaseline(
		context.Background(),
		&ssm.DescribeEffectivePatchesForPatchBaselineInput{BaselineID: created.BaselineID},
	)
	require.NoError(t, err)

	statuses := map[string]string{}
	for _, ep := range out.EffectivePatches {
		statuses[ep.Patch.Name] = ep.PatchStatus.DeploymentStatus
	}

	assert.Equal(t, "EXPLICIT_APPROVED", statuses["EXPLICIT-APPROVED-1"])
	assert.Equal(t, "EXPLICIT_REJECTED", statuses["EXPLICIT-REJECTED-1"])
}

// TestSendCommand_RunPatchBaseline_ApprovalRules_Compliance proves rule-based
// approval reaches the observable compliance surface (InstancePatchState /
// PatchComplianceData), not just DescribeEffectivePatchesForPatchBaseline.
func TestSendCommand_RunPatchBaseline_ApprovalRules_Compliance(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	oldRelease := ssm.UnixTimeFloat(time.Now().AddDate(0, 0, -400))
	recentRelease := ssm.UnixTimeFloat(time.Now().AddDate(0, 0, -1))

	b.AddAvailablePatchInternal(ssm.Patch{
		Name: "RULE-OLD-1", Product: "AmazonLinux2",
		Classification: "Security", Severity: "Critical", ReleaseDate: oldRelease,
	})
	b.AddAvailablePatchInternal(ssm.Patch{
		Name: "RULE-NEW-1", Product: "AmazonLinux2",
		Classification: "Security", Severity: "Critical", ReleaseDate: recentRelease,
	})

	createResp := doRequest(t, h, "CreatePatchBaseline", `{
		"Name": "rule-approval-baseline",
		"OperatingSystem": "AMAZON_LINUX_2",
		"ApprovalRules": {
			"PatchRules": [{
				"PatchFilterGroup": {"PatchFilters": [{"Key": "PRODUCT", "Values": ["AmazonLinux2"]}]},
				"ApproveAfterDays": 30
			}]
		}
	}`)
	require.Equal(t, http.StatusOK, createResp.Code)

	var created struct {
		BaselineID string `json:"BaselineId"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	regResp := doRequest(t, h, "RegisterPatchBaselineForPatchGroup", `{
		"BaselineId":"`+created.BaselineID+`","PatchGroup":"rule-group"
	}`)
	require.Equal(t, http.StatusOK, regResp.Code)

	sendResp := doRequest(t, h, "SendCommand", `{
		"DocumentName": "AWS-RunPatchBaseline",
		"InstanceIds": ["i-ruletest"],
		"Parameters": {"PatchGroup": ["rule-group"], "Operation": ["Install"]}
	}`)
	require.Equal(t, http.StatusOK, sendResp.Code)

	patchesResp := doRequest(t, h, "DescribeInstancePatches", `{"InstanceId":"i-ruletest"}`)
	require.Equal(t, http.StatusOK, patchesResp.Code)

	var patches struct {
		Patches []struct {
			Title string `json:"Title"`
			State string `json:"State"`
		} `json:"Patches"`
	}
	require.NoError(t, json.Unmarshal(patchesResp.Body.Bytes(), &patches))

	states := map[string]string{}
	for _, p := range patches.Patches {
		states[p.Title] = p.State
	}

	assert.Equal(t, "Installed", states["RULE-OLD-1"],
		"a patch released 400 days ago must be rule-approved and installed by a 30-day ApproveAfterDays rule")
	assert.Equal(t, "Missing", states["RULE-NEW-1"],
		"a patch released yesterday must not yet be rule-approved by a 30-day ApproveAfterDays rule")
}
