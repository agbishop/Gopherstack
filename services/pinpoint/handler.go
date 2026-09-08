package pinpoint

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	pinpointService         = "mobiletargeting"
	pinpointMatchPriority   = 87
	appSubPathParts         = 2
	pinpointDefaultPageSize = 500
	defaultPageSize         = 100 // page size used by parsePageParams/applyPageParams below

	templateSubPathParts = 2
	unknownOperation     = "Unknown"

	// kpiDefaultRangeDays is the trailing window used to synthesise
	// StartTime/EndTime when a GetXxxDateRangeKpi request omits the
	// (optional) start-time/end-time query params.
	kpiDefaultRangeDays = 7

	// sub-path segment constants used throughout dispatch helpers.
	subPathJobsExport       = "jobs/export"
	subPathJobsImport       = "jobs/import"
	subPathVersions         = "versions"
	subPathExecutionMetrics = "execution-metrics"
	phoneValidatePath       = "/v1/phone/number/validate"

	// dispatchSplitN is the split count used in journey/campaign/segment sub-path dispatch.
	dispatchSplitTwo   = 2
	dispatchSplitThree = 3

	// acceptedMessage is the standard response message for accepted operations.
	acceptedMessage = "Accepted"
)

// errInvalidRequestBody is returned by createTemplateByType when the request body cannot be parsed.
var errInvalidRequestBody = errors.New("invalid request body")

// errUnsupportedTemplateType is returned when an unknown template type is provided.
var errUnsupportedTemplateType = errors.New("unsupported template type")

// Handler is the HTTP handler for the Amazon Pinpoint REST API.
type Handler struct {
	Backend       StorageBackend
	AccountID     string
	DefaultRegion string
}

// NewHandler creates a new Pinpoint handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Reset clears the handler's backend state (used for test isolation).
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "Pinpoint" }

// GetSupportedOperations returns the list of supported Pinpoint operations.
// The list is assembled from per-resource-family helper functions (below)
// purely to keep this function's own line count down for the funlen linter
// (suppressing it is banned project-wide), and there is no real branching
// logic to simplify here, only a long literal list of operation names. No
// package-level state is introduced: each helper returns a fresh local slice.
func (h *Handler) GetSupportedOperations() []string {
	var ops []string

	for _, group := range [][]string{
		supportedOpsAppFamily(), supportedOpsTagFamily(), supportedOpsCampaignFamily(),
		supportedOpsSegmentFamily(), supportedOpsJourneyFamily(), supportedOpsTemplateFamily(),
		supportedOpsChannelFamily(), supportedOpsEndpointFamily(), supportedOpsEventStreamFamily(),
		supportedOpsMessagingFamily(), supportedOpsEventFamily(), supportedOpsPhoneFamily(),
		supportedOpsJobFamily(), supportedOpsRecommenderFamily(),
	} {
		ops = append(ops, group...)
	}

	return ops
}

func supportedOpsAppFamily() []string {
	return []string{
		"CreateApp", "DeleteApp", "GetApp", "GetApplicationSettings", "GetApps",
		"UpdateApplicationSettings", "GetApplicationDateRangeKpi",
	}
}

func supportedOpsTagFamily() []string {
	return []string{"ListTagsForResource", "TagResource", "UntagResource"}
}

func supportedOpsCampaignFamily() []string {
	return []string{
		"CreateCampaign", "DeleteCampaign", "GetCampaign", "GetCampaigns", "UpdateCampaign",
		"GetCampaignActivities", "GetCampaignDateRangeKpi", "GetCampaignVersion", "GetCampaignVersions",
	}
}

func supportedOpsSegmentFamily() []string {
	return []string{
		"CreateSegment", "DeleteSegment", "GetSegment", "GetSegments", "UpdateSegment",
		"GetSegmentExportJobs", "GetSegmentImportJobs", "GetSegmentVersion", "GetSegmentVersions",
	}
}

