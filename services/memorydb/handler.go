package memorydb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	memorydbService       = "memorydb"
	memorydbMatchPriority = 87
	memorydbTargetPrefix  = "AmazonMemoryDB."
)

// Handler is the HTTP handler for the AWS MemoryDB JSON 1.1 API.
type Handler struct {
	Backend       StorageBackend
	AccountID     string
	DefaultRegion string
}

// NewHandler creates a new MemoryDB handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "MemoryDB" }

// Reset clears all state by delegating to the backend.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// GetSupportedOperations returns the list of supported MemoryDB operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"BatchUpdateCluster",
		"CopySnapshot",
		// "ExportSnapshot" is deliberately NOT advertised — it is not a real
		// MemoryDB SDK operation (verified against botocore's memorydb
		// service-2.json: only CopySnapshot/CreateSnapshot/DeleteSnapshot/
		// DescribeSnapshots exist in the snapshot family; MemoryDB has no
		// export-to-S3 API at all). MemoryDB dispatches purely by X-Amz-Target
		// header value, so a real client can never send this target and the
		// route (handleExportSnapshot, a validate-and-return no-op) is
		// unreachable by real traffic either way; it stays wired as internal
		// test scaffolding only — see gopherstack-vhw2 category A, same
		// resolution as DAX's ResetParameterGroup and EMR's ListTagsForResource.
		"CreateACL",
		"CreateCluster",
		"CreateMultiRegionCluster",
		"CreateParameterGroup",
		"CreateSnapshot",
		"CreateSubnetGroup",
		"CreateUser",
		"DeleteACL",
		"DeleteCluster",
		"DeleteMultiRegionCluster",
		"DeleteParameterGroup",
		"DeleteSnapshot",
		"DeleteSubnetGroup",
		"DeleteUser",
		"DescribeACLs",
		"DescribeClusters",
		"DescribeEngineVersions",
		"DescribeEvents",
		"DescribeMultiRegionClusters",
		"DescribeMultiRegionParameterGroups",
		"DescribeMultiRegionParameters",
		"DescribeParameterGroups",
		"DescribeParameters",
		"DescribeReservedNodes",
		"DescribeReservedNodesOfferings",
		"DescribeServiceUpdates",
		"DescribeSnapshots",
		"DescribeSubnetGroups",
		"DescribeUsers",
		"FailoverShard",
		"ListAllowedMultiRegionClusterUpdates",
		"ListAllowedNodeTypeUpdates",
		"ListTags",
		"PurchaseReservedNodesOffering",
		"ResetParameterGroup",
		"TagResource",
		"UntagResource",
		"UpdateACL",
		"UpdateCluster",
		"UpdateMultiRegionCluster",
		"UpdateParameterGroup",
		"UpdateSubnetGroup",
		"UpdateUser",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return memorydbService }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches MemoryDB JSON 1.1 API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		if strings.HasPrefix(target, memorydbTargetPrefix) {
			return true
		}

		return httputils.ExtractServiceFromRequest(c.Request()) == memorydbService
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return memorydbMatchPriority }

// ExtractOperation extracts the operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	if !strings.HasPrefix(target, memorydbTargetPrefix) {
		return "Unknown"
	}

	return strings.TrimPrefix(target, memorydbTargetPrefix)
}

// ExtractResource extracts the primary resource name from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	resourceKeys := []string{
		"ClusterName", "ACLName", "SubnetGroupName",
		"UserName", "ParameterGroupName", "SnapshotName",
		"ResourceArn",
	}

	for _, key := range resourceKeys {
		if v, ok := data[key]; ok {
			if s, isStr := v.(string); isStr {
				return s
			}
		}
	}

	return ""
}

// Handler returns the Echo handler function for MemoryDB requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		target := c.Request().Header.Get("X-Amz-Target")

		if !strings.HasPrefix(target, memorydbTargetPrefix) {
			return writeError(
				c,
				http.StatusBadRequest,
				"InvalidParameterValueException",
				"missing or invalid X-Amz-Target header",
			)
		}

		op := strings.TrimPrefix(target, memorydbTargetPrefix)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "memorydb: failed to read request body", "error", err)

			return writeError(c, http.StatusInternalServerError, "InternalFailure", "failed to read request body")
		}

		log.DebugContext(ctx, "memorydb request", "op", op)

		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
		regionCtx := context.WithValue(ctx, regionContextKey{}, region)

		return h.dispatch(regionCtx, c, op, body)
	}
}

// dispatch routes to the appropriate handler based on the operation name.
func (h *Handler) dispatch(ctx context.Context, c *echo.Context, op string, body []byte) error {
	if handled, result := h.dispatchCoreOps(ctx, c, op, body); handled {
		return result
	}

	if handled, result := h.dispatchNewOps(ctx, c, op, body); handled {
		return result
	}

	return writeError(c, http.StatusBadRequest, "UnknownOperationException", "unknown operation: "+op)
}

