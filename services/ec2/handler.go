package ec2

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	ec2APIVersion = "2016-11-15"
	ec2XMLNS      = "http://ec2.amazonaws.com/doc/2016-11-15/"
	unknownOp     = "Unknown"
	// errCodeInvalidParameterValue is the EC2 "InvalidParameterValue" API error code, shared by
	// several sentinel error mappings below.
	errCodeInvalidParameterValue = "InvalidParameterValue"
	// errCodeInvalidAssociationIDNotFound is the EC2 "InvalidAssociationID.NotFound" API error
	// code, shared by several distinct "association not found" sentinel errors below.
	errCodeInvalidAssociationIDNotFound = "InvalidAssociationID.NotFound"
	ec2PaginationSalt                   = "ec2-opaque-pagination-v1"
)

// Handler is the Echo HTTP handler for EC2 operations.
type Handler struct {
	Backend Backend `json:"backend"`
	ops     map[string]ec2ActionFn
	janitor *Janitor
	// svcCtx is the service-lifetime context derived from the root service
	// context via StartWorker. It is used for detached background work
	// (compute launch/terminate hooks) that must outlive the per-request HTTP
	// context but should still be cancelled at service shutdown.
	// Falls back to context.Background until StartWorker has run.
	svcCtx    context.Context
	AccountID string `json:"accountID,omitempty"`
	Region    string `json:"region,omitempty"`
}

// NewHandler creates a new EC2 handler with the given backend.
// The dispatch table is built once and cached in h.ops.
func NewHandler(backend Backend) *Handler {
	h := &Handler{Backend: backend, svcCtx: context.Background()}
	h.ops = h.buildOps()

	return h
}

// Reset clears all backend resource state and re-caches the dispatch table.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// WithJanitor attaches a background janitor to the handler.
// If the backend is not an *InMemoryBackend, this is a no-op.
// The optional taskTimeout bounds each sweep; 0 means no per-task timeout.
func (h *Handler) WithJanitor(
	interval, terminatedTTL, cancelledSpotTTL time.Duration,
	taskTimeout ...time.Duration,
) *Handler {
	if mem, ok := h.Backend.(*InMemoryBackend); ok {
		j := NewJanitor(mem, interval, terminatedTTL, cancelledSpotTTL)
		if len(taskTimeout) > 0 {
			j.TaskTimeout = taskTimeout[0]
		}

		h.janitor = j
	}

	return h
}

