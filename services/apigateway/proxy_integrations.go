package apigateway

import (
	"container/list"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// handleAWSProxy handles an AWS_PROXY Lambda integration — the full event is forwarded as-is.
func (h *Handler) handleAWSProxy(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	apiID, stageName string,
	resource *Resource,
	integration *Integration,
	pathParams map[string]string,
) {
	if h.lambda == nil {
		http.Error(w, "Lambda integration not configured", http.StatusServiceUnavailable)

		return
	}

	event, buildErr := BuildProxyEvent(r, apiID, stageName, resource.Path, r.URL.Path, pathParams)
	if buildErr != nil {
		logger.Load(ctx).ErrorContext(ctx, "APIGateway proxy: failed to build event", "error", buildErr)
		http.Error(w, "Internal server error", http.StatusInternalServerError)

		return
	}

	payload, _ := json.Marshal(event)

	respBytes, _, invokeErr := h.lambda.InvokeFunction(
		ctx,
		ExtractLambdaFunctionName(integration.URI),
		"RequestResponse",
		payload,
	)
	if invokeErr != nil {
		logger.Load(ctx).WarnContext(ctx, "APIGateway proxy: Lambda invocation failed",
			"uri", integration.URI, "error", invokeErr)
		http.Error(w, "Internal server error", http.StatusInternalServerError)

		return
	}

	// Parse Lambda response.
	var lambdaResp LambdaProxyResponse
	if parseErr := json.Unmarshal(respBytes, &lambdaResp); parseErr != nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes) //nolint:gosec // local emulation: response passthrough is intentional

		return
	}

	for k, v := range lambdaResp.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")

	statusCode := lambdaResp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	var bodyBytes []byte
	if lambdaResp.IsBase64Encoded {
		decoded, decErr := base64.StdEncoding.DecodeString(lambdaResp.Body)
		if decErr == nil {
			bodyBytes = decoded
		} else {
			bodyBytes = []byte(lambdaResp.Body)
		}
	} else {
		bodyBytes = []byte(lambdaResp.Body)
	}

	bodyBytes = maybeCompressResponse(w, r, bodyBytes, h.minCompressSize(apiID))
	w.WriteHeader(statusCode)
	_, _ = w.Write(bodyBytes)
}

// minCompressSize returns the MinimumCompressionSize for the given API (0 = disabled).
func (h *Handler) minCompressSize(apiID string) int {
	api, err := h.Backend.GetRestAPI(apiID)
	if err != nil || api == nil {
		return 0
	}

	return api.MinimumCompressionSize
}

// handleAWSIntegration handles an AWS (non-proxy) integration using VTL templates.
// The integration URI names its target service (see awsIntegrationTarget); when that
// target is sqs or sns and the corresponding hook is wired, the request is dispatched
// there instead of Lambda. Every other target -- including sqs/sns with no hook wired,
// dynamodb, kinesis, states, and lambda itself -- keeps the original Lambda-invoke path
// unchanged.
func (h *Handler) handleAWSIntegration(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	apiID, stageName string,
	resource *Resource,
	stageVars map[string]string,
	integration *Integration,
) {
	region, service, kind, spec := awsIntegrationTarget(integration.URI)
	if h.canDispatchToTarget(service, kind, spec) {
		h.handleAWSServiceIntegration(
			ctx, w, r, apiID, stageName, resource, stageVars, integration, region, service, spec,
		)

		return
	}

	if h.lambda == nil {
		http.Error(w, "Lambda integration not configured", http.StatusServiceUnavailable)

		return
	}

	payload, vtlCtx, readErr := h.buildAWSIntegrationPayload(w, r, apiID, stageName, resource, stageVars, integration)
	if readErr != nil {
		writeAWSIntegrationReadError(ctx, w, readErr)

		return
	}

	// Invoke Lambda.
	respBytes, _, invokeErr := h.lambda.InvokeFunction(
		ctx,
		ExtractLambdaFunctionName(integration.URI),
		"RequestResponse",
		payload,
	)
	if invokeErr != nil {
		logger.Load(ctx).WarnContext(ctx, "APIGateway AWS integration: Lambda invocation failed",
			"uri", integration.URI, "error", invokeErr)
		http.Error(w, "Internal server error", http.StatusInternalServerError)

		return
	}

	// Apply response mapping template using status-code pattern matching.
	responseBody, statusCode := h.applyResponseTemplate(respBytes, integration, vtlCtx.RequestID)

	responseBody = maybeCompressResponse(w, r, responseBody, h.minCompressSize(apiID))
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	_, _ = w.Write(responseBody) //nolint:gosec // local emulation: response passthrough is intentional
}

