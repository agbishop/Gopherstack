package apigatewayv2

import (
	"encoding/json"
	"time"
)

// isoTime wraps [time.Time] and marshals to/from a JSON string in RFC3339 format,
// which is the __timestampIso8601 format expected by the AWS SDK v2 API Gateway V2 client.
type isoTime struct {
	time.Time
}

func (t isoTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Format(time.RFC3339))
}

func (t *isoTime) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}

	t.Time = parsed

	return nil
}

// CorsConfiguration holds CORS settings for an HTTP API.
type CorsConfiguration struct {
	AllowOrigins     []string `json:"allowOrigins,omitempty"`
	AllowMethods     []string `json:"allowMethods,omitempty"`
	AllowHeaders     []string `json:"allowHeaders,omitempty"`
	ExposeHeaders    []string `json:"exposeHeaders,omitempty"`
	MaxAge           int32    `json:"maxAge,omitempty"`
	AllowCredentials bool     `json:"allowCredentials,omitempty"`
}

// JwtConfiguration holds JWT authorizer configuration.
type JwtConfiguration struct {
	Issuer   string   `json:"issuer,omitempty"`
	Audience []string `json:"audience,omitempty"`
}

// AccessLogSettings holds access log destination settings for a stage.
type AccessLogSettings struct {
	DestinationArn string `json:"destinationArn,omitempty"`
	Format         string `json:"format,omitempty"`
}

// IntegrationTLSConfig holds the TLS configuration for a private integration.
// Supported only for HTTP APIs.
type IntegrationTLSConfig struct {
	ServerNameToVerify string `json:"serverNameToVerify,omitempty"`
}

// MutualTLSAuthentication holds the mutual TLS authentication configuration
// for a custom domain name.
type MutualTLSAuthentication struct {
	TruststoreURI      string   `json:"truststoreUri,omitempty"`
	TruststoreVersion  string   `json:"truststoreVersion,omitempty"`
	TruststoreWarnings []string `json:"truststoreWarnings,omitempty"`
}

// RouteSettings holds per-route throttling and logging settings for a stage.
type RouteSettings struct {
	LoggingLevel           string  `json:"loggingLevel,omitempty"`
	ThrottlingRateLimit    float64 `json:"throttlingRateLimit,omitempty"`
	ThrottlingBurstLimit   int32   `json:"throttlingBurstLimit,omitempty"`
	DataTraceEnabled       bool    `json:"dataTraceEnabled,omitempty"`
	DetailedMetricsEnabled bool    `json:"detailedMetricsEnabled,omitempty"`
}

// RouteRequiredParameter indicates whether a request parameter is required.
type RouteRequiredParameter struct {
	Required bool `json:"required"`
}

// API represents an HTTP API (API Gateway v2).
type API struct {
	CorsConfiguration         *CorsConfiguration `json:"corsConfiguration,omitempty"`
	CreatedDate               isoTime            `json:"createdDate"`
	Tags                      map[string]string  `json:"tags,omitempty"`
	APIID                     string             `json:"apiId"`
	Name                      string             `json:"name"`
	Description               string             `json:"description,omitempty"`
	ProtocolType              string             `json:"protocolType"`
	RouteSelectionExpression  string             `json:"routeSelectionExpression,omitempty"`
	APIEndpoint               string             `json:"apiEndpoint,omitempty"`
	Version                   string             `json:"version,omitempty"`
	APIKeySelectionExpression string             `json:"apiKeySelectionExpression,omitempty"`
	// IPAddressType is the IP address types that can invoke the API: "ipv4"
	// (default) or "dualstack".
	IPAddressType string `json:"ipAddressType,omitempty"`
	// ImportInfo carries validation feedback from an ImportApi/ReimportApi
	// call about OpenAPI properties ignored during import. Always empty for
	// an API created via CreateApi (not applicable) or via a well-formed
	// import (no properties ignored).
	ImportInfo []string `json:"importInfo,omitempty"`
	// Warnings carries warning messages reported when FailOnWarnings is set
	// during an ImportApi/ReimportApi call. Always empty for an API created
	// via CreateApi, or a well-formed import.
	Warnings                  []string `json:"warnings,omitempty"`
	DisableSchemaValidation   bool     `json:"disableSchemaValidation,omitempty"`
	DisableExecuteAPIEndpoint bool     `json:"disableExecuteApiEndpoint,omitempty"`
}

// Stage represents a deployment stage for an HTTP API.
type Stage struct {
	AccessLogSettings    *AccessLogSettings       `json:"accessLogSettings,omitempty"`
	DefaultRouteSettings *RouteSettings           `json:"defaultRouteSettings,omitempty"`
	RouteSettings        map[string]RouteSettings `json:"routeSettings,omitempty"`
	Tags                 map[string]string        `json:"tags,omitempty"`
	CreatedDate          isoTime                  `json:"createdDate"`
	LastUpdatedDate      isoTime                  `json:"lastUpdatedDate"`
	StageVariables       map[string]string        `json:"stageVariables,omitempty"`
	StageName            string                   `json:"stageName"`
	APIID                string                   `json:"-"`
	DeploymentID         string                   `json:"deploymentId,omitempty"`
	Description          string                   `json:"description,omitempty"`
	// ClientCertificateID identifies a client certificate for a Stage. Supported
	// only for WebSocket APIs.
	ClientCertificateID string `json:"clientCertificateId,omitempty"`
	AutoDeploy          bool   `json:"autoDeploy"`
	// APIGatewayManaged is true for the $default stage auto-provisioned by
	// CreateApi's quick-create shortcut (routeKey+target); such a stage cannot
	// be modified via UpdateStage. False for stages a caller created directly.
	APIGatewayManaged bool `json:"apiGatewayManaged"`
}

