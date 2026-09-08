package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	sdktypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

// backupARN builds the ARN for a DynamoDB backup.
// Format: arn:aws:dynamodb:{region}:{account}:table/{table}/backup/{timestamp}-{unique}.
// The unique suffix prevents ARN collisions when multiple backups are created in the same millisecond.
func backupARN(region, accountID, tableName string, ts time.Time) string {
	resource := fmt.Sprintf(
		"table/%s/backup/%016d-%s",
		tableName,
		ts.UnixMilli(),
		strings.ReplaceAll(uuid.New().String(), "-", "")[:exportIDSuffixLen],
	)

	return arn.Build("dynamodb", region, accountID, resource)
}

func (h *DynamoDBHandler) createBackup(ctx context.Context, body []byte) (any, error) {
	var req models.CreateBackupInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	out, err := h.Backend.CreateBackup(ctx, &sdkdynamodb.CreateBackupInput{
		TableName:  &req.TableName,
		BackupName: &req.BackupName,
	})
	if err != nil {
		return nil, err
	}

	bd := out.BackupDetails

	return &models.CreateBackupOutput{
		BackupDetails: models.BackupDetails{
			BackupArn:              aws.ToString(bd.BackupArn),
			BackupName:             aws.ToString(bd.BackupName),
			BackupStatus:           string(bd.BackupStatus),
			BackupType:             string(bd.BackupType),
			BackupCreationDateTime: float64(aws.ToTime(bd.BackupCreationDateTime).UTC().Unix()),
			BackupSizeBytes:        aws.ToInt64(bd.BackupSizeBytes),
		},
	}, nil
}

func (h *DynamoDBHandler) describeBackup(ctx context.Context, body []byte) (any, error) {
	var req models.DescribeBackupInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	if req.BackupArn == "" {
		return nil, NewValidationException("BackupArn is required")
	}

	out, err := h.Backend.DescribeBackup(ctx, &sdkdynamodb.DescribeBackupInput{
		BackupArn: &req.BackupArn,
	})
	if err != nil {
		return nil, err
	}

	bd := out.BackupDescription

	return &models.DescribeBackupOutput{
		BackupDescription: buildBackupDescriptionFromSDK(bd),
	}, nil
}

func (h *DynamoDBHandler) deleteBackup(ctx context.Context, body []byte) (any, error) {
	var req models.DeleteBackupInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	out, err := h.Backend.DeleteBackup(ctx, &sdkdynamodb.DeleteBackupInput{
		BackupArn: &req.BackupArn,
	})
	if err != nil {
		return nil, err
	}

	return &models.DeleteBackupOutput{
		BackupDescription: buildBackupDescriptionFromSDK(out.BackupDescription),
	}, nil
}

func (h *DynamoDBHandler) listBackups(ctx context.Context, body []byte) (any, error) {
	var req models.ListBackupsInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	db, ok := h.Backend.(*InMemoryDB)
	if !ok {
		return nil, NewInternalServerError("backup operations require in-memory backend")
	}

	region := h.regionFromHandlerContext(ctx)

	summaries := collectBackupSummariesRLocked(
		db,
		region,
		req.TableName,
		req.BackupType,
		req.TimeRangeLowerBound,
		req.TimeRangeUpperBound,
	)

	// Sort by creation time (then ARN) for deterministic ordering.
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].BackupCreationDateTime != summaries[j].BackupCreationDateTime {
			return summaries[i].BackupCreationDateTime < summaries[j].BackupCreationDateTime
		}

		return summaries[i].BackupArn < summaries[j].BackupArn
	})

	page, lastEvaluatedArn := paginateBackupSummaries(
		summaries,
		req.ExclusiveStartBackupArn,
		req.Limit,
	)

	return &models.ListBackupsOutput{
		BackupSummaries:        page,
		LastEvaluatedBackupArn: lastEvaluatedArn,
	}, nil
}

