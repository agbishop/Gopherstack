package apigateway

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// StorageBackend is the interface for the API Gateway in-memory store.
type StorageBackend interface {
	// REST APIs
	CreateRestAPI(input CreateRestAPIInput) (*RestAPI, error)
	DeleteRestAPI(restAPIID string) error
	GetRestAPI(restAPIID string) (*RestAPI, error)
	GetRestAPIs(limit int, position string) ([]RestAPI, string, error)
	UpdateRestAPI(restAPIID string, input UpdateRestAPIInput) (*RestAPI, error)

	// Resources
	GetResources(restAPIID, position string, limit int) ([]Resource, string, error)
	// ResourcesForRouting returns every resource for an API together with a version
	// counter that changes whenever the API's resource set is mutated. The data-plane
	// proxy uses it to build and cache a routing trie without re-copying the full
	// resource set (and paging past AWS's default page size) on every request.
	ResourcesForRouting(restAPIID string) ([]Resource, uint64, error)
	GetResource(restAPIID, resourceID string) (*Resource, error)
	CreateResource(restAPIID, parentID, pathPart string) (*Resource, error)
	DeleteResource(restAPIID, resourceID string) error
	UpdateResource(restAPIID, resourceID string, input UpdateResourceInput) (*Resource, error)

	// Methods
	PutMethod(input PutMethodInput) (*Method, error)
	GetMethod(restAPIID, resourceID, httpMethod string) (*Method, error)
	DeleteMethod(restAPIID, resourceID, httpMethod string) error

	// Method Responses
	PutMethodResponse(
		restAPIID, resourceID, httpMethod, statusCode string,
		input PutMethodResponseInput,
	) (*MethodResponse, error)
	GetMethodResponse(restAPIID, resourceID, httpMethod, statusCode string) (*MethodResponse, error)
	DeleteMethodResponse(restAPIID, resourceID, httpMethod, statusCode string) error

	// Integrations
	PutIntegration(
		restAPIID, resourceID, httpMethod string,
		input PutIntegrationInput,
	) (*Integration, error)
	GetIntegration(restAPIID, resourceID, httpMethod string) (*Integration, error)
	DeleteIntegration(restAPIID, resourceID, httpMethod string) error

	// Integration Responses
	PutIntegrationResponse(
		restAPIID, resourceID, httpMethod, statusCode string,
		input PutIntegrationResponseInput,
	) (*IntegrationResponse, error)
	GetIntegrationResponse(
		restAPIID, resourceID, httpMethod, statusCode string,
	) (*IntegrationResponse, error)
	DeleteIntegrationResponse(restAPIID, resourceID, httpMethod, statusCode string) error

	// Deployments
	CreateDeployment(restAPIID, stageName, description string) (*Deployment, error)
	GetDeployment(restAPIID, deploymentID string) (*Deployment, error)
	GetDeployments(restAPIID string) ([]Deployment, error)
	DeleteDeployment(restAPIID, deploymentID string) error
	UpdateDeployment(
		restAPIID, deploymentID string,
		input UpdateDeploymentInput,
	) (*Deployment, error)

	// Stages
	GetStages(restAPIID string) ([]Stage, error)
	GetStage(restAPIID, stageName string) (*Stage, error)
	DeleteStage(restAPIID, stageName string) error

	// Authorizers
	CreateAuthorizer(restAPIID string, input CreateAuthorizerInput) (*Authorizer, error)
	GetAuthorizer(restAPIID, authorizerID string) (*Authorizer, error)
	GetAuthorizers(restAPIID string) ([]Authorizer, error)
	UpdateAuthorizer(
		restAPIID, authorizerID string,
		input UpdateAuthorizerInput,
	) (*Authorizer, error)
	DeleteAuthorizer(restAPIID, authorizerID string) error

	// Request Validators
	CreateRequestValidator(
		restAPIID string,
		input CreateRequestValidatorInput,
	) (*RequestValidator, error)
	GetRequestValidator(restAPIID, validatorID string) (*RequestValidator, error)
	GetRequestValidators(restAPIID string) ([]RequestValidator, error)
	UpdateRequestValidator(
		restAPIID, validatorID string,
		input UpdateRequestValidatorInput,
	) (*RequestValidator, error)
	DeleteRequestValidator(restAPIID, validatorID string) error

	// API Keys
	CreateAPIKey(input CreateAPIKeyInput) (*APIKey, error)
	GetAPIKey(id string) (*APIKey, error)
	GetAPIKeyByValue(value string) (*APIKey, error)
	GetAPIKeys() ([]APIKey, error)
	GetAPIKeysPage(limit int, position string) ([]APIKey, string, error)
	DeleteAPIKey(id string) error
	UpdateAPIKey(id string, input UpdateAPIKeyInput) (*APIKey, error)

	// Base Path Mappings
	CreateBasePathMapping(input CreateBasePathMappingInput) (*BasePathMapping, error)
	GetBasePathMapping(domainName, basePath string) (*BasePathMapping, error)
	GetBasePathMappings(domainName string) ([]BasePathMapping, error)
	DeleteBasePathMapping(domainName, basePath string) error

	// Documentation Parts (per-API)
	CreateDocumentationPart(input CreateDocumentationPartInput) (*DocumentationPart, error)
	GetDocumentationPart(restAPIID, docPartID string) (*DocumentationPart, error)
	GetDocumentationParts(restAPIID string) ([]DocumentationPart, error)
	DeleteDocumentationPart(restAPIID, docPartID string) error

	// Documentation Versions (per-API)
	CreateDocumentationVersion(input CreateDocumentationVersionInput) (*DocumentationVersion, error)
	GetDocumentationVersion(restAPIID, version string) (*DocumentationVersion, error)
	GetDocumentationVersions(restAPIID string) ([]DocumentationVersion, error)
	DeleteDocumentationVersion(restAPIID, version string) error

	// Domain Names
	CreateDomainName(input CreateDomainNameInput) (*DomainName, error)
	GetDomainName(name string) (*DomainName, error)
	GetDomainNames(resourceOwner string) ([]DomainName, error)
	GetDomainNamesPage(limit int, position string) ([]DomainName, string, error)
	DeleteDomainName(name string) error

	// Domain Name Access Associations
	CreateDomainNameAccessAssociation(
		input CreateDomainNameAccessAssociationInput,
	) (*DomainNameAccessAssociation, error)
	GetDomainNameAccessAssociations(resourceOwner string) ([]DomainNameAccessAssociation, error)
	DeleteDomainNameAccessAssociation(arn string) error
	RejectDomainNameAccessAssociation(arn, domainNameARN string) error

	// Models (per-API)
	CreateModel(input CreateModelInput) (*Model, error)
	GetModel(restAPIID, modelName string) (*Model, error)
	GetModels(restAPIID string) ([]Model, error)
	DeleteModel(restAPIID, modelName string) error
	UpdateModel(restAPIID, modelName string, input UpdateModelInput) (*Model, error)

	// Standalone Stage creation
	CreateStage(input CreateStageInput) (*Stage, error)
	UpdateStage(restAPIID, stageName string, input UpdateStageInput) (*Stage, error)

	// Usage Plans
	CreateUsagePlan(input CreateUsagePlanInput) (*UsagePlan, error)
	GetUsagePlan(id string) (*UsagePlan, error)
	GetUsagePlans() ([]UsagePlan, error)
	GetUsagePlansForKey(keyID string) ([]UsagePlan, error)
	GetUsagePlansPage(limit int, position string) ([]UsagePlan, string, error)
	DeleteUsagePlan(id string) error

	// Usage Plan Keys
	CreateUsagePlanKey(input CreateUsagePlanKeyInput) (*UsagePlanKey, error)
	GetUsagePlanKey(usagePlanID, keyID string) (*UsagePlanKey, error)
	GetUsagePlanKeys(usagePlanID string) ([]UsagePlanKey, error)
	DeleteUsagePlanKey(usagePlanID, keyID string) error

	// Account
	GetAccount() (*Account, error)

	// Tags
	GetResourceTags(resourceARN string) (map[string]string, error)
	TagResource(resourceARN string, tags map[string]string) error
	UntagResource(resourceARN string, tagKeys []string) error

	// Test Invocation
	TestInvokeMethod(input TestInvokeMethodInput) (*TestInvokeMethodOutput, error)

	// Update operations.
	UpdateUsagePlan(input UpdateUsagePlanInput) (*UsagePlan, error)
	UpdateDomainName(input UpdateDomainNameInput) (*DomainName, error)
	UpdateBasePathMapping(input UpdateBasePathMappingInput) (*BasePathMapping, error)
	UpdateDocumentationPart(input UpdateDocumentationPartInput) (*DocumentationPart, error)
	UpdateDocumentationVersion(input UpdateDocumentationVersionInput) (*DocumentationVersion, error)
	UpdateMethod(input UpdateMethodInput) (*Method, error)
	UpdateIntegration(input UpdateIntegrationInput) (*Integration, error)
	UpdateIntegrationResponse(input UpdateIntegrationResponseInput) (*IntegrationResponse, error)
	UpdateMethodResponse(input UpdateMethodResponseInput) (*MethodResponse, error)
	UpdateAccount(input UpdateAccountInput) (*Account, error)
	TestInvokeAuthorizer(input TestInvokeAuthorizerInput) (*TestInvokeAuthorizerOutput, error)
	GetModelTemplate(restAPIID, modelName string) (string, error)

	// Gateway response operations.
	GetGatewayResponse(restAPIID, responseType string) (*GatewayResponse, error)
	GetGatewayResponses(restAPIID string) ([]GatewayResponse, error)
	PutGatewayResponse(input PutGatewayResponseInput) (*GatewayResponse, error)
	UpdateGatewayResponse(input PutGatewayResponseInput) (*GatewayResponse, error)
	DeleteGatewayResponse(restAPIID, responseType string) error

	// Client certificate operations.
	GenerateClientCertificate(input GenerateClientCertificateInput) (*ClientCertificate, error)
	GetClientCertificate(id string) (*ClientCertificate, error)
	GetClientCertificates() ([]ClientCertificate, error)
	DeleteClientCertificate(id string) error

	// Usage operations.
	GetUsage(input GetUsageInput) (*UsageData, error)
	// EnforceUsagePlan applies usage-plan quota and throttle/burst limits for an API
	// key on the given API stage. It returns nil when the request is allowed (or when
	// the key is not associated with a usage plan for the stage), ErrQuotaExceeded when
	// the period quota is exhausted, or ErrThrottled when the rate/burst limit is hit.
	EnforceUsagePlan(apiID, stageName, keyID string) error
	// EnforceMethodThrottle applies a stage's MethodSettings throttling for a request to
	// resourcePath/httpMethod, independent of any usage plan or API key. It returns nil
	// when the limit isn't configured or the request is within it, or ErrThrottled when
	// the rate/burst limit is hit.
	EnforceMethodThrottle(apiID, stageName, resourcePath, httpMethod string) error

	// VPC Link operations.
	CreateVpcLink(input CreateVpcLinkInput) (*VpcLink, error)
	GetVpcLink(id string) (*VpcLink, error)
	GetVpcLinks() ([]VpcLink, error)
	DeleteVpcLink(id string) error
	UpdateVpcLink(input UpdateVpcLinkInput) (*VpcLink, error)

	// Client certificate update.
	UpdateClientCertificate(input UpdateClientCertificateInput) (*ClientCertificate, error)

	// OpenAPI export.
	GetExport(restAPIID, stageName, exportType string) (map[string]any, error)

	// SDK generation.
	GetSdkTypes() []SdkType
	GetSdkType(id string) (*SdkType, error)
	GetSdk(restAPIID, stageName, sdkType string) (*SdkExport, error)

	// API key / documentation part bulk import.
	ImportAPIKeys(body []byte, format string, failOnWarnings bool) ([]string, []string, error)
	ImportDocumentationParts(
		restAPIID string,
		body []byte,
		mode string,
		failOnWarnings bool,
	) ([]string, []string, error)

	// Usage update.
	UpdateUsage(usagePlanID, keyID string, dateValues map[string]string) (*UsageData, error)

	// OpenAPI import.
	ImportRestAPI(input ImportRestAPIInput) (*RestAPI, error)
	PutRestAPI(input PutRestAPIInput) (*RestAPI, error)
}

