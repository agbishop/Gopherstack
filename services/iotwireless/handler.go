package iotwireless

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	opAssociateAwsAccountWithPartnerAccount     = "AssociateAwsAccountWithPartnerAccount"
	opAssociateMulticastGroupWithFuotaTask      = "AssociateMulticastGroupWithFuotaTask"
	opAssociateWirelessDeviceWithFuotaTask      = "AssociateWirelessDeviceWithFuotaTask"
	opAssociateWirelessDeviceWithMulticastGroup = "AssociateWirelessDeviceWithMulticastGroup"
	opAssociateWirelessDeviceWithThing          = "AssociateWirelessDeviceWithThing"
	opAssociateWirelessGatewayWithCertificate   = "AssociateWirelessGatewayWithCertificate"
	opAssociateWirelessGatewayWithThing         = "AssociateWirelessGatewayWithThing"
	opCancelMulticastGroupSession               = "CancelMulticastGroupSession"
	opListTagsForResource                       = "ListTagsForResource"
	opTagResource                               = "TagResource"
	opUntagResource                             = "UntagResource"
)

// Additional operation name constants for the extended operation set.
const (
	opCreateMulticastGroup                                  = "CreateMulticastGroup"
	opGetMulticastGroup                                     = "GetMulticastGroup"
	opListMulticastGroups                                   = "ListMulticastGroups"
	opDeleteMulticastGroup                                  = "DeleteMulticastGroup"
	opUpdateMulticastGroup                                  = "UpdateMulticastGroup"
	opGetMulticastGroupSession                              = "GetMulticastGroupSession"
	opStartMulticastGroupSession                            = "StartMulticastGroupSession"
	opListMulticastGroupsByFuotaTask                        = "ListMulticastGroupsByFuotaTask"
	opSendDataToMulticastGroup                              = "SendDataToMulticastGroup"
	opStartBulkAssociateWirelessDeviceWithMulticastGroup    = "StartBulkAssociateWirelessDeviceWithMulticastGroup"
	opStartBulkDisassociateWirelessDeviceFromMulticastGroup = "StartBulkDisassociateWirelessDeviceFromMulticastGroup"
	opDisassociateWirelessDeviceFromMulticastGroup          = "DisassociateWirelessDeviceFromMulticastGroup"
	opDisassociateMulticastGroupFromFuotaTask               = "DisassociateMulticastGroupFromFuotaTask"
	opDisassociateWirelessDeviceFromFuotaTask               = "DisassociateWirelessDeviceFromFuotaTask"
	opUpdateFuotaTask                                       = "UpdateFuotaTask"
	opStartFuotaTask                                        = "StartFuotaTask"
	opCreateNetworkAnalyzerConfiguration                    = "CreateNetworkAnalyzerConfiguration"
	opGetNetworkAnalyzerConfiguration                       = "GetNetworkAnalyzerConfiguration"
	opListNetworkAnalyzerConfigurations                     = "ListNetworkAnalyzerConfigurations"
	opDeleteNetworkAnalyzerConfiguration                    = "DeleteNetworkAnalyzerConfiguration"
	opUpdateNetworkAnalyzerConfiguration                    = "UpdateNetworkAnalyzerConfiguration"
	opGetEventConfigurationByResourceTypes                  = "GetEventConfigurationByResourceTypes"
	opUpdateEventConfigurationByResourceTypes               = "UpdateEventConfigurationByResourceTypes"
	opListEventConfigurations                               = "ListEventConfigurations"
	opGetResourceEventConfiguration                         = "GetResourceEventConfiguration"
	opUpdateResourceEventConfiguration                      = "UpdateResourceEventConfiguration"
	opGetLogLevelsByResourceTypes                           = "GetLogLevelsByResourceTypes"
	opUpdateLogLevelsByResourceTypes                        = "UpdateLogLevelsByResourceTypes"
	opResetAllResourceLogLevels                             = "ResetAllResourceLogLevels"
	opGetResourceLogLevel                                   = "GetResourceLogLevel"
	opPutResourceLogLevel                                   = "PutResourceLogLevel"
	opResetResourceLogLevel                                 = "ResetResourceLogLevel"
	opGetMetricConfiguration                                = "GetMetricConfiguration"
	opUpdateMetricConfiguration                             = "UpdateMetricConfiguration"
	opGetMetrics                                            = "GetMetrics"
	opGetPosition                                           = "GetPosition"
	opUpdatePosition                                        = "UpdatePosition"
	opGetPositionConfiguration                              = "GetPositionConfiguration"
	opPutPositionConfiguration                              = "PutPositionConfiguration"
	opListPositionConfigurations                            = "ListPositionConfigurations"
	opGetPositionEstimate                                   = "GetPositionEstimate"
	opGetResourcePosition                                   = "GetResourcePosition"
	opUpdateResourcePosition                                = "UpdateResourcePosition"
	opCreateWirelessGatewayTask                             = "CreateWirelessGatewayTask"
	opGetWirelessGatewayTask                                = "GetWirelessGatewayTask"
	opDeleteWirelessGatewayTask                             = "DeleteWirelessGatewayTask"
	opCreateWirelessGatewayTaskDefinition                   = "CreateWirelessGatewayTaskDefinition"
	opGetWirelessGatewayTaskDefinition                      = "GetWirelessGatewayTaskDefinition"
	opListWirelessGatewayTaskDefinitions                    = "ListWirelessGatewayTaskDefinitions"
	opDeleteWirelessGatewayTaskDefinition                   = "DeleteWirelessGatewayTaskDefinition"
	opGetWirelessGatewayCertificate                         = "GetWirelessGatewayCertificate"
	opGetWirelessGatewayFirmwareInformation                 = "GetWirelessGatewayFirmwareInformation"
	opGetWirelessGatewayStatistics                          = "GetWirelessGatewayStatistics"
	opDisassociateWirelessGatewayFromCertificate            = "DisassociateWirelessGatewayFromCertificate"
	opDisassociateWirelessGatewayFromThing                  = "DisassociateWirelessGatewayFromThing"
	opUpdateWirelessGateway                                 = "UpdateWirelessGateway"
	opGetWirelessDeviceStatistics                           = "GetWirelessDeviceStatistics"
	opSendDataToWirelessDevice                              = "SendDataToWirelessDevice"
	opTestWirelessDevice                                    = "TestWirelessDevice"
	opDeregisterWirelessDevice                              = "DeregisterWirelessDevice"
	opUpdateWirelessDevice                                  = "UpdateWirelessDevice"
	opDisassociateWirelessDeviceFromThing                   = "DisassociateWirelessDeviceFromThing"
	opDeleteQueuedMessages                                  = "DeleteQueuedMessages"
	opListQueuedMessages                                    = "ListQueuedMessages"
	opStartWirelessDeviceImportTask                         = "StartWirelessDeviceImportTask"
	opStartSingleWirelessDeviceImportTask                   = "StartSingleWirelessDeviceImportTask"
	opGetWirelessDeviceImportTask                           = "GetWirelessDeviceImportTask"
	opDeleteWirelessDeviceImportTask                        = "DeleteWirelessDeviceImportTask"
	opUpdateWirelessDeviceImportTask                        = "UpdateWirelessDeviceImportTask"
	opListWirelessDeviceImportTasks                         = "ListWirelessDeviceImportTasks"
	opListDevicesForWirelessDeviceImportTask                = "ListDevicesForWirelessDeviceImportTask"
	opGetPartnerAccount                                     = "GetPartnerAccount"
	opListPartnerAccounts                                   = "ListPartnerAccounts"
	opDisassociateAwsAccountFromPartnerAccount              = "DisassociateAwsAccountFromPartnerAccount"
	opUpdatePartnerAccount                                  = "UpdatePartnerAccount"
	opUpdateDestination                                     = "UpdateDestination"
	opGetServiceEndpoint                                    = "GetServiceEndpoint"

	// Destination operation name constants (used in GetSupportedOperations, routing, and dispatch).
	opCreateDestination = "CreateDestination"
	opGetDestination    = "GetDestination"
	opListDestinations  = "ListDestinations"
	opDeleteDestination = "DeleteDestination"

	// path sub-segment constants.
	pathSubSession = "session"
	// singular forms bound by the Associate* PUT ops; iotwireless@v1.59.4
	// serializers.go:140 (/fuota-tasks/{Id}/multicast-group) and :234
	// (/fuota-tasks/{Id}/wireless-device), :328
	// (/multicast-groups/{Id}/wireless-device) — the DELETE/GET variants
	// use the plural pathBase* constants instead.
	pathSubMulticastGroup = "multicast-group"
	pathSubWirelessDevice = "wireless-device"
)

