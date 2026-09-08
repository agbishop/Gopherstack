package redshift

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	statusDisabled     = "disabled"
	keyResourceCluster = "cluster"

	// Operation name constants shared across handler files.
	opCreateUsageLimit              = "CreateUsageLimit"
	opDeleteUsageLimit              = "DeleteUsageLimit"
	opCreateScheduledAction         = "CreateScheduledAction"
	opDeleteScheduledAction         = "DeleteScheduledAction"
	opCreateCustomDomainAssociation = "CreateCustomDomainAssociation"
	opDeleteCustomDomainAssociation = "DeleteCustomDomainAssociation"
	opGetResourcePolicy             = "GetResourcePolicy"
	opPutResourcePolicy             = "PutResourcePolicy"
	opDeleteResourcePolicy          = "DeleteResourcePolicy"
	// opCreateEndpointAccess/opDeleteEndpointAccess are shared with
	// handler_serverless.go: classic Redshift and Redshift Serverless both
	// define real, distinct operations with these exact names.
	opCreateEndpointAccess = "CreateEndpointAccess"
	opDeleteEndpointAccess = "DeleteEndpointAccess"
	// opGetIdentityCenterAuthToken is shared with handler_serverless.go:
	// classic Redshift and Redshift Serverless both define real, distinct
	// operations with this exact name.
	opGetIdentityCenterAuthToken = "GetIdentityCenterAuthToken"
	opUnknown                    = "Unknown"
)

const (
	redshiftVersion = "2012-12-01"
	redshiftXMLNS   = "http://redshift.amazonaws.com/doc/2012-12-01/"
	paramValueTrue  = "true"
)

// Handler is the Echo HTTP handler for Redshift operations.
type Handler struct {
	Backend StorageBackend
	ops     map[string]redshiftActionFn
}

// NewHandler creates a new Redshift handler.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.buildOps()

	return h
}

// Reset clears all backend state and rebuilds the dispatch table.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// StartWorker implements service.BackgroundWorker. It starts the managed cluster
// lifecycle reconciler using the framework-provided background context, so no
// context.Background() is introduced.
func (h *Handler) StartWorker(ctx context.Context) error {
	h.Backend.StartReconciler(ctx)

	return nil
}

// Shutdown implements service.Shutdowner. It stops the reconciler and waits for
// its goroutine to exit, guaranteeing a clean, leak-free shutdown.
func (h *Handler) Shutdown(_ context.Context) {
	h.Backend.StopReconciler()
}

// Ensure Handler satisfies the optional background-lifecycle interfaces.
var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "Redshift" }

// GetSupportedOperations returns supported Redshift operations (sorted).
func (h *Handler) GetSupportedOperations() []string {
	g1 := supportedOpsGroup1()
	g2 := supportedOpsGroup2()
	ops := make([]string, 0, len(g1)+len(g2))
	ops = append(ops, g1...)
	ops = append(ops, g2...)
	sort.Strings(ops)

	return ops
}

func supportedOpsGroup1() []string {
	return []string{
		"AcceptReservedNodeExchange",
		"AddPartner",
		"AssociateDataShareConsumer",
		"AuthorizeClusterSecurityGroupIngress",
		"AuthorizeDataShare",
		"AuthorizeEndpointAccess",
		"AuthorizeSnapshotAccess",
		"BatchDeleteClusterSnapshots",
		"BatchModifyClusterSnapshots",
		"CancelResize",
		"CopyClusterSnapshot",
		"CreateAuthenticationProfile",
		"CreateCluster",
		"CreateClusterParameterGroup",
		"CreateClusterSecurityGroup",
		"CreateClusterSnapshot",
		"CreateClusterSubnetGroup",
		"CreateEventSubscription",
		"CreateSnapshotCopyGrant",
		"CreateSnapshotSchedule",
		"CreateTags",
		opCreateUsageLimit,
		"DeauthorizeDataShare",
		"DeleteAuthenticationProfile",
		"DeleteCluster",
		"DeleteClusterParameterGroup",
		"DeleteClusterSecurityGroup",
		"DeleteClusterSnapshot",
		"DeleteClusterSubnetGroup",
		"DeleteEventSubscription",
		"DeletePartner",
		opDeleteResourcePolicy,
		"DeleteSnapshotCopyGrant",
		"DeleteSnapshotSchedule",
		"DeleteTags",
		opDeleteUsageLimit,
		"DescribeAccountAttributes",
		"DescribeAuthenticationProfiles",
		"DescribeClusterParameterGroups",
		"DescribeClusterParameters",
		"DescribeClusterSecurityGroups",
		"DescribeClusterSnapshots",
		"DescribeClusterSubnetGroups",
		"DescribeClusterTracks",
		"DescribeClusterVersions",
		"DescribeClusters",
		"DescribeDataShares",
		"DescribeDataSharesForConsumer",
		"DescribeDataSharesForProducer",
		"DescribeDefaultClusterParameters",
		"DescribeEndpointAuthorization",
		"DescribeEventCategories",
		"DescribeEventSubscriptions",
		"DescribeEvents",
		"DescribeLoggingStatus",
	}
}

