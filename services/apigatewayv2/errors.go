package apigatewayv2

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// errTypeHeader is the HTTP response header AWS uses to carry the modeled error
// type for the API Gateway v2 REST-JSON protocol. Clients (and the AWS SDKs)
// read this header to map a response to a concrete exception type rather than
// relying on the HTTP status code alone. The value is the canonical MIME form
// of AWS's "x-amzn-ErrorType" (header names are case-insensitive on the wire
// and Go canonicalises them to this casing).
const errTypeHeader = "X-Amzn-Errortype"

// AWS API Gateway v2 modeled exception type names, keyed by the HTTP status code
// they map to. These are the names AWS returns in the X-Amzn-ErrorType header.
const (
	errTypeBadRequest       = "BadRequestException"
	errTypeUnauthorized     = "UnauthorizedException"
	errTypeAccessDenied     = "AccessDeniedException"
	errTypeNotFound         = "NotFoundException"
	errTypeMethodNotAllowed = "MethodNotAllowedException"
	errTypeConflict         = "ConflictException"
	errTypeTooManyRequests  = "TooManyRequestsException"
	errTypeInternal         = "InternalServerErrorException"
)

// errorTypeForStatus returns the AWS modeled exception type name for an HTTP
// status code. API Gateway v2 uses a stable status→exception mapping, so the
// error type can be derived from the status code deterministically.
func errorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return errTypeBadRequest
	case http.StatusUnauthorized:
		return errTypeUnauthorized
	case http.StatusForbidden:
		return errTypeAccessDenied
	case http.StatusNotFound:
		return errTypeNotFound
	case http.StatusMethodNotAllowed:
		return errTypeMethodNotAllowed
	case http.StatusConflict:
		return errTypeConflict
	case http.StatusTooManyRequests:
		return errTypeTooManyRequests
	default:
		return errTypeInternal
	}
}

// writeErr writes an AWS-shaped error response: a {"message": ...} JSON body
// plus the X-Amzn-ErrorType header carrying the modeled exception type derived
// from the HTTP status. This mirrors what real API Gateway v2 returns so SDK
// clients can classify the fault.
func writeErr(c *echo.Context, status int, message string) error {
	return writeErrType(c, status, errorTypeForStatus(status), message)
}

// writeErrType is like writeErr but with an explicit error-type name, for the
// rare cases where the exception type does not follow the default status
// mapping (e.g. an AccessDeniedException returned with a non-403 status).
func writeErrType(c *echo.Context, status int, errType, message string) error {
	if errType != "" {
		c.Response().Header().Set(errTypeHeader, errType)
	}

	return c.JSON(status, notFoundResponse{Message: message})
}

var (
	// ErrAPINotFound is returned when a requested API does not exist.
	ErrAPINotFound = errors.New("NotFoundException")
	// ErrStageNotFound is returned when a requested stage does not exist.
	ErrStageNotFound = errors.New("NotFoundException")
	// ErrRouteNotFound is returned when a requested route does not exist.
	ErrRouteNotFound = errors.New("NotFoundException")
	// ErrIntegrationNotFound is returned when a requested integration does not exist.
	ErrIntegrationNotFound = errors.New("NotFoundException")
	// ErrDeploymentNotFound is returned when a requested deployment does not exist.
	ErrDeploymentNotFound = errors.New("NotFoundException")
	// ErrAuthorizerNotFound is returned when a requested authorizer does not exist.
	ErrAuthorizerNotFound = errors.New("NotFoundException")
	// ErrDomainNameNotFound is returned when a requested domain name does not exist.
	ErrDomainNameNotFound = errors.New("NotFoundException")
	// ErrAPIMappingNotFound is returned when a requested API mapping does not exist.
	ErrAPIMappingNotFound = errors.New("NotFoundException")
	// ErrIntegrationResponseNotFound is returned when a requested integration response does not exist.
	ErrIntegrationResponseNotFound = errors.New("NotFoundException")
	// ErrModelNotFound is returned when a requested model does not exist.
	ErrModelNotFound = errors.New("NotFoundException")
	// ErrRouteResponseNotFound is returned when a requested route response does not exist.
	ErrRouteResponseNotFound = errors.New("NotFoundException")
	// ErrPortalNotFound is returned when a requested portal does not exist.
	ErrPortalNotFound = errors.New("NotFoundException")
	// ErrPortalProductNotFound is returned when a requested portal product does not exist.
	ErrPortalProductNotFound = errors.New("NotFoundException")
	// ErrBadRequest is returned when required fields are missing or invalid.
	ErrBadRequest = errors.New("BadRequestException")
	// ErrAlreadyExists is returned when a resource with the same identifier already exists.
	ErrAlreadyExists = awserr.New("ConflictException", awserr.ErrAlreadyExists)
	// ErrProductPageNotFound is returned when a requested product page does not exist.
	ErrProductPageNotFound = errors.New("NotFoundException")
	// ErrProductREPageNotFound is returned when a requested product REST endpoint page does not exist.
	ErrProductREPageNotFound = errors.New("NotFoundException")
	// ErrVpcLinkNotFound is returned when a requested VPC link does not exist.
	ErrVpcLinkNotFound = errors.New("NotFoundException")
	// ErrRoutingRuleNotFound is returned when a requested routing rule does not exist.
	ErrRoutingRuleNotFound = errors.New("NotFoundException")
	// ErrThrottled is returned by the data plane when a route exceeds its
	// configured throttling rate/burst limit.
	ErrThrottled = errors.New("TooManyRequestsException")
)

// Route-control rejection reasons for the HTTP API data plane. The
// enforce*/apply* chain (http_proxy.go, authorizers.go) returns these
// unwritten so handleHTTPAPIProxy can map and write the response exactly
// once via writeRouteControlRejection, instead of each helper writing its
// own response and returning nil -- which let every checked
// "if ctrlErr != nil" caller in the chain mistake a written rejection for
// success and forward the request to the integration anyway
// (gopherstack-wsvb, the gopherstack-8haq/246v shape).
var (
	errRouteUnauthorized      = errors.New("route: unauthorized")
	errRouteForbidden         = errors.New("route: forbidden")
	errRouteExplicitDeny      = errors.New("route: explicit deny")
	errRouteMissingAuthToken  = errors.New("route: missing authentication token")
	errRouteAuthConfigInvalid = errors.New("route: authorizer configuration invalid")
)

// writeRouteControlRejection maps an unwritten route-control rejection error
// (from applyRouteControls) to its AWS-accurate status/body and writes it
// exactly once.
func writeRouteControlRejection(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrThrottled):
		return writeErr(c, http.StatusTooManyRequests, "Too Many Requests")
	case errors.Is(err, errRouteUnauthorized):
		return writeErr(c, http.StatusUnauthorized, msgUnauthorized)
	case errors.Is(err, errRouteMissingAuthToken):
		return writeErr(c, http.StatusForbidden, msgMissingAuthToken)
	case errors.Is(err, errRouteExplicitDeny):
		return writeErr(c, http.StatusForbidden, msgExplicitDeny)
	case errors.Is(err, errRouteForbidden):
		return writeErr(c, http.StatusForbidden, msgForbidden)
	case errors.Is(err, errRouteAuthConfigInvalid):
		return writeErr(c, http.StatusInternalServerError, msgAuthConfigInvalid)
	default:
		return c.String(http.StatusInternalServerError, "Internal Server Error")
	}
}
