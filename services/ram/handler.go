package ram

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyTypeField    = "__type"
	keyMessageField = "message"
	permStandard    = "STANDARD"

	opCreatePermissionVersion             = "CreatePermissionVersion"
	opCreateResourceShare                 = "CreateResourceShare"
	opDeletePermission                    = "DeletePermission"
	opDeletePermissionVersion             = "DeletePermissionVersion"
	opDeleteResourceShare                 = "DeleteResourceShare"
	opDisassociateResourceShare           = "DisassociateResourceShare"
	opDisassociateResourceSharePermission = "DisassociateResourceSharePermission"
	opEnableSharingWithAwsOrganization    = "EnableSharingWithAwsOrganization"
	opGetPermission                       = "GetPermission"
	opGetResourcePolicies                 = "GetResourcePolicies"
	opGetResourceShareAssociations        = "GetResourceShareAssociations"
	opGetResourceShareInvitations         = "GetResourceShareInvitations"
	opGetResourceShares                   = "GetResourceShares"
	opListResourceSharePermissions        = "ListResourceSharePermissions"
	// opListTagsForResource is an internal route label for POST /listtagsforresource.
	// It is NOT a real AWS RAM SDK operation — the real API has no ListTagsForResource
	// action at all (verified against botocore's ram service-2.json: only
	// /tagresource and /untagresource exist under the tags family). Real clients read
	// tags back via GetResourceShares, whose ResourceShare.Tags field gopherstack
	// already populates correctly. RAM dispatches purely by request path via
	// ramGetListRoutes, so a real client can never send this path and the route is
	// unreachable by real traffic either way; it stays wired below as internal test
	// scaffolding only, unadvertised — see gopherstack-vhw2 category A, same
	// resolution as EMR's ListTagsForResource and CloudFront's
	// GetFunctionAssociations/SetFunctionAssociations.
	opListTagsForResource = "ListTagsForResource"
	opTagResource         = "TagResource"
	opUntagResource       = "UntagResource"
	opUpdateResourceShare = "UpdateResourceShare"
)

const (
	opAcceptResourceShareInvitation    = "AcceptResourceShareInvitation"
	opAssociateResourceShare           = "AssociateResourceShare"
	opAssociateResourceSharePermission = "AssociateResourceSharePermission"
	opCreatePermission                 = "CreatePermission"
)

const (
	opListPendingInvitationResources        = "ListPendingInvitationResources"
	opListPermissionAssociations            = "ListPermissionAssociations"
	opListPermissionVersions                = "ListPermissionVersions"
	opListPermissions                       = "ListPermissions"
	opListPrincipals                        = "ListPrincipals"
	opListReplacePermissionAssociationsWork = "ListReplacePermissionAssociationsWork"
	opListResourceTypes                     = "ListResourceTypes"
	opListResources                         = "ListResources"
	opListSourceAssociations                = "ListSourceAssociations"
	opPromotePermissionCreatedFromPolicy    = "PromotePermissionCreatedFromPolicy"
	opPromoteResourceShareCreatedFromPolicy = "PromoteResourceShareCreatedFromPolicy"
	opRejectResourceShareInvitation         = "RejectResourceShareInvitation"
	opReplacePermissionAssociations         = "ReplacePermissionAssociations"
	opSetDefaultPermissionVersion           = "SetDefaultPermissionVersion"
)

const (
	ramService       = "ram"
	ramMatchPriority = 87
	maxShareNameLen  = 256
)

var ramShareNameRegex = regexp.MustCompile(`^[\w\-.]+$`)

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request")
)

// Handler is the HTTP handler for the AWS RAM REST API.
type Handler struct {
	Backend   StorageBackend
	AccountID string
	Region    string
}

// NewHandler creates a new RAM handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend:   backend,
		AccountID: backend.AccountID(),
		Region:    backend.Region(),
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "RAM" }

