package dlm_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dlm"
)

// ---------------------------------------------------------------------------
// CreateLifecyclePolicy: required-field validation
// ---------------------------------------------------------------------------

// TestBackend_CreateLifecyclePolicy_RequiredFields verifies that
// Description and ExecutionRoleArn -- both "This member is required" on the
// real CreateLifecyclePolicyInput shape -- are rejected with
// InvalidRequestException when missing, instead of silently creating a
// policy no real AWS account could ever produce.
func TestBackend_CreateLifecyclePolicy_RequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
		roleARN     string
		wantErr     bool
	}{
		{
			name:        "missing description is rejected",
			description: "",
			roleARN:     "arn:aws:iam::000000000000:role/r",
			wantErr:     true,
		},
		{
			name:        "missing executionRoleArn is rejected",
			description: "desc",
			roleARN:     "",
			wantErr:     true,
		},
		{
			name:        "both present succeeds",
			description: "desc",
			roleARN:     "arn:aws:iam::000000000000:role/r",
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := dlm.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateLifecyclePolicy(tc.description, tc.roleARN, "ENABLED", nil, nil)

			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, dlm.ErrInvalidRequest)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBackend_CreateLifecyclePolicy_DefaultState verifies that an empty
// State defaults to ENABLED, matching this backend's documented leniency.
func TestBackend_CreateLifecyclePolicy_DefaultState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		state     string
		wantState string
	}{
		{name: "empty state defaults to ENABLED", state: "", wantState: "ENABLED"},
		{name: "explicit DISABLED preserved", state: "DISABLED", wantState: "DISABLED"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := dlm.NewInMemoryBackend("000000000000", "us-east-1")
			p, err := b.CreateLifecyclePolicy("desc", "role", tc.state, nil, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.wantState, p.State)
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateLifecyclePolicy: partial update / nil PolicyDetails
// ---------------------------------------------------------------------------

// TestBackend_UpdateLifecyclePolicy_PartialUpdate verifies partial update
// (only some fields set) leaves the other fields untouched.
func TestBackend_UpdateLifecyclePolicy_PartialUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		updateDesc  string
		updateRole  string
		updateState string
		wantDesc    string
		wantRole    string
		wantState   string
	}{
		{
			name:       "update only description",
			updateDesc: "new description",
			wantDesc:   "new description",
			wantRole:   "arn:aws:iam::000000000000:role/original",
			wantState:  "ENABLED",
		},
		{
			name:        "update only state",
			updateState: "DISABLED",
			wantDesc:    "original description",
			wantRole:    "arn:aws:iam::000000000000:role/original",
			wantState:   "DISABLED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := dlm.NewInMemoryBackend("000000000000", "us-east-1")
			p, err := b.CreateLifecyclePolicy(
				"original description",
				"arn:aws:iam::000000000000:role/original",
				"ENABLED",
				nil,
				nil,
			)
			require.NoError(t, err)

			// An empty test-table string means "field omitted from the
			// request" (nil pointer), matching how handler.go decodes a
			// missing JSON key -- not "explicitly cleared".
			var updateDesc, updateRole *string
			if tc.updateDesc != "" {
				updateDesc = new(tc.updateDesc)
			}

			if tc.updateRole != "" {
				updateRole = new(tc.updateRole)
			}

			err = b.UpdateLifecyclePolicy(p.PolicyID, updateDesc, updateRole, tc.updateState, nil, nil)
			require.NoError(t, err)

			got, err := b.GetLifecyclePolicy(p.PolicyID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDesc, got.Description)
			assert.Equal(t, tc.wantRole, got.ExecutionRoleARN)
			assert.Equal(t, tc.wantState, got.State)
		})
	}
}

