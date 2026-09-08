package ecs

import (
	"testing"
	"time"
)

// lcCluster is the cluster name shared by the lifecycle tests.
const lcCluster = "lc"

// runOneTask registers a task definition, creates a cluster, and runs a single
// task, returning its ARN.
func runOneTask(t *testing.T, b *InMemoryBackend) string {
	t.Helper()

	tdArn := registerSimpleTaskDef(t, b, "lc-app", "nginx")

	if _, err := b.CreateCluster(CreateClusterInput{ClusterName: lcCluster}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	tasks, err := b.RunTask(RunTaskInput{Cluster: lcCluster, TaskDefinition: tdArn, Count: 1})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("RunTask returned %d tasks, want 1", len(tasks))
	}

	return tasks[0].TaskArn
}

func taskStatus(t *testing.T, b *InMemoryBackend, arn string) string {
	t.Helper()

	tasks, _, err := b.DescribeTasks(lcCluster, []string{arn})
	if err != nil {
		t.Fatalf("DescribeTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("DescribeTasks returned %d tasks, want 1", len(tasks))
	}

	return tasks[0].LastStatus
}

func TestStopTaskLifecycle_ObservableIntermediateStates(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	arn := runOneTask(t, b)

	if got := taskStatus(t, b, arn); got != statusRunning {
		t.Fatalf("initial status = %q, want RUNNING", got)
	}

	b.SetStopDelay(time.Millisecond)

	stopped, err := b.StopTask("lc", arn, "user requested")
	if err != nil {
		t.Fatalf("StopTask: %v", err)
	}
	if stopped.LastStatus != statusDeactivating {
		t.Fatalf("StopTask returned status %q, want DEACTIVATING", stopped.LastStatus)
	}

	// Each step advances exactly one observable phase. Drive with a clock that
	// moves well past the per-phase delay each call so each phase becomes due.
	base := time.Now().Add(time.Hour)

	wantSequence := []string{statusStopping, statusDeprovisioning, statusStopped}
	for i, want := range wantSequence {
		b.stepTaskLifecycle(base.Add(time.Duration(i+1) * time.Second))

		if got := taskStatus(t, b, arn); got != want {
			t.Fatalf("after step, status = %q, want %q", got, want)
		}
	}

	// Terminal task must have StoppedAt set and be untracked.
	tasks, _, err := b.DescribeTasks("lc", []string{arn})
	if err != nil {
		t.Fatalf("DescribeTasks: %v", err)
	}
	if tasks[0].StoppedAt == nil {
		t.Error("StoppedAt not set on terminal task")
	}
	if tasks[0].DesiredStatus != statusStopped {
		t.Errorf("DesiredStatus = %q, want STOPPED", tasks[0].DesiredStatus)
	}

	b.mu.RLock("test")
	_, tracked := b.lifecycle[arn]
	b.mu.RUnlock()

	if tracked {
		t.Error("task still tracked in lifecycle map after reaching STOPPED")
	}
}

func TestStopTaskLifecycle_FastPathByDefault(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	arn := runOneTask(t, b)

	// Default stopDelay is zero: StopTask must transition straight to STOPPED.
	stopped, err := b.StopTask("lc", arn, "user requested")
	if err != nil {
		t.Fatalf("StopTask: %v", err)
	}
	if stopped.LastStatus != statusStopped {
		t.Errorf("StopTask (fast path) status = %q, want STOPPED", stopped.LastStatus)
	}

	b.mu.RLock("test")
	_, tracked := b.lifecycle[arn]
	b.mu.RUnlock()

	if tracked {
		t.Error("fast-path stop must not create a lifecycle entry")
	}
}

func TestStopTaskLifecycle_CounterDecrementsOnce(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	arn := runOneTask(t, b)

	clusterRunning := func() int {
		b.mu.RLock("test")
		defer b.mu.RUnlock()

		c, _ := b.clusters.Get("lc")

		return c.RunningTasksCount
	}

	if got := clusterRunning(); got != 1 {
		t.Fatalf("RunningTasksCount before stop = %d, want 1", got)
	}

	b.SetStopDelay(time.Millisecond)

	if _, err := b.StopTask("lc", arn, "stop"); err != nil {
		t.Fatalf("StopTask: %v", err)
	}

	// The running counter must drop as soon as the task leaves RUNNING, and must
	// not be decremented again when it later reaches STOPPED.
	if got := clusterRunning(); got != 0 {
		t.Fatalf("RunningTasksCount after DEACTIVATING = %d, want 0", got)
	}

	base := time.Now().Add(time.Hour)
	for i := range 3 {
		b.stepTaskLifecycle(base.Add(time.Duration(i+1) * time.Second))
	}

	if got := taskStatus(t, b, arn); got != statusStopped {
		t.Fatalf("status after steps = %q, want STOPPED", got)
	}
	if got := clusterRunning(); got != 0 {
		t.Errorf("RunningTasksCount after STOPPED = %d, want 0 (no double decrement)", got)
	}
}

func TestStartTaskLifecycle_ObservableIntermediateStates(t *testing.T) {
	t.Parallel()

	// The observable start pipeline applies to the no-runner path (with a real
	// runner the task already dwells in PENDING during the container start).
	b := NewInMemoryBackend("123456789012", "us-east-1", nil)
	b.SetStartDelay(time.Millisecond)

	tdArn := registerSimpleTaskDef(t, b, "lc-app", "nginx")
	if _, err := b.CreateCluster(CreateClusterInput{ClusterName: "lc"}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	tasks, err := b.RunTask(RunTaskInput{Cluster: "lc", TaskDefinition: tdArn, Count: 1})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}

	arn := tasks[0].TaskArn
	if tasks[0].LastStatus != statusProvisioning {
		t.Fatalf("RunTask (delayed start) status = %q, want PROVISIONING", tasks[0].LastStatus)
	}

	base := time.Now().Add(time.Hour)

	wantSequence := []string{statusPending, statusRunning}
	for i, want := range wantSequence {
		b.stepTaskLifecycle(base.Add(time.Duration(i+1) * time.Second))

		if got := taskStatus(t, b, arn); got != want {
			t.Fatalf("after step, status = %q, want %q", got, want)
		}
	}

	b.mu.RLock("test")
	c, _ := b.clusters.Get("lc")
	running := c.RunningTasksCount
	pending := c.PendingTasksCount
	_, tracked := b.lifecycle[arn]
	b.mu.RUnlock()

	if running != 1 {
		t.Errorf("RunningTasksCount = %d, want 1", running)
	}
	if pending != 0 {
		t.Errorf("PendingTasksCount = %d, want 0", pending)
	}
	if tracked {
		t.Error("task still tracked after reaching RUNNING")
	}
}

func TestStopTask_DuringStartDelay_DoesNotResurrect(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1", nil)
	b.SetStartDelay(time.Millisecond)

	tdArn := registerSimpleTaskDef(t, b, "lc-app", "nginx")
	if _, err := b.CreateCluster(CreateClusterInput{ClusterName: "lc"}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	tasks, err := b.RunTask(RunTaskInput{Cluster: "lc", TaskDefinition: tdArn, Count: 1})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}

	arn := tasks[0].TaskArn
	if tasks[0].LastStatus != statusProvisioning {
		t.Fatalf("initial status = %q, want PROVISIONING", tasks[0].LastStatus)
	}

	// StopTask fires while the task is still tracked in the start pipeline
	// (registered by RunTask above) and stopDelay is unset, so this takes the
	// fast path straight to STOPPED.
	stopped, err := b.StopTask("lc", arn, "user requested")
	if err != nil {
		t.Fatalf("StopTask: %v", err)
	}
	if stopped.LastStatus != statusStopped {
		t.Fatalf("StopTask status = %q, want STOPPED", stopped.LastStatus)
	}

	// Drive the lifecycle stepper as the reconciler would on later ticks. A
	// leftover start-pipeline entry must not resurrect the stopped task.
	future := time.Now().Add(time.Hour)
	b.stepTaskLifecycle(future)
	b.stepTaskLifecycle(future.Add(time.Second))

	if got := taskStatus(t, b, arn); got != statusStopped {
		t.Fatalf("status after lifecycle steps = %q, want STOPPED (task was resurrected)", got)
	}

	b.mu.RLock("test")
	_, tracked := b.lifecycle[arn]
	b.mu.RUnlock()

	if tracked {
		t.Error("stopped task still tracked in lifecycle map")
	}
}

func TestStepTaskLifecycle_RespectsDelay(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	arn := runOneTask(t, b)

	b.SetStopDelay(time.Hour)

	if _, err := b.StopTask("lc", arn, "stop"); err != nil {
		t.Fatalf("StopTask: %v", err)
	}

	// Stepping with a clock before the delay elapses must not advance the task.
	b.stepTaskLifecycle(time.Now())

	if got := taskStatus(t, b, arn); got != statusDeactivating {
		t.Errorf("status after premature step = %q, want DEACTIVATING (delay not elapsed)", got)
	}
}
