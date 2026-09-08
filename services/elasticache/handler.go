package elasticache

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	elasticacheVersion = "2015-02-02"
	elasticacheNS      = "http://elasticache.amazonaws.com/doc/2015-02-02/"
	unknownOp          = "Unknown"
)

// errInvalidMaxRecords is the sentinel parsePagination wraps when MaxRecords
// falls outside AWS's modeled [20,100] range.
var errInvalidMaxRecords = errors.New("invalid MaxRecords")

// errResponseWritten is returned by parsePaginationChecked and
// describeListChecked instead of xmlError's own nil-on-success-write result,
// so that the many callers doing "if err != nil { return err }" -- both the
// ~13 direct parsePaginationChecked callers and describeListChecked's own
// ~7 callers -- actually stop instead of silently falling through to a
// second, corrupting write on an already-committed response (gopherstack-8haq).
// Handler() translates it back to nil at the top of the dispatch chain, so
// telemetry/logging see the same "handled, response already sent" shape as
// every other xmlError call site.
var errResponseWritten = errors.New("elasticache: response already written")

// Handler is the Echo HTTP handler for ElastiCache operations.
type Handler struct {
	Backend   StorageBackend
	AccountID string
	Region    string
}

// NewHandler creates a new ElastiCache handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "ElastiCache" }

// GetSupportedOperations returns all supported ElastiCache operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateCacheCluster",
		"DeleteCacheCluster",
		"DescribeCacheClusters",
		"ModifyCacheCluster",
		"ListTagsForResource",
		"AddTagsToResource",
		"RemoveTagsFromResource",
		"CreateReplicationGroup",
		"DeleteReplicationGroup",
		"DescribeReplicationGroups",
		"ModifyReplicationGroup",
		"TestFailover",
		"CreateCacheParameterGroup",
		"DeleteCacheParameterGroup",
		"DescribeCacheParameterGroups",
		"ModifyCacheParameterGroup",
		"ResetCacheParameterGroup",
		"DescribeCacheParameters",
		"CreateCacheSubnetGroup",
		"DeleteCacheSubnetGroup",
		"DescribeCacheSubnetGroups",
		"ModifyCacheSubnetGroup",
		"CreateSnapshot",
		"DeleteSnapshot",
		"DescribeSnapshots",
		"CopySnapshot",
		"DescribeEvents",
		// New ops
		"CreateCacheSecurityGroup",
		"AuthorizeCacheSecurityGroupIngress",
		"CreateGlobalReplicationGroup",
		"CreateServerlessCache",
		"CreateServerlessCacheSnapshot",
		"CopyServerlessCacheSnapshot",
		"CreateUser",
		"BatchApplyUpdateAction",
		"BatchStopUpdateAction",
		"CompleteMigration",
		// Ops2
		"DeleteUser",
		"DescribeUsers",
		"ModifyUser",
		"CreateUserGroup",
		"DeleteUserGroup",
		"DescribeUserGroups",
		"ModifyUserGroup",
		"DeleteGlobalReplicationGroup",
		"DescribeGlobalReplicationGroups",
		"DisassociateGlobalReplicationGroup",
		"FailoverGlobalReplicationGroup",
		"IncreaseNodeGroupsInGlobalReplicationGroup",
		"DecreaseNodeGroupsInGlobalReplicationGroup",
		"ModifyGlobalReplicationGroup",
		"RebalanceSlotsInGlobalReplicationGroup",
		"DescribeReservedCacheNodes",
		"DescribeReservedCacheNodesOfferings",
		"PurchaseReservedCacheNodesOffering",
		"DeleteServerlessCache",
		"DeleteServerlessCacheSnapshot",
		"DescribeServerlessCaches",
		"DescribeServerlessCacheSnapshots",
		"ExportServerlessCacheSnapshot",
		"ModifyServerlessCache",
		"StartMigration",
		"TestMigration",
		"IncreaseReplicaCount",
		"DecreaseReplicaCount",
		"ModifyReplicationGroupShardConfiguration",
		"DescribeCacheEngineVersions",
		"RebootCacheCluster",
		"DeleteCacheSecurityGroup",
		"DescribeCacheSecurityGroups",
		"RevokeCacheSecurityGroupIngress",
		"DescribeEngineDefaultParameters",
		"DescribeServiceUpdates",
		"DescribeUpdateActions",
		"ListAllowedNodeTypeModifications",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "elasticache" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this ElastiCache instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// RouteMatcher returns a matcher for ElastiCache query-protocol requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost {
			return false
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/x-www-form-urlencoded") {
			return false
		}
		body, err := httputils.ReadBody(r)
		if err != nil {
			// Body unreadable (e.g. oversized): fall back to the User-Agent
			// marker every aws-sdk-go-v2 elasticache client sets
			// (api_client.go's AddSDKAgentKeyValue -- "api/elasticache").
			// That still identifies this as ours, so claim it and let
			// Handler() produce the typed error instead of masking the
			// read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/elasticache")
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == elasticacheVersion &&
			slices.Contains(h.GetSupportedOperations(), vals.Get("Action"))
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityPathSubdomain }

