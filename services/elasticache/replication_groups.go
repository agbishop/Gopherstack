package elasticache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// authTokenHexLen is the byte length of a generated auth token before hex encoding.
const authTokenHexLen = 32

// redisClusterHashSlots is the total number of hash slots in a Redis cluster (gap #2).
const redisClusterHashSlots = 16384

// dataTieringMinMajorVersion is the minimum engine major version required for data tiering (gap #9).
const dataTieringMinMajorVersion = 7

// semverSplitParts is the max parts to split a semver string into for major version extraction.
const semverSplitParts = 2

// decimalBase is the base for decimal digit accumulation.
const decimalBase = 10

// Transit encryption mode constants.
const (
	transitEncryptionModePreferred = "preferred"
	transitEncryptionModeRequired  = "required"
)

func (b *InMemoryBackend) replicationGroupARN(region, id string) string {
	return arn.Build("elasticache", region, b.accountID, "replicationgroup:"+id)
}

// createReplicationGroupLocked creates a replication group assuming b.mu is already held.
func (b *InMemoryBackend) createReplicationGroupLocked(
	region, id, description, paramGroupName, maintenanceWindow, snapshotWindow string,
) *ReplicationGroup {
	rg := &ReplicationGroup{
		ReplicationGroupID:         id,
		Description:                description,
		Status:                     statusAvailable,
		ARN:                        b.replicationGroupARN(region, id),
		Tags:                       tags.New("elasticache.rg." + id + ".tags"),
		CreatedAt:                  time.Now(),
		CacheParameterGroupName:    paramGroupName,
		PreferredMaintenanceWindow: maintenanceWindow,
		SnapshotWindow:             snapshotWindow,
	}
	b.markCreatingLocked(&rg.PendingStatus, &rg.AvailableAt)
	b.replicationGroupsStore(region).Put(rg)
	b.appendEventLocked(id, "replication-group", "replication group created")

	return rg
}

