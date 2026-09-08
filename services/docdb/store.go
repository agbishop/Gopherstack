package docdb

import (
	"context"
	"maps"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
// DocDB resources are isolated per region: every backend operation resolves the
// caller's region from the request context and operates only on that region's
// nested store.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

// regionFromARN extracts the region component (index 3) from an AWS ARN
// (arn:partition:service:region:account:resource), falling back to defaultRegion.
func regionFromARN(resourceARN, defaultRegion string) string {
	parts := strings.Split(resourceARN, ":")
	const regionIndex = 3
	if len(parts) > regionIndex && parts[regionIndex] != "" {
		return parts[regionIndex]
	}

	return defaultRegion
}

func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:                  store.NewRegistry(),
		tags:                      make(map[string]map[string][]Tag),
		eventsLog:                 make(map[string][]Event),
		pendingMaintenanceActions: make(map[string]map[string]PendingMaintenanceAction),
		accountID:                 accountID,
		region:                    region,
		mu:                        lockmetrics.New("docdb"),
	}
	registerAllTables(b)

	return b
}

// Region returns the backend's configured default AWS region.
func (b *InMemoryBackend) Region() string { return b.region }

// regionKey builds the composite store.Table primary key ("region|id") shared
// by every region-qualified table registered in store_setup.go.
func regionKey(region, id string) string { return region + "|" + id }

// The following Get/Has/Put/Delete/InRegion helpers replace the old lazy
// per-region map accessors (clustersStore(region) etc.) with store.Table /
// store.Index operations. Callers must still hold b.mu, exactly as before --
// store.Table performs no locking of its own (see pkgs/store's package doc).

func (b *InMemoryBackend) clusterGet(region, id string) (*DBCluster, bool) {
	return b.clusters.Get(regionKey(region, id))
}

func (b *InMemoryBackend) clusterHas(region, id string) bool {
	return b.clusters.Has(regionKey(region, id))
}

func (b *InMemoryBackend) clusterPut(v *DBCluster) { b.clusters.Put(v) }

func (b *InMemoryBackend) clusterDelete(region, id string) { b.clusters.Delete(regionKey(region, id)) }

func (b *InMemoryBackend) clustersInRegion(region string) []*DBCluster {
	return b.clustersByRegion.Get(region)
}

func (b *InMemoryBackend) instanceGet(region, id string) (*DBInstance, bool) {
	return b.instances.Get(regionKey(region, id))
}

func (b *InMemoryBackend) instanceHas(region, id string) bool {
	return b.instances.Has(regionKey(region, id))
}

func (b *InMemoryBackend) instancePut(v *DBInstance) { b.instances.Put(v) }

func (b *InMemoryBackend) instanceDelete(region, id string) {
	b.instances.Delete(regionKey(region, id))
}

func (b *InMemoryBackend) instancesInRegion(region string) []*DBInstance {
	return b.instancesByRegion.Get(region)
}

func (b *InMemoryBackend) subnetGroupGet(region, name string) (*DBSubnetGroup, bool) {
	return b.subnetGroups.Get(regionKey(region, name))
}

func (b *InMemoryBackend) subnetGroupHas(region, name string) bool {
	return b.subnetGroups.Has(regionKey(region, name))
}

func (b *InMemoryBackend) subnetGroupPut(v *DBSubnetGroup) { b.subnetGroups.Put(v) }

func (b *InMemoryBackend) subnetGroupDelete(region, name string) {
	b.subnetGroups.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) subnetGroupsInRegion(region string) []*DBSubnetGroup {
	return b.subnetGroupsByRegion.Get(region)
}

func (b *InMemoryBackend) clusterParameterGroupGet(
	region, name string,
) (*DBClusterParameterGroup, bool) {
	return b.clusterParameterGroups.Get(regionKey(region, name))
}

func (b *InMemoryBackend) clusterParameterGroupHas(region, name string) bool {
	return b.clusterParameterGroups.Has(regionKey(region, name))
}

func (b *InMemoryBackend) clusterParameterGroupPut(v *DBClusterParameterGroup) {
	b.clusterParameterGroups.Put(v)
}

func (b *InMemoryBackend) clusterParameterGroupDelete(region, name string) {
	b.clusterParameterGroups.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) clusterParameterGroupsInRegion(region string) []*DBClusterParameterGroup {
	return b.clusterParameterGroupsByRegion.Get(region)
}

func (b *InMemoryBackend) clusterSnapshotGet(region, id string) (*DBClusterSnapshot, bool) {
	return b.clusterSnapshots.Get(regionKey(region, id))
}

func (b *InMemoryBackend) clusterSnapshotHas(region, id string) bool {
	return b.clusterSnapshots.Has(regionKey(region, id))
}

func (b *InMemoryBackend) clusterSnapshotPut(v *DBClusterSnapshot) { b.clusterSnapshots.Put(v) }

func (b *InMemoryBackend) clusterSnapshotDelete(region, id string) {
	b.clusterSnapshots.Delete(regionKey(region, id))
}

