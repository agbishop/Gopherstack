package glue_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glue"
)

// TestJobRun_ReachesTimeoutState proves JobRun.Timeout actually drives job-run
// lifecycle instead of only being echoed back on Describe -- gopherstack-fc4r.
// A run configured with a Timeout smaller than its natural completion window
// reaches TIMEOUT instead of completing normally; a run with no meaningful
// timeout still succeeds, unaffected.
func TestJobRun_ReachesTimeoutState(t *testing.T) {
	t.Parallel()

	t.Run("timeout_reached_before_natural_completion", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := glue.NewInMemoryBackend("000000000000", "us-east-1")
			t.Cleanup(b.Close)

			_, err := b.CreateJob(glue.Job{
				Name:    "j",
				Role:    "arn:aws:iam::000000000000:role/glue",
				Command: glue.JobCommand{Name: "glueetl"},
			})
			require.NoError(t, err)

			run, err := b.StartJobRunWithOptions("j", nil, glue.StartJobRunOptions{Timeout: 1})
			require.NoError(t, err)

			// Past STARTING->RUNNING (150ms) and the scaled Timeout deadline,
			// but well before natural RUNNING->SUCCEEDED completion (450ms).
			time.Sleep(200 * time.Millisecond)

			got, err := b.GetJobRun("j", run.ID)
			require.NoError(t, err)
			assert.Equal(t, "TIMEOUT", got.JobRunState)
			assert.NotZero(t, got.CompletedOn)
			assert.Equal(t, 60, got.ExecutionTime)
			assert.NotEmpty(t, got.ErrorMessage)
		})
	})

	t.Run("ordinary_timeout_does_not_prevent_success", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := glue.NewInMemoryBackend("000000000000", "us-east-1")
			t.Cleanup(b.Close)

			_, err := b.CreateJob(glue.Job{
				Name:    "j",
				Role:    "arn:aws:iam::000000000000:role/glue",
				Command: glue.JobCommand{Name: "glueetl"},
			})
			require.NoError(t, err)

			run, err := b.StartJobRunWithOptions("j", nil, glue.StartJobRunOptions{Timeout: 60})
			require.NoError(t, err)

			// Past natural completion (450ms). A 60-minute Timeout is six
			// times the notional run length, so it must not bite.
			time.Sleep(500 * time.Millisecond)

			got, err := b.GetJobRun("j", run.ID)
			require.NoError(t, err)
			assert.Equal(t, "SUCCEEDED", got.JobRunState)
		})
	})
}
