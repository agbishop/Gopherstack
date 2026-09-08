package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	sdktypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

// maxBatchExecuteStatements is the maximum number of PartiQL statements per
// BatchExecuteStatement call, matching the AWS service limit.
const maxBatchExecuteStatements = 25

// tableBackupSnapshot holds the fields captured from a Table under RLock for backup creation.
type tableBackupSnapshot struct {
	SSEType                string
	TableID                string
	Status                 string
	StreamViewType         string
	BillingMode            string
	TableArn               string
	SSEKMSMasterKeyArn     string
	KeySchema              []models.KeySchemaElement
	Items                  []map[string]any
	LocalSecondaryIndexes  []models.LocalSecondaryIndex
	GlobalSecondaryIndexes []models.GlobalSecondaryIndex
	AttributeDefinitions   []models.AttributeDefinition
	ProvisionedThroughput  models.ProvisionedThroughputDescription
	SSEEnabled             bool
	StreamsEnabled         bool
}

func snapshotTableForBackup(table *Table) tableBackupSnapshot {
	table.mu.RLock("CreateBackup")
	defer table.mu.RUnlock()

	snap := tableBackupSnapshot{
		Items:                 deepCopyItems(table.Items),
		TableArn:              table.TableArn,
		TableID:               table.TableID,
		ProvisionedThroughput: table.ProvisionedThroughput,
		BillingMode:           table.BillingMode,
		SSEEnabled:            table.SSEEnabled,
		SSEType:               table.SSEType,
		SSEKMSMasterKeyArn:    table.SSEKMSMasterKeyArn,
		StreamsEnabled:        table.StreamsEnabled,
		StreamViewType:        table.StreamViewType,
		Status:                table.Status,

		KeySchema: make([]models.KeySchemaElement, len(table.KeySchema))}
	copy(snap.KeySchema, table.KeySchema)
	snap.AttributeDefinitions = make([]models.AttributeDefinition, len(table.AttributeDefinitions))
	copy(snap.AttributeDefinitions, table.AttributeDefinitions)
	snap.GlobalSecondaryIndexes = make(
		[]models.GlobalSecondaryIndex,
		len(table.GlobalSecondaryIndexes),
	)
	copy(snap.GlobalSecondaryIndexes, table.GlobalSecondaryIndexes)
	snap.LocalSecondaryIndexes = make(
		[]models.LocalSecondaryIndex,
		len(table.LocalSecondaryIndexes),
	)
	copy(snap.LocalSecondaryIndexes, table.LocalSecondaryIndexes)

	return snap
}

// CreateBackup creates a point-in-time backup of the named DynamoDB table.
// It satisfies the StorageBackend interface using official AWS SDK v2 types.
func (db *InMemoryDB) CreateBackup(
	ctx context.Context,
	input *sdkdynamodb.CreateBackupInput,
) (*sdkdynamodb.CreateBackupOutput, error) {
	if input == nil {
		return nil, NewValidationException("CreateBackupInput must not be nil")
	}

	tableName := aws.ToString(input.TableName)
	backupName := aws.ToString(input.BackupName)

	if tableName == "" {
		return nil, NewValidationException("TableName is required")
	}

	if backupName == "" {
		return nil, NewValidationException("BackupName is required")
	}

	region := getRegionFromContext(ctx, db)

	table, err := db.getTable(ctx, tableName)
	if err != nil {
		return nil, err
	}

	snap := snapshotTableForBackup(table)

	if snap.Status != models.TableStatusActive {
		return nil, NewValidationException(
			fmt.Sprintf(
				"table %q is not ACTIVE (status=%s); backups can only be created on ACTIVE tables",
				tableName, snap.Status,
			),
		)
	}

	// Check for duplicate backup name scoped to this table; AWS returns BackupInUseException.
	if db.duplicateBackupNameExistsRLocked(tableName, backupName) {
		return nil, NewBackupInUseException(
			fmt.Sprintf(
				"backup with name %q already exists for table %q",
				backupName,
				tableName,
			),
		)
	}

	now := time.Now()
	bkpARN := backupARN(region, db.accountID, tableName, now)
	sizeBytes := estimateTableSizeBytes(table)

	backup := &Backup{
		BackupArn: bkpARN, BackupName: backupName,
		BackupStatus: models.BackupStatusAvailable, BackupType: models.BackupTypeUser,
		TableName: tableName, TableArn: snap.TableArn, TableID: snap.TableID,
		CreationDateTime: now, Items: snap.Items,
		KeySchema: snap.KeySchema, AttributeDefinitions: snap.AttributeDefinitions,
		GlobalSecondaryIndexes: snap.GlobalSecondaryIndexes,
		LocalSecondaryIndexes:  snap.LocalSecondaryIndexes,
		ProvisionedThroughput:  snap.ProvisionedThroughput, BillingMode: snap.BillingMode,
		SSEEnabled: snap.SSEEnabled, SSEType: snap.SSEType,
		SSEKMSMasterKeyArn: snap.SSEKMSMasterKeyArn,
		StreamsEnabled:     snap.StreamsEnabled, StreamViewType: snap.StreamViewType,
		SizeBytes: sizeBytes,
	}

	db.insertBackupLocked(backup)

	return &sdkdynamodb.CreateBackupOutput{
		BackupDetails: &sdktypes.BackupDetails{
			BackupArn: aws.String(bkpARN), BackupName: aws.String(backupName),
			BackupStatus: sdktypes.BackupStatusAvailable, BackupType: sdktypes.BackupTypeUser,
			BackupCreationDateTime: aws.Time(now.UTC()), BackupSizeBytes: aws.Int64(sizeBytes),
		},
	}, nil
}

