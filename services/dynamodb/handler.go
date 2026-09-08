package dynamodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

const (
	opTransactWriteItems                = "TransactWriteItems"
	opUntagResource                     = "UntagResource"
	opUpdateContinuousBackups           = "UpdateContinuousBackups"
	opUpdateContributorInsights         = "UpdateContributorInsights"
	opUpdateGlobalTable                 = "UpdateGlobalTable"
	opUpdateGlobalTableSettings         = "UpdateGlobalTableSettings"
	opUpdateItem                        = "UpdateItem"
	opUpdateKinesisStreamingDestination = "UpdateKinesisStreamingDestination"
	opUpdateTable                       = "UpdateTable"
	opUpdateTableReplicaAutoScaling     = "UpdateTableReplicaAutoScaling"
	opUpdateTimeToLive                  = "UpdateTimeToLive"
)

const (
	statusActive = "ACTIVE"

	opBatchGetItem                        = "BatchGetItem"
	opBatchWriteItem                      = "BatchWriteItem"
	opCreateBackup                        = "CreateBackup"
	opCreateGlobalTable                   = "CreateGlobalTable"
	opCreateTable                         = "CreateTable"
	opDeleteBackup                        = "DeleteBackup"
	opDeleteItem                          = "DeleteItem"
	opDeleteResourcePolicy                = "DeleteResourcePolicy"
	opDeleteTable                         = "DeleteTable"
	opDescribeBackup                      = "DescribeBackup"
	opDescribeContinuousBackups           = "DescribeContinuousBackups"
	opDescribeContributorInsights         = "DescribeContributorInsights"
	opDescribeEndpoints                   = "DescribeEndpoints"
	opDescribeGlobalTable                 = "DescribeGlobalTable"
	opDescribeGlobalTableSettings         = "DescribeGlobalTableSettings"
	opDescribeImport                      = "DescribeImport"
	opDescribeKinesisStreamingDestination = "DescribeKinesisStreamingDestination"
	opDescribeLimits                      = "DescribeLimits"
	opDescribeTable                       = "DescribeTable"
	opDescribeTableReplicaAutoScaling     = "DescribeTableReplicaAutoScaling"
	opDescribeTimeToLive                  = "DescribeTimeToLive"
	opDisableKinesisStreamingDestination  = "DisableKinesisStreamingDestination"
	opEnableKinesisStreamingDestination   = "EnableKinesisStreamingDestination"
	opGetItem                             = "GetItem"
	opGetResourcePolicy                   = "GetResourcePolicy"
	opImportTable                         = "ImportTable"
	opListBackups                         = "ListBackups"
	opListContributorInsights             = "ListContributorInsights"
	opListGlobalTables                    = "ListGlobalTables"
	opListImports                         = "ListImports"
	opListTables                          = "ListTables"
	opListTagsOfResource                  = "ListTagsOfResource"
	opPutItem                             = "PutItem"
	opPutResourcePolicy                   = "PutResourcePolicy"
	opQuery                               = "Query"
	opRestoreTableFromBackup              = "RestoreTableFromBackup"
	opRestoreTableToPointInTime           = "RestoreTableToPointInTime"
	opScan                                = "Scan"
	opSearchVectors                       = "SearchVectors"
	opTagResource                         = "TagResource"
	opTransactGetItems                    = "TransactGetItems"
)

var ErrUnknownOperation = errors.New("UnknownOperationException")

// regionContextKey is used to store the AWS region in request context.
type regionContextKey struct{}

// WithRegion returns a derived context that carries the given AWS region.
// External callers (e.g. the DynamoDB Streams handler) use this to scope
// backend operations to the request's SigV4 region.
func WithRegion(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, regionContextKey{}, region)
}

// extractRegionFromAuth extracts the AWS region from the Authorization header.
// AWS Signature Version 4 has format: Credential=AKID/date/region/service/aws4_request
// Falls back to X-Amz-Region header if present, or uses the default region.
func extractRegionFromAuth(r *http.Request, defaultRegion string) string {
	return httputils.ExtractRegionFromRequest(r, defaultRegion)
}

