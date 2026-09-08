package mgn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const mgnServiceName = "mgn"

var errUnknownPath = errors.New("unknown path")

// Handler is the HTTP handler for the AWS Application Migration Service API.
type Handler struct {
	Backend   *InMemoryBackend
	AccountID string
	Region    string
}

// NewHandler creates a new MGN handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{
		Backend:   backend,
		AccountID: backend.accountID,
		Region:    backend.region,
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "MGN" }

// GetSupportedOperations returns every operation this handler routes -- all
// 95 operations on the aws-sdk-go-v2/service/mgn client, per
// sdk_completeness_test.go's empty exception list.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"ArchiveApplication", "ArchiveWave", "AssociateApplications", "AssociateSourceServers",
		"ChangeServerLifeCycleState", "CreateApplication", "CreateConnector",
		"CreateLaunchConfigurationTemplate", "CreateNetworkMigrationDefinition",
		"CreateReplicationConfigurationTemplate", "CreateWave", "DeleteApplication", "DeleteConnector",
		"DeleteJob", "DeleteLaunchConfigurationTemplate", "DeleteNetworkMigrationDefinition",
		"DeleteReplicationConfigurationTemplate", "DeleteSourceServer", "DeleteVcenterClient", "DeleteWave",
		"DescribeJobLogItems", "DescribeJobs", "DescribeLaunchConfigurationTemplates",
		"DescribeReplicationConfigurationTemplates", "DescribeSourceServers", "DescribeVcenterClients",
		"DisassociateApplications", "DisassociateSourceServers", "DisconnectFromService", "FinalizeCutover",
		"GetLaunchConfiguration", "GetNetworkMigrationDefinition", "GetNetworkMigrationMapperSegmentConstruct",
		"GetReplicationConfiguration", "InitializeService", "ListApplications", "ListConnectors",
		"ListExportErrors", "ListExports", "ListImportErrors", "ListImportFileEnrichments", "ListImports",
		"ListManagedAccounts", "ListNetworkMigrationAnalyses", "ListNetworkMigrationAnalysisResults",
		"ListNetworkMigrationCodeGenerations", "ListNetworkMigrationCodeGenerationSegments",
		"ListNetworkMigrationDefinitions", "ListNetworkMigrationDeployedStacks",
		"ListNetworkMigrationDeployments", "ListNetworkMigrationExecutions",
		"ListNetworkMigrationMapperSegmentConstructs", "ListNetworkMigrationMapperSegments",
		"ListNetworkMigrationMappingUpdates", "ListNetworkMigrationMappings", "ListSourceServerActions",
		"ListTagsForResource", "ListTemplateActions", "ListWaves", "MarkAsArchived", "PauseReplication",
		"PutSourceServerAction", "PutTemplateAction", "RemoveSourceServerAction", "RemoveTemplateAction",
		"ResumeReplication", "RetryDataReplication", "StartCutover", "StartExport", "StartImport",
		"StartImportFileEnrichment", "StartNetworkMigrationAnalysis", "StartNetworkMigrationCodeGeneration",
		"StartNetworkMigrationDeployment", "StartNetworkMigrationMapping",
		"StartNetworkMigrationMappingUpdate", "StartReplication", "StartTest", "StopReplication",
		"TagResource", "TerminateTargetInstances", "UnarchiveApplication", "UnarchiveWave", "UntagResource",
		"UpdateApplication", "UpdateConnector", "UpdateLaunchConfiguration",
		"UpdateLaunchConfigurationTemplate", "UpdateNetworkMigrationDefinition",
		"UpdateNetworkMigrationMapperSegment", "UpdateReplicationConfiguration",
		"UpdateReplicationConfigurationTemplate", "UpdateSourceServer", "UpdateSourceServerReplicationType",
		"UpdateWave",
	}
}

