package organizations_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestBackend_TagOperations tests tagging resources.
func TestBackend_TagOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name: "tag_and_untag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			org, _, err := b.CreateOrganization("ALL")
			require.NoError(t, err)

			// TagResource.
			tags := []organizations.Tag{{Key: "env", Value: "test"}}
			err = b.TagResource(org.ID, tags)
			require.NoError(t, err)

			// ListTagsForResource.
			listed, err := b.ListTagsForResource(org.ID)
			require.NoError(t, err)
			assert.Len(t, listed, 1)
			assert.Equal(t, "env", listed[0].Key)
			assert.Equal(t, "test", listed[0].Value)

			// UntagResource.
			err = b.UntagResource(org.ID, []string{"env"})
			require.NoError(t, err)

			// After untag, tags should be empty.
			listed, err = b.ListTagsForResource(org.ID)
			require.NoError(t, err)
			assert.Empty(t, listed)
		})
	}
}

// TestTagOperations_MultiResource tests tagging multiple resource types.
func TestTagOperations_MultiResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceKind string // "account", "ou", "policy"
	}{
		{name: "tag_account", resourceKind: "account"},
		{name: "tag_ou", resourceKind: "ou"},
		{name: "tag_policy", resourceKind: "policy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			var resourceID string

			switch tt.resourceKind {
			case "account":
				s, err := b.CreateAccount("tagged", "tagged@example.com", "", "", nil)
				require.NoError(t, err)
				resourceID = s.AccountID
			case "ou":
				ou, err := b.CreateOrganizationalUnit(rootID, "tagged-ou", nil)
				require.NoError(t, err)
				resourceID = ou.ID
			case "policy":
				p, err := b.CreatePolicy("tagged-p", "", `{}`, "SERVICE_CONTROL_POLICY", nil)
				require.NoError(t, err)
				resourceID = p.PolicySummary.ID
			}

			tags := []organizations.Tag{
				{Key: "team", Value: "platform"},
				{Key: "env", Value: "staging"},
			}

			require.NoError(t, b.TagResource(resourceID, tags))

			listed, err := b.ListTagsForResource(resourceID)
			require.NoError(t, err)
			assert.Len(t, listed, 2)

			require.NoError(t, b.UntagResource(resourceID, []string{"team"}))

			listed, err = b.ListTagsForResource(resourceID)
			require.NoError(t, err)
			assert.Len(t, listed, 1)
			assert.Equal(t, "env", listed[0].Key)
		})
	}
}

