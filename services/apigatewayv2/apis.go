package apigatewayv2

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// CreateAPI creates a new HTTP API.
func (b *InMemoryBackend) CreateAPI(ctx context.Context, input CreateAPIInput) (*API, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrBadRequest)
	}

	b.mu.Lock("CreateAPI")
	defer b.mu.Unlock()

	validProtocols := map[string]bool{protocolTypeHTTP: true, protocolTypeWebSocket: true}
	if !validProtocols[input.ProtocolType] {
		return nil, fmt.Errorf("%w: protocolType must be HTTP or WEBSOCKET", ErrBadRequest)
	}

	if err := validateQuickCreateInput(input); err != nil {
		return nil, err
	}

	// Apply AWS-realistic default IPAddressType ("ipv4") when not provided.
	ipAddressType := input.IPAddressType
	if ipAddressType == "" {
		ipAddressType = ipAddressTypeIPv4
	} else if err := validateIPAddressType(ipAddressType); err != nil {
		return nil, err
	}

	// Apply AWS-realistic default RouteSelectionExpression when not provided.
	rse := input.RouteSelectionExpression
	if rse == "" {
		if input.ProtocolType == protocolTypeWebSocket {
			rse = "$request.body.action"
		} else {
			rse = "${request.method} ${request.path}"
		}
	}

	// Apply AWS-realistic default APIKeySelectionExpression when not provided.
	keySelExpr := input.APIKeySelectionExpression
	if keySelExpr == "" {
		if input.ProtocolType == protocolTypeWebSocket {
			keySelExpr = "$context.authorizer.usageIdentifierKey"
		} else {
			keySelExpr = "$request.header.x-api-key"
		}
	}

	id := randomID()
	api := API{
		APIID:                    id,
		Name:                     input.Name,
		Description:              input.Description,
		ProtocolType:             input.ProtocolType,
		RouteSelectionExpression: rse,
		Version:                  input.Version,
		Tags:                     copyTags(input.Tags),
		APIEndpoint: "https://" + id + ".execute-api." + regionFromCtx(
			ctx,
		) + ".amazonaws.com",
		CreatedDate:               isoTime{time.Now()},
		APIKeySelectionExpression: keySelExpr,
		IPAddressType:             ipAddressType,
		DisableSchemaValidation:   input.DisableSchemaValidation,
		DisableExecuteAPIEndpoint: input.DisableExecuteAPIEndpoint,
	}

	if input.CorsConfiguration != nil {
		clone := *input.CorsConfiguration
		api.CorsConfiguration = &clone
	}

	b.apis.Put(&api)

	if input.RouteKey != "" && input.Target != "" {
		if err := b.quickCreateLocked(id, input.RouteKey, input.Target, input.CredentialsArn); err != nil {
			return nil, err
		}
	}

	cp := api

	return &cp, nil
}

// validateQuickCreateInput enforces CreateApi's routeKey/target ("quick
// create") rules: both fields are supported only for HTTP APIs, AWS requires
// both or neither, and a provided routeKey must be a valid HTTP route key.
func validateQuickCreateInput(input CreateAPIInput) error {
	if input.RouteKey == "" && input.Target == "" {
		return nil
	}

	if input.ProtocolType != protocolTypeHTTP {
		return fmt.Errorf("%w: routeKey and target are supported only for HTTP APIs", ErrBadRequest)
	}

	if input.RouteKey == "" || input.Target == "" {
		return fmt.Errorf("%w: routeKey and target must both be specified", ErrBadRequest)
	}

	return validateHTTPRouteKey(input.RouteKey)
}

