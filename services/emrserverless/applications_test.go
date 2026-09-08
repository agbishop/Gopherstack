package emrserverless_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/emrserverless"
)

// --- CreateApplication client-token idempotency ---

func TestCreateApplication_ClientTokenIdempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		firstToken   string
		secondToken  string
		wantSameApp  bool
		wantConflict bool
	}{
		{
			name:        "same_token_replays_same_application",
			firstToken:  "tok-1",
			secondToken: "tok-1",
			wantSameApp: true,
		},
		{
			name:         "different_token_same_name_conflicts",
			firstToken:   "tok-a",
			secondToken:  "tok-b",
			wantConflict: true,
		},
		{
			name:         "no_token_on_retry_conflicts",
			firstToken:   "tok-only-first",
			secondToken:  "",
			wantConflict: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")

			first, err := b.CreateApplication("idempotent-app", "SPARK", "emr-6.6.0", "", nil,
				emrserverless.CreateApplicationOptions{ClientToken: tt.firstToken})
			require.NoError(t, err)

			second, err := b.CreateApplication("idempotent-app", "SPARK", "emr-6.6.0", "", nil,
				emrserverless.CreateApplicationOptions{ClientToken: tt.secondToken})

			switch {
			case tt.wantConflict:
				require.ErrorIs(t, err, emrserverless.ErrAlreadyExists)
			case tt.wantSameApp:
				require.NoError(t, err)
				assert.Equal(t, first.ApplicationID, second.ApplicationID)
				assert.Equal(t, first.Arn, second.Arn)
			}

			assert.Equal(t, 1, emrserverless.ApplicationCount(b))
		})
	}
}

// TestCreateApplication_StaleTokenFallsThrough verifies that a clientToken
// pointing at an application which no longer exists (deleted after the
// original create) is treated as a cache miss rather than resurrecting a
// stale reference or erroring.
func TestCreateApplication_StaleTokenFallsThrough(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")

	first, err := b.CreateApplication("stale-token-app", "SPARK", "emr-6.6.0", "", nil,
		emrserverless.CreateApplicationOptions{ClientToken: "reused-token"})
	require.NoError(t, err)
	require.NoError(t, b.DeleteApplication(first.ApplicationID))

	second, err := b.CreateApplication("stale-token-app", "SPARK", "emr-6.6.0", "", nil,
		emrserverless.CreateApplicationOptions{ClientToken: "reused-token"})
	require.NoError(t, err)
	assert.NotEqual(t, first.ApplicationID, second.ApplicationID)
}

// --- CreateApplication ExtraConfig passthrough ---

func TestCreateApplication_ExtraConfigPassthrough(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")

	extra := map[string]any{
		"maximumCapacity": map[string]any{"cpu": "400 vCPU", "memory": "3000 GB"},
		"initialCapacity": map[string]any{
			"Driver": map[string]any{"workerCount": float64(2)},
		},
	}

	app, err := b.CreateApplication("extra-config-app", "SPARK", "emr-6.6.0", "", nil,
		emrserverless.CreateApplicationOptions{ExtraConfig: extra})
	require.NoError(t, err)
	require.Equal(t, extra["maximumCapacity"], app.ExtraConfig["maximumCapacity"])
	require.Equal(t, extra["initialCapacity"], app.ExtraConfig["initialCapacity"])

	got, err := b.GetApplication(app.ApplicationID)
	require.NoError(t, err)
	assert.Equal(t, extra["maximumCapacity"], got.ExtraConfig["maximumCapacity"])

	// Mutating the caller's map after the call, and mutating the returned
	// clone, must not affect the stored backend state (clone independence).
	extra["maximumCapacity"].(map[string]any)["cpu"] = "mutated"
	got.ExtraConfig["maximumCapacity"].(map[string]any)["cpu"] = "also-mutated"

	again, err := b.GetApplication(app.ApplicationID)
	require.NoError(t, err)
	assert.Equal(t, "400 vCPU", again.ExtraConfig["maximumCapacity"].(map[string]any)["cpu"])
}

// TestCreateApplication_RequiresFields verifies CreateApplication rejects a
// missing name or type with ErrValidation.
func TestCreateApplication_RequiresFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		appName string
		appType string
	}{
		{name: "requires_name", appName: "", appType: "SPARK"},
		{name: "requires_type", appName: "my-app", appType: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateApplication(tt.appName, tt.appType, "emr-6.6.0", "", nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, emrserverless.ErrValidation)
		})
	}
}