// StartWorker starts the background janitor if configured and captures the
// service-lifetime context for detached compute hook calls.
func (h *Handler) StartWorker(ctx context.Context) error {
	// Capture the root service ctx so detached fire-and-forget compute hooks
	// (launchOnCompute, terminateOnCompute, computeStartOrStop) can run with
	// a context that outlives any individual HTTP request but is cancelled
	// when the service shuts down. Use WithoutCancel-like semantics: just
	// reuse the parent so cancellation propagates from the root.
	h.svcCtx = ctx

	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Shutdown stops background goroutines started by this handler.
// Implements service.Shutdowner. Safe to call multiple times.
func (h *Handler) Shutdown(_ context.Context) {
	if mem, ok := h.Backend.(*InMemoryBackend); ok {
		mem.StopLifecycleReconciler()
	}
}

// Name returns the service name.
func (h *Handler) Name() string {
	return "EC2"
}

// extSupportedOperationsProviders lists every op-family "supported
// operations" function contributing extra (non-core) operation names to
// GetSupportedOperations(). Keeping this as a local slice literal (built
// inside aggregateExtSupportedOperations) rather than one long chain of
// append calls keeps GetSupportedOperations itself short.
func aggregateExtSupportedOperations() []string {
	providers := []func() []string{
		deepDiveSupportedOperations,
		networking1SupportedOperations,
		advancedNetworkingSupportedOperations,
		ipamDiscoverySupportedOperations,
		ipamPolicySupportedOperations,
		ec2CoreSupportedOperations,
		spotFleetSupportedOperations,
		volumesSupportedOperations,
		snapshotsSupportedOperations,
		vpcsSupportedOperations,
		subnetsSupportedOperations,
		securityGroupsSupportedOperations,
		elasticIpsSupportedOperations,
		instancesSupportedOperations,
		networkInterfacesSupportedOperations,
		accountAttrsSupportedOperations,
		vpcEndpointsSupportedOperations,
		natGatewaysSupportedOperations,
		imagesSupportedOperations,
		transitGatewaysSupportedOperations,
		capacityFamilySupportedOperations,
		routeTablesSupportedOperations,
		vpnConnectionsSupportedOperations,
		prefixListsSupportedOperations,
		clientVpnSupportedOperations,
		transitGatewayPeeringSupportedOperations,
		verifiedAccessSupportedOperations,
		networkAclsSupportedOperations,
		launchTemplatesSupportedOperations,
		networkInsightsSupportedOperations,
		flowLogsSupportedOperations,
		localGatewaySupportedOperations,
		tgwMulticastSupportedOperations,
		tgwPeripheralsSupportedOperations,
		vpcConfigSupportedOperations,
		verifiedAccessPolicySupportedOperations,
		fpgaImageSupportedOperations,
		scheduledInstanceSupportedOperations,
		ipPoolSupportedOperations,
		vmImportExportSupportedOperations,
		trunkEnclaveSupportedOperations,
		imageOpsSupportedOperations,
		macHostSupportedOperations,
		secondaryNetSupportedOperations,
		instanceAttrSupportedOperations,
		sqlHaSupportedOperations,
		vpcEncryptionControlSupportedOperations,
		vpnConcentratorSupportedOperations,
		hostReservationSupportedOperations,
		declarativePoliciesSupportedOperations,
		networkPerformanceSupportedOperations,
		managedResourceVisibilitySupportedOperations,
		applicationStatusChecksSupportedOperations,
		stubSupportedOperations,
	}

	// avgOpsPerFamily is a rough preallocation estimate (actual per-family
	// counts vary from 1 to ~30) to avoid repeated slice growth.
	const avgOpsPerFamily = 10

	extOps := make([]string, 0, len(providers)*avgOpsPerFamily)
	for _, supportedOps := range providers {
		extOps = append(extOps, supportedOps()...)
	}

	return extOps
}

// GetSupportedOperations returns the list of supported EC2 operations.
func (h *Handler) GetSupportedOperations() []string {
	return append(coreSupportedOperations(), aggregateExtSupportedOperations()...)
}

// coreSupportedOperations returns the static baseline of operation names
// implemented directly by buildCoreOps, before any op-family extensions are
// appended by GetSupportedOperations.
func coreSupportedOperations() []string {
	return []string{
		"RunInstances",
		"DescribeInstances",
		"TerminateInstances",
		"StartInstances",
		"StopInstances",
		"RebootInstances",
		"DescribeInstanceStatus",
		"DescribeImages",
		"DescribeRegions",
		"DescribeAvailabilityZones",
		"DescribeSecurityGroups",
		"CreateSecurityGroup",
		"DeleteSecurityGroup",
		"AuthorizeSecurityGroupIngress",
		"AuthorizeSecurityGroupEgress",
		"RevokeSecurityGroupIngress",
		"DescribeVpcs",
		"DescribeVpcAttribute",
		"DescribeSubnets",
		"CreateVpc",
		"DeleteVpc",
		"CreateSubnet",
		"DeleteSubnet",
		"CreateKeyPair",
		"DescribeKeyPairs",
		"DeleteKeyPair",
		"ImportKeyPair",
		"CreateVolume",
		"DescribeVolumes",
		"DeleteVolume",
		"AttachVolume",
		"DetachVolume",
		"AllocateAddress",
		"AssociateAddress",
		"DisassociateAddress",
		"ReleaseAddress",
		"DescribeAddresses",
		"CreateInternetGateway",
		"DeleteInternetGateway",
		"DescribeInternetGateways",
		"AttachInternetGateway",
		"DetachInternetGateway",
		"CreateRouteTable",
		"DeleteRouteTable",
		"DescribeRouteTables",
		"CreateRoute",
		"DeleteRoute",
		"AssociateRouteTable",
		"DisassociateRouteTable",
		"CreateNatGateway",
		"DeleteNatGateway",
		"DescribeNatGateways",
		"DescribeNetworkInterfaces",
		"CreateNetworkInterface",
		"DeleteNetworkInterface",
		"AttachNetworkInterface",
		"DetachNetworkInterface",
		"AssignPrivateIpAddresses",
		"UnassignPrivateIpAddresses",
		"ModifyNetworkInterfaceAttribute",
		"RevokeSecurityGroupEgress",
		"DescribeInstanceTypes",
		"DescribeTags",
		"CreateTags",
		"DeleteTags",
		"DescribeInstanceAttribute",
		"ModifyInstanceAttribute",
		"ResetInstanceAttribute",
		"DescribeImageAttribute",
		"DescribeLaunchTemplates",
		"RequestSpotInstances",
		"DescribeSpotInstanceRequests",
		"CancelSpotInstanceRequests",
		"DescribeSpotPriceHistory",
		"CreatePlacementGroup",
		"DescribePlacementGroups",
		"DeletePlacementGroup",
		"DescribeVolumeAttribute",
		"ModifyVolumeAttribute",
		"DescribeSnapshotAttribute",
		"ModifySnapshotAttribute",
		"AcceptAddressTransfer",
		"AcceptCapacityReservationBillingOwnership",
		"AcceptReservedInstancesExchangeQuote",
		"AcceptTransitGatewayMulticastDomainAssociations",
		"AcceptTransitGatewayPeeringAttachment",
		"AcceptTransitGatewayVpcAttachment",
		"AcceptVpcEndpointConnections",
		"AcceptVpcPeeringConnection",
		"AdvertiseByoipCidr",
		"AllocateHosts",
		"DescribeCapacityReservations",
		"DescribeByoipCidrs",
		"DescribeHosts",
		"DescribeVpcPeeringConnections",
		"CreateFleet",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "ec2" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this EC2 instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// RouteMatcher returns a function that matches EC2 requests.
// EC2 requests are form-encoded POSTs containing the EC2 API version.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost {
			return false
		}

		if strings.HasPrefix(r.URL.Path, "/dashboard/") {
			return false
		}

		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/x-www-form-urlencoded") {
			return false
		}

		body, err := httputils.ReadBody(r)
		if err != nil {
			// Body unreadable (e.g. oversized): fall back to the User-Agent
			// marker every aws-sdk-go-v2 ec2 client sets (api_client.go's
			// AddSDKAgentKeyValue -- "api/ec2"). That still identifies this
			// as ours, so claim it and let Handler() produce the typed
			// error instead of masking the read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/ec2")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == ec2APIVersion
	}
}