// memorydbCoreOps maps operation names to handler functions for core MemoryDB operations.
//
//nolint:gochecknoglobals // read-only dispatch table initialized once at startup
var memorydbCoreOps = map[string]func(*Handler, context.Context, *echo.Context, []byte) error{
	"CreateCluster":           (*Handler).handleCreateCluster,
	"DescribeClusters":        (*Handler).handleDescribeClusters,
	"DeleteCluster":           (*Handler).handleDeleteCluster,
	"UpdateCluster":           (*Handler).handleUpdateCluster,
	"CreateACL":               (*Handler).handleCreateACL,
	"DescribeACLs":            (*Handler).handleDescribeACLs,
	"DeleteACL":               (*Handler).handleDeleteACL,
	"UpdateACL":               (*Handler).handleUpdateACL,
	"CreateSubnetGroup":       (*Handler).handleCreateSubnetGroup,
	"DescribeSubnetGroups":    (*Handler).handleDescribeSubnetGroups,
	"DeleteSubnetGroup":       (*Handler).handleDeleteSubnetGroup,
	"UpdateSubnetGroup":       (*Handler).handleUpdateSubnetGroup,
	"CreateUser":              (*Handler).handleCreateUser,
	"DescribeUsers":           (*Handler).handleDescribeUsers,
	"DeleteUser":              (*Handler).handleDeleteUser,
	"UpdateUser":              (*Handler).handleUpdateUser,
	"CreateParameterGroup":    (*Handler).handleCreateParameterGroup,
	"DescribeParameterGroups": (*Handler).handleDescribeParameterGroups,
	"DeleteParameterGroup":    (*Handler).handleDeleteParameterGroup,
	"UpdateParameterGroup":    (*Handler).handleUpdateParameterGroup,
	"ListTags":                (*Handler).handleListTags,
	"TagResource":             (*Handler).handleTagResource,
	"UntagResource":           (*Handler).handleUntagResource,
}

// dispatchCoreOps handles the original core operations.
func (h *Handler) dispatchCoreOps(ctx context.Context, c *echo.Context, op string, body []byte) (bool, error) {
	if fn, ok := memorydbCoreOps[op]; ok {
		return true, fn(h, ctx, c, body)
	}

	return false, nil
}

// dispatchNewOps handles the new operations added in this release.
func (h *Handler) dispatchNewOps(ctx context.Context, c *echo.Context, op string, body []byte) (bool, error) {
	if ok, err := h.dispatchSnapshotAndEngineOps(ctx, c, op, body); ok {
		return true, err
	}

	if ok, err := h.dispatchMultiRegionOps(ctx, c, op, body); ok {
		return true, err
	}

	return h.dispatchParameterAndShardOps(ctx, c, op, body)
}

// dispatchSnapshotAndEngineOps handles snapshot, engine-version, and event operations.
func (h *Handler) dispatchSnapshotAndEngineOps(
	ctx context.Context,
	c *echo.Context,
	op string,
	body []byte,
) (bool, error) {
	switch op {
	case "CreateSnapshot":

		return true, h.handleCreateSnapshot(ctx, c, body)
	case "DescribeSnapshots":

		return true, h.handleDescribeSnapshots(ctx, c, body)
	case "CopySnapshot":

		return true, h.handleCopySnapshot(ctx, c, body)
	case "DeleteSnapshot":

		return true, h.handleDeleteSnapshot(ctx, c, body)
	case "ExportSnapshot":

		return true, h.handleExportSnapshot(ctx, c, body)
	case "DescribeEngineVersions":

		return true, h.handleDescribeEngineVersions(ctx, c, body)
	case "DescribeEvents":

		return true, h.handleDescribeEvents(ctx, c, body)
	case "BatchUpdateCluster":

		return true, h.handleBatchUpdateCluster(ctx, c, body)
	case "DescribeServiceUpdates":

		return true, h.handleDescribeServiceUpdates(ctx, c, body)
	}

	return false, nil
}

// dispatchMultiRegionOps handles multi-region cluster and parameter group operations.
func (h *Handler) dispatchMultiRegionOps(ctx context.Context, c *echo.Context, op string, body []byte) (bool, error) {
	switch op {
	case "CreateMultiRegionCluster":

		return true, h.handleCreateMultiRegionCluster(ctx, c, body)
	case "DeleteMultiRegionCluster":

		return true, h.handleDeleteMultiRegionCluster(ctx, c, body)
	case "DescribeMultiRegionClusters":

		return true, h.handleDescribeMultiRegionClusters(ctx, c, body)
	case "DescribeMultiRegionParameterGroups":

		return true, h.handleDescribeMultiRegionParameterGroups(ctx, c, body)
	case "UpdateMultiRegionCluster":

		return true, h.handleUpdateMultiRegionCluster(ctx, c, body)
	case "ListAllowedMultiRegionClusterUpdates":

		return true, h.handleListAllowedMultiRegionClusterUpdates(ctx, c, body)
	}

	return false, nil
}

