package forecast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const forecastTargetPrefix = "AmazonForecast."

type operationMode string

const (
	modeCreate   operationMode = "create"
	modeDescribe operationMode = "describe"
	modeList     operationMode = "list"
	modeDelete   operationMode = "delete"
	modeUpdate   operationMode = "update"

	defaultListPageSize = 100
)

type operationSpec struct {
	kind      resourceKind
	mode      operationMode
	nameField string
	arnField  string
	listField string
	// summaryFields lists the Data keys the real List op's <Kind>Summary type
	// declares (verified per-kind against aws-sdk-go-v2/service/forecast's
	// types.go); summaryStatus reports whether that Summary type declares
	// Status. Describe/Create/Update keep the full resourceOutput -- only List
	// is narrowed, since AWS scopes List responses but not those.
	summaryFields []string
	summaryStatus bool
}

// Handler serves Amazon Forecast JSON protocol operations.
type Handler struct {
	Backend *InMemoryBackend
	ops     map[string]operationSpec
}

// NewHandler creates Forecast HTTP handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{Backend: backend, ops: forecastOperations()}
}

// Name returns service registry name.
func (h *Handler) Name() string { return "Forecast" }

// Reset clears all backend state for the /_gopherstack/reset test hook.
func (h *Handler) Reset() { h.Backend.Reset() }

// ChaosServiceName returns fault injection service identifier.
func (h *Handler) ChaosServiceName() string { return "forecast" }

// ChaosOperations returns supported fault injection operations.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns regions handled by instance.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// GetSupportedOperations reports implemented Amazon Forecast operations.
func (h *Handler) GetSupportedOperations() []string {
	result := make([]string, 0, len(h.ops)+1)
	for operation := range h.ops {
		result = append(result, operation)
	}
	result = append(result, "ListMonitorEvaluations")
	result = append(result, "DeleteResourceTree")
	result = append(result, "ResumeResource")
	result = append(result, "StopResource")
	result = append(result, "GetAccuracyMetrics")
	result = append(result, "ListTagsForResource")
	result = append(result, "TagResource")
	result = append(result, "UntagResource")

	return result
}

// RouteMatcher matches Forecast target requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), forecastTargetPrefix)
	}
}

// MatchPriority returns Forecast header match priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts operation from Forecast target.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return strings.TrimPrefix(c.Request().Header.Get("X-Amz-Target"), forecastTargetPrefix)
}

// ExtractResource returns no generic resource identifier.
func (h *Handler) ExtractResource(_ *echo.Context) string { return "" }

// Handler returns Echo handler.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c,
			logger.Load(c.Request().Context()),
			h.Name(),
			"application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

func (h *Handler) dispatch(_ context.Context, action string, body []byte) ([]byte, error) {
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, err
	}

	if action == "ListMonitorEvaluations" {
		return h.dispatchListMonitorEvaluations(input)
	}
	if action == "DeleteResourceTree" {
		return h.dispatchDeleteResourceTree(input)
	}
	if action == "ResumeResource" {
		return h.dispatchResumeResource(input)
	}
	if action == "StopResource" {
		return h.dispatchStopResource(input)
	}
	if action == "GetAccuracyMetrics" {
		return h.dispatchGetAccuracyMetrics(input)
	}
	if action == "ListTagsForResource" {
		return h.dispatchListTagsForResource(input)
	}
	if action == "TagResource" {
		return h.dispatchTagResource(input)
	}
	if action == "UntagResource" {
		return h.dispatchUntagResource(input)
	}

	spec, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrValidation, action)
	}

	output, err := h.execute(action, spec, input)
	if err != nil {
		return nil, err
	}

	return json.Marshal(output)
}

