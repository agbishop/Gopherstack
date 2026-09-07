package rds

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// Handler is the Echo HTTP handler for RDS operations.
type Handler struct {
	Backend       *InMemoryBackend
	backends      map[string]*InMemoryBackend
	handlers      map[string]*Handler
	accountID     string
	defaultRegion string
	mu            sync.Mutex
}

// NewHandler creates a new RDS handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{
		Backend:       backend,
		backends:      make(map[string]*InMemoryBackend),
		handlers:      make(map[string]*Handler),
		accountID:     backend.AccountID(),
		defaultRegion: backend.Region(),
	}
	h.backends[backend.Region()] = backend
	h.handlers[backend.Region()] = &Handler{Backend: backend}

	return h
}

func (h *Handler) getHandlerForRegion(region string) *Handler {
	if h.defaultRegion == "" {
		// Not the main router handler.

		return h
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if handler, ok := h.handlers[region]; ok {
		return handler
	}
	backend := NewInMemoryBackend(h.accountID, region)
	h.backends[region] = backend
	handler := &Handler{Backend: backend}
	h.handlers[region] = handler

	return handler
}

// Reset clears all backend state. Useful for test isolation.
func (h *Handler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.backends) > 0 {
		for _, b := range h.backends {
			b.Reset()
		}
	} else if h.Backend != nil {
		h.Backend.Reset()
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "RDS" }

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "rds" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this RDS instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches RDS requests.
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
			// marker every aws-sdk-go-v2 rds client sets (api_client.go's
			// AddSDKAgentKeyValue -- "api/rds"). That still identifies this
			// as ours, so claim it and let Handler() produce the typed
			// error instead of masking the read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/rds")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == rdsVersion
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityFormRDS }

const unknownOp = "Unknown"

// ExtractOperation extracts the RDS action from the request.
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

// ExtractResource returns the DB instance identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}

	return vals.Get("DBInstanceIdentifier")
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		body, err := httputils.ReadBody(r)
		if err != nil {
			return h.writeError(c, http.StatusInternalServerError, "InternalFailure", "failed to read request body")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return h.writeError(c, http.StatusBadRequest, "ValidationException", "failed to parse request body")
		}
		action := vals.Get("Action")
		if action == "" {
			return h.writeError(c, http.StatusBadRequest, "MissingAction", "missing Action parameter")
		}

		region := httputils.ExtractRegionFromRequest(r, h.defaultRegion)
		handler := h.getHandlerForRegion(region)

		resp, opErr := handler.dispatch(action, vals)
		if opErr != nil {
			return handler.handleOpError(c, action, opErr)
		}

		xmlBytes, err := marshalXML(resp)
		if err != nil {
			return handler.writeError(c, http.StatusInternalServerError, "InternalFailure", "internal server error")
		}

		return c.Blob(http.StatusOK, "text/xml", xmlBytes)
	}
}

// dispatch routes the RDS action to the appropriate handler.
func (h *Handler) dispatch(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateDBInstance":

		return h.handleCreateDBInstance(vals)
	case "DeleteDBInstance":

		return h.handleDeleteDBInstance(vals)
	case "DescribeDBInstances":

		return h.handleDescribeDBInstances(vals)
	case "ModifyDBInstance":

		return h.handleModifyDBInstance(vals)
	case "CreateDBSnapshot":

		return h.handleCreateDBSnapshot(vals)
	case "DescribeDBSnapshots":

		return h.handleDescribeDBSnapshots(vals)
	case "DeleteDBSnapshot":

		return h.handleDeleteDBSnapshot(vals)
	case "CreateDBSubnetGroup":

		return h.handleCreateDBSubnetGroup(vals)
	case "DescribeDBSubnetGroups":

		return h.handleDescribeDBSubnetGroups(vals)
	case "DeleteDBSubnetGroup":

		return h.handleDeleteDBSubnetGroup(vals)
	case "ListTagsForResource":

		return h.handleListTagsForResource(vals)
	case "AddTagsToResource":

		return h.handleAddTagsToResource(vals)
	case "RemoveTagsFromResource":

		return h.handleRemoveTagsFromResource(vals)
	default:

		return h.dispatchExtended(action, vals)
	}
}

