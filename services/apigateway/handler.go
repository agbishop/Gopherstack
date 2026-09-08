package apigateway

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyPosition       = "position"
	keyLimit          = "limit"
	litTrue           = "true"
	headerContentType = "Content-Type"
	// modeImport is the "mode" query parameter value that distinguishes
	// ImportRestApi (POST /restapis?mode=import) and ImportApiKeys
	// (POST /apikeys?mode=import) from their plain-create counterparts.
	modeImport = "import"
)

// JWKSProvider resolves RSA public keys for JWT signature verification.
// Implementations return an error when the issuer or key is unknown.
type JWKSProvider interface {
	GetJWTPublicKey(issuerURL, kid string) (*rsa.PublicKey, error)
}

// Handler is the Echo HTTP service handler for API Gateway operations.
type Handler struct {
	Backend      StorageBackend
	jwksProvider JWKSProvider
	lambda       LambdaInvoker
	sqsSender    SQSSender
	snsPublisher SNSPublisher
	authCache    *authorizerCache
	httpClient   *http.Client
	// selRegexpCache is a bounded LRU of compiled selection-pattern regexps. It is
	// keyed by user-supplied patterns, so it must be size-capped to prevent unbounded
	// growth.
	selRegexpCache *regexpCache
	// dispatchCache is the op→handler table, built exactly once (see dispatchOnce)
	// instead of per request.
	dispatchCache map[string]actionFn
	// trieCache holds the per-API routing trie (map[apiID]*trieCacheEntry). It is
	// rebuilt only when the API's resource-set version changes.
	trieCache sync.Map
	// dispatchOnce guards the one-time build of dispatchCache.
	dispatchOnce sync.Once
}

// NewHandler creates a new API Gateway handler with a default HTTP client timeout.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend:        backend,
		authCache:      newAuthorizerCache(),
		httpClient:     &http.Client{Timeout: apiGWHTTPTimeout},
		selRegexpCache: newRegexpCache(defaultRegexpCacheMaxEntries),
	}
}

// SetLambdaInvoker configures the Lambda invoker for AWS_PROXY integrations.
func (h *Handler) SetLambdaInvoker(lambda LambdaInvoker) {
	h.lambda = lambda
}

// SetSQSSender configures the SQS hook for AWS integrations whose URI targets sqs.
// Left unset, such integrations fall back to the pre-existing Lambda-invoke behaviour
// (see handleAWSIntegration) rather than erroring -- an unwired hook is a no-op, not
// a rejection.
func (h *Handler) SetSQSSender(sender SQSSender) {
	h.sqsSender = sender
}

// SetSNSPublisher configures the SNS hook for AWS integrations whose URI targets
// sns action/Publish. Left unset, such integrations fall back to the pre-existing
// Lambda-invoke behaviour rather than erroring.
func (h *Handler) SetSNSPublisher(publisher SNSPublisher) {
	h.snsPublisher = publisher
}

// SetJWKSProvider configures the JWKS provider used to verify Cognito JWT signatures.
func (h *Handler) SetJWKSProvider(p JWKSProvider) {
	h.jwksProvider = p
}

// SetHTTPClient configures the HTTP client used for HTTP/HTTP_PROXY integrations.
// If not set, a dedicated client with a 30-second timeout is used.
func (h *Handler) SetHTTPClient(c *http.Client) {
	h.httpClient = c
}

// apiGWHTTPTimeout is the timeout applied to HTTP/HTTP_PROXY integration requests.
const apiGWHTTPTimeout = 30 * time.Second

// getHTTPClient returns the configured HTTP client.
func (h *Handler) getHTTPClient() *http.Client {
	return h.httpClient
}

// Name returns the service name.
func (h *Handler) Name() string { return "APIGateway" }

// GetSupportedOperations returns all mocked API Gateway operations.
func (h *Handler) GetSupportedOperations() []string {
	return slices.Concat(coreAPIGatewayOperations(), extendedAPIGatewayOperations())
}

