package stepfunctions

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"time"
)

// SweepTaskTokens evicts task tokens that have exceeded the TTL without a worker response,
// signalling the blocked InvokeActivity/WaitForTaskToken goroutine so it is not leaked.
// Returns evicted count.
func (b *InMemoryBackend) SweepTaskTokens() int {
	ttl := b.settings.TaskTokenTTL
	if ttl == 0 {
		ttl = defaultTaskTokenTTL
	}

	cutoff := time.Now().Add(-ttl)

	// Phase 1: collect stale tokens under read lock.
	b.mu.RLock("SweepTaskTokens.scan")

	var staleTokens []string

	for token, entry := range b.tasksByToken {
		if !entry.createdAt.IsZero() && entry.createdAt.Before(cutoff) {
			staleTokens = append(staleTokens, token)
		}
	}

	b.mu.RUnlock()

	if len(staleTokens) == 0 {
		return 0
	}

	// Phase 2: delete under write lock, collecting entries for notification.
	b.mu.Lock("SweepTaskTokens.delete")

	var stale []*activityTaskEntry

	for _, token := range staleTokens {
		entry, ok := b.tasksByToken[token]
		if !ok {
			continue // deleted between phases
		}

		if entry.createdAt.IsZero() || !entry.createdAt.Before(cutoff) {
			continue // renewed between phases
		}

		stale = append(stale, entry)
		delete(b.tasksByToken, token)

		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}
	}

	b.mu.Unlock()

	for _, entry := range stale {
		select {
		case entry.resultCh <- activityTaskResult{errCode: "TaskTimedOut", cause: "task token TTL exceeded"}:
		default:
		}
	}

	return len(stale)
}

// CreateActivity creates a new activity resource in the caller's region.
func (b *InMemoryBackend) CreateActivity(ctx context.Context, name string) (*Activity, error) {
	if err := validateName(name, maxActivityNameLen); err != nil {
		return nil, err
	}

	region := getRegionFromContext(ctx, b.region)
	actARN := b.activityARN(region, name)

	b.mu.Lock("CreateActivity")
	defer b.mu.Unlock()

	actIdx := b.regionActivityIndex(region)
	if _, exists := actIdx[name]; exists {
		return nil, fmt.Errorf("%w: %s", ErrActivityAlreadyExists, name)
	}

	a := &Activity{
		Name:         name,
		ActivityArn:  actARN,
		CreationDate: float64(time.Now().Unix()),
	}
	b.activities.Put(a)
	actIdx[name] = actARN
	b.pendingTaskQueues[actARN] = make(chan *activityTaskEntry, maxPendingActivityTasks)

	cp := *a

	return &cp, nil
}

// SetActivityEncryptionConfiguration sets an activity's server-side
// encryption configuration. Mirrors SetStateMachineConfigurations'
// established pattern (state_machines.go) for optional post-create
// configuration supplied inline on the CreateActivity request.
func (b *InMemoryBackend) SetActivityEncryptionConfiguration(
	activityArn string,
	encryption *EncryptionConfiguration,
) error {
	b.mu.Lock("SetActivityEncryptionConfiguration")
	defer b.mu.Unlock()

	a, ok := b.activities.Get(activityArn)
	if !ok || a == nil {
		return fmt.Errorf("%w: %s", ErrActivityDoesNotExist, activityArn)
	}

	a.EncryptionConfiguration = encryption

	return nil
}

// DeleteActivity deletes an activity and closes its pending task queue.
// AWS: DeleteActivity's own error switch models only InvalidArn -- no
// ActivityDoesNotExist -- so it is idempotent on a missing activity.
func (b *InMemoryBackend) DeleteActivity(activityArn string) error {
	b.mu.Lock("DeleteActivity")
	defer b.mu.Unlock()

	a, exists := b.activities.Get(activityArn)
	if !exists {
		return nil
	}

	b.activities.Delete(activityArn)

	actRegion := regionFromARN(activityArn, b.region)
	delete(b.activityNameIndex[actRegion], a.Name)

	if queue, hasQueue := b.pendingTaskQueues[activityArn]; hasQueue {
		close(queue)
		delete(b.pendingTaskQueues, activityArn)
	}

	taskTokens := make([]string, 0, len(b.tasksByToken))
	for taskToken, entry := range b.tasksByToken {
		if entry.activityArn == activityArn {
			taskTokens = append(taskTokens, taskToken)
		}
	}
	for _, taskToken := range taskTokens {
		entry := b.tasksByToken[taskToken]
		delete(b.tasksByToken, taskToken)
		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}
		// Unblock any InvokeActivity goroutine waiting on this task's resultCh.
		select {
		case entry.resultCh <- activityTaskResult{errCode: "ActivityDoesNotExist", cause: "activity deleted"}:
		default:
		}
	}

	return nil
}