// duplicateBackupNameExistsRLocked reports whether a non-deleted backup with
// the given name already exists for tableName, under a defer-protected
// db.mu.RLock.
func (db *InMemoryDB) duplicateBackupNameExistsRLocked(tableName, backupName string) bool {
	db.mu.RLock("CreateBackup.checkDuplicate")
	defer db.mu.RUnlock()

	for _, existing := range db.backups.All() {
		if existing.TableName == tableName && existing.BackupName == backupName &&
			existing.BackupStatus != models.BackupStatusDeleted {
			return true
		}
	}

	return false
}

// insertBackupLocked stores backup and evicts the oldest entries beyond
// maxBackupsRetained, under a defer-protected db.mu.Lock.
func (db *InMemoryDB) insertBackupLocked(backup *Backup) {
	db.mu.Lock("CreateBackup")
	defer db.mu.Unlock()

	db.backups.Put(backup)
	evictOldestFromTable(
		db.backups,
		maxBackupsRetained,
		backupKeyFn,
		func(b *Backup) time.Time { return b.CreationDateTime },
	)
}

// DescribeBackup returns the full description of a backup by ARN.
// It satisfies the StorageBackend interface using official AWS SDK v2 types.
func (db *InMemoryDB) DescribeBackup(
	ctx context.Context,
	input *sdkdynamodb.DescribeBackupInput,
) (*sdkdynamodb.DescribeBackupOutput, error) {
	if input == nil {
		return nil, NewValidationException("DescribeBackupInput must not be nil")
	}

	backupArn := aws.ToString(input.BackupArn)
	if backupArn == "" {
		return nil, NewValidationException("BackupArn is required")
	}

	requestRegion := getRegionFromContext(ctx, db)
	if db.regionFromARN(backupArn) != requestRegion {
		return nil, NewResourceNotFoundException("backup not found: " + backupArn)
	}

	backupCopy, exists := db.backupCopyRLocked(backupArn)
	if !exists {
		return nil, NewResourceNotFoundException("backup not found: " + backupArn)
	}

	return &sdkdynamodb.DescribeBackupOutput{
		BackupDescription: buildSDKBackupDescription(&backupCopy),
	}, nil
}

// backupCopyRLocked returns a copy of the backup stored under backupArn (and
// whether it exists) under a defer-protected db.mu.RLock.
func (db *InMemoryDB) backupCopyRLocked(backupArn string) (Backup, bool) {
	db.mu.RLock("DescribeBackup")
	defer db.mu.RUnlock()

	backup, exists := db.backups.Get(backupArn)
	if !exists {
		return Backup{}, false
	}

	return *backup, true
}

