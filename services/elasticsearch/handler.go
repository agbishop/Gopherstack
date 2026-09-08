package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyInstanceType           = "InstanceType"
	keyInstanceCount          = "InstanceCount"
	keyEBSEnabled             = "EBSEnabled"
	keyVolumeSize             = "VolumeSize"
	keyVolumeType             = "VolumeType"
	keyIops                   = "Iops"
	keyThroughput             = "Throughput"
	keyDedicatedMasterEnabled = "DedicatedMasterEnabled"
	keyDedicatedMasterType    = "DedicatedMasterType"
	keyDedicatedMasterCount   = "DedicatedMasterCount"
	keyZoneAwarenessEnabled   = "ZoneAwarenessEnabled"
	keyZoneAwarenessConfig    = "ZoneAwarenessConfig"
	keyWarmEnabled            = "WarmEnabled"
	keyWarmType               = "WarmType"
	keyWarmCount              = "WarmCount"
	keyEnabled                = "Enabled"

	keyCrossClusterSearchConnection = "CrossClusterSearchConnection"
	minimumInstanceCount            = 1
	maximumInstanceCount            = 20

	maxTagKeyLen           = 128
	maxTagValueLen         = 256
	maxTagsPerResource     = 50
	maxDescribeDomainNames = 5
)

const (
	elasticsearchPathPrefix     = "/2015-01-01/es/domain"
	elasticsearchTagsPath       = "/2015-01-01/tags"
	elasticsearchTagsRemove     = "/2015-01-01/tags-removal"
	elasticsearchDomainInfo     = "/2015-01-01/es/domain-info"
	elasticsearchServiceRole    = "/2015-01-01/es/role"
	elasticsearchSoftwareUpdate = "/2015-01-01/es/serviceSoftwareUpdate"
	elasticsearchCCSInbound     = "/2015-01-01/es/ccs/inboundConnection"
	elasticsearchCCSOutbound    = "/2015-01-01/es/ccs/outboundConnection"
	elasticsearchVpcEndpoints   = "/2015-01-01/es/vpcEndpoints"
	elasticsearchPackages       = "/2015-01-01/packages"
	elasticsearchDomainPackages = "/2015-01-01/domain"

	opUnknown         = "Unknown"
	opListDomainNames = "ListDomainNames"
)

const (
	elasticsearchCCSInboundSearch   = "/2015-01-01/es/ccs/inboundConnection/search"
	elasticsearchCCSOutboundSearch  = "/2015-01-01/es/ccs/outboundConnection/search"
	elasticsearchUpgradeDomain      = "/2015-01-01/es/upgradeDomain"
	elasticsearchCompatibleVersions = "/2015-01-01/es/compatibleVersions"
	elasticsearchVersions           = "/2015-01-01/es/versions"
	elasticsearchInstanceTypes      = "/2015-01-01/es/instanceTypes"
	elasticsearchInstanceTypeLimits = "/2015-01-01/es/instanceTypeLimits"
	elasticsearchReservedOfferings  = "/2015-01-01/es/reservedInstanceOfferings"
	elasticsearchReservedInstances  = "/2015-01-01/es/reservedInstances"
	elasticsearchPurchaseReserved   = "/2015-01-01/es/purchaseReservedInstanceOffering"
)

// Handler is the HTTP handler for Elasticsearch operations.
type Handler struct {
	Backend   *InMemoryBackend
	ops       map[string]http.HandlerFunc
	AccountID string
	Region    string
}

// NewHandler creates a new Elasticsearch Handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.buildOps()

	return h
}

