package apigatewayv2

import (
	"context"
)

// StorageBackend is the interface for the API Gateway v2 in-memory store.
type StorageBackend interface {
	// APIs
	CreateAPI(ctx context.Context, input CreateAPIInput) (*API, error)
	GetAPI(apiID string) (*API, error)
	GetAPIs() ([]API, error)
	DeleteAPI(apiID string) error
	UpdateAPI(apiID string, input UpdateAPIInput) (*API, error)

	// Stages
	CreateStage(apiID string, input CreateStageInput) (*Stage, error)
	GetStage(apiID, stageName string) (*Stage, error)
	GetStages(apiID string) ([]Stage, error)
	DeleteStage(apiID, stageName string) error
	UpdateStage(apiID, stageName string, input UpdateStageInput) (*Stage, error)

	// Routes
	CreateRoute(apiID string, input CreateRouteInput) (*Route, error)
	GetRoute(apiID, routeID string) (*Route, error)
	GetRoutes(apiID string) ([]Route, error)
	DeleteRoute(apiID, routeID string) error
	UpdateRoute(apiID, routeID string, input UpdateRouteInput) (*Route, error)

	// Integrations
	CreateIntegration(apiID string, input CreateIntegrationInput) (*Integration, error)
	GetIntegration(apiID, integrationID string) (*Integration, error)
	GetIntegrations(apiID string) ([]Integration, error)
	DeleteIntegration(apiID, integrationID string) error
	UpdateIntegration(apiID, integrationID string, input UpdateIntegrationInput) (*Integration, error)

	// Deployments
	CreateDeployment(apiID string, input CreateDeploymentInput) (*Deployment, error)
	GetDeployment(apiID, deploymentID string) (*Deployment, error)
	GetDeployments(apiID string) ([]Deployment, error)
	DeleteDeployment(apiID, deploymentID string) error

	// Authorizers
	CreateAuthorizer(apiID string, input CreateAuthorizerInput) (*Authorizer, error)
	GetAuthorizer(apiID, authorizerID string) (*Authorizer, error)
	GetAuthorizers(apiID string) ([]Authorizer, error)
	DeleteAuthorizer(apiID, authorizerID string) error
	UpdateAuthorizer(apiID, authorizerID string, input UpdateAuthorizerInput) (*Authorizer, error)

	// Domain Names
	CreateDomainName(ctx context.Context, input CreateDomainNameInput) (*DomainName, error)

	// API Mappings
	CreateAPIMapping(domainName string, input CreateAPIMappingInput) (*APIMapping, error)

	// Integration Responses
	CreateIntegrationResponse(
		apiID, integrationID string,
		input CreateIntegrationResponseInput,
	) (*IntegrationResponse, error)

	// Models
	CreateModel(apiID string, input CreateModelInput) (*Model, error)

	// Route Responses
	CreateRouteResponse(apiID, routeID string, input CreateRouteResponseInput) (*RouteResponse, error)

	// Portals
	CreatePortal(input CreatePortalInput) (*Portal, error)

	// Portal Products
	CreatePortalProduct(input CreatePortalProductInput) (*PortalProduct, error)

	// Product Pages
	CreateProductPage(portalProductID string, input CreateProductPageInput) (*ProductPage, error)

	// Product REST Endpoint Pages
	CreateProductRestEndpointPage(
		portalProductID string,
		input CreateProductRestEndpointPageInput,
	) (*ProductRestEndpointPage, error)

	// VPC links
	CreateVpcLink(input CreateVpcLinkInput) (*VpcLink, error)
	GetVpcLink(vpcLinkID string) (*VpcLink, error)
	GetVpcLinks() ([]VpcLink, error)
	UpdateVpcLink(vpcLinkID string, input UpdateVpcLinkInput) (*VpcLink, error)
	DeleteVpcLink(vpcLinkID string) error

	// Routing rules
	CreateRoutingRule(
		ctx context.Context,
		domainName string,
		input CreateRoutingRuleInput,
	) (*RoutingRule, error)
	GetRoutingRule(domainName, routingRuleID string) (*RoutingRule, error)
	ListRoutingRules(domainName string) ([]RoutingRule, error)
	PutRoutingRule(domainName, routingRuleID string, input PutRoutingRuleInput) (*RoutingRule, error)
	DeleteRoutingRule(domainName, routingRuleID string) error

	// Portal sharing policy
	GetPortalProductSharingPolicy(portalProductID string) (*PortalProductSharingPolicy, error)
	PutPortalProductSharingPolicy(portalProductID, policyDocument string) (*PortalProductSharingPolicy, error)
	DeletePortalProductSharingPolicy(portalProductID string) error

	// Domain Names - Get/Delete
	GetDomainName(domainName string) (*DomainName, error)
	GetDomainNames() ([]DomainName, error)
	DeleteDomainName(domainName string) error

	// API Mappings - Get/Delete
	GetAPIMapping(domainName, mappingID string) (*APIMapping, error)
	GetAPIMappings(domainName string) ([]APIMapping, error)
	DeleteAPIMapping(domainName, mappingID string) error

	// Integration Responses - Get/Delete
	GetIntegrationResponse(apiID, integrationID, responseID string) (*IntegrationResponse, error)
	GetIntegrationResponses(apiID, integrationID string) ([]IntegrationResponse, error)
	DeleteIntegrationResponse(apiID, integrationID, responseID string) error

	// Models - Get/Delete
	GetModel(apiID, modelID string) (*Model, error)
	GetModels(apiID string) ([]Model, error)
	DeleteModel(apiID, modelID string) error

	// Route Responses - Get/Delete
	GetRouteResponse(apiID, routeID, responseID string) (*RouteResponse, error)
	GetRouteResponses(apiID, routeID string) ([]RouteResponse, error)
	DeleteRouteResponse(apiID, routeID, responseID string) error

	// Portals - Get/List
	GetPortal(portalID string) (*Portal, error)
	ListPortals() ([]Portal, error)

	// Portal Products - Get/List
	GetPortalProduct(portalProductID string) (*PortalProduct, error)
	ListPortalProducts() ([]PortalProduct, error)

	// Product Pages - List
	ListProductPages(portalProductID string) ([]ProductPage, error)

	// Product REST Endpoint Pages - List
	ListProductRestEndpointPages(portalProductID string) ([]ProductRestEndpointPage, error)

	// Tags
	TagResource(resourceARN string, tags map[string]string) error
	UntagResource(resourceARN string, tagKeys []string) error
	GetTags(resourceARN string) (map[string]string, error)

	// ExportAPI generates an OpenAPI specification for the API's routes.
	ExportAPI(apiID string) (map[string]any, error)

	// UpdateAPIMapping
	UpdateAPIMapping(domainName, mappingID string, input UpdateAPIMappingInput) (*APIMapping, error)

	// UpdateDeployment
	UpdateDeployment(apiID, deploymentID string, input UpdateDeploymentInput) (*Deployment, error)

	// UpdateDomainName
	UpdateDomainName(domainName string, input UpdateDomainNameInput) (*DomainName, error)

	// UpdateIntegrationResponse
	UpdateIntegrationResponse(
		apiID, integrationID, responseID string,
		input UpdateIntegrationResponseInput,
	) (*IntegrationResponse, error)

	// UpdateModel
	UpdateModel(apiID, modelID string, input UpdateModelInput) (*Model, error)

	// UpdateRouteResponse
	UpdateRouteResponse(apiID, routeID, responseID string, input UpdateRouteResponseInput) (*RouteResponse, error)

	// UpdatePortal
	UpdatePortal(portalID string, input UpdatePortalInput) (*Portal, error)

	// UpdatePortalProduct
	UpdatePortalProduct(portalProductID string, input UpdatePortalProductInput) (*PortalProduct, error)

	// UpdateProductPage
	UpdateProductPage(portalProductID, pageID string, input UpdateProductPageInput) (*ProductPage, error)

	// UpdateProductRestEndpointPage
	UpdateProductRestEndpointPage(
		portalProductID, pageID string,
		input UpdateProductRestEndpointPageInput,
	) (*ProductRestEndpointPage, error)

	// DeletePortal
	DeletePortal(portalID string) error

	// DeletePortalProduct
	DeletePortalProduct(portalProductID string) error

	// GetProductPage
	GetProductPage(portalProductID, pageID string) (*ProductPage, error)

	// GetProductRestEndpointPage
	GetProductRestEndpointPage(portalProductID, pageID string) (*ProductRestEndpointPage, error)

	// DeleteProductPage
	DeleteProductPage(portalProductID, pageID string) error

	// DeleteProductRestEndpointPage
	DeleteProductRestEndpointPage(portalProductID, pageID string) error

	// ResetAuthorizersCache
	ResetAuthorizersCache(apiID, stageName string) error

	// DeleteCorsConfiguration clears the CORS configuration on an API.
	DeleteCorsConfiguration(apiID string) error

	// DeleteAccessLogSettings clears the access log settings on a stage.
	DeleteAccessLogSettings(apiID, stageName string) error

	// DeleteRouteSettings removes per-route settings for a specific routeKey from a stage.
	DeleteRouteSettings(apiID, stageName, routeKey string) error

	// DeleteRouteRequestParameter removes a specific request parameter from a route.
	DeleteRouteRequestParameter(apiID, routeID, requestParameterKey string) error

	// EnforceRouteThrottle applies a stage's RouteSettings/DefaultRouteSettings
	// throttling for a request to routeKey. It returns ErrThrottled when the
	// limit is exceeded, or nil when unconfigured or within the configured rate.
	EnforceRouteThrottle(apiID, stageName, routeKey string) error
}