// dispatchExtended routes the first subset of extended RDS actions (parameter groups, option groups,
// and cluster CRUD) to the appropriate handler. See dispatchExtended2 for the remaining actions.
func (h *Handler) dispatchExtended(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateDBParameterGroup":

		return h.handleCreateDBParameterGroup(vals)
	case "DescribeDBParameterGroups":

		return h.handleDescribeDBParameterGroups(vals)
	case "DeleteDBParameterGroup":

		return h.handleDeleteDBParameterGroup(vals)
	case "ModifyDBParameterGroup":

		return h.handleModifyDBParameterGroup(vals)
	case "DescribeDBParameters":

		return h.handleDescribeDBParameters(vals)
	case "ResetDBParameterGroup":

		return h.handleResetDBParameterGroup(vals)
	case "CreateOptionGroup":

		return h.handleCreateOptionGroup(vals)
	case "DescribeOptionGroups":

		return h.handleDescribeOptionGroups(vals)
	case "DeleteOptionGroup":

		return h.handleDeleteOptionGroup(vals)
	case "ModifyOptionGroup":

		return h.handleModifyOptionGroup(vals)
	case "DescribeOptionGroupOptions":

		return h.handleDescribeOptionGroupOptions(vals)
	case "CreateDBCluster":

		return h.handleCreateDBCluster(vals)
	case "DescribeDBClusters":

		return h.handleDescribeDBClusters(vals)
	default:

		return h.dispatchExtended2(action, vals)
	}
}

// dispatchExtended2 routes the remaining extended RDS actions (cluster modifications, snapshots,
// replicas, and misc operations) to the appropriate handler. Split from dispatchExtended to keep
// cyclomatic complexity within limits.
func (h *Handler) dispatchExtended2(action string, vals url.Values) (any, error) {
	switch action {
	case "DeleteDBCluster":

		return h.handleDeleteDBCluster(vals)
	case "ModifyDBCluster":

		return h.handleModifyDBCluster(vals)
	case "CreateDBClusterParameterGroup":

		return h.handleCreateDBClusterParameterGroup(vals)
	case "DescribeDBClusterParameterGroups":

		return h.handleDescribeDBClusterParameterGroups(vals)
	case "CreateDBClusterSnapshot":

		return h.handleCreateDBClusterSnapshot(vals)
	case "DescribeDBClusterSnapshots":

		return h.handleDescribeDBClusterSnapshots(vals)
	case "CreateDBInstanceReadReplica":

		return h.handleCreateDBInstanceReadReplica(vals)
	case "PromoteReadReplica":

		return h.handlePromoteReadReplica(vals)
	case "RebootDBInstance":

		return h.handleRebootDBInstance(vals)
	case "DescribeDBEngineVersions":

		return h.handleDescribeDBEngineVersions(vals)
	case "DescribeOrderableDBInstanceOptions":

		return h.handleDescribeOrderableDBInstanceOptions(vals)
	case "DescribeDBLogFiles":

		return h.handleDescribeDBLogFiles(vals)
	case "DownloadDBLogFilePortion":

		return h.handleDownloadDBLogFilePortion(vals)
	default:

		return h.dispatchExtended3(action, vals)
	}
}