// buildOps builds the cached dispatch table for fixed-path Elasticsearch routes.
// Routes with dynamic path segments (e.g., domain name, connection ID) are
// handled separately via the domain and prefix routers.
func (h *Handler) buildOps() map[string]http.HandlerFunc {
	describeInbound := h.handleDescribeInboundCrossClusterSearchConnections
	describeOutbound := h.handleDescribeOutboundCrossClusterSearchConnections
	describeReservedOfferings := h.handleDescribeReservedElasticsearchInstanceOfferings
	purchaseReserved := h.handlePurchaseReservedElasticsearchInstanceOffering

	return map[string]http.HandlerFunc{
		http.MethodPost + " " + elasticsearchDomainInfo:                 h.handleDescribeElasticsearchDomains,
		http.MethodGet + " " + elasticsearchTagsPath:                    h.handleListTags,
		http.MethodPost + " " + elasticsearchTagsPath:                   h.handleAddTags,
		http.MethodPost + " " + elasticsearchTagsRemove:                 h.handleRemoveTags,
		http.MethodDelete + " " + elasticsearchServiceRole:              h.handleDeleteElasticsearchServiceRole,
		http.MethodPost + " " + elasticsearchSoftwareUpdate + "/cancel": h.handleCancelElasticsearchServiceSoftwareUpdate,
		http.MethodPost + " " + elasticsearchSoftwareUpdate + "/start":  h.handleStartElasticsearchServiceSoftwareUpdate,
		http.MethodGet + " " + elasticsearchDomainPackages:              h.handleListDomainNames,
		// elasticsearchPathPrefix (bare, no domain name segment) is this
		// emulator's own collection resource for the domain CRUD family
		// (POST there creates a domain). AWS's real ListDomainNames lives at
		// the distinct elasticsearchDomainPackages ("/2015-01-01/domain")
		// resource per the aws-sdk-go-v2 serializer, and that mapping above
		// is load-bearing for real SDK clients. This second entry additionally
		// serves GET on the bare CRUD collection path as ListDomainNames too,
		// so a caller that lists the same collection resource it created
		// against gets a 200 instead of falling through to a 404 -- it does
		// not shadow CreateElasticsearchDomain (POST) or DescribeElasticsearchDomain
		// (GET with a name segment).
		http.MethodGet + " " + elasticsearchPathPrefix:                  h.handleListDomainNames,
		http.MethodPost + " " + elasticsearchCCSOutbound:                h.handleCreateOutboundCrossClusterSearchConnection,
		http.MethodPost + " " + elasticsearchVpcEndpoints:               h.handleCreateVpcEndpoint,
		http.MethodPost + " " + elasticsearchPackages:                   h.handleCreatePackage,
		http.MethodPost + " " + elasticsearchCCSInboundSearch:           describeInbound,
		http.MethodPost + " " + elasticsearchCCSOutboundSearch:          describeOutbound,
		http.MethodPost + " " + elasticsearchPackages + "/describe":     h.handleDescribePackages,
		http.MethodPost + " " + elasticsearchPackages + "/update":       h.handleUpdatePackage,
		http.MethodPost + " " + elasticsearchVpcEndpoints + "/describe": h.handleDescribeVpcEndpoints,
		http.MethodPost + " " + elasticsearchVpcEndpoints + "/update":   h.handleUpdateVpcEndpoint,
		http.MethodGet + " " + elasticsearchVpcEndpoints:                h.handleListVpcEndpoints,
		http.MethodGet + " " + elasticsearchCompatibleVersions:          h.handleGetCompatibleElasticsearchVersions,
		http.MethodGet + " " + elasticsearchVersions:                    h.handleListElasticsearchVersions,
		http.MethodGet + " " + elasticsearchReservedOfferings:           describeReservedOfferings,
		http.MethodGet + " " + elasticsearchReservedInstances:           h.handleDescribeReservedElasticsearchInstances,
		http.MethodPost + " " + elasticsearchPurchaseReserved:           purchaseReserved,
		http.MethodPost + " " + elasticsearchUpgradeDomain:              h.handleUpgradeElasticsearchDomain,
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "Elasticsearch" }

// reqContext returns the request context with the SigV4-derived AWS region
// attached so backend operations route to the correct per-region store.
func (h *Handler) reqContext(r *http.Request) context.Context {
	region := httputils.ExtractRegionFromRequest(r, h.Backend.Region())

	return context.WithValue(r.Context(), regionContextKey{}, region)
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityPathSubdomain }

// RouteMatcher returns a matcher that selects Elasticsearch requests by path prefix.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return matchElasticsearchPath(c.Request().URL.Path)
	}
}

