package emrserverless

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	// listAppsMinResults / listAppsMaxResults bound the maxResults query
	// parameter on EMR Serverless list operations (AWS range: 1-50).
	listAppsMinResults = 1
	listAppsMaxResults = 50

	opUnknown        = "Unknown"
	keyApplicationID = "applicationId"
	keyArn           = "arn"
	keyName          = "name"
	keyState         = "state"
	keyCreatedAt     = "createdAt"
	keyUpdatedAt     = "updatedAt"
	keyTags          = "tags"
	keyReleaseLabel  = "releaseLabel"
	keyStateDetails  = "stateDetails"
	keySessionID     = "sessionId"
	keyCreatedBy     = "createdBy"
	keyType          = "type"
	keyExecutionRole = "executionRole"
	keyAttempt       = "attempt"
	keyMode          = "mode"
)

const (
	opCreateApplication     = "CreateApplication"
	opGetApplication        = "GetApplication"
	opListApplications      = "ListApplications"
	opUpdateApplication     = "UpdateApplication"
	opDeleteApplication     = "DeleteApplication"
	opStartApplication      = "StartApplication"
	opStopApplication       = "StopApplication"
	opStartJobRun           = "StartJobRun"
	opGetJobRun             = "GetJobRun"
	opListJobRuns           = "ListJobRuns"
	opCancelJobRun          = "CancelJobRun"
	opGetDashboardForJobRun = "GetDashboardForJobRun"
	opListJobRunAttempts    = "ListJobRunAttempts"
	opGetResourceDashboard  = "GetResourceDashboard"
	opStartSession          = "StartSession"
	opGetSession            = "GetSession"
	opListSessions          = "ListSessions"
	opTerminateSession      = "TerminateSession"
	opGetSessionEndpoint    = "GetSessionEndpoint"
	opListTagsForResource   = "ListTagsForResource"
	opTagResource           = "TagResource"
	opUntagResource         = "UntagResource"
)

const (
	pathApplications     = "/applications"
	pathTags             = "/tags/"
	emrServerlessService = "emr-serverless"
	emrMatchPriority     = 87
	pathJobRuns          = "jobruns"
	pathSessions         = "sessions"
)

// Handler is the Echo HTTP handler for EMR Serverless operations (REST-JSON protocol).
type Handler struct {
	Backend *InMemoryBackend
}

// NewHandler creates a new EMR Serverless handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{Backend: backend}
}

// Reset clears all backend state. Used for test isolation.
func (h *Handler) Reset() { h.Backend.Reset() }

// Name returns the service name.
func (h *Handler) Name() string { return "EmrServerless" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateApplication,
		opGetApplication,
		opListApplications,
		opUpdateApplication,
		opDeleteApplication,
		opStartApplication,
		opStopApplication,
		opStartJobRun,
		opGetJobRun,
		opListJobRuns,
		opCancelJobRun,
		opGetDashboardForJobRun,
		opListJobRunAttempts,
		opGetResourceDashboard,
		opStartSession,
		opGetSession,
		opListSessions,
		opTerminateSession,
		opGetSessionEndpoint,
		opListTagsForResource,
		opTagResource,
		opUntagResource,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "emr-serverless" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches EMR Serverless requests.
// For /applications paths, it additionally checks the Authorization header
// service name to distinguish from AppConfig (which also uses /applications).
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if strings.HasPrefix(path, pathTags+"arn:aws:emr-serverless:") {
			return true
		}

		if path == pathApplications || strings.HasPrefix(path, pathApplications+"/") {
			return httputils.ExtractServiceFromRequest(c.Request()) == emrServerlessService
		}

		return false
	}
}

// MatchPriority returns the routing priority.
// Uses 87 to be evaluated before AppConfig (priority 86) which also uses /applications paths.
func (h *Handler) MatchPriority() int { return emrMatchPriority }

const (
	pathPartsApplication   = 1
	pathPartsWithSub       = 2
	pathPartsWithJobRun    = 3
	pathPartsWithJobRunSub = 4
)

// emrRoute holds the parsed route information.
type emrRoute struct {
	applicationID string
	jobRunID      string
	sessionID     string
	resourceARN   string
	operation     string
}