// DynamoDBHandler handles HTTP requests for DynamoDB operations.
//
//nolint:revive // Stuttering preferred here for clarity per Plan.md
type DynamoDBHandler struct {
	Backend       StorageBackend
	janitor       *Janitor
	janitorCancel context.CancelFunc
	janitorDone   chan struct{}
	DefaultRegion string
	janitorMu     sync.Mutex
}

// NewHandler creates a new DynamoDB handler with the given storage backend.
func NewHandler(backend StorageBackend) *DynamoDBHandler {
	h := &DynamoDBHandler{
		Backend:       backend,
		DefaultRegion: config.DefaultRegion,
	}

	return h
}

// WithJanitor attaches a background janitor to the handler.
// The optional janitorTimeout parameter bounds each individual janitor task;
// zero (or omitted) disables per-task timeouts.
func (h *DynamoDBHandler) WithJanitor(
	settings Settings,
	janitorTimeout ...time.Duration,
) *DynamoDBHandler {
	h.DefaultRegion = settings.DefaultRegion
	if h.DefaultRegion == "" {
		h.DefaultRegion = config.DefaultRegion
	}
	if memBackend, ok := h.Backend.(*InMemoryDB); ok {
		memBackend.SetDefaultRegion(h.DefaultRegion)
		j := NewJanitor(memBackend, settings)
		if len(janitorTimeout) > 0 {
			j.TaskTimeout = janitorTimeout[0]
		}
		h.janitor = j
	}

	return h
}

// StartWorker starts the background janitor if it is configured.
func (h *DynamoDBHandler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		h.janitorMu.Lock()
		if h.janitorDone != nil {
			h.janitorMu.Unlock()

			return nil
		}

		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		h.janitorCancel = cancel
		h.janitorDone = done
		h.janitorMu.Unlock()

		go func() {
			defer close(done)
			h.janitor.Run(runCtx)
		}()
	}

	return nil
}