// Reset clears all stored state in the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return mgnServiceName }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// RouteMatcher returns a matcher that reports true iff the request resolves
// to a known routeEntry via routeKey.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		segs := rawPathSegments(c.Request())

		// The tags trio is keyed by method + "tags" alone (see routeKey), so
		// it must not claim another service's /tags/{arn} request -- only
		// the ARN's own service segment disambiguates the true owner.
		if len(segs) > 0 && segs[0] == "tags" {
			return httputils.MatchesTaggedResourceARN(c.Request().URL.Path, mgnServiceName)
		}

		_, ok := h.dispatch(c.Request())

		return ok
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityPathVersioned }

// ExtractOperation extracts the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	entry, _ := h.dispatch(c.Request())

	return entry.op
}

// ExtractResource extracts the primary resource identifier from the
// request -- the ARN in the /tags/{resourceArn} path, or empty for every
// other (body-addressed) operation.
func (h *Handler) ExtractResource(c *echo.Context) string {
	const tagsPathSegs = 2

	segs := rawPathSegments(c.Request())
	if len(segs) >= tagsPathSegs && segs[0] == "tags" {
		return segs[1]
	}

	return ""
}

type dispatchFunc func(ctx context.Context, r *http.Request, body []byte) ([]byte, error)

type routeEntry struct {
	fn dispatchFunc
	op string
}

// routeKey builds the flat lookup key routes() is keyed by: HTTP method plus the
// operation-name path segment. 92 of 95 ops are literal POST /<OperationName>; 25
// of those are namespaced POST /network-migration/<OperationName> instead, so
// operationSegment strips that prefix first. The tags trio is keyed by method +
// "tags" regardless of the ARN segment that follows.
func routeKey(method string, segs []string) string {
	return method + " " + operationSegment(segs)
}

// operationSegment returns the path segment naming the operation: segs[0]
// normally, or segs[1] when segs[0] is the "network-migration" namespace
// prefix, or the literal "tags" for the tagging trio's ARN-suffixed path.
func operationSegment(segs []string) string {
	const networkMigrationSegs = 2

	if len(segs) == 0 {
		return ""
	}

	if segs[0] == "network-migration" && len(segs) >= networkMigrationSegs {
		return segs[1]
	}

	return segs[0]
}

// Handler returns the Echo handler function for MGN requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "mgn: failed to read request body", "error", err)

			return h.handleError(c, err)
		}

		entry, ok := h.dispatch(c.Request())
		if !ok {
			return h.handleError(c, fmt.Errorf("%w: %s %s", errUnknownPath, c.Request().Method, c.Request().URL.Path))
		}

		result, dispErr := entry.fn(ctx, c.Request(), body)
		if dispErr != nil {
			log.ErrorContext(ctx, "mgn: operation failed", "op", entry.op, "error", dispErr)

			return h.handleError(c, dispErr)
		}

		if result == nil {
			return c.JSONBlob(http.StatusOK, []byte("{}"))
		}

		return c.JSONBlob(http.StatusOK, result)
	}
}

// dispatch resolves r to its routeEntry via the flat routes() table.
func (h *Handler) dispatch(r *http.Request) (routeEntry, bool) {
	segs := rawPathSegments(r)
	if len(segs) == 0 {
		return routeEntry{}, false
	}

	entry, ok := h.routes()[routeKey(r.Method, segs)]

	return entry, ok
}

// handleError renders err as one of MGN's restjson1 error envelopes:
// {"message": "..."} with an X-Amzn-Errortype header, plus the extra
// fields each of the 8 wire exception shapes carries (see errors.go's
// apiError).
func (h *Handler) handleError(c *echo.Context, err error) error {
	status, errType := classifyMGNError(err)

	body := map[string]any{"message": err.Error()}

	if apiErr, ok := errors.AsType[*apiError](err); ok {
		addErrorFields(body, apiErr)
	}

	payload, _ := json.Marshal(body)

	c.Response().Header().Set("X-Amzn-Errortype", errType)

	return c.JSONBlob(status, payload)
}

