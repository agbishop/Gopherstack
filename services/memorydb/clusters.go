package memorydb

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// validateMaintenanceWindow validates the AWS MemoryDB maintenance window format ddd:hh24:mi-ddd:hh24:mi.
func validateMaintenanceWindow(w string) error {
	if w == "" {
		return nil
	}
	parts := strings.SplitN(w, "-", splitParts)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("MaintenanceWindow must be in format ddd:hh24:mi-ddd:hh24:mi: %w", ErrValidation)
	}

	return nil
}

// validateSnapshotWindow validates the AWS MemoryDB snapshot window format hh24:mi-hh24:mi.
func validateSnapshotWindow(w string) error {
	if w == "" {
		return nil
	}
	parts := strings.SplitN(w, "-", splitParts)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("SnapshotWindow must be in format hh24:mi-hh24:mi: %w", ErrValidation)
	}

	return nil
}

// clusterDefaults holds resolved default values for a new cluster.
type clusterDefaults struct {
	engine        string
	engineVersion string
	nodeType      string
	port          int32
	numShards     int32
	numReplicas   int32
	tlsEnabled    bool
}

// isSupportedEngineVersion reports whether v is a supported MemoryDB engine version.
func isSupportedEngineVersion(v string) bool {
	switch v {
	case engineVersion62, engineVersion70, engineVersion71, engineVersion72, engineVersion80:
		return true
	default:
		return false
	}
}

// validateCreateClusterRefs checks that ACL, subnet group, and parameter group referenced in
// req all exist in the backend (caller must hold b.mu).
func (b *InMemoryBackend) validateCreateClusterRefs(region string, req *createClusterRequest) (string, error) {
	aclName := req.ACLName
	if aclName == "" {
		aclName = openAccessACL
	}

	if _, ok := b.aclsStore(region).Get(aclName); !ok {
		return "", fmt.Errorf("ACL %q not found: %w", aclName, ErrACLNotFound)
	}

	if req.SubnetGroupName != "" {
		if _, ok := b.subnetGroupsStore(region).Get(req.SubnetGroupName); !ok {
			return "", fmt.Errorf("subnet group %q not found: %w", req.SubnetGroupName, ErrSubnetGroupNotFound)
		}
	}

	if req.ParameterGroupName != "" {
		if _, ok := b.parameterGroupsStore(region).Get(req.ParameterGroupName); !ok {
			return "", fmt.Errorf("parameter group %q not found: %w", req.ParameterGroupName, ErrParameterGroupNotFound)
		}
	}

	if req.MultiRegionClusterName != "" {
		if _, ok := b.multiRegionClusters.Get(req.MultiRegionClusterName); !ok {
			return "", fmt.Errorf(
				"multi-region cluster %q not found: %w", req.MultiRegionClusterName, ErrMultiRegionClusterNotFound,
			)
		}
	}

	return aclName, nil
}

// resolveClusterDefaults fills in default values for optional cluster fields.
func resolveClusterDefaults(req *createClusterRequest) (clusterDefaults, error) {
	d := clusterDefaults{
		engine:        req.Engine,
		engineVersion: req.EngineVersion,
		nodeType:      req.NodeType,
		port:          defaultPort,
		numShards:     1,
		numReplicas:   1,
		tlsEnabled:    true,
	}

	if err := resolveEngineDefaults(&d); err != nil {
		return d, err
	}

	if err := resolveNodeDefaults(&d); err != nil {
		return d, err
	}

	applyOptionalOverrides(&d, req)

	if err := validateClusterBounds(&d); err != nil {
		return d, err
	}

	return d, nil
}

// resolveEngineDefaults sets engine and version defaults, returning an error for unsupported values.
func resolveEngineDefaults(d *clusterDefaults) error {
	if d.engine == "" {
		d.engine = engineRedis
	}

	if d.engine != engineRedis && d.engine != engineValkey {
		return fmt.Errorf("engine %q is not supported (must be redis or valkey): %w", d.engine, ErrValidation)
	}

	if d.engineVersion == "" {
		if d.engine == engineValkey {
			d.engineVersion = defaultValkeyEngineVersion
		} else {
			d.engineVersion = defaultEngineVersion
		}
	}

	if !isSupportedEngineVersion(d.engineVersion) {
		return fmt.Errorf("engine version %q is not supported: %w", d.engineVersion, ErrValidation)
	}

	return nil
}

