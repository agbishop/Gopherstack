package elasticache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	gopherDNS "github.com/blackbirdworks/gopherstack/pkgs/dns"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

func (b *InMemoryBackend) clusterARN(region, id string) string {
	return arn.Build("elasticache", region, b.accountID, "cluster:"+id)
}

// defaultEngineVersion returns the realistic default version for the given engine.
func defaultEngineVersion(engine string) string {
	switch engine {
	case engineMemcached:
		return "1.6.17"
	case engineValkey:
		return versionValkey82
	default:
		return versionRedis710
	}
}

// clusterEngine holds the started data-plane engine (embedded redis or a
// pass-through port) for a cluster, produced outside the backend lock.
type clusterEngine struct {
	mini          *miniredis.Miniredis
	connectAddr   string
	port          int
	allocatedPort int
}

// startClusterEngine starts the data-plane engine for a cluster WITHOUT holding
// b.mu. Listener startup (miniredis.StartAddr) is the slow part and must not
// serialise every other backend op behind it. b.engineMode/b.allocator are set
// once at construction and never mutated, so they are safe to read lock-free.
func (b *InMemoryBackend) startClusterEngine(port int) (*clusterEngine, error) {
	if b.engineMode != EngineEmbedded {
		p := port
		if p <= 0 {
			p = 6379
		}

		return &clusterEngine{port: p}, nil
	}

	mr := miniredis.NewMiniRedis()

	eng := &clusterEngine{mini: mr}
	if b.allocator != nil {
		allocatedPort, err := b.allocator.Acquire("elasticache")
		if err != nil {
			return nil, fmt.Errorf("allocate miniredis port: %w", err)
		}
		if startErr := mr.StartAddr(fmt.Sprintf("0.0.0.0:%d", allocatedPort)); startErr != nil {
			_ = b.allocator.Release(allocatedPort)

			return nil, fmt.Errorf("start miniredis on port %d: %w", allocatedPort, startErr)
		}
		eng.allocatedPort = allocatedPort
	} else if err := mr.Start(); err != nil {
		return nil, fmt.Errorf("start miniredis: %w", err)
	}

	addr := mr.Server().Addr()
	eng.port = addr.Port
	host := addr.IP.String()
	if addr.IP.IsUnspecified() {
		// 0.0.0.0 is a bind wildcard, not a connectable target; publish loopback.
		host = "127.0.0.1"
	}
	eng.connectAddr = net.JoinHostPort(host, strconv.Itoa(addr.Port))

	return eng, nil
}

// releaseEngine discards an engine that was started but never inserted (lost a
// create race). Safe to call with a nil or pass-through engine.
func (b *InMemoryBackend) releaseEngine(eng *clusterEngine) {
	if eng == nil {
		return
	}
	if eng.mini != nil {
		eng.mini.Close()
	}
	if b.allocator != nil && eng.allocatedPort > 0 {
		_ = b.allocator.Release(eng.allocatedPort)
	}
}

// insertClusterLocked builds and stores a cluster from an already-started
// engine. Must hold b.mu.
func (b *InMemoryBackend) insertClusterLocked(
	region, id, engine, nodeType, paramGroupName, maintenanceWindow, snapshotWindow string,
	numCacheNodes int,
	eng *clusterEngine,
) *Cluster {
	if engine == "" {
		engine = engineRedis
	}
	if nodeType == "" {
		nodeType = nodeTypeT3Micro
	}
	if numCacheNodes <= 0 {
		numCacheNodes = 1
	}

	c := &Cluster{
		ClusterID:                  id,
		Engine:                     engine,
		EngineVersion:              defaultEngineVersion(engine),
		Status:                     statusAvailable,
		NodeType:                   nodeType,
		NumCacheNodes:              numCacheNodes,
		ARN:                        b.clusterARN(region, id),
		Region:                     region,
		Tags:                       tags.New("elasticache.cluster." + id + ".tags"),
		CreatedAt:                  time.Now(),
		CacheParameterGroupName:    paramGroupName,
		PreferredMaintenanceWindow: maintenanceWindow,
		SnapshotWindow:             snapshotWindow,
		mini:                       eng.mini,
		Port:                       eng.port,
		AllocatedPort:              eng.allocatedPort,
		ConnectAddress:             eng.connectAddr,
	}
	b.markCreatingLocked(&c.PendingStatus, &c.AvailableAt)

	c.Endpoint = gopherDNS.SyntheticHostname(id, randomSuffix(), region, "cache")
	b.registerClusterDNSLocked(c)

	b.clustersStore(region).Put(c)
	b.appendEventLocked(id, "cache-cluster", "cluster created")

	return c
}