func supportedOpsGroup2() []string {
	return []string{
		"DescribeOrderableClusterOptions",
		"DescribePartners",
		"DescribeReservedNodeExchangeStatus",
		"DescribeReservedNodeOfferings",
		"DescribeReservedNodes",
		"DescribeResize",
		"DescribeSnapshotCopyGrants",
		"DescribeSnapshotSchedules",
		"DescribeStorage",
		"DescribeTableRestoreStatus",
		"DescribeTags",
		"DescribeUsageLimits",
		"DisableLogging",
		"DisableSnapshotCopy",
		"DisassociateDataShareConsumer",
		"EnableLogging",
		"EnableSnapshotCopy",
		"FailoverPrimaryCompute",
		"GetClusterCredentials",
		"GetClusterCredentialsWithIAM",
		"GetReservedNodeExchangeConfigurationOptions",
		"GetReservedNodeExchangeOfferings",
		opGetResourcePolicy,
		"ModifyAuthenticationProfile",
		"ModifyCluster",
		"ModifyClusterIamRoles",
		"ModifyClusterMaintenance",
		"ModifyClusterParameterGroup",
		"ModifyClusterSnapshot",
		"ModifyClusterSnapshotSchedule",
		"ModifyClusterSubnetGroup",
		"ModifyEventSubscription",
		"ModifySnapshotCopyRetentionPeriod",
		"ModifySnapshotSchedule",
		"ModifyUsageLimit",
		"PauseCluster",
		"PurchaseReservedNodeOffering",
		opPutResourcePolicy,
		"RebootCluster",
		"RejectDataShare",
		"ResetClusterParameterGroup",
		"ResizeCluster",
		"RestoreFromClusterSnapshot",
		"ResumeCluster",
		"RevokeClusterSecurityGroupIngress",
		"RevokeEndpointAccess",
		"RevokeSnapshotAccess",
		"RotateEncryptionKey",
		"UpdatePartnerStatus",
		// Completeness pass — previously notImplemented
		opCreateCustomDomainAssociation,
		opCreateEndpointAccess,
		"CreateHsmClientCertificate",
		"CreateHsmConfiguration",
		"CreateIntegration",
		"CreateQev2IdcApplication",
		"CreateRedshiftIdcApplication",
		opCreateScheduledAction,
		opDeleteCustomDomainAssociation,
		opDeleteEndpointAccess,
		"DeleteHsmClientCertificate",
		"DeleteHsmConfiguration",
		"DeleteIntegration",
		"DeleteQev2IdcApplication",
		"DeleteRedshiftIdcApplication",
		opDeleteScheduledAction,
		"DeregisterNamespace",
		"DescribeClusterDbRevisions",
		"DescribeCustomDomainAssociations",
		"DescribeEndpointAccess",
		"DescribeHsmClientCertificates",
		"DescribeHsmConfigurations",
		"DescribeInboundIntegrations",
		"DescribeIntegrations",
		"DescribeNodeConfigurationOptions",
		"DescribeQev2IdcApplications",
		"DescribeRedshiftIdcApplications",
		"DescribeScheduledActions",
		opGetIdentityCenterAuthToken,
		"ListRecommendations",
		"ModifyAquaConfiguration",
		"ModifyClusterDbRevision",
		"ModifyCustomDomainAssociation",
		"ModifyEndpointAccess",
		"ModifyIntegration",
		"ModifyLakehouseConfiguration",
		"ModifyQev2IdcApplication",
		"ModifyRedshiftIdcApplication",
		"ModifyScheduledAction",
		"RegisterNamespace",
		"RestoreTableFromClusterSnapshot",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "redshift" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Redshift instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches Redshift requests.
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
			// marker every aws-sdk-go-v2 redshift client sets (api_client.go's
			// AddSDKAgentKeyValue -- "api/redshift"). That still identifies
			// this as ours, so claim it and let Handler() produce the typed
			// error instead of masking the read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/redshift")
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == redshiftVersion
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityFormRedshift }

// ExtractOperation extracts the Redshift action from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return opUnknown
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return opUnknown
	}
	action := vals.Get("Action")
	if action == "" {
		return opUnknown
	}

	return action
}

// ExtractResource returns the cluster identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}

	return vals.Get("ClusterIdentifier")
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
			return h.writeError(c, http.StatusInternalServerError, "InternalFailure", "failed to parse request body")
		}
		action := vals.Get("Action")
		if action == "" {
			return h.writeError(c, http.StatusBadRequest, "MissingAction", "missing Action parameter")
		}

		return h.dispatch(c, action, vals)
	}
}

type redshiftActionFn func(vals url.Values) (any, error)

func (h *Handler) buildOps() map[string]redshiftActionFn {
	ops := h.buildOpsGroup1()
	maps.Copy(ops, h.buildOpsGroup2())
	maps.Copy(ops, h.buildOpsGroup3())

	return ops
}