// DescribeActivity returns activity details.
func (b *InMemoryBackend) DescribeActivity(activityArn string) (*Activity, error) {
	b.mu.RLock("DescribeActivity")
	defer b.mu.RUnlock()

	a, ok := b.activities.Get(activityArn)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrActivityDoesNotExist, activityArn)
	}

	cp := *a

	return &cp, nil
}

// ListActivities returns activities in the caller's region with optional pagination.
func (b *InMemoryBackend) ListActivities(
	ctx context.Context,
	nextToken string,
	maxResults int,
) ([]Activity, string, error) {
	region := getRegionFromContext(ctx, b.region)

	b.mu.RLock("ListActivities")
	defer b.mu.RUnlock()

	all := make([]Activity, 0, b.activities.Len())
	for _, a := range b.activities.All() {
		if regionFromARN(a.ActivityArn, b.region) != region {
			continue
		}

		all = append(all, *a)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	acts, token := paginate(all, nextToken, maxResults)

	return acts, token, nil
}

// GetActivityTask long-polls for a pending task (up to 60 seconds).
// Returns an empty ActivityTask (TaskToken="") if no task is available — AWS-compatible behavior.
func (b *InMemoryBackend) GetActivityTask(
	ctx context.Context,
	activityArn, _ /* workerName */ string,
) (*ActivityTask, error) {
	b.mu.RLock("GetActivityTask")
	queue, ok := b.pendingTaskQueues[activityArn]
	b.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrActivityDoesNotExist, activityArn)
	}

	pollCtx, cancel := context.WithTimeout(ctx, activityPollTimeout)
	defer cancel()

	select {
	case entry, open := <-queue:
		if !open || entry == nil {
			// Channel was closed (activity destroyed or backend reset).
			return &ActivityTask{}, nil
		}

		return &ActivityTask{TaskToken: entry.taskToken, Input: entry.input}, nil
	case <-pollCtx.Done():
		return &ActivityTask{}, nil
	}
}

// SendTaskSuccess signals successful completion of an activity task with output.
func (b *InMemoryBackend) SendTaskSuccess(taskToken, output string) error {
	b.mu.Lock("SendTaskSuccess")
	entry, ok := b.tasksByToken[taskToken]

	if !ok {
		b.mu.Unlock()

		return fmt.Errorf("%w: %s", ErrTaskTokenNotFound, taskToken)
	}

	delete(b.tasksByToken, taskToken)
	b.mu.Unlock()

	select {
	case entry.resultCh <- activityTaskResult{output: output, succeeded: true}:
	default:
	}

	return nil
}

// SendTaskFailure signals failure of an activity task.
func (b *InMemoryBackend) SendTaskFailure(taskToken, errCode, cause string) error {
	b.mu.Lock("SendTaskFailure")
	entry, ok := b.tasksByToken[taskToken]

	if !ok {
		b.mu.Unlock()

		return fmt.Errorf("%w: %s", ErrTaskTokenNotFound, taskToken)
	}

	delete(b.tasksByToken, taskToken)
	b.mu.Unlock()

	select {
	case entry.resultCh <- activityTaskResult{errCode: errCode, cause: cause, succeeded: false}:
	default:
	}

	return nil
}

// SendTaskHeartbeat resets the heartbeat timer and renews createdAt (the
// TaskTokenTTL janitor backstop) for an activity task. createdAt is not the
// overall task timeout -- that's enforced by the ASL executor's own ctx
// deadline around InvokeActivity/WaitForTaskToken (asl/executor.go
// runTaskAttempt) -- so renewing it here only proves liveness to the leak
// sweep, it can't extend a Task state's TimeoutSeconds.
func (b *InMemoryBackend) SendTaskHeartbeat(taskToken string) error {
	b.mu.Lock("SendTaskHeartbeat")
	entry, ok := b.tasksByToken[taskToken]
	if ok {
		entry.createdAt = time.Now()
	}
	b.mu.Unlock()

	if !ok {
		return fmt.Errorf("%w: %s", ErrTaskTokenNotFound, taskToken)
	}

	if entry.heartbeatTimer != nil && entry.heartbeatDuration > 0 {
		if !entry.heartbeatTimer.Stop() {
			select {
			case <-entry.heartbeatTimer.C:
			default:
			}
		}

		entry.heartbeatTimer.Reset(entry.heartbeatDuration)
	}

	return nil
}

