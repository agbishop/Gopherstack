package ssm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

// assertBodyContains checks that the response body contains the given substring.
func assertBodyContains(t *testing.T, rec *httptest.ResponseRecorder, s string) {
	t.Helper()
	assert.Contains(t, rec.Body.String(), s)
}

// TestStubOps_DeregisterTargetFromMaintenanceWindow exercises that stub (not-found returns error).
func TestStubOps_DeregisterTargetFromMaintenanceWindow(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	// Non-existent target returns 400 — verifies the handler path is exercised.
	rec := doRequest(
		t,
		h,
		"DeregisterTargetFromMaintenanceWindow",
		`{"WindowId":"mw-1234","WindowTargetId":"target-1234"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestStubOps_DeregisterTaskFromMaintenanceWindow exercises that stub (not-found returns error).
func TestStubOps_DeregisterTaskFromMaintenanceWindow(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	// Non-existent task returns 400 — verifies the handler path is exercised.
	rec := doRequest(
		t,
		h,
		"DeregisterTaskFromMaintenanceWindow",
		`{"WindowId":"mw-1234","WindowTaskId":"task-1234"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestStubOps_DescribeMaintenanceWindows exercises that stub.
func TestStubOps_DescribeMaintenanceWindows(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	rec := doRequest(t, h, "DescribeMaintenanceWindows", `{}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_UpdateMaintenanceWindowTarget exercises that stub.
func TestStubOps_UpdateMaintenanceWindowTarget(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	rec := doRequest(
		t,
		h,
		"UpdateMaintenanceWindowTarget",
		`{"WindowId":"mw-1234","WindowTargetId":"tgt-1234"}`,
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_UpdateMaintenanceWindowTask exercises that stub.
func TestStubOps_UpdateMaintenanceWindowTask(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	rec := doRequest(
		t,
		h,
		"UpdateMaintenanceWindowTask",
		`{"WindowId":"mw-1234","WindowTaskId":"task-1234"}`,
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestSSMHandler_ChaosOps verifies the chaos interface methods compile and return.
func TestSSMHandler_ChaosOps(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	assert.Equal(t, "ssm", h.ChaosServiceName())
	assert.NotEmpty(t, h.ChaosOperations())
	assert.NotEmpty(t, h.ChaosRegions())
}
func TestCreatePatchBaseline_ValidationError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantErr    string
		wantStatus int
	}{
		{
			name:       "missing_name",
			body:       `{"OperatingSystem":"WINDOWS"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			rec := doRequest(t, h, "CreatePatchBaseline", tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantErr)
		})
	}
}

// TestPatchBaseline_DefaultOS verifies default OperatingSystem.
func TestPatchBaseline_DefaultOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   string
		wantOS string
	}{
		{
			name:   "default_os_when_omitted",
			body:   `{"Name":"MyBaseline"}`,
			wantOS: "WINDOWS",
		},
		{
			name:   "explicit_linux_os",
			body:   `{"Name":"LinuxBaseline","OperatingSystem":"AMAZON_LINUX_2"}`,
			wantOS: "AMAZON_LINUX_2",
		},
		{
			name:   "explicit_windows_os",
			body:   `{"Name":"WinBaseline","OperatingSystem":"WINDOWS"}`,
			wantOS: "WINDOWS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := ssm.NewInMemoryBackend()
			rec := doRequest(t, ssm.NewHandler(backend), "CreatePatchBaseline", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp ssm.CreatePatchBaselineOutput
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.NotEmpty(t, resp.BaselineID)

			bl := backend.GetPatchBaselineInternal(resp.BaselineID)
			assert.Equal(t, tt.wantOS, bl.OperatingSystem)
		})
	}
}
func TestSSMBounds_DescribeMaintenanceWindowsForTarget(t *testing.T) {
	t.Parallel()

	_, b := newTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		maxResults *int64
		name       string
		wantError  bool
	}{
		{nil, "nil uses default", false},
		{ptr64(1), "1 is valid", false},
		{ptr64(100), "100 is valid cap", false},
		{ptr64(101), "101 exceeds cap", true},
		{ptr64(0), "0 is invalid", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := b.DescribeMaintenanceWindowsForTarget(ctx, &ssm.DescribeMaintenanceWindowsForTargetInput{
				ResourceType: "INSTANCE",
				Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-test"}}},
				MaxResults:   tc.maxResults,
			})

			if tc.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, out)
			}
		})
	}
}
func newBackend(t *testing.T) *ssm.InMemoryBackend {
	t.Helper()

	return ssm.NewInMemoryBackend()
}