// DeleteBackup removes an existing backup by ARN and returns its description.
// It satisfies the StorageBackend interface using official AWS SDK v2 types.
func (db *InMemoryDB) DeleteBackup(
	ctx context.Context,
	input *sdkdynamodb.DeleteBackupInput,
) (*sdkdynamodb.DeleteBackupOutput, error) {
	if input == nil {
		return nil, NewValidationException("DeleteBackupInput must not be nil")
	}

	backupArn := aws.ToString(input.BackupArn)
	if backupArn == "" {
		return nil, NewValidationException("BackupArn is required")
	}

	requestRegion := getRegionFromContext(ctx, db)
	if db.regionFromARN(backupArn) != requestRegion {
		return nil, NewResourceNotFoundException("backup not found: " + backupArn)
	}

	db.mu.Lock("DeleteBackup")
	defer db.mu.Unlock()

	backup, exists := db.backups.Get(backupArn)
	if !exists {
		return nil, NewResourceNotFoundException("backup not found: " + backupArn)
	}

	backupCopy := *backup
	backupCopy.BackupStatus = models.BackupStatusDeleted
	db.backups.Delete(backupArn)

	return &sdkdynamodb.DeleteBackupOutput{
		BackupDescription: buildSDKBackupDescription(&backupCopy),
	}, nil
}

// buildSDKBackupDetails converts an internal Backup into SDK BackupDetails.
// Returns nil if b is nil.
func buildSDKBackupDetails(b *Backup) *sdktypes.BackupDetails {
	if b == nil {
		return nil
	}

	return &sdktypes.BackupDetails{
		BackupArn:              aws.String(b.BackupArn),
		BackupName:             aws.String(b.BackupName),
		BackupStatus:           sdktypes.BackupStatus(b.BackupStatus),
		BackupType:             sdktypes.BackupType(b.BackupType),
		BackupCreationDateTime: aws.Time(b.CreationDateTime.UTC()),
		BackupSizeBytes:        aws.Int64(b.SizeBytes),
	}
}

// buildSDKSourceTableDetails converts an internal Backup into SDK SourceTableDetails.
// Returns nil if b is nil.
func buildSDKSourceTableDetails(b *Backup) *sdktypes.SourceTableDetails {
	if b == nil {
		return nil
	}

	sdkKeys := make([]sdktypes.KeySchemaElement, 0, len(b.KeySchema))
	for _, ks := range b.KeySchema {
		sdkKeys = append(sdkKeys, sdktypes.KeySchemaElement{
			AttributeName: aws.String(ks.AttributeName),
			KeyType:       sdktypes.KeyType(ks.KeyType),
		})
	}

	// Use the actual provisioned throughput captured at backup creation time.
	readCU := int64(b.ProvisionedThroughput.ReadCapacityUnits)
	if readCU == 0 {
		readCU = models.DefaultReadCapacity
	}

	writeCU := int64(b.ProvisionedThroughput.WriteCapacityUnits)
	if writeCU == 0 {
		writeCU = models.DefaultWriteCapacity
	}

	return &sdktypes.SourceTableDetails{
		TableName: aws.String(b.TableName),
		TableId:   aws.String(b.TableID),
		TableArn:  aws.String(b.TableArn),
		KeySchema: sdkKeys,
		ProvisionedThroughput: &sdktypes.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(readCU),
			WriteCapacityUnits: aws.Int64(writeCU),
		},
		TableCreationDateTime: aws.Time(b.CreationDateTime.UTC()),
	}
}

// buildSDKBackupDescription converts an internal Backup into a full SDK BackupDescription.
// Returns nil if b is nil.
func buildSDKBackupDescription(b *Backup) *sdktypes.BackupDescription {
	if b == nil {
		return nil
	}

	return &sdktypes.BackupDescription{
		BackupDetails:      buildSDKBackupDetails(b),
		SourceTableDetails: buildSDKSourceTableDetails(b),
	}
}

const (
	continuousBackupsStatusEnabled  = "ENABLED"
	continuousBackupsStatusDisabled = "DISABLED"
)

