// Package dynamodb implements the AWS DynamoDB mock service.
// global_tables.go implements the CreateGlobalTable/DescribeGlobalTable/
// ListGlobalTables/UpdateGlobalTable/UpdateGlobalTableSettings family. Physical
// replica propagation lives in replication.go.
package dynamodb

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/ptrconv"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

const (
	errGlobalTableNotFoundType = "com.amazonaws.dynamodb.v20120810#GlobalTableNotFoundException"
)

// --- CreateGlobalTable ---

// CreateGlobalTable creates a global table, physically instantiating replica Table entries
// in each specified region. If a table named GlobalTableName already exists in a region,
// it is adopted into the global table; otherwise a new empty table is created there.
// The source schema is taken from the first region where the table already exists.
// All replicas get their Replicas field populated with the other regions, matching the
// DescribeTable output that AWS returns for global tables.
func (db *InMemoryDB) CreateGlobalTable(
	_ context.Context,
	input *dynamodb.CreateGlobalTableInput,
) (*dynamodb.CreateGlobalTableOutput, error) {
	if input.GlobalTableName == nil || *input.GlobalTableName == "" {
		return nil, NewValidationException("GlobalTableName is required")
	}

	if len(input.ReplicationGroup) == 0 {
		return nil, NewValidationException("ReplicationGroup must contain at least one region")
	}

	name := *input.GlobalTableName
	regions := collectValidRegions(input.ReplicationGroup)

	if len(regions) == 0 {
		return nil, NewValidationException(
			"ReplicationGroup must contain at least one valid region",
		)
	}

	db.mu.Lock("CreateGlobalTable")
	defer db.mu.Unlock()

	if _, exists := db.globalTables.Get(name); exists {
		return nil, &Error{
			Type:    "com.amazonaws.dynamodb.v20120810#GlobalTableAlreadyExistsException",
			Message: fmt.Sprintf("Global table with name %s already exists", name),
		}
	}

	source := db.findSourceTable(name, regions)

	globalTableARN := arn.Build("dynamodb", regions[0], db.accountID, "global-table/"+name)
	now := time.Now()

	allReplicas := buildAllReplicas(regions)

	db.ensureReplicaTablesLocked(name, regions, source, allReplicas, now)

	db.globalTables.Put(&StoredGlobalTable{
		GlobalTableName:  name,
		GlobalTableArn:   globalTableARN,
		CreationDateTime: now,
		ReplicationGroup: regions,
	})

	sdkReplicas := buildSDKReplicaDescriptions(regions)

	return &dynamodb.CreateGlobalTableOutput{
		GlobalTableDescription: &types.GlobalTableDescription{
			GlobalTableName:   &name,
			GlobalTableArn:    &globalTableARN,
			GlobalTableStatus: types.GlobalTableStatusActive,
			CreationDateTime:  &now,
			ReplicationGroup:  sdkReplicas,
		},
	}, nil
}

// collectValidRegions extracts non-empty region names from a replication group.
func collectValidRegions(group []types.Replica) []string {
	regions := make([]string, 0, len(group))

	for _, r := range group {
		if r.RegionName != nil && *r.RegionName != "" {
			regions = append(regions, *r.RegionName)
		}
	}

	return regions
}

// findSourceTable returns the first Table named `name` found in the given regions.
// Must be called with db.mu held (at least read).
func (db *InMemoryDB) findSourceTable(name string, regions []string) *Table {
	for _, region := range regions {
		if tbl, tableExists := db.tables.Get(tableKey(region, name)); tableExists {
			return tbl
		}
	}

	return nil
}

// buildAllReplicas builds the complete ReplicaDescription slice for all regions.
func buildAllReplicas(regions []string) []models.ReplicaDescription {
	all := make([]models.ReplicaDescription, 0, len(regions))

	for _, r := range regions {
		all = append(all, models.ReplicaDescription{
			RegionName:    r,
			ReplicaStatus: statusActive,
		})
	}

	return all
}

// ensureReplicaTablesLocked creates or adopts a Table in each region under db.mu.
// Must be called with db.mu held (write).
func (db *InMemoryDB) ensureReplicaTablesLocked(
	name string,
	regions []string,
	source *Table,
	allReplicas []models.ReplicaDescription,
	now time.Time,
) {
	for _, region := range regions {
		if existing, exists := db.tables.Get(tableKey(region, name)); !exists {
			replica := db.buildReplicaTable(name, region, source, now)
			replica.GlobalTableName = name
			db.tables.Put(replica)
		} else {
			existing.GlobalTableName = name
		}
	}

	for _, region := range regions {
		if t, ok := db.tables.Get(tableKey(region, name)); ok {
			t.Replicas = buildReplicasExcluding(allReplicas, region)
		}
	}
}