func supportedOpsJourneyFamily() []string {
	return []string{
		"CreateJourney", "DeleteJourney", "GetJourney", "ListJourneys", "UpdateJourney",
		"UpdateJourneyState", "GetJourneyDateRangeKpi", "GetJourneyExecutionMetrics",
		"GetJourneyExecutionActivityMetrics", "GetJourneyRuns", "GetJourneyRunExecutionMetrics",
		"GetJourneyRunExecutionActivityMetrics",
	}
}

func supportedOpsTemplateFamily() []string {
	return []string{
		"CreateEmailTemplate", "GetEmailTemplate", "UpdateEmailTemplate", "DeleteEmailTemplate",
		"CreateInAppTemplate", "GetInAppTemplate", "UpdateInAppTemplate", "DeleteInAppTemplate",
		"CreatePushTemplate", "GetPushTemplate", "UpdatePushTemplate", "DeletePushTemplate",
		"CreateSmsTemplate", "GetSmsTemplate", "UpdateSmsTemplate", "DeleteSmsTemplate",
		"CreateVoiceTemplate", "GetVoiceTemplate", "UpdateVoiceTemplate", "DeleteVoiceTemplate",
		"ListTemplates", "ListTemplateVersions", "UpdateTemplateActiveVersion",
	}
}

func supportedOpsChannelFamily() []string {
	return []string{
		"GetAdmChannel", "UpdateAdmChannel", "DeleteAdmChannel",
		"GetApnsChannel", "UpdateApnsChannel", "DeleteApnsChannel",
		"GetApnsSandboxChannel", "UpdateApnsSandboxChannel", "DeleteApnsSandboxChannel",
		"GetApnsVoipChannel", "UpdateApnsVoipChannel", "DeleteApnsVoipChannel",
		"GetApnsVoipSandboxChannel", "UpdateApnsVoipSandboxChannel", "DeleteApnsVoipSandboxChannel",
		"GetBaiduChannel", "UpdateBaiduChannel", "DeleteBaiduChannel",
		"GetEmailChannel", "UpdateEmailChannel", "DeleteEmailChannel",
		"GetGcmChannel", "UpdateGcmChannel", "DeleteGcmChannel",
		"GetSmsChannel", "UpdateSmsChannel", "DeleteSmsChannel",
		"GetVoiceChannel", "UpdateVoiceChannel", "DeleteVoiceChannel",
		"GetChannels",
	}
}

func supportedOpsEndpointFamily() []string {
	return []string{
		"GetEndpoint", "UpdateEndpoint", "DeleteEndpoint", "GetUserEndpoints",
		"DeleteUserEndpoints", "UpdateEndpointsBatch", "RemoveAttributes",
	}
}

func supportedOpsEventStreamFamily() []string {
	return []string{"GetEventStream", "PutEventStream", "DeleteEventStream"}
}

func supportedOpsMessagingFamily() []string {
	return []string{"SendMessages", "SendUsersMessages", "SendOTPMessage", "VerifyOTPMessage"}
}

func supportedOpsEventFamily() []string {
	return []string{"PutEvents", "GetInAppMessages"}
}

func supportedOpsPhoneFamily() []string {
	return []string{"PhoneNumberValidate"}
}

func supportedOpsJobFamily() []string {
	return []string{
		"CreateExportJob", "GetExportJob", "GetExportJobs",
		"CreateImportJob", "GetImportJob", "GetImportJobs",
	}
}

func supportedOpsRecommenderFamily() []string {
	return []string{
		"CreateRecommenderConfiguration", "GetRecommenderConfiguration", "GetRecommenderConfigurations",
		"UpdateRecommenderConfiguration", "DeleteRecommenderConfiguration",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return pinpointService }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.DefaultRegion} }