// MatchPriority returns the routing priority for the EC2 handler.
func (h *Handler) MatchPriority() int {
	return service.PriorityFormStandard
}

// ExtractOperation extracts the EC2 action from the request form.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return unknownOp
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return unknownOp
	}

	action := vals.Get("Action")
	if action == "" {
		return unknownOp
	}

	return action
}

// ExtractResource returns the primary resource identifier from the EC2 request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}

	resourceKeys := []string{
		"InstanceId.1", "GroupId.1", "GroupId",
		"VpcId.1", "VpcId", "SubnetId.1", "SubnetId",
		"ResourceId.1", "ResourceId",
	}

	for _, key := range resourceKeys {
		if v := vals.Get(key); v != "" {
			return v
		}
	}

	return ""
}

// Handler returns the Echo handler function for EC2 requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		reqID := newRequestID()

		r := c.Request()
		body, err := httputils.ReadBody(r)
		if err != nil {
			log.ErrorContext(ctx, "failed to read EC2 request body", "error", err)

			return h.writeError(
				c,
				reqID,
				http.StatusInternalServerError,
				"InternalFailure",
				"failed to read request body",
			)
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return h.writeError(
				c,
				reqID,
				http.StatusBadRequest,
				errCodeInvalidParameterValue,
				"failed to parse request body",
			)
		}

		action := vals.Get("Action")
		if action == "" {
			return h.writeError(
				c,
				reqID,
				http.StatusBadRequest,
				"MissingAction",
				"missing Action parameter",
			)
		}

		log.DebugContext(ctx, "EC2 request", "action", action)

		resp, opErr := h.dispatch(action, vals, reqID)
		if opErr != nil {
			return h.handleOpError(c, reqID, action, opErr)
		}

		xmlBytes, marshalErr := marshalXML(resp)
		if marshalErr != nil {
			log.ErrorContext(
				ctx,
				"failed to marshal EC2 response",
				"action",
				action,
				"error",
				marshalErr,
			)

			return h.writeError(
				c,
				reqID,
				http.StatusInternalServerError,
				"InternalFailure",
				"internal server error",
			)
		}

		return c.Blob(http.StatusOK, "text/xml", xmlBytes)
	}
}

type ec2ActionFn func(vals url.Values, reqID string) (any, error)

// opRegistrars lists every op-family registration function, in the exact
// order buildOps must call them: several later entries (registerInstanceAttrOps,
// registerAdvancedNetworkingOps, registerSpotFleetOps) intentionally override
// stub entries registered earlier, so this order must be preserved.
func (h *Handler) opRegistrars() []func(*Handler, map[string]ec2ActionFn) {
	return []func(*Handler, map[string]ec2ActionFn){
		registerDeepDiveOps,
		registerAcceptAndAdvancedOps,
		registerNetworking1Ops,
		registerEC2CoreOps,
		registerVolumesOps,
		registerSnapshotsOps,
		registerVpcsOps,
		registerSubnetsOps,
		registerSecurityGroupsOps,
		registerElasticIpsOps,
		registerInstancesOps,
		registerNetworkInterfacesOps,
		registerAccountAttrsOps,
		registerVpcEndpointsOps,
		registerNatGatewaysOps,
		registerImagesOps,
		registerTransitGatewaysOps,
		registerRouteTablesOps,
		registerVpnConnectionsOps,
		registerPrefixListsOps,
		registerClientVpnOps,
		registerTransitGatewayPeeringOps,
		registerVerifiedAccessOps,
		registerNetworkAclsOps,
		registerLaunchTemplatesOps,
		registerNetworkInsightsOps,
		registerFlowLogsOps,
		registerByoipOps,
		registerCarrierGatewaysOps,
		registerFleetOps,
		registerReservedInstancesOps,
		registerTrafficMirrorOps,
		registerRouteServerOps,
		registerLocalGatewayOps,
		registerTGWMulticastOps,
		registerTGWPeripheralsOps,
		registerVpcConfigOps,
		registerCapacityFamilyOps,
		registerVerifiedAccessPolicyOps,
		registerFpgaImageOps,
		registerScheduledInstanceOps,
		registerIPPoolOps,
		registerVMImportExportOps,
		registerTrunkEnclaveOps,
		registerImageOpsHandlers,
		registerMacHostOps,
		registerSecondaryNetOps,
		// registerInstanceAttrOps supplies the real implementations for the instance-modify
		// and event-window-association operations that origin/parity-sweep-2's now-removed
		// handler_audit.go duplicated (ModifyInstancePlacement, ModifyInstanceCpuOptions,
		// ModifyInstanceMaintenanceOptions, ModifyInstanceNetworkPerformanceOptions,
		// AssociateInstanceEventWindow) plus several more; see handler_instance_attrs.go.
		registerInstanceAttrOps,
		registerSQLHaOps,
		registerVpcEncryptionControlOps,
		registerVpnConcentratorOps,
		registerHostReservationOps,
		registerDeclarativePoliciesOps,
		registerNetworkPerformanceOps,
		registerManagedResourceVisibilityOps,
		registerApplicationStatusChecksOps,
		// registerAdvancedNetworkingOps must run last to override stub entries.
		registerAdvancedNetworkingOps,
		registerIpamDiscoveryOps,
		registerIpamPolicyOps,
		// registerSpotFleetOps overrides stub spot fleet handlers with real implementations.
		registerSpotFleetOps,
	}
}