// collectBackupSummaries gathers matching backup summaries from the in-memory store.
// Must be called while holding db.mu (read or write lock).
// Only backups whose ARN encodes requestRegion are returned.
// timeRangeLower/Upper are Unix epoch seconds (float64); nil means no bound.
func collectBackupSummaries(
	db *InMemoryDB,
	requestRegion string,
	tableName, backupType string,
	timeRangeLower, timeRangeUpper *float64,
) []models.BackupSummary {
	all := db.backups.All()
	summaries := make([]models.BackupSummary, 0, len(all))

	for _, b := range all {
		if db.regionFromARN(b.BackupArn) != requestRegion {
			continue
		}

		if tableName != "" && b.TableName != tableName {
			continue
		}

		if backupType != "" && b.BackupType != backupType {
			continue
		}

		createdAt := b.CreationDateTime.UTC()

		if timeRangeLower != nil && createdAt.Before(time.Unix(int64(*timeRangeLower), 0).UTC()) {
			continue
		}

		if timeRangeUpper != nil && !createdAt.Before(time.Unix(int64(*timeRangeUpper), 0).UTC()) {
			continue
		}

		summaries = append(summaries, models.BackupSummary{
			BackupArn:              b.BackupArn,
			BackupName:             b.BackupName,
			BackupStatus:           b.BackupStatus,
			BackupType:             b.BackupType,
			BackupCreationDateTime: float64(b.CreationDateTime.UTC().Unix()),
			TableName:              b.TableName,
			TableArn:               b.TableArn,
			TableID:                b.TableID,
			BackupSizeBytes:        b.SizeBytes,
		})
	}

	return summaries
}

// collectBackupSummariesRLocked wraps collectBackupSummaries in a
// defer-protected db.mu.RLock, so a panic while filtering db.backups can never
// leave db.mu read-locked forever.
func collectBackupSummariesRLocked(
	db *InMemoryDB,
	requestRegion string,
	tableName, backupType string,
	timeRangeLower, timeRangeUpper *float64,
) []models.BackupSummary {
	db.mu.RLock("ListBackups")
	defer db.mu.RUnlock()

	return collectBackupSummaries(db, requestRegion, tableName, backupType, timeRangeLower, timeRangeUpper)
}

// paginateBackupSummaries applies cursor-based pagination to a sorted backup summary list.
// It returns the page and the last-evaluated ARN (empty if no more pages).
func paginateBackupSummaries(
	summaries []models.BackupSummary,
	startArn string,
	limit int,
) ([]models.BackupSummary, string) {
	// Apply ExclusiveStartBackupArn as the starting cursor.
	start := 0
	if startArn != "" {
		for i, s := range summaries {
			if s.BackupArn == startArn {
				start = i + 1

				break
			}
		}
	}

	// Apply pagination limit relative to the starting cursor.
	end := len(summaries)
	lastEvaluatedArn := ""

	if limit > 0 && start+limit < len(summaries) {
		end = start + limit
		lastEvaluatedArn = summaries[end-1].BackupArn
	}

	if start >= len(summaries) {
		return []models.BackupSummary{}, lastEvaluatedArn
	}

	return summaries[start:end], lastEvaluatedArn
}

// restoredTableParams holds the schema and data for a table restore operation.
type restoredTableParams struct {
	BillingMode            string
	SSEType                string
	SSEKMSMasterKeyArn     string
	StreamViewType         string
	OnDemandMaxReadRRU     *int64
	OnDemandMaxWriteRRU    *int64
	Items                  []map[string]any
	KeySchema              []models.KeySchemaElement
	AttributeDefinitions   []models.AttributeDefinition
	GlobalSecondaryIndexes []models.GlobalSecondaryIndex
	LocalSecondaryIndexes  []models.LocalSecondaryIndex
	ProvisionedThroughput  models.ProvisionedThroughputDescription
	SSEEnabled             bool
	StreamsEnabled         bool
}

// installRestoredTable creates the target table from p under db.mu.
// Returns the new Table + its ID, or ResourceInUseException if it already exists.
func (db *InMemoryDB) installRestoredTable(
	region, tableName string,
	p restoredTableParams,
) (*Table, string, error) {
	db.mu.Lock("RestoreTable")
	defer db.mu.Unlock()

	if _, tExists := db.tables.Get(tableKey(region, tableName)); tExists {
		return nil, "", NewResourceInUseException("table already exists: " + tableName)
	}

	newTableID := uuid.New().String()
	newTable := &Table{
		Name:                   tableName,
		TableID:                newTableID,
		KeySchema:              p.KeySchema,
		AttributeDefinitions:   p.AttributeDefinitions,
		GlobalSecondaryIndexes: p.GlobalSecondaryIndexes,
		LocalSecondaryIndexes:  p.LocalSecondaryIndexes,
		Items:                  p.Items,
		Status:                 models.TableStatusActive,
		CreationDateTime:       time.Now(),
		BillingMode:            p.BillingMode,
		SSEEnabled:             p.SSEEnabled,
		SSEType:                p.SSEType,
		SSEKMSMasterKeyArn:     p.SSEKMSMasterKeyArn,
		StreamsEnabled:         p.StreamsEnabled,
		StreamViewType:         p.StreamViewType,
		TableArn:               arn.Build("dynamodb", region, db.accountID, "table/"+tableName),
		mu:                     lockmetrics.New("ddb.table." + tableName),
		ProvisionedThroughput:  p.ProvisionedThroughput,
		OnDemandMaxReadRRU:     p.OnDemandMaxReadRRU,
		OnDemandMaxWriteRRU:    p.OnDemandMaxWriteRRU,
	}
	newTable.initializeIndexes()
	newTable.rebuildIndexes()

	db.tables.Put(newTable)

	return newTable, newTableID, nil
}