const apiIDChars = "abcdefghijklmnopqrstuvwxyz0123456789"

const (
	apiIDLength       = 10
	resourceIDLength  = 6
	apiKeyValueLength = 40 // AWS generates a 40-character alphanumeric key value

	// defaultBurstLimit and defaultRateLimit match AWS API Gateway defaults.
	defaultBurstLimit = 5000
	defaultRateLimit  = 10000.0

	// defaultPageSize is used when no limit is specified in paginated list operations.
	// AWS API Gateway defaults list operations to a page size of 25.
	defaultPageSize = 25

	// clientCertValidityDays is the number of days a generated client certificate is valid.
	// AWS issues certificates with a 2-year validity period.
	clientCertValidityDays = 730

	// defaultIntegrationTimeoutMs is the AWS default integration timeout in milliseconds.
	// AWS API Gateway applies this when timeoutInMillis is not specified on PutIntegration.
	defaultIntegrationTimeoutMs = 29000
)

// contentTypeJSON is the standard JSON content type used in integration templates and responses.
const contentTypeJSON = "application/json"

// stageInvokeURL returns the gopherstack proxy path for a deployed stage.
// The full URL is relative — clients prepend their gopherstack base URL.
func stageInvokeURL(restAPIID, stageName string) string {
	return "/proxy/" + restAPIID + "/" + stageName
}

