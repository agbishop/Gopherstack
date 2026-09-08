package autoscaling

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CompleteLifecycleAction completes a lifecycle action for the given group and hook.
func (b *InMemoryBackend) CompleteLifecycleAction(input CompleteLifecycleActionInput) error {
	b.mu.Lock("CompleteLifecycleAction")
	defer b.mu.Unlock()

	if !b.groups.Has(input.AutoScalingGroupName) {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, input.AutoScalingGroupName)
	}

	if input.LifecycleHookName == "" {
		return fmt.Errorf("%w: LifecycleHookName is required", ErrInvalidParameter)
	}

	if input.LifecycleActionResult == "" {
		return fmt.Errorf("%w: LifecycleActionResult is required", ErrInvalidParameter)
	}

	// Validate LifecycleActionResult is CONTINUE or ABANDON (case-insensitive)
	upper := strings.ToUpper(input.LifecycleActionResult)
	if upper != lifecycleActionContinue && upper != lifecycleActionAbandon {
		return fmt.Errorf("%w: LifecycleActionResult must be CONTINUE or ABANDON, got %q",
			ErrInvalidParameter, input.LifecycleActionResult)
	}

	action := b.findPendingHookAction(
		input.LifecycleActionToken, input.AutoScalingGroupName, input.LifecycleHookName, input.InstanceID,
	)
	if action != nil {
		action.timer.Stop()
		b.pendingHookTokens.Delete(action.Token)
		b.applyLifecycleResult(action, upper)
	}

	return nil
}

// DeleteLifecycleHook removes a lifecycle hook from the specified Auto Scaling group.
func (b *InMemoryBackend) DeleteLifecycleHook(groupName, hookName string) error {
	b.mu.Lock("DeleteLifecycleHook")
	defer b.mu.Unlock()

	if !b.groups.Has(groupName) {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	key := scopedKey(groupName, hookName)
	if !b.lifecycleHooks.Has(key) {
		return fmt.Errorf("%w: lifecycle hook %q not found", ErrLifecycleHookNotFound, hookName)
	}

	b.resolveOutstandingHookActions(groupName, hookName)
	b.lifecycleHooks.Delete(key)

	// A launching ABANDON resolved above terminates-and-replaces the instance
	// (applyLifecycleResult), and the replacement is re-gated via
	// gateNewLaunchInstances/firstHookInChain -- which, since hookName was
	// still registered during the call above, can re-arm it right back onto
	// the hook now being deleted. Resolve once more now that hookName is
	// actually gone, so firstHookInChain skips it and the replacement isn't
	// stranded the same way.
	b.resolveOutstandingHookActions(groupName, hookName)

	return nil
}

// AddLifecycleHook stores a lifecycle hook for the given group.
// This is the backend helper used by PutLifecycleHook and by tests.
func (b *InMemoryBackend) AddLifecycleHook(hook LifecycleHook) error {
	b.mu.Lock("AddLifecycleHook")
	defer b.mu.Unlock()

	if !b.groups.Has(hook.AutoScalingGroupName) {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, hook.AutoScalingGroupName)
	}

	cp := hook
	b.putLifecycleHookLocked(&cp)

	return nil
}

// putLifecycleHookLocked upserts hook into b.lifecycleHooks, assigning it the
// next chain Sequence on first registration and preserving the existing one
// across updates. Must be called with b.mu held (write lock).
func (b *InMemoryBackend) putLifecycleHookLocked(hook *LifecycleHook) {
	key := scopedKey(hook.AutoScalingGroupName, hook.LifecycleHookName)
	if existing, ok := b.lifecycleHooks.Get(key); ok {
		hook.Sequence = existing.Sequence
	} else {
		b.nextHookSeq++
		hook.Sequence = b.nextHookSeq
	}

	b.lifecycleHooks.Put(hook)
}

// recomputeNextHookSeqLocked restores nextHookSeq after a snapshot Restore, so
// hooks registered post-restore chain after every restored hook instead of
// colliding with (or racing behind) Sequence numbers already in use. Must be
// called with b.mu held (write lock).
func (b *InMemoryBackend) recomputeNextHookSeqLocked() {
	var maxSeq int64

	for _, h := range b.lifecycleHooks.All() {
		if h.Sequence > maxSeq {
			maxSeq = h.Sequence
		}
	}

	b.nextHookSeq = maxSeq
}