// continuousBackupsStatusForExistingTable is always ENABLED. Verified against
// api_op_UpdateContinuousBackups.go: UpdateContinuousBackupsInput has exactly
// two members, TableName and PointInTimeRecoverySpecification -- there is no
// field anywhere in the SDK that lets a caller set ContinuousBackupsStatus
// itself, only the nested PointInTimeRecoveryStatus (via
// PointInTimeRecoverySpecification.PointInTimeRecoveryEnabled). The two are
// genuinely distinct fields; only PointInTimeRecoveryStatus is derived here,
// from table.PITREnabled.
const continuousBackupsStatusForExistingTable = sdktypes.ContinuousBackupsStatus(continuousBackupsStatusEnabled)

// defaultRecoveryPeriodInDays matches PointInTimeRecoverySpecification's
// documented default (types.go): "If no value is provided, the value will
// default to 35".
const defaultRecoveryPeriodInDays int32 = 35

// pitrStateRLocked returns whether PITR is enabled, the configured recovery
// period, and, when enabled with at least one snapshot taken, the
// earliest/latest restorable timestamps, under a defer-protected
// table.mu.RLock.
func pitrStateRLocked(table *Table) (bool, int32, time.Time, time.Time) {
	table.mu.RLock(opDescribeContinuousBackups)
	defer table.mu.RUnlock()

	pitrEnabled := table.PITREnabled
	recoveryPeriodInDays := table.RecoveryPeriodInDays

	var earliest, latest time.Time
	// EarliestRestorableDateTime tracks the oldest available snapshot.
	// LatestRestorableDateTime is "now" while PITR is active -- AWS
	// guarantees you can always recover to the current instant.
	if pitrEnabled && len(table.PITRSnapshots) > 0 {
		earliest = table.PITRSnapshots[0].Taken
		latest = time.Now().UTC()
	}

	return pitrEnabled, recoveryPeriodInDays, earliest, latest
}

// setPITREnabledLocked sets table.PITREnabled and table.RecoveryPeriodInDays
// and, when disabling, releases the snapshot ring, under a defer-protected
// table.mu.Lock. recoveryPeriodInDays is ignored when disabling.
func setPITREnabledLocked(table *Table, pitrEnabled bool, recoveryPeriodInDays int32) {
	table.mu.Lock(opUpdateContinuousBackups)
	defer table.mu.Unlock()

	table.PITREnabled = pitrEnabled
	if !pitrEnabled {
		// Releasing memory the moment the feature is turned off keeps the
		// per-table footprint tight; re-enabling starts a fresh ring.
		table.PITRSnapshots = nil
		table.RecoveryPeriodInDays = 0

		return
	}

	table.RecoveryPeriodInDays = recoveryPeriodInDays
}

// DescribeContinuousBackups returns the PITR settings for a table.
// It satisfies the StorageBackend interface using official AWS SDK v2 types.
func (db *InMemoryDB) DescribeContinuousBackups(
	ctx context.Context,
	input *sdkdynamodb.DescribeContinuousBackupsInput,
) (*sdkdynamodb.DescribeContinuousBackupsOutput, error) {
	tableName := aws.ToString(input.TableName)
	if tableName == "" {
		return nil, NewValidationException("TableName is required")
	}

	table, err := db.getTable(ctx, tableName)
	if err != nil {
		return nil, err
	}

	pitrEnabled, recoveryPeriodInDays, earliest, latest := pitrStateRLocked(table)

	desc := &sdktypes.PointInTimeRecoveryDescription{
		PointInTimeRecoveryStatus: sdktypes.PointInTimeRecoveryStatusDisabled,
	}
	if pitrEnabled {
		desc.PointInTimeRecoveryStatus = sdktypes.PointInTimeRecoveryStatusEnabled
		desc.RecoveryPeriodInDays = aws.Int32(recoveryPeriodInDays)
		if !earliest.IsZero() {
			desc.EarliestRestorableDateTime = aws.Time(earliest)
			desc.LatestRestorableDateTime = aws.Time(latest)
		}
	}

	return &sdkdynamodb.DescribeContinuousBackupsOutput{
		ContinuousBackupsDescription: &sdktypes.ContinuousBackupsDescription{
			ContinuousBackupsStatus:        continuousBackupsStatusForExistingTable,
			PointInTimeRecoveryDescription: desc,
		},
	}, nil
}

