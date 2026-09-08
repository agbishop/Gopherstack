package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	errReplicaCreateRegionRequired = errors.New(
		"RegionName is required for ReplicaUpdates Create action",
	)
	errReplicaDeleteRegionRequired = errors.New(
		"RegionName is required for ReplicaUpdates Delete action",
	)
)

// getRegionFromContext extracts the region from the request context.
// Returns the default region if region is not found in context.
func getRegionFromContext(ctx context.Context, db *InMemoryDB) string {
	if region, ok := ctx.Value(regionContextKey{}).(string); ok && region != "" {
		return region
	}
	// Fall back to the central awsmeta identity before the backend default so the
	// region stays consistent with the rest of the stack.
	if region := awsmeta.Region(ctx); region != "" {
		return region
	}

	return db.defaultRegion
}

// accountFromContext returns the request's AWS account, preferring a per-request
// override carried on awsmeta (e.g. X-Amz-Account-Id) over the backend default.
// Falls back to db.accountID when awsmeta carries only the placeholder account.
func accountFromContext(ctx context.Context, db *InMemoryDB) string {
	if a := awsmeta.Account(ctx); a != "" && a != awsmeta.DefaultAccount {
		return a
	}

	return db.accountID
}

// throttleKey returns the throttler key for the given region and table.
func throttleKey(region, tableName string) string {
	return region + ":" + tableName
}

// CreateTableInRegion creates a DynamoDB table in the specified region, bypassing
// the HTTP-layer region extraction. The supplied region always takes precedence,
// even if the context already carries a region value. Useful for tests that need
// tables in non-default regions.
func (db *InMemoryDB) CreateTableInRegion(
	ctx context.Context,
	input *dynamodb.CreateTableInput,
	region string,
) (*dynamodb.CreateTableOutput, error) {
	return db.CreateTable(context.WithValue(ctx, regionContextKey{}, region), input)
}

// validateCreateTableInput validates a CreateTable request before any shared state
// is touched. It returns a validation error describing the first failure encountered.
func validateCreateTableInput(input *dynamodb.CreateTableInput) error {
	if input.BillingMode != "" &&
		input.BillingMode != types.BillingModeProvisioned &&
		input.BillingMode != types.BillingModePayPerRequest {
		return NewValidationException(fmt.Sprintf(
			"1 validation error detected: Value '%s' at 'billingMode' failed to satisfy constraint:"+
				" Member must satisfy enum value set: [PROVISIONED, PAY_PER_REQUEST]",
			input.BillingMode,
		))
	}

	if err := validateAttributeDefinitions(input); err != nil {
		return err
	}

	if err := validateCreateTableKeySchema(models.FromSDKKeySchema(input.KeySchema)); err != nil {
		return err
	}

	if err := validateProvisionedThroughput(input.ProvisionedThroughput, input.BillingMode); err != nil {
		return err
	}

	// Enforce GSI/LSI count limits before constructing the table.
	if err := validateGSICount(nil, len(input.GlobalSecondaryIndexes)); err != nil {
		return err
	}

	if err := validateGSIThroughput(input.GlobalSecondaryIndexes, input.BillingMode); err != nil {
		return err
	}

	if err := validateLSICount(models.FromSDKLocalSecondaryIndexes(input.LocalSecondaryIndexes)); err != nil {
		return err
	}

	return nil
}

func (db *InMemoryDB) CreateTable(
	ctx context.Context,
	input *dynamodb.CreateTableInput,
) (*dynamodb.CreateTableOutput, error) {
	tableName := aws.ToString(input.TableName)
	if tableName == "" {
		return nil, NewValidationException("Table name is required")
	}

	region := getRegionFromContext(ctx, db)

	// Pre-lock: validate input and construct the table object. These operations
	// touch no shared state and can run concurrently with other requests. Building
	// the struct before the lock keeps the critical section to a handful of map ops,
	// significantly reducing contention when thousands of tables are created together.
	if err := validateCreateTableInput(input); err != nil {
		return nil, err
	}

	newTable := newTableFromCreateInput(tableName, input)
	newTable.TableID = uuid.New().String()
	newTable.CreationDateTime = time.Now().UTC()
	newTable.TableArn = arn.Build("dynamodb", region, db.accountID, "table/"+tableName)

	if input.StreamSpecification != nil && aws.ToBool(input.StreamSpecification.StreamEnabled) {
		streamCreatedAt := newTable.CreationDateTime
		newTable.StreamsEnabled = true
		newTable.StreamViewType = string(input.StreamSpecification.StreamViewType)
		newTable.StreamCreatedAt = streamCreatedAt
		newTable.StreamARN = db.buildStreamARNInRegion(tableName, region, streamCreatedAt)
		// Initialize the first shard so DescribeStream/GetShardIterator work immediately.
		newTable.StreamShards = []StreamShard{
			{
				ShardID:             streamShardID,
				StartingSequenceNum: 1,
			},
		}
	}

	// Set initial table status based on createDelay setting.
	if db.createDelay > 0 {
		newTable.Status = string(types.TableStatusCreating)

		newTable.activateTimer = time.AfterFunc(db.createDelay, func() {
			activateTableLocked(newTable)
		})
	} else {
		newTable.Status = string(types.TableStatusActive)
	}

	// Minimal critical section: check for duplicate, then insert into the shared maps.
	if !db.insertNewTableLocked(region, tableName, newTable) {
		// Release resources we allocated before discovering the duplicate.
		stopTableTimers(newTable)
		newTable.mu.Close()

		return nil, NewResourceInUseException("table already exists: " + tableName)
	}

	// Throttler has its own internal lock; call after releasing db.mu to keep the
	// critical section above as short as possible.
	rcu := int64(newTable.ProvisionedThroughput.ReadCapacityUnits)
	wcu := int64(newTable.ProvisionedThroughput.WriteCapacityUnits)
	db.throttler.SetTableCapacity(throttleKey(region, tableName), rcu, wcu)

	return buildCreateTableOutput(input, newTable), nil
}

// activateTableLocked flips a newly-created table's status to ACTIVE under a
// defer-protected table.mu.Lock. Used as the CreateTable activation timer's
// callback.
func activateTableLocked(newTable *Table) {
	newTable.mu.Lock("activate")
	defer newTable.mu.Unlock()

	newTable.Status = string(types.TableStatusActive)
}

// insertNewTableLocked inserts newTable into db.tables (and the stream ARN
// index, if applicable) under a defer-protected db.mu.Lock, unless a table
// already exists at the same key -- in which case it makes no changes and
// returns false so the caller can release newTable's own resources outside
// this lock.
func (db *InMemoryDB) insertNewTableLocked(region, tableName string, newTable *Table) bool {
	db.mu.Lock("CreateTable")
	defer db.mu.Unlock()

	if _, exists := db.tables.Get(tableKey(region, tableName)); exists {
		return false
	}

	db.tables.Put(newTable)
	newTable.kinesisEmitter = db.kinesisEmitter

	if newTable.StreamARN != "" {
		db.streamARNIndex.Put(newTable)
	}

	return true
}