// buildReplicaTable creates a new Table for use as a global table replica.
// If a source table exists it is cloned; otherwise a placeholder table is created.
func (db *InMemoryDB) buildReplicaTable(name, region string, source *Table, now time.Time) *Table {
	if source != nil {
		return cloneTableSchema(source, name, region, db.accountID)
	}

	t := &Table{
		Name:             name,
		Status:           statusActive,
		Items:            make([]map[string]any, 0),
		TableID:          uuid.New().String(),
		CreationDateTime: now,
		TableArn:         arn.Build("dynamodb", region, db.accountID, "table/"+name),
	}
	t.mu = newTableMutex(name)
	t.initializeIndexes()

	return t
}

// buildSDKReplicaDescriptions converts region names to SDK ReplicaDescription slice.
func buildSDKReplicaDescriptions(regions []string) []types.ReplicaDescription {
	out := make([]types.ReplicaDescription, 0, len(regions))

	for _, r := range regions {
		out = append(out, types.ReplicaDescription{
			RegionName:    &r,
			ReplicaStatus: types.ReplicaStatusActive,
		})
	}

	return out
}

// getGlobalTableRLocked returns the global table stored under name (and
// whether it exists) under a defer-protected db.mu.RLock.
func (db *InMemoryDB) getGlobalTableRLocked(name string) (*StoredGlobalTable, bool) {
	db.mu.RLock("DescribeGlobalTable")
	defer db.mu.RUnlock()

	return db.globalTables.Get(name)
}

// replicaTableCapacityRLocked returns the effective read/write capacity units
// for the table named name in region, falling back to the account maximums
// when the table doesn't exist in that region or has no explicit throughput
// set. Each lock acquisition (db.mu then table.mu) is defer-protected, so a
// panic reading either can never leave a lock held forever.
func (db *InMemoryDB) replicaTableCapacityRLocked(region, name string) (int64, int64) {
	rcu := accountMaxReadCapacityUnits
	wcu := accountMaxWriteCapacityUnits

	tbl := db.getTableInRegionRLocked(region, name, "DescribeGlobalTableSettings.table")
	if tbl == nil {
		return rcu, wcu
	}

	pt := tbl.provisionedThroughputRLocked("DescribeGlobalTableSettings.throughput")
	if pt.ReadCapacityUnits > 0 {
		rcu = int64(pt.ReadCapacityUnits)
	}
	if pt.WriteCapacityUnits > 0 {
		wcu = int64(pt.WriteCapacityUnits)
	}

	return rcu, wcu
}

// getTableInRegionRLocked looks up a table by (region, name) under a
// defer-protected db.mu.RLock, using op as the lock's metrics label.
func (db *InMemoryDB) getTableInRegionRLocked(region, name, op string) *Table {
	db.mu.RLock(op)
	defer db.mu.RUnlock()

	tbl, _ := db.tables.Get(tableKey(region, name))

	return tbl
}

// provisionedThroughputRLocked returns table.ProvisionedThroughput under a
// defer-protected table.mu.RLock, using op as the lock's metrics label.
func (table *Table) provisionedThroughputRLocked(op string) models.ProvisionedThroughputDescription {
	table.mu.RLock(op)
	defer table.mu.RUnlock()

	return table.ProvisionedThroughput
}

// --- DescribeGlobalTable ---

