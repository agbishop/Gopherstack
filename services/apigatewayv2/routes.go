package apigatewayv2

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// validRouteAuthorizationType reports whether t is a valid route AuthorizationType
// for API Gateway v2 (the AWS-modeled enum: NONE, AWS_IAM, JWT, CUSTOM).
func validRouteAuthorizationType(t string) bool {
	switch t {
	case authorizationTypeNone, authorizationTypeAWSIAM, authorizerTypeJWT, authorizationTypeCustom:
		return true
	default:
		return false
	}
}

// isValidHTTPRouteKeyMethod reports whether method is accepted in an HTTP API route key.
func isValidHTTPRouteKeyMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", httpMethodAny:
		return true
	default:
		return false
	}
}

// validateHTTPRouteKey returns ErrBadRequest if key is invalid for an HTTP API.
// Valid forms: "$default" or "METHOD /path" (e.g. "GET /items").
func validateHTTPRouteKey(key string) error {
	if key == "$default" {
		return nil
	}

	const maxParts = 2
	parts := strings.SplitN(key, " ", maxParts)
	if len(parts) != maxParts || !isValidHTTPRouteKeyMethod(parts[0]) ||
		!strings.HasPrefix(parts[1], "/") {
		return fmt.Errorf(
			"%w: routeKey must be $default or start with a valid HTTP method and a forward slash, e.g. GET /items",
			ErrBadRequest,
		)
	}

	return nil
}

// CreateRoute creates a new route for an API.
func (b *InMemoryBackend) CreateRoute(apiID string, input CreateRouteInput) (*Route, error) {
	b.mu.Lock("CreateRoute")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, ErrAPINotFound
	}

	if input.RouteKey == "" {
		return nil, fmt.Errorf("%w: routeKey is required", ErrBadRequest)
	}

	if api.ProtocolType == protocolTypeHTTP {
		if err := validateHTTPRouteKey(input.RouteKey); err != nil {
			return nil, err
		}
	}

	for _, existing := range b.routesByAPI.Get(apiID) {
		if existing.RouteKey == input.RouteKey {
			return nil, fmt.Errorf(
				"%w: route key %q already exists",
				ErrAlreadyExists,
				input.RouteKey,
			)
		}
	}

	authType := input.AuthorizationType
	if authType == "" {
		authType = authorizationTypeNone
	}

	if !validRouteAuthorizationType(authType) {
		return nil, fmt.Errorf(
			"%w: authorizationType must be one of NONE, AWS_IAM, JWT, CUSTOM", ErrBadRequest,
		)
	}

	if (authType == authorizerTypeJWT || authType == authorizationTypeCustom) &&
		input.AuthorizerID == "" {
		return nil, fmt.Errorf(
			"%w: authorizerId is required for %s authorization",
			ErrBadRequest,
			authType,
		)
	}

	authScopes := input.AuthorizationScopes
	if authScopes == nil {
		authScopes = []string{}
	}

	id := randomID()
	route := &Route{
		RouteID:                  id,
		APIID:                    apiID,
		RouteKey:                 input.RouteKey,
		Target:                   input.Target,
		AuthorizationType:        authType,
		AuthorizerID:             input.AuthorizerID,
		OperationName:            input.OperationName,
		ModelSelectionExpression: input.ModelSelectionExpression,
		RequestModels:            input.RequestModels,
		RequestParameters:        input.RequestParameters,
		AuthorizationScopes:      authScopes,
		APIKeyRequired:           input.APIKeyRequired,
	}

	b.routes.Put(route)
	b.autoDeployLocked(apiID)

	cp := *route

	return &cp, nil
}

// GetRoute retrieves a route by ID.
func (b *InMemoryBackend) GetRoute(apiID, routeID string) (*Route, error) {
	b.mu.RLock("GetRoute")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	r, ok := b.routes.Get(routeKey(apiID, routeID))
	if !ok {
		return nil, ErrRouteNotFound
	}

	cp := *r

	return &cp, nil
}

// GetRoutes retrieves all routes for an API.
func (b *InMemoryBackend) GetRoutes(apiID string) ([]Route, error) {
	b.mu.RLock("GetRoutes")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	routes := b.routesByAPI.Get(apiID)
	result := make([]Route, 0, len(routes))

	for _, r := range routes {
		result = append(result, *r)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].RouteID < result[j].RouteID
	})

	return result, nil
}

// DeleteRoute removes a route from an API.
func (b *InMemoryBackend) DeleteRoute(apiID, routeID string) error {
	b.mu.Lock("DeleteRoute")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	r, ok := b.routes.Get(routeKey(apiID, routeID))
	if !ok {
		return ErrRouteNotFound
	}

	b.routes.Delete(routeKey(apiID, routeID))

	for _, rr := range slices.Clone(b.routeResponsesByRoute.Get(routeKey(apiID, routeID))) {
		b.routeResponses.Delete(routeResponseKey(apiID, routeID, rr.RouteResponseID))
	}

	for _, s := range b.stagesByAPI.Get(apiID) {
		b.clearRouteThrottleBucket(apiID, s.StageName, r.RouteKey)
	}

	b.autoDeployLocked(apiID)

	return nil
}

