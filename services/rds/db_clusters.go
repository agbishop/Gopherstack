package rds

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"time"
)

// CreateDBCluster creates a new DB cluster.
func (b *InMemoryBackend) CreateDBCluster(
	id, engine, masterUser, dbName, paramGroupName string,
	port int,
	serverlessV2Cfg *ServerlessV2ScalingConfiguration,
	opts DBClusterOptions,
) (*DBCluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	if err := validateDBClusterEngine(engine); err != nil {
		return nil, err
	}
	if err := ValidateStorageTypeForCluster(opts.StorageType); err != nil {
		return nil, err
	}
	if err := ValidateEngineLifecycleSupport(opts.EngineLifecycleSupport); err != nil {
		return nil, err
	}
	b.mu.Lock("CreateDBCluster")
	defer b.mu.Unlock()
	if _, exists := b.clusters.Get(normalizeID(id)); exists {
		return nil, fmt.Errorf("%w: cluster %s already exists", ErrClusterAlreadyExists, id)
	}

	var replicationSource *DBCluster
	if opts.ReplicationSourceIdentifier != "" {
		var srcExists bool
		replicationSource, srcExists = b.clusters.Get(normalizeID(opts.ReplicationSourceIdentifier))
		if !srcExists {
			return nil, fmt.Errorf(
				"%w: source cluster %s not found", ErrClusterNotFound, opts.ReplicationSourceIdentifier,
			)
		}
	}
	if engine == "" {
		engine = "aurora-postgresql"
	}
	if paramGroupName == "" {
		paramGroupName = "default." + engine
	}
	if port <= 0 {
		port = enginePort(engine)
	}
	endpoint := fmt.Sprintf("%s.cluster.%s.%s.rds.amazonaws.com", id, b.accountID, b.region)
	readerEndpoint := fmt.Sprintf("%s.cluster-ro.%s.%s.rds.amazonaws.com", id, b.accountID, b.region)
	networkType := opts.NetworkType
	if networkType == "" {
		networkType = "IPV4"
	}
	cluster := &DBCluster{
		ClusterCreateTime:            time.Now().UTC(),
		DBClusterIdentifier:          id,
		DBClusterArn:                 b.rdsARN("cluster", id),
		DBClusterResourceID:          "cluster-" + id,
		Engine:                       engine,
		EngineVersion:                opts.EngineVersion,
		Status:                       instanceStatusAvailable,
		MasterUsername:               masterUser,
		DatabaseName:                 dbName,
		DBClusterParameterGroupName:  paramGroupName,
		Endpoint:                     endpoint,
		ReaderEndpoint:               readerEndpoint,
		NetworkType:                  networkType,
		StorageType:                  opts.StorageType,
		EngineLifecycleSupport:       opts.EngineLifecycleSupport,
		OptimizedWrites:              opts.OptimizedWrites,
		Port:                         port,
		ServerlessV2ScalingConfig:    serverlessV2Cfg,
		KmsKeyID:                     opts.KmsKeyID,
		PreferredBackupWindow:        opts.PreferredBackupWindow,
		PreferredMaintenanceWindow:   opts.PreferredMaintenanceWindow,
		MonitoringRoleArn:            opts.MonitoringRoleArn,
		EnabledCloudwatchLogsExports: opts.EnabledCloudwatchLogsExports,
		AvailabilityZones:            opts.AvailabilityZones,
		BacktrackWindow:              opts.BacktrackWindow,
		BackupRetentionPeriod:        opts.BackupRetentionPeriod,
		MonitoringInterval:           opts.MonitoringInterval,
		MultiAZ:                      opts.MultiAZ,
		StorageEncrypted:             opts.StorageEncrypted,
		CopyTagsToSnapshot:           opts.CopyTagsToSnapshot,
		DeletionProtection:           opts.DeletionProtection,
		ReplicationSourceIdentifier:  opts.ReplicationSourceIdentifier,
		DBClusterMembers:             []DBClusterMember{},
	}
	b.clusters.Put(cluster)

	if replicationSource != nil {
		replicationSource.ReadReplicaIdentifiers = append(replicationSource.ReadReplicaIdentifiers, id)
	}

	cp := *cluster

	return &cp, nil
}