// Route represents a route in an HTTP API.
type Route struct {
	RequestModels            map[string]string                 `json:"requestModels,omitempty"`
	RequestParameters        map[string]RouteRequiredParameter `json:"requestParameters,omitempty"`
	RouteID                  string                            `json:"routeId"`
	APIID                    string                            `json:"-"`
	RouteKey                 string                            `json:"routeKey"`
	Target                   string                            `json:"target,omitempty"`
	AuthorizationType        string                            `json:"authorizationType,omitempty"`
	AuthorizerID             string                            `json:"authorizerId,omitempty"`
	OperationName            string                            `json:"operationName,omitempty"`
	ModelSelectionExpression string                            `json:"modelSelectionExpression,omitempty"`
	AuthorizationScopes      []string                          `json:"authorizationScopes"`
	APIKeyRequired           bool                              `json:"apiKeyRequired"`
	// APIGatewayManaged is true for the $default route auto-provisioned by
	// CreateApi's quick-create shortcut (routeKey+target); its route key
	// cannot be modified. False for routes a caller created directly.
	APIGatewayManaged bool `json:"apiGatewayManaged"`
}

// Integration represents a backend integration for a route.
type Integration struct {
	TLSConfig                   *IntegrationTLSConfig `json:"tlsConfig,omitempty"`
	RequestParameters           map[string]string     `json:"requestParameters,omitempty"`
	RequestTemplates            map[string]string     `json:"requestTemplates,omitempty"`
	IntegrationID               string                `json:"integrationId"`
	APIID                       string                `json:"-"`
	IntegrationType             string                `json:"integrationType"`
	IntegrationSubtype          string                `json:"integrationSubtype,omitempty"`
	IntegrationMethod           string                `json:"integrationMethod,omitempty"`
	IntegrationURI              string                `json:"integrationUri,omitempty"`
	Description                 string                `json:"description,omitempty"`
	PayloadFormatVersion        string                `json:"payloadFormatVersion,omitempty"`
	ConnectionType              string                `json:"connectionType,omitempty"`
	ConnectionID                string                `json:"connectionId,omitempty"`
	TemplateSelectionExpression string                `json:"templateSelectionExpression,omitempty"`
	PassthroughBehavior         string                `json:"passthroughBehavior,omitempty"`
	// CredentialsArn specifies the credentials required for the integration,
	// if any: an IAM role ARN, or "arn:aws:iam::*:user/*" to pass the
	// caller's identity through. Empty (unset) uses resource-based
	// permissions on supported AWS services.
	CredentialsArn  string `json:"credentialsArn,omitempty"`
	TimeoutInMillis int32  `json:"timeoutInMillis,omitempty"`
	// APIGatewayManaged is true for the integration auto-provisioned by
	// CreateApi's quick-create shortcut (routeKey+target). Unlike managed
	// routes/stages, a managed integration can still be updated -- just not
	// deleted. False for integrations a caller created directly.
	APIGatewayManaged bool `json:"apiGatewayManaged"`
}

// Deployment represents an API deployment.
type Deployment struct {
	CreatedDate      isoTime `json:"createdDate"`
	DeploymentID     string  `json:"deploymentId"`
	APIID            string  `json:"-"`
	Description      string  `json:"description,omitempty"`
	DeploymentStatus string  `json:"deploymentStatus"`
	// Routes and Integrations are the routing snapshot captured at deployment
	// time: the exact set the data plane matches against for a stage pinned
	// to this deployment (gopherstack-cfr1), never the API's live current
	// state. Not part of the real GetDeploymentOutput wire shape -- internal
	// only, hence json:"-".
	Routes       []Route       `json:"-"`
	Integrations []Integration `json:"-"`
	AutoDeployed bool          `json:"autoDeployed"`
}

// Authorizer represents an authorizer for an HTTP API.
type Authorizer struct {
	JwtConfiguration               *JwtConfiguration `json:"jwtConfiguration,omitempty"`
	AuthorizerID                   string            `json:"authorizerId"`
	APIID                          string            `json:"-"`
	Name                           string            `json:"name"`
	AuthorizerType                 string            `json:"authorizerType"`
	AuthorizerURI                  string            `json:"authorizerUri,omitempty"`
	AuthorizerCredentialsArn       string            `json:"authorizerCredentialsArn,omitempty"`
	AuthorizerPayloadFormatVersion string            `json:"authorizerPayloadFormatVersion,omitempty"`
	IdentitySource                 []string          `json:"identitySource,omitempty"`
	AuthorizerResultTTLInSeconds   int32             `json:"authorizerResultTtlInSeconds"`
	EnableSimpleResponses          bool              `json:"enableSimpleResponses"`
}

// CreateAPIInput is the input for CreateAPI.
type CreateAPIInput struct {
	CorsConfiguration         *CorsConfiguration `json:"corsConfiguration,omitempty"`
	Tags                      map[string]string  `json:"tags,omitempty"`
	Name                      string             `json:"name"`
	Description               string             `json:"description,omitempty"`
	ProtocolType              string             `json:"protocolType"`
	RouteSelectionExpression  string             `json:"routeSelectionExpression,omitempty"`
	Version                   string             `json:"version,omitempty"`
	APIKeySelectionExpression string             `json:"apiKeySelectionExpression,omitempty"`
	// RouteKey and Target together drive CreateApi's "quick create" shortcut
	// (HTTP APIs only): when both are set, the backend auto-provisions a
	// $default route, an integration targeting Target, and an auto-deployed
	// $default stage, all marked apiGatewayManaged.
	RouteKey string `json:"routeKey,omitempty"`
	Target   string `json:"target,omitempty"`
	// CredentialsArn is part of quick create: the credentials (IAM role ARN)
	// for the auto-provisioned integration, if any.
	CredentialsArn string `json:"credentialsArn,omitempty"`
	// IPAddressType is the IP address types that can invoke the API: "ipv4"
	// (default) or "dualstack".
	IPAddressType             string `json:"ipAddressType,omitempty"`
	DisableSchemaValidation   bool   `json:"disableSchemaValidation,omitempty"`
	DisableExecuteAPIEndpoint bool   `json:"disableExecuteApiEndpoint,omitempty"`
}

