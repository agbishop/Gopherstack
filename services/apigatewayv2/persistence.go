package apigatewayv2

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// apigatewayv2SnapshotVersion identifies the shape of backendSnapshot's Tables
// blob (the set of "clean" tables on b.registry plus the DTO tables built
// below for the "dirty" ones -- see store_setup.go's registerAllTables doc
// for the clean/dirty split). Bump whenever a change to a DTO type, a
// registered table's value type, or backendSnapshot itself would make an
// older snapshot unsafe to decode as the current shape. Restore compares this
// against the persisted value and discards (rather than attempts to
// partially decode) any mismatch -- see Restore below. Mirrors the
// services/apigateway pilot (commit 6da0334e).
const apigatewayv2SnapshotVersion = 1

// stageSnapshot, routeSnapshot, ..., routingRuleSnapshot are DTOs used ONLY
// for Snapshot/Restore. Each mirrors its live type field for field, except
// the identity field (APIID/DomainName/PortalProductID) is given a real JSON
// tag here instead of the live type's `json:"-"` (see models.go) --
// marshaling the live type directly would silently drop that field, and
// unmarshaling into it would leave it permanently empty, corrupting the flat
// table's composite key on restore. This is the same DTO-registry technique
// services/apigateway uses for its Resource/Stage/etc types (commit
// 6da0334e).
type stageSnapshot struct {
	AccessLogSettings    *AccessLogSettings       `json:"accessLogSettings,omitempty"`
	DefaultRouteSettings *RouteSettings           `json:"defaultRouteSettings,omitempty"`
	RouteSettings        map[string]RouteSettings `json:"routeSettings,omitempty"`
	Tags                 map[string]string        `json:"tags,omitempty"`
	CreatedDate          isoTime                  `json:"createdDate"`
	LastUpdatedDate      isoTime                  `json:"lastUpdatedDate"`
	StageVariables       map[string]string        `json:"stageVariables,omitempty"`
	StageName            string                   `json:"stageName"`
	APIID                string                   `json:"apiId"`
	DeploymentID         string                   `json:"deploymentId,omitempty"`
	Description          string                   `json:"description,omitempty"`
	ClientCertificateID  string                   `json:"clientCertificateId,omitempty"`
	AutoDeploy           bool                     `json:"autoDeploy"`
	APIGatewayManaged    bool                     `json:"apiGatewayManaged"`
}

func stageSnapshotKey(v *stageSnapshot) string { return stageKey(v.APIID, v.StageName) }

func toStageSnapshot(v *Stage) *stageSnapshot {
	return &stageSnapshot{
		AccessLogSettings:    v.AccessLogSettings,
		DefaultRouteSettings: v.DefaultRouteSettings,
		RouteSettings:        v.RouteSettings,
		Tags:                 v.Tags,
		CreatedDate:          v.CreatedDate,
		LastUpdatedDate:      v.LastUpdatedDate,
		StageVariables:       v.StageVariables,
		StageName:            v.StageName,
		APIID:                v.APIID,
		DeploymentID:         v.DeploymentID,
		Description:          v.Description,
		ClientCertificateID:  v.ClientCertificateID,
		AutoDeploy:           v.AutoDeploy,
		APIGatewayManaged:    v.APIGatewayManaged,
	}
}

func fromStageSnapshot(v *stageSnapshot) *Stage {
	return &Stage{
		AccessLogSettings:    v.AccessLogSettings,
		DefaultRouteSettings: v.DefaultRouteSettings,
		RouteSettings:        v.RouteSettings,
		Tags:                 v.Tags,
		CreatedDate:          v.CreatedDate,
		LastUpdatedDate:      v.LastUpdatedDate,
		StageVariables:       v.StageVariables,
		StageName:            v.StageName,
		APIID:                v.APIID,
		DeploymentID:         v.DeploymentID,
		Description:          v.Description,
		ClientCertificateID:  v.ClientCertificateID,
		AutoDeploy:           v.AutoDeploy,
		APIGatewayManaged:    v.APIGatewayManaged,
	}
}

type routeSnapshot struct {
	RequestModels            map[string]string                 `json:"requestModels,omitempty"`
	RequestParameters        map[string]RouteRequiredParameter `json:"requestParameters,omitempty"`
	RouteID                  string                            `json:"routeId"`
	APIID                    string                            `json:"apiId"`
	RouteKey                 string                            `json:"routeKey"`
	Target                   string                            `json:"target,omitempty"`
	AuthorizationType        string                            `json:"authorizationType,omitempty"`
	AuthorizerID             string                            `json:"authorizerId,omitempty"`
	OperationName            string                            `json:"operationName,omitempty"`
	ModelSelectionExpression string                            `json:"modelSelectionExpression,omitempty"`
	AuthorizationScopes      []string                          `json:"authorizationScopes"`
	APIKeyRequired           bool                              `json:"apiKeyRequired"`
	APIGatewayManaged        bool                              `json:"apiGatewayManaged"`
}