// createTestBaseline creates a patch baseline and returns its ID.
func createTestBaseline(t *testing.T, b *ssm.InMemoryBackend, name string) string {
	t.Helper()

	out, err := b.CreatePatchBaseline(context.TODO(), &ssm.CreatePatchBaselineInput{
		Name:            name,
		OperatingSystem: "AMAZON_LINUX_2",
	})
	require.NoError(t, err)

	return out.BaselineID
}

// createTestWindow creates a maintenance window and returns its ID.
func createTestWindow(t *testing.T, b *ssm.InMemoryBackend) string {
	t.Helper()

	out, err := b.CreateMaintenanceWindow(context.TODO(), &ssm.CreateMaintenanceWindowInput{
		Name:     "test-window",
		Schedule: "cron(0 9 ? * MON *)",
		Duration: 2,
		Cutoff:   1,
	})
	require.NoError(t, err)

	return out.WindowID
}

// createTestOpsItem creates an OpsItem and returns its ID.
func createTestOpsItem(t *testing.T, b *ssm.InMemoryBackend) string {
	t.Helper()

	out, err := b.CreateOpsItem(context.TODO(), &ssm.CreateOpsItemInput{
		Title:       "test-item",
		Source:      "test",
		Description: "test description",
	})
	require.NoError(t, err)

	return out.OpsItemID
}
func TestBackendOps_GetMaintenanceWindow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	out, err := b.GetMaintenanceWindow(context.TODO(), &ssm.GetMaintenanceWindowInput{WindowID: wid})
	require.NoError(t, err)
	assert.Equal(t, wid, out.WindowID)
}
func TestBackendOps_DeleteMaintenanceWindow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	out, err := b.DeleteMaintenanceWindow(context.TODO(), &ssm.DeleteMaintenanceWindowInput{WindowID: wid})
	require.NoError(t, err)
	assert.Equal(t, wid, out.WindowID)

	// Should be gone.
	_, err = b.GetMaintenanceWindow(context.TODO(), &ssm.GetMaintenanceWindowInput{WindowID: wid})
	require.Error(t, err)
}
func TestBackendOps_UpdateMaintenanceWindow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	out, err := b.UpdateMaintenanceWindow(context.TODO(), &ssm.UpdateMaintenanceWindowInput{
		WindowID: wid,
		Name:     "updated-window",
	})
	require.NoError(t, err)
	assert.Equal(t, "updated-window", out.Name)
}
func TestBackendOps_GetMaintenanceWindowTask(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	taskOut, err := b.RegisterTaskWithMaintenanceWindow(context.TODO(), &ssm.RegisterTaskWithMaintenanceWindowInput{
		WindowID: wid,
		TaskArn:  "AWS-RunShellScript",
		TaskType: "RUN_COMMAND",
	})
	require.NoError(t, err)

	out, err := b.GetMaintenanceWindowTask(context.TODO(), &ssm.GetMaintenanceWindowTaskInput{
		WindowID:     wid,
		WindowTaskID: taskOut.WindowTaskID,
	})
	require.NoError(t, err)
	assert.Equal(t, taskOut.WindowTaskID, out.WindowTaskID)
}
func TestBackendOps_RegisterTargetWithMaintenanceWindow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	out, err := b.RegisterTargetWithMaintenanceWindow(context.TODO(), &ssm.RegisterTargetWithMaintenanceWindowInput{
		WindowID:     wid,
		ResourceType: "INSTANCE",
		Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-test"}}},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.WindowTargetID)
}
func TestBackendOps_RegisterTaskWithMaintenanceWindow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	out, err := b.RegisterTaskWithMaintenanceWindow(context.TODO(), &ssm.RegisterTaskWithMaintenanceWindowInput{
		WindowID: wid,
		TaskArn:  "AWS-RunShellScript",
		TaskType: "RUN_COMMAND",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.WindowTaskID)
}

// TestRegisterTaskWithMaintenanceWindow_TargetsRoundTrip locks in the fix for
// a real gap: RegisterTaskWithMaintenanceWindowInput.Targets (confirmed
// against aws-sdk-go-v2/service/ssm@v1.73.4's api_op_RegisterTaskWithMaintenanceWindow.go
// -- "the targets (either managed nodes or maintenance window targets)")
// was previously accepted by the wire shape but silently discarded, so a
// registered task could never record which nodes/window-targets it applies
// to. Also covers UpdateMaintenanceWindowTask persisting a Targets change.
func TestRegisterTaskWithMaintenanceWindow_TargetsRoundTrip(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	ctx := context.TODO()
	wid := createTestWindow(t, b)

	registered, err := b.RegisterTaskWithMaintenanceWindow(ctx, &ssm.RegisterTaskWithMaintenanceWindowInput{
		WindowID: wid,
		TaskArn:  "AWS-RunShellScript",
		TaskType: "RUN_COMMAND",
		Targets:  []ssm.WindowTarget{{Key: "WindowTargetIds", Values: []string{"wt-abc"}}},
	})
	require.NoError(t, err)

	described, err := b.DescribeMaintenanceWindowTasks(ctx, &ssm.DescribeMaintenanceWindowTasksInput{WindowID: wid})
	require.NoError(t, err)
	require.Len(t, described.Tasks, 1)
	require.Len(t, described.Tasks[0].Targets, 1)
	assert.Equal(t, "WindowTargetIds", described.Tasks[0].Targets[0].Key)
	assert.Equal(t, []string{"wt-abc"}, described.Tasks[0].Targets[0].Values)

	updated, err := b.UpdateMaintenanceWindowTask(ctx, &ssm.UpdateMaintenanceWindowTaskInput{
		WindowID:     wid,
		WindowTaskID: registered.WindowTaskID,
		Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-xyz"}}},
	})
	require.NoError(t, err)
	require.Len(t, updated.Targets, 1)
	assert.Equal(t, "InstanceIds", updated.Targets[0].Key)
}

func TestBackendOps_DeregisterTargetFromMaintenanceWindow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	targetOut, err := b.RegisterTargetWithMaintenanceWindow(
		context.TODO(),
		&ssm.RegisterTargetWithMaintenanceWindowInput{
			WindowID:     wid,
			ResourceType: "INSTANCE",
			Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-test"}}},
		},
	)
	require.NoError(t, err)

	out, err := b.DeregisterTargetFromMaintenanceWindow(context.TODO(), &ssm.DeregisterTargetFromMaintenanceWindowInput{
		WindowID:       wid,
		WindowTargetID: targetOut.WindowTargetID,
	})
	require.NoError(t, err)
	assert.NotNil(t, out)
}
func TestBackendOps_DeregisterTaskFromMaintenanceWindow(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	taskOut, err := b.RegisterTaskWithMaintenanceWindow(context.TODO(), &ssm.RegisterTaskWithMaintenanceWindowInput{
		WindowID: wid,
		TaskArn:  "AWS-RunShellScript",
		TaskType: "RUN_COMMAND",
	})
	require.NoError(t, err)

	out, err := b.DeregisterTaskFromMaintenanceWindow(context.TODO(), &ssm.DeregisterTaskFromMaintenanceWindowInput{
		WindowID:     wid,
		WindowTaskID: taskOut.WindowTaskID,
	})
	require.NoError(t, err)
	assert.NotNil(t, out)
}
func TestBackendOps_DescribeMaintenanceWindows(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	createTestWindow(t, b)

	out, err := b.DescribeMaintenanceWindows(context.TODO(), &ssm.DescribeMaintenanceWindowsInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, out.WindowIdentities)
}
func TestBackendOps_DescribeMaintenanceWindowsForTarget(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	out, err := b.DescribeMaintenanceWindowsForTarget(context.TODO(), &ssm.DescribeMaintenanceWindowsForTargetInput{
		ResourceType: "INSTANCE",
		Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-test"}}},
	})
	require.NoError(t, err)
	assert.NotNil(t, out.WindowIdentities)
}
func TestBackendOps_DescribeMaintenanceWindowTargets(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	_, err := b.RegisterTargetWithMaintenanceWindow(context.TODO(), &ssm.RegisterTargetWithMaintenanceWindowInput{
		WindowID:     wid,
		ResourceType: "INSTANCE",
		Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-test"}}},
	})
	require.NoError(t, err)

	out, err := b.DescribeMaintenanceWindowTargets(context.TODO(), &ssm.DescribeMaintenanceWindowTargetsInput{
		WindowID: wid,
	})
	require.NoError(t, err)
	assert.Len(t, out.Targets, 1)
}
func TestBackendOps_DescribeMaintenanceWindowTasks(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	_, err := b.RegisterTaskWithMaintenanceWindow(context.TODO(), &ssm.RegisterTaskWithMaintenanceWindowInput{
		WindowID: wid,
		TaskArn:  "AWS-RunShellScript",
		TaskType: "RUN_COMMAND",
	})
	require.NoError(t, err)

	out, err := b.DescribeMaintenanceWindowTasks(context.TODO(), &ssm.DescribeMaintenanceWindowTasksInput{
		WindowID: wid,
	})
	require.NoError(t, err)
	assert.Len(t, out.Tasks, 1)
}
func TestBackendOps_UpdateMaintenanceWindowTarget(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	targetOut, err := b.RegisterTargetWithMaintenanceWindow(
		context.TODO(),
		&ssm.RegisterTargetWithMaintenanceWindowInput{
			WindowID:     wid,
			ResourceType: "INSTANCE",
			Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-test"}}},
		},
	)
	require.NoError(t, err)

	out, err := b.UpdateMaintenanceWindowTarget(context.TODO(), &ssm.UpdateMaintenanceWindowTargetInput{
		WindowID:       wid,
		WindowTargetID: targetOut.WindowTargetID,
		Name:           "updated-target",
	})
	require.NoError(t, err)
	assert.Equal(t, "updated-target", out.Name)
}
func TestBackendOps_UpdateMaintenanceWindowTask(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	taskOut, err := b.RegisterTaskWithMaintenanceWindow(context.TODO(), &ssm.RegisterTaskWithMaintenanceWindowInput{
		WindowID: wid,
		TaskArn:  "AWS-RunShellScript",
		TaskType: "RUN_COMMAND",
	})
	require.NoError(t, err)

	priority := int32(5)
	out, err := b.UpdateMaintenanceWindowTask(context.TODO(), &ssm.UpdateMaintenanceWindowTaskInput{
		WindowID:     wid,
		WindowTaskID: taskOut.WindowTaskID,
		Name:         "updated-task",
		Priority:     &priority,
	})
	require.NoError(t, err)
	assert.Equal(t, "updated-task", out.Name)
	assert.Equal(t, int32(5), out.Priority)
}
func TestMaintenanceWindowsForTarget_NoMatchReturnsEmpty(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)

	body, _ := json.Marshal(map[string]any{
		"ResourceType": "INSTANCE",
		"Targets": []map[string]any{
			{"Key": "InstanceIds", "Values": []string{"i-notregistered"}},
		},
	})
	rec := doRequest(t, h, "DescribeMaintenanceWindowsForTarget", string(body))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	identities := resp["WindowIdentities"].([]any)
	assert.Empty(t, identities)
}

