package workspaces_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workspaces"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func createWorkspaceWithSpec(t *testing.T, h *workspaces.Handler, userID, dirID string) string {
	t.Helper()

	// Ensure the directory is registered; ignore duplicate-registration errors.
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{
		"DirectoryId": dirID,
	})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    userID,
				"DirectoryId": dirID,
				"BundleId":    "wsb-bh8rsxt14",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pending, _ := resp["PendingRequests"].([]any)
	require.Len(t, pending, 1)

	return pending[0].(map[string]any)["WorkspaceId"].(string)
}

func describeWorkspacesPage(
	t *testing.T, h *workspaces.Handler, nextToken string, limit int,
) ([]string, string) {
	t.Helper()

	body := map[string]any{}
	if nextToken != "" {
		body["NextToken"] = nextToken
	}

	if limit > 0 {
		body["Limit"] = limit
	}

	rec := doTargetRequest(t, h, "DescribeWorkspaces", body)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	wsList, _ := resp["Workspaces"].([]any)
	ids := make([]string, 0, len(wsList))

	for _, w := range wsList {
		ids = append(ids, w.(map[string]any)["WorkspaceId"].(string))
	}

	nextPage, _ := resp["NextToken"].(string)

	return ids, nextPage
}

// ---------------------------------------------------------------------------
// CreateWorkspace / registered directory requirement
// ---------------------------------------------------------------------------

// TestCreateWorkspace_RequiresRegisteredDirectory verifies that CreateWorkspace
// returns an error for an unregistered directory and succeeds after registration.
func TestCreateWorkspace_RequiresRegisteredDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dirID     string
		register  bool
		wantError bool
	}{
		{
			name:      "unregistered directory returns error",
			dirID:     "d-unregistered",
			register:  false,
			wantError: true,
		},
		{
			name:      "registered directory succeeds",
			dirID:     "d-registered",
			register:  true,
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := workspaces.NewInMemoryBackend("000000000000", "us-east-1")

			if tc.register {
				require.NoError(t, b.RegisterWorkspaceDirectory(tc.dirID, nil, nil))
			}

			_, err := b.CreateWorkspace(context.Background(), &workspaces.WorkspaceCreationSpec{
				UserName:    "alice",
				DirectoryID: tc.dirID,
				BundleID:    "wsb-bh8rsxt14",
			})

			if tc.wantError {
				assert.Error(t, err, "unregistered directory must return error")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDescribeWorkspaces_FiltersByRegion verifies that workspaces created in
// one region are not returned when describing from another region.
func TestDescribeWorkspaces_FiltersByRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		createRegion string
		queryRegion  string
		wantCount    int
	}{
		{
			name:         "same region returns workspace",
			createRegion: "us-east-1",
			queryRegion:  "us-east-1",
			wantCount:    1,
		},
		{
			name:         "different region returns nothing",
			createRegion: "us-east-1",
			queryRegion:  "eu-west-1",
			wantCount:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := workspaces.NewInMemoryBackend("000000000000", tc.createRegion)
			require.NoError(t, b.RegisterWorkspaceDirectory("d-test", nil, nil))

			createCtx := ctxWithRegion(tc.createRegion)
			_, err := b.CreateWorkspace(createCtx, &workspaces.WorkspaceCreationSpec{
				UserName:    "alice",
				DirectoryID: "d-test",
				BundleID:    "wsb-bh8rsxt14",
			})
			require.NoError(t, err)

			queryCtx := ctxWithRegion(tc.queryRegion)
			wsList, _, err := b.DescribeWorkspaces(queryCtx, nil, nil, nil, nil, 0, "")
			require.NoError(t, err)
			assert.Len(t, wsList, tc.wantCount)
		})
	}
}

// ---------------------------------------------------------------------------
// WorkspaceId format
// ---------------------------------------------------------------------------

