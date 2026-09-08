package identitystore_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/identitystore"
)

// invalidStoreID fails patternIdentityStoreID (neither "d-"+10 hex nor a
// UUID) -- it is used to prove requireIdentityStoreID's rejection actually
// stops the request instead of merely writing a 400 alongside it
// (gopherstack-n7nk).
const invalidStoreID = "not-a-valid-store-id"

// TestRequireIdentityStoreIDGuardBlocksMutations proves that all 8 mutation
// operations gated by requireIdentityStoreID actually refuse to mutate when
// IdentityStoreId fails validation. A status-code-only assertion (rec.Code
// == 400) PASSES against the unfixed bug: requireIdentityStoreID's rejection
// write happens first and its 400 status wins the race for
// Response.Committed, so rec.Code reads 400 either way while the mutation
// still silently proceeds underneath it. Every case here instead re-reads
// backend state through the exported Handler.Backend field to prove the
// resource was never created/updated/deleted.
func TestRequireIdentityStoreIDGuardBlocksMutations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *identitystore.Handler)
		name string
	}{
		{
			name: "create_group",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateGroup", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"DisplayName":     "ShouldNotExist",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				groups := h.Backend.ListGroups(t.Context(), invalidStoreID)
				assert.Empty(t, groups, "CreateGroup must not create a group when IdentityStoreId fails validation")
			},
		},
		{
			name: "create_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				rec := doRequest(t, h, "CreateUser", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"UserName":        "should.not.exist",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				users := h.Backend.ListUsers(t.Context(), invalidStoreID)
				assert.Empty(t, users, "CreateUser must not create a user when IdentityStoreId fails validation")
			},
		},
		{
			name: "create_group_membership",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				ctx := t.Context()
				group, err := h.Backend.CreateGroup(
					ctx,
					invalidStoreID,
					&identitystore.CreateGroupRequest{DisplayName: "G"},
				)
				require.NoError(t, err)
				user, err := h.Backend.CreateUser(ctx, invalidStoreID, &identitystore.CreateUserRequest{UserName: "u"})
				require.NoError(t, err)

				rec := doRequest(t, h, "CreateGroupMembership", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"GroupId":         group.GroupID,
					"MemberId":        map[string]any{"UserId": user.UserID},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				memberships := h.Backend.ListGroupMemberships(ctx, invalidStoreID, group.GroupID)
				assert.Empty(t, memberships,
					"CreateGroupMembership must not create a membership when IdentityStoreId fails validation")
			},
		},
		{
			name: "update_group",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				ctx := t.Context()
				group, err := h.Backend.CreateGroup(
					ctx,
					invalidStoreID,
					&identitystore.CreateGroupRequest{DisplayName: "Original"},
				)
				require.NoError(t, err)

				rec := doRequest(t, h, "UpdateGroup", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"GroupId":         group.GroupID,
					"Operations": []map[string]any{
						{"AttributePath": "displayName", "AttributeValue": "Changed"},
					},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				after, err := h.Backend.DescribeGroup(ctx, invalidStoreID, group.GroupID)
				require.NoError(t, err)
				assert.Equal(t, "Original", after.DisplayName,
					"UpdateGroup must not mutate the group when IdentityStoreId fails validation")
			},
		},
		{
			name: "delete_group",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				ctx := t.Context()
				group, err := h.Backend.CreateGroup(
					ctx,
					invalidStoreID,
					&identitystore.CreateGroupRequest{DisplayName: "ToKeep"},
				)
				require.NoError(t, err)

				rec := doRequest(t, h, "DeleteGroup", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"GroupId":         group.GroupID,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				_, err = h.Backend.DescribeGroup(ctx, invalidStoreID, group.GroupID)
				assert.NoError(t, err, "DeleteGroup must not delete the group when IdentityStoreId fails validation")
			},
		},
		{
			name: "update_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				ctx := t.Context()
				user, err := h.Backend.CreateUser(
					ctx,
					invalidStoreID,
					&identitystore.CreateUserRequest{UserName: "original.name"},
				)
				require.NoError(t, err)

				rec := doRequest(t, h, "UpdateUser", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"UserId":          user.UserID,
					"Operations": []map[string]any{
						{"AttributePath": "displayName", "AttributeValue": "Changed"},
					},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				after, err := h.Backend.DescribeUser(ctx, invalidStoreID, user.UserID)
				require.NoError(t, err)
				assert.Empty(t, after.DisplayName,
					"UpdateUser must not mutate the user when IdentityStoreId fails validation")
			},
		},
		{
			name: "delete_user",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				ctx := t.Context()
				user, err := h.Backend.CreateUser(
					ctx,
					invalidStoreID,
					&identitystore.CreateUserRequest{UserName: "to.keep"},
				)
				require.NoError(t, err)

				rec := doRequest(t, h, "DeleteUser", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"UserId":          user.UserID,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				_, err = h.Backend.DescribeUser(ctx, invalidStoreID, user.UserID)
				assert.NoError(t, err, "DeleteUser must not delete the user when IdentityStoreId fails validation")
			},
		},
		{
			name: "delete_group_membership",
			run: func(t *testing.T, h *identitystore.Handler) {
				t.Helper()

				ctx := t.Context()
				group, err := h.Backend.CreateGroup(
					ctx,
					invalidStoreID,
					&identitystore.CreateGroupRequest{DisplayName: "G"},
				)
				require.NoError(t, err)
				user, err := h.Backend.CreateUser(ctx, invalidStoreID, &identitystore.CreateUserRequest{UserName: "u"})
				require.NoError(t, err)
				membership, err := h.Backend.CreateGroupMembership(
					ctx, invalidStoreID, group.GroupID, identitystore.MemberID{UserID: user.UserID},
				)
				require.NoError(t, err)

				rec := doRequest(t, h, "DeleteGroupMembership", map[string]any{
					"IdentityStoreId": invalidStoreID,
					"MembershipId":    membership.MembershipID,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				_, err = h.Backend.DescribeGroupMembership(ctx, invalidStoreID, membership.MembershipID)
				assert.NoError(t, err,
					"DeleteGroupMembership must not delete the membership when IdentityStoreId fails validation")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.run(t, newTestHandler())
		})
	}
}