// DescribeGlobalTable returns the description of a global table.
func (db *InMemoryDB) DescribeGlobalTable(
	_ context.Context,
	input *dynamodb.DescribeGlobalTableInput,
) (*dynamodb.DescribeGlobalTableOutput, error) {
	if input.GlobalTableName == nil || *input.GlobalTableName == "" {
		return nil, NewValidationException("GlobalTableName is required")
	}

	name := *input.GlobalTableName

	gt, exists := db.getGlobalTableRLocked(name)
	if !exists {
		return nil, &Error{
			Type:    errGlobalTableNotFoundType,
			Message: fmt.Sprintf("Global table with name %s not found", name),
		}
	}

	sdkReplicas := make([]types.ReplicaDescription, 0, len(gt.ReplicationGroup))
	for _, region := range gt.ReplicationGroup {
		sdkReplicas = append(sdkReplicas, types.ReplicaDescription{
			RegionName:    &region,
			ReplicaStatus: types.ReplicaStatusActive,
		})
	}

	return &dynamodb.DescribeGlobalTableOutput{
		GlobalTableDescription: &types.GlobalTableDescription{
			GlobalTableName:   &gt.GlobalTableName,
			GlobalTableArn:    &gt.GlobalTableArn,
			GlobalTableStatus: types.GlobalTableStatusActive,
			CreationDateTime:  &gt.CreationDateTime,
			ReplicationGroup:  sdkReplicas,
		},
	}, nil
}

// --- DescribeGlobalTableSettings ---

// DescribeGlobalTableSettings returns per-replica settings for a global table.
func (db *InMemoryDB) DescribeGlobalTableSettings(
	_ context.Context,
	input *dynamodb.DescribeGlobalTableSettingsInput,
) (*dynamodb.DescribeGlobalTableSettingsOutput, error) {
	if input.GlobalTableName == nil || *input.GlobalTableName == "" {
		return nil, NewValidationException("GlobalTableName is required")
	}

	name := *input.GlobalTableName

	gt, exists := db.getGlobalTableRLocked(name)
	if !exists {
		return nil, &Error{
			Type:    errGlobalTableNotFoundType,
			Message: fmt.Sprintf("Global table with name %s not found", name),
		}
	}

	effectiveBilling := types.BillingModePayPerRequest
	if gt.BillingMode != "" {
		effectiveBilling = types.BillingMode(gt.BillingMode)
	}

	replicaSettings := make([]types.ReplicaSettingsDescription, 0, len(gt.ReplicationGroup))
	for _, region := range gt.ReplicationGroup {
		rcu, wcu := db.replicaTableCapacityRLocked(region, name)

		desc := types.ReplicaSettingsDescription{
			RegionName:                           &region,
			ReplicaStatus:                        types.ReplicaStatusActive,
			ReplicaProvisionedReadCapacityUnits:  &rcu,
			ReplicaProvisionedWriteCapacityUnits: &wcu,
			ReplicaBillingModeSummary:            &types.BillingModeSummary{BillingMode: effectiveBilling},
			ReplicaProvisionedWriteCapacityAutoScalingSettings: sdkAutoScalingSettingsDescription(
				gt.WriteCapacityAutoScaling,
			),
		}

		var rs *StoredReplicaSettings
		if stored, ok := gt.ReplicaSettings[region]; ok && stored != nil {
			rs = stored
			if rs.TableClass != "" {
				desc.ReplicaTableClassSummary = &types.TableClassSummary{
					TableClass: types.TableClass(rs.TableClass),
				}
			}
			desc.ReplicaProvisionedReadCapacityAutoScalingSettings = sdkAutoScalingSettingsDescription(
				rs.ReadCapacityAutoScaling,
			)
		}

		desc.ReplicaGlobalSecondaryIndexSettings = db.replicaGSISettingsRLocked(
			region, name, rs, gt.GSIWriteCapacityAutoScaling,
		)

		replicaSettings = append(replicaSettings, desc)
	}

	return &dynamodb.DescribeGlobalTableSettingsOutput{
		GlobalTableName: &gt.GlobalTableName,
		ReplicaSettings: replicaSettings,
	}, nil
}

// --- ListGlobalTables ---

// ListGlobalTables returns global tables, optionally filtered by region, with pagination support.
func (db *InMemoryDB) ListGlobalTables(
	_ context.Context,
	input *dynamodb.ListGlobalTablesInput,
) (*dynamodb.ListGlobalTablesOutput, error) {
	db.mu.RLock("ListGlobalTables")
	defer db.mu.RUnlock()

	regionFilter := ptrconv.String(input.RegionName)
	startName := ptrconv.String(input.ExclusiveStartGlobalTableName)

	names := sortedGlobalTableNames(db.globalTables, startName)
	filtered := filterGlobalTables(db.globalTables, names, regionFilter)
	filtered, lastEvaluated := applyGlobalTableLimit(filtered, input.Limit)

	return &dynamodb.ListGlobalTablesOutput{
		GlobalTables:                 filtered,
		LastEvaluatedGlobalTableName: lastEvaluated,
	}, nil
}

