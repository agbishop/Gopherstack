package rdsdata

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyTypeField        = "__type"
	keyMessageField     = "message"
	keyResourceARNField = "resourceArn"
	keySecretARNField   = "secretArn"
)

const (
	opBatchExecuteStatement = "BatchExecuteStatement"
	opBeginTransaction      = "BeginTransaction"
	opCommitTransaction     = "CommitTransaction"
	opExecuteSQL            = "ExecuteSql"
	opExecuteStatement      = "ExecuteStatement"
	opRollbackTransaction   = "RollbackTransaction"
)

// RecordsFormatType enum values (types.RecordsFormatType in the real SDK).
const (
	formatRecordsAsNone = "NONE"
	formatRecordsAsJSON = "JSON"
)

const (
	rdsdataService       = "rds-data"
	rdsdataMatchPriority = 87

	pathExecute             = "/Execute"
	pathBatchExecute        = "/BatchExecute"
	pathBeginTransaction    = "/BeginTransaction"
	pathCommitTransaction   = "/CommitTransaction"
	pathRollbackTransaction = "/RollbackTransaction"
	pathExecuteSQL          = "/ExecuteSql"
)

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request")
)

// Handler is the HTTP handler for the RDS Data REST API.
type Handler struct {
	Backend   StorageBackend
	janitor   *Janitor
	AccountID string
	Region    string
}

// NewHandler creates a new RDS Data handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend:   backend,
		AccountID: backend.AccountID(),
		Region:    backend.Region(),
	}
}

// WithJanitor attaches a background Janitor that expires transactions per
// BeginTransaction's documented lifetime (see janitor.go). backend must be
// the same *InMemoryBackend passed to NewHandler -- the Janitor needs direct
// map access the StorageBackend interface doesn't expose.
func (h *Handler) WithJanitor(backend *InMemoryBackend, interval, idleTimeout, maxLifetime time.Duration,
	taskTimeout ...time.Duration,
) *Handler {
	j := NewJanitor(backend, interval, idleTimeout, maxLifetime)
	if len(taskTimeout) > 0 {
		j.TaskTimeout = taskTimeout[0]
	}

	h.janitor = j

	return h
}

// StartWorker starts the background janitor if configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Reset clears all handler and backend state. Useful for test isolation.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "RDSData" }

// GetSupportedOperations returns the list of supported RDS Data operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opBatchExecuteStatement,
		opBeginTransaction,
		opCommitTransaction,
		opExecuteSQL,
		opExecuteStatement,
		opRollbackTransaction,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return rdsdataService }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// RouteMatcher returns a function that matches RDS Data API requests.
// All path-based matches are gated on the SigV4 service name to prevent
// routing conflicts with other services that share similar REST paths.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		if httputils.ExtractServiceFromRequest(c.Request()) != rdsdataService {
			return false
		}

		path := c.Request().URL.Path

		switch path {
		case pathExecute, pathBatchExecute, pathBeginTransaction,
			pathCommitTransaction, pathRollbackTransaction, pathExecuteSQL:
			return true
		}

		return false
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return rdsdataMatchPriority }

// ExtractOperation extracts the operation name from the request path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	switch c.Request().URL.Path {
	case pathExecute:
		return opExecuteStatement
	case pathBatchExecute:
		return opBatchExecuteStatement
	case pathBeginTransaction:
		return opBeginTransaction
	case pathCommitTransaction:
		return opCommitTransaction
	case pathRollbackTransaction:
		return opRollbackTransaction
	case pathExecuteSQL:
		return opExecuteSQL
	default:
		return "Unknown"
	}
}

// ExtractResource always returns an empty string for the RDS Data API.
// The resource is identified by a resourceArn in the request body, but
// parsing the body here would require double-buffering; metrics and logging
// can rely on ExtractOperation instead.
func (h *Handler) ExtractResource(_ *echo.Context) string {
	return ""
}

// Handler returns the Echo handler function for RDS Data requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		// Resolve the per-request region (from SigV4 / X-Amz-Region) and attach
		// it to the context so backend operations are region-scoped.
		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
		ctx = context.WithValue(ctx, regionContextKey{}, region)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "rdsdata: failed to read request body", "error", err)

			return writeInternalServerError(c)
		}

		op := h.ExtractOperation(c)

		result, dispErr := h.dispatch(ctx, op, body)
		if dispErr != nil {
			return h.handleError(c, dispErr)
		}

		if result == nil {
			return c.NoContent(http.StatusOK)
		}

		return c.JSONBlob(http.StatusOK, result)
	}
}

