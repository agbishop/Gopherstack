package scheduler_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/scheduler"
)

// createScheduleViaHandler sends a CreateSchedule request through h and requires 200 OK.
func createScheduleViaHandler(t *testing.T, h *scheduler.Handler, name, groupName, expr string) {
	t.Helper()

	body := map[string]any{
		"Name":               name,
		"GroupName":          groupName,
		"ScheduleExpression": expr,
		"Target": map[string]string{
			"Arn":     "arn:aws:lambda:us-east-1:000000000000:function:fn",
			"RoleArn": "arn:aws:iam::000000000000:role/r",
		},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	}
	rec := doSchedulerRequest(t, h, "CreateSchedule", body)
	require.Equal(t, http.StatusOK, rec.Code)
}

// createBaseSchedule sends a CreateSchedule request and requires 200 OK, using the
// same minimal rate(1 hour)/SQS target shape used by the target-round-trip and
// pagination tests.
func createBaseSchedule(t *testing.T, h *scheduler.Handler, name string) {
	t.Helper()

	body := map[string]any{
		"Name":               name,
		"ScheduleExpression": "rate(1 hour)",
		"Target": map[string]any{
			"Arn":     "arn:aws:sqs:us-east-1:000000000000:q",
			"RoleArn": "arn:aws:iam::000000000000:role/r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	}

	rec := doSchedulerRequest(t, h, "CreateSchedule", body)
	require.Equal(t, http.StatusOK, rec.Code, "create failed: %s", rec.Body.String())
}

func TestSchedulerHandler_CreateSchedule(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(5 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:aws:lambda:us-east-1:000000000000:function:my-fn",
			"RoleArn": "arn:aws:iam::000000000000:role/my-role",
		},
		"FlexibleTimeWindow": map[string]any{
			"Mode": "OFF",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["ScheduleArn"], "arn:aws:scheduler:")
}

func TestSchedulerHandler_CreateScheduleAlreadyExists(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	body := map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(5 minutes)",
		"Target":             map[string]string{"Arn": "arn:aws:lambda:::fn", "RoleArn": "arn:aws:iam:::role"},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	}
	doSchedulerRequest(t, h, "CreateSchedule", body)

	rec := doSchedulerRequest(t, h, "CreateSchedule", body)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestSchedulerHandler_CreateScheduleDefaultState(t *testing.T) {
	t.Parallel()

	// When State is omitted, it should default to ENABLED.
	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "no-state-schedule",
		"ScheduleExpression": "rate(1 hour)",
		"Target": map[string]string{
			"Arn":     "arn:aws:lambda:us-east-1:0:function:f",
			"RoleArn": "arn:aws:iam::0:role/r",
		},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	getRec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "no-state-schedule"})
	require.Equal(t, http.StatusOK, getRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))
	assert.Equal(t, "ENABLED", resp["State"])
}

// TestCreateSchedule_DefaultStateEnabled duplicates
// TestSchedulerHandler_CreateScheduleDefaultState's assertion (omitted State
// defaults to ENABLED) with a distinct schedule name and via a plain create+get,
// kept as a separate case per the source de-duplication sweep's "preserve all
// cases" rule.
func TestCreateSchedule_DefaultStateEnabled(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "default-state",
		"ScheduleExpression": "rate(1 hour)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "default-state"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ENABLED", resp["State"])
}