func (h *Handler) buildOps() map[string]ec2ActionFn {
	ops := h.buildCoreOps()

	for _, register := range h.opRegistrars() {
		register(h, ops)
	}

	return ops
}

func (h *Handler) buildCoreOps() map[string]ec2ActionFn {
	return map[string]ec2ActionFn{
		"RunInstances":                    h.handleRunInstances,
		"DescribeInstances":               h.handleDescribeInstances,
		"TerminateInstances":              h.handleTerminateInstances,
		"DescribeSecurityGroups":          h.handleDescribeSecurityGroups,
		"CreateSecurityGroup":             h.handleCreateSecurityGroup,
		"DeleteSecurityGroup":             h.handleDeleteSecurityGroup,
		"RevokeSecurityGroupEgress":       h.handleRevokeSecurityGroupEgress,
		"DescribeVpcs":                    h.handleDescribeVpcs,
		"DescribeVpcAttribute":            h.handleDescribeVpcAttribute,
		"DescribeSubnets":                 h.handleDescribeSubnets,
		"CreateVpc":                       h.handleCreateVpc,
		"DeleteVpc":                       h.handleDeleteVpc,
		"CreateSubnet":                    h.handleCreateSubnet,
		"DeleteSubnet":                    h.handleDeleteSubnet,
		"DescribeInstanceTypes":           h.handleDescribeInstanceTypes,
		"DescribeTags":                    h.handleDescribeTags,
		"CreateTags":                      h.handleCreateTags,
		"DeleteTags":                      h.handleDeleteTags,
		"DescribeInstanceAttribute":       h.handleDescribeInstanceAttribute,
		"ModifyInstanceAttribute":         h.handleModifyInstanceAttribute,
		"ResetInstanceAttribute":          h.handleResetInstanceAttribute,
		"StartInstances":                  h.handleStartInstances,
		"StopInstances":                   h.handleStopInstances,
		"RebootInstances":                 h.handleRebootInstances,
		"DescribeInstanceStatus":          h.handleDescribeInstanceStatus,
		"DescribeImages":                  h.handleDescribeImages,
		"DescribeRegions":                 h.handleDescribeRegions,
		"DescribeAvailabilityZones":       h.handleDescribeAvailabilityZones,
		"CreateKeyPair":                   h.handleCreateKeyPair,
		"DescribeKeyPairs":                h.handleDescribeKeyPairs,
		"DeleteKeyPair":                   h.handleDeleteKeyPair,
		"ImportKeyPair":                   h.handleImportKeyPair,
		"CreateVolume":                    h.handleCreateVolume,
		"DescribeVolumes":                 h.handleDescribeVolumes,
		"DeleteVolume":                    h.handleDeleteVolume,
		"AttachVolume":                    h.handleAttachVolume,
		"DetachVolume":                    h.handleDetachVolume,
		"DescribeVolumeAttribute":         h.handleDescribeVolumeAttribute,
		"ModifyVolumeAttribute":           h.handleModifyVolumeAttribute,
		"DescribeSnapshotAttribute":       h.handleDescribeSnapshotAttribute,
		"ModifySnapshotAttribute":         h.handleModifySnapshotAttribute,
		"AllocateAddress":                 h.handleAllocateAddress,
		"AssociateAddress":                h.handleAssociateAddress,
		"DisassociateAddress":             h.handleDisassociateAddress,
		"ReleaseAddress":                  h.handleReleaseAddress,
		"DescribeAddresses":               h.handleDescribeAddresses,
		"CreateInternetGateway":           h.handleCreateInternetGateway,
		"DeleteInternetGateway":           h.handleDeleteInternetGateway,
		"DescribeInternetGateways":        h.handleDescribeInternetGateways,
		"AttachInternetGateway":           h.handleAttachInternetGateway,
		"DetachInternetGateway":           h.handleDetachInternetGateway,
		"CreateRouteTable":                h.handleCreateRouteTable,
		"DeleteRouteTable":                h.handleDeleteRouteTable,
		"DescribeRouteTables":             h.handleDescribeRouteTables,
		"CreateRoute":                     h.handleCreateRoute,
		"DeleteRoute":                     h.handleDeleteRoute,
		"AssociateRouteTable":             h.handleAssociateRouteTable,
		"DisassociateRouteTable":          h.handleDisassociateRouteTable,
		"CreateNatGateway":                h.handleCreateNatGateway,
		"DeleteNatGateway":                h.handleDeleteNatGateway,
		"DescribeNatGateways":             h.handleDescribeNatGateways,
		"DescribeNetworkInterfaces":       h.handleDescribeNetworkInterfaces,
		"CreateNetworkInterface":          h.handleCreateNetworkInterface,
		"DeleteNetworkInterface":          h.handleDeleteNetworkInterface,
		"AttachNetworkInterface":          h.handleAttachNetworkInterface,
		"DetachNetworkInterface":          h.handleDetachNetworkInterface,
		"AssignPrivateIpAddresses":        h.handleAssignPrivateIPAddresses,
		"UnassignPrivateIpAddresses":      h.handleUnassignPrivateIPAddresses,
		"ModifyNetworkInterfaceAttribute": h.handleModifyNetworkInterfaceAttribute,
		"AuthorizeSecurityGroupIngress":   h.handleAuthorizeSecurityGroupIngress,
		"AuthorizeSecurityGroupEgress":    h.handleAuthorizeSecurityGroupEgress,
		"RevokeSecurityGroupIngress":      h.handleRevokeSecurityGroupIngress,
		"DescribeImageAttribute":          h.handleDescribeImageAttribute,
		"DescribeLaunchTemplates":         h.handleDescribeLaunchTemplates,
		"RequestSpotInstances":            h.handleRequestSpotInstances,
		"DescribeSpotInstanceRequests":    h.handleDescribeSpotInstanceRequests,
		"CancelSpotInstanceRequests":      h.handleCancelSpotInstanceRequests,
		"DescribeSpotPriceHistory":        h.handleDescribeSpotPriceHistory,
		"CreatePlacementGroup":            h.handleCreatePlacementGroup,
		"DescribePlacementGroups":         h.handleDescribePlacementGroups,
		"DeletePlacementGroup":            h.handleDeletePlacementGroup,
		"DescribeVpcPeeringConnections":   h.handleDescribeVpcPeeringConnections,
	}
}