func (h *Handler) dispatch(ctx context.Context, op string, body []byte) ([]byte, error) {
	switch op {
	case opExecuteStatement:
		return h.handleExecuteStatement(ctx, body)
	case opBatchExecuteStatement:
		return h.handleBatchExecuteStatement(ctx, body)
	case opBeginTransaction:
		return h.handleBeginTransaction(ctx, body)
	case opCommitTransaction:
		return h.handleCommitTransaction(ctx, body)
	case opRollbackTransaction:
		return h.handleRollbackTransaction(ctx, body)
	case opExecuteSQL:
		return h.handleExecuteSQL(ctx, body)
	default:
		return nil, fmt.Errorf("%w: %s", errUnknownAction, op)
	}
}

// writeInternalServerError renders a ReadBody-failure (body too large, read
// error) as rdsdata's own restjson1 error envelope. aws-sdk-go-v2's
// restjson.GetErrorInfo (aws/protocol/restjson/decoder_util.go) JSON-decodes
// the body for __type/message, so the bare text/plain this used to send
// deserialized client-side as smithy.GenericAPIError{Code:"UnknownError"}
// (gopherstack-o7gx). InternalServerErrorException is rdsdata's own modeled
// internal error (rdsdata@v1.35.4 types/errors.go).
func writeInternalServerError(c *echo.Context) error {
	payload, err := json.Marshal(map[string]string{
		keyTypeField:    "InternalServerErrorException",
		keyMessageField: "internal server error",
	})
	if err != nil {
		return err
	}

	return c.JSONBlob(http.StatusInternalServerError, payload)
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.Is(err, ErrTransactionNotFound):
		payload, _ := json.Marshal(map[string]string{
			keyTypeField:    "TransactionNotFoundException",
			keyMessageField: err.Error(),
		})

		return c.JSONBlob(http.StatusBadRequest, payload)
	case errIsValidation(err):
		payload, _ := json.Marshal(map[string]string{
			keyTypeField:    "BadRequestException",
			keyMessageField: err.Error(),
		})

		return c.JSONBlob(http.StatusBadRequest, payload)
	case errors.Is(err, errInvalidRequest), errors.Is(err, errUnknownAction),
		errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return c.JSON(http.StatusBadRequest, map[string]string{keyMessageField: err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{keyMessageField: err.Error()})
	}
}

type executeStatementRequest struct {
	ResultSetOptions      *resultSetOptionsRequest `json:"resultSetOptions"`
	ResourceArn           string                   `json:"resourceArn"`
	SecretArn             string                   `json:"secretArn"`
	SQL                   string                   `json:"sql"`
	Database              string                   `json:"database"`
	Schema                string                   `json:"schema"`
	TransactionID         string                   `json:"transactionId"`
	FormatRecordsAs       string                   `json:"formatRecordsAs"`
	Parameters            []SQLParameter           `json:"parameters"`
	IncludeResultMetadata bool                     `json:"includeResultMetadata"`
	ContinueAfterTimeout  bool                     `json:"continueAfterTimeout"`
}

// resultSetOptionsRequest mirrors types.ResultSetOptions
// (aws-sdk-go-v2/service/rdsdata). Both fields are enums validated by
// validateResultSetOptions before being handed to the engine as
// resultSetOptions (store.go).
type resultSetOptionsRequest struct {
	DecimalReturnType string `json:"decimalReturnType"`
	LongReturnType    string `json:"longReturnType"`
}

// validateResultSetOptions reports an error for any decimalReturnType/
// longReturnType value other than the real API's enum members ("" is treated
// as the default for each). A nil opts is valid (the field is optional).
func validateResultSetOptions(opts *resultSetOptionsRequest) error {
	if opts == nil {
		return nil
	}

	switch opts.DecimalReturnType {
	case "", decimalReturnTypeString, decimalReturnTypeDoubleOrLong:
	default:
		return fmt.Errorf("%w: invalid resultSetOptions.decimalReturnType %q", ErrValidation, opts.DecimalReturnType)
	}

	switch opts.LongReturnType {
	case "", longReturnTypeString, longReturnTypeLong:
	default:
		return fmt.Errorf("%w: invalid resultSetOptions.longReturnType %q", ErrValidation, opts.LongReturnType)
	}

	return nil
}

// validateNoArrayParameters rejects any parameter whose value is an
// arrayValue: real AWS documents "Array parameters are not supported" for
// both ExecuteStatementInput.Parameters and BatchExecuteStatementInput.
// ParameterSets.
func validateNoArrayParameters(params []SQLParameter) error {
	for _, p := range params {
		if p.Value.ArrayValue != nil {
			return fmt.Errorf("%w: array parameters are not supported (parameter %q)", ErrValidation, p.Name)
		}
	}

	return nil
}

