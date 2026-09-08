package apigatewayv2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// Data-plane authorization messages mirror what real API Gateway v2 returns on
// the execute-api endpoint for failed authorization.
const (
	msgUnauthorized      = "Unauthorized"
	msgForbidden         = "Forbidden"
	msgExplicitDeny      = "User is not authorized to access this resource with an explicit deny"
	msgMissingAuthToken  = "Missing Authentication Token"
	msgAuthConfigInvalid = "Internal server error"

	// execute-api invoke action used in Lambda authorizer IAM policy documents.
	executeAPIInvokeAction = "execute-api:Invoke"
)

// authDecision is a cached authorizer result.
type authDecision struct {
	expireAt time.Time
	allow    bool
}

// authorizerCache caches Lambda authorizer decisions keyed by authorizer id and
// identity-source value, honouring the authorizer's result TTL. It is safe for
// concurrent use, which matters because the data plane authorizes requests on
// many goroutines at once.
type authorizerCache struct {
	m  map[string]authDecision
	mu sync.Mutex
}

func newAuthorizerCache() *authorizerCache {
	return &authorizerCache{m: make(map[string]authDecision)}
}

// get returns the cached decision for key when present and unexpired.
func (a *authorizerCache) get(key string) (bool, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	d, present := a.m[key]
	if !present {
		return false, false
	}

	if time.Now().After(d.expireAt) {
		delete(a.m, key)

		return false, false
	}

	return d.allow, true
}

// put stores a decision for key with the given TTL. A non-positive TTL disables
// caching for that entry (AWS treats a 0 TTL as "do not cache").
func (a *authorizerCache) put(key string, allow bool, ttl time.Duration) {
	if ttl <= 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.m[key] = authDecision{allow: allow, expireAt: time.Now().Add(ttl)}
}

// reset clears the entire cache. Used by ResetAuthorizersCache.
func (a *authorizerCache) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.m = make(map[string]authDecision)
}

// purge removes every cached decision for authorizerID (all identity-source
// values), used when the authorizer itself -- or the API owning it -- is
// deleted, so stale decisions don't linger in memory until their TTL expires
// (bd: gopherstack-wmh).
func (a *authorizerCache) purge(authorizerID string) {
	prefix := authorizerID + "\n"

	a.mu.Lock()
	defer a.mu.Unlock()

	for k := range a.m {
		if strings.HasPrefix(k, prefix) {
			delete(a.m, k)
		}
	}
}

// requestAuthorizerEventV2 is the payload-format-2.0 event delivered to a
// REQUEST (Lambda) authorizer for an HTTP API.
type requestAuthorizerEventV2 struct {
	Headers               map[string]string     `json:"headers,omitempty"`
	QueryStringParameters map[string]string     `json:"queryStringParameters,omitempty"`
	RequestContext        httpAPIRequestContext `json:"requestContext"`
	Version               string                `json:"version"`
	Type                  string                `json:"type"`
	RouteArn              string                `json:"routeArn"`
	RouteKey              string                `json:"routeKey"`
	RawPath               string                `json:"rawPath"`
	RawQueryString        string                `json:"rawQueryString"`
	IdentitySource        []string              `json:"identitySource"`
}

// requestAuthorizerEventV1 is the payload-format-1.0 event delivered to a
// REQUEST authorizer (IAM-policy response).
type requestAuthorizerEventV1 struct {
	Headers               map[string]string       `json:"headers,omitempty"`
	QueryStringParameters map[string]string       `json:"queryStringParameters,omitempty"`
	RequestContext        httpAPIV1RequestContext `json:"requestContext"`
	Type                  string                  `json:"type"`
	MethodArn             string                  `json:"methodArn"`
	Resource              string                  `json:"resource"`
	Path                  string                  `json:"path"`
	HTTPMethod            string                  `json:"httpMethod"`
	IdentitySource        []string                `json:"identitySource"`
}

// simpleAuthorizerResponse is the payload-2.0 simple-response shape returned by
// an authorizer with EnableSimpleResponses.
type simpleAuthorizerResponse struct {
	Context      map[string]any `json:"context,omitempty"`
	IsAuthorized bool           `json:"isAuthorized"`
}

