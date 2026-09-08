package fis_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDLength(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", minimalTemplateBody())
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp struct {
		ExperimentTemplate struct {
			ID string `json:"id"`
		} `json:"experimentTemplate"`
	}

	mustJSON(t, rec, &resp)
	id := resp.ExperimentTemplate.ID

	assert.True(t, strings.HasPrefix(id, "EXT"), "expected EXT prefix, got %q", id)
	assert.Len(t, id, 16, "expected 16-char total ID, got %q", id)
}

func TestCreateTemplate_RoleArn_Required(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"stopConditions": []map[string]any{{"source": "none"}},
		"targets":        map[string]any{},
		"actions":        map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateTemplate_RoleArn_InvalidFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn":        "not-a-real-arn",
		"stopConditions": []map[string]any{{"source": "none"}},
		"targets":        map[string]any{},
		"actions":        map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateTemplate_RoleArn_ValidFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, arn := range []string{
		"arn:aws:iam::000000000000:role/FISRole",
		"arn:aws:iam::123456789012:role/path/to/role",
	} {
		body := map[string]any{
			"roleArn":        arn,
			"stopConditions": []map[string]any{{"source": "none"}},
			"targets":        map[string]any{},
			"actions":        map[string]any{},
		}

		rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
		assert.Equal(t, http.StatusCreated, rec.Code, "ARN %q should be valid", arn)
	}
}

// ----------------------------------------
// Issue #8 — selectionMode validation
// ----------------------------------------

func TestCreateTemplate_SelectionMode_Required(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn":        "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{{"source": "none"}},
		"targets": map[string]any{
			"MyInstances": map[string]any{
				"resourceType": "aws:ec2:instance",
				// selectionMode intentionally omitted
			},
		},
		"actions": map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateTemplate_SelectionMode_Invalid(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, mode := range []string{"random", "SOME", "count(3)", "percent-50"} {
		body := map[string]any{
			"roleArn":        "arn:aws:iam::000000000000:role/FISRole",
			"stopConditions": []map[string]any{{"source": "none"}},
			"targets": map[string]any{
				"MyInstances": map[string]any{
					"resourceType":  "aws:ec2:instance",
					"selectionMode": mode,
				},
			},
			"actions": map[string]any{},
		}

		rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "selectionMode %q should be rejected", mode)
	}
}

func TestCreateTemplate_SelectionMode_Valid(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, mode := range []string{"ALL", "COUNT(1)", "COUNT(10)", "PERCENT(50)", "PERCENT(100)"} {
		body := map[string]any{
			"roleArn":        "arn:aws:iam::000000000000:role/FISRole",
			"stopConditions": []map[string]any{{"source": "none"}},
			"targets": map[string]any{
				"MyInstances": map[string]any{
					"resourceType":  "aws:ec2:instance",
					"selectionMode": mode,
				},
			},
			"actions": map[string]any{},
		}

		rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
		assert.Equal(t, http.StatusCreated, rec.Code, "selectionMode %q should be valid", mode)
	}
}

// ----------------------------------------
// Issue #8 — action target reference validation
// ----------------------------------------

func TestCreateTemplate_Action_UndefinedTarget_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn":        "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{{"source": "none"}},
		"targets":        map[string]any{},
		"actions": map[string]any{
			"myAction": map[string]any{
				"actionId": "aws:ec2:stop-instances",
				"targets":  map[string]string{"Instances": "NonExistentTarget"},
			},
		},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ----------------------------------------
// Issue #21 — aws:fis:wait duration required
// ----------------------------------------

func TestCreateTemplate_Wait_NoDuration_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn":        "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{{"source": "none"}},
		"targets":        map[string]any{},
		"actions": map[string]any{
			"wait": map[string]any{
				"actionId":   "aws:fis:wait",
				"parameters": map[string]string{},
			},
		},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateTemplate_StopConditions_Required(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/FISRole",
		"targets": map[string]any{},
		"actions": map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ----------------------------------------
// gopherstack-x842 — stop condition source/value cross-field validation
// ----------------------------------------

func TestCreateTemplate_StopCondition_AlarmSource_RequiresValue(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn":        "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{{"source": "aws:cloudwatch:alarm"}},
		"targets":        map[string]any{},
		"actions":        map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp struct {
		Type string `json:"__type"`
	}

	mustJSON(t, rec, &resp)
	assert.Equal(t, "ValidationException", resp.Type)
}

func TestCreateTemplate_StopCondition_SourceMustBeExactMatch(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{
			{
				"source": "aws:cloudwatch:alarmBogusSuffix",
				"value":  "arn:aws:cloudwatch:us-east-1:000000000000:alarm:MyAlarm",
			},
		},
		"targets": map[string]any{},
		"actions": map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateTemplate_StopCondition_AlarmSource_WithValue_Accepted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{
			{
				"source": "aws:cloudwatch:alarm",
				"value":  "arn:aws:cloudwatch:us-east-1:000000000000:alarm:MyAlarm",
			},
		},
		"targets": map[string]any{},
		"actions": map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

// ----------------------------------------
// Issue #22 — parseISODuration rejects months/years/weeks
// ----------------------------------------

// TestFISRoleArnValidation verifies that isValidRoleArn requires a 12-digit account ID.
func TestFISRoleArnValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		roleArn    string
		wantStatus int
	}{
		{
			name:       "valid 12-digit account",
			roleArn:    "arn:aws:iam::000000000000:role/FISRole",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "3-digit account rejects",
			roleArn:    "arn:aws:iam::000:role/FISRole",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing account rejects",
			roleArn:    "arn:aws:iam:::role/FISRole",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-digit account rejects",
			roleArn:    "arn:aws:iam::abc123456789:role/FISRole",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "13-digit account rejects",
			roleArn:    "arn:aws:iam::0000000000000:role/FISRole",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(
				t, h, http.MethodPost, "/experimentTemplates",
				minimalTemplate("rolearn-test", tc.roleArn),
			)
			assert.Equal(t, tc.wantStatus, rec.Code, "roleArn=%q", tc.roleArn)
		})
	}
}