// DescribeDBClusters returns clusters. If id is non-empty, returns only that cluster.
func (b *InMemoryBackend) DescribeDBClusters(id string) ([]DBCluster, error) {
	b.mu.RLock("DescribeDBClusters")
	defer b.mu.RUnlock()
	if id != "" {
		cluster, exists := b.clusters.Get(normalizeID(id))
		if !exists {
			return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
		}
		cp := *cluster
		b.overlayFailoverStatusRLocked(&cp)

		return []DBCluster{cp}, nil
	}
	result := make([]DBCluster, 0, b.clusters.Len())
	for _, cluster := range b.clusters.All() {
		cp := *cluster
		b.overlayFailoverStatusRLocked(&cp)
		result = append(result, cp)
	}
	slices.SortFunc(result, func(a, b DBCluster) int {
		if a.DBClusterIdentifier < b.DBClusterIdentifier {
			return -1
		}
		if a.DBClusterIdentifier > b.DBClusterIdentifier {
			return 1
		}

		return 0
	})

	return result, nil
}

// isKnownDBClusterFilterName reports whether name is a Filters.Filter.N.Name
// value AWS recognizes for DescribeDBClusters. "domain" and "clone-group-id"
// are accepted (to avoid rejecting otherwise-valid client requests) but have
// no meaningful analog in this emulator (Directory Service domain membership,
// Aurora clone groups), so they are not implemented as match predicates.
func isKnownDBClusterFilterName(name string) bool {
	switch name {
	case "clone-group-id", filterNameDBClusterID, "db-cluster-resource-id", filterNameDomain, filterNameEngine:
		return true
	default:
		return false
	}
}

// applyDBClusterFilters narrows clusters per the AWS DescribeDBClusters
// Filters contract: each filter ANDs together, and a filter's Values list is
// OR-matched against the corresponding cluster field. An unrecognized filter
// name returns InvalidParameterValue, matching real AWS.
func applyDBClusterFilters(vals url.Values, clusters []DBCluster) ([]DBCluster, error) {
	filters := parseDescribeFilters(vals)
	if len(filters) == 0 {
		return clusters, nil
	}

	for name := range filters {
		if !isKnownDBClusterFilterName(name) {
			return nil, fmt.Errorf("%w: Unrecognized filter name: %s", ErrInvalidParameter, name)
		}
	}

	filtered := make([]DBCluster, 0, len(clusters))
	for _, c := range clusters {
		if matchesAllDBClusterFilters(c, filters) {
			filtered = append(filtered, c)
		}
	}

	return filtered, nil
}

func matchesAllDBClusterFilters(c DBCluster, filters map[string][]string) bool {
	for name, values := range filters {
		switch name {
		case filterNameDBClusterID:
			if !containsFoldIDOrARN(values, c.DBClusterIdentifier) {
				return false
			}
		case "db-cluster-resource-id":
			if !slices.Contains(values, c.DBClusterResourceID) {
				return false
			}
		case filterNameEngine:
			if !slices.Contains(values, c.Engine) {
				return false
			}
		case "clone-group-id", filterNameDomain:
			// Not modeled; accept unconditionally.
		}
	}

	return true
}

// DeleteDBCluster removes the given cluster.
// DeleteDBCluster removes the DB cluster with the given identifier, skipping
// the AWS final-snapshot contract (SkipFinalSnapshot=true). It exists for
// existing callers (e.g. CloudFormation resource cleanup) that pre-date the
// SkipFinalSnapshot/FinalDBSnapshotIdentifier parameters. New callers that
// need AWS-accurate DeleteDBCluster behavior should use
// DeleteDBClusterWithOptions.
func (b *InMemoryBackend) DeleteDBCluster(id string) (*DBCluster, error) {
	return b.DeleteDBClusterWithOptions(id, true, "")
}