// iamPolicyResponse is the IAM-policy-document response shape returned by a
// REQUEST authorizer that does not use simple responses.
type iamPolicyResponse struct {
	Context        map[string]any `json:"context,omitempty"`
	PrincipalID    string         `json:"principalId"`
	PolicyDocument iamPolicyDoc   `json:"policyDocument"`
}

type iamPolicyDoc struct {
	Version   string          `json:"Version"`
	Statement []iamPolicyStmt `json:"Statement"`
}

type iamPolicyStmt struct {
	Effect   string          `json:"Effect"`
	Action   flexibleStrings `json:"Action"`
	Resource flexibleStrings `json:"Resource"`
}

// flexibleStrings decodes a JSON value that may be either a single string or an
// array of strings (both are valid in an IAM policy element).
type flexibleStrings []string

func (f *flexibleStrings) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		*f = nil

		return nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var arr []string
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}

		*f = arr

		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	*f = []string{s}

	return nil
}

// buildRouteArn constructs the execute-api ARN for a matched route, used as the
// routeArn/methodArn in authorizer events and as the resource an IAM policy is
// evaluated against.
func buildRouteArn(apiID, stage, method, resourcePath string) string {
	if resourcePath == "" {
		resourcePath = "/"
	}

	if !strings.HasPrefix(resourcePath, "/") {
		resourcePath = "/" + resourcePath
	}

	// arn:aws:execute-api:region:account:apiId/stage/METHOD/resource-path
	return "arn:aws:execute-api:" + defaultRegion + ":" + config.DefaultAccountID + ":" +
		apiID + "/" + stage + "/" + method + resourcePath
}

// enforceRequestAuthorizer invokes a REQUEST (Lambda) authorizer for the matched
// route and returns a non-nil, unwritten error when the request is not
// authorized; the caller maps and writes it exactly once. It supports both the
// simple (isAuthorized) and IAM-policy response shapes, honours identity-source
// extraction, and caches decisions for the authorizer's result TTL.
func (h *Handler) enforceRequestAuthorizer(
	c *echo.Context,
	apiID, stageName string,
	route *Route,
	auth *Authorizer,
	resourcePath string,
) error {
	log := logger.Load(c.Request().Context())
	req := c.Request()

	if h.lambdaInvoker == nil {
		log.Error("apigatewayv2: no lambda invoker configured for REQUEST authorizer")

		return errRouteAuthConfigInvalid
	}

	// Extract identity sources. AWS requires every configured identity source to
	// be present; a missing one yields 401 without invoking the function.
	idValues, allPresent := extractIdentitySources(req, auth.IdentitySource)
	if len(auth.IdentitySource) > 0 && !allPresent {
		log.Info("apigatewayv2: REQUEST authorizer missing identity source", "authorizerId", auth.AuthorizerID)

		return errRouteUnauthorized
	}

	cacheKey := auth.AuthorizerID + "\n" + strings.Join(idValues, "\n")
	if allow, ok := h.authCache.get(cacheKey); ok {
		return finishAuthDecision(allow)
	}

	method := req.Method
	routeArn := buildRouteArn(apiID, stageName, method, resourcePath)

	payload, buildErr := buildAuthorizerPayload(req, auth, apiID, stageName, route.RouteKey, resourcePath, routeArn)
	if buildErr != nil {
		log.Error("apigatewayv2: failed to build authorizer payload", "error", buildErr)

		return errRouteAuthConfigInvalid
	}

	authArn := extractHTTPAPILambdaARN(auth.AuthorizerURI)

	respBytes, _, invokeErr := h.lambdaInvoker.InvokeFunction(req.Context(), authArn, "RequestResponse", payload)
	if invokeErr != nil {
		log.Error("apigatewayv2: REQUEST authorizer invocation failed", "arn", authArn, "error", invokeErr)

		return errRouteAuthConfigInvalid
	}

	allow, denyExplicit := evaluateAuthorizerResponse(respBytes, auth, routeArn)

	ttl := time.Duration(auth.AuthorizerResultTTLInSeconds) * time.Second
	h.authCache.put(cacheKey, allow, ttl)

	if !allow {
		if denyExplicit {
			return errRouteExplicitDeny
		}

		return errRouteForbidden
	}

	return nil
}

// finishAuthDecision returns the unwritten rejection error for a cached
// decision, or nil when allowed.
func finishAuthDecision(allow bool) error {
	if allow {
		return nil
	}

	return errRouteForbidden
}