func (h *Handler) execute(action string, spec operationSpec, input map[string]any) (map[string]any, error) {
	switch spec.mode {
	case modeCreate:
		resource, err := h.Backend.create(
			spec.kind,
			action,
			stringValue(input[spec.nameField]),
			input,
			createFailureMessage(spec.kind, input),
		)
		if err != nil {
			return nil, err
		}

		return map[string]any{spec.arnField: resource.ARN}, nil
	case modeDescribe:
		resource, err := h.Backend.describe(spec.kind, resourceIdentifier(spec, input))
		if err != nil {
			return nil, err
		}

		output := resourceOutput(spec, resource)
		if spec.kind == kindMonitor {
			if eval, ok := h.Backend.latestMonitorEvaluation(resource.ARN); ok {
				output["LastEvaluationState"] = eval.EvaluationState
				output["LastEvaluationTime"] = awstime.Epoch(eval.EvaluationTime)
			}
		}

		return output, nil
	case modeUpdate:
		resource, err := h.Backend.update(spec.kind, resourceIdentifier(spec, input), input)
		if err != nil {
			return nil, err
		}

		return map[string]any{spec.arnField: resource.ARN}, nil
	case modeDelete:
		if err := h.Backend.delete(spec.kind, resourceIdentifier(spec, input)); err != nil {
			return nil, err
		}

		return map[string]any{}, nil
	case modeList:

		return listOutput(spec, h.Backend.list(spec.kind), input)
	default:

		return nil, fmt.Errorf("%w: unsupported operation mode", ErrValidation)
	}
}

func (h *Handler) dispatchListMonitorEvaluations(input map[string]any) ([]byte, error) {
	evaluations, err := h.Backend.listMonitorEvaluations(stringValue(input["MonitorArn"]))
	if err != nil {
		return nil, err
	}

	maxResults := 0
	if mr, ok := input["MaxResults"].(float64); ok {
		maxResults = int(mr)
	}
	nextToken, _ := input["NextToken"].(string)

	if tokenErr := page.ValidateToken(nextToken); tokenErr != nil {
		return nil, fmt.Errorf("%w: NextToken %q is not valid", ErrInvalidNextToken, nextToken)
	}

	evaluations = filterMonitorEvaluations(evaluations, filtersFromInput(input))

	entries := make([]map[string]any, 0, len(evaluations))
	for _, e := range evaluations {
		entries = append(entries, monitorEvaluationOutput(e))
	}

	pg := page.New(entries, nextToken, maxResults, defaultListPageSize)
	out := map[string]any{"PredictorMonitorEvaluations": pg.Data}
	if pg.Next != "" {
		out["NextToken"] = pg.Next
	}

	return json.Marshal(out)
}

// filterMonitorEvaluations applies ListMonitorEvaluations's Filters
// parameter, whose only real Key is "EvaluationState" (api_op_
// ListMonitorEvaluations.go's doc comment) -- MonitorEvaluation.
// EvaluationState is a plain string field, so this reuses the same
// IS/IS_NOT matching listOutput's applyFilters uses for List<Kind>
// operations.
func filterMonitorEvaluations(evaluations []MonitorEvaluation, filters []resourceFilter) []MonitorEvaluation {
	if len(filters) == 0 {
		return evaluations
	}

	result := make([]MonitorEvaluation, 0, len(evaluations))
	for _, e := range evaluations {
		if monitorEvaluationMatchesFilters(e, filters) {
			result = append(result, e)
		}
	}

	return result
}

func monitorEvaluationMatchesFilters(e MonitorEvaluation, filters []resourceFilter) bool {
	for _, f := range filters {
		if f.key != "EvaluationState" {
			continue
		}

		matches := e.EvaluationState == f.value
		if f.condition == "IS_NOT" {
			matches = !matches
		}
		if !matches {
			return false
		}
	}

	return true
}

// monitorEvaluationOutput converts a MonitorEvaluation to its wire shape.
// CreationTime/EvaluationTime must be epoch-seconds JSON numbers (JSON-RPC
// 1.1 timestamp format, pkgs/awstime.Epoch) -- MonitorEvaluation's own
// `json:"CreationTime"` struct tag marshals time.Time as an RFC3339 string
// instead, which the real SDK's ListMonitorEvaluations deserializer
// rejects.
func monitorEvaluationOutput(e MonitorEvaluation) map[string]any {
	out := map[string]any{
		"CreationTime":    awstime.Epoch(e.CreationTime),
		"EvaluationTime":  awstime.Epoch(e.EvaluationTime),
		"MonitorArn":      e.MonitorArn,
		"MonitorName":     e.MonitorName,
		"Status":          e.Status,
		"EvaluationState": e.EvaluationState,
		"MetricResults":   e.MetricResults,
	}
	if e.ResourceArn != "" {
		out["ResourceArn"] = e.ResourceArn
	}
	if e.Message != "" {
		out["Message"] = e.Message
	}
	if e.PredictorEvent != nil {
		out["PredictorEvent"] = e.PredictorEvent
	}

	return out
}

