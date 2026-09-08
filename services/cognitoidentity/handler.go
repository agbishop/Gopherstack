package cognitoidentity

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
	cognitoIdentityTargetPrefix = "AWSCognitoIdentityService."
	contentType                 = "application/x-amz-json-1.1"
)

var errUnknownAction = errors.New("UnknownOperationException")

// Handler is the Echo HTTP handler for Cognito Identity Pool operations.
type Handler struct {
	Backend *InMemoryBackend
	ops     map[string]service.JSONOpFunc
	region  string
}

// NewHandler creates a new Cognito Identity handler.
func NewHandler(backend *InMemoryBackend, region string) *Handler {
	h := &Handler{Backend: backend, region: region}
	h.ops = h.buildOps()

	return h
}

// Reset clears all backend state and rebuilds the dispatch table.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "CognitoIdentity" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateIdentityPool",
		"DeleteIdentityPool",
		"DescribeIdentityPool",
		"ListIdentityPools",
		"UpdateIdentityPool",
		"GetId",
		"GetCredentialsForIdentity",
		"GetOpenIdToken",
		"SetIdentityPoolRoles",
		"GetIdentityPoolRoles",
		"DeleteIdentities",
		"DescribeIdentity",
		"GetOpenIdTokenForDeveloperIdentity",
		"GetPrincipalTagAttributeMap",
		"ListIdentities",
		"ListTagsForResource",
		"LookupDeveloperIdentity",
		"MergeDeveloperIdentities",
		"SetPrincipalTagAttributeMap",
		"TagResource",
		"UnlinkDeveloperIdentity",
		"UnlinkIdentity",
		"UntagResource",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "cognito-identity" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Cognito Identity instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.region} }

// RouteMatcher returns a function that matches Cognito Identity requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), cognitoIdentityTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the Cognito Identity action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, cognitoIdentityTargetPrefix)

	if action == "" || action == target {
		return "Unknown"
	}

	return action
}

// ExtractResource extracts the identity pool or identity resource from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var req struct {
		IdentityPoolID string `json:"IdentityPoolId"`
		IdentityID     string `json:"IdentityId"`
	}

	_ = json.Unmarshal(body, &req)

	if req.IdentityPoolID != "" {
		return req.IdentityPoolID
	}

	return req.IdentityID
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		// Resolve the per-request region (from SigV4 / X-Amz-Region) and attach
		// it to the context so backend operations are region-scoped.
		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())

		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"AWSCognitoIdentityService", contentType,
			h.GetSupportedOperations(),
			func(ctx context.Context, action string, body []byte) ([]byte, error) {
				return h.dispatch(context.WithValue(ctx, regionContextKey{}, region), action, body)
			},
			h.handleError,
		)
	}
}

func (h *Handler) buildOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"CreateIdentityPool":                 service.WrapOp(h.handleCreateIdentityPool),
		"DeleteIdentityPool":                 service.WrapOp(h.handleDeleteIdentityPool),
		"DescribeIdentityPool":               service.WrapOp(h.handleDescribeIdentityPool),
		"ListIdentityPools":                  service.WrapOp(h.handleListIdentityPools),
		"UpdateIdentityPool":                 service.WrapOp(h.handleUpdateIdentityPool),
		"GetId":                              service.WrapOp(h.handleGetID),
		"GetCredentialsForIdentity":          service.WrapOp(h.handleGetCredentialsForIdentity),
		"GetOpenIdToken":                     service.WrapOp(h.handleGetOpenIDToken),
		"SetIdentityPoolRoles":               service.WrapOp(h.handleSetIdentityPoolRoles),
		"GetIdentityPoolRoles":               service.WrapOp(h.handleGetIdentityPoolRoles),
		"DeleteIdentities":                   service.WrapOp(h.handleDeleteIdentities),
		"DescribeIdentity":                   service.WrapOp(h.handleDescribeIdentity),
		"GetOpenIdTokenForDeveloperIdentity": service.WrapOp(h.handleGetOpenIDTokenForDeveloperIdentity),
		"GetPrincipalTagAttributeMap":        service.WrapOp(h.handleGetPrincipalTagAttributeMap),
		"ListIdentities":                     service.WrapOp(h.handleListIdentities),
		"ListTagsForResource":                service.WrapOp(h.handleListTagsForResource),
		"LookupDeveloperIdentity":            service.WrapOp(h.handleLookupDeveloperIdentity),
		"MergeDeveloperIdentities":           service.WrapOp(h.handleMergeDeveloperIdentities),
		"SetPrincipalTagAttributeMap":        service.WrapOp(h.handleSetPrincipalTagAttributeMap),
		"TagResource":                        service.WrapOp(h.handleTagResource),
		"UnlinkDeveloperIdentity":            service.WrapOp(h.handleUnlinkDeveloperIdentity),
		"UnlinkIdentity":                     service.WrapOp(h.handleUnlinkIdentity),
		"UntagResource":                      service.WrapOp(h.handleUntagResource),
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
	errType, statusCode := resolveErrorType(err)

	return c.JSON(statusCode, service.JSONErrorResponse{
		Type:    errType,
		Message: err.Error(),
	})
}

// cognitoIdentitySentinelErrors maps sentinel errors to their AWS exception type names.
var cognitoIdentitySentinelErrors = []struct { //nolint:gochecknoglobals // package-level lookup table
	sentinel error
	typeName string
}{
	{ErrIdentityPoolNotFound, ErrIdentityPoolNotFound.Error()},
	{ErrIdentityPoolAlreadyExists, ErrIdentityPoolAlreadyExists.Error()},
	{ErrInvalidParameter, ErrInvalidParameter.Error()},
	{ErrNotAuthorized, ErrNotAuthorized.Error()},
	{ErrResourceConflict, ErrResourceConflict.Error()},
	{ErrDeveloperUserAlreadyRegistered, ErrDeveloperUserAlreadyRegistered.Error()},
	{ErrInvalidIdentityPoolConfiguration, ErrInvalidIdentityPoolConfiguration.Error()},
	{errUnknownAction, "UnknownOperationException"},
}

func resolveErrorType(err error) (string, int) {
	for _, entry := range cognitoIdentitySentinelErrors {
		if errors.Is(err, entry.sentinel) {
			statusCode := http.StatusBadRequest
			if errors.Is(err, ErrNotAuthorized) {
				statusCode = http.StatusForbidden
			}

			return entry.typeName, statusCode
		}
	}

	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return ErrInvalidParameter.Error(), http.StatusBadRequest
	}

	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return ErrInvalidParameter.Error(), http.StatusBadRequest
	}

	// InternalErrorException is cognitoidentity's modeled generic-server-error wire type
	// (confirmed in aws-sdk-go-v2/service/cognitoidentity deserializers.go's per-operation
	// errorCode switches, which recognize "InternalErrorException" on every operation).
	// The Query/EC2-protocol-style "InternalFailure" used by some other gopherstack
	// services does not match any case there, so a real client would fall back to an
	// untyped smithy API error instead of a typed *types.InternalErrorException.
	return "InternalErrorException", http.StatusInternalServerError
}
