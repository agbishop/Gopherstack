package neptune

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

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

// The following lazy per-region store helpers return the resource map for the
// given region, creating it on first use. Callers must hold b.mu. clusterRoles
// and tags remain raw maps -- see the InMemoryBackend doc comment for why.

func (b *InMemoryBackend) clusterRolesStore(region string) map[string][]string {
	if b.clusterRoles[region] == nil {
		b.clusterRoles[region] = make(map[string][]string)
	}

	return b.clusterRoles[region]
}

// cloneCluster deep-copies a DBCluster to avoid shared slice/pointer mutation.
func cloneCluster(c *DBCluster) DBCluster {
	cp := *c
	cp.DBClusterMembers = make([]DBClusterMember, len(c.DBClusterMembers))
	copy(cp.DBClusterMembers, c.DBClusterMembers)
	cp.AssociatedRoles = make([]string, len(c.AssociatedRoles))
	copy(cp.AssociatedRoles, c.AssociatedRoles)
	cp.VpcSecurityGroupIDs = make([]string, len(c.VpcSecurityGroupIDs))
	copy(cp.VpcSecurityGroupIDs, c.VpcSecurityGroupIDs)
	cp.AvailabilityZones = make([]string, len(c.AvailabilityZones))
	copy(cp.AvailabilityZones, c.AvailabilityZones)
	if c.ServerlessV2ScalingConfig != nil {
		sv2 := *c.ServerlessV2ScalingConfig
		cp.ServerlessV2ScalingConfig = &sv2
	}
	if c.MasterUserManagedSecret != nil {
		ms := *c.MasterUserManagedSecret
		cp.MasterUserManagedSecret = &ms
	}

	return cp
}

// clusterARN returns the region-scoped ARN for a Neptune DB cluster.
func (b *InMemoryBackend) clusterARN(region, id string) string {
	return arn.Build("neptune", region, b.accountID, "cluster:"+id)
}

// clusterIdentifierFromARN extracts the DBClusterIdentifier from a Neptune
// cluster ARN built by clusterARN (arn:partition:neptune:region:account:cluster:id).
func clusterIdentifierFromARN(clusterARN string) string {
	const marker = "cluster:"
	if idx := strings.LastIndex(clusterARN, marker); idx != -1 {
		return clusterARN[idx+len(marker):]
	}

	return clusterARN
}

// clusterByARNLocked resolves a Neptune cluster ARN to its DBCluster, if
// tracked by this backend (global clusters reference members by ARN, which
// may live in a different region than the caller's own). Caller must hold
// b.mu.
func (b *InMemoryBackend) clusterByARNLocked(clusterARN, defaultRegion string) (*DBCluster, bool) {
	region := regionFromARN(clusterARN, defaultRegion)

	return b.clusterGet(region, clusterIdentifierFromARN(clusterARN))
}

// attachClusterToGlobalClusterLocked joins cl to the existing global cluster
// globalClusterID -- real Neptune clusters join a global cluster via
// CreateDBCluster's GlobalClusterIdentifier member (api_op_CreateDBCluster.go:
// 129), previously entirely unmodeled by this backend (CreateDBCluster never
// even parsed it, and DBCluster had no field to hold it). cl becomes the
// writer only if the global cluster has no members yet; a later join adds it
// as a secondary reader, mirroring the real distinction between a global
// cluster's original source member and members added afterward. Caller must
// hold b.mu (write lock).
func (b *InMemoryBackend) attachClusterToGlobalClusterLocked(
	region string, cl *DBCluster, globalClusterID string,
) error {
	gc, exists := b.globalClusters.Get(globalClusterID)
	if !exists {
		return fmt.Errorf("%w: global cluster %s not found", ErrGlobalClusterNotFound, globalClusterID)
	}

	cl.GlobalClusterIdentifier = globalClusterID
	gc.GlobalClusterMembers = append(gc.GlobalClusterMembers, GlobalClusterMember{
		DBClusterARN: b.clusterARN(region, cl.DBClusterIdentifier),
		IsWriter:     len(gc.GlobalClusterMembers) == 0,
	})

	return nil
}