func (h *Handler) buildOpsGroup1() map[string]redshiftActionFn {
	return map[string]redshiftActionFn{
		"AcceptReservedNodeExchange":           h.handleAcceptReservedNodeExchange,
		"AddPartner":                           h.handleAddPartner,
		"AssociateDataShareConsumer":           h.handleAssociateDataShareConsumer,
		"AuthorizeClusterSecurityGroupIngress": h.handleAuthorizeClusterSecurityGroupIngress,
		"AuthorizeDataShare":                   h.handleAuthorizeDataShare,
		"AuthorizeEndpointAccess":              h.handleAuthorizeEndpointAccess,
		"AuthorizeSnapshotAccess":              h.handleAuthorizeSnapshotAccess,
		"BatchDeleteClusterSnapshots":          h.handleBatchDeleteClusterSnapshots,
		"BatchModifyClusterSnapshots":          h.handleBatchModifyClusterSnapshots,
		"CancelResize":                         h.handleCancelResize,
		"CopyClusterSnapshot":                  h.handleCopyClusterSnapshot,
		"CreateAuthenticationProfile":          h.handleCreateAuthenticationProfile,
		"CreateCluster":                        h.handleCreateCluster,
		"CreateClusterParameterGroup":          h.handleCreateClusterParameterGroup,
		"CreateClusterSecurityGroup":           h.handleCreateClusterSecurityGroup,
		"CreateClusterSnapshot":                h.handleCreateClusterSnapshot,
		"CreateClusterSubnetGroup":             h.handleCreateClusterSubnetGroup,
		"CreateEventSubscription":              h.handleCreateEventSubscription,
		"CreateSnapshotCopyGrant":              h.handleCreateSnapshotCopyGrant,
		"CreateSnapshotSchedule":               h.handleCreateSnapshotSchedule,
		"CreateTags":                           h.handleCreateTags,
		opCreateUsageLimit:                     h.handleCreateUsageLimit,
		"DeauthorizeDataShare":                 h.handleDeauthorizeDataShare,
		"DeleteAuthenticationProfile":          h.handleDeleteAuthenticationProfile,
		"DeleteCluster":                        h.handleDeleteCluster,
		"DeleteClusterParameterGroup":          h.handleDeleteClusterParameterGroup,
		"DeleteClusterSecurityGroup":           h.handleDeleteClusterSecurityGroup,
		"DeleteClusterSnapshot":                h.handleDeleteClusterSnapshot,
		"DeleteClusterSubnetGroup":             h.handleDeleteClusterSubnetGroup,
		"DeleteEventSubscription":              h.handleDeleteEventSubscription,
		"DeletePartner":                        h.handleDeletePartner,
		opDeleteResourcePolicy:                 h.handleDeleteResourcePolicy,
		"DeleteSnapshotCopyGrant":              h.handleDeleteSnapshotCopyGrant,
		"DeleteSnapshotSchedule":               h.handleDeleteSnapshotSchedule,
		"DeleteTags":                           h.handleDeleteTags,
		opDeleteUsageLimit:                     h.handleDeleteUsageLimit,
		"DescribeAccountAttributes":            h.handleDescribeAccountAttributes,
		"DescribeAuthenticationProfiles":       h.handleDescribeAuthenticationProfiles,
		"DescribeClusterParameterGroups":       h.handleDescribeClusterParameterGroups,
		"DescribeClusterParameters":            h.handleDescribeClusterParameters,
		"DescribeClusterSecurityGroups":        h.handleDescribeClusterSecurityGroups,
		"DescribeClusterSnapshots":             h.handleDescribeClusterSnapshots,
		"DescribeClusterSubnetGroups":          h.handleDescribeClusterSubnetGroups,
		"DescribeClusterTracks":                h.handleDescribeClusterTracks,
		"DescribeClusterVersions":              h.handleDescribeClusterVersions,
		"DescribeClusters":                     h.handleDescribeClusters,
		"DescribeDataShares":                   h.handleDescribeDataShares,
		"DescribeDataSharesForConsumer":        h.handleDescribeDataSharesForConsumer,
		"DescribeDataSharesForProducer":        h.handleDescribeDataSharesForProducer,
		"DescribeDefaultClusterParameters":     h.handleDescribeDefaultClusterParameters,
		"DescribeEndpointAuthorization":        h.handleDescribeEndpointAuthorization,
		"DescribeEventCategories":              h.handleDescribeEventCategories,
		"DescribeEventSubscriptions":           h.handleDescribeEventSubscriptions,
	}
}