// coreAPIGatewayOperations lists the CRUD operations for REST APIs, resources,
// methods, integrations, deployments, stages, authorizers, request validators,
// and API keys.
func coreAPIGatewayOperations() []string {
	return []string{
		opCreateRestAPI,
		opDeleteRestAPI,
		opGetRestAPI,
		opGetRestApis,
		opGetResources,
		opGetResource,
		opCreateResource,
		opDeleteResource,
		opPutMethod,
		opGetMethod,
		opDeleteMethod,
		opPutMethodResponse,
		opGetMethodResponse,
		opDeleteMethodResponse,
		opPutIntegration,
		opGetIntegration,
		opDeleteIntegration,
		opPutIntegrationResponse,
		opGetIntegrationResponse,
		opDeleteIntegrationResponse,
		opCreateDeployment,
		opGetDeployment,
		opGetDeployments,
		opDeleteDeployment,
		opGetStages,
		opGetStage,
		opDeleteStage,
		opCreateAuthorizer,
		opGetAuthorizer,
		opGetAuthorizers,
		opUpdateAuthorizer,
		opDeleteAuthorizer,
		opCreateRequestValidator,
		opGetRequestValidator,
		opGetRequestValidators,
		opUpdateRequestValidator,
		opDeleteRequestValidator,
		opCreateAPIKey,
		opGetAPIKey,
		opGetAPIKeys,
		opDeleteAPIKey,
		opUpdateAPIKey,
	}
}

// extendedAPIGatewayOperations lists the remaining CRUD/update operations for
// base path mappings, documentation, domain names, models, stages, usage
// plans, tags, gateway responses, client certificates, usage, VPC links, SDK
// generation, and bulk import/export.
func extendedAPIGatewayOperations() []string {
	return []string{
		opCreateBasePathMapping,
		opGetBasePathMapping,
		opGetBasePathMappings,
		opDeleteBasePathMapping,
		opCreateDocumentationPart,
		opGetDocumentationPart,
		opGetDocumentationParts,
		opDeleteDocumentationPart,
		opCreateDocumentationVersion,
		opGetDocumentationVersion,
		opGetDocumentationVersions,
		opDeleteDocumentationVersion,
		opCreateDomainName,
		opGetDomainName,
		opGetDomainNames,
		opDeleteDomainName,
		opCreateDomainNameAccessAssociation,
		opCreateModel,
		opGetModel,
		opGetModels,
		opDeleteModel,
		opUpdateModel,
		opCreateStage,
		opUpdateStage,
		opFlushStageCache,
		opFlushStageAuthorizersCache,
		opCreateUsagePlan,
		opGetUsagePlan,
		opGetUsagePlans,
		opDeleteUsagePlan,
		opCreateUsagePlanKey,
		opGetUsagePlanKey,
		opGetUsagePlanKeys,
		opDeleteUsagePlanKey,
		opUpdateRestAPI,
		opUpdateDeployment,
		opUpdateResource,
		opGetAccount,
		opGetTags,
		opTagResource,
		opUntagResource,
		opTestInvokeMethod,
		opUpdateUsagePlan,
		opUpdateDomainName,
		opUpdateBasePathMapping,
		opUpdateDocumentationPart,
		opUpdateDocumentationVersion,
		opUpdateMethod,
		opUpdateIntegration,
		opUpdateIntegrationResponse,
		opUpdateMethodResponse,
		opUpdateAccount,
		opTestInvokeAuthorizer,
		opGetModelTemplate,
		opGetGatewayResponse,
		opGetGatewayResponses,
		opPutGatewayResponse,
		opDeleteGatewayResponse,
		opGenerateClientCertificate,
		opGetClientCertificate,
		opGetClientCertificates,
		opDeleteClientCertificate,
		opGetUsage,
		// Stub implementations.
		opCreateVpcLink,
		opDeleteDomainNameAccessAssociation,
		opDeleteVpcLink,
		opGetDomainNameAccessAssociations,
		opGetExport,
		opGetSdk,
		opGetSdkType,
		opGetSdkTypes,
		opGetVpcLink,
		opGetVpcLinks,
		opImportAPIKeys,
		opImportDocumentationParts,
		opImportRestAPI,
		opPutRestAPI,
		opRejectDomainNameAccessAssociation,
		opUpdateClientCertificate,
		opUpdateGatewayResponse,
		opUpdateUsage,
		opUpdateVpcLink,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "apigateway" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this API Gateway instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a matcher for API Gateway requests.
// Matches X-Amz-Target (JSON protocol) and REST paths (/restapis/..., /apikeys, /domainnames/..., /usageplans/...).
// The /tags/{arn} path is only matched when the ARN belongs to an API Gateway resource
// (contains ":apigateway:") so that other services (e.g. FIS) can own their own tag routes.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		if strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), "APIGateway.") {
			return true
		}

		path := c.Request().URL.Path

		if isAPIGWTopLevelRESTPath(path) {
			return true
		}

		// /tags/{arn} — only claim this path when the ARN is an API Gateway resource.
		// API Gateway ARNs contain ":apigateway:" (e.g. arn:aws:apigateway:us-east-1::/restapis/xyz).
		if after, ok := strings.CutPrefix(path, "/tags/"); ok {
			return strings.Contains(after, ":apigateway:")
		}

		return false
	}
}