// dispatch routes the EC2 action to the appropriate handler function.
// If DryRun=true is present in vals, the request is validated and then
// rejected with ErrDryRunOperation (HTTP 412) as real AWS does.
func (h *Handler) dispatch(action string, vals url.Values, reqID string) (any, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s is not a supported EC2 action", ErrInvalidParameter, action)
	}

	if vals.Get("DryRun") == ec2BooleanTrue {
		return nil, ErrDryRunOperation
	}

	return fn(vals, reqID)
}

// ---- error handling ----

// errCodeLookup maps sentinel errors to their EC2 API error codes.
//
//nolint:gochecknoglobals // package-level mapping, analogous to a lookup table
var errCodeLookup = []struct {
	err  error
	code string
}{
	{ErrInstanceNotFound, "InvalidInstanceID.NotFound"},
	{ErrSecurityGroupNotFound, "InvalidGroup.NotFound"},
	{ErrVPCNotFound, "InvalidVpcID.NotFound"},
	{ErrSubnetNotFound, "InvalidSubnetID.NotFound"},
	{ErrDuplicateSGName, "InvalidGroup.Duplicate"},
	{ErrKeyPairNotFound, "InvalidKeyPair.NotFound"},
	{ErrDuplicateKeyPairName, "InvalidKeyPair.Duplicate"},
	{ErrVolumeNotFound, "InvalidVolume.NotFound"},
	{ErrVolumeInUse, "VolumeInUse"},
	{ErrAddressNotFound, "InvalidAllocationID.NotFound"},
	{ErrInternetGatewayNotFound, "InvalidInternetGatewayID.NotFound"},
	{ErrRouteTableNotFound, "InvalidRouteTableID.NotFound"},
	{ErrNatGatewayNotFound, "InvalidNatGatewayID.NotFound"},
	{ErrRouteNotFound, "InvalidRoute.NotFound"},
	{ErrAssociationNotFound, errCodeInvalidAssociationIDNotFound},
	{ErrNetworkInterfaceNotFound, "InvalidNetworkInterfaceID.NotFound"},
	{ErrNetworkInterfaceInUse, "InvalidNetworkInterfaceID.InUse"},
	{ErrNetworkInterfacePermissionNotFound, "InvalidPermission.NotFound"},
	{ErrAttachmentNotFound, "InvalidAttachmentID.NotFound"},
	{ErrSpotRequestNotFound, "InvalidSpotInstanceRequestID.NotFound"},
	{ErrPlacementGroupNotFound, "InvalidPlacementGroup.NotFound"},
	{ErrDuplicatePlacementGroupName, "InvalidPlacementGroup.Duplicate"},
	{ErrInvalidInstanceState, "IncorrectInstanceState"},
	{ErrAddressTransferNotFound, "InvalidAddressTransfer.NotFound"},
	{ErrCapacityReservationNotFound, "InvalidCapacityReservationId.NotFound"},
	{ErrReservedInstancesNotFound, "InvalidReservedInstancesId.NotFound"},
	{ErrTransitGatewayAttachmentNotFound, "InvalidTransitGatewayAttachmentID.NotFound"},
	{ErrVpcPeeringConnectionNotFound, "InvalidVpcPeeringConnectionID.NotFound"},
	{ErrVpcEndpointNotFound, "InvalidVpcEndpointService.NotFound"},
	{ErrByoipCidrNotFound, "InvalidByoipCidr.NotFound"},
	{ErrHostNotFound, "InvalidHostID.NotFound"},
	{ErrInstanceEventWindowNotFound, "InvalidInstanceEventWindowId.NotFound"},
	{ErrCIDRConflict, "InvalidVpc.Conflict"},
	{ErrClientVpnEndpointNotFound, "InvalidClientVpnEndpointId.NotFound"},
	{ErrTrafficMirrorFilterNotFound, "InvalidTrafficMirrorFilterId.NotFound"},
	{ErrTrafficMirrorFilterRuleNotFound, "InvalidTrafficMirrorFilterRuleId.NotFound"},
	{ErrTrafficMirrorSessionNotFound, "InvalidTrafficMirrorSessionId.NotFound"},
	{ErrTrafficMirrorTargetNotFound, "InvalidTrafficMirrorTargetId.NotFound"},
	{ErrVpnConnectionNotFound, "InvalidVpnConnectionID.NotFound"},
	{ErrVpnGatewayNotFound, "InvalidVpnGatewayID.NotFound"},
	{ErrCustomerGatewayNotFound, "InvalidCustomerGatewayID.NotFound"},
	{ErrVpnTunnelNotFound, errCodeInvalidParameterValue},
	{ErrVpcEndpointServiceNotFound, "InvalidVpcEndpointService.NotFound"},
	{ErrDependencyViolation, "DependencyViolation"},
	{ErrAddressInUse, "InvalidIPAddress.InUse"},
	{ErrResourceAlreadyAssociated, "Resource.AlreadyAssociated"},
	{ErrVpcClassicLinkDisabled, "VpcClassicLinkDisabled"},
	{ErrClassicLinkInstanceNotFound, "InvalidInstanceID.NotFound"},
	{ErrVpcBlockPublicAccessExclusionNotFound, "InvalidVpcBlockPublicAccessExclusionId.NotFound"},
	{ErrIpamPolicyNotFound, "InvalidIpamPolicyId.NotFound"},
	{ErrIpamAllocationNotFound, "InvalidIpamPoolAllocationId.NotFound"},
	{ErrVpcEndpointIDNotFound, "InvalidVpcEndpointId.NotFound"},
	{ErrIpamOrgAdminAccountNotFound, errCodeInvalidParameterValue},
	{ErrTGWPolicyTableNotFound, "InvalidTransitGatewayPolicyTableId.NotFound"},
	{ErrTGWRouteTableAnnouncementNotFound, "InvalidTransitGatewayRouteTableAnnouncementId.NotFound"},
	{ErrTransitGatewayNotFound, "InvalidTransitGatewayID.NotFound"},
	{ErrTGWRouteTableNotFound, "InvalidTransitGatewayRouteTableId.NotFound"},
	{ErrTGWMeteringPolicyNotFound, "InvalidTransitGatewayMeteringPolicyId.NotFound"},
	{ErrTGWAttachmentNotFound, "InvalidTransitGatewayAttachmentID.NotFound"},
	{ErrTGWPrefixListRefNotFound, "InvalidTransitGatewayPrefixListReferenceId.NotFound"},
	{ErrVerifiedAccessEndpointNotFound, "InvalidVerifiedAccessEndpointId.NotFound"},
	{ErrVerifiedAccessGroupNotFound, "InvalidVerifiedAccessGroupId.NotFound"},
	{ErrVerifiedAccessInstanceNotFound, "InvalidVerifiedAccessInstanceId.NotFound"},
	{ErrVerifiedAccessTrustProviderNF, "InvalidVerifiedAccessTrustProviderId.NotFound"},
	{ErrFpgaImageNotFound, "InvalidFpgaImageID.NotFound"},
	{ErrScheduledInstanceNotFound, "InvalidScheduledInstance.NotFound"},
	{ErrScheduledInstancePurchaseToken, errCodeInvalidParameterValue},
	{ErrCoipPoolNotFound, "InvalidPoolID.NotFound"},
	{ErrCoipCidrNotFound, errCodeInvalidParameterValue},
	{ErrIpv4PoolNotFound, "InvalidPublicIpv4Pool.NotFound"},
	{ErrIpv4PoolCidrNotFound, errCodeInvalidParameterValue},
	{ErrIpv6PoolNotFound, errCodeInvalidParameterValue},
	{ErrImageNotFound, "InvalidAMIID.NotFound"},
	{ErrUsageReportNotFound, errCodeInvalidParameterValue},
	{ErrBundleTaskNotFound, "InvalidBundleID.NotFound"},
	{ErrConversionTaskNotFound, "InvalidConversionTaskId.NotFound"},
	{ErrExportTaskNotFound, "InvalidExportTaskID.NotFound"},
	{ErrImportTaskNotFound, errCodeInvalidParameterValue},
	{ErrTaskNotCancellable, "IncorrectState"},
	{ErrTrunkAssociationNotFound, errCodeInvalidAssociationIDNotFound},
	{ErrEnclaveCertRoleAssociationNotFound, errCodeInvalidParameterValue},
	{ErrTooManyEnclaveCertRoles, "LimitExceeded"},
	{ErrMacInstanceRequired, errCodeInvalidParameterValue},
	{ErrSecondaryNetworkNotFound, "InvalidSecondaryNetworkID.NotFound"},
	{ErrSecondarySubnetNotFound, "InvalidSecondarySubnetID.NotFound"},
	{ErrSecondaryNetworkHasSubnets, "DependencyViolation"},
	{ErrInstanceEventWindowNotFound, "InvalidInstanceEventWindowId.NotFound"},
	{ErrCapacityReservationFull, "CapacityReservationFull"},
	{ErrFlowLogNotFound, "InvalidFlowLogId.NotFound"},
	{ErrTGWPropagationNotFound, "InvalidTransitGatewayRouteTablePropagation.NotFound"},
	{ErrInterruptibleAllocationNotFound, "InvalidCapacityReservationId.NotFound"},
	{ErrPublicIPNotFound, "InvalidAddress.NotFound"},
	{ErrInvalidParameter, errCodeInvalidParameterValue},
	{ErrInvalidUserData, "InvalidUserData.Malformed"},
	{ErrMissingParameter, "MissingParameter"},
	{ErrInvalidPaginationToken, "InvalidPaginationToken"},
	{ErrOperationNotPermitted, "OperationNotPermitted"},
	{ErrApplicationStatusCheckNotFound, "InvalidApplicationStatusCheckId.NotFound"},
	{ErrInvalidParameterCombination, "InvalidParameterCombination"},
	{ErrTooManyApplicationStatusChecks, "ApplicationStatusCheckLimitExceeded"},
	{ErrOutpostArnNotFound, errCodeInvalidParameterValue},
	{ErrInsufficientInstanceCapacity, "InsufficientInstanceCapacity"},
	{ErrResourceCountExceeded, "ResourceCountExceeded"},
	{ErrIAMInstanceProfileAlreadyAssociated, "IncorrectState"},
	{ErrIAMAssociationNotFound, errCodeInvalidAssociationIDNotFound},
}

