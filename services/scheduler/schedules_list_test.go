package scheduler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/scheduler"
)

func TestSchedulerHandler_ListSchedules(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "s1",
		"ScheduleExpression": "rate(1 minute)",
		"Target": map[string]string{
			"Arn":     "arn:a",
			"RoleArn": "arn:r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})
	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "s2",
		"ScheduleExpression": "rate(2 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:a",
			"RoleArn": "arn:r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "ListSchedules", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp, "Schedules")
	schedules, ok := resp["Schedules"].([]any)
	require.True(t, ok)
	assert.Len(t, schedules, 2)
}

func TestListSchedules_FilterByGroupName(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	b := h.Backend.(*scheduler.InMemoryBackend)

	_, err := b.CreateScheduleGroup(context.Background(), "g1", nil)
	require.NoError(t, err)
	_, err = b.CreateScheduleGroup(context.Background(), "g2", nil)
	require.NoError(t, err)

	createScheduleViaHandler(t, h, "in-g1", "g1", "rate(1 minute)")
	createScheduleViaHandler(t, h, "in-g2", "g2", "rate(2 minutes)")

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{"GroupName": "g1"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	schedules := resp["Schedules"].([]any)
	require.Len(t, schedules, 1)
	assert.Equal(t, "in-g1", schedules[0].(map[string]any)["Name"])
}

func TestListSchedules_FilterByNamePrefix(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	createScheduleViaHandler(t, h, "prod-sched", "", "rate(1 minute)")
	createScheduleViaHandler(t, h, "dev-sched", "", "rate(2 minutes)")
	createScheduleViaHandler(t, h, "prod-other", "", "rate(3 minutes)")

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{"NamePrefix": "prod-"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	schedules := resp["Schedules"].([]any)
	assert.Len(t, schedules, 2)
}

func TestListSchedules_FilterByState(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	createScheduleViaHandler(t, h, "enabled-sched", "", "rate(1 minute)")
	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "disabled-sched",
		"ScheduleExpression": "rate(2 minutes)",
		"State":              "DISABLED",
		"Target":             map[string]string{"Arn": "arn:a", "RoleArn": "arn:r"},
		"FlexibleTimeWindow": map[string]string{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{"State": "ENABLED"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	schedules := resp["Schedules"].([]any)
	require.Len(t, schedules, 1)
	assert.Equal(t, "enabled-sched", schedules[0].(map[string]any)["Name"])
}

func TestListSchedules_Sorted(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	createScheduleViaHandler(t, h, "zebra", "", "rate(1 minute)")
	createScheduleViaHandler(t, h, "alpha", "", "rate(2 minutes)")
	createScheduleViaHandler(t, h, "middle", "", "rate(3 minutes)")

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	schedules := resp["Schedules"].([]any)
	require.Len(t, schedules, 3)
	assert.Equal(t, "alpha", schedules[0].(map[string]any)["Name"])
	assert.Equal(t, "middle", schedules[1].(map[string]any)["Name"])
	assert.Equal(t, "zebra", schedules[2].(map[string]any)["Name"])
}

func TestListSchedules_IncludesGroupNameAndDates(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	b := h.Backend.(*scheduler.InMemoryBackend)

	_, err := b.CreateScheduleGroup(context.Background(), "custom-g", nil)
	require.NoError(t, err)

	createScheduleViaHandler(t, h, "dated-sched", "custom-g", "rate(1 minute)")

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{"GroupName": "custom-g"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	schedules := resp["Schedules"].([]any)
	require.Len(t, schedules, 1)

	s := schedules[0].(map[string]any)
	assert.Equal(t, "custom-g", s["GroupName"])
	assert.NotEmpty(t, s["CreationDate"])
	assert.NotEmpty(t, s["LastModificationDate"])
}

// TestListSchedules_TimestampsAreEpochSeconds verifies ListSchedules returns epoch seconds.
func TestListSchedules_TimestampsAreEpochSeconds(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	createScheduleViaHandler(t, h, "list-ts-s", "", "rate(1 minute)")

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Schedules []map[string]json.RawMessage `json:"Schedules"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.Schedules)

	var cd float64
	require.NoError(t, json.Unmarshal(out.Schedules[0]["CreationDate"], &cd))
	assert.Greater(t, cd, float64(0))
}

// TestListSchedules_NextTokenOmittedWhenComplete verifies NextToken is omitted when all results fit.
func TestListSchedules_NextTokenOmittedWhenComplete(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	createScheduleViaHandler(t, h, "nt-s1", "", "rate(1 minute)")

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		NextToken string `json:"NextToken"`
		Schedules []any  `json:"Schedules"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	// NextToken should be empty when all results fit.
	assert.Empty(t, out.NextToken)
}

// TestListSchedules_MaxResultsPagination verifies MaxResults paginates via NextToken.
func TestListSchedules_MaxResultsPagination(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	for _, name := range []string{"sched-a", "sched-b", "sched-c", "sched-d", "sched-e"} {
		createBaseSchedule(t, h, name)
	}

	// Get first page of 2.
	rec1 := doSchedulerRequest(t, h, "ListSchedules", map[string]any{"MaxResults": "2"})
	require.Equal(t, http.StatusOK, rec1.Code)

	var page1 map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &page1))

	var schedules1 []json.RawMessage
	require.NoError(t, json.Unmarshal(page1["Schedules"], &schedules1))
	require.Len(t, schedules1, 2)

	var nextToken string
	require.NoError(t, json.Unmarshal(page1["NextToken"], &nextToken))
	assert.NotEmpty(t, nextToken)

	// Get second page using the token.
	rec2 := doSchedulerRequest(t, h, "ListSchedules", map[string]any{"MaxResults": "2", "NextToken": nextToken})
	require.Equal(t, http.StatusOK, rec2.Code)

	var page2 map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &page2))

	var schedules2 []json.RawMessage
	require.NoError(t, json.Unmarshal(page2["Schedules"], &schedules2))
	assert.Len(t, schedules2, 2)
}

// TestListSchedules_NoTokenReturnsAll verifies an unpaginated ListSchedules call
// returns every schedule with no NextToken.
func TestListSchedules_NoTokenReturnsAll(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	for _, name := range []string{"all-a", "all-b", "all-c"} {
		createBaseSchedule(t, h, name)
	}

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	var schedules []json.RawMessage
	require.NoError(t, json.Unmarshal(out["Schedules"], &schedules))
	assert.Len(t, schedules, 3)
	assert.Nil(t, out["NextToken"])
}

// TestListSchedules_IncludesTargetSummary verifies that each schedule returned by
// ListSchedules includes a Target object with Arn. Real AWS's TargetSummary type
// (used by ScheduleSummary.Target) has only an Arn field -- no RoleArn -- so this
// intentionally does not assert on RoleArn.
func TestListSchedules_IncludesTargetSummary(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "sched-with-target",
		"ScheduleExpression": "rate(5 minutes)",
		"Target": map[string]string{
			"Arn":     "arn:aws:lambda:us-east-1:123:function:fn",
			"RoleArn": "arn:aws:iam::123:role/r",
		},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})

	rec := doSchedulerRequest(t, h, "ListSchedules", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Schedules []map[string]json.RawMessage `json:"Schedules"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Schedules, 1)

	item := out.Schedules[0]
	targetRaw, hasTarget := item["Target"]
	assert.True(t, hasTarget, "ListSchedules item must have a 'Target' field")

	var target struct {
		Arn string `json:"Arn"`
	}
	require.NoError(t, json.Unmarshal(targetRaw, &target))
	assert.Equal(t, "arn:aws:lambda:us-east-1:123:function:fn", target.Arn)

	// Real AWS's TargetSummary has no RoleArn field; assert gopherstack doesn't
	// invent one on the wire.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(targetRaw, &raw))
	_, hasRoleArn := raw["RoleArn"]
	assert.False(t, hasRoleArn, "ListSchedules Target summary must not include RoleArn (not in real TargetSummary)")
}

// TestListSchedules_TargetMatchesGetSchedule verifies that the Target.Arn in the
// list summary matches the Target.Arn in the full GetSchedule response. It does
// not compare RoleArn: real AWS's TargetSummary (used by ListSchedules) has no
// RoleArn field at all, unlike the full Target shape returned by GetSchedule.
func TestListSchedules_TargetMatchesGetSchedule(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	const targetArn = "arn:aws:sqs:us-east-1:999:my-queue"
	const roleArn = "arn:aws:iam::999:role/scheduler"

	doSchedulerRequest(t, h, "CreateSchedule", map[string]any{
		"Name":               "match-sched",
		"ScheduleExpression": "cron(0 12 * * ? *)",
		"Target":             map[string]string{"Arn": targetArn, "RoleArn": roleArn},
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
	})

	listRec := doSchedulerRequest(t, h, "ListSchedules", nil)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listOut struct {
		Schedules []struct {
			Target struct {
				Arn string `json:"Arn"`
			} `json:"Target"`
		} `json:"Schedules"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
	require.Len(t, listOut.Schedules, 1)
	assert.Equal(t, targetArn, listOut.Schedules[0].Target.Arn)
}
