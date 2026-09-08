// White-box: exercises instanceRefreshTransitionDelay directly, mirroring
// scheduled_action_cron_test.go / scheduled_action_scheduler_test.go's
// rationale for living in-package.
package autoscaling //nolint:testpackage // see comment above

import (
	"testing"
	"testing/synctest"
	"time"
)

func newRefreshTestGroup(t *testing.T, b *InMemoryBackend, name string, desired int32) {
	t.Helper()

	_, err := b.CreateAutoScalingGroup(CreateAutoScalingGroupInput{
		AutoScalingGroupName: name,
		MinSize:              0,
		MaxSize:              5,
		DesiredCapacity:      desired,
	})
	if err != nil {
		t.Fatalf("CreateAutoScalingGroup: %v", err)
	}
}

func soleRefresh(t *testing.T, b *InMemoryBackend, group string) InstanceRefresh {
	t.Helper()

	refreshes, err := b.DescribeInstanceRefreshes(group, nil)
	if err != nil {
		t.Fatalf("DescribeInstanceRefreshes: %v", err)
	}

	if len(refreshes) != 1 {
		t.Fatalf("DescribeInstanceRefreshes: got %d refreshes, want 1", len(refreshes))
	}

	return refreshes[0]
}

// TestInstanceRefreshTransition_Successful verifies a started refresh stays
// InProgress until instanceRefreshTransitionDelay elapses, then reaches
// Successful with EndTime/PercentageComplete/InstancesToUpdate populated.
func TestInstanceRefreshTransition_Successful(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := NewInMemoryBackend()
		defer b.Close()

		newRefreshTestGroup(t, b, "refresh-asg", 3)

		if refreshes, err := b.DescribeInstanceRefreshes("refresh-asg", nil); err != nil || len(refreshes) != 0 {
			t.Fatalf("DescribeInstanceRefreshes before start: refreshes=%v err=%v", refreshes, err)
		}

		started, err := b.StartInstanceRefresh("refresh-asg")
		if err != nil {
			t.Fatalf("StartInstanceRefresh: %v", err)
		}

		if started.Status != statusInProgress {
			t.Fatalf("Status after start = %q, want %q", started.Status, statusInProgress)
		}

		if started.InstancesToUpdate != 3 {
			t.Fatalf("InstancesToUpdate after start = %d, want 3", started.InstancesToUpdate)
		}

		// Still in flight before the transition delay elapses.
		if got := soleRefresh(t, b, "refresh-asg"); got.Status != statusInProgress {
			t.Fatalf("Status before delay = %q, want %q (must not report a terminal status early)",
				got.Status, statusInProgress)
		}

		time.Sleep(instanceRefreshTransitionDelay + time.Millisecond)
		synctest.Wait()

		got := soleRefresh(t, b, "refresh-asg")
		if got.Status != statusSuccessful {
			t.Fatalf("Status after delay = %q, want %q", got.Status, statusSuccessful)
		}

		if got.PercentageComplete != completedProgress {
			t.Fatalf("PercentageComplete = %d, want %d", got.PercentageComplete, completedProgress)
		}

		if got.InstancesToUpdate != 0 {
			t.Fatalf("InstancesToUpdate = %d, want 0", got.InstancesToUpdate)
		}

		if got.EndTime.IsZero() {
			t.Fatal("EndTime is zero, want set")
		}
	})
}

// TestInstanceRefreshTransition_Cancelled verifies CancelInstanceRefresh sets
// Cancelling immediately, supersedes the in-flight completion timer (the
// refresh must never reach Successful once cancelled), and reaches Cancelled
// after instanceRefreshTransitionDelay.
func TestInstanceRefreshTransition_Cancelled(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := NewInMemoryBackend()
		defer b.Close()

		newRefreshTestGroup(t, b, "cancel-asg", 2)

		if _, err := b.StartInstanceRefresh("cancel-asg"); err != nil {
			t.Fatalf("StartInstanceRefresh: %v", err)
		}

		if _, err := b.CancelInstanceRefresh("cancel-asg"); err != nil {
			t.Fatalf("CancelInstanceRefresh: %v", err)
		}

		if got := soleRefresh(t, b, "cancel-asg"); got.Status != statusCancelling {
			t.Fatalf("Status after cancel = %q, want %q", got.Status, statusCancelling)
		}

		time.Sleep(instanceRefreshTransitionDelay + time.Millisecond)
		synctest.Wait()

		got := soleRefresh(t, b, "cancel-asg")
		if got.Status != statusCancelled {
			t.Fatalf("Status after delay = %q, want %q (superseded Successful timer must not have fired)",
				got.Status, statusCancelled)
		}

		if got.EndTime.IsZero() {
			t.Fatal("EndTime is zero, want set")
		}
	})
}