// getBackupRLocked returns the backup stored under backupArn (and whether it
// exists) under a defer-protected db.mu.RLock.
func (db *InMemoryDB) getBackupRLocked(backupArn string) (*Backup, bool) {
	db.mu.RLock("RestoreTableFromBackup.lookup")
	defer db.mu.RUnlock()

	return db.backups.Get(backupArn)
}

// toSDKProvisionedThroughputOverride converts the wire-format override to the
// SDK type. pt may be nil, matching an omitted request member.
func toSDKProvisionedThroughputOverride(pt *models.ProvisionedThroughput) *sdktypes.ProvisionedThroughput {
	if pt == nil {
		return nil
	}

	return &sdktypes.ProvisionedThroughput{
		ReadCapacityUnits:  pt.ReadCapacityUnits,
		WriteCapacityUnits: pt.WriteCapacityUnits,
	}
}

// toSDKGSIOverride converts the wire-format GlobalSecondaryIndexOverride to
// the SDK type, preserving the nil/empty distinction that
// models.ToSDKGlobalSecondaryIndexes collapses: nil means the override was
// omitted (restore keeps the source's GSIs), a non-nil empty slice means the
// caller explicitly asked to exclude every GSI from the restored table.
func toSDKGSIOverride(gsis []models.GlobalSecondaryIndex) []sdktypes.GlobalSecondaryIndex {
	if gsis == nil {
		return nil
	}

	out := models.ToSDKGlobalSecondaryIndexes(gsis)
	if out == nil {
		out = []sdktypes.GlobalSecondaryIndex{}
	}

	return out
}

func (h *DynamoDBHandler) restoreTableFromBackup(ctx context.Context, body []byte) (any, error) {
	var req models.RestoreTableFromBackupInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	if req.BackupArn == "" {
		return nil, NewValidationException("BackupArn is required")
	}

	if req.TargetTableName == "" {
		return nil, NewValidationException("TargetTableName is required")
	}

	out, err := h.Backend.RestoreTableFromBackup(ctx, &sdkdynamodb.RestoreTableFromBackupInput{
		BackupArn:                     &req.BackupArn,
		TargetTableName:               &req.TargetTableName,
		BillingModeOverride:           sdktypes.BillingMode(req.BillingModeOverride),
		ProvisionedThroughputOverride: toSDKProvisionedThroughputOverride(req.ProvisionedThroughputOverride),
		GlobalSecondaryIndexOverride:  toSDKGSIOverride(req.GlobalSecondaryIndexOverride),
		OnDemandThroughputOverride:    models.ToSDKOnDemandThroughput(req.OnDemandThroughputOverride),
		SSESpecificationOverride:      models.ToSDKSSESpecification(req.SSESpecificationOverride),
	})
	if err != nil {
		return nil, err
	}

	return &models.RestoreTableFromBackupOutput{
		TableDescription: models.FromSDKTableDescription(out.TableDescription),
	}, nil
}

// selectPITRItems returns the items to restore for a RestoreTableToPointInTime
// call. Caller must hold sourceTable.mu.RLock.
//
// Behaviour:
//   - UseLatestRestorableTime or RestoreDateTime nil → current items.
//   - RestoreDateTime set → newest snapshot whose Taken <= RestoreDateTime.
//   - No matching snapshot (e.g. requested time is before the table was created
//     or the snapshot window has rotated past it) → nil, signalling the caller
//     to return InvalidRestoreTimeException (real AWS returns this, not an
//     empty table, when RestoreDateTime falls outside the recoverable window).
func selectPITRItems(
	sourceTable *Table,
	input *sdkdynamodb.RestoreTableToPointInTimeInput,
) []map[string]any {
	if aws.ToBool(input.UseLatestRestorableTime) || input.RestoreDateTime == nil {
		return deepCopyItems(sourceTable.Items)
	}

	t := input.RestoreDateTime.UTC()

	// Newest snapshot at-or-before t. Snapshots are appended in time order so
	// scanning backwards is O(k) where k is the index from the end.
	for _, snap := range slices.Backward(sourceTable.PITRSnapshots) {
		if !snap.Taken.After(t) {
			return deepCopyItems(snap.Items)
		}
	}

	return nil
}