// TestBackend_UpdateLifecyclePolicy_ExplicitEmptyClearsField verifies the
// *string presence semantics on description/executionRoleARN: the real
// UpdateLifecyclePolicyInput carries both as *string, and its serializer
// only omits the wire JSON key when the pointer itself is nil (see
// awsRestjson1_serializeOpDocumentUpdateLifecyclePolicyInput in the vendored
// SDK) -- an explicit empty string is a real, distinct wire value, not a
// synonym for "omitted".
func TestBackend_UpdateLifecyclePolicy_ExplicitEmptyClearsField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		description *string
		name        string
		wantDesc    string
	}{
		{name: "nil (omitted) leaves description unchanged", description: nil, wantDesc: "original"},
		{name: "pointer to empty string clears description", description: new(""), wantDesc: ""},
		{name: "pointer to new value sets description", description: new("new"), wantDesc: "new"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := dlm.NewInMemoryBackend("000000000000", "us-east-1")
			p, err := b.CreateLifecyclePolicy("original", "arn:aws:iam::000000000000:role/r", "ENABLED", nil, nil)
			require.NoError(t, err)

			require.NoError(t, b.UpdateLifecyclePolicy(p.PolicyID, tc.description, nil, "", nil, nil))

			got, err := b.GetLifecyclePolicy(p.PolicyID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDesc, got.Description)
		})
	}
}

// TestBackend_UpdateLifecyclePolicy_NilDetailsPreserved verifies that a nil
// PolicyDetails on update does not clear the previously stored details.
func TestBackend_UpdateLifecyclePolicy_NilDetailsPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "nil_details_does_not_clear"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := dlm.NewInMemoryBackend("000000000000", "us-east-1")

			details := map[string]any{"PolicyType": "EBS_SNAPSHOT_MANAGEMENT"}
			p, err := b.CreateLifecyclePolicy("t", "arn:aws:iam::000000000000:role/r", "ENABLED", nil, details)
			require.NoError(t, err)

			// Update with nil PolicyDetails — must not clear existing details.
			require.NoError(t, b.UpdateLifecyclePolicy(p.PolicyID, new("new desc"), nil, "", nil, nil))

			got, err := b.GetLifecyclePolicy(p.PolicyID)
			require.NoError(t, err)
			assert.Equal(t, "new desc", got.Description)
			assert.Equal(t, "EBS_SNAPSHOT_MANAGEMENT", got.PolicyDetails["PolicyType"])
		})
	}
}

// ---------------------------------------------------------------------------
// GetLifecyclePolicies: PolicyType summary field
// ---------------------------------------------------------------------------

// TestBackend_GetLifecyclePolicies_PolicyType verifies the LifecyclePolicySummary
// wire shape's PolicyType field (present on every element of the real
// GetLifecyclePolicies response, previously missing from this backend's
// PolicySummary), including the EBS_SNAPSHOT_MANAGEMENT default AWS applies
// when PolicyDetails omits PolicyType.
func TestBackend_GetLifecyclePolicies_PolicyType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		policyDetails map[string]any
		name          string
		wantType      string
	}{
		{
			name:          "explicit PolicyType is echoed",
			policyDetails: map[string]any{"PolicyType": "IMAGE_MANAGEMENT"},
			wantType:      "IMAGE_MANAGEMENT",
		},
		{
			name:          "nil PolicyDetails defaults to EBS_SNAPSHOT_MANAGEMENT",
			policyDetails: nil,
			wantType:      "EBS_SNAPSHOT_MANAGEMENT",
		},
		{
			name:          "PolicyDetails without PolicyType defaults to EBS_SNAPSHOT_MANAGEMENT",
			policyDetails: map[string]any{"ResourceTypes": []any{"VOLUME"}},
			wantType:      "EBS_SNAPSHOT_MANAGEMENT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := dlm.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateLifecyclePolicy(
				"d", "arn:aws:iam::000000000000:role/r", "ENABLED", nil, tc.policyDetails,
			)
			require.NoError(t, err)

			summaries, err := b.GetLifecyclePolicies(dlm.PolicyFilter{})
			require.NoError(t, err)
			require.Len(t, summaries, 1)
			assert.Equal(t, tc.wantType, summaries[0].PolicyType)
		})
	}
}

// ---------------------------------------------------------------------------
// GetLifecyclePolicies: PolicyIDs / ResourceTypes / TargetTags / TagsToAdd
// filters
// ---------------------------------------------------------------------------

