package workspaces_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/workspaces/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CreateWorkspaces input validation
// ---------------------------------------------------------------------------

func TestCreateWorkspaces_EmptyList_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []any{},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateWorkspaces_MissingUserName_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{"DirectoryId": "d-abc", "BundleId": "wsb-bh8rsxt14"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "missing UserName must return 400")
}

func TestCreateWorkspaces_MissingDirectoryId_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{"UserName": "alice", "BundleId": "wsb-bh8rsxt14"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "missing DirectoryId must return 400")
}

func TestCreateWorkspaces_MissingBundleId_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{"UserName": "alice", "DirectoryId": "d-abc"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "missing BundleId must return 400")
}

func TestCreateWorkspaces_TooMany_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	specs := make([]map[string]any, 26)
	for i := range specs {
		specs[i] = map[string]any{
			"UserName":    fmt.Sprintf("user%d", i),
			"DirectoryId": "d-abc",
			"BundleId":    "wsb-bh8rsxt14",
		}
	}

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": specs,
	})
	assert.Equal(
		t,
		http.StatusBadRequest,
		rec.Code,
		"more than 25 workspaces per call must return 400",
	)
}

func TestCreateWorkspaces_MaxAllowed_Returns200(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-abc"})

	specs := make([]map[string]any, 25)
	for i := range specs {
		specs[i] = map[string]any{
			"UserName":    fmt.Sprintf("user%d", i),
			"DirectoryId": "d-abc",
			"BundleId":    "wsb-bh8rsxt14",
		}
	}

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": specs,
	})
	assert.Equal(
		t,
		http.StatusOK,
		rec.Code,
		"exactly 25 workspaces per call is the max and must succeed",
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pending := resp["PendingRequests"].([]any)
	assert.Len(t, pending, 25)
}

// ---------------------------------------------------------------------------
// WorkspaceProperties in CreateWorkspaces
// ---------------------------------------------------------------------------

func TestCreateWorkspaces_WithProperties_Stored(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-abc"})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "alice",
				"DirectoryId": "d-abc",
				"BundleId":    "wsb-bh8rsxt14",
				"WorkspaceProperties": map[string]any{
					"RunningMode":       "AUTO_STOP",
					"ComputeTypeName":   "STANDARD",
					"UserVolumeSizeGib": 50,
				},
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

	propsRaw, hasProps := ws["WorkspaceProperties"]
	assert.True(
		t,
		hasProps,
		"PendingRequests must include WorkspaceProperties when set at creation",
	)
	require.NotNil(t, propsRaw)

	props := propsRaw.(map[string]any)
	assert.Equal(t, "AUTO_STOP", props["RunningMode"])
	assert.Equal(t, "STANDARD", props["ComputeTypeName"])

	// Confirm properties also appear in DescribeWorkspaces.
	rec2 := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &descResp))
	wsList := descResp["Workspaces"].([]any)
	require.Len(t, wsList, 1)

	descWs := wsList[0].(map[string]any)
	descPropsRaw, hasDescProps := descWs["WorkspaceProperties"]
	assert.True(
		t,
		hasDescProps,
		"DescribeWorkspaces must reflect creation-time WorkspaceProperties",
	)

	descProps := descPropsRaw.(map[string]any)
	assert.Equal(t, "AUTO_STOP", descProps["RunningMode"])
}

// TestCreateWorkspaces_DefaultsRunningModeToAlwaysOn verifies that a
// WorkSpace created with no RunningMode (or WorkspaceProperties omitted
// entirely) gets ALWAYS_ON, matching CreateWorkspacesPool's precedent for
// an omitted RunningMode (pools.go's poolsRunningModeAlwaysOn).
func TestCreateWorkspaces_DefaultsRunningModeToAlwaysOn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-default1"})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "alice",
				"DirectoryId": "d-default1",
				"BundleId":    "wsb-bh8rsxt14",
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

	propsRaw, hasProps := ws["WorkspaceProperties"]
	require.True(t, hasProps, "PendingRequests must include the defaulted WorkspaceProperties")
	assert.Equal(t, "ALWAYS_ON", propsRaw.(map[string]any)["RunningMode"])

	descRec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	wsList := descResp["Workspaces"].([]any)
	require.Len(t, wsList, 1)

	descPropsRaw, hasDescProps := wsList[0].(map[string]any)["WorkspaceProperties"]
	require.True(t, hasDescProps, "DescribeWorkspaces must reflect the defaulted RunningMode")
	assert.Equal(t, "ALWAYS_ON", descPropsRaw.(map[string]any)["RunningMode"])
}