// dispatchExtended3 routes the next set of extended RDS actions (cluster start/stop, snapshot
// management, and restore operations). Split from dispatchExtended2 to keep cyclomatic complexity
// within limits.
func (h *Handler) dispatchExtended3(action string, vals url.Values) (any, error) {
	switch action {
	case opDescribeGlobalClusters:

		return h.handleDescribeGlobalClusters(vals)
	case "StartDBCluster":

		return h.handleStartDBCluster(vals)
	case "StopDBCluster":

		return h.handleStopDBCluster(vals)
	case "DeleteDBClusterSnapshot":

		return h.handleDeleteDBClusterSnapshot(vals)
	case "RestoreDBClusterFromSnapshot":

		return h.handleRestoreDBClusterFromSnapshot(vals)
	case "RestoreDBClusterToPointInTime":

		return h.handleRestoreDBClusterToPointInTime(vals)
	case "CopyDBClusterSnapshot":

		return h.handleCopyDBClusterSnapshot(vals)
	default:

		return h.dispatchExtended4(action, vals)
	}
}

// dispatchExtended4 routes the final set of extended RDS actions (cluster endpoints, export tasks,
// and instance modification validation). Split from dispatchExtended3 to keep cyclomatic complexity
// within limits.
func (h *Handler) dispatchExtended4(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateDBClusterEndpoint":

		return h.handleCreateDBClusterEndpoint(vals)
	case "DescribeDBClusterEndpoints":

		return h.handleDescribeDBClusterEndpoints(vals)
	case "DeleteDBClusterEndpoint":

		return h.handleDeleteDBClusterEndpoint(vals)
	case "DescribeValidDBInstanceModifications":

		return h.handleDescribeValidDBInstanceModifications(vals)
	case "StartExportTask":

		return h.handleStartExportTask(vals)
	case "DescribeExportTasks":

		return h.handleDescribeExportTasks(vals)
	case "CancelExportTask":

		return h.handleCancelExportTask(vals)
	default:

		return h.dispatchExtended5(action, vals)
	}
}

// dispatchExtended5 routes instance restore, snapshot copy, start/stop, and global cluster operations.
// Split from dispatchExtended4 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended5(action string, vals url.Values) (any, error) {
	switch action {
	case "RestoreDBInstanceFromDBSnapshot":

		return h.handleRestoreDBInstanceFromDBSnapshot(vals)
	case "RestoreDBInstanceToPointInTime":

		return h.handleRestoreDBInstanceToPointInTime(vals)
	case "CopyDBSnapshot":

		return h.handleCopyDBSnapshot(vals)
	case "StartDBInstance":

		return h.handleStartDBInstance(vals)
	case "StopDBInstance":

		return h.handleStopDBInstance(vals)
	case "CreateGlobalCluster":

		return h.handleCreateGlobalCluster(vals)
	case opDescribeGlobalClusters:

		return h.handleDescribeGlobalClusters(vals)
	case "DeleteGlobalCluster":

		return h.handleDeleteGlobalCluster(vals)
	case "ModifyGlobalCluster":

		return h.handleModifyGlobalCluster(vals)
	default:

		return h.dispatchExtended6(action, vals)
	}
}

// dispatchExtended6 routes the new RDS operations added in this iteration.
// Split from dispatchExtended5 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended6(action string, vals url.Values) (any, error) {
	switch action {
	case "AddRoleToDBCluster":

		return h.handleAddRoleToDBCluster(vals)
	case "AddRoleToDBInstance":

		return h.handleAddRoleToDBInstance(vals)
	case "AddSourceIdentifierToSubscription":

		return h.handleAddSourceIdentifierToSubscription(vals)
	case "ApplyPendingMaintenanceAction":

		return h.handleApplyPendingMaintenanceAction(vals)
	case "AuthorizeDBSecurityGroupIngress":

		return h.handleAuthorizeDBSecurityGroupIngress(vals)
	case "BacktrackDBCluster":

		return h.handleBacktrackDBCluster(vals)
	case "CopyDBClusterParameterGroup":

		return h.handleCopyDBClusterParameterGroup(vals)
	case "CopyDBParameterGroup":

		return h.handleCopyDBParameterGroup(vals)
	case "CopyOptionGroup":

		return h.handleCopyOptionGroup(vals)
	case "CreateBlueGreenDeployment":

		return h.handleCreateBlueGreenDeployment(vals)
	case "RemoveRoleFromDBCluster":

		return h.handleRemoveRoleFromDBCluster(vals)
	case "RemoveRoleFromDBInstance":

		return h.handleRemoveRoleFromDBInstance(vals)
	case "RemoveSourceIdentifierFromSubscription":

		return h.handleRemoveSourceIdentifierFromSubscription(vals)
	default:

		return h.dispatchExtended7(action, vals)
	}
}

