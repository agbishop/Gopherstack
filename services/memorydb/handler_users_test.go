package memorydb_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/memorydb"
)

// TestHandler_DescribeUsers_All tests DescribeUsers with no filter.
func TestHandler_DescribeUsers_All(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateUser", map[string]any{
		"UserName":     "u1",
		"AccessString": "on ~*",
		"AuthenticationMode": map[string]any{
			"Type":      "password",
			"Passwords": []string{"pass1"},
		},
	})

	rec := doRequest(t, h, "DescribeUsers", map[string]any{})
	assert.Equal(t, 200, rec.Code)
}

func TestWireAuthType_DefaultUserIsNoPassword(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateUser", map[string]any{
		"UserName":     "default-auth-user",
		"AccessString": "on ~* +@all",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	user, _ := resp["User"].(map[string]any)
	auth, _ := user["Authentication"].(map[string]any)
	assert.Equal(t, "no-password", auth["Type"],
		"a user created without AuthenticationMode.Type must report Authentication.Type=no-password on the wire")
}

func TestWireAuthType_UpdateUserValidatesType(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateUser", map[string]any{
		"UserName":     "update-auth-user",
		"AccessString": "on ~* +@all",
		"AuthenticationMode": map[string]any{
			"Type":      "password",
			"Passwords": []string{"initialpassword1"},
		},
	})

	// UpdateUser must reject the same invalid combination CreateUser rejects:
	// iam auth with passwords supplied.
	rec := doRequest(t, h, "UpdateUser", map[string]any{
		"UserName": "update-auth-user",
		"AuthenticationMode": map[string]any{
			"Type":      "iam",
			"Passwords": []string{"shouldnotbeallowed"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"UpdateUser must reject AuthenticationMode.Type=iam combined with Passwords, like CreateUser does")

	// A valid switch to no-password-required aliases to "no-password" on the wire.
	rec = doRequest(t, h, "UpdateUser", map[string]any{
		"UserName": "update-auth-user",
		"AuthenticationMode": map[string]any{
			"Type": "no-password-required",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	user, _ := resp["User"].(map[string]any)
	auth, _ := user["Authentication"].(map[string]any)
	assert.Equal(t, "no-password", auth["Type"])
}

// -- Returned-pointer isolation concurrency smoke tests ---------------------
//
// Several backend Create*/Copy*/Export* methods used to return the live
// table entry instead of a defensive clone, unlike the rest of this file's
// read paths (see the "Deep-copy helpers" section in backend.go). The
// current wire converters (toSnapshotObject, toParameterGroupObject, ...)
// don't happen to serialize the affected Tags/Parameters maps, so this
// wasn't independently reproducible as a failing -race case today; fixed
// anyway for consistency with the established clone-on-return convention,
// since any converter that starts including those maps would otherwise
// reintroduce a live race against concurrent TagResource/Update* calls on
// the same resource. These tests exercise that concurrent traffic under
// -race as a smoke test/guard.

// doRequest calls require.* internally, which must only run on the test's
// own goroutine, so concurrent callers below use doRequestAsync and collect
// each response's status code over a channel to assert on afterward, in the
// main test goroutine.

// TestParity_UserResponse_ACLNames verifies ACLNames is returned instead of UserGroupCount.
func TestHandler_UserResponse_ACLNames(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateUser", map[string]any{
		"UserName":           "acl-user",
		"AccessString":       "on ~* &* +@all",
		"AuthenticationMode": map[string]any{"Type": "no-password-required"},
	})

	doRequest(t, h, "CreateACL", map[string]any{
		"ACLName":   "acl-alpha",
		"UserNames": []string{"acl-user"},
	})
	doRequest(t, h, "CreateACL", map[string]any{
		"ACLName":   "acl-beta",
		"UserNames": []string{"acl-user"},
	})

	users := doDescribeUsers(t, h, "acl-user")
	require.Len(t, users, 1)
	user, _ := users[0].(map[string]any)

	aclNames, _ := user["ACLNames"].([]any)
	assert.Len(t, aclNames, 2, "ACLNames must contain both ACLs")

	names := make([]string, 0, len(aclNames))
	for _, n := range aclNames {
		if s, ok := n.(string); ok {
			names = append(names, s)
		}
	}

	assert.Contains(t, names, "acl-alpha")
	assert.Contains(t, names, "acl-beta")

	_, hasOld := user["UserGroupCount"]
	assert.False(t, hasOld, "UserGroupCount must not appear in response")
}

// TestHandler_UserResponse_NoEngineField verifies the User response has no
// "Engine" field -- confirmed absent from the real SDK's User type
// (deserializers.go's awsAwsjson11_deserializeDocumentUser only recognizes
// AccessString, ACLNames, ARN, Authentication, MinimumEngineVersion, Name,
// Status); a prior pass fabricated one and this locked its presence in, which
// this test now inverts.
func TestHandler_UserResponse_NoEngineField(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateUser", map[string]any{
		"UserName":           "engine-user",
		"AccessString":       "on ~* &* +@all",
		"AuthenticationMode": map[string]any{"Type": "no-password-required"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	u, _ := resp["User"].(map[string]any)
	_, hasEngine := u["Engine"]
	assert.False(t, hasEngine, "Engine must not appear in the User response (not part of the real SDK type)")
}

// TestParity_UserResponse_ACLNames_CreateReturnsEmpty verifies newly created user has empty ACLNames.
func TestHandler_UserResponse_ACLNames_CreateReturnsEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateUser", map[string]any{
		"UserName":           "fresh-user",
		"AccessString":       "on ~* &* +@all",
		"AuthenticationMode": map[string]any{"Type": "no-password-required"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	u, _ := resp["User"].(map[string]any)

	aclNames, hasField := u["ACLNames"]
	assert.True(t, hasField, "ACLNames must be present in CreateUser response")
	aclList, _ := aclNames.([]any)
	assert.Empty(t, aclList, "newly created user must have empty ACLNames")
}

// TestHandler_User_CRUD tests full User lifecycle through the handler.
func TestHandler_User_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		setup      func(*memorydb.Handler)
		name       string
		op         string
		wantStatus int
	}{
		{
			name: "create user",
			op:   "CreateUser",
			body: map[string]any{
				"UserName":     "my-user",
				"AccessString": "on ~* +@all",
				"AuthenticationMode": map[string]any{
					"Type":      "password",
					"Passwords": []string{"mypassword123"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "create user missing name",
			op:         "CreateUser",
			body:       map[string]any{"AccessString": "on ~*"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "describe users",
			op:   "DescribeUsers",
			setup: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":     "user-x",
					"AccessString": "on ~*",
					"AuthenticationMode": map[string]any{
						"Type":      "password",
						"Passwords": []string{"pass1"},
					},
				})
			},
			body:       map[string]any{"UserName": "user-x"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "describe users not found",
			op:         "DescribeUsers",
			body:       map[string]any{"UserName": "no-such"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "delete user",
			op:   "DeleteUser",
			setup: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":     "del-user",
					"AccessString": "on ~*",
					"AuthenticationMode": map[string]any{
						"Type":      "password",
						"Passwords": []string{"pass1"},
					},
				})
			},
			body:       map[string]any{"UserName": "del-user"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete user missing name",
			op:         "DeleteUser",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "delete user not found",
			op:         "DeleteUser",
			body:       map[string]any{"UserName": "no-such"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "update user",
			op:   "UpdateUser",
			setup: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":     "upd-user",
					"AccessString": "on ~*",
					"AuthenticationMode": map[string]any{
						"Type":      "password",
						"Passwords": []string{"pass1"},
					},
				})
			},
			body: map[string]any{
				"UserName":     "upd-user",
				"AccessString": "on ~* +@read",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "update user missing name",
			op:         "UpdateUser",
			body:       map[string]any{"AccessString": "on ~*"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "update user not found",
			op:         "UpdateUser",
			body:       map[string]any{"UserName": "no-such"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_User_MinimumEngineVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		userName string
	}{
		{"created user has MinimumEngineVersion", "test-user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			rec := doRequest(t, h, "CreateUser", map[string]any{
				"UserName":     tt.userName,
				"AccessString": "on ~* &* +@all",
				"AuthenticationMode": map[string]any{
					"Type": "no-password-required",
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			users := doDescribeUsers(t, h, tt.userName)
			require.Len(t, users, 1)

			user, _ := users[0].(map[string]any)
			minVersion, _ := user["MinimumEngineVersion"].(string)
			assert.Equal(t, "6.2", minVersion)
		})
	}
}

func TestHandler_User_ACLNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupFn      func(h *memorydb.Handler)
		userName     string
		wantACLCount int
	}{
		{
			name: "user in no ACL has empty ACLNames",
			setupFn: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":           "standalone-user",
					"AccessString":       "on ~* &* +@all",
					"AuthenticationMode": map[string]any{"Type": "no-password-required"},
				})
			},
			userName:     "standalone-user",
			wantACLCount: 0,
		},
		{
			name: "user in one ACL has one ACLName",
			setupFn: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":           "grouped-user",
					"AccessString":       "on ~* &* +@all",
					"AuthenticationMode": map[string]any{"Type": "no-password-required"},
				})
				doRequest(t, h, "CreateACL", map[string]any{
					"ACLName":   "test-acl-group",
					"UserNames": []string{"grouped-user"},
				})
			},
			userName:     "grouped-user",
			wantACLCount: 1,
		},
		{
			name: "user in two ACLs has two ACLNames",
			setupFn: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":           "multi-acl-user",
					"AccessString":       "on ~* &* +@all",
					"AuthenticationMode": map[string]any{"Type": "no-password-required"},
				})
				doRequest(t, h, "CreateACL", map[string]any{
					"ACLName":   "test-acl-a",
					"UserNames": []string{"multi-acl-user"},
				})
				doRequest(t, h, "CreateACL", map[string]any{
					"ACLName":   "test-acl-b",
					"UserNames": []string{"multi-acl-user"},
				})
			},
			userName:     "multi-acl-user",
			wantACLCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			tt.setupFn(h)

			users := doDescribeUsers(t, h, tt.userName)
			require.Len(t, users, 1)

			user, _ := users[0].(map[string]any)
			aclNames, _ := user["ACLNames"].([]any)
			assert.Len(t, aclNames, tt.wantACLCount)
		})
	}
}

