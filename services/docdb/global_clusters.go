package docdb

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// This file backs the "GlobalCluster has no GlobalClusterMembers subresource"
// gap identified in PARITY.md: CreateGlobalCluster never added the source
// cluster as a member, DescribeGlobalClusters always reported an empty
// member list, and RemoveFromGlobalCluster was a pure no-op with respect to
// membership. types.GlobalCluster.GlobalClusterMembers ([]GlobalClusterMember,
// tracking each member cluster's ARN/IsWriter/Readers) is now backed by a
// real GlobalClusterMembers field, populated on Create (when
// SourceDBClusterIdentifier resolves to a real cluster) and mutated for real
// by Failover/Switchover/Remove -- mirroring the already-completed neptune
// service's identical fix (services/neptune/global_clusters.go).

// isDocDBARN reports whether s looks like an AWS ARN, as opposed to a bare
// resource identifier -- both forms are accepted for SourceDBClusterIdentifier/
// TargetDbClusterIdentifier/DbClusterIdentifier parameters throughout this
// file, matching this backend's existing leniency elsewhere.
func isDocDBARN(s string) bool { return strings.HasPrefix(s, "arn:") }

// resolveClusterARN resolves a caller-supplied cluster reference (either an
// ARN or a bare DBClusterIdentifier looked up in region) to its ARN. The
// second return value reports whether the reference identifies a real
// cluster this backend knows about.
func (b *InMemoryBackend) resolveClusterARN(region, ref string) (string, bool) {
	if ref == "" {
		return "", false
	}
	if isDocDBARN(ref) {
		return ref, true
	}
	if c, exists := b.clusterGet(region, ref); exists {
		return b.clusterARN(region, c.DBClusterIdentifier), true
	}

	return ref, false
}

// CreateGlobalCluster creates a global cluster. When sourceDBClusterID
// resolves to a real cluster in the caller's region, it is added as the
// initial (and, per AWS's current one-item limit, sole) writer member.
func (b *InMemoryBackend) CreateGlobalCluster(
	ctx context.Context,
	id, sourceDBClusterID, engine, engineVersion string,
) (*GlobalCluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: GlobalClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateGlobalCluster")
	defer b.mu.Unlock()
	if b.globalClusters.Has(id) {
		return nil, fmt.Errorf("%w: global cluster %s already exists", ErrGlobalClusterAlreadyExists, id)
	}
	if engine == "" {
		engine = docDBEngine
	}
	if engineVersion == "" {
		engineVersion = defaultEngineVersion
	}
	gc := &GlobalCluster{
		GlobalClusterIdentifier: id,
		SourceDBClusterID:       sourceDBClusterID,
		Status:                  statusAvailable,
		Engine:                  engine,
		EngineVersion:           engineVersion,
		GlobalClusterArn:        b.globalClusterARN(id),
	}
	if clusterARN, exists := b.resolveClusterARN(region, sourceDBClusterID); exists {
		gc.GlobalClusterMembers = []GlobalClusterMember{
			{DBClusterArn: clusterARN, IsWriter: true, SynchronizationStatus: "synced"},
		}
		if c, ok := b.clusterGet(region, sourceDBClusterID); ok {
			gc.EngineVersion = c.EngineVersion
			gc.StorageEncrypted = c.StorageEncrypted
		}
	}
	b.globalClusters.Put(gc)

	return copyGlobalCluster(gc), nil
}

// DeleteGlobalCluster deletes a global cluster.
func (b *InMemoryBackend) DeleteGlobalCluster(_ context.Context, id string) (*GlobalCluster, error) {
	b.mu.Lock("DeleteGlobalCluster")
	defer b.mu.Unlock()
	gc, exists := b.globalClusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: global cluster %s not found", ErrGlobalClusterNotFound, id)
	}

	if gc.DeletionProtection {
		return nil, fmt.Errorf(
			"%w: cannot delete protected global cluster %s, disable deletion protection first",
			ErrInvalidGlobalClusterState, id,
		)
	}

	if len(gc.GlobalClusterMembers) > 0 {
		return nil, fmt.Errorf(
			"%w: global cluster %s still has %d member cluster(s) attached, detach or delete them first",
			ErrInvalidGlobalClusterState, id, len(gc.GlobalClusterMembers),
		)
	}

	cp := copyGlobalCluster(gc)
	b.globalClusters.Delete(id)

	return cp, nil
}

// DescribeGlobalClusters returns global clusters, optionally filtered by ID, sorted by identifier.
func (b *InMemoryBackend) DescribeGlobalClusters(_ context.Context, id string) []GlobalCluster {
	b.mu.RLock("DescribeGlobalClusters")
	defer b.mu.RUnlock()
	if id != "" {
		gc, exists := b.globalClusters.Get(id)
		if !exists {
			return []GlobalCluster{}
		}

		return []GlobalCluster{*copyGlobalCluster(gc)}
	}
	globalClusters := b.globalClusters.All()
	result := make([]GlobalCluster, 0, len(globalClusters))
	for _, gc := range globalClusters {
		result = append(result, *copyGlobalCluster(gc))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].GlobalClusterIdentifier < result[j].GlobalClusterIdentifier
	})

	return result
}