func (b *InMemoryBackend) clusterSnapshotsInRegion(region string) []*DBClusterSnapshot {
	return b.clusterSnapshotsByRegion.Get(region)
}

func (b *InMemoryBackend) eventSubscriptionGet(region, name string) (*EventSubscription, bool) {
	return b.eventSubscriptions.Get(regionKey(region, name))
}

func (b *InMemoryBackend) eventSubscriptionHas(region, name string) bool {
	return b.eventSubscriptions.Has(regionKey(region, name))
}

func (b *InMemoryBackend) eventSubscriptionPut(v *EventSubscription) { b.eventSubscriptions.Put(v) }

func (b *InMemoryBackend) eventSubscriptionDelete(region, name string) {
	b.eventSubscriptions.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) eventSubscriptionsInRegion(region string) []*EventSubscription {
	return b.eventSubscriptionsByRegion.Get(region)
}

// snapshotAttributesGet/Has/Put/Delete have no InRegion counterpart:
// snapshotAttributes carries no byRegion index (see the InMemoryBackend doc
// comment and store_setup.go).

func (b *InMemoryBackend) snapshotAttributesGet(
	region, id string,
) (*DBClusterSnapshotAttributesResult, bool) {
	return b.snapshotAttributes.Get(regionKey(region, id))
}

func (b *InMemoryBackend) snapshotAttributesPut(v *DBClusterSnapshotAttributesResult) {
	b.snapshotAttributes.Put(v)
}

func (b *InMemoryBackend) snapshotAttributesDelete(region, id string) {
	b.snapshotAttributes.Delete(regionKey(region, id))
}

// The following lazy per-region store helper returns the tags map for the
// given region, creating it on first use. Callers must hold b.mu. tags
// remains a raw map -- see the InMemoryBackend doc comment for why.

func (b *InMemoryBackend) tagsStore(region string) map[string][]Tag {
	if b.tags[region] == nil {
		b.tags[region] = make(map[string][]Tag)
	}

	return b.tags[region]
}

// tagsStoreRO returns the region-scoped tags map for region without mutating
// the outer map. Safe to call while holding only b.mu.RLock(): if the region
// has not been observed yet, it returns a fresh, unregistered, empty map
// instead of lazily creating (and persisting) an entry.
func (b *InMemoryBackend) tagsStoreRO(region string) map[string][]Tag {
	if v := b.tags[region]; v != nil {
		return v
	}

	return map[string][]Tag{}
}

// Reset clears all stored state, returning the backend to an empty state.
//
// It calls b.registry.ResetAll() rather than re-registering tables:
// registerAllTables must run exactly once, at construction (store.Register
// panics on a duplicate name) -- see the doc comment on registerAllTables in
// store_setup.go.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.tags = make(map[string]map[string][]Tag)
	b.eventsLog = make(map[string][]Event)
	b.pendingMaintenanceActions = make(map[string]map[string]PendingMaintenanceAction)
}

// clusterARN returns the ARN for a DB cluster in the given region.
func (b *InMemoryBackend) clusterARN(region, id string) string {
	return arn.Build("rds", region, b.accountID, "cluster:"+id)
}

// instanceARN returns the ARN for a DB instance in the given region.
func (b *InMemoryBackend) instanceARN(region, id string) string {
	return arn.Build("rds", region, b.accountID, "db:"+id)
}

// subnetGroupARN returns the ARN for a DB subnet group in the given region.
func (b *InMemoryBackend) subnetGroupARN(region, name string) string {
	return arn.Build("rds", region, b.accountID, "subgrp:"+name)
}

// clusterParameterGroupARN returns the ARN for a DB cluster parameter group in the given region.
func (b *InMemoryBackend) clusterParameterGroupARN(region, name string) string {
	return arn.Build("rds", region, b.accountID, "cluster-pg:"+name)
}

// clusterSnapshotARN returns the ARN for a DB cluster snapshot in the given region.
func (b *InMemoryBackend) clusterSnapshotARN(region, id string) string {
	return arn.Build("rds", region, b.accountID, "cluster-snapshot:"+id)
}

// globalClusterARN returns the ARN for a global cluster.
func (b *InMemoryBackend) globalClusterARN(id string) string {
	return arn.Build("rds", b.region, b.accountID, "global-cluster:"+id)
}

// eventSubscriptionARN returns the ARN for an event subscription in the
// given region, matching the "es:" resource-type prefix RDS-family event
// subscription ARNs use.
func (b *InMemoryBackend) eventSubscriptionARN(region, name string) string {
	return arn.Build("rds", region, b.accountID, "es:"+name)
}

// AddDBClusterInternal seeds a cluster directly for testing.
func (b *InMemoryBackend) AddDBClusterInternal(cluster *DBCluster) {
	b.mu.Lock("AddDBClusterInternal")
	defer b.mu.Unlock()
	cluster.region = b.region
	b.clusterPut(cluster)
}

// AddDBInstanceInternal seeds an instance directly for testing.
func (b *InMemoryBackend) AddDBInstanceInternal(inst *DBInstance) {
	b.mu.Lock("AddDBInstanceInternal")
	defer b.mu.Unlock()
	inst.region = b.region
	b.instancePut(inst)
}

