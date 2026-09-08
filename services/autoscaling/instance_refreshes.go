package autoscaling

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// CancelInstanceRefresh cancels an active instance refresh for the group.
// It returns the ID of the cancelled refresh.
func (b *InMemoryBackend) CancelInstanceRefresh(groupName string) (string, error) {
	b.mu.Lock("CancelInstanceRefresh")
	defer b.mu.Unlock()

	if !b.groups.Has(groupName) {
		return "", fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	for _, r := range b.instanceRefreshes[groupName] {
		if r.Status == statusInProgress || r.Status == statusPending {
			r.Status = statusCancelling
			b.armRefreshTransition(r.InstanceRefreshID, groupName, statusCancelled)

			return r.InstanceRefreshID, nil
		}
	}

	return "", fmt.Errorf("%w: no active instance refresh for group %q",
		ErrActiveInstanceRefreshNotFound, groupName)
}

// AddInstanceRefresh stores an instance refresh for the given group (used for testing CancelInstanceRefresh).
func (b *InMemoryBackend) AddInstanceRefresh(refresh InstanceRefresh) error {
	b.mu.Lock("AddInstanceRefresh")
	defer b.mu.Unlock()

	if !b.groups.Has(refresh.AutoScalingGroupName) {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, refresh.AutoScalingGroupName)
	}

	cp := refresh
	b.instanceRefreshes[refresh.AutoScalingGroupName] = append(
		b.instanceRefreshes[refresh.AutoScalingGroupName],
		&cp,
	)

	return nil
}

// StartInstanceRefresh creates a new instance refresh for the group.
func (b *InMemoryBackend) StartInstanceRefresh(groupName string) (*InstanceRefresh, error) {
	return b.StartInstanceRefreshWithInput(StartInstanceRefreshInput{AutoScalingGroupName: groupName})
}

// StartInstanceRefreshWithInput creates a new instance refresh for the group with full input.
func (b *InMemoryBackend) StartInstanceRefreshWithInput(input StartInstanceRefreshInput) (*InstanceRefresh, error) {
	b.mu.Lock("StartInstanceRefresh")
	defer b.mu.Unlock()

	if !b.groups.Has(input.AutoScalingGroupName) {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, input.AutoScalingGroupName)
	}

	for _, r := range b.instanceRefreshes[input.AutoScalingGroupName] {
		if r.Status == statusInProgress || r.Status == statusPending {
			return nil, fmt.Errorf(
				"%w: an instance refresh is already in progress for group %q",
				ErrInstanceRefreshInProgress, input.AutoScalingGroupName,
			)
		}
	}

	strategy := input.Strategy
	if strategy == "" {
		strategy = "Rolling"
	}

	prefs := input.Preferences
	if prefs.MinHealthyPercentage == 0 {
		prefs.MinHealthyPercentage = 90
	}

	g, _ := b.groups.Get(input.AutoScalingGroupName)

	refresh := &InstanceRefresh{
		InstanceRefreshID:    uuid.NewString(),
		AutoScalingGroupName: input.AutoScalingGroupName,
		Status:               statusInProgress,
		StartTime:            time.Now(),
		Strategy:             strategy,
		Preferences:          prefs,
		InstancesToUpdate:    int32(len(g.Instances)), //nolint:gosec // bounded by maxDesiredCapacity
	}

	b.instanceRefreshes[input.AutoScalingGroupName] = append(b.instanceRefreshes[input.AutoScalingGroupName], refresh)
	b.armRefreshTransition(refresh.InstanceRefreshID, input.AutoScalingGroupName, statusSuccessful)

	cp := *refresh

	return &cp, nil
}