// buildAuthorizerPayload marshals the correct authorizer event for the
// authorizer's payload format version.
func buildAuthorizerPayload(
	req *http.Request,
	auth *Authorizer,
	apiID, stageName, routeKey, resourcePath, routeArn string,
) ([]byte, error) {
	headers := buildHTTPAPIHeaders(req)

	qsp := make(map[string]string, len(req.URL.Query()))
	for k, vs := range req.URL.Query() {
		qsp[k] = strings.Join(vs, ",")
	}

	if authorizerUsesV1Payload(auth) {
		event := requestAuthorizerEventV1{
			Type:                  authorizerTypeRequest,
			MethodArn:             routeArn,
			Resource:              resourcePath,
			Path:                  resourcePath,
			HTTPMethod:            req.Method,
			Headers:               headers,
			QueryStringParameters: qsp,
			IdentitySource:        auth.IdentitySource,
			RequestContext: httpAPIV1RequestContext{
				ResourcePath: routeKey,
				HTTPMethod:   req.Method,
				Stage:        stageName,
				APIId:        apiID,
			},
		}

		return json.Marshal(event)
	}

	event := requestAuthorizerEventV2{
		Version:               payloadFormatV2,
		Type:                  authorizerTypeRequest,
		RouteArn:              routeArn,
		RouteKey:              routeKey,
		RawPath:               resourcePath,
		RawQueryString:        req.URL.RawQuery,
		Headers:               headers,
		QueryStringParameters: qsp,
		IdentitySource:        auth.IdentitySource,
		RequestContext: httpAPIRequestContext{
			AccountID:  config.DefaultAccountID,
			APIID:      apiID,
			DomainName: apiID + ".execute-api." + defaultRegion + ".amazonaws.com",
			RouteKey:   routeKey,
			Stage:      stageName,
			HTTP: httpAPIHTTPContext{
				Method:   req.Method,
				Path:     resourcePath,
				Protocol: req.Proto,
				SourceIP: realIP(req),
			},
		},
	}

	return json.Marshal(event)
}

// authorizerUsesV1Payload reports whether the authorizer expects the 1.0 event
// (and therefore returns an IAM policy document). Simple responses require 2.0.
func authorizerUsesV1Payload(auth *Authorizer) bool {
	return auth.AuthorizerPayloadFormatVersion == payloadFormatV1
}

// evaluateAuthorizerResponse interprets an authorizer's Lambda response and
// returns whether the request is allowed and whether the denial is explicit.
func evaluateAuthorizerResponse(respBytes []byte, auth *Authorizer, routeArn string) (bool, bool) {
	// Simple response: only valid with payload 2.0 + EnableSimpleResponses.
	if auth.EnableSimpleResponses && !authorizerUsesV1Payload(auth) {
		var simple simpleAuthorizerResponse
		if err := json.Unmarshal(respBytes, &simple); err == nil {
			return simple.IsAuthorized, false
		}

		return false, false
	}

	// IAM policy response.
	var policy iamPolicyResponse
	if err := json.Unmarshal(respBytes, &policy); err != nil {
		return false, false
	}

	return evaluateIAMPolicy(policy.PolicyDocument.Statement, routeArn)
}

// evaluateIAMPolicy evaluates execute-api:Invoke statements against the route
// ARN. An explicit Deny always wins; otherwise a matching Allow grants access.
// With no matching statement the request is denied (implicit deny).
func evaluateIAMPolicy(statements []iamPolicyStmt, routeArn string) (bool, bool) {
	matchedAllow := false

	for _, stmt := range statements {
		if !statementCoversInvoke(stmt) {
			continue
		}

		if !anyResourceMatches(stmt.Resource, routeArn) {
			continue
		}

		if strings.EqualFold(stmt.Effect, "Deny") {
			return false, true
		}

		if strings.EqualFold(stmt.Effect, "Allow") {
			matchedAllow = true
		}
	}

	return matchedAllow, false
}

// statementCoversInvoke reports whether a statement's Action list covers the
// execute-api:Invoke action (directly or via wildcard).
func statementCoversInvoke(stmt iamPolicyStmt) bool {
	if len(stmt.Action) == 0 {
		return true
	}

	for _, a := range stmt.Action {
		if a == "*" || a == executeAPIInvokeAction || a == "execute-api:*" || arnGlobMatch(a, executeAPIInvokeAction) {
			return true
		}
	}

	return false
}