// registerClusterDNSLocked registers the cluster's synthetic endpoint hostname
// and, when the registrar supports it, binds an A record to the real bound IP so
// the published endpoint is genuinely reachable. Must hold b.mu.
func (b *InMemoryBackend) registerClusterDNSLocked(c *Cluster) {
	if b.dnsRegistrar == nil || c.Endpoint == "" {
		return
	}
	b.dnsRegistrar.Register(c.Endpoint)

	if c.ConnectAddress == "" {
		return
	}
	if host, _, err := net.SplitHostPort(c.ConnectAddress); err == nil && host != "" {
		if rr, ok := b.dnsRegistrar.(dnsRecordRegistrar); ok {
			rr.RegisterRecord(c.Endpoint, "A", []string{host})
		}
	}
}

// CreateCluster creates a new cache cluster.
func (b *InMemoryBackend) CreateCluster(ctx context.Context, id, engine, nodeType string, port int) (*Cluster, error) {
	region := getRegion(ctx, b.region)

	var exists bool
	func() {
		b.mu.Lock("CreateCluster.reserve")
		defer b.mu.Unlock()
		b.pruneRegionLocked(region)
		_, exists = b.clustersStore(region).Get(id)
	}()
	if exists {
		return nil, ErrClusterAlreadyExists
	}

	eng, err := b.startClusterEngine(port)
	if err != nil {
		return nil, err
	}

	b.mu.Lock("CreateCluster.insert")
	defer b.mu.Unlock()
	if _, dup := b.clustersStore(region).Get(id); dup {
		b.releaseEngine(eng)

		return nil, ErrClusterAlreadyExists
	}

	c := b.insertClusterLocked(region, id, engine, nodeType, "", "", "", 1, eng)

	return b.clusterView(c), nil
}

// CreateClusterWithOptions creates a new cache cluster with optional parameter group and scheduling windows.
func (b *InMemoryBackend) CreateClusterWithOptions(
	ctx context.Context,
	id, engine, nodeType, paramGroupName, maintenanceWindow, snapshotWindow string,
	numCacheNodes, port int,
) (*Cluster, error) {
	region := getRegion(ctx, b.region)

	var reserveErr error
	func() {
		b.mu.Lock("CreateClusterWithOptions.reserve")
		defer b.mu.Unlock()
		b.pruneRegionLocked(region)
		if _, exists := b.clustersStore(region).Get(id); exists {
			reserveErr = ErrClusterAlreadyExists

			return
		}
		if paramGroupName != "" {
			pg, ok := b.parameterGroupsStore(region).Get(paramGroupName)
			if !ok {
				reserveErr = ErrParameterGroupNotFound

				return
			}
			if err := validateParamGroupFamily(engine, pg.Family); err != nil {
				reserveErr = err

				return
			}
		}
	}()
	if reserveErr != nil {
		return nil, reserveErr
	}

	eng, err := b.startClusterEngine(port)
	if err != nil {
		return nil, err
	}

	b.mu.Lock("CreateClusterWithOptions.insert")
	defer b.mu.Unlock()
	if _, exists := b.clustersStore(region).Get(id); exists {
		b.releaseEngine(eng)

		return nil, ErrClusterAlreadyExists
	}

	c := b.insertClusterLocked(
		region, id, engine, nodeType, paramGroupName, maintenanceWindow, snapshotWindow, numCacheNodes, eng,
	)

	return b.clusterView(c), nil
}