// matchElasticsearchPath returns true if the path matches a known Elasticsearch API path.
func matchElasticsearchPath(path string) bool {
	return matchElasticsearchCorePaths(path) || matchElasticsearchExtPaths(path)
}

// matchElasticsearchCorePaths returns true if path matches core Elasticsearch paths.
func matchElasticsearchCorePaths(path string) bool {
	return strings.HasPrefix(path, elasticsearchPathPrefix) ||
		path == elasticsearchDomainInfo ||
		path == elasticsearchTagsPath ||
		path == elasticsearchTagsRemove ||
		path == elasticsearchServiceRole ||
		strings.HasPrefix(path, elasticsearchSoftwareUpdate) ||
		strings.HasPrefix(path, elasticsearchCCSInbound) ||
		strings.HasPrefix(path, elasticsearchCCSOutbound) ||
		path == elasticsearchVpcEndpoints ||
		strings.HasPrefix(path, elasticsearchVpcEndpoints+"/")
}

// matchElasticsearchExtPaths returns true if path matches extended Elasticsearch paths.
func matchElasticsearchExtPaths(path string) bool {
	return strings.HasPrefix(path, elasticsearchPackages) ||
		path == elasticsearchDomainPackages ||
		strings.HasPrefix(path, elasticsearchDomainPackages+"/") ||
		strings.HasPrefix(path, elasticsearchUpgradeDomain) ||
		path == elasticsearchCompatibleVersions ||
		path == elasticsearchVersions ||
		strings.HasPrefix(path, elasticsearchInstanceTypes) ||
		strings.HasPrefix(path, elasticsearchInstanceTypeLimits) ||
		path == elasticsearchReservedOfferings ||
		path == elasticsearchReservedInstances ||
		path == elasticsearchPurchaseReserved
}

// GetSupportedOperations returns supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AcceptInboundCrossClusterSearchConnection",
		"AddTags",
		"AssociatePackage",
		"AuthorizeVpcEndpointAccess",
		"CancelDomainConfigChange",
		"CancelElasticsearchServiceSoftwareUpdate",
		"CreateElasticsearchDomain",
		"CreateOutboundCrossClusterSearchConnection",
		"CreatePackage",
		"CreateVpcEndpoint",
		"DeleteElasticsearchDomain",
		"DeleteElasticsearchServiceRole",
		"DeleteInboundCrossClusterSearchConnection",
		"DeleteOutboundCrossClusterSearchConnection",
		"DeletePackage",
		"DeleteVpcEndpoint",
		"DescribeDomainAutoTunes",
		"DescribeDomainChangeProgress",
		"DescribeElasticsearchDomain",
		"DescribeElasticsearchDomainConfig",
		"DescribeElasticsearchDomains",
		"DescribeElasticsearchInstanceTypeLimits",
		"DescribeInboundCrossClusterSearchConnections",
		"DescribeOutboundCrossClusterSearchConnections",
		"DescribePackages",
		"DescribeReservedElasticsearchInstanceOfferings",
		"DescribeReservedElasticsearchInstances",
		"DescribeVpcEndpoints",
		"DissociatePackage",
		"GetCompatibleElasticsearchVersions",
		"GetPackageVersionHistory",
		"GetUpgradeHistory",
		"GetUpgradeStatus",
		opListDomainNames,
		"ListDomainsForPackage",
		"ListElasticsearchInstanceTypes",
		"ListElasticsearchVersions",
		"ListPackagesForDomain",
		"ListTags",
		"ListVpcEndpointAccess",
		"ListVpcEndpoints",
		"ListVpcEndpointsForDomain",
		"PurchaseReservedElasticsearchInstanceOffering",
		"RejectInboundCrossClusterSearchConnection",
		"RemoveTags",
		"RevokeVpcEndpointAccess",
		"StartElasticsearchServiceSoftwareUpdate",
		"UpdateElasticsearchDomainConfig",
		"UpdatePackage",
		"UpdateVpcEndpoint",
		"UpgradeElasticsearchDomain",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "es" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Elasticsearch instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// ExtractResource returns the domain name from the request path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path
	rest := strings.TrimPrefix(path, elasticsearchPathPrefix+"/")

	if rest == path {
		return ""
	}

	return strings.TrimSuffix(rest, "/")
}