// UpdateAPIInput is the input for UpdateAPI (PATCH).
type UpdateAPIInput struct {
	CorsConfiguration         *CorsConfiguration `json:"corsConfiguration,omitempty"`
	Tags                      map[string]string  `json:"tags,omitempty"`
	DisableSchemaValidation   *bool              `json:"disableSchemaValidation,omitempty"`
	DisableExecuteAPIEndpoint *bool              `json:"disableExecuteApiEndpoint,omitempty"`
	Name                      string             `json:"name,omitempty"`
	Description               string             `json:"description,omitempty"`
	RouteSelectionExpression  string             `json:"routeSelectionExpression,omitempty"`
	Version                   string             `json:"version,omitempty"`
	APIKeySelectionExpression string             `json:"apiKeySelectionExpression,omitempty"`
	// RouteKey and Target are part of quick create: if set, they replace the
	// route key / integration target of the API's existing quick-create
	// route and integration (each is independently optional; either requires
	// the API to already have a quick-create route/integration).
	RouteKey string `json:"routeKey,omitempty"`
	Target   string `json:"target,omitempty"`
	// CredentialsArn is part of quick create: if set, it replaces the
	// credentials associated with the quick-create integration (requires the
	// API to already have a quick-create integration).
	CredentialsArn string `json:"credentialsArn,omitempty"`
	// IPAddressType is the IP address types that can invoke the API: "ipv4"
	// or "dualstack".
	IPAddressType string `json:"ipAddressType,omitempty"`
}

// CreateStageInput is the input for CreateStage.
type CreateStageInput struct {
	AccessLogSettings    *AccessLogSettings       `json:"accessLogSettings,omitempty"`
	DefaultRouteSettings *RouteSettings           `json:"defaultRouteSettings,omitempty"`
	RouteSettings        map[string]RouteSettings `json:"routeSettings,omitempty"`
	StageVariables       map[string]string        `json:"stageVariables,omitempty"`
	Tags                 map[string]string        `json:"tags,omitempty"`
	StageName            string                   `json:"stageName"`
	DeploymentID         string                   `json:"deploymentId,omitempty"`
	Description          string                   `json:"description,omitempty"`
	ClientCertificateID  string                   `json:"clientCertificateId,omitempty"`
	AutoDeploy           bool                     `json:"autoDeploy"`
}

// UpdateStageInput is the input for UpdateStage (PATCH).
type UpdateStageInput struct {
	AccessLogSettings    *AccessLogSettings       `json:"accessLogSettings,omitempty"`
	DefaultRouteSettings *RouteSettings           `json:"defaultRouteSettings,omitempty"`
	RouteSettings        map[string]RouteSettings `json:"routeSettings,omitempty"`
	StageVariables       map[string]string        `json:"stageVariables,omitempty"`
	AutoDeploy           *bool                    `json:"autoDeploy,omitempty"`
	DeploymentID         string                   `json:"deploymentId,omitempty"`
	Description          string                   `json:"description,omitempty"`
	ClientCertificateID  string                   `json:"clientCertificateId,omitempty"`
}

// CreateRouteInput is the input for CreateRoute.
type CreateRouteInput struct {
	RequestModels            map[string]string                 `json:"requestModels,omitempty"`
	RequestParameters        map[string]RouteRequiredParameter `json:"requestParameters,omitempty"`
	RouteKey                 string                            `json:"routeKey"`
	Target                   string                            `json:"target,omitempty"`
	AuthorizationType        string                            `json:"authorizationType,omitempty"`
	AuthorizerID             string                            `json:"authorizerId,omitempty"`
	OperationName            string                            `json:"operationName,omitempty"`
	ModelSelectionExpression string                            `json:"modelSelectionExpression,omitempty"`
	AuthorizationScopes      []string                          `json:"authorizationScopes,omitempty"`
	APIKeyRequired           bool                              `json:"apiKeyRequired,omitempty"`
}

// UpdateRouteInput is the input for UpdateRoute (PATCH).
type UpdateRouteInput struct {
	RequestModels            map[string]string                 `json:"requestModels,omitempty"`
	RequestParameters        map[string]RouteRequiredParameter `json:"requestParameters,omitempty"`
	RouteKey                 string                            `json:"routeKey,omitempty"`
	Target                   string                            `json:"target,omitempty"`
	AuthorizationType        string                            `json:"authorizationType,omitempty"`
	AuthorizerID             string                            `json:"authorizerId,omitempty"`
	OperationName            string                            `json:"operationName,omitempty"`
	ModelSelectionExpression string                            `json:"modelSelectionExpression,omitempty"`
	APIKeyRequired           *bool                             `json:"apiKeyRequired,omitempty"`
	AuthorizationScopes      []string                          `json:"authorizationScopes,omitempty"`
}

// CreateIntegrationInput is the input for CreateIntegration.
type CreateIntegrationInput struct {
	TLSConfig                   *IntegrationTLSConfig `json:"tlsConfig,omitempty"`
	RequestParameters           map[string]string     `json:"requestParameters,omitempty"`
	RequestTemplates            map[string]string     `json:"requestTemplates,omitempty"`
	IntegrationType             string                `json:"integrationType"`
	IntegrationSubtype          string                `json:"integrationSubtype,omitempty"`
	IntegrationMethod           string                `json:"integrationMethod,omitempty"`
	IntegrationURI              string                `json:"integrationUri,omitempty"`
	Description                 string                `json:"description,omitempty"`
	PayloadFormatVersion        string                `json:"payloadFormatVersion,omitempty"`
	ConnectionType              string                `json:"connectionType,omitempty"`
	ConnectionID                string                `json:"connectionId,omitempty"`
	TemplateSelectionExpression string                `json:"templateSelectionExpression,omitempty"`
	PassthroughBehavior         string                `json:"passthroughBehavior,omitempty"`
	CredentialsArn              string                `json:"credentialsArn,omitempty"`
	TimeoutInMillis             int32                 `json:"timeoutInMillis,omitempty"`
}