// opErrCode resolves an error to its EC2 API error code and HTTP status code.
func opErrCode(opErr error) (string, int) {
	if errors.Is(opErr, ErrDryRunOperation) {
		return "DryRunOperation", http.StatusPreconditionFailed
	}

	for _, entry := range errCodeLookup {
		if errors.Is(opErr, entry.err) {
			return entry.code, http.StatusBadRequest
		}
	}

	return "InternalFailure", http.StatusInternalServerError
}

func (h *Handler) handleOpError(c *echo.Context, reqID, action string, opErr error) error {
	code, statusCode := opErrCode(opErr)

	if statusCode == http.StatusInternalServerError {
		logger.Load(c.Request().Context()).
			Error("EC2 internal error", "error", opErr, "action", action)
	}

	return h.writeError(c, reqID, statusCode, code, opErr.Error())
}

func (h *Handler) writeError(
	c *echo.Context,
	reqID string,
	statusCode int,
	code, message string,
) error {
	errResp := &ec2ErrorResponse{
		XMLName:   xml.Name{Local: "Response"},
		Errors:    ec2ErrorsWrapper{Error: ec2Error{Code: code, Message: message}},
		RequestID: reqID,
	}

	xmlBytes, err := marshalXML(errResp)
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	return c.Blob(statusCode, "text/xml", xmlBytes)
}