// encodePosition returns an opaque, mutation-stable pagination cursor for the given
// sort key. The cursor encodes the key of the last item on the current page (rather
// than a fragile numeric offset), so concurrent inserts/deletes do not cause items
// to be skipped or repeated across pages. AWS returns similarly opaque tokens.
func encodePosition(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("k:" + key))
}

// decodePosition reverses encodePosition. It returns ("", false) for an empty or
// malformed cursor, in which case pagination restarts from the beginning.
func decodePosition(position string) (string, bool) {
	if position == "" {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(position)
	if err != nil {
		return "", false
	}
	s := string(raw)
	after, ok := strings.CutPrefix(s, "k:")
	if !ok {
		return "", false
	}

	return after, true
}

// paginatePageByKey applies limit/cursor pagination to a slice that is already sorted
// ascending by the key returned by keyOf. It returns the page and an opaque next-page
// cursor (empty when the last page is reached). The cursor is mutation-stable: it is
// derived from the last returned item's key, so resuming does the right thing even if
// the underlying collection changed between calls.
func paginatePageByKey[T any](
	all []T,
	limit int,
	position string,
	keyOf func(T) string,
) ([]T, string) {
	if limit <= 0 {
		limit = defaultPageSize
	}

	startIdx := 0
	if cursor, ok := decodePosition(position); ok {
		// Resume at the first item strictly after the cursor key.
		startIdx = sort.Search(len(all), func(i int) bool { return keyOf(all[i]) > cursor })
	}

	if startIdx >= len(all) {
		return []T{}, ""
	}

	end := startIdx + limit
	var outPosition string
	if end < len(all) {
		outPosition = encodePosition(keyOf(all[end-1]))
	} else {
		end = len(all)
	}

	return all[startIdx:end], outPosition
}

// randomID generates a cryptographically random alphanumeric ID of the given length.
func randomID(length int) string {
	b := make([]byte, length)
	charCount := uint64(len(apiIDChars))

	for i := range b {
		var v [8]byte
		_, _ = rand.Read(v[:])
		b[i] = apiIDChars[binary.BigEndian.Uint64(v[:])%charCount]
	}

	return string(b)
}

// initTagsFromInput returns a new tags.Tags store seeded from inputTags (if non-nil)
// or an empty store, using the given name prefix for the backing store label.
func initTagsFromInput(name string, inputTags *tags.Tags) *tags.Tags {
	if inputTags == nil {
		return tags.New(name)
	}

	return tags.FromMap(name, inputTags.Clone())
}

// InMemoryBackend implements StorageBackend using in-memory maps, with every
// resource collection registered as a *store.Table on registry (see
// store_setup.go). Resource families that AWS scopes to a REST API
// (resources, deployments, stages, authorizers, requestValidators,
// documentationParts, documentationVersions, models) are flat tables keyed by
// a composite "<restAPIID>#<childID>" string (see resourceKey et al in
// store_setup.go), with a secondary "byAPI" [store.Index] answering "all
// children of REST API X" -- replacing the old map[string]*apiData nesting.
type InMemoryBackend struct {
	account *Account

	restApis *store.Table[RestAPI]

	resources      *store.Table[Resource]
	resourcesByAPI *store.Index[Resource]

	deployments      *store.Table[Deployment]
	deploymentsByAPI *store.Index[Deployment]

	stages      *store.Table[Stage]
	stagesByAPI *store.Index[Stage]

	authorizers      *store.Table[Authorizer]
	authorizersByAPI *store.Index[Authorizer]

	requestValidators      *store.Table[RequestValidator]
	requestValidatorsByAPI *store.Index[RequestValidator]

	documentationParts      *store.Table[DocumentationPart]
	documentationPartsByAPI *store.Index[DocumentationPart]

	documentationVersions      *store.Table[DocumentationVersion]
	documentationVersionsByAPI *store.Index[DocumentationVersion]

	models      *store.Table[Model]
	modelsByAPI *store.Index[Model]

	// resourceVersions (restAPIID → counter) is bumped whenever a REST API's
	// resource set is mutated. The data-plane proxy uses it to invalidate its
	// cached routing trie. Left as a plain map: not a resource collection, so
	// it doesn't fit store.Table's shape (see store_setup.go's
	// registerAllTables doc).
	resourceVersions map[string]uint64

	apiKeys        *store.Table[APIKey]
	apiKeysByValue map[string]string // key value → key ID, O(1) data-plane lookup

	usage *usageTracker // usage-plan quota + throttle state

	basePathMappings             *store.Table[BasePathMapping] // key: domainName + "#" + basePath
	domainNames                  *store.Table[DomainName]
	domainNameAccessAssociations *store.Table[DomainNameAccessAssociation]

	usagePlans          *store.Table[UsagePlan]
	usagePlanKeys       *store.Table[UsagePlanKey] // key: usagePlanID + "#" + keyID
	usagePlanKeysByPlan *store.Index[UsagePlanKey]

	gatewayResponses   *store.Table[GatewayResponse]   // key: restAPIID + "#" + responseType
	clientCertificates *store.Table[ClientCertificate] // key: clientCertificateID
	vpcLinks           *store.Table[VpcLink]

	// usageOverrides (usagePlanID → keyID → remaining quota) is set via
	// UpdateUsage. Left as a plain map: the value (int64) carries no identity
	// field of its own to key a store.Table by.
	usageOverrides map[string]map[string]int64

	registry *store.Registry
	mu       *lockmetrics.RWMutex
}

// defaultAccount returns the API Gateway account settings AWS assigns to a
// fresh account, used both at construction and on Reset.
func defaultAccount() *Account {
	return &Account{
		ThrottleSettings: &ThrottleSettings{
			BurstLimit: defaultBurstLimit,
			RateLimit:  defaultRateLimit,
		},
		Features:      []string{"UsagePlans"},
		APIKeyVersion: "1",
	}
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		account:          defaultAccount(),
		registry:         store.NewRegistry(),
		resourceVersions: make(map[string]uint64),
		apiKeysByValue:   make(map[string]string),
		usage:            newUsageTracker(),
		usageOverrides:   make(map[string]map[string]int64),
		mu:               lockmetrics.New("apigateway"),
	}
	registerAllTables(b)

	return b
}