// quickCreateLocked implements CreateApi's routeKey+target shortcut: it
// auto-provisions the integration, $default route, and auto-deployed
// $default stage that real AWS creates for a "quick create" HTTP API,
// mirroring the resources a caller would otherwise create by hand via
// CreateIntegration/CreateRoute/CreateStage. All three are marked
// apiGatewayManaged, matching AWS (the $default route key and $default stage
// become immutable; the integration stays updatable but not deletable while
// the API exists). credentialsArn is CreateApiInput's quick-create-only
// CredentialsArn, passed through to the auto-provisioned integration
// unchanged (AWS notes it is "currently not used for HTTP integrations" but
// is still stored). Callers must already hold b.mu and have inserted apiID
// into b.apis.
func (b *InMemoryBackend) quickCreateLocked(apiID, routeKey, target, credentialsArn string) error {
	integrationType := integrationTypeHTTPProxy
	if isLambdaFunctionARN(target) {
		integrationType = IntegrationTypeAWSProxy
	}

	integration, err := buildIntegration(apiID, protocolTypeHTTP, CreateIntegrationInput{
		IntegrationType:   integrationType,
		IntegrationURI:    target,
		IntegrationMethod: httpMethodAny,
		CredentialsArn:    credentialsArn,
	})
	if err != nil {
		return err
	}

	integration.APIGatewayManaged = true
	b.integrations.Put(integration)

	route := &Route{
		RouteID:             randomID(),
		APIID:               apiID,
		RouteKey:            routeKey,
		Target:              "integrations/" + integration.IntegrationID,
		AuthorizationType:   authorizationTypeNone,
		AuthorizationScopes: []string{},
		APIGatewayManaged:   true,
	}
	b.routes.Put(route)

	now := isoTime{time.Now()}
	b.stages.Put(&Stage{
		StageName:         routeKeyDefault,
		APIID:             apiID,
		AutoDeploy:        true,
		CreatedDate:       now,
		LastUpdatedDate:   now,
		APIGatewayManaged: true,
	})

	b.autoDeployLocked(apiID)

	return nil
}

// validateIPAddressType returns ErrBadRequest if ipAddressType is not one of
// the modeled enum values (ipv4, dualstack).
func validateIPAddressType(ipAddressType string) error {
	switch ipAddressType {
	case ipAddressTypeIPv4, ipAddressTypeDualstack:
		return nil
	default:
		return fmt.Errorf("%w: ipAddressType must be one of ipv4, dualstack", ErrBadRequest)
	}
}

// isLambdaFunctionARN reports whether target looks like a Lambda function
// ARN (arn:{partition}:lambda:...:function:...). CreateApi's quick-create
// shortcut uses this to pick AWS_PROXY vs HTTP_PROXY for the auto-created
// integration: per AWS, a Lambda ARN target yields AWS_PROXY, any other
// (URL) target yields HTTP_PROXY.
func isLambdaFunctionARN(target string) bool {
	return strings.HasPrefix(target, "arn:") &&
		strings.Contains(target, ":lambda:") &&
		strings.Contains(target, ":function:")
}

// GetAPI retrieves an API by ID.
func (b *InMemoryBackend) GetAPI(apiID string) (*API, error) {
	b.mu.RLock("GetAPI")
	defer b.mu.RUnlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, ErrAPINotFound
	}

	cp := *api

	return &cp, nil
}

// GetAPIs retrieves all APIs.
func (b *InMemoryBackend) GetAPIs() ([]API, error) {
	b.mu.RLock("GetAPIs")
	defer b.mu.RUnlock()

	all := b.apis.All()
	result := make([]API, 0, len(all))

	for _, api := range all {
		result = append(result, *api)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].APIID < result[j].APIID
	})

	return result, nil
}