// isAPIGWTopLevelRESTPath reports whether path matches one of the top-level
// REST resource collections API Gateway's control-plane API exposes. Shared by
// RouteMatcher and Handler so the two stay in sync.
func isAPIGWTopLevelRESTPath(path string) bool {
	prefixes := []string{
		"/restapis",
		"/apikeys",
		"/domainnames",
		"/usageplans",
		"/" + apiGWSegClientCerts,
		"/" + apiGWSegVpcLinks,
		"/" + apiGWSegSdkTypes,
		"/" + apiGWSegDomainNameAccessAssociations,
		"/" + apiGWSegRejectDomainNameAccessAssociations,
	}

	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}

	// AWS's own SDK only ever emits the bare "/account" path (confirmed against
	// aws-sdk-go-v2/service/apigateway's serializers.go SplitURI calls for both
	// GetAccount and UpdateAccount) -- API Gateway's Account resource has no
	// sub-paths. A "/account/" prefix claim here previously shadowed
	// QuickSight's CreateAccountSubscription/DescribeAccountSubscription/
	// DeleteAccountSubscription, which live at "/account/{AwsAccountId}".
	return path == "/account"
}

// MatchPriority returns the routing priority for the API Gateway handler.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the operation name from the X-Amz-Target header or REST path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	const targetParts = 2
	if len(parts) == targetParts {
		return parts[1]
	}

	op, _, _ := parseAPIGWRESTPath(c.Request().Method, c.Request().URL.Path, c.Request().URL.Query())

	return op
}

// ExtractResource extracts the resource identifier from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	for _, key := range []string{keyRestAPIID, keyAPIName} {
		if v, ok := data[key].(string); ok && v != "" {
			return v
		}
	}

	return ""
}

// Handler returns the Echo handler function for API Gateway requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		if c.Request().Method == http.MethodGet && c.Request().URL.Path == "/" {
			return c.JSON(http.StatusOK, h.GetSupportedOperations())
		}

		// Handle proxy invocations for deployed API stages.
		// Path format: /proxy/{apiId}/{stageName}/{resourcePath}
		if strings.HasPrefix(c.Request().URL.Path, "/proxy/") {
			return h.handleStageProxyEcho(c)
		}

		// Handle data-plane invocations via the standard AWS endpoint format.
		// Path format: /restapis/{apiId}/{stageName}/_user_request_/{resourcePath}
		if isUserRequestPath(c.Request().URL.Path) {
			return h.handleUserRequestEcho(c)
		}

		// REST API paths: /restapis/..., /apikeys, /domainnames/..., /usageplans/...
		path := c.Request().URL.Path
		tagsAfter, isTagsPath := strings.CutPrefix(path, "/tags/")
		isAPIGWTagPath := isTagsPath && strings.Contains(tagsAfter, ":apigateway:")
		isRESTPath := isAPIGWTopLevelRESTPath(path) || isAPIGWTagPath

		if isRESTPath && !strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), "APIGateway.") {
			return h.handleRESTAPI(c)
		}

		return h.handleJSONProtocol(c)
	}
}

