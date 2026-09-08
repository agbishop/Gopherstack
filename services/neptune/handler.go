package neptune

import (
	"context"
	"encoding/xml"
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

const (
	engineDescriptionAmazonNeptune = "Amazon Neptune"
	pgFamilyNeptune12              = "neptune1.2"
	pgFamilyNeptune13              = "neptune1.3"
	sourceTypeNotification         = "notification"
	formTrue                       = "true"
	unknownOp                      = "Unknown"
)

const (
	neptuneVersion = "2014-10-31"
	neptuneXMLNS   = "http://rds.amazonaws.com/doc/2014-10-31/"
)

// Handler is the Echo HTTP handler for Neptune operations.
type Handler struct {
	Backend StorageBackend
}

// NewHandler creates a new Neptune handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "Neptune" }

// Reset clears all backend state.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// regionFromRequest resolves the AWS region for a request from its SigV4
// credential scope, falling back to the backend's default region.
func (h *Handler) regionFromRequest(c *echo.Context) string {
	return httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
}

// GetSupportedOperations returns supported Neptune operations (sorted).
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AddRoleToDBCluster",
		"AddSourceIdentifierToSubscription",
		"AddTagsToResource",
		"ApplyPendingMaintenanceAction",
		"CopyDBClusterParameterGroup",
		"CopyDBClusterSnapshot",
		"CopyDBParameterGroup",
		"CreateDBCluster",
		"CreateDBClusterEndpoint",
		"CreateDBClusterParameterGroup",
		"CreateDBClusterSnapshot",
		"CreateDBInstance",
		"CreateDBParameterGroup",
		"CreateDBSubnetGroup",
		"CreateEventSubscription",
		"CreateGlobalCluster",
		"DeleteDBCluster",
		"DeleteDBClusterEndpoint",
		"DeleteDBClusterParameterGroup",
		"DeleteDBClusterSnapshot",
		"DeleteDBInstance",
		"DeleteDBParameterGroup",
		"DeleteDBSubnetGroup",
		"DeleteEventSubscription",
		"DeleteGlobalCluster",
		"DescribeDBClusterEndpoints",
		"DescribeDBClusterParameterGroups",
		"DescribeDBClusterParameters",
		"DescribeDBClusterSnapshotAttributes",
		"DescribeDBClusterSnapshots",
		"DescribeDBClusters",
		"DescribeDBEngineVersions",
		"DescribeDBInstances",
		"DescribeDBParameterGroups",
		"DescribeDBParameters",
		"DescribeDBSubnetGroups",
		"DescribeEngineDefaultClusterParameters",
		"DescribeEngineDefaultParameters",
		"DescribeEventCategories",
		"DescribeEventSubscriptions",
		"DescribeEvents",
		"DescribeGlobalClusters",
		"DescribeOrderableDBInstanceOptions",
		"DescribePendingMaintenanceActions",
		"DescribeValidDBInstanceModifications",
		"FailoverDBCluster",
		"FailoverGlobalCluster",
		"ListTagsForResource",
		"ModifyDBCluster",
		"ModifyDBClusterEndpoint",
		"ModifyDBClusterParameterGroup",
		"ModifyDBClusterSnapshotAttribute",
		"ModifyDBInstance",
		"ModifyDBParameterGroup",
		"ModifyDBSubnetGroup",
		"ModifyEventSubscription",
		"ModifyGlobalCluster",
		"PromoteReadReplicaDBCluster",
		"RebootDBInstance",
		"RemoveFromGlobalCluster",
		"RemoveRoleFromDBCluster",
		"RemoveSourceIdentifierFromSubscription",
		"RemoveTagsFromResource",
		"ResetDBClusterParameterGroup",
		"ResetDBParameterGroup",
		"RestoreDBClusterFromSnapshot",
		"RestoreDBClusterToPointInTime",
		"StartDBCluster",
		"StopDBCluster",
		"SwitchoverGlobalCluster",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "neptune" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Neptune instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches Neptune requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost {
			return false
		}
		if strings.HasPrefix(r.URL.Path, "/dashboard/") {
			return false
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/x-www-form-urlencoded") {
			return false
		}
		// Checks both User-Agent (native SDKs) and X-Amz-User-Agent (the AWS
		// SDK for JavaScript in a browser, which cannot set User-Agent
		// itself) -- see service.MatchesUserAgentMarker.
		if !service.MatchesUserAgentMarker(r.Header, "api/neptune") {
			return false
		}
		body, err := httputils.ReadBody(r)
		if err != nil {
			return true
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == neptuneVersion
	}
}

// MatchPriority returns the routing priority for Neptune (higher than RDS to intercept Neptune requests first).
func (h *Handler) MatchPriority() int { return service.PriorityFormNeptune }