const (
	iotwirelessService       = "iotwireless"
	iotwirelessMatchPriority = 86
)

// Handler is the HTTP handler for the IoT Wireless REST API.
type Handler struct {
	Backend       StorageBackend
	AccountID     string
	DefaultRegion string
}

// NewHandler creates a new IoT Wireless handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Reset clears the handler's backend state, returning it to a pristine condition.
func (h *Handler) Reset() {
	if r, ok := h.Backend.(interface{ Reset() }); ok {
		r.Reset()
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "IoTWireless" }

// supportedWirelessDeviceOps returns operation names for wireless device
// CRUD, messaging, and thing-association operations.
func supportedWirelessDeviceOps() []string {
	return []string{
		"CreateWirelessDevice",
		"GetWirelessDevice",
		"ListWirelessDevices",
		"DeleteWirelessDevice",
		"UpdateWirelessDevice",
		"DeregisterWirelessDevice",
		"GetWirelessDeviceStatistics",
		"SendDataToWirelessDevice",
		"TestWirelessDevice",
		"DisassociateWirelessDeviceFromThing",
		"DeleteQueuedMessages",
		"ListQueuedMessages",
	}
}

// supportedWirelessGatewayOps returns operation names for wireless gateway
// CRUD, certificate/thing association, task, and task-definition operations.
func supportedWirelessGatewayOps() []string {
	return []string{
		"CreateWirelessGateway",
		"GetWirelessGateway",
		"ListWirelessGateways",
		"DeleteWirelessGateway",
		"UpdateWirelessGateway",
		"GetWirelessGatewayCertificate",
		"GetWirelessGatewayFirmwareInformation",
		"GetWirelessGatewayStatistics",
		"GetWirelessGatewayTask",
		"DeleteWirelessGatewayTask",
		"CreateWirelessGatewayTask",
		"DisassociateWirelessGatewayFromCertificate",
		"DisassociateWirelessGatewayFromThing",
		"CreateWirelessGatewayTaskDefinition",
		"GetWirelessGatewayTaskDefinition",
		"ListWirelessGatewayTaskDefinitions",
		"DeleteWirelessGatewayTaskDefinition",
	}
}

// supportedProfileAndDestinationOps returns operation names for service
// profile, destination, and resource-tagging operations.
func supportedProfileAndDestinationOps() []string {
	return []string{
		"CreateServiceProfile",
		"GetServiceProfile",
		"ListServiceProfiles",
		"DeleteServiceProfile",
		opCreateDestination,
		opGetDestination,
		opListDestinations,
		opDeleteDestination,
		opUpdateDestination,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
	}
}

// supportedAssociationOps returns operation names for cross-resource
// association operations (partner accounts, FUOTA tasks, multicast groups,
// things, certificates).
func supportedAssociationOps() []string {
	return []string{
		opAssociateAwsAccountWithPartnerAccount,
		opAssociateMulticastGroupWithFuotaTask,
		opAssociateWirelessDeviceWithFuotaTask,
		opAssociateWirelessDeviceWithMulticastGroup,
		opAssociateWirelessDeviceWithThing,
		opAssociateWirelessGatewayWithCertificate,
		opAssociateWirelessGatewayWithThing,
		opCancelMulticastGroupSession,
	}
}

// supportedDeviceProfileAndFuotaOps returns operation names for device
// profile and FUOTA task operations.
func supportedDeviceProfileAndFuotaOps() []string {
	return []string{
		"CreateDeviceProfile",
		"GetDeviceProfile",
		"ListDeviceProfiles",
		"DeleteDeviceProfile",
		"CreateFuotaTask",
		"GetFuotaTask",
		"ListFuotaTasks",
		"DeleteFuotaTask",
		"UpdateFuotaTask",
		"StartFuotaTask",
		"DisassociateMulticastGroupFromFuotaTask",
		"DisassociateWirelessDeviceFromFuotaTask",
	}
}

// supportedMulticastAndAnalyzerOps returns operation names for multicast
// group and network analyzer configuration operations.
func supportedMulticastAndAnalyzerOps() []string {
	return []string{
		"CreateMulticastGroup",
		"GetMulticastGroup",
		"ListMulticastGroups",
		"DeleteMulticastGroup",
		"UpdateMulticastGroup",
		"GetMulticastGroupSession",
		"StartMulticastGroupSession",
		"ListMulticastGroupsByFuotaTask",
		"SendDataToMulticastGroup",
		"StartBulkAssociateWirelessDeviceWithMulticastGroup",
		"StartBulkDisassociateWirelessDeviceFromMulticastGroup",
		"DisassociateWirelessDeviceFromMulticastGroup",
		"CreateNetworkAnalyzerConfiguration",
		"GetNetworkAnalyzerConfiguration",
		"ListNetworkAnalyzerConfigurations",
		"DeleteNetworkAnalyzerConfiguration",
		"UpdateNetworkAnalyzerConfiguration",
	}
}

// supportedEventAndLogOps returns operation names for event configuration
// and log level operations.
func supportedEventAndLogOps() []string {
	return []string{
		"GetEventConfigurationByResourceTypes",
		"UpdateEventConfigurationByResourceTypes",
		"ListEventConfigurations",
		"GetResourceEventConfiguration",
		"UpdateResourceEventConfiguration",
		"GetLogLevelsByResourceTypes",
		"UpdateLogLevelsByResourceTypes",
		"ResetAllResourceLogLevels",
		"GetResourceLogLevel",
		"PutResourceLogLevel",
		"ResetResourceLogLevel",
	}
}

// supportedMetricsAndPositionOps returns operation names for metrics and
// device/gateway positioning operations.
func supportedMetricsAndPositionOps() []string {
	return []string{
		"GetMetricConfiguration",
		"UpdateMetricConfiguration",
		"GetMetrics",
		"GetPosition",
		"UpdatePosition",
		"GetPositionConfiguration",
		"PutPositionConfiguration",
		"ListPositionConfigurations",
		"GetPositionEstimate",
		"GetResourcePosition",
		"UpdateResourcePosition",
	}
}

// supportedImportAndPartnerOps returns operation names for wireless device
// import task and partner account operations.
func supportedImportAndPartnerOps() []string {
	return []string{
		"GetWirelessDeviceImportTask",
		"DeleteWirelessDeviceImportTask",
		"UpdateWirelessDeviceImportTask",
		"ListWirelessDeviceImportTasks",
		"ListDevicesForWirelessDeviceImportTask",
		"StartWirelessDeviceImportTask",
		"StartSingleWirelessDeviceImportTask",
		"GetPartnerAccount",
		"ListPartnerAccounts",
		"DisassociateAwsAccountFromPartnerAccount",
		"UpdatePartnerAccount",
		"GetServiceEndpoint",
	}
}

// supportedOperationGroups lists every operation-name group that together
// make up the full IoT Wireless supported-operations list. Split into
// per-family slices (rather than one long literal) so no single function
// carries the whole exhaustive list -- see GetSupportedOperations.
func supportedOperationGroups() [][]string {
	return [][]string{
		supportedWirelessDeviceOps(),
		supportedWirelessGatewayOps(),
		supportedProfileAndDestinationOps(),
		supportedAssociationOps(),
		supportedDeviceProfileAndFuotaOps(),
		supportedMulticastAndAnalyzerOps(),
		supportedEventAndLogOps(),
		supportedMetricsAndPositionOps(),
		supportedImportAndPartnerOps(),
	}
}

// GetSupportedOperations returns the list of supported IoT Wireless operations.
func (h *Handler) GetSupportedOperations() []string {
	var ops []string

	for _, group := range supportedOperationGroups() {
		ops = append(ops, group...)
	}

	return ops
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return iotwirelessService }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this IoT Wireless instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.DefaultRegion} }