// ServeHTTP implements [http.Handler] for the Elasticsearch service.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Fast O(1) lookup for fixed-path routes.
	key := r.Method + " " + r.URL.Path
	if fn, ok := h.ops[key]; ok {
		fn(w, r)

		return
	}

	if h.handlePrefixRoutes(w, r) {
		return
	}

	h.handleDomainRoutes(w, r)
}

// handlePrefixRoutes handles routes that require prefix matching with path params.
// Returns true if the request was handled.
func (h *Handler) handlePrefixRoutes(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path

	if h.handleCCSPrefixRoutes(w, r, path) {
		return true
	}

	if h.handlePackagePrefixRoutes(w, r, path) {
		return true
	}

	return h.handleInstanceUpgradePrefixRoutes(w, r, path)
}

// handleCCSPrefixRoutes handles CCS and VPC endpoint prefix routes.
func (h *Handler) handleCCSPrefixRoutes(w http.ResponseWriter, r *http.Request, path string) bool {
	// CCS inbound routes (ordered most-specific first)
	if strings.HasPrefix(path, elasticsearchCCSInbound+"/") {
		switch {
		case strings.HasSuffix(path, "/accept") && r.Method == http.MethodPut:
			h.handleAcceptInboundCrossClusterSearchConnection(w, r)

			return true
		case strings.HasSuffix(path, "/reject") && r.Method == http.MethodPut:
			h.handleRejectInboundCrossClusterSearchConnection(w, r)

			return true
		case r.Method == http.MethodDelete:
			h.handleDeleteInboundCrossClusterSearchConnection(w, r)

			return true
		}
	}

	if strings.HasPrefix(path, elasticsearchCCSOutbound+"/") && r.Method == http.MethodDelete {
		h.handleDeleteOutboundCrossClusterSearchConnection(w, r)

		return true
	}

	if strings.HasPrefix(path, elasticsearchVpcEndpoints+"/") && r.Method == http.MethodDelete {
		h.handleDeleteVpcEndpoint(w, r)

		return true
	}

	return false
}

// handlePackagePrefixRoutes handles package and domain-package prefix routes.
func (h *Handler) handlePackagePrefixRoutes(w http.ResponseWriter, r *http.Request, path string) bool {
	if h.handlePackageAssocDisassoc(w, r, path) {
		return true
	}

	return h.handlePackageHistoryAndDelete(w, r, path)
}

// handlePackageAssocDisassoc handles package association and history listing operations.
func (h *Handler) handlePackageAssocDisassoc(w http.ResponseWriter, r *http.Request, path string) bool {
	switch {
	case strings.HasPrefix(path, elasticsearchPackages+"/associate/") && r.Method == http.MethodPost:
		h.handleAssociatePackage(w, r)

		return true
	case strings.HasPrefix(path, elasticsearchPackages+"/dissociate/") && r.Method == http.MethodPost:
		h.handleDissociatePackage(w, r)

		return true
	case strings.HasPrefix(path, elasticsearchPackages+"/") &&
		strings.HasSuffix(path, "/history") &&
		r.Method == http.MethodGet:
		h.handleGetPackageVersionHistory(w, r)

		return true
	case strings.HasPrefix(path, elasticsearchPackages+"/") &&
		strings.HasSuffix(path, "/domains") &&
		r.Method == http.MethodGet:
		h.handleListDomainsForPackage(w, r)

		return true
	}

	return false
}

