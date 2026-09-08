package apigateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

var (
	errUnexpectedSigningMethod = errors.New("unexpected JWT signing method")
	errNoJWKSProvider          = errors.New("no JWKS provider configured")
)

// defaultAuthorizerTTL is the default authorizer result cache TTL (AWS default: 300 s).
const defaultAuthorizerTTL = 300 * time.Second

const defaultAuthorizerCacheMaxEntries = 1024

const defaultIdentitySource = "method.request.header.Authorization"

// jsonMessageKey is the JSON key AWS uses for the human-readable error text in every
// API Gateway error response body.
const jsonMessageKey = "message"

// maxProxyRequestBodyBytes caps API Gateway proxy request bodies. AWS limits the
// Lambda synchronous invoke payload to 6 MiB; bodies larger than that cannot be
// forwarded anyway, so cap reads to prevent unbounded io.ReadAll memory usage.
const maxProxyRequestBodyBytes = 6 * 1024 * 1024 // 6 MiB

// LambdaInvoker can invoke a Lambda function by name/ARN.
type LambdaInvoker interface {
	InvokeFunction(ctx context.Context, name, invocationType string, payload []byte) ([]byte, int, error)
}

// SQSSender can send a message to an SQS queue by ARN, for AWS integrations whose
// URI targets sqs (arn:aws:apigateway:{region}:sqs:path/{accountId}/{queueName}).
// Mirrors the SQSSender interface already declared by eventbridge, s3, and pipes --
// same consuming-service-declares-the-interface convention, wired in cli.go.
type SQSSender interface {
	SendMessageToQueue(ctx context.Context, queueARN, messageBody string) error
}

// SNSPublisher can publish a message to an SNS topic by ARN, for AWS integrations
// whose URI targets sns action/Publish. Mirrors the SNSPublisher interface already
// declared by cloudwatch, eventbridge, pipes, s3, and ses.
type SNSPublisher interface {
	PublishToTopic(ctx context.Context, topicARN, message string) error
}

// LambdaProxyEvent is the API Gateway Lambda proxy event format.
// https://docs.aws.amazon.com/apigateway/latest/developerguide/set-up-lambda-proxy-integrations.html
type LambdaProxyEvent struct {
	QueryStringParameters map[string]string   `json:"queryStringParameters,omitempty"`
	Headers               map[string]string   `json:"headers,omitempty"`
	MultiValueHeaders     map[string][]string `json:"multiValueHeaders,omitempty"`
	PathParameters        map[string]string   `json:"pathParameters,omitempty"`
	MultiValueQueryString map[string][]string `json:"multiValueQueryStringParameters,omitempty"`
	StageVariables        map[string]string   `json:"stageVariables,omitempty"`
	RequestContext        LambdaProxyContext  `json:"requestContext"`
	Resource              string              `json:"resource"`
	Path                  string              `json:"path"`
	HTTPMethod            string              `json:"httpMethod"`
	Body                  string              `json:"body,omitempty"`
	IsBase64Encoded       bool                `json:"isBase64Encoded"`
}

// LambdaProxyContext provides context for the Lambda proxy event.
type LambdaProxyContext struct {
	Authorizer   map[string]any `json:"authorizer,omitempty"`
	ResourcePath string         `json:"resourcePath"`
	HTTPMethod   string         `json:"httpMethod"`
	Stage        string         `json:"stage"`
	APIId        string         `json:"apiId"`
	RequestID    string         `json:"requestId,omitempty"`
}

type ctxKey int

const ctxKeyClaims ctxKey = 1

// LambdaProxyResponse is the response format from a Lambda proxy function.
type LambdaProxyResponse struct {
	Headers         map[string]string `json:"headers,omitempty"`
	Body            string            `json:"body,omitempty"`
	StatusCode      int               `json:"statusCode"`
	IsBase64Encoded bool              `json:"isBase64Encoded,omitempty"`
}

