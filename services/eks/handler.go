package eks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyName           = "name"
	keyStatusField    = "status"
	keyVersion        = "version"
	keyCreatedAt      = "createdAt"
	keyNodegroup      = "nodegroup"
	keyUpdate         = "update"
	keyType           = "type"
	keyPrincipalArn   = "principalArn"
	keyUsername       = "username"
	keyPolicyArn      = "policyArn"
	keySubscription   = "subscription"
	keyFargateProfile = "fargateProfile"
	keyTags           = "tags"
	keyEnabled        = "enabled"
	keyModifiedAt     = "modifiedAt"
	keyHealth         = "health"
	keyNamespace      = "namespace"
)

const (
	opUnknown         = "Unknown"
	keyAddons         = "addons"
	keyArn            = "arn"
	keyClusterName    = "clusterName"
	keyCluster        = "cluster"
	keyAccessEntry    = "accessEntry"
	keyAccessEntryArn = "accessEntryArn"
	keyAddon          = "addon"
	keyCapability     = "capability"
	keyAssociation    = "association"
)

const (
	opUpdateNodegroupConfig         = "UpdateNodegroupConfig"
	opUpdateNodegroupVersion        = "UpdateNodegroupVersion"
	opTagResource                   = "TagResource"
	opUntagResource                 = "UntagResource"
	opUpdateAccessEntry             = "UpdateAccessEntry"
	opUpdateAddon                   = "UpdateAddon"
	opUpdateCapability              = "UpdateCapability"
	opUpdateEksAnywhereSubscription = "UpdateEksAnywhereSubscription"
	opUpdatePodIdentityAssociation  = "UpdatePodIdentityAssociation"
	opStartInsightsRefresh          = "StartInsightsRefresh"
	opUpdateClusterConfig           = "UpdateClusterConfig"
	opUpdateClusterVersion          = "UpdateClusterVersion"
	opRegisterCluster               = "RegisterCluster"
	opCancelUpdate                  = "CancelUpdate"
)

const (
	opListClusters                 = "ListClusters"
	opListNodegroups               = "ListNodegroups"
	opListTagsForResource          = "ListTagsForResource"
	opListAccessEntries            = "ListAccessEntries"
	opListAccessPolicies           = "ListAccessPolicies"
	opListAssociatedAccessPolicies = "ListAssociatedAccessPolicies"
	opListAddons                   = "ListAddons"
	opListCapabilities             = "ListCapabilities"
	opListEksAnywhereSubscriptions = "ListEksAnywhereSubscriptions"
	opListFargateProfiles          = "ListFargateProfiles"
	opListPodIdentityAssociations  = "ListPodIdentityAssociations"
	opListIdentityProviderConfigs  = "ListIdentityProviderConfigs"
	opListInsights                 = "ListInsights"
	opListUpdates                  = "ListUpdates"
)

const (
	opDescribeNodegroup                  = "DescribeNodegroup"
	opDisassociateAccessPolicy           = "DisassociateAccessPolicy"
	opDescribePodIdentityAssociation     = "DescribePodIdentityAssociation"
	opDisassociateIdentityProviderConfig = "DisassociateIdentityProviderConfig"
	opDescribeInsight                    = "DescribeInsight"
	opDescribeInsightsRefresh            = "DescribeInsightsRefresh"
	opDescribeUpdate                     = "DescribeUpdate"
)

const (
	opAssociateAccessPolicy           = "AssociateAccessPolicy"
	opAssociateEncryptionConfig       = "AssociateEncryptionConfig"
	opAssociateIdentityProviderConfig = "AssociateIdentityProviderConfig"
	opCreateAccessEntry               = "CreateAccessEntry"
	opCreateAddon                     = "CreateAddon"
	opCreateCapability                = "CreateCapability"
	opCreateCluster                   = "CreateCluster"
	opCreateEksAnywhereSubscription   = "CreateEksAnywhereSubscription"
	opCreateFargateProfile            = "CreateFargateProfile"
	opCreateNodegroup                 = "CreateNodegroup"
	opCreatePodIdentityAssociation    = "CreatePodIdentityAssociation"
	opDeleteAccessEntry               = "DeleteAccessEntry"
	opDeleteAddon                     = "DeleteAddon"
	opDeleteCapability                = "DeleteCapability"
	opDeleteCluster                   = "DeleteCluster"
	opDeleteEksAnywhereSubscription   = "DeleteEksAnywhereSubscription"
	opDeleteFargateProfile            = "DeleteFargateProfile"
	opDeleteNodegroup                 = "DeleteNodegroup"
	opDeletePodIdentityAssociation    = "DeletePodIdentityAssociation"
	opDeregisterCluster               = "DeregisterCluster"
	opDescribeAccessEntry             = "DescribeAccessEntry"
	opDescribeAddon                   = "DescribeAddon"
	opDescribeAddonConfiguration      = "DescribeAddonConfiguration"
	opDescribeAddonVersions           = "DescribeAddonVersions"
	opDescribeCapability              = "DescribeCapability"
	opDescribeCluster                 = "DescribeCluster"
	opDescribeClusterVersions         = "DescribeClusterVersions"
	opDescribeEksAnywhereSubscription = "DescribeEksAnywhereSubscription"
	opDescribeFargateProfile          = "DescribeFargateProfile"
	opDescribeIdentityProviderConfig  = "DescribeIdentityProviderConfig"
)

