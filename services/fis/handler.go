package fis

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	opCreateExperimentTemplate                  = "CreateExperimentTemplate"
	opCreateTargetAccountConfiguration          = "CreateTargetAccountConfiguration"
	opDeleteExperimentTemplate                  = "DeleteExperimentTemplate"
	opDeleteTargetAccountConfiguration          = "DeleteTargetAccountConfiguration"
	opGetAction                                 = "GetAction"
	opGetExperiment                             = "GetExperiment"
	opGetExperimentTargetAccountConfiguration   = "GetExperimentTargetAccountConfiguration"
	opGetExperimentTemplate                     = "GetExperimentTemplate"
	opGetSafetyLever                            = "GetSafetyLever"
	opGetTargetAccountConfiguration             = "GetTargetAccountConfiguration"
	opGetTargetResourceType                     = "GetTargetResourceType"
	opListActions                               = "ListActions"
	opListExperimentResolvedTargets             = "ListExperimentResolvedTargets"
	opListExperimentTargetAccountConfigurations = "ListExperimentTargetAccountConfigurations"
	opListExperimentTemplates                   = "ListExperimentTemplates"
	opListExperiments                           = "ListExperiments"
	opListTagsForResource                       = "ListTagsForResource"
	opListTargetAccountConfigurations           = "ListTargetAccountConfigurations"
	opListTargetResourceTypes                   = "ListTargetResourceTypes"
	opStartExperiment                           = "StartExperiment"
	opStopExperiment                            = "StopExperiment"
	opTagResource                               = "TagResource"
	opUntagResource                             = "UntagResource"
	opUpdateExperimentTemplate                  = "UpdateExperimentTemplate"
	opUpdateSafetyLeverState                    = "UpdateSafetyLeverState"
	opUpdateTargetAccountConfiguration          = "UpdateTargetAccountConfiguration"
)

const (
	// minSegmentsForID is the minimum number of path segments required for a resource ID.
	minSegmentsForID = 2
	// maxPathSegments limits how many segments pathSegments returns.
	// Five segments is enough to handle the deepest FIS paths:
	// /experimentTemplates/{id}/targetAccountConfigurations/{accountId}.
	maxPathSegments = 5
)

const (
	// pathExperimentTemplates is the root path for experiment templates.
	pathExperimentTemplates = "experimentTemplates"
	// pathExperiments is the root path for experiments.
	pathExperiments = "experiments"
	// pathActions is the root path for FIS actions.
	pathActions = "actions"
	// pathTargetResourceTypes is the root path for target resource types.
	pathTargetResourceTypes = "targetResourceTypes"
	// pathTags is the root path for resource tags.
	pathTags = "tags"
	// pathSafetyLevers is the root path for safety levers.
	pathSafetyLevers = "safetyLevers"
	// pathTargetAccountConfigurations is the sub-path for target account configurations.
	pathTargetAccountConfigurations = "targetAccountConfigurations"
	// subPathResolvedTargets is the sub-path segment for resolved targets.
	subPathResolvedTargets = "resolvedTargets"
	// subPathStop is the sub-path segment for stopping an experiment (AWS canonical).
	subPathStop = "stop"
)

// Handler is the Echo HTTP handler for the FIS REST API.
type Handler struct {
	Backend       StorageBackend
	janitor       *Janitor
	DefaultRegion string
	AccountID     string
}

// NewHandler creates a new FIS handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// WithJanitor attaches a background janitor to the handler.
// If the backend is not an *InMemoryBackend, this is a no-op.
// The optional taskTimeout bounds each sweep; 0 means no per-task timeout.
func (h *Handler) WithJanitor(
	interval, experimentTTL time.Duration,
	taskTimeout ...time.Duration,
) *Handler {
	if mem, ok := h.Backend.(*InMemoryBackend); ok {
		j := NewJanitor(mem, interval, experimentTTL)
		if len(taskTimeout) > 0 {
			j.TaskTimeout = taskTimeout[0]
		}

		h.janitor = j
	}

	return h
}

// StartWorker starts the background janitor if configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Shutdown cancels all running experiment goroutines to prevent resource leaks.
// It satisfies service.Shutdowner.
func (h *Handler) Shutdown(_ context.Context) {
	type stopper interface{ StopAllExperiments() }

	if s, ok := h.Backend.(stopper); ok {
		s.StopAllExperiments()
	}
}

// SetFaultStore injects the chaos FaultStore into the backend for inject-api-* actions.
func (h *Handler) SetFaultStore(store *chaos.FaultStore) {
	h.Backend.SetFaultStore(store)
}