// DeleteDBClusterWithOptions removes the DB cluster with the given identifier,
// honoring the AWS DeleteDBCluster parameter contract:
//   - SkipFinalSnapshot=false (the AWS default) requires a non-empty
//     finalSnapshotID; a manual cluster snapshot is taken before the cluster
//     is removed.
//   - SkipFinalSnapshot=true is mutually exclusive with a non-empty
//     finalSnapshotID (AWS: InvalidParameterCombination either way).
func (b *InMemoryBackend) DeleteDBClusterWithOptions(
	id string, skipFinalSnapshot bool, finalSnapshotID string,
) (*DBCluster, error) {
	b.mu.Lock("DeleteDBCluster")
	defer b.mu.Unlock()

	// Resolve the target cluster before validating the snapshot parameter
	// combination, matching AWS's behavior of returning DBClusterNotFoundFault
	// for a nonexistent cluster even when the snapshot params are also invalid.
	cluster, exists := b.clusters.Get(normalizeID(id))
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}

	if !skipFinalSnapshot && finalSnapshotID == "" {
		return nil, fmt.Errorf(
			"%w: FinalDBSnapshotIdentifier is required unless SkipFinalSnapshot is specified",
			ErrInvalidParameterCombination,
		)
	}
	if skipFinalSnapshot && finalSnapshotID != "" {
		return nil, fmt.Errorf(
			"%w: the FinalDBSnapshotIdentifier parameter cannot be specified when SkipFinalSnapshot is enabled",
			ErrInvalidParameterCombination,
		)
	}

	if !skipFinalSnapshot {
		if _, snapExists := b.clusterSnapshots.Get(normalizeID(finalSnapshotID)); snapExists {
			return nil, fmt.Errorf(
				"%w: cluster snapshot %s already exists",
				ErrClusterSnapshotAlreadyExists,
				finalSnapshotID,
			)
		}
		b.clusterSnapshots.Put(b.newManualClusterSnapshotLocked(finalSnapshotID, cluster))
	}

	cp := *cluster
	// Clear the cluster association on any member instances so they appear standalone.
	for _, member := range cluster.DBClusterMembers {
		if inst, ok := b.instances.Get(normalizeID(member.DBInstanceIdentifier)); ok {
			inst.DBClusterIdentifier = ""
		}
	}
	// canonicalID is the identifier as originally stored (its creation-time
	// casing), which may differ purely in case from the caller-supplied id
	// here since normalizeID folds both to the same table row. Auxiliary
	// maps (tags, FIS faults, roles, endpoints) are not case-normalized
	// themselves, so they must be keyed off the same casing used when they
	// were populated -- see normalizeID's doc comment.
	canonicalID := cluster.DBClusterIdentifier

	// Remove this cluster from its source's ReadReplicaIdentifiers.
	if cluster.ReplicationSourceIdentifier != "" {
		if src, srcExists := b.clusters.Get(normalizeID(cluster.ReplicationSourceIdentifier)); srcExists {
			src.ReadReplicaIdentifiers = slices.DeleteFunc(src.ReadReplicaIdentifiers, func(s string) bool {
				return idEqual(s, canonicalID)
			})
		}
	}

	b.clusters.Delete(normalizeID(id))
	delete(b.tags, b.rdsARN("cluster", canonicalID))
	delete(b.fisFailoverFaults, canonicalID)
	delete(b.clusterRoles, canonicalID)
	b.deleteClusterEndpointsLocked(canonicalID)

	return &cp, nil
}

// deleteClusterEndpointsLocked removes every custom DB cluster endpoint (and
// its tags) belonging to clusterID. Real RDS tears down a cluster's custom
// endpoints along with the cluster itself, since an endpoint has no
// independent existence apart from its parent cluster; leaving them behind
// is a ghost-row leak (DescribeDBClusterEndpoints would keep returning
// endpoints pointing at a deleted cluster, and the map grows unboundedly
// across create/delete cycles). Caller must already hold b.mu for writing.
// clusterID is compared case-insensitively since DBClusterIdentifier is a
// case-insensitive AWS identifier (see normalizeID).
func (b *InMemoryBackend) deleteClusterEndpointsLocked(clusterID string) {
	for _, ep := range b.clusterEndpoints.All() {
		if !idEqual(ep.DBClusterIdentifier, clusterID) {
			continue
		}
		b.clusterEndpoints.Delete(ep.DBClusterEndpointIdentifier)
		delete(b.tags, b.rdsARN("cluster-endpoint", ep.DBClusterEndpointIdentifier))
	}
}

// applyDBClusterOpts applies DBClusterOptions fields to a cluster in-place.
func applyDBClusterOpts(cluster *DBCluster, paramGroupName string, opts DBClusterOptions) {
	applyDBClusterStringOpts(cluster, paramGroupName, opts)
	applyDBClusterBoolOpts(cluster, opts)
}

