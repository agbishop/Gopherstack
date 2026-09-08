package codebuild_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codebuild"
)

func newTestBackend(t *testing.T) *codebuild.InMemoryBackend {
	t.Helper()

	return codebuild.NewInMemoryBackend("123456789012", "us-east-1")
}

// TestJanitor_SweepCompletedBuilds verifies that the janitor removes builds in
// terminal states whose EndTime is past the configured TTL.
func TestJanitor_SweepCompletedBuilds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      string
		endOffset   time.Duration // negative = in the past
		ttl         time.Duration
		wantEvicted bool
	}{
		{
			name:        "evict_succeeded_past_ttl",
			status:      "SUCCEEDED",
			endOffset:   -25 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: true,
		},
		{
			name:        "evict_failed_past_ttl",
			status:      "FAILED",
			endOffset:   -25 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: true,
		},
		{
			name:        "evict_stopped_past_ttl",
			status:      "STOPPED",
			endOffset:   -25 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: true,
		},
		{
			name:        "evict_fault_past_ttl",
			status:      "FAULT",
			endOffset:   -25 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: true,
		},
		{
			name:        "keep_succeeded_within_ttl",
			status:      "SUCCEEDED",
			endOffset:   -1 * time.Hour,
			ttl:         24 * time.Hour,
			wantEvicted: false,
		},
		{
			name:        "keep_in_progress",
			status:      "IN_PROGRESS",
			endOffset:   0,
			ttl:         24 * time.Hour,
			wantEvicted: false,
		},
		{
			name:        "keep_no_endtime",
			status:      "SUCCEEDED",
			endOffset:   0,
			ttl:         24 * time.Hour,
			wantEvicted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newTestBackend(t)

			src := codebuild.ProjectSource{Type: "NO_SOURCE"}
			arts := codebuild.ProjectArtifacts{Type: "NO_ARTIFACTS"}
			env := codebuild.ProjectEnvironment{
				Type:        "LINUX_CONTAINER",
				Image:       "img",
				ComputeType: "BUILD_GENERAL1_SMALL",
			}
			_, err := backend.CreateProject(codebuild.ProjectConfig{
				Name:        "proj",
				Source:      &src,
				Artifacts:   &arts,
				Environment: &env,
			})
			require.NoError(t, err)

			build, err := backend.StartBuild("proj", codebuild.StartBuildConfig{})
			require.NoError(t, err)

			if tt.endOffset != 0 {
				backend.SetBuildEndTime(build.ID, tt.status, time.Now().Add(tt.endOffset))
			} else if tt.status != "IN_PROGRESS" {
				// SUCCEEDED with no end time: set status only, leave EndTime=0.
				backend.SetBuildEndTime(build.ID, tt.status, time.Time{})
			}

			janitor := codebuild.NewJanitor(backend, time.Hour, tt.ttl)
			janitor.SweepOnce(t.Context())

			builds, err := backend.ListBuildsForProject("proj")
			require.NoError(t, err)

			if tt.wantEvicted {
				assert.NotContains(t, builds, build.ID, "build should be evicted")
				assert.Equal(t, 0, backend.BuildCount())
			} else {
				assert.Contains(t, builds, build.ID, "build should be preserved")
			}
		})
	}
}

// TestDeleteProject_DoesNotCleanupBuilds verifies that deleting a project
// leaves its builds intact.
//
// This replaces a prior test (TestDeleteProject_CleanupBuilds) that asserted
// the opposite -- that DeleteProject cascade-deleted a project's builds. That
// assertion encoded a real parity bug: aws-sdk-go-v2/service/codebuild@v1.72.4's
// api_op_DeleteProject.go doc comment states plainly: "Deletes a build
// project. When you delete a project, its builds are not deleted." The
// backend was deleting them anyway; PARITY.md had (incorrectly) recorded the
// cascade as an intentional cleanup rather than a bug.
func TestDeleteProject_DoesNotCleanupBuilds(t *testing.T) {
	t.Parallel()

	backend := newTestBackend(t)

	src2 := codebuild.ProjectSource{Type: "NO_SOURCE"}
	arts2 := codebuild.ProjectArtifacts{Type: "NO_ARTIFACTS"}
	env2 := codebuild.ProjectEnvironment{Type: "LINUX_CONTAINER", Image: "img", ComputeType: "BUILD_GENERAL1_SMALL"}
	_, err := backend.CreateProject(codebuild.ProjectConfig{
		Name:        "proj",
		Source:      &src2,
		Artifacts:   &arts2,
		Environment: &env2,
	})
	require.NoError(t, err)

	_, err = backend.StartBuild("proj", codebuild.StartBuildConfig{})
	require.NoError(t, err)

	_, err = backend.StartBuild("proj", codebuild.StartBuildConfig{})
	require.NoError(t, err)

	assert.Equal(t, 2, backend.BuildCount(), "should have 2 builds before delete")
	assert.Equal(t, 2, backend.BuildsByProjectSize("proj"), "project index should have 2 before delete")

	err = backend.DeleteProject("proj")
	require.NoError(t, err)

	assert.Equal(t, 2, backend.BuildCount(), "builds must survive project deletion, matching real AWS")
	assert.Equal(t, 2, backend.BuildARNIndexSize(), "ARN index must still resolve the surviving builds")
	assert.Equal(t, 2, backend.BuildsByProjectSize("proj"), "project index must still list the surviving builds")
}