// defaultHeartbeatTimeout is the default HeartbeatTimeout for lifecycle hooks (1 hour), matching AWS.
const defaultHeartbeatTimeout = int32(3600)

// PutLifecycleHook creates or updates a lifecycle hook on an Auto Scaling group.
func (b *InMemoryBackend) PutLifecycleHook(hook LifecycleHook) error {
	b.mu.Lock("PutLifecycleHook")
	defer b.mu.Unlock()

	if hook.LifecycleHookName == "" {
		return fmt.Errorf("%w: LifecycleHookName is required", ErrInvalidParameter)
	}

	if !b.groups.Has(hook.AutoScalingGroupName) {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, hook.AutoScalingGroupName)
	}

	if err := normalizeLifecycleHook(&hook); err != nil {
		return err
	}

	cp := hook
	b.putLifecycleHookLocked(&cp)

	return nil
}

// normalizeLifecycleHook validates a LifecycleHook's fields and fills in AWS
// defaults (HeartbeatTimeout=3600, DefaultResult=ABANDON, GlobalTimeout=HeartbeatTimeout).
// Shared by PutLifecycleHook and CreateAutoScalingGroup's LifecycleHookSpecificationList.
func normalizeLifecycleHook(hook *LifecycleHook) error {
	// Validate LifecycleTransition if provided
	if hook.LifecycleTransition != "" &&
		hook.LifecycleTransition != transitionLaunching &&
		hook.LifecycleTransition != transitionTerminating {
		return fmt.Errorf(
			"%w: LifecycleTransition must be %s or %s",
			ErrInvalidParameter, transitionLaunching, transitionTerminating,
		)
	}

	// Default HeartbeatTimeout to 3600 if not provided (matching AWS behavior).
	if hook.HeartbeatTimeout == 0 {
		hook.HeartbeatTimeout = defaultHeartbeatTimeout
	}

	// AWS: HeartbeatTimeout must be 30..172800 (48 h).
	const minHeartbeat = int32(30)
	const maxHeartbeat = int32(172800)

	if hook.HeartbeatTimeout < minHeartbeat || hook.HeartbeatTimeout > maxHeartbeat {
		return fmt.Errorf(
			"%w: HeartbeatTimeout must be between %d and %d seconds, got %d",
			ErrInvalidParameter, minHeartbeat, maxHeartbeat, hook.HeartbeatTimeout,
		)
	}

	// DefaultResult must be CONTINUE or ABANDON.
	if hook.DefaultResult != "" && hook.DefaultResult != lifecycleActionContinue &&
		hook.DefaultResult != lifecycleActionAbandon {
		return fmt.Errorf(
			"%w: DefaultResult must be CONTINUE or ABANDON, got %q",
			ErrInvalidParameter, hook.DefaultResult,
		)
	}

	// Default DefaultResult to ABANDON if not provided.
	if hook.DefaultResult == "" {
		hook.DefaultResult = lifecycleActionAbandon
	}

	// GlobalTimeout = HeartbeatTimeout * numberOfRetries; AWS uses numberOfRetries=1 by default.
	hook.GlobalTimeout = hook.HeartbeatTimeout

	return nil
}

// DescribeLifecycleHooks returns lifecycle hooks for the given group, optionally filtered by name.
func (b *InMemoryBackend) DescribeLifecycleHooks(groupName string, hookNames []string) ([]LifecycleHook, error) {
	b.mu.RLock("DescribeLifecycleHooks")
	defer b.mu.RUnlock()

	if !b.groups.Has(groupName) {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	hooks := b.lifecycleHooksByGroup.Get(groupName)

	if len(hookNames) > 0 {
		result := make([]LifecycleHook, 0, len(hookNames))

		for _, name := range hookNames {
			h, ok := b.lifecycleHooks.Get(scopedKey(groupName, name))
			if !ok {
				return nil, fmt.Errorf("%w: lifecycle hook %q not found", ErrLifecycleHookNotFound, name)
			}

			result = append(result, *h)
		}

		return result, nil
	}

	result := make([]LifecycleHook, 0, len(hooks))

	for _, h := range hooks {
		result = append(result, *h)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LifecycleHookName < result[j].LifecycleHookName
	})

	return result, nil
}