// SetActionProviders registers external FIS action providers with the backend.
func (h *Handler) SetActionProviders(providers []service.FISActionProvider) {
	h.Backend.SetActionProviders(providers)
}

// SetAlarmStateSubscriber registers the CloudWatch alarm-state-change hook with the backend.
func (h *Handler) SetAlarmStateSubscriber(sub AlarmStateSubscriber) {
	h.Backend.SetAlarmStateSubscriber(sub)
}

// Name returns the service name.
func (h *Handler) Name() string { return "FIS" }

// GetSupportedOperations returns the list of supported FIS operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateExperimentTemplate,
		opGetExperimentTemplate,
		opUpdateExperimentTemplate,
		opDeleteExperimentTemplate,
		opListExperimentTemplates,
		opStartExperiment,
		opGetExperiment,
		opStopExperiment,
		opListExperiments,
		opListExperimentResolvedTargets,
		opGetAction,
		opListActions,
		opGetTargetResourceType,
		opListTargetResourceTypes,
		opGetSafetyLever,
		opUpdateSafetyLeverState,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
		opCreateTargetAccountConfiguration,
		opDeleteTargetAccountConfiguration,
		opGetExperimentTargetAccountConfiguration,
		opGetTargetAccountConfiguration,
		opListExperimentTargetAccountConfigurations,
		opListTargetAccountConfigurations,
		opUpdateTargetAccountConfiguration,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "fis" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this FIS instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.DefaultRegion} }

// RouteMatcher returns a function that matches FIS REST API requests by path prefix.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path
		for _, prefix := range []string{
			"/" + pathExperimentTemplates,
			"/" + pathExperiments,
			"/" + pathActions,
			"/" + pathTargetResourceTypes,
			"/" + pathSafetyLevers,
		} {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}

		// Match /tags/{arn} — only for FIS-owned resources (arn:aws:fis:...).
		segs := pathSegments(path)
		if len(segs) >= minSegmentsForID && segs[0] == pathTags {
			return strings.Contains(segs[1], ":fis:")
		}

		return false
	}
}

// MatchPriority returns the routing priority for the FIS handler.
// FIS uses path-based routing and is inserted between PriorityPathVersioned (85) and PriorityFormRDS (84).
func (h *Handler) MatchPriority() int { return service.PriorityPathVersioned }

// ExtractOperation extracts the FIS operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _ := parseFISPath(c.Request().Method, c.Request().URL.Path)

	return op
}

// ExtractResource extracts the resource ID from the URL path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	segs := pathSegments(c.Request().URL.Path)
	if len(segs) >= minSegmentsForID {
		return segs[1]
	}

	return ""
}

// Handler returns the Echo handler function for FIS requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		op, id := parseFISPath(c.Request().Method, c.Request().URL.Path)
		if op == "" {
			return h.writeError(c, http.StatusNotFound, "not found", "")
		}

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "fis: failed to read request body", "error", err)

			return h.writeError(
				c,
				http.StatusInternalServerError,
				"failed to read request body",
				"",
			)
		}

		log.DebugContext(ctx, "fis request", "op", op, "id", id)

		return h.dispatch(ctx, c, op, id, body)
	}
}

// dispatch routes a parsed FIS operation to the appropriate backend call.
func (h *Handler) dispatch(ctx context.Context, c *echo.Context, op, id string, body []byte) error {
	if handled, err := h.dispatchTargetAccountOps(c, op, id, body); handled {
		return err
	}

	if handled, err := h.dispatchExperimentOps(ctx, c, op, id, body); handled {
		return err
	}

	if handled, err := h.dispatchActionTagOps(c, op, id, body); handled {
		return err
	}

	return h.writeError(c, http.StatusNotFound, "unknown operation: "+op, "")
}

// dispatchExperimentOps handles experiment template and experiment operations.
func (h *Handler) dispatchExperimentOps(
	ctx context.Context,
	c *echo.Context,
	op, id string,
	body []byte,
) (bool, error) {
	switch op {
	case opCreateExperimentTemplate:
		return true, h.handleCreateExperimentTemplate(ctx, c, body)
	case opGetExperimentTemplate:
		return true, h.handleGetExperimentTemplate(c, id)
	case opUpdateExperimentTemplate:
		return true, h.handleUpdateExperimentTemplate(c, id, body)
	case opDeleteExperimentTemplate:
		return true, h.handleDeleteExperimentTemplate(c, id)
	case opListExperimentTemplates:
		return true, h.handleListExperimentTemplates(c)
	case opStartExperiment:
		return true, h.handleStartExperiment(ctx, c, body)
	case opGetExperiment:
		return true, h.handleGetExperiment(c, id)
	case opStopExperiment:
		return true, h.handleStopExperiment(c, id)
	case opListExperiments:
		return true, h.handleListExperiments(c)
	case opListExperimentResolvedTargets:
		return true, h.handleListExperimentResolvedTargets(c, id)
	}

	return false, nil
}