// RouteMatcher returns a function that matches Pinpoint API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		if httputils.ExtractServiceFromRequest(c.Request()) != pinpointService {
			return false
		}

		path := c.Request().URL.Path

		return strings.HasPrefix(path, "/v1/apps") ||
			strings.HasPrefix(path, "/v1/tags/") ||
			strings.HasPrefix(path, "/v1/templates") ||
			strings.HasPrefix(path, "/v1/recommenders") ||
			strings.HasPrefix(path, "/v1/phone/") ||
			path == phoneValidatePath
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return pinpointMatchPriority }

// ExtractOperation extracts the operation name from the request path and method.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	method := c.Request().Method
	path := c.Request().URL.Path

	if op := extractTagOrAppsOp(h, method, path); op != unknownOperation {
		return op
	}

	return extractRecommenderTemplateOp(h, method, path)
}

// extractTagOrAppsOp handles tag and app operations.
func extractTagOrAppsOp(h *Handler, method, path string) string {
	switch {
	case strings.HasPrefix(path, "/v1/tags/"):
		return extractTagOperation(method)
	case path == "/v1/apps" || path == "/v1/apps/":
		return extractAppsCollectionOp(method)
	case strings.HasPrefix(path, "/v1/apps/"):
		return extractAppsResourceOp(h, method, path)
	}

	return unknownOperation
}

// extractRecommenderTemplateOp handles recommender and template operations.
func extractRecommenderTemplateOp(h *Handler, method, path string) string {
	switch {
	case path == "/v1/recommenders" || path == "/v1/recommenders/":
		return extractRecommendersCollectionOp(method)
	case strings.HasPrefix(path, "/v1/recommenders/"):
		return extractRecommenderResourceOp(method, path)
	case path == "/v1/templates" || path == "/v1/templates/":
		return "ListTemplates"
	case strings.HasPrefix(path, "/v1/templates/"):
		return h.extractTemplateOperation(method, path)
	case path == phoneValidatePath:
		return "PhoneNumberValidate"
	}

	return unknownOperation
}

func (h *Handler) extractAppSubOperation(method, suffix string) string {
	parts := strings.SplitN(suffix, "/", appSubPathParts)
	if len(parts) != appSubPathParts {
		return unknownOperation
	}

	subPath := parts[1]

	if op := h.extractAppCoreSubOp(method, subPath); op != unknownOperation {
		return op
	}

	return h.extractAppExtSubOp(method, subPath)
}

// extractAppCoreSubOp resolves settings, campaign, journey, segment, and job operations.
func (h *Handler) extractAppCoreSubOp(method, subPath string) string {
	switch {
	case subPath == "settings":
		return extractSettingsOp(method)
	case subPath == "campaigns":
		return extractCampaignsOp(method)
	case strings.HasPrefix(subPath, "campaigns/"):
		return h.extractCampaignSubOp(method, strings.TrimPrefix(subPath, "campaigns/"))
	case subPath == "journeys":
		return extractJourneysOp(method)
	case strings.HasPrefix(subPath, "journeys/"):
		return h.extractJourneySubOp(method, strings.TrimPrefix(subPath, "journeys/"))
	case subPath == "segments":
		return extractSegmentsOp(method)
	case strings.HasPrefix(subPath, "segments/"):
		return h.extractSegmentSubOp(method, strings.TrimPrefix(subPath, "segments/"))
	case subPath == subPathJobsExport:
		return extractExportJobsOp(method)
	case strings.HasPrefix(subPath, subPathJobsExport+"/"):
		return "GetExportJob"
	case subPath == subPathJobsImport:
		return extractImportJobsOp(method)
	case strings.HasPrefix(subPath, subPathJobsImport+"/"):
		return "GetImportJob"
	}

	return unknownOperation
}

