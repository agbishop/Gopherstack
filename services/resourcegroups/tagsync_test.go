package resourcegroups_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/resourcegroups"
)

// TestTaskARN_Uniqueness verifies that rapid task creation produces unique ARNs.
func TestTaskARN_Uniqueness(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")

	// Create one group and start many tasks in rapid succession.
	_, err := b.CreateGroup(context.Background(), "rapid-group", "", nil, nil, nil)
	require.NoError(t, err)

	const taskCount = 20
	taskARNs := make(map[string]bool, taskCount)

	for range taskCount {
		task, tErr := b.StartTagSyncTask(
			context.Background(),
			"rapid-group",
			"arn:aws:iam::000000000000:role/r",
			"k", "v", nil,
		)
		require.NoError(t, tErr)
		assert.False(t, taskARNs[task.TaskArn], "duplicate TaskArn: %s", task.TaskArn)
		taskARNs[task.TaskArn] = true
	}
}

// TestTaskARN_ContainsGroupName verifies task ARN includes group name.
func TestTaskARN_ContainsGroupName(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateGroup(context.Background(), "named-group", "", nil, nil, nil)
	require.NoError(t, err)

	task, err := b.StartTagSyncTask(
		context.Background(), "named-group", "arn:aws:iam::000000000000:role/r", "", "", nil,
	)
	require.NoError(t, err)
	assert.Contains(t, task.TaskArn, "named-group")
}

