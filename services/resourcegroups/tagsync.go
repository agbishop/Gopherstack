package resourcegroups

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// TagSyncTask status constants. AWS documents only these two values for
// TagSyncTaskStatus; there is no CANCELLED status because cancelling a task
// deletes it outright (see CancelTagSyncTask).
const (
	tagSyncTaskStatusActive = "ACTIVE"
)

// tagSyncTaskTTL is the maximum age of a non-active (e.g. errored) tag-sync
// task before it is evicted from memory during list operations.
const tagSyncTaskTTL = 24 * time.Hour

// StartTagSyncTask creates a new tag-sync task for an application group.
func (b *InMemoryBackend) StartTagSyncTask(
	ctx context.Context,
	nameOrARN, roleARN, tagKey, tagValue string,
	resourceQuery *ResourceQuery,
) (*TagSyncTask, error) {
	b.mu.Lock("StartTagSyncTask")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	g, ok := b.groups.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	b.taskIDCounter++
	taskARN := arn.Build(
		"resource-groups",
		region,
		b.accountID,
		fmt.Sprintf("tag-sync-task/%s-%s-%d", name, time.Now().Format("20060102150405"), b.taskIDCounter),
	)

	task := &TagSyncTask{
		TaskArn:       taskARN,
		GroupArn:      g.ARN,
		GroupName:     name,
		RoleArn:       roleARN,
		TagKey:        tagKey,
		TagValue:      tagValue,
		ResourceQuery: resourceQuery,
		Status:        tagSyncTaskStatusActive,
		CreatedAt:     time.Now().UTC(),
	}

	b.tagSyncTasks.Put(task)

	cp := *task

	return &cp, nil
}

// CancelTagSyncTask deletes a tag-sync task. AWS documents CancelTagSyncTask
// as taking "the TaskArn of the tag-sync task you want to delete", and
// TagSyncTaskStatus's only valid wire values are ACTIVE and ERROR -- there is
// no CANCELLED status. So, unlike an in-place status transition, a cancelled
// task is removed outright: subsequent GetTagSyncTask/ListTagSyncTasks calls
// no longer find it.
//
// gopherstack-m4k0 (errtargetaudit): the guard below emits NotFoundException
// on an unknown TaskArn, a code confirmed NOT in this op's modeled error set
// (deserializeOpErrorCancelTagSyncTask, resourcegroups@v1.36.4
// deserializers.go: BadRequestException/ForbiddenException/
// InternalServerErrorException/MethodNotAllowedException/
// TooManyRequestsException/UnauthorizedException only; the live
// docs.aws.amazon.com/ARG/latest/APIReference/API_CancelTagSyncTask.html
// Errors section agrees, and botocore's service-2.json documentation string
// carries no idempotency language either). Left unfixed: two candidate
// remedies were weighed and neither is confirmed. (1) Idempotent success --
// the only evidence offered was the doc's "HTTP 200 response with an empty
// HTTP body" sentence, but that sentence is generic Response-shape
// boilerplate, not idempotency evidence (same trap that got codepipeline's
// DeletePipeline/DeleteCustomActionType reverted under gopherstack-3djp: the
// identical sentence appears verbatim on this service's own GetGroup/
// DeleteGroup, which DO error on not-found). (2) BadRequestException, which
// this op does declare and which an unmatched TaskArn could plausibly map
// to -- but no doc or sibling behavior confirms AWS actually does this
// rather than something else entirely; it is inference, not a confirmed
// replacement. No declared code is an obvious substitute.
func (b *InMemoryBackend) CancelTagSyncTask(ctx context.Context, taskARN string) error {
	b.mu.Lock("CancelTagSyncTask")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	tableKey := regionKey(region, taskARN)

	if !b.tagSyncTasks.Has(tableKey) {
		return fmt.Errorf("%w: task %s not found", ErrTagSyncTaskNotFound, taskARN)
	}

	b.tagSyncTasks.Delete(tableKey)

	return nil
}

// GetTagSyncTask returns a copy of a tag-sync task by ARN.
func (b *InMemoryBackend) GetTagSyncTask(ctx context.Context, taskARN string) (*TagSyncTask, error) {
	b.mu.RLock("GetTagSyncTask")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	task, ok := b.tagSyncTasks.Get(regionKey(region, taskARN))
	if !ok {
		return nil, fmt.Errorf("%w: task %s not found", ErrTagSyncTaskNotFound, taskARN)
	}

	cp := *task

	return &cp, nil
}

// ListTagSyncTasks returns all tag-sync tasks, optionally filtered by group ARN or name,
// paginated. Inactive tasks older than tagSyncTaskTTL are evicted before results are assembled.
// Results are sorted by TaskArn for deterministic ordering.
// Returns tasks, a continuation token (empty when no more results), and any error.
func (b *InMemoryBackend) ListTagSyncTasks(
	ctx context.Context,
	filters []ListTagSyncTasksFilter,
	nextToken string,
	maxResults int,
) ([]TagSyncTask, string, error) {
	b.mu.Lock("ListTagSyncTasks")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	cutoff := time.Now().UTC().Add(-tagSyncTaskTTL)

	// Evict stale non-active tasks. slices.Clone the index result first:
	// Table.Delete mutates the index's backing slice in place, which would
	// otherwise corrupt this in-progress range.
	for _, task := range slices.Clone(b.tagSyncTasksByRegion.Get(region)) {
		if task.Status != tagSyncTaskStatusActive && task.CreatedAt.Before(cutoff) {
			b.tagSyncTasks.Delete(regionKey(region, task.TaskArn))
		}
	}

	tasks := b.tagSyncTasksByRegion.Get(region)
	out := make([]TagSyncTask, 0, len(tasks))

	for _, task := range tasks {
		if !taskMatchesFilters(task, filters) {
			continue
		}

		out = append(out, *task)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].TaskArn < out[j].TaskArn })

	page, token := paginate(out, func(t TagSyncTask) string { return t.TaskArn }, nextToken, maxResults)

	return page, token, nil
}

// taskMatchesFilters returns true when task satisfies all provided filter criteria.
// An empty filter list matches all tasks.
func taskMatchesFilters(task *TagSyncTask, filters []ListTagSyncTasksFilter) bool {
	if len(filters) == 0 {
		return true
	}

	for _, f := range filters {
		if (f.GroupArn == "" || f.GroupArn == task.GroupArn) &&
			(f.GroupName == "" || f.GroupName == task.GroupName) {
			return true
		}
	}

	return false
}