// validateRouteKeyUpdate checks that newKey is valid for protocolType and is
// not a duplicate among routes (excluding the route being updated). It does
// not mutate r.
func validateRouteKeyUpdate(routes []*Route, routeID, newKey, protocolType string) error {
	if protocolType == protocolTypeHTTP {
		if err := validateHTTPRouteKey(newKey); err != nil {
			return err
		}
	}

	for _, existing := range routes {
		if existing.RouteID != routeID && existing.RouteKey == newKey {
			return fmt.Errorf("%w: route key %q already exists", ErrAlreadyExists, newKey)
		}
	}

	return nil
}

// validateRouteAuthUpdate checks AuthorizationType against the AWS enum. It
// does not mutate r.
func validateRouteAuthUpdate(input UpdateRouteInput) error {
	if input.AuthorizationType != "" && !validRouteAuthorizationType(input.AuthorizationType) {
		return fmt.Errorf(
			"%w: authorizationType must be one of NONE, AWS_IAM, JWT, CUSTOM",
			ErrBadRequest,
		)
	}

	return nil
}

// applyRouteAuthUpdate applies AuthorizationType/AuthorizerID changes from a
// route update. Callers must validate first (validateRouteAuthUpdate).
func applyRouteAuthUpdate(r *Route, input UpdateRouteInput) {
	if input.AuthorizationType != "" {
		r.AuthorizationType = input.AuthorizationType
		if input.AuthorizationType == authorizationTypeNone {
			r.AuthorizerID = ""
		}
	}

	if input.AuthorizerID != "" {
		r.AuthorizerID = input.AuthorizerID
	}
}

// validateManagedRouteKeyUpdate rejects a route-key change on a quick-create
// managed route ("If you created an API using quick create, the $default
// route is managed by API Gateway. You can't modify the $default route key."
// -- service-2.json, Route.ApiGatewayManaged doc), otherwise validates the
// new key via validateRouteKeyUpdate. It does not mutate r.
func validateManagedRouteKeyUpdate(
	r *Route,
	routes []*Route,
	routeID, newKey, protocolType string,
) error {
	if newKey == "" || newKey == r.RouteKey {
		return nil
	}

	if r.APIGatewayManaged {
		return fmt.Errorf(
			"%w: the route key of a quick-create managed route can't be modified",
			ErrBadRequest,
		)
	}

	return validateRouteKeyUpdate(routes, routeID, newKey, protocolType)
}

// applyRouteUpdate mutates r with input's fields. Callers must validate
// first (validateManagedRouteKeyUpdate, validateRouteAuthUpdate).
func applyRouteUpdate(r *Route, input UpdateRouteInput) {
	if input.RouteKey != "" {
		r.RouteKey = input.RouteKey
	}

	if input.Target != "" {
		r.Target = input.Target
	}

	applyRouteAuthUpdate(r, input)

	if input.OperationName != "" {
		r.OperationName = input.OperationName
	}

	if input.ModelSelectionExpression != "" {
		r.ModelSelectionExpression = input.ModelSelectionExpression
	}

	if input.RequestModels != nil {
		r.RequestModels = input.RequestModels
	}

	if input.RequestParameters != nil {
		r.RequestParameters = input.RequestParameters
	}

	if input.AuthorizationScopes != nil {
		r.AuthorizationScopes = input.AuthorizationScopes
	}

	if input.APIKeyRequired != nil {
		r.APIKeyRequired = *input.APIKeyRequired
	}
}

// UpdateRoute updates fields on an existing route. All of input is validated
// before any field of r is mutated, so a rejected update never leaves the
// route in a partially-applied state.
func (b *InMemoryBackend) UpdateRoute(
	apiID, routeID string,
	input UpdateRouteInput,
) (*Route, error) {
	b.mu.Lock("UpdateRoute")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, ErrAPINotFound
	}

	r, ok := b.routes.Get(routeKey(apiID, routeID))
	if !ok {
		return nil, ErrRouteNotFound
	}

	if err := validateManagedRouteKeyUpdate(
		r, b.routesByAPI.Get(apiID), routeID, input.RouteKey, api.ProtocolType,
	); err != nil {
		return nil, err
	}

	if err := validateRouteAuthUpdate(input); err != nil {
		return nil, err
	}

	oldRouteKey := r.RouteKey

	applyRouteUpdate(r, input)

	if r.RouteKey != oldRouteKey {
		for _, s := range b.stagesByAPI.Get(apiID) {
			b.clearRouteThrottleBucket(apiID, s.StageName, oldRouteKey)
		}
	}

	b.autoDeployLocked(apiID)

	cp := *r

	return &cp, nil
}

// DeleteRouteRequestParameter removes a specific request parameter from a route.
func (b *InMemoryBackend) DeleteRouteRequestParameter(
	apiID, routeID, requestParameterKey string,
) error {
	b.mu.Lock("DeleteRouteRequestParameter")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	r, ok := b.routes.Get(routeKey(apiID, routeID))
	if !ok {
		return ErrRouteNotFound
	}

	if r.RequestParameters != nil {
		delete(r.RequestParameters, requestParameterKey)
	}

	return nil
}