// newTableFromCreateInput allocates and initialises a Table from a CreateTable request.
func newTableFromCreateInput(tableName string, input *dynamodb.CreateTableInput) *Table {
	t := &Table{
		Name:                   tableName,
		KeySchema:              models.FromSDKKeySchema(input.KeySchema),
		AttributeDefinitions:   models.FromSDKAttributeDefinitions(input.AttributeDefinitions),
		GlobalSecondaryIndexes: models.FromSDKGlobalSecondaryIndexes(input.GlobalSecondaryIndexes),
		LocalSecondaryIndexes:  models.FromSDKLocalSecondaryIndexes(input.LocalSecondaryIndexes),
		Items:                  make([]map[string]any, 0),
		itemSizes:              make([]int, 0),
		mu:                     lockmetrics.New("ddb.table." + tableName),
		ProvisionedThroughput: models.ProvisionedThroughputDescription{
			ReadCapacityUnits:  models.DefaultReadCapacity,
			WriteCapacityUnits: models.DefaultWriteCapacity,
		},
	}

	if pt := input.ProvisionedThroughput; pt != nil {
		if pt.ReadCapacityUnits != nil {
			t.ProvisionedThroughput.ReadCapacityUnits = int(*pt.ReadCapacityUnits)
		}

		if pt.WriteCapacityUnits != nil {
			t.ProvisionedThroughput.WriteCapacityUnits = int(*pt.WriteCapacityUnits)
		}
	}

	if input.TableClass != "" {
		t.TableClass = string(input.TableClass)
	}
	t.DeletionProtectionEnabled = aws.ToBool(input.DeletionProtectionEnabled)

	if input.BillingMode == types.BillingModePayPerRequest {
		t.BillingMode = string(types.BillingModePayPerRequest)
	} else {
		t.BillingMode = string(types.BillingModeProvisioned)
	}

	if odt := input.OnDemandThroughput; odt != nil {
		t.OnDemandMaxReadRRU = odt.MaxReadRequestUnits
		t.OnDemandMaxWriteRRU = odt.MaxWriteRequestUnits
	}

	if input.SSESpecification != nil {
		t.SSEEnabled = input.SSESpecification.Enabled == nil ||
			aws.ToBool(input.SSESpecification.Enabled)
		if t.SSEEnabled {
			t.SSEType = string(input.SSESpecification.SSEType)
			if t.SSEType == "" {
				t.SSEType = string(types.SSETypeKms)
			}
			t.SSEKMSMasterKeyArn = aws.ToString(input.SSESpecification.KMSMasterKeyId)
		}
	}

	if len(input.Tags) > 0 {
		t.Tags = tags.New("dynamodb.table." + tableName + ".tags")
		for _, tag := range input.Tags {
			t.Tags.Set(aws.ToString(tag.Key), aws.ToString(tag.Value))
		}
	}

	t.initializeIndexes()

	return t
}

func validateAttributeDefinitions(input *dynamodb.CreateTableInput) error {
	defs := make(map[string]struct{})
	for _, ad := range input.AttributeDefinitions {
		defs[aws.ToString(ad.AttributeName)] = struct{}{}
	}

	referenced := make(map[string]struct{})
	// Check Table KeySchema
	for _, k := range input.KeySchema {
		name := aws.ToString(k.AttributeName)
		referenced[name] = struct{}{}
		if _, ok := defs[name]; !ok {
			return NewValidationException(
				fmt.Sprintf(
					"Parameter AttributeDefinitions does not contain definition for attribute %s which is used in KeySchema",
					name,
				),
			)
		}
	}

	// Check GSI KeySchema
	for _, gsi := range input.GlobalSecondaryIndexes {
		for _, k := range gsi.KeySchema {
			name := aws.ToString(k.AttributeName)
			referenced[name] = struct{}{}
			if _, ok := defs[name]; !ok {
				return NewValidationException(fmt.Sprintf(
					"Parameter AttributeDefinitions does not contain definition for attribute"+
						" %s which is used in GlobalSecondaryIndexes",
					name,
				))
			}
		}
	}

	// Check LSI KeySchema
	for _, lsi := range input.LocalSecondaryIndexes {
		for _, k := range lsi.KeySchema {
			name := aws.ToString(k.AttributeName)
			referenced[name] = struct{}{}
			if _, ok := defs[name]; !ok {
				return NewValidationException(fmt.Sprintf(
					"Parameter AttributeDefinitions does not contain definition for attribute"+
						" %s which is used in LocalSecondaryIndexes",
					name,
				))
			}
		}
	}

	// All defs must be referenced
	for name := range defs {
		if _, ok := referenced[name]; !ok {
			return NewValidationException(
				"Parameter AttributeDefinitions contains unused attribute: " + name,
			)
		}
	}

	return nil
}

// buildCreateTableOutput constructs the wire response for CreateTable. t is
// already visible in db.tables by the time this runs (CreateTable inserts it
// before releasing db.mu), and its activateTimer (when createDelay > 0) can
// fire concurrently on another goroutine and flip t.Status under t.mu at any
// moment -- so every field read here goes through t.mu, matching the pattern
// snapshotTable/DescribeTable already use for the same reason.
func buildCreateTableOutput(
	input *dynamodb.CreateTableInput,
	t *Table,
) *dynamodb.CreateTableOutput {
	gsiDescs := make([]models.GlobalSecondaryIndexDescription, len(input.GlobalSecondaryIndexes))
	for i, gsi := range input.GlobalSecondaryIndexes {
		gsiDescs[i] = models.GlobalSecondaryIndexDescription{
			IndexName:  aws.ToString(gsi.IndexName),
			IndexArn:   indexArn(t.TableArn, aws.ToString(gsi.IndexName)),
			KeySchema:  models.FromSDKKeySchema(gsi.KeySchema),
			Projection: models.FromSDKProjection(gsi.Projection),
			ProvisionedThroughput: models.ProvisionedThroughputDescription{
				ReadCapacityUnits:  models.DefaultReadCapacity,
				WriteCapacityUnits: models.DefaultWriteCapacity,
			},
			IndexStatus: models.TableStatusActive,
		}
	}

	lsiDescs := make([]models.LocalSecondaryIndexDescription, len(input.LocalSecondaryIndexes))
	for i, lsi := range input.LocalSecondaryIndexes {
		lsiDescs[i] = models.LocalSecondaryIndexDescription{
			IndexName:  aws.ToString(lsi.IndexName),
			IndexArn:   indexArn(t.TableArn, aws.ToString(lsi.IndexName)),
			KeySchema:  models.FromSDKKeySchema(lsi.KeySchema),
			Projection: models.FromSDKProjection(lsi.Projection),
		}
	}

	rcu, wcu, tableStatus, keySchema, attrDefs, sseEnabled, sseType, sseKMSMasterKeyArn :=
		snapshotTableForCreateOutputRLocked(t)

	td := &types.TableDescription{
		TableName:              input.TableName,
		TableArn:               aws.String(t.TableArn),
		TableStatus:            tableStatus,
		KeySchema:              keySchema,
		AttributeDefinitions:   attrDefs,
		GlobalSecondaryIndexes: models.ToSDKGlobalSecondaryIndexDescriptions(gsiDescs),
		LocalSecondaryIndexes:  models.ToSDKLocalSecondaryIndexDescriptions(lsiDescs),
		ItemCount:              aws.Int64(0),
		ProvisionedThroughput: &types.ProvisionedThroughputDescription{
			ReadCapacityUnits:  &rcu,
			WriteCapacityUnits: &wcu,
		},
	}
	// t.TableID is assigned once at creation, before newTable is published to
	// db.tables, so it is immutable by the time buildCreateTableOutput runs --
	// safe to read directly, same as t.TableArn above. DescribeTable already
	// returns TableId (buildTableDescription); CreateTable's own response
	// dropped it despite computing the same value.
	if t.TableID != "" {
		td.TableId = aws.String(t.TableID)
	}
	applySSEDescription(td, sseEnabled, sseType, sseKMSMasterKeyArn)

	return &dynamodb.CreateTableOutput{TableDescription: td}
}

// snapshotTableForCreateOutputRLocked copies every field buildCreateTableOutput
// needs from t under a defer-protected table.mu.RLock, so a panic partway
// through can never leave table.mu read-locked forever.
func snapshotTableForCreateOutputRLocked(t *Table) (
	int64,
	int64,
	types.TableStatus,
	[]types.KeySchemaElement,
	[]types.AttributeDefinition,
	bool,
	string,
	string,
) {
	t.mu.RLock("buildCreateTableOutput")
	defer t.mu.RUnlock()

	rcu := int64(t.ProvisionedThroughput.ReadCapacityUnits)
	wcu := int64(t.ProvisionedThroughput.WriteCapacityUnits)

	tableStatus := types.TableStatus(t.Status)
	if tableStatus == "" {
		tableStatus = types.TableStatusActive
	}

	keySchema := models.ToSDKKeySchema(t.KeySchema)
	attrDefs := models.ToSDKAttributeDefinitions(t.AttributeDefinitions)

	return rcu, wcu, tableStatus, keySchema, attrDefs, t.SSEEnabled, t.SSEType, t.SSEKMSMasterKeyArn
}