// TestBackend_GetLifecyclePolicies_MultipleIDFilter verifies the PolicyIDs
// filter narrows results to just the requested IDs.
func TestBackend_GetLifecyclePolicies_MultipleIDFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		createN   int
		filterN   int
		wantCount int
	}{
		{
			name:      "filter 2 of 3 policies by ID",
			createN:   3,
			filterN:   2,
			wantCount: 2,
		},
		{
			name:      "empty filter returns all",
			createN:   2,
			filterN:   0,
			wantCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := dlm.NewInMemoryBackend("000000000000", "us-east-1")
			var ids []string
			for i := range tc.createN {
				p, err := b.CreateLifecyclePolicy(fmt.Sprintf("desc-%d", i), "role", "ENABLED", nil, nil)
				require.NoError(t, err)
				ids = append(ids, p.PolicyID)
			}

			var filter []string
			if tc.filterN > 0 {
				filter = ids[:tc.filterN]
			}

			result, err := b.GetLifecyclePolicies(dlm.PolicyFilter{PolicyIDs: filter})
			require.NoError(t, err)
			assert.Len(t, result, tc.wantCount)
		})
	}
}

// TestBackend_GetLifecyclePolicies_ResourceTypesFilter verifies the
// resourceTypes query filter, previously accepted on the wire but silently
// ignored by the backend (a disguised no-op).
func TestBackend_GetLifecyclePolicies_ResourceTypesFilter(t *testing.T) {
	t.Parallel()

	b := dlm.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateLifecyclePolicy(
		"volume-policy", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{"ResourceTypes": []any{"VOLUME"}},
	)
	require.NoError(t, err)

	_, err = b.CreateLifecyclePolicy(
		"instance-policy", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{"ResourceTypes": []any{"INSTANCE"}},
	)
	require.NoError(t, err)

	tests := []struct {
		name          string
		wantDesc      string
		resourceTypes []string
		wantCount     int
	}{
		{name: "no filter returns all", resourceTypes: nil, wantCount: 2},
		{name: "VOLUME filter returns one", resourceTypes: []string{"VOLUME"}, wantCount: 1, wantDesc: "volume-policy"},
		{
			name:          "case-insensitive match",
			resourceTypes: []string{"volume"},
			wantCount:     1,
			wantDesc:      "volume-policy",
		},
		{
			name:          "unmatched resource type returns none",
			resourceTypes: []string{"BOGUS"},
			wantCount:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, listErr := b.GetLifecyclePolicies(dlm.PolicyFilter{ResourceTypes: tc.resourceTypes})
			require.NoError(t, listErr)
			require.Len(t, result, tc.wantCount)

			if tc.wantDesc != "" {
				assert.Equal(t, tc.wantDesc, result[0].Description)
			}
		})
	}
}

// TestBackend_GetLifecyclePolicies_DefaultPolicyTypeFilter verifies the
// defaultPolicyType filter (VOLUME/INSTANCE/ALL) narrows results to default
// policies only, matching GetLifecyclePoliciesInput.DefaultPolicyType
// (aws-sdk-go-v2/service/dlm@v1.39.4/api_op_GetLifecyclePolicies.go:32-40).
func TestBackend_GetLifecyclePolicies_DefaultPolicyTypeFilter(t *testing.T) {
	t.Parallel()

	b := dlm.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateLifecyclePolicy(
		"custom", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{"ResourceTypes": []any{"VOLUME"}},
	)
	require.NoError(t, err)

	_, err = b.CreateLifecyclePolicy(
		"volume-default", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{"ResourceType": "VOLUME", "PolicyLanguage": "SIMPLIFIED"},
	)
	require.NoError(t, err)

	_, err = b.CreateLifecyclePolicy(
		"instance-default", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{"ResourceType": "INSTANCE", "PolicyLanguage": "SIMPLIFIED"},
	)
	require.NoError(t, err)

	tests := []struct {
		name              string
		defaultPolicyType string
		wantDescs         []string
	}{
		{
			name: "no filter returns all", defaultPolicyType: "",
			wantDescs: []string{"custom", "volume-default", "instance-default"},
		},
		{
			name: "VOLUME returns only volume default", defaultPolicyType: "VOLUME",
			wantDescs: []string{"volume-default"},
		},
		{
			name: "INSTANCE returns only instance default", defaultPolicyType: "INSTANCE",
			wantDescs: []string{"instance-default"},
		},
		{
			name: "ALL returns every default policy", defaultPolicyType: "ALL",
			wantDescs: []string{"volume-default", "instance-default"},
		},
		{name: "case-insensitive match", defaultPolicyType: "volume", wantDescs: []string{"volume-default"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, listErr := b.GetLifecyclePolicies(dlm.PolicyFilter{DefaultPolicyType: tc.defaultPolicyType})
			require.NoError(t, listErr)
			require.Len(t, result, len(tc.wantDescs))

			got := make([]string, 0, len(result))
			for _, p := range result {
				got = append(got, p.Description)
			}
			assert.ElementsMatch(t, tc.wantDescs, got)
		})
	}
}