// --- UpdateGlobalTable ---

// UpdateGlobalTable adds or removes replica regions for an existing global table.
// Create actions physically create a new Table entry in the target region (cloning the source schema).
// Delete actions remove the Table entry from the target region.
func (db *InMemoryDB) UpdateGlobalTable(
	_ context.Context,
	input *dynamodb.UpdateGlobalTableInput,
) (*dynamodb.UpdateGlobalTableOutput, error) {
	if input.GlobalTableName == nil || *input.GlobalTableName == "" {
		return nil, NewValidationException("GlobalTableName is required")
	}

	if len(input.ReplicaUpdates) == 0 {
		return nil, NewValidationException("ReplicaUpdates must contain at least one update")
	}

	name := *input.GlobalTableName

	db.mu.Lock("UpdateGlobalTable")
	defer db.mu.Unlock()

	gt, exists := db.globalTables.Get(name)
	if !exists {
		return nil, &Error{
			Type:    errGlobalTableNotFoundType,
			Message: fmt.Sprintf("Global table with name %s not found", name),
		}
	}

	source := db.findSourceTableLocked(name, gt.ReplicationGroup)

	for _, update := range input.ReplicaUpdates {
		if err := db.applyGlobalTableReplicaUpdate(name, gt, update, source); err != nil {
			return nil, err
		}
	}

	// Rebuild per-replica Replicas field.
	db.rebuildGlobalTableReplicasLocked(name, gt.ReplicationGroup)

	sdkReplicas := buildSDKReplicaDescriptions(gt.ReplicationGroup)

	return &dynamodb.UpdateGlobalTableOutput{
		GlobalTableDescription: &types.GlobalTableDescription{
			GlobalTableName:   &name,
			GlobalTableArn:    &gt.GlobalTableArn,
			GlobalTableStatus: types.GlobalTableStatusActive,
			CreationDateTime:  &gt.CreationDateTime,
			ReplicationGroup:  sdkReplicas,
		},
	}, nil
}

// findSourceTableLocked returns the first existing Table for the given name across regions.
// Must be called with db.mu held for reading.
func (db *InMemoryDB) findSourceTableLocked(name string, regions []string) *Table {
	for _, region := range regions {
		if t, tableExists := db.tables.Get(tableKey(region, name)); tableExists {
			return t
		}
	}

	return nil
}

// applyGlobalTableReplicaUpdate processes a single Create or Delete replica update.
// Must be called with db.mu held for writing.
func (db *InMemoryDB) applyGlobalTableReplicaUpdate(
	name string,
	gt *StoredGlobalTable,
	update types.ReplicaUpdate,
	source *Table,
) error {
	switch {
	case update.Create != nil:
		return db.applyGlobalTableReplicaCreate(
			name,
			gt,
			ptrconv.String(update.Create.RegionName),
			source,
		)
	case update.Delete != nil:
		return db.applyGlobalTableReplicaDelete(name, gt, ptrconv.String(update.Delete.RegionName))
	}

	return nil
}

// applyGlobalTableReplicaCreate adds a new region to a global table.
// Must be called with db.mu held for writing.
func (db *InMemoryDB) applyGlobalTableReplicaCreate(
	name string,
	gt *StoredGlobalTable,
	regionName string,
	source *Table,
) error {
	if regionName == "" {
		return NewValidationException("RegionName is required for Create action")
	}

	if !slices.Contains(gt.ReplicationGroup, regionName) {
		gt.ReplicationGroup = append(gt.ReplicationGroup, regionName)
	}

	if existing, tableExists := db.tables.Get(tableKey(regionName, name)); !tableExists {
		replica := db.buildReplicaTableLocked(name, regionName, source)
		replica.GlobalTableName = name
		db.tables.Put(replica)
	} else {
		existing.GlobalTableName = name
	}

	return nil
}

// applyGlobalTableReplicaDelete removes a region from a global table.
// Must be called with db.mu held for writing.
func (db *InMemoryDB) applyGlobalTableReplicaDelete(
	name string,
	gt *StoredGlobalTable,
	regionName string,
) error {
	if regionName == "" {
		return NewValidationException("RegionName is required for Delete action")
	}

	remaining := make([]string, 0, len(gt.ReplicationGroup))
	for _, r := range gt.ReplicationGroup {
		if r != regionName {
			remaining = append(remaining, r)
		}
	}

	gt.ReplicationGroup = remaining

	db.tables.Delete(tableKey(regionName, name))

	return nil
}