// extractAppExtSubOp resolves messaging, KPI, channel, endpoint, and user operations.
func (h *Handler) extractAppExtSubOp(method, subPath string) string {
	switch {
	case subPath == "eventstream":
		return extractEventStreamOp(method)
	case subPath == "messages":
		return "SendMessages"
	case subPath == "users-messages":
		return "SendUsersMessages"
	case subPath == "otp":
		return "SendOTPMessage"
	case subPath == "verify-otp":
		return "VerifyOTPMessage"
	case subPath == "events":
		return "PutEvents"
	case strings.HasPrefix(subPath, "kpis/daterange/"):
		return "GetApplicationDateRangeKpi"
	case subPath == "channels":
		return "GetChannels"
	case strings.HasPrefix(subPath, "channels/"):
		return h.extractChannelOp(method, strings.TrimPrefix(subPath, "channels/"))
	case subPath == "endpoints":
		return "UpdateEndpointsBatch"
	case strings.HasPrefix(subPath, "endpoints/"):
		return h.extractEndpointSubOp(method, strings.TrimPrefix(subPath, "endpoints/"))
	case strings.HasPrefix(subPath, "users/"):
		userID := strings.TrimPrefix(subPath, "users/")
		if userID != "" {
			return extractUserEndpointsOp(method)
		}
	case strings.HasPrefix(subPath, "attributes/"):
		return "RemoveAttributes"
	}

	return unknownOperation
}

// ExtractResource extracts the app ID or decoded ARN from the request path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path

	switch {
	case strings.HasPrefix(path, "/v1/apps/"):
		return strings.TrimPrefix(path, "/v1/apps/")
	case strings.HasPrefix(path, "/v1/tags/"):
		escaped := strings.TrimPrefix(path, "/v1/tags/")
		decoded, err := url.PathUnescape(escaped)
		if err != nil {
			return escaped
		}

		return decoded
	case strings.HasPrefix(path, "/v1/templates/"):
		return strings.TrimPrefix(path, "/v1/templates/")
	case strings.HasPrefix(path, "/v1/recommenders"):
		return strings.TrimPrefix(path, "/v1/recommenders")
	}

	return ""
}

// Handler returns the echo.HandlerFunc for this service.
func (h *Handler) Handler() echo.HandlerFunc {
	return h.ServeHTTP
}

// ServeHTTP dispatches Pinpoint API requests.
func (h *Handler) ServeHTTP(c *echo.Context) error {
	path := c.Request().URL.Path

	switch {
	case strings.HasPrefix(path, "/v1/tags/"):
		return h.dispatchTags(c, path)
	case path == "/v1/apps" || path == "/v1/apps/":
		return h.dispatchApps(c)
	case strings.HasPrefix(path, "/v1/apps/"):
		suffix := strings.TrimPrefix(path, "/v1/apps/")
		if strings.Contains(suffix, "/") {
			return h.dispatchAppSubPath(c, suffix)
		}

		return h.dispatchApp(c, suffix)
	case path == "/v1/recommenders" || path == "/v1/recommenders/":
		return h.dispatchRecommenders(c)
	case strings.HasPrefix(path, "/v1/recommenders/"):
		return h.dispatchRecommenderByID(c, strings.TrimPrefix(path, "/v1/recommenders/"))
	case strings.HasPrefix(path, "/v1/templates"):
		return h.dispatchTemplates(c, path)
	case path == phoneValidatePath:
		return h.handlePhoneNumberValidate(c)
	}

	ctx := c.Request().Context()
	log := logger.Load(ctx)
	log.WarnContext(ctx, "pinpoint: unhandled request", "method", c.Request().Method, "path", path)

	return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", "resource not found")
}

func (h *Handler) dispatchAppSubPath(c *echo.Context, suffix string) error {
	parts := strings.SplitN(suffix, "/", appSubPathParts)
	if len(parts) != appSubPathParts {
		return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", "resource not found")
	}

	appID, subPath := parts[0], parts[1]

	if handled, err := h.tryAppCoreSubPath(c, appID, subPath); handled {
		return err
	}

	if handled, err := h.tryAppExtSubPath(c, appID, subPath); handled {
		return err
	}

	return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", "resource not found")
}