func (h *Handler) handleOpError(c *echo.Context, action string, opErr error) error {
	statusCode := http.StatusBadRequest
	code := rdsErrorCode(opErr)

	if code == "" {
		code = "InternalFailure"
		statusCode = http.StatusInternalServerError
		logger.Load(c.Request().Context()).Error("RDS internal error", "error", opErr, "action", action)
	}

	return h.writeError(c, statusCode, code, opErr.Error())
}

func rdsErrorCode(opErr error) string {
	type errorMapping struct {
		sentinel error
		code     string
	}

	mappings := []errorMapping{
		{ErrInstanceNotFound, "DBInstanceNotFound"},
		{ErrInstanceAlreadyExists, "DBInstanceAlreadyExists"},
		{ErrSnapshotNotFound, "DBSnapshotNotFound"},
		{ErrSnapshotAlreadyExists, "DBSnapshotAlreadyExists"},
		{ErrSubnetGroupNotFound, "DBSubnetGroupNotFoundFault"},
		{ErrSubnetGroupAlreadyExists, "DBSubnetGroupAlreadyExists"},
		{ErrInvalidParameter, "InvalidParameterValue"},
		{ErrInvalidParameterCombination, "InvalidParameterCombination"},
		{ErrUnknownAction, "InvalidAction"},
		{ErrInvalidDBInstanceState, "InvalidDBInstanceState"},
		{ErrResourceNotFound, "ResourceNotFoundFault"},
		{ErrCustomDBEngineVersionNotFound, "CustomDBEngineVersionNotFoundFault"},
		{ErrCustomDBEngineVersionAlreadyExists, "CustomDBEngineVersionAlreadyExistsFault"},
		{ErrParameterGroupNotFound, "DBParameterGroupNotFound"},
		{ErrParameterGroupAlreadyExists, "DBParameterGroupAlreadyExists"},
		{ErrOptionGroupNotFound, "OptionGroupNotFoundFault"},
		{ErrOptionGroupAlreadyExists, "OptionGroupAlreadyExistsFault"},
		{ErrClusterNotFound, "DBClusterNotFoundFault"},
		{ErrClusterAlreadyExists, "DBClusterAlreadyExistsFault"},
		{ErrClusterSnapshotNotFound, "DBClusterSnapshotNotFoundFault"},
		{ErrClusterSnapshotAlreadyExists, "DBClusterSnapshotAlreadyExistsFault"},
		{ErrClusterEndpointNotFound, "DBClusterEndpointNotFoundFault"},
		{ErrClusterEndpointAlreadyExists, "DBClusterEndpointAlreadyExistsFault"},
		{ErrExportTaskNotFound, "ExportTaskNotFound"},
		{ErrExportTaskAlreadyExists, "ExportTaskAlreadyExists"},
		{ErrGlobalClusterNotFound, "GlobalClusterNotFoundFault"},
		{ErrGlobalClusterAlreadyExists, "GlobalClusterAlreadyExistsFault"},
		{ErrInvalidDBClusterStateFault, errCodeInvalidDBClusterStateFault},
		{ErrInvalidGlobalClusterState, "InvalidGlobalClusterStateFault"},
		{ErrEventSubscriptionNotFound, "SubscriptionNotFound"},
		{ErrEventSubscriptionAlreadyExists, "SubscriptionAlreadyExist"},
		{ErrDBSecurityGroupNotFound, "DBSecurityGroupNotFound"},
		{ErrDBSecurityGroupAlreadyExists, "DBSecurityGroupAlreadyExists"},
		{ErrDBSecurityGroupInvalidState, "InvalidDBSecurityGroupState"},
		{ErrBlueGreenDeploymentNotFound, "BlueGreenDeploymentNotFoundFault"},
		{ErrBlueGreenDeploymentAlreadyExists, "BlueGreenDeploymentAlreadyExistsFault"},
		{ErrDBShardGroupNotFound, "DBShardGroupNotFound"},
		{ErrDBShardGroupAlreadyExists, "DBShardGroupAlreadyExists"},
		{ErrIntegrationNotFound, "IntegrationNotFoundFault"},
		{ErrIntegrationAlreadyExists, "IntegrationAlreadyExistsFault"},
		{ErrTenantDatabaseNotFound, "TenantDatabaseNotFound"},
		{ErrTenantDatabaseAlreadyExists, "TenantDatabaseAlreadyExists"},
		{ErrDBClusterAutomatedBackupNotFound, "DBClusterAutomatedBackupNotFoundFault"},
		{ErrDBInstanceAutomatedBackupNotFound, "DBInstanceAutomatedBackupNotFound"},
		{ErrActivityStreamAlreadyStarted, errCodeInvalidDBClusterStateFault},
		{ErrActivityStreamNotStarted, errCodeInvalidDBClusterStateFault},
		{ErrDBProxyAlreadyExists, "DBProxyAlreadyExistsFault"},
		{ErrDBProxyEndpointAlreadyExists, "DBProxyEndpointAlreadyExistsFault"},
		{ErrCannotDeleteDefaultProxyEndpoint, "InvalidDBProxyEndpointStateFault"},
	}

	for _, m := range mappings {
		if errors.Is(opErr, m.sentinel) {
			return m.code
		}
	}

	return ""
}