// GetSupportedOperations returns the list of supported RAM operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opAcceptResourceShareInvitation,
		opAssociateResourceShare,
		opAssociateResourceSharePermission,
		opCreatePermission,
		opCreatePermissionVersion,
		opCreateResourceShare,
		opDeletePermission,
		opDeletePermissionVersion,
		opDeleteResourceShare,
		opDisassociateResourceShare,
		opDisassociateResourceSharePermission,
		opEnableSharingWithAwsOrganization,
		opGetPermission,
		opGetResourcePolicies,
		opGetResourceShareAssociations,
		opGetResourceShareInvitations,
		opGetResourceShares,
		opListPendingInvitationResources,
		opListPermissionAssociations,
		opListPermissionVersions,
		opListPermissions,
		opListPrincipals,
		opListReplacePermissionAssociationsWork,
		opListResourceSharePermissions,
		opListResourceTypes,
		opListResources,
		opListSourceAssociations,
		// opListTagsForResource is deliberately NOT advertised — see its doc comment
		// in the const block above; it is not a real RAM SDK operation.
		opPromotePermissionCreatedFromPolicy,
		opPromoteResourceShareCreatedFromPolicy,
		opRejectResourceShareInvitation,
		opReplacePermissionAssociations,
		opSetDefaultPermissionVersion,
		opTagResource,
		opUntagResource,
		opUpdateResourceShare,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return ramService }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// RouteMatcher returns a function that matches RAM API requests.
// All path-based matches are gated on the SigV4 service name to prevent
// routing conflicts with other services that share similar REST paths.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		if httputils.ExtractServiceFromRequest(c.Request()) != ramService {
			return false
		}

		path := c.Request().URL.Path

		return isRAMCreateOrAcceptPath(path) || isRAMDeletePath(path) ||
			isRAMGetPath(path) || isRAMAssociationPath(path) || isRAMTagPath(path) ||
			isRAMListPath(path) || isRAMPromotePath(path)
	}
}

// isRAMCreateOrAcceptPath returns true for create/accept paths.
func isRAMCreateOrAcceptPath(path string) bool {
	return strings.HasPrefix(path, "/acceptresourceshareinvitation") ||
		strings.HasPrefix(path, "/createpermission") ||
		strings.HasPrefix(path, "/createresourceshare") ||
		strings.HasPrefix(path, "/enablesharingwithawsorganization") ||
		strings.HasPrefix(path, "/rejectresourceshareinvitation") ||
		strings.HasPrefix(path, "/setdefaultpermissionversion") ||
		strings.HasPrefix(path, "/updateresourceshare")
}

// isRAMListPath returns true for new list operation paths.
func isRAMListPath(path string) bool {
	return strings.HasPrefix(path, "/listpendinginvitationresources") ||
		strings.HasPrefix(path, "/listpermissionassociations") ||
		strings.HasPrefix(path, "/listpermissionversions") ||
		strings.HasPrefix(path, "/listpermissions") ||
		strings.HasPrefix(path, "/listprincipals") ||
		strings.HasPrefix(path, "/listreplacepermissionassociationswork") ||
		strings.HasPrefix(path, "/listresourcetypes") ||
		strings.HasPrefix(path, "/listresources") ||
		strings.HasPrefix(path, "/listsourceassociations")
}

// isRAMPromotePath returns true for promote/replace paths.
func isRAMPromotePath(path string) bool {
	return strings.HasPrefix(path, "/promotepermissioncreatedfrompolicy") ||
		strings.HasPrefix(path, "/promoteresourcesharecreatedfrompolicy") ||
		strings.HasPrefix(path, "/replacepermissionassociations")
}

// isRAMDeletePath returns true for delete paths.
func isRAMDeletePath(path string) bool {
	return strings.HasPrefix(path, "/deletepermission") ||
		strings.HasPrefix(path, "/deleteresourceshare")
}

// isRAMGetPath returns true for read paths.
func isRAMGetPath(path string) bool {
	return strings.HasPrefix(path, "/getpermission") ||
		strings.HasPrefix(path, "/getresourcepolicies") ||
		strings.HasPrefix(path, "/getresourceshareassociations") ||
		strings.HasPrefix(path, "/getresourceshareinvitations") ||
		strings.HasPrefix(path, "/getresourceshares") ||
		strings.HasPrefix(path, "/listresourcesharepermissions") ||
		strings.HasPrefix(path, "/listtagsforresource")
}

// isRAMAssociationPath returns true for association/disassociation paths.
func isRAMAssociationPath(path string) bool {
	return strings.HasPrefix(path, "/associateresourcesharepermission") ||
		strings.HasPrefix(path, "/associateresourceshare") ||
		strings.HasPrefix(path, "/disassociateresourcesharepermission") ||
		strings.HasPrefix(path, "/disassociateresourceshare")
}

// isRAMTagPath returns true for tag paths.
func isRAMTagPath(path string) bool {
	return strings.HasPrefix(path, "/tagresource") ||
		strings.HasPrefix(path, "/untagresource")
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return ramMatchPriority }

