package redshift

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// AuthorizeSnapshotAccess grants another account restore access to a snapshot.
func (b *InMemoryBackend) AuthorizeSnapshotAccess(snapshotID, accountWithRestoreAccess string) (*Snapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotIdentifier is required", ErrInvalidParameter)
	}
	if accountWithRestoreAccess == "" {
		return nil, fmt.Errorf("%w: AccountWithRestoreAccess is required", ErrInvalidParameter)
	}

	b.mu.Lock("AuthorizeSnapshotAccess")
	defer b.mu.Unlock()

	snap, exists := b.snapshots.Get(snapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: snapshot %s not found", ErrSnapshotNotFound, snapshotID)
	}

	for _, a := range snap.AccountsWithRestoreAccess {
		if a.AccountID == accountWithRestoreAccess {
			return nil, fmt.Errorf(
				"%w: account %s already has restore access to snapshot %s",
				ErrAuthorizationAlreadyExists, accountWithRestoreAccess, snapshotID,
			)
		}
	}

	snap.AccountsWithRestoreAccess = append(snap.AccountsWithRestoreAccess, AccountWithRestoreAccess{
		AccountID: accountWithRestoreAccess,
	})

	return cloneSnapshot(snap), nil
}

// RevokeSnapshotAccess removes restore access for the given account from a snapshot.
func (b *InMemoryBackend) RevokeSnapshotAccess(snapshotID, accountWithRestoreAccess string) (*Snapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotIdentifier is required", ErrInvalidParameter)
	}
	if accountWithRestoreAccess == "" {
		return nil, fmt.Errorf("%w: AccountWithRestoreAccess is required", ErrInvalidParameter)
	}

	b.mu.Lock("RevokeSnapshotAccess")
	defer b.mu.Unlock()

	snap, exists := b.snapshots.Get(snapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: snapshot %s not found", ErrSnapshotNotFound, snapshotID)
	}

	filtered := make([]AccountWithRestoreAccess, 0, len(snap.AccountsWithRestoreAccess))
	found := false

	for _, a := range snap.AccountsWithRestoreAccess {
		if a.AccountID == accountWithRestoreAccess {
			found = true

			continue
		}

		filtered = append(filtered, a)
	}

	if !found {
		return nil, fmt.Errorf(
			"%w: account %s does not have restore access",
			ErrSnapshotAccessNotFound,
			accountWithRestoreAccess,
		)
	}

	snap.AccountsWithRestoreAccess = filtered

	return cloneSnapshot(snap), nil
}

// ModifyClusterSnapshot updates the manual retention period of a snapshot.
// A nil retentionPeriod means the caller omitted ManualSnapshotRetentionPeriod
// entirely (e.g. a Force-only call) and leaves the existing value untouched --
// distinct from an explicit -1, which real AWS defines as "retain indefinitely".
func (b *InMemoryBackend) ModifyClusterSnapshot(snapshotID string, retentionPeriod *int, _ bool) (*Snapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyClusterSnapshot")
	defer b.mu.Unlock()

	snap, exists := b.snapshots.Get(snapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: snapshot %s not found", ErrSnapshotNotFound, snapshotID)
	}

	if retentionPeriod != nil {
		snap.ManualSnapshotRetentionPeriod = *retentionPeriod
	}

	return cloneSnapshot(snap), nil
}

// BatchDeleteClusterSnapshots deletes multiple cluster snapshots. It returns the list of errors for
// snapshots that could not be deleted and the list of successfully deleted snapshot identifiers.
func (b *InMemoryBackend) BatchDeleteClusterSnapshots(identifiers []string) ([]SnapshotBatchError, []string) {
	b.mu.Lock("BatchDeleteClusterSnapshots")
	defer b.mu.Unlock()

	var batchErrors []SnapshotBatchError

	var deleted []string

	for _, id := range identifiers {
		if _, exists := b.snapshots.Get(id); !exists {
			batchErrors = append(batchErrors, SnapshotBatchError{
				SnapshotIdentifier: id,
				FailureCode:        errClusterSnapshotNotFound,
				FailureReason:      fmt.Sprintf("snapshot %s not found", id),
			})

			continue
		}

		b.snapshots.Delete(id)
		deleted = append(deleted, id)
	}

	return batchErrors, deleted
}