// rebuildGlobalTableReplicasLocked refreshes the Replicas field on every regional Table entry.
// Must be called with db.mu held for writing.
func (db *InMemoryDB) rebuildGlobalTableReplicasLocked(name string, regions []string) {
	allReplicas := buildAllReplicas(regions)
	for _, region := range regions {
		if t, tableExists := db.tables.Get(tableKey(region, name)); tableExists {
			t.Replicas = buildReplicasExcluding(allReplicas, region)
		}
	}
}

// buildReplicaTableLocked creates or returns a Table for a new global table region.
// Must be called with db.mu held (write).
func (db *InMemoryDB) buildReplicaTableLocked(name, region string, source *Table) *Table {
	if source != nil {
		return cloneTableSchema(source, name, region, db.accountID)
	}

	t := &Table{
		Name:             name,
		Status:           statusActive,
		Items:            make([]map[string]any, 0),
		TableID:          uuid.New().String(),
		CreationDateTime: time.Now(),
		TableArn:         arn.Build("dynamodb", region, db.accountID, "table/"+name),
	}
	t.mu = newTableMutex(name)
	t.initializeIndexes()

	return t
}

// sortedGlobalTableNames returns sorted global table names starting after startName.
func sortedGlobalTableNames(tables *store.Table[StoredGlobalTable], startName string) []string {
	all := tables.All()
	names := make([]string, 0, len(all))
	for _, gt := range all {
		names = append(names, gt.GlobalTableName)
	}
	sort.Strings(names)

	if startName == "" {
		return names
	}

	for i, name := range names {
		if name > startName {
			return names[i:]
		}
	}

	return nil
}

// filterGlobalTables converts stored global tables to SDK types, applying an optional region filter.
func filterGlobalTables(
	tables *store.Table[StoredGlobalTable],
	names []string,
	regionFilter string,
) []types.GlobalTable {
	filtered := make([]types.GlobalTable, 0, len(names))

	for _, name := range names {
		gt, ok := tables.Get(name)
		if !ok {
			continue
		}

		if regionFilter != "" && !slices.Contains(gt.ReplicationGroup, regionFilter) {
			continue
		}

		replicas := make([]types.Replica, 0, len(gt.ReplicationGroup))
		for _, region := range gt.ReplicationGroup {
			replicas = append(replicas, types.Replica{RegionName: &region})
		}

		filtered = append(filtered, types.GlobalTable{
			GlobalTableName:  &name,
			ReplicationGroup: replicas,
		})
	}

	return filtered
}

// defaultListGlobalTablesLimit is ListGlobalTablesInput.Limit's documented
// default when the caller omits it (api_op_ListGlobalTables.go:35: "if the
// parameter is not specified, DynamoDB defaults to 100").
const defaultListGlobalTablesLimit = 100

// applyGlobalTableLimit applies a page size limit to the result set, falling
// back to defaultListGlobalTablesLimit when the caller didn't specify one.
// Returns the (possibly truncated) list and an optional cursor for the next page.
func applyGlobalTableLimit(list []types.GlobalTable, limit *int32) ([]types.GlobalTable, *string) {
	n := defaultListGlobalTablesLimit
	if limit != nil {
		n = int(*limit)
	}

	if n >= len(list) {
		return list, nil
	}

	if n <= 0 {
		return []types.GlobalTable{}, nil
	}

	last := *list[n-1].GlobalTableName

	return list[:n], &last
}

// --- UpdateGlobalTableSettings ---

// UpdateGlobalTableSettings persists global and per-replica billing/throughput settings
// and returns the updated state for each replica.
func (db *InMemoryDB) UpdateGlobalTableSettings(
	_ context.Context,
	input *dynamodb.UpdateGlobalTableSettingsInput,
) (*dynamodb.UpdateGlobalTableSettingsOutput, error) {
	if input.GlobalTableName == nil || *input.GlobalTableName == "" {
		return nil, NewValidationException("GlobalTableName is required")
	}

	name := *input.GlobalTableName

	snap, exists := db.updateGlobalTableSettingsLocked(name, input)
	if !exists {
		return nil, &Error{
			Type:    errGlobalTableNotFoundType,
			Message: fmt.Sprintf("Global table with name %s not found", name),
		}
	}

	effectiveBilling := types.BillingModePayPerRequest
	if snap.billingMode != "" {
		effectiveBilling = types.BillingMode(snap.billingMode)
	}

	replicas := make([]types.ReplicaSettingsDescription, 0, len(snap.replicationGroup))
	for _, region := range snap.replicationGroup {
		replicas = append(replicas, buildGlobalTableReplicaDesc(region, effectiveBilling, snap))
	}

	return &dynamodb.UpdateGlobalTableSettingsOutput{
		GlobalTableName: &name,
		ReplicaSettings: replicas,
	}, nil
}

