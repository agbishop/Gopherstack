package amplify_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/amplify"
)

// TestInMemoryBackend_StartJob_InvalidJobType verifies real Amplify's
// server-side JobType enum validation: StartJob rejects any value outside
// RELEASE/RETRY/MANUAL/WEB_HOOK with a BadRequestException.
func TestInMemoryBackend_StartJob_InvalidJobType(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app := seedApp(t, b, "BadJobTypeApp")
	branch := seedMainBranch(t, b, app.AppID)

	_, err := b.StartJob(app.AppID, branch.BranchName, "NOT_A_REAL_TYPE", "", "", "", time.Time{})
	require.Error(t, err)
	require.ErrorIs(t, err, awserr.ErrInvalidParameter)
}

// TestInMemoryBackend_StartJob_RetryRequiresJobID verifies real Amplify's
// StartJobInput validation: jobId is required when jobType is RETRY.
func TestInMemoryBackend_StartJob_RetryRequiresJobID(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app := seedApp(t, b, "RetryApp")
	branch := seedMainBranch(t, b, app.AppID)

	_, err := b.StartJob(app.AppID, branch.BranchName, "RETRY", "", "", "", time.Time{})
	require.Error(t, err)
	require.ErrorIs(t, err, awserr.ErrInvalidParameter)
}

// TestInMemoryBackend_StartJob_RetryInheritsCommitInfo verifies a RETRY job
// that names an existing prior job inherits that job's commit metadata when
// the caller doesn't supply its own -- matching real Amplify's "retry the
// same commit" semantics for the RETRY job type.
func TestInMemoryBackend_StartJob_RetryInheritsCommitInfo(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app := seedApp(t, b, "RetryInheritApp")
	branch := seedMainBranch(t, b, app.AppID)

	commitTime := time.Unix(1700000000, 0).UTC()

	original, setupErr := b.StartJob(
		app.AppID, branch.BranchName, "RELEASE", "", "abc123", "original commit", commitTime,
	)
	require.NoError(t, setupErr)

	t.Run("inherits_when_caller_omits_commit_fields", func(t *testing.T) {
		t.Parallel()

		retry, err := b.StartJob(
			app.AppID, branch.BranchName, "RETRY", original.JobID, "", "", time.Time{},
		)
		require.NoError(t, err)

		assert.Equal(t, "abc123", retry.CommitID)
		assert.Equal(t, "original commit", retry.CommitMsg)
		assert.Equal(t, commitTime.Unix(), retry.CommitTime.Unix())
		assert.Equal(t, amplify.JobTypeRetry, retry.Type)
		assert.NotEqual(t, original.JobID, retry.JobID, "retry creates a new job, not an in-place mutation")
	})

	t.Run("caller_supplied_commit_fields_win", func(t *testing.T) {
		t.Parallel()

		retry, err := b.StartJob(
			app.AppID, branch.BranchName, "RETRY", original.JobID, "def456", "override", time.Time{},
		)
		require.NoError(t, err)

		assert.Equal(t, "def456", retry.CommitID)
		assert.Equal(t, "override", retry.CommitMsg)
	})

	t.Run("nonexistent_prior_job_still_starts_fresh", func(t *testing.T) {
		t.Parallel()

		retry, err := b.StartJob(
			app.AppID, branch.BranchName, "RETRY", "does-not-exist", "fresh-commit", "fresh msg", time.Time{},
		)
		require.NoError(t, err, "a RETRY jobId that no longer exists is not itself an error")
		assert.Equal(t, "fresh-commit", retry.CommitID)
	})
}

// TestInMemoryBackend_StartJob_CommitTime verifies StartJob's commitTime
// parameter (real Amplify's StartJobInput.CommitTime / JobSummary.CommitTime,
// previously unmodeled -- see PARITY.md) round-trips onto the created Job,
// and is left the zero time (omitted from the wire view) when not supplied.
func TestInMemoryBackend_StartJob_CommitTime(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app := seedApp(t, b, "CommitTimeApp")
	branch := seedMainBranch(t, b, app.AppID)

	t.Run("zero_value_when_omitted", func(t *testing.T) {
		t.Parallel()

		job, err := b.StartJob(app.AppID, branch.BranchName, "RELEASE", "", "", "", time.Time{})
		require.NoError(t, err)
		assert.True(t, job.CommitTime.IsZero())
	})

	t.Run("round_trips_when_supplied", func(t *testing.T) {
		t.Parallel()

		commitTime := time.Unix(1700000000, 0).UTC()
		job, err := b.StartJob(app.AppID, branch.BranchName, "RELEASE", "", "abc", "msg", commitTime)
		require.NoError(t, err)
		assert.Equal(t, commitTime.Unix(), job.CommitTime.Unix())
	})
}

// TestInMemoryBackend_StartJob_UnknownAppOrBranch verifies StartJob's
// not-found checks fire before enum/RETRY validation would otherwise mask
// them -- a caller pointing at a nonexistent app/branch should see
// NotFoundException, not a validation error about a field it didn't even
// reach yet.
func TestInMemoryBackend_StartJob_UnknownAppOrBranch(t *testing.T) {
	t.Parallel()

	t.Run("unknown_app", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()

		_, err := b.StartJob("nonexistent", "main", "RELEASE", "", "", "", time.Time{})
		require.Error(t, err)
		require.ErrorIs(t, err, awserr.ErrNotFound)
	})

	t.Run("unknown_branch", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		app := seedApp(t, b, "NoBranchApp")

		_, err := b.StartJob(app.AppID, "nonexistent", "RELEASE", "", "", "", time.Time{})
		require.Error(t, err)
		require.ErrorIs(t, err, awserr.ErrNotFound)
	})
}

// TestInMemoryBackend_StopJob_RejectsTerminalJob verifies real Amplify's
// StopJob doc ("[s]tops a job that is in progress") is enforced: a job
// already in a terminal state can't be stopped, so its recorded outcome
// (SUCCEED/FAILED/CANCELLED) is never silently overwritten.
func TestInMemoryBackend_StopJob_RejectsTerminalJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		terminate func(t *testing.T, b *amplify.InMemoryBackend, appID, branchName, jobID string)
		name      string
	}{
		{
			name: "already_cancelled",
			terminate: func(t *testing.T, b *amplify.InMemoryBackend, appID, branchName, jobID string) {
				t.Helper()

				_, err := b.StopJob(appID, branchName, jobID)
				require.NoError(t, err)
			},
		},
		{
			name: "already_succeeded_via_janitor",
			terminate: func(t *testing.T, b *amplify.InMemoryBackend, _, _, _ string) {
				t.Helper()

				j := amplify.NewJanitor(b, time.Millisecond)
				j.SweepOnce(t.Context())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			app := seedApp(t, b, "StopTerminalApp-"+tt.name)
			branch := seedMainBranch(t, b, app.AppID)

			job, err := b.StartJob(app.AppID, branch.BranchName, "RELEASE", "", "", "", time.Time{})
			require.NoError(t, err)

			tt.terminate(t, b, app.AppID, branch.BranchName, job.JobID)

			_, err = b.StopJob(app.AppID, branch.BranchName, job.JobID)
			require.Error(t, err)
			require.ErrorIs(t, err, awserr.ErrInvalidParameter)
		})
	}
}