// ExtractOperation extracts the Action from the form body.
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

// ExtractResource extracts the primary resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}
	for _, key := range []string{
		"CacheClusterId",
		"ReplicationGroupId",
		"CacheParameterGroupName",
		"CacheSubnetGroupName",
		"SnapshotName",
		"ResourceName",
		"CacheSecurityGroupName",
		"GlobalReplicationGroupIdSuffix",
		"ServerlessCacheName",
		"ServerlessCacheSnapshotName",
		"UserId",
	} {
		if v := vals.Get(key); v != "" {
			return v
		}
	}

	return ""
}

type elasticacheActionFn func(ctx context.Context, c *echo.Context, form url.Values) error

func (h *Handler) regionFromRequest(c *echo.Context) string {
	return httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
}

func (h *Handler) dispatchTable() map[string]elasticacheActionFn {
	return map[string]elasticacheActionFn{
		"CreateCacheCluster":           h.createCacheCluster,
		"DeleteCacheCluster":           h.deleteCacheCluster,
		"DescribeCacheClusters":        h.describeCacheClusters,
		"ModifyCacheCluster":           h.modifyCacheCluster,
		"ListTagsForResource":          h.listTagsForResource,
		"AddTagsToResource":            h.addTagsToResource,
		"RemoveTagsFromResource":       h.removeTagsFromResource,
		"CreateReplicationGroup":       h.createReplicationGroup,
		"DeleteReplicationGroup":       h.deleteReplicationGroup,
		"DescribeReplicationGroups":    h.describeReplicationGroups,
		"ModifyReplicationGroup":       h.modifyReplicationGroup,
		"TestFailover":                 h.testFailoverReplicationGroup,
		"CreateCacheParameterGroup":    h.createCacheParameterGroup,
		"DeleteCacheParameterGroup":    h.deleteCacheParameterGroup,
		"DescribeCacheParameterGroups": h.describeCacheParameterGroups,
		"ModifyCacheParameterGroup":    h.modifyCacheParameterGroup,
		"ResetCacheParameterGroup":     h.resetCacheParameterGroup,
		"DescribeCacheParameters":      h.describeCacheParameters,
		"CreateCacheSubnetGroup":       h.createCacheSubnetGroup,
		"DeleteCacheSubnetGroup":       h.deleteCacheSubnetGroup,
		"DescribeCacheSubnetGroups":    h.describeCacheSubnetGroups,
		"ModifyCacheSubnetGroup":       h.modifyCacheSubnetGroup,
		"CreateSnapshot":               h.createSnapshot,
		"DeleteSnapshot":               h.deleteSnapshot,
		"DescribeSnapshots":            h.describeSnapshots,
		"CopySnapshot":                 h.copySnapshot,
		"DescribeEvents":               h.describeEvents,
		// New ops
		"CreateCacheSecurityGroup":           h.createCacheSecurityGroup,
		"AuthorizeCacheSecurityGroupIngress": h.authorizeCacheSecurityGroupIngress,
		"CreateGlobalReplicationGroup":       h.createGlobalReplicationGroup,
		"CreateServerlessCache":              h.createServerlessCache,
		"CreateServerlessCacheSnapshot":      h.createServerlessCacheSnapshot,
		"CopyServerlessCacheSnapshot":        h.copyServerlessCacheSnapshot,
		"CreateUser":                         h.createUser,
		"BatchApplyUpdateAction":             h.batchApplyUpdateAction,
		"BatchStopUpdateAction":              h.batchStopUpdateAction,
		"CompleteMigration":                  h.completeMigration,
		// Ops2
		"DeleteUser":                                 h.deleteUser,
		"DescribeUsers":                              h.describeUsers,
		"ModifyUser":                                 h.modifyUser,
		"CreateUserGroup":                            h.createUserGroup,
		"DeleteUserGroup":                            h.deleteUserGroup,
		"DescribeUserGroups":                         h.describeUserGroups,
		"ModifyUserGroup":                            h.modifyUserGroup,
		"DeleteGlobalReplicationGroup":               h.deleteGlobalReplicationGroup,
		"DescribeGlobalReplicationGroups":            h.describeGlobalReplicationGroups,
		"DisassociateGlobalReplicationGroup":         h.disassociateGlobalReplicationGroup,
		"FailoverGlobalReplicationGroup":             h.failoverGlobalReplicationGroup,
		"IncreaseNodeGroupsInGlobalReplicationGroup": h.increaseNodeGroupsInGlobalReplicationGroup,
		"DecreaseNodeGroupsInGlobalReplicationGroup": h.decreaseNodeGroupsInGlobalReplicationGroup,
		"ModifyGlobalReplicationGroup":               h.modifyGlobalReplicationGroup,
		"RebalanceSlotsInGlobalReplicationGroup":     h.rebalanceSlotsInGlobalReplicationGroup,
		"DescribeReservedCacheNodes":                 h.describeReservedCacheNodes,
		"DescribeReservedCacheNodesOfferings":        h.describeReservedCacheNodesOfferings,
		"PurchaseReservedCacheNodesOffering":         h.purchaseReservedCacheNodesOffering,
		"DeleteServerlessCache":                      h.deleteServerlessCache,
		"DeleteServerlessCacheSnapshot":              h.deleteServerlessCacheSnapshot,
		"DescribeServerlessCaches":                   h.describeServerlessCaches,
		"DescribeServerlessCacheSnapshots":           h.describeServerlessCacheSnapshots,
		"ExportServerlessCacheSnapshot":              h.exportServerlessCacheSnapshot,
		"ModifyServerlessCache":                      h.modifyServerlessCache,
		"StartMigration":                             h.startMigration,
		"TestMigration":                              h.testMigration,
		"IncreaseReplicaCount":                       h.increaseReplicaCount,
		"DecreaseReplicaCount":                       h.decreaseReplicaCount,
		"ModifyReplicationGroupShardConfiguration":   h.modifyReplicationGroupShardConfiguration,
		"DescribeCacheEngineVersions":                h.describeCacheEngineVersions,
		"RebootCacheCluster":                         h.rebootCacheCluster,
		"DeleteCacheSecurityGroup":                   h.deleteCacheSecurityGroup,
		"DescribeCacheSecurityGroups":                h.describeCacheSecurityGroups,
		"RevokeCacheSecurityGroupIngress":            h.revokeCacheSecurityGroupIngress,
		"DescribeEngineDefaultParameters":            h.describeEngineDefaultParameters,
		"DescribeServiceUpdates":                     h.describeServiceUpdates,
		"DescribeUpdateActions":                      h.describeUpdateActions,
		"ListAllowedNodeTypeModifications":           h.listAllowedNodeTypeModifications,
	}
}