func (h *Handler) writeError(c *echo.Context, statusCode int, code, message string) error {
	errResp := &rdsErrorResponse{
		Xmlns: rdsXMLNS,
		Error: rdsError{Code: code, Message: message, Type: "Sender"},
	}

	xmlBytes, err := marshalXML(errResp)
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	return c.Blob(statusCode, "text/xml", xmlBytes)
}

type rdsError struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
	Type    string `xml:"Type"`
}

type rdsErrorResponse struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
	Error   rdsError `xml:"Error"`
}

// dispatchExtended16 routes batch-3 operations: DescribeServerlessV2PlatformVersions.
//
// DescribeCustomDBEngineVersions is deliberately not routed here: it is not a real RDS
// operation. Real AWS has no separate "describe custom engine versions" call -- custom
// engine versions are returned by DescribeDBEngineVersions like any other engine/version
// pair (see the DescribeDBEngineVersions doc comment in engine_versions.go). Called from
// the default branch of dispatchExtended15.
func (h *Handler) dispatchExtended16(action string, vals url.Values) (any, error) {
	switch action {
	case "DescribeServerlessV2PlatformVersions":
		return h.handleDescribeServerlessV2PlatformVersions(vals)
	default:
		return nil, fmt.Errorf("%w: %s is not a valid RDS action", ErrUnknownAction, action)
	}
}

// dispatchExtended7 routes the new RDS operations added in refinement 1.
// Split from dispatchExtended6 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended7(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateEventSubscription":
		return h.handleCreateEventSubscription(vals)
	case "DeleteEventSubscription":
		return h.handleDeleteEventSubscription(vals)
	case "DescribeEventSubscriptions":
		return h.handleDescribeEventSubscriptions(vals)
	case "ModifyEventSubscription":
		return h.handleModifyEventSubscription(vals)
	case "DescribeEvents":
		return h.handleDescribeEvents(vals)
	case "DescribeEventCategories":
		return h.handleDescribeEventCategories(vals)
	case "DescribeBlueGreenDeployments":
		return h.handleDescribeBlueGreenDeployments(vals)
	case "DeleteBlueGreenDeployment":
		return h.handleDeleteBlueGreenDeployment(vals)
	case "SwitchoverBlueGreenDeployment":
		return h.handleSwitchoverBlueGreenDeployment(vals)
	case "DescribeDBSecurityGroups":
		return h.handleDescribeDBSecurityGroups(vals)
	default:
		return h.dispatchExtended8(action, vals)
	}
}