// UpdateContinuousBackups enables or disables PITR for a table.
// It satisfies the StorageBackend interface using official AWS SDK v2 types.
func (db *InMemoryDB) UpdateContinuousBackups(
	ctx context.Context,
	input *sdkdynamodb.UpdateContinuousBackupsInput,
) (*sdkdynamodb.UpdateContinuousBackupsOutput, error) {
	tableName := aws.ToString(input.TableName)
	if tableName == "" {
		return nil, NewValidationException("TableName is required")
	}

	pitrEnabled := false
	recoveryPeriodInDays := defaultRecoveryPeriodInDays
	if spec := input.PointInTimeRecoverySpecification; spec != nil {
		pitrEnabled = aws.ToBool(spec.PointInTimeRecoveryEnabled)
		if spec.RecoveryPeriodInDays != nil {
			recoveryPeriodInDays = *spec.RecoveryPeriodInDays
		}
	}

	const minRecoveryPeriodInDays = 1

	const maxRecoveryPeriodInDays = 35
	if recoveryPeriodInDays < minRecoveryPeriodInDays || recoveryPeriodInDays > maxRecoveryPeriodInDays {
		return nil, NewValidationException("RecoveryPeriodInDays must be between 1 and 35")
	}

	table, err := db.getTable(ctx, tableName)
	if err != nil {
		return nil, err
	}

	setPITREnabledLocked(table, pitrEnabled, recoveryPeriodInDays)

	desc := &sdktypes.PointInTimeRecoveryDescription{
		PointInTimeRecoveryStatus: sdktypes.PointInTimeRecoveryStatusDisabled,
	}
	if pitrEnabled {
		desc.PointInTimeRecoveryStatus = sdktypes.PointInTimeRecoveryStatusEnabled
		desc.RecoveryPeriodInDays = aws.Int32(recoveryPeriodInDays)
	}

	return &sdkdynamodb.UpdateContinuousBackupsOutput{
		ContinuousBackupsDescription: &sdktypes.ContinuousBackupsDescription{
			ContinuousBackupsStatus:        continuousBackupsStatusForExistingTable,
			PointInTimeRecoveryDescription: desc,
		},
	}, nil
}

// tableDescriptionToSDK converts a models.TableDescription into the SDK type,
// covering the subset of fields RestoreTableFromBackup and
// RestoreTableToPointInTime populate.
func tableDescriptionToSDK(d models.TableDescription) *sdktypes.TableDescription {
	td := &sdktypes.TableDescription{
		TableName:              aws.String(d.TableName),
		TableStatus:            sdktypes.TableStatus(d.TableStatus),
		TableArn:               aws.String(d.TableArn),
		TableId:                aws.String(d.TableID),
		KeySchema:              models.ToSDKKeySchema(d.KeySchema),
		AttributeDefinitions:   models.ToSDKAttributeDefinitions(d.AttributeDefinitions),
		GlobalSecondaryIndexes: models.ToSDKGlobalSecondaryIndexDescriptions(d.GlobalSecondaryIndexes),
		LocalSecondaryIndexes:  models.ToSDKLocalSecondaryIndexDescriptions(d.LocalSecondaryIndexes),
		ItemCount:              aws.Int64(int64(d.ItemCount)),
	}
	if d.BillingModeSummary != nil {
		td.BillingModeSummary = &sdktypes.BillingModeSummary{
			BillingMode: sdktypes.BillingMode(d.BillingModeSummary.BillingMode),
		}
	}

	return td
}

// resolveGSIOverride returns the GSI list a restored table should use: a copy
// of source when override is nil (omitted, meaning "keep the source's
// GSIs"), or override converted to the wire type otherwise -- including an
// explicit empty override, which means "restore with no GSIs at all".
func resolveGSIOverride(
	source []models.GlobalSecondaryIndex,
	override []sdktypes.GlobalSecondaryIndex,
) []models.GlobalSecondaryIndex {
	if override == nil {
		gsis := make([]models.GlobalSecondaryIndex, len(source))
		copy(gsis, source)

		return gsis
	}

	return models.FromSDKGlobalSecondaryIndexes(override)
}