// globalTableSettingsSnapshot is a copy of the StoredGlobalTable fields
// UpdateGlobalTableSettings needs to build its response, taken under
// db.mu.Lock so the response-building code below can run lock-free.
type globalTableSettingsSnapshot struct {
	writeCapacityUnits          *int64
	writeCapacityAutoScaling    *autoScalingThroughput
	replicaSettings             map[string]*StoredReplicaSettings
	gsiWriteCapacityAutoScaling map[string]*autoScalingThroughput
	billingMode                 string
	replicationGroup            []string
}

// updateGlobalTableSettingsLocked applies the UpdateGlobalTableSettings
// mutation and snapshots the resulting state, all under a single
// defer-protected db.mu.Lock. Returns exists=false if the named global table
// does not exist.
func (db *InMemoryDB) updateGlobalTableSettingsLocked(
	name string,
	input *dynamodb.UpdateGlobalTableSettingsInput,
) (globalTableSettingsSnapshot, bool) {
	db.mu.Lock("UpdateGlobalTableSettings")
	defer db.mu.Unlock()

	gt, exists := db.globalTables.Get(name)
	if !exists {
		return globalTableSettingsSnapshot{}, false
	}

	applyGlobalTableSettingsMutation(gt, input)

	replicationGroup := make([]string, len(gt.ReplicationGroup))
	copy(replicationGroup, gt.ReplicationGroup)

	return globalTableSettingsSnapshot{
		billingMode:                 gt.BillingMode,
		writeCapacityUnits:          gt.WriteCapacityUnits,
		writeCapacityAutoScaling:    gt.WriteCapacityAutoScaling,
		replicationGroup:            replicationGroup,
		replicaSettings:             gt.ReplicaSettings,
		gsiWriteCapacityAutoScaling: gt.GSIWriteCapacityAutoScaling,
	}, true
}

// applyGlobalTableSettingsMutation mutates gt with billing mode, write capacity, and
// per-replica setting changes from input.
func applyGlobalTableSettingsMutation(
	gt *StoredGlobalTable,
	input *dynamodb.UpdateGlobalTableSettingsInput,
) {
	if string(input.GlobalTableBillingMode) != "" {
		gt.BillingMode = string(input.GlobalTableBillingMode)
	}

	if input.GlobalTableProvisionedWriteCapacityUnits != nil {
		v := *input.GlobalTableProvisionedWriteCapacityUnits
		gt.WriteCapacityUnits = &v
	}

	if input.GlobalTableProvisionedWriteCapacityAutoScalingSettingsUpdate != nil {
		gt.WriteCapacityAutoScaling = throughputFromUpdate(
			input.GlobalTableProvisionedWriteCapacityAutoScalingSettingsUpdate,
		)
	}

	applyGlobalTableGSISettingsUpdates(gt, input.GlobalTableGlobalSecondaryIndexSettingsUpdate)
	applyReplicaSettingsUpdates(gt, input.ReplicaSettingsUpdate)
}

// applyGlobalTableGSISettingsUpdates persists the global (not per-replica)
// write-capacity autoscaling settings for each named GSI, keyed by index name.
func applyGlobalTableGSISettingsUpdates(
	gt *StoredGlobalTable,
	updates []types.GlobalTableGlobalSecondaryIndexSettingsUpdate,
) {
	for _, gu := range updates {
		if gu.IndexName == nil || gu.ProvisionedWriteCapacityAutoScalingSettingsUpdate == nil {
			continue
		}
		if gt.GSIWriteCapacityAutoScaling == nil {
			gt.GSIWriteCapacityAutoScaling = make(map[string]*autoScalingThroughput)
		}
		gt.GSIWriteCapacityAutoScaling[*gu.IndexName] = throughputFromUpdate(
			gu.ProvisionedWriteCapacityAutoScalingSettingsUpdate,
		)
	}
}