// RouteMatcher returns a function that matches IoT Wireless REST API requests.
// All paths are disambiguated via the SigV4 credential-scope service name to
// prevent mis-routing with other REST-JSON services that share similar paths.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		for _, prefix := range []string{
			"/" + pathBaseWirelessDevices,
			"/" + pathBaseWirelessGateways,
			"/" + pathBaseServiceProfiles,
			"/" + pathBaseDestinations,
			"/" + pathBaseDeviceProfiles,
			"/" + pathBaseFuotaTasks,
			"/" + pathBaseMulticastGroups,
			"/" + pathBasePartnerAccounts,
			"/" + pathBaseNetworkAnalyzerConfigs,
			"/" + pathBaseEventConfigsResourceTypes,
			"/" + pathBaseEventConfigs,
			"/" + pathBaseLogLevels,
			"/" + pathBaseMetricConfiguration,
			"/" + pathBaseMetrics,
			"/" + pathBasePositions,
			"/" + pathBasePositionConfigurations,
			"/" + pathBasePositionEstimate,
			"/" + pathBaseResourcePositions,
			"/" + pathBaseWirelessGatewayTaskDefs,
			"/" + pathBaseWirelessDeviceImportTask,
			"/" + pathBaseWirelessDeviceImportTasks,
			"/" + pathBaseSingleWirelessDeviceImport,
			"/" + pathBaseServiceEndpoint,
		} {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return httputils.ExtractServiceFromRequest(c.Request()) == iotwirelessService
			}
		}

		// TagResource / UntagResource / ListTagsForResource bind to the bare
		// "/tags" path; the resourceArn travels as a query parameter, never
		// as a path segment (confirmed against aws-sdk-go-v2's REST-JSON
		// serializer: SplitURI("/tags") + SetQuery("resourceArn")).
		if path == "/tags" {
			return httputils.ExtractServiceFromRequest(c.Request()) == iotwirelessService
		}

		return false
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return iotwirelessMatchPriority }

