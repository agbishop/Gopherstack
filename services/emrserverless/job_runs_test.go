package emrserverless_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/emrserverless"
)

// --- StartJobRun auto-start ---

// TestStartJobRun_AutoStartsApplication verifies the documented default
// behavior of aws-sdk-go-v2's types.AutoStartConfig.Enabled: "Enables the
// application to automatically start on job submission. Defaults to true."
// A job run submitted to a CREATED (never-started) application must leave
// the application observably STARTED, not silently stale.
func TestStartJobRun_AutoStartsApplication(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("111111111111", "us-east-1")
	app, err := b.CreateApplication("autostart-app", "SPARK", "emr-6.9.0", "", nil)
	require.NoError(t, err)
	require.Equal(t, emrserverless.ApplicationStateCreated, app.State)

	_, err = b.StartJobRun(app.ApplicationID, "arn:aws:iam::111111111111:role/exec", "job", "", nil)
	require.NoError(t, err)

	got, err := b.GetApplication(app.ApplicationID)
	require.NoError(t, err)
	assert.Equal(t, emrserverless.ApplicationStateStarted, got.State,
		"StartJobRun must auto-start a non-STARTED application when autoStartConfiguration is not explicitly disabled")
}

// TestStartJobRun_RejectsWhenAutoStartDisabled verifies that explicitly
// disabling autoStartConfiguration ("enabled": false) makes StartJobRun
// reject a job run on a non-STARTED application instead of silently
// starting it.
func TestStartJobRun_RejectsWhenAutoStartDisabled(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("111111111111", "us-east-1")
	opts := emrserverless.CreateApplicationOptions{
		ExtraConfig: map[string]any{"autoStartConfiguration": map[string]any{"enabled": false}},
	}
	app, err := b.CreateApplication("no-autostart-app", "SPARK", "emr-6.9.0", "", nil, opts)
	require.NoError(t, err)
	require.Equal(t, emrserverless.ApplicationStateCreated, app.State)

	_, err = b.StartJobRun(app.ApplicationID, "arn:aws:iam::111111111111:role/exec", "job", "", nil)
	require.Error(t, err)

	got, err := b.GetApplication(app.ApplicationID)
	require.NoError(t, err)
	assert.Equal(t, emrserverless.ApplicationStateCreated, got.State,
		"a rejected StartJobRun must not mutate application state")
}

// --- StartJobRun client-token idempotency ---

func TestStartJobRun_ClientTokenIdempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		firstToken  string
		secondToken string
		wantSameRun bool
	}{
		{
			name:        "same_token_replays_same_job_run",
			firstToken:  "jr-tok-1",
			secondToken: "jr-tok-1",
			wantSameRun: true,
		},
		{
			name:        "different_token_creates_new_run",
			firstToken:  "jr-tok-a",
			secondToken: "jr-tok-b",
			wantSameRun: false,
		},
		{
			name:        "no_token_creates_new_run_each_time",
			firstToken:  "",
			secondToken: "",
			wantSameRun: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
			app, err := b.CreateApplication("jr-token-app", "SPARK", "emr-6.6.0", "", nil)
			require.NoError(t, err)

			first, err := b.StartJobRun(app.ApplicationID, "arn:aws:iam::000000000000:role/r", "run", "", nil,
				emrserverless.StartJobRunOptions{ClientToken: tt.firstToken})
			require.NoError(t, err)

			second, err := b.StartJobRun(app.ApplicationID, "arn:aws:iam::000000000000:role/r", "run", "", nil,
				emrserverless.StartJobRunOptions{ClientToken: tt.secondToken})
			require.NoError(t, err)

			if tt.wantSameRun {
				assert.Equal(t, first.JobRunID, second.JobRunID)
				assert.Equal(t, 1, emrserverless.JobRunCount(b))
			} else {
				assert.NotEqual(t, first.JobRunID, second.JobRunID)
				assert.Equal(t, 2, emrserverless.JobRunCount(b))
			}
		})
	}
}

// --- StartJobRun JobDriver / ConfigurationOverrides passthrough ---

func TestStartJobRun_JobDriverPassthrough(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
	app, err := b.CreateApplication("jr-driver-app", "SPARK", "emr-6.6.0", "", nil)
	require.NoError(t, err)

	jobDriver := map[string]any{
		"sparkSubmit": map[string]any{
			"entryPoint":            "s3://bucket/job.py",
			"entryPointArguments":   []any{"--input", "s3://bucket/in"},
			"sparkSubmitParameters": "--conf spark.executor.cores=4",
		},
	}
	configOverrides := map[string]any{
		"applicationConfiguration": []any{
			map[string]any{"classification": "spark-defaults", "properties": map[string]any{"k": "v"}},
		},
	}

	jr, err := b.StartJobRun(app.ApplicationID, "arn:aws:iam::000000000000:role/r", "driver-run", "", nil,
		emrserverless.StartJobRunOptions{JobDriver: jobDriver, ConfigurationOverrides: configOverrides})
	require.NoError(t, err)
	assert.Equal(t, jobDriver, jr.JobDriver)
	assert.Equal(t, configOverrides, jr.ConfigurationOverrides)

	got, err := b.GetJobRun(app.ApplicationID, jr.JobRunID)
	require.NoError(t, err)
	assert.Equal(t, jobDriver, got.JobDriver)
	assert.Equal(t, configOverrides, got.ConfigurationOverrides)
}

