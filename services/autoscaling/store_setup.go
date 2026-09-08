package autoscaling

// Code in this file supports Phase 3.3 of the datalayer refactor: every
// map[string]*T resource field on InMemoryBackend that is a pure function of
// the stored value's own identity is registered exactly once, here, as a
// *store.Table[T] on b.registry. See pkgs/store's package doc and the ec2
// (commit 12e611a4), sqs (commit 0f09d77c), and ecs (composite-key +
// secondary store.Index) conversions this follows.
//
// scheduledActions, lifecycleHooks, and scalingPolicies used to be nested
// map[string]map[string]*T (group name -> item name -> value). They are
// flattened into a single Table keyed by a composite "group/item" string,
// with a secondary store.Index grouping by AutoScalingGroupName so the old
// "all X in group Y" access pattern (b.scheduledActions[groupName], ...)
// still resolves in O(k).
//
// A handful of fields are deliberately NOT registered here and remain plain
// maps -- see the comment above registerAllTables for the list and why.
import (
	"fmt"
	"slices"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// describeByNames is the shared shape behind DescribeAutoScalingGroups and
// DescribeLaunchConfigurations: given a non-empty names filter, look each
// name up individually (erroring via notFound on the first miss, matching
// AWS's all-or-nothing DescribeAutoScalingGroups/DescribeLaunchConfigurations
// semantics); given an empty filter, return every table entry sorted by less.
func describeByNames[V any](
	table *store.Table[V], names []string, notFound error, less func(a, b *V) bool,
) ([]V, error) {
	if len(names) > 0 {
		result := make([]V, 0, len(names))

		for _, name := range names {
			v, ok := table.Get(name)
			if !ok {
				return nil, fmt.Errorf("%w: %q", notFound, name)
			}

			result = append(result, *v)
		}

		return result, nil
	}

	all := table.All()
	result := make([]V, 0, len(all))

	for _, v := range all {
		result = append(result, *v)
	}

	sort.Slice(result, func(i, j int) bool { return less(&result[i], &result[j]) })

	return result, nil
}

// scopedKey builds a composite store.Table key for a resource whose identity
// is only unique within an owning Auto Scaling group (scheduled action,
// lifecycle hook, and scaling policy names are only unique per group, not
// globally).
func scopedKey(groupName, name string) string { return groupName + "/" + name }

// ---- primary key functions ----

func groupsKeyFn(v *AutoScalingGroup) string                    { return v.AutoScalingGroupName }
func launchConfigurationsKeyFn(v *LaunchConfiguration) string   { return v.LaunchConfigurationName }
func warmPoolsKeyFn(v *WarmPool) string                         { return v.AutoScalingGroupName }
func pendingHookTokensKeyFn(v *pendingHookAction) string        { return v.Token }
func pendingRefreshActionsKeyFn(v *pendingRefreshAction) string { return v.ID }

func scheduledActionsKeyFn(v *ScheduledAction) string {
	return scopedKey(v.AutoScalingGroupName, v.ScheduledActionName)
}
func scheduledActionsGroupIndexKeyFn(v *ScheduledAction) string { return v.AutoScalingGroupName }

func lifecycleHooksKeyFn(v *LifecycleHook) string {
	return scopedKey(v.AutoScalingGroupName, v.LifecycleHookName)
}
func lifecycleHooksGroupIndexKeyFn(v *LifecycleHook) string { return v.AutoScalingGroupName }

func scalingPoliciesKeyFn(v *ScalingPolicy) string {
	return scopedKey(v.AutoScalingGroupName, v.PolicyName)
}
func scalingPoliciesGroupIndexKeyFn(v *ScalingPolicy) string { return v.AutoScalingGroupName }

// registerAllTables registers every converted resource map on b.registry
// exactly once. It must be called during construction only (immediately
// after b.registry is created), never on every reset -- store.Register
// panics on a duplicate name, so a full state wipe goes through
// registry.ResetAll() (plus pendingHookTokens.Reset(), since that table is
// intentionally unregistered) instead; see InMemoryBackend.Close and
// Restore.
//
// The following resource fields are deliberately left as plain maps (not
// registered here):
//   - activities: value type []ScalingActivity is a slice of the value type
//     itself (not *T), and many call sites append to a group's activity
//     history -- out of scope for store.Table, matching the ec2/ecs
//     precedent of leaving map[string][]*T-shaped fields raw (e.g. ecs's
//     taskDefinitions).
//   - instanceRefreshes: value type []*InstanceRefresh -- map[string][]*T is
//     the same out-of-scope shape as ecs's taskDefinitions/
//     daemonTaskDefinitions exclusion (revision/insertion order across the
//     slice is load-bearing there too).
//   - notificationConfigs: value type []*NotificationConfiguration -- same
//     map[string][]*T exclusion; NotificationConfiguration also carries no
//     identity field of its own (no ARN/ID), only reachable via its owning
//     group + topic, matching ec2's instanceIMDSOptions-style exclusion.
//   - instanceIndex: value type is a bare string (groupName), not a struct
//     with derivable identity -- a pure instanceID->groupName reverse-lookup
//     cache, not a resource collection, and already excluded from
//     persistence (rebuilt from b.groups on Restore).
func registerAllTables(b *InMemoryBackend) {
	b.groups = store.Register(b.registry, "groups", store.New(groupsKeyFn))
	b.launchConfigurations = store.Register(
		b.registry, "launchConfigurations", store.New(launchConfigurationsKeyFn),
	)
	b.warmPools = store.Register(b.registry, "warmPools", store.New(warmPoolsKeyFn))

	b.scheduledActions = store.Register(b.registry, "scheduledActions", store.New(scheduledActionsKeyFn))
	b.scheduledActionsByGroup = b.scheduledActions.AddIndex("byGroup", scheduledActionsGroupIndexKeyFn)

	b.lifecycleHooks = store.Register(b.registry, "lifecycleHooks", store.New(lifecycleHooksKeyFn))
	b.lifecycleHooksByGroup = b.lifecycleHooks.AddIndex("byGroup", lifecycleHooksGroupIndexKeyFn)

	b.scalingPolicies = store.Register(b.registry, "scalingPolicies", store.New(scalingPoliciesKeyFn))
	b.scalingPoliciesByGroup = b.scalingPolicies.AddIndex("byGroup", scalingPoliciesGroupIndexKeyFn)

	// pendingHookTokens and pendingRefreshActions are intentionally NOT
	// registered on b.registry: both hold live *time.Timer state that must
	// never be persisted (see Snapshot/Restore in persistence.go, which
	// cancel and rebuild timers explicitly instead of round-tripping them
	// through JSON).
	b.pendingHookTokens = store.New(pendingHookTokensKeyFn)
	b.pendingRefreshActions = store.New(pendingRefreshActionsKeyFn)
}

// ---- per-group helpers ----
//
// store.Index.Get returns a slice OWNED by the index -- callers must not
// mutate it, and must not hold onto it across a Table.Put/Delete/Restore on
// the same table (each of which may reuse or invalidate it via idx.remove).
// Every helper below that deletes while iterating therefore snapshots the
// group into a private copy first (slices.Clone), matching the cascade-
// delete pattern ecs uses for its own byCluster/byService indexes. All must
// be called with the write lock held.

// scheduledActionsInGroupLocked returns a snapshot copy of every scheduled
// action belonging to groupName.
func (b *InMemoryBackend) scheduledActionsInGroupLocked(groupName string) []*ScheduledAction {
	return slices.Clone(b.scheduledActionsByGroup.Get(groupName))
}

// lifecycleHooksInGroupLocked returns a snapshot copy of every lifecycle hook
// belonging to groupName.
func (b *InMemoryBackend) lifecycleHooksInGroupLocked(groupName string) []*LifecycleHook {
	return slices.Clone(b.lifecycleHooksByGroup.Get(groupName))
}

// scalingPoliciesInGroupLocked returns a snapshot copy of every scaling
// policy belonging to groupName.
func (b *InMemoryBackend) scalingPoliciesInGroupLocked(groupName string) []*ScalingPolicy {
	return slices.Clone(b.scalingPoliciesByGroup.Get(groupName))
}

// deleteScheduledActionsForGroupLocked removes every scheduled action
// belonging to groupName.
func (b *InMemoryBackend) deleteScheduledActionsForGroupLocked(groupName string) {
	for _, a := range b.scheduledActionsInGroupLocked(groupName) {
		b.scheduledActions.Delete(scheduledActionsKeyFn(a))
	}
}

// deleteLifecycleHooksForGroupLocked removes every lifecycle hook belonging
// to groupName.
func (b *InMemoryBackend) deleteLifecycleHooksForGroupLocked(groupName string) {
	for _, h := range b.lifecycleHooksInGroupLocked(groupName) {
		b.lifecycleHooks.Delete(lifecycleHooksKeyFn(h))
	}
}

// deleteScalingPoliciesForGroupLocked removes every scaling policy belonging
// to groupName.
func (b *InMemoryBackend) deleteScalingPoliciesForGroupLocked(groupName string) {
	for _, p := range b.scalingPoliciesInGroupLocked(groupName) {
		b.scalingPolicies.Delete(scalingPoliciesKeyFn(p))
	}
}