// Handler returns the Echo handler function for ElastiCache requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			return xmlError(c, http.StatusInternalServerError, "InternalFailure", "cannot read body")
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "cannot parse form")
		}
		action := vals.Get("Action")
		fn, ok := h.dispatchTable()[action]
		if !ok {
			return xmlError(c, http.StatusBadRequest, "InvalidAction", "unknown action: "+action)
		}

		region := h.regionFromRequest(c)
		ctx := context.WithValue(c.Request().Context(), regionContextKey{}, region)

		fnErr := fn(ctx, c, vals)
		if fnErr == nil {
			return nil
		}

		if errors.Is(fnErr, errResponseWritten) {
			return nil
		}

		return fnErr
	}
}

// parseFormTags extracts Tags.Tag.N.Key/Value pairs from a form.
func parseFormTags(form url.Values) map[string]string {
	tags := make(map[string]string)
	for i := 1; ; i++ {
		key := form.Get(fmt.Sprintf("Tags.Tag.%d.Key", i))
		if key == "" {
			break
		}
		val := form.Get(fmt.Sprintf("Tags.Tag.%d.Value", i))
		tags[key] = val
	}

	return tags
}

// minMaxRecords and maxMaxRecords bound the MaxRecords parameter accepted by
// every paginated ElastiCache Describe*/List* operation. AWS rejects values
// outside [20,100] with InvalidParameterValueException (wire code
// InvalidParameterValue, HTTP 400) rather than silently clamping them.
const (
	minMaxRecords = 20
	maxMaxRecords = 100
)

// parsePagination extracts Marker and MaxRecords from query form values. When
// MaxRecords is present but non-numeric or outside AWS's modeled [20,100]
// range, it returns a non-nil error the caller should surface as
// InvalidParameterValue. A missing MaxRecords falls back to the operation's
// default (0, resolved downstream by pkgs/page).
func parsePagination(form url.Values) (string, int, error) {
	marker := form.Get("Marker")

	s := form.Get("MaxRecords")
	if s == "" {
		return marker, 0, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return "", 0, fmt.Errorf("%w: MaxRecords must be an integer, got %q", errInvalidMaxRecords, s)
	}

	if n < minMaxRecords || n > maxMaxRecords {
		return "", 0, fmt.Errorf(
			"%w: MaxRecords must be between %d and %d, got %d",
			errInvalidMaxRecords, minMaxRecords, maxMaxRecords, n,
		)
	}

	return marker, n, nil
}