// BatchModifyClusterSnapshots modifies the retention period for a list of snapshots.
// The force parameter is accepted for API compatibility but has no effect in the in-memory backend.
// A nil retentionPeriod means the caller omitted ManualSnapshotRetentionPeriod
// entirely and leaves each snapshot's existing value untouched -- same
// omitted-vs-explicit-(-1) distinction as ModifyClusterSnapshot.
// Returns errors and the list of successfully modified snapshot identifiers.
func (b *InMemoryBackend) BatchModifyClusterSnapshots(
	identifiers []string,
	retentionPeriod *int,
	_ bool,
) ([]SnapshotBatchError, []string) {
	b.mu.Lock("BatchModifyClusterSnapshots")
	defer b.mu.Unlock()

	var batchErrors []SnapshotBatchError

	var modified []string

	for _, id := range identifiers {
		snap, exists := b.snapshots.Get(id)
		if !exists {
			batchErrors = append(batchErrors, SnapshotBatchError{
				SnapshotIdentifier: id,
				FailureCode:        errClusterSnapshotNotFound,
				FailureReason:      fmt.Sprintf("snapshot %s not found", id),
			})

			continue
		}

		if retentionPeriod != nil {
			snap.ManualSnapshotRetentionPeriod = *retentionPeriod
		}

		modified = append(modified, id)
	}

	return batchErrors, modified
}

// AddSnapshotInternal seeds a snapshot directly into the backend.
func (b *InMemoryBackend) AddSnapshotInternal(snap *Snapshot) {
	b.mu.Lock("AddSnapshotInternal")
	defer b.mu.Unlock()
	b.snapshots.Put(snap)
}

// cloneSnapshot returns a deep copy of a Snapshot.
func cloneSnapshot(snap *Snapshot) *Snapshot {
	cp := *snap
	cp.AccountsWithRestoreAccess = make([]AccountWithRestoreAccess, len(snap.AccountsWithRestoreAccess))
	copy(cp.AccountsWithRestoreAccess, snap.AccountsWithRestoreAccess)

	return &cp
}

// CreateClusterSnapshot creates a manual snapshot of the specified cluster.
func (b *InMemoryBackend) CreateClusterSnapshot(snapshotID, clusterID string) (*Snapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotIdentifier is required", ErrInvalidParameter)
	}
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateClusterSnapshot")
	defer b.mu.Unlock()

	srcCluster, exists := b.clusters.Get(clusterID)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	if b.snapshots.Has(snapshotID) {
		return nil, fmt.Errorf("%w: snapshot %s already exists", ErrSnapshotAlreadyExists, snapshotID)
	}

	snap := &Snapshot{
		SnapshotIdentifier:            snapshotID,
		ClusterIdentifier:             clusterID,
		SnapshotType:                  "manual",
		Status:                        "available",
		AccountsWithRestoreAccess:     []AccountWithRestoreAccess{},
		ManualSnapshotRetentionPeriod: -1,
		SnapshotCreateTime:            time.Now(),
		NodeType:                      srcCluster.NodeType,
		DBName:                        srcCluster.DBName,
		MasterUsername:                srcCluster.MasterUsername,
		NumberOfNodes:                 srcCluster.NumberOfNodes,
	}
	b.snapshots.Put(snap)

	return cloneSnapshot(snap), nil
}

// DeleteClusterSnapshot removes a cluster snapshot.
func (b *InMemoryBackend) DeleteClusterSnapshot(snapshotID string) (*Snapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteClusterSnapshot")
	defer b.mu.Unlock()

	snap, exists := b.snapshots.Get(snapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: snapshot %s not found", ErrSnapshotNotFound, snapshotID)
	}

	if len(snap.AccountsWithRestoreAccess) > 0 {
		return nil, fmt.Errorf(
			"%w: snapshot %s still has accounts authorized for restore access; revoke access first",
			ErrSnapshotHasAuthorizedAccounts, snapshotID,
		)
	}

	cp := cloneSnapshot(snap)
	b.snapshots.Delete(snapshotID)

	return cp, nil
}

// DescribeClusterSnapshots returns snapshots, optionally filtered by snapshotID, clusterID,
// snapshotType ("manual" or "automated"), and clusterExists. An empty snapshotType matches all
// types. Results are ordered by SnapshotIdentifier ascending so handleDescribeClusterSnapshots'
// Marker-based pagination (handler_snapshots.go) sees a reproducible order across calls.
//
// clusterExists, when non-nil, filters snapshots by whether their ClusterIdentifier is still
// present in this account's cluster table -- a single-account existence check, not the
// cross-account snapshot-ownership model OwnerAccount would need (DescribeClusterSnapshotsInput
// doc, api_op_DescribeClusterSnapshots.go: true selects snapshots of a still-existing cluster
// and requires clusterID; false selects snapshots whose cluster no longer exists, which with no
// clusterID given means every orphaned snapshot).
func (b *InMemoryBackend) DescribeClusterSnapshots(
	snapshotID, clusterID, snapshotType string, clusterExists *bool,
) ([]Snapshot, error) {
	if clusterExists != nil && *clusterExists && clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required when ClusterExists is true", ErrInvalidParameter)
	}

	b.mu.RLock("DescribeClusterSnapshots")
	defer b.mu.RUnlock()

	if snapshotID != "" {
		snap, exists := b.snapshots.Get(snapshotID)
		if !exists {
			return nil, fmt.Errorf("%w: snapshot %s not found", ErrSnapshotNotFound, snapshotID)
		}

		return []Snapshot{*cloneSnapshot(snap)}, nil
	}

	result := make([]Snapshot, 0, b.snapshots.Len())

	for _, snap := range b.snapshots.Snapshot() {
		if clusterID != "" && snap.ClusterIdentifier != clusterID {
			continue
		}
		if snapshotType != "" && snap.SnapshotType != snapshotType {
			continue
		}
		if clusterExists != nil {
			if _, exists := b.clusters.Get(snap.ClusterIdentifier); exists != *clusterExists {
				continue
			}
		}
		result = append(result, *cloneSnapshot(snap))
	}

	return result, nil
}