// deleteAPIChildrenLocked removes every child resource (stages, routes,
// integrations, deployments, authorizers, models, and their nested
// responses) belonging to apiID. It mirrors the implicit cascade that
// deleting the pre-Phase-3.3 nested apiData map provided. Callers must
// already hold b.mu.Lock.
func (b *InMemoryBackend) deleteAPIChildrenLocked(apiID string) {
	for _, s := range slices.Clone(b.stagesByAPI.Get(apiID)) {
		b.stages.Delete(stageKey(apiID, s.StageName))
		b.clearStageThrottleBuckets(apiID, s.StageName)
	}

	for _, r := range slices.Clone(b.routesByAPI.Get(apiID)) {
		for _, rr := range slices.Clone(b.routeResponsesByRoute.Get(routeKey(apiID, r.RouteID))) {
			b.routeResponses.Delete(routeResponseKey(apiID, r.RouteID, rr.RouteResponseID))
		}

		b.routes.Delete(routeKey(apiID, r.RouteID))
	}

	for _, i := range slices.Clone(b.integrationsByAPI.Get(apiID)) {
		for _, ir := range slices.Clone(b.integrationResponsesByIntegration.Get(integrationKey(apiID, i.IntegrationID))) {
			b.integrationResponses.Delete(
				integrationResponseKey(apiID, i.IntegrationID, ir.IntegrationResponseID),
			)
		}

		b.integrations.Delete(integrationKey(apiID, i.IntegrationID))
	}

	for _, dep := range slices.Clone(b.deploymentsByAPI.Get(apiID)) {
		b.deployments.Delete(deploymentKey(apiID, dep.DeploymentID))
	}

	for _, a := range slices.Clone(b.authorizersByAPI.Get(apiID)) {
		b.authorizers.Delete(authorizerKey(apiID, a.AuthorizerID))
	}

	for _, m := range slices.Clone(b.modelsByAPI.Get(apiID)) {
		b.models.Delete(modelKey(apiID, m.ModelID))
	}
}

// DeleteAPI removes an API by ID.
func (b *InMemoryBackend) DeleteAPI(apiID string) error {
	b.mu.Lock("DeleteAPI")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	b.deleteAPIChildrenLocked(apiID)
	b.apis.Delete(apiID)

	// Clean up stale API mappings pointing to this API.
	for _, m := range b.apiMappings.All() {
		if m.APIID == apiID {
			b.apiMappings.Delete(apiMappingKey(m.DomainName, m.APIMappingID))
		}
	}

	return nil
}

// UpdateAPI updates fields on an existing API. All of input is validated
// before any field is mutated, so a rejected update never leaves the API (or
// its quick-create route/integration) in a partially-applied state.
func (b *InMemoryBackend) UpdateAPI(apiID string, input UpdateAPIInput) (*API, error) {
	b.mu.Lock("UpdateAPI")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, ErrAPINotFound
	}

	if input.IPAddressType != "" {
		if err := validateIPAddressType(input.IPAddressType); err != nil {
			return nil, err
		}
	}

	route, integration, err := b.validateQuickCreateUpdateLocked(apiID, input)
	if err != nil {
		return nil, err
	}

	if input.Name != "" {
		api.Name = input.Name
	}

	if input.Description != "" {
		api.Description = input.Description
	}

	if input.RouteSelectionExpression != "" {
		api.RouteSelectionExpression = input.RouteSelectionExpression
	}

	if input.Version != "" {
		api.Version = input.Version
	}

	if input.Tags != nil {
		api.Tags = copyTags(input.Tags)
	}

	if input.APIKeySelectionExpression != "" {
		api.APIKeySelectionExpression = input.APIKeySelectionExpression
	}

	if input.CorsConfiguration != nil {
		clone := *input.CorsConfiguration
		api.CorsConfiguration = &clone
	}

	if input.DisableSchemaValidation != nil {
		api.DisableSchemaValidation = *input.DisableSchemaValidation
	}

	if input.DisableExecuteAPIEndpoint != nil {
		api.DisableExecuteAPIEndpoint = *input.DisableExecuteAPIEndpoint
	}

	if input.IPAddressType != "" {
		api.IPAddressType = input.IPAddressType
	}

	applyQuickCreateUpdateMutateLocked(route, integration, input)

	cp := *api

	return &cp, nil
}

