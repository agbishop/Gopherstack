package apigatewayv2

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"gopkg.in/yaml.v3"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

func extractAPIsOp(path, method string) string {
	segs := pathSegments(path)
	nsegs := len(segs)

	var seg1 string

	switch {
	case nsegs == segCountDeepRes:
		// e.g. ["{apiId}", "integrations", "{id}", "integrationresponses", "{responseId}"]
		seg1 = segs[segCountDeepColl-1]
	case nsegs == segCountDeepColl:
		// pathSegments strips the /v2/apis prefix, so segs[3] is the 4th element
		// (0-indexed last) of e.g. ["{apiId}", "integrations", "{id}", "integrationresponses"].
		seg1 = segs[segCountDeepColl-1]
	case nsegs >= segCountSubColl:
		seg1 = segs[segCountSubColl-1]
	}

	key := operationKey{segs: nsegs, seg1: seg1, method: method}

	if op, ok := onceOpTable()[key]; ok {
		return op
	}

	return opUnknown
}

// handleAPIsPath dispatches requests under the /v2/apis prefix.
func (h *Handler) handleAPIsPath(c *echo.Context, method, path string) error {
	segs := pathSegments(path)

	switch len(segs) {
	case segCountAPIs:
		return h.handleAPIs(c, method)
	case segCountAPIByID:
		return h.handleAPI(c, method, segs[0])
	case segCountSubColl:
		return h.handleSubCollection(c, method, segs[0], segs[1])
	case segCountSubRes:
		return h.handleSubResource(c, method, segs[0], segs[1], segs[2])
	case segCountDeepColl:
		return h.handleDeepCollection(c, method, segs[0], segs[1], segs[2], segs[3])
	case segCountDeepRes:
		return h.handleDeepResource(c, method, segs[0], segs[1], segs[2], segs[3], segs[4])
	default:
		logger.Load(c.Request().Context()).Warn("apigatewayv2: unhandled path", "path", path)

		return writeErr(c, http.StatusNotFound, msgNotFound)
	}
}

// handleAPIs handles POST /v2/apis and GET /v2/apis.
func (h *Handler) handleAPIs(c *echo.Context, method string) error {
	switch method {
	case http.MethodPost:
		return h.handleCreateAPI(c)
	case http.MethodGet:
		return h.handleGetAPIs(c)
	case http.MethodPut:
		return h.handleImportAPI(c)
	default:
		return writeErr(c, http.StatusMethodNotAllowed, msgMethodNotAllowed)
	}
}

// handleAPI handles /v2/apis/{apiId}.
func (h *Handler) handleAPI(c *echo.Context, method, apiID string) error {
	switch method {
	case http.MethodGet:
		return h.handleGetAPI(c, apiID)
	case http.MethodDelete:
		return h.handleDeleteAPI(c, apiID)
	case http.MethodPatch:
		return h.handleUpdateAPI(c, apiID)
	case http.MethodPut:
		return h.handleReimportAPI(c, apiID)
	default:
		return writeErr(c, http.StatusMethodNotAllowed, msgMethodNotAllowed)
	}
}

func (h *Handler) handleCreateAPI(c *echo.Context) error {
	log := logger.Load(c.Request().Context())

	var input CreateAPIInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return writeErr(c, http.StatusBadRequest, msgInvalidBody)
	}

	api, err := h.Backend.CreateAPI(c.Request().Context(), input)
	if err != nil {
		log.Error("apigatewayv2: create api failed", "error", err)

		if errors.Is(err, ErrBadRequest) {
			return writeErr(c, http.StatusBadRequest, err.Error())
		}

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, api)
}