// BuildProxyEvent converts an incoming HTTP request to a Lambda proxy event.
// pathParameters are the path variable values extracted by the routing engine (may be nil).
func BuildProxyEvent(
	r *http.Request,
	apiID, stageName, resource, path string,
	pathParameters map[string]string,
) (*LambdaProxyEvent, error) {
	// Read body.
	var bodyStr string
	var isBase64 bool

	if r.Body != nil {
		bodyBytes, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxProxyRequestBodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		if utf8.Valid(bodyBytes) {
			bodyStr = string(bodyBytes)
		} else {
			bodyStr = base64.StdEncoding.EncodeToString(bodyBytes)
			isBase64 = true
		}
	}

	// Build headers map.
	headers := make(map[string]string)
	multiValueHeaders := make(map[string][]string)

	for k, vs := range r.Header {
		lower := strings.ToLower(k)
		headers[lower] = vs[len(vs)-1] // last value
		multiValueHeaders[lower] = vs
	}

	// Build query parameters.
	qsp := make(map[string]string)
	mqsp := make(map[string][]string)

	for k, vs := range r.URL.Query() {
		qsp[k] = vs[len(vs)-1]
		mqsp[k] = vs
	}

	var authorizer map[string]any
	if claims, ok := r.Context().Value(ctxKeyClaims).(jwt.MapClaims); ok {
		authorizer = map[string]any{
			"claims": claims,
		}
	}

	return &LambdaProxyEvent{
		HTTPMethod:            r.Method,
		Path:                  path,
		Resource:              resource,
		Headers:               headers,
		MultiValueHeaders:     multiValueHeaders,
		QueryStringParameters: qsp,
		MultiValueQueryString: mqsp,
		PathParameters:        pathParameters,
		Body:                  bodyStr,
		IsBase64Encoded:       isBase64,
		RequestContext: LambdaProxyContext{
			ResourcePath: resource,
			HTTPMethod:   r.Method,
			Stage:        stageName,
			APIId:        apiID,
			Authorizer:   authorizer,
		},
	}, nil
}

// handleProxyRequest handles a single HTTP request for a Lambda proxy integration.
func (h *Handler) handleProxyRequest(apiID, stageName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// AWS requires an explicit deployment before a stage is invocable, and rejects
		// any request whose {stage} segment doesn't name a real, deployed stage -- with
		// 403 "Missing Authentication Token", not 404. Gate on that here so an API with
		// resources/methods/integrations configured but never deployed (or invoked with a
		// made-up stage name) cannot be routed to.
		if _, err := h.Backend.GetStage(apiID, stageName); err != nil {
			writeMissingAuthenticationTokenResponse(w)

			return
		}

		// Resolve the routing trie (cached per resource-set version) and match.
		trie, err := h.routingTrie(apiID)
		if err != nil {
			logger.Load(ctx).ErrorContext(ctx, "APIGateway proxy: failed to get resources", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)

			return
		}

		// Match request path to resource path, extracting any path parameters.
		resource, pathParams := matchResourceTrie(trie, r.URL.Path, stageName)
		if resource == nil {
			writeMissingAuthenticationTokenResponse(w)

			return
		}

		// Handle CORS preflight OPTIONS request.
		if r.Method == http.MethodOptions {
			if resource.CorsConfiguration != nil {
				h.writeCORSPreflight(w, r, resource.CorsConfiguration)

				return
			}
		}

		// Attach CORS headers to the response when configured.
		if resource.CorsConfiguration != nil {
			h.addCORSHeaders(w, r, resource.CorsConfiguration)
		}

		// Apply method-level access controls (throttle, authorizer, request validator).
		denied := h.applyMethodControls(
			ctx, w, r, apiID, stageName, resource.ID, resource.Path, pathParams,
		)
		if denied {
			return
		}

		// Get the integration.
		integration, err := h.Backend.GetIntegration(apiID, resource.ID, r.Method)
		if err != nil {
			// Fall back to any method.
			integration, err = h.Backend.GetIntegration(apiID, resource.ID, "ANY")
			if err != nil {
				writeMissingAuthenticationTokenResponse(w)

				return
			}
		}

		h.dispatchIntegration(ctx, w, r, apiID, stageName, resource, integration, pathParams)
	}
}