// UpdateIntegrationInput is the input for UpdateIntegration (PATCH).
type UpdateIntegrationInput struct {
	TLSConfig                   *IntegrationTLSConfig `json:"tlsConfig,omitempty"`
	RequestParameters           map[string]string     `json:"requestParameters,omitempty"`
	RequestTemplates            map[string]string     `json:"requestTemplates,omitempty"`
	IntegrationType             string                `json:"integrationType,omitempty"`
	IntegrationSubtype          string                `json:"integrationSubtype,omitempty"`
	IntegrationMethod           string                `json:"integrationMethod,omitempty"`
	IntegrationURI              string                `json:"integrationUri,omitempty"`
	Description                 string                `json:"description,omitempty"`
	PayloadFormatVersion        string                `json:"payloadFormatVersion,omitempty"`
	ConnectionType              string                `json:"connectionType,omitempty"`
	ConnectionID                string                `json:"connectionId,omitempty"`
	TemplateSelectionExpression string                `json:"templateSelectionExpression,omitempty"`
	PassthroughBehavior         string                `json:"passthroughBehavior,omitempty"`
	CredentialsArn              string                `json:"credentialsArn,omitempty"`
	TimeoutInMillis             int32                 `json:"timeoutInMillis,omitempty"`
}

// CreateDeploymentInput is the input for CreateDeployment.
type CreateDeploymentInput struct {
	Description string `json:"description,omitempty"`
	StageName   string `json:"stageName,omitempty"`
}

// CreateAuthorizerInput is the input for CreateAuthorizer.
type CreateAuthorizerInput struct {
	JwtConfiguration               *JwtConfiguration `json:"jwtConfiguration,omitempty"`
	Name                           string            `json:"name"`
	AuthorizerType                 string            `json:"authorizerType"`
	AuthorizerURI                  string            `json:"authorizerUri,omitempty"`
	AuthorizerCredentialsArn       string            `json:"authorizerCredentialsArn,omitempty"`
	AuthorizerPayloadFormatVersion string            `json:"authorizerPayloadFormatVersion,omitempty"`
	IdentitySource                 []string          `json:"identitySource,omitempty"`
	AuthorizerResultTTLInSeconds   int32             `json:"authorizerResultTtlInSeconds,omitempty"`
	EnableSimpleResponses          bool              `json:"enableSimpleResponses,omitempty"`
}

// UpdateAuthorizerInput is the input for UpdateAuthorizer (PATCH).
type UpdateAuthorizerInput struct {
	JwtConfiguration               *JwtConfiguration `json:"jwtConfiguration,omitempty"`
	AuthorizerResultTTLInSeconds   *int32            `json:"authorizerResultTtlInSeconds,omitempty"`
	EnableSimpleResponses          *bool             `json:"enableSimpleResponses,omitempty"`
	Name                           *string           `json:"name,omitempty"`
	AuthorizerURI                  *string           `json:"authorizerUri,omitempty"`
	AuthorizerCredentialsArn       *string           `json:"authorizerCredentialsArn,omitempty"`
	AuthorizerPayloadFormatVersion *string           `json:"authorizerPayloadFormatVersion,omitempty"`
	AuthorizerType                 string            `json:"authorizerType,omitempty"`
	IdentitySource                 []string          `json:"identitySource,omitempty"`
}

// UpdateAPIMappingInput is the input for UpdateAPIMapping (PATCH).
type UpdateAPIMappingInput struct {
	APIID         string `json:"apiId,omitempty"`
	Stage         string `json:"stage,omitempty"`
	APIMappingKey string `json:"apiMappingKey,omitempty"`
}

// UpdateDeploymentInput is the input for UpdateDeployment (PATCH).
type UpdateDeploymentInput struct {
	Description string `json:"description,omitempty"`
}

// UpdateDomainNameInput is the input for UpdateDomainName (PATCH).
type UpdateDomainNameInput struct {
	MutualTLSAuthentication  *MutualTLSAuthentication  `json:"mutualTlsAuthentication,omitempty"`
	Tags                     map[string]string         `json:"tags,omitempty"`
	RoutingMode              string                    `json:"routingMode,omitempty"`
	DomainNameConfigurations []DomainNameConfiguration `json:"domainNameConfigurations,omitempty"`
}

// UpdateIntegrationResponseInput is the input for UpdateIntegrationResponse (PATCH).
type UpdateIntegrationResponseInput struct {
	ResponseParameters          map[string]string `json:"responseParameters,omitempty"`
	ResponseTemplates           map[string]string `json:"responseTemplates,omitempty"`
	IntegrationResponseKey      string            `json:"integrationResponseKey,omitempty"`
	ContentHandlingStrategy     string            `json:"contentHandlingStrategy,omitempty"`
	TemplateSelectionExpression string            `json:"templateSelectionExpression,omitempty"`
}

// UpdateModelInput is the input for UpdateModel (PATCH).
type UpdateModelInput struct {
	Name        string `json:"name,omitempty"`
	Schema      string `json:"schema,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Description string `json:"description,omitempty"`
}

// UpdateRouteResponseInput is the input for UpdateRouteResponse (PATCH).
type UpdateRouteResponseInput struct {
	ResponseModels           map[string]string `json:"responseModels,omitempty"`
	RouteResponseKey         string            `json:"routeResponseKey,omitempty"`
	ModelSelectionExpression string            `json:"modelSelectionExpression,omitempty"`
}

// UpdatePortalInput is the input for UpdatePortal (PATCH). Status is
// internal-only (set by handlePublishPortal/handleDisablePortal, never by a
// real client -- the real UpdatePortalInput has no such member, confirmed
// against api_op_UpdatePortal.go) and must never be JSON-decoded from a
// request body.
type UpdatePortalInput struct {
	Tags                      map[string]string `json:"tags,omitempty"`
	LogoURI                   string            `json:"logoUri,omitempty"`
	RumAppMonitorName         string            `json:"rumAppMonitorName,omitempty"`
	Status                    string            `json:"-"`
	PublishDescription        string            `json:"-"`
	IncludedPortalProductArns []string          `json:"includedPortalProductArns,omitempty"`
}

// UpdatePortalProductInput is the input for UpdatePortalProduct (PATCH).
type UpdatePortalProductInput struct {
	Tags        map[string]string `json:"tags,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	Description string            `json:"description,omitempty"`
}

