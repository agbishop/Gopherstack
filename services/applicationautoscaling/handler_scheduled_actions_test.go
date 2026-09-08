package applicationautoscaling_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_PutScheduledAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/my-svc", 1, 10)
	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "scale-up",
		"Schedule":            "cron(0 9 * * ? *)",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["ScheduledActionARN"], "arn:aws:autoscaling:")
	assert.Contains(t, resp["ScheduledActionARN"], "scheduledAction:")
}

func TestHandler_PutScheduledAction_Upsert(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/my-svc", 1, 10)
	base := map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "scale-up",
		"Schedule":            "cron(0 9 * * ? *)",
	}

	rec1 := doRequest(t, h, "PutScheduledAction", base)
	require.Equal(t, http.StatusOK, rec1.Code)

	base["Schedule"] = "cron(0 10 * * ? *)"
	rec2 := doRequest(t, h, "PutScheduledAction", base)
	require.Equal(t, http.StatusOK, rec2.Code)

	// Should only have one action
	descRec := doRequest(t, h, "DescribeScheduledActions", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, descRec.Code)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	actions := descResp["ScheduledActions"].([]any)
	assert.Len(t, actions, 1)
}

func TestHandler_PutScheduledAction_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "missing_namespace",
			body: map[string]any{
				"ResourceId":          "service/default/my-svc",
				"ScalableDimension":   "ecs:service:DesiredCount",
				"ScheduledActionName": "my-action",
				"Schedule":            "rate(5 minutes)",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_action_name",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/my-svc",
				"ScalableDimension": "ecs:service:DesiredCount",
				"Schedule":          "rate(5 minutes)",
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "PutScheduledAction", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_PutScheduledAction_MissingSchedule(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "my-action",
		// Schedule intentionally omitted
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 when Schedule is missing")
}

func TestHandler_PutScheduledAction_WithScalableTargetAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/my-svc", 1, 10)
	minCap := int32(2)
	maxCap := int32(20)
	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "scale-up-morning",
		"Schedule":            "cron(0 8 * * ? *)",
		"ScalableTargetAction": map[string]any{
			"MinCapacity": minCap,
			"MaxCapacity": maxCap,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify ScalableTargetAction is returned in DescribeScheduledActions
	descRec := doRequest(t, h, "DescribeScheduledActions", map[string]any{
		"ServiceNamespace": "ecs",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	actions, ok := resp["ScheduledActions"].([]any)
	require.True(t, ok)
	require.Len(t, actions, 1)

	action := actions[0].(map[string]any)
	sta, ok := action["ScalableTargetAction"].(map[string]any)
	require.True(t, ok, "expected ScalableTargetAction in response")
	assert.InDelta(t, float64(2), sta["MinCapacity"], 0)
	assert.InDelta(t, float64(20), sta["MaxCapacity"], 0)
}

func TestHandler_PutScheduledAction_StartEndTimeTimezone(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/my-svc", 1, 10)
	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "morning-scale",
		"Schedule":            "cron(0 8 * * ? *)",
		"Timezone":            "America/New_York",
		"StartTime":           1704096000, // 2024-01-01T08:00:00Z
		"EndTime":             1735632000, // 2024-12-31T08:00:00Z
	})
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doRequest(t, h, "DescribeScheduledActions", map[string]any{
		"ServiceNamespace": "ecs",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	actions, ok := resp["ScheduledActions"].([]any)
	require.True(t, ok)
	require.Len(t, actions, 1)

	action := actions[0].(map[string]any)
	assert.Equal(t, "America/New_York", action["Timezone"])
	assert.InDelta(t, 1704096000, action["StartTime"], 0.001, "expected StartTime as epoch seconds in response")
	assert.InDelta(t, 1735632000, action["EndTime"], 0.001, "expected EndTime as epoch seconds in response")
}

func TestHandler_PutScheduledAction_InvalidStartTime(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "bad-action",
		"Schedule":            "rate(1 hour)",
		"StartTime":           "not-a-timestamp",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DeleteScheduledAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		preCreate bool
		wantCode  int
	}{
		{name: "success", preCreate: true, wantCode: http.StatusOK},
		{name: "not_found", preCreate: false, wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.preCreate {
				seedTarget(t, h, "service/default/my-svc", 1, 10)
				doRequest(t, h, "PutScheduledAction", map[string]any{
					"ServiceNamespace":    "ecs",
					"ResourceId":          "service/default/my-svc",
					"ScalableDimension":   "ecs:service:DesiredCount",
					"ScheduledActionName": "scale-up",
					"Schedule":            "cron(0 9 * * ? *)",
				})
			}

			rec := doRequest(t, h, "DeleteScheduledAction", map[string]any{
				"ServiceNamespace":    "ecs",
				"ResourceId":          "service/default/my-svc",
				"ScalableDimension":   "ecs:service:DesiredCount",
				"ScheduledActionName": "scale-up",
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_DeleteScheduledAction_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "missing_action_name",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/my-svc",
				"ScalableDimension": "ecs:service:DesiredCount",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_service_namespace",
			body: map[string]any{
				"ScheduledActionName": "my-action",
				"ResourceId":          "service/default/my-svc",
				"ScalableDimension":   "ecs:service:DesiredCount",
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "DeleteScheduledAction", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_DescribeScheduledActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    string
		wantCount int
	}{
		{name: "all", filter: "", wantCount: 2},
		{name: "filtered", filter: "ecs", wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			seedTarget(t, h, "service/default/svc1", 1, 10)
			seedTargetNS(t, h, "dynamodb", "table/t1", "dynamodb:table:ReadCapacityUnits", 1, 10)
			doRequest(t, h, "PutScheduledAction", map[string]any{
				"ServiceNamespace":    "ecs",
				"ResourceId":          "service/default/svc1",
				"ScalableDimension":   "ecs:service:DesiredCount",
				"ScheduledActionName": "action-ecs",
				"Schedule":            "rate(1 hour)",
			})
			doRequest(t, h, "PutScheduledAction", map[string]any{
				"ServiceNamespace":    "dynamodb",
				"ResourceId":          "table/t1",
				"ScalableDimension":   "dynamodb:table:ReadCapacityUnits",
				"ScheduledActionName": "action-ddb",
				"Schedule":            "rate(2 hours)",
			})

			rec := doRequest(t, h, "DescribeScheduledActions", map[string]any{"ServiceNamespace": tt.filter})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			actions, ok := resp["ScheduledActions"].([]any)
			require.True(t, ok)
			assert.Len(t, actions, tt.wantCount)
		})
	}
}

func TestHandler_DescribeScheduledActions_RicherFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body      map[string]any
		name      string
		wantCount int
	}{
		{
			name:      "filter_by_resource_id",
			body:      map[string]any{"ResourceId": "service/default/svc1"},
			wantCount: 1,
		},
		{
			name:      "filter_by_action_names",
			body:      map[string]any{"ScheduledActionNames": []string{"action-ecs"}},
			wantCount: 1,
		},
		{
			name:      "no_filter_returns_all",
			body:      map[string]any{},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			seedTarget(t, h, "service/default/svc1", 1, 10)
			seedTargetNS(t, h, "dynamodb", "table/t1", "dynamodb:table:ReadCapacityUnits", 1, 10)
			doRequest(t, h, "PutScheduledAction", map[string]any{
				"ServiceNamespace":    "ecs",
				"ResourceId":          "service/default/svc1",
				"ScalableDimension":   "ecs:service:DesiredCount",
				"ScheduledActionName": "action-ecs",
				"Schedule":            "rate(1 hour)",
			})
			doRequest(t, h, "PutScheduledAction", map[string]any{
				"ServiceNamespace":    "dynamodb",
				"ResourceId":          "table/t1",
				"ScalableDimension":   "dynamodb:table:ReadCapacityUnits",
				"ScheduledActionName": "action-ddb",
				"Schedule":            "rate(2 hours)",
			})

			rec := doRequest(t, h, "DescribeScheduledActions", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			actions, ok := resp["ScheduledActions"].([]any)
			require.True(t, ok)
			assert.Len(t, actions, tt.wantCount)
		})
	}
}

func TestHandler_MaxResults_DescribeScheduledActions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/my-svc", 1, 10)
	for i := range 4 {
		doRequest(t, h, "PutScheduledAction", map[string]any{
			"ServiceNamespace":    "ecs",
			"ResourceId":          "service/default/my-svc",
			"ScalableDimension":   "ecs:service:DesiredCount",
			"ScheduledActionName": fmt.Sprintf("action-%d", i),
			"Schedule":            "rate(1 hour)",
		})
	}

	rec := doRequest(t, h, "DescribeScheduledActions", map[string]any{
		"ServiceNamespace": "ecs",
		"MaxResults":       int32(2),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	actions, ok := resp["ScheduledActions"].([]any)
	require.True(t, ok)
	assert.Len(t, actions, 2)
}

func TestScheduledActionCRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const (
		ns     = "ecs"
		res    = "service/default/batch"
		dim    = "ecs:service:DesiredCount"
		action = "scale-down-night"
	)

	seedTarget(t, h, res, 0, 10)

	putRec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    ns,
		"ResourceId":          res,
		"ScalableDimension":   dim,
		"ScheduledActionName": action,
		"Schedule":            "cron(0 0 * * ? *)",
		"ScalableTargetAction": map[string]any{
			"MinCapacity": 0,
			"MaxCapacity": 0,
		},
	})
	require.Equal(t, http.StatusOK, putRec.Code)
	var putOut map[string]any
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putOut))
	actionARN := putOut["ScheduledActionARN"].(string)
	assert.NotEmpty(t, actionARN)

	descRec := doRequest(t, h, "DescribeScheduledActions", map[string]any{
		"ServiceNamespace": ns,
	})
	var descOut map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	actions := descOut["ScheduledActions"].([]any)
	require.Len(t, actions, 1)
	a0 := actions[0].(map[string]any)
	assert.Equal(t, action, a0["ScheduledActionName"])
	assert.Equal(t, actionARN, a0["ScheduledActionARN"])
	assert.NotNil(t, a0["CreationTime"])

	delRec := doRequest(t, h, "DeleteScheduledAction", map[string]any{
		"ServiceNamespace":    ns,
		"ResourceId":          res,
		"ScalableDimension":   dim,
		"ScheduledActionName": action,
	})
	assert.Equal(t, http.StatusOK, delRec.Code)

	afterRec := doRequest(t, h, "DescribeScheduledActions", map[string]any{
		"ServiceNamespace": ns,
	})
	var afterOut map[string]any
	require.NoError(t, json.Unmarshal(afterRec.Body.Bytes(), &afterOut))
	assert.Empty(t, afterOut["ScheduledActions"])
}