// TestRequireIdentityStoreIDGuardReadsSingleWellFormedBody covers the 12
// read-side call sites representatively (both requireIdentityStoreID's
// direct call sites, via DescribeGroup/DescribeUser, and the
// parseAlternateIDRequest chain feeding GetGroupId/GetUserId).
//
// The status code alone does not distinguish fixed from broken here either:
// requireIdentityStoreID's 400 write commits Response.Committed first, so a
// second write from the read handler falling through to a real (NotFound)
// backend call cannot change rec.Code -- only rec.Body, which ends up as two
// JSON documents concatenated back-to-back. This is exactly what pinpoint's
// gopherstack-246v fix had to assert (see PARITY.md); json.Unmarshal fails
// with "invalid character '{' after top-level value" against that shape.
func TestRequireIdentityStoreIDGuardReadsSingleWellFormedBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
		op   string
	}{
		{
			name: "describe_group_invalid_store_id",
			op:   "DescribeGroup",
			body: map[string]any{"IdentityStoreId": invalidStoreID, "GroupId": "some-group-id"},
		},
		{
			name: "describe_user_invalid_store_id",
			op:   "DescribeUser",
			body: map[string]any{"IdentityStoreId": invalidStoreID, "UserId": "some-user-id"},
		},
		{
			name: "get_group_id_invalid_store_id",
			op:   "GetGroupId",
			body: map[string]any{
				"IdentityStoreId": invalidStoreID,
				"AlternateIdentifier": map[string]any{
					"UniqueAttribute": map[string]any{
						"AttributePath":  "displayName",
						"AttributeValue": "whatever",
					},
				},
			},
		},
		{
			name: "get_user_id_invalid_store_id",
			op:   "GetUserId",
			body: map[string]any{
				"IdentityStoreId": invalidStoreID,
				"AlternateIdentifier": map[string]any{
					"UniqueAttribute": map[string]any{
						"AttributePath":  "userName",
						"AttributeValue": "whatever",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doRequest(t, h, tt.op, tt.body)

			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]any
			require.NoError(
				t,
				json.Unmarshal(rec.Body.Bytes(), &resp),
				"response body must be a single well-formed JSON document, not two writes concatenated: %s",
				rec.Body.String(),
			)
			assert.Equal(t, "ValidationException", resp["__type"])
		})
	}
}