// resolveNodeDefaults sets the node type default and validates the prefix.
func resolveNodeDefaults(d *clusterDefaults) error {
	if d.nodeType == "" {
		d.nodeType = defaultNodeType
	}

	if !strings.HasPrefix(d.nodeType, "db.") {
		return fmt.Errorf("node type %q is invalid: must begin with 'db.': %w", d.nodeType, ErrValidation)
	}

	return nil
}

// applyOptionalOverrides applies caller-provided overrides for port, shards, replicas, and TLS.
func applyOptionalOverrides(d *clusterDefaults, req *createClusterRequest) {
	if req.Port != nil {
		d.port = *req.Port
	}

	if req.NumShards != nil {
		d.numShards = *req.NumShards
	}

	if req.NumReplicasPerShard != nil {
		d.numReplicas = *req.NumReplicasPerShard
	}

	if req.TLSEnabled != nil {
		d.tlsEnabled = *req.TLSEnabled
	}
}

// validateClusterBounds checks that shard and replica counts are within allowed ranges.
func validateClusterBounds(d *clusterDefaults) error {
	if d.numShards < 1 || d.numShards > 500 {
		return fmt.Errorf("NumShards must be between 1 and 500: %w", ErrValidation)
	}

	if d.numReplicas < 0 || d.numReplicas > 5 {
		return fmt.Errorf("NumReplicasPerShard must be between 0 and 5: %w", ErrValidation)
	}

	return nil
}

// applySnapshotRestoreConfig overrides cluster defaults from a source snapshot config.
func applySnapshotRestoreConfig(d *clusterDefaults, snap *Snapshot) {
	if snap.ClusterConfiguration.EngineVersion != "" {
		d.engineVersion = snap.ClusterConfiguration.EngineVersion
	}

	if snap.ClusterConfiguration.NodeType != "" {
		d.nodeType = snap.ClusterConfiguration.NodeType
	}

	if snap.ClusterConfiguration.NumShards > 0 {
		d.numShards = snap.ClusterConfiguration.NumShards
	}

	if snap.ClusterConfiguration.Port > 0 {
		d.port = snap.ClusterConfiguration.Port
	}
}

// seedAutomatedSnapshotLocked creates an automated snapshot for a new cluster if retention is configured.
// Caller must hold b.mu.
func (b *InMemoryBackend) seedAutomatedSnapshotLocked(region, accountID string, c *Cluster) {
	autoName := "automatic." + c.Name + "-" + time.Now().UTC().Format("20060102150405")
	autoARN := arn.Build("memorydb", region, accountID, "snapshot/"+autoName)
	autoSnap := &Snapshot{
		Name:                 autoName,
		ARN:                  autoARN,
		ClusterName:          c.Name,
		Status:               snapshotStatusAvailable,
		Source:               snapshotSourceAutomated,
		DataTiering:          c.DataTiering,
		Tags:                 make(map[string]string),
		CreatedAt:            time.Now(),
		ClusterConfiguration: b.snapshotClusterConfigFor(c),
	}
	b.snapshotsStore(region).Put(autoSnap)
	b.arnToResourceStore(region)[autoARN] = resourceRef{Kind: resourceKindSnapshot, Name: autoName}
}

// resolveDataTiering converts the optional DataTiering request field to the AWS string value.
func resolveDataTiering(req *createClusterRequest) string {
	if req.DataTiering != nil && *req.DataTiering {
		return "true"
	}

	return "false"
}

// applyClusterNetworkDefaults sets NetworkType and IPDiscovery defaults.
func applyClusterNetworkDefaults(c *Cluster, req *createClusterRequest) {
	c.NetworkType = req.NetworkType
	if c.NetworkType == "" {
		c.NetworkType = networkTypeIPv4
	}
	c.IPDiscovery = req.IPDiscovery
	if c.IPDiscovery == "" {
		c.IPDiscovery = networkTypeIPv4
	}
}