// resolveOutstandingHookActions completes, rather than silently cancels, any
// pending lifecycle action gated by hookName before the hook is deleted --
// api_op_DeleteLifecycleHook.go: "If there are any outstanding lifecycle
// actions, they are completed first (ABANDON for launching instances,
// CONTINUE for terminating instances)". Must run before hookName is removed
// from b.lifecycleHooks (applyLifecycleResult's chain lookup needs to still
// find it), and with b.mu held (write lock).
func (b *InMemoryBackend) resolveOutstandingHookActions(groupName, hookName string) {
	var toResolve []*pendingHookAction

	b.pendingHookTokens.Range(func(action *pendingHookAction) bool {
		if action.GroupName == groupName && action.HookName == hookName {
			toResolve = append(toResolve, action)
		}

		return true
	})

	for _, action := range toResolve {
		action.timer.Stop()
		b.pendingHookTokens.Delete(action.Token)

		result := lifecycleActionAbandon
		if action.Transition == transitionTerminating {
			result = lifecycleActionContinue
		}

		b.applyLifecycleResult(action, result)
	}
}

// cleanupHookTimers cancels and removes pending hook actions for a group/hook.
// If hookName is empty, all pending actions for the group are cancelled.
// Must be called with b.mu held (write lock).
func (b *InMemoryBackend) cleanupHookTimers(groupName, hookName string) {
	var toRemove []string

	b.pendingHookTokens.Range(func(action *pendingHookAction) bool {
		if action.GroupName == groupName && (hookName == "" || action.HookName == hookName) {
			action.timer.Stop()
			toRemove = append(toRemove, action.Token)
		}

		return true
	})

	for _, token := range toRemove {
		b.pendingHookTokens.Delete(token)
	}
}

// lifecycleHookChain returns the hooks registered for transition, ordered by
// registration Sequence (ties broken by name for determinism). AWS documents
// hooks on one transition as an ORDERED CHAIN, not concurrent execution:
// ABANDON "stops any remaining actions, such as other lifecycle hooks" while
// CONTINUE "allows any other lifecycle hooks to complete" (lifecycle-hooks.html).
// Neither PutLifecycleHookInput nor LifecycleHookSpecification carries an
// order/priority field, so registration order is the defensible default.
// Must be called with b.mu held.
func lifecycleHookChain(hooks []*LifecycleHook, transition string) []*LifecycleHook {
	chain := make([]*LifecycleHook, 0, len(hooks))

	for _, h := range hooks {
		if h.LifecycleTransition == transition {
			chain = append(chain, h)
		}
	}

	sort.Slice(chain, func(i, j int) bool {
		if chain[i].Sequence != chain[j].Sequence {
			return chain[i].Sequence < chain[j].Sequence
		}

		return chain[i].LifecycleHookName < chain[j].LifecycleHookName
	})

	return chain
}

// firstHookInChain returns the first hook (by chain order) registered for
// transition, or nil if none is registered. Must be called with b.mu held.
func firstHookInChain(hooks []*LifecycleHook, transition string) *LifecycleHook {
	chain := lifecycleHookChain(hooks, transition)
	if len(chain) == 0 {
		return nil
	}

	return chain[0]
}

// nextHookInChain returns the hook chained after currentHookName for
// transition, or nil if currentHookName was last in the chain, or is no
// longer registered (e.g. deleted mid-wait, in which case the chain stops
// rather than guessing a position). Must be called with b.mu held.
func nextHookInChain(hooks []*LifecycleHook, transition, currentHookName string) *LifecycleHook {
	chain := lifecycleHookChain(hooks, transition)

	for i, h := range chain {
		if h.LifecycleHookName == currentHookName {
			if i+1 < len(chain) {
				return chain[i+1]
			}

			return nil
		}
	}

	return nil
}