// validateQuickCreateUpdateLocked validates UpdateApiInput's routeKey/target/
// credentialsArn fields, which are "part of quick create", against the API's
// existing quick-create route/integration (found via APIGatewayManaged),
// matching AWS ("you can update a quick-created target, but you can't remove
// it from an API"). It does not mutate the route/integration -- callers
// apply the change via applyQuickCreateUpdateMutateLocked only after every
// field on the whole UpdateAPIInput has validated successfully. Callers must
// already hold b.mu.
func (b *InMemoryBackend) validateQuickCreateUpdateLocked(
	apiID string,
	input UpdateAPIInput,
) (*Route, *Integration, error) {
	var route *Route

	if input.RouteKey != "" {
		route = findManagedRoute(b.routesByAPI.Get(apiID))
		if route == nil {
			return nil, nil, fmt.Errorf(
				"%w: API has no quick-create route to update",
				ErrBadRequest,
			)
		}

		if err := validateHTTPRouteKey(input.RouteKey); err != nil {
			return nil, nil, err
		}

		for _, existing := range b.routesByAPI.Get(apiID) {
			if existing.RouteID != route.RouteID && existing.RouteKey == input.RouteKey {
				return nil, nil, fmt.Errorf(
					"%w: route key %q already exists",
					ErrAlreadyExists,
					input.RouteKey,
				)
			}
		}
	}

	var integration *Integration

	if input.Target != "" {
		integration = findManagedIntegration(b.integrationsByAPI.Get(apiID))
		if integration == nil {
			return nil, nil, fmt.Errorf(
				"%w: API has no quick-create target to update",
				ErrBadRequest,
			)
		}
	}

	if input.CredentialsArn != "" && integration == nil {
		integration = findManagedIntegration(b.integrationsByAPI.Get(apiID))
		if integration == nil {
			return nil, nil, fmt.Errorf(
				"%w: API has no quick-create integration to update",
				ErrBadRequest,
			)
		}
	}

	return route, integration, nil
}

// applyQuickCreateUpdateMutateLocked applies the routeKey/target/
// credentialsArn changes validateQuickCreateUpdateLocked already validated.
// route/integration are nil when the corresponding input field was unset.
// Callers must already hold b.mu.
func applyQuickCreateUpdateMutateLocked(
	route *Route,
	integration *Integration,
	input UpdateAPIInput,
) {
	if input.RouteKey != "" {
		route.RouteKey = input.RouteKey
	}

	if input.Target != "" {
		integration.IntegrationURI = input.Target
		integration.IntegrationType = integrationTypeHTTPProxy

		if isLambdaFunctionARN(input.Target) {
			integration.IntegrationType = IntegrationTypeAWSProxy
		}
	}

	if input.CredentialsArn != "" {
		integration.CredentialsArn = input.CredentialsArn
	}
}

// findManagedRoute returns the quick-create-provisioned route among routes,
// or nil if none is managed (the API wasn't quick-created).
func findManagedRoute(routes []*Route) *Route {
	for _, r := range routes {
		if r.APIGatewayManaged {
			return r
		}
	}

	return nil
}

// findManagedIntegration returns the quick-create-provisioned integration
// among integrations, or nil if none is managed (the API wasn't
// quick-created).
func findManagedIntegration(integrations []*Integration) *Integration {
	for _, i := range integrations {
		if i.APIGatewayManaged {
			return i
		}
	}

	return nil
}

// DeleteCorsConfiguration clears the CORS configuration on an API.
func (b *InMemoryBackend) DeleteCorsConfiguration(apiID string) error {
	b.mu.Lock("DeleteCorsConfiguration")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return ErrAPINotFound
	}

	api.CorsConfiguration = nil

	return nil
}