// resolveSSEOverride applies SSESpecificationOverride to the source table's
// encryption state, mirroring the CreateTable SSESpecification handling in
// newTableFromCreateInput. override may be nil, matching an omitted request
// member, in which case the source's encryption state passes through.
func resolveSSEOverride(
	enabled bool, sseType, kmsKeyArn string,
	override *sdktypes.SSESpecification,
) (bool, string, string) {
	if override == nil {
		return enabled, sseType, kmsKeyArn
	}

	newEnabled := override.Enabled == nil || aws.ToBool(override.Enabled)
	if !newEnabled {
		return false, "", ""
	}

	newType := string(override.SSEType)
	if newType == "" {
		newType = string(sdktypes.SSETypeKms)
	}

	return true, newType, aws.ToString(override.KMSMasterKeyId)
}

// resolveOnDemandThroughputOverride returns the on-demand throughput caps a
// restored table should use: override when supplied, otherwise the source's
// existing caps unchanged.
func resolveOnDemandThroughputOverride(
	sourceRead, sourceWrite *int64,
	override *sdktypes.OnDemandThroughput,
) (*int64, *int64) {
	if override == nil {
		return sourceRead, sourceWrite
	}

	return override.MaxReadRequestUnits, override.MaxWriteRequestUnits
}

// RestoreTableFromBackup creates a new table populated from an existing backup.
// It satisfies the StorageBackend interface using official AWS SDK v2 types.
func (db *InMemoryDB) RestoreTableFromBackup(
	ctx context.Context,
	input *sdkdynamodb.RestoreTableFromBackupInput,
) (*sdkdynamodb.RestoreTableFromBackupOutput, error) {
	backupArn := aws.ToString(input.BackupArn)
	targetTableName := aws.ToString(input.TargetTableName)

	if backupArn == "" {
		return nil, NewValidationException("BackupArn is required")
	}

	if targetTableName == "" {
		return nil, NewValidationException("TargetTableName is required")
	}

	backup, exists := db.getBackupRLocked(backupArn)
	if !exists {
		return nil, NewResourceNotFoundException("backup not found: " + backupArn)
	}

	region := getRegionFromContext(ctx, db)

	var throughputOverride *models.ProvisionedThroughput
	if input.ProvisionedThroughputOverride != nil {
		throughputOverride = &models.ProvisionedThroughput{
			ReadCapacityUnits:  input.ProvisionedThroughputOverride.ReadCapacityUnits,
			WriteCapacityUnits: input.ProvisionedThroughputOverride.WriteCapacityUnits,
		}
	}

	billingMode, provThroughput := resolveBillingAndThroughput(
		backup.BillingMode, string(input.BillingModeOverride),
		backup.ProvisionedThroughput, throughputOverride,
	)

	gsis := resolveGSIOverride(backup.GlobalSecondaryIndexes, input.GlobalSecondaryIndexOverride)
	lsis := make([]models.LocalSecondaryIndex, len(backup.LocalSecondaryIndexes))
	copy(lsis, backup.LocalSecondaryIndexes)
	keySchema := make([]models.KeySchemaElement, len(backup.KeySchema))
	copy(keySchema, backup.KeySchema)
	attrDefs := make([]models.AttributeDefinition, len(backup.AttributeDefinitions))
	copy(attrDefs, backup.AttributeDefinitions)

	sseEnabled, sseType, sseKMSMasterKeyArn := resolveSSEOverride(
		backup.SSEEnabled, backup.SSEType, backup.SSEKMSMasterKeyArn,
		input.SSESpecificationOverride,
	)
	onDemandMaxReadRRU, onDemandMaxWriteRRU := resolveOnDemandThroughputOverride(
		nil, nil, input.OnDemandThroughputOverride,
	)

	p := restoredTableParams{
		Items: deepCopyItems(backup.Items), KeySchema: keySchema, AttributeDefinitions: attrDefs,
		GlobalSecondaryIndexes: gsis, LocalSecondaryIndexes: lsis,
		ProvisionedThroughput: provThroughput, BillingMode: billingMode,
		SSEEnabled: sseEnabled, SSEType: sseType, SSEKMSMasterKeyArn: sseKMSMasterKeyArn,
		StreamsEnabled: backup.StreamsEnabled, StreamViewType: backup.StreamViewType,
		OnDemandMaxReadRRU: onDemandMaxReadRRU, OnDemandMaxWriteRRU: onDemandMaxWriteRRU,
	}

	newTable, newTableID, err := db.installRestoredTable(region, targetTableName, p)
	if err != nil {
		return nil, err
	}

	td := tableDescriptionToSDK(models.TableDescription{
		TableName: targetTableName, TableStatus: models.TableStatusActive,
		TableArn: newTable.TableArn, TableID: newTableID,
		KeySchema: keySchema, AttributeDefinitions: attrDefs,
		GlobalSecondaryIndexes: buildGSIDescriptions(gsis, int64(len(p.Items)), newTable.TableArn),
		LocalSecondaryIndexes:  buildLSIDescriptions(lsis, newTable.TableArn),
		BillingModeSummary:     billingModeSummary(billingMode),
		ItemCount:              len(p.Items),
	})
	applySSEDescription(td, sseEnabled, sseType, sseKMSMasterKeyArn)
	if onDemandMaxReadRRU != nil || onDemandMaxWriteRRU != nil {
		td.OnDemandThroughput = &sdktypes.OnDemandThroughput{
			MaxReadRequestUnits:  onDemandMaxReadRRU,
			MaxWriteRequestUnits: onDemandMaxWriteRRU,
		}
	}

	return &sdkdynamodb.RestoreTableFromBackupOutput{TableDescription: td}, nil
}