const (
	maxTagKeyLen  = 128
	maxTagValLen  = 256
	maxTagsPerRes = 50
)

const (
	eksMatchPriority = service.PriorityPathVersioned

	pathClusters = "/clusters"
	pathEKSTags  = "/tags/"
	// pathSubscriptions is /eks-anywhere-subscriptions on the wire (not
	// "/subscriptions" -- verified against aws-sdk-go-v2/service/eks
	// serializers.go's httpbinding.SplitURI for Create/List/Describe/Delete/
	// UpdateEksAnywhereSubscription). The Go identifier keeps the old name for
	// minimal diff churn across call sites.
	pathSubscriptions      = "/eks-anywhere-subscriptions"
	pathAccessPolicies     = "/access-policies"
	pathAddonVersions      = "/addons/supported-versions"
	pathAddonConfiguration = "/addons/configuration-schemas"
	pathClusterVersions    = "/cluster-versions"
	// pathClusterRegistrations is the global (non-cluster-nested) path for
	// RegisterCluster (POST, no id) and DeregisterCluster (DELETE /{name}).
	pathClusterRegistrations = "/cluster-registrations"
)

// Handler is the Echo HTTP handler for AWS EKS operations (REST-JSON protocol).
type Handler struct {
	Backend *InMemoryBackend
}

// NewHandler creates a new EKS handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{Backend: backend}
}

// Reset clears all backend state.
func (h *Handler) Reset() { h.Backend.Reset() }

// Shutdown stops the backend's scheduled state-transition timers so no timer
// goroutine outlives the service. Invoked on server shutdown via
// service.Shutdowner.
func (h *Handler) Shutdown(_ context.Context) { h.Backend.Close() }

// Name returns the service name.
func (h *Handler) Name() string { return "EKS" }

// GetSupportedOperations returns the list of supported EKS operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateCluster,
		opDescribeCluster,
		opListClusters,
		opDeleteCluster,
		opCreateNodegroup,
		opDescribeNodegroup,
		opListNodegroups,
		opDeleteNodegroup,
		opUpdateNodegroupConfig,
		opUpdateNodegroupVersion,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
		opAssociateAccessPolicy,
		opAssociateEncryptionConfig,
		opAssociateIdentityProviderConfig,
		opCreateAccessEntry,
		opDeleteAccessEntry,
		opDescribeAccessEntry,
		opListAccessEntries,
		opUpdateAccessEntry,
		opListAccessPolicies,
		opListAssociatedAccessPolicies,
		opDisassociateAccessPolicy,
		opCreateAddon,
		opDeleteAddon,
		opDescribeAddon,
		opDescribeAddonConfiguration,
		opDescribeAddonVersions,
		opListAddons,
		opUpdateAddon,
		opCreateCapability,
		opDeleteCapability,
		opDescribeCapability,
		opListCapabilities,
		opUpdateCapability,
		opCreateEksAnywhereSubscription,
		opDeleteEksAnywhereSubscription,
		opDescribeEksAnywhereSubscription,
		opListEksAnywhereSubscriptions,
		opUpdateEksAnywhereSubscription,
		opCreateFargateProfile,
		opDeleteFargateProfile,
		opDescribeFargateProfile,
		opListFargateProfiles,
		opCreatePodIdentityAssociation,
		opDeletePodIdentityAssociation,
		opDescribePodIdentityAssociation,
		opListPodIdentityAssociations,
		opUpdatePodIdentityAssociation,
		opDescribeIdentityProviderConfig,
		opListIdentityProviderConfigs,
		opDisassociateIdentityProviderConfig,
		opDescribeInsight,
		opListInsights,
		opStartInsightsRefresh,
		opDescribeInsightsRefresh,
		opUpdateClusterConfig,
		opUpdateClusterVersion,
		opDescribeUpdate,
		opListUpdates,
		opCancelUpdate,
		opRegisterCluster,
		opDeregisterCluster,
		opDescribeClusterVersions,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "eks" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this EKS instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches AWS EKS REST requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		return path == pathClusters ||
			strings.HasPrefix(path, pathClusters+"/") ||
			strings.HasPrefix(path, pathEKSTags+"arn:aws:eks:") ||
			path == pathSubscriptions ||
			strings.HasPrefix(path, pathSubscriptions+"/") ||
			path == pathAccessPolicies ||
			path == pathAddonVersions ||
			path == pathAddonConfiguration ||
			path == pathClusterVersions ||
			path == pathClusterRegistrations ||
			strings.HasPrefix(path, pathClusterRegistrations+"/")
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return eksMatchPriority }

