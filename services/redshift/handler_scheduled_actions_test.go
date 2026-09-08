package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// ---- CreateScheduledAction ----

func TestHandler_CreateScheduledAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			body: "Action=CreateScheduledAction&Version=2012-12-01" +
				"&ScheduledActionName=my-action" +
				"&Schedule=cron(0+12+*+*+?+*)" +
				"&IamRole=arn:aws:iam::123456789012:role/MyRole" +
				"&ScheduledActionDescription=Daily+resize",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateScheduledActionResponse", "my-action", "ACTIVE"},
		},
		{
			name:         "missing_name",
			body:         "Action=CreateScheduledAction&Version=2012-12-01&Schedule=cron(0+12+*+*+?+*)",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "duplicate",
			body: "Action=CreateScheduledAction&Version=2012-12-01" +
				"&ScheduledActionName=dup-action" +
				"&Schedule=cron(0+12+*+*+?+*)",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ScheduledActionAlreadyExists"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.name == "duplicate" {
				postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
					"&ScheduledActionName=dup-action&Schedule=cron(0+12+*+*+?+*)")
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DeleteScheduledAction ----

func TestHandler_DeleteScheduledAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "success",
			body:         "Action=DeleteScheduledAction&Version=2012-12-01&ScheduledActionName=my-action",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteScheduledActionResponse"},
		},
		{
			name:         "not_found",
			body:         "Action=DeleteScheduledAction&Version=2012-12-01&ScheduledActionName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ScheduledActionNotFound"},
		},
		{
			name:         "missing_name",
			body:         "Action=DeleteScheduledAction&Version=2012-12-01",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.name == "success" {
				postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
					"&ScheduledActionName=my-action&Schedule=cron(0+12+*+*+?+*)")
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- DescribeScheduledActions ----

func TestHandler_DescribeScheduledActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "empty",
			body:         "Action=DescribeScheduledActions&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeScheduledActionsResponse"},
		},
		{
			name:         "with_data",
			body:         "Action=DescribeScheduledActions&Version=2012-12-01",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeScheduledActionsResponse", "test-action"},
		},
		{
			name:         "filter_by_name",
			body:         "Action=DescribeScheduledActions&Version=2012-12-01&ScheduledActionName=test-action",
			wantCode:     http.StatusOK,
			wantContains: []string{"test-action", "ACTIVE"},
		},
		{
			name:         "filter_not_found",
			body:         "Action=DescribeScheduledActions&Version=2012-12-01&ScheduledActionName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ScheduledActionNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.name == "with_data" || tt.name == "filter_by_name" {
				postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
					"&ScheduledActionName=test-action&Schedule=cron(0+12+*+*+?+*)")
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// ---- ModifyScheduledAction ----

func TestHandler_ModifyScheduledAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success_update_schedule",
			body: "Action=ModifyScheduledAction&Version=2012-12-01" +
				"&ScheduledActionName=my-action" +
				"&Schedule=cron(0+6+*+*+?+*)",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyScheduledActionResponse", "my-action"},
		},
		{
			name: "success_update_description",
			body: "Action=ModifyScheduledAction&Version=2012-12-01" +
				"&ScheduledActionName=my-action" +
				"&ScheduledActionDescription=Updated+description",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyScheduledActionResponse", "my-action"},
		},
		{
			name:         "not_found",
			body:         "Action=ModifyScheduledAction&Version=2012-12-01&ScheduledActionName=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ScheduledActionNotFound"},
		},
		{
			name:         "missing_name",
			body:         "Action=ModifyScheduledAction&Version=2012-12-01&Schedule=cron(0+6+*+*+?+*)",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			if tt.name == "success_update_schedule" || tt.name == "success_update_description" {
				postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
					"&ScheduledActionName=my-action&Schedule=cron(0+12+*+*+?+*)")
			}

			rec := postRedshiftForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestHandler_ScheduledAction_NextInvocations locks in that NextInvocations is