// RestoreTableToPointInTime creates a new table populated from a PITR snapshot
// of an existing table. It satisfies the StorageBackend interface using
// official AWS SDK v2 types.
func (db *InMemoryDB) RestoreTableToPointInTime(
	ctx context.Context,
	input *sdkdynamodb.RestoreTableToPointInTimeInput,
) (*sdkdynamodb.RestoreTableToPointInTimeOutput, error) {
	sourceTableName := aws.ToString(input.SourceTableName)
	targetTableName := aws.ToString(input.TargetTableName)

	if sourceTableName == "" {
		return nil, NewValidationException("SourceTableName is required")
	}

	if targetTableName == "" {
		return nil, NewValidationException("TargetTableName is required")
	}

	sourceTable, err := db.getTable(ctx, sourceTableName)
	if err != nil {
		return nil, err
	}

	p, pitrEnabled, itemsCopy := snapshotSourceForPITR(sourceTable, input)

	if !pitrEnabled {
		return nil, NewValidationException(
			"point in time recovery is not enabled for table: " + sourceTableName,
		)
	}

	if itemsCopy == nil {
		return nil, NewInvalidRestoreTimeException(
			"requested RestoreDateTime is outside the available recovery window for table: " +
				sourceTableName,
		)
	}

	var throughputOverride *models.ProvisionedThroughput
	if input.ProvisionedThroughputOverride != nil {
		throughputOverride = &models.ProvisionedThroughput{
			ReadCapacityUnits:  input.ProvisionedThroughputOverride.ReadCapacityUnits,
			WriteCapacityUnits: input.ProvisionedThroughputOverride.WriteCapacityUnits,
		}
	}

	billingMode, provThroughput := resolveBillingAndThroughput(
		p.BillingMode,
		string(input.BillingModeOverride),
		p.ProvisionedThroughput,
		throughputOverride,
	)
	p.Items = itemsCopy
	p.BillingMode = billingMode
	p.ProvisionedThroughput = provThroughput
	p.GlobalSecondaryIndexes = resolveGSIOverride(p.GlobalSecondaryIndexes, input.GlobalSecondaryIndexOverride)

	sseEnabled, sseType, sseKMSMasterKeyArn := resolveSSEOverride(
		p.SSEEnabled, p.SSEType, p.SSEKMSMasterKeyArn, input.SSESpecificationOverride,
	)
	p.SSEEnabled, p.SSEType, p.SSEKMSMasterKeyArn = sseEnabled, sseType, sseKMSMasterKeyArn

	p.OnDemandMaxReadRRU, p.OnDemandMaxWriteRRU = resolveOnDemandThroughputOverride(
		p.OnDemandMaxReadRRU, p.OnDemandMaxWriteRRU, input.OnDemandThroughputOverride,
	)

	region := getRegionFromContext(ctx, db)
	newTable, newTableID, installErr := db.installRestoredTable(region, targetTableName, p)
	if installErr != nil {
		return nil, installErr
	}

	td := tableDescriptionToSDK(models.TableDescription{
		TableName: targetTableName, TableStatus: models.TableStatusActive,
		TableArn: newTable.TableArn, TableID: newTableID,
		KeySchema: p.KeySchema, AttributeDefinitions: p.AttributeDefinitions,
		GlobalSecondaryIndexes: buildGSIDescriptions(
			p.GlobalSecondaryIndexes,
			int64(len(itemsCopy)),
			newTable.TableArn,
		),
		LocalSecondaryIndexes: buildLSIDescriptions(p.LocalSecondaryIndexes, newTable.TableArn),
		BillingModeSummary:    billingModeSummary(billingMode),
		ItemCount:             len(itemsCopy),
	})
	applySSEDescription(td, sseEnabled, sseType, sseKMSMasterKeyArn)
	if p.OnDemandMaxReadRRU != nil || p.OnDemandMaxWriteRRU != nil {
		td.OnDemandThroughput = &sdktypes.OnDemandThroughput{
			MaxReadRequestUnits:  p.OnDemandMaxReadRRU,
			MaxWriteRequestUnits: p.OnDemandMaxWriteRRU,
		}
	}

	return &sdkdynamodb.RestoreTableToPointInTimeOutput{TableDescription: td}, nil
}