// parseEMRPath maps HTTP method + path to an operation and resource identifiers.
func parseEMRPath(method, rawPath string) emrRoute {
	path, _ := url.PathUnescape(rawPath)

	if after, ok := strings.CutPrefix(path, pathTags); ok {
		return parseTagRoute(method, after)
	}

	if path == pathApplications {
		return parseApplicationsCollection(method)
	}

	suffix := strings.TrimPrefix(path, pathApplications+"/")
	parts := strings.SplitN(suffix, "/", pathPartsWithJobRunSub)

	switch len(parts) {
	case pathPartsApplication:

		return parseSingleAppRoute(method, parts[0])
	case pathPartsWithSub:

		return parseAppSubRoute(method, parts[0], parts[1])
	case pathPartsWithJobRun:

		return parseJobRunRoute(method, parts[0], parts[1], parts[2])
	case pathPartsWithJobRunSub:

		return parseJobRunSubRoute(method, parts[0], parts[1], parts[2], parts[3])
	}

	return emrRoute{operation: opUnknown}
}

func parseTagRoute(method, resourceARN string) emrRoute {
	switch method {
	case http.MethodGet:

		return emrRoute{operation: opListTagsForResource, resourceARN: resourceARN}
	case http.MethodPost:

		return emrRoute{operation: opTagResource, resourceARN: resourceARN}
	case http.MethodDelete:

		return emrRoute{operation: opUntagResource, resourceARN: resourceARN}
	}

	return emrRoute{operation: opUnknown}
}

func parseApplicationsCollection(method string) emrRoute {
	switch method {
	case http.MethodPost:

		return emrRoute{operation: opCreateApplication}
	case http.MethodGet:

		return emrRoute{operation: opListApplications}
	}

	return emrRoute{operation: opUnknown}
}

func parseSingleAppRoute(method, appID string) emrRoute {
	switch method {
	case http.MethodGet:

		return emrRoute{operation: opGetApplication, applicationID: appID}
	case http.MethodPatch:

		return emrRoute{operation: opUpdateApplication, applicationID: appID}
	case http.MethodDelete:

		return emrRoute{operation: opDeleteApplication, applicationID: appID}
	}

	return emrRoute{operation: opUnknown}
}

func parseAppSubRoute(method, appID, sub string) emrRoute {
	switch sub {
	case "start":
		if method == http.MethodPost {
			return emrRoute{operation: opStartApplication, applicationID: appID}
		}
	case "stop":
		if method == http.MethodPost {
			return emrRoute{operation: opStopApplication, applicationID: appID}
		}
	case "dashboard":
		if method == http.MethodGet {
			return emrRoute{operation: opGetResourceDashboard, applicationID: appID}
		}
	case pathJobRuns:
		switch method {
		case http.MethodPost:

			return emrRoute{operation: opStartJobRun, applicationID: appID}
		case http.MethodGet:

			return emrRoute{operation: opListJobRuns, applicationID: appID}
		}
	case pathSessions:
		switch method {
		case http.MethodPost:

			return emrRoute{operation: opStartSession, applicationID: appID}
		case http.MethodGet:

			return emrRoute{operation: opListSessions, applicationID: appID}
		}
	}

	return emrRoute{operation: opUnknown}
}

func parseJobRunRoute(method, appID, sub, jobRunID string) emrRoute {
	if sub == pathSessions {
		switch method {
		case http.MethodGet:

			return emrRoute{operation: opGetSession, applicationID: appID, sessionID: jobRunID}
		case http.MethodDelete:

			return emrRoute{operation: opTerminateSession, applicationID: appID, sessionID: jobRunID}
		}
	}

	if sub != pathJobRuns {
		return emrRoute{operation: opUnknown}
	}

	switch method {
	case http.MethodGet:

		return emrRoute{operation: opGetJobRun, applicationID: appID, jobRunID: jobRunID}
	case http.MethodDelete:

		return emrRoute{operation: opCancelJobRun, applicationID: appID, jobRunID: jobRunID}
	}

	return emrRoute{operation: opUnknown}
}