func TestSchedulerHandler_CreateScheduleCronExpression(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "cron-schedule",
		"ScheduleExpression": "cron(0 12 * * ? *)",
		"Target": map[string]string{
			"Arn":     "arn:aws:lambda:us-east-1:0:function:f",
			"RoleArn": "arn:aws:iam::0:role/r",
		},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"State":              "ENABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestCreateSchedule_ScheduleExpression_Validation asserts ValidationException for bad cron formats.
func TestCreateSchedule_ScheduleExpression_Validation(t *testing.T) {
	t.Parallel()

	validTarget := map[string]any{
		"Arn":     "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		"RoleArn": "arn:aws:iam::123456789012:role/scheduler-role",
	}

	tests := []struct {
		name     string
		expr     string
		wantType string
		wantCode int
	}{
		{
			name:     "valid_rate_expression",
			expr:     "rate(5 minutes)",
			wantCode: http.StatusOK,
		},
		{
			name:     "valid_cron_6_fields",
			expr:     "cron(0 12 * * ? *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "valid_at_expression",
			expr:     "at(2024-01-01T00:00:00)",
			wantCode: http.StatusOK,
		},
		{
			name:     "empty_expression_rejected",
			expr:     "",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "unknown_prefix_rejected",
			expr:     "every(5 minutes)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_missing_closing_paren_rejected",
			expr:     "cron(0 12 * * ?",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_too_few_fields_rejected",
			expr:     "cron(0 12 * *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_too_many_fields_rejected",
			expr:     "cron(0 12 * * ? * extra)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "rate_missing_unit_rejected",
			expr:     "rate(5)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "rate_zero_value_rejected",
			expr:     "rate(0 minutes)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "rate_unknown_unit_rejected",
			expr:     "rate(5 fortnights)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "rate_negative_value_rejected",
			expr:     "rate(-5 minutes)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "at_bad_format_rejected",
			expr:     "at(2024-01-01)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_garbage_field_rejected",
			expr:     "cron(0 12 * * ? GARBAGE)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_minute_out_of_range_rejected",
			expr:     "cron(60 12 * * ? *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_hour_out_of_range_rejected",
			expr:     "cron(0 24 * * ? *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_day_of_month_zero_rejected",
			expr:     "cron(0 12 0 * ? *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_month_out_of_range_rejected",
			expr:     "cron(0 12 1 13 ? *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_day_of_week_out_of_range_rejected",
			expr:     "cron(0 12 ? * 8 *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_year_before_1970_rejected",
			expr:     "cron(0 12 1 1 ? 1969)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_year_after_2199_rejected",
			expr:     "cron(0 12 1 1 ? 2200)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_question_mark_in_minutes_rejected",
			expr:     "cron(? 12 * * ? *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_question_mark_in_month_rejected",
			expr:     "cron(0 12 ? ? ? *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_both_day_fields_wildcard_rejected",
			expr:     "cron(0 12 * * * *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_hash_combined_with_comma_rejected",
			expr:     "cron(0 12 ? * 3#1,6#3 *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_slash_in_day_of_week_rejected",
			expr:     "cron(0 12 ? * 1-5/2 *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "cron_valid_range_and_step_accepted",
			expr:     "cron(0-5 */15 * * ? *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_month_and_dow_names_accepted",
			expr:     "cron(0 10 ? * MON-FRI *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_last_friday_of_month_accepted",
			expr:     "cron(15 10 ? * 6L 2026-2027)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_nth_weekday_accepted",
			expr:     "cron(0 12 ? * 3#2 *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_nearest_weekday_accepted",
			expr:     "cron(0 12 3W * ? *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_last_day_of_month_accepted",
			expr:     "cron(0 12 L * ? *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_last_day_offset_accepted",
			expr:     "cron(30 23 L-2 * ? *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_last_weekday_of_month_accepted",
			expr:     "cron(0 12 LW * ? *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_day_of_week_last_offset_accepted",
			expr:     "cron(0 12 ? * L-1 *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_valid_list_containing_last_day_accepted",
			expr:     "cron(0 12 L,15 * ? *)",
			wantCode: http.StatusOK,
		},
		{
			name:     "cron_last_day_offset_non_numeric_rejected",
			expr:     "cron(0 12 L-abc * ? *)",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)

			rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
				"Name":               "sched-" + tt.name,
				"ScheduleExpression": tt.expr,
				"Target":             validTarget,
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantType != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

// TestCreateSchedule_NameValidation covers Name field length and character-set rules.
func TestCreateSchedule_NameValidation(t *testing.T) {
	t.Parallel()

	t.Run("invalid_chars", func(t *testing.T) {
		t.Parallel()

		h := newTestSchedulerHandler(t)

		rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
			"Name":               "bad name!",
			"ScheduleExpression": "rate(1 hour)",
			"Target": map[string]any{
				"Arn":     "arn:aws:sqs:us-east-1:0:q",
				"RoleArn": "arn:aws:iam::0:role/r",
			},
			"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ValidationException")
	})

	t.Run("too_long", func(t *testing.T) {
		t.Parallel()

		h := newTestSchedulerHandler(t)
		var sb strings.Builder
		for range 65 {
			sb.WriteByte('a')
		}
		longName := sb.String()

		rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
			"Name":               longName,
			"ScheduleExpression": "rate(1 hour)",
			"Target": map[string]any{
				"Arn":     "arn:aws:sqs:us-east-1:0:q",
				"RoleArn": "arn:aws:iam::0:role/r",
			},
			"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid_chars", func(t *testing.T) {
		t.Parallel()

		h := newTestSchedulerHandler(t)

		for _, name := range []string{"my-schedule", "sched_1.0", "ABC123"} {
			rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
				"Name":               name,
				"ScheduleExpression": "rate(1 hour)",
				"Target": map[string]any{
					"Arn":     "arn:aws:sqs:us-east-1:0:q",
					"RoleArn": "arn:aws:iam::0:role/r",
				},
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			})
			assert.Equal(t, http.StatusOK, rec.Code, "valid name %q should be accepted", name)
		}
	})
}

func TestCreateSchedule_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "missing_name",
			body: map[string]any{
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_expression",
			body: map[string]any{
				"Name":               "s1",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_target_arn",
			body: map[string]any{
				"Name":               "s1",
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_target_role_arn",
			body: map[string]any{
				"Name":               "s1",
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"Arn": "arn:a"},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_ftw_mode",
			body: map[string]any{
				"Name":               "s1",
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]any{},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "group_not_found",
			body: map[string]any{
				"Name":               "s1",
				"GroupName":          "nonexistent-group",
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			rec := doSchedulerRequest(t, h, "CreateSchedule", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestCreateSchedule_MissingRequiredFields verifies required field errors.
func TestCreateSchedule_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "missing_name",
			body: map[string]any{
				"ScheduleExpression": "rate(1 minute)",
				"Target": map[string]string{
					"Arn":     "arn:aws:sqs:us-east-1:0:q",
					"RoleArn": "arn:aws:iam::0:role/r",
				},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
		},
		{
			name: "missing_expression",
			body: map[string]any{
				"Name": "no-expr",
				"Target": map[string]string{
					"Arn":     "arn:aws:sqs:us-east-1:0:q",
					"RoleArn": "arn:aws:iam::0:role/r",
				},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
		},
		{
			name: "missing_target_arn",
			body: map[string]any{
				"Name":               "no-tgt-arn",
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"RoleArn": "arn:aws:iam::0:role/r"},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			rec := doSchedulerRequest(t, h, "CreateSchedule", tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestCreateSchedule_ValidateState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   string
		wantErr bool
	}{
		{name: "enabled", state: "ENABLED", wantErr: false},
		{name: "disabled", state: "DISABLED", wantErr: false},
		{name: "empty_defaults_enabled", state: "", wantErr: false},
		{name: "invalid", state: "RUNNING", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			body := map[string]any{
				"Name":               "state-test",
				"ScheduleExpression": "rate(1 minute)",
				"Target": map[string]string{
					"Arn":     "arn:aws:sqs:us-east-1:0:q",
					"RoleArn": "arn:aws:iam::0:role/r",
				},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			}

			if tt.state != "" {
				body["State"] = tt.state
			}

			rec := doSchedulerRequest(t, h, "CreateSchedule", body)
			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			} else {
				assert.Equal(t, http.StatusOK, rec.Code)
			}
		})
	}
}

// TestValidateActionAfterCompletion verifies ActionAfterCompletion is rejected
// unless it is NONE, DELETE, or omitted, on both CreateSchedule and
// UpdateSchedule (AWS: aws-sdk-go-v2/service/scheduler/types.ActionAfterCompletion
// only defines NONE and DELETE).
func TestValidateActionAfterCompletion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  string
		wantErr bool
	}{
		{name: "none", action: "NONE", wantErr: false},
		{name: "delete", action: "DELETE", wantErr: false},
		{name: "empty_defaults", action: "", wantErr: false},
		{name: "invalid", action: "TERMINATE", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			body := map[string]any{
				"Name":               "action-after-completion-test",
				"ScheduleExpression": "rate(1 minute)",
				"Target": map[string]string{
					"Arn":     "arn:aws:sqs:us-east-1:0:q",
					"RoleArn": "arn:aws:iam::0:role/r",
				},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			}

			if tt.action != "" {
				body["ActionAfterCompletion"] = tt.action
			}

			createRec := doSchedulerRequest(t, h, "CreateSchedule", body)
			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, createRec.Code)
			} else {
				assert.Equal(t, http.StatusOK, createRec.Code)
			}

			// UpdateSchedule must reject the same invalid values on an existing schedule.
			createScheduleViaHandler(t, h, "action-after-completion-update-test", "", "rate(1 minute)")
			updateBody := map[string]any{
				"Name":               "action-after-completion-update-test",
				"ScheduleExpression": "rate(5 minutes)",
				"Target": map[string]string{
					"Arn":     "arn:aws:sqs:us-east-1:0:q",
					"RoleArn": "arn:aws:iam::0:role/r",
				},
				"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
			}
			if tt.action != "" {
				updateBody["ActionAfterCompletion"] = tt.action
			}

			updateRec := doSchedulerRequest(t, h, "UpdateSchedule", updateBody)
			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, updateRec.Code)
			} else {
				assert.Equal(t, http.StatusOK, updateRec.Code)
			}
		})
	}
}