// classifyMGNError maps err to its HTTP status and wire exception type.
func classifyMGNError(err error) (int, string) {
	switch {
	case errors.Is(err, errAccessDenied):
		return http.StatusForbidden, "AccessDeniedException"
	case errors.Is(err, errConflictSentinel):
		return http.StatusConflict, "ConflictException"
	case errors.Is(err, errInternalServer):
		return http.StatusInternalServerError, "InternalServerException"
	case errors.Is(err, errNotFoundSentinel):
		return http.StatusNotFound, "ResourceNotFoundException"
	case errors.Is(err, errQuotaExceeded):
		return http.StatusBadRequest, "ServiceQuotaExceededException"
	case errors.Is(err, errThrottling):
		return http.StatusTooManyRequests, "ThrottlingException"
	case errors.Is(err, errUninitializedAccount):
		return http.StatusBadRequest, "UninitializedAccountException"
	case errors.Is(err, errValidationSentinel):
		return http.StatusBadRequest, "ValidationException"
	case errors.Is(err, errUnknownPath):
		return http.StatusNotFound, "ResourceNotFoundException"
	default:
		return http.StatusInternalServerError, "InternalServerException"
	}
}

// addErrorFields populates body with apiErr's extra wire fields --
// ResourceId/ResourceType (Conflict/ResourceNotFound/ServiceQuotaExceeded),
// Reason (Validation), ServiceCode/QuotaCode (ServiceQuotaExceeded/
// Throttling). Only the fields the concrete exception shape defines are
// ever populated on apiErr in the first place (see errors.go's
// constructors), so this is safe to apply unconditionally.
func addErrorFields(body map[string]any, apiErr *apiError) {
	if apiErr == nil {
		return
	}

	if apiErr.resourceID != "" {
		body["resourceId"] = apiErr.resourceID
	}

	if apiErr.resourceType != "" {
		body["resourceType"] = apiErr.resourceType
	}

	if apiErr.reason != "" {
		body["reason"] = apiErr.reason
	}

	if apiErr.serviceCode != "" {
		body["serviceCode"] = apiErr.serviceCode
	}

	if apiErr.quotaCode != "" {
		body["quotaCode"] = apiErr.quotaCode
	}

	if apiErr.hasQuota {
		body["quotaValue"] = apiErr.quotaValue
	}
}

// rawPathSegments splits the raw (or decoded) URL path into non-empty
// segments, URL-decoding each segment individually so an encoded slash in
// the /tags/{resourceArn} path parameter is preserved as a single segment --
// matches services/outposts/services/resiliencehub/services/grafana's
// identically-named helper (the house fix for the ARN-in-path route-
// matching trap).
func rawPathSegments(r *http.Request) []string {
	rawPath := r.URL.RawPath
	if rawPath == "" {
		rawPath = r.URL.Path
	}

	rawPath = strings.TrimPrefix(rawPath, "/")
	parts := strings.Split(rawPath, "/")

	segments := make([]string, 0, len(parts))

	for _, p := range parts {
		if p == "" {
			continue
		}

		decoded, err := url.PathUnescape(p)
		if err != nil {
			decoded = p
		}

		segments = append(segments, decoded)
	}

	return segments
}

// queryMaxResults parses the "maxResults" query parameter -- unused by this
// service (every MaxResults-bearing op takes it in the JSON body, even for
// DescribeVcenterClients' GET -- confirmed by direct SDK read: its Input's
// MaxResults/NextToken are httpQuery-bound, not JSON body, so this DOES
// apply there) -- kept as a named helper for that one op's handler.
func queryMaxResults(q url.Values) int32 {
	raw := q.Get("maxResults")
	if raw == "" {
		return 0
	}

	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n)
}

func queryNextToken(q url.Values) string { return q.Get("nextToken") }

// decodeJSONBody unmarshals body into v, treating an empty body as a no-op
// (leaving v at its zero value) rather than a JSON error -- several
// operations in this service (e.g. InitializeService, DescribeVcenterClients)
// take no JSON body fields at all.
func decodeJSONBody(body []byte, v any) error {
	if len(body) == 0 {
		return nil
	}

	if err := json.Unmarshal(body, v); err != nil {
		return validationError("malformed request body: " + err.Error())
	}

	return nil
}

// marshalResponse marshals v, wrapping any error as an internal failure
// (marshaling our own well-typed wire structs should never fail).
func marshalResponse(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, internalServerError("mgn: marshal response: " + err.Error())
	}

	return data, nil
}
