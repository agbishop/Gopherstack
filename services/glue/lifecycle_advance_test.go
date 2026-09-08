package glue_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glue"
)

// TestCrawlerReadPaths_ReflectLiveStateAfterTransition proves that
// BatchGetCrawlers and GetCrawlerMetrics observe the RUNNING->READY
// transition the same way GetCrawler/GetCrawlers already do. GetCrawler and
// GetCrawlers call advanceStates before reading (see reconciler.go's doc
// comment on advanceStates), so the crawl completion is never stale there --
// but BatchGetCrawlers and GetCrawlerMetrics read Crawler.State/
// CrawlerMetrics.StillEstimating directly with no such call, so a caller who
// only ever calls these two ops sees a crawl that finished 200ms+ ago as
// still running.
func TestCrawlerReadPaths_ReflectLiveStateAfterTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, b *glue.InMemoryBackend, name string)
		name  string
	}{
		{
			name: "batch_get_crawlers",
			check: func(t *testing.T, b *glue.InMemoryBackend, name string) {
				t.Helper()

				found, missing := b.BatchGetCrawlers([]string{name})
				require.Empty(t, missing)
				require.Len(t, found, 1)
				assert.Equal(t, "READY", found[0].State)
			},
		},
		{
			name: "get_crawler_metrics",
			check: func(t *testing.T, b *glue.InMemoryBackend, name string) {
				t.Helper()

				metrics := b.GetCrawlerMetrics([]string{name})
				require.Len(t, metrics, 1)
				assert.False(t, metrics[0].StillEstimating)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := glue.NewInMemoryBackend("000000000000", "us-east-1")
				t.Cleanup(b.Close)

				_, err := b.CreateCrawler("cr", "arn:aws:iam::000000000000:role/glue", "", glue.CrawlerTarget{}, nil)
				require.NoError(t, err)
				require.NoError(t, b.StartCrawler("cr"))

				time.Sleep(300 * time.Millisecond) // past crawlerTransitionDelay (200ms)

				tc.check(t, b, "cr")
			})
		})
	}
}

// TestCrawlerMutationGuards_RespectLiveStateAfterTransition proves that
// StartCrawler/StopCrawler/DeleteCrawler/UpdateCrawlerWithOptions decide
// against the live crawler state, not a stale RUNNING snapshot left over
// from before the crawl finished. Each op checks c.State to accept or reject
// the call; without advanceStates first, a crawl that finished 200ms+ ago
// still reads as RUNNING, so a Start would be wrongly rejected and a
// Stop/Delete/Update would be wrongly allowed (or wrongly rejected) against
// the finished crawler.
func TestCrawlerMutationGuards_RespectLiveStateAfterTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		act  func(b *glue.InMemoryBackend, name string) error
		name string
	}{
		{
			name: "start_crawler_restart",
			act: func(b *glue.InMemoryBackend, name string) error {
				return b.StartCrawler(name)
			},
		},
		{
			name: "delete_crawler",
			act: func(b *glue.InMemoryBackend, name string) error {
				return b.DeleteCrawler(name)
			},
		},
		{
			name: "update_crawler",
			act: func(b *glue.InMemoryBackend, name string) error {
				return b.UpdateCrawlerWithOptions(
					name, "arn:aws:iam::000000000000:role/glue", "", glue.CrawlerTarget{}, glue.CrawlerOptions{},
				)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := glue.NewInMemoryBackend("000000000000", "us-east-1")
				t.Cleanup(b.Close)

				_, err := b.CreateCrawler("cr", "arn:aws:iam::000000000000:role/glue", "", glue.CrawlerTarget{}, nil)
				require.NoError(t, err)
				require.NoError(t, b.StartCrawler("cr"))

				time.Sleep(300 * time.Millisecond) // past crawlerTransitionDelay (200ms)

				assert.NoError(t, tc.act(b, "cr"))
			})
		})
	}
}

