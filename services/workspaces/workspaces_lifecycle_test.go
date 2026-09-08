package workspaces_test

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workspaces"
)

// ---------------------------------------------------------------------------
// GetWorkspacesConnectionStatus
// ---------------------------------------------------------------------------

func TestConnectionStatus_Available_IsDisconnected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "DescribeWorkspacesConnectionStatus", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	statuses := resp["WorkspacesConnectionStatus"].([]any)
	require.Len(t, statuses, 1)
	assert.Equal(t, "DISCONNECTED", statuses[0].(map[string]any)["ConnectionState"],
		"AVAILABLE workspace must have DISCONNECTED connection state")
}

func TestConnectionStatus_Stopped_IsNotConnected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createStartStopEligibleWorkspace(t, h)

	stopRec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, stopRec.Code)

	var stopResp map[string]any
	require.NoError(t, json.Unmarshal(stopRec.Body.Bytes(), &stopResp))
	stopFailures, _ := stopResp["FailedRequests"].([]any)
	require.Empty(t, stopFailures, "StopWorkspaces must succeed to reach STOPPED")

	rec := doTargetRequest(t, h, "DescribeWorkspacesConnectionStatus", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	statuses := resp["WorkspacesConnectionStatus"].([]any)
	require.Len(t, statuses, 1)
	assert.Equal(t, "NOT_CONNECTED", statuses[0].(map[string]any)["ConnectionState"],
		"STOPPED workspace must have NOT_CONNECTED connection state")
}