// tryAppCoreSubPath handles settings, campaigns, journeys, segments, and job paths.
// Returns (true, err) if the path was handled.
func (h *Handler) tryAppCoreSubPath(c *echo.Context, appID, subPath string) (bool, error) {
	switch {
	case subPath == "settings":
		return true, h.dispatchAppSettings(c, appID)
	case subPath == "campaigns":
		return true, h.dispatchCampaigns(c, appID)
	case strings.HasPrefix(subPath, "campaigns/"):
		return true, h.dispatchCampaignByID(c, appID, strings.TrimPrefix(subPath, "campaigns/"))
	case subPath == "journeys":
		return true, h.dispatchJourneys(c, appID)
	case strings.HasPrefix(subPath, "journeys/"):
		return true, h.dispatchJourneyByID(c, appID, strings.TrimPrefix(subPath, "journeys/"))
	case subPath == "segments":
		return true, h.dispatchSegments(c, appID)
	case strings.HasPrefix(subPath, "segments/"):
		return true, h.dispatchSegmentByID(c, appID, strings.TrimPrefix(subPath, "segments/"))
	case subPath == subPathJobsExport:
		return true, h.dispatchExportJobs(c, appID, "")
	case strings.HasPrefix(subPath, subPathJobsExport+"/"):
		return true, h.handleGetExportJob(c, appID, strings.TrimPrefix(subPath, "jobs/export/"))
	case subPath == subPathJobsImport:
		return true, h.dispatchImportJobs(c, appID, "")
	case strings.HasPrefix(subPath, subPathJobsImport+"/"):
		return true, h.handleGetImportJob(c, appID, strings.TrimPrefix(subPath, "jobs/import/"))
	case subPath == "eventstream":
		return true, h.dispatchEventStream(c, appID)
	}

	return false, nil
}

// tryAppExtSubPath handles messaging, KPI, channel, endpoint, and user paths.
// Returns (true, err) if the path was handled.
func (h *Handler) tryAppExtSubPath(c *echo.Context, appID, subPath string) (bool, error) {
	switch {
	case subPath == "messages":
		return true, h.handleSendMessages(c, appID)
	case subPath == "users-messages":
		return true, h.handleSendUsersMessages(c, appID)
	case subPath == "otp":
		return true, h.handleSendOTPMessage(c, appID)
	case subPath == "verify-otp":
		return true, h.handleVerifyOTPMessage(c, appID)
	case subPath == "events":
		return true, h.handlePutEvents(c, appID)
	case strings.HasPrefix(subPath, "kpis/daterange/"):
		return true, h.handleGetApplicationDateRangeKpi(c, appID, strings.TrimPrefix(subPath, "kpis/daterange/"))
	case strings.HasPrefix(subPath, "channels/"):
		return true, h.dispatchChannelByType(c, appID, strings.TrimPrefix(subPath, "channels/"))
	case subPath == "channels":
		return true, h.handleGetChannels(c, appID)
	case strings.HasPrefix(subPath, "endpoints/"):
		return true, h.dispatchEndpointByID(c, appID, strings.TrimPrefix(subPath, "endpoints/"))
	case subPath == "endpoints":
		if c.Request().Method == http.MethodPut {
			return true, h.handleUpdateEndpointsBatch(c, appID)
		}
	case strings.HasPrefix(subPath, "users/"):
		return true, h.dispatchUserByID(c, appID, strings.TrimPrefix(subPath, "users/"))
	case strings.HasPrefix(subPath, "attributes/"):
		attrType := strings.TrimPrefix(subPath, "attributes/")
		if c.Request().Method == http.MethodPut {
			return true, h.handleRemoveAttributes(c, appID, attrType)
		}
	}

	return false, nil
}

// writeErrorResponse writes a JSON error response in the Pinpoint REST API format.
func writeErrorResponse(c *echo.Context, statusCode int, errorType, message string) error {
	httputils.WriteJSON(c.Request().Context(), c.Response(), statusCode, map[string]string{
		"message": message,
		"__type":  errorType,
	})

	return nil
}

// errNameRequired is returned when a required Name field is missing.
var errNameRequired = errors.New("Name is required")