// TestJanitor_SweepCleansARNIndex verifies that sweeping builds also removes
// their entries from the buildARNIndex so ARN-based lookups on evicted builds
// report the build as not found instead of resolving a deleted resource.
func TestJanitor_SweepCleansARNIndex(t *testing.T) {
	t.Parallel()

	backend := newTestBackend(t)

	src3 := codebuild.ProjectSource{Type: "NO_SOURCE"}
	arts3 := codebuild.ProjectArtifacts{Type: "NO_ARTIFACTS"}
	env3 := codebuild.ProjectEnvironment{Type: "LINUX_CONTAINER", Image: "img", ComputeType: "BUILD_GENERAL1_SMALL"}
	_, err := backend.CreateProject(codebuild.ProjectConfig{
		Name:        "proj",
		Source:      &src3,
		Artifacts:   &arts3,
		Environment: &env3,
	})
	require.NoError(t, err)

	build, err := backend.StartBuild("proj", codebuild.StartBuildConfig{})
	require.NoError(t, err)

	// Mark build as terminal and past the TTL.
	backend.SetBuildEndTime(build.ID, "SUCCEEDED", time.Now().Add(-25*time.Hour))

	assert.Equal(t, 1, backend.BuildARNIndexSize(), "ARN index should have 1 entry before sweep")
	assert.Equal(t, 1, backend.BuildsByProjectSize("proj"), "project index should have 1 entry before sweep")

	janitor := codebuild.NewJanitor(backend, time.Hour, 24*time.Hour)
	janitor.SweepOnce(t.Context())

	assert.Equal(t, 0, backend.BuildCount(), "build should be evicted")
	assert.Equal(t, 0, backend.BuildARNIndexSize(), "ARN index should be empty after sweep")
	assert.Equal(t, 0, backend.BuildsByProjectSize("proj"), "project index should be empty after sweep")

	// An ARN-based lookup for the evicted build must report it as not found.
	found, notFound := backend.BatchGetBuilds([]string{build.Arn})
	assert.Empty(t, found, "evicted build must not resolve by ARN")
	assert.Equal(t, []string{build.Arn}, notFound)
}

// TestJanitor_AdvanceInProgressBuilds verifies that the janitor transitions
// IN_PROGRESS builds and build batches to a terminal SUCCEEDED state, so that
// clients polling BatchGetBuilds/BatchGetBuildBatches observe real progress
// instead of spinning on IN_PROGRESS forever.
func TestJanitor_AdvanceInProgressBuilds(t *testing.T) {
	t.Parallel()

	backend := newTestBackend(t)

	src := codebuild.ProjectSource{Type: "NO_SOURCE"}
	arts := codebuild.ProjectArtifacts{Type: "NO_ARTIFACTS"}
	env := codebuild.ProjectEnvironment{
		Type:        "LINUX_CONTAINER",
		Image:       "img",
		ComputeType: "BUILD_GENERAL1_SMALL",
	}
	_, err := backend.CreateProject(codebuild.ProjectConfig{
		Name:        "advance-proj",
		Source:      &src,
		Artifacts:   &arts,
		Environment: &env,
	})
	require.NoError(t, err)

	build, err := backend.StartBuild("advance-proj", codebuild.StartBuildConfig{})
	require.NoError(t, err)
	require.Equal(t, "IN_PROGRESS", build.BuildStatus)

	batch, err := backend.StartBuildBatch("advance-proj")
	require.NoError(t, err)
	require.Equal(t, "IN_PROGRESS", batch.BuildBatchStatus)

	janitor := codebuild.NewJanitor(backend, time.Hour, 24*time.Hour)
	janitor.SweepOnce(t.Context())

	gotBuilds, notFound := backend.BatchGetBuilds([]string{build.ID})
	require.Empty(t, notFound)
	require.Len(t, gotBuilds, 1)
	assert.Equal(t, "SUCCEEDED", gotBuilds[0].BuildStatus, "IN_PROGRESS build must advance to SUCCEEDED")
	assert.True(t, gotBuilds[0].BuildComplete, "advanced build must be marked complete")
	assert.Equal(t, "COMPLETED", gotBuilds[0].CurrentPhase)
	assert.Greater(t, gotBuilds[0].EndTime, float64(0), "advanced build must have an endTime")

	gotBatches, batchNotFound := backend.BatchGetBuildBatches([]string{batch.ID})
	require.Empty(t, batchNotFound)
	require.Len(t, gotBatches, 1)
	assert.Equal(t, "SUCCEEDED", gotBatches[0].BuildBatchStatus, "IN_PROGRESS build batch must advance to SUCCEEDED")
	assert.Greater(t, gotBatches[0].EndTime, float64(0), "advanced build batch must have an endTime")
}