func (h *Handler) dispatchDeleteResourceTree(input map[string]any) ([]byte, error) {
	err := h.Backend.DeleteResourceTree(stringValue(input[fieldResourceArn]))
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{})
}

func (h *Handler) dispatchResumeResource(input map[string]any) ([]byte, error) {
	err := h.Backend.UpdateResourceStatus(stringValue(input[fieldResourceArn]), statusActive)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{})
}

func (h *Handler) dispatchStopResource(input map[string]any) ([]byte, error) {
	err := h.Backend.UpdateResourceStatus(stringValue(input[fieldResourceArn]), statusStopped)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{})
}

func (h *Handler) dispatchGetAccuracyMetrics(input map[string]any) ([]byte, error) {
	metrics, err := h.Backend.GetAccuracyMetrics(stringValue(input[fieldPredictorArn]))
	if err != nil {
		return nil, err
	}

	return json.Marshal(metrics)
}

func (h *Handler) dispatchListTagsForResource(input map[string]any) ([]byte, error) {
	tags, err := h.Backend.ListTagsForResource(stringValue(input[fieldResourceArn]))
	if err != nil {
		return nil, err
	}
	var tagList []map[string]string
	for k, v := range tags {
		tagList = append(tagList, map[string]string{"Key": k, "Value": v})
	}

	return json.Marshal(map[string]any{"Tags": tagList})
}

func (h *Handler) dispatchTagResource(input map[string]any) ([]byte, error) {
	err := h.Backend.TagResource(stringValue(input[fieldResourceArn]), tagsFromInput(input))
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{})
}

func (h *Handler) dispatchUntagResource(input map[string]any) ([]byte, error) {
	var tagKeys []string
	if keys, ok := input["TagKeys"].([]any); ok {
		for _, k := range keys {
			tagKeys = append(tagKeys, stringValue(k))
		}
	}
	err := h.Backend.UntagResource(stringValue(input[fieldResourceArn]), tagKeys)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{})
}

func resourceIdentifier(spec operationSpec, input map[string]any) string {
	if value := stringValue(input[spec.arnField]); value != "" {
		return value
	}

	return stringValue(input[spec.nameField])
}

func resourceOutput(spec operationSpec, resource *Resource) map[string]any {
	output := cloneMap(resource.Data)
	output[spec.nameField] = resource.Name
	output[spec.arnField] = resource.ARN
	output["Status"] = resource.Status
	output["CreationTime"] = awstime.Epoch(resource.CreatedAt)
	output["LastModificationTime"] = awstime.Epoch(resource.UpdatedAt)
	if resource.Message != "" {
		output["Message"] = resource.Message
	}

	return output
}

// summaryOutput builds a List-scoped resource representation restricted to
// spec.summaryFields, the real SDK <Kind>Summary type's declared members --
// unlike resourceOutput, which passes the full create-request Data through
// unscoped and is only correct for Describe/Create/Update, where AWS returns
// that full shape.
func summaryOutput(spec operationSpec, resource *Resource) map[string]any {
	output := make(map[string]any, len(spec.summaryFields))
	for _, key := range spec.summaryFields {
		if value, ok := resource.Data[key]; ok {
			output[key] = cloneValue(value)
		}
	}
	output[spec.nameField] = resource.Name
	output[spec.arnField] = resource.ARN
	output["CreationTime"] = awstime.Epoch(resource.CreatedAt)
	output["LastModificationTime"] = awstime.Epoch(resource.UpdatedAt)
	if spec.summaryStatus {
		output["Status"] = resource.Status
	}

	return output
}

