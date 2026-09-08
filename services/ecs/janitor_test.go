package ecs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecs"
)

func setupJanitorBackend(t *testing.T) *ecs.InMemoryBackend {
	t.Helper()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	_, err := backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "test-cluster"})
	require.NoError(t, err)

	_, err = backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "test-family",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	return backend
}

func TestJanitor_SweepsStoppedTasksOlderThanTTL(t *testing.T) {
	t.Parallel()

	backend := setupJanitorBackend(t)

	// Run a task then stop it.
	tasks, err := backend.RunTask(ecs.RunTaskInput{
		Cluster:        "test-cluster",
		TaskDefinition: "test-family",
		Count:          1,
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	taskArn := tasks[0].TaskArn
	_, err = backend.StopTask("test-cluster", taskArn, "testing")
	require.NoError(t, err)

	// Create a janitor with a very short TTL so the stopped task is immediately stale.
	janitor := ecs.NewJanitor(backend, time.Second)
	janitor.SetTaskTTL(1 * time.Millisecond)

	// Allow time for the task to expire.
	time.Sleep(5 * time.Millisecond)

	janitor.SweepOnce(context.Background())

	// The stopped task should have been evicted.
	listed, err := backend.ListTasks("test-cluster")
	require.NoError(t, err)
	assert.Empty(t, listed)
}

func TestJanitor_SweptTaskLosesResourceTags(t *testing.T) {
	t.Parallel()

	backend := setupJanitorBackend(t)

	tasks, err := backend.RunTask(ecs.RunTaskInput{
		Cluster:        "test-cluster",
		TaskDefinition: "test-family",
		Count:          1,
		Tags:           []ecs.Tag{{Key: "env", Value: "test"}},
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	taskArn := tasks[0].TaskArn

	tags, err := backend.ListTagsForResource(taskArn)
	require.NoError(t, err)
	require.NotEmpty(t, tags, "tags should be recorded immediately after RunTask")

	_, err = backend.StopTask("test-cluster", taskArn, "testing")
	require.NoError(t, err)

	janitor := ecs.NewJanitor(backend, time.Second)
	janitor.SetTaskTTL(1 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	janitor.SweepOnce(context.Background())

	// The task is gone; its resourceTags side-map entry must go with it, or a
	// stale ARN keeps answering ListTagsForResource forever.
	tags, err = backend.ListTagsForResource(taskArn)
	require.NoError(t, err)
	assert.Empty(t, tags, "resourceTags leaked a ghost row for a swept task")
}

func TestJanitor_DoesNotSweepRunningTasks(t *testing.T) {
	t.Parallel()

	backend := setupJanitorBackend(t)

	tasks, err := backend.RunTask(ecs.RunTaskInput{
		Cluster:        "test-cluster",
		TaskDefinition: "test-family",
		Count:          1,
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	janitor := ecs.NewJanitor(backend, time.Second)
	janitor.SetTaskTTL(1 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	janitor.SweepOnce(context.Background())

	listed, err := backend.ListTasks("test-cluster")
	require.NoError(t, err)
	assert.Len(t, listed, 1)
}

func TestJanitor_DoesNotSweepRecentlyStoppedTasks(t *testing.T) {
	t.Parallel()

	backend := setupJanitorBackend(t)

	tasks, err := backend.RunTask(ecs.RunTaskInput{
		Cluster:        "test-cluster",
		TaskDefinition: "test-family",
		Count:          1,
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	_, err = backend.StopTask("test-cluster", tasks[0].TaskArn, "testing")
	require.NoError(t, err)

	// Use a long TTL so the recently-stopped task is not yet stale.
	janitor := ecs.NewJanitor(backend, time.Second)
	janitor.SetTaskTTL(time.Hour)

	janitor.SweepOnce(context.Background())

	// ListTasks defaults to RUNNING (ListTasksInput.DesiredStatus's documented
	// default), so a stopped-but-not-yet-swept task must be looked up
	// explicitly by its STOPPED status to confirm it's still in the store.
	listed, err := backend.ListTasksFiltered(ecs.ListTasksInput{Cluster: "test-cluster", DesiredStatus: "STOPPED"})
	require.NoError(t, err)
	assert.Len(t, listed, 1)
}

func TestJanitor_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	backend := setupJanitorBackend(t)

	janitor := ecs.NewJanitor(backend, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	go func() {
		janitor.Run(ctx)
		close(done)
	}()

	// Let it tick once.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Janitor exited as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("janitor did not exit after context cancellation")
	}
}