func routeSnapshotKey(v *routeSnapshot) string { return routeKey(v.APIID, v.RouteID) }

func toRouteSnapshot(v *Route) *routeSnapshot {
	return &routeSnapshot{
		RequestModels:            v.RequestModels,
		RequestParameters:        v.RequestParameters,
		RouteID:                  v.RouteID,
		APIID:                    v.APIID,
		RouteKey:                 v.RouteKey,
		Target:                   v.Target,
		AuthorizationType:        v.AuthorizationType,
		AuthorizerID:             v.AuthorizerID,
		OperationName:            v.OperationName,
		ModelSelectionExpression: v.ModelSelectionExpression,
		AuthorizationScopes:      v.AuthorizationScopes,
		APIKeyRequired:           v.APIKeyRequired,
		APIGatewayManaged:        v.APIGatewayManaged,
	}
}

func fromRouteSnapshot(v *routeSnapshot) *Route {
	return &Route{
		RequestModels:            v.RequestModels,
		RequestParameters:        v.RequestParameters,
		RouteID:                  v.RouteID,
		APIID:                    v.APIID,
		RouteKey:                 v.RouteKey,
		Target:                   v.Target,
		AuthorizationType:        v.AuthorizationType,
		AuthorizerID:             v.AuthorizerID,
		OperationName:            v.OperationName,
		ModelSelectionExpression: v.ModelSelectionExpression,
		AuthorizationScopes:      v.AuthorizationScopes,
		APIKeyRequired:           v.APIKeyRequired,
		APIGatewayManaged:        v.APIGatewayManaged,
	}
}

type integrationSnapshot struct {
	TLSConfig                   *IntegrationTLSConfig `json:"tlsConfig,omitempty"`
	RequestParameters           map[string]string     `json:"requestParameters,omitempty"`
	RequestTemplates            map[string]string     `json:"requestTemplates,omitempty"`
	IntegrationID               string                `json:"integrationId"`
	APIID                       string                `json:"apiId"`
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
	APIGatewayManaged           bool                  `json:"apiGatewayManaged"`
}

func integrationSnapshotKey(v *integrationSnapshot) string {
	return integrationKey(v.APIID, v.IntegrationID)
}

func toIntegrationSnapshot(v *Integration) *integrationSnapshot {
	return &integrationSnapshot{
		TLSConfig:                   v.TLSConfig,
		RequestParameters:           v.RequestParameters,
		RequestTemplates:            v.RequestTemplates,
		IntegrationID:               v.IntegrationID,
		APIID:                       v.APIID,
		IntegrationType:             v.IntegrationType,
		IntegrationSubtype:          v.IntegrationSubtype,
		IntegrationMethod:           v.IntegrationMethod,
		IntegrationURI:              v.IntegrationURI,
		Description:                 v.Description,
		PayloadFormatVersion:        v.PayloadFormatVersion,
		ConnectionType:              v.ConnectionType,
		ConnectionID:                v.ConnectionID,
		TemplateSelectionExpression: v.TemplateSelectionExpression,
		PassthroughBehavior:         v.PassthroughBehavior,
		CredentialsArn:              v.CredentialsArn,
		TimeoutInMillis:             v.TimeoutInMillis,
		APIGatewayManaged:           v.APIGatewayManaged,
	}
}

func fromIntegrationSnapshot(v *integrationSnapshot) *Integration {
	return &Integration{
		TLSConfig:                   v.TLSConfig,
		RequestParameters:           v.RequestParameters,
		RequestTemplates:            v.RequestTemplates,
		IntegrationID:               v.IntegrationID,
		APIID:                       v.APIID,
		IntegrationType:             v.IntegrationType,
		IntegrationSubtype:          v.IntegrationSubtype,
		IntegrationMethod:           v.IntegrationMethod,
		IntegrationURI:              v.IntegrationURI,
		Description:                 v.Description,
		PayloadFormatVersion:        v.PayloadFormatVersion,
		ConnectionType:              v.ConnectionType,
		ConnectionID:                v.ConnectionID,
		TemplateSelectionExpression: v.TemplateSelectionExpression,
		PassthroughBehavior:         v.PassthroughBehavior,
		CredentialsArn:              v.CredentialsArn,
		TimeoutInMillis:             v.TimeoutInMillis,
		APIGatewayManaged:           v.APIGatewayManaged,
	}
}