func (h *Handler) buildOpsGroup2() map[string]redshiftActionFn {
	return map[string]redshiftActionFn{
		"DescribeEvents":                              h.handleDescribeEvents,
		"DescribeLoggingStatus":                       h.handleDescribeLoggingStatus,
		"DescribeOrderableClusterOptions":             h.handleDescribeOrderableClusterOptions,
		"DescribePartners":                            h.handleDescribePartners,
		"DescribeReservedNodeExchangeStatus":          h.handleDescribeReservedNodeExchangeStatus,
		"DescribeReservedNodeOfferings":               h.handleDescribeReservedNodeOfferings,
		"DescribeReservedNodes":                       h.handleDescribeReservedNodes,
		"DescribeResize":                              h.handleDescribeResize,
		"DescribeSnapshotCopyGrants":                  h.handleDescribeSnapshotCopyGrants,
		"DescribeSnapshotSchedules":                   h.handleDescribeSnapshotSchedules,
		"DescribeStorage":                             h.handleDescribeStorage,
		"DescribeTableRestoreStatus":                  h.handleDescribeTableRestoreStatus,
		"DescribeTags":                                h.handleDescribeTags,
		"DescribeUsageLimits":                         h.handleDescribeUsageLimits,
		"DisableLogging":                              h.handleDisableLogging,
		"DisableSnapshotCopy":                         h.handleDisableSnapshotCopy,
		"DisassociateDataShareConsumer":               h.handleDisassociateDataShareConsumer,
		"EnableLogging":                               h.handleEnableLogging,
		"EnableSnapshotCopy":                          h.handleEnableSnapshotCopy,
		"FailoverPrimaryCompute":                      h.handleFailoverPrimaryCompute,
		"GetClusterCredentials":                       h.handleGetClusterCredentials,
		"GetClusterCredentialsWithIAM":                h.handleGetClusterCredentialsWithIAM,
		opGetResourcePolicy:                           h.handleGetResourcePolicy,
		"GetReservedNodeExchangeConfigurationOptions": h.handleGetReservedNodeExchangeConfigurationOptions,
		"GetReservedNodeExchangeOfferings":            h.handleGetReservedNodeExchangeOfferings,
		"ModifyAuthenticationProfile":                 h.handleModifyAuthenticationProfile,
		"ModifyCluster":                               h.handleModifyCluster,
		"ModifyClusterIamRoles":                       h.handleModifyClusterIamRoles,
		"ModifyClusterMaintenance":                    h.handleModifyClusterMaintenance,
		"ModifyClusterParameterGroup":                 h.handleModifyClusterParameterGroup,
		"ModifyClusterSnapshot":                       h.handleModifyClusterSnapshot,
		"ModifyClusterSnapshotSchedule":               h.handleModifyClusterSnapshotSchedule,
		"ModifyClusterSubnetGroup":                    h.handleModifyClusterSubnetGroup,
		"ModifyEventSubscription":                     h.handleModifyEventSubscription,
		"ModifySnapshotCopyRetentionPeriod":           h.handleModifySnapshotCopyRetentionPeriod,
		"ModifySnapshotSchedule":                      h.handleModifySnapshotSchedule,
		"ModifyUsageLimit":                            h.handleModifyUsageLimit,
		"PauseCluster":                                h.handlePauseCluster,
		"PurchaseReservedNodeOffering":                h.handlePurchaseReservedNodeOffering,
		opPutResourcePolicy:                           h.handlePutResourcePolicy,
		"RebootCluster":                               h.handleRebootCluster,
		"RejectDataShare":                             h.handleRejectDataShare,
		"ResetClusterParameterGroup":                  h.handleResetClusterParameterGroup,
		"ResizeCluster":                               h.handleResizeCluster,
		"RestoreFromClusterSnapshot":                  h.handleRestoreFromClusterSnapshot,
		"ResumeCluster":                               h.handleResumeCluster,
		"RevokeClusterSecurityGroupIngress":           h.handleRevokeClusterSecurityGroupIngress,
		"RevokeEndpointAccess":                        h.handleRevokeEndpointAccess,
		"RevokeSnapshotAccess":                        h.handleRevokeSnapshotAccess,
		"RotateEncryptionKey":                         h.handleRotateEncryptionKey,
		"UpdatePartnerStatus":                         h.handleUpdatePartnerStatus,
	}
}

// buildOpsGroup3 returns dispatch entries for the remaining operation
// families: custom domains, endpoint access, HSM, integrations, IDC
// applications, scheduled actions, and cluster/namespace informational ops.
func (h *Handler) buildOpsGroup3() map[string]redshiftActionFn {
	return map[string]redshiftActionFn{
		opCreateCustomDomainAssociation:    h.handleCreateCustomDomainAssociation,
		opCreateEndpointAccess:             h.handleCreateEndpointAccess,
		"CreateHsmClientCertificate":       h.handleCreateHsmClientCertificate,
		"CreateHsmConfiguration":           h.handleCreateHsmConfiguration,
		"CreateIntegration":                h.handleCreateIntegration,
		"CreateQev2IdcApplication":         h.handleCreateQev2IdcApplication,
		"CreateRedshiftIdcApplication":     h.handleCreateIdcApplication,
		opCreateScheduledAction:            h.handleCreateScheduledAction,
		opDeleteCustomDomainAssociation:    h.handleDeleteCustomDomainAssociation,
		opDeleteEndpointAccess:             h.handleDeleteEndpointAccess,
		"DeleteHsmClientCertificate":       h.handleDeleteHsmClientCertificate,
		"DeleteHsmConfiguration":           h.handleDeleteHsmConfiguration,
		"DeleteIntegration":                h.handleDeleteIntegration,
		"DeleteQev2IdcApplication":         h.handleDeleteQev2IdcApplication,
		"DeleteRedshiftIdcApplication":     h.handleDeleteIdcApplication,
		opDeleteScheduledAction:            h.handleDeleteScheduledAction,
		"DeregisterNamespace":              h.handleDeregisterNamespace,
		"DescribeClusterDbRevisions":       h.handleDescribeClusterDBRevisions,
		"DescribeCustomDomainAssociations": h.handleDescribeCustomDomainAssociations,
		"DescribeEndpointAccess":           h.handleDescribeEndpointAccess,
		"DescribeHsmClientCertificates":    h.handleDescribeHsmClientCertificates,
		"DescribeHsmConfigurations":        h.handleDescribeHsmConfigurations,
		"DescribeInboundIntegrations":      h.handleDescribeInboundIntegrations,
		"DescribeIntegrations":             h.handleDescribeIntegrations,
		"DescribeNodeConfigurationOptions": h.handleDescribeNodeConfigurationOptions,
		"DescribeQev2IdcApplications":      h.handleDescribeQev2IdcApplications,
		"DescribeRedshiftIdcApplications":  h.handleDescribeIdcApplications,
		"DescribeScheduledActions":         h.handleDescribeScheduledActions,
		opGetIdentityCenterAuthToken:       h.handleGetIdentityCenterAuthToken,
		"ListRecommendations":              h.handleListRecommendations,
		"ModifyAquaConfiguration":          h.handleModifyAquaConfiguration,
		"ModifyClusterDbRevision":          h.handleModifyClusterDBRevision,
		"ModifyCustomDomainAssociation":    h.handleModifyCustomDomainAssociation,
		"ModifyEndpointAccess":             h.handleModifyEndpointAccess,
		"ModifyIntegration":                h.handleModifyIntegration,
		"ModifyLakehouseConfiguration":     h.handleModifyLakehouseConfiguration,
		"ModifyQev2IdcApplication":         h.handleModifyQev2IdcApplication,
		"ModifyRedshiftIdcApplication":     h.handleModifyIdcApplication,
		"ModifyScheduledAction":            h.handleModifyScheduledAction,
		"RegisterNamespace":                h.handleRegisterNamespace,
		"RestoreTableFromClusterSnapshot":  h.handleRestoreTableFromClusterSnapshot,
	}
}