// writeJSONProtocolDispatchError writes an ErrorResponse for a failure in
// handleJSONProtocol itself (bad method, missing/malformed X-Amz-Target,
// body-read failure) -- framework-level errors that never reach dispatch or
// handleError. These previously went out as bare text/plain, which the
// __type/message JSON error decoder shared by the JSON-RPC family
// (aws-sdk-go-v2@v1.43.4 aws/protocol/restjson.GetErrorInfo) cannot read:
// every such response reached a client as
// smithy.GenericAPIError{Code:"UnknownError"} (gopherstack-wlo1).
func writeJSONProtocolDispatchError(c *echo.Context, status int, errType, message string) error {
	c.Response().Header().Set(headerContentType, "application/x-amz-json-1.1")

	payload, err := json.Marshal(ErrorResponse{Type: errType, Message: message})
	if err != nil {
		return err
	}

	return c.JSONBlob(status, payload)
}

// handleJSONProtocol handles requests using the JSON protocol (X-Amz-Target header).
func (h *Handler) handleJSONProtocol(c *echo.Context) error {
	ctx := c.Request().Context()
	log := logger.Load(ctx)

	if c.Request().Method != http.MethodPost {
		return writeJSONProtocolDispatchError(c, http.StatusMethodNotAllowed,
			"UnknownOperationException", "Method not allowed")
	}

	target := c.Request().Header.Get("X-Amz-Target")
	if target == "" {
		return writeJSONProtocolDispatchError(c, http.StatusBadRequest,
			"UnknownOperationException", "Missing X-Amz-Target")
	}

	parts := strings.Split(target, ".")
	const targetParts = 2
	if len(parts) != targetParts {
		return writeJSONProtocolDispatchError(c, http.StatusBadRequest,
			"UnknownOperationException", "Invalid X-Amz-Target")
	}
	action := parts[1]

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		log.ErrorContext(ctx, "failed to read request body", "error", err)

		return writeJSONProtocolDispatchError(c, http.StatusInternalServerError,
			"InternalFailure", "internal server error")
	}

	log.DebugContext(ctx, "APIGateway request", "action", action)

	statusCode, response, raw, reqErr := h.dispatch(ctx, action, body)
	if reqErr != nil {
		return h.handleError(ctx, c, action, reqErr)
	}

	if raw != nil {
		return writeRawBinaryResponse(c, statusCode, raw)
	}

	c.Response().Header().Set(headerContentType, "application/x-amz-json-1.1")
	if statusCode == http.StatusNoContent {
		return c.NoContent(http.StatusNoContent)
	}

	return c.JSONBlob(statusCode, response)
}