func (db *InMemoryDB) DeleteTable(
	ctx context.Context,
	input *dynamodb.DeleteTableInput,
) (*dynamodb.DeleteTableOutput, error) {
	tableName := aws.ToString(input.TableName)
	if tableName == "" {
		return nil, NewValidationException("Table name is required")
	}

	region := getRegionFromContext(ctx, db)

	db.mu.Lock("DeleteTable")
	defer db.mu.Unlock()

	table, tableExists := db.tables.Get(tableKey(region, tableName))
	if !tableExists {
		return nil, NewResourceNotFoundException("table not found: " + tableName)
	}

	// AWS rejects DeleteTable when DeletionProtectionEnabled is true.
	// The protection must be disabled (via UpdateTable) before the table can be deleted.
	if table.DeletionProtectionEnabled {
		return nil, NewValidationException(
			"Table cannot be deleted while DeletionProtectionEnabled is set to True. " +
				"Update the table first to disable deletion protection.",
		)
	}

	table.mu.RLock("DeleteTable.statusCheck")
	status := types.TableStatus(table.Status)
	table.mu.RUnlock()

	if status == types.TableStatusCreating || status == types.TableStatusUpdating {
		return nil, NewResourceInUseException(
			"Attempt to change a resource which is still in use: Table is being created or updated: " + tableName,
		)
	}

	// Cancel any pending activation timer and any in-flight GSI lifecycle timers.
	// Stop() is called while db.mu is held, which prevents a concurrent CreateTable from
	// racing with us. If a timer has already fired and the callback is in progress, Stop()
	// returns false but the callback only writes table.Status or GSI.IndexStatus (both
	// guarded by table.mu) on an object that is about to move to deletingTables —
	// this is benign; the janitor will clean it up regardless.
	stopTableTimers(table)

	db.tables.Delete(tableKey(region, tableName))
	db.deletingTables.Put(table)
	db.throttler.DeleteTable(throttleKey(region, tableName))

	// Remove the deleted table's region from its global table's ReplicationGroup.
	// If no replicas remain, remove the global table entry entirely.
	if table.GlobalTableName != "" {
		db.removeGlobalTableReplicaLocked(table.GlobalTableName, region)
	}

	// Remove from stream ARN reverse index.
	if table.StreamARN != "" {
		db.streamARNIndex.Delete(table.StreamARN)
	}

	// Clear any FIS global-table-pause-replication fault. TableArn is
	// deterministic from the table name, so a same-named recreated table
	// would otherwise inherit a stale paused-replication state.
	delete(db.fisReplicationPaused, table.TableArn)

	// The table is already unlinked from db.tables, but a caller that grabbed
	// this same *Table just before this delete can still be actively writing to
	// it under table.mu without re-checking db.tables -- reading these fields
	// here without table.mu would be a real data race, so take a read lock for
	// the snapshot, consistent with this backend's db.mu -> table.mu order.
	gsis, keySchema, attrDefs, itemCountSnapshot := snapshotTableForDeleteOutputRLocked(table)

	gsiDescs := make([]models.GlobalSecondaryIndexDescription, len(gsis))
	for i, gsi := range gsis {
		rc := int64(models.DefaultReadCapacity)
		wc := int64(models.DefaultWriteCapacity)
		if gsi.ProvisionedThroughput.ReadCapacityUnits != nil {
			rc = *gsi.ProvisionedThroughput.ReadCapacityUnits
		}
		if gsi.ProvisionedThroughput.WriteCapacityUnits != nil {
			wc = *gsi.ProvisionedThroughput.WriteCapacityUnits
		}
		gsiDescs[i] = models.GlobalSecondaryIndexDescription{
			IndexName:  gsi.IndexName,
			IndexArn:   indexArn(table.TableArn, gsi.IndexName),
			KeySchema:  gsi.KeySchema,
			Projection: gsi.Projection,
			ProvisionedThroughput: models.ProvisionedThroughputDescription{
				ReadCapacityUnits:  int(rc),
				WriteCapacityUnits: int(wc),
			},
			IndexStatus: "DELETING",
			ItemCount:   itemCountSnapshot,
		}
	}

	sdkGSIs := models.ToSDKGlobalSecondaryIndexDescriptions(gsiDescs)
	sdkKeySchema := models.ToSDKKeySchema(keySchema)
	sdkAttrDefs := models.ToSDKAttributeDefinitions(attrDefs)
	itemCount := int64(itemCountSnapshot)

	return &dynamodb.DeleteTableOutput{
		TableDescription: &types.TableDescription{
			TableName:              input.TableName,
			TableArn:               aws.String(table.TableArn),
			TableStatus:            types.TableStatusDeleting,
			KeySchema:              sdkKeySchema,
			AttributeDefinitions:   sdkAttrDefs,
			GlobalSecondaryIndexes: sdkGSIs,
			ItemCount:              &itemCount,
		},
	}, nil
}

// snapshotTableForDeleteOutputRLocked copies the GSI list, key schema,
// attribute definitions, and item count needed to build DeleteTable's
// response, under a defer-protected table.mu.RLock. See the DeleteTable call
// site comment for why this snapshot must be taken under table.mu even though
// the table has already been unlinked from db.tables.
func snapshotTableForDeleteOutputRLocked(table *Table) (
	[]models.GlobalSecondaryIndex,
	[]models.KeySchemaElement,
	[]models.AttributeDefinition,
	int,
) {
	table.mu.RLock("DeleteTable.snapshot")
	defer table.mu.RUnlock()

	gsis := make([]models.GlobalSecondaryIndex, len(table.GlobalSecondaryIndexes))
	copy(gsis, table.GlobalSecondaryIndexes)
	keySchema := make([]models.KeySchemaElement, len(table.KeySchema))
	copy(keySchema, table.KeySchema)
	attrDefs := make([]models.AttributeDefinition, len(table.AttributeDefinitions))
	copy(attrDefs, table.AttributeDefinitions)

	return gsis, keySchema, attrDefs, len(table.Items)
}

// removeGlobalTableReplicaLocked removes a region from a global table's ReplicationGroup.
// If no replicas remain after removal, the global table entry itself is deleted.
// Must be called with db.mu held for writing.
func (db *InMemoryDB) removeGlobalTableReplicaLocked(globalTableName, region string) {
	gt, gtExists := db.globalTables.Get(globalTableName)
	if !gtExists {
		return
	}

	remaining := make([]string, 0, len(gt.ReplicationGroup))
	for _, r := range gt.ReplicationGroup {
		if r != region {
			remaining = append(remaining, r)
		}
	}

	if len(remaining) == 0 {
		db.globalTables.Delete(globalTableName)
	} else {
		gt.ReplicationGroup = remaining
	}
}

// indexArn builds a secondary index's ARN from its table's ARN, matching
// AWS's "arn:.../table/<name>/index/<indexName>" convention (confirmed by
// the field's presence on both GlobalSecondaryIndexDescription and
// LocalSecondaryIndexDescription -- dynamodb@v1.63.1 types/types.go:1676
// and :2292 -- there's no other index-scoped ARN in the DynamoDB namespace).
func indexArn(tableArn, indexName string) string {
	if tableArn == "" || indexName == "" {
		return ""
	}

	return tableArn + "/index/" + indexName
}

func buildGSIDescriptions(
	gsiList []models.GlobalSecondaryIndex,
	itemCount int64,
	tableArn string,
) []models.GlobalSecondaryIndexDescription {
	gsiDescs := make([]models.GlobalSecondaryIndexDescription, len(gsiList))
	for i, gsi := range gsiList {
		rc := int64(models.DefaultReadCapacity)
		wc := int64(models.DefaultWriteCapacity)
		if gsi.ProvisionedThroughput.ReadCapacityUnits != nil {
			rc = *gsi.ProvisionedThroughput.ReadCapacityUnits
		}
		if gsi.ProvisionedThroughput.WriteCapacityUnits != nil {
			wc = *gsi.ProvisionedThroughput.WriteCapacityUnits
		}

		status := gsi.IndexStatus
		if status == "" {
			status = models.TableStatusActive
		}

		gsiDescs[i] = models.GlobalSecondaryIndexDescription{
			IndexName:  gsi.IndexName,
			IndexArn:   indexArn(tableArn, gsi.IndexName),
			KeySchema:  gsi.KeySchema,
			Projection: gsi.Projection,
			ProvisionedThroughput: models.ProvisionedThroughputDescription{
				ReadCapacityUnits:  int(rc),
				WriteCapacityUnits: int(wc),
			},
			IndexStatus: status,
			ItemCount:   int(itemCount),
		}
	}

	return gsiDescs
}