// deploymentSnapshot's Routes/Integrations persist the routing snapshot
// (gopherstack-cfr1) a stage pinned to this deployment serves, reusing
// routeSnapshot/integrationSnapshot so the nested entries round-trip through
// the same field-for-field mirroring as the top-level route/integration
// tables above.
type deploymentSnapshot struct {
	CreatedDate      isoTime               `json:"createdDate"`
	DeploymentID     string                `json:"deploymentId"`
	APIID            string                `json:"apiId"`
	Description      string                `json:"description,omitempty"`
	DeploymentStatus string                `json:"deploymentStatus"`
	Routes           []routeSnapshot       `json:"routes,omitempty"`
	Integrations     []integrationSnapshot `json:"integrations,omitempty"`
	AutoDeployed     bool                  `json:"autoDeployed"`
}

func deploymentSnapshotKey(v *deploymentSnapshot) string {
	return deploymentKey(v.APIID, v.DeploymentID)
}

func toDeploymentSnapshot(v *Deployment) *deploymentSnapshot {
	routes := make([]routeSnapshot, len(v.Routes))
	for i := range v.Routes {
		routes[i] = *toRouteSnapshot(&v.Routes[i])
	}

	integrations := make([]integrationSnapshot, len(v.Integrations))
	for i := range v.Integrations {
		integrations[i] = *toIntegrationSnapshot(&v.Integrations[i])
	}

	return &deploymentSnapshot{
		CreatedDate:      v.CreatedDate,
		DeploymentID:     v.DeploymentID,
		APIID:            v.APIID,
		Description:      v.Description,
		DeploymentStatus: v.DeploymentStatus,
		AutoDeployed:     v.AutoDeployed,
		Routes:           routes,
		Integrations:     integrations,
	}
}

func fromDeploymentSnapshot(v *deploymentSnapshot) *Deployment {
	routes := make([]Route, len(v.Routes))
	for i := range v.Routes {
		routes[i] = *fromRouteSnapshot(&v.Routes[i])
	}

	integrations := make([]Integration, len(v.Integrations))
	for i := range v.Integrations {
		integrations[i] = *fromIntegrationSnapshot(&v.Integrations[i])
	}

	return &Deployment{
		CreatedDate:      v.CreatedDate,
		DeploymentID:     v.DeploymentID,
		APIID:            v.APIID,
		Description:      v.Description,
		DeploymentStatus: v.DeploymentStatus,
		AutoDeployed:     v.AutoDeployed,
		Routes:           routes,
		Integrations:     integrations,
	}
}

type authorizerSnapshot struct {
	JwtConfiguration               *JwtConfiguration `json:"jwtConfiguration,omitempty"`
	AuthorizerID                   string            `json:"authorizerId"`
	APIID                          string            `json:"apiId"`
	Name                           string            `json:"name"`
	AuthorizerType                 string            `json:"authorizerType"`
	AuthorizerURI                  string            `json:"authorizerUri,omitempty"`
	AuthorizerCredentialsArn       string            `json:"authorizerCredentialsArn,omitempty"`
	AuthorizerPayloadFormatVersion string            `json:"authorizerPayloadFormatVersion,omitempty"`
	IdentitySource                 []string          `json:"identitySource,omitempty"`
	AuthorizerResultTTLInSeconds   int32             `json:"authorizerResultTtlInSeconds,omitempty"`
	EnableSimpleResponses          bool              `json:"enableSimpleResponses,omitempty"`
}

func authorizerSnapshotKey(v *authorizerSnapshot) string {
	return authorizerKey(v.APIID, v.AuthorizerID)
}

func toAuthorizerSnapshot(v *Authorizer) *authorizerSnapshot {
	return &authorizerSnapshot{
		JwtConfiguration:               v.JwtConfiguration,
		AuthorizerID:                   v.AuthorizerID,
		APIID:                          v.APIID,
		Name:                           v.Name,
		AuthorizerType:                 v.AuthorizerType,
		AuthorizerURI:                  v.AuthorizerURI,
		AuthorizerCredentialsArn:       v.AuthorizerCredentialsArn,
		AuthorizerPayloadFormatVersion: v.AuthorizerPayloadFormatVersion,
		IdentitySource:                 v.IdentitySource,
		AuthorizerResultTTLInSeconds:   v.AuthorizerResultTTLInSeconds,
		EnableSimpleResponses:          v.EnableSimpleResponses,
	}
}