// buildAWSIntegrationPayload reads the request body, builds the VTL request context,
// and applies the integration's request mapping template (if any). Shared by the
// Lambda and wired-service AWS integration paths.
func (h *Handler) buildAWSIntegrationPayload(
	w http.ResponseWriter,
	r *http.Request,
	apiID, stageName string,
	resource *Resource,
	stageVars map[string]string,
	integration *Integration,
) ([]byte, VTLContext, error) {
	rawBody, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProxyRequestBodyBytes))
	if readErr != nil {
		return nil, VTLContext{}, readErr
	}

	resourcePath := "/"
	if resource != nil && resource.Path != "" {
		resourcePath = resource.Path
	}

	vtlCtx := VTLContext{
		Body:           string(rawBody),
		RequestID:      r.Header.Get("X-Amzn-Requestid"),
		HTTPMethod:     r.Method,
		ResourcePath:   resourcePath,
		Path:           r.URL.Path,
		Stage:          stageName,
		APIID:          apiID,
		SourceIP:       realClientIP(r),
		UserAgent:      r.Header.Get("User-Agent"),
		StageVariables: stageVars,
	}

	// Apply request mapping template (content-type "application/json" is standard).
	payload := rawBody
	if tpl, ok := integration.RequestTemplates[contentTypeJSON]; ok && tpl != "" {
		payload = []byte(RenderTemplate(tpl, vtlCtx))
	}

	return payload, vtlCtx, nil
}