func (h *Handler) handleGetAPIs(c *echo.Context) error {
	log := logger.Load(c.Request().Context())

	apis, err := h.Backend.GetAPIs()
	if err != nil {
		log.Error("apigatewayv2: get apis failed", "error", err)

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	maxResults, nextToken := apigwPaginationParams(c)
	p := page.New(apis, nextToken, maxResults, apigwDefaultPageSize)

	return c.JSON(http.StatusOK, listApisOutput{Items: p.Data, NextToken: p.Next})
}

func (h *Handler) handleGetAPI(c *echo.Context, apiID string) error {
	log := logger.Load(c.Request().Context())

	api, err := h.Backend.GetAPI(apiID)
	if err != nil {
		log.Error("apigatewayv2: get api failed", logKeyAPIID, apiID, "error", err)

		if errors.Is(err, ErrAPINotFound) {
			return writeErr(c, http.StatusNotFound, msgNotFound)
		}

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, api)
}

func (h *Handler) handleDeleteAPI(c *echo.Context, apiID string) error {
	log := logger.Load(c.Request().Context())

	// Snapshot the API's authorizer IDs before the cascade delete removes
	// them, so any cached decisions for those authorizers can be purged
	// afterward (bd: gopherstack-wmh). A lookup failure here just means
	// nothing to purge -- DeleteAPI below still enforces ErrAPINotFound.
	authorizers, _ := h.Backend.GetAuthorizers(apiID)

	if err := h.Backend.DeleteAPI(apiID); err != nil {
		log.Error("apigatewayv2: delete api failed", logKeyAPIID, apiID, "error", err)

		if errors.Is(err, ErrAPINotFound) {
			return writeErr(c, http.StatusNotFound, msgNotFound)
		}

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	if h.authCache != nil {
		for _, a := range authorizers {
			h.authCache.purge(a.AuthorizerID)
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleUpdateAPI(c *echo.Context, apiID string) error {
	log := logger.Load(c.Request().Context())

	var input UpdateAPIInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return writeErr(c, http.StatusBadRequest, msgInvalidBody)
	}

	api, err := h.Backend.UpdateAPI(apiID, input)
	if err != nil {
		log.Error("apigatewayv2: update api failed", logKeyAPIID, apiID, "error", err)

		if errors.Is(err, ErrAPINotFound) {
			return writeErr(c, http.StatusNotFound, msgNotFound)
		}

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, api)
}

// openAPISpec is a minimal representation of an OpenAPI 3 / Swagger 2 document.
// Only the fields needed to derive an API name, routes, and integrations are
// modeled; unknown fields are ignored so that minimal specs are tolerated.
type openAPISpec struct {
	Paths    map[string]map[string]openAPIOperation `json:"paths"`
	Info     openAPIInfo                            `json:"info"`
	BasePath string                                 `json:"basePath"` // Swagger 2
	Servers  []openAPIServer                        `json:"servers"`  // OpenAPI 3
}

type openAPIInfo struct {
	Title string `json:"title"`
}

// openAPIServer is the OpenAPI 3 servers[] entry; its URL's path component is
// the OpenAPI-3 equivalent of Swagger 2's top-level basePath.
type openAPIServer struct {
	URL string `json:"url"`
}

const (
	basepathIgnore  = "ignore"
	basepathPrepend = "prepend"
	basepathSplit   = "split"
)

// validateBasepath returns ErrBadRequest unless basepath is empty (defaults
// to ignore) or one of the three values ImportApiInput/ReimportApiInput
// document (api_op_ImportApi.go:37-41, aws-sdk-go-v2/service/apigatewayv2@v1.37.4:
// "Valid values are ignore, prepend, and split. The default value is ignore.").
func validateBasepath(basepath string) error {
	switch basepath {
	case "", basepathIgnore, basepathPrepend, basepathSplit:
		return nil
	default:
		return fmt.Errorf("%w: basepath must be one of ignore, prepend, split", ErrBadRequest)
	}
}

// validateFailOnWarnings reads the failOnWarnings query param (ImportApiInput/
// ReimportApiInput, same querystring location as basepath). It is read and
// validated rather than silently dropped, but has no further observable
// effect yet: this emulator's OpenAPI import (parseOpenAPISpec/
// applyOpenAPIToAPI) never generates import warnings for any spec it accepts,
// so there is never a warning for failOnWarnings to escalate -- see
// PARITY.md gaps (matches the precedent set for API.ImportInfo/Warnings,
// which are deliberately never populated with speculative text).
func validateFailOnWarnings(c *echo.Context) error {
	raw := c.Request().URL.Query().Get("failOnWarnings")
	if raw == "" {
		return nil
	}

	if _, err := strconv.ParseBool(raw); err != nil {
		return fmt.Errorf("%w: failOnWarnings must be a boolean", ErrBadRequest)
	}

	return nil
}

// specBasePath returns the OpenAPI document's declared base path: Swagger 2's
// top-level basePath, or the path component of OpenAPI 3's first servers[]
// entry. Returns "" if neither is present or the server URL doesn't parse.
func specBasePath(spec *openAPISpec) string {
	if spec.BasePath != "" {
		return spec.BasePath
	}

	if len(spec.Servers) == 0 {
		return ""
	}

	u, err := url.Parse(spec.Servers[0].URL)
	if err != nil {
		return ""
	}

	return u.Path
}

type openAPIOperation struct {
	Integration *openAPIIntegration `json:"x-amazon-apigateway-integration"`
}

// openAPIIntegration models the x-amazon-apigateway-integration extension.
type openAPIIntegration struct {
	Type                 string `json:"type"`
	HTTPMethod           string `json:"httpMethod"`
	URI                  string `json:"uri"`
	PayloadFormatVersion string `json:"payloadFormatVersion"`
	ConnectionType       string `json:"connectionType"`
	TimeoutInMillis      int32  `json:"timeoutInMillis"`
}

// parseOpenAPISpec decodes an OpenAPI body into an openAPISpec, tolerating
// minimal/partial documents.
func parseOpenAPISpec(body string) (*openAPISpec, error) {
	var spec openAPISpec
	if strings.TrimSpace(body) == "" {
		return &spec, nil
	}

	if err := json.Unmarshal([]byte(body), &spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

// applyOpenAPIToAPI creates a route (and integration, when defined) for each
// path+method in the spec. Entries that are not valid HTTP route keys are
// skipped gracefully. When basepath is "prepend", the document's declared
// base path (specBasePath) is prefixed onto every route path; "split" is not
// implemented (falls back to the "ignore" default, since API Gateway's
// prepend/split base-path semantics aren't described by the SDK wire model,
// only by prose docs -- see PARITY.md gaps).
func (h *Handler) applyOpenAPIToAPI(apiID string, spec *openAPISpec, basepath string) {
	prefix := ""
	if basepath == basepathPrepend {
		prefix = strings.TrimSuffix(specBasePath(spec), "/")
	}

	for path, methods := range spec.Paths {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		if prefix != "" {
			path = prefix + path
		}

		for method, op := range methods {
			h.applyOpenAPIOperation(apiID, path, method, op)
		}
	}
}

// applyOpenAPIOperation creates the route (and integration, when defined) for
// a single path+method entry from an imported OpenAPI spec. Route keys the
// backend would reject (e.g. spec-level fields like "parameters" that share
// the path map) are skipped gracefully.
func (h *Handler) applyOpenAPIOperation(apiID, path, method string, op openAPIOperation) {
	routeKey := strings.ToUpper(method) + " " + path
	if err := validateHTTPRouteKey(routeKey); err != nil {
		return
	}

	var target string

	if op.Integration != nil {
		integ, err := h.Backend.CreateIntegration(apiID, CreateIntegrationInput{
			IntegrationType:      openAPIIntegrationType(op.Integration.Type),
			IntegrationMethod:    op.Integration.HTTPMethod,
			IntegrationURI:       op.Integration.URI,
			PayloadFormatVersion: op.Integration.PayloadFormatVersion,
			ConnectionType:       op.Integration.ConnectionType,
			TimeoutInMillis:      op.Integration.TimeoutInMillis,
		})
		if err == nil && integ != nil {
			target = "integrations/" + integ.IntegrationID
		}
	}

	if _, err := h.Backend.CreateRoute(apiID, CreateRouteInput{
		RouteKey: routeKey,
		Target:   target,
	}); err != nil {
		return
	}
}

// openAPIIntegrationType maps an OpenAPI x-amazon-apigateway-integration
// "type" extension value to the API Gateway v2 IntegrationType wire value.
func openAPIIntegrationType(t string) string {
	switch t {
	case "http", "http_proxy":
		return integrationTypeHTTPProxy
	case "aws", "aws_proxy":
		return IntegrationTypeAWSProxy
	case "mock":
		return integrationTypeMock
	case "":
		return IntegrationTypeAWSProxy
	default:
		return strings.ToUpper(t)
	}
}

func (h *Handler) handleImportAPI(c *echo.Context) error {
	// basepath and failOnWarnings are HTTP query-string params on the real
	// ImportApiInput, not JSON body fields (api_op_ImportApi.go:37-45,
	// aws-sdk-go-v2/service/apigatewayv2@v1.37.4).
	basepath := c.Request().URL.Query().Get("basepath")
	if err := validateBasepath(basepath); err != nil {
		return writeErr(c, http.StatusBadRequest, err.Error())
	}

	if err := validateFailOnWarnings(c); err != nil {
		return writeErr(c, http.StatusBadRequest, err.Error())
	}

	var input struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return writeErr(c, http.StatusBadRequest, msgInvalidBody)
	}

	spec, err := parseOpenAPISpec(input.Body)
	if err != nil {
		return writeErr(c, http.StatusBadRequest, msgInvalidBody)
	}

	name := spec.Info.Title
	if name == "" {
		name = "imported-api"
	}

	api, err := h.Backend.CreateAPI(c.Request().Context(), CreateAPIInput{
		Name:         name,
		ProtocolType: protocolTypeHTTP,
	})
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	h.applyOpenAPIToAPI(api.APIID, spec, basepath)

	return c.JSON(http.StatusCreated, api)
}

func (h *Handler) handleReimportAPI(c *echo.Context, apiID string) error {
	basepath := c.Request().URL.Query().Get("basepath")
	if err := validateBasepath(basepath); err != nil {
		return writeErr(c, http.StatusBadRequest, err.Error())
	}

	if err := validateFailOnWarnings(c); err != nil {
		return writeErr(c, http.StatusBadRequest, err.Error())
	}

	var input struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return writeErr(c, http.StatusBadRequest, msgInvalidBody)
	}

	spec, err := parseOpenAPISpec(input.Body)
	if err != nil {
		return writeErr(c, http.StatusBadRequest, msgInvalidBody)
	}

	// Replace existing routes and integrations from the new spec.
	if routes, rErr := h.Backend.GetRoutes(apiID); rErr == nil {
		for _, r := range routes {
			_ = h.Backend.DeleteRoute(apiID, r.RouteID)
		}
	} else if errors.Is(rErr, ErrAPINotFound) {
		return writeErr(c, http.StatusNotFound, msgNotFound)
	}

	if integrations, iErr := h.Backend.GetIntegrations(apiID); iErr == nil {
		for _, i := range integrations {
			_ = h.Backend.DeleteIntegration(apiID, i.IntegrationID)
		}
	}

	update := UpdateAPIInput{}
	if spec.Info.Title != "" {
		update.Name = spec.Info.Title
	}

	api, err := h.Backend.UpdateAPI(apiID, update)
	if err != nil {
		if errors.Is(err, ErrAPINotFound) {
			return writeErr(c, http.StatusNotFound, msgNotFound)
		}

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	h.applyOpenAPIToAPI(apiID, spec, basepath)

	return c.JSON(http.StatusCreated, api)
}

func (h *Handler) handleDeleteCorsConfiguration(c *echo.Context, apiID string) error {
	log := logger.Load(c.Request().Context())

	if err := h.Backend.DeleteCorsConfiguration(apiID); err != nil {
		log.Error("apigatewayv2: delete cors configuration failed", logKeyAPIID, apiID, "error", err)

		if errors.Is(err, ErrAPINotFound) {
			return writeErr(c, http.StatusNotFound, msgNotFound)
		}

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// apigwExtensionPrefix is the key prefix for AWS API Gateway extensions in an
// exported OpenAPI document (e.g. x-amazon-apigateway-authtype).
const apigwExtensionPrefix = "x-amazon-apigateway-"

// includeExtensions reads ExportApiInput's includeExtensions query param
// (api_op_ExportApi.go:52, "*bool ... included by default"), defaulting to
// true (AWS's documented default) when absent or unparseable.
func includeExtensions(c *echo.Context) bool {
	raw := c.QueryParam("includeExtensions")
	if raw == "" {
		return true
	}

	v, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}

	return v
}

// stripAPIGatewayExtensions recursively removes x-amazon-apigateway-* keys
// from an exported OpenAPI document, for includeExtensions=false.
func stripAPIGatewayExtensions(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}

	stripMapExtensions(m)

	return m
}

func stripMapExtensions(m map[string]any) {
	for k, v := range m {
		if strings.HasPrefix(k, apigwExtensionPrefix) {
			delete(m, k)

			continue
		}

		stripExtensionsIn(v)
	}
}

func stripExtensionsIn(v any) {
	switch t := v.(type) {
	case map[string]any:
		stripMapExtensions(t)
	case []any:
		for _, item := range t {
			stripExtensionsIn(item)
		}
	}
}

func (h *Handler) handleExportAPI(c *echo.Context, apiID, specification string) error {
	// API Gateway v2 only supports the OAS30 specification for exports.
	if specification != "" && specification != "OAS30" {
		return writeErr(c, http.StatusBadRequest, "specification must be OAS30")
	}

	// outputType is a required query param on the real ExportApiInput (verified
	// against aws-sdk-go-v2/service/apigatewayv2's validateOpExportApiInput);
	// valid values are JSON and YAML.
	outputType := c.QueryParam("outputType")

	switch {
	case outputType == "":
		return writeErr(c, http.StatusBadRequest, "outputType is required")
	case strings.EqualFold(outputType, "JSON"):
	case strings.EqualFold(outputType, "YAML"):
	default:
		return writeErr(c, http.StatusBadRequest, "outputType must be JSON or YAML")
	}

	spec, err := h.Backend.ExportAPI(apiID)
	if err != nil {
		if errors.Is(err, ErrAPINotFound) {
			return writeErr(c, http.StatusNotFound, msgNotFound)
		}

		return writeErr(c, http.StatusInternalServerError, err.Error())
	}

	if !includeExtensions(c) {
		spec = stripAPIGatewayExtensions(spec)
	}

	// AWS returns the raw OpenAPI document as the HTTP response body (the SDK's
	// ExportApi `Body` blob), not a wrapper object.
	if strings.EqualFold(outputType, "YAML") {
		blob, mErr := yaml.Marshal(spec)
		if mErr != nil {
			return writeErr(c, http.StatusInternalServerError, mErr.Error())
		}

		return c.Blob(http.StatusOK, "application/x-yaml", blob)
	}

	blob, mErr := json.Marshal(spec)
	if mErr != nil {
		return writeErr(c, http.StatusInternalServerError, mErr.Error())
	}

	return c.JSONBlob(http.StatusOK, blob)
}