// computed from the real Schedule cron()/at() expression (this backend has no
// pricing-style fallback for this field -- an unparseable expression like
// "rate(1 day)", which real Redshift does not accept for ScheduledAction.Schedule,
// must yield no fabricated NextInvocations at all).
func TestHandler_ScheduledAction_NextInvocations(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()

	// Every minute -- guaranteed at least one NextInvocations entry within a
	// minute of "now", wrapped in the real ScheduledActionTime list element
	// (confirmed against awsAwsquery_deserializeDocumentScheduledActionTimeList
	// in aws-sdk-go-v2/service/redshift@v1.65.4/deserializers.go).
	rec := postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=every-minute&Schedule=cron(*+*+*+*+?+*)")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "<NextInvocations><ScheduledActionTime>")

	rec = postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=future-at&Schedule=at(2099-01-01T00:00:00)")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	const wantFutureInvocation = "<NextInvocations><ScheduledActionTime>2099-01-01T00:00:00Z" +
		"</ScheduledActionTime></NextInvocations>"
	assert.Contains(t, rec.Body.String(), wantFutureInvocation)

	rec = postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=past-at&Schedule=at(2000-01-01T00:00:00)")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "<ScheduledActionTime>")

	rec = postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=unsupported-rate&Schedule=rate(1+day)")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "<ScheduledActionTime>")
}

// ---- Backend tests for ScheduledAction ----

func TestBackend_ScheduledAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *redshift.InMemoryBackend)
		name string
	}{
		{
			name: "create_increments_count",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateScheduledAction(
					"action-1", "cron(0 12 * * ? *)", "arn:aws:iam::123:role/R", "desc", nil, nil,
				)
				require.NoError(t, err)
				assert.Equal(t, 1, redshift.ScheduledActionCount(b))
			},
		},
		{
			name: "delete_decrements_count",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateScheduledAction("action-del", "cron(0 12 * * ? *)", "", "", nil, nil)
				require.NoError(t, err)
				err = b.DeleteScheduledAction("action-del")
				require.NoError(t, err)
				assert.Equal(t, 0, redshift.ScheduledActionCount(b))
			},
		},
		{
			name: "describe_all_returns_all",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateScheduledAction("a1", "cron(0 12 * * ? *)", "", "", nil, nil)
				require.NoError(t, err)
				_, err = b.CreateScheduledAction("a2", "rate(1 day)", "", "", nil, nil)
				require.NoError(t, err)
				actions, err := b.DescribeScheduledActions("")
				require.NoError(t, err)
				assert.Len(t, actions, 2)
			},
		},
		{
			name: "modify_updates_schedule",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateScheduledAction("action-mod", "cron(0 12 * * ? *)", "", "", nil, nil)
				require.NoError(t, err)
				updated, err := b.ModifyScheduledAction("action-mod", "rate(1 hour)", "", "", nil, nil)
				require.NoError(t, err)
				assert.Equal(t, "rate(1 hour)", updated.Schedule)
			},
		},
		{
			name: "modify_updates_description",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateScheduledAction("action-desc", "cron(0 12 * * ? *)", "", "old desc", nil, nil)
				require.NoError(t, err)
				updated, err := b.ModifyScheduledAction("action-desc", "", "", "new desc", nil, nil)
				require.NoError(t, err)
				assert.Equal(t, "new desc", updated.ScheduledActionDescription)
			},
		},
		{
			name: "modify_not_found_returns_error",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.ModifyScheduledAction("nonexistent", "rate(1 day)", "", "", nil, nil)
				require.Error(t, err)
				assert.ErrorIs(t, err, redshift.ErrScheduledActionNotFound)
			},
		},
		{
			name: "duplicate_create_returns_error",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateScheduledAction("action-dup", "cron(0 12 * * ? *)", "", "", nil, nil)
				require.NoError(t, err)
				_, err = b.CreateScheduledAction("action-dup", "rate(1 day)", "", "", nil, nil)
				require.Error(t, err)
				assert.ErrorIs(t, err, redshift.ErrScheduledActionAlreadyExists)
			},
		},
		{
			name: "delete_not_found_returns_error",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				err := b.DeleteScheduledAction("nonexistent")
				require.Error(t, err)
				assert.ErrorIs(t, err, redshift.ErrScheduledActionNotFound)
			},
		},
		{
			name: "state_is_active_on_create",
			run: func(t *testing.T, b *redshift.InMemoryBackend) {
				t.Helper()
				a, err := b.CreateScheduledAction("action-state", "cron(0 12 * * ? *)", "", "", nil, nil)
				require.NoError(t, err)
				assert.Equal(t, "ACTIVE", a.State)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("123456789012", "us-east-1")
			tt.run(t, b)
		})
	}
}

