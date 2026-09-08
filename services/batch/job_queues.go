package batch

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const maxJobQueueNameLength = 128

// lookupJQByNameOrARN returns a job queue by name or ARN within region.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) lookupJQByNameOrARN(region, nameOrARN string) (*JobQueue, bool) {
	if jq, ok := b.jobQueues.Get(regionKey(region, nameOrARN)); ok {
		return jq, true
	}

	for _, jq := range b.jobQueuesByRegion.Get(region) {
		if jq.JobQueueArn == nameOrARN {
			return jq, true
		}
	}

	return nil, false
}

// CreateJobQueue creates a new job queue.
func (b *InMemoryBackend) CreateJobQueue(
	ctx context.Context,
	name string,
	priority int32,
	state string,
	ceOrder []ComputeEnvironmentOrder,
	tags map[string]string,
	schedulingPolicyArn string,
	jobStateTimeLimitActions []JobStateTimeLimitAction,
	jobQueueType string,
	serviceEnvironmentOrder []ServiceEnvironmentOrder,
) (*JobQueue, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateJobQueue")
	defer b.mu.Unlock()

	if len(name) == 0 || len(name) > maxJobQueueNameLength {
		return nil, fmt.Errorf(
			"%w: jobQueueName must be between 1 and %d characters",
			ErrValidation, maxJobQueueNameLength,
		)
	}

	if b.jobQueues.Has(regionKey(region, name)) {
		return nil, fmt.Errorf("%w: job queue %s already exists", ErrAlreadyExists, name)
	}

	if len(ceOrder) > 0 && len(serviceEnvironmentOrder) > 0 {
		return nil, fmt.Errorf(
			"%w: a job queue can't have both computeEnvironmentOrder and serviceEnvironmentOrder",
			ErrValidation,
		)
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	jqARN := arn.Build("batch", region, b.accountID, "job-queue/"+name)

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	orderCopy := make([]ComputeEnvironmentOrder, len(ceOrder))
	copy(orderCopy, ceOrder)

	seOrderCopy := make([]ServiceEnvironmentOrder, len(serviceEnvironmentOrder))
	copy(seOrderCopy, serviceEnvironmentOrder)

	var actionsCopy []JobStateTimeLimitAction
	if len(jobStateTimeLimitActions) > 0 {
		actionsCopy = make([]JobStateTimeLimitAction, len(jobStateTimeLimitActions))
		copy(actionsCopy, jobStateTimeLimitActions)
	}

	jq := &JobQueue{
		region:                   region,
		JobQueueName:             name,
		JobQueueArn:              jqARN,
		State:                    state,
		Status:                   statusValid,
		Priority:                 priority,
		ComputeEnvironmentOrder:  orderCopy,
		ServiceEnvironmentOrder:  seOrderCopy,
		JobQueueType:             jobQueueType,
		Tags:                     tagsCopy,
		SchedulingPolicyArn:      schedulingPolicyArn,
		JobStateTimeLimitActions: actionsCopy,
	}
	b.jobQueues.Put(jq)
	b.jqsByARN[jqARN] = name
	// Register this queue as a reference for each compute environment it orders.
	for _, ceOrder := range orderCopy {
		ceName := ceOrder.ComputeEnvironment
		if b.ceToQueues[ceName] == nil {
			b.ceToQueues[ceName] = make(map[string]struct{})
		}
		b.ceToQueues[ceName][name] = struct{}{}
	}
	cp := *jq

	return &cp, nil
}

// cloneJobQueueWithTags returns a tag-cloned copy of jq.
func cloneJobQueueWithTags(jq *JobQueue) *JobQueue {
	cp := *jq
	cp.Tags = tagsCloneOrEmpty(jq.Tags)
	if cp.ComputeEnvironmentOrder == nil {
		cp.ComputeEnvironmentOrder = []ComputeEnvironmentOrder{}
	}

	return &cp
}

// DescribeJobQueues returns job queues, optionally filtered by names/ARNs.
// When names are provided, all matching queues are returned without pagination.
// When names is empty, results are paginated using maxResults and nextToken.
func (b *InMemoryBackend) DescribeJobQueues(
	ctx context.Context,
	names []string,
	maxResults int32,
	nextToken string,
) ([]*JobQueue, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeJobQueues")
	defer b.mu.RUnlock()

	return describeResourcesPaginated(
		names, maxResults, nextToken,
		func(nameOrARN string) (*JobQueue, bool) { return b.lookupJQByNameOrARN(region, nameOrARN) },
		func() []string {
			return sortedNames(b.jobQueuesByRegion.Get(region), func(jq *JobQueue) string { return jq.JobQueueName })
		},
		func(key string) (*JobQueue, bool) { return b.jobQueues.Get(regionKey(region, key)) },
		cloneJobQueueWithTags,
	)
}

// UpdateJobQueue updates a job queue's state, priority, CE order, scheduling
// policy, and/or time-limit actions.
func (b *InMemoryBackend) UpdateJobQueue(
	ctx context.Context,
	nameOrARN string,
	priority *int32,
	state, schedulingPolicyArn string,
	ceOrder []ComputeEnvironmentOrder,
	jobStateTimeLimitActions []JobStateTimeLimitAction,
	serviceEnvironmentOrder []ServiceEnvironmentOrder,
) (*JobQueue, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateJobQueue")
	defer b.mu.Unlock()

	jq, ok := b.lookupJQByNameOrARN(region, nameOrARN)
	if !ok {
		return nil, fmt.Errorf("%w: job queue %s not found", ErrNotFound, nameOrARN)
	}

	if state != "" && state != stateEnabled && state != stateDisabled {
		return nil, fmt.Errorf("%w: state must be %s or %s", ErrValidation, stateEnabled, stateDisabled)
	}

	if state != "" {
		jq.State = state
	}

	if priority != nil {
		jq.Priority = *priority
	}

	// batch@v1.68.4 api_op_UpdateJobQueue.go: "Once a job queue is created,
	// the fair-share scheduling policy can be replaced but not removed" --
	// so only a non-empty value ever overwrites the existing one.
	if schedulingPolicyArn != "" {
		jq.SchedulingPolicyArn = schedulingPolicyArn
	}

	if ceOrder != nil {
		// Remove old CE references from the reverse index.
		for _, old := range jq.ComputeEnvironmentOrder {
			if refs := b.ceToQueues[old.ComputeEnvironment]; refs != nil {
				delete(refs, jq.JobQueueName)
			}
		}
		orderCopy := make([]ComputeEnvironmentOrder, len(ceOrder))
		copy(orderCopy, ceOrder)
		jq.ComputeEnvironmentOrder = orderCopy
		// Add new CE references to the reverse index.
		for _, ceRef := range orderCopy {
			if b.ceToQueues[ceRef.ComputeEnvironment] == nil {
				b.ceToQueues[ceRef.ComputeEnvironment] = make(map[string]struct{})
			}
			b.ceToQueues[ceRef.ComputeEnvironment][jq.JobQueueName] = struct{}{}
		}
	}

	if serviceEnvironmentOrder != nil {
		seOrderCopy := make([]ServiceEnvironmentOrder, len(serviceEnvironmentOrder))
		copy(seOrderCopy, serviceEnvironmentOrder)
		jq.ServiceEnvironmentOrder = seOrderCopy
	}

	if jobStateTimeLimitActions != nil {
		actionsCopy := make([]JobStateTimeLimitAction, len(jobStateTimeLimitActions))
		copy(actionsCopy, jobStateTimeLimitActions)
		jq.JobStateTimeLimitActions = actionsCopy
	}

	cp := *jq

	return &cp, nil
}

// DeleteJobQueue removes a job queue. Jobs still in the queue are terminated
// (transitioned to FAILED), not deleted -- api_op_DeleteJobQueue.go: "All
// jobs in the queue are eventually terminated when you delete a job queue."
// Job history remains describable by ID afterward, same as TerminateJob;
// the janitor's normal CompletedJobTTL sweep evicts it later.
// The queue must be in DISABLED state before deletion.
func (b *InMemoryBackend) DeleteJobQueue(ctx context.Context, nameOrARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteJobQueue")
	defer b.mu.Unlock()

	jq, ok := b.lookupJQByNameOrARN(region, nameOrARN)
	if !ok {
		return fmt.Errorf("%w: job queue %s not found", ErrNotFound, nameOrARN)
	}

	if jq.State != "DISABLED" {
		return fmt.Errorf("%w: job queue %s must be DISABLED before it can be deleted", ErrValidation, nameOrARN)
	}

	queueName := jq.JobQueueName

	// jobsByQueueIdx is keyed by the job's JobQueue field, which stores the
	// queue's ARN (see the JobQueue field comment on jobs.go's SubmitJob).
	now := time.Now().UnixMilli()
	for _, j := range b.jobsByQueueIdx.Get(regionKey(region, jq.JobQueueArn)) {
		if isTerminalJobStatus(j.Status) {
			continue
		}

		j.Status = jobStatusFailed
		j.StatusReason = "job queue deleted"
		j.StoppedAt = &now
		j.IsTerminated = true
	}

	delete(b.jqsByARN, jq.JobQueueArn)
	// Remove this queue from the CE→queues reverse index.
	for _, ceOrder := range jq.ComputeEnvironmentOrder {
		if refs := b.ceToQueues[ceOrder.ComputeEnvironment]; refs != nil {
			delete(refs, queueName)
		}
	}
	b.jobQueues.Delete(regionKey(region, queueName))

	return nil
}

// GetJobQueueSnapshot returns a snapshot of the front of a job queue.
func (b *InMemoryBackend) GetJobQueueSnapshot(ctx context.Context, jobQueue string) (*JobQueueSnapshot, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetJobQueueSnapshot")
	defer b.mu.RUnlock()

	jq, ok := b.lookupJQByNameOrARN(region, jobQueue)
	if !ok {
		return nil, fmt.Errorf("%w: job queue %s not found", ErrNotFound, jobQueue)
	}

	group := b.jobsByQueueIdx.Get(regionKey(region, jq.JobQueueArn))
	runnableJobs := make([]*Job, 0, len(group))

	for _, j := range group {
		if j.Status == jobStatusRunnable {
			runnableJobs = append(runnableJobs, j)
		}
	}

	sort.Slice(runnableJobs, func(i, j int) bool { return runnableJobs[i].CreatedAt < runnableJobs[j].CreatedAt })

	const maxFrontOfQueue = 100
	if len(runnableJobs) > maxFrontOfQueue {
		runnableJobs = runnableJobs[:maxFrontOfQueue]
	}

	foqJobs := make([]FrontOfQueueJob, 0, len(runnableJobs))
	now := time.Now().UnixMilli()

	for _, j := range runnableJobs {
		foqJobs = append(foqJobs, FrontOfQueueJob{
			JobArn:                 j.JobARN,
			EarliestTimeAtPosition: j.CreatedAt,
		})
	}

	return &JobQueueSnapshot{
		FrontOfQueue: &FrontOfQueue{
			Jobs:          foqJobs,
			LastUpdatedAt: now,
		},
	}, nil
}
