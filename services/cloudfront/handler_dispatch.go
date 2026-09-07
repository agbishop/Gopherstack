package cloudfront

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func (h *Handler) dispatch(c *echo.Context, operation, resource string) error {
	if err := h.dispatchCreate(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	if err := h.dispatchList(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	if err := h.dispatchGetOrMutate(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	if err := h.dispatchMisc(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchStubs(c, operation)
}

// errNotDispatched is a sentinel returned by sub-dispatchers when an operation
// did not match, so the caller can try the next sub-dispatcher.
var errNotDispatched = errors.New("not dispatched")

func (h *Handler) dispatchCreate(c *echo.Context, operation string) error {
	if err := h.dispatchCreateCore(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchCreateExtended(c, operation)
}

func (h *Handler) dispatchCreateCore(c *echo.Context, operation string) error {
	switch operation {
	case opCreateAnycastIPList:
		return h.handleCreateAnycastIPList(c)
	case opCreateCachePolicy:
		return h.handleCreateCachePolicy(c)
	case opCreateCloudFrontOriginAccessIdentity:
		return h.handleCreateOAI(c)
	case opCreateConnectionFunction:
		return h.handleCreateConnectionFunction(c)
	case opCreateConnectionGroup:
		return h.handleCreateConnectionGroup(c)
	case opCreateContinuousDeploymentPolicy:
		return h.handleCreateContinuousDeploymentPolicy(c)
	case opCreateDistribution:
		return h.handleCreateDistribution(c)
	case opCreateFunction:
		return h.handleCreateFunction(c)
	case opCreateOriginAccessControl:
		return h.handleCreateOriginAccessControl(c)
	case opCreateOriginRequestPolicy:
		return h.handleCreateOriginRequestPolicy(c)
	case opCreateResponseHeadersPolicy:
		return h.handleCreateResponseHeadersPolicy(c)
	default:

		return errNotDispatched
	}
}

func (h *Handler) dispatchCreateExtended(c *echo.Context, operation string) error {
	switch operation {
	case opCreateFieldLevelEncryptionConfig:
		return h.handleCreateFieldLevelEncryption(c)
	case opCreateFieldLevelEncryptionProfile:
		return h.handleCreateFieldLevelEncryptionProfile(c)
	case opCreatePublicKey:
		return h.handleCreatePublicKey(c)
	case opCreateKeyGroup:
		return h.handleCreateKeyGroup(c)
	case opCreateRealtimeLogConfig:
		return h.handleCreateRealtimeLogConfig(c)
	case opCreateKeyValueStore:
		return h.handleCreateKeyValueStore(c)
	case opCreateVpcOrigin:
		return h.handleCreateVpcOrigin(c)
	case opCreateStreamingDistribution:
		return h.handleCreateStreamingDistribution(c)
	case opCreateStreamingDistributionWithTags:
		return h.handleCreateStreamingDistributionWithTags(c)
	default:

		return errNotDispatched
	}
}

// dispatchGetOrMutate handles all GET, PUT, and DELETE operations.
//

func (h *Handler) dispatchGetOrMutate(c *echo.Context, operation, resource string) error {
	if err := h.dispatchGetOrMutateCoreOps(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	if err := h.dispatchGetOrMutateEncryptionOps(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchGetOrMutateExtOps(c, operation, resource)
}

// dispatchGetOrMutateCoreOps handles core GET, DELETE, and UPDATE operations.
func (h *Handler) dispatchGetOrMutateCoreOps(c *echo.Context, operation, resource string) error {
	if err := h.dispatchGetDistributionAndCachePolicyOps(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchGetIdentityAndPolicyDeleteOps(c, operation, resource)
}

// dispatchGetDistributionAndCachePolicyOps handles get ops for distributions, cache policy, functions, OAI, and OAC.
func (h *Handler) dispatchGetDistributionAndCachePolicyOps(
	c *echo.Context, operation, resource string,
) error {
	switch operation {
	case opGetCachePolicy:
		return h.handleGetCachePolicy(c, resource)
	case opGetCachePolicyConfig:
		return h.handleGetCachePolicyConfig(c, resource)
	case opGetDistribution:
		return h.handleGetDistribution(c, resource)
	case opGetDistributionConfig:
		return h.handleGetDistributionConfig(c, resource)
	case opGetFunction:
		return h.handleGetFunction(c, resource)
	case opGetInvalidation:
		return h.handleGetInvalidation(c, resource)
	case opGetOriginAccessControl:
		return h.handleGetOriginAccessControl(c, resource)
	case opGetOriginAccessControlConfig:
		return h.handleGetOriginAccessControlConfig(c, resource)
	case opGetCloudFrontOriginAccessIdentity:
		return h.handleGetOAI(c, resource)
	}

	return errNotDispatched
}

// dispatchGetIdentityAndPolicyDeleteOps handles get ops for OAI config / policies and all core delete ops.
func (h *Handler) dispatchGetIdentityAndPolicyDeleteOps(
	c *echo.Context, operation, resource string,
) error {
	switch operation {
	case opGetCloudFrontOriginAccessIdentityConfig:
		return h.handleGetOAIConfig(c, resource)
	case opGetOriginRequestPolicy:
		return h.handleGetOriginRequestPolicy(c, resource)
	case opGetOriginRequestPolicyConfig:
		return h.handleGetOriginRequestPolicyConfig(c, resource)
	case opGetResponseHeadersPolicy:
		return h.handleGetResponseHeadersPolicy(c, resource)
	case opGetResponseHeadersPolicyConfig:
		return h.handleGetResponseHeadersPolicyConfig(c, resource)
	case opDeleteCachePolicy:
		return h.handleDeleteCachePolicy(c, resource)
	case opDeleteDistribution:
		return h.handleDeleteDistribution(c, resource)
	case opDeleteFunction:
		return h.handleDeleteFunction(c, resource)
	case opDeleteOriginAccessControl:
		return h.handleDeleteOriginAccessControl(c, resource)
	}

	return errNotDispatched
}

// dispatchGetOrMutateEncryptionOps handles OAI, policy, and encryption operations.
func (h *Handler) dispatchGetOrMutateEncryptionOps(c *echo.Context, operation, resource string) error {
	if err := h.dispatchUpdateDeletePolicyAndOAIOps(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchFieldLevelEncryptionOps(c, operation, resource)
}

// dispatchUpdateDeletePolicyAndOAIOps handles update/delete operations for OAI, policies, distribution, and function.
func (h *Handler) dispatchUpdateDeletePolicyAndOAIOps(
	c *echo.Context, operation, resource string,
) error {
	switch operation {
	case opDeleteCloudFrontOriginAccessIdentity:
		return h.handleDeleteOAI(c, resource)
	case opDeleteOriginRequestPolicy:
		return h.handleDeleteOriginRequestPolicy(c, resource)
	case opDeleteResponseHeadersPolicy:
		return h.handleDeleteResponseHeadersPolicy(c, resource)
	case opUpdateCloudFrontOAI:
		return h.handleUpdateOAI(c, resource)
	case opUpdateCachePolicy:
		return h.handleUpdateCachePolicy(c, resource)
	case opUpdateDistribution:
		return h.handleUpdateDistribution(c, resource)
	case opUpdateFunction:
		return h.handleUpdateFunction(c, resource)
	case opUpdateOriginAccessControl:
		return h.handleUpdateOriginAccessControl(c, resource)
	case opUpdateOriginRequestPolicy:
		return h.handleUpdateOriginRequestPolicy(c, resource)
	case opUpdateResponseHeadersPolicy:
		return h.handleUpdateResponseHeadersPolicy(c, resource)
	}

	return errNotDispatched
}

// dispatchFieldLevelEncryptionOps handles field-level encryption config and profile operations.
func (h *Handler) dispatchFieldLevelEncryptionOps(
	c *echo.Context, operation, resource string,
) error {
	switch operation {
	case opGetFieldLevelEncryption:
		return h.handleGetFieldLevelEncryption(c, resource)
	case opGetFieldLevelEncryptionConfig:
		return h.handleGetFieldLevelEncryptionConfig(c, resource)
	case opUpdateFieldLevelEncryptionConfig:
		return h.handleUpdateFieldLevelEncryption(c, resource)
	case opDeleteFieldLevelEncryptionConfig:
		return h.handleDeleteFieldLevelEncryption(c, resource)
	case opGetFieldLevelEncryptionProfile:
		return h.handleGetFieldLevelEncryptionProfile(c, resource)
	case opGetFieldLevelEncryptionProfileConfig:
		return h.handleGetFieldLevelEncryptionProfileConfig(c, resource)
	case opUpdateFieldLevelEncryptionProfile:
		return h.handleUpdateFieldLevelEncryptionProfile(c, resource)
	case opDeleteFieldLevelEncryptionProfile:
		return h.handleDeleteFieldLevelEncryptionProfile(c, resource)
	}

	return errNotDispatched
}

// dispatchGetOrMutateExtOps handles public key, key group, log config, key value store, VPC origin,
// and streaming distribution operations.
func (h *Handler) dispatchGetOrMutateExtOps(c *echo.Context, operation, resource string) error {
	if err := h.dispatchPublicKeyAndGroupOps(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	if err := h.dispatchLogStoreVPCOps(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchStreamingDistributionOps(c, operation, resource)
}

// dispatchStreamingDistributionOps handles get, update, and delete operations for streaming distributions.
func (h *Handler) dispatchStreamingDistributionOps(c *echo.Context, operation, resource string) error {
	switch operation {
	case opGetStreamingDistribution:
		return h.handleGetStreamingDistribution(c, resource)
	case opGetStreamingDistributionConfig:
		return h.handleGetStreamingDistributionConfig(c, resource)
	case opUpdateStreamingDistribution:
		return h.handleUpdateStreamingDistribution(c, resource)
	case opDeleteStreamingDistribution:
		return h.handleDeleteStreamingDistribution(c, resource)
	default:

		return errNotDispatched
	}
}

// dispatchPublicKeyAndGroupOps handles public key and key group operations.
func (h *Handler) dispatchPublicKeyAndGroupOps(c *echo.Context, operation, resource string) error {
	switch operation {
	case opGetPublicKey:
		return h.handleGetPublicKey(c, resource)
	case opGetPublicKeyConfig:
		return h.handleGetPublicKeyConfig(c, resource)
	case opUpdatePublicKey:
		return h.handleUpdatePublicKey(c, resource)
	case opDeletePublicKey:
		return h.handleDeletePublicKey(c, resource)
	case opGetKeyGroup:
		return h.handleGetKeyGroup(c, resource)
	case opGetKeyGroupConfig:
		return h.handleGetKeyGroupConfig(c, resource)
	case opUpdateKeyGroup:
		return h.handleUpdateKeyGroup(c, resource)
	case opDeleteKeyGroup:
		return h.handleDeleteKeyGroup(c, resource)
	}

	return errNotDispatched
}

// dispatchLogStoreVPCOps handles realtime log config, key value store, and VPC origin operations.
func (h *Handler) dispatchLogStoreVPCOps(c *echo.Context, operation, resource string) error {
	if err := h.dispatchKVSOps(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	switch operation {
	case opGetRealtimeLogConfig:
		return h.handleGetRealtimeLogConfig(c)
	case opUpdateRealtimeLogConfig:
		return h.handleUpdateRealtimeLogConfig(c)
	case opDeleteRealtimeLogConfig:
		return h.handleDeleteRealtimeLogConfig(c)
	case opGetVpcOrigin:
		return h.handleGetVpcOrigin(c, resource)
	case opUpdateVpcOrigin:
		return h.handleUpdateVpcOrigin(c, resource)
	case opDeleteVpcOrigin:
		return h.handleDeleteVpcOrigin(c, resource)
	case opGetContinuousDeploymentPolicy, opGetContinuousDeploymentPolicyConfig:
		return h.handleGetContinuousDeploymentPolicy(c, resource)
	case opUpdateContinuousDeploymentPolicy:
		return h.handleUpdateContinuousDeploymentPolicy(c, resource)
	case opDeleteContinuousDeploymentPolicy:
		return h.handleDeleteContinuousDeploymentPolicy(c, resource)
	default:

		return errNotDispatched
	}
}

// dispatchKVSOps handles KVS control-plane operations.
func (h *Handler) dispatchKVSOps(c *echo.Context, operation, resource string) error {
	switch operation {
	case opDescribeKeyValueStore:
		return h.handleGetKeyValueStore(c, resource)
	case opUpdateKeyValueStore:
		return h.handleUpdateKeyValueStore(c, resource)
	case opDeleteKeyValueStore:
		return h.handleDeleteKeyValueStore(c, resource)
	default:

		return errNotDispatched
	}
}

func (h *Handler) dispatchList(c *echo.Context, operation, resource string) error {
	if err := h.dispatchListCore(c, operation, resource); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchListExtended(c, operation)
}

func (h *Handler) dispatchListCore(c *echo.Context, operation, resource string) error {
	switch operation {
	case opListCachePolicies:
		return h.handleListCachePolicies(c)
	case opListDistributions:
		return h.handleListDistributions(c)
	case opListFunctions:
		return h.handleListFunctions(c)
	case opListInvalidations:
		return h.handleListInvalidations(c, resource)
	case opListOriginAccessControls:
		return h.handleListOriginAccessControls(c)
	case opListCloudFrontOriginAccessIdentities:
		return h.handleListOAIs(c)
	case opListOriginRequestPolicies:
		return h.handleListOriginRequestPolicies(c)
	case opListResponseHeadersPolicies:
		return h.handleListResponseHeadersPolicies(c)
	case opListTagsForResource:
		return h.handleListTagsForResource(c)
	default:

		return errNotDispatched
	}
}

func (h *Handler) dispatchListExtended(c *echo.Context, operation string) error {
	switch operation {
	case opListFieldLevelEncryptionConfigs:
		return h.handleListFieldLevelEncryptions(c)
	case opListFieldLevelEncryptionProfiles:
		return h.handleListFieldLevelEncryptionProfiles(c)
	case opListPublicKeys:
		return h.handleListPublicKeys(c)
	case opListKeyGroups:
		return h.handleListKeyGroups(c)
	case opListRealtimeLogConfigs:
		return h.handleListRealtimeLogConfigs(c)
	case opListKeyValueStores:
		return h.handleListKeyValueStores(c)
	case opListVpcOrigins:
		return h.handleListVpcOrigins(c)
	case opListContinuousDeploymentPolicies:
		return h.handleListContinuousDeploymentPolicies(c)
	case opListStreamingDistributions:
		return h.handleListStreamingDistributions(c)
	default:

		return errNotDispatched
	}
}

func (h *Handler) dispatchMisc(c *echo.Context, operation, resource string) error {
	switch operation {
	case opAssociateAlias:
		return h.handleAssociateAlias(c, resource)
	case opAssociateDistributionTenantWebACL:
		return h.handleAssociateDistributionTenantWebACL(c, resource)
	case opAssociateDistributionWebACL:
		return h.handleAssociateDistributionWebACL(c, resource)
	case opCopyDistribution:
		return h.handleCopyDistribution(c, resource)
	case opCreateInvalidation:
		return h.handleCreateInvalidation(c, resource)
	case opDescribeFunction:
		return h.handleDescribeFunction(c, resource)
	case opPublishFunction:
		return h.handlePublishFunction(c, resource)
	case opTagResource:
		return h.handleTagResource(c)
	case opTestFunction:
		return h.handleTestFunction(c, resource)
	case opUntagResource:
		return h.handleUntagResource(c)
	case opGetFunctionAssociations:
		return h.handleGetFunctionAssociations(c, resource)
	case opSetFunctionAssociations:
		return h.handleSetFunctionAssociations(c, resource)
	default:

		return errNotDispatched
	}
}

// dispatchStubs is the dispatch table for CloudFront operations that don't have a
// dedicated route match earlier in the handler chain. Despite the legacy name, every
// operation handled here is backed by real InMemoryBackend state — there are no
// remaining hardcoded/empty stub responses. Unknown operations fall through to a
// real NoSuchOperation error at the end of the chain.
func (h *Handler) dispatchStubs(c *echo.Context, operation string) error {
	if err := h.dispatchStubsDistributions(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	if err := h.dispatchStubsTrustAndMisc(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchStubsConnectionAndPolicy(c, operation)
}

// dispatchStubsDistributions handles distribution tenant and monitoring responses.
func (h *Handler) dispatchStubsDistributions(c *echo.Context, operation string) error {
	if err := h.dispatchStubsDistributionTenant(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchStubsMonitoringAndStreaming(c, operation)
}

// dispatchStubsDistributionTenant handles distribution tenant and web ACL operations.
func (h *Handler) dispatchStubsDistributionTenant(c *echo.Context, operation string) error {
	path := c.Request().URL.Path

	switch operation {
	case opCreateDistributionTenant:
		return h.handleCreateDistributionTenant(c)
	case opCreateDistributionWithTags:
		return h.handleCreateDistributionWithTags(c)
	case opUpdateDistributionTenant:
		return h.handleUpdateDistributionTenant(c, extractResourceID(path, "distribution-tenant/"))
	case opDeleteDistributionTenant:
		return h.handleDeleteDistributionTenant(c, extractResourceID(path, "distribution-tenant/"))
	case opGetDistributionTenant:
		return h.handleGetDistributionTenant(c, extractResourceID(path, "distribution-tenant/"))
	case opGetDistributionTenantByDomain:
		return h.handleGetDistributionTenantByDomain(c)
	case opListDistributionTenants:
		return h.handleListDistributionTenants(c)
	case opListDistributionTenantsByCustom:
		return h.handleListDistributionTenantsByCustomization(c)
	case opUpdateDistributionWithStagingConfig:
		return h.handleUpdateDistributionWithStagingConfig(c, extractResourceID(path, "distribution/"))
	case opUpdateDomainAssociation:
		return h.handleUpdateDomainAssociation(c)
	case opVerifyDNSConfiguration:
		return h.handleVerifyDNSConfiguration(c)
	}

	return errNotDispatched
}

// dispatchStubsMonitoringAndStreaming handles monitoring subscription and web ACL disassociation stubs.
func (h *Handler) dispatchStubsMonitoringAndStreaming(c *echo.Context, operation string) error {
	switch operation {
	case opCreateMonitoringSubscription:
		distID := extractMonitoringDistID(c.Request().URL.Path)

		return h.handleCreateMonitoringSubscription(c, distID)
	case opGetMonitoringSubscription:
		distID := extractMonitoringDistID(c.Request().URL.Path)

		return h.handleGetMonitoringSubscription(c, distID)
	case opDeleteMonitoringSubscription:
		distID := extractMonitoringDistID(c.Request().URL.Path)

		return h.handleDeleteMonitoringSubscription(c, distID)
	case opDisassociateDistributionWebACL:
		distID := strings.TrimSuffix(
			strings.TrimPrefix(c.Request().URL.Path, cfPathPrefix+"distribution/"),
			"/disassociate-web-acl",
		)

		return h.handleDisassociateDistributionWebACL(c, distID)
	case opDisassociateDistributionTenantWebACL:
		tenantID := strings.TrimSuffix(
			strings.TrimPrefix(c.Request().URL.Path, cfPathPrefix+"distribution-tenant/"),
			"/disassociate-web-acl",
		)

		return h.handleDisassociateDistributionTenantWebACL(c, tenantID)
	}

	return errNotDispatched
}

// dispatchStubsTrustAndMisc handles trust store, anycast, and connection function stubs.
func (h *Handler) dispatchStubsTrustAndMisc(c *echo.Context, operation string) error {
	if err := h.dispatchStubsTrustAnycast(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchStubsConnectionFunction(c, operation)
}

// dispatchStubsTrustAnycast handles trust store and anycast IP list stubs.
func (h *Handler) dispatchStubsTrustAnycast(c *echo.Context, operation string) error {
	path := c.Request().URL.Path
	switch operation {
	case opCreateTrustStore:
		return h.handleCreateTrustStore(c)
	case opGetTrustStore:
		return h.handleGetTrustStore(c, extractResourceID(path, "trust-store/"))
	case opUpdateTrustStore:
		return h.handleUpdateTrustStore(c, extractResourceID(path, "trust-store/"))
	case opDeleteTrustStore:
		return h.handleDeleteTrustStore(c, extractResourceID(path, "trust-store/"))
	case opListTrustStores:
		return h.handleListTrustStores(c)
	case opGetAnycastIPList:
		return h.handleGetAnycastIPList(c, extractResourceID(path, "anycast-ip-list/"))
	case opUpdateAnycastIPList:
		return h.handleUpdateAnycastIPList(c, extractResourceID(path, "anycast-ip-list/"))
	case opDeleteAnycastIPList:
		return h.handleDeleteAnycastIPList(c, extractResourceID(path, "anycast-ip-list/"))
	case opListAnycastIPLists:
		return h.handleListAnycastIPLists(c)
	}

	return errNotDispatched
}

// dispatchStubsConnectionFunction handles connection function and connection group stubs.
func (h *Handler) dispatchStubsConnectionFunction(c *echo.Context, operation string) error {
	path := c.Request().URL.Path
	switch operation {
	case opDescribeConnectionFunction:
		return h.handleDescribeConnectionFunction(c, extractResourceID(path, "connection-function/"))
	case opGetConnectionFunction:
		return h.handleGetConnectionFunction(c, extractResourceID(path, "connection-function/"))
	case opUpdateConnectionFunction:
		return h.handleUpdateConnectionFunction(c, extractResourceID(path, "connection-function/"))
	case opDeleteConnectionFunction:
		return h.handleDeleteConnectionFunction(c, extractResourceID(path, "connection-function/"))
	case opListConnectionFunctions:
		return h.handleListConnectionFunctions(c)
	case opPublishConnectionFunction:
		return h.handlePublishConnectionFunction(c, extractResourceID(path, "connection-function/"))
	case opTestConnectionFunction:
		return h.handleTestConnectionFunction(c, extractResourceID(path, "connection-function/"))
	case opGetConnectionGroup:
		return h.handleGetConnectionGroup(c, extractResourceID(path, "connection-group/"))
	case opGetConnectionGroupByRoutingEndpoint:
		return h.handleGetConnectionGroupByRoutingEndpoint(
			c,
			c.Request().URL.Query().Get("RoutingEndpoint"),
		)
	}

	return errNotDispatched
}

// dispatchStubsConnectionAndPolicy handles connection group, continuous deployment, resource policy, and misc stubs.
func (h *Handler) dispatchStubsConnectionAndPolicy(c *echo.Context, operation string) error {
	if err := h.dispatchStubsConnectionGroupAndCDP(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchStubsResourcePolicyAndMisc(c, operation)
}

// dispatchStubsConnectionGroupAndCDP handles connection group and continuous deployment policy stubs.
func (h *Handler) dispatchStubsConnectionGroupAndCDP(
	c *echo.Context,
	operation string,
) error {
	path := c.Request().URL.Path

	switch operation {
	case opUpdateConnectionGroup:
		return h.handleUpdateConnectionGroup(c, extractResourceID(path, "connection-group/"))
	case opDeleteConnectionGroup:
		return h.handleDeleteConnectionGroup(c, extractResourceID(path, "connection-group/"))
	case opListConnectionGroups:
		return h.handleListConnectionGroups(c)

	// Continuous deployment policy — promoted to real handlers.

	// Resource policy.
	case opGetResourcePolicy:
		return h.handleGetResourcePolicy(c)
	case opPutResourcePolicy:
		return h.handlePutResourcePolicy(c)
	case opDeleteResourcePolicy:
		return h.handleDeleteResourcePolicy(c)
	}

	return errNotDispatched
}

// dispatchStubsResourcePolicyAndMisc handles distribution list, invalidation, and
// managed-certificate-details routes (the latter now backed by real state, not a stub).
func (h *Handler) dispatchStubsResourcePolicyAndMisc(c *echo.Context, operation string) error {
	if err := h.dispatchStubsDistributionListBy(c, operation); !errors.Is(err, errNotDispatched) {
		return err
	}

	return h.dispatchStubsTenantAndCerts(c, operation)
}

// dispatchStubsDistributionListBy handles the ListDistributionsBy-* operations.
// Identifier sources vary per op (cloudfront@v1.67.4 serializers.go, each
// op's HttpBindings func): most are a "distributionsBy*/{Id}" URI label,
// ListDistributionsByConnectionFunction and ListDistributionsByTrustStore
// carry theirs as a query value with no URI label at all, and
// ListDistributionsByRealtimeLogConfig carries its ARN in the XML body.
func (h *Handler) dispatchStubsDistributionListBy(c *echo.Context, operation string) error {
	path := c.Request().URL.Path

	switch operation {
	case opListDistributionsByCachePolicyID:
		return h.handleListDistributionsByCachePolicyID(c, extractResourceID(path, "distributionsByCachePolicyId/"))
	case opListDistributionsByOriginRequestPol:
		return h.handleListDistributionsByOriginRequestPolicyID(
			c,
			extractResourceID(path, "distributionsByOriginRequestPolicyId/"),
		)
	case opListDistributionsByResponseHeadersPol:
		return h.handleListDistributionsByResponseHeadersPolicyID(
			c,
			extractResourceID(path, "distributionsByResponseHeadersPolicyId/"),
		)
	case opListDistributionsByWebACLID:
		return h.handleListDistributionsByWebACLID(c, extractResourceID(path, "distributionsByWebACLId/"))
	case opListDistributionsByRealtimeLogConfig:
		return h.handleListDistributionsByRealtimeLogConfig(c, decodeListDistributionsByRealtimeLogConfigBody(c))
	case opListDistributionsByKeyGroup:
		return h.handleListDistributionsByKeyGroup(c, extractResourceID(path, "distributionsByKeyGroupId/"))
	case opListDistributionsByVpcOriginID:
		return h.handleListDistributionsByVpcOriginID(c, extractResourceID(path, "distributionsByVpcOriginId/"))
	case opListDistributionsByAnycastIPListID:
		return h.handleListDistributionsByAnycastIPListID(
			c,
			extractResourceID(path, "distributionsByAnycastIpListId/"),
		)
	case opListDistributionsByConnectionFunction:
		return h.handleListDistributionsByConnectionFunction(
			c,
			c.Request().URL.Query().Get("ConnectionFunctionIdentifier"),
		)
	case opListDistributionsByConnectionMode:
		return h.handleListDistributionsByConnectionMode(c, extractResourceID(path, "distributionsByConnectionMode/"))
	case opListDistributionsByTrustStore:
		return h.handleListDistributionsByTrustStore(c, c.Request().URL.Query().Get("TrustStoreIdentifier"))
	case opListDistributionsByOwnedResource:
		return h.handleListDistributionsByOwnedResource(
			c,
			extractResourceID(path, "distributionsByOwnedResource/"),
		)
	case opListConflictingAliases:
		return h.handleListConflictingAliases(c)
	case opListDomainConflicts:
		return h.handleListDomainConflicts(c)
	}

	return errNotDispatched
}

// dispatchStubsTenantAndCerts handles tenant invalidation routes and
// GetManagedCertificateDetails (now backed by real per-tenant state, not a stub).
func (h *Handler) dispatchStubsTenantAndCerts(c *echo.Context, operation string) error {
	path := c.Request().URL.Path

	switch operation {
	case opCreateInvalidationForDistTenant:
		return h.handleCreateInvalidationForTenant(c, extractResourceID(path, "distribution-tenant/"))
	case opGetInvalidationForDistTenant:
		return h.handleGetInvalidationForTenant(c, extractResourceID(path, "distribution-tenant/"))
	case opListInvalidationsForDistTenant:
		return h.handleListInvalidationsForTenant(c, extractResourceID(path, "distribution-tenant/"))
	case opGetManagedCertificateDetails:
		return h.handleGetManagedCertificateDetails(c, extractResourceID(path, "managed-certificate/"))
	default:

		return xmlResp(c, http.StatusNotFound, cfErrorXML("NoSuchOperation", "unknown operation: "+operation))
	}
}

// notFoundCode returns the CloudFront error code for well-known not-found errors.
// The second return value is false when err is not a known not-found error.
func notFoundCode(err error) (string, bool) {
	if code, ok := notFoundCodeCore(err); ok {
		return code, true
	}

	return notFoundCodeExtended(err)
}

// notFoundCodeCore checks core distribution, OAI, and policy not-found errors.
func notFoundCodeCore(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrNotFound):
		return "NoSuchDistribution", true
	case errors.Is(err, ErrOAINotFound):
		return "NoSuchCloudFrontOriginAccessIdentity", true
	case errors.Is(err, ErrCachePolicyNotFound):
		return "NoSuchCachePolicy", true
	case errors.Is(err, ErrAnycastIPListNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrConnectionFunctionNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrConnectionGroupNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrContinuousDeploymentPolicyNotFound):
		return "NoSuchContinuousDeploymentPolicy", true
	case errors.Is(err, ErrInvalidationNotFound):
		return "NoSuchInvalidation", true
	case errors.Is(err, ErrOACNotFound):
		return "NoSuchOriginAccessControl", true
	case errors.Is(err, ErrResponseHeadersPolicyNotFound):
		return "NoSuchResponseHeadersPolicy", true
	case errors.Is(err, ErrFunctionNotFound):
		return "NoSuchFunctionExists", true
	case errors.Is(err, ErrOriginRequestPolicyNotFound):
		return "NoSuchOriginRequestPolicy", true
	}

	return "", false
}

// notFoundCodeExtended checks FLE, public key, key group, realtime log, KVS, and VPC origin errors.
func notFoundCodeExtended(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrFLENotFound):
		return "NoSuchFieldLevelEncryptionConfig", true
	case errors.Is(err, ErrFLEProfileNotFound):
		return "NoSuchFieldLevelEncryptionProfile", true
	case errors.Is(err, ErrPublicKeyNotFound):
		return "NoSuchPublicKey", true
	case errors.Is(err, ErrKeyGroupNotFound):
		return "NoSuchResource", true
	case errors.Is(err, ErrRealtimeLogConfigNotFound):
		return "NoSuchRealtimeLogConfig", true
	case errors.Is(err, ErrKeyValueStoreNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrVpcOriginNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrDistributionTenantNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrStreamingDistributionNotFound):
		return "NoSuchStreamingDistribution", true
	case errors.Is(err, ErrTrustStoreNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrResourcePolicyNotFound):
		return codeEntityNotFound, true
	case errors.Is(err, ErrMonitoringSubscriptionNotFound):
		return "NoSuchMonitoringSubscription", true
	case errors.Is(err, ErrDomainControlValidationResourceNotFound):
		return codeEntityNotFound, true
	}

	return "", false
}

// errCodeMapping pairs a sentinel error with the wire HTTP status + AWS error code to
// emit when handleError sees it in an error chain. Order matters only in that the first
// match wins; since every sentinel here wraps a distinct awserr category value (never
// another entry in this table), no two entries can both match the same error, so in
// practice order is unconstrained.
//
//nolint:gochecknoglobals // package-level lookup table, analogous to EC2's errCodeLookup
var errCodeMapping = []struct {
	err    error
	code   string
	status int
}{
	{ErrDistributionNotDisabled, "DistributionNotDisabled", http.StatusConflict},
	{ErrPublicKeyInUse, "PublicKeyInUse", http.StatusConflict},
	{ErrFLEProfileInUse, "FieldLevelEncryptionProfileInUse", http.StatusConflict},
	{ErrCachePolicyInUse, "CachePolicyInUse", http.StatusConflict},
	{ErrOriginRequestPolicyInUse, "OriginRequestPolicyInUse", http.StatusConflict},
	{ErrResponseHeadersPolicyInUse, "ResponseHeadersPolicyInUse", http.StatusConflict},
	{ErrFunctionInUse, "FunctionInUse", http.StatusConflict},
	{ErrOAIInUse, "CloudFrontOriginAccessIdentityInUse", http.StatusConflict},
	{ErrOAIAlreadyExists, "CloudFrontOriginAccessIdentityAlreadyExists", http.StatusConflict},
	{ErrOACInUse, "OriginAccessControlInUse", http.StatusConflict},
	{ErrKeyGroupInUse, "ResourceInUse", http.StatusConflict},
	{ErrDistributionAlreadyExists, "DistributionAlreadyExists", http.StatusConflict},
	{ErrStreamingDistributionAlreadyExists, "StreamingDistributionAlreadyExists", http.StatusConflict},
	{ErrIllegalDelete, "IllegalDelete", http.StatusBadRequest},
	{ErrIllegalUpdate, "IllegalUpdate", http.StatusBadRequest},
	{ErrCachePolicyAlreadyExists, "CachePolicyAlreadyExists", http.StatusConflict},
	{ErrOriginRequestPolicyAlreadyExists, "OriginRequestPolicyAlreadyExists", http.StatusConflict},
	{ErrResponseHeadersPolicyAlreadyExists, "ResponseHeadersPolicyAlreadyExists", http.StatusConflict},
	{ErrOriginAccessControlAlreadyExists, "OriginAccessControlAlreadyExists", http.StatusConflict},
	{ErrFunctionAlreadyExists, "FunctionAlreadyExists", http.StatusConflict},
	{ErrFLEAlreadyExists, "FieldLevelEncryptionConfigAlreadyExists", http.StatusConflict},
	{ErrFLEProfileAlreadyExists, "FieldLevelEncryptionProfileAlreadyExists", http.StatusConflict},
	{ErrPublicKeyAlreadyExists, "PublicKeyAlreadyExists", http.StatusConflict},
	{ErrKeyGroupAlreadyExists, "KeyGroupAlreadyExists", http.StatusConflict},
	{ErrRealtimeLogConfigAlreadyExists, "RealtimeLogConfigAlreadyExists", http.StatusConflict},
	{ErrAlreadyExists, "EntityAlreadyExists", http.StatusConflict},
	{ErrConnectionGroupAlreadyExists, "EntityAlreadyExists", http.StatusConflict},
	{ErrInvalidTagging, "InvalidTagging", http.StatusBadRequest},
	{ErrStreamingDistributionNotDisabled, "StreamingDistributionNotDisabled", http.StatusConflict},
	{ErrCNAMEAlreadyExists, "CNAMEAlreadyExists", http.StatusConflict},
	{ErrInconsistentQuantities, "InconsistentQuantities", http.StatusBadRequest},
	{ErrValidation, "InvalidArgument", http.StatusBadRequest},
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	if code, ok := notFoundCode(err); ok {
		return xmlResp(c, http.StatusNotFound, cfErrorXML(code, err.Error()))
	}

	for _, m := range errCodeMapping {
		if errors.Is(err, m.err) {
			return xmlResp(c, m.status, cfErrorXML(m.code, err.Error()))
		}
	}

	return xmlResp(
		c,
		http.StatusInternalServerError,
		cfErrorXML("InternalFailure", err.Error()),
	)
}

// --- Distribution handlers ---