// BatchExecuteStatement executes multiple PartiQL statements and returns their results.
// It satisfies the StorageBackend interface using official AWS SDK v2 types.
//
// AWS limit: at most maxBatchExecuteStatements statements per call.
// The ConsistentRead flag on each statement is forwarded to the underlying
// Query / Scan execution so strongly-consistent reads are honoured.
func (db *InMemoryDB) BatchExecuteStatement(
	ctx context.Context,
	input *sdkdynamodb.BatchExecuteStatementInput,
) (*sdkdynamodb.BatchExecuteStatementOutput, error) {
	if input == nil {
		return nil, NewValidationException("BatchExecuteStatementInput must not be nil")
	}

	if len(input.Statements) > maxBatchExecuteStatements {
		return nil, NewValidationException(
			fmt.Sprintf("too many statements: %d exceeds the limit of %d",
				len(input.Statements), maxBatchExecuteStatements),
		)
	}

	runner := &partiQLRunner{backend: db}
	responses := make([]sdktypes.BatchStatementResponse, 0, len(input.Statements))

	for _, stmt := range input.Statements {
		params := make([]map[string]any, 0, len(stmt.Parameters))

		for _, p := range stmt.Parameters {
			// models.FromSDKAttributeValue always returns map[string]any or nil.
			if wireMap, ok := models.FromSDKAttributeValue(p).(map[string]any); ok {
				params = append(params, wireMap)
			}
		}

		req := executeStatementRequest{
			Statement:      aws.ToString(stmt.Statement),
			Parameters:     params,
			ConsistentRead: aws.ToBool(stmt.ConsistentRead),
		}

		result, err := runner.executeStatement(ctx, req)
		if err != nil {
			resp := sdktypes.BatchStatementResponse{
				Error: &sdktypes.BatchStatementError{
					Code:    sdktypes.BatchStatementErrorCodeEnum("StatementError"),
					Message: aws.String(err.Error()),
				},
			}
			if tableName := extractPartiQLTableName(req.Statement); tableName != "" {
				resp.TableName = aws.String(tableName)
			}

			responses = append(responses, resp)

			continue
		}

		resp := sdktypes.BatchStatementResponse{}
		if len(result.Items) > 0 {
			// BatchExecuteStatement returns at most one item per statement (AWS spec).
			// INSERT/UPDATE/DELETE return no item; SELECT returns the first matching item.
			sdkItem, convErr := models.ToSDKItem(result.Items[0])
			if convErr == nil {
				resp.Item = sdkItem
			}
		}

		responses = append(responses, resp)
	}

	return &sdkdynamodb.BatchExecuteStatementOutput{
		Responses: responses,
	}, nil
}