// Shutdown stops the janitor worker and waits for it to exit (or until ctx expires).
func (h *DynamoDBHandler) Shutdown(ctx context.Context) {
	h.janitorMu.Lock()
	cancel := h.janitorCancel
	done := h.janitorDone
	h.janitorCancel = nil
	h.janitorDone = nil
	h.janitorMu.Unlock()

	if cancel == nil || done == nil {
		return
	}

	cancel()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

var (
	_ service.BackgroundWorker = (*DynamoDBHandler)(nil)
	_ service.Shutdowner       = (*DynamoDBHandler)(nil)
)

// GetSupportedOperations returns a sorted list of supported DynamoDB operations.
func (h *DynamoDBHandler) GetSupportedOperations() []string {
	return []string{
		"BatchExecuteStatement",
		opBatchGetItem,
		opBatchWriteItem,
		opCreateBackup,
		opCreateGlobalTable,
		opCreateTable,
		opDeleteBackup,
		opDeleteItem,
		opDeleteResourcePolicy,
		opDeleteTable,
		opDescribeBackup,
		opDescribeContributorInsights,
		opDescribeContinuousBackups,
		opDescribeEndpoints,
		"DescribeExport",
		opDescribeGlobalTable,
		opDescribeGlobalTableSettings,
		opDescribeImport,
		opDescribeKinesisStreamingDestination,
		opDescribeLimits,
		opDescribeTable,
		opDescribeTableReplicaAutoScaling,
		opDescribeTimeToLive,
		opDisableKinesisStreamingDestination,
		opEnableKinesisStreamingDestination,
		"ExecuteStatement",
		"ExecuteTransaction",
		"ExportTableToPointInTime",
		opGetItem,
		opGetResourcePolicy,
		opImportTable,
		opListBackups,
		opListContributorInsights,
		"ListExports",
		opListGlobalTables,
		opListImports,
		opListTables,
		opListTagsOfResource,
		opPutItem,
		opPutResourcePolicy,
		opQuery,
		opRestoreTableFromBackup,
		opRestoreTableToPointInTime,
		opScan,
		opSearchVectors,
		opTagResource,
		opTransactGetItems,
		opTransactWriteItems,
		opUntagResource,
		opUpdateContinuousBackups,
		opUpdateContributorInsights,
		opUpdateGlobalTable,
		opUpdateGlobalTableSettings,
		opUpdateItem,
		opUpdateKinesisStreamingDestination,
		opUpdateTable,
		opUpdateTableReplicaAutoScaling,
		opUpdateTimeToLive,
	}
}

// Regions returns all regions with tables in the backend.
// Returns an empty slice when not using the in-memory backend.
func (h *DynamoDBHandler) Regions() []string {
	if b, ok := h.Backend.(*InMemoryDB); ok {
		return b.Regions()
	}

	return []string{}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *DynamoDBHandler) ChaosServiceName() string { return "dynamodb" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *DynamoDBHandler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this DynamoDB instance handles.
func (h *DynamoDBHandler) ChaosRegions() []string {
	regions := h.Regions()
	if len(regions) == 0 {
		return []string{h.DefaultRegion}
	}

	return regions
}

// TableNamesByRegion returns table names in the given region (all if empty).
// Returns an empty slice when not using the in-memory backend.
func (h *DynamoDBHandler) TableNamesByRegion(region string) []string {
	if b, ok := h.Backend.(*InMemoryDB); ok {
		return b.TableNamesByRegion(region)
	}

	return []string{}
}

// DescribeTableInRegion returns a table from the backend for a specific region.
// Returns nil when not using the in-memory backend or when the table is not found.
func (h *DynamoDBHandler) DescribeTableInRegion(region, tableName string) *Table {
	b, ok := h.Backend.(*InMemoryDB)
	if !ok {
		return nil
	}

	table, exists := b.GetTableInRegion(tableName, region)
	if !exists {
		return nil
	}

	return table
}

// Handler is the Echo HTTP handler for DynamoDB operations.
func (h *DynamoDBHandler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		if c.Request().Method == http.MethodGet && c.Request().URL.Path == "/" {
			ops := h.GetSupportedOperations()

			return c.JSON(http.StatusOK, ops)
		}

		if c.Request().Method != http.MethodPost {
			return writeDynamoDBDispatchError(c, http.StatusMethodNotAllowed,
				"UnknownOperationException", "Method not allowed")
		}

		target := c.Request().Header.Get("X-Amz-Target")
		if target == "" {
			return writeDynamoDBDispatchError(c, http.StatusBadRequest,
				"UnknownOperationException", "Missing X-Amz-Target")
		}

		const targetParts = 2
		parts := strings.Split(target, ".")
		if len(parts) != targetParts {
			return writeDynamoDBDispatchError(c, http.StatusBadRequest,
				"UnknownOperationException", "Invalid X-Amz-Target")
		}
		action := parts[1]

		// Resolve region from the central awsmeta identity (populated by the global
		// middleware), falling back to local SigV4/header extraction when absent.
		region := awsmeta.Region(ctx)
		if region == "" {
			region = extractRegionFromAuth(c.Request(), h.DefaultRegion)
		}
		ctx = context.WithValue(ctx, regionContextKey{}, region)

		if service.IsCBORRequest(c.Request()) {
			return h.handleCBORRequest(ctx, c, log, action)
		}

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "failed to read request body", "error", err)

			return writeDynamoDBDispatchError(c, http.StatusInternalServerError,
				"InternalFailure", "internal server error")
		}

		log.DebugContext(ctx, "DynamoDB request", "action", action, "body", string(body))

		response, reqErr := h.dispatch(ctx, action, body)
		if reqErr != nil {
			return h.handleError(ctx, c, action, reqErr)
		}

		payload, err := json.Marshal(response)
		if err != nil {
			log.ErrorContext(ctx, "failed to marshal JSON response", "error", err)

			return writeDynamoDBDispatchError(c, http.StatusInternalServerError,
				"InternalFailure", "internal server error")
		}

		checksum := crc32.ChecksumIEEE(payload)
		c.Response().Header().Set("X-Amz-Crc32", strconv.FormatUint(uint64(checksum), 10))
		c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

		return c.JSONBlob(http.StatusOK, payload)
	}
}

// Name returns the service identifier.
func (h *DynamoDBHandler) Name() string {
	return "DynamoDB"
}

// Purge implements service.Purgeable by deleting resources older than cutoff.
func (h *DynamoDBHandler) Purge(ctx context.Context, cutoff time.Time) {
	if db, ok := h.Backend.(*InMemoryDB); ok {
		db.Purge(ctx, cutoff)
	}
}