// TestStartJobRun_NoJobDriverOmitsField verifies that StartJobRun without a
// jobDriver leaves JobRun.JobDriver nil rather than fabricating a value, so
// jobRunToMap omits the key entirely.
func TestStartJobRun_NoJobDriverOmitsField(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
	app, err := b.CreateApplication("jr-no-driver-app", "SPARK", "emr-6.6.0", "", nil)
	require.NoError(t, err)

	jr, err := b.StartJobRun(app.ApplicationID, "arn:aws:iam::000000000000:role/r", "no-driver-run", "", nil)
	require.NoError(t, err)
	assert.Nil(t, jr.JobDriver)
	assert.Nil(t, jr.ConfigurationOverrides)
}

// --- StartJobRun CreatedBy / ExecutionTimeoutMinutes ---

// TestStartJobRun_CreatedByAndTimeoutDefaults verifies StartJobRun populates
// CreatedBy (required on the real JobRun response shape; this backend uses
// the execution role ARN as a best-effort substitute) and defaults
// ExecutionTimeoutMinutes to 720 (matching the real API's documented
// behavior when no timeout is supplied) rather than leaving them zero.
func TestStartJobRun_CreatedByAndTimeoutDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		executionTimeout     int64
		wantExecutionTimeout int64
	}{
		{name: "unset_defaults_to_720", executionTimeout: 0, wantExecutionTimeout: 720},
		{name: "explicit_value_preserved", executionTimeout: 45, wantExecutionTimeout: 45},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
			app, err := b.CreateApplication("timeout-app", "SPARK", "emr-6.6.0", "", nil)
			require.NoError(t, err)

			jr, err := b.StartJobRun(app.ApplicationID, "arn:aws:iam::000000000000:role/creator", "run", "", nil,
				emrserverless.StartJobRunOptions{ExecutionTimeoutMinutes: tt.executionTimeout})
			require.NoError(t, err)

			assert.Equal(t, "arn:aws:iam::000000000000:role/creator", jr.CreatedBy)
			assert.Equal(t, tt.wantExecutionTimeout, jr.ExecutionTimeoutMinutes)

			got, err := b.GetJobRun(app.ApplicationID, jr.JobRunID)
			require.NoError(t, err)
			assert.Equal(t, "arn:aws:iam::000000000000:role/creator", got.CreatedBy)
			assert.Equal(t, tt.wantExecutionTimeout, got.ExecutionTimeoutMinutes)
		})
	}
}

// TestStartJobRun_ExecutionIamPolicyAndRetryPolicyPassthrough verifies
// ExecutionIamPolicy and RetryPolicy (real StartJobRunInput fields) are
// stored and echoed back by GetJobRun, with independent clone semantics
// matching JobDriver/ConfigurationOverrides.
func TestStartJobRun_ExecutionIamPolicyAndRetryPolicyPassthrough(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
	app, err := b.CreateApplication("policy-retry-app", "SPARK", "emr-6.6.0", "", nil)
	require.NoError(t, err)

	execPolicy := map[string]any{"policy": "some-policy-json"}
	retryPolicy := map[string]any{"maxAttempts": float64(5)}

	jr, err := b.StartJobRun(app.ApplicationID, "arn:aws:iam::000000000000:role/r", "policy-run", "", nil,
		emrserverless.StartJobRunOptions{ExecutionIamPolicy: execPolicy, RetryPolicy: retryPolicy})
	require.NoError(t, err)
	assert.Equal(t, execPolicy, jr.ExecutionIamPolicy)
	assert.Equal(t, retryPolicy, jr.RetryPolicy)

	got, err := b.GetJobRun(app.ApplicationID, jr.JobRunID)
	require.NoError(t, err)
	assert.Equal(t, execPolicy, got.ExecutionIamPolicy)
	assert.Equal(t, retryPolicy, got.RetryPolicy)
}

// --- JobRun ops when no runs exist for an application ---

// TestJobRunOps_NoRunsForApp verifies GetJobRun, GetDashboardForJobRun, and
// CancelJobRun all return ErrNotFound (rather than panicking) when the
// application has no job runs at all.
func TestJobRunOps_NoRunsForApp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		call func(b *emrserverless.InMemoryBackend, appID string) error
		name string
	}{
		{
			name: "GetJobRun",
			call: func(b *emrserverless.InMemoryBackend, appID string) error {
				_, err := b.GetJobRun(appID, "nonexistent-run")

				return err
			},
		},
		{
			name: "GetDashboardForJobRun",
			call: func(b *emrserverless.InMemoryBackend, appID string) error {
				_, err := b.GetDashboardForJobRun(appID, "nonexistent-run")

				return err
			},
		},
		{
			name: "CancelJobRun",
			call: func(b *emrserverless.InMemoryBackend, appID string) error {
				_, err := b.CancelJobRun(appID, "nonexistent-run")

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
			app, err := b.CreateApplication("no-runs-app", "SPARK", "emr-6.6.0", "", nil)
			require.NoError(t, err)

			err = tt.call(b, app.ApplicationID)
			require.Error(t, err)
			assert.ErrorIs(t, err, emrserverless.ErrNotFound)
		})
	}
}