// parsePaginationChecked parses Marker/MaxRecords and, on failure, writes the
// InvalidParameterValue error response to c and returns it as err so the
// caller can `return err` and stop -- centralizes the boilerplate every
// paginated Describe*/List* handler needs (avoids ~15 near-identical
// call-and-check blocks, which trips the dupl linter).
func parsePaginationChecked(c *echo.Context, form url.Values) (string, int, error) {
	marker, maxRecords, err := parsePagination(form)
	if err != nil {
		_ = xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())

		return "", 0, errResponseWritten
	}

	return marker, maxRecords, nil
}

// describeListChecked runs the sequence shared by every paginated
// Describe*/List* handler: validate Marker/MaxRecords, invoke the backend
// call, and split its error into NotFound-vs-InternalFailure. Centralizing
// this dedups what would otherwise be ~15 near-identical handler bodies
// (avoiding a wall of //nolint:dupl -- each handler differs only in its
// backend call and the XML envelope it builds from the result, both of
// which stay in the caller).
func describeListChecked[T any](
	c *echo.Context, form url.Values,
	call func(marker string, maxRecords int) (page.Page[T], error),
	notFound error, notFoundStatus int, notFoundCode, notFoundMsg string,
) (page.Page[T], error) {
	marker, maxRecords, err := parsePaginationChecked(c, form)
	if err != nil {
		return page.Page[T]{}, err
	}

	p, err := call(marker, maxRecords)
	if err != nil {
		if errors.Is(err, notFound) {
			_ = xmlError(c, notFoundStatus, notFoundCode, notFoundMsg)

			return page.Page[T]{}, errResponseWritten
		}

		_ = xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())

		return page.Page[T]{}, errResponseWritten
	}

	return p, nil
}

// parseRepeatedField extracts a list of values from form fields with numeric suffixes.
// e.g., "ReplicationGroupIds.member.1", "ReplicationGroupIds.member.2", etc.
func parseRepeatedField(form url.Values, prefix string) []string {
	var items []string
	for i := 1; ; i++ {
		v := form.Get(fmt.Sprintf("%s.%d", prefix, i))
		if v == "" {
			break
		}
		items = append(items, v)
	}

	return items
}

// parseUserIDFilters extracts Values from DescribeUsersInput.Filters entries
// named "UserId" -- the only documented Filters[].Name (elasticache@v1.56.4
// api_op_DescribeUsers.go: "The property being filtered. For example,
// UserId.").
func parseUserIDFilters(form url.Values) []string {
	var ids []string
	for i := 1; ; i++ {
		name := form.Get(fmt.Sprintf("Filters.member.%d.Name", i))
		if name == "" {
			break
		}
		if name == "UserId" {
			ids = append(ids, parseRepeatedField(form, fmt.Sprintf("Filters.member.%d.Values.member", i))...)
		}
	}

	return ids
}

// Reset clears all backend state.
func (h *Handler) Reset() {
	type resetter interface{ Reset() }
	if r, ok := h.Backend.(resetter); ok {
		r.Reset()
	}
}

func xmlResp(c *echo.Context, status int, v any) error {
	data, err := xml.Marshal(v)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	c.Response().Header().Set("Content-Type", "text/xml; charset=utf-8")
	c.Response().WriteHeader(status)
	_, _ = c.Response().Write([]byte(xml.Header))
	_, _ = c.Response().Write(data)

	return nil
}

// xmlErrorDetail holds the fault type, code, and message for an ElastiCache XML
// error, matching the AWS query-protocol error envelope.
type xmlErrorDetail struct {
	Type    string `xml:"Type"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

type xmlErrorResp struct {
	XMLName   xml.Name       `xml:"ErrorResponse"`
	Xmlns     string         `xml:"xmlns,attr"`
	Error     xmlErrorDetail `xml:"Error"`
	RequestID string         `xml:"RequestId"`
}

// faultType classifies an HTTP status into the AWS query-protocol fault Type.
// Client-side faults (4xx: validation, not-found, conflict) are "Sender";
// server-side faults (5xx) are "Receiver".
func faultType(status int) string {
	if status >= http.StatusInternalServerError {
		return "Receiver"
	}

	return "Sender"
}

// newRequestID returns a fresh correlation ID for a response, mirroring the
// per-request x-amzn-RequestId AWS attaches to every call.
func newRequestID() string {
	return uuid.NewString()
}

func xmlError(c *echo.Context, status int, code, message string) error {
	resp := xmlErrorResp{
		Xmlns:     elasticacheNS,
		RequestID: newRequestID(),
	}
	resp.Error.Type = faultType(status)
	resp.Error.Code = code
	resp.Error.Message = message

	return xmlResp(c, status, resp)
}