// applyReplicaSettingsUpdates persists per-replica billing, throughput, and GSI changes onto gt.
func applyReplicaSettingsUpdates(gt *StoredGlobalTable, updates []types.ReplicaSettingsUpdate) {
	if len(updates) == 0 {
		return
	}

	if gt.ReplicaSettings == nil {
		gt.ReplicaSettings = make(map[string]*StoredReplicaSettings)
	}

	for _, ru := range updates {
		applySingleReplicaSettingsUpdate(gt, ru)
	}
}

func applySingleReplicaSettingsUpdate(gt *StoredGlobalTable, ru types.ReplicaSettingsUpdate) {
	if ru.RegionName == nil {
		return
	}

	region := *ru.RegionName
	rs := gt.ReplicaSettings[region]
	if rs == nil {
		rs = &StoredReplicaSettings{}
		gt.ReplicaSettings[region] = rs
	}

	if string(ru.ReplicaTableClass) != "" {
		rs.TableClass = string(ru.ReplicaTableClass)
	}

	if ru.ReplicaProvisionedReadCapacityUnits != nil {
		v := *ru.ReplicaProvisionedReadCapacityUnits
		rs.ReadCapacityUnits = &v
	}

	if ru.ReplicaProvisionedReadCapacityAutoScalingSettingsUpdate != nil {
		rs.ReadCapacityAutoScaling = throughputFromUpdate(
			ru.ReplicaProvisionedReadCapacityAutoScalingSettingsUpdate,
		)
	}

	applyGSISettingsUpdates(rs, ru.ReplicaGlobalSecondaryIndexSettingsUpdate)
}

func applyGSISettingsUpdates(
	rs *StoredReplicaSettings,
	updates []types.ReplicaGlobalSecondaryIndexSettingsUpdate,
) {
	for _, gu := range updates {
		if gu.IndexName == nil {
			continue
		}
		if rs.GSISettings == nil {
			rs.GSISettings = make(map[string]*StoredReplicaGSISettings)
		}
		gsiName := *gu.IndexName
		grs := rs.GSISettings[gsiName]
		if grs == nil {
			grs = &StoredReplicaGSISettings{}
			rs.GSISettings[gsiName] = grs
		}
		if gu.ProvisionedReadCapacityUnits != nil {
			v := *gu.ProvisionedReadCapacityUnits
			grs.ReadCapacityUnits = &v
		}
		if gu.ProvisionedReadCapacityAutoScalingSettingsUpdate != nil {
			grs.ReadCapacityAutoScaling = throughputFromUpdate(
				gu.ProvisionedReadCapacityAutoScalingSettingsUpdate,
			)
		}
	}
}