// handlePackageHistoryAndDelete handles package delete and domain-package listing.
func (h *Handler) handlePackageHistoryAndDelete(w http.ResponseWriter, r *http.Request, path string) bool {
	if strings.HasPrefix(path, elasticsearchPackages+"/") && r.Method == http.MethodDelete {
		rest := strings.TrimPrefix(path, elasticsearchPackages+"/")
		if !strings.Contains(rest, "/") {
			h.handleDeletePackage(w, r)

			return true
		}
	}

	if strings.HasPrefix(path, elasticsearchDomainPackages+"/") &&
		strings.HasSuffix(path, "/packages") &&
		r.Method == http.MethodGet {
		h.handleListPackagesForDomain(w, r)

		return true
	}

	return false
}

// handleInstanceUpgradePrefixRoutes handles instance type and upgrade prefix routes.
func (h *Handler) handleInstanceUpgradePrefixRoutes(w http.ResponseWriter, r *http.Request, path string) bool {
	if strings.HasPrefix(path, elasticsearchInstanceTypes+"/") && r.Method == http.MethodGet {
		h.handleListElasticsearchInstanceTypes(w, r)

		return true
	}

	if strings.HasPrefix(path, elasticsearchInstanceTypeLimits+"/") && r.Method == http.MethodGet {
		h.handleDescribeElasticsearchInstanceTypeLimits(w, r)

		return true
	}

	if strings.HasPrefix(path, elasticsearchUpgradeDomain+"/") {
		switch {
		case strings.HasSuffix(path, "/history") && r.Method == http.MethodGet:
			h.handleGetUpgradeHistory(w, r)

			return true
		case strings.HasSuffix(path, "/status") && r.Method == http.MethodGet:
			h.handleGetUpgradeStatus(w, r)

			return true
		}
	}

	return false
}

func (h *Handler) handleDomainRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, elasticsearchPathPrefix)

	switch {
	case (rest == "" || rest == "/") && r.Method == http.MethodPost:
		h.handleCreateDomain(w, r)
	// Note: a bare GET here (ListDomainNames) is handled by the fast-path
	// buildOps entry in ServeHTTP before this function is ever reached; it
	// is not duplicated in this switch. See the buildOps comment.
	case strings.HasPrefix(rest, "/") && r.Method == http.MethodGet:
		h.handleGetDomainRoute(w, r, rest)
	case strings.HasPrefix(rest, "/") && r.Method == http.MethodDelete:
		h.handleDeleteDomain(w, r, domainNameFromRest(rest))
	case strings.HasPrefix(rest, "/") && r.Method == http.MethodPost:
		h.handlePostDomainRoute(w, r, rest)
	default:
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", "route not found")
	}
}

func (h *Handler) handleGetDomainRoute(w http.ResponseWriter, r *http.Request, rest string) {
	trimmed := domainNameFromRest(rest)
	switch {
	case strings.HasSuffix(trimmed, "/config"):
		domainName, _ := strings.CutSuffix(trimmed, "/config")
		h.handleDescribeDomainConfig(w, r, domainName)
	case strings.HasSuffix(trimmed, "/autoTunes"):
		domainName, _ := strings.CutSuffix(trimmed, "/autoTunes")
		h.handleDescribeDomainAutoTunes(w, r, domainName)
	case strings.HasSuffix(trimmed, "/progress"):
		domainName, _ := strings.CutSuffix(trimmed, "/progress")
		h.handleDescribeDomainChangeProgress(w, r, domainName)
	case strings.HasSuffix(trimmed, "/listVpcEndpointAccess"):
		domainName, _ := strings.CutSuffix(trimmed, "/listVpcEndpointAccess")
		h.handleListVpcEndpointAccess(w, r, domainName)
	case strings.HasSuffix(trimmed, "/vpcEndpoints"):
		domainName, _ := strings.CutSuffix(trimmed, "/vpcEndpoints")
		h.handleListVpcEndpointsForDomain(w, r, domainName)
	default:
		h.handleDescribeDomain(w, r, trimmed)
	}
}