// TestStopCrawler_RejectsAfterCompletion proves StopCrawler is rejected once
// the crawl has genuinely finished, rather than succeeding against a stale
// RUNNING read and forcing an already-READY crawler into STOPPING.
func TestStopCrawler_RejectsAfterCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := glue.NewInMemoryBackend("000000000000", "us-east-1")
		t.Cleanup(b.Close)

		_, err := b.CreateCrawler("cr", "arn:aws:iam::000000000000:role/glue", "", glue.CrawlerTarget{}, nil)
		require.NoError(t, err)
		require.NoError(t, b.StartCrawler("cr"))

		time.Sleep(300 * time.Millisecond) // past crawlerTransitionDelay (200ms)

		require.ErrorIs(t, b.StopCrawler("cr"), glue.ErrCrawlerNotRunning)

		c, err := b.GetCrawler("cr")
		require.NoError(t, err)
		assert.Equal(t, "READY", c.State)
	})
}

// TestJobRunLiveState_RespectsLifecycleAdvance proves job-run mutation guards
// (concurrency limits, BatchStopJobRun's stoppable-state check) decide
// against the live JobRunState, not a stale RUNNING snapshot from before the
// run actually finished.
func TestJobRunLiveState_RespectsLifecycleAdvance(t *testing.T) {
	t.Parallel()

	t.Run("start_run_respects_freed_concurrency_slot", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := glue.NewInMemoryBackend("000000000000", "us-east-1")
			t.Cleanup(b.Close)

			_, err := b.CreateJob(glue.Job{
				Name:              "j",
				Role:              "arn:aws:iam::000000000000:role/glue",
				Command:           glue.JobCommand{Name: "glueetl"},
				ExecutionProperty: glue.ExecutionProperty{MaxConcurrentRuns: 1},
			})
			require.NoError(t, err)

			_, err = b.StartJobRun("j", nil)
			require.NoError(t, err)

			// Past STARTING->RUNNING (150ms) and RUNNING->SUCCEEDED (300ms).
			time.Sleep(500 * time.Millisecond)

			_, err = b.StartJobRun("j", nil)
			assert.NoError(t, err)
		})
	})

	t.Run("batch_stop_rejects_completed_run", func(t *testing.T) {
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

			run, err := b.StartJobRun("j", nil)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			successes, errs := b.BatchStopJobRun("j", []string{run.ID})
			require.Empty(t, successes)
			require.Len(t, errs, 1)
			assert.Equal(t, "IllegalStateException", errs[0].ErrorDetail.ErrorCode)

			got, err := b.GetJobRun("j", run.ID)
			require.NoError(t, err)
			assert.Equal(t, "SUCCEEDED", got.JobRunState)
		})
	})
}

// TestBatchStopJobRun_AdvancesToStopped proves a stopped run reaches the real
// terminal STOPPED state (glue@v1.152.0 types/enums.go JobRunStateStopped) on
// its own, rather than sitting in STOPPING forever. AWS's StartJobRun/
// BatchStopJobRun contract only ever transitions a run into STOPPING; the
// backend itself is responsible for winding it down to STOPPED, the same way
// it winds STARTING down to RUNNING and RUNNING down to SUCCEEDED.
func TestBatchStopJobRun_AdvancesToStopped(t *testing.T) {
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

		run, err := b.StartJobRun("j", nil)
		require.NoError(t, err)

		successes, errs := b.BatchStopJobRun("j", []string{run.ID})
		require.Empty(t, errs)
		require.Len(t, successes, 1)

		got, err := b.GetJobRun("j", run.ID)
		require.NoError(t, err)
		require.Equal(t, "STOPPING", got.JobRunState)

		time.Sleep(500 * time.Millisecond)

		got, err = b.GetJobRun("j", run.ID)
		require.NoError(t, err)
		assert.Equal(t, "STOPPED", got.JobRunState)
		assert.NotZero(t, got.CompletedOn)
	})
}