// ExtractOperation extracts the operation name from the request path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	path := c.Request().URL.Path

	if op := extractCommonOperation(path); op != "" {
		return op
	}

	return extractAssociationOperation(path)
}

// extractCommonOperation maps non-association RAM paths to their operation names.
func extractCommonOperation(path string) string {
	if op := extractCreateDeleteOp(path); op != "" {
		return op
	}

	return extractGetListOp(path)
}

// extractCreateDeleteOp maps create/delete/accept paths to operation names.
func extractCreateDeleteOp(path string) string {
	switch {
	case strings.HasPrefix(path, "/acceptresourceshareinvitation"):
		return opAcceptResourceShareInvitation
	case strings.HasPrefix(path, "/createpermissionversion"):
		return opCreatePermissionVersion
	case strings.HasPrefix(path, "/createpermission"):
		return opCreatePermission
	case strings.HasPrefix(path, "/createresourceshare"):
		return opCreateResourceShare
	case strings.HasPrefix(path, "/deletepermissionversion"):
		return opDeletePermissionVersion
	case strings.HasPrefix(path, "/deletepermission"):
		return opDeletePermission
	case strings.HasPrefix(path, "/deleteresourceshare"):
		return opDeleteResourceShare
	case strings.HasPrefix(path, "/enablesharingwithawsorganization"):
		return opEnableSharingWithAwsOrganization
	case strings.HasPrefix(path, "/rejectresourceshareinvitation"):
		return opRejectResourceShareInvitation
	case strings.HasPrefix(path, "/setdefaultpermissionversion"):
		return opSetDefaultPermissionVersion
	case strings.HasPrefix(path, "/updateresourceshare"):
		return opUpdateResourceShare
	default:
		return ""
	}
}

// ramGetListRoutes maps path prefixes to operation names for RAM get/list paths.
// Longer prefixes must appear before shorter ones that share a prefix.
//
//nolint:gochecknoglobals // read-only route table initialized once at startup
var ramGetListRoutes = []struct {
	prefix string
	op     string
}{
	{"/getpermission", opGetPermission},
	{"/getresourcepolicies", opGetResourcePolicies},
	{"/getresourceshareassociations", opGetResourceShareAssociations},
	{"/getresourceshareinvitations", opGetResourceShareInvitations},
	{"/getresourceshares", opGetResourceShares},
	{"/listpendinginvitationresources", opListPendingInvitationResources},
	{"/listpermissionassociations", opListPermissionAssociations},
	{"/listpermissionversions", opListPermissionVersions},
	{"/listpermissions", opListPermissions},
	{"/listprincipals", opListPrincipals},
	{"/listreplacepermissionassociationswork", opListReplacePermissionAssociationsWork},
	{"/listresourcesharepermissions", opListResourceSharePermissions},
	{"/listresourcetypes", opListResourceTypes},
	{"/listresources", opListResources},
	{"/listsourceassociations", opListSourceAssociations},
	{"/promotepermissioncreatedfrompolicy", opPromotePermissionCreatedFromPolicy},
	{"/promoteresourcesharecreatedfrompolicy", opPromoteResourceShareCreatedFromPolicy},
	{"/replacepermissionassociations", opReplacePermissionAssociations},
	{"/listtagsforresource", opListTagsForResource},
	{"/tagresource", opTagResource},
	{"/untagresource", opUntagResource},
}

// extractGetListOp maps get/list paths to operation names.
func extractGetListOp(path string) string {
	for _, r := range ramGetListRoutes {
		if strings.HasPrefix(path, r.prefix) {
			return r.op
		}
	}

	return ""
}

// extractAssociationOperation maps associate/disassociate RAM paths to their operation names.
func extractAssociationOperation(path string) string {
	switch {
	case strings.HasPrefix(path, "/associateresourcesharepermission"):
		return opAssociateResourceSharePermission
	case strings.HasPrefix(path, "/disassociateresourcesharepermission"):
		return opDisassociateResourceSharePermission
	case strings.HasPrefix(path, "/disassociateresourceshare"):
		return opDisassociateResourceShare
	case strings.HasPrefix(path, "/associateresourceshare"):
		return opAssociateResourceShare
	default:
		return "Unknown"
	}
}

// ExtractResource extracts the resource share ARN from the request body or query.
func (h *Handler) ExtractResource(c *echo.Context) string {
	return c.Request().URL.Query().Get("resourceShareArn")
}

