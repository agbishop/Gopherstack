package quicksight_test

import (
	"fmt"
	"net/http"
	"testing"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/quicksight/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- CustomPermissions CRUD round-trip and errors ----

func TestQuickSight_CustomPermissionsCRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, http.MethodPost, accountPath("/custom-permissions"), map[string]any{
		"CustomPermissionsName": "cp1",
		"Capabilities":          map[string]any{"ExportToCsv": "DENY"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)
	arn, ok := parseBody(t, createRec)["Arn"].(string)
	require.True(t, ok)
	assert.Contains(t, arn, "custom-permissions/cp1")

	dupRec := doRequest(t, h, http.MethodPost, accountPath("/custom-permissions"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	assert.Equal(t, http.StatusConflict, dupRec.Code)

	describeRec := doRequest(t, h, http.MethodGet, accountPath("/custom-permissions/cp1"), nil)
	require.Equal(t, http.StatusOK, describeRec.Code)
	cp := parseBody(t, describeRec)["CustomPermissions"].(map[string]any)
	caps := cp["Capabilities"].(map[string]any)
	assert.Equal(t, "DENY", caps["ExportToCsv"])

	missingRec := doRequest(t, h, http.MethodGet, accountPath("/custom-permissions/notexist"), nil)
	assert.Equal(t, http.StatusNotFound, missingRec.Code)

	updateRec := doRequest(t, h, http.MethodPut, accountPath("/custom-permissions/cp1"), map[string]any{
		"Capabilities": map[string]any{"ExportToCsv": "ALLOW"},
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	describeAfterUpdate := doRequest(t, h, http.MethodGet, accountPath("/custom-permissions/cp1"), nil)
	afterCaps := parseBody(t, describeAfterUpdate)["CustomPermissions"].(map[string]any)["Capabilities"].(map[string]any)
	assert.Equal(t, "ALLOW", afterCaps["ExportToCsv"])

	// Assign to a role, then deleting should conflict.
	assignRec := doRequest(t, h, http.MethodPut, nsPath("/roles/AUTHOR/custom-permission"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	require.Equal(t, http.StatusOK, assignRec.Code)

	inUseRec := doRequest(t, h, http.MethodDelete, accountPath("/custom-permissions/cp1"), nil)
	assert.Equal(t, http.StatusConflict, inUseRec.Code)

	unassignRec := doRequest(t, h, http.MethodDelete, nsPath("/roles/AUTHOR/custom-permission"), nil)
	require.Equal(t, http.StatusOK, unassignRec.Code)

	deleteRec := doRequest(t, h, http.MethodDelete, accountPath("/custom-permissions/cp1"), nil)
	require.Equal(t, http.StatusOK, deleteRec.Code)
	// DeleteCustomPermissionsOutput carries Arn (api_op_DeleteCustomPermissions.go)
	// -- this backend already builds it deterministically at Create time, just
	// never surfaced it back on delete.
	assert.Equal(t, arn, parseBody(t, deleteRec)["Arn"])

	deleteMissingRec := doRequest(t, h, http.MethodDelete, accountPath("/custom-permissions/cp1"), nil)
	assert.Equal(t, http.StatusNotFound, deleteMissingRec.Code)
}

// ---- ListCustomPermissions pagination ----

func TestQuickSight_ListCustomPermissions_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		doRequest(t, h, http.MethodPost, accountPath("/custom-permissions"), map[string]any{
			"CustomPermissionsName": name,
		})
	}

	rec := doRequest(t, h, http.MethodGet, accountPath("/custom-permissions?max-results=2"), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := parseBody(t, rec)
	items, ok := body["CustomPermissionsList"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
	next := body["NextToken"].(string)
	require.NotEmpty(t, next)

	page2 := doRequest(
		t, h, http.MethodGet,
		accountPath(fmt.Sprintf("/custom-permissions?max-results=2&next-token=%s", next)), nil,
	)
	require.Equal(t, http.StatusOK, page2.Code)
	assert.Len(t, parseBody(t, page2)["CustomPermissionsList"].([]any), 2)
}

// ---- Role custom permission errors ----

func TestQuickSight_RoleCustomPermission_Errors(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Describe before anything is assigned -> 404.
	describeRec := doRequest(t, h, http.MethodGet, nsPath("/roles/READER/custom-permission"), nil)
	assert.Equal(t, http.StatusNotFound, describeRec.Code)

	// Assigning a CustomPermissionsName that doesn't exist -> 404.
	updateRec := doRequest(t, h, http.MethodPut, nsPath("/roles/READER/custom-permission"), map[string]any{
		"CustomPermissionsName": "doesnotexist",
	})
	assert.Equal(t, http.StatusNotFound, updateRec.Code)

	// Deleting a role custom permission that was never set -> 404.
	deleteRec := doRequest(t, h, http.MethodDelete, nsPath("/roles/READER/custom-permission"), nil)
	assert.Equal(t, http.StatusNotFound, deleteRec.Code)
}

// ---- Role membership CRUD round-trip and errors ----

func TestQuickSight_RoleMembershipCRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, http.MethodPost, nsPath("/roles/AUTHOR/members/group1"), nil)
	require.Equal(t, http.StatusOK, createRec.Code)

	dupRec := doRequest(t, h, http.MethodPost, nsPath("/roles/AUTHOR/members/group1"), nil)
	assert.Equal(t, http.StatusConflict, dupRec.Code)

	listRec := doRequest(t, h, http.MethodGet, nsPath("/roles/AUTHOR/members"), nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	members := parseBody(t, listRec)["MembersList"].([]any)
	assert.Contains(t, members, "group1")

	deleteRec := doRequest(t, h, http.MethodDelete, nsPath("/roles/AUTHOR/members/group1"), nil)
	require.Equal(t, http.StatusOK, deleteRec.Code)

	deleteMissingRec := doRequest(t, h, http.MethodDelete, nsPath("/roles/AUTHOR/members/group1"), nil)
	assert.Equal(t, http.StatusNotFound, deleteMissingRec.Code)
}

// Anti-drift: every value the pinned SDK's types.Role enum knows about must
// be accepted, so a hand-maintained allowlist can't fall behind or invent a
// nonexistent role.
func TestQuickSight_RoleMembership_EverySDKRoleAccepted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, role := range sdktypes.Role("").Values() {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodPost, nsPath(fmt.Sprintf("/roles/%s/members/member-%s", role, role)), nil)
			assert.Equal(t, http.StatusOK, rec.Code, "SDK Role %s must be accepted", role)
		})
	}
}

// RESTRICTED_AUTHOR/RESTRICTED_READER are not members of types.Role -- a
// hand-copied allowlist previously invented them and accepted input the real
// API rejects (the more-permissive-than-AWS class).
func TestQuickSight_RoleMembership_NonexistentRolesRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, role := range []string{"RESTRICTED_AUTHOR", "RESTRICTED_READER"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodPost, nsPath("/roles/"+role+"/members/member1"), nil)
			assert.Equal(t, http.StatusBadRequest, rec.Code, "role %s is not a real SDK enum member", role)
		})
	}
}