func buildLSIDescriptions(
	lsiList []models.LocalSecondaryIndex,
	tableArn string,
) []models.LocalSecondaryIndexDescription {
	lsiDescs := make([]models.LocalSecondaryIndexDescription, len(lsiList))
	for i, lsi := range lsiList {
		lsiDescs[i] = models.LocalSecondaryIndexDescription{
			IndexName:      lsi.IndexName,
			IndexArn:       indexArn(tableArn, lsi.IndexName),
			KeySchema:      lsi.KeySchema,
			Projection:     lsi.Projection,
			IndexSizeBytes: 0,
			ItemCount:      0,
		}
	}

	return lsiDescs
}

func (db *InMemoryDB) DescribeTable(
	ctx context.Context,
	input *dynamodb.DescribeTableInput,
) (*dynamodb.DescribeTableOutput, error) {
	tableName := aws.ToString(input.TableName)

	region := getRegionFromContext(ctx, db)

	table := db.getTableInRegionRLocked(region, tableName, "DescribeTable")
	if table == nil {
		return nil, NewResourceNotFoundException("table not found: " + tableName)
	}

	tableDesc := buildTableDescription(input.TableName, table)

	return &dynamodb.DescribeTableOutput{Table: tableDesc}, nil
}

// tableSnapshot is a lock-free snapshot of table metadata for building SDK
// responses. Field order is govet/fieldalignment-tuned.
type tableSnapshot struct {
	creationDT                time.Time
	onDemandMaxReadRRU        *int64
	onDemandMaxWriteRRU       *int64
	tableClass                string
	streamARN                 string
	streamViewType            string
	tableID                   string
	globalTableName           string
	billingMode               string
	sseType                   string
	sseKMSMasterKeyArn        string
	tableArn                  string
	tableStatus               types.TableStatus
	lsiList                   []models.LocalSecondaryIndex
	attrDefs                  []models.AttributeDefinition
	gsiList                   []models.GlobalSecondaryIndex
	keySchema                 []models.KeySchemaElement
	replicaList               []models.ReplicaDescription
	pt                        models.ProvisionedThroughputDescription
	itemCount                 int64
	itemSizeBytes             int64
	sseEnabled                bool
	streamsEnabled            bool
	deletionProtectionEnabled bool
}

func snapshotTable(table *Table) tableSnapshot {
	table.mu.RLock("DescribeTable")
	defer table.mu.RUnlock()

	s := tableSnapshot{
		keySchema: make([]models.KeySchemaElement, len(table.KeySchema)),
		attrDefs: make(
			[]models.AttributeDefinition,
			len(table.AttributeDefinitions),
		),
		gsiList: make(
			[]models.GlobalSecondaryIndex,
			len(table.GlobalSecondaryIndexes),
		),
		lsiList: make(
			[]models.LocalSecondaryIndex,
			len(table.LocalSecondaryIndexes),
		),
		replicaList:               make([]models.ReplicaDescription, len(table.Replicas)),
		itemCount:                 int64(len(table.Items)),
		itemSizeBytes:             estimateTableSizeBytes(table),
		pt:                        table.ProvisionedThroughput,
		tableStatus:               types.TableStatus(table.Status),
		tableArn:                  table.TableArn,
		tableID:                   table.TableID,
		creationDT:                table.CreationDateTime,
		streamARN:                 table.StreamARN,
		streamsEnabled:            table.StreamsEnabled,
		streamViewType:            table.StreamViewType,
		deletionProtectionEnabled: table.DeletionProtectionEnabled,
		tableClass:                table.TableClass,
		globalTableName:           table.GlobalTableName,
		billingMode:               table.BillingMode,
		sseEnabled:                table.SSEEnabled,
		sseType:                   table.SSEType,
		sseKMSMasterKeyArn:        table.SSEKMSMasterKeyArn,
		onDemandMaxReadRRU:        table.OnDemandMaxReadRRU,
		onDemandMaxWriteRRU:       table.OnDemandMaxWriteRRU,
	}
	copy(s.keySchema, table.KeySchema)
	copy(s.attrDefs, table.AttributeDefinitions)
	copy(s.gsiList, table.GlobalSecondaryIndexes)
	copy(s.lsiList, table.LocalSecondaryIndexes)
	copy(s.replicaList, table.Replicas)

	if s.tableStatus == "" {
		s.tableStatus = types.TableStatusActive
	}

	return s
}

// buildTableDescription constructs the SDK TableDescription for a DescribeTable response.
func buildTableDescription(tableName *string, table *Table) *types.TableDescription {
	s := snapshotTable(table)

	gsiDescs := buildGSIDescriptions(s.gsiList, s.itemCount, s.tableArn)
	lsiDescs := buildLSIDescriptions(s.lsiList, s.tableArn)

	rcu := int64(s.pt.ReadCapacityUnits)
	wcu := int64(s.pt.WriteCapacityUnits)

	tableSizeBytes := s.itemSizeBytes

	billingMode := types.BillingModeProvisioned
	if s.billingMode == string(types.BillingModePayPerRequest) {
		billingMode = types.BillingModePayPerRequest
	}

	td := &types.TableDescription{
		TableName:                 tableName,
		TableStatus:               s.tableStatus,
		KeySchema:                 models.ToSDKKeySchema(s.keySchema),
		AttributeDefinitions:      models.ToSDKAttributeDefinitions(s.attrDefs),
		GlobalSecondaryIndexes:    models.ToSDKGlobalSecondaryIndexDescriptions(gsiDescs),
		LocalSecondaryIndexes:     models.ToSDKLocalSecondaryIndexDescriptions(lsiDescs),
		Replicas:                  toSDKReplicaDescriptions(s.replicaList),
		ItemCount:                 &s.itemCount,
		TableSizeBytes:            &tableSizeBytes,
		BillingModeSummary:        &types.BillingModeSummary{BillingMode: billingMode},
		DeletionProtectionEnabled: &s.deletionProtectionEnabled,
	}

	// Only populate ProvisionedThroughput for PROVISIONED billing mode.
	if billingMode == types.BillingModeProvisioned {
		td.ProvisionedThroughput = &types.ProvisionedThroughputDescription{
			ReadCapacityUnits:  &rcu,
			WriteCapacityUnits: &wcu,
		}
	}

	if s.onDemandMaxReadRRU != nil || s.onDemandMaxWriteRRU != nil {
		td.OnDemandThroughput = &types.OnDemandThroughput{
			MaxReadRequestUnits:  s.onDemandMaxReadRRU,
			MaxWriteRequestUnits: s.onDemandMaxWriteRRU,
		}
	}

	// Populate GlobalTableVersion for tables that are part of a global table.
	// AWS uses "2019.11.21" for Global Tables v2 (the version created by CreateGlobalTable v2 /
	// UpdateTable.ReplicaUpdates) and "2017.11.29" for the legacy API.
	// Our CreateGlobalTable follows the v1 API model; we use the v2 version for tables
	// promoted via UpdateTable.ReplicaUpdates (applyReplicaCreate). Either way, if the
	// table has replicas or is part of a global table, return the version string.
	if s.globalTableName != "" || len(s.replicaList) > 0 {
		gtv := "2019.11.21"
		td.GlobalTableVersion = &gtv
	}

	if s.tableClass != "" {
		td.TableClassSummary = &types.TableClassSummary{
			TableClass: types.TableClass(s.tableClass),
		}
	}

	if s.tableArn != "" {
		td.TableArn = &s.tableArn
	}
	if s.tableID != "" {
		td.TableId = &s.tableID
	}
	if !s.creationDT.IsZero() {
		td.CreationDateTime = &s.creationDT
	}

	applyStreamSpec(td, s.streamsEnabled, s.streamARN, s.streamViewType)
	applySSEDescription(td, s.sseEnabled, s.sseType, s.sseKMSMasterKeyArn)

	return td
}