// --- StartApplication state transitions ---

// TestStartApplication_RejectsInvalidStates verifies StartApplication returns
// an error for states that cannot transition to STARTED.
func TestStartApplication_RejectsInvalidStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fromState string
		wantErr   bool
	}{
		{name: "from_stopped", fromState: emrserverless.ApplicationStateStopped, wantErr: false},
		{name: "from_terminated", fromState: emrserverless.ApplicationStateTerminated, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("111111111111", "us-east-1")
			app, err := b.CreateApplication("state-test", "SPARK", "emr-6.9.0", "", nil)
			require.NoError(t, err)

			emrserverless.SetApplicationState(b, app.ApplicationID, tt.fromState)

			startErr := b.StartApplication(app.ApplicationID)
			if tt.wantErr {
				assert.Error(t, startErr)
			} else {
				assert.NoError(t, startErr)
			}
		})
	}
}

// --- DeleteApplication state transitions ---

// TestDeleteApplication_RejectsActiveStates verifies DeleteApplication
// rejects applications that have not been stopped.
func TestDeleteApplication_RejectsActiveStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fromState string
		wantErr   bool
	}{
		{name: "from_stopped", fromState: emrserverless.ApplicationStateStopped, wantErr: false},
		{name: "from_created", fromState: emrserverless.ApplicationStateCreated, wantErr: false},
		{name: "from_started", fromState: emrserverless.ApplicationStateStarted, wantErr: true},
		{name: "from_starting", fromState: emrserverless.ApplicationStateStarting, wantErr: true},
		{name: "from_stopping", fromState: emrserverless.ApplicationStateStopping, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("111111111111", "us-east-1")
			app, err := b.CreateApplication("del-state-test", "SPARK", "emr-6.9.0", "", nil)
			require.NoError(t, err)

			emrserverless.SetApplicationState(b, app.ApplicationID, tt.fromState)

			delErr := b.DeleteApplication(app.ApplicationID)
			if tt.wantErr {
				assert.Error(t, delErr)
			} else {
				assert.NoError(t, delErr)
			}
		})
	}
}

// --- StopApplication job-run precondition ---

// TestStopApplication_RejectsWithActiveJobRuns verifies the documented
// precondition on aws-sdk-go-v2's api_op_StopApplication.go ("All scheduled
// and running jobs must be completed or cancelled before stopping an
// application").
func TestStopApplication_RejectsWithActiveJobRuns(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("111111111111", "us-east-1")
	app, err := b.CreateApplication("stop-precondition-app", "SPARK", "emr-6.9.0", "", nil)
	require.NoError(t, err)

	jr, err := b.StartJobRun(app.ApplicationID, "arn:aws:iam::111111111111:role/exec", "job", "", nil)
	require.NoError(t, err)

	stopErr := b.StopApplication(app.ApplicationID)
	require.Error(t, stopErr, "StopApplication must reject while a job run is not in a terminal state")

	_, err = b.CancelJobRun(app.ApplicationID, jr.JobRunID)
	require.NoError(t, err)

	require.NoError(t, b.StopApplication(app.ApplicationID))
}

// --- Non-nil tags ---

func TestNonNilTags_Application(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
	app, err := b.CreateApplication("no-tag-app", "SPARK", "emr-6.6.0", "", nil)
	require.NoError(t, err)
	assert.NotNil(t, app.Tags)
}

// --- Sorted ListApplications ---

func TestSortedListApplications(t *testing.T) {
	t.Parallel()

	b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")

	// Insert via seed helper with deterministic IDs.
	now := time.Now().UTC()

	for _, id := range []string{"zzz", "aaa", "mmm"} {
		b.AddApplicationInternal(&emrserverless.Application{
			ApplicationID: id,
			Arn:           "arn:aws:emr-serverless:us-east-1:000000000000:/applications/" + id,
			Name:          "app-" + id,
			Type:          "SPARK",
			State:         emrserverless.ApplicationStateCreated,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}

	apps, _ := b.ListApplications("", 0)
	require.Len(t, apps, 3)
	assert.Equal(t, "aaa", apps[0].ApplicationID)
	assert.Equal(t, "mmm", apps[1].ApplicationID)
	assert.Equal(t, "zzz", apps[2].ApplicationID)
}