func TestConnectionStatus_AllWorkspaces_NoFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	id1 := createWorkspaceWithSpec(t, h, "u1", "d-aaa")
	id2 := createWorkspaceWithSpec(t, h, "u2", "d-aaa")

	// No WorkspaceIds filter -> return all.
	rec := doTargetRequest(t, h, "DescribeWorkspacesConnectionStatus", map[string]any{
		"WorkspaceIds": []string{},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	statuses := resp["WorkspacesConnectionStatus"].([]any)
	assert.Len(t, statuses, 2, "empty WorkspaceIds must return status for all workspaces")

	ids := map[string]struct{}{id1: {}, id2: {}}
	for _, s := range statuses {
		id := s.(map[string]any)["WorkspaceId"].(string)
		_, ok := ids[id]
		assert.True(t, ok, "unexpected workspace ID in connection status: %q", id)
	}
}

func TestConnectionStatus_UnknownID_Excluded(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTargetRequest(t, h, "DescribeWorkspacesConnectionStatus", map[string]any{
		"WorkspaceIds": []string{"ws-doesnotexist"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	statuses := resp["WorkspacesConnectionStatus"].([]any)
	assert.Empty(t, statuses, "unknown workspace ID must be silently excluded from status results")
}

// ---------------------------------------------------------------------------
// Reboot/Rebuild do not change state
// ---------------------------------------------------------------------------

func TestRebootWorkspaces_DoesNotChangeState(t *testing.T) {
	t.Parallel()

	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h := workspaces.NewHandler(backend)
	wsID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "RebootWorkspaces", map[string]any{
		"RebootWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, "AVAILABLE", workspaces.WorkspaceState(backend, wsID),
		"RebootWorkspaces must not change workspace state")
}

func TestRebuildWorkspaces_DoesNotChangeState(t *testing.T) {
	t.Parallel()

	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h := workspaces.NewHandler(backend)
	wsID := createStartStopEligibleWorkspace(t, h)

	// Stop first to verify the state is NOT reset on rebuild.
	doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, "STOPPED", workspaces.WorkspaceState(backend, wsID))

	rec := doTargetRequest(t, h, "RebuildWorkspaces", map[string]any{
		"RebuildWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, "STOPPED", workspaces.WorkspaceState(backend, wsID),
		"RebuildWorkspaces must not change workspace state")
}

// ---------------------------------------------------------------------------
// Reboot/Rebuild state preconditions
// ---------------------------------------------------------------------------

// TestRebootRebuildWorkspaces_StatePrecondition verifies real AWS's
// documented state preconditions ("You cannot reboot a WorkSpace unless its
// state is AVAILABLE, UNHEALTHY, or REBOOTING" /
// "You cannot rebuild a WorkSpace unless its state is AVAILABLE, ERROR,
// UNHEALTHY, STOPPED, or REBOOTING") are enforced as a per-item
// FailedRequests entry, not silently accepted.
func TestRebootRebuildWorkspaces_StatePrecondition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		target      string
		requestsKey string
		wantFailed  bool
	}{
		{
			name: "reboot rejects admin-maintenance workspace", target: "RebootWorkspaces",
			requestsKey: "RebootWorkspaceRequests", wantFailed: true,
		},
		{
			name: "rebuild rejects admin-maintenance workspace", target: "RebuildWorkspaces",
			requestsKey: "RebuildWorkspaceRequests", wantFailed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
			h := workspaces.NewHandler(backend)
			wsID := createWorkspace(t, h)

			modRec := doTargetRequest(t, h, "ModifyWorkspaceState", map[string]any{
				"WorkspaceId":    wsID,
				"WorkspaceState": "ADMIN_MAINTENANCE",
			})
			require.Equal(t, http.StatusOK, modRec.Code)

			rec := doTargetRequest(t, h, tc.target, map[string]any{
				tc.requestsKey: []map[string]any{{"WorkspaceId": wsID}},
			})
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

			var out struct {
				FailedRequests []struct {
					WorkspaceId  string `json:"WorkspaceId"` //nolint:revive,staticcheck // AWS wire casing.
					ErrorCode    string `json:"ErrorCode"`
					ErrorMessage string `json:"ErrorMessage"`
				} `json:"FailedRequests"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			if !tc.wantFailed {
				assert.Empty(t, out.FailedRequests)

				return
			}

			require.Len(t, out.FailedRequests, 1)
			assert.Equal(t, wsID, out.FailedRequests[0].WorkspaceId)
			assert.Equal(t, "OperationNotSupportedException", out.FailedRequests[0].ErrorCode)
		})
	}
}

func TestRebootWorkspaces_AllowsAvailableAndStopped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state string
		stop  bool
	}{
		{name: "available workspace is rebootable", stop: false, state: "AVAILABLE"},
		{name: "stopped workspace is not rebootable", stop: true, state: "STOPPED"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
			h := workspaces.NewHandler(backend)
			wsID := createStartStopEligibleWorkspace(t, h)

			if tc.stop {
				stopRec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
					"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
				})
				require.Equal(t, http.StatusOK, stopRec.Code)

				var stopResp map[string]any
				require.NoError(t, json.Unmarshal(stopRec.Body.Bytes(), &stopResp))
				stopFailures, _ := stopResp["FailedRequests"].([]any)
				require.Empty(t, stopFailures, "StopWorkspaces must succeed to reach STOPPED")
			}

			rec := doTargetRequest(t, h, "RebootWorkspaces", map[string]any{
				"RebootWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
			})
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

			var out struct {
				FailedRequests []struct {
					ErrorCode string `json:"ErrorCode"`
				} `json:"FailedRequests"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			if tc.stop {
				require.Len(t, out.FailedRequests, 1)
				assert.Equal(t, "OperationNotSupportedException", out.FailedRequests[0].ErrorCode)
			} else {
				assert.Empty(t, out.FailedRequests)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MigrateWorkspace
// ---------------------------------------------------------------------------

func TestMigrateWorkspace_SourceRemovedTargetCreated(t *testing.T) {
	t.Parallel()

	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h := workspaces.NewHandler(backend)
	srcID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "MigrateWorkspace", map[string]any{
		"SourceWorkspaceId": srcID,
		"BundleId":          "wsb-gm4d5tx2v",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Original workspace must no longer exist.
	assert.Empty(t, workspaces.WorkspaceState(backend, srcID),
		"source workspace must be removed after migration")

	// A new workspace must exist.
	targetID, _ := resp["TargetWorkspaceId"].(string)
	require.NotEmpty(t, targetID)
	assert.NotEqual(t, srcID, targetID)
	assert.Equal(t, "AVAILABLE", workspaces.WorkspaceState(backend, targetID))
}

func TestMigrateWorkspace_UnknownSource_Returns404(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "MigrateWorkspace", map[string]any{
		"SourceWorkspaceId": "ws-doesnotexist",
		"BundleId":          "wsb-gm4d5tx2v",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMigrateWorkspace(t *testing.T) { //nolint:paralleltest // existing issue.
	h, _ := newTestHandlerWithBackend(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-test"})

	// Create workspace
	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "carol",
				"DirectoryId": "d-test",
				"BundleId":    "wsb-old",
			},
		},
	})
	var wsOut struct {
		PendingRequests []map[string]any `json:"PendingRequests"`
	}
	decodeJSON(t, rec.Body.Bytes(), &wsOut)

	srcID := wsOut.PendingRequests[0]["WorkspaceId"].(string)

	// Migrate
	rec2 := doTargetRequest(t, h, "MigrateWorkspace", map[string]any{
		"SourceWorkspaceId": srcID,
		"BundleId":          "wsb-new",
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("migrate: expected 200, got %d: %s", rec2.Code, rec2.Body)
	}

	var migrateOut map[string]string
	decodeJSON(t, rec2.Body.Bytes(), &migrateOut)

	if migrateOut["SourceWorkspaceId"] != srcID {
		t.Fatalf("expected SourceWorkspaceId=%s, got %s", srcID, migrateOut["SourceWorkspaceId"])
	}

	if migrateOut["TargetWorkspaceId"] == "" {
		t.Fatal("expected non-empty TargetWorkspaceId")
	}

	if migrateOut["TargetWorkspaceId"] == srcID {
		t.Fatal("target should differ from source")
	}
}

// ---------------------------------------------------------------------------
// DescribeWorkspaces edge cases
// ---------------------------------------------------------------------------

func TestDescribeWorkspaces_EmptyList_WhenNoWorkspaces(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList := resp["Workspaces"].([]any)
	assert.Empty(t, wsList)

	_, hasToken := resp["NextToken"]
	assert.False(t, hasToken, "NextToken must be absent when there are no results")
}

func TestDescribeWorkspaces_MultipleIDs_AllReturned(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id1 := createWorkspaceWithSpec(t, h, "u1", "d-aaa")
	id2 := createWorkspaceWithSpec(t, h, "u2", "d-aaa")

	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{id1, id2},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList := resp["Workspaces"].([]any)
	assert.Len(t, wsList, 2)
}

func TestDescribeWorkspaces_FilterByUserName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createWorkspaceWithSpec(t, h, "alice", "d-aaa")
	createWorkspaceWithSpec(t, h, "bob", "d-aaa")
	createWorkspaceWithSpec(t, h, "alice", "d-aaa")

	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"UserName":    "alice",
		"DirectoryId": "d-aaa",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList := resp["Workspaces"].([]any)
	assert.Len(t, wsList, 2, "UserName filter must return only Alice's workspaces")

	for _, w := range wsList {
		assert.Equal(t, "alice", w.(map[string]any)["UserName"])
	}
}

func TestCreateWorkspaces_VolumeEncryption_Propagated(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-abc"})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":                    "alice",
				"DirectoryId":                 "d-abc",
				"BundleId":                    "wsb-bh8rsxt14",
				"VolumeEncryptionKey":         "arn:aws:kms:us-east-1:123456789012:key/abc123",
				"UserVolumeEncryptionEnabled": true,
				"RootVolumeEncryptionEnabled": true,
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	pending := createResp["PendingRequests"].([]any)
	require.Len(t, pending, 1)
	ws := pending[0].(map[string]any)
	wsID := ws["WorkspaceId"].(string)

	// VolumeEncryptionKey must be propagated in DescribeWorkspaces.
	rec2 := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &descResp))
	wsList := descResp["Workspaces"].([]any)
	require.Len(t, wsList, 1)
	descWs := wsList[0].(map[string]any)
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/abc123", descWs["VolumeEncryptionKey"])
}

// ---------------------------------------------------------------------------
// Error message formatting
// ---------------------------------------------------------------------------

func TestErrorMessages_AreNotGarbledByExtraFormatArgs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		name    string
		target  string
		body    map[string]any
		wantErr string
	}{
		{
			name:   "CreateWorkspaces empty list",
			target: "CreateWorkspaces",
			body:   map[string]any{"Workspaces": []map[string]any{}},
		},
		{
			name:   "CreateWorkspaces missing UserName",
			target: "CreateWorkspaces",
			body: map[string]any{
				"Workspaces": []map[string]any{
					{"DirectoryId": "d-1234567890", "BundleId": "wsb-bh8rsxt14"},
				},
			},
		},
		{
			name:   "CreateTags empty ResourceId",
			target: "CreateTags",
			body:   map[string]any{"ResourceId": "", "Tags": []map[string]any{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doTargetRequest(t, h, tc.target, tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]string

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotContains(t, resp["message"], "%!(EXTRA",
				"error message must not contain a garbled fmt.Sprintf EXTRA marker")
			assert.NotEmpty(t, resp["message"])
		})
	}
}

// ---------------------------------------------------------------------------
// CreateWorkspaces partial failure
// ---------------------------------------------------------------------------

func TestCreateWorkspaces_PartialFailure_DoesNotAbortWholeBatch(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{
		"DirectoryId": "d-registered000",
	})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "gooduser",
				"DirectoryId": "d-registered000",
				"BundleId":    "wsb-bh8rsxt14",
			},
			{
				"UserName":    "baduser",
				"DirectoryId": "d-not-registered",
				"BundleId":    "wsb-bh8rsxt14",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code,
		"a per-item runtime failure must not fail the whole CreateWorkspaces call")

	var resp struct {
		FailedRequests []struct {
			WorkspaceRequest struct {
				DirectoryID string `json:"DirectoryId"`
				UserName    string `json:"UserName"`
			} `json:"WorkspaceRequest"`
			ErrorCode    string `json:"ErrorCode"`
			ErrorMessage string `json:"ErrorMessage"`
		} `json:"FailedRequests"`
		PendingRequests []struct {
			WorkspaceID string `json:"WorkspaceId"`
			UserName    string `json:"UserName"`
		} `json:"PendingRequests"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.PendingRequests, 1, "the valid spec must succeed")
	assert.Equal(t, "gooduser", resp.PendingRequests[0].UserName)
	assert.NotEmpty(t, resp.PendingRequests[0].WorkspaceID)

	require.Len(t, resp.FailedRequests, 1, "the invalid spec must be reported as a failure")
	assert.Equal(t, "baduser", resp.FailedRequests[0].WorkspaceRequest.UserName)
	assert.Equal(t, "d-not-registered", resp.FailedRequests[0].WorkspaceRequest.DirectoryID)
	assert.Equal(t, "InvalidParameterValuesException", resp.FailedRequests[0].ErrorCode)
	assert.Contains(t, resp.FailedRequests[0].ErrorMessage, "d-not-registered")
}

// ---------------------------------------------------------------------------
// RestoreWorkspace / snapshots / management CIDR ranges
// ---------------------------------------------------------------------------

func TestRestoreWorkspace_UnknownID_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTargetRequest(t, h, "RestoreWorkspace", map[string]any{
		"WorkspaceId": "ws-doesnotexist",
	})

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRestoreWorkspace_KnownID_Succeeds(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "RestoreWorkspace", map[string]any{
		"WorkspaceId": wsID,
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWorkspaceLevelOps(t *testing.T) { //nolint:paralleltest // existing issue.
	h, _ := newTestHandlerWithBackend(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-test"})

	// Create a workspace
	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "bob",
				"DirectoryId": "d-test",
				"BundleId":    "wsb-src",
			},
		},
	})
	var wsOut struct {
		PendingRequests []map[string]any `json:"PendingRequests"`
	}
	decodeJSON(t, rec.Body.Bytes(), &wsOut)

	wsID := wsOut.PendingRequests[0]["WorkspaceId"].(string)

	tests := []struct {
		body map[string]any
		name string
		op   string
	}{
		{
			name: "RestoreWorkspace",
			op:   "RestoreWorkspace",
			body: map[string]any{"WorkspaceId": wsID},
		},
		{
			name: "DescribeWorkspaceSnapshots",
			op:   "DescribeWorkspaceSnapshots",
			body: map[string]any{"WorkspaceId": wsID},
		},
		{
			name: "ListAvailableManagementCidrRanges",
			op:   "ListAvailableManagementCidrRanges",
			body: map[string]any{"ManagementCidrRangeConstraint": "10.0.0.0/16"},
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			r := doTargetRequest(t, h, tc.op, tc.body)
			if r.Code != http.StatusOK {
				t.Fatalf("%s: expected 200, got %d: %s", tc.op, r.Code, r.Body)
			}
		})
	}
}

func TestCreateStandbyWorkspaces(t *testing.T) { //nolint:paralleltest // existing issue.
	h, _ := newTestHandlerWithBackend(t)

	// DirectoryId must be registered for a standby WorkSpace request to
	// succeed (per-item runtime validation, matching CreateWorkspaces).
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-test"})

	rec := doTargetRequest(t, h, "CreateStandbyWorkspaces", map[string]any{
		"PrimaryRegion": "us-east-1",
		"StandbyWorkspaces": []map[string]any{
			{"PrimaryWorkspaceId": "ws-000001", "DirectoryId": "d-test"},
			{"PrimaryWorkspaceId": "ws-000002", "DirectoryId": "d-test"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var out struct {
		FailedStandbyRequests  []any `json:"FailedStandbyRequests"`
		PendingStandbyRequests []any `json:"PendingStandbyRequests"`
	}
	decodeJSON(t, rec.Body.Bytes(), &out)

	if len(out.FailedStandbyRequests) != 0 {
		t.Fatalf("expected no failures, got %d", len(out.FailedStandbyRequests))
	}

	if len(out.PendingStandbyRequests) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(out.PendingStandbyRequests))
	}
}

// TestCreateStandbyWorkspaces_PartialFailure verifies AWS's batch
// partial-failure semantics: a runtime failure for one StandbyWorkspace
// request (an unregistered DirectoryId) is reported in FailedStandbyRequests
// -- echoing back the original StandbyWorkspaceRequest -- without aborting
// the rest of the batch, matching CreateWorkspaces. Previously this op was a
// stub that always reported zero failures regardless of input.
func TestCreateStandbyWorkspaces_PartialFailure(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-good"})

	rec := doTargetRequest(t, h, "CreateStandbyWorkspaces", map[string]any{
		"PrimaryRegion": "us-east-1",
		"StandbyWorkspaces": []map[string]any{
			{"PrimaryWorkspaceId": "ws-000001", "DirectoryId": "d-good"},
			{"PrimaryWorkspaceId": "ws-000002", "DirectoryId": "d-unregistered"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var out struct {
		FailedStandbyRequests []struct {
			StandbyWorkspaceRequest struct {
				DirectoryID        string `json:"DirectoryId"`
				PrimaryWorkspaceID string `json:"PrimaryWorkspaceId"`
			} `json:"StandbyWorkspaceRequest"`
			ErrorCode    string `json:"ErrorCode"`
			ErrorMessage string `json:"ErrorMessage"`
		} `json:"FailedStandbyRequests"`
		PendingStandbyRequests []map[string]any `json:"PendingStandbyRequests"`
	}
	decodeJSON(t, rec.Body.Bytes(), &out)

	if len(out.PendingStandbyRequests) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(out.PendingStandbyRequests))
	}

	if len(out.FailedStandbyRequests) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(out.FailedStandbyRequests))
	}

	failed := out.FailedStandbyRequests[0]
	if failed.ErrorCode != "InvalidParameterValuesException" {
		t.Fatalf("expected InvalidParameterValuesException, got %s", failed.ErrorCode)
	}

	if failed.StandbyWorkspaceRequest.DirectoryID != "d-unregistered" {
		t.Fatalf(
			"expected echoed DirectoryId=d-unregistered, got %s",
			failed.StandbyWorkspaceRequest.DirectoryID,
		)
	}

	if failed.StandbyWorkspaceRequest.PrimaryWorkspaceID != "ws-000002" {
		t.Fatalf(
			"expected echoed PrimaryWorkspaceId=ws-000002, got %s",
			failed.StandbyWorkspaceRequest.PrimaryWorkspaceID,
		)
	}
}

// TestCreateStandbyWorkspaces_RequiresFields verifies whole-request shape
// validation for required fields (PrimaryRegion, a non-empty
// StandbyWorkspaces list, and each item's DirectoryId/PrimaryWorkspaceId).
func TestCreateStandbyWorkspaces_RequiresFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "missing PrimaryRegion",
			body: map[string]any{
				"StandbyWorkspaces": []map[string]any{
					{"PrimaryWorkspaceId": "ws-1", "DirectoryId": "d-1"},
				},
			},
		},
		{
			name: "empty StandbyWorkspaces",
			body: map[string]any{
				"PrimaryRegion":     "us-east-1",
				"StandbyWorkspaces": []map[string]any{},
			},
		},
		{
			name: "missing DirectoryId",
			body: map[string]any{
				"PrimaryRegion": "us-east-1",
				"StandbyWorkspaces": []map[string]any{
					{"PrimaryWorkspaceId": "ws-1"},
				},
			},
		},
		{
			name: "missing PrimaryWorkspaceId",
			body: map[string]any{
				"PrimaryRegion": "us-east-1",
				"StandbyWorkspaces": []map[string]any{
					{"DirectoryId": "d-1"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandlerWithBackend(t)

			rec := doTargetRequest(t, h, "CreateStandbyWorkspaces", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body)
			}
		})
	}
}

// TestListAvailableManagementCidrRanges verifies ManagementCidrRangeConstraint
// is enforced as required (real smithy `required` field, previously ignored
// by a stub that always returned the same 3 hardcoded ranges regardless of
// input) and that the derived ranges are real /26 sub-blocks contained within
// the given constraint.
func TestListAvailableManagementCidrRanges(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	t.Run("missing constraint is rejected", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "ListAvailableManagementCidrRanges", map[string]any{})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("invalid CIDR is rejected", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "ListAvailableManagementCidrRanges", map[string]any{
			"ManagementCidrRangeConstraint": "not-a-cidr",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("valid constraint returns contained /26 ranges", func(t *testing.T) {
		t.Parallel()

		rec := doTargetRequest(t, h, "ListAvailableManagementCidrRanges", map[string]any{
			"ManagementCidrRangeConstraint": "10.0.0.0/24",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
		}

		var out struct {
			ManagementCidrRanges []string `json:"ManagementCidrRanges"`
		}
		decodeJSON(t, rec.Body.Bytes(), &out)

		if len(out.ManagementCidrRanges) == 0 {
			t.Fatal("expected non-empty CIDR ranges")
		}

		_, constraint, err := net.ParseCIDR("10.0.0.0/24")
		if err != nil {
			t.Fatalf("test setup: %v", err)
		}

		for _, r := range out.ManagementCidrRanges {
			ip, _, parseErr := net.ParseCIDR(r)
			if parseErr != nil {
				t.Fatalf("returned range %q is not a valid CIDR: %v", r, parseErr)
			}

			if !strings.HasSuffix(r, "/26") {
				t.Fatalf("expected a /26 sub-range, got %q", r)
			}

			if !constraint.Contains(ip) {
				t.Fatalf("returned range %q is not contained in constraint 10.0.0.0/24", r)
			}
		}
	})
}