// applySSEDescription populates td.SSEDescription from the table's stored SSE
// configuration. Real DynamoDB always returns an SSEDescription, even for
// tables that were created without SSESpecification — they report SSEType=AES256
// with an AWS-owned key (no ARN). When the user enables SSE-KMS we report the
// KMS key ARN. Status is always ENABLED in the mock (no rotation in flight).
func applySSEDescription(td *types.TableDescription, enabled bool, sseType, kmsKeyArn string) {
	desc := &types.SSEDescription{
		Status: types.SSEStatusEnabled,
	}

	if sseType == "" {
		if enabled {
			sseType = string(types.SSETypeKms)
		} else {
			sseType = string(types.SSETypeAes256)
		}
	}

	desc.SSEType = types.SSEType(sseType)

	if kmsKeyArn != "" {
		desc.KMSMasterKeyArn = &kmsKeyArn
	}
	td.SSEDescription = desc
}

// applyStreamSpec fills the stream-related fields of a TableDescription when streams are enabled.
func applyStreamSpec(td *types.TableDescription, enabled bool, streamARN, viewType string) {
	if !enabled || streamARN == "" {
		return
	}

	td.LatestStreamArn = &streamARN

	// LatestStreamLabel is the last path segment of the stream ARN (the timestamp portion).
	streamLabel := streamARN
	if idx := strings.LastIndex(streamARN, "/"); idx >= 0 {
		streamLabel = streamARN[idx+1:]
	}

	td.LatestStreamLabel = &streamLabel
	td.StreamSpecification = &types.StreamSpecification{
		StreamEnabled:  aws.Bool(true),
		StreamViewType: types.StreamViewType(viewType),
	}
}

// UpdateTable modifies a DynamoDB table's provisioned throughput, GSI list, stream spec, and replicas.
func (db *InMemoryDB) UpdateTable(
	ctx context.Context,
	input *dynamodb.UpdateTableInput,
) (*dynamodb.UpdateTableOutput, error) {
	tableName := aws.ToString(input.TableName)
	if tableName == "" {
		return nil, NewValidationException("Table name is required")
	}

	table, err := db.getTable(ctx, tableName)
	if err != nil {
		return nil, err
	}

	// Apply all table mutations under a single lock acquisition, then release
	// before updating db-level state (stream ARN index, throttler) to minimize the
	// table.mu critical section and avoid lock-ordering issues.
	var (
		oldStreamARN string
		newStreamARN string
		rcu          int64
		wcu          int64
		out          *dynamodb.UpdateTableOutput
		region       = getRegionFromContext(ctx, db)
	)

	if updateErr := db.applyUpdateTableLocked(
		table, tableName, region, input,
		&oldStreamARN, &newStreamARN, &rcu, &wcu, &out,
	); updateErr != nil {
		return nil, updateErr
	}

	// For Global Tables v2: create physical Table entries for each new replica region.
	// This must run outside table.mu (to avoid lock inversion with db.mu).
	if len(input.ReplicaUpdates) > 0 {
		db.applyReplicaTableEntries(tableName, region, table, input.ReplicaUpdates)
	}

	// Update throttler outside table.mu: SetTableCapacity takes its own internal
	// lock, so calling it inside the table lock would unnecessarily extend the
	// critical section and increase contention with concurrent reads/writes.
	db.throttler.SetTableCapacity(throttleKey(region, tableName), rcu, wcu)

	// Update the stream ARN reverse index under db.mu (after the table lock has been
	// released — never hold both table.mu and db.mu simultaneously to prevent deadlocks).
	if oldStreamARN != newStreamARN {
		updateStreamARNIndexLocked(db, table, oldStreamARN, newStreamARN)
	}

	return out, nil
}

// updateStreamARNIndexLocked removes oldARN (if set) and adds table under
// newARN (if set) to db.streamARNIndex, under a defer-protected db.mu.Lock.
func updateStreamARNIndexLocked(db *InMemoryDB, table *Table, oldARN, newARN string) {
	db.mu.Lock("UpdateTable.streamARNIndex")
	defer db.mu.Unlock()

	if oldARN != "" {
		db.streamARNIndex.Delete(oldARN)
	}
	if newARN != "" {
		db.streamARNIndex.Put(table)
	}
}

// applyUpdateTableLocked applies all table mutations under table.mu. It is extracted from
// UpdateTable to reduce cognitive complexity of the parent function.
// countUpdateTableMutations counts the mutually-exclusive UpdateTable mutation
// groups present in the input. AWS allows only one of modifying throughput
// or creating/removing a global secondary index per call.
//
// BillingMode shares ProvisionedThroughput's group because AWS requires them
// together: "When switching from pay-per-request to provisioned capacity, initial
// provisioned capacity values must be set" (api_op_UpdateTable.go:60-63, sdk
// v1.63.1).
func countUpdateTableMutations(input *dynamodb.UpdateTableInput) int {
	mutations := 0
	if input.ProvisionedThroughput != nil || input.BillingMode != "" {
		mutations++
	}

	if len(input.GlobalSecondaryIndexUpdates) > 0 {
		mutations++
	}

	return mutations
}

// validateUpdateTableMutation enforces the at-most-one-mutation rule and
// validates provisioned throughput against the effective billing mode.
func validateUpdateTableMutation(table *Table, input *dynamodb.UpdateTableInput) error {
	if countUpdateTableMutations(input) > 1 {
		return NewValidationException(
			"One or more parameter values were invalid: " +
				"Cannot perform more than one of the following operations at once: " +
				"Modify provisioned throughput settings, " +
				"Create or remove a global secondary index",
		)
	}

	if input.BillingMode == "" && input.ProvisionedThroughput == nil {
		return nil
	}

	billingMode := table.BillingMode
	if input.BillingMode != "" {
		billingMode = string(input.BillingMode)
	}

	return validateProvisionedThroughput(input.ProvisionedThroughput, types.BillingMode(billingMode))
}

func (db *InMemoryDB) applyUpdateTableLocked(
	table *Table,
	tableName string,
	region string,
	input *dynamodb.UpdateTableInput,
	oldStreamARN, newStreamARN *string,
	rcu, wcu *int64,
	out **dynamodb.UpdateTableOutput,
) error {
	if err := validateUpdateTableMutation(table, input); err != nil {
		return err
	}

	table.mu.Lock("UpdateTable")
	defer table.mu.Unlock()

	applyUpdateTableThroughput(table, input.ProvisionedThroughput)
	applyUpdateTableAttrDefs(table, input.AttributeDefinitions)

	if len(input.GlobalSecondaryIndexUpdates) > 0 {
		if gsiErr := db.applyGSIUpdates(table, input.GlobalSecondaryIndexUpdates); gsiErr != nil {
			return gsiErr
		}
	}

	*oldStreamARN, *newStreamARN = db.applyStreamSpec(
		table,
		tableName,
		input.StreamSpecification,
		region,
	)

	if replicaErr := applyReplicaUpdates(table, input.ReplicaUpdates); replicaErr != nil {
		return NewValidationException(replicaErr.Error())
	}

	if input.DeletionProtectionEnabled != nil {
		table.DeletionProtectionEnabled = *input.DeletionProtectionEnabled
	}

	if input.TableClass != "" {
		table.TableClass = string(input.TableClass)
	}

	if input.BillingMode != "" {
		table.BillingMode = string(input.BillingMode)
	}

	if input.SSESpecification != nil {
		if input.SSESpecification.Enabled == nil || aws.ToBool(input.SSESpecification.Enabled) {
			table.SSEEnabled = true
			table.SSEType = string(input.SSESpecification.SSEType)
			if table.SSEType == "" {
				table.SSEType = string(types.SSETypeKms)
			}
			table.SSEKMSMasterKeyArn = aws.ToString(input.SSESpecification.KMSMasterKeyId)
		} else {
			table.SSEEnabled = false
			table.SSEType = ""
			table.SSEKMSMasterKeyArn = ""
		}
	}

	*rcu = int64(table.ProvisionedThroughput.ReadCapacityUnits)
	*wcu = int64(table.ProvisionedThroughput.WriteCapacityUnits)
	*out = buildUpdateTableOutput(input, table)

	return nil
}

