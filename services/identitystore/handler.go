package identitystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyNextToken       = "NextToken"
	keyIdentityStoreID = "IdentityStoreId"
	keyErrType         = "__type"
	keyErrMessage      = "message"
)

const (
	// targetPrefix is the X-Amz-Target prefix for the Identity Store JSON protocol.
	targetPrefix = "AWSIdentityStore."
	// isMemberInGroupsOp is the operation name for the IsMemberInGroups API call.
	isMemberInGroupsOp = "IsMemberInGroups"
)

// Operation name constants for Identity Store.
const (
	opCreateUser                    = "CreateUser"
	opDescribeUser                  = "DescribeUser"
	opListUsers                     = "ListUsers"
	opUpdateUser                    = "UpdateUser"
	opDeleteUser                    = "DeleteUser"
	opGetUserID                     = "GetUserId"
	opCreateGroup                   = "CreateGroup"
	opDescribeGroup                 = "DescribeGroup"
	opListGroups                    = "ListGroups"
	opUpdateGroup                   = "UpdateGroup"
	opDeleteGroup                   = "DeleteGroup"
	opGetGroupID                    = "GetGroupId"
	opCreateGroupMembership         = "CreateGroupMembership"
	opDescribeGroupMembership       = "DescribeGroupMembership"
	opListGroupMemberships          = "ListGroupMemberships"
	opDeleteGroupMembership         = "DeleteGroupMembership"
	opListGroupMembershipsForMember = "ListGroupMembershipsForMember"
	opGetGroupMembershipID          = "GetGroupMembershipId"
)

// Handler is the Echo HTTP handler for the Identity Store REST API.
type Handler struct {
	Backend *InMemoryBackend
}

// NewHandler creates a new Identity Store handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "IdentityStore" }

// GetSupportedOperations returns the list of supported Identity Store operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateUser,
		opDescribeUser,
		opListUsers,
		opUpdateUser,
		opDeleteUser,
		opGetUserID,
		opCreateGroup,
		opDescribeGroup,
		opListGroups,
		opUpdateGroup,
		opDeleteGroup,
		opGetGroupID,
		opCreateGroupMembership,
		opDescribeGroupMembership,
		opListGroupMemberships,
		opDeleteGroupMembership,
		opGetGroupMembershipID,
		opListGroupMembershipsForMember,
		"IsMemberInGroups",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "identitystore" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Identity Store instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches Identity Store JSON protocol requests.
// The SDK uses X-Amz-Target: AWSIdentityStore.{Operation} with POST to /.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), targetPrefix)
	}
}

// MatchPriority returns the routing priority.
// Uses PriorityHeaderExact since matching is by X-Amz-Target header.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the Identity Store operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return strings.TrimPrefix(c.Request().Header.Get("X-Amz-Target"), targetPrefix)
}

// ExtractResource extracts the IdentityStoreId from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var req struct {
		IdentityStoreID string `json:"IdentityStoreId"`
	}

	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		return ""
	}

	return req.IdentityStoreID
}

// Handler returns the Echo handler function for Identity Store requests.
// The Identity Store SDK uses the JSON 1.1 protocol: POST / with X-Amz-Target header.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()

		// Resolve per-request region from SigV4 credential scope or X-Amz-Region,
		// then attach it to the context so backend operations are region-scoped.
		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
		ctx = context.WithValue(ctx, regionContextKey{}, region)

		log := logger.Load(ctx)

		target := c.Request().Header.Get("X-Amz-Target")
		op := strings.TrimPrefix(target, targetPrefix)
		if op == "" || op == target {
			return h.writeError(c, http.StatusBadRequest, "UnrecognizedClientException", "missing X-Amz-Target header")
		}

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "identitystore: failed to read request body", "error", err)

			return h.writeError(c, http.StatusInternalServerError, "InternalFailure", "failed to read request body")
		}

		log.DebugContext(ctx, "identitystore request", "op", op)

		if dispatchErr := h.dispatch(ctx, c, op, body); dispatchErr != nil {
			if errors.Is(dispatchErr, errResponseWritten) {
				return nil
			}

			return dispatchErr
		}

		return nil
	}
}

// ----------------------------------------
// Shared request/response types
// ----------------------------------------

// attributeOperation is a single UpdateUser/UpdateGroup attribute mutation.
type attributeOperation struct {
	AttributeValue any    `json:"AttributeValue"`
	AttributePath  string `json:"AttributePath"`
}

// alternateIdentifier is the shared GetUserId/GetGroupId lookup key shape.
type alternateIdentifier struct {
	UniqueAttribute *uniqueAttribute `json:"UniqueAttribute"`
	ExternalID      *externalID      `json:"ExternalId"`
}

type uniqueAttribute struct {
	AttributePath  string `json:"AttributePath"`
	AttributeValue string `json:"AttributeValue"`
}

