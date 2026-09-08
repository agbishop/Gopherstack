package autoscaling

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// autoscalingSnapshotVersion identifies the shape of [backendSnapshot]. It
// must be bumped whenever a change to backendSnapshot (or any value type a
// registered store.Table holds) would make an older snapshot unsafe to
// decode as the current shape (e.g. a field's meaning or type changes).
// Restore compares this against the persisted value and discards (rather
// than attempts to partially decode) any mismatch -- see Restore below. This
// is the first versioned shape (the pre-Phase-3.3 snapshot had no version
// field at all, so it unmarshals as Version 0 and is correctly treated as
// incompatible).
const autoscalingSnapshotVersion = 1

// backendSnapshot is the top-level on-disk shape for the Auto Scaling
// backend.
//
// Tables holds one JSON-encoded array per store.Table registered on
// b.registry (groups, launchConfigurations, warmPools, scheduledActions,
// lifecycleHooks, scalingPolicies), produced by store.Registry.SnapshotAll().
// Every value type behind those tables (AutoScalingGroup, LaunchConfiguration,
// WarmPool, ScheduledAction, LifecycleHook, ScalingPolicy) is a plain,
// fully-JSON-serialisable struct, so they are stored directly rather than
// through a separate DTO.
//
// Activities, InstanceRefreshes, and NotificationConfigs remain plain
// map[string][]T/[]*T fields -- see store_setup.go's registerAllTables doc
// for why those three are not store.Table-backed.
type backendSnapshot struct {
	Tables              map[string]json.RawMessage              `json:"tables"`
	Activities          map[string][]ScalingActivity            `json:"activities"`
	InstanceRefreshes   map[string][]*InstanceRefresh           `json:"instanceRefreshes"`
	NotificationConfigs map[string][]*NotificationConfiguration `json:"notificationConfigs"`
	Version             int                                     `json:"version"`
}

// Snapshot serialises the backend state to JSON.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables hold plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad
		// input data. Log and skip the snapshot rather than panic, matching
		// the persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx,
			"autoscaling: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:             autoscalingSnapshotVersion,
		Tables:              tables,
		Activities:          b.activities,
		InstanceRefreshes:   b.instanceRefreshes,
		NotificationConfigs: b.notificationConfigs,
	}

	return persistence.MarshalSnapshot(ctx, "autoscaling", snap)
}

// Restore populates the backend from a JSON payload.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "autoscaling", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	// Cancel any in-flight lifecycle action timers before replacing state (item 28).
	b.pendingHookTokens.Range(func(action *pendingHookAction) bool {
		action.timer.Stop()

		return true
	})
	b.pendingHookTokens.Reset()

	// Same for in-flight instance-refresh transition timers.
	b.pendingRefreshActions.Range(func(action *pendingRefreshAction) bool {
		action.timer.Stop()

		return true
	})
	b.pendingRefreshActions.Reset()

	if snap.Version != autoscalingSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields (e.g. the pre-Phase-3.3 nested
		// map[string]map[string]*T shape unmarshaling into today's flattened
		// composite-key tables). Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"autoscaling: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", autoscalingSnapshotVersion)

		b.registry.ResetAll()
		b.activities = make(map[string][]ScalingActivity)
		b.instanceRefreshes = make(map[string][]*InstanceRefresh)
		b.notificationConfigs = make(map[string][]*NotificationConfiguration)
		b.instanceIndex = make(map[string]string)
		b.nextHookSeq = 0

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("autoscaling: restore snapshot tables: %w", err)
	}

	// nextHookSeq is not itself persisted; recompute it from the restored hooks
	// so any hook registered post-restore chains after all of them.
	b.recomputeNextHookSeqLocked()

	if snap.Activities != nil {
		b.activities = snap.Activities
	} else {
		b.activities = make(map[string][]ScalingActivity)
	}

	if snap.InstanceRefreshes != nil {
		b.instanceRefreshes = snap.InstanceRefreshes
	} else {
		b.instanceRefreshes = make(map[string][]*InstanceRefresh)
	}

	if snap.NotificationConfigs != nil {
		b.notificationConfigs = snap.NotificationConfigs
	} else {
		b.notificationConfigs = make(map[string][]*NotificationConfiguration)
	}

	// Rebuild instance index from restored groups.
	b.instanceIndex = make(map[string]string)
	for _, g := range b.groups.All() {
		for _, inst := range g.Instances {
			b.instanceIndex[inst.InstanceID] = g.AutoScalingGroupName
		}
	}

	// Re-arm heartbeat timers for any instances restored mid lifecycle-hook wait;
	// in-flight timers themselves are never persisted (see pendingHookTokens above).
	b.rearmPendingWaits()

	// Re-arm transition timers for any instance refresh restored mid InProgress,
	// Cancelling, or RollbackInProgress; same rationale as rearmPendingWaits.
	b.rearmPendingRefreshes()

	return nil
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}
	if s, ok := h.Backend.(snapshotter); ok {
		return s.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}
	if r, ok := h.Backend.(restorer); ok {
		return r.Restore(ctx, data)
	}

	return nil
}