// dispatchParameterAndShardOps handles parameter group and shard operations.
func (h *Handler) dispatchParameterAndShardOps(
	ctx context.Context,
	c *echo.Context,
	op string,
	body []byte,
) (bool, error) {
	switch op {
	case "DescribeParameters":

		return true, h.handleDescribeParameters(ctx, c, body)
	case "ResetParameterGroup":

		return true, h.handleResetParameterGroup(ctx, c, body)
	case "FailoverShard":

		return true, h.handleFailoverShard(ctx, c, body)
	case "ListAllowedNodeTypeUpdates":

		return true, h.handleListAllowedNodeTypeUpdates(ctx, c, body)
	case "DescribeReservedNodes":

		return true, h.handleDescribeReservedNodes(ctx, c, body)
	case "DescribeReservedNodesOfferings":

		return true, h.handleDescribeReservedNodesOfferings(ctx, c, body)
	case "PurchaseReservedNodesOffering":

		return true, h.handlePurchaseReservedNodesOffering(ctx, c, body)
	case "DescribeMultiRegionParameters":

		return true, h.handleDescribeMultiRegionParameters(ctx, c, body)
	}

	return false, nil
}

// -- Cluster handlers ------------------------------------------------------------

// paginateItems applies cursor-based pagination to a slice of named items.
// getName extracts the name used as a pagination cursor from each item.
func paginateItems[T any](items []T, token string, maxResults *int32, getName func(T) string) ([]T, string) {
	if token != "" {
		items = items[findStartIndex(items, token, getName):]
	}

	limit := 100
	if maxResults != nil && *maxResults > 0 && int(*maxResults) < limit {
		limit = int(*maxResults)
	}

	var nextToken string

	if limit < len(items) {
		nextToken = getName(items[limit])
		items = items[:limit]
	}

	return items, nextToken
}

// findStartIndex returns the index of the item whose name equals token, or 0
// if not found. token is emitted by paginateItems as the name of the first
// item of the next page (items[limit], inclusive) -- resuming at i+1 instead
// of i silently dropped that item from every page boundary.
func findStartIndex[T any](items []T, token string, getName func(T) string) int {
	for i, item := range items {
		if getName(item) == token {
			return i
		}
	}

	return 0
}

// errCodeLookup maps specific backend sentinel errors to the exact AWS
// MemoryDB fault name real aws-sdk-go-v2 clients switch on via
// errors.As(&types.XyzFault{}) (aws-sdk-go-v2/service/memorydb/types.errors.go).
// MemoryDB defines no generic "ResourceNotFoundException"/"ResourceInUseException"
// -- every fault is resource-specific (ClusterNotFoundFault, ACLNotFoundFault,
// ...), so writeBackendError must not collapse them into one bucket the way a
// coarse awserr-category switch would. Checked before the generic fallback
// below; analogous to EC2's errCodeLookup / CloudFront's errCodeMapping.
//
// All statuses are http.StatusBadRequest (400): confirmed against the
// aws-sdk-go v1 model (models/apis/memorydb/2021-01-01/api-2.json) -- every
// one of MemoryDB's ~53 exception shapes has an empty "error" trait ({}, no
// httpStatusCode override), and the JSON-protocol default for an
// unoverridden client-fault shape is 400. This also matches
// deserializers.go's error dispatch, which resolves the typed error purely
// from the __type/code field (resolveProtocolErrorType), never from
// response.StatusCode -- so a real aws-sdk-go-v2 client's errors.As(&types.
// ClusterNotFoundFault{}) behaves identically regardless of which 4xx status
// is on the wire. A prior pass left the pre-existing 404/409 split in place
// pending this confirmation; fixed now that the model has been checked.
//
//nolint:gochecknoglobals // read-only lookup table initialized once at startup
var errCodeLookup = []struct {
	err    error
	code   string
	status int
}{
	{ErrClusterNotFound, "ClusterNotFoundFault", http.StatusBadRequest},
	{ErrClusterAlreadyExists, "ClusterAlreadyExistsFault", http.StatusBadRequest},
	{ErrACLInUse, "InvalidACLStateFault", http.StatusBadRequest},
	{ErrACLNotFound, "ACLNotFoundFault", http.StatusBadRequest},
	{ErrACLAlreadyExists, "ACLAlreadyExistsFault", http.StatusBadRequest},
	{ErrSubnetGroupInUse, "SubnetGroupInUseFault", http.StatusBadRequest},
	{ErrSubnetGroupNotFound, "SubnetGroupNotFoundFault", http.StatusBadRequest},
	{ErrSubnetGroupAlreadyExists, "SubnetGroupAlreadyExistsFault", http.StatusBadRequest},
	{ErrUserNotFound, "UserNotFoundFault", http.StatusBadRequest},
	{ErrUserAlreadyExists, "UserAlreadyExistsFault", http.StatusBadRequest},
	{ErrParameterGroupNotFound, "ParameterGroupNotFoundFault", http.StatusBadRequest},
	{ErrParameterGroupAlreadyExists, "ParameterGroupAlreadyExistsFault", http.StatusBadRequest},
	{ErrParameterGroupInUse, "InvalidParameterGroupStateFault", http.StatusBadRequest},
	{ErrSnapshotNotFound, "SnapshotNotFoundFault", http.StatusBadRequest},
	{ErrSnapshotAlreadyExists, "SnapshotAlreadyExistsFault", http.StatusBadRequest},
	{ErrMultiRegionClusterNotFound, "MultiRegionClusterNotFoundFault", http.StatusBadRequest},
	{ErrMultiRegionClusterAlreadyExists, "MultiRegionClusterAlreadyExistsFault", http.StatusBadRequest},
	{ErrMultiRegionParameterGroupNotFound, "MultiRegionParameterGroupNotFoundFault", http.StatusBadRequest},
	{ErrInvalidARN, "InvalidARNFault", http.StatusBadRequest},
	{ErrReservationAlreadyExists, "ReservedNodeAlreadyExistsFault", http.StatusBadRequest},
	{ErrServiceUpdateNotFound, "ServiceUpdateNotFoundFault", http.StatusBadRequest},
}