// DeleteCluster stops and removes a cluster.
func (b *InMemoryBackend) DeleteCluster(ctx context.Context, id string) error {
	b.mu.Lock("DeleteCluster")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	b.pruneRegionLocked(region)
	tbl := b.clustersStore(region)
	c, exists := tbl.Get(id)
	if !exists || isReaped(b.now(), c.PendingStatus, c.AvailableAt) {
		return ErrClusterNotFound
	}
	if err := b.requireAvailableLocked(c.Status, c.PendingStatus, c.AvailableAt, ErrClusterNotAvailable); err != nil {
		return err
	}

	// AWS refuses DeleteCacheCluster for a cluster that is the last read replica
	// of a replication group; use DeleteReplicationGroup instead.
	if c.ReplicationGroupID != "" && b.isLastRGMemberLocked(region, c.ReplicationGroupID, id) {
		return ErrClusterInReplicationGroup
	}

	// With a lifecycle delay, dwell in "deleting" so waiters can observe it; the
	// engine stays live and the entry is reaped by the next write op once the
	// deadline passes. Without a delay (default), delete synchronously.
	if d := b.pendingUntil(); !d.IsZero() {
		c.PendingStatus = statusDeleting
		c.AvailableAt = d
		b.appendEventLocked(id, "cache-cluster", "cluster deleting")

		return nil
	}

	b.releaseClusterLocked(c)
	tbl.Delete(id)
	b.appendEventLocked(id, "cache-cluster", "cluster deleted")

	return nil
}

// isLastRGMemberLocked reports whether clusterID is the only remaining cluster
// carrying replicationGroupID in region. Caller must hold b.mu.
func (b *InMemoryBackend) isLastRGMemberLocked(region, replicationGroupID, clusterID string) bool {
	for _, other := range b.clustersStore(region).All() {
		if other.ClusterID != clusterID && other.ReplicationGroupID == replicationGroupID {
			return false
		}
	}

	return true
}