// TestValidateFlexibleTimeWindowMode verifies invalid mode is rejected and
// FLEXIBLE mode requires MaximumWindowInMinutes > 0.
func TestValidateFlexibleTimeWindowMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ftw     map[string]any
		name    string
		wantErr bool
	}{
		{name: "off", ftw: map[string]any{"Mode": "OFF"}, wantErr: false},
		{name: "flexible", ftw: map[string]any{"Mode": "FLEXIBLE", "MaximumWindowInMinutes": 15}, wantErr: false},
		{name: "flexible_no_window", ftw: map[string]any{"Mode": "FLEXIBLE"}, wantErr: true},
		{name: "invalid", ftw: map[string]any{"Mode": "STRICT"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
				"Name":               "mode-test-" + tt.name,
				"ScheduleExpression": "rate(1 minute)",
				"Target": map[string]string{
					"Arn":     "arn:aws:sqs:us-east-1:0:q",
					"RoleArn": "arn:aws:iam::0:role/r",
				},
				"FlexibleTimeWindow": tt.ftw,
			})
			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			} else {
				assert.Equal(t, http.StatusOK, rec.Code)
			}
		})
	}
}

// TestCreateSchedule_FlexibleModeRequiresMaximumWindowInMinutes verifies that
// FlexibleTimeWindow.Mode=FLEXIBLE without MaximumWindowInMinutes returns 400.
func TestCreateSchedule_FlexibleModeRequiresMaximumWindowInMinutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ftw      map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "flexible_with_window_ok",
			ftw:      map[string]any{"Mode": "FLEXIBLE", "MaximumWindowInMinutes": 30},
			wantCode: http.StatusOK,
		},
		{
			name:     "flexible_without_window_rejected",
			ftw:      map[string]any{"Mode": "FLEXIBLE"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "flexible_zero_window_rejected",
			ftw:      map[string]any{"Mode": "FLEXIBLE", "MaximumWindowInMinutes": 0},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "off_without_window_ok",
			ftw:      map[string]any{"Mode": "OFF"},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
				"Name":               "ftw-test-" + tt.name,
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": tt.ftw,
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusBadRequest {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, "ValidationException", errResp["__type"])
			}
		})
	}
}

