package workspaces_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/services/workspaces"
)

// ---------------------------------------------------------------------------
// Shared test helpers
// ---------------------------------------------------------------------------

func newTestHandler(t *testing.T) *workspaces.Handler {
	t.Helper()
	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")

	return workspaces.NewHandler(backend)
}

// newTestHandlerWithBackend creates a handler and returns both handler and backend.
func newTestHandlerWithBackend(
	t *testing.T,
) (*workspaces.Handler, *workspaces.InMemoryBackend) {
	t.Helper()

	b := workspaces.NewInMemoryBackend("111122223333", "us-east-1")
	h := workspaces.NewHandler(b)

	return h, b
}

// ctxWithRegion returns a context carrying the given region via awsmeta.
func ctxWithRegion(region string) context.Context {
	return awsmeta.Set(context.Background(), &awsmeta.Metadata{Region: region})
}

func doTargetRequest(
	t *testing.T,
	h *workspaces.Handler,
	target string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte

	if body != nil {
		var marshalErr error

		bodyBytes, marshalErr = json.Marshal(body)
		require.NoError(t, marshalErr)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "WorkspacesService."+target)
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	handlerErr := h.Handler()(c)
	require.NoError(t, handlerErr)

	return rec
}

func decodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()

	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, body)
	}
}

// dispatchCheck dispatches a JSON request and checks the status code.
func dispatchCheck( //nolint:unused // existing issue.
	t *testing.T,
	h *workspaces.Handler,
	target string,
	body any,
	wantCode int,
) *httptest.ResponseRecorder {
	t.Helper()

	rec := doTargetRequest(t, h, target, body)
	if rec.Code != wantCode {
		t.Fatalf("%s: expected %d, got %d: %s", target, wantCode, rec.Code, rec.Body)
	}

	return rec
}

func createWorkspace(t *testing.T, h *workspaces.Handler) string {
	t.Helper()

	// Ensure the test directory is registered; a 200 or 400 (already exists) are both fine.
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{
		"DirectoryId": "d-1234567890",
	})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "testuser",
				"DirectoryId": "d-1234567890",
				"BundleId":    "wsb-bh8rsxt14",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	pending, ok := resp["PendingRequests"].([]any)
	require.True(t, ok, "PendingRequests missing from response")
	require.Len(t, pending, 1)

	ws, ok := pending[0].(map[string]any)
	require.True(t, ok)

	id, ok := ws["WorkspaceId"].(string)
	require.True(t, ok)

	return id
}

// createStartStopEligibleWorkspace creates a workspace with RunningMode AUTO_STOP,
// the running mode Start/StopWorkspaces require (api_op_StartWorkspaces.go /
// api_op_StopWorkspaces.go doc comments: "a running mode of AutoStop or Manual").
// Use this instead of createWorkspace in any test that needs a Start/Stop call to
// actually succeed -- createWorkspace leaves RunningMode unset, which fails that
// precondition.
func createStartStopEligibleWorkspace(t *testing.T, h *workspaces.Handler) string {
	t.Helper()

	return createWorkspaceWithRunningMode(t, h, "AUTO_STOP")
}

// createWorkspaceWithRunningMode creates a workspace with the given
// WorkspaceProperties.RunningMode.
func createWorkspaceWithRunningMode(t *testing.T, h *workspaces.Handler, runningMode string) string {
	t.Helper()

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{
		"DirectoryId": "d-1234567890",
	})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "testuser",
				"DirectoryId": "d-1234567890",
				"BundleId":    "wsb-bh8rsxt14",
				"WorkspaceProperties": map[string]any{
					"RunningMode": runningMode,
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	pending, ok := resp["PendingRequests"].([]any)
	require.True(t, ok, "PendingRequests missing from response")
	require.Len(t, pending, 1)

	ws, ok := pending[0].(map[string]any)
	require.True(t, ok)

	id, ok := ws["WorkspaceId"].(string)
	require.True(t, ok)

	return id
}