// dispatchExtended8 routes additional RDS operations added in refinement 1.
// Split from dispatchExtended7 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended8(action string, vals url.Values) (any, error) {
	switch action {
	case "DeleteDBSecurityGroup":
		return h.handleDeleteDBSecurityGroup(vals)
	case "RevokeDBSecurityGroupIngress":
		return h.handleRevokeDBSecurityGroupIngress(vals)
	case "FailoverDBCluster":
		return h.handleFailoverDBCluster(vals)
	case "RebootDBCluster":
		return h.handleRebootDBCluster(vals)
	case "DeleteDBClusterParameterGroup":
		return h.handleDeleteDBClusterParameterGroup(vals)
	case "ModifyDBClusterParameterGroup":
		return h.handleModifyDBClusterParameterGroup(vals)
	case "DescribeDBClusterParameters":
		return h.handleDescribeDBClusterParameters(vals)
	case "ResetDBClusterParameterGroup":
		return h.handleResetDBClusterParameterGroup(vals)
	case "ModifyDBSubnetGroup":
		return h.handleModifyDBSubnetGroup(vals)
	case "ModifyDBClusterEndpoint":
		return h.handleModifyDBClusterEndpoint(vals)
	default:
		return h.dispatchExtended9(action, vals)
	}
}

// dispatchExtended9 routes RDS operations added in refinement 2.
// Split from dispatchExtended8 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended9(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateDBSecurityGroup":
		return h.handleCreateDBSecurityGroup(vals)
	case "RemoveFromGlobalCluster":
		return h.handleRemoveFromGlobalCluster(vals)
	case "FailoverGlobalCluster":
		return h.handleFailoverGlobalCluster(vals)
	case "SwitchoverGlobalCluster":
		return h.handleSwitchoverGlobalCluster(vals)
	case "SwitchoverReadReplica":
		return h.handleSwitchoverReadReplica(vals)
	case "PromoteReadReplicaDBCluster":
		return h.handlePromoteReadReplicaDBCluster(vals)
	case "DescribeAccountAttributes":
		return h.handleDescribeAccountAttributes(vals)
	case "DescribeCertificates":
		return h.handleDescribeCertificates(vals)
	case "ModifyCertificates":
		return h.handleModifyCertificates(vals)
	case "DescribePendingMaintenanceActions":
		return h.handleDescribePendingMaintenanceActions(vals)
	default:
		return h.dispatchExtended10(action, vals)
	}
}

// dispatchExtended10 routes additional RDS operations added in refinement 2.
// Split from dispatchExtended9 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended10(action string, vals url.Values) (any, error) {
	switch action {
	case "DescribeSourceRegions":
		return h.handleDescribeSourceRegions(vals)
	case "DescribeDBMajorEngineVersions":
		return h.handleDescribeDBMajorEngineVersions(vals)
	case "DescribeEngineDefaultParameters":
		return h.handleDescribeEngineDefaultParameters(vals)
	case "DescribeEngineDefaultClusterParameters":
		return h.handleDescribeEngineDefaultClusterParameters(vals)
	case "DescribeDBSnapshotAttributes":
		return h.handleDescribeDBSnapshotAttributes(vals)
	case "ModifyDBSnapshot":
		return h.handleModifyDBSnapshot(vals)
	case "ModifyDBSnapshotAttribute":
		return h.handleModifyDBSnapshotAttribute(vals)
	case "DescribeDBClusterSnapshotAttributes":
		return h.handleDescribeDBClusterSnapshotAttributes(vals)
	case "ModifyDBClusterSnapshotAttribute":
		return h.handleModifyDBClusterSnapshotAttribute(vals)
	case "DescribeDBClusterBacktracks":
		return h.handleDescribeDBClusterBacktracks(vals)
	case "EnableHttpEndpoint":
		return h.handleEnableHTTPEndpoint(vals)
	case "DisableHttpEndpoint":
		return h.handleDisableHTTPEndpoint(vals)
	case "ModifyCurrentDBClusterCapacity":
		return h.handleModifyCurrentDBClusterCapacity(vals)
	default:
		return h.dispatchExtended11(action, vals)
	}
}