// TestCreateSchedule_FlexibleWindowRoundtrip verifies that MaximumWindowInMinutes is
// persisted and returned in GetSchedule when Mode is FLEXIBLE.
func TestCreateSchedule_FlexibleWindowRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "ftw-roundtrip",
		"ScheduleExpression": "rate(10 minutes)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]any{"Mode": "FLEXIBLE", "MaximumWindowInMinutes": 20},
	})

	getRec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "ftw-roundtrip"})
	require.Equal(t, http.StatusOK, getRec.Code)

	var out struct {
		FlexibleTimeWindow struct {
			Mode                   string `json:"Mode"`
			MaximumWindowInMinutes int    `json:"MaximumWindowInMinutes"`
		} `json:"FlexibleTimeWindow"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &out))
	assert.Equal(t, "FLEXIBLE", out.FlexibleTimeWindow.Mode)
	assert.Equal(t, 20, out.FlexibleTimeWindow.MaximumWindowInMinutes)
}

// TestCreateSchedule_FlexibleTimeWindowMaximumWindowInMinutes verifies
// MaximumWindowInMinutes is persisted (variant seeding via ftw-sched).
func TestCreateSchedule_FlexibleTimeWindowMaximumWindowInMinutes(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "ftw-sched",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]any{"Mode": "FLEXIBLE", "MaximumWindowInMinutes": 15},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "ftw-sched"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out struct {
		FlexibleTimeWindow struct {
			Mode                   string `json:"Mode"`
			MaximumWindowInMinutes int    `json:"MaximumWindowInMinutes"`
		} `json:"FlexibleTimeWindow"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))
	assert.Equal(t, "FLEXIBLE", out.FlexibleTimeWindow.Mode)
	assert.Equal(t, 15, out.FlexibleTimeWindow.MaximumWindowInMinutes)
}

func TestSchedulerHandler_GetSchedule(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(5 minutes)",
		"Target":             map[string]string{"Arn": "arn:aws:lambda:::fn", "RoleArn": "arn:aws:iam:::role"},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "my-schedule"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "my-schedule", resp["Name"])
	assert.Equal(t, "rate(5 minutes)", resp["ScheduleExpression"])
	assert.Contains(t, resp, "Target")
	assert.Contains(t, resp, "FlexibleTimeWindow")
}