// anyResourceMatches reports whether any resource pattern matches the route ARN.
func anyResourceMatches(resources flexibleStrings, routeArn string) bool {
	for _, r := range resources {
		if arnGlobMatch(r, routeArn) {
			return true
		}
	}

	return false
}

// arnGlobMatch matches an ARN pattern containing '*' (any sequence) and '?'
// (any single char) wildcards against a concrete value, as AWS IAM does.
func arnGlobMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}

	return globMatch(pattern, value)
}

// globMatch implements '*'/'?' wildcard matching without regex allocation.
func globMatch(pattern, s string) bool {
	// Iterative backtracking matcher.
	var (
		px, sx         int
		starIdx        = -1
		matchIdx       int
		plen, slentext = len(pattern), len(s)
	)

	for sx < slentext {
		switch {
		case px < plen && (pattern[px] == '?' || pattern[px] == s[sx]):
			px++
			sx++
		case px < plen && pattern[px] == '*':
			starIdx = px
			matchIdx = sx
			px++
		case starIdx != -1:
			px = starIdx + 1
			matchIdx++
			sx = matchIdx
		default:
			return false
		}
	}

	for px < plen && pattern[px] == '*' {
		px++
	}

	return px == plen
}

// extractIdentitySources resolves each configured identity-source expression to
// a value. It returns the ordered values and whether every source was present
// and non-empty.
func extractIdentitySources(r *http.Request, sources []string) ([]string, bool) {
	allPresent := true
	values := make([]string, 0, len(sources))

	for _, src := range sources {
		v := resolveIdentitySource(r, src)
		if v == "" {
			allPresent = false
		}

		values = append(values, v)
	}

	return values, allPresent
}

// resolveIdentitySource resolves a single identity-source expression (header,
// query string, or stage variable) to its value.
func resolveIdentitySource(r *http.Request, src string) string {
	if name, ok := strings.CutPrefix(src, "$request.header."); ok {
		return r.Header.Get(name)
	}

	if qname, ok := strings.CutPrefix(src, "$request.querystring."); ok {
		return r.URL.Query().Get(qname)
	}

	if cname, ok := strings.CutPrefix(src, "$request.cookie."); ok {
		if ck, err := r.Cookie(cname); err == nil {
			return ck.Value
		}

		return ""
	}

	return ""
}

// enforceIAMAuth enforces AWS_IAM authorization on the data plane. Full SigV4
// verification requires the caller's secret key, which the data plane does not
// hold, so this enforces the observable AWS contract: an unsigned request (no
// SigV4 Authorization header and no session-token header) is rejected -- with
// an unwritten error the caller maps to 403 "Missing Authentication Token" and
// writes exactly once, exactly as real API Gateway returns it.
func (h *Handler) enforceIAMAuth(c *echo.Context) error {
	req := c.Request()

	authz := req.Header.Get("Authorization")
	if strings.HasPrefix(authz, "AWS4-HMAC-SHA256") {
		return nil
	}

	// Presigned requests carry the algorithm in the query string instead.
	if req.URL.Query().Get("X-Amz-Algorithm") == "AWS4-HMAC-SHA256" {
		return nil
	}

	logger.Load(req.Context()).Info("apigatewayv2: AWS_IAM route rejected unsigned request")

	return errRouteMissingAuthToken
}