// CreateCluster creates a new MemoryDB cluster.
func buildCluster(region, clusterARN, aclName string, req *createClusterRequest, d clusterDefaults) *Cluster {
	c := &Cluster{
		Name:                    req.ClusterName,
		ARN:                     clusterARN,
		Description:             req.Description,
		NodeType:                d.nodeType,
		EngineVersion:           d.engineVersion,
		Engine:                  d.engine,
		ACLName:                 aclName,
		SubnetGroupName:         req.SubnetGroupName,
		ParameterGroupName:      req.ParameterGroupName,
		KmsKeyID:                req.KmsKeyID,
		SnsTopicArn:             req.SnsTopicArn,
		MaintenanceWindow:       req.MaintenanceWindow,
		SnapshotWindow:          req.SnapshotWindow,
		NumShards:               d.numShards,
		NumReplicasPerShard:     d.numReplicas,
		Port:                    d.port,
		TLSEnabled:              d.tlsEnabled,
		Status:                  clusterStatusAvailable,
		Tags:                    tagsFromSlice(req.Tags),
		CreatedAt:               time.Now(),
		Region:                  region,
		SecurityGroupIDs:        req.SecurityGroupIDs,
		AutoMinorVersionUpgrade: req.AutoMinorVersionUpgrade == nil || *req.AutoMinorVersionUpgrade,
		MultiRegionClusterName:  req.MultiRegionClusterName,
	}
	c.DataTiering = resolveDataTiering(req)
	applyClusterNetworkDefaults(c, req)
	if req.SnapshotRetentionLimit != nil {
		c.SnapshotRetentionLimit = *req.SnapshotRetentionLimit
	}
	if d.numReplicas > 0 {
		c.AvailabilityMode = "MultiAZ"
	} else {
		c.AvailabilityMode = "SingleAZ"
	}
	c.Endpoint = req.ClusterName + ".memorydb." + region + ".amazonaws.com"

	return c
}

func (b *InMemoryBackend) CreateCluster(ctx context.Context, req *createClusterRequest) (*Cluster, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	if err := validateResourceName(req.ClusterName, "cluster"); err != nil {
		return nil, err
	}

	if _, exists := b.clustersStore(region).Get(req.ClusterName); exists {
		return nil, ErrClusterAlreadyExists
	}

	aclName, err := b.validateCreateClusterRefs(region, req)
	if err != nil {
		return nil, err
	}

	var restoreSnap *Snapshot
	if req.SnapshotName != "" {
		s, ok := b.snapshotsStore(region).Get(req.SnapshotName)
		if !ok {
			// LANDMINE (gopherstack-me2v): CreateCluster's declared error set
			// (deserializers.go deserializeOpErrorCreateCluster) has no
			// SnapshotNotFoundFault -- unlike ACLName/SubnetGroupName/
			// ParameterGroupName above, which all have their own declared
			// NotFound faults. Do not "fix" this by guessing a replacement;
			// candidates are InvalidParameterValueException (declared here)
			// or leaving SnapshotNotFoundFault as-is if real AWS in fact
			// returns it undeclared. Needs verified evidence either way.
			return nil, fmt.Errorf("snapshot %q not found: %w", req.SnapshotName, ErrSnapshotNotFound)
		}
		restoreSnap = s
	}

	d, err := resolveClusterDefaults(req)
	if err != nil {
		return nil, err
	}

	if restoreSnap != nil {
		applySnapshotRestoreConfig(&d, restoreSnap)
	}

	if errMW := validateMaintenanceWindow(req.MaintenanceWindow); errMW != nil {
		return nil, errMW
	}
	if errSW := validateSnapshotWindow(req.SnapshotWindow); errSW != nil {
		return nil, errSW
	}

	clusterARN := arn.Build("memorydb", region, b.accountID, "cluster/"+req.ClusterName)
	c := buildCluster(region, clusterARN, aclName, req, d)
	b.markCreatingLocked(c)

	b.clustersStore(region).Put(c)
	b.arnToResourceStore(region)[clusterARN] = resourceRef{Kind: resourceKindCluster, Name: req.ClusterName}

	b.appendEventLocked(region, &Event{
		Date:       time.Now(),
		SourceName: req.ClusterName,
		SourceType: resourceKindCluster,
		Message:    "Cluster " + req.ClusterName + " created",
	})

	if c.SnapshotRetentionLimit > 0 {
		b.seedAutomatedSnapshotLocked(region, b.accountID, c)
	}

	return b.clusterView(c), nil
}