// dispatchActionTagOps handles action, target resource type, tag, and safety lever operations.
func (h *Handler) dispatchActionTagOps(c *echo.Context, op, id string, body []byte) (bool, error) {
	switch op {
	case opGetAction:
		return true, h.handleGetAction(c, id)
	case opListActions:
		return true, h.handleListActions(c)
	case opGetTargetResourceType:
		rt, _ := url.PathUnescape(id)

		return true, h.handleGetTargetResourceType(c, rt)
	case opListTargetResourceTypes:
		return true, h.handleListTargetResourceTypes(c)
	case opTagResource:
		return true, h.handleTagResource(c, id, body)
	case opUntagResource:
		return true, h.handleUntagResource(c, id, c.Request().URL.Query())
	case opListTagsForResource:
		return true, h.handleListTagsForResource(c, id)
	case opGetSafetyLever:
		return true, h.handleGetSafetyLever(c, id)
	case opUpdateSafetyLeverState:
		return true, h.handleUpdateSafetyLeverState(c, id, body)
	}

	return false, nil
}

// dispatchTargetAccountOps handles the target account configuration operations.
// Returns (true, err) when the operation was handled, (false, nil) otherwise.
func (h *Handler) dispatchTargetAccountOps(
	c *echo.Context,
	op, id string,
	body []byte,
) (bool, error) {
	switch op {
	case opCreateTargetAccountConfiguration:
		return true, h.handleCreateTargetAccountConfiguration(c, id, body)
	case opDeleteTargetAccountConfiguration:
		return true, h.handleDeleteTargetAccountConfiguration(c, id)
	case opGetTargetAccountConfiguration:
		return true, h.handleGetTargetAccountConfiguration(c, id)
	case opUpdateTargetAccountConfiguration:
		return true, h.handleUpdateTargetAccountConfiguration(c, id, body)
	case opListTargetAccountConfigurations:
		return true, h.handleListTargetAccountConfigurations(c, id)
	case opGetExperimentTargetAccountConfiguration:
		return true, h.handleGetExperimentTargetAccountConfiguration(c, id)
	case opListExperimentTargetAccountConfigurations:
		return true, h.handleListExperimentTargetAccountConfigurations(c, id)
	}

	return false, nil
}

// ----------------------------------------
// Error helpers
// ----------------------------------------

func (h *Handler) writeError(c *echo.Context, status int, message, resourceID string) error {
	return h.writeTypedError(c, status, "", message, resourceID)
}

func (h *Handler) writeTypedError(
	c *echo.Context,
	status int,
	errType, message, resourceID string,
) error {
	resp := errorResponseDTO{Type: errType, Message: message, ResourceID: resourceID}

	return c.JSON(status, resp)
}

// errorClass holds the AWS exception type name and HTTP status code that a sentinel
// backend error maps to.
type errorClass struct {
	exceptionType string
	httpStatus    int
}

// classifyError maps a backend sentinel error to its AWS exception type and HTTP
// status. The real FIS API model defines exactly four exception shapes service-wide
// (ValidationException, ResourceNotFoundException, ConflictException,
// ServiceQuotaExceededException) — there are no per-resource "XyzNotFoundException"
// or "TooManyTagsException" shapes. Most operations' generated deserializers in
// aws-sdk-go-v2/service/fis recognize a subset of these four __type strings; three
// (TagResource, UntagResource, ListTagsForResource) recognize none of them — their
// deserializers have an empty switch, so no type string reaches a typed error client
// side for those three regardless of what is sent. ResourceNotFoundException/404 is
// still emitted for a missing ARN on those three as the closest faithful guess, since
// no typed shape exists to prefer instead.
//
// Per the SDK's per-operation deserializers: every not-found condition (template,
// experiment, action, target resource type, safety lever, target account
// configuration, or a generically tagged resource) maps to ResourceNotFoundException
// where the operation declares it; StopExperiment does not model ConflictException,
// so stopping a non-running experiment maps to ValidationException; and
// ServiceQuotaExceededException carries HTTP 402 (Payment Required), not 429.
func classifyError(err error) errorClass {
	switch {
	case errors.Is(err, ErrValidation), errors.Is(err, ErrTooManyTags), errors.Is(err, ErrExperimentNotRunning):
		return errorClass{exceptionType: "ValidationException", httpStatus: http.StatusBadRequest}
	case errors.Is(err, ErrTooManyExperiments):
		return errorClass{exceptionType: "ServiceQuotaExceededException", httpStatus: http.StatusPaymentRequired}
	case errors.Is(err, ErrSafetyLeverEngaged):
		return errorClass{exceptionType: "ConflictException", httpStatus: http.StatusConflict}
	case errors.Is(err, ErrTemplateNotFound),
		errors.Is(err, ErrExperimentNotFound),
		errors.Is(err, ErrActionNotFound),
		errors.Is(err, ErrTargetResourceTypeNotFound),
		errors.Is(err, ErrResourceNotFound),
		errors.Is(err, ErrSafetyLeverNotFound),
		errors.Is(err, ErrTargetAccountConfigNotFound):
		return errorClass{exceptionType: "ResourceNotFoundException", httpStatus: http.StatusNotFound}
	default:
		return errorClass{exceptionType: "InternalServerError", httpStatus: http.StatusInternalServerError}
	}
}