// TestJanitor_AdvanceInProgressBuilds_LeavesTerminalBuildsAlone verifies that the
// advancement pass does not touch builds/batches already in a terminal state
// (e.g. explicitly stopped by the caller).
func TestJanitor_AdvanceInProgressBuilds_LeavesTerminalBuildsAlone(t *testing.T) {
	t.Parallel()

	backend := newTestBackend(t)

	src := codebuild.ProjectSource{Type: "NO_SOURCE"}
	arts := codebuild.ProjectArtifacts{Type: "NO_ARTIFACTS"}
	env := codebuild.ProjectEnvironment{
		Type:        "LINUX_CONTAINER",
		Image:       "img",
		ComputeType: "BUILD_GENERAL1_SMALL",
	}
	_, err := backend.CreateProject(codebuild.ProjectConfig{
		Name:        "stopped-proj",
		Source:      &src,
		Artifacts:   &arts,
		Environment: &env,
	})
	require.NoError(t, err)

	build, err := backend.StartBuild("stopped-proj", codebuild.StartBuildConfig{})
	require.NoError(t, err)

	stopped, err := backend.StopBuild(build.ID)
	require.NoError(t, err)
	require.Equal(t, "STOPPED", stopped.BuildStatus)

	janitor := codebuild.NewJanitor(backend, time.Hour, 24*time.Hour)
	janitor.SweepOnce(t.Context())

	gotBuilds, notFound := backend.BatchGetBuilds([]string{build.ID})
	require.Empty(t, notFound)
	require.Len(t, gotBuilds, 1)
	assert.Equal(t, "STOPPED", gotBuilds[0].BuildStatus, "already-terminal build must not be re-advanced to SUCCEEDED")
}

// TestJanitor_RunContext verifies that the janitor stops when context is cancelled.
func TestCodeBuildJanitor_RunContext(t *testing.T) {
	t.Parallel()

	backend := newTestBackend(t)
	janitor := codebuild.NewJanitor(backend, 10*time.Millisecond, time.Hour)

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})

	go func() {
		janitor.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "janitor did not stop after context cancellation")
	}
}

// TestCodeBuildJanitor_TaskTimeout_WithJanitor verifies that WithJanitor propagates
// the variadic taskTimeout parameter to the janitor's TaskTimeout field.
func TestCodeBuildJanitor_TaskTimeout_WithJanitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		taskTimeout time.Duration
		want        time.Duration
	}{
		{
			name:        "zero_timeout",
			taskTimeout: 0,
			want:        0,
		},
		{
			name:        "30s_timeout",
			taskTimeout: 30 * time.Second,
			want:        30 * time.Second,
		},
		{
			name:        "1min_timeout",
			taskTimeout: time.Minute,
			want:        time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := codebuild.NewHandler(codebuild.NewInMemoryBackend("000000000000", "us-east-1"))
			h.WithJanitor(time.Minute, 24*time.Hour, tt.taskTimeout)

			assert.Equal(t, tt.want, h.GetJanitorTaskTimeout())
		})
	}
}

// TestCodeBuildJanitor_DefaultInterval verifies that a zero interval in WithJanitor
// results in the default interval being used.
func TestCodeBuildJanitor_DefaultInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{
			name:     "zero_uses_default",
			interval: 0,
			want:     codebuild.DefaultJanitorInterval,
		},
		{
			name:     "custom_interval_propagated",
			interval: 5 * time.Minute,
			want:     5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := codebuild.NewHandler(codebuild.NewInMemoryBackend("123456789012", "us-east-1"))
			h.WithJanitor(tt.interval, 0)

			assert.Equal(t, tt.want, h.GetJanitorInterval())
		})
	}
}