// armLifecycleWait puts inst into the appropriate "Wait" lifecycle state and starts a
// heartbeat timer that resolves the action with the hook's DefaultResult if
// CompleteLifecycleAction/RecordLifecycleActionHeartbeat don't intervene first. Must
// be called with b.mu held (write lock).
func (b *InMemoryBackend) armLifecycleWait(
	g *AutoScalingGroup, hook *LifecycleHook, inst *Instance, transition string, disposition terminationDisposition,
) {
	if transition == transitionLaunching {
		inst.LifecycleState = lifecycleStatePendingWait
	} else {
		inst.LifecycleState = lifecycleStateTerminatingWait
	}

	inst.LifecycleHookName = hook.LifecycleHookName

	token := uuid.NewString()
	timeout := time.Duration(hook.HeartbeatTimeout) * time.Second

	action := &pendingHookAction{
		Token:         token,
		GroupName:     g.AutoScalingGroupName,
		HookName:      hook.LifecycleHookName,
		InstanceID:    inst.InstanceID,
		Transition:    transition,
		DefaultResult: hook.DefaultResult,
		timeout:       timeout,
		Disposition:   disposition,
	}
	action.timer = time.AfterFunc(timeout, func() {
		b.resolveLifecycleWait(token, action.DefaultResult)
	})
	b.pendingHookTokens.Put(action)
}

// resolveLifecycleWait applies result (CONTINUE/ABANDON) to the pending action
// identified by token, if it is still pending. Called both from expired timers (its
// own goroutine, hence it takes the lock itself) and, indirectly, from explicit
// CompleteLifecycleAction calls.
func (b *InMemoryBackend) resolveLifecycleWait(token, result string) {
	b.mu.Lock("resolveLifecycleWait")
	defer b.mu.Unlock()

	action, ok := b.pendingHookTokens.Get(token)
	if !ok {
		return // already resolved (race between timer and an explicit Complete call)
	}

	b.pendingHookTokens.Delete(token)
	b.applyLifecycleResult(action, result)
}

// applyLifecycleResult performs the actual state transition once a lifecycle wait
// resolves (either explicitly via CompleteLifecycleAction or via heartbeat timeout).
// Must be called with b.mu held (write lock).
func (b *InMemoryBackend) applyLifecycleResult(action *pendingHookAction, result string) {
	g, ok := b.groups.Get(action.GroupName)
	if !ok {
		return // group was deleted while the action was pending
	}

	// CONTINUE "allows any other lifecycle hooks to complete" (lifecycle-hooks.html):
	// if another hook is chained after this one, arm it and stop here. ABANDON must
	// never reach armNextInChain -- it "stops any remaining actions, such as other
	// lifecycle hooks" and falls through to the terminal handling below instead.
	if strings.EqualFold(result, lifecycleActionContinue) && b.armNextInChain(g, action) {
		return
	}

	switch action.Transition {
	case transitionLaunching:
		if strings.EqualFold(result, lifecycleActionContinue) {
			for i := range g.Instances {
				if g.Instances[i].InstanceID == action.InstanceID {
					g.Instances[i].LifecycleState = lifecycleStateInService
					g.Instances[i].LifecycleHookName = ""

					break
				}
			}
		} else {
			// ABANDON: "we can terminate and replace the instance" (AWS docs,
			// lifecycle-hooks.html). Terminate the failed instance, then top the
			// group back up to DesiredCapacity exactly as finishTermination's
			// terminationReplace disposition does, so the replacement is itself
			// gated by the same launching hook (from its first hook, chained forward
			// like any other new instance).
			b.removeInstanceByID(g, action.InstanceID)

			oldLen := len(g.Instances)
			g.Instances = b.adjustInstances(g, g.Instances, g.DesiredCapacity)

			for _, inst := range g.Instances[oldLen:] {
				b.instanceIndex[inst.InstanceID] = g.AutoScalingGroupName
			}

			b.gateNewLaunchInstances(g, oldLen)
		}
	case transitionTerminating:
		// Reached on ABANDON at any chain position, or CONTINUE with no further
		// chained hook -- both proceed to the actual termination.
		b.finishTermination(g, action)
	}
}

// armNextInChain re-arms action's instance on the hook chained after
// action.HookName for action.Transition, if one is registered, and reports
// whether it did so. Must be called with b.mu held.
func (b *InMemoryBackend) armNextInChain(g *AutoScalingGroup, action *pendingHookAction) bool {
	next := nextHookInChain(b.lifecycleHooksByGroup.Get(g.AutoScalingGroupName), action.Transition, action.HookName)
	if next == nil {
		return false
	}

	for i := range g.Instances {
		if g.Instances[i].InstanceID == action.InstanceID {
			b.armLifecycleWait(g, next, &g.Instances[i], action.Transition, action.Disposition)

			return true
		}
	}

	return false
}

