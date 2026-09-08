package redshift_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// assertStopsPromptly asserts that stop returns within the timeout. The
// reconciler's StopReconciler joins its goroutine via the WaitGroup, so a
// prompt return proves the goroutine exited; if it leaked, Wait would block
// and this reports it. Counting runtime.NumGoroutine() cannot be used here:
// these tests run with t.Parallel(), so goroutines started by unrelated tests
// inflate the process-wide count and produce spurious failures.
func assertStopsPromptly(t *testing.T, timeout time.Duration, stop func()) {
	t.Helper()

	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		stop()
	}()

	select {
	case <-stopped:
	case <-time.After(timeout):
		require.Fail(t, "reconciler goroutine did not exit after cancel/stop")
	}
}

// waitFor polls cond until it returns true or the deadline elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}

		time.Sleep(2 * time.Millisecond)
	}

	return cond()
}

// describeCount returns the number of clusters, driving the lazy read-time state
// machine (DescribeClusters advances due transitions).
func describeCount(t *testing.T, b *redshift.InMemoryBackend) int {
	t.Helper()

	clusters, _, err := b.DescribeClusters("", "", 0, nil, nil)
	require.NoError(t, err)

	return len(clusters)
}

func clusterStatus(t *testing.T, b *redshift.InMemoryBackend, id string) (string, bool) {
	t.Helper()

	clusters, _, err := b.DescribeClusters(id, "", 0, nil, nil)
	if err != nil {
		return "", false
	}

	require.Len(t, clusters, 1)

	return clusters[0].Status, true
}

// TestReconciler_LazyReadAdvancesState verifies that cluster states advance on
// read even when the background reconciler is NOT running (no goroutine).
func TestReconciler_LazyReadAdvancesState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus string
		delay      time.Duration
	}{
		{name: "instant available with zero delay", wantStatus: "available", delay: 0},
		{name: "creating then available", wantStatus: "available", delay: 20 * time.Millisecond},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
			redshift.SetClusterActivationDelay(b, tc.delay)

			_, err := b.CreateCluster("c1", "dc2.large", "dev", "admin", nil, "")
			require.NoError(t, err)

			if tc.delay > 0 {
				status, ok := clusterStatus(t, b, "c1")
				require.True(t, ok)
				assert.Equal(t, "creating", status)
			}

			ok := waitFor(t, time.Second, func() bool {
				s, found := clusterStatus(t, b, "c1")

				return found && s == tc.wantStatus
			})
			require.True(t, ok, "cluster did not reach %s", tc.wantStatus)
			assert.False(t, redshift.ReconcilerRunning(b), "no reconciler goroutine should be running")
		})
	}
}