func (h *Handler) handlePostDomainRoute(w http.ResponseWriter, r *http.Request, rest string) {
	trimmed := domainNameFromRest(rest)

	switch {
	case strings.HasSuffix(trimmed, "/config/cancel"):
		domainName, _ := strings.CutSuffix(trimmed, "/config/cancel")
		h.handleCancelDomainConfigChange(w, r, domainName)
	case strings.HasSuffix(trimmed, "/config"):
		domainName, _ := strings.CutSuffix(trimmed, "/config")
		h.handleUpdateDomainConfig(w, r, domainName)
	case strings.HasSuffix(trimmed, "/authorizeVpcEndpointAccess"):
		domainName, _ := strings.CutSuffix(trimmed, "/authorizeVpcEndpointAccess")
		h.handleAuthorizeVpcEndpointAccess(w, r, domainName)
	case strings.HasSuffix(trimmed, "/revokeVpcEndpointAccess"):
		domainName, _ := strings.CutSuffix(trimmed, "/revokeVpcEndpointAccess")
		h.handleRevokeVpcEndpointAccess(w, r, domainName)
	default:
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", "route not found")
	}
}

func domainNameFromRest(rest string) string {
	name := strings.TrimPrefix(rest, "/")

	return strings.TrimSuffix(name, "/")
}

// Handle satisfies the Echo handler interface.
func (h *Handler) Handle(c *echo.Context) error {
	h.ServeHTTP(c.Response(), c.Request())

	return nil
}

// Handler returns the Echo HandlerFunc for this service.
func (h *Handler) Handler() echo.HandlerFunc {
	return h.Handle
}

type errorResponseJSON struct {
	Message string `json:"message"`
}

func (h *Handler) writeError(r *http.Request, w http.ResponseWriter, status int, code, message string) {
	ctx := r.Context()
	logger.Load(ctx).ErrorContext(r.Context(), "elasticsearch error", "code", code, "message", message)
	w.Header().Set("x-amzn-ErrorType", code)
	httputils.WriteJSON(ctx, w, status, errorResponseJSON{Message: message})
}

func (h *Handler) writeJSON(r *http.Request, w http.ResponseWriter, v any) {
	httputils.WriteJSON(r.Context(), w, http.StatusOK, v)
}

func (h *Handler) decodeRequest(w http.ResponseWriter, r *http.Request, out any) bool {
	body, err := httputils.ReadBody(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "failed to read body")

		return false
	}

	if len(body) == 0 {
		return true
	}

	if err = json.Unmarshal(body, out); err != nil {
		h.writeError(r, w, http.StatusBadRequest, "ValidationException", "invalid JSON body")

		return false
	}

	return true
}

func (h *Handler) writeOperationError(r *http.Request, w http.ResponseWriter, err error) {
	if errors.Is(err, ErrDomainNotFound) || errors.Is(err, ErrPackageNotFound) ||
		errors.Is(err, ErrVpcEndpointNotFound) || errors.Is(err, ErrConnectionNotFound) ||
		errors.Is(err, ErrOfferingNotFound) {
		h.writeError(r, w, http.StatusNotFound, "ResourceNotFoundException", err.Error())

		return
	}

	h.writeError(r, w, http.StatusBadRequest, "ValidationException", err.Error())
}

func pathID(path, prefix, suffix string) string {
	id := strings.TrimPrefix(path, prefix)
	id, _ = strings.CutSuffix(id, suffix)

	return id
}