// applyReplicaTableEntries creates or removes physical Table entries for Global Tables v2
// replica operations triggered by UpdateTable.ReplicaUpdates.
// Must be called after the table lock has been released.
func (db *InMemoryDB) applyReplicaTableEntries(
	tableName string,
	currentRegion string,
	source *Table,
	updates []types.ReplicationGroupUpdate,
) {
	db.mu.Lock("UpdateTable.replicaEntries")
	defer db.mu.Unlock()

	// Ensure the source table is marked as a global table.
	existingGTName := globalTableNameOfRLocked(source)

	if existingGTName == "" {
		setGlobalTableNameLocked(source, tableName)
	}

	for _, u := range updates {
		db.applyOneReplicaTableEntry(tableName, currentRegion, source, u)
	}
}

// globalTableNameOfRLocked returns source.GlobalTableName under a
// defer-protected table.mu.RLock.
func globalTableNameOfRLocked(source *Table) string {
	source.mu.RLock("UpdateTable.replicaEntries.read")
	defer source.mu.RUnlock()

	return source.GlobalTableName
}

// setGlobalTableNameLocked sets source.GlobalTableName under a
// defer-protected table.mu.Lock.
func setGlobalTableNameLocked(source *Table, tableName string) {
	source.mu.Lock("UpdateTable.replicaEntries.setGT")
	defer source.mu.Unlock()

	source.GlobalTableName = tableName
}

// applyOneReplicaTableEntry handles a single Create or Delete ReplicationGroupUpdate.
// Must be called with db.mu held for writing.
func (db *InMemoryDB) applyOneReplicaTableEntry(
	tableName string,
	currentRegion string,
	source *Table,
	update types.ReplicationGroupUpdate,
) {
	switch {
	case update.Create != nil:
		regionName := aws.ToString(update.Create.RegionName)
		if regionName == "" || regionName == currentRegion {
			return
		}

		if existing, exists := db.tables.Get(tableKey(regionName, tableName)); !exists {
			replica := cloneTableSchema(source, tableName, regionName, db.accountID)
			replica.GlobalTableName = tableName

			cloneSourceItemsIntoReplicaRLocked(source, replica)

			if len(replica.Items) > 0 {
				replica.rebuildIndexes()
			}

			db.tables.Put(replica)
		} else {
			existing.GlobalTableName = tableName
		}

	case update.Delete != nil:
		regionName := aws.ToString(update.Delete.RegionName)
		if regionName == "" || regionName == currentRegion {
			return
		}

		db.tables.Delete(tableKey(regionName, tableName))
	}
}

// cloneSourceItemsIntoReplicaRLocked deep-copies source's items and item sizes
// into replica, under a defer-protected source.mu.RLock. replica is not yet
// visible to any other goroutine at this point, so it needs no lock of its own.
func cloneSourceItemsIntoReplicaRLocked(source, replica *Table) {
	source.mu.RLock("cloneItems")
	defer source.mu.RUnlock()

	replica.Items = make([]map[string]any, len(source.Items))
	replica.itemSizes = make([]int, len(source.itemSizes))
	replica.totalItemSizeBytes = source.totalItemSizeBytes
	for i, item := range source.Items {
		replica.Items[i] = deepCopyItem(item)
		replica.itemSizes[i] = source.itemSizes[i]
	}
}

// applyReplicaUpdates processes Global Tables v2 replica create/update/delete actions
// and updates the Replicas metadata list on the table.
// Physical Table entries for new regions are created separately by applyReplicaTableEntries.
// Returns an error if any update has an empty RegionName.
func applyReplicaUpdates(table *Table, updates []types.ReplicationGroupUpdate) error {
	for _, u := range updates {
		switch {
		case u.Create != nil:
			regionName := aws.ToString(u.Create.RegionName)
			if regionName == "" {
				return errReplicaCreateRegionRequired
			}

			applyReplicaCreate(table, regionName)
		case u.Delete != nil:
			regionName := aws.ToString(u.Delete.RegionName)
			if regionName == "" {
				return errReplicaDeleteRegionRequired
			}

			applyReplicaDelete(table, regionName)
		case u.Update != nil:
			regionName := aws.ToString(u.Update.RegionName)
			if regionName == "" {
				return errReplicaCreateRegionRequired
			}

			applyReplicaUpdate(table, regionName, u.Update)
		}
	}

	return nil
}

func applyReplicaCreate(table *Table, regionName string) {
	if regionName == "" {
		return
	}

	for _, r := range table.Replicas {
		if r.RegionName == regionName {
			return
		}
	}

	table.Replicas = append(table.Replicas, models.ReplicaDescription{
		RegionName:    regionName,
		ReplicaStatus: statusActive,
	})
}

func applyReplicaDelete(table *Table, regionName string) {
	if regionName == "" {
		return
	}

	updated := make([]models.ReplicaDescription, 0, len(table.Replicas))
	for _, r := range table.Replicas {
		if r.RegionName != regionName {
			updated = append(updated, r)
		}
	}

	table.Replicas = updated
}

// applyReplicaUpdate applies per-replica setting overrides (table class, provisioned throughput)
// from an UpdateReplicationGroupMemberAction. The replica must already exist.
func applyReplicaUpdate(
	table *Table,
	regionName string,
	action *types.UpdateReplicationGroupMemberAction,
) {
	for i := range table.Replicas {
		if table.Replicas[i].RegionName != regionName {
			continue
		}

		if string(action.TableClassOverride) != "" {
			table.Replicas[i].TableClassOverride = string(action.TableClassOverride)
		}

		if action.ProvisionedThroughputOverride != nil &&
			action.ProvisionedThroughputOverride.ReadCapacityUnits != nil {
			rcu := *action.ProvisionedThroughputOverride.ReadCapacityUnits
			table.Replicas[i].ProvisionedReadCapacityUnits = &rcu
		}

		if len(action.GlobalSecondaryIndexes) > 0 {
			overrides := make([]models.ReplicaGSIOverride, 0, len(action.GlobalSecondaryIndexes))
			for _, g := range action.GlobalSecondaryIndexes {
				ov := models.ReplicaGSIOverride{IndexName: aws.ToString(g.IndexName)}
				if g.ProvisionedThroughputOverride != nil &&
					g.ProvisionedThroughputOverride.ReadCapacityUnits != nil {
					rcu := *g.ProvisionedThroughputOverride.ReadCapacityUnits
					ov.ProvisionedReadCapacity = &rcu
				}
				overrides = append(overrides, ov)
			}
			table.Replicas[i].GlobalSecondaryIndexes = overrides
		}

		return
	}
}

// applyUpdateTableThroughput updates provisioned throughput on the table.
func applyUpdateTableThroughput(table *Table, pt *types.ProvisionedThroughput) {
	if pt == nil {
		return
	}

	if pt.ReadCapacityUnits != nil {
		table.ProvisionedThroughput.ReadCapacityUnits = int(*pt.ReadCapacityUnits)
	}

	if pt.WriteCapacityUnits != nil {
		table.ProvisionedThroughput.WriteCapacityUnits = int(*pt.WriteCapacityUnits)
	}
}

// applyUpdateTableAttrDefs merges new attribute definitions into the table (keeps existing ones).
func applyUpdateTableAttrDefs(table *Table, sdkADs []types.AttributeDefinition) {
	if len(sdkADs) == 0 {
		return
	}

	existing := make(map[string]struct{}, len(table.AttributeDefinitions))
	for _, ad := range table.AttributeDefinitions {
		existing[ad.AttributeName] = struct{}{}
	}

	for _, sdkAD := range sdkADs {
		name := aws.ToString(sdkAD.AttributeName)
		if _, found := existing[name]; !found {
			table.AttributeDefinitions = append(
				table.AttributeDefinitions,
				models.AttributeDefinition{
					AttributeName: name,
					AttributeType: string(sdkAD.AttributeType),
				},
			)
		}
	}
}