func TestGetSchedule_ReturnsGroupNameAndDates(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	// Create a custom group first.
	doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": "mygroup"})

	// Create a schedule in that group.
	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "gs-sched",
		"GroupName":          "mygroup",
		"ScheduleExpression": "rate(1 hour)",
		"Description":        "test description",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "gs-sched", "GroupName": "mygroup"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, "mygroup", resp["GroupName"])
	assert.Equal(t, "test description", resp["Description"])
	assert.NotEmpty(t, resp["CreationDate"])
	assert.NotEmpty(t, resp["LastModificationDate"])
}

func TestGetSchedule_ReturnsARNWithGroupName(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": "my-group"})

	createRec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "grouped-sched",
		"GroupName":          "my-group",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	assert.Contains(t, createResp["ScheduleArn"], "schedule/my-group/grouped-sched")
}

func TestGetSchedule_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "nonexistent"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetSchedule_TimestampsAreEpochSeconds verifies that GetSchedule returns CreationDate
// and LastModificationDate as JSON numbers (Unix epoch seconds), as required by the AWS SDK.
func TestGetSchedule_TimestampsAreEpochSeconds(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "ts-schedule",
		"ScheduleExpression": "rate(1 hour)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "ts-schedule"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))

	// CreationDate must be a JSON number (float64), not a string.
	var creationDate float64
	require.NoError(t, json.Unmarshal(out["CreationDate"], &creationDate), "CreationDate must be a number")
	assert.Greater(t, creationDate, float64(0), "CreationDate must be a positive epoch value")

	var lastModDate float64
	require.NoError(
		t,
		json.Unmarshal(out["LastModificationDate"], &lastModDate),
		"LastModificationDate must be a number",
	)
	assert.GreaterOrEqual(t, lastModDate, creationDate)
}

// TestCreateSchedule_EpochSecondsConvertBack verifies epochSecondsToTime converts correctly.
func TestCreateSchedule_EpochSecondsConvertBack(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	now := time.Now().UTC().Truncate(time.Second)
	epoch := float64(now.Unix())

	// Store and retrieve to test round-trip through epoch conversion.
	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "epoch-test",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"StartDate":          epoch,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "epoch-test"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))

	var startDate float64
	require.NoError(t, json.Unmarshal(out["StartDate"], &startDate))
	// The stored epoch should round-trip back to the same second.
	assert.InDelta(t, epoch, startDate, 1.0)
}

// TestCreateSchedule_ActionAfterCompletion verifies the ActionAfterCompletion field is persisted.
func TestCreateSchedule_ActionAfterCompletion(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "aac-sched",
		"ScheduleExpression": "rate(1 minute)",
		"Target": map[string]string{
			"Arn":     "arn:aws:sqs:us-east-1:0:q",
			"RoleArn": "arn:aws:iam::0:role/r",
		},
		"FlexibleTimeWindow":    map[string]string{"Mode": "OFF"},
		"ActionAfterCompletion": "DELETE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "aac-sched"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))
	assert.Equal(t, "DELETE", out["ActionAfterCompletion"])
}

// TestCreateSchedule_KmsKeyArn verifies the KmsKeyArn field is persisted.
func TestCreateSchedule_KmsKeyArn(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	kmsARN := "arn:aws:kms:us-east-1:000000000000:key/12345678-abcd-ef01-2345-6789abcdef01"

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "kms-sched",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"KmsKeyArn":          kmsARN,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "kms-sched"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))
	assert.Equal(t, kmsARN, out["KmsKeyArn"])
}

// TestCreateSchedule_StartDateEndDate verifies optional StartDate and EndDate on schedules.
func TestCreateSchedule_StartDateEndDate(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	startEpoch := float64(time.Now().Unix())
	endEpoch := float64(time.Now().Add(24 * time.Hour).Unix())

	rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "dated-sched",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"StartDate":          startEpoch,
		"EndDate":            endEpoch,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "dated-sched"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))

	_, hasStart := out["StartDate"]
	_, hasEnd := out["EndDate"]
	assert.True(t, hasStart, "StartDate should be present in GetSchedule response")
	assert.True(t, hasEnd, "EndDate should be present in GetSchedule response")
}

// TestCreateSchedule_ScheduleExpressionTimezone verifies ScheduleExpressionTimezone round-trips.
func TestCreateSchedule_ScheduleExpressionTimezone(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":                       "tz-sched",
		"ScheduleExpression":         "cron(0 9 * * ? *)",
		"ScheduleExpressionTimezone": "America/New_York",
		"Target":                     map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow":         map[string]string{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "tz-sched"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "America/New_York", resp["ScheduleExpressionTimezone"])
}