// -- Finding 20: Snapshot cluster configuration ----------------------------------

func TestHandler_User_ACLNames_Accurate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		aclCount int
	}{
		{"user in 0 ACLs", 0},
		{"user in 1 ACL", 1},
		{"user in 2 ACLs", 2},
		{"user in 3 ACLs", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			// Create a user.
			rec := doRequest(t, h, "CreateUser", map[string]any{
				"UserName":           "ugc-user",
				"AccessString":       "on ~* &* +@all",
				"AuthenticationMode": map[string]any{"Type": "no-password-required"},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// Add user to N ACLs.
			for i := range tt.aclCount {
				aclName := "ugc-acl-" + string(rune('a'+i))
				rec2 := doRequest(t, h, "CreateACL", map[string]any{
					"ACLName":   aclName,
					"UserNames": []string{"ugc-user"},
				})
				require.Equal(t, http.StatusOK, rec2.Code, "create ACL: %s", rec2.Body)
			}

			users := doDescribeUsers(t, h, "ugc-user")
			require.Len(t, users, 1)
			user, _ := users[0].(map[string]any)
			aclNames, _ := user["ACLNames"].([]any)
			assert.Len(t, aclNames, tt.aclCount,
				"ACLNames length should be %d for user in %d ACLs", tt.aclCount, tt.aclCount)
		})
	}
}

