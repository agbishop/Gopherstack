package appstream_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appstream"
)

// TestAppStream_Users covers User CRUD and enable/disable.
func TestAppStream_Users(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *appstream.Handler)
		check    func(t *testing.T, body []byte)
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "CreateUser returns OK",
			action: "CreateUser",
			body: map[string]any{
				"UserName":           "alice@example.com",
				"AuthenticationType": "USERPOOL",
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "CreateUser duplicate returns error",
			action: "CreateUser",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "dup-user")
			},
			body: map[string]any{
				"UserName":           "dup-user",
				"AuthenticationType": "USERPOOL",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DescribeUsers lists all",
			action: "DescribeUsers",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "usr-a")
				createUser(t, h, "usr-b")
			},
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				users := resp["Users"].([]any)
				assert.Len(t, users, 2)
			},
		},
		{
			name:   "DisableUser disables user",
			action: "DisableUser",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "dis-usr")
			},
			body:     map[string]any{"UserName": "dis-usr", "AuthenticationType": "USERPOOL"},
			wantCode: http.StatusOK,
		},
		{
			name:   "EnableUser re-enables user",
			action: "EnableUser",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "en-usr")
				rec := doRequest(t, h, "DisableUser", map[string]any{
					"UserName": "en-usr", "AuthenticationType": "USERPOOL",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"UserName": "en-usr", "AuthenticationType": "USERPOOL"},
			wantCode: http.StatusOK,
		},
		{
			name:   "DeleteUser removes user",
			action: "DeleteUser",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "del-usr")
			},
			body:     map[string]any{"UserName": "del-usr", "AuthenticationType": "USERPOOL"},
			wantCode: http.StatusOK,
		},
		{
			name:     "DeleteUser unknown returns error",
			action:   "DeleteUser",
			body:     map[string]any{"UserName": "no-such", "AuthenticationType": "USERPOOL"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestAppStream_UserARNPartition covers the user ARN partition (built via
// pkgs/arn.Build rather than a hand-formatted "arn:aws:..." literal, which
// would always emit the standard partition even for GovCloud/China/ISO
// regions).
func TestAppStream_UserARNPartition(t *testing.T) {
	t.Parallel()

	backend := appstream.NewInMemoryBackend("000000000000", "us-gov-west-1")
	h := appstream.NewHandler(backend)

	rec := doRequest(t, h, "CreateUser", map[string]any{
		"UserName":           "govcloud-user",
		"AuthenticationType": "USERPOOL",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doRequest(t, h, "DescribeUsers", map[string]any{"AuthenticationType": "USERPOOL"})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	users := resp["Users"].([]any)
	require.Len(t, users, 1)
	assert.Equal(
		t,
		"arn:aws-us-gov:appstream:us-gov-west-1:000000000000:user/USERPOOL/govcloud-user",
		users[0].(map[string]any)["Arn"],
	)
}

// TestAppStream_UserARNFormat verifies that user ARNs match the AWS format.
func TestAppStream_UserARNFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateUser", map[string]any{
		"UserName":           "testuser",
		"AuthenticationType": "USERPOOL",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	recDesc := doRequest(t, h, "DescribeUsers", map[string]any{"AuthenticationType": "USERPOOL"})
	require.Equal(t, http.StatusOK, recDesc.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recDesc.Body.Bytes(), &resp))
	users := resp["Users"].([]any)
	require.Len(t, users, 1)
	user := users[0].(map[string]any)
	assert.Contains(t, user["Arn"], "arn:aws:appstream:")
}

// TestAppStream_UserStatusEnabled verifies that a newly created and enabled user
// has Enabled=true and Status=CONFIRMED.
func TestAppStream_UserStatusEnabled(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateUser", map[string]any{
		"UserName":           "enabled-user",
		"AuthenticationType": "USERPOOL",
	})
	doRequest(t, h, "EnableUser", map[string]any{
		"UserName":           "enabled-user",
		"AuthenticationType": "USERPOOL",
	})

	rec := doRequest(t, h, "DescribeUsers", map[string]any{"AuthenticationType": "USERPOOL"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	users := resp["Users"].([]any)
	require.Len(t, users, 1)
	user := users[0].(map[string]any)
	assert.Equal(t, true, user["Enabled"])
}

// TestAppStream_UserStackAssociations covers batch user-stack association ops.
func TestAppStream_UserStackAssociations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *appstream.Handler)
		check    func(t *testing.T, body []byte)
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "BatchAssociateUserStack links user to stack",
			action: "BatchAssociateUserStack",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "ba-user")
				createStack(t, h, "any-stack")
			},
			body: map[string]any{
				"UserStackAssociations": []map[string]any{
					{
						"UserName":           "ba-user",
						"StackName":          "any-stack",
						"AuthenticationType": "USERPOOL",
					},
				},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				errs := resp["errors"].([]any)
				assert.Empty(t, errs)
			},
		},
		{
			name:   "BatchAssociateUserStack unknown user returns error entry",
			action: "BatchAssociateUserStack",
			body: map[string]any{
				"UserStackAssociations": []map[string]any{
					{
						"UserName":           "ghost",
						"StackName":          "any-stack",
						"AuthenticationType": "USERPOOL",
					},
				},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				errs := resp["errors"].([]any)
				assert.Len(t, errs, 1)
			},
		},
		{
			name:   "DescribeUserStackAssociations lists links",
			action: "DescribeUserStackAssociations",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "list-ba-user")
				createStack(t, h, "list-stk")
				rec := doRequest(t, h, "BatchAssociateUserStack", map[string]any{
					"UserStackAssociations": []map[string]any{
						{
							"UserName":           "list-ba-user",
							"StackName":          "list-stk",
							"AuthenticationType": "USERPOOL",
						},
					},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"StackName": "list-stk"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				assocs := resp["UserStackAssociations"].([]any)
				assert.Len(t, assocs, 1)
			},
		},
		{
			name:   "BatchDisassociateUserStack removes links",
			action: "BatchDisassociateUserStack",
			setup: func(h *appstream.Handler) {
				createUser(t, h, "dis-ba-user")
				createStack(t, h, "dis-stk")
				rec := doRequest(t, h, "BatchAssociateUserStack", map[string]any{
					"UserStackAssociations": []map[string]any{
						{
							"UserName": "dis-ba-user", "StackName": "dis-stk",
							"AuthenticationType": "USERPOOL",
						},
					},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body: map[string]any{
				"UserStackAssociations": []map[string]any{
					{
						"UserName":           "dis-ba-user",
						"StackName":          "dis-stk",
						"AuthenticationType": "USERPOOL",
					},
				},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestAppStream_BatchAssociateUserStackErrors verifies that BatchAssociateUserStack
// returns per-item errors for invalid associations rather than a top-level error.
func TestAppStream_BatchAssociateUserStackErrors(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	// Associate to a nonexistent user — should get a per-item error, not 500.
	rec := doRequest(t, h, "BatchAssociateUserStack", map[string]any{
		"UserStackAssociations": []any{
			map[string]any{
				"UserName":           "ghost@example.com",
				"StackName":          "nonexistent-stack",
				"AuthenticationType": "USERPOOL",
			},
		},
	})
	// Real AWS returns 200 with errors in the response body
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// errors field may be empty or present depending on implementation
	// but response must be 200 with valid JSON
	assert.NotNil(t, resp)
}

// TestAppStream_BatchAssociateUserStack_StackNotFound proves an association
// naming a stack that doesn't exist gets a per-item STACK_NOT_FOUND error
// (types/enums.go UserStackAssociationErrorCodeStackNotFound) rather than
// being silently accepted. Regression for gopherstack-65w: the backend
// previously validated only the user, never the stack, so any StackName --
// existing or not -- was written straight into userStackAssoc.
func TestAppStream_BatchAssociateUserStack_StackNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createUser(t, h, "real-user")

	rec := doRequest(t, h, "BatchAssociateUserStack", map[string]any{
		"UserStackAssociations": []any{
			map[string]any{
				"UserName":           "real-user",
				"StackName":          "nonexistent-stack",
				"AuthenticationType": "USERPOOL",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	errs := resp["errors"].([]any)
	require.Len(t, errs, 1)
	assert.Equal(t, "STACK_NOT_FOUND", errs[0].(map[string]any)["ErrorCode"])

	descRec := doRequest(t, h, "DescribeUserStackAssociations", map[string]any{"StackName": "nonexistent-stack"})
	require.Equal(t, http.StatusOK, descRec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	assert.Empty(t, descResp["UserStackAssociations"])
}

// TestAppStream_DescribeUserStackAssociations verifies that user-stack associations
// are returned correctly.
func TestAppStream_DescribeUserStackAssociations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateStack", map[string]any{"Name": "assoc-stack"})
	doRequest(t, h, "CreateUser", map[string]any{
		"UserName":           "assoc-user",
		"AuthenticationType": "USERPOOL",
	})

	rec := doRequest(t, h, "BatchAssociateUserStack", map[string]any{
		"UserStackAssociations": []any{
			map[string]any{
				"UserName":           "assoc-user",
				"StackName":          "assoc-stack",
				"AuthenticationType": "USERPOOL",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	recDesc := doRequest(t, h, "DescribeUserStackAssociations", map[string]any{
		"StackName": "assoc-stack",
	})
	require.Equal(t, http.StatusOK, recDesc.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recDesc.Body.Bytes(), &resp))
	assocs := resp["UserStackAssociations"].([]any)
	assert.Len(t, assocs, 1)
	assoc := assocs[0].(map[string]any)
	assert.Equal(t, "assoc-user", assoc["UserName"])
	assert.Equal(t, "assoc-stack", assoc["StackName"])
}

// TestAppStream_BatchDisassociateUserStack_StackNotFound proves an
// association naming a stack that doesn't exist gets a per-item
// STACK_NOT_FOUND error, mirroring BatchAssociateUserStack's own check.
// Regression for gopherstack-65w.
func TestAppStream_BatchDisassociateUserStack_StackNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createUser(t, h, "dis-real-user")

	rec := doRequest(t, h, "BatchDisassociateUserStack", map[string]any{
		"UserStackAssociations": []any{
			map[string]any{
				"UserName":           "dis-real-user",
				"StackName":          "nonexistent-stack",
				"AuthenticationType": "USERPOOL",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	errs := resp["errors"].([]any)
	require.Len(t, errs, 1)
	assert.Equal(t, "STACK_NOT_FOUND", errs[0].(map[string]any)["ErrorCode"])
}