// RouteMatcher returns a matcher for DynamoDB requests (by X-Amz-Target header).
func (h *DynamoDBHandler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, "DynamoDB_")
	}
}

// MatchPriority returns the priority for the DynamoDB matcher.
// Header-based matchers have high priority (100).
func (h *DynamoDBHandler) MatchPriority() int {
	return service.PriorityHeaderExact
}

// ExtractOperation extracts the DynamoDB operation from the X-Amz-Target header.
func (h *DynamoDBHandler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	const actionParts = 2
	if len(parts) == actionParts {
		return parts[1]
	}

	return "unknown"
}

// ExtractResource extracts the table name from the DynamoDB request body.
func (h *DynamoDBHandler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	if tbl, exists := data["TableName"]; exists {
		if tblStr, ok := tbl.(string); ok && tblStr != "" {
			return tblStr
		}
	}

	// Backup operations carry BackupArn instead of TableName.
	if arnVal, exists := data["BackupArn"]; exists {
		if arnStr, ok := arnVal.(string); ok && arnStr != "" {
			return extractTableFromBackupARN(arnStr)
		}
	}

	return ""
}

// extractTableFromBackupARN returns the table name embedded in a DynamoDB backup ARN.
// ARN format: arn:aws:dynamodb:REGION:ACCOUNT:table/NAME/backup/SUFFIX
// Returns the full ARN string unchanged if the expected structure is not found.
func extractTableFromBackupARN(arnStr string) string {
	// Resource component follows the last ':' in the ARN.
	if idx := strings.LastIndex(arnStr, ":"); idx >= 0 {
		resource := arnStr[idx+1:]
		if strings.HasPrefix(resource, "table/") {
			rest := resource[len("table/"):]
			if tableName, _, found := strings.Cut(rest, "/"); found {
				return tableName
			}

			return rest
		}
	}

	return arnStr
}

func (h *DynamoDBHandler) dispatch(ctx context.Context, action string, body []byte) (any, error) {
	switch action {
	case opCreateTable,
		opDeleteTable,
		opDescribeTable,
		opListTables,
		opTagResource,
		opUntagResource,
		opListTagsOfResource,
		opUpdateTable,
		opUpdateTimeToLive,
		opDescribeTimeToLive:
		return h.dispatchTableOps(ctx, action, body)
	case opPutItem,
		opGetItem,
		opDeleteItem,
		opUpdateItem,
		opQuery,
		opScan,
		opSearchVectors,
		opBatchGetItem,
		opBatchWriteItem:
		return h.dispatchItemOps(ctx, action, body)
	case opTransactWriteItems, opTransactGetItems:
		return h.dispatchTransactOps(ctx, action, body)
	case "ExecuteStatement":
		return h.handleExecuteStatement(ctx, body)
	case "BatchExecuteStatement":
		return h.handleBatchExecuteStatement(ctx, body)
	case opDescribeContinuousBackups,
		opUpdateContinuousBackups,
		opCreateBackup,
		opDescribeBackup,
		opDeleteBackup,
		opListBackups,
		opRestoreTableFromBackup,
		opRestoreTableToPointInTime,
		opDescribeTableReplicaAutoScaling:
		return h.dispatchBackupOps(ctx, action, body)
	case "ExportTableToPointInTime":
		return h.exportTableToPointInTime(ctx, body)
	case "DescribeExport":
		return h.describeExport(ctx, body)
	case "ListExports":
		return h.listExports(ctx, body)
	case opCreateGlobalTable,
		opDescribeGlobalTable,
		opDescribeGlobalTableSettings,
		opListGlobalTables,
		opUpdateGlobalTable,
		opUpdateGlobalTableSettings,
		opEnableKinesisStreamingDestination,
		opDescribeKinesisStreamingDestination,
		opDisableKinesisStreamingDestination,
		opUpdateKinesisStreamingDestination,
		opDescribeLimits,
		opDescribeEndpoints,
		opDescribeContributorInsights,
		opListContributorInsights,
		opUpdateContributorInsights,
		opUpdateTableReplicaAutoScaling,
		opDeleteResourcePolicy,
		opGetResourcePolicy,
		opPutResourcePolicy,
		opDescribeImport,
		opImportTable,
		opListImports:
		return h.dispatchExtraOps(ctx, action, body)
	case "ExecuteTransaction":
		return h.handleExecuteTransaction(ctx, body)
	default:
		return nil, fmt.Errorf("%w:%s", ErrUnknownOperation, action)
	}
}