// CreateDBCluster creates a new Neptune DB cluster.
func (b *InMemoryBackend) CreateDBCluster(
	ctx context.Context,
	id, paramGroupName string,
	port int,
	opts DBClusterCreateOptions,
) (*DBCluster, error) {
	backupRetention, err := validateCreateClusterParams(id, port, opts)
	if err != nil {
		return nil, err
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateDBCluster")
	defer b.mu.Unlock()
	if b.clusterHas(region, id) {
		return nil, fmt.Errorf("%w: cluster %s already exists", ErrClusterAlreadyExists, id)
	}
	cluster := b.buildNewCluster(region, id, paramGroupName, port, backupRetention, opts)
	if opts.GlobalClusterIdentifier != "" {
		if attachErr := b.attachClusterToGlobalClusterLocked(
			region,
			cluster,
			opts.GlobalClusterIdentifier,
		); attachErr != nil {
			return nil, attachErr
		}
	}
	b.clusterPut(cluster)
	b.recordEvent(region, id, sourceTypeDBCluster, "DB cluster created", "creation")
	cp := cloneCluster(cluster)

	return &cp, nil
}

// validateCreateClusterParams validates CreateDBCluster inputs and returns the
// effective backup retention period to use.
func validateCreateClusterParams(
	id string, port int, opts DBClusterCreateOptions,
) (int, error) {
	if err := validateNeptuneIdentifier(id, "DBClusterIdentifier"); err != nil {
		return 0, err
	}
	if port != 0 && (port < minNeptunePort || port > maxNeptunePort) {
		return 0, fmt.Errorf(
			"%w: Port %d is not a valid Neptune port; must be between %d and %d",
			ErrInvalidParameter, port, minNeptunePort, maxNeptunePort,
		)
	}
	backupRetention := defaultBackupRetentionPeriod
	if opts.BackupRetentionPeriod != 0 {
		backupRetention = opts.BackupRetentionPeriod
	}
	if backupRetention < minBackupRetentionPeriod || backupRetention > maxBackupRetentionPeriod {
		return 0, fmt.Errorf(
			"%w: BackupRetentionPeriod %d is not valid; must be between %d and %d",
			ErrInvalidParameter,
			backupRetention,
			minBackupRetentionPeriod,
			maxBackupRetentionPeriod,
		)
	}

	return backupRetention, nil
}

// buildNewCluster constructs a DBCluster from the create options, applying defaults.
func (b *InMemoryBackend) buildNewCluster(
	region, id, paramGroupName string,
	port, backupRetention int,
	opts DBClusterCreateOptions,
) *DBCluster {
	if paramGroupName == "" {
		paramGroupName = pgFamilyDefaultNeptune13
	}
	if port <= 0 {
		port = defaultNeptunePort
	}
	engineVersion := defaultEngineVersion
	if opts.EngineVersion != "" {
		engineVersion = opts.EngineVersion
	}
	engineMode := engineModeProvisioned
	if opts.EngineMode != "" {
		engineMode = opts.EngineMode
	}
	storageType := defaultStorageType
	if opts.StorageType != "" {
		storageType = opts.StorageType
	}
	// IPV4 is AWS's documented default when NetworkType is unspecified
	// (neptune@v1.48.4 api_op_CreateDBCluster.go:161: "IPV4 - (the default)").
	networkType := networkTypeIPv4
	if opts.NetworkType != "" {
		networkType = opts.NetworkType
	}
	endpoint := fmt.Sprintf("%s.cluster.%s.neptune.amazonaws.com", id, region)
	readerEndpoint := fmt.Sprintf(
		"%s.cluster-ro.%s.neptune.amazonaws.com",
		id,
		region,
	)
	hostedZoneID := fmt.Sprintf("Z%s", strings.ToUpper(region))
	vpcSGs := make([]string, len(opts.VpcSecurityGroupIDs))
	copy(vpcSGs, opts.VpcSecurityGroupIDs)
	azs := make([]string, len(opts.AvailabilityZones))
	copy(azs, opts.AvailabilityZones)
	cluster := &DBCluster{
		region:                          region,
		DBClusterIdentifier:             id,
		DBClusterArn:                    b.clusterARN(region, id),
		DBClusterResourceID:             fmt.Sprintf("cluster-%s", id),
		ClusterCreateTime:               nowISO8601(),
		Engine:                          neptuneEngine,
		EngineVersion:                   engineVersion,
		EngineMode:                      engineMode,
		Status:                          clusterStatusAvailable,
		DBClusterParameterGroupName:     paramGroupName,
		DBSubnetGroupName:               opts.DBSubnetGroupName,
		Endpoint:                        endpoint,
		ReaderEndpoint:                  readerEndpoint,
		Port:                            port,
		DBClusterMembers:                []DBClusterMember{},
		AssociatedRoles:                 []string{},
		VpcSecurityGroupIDs:             vpcSGs,
		AvailabilityZones:               azs,
		BackupRetentionPeriod:           backupRetention,
		AllocatedStorage:                defaultAllocatedStorage,
		StorageEncrypted:                opts.StorageEncrypted,
		EnableIAMDatabaseAuthentication: opts.EnableIAMDatabaseAuthentication,
		DeletionProtection:              opts.DeletionProtection,
		CopyTagsToSnapshot:              opts.CopyTagsToSnapshot,
		PreferredBackupWindow:           opts.PreferredBackupWindow,
		PreferredMaintenanceWindow:      opts.PreferredMaintenanceWindow,
		KmsKeyID:                        opts.KmsKeyID,
		ServerlessV2ScalingConfig:       opts.ServerlessV2ScalingConfig,
		MasterUsername:                  opts.MasterUsername,
		StorageType:                     storageType,
		HostedZoneID:                    hostedZoneID,
		NetworkType:                     networkType,
	}
	if opts.ManageMasterUserPassword {
		cluster.MasterUserManagedSecret = &MasterUserManagedSecret{
			SecretARN: fmt.Sprintf(
				"arn:aws:secretsmanager:%s:%s:secret:rds!cluster-%s",
				region,
				b.accountID,
				id,
			),
			SecretStatus: subscriptionStatusActive,
		}
	}

	return cluster
}

// DescribeDBClusters returns all Neptune DB clusters or a specific one.
// Filters (when set) restrict results to matching clusters.
func (b *InMemoryBackend) DescribeDBClusters(
	ctx context.Context,
	id string,
	filters DBClusterFilters,
) ([]DBCluster, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBClusters")
	defer b.mu.RUnlock()
	if id != "" {
		c, exists := b.clusterGet(region, id)
		if !exists {
			return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
		}

		return []DBCluster{cloneCluster(c)}, nil
	}
	clusters := b.clustersInRegion(region)
	result := make([]DBCluster, 0, len(clusters))
	for _, c := range clusters {
		if len(filters.Engine) > 0 && !slices.Contains(filters.Engine, c.Engine) {
			continue
		}
		if len(filters.EngineVersion) > 0 && !slices.Contains(filters.EngineVersion, c.EngineVersion) {
			continue
		}
		if len(filters.Status) > 0 && !slices.Contains(filters.Status, c.Status) {
			continue
		}
		result = append(result, cloneCluster(c))
	}
	slices.SortFunc(result, func(a, b DBCluster) int {
		return strings.Compare(a.DBClusterIdentifier, b.DBClusterIdentifier)
	})

	return result, nil
}

// DeleteDBCluster deletes a Neptune DB cluster and all associated DB instances.
func (b *InMemoryBackend) DeleteDBCluster(
	ctx context.Context,
	id string,
	opts DBClusterDeleteOptions,
) (*DBCluster, error) {
	region := getRegion(ctx, b.region)
	// Validate FinalDBSnapshotIdentifier before acquiring the lock.
	if !opts.SkipFinalSnapshot && opts.FinalDBSnapshotIdentifier == "" {
		return nil, fmt.Errorf(
			"%w: FinalDBSnapshotIdentifier is required when SkipFinalSnapshot is false",
			ErrSnapshotRequired,
		)
	}
	if !opts.SkipFinalSnapshot && opts.FinalDBSnapshotIdentifier != "" {
		if err := validateNeptuneIdentifier(
			opts.FinalDBSnapshotIdentifier,
			"FinalDBSnapshotIdentifier",
		); err != nil {
			return nil, err
		}
	}
	b.mu.Lock("DeleteDBCluster")
	defer b.mu.Unlock()
	c, exists := b.clusterGet(region, id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	if c.DeletionProtection {
		return nil, fmt.Errorf(
			"%w: cluster %s cannot be deleted because deletion protection is enabled",
			ErrInvalidDBClusterStateFault,
			id,
		)
	}
	cp := cloneCluster(c)
	// Create a final snapshot when requested.
	if !opts.SkipFinalSnapshot && opts.FinalDBSnapshotIdentifier != "" {
		if !b.clusterSnapshotHas(region, opts.FinalDBSnapshotIdentifier) {
			b.clusterSnapshotPut(&DBClusterSnapshot{
				region:                      region,
				DBClusterSnapshotIdentifier: opts.FinalDBSnapshotIdentifier,
				DBClusterSnapshotArn: b.clusterSnapshotARN(
					region,
					opts.FinalDBSnapshotIdentifier,
				),
				DBClusterIdentifier:              id,
				Engine:                           neptuneEngine,
				EngineVersion:                    c.EngineVersion,
				Status:                           snapshotStatusAvailable,
				StorageEncrypted:                 c.StorageEncrypted,
				KmsKeyID:                         c.KmsKeyID,
				IAMDatabaseAuthenticationEnabled: c.EnableIAMDatabaseAuthentication,
				Port:                             c.Port,
				PercentProgress:                  percentProgressComplete,
				AllocatedStorage:                 c.AllocatedStorage,
				SnapshotType:                     snapshotSourceManual,
				SnapshotCreateTime:               nowISO8601(),
				ClusterCreateTime:                c.ClusterCreateTime,
			})
		}
	}
	b.clusterDelete(region, id)
	delete(b.tagsStore(region), b.clusterARN(region, id))
	delete(b.clusterRolesStore(region), id)

	// Clean up all instances associated with this cluster. slices.Clone first:
	// instanceDelete mutates the byRegion index that instancesInRegion returns,
	// so iterating the live slice while deleting from it would be unsafe.
	tagStore := b.tagsStore(region)
	for _, inst := range slices.Clone(b.instancesInRegion(region)) {
		if inst.DBClusterIdentifier == id {
			b.instanceDelete(region, inst.DBInstanceIdentifier)
			delete(tagStore, b.instanceARN(region, inst.DBInstanceIdentifier))
		}
	}

	// Clean up all custom endpoints associated with this cluster (same
	// clone-before-delete rationale as above).
	for _, ep := range slices.Clone(b.clusterEndpointsInRegion(region)) {
		if ep.DBClusterIdentifier == id {
			b.clusterEndpointDelete(region, ep.DBClusterEndpointIdentifier)
		}
	}

	b.detachDeletedClusterFromGlobalCluster(region, id, c.GlobalClusterIdentifier)
	b.recordEvent(region, id, sourceTypeDBCluster, "DB cluster deleted", "deletion")

	return &cp, nil
}

// detachDeletedClusterFromGlobalCluster removes a just-deleted DB cluster
// from its global cluster's membership list, if it belonged to one. Per
// api_op_DeleteGlobalCluster.go, membership only blocks DeleteGlobalCluster
// while a member is "detached or deleted" -- without this, a member cluster
// deleted directly (not via RemoveFromGlobalCluster) would leave a ghost
// entry that blocks its global cluster from ever being deleted. Callers must
// hold the write lock.
func (b *InMemoryBackend) detachDeletedClusterFromGlobalCluster(region, id, globalClusterID string) {
	if globalClusterID == "" {
		return
	}
	gc, ok := b.globalClusters.Get(globalClusterID)
	if !ok {
		return
	}
	clusterARN := b.clusterARN(region, id)
	kept := make([]GlobalClusterMember, 0, len(gc.GlobalClusterMembers))
	for _, m := range gc.GlobalClusterMembers {
		if m.DBClusterARN != clusterARN {
			kept = append(kept, m)
		}
	}
	gc.GlobalClusterMembers = kept
}

// ModifyDBCluster modifies a Neptune DB cluster.
func (b *InMemoryBackend) ModifyDBCluster(
	ctx context.Context, id, paramGroupName string, opts DBClusterModifyOptions,
) (*DBCluster, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyDBCluster")
	defer b.mu.Unlock()
	c, exists := b.clusterGet(region, id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	if paramGroupName != "" {
		c.DBClusterParameterGroupName = paramGroupName
	}
	applyClusterScalarModifications(c, opts)
	if err := applyClusterBackupRetention(c, opts); err != nil {
		return nil, err
	}
	applyClusterSecurityGroups(c, opts)
	b.applyClusterMasterSecret(c, region, id, opts)
	cp := cloneCluster(c)

	return &cp, nil
}

// applyClusterScalarModifications applies the optional scalar fields of opts onto c.
func applyClusterScalarModifications(c *DBCluster, opts DBClusterModifyOptions) {
	if opts.EngineVersion != "" {
		c.EngineVersion = opts.EngineVersion
	}
	if opts.NetworkType != "" {
		c.NetworkType = opts.NetworkType
	}
	if opts.PreferredBackupWindow != "" {
		c.PreferredBackupWindow = opts.PreferredBackupWindow
	}
	if opts.PreferredMaintenanceWindow != "" {
		c.PreferredMaintenanceWindow = opts.PreferredMaintenanceWindow
	}
	if opts.IamAuthSet {
		c.EnableIAMDatabaseAuthentication = opts.EnableIAMDatabaseAuthentication
	}
	if opts.DeletionProtectionSet {
		c.DeletionProtection = opts.DeletionProtection
	}
	if opts.ServerlessV2ScalingConfig != nil {
		sv2 := *opts.ServerlessV2ScalingConfig
		c.ServerlessV2ScalingConfig = &sv2
	}
	if opts.CopyTagsToSnapshotSet {
		c.CopyTagsToSnapshot = opts.CopyTagsToSnapshot
	}
}

// applyClusterBackupRetention validates and applies the backup retention period.
func applyClusterBackupRetention(c *DBCluster, opts DBClusterModifyOptions) error {
	if !opts.BackupRetentionPeriodSet {
		return nil
	}
	if opts.BackupRetentionPeriod < minBackupRetentionPeriod ||
		opts.BackupRetentionPeriod > maxBackupRetentionPeriod {
		return fmt.Errorf(
			"%w: BackupRetentionPeriod %d is not valid; must be between %d and %d",
			ErrInvalidParameter,
			opts.BackupRetentionPeriod,
			minBackupRetentionPeriod,
			maxBackupRetentionPeriod,
		)
	}
	c.BackupRetentionPeriod = opts.BackupRetentionPeriod

	return nil
}

// applyClusterSecurityGroups replaces the cluster VPC security groups when provided.
func applyClusterSecurityGroups(c *DBCluster, opts DBClusterModifyOptions) {
	if len(opts.VpcSecurityGroupIDs) == 0 {
		return
	}
	vpcSGs := make([]string, len(opts.VpcSecurityGroupIDs))
	copy(vpcSGs, opts.VpcSecurityGroupIDs)
	c.VpcSecurityGroupIDs = vpcSGs
}

// applyClusterMasterSecret provisions a managed master-user secret when requested.
func (b *InMemoryBackend) applyClusterMasterSecret(
	c *DBCluster, region, id string, opts DBClusterModifyOptions,
) {
	if !opts.ManageMasterUserPassword || c.MasterUserManagedSecret != nil {
		return
	}
	c.MasterUserManagedSecret = &MasterUserManagedSecret{
		SecretARN: fmt.Sprintf(
			"arn:aws:secretsmanager:%s:%s:secret:rds!cluster-%s",
			region,
			b.accountID,
			id,
		),
		SecretStatus: subscriptionStatusActive,
	}
}

// StopDBCluster stops a Neptune DB cluster.
func (b *InMemoryBackend) StopDBCluster(ctx context.Context, id string) (*DBCluster, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("StopDBCluster")
	defer b.mu.Unlock()
	c, exists := b.clusterGet(region, id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	if c.Status == clusterStatusStopped {
		return nil, fmt.Errorf(
			"%w: cluster %s is already stopped",
			ErrInvalidDBClusterStateFault,
			id,
		)
	}
	c.Status = clusterStatusStopped
	b.recordEvent(region, id, sourceTypeDBCluster, "DB cluster stopped", sourceTypeNotification)
	cp := cloneCluster(c)

	return &cp, nil
}

// StartDBCluster starts a stopped Neptune DB cluster.
func (b *InMemoryBackend) StartDBCluster(ctx context.Context, id string) (*DBCluster, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("StartDBCluster")
	defer b.mu.Unlock()
	c, exists := b.clusterGet(region, id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	if c.Status != clusterStatusStopped {
		return nil, fmt.Errorf(
			"%w: cluster %s is not in stopped state",
			ErrInvalidDBClusterStateFault,
			id,
		)
	}
	c.Status = clusterStatusAvailable
	b.recordEvent(region, id, sourceTypeDBCluster, "DB cluster started", sourceTypeNotification)
	cp := cloneCluster(c)

	return &cp, nil
}

// FailoverDBCluster triggers a failover for a Neptune DB cluster.
func (b *InMemoryBackend) FailoverDBCluster(
	ctx context.Context, id, targetInstanceID string,
) (*DBCluster, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("FailoverDBCluster")
	defer b.mu.Unlock()
	c, exists := b.clusterGet(region, id)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, id)
	}
	if err := promoteClusterMember(c, targetInstanceID); err != nil {
		return nil, err
	}
	b.recordEvent(region, id, sourceTypeDBCluster, "DB cluster failover completed", "failover")
	cp := cloneCluster(c)

	return &cp, nil
}

// promoteClusterMember performs the state change of a failover: it promotes
// one non-writer member of c to writer and demotes the current writer,
// mirroring the DBClusterMembers.IsClusterWriter flip AWS performs. When
// targetInstanceID is empty, AWS (and this backend) picks any available
// reader; when set, it must name an existing member other than the current
// writer. A cluster with fewer than two members has no reader to fail over
// to, matching AWS's InvalidDBClusterStateFault in that situation.
func promoteClusterMember(c *DBCluster, targetInstanceID string) error {
	if len(c.DBClusterMembers) < minFailoverClusterMembers {
		return fmt.Errorf(
			"%w: cluster %s has no reader instance available to fail over to",
			ErrInvalidDBClusterStateFault,
			c.DBClusterIdentifier,
		)
	}
	targetIdx := -1
	for i, m := range c.DBClusterMembers {
		if targetInstanceID != "" {
			if m.DBInstanceIdentifier == targetInstanceID {
				targetIdx = i
			}

			continue
		}
		if !m.IsClusterWriter && targetIdx == -1 {
			targetIdx = i
		}
	}
	if targetIdx == -1 {
		return fmt.Errorf(
			"%w: target instance %s is not a valid failover target for cluster %s",
			ErrInvalidDBInstanceStateFault,
			targetInstanceID,
			c.DBClusterIdentifier,
		)
	}
	if c.DBClusterMembers[targetIdx].IsClusterWriter {
		return fmt.Errorf(
			"%w: target instance %s is already the writer for cluster %s",
			ErrInvalidDBInstanceStateFault,
			targetInstanceID,
			c.DBClusterIdentifier,
		)
	}
	for i := range c.DBClusterMembers {
		c.DBClusterMembers[i].IsClusterWriter = i == targetIdx
	}

	return nil
}

// AddRoleToDBCluster associates an IAM role with a Neptune DB cluster.
func (b *InMemoryBackend) AddRoleToDBCluster(ctx context.Context, clusterID, roleARN string) error {
	if clusterID == "" {
		return fmt.Errorf("%w: DBClusterIdentifier is required", ErrInvalidParameter)
	}
	if roleARN == "" {
		return fmt.Errorf("%w: RoleArn is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("AddRoleToDBCluster")
	defer b.mu.Unlock()
	cluster, exists := b.clusterGet(region, clusterID)
	if !exists {
		return fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}
	roles := b.clusterRolesStore(region)
	if slices.Contains(roles[clusterID], roleARN) {
		return nil
	}
	roles[clusterID] = append(roles[clusterID], roleARN)
	if !slices.Contains(cluster.AssociatedRoles, roleARN) {
		cluster.AssociatedRoles = append(cluster.AssociatedRoles, roleARN)
	}

	return nil
}

// RemoveRoleFromDBCluster removes an IAM role association from a Neptune DB cluster.
func (b *InMemoryBackend) RemoveRoleFromDBCluster(
	ctx context.Context,
	clusterID, roleARN string,
) error {
	if clusterID == "" {
		return fmt.Errorf("%w: DBClusterIdentifier is required", ErrInvalidParameter)
	}
	if roleARN == "" {
		return fmt.Errorf("%w: RoleArn is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("RemoveRoleFromDBCluster")
	defer b.mu.Unlock()
	cluster, exists := b.clusterGet(region, clusterID)
	if !exists {
		return fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}
	rolesStore := b.clusterRolesStore(region)
	roles := rolesStore[clusterID]
	kept := make([]string, 0, len(roles))
	for _, r := range roles {
		if r != roleARN {
			kept = append(kept, r)
		}
	}
	rolesStore[clusterID] = kept
	keptRoles := make([]string, 0, len(cluster.AssociatedRoles))
	for _, r := range cluster.AssociatedRoles {
		if r != roleARN {
			keptRoles = append(keptRoles, r)
		}
	}
	cluster.AssociatedRoles = keptRoles

	return nil
}

// RestoreDBClusterFromSnapshot restores a Neptune DB cluster from a snapshot.
func (b *InMemoryBackend) RestoreDBClusterFromSnapshot(
	ctx context.Context, snapshotID, clusterID string,
) (*DBCluster, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: DBClusterSnapshotIdentifier is required", ErrInvalidParameter)
	}
	if clusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("RestoreDBClusterFromSnapshot")
	defer b.mu.Unlock()
	snap, snapExists := b.clusterSnapshotGet(region, snapshotID)
	if !snapExists {
		return nil, fmt.Errorf(
			"%w: cluster snapshot %s not found",
			ErrClusterSnapshotNotFound,
			snapshotID,
		)
	}
	if b.clusterHas(region, clusterID) {
		return nil, fmt.Errorf("%w: cluster %s already exists", ErrClusterAlreadyExists, clusterID)
	}
	// Derive parameter group from the source cluster if available.
	paramGroupName := pgFamilyDefaultNeptune13
	if srcCluster, ok := b.clusterGet(region, snap.DBClusterIdentifier); ok {
		paramGroupName = srcCluster.DBClusterParameterGroupName
	}
	endpoint := fmt.Sprintf("%s.cluster.%s.neptune.amazonaws.com", clusterID, region)
	readerEndpoint := fmt.Sprintf("%s.cluster-ro.%s.neptune.amazonaws.com", clusterID, region)
	cluster := &DBCluster{
		region:                      region,
		DBClusterIdentifier:         clusterID,
		DBClusterArn:                b.clusterARN(region, clusterID),
		DBClusterResourceID:         fmt.Sprintf("cluster-%s", clusterID),
		ClusterCreateTime:           nowISO8601(),
		Engine:                      snap.Engine,
		EngineVersion:               snap.EngineVersion,
		EngineMode:                  engineModeProvisioned,
		Status:                      clusterStatusAvailable,
		DBClusterParameterGroupName: paramGroupName,
		Endpoint:                    endpoint,
		ReaderEndpoint:              readerEndpoint,
		Port:                        defaultNeptunePort,
		StorageEncrypted:            snap.StorageEncrypted,
		DBClusterMembers:            []DBClusterMember{},
		BackupRetentionPeriod:       defaultBackupRetentionPeriod,
	}
	b.clusterPut(cluster)
	cp := cloneCluster(cluster)

	return &cp, nil
}

// RestoreDBClusterToPointInTime restores a Neptune DB cluster to a point in time.
func (b *InMemoryBackend) RestoreDBClusterToPointInTime(
	ctx context.Context, srcClusterID, targetClusterID string,
) (*DBCluster, error) {
	if srcClusterID == "" {
		return nil, fmt.Errorf("%w: SourceDBClusterIdentifier is required", ErrInvalidParameter)
	}
	if targetClusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("RestoreDBClusterToPointInTime")
	defer b.mu.Unlock()
	src, srcExists := b.clusterGet(region, srcClusterID)
	if !srcExists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, srcClusterID)
	}
	if b.clusterHas(region, targetClusterID) {
		return nil, fmt.Errorf(
			"%w: cluster %s already exists",
			ErrClusterAlreadyExists,
			targetClusterID,
		)
	}
	endpoint := fmt.Sprintf("%s.cluster.%s.neptune.amazonaws.com", targetClusterID, region)
	readerEndpoint := fmt.Sprintf("%s.cluster-ro.%s.neptune.amazonaws.com", targetClusterID, region)
	cluster := &DBCluster{
		region:                          region,
		DBClusterIdentifier:             targetClusterID,
		DBClusterArn:                    b.clusterARN(region, targetClusterID),
		DBClusterResourceID:             fmt.Sprintf("cluster-%s", targetClusterID),
		ClusterCreateTime:               nowISO8601(),
		Engine:                          src.Engine,
		EngineVersion:                   src.EngineVersion,
		EngineMode:                      src.EngineMode,
		Status:                          clusterStatusAvailable,
		DBClusterParameterGroupName:     src.DBClusterParameterGroupName,
		Endpoint:                        endpoint,
		ReaderEndpoint:                  readerEndpoint,
		Port:                            src.Port,
		StorageEncrypted:                src.StorageEncrypted,
		EnableIAMDatabaseAuthentication: src.EnableIAMDatabaseAuthentication,
		DeletionProtection:              src.DeletionProtection,
		DBClusterMembers:                []DBClusterMember{},
		BackupRetentionPeriod:           src.BackupRetentionPeriod,
	}
	b.clusterPut(cluster)
	cp := cloneCluster(cluster)

	return &cp, nil
}

// AddClusterInternal creates a cluster directly, bypassing normal validation. Used for seeding tests.
func (b *InMemoryBackend) AddClusterInternal(id string) *DBCluster {
	b.mu.Lock("AddClusterInternal")
	defer b.mu.Unlock()
	endpoint := fmt.Sprintf("%s.cluster.%s.neptune.amazonaws.com", id, b.region)
	readerEndpoint := fmt.Sprintf("%s.cluster-ro.%s.neptune.amazonaws.com", id, b.region)
	c := &DBCluster{
		region:                      b.region,
		DBClusterIdentifier:         id,
		DBClusterArn:                b.clusterARN(b.region, id),
		Engine:                      neptuneEngine,
		EngineVersion:               defaultEngineVersion,
		EngineMode:                  engineModeProvisioned,
		Status:                      clusterStatusAvailable,
		DBClusterParameterGroupName: pgFamilyDefaultNeptune13,
		Endpoint:                    endpoint,
		ReaderEndpoint:              readerEndpoint,
		Port:                        defaultNeptunePort,
		BackupRetentionPeriod:       defaultBackupRetentionPeriod,
	}
	b.clusterPut(c)
	cp := cloneCluster(c)

	return &cp
}