// dispatch routes the Redshift action to the appropriate handler function.
func (h *Handler) dispatch(c *echo.Context, action string, vals url.Values) error {
	fn, ok := h.ops[action]
	if !ok {
		return h.writeError(c, http.StatusBadRequest, "InvalidAction",
			action+" is not a valid Redshift action")
	}

	resp, opErr := fn(vals)
	if opErr != nil {
		return h.handleOpError(c, action, opErr)
	}

	return h.writeXMLResponse(c, http.StatusOK, resp)
}

func (h *Handler) handleCreateCluster(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	nodeType := vals.Get("NodeType")
	dbName := vals.Get("DBName")
	masterUser := vals.Get("MasterUsername")
	password := vals.Get("MasterUserPassword")
	clusterSecurityGroups := parseStringList(vals, "ClusterSecurityGroups.ClusterSecurityGroupName.")
	clusterParameterGroupName := vals.Get("ClusterParameterGroupName")

	if password != "" {
		if err := validateMasterUserPassword(password); err != nil {
			return nil, err
		}
	}

	cluster, err := h.Backend.CreateCluster(
		id,
		nodeType,
		dbName,
		masterUser,
		clusterSecurityGroups,
		clusterParameterGroupName,
	)
	if err != nil {
		return nil, err
	}

	return &createClusterResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

func (h *Handler) handleDeleteCluster(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	skipFinalStr := vals.Get("SkipFinalClusterSnapshot")
	finalSnapshotID := vals.Get("FinalClusterSnapshotIdentifier")

	// When SkipFinalClusterSnapshot is explicitly "false", enforce AWS snapshot semantics.
	if skipFinalStr == "false" {
		if finalSnapshotID == "" {
			return nil, fmt.Errorf(
				"%w: FinalClusterSnapshotIdentifier is required when SkipFinalClusterSnapshot is false",
				ErrInvalidParameter,
			)
		}

		if _, err := h.Backend.CreateClusterSnapshot(finalSnapshotID, id); err != nil {
			return nil, err
		}
	}

	cluster, err := h.Backend.DeleteCluster(id)
	if err != nil {
		return nil, err
	}

	return &deleteClusterResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

func (h *Handler) handleDescribeClusters(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	tagKeys := parseRedshiftTagKeysAt(vals, "TagKeys.TagKey.")
	tagValues := parseRedshiftTagKeysAt(vals, "TagValues.TagValue.")
	marker := vals.Get("Marker")

	maxRecords := 0
	if s := vals.Get("MaxRecords"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxRecords = n
		}
	}

	clusters, nextMarker, err := h.Backend.DescribeClusters(id, marker, maxRecords, tagKeys, tagValues)
	if err != nil {
		return nil, err
	}

	// Fetch the live tag map once (not once per cluster) to embed each
	// cluster's Tags in its response -- cloneCluster sets Tags=nil so we
	// cannot read tags from the cloned value.
	allTags := h.Backend.DescribeTags()

	members := make([]xmlCluster, 0, len(clusters))

	for _, c := range clusters {
		cp := c
		members = append(members, toXMLClusterWithTags(&cp, allTags[c.ClusterIdentifier]))
	}

	return &describeClustersResponse{
		Xmlns:    redshiftXMLNS,
		Clusters: xmlClusterList{Members: members},
		Marker:   nextMarker,
	}, nil
}

// validateMasterUserPassword enforces AWS CreateCluster password rules.
// Password must be 8-64 printable ASCII chars, contain at least one uppercase letter,
// one lowercase letter, and one digit; must not contain space, /, ", @, ', or \.
func validateMasterUserPassword(password string) error {
	const (
		minLen = 8
		maxLen = 64
	)

	if l := len(password); l < minLen || l > maxLen {
		return fmt.Errorf(
			"%w: MasterUserPassword must be 8–64 characters (got %d)",
			ErrInvalidParameter, l,
		)
	}

	for _, ch := range password {
		switch ch {
		case ' ', '/', '"', '@', '\'', '\\':
			return fmt.Errorf(
				"%w: MasterUserPassword must not contain space, /, \", @, ', or \\",
				ErrInvalidParameter,
			)
		}
	}

	var hasUpper, hasLower, hasDigit bool

	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf(
			"%w: MasterUserPassword must contain at least one uppercase letter, one lowercase letter, and one digit",
			ErrInvalidParameter,
		)
	}

	return nil
}