func (h *DynamoDBHandler) dispatchBackupOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	switch action {
	case opDescribeContinuousBackups:
		return h.describeContinuousBackups(ctx, body)
	case opUpdateContinuousBackups:
		return h.updateContinuousBackups(ctx, body)
	case opCreateBackup:
		return h.createBackup(ctx, body)
	case opDescribeBackup:
		return h.describeBackup(ctx, body)
	case opDeleteBackup:
		return h.deleteBackup(ctx, body)
	case opListBackups:
		return h.listBackups(ctx, body)
	case opRestoreTableFromBackup:
		return h.restoreTableFromBackup(ctx, body)
	case opRestoreTableToPointInTime:
		return h.restoreTableToPointInTime(ctx, body)
	case opDescribeTableReplicaAutoScaling:
		return h.describeTableReplicaAutoScaling(ctx, body)
	default:
		return nil, fmt.Errorf("%w:%s", ErrUnknownOperation, action)
	}
}

// Helper for operations where Adapter allows error.
func handleOpErr[WireIn any, SDKIn any, SDKOut any, WireOut any](
	ctx context.Context,
	action string,
	body []byte,
	toSDK func(*WireIn) (*SDKIn, error),
	doOp func(context.Context, *SDKIn) (*SDKOut, error),
	fromSDK func(*SDKOut) *WireOut,
) (any, error) {
	log := logger.Load(ctx)

	var input WireIn
	if len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}
	}

	debugEnabled := log.Enabled(ctx, slog.LevelDebug)
	if debugEnabled {
		inputJSON, _ := json.Marshal(input)
		log.DebugContext(ctx, "handler input", "action", action, "input", string(inputJSON))
	}

	sdkInput, err := toSDK(&input)
	if err != nil {
		return nil, err
	}
	sdkOutput, err := doOp(ctx, sdkInput)
	if err != nil {
		return nil, err
	}

	wireOutput := fromSDK(sdkOutput)

	if debugEnabled {
		outputJSON, _ := json.Marshal(wireOutput)
		log.DebugContext(ctx, "handler output", "action", action, "output", string(outputJSON))
	}

	return wireOutput, nil
}

// Helper for operations where Adapter does not return error.
func handleOp[WireIn any, SDKIn any, SDKOut any, WireOut any](
	ctx context.Context,
	action string,
	body []byte,
	toSDK func(*WireIn) *SDKIn,
	doOp func(context.Context, *SDKIn) (*SDKOut, error),
	fromSDK func(*SDKOut) *WireOut,
) (any, error) {
	log := logger.Load(ctx)

	var input WireIn
	if len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}
	}

	debugEnabled := log.Enabled(ctx, slog.LevelDebug)
	if debugEnabled {
		inputJSON, _ := json.Marshal(input)
		log.DebugContext(ctx, "handler input", "action", action, "input", string(inputJSON))
	}

	sdkInput := toSDK(&input)
	sdkOutput, err := doOp(ctx, sdkInput)
	if err != nil {
		return nil, err
	}

	wireOutput := fromSDK(sdkOutput)

	if debugEnabled {
		outputJSON, _ := json.Marshal(wireOutput)
		log.DebugContext(ctx, "handler output", "action", action, "output", string(outputJSON))
	}

	return wireOutput, nil
}

