package apigatewayv2

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"maps"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// defaultRegion is used for ARNs and execute-api endpoints when the request
// context carries no region (e.g. an unsigned request).
const defaultRegion = "us-east-1"

// logKeyAPIID is the structured-log field name/map key used for an API ID
// across handler error logging and WebSocket event payloads. Centralized as a
// constant (rather than repeating the "apiId" literal) per goconst.
const logKeyAPIID = "apiId"

// regionFromCtx returns the request-scoped region from the ctxbag, falling back
// to the service default so endpoints/ARNs are always well-formed.
func regionFromCtx(ctx context.Context) string {
	if r := awsmeta.Region(ctx); r != "" {
		return r
	}

	return defaultRegion
}

const (
	apiIDChars  = "abcdefghijklmnopqrstuvwxyz0123456789"
	apiIDLength = 10

	authorizerTypeJWT     = "JWT"
	authorizerTypeRequest = "REQUEST"
	authorizationTypeNone = "NONE"
	// authorizationTypeCustom is the route AuthorizationType for a Lambda
	// (REQUEST-type) authorizer.
	authorizationTypeCustom = "CUSTOM"
	// authorizationTypeAWSIAM is the route AuthorizationType requiring SigV4.
	authorizationTypeAWSIAM = "AWS_IAM"
	protocolTypeHTTP        = "HTTP"
	integrationTypeHTTP     = "HTTP"
	integrationTypeMock     = "MOCK"
	// httpMethodAny is the HTTP route-key method wildcard ("ANY /path")
	// matching every HTTP method, also used as the default IntegrationMethod
	// for the integration CreateApi's quick-create shortcut auto-provisions.
	httpMethodAny = "ANY"

	integrationTimeoutMin = int32(50)
	// integrationTimeoutMaxWebSocket is the maximum (and default) integration
	// timeout for WebSocket APIs: 29 seconds.
	integrationTimeoutMaxWebSocket = int32(29000)
	// integrationTimeoutMaxHTTP is the maximum (and default) integration timeout
	// for HTTP APIs: 30 seconds. AWS allows a longer ceiling for HTTP APIs than
	// for WebSocket APIs; see CreateIntegration/UpdateIntegration docs.
	integrationTimeoutMaxHTTP = int32(30000)

	// connectionTypeInternet is the default Integration ConnectionType when the
	// caller does not specify one.
	connectionTypeInternet = "INTERNET"
	connectionTypeVpcLink  = "VPC_LINK"

	// ipAddressTypeIPv4 is the default API/DomainName IPAddressType when the
	// caller does not specify one.
	ipAddressTypeIPv4      = "ipv4"
	ipAddressTypeDualstack = "dualstack"

	// routingModeAPIMappingOnly is the default DomainName RoutingMode when
	// the caller does not specify one.
	routingModeAPIMappingOnly            = "API_MAPPING_ONLY"
	routingModeRoutingRuleOnly           = "ROUTING_RULE_ONLY"
	routingModeRoutingRuleThenAPIMapping = "ROUTING_RULE_THEN_API_MAPPING"
)

const (
	// IntegrationTypeAWSProxy is the AWS_PROXY integration type.
	IntegrationTypeAWSProxy = "AWS_PROXY"
	// integrationTypeHTTPProxy ("HTTP_PROXY") is declared in http_proxy.go.
)

// InMemoryBackend implements StorageBackend using pkgs/store tables. Every
// resource family nested under an API/domain name/portal product (formerly a
// per-parent nested map -- see apiData in the pre-Phase-3.3 history) is now a
// single flat table keyed by a composite "<parentID>#<childID>" string, with
// a secondary [store.Index] grouping by parent ID. See store_setup.go's doc
// comment for the full clean/dirty table split.
type InMemoryBackend struct {
	apis                              *store.Table[API]
	stages                            *store.Table[Stage]
	stagesByAPI                       *store.Index[Stage]
	routes                            *store.Table[Route]
	routesByAPI                       *store.Index[Route]
	integrations                      *store.Table[Integration]
	integrationsByAPI                 *store.Index[Integration]
	deployments                       *store.Table[Deployment]
	deploymentsByAPI                  *store.Index[Deployment]
	authorizers                       *store.Table[Authorizer]
	authorizersByAPI                  *store.Index[Authorizer]
	models                            *store.Table[Model]
	modelsByAPI                       *store.Index[Model]
	integrationResponses              *store.Table[IntegrationResponse]
	integrationResponsesByIntegration *store.Index[IntegrationResponse]
	routeResponses                    *store.Table[RouteResponse]
	routeResponsesByRoute             *store.Index[RouteResponse]
	domainNames                       *store.Table[DomainName]
	apiMappings                       *store.Table[APIMapping]
	apiMappingsByDomain               *store.Index[APIMapping]
	portals                           *store.Table[Portal]
	portalProducts                    *store.Table[PortalProduct]
	// portalProductSharingPolicies (portalProductID -> policy document) is a
	// plain map, not a store.Table -- see store_setup.go's doc comment for why.
	portalProductSharingPolicies  map[string]string
	productPages                  *store.Table[ProductPage]
	productPagesByPortalProduct   *store.Index[ProductPage]
	productREPages                *store.Table[ProductRestEndpointPage]
	productREPagesByPortalProduct *store.Index[ProductRestEndpointPage]
	vpcLinks                      *store.Table[VpcLink]
	routingRules                  *store.Table[RoutingRule]
	routingRulesByDomain          *store.Index[RoutingRule]
	registry                      *store.Registry
	// routeThrottleBuckets holds the ephemeral per-(api,stage,route) token-bucket
	// state for RouteSettings/DefaultRouteSettings throttling. Not part of any
	// persisted snapshot -- like apigateway v1's usageTracker, it's data-plane
	// rate-limiter state, not resource configuration.
	routeThrottleBuckets map[string]*tokenBucket
	mu                   *lockmetrics.RWMutex
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		portalProductSharingPolicies: make(map[string]string),
		routeThrottleBuckets:         make(map[string]*tokenBucket),
		registry:                     store.NewRegistry(),
		mu:                           lockmetrics.New("apigatewayv2"),
	}
	registerAllTables(b)

	return b
}

// copyTags returns a deep copy of a tags map, guarding against nil.
func copyTags(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}

	return maps.Clone(src)
}

// Reset clears all backend state, reinitialising all tables.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	// The "dirty" tables (see store_setup.go's registerAllTables doc) are
	// deliberately NOT on b.registry, so each needs its own Reset() call here.
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
	b.routeThrottleBuckets = make(map[string]*tokenBucket)
}

// randomID generates a cryptographically random 10-character alphanumeric ID.
func randomID() string {
	b := make([]byte, apiIDLength)
	charCount := uint64(len(apiIDChars))

	for i := range b {
		var v [8]byte
		// crypto/rand.Read always fills the buffer and never returns a non-nil error.
		_, _ = rand.Read(v[:])
		b[i] = apiIDChars[binary.BigEndian.Uint64(v[:])%charCount]
	}

	return string(b)
}