// applyGSIUpdates applies Create / Update / Delete GSI actions.
// Returns the first error encountered (e.g. LimitExceededException).
func (db *InMemoryDB) applyGSIUpdates(
	table *Table,
	updates []types.GlobalSecondaryIndexUpdate,
) error {
	for _, u := range updates {
		switch {
		case u.Create != nil:
			if err := db.applyGSICreate(table, u.Create); err != nil {
				return err
			}
		case u.Update != nil:
			db.applyGSIUpdate(table, u.Update)
		case u.Delete != nil:
			db.applyGSIDelete(table, u.Delete)
		}
	}

	return nil
}

func (db *InMemoryDB) applyGSICreate(
	table *Table,
	c *types.CreateGlobalSecondaryIndexAction,
) error {
	if err := validateGSICount(table.GlobalSecondaryIndexes, 1); err != nil {
		return err
	}

	newGSI := models.GlobalSecondaryIndex{
		IndexName:  aws.ToString(c.IndexName),
		KeySchema:  models.FromSDKKeySchema(c.KeySchema),
		Projection: models.FromSDKProjection(c.Projection),
	}

	if c.ProvisionedThroughput != nil {
		newGSI.ProvisionedThroughput = models.ProvisionedThroughput{
			ReadCapacityUnits:  c.ProvisionedThroughput.ReadCapacityUnits,
			WriteCapacityUnits: c.ProvisionedThroughput.WriteCapacityUnits,
		}
	}

	// Simulated GSI lifecycle: CREATING -> ACTIVE
	// If createDelay is set, use a timer; otherwise transition immediately.
	// Since table.GlobalSecondaryIndexes is a slice and we need to update the status in-place later,
	// we use the slice index. note: this assumes indexes are not deleted while a timer is pending.
	idx := len(table.GlobalSecondaryIndexes)
	newGSI.IndexStatus = string(types.IndexStatusCreating)

	table.GlobalSecondaryIndexes = append(table.GlobalSecondaryIndexes, newGSI)

	// Re-get pointer to the added entry to setup timer
	gsiPtr := &table.GlobalSecondaryIndexes[idx]

	if delay := db.createDelay; delay > 0 {
		gsiPtr.IndexStatusTimer = time.AfterFunc(delay, func() {
			table.mu.Lock("GSIActivate")
			defer table.mu.Unlock()
			// Find the GSI again by name as slice might have moved or item deleted
			for i := range table.GlobalSecondaryIndexes {
				if table.GlobalSecondaryIndexes[i].IndexName == gsiPtr.IndexName {
					table.GlobalSecondaryIndexes[i].IndexStatus = string(types.IndexStatusActive)

					break
				}
			}
		})
	} else {
		gsiPtr.IndexStatus = string(types.IndexStatusActive)
	}

	// Rebuild (not just initialise) so existing items remain indexed after the GSI
	// definition is added. initializeIndexes() would clear the primary key index,
	// forcing O(n) scans for all subsequent primary-key queries.
	table.rebuildIndexes()

	return nil
}

func (db *InMemoryDB) applyGSIUpdate(
	table *Table,
	u *types.UpdateGlobalSecondaryIndexAction,
) { // Changed to method
	idxName := aws.ToString(u.IndexName)

	for i, gsi := range table.GlobalSecondaryIndexes {
		if gsi.IndexName == idxName && u.ProvisionedThroughput != nil {
			table.GlobalSecondaryIndexes[i].ProvisionedThroughput = models.ProvisionedThroughput{
				ReadCapacityUnits:  u.ProvisionedThroughput.ReadCapacityUnits,
				WriteCapacityUnits: u.ProvisionedThroughput.WriteCapacityUnits,
			}

			return
		}
	}
}

func (db *InMemoryDB) applyGSIDelete(table *Table, d *types.DeleteGlobalSecondaryIndexAction) {
	idxName := aws.ToString(d.IndexName)

	foundIdx := -1
	for i, gsi := range table.GlobalSecondaryIndexes {
		if gsi.IndexName == idxName {
			foundIdx = i

			break
		}
	}

	if foundIdx == -1 {
		return
	}

	// Simulated GSI lifecycle: DELETING -> removed
	if delay := db.createDelay; delay > 0 {
		gsiPtr := &table.GlobalSecondaryIndexes[foundIdx]
		gsiPtr.IndexStatus = string(types.IndexStatusDeleting)
		if gsiPtr.IndexStatusTimer != nil {
			gsiPtr.IndexStatusTimer.Stop()
		}
		gsiPtr.IndexStatusTimer = time.AfterFunc(delay, func() {
			table.mu.Lock("GSIRemove")
			defer table.mu.Unlock()
			// Find the GSI again by name as slice might have moved
			for i := range table.GlobalSecondaryIndexes {
				if table.GlobalSecondaryIndexes[i].IndexName == idxName {
					// Remove from slice
					table.GlobalSecondaryIndexes = append(
						table.GlobalSecondaryIndexes[:i],
						table.GlobalSecondaryIndexes[i+1:]...,
					)

					break
				}
			}
		})
	} else {
		// Immediate removal: stop any pending create/activation timer to prevent
		// the orphaned AfterFunc from firing after the GSI slice entry is gone.
		gsiPtr := &table.GlobalSecondaryIndexes[foundIdx]
		if gsiPtr.IndexStatusTimer != nil {
			gsiPtr.IndexStatusTimer.Stop()
			gsiPtr.IndexStatusTimer = nil
		}
		table.GlobalSecondaryIndexes = append(
			table.GlobalSecondaryIndexes[:foundIdx],
			table.GlobalSecondaryIndexes[foundIdx+1:]...,
		)
	}

	// Rebuild to clear the index from memory immediately
	table.rebuildIndexes()
}

// applyStreamSpec enables or disables streams on the table.
// Returns the old stream ARN (to remove from the index) and the new ARN (to add), so the caller
// can update db.streamARNIndex under db.mu after releasing the table lock.
func (db *InMemoryDB) applyStreamSpec(
	table *Table,
	tableName string,
	ss *types.StreamSpecification,
	region string,
) (string, string) {
	if ss == nil {
		return "", ""
	}

	oldARN := table.StreamARN

	if aws.ToBool(ss.StreamEnabled) {
		table.StreamsEnabled = true
		table.StreamViewType = string(ss.StreamViewType)

		if table.StreamARN == "" {
			streamCreatedAt := time.Now().UTC()
			table.StreamCreatedAt = streamCreatedAt
			table.StreamARN = db.buildStreamARNInRegion(tableName, region, streamCreatedAt)
			// Initialize the first shard when streams are newly enabled via UpdateTable.
			table.StreamShards = []StreamShard{
				{
					ShardID:             streamShardID,
					StartingSequenceNum: table.streamSeq + 1,
				},
			}
		}

		return oldARN, table.StreamARN
	}

	table.StreamsEnabled = false
	table.StreamViewType = ""
	table.StreamARN = ""
	table.StreamShards = nil
	table.streamTrimSeq = 0

	return oldARN, ""
}