// toSDKRestoreDateTime converts the wire-format RestoreDateTime (Unix epoch
// seconds, with optional fractional part -- the shape the real AWS SDK's
// awsjson1_0 protocol emits) into a *time.Time. secs may be nil, matching an
// omitted request member.
func toSDKRestoreDateTime(secs *float64) *time.Time {
	if secs == nil {
		return nil
	}

	const nanosPerSec = float64(time.Second / time.Nanosecond)
	t := time.Unix(int64(*secs), int64((*secs-float64(int64(*secs)))*nanosPerSec)).UTC()

	return &t
}

func (h *DynamoDBHandler) restoreTableToPointInTime(ctx context.Context, body []byte) (any, error) {
	var req models.RestoreTableToPointInTimeInput
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	if req.SourceTableName == "" {
		return nil, NewValidationException("SourceTableName is required")
	}

	if req.TargetTableName == "" {
		return nil, NewValidationException("TargetTableName is required")
	}

	out, err := h.Backend.RestoreTableToPointInTime(ctx, &sdkdynamodb.RestoreTableToPointInTimeInput{
		SourceTableName:               &req.SourceTableName,
		TargetTableName:               &req.TargetTableName,
		BillingModeOverride:           sdktypes.BillingMode(req.BillingModeOverride),
		ProvisionedThroughputOverride: toSDKProvisionedThroughputOverride(req.ProvisionedThroughputOverride),
		UseLatestRestorableTime:       aws.Bool(req.UseLatestRestorableTime),
		RestoreDateTime:               toSDKRestoreDateTime(req.RestoreDateTime),
		GlobalSecondaryIndexOverride:  toSDKGSIOverride(req.GlobalSecondaryIndexOverride),
		OnDemandThroughputOverride:    models.ToSDKOnDemandThroughput(req.OnDemandThroughputOverride),
		SSESpecificationOverride:      models.ToSDKSSESpecification(req.SSESpecificationOverride),
	})
	if err != nil {
		return nil, err
	}

	return &models.RestoreTableToPointInTimeOutput{
		TableDescription: models.FromSDKTableDescription(out.TableDescription),
	}, nil
}

// snapshotSourceForPITR captures schema + metadata from sourceTable under RLock.
// Returns (params, pitrEnabled, items). Items is nil when no snapshot matched the
// requested RestoreDateTime.
func snapshotSourceForPITR(
	sourceTable *Table,
	input *sdkdynamodb.RestoreTableToPointInTimeInput,
) (restoredTableParams, bool, []map[string]any) {
	sourceTable.mu.RLock("RestoreTableToPointInTime")
	defer sourceTable.mu.RUnlock()

	pitrEnabled := sourceTable.PITREnabled
	itemsCopy := selectPITRItems(sourceTable, input)

	p := restoredTableParams{
		ProvisionedThroughput: sourceTable.ProvisionedThroughput,
		BillingMode:           sourceTable.BillingMode,
		SSEEnabled:            sourceTable.SSEEnabled,
		SSEType:               sourceTable.SSEType,
		SSEKMSMasterKeyArn:    sourceTable.SSEKMSMasterKeyArn,
		StreamsEnabled:        sourceTable.StreamsEnabled,
		StreamViewType:        sourceTable.StreamViewType,
		OnDemandMaxReadRRU:    sourceTable.OnDemandMaxReadRRU,
		OnDemandMaxWriteRRU:   sourceTable.OnDemandMaxWriteRRU,

		KeySchema: make([]models.KeySchemaElement, len(sourceTable.KeySchema))}
	copy(p.KeySchema, sourceTable.KeySchema)
	p.AttributeDefinitions = make(
		[]models.AttributeDefinition,
		len(sourceTable.AttributeDefinitions),
	)
	copy(p.AttributeDefinitions, sourceTable.AttributeDefinitions)
	p.GlobalSecondaryIndexes = make(
		[]models.GlobalSecondaryIndex,
		len(sourceTable.GlobalSecondaryIndexes),
	)
	copy(p.GlobalSecondaryIndexes, sourceTable.GlobalSecondaryIndexes)
	p.LocalSecondaryIndexes = make(
		[]models.LocalSecondaryIndex,
		len(sourceTable.LocalSecondaryIndexes),
	)
	copy(p.LocalSecondaryIndexes, sourceTable.LocalSecondaryIndexes)

	return p, pitrEnabled, itemsCopy
}