// writeBackendError translates a backend error to an HTTP response. It
// prefers the specific AWS fault name from errCodeLookup; the coarse
// awserr-category switch below is a best-effort fallback only, for any
// sentinel error not (yet) cataloged in errCodeLookup above.
func (h *Handler) writeBackendError(c *echo.Context, err error) error {
	for _, e := range errCodeLookup {
		if errors.Is(err, e.err) {
			return writeError(c, e.status, e.code, err.Error())
		}
	}

	// Every category below is a client fault; per the httpStatusCode
	// confirmation above (all-400) these use http.StatusBadRequest uniformly,
	// matching errCodeLookup. Only a true unclassified/unexpected error falls
	// to the 500 default.
	switch {
	case errors.Is(err, awserr.ErrNotFound):

		return writeError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	case errors.Is(err, awserr.ErrAlreadyExists):

		return writeError(c, http.StatusBadRequest, "ResourceInUseException", err.Error())
	case errors.Is(err, awserr.ErrInvalidParameter):

		return writeError(c, http.StatusBadRequest, "InvalidParameterValueException", err.Error())
	case errors.Is(err, awserr.ErrConflict):

		return writeError(c, http.StatusBadRequest, "InvalidRequestException", err.Error())
	default:

		return writeError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}
}

const (
	maxTagKeyLen   = 128
	maxTagValueLen = 256
	maxTagsPerRes  = 50
)

var (
	errTagQuotaExceeded = fmt.Errorf("tag quota exceeded: a resource may have at most %d tags", maxTagsPerRes)
	errTagKeyLength     = fmt.Errorf("tag key length must be between 1 and %d characters", maxTagKeyLen)
	errTagKeyPrefix     = errors.New(`tag key must not start with reserved prefix "aws:"`)
	errTagValueLength   = fmt.Errorf("tag value length must be 0-%d characters", maxTagValueLen)
)

// validateTagEntries enforces AWS MemoryDB tag constraints:
// key 1-128 chars, no "aws:" prefix; value 0-256 chars; at most 50 tags.
func validateTagEntries(tags []tagEntry) error {
	if len(tags) > maxTagsPerRes {
		return errTagQuotaExceeded
	}

	for _, t := range tags {
		klen := utf8.RuneCountInString(t.Key)
		if klen == 0 || klen > maxTagKeyLen {
			return errTagKeyLength
		}

		if strings.HasPrefix(t.Key, "aws:") {
			return errTagKeyPrefix
		}

		if vlen := utf8.RuneCountInString(t.Value); vlen > maxTagValueLen {
			return errTagValueLength
		}
	}

	return nil
}

// writeError writes a JSON error response using the standard AWS JSON 1.1 envelope.
func writeError(c *echo.Context, status int, errType, message string) error {
	return c.JSON(status, errorResponse{Type: errType, Message: message})
}

// Purge implements service.Purgeable by removing all MemoryDB resources older than cutoff.
func (h *Handler) Purge(ctx context.Context, cutoff time.Time) {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Purge(ctx, cutoff)
	}
}