// eksRoute holds the parsed information from an EKS REST request path.
type eksRoute struct {
	clusterName   string
	nodegroupName string
	principalARN  string
	resourceARN   string
	operation     string
}

// parseClusterSubPath handles /clusters/{name}/... paths after extracting clusterName.
func parseClusterSubPath(method, clusterName string, parts []string) eksRoute {
	const maxPathParts = 3

	// /clusters/{name}. UpdateClusterConfig is NOT a bare-path PUT on this
	// route -- its real path is /clusters/{name}/update-config (POST), handled
	// below alongside "capabilities" and "updates".
	if len(parts) == 1 {
		switch method {
		case http.MethodGet:
			return eksRoute{operation: opDescribeCluster, clusterName: clusterName}
		case http.MethodDelete:
			return eksRoute{operation: opDeleteCluster, clusterName: clusterName}
		}

		return eksRoute{operation: opUnknown}
	}

	switch parts[1] {
	case "node-groups":
		return parseNodegroupRoute(method, clusterName, parts)
	case "access-entries":
		return parseAccessEntryRoute(method, clusterName, parts)
	case keyAddons:
		return parseAddonRoute(method, clusterName, parts)
	case "capabilities":
		return parseCapabilityRoute(method, clusterName, parts)
	case "fargate-profiles":
		return parseFargateProfileRoute(method, clusterName, parts)
	case "insights":
		return parseInsightsRoute(method, clusterName, parts)
	case "insights-refresh":
		return parseInsightsRefreshRoute(method, clusterName, parts)
	case "updates":
		return parseUpdatesRoute(method, clusterName, parts)
	case "update-config":
		const updateConfigParts = 2
		if len(parts) == updateConfigParts && method == http.MethodPost {
			return eksRoute{operation: opUpdateClusterConfig, clusterName: clusterName}
		}

		return eksRoute{operation: opUnknown}
	}

	return parseClusterAssocPath(method, clusterName, parts, maxPathParts)
}

// parseClusterAssocPath handles associate paths and pod-identity-associations.
func parseClusterAssocPath(method, clusterName string, parts []string, maxParts int) eksRoute {
	if parts[1] == "encryption-config" && len(parts) == maxParts && parts[2] == "associate" {
		if method == http.MethodPost {
			return eksRoute{operation: opAssociateEncryptionConfig, clusterName: clusterName}
		}

		return eksRoute{operation: opUnknown}
	}

	if parts[1] == "identity-provider-configs" {
		return parseIdentityProviderRoute(method, clusterName, parts, maxParts)
	}

	if parts[1] == "pod-identity-associations" {
		return parsePodIdentityRoute(method, clusterName, parts)
	}

	return eksRoute{operation: opUnknown}
}

// parseEKSPath maps HTTP method + path to an operation name and resource identifiers.
func parseEKSPath(method, rawPath string) eksRoute {
	path, _ := url.PathUnescape(rawPath)

	if r, ok := parseGlobalEKSPath(method, path); ok {
		return r
	}

	// /clusters and /clusters/{name}/...
	if !strings.HasPrefix(path, pathClusters) {
		return eksRoute{operation: opUnknown}
	}

	rest := strings.TrimPrefix(path, pathClusters)

	// /clusters
	if rest == "" {
		if method == http.MethodPost {
			return eksRoute{operation: opCreateCluster}
		}
		if method == http.MethodGet {
			return eksRoute{operation: opListClusters}
		}

		return eksRoute{operation: opUnknown}
	}

	// /clusters/{name}[/...]
	rest = strings.TrimPrefix(rest, "/")

	const maxPathParts = 3

	parts := strings.SplitN(rest, "/", maxPathParts)

	return parseClusterSubPath(method, parts[0], parts)
}