// buildBackupDescriptionFromSDK converts an SDK BackupDescription (as returned by the
// StorageBackend interface) into the wire-format models.BackupDescription.
func buildBackupDescriptionFromSDK(bd *sdktypes.BackupDescription) models.BackupDescription {
	if bd == nil {
		return models.BackupDescription{}
	}

	var details models.BackupDetails
	if bd.BackupDetails != nil {
		details = models.BackupDetails{
			BackupArn:    aws.ToString(bd.BackupDetails.BackupArn),
			BackupName:   aws.ToString(bd.BackupDetails.BackupName),
			BackupStatus: string(bd.BackupDetails.BackupStatus),
			BackupType:   string(bd.BackupDetails.BackupType),
			BackupCreationDateTime: float64(
				aws.ToTime(bd.BackupDetails.BackupCreationDateTime).UTC().Unix(),
			),
			BackupSizeBytes: aws.ToInt64(bd.BackupDetails.BackupSizeBytes),
		}
	}

	var src models.SourceTableDetails
	if bd.SourceTableDetails != nil {
		src = models.SourceTableDetails{
			TableName: aws.ToString(bd.SourceTableDetails.TableName),
			TableArn:  aws.ToString(bd.SourceTableDetails.TableArn),
			TableID:   aws.ToString(bd.SourceTableDetails.TableId),
			TableCreationDateTime: float64(
				aws.ToTime(bd.SourceTableDetails.TableCreationDateTime).UTC().Unix(),
			),
		}

		if pt := bd.SourceTableDetails.ProvisionedThroughput; pt != nil {
			src.ProvisionedThroughput = models.ProvisionedThroughput{
				ReadCapacityUnits:  pt.ReadCapacityUnits,
				WriteCapacityUnits: pt.WriteCapacityUnits,
			}
		}

		// Preserve key schema from SDK representation.
		for _, ks := range bd.SourceTableDetails.KeySchema {
			src.KeySchema = append(src.KeySchema, models.KeySchemaElement{
				AttributeName: aws.ToString(ks.AttributeName),
				KeyType:       string(ks.KeyType),
			})
		}
	}

	return models.BackupDescription{
		BackupDetails:      details,
		SourceTableDetails: src,
	}
}

// regionFromHandlerContext extracts the region from context using the
// regionContextKey, falling back to the central awsmeta identity and then the
// handler default.
func (h *DynamoDBHandler) regionFromHandlerContext(ctx context.Context) string {
	if region, ok := ctx.Value(regionContextKey{}).(string); ok && region != "" {
		return region
	}
	if region := awsmeta.Region(ctx); region != "" {
		return region
	}

	return h.DefaultRegion
}

// deepCopyItems returns a deep copy of all provided DynamoDB items.
// DynamoDB attribute values may contain nested maps and lists, so a simple
// shallow copy is insufficient. Uses recursive map copy for efficiency.
func deepCopyItems(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return []map[string]any{}
	}

	copied := make([]map[string]any, len(items))
	for i, item := range items {
		copied[i] = deepCopyItem(item)
	}

	return copied
}

// resolveBillingAndThroughput applies caller-supplied overrides to the sourced
// billing mode and provisioned throughput, returning the final values to use.
func resolveBillingAndThroughput(
	srcBilling string,
	billingOverride string,
	srcThroughput models.ProvisionedThroughputDescription,
	throughputOverride *models.ProvisionedThroughput,
) (string, models.ProvisionedThroughputDescription) {
	billing := srcBilling
	if billingOverride != "" {
		billing = billingOverride
	}

	pt := srcThroughput
	if throughputOverride != nil {
		var rc, wc int
		if throughputOverride.ReadCapacityUnits != nil {
			rc = int(*throughputOverride.ReadCapacityUnits)
		}
		if throughputOverride.WriteCapacityUnits != nil {
			wc = int(*throughputOverride.WriteCapacityUnits)
		}
		pt = models.ProvisionedThroughputDescription{
			ReadCapacityUnits:  rc,
			WriteCapacityUnits: wc,
		}
	} else if pt.ReadCapacityUnits == 0 {
		pt.ReadCapacityUnits = models.DefaultReadCapacity
		pt.WriteCapacityUnits = models.DefaultWriteCapacity
	}

	return billing, pt
}

// billingModeSummary returns a BillingModeSummaryDescription pointer when
// billingMode is non-empty, or nil otherwise.
func billingModeSummary(billingMode string) *models.BillingModeSummaryDescription {
	if billingMode == "" {
		return nil
	}

	return &models.BillingModeSummaryDescription{BillingMode: billingMode}
}