func listOutput(spec operationSpec, resources []*Resource, input map[string]any) (map[string]any, error) {
	maxResults := 0
	if mr, ok := input["MaxResults"].(float64); ok {
		maxResults = int(mr)
	}
	nextToken, _ := input["NextToken"].(string)

	if err := page.ValidateToken(nextToken); err != nil {
		return nil, fmt.Errorf("%w: NextToken %q is not valid", ErrInvalidNextToken, nextToken)
	}

	resources = applyFilters(spec, resources, filtersFromInput(input))

	summaries := make([]map[string]any, 0, len(resources))
	for _, r := range resources {
		summaries = append(summaries, summaryOutput(spec, r))
	}

	pg := page.New(summaries, nextToken, maxResults, defaultListPageSize)
	out := map[string]any{spec.listField: pg.Data}
	if pg.Next != "" {
		out["NextToken"] = pg.Next
	}

	return out, nil
}

// resourceFilter is one entry of a List operation's Filters array (every
// Filter-bearing Forecast List op uses the identical types.Filter shape:
// Condition IS/IS_NOT, Key, Value).
type resourceFilter struct {
	condition string
	key       string
	value     string
}

func filtersFromInput(input map[string]any) []resourceFilter {
	raw, ok := input["Filters"].([]any)
	if !ok {
		return nil
	}

	filters := make([]resourceFilter, 0, len(raw))
	for _, f := range raw {
		m, mapOK := f.(map[string]any)
		if !mapOK {
			continue
		}
		filters = append(filters, resourceFilter{
			condition: stringValue(m["Condition"]),
			key:       stringValue(m["Key"]),
			value:     stringValue(m["Value"]),
		})
	}

	return filters
}

// applyFilters keeps only the resources matching every filter that can be
// resolved to an actual value on the resource: "Status" reads
// resource.Status, a Key matching the operation's own ARN field reads
// resource.ARN, and any other Key is looked up directly in resource.Data
// under that name -- covering every Filter Key the real Forecast List ops
// declare except two structural gaps left unfiltered (not silently
// mismatched): ListForecasts/ListPredictors's "DatasetGroupArn" (the
// predictor's DatasetGroupArn lives nested under InputDataConfig/DataConfig
// and is never recorded top-level -- see addCRUD's Predictor registration
// comment) and ListExplainabilityExports's "ResourceArn" (the create
// request's own field is ExplainabilityArn, not ResourceArn -- no data
// exists under the literal filter Key name). A filter whose Key cannot be
// resolved is left unapplied rather than treated as never matching, so an
// unfixable filter degrades to "not yet honoured" instead of "always
// empty".
func applyFilters(spec operationSpec, resources []*Resource, filters []resourceFilter) []*Resource {
	if len(filters) == 0 {
		return resources
	}

	result := make([]*Resource, 0, len(resources))
	for _, r := range resources {
		if resourceMatchesFilters(spec, r, filters) {
			result = append(result, r)
		}
	}

	return result
}

func resourceMatchesFilters(spec operationSpec, r *Resource, filters []resourceFilter) bool {
	for _, f := range filters {
		value, resolvable := filterFieldValue(spec, r, f.key)
		if !resolvable {
			continue
		}

		matches := value == f.value
		if f.condition == "IS_NOT" {
			matches = !matches
		}
		if !matches {
			return false
		}
	}

	return true
}

func filterFieldValue(spec operationSpec, r *Resource, key string) (string, bool) {
	switch key {
	case "Status":
		return r.Status, true
	case spec.arnField:
		return r.ARN, true
	}

	if v, ok := r.Data[key]; ok {
		return stringValue(v), true
	}

	return "", false
}

// createFailureMessage reports the DescribeDatasetImportJobOutput.Message
// ("If an error occurred, an informational message about the error",
// forecast@v1.44.4 api_op_DescribeDatasetImportJob.go) this backend's one
// modeled failure condition should carry, or "" if the create should
// succeed. Only kindDatasetImportJob can fail here.
func createFailureMessage(kind resourceKind, input map[string]any) string {
	if kind != kindDatasetImportJob {
		return ""
	}

	dataSource, ok := input["DataSource"].(map[string]any)
	if !ok {
		return "DataSource is required"
	}
	s3Config, ok := dataSource["S3Config"].(map[string]any)
	if !ok {
		return "DataSource.S3Config is required"
	}
	if stringValue(s3Config["Path"]) == "" {
		return "DataSource.S3Config.Path is required"
	}

	return ""
}