// handleRESTAPI handles REST-style API Gateway calls (e.g. from the AWS SDK v2).
// It parses the URL path, extracts path parameters, merges them with the request
// body, and dispatches to the existing typed action handlers.
func (h *Handler) handleRESTAPI(c *echo.Context) error {
	ctx := c.Request().Context()

	query := c.Request().URL.Query()

	action, pathParams, ok := parseAPIGWRESTPath(c.Request().Method, c.Request().URL.Path, query)
	if !ok {
		return h.handleError(ctx, c, action, errUnknownOperation)
	}

	// ImportRestApi, PutRestApi, ImportApiKeys, and ImportDocumentationParts all
	// carry an opaque raw document as their request body (Content-Type:
	// application/octet-stream — an OpenAPI/Swagger spec, a CSV file, or a JSON
	// documentation-parts payload), not a JSON object of named parameters.
	// Merging query parameters into it the way other operations do would corrupt
	// non-JSON-object bodies, so build a small envelope instead and skip the
	// generic merge below.
	if isRawBodyAPIGWAction(action) {
		return h.dispatchRestAPISpec(c, action, pathParams, query)
	}

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		logger.Load(ctx).ErrorContext(ctx, "failed to read request body", "error", err)

		return writeJSONProtocolDispatchError(c, http.StatusInternalServerError,
			"InternalFailure", "internal server error")
	}

	// OpenAPI import (ImportRestApi / PutRestApi) carries the raw spec document
	// as the HTTP body. These are detected here because they share REST paths
	// with CreateRestApi (POST /restapis) and UpdateRestApi (PUT /restapis/{id})
	// but are distinguished by the request method/query, and the body must be
	// passed through verbatim rather than treated as a flat field object.
	if importAction, importBody, isImport := detectImportRESTAPI(
		c.Request().Method, action, pathParams, c.Request().URL.Query(), body,
	); isImport {
		return h.dispatchAndRespond(ctx, c, importAction, importBody, contentTypeJSON)
	}

	// GET requests have no body; normalise to an empty JSON object so that
	// json.Unmarshal calls in the action handlers don't fail with
	// "unexpected end of JSON input".
	if len(body) == 0 {
		body = []byte("{}")
	}

	// Convert RFC 6902 patch documents ({"patchOperations":[...]}, or a bare
	// array) to flat JSON objects so PATCH handlers can read fields directly
	// (e.g. [{"op":"replace","path":"/name","value":"x"}] becomes
	// {"name":"x"}); see patch.go for the resource-specific map/struct/list
	// merges this also handles (stage variables, canary promotion, per-route
	// method settings, binary media types, usage-plan API stages, gateway
	// response parameters/templates) that a flat single-field replace cannot
	// express.
	body, patchErr := h.applyStructuredPatch(action, pathParams, body)
	if patchErr != nil {
		return h.handleError(ctx, c, action, patchErr)
	}

	// Merge path parameters into the JSON body so existing handlers can read them.
	for k, v := range pathParams {
		body = injectJSONFieldAPIGW(body, k, v)
	}

	// Merge query string parameters (e.g., limit, position) into the JSON body.
	for k, v := range c.Request().URL.Query() {
		if len(v) > 0 {
			body = injectJSONFieldAPIGW(body, k, v[0])
		}
	}

	return h.dispatchAndRespond(ctx, c, action, body, contentTypeJSON)
}

// dispatchAndRespond runs an action through the dispatch table and writes the
// HTTP response, including correct handling of 204 No Content responses.
func (h *Handler) dispatchAndRespond(
	ctx context.Context, c *echo.Context, action string, body []byte, contentType string,
) error {
	statusCode, response, raw, reqErr := h.dispatch(ctx, action, body)
	if reqErr != nil {
		return h.handleError(ctx, c, action, reqErr)
	}

	if raw != nil {
		return writeRawBinaryResponse(c, statusCode, raw)
	}

	c.Response().Header().Set(headerContentType, contentType)
	if statusCode == http.StatusNoContent {
		return c.NoContent(http.StatusNoContent)
	}

	return c.JSONBlob(statusCode, response)
}

// writeRawBinaryResponse writes a *rawBinaryResponse with its real
// Content-Type/Content-Disposition headers and raw body, bypassing the
// JSON envelope dispatchAndRespond/handleJSONProtocol otherwise write.
func writeRawBinaryResponse(c *echo.Context, statusCode int, raw *rawBinaryResponse) error {
	if raw.contentDisposition != "" {
		c.Response().Header().Set("Content-Disposition", raw.contentDisposition)
	}

	return c.Blob(statusCode, raw.contentType, raw.body)
}