// TestReconciler_BackgroundAdvancesState verifies the managed reconciler advances
// creating→available and deleting→removed without any explicit read.
func TestReconciler_BackgroundAdvancesState(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	redshift.SetClusterActivationDelay(b, 10*time.Millisecond)
	redshift.SetReconcileInterval(b, 3*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	b.StartReconciler(ctx)
	t.Cleanup(b.StopReconciler)

	_, err := b.CreateCluster("bg", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	// Reconciler flips to available.
	require.True(t, waitFor(t, time.Second, func() bool {
		s, ok := clusterStatus(t, b, "bg")

		return ok && s == "available"
	}), "reconciler did not advance to available")

	// Async delete: enters deleting, then the reconciler removes it.
	_, err = b.DeleteCluster("bg")
	require.NoError(t, err)

	require.True(t, waitFor(t, time.Second, func() bool {
		return redshift.ClusterCount(b) == 0
	}), "reconciler did not remove deleted cluster")
}

// TestReconciler_DeleteCancelsPendingCreate ensures deleting a still-creating
// cluster supersedes the pending create transition and removes it cleanly.
func TestReconciler_DeleteCancelsPendingCreate(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	redshift.SetClusterActivationDelay(b, 50*time.Millisecond)

	_, err := b.CreateCluster("pending", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)
	assert.Equal(t, 1, redshift.PendingClusterTransitions(b))

	// Delete while still creating schedules an async removal.
	deleted, err := b.DeleteCluster("pending")
	require.NoError(t, err)
	assert.Equal(t, "deleting", deleted.Status)

	require.True(t, waitFor(t, time.Second, func() bool {
		return describeCount(t, b) == 0
	}), "cluster not removed after async delete")
	assert.Equal(t, 0, redshift.PendingClusterTransitions(b),
		"no dangling transitions should remain")
}

// TestReconciler_AsyncDelete_ClearsLoggingStatuses verifies that the delayed
// (async) DeleteCluster path also clears loggingStatuses for the deleted
// cluster, matching the synchronous path. Otherwise a new cluster created
// with the same (user-chosen, reusable) ClusterIdentifier inherits the
// deleted cluster's stale logging status.
func TestReconciler_AsyncDelete_ClearsLoggingStatuses(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	redshift.SetClusterActivationDelay(b, 5*time.Millisecond)

	_, err := b.CreateCluster("reused-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	_, err = b.EnableLogging("reused-cluster", "my-bucket", "")
	require.NoError(t, err)

	_, err = b.DeleteCluster("reused-cluster")
	require.NoError(t, err)

	require.True(t, waitFor(t, time.Second, func() bool {
		return describeCount(t, b) == 0
	}), "cluster not removed after async delete")

	_, err = b.CreateCluster("reused-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	status, err := b.GetLoggingStatus("reused-cluster")
	require.NoError(t, err)
	assert.False(t, status.LoggingEnabled,
		"recreated cluster must not inherit the deleted cluster's logging status")
}

// TestReconciler_ResetClearsTransitions verifies Reset cancels pending
// transitions and clears state without leaking work.
func TestReconciler_ResetClearsTransitions(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	redshift.SetClusterActivationDelay(b, time.Hour) // never fires on its own

	for i := range 20 {
		_, err := b.CreateCluster(fmt.Sprintf("c%02d", i), "dc2.large", "dev", "admin", nil, "")
		require.NoError(t, err)
	}

	assert.Equal(t, 20, redshift.PendingClusterTransitions(b))

	b.Reset()

	assert.Equal(t, 0, redshift.ClusterCount(b))
	assert.Equal(t, 0, redshift.PendingClusterTransitions(b))
}

// TestReconciler_NoGoroutineLeak creates and deletes many clusters under a
// running reconciler, then asserts a clean, leak-free shutdown: after Stop the
// reconciler goroutine is joined on stop.
func TestReconciler_NoGoroutineLeak(t *testing.T) {
	t.Parallel()

	const numClusters = 40

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	redshift.SetClusterActivationDelay(b, 5*time.Millisecond)
	redshift.SetReconcileInterval(b, 2*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	b.StartReconciler(ctx)
	require.True(t, redshift.ReconcilerRunning(b))

	for i := range numClusters {
		_, err := b.CreateCluster(fmt.Sprintf("leak%03d", i), "dc2.large", "dev", "admin", nil, "")
		require.NoError(t, err)
	}

	// Wait for all to become available.
	require.True(t, waitFor(t, 3*time.Second, func() bool {
		clusters, _, err := b.DescribeClusters("", "", 0, nil, nil)
		if err != nil {
			return false
		}

		if len(clusters) != numClusters {
			return false
		}

		for i := range clusters {
			if clusters[i].Status != "available" {
				return false
			}
		}

		return true
	}), "not all clusters became available")

	// Delete all; reconciler removes them asynchronously.
	for i := range numClusters {
		_, err := b.DeleteCluster(fmt.Sprintf("leak%03d", i))
		require.NoError(t, err)
	}

	require.True(t, waitFor(t, 3*time.Second, func() bool {
		return redshift.ClusterCount(b) == 0
	}), "clusters not fully removed")

	b.Reset()
	assertStopsPromptly(t, 2*time.Second, b.StopReconciler)
	cancel()

	assert.False(t, redshift.ReconcilerRunning(b))
}

// TestReconciler_StartStopIdempotent verifies double start/stop is safe.
func TestReconciler_StartStopIdempotent(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	redshift.SetReconcileInterval(b, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	b.StartReconciler(ctx)
	b.StartReconciler(ctx) // second start is a no-op
	assert.True(t, redshift.ReconcilerRunning(b))

	b.StopReconciler()
	b.StopReconciler() // second stop is a no-op
	assert.False(t, redshift.ReconcilerRunning(b))
}

// TestReconciler_ContextCancelStops verifies cancelling the context stops the
// reconciler goroutine (framework-driven shutdown path).
func TestReconciler_ContextCancelStops(t *testing.T) {
	t.Parallel()

	b := redshift.NewInMemoryBackend("000000000000", "us-east-1")
	redshift.SetReconcileInterval(b, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	b.StartReconciler(ctx)

	cancel()
	assertStopsPromptly(t, 2*time.Second, b.StopReconciler)
}

// TestClusterLifecycle_CreatingToAvailable verifies the creating→available state machine.
func TestClusterLifecycle_CreatingToAvailable(t *testing.T) {
	t.Parallel()

	b := newRedshiftBackend()
	redshift.SetClusterActivationDelay(b, 50*time.Millisecond)

	_, err := b.CreateCluster("lifecycle-cluster", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	// Immediately after create, status should be "creating".
	clusters, _, err := b.DescribeClusters("lifecycle-cluster", "", 0, nil, nil)
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, "creating", clusters[0].Status,
		"cluster should be in creating state immediately after CreateCluster")

	// After the activation delay, status should be "available".
	time.Sleep(200 * time.Millisecond)

	clusters2, _, err := b.DescribeClusters("lifecycle-cluster", "", 0, nil, nil)
	require.NoError(t, err)
	require.Len(t, clusters2, 1)
	assert.Equal(t, "available", clusters2[0].Status, "cluster should be available after activation delay")
}