// TestTagResource_ExistenceValidation verifies that TagResource/UntagResource/
// ListTagsForResource validate that the resource exists.
func TestTagResource_ExistenceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		resourceFn func(b *organizations.InMemoryBackend, rootID string) string
		name       string
		wantErr    bool
	}{
		{
			name: "root_id_valid",
			resourceFn: func(_ *organizations.InMemoryBackend, rootID string) string {
				return rootID
			},
			wantErr: false,
		},
		{
			name: "account_id_valid",
			resourceFn: func(b *organizations.InMemoryBackend, _ string) string {
				s, err := b.CreateAccount("tag-acct", "tag@example.com", "", "", nil)
				if err != nil {
					panic(err)
				}

				return s.AccountID
			},
			wantErr: false,
		},
		{
			name: "ou_id_valid",
			resourceFn: func(b *organizations.InMemoryBackend, rootID string) string {
				ou, err := b.CreateOrganizationalUnit(rootID, "TagOU", nil)
				if err != nil {
					panic(err)
				}

				return ou.ID
			},
			wantErr: false,
		},
		{
			name: "policy_id_valid",
			resourceFn: func(b *organizations.InMemoryBackend, _ string) string {
				p, err := b.CreatePolicy("tag-pol", "", `{}`, "TAG_POLICY", nil)
				if err != nil {
					panic(err)
				}

				return p.PolicySummary.ID
			},
			wantErr: false,
		},
		{
			name: "unknown_id_rejected",
			resourceFn: func(_ *organizations.InMemoryBackend, _ string) string {
				return "ou-nonexistent-00000000"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)
			resourceID := tt.resourceFn(b, rootID)

			// Test TagResource.
			err := b.TagResource(resourceID, []organizations.Tag{{Key: "k", Value: "v"}})
			if tt.wantErr {
				require.Error(t, err, "TagResource should fail for unknown resource")
			} else {
				require.NoError(t, err, "TagResource should succeed")
			}

			// Test UntagResource.
			err = b.UntagResource(resourceID, []string{"k"})
			if tt.wantErr {
				require.Error(t, err, "UntagResource should fail for unknown resource")
			} else {
				require.NoError(t, err, "UntagResource should succeed")
			}

			// Test ListTagsForResource.
			_, err = b.ListTagsForResource(resourceID)
			if tt.wantErr {
				require.Error(t, err, "ListTagsForResource should fail for unknown resource")
			} else {
				require.NoError(t, err, "ListTagsForResource should succeed")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Item 13: RegisterDelegatedAdministrator service access check
// ---------------------------------------------------------------------------

// TestTagResource_InvalidResource_ViaHandler tests tag resource validation via handler.
func TestTagResource_InvalidResource_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})
	require.Equal(t, http.StatusOK, rec.Code)

	// Tag a non-existent resource.
	rec = doRequest(t, h, "TagResource", map[string]any{
		"ResourceId": "ou-nonexistent-12345678",
		"Tags":       []map[string]string{{"Key": "k", "Value": "v"}},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestTagOps_NonExistentResource_TargetNotFoundException verifies that tag
// operations on a resource ID that does not exist return TargetNotFoundException (not
// InvalidInputException).  Real AWS raises TargetNotFoundException for unknown resource
// IDs in all three tag operations.
func TestTagOps_NonExistentResource_TargetNotFoundException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(b *organizations.InMemoryBackend) error
		name string
	}{
		{
			name: "TagResource",
			fn: func(b *organizations.InMemoryBackend) error {
				return b.TagResource("ou-xxxx-nonexistent", []organizations.Tag{{Key: "k", Value: "v"}})
			},
		},
		{
			name: "UntagResource",
			fn: func(b *organizations.InMemoryBackend) error {
				return b.UntagResource("ou-xxxx-nonexistent", []string{"k"})
			},
		},
		{
			name: "ListTagsForResource",
			fn: func(b *organizations.InMemoryBackend) error {
				_, err := b.ListTagsForResource("ou-xxxx-nonexistent")

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			err := tt.fn(b)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "TargetNotFoundException",
				"%s on unknown resource must return TargetNotFoundException, got: %v", tt.name, err)
		})
	}
}

// TestTagOps_NonExistentResource_ViaHandler verifies the HTTP response includes
// TargetNotFoundException (not InvalidInputException) when tagging a non-existent resource.
func TestTagOps_NonExistentResource_ViaHandler(t *testing.T) {
	t.Parallel()

	const bogusID = "ou-xxxx-nonexistent1"

	tests := []struct {
		op   string
		body map[string]any
		name string
	}{
		{
			name: "TagResource",
			op:   "TagResource",
			body: map[string]any{
				"ResourceId": bogusID,
				"Tags":       []map[string]string{{"Key": "k", "Value": "v"}},
			},
		},
		{
			name: "UntagResource",
			op:   "UntagResource",
			body: map[string]any{
				"ResourceId": bogusID,
				"TagKeys":    []string{"k"},
			},
		},
		{
			name: "ListTagsForResource",
			op:   "ListTagsForResource",
			body: map[string]any{"ResourceId": bogusID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

			rec := doRequest(t, h, tt.op, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp map[string]string
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
			assert.Equal(t, "TargetNotFoundException", errResp["__type"],
				"%s on unknown resource must return TargetNotFoundException", tt.name)
		})
	}
}

// TestHandler_TagOperations tests tag CRUD operations via handler.
func TestHandler_TagOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "tag_untag_list",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			// Get org ID via DescribeOrganization.
			rec := doRequest(t, h, "DescribeOrganization", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)

			var descResp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&descResp))
			org := descResp["Organization"].(map[string]any)
			orgID := org["Id"].(string)

			// TagResource.
			rec = doRequest(t, h, "TagResource", map[string]any{
				"ResourceId": orgID,
				"Tags":       []map[string]string{{"Key": "env", "Value": "test"}},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			// ListTagsForResource.
			rec = doRequest(t, h, "ListTagsForResource", map[string]any{"ResourceId": orgID})
			assert.Equal(t, tt.wantStatus, rec.Code)

			var tagsResp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&tagsResp))
			tags, ok := tagsResp["Tags"].([]any)
			require.True(t, ok)
			assert.NotEmpty(t, tags)

			// UntagResource.
			rec = doRequest(t, h, "UntagResource", map[string]any{
				"ResourceId": orgID,
				"TagKeys":    []string{"env"},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestTagResource_ReservedPrefixRejected verifies that tag keys starting with
// the reserved "aws:" prefix (case-insensitive) are rejected with
// InvalidInputException(INVALID_SYSTEM_TAGS_PARAMETER), matching real AWS.
func TestTagResource_ReservedPrefixRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "lowercase", key: "aws:cloudformation:stack-name"},
		{name: "uppercase", key: "AWS:reserved"},
		{name: "mixed_case", key: "Aws:Reserved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			err := b.TagResource(rootID, []organizations.Tag{{Key: tt.key, Value: "v"}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "INVALID_SYSTEM_TAGS_PARAMETER")
		})
	}
}