const forecastContentType = "application/x-amz-json-1.1"

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	var code int
	var errType string

	switch {
	case errors.Is(err, ErrNotFound):
		code, errType = http.StatusBadRequest, "ResourceNotFoundException"
	case errors.Is(err, ErrAlreadyExists):
		code, errType = http.StatusBadRequest, "ResourceAlreadyExistsException"
	case errors.Is(err, ErrResourceInUse):
		code, errType = http.StatusBadRequest, "ResourceInUseException"
	case errors.Is(err, ErrInvalidNextToken):
		code, errType = http.StatusBadRequest, "InvalidNextTokenException"
	case errors.Is(err, ErrValidation):
		code, errType = http.StatusBadRequest, "InvalidInputException"
	case errors.Is(err, ErrTagLimitExceeded):
		code, errType = http.StatusBadRequest, "LimitExceededException"
	default:
		code, errType = http.StatusInternalServerError, "InternalServerException"
	}

	payload, marshalErr := json.Marshal(map[string]string{"__type": errType, "message": err.Error()})
	if marshalErr != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	c.Response().Header().Set("Content-Type", forecastContentType)

	return c.JSONBlob(code, payload)
}

// summaryFields/summaryStatus arguments throughout registerDataOperations and
// registerForecastingOperations are each verified against that kind's real
// List<Kind>sOutput.<Kind>Summary declaration in
// aws-sdk-go-v2/service/forecast/types/types.go -- not derived from the
// Describe shape or from a sibling by analogy. Every field the real Summary
// type omits (e.g. Predictor's InputDataConfig/TrainingParameters/
// AlgorithmArn, Dataset's Schema/EncryptionConfig) is left out, so
// listOutput's summaryOutput no longer echoes the full create-request body.
func forecastOperations() map[string]operationSpec {
	operations := make(map[string]operationSpec)
	registerDataOperations(operations)
	registerForecastingOperations(operations)
	operations["CreateAutoPredictor"] = operationSpec{
		kind: kindPredictor, mode: modeCreate, nameField: "PredictorName",
		arnField: fieldPredictorArn, listField: "Predictors",
	}
	operations["DescribeAutoPredictor"] = operationSpec{
		kind: kindPredictor, mode: modeDescribe, nameField: "PredictorName",
		arnField: fieldPredictorArn, listField: "Predictors",
	}

	return operations
}

func registerDataOperations(operations map[string]operationSpec) {
	addCRUD(
		operations,
		"DatasetGroup",
		kindDatasetGroup,
		"DatasetGroupName",
		"DatasetGroupArn",
		"DatasetGroups",
		true,
		nil, // DatasetGroupSummary: no extra fields, no Status
		false,
	)
	// update=false: real Forecast has no UpdateDataset operation (verified against
	// aws-sdk-go-v2/service/forecast.Client: only UpdateDatasetGroup exists among
	// dataset-family Update* methods; datasets are immutable after creation, only
	// re-imported via CreateDatasetImportJob). A prior pass wired this addCRUD call
	// with update=true, which both advertised and dispatched a fabricated
	// "UpdateDataset" operation no real client can send — caught by pkgs/sdkcheck's
	// reverse check (commit 12cfe14d5; gopherstack-vhw2 category A). Unlike some
	// other findings in this campaign, nothing legitimate depended on the route (no
	// test exercised it, PARITY.md's Dataset family note already only claimed
	// Create/Describe/Delete/List), so it is deleted outright rather than kept
	// wired-but-unadvertised.
	addCRUD(
		operations, "Dataset", kindDataset, "DatasetName", "DatasetArn", "Datasets", false,
		[]string{"DatasetType", "Domain"}, false, // DatasetSummary: no Status
	)
	addCRUD(
		operations,
		"DatasetImportJob",
		kindDatasetImportJob,
		"DatasetImportJobName",
		"DatasetImportJobArn",
		"DatasetImportJobs",
		false,
		[]string{"DataSource", "ImportMode"},
		true,
	)
	addCRUD(
		operations, "Predictor", kindPredictor, "PredictorName", fieldPredictorArn, "Predictors", false,
		// PredictorSummary also declares DatasetGroupArn, IsAutoPredictor and
		// ReferencePredictorSummary, but none has a backend field to source it
		// from: CreatePredictor's DatasetGroupArn lives nested under
		// InputDataConfig (not top-level), CreateAutoPredictor's under
		// DataConfig, and IsAutoPredictor/ReferencePredictorSummary are never
		// recorded at all. Left absent rather than fabricated; a separate,
		// pre-existing missing-field gap, not this issue's over-wide class.
		nil, true,
	)
}