// --- CancelJobRun terminal state rejection ---

func TestCancelJobRun_RejectsTerminalState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state string
	}{
		{"already_cancelled", emrserverless.JobRunStateCancelled},
		{"already_succeeded", emrserverless.JobRunStateSuccess},
		{"already_failed", emrserverless.JobRunStateFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
			appID := "app-cancel-state"
			now := time.Now().UTC()

			app := &emrserverless.Application{
				ApplicationID: appID,
				Arn:           "arn:aws:emr-serverless:us-east-1:000000000000:/applications/" + appID,
				Name:          "cancel-state-app",
				Type:          "SPARK",
				State:         emrserverless.ApplicationStateStarted,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			b.AddApplicationInternal(app)

			jr := &emrserverless.JobRun{
				ApplicationID:    appID,
				JobRunID:         "jr-terminal",
				Arn:              "arn:aws:emr-serverless:us-east-1:000000000000:/applications/" + appID + "/jobruns/jr-terminal",
				Name:             "terminal-run",
				State:            tt.state,
				ExecutionRoleArn: "arn:aws:iam::000000000000:role/role",
				CreatedAt:        now,
				UpdatedAt:        now,
			}
			b.AddJobRunInternal(jr)

			_, err := b.CancelJobRun(appID, "jr-terminal")
			require.Error(t, err)
			assert.ErrorIs(t, err, emrserverless.ErrInvalidState)
		})
	}
}

// --- Non-nil tags ---

func TestNonNilTags_JobRun(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateApplication("jr-notag-app", "SPARK", "emr-6.6.0", "", nil)
	require.NoError(t, err)
	apps, _ := b.ListApplications("", 0)
	require.Len(t, apps, 1)
	appID := apps[0].ApplicationID

	jr, err := b.StartJobRun(appID, "arn:aws:iam::000000000000:role/r", "test-run", "", nil)
	require.NoError(t, err)
	assert.NotNil(t, jr.Tags)
}

// --- ListJobRuns is sorted ---

func TestSortedListJobRuns(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
	app, err := b.CreateApplication("sorted-runs-app", "SPARK", "emr-6.6.0", "", nil)
	require.NoError(t, err)

	now := time.Now().UTC()

	for _, runID := range []string{"zzz-run", "aaa-run", "mmm-run"} {
		arnVal := "arn:aws:emr-serverless:us-east-1:000000000000:/applications/" +
			app.ApplicationID + "/jobruns/" + runID
		b.AddJobRunInternal(&emrserverless.JobRun{
			ApplicationID:    app.ApplicationID,
			JobRunID:         runID,
			Arn:              arnVal,
			Name:             "run-" + runID,
			State:            emrserverless.JobRunStateRunning,
			ExecutionRoleArn: "arn:aws:iam::000000000000:role/r",
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}

	runs, _, err := b.ListJobRuns(app.ApplicationID, "", 0)
	require.NoError(t, err)
	require.Len(t, runs, 3)
	assert.Equal(t, "aaa-run", runs[0].JobRunID)
	assert.Equal(t, "mmm-run", runs[1].JobRunID)
	assert.Equal(t, "zzz-run", runs[2].JobRunID)
}

// --- JobRunState enum matches the real SDK ---

// TestJobRunStateConstants_MatchSDK guards against gopherstack's JobRunState
// constants silently drifting from types.JobRunState.Values() in
// aws-sdk-go-v2/service/emrserverless@v1.44.4 (types/enums.go:76-84): a
// client deserialising a job run this backend did not create (e.g. seeded
// via AddJobRunInternal, or in a future lifecycle-simulation pass) must not
// hit a state gopherstack has no constant for.
func TestJobRunStateConstants_MatchSDK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"submitted", emrserverless.JobRunStateSubmitted, "SUBMITTED"},
		{"pending", emrserverless.JobRunStatePending, "PENDING"},
		{"scheduled", emrserverless.JobRunStateScheduled, "SCHEDULED"},
		{"running", emrserverless.JobRunStateRunning, "RUNNING"},
		{"success", emrserverless.JobRunStateSuccess, "SUCCESS"},
		{"failed", emrserverless.JobRunStateFailed, "FAILED"},
		{"cancelling", emrserverless.JobRunStateCancelling, "CANCELLING"},
		{"cancelled", emrserverless.JobRunStateCancelled, "CANCELLED"},
		{"queued", emrserverless.JobRunStateQueued, "QUEUED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.got)
		})
	}
}