func (h *Handler) writeBackendError(c *echo.Context, err error, id string) error {
	ec := classifyError(err)

	return h.writeTypedError(c, ec.httpStatus, ec.exceptionType, err.Error(), id)
}

// ----------------------------------------
// Path parsing
// ----------------------------------------

// parseFISPath maps an HTTP method + URL path to a FIS operation name and optional resource ID.
// Returns ("", "") when no pattern matches.
func parseFISPath(method, path string) (string, string) {
	segs := pathSegments(path)
	if len(segs) == 0 {
		return "", ""
	}

	root := segs[0]
	hasID := len(segs) >= minSegmentsForID

	switch root {
	case pathExperimentTemplates:
		return parseFISExperimentTemplatePath(method, segs, hasID)
	case pathExperiments:
		return parseFISExperimentPath(method, segs, hasID)
	case pathActions:
		return parseFISActionPath(method, segs, hasID)
	case pathTargetResourceTypes:
		return parseFISTargetResourceTypePath(method, segs, hasID)
	case pathTags:
		return parseFISTagPath(method, segs, hasID)
	case pathSafetyLevers:
		return parseFISSafetyLeverPath(method, segs, hasID)
	}

	return "", ""
}

// parseFISExperimentTemplatePath routes experiment template paths.
func parseFISExperimentTemplatePath(method string, segs []string, hasID bool) (string, string) {
	if !hasID {
		if method == http.MethodPost {
			return opCreateExperimentTemplate, ""
		}
		if method == http.MethodGet {
			return opListExperimentTemplates, ""
		}

		return "", ""
	}

	if op, id := parseFISTemplateSubPath(method, segs); op != "" {
		return op, id
	}

	switch method {
	case http.MethodGet:
		return opGetExperimentTemplate, segs[1]
	case http.MethodPatch:
		return opUpdateExperimentTemplate, segs[1]
	case http.MethodDelete:
		return opDeleteExperimentTemplate, segs[1]
	}

	return "", ""
}

// parseFISTemplateSubPath routes sub-paths of experiment templates (target account configurations).
func parseFISTemplateSubPath(method string, segs []string) (string, string) {
	if len(segs) >= 4 && segs[2] == pathTargetAccountConfigurations {
		compositeID := segs[1] + "/" + segs[3]
		switch method {
		case http.MethodPost:
			return opCreateTargetAccountConfiguration, compositeID
		case http.MethodGet:
			return opGetTargetAccountConfiguration, compositeID
		case http.MethodPatch:
			return opUpdateTargetAccountConfiguration, compositeID
		case http.MethodDelete:
			return opDeleteTargetAccountConfiguration, compositeID
		}
	}

	if len(segs) >= 3 && segs[2] == pathTargetAccountConfigurations && method == http.MethodGet {
		return opListTargetAccountConfigurations, segs[1]
	}

	return "", ""
}

// parseFISExperimentPath routes experiment paths.
func parseFISExperimentPath(method string, segs []string, hasID bool) (string, string) {
	if !hasID {
		if method == http.MethodPost {
			return opStartExperiment, ""
		}
		if method == http.MethodGet {
			return opListExperiments, ""
		}

		return "", ""
	}

	if op, id := parseFISExperimentSubPath(method, segs); op != "" {
		return op, id
	}

	switch method {
	case http.MethodGet:
		return opGetExperiment, segs[1]
	case http.MethodDelete:
		return opStopExperiment, segs[1]
	}

	return "", ""
}