// dispatchExtended11 routes remaining RDS operations added in refinement 2.
// Split from dispatchExtended10 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended11(action string, vals url.Values) (any, error) {
	switch action {
	case "RestoreDBInstanceFromS3":
		return h.handleRestoreDBInstanceFromS3(vals)
	case "RestoreDBClusterFromS3":
		return h.handleRestoreDBClusterFromS3(vals)
	case "ModifyDBRecommendation":
		return h.handleModifyDBRecommendation(vals)
	case "DescribeDBRecommendations":
		return h.handleDescribeDBRecommendations(vals)
	case "PurchaseReservedDBInstancesOffering":
		return h.handlePurchaseReservedDBInstancesOffering(vals)
	case "DescribeReservedDBInstances":
		return h.handleDescribeReservedDBInstances(vals)
	case "DescribeReservedDBInstancesOfferings":
		return h.handleDescribeReservedDBInstancesOfferings(vals)
	default:
		return h.dispatchExtended12(action, vals)
	}
}

// dispatchExtended12 routes DB Proxy and Activity Stream operations added in refinement 3.
// Split from dispatchExtended11 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended12(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateDBProxy":
		return h.handleCreateDBProxy(vals)
	case "DeleteDBProxy":
		return h.handleDeleteDBProxy(vals)
	case "DescribeDBProxies":
		return h.handleDescribeDBProxies(vals)
	case "ModifyDBProxy":
		return h.handleModifyDBProxy(vals)
	case "RegisterDBProxyTargets":
		return h.handleRegisterDBProxyTargets(vals)
	case "DeregisterDBProxyTargets":
		return h.handleDeregisterDBProxyTargets(vals)
	case "DescribeDBProxyTargets":
		return h.handleDescribeDBProxyTargets(vals)
	case "DescribeDBProxyTargetGroups":
		return h.handleDescribeDBProxyTargetGroups(vals)
	case "ModifyDBProxyTargetGroup":
		return h.handleModifyDBProxyTargetGroup(vals)
	default:
		return h.dispatchExtended13(action, vals)
	}
}

// dispatchExtended13 routes proxy endpoint and activity stream operations added in refinement 3.
// Split from dispatchExtended12 to keep cyclomatic complexity within limits.
func (h *Handler) dispatchExtended13(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateDBProxyEndpoint":
		return h.handleCreateDBProxyEndpoint(vals)
	case "DeleteDBProxyEndpoint":
		return h.handleDeleteDBProxyEndpoint(vals)
	case "DescribeDBProxyEndpoints":
		return h.handleDescribeDBProxyEndpoints(vals)
	case "ModifyDBProxyEndpoint":
		return h.handleModifyDBProxyEndpoint(vals)
	case "StartActivityStream":
		return h.handleStartActivityStream(vals)
	case "StopActivityStream":
		return h.handleStopActivityStream(vals)
	case "ModifyActivityStream":
		return h.handleModifyActivityStream(vals)
	default:
		return h.dispatchExtended14(action, vals)
	}
}

// errRDSStubNotHandled is a sentinel used internally to signal that a dispatch
// helper did not recognise the action. It is never returned to callers.
var errRDSStubNotHandled = errors.New("rds: action not handled by this dispatcher")