// TestCreateWorkspaces_ExplicitRunningMode_NotOverwritten verifies that an
// explicitly-set, valid RunningMode is stored as-is and not replaced by the
// ALWAYS_ON default.
func TestCreateWorkspaces_ExplicitRunningMode_NotOverwritten(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-explicit1"})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "alice",
				"DirectoryId": "d-explicit1",
				"BundleId":    "wsb-bh8rsxt14",
				"WorkspaceProperties": map[string]any{
					"RunningMode": "AUTO_STOP",
				},
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

	assert.Equal(t, "AUTO_STOP", ws["WorkspaceProperties"].(map[string]any)["RunningMode"])

	descRec := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	wsList := descResp["Workspaces"].([]any)
	require.Len(t, wsList, 1)
	assert.Equal(t, "AUTO_STOP", wsList[0].(map[string]any)["WorkspaceProperties"].(map[string]any)["RunningMode"],
		"explicit RunningMode must not be overwritten by the ALWAYS_ON default")
}

// TestCreateWorkspaces_InvalidRunningMode_FailsPerItem verifies that an
// unrecognized RunningMode surfaces as a per-item FailedCreateWorkspaceRequest
// (CreateWorkspaces is a partial-failure batch op), not a top-level error and
// not silent acceptance. MANUAL is included: it's WorkSpaces Core-only per
// CreateWorkspaces's own doc comment ("The MANUAL running mode value is only
// supported by Amazon WorkSpaces Core... allow-listed", api_op_CreateWorkspaces.go),
// the same restriction ModifyWorkspaceProperties enforces.
func TestCreateWorkspaces_InvalidRunningMode_FailsPerItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode string
	}{
		{name: "unknown_value", mode: "TURBO_MODE"},
		{name: "manual_core_only", mode: "MANUAL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-invalid1"})

			rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
				"Workspaces": []map[string]any{
					{
						"UserName":    "alice",
						"DirectoryId": "d-invalid1",
						"BundleId":    "wsb-bh8rsxt14",
						"WorkspaceProperties": map[string]any{
							"RunningMode": tc.mode,
						},
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code, "batch op must return 200 even with a failed item")

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			pending, _ := resp["PendingRequests"].([]any)
			assert.Empty(t, pending, "the invalid item must not be created")

			failed, ok := resp["FailedRequests"].([]any)
			require.True(t, ok)
			require.Len(t, failed, 1)

			item := failed[0].(map[string]any)
			assert.Equal(t, "InvalidParameterValuesException", item["ErrorCode"])
		})
	}
}

// ---------------------------------------------------------------------------
// SubnetId propagation
// ---------------------------------------------------------------------------

func TestCreateWorkspaces_SubnetId_Propagated(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{"DirectoryId": "d-abc"})

	rec := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{
				"UserName":    "alice",
				"DirectoryId": "d-abc",
				"BundleId":    "wsb-bh8rsxt14",
				"SubnetId":    "subnet-12345678",
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

	// Confirm SubnetId in DescribeWorkspaces.
	rec2 := doTargetRequest(t, h, "DescribeWorkspaces", map[string]any{
		"WorkspaceIds": []string{wsID},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &descResp))
	wsList := descResp["Workspaces"].([]any)
	require.Len(t, wsList, 1)
	assert.Equal(t, "subnet-12345678", wsList[0].(map[string]any)["SubnetId"])
}

// ---------------------------------------------------------------------------
// ModifyWorkspaceProperties validation
// ---------------------------------------------------------------------------