// writeAWSIntegrationReadError writes the appropriate error response for a
// buildAWSIntegrationPayload failure.
func writeAWSIntegrationReadError(ctx context.Context, w http.ResponseWriter, readErr error) {
	if _, ok := errors.AsType[*http.MaxBytesError](readErr); ok {
		http.Error(w, "Request entity too large", http.StatusRequestEntityTooLarge)

		return
	}

	logger.Load(ctx).ErrorContext(ctx, "APIGateway AWS integration: failed to read body", "error", readErr)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

// handleAWSServiceIntegration dispatches an AWS integration to a wired non-Lambda
// target (sqs SendMessage, sns Publish). This is a simplified passthrough, not VTL
// mapping-template evaluation: the rendered request payload becomes the target's
// message body/text verbatim, with no AWS query-protocol (Action=...&...) encoding,
// and the HTTP response is a bare "{}" run back through the same status-code /
// response-template matching the Lambda path uses. See PARITY.md.
func (h *Handler) handleAWSServiceIntegration(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	apiID, stageName string,
	resource *Resource,
	stageVars map[string]string,
	integration *Integration,
	region, service, spec string,
) {
	payload, vtlCtx, readErr := h.buildAWSIntegrationPayload(w, r, apiID, stageName, resource, stageVars, integration)
	if readErr != nil {
		writeAWSIntegrationReadError(ctx, w, readErr)

		return
	}

	var dispatchErr error

	switch service {
	case "sqs":
		dispatchErr = h.dispatchSQS(ctx, region, spec, payload)
	case "sns":
		dispatchErr = h.dispatchSNS(ctx, r, integration, payload)
	}

	if dispatchErr != nil {
		logger.Load(ctx).WarnContext(ctx, "APIGateway AWS integration: target service call failed",
			"uri", integration.URI, "service", service, "error", dispatchErr)
		http.Error(w, "Internal server error", http.StatusInternalServerError)

		return
	}

	responseBody, statusCode := h.applyResponseTemplate([]byte("{}"), integration, vtlCtx.RequestID)
	responseBody = maybeCompressResponse(w, r, responseBody, h.minCompressSize(apiID))
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	_, _ = w.Write(responseBody) //nolint:gosec // local emulation: response passthrough is intentional
}

// sqsQueuePathSegments is the expected segment count of the path-style sqs
// service_api: "{accountId}/{queueName}".
const sqsQueuePathSegments = 2

// dispatchSQS sends payload as the message body to the SQS queue named in spec
// ("{accountId}/{queueName}", the path-style AWS integration service_api).
func (h *Handler) dispatchSQS(ctx context.Context, region, spec string, payload []byte) error {
	segments := strings.Split(spec, "/")
	if len(segments) != sqsQueuePathSegments {
		// Unreachable today (canDispatchToTarget validates spec first) -- must never report success.
		return errSQSQueuePathMalformed
	}

	queueARN := "arn:aws:sqs:" + region + ":" + segments[0] + ":" + segments[1]

	return h.sqsSender.SendMessageToQueue(ctx, queueARN, string(payload))
}

// errSQSQueuePathMalformed is returned when dispatchSQS's spec fails the
// "{accountId}/{queueName}" path-segment check.
var errSQSQueuePathMalformed = errors.New("apigateway: sqs integration service_api path is malformed")

// errSNSTopicArnUnresolved is returned when an sns action/Publish integration has no
// resolvable TopicArn -- neither via its RequestParameters mapping nor a TopicArn
// query parameter on the incoming request.
var errSNSTopicArnUnresolved = errors.New("apigateway: sns integration request has no resolvable TopicArn")

// dispatchSNS publishes payload as the message to the SNS topic resolved from the
// integration's RequestParameters mapping or, absent one, the incoming request's
// "TopicArn" query parameter -- real API Gateway supplies the topic ARN this way for
// action-style sns integrations, since it is not encoded in the integration URI.
func (h *Handler) dispatchSNS(ctx context.Context, r *http.Request, integration *Integration, payload []byte) error {
	topicARN := resolveRequestParamSource(r, integration.RequestParameters["integration.request.querystring.TopicArn"])
	if topicARN == "" {
		topicARN = r.URL.Query().Get("TopicArn")
	}

	if topicARN == "" {
		return errSNSTopicArnUnresolved
	}

	return h.snsPublisher.PublishToTopic(ctx, topicARN, string(payload))
}

// awsIntegrationURIFields is the colon-delimited field count of an AWS/AWS_PROXY
// integration URI matching arn:aws:apigateway:{region}:{service}:path|action/{service_api}.
const awsIntegrationURIFields = 6

// awsIntegrationTarget parses an AWS/AWS_PROXY integration URI of the documented form
//
//	arn:aws:apigateway:{region}:{subdomain.service|service}:path|action/{service_api}
//
// (aws-sdk-go-v2 service/apigateway/types/types.go, Integration.Uri godoc). It returns
// the region and service tokens, whether the service_api is "path" or "action" style,
// and spec: for "path" style the raw path after "path/"; for "action" style the action
// name only (any literal "&..." query params on the URI are dropped). Returns all
// empty strings if uri doesn't match this shape (e.g. a bare Lambda ARN/name, which the
// Lambda path already handles via ExtractLambdaFunctionName).
func awsIntegrationTarget(uri string) (string, string, string, string) {
	parts := strings.SplitN(uri, ":", awsIntegrationURIFields)
	if len(parts) != awsIntegrationURIFields {
		return "", "", "", ""
	}

	region, service := parts[3], parts[4]

	kind, rest, ok := strings.Cut(parts[5], "/")
	if !ok {
		return region, service, "", ""
	}

	if kind == "action" {
		action, _, _ := strings.Cut(rest, "&")

		return region, service, kind, action
	}

	return region, service, kind, rest
}

// canDispatchToTarget reports whether service/kind/spec (from awsIntegrationTarget)
// names a wired non-Lambda target this handler can dispatch to.
func (h *Handler) canDispatchToTarget(service, kind, spec string) bool {
	switch {
	case service == "sqs" && kind == "path" && h.sqsSender != nil:
		return sqsQueuePathValid(spec)
	case service == "sns" && kind == "action" && spec == "Publish" && h.snsPublisher != nil:
		return true
	default:
		return false
	}
}

// sqsQueuePathValid reports whether spec is a well-formed "{accountId}/{queueName}"
// path-style sqs service_api.
func sqsQueuePathValid(spec string) bool {
	segments := strings.Split(spec, "/")

	return len(segments) == sqsQueuePathSegments && segments[0] != "" && segments[1] != ""
}

// applyResponseTemplate selects the best-matching integration response by status code pattern
// (using regex selectionPattern), applies VTL response template and contentHandling conversion,
// and returns the rendered body and HTTP status code. Falls back to the raw response bytes and 200 if no match.
func (h *Handler) applyResponseTemplate(respBytes []byte, integration *Integration, requestID string) ([]byte, int) {
	if integration.IntegrationResponses == nil {
		return respBytes, http.StatusOK
	}

	// Try to find a matching integration response by selectionPattern (regex) against respBytes.
	// If no pattern matches, fall back to the "default" or "200" entry.
	ir := h.matchIntegrationResponse(integration.IntegrationResponses, string(respBytes))
	if ir == nil {
		return respBytes, http.StatusOK
	}

	statusCode := http.StatusOK
	if sc := parseStatusCode(ir.StatusCode); sc > 0 {
		statusCode = sc
	}

	body := respBytes
	tpl, ok := ir.ResponseTemplates[contentTypeJSON]

	if ok && tpl != "" {
		respVTLCtx := VTLContext{
			Body:      string(respBytes),
			RequestID: requestID,
		}
		body = []byte(RenderTemplate(tpl, respVTLCtx))
	}

	body = applyContentHandling(body, ir.ContentHandling)

	return body, statusCode
}

// applyContentHandling converts body bytes according to the AWS contentHandling setting.
// CONVERT_TO_BINARY: base64-decode the body (UTF-8 string → binary bytes).
// CONVERT_TO_TEXT:   base64-encode the body (binary bytes → base64 string).
// Empty string: pass through unchanged.
func applyContentHandling(body []byte, handling string) []byte {
	switch handling {
	case "CONVERT_TO_BINARY":
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
		if err != nil {
			return body
		}

		return decoded
	case "CONVERT_TO_TEXT":
		encoded := base64.StdEncoding.EncodeToString(body)

		return []byte(encoded)
	default:

		return body
	}
}

// matchIntegrationResponse finds the best-matching IntegrationResponse entry for the given body.
// Priority:
//  1. An entry whose selectionPattern regex matches the body (first match wins).
//  2. The "default" entry (empty selectionPattern treated as catch-all).
//  3. The "200" entry if it has no selectionPattern.
func (h *Handler) matchIntegrationResponse(
	responses map[string]*IntegrationResponse,
	body string,
) *IntegrationResponse {
	var defaultEntry *IntegrationResponse

	for _, ir := range responses {
		if ir == nil {
			continue
		}

		pat := ir.SelectionPattern
		if pat == "" {
			// Treat entries without a selection pattern as the default/catch-all.
			defaultEntry = ir

			continue
		}

		re := h.cachedRegexp(pat)
		if re == nil {
			continue
		}

		if re.MatchString(body) {
			return ir
		}
	}

	if defaultEntry != nil {
		return defaultEntry
	}

	// No pattern and no default: return nil.

	return nil
}

// cachedRegexp returns a compiled regexp for the pattern, using the handler's bounded
// LRU cache. Returns nil (also cached) if the pattern does not compile.
func (h *Handler) cachedRegexp(pattern string) *regexp.Regexp {
	if re, ok := h.selRegexpCache.get(pattern); ok {
		return re
	}

	re, compileErr := regexp.Compile(pattern)
	if compileErr != nil {
		h.selRegexpCache.put(pattern, nil)

		return nil
	}

	h.selRegexpCache.put(pattern, re)

	return re
}

// defaultRegexpCacheMaxEntries bounds the compiled selection-pattern regexp cache.
const defaultRegexpCacheMaxEntries = 1024

// regexpCache is a mutex-guarded LRU of compiled regexps keyed by user-supplied
// selection patterns. It caps its size so that unbounded distinct patterns cannot leak
// memory. A cached nil value records a pattern that failed to compile.
type regexpCache struct {
	entries    map[string]*list.Element
	order      *list.List
	mu         sync.Mutex
	maxEntries int
}

type regexpCacheEntry struct {
	re  *regexp.Regexp
	key string
}

func newRegexpCache(maxEntries int) *regexpCache {
	if maxEntries <= 0 {
		maxEntries = defaultRegexpCacheMaxEntries
	}

	return &regexpCache{
		entries:    make(map[string]*list.Element),
		order:      list.New(),
		maxEntries: maxEntries,
	}
}

func (c *regexpCache) get(pattern string) (*regexp.Regexp, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[pattern]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	entry, _ := elem.Value.(regexpCacheEntry)

	return entry.re, true
}

func (c *regexpCache) put(pattern string, re *regexp.Regexp) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.entries[pattern]; ok {
		elem.Value = regexpCacheEntry{re: re, key: pattern}
		c.order.MoveToFront(elem)

		return
	}

	elem := c.order.PushFront(regexpCacheEntry{re: re, key: pattern})
	c.entries[pattern] = elem

	for c.order.Len() > c.maxEntries {
		back := c.order.Back()
		if back == nil {
			break
		}
		entry, _ := back.Value.(regexpCacheEntry)
		delete(c.entries, entry.key)
		c.order.Remove(back)
	}
}