// validateFormatRecordsAs reports an error for any formatRecordsAs value
// other than the real API's two enum members ("" is treated as the default,
// NONE).
func validateFormatRecordsAs(v string) error {
	switch v {
	case "", formatRecordsAsNone, formatRecordsAsJSON:
		return nil
	default:
		return fmt.Errorf("%w: invalid formatRecordsAs %q", ErrValidation, v)
	}
}

type requiredField struct {
	name  string
	value string
}

func validateRequiredFields(fields ...requiredField) error {
	for _, field := range fields {
		if field.value == "" {
			return fmt.Errorf("%w: missing %s", errInvalidRequest, field.name)
		}
	}

	return nil
}

func (h *Handler) handleExecuteStatement(ctx context.Context, body []byte) ([]byte, error) {
	var req executeStatementRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := validateRequiredFields(
		requiredField{name: keyResourceARNField, value: req.ResourceArn},
		requiredField{name: keySecretARNField, value: req.SecretArn},
		requiredField{name: "sql", value: req.SQL},
	); err != nil {
		return nil, err
	}

	if err := validateFormatRecordsAs(req.FormatRecordsAs); err != nil {
		return nil, err
	}

	if err := validateResultSetOptions(req.ResultSetOptions); err != nil {
		return nil, err
	}

	if err := validateNoArrayParameters(req.Parameters); err != nil {
		return nil, err
	}

	if req.ResultSetOptions != nil {
		ctx = context.WithValue(ctx, resultSetOptionsContextKey{}, resultSetOptions{
			DecimalReturnType: req.ResultSetOptions.DecimalReturnType,
			LongReturnType:    req.ResultSetOptions.LongReturnType,
		})
	}

	records, columns, updated, generatedFields, err := h.Backend.ExecuteStatement(
		ctx, req.ResourceArn, req.SQL, req.TransactionID, req.Parameters...)
	if err != nil {
		return nil, err
	}

	if generatedFields == nil {
		generatedFields = []Field{}
	}

	// Use a map so columnMetadata/records/formattedRecords can be
	// conditionally included, matching real AWS response shaping.
	resp := map[string]any{
		"generatedFields":        generatedFields,
		"numberOfRecordsUpdated": updated,
	}

	// formatRecordsAs=JSON only applies to SELECT statements; real AWS
	// ignores it for other statement types, which fall through to the
	// normal records/columnMetadata shape (empty, since DML has no rows).
	if req.FormatRecordsAs == formatRecordsAsJSON && isQuery(req.SQL) {
		formatted, ferr := formatRecordsAsJSONString(records, columns)
		if ferr != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidRequest, ferr)
		}

		resp["formattedRecords"] = formatted
	} else {
		resp["records"] = records

		if req.IncludeResultMetadata {
			resp["columnMetadata"] = columns
		}
	}

	return json.Marshal(resp)
}

// formatRecordsAsJSONString renders records as the JSON string real AWS
// returns in formattedRecords when formatRecordsAs=JSON: an array of row
// objects keyed by column name, with each Field unwrapped to its native
// JSON-representable value.
func formatRecordsAsJSONString(records [][]Field, columns []ColumnMetadata) (string, error) {
	rows := make([]map[string]any, len(records))

	for i, record := range records {
		row := make(map[string]any, len(record))

		for j, field := range record {
			name := fmt.Sprintf("column%d", j+1)
			if j < len(columns) {
				name = columns[j].Name
			}

			row[name] = fieldToJSONValue(field)
		}

		rows[i] = row
	}

	out, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("marshal formatted records: %w", err)
	}

	return string(out), nil
}

// fieldToJSONValue unwraps a Field union into its native JSON value. Blobs
// are base64-encoded, matching how JSON (which has no binary type) must
// represent them.
func fieldToJSONValue(f Field) any {
	if f.BlobValue != nil {
		return base64.StdEncoding.EncodeToString(f.BlobValue)
	}

	return fieldToValue(f)
}

type batchExecuteStatementRequest struct {
	ResourceArn   string           `json:"resourceArn"`
	SecretArn     string           `json:"secretArn"`
	SQL           string           `json:"sql"`
	Database      string           `json:"database"`
	Schema        string           `json:"schema"`
	TransactionID string           `json:"transactionId"`
	ParameterSets [][]SQLParameter `json:"parameterSets"`
}

type batchExecuteStatementResponse struct {
	UpdateResults []UpdateResult `json:"updateResults"`
}