// ModifyGlobalCluster modifies a global cluster.
func (b *InMemoryBackend) ModifyGlobalCluster(
	_ context.Context,
	id, newID string,
	deletionProtection *bool,
) (*GlobalCluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: GlobalClusterIdentifier is required", ErrInvalidParameter)
	}
	b.mu.Lock("ModifyGlobalCluster")
	defer b.mu.Unlock()
	gc, exists := b.globalClusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: global cluster %s not found", ErrGlobalClusterNotFound, id)
	}
	if deletionProtection != nil {
		gc.DeletionProtection = *deletionProtection
	}
	if newID != "" && newID != id {
		b.globalClusters.Delete(id)
		gc.GlobalClusterIdentifier = newID
		gc.GlobalClusterArn = b.globalClusterARN(newID)
		b.globalClusters.Put(gc)
	}

	return copyGlobalCluster(gc), nil
}

// promoteGlobalClusterWriter flips IsWriter on gc.GlobalClusterMembers so
// that targetDBClusterID becomes the sole writer, backing both
// FailoverGlobalCluster and SwitchoverGlobalCluster's real member-promotion
// behavior (previously both were disguised no-ops -- see PARITY.md). When
// targetDBClusterID is empty, this is a no-op (nothing to promote). When it
// isn't yet a tracked member, this backend has no separate "join global
// cluster" operation to have attached it beforehand (real DocDB clusters
// join via CreateDBCluster-time GlobalClusterIdentifier attachment, which
// this backend does not model, matching the already-completed neptune
// service's precedent), so a target that resolves to a real DB cluster in
// this account is attached as the new writer -- the closest real analogue
// this backend can express. A target this backend cannot resolve at all
// (neither an existing member, an ARN, nor a known cluster identifier) is
// left as a no-op rather than an error: real AWS would reject it, but this
// backend has no way to distinguish "a genuine but not-yet-modeled
// cross-region secondary" from "a typo" without a join operation.
func (b *InMemoryBackend) promoteGlobalClusterWriter(region string, gc *GlobalCluster, targetDBClusterID string) {
	if targetDBClusterID == "" {
		return
	}
	targetARN, targetExists := b.resolveClusterARN(region, targetDBClusterID)
	found := false
	for i := range gc.GlobalClusterMembers {
		gc.GlobalClusterMembers[i].IsWriter = gc.GlobalClusterMembers[i].DBClusterArn == targetARN
		found = found || gc.GlobalClusterMembers[i].IsWriter
	}
	if found || !targetExists {
		return
	}
	for i := range gc.GlobalClusterMembers {
		gc.GlobalClusterMembers[i].IsWriter = false
	}
	gc.GlobalClusterMembers = append(gc.GlobalClusterMembers, GlobalClusterMember{
		DBClusterArn:          targetARN,
		IsWriter:              true,
		SynchronizationStatus: "synced",
	})
}

// FailoverGlobalCluster initiates a failover for a global cluster, promoting
// targetDBClusterID to the new writer -- see promoteGlobalClusterWriter.
func (b *InMemoryBackend) FailoverGlobalCluster(
	ctx context.Context, id, targetDBClusterID string,
) (*GlobalCluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: GlobalClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("FailoverGlobalCluster")
	defer b.mu.Unlock()
	gc, exists := b.globalClusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: global cluster %s not found", ErrGlobalClusterNotFound, id)
	}
	gc.Status = "failing-over"
	b.promoteGlobalClusterWriter(region, gc, targetDBClusterID)

	return copyGlobalCluster(gc), nil
}

// RemoveFromGlobalCluster removes a DB cluster from a global cluster,
// deleting the member entry whose ARN matches dbClusterID (accepted as
// either an ARN or a bare identifier, resolved the same way as Failover/
// Switchover's target).
func (b *InMemoryBackend) RemoveFromGlobalCluster(
	ctx context.Context,
	globalClusterID, dbClusterID string,
) (*GlobalCluster, error) {
	if globalClusterID == "" {
		return nil, fmt.Errorf("%w: GlobalClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("RemoveFromGlobalCluster")
	defer b.mu.Unlock()
	gc, exists := b.globalClusters.Get(globalClusterID)
	if !exists {
		return nil, fmt.Errorf("%w: global cluster %s not found", ErrGlobalClusterNotFound, globalClusterID)
	}
	targetARN, _ := b.resolveClusterARN(region, dbClusterID)
	kept := make([]GlobalClusterMember, 0, len(gc.GlobalClusterMembers))
	for _, m := range gc.GlobalClusterMembers {
		if m.DBClusterArn != targetARN {
			kept = append(kept, m)
		}
	}
	gc.GlobalClusterMembers = kept

	return copyGlobalCluster(gc), nil
}

// SwitchoverGlobalCluster initiates a switchover for a global cluster,
// promoting targetDBClusterID the same way FailoverGlobalCluster does -- the
// two AWS operations differ in data-loss guarantees (switchover is graceful,
// failover is not), a distinction this synchronous in-memory backend has no
// failure window to model, so both perform the same real member promotion.
func (b *InMemoryBackend) SwitchoverGlobalCluster(
	ctx context.Context, id, targetDBClusterID string,
) (*GlobalCluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: GlobalClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("SwitchoverGlobalCluster")
	defer b.mu.Unlock()
	gc, exists := b.globalClusters.Get(id)
	if !exists {
		return nil, fmt.Errorf("%w: global cluster %s not found", ErrGlobalClusterNotFound, id)
	}
	gc.Status = "switching-over"
	b.promoteGlobalClusterWriter(region, gc, targetDBClusterID)

	return copyGlobalCluster(gc), nil
}