func parseGlobalEKSPath(method, path string) (eksRoute, bool) {
	// /tags/{resourceArn}
	if after, ok := strings.CutPrefix(path, pathEKSTags); ok {
		switch method {
		case http.MethodPost:
			return eksRoute{operation: opTagResource, resourceARN: after}, true
		case http.MethodDelete:
			return eksRoute{operation: opUntagResource, resourceARN: after}, true
		case http.MethodGet:
			return eksRoute{operation: opListTagsForResource, resourceARN: after}, true
		}

		return eksRoute{operation: opUnknown}, true
	}

	if r, ok := parseStaticEKSPath(method, path); ok {
		return r, true
	}

	return parseResourceEKSPath(method, path)
}

func parseStaticEKSPath(method, path string) (eksRoute, bool) {
	switch path {
	case pathAccessPolicies:
		if method == http.MethodGet {
			return eksRoute{operation: opListAccessPolicies}, true
		}

		return eksRoute{operation: opUnknown}, true
	case pathAddonVersions:
		if method == http.MethodGet {
			return eksRoute{operation: opDescribeAddonVersions}, true
		}

		return eksRoute{operation: opUnknown}, true
	case pathClusterVersions:
		if method == http.MethodGet {
			return eksRoute{operation: opDescribeClusterVersions}, true
		}

		return eksRoute{operation: opUnknown}, true
	}

	return eksRoute{}, false
}

func parseResourceEKSPath(method, path string) (eksRoute, bool) {
	if path == pathAddonConfiguration {
		if method == http.MethodGet {
			return eksRoute{operation: opDescribeAddonConfiguration}, true
		}

		return eksRoute{operation: opUnknown}, true
	}

	if r, ok := parseClusterRegistrationsEKSPath(method, path); ok {
		return r, true
	}

	return parseSubscriptionEKSPath(method, path)
}

// ExtractOperation extracts the EKS operation name from the REST path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	r := parseEKSPath(c.Request().Method, c.Request().URL.Path)

	return r.operation
}

// ExtractResource extracts the primary resource identifier from the URL path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	r := parseEKSPath(c.Request().Method, c.Request().URL.Path)
	if r.clusterName != "" {
		return r.clusterName
	}

	return r.resourceARN
}

// Handler returns the Echo handler function for EKS requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := logger.Load(c.Request().Context())
		route := parseEKSPath(c.Request().Method, c.Request().URL.Path)

		log.Debug("eks request", "operation", route.operation, keyCluster, route.clusterName)

		var body []byte
		if c.Request().Body != nil {
			decoder := json.NewDecoder(c.Request().Body)
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err == nil {
				body = raw
			}
		}

		return h.dispatch(c, route, body)
	}
}

// dispatch routes a parsed eksRoute to its owning per-family dispatcher. Each
// dispatcher reports whether it handled the operation so the chain can stop
// at the first match.
func (h *Handler) dispatch(c *echo.Context, route eksRoute, body []byte) error {
	dispatchers := []func(*echo.Context, eksRoute, []byte) (bool, error){
		h.dispatchClusterOps,
		h.dispatchTagOps,
		h.dispatchNodegroupOps,
		h.dispatchAccessEntryOps,
		h.dispatchAddonOps,
		h.dispatchCapabilityOps,
		h.dispatchSubscriptionOps,
		h.dispatchFargateOps,
		h.dispatchPodIdentityOps,
		h.dispatchIDPOps,
		h.dispatchInsightsOps,
		h.dispatchUpdateOps,
	}

	for _, fn := range dispatchers {
		if handled, err := fn(c, route, body); handled {
			return err
		}
	}

	return c.JSON(http.StatusNotFound, errResp("ResourceNotFoundException", "unknown operation: "+route.operation))
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, errResp("ResourceNotFoundException", err.Error()))
	case errors.Is(err, ErrAlreadyExists):
		return c.JSON(http.StatusConflict, errResp("ResourceInUseException", err.Error()))
	case errors.Is(err, ErrValidation):
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", err.Error()))
	case errors.Is(err, ErrInvalidRequest):
		return c.JSON(http.StatusBadRequest, errResp("InvalidRequestException", err.Error()))
	default:
		return c.JSON(http.StatusInternalServerError, errResp("InternalFailure", err.Error()))
	}
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"code": code, "message": msg}
}