// applyDBClusterStringOpts applies string and numeric fields from opts to cluster.
func applyDBClusterStringOpts(cluster *DBCluster, paramGroupName string, opts DBClusterOptions) {
	if paramGroupName != "" {
		cluster.DBClusterParameterGroupName = paramGroupName
	}
	if opts.EngineVersion != "" {
		cluster.EngineVersion = opts.EngineVersion
	}
	if opts.KmsKeyID != "" {
		cluster.KmsKeyID = opts.KmsKeyID
	}
	if opts.PreferredBackupWindow != "" {
		cluster.PreferredBackupWindow = opts.PreferredBackupWindow
	}
	if opts.PreferredMaintenanceWindow != "" {
		cluster.PreferredMaintenanceWindow = opts.PreferredMaintenanceWindow
	}
	if opts.MonitoringRoleArn != "" {
		cluster.MonitoringRoleArn = opts.MonitoringRoleArn
	}
	if opts.BacktrackWindow > 0 {
		cluster.BacktrackWindow = opts.BacktrackWindow
	}
	if opts.MonitoringInterval >= 0 {
		cluster.MonitoringInterval = opts.MonitoringInterval
	}
	if len(opts.EnabledCloudwatchLogsExports) > 0 {
		cluster.EnabledCloudwatchLogsExports = opts.EnabledCloudwatchLogsExports
	}
	if opts.StorageType != "" {
		cluster.StorageType = opts.StorageType
	}
	if opts.NetworkType != "" {
		cluster.NetworkType = opts.NetworkType
	}
	if opts.EngineLifecycleSupport != "" {
		cluster.EngineLifecycleSupport = opts.EngineLifecycleSupport
	}
}

// applyDBClusterBoolOpts applies boolean fields from opts to cluster.
func applyDBClusterBoolOpts(cluster *DBCluster, opts DBClusterOptions) {
	if opts.MultiAZ {
		cluster.MultiAZ = opts.MultiAZ
	}
	if opts.StorageEncryptedChanged {
		cluster.StorageEncrypted = opts.StorageEncrypted
	}
	if opts.CopyTagsToSnapshot {
		cluster.CopyTagsToSnapshot = opts.CopyTagsToSnapshot
	}
	if opts.DeletionProtectionSet {
		cluster.DeletionProtection = opts.DeletionProtection
	} else if opts.DeletionProtection {
		cluster.DeletionProtection = true
	}
	if opts.OptimizedWrites {
		cluster.OptimizedWrites = true
	}
}

// ModifyDBCluster modifies a DB cluster.
func (b *InMemoryBackend) ModifyDBCluster(id, paramGroupName string, opts DBClusterOptions) (*DBCluster, error) {
	b.mu.Lock("ModifyDBCluster")
	defer b.mu.Unlock()
	cluster, exists := b.clusters.Get(normalizeID(id))
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	applyDBClusterOpts(cluster, paramGroupName, opts)
	cp := *cluster

	return &cp, nil
}

// StartDBCluster starts a stopped DB cluster.
func (b *InMemoryBackend) StartDBCluster(id string) (*DBCluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	b.mu.Lock("StartDBCluster")
	defer b.mu.Unlock()
	cluster, exists := b.clusters.Get(normalizeID(id))
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	cluster.Status = instanceStatusAvailable
	cp := *cluster

	return &cp, nil
}

// StopDBCluster stops a running DB cluster.
func (b *InMemoryBackend) StopDBCluster(id string) (*DBCluster, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	b.mu.Lock("StopDBCluster")
	defer b.mu.Unlock()
	cluster, exists := b.clusters.Get(normalizeID(id))
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	cluster.Status = "stopped"
	cp := *cluster

	return &cp, nil
}