// dispatchExtended14 routes the stub RDS operations added for SDK completeness.
func (h *Handler) dispatchExtended14(action string, vals url.Values) (any, error) {
	if r, err := h.dispatchEngineShardOps(action, vals); !errors.Is(err, errRDSStubNotHandled) {
		return r, err
	}

	if r, err := h.dispatchIntegrationTenantOps(action, vals); !errors.Is(err, errRDSStubNotHandled) {
		return r, err
	}

	if r, err := h.dispatchAutomatedBackupOps(action, vals); !errors.Is(err, errRDSStubNotHandled) {
		return r, err
	}

	return nil, fmt.Errorf("%w: %s is not a valid RDS action", ErrUnknownAction, action)
}

// dispatchEngineShardOps handles custom engine version and shard group operations.
func (h *Handler) dispatchEngineShardOps(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateCustomDBEngineVersion":

		return h.handleCreateCustomDBEngineVersion(vals)
	case "DeleteCustomDBEngineVersion":

		return h.handleDeleteCustomDBEngineVersion(vals)
	case "ModifyCustomDBEngineVersion":

		return h.handleModifyCustomDBEngineVersion(vals)
	case "CreateDBShardGroup":

		return h.handleCreateDBShardGroup(vals)
	case "DeleteDBShardGroup":

		return h.handleDeleteDBShardGroup(vals)
	case "DescribeDBShardGroups":

		return h.handleDescribeDBShardGroups(vals)
	case "ModifyDBShardGroup":

		return h.handleModifyDBShardGroup(vals)
	case "RebootDBShardGroup":

		return h.handleRebootDBShardGroup(vals)
	}

	return nil, errRDSStubNotHandled
}

// dispatchIntegrationTenantOps handles integration and tenant database operations.
func (h *Handler) dispatchIntegrationTenantOps(action string, vals url.Values) (any, error) {
	switch action {
	case "CreateIntegration":

		return h.handleCreateIntegration(vals)
	case "DeleteIntegration":

		return h.handleDeleteIntegration(vals)
	case "DescribeIntegrations":

		return h.handleDescribeIntegrations(vals)
	case "ModifyIntegration":

		return h.handleModifyIntegration(vals)
	case "CreateTenantDatabase":

		return h.handleCreateTenantDatabase(vals)
	case "DeleteTenantDatabase":

		return h.handleDeleteTenantDatabase(vals)
	case "DescribeTenantDatabases":

		return h.handleDescribeTenantDatabases(vals)
	case "ModifyTenantDatabase":

		return h.handleModifyTenantDatabase(vals)
	}

	return nil, errRDSStubNotHandled
}

// dispatchAutomatedBackupOps handles automated backup and snapshot tenant database operations.
func (h *Handler) dispatchAutomatedBackupOps(action string, vals url.Values) (any, error) {
	switch action {
	case "DeleteDBClusterAutomatedBackup":

		return h.handleDeleteDBClusterAutomatedBackup(vals)
	case "DeleteDBInstanceAutomatedBackup":

		return h.handleDeleteDBInstanceAutomatedBackup(vals)
	case "DescribeDBClusterAutomatedBackups":

		return h.handleDescribeDBClusterAutomatedBackups(vals)
	case "DescribeDBInstanceAutomatedBackups":

		return h.handleDescribeDBInstanceAutomatedBackups(vals)
	case "StartDBInstanceAutomatedBackupsReplication":

		return h.handleStartDBInstanceAutomatedBackupsReplication(vals)
	case "StopDBInstanceAutomatedBackupsReplication":

		return h.handleStopDBInstanceAutomatedBackupsReplication(vals)
	case "DescribeDBSnapshotTenantDatabases":

		return h.handleDescribeDBSnapshotTenantDatabases(vals)
	default:
		return h.dispatchExtended15(action, vals)
	}
}

// dispatchExtended15 routes Performance Insights and any future stub operations.
func (h *Handler) dispatchExtended15(action string, vals url.Values) (any, error) {
	switch action {
	case "GetPerformanceInsightsMetrics":
		return h.handleGetPerformanceInsightsMetricsReal(vals)
	default:
		return h.dispatchExtended16(action, vals)
	}
}