// TestCreateSchedule_ScheduleExpressionTimezone_Validation asserts ValidationException
// for an unresolvable IANA timezone name, and that empty/valid names are accepted.
// An invalid timezone can never be evaluated against wall-clock time by the runner
// (see Runner.cachedLocation), so it is rejected at write time.
func TestCreateSchedule_ScheduleExpressionTimezone_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tz       string
		wantCode int
	}{
		{name: "empty_defaults_to_utc", tz: "", wantCode: http.StatusOK},
		{name: "valid_iana_name", tz: "Europe/London", wantCode: http.StatusOK},
		{name: "unresolvable_name_rejected", tz: "Not/ARealZone", wantCode: http.StatusBadRequest},
		{name: "garbage_rejected", tz: "not-a-timezone", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)

			rec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
				"Name":                       "tz-validate-" + tt.name,
				"ScheduleExpression":         "rate(1 hour)",
				"ScheduleExpressionTimezone": tt.tz,
				"Target":                     map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow":         map[string]string{"Mode": "OFF"},
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "ValidationException", resp["__type"])
			}
		})
	}
}

// TestUpdateSchedule_ScheduleExpressionTimezone_Validation asserts UpdateSchedule
// also rejects an unresolvable ScheduleExpressionTimezone.
func TestUpdateSchedule_ScheduleExpressionTimezone_Validation(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "tz-update-sched",
		"ScheduleExpression": "rate(1 hour)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":                       "tz-update-sched",
		"ScheduleExpression":         "rate(1 hour)",
		"ScheduleExpressionTimezone": "Definitely/Invalid",
		"Target":                     map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow":         map[string]string{"Mode": "OFF"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

func TestSchedulerHandler_DeleteSchedule(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(5 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:a",
			"RoleArn": "arn:r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "DeleteSchedule", map[string]any{"Name": "my-schedule"})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify deleted
	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "my-schedule"})
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

func TestDeleteSchedule_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "DeleteSchedule", map[string]any{"Name": "nonexistent"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteSchedule_InCustomGroup(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	b := h.Backend.(*scheduler.InMemoryBackend)

	_, err := b.CreateScheduleGroup(context.Background(), "custom", nil)
	require.NoError(t, err)

	createScheduleViaHandler(t, h, "del-sched", "custom", "rate(1 minute)")
	assert.Equal(t, 1, scheduler.ScheduleCount(b))

	delRec := doSchedulerRequest(t, h, "DeleteSchedule", map[string]any{
		"Name":      "del-sched",
		"GroupName": "custom",
	})
	require.Equal(t, http.StatusOK, delRec.Code)
	assert.Equal(t, 0, scheduler.ScheduleCount(b))
}

func TestSchedulerHandler_UpdateSchedule(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(5 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:a",
			"RoleArn": "arn:r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "my-schedule",
		"ScheduleExpression": "rate(10 minutes)",
		"Target":             map[string]string{"Arn": "arn:a2", "RoleArn": "arn:r2"},
		"State":              "DISABLED",
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["ScheduleArn"], "arn:aws:scheduler:")

	// Verify the update
	getRec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "my-schedule"})
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	assert.Equal(t, "rate(10 minutes)", getResp["ScheduleExpression"])
	assert.Equal(t, "DISABLED", getResp["State"])
}

func TestUpdateSchedule_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "nonexistent",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUpdateSchedule_UpdatesLastModificationDate(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	h := scheduler.NewHandler(b)

	createScheduleViaHandler(t, h, "upd-sched", "", "rate(1 minute)")

	s1, err := b.GetSchedule(context.Background(), "upd-sched", "")
	require.NoError(t, err)

	// Advance time enough to guarantee LastModificationDate changes.
	time.Sleep(1100 * time.Millisecond)

	doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "upd-sched",
		"ScheduleExpression": "rate(2 minutes)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"State":              "ENABLED",
	})

	s2, err := b.GetSchedule(context.Background(), "upd-sched", "")
	require.NoError(t, err)

	assert.True(t, s2.LastModificationDate.After(s1.LastModificationDate),
		"LastModificationDate should advance after UpdateSchedule")
	assert.Equal(t, "rate(2 minutes)", s2.ScheduleExpression)
}