// ExportAPI generates a basic OpenAPI 3.0 specification for the API's routes.
func (b *InMemoryBackend) ExportAPI(apiID string) (map[string]any, error) {
	b.mu.RLock("ExportAPI")
	defer b.mu.RUnlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, ErrAPINotFound
	}

	const routeKeyParts = 2

	paths := map[string]any{}

	for _, route := range b.routesByAPI.Get(apiID) {
		// Parse route key: e.g. "GET /items" or "$connect" (WebSocket)
		parts := strings.SplitN(route.RouteKey, " ", routeKeyParts)

		var method, routePath string

		if len(parts) == routeKeyParts {
			method = strings.ToLower(parts[0])
			routePath = parts[1]
		} else {
			// WebSocket route like $connect, $disconnect, $default
			method = "get"
			routePath = "/" + strings.TrimPrefix(route.RouteKey, "$")
		}

		if _, exists := paths[routePath]; !exists {
			paths[routePath] = map[string]any{}
		}

		pathItem, _ := paths[routePath].(map[string]any)

		op := map[string]any{
			"operationId": route.RouteID,
			"responses":   map[string]any{"200": map[string]any{"description": "Success"}},
		}

		if route.OperationName != "" {
			op["summary"] = route.OperationName
		}

		if secName := b.exportRouteSecurityName(apiID, route); secName != "" {
			scopes := route.AuthorizationScopes
			if scopes == nil {
				scopes = []string{}
			}

			op["security"] = []any{map[string]any{secName: scopes}}
		}

		pathItem[method] = op
	}

	info := map[string]any{
		"title":   api.Name,
		"version": api.Version,
	}

	if api.Description != "" {
		info["description"] = api.Description
	}

	spec := map[string]any{
		"openapi": "3.0.1",
		"info":    info,
		"paths":   paths,
	}

	if schemes := b.exportSecuritySchemes(apiID); len(schemes) > 0 {
		spec["components"] = map[string]any{"securitySchemes": schemes}
	}

	return spec, nil
}

// exportRouteSecurityName returns the OpenAPI security-scheme name that a route
// references, or "" when the route requires no authorization. JWT/CUSTOM routes
// reference their authorizer by name; AWS_IAM references the "sigv4" scheme.
func (b *InMemoryBackend) exportRouteSecurityName(apiID string, route *Route) string {
	switch route.AuthorizationType {
	case authorizationTypeAWSIAM:
		return "sigv4"
	case authorizerTypeJWT, authorizationTypeCustom:
		if a, ok := b.authorizers.Get(authorizerKey(apiID, route.AuthorizerID)); ok {
			return a.Name
		}

		return route.AuthorizationType
	default:
		return ""
	}
}

// exportSecuritySchemes builds the components.securitySchemes block from the
// API's authorizers and any AWS_IAM routes, mirroring the AWS OpenAPI export
// (JWT authorizers carry the x-amazon-apigateway-authorizer extension).
// openAPIKeyType is the OpenAPI/JSON "type" discriminator key used in exported
// security schemes.
const openAPIKeyType = "type"

func (b *InMemoryBackend) exportSecuritySchemes(apiID string) map[string]any {
	schemes := map[string]any{}

	for _, a := range b.authorizersByAPI.Get(apiID) {
		if a.AuthorizerType != authorizerTypeJWT {
			continue
		}

		schemes[a.Name] = jwtSecurityScheme(a)
	}

	// Emit a sigv4 scheme when any route uses AWS_IAM authorization.
	for _, route := range b.routesByAPI.Get(apiID) {
		if route.AuthorizationType == authorizationTypeAWSIAM {
			schemes["sigv4"] = map[string]any{
				openAPIKeyType:                 "apiKey",
				"name":                         "Authorization",
				"in":                           "header",
				"x-amazon-apigateway-authtype": "awsSigv4",
			}

			break
		}
	}

	return schemes
}

// jwtSecurityScheme builds the OpenAPI securityScheme entry for a JWT authorizer,
// including the AWS x-amazon-apigateway-authorizer extension.
func jwtSecurityScheme(a *Authorizer) map[string]any {
	ext := map[string]any{
		openAPIKeyType:   "jwt",
		"identitySource": strings.Join(a.IdentitySource, ","),
	}

	if a.JwtConfiguration != nil {
		jwtCfg := map[string]any{}
		if a.JwtConfiguration.Issuer != "" {
			jwtCfg["issuer"] = a.JwtConfiguration.Issuer
		}

		if len(a.JwtConfiguration.Audience) > 0 {
			jwtCfg["audience"] = a.JwtConfiguration.Audience
		}

		ext["jwtConfiguration"] = jwtCfg
	}

	return map[string]any{
		openAPIKeyType:                   "oauth2",
		"flows":                          map[string]any{},
		"x-amazon-apigateway-authorizer": ext,
	}
}