// parseFISExperimentSubPath routes sub-paths of experiments.
func parseFISExperimentSubPath(method string, segs []string) (string, string) {
	if method == http.MethodGet && len(segs) >= 3 && segs[2] == subPathResolvedTargets {
		return opListExperimentResolvedTargets, segs[1]
	}

	// POST /experiments/{id}/stop — AWS canonical StopExperiment route.
	if method == http.MethodPost && len(segs) >= 3 && segs[2] == subPathStop {
		return opStopExperiment, segs[1]
	}

	if len(segs) >= 4 && segs[2] == pathTargetAccountConfigurations && method == http.MethodGet {
		return opGetExperimentTargetAccountConfiguration, segs[1] + "/" + segs[3]
	}

	if len(segs) >= 3 && segs[2] == pathTargetAccountConfigurations && method == http.MethodGet {
		return opListExperimentTargetAccountConfigurations, segs[1]
	}

	return "", ""
}

// parseFISActionPath routes action paths.
func parseFISActionPath(method string, segs []string, hasID bool) (string, string) {
	switch {
	case method == http.MethodGet && !hasID:
		return opListActions, ""
	case method == http.MethodGet && hasID:
		return opGetAction, segs[1]
	}

	return "", ""
}

// parseFISTargetResourceTypePath routes target resource type paths.
func parseFISTargetResourceTypePath(method string, segs []string, hasID bool) (string, string) {
	switch {
	case method == http.MethodGet && !hasID:
		return opListTargetResourceTypes, ""
	case method == http.MethodGet && hasID:
		return opGetTargetResourceType, segs[1]
	}

	return "", ""
}

// parseFISTagPath routes tag management paths.
func parseFISTagPath(method string, segs []string, hasID bool) (string, string) {
	if !hasID {
		return "", ""
	}

	arnStr := strings.Join(segs[1:], "/")
	switch method {
	case http.MethodGet:
		return opListTagsForResource, arnStr
	case http.MethodPost:
		return opTagResource, arnStr
	case http.MethodDelete:
		return opUntagResource, arnStr
	}

	return "", ""
}

// parseFISSafetyLeverPath routes safety lever paths.
func parseFISSafetyLeverPath(method string, segs []string, hasID bool) (string, string) {
	if !hasID {
		return "", ""
	}

	switch method {
	case http.MethodGet:
		return opGetSafetyLever, segs[1]
	case http.MethodPatch:
		return opUpdateSafetyLeverState, segs[1]
	}

	return "", ""
}

// pathSegments splits a URL path into non-empty segments.
func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}

	return strings.SplitN(trimmed, "/", maxPathSegments)
}

// ----------------------------------------
// Pagination helpers
// ----------------------------------------

// defaultMaxResults is the default page size for list operations.
const defaultMaxResults = 20

// absoluteMaxResults is the maximum allowed page size.
const absoluteMaxResults = 100

// encodePageToken encodes an integer offset as an opaque base64 token.
func encodePageToken(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// decodePageToken decodes a token produced by encodePageToken.
func decodePageToken(tok string) (int, error) {
	b, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(string(b))
}

// paginateWithToken decodes the offset-based nextToken and returns (pageSize, startOffset).
// The caller slices [startOffset : startOffset+pageSize].
func paginateWithToken(_ []string, q url.Values) (int, int) {
	mr := defaultMaxResults

	if v := q.Get("maxResults"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mr = n
		}
	}

	mr = min(mr, absoluteMaxResults)

	start := 0

	if tok := q.Get("nextToken"); tok != "" {
		if n, err := decodePageToken(tok); err == nil && n > 0 {
			start = n
		}
	}

	return mr, start
}

// paginatePage applies paginateWithToken's offset/limit contract to a slice of items,
// returning the current page and the nextToken for the remainder (empty when the page
// reaches the end of items). ids is used only for maxResults/nextToken decoding, matching
// paginateWithToken's existing contract.
func paginatePage[T any](items []T, ids []string, q url.Values) ([]T, string) {
	maxResults, start := paginateWithToken(ids, q)

	// A stale or tampered nextToken can decode to an offset past the current
	// item count (e.g. the list shrank between calls); clamp before slicing
	// so it degrades to an empty page instead of panicking.
	start = min(start, len(items))

	end := min(start+maxResults, len(items))

	var nextTok string

	if end < len(items) {
		nextTok = encodePageToken(end)
	}

	return items[start:end], nextTok
}