// toXMLCluster converts a backend Cluster into its wire shape, including the
// Tags list. Callers of the backend (CreateCluster, DescribeClusters, etc.) always
// receive Cluster values with Tags=nil (see cloneCluster in store.go -- tags.Tags
// wraps a live safemap and is deliberately not copied by value), so tags must be
// looked up separately here via DescribeTags rather than read off c.Tags. This
// calls DescribeTags (an O(all clusters) scan) once per invocation, which is fine
// for the single-cluster call sites (Create/Modify/Delete/...) but must NOT be
// used in a per-cluster loop over DescribeClusters results -- see
// toXMLClusterWithTags for that path.
func (h *Handler) toXMLCluster(c *Cluster) xmlCluster {
	return toXMLClusterWithTags(c, h.Backend.DescribeTags()[c.ClusterIdentifier])
}

// clusterParameterGroupNameOrDefault reports the parameter group a cluster
// is associated with, falling back to the auto-provisioned default when
// CreateCluster was not given an explicit one (real AWS: CreateClusterInput.
// ClusterParameterGroupName's own doc comment, "Default: The default Amazon
// Redshift cluster parameter group").
func clusterParameterGroupNameOrDefault(name string) string {
	if name == "" {
		return "default.redshift-1.0"
	}

	return name
}

// toXMLClusterWithTags is the tag-map-parameterized core of toXMLCluster, split out
// so a caller iterating many clusters (handleDescribeClusters) can fetch the full
// tag map once with a single DescribeTags call instead of once per cluster.
func toXMLClusterWithTags(c *Cluster, tags map[string]string) xmlCluster {
	return xmlCluster{
		Tags:                             tagMapToKVList(tags),
		ClusterIdentifier:                c.ClusterIdentifier,
		NodeType:                         c.NodeType,
		ClusterType:                      c.ClusterType,
		Endpoint:                         c.Endpoint,
		EndpointPort:                     c.Port,
		ClusterStatus:                    c.Status,
		ClusterAvailabilityStatus:        "Available",
		AvailabilityZoneRelocationStatus: statusDisabled,
		MultiAZ:                          "Disabled",
		NumberOfNodes:                    c.NumberOfNodes,
		Encrypted:                        c.Encrypted,
		EnhancedVpcRouting:               c.EnhancedVpcRouting,
		SnapshotScheduleIdentifier:       c.SnapshotScheduleIdentifier,
		SnapshotScheduleState:            c.SnapshotScheduleState,
		CatalogArn:                       c.CatalogArn,
		LakehouseRegistrationStatus:      c.LakehouseRegistrationStatus,
		AquaConfiguration:                defaultAquaConfig(),
		ClusterNodes: xmlClusterNodes{
			Members: []xmlClusterNode{{
				NodeRole:         "LEADER",
				PrivateIPAddress: "10.0.0.1",
				PublicIPAddress:  "0.0.0.0",
			}},
		},
		ClusterParameterGroups: xmlClusterParamGroups{
			Members: []xmlClusterParamGroup{{
				ParameterGroupName:   clusterParameterGroupNameOrDefault(c.ClusterParameterGroupName),
				ParameterApplyStatus: "in-sync",
			}},
		},
		ClusterSecurityGroups: xmlClusterSecGroups{
			Members: func() []xmlClusterSecGroupMembership {
				members := make([]xmlClusterSecGroupMembership, 0, len(c.ClusterSecurityGroups))
				for _, name := range c.ClusterSecurityGroups {
					members = append(members, xmlClusterSecGroupMembership{
						ClusterSecurityGroupName: name,
						Status:                   "active",
					})
				}

				return members
			}(),
		},
		DBName:                     c.DBName,
		MasterUsername:             c.MasterUsername,
		KmsKeyID:                   c.KmsKeyID,
		PreferredMaintenanceWindow: c.PreferredMaintenanceWindow,
		ClusterVersion:             c.ClusterVersion,
		PubliclyAccessible:         c.PubliclyAccessible,
		VpcSecurityGroups: func() []xmlVpcSecurityGroupMembership {
			members := make([]xmlVpcSecurityGroupMembership, 0, len(c.VpcSecurityGroupIDs))
			for _, id := range c.VpcSecurityGroupIDs {
				members = append(members, xmlVpcSecurityGroupMembership{VpcSecurityGroupID: id, Status: "active"})
			}

			return members
		}(),
		IamRoles: func() xmlIamRoles {
			roles := make([]xmlIamRole, 0, len(c.IamRoles))
			for _, arn := range c.IamRoles {
				roles = append(roles, xmlIamRole{IamRoleArn: arn, ApplyStatus: "in-sync"})
			}

			return xmlIamRoles{Members: roles}
		}(),
	}
}