// RestoreDBClusterFromSnapshot creates a new DB cluster from the given snapshot.
func (b *InMemoryBackend) RestoreDBClusterFromSnapshot(clusterID, snapshotID, engine string) (*DBCluster, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotIdentifier must not be empty", ErrInvalidParameter)
	}
	b.mu.Lock("RestoreDBClusterFromSnapshot")
	defer b.mu.Unlock()
	if _, exists := b.clusters.Get(normalizeID(clusterID)); exists {
		return nil, fmt.Errorf("%w: cluster %s already exists", ErrClusterAlreadyExists, clusterID)
	}
	snap, exists := b.clusterSnapshots.Get(normalizeID(snapshotID))
	if !exists {
		return nil, fmt.Errorf("%w: cluster snapshot %s not found", ErrClusterSnapshotNotFound, snapshotID)
	}
	if engine == "" {
		engine = snap.Engine
	}
	endpoint := fmt.Sprintf("%s.cluster.%s.%s.rds.amazonaws.com", clusterID, b.accountID, b.region)
	cluster := &DBCluster{
		DBClusterIdentifier:         clusterID,
		DBClusterArn:                b.rdsARN("cluster", clusterID),
		Engine:                      engine,
		Status:                      instanceStatusAvailable,
		DBClusterParameterGroupName: "default." + engine,
		Endpoint:                    endpoint,
		Port:                        enginePort(engine),
	}
	b.clusters.Put(cluster)
	cp := *cluster

	return &cp, nil
}