// UpdateProductPageInput is the input for UpdateProductPage (PATCH).
type UpdateProductPageInput struct {
	DisplayContent map[string]any `json:"displayContent,omitempty"`
}

// UpdateProductRestEndpointPageInput is the input for UpdateProductRestEndpointPage (PATCH).
type UpdateProductRestEndpointPageInput struct {
	DisplayContent map[string]any `json:"displayContent,omitempty"`
}

// PublishPortalInput is the input for PublishPortal (POST).
type PublishPortalInput struct {
	Description string `json:"description,omitempty"`
}

// listApisOutput is the response body for GetAPIs.
type listApisOutput struct {
	NextToken string `json:"nextToken,omitempty"`
	Items     []API  `json:"items"`
}

// listStagesOutput is the response body for GetStages.
type listStagesOutput struct {
	NextToken string  `json:"nextToken,omitempty"`
	Items     []Stage `json:"items"`
}

// listRoutesOutput is the response body for GetRoutes.
type listRoutesOutput struct {
	NextToken string  `json:"nextToken,omitempty"`
	Items     []Route `json:"items"`
}

// listIntegrationsOutput is the response body for GetIntegrations.
type listIntegrationsOutput struct {
	NextToken string        `json:"nextToken,omitempty"`
	Items     []Integration `json:"items"`
}

// listDeploymentsOutput is the response body for GetDeployments.
type listDeploymentsOutput struct {
	NextToken string       `json:"nextToken,omitempty"`
	Items     []Deployment `json:"items"`
}

// listAuthorizersOutput is the response body for GetAuthorizers.
type listAuthorizersOutput struct {
	NextToken string       `json:"nextToken,omitempty"`
	Items     []Authorizer `json:"items"`
}

// notFoundResponse is returned when a resource is not found.
type notFoundResponse struct {
	Message string `json:"message"`
}

// DomainNameConfiguration represents a domain name configuration entry.
type DomainNameConfiguration struct {
	CertificateArn                      string `json:"certificateArn,omitempty"`
	DomainNameStatus                    string `json:"domainNameStatus,omitempty"`
	EndpointType                        string `json:"endpointType,omitempty"`
	SecurityPolicy                      string `json:"securityPolicy,omitempty"`
	APIGatewayDomainName                string `json:"apiGatewayDomainName,omitempty"`
	HostedZoneID                        string `json:"hostedZoneId,omitempty"`
	CertificateName                     string `json:"certificateName,omitempty"`
	OwnershipVerificationCertificateArn string `json:"ownershipVerificationCertificateArn,omitempty"`
}

// DomainName represents a custom domain name for API Gateway v2.
type DomainName struct {
	MutualTLSAuthentication       *MutualTLSAuthentication `json:"mutualTlsAuthentication,omitempty"`
	Tags                          map[string]string        `json:"tags,omitempty"`
	DomainNameValue               string                   `json:"domainName"`
	DomainNameArn                 string                   `json:"domainNameArn,omitempty"`
	APIMappingSelectionExpression string                   `json:"apiMappingSelectionExpression,omitempty"`
	// RoutingMode is the routing mode: API_MAPPING_ONLY (default),
	// ROUTING_RULE_ONLY, or ROUTING_RULE_THEN_API_MAPPING. The
	// ROUTING_RULE_* modes only take effect together with RoutingRule
	// resources on this domain name, which are out of scope for this
	// emulator (see gopherstack-e81); the field itself is still stored and
	// round-tripped for wire completeness.
	RoutingMode              string                    `json:"routingMode,omitempty"`
	DomainNameConfigurations []DomainNameConfiguration `json:"domainNameConfigurations"`
}

// CreateDomainNameInput is the input for CreateDomainName.
type CreateDomainNameInput struct {
	MutualTLSAuthentication  *MutualTLSAuthentication  `json:"mutualTlsAuthentication,omitempty"`
	Tags                     map[string]string         `json:"tags,omitempty"`
	DomainNameValue          string                    `json:"domainName"`
	RoutingMode              string                    `json:"routingMode,omitempty"`
	DomainNameConfigurations []DomainNameConfiguration `json:"domainNameConfigurations,omitempty"`
}

// APIMapping represents an API mapping for a custom domain name.
type APIMapping struct {
	APIID         string `json:"apiId"`
	APIMappingID  string `json:"apiMappingId"`
	DomainName    string `json:"-"`
	Stage         string `json:"stage"`
	APIMappingKey string `json:"apiMappingKey,omitempty"`
}

// CreateAPIMappingInput is the input for CreateAPIMapping.
type CreateAPIMappingInput struct {
	APIID         string `json:"apiId"`
	Stage         string `json:"stage"`
	APIMappingKey string `json:"apiMappingKey,omitempty"`
}

// IntegrationResponse represents an integration response.
type IntegrationResponse struct {
	ResponseParameters          map[string]string `json:"responseParameters,omitempty"`
	ResponseTemplates           map[string]string `json:"responseTemplates,omitempty"`
	IntegrationResponseID       string            `json:"integrationResponseId"`
	IntegrationResponseKey      string            `json:"integrationResponseKey"`
	APIID                       string            `json:"-"`
	IntegrationID               string            `json:"-"`
	ContentHandlingStrategy     string            `json:"contentHandlingStrategy,omitempty"`
	TemplateSelectionExpression string            `json:"templateSelectionExpression,omitempty"`
}