// CreateReplicationGroup creates a replication group.
func (b *InMemoryBackend) CreateReplicationGroup(
	ctx context.Context,
	id, description string,
) (*ReplicationGroup, error) {
	b.mu.Lock("CreateReplicationGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	b.pruneRegionLocked(region)
	if _, exists := b.replicationGroupsStore(region).Get(id); exists {
		return nil, ErrReplicationGroupAlreadyExists
	}

	return b.replicationGroupView(b.createReplicationGroupLocked(region, id, description, "", "", "")), nil
}

// CreateReplicationGroupWithOptions creates a replication group with optional parameter group and scheduling windows.
func (b *InMemoryBackend) CreateReplicationGroupWithOptions(
	ctx context.Context,
	id, description, paramGroupName, maintenanceWindow, snapshotWindow string,
) (*ReplicationGroup, error) {
	b.mu.Lock("CreateReplicationGroupWithOptions")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	if _, exists := b.replicationGroupsStore(region).Get(id); exists {
		return nil, ErrReplicationGroupAlreadyExists
	}

	if paramGroupName != "" {
		if _, ok := b.parameterGroupsStore(region).Get(paramGroupName); !ok {
			return nil, ErrParameterGroupNotFound
		}
	}

	return b.replicationGroupView(b.createReplicationGroupLocked(
		region,
		id,
		description,
		paramGroupName,
		maintenanceWindow,
		snapshotWindow,
	)), nil
}

// DeleteReplicationGroup removes a replication group.
func (b *InMemoryBackend) DeleteReplicationGroup(ctx context.Context, id string) error {
	b.mu.Lock("DeleteReplicationGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	b.pruneRegionLocked(region)
	tbl := b.replicationGroupsStore(region)
	rg, exists := tbl.Get(id)
	if !exists || isReaped(b.now(), rg.PendingStatus, rg.AvailableAt) {
		return ErrReplicationGroupNotFound
	}
	if err := b.requireAvailableLocked(
		rg.Status, rg.PendingStatus, rg.AvailableAt, ErrReplicationGroupNotAvailable,
	); err != nil {
		return err
	}

	if d := b.pendingUntil(); !d.IsZero() {
		rg.PendingStatus = statusDeleting
		rg.AvailableAt = d
		b.appendEventLocked(id, "replication-group", "replication group deleting")

		return nil
	}

	rg.Tags.Close()
	tbl.Delete(id)
	b.appendEventLocked(id, "replication-group", "replication group deleted")

	return nil
}

// DescribeReplicationGroups returns one replication group by id, or a paginated list of all when id is empty.
func (b *InMemoryBackend) DescribeReplicationGroups(
	ctx context.Context,
	id, marker string,
	maxRecords int,
) (page.Page[ReplicationGroup], error) {
	b.mu.RLock("DescribeReplicationGroups")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	p, err := describePaged(b.replicationGroupsStoreRO(region), id, ErrReplicationGroupNotFound, nil,
		func(rg ReplicationGroup) string { return rg.ReplicationGroupID }, marker, maxRecords)

	return b.finalizeReplicationGroupPage(id, p, err)
}

// ModifyReplicationGroup modifies an existing replication group.
func (b *InMemoryBackend) ModifyReplicationGroup(
	ctx context.Context,
	id, description, paramGroupName, engineVersion, cacheNodeType, maintenanceWindow, snapshotWindow string,
	automaticFailoverEnabled, multiAZEnabled *bool,
) (*ReplicationGroup, error) {
	b.mu.Lock("ModifyReplicationGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, exists := b.replicationGroupsStore(region).Get(id)
	if !exists {
		return nil, ErrReplicationGroupNotFound
	}

	if description != "" {
		rg.Description = description
	}

	if paramGroupName != "" {
		if _, ok := b.parameterGroupsStore(region).Get(paramGroupName); !ok {
			return nil, ErrParameterGroupNotFound
		}
		rg.CacheParameterGroupName = paramGroupName
	}

	if engineVersion != "" {
		rg.EngineVersion = engineVersion
	}

	if cacheNodeType != "" {
		rg.CacheNodeType = cacheNodeType
	}

	if automaticFailoverEnabled != nil {
		if *automaticFailoverEnabled {
			rg.AutomaticFailover = statusEnabled
		} else {
			rg.AutomaticFailover = statusDisabled
		}
	}

	if multiAZEnabled != nil {
		rg.MultiAZEnabled = *multiAZEnabled
	}

	if maintenanceWindow != "" {
		rg.PreferredMaintenanceWindow = maintenanceWindow
	}

	if snapshotWindow != "" {
		rg.SnapshotWindow = snapshotWindow
	}

	b.markTransitionLocked(&rg.PendingStatus, &rg.AvailableAt, statusModifying)
	b.appendEventLocked(id, "replication-group", "replication group modified")

	return b.replicationGroupView(rg), nil
}

// FailoverReplicationGroup simulates a failover for the given replication group.
func (b *InMemoryBackend) FailoverReplicationGroup(ctx context.Context, id, _ string) (*ReplicationGroup, error) {
	b.mu.Lock("FailoverReplicationGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, exists := b.replicationGroupsStore(region).Get(id)
	if !exists {
		return nil, ErrReplicationGroupNotFound
	}
	if err := b.requireAvailableLocked(
		rg.Status, rg.PendingStatus, rg.AvailableAt, ErrReplicationGroupNotAvailable,
	); err != nil {
		return nil, err
	}

	rg.Status = statusAvailable
	b.markTransitionLocked(&rg.PendingStatus, &rg.AvailableAt, statusFailingOver)
	b.appendEventLocked(id, "replication-group", "failover completed")

	return b.replicationGroupView(rg), nil
}

// resizeNodeGroups resizes a node-group slice to targetCount, preserving existing groups
// and adding stub groups as needed. replicaCount controls the replica stub count.
func resizeNodeGroups(existing []NodeGroup, targetCount, replicaCount int) []NodeGroup {
	if targetCount <= 0 {
		return existing
	}

	if len(existing) == targetCount {
		return existing
	}

	out := make([]NodeGroup, targetCount)
	copy(out, existing)

	slotSize := redisClusterHashSlots / targetCount
	for i := len(existing); i < targetCount; i++ {
		slotStart := i * slotSize
		slotEnd := slotStart + slotSize - 1
		if i == targetCount-1 {
			slotEnd = redisClusterHashSlots - 1
		}

		ng := NodeGroup{
			NodeGroupID: fmt.Sprintf("%04d", i+1),
			Status:      statusAvailable,
			Slots:       fmt.Sprintf("%d-%d", slotStart, slotEnd),

			Replicas: make([]NodeGroupNode, replicaCount)}
		out[i] = ng
	}

	return out
}

// ----------------------------------------
// generateAuthToken creates a random 64-char hex token (gap #4)
// ----------------------------------------

func generateAuthToken() string {
	b := make([]byte, authTokenHexLen)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// ----------------------------------------
// validateCreateOpts validates cross-field constraints on create options
// ----------------------------------------

func validateCreateOpts(opts ReplicationGroupCreateOpts) error {
	if opts.DataTieringEnabled {
		if err := validateDataTiering(opts.CacheNodeType, opts.Engine, opts.EngineVersion); err != nil {
			return err
		}
	}

	if opts.TransitEncryptionMode == transitEncryptionModeRequired && !opts.AuthTokenEnabled {
		return ErrAuthTokenRequiredForMode
	}

	return nil
}

// validateDataTiering checks node-type and engine constraints for data tiering (gap #9).
func validateDataTiering(nodeType, engine, engineVersion string) error {
	if nodeType != "" && !strings.HasPrefix(nodeType, "cache.r7g") {
		return ErrDataTieringInvalid
	}

	if engine != "" && engine != engineRedis && engine != engineValkey {
		return ErrDataTieringInvalid
	}

	if engineVersion != "" {
		major := majorVersion(engineVersion)
		if major < dataTieringMinMajorVersion {
			return ErrDataTieringInvalid
		}
	}

	return nil
}

// majorVersion parses the leading integer from a semver string.
func majorVersion(v string) int {
	parts := strings.SplitN(v, ".", semverSplitParts)
	if len(parts) == 0 {
		return 0
	}

	n := 0
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			break
		}
		n = n*decimalBase + int(r-'0')
	}

	return n
}

// ----------------------------------------
// CreateReplicationGroupFull (gaps #1-#15)
// ----------------------------------------

// CreateReplicationGroupFull creates a replication group with the full set of options.
func (b *InMemoryBackend) CreateReplicationGroupFull(
	ctx context.Context,
	opts ReplicationGroupCreateOpts,
) (*ReplicationGroup, error) {
	b.mu.Lock("CreateReplicationGroupFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rgStore := b.replicationGroupsStore(region)

	if _, exists := rgStore.Get(opts.ID); exists {
		return nil, ErrReplicationGroupAlreadyExists
	}

	if opts.ParameterGroupName != "" {
		if _, ok := b.parameterGroupsStore(region).Get(opts.ParameterGroupName); !ok {
			return nil, ErrParameterGroupNotFound
		}
	}

	if opts.SnapshotName != "" {
		snap, ok := b.snapshotsStore(region).Get(opts.SnapshotName)
		if !ok || isReaped(b.now(), snap.PendingStatus, snap.AvailableAt) {
			return nil, ErrSnapshotNotFound
		}

		// Restoring from a snapshot inherits its engine/version/node type for
		// any field the caller didn't explicitly override.
		if opts.Engine == "" {
			opts.Engine = snap.Engine
		}

		if opts.EngineVersion == "" {
			opts.EngineVersion = snap.EngineVersion
		}

		if opts.CacheNodeType == "" {
			opts.CacheNodeType = snap.NodeType
		}
	}

	if err := validateCreateOpts(opts); err != nil {
		return nil, err
	}

	rg := b.buildReplicationGroupFromCreateOpts(region, opts)
	b.markCreatingLocked(&rg.PendingStatus, &rg.AvailableAt)
	rgStore.Put(rg)
	b.appendEventLocked(opts.ID, "replication-group", "replication group created")

	return b.replicationGroupView(rg), nil
}

// buildReplicationGroupFromCreateOpts assembles the ReplicationGroup from opts.
func (b *InMemoryBackend) buildReplicationGroupFromCreateOpts(
	region string,
	opts ReplicationGroupCreateOpts,
) *ReplicationGroup {
	rg := &ReplicationGroup{
		ReplicationGroupID:         opts.ID,
		Description:                opts.Description,
		Status:                     statusAvailable,
		ARN:                        b.replicationGroupARN(region, opts.ID),
		Tags:                       tags.New("elasticache.rg." + opts.ID + ".tags"),
		CreatedAt:                  time.Now(),
		CacheParameterGroupName:    opts.ParameterGroupName,
		PreferredMaintenanceWindow: opts.MaintenanceWindow,
		SnapshotWindow:             opts.SnapshotWindow,
		ClusterModeEnabled:         opts.ClusterModeEnabled,
		AtRestEncryptionEnabled:    opts.AtRestEncryptionEnabled,
		TransitEncryptionEnabled:   opts.TransitEncryptionEnabled,
		TransitEncryptionMode:      opts.TransitEncryptionMode,
		KmsKeyID:                   opts.KmsKeyID,
		NotificationTopicArn:       opts.NotificationTopicArn,
		DataTieringEnabled:         opts.DataTieringEnabled,
		MultiAZEnabled:             opts.MultiAZEnabled,
		SnapshotRetentionLimit:     opts.SnapshotRetentionLimit,
		LogDeliveryConfigurations:  opts.LogDeliveryConfigurations,
		Durability:                 opts.Durability,
	}

	applyAuthToken(rg, opts.AuthToken, opts.AuthTokenEnabled)

	if opts.AutomaticFailoverEnabled {
		rg.AutomaticFailover = statusEnabled
	}

	if opts.Engine != "" {
		rg.Engine = opts.Engine
	}

	if opts.EngineVersion != "" {
		rg.EngineVersion = opts.EngineVersion
	}

	if opts.CacheNodeType != "" {
		rg.CacheNodeType = opts.CacheNodeType
	}

	if opts.NumNodeGroups > 0 {
		rg.NodeGroups = resizeNodeGroups(nil, int(opts.NumNodeGroups), int(opts.ReplicasPerNodeGroup))
	}

	if opts.ReplicasPerNodeGroup > 0 {
		rg.ReplicaCount = opts.ReplicasPerNodeGroup
	}

	if len(opts.UserGroupIDs) > 0 {
		rg.UserGroupIDs = opts.UserGroupIDs
	}

	if len(opts.Tags) > 0 {
		for k, v := range opts.Tags {
			rg.Tags.Set(k, v)
		}
	}

	return rg
}

// applyAuthToken sets auth-token fields on a replication group.
func applyAuthToken(rg *ReplicationGroup, token string, enabled bool) {
	if enabled {
		rg.AuthTokenEnabled = true
		if token == "" {
			token = generateAuthToken()
		}
		rg.AuthToken = token
		now := time.Now()
		rg.AuthTokenLastModifiedDate = &now
	}
}

// ----------------------------------------
// ModifyReplicationGroupFull (gap #7 ApplyImmediately, gap #4 auth token rotation)
// ----------------------------------------

// ModifyReplicationGroupFull modifies a replication group with the full set of options.
func (b *InMemoryBackend) ModifyReplicationGroupFull(
	ctx context.Context,
	id string,
	opts ReplicationGroupModifyOpts,
) (*ReplicationGroup, error) {
	b.mu.Lock("ModifyReplicationGroupFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, exists := b.replicationGroupsStore(region).Get(id)
	if !exists {
		return nil, ErrReplicationGroupNotFound
	}
	if err := b.requireAvailableLocked(
		rg.Status, rg.PendingStatus, rg.AvailableAt, ErrReplicationGroupNotAvailable,
	); err != nil {
		return nil, err
	}

	if opts.ParameterGroupName != "" {
		if _, ok := b.parameterGroupsStore(region).Get(opts.ParameterGroupName); !ok {
			return nil, ErrParameterGroupNotFound
		}
	}

	if err := validateTransitEncryptionModify(rg, opts); err != nil {
		return nil, err
	}

	b.applyModifyOptsLocked(rg, opts)
	b.markTransitionLocked(&rg.PendingStatus, &rg.AvailableAt, statusModifying)
	b.appendEventLocked(id, "replication-group", "replication group modified")

	return b.replicationGroupView(rg), nil
}

// applyModifyOptsLocked applies modification options to an existing replication group.
func (b *InMemoryBackend) applyModifyOptsLocked(rg *ReplicationGroup, opts ReplicationGroupModifyOpts) {
	if opts.Description != "" {
		rg.Description = opts.Description
	}

	if opts.ParameterGroupName != "" {
		rg.CacheParameterGroupName = opts.ParameterGroupName
	}

	if opts.EngineVersion != "" {
		rg.EngineVersion = opts.EngineVersion
	}

	if opts.CacheNodeType != "" {
		rg.CacheNodeType = opts.CacheNodeType
	}

	if opts.MaintenanceWindow != "" {
		rg.PreferredMaintenanceWindow = opts.MaintenanceWindow
	}

	if opts.SnapshotWindow != "" {
		rg.SnapshotWindow = opts.SnapshotWindow
	}

	if opts.NotificationTopicArn != "" {
		rg.NotificationTopicArn = opts.NotificationTopicArn
	}

	if opts.Durability != "" {
		rg.Durability = opts.Durability
	}

	if len(opts.LogDeliveryConfigurations) > 0 {
		rg.LogDeliveryConfigurations = opts.LogDeliveryConfigurations
	}

	if opts.SnapshotRetentionLimit != nil {
		rg.SnapshotRetentionLimit = *opts.SnapshotRetentionLimit
	}

	applyAutoFailoverModify(rg, opts.AutomaticFailoverEnabled)

	if opts.MultiAZEnabled != nil {
		rg.MultiAZEnabled = *opts.MultiAZEnabled
	}

	if opts.ReplicaCount != nil {
		rg.ReplicaCount = *opts.ReplicaCount
	}

	applyUserGroupIDsModify(rg, opts.UserGroupIDsToAdd, opts.UserGroupIDsToRemove)
	applyAuthTokenModify(rg, opts.AuthToken, opts.AuthTokenUpdateStrategy)
	applyTransitEncryptionModify(rg, opts.TransitEncryptionMode)
	applyPendingChanges(rg, opts)
}

func applyAutoFailoverModify(rg *ReplicationGroup, enabled *bool) {
	if enabled == nil {
		return
	}

	if *enabled {
		rg.AutomaticFailover = statusEnabled
	} else {
		rg.AutomaticFailover = statusDisabled
	}
}

// applyUserGroupIDsModify adds/removes user group IDs on a replication group (gap #15).
func applyUserGroupIDsModify(rg *ReplicationGroup, toAdd, toRemove []string) {
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return
	}

	removeSet := make(map[string]bool, len(toRemove))
	for _, id := range toRemove {
		removeSet[id] = true
	}

	filtered := rg.UserGroupIDs[:0:0]
	for _, id := range rg.UserGroupIDs {
		if !removeSet[id] {
			filtered = append(filtered, id)
		}
	}

	addSet := make(map[string]bool)
	for _, id := range filtered {
		addSet[id] = true
	}

	for _, id := range toAdd {
		if !addSet[id] {
			filtered = append(filtered, id)
		}
	}

	rg.UserGroupIDs = filtered
}

// applyAuthTokenModify handles auth token rotation strategies (gap #4).
func applyAuthTokenModify(rg *ReplicationGroup, token, strategy string) {
	if token == "" && strategy == "" {
		return
	}

	switch strategy {
	case "DELETE":
		rg.AuthToken = ""
		rg.AuthTokenEnabled = false
		rg.AuthTokenLastModifiedDate = nil
	case "SET":
		if token == "" {
			token = generateAuthToken()
		}
		rg.AuthToken = token
		rg.AuthTokenEnabled = true
		now := time.Now()
		rg.AuthTokenLastModifiedDate = &now
	case "ROTATE":
		rg.AuthToken = generateAuthToken()
		now := time.Now()
		rg.AuthTokenLastModifiedDate = &now
	}
}

// validateTransitEncryptionModify rejects switching TransitEncryptionMode to
// "required" when the replication group will end up without an auth token,
// matching AWS's rule that required-mode transit encryption needs an auth
// token enabled (either already present or set in the same request).
func validateTransitEncryptionModify(rg *ReplicationGroup, opts ReplicationGroupModifyOpts) error {
	if opts.TransitEncryptionMode != transitEncryptionModeRequired {
		return nil
	}

	authTokenWillBeEnabled := rg.AuthTokenEnabled

	switch opts.AuthTokenUpdateStrategy {
	case "SET", "ROTATE":
		authTokenWillBeEnabled = true
	case "DELETE":
		authTokenWillBeEnabled = false
	}

	if opts.AuthToken != "" {
		authTokenWillBeEnabled = true
	}

	if !authTokenWillBeEnabled {
		return ErrTransitEncryptionModeInvalid
	}

	return nil
}

// applyTransitEncryptionModify applies transit encryption mode change (gap #13).
func applyTransitEncryptionModify(rg *ReplicationGroup, mode string) {
	if mode == "" {
		return
	}

	rg.TransitEncryptionMode = mode

	if mode == transitEncryptionModeRequired || mode == transitEncryptionModePreferred {
		rg.TransitEncryptionEnabled = true
	}
}

// applyPendingChanges records pending modifications when ApplyImmediately is false (gap #7).
func applyPendingChanges(rg *ReplicationGroup, opts ReplicationGroupModifyOpts) {
	if opts.ApplyImmediately {
		rg.PendingModifiedValues = nil

		return
	}

	if opts.CacheNodeType == "" && opts.EngineVersion == "" &&
		opts.AutomaticFailoverEnabled == nil && opts.ReplicaCount == nil {
		return
	}

	pending := &RGPendingModifiedValues{}

	if opts.CacheNodeType != "" {
		pending.CacheNodeType = opts.CacheNodeType
	}

	if opts.EngineVersion != "" {
		pending.EngineVersion = opts.EngineVersion
	}

	if opts.AutomaticFailoverEnabled != nil {
		if *opts.AutomaticFailoverEnabled {
			pending.AutomaticFailoverStatus = statusEnabled
		} else {
			pending.AutomaticFailoverStatus = statusDisabled
		}
	}

	if opts.ReplicaCount != nil {
		rc := *opts.ReplicaCount
		pending.ReplicaCount = &rc
	}

	rg.PendingModifiedValues = pending
}

// ----------------------------------------
// TriggerAutoSnapshot (gap #14)
// ----------------------------------------

// TriggerAutoSnapshot creates an automated snapshot for the given replication group.
func (b *InMemoryBackend) TriggerAutoSnapshot(ctx context.Context, replicationGroupID string) (*CacheSnapshot, error) {
	b.mu.Lock("TriggerAutoSnapshot")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, ok := b.replicationGroupsStore(region).Get(replicationGroupID)
	if !ok {
		return nil, ErrReplicationGroupNotFound
	}

	snapStore := b.snapshotsStore(region)
	snapName := buildAutoSnapshotName(replicationGroupID)
	if _, exists := snapStore.Get(snapName); exists {
		return nil, ErrSnapshotAlreadyExists
	}

	snap := buildAutoSnapshot(b, region, snapName, rg)
	snapStore.Put(snap)

	b.appendEventLocked(replicationGroupID, "replication-group", "automated snapshot created: "+snapName)
	pruneExpiredSnapshots(b, snapStore, replicationGroupID, rg.SnapshotRetentionLimit)

	result := *snap

	return &result, nil
}

// buildAutoSnapshotName generates a daily automated snapshot name.
func buildAutoSnapshotName(replicationGroupID string) string {
	return replicationGroupID + "-auto-" + time.Now().UTC().Format("2006-01-02")
}

// buildAutoSnapshot constructs the snapshot object.
func buildAutoSnapshot(b *InMemoryBackend, region, snapName string, rg *ReplicationGroup) *CacheSnapshot {
	ev := rg.EngineVersion
	if ev == "" {
		ev = defaultEngineVersion(engineRedis)
	}

	return &CacheSnapshot{
		SnapshotName:       snapName,
		ReplicationGroupID: rg.ReplicationGroupID,
		Status:             statusAvailable,
		ARN:                b.snapshotARN(region, snapName),
		SnapshotSource:     snapshotSourceAutomated,
		Engine:             engineRedis,
		EngineVersion:      ev,
		NodeType:           rg.CacheNodeType,
		CreatedAt:          time.Now(),
		Tags:               tags.New("elasticache.snapshot." + snapName + ".tags"),
	}
}

// sortAutoSnapshots sorts snapshots by CreatedAt ascending (oldest first).
func sortAutoSnapshots(snaps []CacheSnapshot) {
	n := len(snaps)
	for i := range n - 1 {
		for j := i + 1; j < n; j++ {
			if snaps[i].CreatedAt.After(snaps[j].CreatedAt) {
				snaps[i], snaps[j] = snaps[j], snaps[i]
			}
		}
	}
}

// pruneExpiredSnapshots removes automated snapshots beyond the retention limit (gap #14).
func pruneExpiredSnapshots(
	_ *InMemoryBackend,
	tbl *store.Table[CacheSnapshot],
	replicationGroupID string,
	retentionLimit int,
) {
	if retentionLimit <= 0 {
		return
	}

	var autoSnaps []CacheSnapshot
	for _, s := range tbl.All() {
		if s.ReplicationGroupID == replicationGroupID && s.SnapshotSource == "automated" {
			autoSnaps = append(autoSnaps, *s)
		}
	}

	if len(autoSnaps) <= retentionLimit {
		return
	}

	// Sort oldest first.
	sortAutoSnapshots(autoSnaps)

	excess := len(autoSnaps) - retentionLimit
	for i := range excess {
		snap := autoSnaps[i]
		if s, ok := tbl.Get(snap.SnapshotName); ok {
			s.Tags.Close()
			tbl.Delete(snap.SnapshotName)
		}
	}
}

// StartMigration starts a migration for a replication group.
// CustomerNodeEndpointList is required on the wire (aws-sdk-go-v2's
// StartMigrationInput); this backend has nowhere real to echo the endpoints
// back (ReplicationGroup's response shape never carries them), so it acts on
// the field by enforcing AWS's required-member contract instead.
func (b *InMemoryBackend) StartMigration(
	ctx context.Context,
	replicationGroupID string,
	customerNodeEndpoints []CustomerNodeEndpoint,
) (*ReplicationGroup, error) {
	if len(customerNodeEndpoints) == 0 {
		return nil, ErrCustomerNodeEndpointsRequired
	}

	b.mu.Lock("StartMigration")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, ok := b.replicationGroupsStore(region).Get(replicationGroupID)
	if !ok {
		return nil, ErrReplicationGroupNotFound
	}

	rg.Status = "migrating"
	result := *rg

	return &result, nil
}

// TestMigration tests a migration for a replication group. See StartMigration's
// doc comment for why CustomerNodeEndpointList is validated but not persisted.
func (b *InMemoryBackend) TestMigration(
	ctx context.Context,
	replicationGroupID string,
	customerNodeEndpoints []CustomerNodeEndpoint,
) (*ReplicationGroup, error) {
	if len(customerNodeEndpoints) == 0 {
		return nil, ErrCustomerNodeEndpointsRequired
	}

	b.mu.Lock("TestMigration")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, ok := b.replicationGroupsStore(region).Get(replicationGroupID)
	if !ok {
		return nil, ErrReplicationGroupNotFound
	}

	result := *rg

	return &result, nil
}

// IncreaseReplicaCount increases the replica count for a replication group.
// AWS documents ApplyImmediately as required and states "ApplyImmediately=False
// is not currently supported" for this operation, so applyImmediately=false is
// rejected rather than silently accepted or faked as a deferred change.
func (b *InMemoryBackend) IncreaseReplicaCount(
	ctx context.Context,
	replicationGroupID string,
	newReplicaCount int32,
	applyImmediately bool,
) (*ReplicationGroup, error) {
	if !applyImmediately {
		return nil, ErrApplyImmediatelyRequired
	}

	b.mu.Lock("IncreaseReplicaCount")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, ok := b.replicationGroupsStore(region).Get(replicationGroupID)
	if !ok {
		return nil, ErrReplicationGroupNotFound
	}
	if err := b.requireAvailableLocked(
		rg.Status, rg.PendingStatus, rg.AvailableAt, ErrReplicationGroupNotAvailable,
	); err != nil {
		return nil, err
	}

	if newReplicaCount > 0 {
		rg.ReplicaCount = newReplicaCount
	}

	b.appendEventLocked(replicationGroupID, "replication-group", "replica count increased")

	result := *rg

	return &result, nil
}

// DecreaseReplicaCount decreases the replica count for a replication group.
// See IncreaseReplicaCount's doc comment: AWS documents ApplyImmediately=false
// as unsupported for this operation too.
func (b *InMemoryBackend) DecreaseReplicaCount(
	ctx context.Context,
	replicationGroupID string,
	newReplicaCount int32,
	applyImmediately bool,
) (*ReplicationGroup, error) {
	if !applyImmediately {
		return nil, ErrApplyImmediatelyRequired
	}

	b.mu.Lock("DecreaseReplicaCount")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, ok := b.replicationGroupsStore(region).Get(replicationGroupID)
	if !ok {
		return nil, ErrReplicationGroupNotFound
	}
	if err := b.requireAvailableLocked(
		rg.Status, rg.PendingStatus, rg.AvailableAt, ErrReplicationGroupNotAvailable,
	); err != nil {
		return nil, err
	}

	if newReplicaCount >= 0 {
		rg.ReplicaCount = newReplicaCount
	}

	b.appendEventLocked(replicationGroupID, "replication-group", "replica count decreased")

	result := *rg

	return &result, nil
}

// ModifyReplicationGroupShardConfiguration modifies the shard configuration of a replication group.
// Cluster mode must be enabled to use this operation. AWS documents
// ApplyImmediately as required with "Value: true" -- "the only permitted
// value for this parameter is true" -- so applyImmediately=false is rejected.
func (b *InMemoryBackend) ModifyReplicationGroupShardConfiguration(
	ctx context.Context,
	replicationGroupID string,
	nodeGroupCount int32,
	applyImmediately bool,
) (*ReplicationGroup, error) {
	if !applyImmediately {
		return nil, ErrApplyImmediatelyRequired
	}

	b.mu.Lock("ModifyReplicationGroupShardConfiguration")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, ok := b.replicationGroupsStore(region).Get(replicationGroupID)
	if !ok {
		return nil, ErrReplicationGroupNotFound
	}
	if err := b.requireAvailableLocked(
		rg.Status, rg.PendingStatus, rg.AvailableAt, ErrReplicationGroupNotAvailable,
	); err != nil {
		return nil, err
	}

	if !rg.ClusterModeEnabled {
		return nil, ErrClusterModeRequired
	}

	if nodeGroupCount > 0 {
		rg.NodeGroups = resizeNodeGroups(rg.NodeGroups, int(nodeGroupCount), int(rg.ReplicaCount))
	}

	b.appendEventLocked(replicationGroupID, "replication-group", "shard configuration modified")

	result := *rg

	return &result, nil
}

// CompleteMigration completes an online data migration from an external Redis server to this replication group.
func (b *InMemoryBackend) CompleteMigration(
	ctx context.Context,
	replicationGroupID string,
	_ bool,
) (*ReplicationGroup, error) {
	b.mu.Lock("CompleteMigration")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	rg, ok := b.replicationGroupsStore(region).Get(replicationGroupID)
	if !ok {
		return nil, ErrReplicationGroupNotFound
	}

	rg.Status = statusAvailable
	result := *rg

	return &result, nil
}

// ----------------------------------------
// Seed helpers (for test isolation)
// ----------------------------------------
