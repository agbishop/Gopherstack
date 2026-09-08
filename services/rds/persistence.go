package rds

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// rdsSnapshotVersion identifies the shape of backendSnapshot's Tables blob
// (i.e. the set/shape of resources registered on b.registry -- see
// registerAllTables in store_setup.go). It must be bumped whenever a change
// there would make an older snapshot unsafe to decode as the current shape.
// Restore compares this against the persisted value and discards (rather than
// attempts to partially decode) any mismatch -- see Restore below. This
// mirrors the services/ec2 (commit 12e611a4) and services/sqs (commit
// 0f09d77c) conversions.
// Bumped to 2: instanceRoles changed shape from map[string][]string (role ARNs, collapsing
// different FeatureName associations together) to map[string]map[string]string (instance ID ->
// FeatureName -> role ARN), matching AWS's per-feature role slots.
// Bumped to 3: clusterRoles changed shape from map[string][]string (bare role ARNs, with
// FeatureName never stored at all) to map[string][]DBClusterRole (role ARN + FeatureName pairs)
// so DescribeDBClusters can emit AssociatedRoles and a role ARN reused across two different
// explicit FeatureName values no longer silently drops the second association -- see
// gopherstack-1jkv and PARITY.md.
const rdsSnapshotVersion = 3

type backendSnapshot struct {
	Tables                  map[string]json.RawMessage             `json:"tables"`
	Tags                    map[string][]Tag                       `json:"tags"`
	ClusterRoles            map[string][]DBClusterRole             `json:"clusterRoles"`
	InstanceRoles           map[string]map[string]string           `json:"instanceRoles"`
	ProxyTargets            map[string][]DBProxyTarget             `json:"proxyTargets"`
	InstanceReadyAt         map[string]time.Time                   `json:"instanceReadyAt"`
	ClusterReadyAt          map[string]time.Time                   `json:"clusterReadyAt"`
	AutomatedBackups        map[string]*DBInstanceAutomatedBackup  `json:"automatedBackups"`
	SnapshotTenantDatabases map[string][]*DBSnapshotTenantDatabase `json:"snapshotTenantDatabases"`
	InstanceLogFiles        map[string][]DBLogFile                 `json:"instanceLogFiles"`
	InstanceLogContent      map[string]map[string]string           `json:"instanceLogContent"`
	AccountID               string                                 `json:"accountID"`
	Region                  string                                 `json:"region"`
	DefaultCACertificateID  string                                 `json:"defaultCACertificateID"`
	Version                 int                                    `json:"version"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables are plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad
		// input data. Log and skip the snapshot rather than panic, matching
		// the persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx, "rds: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:                 rdsSnapshotVersion,
		Tables:                  tables,
		Tags:                    b.tags,
		ClusterRoles:            b.clusterRoles,
		InstanceRoles:           b.instanceRoles,
		ProxyTargets:            b.proxyTargets,
		InstanceReadyAt:         b.instanceReadyAt,
		ClusterReadyAt:          b.clusterReadyAt,
		AutomatedBackups:        b.automatedBackups,
		SnapshotTenantDatabases: b.snapshotTenantDatabases,
		InstanceLogFiles:        b.instanceLogFiles,
		InstanceLogContent:      b.instanceLogContent,
		AccountID:               b.accountID,
		Region:                  b.region,
		DefaultCACertificateID:  b.defaultCACertificateID,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "rds: failed to marshal snapshot", "error", err)

		return nil
	}

	return data
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "rds", data, &snap); err != nil {
		return err
	}

	ensureNonNilMaps(&snap)

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != rdsSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption. Mirrors the services/ec2 and services/sqs conversions.
		logger.Load(ctx).WarnContext(ctx,
			"rds: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", rdsSnapshotVersion)

		b.registry.ResetAll()

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("rds: restore snapshot tables: %w", err)
	}

	b.tags = snap.Tags
	b.clusterRoles = snap.ClusterRoles
	b.instanceRoles = snap.InstanceRoles
	b.proxyTargets = snap.ProxyTargets
	b.instanceReadyAt = snap.InstanceReadyAt
	if snap.ClusterReadyAt != nil {
		b.clusterReadyAt = snap.ClusterReadyAt
	} else {
		b.clusterReadyAt = make(map[string]time.Time)
	}
	b.accountID = snap.AccountID
	b.region = snap.Region
	b.automatedBackups = snap.AutomatedBackups
	b.snapshotTenantDatabases = snap.SnapshotTenantDatabases
	b.instanceLogFiles = snap.InstanceLogFiles
	b.instanceLogContent = snap.InstanceLogContent
	if snap.DefaultCACertificateID != "" {
		b.defaultCACertificateID = snap.DefaultCACertificateID
	} else {
		b.defaultCACertificateID = defaultCACertificateID
	}
	// FIS fault state is transient — clear it on restore so stale faults are not retained.
	b.fisFailoverFaults = make(map[string]time.Time)

	// The reconciler goroutine is not part of the snapshot and does not
	// restart itself: without this, a restored instance/cluster whose
	// pending transition deadline has already passed (the common case,
	// since a restart takes longer than the transition delay) would stay
	// stuck in "creating"/"modifying"/"rebooting" forever, since no
	// Describe* call reconciles state and nothing else schedules the
	// reconciler until an unrelated instance/cluster op happens to run.
	b.reconcileInstancesLocked()
	if len(b.instanceReadyAt) > 0 || len(b.clusterReadyAt) > 0 {
		b.scheduleReconcilerLocked()
	}

	return nil
}

// ensureNonNilMaps initialises any nil maps in a deserialized snapshot so the backend
// never operates on nil maps after a restore.
func ensureNonNilMaps(snap *backendSnapshot) {
	if snap.Tags == nil {
		snap.Tags = make(map[string][]Tag)
	}

	if snap.ClusterRoles == nil {
		snap.ClusterRoles = make(map[string][]DBClusterRole)
	}

	if snap.InstanceRoles == nil {
		snap.InstanceRoles = make(map[string]map[string]string)
	}

	if snap.ProxyTargets == nil {
		snap.ProxyTargets = make(map[string][]DBProxyTarget)
	}

	if snap.InstanceReadyAt == nil {
		snap.InstanceReadyAt = make(map[string]time.Time)
	}

	if snap.AutomatedBackups == nil {
		snap.AutomatedBackups = make(map[string]*DBInstanceAutomatedBackup)
	}

	if snap.SnapshotTenantDatabases == nil {
		snap.SnapshotTenantDatabases = make(map[string][]*DBSnapshotTenantDatabase)
	}

	if snap.InstanceLogFiles == nil {
		snap.InstanceLogFiles = make(map[string][]DBLogFile)
	}

	if snap.InstanceLogContent == nil {
		snap.InstanceLogContent = make(map[string]map[string]string)
	}
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.defaultRegion == "" || len(h.backends) == 0 {
		return h.Backend.Snapshot(ctx)
	}

	snaps := make(map[string]json.RawMessage)
	for region, b := range h.backends {
		snaps[region] = b.Snapshot(ctx)
	}
	data, _ := json.Marshal(snaps)

	return data
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.defaultRegion == "" || len(h.backends) == 0 {
		return h.Backend.Restore(ctx, data)
	}

	var snaps map[string]json.RawMessage
	if err := json.Unmarshal(data, &snaps); err != nil {
		// Fallback for backwards compatibility with single-region snapshots.
		return h.Backend.Restore(ctx, data)
	}

	// Check if this might be a single-region snapshot that happens to unmarshal as map[string]json.RawMessage.
	// Since backendSnapshot has "tables", "tags", etc. as keys, they might match.
	// We can check if "tables" exists as a key.
	if _, hasTables := snaps["tables"]; hasTables {
		return h.Backend.Restore(ctx, data)
	}

	for region, raw := range snaps {
		b, ok := h.backends[region]
		if !ok {
			b = NewInMemoryBackend(h.accountID, region)
			h.backends[region] = b
			h.handlers[region] = &Handler{Backend: b}
		}
		if err := b.Restore(ctx, raw); err != nil {
			return err
		}
	}

	return nil
}