// CopyClusterSnapshot copies a snapshot to a new identifier, optionally to a different region.
func (b *InMemoryBackend) CopyClusterSnapshot(
	sourceSnapshotID, destinationSnapshotID string,
) (*Snapshot, error) {
	if sourceSnapshotID == "" {
		return nil, fmt.Errorf("%w: SourceSnapshotIdentifier is required", ErrInvalidParameter)
	}
	if destinationSnapshotID == "" {
		return nil, fmt.Errorf("%w: TargetSnapshotIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("CopyClusterSnapshot")
	defer b.mu.Unlock()

	src, exists := b.snapshots.Get(sourceSnapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: snapshot %s not found", ErrSnapshotNotFound, sourceSnapshotID)
	}

	if _, dstExists := b.snapshots.Get(destinationSnapshotID); dstExists {
		return nil, fmt.Errorf("%w: snapshot %s already exists", ErrSnapshotAlreadyExists, destinationSnapshotID)
	}

	cp := cloneSnapshot(src)
	cp.SnapshotIdentifier = destinationSnapshotID
	cp.SnapshotType = "manual"
	b.snapshots.Put(cp)

	result := cloneSnapshot(cp)

	return result, nil
}

// RestoreFromClusterSnapshot creates a new cluster from an existing snapshot.
func (b *InMemoryBackend) RestoreFromClusterSnapshot(clusterID, snapshotID string) (*Cluster, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("RestoreFromClusterSnapshot")
	defer b.mu.Unlock()

	snap, exists := b.snapshots.Get(snapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: snapshot %s not found", ErrSnapshotNotFound, snapshotID)
	}

	if _, clusterExists := b.clusters.Get(clusterID); clusterExists {
		return nil, fmt.Errorf("%w: cluster %s already exists", ErrClusterAlreadyExists, clusterID)
	}

	nodeType := snap.NodeType
	if nodeType == "" {
		nodeType = defaultNodeType
	}

	dbName := snap.DBName
	if dbName == "" {
		dbName = defaultDBName
	}

	masterUser := snap.MasterUsername
	if masterUser == "" {
		masterUser = defaultMasterUsername
	}

	numberOfNodes := snap.NumberOfNodes
	if numberOfNodes == 0 {
		numberOfNodes = 1
	}

	endpoint := fmt.Sprintf("%s.%s.%s.redshift.amazonaws.com", clusterID, b.accountID, b.region)

	// Mirror CreateCluster's lifecycle model: when no activation delay is
	// configured, the restored cluster is immediately "available" (matching
	// every other synchronous lifecycle transition in this backend). When a
	// delay is configured, start in "restoring" and schedule the transition
	// to "available" via the managed reconciler -- previously this status
	// was hardcoded to "restoring" with no transition ever scheduled, so a
	// restored cluster stayed stuck in "restoring" forever and SDK waiters
	// polling for cluster-available would never observe completion.
	initialStatus := clusterStatusAvailable
	if b.clusterActivationDelay > 0 {
		initialStatus = "restoring"
	}

	cluster := &Cluster{
		ClusterIdentifier: clusterID,
		NodeType:          nodeType,
		ClusterType:       clusterTypeMultiNode,
		Endpoint:          endpoint,
		Status:            initialStatus,
		DBName:            dbName,
		MasterUsername:    masterUser,
		Port:              defaultPort,
		NumberOfNodes:     numberOfNodes,
		// Every cluster must own a live Tags collection: CreateCluster does
		// this, and DescribeTags/CreateTags/DeleteTags call methods on
		// cluster.Tags unconditionally for every cluster in the backend. A
		// nil Tags here previously caused a nil-pointer panic the moment
		// DescribeTags (or a tag-filtered DescribeClusters) ran after any
		// restore.
		Tags: tags.New("redshift.cluster." + clusterID + ".tags"),
	}

	b.clusters.Put(cluster)

	if b.clusterActivationDelay > 0 {
		b.scheduleClusterTransitionLocked(clusterID, &clusterTransition{
			effectiveAt: time.Now().Add(b.clusterActivationDelay),
			status:      clusterStatusAvailable,
		})
	}

	if b.dnsRegistrar != nil {
		b.dnsRegistrar.Register(endpoint)
	}

	cp := cloneCluster(cluster)

	return &cp, nil
}