// ExtractOperation extracts the IoT Wireless operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _ := parseIoTWirelessPath(c.Request().Method, c.Request().URL.Path)

	return op
}

// ExtractResource extracts the resource ID from the URL path. Tag operations
// are the one exception: their resource ARN travels as the "resourceArn"
// query parameter rather than a path segment (see parseIoTWirelessPath's
// "tags" case), so it's read from the query here for CloudTrail capture.
func (h *Handler) ExtractResource(c *echo.Context) string {
	if c.Request().URL.Path == "/tags" {
		return c.Request().URL.Query().Get("resourceArn")
	}

	_, resource := parseIoTWirelessPath(c.Request().Method, c.Request().URL.Path)

	return resource
}

// Handler returns the Echo handler function for IoT Wireless requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		method := c.Request().Method
		path := c.Request().URL.Path

		op, resource := parseIoTWirelessPath(method, path)
		if op == "" {
			return writeError(c, http.StatusNotFound, "resource not found")
		}

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "iotwireless: failed to read request body", "error", err)

			return writeError(c, http.StatusInternalServerError, "failed to read request body")
		}

		log.DebugContext(ctx, "iotwireless request", "op", op, "resource", resource)

		return h.dispatch(c, op, resource, body, c.Request().URL.Query())
	}
}

// dispatch routes to the appropriate handler based on the operation name.
func (h *Handler) dispatch(c *echo.Context, op, resource string, body []byte, query url.Values) error {
	if handled, result := h.dispatchCoreOps(c, op, resource, body); handled {
		return result
	}

	if handled, result := h.dispatchTagOps(c, op, resource, body, query); handled {
		return result
	}

	return writeError(c, http.StatusNotFound, "unknown operation")
}