// TestWorkspaceIDFormat verifies that created workspace IDs match the AWS
// pattern: "ws-" followed by exactly 8 lowercase hex characters.
func TestWorkspaceIDFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	assert.True(t, strings.HasPrefix(wsID, "ws-"), "WorkspaceId must start with ws-, got %q", wsID)

	hexPart := strings.TrimPrefix(wsID, "ws-")
	assert.Len(
		t,
		hexPart,
		8,
		"WorkspaceId hex suffix must be 8 chars, got %d in %q",
		len(hexPart),
		wsID,
	)

	for _, ch := range hexPart {
		assert.True(t, (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'),
			"WorkspaceId hex chars must be lowercase hex, got %q in %q", string(ch), wsID)
	}
}

// ---------------------------------------------------------------------------
// StopWorkspaces state transition: AVAILABLE -> STOPPED
// ---------------------------------------------------------------------------

// TestStopWorkspaces_TransitionsToStopped verifies that after StopWorkspaces
// the workspace state changes to STOPPED.
func TestStopWorkspaces_TransitionsToStopped(t *testing.T) {
	t.Parallel()

	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h := workspaces.NewHandler(backend)
	wsID := createStartStopEligibleWorkspace(t, h)

	assert.Equal(
		t,
		"AVAILABLE",
		workspaces.WorkspaceState(backend, wsID),
		"initial state must be AVAILABLE",
	)

	rec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	assert.Empty(t, failures)

	assert.Equal(t, "STOPPED", workspaces.WorkspaceState(backend, wsID),
		"workspace state must be STOPPED after StopWorkspaces")
}

// TestStopWorkspaces_StateVisibleInDescribe verifies that the STOPPED state
// is reflected in DescribeWorkspaces.
func TestStopWorkspaces_StateVisibleInDescribe(t *testing.T) {
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

	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList, _ := resp["Workspaces"].([]any)
	require.Len(t, wsList, 1)
	ws := wsList[0].(map[string]any)
	assert.Equal(t, "STOPPED", ws["State"], "DescribeWorkspaces must reflect STOPPED state")
}

// ---------------------------------------------------------------------------
// StartWorkspaces state transition: STOPPED -> AVAILABLE
// ---------------------------------------------------------------------------

// TestStartWorkspaces_ResumesFromStopped verifies that after StartWorkspaces
// a STOPPED workspace returns to AVAILABLE.
func TestStartWorkspaces_ResumesFromStopped(t *testing.T) {
	t.Parallel()

	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h := workspaces.NewHandler(backend)
	wsID := createStartStopEligibleWorkspace(t, h)

	doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, "STOPPED", workspaces.WorkspaceState(backend, wsID))

	rec := doTargetRequest(t, h, "StartWorkspaces", map[string]any{
		"StartWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	assert.Empty(t, failures)

	assert.Equal(t, "AVAILABLE", workspaces.WorkspaceState(backend, wsID),
		"workspace state must return to AVAILABLE after StartWorkspaces")
}

// ---------------------------------------------------------------------------
// Stop/Start state preconditions
// ---------------------------------------------------------------------------

// TestStopWorkspaces_AlreadyStopped_Fails verifies real AWS's documented
// precondition ("You cannot stop a WorkSpace unless ... a state of AVAILABLE,
// IMPAIRED, UNHEALTHY, or ERROR", api_op_StopWorkspaces.go doc comment):
// STOPPED is not in that list, so stopping an already-STOPPED workspace must
// report a per-item failure, not silently succeed.
func TestStopWorkspaces_AlreadyStopped_Fails(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createStartStopEligibleWorkspace(t, h)

	firstStopRec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, firstStopRec.Code)

	var firstStopResp map[string]any
	require.NoError(t, json.Unmarshal(firstStopRec.Body.Bytes(), &firstStopResp))
	firstStopFailures, _ := firstStopResp["FailedRequests"].([]any)
	require.Empty(t, firstStopFailures, "first StopWorkspaces call must succeed to reach STOPPED")

	rec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	require.Len(t, failures, 1, "stopping an already-STOPPED workspace must fail")
	failed := failures[0].(map[string]any)
	assert.Equal(t, wsID, failed["WorkspaceId"])
	assert.Equal(t, "InvalidResourceStateException", failed["ErrorCode"])
}