// SetClusterSubnetGroupName records the cache subnet group a cluster was
// created with. Kept separate from CreateClusterWithOptions to avoid
// widening that method's already-long positional signature.
func (b *InMemoryBackend) SetClusterSubnetGroupName(ctx context.Context, id, subnetGroupName string) error {
	if subnetGroupName == "" {
		return nil
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("SetClusterSubnetGroupName")
	defer b.mu.Unlock()

	c, exists := b.clustersStore(region).Get(id)
	if !exists {
		return ErrClusterNotFound
	}

	c.SubnetGroupName = subnetGroupName

	return nil
}

// SetClusterSnapshotRetentionLimit records how many days of automatic
// snapshots a cluster retains. limit is a pointer so a caller that never sent
// SnapshotRetentionLimit (nil) can be distinguished from one that explicitly
// sent 0 -- AWS documents 0 as "backups are turned off" (api_op_
// ModifyCacheCluster.go), not "leave unchanged". Kept separate from
// CreateClusterWithOptions/ModifyCluster to avoid widening their already-long
// positional signatures, same rationale as SetClusterSubnetGroupName.
func (b *InMemoryBackend) SetClusterSnapshotRetentionLimit(ctx context.Context, id string, limit *int) error {
	if limit == nil {
		return nil
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("SetClusterSnapshotRetentionLimit")
	defer b.mu.Unlock()

	c, exists := b.clustersStore(region).Get(id)
	if !exists {
		return ErrClusterNotFound
	}

	c.SnapshotRetentionLimit = *limit

	return nil
}

// SetClusterReplicationGroupID attaches an existing cluster to an existing
// replication group as a read replica, wiring CreateCacheCluster's
// ReplicationGroupId parameter (api_op_CreateCacheCluster.go: "the cluster is
// added to the specified replication group as a read replica"). Kept
// separate from CreateClusterWithOptions for the same reason as
// SetClusterSubnetGroupName.
func (b *InMemoryBackend) SetClusterReplicationGroupID(ctx context.Context, id, replicationGroupID string) error {
	if replicationGroupID == "" {
		return nil
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("SetClusterReplicationGroupID")
	defer b.mu.Unlock()

	if _, exists := b.replicationGroupsStore(region).Get(replicationGroupID); !exists {
		return ErrReplicationGroupNotFound
	}

	c, exists := b.clustersStore(region).Get(id)
	if !exists {
		return ErrClusterNotFound
	}

	c.ReplicationGroupID = replicationGroupID

	return nil
}

// DescribeClusters returns one cluster by id, or a paginated list of all clusters when id is empty.
// When notInRG is true, only clusters with no ReplicationGroupID are returned (standalone clusters).
func (b *InMemoryBackend) DescribeClusters(
	ctx context.Context,
	id, marker string,
	maxRecords int,
	notInRG bool,
) (page.Page[Cluster], error) {
	b.mu.RLock("DescribeClusters")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	var filter func(Cluster) bool
	if notInRG {
		filter = func(c Cluster) bool { return c.ReplicationGroupID == "" }
	}

	p, err := describePaged(b.clustersStoreRO(region), id, ErrClusterNotFound, filter,
		func(c Cluster) string { return c.ClusterID }, marker, maxRecords)

	return b.finalizeClusterPage(id, p, err)
}

// randomSuffix generates a short random hex string for synthetic hostnames.
func randomSuffix() string {
	b := make([]byte, randomSuffixLen)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// ListAll returns all clusters across all regions (used by dashboard).
func (b *InMemoryBackend) ListAll() []Cluster {
	b.mu.RLock("ListAll")
	defer b.mu.RUnlock()
	now := b.now()
	var out []Cluster
	for _, regionClusters := range b.clusters {
		for _, c := range regionClusters.All() {
			if isReaped(now, c.PendingStatus, c.AvailableAt) {
				continue
			}
			cp := *c
			cp.Status = overlayStatus(now, c.Status, c.PendingStatus, c.AvailableAt)
			out = append(out, cp)
		}
	}

	return out
}

// ModifyCluster modifies an existing cache cluster.
func (b *InMemoryBackend) ModifyCluster(
	ctx context.Context,
	id, nodeType, paramGroupName, engineVersion, maintenanceWindow, snapshotWindow string,
	numCacheNodes int,
) (*Cluster, error) {
	b.mu.Lock("ModifyCluster")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	c, exists := b.clustersStore(region).Get(id)
	if !exists {
		return nil, ErrClusterNotFound
	}
	if err := b.requireAvailableLocked(c.Status, c.PendingStatus, c.AvailableAt, ErrClusterNotAvailable); err != nil {
		return nil, err
	}

	if nodeType != "" {
		c.NodeType = nodeType
	}

	if paramGroupName != "" {
		if _, ok := b.parameterGroupsStore(region).Get(paramGroupName); !ok {
			return nil, ErrParameterGroupNotFound
		}
		c.CacheParameterGroupName = paramGroupName
	}

	if engineVersion != "" {
		c.EngineVersion = engineVersion
	}

	if numCacheNodes > 0 {
		c.NumCacheNodes = numCacheNodes
	}

	if maintenanceWindow != "" {
		c.PreferredMaintenanceWindow = maintenanceWindow
	}

	if snapshotWindow != "" {
		c.SnapshotWindow = snapshotWindow
	}

	b.markTransitionLocked(&c.PendingStatus, &c.AvailableAt, statusModifying)
	b.appendEventLocked(id, "cache-cluster", "cluster modified")

	return b.clusterView(c), nil
}

// RebootCacheCluster reboots a cache cluster.
func (b *InMemoryBackend) RebootCacheCluster(
	ctx context.Context,
	clusterID string,
	nodeIDs []string,
) (*Cluster, error) {
	b.mu.Lock("RebootCacheCluster")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	c, ok := b.clustersStore(region).Get(clusterID)
	if !ok {
		return nil, ErrClusterNotFound
	}
	if err := b.requireAvailableLocked(c.Status, c.PendingStatus, c.AvailableAt, ErrClusterNotAvailable); err != nil {
		return nil, err
	}

	// Record an event for each rebooted node (or one general event if no node IDs provided).
	if len(nodeIDs) == 0 {
		b.appendEventLocked(clusterID, "cache-cluster", "cache cluster reboot started")
	} else {
		for _, nodeID := range nodeIDs {
			b.appendEventLocked(clusterID, "cache-cluster", "cache node "+nodeID+" reboot started")
		}
	}

	b.markTransitionLocked(&c.PendingStatus, &c.AvailableAt, statusRebooting)

	return b.clusterView(c), nil
}

// ListAllowedNodeTypeModifications returns a list of allowed node type modifications.
func (b *InMemoryBackend) ListAllowedNodeTypeModifications(_ context.Context, _, _ string) ([]string, error) {
	return []string{
		nodeTypeT3Micro, "cache.t3.small", "cache.t3.medium",
		"cache.m6g.large", "cache.m6g.xlarge",
		"cache.r6g.large", "cache.r6g.xlarge", "cache.r6g.2xlarge",
	}, nil
}