// detectImportRESTAPI recognises ImportRestApi (POST /restapis?mode=import) and
// PutRestApi (PUT /restapis/{id}) requests, returning the resolved action and a
// JSON-encoded typed input whose Body field carries the raw spec document. The
// AWS SDK sends the OpenAPI/Swagger document as the verbatim HTTP body, so it
// must not be merged with path/query parameters like other operations.
func detectImportRESTAPI(
	method, action string, pathParams map[string]string, query url.Values, body []byte,
) (string, []byte, bool) {
	switch {
	case action == opCreateRestAPI && method == http.MethodPost && query.Get("mode") == modeImport:
		in := ImportRestAPIInput{
			Body:           body,
			FailOnWarnings: query.Get("failonwarnings") == litTrue,
		}
		encoded, err := json.Marshal(in)
		if err != nil {
			return "", nil, false
		}

		return opImportRestAPI, encoded, true
	case action == opCreateAPIKey && method == http.MethodPost && query.Get("mode") == modeImport:
		// ImportApiKeys (POST /apikeys?mode=import&format=csv) carries the raw API
		// key file (csv or json) as the verbatim HTTP body.
		in := importAPIKeysInput{
			Body:   body,
			Format: query.Get("format"),
		}
		encoded, err := json.Marshal(in)
		if err != nil {
			return "", nil, false
		}

		return opImportAPIKeys, encoded, true
	case action == opPutRestAPI && method == http.MethodPut && pathParams[keyRestAPIID] != "":
		in := PutRestAPIInput{
			RestAPIID:      pathParams[keyRestAPIID],
			Mode:           query.Get("mode"),
			FailOnWarnings: query.Get("failonwarnings") == litTrue,
			Body:           body,
		}
		encoded, err := json.Marshal(in)
		if err != nil {
			return "", nil, false
		}

		return opPutRestAPI, encoded, true
	}

	return "", nil, false
}

// injectJSONFieldAPIGW merges a key/value string pair into a JSON object body.
// "limit" is the sole Integer-typed apigateway query parameter (every list op
// binds it via encoder.SetQuery("limit").Integer(...), e.g. apigateway@v1.42.4
// serializers.go:4110); every handler input struct types it as Go int, so it
// must be injected as a bare JSON number, not a quoted string, or a real
// client's Limit always 500s on json.Unmarshal.
func injectJSONFieldAPIGW(body []byte, key, value string) []byte {
	var m map[string]json.RawMessage
	if len(body) > 0 {
		if err := json.Unmarshal(body, &m); err != nil {
			m = make(map[string]json.RawMessage)
		}
	} else {
		m = make(map[string]json.RawMessage)
	}

	if key == keyLimit {
		if n, err := strconv.Atoi(value); err == nil {
			m[key] = json.RawMessage(strconv.Itoa(n))
			result, _ := json.Marshal(m)

			return result
		}
	}

	quoted, _ := json.Marshal(value)
	m[key] = json.RawMessage(quoted)

	result, _ := json.Marshal(m)

	return result
}

type actionFn func([]byte) (int, any, error)

// rawBinaryResponse marks an actionFn result that must reach the client
// as-is: real Content-Type/Content-Disposition headers plus a raw payload,
// never a JSON envelope. GetSdk and GetExport are the only apigateway
// operations whose real Output structs bind ContentType/ContentDisposition
// to HTTP headers and Body to the raw response payload (apigateway@v1.42.4
// deserializers.go: awsRestjson1_deserializeOpHttpBindingsGetSdkOutput /
// awsRestjson1_deserializeOpHttpBindingsGetExportOutput bind the headers;
// awsRestjson1_deserializeOpDocumentGetSdkOutput /
// awsRestjson1_deserializeOpDocumentGetExportOutput copy the response body
// straight into Body with no JSON parsing). An actionFn returning
// *rawBinaryResponse makes dispatch skip json.Marshal so dispatchAndRespond
// can write it with c.Blob and the real headers instead.
type rawBinaryResponse struct {
	contentType        string
	contentDisposition string
	body               []byte
}