func TestUpdateSchedule_ValidatesState(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	createScheduleViaHandler(t, h, "upd-state", "", "rate(1 minute)")

	rec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "upd-state",
		"ScheduleExpression": "rate(5 minutes)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"State":              "INVALID",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestUpdateSchedule_OmittedStatePreservesExisting verifies that omitting
// State on UpdateSchedule leaves the schedule's enabled/disabled status unchanged,
// matching aws-sdk-go-v2's UpdateScheduleInput document serializer, which omits the
// "State" JSON key entirely when the field is unset (`if len(v.State) > 0`) rather
// than sending an empty string. Blindly overwriting State with the (empty) input
// would leave the schedule in neither ENABLED nor DISABLED, silently halting the
// runner (checkAndFireSchedules only fires schedules with State == "ENABLED").
func TestUpdateSchedule_OmittedStatePreservesExisting(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	createScheduleViaHandler(t, h, "upd-omit-state", "", "rate(1 minute)")

	// Disable the schedule first.
	disableRec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "upd-omit-state",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
		"State":              "DISABLED",
	})
	require.Equal(t, http.StatusOK, disableRec.Code)

	// Update again without a State field at all.
	rec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "upd-omit-state",
		"ScheduleExpression": "rate(5 minutes)",
		"Target":             map[string]string{"Arn": "arn:aws:sqs:us-east-1:0:q", "RoleArn": "arn:aws:iam::0:role/r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	getRec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "upd-omit-state"})
	require.Equal(t, http.StatusOK, getRec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &out))
	assert.Equal(t, "DISABLED", out["State"], "State must stay DISABLED, not be reset to empty/enabled")
	assert.Equal(t, "rate(5 minutes)", out["ScheduleExpression"])
}

// TestUpdateSchedule_WithActionAfterCompletion verifies ActionAfterCompletion on update.
func TestUpdateSchedule_WithActionAfterCompletion(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	createScheduleViaHandler(t, h, "upd-aac", "", "rate(1 minute)")

	rec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "upd-aac",
		"ScheduleExpression": "rate(5 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:aws:sqs:us-east-1:0:q",
			"RoleArn": "arn:aws:iam::0:role/r",
		},
		"FlexibleTimeWindow":    map[string]string{"Mode": "OFF"},
		"State":                 "ENABLED",
		"ActionAfterCompletion": "NONE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "upd-aac"})
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))
	assert.Equal(t, "NONE", out["ActionAfterCompletion"])
}

// TestUpdateSchedule_RequiredFieldValidation verifies that UpdateSchedule
// enforces the same required-field rules as CreateSchedule: ScheduleExpression,
// Target.Arn, Target.RoleArn, and FlexibleTimeWindow.Mode are all mandatory.
// The previous implementation wrapped each check in "if non-empty" guards, so
// omitting a required field silently zeroed it out in the stored schedule.
func TestUpdateSchedule_RequiredFieldValidation(t *testing.T) {
	t.Parallel()

	validBody := map[string]any{
		"Name":               "test-sched",
		"ScheduleExpression": "rate(5 minutes)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	}

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "valid_update_accepted",
			body:     validBody,
			wantCode: http.StatusOK,
		},
		{
			name: "missing_schedule_expression_rejected",
			body: map[string]any{
				"Name":               "test-sched",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_target_arn_rejected",
			body: map[string]any{
				"Name":               "test-sched",
				"ScheduleExpression": "rate(5 minutes)",
				"Target":             map[string]string{"RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_target_role_arn_rejected",
			body: map[string]any{
				"Name":               "test-sched",
				"ScheduleExpression": "rate(5 minutes)",
				"Target":             map[string]string{"Arn": "arn:a"},
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_flexible_time_window_mode_rejected",
			body: map[string]any{
				"Name":               "test-sched",
				"ScheduleExpression": "rate(5 minutes)",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]any{},
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)

			// Seed the schedule so update has something to update.
			createRec := doSchedulerRequest(t, h, "CreateSchedule", validBody)
			require.Equal(t, http.StatusOK, createRec.Code, "failed to create seed schedule")

			rec := doSchedulerRequest(t, h, "UpdateSchedule", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code, "UpdateSchedule body=%v", tt.body)

			if tt.wantCode == http.StatusBadRequest {
				var errResp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
				assert.Equal(t, "ValidationException", errResp["__type"])
			}
		})
	}
}

// TestUpdateSchedule_ScheduleExpression_SemanticValidation asserts UpdateSchedule
// rejects a structurally-valid but semantically-invalid ScheduleExpression, the
// same class of bug fixed for CreateSchedule (gopherstack-8cg7): a schedule that
// passes shape checks but never fires must be rejected, not silently accepted.
func TestUpdateSchedule_ScheduleExpression_SemanticValidation(t *testing.T) {
	t.Parallel()

	validBody := map[string]any{
		"Name":               "test-sched",
		"ScheduleExpression": "rate(5 minutes)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	}

	tests := []struct {
		name string
		expr string
	}{
		{name: "rate_missing_unit_rejected", expr: "rate(5)"},
		{name: "rate_unknown_unit_rejected", expr: "rate(5 fortnights)"},
		{name: "at_bad_format_rejected", expr: "at(2024-01-01)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)

			createRec := doSchedulerRequest(t, h, "CreateSchedule", validBody)
			require.Equal(t, http.StatusOK, createRec.Code, "failed to create seed schedule")

			body := map[string]any{
				"Name":               "test-sched",
				"ScheduleExpression": tt.expr,
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			}

			rec := doSchedulerRequest(t, h, "UpdateSchedule", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
			assert.Equal(t, "ValidationException", errResp["__type"])
		})
	}
}

// TestUpdateSchedule_DoesNotBlankFields verifies that a valid UpdateSchedule
// replaces the stored schedule correctly (not zeroing fields from prior state).
func TestUpdateSchedule_DoesNotBlankFields(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	createRec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "blank-test",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:orig", "RoleArn": "arn:role-orig"},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	updateRec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "blank-test",
		"ScheduleExpression": "rate(10 minutes)",
		"Target":             map[string]string{"Arn": "arn:new", "RoleArn": "arn:role-new"},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	getRec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "blank-test"})
	require.Equal(t, http.StatusOK, getRec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&out))

	assert.Equal(t, "rate(10 minutes)", out["ScheduleExpression"])

	target, _ := out["Target"].(map[string]any)
	assert.Equal(t, "arn:new", target["Arn"])
	assert.Equal(t, "arn:role-new", target["RoleArn"])
}