// AddDBSubnetGroupInternal seeds a subnet group directly for testing.
func (b *InMemoryBackend) AddDBSubnetGroupInternal(sg *DBSubnetGroup) {
	b.mu.Lock("AddDBSubnetGroupInternal")
	defer b.mu.Unlock()
	sg.region = b.region
	b.subnetGroupPut(sg)
}

// AddDBClusterParameterGroupInternal seeds a parameter group directly for testing.
func (b *InMemoryBackend) AddDBClusterParameterGroupInternal(pg *DBClusterParameterGroup) {
	b.mu.Lock("AddDBClusterParameterGroupInternal")
	defer b.mu.Unlock()
	pg.region = b.region
	b.clusterParameterGroupPut(pg)
}

// AddDBClusterSnapshotInternal seeds a snapshot directly for testing.
func (b *InMemoryBackend) AddDBClusterSnapshotInternal(snap *DBClusterSnapshot) {
	b.mu.Lock("AddDBClusterSnapshotInternal")
	defer b.mu.Unlock()
	snap.region = b.region
	b.clusterSnapshotPut(snap)
}

// AddEventSubscriptionInternal seeds an event subscription directly for testing.
func (b *InMemoryBackend) AddEventSubscriptionInternal(sub *EventSubscription) {
	b.mu.Lock("AddEventSubscriptionInternal")
	defer b.mu.Unlock()
	sub.region = b.region
	b.eventSubscriptionPut(sub)
}

// AddGlobalClusterInternal seeds a global cluster directly for testing.
func (b *InMemoryBackend) AddGlobalClusterInternal(gc *GlobalCluster) {
	b.mu.Lock("AddGlobalClusterInternal")
	defer b.mu.Unlock()
	b.globalClusters.Put(gc)
}

// copyCluster returns a deep copy of a DBCluster.
func copyCluster(c *DBCluster) *DBCluster {
	cp := *c
	cp.Tags = copyTags(c.Tags)
	if len(c.AvailabilityZones) > 0 {
		cp.AvailabilityZones = make([]string, len(c.AvailabilityZones))
		copy(cp.AvailabilityZones, c.AvailabilityZones)
	}
	if len(c.VpcSecurityGroupIDs) > 0 {
		cp.VpcSecurityGroupIDs = make([]string, len(c.VpcSecurityGroupIDs))
		copy(cp.VpcSecurityGroupIDs, c.VpcSecurityGroupIDs)
	}
	if len(c.EnabledCloudwatchLogsExports) > 0 {
		cp.EnabledCloudwatchLogsExports = make([]string, len(c.EnabledCloudwatchLogsExports))
		copy(cp.EnabledCloudwatchLogsExports, c.EnabledCloudwatchLogsExports)
	}
	if len(c.ReadReplicaIdentifiers) > 0 {
		cp.ReadReplicaIdentifiers = make([]string, len(c.ReadReplicaIdentifiers))
		copy(cp.ReadReplicaIdentifiers, c.ReadReplicaIdentifiers)
	}

	return &cp
}

// copyInstance returns a deep copy of a DBInstance.
func copyInstance(inst *DBInstance) *DBInstance {
	cp := *inst
	cp.Tags = copyTags(inst.Tags)

	return &cp
}

// copyEventSubscription returns a deep copy of an EventSubscription.
func copyEventSubscription(sub *EventSubscription) *EventSubscription {
	cp := *sub
	cp.SourceIDs = make([]string, len(sub.SourceIDs))
	copy(cp.SourceIDs, sub.SourceIDs)
	cp.EventCategories = make([]string, len(sub.EventCategories))
	copy(cp.EventCategories, sub.EventCategories)

	return &cp
}

// copyGlobalCluster returns a deep copy of a GlobalCluster, including its
// GlobalClusterMembers slice (and each member's own Readers slice).
func copyGlobalCluster(gc *GlobalCluster) *GlobalCluster {
	cp := *gc
	cp.GlobalClusterMembers = make([]GlobalClusterMember, len(gc.GlobalClusterMembers))
	for i, m := range gc.GlobalClusterMembers {
		mc := m
		mc.Readers = make([]string, len(m.Readers))
		copy(mc.Readers, m.Readers)
		cp.GlobalClusterMembers[i] = mc
	}

	return &cp
}

// copyTags returns a deep copy of a string map (tags).
func copyTags(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)

	return dst
}

// tagsFromMap converts a map[string]string to []Tag.
func tagsFromMap(m map[string]string) []Tag {
	tags := make([]Tag, 0, len(m))
	for k, v := range m {
		tags = append(tags, Tag{Key: k, Value: v})
	}

	return tags
}

// firstAZ returns the first availability zone from a slice, or empty string.
func firstAZ(azs []string) string {
	if len(azs) == 0 {
		return ""
	}

	return azs[0]
}

// valueOrDefault returns s if non-empty, otherwise returns def.
func valueOrDefault(s, def string) string {
	if s == "" {
		return def
	}

	return s
}