// buildUpdateTableOutput constructs the UpdateTable response from the current table state.
func buildUpdateTableOutput(
	input *dynamodb.UpdateTableInput,
	table *Table,
) *dynamodb.UpdateTableOutput {
	rcu := int64(table.ProvisionedThroughput.ReadCapacityUnits)
	wcu := int64(table.ProvisionedThroughput.WriteCapacityUnits)

	gsiDescs := make([]types.GlobalSecondaryIndexDescription, 0, len(table.GlobalSecondaryIndexes))

	for _, gsi := range table.GlobalSecondaryIndexes {
		rc := int64(models.DefaultReadCapacity)
		wc := int64(models.DefaultWriteCapacity)

		if gsi.ProvisionedThroughput.ReadCapacityUnits != nil {
			rc = *gsi.ProvisionedThroughput.ReadCapacityUnits
		}

		if gsi.ProvisionedThroughput.WriteCapacityUnits != nil {
			wc = *gsi.ProvisionedThroughput.WriteCapacityUnits
		}

		status := gsi.IndexStatus
		if status == "" {
			status = string(types.IndexStatusActive)
		}

		gsiDescs = append(gsiDescs, types.GlobalSecondaryIndexDescription{
			IndexName:   &gsi.IndexName,
			KeySchema:   models.ToSDKKeySchema(gsi.KeySchema),
			Projection:  models.ToSDKProjection(gsi.Projection),
			IndexStatus: types.IndexStatus(status),
			ProvisionedThroughput: &types.ProvisionedThroughputDescription{
				ReadCapacityUnits:  &rc,
				WriteCapacityUnits: &wc,
			},
		})
	}

	td := &types.TableDescription{
		TableName:                 input.TableName,
		TableArn:                  aws.String(table.TableArn),
		TableStatus:               types.TableStatusActive,
		KeySchema:                 models.ToSDKKeySchema(table.KeySchema),
		AttributeDefinitions:      models.ToSDKAttributeDefinitions(table.AttributeDefinitions),
		GlobalSecondaryIndexes:    gsiDescs,
		Replicas:                  toSDKReplicaDescriptions(table.Replicas),
		DeletionProtectionEnabled: aws.Bool(table.DeletionProtectionEnabled),
		ProvisionedThroughput: &types.ProvisionedThroughputDescription{
			ReadCapacityUnits:  &rcu,
			WriteCapacityUnits: &wcu,
		},
	}

	if table.TableClass != "" {
		td.TableClassSummary = &types.TableClassSummary{
			TableClass: types.TableClass(table.TableClass),
		}
	}

	if table.BillingMode != "" {
		td.BillingModeSummary = &types.BillingModeSummary{
			BillingMode: types.BillingMode(table.BillingMode),
		}
	}

	if table.SSEEnabled {
		td.SSEDescription = &types.SSEDescription{
			Status:          types.SSEStatusEnabled,
			SSEType:         types.SSEType(table.SSEType),
			KMSMasterKeyArn: aws.String(table.SSEKMSMasterKeyArn),
		}
	}

	return &dynamodb.UpdateTableOutput{
		TableDescription: td,
	}
}

// toSDKReplicaDescriptions converts internal replica metadata to SDK types.
func toSDKReplicaDescriptions(replicas []models.ReplicaDescription) []types.ReplicaDescription {
	if len(replicas) == 0 {
		return nil
	}

	out := make([]types.ReplicaDescription, len(replicas))
	for i, r := range replicas {
		regionName := r.RegionName
		desc := types.ReplicaDescription{
			RegionName:    &regionName,
			ReplicaStatus: types.ReplicaStatus(r.ReplicaStatus),
		}

		if r.TableClassOverride != "" {
			tc := types.TableClass(r.TableClassOverride)
			desc.ReplicaTableClassSummary = &types.TableClassSummary{TableClass: tc}
		}

		if r.ProvisionedReadCapacityUnits != nil {
			rcu := *r.ProvisionedReadCapacityUnits
			desc.ProvisionedThroughputOverride = &types.ProvisionedThroughputOverride{
				ReadCapacityUnits: &rcu,
			}
		}

		if len(r.GlobalSecondaryIndexes) > 0 {
			gsis := make(
				[]types.ReplicaGlobalSecondaryIndexDescription,
				len(r.GlobalSecondaryIndexes),
			)
			for j, g := range r.GlobalSecondaryIndexes {
				name := g.IndexName
				gd := types.ReplicaGlobalSecondaryIndexDescription{IndexName: &name}
				if g.ProvisionedReadCapacity != nil {
					rcu := *g.ProvisionedReadCapacity
					gd.ProvisionedThroughputOverride = &types.ProvisionedThroughputOverride{
						ReadCapacityUnits: &rcu,
					}
				}
				gsis[j] = gd
			}
			desc.GlobalSecondaryIndexes = gsis
		}

		out[i] = desc
	}

	return out
}

func (db *InMemoryDB) UpdateTimeToLive(
	ctx context.Context,
	input *dynamodb.UpdateTimeToLiveInput,
) (*dynamodb.UpdateTimeToLiveOutput, error) {
	tableName := aws.ToString(input.TableName)
	if tableName == "" {
		return nil, NewValidationException("Table name is required")
	}

	table, err := db.getTable(ctx, tableName)
	if err != nil {
		return nil, err
	}

	if input.TimeToLiveSpecification == nil {
		return nil, NewValidationException("TimeToLiveSpecification is required")
	}

	attrName := aws.ToString(input.TimeToLiveSpecification.AttributeName)
	enabled := aws.ToBool(input.TimeToLiveSpecification.Enabled)

	if enabled && attrName == "" {
		return nil, NewValidationException(
			"TimeToLive attribute name must not be empty when enabling TTL",
		)
	}

	table.mu.Lock("UpdateTimeToLive")
	defer table.mu.Unlock()

	if enabled {
		table.TTLAttribute = attrName
	} else {
		table.TTLAttribute = ""
	}

	return &dynamodb.UpdateTimeToLiveOutput{
		TimeToLiveSpecification: input.TimeToLiveSpecification,
	}, nil
}

func (db *InMemoryDB) DescribeTimeToLive(
	ctx context.Context,
	input *dynamodb.DescribeTimeToLiveInput,
) (*dynamodb.DescribeTimeToLiveOutput, error) {
	tableName := aws.ToString(input.TableName)
	table, err := db.getTable(ctx, tableName)
	if err != nil {
		return nil, err
	}

	table.mu.RLock("DescribeTimeToLive")
	defer table.mu.RUnlock()

	if table.TTLAttribute == "" {
		return &dynamodb.DescribeTimeToLiveOutput{
			TimeToLiveDescription: &types.TimeToLiveDescription{
				TimeToLiveStatus: types.TimeToLiveStatusDisabled,
			},
		}, nil
	}

	return &dynamodb.DescribeTimeToLiveOutput{
		TimeToLiveDescription: &types.TimeToLiveDescription{
			AttributeName:    aws.String(table.TTLAttribute),
			TimeToLiveStatus: types.TimeToLiveStatusEnabled,
		},
	}, nil
}

// regionTableNamesRLocked returns the names of every table in region under a
// defer-protected db.mu.RLock.
func regionTableNamesRLocked(db *InMemoryDB, region string) []string {
	db.mu.RLock("ListTables")
	defer db.mu.RUnlock()

	regionTables := db.tablesByRegion.Get(region)
	names := make([]string, 0, len(regionTables))
	for _, t := range regionTables {
		names = append(names, t.Name)
	}

	return names
}

func (db *InMemoryDB) ListTables(
	ctx context.Context,
	input *dynamodb.ListTablesInput,
) (*dynamodb.ListTablesOutput, error) {
	if err := validateListTablesLimit(input.Limit); err != nil {
		return nil, err
	}

	region := getRegionFromContext(ctx, db)

	// Snapshot table names under lock, then release immediately
	names := regionTableNamesRLocked(db, region)

	// Sort outside the lock to reduce contention
	sort.Strings(names)

	startName := aws.ToString(input.ExclusiveStartTableName)
	startIndex := 0
	if startName != "" {
		idx, found := findStartIndex(names, startName)
		if !found {
			return &dynamodb.ListTablesOutput{
				TableNames: []string{},
			}, nil
		}

		startIndex = idx
	}

	names = names[startIndex:]

	limit := int(aws.ToInt32(input.Limit))
	if limit <= 0 {
		limit = 100
	}

	var lastEvaluatedName string
	if len(names) > limit {
		lastEvaluatedName = names[limit-1]
		names = names[:limit]
	}

	out := &dynamodb.ListTablesOutput{
		TableNames: names,
	}

	if lastEvaluatedName != "" {
		out.LastEvaluatedTableName = aws.String(lastEvaluatedName)
	}

	return out, nil
}

// findStartIndex returns the index of the first name strictly greater than after,
// and whether such a name exists.
func findStartIndex(names []string, after string) (int, bool) {
	for i, name := range names {
		if name > after {
			return i, true
		}
	}

	return 0, false
}