// dispatchCoreOps routes all non-tag operations.
func (h *Handler) dispatchCoreOps(c *echo.Context, op, resource string, body []byte) (bool, error) {
	if handled, result := h.dispatchWirelessDevice(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchWirelessGateway(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchServiceProfile(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchDestination(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchNewOps(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchMulticastOps(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchNetworkAnalyzerOps(c, op, resource, body); handled {
		return true, result
	}

	return h.dispatchExtendedOpsGroup(c, op, resource, body)
}

// dispatchExtendedOpsGroup routes metrics, position, gateway task, import task, and partner ops.
func (h *Handler) dispatchExtendedOpsGroup(c *echo.Context, op, resource string, body []byte) (bool, error) {
	if handled, result := h.dispatchMetricsAndLogOps(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchPositionOps(c, op, resource); handled {
		return true, result
	}

	if handled, result := h.dispatchGatewayTaskOps(c, op, resource, body); handled {
		return true, result
	}

	if handled, result := h.dispatchImportTaskOps(c, op, resource); handled {
		return true, result
	}

	return h.dispatchPartnerAndMiscOps(c, op, resource, body)
}

// dispatchTagOps handles resource tagging operations. The resource ARN
// travels as the "resourceArn" query parameter (see parseIoTWirelessPath's
// "tags" case), not as the path-derived resource argument.
func (h *Handler) dispatchTagOps(c *echo.Context, op, _ string, body []byte, query url.Values) (bool, error) {
	resourceArn := query.Get("resourceArn")

	switch op {
	case opListTagsForResource:
		return true, h.listTagsForResource(c, resourceArn)
	case opTagResource:
		return true, h.tagResource(c, resourceArn, body)
	case opUntagResource:
		return true, h.untagResource(c, resourceArn, query)
	}

	return false, nil
}

// dispatchMulticastOps handles multicast group operations.
func (h *Handler) dispatchMulticastOps(c *echo.Context, op, resource string, body []byte) (bool, error) {
	if handled, result := h.dispatchMulticastGroupCRUDOps(c, op, resource); handled {
		return true, result
	}

	return h.dispatchMulticastAssocOps(c, op, resource, body)
}

// dispatchMulticastGroupCRUDOps handles multicast group CRUD and session operations.
func (h *Handler) dispatchMulticastGroupCRUDOps(c *echo.Context, op, resource string) (bool, error) {
	switch op {
	case opCreateMulticastGroup:
		return true, h.createMulticastGroup(c)
	case opGetMulticastGroup:
		return true, h.getMulticastGroup(c, resource)
	case opListMulticastGroups:
		return true, h.listMulticastGroups(c)
	case opDeleteMulticastGroup:
		return true, h.deleteMulticastGroup(c, resource)
	case opUpdateMulticastGroup:
		return true, h.updateMulticastGroup(c, resource)
	case opGetMulticastGroupSession:
		return true, h.getMulticastGroupSession(c, resource)
	case opStartMulticastGroupSession:
		return true, h.startMulticastGroupSession(c, resource)
	case opListMulticastGroupsByFuotaTask:
		return true, h.listMulticastGroupsByFuotaTask(c, resource)
	case opSendDataToMulticastGroup:
		return true, h.sendDataToMulticastGroup(c, resource)
	}

	return false, nil
}

// dispatchMulticastAssocOps handles multicast group association/disassociation and FUOTA task operations.
func (h *Handler) dispatchMulticastAssocOps(c *echo.Context, op, resource string, _ []byte) (bool, error) {
	switch op {
	case opStartBulkAssociateWirelessDeviceWithMulticastGroup:
		return true, h.startBulkAssociateWirelessDeviceWithMulticastGroup(c, resource)
	case opStartBulkDisassociateWirelessDeviceFromMulticastGroup:
		return true, h.startBulkDisassociateWirelessDeviceFromMulticastGroup(c, resource)
	case opDisassociateWirelessDeviceFromMulticastGroup:
		return true, h.disassociateWirelessDeviceFromMulticastGroup(c, resource, lastPathSegment(c))
	case opDisassociateMulticastGroupFromFuotaTask:
		return true, h.disassociateMulticastGroupFromFuotaTask(c, resource, lastPathSegment(c))
	case opDisassociateWirelessDeviceFromFuotaTask:
		return true, h.disassociateWirelessDeviceFromFuotaTask(c, resource, lastPathSegment(c))
	case opStartFuotaTask:
		return true, h.startFuotaTask(c, resource)
	case opUpdateFuotaTask:
		return true, h.updateFuotaTask(c, resource)
	}

	return false, nil
}

// dispatchNetworkAnalyzerOps handles network analyzer configuration operations.
func (h *Handler) dispatchNetworkAnalyzerOps(c *echo.Context, op, resource string, _ []byte) (bool, error) {
	switch op {
	case opCreateNetworkAnalyzerConfiguration:
		return true, h.createNetworkAnalyzerConfiguration(c)
	case opGetNetworkAnalyzerConfiguration:
		return true, h.getNetworkAnalyzerConfiguration(c, resource)
	case opListNetworkAnalyzerConfigurations:
		return true, h.listNetworkAnalyzerConfigurations(c)
	case opDeleteNetworkAnalyzerConfiguration:
		return true, h.deleteNetworkAnalyzerConfiguration(c, resource)
	case opUpdateNetworkAnalyzerConfiguration:
		return true, h.updateNetworkAnalyzerConfiguration(c, resource)
	case opGetEventConfigurationByResourceTypes:
		return true, h.getEventConfigurationByResourceTypes(c)
	case opUpdateEventConfigurationByResourceTypes:
		return true, h.updateEventConfigurationByResourceTypes(c)
	case opListEventConfigurations:
		return true, h.listEventConfigurations(c)
	case opGetResourceEventConfiguration:
		return true, h.getResourceEventConfiguration(c, resource)
	case opUpdateResourceEventConfiguration:
		return true, h.updateResourceEventConfiguration(c, resource)
	}

	return false, nil
}

// dispatchMetricsAndLogOps handles metrics, log level, and event config operations.
func (h *Handler) dispatchMetricsAndLogOps(c *echo.Context, op, resource string, _ []byte) (bool, error) {
	switch op {
	case opGetLogLevelsByResourceTypes:
		return true, h.getLogLevelsByResourceTypes(c)
	case opUpdateLogLevelsByResourceTypes:
		return true, h.updateLogLevelsByResourceTypes(c)
	case opResetAllResourceLogLevels:
		return true, h.resetAllResourceLogLevels(c)
	case opGetResourceLogLevel:
		return true, h.getResourceLogLevel(c, resource)
	case opPutResourceLogLevel:
		return true, h.putResourceLogLevel(c, resource)
	case opResetResourceLogLevel:
		return true, h.resetResourceLogLevel(c, resource)
	case opGetMetricConfiguration:
		return true, h.getMetricConfiguration(c)
	case opUpdateMetricConfiguration:
		return true, h.updateMetricConfiguration(c)
	case opGetMetrics:
		return true, h.getMetrics(c)
	}

	return false, nil
}

// dispatchPositionOps handles position and location operations.
func (h *Handler) dispatchPositionOps(c *echo.Context, op, resource string) (bool, error) {
	switch op {
	case opGetPosition:
		return true, h.getPosition(c, resource)
	case opUpdatePosition:
		return true, h.updatePosition(c, resource)
	case opGetPositionConfiguration:
		return true, h.getPositionConfiguration(c, resource)
	case opPutPositionConfiguration:
		return true, h.putPositionConfiguration(c, resource)
	case opListPositionConfigurations:
		return true, h.listPositionConfigurations(c)
	case opGetPositionEstimate:
		return true, h.getPositionEstimate(c)
	case opGetResourcePosition:
		return true, h.getResourcePosition(c, resource)
	case opUpdateResourcePosition:
		return true, h.updateResourcePosition(c, resource)
	}

	return false, nil
}

// dispatchGatewayTaskOps handles gateway task and task definition operations.
func (h *Handler) dispatchGatewayTaskOps(c *echo.Context, op, resource string, _ []byte) (bool, error) {
	switch op {
	case opCreateWirelessGatewayTask:
		return true, h.createWirelessGatewayTask(c, resource)
	case opGetWirelessGatewayTask:
		return true, h.getWirelessGatewayTask(c, resource)
	case opDeleteWirelessGatewayTask:
		return true, h.deleteWirelessGatewayTask(c, resource)
	case opCreateWirelessGatewayTaskDefinition:
		return true, h.createWirelessGatewayTaskDefinition(c)
	case opGetWirelessGatewayTaskDefinition:
		return true, h.getWirelessGatewayTaskDefinition(c, resource)
	case opListWirelessGatewayTaskDefinitions:
		return true, h.listWirelessGatewayTaskDefinitions(c)
	case opDeleteWirelessGatewayTaskDefinition:
		return true, h.deleteWirelessGatewayTaskDefinition(c, resource)
	}

	return false, nil
}

// dispatchImportTaskOps handles wireless device import task operations.
func (h *Handler) dispatchImportTaskOps(c *echo.Context, op, resource string) (bool, error) {
	switch op {
	case opStartWirelessDeviceImportTask:
		return true, h.startWirelessDeviceImportTask(c)
	case opStartSingleWirelessDeviceImportTask:
		return true, h.startSingleWirelessDeviceImportTask(c)
	case opGetWirelessDeviceImportTask:
		return true, h.getWirelessDeviceImportTask(c, resource)
	case opDeleteWirelessDeviceImportTask:
		return true, h.deleteWirelessDeviceImportTask(c, resource)
	case opUpdateWirelessDeviceImportTask:
		return true, h.updateWirelessDeviceImportTask(c, resource)
	case opListWirelessDeviceImportTasks:
		return true, h.listWirelessDeviceImportTasks(c)
	case opListDevicesForWirelessDeviceImportTask:
		return true, h.listDevicesForWirelessDeviceImportTask(c)
	}

	return false, nil
}

// dispatchPartnerAndMiscOps handles partner account, destination update, and misc operations.
func (h *Handler) dispatchPartnerAndMiscOps(c *echo.Context, op, resource string, _ []byte) (bool, error) {
	if handled, result := h.dispatchPartnerOps(c, op, resource); handled {
		return true, result
	}

	return h.dispatchGatewayDeviceMiscOps(c, op, resource)
}

// dispatchPartnerOps handles partner account and destination operations.
func (h *Handler) dispatchPartnerOps(c *echo.Context, op, resource string) (bool, error) {
	switch op {
	case opGetPartnerAccount:
		return true, h.getPartnerAccount(c, resource)
	case opListPartnerAccounts:
		return true, h.listPartnerAccounts(c)
	case opDisassociateAwsAccountFromPartnerAccount:
		return true, h.disassociateAwsAccountFromPartnerAccount(c, resource)
	case opUpdatePartnerAccount:
		return true, h.updatePartnerAccount(c, resource)
	case opUpdateDestination:
		return true, h.updateDestination(c, resource)
	case opGetServiceEndpoint:
		return true, h.getServiceEndpoint(c)
	case opGetWirelessGatewayCertificate:
		return true, h.getWirelessGatewayCertificate(c, resource)
	case opGetWirelessGatewayFirmwareInformation:
		return true, h.getWirelessGatewayFirmwareInformation(c, resource)
	case opGetWirelessGatewayStatistics:
		return true, h.getWirelessGatewayStatistics(c, resource)
	case opDisassociateWirelessGatewayFromCertificate:
		return true, h.disassociateWirelessGatewayFromCertificate(c, resource)
	}

	return false, nil
}

// dispatchGatewayDeviceMiscOps handles wireless gateway and device miscellaneous operations.
func (h *Handler) dispatchGatewayDeviceMiscOps(c *echo.Context, op, resource string) (bool, error) {
	switch op {
	case opDisassociateWirelessGatewayFromThing:
		return true, h.disassociateWirelessGatewayFromThing(c, resource)
	case opUpdateWirelessGateway:
		return true, h.updateWirelessGateway(c, resource)
	case opGetWirelessDeviceStatistics:
		return true, h.getWirelessDeviceStatistics(c, resource)
	case opSendDataToWirelessDevice:
		return true, h.sendDataToWirelessDevice(c, resource)
	case opTestWirelessDevice:
		return true, h.testWirelessDevice(c, resource)
	case opDeregisterWirelessDevice:
		return true, h.deregisterWirelessDevice(c, resource)
	case opUpdateWirelessDevice:
		return true, h.updateWirelessDevice(c, resource)
	case opDisassociateWirelessDeviceFromThing:
		return true, h.disassociateWirelessDeviceFromThing(c, resource)
	case opDeleteQueuedMessages:
		return true, h.deleteQueuedMessages(c, resource)
	case opListQueuedMessages:
		return true, h.listQueuedMessages(c, resource)
	}

	return false, nil
}

// dispatchWirelessDevice handles wireless device operations.
func (h *Handler) dispatchWirelessDevice(c *echo.Context, op, resource string, body []byte) (bool, error) {
	switch op {
	case "CreateWirelessDevice":
		return true, h.createWirelessDevice(c, body)
	case "GetWirelessDevice":
		return true, h.getWirelessDevice(c, resource)
	case "ListWirelessDevices":
		return true, h.listWirelessDevices(c)
	case "DeleteWirelessDevice":
		return true, h.deleteWirelessDevice(c, resource)
	}

	return false, nil
}

// dispatchWirelessGateway handles wireless gateway operations.
func (h *Handler) dispatchWirelessGateway(c *echo.Context, op, resource string, body []byte) (bool, error) {
	switch op {
	case "CreateWirelessGateway":
		return true, h.createWirelessGateway(c, body)
	case "GetWirelessGateway":
		return true, h.getWirelessGateway(c, resource)
	case "ListWirelessGateways":
		return true, h.listWirelessGateways(c)
	case "DeleteWirelessGateway":
		return true, h.deleteWirelessGateway(c, resource)
	}

	return false, nil
}

// dispatchServiceProfile handles service profile operations.
func (h *Handler) dispatchServiceProfile(c *echo.Context, op, resource string, body []byte) (bool, error) {
	switch op {
	case "CreateServiceProfile":
		return true, h.createServiceProfile(c, body)
	case "GetServiceProfile":
		return true, h.getServiceProfile(c, resource)
	case "ListServiceProfiles":
		return true, h.listServiceProfiles(c)
	case "DeleteServiceProfile":
		return true, h.deleteServiceProfile(c, resource)
	}

	return false, nil
}

// dispatchDestination handles destination operations.
func (h *Handler) dispatchDestination(c *echo.Context, op, resource string, body []byte) (bool, error) {
	switch op {
	case opCreateDestination:
		return true, h.createDestination(c, body)
	case opGetDestination:
		return true, h.getDestination(c, resource)
	case opListDestinations:
		return true, h.listDestinations(c)
	case opDeleteDestination:
		return true, h.deleteDestination(c, resource)
	}

	return false, nil
}

// dispatchNewOps handles operations added in this implementation.
func (h *Handler) dispatchNewOps(c *echo.Context, op, resource string, body []byte) (bool, error) {
	if handled, err := h.dispatchNewCRUDOps(c, op, resource, body); handled {
		return true, err
	}

	return h.dispatchAssociationOps(c, op, resource, body)
}

// dispatchNewCRUDOps handles CRUD operations for DeviceProfile and FuotaTask.
func (h *Handler) dispatchNewCRUDOps(c *echo.Context, op, resource string, body []byte) (bool, error) {
	switch op {
	case "CreateDeviceProfile":
		return true, h.createDeviceProfile(c, body)
	case "GetDeviceProfile":
		return true, h.getDeviceProfile(c, resource)
	case "ListDeviceProfiles":
		return true, h.listDeviceProfiles(c)
	case "DeleteDeviceProfile":
		return true, h.deleteDeviceProfile(c, resource)
	case "CreateFuotaTask":
		return true, h.createFuotaTask(c, body)
	case "GetFuotaTask":
		return true, h.getFuotaTask(c, resource)
	case "ListFuotaTasks":
		return true, h.listFuotaTasks(c)
	case "DeleteFuotaTask":
		return true, h.deleteFuotaTask(c, resource)
	}

	return false, nil
}

// dispatchAssociationOps handles AWS IoT Wireless resource-association operations.
func (h *Handler) dispatchAssociationOps(c *echo.Context, op, resource string, body []byte) (bool, error) {
	switch op {
	case opAssociateAwsAccountWithPartnerAccount:
		return true, h.associateAwsAccountWithPartnerAccount(c, body)
	case opAssociateMulticastGroupWithFuotaTask:
		return true, h.associateMulticastGroupWithFuotaTask(c, resource, body)
	case opAssociateWirelessDeviceWithFuotaTask:
		return true, h.associateWirelessDeviceWithFuotaTask(c, resource, body)
	case opAssociateWirelessDeviceWithMulticastGroup:
		return true, h.associateWirelessDeviceWithMulticastGroup(c, resource, body)
	case opAssociateWirelessDeviceWithThing:
		return true, h.associateWirelessDeviceWithThing(c, resource, body)
	case opAssociateWirelessGatewayWithCertificate:
		return true, h.associateWirelessGatewayWithCertificate(c, resource, body)
	case opAssociateWirelessGatewayWithThing:
		return true, h.associateWirelessGatewayWithThing(c, resource, body)
	case opCancelMulticastGroupSession:
		return true, h.cancelMulticastGroupSession(c, resource)
	}

	return false, nil
}

type errorResponse struct {
	Type    string `json:"__type"`
	Message string `json:"Message"`
}

// awsErrorType maps an HTTP status code to the modeled IoT Wireless exception
// name (ResourceNotFoundException, ValidationException, ...) that real AWS
// returns for that status. writeError sets this as the X-Amzn-Errortype
// header and the __type body field: the aws-sdk-go-v2 REST-JSON error
// deserializer (awsRestjson1_deserializeOpError*) reads whichever of those is
// present to decide which typed *types.XxxException to construct. Without
// either, every error from this service deserializes into an untyped
// smithy.GenericAPIError{Code: "UnknownError"}, so `errors.As(err,
// &types.ResourceNotFoundException{})`-style handling (used by waiters,
// retries, and most application code) silently never matches.
func awsErrorType(status int) string {
	switch status {
	case http.StatusNotFound:
		return "ResourceNotFoundException"
	case http.StatusBadRequest:
		return "ValidationException"
	case http.StatusForbidden:
		return "AccessDeniedException"
	case http.StatusConflict:
		return "ConflictException"
	case http.StatusTooManyRequests:
		return "ThrottlingException"
	default:
		return "InternalServerException"
	}
}

// writeError writes a JSON error response, setting the X-Amzn-Errortype
// header (and __type body field) so aws-sdk-go-v2 clients deserialize it into
// the correctly typed modeled exception. See awsErrorType.
func writeError(c *echo.Context, status int, message string) error {
	errType := awsErrorType(status)

	c.Response().Header().Set("Content-Type", "application/json")
	c.Response().Header().Set("X-Amzn-Errortype", errType)
	c.Response().WriteHeader(status)

	_ = json.NewEncoder(c.Response()).Encode(errorResponse{Type: errType, Message: message})

	return nil
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(c *echo.Context, status int, v any) error {
	c.Response().Header().Set("Content-Type", "application/json")
	c.Response().WriteHeader(status)

	_ = json.NewEncoder(c.Response()).Encode(v)

	return nil
}

// decodeNotFoundError maps not-found sentinel errors to 404.
func isNotFound(err error) bool {
	return errors.Is(err, ErrDeviceNotFound) ||
		errors.Is(err, ErrGatewayNotFound) ||
		errors.Is(err, ErrServiceProfileNotFound) ||
		errors.Is(err, ErrDestinationNotFound) ||
		errors.Is(err, ErrDeviceProfileNotFound) ||
		errors.Is(err, ErrFuotaTaskNotFound) ||
		errors.Is(err, ErrMulticastGroupNotFound) ||
		errors.Is(err, ErrNetworkAnalyzerConfigNotFound) ||
		errors.Is(err, ErrImportTaskNotFound) ||
		errors.Is(err, ErrGatewayTaskDefNotFound) ||
		errors.Is(err, ErrGatewayTaskNotFound) ||
		errors.Is(err, ErrPartnerAccountNotFound) ||
		errors.Is(err, ErrMulticastGroupSessionNotFound)
}

// isConflict reports whether err is one of the delete-precondition sentinel
// errors (a resource still referenced by another resource).
func isConflict(err error) bool {
	return errors.Is(err, ErrDeviceProfileInUse) ||
		errors.Is(err, ErrServiceProfileInUse) ||
		errors.Is(err, ErrDestinationInUse) ||
		errors.Is(err, ErrMulticastGroupInUse)
}

// handleError writes an appropriate HTTP error response for a backend error.
func handleError(c *echo.Context, err error) error {
	if isNotFound(err) {
		return writeError(c, http.StatusNotFound, err.Error())
	}

	if errors.Is(err, ErrValidation) {
		return writeError(c, http.StatusBadRequest, err.Error())
	}

	if isConflict(err) {
		return writeError(c, http.StatusConflict, err.Error())
	}

	return writeError(c, http.StatusInternalServerError, err.Error())
}

// thingNameFromArn extracts the Thing name from an IoT Thing ARN
// (arn:aws:iot:region:account:thing/name), matching how real AWS derives
// ThingName from the ThingArn supplied to AssociateWirelessDeviceWithThing /
// AssociateWirelessGatewayWithThing (the request never carries ThingName
// directly). Returns "" if thingArn is empty or has no "/" separator.
func thingNameFromArn(thingArn string) string {
	idx := strings.LastIndex(thingArn, "/")
	if idx < 0 {
		return ""
	}

	return thingArn[idx+1:]
}

// maxStubBodyBytes caps stub request body reads to prevent unbounded memory
// usage on attacker-controlled inputs. IoT Wireless API payloads are far below
// 1 MiB; cap conservatively.
const maxStubBodyBytes = 1 << 20

// readStubBody returns the request body capped at maxStubBodyBytes. Errors are
// swallowed because stubs treat unparsed input as empty (matching prior behavior).
func readStubBody(c *echo.Context) []byte {
	body, _ := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, maxStubBodyBytes))

	return body
}

// stubNoContent writes 204 No Content and returns nil.
func stubNoContent(c *echo.Context) error {
	c.Response().WriteHeader(http.StatusNoContent)

	return nil
}

// lastPathSegment returns the final "/"-separated segment of the request
// path. parseIoTWirelessPath's (op, resource) pair only ever carries the
// top-level {Id} path parameter, never a trailing sub-resource ID (e.g. the
// {WirelessDeviceId} in DELETE
// /multicast-groups/{Id}/wireless-devices/{WirelessDeviceId}); handlers that
// need that trailing ID recover it directly from the URL here.
func lastPathSegment(c *echo.Context) string {
	path := c.Request().URL.Path

	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return ""
	}

	return path[idx+1:]
}