// resolveErrCode returns the AWS error code and HTTP status for an operation error.
// errCodeSentinels lists every sentinel error this backend can return from an
// operation. The wire error <Code> is always the sentinel's own Error() text (see
// errors.go, where each value is verified verbatim against the real SDK's
// ErrorCode() for the matching fault type) -- there is deliberately no separate
// duplicated string here, since keeping two copies in sync previously caused the
// wire code to silently drift from the sentinel (e.g. IdcApplication's fault names
// went out of sync during an earlier pass).
//
//nolint:gochecknoglobals // static sentinel lookup table, analogous to tableRegistrations
var errCodeSentinels = []error{
	ErrClusterNotFound,
	ErrClusterAlreadyExists,
	ErrInvalidParameter,
	ErrReservedNodeNotFound,
	ErrReservedNodeAlreadyExists,
	ErrReservedNodeOfferingNotFound,
	ErrPartnerNotFound,
	ErrDataShareNotFound,
	ErrSecurityGroupNotFound,
	ErrSecurityGroupAlreadyExists,
	ErrSnapshotNotFound,
	ErrSnapshotAlreadyExists,
	ErrEndpointAuthNotFound,
	ErrEndpointAuthAlreadyExists,
	ErrResizeNotFound,
	ErrResizeNotCancellable,
	ErrParameterGroupNotFound,
	ErrParameterGroupAlreadyExists,
	ErrSubnetGroupNotFound,
	ErrSubnetGroupAlreadyExists,
	ErrEventSubscriptionNotFound,
	ErrEventSubscriptionAlreadyExists,
	ErrSnapshotCopyGrantNotFound,
	ErrSnapshotCopyGrantAlreadyExists,
	ErrSnapshotScheduleNotFound,
	ErrSnapshotScheduleAlreadyExists,
	ErrUsageLimitNotFound,
	ErrAuthProfileNotFound,
	ErrAuthProfileAlreadyExists,
	ErrResourcePolicyNotFound,
	ErrSnapshotCopyAlreadyEnabled,
	ErrSnapshotCopyNotEnabled,
	ErrHsmClientCertNotFound,
	ErrHsmClientCertAlreadyExists,
	ErrHsmConfigNotFound,
	ErrHsmConfigAlreadyExists,
	ErrScheduledActionNotFound,
	ErrScheduledActionAlreadyExists,
	ErrCustomDomainNotFound,
	ErrCustomDomainAlreadyExists,
	ErrEndpointAccessNotFound,
	ErrEndpointAccessAlreadyExists,
	ErrIntegrationNotFound,
	ErrIntegrationAlreadyExists,
	ErrIdcApplicationNotFound,
	ErrIdcApplicationAlreadyExists,
	ErrQev2IdcApplicationNotFound,
	ErrQev2IdcApplicationAlreadyExists,
	ErrNamespaceRegistrationInvalidClusterState,
	ErrInvalidNamespace,
	ErrSnapshotAccessNotFound,
	ErrSecurityGroupIngressNotFound,
	ErrAuthorizationAlreadyExists,
	ErrSecurityGroupInvalidState,
	ErrParameterGroupInvalidState,
	ErrSnapshotHasAuthorizedAccounts,
	ErrInvalidS3KeyPrefix,
	ErrClusterInvalidState,
}

func resolveErrCode(opErr error) (string, int) {
	for _, sentinel := range errCodeSentinels {
		if errors.Is(opErr, sentinel) {
			return sentinel.Error(), http.StatusBadRequest
		}
	}

	return "InternalFailure", http.StatusInternalServerError
}

func (h *Handler) handleOpError(c *echo.Context, action string, opErr error) error {
	code, statusCode := resolveErrCode(opErr)

	if statusCode == http.StatusInternalServerError {
		logger.Load(c.Request().Context()).Error("Redshift internal error", "error", opErr, "action", action)
	}

	return h.writeError(c, statusCode, code, opErr.Error())
}

func (h *Handler) writeError(c *echo.Context, statusCode int, code, message string) error {
	errResp := &redshiftErrorResponse{
		Xmlns: redshiftXMLNS,
		Error: redshiftError{Code: code, Message: message, Type: "Sender"},
	}
	xmlBytes, err := marshalXML(errResp)
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	return c.Blob(statusCode, "text/xml", xmlBytes)
}

func marshalXML(v any) ([]byte, error) {
	raw, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), raw...), nil
}

// ---- XML response types ----

type redshiftError struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
	Type    string `xml:"Type"`
}

type redshiftErrorResponse struct {
	XMLName xml.Name      `xml:"ErrorResponse"`
	Xmlns   string        `xml:"xmlns,attr"`
	Error   redshiftError `xml:"Error"`
}