// CreateAuthorizer creates a new authorizer for an API.
func (b *InMemoryBackend) CreateAuthorizer(apiID string, input CreateAuthorizerInput) (*Authorizer, error) {
	b.mu.Lock("CreateAuthorizer")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrBadRequest)
	}

	validTypes := map[string]bool{authorizerTypeJWT: true, "REQUEST": true, "CUSTOM": true}
	if !validTypes[input.AuthorizerType] {
		return nil, fmt.Errorf("%w: authorizerType must be JWT, REQUEST, or CUSTOM", ErrBadRequest)
	}

	// Validate JWT authorizer requires JwtConfiguration.
	if input.AuthorizerType == authorizerTypeJWT && input.JwtConfiguration == nil {
		return nil, fmt.Errorf("%w: jwtConfiguration is required for JWT authorizers", ErrBadRequest)
	}

	// Apply AWS-realistic defaults for IdentitySource.
	identitySource := input.IdentitySource
	if len(identitySource) == 0 && input.AuthorizerType == authorizerTypeJWT {
		identitySource = []string{"$request.header.Authorization"}
	}

	id := randomID()
	authorizer := &Authorizer{
		AuthorizerID:                   id,
		APIID:                          apiID,
		Name:                           input.Name,
		AuthorizerType:                 input.AuthorizerType,
		AuthorizerURI:                  input.AuthorizerURI,
		IdentitySource:                 identitySource,
		AuthorizerCredentialsArn:       input.AuthorizerCredentialsArn,
		AuthorizerResultTTLInSeconds:   input.AuthorizerResultTTLInSeconds,
		AuthorizerPayloadFormatVersion: input.AuthorizerPayloadFormatVersion,
		EnableSimpleResponses:          input.EnableSimpleResponses,
	}

	if input.JwtConfiguration != nil {
		clone := *input.JwtConfiguration
		authorizer.JwtConfiguration = &clone
	}

	b.authorizers.Put(authorizer)

	cp := *authorizer

	return &cp, nil
}

// GetAuthorizer retrieves an authorizer by ID.
func (b *InMemoryBackend) GetAuthorizer(apiID, authorizerID string) (*Authorizer, error) {
	b.mu.RLock("GetAuthorizer")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	a, ok := b.authorizers.Get(authorizerKey(apiID, authorizerID))
	if !ok {
		return nil, ErrAuthorizerNotFound
	}

	cp := *a

	return &cp, nil
}

// GetAuthorizers retrieves all authorizers for an API.
func (b *InMemoryBackend) GetAuthorizers(apiID string) ([]Authorizer, error) {
	b.mu.RLock("GetAuthorizers")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	authorizers := b.authorizersByAPI.Get(apiID)
	result := make([]Authorizer, 0, len(authorizers))

	for _, a := range authorizers {
		result = append(result, *a)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].AuthorizerID < result[j].AuthorizerID
	})

	return result, nil
}

// DeleteAuthorizer removes an authorizer from an API.
func (b *InMemoryBackend) DeleteAuthorizer(apiID, authorizerID string) error {
	b.mu.Lock("DeleteAuthorizer")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	if !b.authorizers.Delete(authorizerKey(apiID, authorizerID)) {
		return ErrAuthorizerNotFound
	}

	return nil
}

// UpdateAuthorizer updates fields on an existing authorizer.
func (b *InMemoryBackend) UpdateAuthorizer(
	apiID, authorizerID string,
	input UpdateAuthorizerInput,
) (*Authorizer, error) {
	b.mu.Lock("UpdateAuthorizer")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	a, ok := b.authorizers.Get(authorizerKey(apiID, authorizerID))
	if !ok {
		return nil, ErrAuthorizerNotFound
	}

	if input.Name != nil {
		if *input.Name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", ErrBadRequest)
		}

		a.Name = *input.Name
	}

	if input.AuthorizerType != "" {
		a.AuthorizerType = input.AuthorizerType
	}

	if input.AuthorizerURI != nil {
		a.AuthorizerURI = *input.AuthorizerURI
	}

	if len(input.IdentitySource) > 0 {
		a.IdentitySource = input.IdentitySource
	}

	if input.AuthorizerCredentialsArn != nil {
		a.AuthorizerCredentialsArn = *input.AuthorizerCredentialsArn
	}

	if input.AuthorizerResultTTLInSeconds != nil {
		a.AuthorizerResultTTLInSeconds = *input.AuthorizerResultTTLInSeconds
	}

	if input.AuthorizerPayloadFormatVersion != nil {
		a.AuthorizerPayloadFormatVersion = *input.AuthorizerPayloadFormatVersion
	}

	if input.EnableSimpleResponses != nil {
		a.EnableSimpleResponses = *input.EnableSimpleResponses
	}

	if input.JwtConfiguration != nil {
		clone := *input.JwtConfiguration
		a.JwtConfiguration = &clone
	}

	cp := *a

	return &cp, nil
}

// ResetAuthorizersCache is a no-op for the in-memory backend (caching is not simulated).
func (b *InMemoryBackend) ResetAuthorizersCache(apiID, stageName string) error {
	b.mu.RLock("ResetAuthorizersCache")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	if !b.stages.Has(stageKey(apiID, stageName)) {
		return ErrStageNotFound
	}

	return nil
}
