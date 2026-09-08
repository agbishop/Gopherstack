package verifiedpermissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	targetPrefix    = "VerifiedPermissions."
	keyTypeField    = "__type"
	keyMessageField = "message"

	// maxPolicyStoreDescriptionLen is the AWS upper bound on a policy store
	// description (PolicyStoreDescription: max length 150).
	maxPolicyStoreDescriptionLen = 150
)

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request")
)

// Handler is the Echo HTTP handler for Amazon Verified Permissions operations.
type Handler struct {
	Backend StorageBackend
	ops     map[string]service.JSONOpFunc
}

// NewHandler creates a new Verified Permissions handler.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.buildOps()

	return h
}

// Reset clears all Verified Permissions state.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "VerifiedPermissions" }

// GetSupportedOperations returns the list of supported Verified Permissions operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"BatchGetPolicy",
		"BatchIsAuthorized",
		"BatchIsAuthorizedWithToken",
		"CreateIdentitySource",
		"CreatePolicy",
		"CreatePolicyStore",
		"CreatePolicyStoreAlias",
		"CreatePolicyTemplate",
		"DeleteIdentitySource",
		"DeletePolicy",
		"DeletePolicyStore",
		"DeletePolicyStoreAlias",
		"DeletePolicyTemplate",
		"GetIdentitySource",
		"GetPolicy",
		"GetPolicyStore",
		"GetPolicyStoreAlias",
		"GetPolicyTemplate",
		"GetSchema",
		"IsAuthorized",
		"IsAuthorizedWithToken",
		"ListIdentitySources",
		"ListPolicies",
		"ListPolicyStoreAliases",
		"ListPolicyStores",
		"ListPolicyTemplates",
		"ListTagsForResource",
		"PutSchema",
		"TagResource",
		"UntagResource",
		"UpdateIdentitySource",
		"UpdatePolicy",
		"UpdatePolicyStore",
		"UpdatePolicyTemplate",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "verifiedpermissions" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function that matches Verified Permissions API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), targetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the Verified Permissions action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, targetPrefix)

	if action == "" || action == target {
		return "Unknown"
	}

	return action
}

// ExtractResource extracts the resource identifier from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var req struct {
		PolicyStoreID    string `json:"policyStoreId"`
		PolicyID         string `json:"policyId"`
		PolicyTemplateID string `json:"policyTemplateId"`
	}
	_ = json.Unmarshal(body, &req)

	if req.PolicyStoreID != "" && req.PolicyID != "" {
		return req.PolicyStoreID + "/" + req.PolicyID
	}

	if req.PolicyStoreID != "" && req.PolicyTemplateID != "" {
		return req.PolicyStoreID + "/" + req.PolicyTemplateID
	}

	return req.PolicyStoreID
}

// Handler returns the Echo handler function for Verified Permissions requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"VerifiedPermissions", "application/x-amz-json-1.0",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

func (h *Handler) buildOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"BatchGetPolicy":             service.WrapOp(h.handleBatchGetPolicy),
		"BatchIsAuthorized":          service.WrapOp(h.handleBatchIsAuthorized),
		"BatchIsAuthorizedWithToken": service.WrapOp(h.handleBatchIsAuthorizedWithToken),
		"CreateIdentitySource":       service.WrapOp(h.handleCreateIdentitySource),
		"CreatePolicyStore":          service.WrapOp(h.handleCreatePolicyStore),
		"CreatePolicyStoreAlias":     service.WrapOp(h.handleCreatePolicyStoreAlias),
		"CreatePolicy":               service.WrapOp(h.handleCreatePolicy),
		"CreatePolicyTemplate":       service.WrapOp(h.handleCreatePolicyTemplate),
		"DeleteIdentitySource":       service.WrapOp(h.handleDeleteIdentitySource),
		"DeletePolicyStore":          service.WrapOp(h.handleDeletePolicyStore),
		"DeletePolicyStoreAlias":     service.WrapOp(h.handleDeletePolicyStoreAlias),
		"DeletePolicy":               service.WrapOp(h.handleDeletePolicy),
		"DeletePolicyTemplate":       service.WrapOp(h.handleDeletePolicyTemplate),
		"GetIdentitySource":          service.WrapOp(h.handleGetIdentitySource),
		"GetPolicyStore":             service.WrapOp(h.handleGetPolicyStore),
		"GetPolicyStoreAlias":        service.WrapOp(h.handleGetPolicyStoreAlias),
		"GetPolicy":                  service.WrapOp(h.handleGetPolicy),
		"GetPolicyTemplate":          service.WrapOp(h.handleGetPolicyTemplate),
		"GetSchema":                  service.WrapOp(h.handleGetSchema),
		"IsAuthorized":               service.WrapOp(h.handleIsAuthorized),
		"IsAuthorizedWithToken":      service.WrapOp(h.handleIsAuthorizedWithToken),
		"ListIdentitySources":        service.WrapOp(h.handleListIdentitySources),
		"ListPolicyStores":           service.WrapOp(h.handleListPolicyStores),
		"ListPolicyStoreAliases":     service.WrapOp(h.handleListPolicyStoreAliases),
		"ListPolicies":               service.WrapOp(h.handleListPolicies),
		"ListPolicyTemplates":        service.WrapOp(h.handleListPolicyTemplates),
		"ListTagsForResource":        service.WrapOp(h.handleListTagsForResource),
		"PutSchema":                  service.WrapOp(h.handlePutSchema),
		"TagResource":                service.WrapOp(h.handleTagResource),
		"UntagResource":              service.WrapOp(h.handleUntagResource),
		"UpdateIdentitySource":       service.WrapOp(h.handleUpdateIdentitySource),
		"UpdatePolicyStore":          service.WrapOp(h.handleUpdatePolicyStore),
		"UpdatePolicy":               service.WrapOp(h.handleUpdatePolicy),
		"UpdatePolicyTemplate":       service.WrapOp(h.handleUpdatePolicyTemplate),
	}
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
	}

	result, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.Is(err, awserr.ErrNotFound):
		return c.JSON(http.StatusBadRequest, map[string]string{
			keyTypeField:    "ResourceNotFoundException",
			keyMessageField: err.Error(),
		})
	case errors.Is(err, ErrPolicyStoreDeletionProtected):
		return c.JSON(http.StatusBadRequest, map[string]string{
			keyTypeField:    "InvalidStateException",
			keyMessageField: err.Error(),
		})
	case errors.Is(err, awserr.ErrConflict):
		return c.JSON(http.StatusBadRequest, map[string]string{
			keyTypeField:    "ConflictException",
			keyMessageField: err.Error(),
		})
	case errors.Is(err, errUnknownAction):
		return c.JSON(http.StatusBadRequest, map[string]string{
			keyTypeField:    "UnknownOperationException",
			keyMessageField: err.Error(),
		})
	case errors.Is(err, ErrTooManyTags):
		return c.JSON(http.StatusBadRequest, map[string]string{
			keyTypeField:    "TooManyTagsException",
			keyMessageField: err.Error(),
		})
	case errors.Is(err, awserr.ErrInvalidParameter), errors.Is(err, errInvalidRequest),
		errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return c.JSON(http.StatusBadRequest, map[string]string{
			keyTypeField:    "ValidationException",
			keyMessageField: err.Error(),
		})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{
			keyTypeField:    "InternalServerException",
			keyMessageField: err.Error(),
		})
	}
}