// -- Service updates filtering accuracy (finding 23) ----------------------------

// TestHandler_UpdateUser_WithAuthMode tests UpdateUser with authentication mode changes.
func TestHandler_UpdateUser_WithAuthMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		updateBody map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "update access string",
			updateBody: map[string]any{
				"UserName":     "update-user",
				"AccessString": "on ~new-prefix",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "update with auth mode",
			updateBody: map[string]any{
				"UserName":     "update-user",
				"AccessString": "on ~*",
				"AuthenticationMode": map[string]any{
					"Type":      "password",
					"Passwords": []string{"new-pass"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "update nonexistent user returns 400",
			updateBody: map[string]any{
				"UserName":     "no-such-user",
				"AccessString": "on ~*",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.name != "update nonexistent user returns 400" {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":     "update-user",
					"AccessString": "on ~*",
					"AuthenticationMode": map[string]any{
						"Type":      "password",
						"Passwords": []string{"pass"},
					},
				})
			}

			rec := doRequest(t, h, "UpdateUser", tt.updateBody)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_CreateUser_AuthTypes tests user creation with different auth types.
func TestHandler_CreateUser_AuthTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "iam auth type",
			body: map[string]any{
				"UserName":     "iam-user",
				"AccessString": "on ~*",
				"AuthenticationMode": map[string]any{
					"Type": "iam",
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "no-password auth type",
			body: map[string]any{
				"UserName":     "nopw-user",
				"AccessString": "on ~*",
				"AuthenticationMode": map[string]any{
					"Type": "no-password",
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "invalid auth type returns 400",
			body: map[string]any{
				"UserName":     "bad-auth-user",
				"AccessString": "on ~*",
				"AuthenticationMode": map[string]any{
					"Type": "invalid-type",
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "iam with passwords returns 400",
			body: map[string]any{
				"UserName":     "iam-pw-user",
				"AccessString": "on ~*",
				"AuthenticationMode": map[string]any{
					"Type":      "iam",
					"Passwords": []string{"password"},
				},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateUser", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_DeleteUser_InACL tests that deleting a user that is in an ACL
// succeeds (cascade), per api_op_DeleteUser.go's doc comment.
func TestHandler_DeleteUser_InACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{name: "delete user in ACL cascades", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			doRequest(t, h, "CreateUser", map[string]any{
				"UserName":     "acl-member",
				"AccessString": "on ~*",
				"AuthenticationMode": map[string]any{
					"Type":      "password",
					"Passwords": []string{"pass"},
				},
			})

			doRequest(t, h, "CreateACL", map[string]any{
				"ACLName":   "has-user",
				"UserNames": []string{"acl-member"},
			})

			rec := doRequest(t, h, "DeleteUser", map[string]any{"UserName": "acl-member"})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_CreateUser_IamAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "iam auth type accepted",
			body: map[string]any{
				"UserName":     "iam-user",
				"AccessString": "on ~* +@all",
				"AuthenticationMode": map[string]any{
					"Type": "iam",
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "iam auth with passwords rejected",
			body: map[string]any{
				"UserName":     "iam-user",
				"AccessString": "on ~* +@all",
				"AuthenticationMode": map[string]any{
					"Type":      "iam",
					"Passwords": []string{"somepassword"},
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "no-password-required accepted",
			body: map[string]any{
				"UserName":     "nopass-user",
				"AccessString": "on ~* +@all",
				"AuthenticationMode": map[string]any{
					"Type": "no-password-required",
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "password auth accepted",
			body: map[string]any{
				"UserName":     "pass-user",
				"AccessString": "on ~* +@all",
				"AuthenticationMode": map[string]any{
					"Type":      "password",
					"Passwords": []string{"mypassword123"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "invalid auth type rejected",
			body: map[string]any{
				"UserName":     "bad-user",
				"AccessString": "on ~* +@all",
				"AuthenticationMode": map[string]any{
					"Type": "kerberos",
				},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateUser", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// -- Automated snapshots (Gap 16) ----------------------------------------------

func TestHandler_UserCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*memorydb.Handler)
		body       map[string]any
		name       string
		op         string
		wantStatus int
	}{
		{
			name: "create user password auth",
			op:   "CreateUser",
			body: map[string]any{
				"UserName":     "test-user",
				"AccessString": "on ~* +@all",
				"AuthenticationMode": map[string]any{
					"Type":      "password",
					"Passwords": []string{"mypassword"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "create user missing name",
			op:         "CreateUser",
			body:       map[string]any{"AccessString": "on ~* +@all"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "describe users",
			op:   "DescribeUsers",
			setup: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":           "test-user",
					"AccessString":       "on ~* +@all",
					"AuthenticationMode": map[string]any{"Type": "no-password-required"},
				})
			},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
		},
		{
			name: "update user access string",
			op:   "UpdateUser",
			setup: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":           "test-user",
					"AccessString":       "on ~* +@all",
					"AuthenticationMode": map[string]any{"Type": "no-password-required"},
				})
			},
			body: map[string]any{
				"UserName":     "test-user",
				"AccessString": "on ~key* +@read",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "delete user",
			op:   "DeleteUser",
			setup: func(h *memorydb.Handler) {
				doRequest(t, h, "CreateUser", map[string]any{
					"UserName":           "test-user",
					"AccessString":       "on ~* +@all",
					"AuthenticationMode": map[string]any{"Type": "no-password-required"},
				})
			},
			body:       map[string]any{"UserName": "test-user"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// -- MultiRegionCluster --------------------------------------------------------