// applyMethodControls runs the throttle, authorizer, and request validator checks for the
// matched method. Returns true if the request was denied and the response has already been
// written.
func (h *Handler) applyMethodControls(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	apiID, stageName, resourceID, resourcePath string,
	pathParams map[string]string,
) bool {
	method, methodErr := h.Backend.GetMethod(apiID, resourceID, r.Method)
	if methodErr != nil {
		method, methodErr = h.Backend.GetMethod(apiID, resourceID, "ANY")
	}

	if methodErr != nil || method == nil {
		return false
	}

	if method.APIKeyRequired {
		if h.enforceAPIKey(ctx, w, r, apiID, stageName) {
			return true
		}
	}

	// Stage MethodSettings throttling: the tier below usage-plan per-client/per-method
	// limits (api-gateway-request-throttling.html's precedence list), and the only
	// throttle path that fires for traffic that isn't apiKeyRequired.
	if h.enforceMethodThrottle(ctx, w, apiID, stageName, resourcePath, r.Method) {
		return true
	}

	if method.AuthorizerID != "" {
		if h.runAuthorizer(ctx, w, r, apiID, stageName, method.AuthorizerID) {
			return true
		}
	}

	if method.RequestValidatorID != "" {
		if h.runRequestValidator(ctx, w, r, apiID, method, pathParams) {
			return true
		}
	}

	return false
}

// enforceAPIKey validates the x-api-key header against enabled API keys and, when the
// key is associated with a usage plan for the stage, enforces the plan's quota and
// rate/burst throttle. Returns true if the request was denied (response already
// written).
func (h *Handler) enforceAPIKey(
	ctx context.Context, w http.ResponseWriter, r *http.Request, apiID, stageName string,
) bool {
	keyValue := r.Header.Get("X-Api-Key")
	if keyValue == "" {
		logger.Load(ctx).InfoContext(ctx, "APIGateway proxy: missing x-api-key", "apiId", apiID)
		http.Error(w, "Forbidden", http.StatusForbidden)

		return true
	}

	apiKey, err := h.Backend.GetAPIKeyByValue(keyValue)
	if err != nil || apiKey == nil {
		logger.Load(ctx).InfoContext(ctx, "APIGateway proxy: invalid x-api-key", "apiId", apiID)
		http.Error(w, "Forbidden", http.StatusForbidden)

		return true
	}

	if !apiKey.Enabled {
		logger.Load(ctx).InfoContext(ctx, "APIGateway proxy: disabled API key", "apiId", apiID, "keyId", apiKey.ID)
		http.Error(w, "Forbidden", http.StatusForbidden)

		return true
	}

	return h.enforceUsagePlan(ctx, w, apiID, stageName, apiKey.ID)
}

// enforceUsagePlan applies usage-plan quota/throttle limits for the key and writes the
// AWS-accurate 429 response when a limit is exceeded. Returns true when the request
// was denied.
func (h *Handler) enforceUsagePlan(
	ctx context.Context, w http.ResponseWriter, apiID, stageName, keyID string,
) bool {
	err := h.Backend.EnforceUsagePlan(apiID, stageName, keyID)
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrQuotaExceeded):
		logger.Load(ctx).InfoContext(ctx, "APIGateway proxy: usage-plan quota exceeded",
			"apiId", apiID, "keyId", keyID)
		writeThrottleResponse(w, "LimitExceededException", "Limit Exceeded")

		return true
	case errors.Is(err, ErrThrottled):
		logger.Load(ctx).InfoContext(ctx, "APIGateway proxy: usage-plan throttle exceeded",
			"apiId", apiID, "keyId", keyID)
		writeThrottleResponse(w, "TooManyRequestsException", "Too Many Requests")

		return true
	default:
		logger.Load(ctx).WarnContext(ctx, "APIGateway proxy: usage-plan enforcement error", "error", err)

		return false
	}
}