// RestoreDBClusterToPointInTime creates a new DB cluster as a point-in-time restore of the source cluster.
func (b *InMemoryBackend) RestoreDBClusterToPointInTime(clusterID, sourceClusterID string) (*DBCluster, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	if sourceClusterID == "" {
		return nil, fmt.Errorf("%w: SourceDBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	b.mu.Lock("RestoreDBClusterToPointInTime")
	defer b.mu.Unlock()
	if _, exists := b.clusters.Get(normalizeID(clusterID)); exists {
		return nil, fmt.Errorf("%w: cluster %s already exists", ErrClusterAlreadyExists, clusterID)
	}
	source, exists := b.clusters.Get(normalizeID(sourceClusterID))
	if !exists {
		return nil, fmt.Errorf("%w: source cluster %s not found", ErrClusterNotFound, sourceClusterID)
	}
	endpoint := fmt.Sprintf("%s.cluster.%s.%s.rds.amazonaws.com", clusterID, b.accountID, b.region)
	cluster := &DBCluster{
		DBClusterIdentifier:         clusterID,
		DBClusterArn:                b.rdsARN("cluster", clusterID),
		Engine:                      source.Engine,
		Status:                      instanceStatusAvailable,
		MasterUsername:              source.MasterUsername,
		DatabaseName:                source.DatabaseName,
		DBClusterParameterGroupName: source.DBClusterParameterGroupName,
		Endpoint:                    endpoint,
		Port:                        source.Port,
	}
	b.clusters.Put(cluster)
	cp := *cluster

	return &cp, nil
}

// clusterRoleStatusActive is the static Status value this emulator reports
// for every DBClusterRole: the backend applies role associations
// synchronously, so the PENDING/INVALID states real AWS's DBClusterRole.Status
// can report (types.go:1522-1531) never apply here.
const clusterRoleStatusActive = "ACTIVE"

// AddRoleToDBCluster associates an IAM role with the given DB cluster for the
// given feature (e.g. S3_INTEGRATION). Unlike the instance-side FeatureName
// (required, fixed in gopherstack-i101), FeatureName is optional here
// (rds@v1.124.1 api_op_AddRoleToDBCluster.go:39-43), so real AWS's behavior
// when a client omits it on two different-role adds is unverified -- see
// upsertClusterRole for the documented placeholder.
func (b *InMemoryBackend) AddRoleToDBCluster(clusterID, roleARN, featureName string) error {
	if clusterID == "" {
		return fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	if roleARN == "" {
		return fmt.Errorf("%w: RoleArn must not be empty", ErrInvalidParameter)
	}

	b.mu.Lock("AddRoleToDBCluster")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	// Key b.clusterRoles off cluster.DBClusterIdentifier (the stored,
	// creation-time casing), not the raw clusterID argument -- see
	// normalizeID; clusterRoles is a plain map with no normalization of its
	// own.
	canonicalID := cluster.DBClusterIdentifier
	b.clusterRoles[canonicalID] = upsertClusterRole(b.clusterRoles[canonicalID], roleARN, featureName)

	return nil
}

// upsertClusterRole adds roleARN under featureName, replacing any existing
// association already using that featureName -- matching the "adding a
// different role for a feature already in use replaces it" semantics
// gopherstack-i101 established for the (required) instance-side FeatureName.
//
// featureName == "" means the client omitted the optional field. Real AWS's
// behavior for two omitted-FeatureName adds is unverified (gopherstack-1jkv),
// so this is a documented placeholder rather than a guess: within that
// bucket, matching is by RoleArn too, so two different roles both added
// without FeatureName coexist instead of the second silently discarding the
// first (the collapse this function exists to avoid). This preserves the
// pre-fix behavior for that specific bucket; only the previously-unkeyed
// same-ARN-different-FeatureName case actually changes.
func upsertClusterRole(roles []DBClusterRole, roleARN, featureName string) []DBClusterRole {
	for i, r := range roles {
		if r.FeatureName != featureName {
			continue
		}
		if featureName == "" && r.RoleArn != roleARN {
			continue
		}
		roles[i].RoleArn = roleARN
		roles[i].Status = clusterRoleStatusActive

		return roles
	}

	return append(roles, DBClusterRole{RoleArn: roleARN, FeatureName: featureName, Status: clusterRoleStatusActive})
}

// ClusterAssociatedRoles returns a copy of the IAM roles associated with the
// given cluster (AssociatedRoles on DescribeDBClusters' DBCluster,
// rds@v1.124.1 types.go:1511). Returns nil if the cluster does not exist.
func (b *InMemoryBackend) ClusterAssociatedRoles(clusterID string) []DBClusterRole {
	b.mu.RLock("ClusterAssociatedRoles")
	defer b.mu.RUnlock()

	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return nil
	}

	roles := b.clusterRoles[cluster.DBClusterIdentifier]
	cp := make([]DBClusterRole, len(roles))
	copy(cp, roles)

	return cp
}

// BacktrackDBCluster backtracks an Aurora DB cluster to a specific time.
func (b *InMemoryBackend) BacktrackDBCluster(
	clusterID, backtrackTo string,
) (*DBClusterBacktrack, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	if backtrackTo == "" {
		return nil, fmt.Errorf("%w: BacktrackTo must not be empty", ErrInvalidParameter)
	}

	b.mu.RLock("BacktrackDBCluster")
	defer b.mu.RUnlock()

	if _, exists := b.clusters.Get(normalizeID(clusterID)); !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	result := &DBClusterBacktrack{
		DBClusterIdentifier: clusterID,
		BacktrackIdentifier: newBacktrackID(),
		BacktrackTo:         backtrackTo,
		Status:              backtrackStatusApplying,
	}

	return result, nil
}

// RemoveRoleFromDBCluster disassociates an IAM role from the given cluster's
// feature slot. Returns an error if the cluster does not exist. Removing a
// role that is not associated, or whose ARN doesn't match what's currently
// associated with that FeatureName (including the omitted-FeatureName ""
// bucket -- see upsertClusterRole), is a no-op.
func (b *InMemoryBackend) RemoveRoleFromDBCluster(clusterID, roleARN, featureName string) error {
	if clusterID == "" {
		return fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	if roleARN == "" {
		return fmt.Errorf("%w: RoleArn must not be empty", ErrInvalidParameter)
	}

	b.mu.Lock("RemoveRoleFromDBCluster")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	canonicalID := cluster.DBClusterIdentifier
	roles := b.clusterRoles[canonicalID]
	idx := slices.IndexFunc(roles, func(r DBClusterRole) bool {
		return r.FeatureName == featureName && r.RoleArn == roleARN
	})
	if idx >= 0 {
		b.clusterRoles[canonicalID] = slices.Delete(roles, idx, idx+1)
	}

	return nil
}

const backtrackIDBytes = 8

// newBacktrackID generates a unique backtrack identifier using random bytes.
func newBacktrackID() string {
	buf := make([]byte, backtrackIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "backtrack-unknown"
	}

	return "backtrack-" + hex.EncodeToString(buf)
}

// ValidateStorageTypeForCluster returns an error if the storage type is not valid for
// an Aurora cluster.
func ValidateStorageTypeForCluster(storageType string) error {
	switch storageType {
	case "", storageTypeAurora, storageTypeAuroraIOOptimized:
		return nil
	default:
		return fmt.Errorf(
			"%w: StorageType for Aurora cluster must be %q or %q, got %q",
			ErrInvalidParameter,
			storageTypeAurora,
			storageTypeAuroraIOOptimized,
			storageType,
		)
	}
}

// FailoverDBCluster triggers a failover on an Aurora DB cluster, promoting an
// Aurora Replica to be the new cluster writer (rds@v1.124.1
// api_op_FailoverDBCluster.go:13-14). targetDBInstanceIdentifier, when given,
// must name an existing cluster member other than the current writer; when
// empty, the first non-writer member is promoted instead, mirroring AWS
// auto-selecting a replica.
func (b *InMemoryBackend) FailoverDBCluster(clusterID, targetDBInstanceIdentifier string) (*DBCluster, error) {
	b.mu.Lock("FailoverDBCluster")
	defer b.mu.Unlock()
	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}
	if cluster.Status != instanceStatusAvailable {
		return nil, fmt.Errorf("%w: cluster %s is not in available state", ErrInvalidDBClusterStateFault, clusterID)
	}

	var targetIdx int
	if targetDBInstanceIdentifier != "" {
		targetIdx = slices.IndexFunc(cluster.DBClusterMembers, func(m DBClusterMember) bool {
			return idEqual(m.DBInstanceIdentifier, targetDBInstanceIdentifier)
		})
		if targetIdx < 0 {
			return nil, fmt.Errorf(
				"%w: %s is not a member of cluster %s",
				ErrInvalidDBInstanceState, targetDBInstanceIdentifier, clusterID,
			)
		}
	} else {
		targetIdx = slices.IndexFunc(cluster.DBClusterMembers, func(m DBClusterMember) bool {
			return !m.IsClusterWriter
		})
	}

	cluster.Status = clusterStatusFailingOver
	b.publishClusterEventLocked(cluster.DBClusterIdentifier, "DB cluster failover started")
	if targetIdx >= 0 {
		for i := range cluster.DBClusterMembers {
			cluster.DBClusterMembers[i].IsClusterWriter = i == targetIdx
		}
	}
	cluster.Status = instanceStatusAvailable
	cp := *cluster

	return &cp, nil
}

// RebootDBCluster reboots the named Aurora DB cluster.
// The cluster transitions to "rebooting" status and reverts to "available" after a brief delay.
func (b *InMemoryBackend) RebootDBCluster(clusterID string) (*DBCluster, error) {
	var (
		result *DBCluster
		err    error
	)

	func() {
		b.mu.Lock("RebootDBCluster")
		defer b.mu.Unlock()

		cluster, exists := b.clusters.Get(normalizeID(clusterID))
		if !exists {
			err = fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)

			return
		}
		if cluster.Status != instanceStatusAvailable {
			err = fmt.Errorf("%w: cluster %s is not in available state", ErrInvalidDBClusterStateFault, clusterID)

			return
		}
		cluster.Status = "rebooting"
		b.clusterReadyAt[cluster.DBClusterIdentifier] = time.Now().Add(instanceTransitionDelay)
		b.scheduleReconcilerLocked()
		cp := *cluster
		result = &cp
	}()

	if err != nil {
		return nil, err
	}

	return result, nil
}

// publishClusterEventLocked publishes an event for a DB cluster.
// Must be called with the write lock held.
func (b *InMemoryBackend) publishClusterEventLocked(clusterID, msg string) {
	event := Event{
		Message:          msg,
		SourceIdentifier: clusterID,
		SourceType:       "db-cluster",
		SourceArn:        b.rdsARN("cluster", clusterID),
		CreatedAt:        time.Now(),
	}
	b.events = append(b.events, event)
	if len(b.events) > maxEvents {
		b.events = b.events[len(b.events)-maxEvents:]
	}
}

// PromoteReadReplicaDBCluster promotes a read replica DB cluster.
func (b *InMemoryBackend) PromoteReadReplicaDBCluster(clusterID string) (*DBCluster, error) {
	b.mu.Lock("PromoteReadReplicaDBCluster")
	defer b.mu.Unlock()
	cluster, ok := b.clusters.Get(normalizeID(clusterID))
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterID)
	}
	cluster.Status = instanceStatusAvailable
	cp := *cluster

	return &cp, nil
}