func fromAuthorizerSnapshot(v *authorizerSnapshot) *Authorizer {
	return &Authorizer{
		JwtConfiguration:               v.JwtConfiguration,
		AuthorizerID:                   v.AuthorizerID,
		APIID:                          v.APIID,
		Name:                           v.Name,
		AuthorizerType:                 v.AuthorizerType,
		AuthorizerURI:                  v.AuthorizerURI,
		AuthorizerCredentialsArn:       v.AuthorizerCredentialsArn,
		AuthorizerPayloadFormatVersion: v.AuthorizerPayloadFormatVersion,
		IdentitySource:                 v.IdentitySource,
		AuthorizerResultTTLInSeconds:   v.AuthorizerResultTTLInSeconds,
		EnableSimpleResponses:          v.EnableSimpleResponses,
	}
}

type modelSnapshot struct {
	ModelID     string `json:"modelId"`
	APIID       string `json:"apiId"`
	Name        string `json:"name"`
	Schema      string `json:"schema,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Description string `json:"description,omitempty"`
}

func modelSnapshotKey(v *modelSnapshot) string { return modelKey(v.APIID, v.ModelID) }

func toModelSnapshot(v *Model) *modelSnapshot {
	return &modelSnapshot{
		ModelID:     v.ModelID,
		APIID:       v.APIID,
		Name:        v.Name,
		Schema:      v.Schema,
		ContentType: v.ContentType,
		Description: v.Description,
	}
}

func fromModelSnapshot(v *modelSnapshot) *Model {
	return &Model{
		ModelID:     v.ModelID,
		APIID:       v.APIID,
		Name:        v.Name,
		Schema:      v.Schema,
		ContentType: v.ContentType,
		Description: v.Description,
	}
}

type integrationResponseSnapshot struct {
	ResponseParameters          map[string]string `json:"responseParameters,omitempty"`
	ResponseTemplates           map[string]string `json:"responseTemplates,omitempty"`
	IntegrationResponseID       string            `json:"integrationResponseId"`
	IntegrationResponseKey      string            `json:"integrationResponseKey"`
	APIID                       string            `json:"apiId"`
	IntegrationID               string            `json:"integrationId"`
	ContentHandlingStrategy     string            `json:"contentHandlingStrategy,omitempty"`
	TemplateSelectionExpression string            `json:"templateSelectionExpression,omitempty"`
}

func integrationResponseSnapshotKey(v *integrationResponseSnapshot) string {
	return integrationResponseKey(v.APIID, v.IntegrationID, v.IntegrationResponseID)
}

func toIntegrationResponseSnapshot(v *IntegrationResponse) *integrationResponseSnapshot {
	return &integrationResponseSnapshot{
		ResponseParameters:          v.ResponseParameters,
		ResponseTemplates:           v.ResponseTemplates,
		IntegrationResponseID:       v.IntegrationResponseID,
		IntegrationResponseKey:      v.IntegrationResponseKey,
		APIID:                       v.APIID,
		IntegrationID:               v.IntegrationID,
		ContentHandlingStrategy:     v.ContentHandlingStrategy,
		TemplateSelectionExpression: v.TemplateSelectionExpression,
	}
}

func fromIntegrationResponseSnapshot(v *integrationResponseSnapshot) *IntegrationResponse {
	return &IntegrationResponse{
		ResponseParameters:          v.ResponseParameters,
		ResponseTemplates:           v.ResponseTemplates,
		IntegrationResponseID:       v.IntegrationResponseID,
		IntegrationResponseKey:      v.IntegrationResponseKey,
		APIID:                       v.APIID,
		IntegrationID:               v.IntegrationID,
		ContentHandlingStrategy:     v.ContentHandlingStrategy,
		TemplateSelectionExpression: v.TemplateSelectionExpression,
	}
}

type routeResponseSnapshot struct {
	ResponseModels           map[string]string `json:"responseModels,omitempty"`
	RouteResponseID          string            `json:"routeResponseId"`
	RouteResponseKey         string            `json:"routeResponseKey"`
	APIID                    string            `json:"apiId"`
	RouteID                  string            `json:"routeId"`
	ModelSelectionExpression string            `json:"modelSelectionExpression,omitempty"`
}

func routeResponseSnapshotKey(v *routeResponseSnapshot) string {
	return routeResponseKey(v.APIID, v.RouteID, v.RouteResponseID)
}

func toRouteResponseSnapshot(v *RouteResponse) *routeResponseSnapshot {
	return &routeResponseSnapshot{
		ResponseModels:           v.ResponseModels,
		RouteResponseID:          v.RouteResponseID,
		RouteResponseKey:         v.RouteResponseKey,
		APIID:                    v.APIID,
		RouteID:                  v.RouteID,
		ModelSelectionExpression: v.ModelSelectionExpression,
	}
}

func fromRouteResponseSnapshot(v *routeResponseSnapshot) *RouteResponse {
	return &RouteResponse{
		ResponseModels:           v.ResponseModels,
		RouteResponseID:          v.RouteResponseID,
		RouteResponseKey:         v.RouteResponseKey,
		APIID:                    v.APIID,
		RouteID:                  v.RouteID,
		ModelSelectionExpression: v.ModelSelectionExpression,
	}
}

type apiMappingSnapshot struct {
	APIID         string `json:"apiId"`
	APIMappingID  string `json:"apiMappingId"`
	DomainName    string `json:"domainName"`
	Stage         string `json:"stage"`
	APIMappingKey string `json:"apiMappingKey,omitempty"`
}

func apiMappingSnapshotKey(v *apiMappingSnapshot) string {
	return apiMappingKey(v.DomainName, v.APIMappingID)
}

func toAPIMappingSnapshot(v *APIMapping) *apiMappingSnapshot {
	return &apiMappingSnapshot{
		APIID:         v.APIID,
		APIMappingID:  v.APIMappingID,
		DomainName:    v.DomainName,
		Stage:         v.Stage,
		APIMappingKey: v.APIMappingKey,
	}
}

func fromAPIMappingSnapshot(v *apiMappingSnapshot) *APIMapping {
	return &APIMapping{
		APIID:         v.APIID,
		APIMappingID:  v.APIMappingID,
		DomainName:    v.DomainName,
		Stage:         v.Stage,
		APIMappingKey: v.APIMappingKey,
	}
}

type productPageSnapshot struct {
	LastModified    *isoTime       `json:"lastModified,omitempty"`
	DisplayContent  map[string]any `json:"displayContent,omitempty"`
	ProductPageID   string         `json:"productPageId"`
	PortalProductID string         `json:"portalProductId"`
}

func productPageSnapshotKey(v *productPageSnapshot) string {
	return productPageKey(v.PortalProductID, v.ProductPageID)
}

func toProductPageSnapshot(v *ProductPage) *productPageSnapshot {
	return &productPageSnapshot{
		LastModified:    v.LastModified,
		DisplayContent:  v.DisplayContent,
		ProductPageID:   v.ProductPageID,
		PortalProductID: v.PortalProductID,
	}
}

func fromProductPageSnapshot(v *productPageSnapshot) *ProductPage {
	return &ProductPage{
		LastModified:    v.LastModified,
		DisplayContent:  v.DisplayContent,
		ProductPageID:   v.ProductPageID,
		PortalProductID: v.PortalProductID,
	}
}

type productREPageSnapshot struct {
	LastModified              *isoTime                `json:"lastModified,omitempty"`
	RestEndpointIdentifier    *RestEndpointIdentifier `json:"restEndpointIdentifier,omitempty"`
	DisplayContent            map[string]any          `json:"displayContent,omitempty"`
	ProductRestEndpointPageID string                  `json:"productRestEndpointPageId"`
	PortalProductID           string                  `json:"portalProductId"`
}

func productREPageSnapshotKey(v *productREPageSnapshot) string {
	return productREPageKey(v.PortalProductID, v.ProductRestEndpointPageID)
}

func toProductREPageSnapshot(v *ProductRestEndpointPage) *productREPageSnapshot {
	return &productREPageSnapshot{
		LastModified:              v.LastModified,
		DisplayContent:            v.DisplayContent,
		RestEndpointIdentifier:    v.RestEndpointIdentifier,
		ProductRestEndpointPageID: v.ProductRestEndpointPageID,
		PortalProductID:           v.PortalProductID,
	}
}

func fromProductREPageSnapshot(v *productREPageSnapshot) *ProductRestEndpointPage {
	return &ProductRestEndpointPage{
		LastModified:              v.LastModified,
		DisplayContent:            v.DisplayContent,
		RestEndpointIdentifier:    v.RestEndpointIdentifier,
		ProductRestEndpointPageID: v.ProductRestEndpointPageID,
		PortalProductID:           v.PortalProductID,
	}
}

type routingRuleSnapshot struct {
	DomainName     string                 `json:"domainName"`
	RoutingRuleID  string                 `json:"routingRuleId"`
	RoutingRuleARN string                 `json:"routingRuleArn,omitempty"`
	Actions        []RoutingRuleAction    `json:"actions,omitempty"`
	Conditions     []RoutingRuleCondition `json:"conditions,omitempty"`
	Priority       int32                  `json:"priority"`
}

func routingRuleSnapshotKey(v *routingRuleSnapshot) string {
	return routingRuleKey(v.DomainName, v.RoutingRuleID)
}

func toRoutingRuleSnapshot(v *RoutingRule) *routingRuleSnapshot {
	return &routingRuleSnapshot{
		DomainName:     v.DomainName,
		RoutingRuleID:  v.RoutingRuleID,
		RoutingRuleARN: v.RoutingRuleARN,
		Actions:        v.Actions,
		Conditions:     v.Conditions,
		Priority:       v.Priority,
	}
}

func fromRoutingRuleSnapshot(v *routingRuleSnapshot) *RoutingRule {
	return &RoutingRule{
		DomainName:     v.DomainName,
		RoutingRuleID:  v.RoutingRuleID,
		RoutingRuleARN: v.RoutingRuleARN,
		Actions:        v.Actions,
		Conditions:     v.Conditions,
		Priority:       v.Priority,
	}
}

// dirtyTableNames lists the "dirty" table names shared by Snapshot and
// Restore (see store_setup.go's registerAllTables doc). Both build an
// ephemeral DTO [store.Registry] under these exact names, so a snapshot
// written by one version of Snapshot always lines up with the same version
// of Restore.
//
//nolint:gochecknoglobals // fixed lookup table, mirrors errCodeLookup-style tables elsewhere
var dirtyTableNames = struct {
	stages, routes, integrations, deployments, authorizers, models,
	integrationResponses, routeResponses, apiMappings,
	productPages, productREPages, routingRules string
}{
	stages:               "stages",
	routes:               "routes",
	integrations:         "integrations",
	deployments:          "deployments",
	authorizers:          "authorizers",
	models:               "models",
	integrationResponses: "integrationResponses",
	routeResponses:       "routeResponses",
	apiMappings:          "apiMappings",
	productPages:         "productPages",
	productREPages:       "productREPages",
	routingRules:         "routingRules",
}

// backendSnapshot is the top-level on-disk shape for the API Gateway v2
// backend.
//
// Tables holds one JSON-encoded array per table -- both the "clean" tables
// registered on b.registry (produced by [store.Registry.SnapshotAll]) and the
// "dirty" DTO tables built inline in Snapshot (see store_setup.go's
// registerAllTables doc for the clean/dirty split), merged into one map so a
// single Tables blob round-trips the whole backend.
type backendSnapshot struct {
	Tables                       map[string]json.RawMessage `json:"tables"`
	PortalProductSharingPolicies map[string]string          `json:"portalProductSharingPolicies,omitempty"`
	Version                      int                        `json:"version"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables are plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad
		// input data. Log and skip the snapshot rather than panic, matching
		// the persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx, "apigatewayv2: snapshot table marshal failed", "error", err)

		return nil
	}

	dtoReg := store.NewRegistry()
	stageDTOs := store.Register(dtoReg, dirtyTableNames.stages, store.New(stageSnapshotKey))
	routeDTOs := store.Register(dtoReg, dirtyTableNames.routes, store.New(routeSnapshotKey))
	integrationDTOs := store.Register(dtoReg, dirtyTableNames.integrations, store.New(integrationSnapshotKey))
	deploymentDTOs := store.Register(dtoReg, dirtyTableNames.deployments, store.New(deploymentSnapshotKey))
	authorizerDTOs := store.Register(dtoReg, dirtyTableNames.authorizers, store.New(authorizerSnapshotKey))
	modelDTOs := store.Register(dtoReg, dirtyTableNames.models, store.New(modelSnapshotKey))
	integrationResponseDTOs := store.Register(
		dtoReg, dirtyTableNames.integrationResponses, store.New(integrationResponseSnapshotKey),
	)
	routeResponseDTOs := store.Register(dtoReg, dirtyTableNames.routeResponses, store.New(routeResponseSnapshotKey))
	apiMappingDTOs := store.Register(dtoReg, dirtyTableNames.apiMappings, store.New(apiMappingSnapshotKey))
	productPageDTOs := store.Register(dtoReg, dirtyTableNames.productPages, store.New(productPageSnapshotKey))
	productREPageDTOs := store.Register(dtoReg, dirtyTableNames.productREPages, store.New(productREPageSnapshotKey))
	routingRuleDTOs := store.Register(dtoReg, dirtyTableNames.routingRules, store.New(routingRuleSnapshotKey))

	for _, v := range b.stages.Snapshot() {
		stageDTOs.Put(toStageSnapshot(v))
	}
	for _, v := range b.routes.Snapshot() {
		routeDTOs.Put(toRouteSnapshot(v))
	}
	for _, v := range b.integrations.Snapshot() {
		integrationDTOs.Put(toIntegrationSnapshot(v))
	}
	for _, v := range b.deployments.Snapshot() {
		deploymentDTOs.Put(toDeploymentSnapshot(v))
	}
	for _, v := range b.authorizers.Snapshot() {
		authorizerDTOs.Put(toAuthorizerSnapshot(v))
	}
	for _, v := range b.models.Snapshot() {
		modelDTOs.Put(toModelSnapshot(v))
	}
	for _, v := range b.integrationResponses.Snapshot() {
		integrationResponseDTOs.Put(toIntegrationResponseSnapshot(v))
	}
	for _, v := range b.routeResponses.Snapshot() {
		routeResponseDTOs.Put(toRouteResponseSnapshot(v))
	}
	for _, v := range b.apiMappings.Snapshot() {
		apiMappingDTOs.Put(toAPIMappingSnapshot(v))
	}
	for _, v := range b.productPages.Snapshot() {
		productPageDTOs.Put(toProductPageSnapshot(v))
	}
	for _, v := range b.productREPages.Snapshot() {
		productREPageDTOs.Put(toProductREPageSnapshot(v))
	}
	for _, v := range b.routingRules.Snapshot() {
		routingRuleDTOs.Put(toRoutingRuleSnapshot(v))
	}

	dirtyTables, err := dtoReg.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "apigatewayv2: snapshot DTO table marshal failed", "error", err)

		return nil
	}

	maps.Copy(tables, dirtyTables)

	snap := backendSnapshot{
		Version:                      apigatewayv2SnapshotVersion,
		Tables:                       tables,
		PortalProductSharingPolicies: b.portalProductSharingPolicies,
	}

	return persistence.MarshalSnapshot(ctx, "apigatewayv2", snap)
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "apigatewayv2", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != apigatewayv2SnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"apigatewayv2: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", apigatewayv2SnapshotVersion)

		b.registry.ResetAll()
		b.stages.Reset()
		b.routes.Reset()
		b.integrations.Reset()
		b.deployments.Reset()
		b.authorizers.Reset()
		b.models.Reset()
		b.integrationResponses.Reset()
		b.routeResponses.Reset()
		b.apiMappings.Reset()
		b.productPages.Reset()
		b.productREPages.Reset()
		b.routingRules.Reset()
		b.portalProductSharingPolicies = make(map[string]string)

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("apigatewayv2: restore snapshot tables: %w", err)
	}

	dtoReg := store.NewRegistry()
	stageDTOs := store.Register(dtoReg, dirtyTableNames.stages, store.New(stageSnapshotKey))
	routeDTOs := store.Register(dtoReg, dirtyTableNames.routes, store.New(routeSnapshotKey))
	integrationDTOs := store.Register(dtoReg, dirtyTableNames.integrations, store.New(integrationSnapshotKey))
	deploymentDTOs := store.Register(dtoReg, dirtyTableNames.deployments, store.New(deploymentSnapshotKey))
	authorizerDTOs := store.Register(dtoReg, dirtyTableNames.authorizers, store.New(authorizerSnapshotKey))
	modelDTOs := store.Register(dtoReg, dirtyTableNames.models, store.New(modelSnapshotKey))
	integrationResponseDTOs := store.Register(
		dtoReg, dirtyTableNames.integrationResponses, store.New(integrationResponseSnapshotKey),
	)
	routeResponseDTOs := store.Register(dtoReg, dirtyTableNames.routeResponses, store.New(routeResponseSnapshotKey))
	apiMappingDTOs := store.Register(dtoReg, dirtyTableNames.apiMappings, store.New(apiMappingSnapshotKey))
	productPageDTOs := store.Register(dtoReg, dirtyTableNames.productPages, store.New(productPageSnapshotKey))
	productREPageDTOs := store.Register(dtoReg, dirtyTableNames.productREPages, store.New(productREPageSnapshotKey))
	routingRuleDTOs := store.Register(dtoReg, dirtyTableNames.routingRules, store.New(routingRuleSnapshotKey))

	if err := dtoReg.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("apigatewayv2: restore snapshot DTO tables: %w", err)
	}

	restoreDirtyTables(b, stageDTOs, routeDTOs, integrationDTOs, deploymentDTOs, authorizerDTOs, modelDTOs,
		integrationResponseDTOs, routeResponseDTOs, apiMappingDTOs, productPageDTOs, productREPageDTOs, routingRuleDTOs)

	if snap.PortalProductSharingPolicies != nil {
		b.portalProductSharingPolicies = snap.PortalProductSharingPolicies
	} else {
		b.portalProductSharingPolicies = make(map[string]string)
	}

	return nil
}