func (h *Handler) handleBatchExecuteStatement(ctx context.Context, body []byte) ([]byte, error) {
	var req batchExecuteStatementRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := validateRequiredFields(
		requiredField{name: keyResourceARNField, value: req.ResourceArn},
		requiredField{name: keySecretARNField, value: req.SecretArn},
		requiredField{name: "sql", value: req.SQL},
	); err != nil {
		return nil, err
	}

	for _, params := range req.ParameterSets {
		if err := validateNoArrayParameters(params); err != nil {
			return nil, err
		}
	}

	results, err := h.Backend.BatchExecuteStatement(ctx, req.ResourceArn, req.SQL, req.TransactionID, req.ParameterSets)
	if err != nil {
		return nil, err
	}

	return json.Marshal(batchExecuteStatementResponse{UpdateResults: results})
}

type beginTransactionRequest struct {
	ResourceArn string `json:"resourceArn"`
	SecretArn   string `json:"secretArn"`
	Database    string `json:"database"`
	Schema      string `json:"schema"`
}

type beginTransactionResponse struct {
	TransactionID string `json:"transactionId"`
}

func (h *Handler) handleBeginTransaction(ctx context.Context, body []byte) ([]byte, error) {
	var req beginTransactionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := validateRequiredFields(
		requiredField{name: keyResourceARNField, value: req.ResourceArn},
		requiredField{name: keySecretARNField, value: req.SecretArn},
	); err != nil {
		return nil, err
	}

	txID, err := h.Backend.BeginTransaction(ctx, req.ResourceArn)
	if err != nil {
		return nil, err
	}

	return json.Marshal(beginTransactionResponse{TransactionID: txID})
}

type commitTransactionRequest struct {
	ResourceArn   string `json:"resourceArn"`
	SecretArn     string `json:"secretArn"`
	TransactionID string `json:"transactionId"`
}

type commitTransactionResponse struct {
	TransactionStatus string `json:"transactionStatus"`
}

func (h *Handler) handleCommitTransaction(ctx context.Context, body []byte) ([]byte, error) {
	var req commitTransactionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := validateRequiredFields(
		requiredField{name: keyResourceARNField, value: req.ResourceArn},
		requiredField{name: keySecretARNField, value: req.SecretArn},
		requiredField{name: "transactionId", value: req.TransactionID},
	); err != nil {
		return nil, err
	}

	status, err := h.Backend.CommitTransaction(ctx, req.TransactionID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(commitTransactionResponse{TransactionStatus: status})
}

type rollbackTransactionRequest struct {
	ResourceArn   string `json:"resourceArn"`
	SecretArn     string `json:"secretArn"`
	TransactionID string `json:"transactionId"`
}

type rollbackTransactionResponse struct {
	TransactionStatus string `json:"transactionStatus"`
}

func (h *Handler) handleRollbackTransaction(ctx context.Context, body []byte) ([]byte, error) {
	var req rollbackTransactionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := validateRequiredFields(
		requiredField{name: keyResourceARNField, value: req.ResourceArn},
		requiredField{name: keySecretARNField, value: req.SecretArn},
		requiredField{name: "transactionId", value: req.TransactionID},
	); err != nil {
		return nil, err
	}

	status, err := h.Backend.RollbackTransaction(ctx, req.TransactionID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(rollbackTransactionResponse{TransactionStatus: status})
}

type executeSQLRequest struct {
	AwsSecretStoreArn      string `json:"awsSecretStoreArn"`
	DBClusterOrInstanceArn string `json:"dbClusterOrInstanceArn"`
	SQLStatements          string `json:"sqlStatements"`
	Database               string `json:"database"`
	Schema                 string `json:"schema"`
}

type executeSQLResponse struct {
	SQLStatementResults []SQLStatementResult `json:"sqlStatementResults"`
}

func (h *Handler) handleExecuteSQL(ctx context.Context, body []byte) ([]byte, error) {
	var req executeSQLRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := validateRequiredFields(
		requiredField{name: "dbClusterOrInstanceArn", value: req.DBClusterOrInstanceArn},
		requiredField{name: "awsSecretStoreArn", value: req.AwsSecretStoreArn},
		requiredField{name: "sqlStatements", value: req.SQLStatements},
	); err != nil {
		return nil, err
	}

	results, err := h.Backend.ExecuteSQL(ctx, req.DBClusterOrInstanceArn, req.SQLStatements)
	if err != nil {
		return nil, err
	}

	return json.Marshal(executeSQLResponse{SQLStatementResults: results})
}