// handleHTTPProxy forwards the request to the target URI specified in the integration.
// Both HTTP and HTTP_PROXY integration types are handled identically: the request
// is forwarded as-is and the upstream response is returned directly to the caller.
func (h *Handler) handleHTTPProxy(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	integration *Integration,
) {
	//nolint:gosec // local emulation: integration URI is test-configured
	targetReq, err := http.NewRequestWithContext(
		ctx,
		r.Method,
		integration.URI,
		r.Body,
	)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "APIGateway HTTP proxy: bad integration URI",
			"uri", integration.URI, "error", err)
		http.Error(w, "Bad integration URI", http.StatusBadGateway)

		return
	}

	// Merge query parameters from the integration URI with the incoming request's query string.
	// This preserves any required query params baked into the integration URI.
	mergedQuery := targetReq.URL.Query()
	for key, values := range r.URL.Query() {
		for _, value := range values {
			mergedQuery.Add(key, value)
		}
	}
	targetReq.URL.RawQuery = mergedQuery.Encode()
	for k, vs := range r.Header {
		for _, v := range vs {
			targetReq.Header.Add(k, v)
		}
	}

	// Apply integration RequestParameters mappings (e.g. forward a method header as an integration header).
	if len(integration.RequestParameters) > 0 {
		applyIntegrationRequestParams(r, targetReq, integration.RequestParameters)
	}

	client := h.getHTTPClient()

	//nolint:gosec // local emulation: integration URI is test-configured
	resp, doErr := client.Do(targetReq)
	if doErr != nil {
		logger.Load(ctx).WarnContext(ctx, "APIGateway HTTP proxy: upstream request failed",
			"uri", integration.URI, "error", doErr)
		http.Error(w, "Upstream request failed", http.StatusBadGateway)

		return
	}

	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleMockIntegration returns a static response configured on the integration.