// TestUpdateSchedule_ClearsOmittedOptionalFields verifies that UpdateSchedule is a
// full replacement (api_op_UpdateSchedule.go:16-19): StartDate, EndDate, and
// KmsKeyArn are true optional/pointer fields on the wire (unlike the State enum,
// which is exempted -- see the UpdateSchedule backend comment), so omitting them
// from an update must reset them, not preserve the prior value.
func TestUpdateSchedule_ClearsOmittedOptionalFields(t *testing.T) {
	t.Parallel()

	epoch := float64(time.Now().UTC().Truncate(time.Second).Unix())
	kmsARN := "arn:aws:kms:us-east-1:000000000000:key/12345678-abcd-ef01-2345-6789abcdef01"

	tests := []struct {
		name       string
		createBody map[string]any
		field      string
	}{
		{name: "start_date", createBody: map[string]any{"StartDate": epoch}, field: "StartDate"},
		{name: "end_date", createBody: map[string]any{"EndDate": epoch}, field: "EndDate"},
		{name: "kms_key_arn", createBody: map[string]any{"KmsKeyArn": kmsARN}, field: "KmsKeyArn"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)

			create := map[string]any{
				"Name":               "clear-" + tc.name,
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			}
			maps.Copy(create, tc.createBody)

			createRec := doSchedulerRequest(t, h, "CreateSchedule", create)
			require.Equal(t, http.StatusOK, createRec.Code)

			updateRec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
				"Name":               "clear-" + tc.name,
				"ScheduleExpression": "rate(1 minute)",
				"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
				"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
			})
			require.Equal(t, http.StatusOK, updateRec.Code)

			getRec := doSchedulerRequest(t, h, "GetSchedule", map[string]any{"Name": "clear-" + tc.name})
			require.Equal(t, http.StatusOK, getRec.Code)

			var out map[string]any
			require.NoError(t, json.NewDecoder(getRec.Body).Decode(&out))

			assert.NotContains(t, out, tc.field, "%s should be cleared after an update that omits it", tc.field)
		})
	}
}

// TestUpdateSchedule_FlexibleValidation verifies that UpdateSchedule also
// enforces the MaximumWindowInMinutes requirement for FLEXIBLE mode.
func TestUpdateSchedule_FlexibleValidation(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	// Create with OFF mode.
	createRec := doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "upd-ftw-test",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	// Update to FLEXIBLE without MaximumWindowInMinutes — must be rejected.
	rec := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "upd-ftw-test",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]any{"Mode": "FLEXIBLE"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Update to FLEXIBLE with MaximumWindowInMinutes — must succeed.
	rec2 := doSchedulerRequest(t, h, "UpdateSchedule", map[string]any{
		"Name":               "upd-ftw-test",
		"ScheduleExpression": "rate(1 minute)",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]any{"Mode": "FLEXIBLE", "MaximumWindowInMinutes": 10},
	})
	assert.Equal(t, http.StatusOK, rec2.Code)
}