type xmlCluster struct {
	AquaConfiguration                xmlAquaConfig                   `xml:"AquaConfiguration"`
	MasterUsername                   string                          `xml:"MasterUsername"`
	PreferredMaintenanceWindow       string                          `xml:"PreferredMaintenanceWindow,omitempty"`
	ClusterType                      string                          `xml:"ClusterType,omitempty"`
	Endpoint                         string                          `xml:"Endpoint>Address"`
	ClusterStatus                    string                          `xml:"ClusterStatus"`
	NodeType                         string                          `xml:"NodeType"`
	ClusterAvailabilityStatus        string                          `xml:"ClusterAvailabilityStatus"`
	MultiAZ                          string                          `xml:"MultiAZ"`
	ClusterIdentifier                string                          `xml:"ClusterIdentifier"`
	SnapshotScheduleIdentifier       string                          `xml:"SnapshotScheduleIdentifier,omitempty"`
	DBName                           string                          `xml:"DBName"`
	KmsKeyID                         string                          `xml:"KmsKeyId,omitempty"`
	AvailabilityZoneRelocationStatus string                          `xml:"AvailabilityZoneRelocationStatus"`
	SnapshotScheduleState            string                          `xml:"SnapshotScheduleState,omitempty"`
	CatalogArn                       string                          `xml:"CatalogArn,omitempty"`
	LakehouseRegistrationStatus      string                          `xml:"LakehouseRegistrationStatus,omitempty"`
	ClusterParameterGroups           xmlClusterParamGroups           `xml:"ClusterParameterGroups"`
	ClusterSecurityGroups            xmlClusterSecGroups             `xml:"ClusterSecurityGroups"`
	ClusterNodes                     xmlClusterNodes                 `xml:"ClusterNodes"`
	IamRoles                         xmlIamRoles                     `xml:"IamRoles"`
	Tags                             []svcTags.KV                    `xml:"Tags>Tag,omitempty"`
	ClusterVersion                   string                          `xml:"ClusterVersion,omitempty"`
	VpcSecurityGroups                []xmlVpcSecurityGroupMembership `xml:"VpcSecurityGroups>VpcSecurityGroup,omitempty"`
	NumberOfNodes                    int                             `xml:"NumberOfNodes,omitempty"`
	EndpointPort                     int                             `xml:"Endpoint>Port,omitempty"`
	EnhancedVpcRouting               bool                            `xml:"EnhancedVpcRouting"`
	Encrypted                        bool                            `xml:"Encrypted"`
	PubliclyAccessible               bool                            `xml:"PubliclyAccessible"`
}

type xmlAquaConfig struct {
	AquaConfigurationStatus string `xml:"AquaConfigurationStatus"`
	AquaStatus              string `xml:"AquaStatus"`
}

// defaultAquaConfig returns AQUA's permanently-retired status. Both
// AquaConfigurationStatus and AquaStatus are documented "This field is
// retired" on types.AquaConfiguration (aws-sdk-go-v2/service/redshift@v1.65.4)
// -- Amazon Redshift no longer supports enabling/disabling AQUA, so every
// cluster reports it disabled. Shared by every Cluster-returning response and
// by ModifyAquaConfiguration's own response so the two can't silently diverge
// (they previously did: this handler's AquaConfiguration used "disabled" while
// ModifyAquaConfiguration's stub separately hardcoded "auto").
func defaultAquaConfig() xmlAquaConfig {
	return xmlAquaConfig{
		AquaConfigurationStatus: statusDisabled,
		AquaStatus:              statusDisabled,
	}
}

type xmlClusterNode struct {
	NodeRole         string `xml:"NodeRole"`
	PrivateIPAddress string `xml:"PrivateIPAddress"`
	PublicIPAddress  string `xml:"PublicIPAddress"`
}

type xmlClusterNodes struct {
	Members []xmlClusterNode `xml:"member"`
}

type xmlClusterParamGroup struct {
	ParameterGroupName   string `xml:"ParameterGroupName"`
	ParameterApplyStatus string `xml:"ParameterApplyStatus"`
}

type xmlClusterParamGroups struct {
	Members []xmlClusterParamGroup `xml:"ClusterParameterGroup"`
}

// xmlClusterSecGroupMembership mirrors types.ClusterSecurityGroupMembership,
// the per-cluster association entry embedded in a Cluster response -- not to
// be confused with xmlClusterSecurityGroup (handler_security_groups.go),
// which is the full group returned by DescribeClusterSecurityGroups.
type xmlClusterSecGroupMembership struct {
	ClusterSecurityGroupName string `xml:"ClusterSecurityGroupName"`
	Status                   string `xml:"Status"`
}

type xmlClusterSecGroups struct {
	Members []xmlClusterSecGroupMembership `xml:"ClusterSecurityGroup"`
}

type xmlIamRole struct {
	IamRoleArn  string `xml:"IamRoleArn"`
	ApplyStatus string `xml:"ApplyStatus"`
}

type xmlIamRoles struct {
	Members []xmlIamRole `xml:"ClusterIamRole"`
}

type createClusterResponse struct {
	XMLName xml.Name   `xml:"CreateClusterResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"CreateClusterResult>Cluster"`
}

type deleteClusterResponse struct {
	XMLName xml.Name   `xml:"DeleteClusterResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"DeleteClusterResult>Cluster"`
}

type xmlClusterList struct {
	Members []xmlCluster `xml:"Cluster"`
}

type describeClustersResponse struct {
	XMLName  xml.Name       `xml:"DescribeClustersResponse"`
	Xmlns    string         `xml:"xmlns,attr"`
	Marker   string         `xml:"DescribeClustersResult>Marker,omitempty"`
	Clusters xmlClusterList `xml:"DescribeClustersResult>Clusters"`
}

func (h *Handler) writeXMLResponse(c *echo.Context, status int, v any) error {
	xmlBytes, err := marshalXML(v)
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	return c.Blob(status, "text/xml", xmlBytes)
}