// DescribeDBClusterBacktracks returns backtracks for a DB cluster.
func (b *InMemoryBackend) DescribeDBClusterBacktracks(clusterID string) ([]DBClusterBacktrack, error) {
	b.mu.RLock("DescribeDBClusterBacktracks")
	defer b.mu.RUnlock()
	if _, ok := b.clusters.Get(normalizeID(clusterID)); !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterID)
	}

	return []DBClusterBacktrack{}, nil
}

// ModifyCurrentDBClusterCapacity modifies the serverless capacity of a DB cluster.
func (b *InMemoryBackend) ModifyCurrentDBClusterCapacity(clusterID string, capacity int) (*DBCluster, error) {
	b.mu.Lock("ModifyCurrentDBClusterCapacity")
	defer b.mu.Unlock()
	cluster, ok := b.clusters.Get(normalizeID(clusterID))
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterID)
	}
	cluster.ServerlessCapacity = capacity
	cp := *cluster

	return &cp, nil
}

// RestoreDBClusterFromS3 restores a DB cluster from an S3 backup.
// s3IngestionRoleArn, sourceEngine and sourceEngineVersion are required
// members of RestoreDBClusterFromS3Input (api_op_RestoreDBClusterFromS3.go:90,97,105,114)
// describing the source backup; the real DBCluster response shape has no
// fields for them (grepped types/types.go), so they're validated as
// required but not persisted -- there's no real state to echo them into.
func (b *InMemoryBackend) RestoreDBClusterFromS3(
	id, engine, masterUsername, s3Bucket, s3IngestionRoleArn, sourceEngine, sourceEngineVersion string,
) (*DBCluster, error) {
	if s3Bucket == "" {
		return nil, fmt.Errorf("%w: s3BucketName is required", ErrInvalidParameter)
	}
	if id == "" {
		return nil, fmt.Errorf("%w: dbClusterIdentifier is required", ErrInvalidParameter)
	}
	if engine == "" {
		return nil, fmt.Errorf("%w: engine is required", ErrInvalidParameter)
	}
	if masterUsername == "" {
		return nil, fmt.Errorf("%w: masterUsername is required", ErrInvalidParameter)
	}
	if s3IngestionRoleArn == "" {
		return nil, fmt.Errorf("%w: s3IngestionRoleArn is required", ErrInvalidParameter)
	}
	if sourceEngine == "" {
		return nil, fmt.Errorf("%w: sourceEngine is required", ErrInvalidParameter)
	}
	if sourceEngineVersion == "" {
		return nil, fmt.Errorf("%w: sourceEngineVersion is required", ErrInvalidParameter)
	}
	b.mu.Lock("RestoreDBClusterFromS3")
	defer b.mu.Unlock()
	if _, exists := b.clusters.Get(normalizeID(id)); exists {
		return nil, fmt.Errorf("%w: %s", ErrClusterAlreadyExists, id)
	}
	cluster := &DBCluster{
		DBClusterIdentifier: id,
		DBClusterArn:        b.rdsARN("cluster", id),
		Engine:              engine,
		MasterUsername:      masterUsername,
		Status:              "creating",
	}
	b.clusters.Put(cluster)
	cp := *cluster

	return &cp, nil
}