// rearmPendingWaits re-arms heartbeat timers for any instances left in a lifecycle
// "Wait" state by a restored snapshot. In-flight timers are never persisted (see
// pendingHookTokens/backendSnapshot), so without this an instance restored mid-wait
// would be stuck in Pending:Wait/Terminating:Wait forever. Must be called with b.mu
// held (write lock); intended to run once, right after Restore repopulates b.groups.
func (b *InMemoryBackend) rearmPendingWaits() {
	for _, g := range b.groups.All() {
		for i := range g.Instances {
			inst := &g.Instances[i]

			var transition string

			switch inst.LifecycleState {
			case lifecycleStatePendingWait:
				transition = transitionLaunching
			case lifecycleStateTerminatingWait:
				transition = transitionTerminating
			default:
				continue
			}

			// Resume at the hook the instance was actually waiting on, not the start
			// of the chain -- inst.LifecycleHookName is empty only for a
			// pre-chain-tracking snapshot or a hook deleted while persisted, in
			// which case restarting the chain is the best available fallback.
			hook, ok := b.lifecycleHooks.Get(scopedKey(g.AutoScalingGroupName, inst.LifecycleHookName))
			if !ok {
				hook = firstHookInChain(b.lifecycleHooksByGroup.Get(g.AutoScalingGroupName), transition)
			}

			heartbeat := defaultHeartbeatTimeout
			defaultResult := lifecycleActionAbandon
			hookName := ""

			if hook != nil {
				heartbeat = hook.HeartbeatTimeout
				defaultResult = hook.DefaultResult
				hookName = hook.LifecycleHookName
				inst.LifecycleHookName = hookName
			}

			token := uuid.NewString()
			action := &pendingHookAction{
				Token:         token,
				GroupName:     g.AutoScalingGroupName,
				HookName:      hookName,
				InstanceID:    inst.InstanceID,
				Transition:    transition,
				DefaultResult: defaultResult,
				timeout:       time.Duration(heartbeat) * time.Second,
			}
			action.timer = time.AfterFunc(action.timeout, func() {
				b.resolveLifecycleWait(token, action.DefaultResult)
			})
			b.pendingHookTokens.Put(action)
		}
	}
}

// findPendingHookAction looks up a pending lifecycle action, first by explicit token
// (as AWS does) and, when the token is empty, by (groupName, hookName, instanceID) —
// AWS's CompleteLifecycleAction and RecordLifecycleActionHeartbeat both accept either
// a token or an instance ID. Must be called with b.mu held.
func (b *InMemoryBackend) findPendingHookAction(token, groupName, hookName, instanceID string) *pendingHookAction {
	if token != "" {
		action, _ := b.pendingHookTokens.Get(token)

		return action
	}

	var found *pendingHookAction

	b.pendingHookTokens.Range(func(action *pendingHookAction) bool {
		if action.GroupName == groupName && action.HookName == hookName && action.InstanceID == instanceID {
			found = action

			return false
		}

		return true
	})

	return found
}

// DescribeLifecycleHookTypes returns the supported lifecycle hook transition types.
func (b *InMemoryBackend) DescribeLifecycleHookTypes() ([]string, error) {
	return []string{
		"autoscaling:EC2_INSTANCE_LAUNCHING",
		"autoscaling:EC2_INSTANCE_TERMINATING",
	}, nil
}

// RecordLifecycleActionHeartbeat resets or validates a lifecycle action heartbeat.
func (b *InMemoryBackend) RecordLifecycleActionHeartbeat(input RecordLifecycleActionHeartbeatInput) error {
	b.mu.Lock("RecordLifecycleActionHeartbeat")
	defer b.mu.Unlock()

	if !b.groups.Has(input.AutoScalingGroupName) {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, input.AutoScalingGroupName)
	}

	if action := b.findPendingHookAction(
		input.LifecycleActionToken, input.AutoScalingGroupName, input.LifecycleHookName, input.InstanceID,
	); action != nil {
		action.timer.Stop()
		token := action.Token
		defaultResult := action.DefaultResult
		action.timer = time.AfterFunc(action.timeout, func() {
			b.resolveLifecycleWait(token, defaultResult)
		})

		return nil
	}

	// Validate hook exists
	if !b.lifecycleHooks.Has(scopedKey(input.AutoScalingGroupName, input.LifecycleHookName)) {
		return fmt.Errorf("%w: lifecycle hook %q not found", ErrLifecycleHookNotFound, input.LifecycleHookName)
	}

	return nil
}