// namedResourceCreatorFn creates a named resource and returns the JSON-serialisable response or an error.
type namedResourceCreatorFn func(body []byte, region, appID string) (any, error)

// handleCreateNamedAppResource is a shared handler for app-scoped named resource creation.
func (h *Handler) handleCreateNamedAppResource(c *echo.Context, appID string, creator namedResourceCreatorFn) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)

	resp, creationErr := creator(body, region, appID)
	if creationErr != nil {
		switch {
		case errors.Is(creationErr, errInvalidRequestBody):
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
		case errors.Is(creationErr, errNameRequired):
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "Name is required")
		case errors.Is(creationErr, awserr.ErrNotFound):
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", creationErr.Error())
		default:
			return writeErrorResponse(
				c,
				http.StatusInternalServerError,
				"InternalServerErrorException",
				creationErr.Error(),
			)
		}
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusCreated, resp)

	return nil
}

// parsePageParams parses page-size and token query params.
// Returns offset (0-based index) and page size.
func parsePageParams(c *echo.Context) (int, int) {
	pageSize := defaultPageSize
	offset := 0

	if ps := c.QueryParam("page-size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 {
			pageSize = n
		}
	}

	if tok := c.QueryParam("token"); tok != "" {
		if raw, err := base64.StdEncoding.DecodeString(tok); err == nil {
			if n, atoiErr := strconv.Atoi(string(raw)); atoiErr == nil && n >= 0 {
				offset = n
			}
		}
	}

	return offset, pageSize
}

// applyPageParams slices items to the requested page and returns the NextToken if more items remain.
func applyPageParams(offset, pageSize, total int) (int, int, *string) {
	if offset >= total {
		offset = total
	}

	end := offset + pageSize

	var nextToken *string

	if end < total {
		nextToken = makeNextToken(end)
	} else {
		end = total
	}

	return offset, end, nextToken
}

// makeNextToken encodes an offset into a pagination token.
func makeNextToken(offset int) *string {
	tok := base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))

	return &tok
}

// parseKPIDateRange parses the start-time/end-time query params shared by
// GetApplicationDateRangeKpi, GetCampaignDateRangeKpi, and
// GetJourneyDateRangeKpi (confirmed at pinpoint@v1.42.4 serializers.go's
// awsRestjson1_serializeOpHttpBindingsGetApplicationDateRangeKpiInput,
// which sets "start-time"/"end-time" as optional query params). The real
// *DateRangeKpiResponse types mark StartTime/EndTime as required members
// regardless, so this always returns a value: the request-supplied range
// when parseable, else a trailing kpiDefaultRangeDays window ending now.
func parseKPIDateRange(c *echo.Context) (string, string) {
	now := time.Now().UTC()
	startT := now.AddDate(0, 0, -kpiDefaultRangeDays)
	endT := now

	if v := c.QueryParam("start-time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			startT = t
		}
	}

	if v := c.QueryParam("end-time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			endT = t
		}
	}

	return startT.UTC().Format(time.RFC3339), endT.UTC().Format(time.RFC3339)
}

// ──────────────────────────────────────────────────
// Channel handlers
// ──────────────────────────────────────────────────

// unmarshalBody reads the HTTP request body and unmarshals it into dst.
// If reading or parsing fails, it writes an error response to c and returns false.
func unmarshalBody(c *echo.Context, dst any) bool {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		_ = writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")

		return false
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return false
	}

	if jsonErr := json.Unmarshal(body, dst); jsonErr != nil {
		_ = writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")

		return false
	}

	return true
}

func writeNotFoundOrInternal(c *echo.Context, err error) error {
	if errors.Is(err, awserr.ErrNotFound) {
		return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
	}

	return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
}

// ──────────────────────────────────────────────────
// Conversion helpers
// ──────────────────────────────────────────────────

// parseVersionParam parses a version integer from a path segment.
func parseVersionParam(s string) (int, error) {
	return strconv.Atoi(s)
}