// Handler returns the Echo handler function for RAM requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "ram: failed to read request body", "error", err)

			return writeInternalServerError(c)
		}

		op := h.ExtractOperation(c)

		result, dispErr := h.dispatch(ctx, op, c, body)
		if dispErr != nil {
			return h.handleError(c, dispErr)
		}

		if result == nil {
			return c.JSON(http.StatusOK, map[string]any{})
		}

		return c.JSONBlob(http.StatusOK, result)
	}
}

func (h *Handler) dispatch(
	ctx context.Context,
	op string,
	c *echo.Context,
	body []byte,
) ([]byte, error) {
	if result, ok, err := h.dispatchMutateOps(ctx, op, c, body); ok {
		return result, err
	}

	if result, ok, err := h.dispatchReadOps(ctx, op, body); ok {
		return result, err
	}

	return nil, fmt.Errorf("%w: %s", errUnknownAction, op)
}

func (h *Handler) dispatchMutateOps(
	ctx context.Context,
	op string,
	c *echo.Context,
	body []byte,
) ([]byte, bool, error) {
	if result, ok, err := h.dispatchCRUDOps(ctx, op, c, body); ok {
		return result, true, err
	}

	if result, ok, err := h.dispatchAssocOps(ctx, op, body); ok {
		return result, true, err
	}

	return nil, false, nil
}

func (h *Handler) dispatchCRUDOps(
	ctx context.Context,
	op string,
	c *echo.Context,
	body []byte,
) ([]byte, bool, error) {
	if r, ok, err := h.dispatchCRUDShareOps(ctx, op, c, body); ok {
		return r, true, err
	}

	return h.dispatchCRUDPermissionOps(ctx, op, c, body)
}

func (h *Handler) dispatchCRUDShareOps(
	ctx context.Context,
	op string,
	c *echo.Context,
	body []byte,
) ([]byte, bool, error) {
	switch op {
	case opAcceptResourceShareInvitation:
		r, err := h.handleAcceptResourceShareInvitation(ctx, body)

		return r, true, err
	case opCreateResourceShare:
		r, err := h.handleCreateResourceShare(ctx, body)

		return r, true, err
	case opDeleteResourceShare:
		r, err := h.handleDeleteResourceShare(ctx, c)

		return r, true, err
	case opUpdateResourceShare:
		r, err := h.handleUpdateResourceShare(ctx, body)

		return r, true, err
	case opRejectResourceShareInvitation:
		r, err := h.handleRejectResourceShareInvitation(ctx, body)

		return r, true, err
	case opPromoteResourceShareCreatedFromPolicy:
		r, err := h.handlePromoteResourceShareCreatedFromPolicy(ctx, body)

		return r, true, err
	case opEnableSharingWithAwsOrganization:
		r, err := h.handleEnableSharingWithAwsOrganization()

		return r, true, err
	case opTagResource:
		err := h.handleTagResource(ctx, body)

		return nil, true, err
	case opUntagResource:
		err := h.handleUntagResource(ctx, body)

		return nil, true, err
	default:
		return nil, false, nil
	}
}

func (h *Handler) dispatchCRUDPermissionOps(
	ctx context.Context,
	op string,
	c *echo.Context,
	body []byte,
) ([]byte, bool, error) {
	switch op {
	case opCreatePermission:
		r, err := h.handleCreatePermission(ctx, body)

		return r, true, err
	case opCreatePermissionVersion:
		r, err := h.handleCreatePermissionVersion(ctx, body)

		return r, true, err
	case opDeletePermission:
		r, err := h.handleDeletePermission(ctx, c)

		return r, true, err
	case opDeletePermissionVersion:
		r, err := h.handleDeletePermissionVersion(ctx, c)

		return r, true, err
	case opSetDefaultPermissionVersion:
		r, err := h.handleSetDefaultPermissionVersion(ctx, body)

		return r, true, err
	case opPromotePermissionCreatedFromPolicy:
		r, err := h.handlePromotePermissionCreatedFromPolicy(ctx, body)

		return r, true, err
	case opReplacePermissionAssociations:
		r, err := h.handleReplacePermissionAssociations(ctx, body)

		return r, true, err
	default:
		return nil, false, nil
	}
}

func (h *Handler) dispatchAssocOps(
	ctx context.Context,
	op string,
	body []byte,
) ([]byte, bool, error) {
	switch op {
	case opAssociateResourceSharePermission:
		r, err := h.handleAssociateResourceSharePermission(ctx, body)

		return r, true, err
	case opAssociateResourceShare:
		r, err := h.handleAssociateResourceShare(ctx, body)

		return r, true, err
	case opDisassociateResourceSharePermission:
		r, err := h.handleDisassociateResourceSharePermission(ctx, body)

		return r, true, err
	case opDisassociateResourceShare:
		r, err := h.handleDisassociateResourceShare(ctx, body)

		return r, true, err
	default:
		return nil, false, nil
	}
}