// RollbackInstanceRefresh rolls back an in-progress instance refresh for the group.
func (b *InMemoryBackend) RollbackInstanceRefresh(groupName string) (string, error) {
	b.mu.Lock("RollbackInstanceRefresh")
	defer b.mu.Unlock()

	if !b.groups.Has(groupName) {
		return "", fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	for _, r := range b.instanceRefreshes[groupName] {
		if r.Status == statusInProgress || r.Status == statusPending {
			r.Status = statusRollbackInProgress
			b.armRefreshTransition(r.InstanceRefreshID, groupName, statusRollbackSuccessful)

			return r.InstanceRefreshID, nil
		}
	}

	return "", fmt.Errorf("%w: no active instance refresh for group %q",
		ErrActiveInstanceRefreshNotFound, groupName)
}

// DescribeInstanceRefreshes returns instance refreshes for the group, optionally filtered by ID.
func (b *InMemoryBackend) DescribeInstanceRefreshes(groupName string, refreshIDs []string) ([]InstanceRefresh, error) {
	b.mu.RLock("DescribeInstanceRefreshes")
	defer b.mu.RUnlock()

	if groupName != "" {
		if !b.groups.Has(groupName) {
			return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
		}
	}

	idFilter := make(map[string]bool, len(refreshIDs))
	for _, id := range refreshIDs {
		idFilter[id] = true
	}

	var result []InstanceRefresh

	groups := b.instanceRefreshes
	if groupName != "" {
		groups = map[string][]*InstanceRefresh{groupName: b.instanceRefreshes[groupName]}
	}

	for _, refreshes := range groups {
		for _, r := range refreshes {
			if len(idFilter) > 0 && !idFilter[r.InstanceRefreshID] {
				continue
			}
			result = append(result, *r)
		}
	}

	// groups is b.instanceRefreshes (a map) when groupName is empty, so account-wide iteration
	// order is randomized run to run; a stable total order is required for pagination to not
	// drop or duplicate records across a page boundary. InstanceRefreshID is a uuid.NewString()
	// value (see StartInstanceRefresh below) -- globally unique, so no tiebreak is needed.
	sort.Slice(result, func(i, j int) bool { return result[i].InstanceRefreshID < result[j].InstanceRefreshID })

	return result, nil
}

// armRefreshTransition schedules refreshID's transition to nextStatus after
// instanceRefreshTransitionDelay, replacing any transition already pending
// for that refresh (cancel/rollback superseding an in-progress completion).
// Must be called with b.mu held (write lock).
func (b *InMemoryBackend) armRefreshTransition(refreshID, groupName, nextStatus string) {
	if existing, ok := b.pendingRefreshActions.Get(refreshID); ok {
		existing.timer.Stop()
		b.pendingRefreshActions.Delete(refreshID)
	}

	action := &pendingRefreshAction{
		ID:         refreshID,
		GroupName:  groupName,
		NextStatus: nextStatus,
	}
	action.timer = time.AfterFunc(instanceRefreshTransitionDelay, func() {
		b.resolveRefreshTransition(refreshID)
	})
	b.pendingRefreshActions.Put(action)
}

// resolveRefreshTransition applies the scheduled status transition for
// refreshID, if it is still pending. A miss means the action was already
// superseded (cancel/rollback re-arming with a new timer) or the backend was
// reset/restored/closed -- Close and Restore stop and clear
// pendingRefreshActions before touching b.instanceRefreshes, so firing late
// into a torn-down or replaced backend is a safe no-op rather than a stale
// write. Called from the timer's own goroutine, hence it takes the lock
// itself, matching resolveLifecycleWait.
func (b *InMemoryBackend) resolveRefreshTransition(refreshID string) {
	b.mu.Lock("resolveRefreshTransition")
	defer b.mu.Unlock()

	action, ok := b.pendingRefreshActions.Get(refreshID)
	if !ok {
		return
	}

	b.pendingRefreshActions.Delete(refreshID)

	for _, r := range b.instanceRefreshes[action.GroupName] {
		if r.InstanceRefreshID != refreshID {
			continue
		}

		r.Status = action.NextStatus
		r.EndTime = time.Now()

		// This backend doesn't tick per-instance progress during InProgress,
		// so PercentageComplete/InstancesToUpdate only move on the terminal
		// transitions where AWS's own semantics pin their end value:
		// Successful always reaches 100%/0 remaining, and a completed
		// rollback always unwinds back to 0%/0 remaining (types.go:1126-1134,
		// "gradually goes back down to zero during a rollback"). Cancelled
		// keeps whatever value it had, matching a cancel that stops after
		// any in-flight replacement completes.
		switch action.NextStatus {
		case statusSuccessful:
			r.PercentageComplete = completedProgress
			r.InstancesToUpdate = 0
		case statusRollbackSuccessful:
			r.PercentageComplete = 0
			r.InstancesToUpdate = 0
		}

		break
	}
}

// cleanupRefreshTimers cancels and removes any pending transition timer for
// groupName's instance refreshes. Must be called with b.mu held (write lock)
// and before groupName's entry in b.instanceRefreshes is deleted.
func (b *InMemoryBackend) cleanupRefreshTimers(groupName string) {
	for _, r := range b.instanceRefreshes[groupName] {
		if existing, ok := b.pendingRefreshActions.Get(r.InstanceRefreshID); ok {
			existing.timer.Stop()
			b.pendingRefreshActions.Delete(r.InstanceRefreshID)
		}
	}
}

// rearmPendingRefreshes re-arms transition timers for any instance refresh
// restored mid InProgress, Cancelling, or RollbackInProgress; in-flight
// timers are never persisted (see pendingRefreshActions in persistence.go).
// Must be called with b.mu held (write lock); intended to run once, right
// after Restore repopulates b.instanceRefreshes.
func (b *InMemoryBackend) rearmPendingRefreshes() {
	for _, refreshes := range b.instanceRefreshes {
		for _, r := range refreshes {
			var next string

			switch r.Status {
			case statusInProgress, statusPending:
				next = statusSuccessful
			case statusCancelling:
				next = statusCancelled
			case statusRollbackInProgress:
				next = statusRollbackSuccessful
			default:
				continue
			}

			b.armRefreshTransition(r.InstanceRefreshID, r.AutoScalingGroupName, next)
		}
	}
}