// TestHandler_ScheduledAction_TargetActionRoundTrips locks in that TargetAction --
// the field that determines what a scheduled action actually does -- survives a
// real request/response round trip. Before this fix, CreateScheduledAction/
// ModifyScheduledAction parsed TargetAction as a single flat top-level string (not
// the nested TargetAction.ResizeCluster.* etc. shape real aws-sdk-go-v2 clients
// send), and DescribeScheduledActions/CreateScheduledActionResult never serialized
// TargetAction into the response at all -- it was silently dropped end to end.
func TestHandler_ScheduledAction_TargetActionRoundTrips(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()

	rec := postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=resize-action&Schedule=cron(0+12+*+*+?+*)"+
		"&TargetAction.ResizeCluster.ClusterIdentifier=my-cluster"+
		"&TargetAction.ResizeCluster.NodeType=ra3.4xlarge"+
		"&TargetAction.ResizeCluster.NumberOfNodes=3")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, "<ResizeCluster>")
	assert.Contains(t, body, "<ClusterIdentifier>my-cluster</ClusterIdentifier>")
	assert.Contains(t, body, "<NodeType>ra3.4xlarge</NodeType>")
	assert.Contains(t, body, "<NumberOfNodes>3</NumberOfNodes>")

	// DescribeScheduledActions must also reflect the stored target action.
	rec = postRedshiftForm(t, h, "Action=DescribeScheduledActions&Version=2012-12-01"+
		"&ScheduledActionName=resize-action")
	require.Equal(t, http.StatusOK, rec.Code)
	body = rec.Body.String()
	assert.Contains(t, body, "<ResizeCluster>")
	assert.Contains(t, body, "<ClusterIdentifier>my-cluster</ClusterIdentifier>")
}

// TestHandler_ScheduledAction_Enable locks in that the Enable request parameter
// (unsupported before this fix -- state was always hardcoded to ACTIVE) actually
// controls the resulting State.
func TestHandler_ScheduledAction_Enable(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()

	rec := postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=disabled-action&Schedule=cron(0+12+*+*+?+*)&Enable=false")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<State>DISABLED</State>")

	rec = postRedshiftForm(t, h, "Action=ModifyScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=disabled-action&Enable=true")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<State>ACTIVE</State>")
}

// TestHandler_DescribeScheduledActions_TargetActionTypeFilter locks in
// DescribeScheduledActionsInput.TargetActionType (api_op_DescribeScheduledActions.go:
// "The type of the scheduled actions to retrieve."), which was parsed nowhere in the
// handler and so never narrowed results.
func TestHandler_DescribeScheduledActions_TargetActionTypeFilter(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()

	postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=resize-action&Schedule=cron(0+12+*+*+?+*)"+
		"&TargetAction.ResizeCluster.ClusterIdentifier=my-cluster"+
		"&TargetAction.ResizeCluster.NumberOfNodes=3")
	postRedshiftForm(t, h, "Action=CreateScheduledAction&Version=2012-12-01"+
		"&ScheduledActionName=pause-action&Schedule=cron(0+12+*+*+?+*)"+
		"&TargetAction.PauseCluster.ClusterIdentifier=my-cluster")

	rec := postRedshiftForm(t, h, "Action=DescribeScheduledActions&Version=2012-12-01"+
		"&TargetActionType=ResizeCluster")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, "resize-action")
	assert.NotContains(t, body, "pause-action")

	rec = postRedshiftForm(t, h, "Action=DescribeScheduledActions&Version=2012-12-01"+
		"&TargetActionType=PauseCluster")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body = rec.Body.String()
	assert.Contains(t, body, "pause-action")
	assert.NotContains(t, body, "resize-action")
}