// ---- User custom permission errors ----

func TestQuickSight_UserCustomPermission_Errors(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, accountPath("/custom-permissions"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// User doesn't exist yet -> 404.
	updateRec := doRequest(t, h, http.MethodPut, nsPath("/users/nosuchuser/custom-permission"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	assert.Equal(t, http.StatusNotFound, updateRec.Code)

	registerRec := doRequest(t, h, http.MethodPost, nsPath("/users"), map[string]any{
		"UserName": "u1", "Email": "u1@example.com", "IdentityType": "QUICKSIGHT", "UserRole": "READER",
	})
	require.Equal(t, http.StatusOK, registerRec.Code)

	// CustomPermissionsName doesn't exist -> 404.
	badPermRec := doRequest(t, h, http.MethodPut, nsPath("/users/u1/custom-permission"), map[string]any{
		"CustomPermissionsName": "doesnotexist",
	})
	assert.Equal(t, http.StatusNotFound, badPermRec.Code)

	okRec := doRequest(t, h, http.MethodPut, nsPath("/users/u1/custom-permission"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	require.Equal(t, http.StatusOK, okRec.Code)

	deleteRec := doRequest(t, h, http.MethodDelete, nsPath("/users/u1/custom-permission"), nil)
	require.Equal(t, http.StatusOK, deleteRec.Code)

	deleteMissingRec := doRequest(t, h, http.MethodDelete, nsPath("/users/u1/custom-permission"), nil)
	assert.Equal(t, http.StatusNotFound, deleteMissingRec.Code)
}

// TestQuickSight_UserCustomPermission_ReflectedInReads is a regression test
// for gopherstack-rt14: UpdateUserCustomPermission wrote to
// userCustomPermissions but DescribeUser/ListUsers never read it back, so a
// value set on a user was never observable. types.User.CustomPermissionsName
// (aws-sdk-go-v2/service/quicksight/types/types.go:23202) is the read shape.
// Subtests share and mutate one user's state, so they run sequentially.
func TestQuickSight_UserCustomPermission_ReflectedInReads(t *testing.T) { //nolint:paralleltest // sequential
	h := newTestHandler(t)

	registerRec := doRequest(t, h, http.MethodPost, nsPath("/users"), map[string]any{
		"UserName": "u1", "Email": "u1@example.com", "IdentityType": "QUICKSIGHT", "UserRole": "READER",
	})
	require.Equal(t, http.StatusOK, registerRec.Code)

	cpRec := doRequest(t, h, http.MethodPost, accountPath("/custom-permissions"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	require.Equal(t, http.StatusOK, cpRec.Code)

	t.Run("absent before update", func(t *testing.T) { //nolint:paralleltest // sequential
		rec := doRequest(t, h, http.MethodGet, nsPath("/users/u1"), nil)
		require.Equal(t, http.StatusOK, rec.Code)
		user, ok := parseBody(t, rec)["User"].(map[string]any)
		require.True(t, ok)
		assert.NotContains(t, user, "CustomPermissionsName")
	})

	updateRec := doRequest(t, h, http.MethodPut, nsPath("/users/u1/custom-permission"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	t.Run("describe user surfaces it", func(t *testing.T) { //nolint:paralleltest // sequential
		rec := doRequest(t, h, http.MethodGet, nsPath("/users/u1"), nil)
		require.Equal(t, http.StatusOK, rec.Code)
		user, ok := parseBody(t, rec)["User"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "cp1", user["CustomPermissionsName"])
	})

	t.Run("list users surfaces it", func(t *testing.T) { //nolint:paralleltest // sequential
		rec := doRequest(t, h, http.MethodGet, nsPath("/users"), nil)
		require.Equal(t, http.StatusOK, rec.Code)
		list, ok := parseBody(t, rec)["UserList"].([]any)
		require.True(t, ok)
		require.Len(t, list, 1)
		user, ok := list[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "cp1", user["CustomPermissionsName"])
	})

	deleteRec := doRequest(t, h, http.MethodDelete, nsPath("/users/u1/custom-permission"), nil)
	require.Equal(t, http.StatusOK, deleteRec.Code)

	t.Run("absent again after delete", func(t *testing.T) { //nolint:paralleltest // sequential
		rec := doRequest(t, h, http.MethodGet, nsPath("/users/u1"), nil)
		require.Equal(t, http.StatusOK, rec.Code)
		user, ok := parseBody(t, rec)["User"].(map[string]any)
		require.True(t, ok)
		assert.NotContains(t, user, "CustomPermissionsName")
	})
}

// ---- Custom Permissions tests ---- //nolint:godot // existing issue.
func TestQuickSight_CustomPermissions(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	tests := []struct {
		body       any
		name       string
		method     string
		path       string
		wantKey    string
		wantStatus int
	}{
		{
			name:       "create custom permissions",
			method:     http.MethodPost,
			path:       accountPath("/custom-permissions"),
			body:       map[string]any{"CustomPermissionsName": "cp1"},
			wantStatus: http.StatusOK,
			wantKey:    "Arn",
		},
		{
			name:       "describe custom permissions",
			method:     http.MethodGet,
			path:       accountPath("/custom-permissions/cp1"),
			wantStatus: http.StatusOK,
			wantKey:    "CustomPermissions",
		},
		{
			name:       "update custom permissions",
			method:     http.MethodPut,
			path:       accountPath("/custom-permissions/cp1"),
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantKey:    "Arn",
		},
		{
			name:       "list custom permissions",
			method:     http.MethodGet,
			path:       accountPath("/custom-permissions"),
			wantStatus: http.StatusOK,
			wantKey:    "CustomPermissionsList",
		},
		{
			name:       "delete custom permissions",
			method:     http.MethodDelete,
			path:       accountPath("/custom-permissions/cp1"),
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantStatus, rec.Code, "status")
			if tc.wantKey != "" {
				body := parseBody(t, rec)
				assert.Contains(t, body, tc.wantKey)
			}
		})
	}
}

// ---- Role Membership tests ---- //nolint:godot // existing issue.
func TestQuickSight_RoleMemberships(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// A custom permissions profile must exist before it can be assigned to a role.
	setupRec := doRequest(t, h, http.MethodPost, accountPath("/custom-permissions"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	require.Equal(t, http.StatusOK, setupRec.Code)

	tests := []struct {
		body       any
		name       string
		method     string
		path       string
		wantKey    string
		wantStatus int
	}{
		{
			name:       "create role membership",
			method:     http.MethodPost,
			path:       nsPath("/roles/ADMIN/members/user1"),
			wantStatus: http.StatusOK,
		},
		{
			name:       "list role memberships",
			method:     http.MethodGet,
			path:       nsPath("/roles/ADMIN/members"),
			wantStatus: http.StatusOK,
			wantKey:    "MembersList",
		},
		{
			name:       "update role custom permission",
			method:     http.MethodPut,
			path:       nsPath("/roles/ADMIN/custom-permission"),
			body:       map[string]any{"CustomPermissionsName": "cp1"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "describe role custom permission",
			method:     http.MethodGet,
			path:       nsPath("/roles/ADMIN/custom-permission"),
			wantStatus: http.StatusOK,
			wantKey:    "CustomPermissionsName",
		},
		{
			name:       "delete role custom permission",
			method:     http.MethodDelete,
			path:       nsPath("/roles/ADMIN/custom-permission"),
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete role membership",
			method:     http.MethodDelete,
			path:       nsPath("/roles/ADMIN/members/user1"),
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantStatus, rec.Code, "status")
			if tc.wantKey != "" {
				body := parseBody(t, rec)
				assert.Contains(t, body, tc.wantKey)
			}
		})
	}
}

// ---- User custom permission tests ---- //nolint:godot // existing issue.
func TestQuickSight_UserCustomPermission(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	setupRec := doRequest(t, h, http.MethodPost, nsPath("/users"), map[string]any{
		"UserName": "user1", "Email": "user1@example.com", "IdentityType": "QUICKSIGHT", "UserRole": "READER",
	})
	require.Equal(t, http.StatusOK, setupRec.Code)

	setupRec = doRequest(t, h, http.MethodPost, accountPath("/custom-permissions"), map[string]any{
		"CustomPermissionsName": "cp1",
	})
	require.Equal(t, http.StatusOK, setupRec.Code)

	tests := []struct {
		body       any
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "update user custom permission",
			method:     http.MethodPut,
			path:       nsPath("/users/user1/custom-permission"),
			body:       map[string]any{"CustomPermissionsName": "cp1"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete user custom permission",
			method:     http.MethodDelete,
			path:       nsPath("/users/user1/custom-permission"),
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantStatus, rec.Code, "status")
		})
	}
}