// createImage creates a real workspace image via CreateWorkspaceImage
// (backed by a real workspace, since CreateWorkspaceImage validates
// WorkspaceId) and returns its ImageId.
func createImage(t *testing.T, h *workspaces.Handler) string {
	t.Helper()

	wsID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "CreateWorkspaceImage", map[string]any{
		"Name":        "img-for-test",
		"Description": "test",
		"WorkspaceId": wsID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	id, ok := resp["ImageId"].(string)
	require.True(t, ok)

	return id
}

// idCounterSuffix parses the sequential hex counter store.go's nextID embeds
// after prefix (e.g. "wsb-00000003" -> 3), letting a test prove the
// backend's shared ID counter did or didn't advance across a call.
func idCounterSuffix(t *testing.T, id, prefix string) int64 {
	t.Helper()

	n, err := strconv.ParseInt(strings.TrimPrefix(id, prefix), 16, 64)
	require.NoError(t, err)

	return n
}

// ---------------------------------------------------------------------------
// Core dispatch/lifecycle tests
// ---------------------------------------------------------------------------

func TestWorkSpaces_Operations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *workspaces.Handler) string
		check    func(t *testing.T, body []byte)
		body     func(setupResult string) any
		name     string
		target   string
		wantCode int
	}{
		{
			name:   "CreateWorkspaces returns pending requests",
			target: "CreateWorkspaces",
			setup: func(h *workspaces.Handler) string {
				doTargetRequest(
					t,
					h,
					"RegisterWorkspaceDirectory",
					map[string]any{"DirectoryId": "d-1234567890"},
				)

				return ""
			},
			body: func(_ string) any {
				return map[string]any{
					"Workspaces": []map[string]any{
						{
							"UserName":    "alice",
							"DirectoryId": "d-1234567890",
							"BundleId":    "wsb-bh8rsxt14",
						},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				pending, ok := resp["PendingRequests"].([]any)
				require.True(t, ok)
				assert.Len(t, pending, 1)
				ws := pending[0].(map[string]any)
				assert.Contains(t, ws["WorkspaceId"], "ws-")
				assert.Equal(t, "alice", ws["UserName"])
			},
		},
		{
			name:   "DescribeWorkspaces returns created workspace",
			target: "DescribeWorkspaces",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"WorkspaceIds": []string{wsID},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				wsList, ok := resp["Workspaces"].([]any)
				require.True(t, ok)
				assert.Len(t, wsList, 1)
			},
		},
		{
			name:   "DescribeWorkspaces with no filter returns all",
			target: "DescribeWorkspaces",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body:     func(_ string) any { return map[string]any{} },
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				wsList, ok := resp["Workspaces"].([]any)
				require.True(t, ok)
				assert.Len(t, wsList, 1)
			},
		},
		{
			name:   "DescribeWorkspacesConnectionStatus returns status",
			target: "DescribeWorkspacesConnectionStatus",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"WorkspaceIds": []string{wsID},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				statuses, ok := resp["WorkspacesConnectionStatus"].([]any)
				require.True(t, ok)
				assert.Len(t, statuses, 1)
			},
		},
		{
			name:   "ModifyWorkspaceState to ADMIN_MAINTENANCE succeeds",
			target: "ModifyWorkspaceState",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"WorkspaceId":    wsID,
					"WorkspaceState": "ADMIN_MAINTENANCE",
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "ModifyWorkspaceState to invalid state returns 400",
			target: "ModifyWorkspaceState",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"WorkspaceId":    wsID,
					"WorkspaceState": "INVALID_STATE",
				}
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "ModifyWorkspaceState unknown workspace returns 404",
			target: "ModifyWorkspaceState",
			body: func(_ string) any {
				return map[string]any{
					"WorkspaceId":    "ws-doesnotexist",
					"WorkspaceState": "AVAILABLE",
				}
			},
			wantCode: http.StatusNotFound,
		},
		{
			name:   "ModifyWorkspaceProperties succeeds",
			target: "ModifyWorkspaceProperties",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"WorkspaceId": wsID,
					"WorkspaceProperties": map[string]any{
						"RunningMode": "AUTO_STOP",
					},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "ModifyWorkspaceProperties unknown workspace returns 404",
			target: "ModifyWorkspaceProperties",
			body: func(_ string) any {
				return map[string]any{
					"WorkspaceId":         "ws-doesnotexist",
					"WorkspaceProperties": map[string]any{},
				}
			},
			wantCode: http.StatusNotFound,
		},
		{
			name:   "RebootWorkspaces with known workspace returns no failures",
			target: "RebootWorkspaces",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"RebootWorkspaceRequests": []map[string]any{
						{"WorkspaceId": wsID},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				failures, ok := resp["FailedRequests"].([]any)
				require.True(t, ok)
				assert.Empty(t, failures)
			},
		},
		{
			name:   "RebootWorkspaces with unknown workspace returns failure",
			target: "RebootWorkspaces",
			body: func(_ string) any {
				return map[string]any{
					"RebootWorkspaceRequests": []map[string]any{
						{"WorkspaceId": "ws-doesnotexist"},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				failures, ok := resp["FailedRequests"].([]any)
				require.True(t, ok)
				assert.Len(t, failures, 1)
			},
		},
		{
			name:   "RebuildWorkspaces with known workspace returns no failures",
			target: "RebuildWorkspaces",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"RebuildWorkspaceRequests": []map[string]any{
						{"WorkspaceId": wsID},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				failures, ok := resp["FailedRequests"].([]any)
				require.True(t, ok)
				assert.Empty(t, failures)
			},
		},
		{
			name:   "StartWorkspaces with known stopped workspace returns no failures",
			target: "StartWorkspaces",
			setup: func(h *workspaces.Handler) string {
				wsID := createStartStopEligibleWorkspace(t, h)
				stopRec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
					"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
				})
				require.Equal(t, http.StatusOK, stopRec.Code)

				var stopResp map[string]any
				require.NoError(t, json.Unmarshal(stopRec.Body.Bytes(), &stopResp))
				stopFailures, _ := stopResp["FailedRequests"].([]any)
				require.Empty(t, stopFailures, "setup: StopWorkspaces must succeed to reach STOPPED")

				return wsID
			},
			body: func(wsID string) any {
				return map[string]any{
					"StartWorkspaceRequests": []map[string]any{
						{"WorkspaceId": wsID},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				failures, ok := resp["FailedRequests"].([]any)
				require.True(t, ok)
				assert.Empty(t, failures)
			},
		},
		{
			name:   "StopWorkspaces with known workspace returns no failures",
			target: "StopWorkspaces",
			setup: func(h *workspaces.Handler) string {
				return createStartStopEligibleWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"StopWorkspaceRequests": []map[string]any{
						{"WorkspaceId": wsID},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				failures, ok := resp["FailedRequests"].([]any)
				require.True(t, ok)
				assert.Empty(t, failures)
			},
		},
		{
			name:   "TerminateWorkspaces deletes workspace",
			target: "TerminateWorkspaces",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"TerminateWorkspaceRequests": []map[string]any{
						{"WorkspaceId": wsID},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				failures, ok := resp["FailedRequests"].([]any)
				require.True(t, ok)
				assert.Empty(t, failures)
			},
		},
		{
			name:   "TerminateWorkspaces unknown workspace returns failure",
			target: "TerminateWorkspaces",
			body: func(_ string) any {
				return map[string]any{
					"TerminateWorkspaceRequests": []map[string]any{
						{"WorkspaceId": "ws-doesnotexist"},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				failures, ok := resp["FailedRequests"].([]any)
				require.True(t, ok)
				assert.Len(t, failures, 1)
			},
		},
		{
			name:   "CreateTags applies tags to workspace",
			target: "CreateTags",
			setup: func(h *workspaces.Handler) string {
				return createWorkspace(t, h)
			},
			body: func(wsID string) any {
				return map[string]any{
					"ResourceId": wsID,
					"Tags": []map[string]any{
						{"Key": "env", "Value": "prod"},
					},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "DeleteTags removes tags from workspace",
			target: "DeleteTags",
			setup: func(h *workspaces.Handler) string {
				wsID := createWorkspace(t, h)
				doTargetRequest(t, h, "CreateTags", map[string]any{
					"ResourceId": wsID,
					"Tags":       []map[string]any{{"Key": "env", "Value": "prod"}},
				})

				return wsID
			},
			body: func(wsID string) any {
				return map[string]any{
					"ResourceId": wsID,
					"TagKeys":    []string{"env"},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "DescribeTags returns applied tags",
			target: "DescribeTags",
			setup: func(h *workspaces.Handler) string {
				wsID := createWorkspace(t, h)
				doTargetRequest(t, h, "CreateTags", map[string]any{
					"ResourceId": wsID,
					"Tags":       []map[string]any{{"Key": "env", "Value": "prod"}},
				})

				return wsID
			},
			body: func(wsID string) any {
				return map[string]any{
					"ResourceId": wsID,
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				tagList, ok := resp["TagList"].([]any)
				require.True(t, ok)
				assert.Len(t, tagList, 1)
				tag := tagList[0].(map[string]any)
				assert.Equal(t, "env", tag["Key"])
				assert.Equal(t, "prod", tag["Value"])
			},
		},
		{
			name:     "DescribeWorkspaceBundles returns bundles",
			target:   "DescribeWorkspaceBundles",
			body:     func(_ string) any { return map[string]any{} },
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				bundles, ok := resp["Bundles"].([]any)
				require.True(t, ok)
				assert.NotEmpty(t, bundles)
			},
		},
		{
			name:     "DescribeWorkspaceDirectories returns empty list",
			target:   "DescribeWorkspaceDirectories",
			body:     func(_ string) any { return map[string]any{} },
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				dirs, ok := resp["Directories"].([]any)
				require.True(t, ok)
				assert.Empty(t, dirs)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var setupResult string
			if tc.setup != nil {
				setupResult = tc.setup(h)
			}

			rec := doTargetRequest(t, h, tc.target, tc.body(setupResult))

			assert.Equal(t, tc.wantCode, rec.Code)

			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestWorkSpaces_TerminateDeletesWorkspace(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h2 := workspaces.NewHandler(backend)

	wsID := createWorkspace(t, h2)
	assert.Equal(t, 1, workspaces.WorkspaceCount(backend))

	rec := doTargetRequest(t, h2, "TerminateWorkspaces", map[string]any{
		"TerminateWorkspaceRequests": []map[string]any{
			{"WorkspaceId": wsID},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, workspaces.WorkspaceCount(backend))

	_ = h
}

func TestWorkSpaces_HandlerOpsLen(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, 91, workspaces.HandlerOpsLen(h))
}