// TestStartWorkspaces_AlreadyAvailable_Fails verifies real AWS's documented
// precondition ("You cannot start a WorkSpace unless ... a state of STOPPED",
// api_op_StartWorkspaces.go doc comment): starting an already-AVAILABLE
// workspace must report a per-item failure, not silently succeed.
func TestStartWorkspaces_AlreadyAvailable_Fails(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "StartWorkspaces", map[string]any{
		"StartWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	require.Len(t, failures, 1, "starting an already-AVAILABLE workspace must fail")
	failed := failures[0].(map[string]any)
	assert.Equal(t, wsID, failed["WorkspaceId"])
	assert.Equal(t, "InvalidResourceStateException", failed["ErrorCode"])
}

// ---------------------------------------------------------------------------
// Stop/Start running-mode precondition (gopherstack-3b8k)
// ---------------------------------------------------------------------------

// TestStopWorkspaces_AlwaysOnRunningMode_Fails verifies the running-mode half
// of StopWorkspaces's documented precondition ("You cannot stop a WorkSpace
// unless it has a running mode of AutoStop or Manual and a state of
// AVAILABLE, IMPAIRED, UNHEALTHY, or ERROR", api_op_StopWorkspaces.go doc
// comment): an ALWAYS_ON workspace is in an accepted state (AVAILABLE) but
// the wrong running mode, so StopWorkspaces must still report a per-item
// failure and leave the workspace running.
func TestStopWorkspaces_AlwaysOnRunningMode_Fails(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	wsID := createWorkspaceWithRunningMode(t, h, "ALWAYS_ON")

	rec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	require.Len(t, failures, 1,
		"stopping an ALWAYS_ON workspace must fail even though its state is AVAILABLE")
	failed := failures[0].(map[string]any)
	assert.Equal(t, wsID, failed["WorkspaceId"])
	assert.Equal(t, "InvalidResourceStateException", failed["ErrorCode"])

	assert.Equal(t, "AVAILABLE", workspaces.WorkspaceState(backend, wsID),
		"a rejected StopWorkspaces call must not change workspace state")
}

// TestStartWorkspaces_AlwaysOnRunningMode_Fails verifies the running-mode
// half of StartWorkspaces's documented precondition ("You cannot start a
// WorkSpace unless it has a running mode of AutoStop or Manual and a state
// of STOPPED", api_op_StartWorkspaces.go doc comment): an ALWAYS_ON
// workspace forced into STOPPED state (via the test-only SetWorkspaceState,
// since a real ALWAYS_ON workspace can never reach STOPPED through
// StopWorkspaces itself, and ModifyWorkspaceState only supports
// AVAILABLE/ADMIN_MAINTENANCE) is in an accepted state but the wrong running
// mode, so StartWorkspaces must still report a per-item failure and leave it
// stopped.
func TestStartWorkspaces_AlwaysOnRunningMode_Fails(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	wsID := createWorkspaceWithRunningMode(t, h, "ALWAYS_ON")

	workspaces.SetWorkspaceState(backend, wsID, "STOPPED")
	require.Equal(t, "STOPPED", workspaces.WorkspaceState(backend, wsID))

	rec := doTargetRequest(t, h, "StartWorkspaces", map[string]any{
		"StartWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	require.Len(t, failures, 1,
		"starting an ALWAYS_ON workspace must fail even though its state is STOPPED")
	failed := failures[0].(map[string]any)
	assert.Equal(t, wsID, failed["WorkspaceId"])
	assert.Equal(t, "InvalidResourceStateException", failed["ErrorCode"])

	assert.Equal(t, "STOPPED", workspaces.WorkspaceState(backend, wsID),
		"a rejected StartWorkspaces call must not change workspace state")
}

// TestStopStartWorkspaces_AutoStopRunningMode_Succeeds verifies that an
// AUTO_STOP workspace -- one of the two documented eligible running modes --
// can actually be stopped and started, pinning the success side of the same
// precondition the two tests above pin the failure side of.
func TestStopStartWorkspaces_AutoStopRunningMode_Succeeds(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandlerWithBackend(t)
	wsID := createWorkspaceWithRunningMode(t, h, "AUTO_STOP")

	stopRec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, stopRec.Code)

	var stopResp map[string]any
	require.NoError(t, json.Unmarshal(stopRec.Body.Bytes(), &stopResp))
	stopFailures, _ := stopResp["FailedRequests"].([]any)
	assert.Empty(t, stopFailures, "AUTO_STOP workspace must be stoppable")
	require.Equal(t, "STOPPED", workspaces.WorkspaceState(backend, wsID))

	startRec := doTargetRequest(t, h, "StartWorkspaces", map[string]any{
		"StartWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	var startResp map[string]any
	require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &startResp))
	startFailures, _ := startResp["FailedRequests"].([]any)
	assert.Empty(t, startFailures, "AUTO_STOP workspace must be startable")
	assert.Equal(t, "AVAILABLE", workspaces.WorkspaceState(backend, wsID))
}

// ---------------------------------------------------------------------------
// Unknown workspace failures for Stop/Start
// ---------------------------------------------------------------------------

// TestStopWorkspaces_UnknownID_ReturnsFailure verifies that StopWorkspaces on
// a non-existent workspace returns a FailedRequest.
func TestStopWorkspaces_UnknownID_ReturnsFailure(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTargetRequest(t, h, "StopWorkspaces", map[string]any{
		"StopWorkspaceRequests": []map[string]any{{"WorkspaceId": "ws-notexist"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	assert.Len(t, failures, 1, "unknown workspace must produce one FailedRequest")

	failed := failures[0].(map[string]any)
	assert.Equal(t, "ws-notexist", failed["WorkspaceId"])
	assert.NotEmpty(t, failed["ErrorCode"])
}

// TestStartWorkspaces_UnknownID_ReturnsFailure verifies that StartWorkspaces
// on a non-existent workspace returns a FailedRequest.
func TestStartWorkspaces_UnknownID_ReturnsFailure(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTargetRequest(t, h, "StartWorkspaces", map[string]any{
		"StartWorkspaceRequests": []map[string]any{{"WorkspaceId": "ws-notexist"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	failures, _ := resp["FailedRequests"].([]any)
	assert.Len(t, failures, 1, "unknown workspace must produce one FailedRequest")
}

// ---------------------------------------------------------------------------
// ModifyWorkspaceProperties persistence
// ---------------------------------------------------------------------------

// TestModifyWorkspaceProperties_Persisted verifies that properties set via
// ModifyWorkspaceProperties are persisted internally.
func TestModifyWorkspaceProperties_Persisted(t *testing.T) {
	t.Parallel()

	backend := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h := workspaces.NewHandler(backend)
	wsID := createWorkspace(t, h)

	preModify := workspaces.WorkspaceProps(backend, wsID)
	require.NotNil(t, preModify, "CreateWorkspaces must default RunningMode, not leave properties nil")
	assert.Equal(t, "ALWAYS_ON", preModify.RunningMode, "unspecified RunningMode must default to ALWAYS_ON")
	assert.Empty(t, preModify.ComputeTypeName, "ComputeTypeName must stay unset before modify")

	rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
		"WorkspaceId": wsID,
		"WorkspaceProperties": map[string]any{
			"RunningMode":       "AUTO_STOP",
			"ComputeTypeName":   "STANDARD",
			"UserVolumeSizeGib": 50,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	props := workspaces.WorkspaceProps(backend, wsID)
	require.NotNil(t, props, "properties must be stored after ModifyWorkspaceProperties")
	assert.Equal(t, "AUTO_STOP", props.RunningMode)
	assert.Equal(t, "STANDARD", props.ComputeTypeName)
	assert.Equal(t, int32(50), props.UserVolumeSizeGib)
}

// TestModifyWorkspaceProperties_VisibleInDescribe verifies that updated
// properties appear in DescribeWorkspaces under WorkspaceProperties.
func TestModifyWorkspaceProperties_VisibleInDescribe(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
		"WorkspaceId": wsID,
		"WorkspaceProperties": map[string]any{
			"RunningMode":     "AUTO_STOP",
			"ComputeTypeName": "VALUE",
		},
	})

	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList, _ := resp["Workspaces"].([]any)
	require.Len(t, wsList, 1)

	ws := wsList[0].(map[string]any)
	propsRaw, ok := ws["WorkspaceProperties"]
	require.True(t, ok, "WorkspaceProperties must be present in DescribeWorkspaces after modify")
	require.NotNil(t, propsRaw)

	props := propsRaw.(map[string]any)
	assert.Equal(t, "AUTO_STOP", props["RunningMode"])
	assert.Equal(t, "VALUE", props["ComputeTypeName"])
}

// TestWorkspaceProperties_DefaultRunningModeBeforeModify verifies that
// DescribeWorkspaces reports WorkspaceProperties.RunningMode as ALWAYS_ON
// (CreateWorkspace's default) before any ModifyWorkspaceProperties call,
// not an absent WorkspaceProperties block.
func TestWorkspaceProperties_DefaultRunningModeBeforeModify(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList, _ := resp["Workspaces"].([]any)
	require.Len(t, wsList, 1)

	ws := wsList[0].(map[string]any)
	propsRaw, hasProps := ws["WorkspaceProperties"]
	require.True(t, hasProps, "WorkspaceProperties must be present with the defaulted RunningMode")
	props := propsRaw.(map[string]any)
	assert.Equal(t, "ALWAYS_ON", props["RunningMode"])
}

// ---------------------------------------------------------------------------
// TerminateWorkspaces removes from DescribeWorkspaces
// ---------------------------------------------------------------------------

// TestTerminateWorkspaces_RemovedFromDescribe verifies that terminated
// workspaces no longer appear in DescribeWorkspaces.
func TestTerminateWorkspaces_RemovedFromDescribe(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	doTargetRequest(t, h, "TerminateWorkspaces", map[string]any{
		"TerminateWorkspaceRequests": []map[string]any{{"WorkspaceId": wsID}},
	})

	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList, _ := resp["Workspaces"].([]any)
	assert.Empty(t, wsList, "terminated workspace must not appear in DescribeWorkspaces")
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

func TestPagination_Limit1(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create 3 workspaces so we can paginate through them.
	createdIDs := make([]string, 0, 3)
	for i := range 3 {
		id := createWorkspaceWithSpec(t, h, fmt.Sprintf("user%d", i), "d-abc123")
		createdIDs = append(createdIDs, id)
	}

	sort.Strings(createdIDs)

	// First page: limit=1 -> one result, token present.
	page1, token1 := describeWorkspacesPage(t, h, "", 1)
	require.Len(t, page1, 1)
	assert.NotEmpty(t, token1, "NextToken must be set when there are more results")
	assert.Equal(t, createdIDs[0], page1[0])

	// Second page: continue from token1 -> second result, token present.
	page2, token2 := describeWorkspacesPage(t, h, token1, 1)
	require.Len(t, page2, 1)
	assert.NotEmpty(t, token2)
	assert.Equal(t, createdIDs[1], page2[0])

	// Third page: continue from token2 -> last result, no token.
	page3, token3 := describeWorkspacesPage(t, h, token2, 1)
	require.Len(t, page3, 1)
	assert.Empty(t, token3, "NextToken must be absent on the last page")
	assert.Equal(t, createdIDs[2], page3[0])
}

func TestPagination_DefaultLimit25(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create exactly 26 workspaces to trigger pagination.
	for i := range 26 {
		createWorkspaceWithSpec(t, h, fmt.Sprintf("user%d", i), "d-abc123")
	}

	// First page: no explicit limit -> defaults to 25.
	page1, token1 := describeWorkspacesPage(t, h, "", 0)
	assert.Len(t, page1, 25, "default page size must be 25")
	assert.NotEmpty(t, token1)

	// Second page: remaining 1 result.
	page2, token2 := describeWorkspacesPage(t, h, token1, 0)
	assert.Len(t, page2, 1)
	assert.Empty(t, token2)

	// No overlap between pages.
	combined := make([]string, 0, len(page1)+len(page2))
	combined = append(combined, page1...)
	combined = append(combined, page2...)
	seen := make(map[string]struct{})

	for _, id := range combined {
		_, already := seen[id]
		assert.False(t, already, "workspace %q appeared in both pages", id)
		seen[id] = struct{}{}
	}

	assert.Len(t, combined, 26)
}

func TestPagination_SortedByID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 5 {
		createWorkspaceWithSpec(t, h, fmt.Sprintf("user%d", i), "d-abc123")
	}

	// Collect all IDs via 5 single-item pages.
	collected := make([]string, 0, 5)
	token := ""

	for range 5 {
		page, next := describeWorkspacesPage(t, h, token, 1)
		require.Len(t, page, 1)
		collected = append(collected, page[0])
		token = next
	}

	// Verify ascending order.
	for i := 1; i < len(collected); i++ {
		assert.Less(t, collected[i-1], collected[i],
			"page results must be in ascending WorkspaceId order")
	}
}

func TestPagination_ExplicitLimitCappedAt25(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 30 {
		createWorkspaceWithSpec(t, h, fmt.Sprintf("user%d", i), "d-abc123")
	}

	// Even if the client requests limit=100, we cap at 25.
	page1, _ := describeWorkspacesPage(t, h, "", 100)
	assert.LessOrEqual(t, len(page1), 25, "limit must be capped at 25")
}

func TestPagination_FilteredByDirectoryID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createWorkspaceWithSpec(t, h, "u1", "d-aaa")
	createWorkspaceWithSpec(t, h, "u2", "d-bbb")
	createWorkspaceWithSpec(t, h, "u3", "d-aaa")

	rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"DirectoryId": "d-aaa",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wsList := resp["Workspaces"].([]any)
	assert.Len(t, wsList, 2, "filter by DirectoryId must return only matching workspaces")
}

func TestWorkspace_RealMembersPopulated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		userName string
		dirID    string
	}{
		{
			name:     "workspace members populated on describe",
			userName: "test-user-alpha",
			dirID:    "d-alpha123",
		},
		{
			name:     "workspace members populated for second user",
			userName: "test-user-beta",
			dirID:    "d-beta456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			wsID := createWorkspaceWithSpec(t, h, tt.userName, tt.dirID)

			rec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
				"WorkspaceIds": []string{wsID},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			wsList, ok := resp["Workspaces"].([]any)
			require.True(t, ok)
			require.Len(t, wsList, 1)

			ws, ok := wsList[0].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, wsID, ws["WorkspaceId"])
			// WorkspaceName is "not applicable if UserName is specified for
			// user-assigned WorkSpaces" (types.WorkspaceRequest.WorkspaceName's
			// doc comment) -- must stay absent, not fabricated from UserName.
			assert.NotContains(t, ws, "WorkspaceName")
			assert.NotEmpty(t, ws["IpAddress"])
		})
	}
}

// TestWorkspace_WorkspaceNameThreadedThrough proves CreateWorkspaces'
// WorkspaceRequest.WorkspaceName (aws-sdk-go-v2/service/workspaces@v1.73.1
// types.WorkspaceRequest.WorkspaceName, real input member for user-decoupled
// WorkSpaces) is actually stored and echoed back by DescribeWorkspaces,
// rather than accepted and silently discarded.
func TestWorkspace_WorkspaceNameThreadedThrough(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{
		"DirectoryId": "d-decoupled123",
	})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":      "[UNDEFINED]",
				"WorkspaceName": "decoupled-ws-1",
				"DirectoryId":   "d-decoupled123",
				"BundleId":      "wsb-bh8rsxt14",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	pending, _ := createResp["PendingRequests"].([]any)
	require.Len(t, pending, 1)
	pendingItem := pending[0].(map[string]any)
	assert.Equal(t, "decoupled-ws-1", pendingItem["WorkspaceName"],
		"CreateWorkspaces' PendingRequests must echo the caller-supplied WorkspaceName")
	wsID := pendingItem["WorkspaceId"].(string)

	descRec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	wsList, _ := descResp["Workspaces"].([]any)
	require.Len(t, wsList, 1)
	ws := wsList[0].(map[string]any)
	assert.Equal(t, "decoupled-ws-1", ws["WorkspaceName"],
		"DescribeWorkspaces must echo the caller-supplied WorkspaceName, not drop it")
}

func TestCreateStandbyWorkspaces_DataReplicationAndRelated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		primaryWorkspaceID string
		dataReplication    string
		dirID              string
	}{
		{
			name:               "standby workspace with primary and replication",
			primaryWorkspaceID: "ws-primary1",
			dataReplication:    "PRIMARY_AS_SOURCE",
			dirID:              "d-standby1",
		},
		{
			name:               "standby workspace with no replication",
			primaryWorkspaceID: "ws-primary2",
			dataReplication:    "NO_REPLICATION",
			dirID:              "d-standby2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{
				"DirectoryId": tt.dirID,
			})

			rec := doTargetRequest(t, h, "CreateStandbyWorkspaces", map[string]any{
				"PrimaryRegion": "us-east-1",
				"StandbyWorkspaces": []map[string]any{
					{
						"PrimaryWorkspaceId": tt.primaryWorkspaceID,
						"DataReplication":    tt.dataReplication,
						"DirectoryId":        tt.dirID,
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				FailedStandbyRequests  []any `json:"FailedStandbyRequests"`
				PendingStandbyRequests []struct {
					WorkspaceID string `json:"WorkspaceId"`
				} `json:"PendingStandbyRequests"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			require.Empty(t, out.FailedStandbyRequests)
			require.Len(t, out.PendingStandbyRequests, 1)

			standbyID := out.PendingStandbyRequests[0].WorkspaceID

			descRec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
				"WorkspaceIds": []string{standbyID},
			})
			require.Equal(t, http.StatusOK, descRec.Code)

			var descResp struct {
				Workspaces []struct {
					WorkspaceID             string `json:"WorkspaceId"`
					DataReplicationSettings *struct {
						DataReplication string `json:"DataReplication"`
					} `json:"DataReplicationSettings"`
					RelatedWorkspaces []struct {
						WorkspaceID string `json:"WorkspaceId"`
						Type        string `json:"Type"`
					} `json:"RelatedWorkspaces"`
				} `json:"Workspaces"`
			}
			require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
			require.Len(t, descResp.Workspaces, 1)

			ws := descResp.Workspaces[0]
			require.NotNil(t, ws.DataReplicationSettings)
			assert.Equal(t, tt.dataReplication, ws.DataReplicationSettings.DataReplication)
			require.Len(t, ws.RelatedWorkspaces, 1)
			assert.Equal(t, tt.primaryWorkspaceID, ws.RelatedWorkspaces[0].WorkspaceID)
			assert.Equal(t, "PRIMARY", ws.RelatedWorkspaces[0].Type)
		})
	}
}