// TestInstanceRefreshTransition_RollbackSuccessful verifies
// RollbackInstanceRefresh sets RollbackInProgress immediately and reaches
// RollbackSuccessful, with progress reset to zero, after the transition delay.
func TestInstanceRefreshTransition_RollbackSuccessful(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := NewInMemoryBackend()
		defer b.Close()

		newRefreshTestGroup(t, b, "rollback-asg", 2)

		if _, err := b.StartInstanceRefresh("rollback-asg"); err != nil {
			t.Fatalf("StartInstanceRefresh: %v", err)
		}

		if _, err := b.RollbackInstanceRefresh("rollback-asg"); err != nil {
			t.Fatalf("RollbackInstanceRefresh: %v", err)
		}

		if got := soleRefresh(t, b, "rollback-asg"); got.Status != statusRollbackInProgress {
			t.Fatalf("Status after rollback = %q, want %q", got.Status, statusRollbackInProgress)
		}

		time.Sleep(instanceRefreshTransitionDelay + time.Millisecond)
		synctest.Wait()

		got := soleRefresh(t, b, "rollback-asg")
		if got.Status != statusRollbackSuccessful {
			t.Fatalf("Status after delay = %q, want %q", got.Status, statusRollbackSuccessful)
		}

		if got.PercentageComplete != 0 {
			t.Fatalf("PercentageComplete = %d, want 0", got.PercentageComplete)
		}

		if got.InstancesToUpdate != 0 {
			t.Fatalf("InstancesToUpdate = %d, want 0", got.InstancesToUpdate)
		}

		if got.EndTime.IsZero() {
			t.Fatal("EndTime is zero, want set")
		}
	})
}

// TestInstanceRefreshTransition_DeleteGroupSafeAfterDelete verifies deleting
// a group with an in-flight refresh (which stops its pending transition
// timer via cleanupRefreshTimers, matching cleanupHookTimers for lifecycle
// hooks) leaves no trace of the refresh once the original transition delay
// elapses -- the fired-or-cancelled timer must not resurrect a refresh for a
// group that no longer exists.
func TestInstanceRefreshTransition_DeleteGroupSafeAfterDelete(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := NewInMemoryBackend()
		defer b.Close()

		newRefreshTestGroup(t, b, "delete-asg", 1)

		if _, err := b.StartInstanceRefresh("delete-asg"); err != nil {
			t.Fatalf("StartInstanceRefresh: %v", err)
		}

		if err := b.DeleteAutoScalingGroup("delete-asg", true); err != nil {
			t.Fatalf("DeleteAutoScalingGroup: %v", err)
		}

		time.Sleep(instanceRefreshTransitionDelay + time.Millisecond)
		synctest.Wait()

		if _, err := b.DescribeInstanceRefreshes("delete-asg", nil); err == nil {
			t.Fatal("DescribeInstanceRefreshes: want error for deleted group, got nil")
		}
	})
}

// TestInstanceRefreshTransition_RestoreRearmsTimer verifies a refresh
// restored mid InProgress keeps advancing to Successful after Restore,
// exercising rearmPendingRefreshes.
func TestInstanceRefreshTransition_RestoreRearmsTimer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		b := NewInMemoryBackend()
		defer b.Close()

		newRefreshTestGroup(t, b, "restore-asg", 1)

		if _, err := b.StartInstanceRefresh("restore-asg"); err != nil {
			t.Fatalf("StartInstanceRefresh: %v", err)
		}

		data := b.Snapshot(ctx)
		if data == nil {
			t.Fatal("Snapshot returned nil")
		}

		nb := NewInMemoryBackend()
		defer nb.Close()

		if err := nb.Restore(ctx, data); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		if got := soleRefresh(t, nb, "restore-asg"); got.Status != statusInProgress {
			t.Fatalf("Status right after restore = %q, want %q", got.Status, statusInProgress)
		}

		time.Sleep(instanceRefreshTransitionDelay + time.Millisecond)
		synctest.Wait()

		got := soleRefresh(t, nb, "restore-asg")
		if got.Status != statusSuccessful {
			t.Fatalf("Status after delay post-restore = %q, want %q (rearmPendingRefreshes must re-arm the timer)",
				got.Status, statusSuccessful)
		}
	})
}