// ExtractOperation extracts the Neptune action from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return unknownOp
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return unknownOp
	}
	action := vals.Get("Action")
	if action == "" {
		return unknownOp
	}

	return action
}

// ExtractResource returns the DB cluster identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}

	return vals.Get("DBClusterIdentifier")
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		body, err := httputils.ReadBody(r)
		if err != nil {
			return h.writeError(
				c,
				http.StatusInternalServerError,
				"InternalFailure",
				"failed to read request body",
			)
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return h.writeError(
				c,
				http.StatusInternalServerError,
				"InternalFailure",
				"failed to parse request body",
			)
		}
		action := vals.Get("Action")
		if action == "" {
			return h.writeError(
				c,
				http.StatusBadRequest,
				"MissingAction",
				"missing Action parameter",
			)
		}
		// Attach the SigV4-derived region so backend ops route to the correct region store.
		ctx := context.WithValue(r.Context(), regionContextKey{}, h.regionFromRequest(c))
		resp, opErr := h.dispatch(ctx, action, vals)
		if opErr != nil {
			return h.handleOpError(c, action, opErr)
		}
		xmlBytes, err := marshalXML(resp)
		if err != nil {
			return h.writeError(
				c,
				http.StatusInternalServerError,
				"InternalFailure",
				"internal server error",
			)
		}

		return c.Blob(http.StatusOK, "text/xml", xmlBytes)
	}
}

func (h *Handler) handleOpError(c *echo.Context, action string, opErr error) error {
	statusCode := http.StatusBadRequest
	code := neptuneErrorCode(opErr)
	if code == "" {
		code = "InternalFailure"
		statusCode = http.StatusInternalServerError
		logger.Load(c.Request().Context()).
			Error("Neptune internal error", "error", opErr, "action", action)
	}

	return h.writeError(c, statusCode, code, opErr.Error())
}

func neptuneErrorCode(opErr error) string {
	type errorMapping struct {
		sentinel error
		code     string
	}
	// Error code strings below are verified against each fault type's
	// ErrorCode() method in aws-sdk-go-v2/service/neptune/types/errors.go (and
	// cross-checked against the per-operation error-deserializer switch
	// statements in deserializers.go). Neptune's generated API model is
	// inconsistent about the "Fault" suffix -- e.g. DBClusterNotFoundFault
	// keeps it ("DBClusterNotFoundFault") but DBInstanceNotFoundFault drops it
	// ("DBInstanceNotFound") -- so do not assume every *Fault sentinel maps to
	// a same-named code with "Fault" appended; a wrong string here means the
	// SDK client can't type-match the fault (errors.As silently fails and the
	// caller sees a generic *smithy.GenericAPIError instead of the typed
	// fault) even though the HTTP status/message still look correct.
	//
	// The cluster-parameter-group family is the most surprising case: Neptune
	// has no distinct "DBClusterParameterGroupAlreadyExists"/"...NotFound"
	// fault for its own CRUD ops -- CreateDBClusterParameterGroup,
	// DeleteDBClusterParameterGroup, ModifyDBClusterParameterGroup, and
	// ResetDBClusterParameterGroup all reuse the plain (non-cluster)
	// "DBParameterGroupAlreadyExists"/"DBParameterGroupNotFound" codes. (A
	// "DBClusterParameterGroupNotFound" fault does exist in the model, but
	// only for other ops referencing a cluster's parameter group by name --
	// e.g. CreateDBCluster/ModifyDBCluster/RestoreDBCluster* -- which this
	// backend does not validate, so it never needs that code.)
	mappings := []errorMapping{
		{ErrClusterNotFound, "DBClusterNotFoundFault"},
		{ErrClusterAlreadyExists, "DBClusterAlreadyExistsFault"},
		{ErrInstanceNotFound, "DBInstanceNotFound"},
		{ErrInstanceAlreadyExists, "DBInstanceAlreadyExists"},
		{ErrSubnetGroupNotFound, "DBSubnetGroupNotFoundFault"},
		{ErrSubnetGroupAlreadyExists, "DBSubnetGroupAlreadyExists"},
		{ErrSubnetGroupInUse, "InvalidDBSubnetGroupStateFault"},
		{ErrClusterParameterGroupNotFound, "DBParameterGroupNotFound"},
		{ErrClusterParameterGroupAlreadyExists, "DBParameterGroupAlreadyExists"},
		{ErrClusterSnapshotNotFound, "DBClusterSnapshotNotFoundFault"},
		{ErrClusterSnapshotAlreadyExists, "DBClusterSnapshotAlreadyExistsFault"},
		{ErrParameterGroupNotFound, "DBParameterGroupNotFound"},
		{ErrParameterGroupAlreadyExists, "DBParameterGroupAlreadyExists"},
		{ErrParameterGroupInUse, "InvalidDBParameterGroupState"},
		{ErrClusterEndpointNotFound, "DBClusterEndpointNotFoundFault"},
		{ErrClusterEndpointAlreadyExists, "DBClusterEndpointAlreadyExistsFault"},
		{ErrSubscriptionNotFound, "SubscriptionNotFound"},
		{ErrSubscriptionAlreadyExists, "SubscriptionAlreadyExist"},
		{ErrGlobalClusterNotFound, "GlobalClusterNotFoundFault"},
		{ErrGlobalClusterAlreadyExists, "GlobalClusterAlreadyExistsFault"},
		{ErrInvalidParameter, "InvalidParameterValue"},
		{ErrUnknownAction, "InvalidAction"},
		{ErrInvalidDBClusterStateFault, "InvalidDBClusterStateFault"},
		{ErrInvalidDBInstanceStateFault, "InvalidDBInstanceState"},
		{ErrInvalidDBClusterSnapshotStateFault, "InvalidDBClusterSnapshotStateFault"},
		{ErrSnapshotRequired, "InvalidParameterCombination"},
		{ErrInvalidGlobalClusterState, "InvalidGlobalClusterStateFault"},
	}
	for _, m := range mappings {
		if errors.Is(opErr, m.sentinel) {
			return m.code
		}
	}

	return ""
}