// TestBackend_GetLifecyclePolicies_TargetTagsFilter verifies the targetTags
// query filter against PolicyDetails.TargetTags ({Key,Value} pairs).
func TestBackend_GetLifecyclePolicies_TargetTagsFilter(t *testing.T) {
	t.Parallel()

	b := dlm.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateLifecyclePolicy(
		"prod-policy", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{
			"TargetTags": []any{map[string]any{"Key": "env", "Value": "prod"}},
		},
	)
	require.NoError(t, err)

	_, err = b.CreateLifecyclePolicy(
		"dev-policy", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{
			"TargetTags": []any{map[string]any{"Key": "env", "Value": "dev"}},
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name       string
		wantDesc   string
		targetTags []string
		wantCount  int
	}{
		{name: "no filter returns all", targetTags: nil, wantCount: 2},
		{
			name:       "matching key=value returns one",
			targetTags: []string{"env=prod"},
			wantCount:  1,
			wantDesc:   "prod-policy",
		},
		{name: "non-matching value returns none", targetTags: []string{"env=staging"}, wantCount: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, listErr := b.GetLifecyclePolicies(dlm.PolicyFilter{TargetTags: tc.targetTags})
			require.NoError(t, listErr)
			require.Len(t, result, tc.wantCount)

			if tc.wantDesc != "" {
				assert.Equal(t, tc.wantDesc, result[0].Description)
			}
		})
	}
}

// TestBackend_GetLifecyclePolicies_TagsToAddFilter verifies the tagsToAdd
// query filter, which matches against the TagsToAdd of any schedule nested
// under PolicyDetails.Schedules.
func TestBackend_GetLifecyclePolicies_TagsToAddFilter(t *testing.T) {
	t.Parallel()

	b := dlm.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateLifecyclePolicy(
		"tagged-policy", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{
			"Schedules": []any{
				map[string]any{
					"Name":      "daily",
					"TagsToAdd": []any{map[string]any{"Key": "team", "Value": "infra"}},
				},
			},
		},
	)
	require.NoError(t, err)

	_, err = b.CreateLifecyclePolicy(
		"untagged-policy", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{
			"Schedules": []any{map[string]any{"Name": "daily"}},
		},
	)
	require.NoError(t, err)

	result, err := b.GetLifecyclePolicies(dlm.PolicyFilter{TagsToAdd: []string{"team=infra"}})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "tagged-policy", result[0].Description)

	result, err = b.GetLifecyclePolicies(dlm.PolicyFilter{TagsToAdd: []string{"team=nomatch"}})
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestBackend_GetLifecyclePolicies_CombinedFilters verifies that the
// filter dimensions (PolicyIDs, State, ResourceTypes, TargetTags, TagsToAdd)
// are ANDed together, not just applied independently.
func TestBackend_GetLifecyclePolicies_CombinedFilters(t *testing.T) {
	t.Parallel()

	b := dlm.NewInMemoryBackend("000000000000", "us-east-1")

	match, err := b.CreateLifecyclePolicy(
		"match", "arn:aws:iam::000000000000:role/r", "ENABLED", nil,
		map[string]any{
			"ResourceTypes": []any{"VOLUME"},
			"TargetTags":    []any{map[string]any{"Key": "env", "Value": "prod"}},
		},
	)
	require.NoError(t, err)

	_, err = b.CreateLifecyclePolicy(
		"wrong-state", "arn:aws:iam::000000000000:role/r", "DISABLED", nil,
		map[string]any{
			"ResourceTypes": []any{"VOLUME"},
			"TargetTags":    []any{map[string]any{"Key": "env", "Value": "prod"}},
		},
	)
	require.NoError(t, err)

	result, err := b.GetLifecyclePolicies(dlm.PolicyFilter{
		State:         "ENABLED",
		ResourceTypes: []string{"VOLUME"},
		TargetTags:    []string{"env=prod"},
	})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, match.PolicyID, result[0].PolicyID)
}