func (h *DynamoDBHandler) dispatchTableOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	// Validate table name from wire payload before dispatching.
	// Tests call InMemoryDB methods directly (short names acceptable there);
	// wire-level requests must satisfy the 3-255 char constraint.
	if err := validateTableNameFromBody(body); err != nil {
		return nil, err
	}

	switch action {
	case opCreateTable:
		return handleOp(
			ctx, action, body,
			models.ToSDKCreateTableInput, h.Backend.CreateTable, models.FromSDKCreateTableOutput,
		)
	case opDeleteTable:
		return handleOp(
			ctx, action, body,
			models.ToSDKDeleteTableInput, h.Backend.DeleteTable, models.FromSDKDeleteTableOutput,
		)
	case opDescribeTable:
		return handleOp(
			ctx,
			action,
			body,
			models.ToSDKDescribeTableInput,
			h.Backend.DescribeTable,
			models.FromSDKDescribeTableOutput,
		)
	case opListTables:
		return handleOp(
			ctx, action, body,
			models.ToSDKListTablesInput, h.Backend.ListTables, models.FromSDKListTablesOutput,
		)
	case opUpdateTable:
		return handleOpErr(
			ctx, action, body,
			models.ToSDKUpdateTableInput, h.Backend.UpdateTable, models.FromSDKUpdateTableOutput,
		)
	case opTagResource:
		return handleOpErr(
			ctx, action, body,
			models.ToSDKTagResourceInput, h.Backend.TagResource, models.FromSDKTagResourceOutput,
		)
	case opUntagResource:
		return handleOpErr(
			ctx,
			action,
			body,
			models.ToSDKUntagResourceInput,
			h.Backend.UntagResource,
			models.FromSDKUntagResourceOutput,
		)
	case opListTagsOfResource:
		return handleOpErr(
			ctx,
			action,
			body,
			models.ToSDKListTagsOfResourceInput,
			h.Backend.ListTagsOfResource,
			models.FromSDKListTagsOfResourceOutput,
		)
	case opUpdateTimeToLive:
		return handleOp(
			ctx,
			action,
			body,
			models.ToSDKUpdateTimeToLiveInput,
			h.Backend.UpdateTimeToLive,
			models.FromSDKUpdateTimeToLiveOutput,
		)
	case opDescribeTimeToLive:
		return handleOp(
			ctx,
			action,
			body,
			models.ToSDKDescribeTimeToLiveInput,
			h.Backend.DescribeTimeToLive,
			models.FromSDKDescribeTimeToLiveOutput,
		)
	default:
		return nil, fmt.Errorf("%w:%s", ErrUnknownOperation, action)
	}
}

func (h *DynamoDBHandler) dispatchItemOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	switch action {
	case opPutItem:
		return handleOpErr(
			ctx, action, body,
			toSDKPutItemInputChecked, h.Backend.PutItem, models.FromSDKPutItemOutput,
		)
	case opGetItem:
		return handleOpErr(
			ctx, action, body,
			models.ToSDKGetItemInput, h.Backend.GetItem, models.FromSDKGetItemOutput,
		)
	case opDeleteItem:
		return handleOpErr(
			ctx, action, body,
			toSDKDeleteItemInputChecked, h.Backend.DeleteItem, models.FromSDKDeleteItemOutput,
		)
	case opScan:
		return handleOpErr(
			ctx, action, body,
			toSDKScanInputChecked, h.Backend.Scan, models.FromSDKScanOutput,
		)
	case opUpdateItem:
		return handleOpErr(
			ctx, action, body,
			toSDKUpdateItemInputChecked, h.Backend.UpdateItem, models.FromSDKUpdateItemOutput,
		)
	case opQuery:
		return handleOpErr(
			ctx, action, body,
			toSDKQueryInputChecked, h.Backend.Query, models.FromSDKQueryOutput,
		)
	case opSearchVectors:
		return handleOpErr(
			ctx, action, body,
			toSDKSearchVectorsInputChecked, h.Backend.SearchVectors, models.FromSDKSearchVectorsOutput,
		)
	case opBatchGetItem:
		return handleOpErr(
			ctx, action, body,
			models.ToSDKBatchGetItemInput, h.Backend.BatchGetItem, models.FromSDKBatchGetItemOutput,
		)
	case opBatchWriteItem:
		return handleOpErr(
			ctx,
			action,
			body,
			models.ToSDKBatchWriteItemInput,
			h.Backend.BatchWriteItem,
			models.FromSDKBatchWriteItemOutput,
		)
	default:
		return nil, fmt.Errorf("%w:%s", ErrUnknownOperation, action)
	}
}