func TestDeleteMaintenanceWindow_CleansUpTargetsAndTasks(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	wid := createTestWindow(t, b)

	targetOut, err := b.RegisterTargetWithMaintenanceWindow(
		context.TODO(), &ssm.RegisterTargetWithMaintenanceWindowInput{
			WindowID:     wid,
			ResourceType: "INSTANCE",
			Targets:      []ssm.WindowTarget{{Key: "InstanceIds", Values: []string{"i-test"}}},
		},
	)
	require.NoError(t, err)

	taskOut, err := b.RegisterTaskWithMaintenanceWindow(
		context.TODO(), &ssm.RegisterTaskWithMaintenanceWindowInput{
			WindowID: wid,
			TaskArn:  "AWS-RunShellScript",
			TaskType: "RUN_COMMAND",
		},
	)
	require.NoError(t, err)

	_, err = b.DeleteMaintenanceWindow(context.TODO(), &ssm.DeleteMaintenanceWindowInput{WindowID: wid})
	require.NoError(t, err)

	targets, err := b.DescribeMaintenanceWindowTargets(
		context.TODO(), &ssm.DescribeMaintenanceWindowTargetsInput{WindowID: wid},
	)
	require.NoError(t, err)
	assert.Empty(t, targets.Targets,
		"target %s must not survive DeleteMaintenanceWindow", targetOut.WindowTargetID)

	tasks, err := b.DescribeMaintenanceWindowTasks(
		context.TODO(), &ssm.DescribeMaintenanceWindowTasksInput{WindowID: wid},
	)
	require.NoError(t, err)
	assert.Empty(t, tasks.Tasks,
		"task %s must not survive DeleteMaintenanceWindow", taskOut.WindowTaskID)
}