// handleStageProxyEcho routes /proxy/{apiId}/{stageName}/{path} requests to the Lambda proxy handler.
func (h *Handler) handleStageProxyEcho(c *echo.Context) error {
	// Strip the /proxy/ prefix to get /{apiId}/{stageName}/{path}
	rest := strings.TrimPrefix(c.Request().URL.Path, "/proxy/")
	const minProxyPathParts = 2
	parts := strings.SplitN(rest, "/", 3) //nolint:mnd // 3-part split: apiId, stage, path
	if len(parts) < minProxyPathParts {
		return c.String(http.StatusNotFound, "invalid proxy path")
	}

	apiID := parts[0]
	stageName := parts[1]

	// Rewrite the URL so handleProxyRequest sees "/{stageName}/{resourcePath}".
	resourcePath := "/"
	if len(parts) == 3 && parts[2] != "" {
		resourcePath = "/" + parts[2]
	}

	r := c.Request().Clone(c.Request().Context())
	r.URL.Path = "/" + stageName + resourcePath

	fn := h.handleProxyRequest(apiID, stageName)
	fn(c.Response(), r)

	return nil
}

// isUserRequestPath reports whether the path follows the data-plane format:
// /restapis/{apiId}/{stageName}/_user_request_/{resourcePath...}.
func isUserRequestPath(path string) bool {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	const minSegs = 4 // restapis, apiId, stageName, _user_request_

	return len(segs) >= minSegs && segs[0] == apiGWSegRestAPIs && segs[3] == "_user_request_"
}

// handleUserRequestEcho handles data-plane invocations at the standard AWS endpoint:
// /restapis/{apiId}/{stageName}/_user_request_/{resourcePath...}.
func (h *Handler) handleUserRequestEcho(c *echo.Context) error {
	segs := strings.Split(strings.TrimPrefix(c.Request().URL.Path, "/"), "/")
	// segs: [restapis, {apiId}, {stageName}, _user_request_, {path...}]
	const (
		idxAPIID     = 1
		idxStageName = 2
		idxPathStart = 4
	)

	apiID := segs[idxAPIID]
	stageName := segs[idxStageName]

	resourcePath := "/"
	if len(segs) > idxPathStart && segs[idxPathStart] != "" {
		resourcePath = "/" + strings.Join(segs[idxPathStart:], "/")
	}

	// Rewrite the URL so handleProxyRequest sees "/{stageName}/{resourcePath}".
	r := c.Request().Clone(c.Request().Context())
	r.URL.Path = "/" + stageName + resourcePath

	fn := h.handleProxyRequest(apiID, stageName)
	fn(c.Response(), r)

	return nil
}

// dispatchTable returns the op→handler table, building it exactly once and caching it.
// The table's closures capture the receiver, so a single build is valid for the
// handler's lifetime; rebuilding it (13 sub-constructors + maps.Copy) per request was
// pure overhead.
func (h *Handler) dispatchTable() map[string]actionFn {
	h.dispatchOnce.Do(func() {
		h.dispatchCache = h.buildDispatchTable()
	})

	return h.dispatchCache
}

// buildDispatchTable assembles the op→handler table from each op-family's
// action map (defined in the matching handler_<family>.go file).
func (h *Handler) buildDispatchTable() map[string]actionFn {
	table := make(map[string]actionFn)
	maps.Copy(table, h.restAPIActions())
	maps.Copy(table, h.resourceActions())
	maps.Copy(table, h.methodActions())
	maps.Copy(table, h.methodResponseActions())
	maps.Copy(table, h.integrationActions())
	maps.Copy(table, h.integrationResponseActions())
	maps.Copy(table, h.deploymentActions())
	maps.Copy(table, h.authorizerActions())
	maps.Copy(table, h.requestValidatorActions())
	maps.Copy(table, h.apiKeyActions())
	maps.Copy(table, h.basePathMappingActions())
	maps.Copy(table, h.domainNameActions())
	maps.Copy(table, h.domainNameAccessAssociationActions())
	maps.Copy(table, h.documentationActions())
	maps.Copy(table, h.schemaModelActions())
	maps.Copy(table, h.usagePlanActions())
	maps.Copy(table, h.accountActions())
	maps.Copy(table, h.tagActions())
	maps.Copy(table, h.gatewayResponseActions())
	maps.Copy(table, h.clientCertificateActions())
	maps.Copy(table, h.vpcLinkActions())
	maps.Copy(table, h.sdkActions())
	maps.Copy(table, h.usageActions())
	maps.Copy(table, h.exportActions())
	maps.Copy(table, h.restAPISpecActions())
	maps.Copy(table, h.importActions())

	return table
}