func (h *DynamoDBHandler) dispatchTransactOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	switch action {
	case opTransactWriteItems:
		return handleOpErr(
			ctx,
			action,
			body,
			toSDKTransactWriteItemsInputChecked,
			h.Backend.TransactWriteItems,
			models.FromSDKTransactWriteItemsOutput,
		)
	case opTransactGetItems:
		return handleOpErr(
			ctx,
			action,
			body,
			models.ToSDKTransactGetItemsInput,
			h.Backend.TransactGetItems,
			models.FromSDKTransactGetItemsOutput,
		)
	default:
		return nil, fmt.Errorf("%w:%s", ErrUnknownOperation, action)
	}
}

// validateTableNameFromBody extracts "TableName" from the JSON body and checks it
// against the DynamoDB table-name constraints. Returns nil when the body has no
// TableName field (caller handles the missing-name error separately).
func validateTableNameFromBody(body []byte) error {
	var req struct {
		TableName string `json:"TableName"`
	}

	_ = json.Unmarshal(body, &req)
	if req.TableName == "" {
		return nil // missing table name is handled downstream
	}

	return validateTableName(req.TableName)
}

// writeDynamoDBDispatchError writes a JSON-RPC 1.0 error envelope for a
// failure in Handler() itself (bad method, missing/malformed X-Amz-Target,
// body-read or marshal failure) -- framework-level errors that never reach
// dispatch/handleError. These previously went out as bare text/plain, which
// smithy-go's JSON-RPC error decoder (aws-sdk-go-v2@v1.43.4
// aws/protocol/restjson.GetErrorInfo, __type/message) cannot read: every
// such response reached a client as smithy.GenericAPIError{Code:"UnknownError"}
// (gopherstack-wlo1).
func writeDynamoDBDispatchError(c *echo.Context, status int, errType, message string) error {
	body, err := json.Marshal(service.JSONErrorResponse{Type: errType, Message: message})
	if err != nil {
		return err
	}

	checksum := crc32.ChecksumIEEE(body)
	c.Response().Header().Set("X-Amz-Crc32", strconv.FormatUint(uint64(checksum), 10))
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

	return c.JSONBlob(status, body)
}

func (h *DynamoDBHandler) handleError(
	ctx context.Context,
	c *echo.Context,
	action string,
	reqErr error,
) error {
	log := logger.Load(ctx)

	if strings.HasPrefix(reqErr.Error(), "UnknownOperationException:") {
		log.WarnContext(ctx, "Unknown action", "action", action)
		body := []byte(
			`{"__type":"com.amazon.coral.service#UnknownOperationException","message":"Action not supported"}`,
		)
		checksum := crc32.ChecksumIEEE(body)
		c.Response().Header().Set("X-Amz-Crc32", strconv.FormatUint(uint64(checksum), 10))
		c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

		return c.JSONBlob(http.StatusBadRequest, body)
	}

	log.ErrorContext(ctx, "Error handling action", "action", action, "error", reqErr)

	statusCode, awsErr := h.classifyError(reqErr)

	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

	payload, _ := json.Marshal(awsErr)
	checksum := crc32.ChecksumIEEE(payload)
	c.Response().Header().Set("X-Amz-Crc32", strconv.FormatUint(uint64(checksum), 10))

	return c.JSONBlob(statusCode, payload)
}

func (h *DynamoDBHandler) classifyError(reqErr error) (int, *Error) {
	// Simple error classification wrapping
	// If it's already a DynamoDB error type/struct, use it.
	// But our internal implementation returns native go errors or custom structs.
	// We need to map them to Wire Error struct.

	if wireErr, ok := errors.AsType[*Error](reqErr); ok {
		// Map type to status code. Most DynamoDB errors return 400.
		if wireErr.Type == errInternalServerErrorType {
			return http.StatusInternalServerError, wireErr
		}

		return http.StatusBadRequest, wireErr
	}

	// Fallback
	var syntaxErr *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError
	if errors.As(reqErr, &syntaxErr) || errors.As(reqErr, &unmarshalTypeError) {
		return http.StatusBadRequest, NewValidationException(
			"JSON Error: " + reqErr.Error(),
		)
	}

	errStr := reqErr.Error()
	if strings.Contains(errStr, "json:") || strings.Contains(errStr, "unmarshal") {
		return http.StatusBadRequest, NewValidationException("JSON Error: " + errStr)
	}

	return http.StatusInternalServerError, &Error{
		Type:    errInternalServerErrorType,
		Message: reqErr.Error(),
	}
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *DynamoDBHandler) Reset() {
	if db, ok := h.Backend.(*InMemoryDB); ok {
		db.Reset()
	}
}