// TestTagResource_DuplicateKeyRejected verifies that a single TagResource call
// with the same key twice is rejected with InvalidInputException(DUPLICATE_TAG_KEY).
func TestTagResource_DuplicateKeyRejected(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	err := b.TagResource(rootID, []organizations.Tag{
		{Key: "env", Value: "prod"},
		{Key: "env", Value: "staging"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DUPLICATE_TAG_KEY")
}

// TestTagResource_KeyLengthLimit verifies AWS's TagKey shape bounds (min 1,
// max 128 characters) are enforced, and leave the resource untagged on reject.
func TestTagResource_KeyLengthLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "empty_key_rejected", key: "", wantErr: true},
		{name: "129_chars_rejected", key: strings.Repeat("k", 129), wantErr: true},
		{name: "128_chars_accepted", key: strings.Repeat("k", 128)},
		{name: "1_char_accepted", key: "k"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			err := b.TagResource(rootID, []organizations.Tag{{Key: tt.key, Value: "v"}})

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "InvalidInputException")

				listed, listErr := b.ListTagsForResource(rootID)
				require.NoError(t, listErr)
				assert.Empty(t, listed, "rejected tag must not be applied")

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestTagResource_ValueLengthLimit verifies AWS's TagValue shape bound (max
// 256 characters; min 0, so empty values are valid) is enforced.
func TestTagResource_ValueLengthLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "257_chars_rejected", value: strings.Repeat("v", 257), wantErr: true},
		{name: "256_chars_accepted", value: strings.Repeat("v", 256)},
		{name: "empty_value_accepted", value: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			err := b.TagResource(rootID, []organizations.Tag{{Key: "k", Value: tt.value}})

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "InvalidInputException")

				listed, listErr := b.ListTagsForResource(rootID)
				require.NoError(t, listErr)
				assert.Empty(t, listed, "rejected tag must not be applied")

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestTagResource_MaxTagLimitExceeded verifies the 50-tags-per-resource cap.
func TestTagResource_MaxTagLimitExceeded(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	over := make([]organizations.Tag, 51)
	for i := range over {
		over[i] = organizations.Tag{Key: "k" + string(rune('a'+i%26)) + string(rune('0'+i/26)), Value: "v"}
	}

	err := b.TagResource(rootID, over)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAX_TAG_LIMIT_EXCEEDED")

	// Exactly 50 in one call succeeds.
	err = b.TagResource(rootID, over[:50])
	require.NoError(t, err)

	listed, err := b.ListTagsForResource(rootID)
	require.NoError(t, err)
	assert.Len(t, listed, 50)
}

// TestTagResource_MaxTagLimitExceeded_AcrossCalls verifies that the 50-tag cap
// is enforced against the merged (existing + new) tag set, not just the tags
// in a single call.
func TestTagResource_MaxTagLimitExceeded_AcrossCalls(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	first := make([]organizations.Tag, 49)
	for i := range first {
		first[i] = organizations.Tag{Key: "k" + string(rune('a'+i%26)) + string(rune('0'+i/26)), Value: "v"}
	}

	require.NoError(t, b.TagResource(rootID, first))

	// Adding 2 more distinct keys pushes the total to 51 -- must be rejected,
	// and neither new tag should have been applied.
	err := b.TagResource(rootID, []organizations.Tag{
		{Key: "extra1", Value: "v"},
		{Key: "extra2", Value: "v"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAX_TAG_LIMIT_EXCEEDED")

	listed, err := b.ListTagsForResource(rootID)
	require.NoError(t, err)
	assert.Len(t, listed, 49, "rejected TagResource call must not partially apply tags")
}

// TestCreateAccount_ReservedTagPrefixRejected verifies CreateAccount validates
// its Tags parameter before creating the account -- an invalid tag list must
// leave no account behind, matching AWS's atomic "the entire request fails" note.
func TestCreateAccount_ReservedTagPrefixRejected(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	before, err := b.ListAccounts()
	require.NoError(t, err)

	_, err = b.CreateAccount("bad-tags", "bad-tags@example.com", "", "",
		[]organizations.Tag{{Key: "aws:reserved", Value: "v"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INVALID_SYSTEM_TAGS_PARAMETER")

	after, err := b.ListAccounts()
	require.NoError(t, err)
	assert.Len(t, after, len(before), "no new account must be created when Tags validation fails")
}

// TestCreateOrganizationalUnit_DuplicateTagKeyRejected verifies
// CreateOrganizationalUnit validates its Tags parameter and creates no OU on
// failure.
func TestCreateOrganizationalUnit_DuplicateTagKeyRejected(t *testing.T) {
	t.Parallel()

	b, rootID := newOrgBackend(t)

	_, err := b.CreateOrganizationalUnit(rootID, "bad-ou", []organizations.Tag{
		{Key: "dup", Value: "1"},
		{Key: "dup", Value: "2"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DUPLICATE_TAG_KEY")

	ous, err := b.ListOrganizationalUnitsForParent(rootID)
	require.NoError(t, err)
	assert.Empty(t, ous, "OU must not be created when Tags validation fails")
}

// TestCreatePolicy_ReservedTagPrefixRejected verifies CreatePolicy validates
// its Tags parameter and creates no policy on failure.
func TestCreatePolicy_ReservedTagPrefixRejected(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	_, err := b.CreatePolicy("bad", "", `{}`, "SERVICE_CONTROL_POLICY",
		[]organizations.Tag{{Key: "aws:reserved", Value: "v"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INVALID_SYSTEM_TAGS_PARAMETER")

	policies, err := b.ListPolicies("SERVICE_CONTROL_POLICY")
	require.NoError(t, err)
	assert.Len(t, policies, 1, "policy must not be created when Tags validation fails "+
		"(only the default FullAWSAccess SCP remains)")
}

// TestTagResource_ReservedPrefix_ViaHandler verifies the HTTP wire error.
func TestTagResource_ReservedPrefix_ViaHandler(t *testing.T) {
	t.Parallel()

	h, rootID := newHandlerWithOrg(t)

	rec := doRequest(t, h, "TagResource", map[string]any{
		"ResourceId": rootID,
		"Tags":       []map[string]string{{"Key": "aws:reserved", "Value": "v"}},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "InvalidInputException", errResp["__type"])
}

// TestListTagsForResource_Sorted verifies sorted output by key.
func TestListTagsForResource_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []organizations.Tag
	}{
		{
			name: "three_tags_sorted_by_key",
			tags: []organizations.Tag{
				{Key: "zzz", Value: "1"},
				{Key: "aaa", Value: "2"},
				{Key: "mmm", Value: "3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			status, err := b.CreateAccount("tagged-acct", "t@example.com", "", "", tt.tags)
			require.NoError(t, err)

			tags, err := b.ListTagsForResource(status.AccountID)
			require.NoError(t, err)

			for i := 1; i < len(tags); i++ {
				assert.LessOrEqual(t, tags[i-1].Key, tags[i].Key, "tags should be sorted by key")
			}
		})
	}
}