// closeAllTags closes the *tags.Tags store of every item in items, skipping
// items whose store is nil.
func closeAllTags[T any](items []*T, get func(*T) *tags.Tags) {
	for _, v := range items {
		if t := get(v); t != nil {
			t.Close()
		}
	}
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	// Close every taggable resource's tag store to prevent resource leaks.
	closeAllTags(b.restApis.All(), func(v *RestAPI) *tags.Tags { return v.Tags })
	closeAllTags(b.apiKeys.All(), func(v *APIKey) *tags.Tags { return v.Tags })
	closeAllTags(b.domainNames.All(), func(v *DomainName) *tags.Tags { return v.Tags })
	closeAllTags(b.usagePlans.All(), func(v *UsagePlan) *tags.Tags { return v.Tags })
	closeAllTags(b.vpcLinks.All(), func(v *VpcLink) *tags.Tags { return v.Tags })
	closeAllTags(
		b.clientCertificates.All(),
		func(v *ClientCertificate) *tags.Tags { return v.Tags },
	)
	closeAllTags(b.stages.All(), func(v *Stage) *tags.Tags { return v.Tags })

	b.registry.ResetAll()
	// The "dirty" tables (see store_setup.go's registerAllTables doc) are
	// deliberately NOT on b.registry, so each needs its own Reset() call here.
	b.resources.Reset()
	b.deployments.Reset()
	b.stages.Reset()
	b.authorizers.Reset()
	b.requestValidators.Reset()
	b.documentationParts.Reset()
	b.documentationVersions.Reset()
	b.models.Reset()
	b.usagePlanKeys.Reset()
	b.resourceVersions = make(map[string]uint64)
	b.apiKeysByValue = make(map[string]string)
	b.usage = newUsageTracker()
	b.usageOverrides = make(map[string]map[string]int64)
	b.account = defaultAccount()
}
