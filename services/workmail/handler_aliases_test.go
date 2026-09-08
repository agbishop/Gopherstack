package workmail_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workmail"
)

// --- Aliases ---

func TestWorkMail_Aliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *workmail.Handler)
		name string
	}{
		{
			name: "create_and_list_alias",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "aliasorg")
				userID := createTestUser(t, h, orgID, "aliasuser", "Alias User")
				// register primary email
				doOp(t, h, "RegisterToWorkMail", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Email":"main@aliasorg.awsapps.com"}`, orgID, userID,
				))
				// add alias
				rec := doOp(t, h, "CreateAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"alt@aliasorg.awsapps.com"}`, orgID, userID,
				))
				require.Equal(t, http.StatusOK, rec.Code)
				// list aliases includes both
				rec2 := doOp(t, h, "ListAliases", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q}`, orgID, userID,
				))
				require.Equal(t, http.StatusOK, rec2.Code)
				m := decodeJSON(t, rec2)
				aliases, ok := m["Aliases"].([]any)
				require.True(t, ok)
				// primary + 1 alias = 2 total
				assert.Len(t, aliases, 2)
			},
		},
		{
			name: "delete_alias",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "delalorg")
				userID := createTestUser(t, h, orgID, "delaluser", "Del Alias")
				doOp(t, h, "CreateAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"bye@delalorg.awsapps.com"}`, orgID, userID,
				))
				rec := doOp(t, h, "DeleteAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"bye@delalorg.awsapps.com"}`, orgID, userID,
				))
				require.Equal(t, http.StatusOK, rec.Code)
				rec2 := doOp(t, h, "ListAliases", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q}`, orgID, userID,
				))
				m := decodeJSON(t, rec2)
				aliases := m["Aliases"].([]any)
				assert.Empty(t, aliases)
			},
		},
		{
			name: "duplicate_alias_returns_conflict",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "dupalorg")
				userID := createTestUser(t, h, orgID, "dupaluser", "Dup")
				doOp(t, h, "CreateAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"same@dupalorg.awsapps.com"}`, orgID, userID,
				))
				rec := doOp(t, h, "CreateAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"same@dupalorg.awsapps.com"}`, orgID, userID,
				))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				m := decodeJSON(t, rec)
				assert.Equal(t, "EmailAddressInUseException", m["__type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.run(t, h)
		})
	}
}

// TestWorkMail_CreateAlias_MaxAliasesPerUser covers the hard, non-adjustable
// "Maximum number of aliases per user" quota of 100
// (docs.aws.amazon.com/workmail/latest/adminguide/workmail_limits.html).
func TestWorkMail_CreateAlias_MaxAliasesPerUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *workmail.Handler)
		name string
	}{
		{
			name: "at_limit_succeeds_one_over_fails",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "aliasquotaorg")
				userID := createTestUser(t, h, orgID, "quotauser", "Quota User")

				for i := range 100 {
					rec := doOp(t, h, "CreateAlias", fmt.Sprintf(
						`{"OrganizationId":%q,"EntityId":%q,"Alias":"alias%d@aliasquotaorg.awsapps.com"}`,
						orgID, userID, i,
					))
					require.Equalf(t, http.StatusOK, rec.Code, "alias %d should succeed at the limit", i)
				}

				rec := doOp(t, h, "CreateAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"alias100@aliasquotaorg.awsapps.com"}`,
					orgID, userID,
				))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				m := decodeJSON(t, rec)
				assert.Equal(t, "LimitExceededException", m["__type"])
			},
		},
		{
			name: "scoped_per_user_not_organization",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "aliasscopeorg")
				user1 := createTestUser(t, h, orgID, "scopeuser1", "Scope User One")
				user2 := createTestUser(t, h, orgID, "scopeuser2", "Scope User Two")

				for i := range 100 {
					rec := doOp(t, h, "CreateAlias", fmt.Sprintf(
						`{"OrganizationId":%q,"EntityId":%q,"Alias":"u1alias%d@aliasscopeorg.awsapps.com"}`,
						orgID, user1, i,
					))
					require.Equalf(t, http.StatusOK, rec.Code, "alias %d should succeed at the limit", i)
				}

				rec := doOp(t, h, "CreateAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"u1alias100@aliasscopeorg.awsapps.com"}`,
					orgID, user1,
				))
				assert.Equal(t, http.StatusBadRequest, rec.Code, "user1 is at its own limit")

				rec2 := doOp(t, h, "CreateAlias", fmt.Sprintf(
					`{"OrganizationId":%q,"EntityId":%q,"Alias":"u2alias@aliasscopeorg.awsapps.com"}`,
					orgID, user2,
				))
				assert.Equal(t, http.StatusOK, rec2.Code, "user2's own quota is untouched by user1's")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.run(t, h)
		})
	}
}
