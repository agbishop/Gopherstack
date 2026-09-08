// Package dynamodb implements the AWS DynamoDB mock service.
// replication.go propagates completed item writes to sibling global-table
// replicas, and provides the table-cloning/mutex-construction helpers used
// when a replica table is physically instantiated.
package dynamodb

import (
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

// --- Global table replication helpers ---

// cloneTableSchema creates a new empty Table in the target region with the same
// key schema, attribute definitions, and throughput as the source.
// The clone gets a fresh TableID, ARN, and empty Items slice.
func cloneTableSchema(src *Table, name, region, accountID string) *Table {
	src.mu.RLock("cloneTableSchema")
	defer src.mu.RUnlock()

	keySchema := make([]models.KeySchemaElement, len(src.KeySchema))
	copy(keySchema, src.KeySchema)

	attrDefs := make([]models.AttributeDefinition, len(src.AttributeDefinitions))
	copy(attrDefs, src.AttributeDefinitions)

	gsis := make([]models.GlobalSecondaryIndex, len(src.GlobalSecondaryIndexes))
	copy(gsis, src.GlobalSecondaryIndexes)

	lsis := make([]models.LocalSecondaryIndex, len(src.LocalSecondaryIndexes))
	copy(lsis, src.LocalSecondaryIndexes)

	t := &Table{
		Name:                      name,
		Status:                    statusActive,
		Items:                     make([]map[string]any, 0),
		itemSizes:                 make([]int, 0),
		totalItemSizeBytes:        0,
		TableID:                   uuid.New().String(),
		CreationDateTime:          time.Now(),
		TableArn:                  arn.Build("dynamodb", region, accountID, "table/"+name),
		KeySchema:                 keySchema,
		AttributeDefinitions:      attrDefs,
		GlobalSecondaryIndexes:    gsis,
		LocalSecondaryIndexes:     lsis,
		ProvisionedThroughput:     src.ProvisionedThroughput,
		TableClass:                src.TableClass,
		DeletionProtectionEnabled: src.DeletionProtectionEnabled,
	}
	t.mu = newTableMutex(name)
	t.initializeIndexes()

	return t
}

// newTableMutex creates a new lockmetrics.RWMutex for the given table name.
func newTableMutex(name string) *lockmetrics.RWMutex {
	return lockmetrics.New("ddb.table." + name)
}

// buildReplicasExcluding returns a slice of ReplicaDescriptions from allReplicas
// excluding the one for excludeRegion (so a table lists all other regions as its replicas).
func buildReplicasExcluding(
	all []models.ReplicaDescription,
	excludeRegion string,
) []models.ReplicaDescription {
	result := make([]models.ReplicaDescription, 0, len(all))

	for _, r := range all {
		if r.RegionName != excludeRegion {
			result = append(result, r)
		}
	}

	return result
}

// replicateItemMutation propagates a completed item write (PUT or DELETE) to all
// sibling replicas of a global table. It is called after the primary write succeeds
// and after the primary table's mutex has been released.
//
// For PUT: finalItem is the item that was written.
// For DELETE: finalItem is the item that was deleted (used to locate it by key).
//
// This simulates DynamoDB global table eventual consistency: every replica converges
// to the same data state, with the last writer winning.
func (db *InMemoryDB) replicateItemMutation(
	tableName string,
	globalTableName string,
	currentRegion string,
	finalItem map[string]any,
	op string,
) {
	if db.IsReplicationPaused(tableName) {
		return
	}

	// Look up global table metadata under read lock.
	gt, exists := db.getGlobalTableForReplicationRLocked(globalTableName)
	if !exists {
		return
	}

	for _, region := range gt.ReplicationGroup {
		if region == currentRegion {
			continue
		}

		db.applyMutationToReplica(tableName, region, finalItem, op)
	}
}

// getGlobalTableForReplicationRLocked returns the global table stored under
// globalTableName (and whether it exists) under a defer-protected db.mu.RLock.
func (db *InMemoryDB) getGlobalTableForReplicationRLocked(globalTableName string) (*StoredGlobalTable, bool) {
	db.mu.RLock("replicateItemMutation-gt")
	defer db.mu.RUnlock()

	return db.globalTables.Get(globalTableName)
}

// applyMutationToReplica applies a single item mutation (PUT or DELETE) to one
// regional replica table. Safe to call concurrently; acquires the replica's lock internally.
func (db *InMemoryDB) applyMutationToReplica(
	tableName string,
	region string,
	finalItem map[string]any,
	op string,
) {
	// Look up the replica table under a short read lock.
	replica := db.getTableInRegionRLocked(region, tableName, "applyMutationToReplica-lookup")
	if replica == nil {
		return
	}

	db.applyReplicaMutationLocked(replica, finalItem, op)
}

// applyReplicaMutationLocked applies the PUT/DELETE mutation to replica under
// a defer-protected replica.mu.Lock.
func (db *InMemoryDB) applyReplicaMutationLocked(replica *Table, finalItem map[string]any, op string) {
	replica.mu.Lock("applyMutationToReplica-mutate")
	defer replica.mu.Unlock()

	if op == replicationOpDelete {
		db.deleteReplicaItemByKey(replica, finalItem)
	} else {
		_, matchIdx := db.findMatchForPut(replica, finalItem)
		db.doPut(replica, deepCopyItem(finalItem), matchIdx)
	}
}

// deleteReplicaItemByKey locates an item in a replica by its key attributes and deletes it.
// Must be called with replica.mu held (write).
func (db *InMemoryDB) deleteReplicaItemByKey(replica *Table, keyItem map[string]any) {
	pkDef, skDef := getPKAndSK(replica.KeySchema)
	pkVal := BuildKeyString(keyItem, pkDef.AttributeName)

	matchIdx := db.resolveReplicaMatchIndex(replica, pkVal, skDef.AttributeName, keyItem)

	if matchIdx >= 0 {
		db.deleteItemAtIndex(replica, matchIdx)
	}
}

// resolveReplicaMatchIndex resolves the Items slice index for the given primary and
// optional sort key values in a replica table.
func (db *InMemoryDB) resolveReplicaMatchIndex(
	replica *Table,
	pkVal string,
	skAttr string,
	keyItem map[string]any,
) int {
	if skAttr != "" {
		skVal := BuildKeyString(keyItem, skAttr)
		if skMap, ok := replica.pkskIndex[pkVal]; ok {
			if idx, found := skMap[skVal]; found {
				return idx
			}
		}

		return -1
	}

	matchIdx, ok := replica.pkIndex[pkVal]
	if !ok {
		return -1
	}

	return matchIdx
}