func (h *Handler) dispatchReadOps(
	ctx context.Context,
	op string,
	body []byte,
) ([]byte, bool, error) {
	if r, ok, err := h.dispatchGetOps(ctx, op, body); ok {
		return r, true, err
	}

	return h.dispatchListOps(ctx, op, body)
}

func (h *Handler) dispatchGetOps(
	ctx context.Context,
	op string,
	body []byte,
) ([]byte, bool, error) {
	switch op {
	case opGetPermission:
		r, err := h.handleGetPermission(ctx, body)

		return r, true, err
	case opGetResourcePolicies:
		r, err := h.handleGetResourcePolicies(ctx, body)

		return r, true, err
	case opGetResourceShareAssociations:
		r, err := h.handleGetResourceShareAssociations(ctx, body)

		return r, true, err
	case opGetResourceShareInvitations:
		r, err := h.handleGetResourceShareInvitations(ctx, body)

		return r, true, err
	case opGetResourceShares:
		r, err := h.handleGetResourceShares(ctx, body)

		return r, true, err
	case opListTagsForResource:
		r, err := h.handleListTagsForResource(ctx, body)

		return r, true, err
	case opListResourceSharePermissions:
		r, err := h.handleListResourceSharePermissions(ctx, body)

		return r, true, err
	default:
		return nil, false, nil
	}
}

func (h *Handler) dispatchListOps(
	ctx context.Context,
	op string,
	body []byte,
) ([]byte, bool, error) {
	switch op {
	case opListPendingInvitationResources:
		r, err := h.handleListPendingInvitationResources(ctx, body)

		return r, true, err
	case opListPermissionAssociations:
		r, err := h.handleListPermissionAssociations(ctx, body)

		return r, true, err
	case opListPermissionVersions:
		r, err := h.handleListPermissionVersions(ctx, body)

		return r, true, err
	case opListPermissions:
		r, err := h.handleListPermissions(ctx, body)

		return r, true, err
	case opListPrincipals:
		r, err := h.handleListPrincipals(ctx, body)

		return r, true, err
	case opListReplacePermissionAssociationsWork:
		r, err := h.handleListReplacePermissionAssociationsWork(ctx, body)

		return r, true, err
	case opListResourceTypes:
		r, err := h.handleListResourceTypes(ctx, body)

		return r, true, err
	case opListResources:
		r, err := h.handleListResources(ctx, body)

		return r, true, err
	case opListSourceAssociations:
		r, err := h.handleListSourceAssociations(ctx, body)

		return r, true, err
	default:
		return nil, false, nil
	}
}

// writeInternalServerError renders a ReadBody-failure (body too large, read
// error) as ram's own restjson1 error envelope. aws-sdk-go-v2's
// restjson.GetErrorInfo (aws/protocol/restjson/decoder_util.go) JSON-decodes
// the body for __type/message, so the bare text/plain this used to send
// deserialized client-side as smithy.GenericAPIError{Code:"UnknownError"}
// (gopherstack-o7gx). ServerInternalException is ram's own modeled internal
// error (ram@v1.39.4 types/errors.go).
func writeInternalServerError(c *echo.Context) error {
	payload, err := json.Marshal(map[string]string{
		keyTypeField:    "ServerInternalException",
		keyMessageField: "internal server error",
	})
	if err != nil {
		return err
	}

	return c.JSONBlob(http.StatusInternalServerError, payload)
}

// codeInvalidParameter is RAM's real InvalidParameterException code, shared
// by ErrPermissionVersionNotFound/ErrInvalidParameter/ErrValidation below --
// each op's own error model was checked individually (deserializers.go);
// all three happen to land on the same real type.
const codeInvalidParameter = "InvalidParameterException"