// enforceMethodThrottle applies a stage's MethodSettings throttling and writes the
// AWS-accurate 429 response when the limit is exceeded. Returns true when the request was
// denied.
func (h *Handler) enforceMethodThrottle(
	ctx context.Context, w http.ResponseWriter, apiID, stageName, resourcePath, httpMethod string,
) bool {
	err := h.Backend.EnforceMethodThrottle(apiID, stageName, resourcePath, httpMethod)
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrThrottled):
		logger.Load(ctx).InfoContext(ctx, "APIGateway proxy: stage method-setting throttle exceeded",
			"apiId", apiID, "stage", stageName, "resourcePath", resourcePath, "method", httpMethod)
		writeThrottleResponse(w, "TooManyRequestsException", "Too Many Requests")

		return true
	default:
		logger.Load(ctx).WarnContext(ctx, "APIGateway proxy: method-throttle enforcement error", "error", err)

		return false
	}
}

// writeThrottleResponse writes the AWS API Gateway 429 error body and x-amzn-ErrorType
// header used for quota (LimitExceededException) and throttle (TooManyRequestsException)
// rejections.
func writeThrottleResponse(w http.ResponseWriter, errorType, message string) {
	w.Header().Set(headerContentType, "application/json")
	w.Header().Set("X-Amzn-Errortype", errorType)
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{jsonMessageKey: message})
}

// writeMissingAuthenticationTokenResponse writes AWS API Gateway's real response for a
// request that never resolves to a deployed stage + matching resource + method: HTTP 403
// with a "Missing Authentication Token" body and x-amzn-errortype header, not a 404. AWS
// returns this for an invalid or undeployed stage, an unmatched resource path, or a
// resource with no method for the request's HTTP verb -- the error fires before
// authentication is ever considered, despite its name.
func writeMissingAuthenticationTokenResponse(w http.ResponseWriter) {
	w.Header().Set(headerContentType, "application/json")
	w.Header().Set("X-Amzn-Errortype", "MissingAuthenticationTokenException")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{jsonMessageKey: "Missing Authentication Token"})
}

// dispatchIntegration routes the request to the appropriate integration handler.
func (h *Handler) dispatchIntegration(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	apiID, stageName string,
	resource *Resource,
	integration *Integration,
	pathParams map[string]string,
) {
	// Interpolate stage variables (${stageVariables.X}) in integration URI/templates.
	stageVars := h.stageVars(apiID, stageName)
	if len(stageVars) > 0 {
		resolved := *integration
		resolved.URI = interpolateStageVars(integration.URI, stageVars)
		if len(integration.RequestTemplates) > 0 {
			resolved.RequestTemplates = make(map[string]string, len(integration.RequestTemplates))
			for k, v := range integration.RequestTemplates {
				resolved.RequestTemplates[k] = interpolateStageVars(v, stageVars)
			}
		}
		integration = &resolved
	}

	switch integration.Type {
	case "AWS_PROXY":
		h.handleAWSProxy(ctx, w, r, apiID, stageName, resource, integration, pathParams)
	case "AWS":
		h.handleAWSIntegration(ctx, w, r, apiID, stageName, resource, stageVars, integration)
	case "HTTP", "HTTP_PROXY":
		h.handleHTTPProxy(ctx, w, r, integration)
	case IntegrationTypeMock:
		h.handleMockIntegration(w, integration)
	default:
		http.Error(w, "Unsupported or unknown integration type for stage URL", http.StatusNotImplemented)
	}
}

// stageVars fetches stage variables for the given API/stage (returns nil on error).
func (h *Handler) stageVars(apiID, stageName string) map[string]string {
	stage, err := h.Backend.GetStage(apiID, stageName)
	if err != nil || stage == nil {
		return nil
	}

	return stage.Variables
}

// interpolateStageVars substitutes ${stageVariables.X} placeholders in s with values from vars.
func interpolateStageVars(s string, vars map[string]string) string {
	if len(vars) == 0 || !strings.Contains(s, "${stageVariables.") {
		return s
	}

	for k, v := range vars {
		s = strings.ReplaceAll(s, "${stageVariables."+k+"}", v)
	}

	return s
}