// TestHandler_PutScheduledAction_UpdateClearsOmittedStartEndTime verifies
// that updating an existing scheduled action without resending
// StartTime/EndTime clears them, per api_op_PutScheduledAction.go's doc: "To
// update a scheduled action, specify the parameters that you want to change.
// If you don't specify start and end times, the old values are deleted".
func TestHandler_PutScheduledAction_UpdateClearsOmittedStartEndTime(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/my-svc", 1, 10)

	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "morning-scale",
		"Schedule":            "cron(0 8 * * ? *)",
		"StartTime":           1704096000,
		"EndTime":             1735632000,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Update without StartTime/EndTime: real AWS deletes the old values.
	rec2 := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "morning-scale",
		"Schedule":            "cron(0 9 * * ? *)",
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	descRec := doRequest(t, h, "DescribeScheduledActions", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	actions, ok := resp["ScheduledActions"].([]any)
	require.True(t, ok)
	require.Len(t, actions, 1)
	action := actions[0].(map[string]any)

	assert.Equal(t, "cron(0 9 * * ? *)", action["Schedule"])
	assert.Nil(t, action["StartTime"], "StartTime must be cleared when omitted on update")
	assert.Nil(t, action["EndTime"], "EndTime must be cleared when omitted on update")
}

// TestHandler_PutScheduledAction_UpdateWithoutScheduleKeepsOld verifies that
// Schedule is not required on an update: PutScheduledActionInput does not mark
// it "This member is required" (only ResourceId/ScalableDimension/
// ScheduledActionName/ServiceNamespace are), and the operation doc's
// "specify the parameters that you want to change" confirms it's left
// unchanged when omitted -- matching every other optional field on this op.
func TestHandler_PutScheduledAction_UpdateWithoutScheduleKeepsOld(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/my-svc", 1, 10)

	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "morning-scale",
		"Schedule":            "cron(0 8 * * ? *)",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	minCap := int32(3)
	rec2 := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "morning-scale",
		// Schedule intentionally omitted.
		"ScalableTargetAction": map[string]any{"MinCapacity": minCap},
	})
	require.Equal(t, http.StatusOK, rec2.Code, "Schedule must be optional on an update to an existing action")

	descRec := doRequest(t, h, "DescribeScheduledActions", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	actions, ok := resp["ScheduledActions"].([]any)
	require.True(t, ok)
	require.Len(t, actions, 1)
	action := actions[0].(map[string]any)

	assert.Equal(t, "cron(0 8 * * ? *)", action["Schedule"], "Schedule must be preserved when omitted on update")
}