// errCodeLookup maps every ram sentinel error to the exact wire code its
// raising op's own deserializeOpError switch models (deserializers.go@
// ram v1.39.4). All entries are HTTP 400. ErrAlreadyExists is the one
// documented exception: CreateResourceShare's own model defines no
// AlreadyExists-shaped exception at all (see its doc in errors.go), so the
// code here is left as the pre-existing fabricated string -- no replacement
// invented, per audit policy.
//
//nolint:gochecknoglobals // read-only lookup table initialized once at startup
var errCodeLookup = []struct {
	err  error
	code string
}{
	{ErrNotFound, "UnknownResourceException"},
	{ErrPermissionNotFound, "UnknownResourceException"},
	{ErrPermissionVersionNotFound, codeInvalidParameter},
	{ErrInvitationNotFound, "ResourceShareInvitationArnNotFoundException"},
	{ErrAlreadyExists, "ResourceShareAlreadyExistsException"},
	{ErrPermissionAlreadyExists, "PermissionAlreadyExistsException"},
	{ErrInvitationAlreadyAccepted, "ResourceShareInvitationAlreadyAcceptedException"},
	{ErrInvitationAlreadyRejected, "ResourceShareInvitationAlreadyRejectedException"},
	{ErrInvitationExpired, "ResourceShareInvitationExpiredException"},
	{ErrPermissionInUse, "OperationNotPermittedException"},
	{ErrOperationNotPermitted, "OperationNotPermittedException"},
	{ErrInvalidParameter, codeInvalidParameter},
	{ErrValidation, codeInvalidParameter},
	{ErrMalformedArn, "MalformedArnException"},
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	for _, e := range errCodeLookup {
		if errors.Is(err, e.err) {
			payload, _ := json.Marshal(map[string]string{keyTypeField: e.code, keyMessageField: err.Error()})

			return c.JSONBlob(http.StatusBadRequest, payload)
		}
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.Is(err, errInvalidRequest), errors.Is(err, errUnknownAction),
		errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return c.JSON(http.StatusBadRequest, map[string]string{keyMessageField: err.Error()})
	default:
		return c.JSON(
			http.StatusInternalServerError,
			map[string]string{keyMessageField: err.Error()},
		)
	}
}

// epochSeconds converts a [time.Time] to Unix epoch seconds as float64,
// as required by the AWS REST-JSON protocol for timestamp fields.
func epochSeconds(t time.Time) float64 {
	return float64(t.Unix())
}

const ramMaxResults = 100

// ramParseNextToken decodes an opaque NextToken string to a slice start index.
// Tokens are base64-encoded offsets; a plain-integer fallback handles any
// tokens produced before this change.
func ramParseNextToken(token string) int {
	if token == "" {
		return 0
	}
	// Try base64-encoded offset first (current format).
	if decoded, decErr := base64.StdEncoding.DecodeString(token); decErr == nil {
		if idx, atoiErr := strconv.Atoi(string(decoded)); atoiErr == nil && idx >= 0 {
			return idx
		}
	}
	// Fallback: plain decimal offset from tokens produced before this change.
	idx, err := strconv.Atoi(token)
	if err != nil || idx < 0 {
		return 0
	}

	return idx
}

// ramEncodeNextToken encodes a pagination offset as an opaque base64 token.
func ramEncodeNextToken(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// ramPaginate applies MaxResults/NextToken pagination to a slice.
// Returns the page, the next opaque token (empty when last page), and a validation error.
func ramPaginate[T any](items []T, nextToken string, maxResults *int32) ([]T, string, error) {
	limit := int32(ramMaxResults)

	if maxResults != nil {
		if *maxResults < 1 || *maxResults > ramMaxResults {
			return nil, "", fmt.Errorf(
				"%w: maxResults must be between 1 and %d",
				ErrInvalidParameter,
				ramMaxResults,
			)
		}

		limit = *maxResults
	}

	start := ramParseNextToken(nextToken)

	if start >= len(items) {
		return items[:0], "", nil
	}

	end := start + int(limit)

	var outToken string

	if end < len(items) {
		outToken = ramEncodeNextToken(end)
	} else {
		end = len(items)
	}

	return items[start:end], outToken, nil
}

// tagObject represents a RAM tag in the JSON API format.
type tagObject struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// toTagObjects converts a map of tags to a slice of tag objects sorted by key.
func toTagObjects(tags map[string]string) []tagObject {
	keys := collections.SortedKeys(tags)
	result := make([]tagObject, 0, len(keys))

	for _, k := range keys {
		result = append(result, tagObject{Key: k, Value: tags[k]})
	}

	return result
}

// fromTagObjects converts a slice of tag objects to a map.
func fromTagObjects(tags []tagObject) map[string]string {
	result := make(map[string]string, len(tags))

	for _, t := range tags {
		result[t.Key] = t.Value
	}

	return result
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	h.Backend.Reset()
}