// buildGlobalTableReplicaDesc constructs a ReplicaSettingsDescription for a
// single region. writeCapacityUnits is gt.WriteCapacityUnits (set from
// UpdateGlobalTableSettingsInput.GlobalTableProvisionedWriteCapacityUnits,
// which is a global -- not per-replica -- setting in the v1 API, so the same
// value applies to every replica).
// buildGlobalTableReplicaDesc builds one region's ReplicaSettingsDescription.
// Write-capacity settings (WCU + its autoscaling) are global-table-level, not
// per-replica, in the v1 API (api_op_UpdateGlobalTableSettings.go), so the
// same snap.writeCapacityUnits/writeCapacityAutoScaling/
// gsiWriteCapacityAutoScaling values apply uniformly to every replica.
func buildGlobalTableReplicaDesc(
	region string,
	billing types.BillingMode,
	snap globalTableSettingsSnapshot,
) types.ReplicaSettingsDescription {
	r := region
	desc := types.ReplicaSettingsDescription{
		RegionName:    &r,
		ReplicaStatus: types.ReplicaStatusActive,
		ReplicaBillingModeSummary: &types.BillingModeSummary{
			BillingMode: billing,
		},
	}

	if snap.writeCapacityUnits != nil {
		wcu := *snap.writeCapacityUnits
		desc.ReplicaProvisionedWriteCapacityUnits = &wcu
	}

	desc.ReplicaProvisionedWriteCapacityAutoScalingSettings = sdkAutoScalingSettingsDescription(
		snap.writeCapacityAutoScaling,
	)

	rs, ok := snap.replicaSettings[region]
	if !ok || rs == nil {
		return desc
	}

	if rs.TableClass != "" {
		tc := types.TableClass(rs.TableClass)
		desc.ReplicaTableClassSummary = &types.TableClassSummary{TableClass: tc}
	}

	if rs.ReadCapacityUnits != nil {
		rcu := *rs.ReadCapacityUnits
		desc.ReplicaProvisionedReadCapacityUnits = &rcu
	}

	desc.ReplicaProvisionedReadCapacityAutoScalingSettings = sdkAutoScalingSettingsDescription(
		rs.ReadCapacityAutoScaling,
	)

	if len(rs.GSISettings) > 0 {
		gsiDescs := make([]types.ReplicaGlobalSecondaryIndexSettingsDescription, 0, len(rs.GSISettings))
		for idxName, grs := range rs.GSISettings {
			name := idxName
			gdesc := types.ReplicaGlobalSecondaryIndexSettingsDescription{
				IndexName:   &name,
				IndexStatus: types.IndexStatusActive,
			}
			if grs != nil && grs.ReadCapacityUnits != nil {
				rcu := *grs.ReadCapacityUnits
				gdesc.ProvisionedReadCapacityUnits = &rcu
			}
			if grs != nil {
				gdesc.ProvisionedReadCapacityAutoScalingSettings = sdkAutoScalingSettingsDescription(
					grs.ReadCapacityAutoScaling,
				)
			}
			if snap.writeCapacityUnits != nil {
				wcu := *snap.writeCapacityUnits
				gdesc.ProvisionedWriteCapacityUnits = &wcu
			}
			gdesc.ProvisionedWriteCapacityAutoScalingSettings = sdkAutoScalingSettingsDescription(
				snap.gsiWriteCapacityAutoScaling[idxName],
			)
			gsiDescs = append(gsiDescs, gdesc)
		}
		desc.ReplicaGlobalSecondaryIndexSettings = gsiDescs
	}

	return desc
}

func (db *InMemoryDB) replicaGSISettingsRLocked(
	region, tableName string,
	rs *StoredReplicaSettings,
	gsiWriteCapacityAutoScaling map[string]*autoScalingThroughput,
) []types.ReplicaGlobalSecondaryIndexSettingsDescription {
	tbl := db.getTableInRegionRLocked(region, tableName, "DescribeGlobalTableSettings.gsi")
	if tbl == nil {
		return nil
	}

	tbl.mu.RLock("DescribeGlobalTableSettings.gsi")
	defer tbl.mu.RUnlock()

	if len(tbl.GlobalSecondaryIndexes) == 0 {
		return nil
	}

	gsiDescs := make([]types.ReplicaGlobalSecondaryIndexSettingsDescription, 0, len(tbl.GlobalSecondaryIndexes))
	for _, gsi := range tbl.GlobalSecondaryIndexes {
		gsiName := gsi.IndexName
		var rcu, wcu int64
		if gsi.ProvisionedThroughput.ReadCapacityUnits != nil {
			rcu = *gsi.ProvisionedThroughput.ReadCapacityUnits
		}
		if gsi.ProvisionedThroughput.WriteCapacityUnits != nil {
			wcu = *gsi.ProvisionedThroughput.WriteCapacityUnits
		}
		var readAutoScaling *autoScalingThroughput
		if rs != nil && rs.GSISettings != nil {
			if grs, ok := rs.GSISettings[gsiName]; ok && grs != nil {
				if grs.ReadCapacityUnits != nil {
					rcu = *grs.ReadCapacityUnits
				}
				readAutoScaling = grs.ReadCapacityAutoScaling
			}
		}
		gsiDesc := types.ReplicaGlobalSecondaryIndexSettingsDescription{
			IndexName:                     &gsiName,
			IndexStatus:                   types.IndexStatusActive,
			ProvisionedReadCapacityUnits:  &rcu,
			ProvisionedWriteCapacityUnits: &wcu,
			ProvisionedReadCapacityAutoScalingSettings: sdkAutoScalingSettingsDescription(readAutoScaling),

			ProvisionedWriteCapacityAutoScalingSettings: sdkAutoScalingSettingsDescription(
				gsiWriteCapacityAutoScaling[gsiName],
			)}
		gsiDescs = append(gsiDescs, gsiDesc)
	}

	return gsiDescs
}