// ---- helpers ----

// parseMemberList extracts ordered list parameters like "InstanceId.1", "InstanceId.2", ...
func parseMemberList(vals url.Values, prefix string) []string {
	var result []string

	for i := 1; ; i++ {
		v := vals.Get(fmt.Sprintf("%s.%d", prefix, i))
		if v == "" {
			return result
		}
		result = append(result, v)
	}
}

// Per-operation MaxResults bounds for the ec2sweep11 pagination fixes below.
// Bounds/defaults come from the pinned SDK's doc comments
// (aws-sdk-go-v2/service/ec2@v1.319.1); ops whose docs give no explicit range
// fall back to DescribeImages' bounds (api_op_DescribeImages.go: 1..1000).
const (
	ec2PageMinDefault = 1
	ec2PageMaxDefault = 1000

	ec2PageDefaultInstanceTopology = 20 // api_op_DescribeInstanceTopology.go: "Default: 20"
	ec2PageMinEventWindows         = 20 // api_op_DescribeInstanceEventWindows.go: "between 20 and 500"
	ec2PageMaxEventWindows         = 500
	ec2PageMinElasticGpus          = 5 // api_op_DescribeElasticGpus.go: "between 5 and 1000"

	ec2PageMinRecycleBin = 5 // api_op_ListVolumesInRecycleBin.go: "Valid range: 5 - 500"
	ec2PageMaxRecycleBin = 500

	ec2PageMinSecurityGroupRules = 5 // api_op_DescribeSecurityGroupRules.go: "between 5 and 1000"
	ec2PageMinSecurityGroups     = 5 // api_op_DescribeSecurityGroups.go: "between 5 and 1000"
	ec2PageMinMovingAddresses    = 5 // api_op_DescribeMovingAddresses.go: "between 5 and 1000"

	ec2PageMaxVolumesModifications = 500 // api_op_DescribeVolumesModifications.go: "up to a limit of 500"

	// Per-operation MaxResults bounds for the reqfielddiff never-declared-field
	// sweep below: these six ops accepted MaxResults/NextToken on the wire but
	// the handler read neither, always returning every item in one page.
	ec2PageDefaultNIPermissions = 50 // api_op_DescribeNetworkInterfacePermissions.go: "50 results ... by default"

	ec2PageMaxReservedInstancesOfferings = 100 // api_op_DescribeReservedInstancesOfferings.go: "maximum is 100"

	ec2PageMinScheduledInstanceAvailability = 5   // api_op_DescribeScheduledInstanceAvailability.go: "between 5 and 300"
	ec2PageMaxScheduledInstanceAvailability = 300 // same: "The default value is 300"

	ec2PageMinScheduledInstances     = 5   // api_op_DescribeScheduledInstances.go: "between 5 and 300"
	ec2PageMaxScheduledInstances     = 300 // same
	ec2PageDefaultScheduledInstances = 100 // same: "The default value is 100"

	ec2PageMinVpnConnectionDeviceTypes = 200  // api_op_GetVpnConnectionDeviceTypes.go: "between 200 and 1000"
	ec2PageMaxVpnConnectionDeviceTypes = 1000 // same
)