func TestModifyWorkspaceProperties_InvalidComputeType_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)
	rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
		"WorkspaceId": wsID,
		"WorkspaceProperties": map[string]any{
			"ComputeTypeName": "GIGACORP_TURBO",
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "unknown compute type must return 400")
}

func TestModifyWorkspaceProperties_InvalidRunningMode_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)
	rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
		"WorkspaceId": wsID,
		"WorkspaceProperties": map[string]any{
			"RunningMode": "TURBO_MODE",
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "unknown running mode must return 400")
}

func TestModifyWorkspaceProperties_ValidComputeTypes_Accept(t *testing.T) {
	t.Parallel()

	validTypes := []string{
		"VALUE", "STANDARD", "PERFORMANCE", "POWER",
		"GRAPHICS", "GRAPHICSPRO", "POWERPRO",
		"GRAPHICS_G4DN", "GRAPHICSPRO_G4DN",
	}

	for _, ct := range validTypes {
		t.Run(ct, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			wsID := createWorkspace(t, h)
			rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
				"WorkspaceId": wsID,
				"WorkspaceProperties": map[string]any{
					"ComputeTypeName": ct,
				},
			})
			assert.Equal(t, http.StatusOK, rec.Code, "ComputeTypeName %q must be accepted", ct)
		})
	}
}

// Anti-drift: every value the pinned SDK's types.Compute enum knows about
// must be accepted, so a hand-maintained allowlist can't fall behind again.
func TestModifyWorkspaceProperties_EverySDKComputeTypeAccepted(t *testing.T) {
	t.Parallel()

	for _, ct := range sdktypes.Compute("").Values() {
		t.Run(string(ct), func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			wsID := createWorkspace(t, h)
			rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
				"WorkspaceId": wsID,
				"WorkspaceProperties": map[string]any{
					"ComputeTypeName": string(ct),
				},
			})
			assert.Equal(t, http.StatusOK, rec.Code, "SDK ComputeTypeName %q must be accepted", ct)
		})
	}
}

func TestModifyWorkspaceProperties_ValidRunningModes_Accept(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"ALWAYS_ON", "AUTO_STOP"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			wsID := createWorkspace(t, h)
			rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
				"WorkspaceId": wsID,
				"WorkspaceProperties": map[string]any{
					"RunningMode": mode,
				},
			})
			assert.Equal(t, http.StatusOK, rec.Code, "RunningMode %q must be accepted", mode)
		})
	}
}

func TestModifyWorkspaceProperties_AutoStopTimeout_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		timeout  int
		wantCode int
	}{
		{name: "60_accepted", timeout: 60, wantCode: http.StatusOK},
		{name: "120_accepted", timeout: 120, wantCode: http.StatusOK},
		{name: "600_accepted", timeout: 600, wantCode: http.StatusOK},
		{name: "30_rejected", timeout: 30, wantCode: http.StatusBadRequest},
		{name: "601_rejected", timeout: 601, wantCode: http.StatusBadRequest},
		{name: "90_not_multiple_of_60_rejected", timeout: 90, wantCode: http.StatusBadRequest},
		{name: "0_accepted_no_op", timeout: 0, wantCode: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			wsID := createWorkspace(t, h)
			rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
				"WorkspaceId": wsID,
				"WorkspaceProperties": map[string]any{
					"RunningModeAutoStopTimeoutInMinutes": tc.timeout,
				},
			})
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestModifyWorkspaceProperties_InvalidComputeType_MessageIsDescriptive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	wsID := createWorkspace(t, h)

	rec := doTargetRequest(t, h, "ModifyWorkspaceProperties", map[string]any{
		"WorkspaceId": wsID,
		"WorkspaceProperties": map[string]any{
			"ComputeTypeName": "NOT_A_REAL_TYPE",
		},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "NOT_A_REAL_TYPE",
		"message should describe the actual invalid value, not just the error code")
	assert.NotContains(t, resp["message"], "%!(EXTRA")
}