// It evaluates the first integrationResponse entry keyed by its status code.
// If no integrationResponses are configured, it defaults to HTTP 200 with an empty body.
func (h *Handler) handleMockIntegration(w http.ResponseWriter, integration *Integration) {
	statusCode, body, ir := mockResponseWithIR(integration)

	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Apply ResponseParameters from IntegrationResponse as response headers.
	if ir != nil {
		applyIntegrationResponseParams(w, ir.ResponseParameters)
	}

	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body)) //nolint:gosec // local emulation: mock integration body is test-configured
}

// mockResponseWithIR resolves the status code, body, and integration response for a MOCK integration.
func mockResponseWithIR(integration *Integration) (int, string, *IntegrationResponse) {
	statusCode := http.StatusOK

	ir := mockIntegrationResponse(integration)
	if ir == nil {
		return statusCode, "", nil
	}

	if sc := parseStatusCode(ir.StatusCode); sc > 0 {
		statusCode = sc
	}

	body := ""
	if ir.ResponseTemplates != nil {
		body = ir.ResponseTemplates["application/json"]
	}

	return statusCode, body, ir
}

// applyIntegrationResponseParams applies integration response parameter mappings as HTTP response headers.
// Parameters of the form "method.response.header.{name}: {value}" set response headers.
// The value can be "integration.response.header.{name}" (reads from integration headers, not yet
// available for MOCK integrations so treated as static) or a static string.
func applyIntegrationResponseParams(w http.ResponseWriter, params map[string]string) {
	const methodRespPrefix = "method.response.header."

	for dest, src := range params {
		if !strings.HasPrefix(dest, methodRespPrefix) {
			continue
		}

		headerName := dest[len(methodRespPrefix):]
		if headerName == "" {
			continue
		}

		// Resolve static values (or simplified integration header echo).
		value := resolveResponseParamSource(src)
		if value != "" {
			w.Header().Set(headerName, value)
		}
	}
}

// resolveResponseParamSource resolves an integration response parameter value.
// Static strings are returned as-is. integration.response.header.{name} references
// are returned as the raw name (for MOCK integrations there is no actual response to read from).
func resolveResponseParamSource(src string) string {
	const integRespPrefix = "integration.response.header."
	if strings.HasPrefix(src, integRespPrefix) {
		return src[len(integRespPrefix):]
	}

	return src
}