// dispatchExtraOps routes non-CRUD administrative and integration operations (global tables,
// Kinesis destinations, contributor insights, resource policies, and imports) to keep the primary
// data plane dispatch focused.
func (h *DynamoDBHandler) dispatchExtraOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	res, err := h.dispatchGlobalTableOps(ctx, action, body)
	if !errors.Is(err, ErrUnknownOperation) {
		return res, err
	}

	res, err = h.dispatchKinesisOps(ctx, action, body)
	if !errors.Is(err, ErrUnknownOperation) {
		return res, err
	}

	res, err = h.dispatchContribAndPolicyOps(ctx, action, body)
	if !errors.Is(err, ErrUnknownOperation) {
		return res, err
	}

	res, err = h.dispatchImportOps(ctx, action, body)
	if !errors.Is(err, ErrUnknownOperation) {
		return res, err
	}

	return nil, fmt.Errorf("%w:%s", ErrUnknownOperation, action)
}

func (h *DynamoDBHandler) dispatchGlobalTableOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	switch action {
	case opCreateGlobalTable:
		return h.handleCreateGlobalTable(ctx, body)
	case opDescribeGlobalTable:
		return h.handleDescribeGlobalTable(ctx, body)
	case opDescribeGlobalTableSettings:
		return h.handleDescribeGlobalTableSettings(ctx, body)
	case opListGlobalTables:
		return h.handleListGlobalTables(ctx, body)
	case opUpdateGlobalTable:
		return h.handleUpdateGlobalTable(ctx, body)
	case opUpdateGlobalTableSettings:
		return h.handleUpdateGlobalTableSettings(ctx, body)
	default:
		return nil, ErrUnknownOperation
	}
}

func (h *DynamoDBHandler) dispatchKinesisOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	switch action {
	case opEnableKinesisStreamingDestination:
		return h.handleEnableKinesisStreamingDestination(ctx, body)
	case opDescribeKinesisStreamingDestination:
		return h.handleDescribeKinesisStreamingDestination(ctx, body)
	case opDisableKinesisStreamingDestination:
		return h.handleDisableKinesisStreamingDestination(ctx, body)
	case opUpdateKinesisStreamingDestination:
		return h.handleUpdateKinesisStreamingDestination(ctx, body)
	default:
		return nil, ErrUnknownOperation
	}
}

func (h *DynamoDBHandler) dispatchContribAndPolicyOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	switch action {
	case opDescribeLimits:
		return h.handleDescribeLimits(ctx)
	case opDescribeEndpoints:
		return h.handleDescribeEndpoints(ctx)
	case opDescribeContributorInsights:
		return h.handleDescribeContributorInsights(ctx, body)
	case opListContributorInsights:
		return h.handleListContributorInsights(ctx, body)
	case opUpdateContributorInsights:
		return h.handleUpdateContributorInsights(ctx, body)
	case opUpdateTableReplicaAutoScaling:
		return h.handleUpdateTableReplicaAutoScaling(ctx, body)
	case opGetResourcePolicy:
		return h.handleGetResourcePolicy(ctx, body)
	case opPutResourcePolicy:
		return h.handlePutResourcePolicy(ctx, body)
	case opDeleteResourcePolicy:
		return h.handleDeleteResourcePolicy(ctx, body)
	default:
		return nil, ErrUnknownOperation
	}
}

func (h *DynamoDBHandler) dispatchImportOps(
	ctx context.Context,
	action string,
	body []byte,
) (any, error) {
	switch action {
	case opDescribeImport:
		return h.handleDescribeImport(ctx, body)
	case opImportTable:
		return h.handleImportTable(ctx, body)
	case opListImports:
		return h.handleListImports(ctx, body)
	default:
		return nil, ErrUnknownOperation
	}
}