type externalID struct {
	Issuer string `json:"Issuer"`
	ID     string `json:"Id"`
}

// ----------------------------------------
// Dispatch
// ----------------------------------------

// identityStoreDispatch maps operation names to their handler functions.
//
//nolint:gochecknoglobals // read-only dispatch table initialized once at startup
var identityStoreDispatch = map[string]func(*Handler, context.Context, *echo.Context, []byte) error{
	// User operations
	opCreateUser:   (*Handler).handleCreateUser,
	opDescribeUser: (*Handler).handleDescribeUser,
	opListUsers:    (*Handler).handleListUsers,
	opUpdateUser:   (*Handler).handleUpdateUser,
	opDeleteUser:   (*Handler).handleDeleteUser,
	opGetUserID:    (*Handler).handleGetUserID,
	// Group operations
	opCreateGroup:   (*Handler).handleCreateGroup,
	opDescribeGroup: (*Handler).handleDescribeGroup,
	opListGroups:    (*Handler).handleListGroups,
	opUpdateGroup:   (*Handler).handleUpdateGroup,
	opDeleteGroup:   (*Handler).handleDeleteGroup,
	opGetGroupID:    (*Handler).handleGetGroupID,
	// Membership operations
	opCreateGroupMembership:         (*Handler).handleCreateGroupMembership,
	opDescribeGroupMembership:       (*Handler).handleDescribeGroupMembership,
	opListGroupMemberships:          (*Handler).handleListGroupMemberships,
	opDeleteGroupMembership:         (*Handler).handleDeleteGroupMembership,
	opGetGroupMembershipID:          (*Handler).handleGetGroupMembershipID,
	opListGroupMembershipsForMember: (*Handler).handleListGroupMembershipsForMember,
	isMemberInGroupsOp:              (*Handler).handleIsMemberInGroups,
}

func (h *Handler) dispatch(ctx context.Context, c *echo.Context, op string, body []byte) error {
	if fn, ok := identityStoreDispatch[op]; ok {
		return fn(h, ctx, c, body)
	}

	return h.writeError(c, http.StatusBadRequest, "UnrecognizedClientException",
		"operation "+op+" is not supported")
}

// ----------------------------------------
// Shared validation helpers
// ----------------------------------------

// errMaxResultsOutOfRange is returned when a list MaxResults value falls
// outside the AWS-permitted 1-100 range.
var errMaxResultsOutOfRange = fmt.Errorf("MaxResults must be between 1 and %d", maxListPageSize)

// validateMaxResults enforces the AWS Identity Store list MaxResults bound.
// MaxResults is optional (0 = unset); when supplied it must be 1-100.
func validateMaxResults(maxResults int32) error {
	if maxResults == 0 {
		return nil
	}

	if maxResults < 1 || maxResults > maxListPageSize {
		return errMaxResultsOutOfRange
	}

	return nil
}

// maxOperationsPerUpdate is the AWS-modeled upper bound on the number of
// AttributeOperation entries in a single UpdateUser/UpdateGroup request (the
// shared AttributeOperations smithy list shape has min:1/max:100).
const maxOperationsPerUpdate = 100

// errOperationsOutOfRange is returned when UpdateUser/UpdateGroup's
// Operations list is empty or exceeds the AWS-permitted 1-100 bound.
var errOperationsOutOfRange = fmt.Errorf(
	"operations must contain between 1 and %d items", maxOperationsPerUpdate,
)

// validateOperations enforces the AWS Identity Store UpdateUser/UpdateGroup
// Operations bound. Operations is a required member with min:1/max:100 on
// the real AttributeOperations shape.
func validateOperations(ops []attributeOperation) error {
	if len(ops) == 0 || len(ops) > maxOperationsPerUpdate {
		return errOperationsOutOfRange
	}

	return nil
}

// alternateIDResult holds the parsed fields from an alternate-identifier request.
type alternateIDResult struct {
	storeID   string
	attrPath  string
	attrValue string
}