func parseJobRunSubRoute(method, appID, sub, jobRunID, action string) emrRoute {
	if sub == pathSessions && action == "endpoint" && method == http.MethodGet {
		return emrRoute{operation: opGetSessionEndpoint, applicationID: appID, sessionID: jobRunID}
	}

	if sub != pathJobRuns {
		return emrRoute{operation: opUnknown}
	}

	if method != http.MethodGet {
		return emrRoute{operation: opUnknown}
	}

	switch action {
	case "dashboard":

		return emrRoute{operation: opGetDashboardForJobRun, applicationID: appID, jobRunID: jobRunID}
	case "attempts":

		return emrRoute{operation: opListJobRunAttempts, applicationID: appID, jobRunID: jobRunID}
	}

	return emrRoute{operation: opUnknown}
}

// ExtractOperation returns the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	route := parseEMRPath(c.Request().Method, c.Request().URL.Path)

	return route.operation
}

// ExtractResource extracts a resource identifier from the request path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	route := parseEMRPath(c.Request().Method, c.Request().URL.Path)

	if route.resourceARN != "" {
		return route.resourceARN
	}

	if route.jobRunID != "" {
		return route.applicationID + "/" + route.jobRunID
	}

	if route.sessionID != "" {
		return route.applicationID + "/" + route.sessionID
	}

	return route.applicationID
}

// Handler returns the Echo handler function for EMR Serverless requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		log := logger.Load(r.Context())
		route := parseEMRPath(r.Method, r.URL.Path)

		log.Debug("emrserverless request", "operation", route.operation)

		body, err := httputils.ReadBody(r)
		if err != nil {
			log.ErrorContext(r.Context(), "emrserverless: failed to read request body", "error", err)

			return c.JSON(http.StatusInternalServerError, errResp("InternalFailure", "internal server error"))
		}

		c.Response().Header().Set("X-Amzn-RequestId", fmt.Sprintf("%x", time.Now().UnixNano()))

		return h.dispatch(c, route, body)
	}
}

// emrDispatchFn is the function signature for EMR Serverless dispatch handlers.
type emrDispatchFn func(*Handler, *echo.Context, emrRoute, []byte) error

// emrDispatchTable maps operation names to their handler wrappers.
//
//nolint:gochecknoglobals // read-only dispatch table initialized once at startup
var emrDispatchTable = map[string]emrDispatchFn{
	opCreateApplication: func(h *Handler, c *echo.Context, _ emrRoute, body []byte) error {
		return h.handleCreateApplication(c, body)
	},
	opGetApplication: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleGetApplication(c, r.applicationID)
	},
	opListApplications: func(h *Handler, c *echo.Context, _ emrRoute, _ []byte) error {
		return h.handleListApplications(c)
	},
	opUpdateApplication: func(h *Handler, c *echo.Context, r emrRoute, body []byte) error {
		return h.handleUpdateApplication(c, r.applicationID, body)
	},
	opDeleteApplication: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleDeleteApplication(c, r.applicationID)
	},
	opStartApplication: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleStartApplication(c, r.applicationID)
	},
	opStopApplication: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleStopApplication(c, r.applicationID)
	},
	opStartJobRun: func(h *Handler, c *echo.Context, r emrRoute, body []byte) error {
		return h.handleStartJobRun(c, r.applicationID, body)
	},
	opGetJobRun: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleGetJobRun(c, r.applicationID, r.jobRunID)
	},
	opListJobRuns: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleListJobRuns(c, r.applicationID)
	},
	opCancelJobRun: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleCancelJobRun(c, r.applicationID, r.jobRunID)
	},
	opGetDashboardForJobRun: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleGetDashboardForJobRun(c, r.applicationID, r.jobRunID)
	},
	opListJobRunAttempts: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleListJobRunAttempts(c, r.applicationID, r.jobRunID)
	},
	opGetResourceDashboard: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleGetResourceDashboard(c, r.applicationID)
	},
	opStartSession: func(h *Handler, c *echo.Context, r emrRoute, body []byte) error {
		return h.handleStartSession(c, r.applicationID, body)
	},
	opGetSession: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleGetSession(c, r.applicationID, r.sessionID)
	},
	opListSessions: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleListSessions(c, r.applicationID)
	},
	opTerminateSession: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleTerminateSession(c, r.applicationID, r.sessionID)
	},
	opGetSessionEndpoint: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleGetSessionEndpoint(c, r.applicationID, r.sessionID)
	},
	opListTagsForResource: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleListTagsForResource(c, r.resourceARN)
	},
	opTagResource: func(h *Handler, c *echo.Context, r emrRoute, body []byte) error {
		return h.handleTagResource(c, r.resourceARN, body)
	},
	opUntagResource: func(h *Handler, c *echo.Context, r emrRoute, _ []byte) error {
		return h.handleUntagResource(c, r.resourceARN, c.Request().URL.Query())
	},
}