// parseEC2Pagination validates MaxResults against [minResults, maxResults]
// (defaultResults when the caller omits it) and decodes NextToken into a byte
// offset, generalizing handleDescribeImages' parseImagesPagination with
// per-operation bounds.
func parseEC2Pagination(vals url.Values, minResults, maxResults, defaultResults int) (int, int, error) {
	limit := defaultResults
	if v := vals.Get("MaxResults"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < minResults || n > maxResults {
			return 0, 0, fmt.Errorf(
				"%w: MaxResults must be between %d and %d",
				ErrInvalidParameter, minResults, maxResults,
			)
		}

		limit = n
	}

	offset := 0
	if tok := vals.Get("NextToken"); tok != "" {
		n := page.DecodeHMACToken(tok, ec2PaginationSalt)
		if n == 0 {
			return 0, 0, fmt.Errorf("%w: the pagination token is not valid", ErrInvalidPaginationToken)
		}

		offset = n
	}

	return limit, offset, nil
}

// pageSlice truncates items[offset:] to at most limit entries, returning the
// page and the HMAC NextToken for the remainder (empty when exhausted).
func pageSlice[T any](items []T, offset, limit int) ([]T, string) {
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]

	var next string
	if len(items) > limit {
		next = page.EncodeHMACToken(offset+limit, ec2PaginationSalt)
		items = items[:limit]
	}

	return items, next
}

// maxTagsPerRequest is the maximum number of tags accepted in a single EC2 request.
// AWS allows up to 50 tags per resource; we use 1000 as a generous but bounded limit.
const maxTagsPerRequest = 1000

// maxFiltersPerRequest is the maximum number of filters accepted in a single EC2 DescribeTags request.
const maxFiltersPerRequest = 100

// parseEC2Tags extracts Tag.N.Key / Tag.N.Value from EC2 form values.
func parseEC2Tags(vals url.Values) map[string]string {
	tags := make(map[string]string)

	for i := 1; i <= maxTagsPerRequest; i++ {
		key := vals.Get(fmt.Sprintf("Tag.%d.Key", i))
		if key == "" {
			return tags
		}

		tags[key] = vals.Get(fmt.Sprintf("Tag.%d.Value", i))
	}

	return tags
}

// parseEC2TagKeys extracts Tag.N.Key from EC2 DeleteTags form values.
func parseEC2TagKeys(vals url.Values) []string {
	var keys []string

	for i := 1; i <= maxTagsPerRequest; i++ {
		key := vals.Get(fmt.Sprintf("Tag.%d.Key", i))
		if key == "" {
			return keys
		}

		keys = append(keys, key)
	}

	return keys
}

// parseTagSpecification extracts tags from TagSpecification.N.Tag.M.Key/Value form values
// for a specific resourceType (e.g. resourceTypeVPC, "subnet", "instance", "security-group").
// Terraform and the AWS SDK send inline tags this way during resource creation.
// Returns a map of tag keys to values for the matched resource type, or an empty map if none found.
func parseTagSpecification(vals url.Values, resourceType string) map[string]string {
	tags := make(map[string]string)

	for i := 1; i <= maxTagsPerRequest; i++ {
		rt := vals.Get(fmt.Sprintf("TagSpecification.%d.ResourceType", i))
		if rt == "" {
			break
		}

		if rt != resourceType {
			continue
		}

		for j := 1; j <= maxTagsPerRequest; j++ {
			key := vals.Get(fmt.Sprintf("TagSpecification.%d.Tag.%d.Key", i, j))
			if key == "" {
				break
			}

			tags[key] = vals.Get(fmt.Sprintf("TagSpecification.%d.Tag.%d.Value", i, j))
		}
	}

	return tags
}

// marshalXML encodes the payload with the XML declaration header.
func marshalXML(v any) ([]byte, error) {
	raw, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), raw...), nil
}

// newRequestID generates a unique request ID.
func newRequestID() string {
	return "gopherstack-ec2-" + uuid.New().String()
}

// ---- XML response types ----

type ec2Error struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

type ec2ErrorsWrapper struct {
	Error ec2Error `xml:"Error"`
}

type ec2ErrorResponse struct {
	XMLName   xml.Name         `xml:"Response"`
	Errors    ec2ErrorsWrapper `xml:"Errors"`
	RequestID string           `xml:"RequestID"`
}