// dispatch routes the action to the correct handler function. When the
// handler returns a *rawBinaryResponse (see its doc comment), raw is
// non-nil and encoded is unset -- the caller must write raw's body and
// headers directly instead of JSON-encoding the response.
func (h *Handler) dispatch(
	_ context.Context, action string, body []byte,
) (int, []byte, *rawBinaryResponse, error) {
	fn, found := h.dispatchTable()[action]
	if !found {
		return 0, nil, nil, fmt.Errorf("%w:%s", errUnknownOperation, action)
	}

	statusCode, response, err := fn(body)
	if err != nil {
		return 0, nil, nil, err
	}

	if raw, isRaw := response.(*rawBinaryResponse); isRaw {
		return statusCode, nil, raw, nil
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		return 0, nil, nil, err
	}

	return statusCode, encoded, nil, nil
}

// handleError writes a standardized JSON error response.
func (h *Handler) handleError(ctx context.Context, c *echo.Context, action string, reqErr error) error {
	log := logger.Load(ctx)
	c.Response().Header().Set(headerContentType, "application/x-amz-json-1.1")

	var errType string
	var statusCode int

	switch {
	case errors.Is(reqErr, ErrRestAPINotFound),
		errors.Is(reqErr, ErrResourceNotFound),
		errors.Is(reqErr, ErrMethodNotFound),
		errors.Is(reqErr, ErrMethodResponseNotFound),
		errors.Is(reqErr, ErrIntegrationResponseNotFound),
		errors.Is(reqErr, ErrDeploymentNotFound),
		errors.Is(reqErr, ErrAuthorizerNotFound),
		errors.Is(reqErr, ErrValidatorNotFound),
		errors.Is(reqErr, ErrAPIKeyNotFound),
		errors.Is(reqErr, ErrBasePathMappingNotFound),
		errors.Is(reqErr, ErrDocumentationPartNotFound),
		errors.Is(reqErr, ErrDocumentationVersionNotFound),
		errors.Is(reqErr, ErrDomainNameNotFound),
		errors.Is(reqErr, ErrDomainNameAccessAssociationNotFound),
		errors.Is(reqErr, ErrModelNotFound),
		errors.Is(reqErr, ErrUsagePlanNotFound),
		errors.Is(reqErr, ErrUsagePlanKeyNotFound),
		errors.Is(reqErr, ErrStageNotFound),
		errors.Is(reqErr, ErrNotFound):
		errType = "NotFoundException"
		statusCode = http.StatusNotFound
	case errors.Is(reqErr, ErrAlreadyExists):
		errType = "ConflictException"
		statusCode = http.StatusConflict
	case errors.Is(reqErr, ErrInvalidParameter):
		errType = "BadRequestException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, errUnknownOperation):
		errType = "UnknownOperationException"
		statusCode = http.StatusBadRequest
	default:
		errType = "InternalServerError"
		statusCode = http.StatusInternalServerError
	}

	if statusCode == http.StatusInternalServerError {
		log.ErrorContext(ctx, "APIGateway internal error", "error", reqErr, "action", action)
	} else {
		log.WarnContext(ctx, "APIGateway request error", "error", reqErr, "action", action)
	}

	errResp := ErrorResponse{
		Type:    errType,
		Message: reqErr.Error(),
	}

	payload, _ := json.Marshal(errResp)

	return c.JSONBlob(statusCode, payload)
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}
}