func (h *Handler) dispatch(c *echo.Context, route emrRoute, body []byte) error {
	if fn, ok := emrDispatchTable[route.operation]; ok {
		return fn(h, c, route, body)
	}

	return c.JSON(http.StatusNotFound, errResp("ResourceNotFoundException", "unknown operation: "+route.operation))
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):

		return c.JSON(http.StatusNotFound, errResp("ResourceNotFoundException", err.Error()))
	case errors.Is(err, ErrAlreadyExists):

		return c.JSON(http.StatusConflict, errResp("ConflictException", err.Error()))
	case errors.Is(err, ErrValidation):

		return c.JSON(http.StatusBadRequest, errResp("ValidationException", err.Error()))
	case errors.Is(err, ErrInvalidState):

		return c.JSON(http.StatusBadRequest, errResp("ValidationException", err.Error()))
	case errors.Is(err, ErrConflict):

		return c.JSON(http.StatusConflict, errResp("ConflictException", err.Error()))
	default:

		return c.JSON(http.StatusInternalServerError, errResp("InternalFailure", err.Error()))
	}
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"code": code, "message": msg}
}

// epochSeconds converts a [time.Time] to a float64 Unix epoch seconds value,
// matching the AWS REST-JSON timestamp serialization format.
func epochSeconds(ts interface{ Unix() int64 }) float64 {
	return float64(ts.Unix())
}

// applicationToMap converts an Application to a map with float64 timestamps
// for correct AWS REST-JSON serialization. Returns a map representation with
// createdAt/updatedAt as float64 Unix epoch seconds values.
// Tags are always included (as an empty map if none are set).
func applicationToMap(app *Application) map[string]any {
	m := map[string]any{
		keyApplicationID: app.ApplicationID,
		"id":             app.ApplicationID, // ApplicationSummary.id in AWS SDK ListApplications response
		keyArn:           app.Arn,
		keyName:          app.Name,
		keyType:          app.Type,
		keyReleaseLabel:  app.ReleaseLabel,
		keyState:         app.State,
		keyCreatedAt:     epochSeconds(app.CreatedAt),
		keyUpdatedAt:     epochSeconds(app.UpdatedAt),
		keyTags:          app.Tags,
	}
	if app.Architecture != "" {
		m["architecture"] = app.Architecture
	}
	if app.StateDetails != "" {
		m[keyStateDetails] = app.StateDetails
	}

	maps.Copy(m, app.ExtraConfig)

	return m
}

// applicationSummaryToMap builds the types.ApplicationSummary shape
// (types/types.go:119) -- no applicationId (Summary uses "id" only, unlike
// the full Application type), tags, or any ExtraConfig sub-object
// (maximumCapacity, networkConfiguration, autoStartConfiguration, etc, up
// to 14 -- see applicationConfigFieldCount), all of which are Get-only
// (confirmed against awsRestjson1_deserializeDocumentApplicationSummary,
// which recognises only architecture/arn/createdAt/id/name/releaseLabel/
// state/stateDetails/type/updatedAt).
func applicationSummaryToMap(app *Application) map[string]any {
	m := map[string]any{
		"id":            app.ApplicationID,
		keyArn:          app.Arn,
		keyName:         app.Name,
		keyType:         app.Type,
		keyReleaseLabel: app.ReleaseLabel,
		keyState:        app.State,
		keyCreatedAt:    epochSeconds(app.CreatedAt),
		keyUpdatedAt:    epochSeconds(app.UpdatedAt),
	}
	if app.Architecture != "" {
		m["architecture"] = app.Architecture
	}
	if app.StateDetails != "" {
		m[keyStateDetails] = app.StateDetails
	}

	return m
}

