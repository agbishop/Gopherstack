package databrew_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Schedule backend ----

func TestCreateSchedule_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	sc, err := b.CreateSchedule(
		context.Background(),
		"my-schedule",
		[]string{"job1", "job2"},
		"cron(0 12 * * ? *)",
		map[string]string{"env": "prod"},
	)
	require.NoError(t, err)
	assert.Equal(t, "my-schedule", sc.Name)
	assert.Equal(t, "cron(0 12 * * ? *)", sc.CronExpression)
	assert.NotEmpty(t, sc.Arn)
	assert.Len(t, sc.JobNames, 2)
	assert.Equal(t, "prod", sc.Tags["env"])
}

func TestCreateSchedule_EmptyName(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateSchedule(context.Background(), "", nil, "cron(...)", nil)
	require.Error(t, err)
}

func TestCreateSchedule_Duplicate(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateSchedule(context.Background(), "sc", nil, "cron(...)", nil)
	require.NoError(t, err)
	_, err = b.CreateSchedule(context.Background(), "sc", nil, "cron(...)", nil)
	require.Error(t, err)
}

func TestDescribeSchedule_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateSchedule(
		context.Background(),
		"sc1",
		[]string{"j1"},
		"cron(0 8 * * ? *)",
		nil,
	)
	require.NoError(t, err)
	sc, err := b.DescribeSchedule(context.Background(), "sc1")
	require.NoError(t, err)
	assert.Equal(t, "sc1", sc.Name)
	assert.Equal(t, []string{"j1"}, sc.JobNames)
}

func TestDescribeSchedule_NotFound(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.DescribeSchedule(context.Background(), "no-such")
	require.Error(t, err)
}

func TestListSchedules(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateSchedule(context.Background(), "sc1", nil, "cron(...)", nil)
	require.NoError(t, err)
	_, err = b.CreateSchedule(context.Background(), "sc2", nil, "cron(...)", nil)
	require.NoError(t, err)
	list, _ := b.ListSchedules(context.Background(), 100, "")
	assert.Len(t, list, 2)
}

func TestUpdateSchedule_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateSchedule(
		context.Background(),
		"upd-sc",
		[]string{"j1"},
		"cron(0 8 * * ? *)",
		nil,
	)
	require.NoError(t, err)
	err = b.UpdateSchedule(
		context.Background(),
		"upd-sc",
		[]string{"j1", "j2"},
		"cron(0 12 * * ? *)",
	)
	require.NoError(t, err)
	sc, err := b.DescribeSchedule(context.Background(), "upd-sc")
	require.NoError(t, err)
	assert.Equal(t, "cron(0 12 * * ? *)", sc.CronExpression)
	assert.Len(t, sc.JobNames, 2)
}

func TestUpdateSchedule_NotFound(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	err := b.UpdateSchedule(context.Background(), "no-such", nil, "")
	require.Error(t, err)
}

// TestUpdateSchedule_OmittedJobNamesPreservesExisting verifies a caller
// updating only CronExpression does not have JobNames clobbered:
// UpdateScheduleInput's JobNames member has no "This member is required"
// marker (only CronExpression and Name do), so omitting it must leave the
// schedule's existing job list intact.
func TestUpdateSchedule_OmittedJobNamesPreservesExisting(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	ctx := context.Background()
	_, err := b.CreateSchedule(ctx, "upd-sc-nojobs", []string{"j1"}, "cron(0 8 * * ? *)", nil)
	require.NoError(t, err)

	err = b.UpdateSchedule(ctx, "upd-sc-nojobs", nil, "cron(0 12 * * ? *)")
	require.NoError(t, err)

	sc, err := b.DescribeSchedule(ctx, "upd-sc-nojobs")
	require.NoError(t, err)
	assert.Equal(t, "cron(0 12 * * ? *)", sc.CronExpression)
	require.Len(t, sc.JobNames, 1, "omitting JobNames must not clobber the existing job list")
	assert.Equal(t, "j1", sc.JobNames[0])
}

func TestDeleteSchedule_Success(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	_, err := b.CreateSchedule(context.Background(), "del-sc", nil, "cron(...)", nil)
	require.NoError(t, err)
	err = b.DeleteSchedule(context.Background(), "del-sc")
	require.NoError(t, err)
	_, err = b.DescribeSchedule(context.Background(), "del-sc")
	require.Error(t, err)
}

func TestDeleteSchedule_NotFound(t *testing.T) {
	t.Parallel()
	b := newTestBackend()
	err := b.DeleteSchedule(context.Background(), "no-such")
	require.Error(t, err)
}

// ---- Schedule handler ----

func TestHandlerCreateSchedule(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(t, h, http.MethodPost, "/databrew/v1/schedules", map[string]any{
		"Name":           "my-schedule",
		"CronExpression": "cron(0 12 * * ? *)",
		"JobNames":       []string{"job1"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "my-schedule", resp["Name"])
}

func TestHandlerDescribeSchedule(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/schedules", map[string]any{
		"Name": "sc1", "CronExpression": "cron(...)",
	})
	rec := databrewReq(t, h, http.MethodGet, "/databrew/v1/schedules/sc1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerDescribeSchedule_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(t, h, http.MethodGet, "/databrew/v1/schedules/no-such", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlerListSchedules(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(t, h, http.MethodGet, "/databrew/v1/schedules", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["Schedules"])
}

func TestHandlerUpdateSchedule(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/schedules", map[string]any{
		"Name": "upd-sc", "CronExpression": "cron(old)",
	})
	rec := databrewReq(t, h, http.MethodPut, "/databrew/v1/schedules/upd-sc", map[string]any{
		"CronExpression": "cron(new)",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerDeleteSchedule(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	databrewReq(t, h, http.MethodPost, "/databrew/v1/schedules", map[string]any{
		"Name": "del-sc", "CronExpression": "cron(...)",
	})
	rec := databrewReq(t, h, http.MethodDelete, "/databrew/v1/schedules/del-sc", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerDeleteSchedule_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	rec := databrewReq(t, h, http.MethodDelete, "/databrew/v1/schedules/no-such", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