func registerForecastingOperations(operations map[string]operationSpec) {
	addCRUD(
		operations,
		"PredictorBacktestExportJob",
		kindPredictorBacktestExport,
		"PredictorBacktestExportJobName",
		"PredictorBacktestExportJobArn",
		"PredictorBacktestExportJobs",
		false,
		[]string{fieldDestination},
		true,
	)
	addCRUD(
		operations, "Forecast", kindForecast, "ForecastName", fieldForecastArn, "Forecasts", false,
		[]string{"PredictorArn"}, true,
	)
	addCRUD(
		operations,
		"ForecastExportJob",
		kindForecastExport,
		"ForecastExportJobName",
		"ForecastExportJobArn",
		"ForecastExportJobs",
		false,
		[]string{fieldDestination},
		true,
	)
	addCRUD(
		operations,
		"ExplainabilityExport",
		kindExplainabilityExport,
		"ExplainabilityExportName",
		"ExplainabilityExportArn",
		"ExplainabilityExports",
		false,
		[]string{fieldDestination},
		true,
	)
	addCRUD(
		operations,
		"WhatIfAnalysis",
		kindWhatIfAnalysis,
		"WhatIfAnalysisName",
		"WhatIfAnalysisArn",
		"WhatIfAnalyses",
		false,
		[]string{fieldForecastArn},
		true,
	)
	addCRUD(
		operations,
		"WhatIfForecast",
		kindWhatIfForecast,
		"WhatIfForecastName",
		"WhatIfForecastArn",
		"WhatIfForecasts",
		false,
		[]string{"WhatIfAnalysisArn"},
		true,
	)
	addCRUD(
		operations,
		"WhatIfForecastExport",
		kindWhatIfForecastExport,
		"WhatIfForecastExportName",
		"WhatIfForecastExportArn",
		"WhatIfForecastExports",
		false,
		[]string{fieldDestination, "WhatIfForecastArns"},
		true,
	)
	addCRUD(
		operations, "Monitor", kindMonitor, "MonitorName", "MonitorArn", "Monitors", false,
		[]string{fieldResourceArn}, true,
	)
	addCRUD(
		operations,
		"Explainability",
		kindExplainability,
		"ExplainabilityName",
		"ExplainabilityArn",
		"Explainabilities",
		false,
		[]string{fieldResourceArn, "ExplainabilityConfig"},
		true,
	)
}

func addCRUD(
	operations map[string]operationSpec,
	base string,
	kind resourceKind,
	nameField string,
	arnField string,
	listField string,
	update bool,
	summaryFields []string,
	summaryStatus bool,
) {
	spec := operationSpec{
		kind: kind, nameField: nameField, arnField: arnField, listField: listField,
		summaryFields: summaryFields, summaryStatus: summaryStatus,
	}
	operations["Create"+base] = withMode(spec, modeCreate)
	operations["Describe"+base] = withMode(spec, modeDescribe)
	operations["List"+plural(base)] = withMode(spec, modeList)
	operations["Delete"+base] = withMode(spec, modeDelete)
	if update {
		operations["Update"+base] = withMode(spec, modeUpdate)
	}
}

func withMode(spec operationSpec, mode operationMode) operationSpec {
	spec.mode = mode

	return spec
}

func plural(base string) string {
	switch base {
	case "WhatIfAnalysis":

		return "WhatIfAnalyses"
	case "Explainability":

		return "Explainabilities"
	default:

		return base + "s"
	}
}