// jobRunToMap converts a JobRun to a map with float64 timestamps
// for correct AWS REST-JSON serialization. Returns a map representation with
// createdAt/updatedAt as float64 Unix epoch seconds values.
// Tags are always included (as an empty map if none are set).
func jobRunToMap(jr *JobRun) map[string]any {
	m := map[string]any{
		keyApplicationID: jr.ApplicationID,
		"jobRunId":       jr.JobRunID,
		"id":             jr.JobRunID, // JobRunSummary.id in AWS SDK ListJobRuns response
		keyArn:           jr.Arn,
		keyName:          jr.Name,
		keyState:         jr.State,
		keyStateDetails:  jr.StateDetails,
		keyMode:          jr.Mode,
		// The response wire field is "executionRole" (types.JobRun.ExecutionRole /
		// types.JobRunSummary.ExecutionRole), NOT "executionRoleArn" -- that name
		// is only used on the StartJobRunInput *request* body. Confirmed against
		// deserializeDocumentJobRun / deserializeDocumentJobRunSummary.
		keyExecutionRole:          jr.ExecutionRoleArn,
		keyCreatedBy:              jr.CreatedBy,
		"executionTimeoutMinutes": jr.ExecutionTimeoutMinutes,
		keyCreatedAt:              epochSeconds(jr.CreatedAt),
		keyUpdatedAt:              epochSeconds(jr.UpdatedAt),
		keyTags:                   jr.Tags,
		keyAttempt:                0,
		// ReleaseLabel and JobDriver (types.JobRun, both "This member is
		// required.") must survive even when empty/nil: ReleaseLabel is copied
		// from the application, whose CreateApplicationInput.ReleaseLabel
		// validator only null-checks the pointer (not its content), and
		// JobDriver is genuinely optional on StartJobRunInput
		// (validateOpStartJobRunInput only validates its content when
		// non-nil). Both were previously guarded by conditionals that dropped
		// the required key entirely for a reachable real-client zero state.
		keyReleaseLabel: jr.ReleaseLabel,
		"jobDriver":     jr.JobDriver,
	}
	if jr.ConfigurationOverrides != nil {
		m["configurationOverrides"] = jr.ConfigurationOverrides
	}
	if jr.ExecutionIamPolicy != nil {
		m["executionIamPolicy"] = jr.ExecutionIamPolicy
	}
	if jr.RetryPolicy != nil {
		m["retryPolicy"] = jr.RetryPolicy
	}

	return m
}

// jobRunSummaryToMap builds the types.JobRunSummary shape
// (types/types.go:661) -- no jobRunId (Summary uses "id" only, unlike the
// full JobRun type), tags, executionTimeoutMinutes, jobDriver,
// configurationOverrides, executionIamPolicy, or retryPolicy, all of which
// are Get-only (confirmed against
// awsRestjson1_deserializeDocumentJobRunSummary, which recognises only
// applicationId/arn/attempt/attemptCreatedAt/attemptUpdatedAt/createdAt/
// createdBy/executionRole/id/mode/name/releaseLabel/state/stateDetails/
// type/updatedAt).
func jobRunSummaryToMap(jr *JobRun) map[string]any {
	m := map[string]any{
		keyApplicationID: jr.ApplicationID,
		"id":             jr.JobRunID,
		keyArn:           jr.Arn,
		keyName:          jr.Name,
		keyState:         jr.State,
		keyStateDetails:  jr.StateDetails,
		keyMode:          jr.Mode,
		keyExecutionRole: jr.ExecutionRoleArn,
		keyCreatedBy:     jr.CreatedBy,
		keyCreatedAt:     epochSeconds(jr.CreatedAt),
		keyUpdatedAt:     epochSeconds(jr.UpdatedAt),
		keyAttempt:       0,
		// ReleaseLabel is required by types.JobRunSummary; see the matching
		// comment in jobRunToMap for why it must never be conditional.
		keyReleaseLabel: jr.ReleaseLabel,
	}

	return m
}

// --- Application handlers ---