// CreateIntegrationResponseInput is the input for CreateIntegrationResponse.
type CreateIntegrationResponseInput struct {
	ResponseParameters          map[string]string `json:"responseParameters,omitempty"`
	ResponseTemplates           map[string]string `json:"responseTemplates,omitempty"`
	IntegrationResponseKey      string            `json:"integrationResponseKey"`
	ContentHandlingStrategy     string            `json:"contentHandlingStrategy,omitempty"`
	TemplateSelectionExpression string            `json:"templateSelectionExpression,omitempty"`
}

// Model represents a data model for an API.
type Model struct {
	ModelID     string `json:"modelId"`
	APIID       string `json:"-"`
	Name        string `json:"name"`
	Schema      string `json:"schema,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Description string `json:"description,omitempty"`
}

// CreateModelInput is the input for CreateModel.
type CreateModelInput struct {
	Name        string `json:"name"`
	Schema      string `json:"schema,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Description string `json:"description,omitempty"`
}

// CognitoConfig mirrors types.CognitoConfig (types.go:245): all three
// members are required whenever CognitoConfig is present (verified against
// validateCognitoConfig, validators.go).
type CognitoConfig struct {
	AppClientID    string `json:"appClientId"`
	UserPoolArn    string `json:"userPoolArn"`
	UserPoolDomain string `json:"userPoolDomain"`
}

// None mirrors types.None (types.go:901), an empty marker struct used by
// the Authorization and EndpointConfigurationRequest unions to select the
// "no auth" / "default domain" branch.
type None struct{}

// Authorization mirrors types.Authorization (types.go:142), a union of
// CognitoConfig and None. Neither branch is required by
// validateAuthorization -- only Authorization itself is required on
// CreatePortalInput.
type Authorization struct {
	CognitoConfig *CognitoConfig `json:"cognitoConfig,omitempty"`
	None          *None          `json:"none,omitempty"`
}

// ACMManaged mirrors types.ACMManaged (types.go:24): both members are
// required whenever ACMManaged is present (verified against
// validateACMManaged, validators.go).
type ACMManaged struct {
	CertificateArn string `json:"certificateArn"`
	DomainName     string `json:"domainName"`
}

// EndpointConfigurationRequest mirrors types.EndpointConfigurationRequest
// (types.go:486), a union of ACMManaged and None.
type EndpointConfigurationRequest struct {
	AcmManaged *ACMManaged `json:"acmManaged,omitempty"`
	None       *None       `json:"none,omitempty"`
}

// EndpointConfigurationResponse mirrors types.EndpointConfigurationResponse
// (types.go:498). PortalDefaultDomainName/PortalDomainHostedZoneId are
// required response members with no client-supplied equivalent -- this
// backend synthesizes them the same way it synthesizes ARNs and execute-api
// endpoints elsewhere (see randomID/defaultRegion), not fabricated business
// data.
type EndpointConfigurationResponse struct {
	CertificateArn           string `json:"certificateArn,omitempty"`
	DomainName               string `json:"domainName,omitempty"`
	PortalDefaultDomainName  string `json:"portalDefaultDomainName"`
	PortalDomainHostedZoneID string `json:"portalDomainHostedZoneId"`
}

// CustomColors mirrors types.CustomColors (types.go:295): all six
// members are required whenever CustomColors is present (verified against
// validateCustomColors, validators.go).
type CustomColors struct {
	AccentColor          string `json:"accentColor"`
	BackgroundColor      string `json:"backgroundColor"`
	ErrorValidationColor string `json:"errorValidationColor"`
	HeaderColor          string `json:"headerColor"`
	NavigationColor      string `json:"navigationColor"`
	TextColor            string `json:"textColor"`
}

// PortalTheme mirrors types.PortalTheme (types.go:1034). LogoLastUploaded
// is set by the (unmodeled) logo-upload API, never by CreatePortal/UpdatePortal,
// so it is omitted here rather than accepted from the client and echoed
// dishonestly.
type PortalTheme struct {
	CustomColors *CustomColors `json:"customColors"`
}

// PortalContent mirrors types.PortalContent (types.go:917).
type PortalContent struct {
	Theme       *PortalTheme `json:"theme"`
	DisplayName string       `json:"displayName"`
	Description string       `json:"description,omitempty"`
}

// Portal represents an API Gateway v2 portal. PublishStatus's wire key is
// "publishStatus", not "status" -- confirmed against
// aws-sdk-go-v2/service/apigatewayv2@v1.37.4's GetPortalOutput/PortalSummary
// (types.PublishStatus: PUBLISHED/PUBLISH_IN_PROGRESS/PUBLISH_FAILED/
// DISABLE_IN_PROGRESS/DISABLE_FAILED/DISABLED -- no "ACTIVE" value exists).
type Portal struct {
	Authorization             *Authorization                 `json:"authorization,omitempty"`
	EndpointConfiguration     *EndpointConfigurationResponse `json:"endpointConfiguration,omitempty"`
	PortalContent             *PortalContent                 `json:"portalContent,omitempty"`
	LastModified              *isoTime                       `json:"lastModified,omitempty"`
	LastPublished             *isoTime                       `json:"lastPublished,omitempty"`
	Tags                      map[string]string              `json:"tags,omitempty"`
	LogoURI                   string                         `json:"logoUri,omitempty"`
	PortalID                  string                         `json:"portalId"`
	PortalArn                 string                         `json:"portalArn,omitempty"`
	PublishStatus             string                         `json:"publishStatus,omitempty"`
	RumAppMonitorName         string                         `json:"rumAppMonitorName,omitempty"`
	LastPublishedDescription  string                         `json:"lastPublishedDescription,omitempty"`
	IncludedPortalProductArns []string                       `json:"includedPortalProductArns,omitempty"`
}

