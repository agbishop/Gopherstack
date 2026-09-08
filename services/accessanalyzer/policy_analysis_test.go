package accessanalyzer_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/accessanalyzer"
)

// policyAllow returns a single-statement Allow policy granting action on resource.
func policyAllow(action, resource string) string {
	p := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect":   "Allow",
				"Action":   action,
				"Resource": resource,
			},
		},
	}

	b, _ := json.Marshal(p)

	return string(b)
}

// policyEmpty returns a valid empty policy.
func policyEmpty() string {
	return `{"Version":"2012-10-17","Statement":[]}`
}

// policyPublic returns a resource policy allowing public access.
func policyPublic() string {
	p := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect":    "Allow",
				"Principal": "*",
				"Action":    "s3:GetObject",
				"Resource":  "arn:aws:s3:::bucket/*",
			},
		},
	}

	b, _ := json.Marshal(p)

	return string(b)
}

// --- Unit tests for policy analysis functions ---

// TestCheckAccessNotGrantedLogic covers the core CheckAccessNotGranted logic.
func TestCheckAccessNotGrantedLogic(t *testing.T) {
	t.Parallel()

	bucketWildcard := "arn:aws:s3:::bucket/*"
	getObj := "s3:GetObject"

	tests := []struct { //nolint:govet // field order chosen for readability
		name       string
		policy     string
		accesses   []accessanalyzer.AccessSpec
		wantResult string
	}{
		{
			name:       "empty_policy_passes",
			policy:     policyEmpty(),
			accesses:   []accessanalyzer.AccessSpec{{Actions: []string{getObj}, Resources: []string{"*"}}},
			wantResult: "PASS",
		},
		{
			name:       "no_accesses_to_check_passes",
			policy:     policyAllow("s3:*", "*"),
			accesses:   []accessanalyzer.AccessSpec{},
			wantResult: "PASS",
		},
		{
			name:   "explicit_allow_fails",
			policy: policyAllow(getObj, bucketWildcard),
			accesses: []accessanalyzer.AccessSpec{
				{Actions: []string{getObj}, Resources: []string{bucketWildcard}},
			},
			wantResult: "FAIL",
		},
		{
			name:   "wildcard_action_matches_fails",
			policy: policyAllow("s3:*", bucketWildcard),
			accesses: []accessanalyzer.AccessSpec{
				{Actions: []string{getObj}, Resources: []string{"arn:aws:s3:::bucket/key"}},
			},
			wantResult: "FAIL",
		},
		{
			name:   "star_action_matches_any_fails",
			policy: policyAllow("*", "*"),
			accesses: []accessanalyzer.AccessSpec{
				{Actions: []string{"iam:CreateUser"}, Resources: []string{"*"}},
			},
			wantResult: "FAIL",
		},
		{
			name:       "deny_statement_does_not_fail",
			policy:     `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:*","Resource":"*"}]}`,
			accesses:   []accessanalyzer.AccessSpec{{Actions: []string{getObj}, Resources: []string{"*"}}},
			wantResult: "PASS",
		},
		{
			name:       "action_mismatch_passes",
			policy:     policyAllow("s3:PutObject", "*"),
			accesses:   []accessanalyzer.AccessSpec{{Actions: []string{getObj}, Resources: []string{"*"}}},
			wantResult: "PASS",
		},
		{
			name:   "resource_mismatch_passes",
			policy: policyAllow(getObj, "arn:aws:s3:::other-bucket/*"),
			accesses: []accessanalyzer.AccessSpec{
				{Actions: []string{getObj}, Resources: []string{bucketWildcard}},
			},
			wantResult: "PASS",
		},
		{
			// NotAction "s3:PutObject" grants every OTHER action, including
			// s3:GetObject -- gopherstack-xyu4 regression, previously
			// NotAction was ignored and this statement was treated as
			// granting nothing, giving a confident, wrong PASS.
			name: "not_action_grants_other_actions_fails",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","NotAction":"s3:PutObject","Resource":"*"}]}`,
			accesses:   []accessanalyzer.AccessSpec{{Actions: []string{getObj}, Resources: []string{"*"}}},
			wantResult: "FAIL",
		},
		{
			name: "not_action_excluded_action_passes",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","NotAction":"s3:GetObject","Resource":"*"}]}`,
			accesses:   []accessanalyzer.AccessSpec{{Actions: []string{getObj}, Resources: []string{"*"}}},
			wantResult: "PASS",
		},
		{
			// NotResource excludes only "other-bucket"; bucketWildcard is
			// still covered, so this grants access to it -- same NotAction
			// bug, mirrored for NotResource.
			name: "not_resource_grants_other_resources_fails",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Action":"s3:GetObject","NotResource":"arn:aws:s3:::other-bucket/*"}]}`,
			accesses: []accessanalyzer.AccessSpec{
				{Actions: []string{getObj}, Resources: []string{bucketWildcard}},
			},
			wantResult: "FAIL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := accessanalyzer.CheckAccessNotGranted(tt.policy, tt.accesses)
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, res.Result)

			if tt.wantResult == "FAIL" {
				assert.NotEmpty(t, res.Reasons)
			}
		})
	}
}

// TestCheckAccessNotGrantedMalformedPolicy proves a syntactically invalid
// policyDocument is reported as ErrMalformedPolicy rather than silently
// parsed as an empty policy (gopherstack-x9ff: previously parsePolicy
// swallowed the json.Unmarshal error, so garbage input reported PASS
// regardless of the requested access).
func TestCheckAccessNotGrantedMalformedPolicy(t *testing.T) {
	t.Parallel()

	_, err := accessanalyzer.CheckAccessNotGranted("not-json", []accessanalyzer.AccessSpec{
		{Actions: []string{"s3:GetObject"}, Resources: []string{"*"}},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, accessanalyzer.ErrMalformedPolicy)
}

// TestCheckAccessNotGrantedEmptyPolicyNotMalformed proves an empty/absent
// policyDocument still parses as a valid empty policy (PASS), not an error --
// gopherstack-x9ff is about garbage input, not absent input.
func TestCheckAccessNotGrantedEmptyPolicyNotMalformed(t *testing.T) {
	t.Parallel()

	res, err := accessanalyzer.CheckAccessNotGranted("", []accessanalyzer.AccessSpec{
		{Actions: []string{"s3:GetObject"}, Resources: []string{"*"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "PASS", res.Result)
}

// TestCheckNoNewAccessLogic covers CheckNoNewAccess diff logic.
func TestCheckNoNewAccessLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		existing   string
		newPol     string
		wantResult string
	}{
		{
			name:       "identical_empty_passes",
			existing:   policyEmpty(),
			newPol:     policyEmpty(),
			wantResult: "PASS",
		},
		{
			name:       "identical_allow_passes",
			existing:   policyAllow("s3:GetObject", "arn:aws:s3:::bucket/*"),
			newPol:     policyAllow("s3:GetObject", "arn:aws:s3:::bucket/*"),
			wantResult: "PASS",
		},
		{
			name:       "new_action_fails",
			existing:   policyAllow("s3:GetObject", "*"),
			newPol:     policyAllow("s3:PutObject", "*"),
			wantResult: "FAIL",
		},
		{
			name:       "new_resource_fails",
			existing:   policyAllow("s3:GetObject", "arn:aws:s3:::bucket-a/*"),
			newPol:     policyAllow("s3:GetObject", "arn:aws:s3:::bucket-b/*"),
			wantResult: "FAIL",
		},
		{
			name:       "empty_existing_new_has_allow_fails",
			existing:   policyEmpty(),
			newPol:     policyAllow("s3:GetObject", "*"),
			wantResult: "FAIL",
		},
		{
			name:       "new_removes_action_passes_no_new_access",
			existing:   policyAllow("s3:*", "*"),
			newPol:     policyAllow("s3:GetObject", "*"),
			wantResult: "PASS",
		},
		{
			// existing grants every action except PutObject via NotAction;
			// the new policy's explicit GetObject grant is therefore already
			// covered. gopherstack-xyu4 regression: previously NotAction was
			// ignored on the existing-policy side too, so this reported a
			// FAIL (new access) even though existing already granted it.
			name: "existing_not_action_already_covers_new_grant_passes",
			existing: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","NotAction":"s3:PutObject","Resource":"*"}]}`,
			newPol:     policyAllow("s3:GetObject", "*"),
			wantResult: "PASS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := accessanalyzer.CheckNoNewAccess(tt.existing, tt.newPol)
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, res.Result)

			if tt.wantResult == "FAIL" {
				assert.NotEmpty(t, res.Reasons)
			}
		})
	}
}

// TestCheckNoNewAccessMalformedPolicy proves a syntactically invalid
// existingPolicyDocument or newPolicyDocument is reported as
// ErrMalformedPolicy rather than silently parsed as an empty policy
// (gopherstack-x9ff).
func TestCheckNoNewAccessMalformedPolicy(t *testing.T) {
	t.Parallel()

	valid := policyEmpty()

	tests := []struct {
		existing string
		newPol   string
		name     string
	}{
		{name: "malformed_existing", existing: "not-json", newPol: valid},
		{name: "malformed_new", existing: valid, newPol: "not-json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := accessanalyzer.CheckNoNewAccess(tt.existing, tt.newPol)
			require.Error(t, err)
			assert.ErrorIs(t, err, accessanalyzer.ErrMalformedPolicy)
		})
	}
}

// TestCheckNoPublicAccessLogic covers CheckNoPublicAccess principal detection.
func TestCheckNoPublicAccessLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policy     string
		wantResult string
	}{
		{
			name:       "empty_policy_passes",
			policy:     policyEmpty(),
			wantResult: "PASS",
		},
		{
			name:       "wildcard_principal_fails",
			policy:     policyPublic(),
			wantResult: "FAIL",
		},
		{
			name: "aws_star_principal_fails",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":"*"}]}`,
			wantResult: "FAIL",
		},
		{
			name: "specific_account_passes",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::123456789012:root"},` +
				`"Action":"s3:GetObject","Resource":"*"}]}`,
			wantResult: "PASS",
		},
		{
			name: "service_principal_passes",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},` +
				`"Action":"s3:GetObject","Resource":"*"}]}`,
			wantResult: "PASS",
		},
		{
			name: "deny_with_wildcard_principal_passes",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Deny","Principal":"*","Action":"s3:*","Resource":"*"}]}`,
			wantResult: "PASS",
		},
		{
			name:       "no_principal_field_passes",
			policy:     policyAllow("s3:GetObject", "*"),
			wantResult: "PASS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res, err := accessanalyzer.CheckNoPublicAccess(tt.policy)
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, res.Result)

			if tt.wantResult == "FAIL" {
				assert.NotEmpty(t, res.Reasons)
			}
		})
	}
}

// TestCheckNoPublicAccessMalformedPolicy proves a syntactically invalid
// policyDocument is reported as ErrMalformedPolicy rather than silently
// parsed as an empty (therefore never-public) policy (gopherstack-x9ff).
func TestCheckNoPublicAccessMalformedPolicy(t *testing.T) {
	t.Parallel()

	_, err := accessanalyzer.CheckNoPublicAccess("not-json")
	require.Error(t, err)
	assert.ErrorIs(t, err, accessanalyzer.ErrMalformedPolicy)
}

// TestValidatePolicyLogic covers ValidatePolicy structural validation.
func TestValidatePolicyLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		policy       string
		policyType   string
		wantFindings []string
	}{
		{
			name:         "valid_empty_identity_policy",
			policy:       policyEmpty(),
			policyType:   "IDENTITY_POLICY",
			wantFindings: nil,
		},
		{
			name:         "invalid_json_returns_error",
			policy:       "not-json",
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"INVALID_POLICY_SYNTAX"},
		},
		{
			name:         "missing_version_suggestion",
			policy:       `{"Statement":[]}`,
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"MISSING_VERSION"},
		},
		{
			name:         "invalid_version_error",
			policy:       `{"Version":"1999-01-01","Statement":[]}`,
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"INVALID_VERSION"},
		},
		{
			name: "invalid_effect",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Permit","Action":"s3:Get*","Resource":"*"}]}`,
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"INVALID_EFFECT"},
		},
		{
			name:         "missing_action",
			policy:       `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Resource":"*"}]}`,
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"MISSING_ACTION_OR_NOT_ACTION"},
		},
		{
			name: "both_action_and_not_action",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Action":"s3:Get*","NotAction":"s3:Put*","Resource":"*"}]}`,
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"BOTH_ACTION_AND_NOT_ACTION"},
		},
		{
			name: "missing_resource_in_identity_policy",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Action":"s3:GetObject"}]}`,
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"MISSING_RESOURCE_OR_NOT_RESOURCE"},
		},
		{
			name: "missing_resource_not_required_for_resource_policy",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Action":"s3:GetObject"}]}`,
			policyType:   "RESOURCE_POLICY",
			wantFindings: nil,
		},
		{
			name: "star_action_and_resource_security_warning",
			policy: `{"Version":"2012-10-17","Statement":[` +
				`{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
			policyType:   "IDENTITY_POLICY",
			wantFindings: []string{"PASS_ROLE_WITH_STAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			findings := accessanalyzer.ValidatePolicy(tt.policy, tt.policyType)

			codes := make([]string, 0, len(findings))
			for _, f := range findings {
				codes = append(codes, f.IssueCode)
			}

			for _, want := range tt.wantFindings {
				assert.Contains(t, codes, want)
			}
		})
	}
}

// --- HTTP handler integration tests for Check* and ValidatePolicy ---

// TestCheckAccessNotGrantedHTTP tests the HTTP handler with real policy inputs.
func TestCheckAccessNotGrantedHTTP(t *testing.T) {
	t.Parallel()

	getObj := "s3:GetObject"

	tests := []struct {
		name       string
		body       map[string]any
		wantResult string
		wantStatus int
	}{
		{
			name: "pass_empty_policy",
			body: map[string]any{
				"policyDocument": policyEmpty(),
				"policyType":     "IDENTITY_POLICY",
				"access": []any{
					map[string]any{"actions": []string{getObj}, "resources": []string{"*"}},
				},
			},
			wantStatus: http.StatusOK,
			wantResult: "PASS",
		},
		{
			name: "fail_when_action_granted",
			body: map[string]any{
				"policyDocument": policyAllow(getObj, "arn:aws:s3:::bucket/*"),
				"policyType":     "IDENTITY_POLICY",
				"access": []any{
					map[string]any{
						"actions":   []string{getObj},
						"resources": []string{"arn:aws:s3:::bucket/*"},
					},
				},
			},
			wantStatus: http.StatusOK,
			wantResult: "FAIL",
		},
		{
			name: "pass_deny_only",
			body: map[string]any{
				"policyDocument": `{"Version":"2012-10-17","Statement":[` +
					`{"Effect":"Deny","Action":"s3:*","Resource":"*"}]}`,
				"policyType": "IDENTITY_POLICY",
				"access": []any{
					map[string]any{"actions": []string{getObj}, "resources": []string{"*"}},
				},
			},
			wantStatus: http.StatusOK,
			wantResult: "PASS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/policy/check-access-not-granted", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantResult, resp["result"])
		})
	}
}

// TestCheckNoNewAccessHTTP tests the HTTP handler for CheckNoNewAccess.
func TestCheckNoNewAccessHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       map[string]any
		wantResult string
		wantStatus int
	}{
		{
			name: "pass_identical_empty",
			body: map[string]any{
				"existingPolicyDocument": policyEmpty(),
				"newPolicyDocument":      policyEmpty(),
				"policyType":             "IDENTITY_POLICY",
			},
			wantStatus: http.StatusOK,
			wantResult: "PASS",
		},
		{
			name: "fail_new_action_added",
			body: map[string]any{
				"existingPolicyDocument": policyEmpty(),
				"newPolicyDocument":      policyAllow("s3:GetObject", "*"),
				"policyType":             "IDENTITY_POLICY",
			},
			wantStatus: http.StatusOK,
			wantResult: "FAIL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/policy/check-no-new-access", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantResult, resp["result"])
		})
	}
}

// TestCheckNoPublicAccessHTTP tests the HTTP handler for CheckNoPublicAccess.
func TestCheckNoPublicAccessHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       map[string]any
		wantResult string
		wantStatus int
	}{
		{
			name: "pass_empty_policy",
			body: map[string]any{
				"policyDocument": policyEmpty(),
				"resourceType":   "AWS::S3::Bucket",
			},
			wantStatus: http.StatusOK,
			wantResult: "PASS",
		},
		{
			name: "fail_wildcard_principal",
			body: map[string]any{
				"policyDocument": policyPublic(),
				"resourceType":   "AWS::S3::Bucket",
			},
			wantStatus: http.StatusOK,
			wantResult: "FAIL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/policy/check-no-public-access", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantResult, resp["result"])
			assert.NotNil(t, resp["reasons"])
		})
	}
}

// TestValidatePolicyHTTP tests the HTTP handler for ValidatePolicy.
func TestValidatePolicyHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // field order chosen for readability
		name            string
		body            map[string]any
		wantStatus      int
		wantHasFindings bool
	}{
		{
			name: "valid_policy_no_findings",
			body: map[string]any{
				"policyDocument": policyEmpty(),
				"policyType":     "IDENTITY_POLICY",
			},
			wantStatus:      http.StatusOK,
			wantHasFindings: false,
		},
		{
			name: "invalid_json_has_findings",
			body: map[string]any{
				"policyDocument": "not-valid-json",
				"policyType":     "IDENTITY_POLICY",
			},
			wantStatus:      http.StatusOK,
			wantHasFindings: true,
		},
		{
			name: "star_action_star_resource_has_findings",
			body: map[string]any{
				"policyDocument": `{"Version":"2012-10-17","Statement":[` +
					`{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
				"policyType": "IDENTITY_POLICY",
			},
			wantStatus:      http.StatusOK,
			wantHasFindings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/policy/validation", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotNil(t, resp["findings"])

			findings := resp["findings"].([]any)

			if tt.wantHasFindings {
				assert.NotEmpty(t, findings)
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}