// applicationConfigFields holds the EMR Serverless application configuration
// sub-objects this in-memory backend does not interpret (it doesn't actually
// provision Spark/Hive workers) but must still store and echo back verbatim
// on GetApplication/ListApplications -- see Application.ExtraConfig. Each
// field is typed `any` so Go's JSON codec preserves whatever shape the
// caller sent (object or, for runtimeConfiguration, array) without this
// backend needing typed structs for every AWS sub-schema. Embedded (not
// named) in createApplicationBody/updateApplicationBody so its JSON tags are
// promoted to the top level of the request body.
type applicationConfigFields struct {
	InitialCapacity                     any `json:"initialCapacity,omitempty"`
	MaximumCapacity                     any `json:"maximumCapacity,omitempty"`
	AutoStartConfiguration              any `json:"autoStartConfiguration,omitempty"`
	AutoStopConfiguration               any `json:"autoStopConfiguration,omitempty"`
	NetworkConfiguration                any `json:"networkConfiguration,omitempty"`
	ImageConfiguration                  any `json:"imageConfiguration,omitempty"`
	MonitoringConfiguration             any `json:"monitoringConfiguration,omitempty"`
	WorkerTypeSpecifications            any `json:"workerTypeSpecifications,omitempty"`
	RuntimeConfiguration                any `json:"runtimeConfiguration,omitempty"`
	InteractiveConfiguration            any `json:"interactiveConfiguration,omitempty"`
	IdentityCenterConfiguration         any `json:"identityCenterConfiguration,omitempty"`
	DiskEncryptionConfiguration         any `json:"diskEncryptionConfiguration,omitempty"`
	JobLevelCostAllocationConfiguration any `json:"jobLevelCostAllocationConfiguration,omitempty"`
	SchedulerConfiguration              any `json:"schedulerConfiguration,omitempty"`
}

// applicationConfigFieldCount is the number of sub-object fields in
// applicationConfigFields, used to size the map toMap builds.
const applicationConfigFieldCount = 14

// toMap returns the subset of fields present in the request, keyed by their
// AWS wire field name, ready to merge into Application.ExtraConfig.
func (f applicationConfigFields) toMap() map[string]any {
	m := make(map[string]any, applicationConfigFieldCount)

	add := func(key string, val any) {
		if val != nil {
			m[key] = val
		}
	}

	add("initialCapacity", f.InitialCapacity)
	add("maximumCapacity", f.MaximumCapacity)
	add("autoStartConfiguration", f.AutoStartConfiguration)
	add("autoStopConfiguration", f.AutoStopConfiguration)
	add("networkConfiguration", f.NetworkConfiguration)
	add("imageConfiguration", f.ImageConfiguration)
	add("monitoringConfiguration", f.MonitoringConfiguration)
	add("workerTypeSpecifications", f.WorkerTypeSpecifications)
	add("runtimeConfiguration", f.RuntimeConfiguration)
	add("interactiveConfiguration", f.InteractiveConfiguration)
	add("identityCenterConfiguration", f.IdentityCenterConfiguration)
	add("diskEncryptionConfiguration", f.DiskEncryptionConfiguration)
	add("jobLevelCostAllocationConfiguration", f.JobLevelCostAllocationConfiguration)
	add("schedulerConfiguration", f.SchedulerConfiguration)

	return m
}