// CreatePortalInput is the input for CreatePortal.
type CreatePortalInput struct {
	Authorization             *Authorization                `json:"authorization"`
	EndpointConfiguration     *EndpointConfigurationRequest `json:"endpointConfiguration"`
	PortalContent             *PortalContent                `json:"portalContent"`
	Tags                      map[string]string             `json:"tags,omitempty"`
	LogoURI                   string                        `json:"logoUri,omitempty"`
	RumAppMonitorName         string                        `json:"rumAppMonitorName,omitempty"`
	IncludedPortalProductArns []string                      `json:"includedPortalProductArns,omitempty"`
}

// PortalProduct represents a portal product. LastModified is a real,
// required PortalProductSummary member (aws-sdk-go-v2/service/
// apigatewayv2@v1.37.4's types.go) this backend previously never tracked.
type PortalProduct struct {
	LastModified     *isoTime          `json:"lastModified,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	PortalProductID  string            `json:"portalProductId"`
	PortalProductArn string            `json:"portalProductArn,omitempty"`
	DisplayName      string            `json:"displayName"`
	Description      string            `json:"description,omitempty"`
}

// CreatePortalProductInput is the input for CreatePortalProduct.
type CreatePortalProductInput struct {
	Tags        map[string]string `json:"tags,omitempty"`
	DisplayName string            `json:"displayName"`
	Description string            `json:"description,omitempty"`
}

// ProductPage represents a product page within a portal product.
type ProductPage struct {
	LastModified    *isoTime       `json:"lastModified,omitempty"`
	DisplayContent  map[string]any `json:"displayContent,omitempty"`
	ProductPageID   string         `json:"productPageId"`
	PortalProductID string         `json:"-"`
}

// CreateProductPageInput is the input for CreateProductPage. DisplayContent
// is a real, required CreateProductPageInput member
// (aws-sdk-go-v2/service/apigatewayv2@v1.37.4's api_op_CreateProductPage.go)
// this backend previously dropped entirely -- the handler decoded a request
// body into this struct, which had no field to receive it.
type CreateProductPageInput struct {
	DisplayContent  map[string]any `json:"displayContent,omitempty"`
	PortalProductID string         `json:"-"`
}

// IdentifierParts mirrors types.IdentifierParts (types.go:551): all four
// members are required whenever IdentifierParts is present (verified against
// validateIdentifierParts, validators.go).
type IdentifierParts struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	RestAPIID string `json:"restApiId"`
	Stage     string `json:"stage"`
}

// RestEndpointIdentifier mirrors types.RestEndpointIdentifier
// (types.go:1138). IdentifierParts itself is optional per
// validateRestEndpointIdentifier -- only RestEndpointIdentifier as a whole is
// a required CreateProductRestEndpointPageInput member.
type RestEndpointIdentifier struct {
	IdentifierParts *IdentifierParts `json:"identifierParts,omitempty"`
}

// ProductRestEndpointPage represents a REST endpoint page within a portal product.
type ProductRestEndpointPage struct {
	LastModified              *isoTime                `json:"lastModified,omitempty"`
	RestEndpointIdentifier    *RestEndpointIdentifier `json:"restEndpointIdentifier,omitempty"`
	DisplayContent            map[string]any          `json:"displayContent,omitempty"`
	ProductRestEndpointPageID string                  `json:"productRestEndpointPageId"`
	PortalProductID           string                  `json:"-"`
}

// CreateProductRestEndpointPageInput is the input for
// CreateProductRestEndpointPage. DisplayContent is a real, optional member
// (aws-sdk-go-v2/service/apigatewayv2@v1.37.4's
// api_op_CreateProductRestEndpointPage.go) this backend previously dropped
// entirely, even though the sibling UpdateProductRestEndpointPage already
// accepts and stores it correctly on the same ProductRestEndpointPage.DisplayContent
// field.
type CreateProductRestEndpointPageInput struct {
	RestEndpointIdentifier *RestEndpointIdentifier `json:"restEndpointIdentifier"`
	DisplayContent         map[string]any          `json:"displayContent,omitempty"`
	PortalProductID        string                  `json:"-"`
}

// RouteResponse represents a route response.
type RouteResponse struct {
	ResponseModels           map[string]string `json:"responseModels,omitempty"`
	RouteResponseID          string            `json:"routeResponseId"`
	RouteResponseKey         string            `json:"routeResponseKey"`
	APIID                    string            `json:"-"`
	RouteID                  string            `json:"-"`
	ModelSelectionExpression string            `json:"modelSelectionExpression,omitempty"`
}

// CreateRouteResponseInput is the input for CreateRouteResponse.
type CreateRouteResponseInput struct {
	ResponseModels           map[string]string `json:"responseModels,omitempty"`
	RouteResponseKey         string            `json:"routeResponseKey"`
	ModelSelectionExpression string            `json:"modelSelectionExpression,omitempty"`
}

// listDomainNamesOutput is the response body for GetDomainNames.
type listDomainNamesOutput struct {
	NextToken string       `json:"nextToken,omitempty"`
	Items     []DomainName `json:"items"`
}

// listAPIMappingsOutput is the response body for GetApiMappings.
type listAPIMappingsOutput struct {
	NextToken string       `json:"nextToken,omitempty"`
	Items     []APIMapping `json:"items"`
}

// listIntegrationResponsesOutput is the response body for GetIntegrationResponses.
type listIntegrationResponsesOutput struct {
	NextToken string                `json:"nextToken,omitempty"`
	Items     []IntegrationResponse `json:"items"`
}

// listModelsOutput is the response body for GetModels.
type listModelsOutput struct {
	NextToken string  `json:"nextToken,omitempty"`
	Items     []Model `json:"items"`
}

// listRouteResponsesOutput is the response body for GetRouteResponses.
type listRouteResponsesOutput struct {
	NextToken string          `json:"nextToken,omitempty"`
	Items     []RouteResponse `json:"items"`
}

// listPortalsOutput is the response body for ListPortals.
type listPortalsOutput struct {
	NextToken string   `json:"nextToken,omitempty"`
	Items     []Portal `json:"items"`
}

// listPortalProductsOutput is the response body for ListPortalProducts.
type listPortalProductsOutput struct {
	NextToken string          `json:"nextToken,omitempty"`
	Items     []PortalProduct `json:"items"`
}

// listProductPagesOutput is the response body for ListProductPages.
type listProductPagesOutput struct {
	NextToken string        `json:"nextToken,omitempty"`
	Items     []ProductPage `json:"items"`
}

// listProductREPagesOutput is the response body for ListProductRestEndpointPages.
type listProductREPagesOutput struct {
	NextToken string                    `json:"nextToken,omitempty"`
	Items     []ProductRestEndpointPage `json:"items"`
}

// VpcLink represents a v2 VPC link.
type VpcLink struct {
	CreatedDate      isoTime           `json:"createdDate"`
	Tags             map[string]string `json:"tags,omitempty"`
	VpcLinkID        string            `json:"vpcLinkId"`
	Name             string            `json:"name"`
	VpcLinkStatus    string            `json:"vpcLinkStatus,omitempty"`
	SecurityGroupIDs []string          `json:"securityGroupIds"`
	SubnetIDs        []string          `json:"subnetIds"`
}

// CreateVpcLinkInput is the input for CreateVpcLink.
type CreateVpcLinkInput struct {
	Tags             map[string]string `json:"tags,omitempty"`
	Name             string            `json:"name"`
	SecurityGroupIDs []string          `json:"securityGroupIds,omitempty"`
	SubnetIDs        []string          `json:"subnetIds,omitempty"`
}

// UpdateVpcLinkInput is the input for UpdateVpcLink.
type UpdateVpcLinkInput struct {
	Name string `json:"name,omitempty"`
}

// RoutingRule represents an API Gateway domain routing rule.
type RoutingRule struct {
	DomainName     string                 `json:"-"`
	RoutingRuleID  string                 `json:"routingRuleId"`
	RoutingRuleARN string                 `json:"routingRuleArn,omitempty"`
	Actions        []RoutingRuleAction    `json:"actions,omitempty"`
	Conditions     []RoutingRuleCondition `json:"conditions,omitempty"`
	Priority       int32                  `json:"priority"`
}

// RoutingRuleAction is a routing rule action. InvokeApi is the only action
// AWS supports (types.go:1280-1287, aws-sdk-go-v2/service/apigatewayv2@v1.37.4).
type RoutingRuleAction struct {
	InvokeAPI *RoutingRuleActionInvokeAPI `json:"invokeApi,omitempty"`
}

// RoutingRuleActionInvokeAPI is the InvokeApi routing rule action target.
type RoutingRuleActionInvokeAPI struct {
	APIID         string `json:"apiId"`
	Stage         string `json:"stage"`
	StripBasePath bool   `json:"stripBasePath,omitempty"`
}

// RoutingRuleCondition is a routing rule condition: base-path and/or header
// matching, ANDed together when both are set (types.go:1310-1319).
type RoutingRuleCondition struct {
	MatchBasePaths *RoutingRuleMatchBasePaths `json:"matchBasePaths,omitempty"`
	MatchHeaders   *RoutingRuleMatchHeaders   `json:"matchHeaders,omitempty"`
}

// RoutingRuleMatchBasePaths matches if the request base path is any of AnyOf.
type RoutingRuleMatchBasePaths struct {
	AnyOf []string `json:"anyOf,omitempty"`
}

// RoutingRuleMatchHeaders matches if any header name/value-glob pair in AnyOf matches.
type RoutingRuleMatchHeaders struct {
	AnyOf []RoutingRuleMatchHeaderValue `json:"anyOf,omitempty"`
}

// RoutingRuleMatchHeaderValue is a single header/value-glob pair to match.
type RoutingRuleMatchHeaderValue struct {
	Header    string `json:"header"`
	ValueGlob string `json:"valueGlob"`
}

// CreateRoutingRuleInput is the input for CreateRoutingRule.
type CreateRoutingRuleInput struct {
	Actions    []RoutingRuleAction    `json:"actions,omitempty"`
	Conditions []RoutingRuleCondition `json:"conditions,omitempty"`
	Priority   int32                  `json:"priority"`
}

// PutRoutingRuleInput is the input for PutRoutingRule.
type PutRoutingRuleInput struct {
	Actions    []RoutingRuleAction    `json:"actions,omitempty"`
	Conditions []RoutingRuleCondition `json:"conditions,omitempty"`
	Priority   int32                  `json:"priority"`
}

// PortalProductSharingPolicy is the response body for portal product sharing policy operations.
type PortalProductSharingPolicy struct {
	PolicyDocument string `json:"policyDocument,omitempty"`
}

// listVpcLinksOutput is the response body for GetVpcLinks.
type listVpcLinksOutput struct {
	NextToken string    `json:"nextToken,omitempty"`
	Items     []VpcLink `json:"items"`
}

// listRoutingRulesOutput is the response body for ListRoutingRules. Unlike
// every other List/Get collection op in this service, the real
// ListRoutingRulesOutput wraps its items under "routingRules", not "items"
// (confirmed at aws-sdk-go-v2/service/apigatewayv2@v1.37.4's
// api_op_ListRoutingRules.go:56 and deserializers.go's
// awsRestjson1_deserializeOpDocumentListRoutingRulesOutput case list).
type listRoutingRulesOutput struct {
	NextToken    string        `json:"nextToken,omitempty"`
	RoutingRules []RoutingRule `json:"routingRules"`
}

// getTagsOutput is the response body for GetTags.
type getTagsOutput struct {
	Tags map[string]string `json:"tags"`
}

// tagResourceInput is the input for TagResource.
type tagResourceInput struct {
	Tags map[string]string `json:"tags"`
}