// TestListTagSyncTasks_Pagination verifies NextToken pagination.
func TestListTagSyncTasks_Pagination(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")

	for i := range 5 {
		name := fmt.Sprintf("task-group-%d", i)
		_, err := b.CreateGroup(context.Background(), name, "", nil, nil, nil)
		require.NoError(t, err)
		_, err = b.StartTagSyncTask(context.Background(), name, "arn:aws:iam::000000000000:role/r", "k", "v", nil)
		require.NoError(t, err)
	}

	page1, tok1, err := b.ListTagSyncTasks(context.Background(), nil, "", 2)
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	require.NotEmpty(t, tok1)

	page2, tok2, err := b.ListTagSyncTasks(context.Background(), nil, tok1, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
	require.NotEmpty(t, tok2)

	page3, tok3, err := b.ListTagSyncTasks(context.Background(), nil, tok2, 2)
	require.NoError(t, err)
	assert.Len(t, page3, 1)
	assert.Empty(t, tok3)
}

// TestListTagSyncTasks_Sorted verifies tasks are returned sorted.
func TestListTagSyncTasks_Sorted(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")

	_, _ = b.CreateGroup(context.Background(), "g1", "", nil, nil, nil)
	_, _ = b.CreateGroup(context.Background(), "g2", "", nil, nil, nil)

	// Start multiple tasks for determinism check.
	_, err1 := b.StartTagSyncTask(context.Background(), "g1", "arn:aws:iam::000000000000:role/r", "k", "v", nil)
	_, err2 := b.StartTagSyncTask(context.Background(), "g2", "arn:aws:iam::000000000000:role/r", "k", "v", nil)
	require.NoError(t, err1)
	require.NoError(t, err2)

	tasks, _, err := b.ListTagSyncTasks(context.Background(), nil, "", 0)
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	assert.LessOrEqual(t, tasks[0].TaskArn, tasks[1].TaskArn)
}

// TestListTagSyncTasks_FilteredByGroupName verifies filter works.
func TestListTagSyncTasks_FilteredByGroupName(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")
	_, _ = b.CreateGroup(context.Background(), "g1", "", nil, nil, nil)
	_, _ = b.CreateGroup(context.Background(), "g2", "", nil, nil, nil)
	_, _ = b.StartTagSyncTask(context.Background(), "g1", "arn:aws:iam::000000000000:role/r", "", "", nil)
	_, _ = b.StartTagSyncTask(context.Background(), "g2", "arn:aws:iam::000000000000:role/r", "", "", nil)

	tasks, _, err := b.ListTagSyncTasks(context.Background(), []resourcegroups.ListTagSyncTasksFilter{
		{GroupName: "g1"},
	}, "", 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "g1", tasks[0].GroupName)
}

// TestListTagSyncTasks_FilterByGroupARN verifies filter by GroupArn.
func TestListTagSyncTasks_FilterByGroupARN(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")
	g1, err := b.CreateGroup(context.Background(), "filter-grp-1", "", nil, nil, nil)
	require.NoError(t, err)
	g2, err := b.CreateGroup(context.Background(), "filter-grp-2", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.StartTagSyncTask(context.Background(), "filter-grp-1", "arn:aws:iam::000000000000:role/r", "", "", nil)
	require.NoError(t, err)
	_, err = b.StartTagSyncTask(context.Background(), "filter-grp-2", "arn:aws:iam::000000000000:role/r", "", "", nil)
	require.NoError(t, err)

	// Filter by g1 ARN.
	tasks, _, err := b.ListTagSyncTasks(context.Background(), []resourcegroups.ListTagSyncTasksFilter{
		{GroupArn: g1.ARN},
	}, "", 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, g1.ARN, tasks[0].GroupArn)

	// Filter by g2 ARN.
	tasks, _, err = b.ListTagSyncTasks(context.Background(), []resourcegroups.ListTagSyncTasksFilter{
		{GroupArn: g2.ARN},
	}, "", 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, g2.ARN, tasks[0].GroupArn)
}

// TestTagSyncTask_FullLifecyclePaginated verifies start/list/cancel/get with pagination.
func TestTagSyncTask_FullLifecyclePaginated(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")

	for i := range 4 {
		name := fmt.Sprintf("task-grp-%d", i)
		_, err := b.CreateGroup(context.Background(), name, "", nil, nil, nil)
		require.NoError(t, err)
		_, err = b.StartTagSyncTask(context.Background(), name, "arn:aws:iam::000000000000:role/r", "k", "v", nil)
		require.NoError(t, err)
	}

	// Paginate: page 1 of 2.
	page1, tok1, err := b.ListTagSyncTasks(context.Background(), nil, "", 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, tok1)

	// Page 2, fetched before any cancellation.
	page2, tok2, err := b.ListTagSyncTasks(context.Background(), nil, tok1, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Empty(t, tok2)
	assert.Len(t, append(page1, page2...), 4)

	// Cancel one task from page 1: AWS documents CancelTagSyncTask as deleting
	// the task outright (TagSyncTaskStatus has no CANCELLED value), so it must
	// no longer be retrievable or listed afterward.
	cancelled := page1[0].TaskArn
	err = b.CancelTagSyncTask(context.Background(), cancelled)
	require.NoError(t, err)

	_, err = b.GetTagSyncTask(context.Background(), cancelled)
	require.ErrorIs(t, err, resourcegroups.ErrTagSyncTaskNotFound)

	remaining, _, err := b.ListTagSyncTasks(context.Background(), nil, "", 0)
	require.NoError(t, err)
	assert.Len(t, remaining, 3, "cancelled task must be removed from ListTagSyncTasks")

	for _, task := range remaining {
		assert.NotEqual(t, cancelled, task.TaskArn)
	}
}

// TestStartTagSyncTask_Reset_CounterRestarts verifies that Reset() zeroes
// taskIDCounter, so the next tag-sync task's ARN suffix restarts at 1
// (matching ec2's fix establishing that this codebase resets ID sequence
// counters on Reset -- nextPrivateIPIndex, nextElasticIPIndex), not a suffix
// that keeps climbing from the pre-Reset run.
func TestStartTagSyncTask_Reset_CounterRestarts(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateGroup(context.Background(), "counter-group", "", nil, nil, nil)
	require.NoError(t, err)

	task1, err := b.StartTagSyncTask(
		context.Background(), "counter-group", "arn:aws:iam::000000000000:role/r", "k", "v", nil,
	)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(task1.TaskArn, "-1"),
		"sanity: first task ARN should end in -1, got %s", task1.TaskArn)

	b.Reset()

	_, err = b.CreateGroup(context.Background(), "counter-group", "", nil, nil, nil)
	require.NoError(t, err)

	task2, err := b.StartTagSyncTask(
		context.Background(), "counter-group", "arn:aws:iam::000000000000:role/r", "k", "v", nil,
	)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(task2.TaskArn, "-1"),
		"taskIDCounter must restart at 1 after Reset, got task ARN %s", task2.TaskArn)
}