func (h *Handler) writeError(c *echo.Context, statusCode int, code, message string) error {
	errResp := &neptuneErrorResponse{
		Xmlns: neptuneXMLNS,
		Error: neptuneError{Code: code, Message: message, Type: "Sender"},
	}
	xmlBytes, err := marshalXML(errResp)
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	return c.Blob(statusCode, "text/xml", xmlBytes)
}

func marshalXML(v any) ([]byte, error) {
	raw, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), raw...), nil
}

// parseMemberList parses a form-encoded list with keys of the form "<prefix>.<N>".
func parseMemberList(vals url.Values, prefix string) []string {
	var result []string
	for i := 1; ; i++ {
		v := vals.Get(fmt.Sprintf("%s.%d", prefix, i))
		if v == "" {
			return result
		}
		result = append(result, v)
	}
}

// parseNeptuneFilterValues scans AWS form-encoded Filters.Filter.N.Name/
// Values.Value.M and returns every value for the named filter, or nil. The
// real serializer (awsAwsquery_serializeDocumentFilterList, neptune@v1.48.4
// serializers.go:5000-5001) wraps Filters entries in "Filter", not the
// generic "member"; each entry's Values list
// (awsAwsquery_serializeDocumentFilterValueList, serializers.go:5012-5013)
// is wrapped in "Value" and is itself a list -- reading only ".Value.1"
// silently dropped every value after the first. Both
// DescribeDBClusters/DescribeDBInstances and DescribePendingMaintenanceActions
// share this FilterList shape.
func parseNeptuneFilterValues(vals url.Values, filterName string) []string {
	for i := 1; ; i++ {
		name := vals.Get(fmt.Sprintf("Filters.Filter.%d.Name", i))
		if name == "" {
			return nil
		}
		if name == filterName {
			return parseMemberList(vals, fmt.Sprintf("Filters.Filter.%d.Values.Value", i))
		}
	}
}

type neptuneError struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
	Type    string `xml:"Type"`
}

type neptuneErrorResponse struct {
	XMLName xml.Name     `xml:"ErrorResponse"`
	Xmlns   string       `xml:"xmlns,attr"`
	Error   neptuneError `xml:"Error"`
}

const defaultNeptuneMaxRecords = 100

// applyNeptuneMarker applies Marker/MaxRecords-based pagination to a slice.
func applyNeptuneMarker[T any](items []T, marker, maxRecordsStr string) ([]T, string) {
	start := 0
	if marker != "" {
		idx, err := strconv.Atoi(marker)
		if err == nil && idx > 0 {
			start = idx
		}
	}

	if start >= len(items) {
		return []T{}, ""
	}

	items = items[start:]

	limit := defaultNeptuneMaxRecords
	if maxRecordsStr != "" {
		if n, err := strconv.Atoi(maxRecordsStr); err == nil && n > 0 {
			limit = n
		}
	}

	if len(items) <= limit {
		return items, ""
	}

	return items[:limit], strconv.Itoa(start + limit)
}

// dispatch routes a Neptune Action to its handler. To keep each switch's
// cyclomatic complexity within lint limits, cases are split by resource
// family across dispatchDBClusterAction/dispatchDBInstanceAction/etc (each
// defined alongside its family's handlers in handler_<family>.go), chained
// via the default case until one recognizes the action.
func (h *Handler) dispatch(ctx context.Context, action string, vals url.Values) (any, error) {
	return h.dispatchDBClusterAction(ctx, action, vals)
}