// WaitForTaskToken registers a callback token and blocks until terminal callback.
// It returns ErrTaskTokenAlreadyExists when token already exists, ErrHeartbeatTimeout
// when heartbeatSeconds elapses without heartbeat/success/failure, or ctx.Err() on cancellation.
func (b *InMemoryBackend) WaitForTaskToken(
	ctx context.Context,
	taskToken string,
	heartbeatSeconds int,
) (string, error) {
	entry := &activityTaskEntry{
		taskToken: taskToken,
		resultCh:  make(chan activityTaskResult, 1),
		createdAt: time.Now(),
	}

	if heartbeatSeconds > 0 {
		entry.heartbeatDuration = time.Duration(heartbeatSeconds) * time.Second
		entry.heartbeatStop = make(chan struct{}, 1)
		entry.heartbeatTimer = time.NewTimer(entry.heartbeatDuration)
	}

	b.mu.Lock("WaitForTaskToken")
	if _, exists := b.tasksByToken[taskToken]; exists {
		b.mu.Unlock()

		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}

		return "", fmt.Errorf("%w: %s", ErrTaskTokenAlreadyExists, taskToken)
	}

	b.tasksByToken[taskToken] = entry
	b.mu.Unlock()

	var heartbeatCh <-chan time.Time
	if entry.heartbeatTimer != nil {
		heartbeatCh = entry.heartbeatTimer.C
	}

	select {
	case result := <-entry.resultCh:
		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}

		if result.succeeded {
			return result.output, nil
		}

		return "", fmt.Errorf("%w: %s", ErrActivityTaskFailed, result.errCode)
	case <-heartbeatCh:
		b.mu.Lock("WaitForTaskToken.heartbeat.timeout")
		delete(b.tasksByToken, taskToken)
		b.mu.Unlock()

		return "", ErrHeartbeatTimeout
	case <-ctx.Done():
		b.mu.Lock("WaitForTaskToken.wait.cancel")
		delete(b.tasksByToken, taskToken)
		b.mu.Unlock()

		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}

		return "", ctx.Err()
	}
}

// InvokeActivity implements asl.ActivityInvoker.
// It enqueues a task for the activity and blocks until a worker calls
// SendTaskSuccess or SendTaskFailure, or the context is cancelled.
// If heartbeatSeconds > 0, the task fails with ErrHeartbeatTimeout if no
// SendTaskHeartbeat call arrives within the interval.
func (b *InMemoryBackend) InvokeActivity(
	ctx context.Context,
	activityArn, inputJSON string,
	heartbeatSeconds int,
) (string, error) {
	tokenBytes := make([]byte, activityTokenBytes)
	if _, err := cryptorand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate task token: %w", err)
	}

	taskToken := base64.URLEncoding.EncodeToString(tokenBytes)

	entry := &activityTaskEntry{
		activityArn: activityArn,
		taskToken:   taskToken,
		input:       inputJSON,
		resultCh:    make(chan activityTaskResult, 1),
		createdAt:   time.Now(),
	}

	if heartbeatSeconds > 0 {
		entry.heartbeatDuration = time.Duration(heartbeatSeconds) * time.Second
		entry.heartbeatStop = make(chan struct{}, 1)
		entry.heartbeatTimer = time.NewTimer(entry.heartbeatDuration)
	}

	b.mu.Lock("InvokeActivity")
	queue, ok := b.pendingTaskQueues[activityArn]

	if !ok {
		b.mu.Unlock()

		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}

		return "", fmt.Errorf("%w: %s", ErrActivityDoesNotExist, activityArn)
	}

	b.tasksByToken[taskToken] = entry
	b.mu.Unlock()

	// Enqueue the task, respecting context cancellation if the queue is full.
	select {
	case queue <- entry:
	case <-ctx.Done():
		b.mu.Lock("InvokeActivity.cancel")
		delete(b.tasksByToken, taskToken)
		b.mu.Unlock()

		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}

		return "", ctx.Err()
	}

	// Resolve heartbeat channel (nil if no timeout configured).
	var heartbeatCh <-chan time.Time
	if entry.heartbeatTimer != nil {
		heartbeatCh = entry.heartbeatTimer.C
	}

	// Wait for the worker to complete the task.
	select {
	case result := <-entry.resultCh:
		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}

		if result.succeeded {
			return result.output, nil
		}

		return "", fmt.Errorf("%w: %s", ErrActivityTaskFailed, result.errCode)
	case <-heartbeatCh:
		b.mu.Lock("InvokeActivity.heartbeat.timeout")
		delete(b.tasksByToken, taskToken)
		b.mu.Unlock()

		return "", ErrHeartbeatTimeout
	case <-ctx.Done():
		b.mu.Lock("InvokeActivity.wait.cancel")
		delete(b.tasksByToken, taskToken)
		b.mu.Unlock()

		if entry.heartbeatTimer != nil {
			entry.heartbeatTimer.Stop()
		}

		return "", ctx.Err()
	}
}