// restoreDirtyTables converts each DTO table's contents back to its live
// value type and loads the corresponding live b.<table> via Table.Restore,
// split out from Restore to keep its growth in check.
func restoreDirtyTables(
	b *InMemoryBackend,
	stageDTOs *store.Table[stageSnapshot],
	routeDTOs *store.Table[routeSnapshot],
	integrationDTOs *store.Table[integrationSnapshot],
	deploymentDTOs *store.Table[deploymentSnapshot],
	authorizerDTOs *store.Table[authorizerSnapshot],
	modelDTOs *store.Table[modelSnapshot],
	integrationResponseDTOs *store.Table[integrationResponseSnapshot],
	routeResponseDTOs *store.Table[routeResponseSnapshot],
	apiMappingDTOs *store.Table[apiMappingSnapshot],
	productPageDTOs *store.Table[productPageSnapshot],
	productREPageDTOs *store.Table[productREPageSnapshot],
	routingRuleDTOs *store.Table[routingRuleSnapshot],
) {
	stages := make([]*Stage, 0, stageDTOs.Len())
	for _, v := range stageDTOs.All() {
		stages = append(stages, fromStageSnapshot(v))
	}
	b.stages.Restore(stages)

	routes := make([]*Route, 0, routeDTOs.Len())
	for _, v := range routeDTOs.All() {
		routes = append(routes, fromRouteSnapshot(v))
	}
	b.routes.Restore(routes)

	integrations := make([]*Integration, 0, integrationDTOs.Len())
	for _, v := range integrationDTOs.All() {
		integrations = append(integrations, fromIntegrationSnapshot(v))
	}
	b.integrations.Restore(integrations)

	deployments := make([]*Deployment, 0, deploymentDTOs.Len())
	for _, v := range deploymentDTOs.All() {
		deployments = append(deployments, fromDeploymentSnapshot(v))
	}
	b.deployments.Restore(deployments)

	authorizers := make([]*Authorizer, 0, authorizerDTOs.Len())
	for _, v := range authorizerDTOs.All() {
		authorizers = append(authorizers, fromAuthorizerSnapshot(v))
	}
	b.authorizers.Restore(authorizers)

	models := make([]*Model, 0, modelDTOs.Len())
	for _, v := range modelDTOs.All() {
		models = append(models, fromModelSnapshot(v))
	}
	b.models.Restore(models)

	integrationResponses := make([]*IntegrationResponse, 0, integrationResponseDTOs.Len())
	for _, v := range integrationResponseDTOs.All() {
		integrationResponses = append(integrationResponses, fromIntegrationResponseSnapshot(v))
	}
	b.integrationResponses.Restore(integrationResponses)

	routeResponses := make([]*RouteResponse, 0, routeResponseDTOs.Len())
	for _, v := range routeResponseDTOs.All() {
		routeResponses = append(routeResponses, fromRouteResponseSnapshot(v))
	}
	b.routeResponses.Restore(routeResponses)

	apiMappings := make([]*APIMapping, 0, apiMappingDTOs.Len())
	for _, v := range apiMappingDTOs.All() {
		apiMappings = append(apiMappings, fromAPIMappingSnapshot(v))
	}
	b.apiMappings.Restore(apiMappings)

	productPages := make([]*ProductPage, 0, productPageDTOs.Len())
	for _, v := range productPageDTOs.All() {
		productPages = append(productPages, fromProductPageSnapshot(v))
	}
	b.productPages.Restore(productPages)

	productREPages := make([]*ProductRestEndpointPage, 0, productREPageDTOs.Len())
	for _, v := range productREPageDTOs.All() {
		productREPages = append(productREPages, fromProductREPageSnapshot(v))
	}
	b.productREPages.Restore(productREPages)

	routingRules := make([]*RoutingRule, 0, routingRuleDTOs.Len())
	for _, v := range routingRuleDTOs.All() {
		routingRules = append(routingRules, fromRoutingRuleSnapshot(v))
	}
	b.routingRules.Restore(routingRules)
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}
	if s, ok := h.Backend.(snapshotter); ok {
		return s.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}
	if r, ok := h.Backend.(restorer); ok {
		return r.Restore(ctx, data)
	}

	return nil
}