// parseAlternateIDRequest decodes a request body that contains IdentityStoreId and
// AlternateIdentifier, validates both are present, and returns the parsed values.
func (h *Handler) parseAlternateIDRequest(c *echo.Context, body []byte) (alternateIDResult, error) {
	var req struct {
		AlternateIdentifier alternateIdentifier `json:"AlternateIdentifier"`
		IdentityStoreID     string              `json:"IdentityStoreId"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		_ = h.writeError(c, http.StatusBadRequest, "ValidationException", "invalid request body")

		return alternateIDResult{}, errResponseWritten
	}

	if err := h.requireIdentityStoreID(c, req.IdentityStoreID); err != nil {
		return alternateIDResult{}, err
	}

	attrPath, attrValue := extractAlternateIdentifier(req.AlternateIdentifier)
	if attrPath == "" {
		_ = h.writeError(c, http.StatusBadRequest, "ValidationException", "AlternateIdentifier is required")

		return alternateIDResult{}, errResponseWritten
	}

	// Only UniqueAttribute.AttributePath is client-controlled free text; the
	// ExternalId branch of extractAlternateIdentifier hardcodes attrPath to
	// the literal "ExternalId", so it never needs an AttributePath pattern
	// check -- but its Issuer/Id values still need their own pattern check.
	if req.AlternateIdentifier.UniqueAttribute != nil {
		const fieldName = "AlternateIdentifier.UniqueAttribute.AttributePath"
		if err := validatePattern(patternAttributePath, fieldName, attrPath); err != nil {
			_ = h.writeError(c, http.StatusBadRequest, "ValidationException", err.Error())

			return alternateIDResult{}, errResponseWritten
		}
	}

	if ext := req.AlternateIdentifier.ExternalID; ext != nil {
		if err := validateExternalIDs([]ExternalID{{Issuer: ext.Issuer, ID: ext.ID}}); err != nil {
			_ = h.writeError(c, http.StatusBadRequest, "ValidationException", err.Error())

			return alternateIDResult{}, errResponseWritten
		}
	}

	return alternateIDResult{storeID: req.IdentityStoreID, attrPath: attrPath, attrValue: attrValue}, nil
}

// externalIDSep is the separator used to encode an ExternalId compound key as a single string.
// Null byte is used because it cannot appear in valid Issuer or ID values (both are typically URLs or UUIDs).
const externalIDSep = "\x00"

// extractAlternateIdentifier extracts the attribute path and value from an AlternateIdentifier.
// For ExternalId, both Issuer and Id are encoded as a compound value separated by externalIDSep.
func extractAlternateIdentifier(ai alternateIdentifier) (string, string) {
	if ai.UniqueAttribute != nil {
		return ai.UniqueAttribute.AttributePath, ai.UniqueAttribute.AttributeValue
	}

	if ai.ExternalID != nil {
		return "ExternalId", ai.ExternalID.Issuer + externalIDSep + ai.ExternalID.ID
	}

	return "", ""
}

// ----------------------------------------
// Error handling helpers
// ----------------------------------------

func (h *Handler) handleBackendError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrUserNotFound):

		return h.writeResourceError(c, "ResourceNotFoundException", err.Error(), "USER")
	case errors.Is(err, ErrGroupNotFound):

		return h.writeResourceError(c, "ResourceNotFoundException", err.Error(), "GROUP")
	case errors.Is(err, ErrMembershipNotFound):

		return h.writeResourceError(c, "ResourceNotFoundException", err.Error(), "GROUP_MEMBERSHIP")
	case errors.Is(err, ErrConflict):

		return h.writeConflictError(c, err.Error())
	case errors.Is(err, ErrValidation):

		return h.writeError(c, http.StatusBadRequest, "ValidationException", err.Error())
	}

	// InternalServerException is the real Identity Store smithy model's
	// modeled internal-error shape (types.InternalServerException); unlike
	// some other gopherstack services this backend never actually returns an
	// unmapped error today (every sentinel above is exhaustive), so this is
	// a defensive fallback rather than a reachable path.
	return h.writeError(c, http.StatusInternalServerError, "InternalServerException", err.Error())
}

// writeResourceError writes a ResourceNotFoundException with a ResourceType field for
// AWS-compatible error responses.
func (h *Handler) writeResourceError(c *echo.Context, errType, message, resourceType string) error {
	return c.JSON(http.StatusNotFound, map[string]string{
		keyErrType:     errType,
		keyErrMessage:  message,
		"ResourceType": resourceType,
	})
}

// writeConflictError writes a ConflictException with a Reason field for
// AWS-compatible error responses. Every ErrConflict this backend raises today
// is a duplicate-value rejection (UserName, primary email, DisplayName, or a
// group membership's (group, member) pair), never a concurrent-modification
// race, so Reason is always UNIQUENESS_CONSTRAINT_VIOLATION -- the only other
// modeled ConflictExceptionReason value is CONCURRENT_MODIFICATION (see
// types/enums.go), which no code path here produces. Deserializers.go's
// awsAwsjson11_deserializeDocumentConflictException parses a top-level
// "Reason" field; omitting it (the previous behavior) left every real SDK
// caller's err.(*types.ConflictException).Reason empty instead of set.
func (h *Handler) writeConflictError(c *echo.Context, message string) error {
	return c.JSON(http.StatusConflict, map[string]string{
		keyErrType:    "ConflictException",
		keyErrMessage: message,
		"Reason":      "UNIQUENESS_CONSTRAINT_VIOLATION",
	})
}

func (h *Handler) writeError(c *echo.Context, statusCode int, errType, message string) error {
	return c.JSON(statusCode, map[string]string{
		keyErrType:    errType,
		keyErrMessage: message,
	})
}