// IsClusterFailoverActive reports whether a FIS failover simulation is currently
// active for the cluster with the given identifier.
// Expired entries are lazily evicted to prevent unbounded map growth.
func (b *InMemoryBackend) IsClusterFailoverActive(clusterID string) bool {
	b.mu.Lock("IsClusterFailoverActive")
	defer b.mu.Unlock()

	exp, ok := b.fisFailoverFaults[clusterID]
	if !ok {
		return false
	}

	if !exp.IsZero() && time.Now().After(exp) {
		// Lazily evict expired entry.
		delete(b.fisFailoverFaults, clusterID)

		return false
	}

	return true
}

// clusterFailoverActiveRLocked reports whether a FIS failover fault is active
// for clusterID, without mutating b.fisFailoverFaults. Caller must hold at
// least b.mu.RLock(); an expired entry is left in place for the locked
// IsClusterFailoverActive path to evict.
func (b *InMemoryBackend) clusterFailoverActiveRLocked(clusterID string) bool {
	exp, ok := b.fisFailoverFaults[clusterID]
	if !ok {
		return false
	}

	return exp.IsZero() || !time.Now().After(exp)
}

// overlayFailoverStatusRLocked sets cluster.Status to "failing-over" when a
// FIS failover fault is active for it, so DescribeDBClusters observably
// reflects an in-progress FIS failover experiment. Caller must hold at least
// b.mu.RLock().
func (b *InMemoryBackend) overlayFailoverStatusRLocked(cluster *DBCluster) {
	if b.clusterFailoverActiveRLocked(cluster.DBClusterIdentifier) {
		cluster.Status = clusterStatusFailingOver
	}
}