// DescribeClusters returns clusters, optionally filtered by name.
func (b *InMemoryBackend) DescribeClusters(ctx context.Context, name string) ([]*Cluster, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.clusters[region]

	if name != "" {
		c, ok := tableGet(t, name)
		if !ok {
			return nil, ErrClusterNotFound
		}

		return []*Cluster{b.clusterView(c)}, nil
	}

	all := tableAll(t)
	result := make([]*Cluster, 0, len(all))
	for _, c := range all {
		result = append(result, b.clusterView(c))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// DeleteCluster removes a cluster.
func (b *InMemoryBackend) DeleteCluster(ctx context.Context, name string) (*Cluster, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	c, ok := b.clustersStore(region).Get(name)
	if !ok {
		return nil, ErrClusterNotFound
	}

	b.clustersStore(region).Delete(name)
	delete(b.arnToResourceStore(region), c.ARN)

	b.appendEventLocked(region, &Event{
		Date:       time.Now(),
		SourceName: name,
		SourceType: resourceKindCluster,
		Message:    "Cluster " + name + " deleted",
	})

	return b.clusterView(c), nil
}

// DeleteClusterWithSnapshot removes a cluster, first creating a snapshot with the given name.
func (b *InMemoryBackend) DeleteClusterWithSnapshot(
	ctx context.Context,
	clusterName, snapshotName string,
) (*Cluster, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	c, ok := b.clustersStore(region).Get(clusterName)
	if !ok {
		return nil, ErrClusterNotFound
	}

	if snapshotName != "" {
		snapshotARN := arn.Build("memorydb", region, b.accountID, "snapshot/"+snapshotName)
		s := &Snapshot{
			Name:                 snapshotName,
			ARN:                  snapshotARN,
			ClusterName:          clusterName,
			Status:               snapshotStatusAvailable,
			Source:               snapshotSourceManual,
			DataTiering:          c.DataTiering,
			Tags:                 make(map[string]string),
			CreatedAt:            time.Now(),
			ClusterConfiguration: b.snapshotClusterConfigFor(c),
		}
		b.snapshotsStore(region).Put(s)
		b.arnToResourceStore(region)[snapshotARN] = resourceRef{Kind: resourceKindSnapshot, Name: snapshotName}
	}

	b.clustersStore(region).Delete(clusterName)
	delete(b.arnToResourceStore(region), c.ARN)

	b.appendEventLocked(region, &Event{
		Date:       time.Now(),
		SourceName: clusterName,
		SourceType: resourceKindCluster,
		Message:    "Cluster " + clusterName + " deleted",
	})

	return b.clusterView(c), nil
}

// applyClusterStringUpdates applies non-nil string field updates from req to c.
func applyClusterStringUpdates(c *Cluster, req *updateClusterRequest) {
	if req.Description != "" {
		c.Description = req.Description
	}

	if req.ACLName != "" {
		c.ACLName = req.ACLName
	}

	if req.NodeType != "" {
		c.NodeType = req.NodeType
	}

	if req.EngineVersion != "" {
		c.EngineVersion = req.EngineVersion
	}

	if req.MaintenanceWindow != "" {
		c.MaintenanceWindow = req.MaintenanceWindow
	}

	if req.SnapshotWindow != "" {
		c.SnapshotWindow = req.SnapshotWindow
	}

	if req.SnsTopicArn != "" {
		c.SnsTopicArn = req.SnsTopicArn
	}

	if req.NetworkType != "" {
		c.NetworkType = req.NetworkType
	}

	if req.IPDiscovery != "" {
		c.IPDiscovery = req.IPDiscovery
	}

	if req.SnsTopicStatus != "" {
		c.SnsTopicStatus = req.SnsTopicStatus
	}

	if req.Engine != "" {
		c.Engine = req.Engine
	}

	if req.ParameterGroupName != "" {
		c.ParameterGroupName = req.ParameterGroupName
	}
}

// validateUpdateClusterRequest validates the update cluster request fields.
func validateUpdateClusterRequest(req *updateClusterRequest) error {
	if req.MaintenanceWindow != "" {
		if err := validateMaintenanceWindow(req.MaintenanceWindow); err != nil {
			return err
		}
	}

	if req.SnapshotWindow != "" {
		if err := validateSnapshotWindow(req.SnapshotWindow); err != nil {
			return err
		}
	}

	if req.ReplicaConfiguration != nil && req.ReplicaConfiguration.ReplicaCount != nil {
		rc := *req.ReplicaConfiguration.ReplicaCount
		if rc < 0 || rc > 5 {
			return fmt.Errorf("NumReplicasPerShard must be between 0 and 5: %w", ErrValidation)
		}
	}

	if req.ShardConfiguration != nil && req.ShardConfiguration.ShardCount != nil {
		sc := *req.ShardConfiguration.ShardCount
		if sc < 1 || sc > 500 {
			return fmt.Errorf("NumShards must be between 1 and 500: %w", ErrValidation)
		}
	}

	if req.SnsTopicStatus != "" {
		if err := validateSnsTopicStatus(req.SnsTopicStatus); err != nil {
			return err
		}
	}

	return nil
}

func validateSnsTopicStatus(s string) error {
	if s != snsTopicStatusActive && s != snsTopicStatusInactive {
		return fmt.Errorf("invalid SnsTopicStatus %q, must be %s or %s: %w",
			s, snsTopicStatusActive, snsTopicStatusInactive, ErrValidation)
	}

	return nil
}

// applyClusterUpdates applies validated optional fields from the update request to the cluster.
func applyClusterUpdates(c *Cluster, req *updateClusterRequest) {
	applyClusterStringUpdates(c, req)

	if req.SnapshotRetentionLimit != nil {
		c.SnapshotRetentionLimit = *req.SnapshotRetentionLimit
	}

	if req.ReplicaConfiguration != nil && req.ReplicaConfiguration.ReplicaCount != nil {
		c.NumReplicasPerShard = *req.ReplicaConfiguration.ReplicaCount
	}

	if req.ShardConfiguration != nil && req.ShardConfiguration.ShardCount != nil {
		c.NumShards = *req.ShardConfiguration.ShardCount
	}

	if req.AutoMinorVersionUpgrade != nil {
		c.AutoMinorVersionUpgrade = *req.AutoMinorVersionUpgrade
	}

	if req.SecurityGroupIDs != nil {
		c.SecurityGroupIDs = req.SecurityGroupIDs
	}
}

// UpdateCluster modifies an existing cluster.
func (b *InMemoryBackend) UpdateCluster(ctx context.Context, req *updateClusterRequest) (*Cluster, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	c, ok := b.clustersStore(region).Get(req.ClusterName)
	if !ok {
		return nil, ErrClusterNotFound
	}

	if err := validateUpdateClusterRequest(req); err != nil {
		return nil, err
	}

	if req.ACLName != "" {
		if _, aclOK := b.aclsStore(region).Get(req.ACLName); !aclOK {
			return nil, fmt.Errorf("ACL %q not found: %w", req.ACLName, ErrACLNotFound)
		}
	}

	if req.ParameterGroupName != "" {
		if _, pgOK := b.parameterGroupsStore(region).Get(req.ParameterGroupName); !pgOK {
			return nil, fmt.Errorf(
				"parameter group %q not found: %w", req.ParameterGroupName, ErrParameterGroupNotFound,
			)
		}
	}

	applyClusterUpdates(c, req)

	b.appendEventLocked(region, &Event{
		Date:       time.Now(),
		SourceName: req.ClusterName,
		SourceType: resourceKindCluster,
		Message:    "Cluster " + req.ClusterName + " modified",
	})

	return b.clusterView(c), nil
}

// -- ACL operations --------------------------------------------------------------

// FailoverShard simulates a shard failover for a cluster, returning the cluster state.
func (b *InMemoryBackend) FailoverShard(ctx context.Context, clusterName, shardName string) (*Cluster, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	c, ok := b.clustersStore(region).Get(clusterName)
	if !ok {
		return nil, ErrClusterNotFound
	}

	msg := "Failover initiated"
	if shardName != "" {
		msg = "Failover initiated for shard " + shardName
	}

	b.appendEventLocked(region, &Event{
		Date:       time.Now(),
		SourceName: clusterName,
		SourceType: resourceKindCluster,
		Message:    msg,
	})

	return b.clusterView(c), nil
}

// -- Node type update operations ------------------------------------------------

// allowedNodeTypes returns the set of node types available for upgrade/downgrade.
func allowedNodeTypes() []string {
	return []string{
		defaultNodeType,
		defaultReservedNodeType,
		"db.r6g.2xlarge",
		"db.r6g.4xlarge",
		"db.r6gd.xlarge",
		"db.t4g.small",
		"db.t4g.medium",
	}
}

// ListAllowedNodeTypeUpdates returns the set of node types a cluster can be updated to.
func (b *InMemoryBackend) ListAllowedNodeTypeUpdates(ctx context.Context, clusterName string) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	if _, ok := tableGet(b.clusters[region], clusterName); !ok {
		return nil, ErrClusterNotFound
	}

	return allowedNodeTypes(), nil
}

// BatchUpdateCluster validates serviceUpdateName against the known service
// updates (real AWS fault for an unknown name: ServiceUpdateNotFoundFault --
// confirmed in botocore's BatchUpdateCluster.errors), then marks it applied on
// every found cluster. It returns a map of name→cluster for all clusters that
// were found; unknown names are omitted, and the caller decides which names
// are processed vs unprocessed.
func (b *InMemoryBackend) BatchUpdateCluster(
	ctx context.Context, clusterNames []string, serviceUpdateName string,
) (map[string]*Cluster, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if serviceUpdateName != "" {
		if _, ok := b.serviceUpdates.Get(serviceUpdateName); !ok {
			return nil, ErrServiceUpdateNotFound
		}
	}

	region := getRegion(ctx, b.defaultRegion)

	result := make(map[string]*Cluster, len(clusterNames))
	for _, name := range clusterNames {
		if c, ok := tableGet(b.clusters[region], name); ok {
			if serviceUpdateName != "" {
				if c.AppliedServiceUpdates == nil {
					c.AppliedServiceUpdates = make(map[string]bool, 1)
				}
				c.AppliedServiceUpdates[serviceUpdateName] = true
			}
			result[name] = b.clusterView(c)
		}
	}

	return result, nil
}

// -- ReservedNode operations ----------------------------------------------------

// ListClusters returns all clusters for use by the dashboard.
func (b *InMemoryBackend) ListClusters() []*Cluster {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []*Cluster
	for _, t := range b.clusters {
		for _, c := range t.All() {
			result = append(result, b.clusterView(c))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	return result
}

// cloneCluster returns a shallow copy of the cluster with a separate tags map.
func cloneCluster(c *Cluster) *Cluster {
	if c == nil {
		return nil
	}

	cp := *c
	cp.Tags = maps.Clone(c.Tags)
	cp.SecurityGroupIDs = append([]string(nil), c.SecurityGroupIDs...)

	return &cp
}

// AddClusterInternal inserts a cluster directly into the backend for testing.
func (b *InMemoryBackend) AddClusterInternal(name, nodeType string) *Cluster {
	b.mu.Lock()
	defer b.mu.Unlock()

	clusterARN := arn.Build("memorydb", b.defaultRegion, b.accountID, "cluster/"+name)
	c := &Cluster{
		Name:      name,
		ARN:       clusterARN,
		NodeType:  nodeType,
		Status:    clusterStatusAvailable,
		ACLName:   openAccessACL,
		Tags:      make(map[string]string),
		CreatedAt: time.Now(),
		Region:    b.defaultRegion,
	}
	b.clustersStore(b.defaultRegion).Put(c)
	b.arnToResourceStore(b.defaultRegion)[clusterARN] = resourceRef{Kind: resourceKindCluster, Name: name}

	return c
}