type createApplicationBody struct {
	applicationConfigFields
	Tags         map[string]string `json:"tags"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	ReleaseLabel string            `json:"releaseLabel"`
	Architecture string            `json:"architecture"`
	ClientToken  string            `json:"clientToken"`
}

type createApplicationResponse struct {
	ApplicationID string `json:"applicationId"`
	Arn           string `json:"arn"`
	Name          string `json:"name"`
}

func (h *Handler) handleCreateApplication(c *echo.Context, body []byte) error {
	var in createApplicationBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	extra := in.toMap()
	if err := validateAutoStopConfig(extra); err != nil {
		return h.handleError(c, err)
	}

	app, err := h.Backend.CreateApplication(in.Name, in.Type, in.ReleaseLabel, in.Architecture, in.Tags,
		CreateApplicationOptions{ClientToken: in.ClientToken, ExtraConfig: extra})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, createApplicationResponse{
		ApplicationID: app.ApplicationID,
		Arn:           app.Arn,
		Name:          app.Name,
	})
}

func (h *Handler) handleGetApplication(c *echo.Context, applicationID string) error {
	app, err := h.Backend.GetApplication(applicationID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"application": applicationToMap(app)})
}

func (h *Handler) handleListApplications(c *echo.Context) error {
	q := c.Request().URL.Query()
	nextToken := q.Get("nextToken")

	maxResults := 0
	if s := q.Get("maxResults"); s != "" {
		// AWS EMR Serverless bounds list maxResults to 1-50.
		n, err := strconv.Atoi(s)
		if err != nil || n < listAppsMinResults || n > listAppsMaxResults {
			return c.JSON(http.StatusBadRequest, errResp(
				"ValidationException",
				"maxResults must be between 1 and 50",
			))
		}

		maxResults = n
	}

	var states []string
	if s := q.Get("states"); s != "" {
		for st := range strings.SplitSeq(s, ",") {
			if trimmed := strings.TrimSpace(st); trimmed != "" {
				states = append(states, trimmed)
			}
		}
	}

	apps, outToken := h.Backend.ListApplications(nextToken, maxResults, states...)
	list := make([]map[string]any, 0, len(apps))

	for _, app := range apps {
		list = append(list, applicationSummaryToMap(app))
	}

	resp := map[string]any{"applications": list}
	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

type updateApplicationBody struct {
	applicationConfigFields
	ReleaseLabel string `json:"releaseLabel"`
}

func (h *Handler) handleUpdateApplication(c *echo.Context, applicationID string, body []byte) error {
	var in updateApplicationBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	extra := in.toMap()
	if err := validateAutoStopConfig(extra); err != nil {
		return h.handleError(c, err)
	}

	app, err := h.Backend.UpdateApplication(applicationID, func(a *Application) {
		if in.ReleaseLabel != "" {
			a.ReleaseLabel = in.ReleaseLabel
		}

		if len(extra) > 0 {
			if a.ExtraConfig == nil {
				a.ExtraConfig = make(map[string]any, len(extra))
			}

			maps.Copy(a.ExtraConfig, extra)
		}
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"application": applicationToMap(app)})
}

func (h *Handler) handleDeleteApplication(c *echo.Context, applicationID string) error {
	if err := h.Backend.DeleteApplication(applicationID); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleStartApplication(c *echo.Context, applicationID string) error {
	if err := h.Backend.StartApplication(applicationID); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleStopApplication(c *echo.Context, applicationID string) error {
	if err := h.Backend.StopApplication(applicationID); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

// --- JobRun handlers ---

type startJobRunBody struct {
	Tags                    map[string]string `json:"tags"`
	JobDriver               any               `json:"jobDriver"`
	ConfigurationOverrides  any               `json:"configurationOverrides"`
	ExecutionIamPolicy      any               `json:"executionIamPolicy"`
	RetryPolicy             any               `json:"retryPolicy"`
	ExecutionRoleArn        string            `json:"executionRoleArn"`
	Name                    string            `json:"name"`
	Mode                    string            `json:"mode"`
	ClientToken             string            `json:"clientToken"`
	ExecutionTimeoutMinutes int64             `json:"executionTimeoutMinutes"`
}

type startJobRunResponse struct {
	ApplicationID string `json:"applicationId"`
	Arn           string `json:"arn"`
	JobRunID      string `json:"jobRunId"`
}

func (h *Handler) handleStartJobRun(c *echo.Context, applicationID string, body []byte) error {
	var in startJobRunBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	jr, err := h.Backend.StartJobRun(applicationID, in.ExecutionRoleArn, in.Name, in.Mode, in.Tags,
		StartJobRunOptions{
			ClientToken:             in.ClientToken,
			JobDriver:               in.JobDriver,
			ConfigurationOverrides:  in.ConfigurationOverrides,
			ExecutionIamPolicy:      in.ExecutionIamPolicy,
			RetryPolicy:             in.RetryPolicy,
			ExecutionTimeoutMinutes: in.ExecutionTimeoutMinutes,
		})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, startJobRunResponse{
		ApplicationID: jr.ApplicationID,
		Arn:           jr.Arn,
		JobRunID:      jr.JobRunID,
	})
}

func (h *Handler) handleGetJobRun(c *echo.Context, applicationID, jobRunID string) error {
	jr, err := h.Backend.GetJobRun(applicationID, jobRunID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"jobRun": jobRunToMap(jr)})
}

func (h *Handler) handleListJobRuns(c *echo.Context, applicationID string) error {
	q := c.Request().URL.Query()
	nextToken := q.Get("nextToken")

	maxResults := 0
	if s := q.Get("maxResults"); s != "" {
		// AWS EMR Serverless bounds list maxResults to 1-50.
		n, err := strconv.Atoi(s)
		if err != nil || n < listAppsMinResults || n > listAppsMaxResults {
			return c.JSON(http.StatusBadRequest, errResp(
				"ValidationException",
				"maxResults must be between 1 and 50",
			))
		}

		maxResults = n
	}

	var states []string
	if s := q.Get("states"); s != "" {
		for st := range strings.SplitSeq(s, ",") {
			if trimmed := strings.TrimSpace(st); trimmed != "" {
				states = append(states, trimmed)
			}
		}
	}

	runs, outToken, err := h.Backend.ListJobRuns(applicationID, nextToken, maxResults, states...)
	if err != nil {
		return h.handleError(c, err)
	}

	list := make([]map[string]any, 0, len(runs))

	for _, jr := range runs {
		list = append(list, jobRunSummaryToMap(jr))
	}

	resp := map[string]any{"jobRuns": list}
	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleCancelJobRun(c *echo.Context, applicationID, jobRunID string) error {
	jr, err := h.Backend.CancelJobRun(applicationID, jobRunID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyApplicationID: jr.ApplicationID,
		"jobRunId":       jr.JobRunID,
	})
}

func (h *Handler) handleGetDashboardForJobRun(c *echo.Context, applicationID, jobRunID string) error {
	dashURL, err := h.Backend.GetDashboardForJobRun(applicationID, jobRunID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"url": dashURL})
}

// jobRunAttemptToMap converts a JobRunAttemptSummary to a map with float64 timestamps.
func jobRunAttemptToMap(a *JobRunAttemptSummary) map[string]any {
	return map[string]any{
		keyApplicationID: a.ApplicationID,
		keyArn:           a.Arn,
		keyCreatedAt:     epochSeconds(a.CreatedAt),
		keyUpdatedAt:     epochSeconds(a.UpdatedAt),
		"jobCreatedAt":   epochSeconds(a.JobCreatedAt),
		keyCreatedBy:     a.CreatedBy,
		keyExecutionRole: a.ExecutionRole,
		"id":             a.ID,
		"releaseLabel":   a.ReleaseLabel,
		keyState:         a.State,
		"stateDetails":   a.StateDetails,
		keyName:          a.Name,
		keyType:          a.Type,
		keyMode:          a.Mode,
		keyAttempt:       a.Attempt,
	}
}

func (h *Handler) handleListJobRunAttempts(c *echo.Context, applicationID, jobRunID string) error {
	q := c.Request().URL.Query()
	nextToken := q.Get("nextToken")

	maxResults := 0
	if s := q.Get("maxResults"); s != "" {
		// AWS EMR Serverless bounds list maxResults to 1-50.
		n, err := strconv.Atoi(s)
		if err != nil || n < listAppsMinResults || n > listAppsMaxResults {
			return c.JSON(http.StatusBadRequest, errResp(
				"ValidationException",
				"maxResults must be between 1 and 50",
			))
		}

		maxResults = n
	}

	attempts, outToken, err := h.Backend.ListJobRunAttempts(applicationID, jobRunID, nextToken, maxResults)
	if err != nil {
		return h.handleError(c, err)
	}

	list := make([]map[string]any, 0, len(attempts))

	for _, a := range attempts {
		list = append(list, jobRunAttemptToMap(a))
	}

	resp := map[string]any{"jobRunAttempts": list}
	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

// --- Tags handlers ---

func (h *Handler) handleListTagsForResource(c *echo.Context, resourceARN string) error {
	tags, err := h.Backend.ListTagsForResource(resourceARN)
	if err != nil {
		return h.handleError(c, err)
	}

	if tags == nil {
		tags = map[string]string{}
	}

	return c.JSON(http.StatusOK, map[string]any{keyTags: tags})
}

type tagResourceBody struct {
	Tags map[string]string `json:"tags"`
}

func (h *Handler) handleTagResource(c *echo.Context, resourceARN string, body []byte) error {
	var in tagResourceBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	if err := h.Backend.TagResource(resourceARN, in.Tags); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleUntagResource(c *echo.Context, resourceARN string, query url.Values) error {
	tagKeys := query["tagKeys"]

	if err := h.Backend.UntagResource(resourceARN, tagKeys); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}