// mockIntegrationResponse returns the "200" integration response, if configured.
func mockIntegrationResponse(integration *Integration) *IntegrationResponse {
	if integration.IntegrationResponses == nil {
		return nil
	}

	ir, ok := integration.IntegrationResponses["200"]
	if !ok || ir == nil {
		return nil
	}

	return ir
}

// parseStatusCode converts a status-code string to an int; returns 0 on error.
func parseStatusCode(s string) int {
	const (
		minHTTP = 100
		maxHTTP = 599
		decBase = 10
	)

	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*decBase + int(c-'0')
	}

	if n < minHTTP || n > maxHTTP {
		return 0
	}

	return n
}

// ExtractLambdaFunctionName extracts a Lambda function name (or short ARN) from either:
//   - A plain function name: "my-function"
//   - A Lambda ARN: "arn:aws:lambda:region:account:function:my-function"
//   - An API Gateway invoke URI containing
//     "arn:aws:apigateway:region:lambda:path/.../functions/{lambdaArn}/invocations"
//
// Returns the input unchanged if it does not match any known pattern.
func ExtractLambdaFunctionName(uri string) string {
	// API Gateway integration URI: extract the Lambda ARN embedded in the path.
	// Format: arn:aws:apigateway:...:lambda:path/2015-03-31/functions/{lambdaArn}/invocations
	const invocations = "/invocations"
	if idx := strings.LastIndex(uri, invocations); idx != -1 {
		// Everything before "/invocations" is the Lambda ARN.
		lambdaARN := uri[:idx]
		// The Lambda ARN may itself be within a path like ".../functions/{arn}"
		const functionsPrefix = "/functions/"
		if fi := strings.LastIndex(lambdaARN, functionsPrefix); fi != -1 {
			lambdaARN = lambdaARN[fi+len(functionsPrefix):]
		}

		return ExtractLambdaFunctionName(lambdaARN)
	}

	// Lambda ARN: "arn:aws:lambda:{region}:{account}:function:{name}" (with optional qualifier).
	// Extract the name (and optional qualifier) after ":function:".
	// Use ":function:" (with leading colon) to avoid matching "function:" inside a function name.
	const functionSegment = ":function:"
	if fi := strings.LastIndex(uri, functionSegment); fi != -1 {
		return uri[fi+len(functionSegment):]
	}

	// Plain name or already-resolved value — return as-is.

	return uri
}

// applyIntegrationRequestParams applies integration request parameter mappings to the outgoing
// HTTP request. Mappings are of the form:
//
//	integration.request.{type}.{name}: method.request.{type}.{name}
//
// where type is header, querystring, or path. Only the header and querystring destination types
// are applied here (path substitution is handled in the URI template). Static string values
// (not starting with "method.request.") are also supported.
func applyIntegrationRequestParams(incoming *http.Request, outgoing *http.Request, params map[string]string) {
	outQuery := outgoing.URL.Query()

	for dest, src := range params {
		value := resolveRequestParamSource(incoming, src)
		if value == "" {
			continue
		}

		// Parse destination: "integration.request.{type}.{name}"
		const integPrefix = "integration.request."
		if !strings.HasPrefix(dest, integPrefix) {
			continue
		}

		rest := dest[len(integPrefix):]
		paramType, paramName, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}

		switch paramType {
		case paramLocationHeader:
			outgoing.Header.Set(paramName, value)
		case paramLocationQuery:
			outQuery.Set(paramName, value)
		}
	}

	outgoing.URL.RawQuery = outQuery.Encode()
}

// resolveRequestParamSource resolves a parameter source expression against the incoming request.
// Supported formats:
//   - method.request.header.{name}
//   - method.request.querystring.{name}
//   - method.request.path.{name}   (returns the raw path segment from the URL)
//   - Any other string is treated as a static value.
func resolveRequestParamSource(r *http.Request, src string) string {
	const methodPrefix = "method.request."
	if !strings.HasPrefix(src, methodPrefix) {
		return src
	}

	rest := src[len(methodPrefix):]
	srcType, srcName, ok := strings.Cut(rest, ".")
	if !ok {
		return ""
	}

	switch srcType {
	case paramLocationHeader:

		return r.Header.Get(srcName)
	case paramLocationQuery:

		return r.URL.Query().Get(srcName)
	case paramLocationPath:
		// Return the named path segment from the raw URL path.
		// This is a best-effort approximation: the actual value depends on route matching.
		segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for _, seg := range segments {
			// Caller must use exact segment value — path parameter names don't map here.
			if seg == srcName {
				return seg
			}
		}

		return ""
	}

	return ""
}
