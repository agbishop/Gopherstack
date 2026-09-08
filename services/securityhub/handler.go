package securityhub

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	securityHubServiceName = "securityhub"
	matchPriority          = service.PriorityPathVersioned

	pathAccounts     = "/accounts"
	pathAssociations = "/associations"

	// Path prefixes shared between RouteMatcher/classifyPath's dispatch
	// tables (handler.go) and their family-specific classify*Path functions.
	pathProductSubscriptions = "/productSubscriptions"
	pathHubV2                = "/hubv2"
	pathMembers              = "/members"
	pathAdministrator        = "/administrator"
	pathMaster               = "/master"
	pathConnectorsV2         = "/connectorsv2"
	pathConnectors           = "/connectors"

	keyMessage                = "Message"
	keyInsightArn             = "InsightArn"
	keyName                   = "Name"
	keyDescription            = "Description"
	keyTitle                  = "Title"
	keyRemediationURL         = "RemediationUrl"
	keySeverityRating         = "SeverityRating"
	keyUpdatedAt              = "UpdatedAt"
	keyActionTargetArn        = "ActionTargetArn"
	keyCreatedAt              = "CreatedAt"
	keyStandardsSubscriptions = "StandardsSubscriptions"
	keyUnprocessedAutoRules   = "UnprocessedAutomationRules"
	keyMaxResults             = "MaxResults"
	keyCreatedBy              = "CreatedBy"
	keyConnectorID            = "ConnectorId"
	keyConnectorArn           = "ConnectorArn"
	keyConnectorStatus        = "ConnectorStatus"
	keyEnablementStatus       = "EnablementStatus"
	keyFirstObservedAt        = "FirstObservedAt"
	keyLastObservedAt         = "LastObservedAt"

	msgNameRequired = "Name is required"

	msgHubNotEnabled   = "SecurityHub is not enabled"
	msgInsightNotFound = "Insight not found"

	// Operation names (returned by ExtractOperation).
	opEnableSecurityHub    = "EnableSecurityHub"
	opDisableSecurityHub   = "DisableSecurityHub"
	opDescribeHub          = "DescribeHub"
	opUpdateSecurityHubCfg = "UpdateSecurityHubConfiguration"

	opGetFindings         = "GetFindings"
	opBatchImportFindings = "BatchImportFindings"
	opBatchUpdateFindings = "BatchUpdateFindings"
	opUpdateFindings      = "UpdateFindings"
	opGetFindingHistory   = "GetFindingHistory"

	opCreateInsight     = "CreateInsight"
	opGetInsights       = "GetInsights"
	opGetInsightResults = "GetInsightResults"
	opUpdateInsight     = "UpdateInsight"
	opDeleteInsight     = "DeleteInsight"

	opBatchEnableStandards  = "BatchEnableStandards"
	opBatchDisableStandards = "BatchDisableStandards"
	opGetEnabledStandards   = "GetEnabledStandards"
	opDescribeStandards     = "DescribeStandards"
	opDescribeStdControls   = "DescribeStandardsControls"
	opUpdateStdControl      = "UpdateStandardsControl"

	opListStdCtlAssocs        = "ListStandardsControlAssociations"
	opBatchGetStdCtlAssocs    = "BatchGetStandardsControlAssociations"
	opBatchUpdateStdCtlAssocs = "BatchUpdateStandardsControlAssociations"

	opCreateActionTarget    = "CreateActionTarget"
	opDescribeActionTargets = "DescribeActionTargets"
	opUpdateActionTarget    = "UpdateActionTarget"
	opDeleteActionTarget    = "DeleteActionTarget"

	opDescribeProducts                = "DescribeProducts"
	opListEnabledProductsForImport    = "ListEnabledProductsForImport"
	opEnableImportFindingsForProduct  = "EnableImportFindingsForProduct"
	opDisableImportFindingsForProduct = "DisableImportFindingsForProduct"

	opGetSecurityControlDefinition   = "GetSecurityControlDefinition"
	opListSecurityControlDefinitions = "ListSecurityControlDefinitions"
	opBatchGetSecurityControls       = "BatchGetSecurityControls"
	opUpdateSecurityControl          = "UpdateSecurityControl"

	opListAutomationRules     = "ListAutomationRules"
	opCreateAutomationRule    = "CreateAutomationRule"
	opBatchGetAutomationRules = "BatchGetAutomationRules"
	opBatchDeleteAutoRules    = "BatchDeleteAutomationRules"
	opBatchUpdateAutoRules    = "BatchUpdateAutomationRules"

	opListTagsForResource = "ListTagsForResource"
	opTagResource         = "TagResource"
	opUntagResource       = "UntagResource"

	// Members.
	opCreateMembers       = "CreateMembers"
	opDeleteMembers       = "DeleteMembers"
	opGetMembers          = "GetMembers"
	opInviteMembers       = "InviteMembers"
	opListMembers         = "ListMembers"
	opDisassociateMembers = "DisassociateMembers"

	// Invitations / Admin.
	opAcceptAdministratorInvitation        = "AcceptAdministratorInvitation"
	opAcceptInvitation                     = "AcceptInvitation"
	opDeclineInvitations                   = "DeclineInvitations"
	opDeleteInvitations                    = "DeleteInvitations"
	opGetInvitationsCount                  = "GetInvitationsCount"
	opListInvitations                      = "ListInvitations"
	opGetAdministratorAccount              = "GetAdministratorAccount"
	opGetMasterAccount                     = "GetMasterAccount"
	opDisassociateFromAdministratorAccount = "DisassociateFromAdministratorAccount"
	opDisassociateFromMasterAccount        = "DisassociateFromMasterAccount"

	// Organization.
	opDescribeOrganizationConfiguration = "DescribeOrganizationConfiguration"
	opUpdateOrganizationConfiguration   = "UpdateOrganizationConfiguration"
	opEnableOrganizationAdminAccount    = "EnableOrganizationAdminAccount"
	opDisableOrganizationAdminAccount   = "DisableOrganizationAdminAccount"
	opListOrganizationAdminAccounts     = "ListOrganizationAdminAccounts"

	// Finding Aggregator.
	opCreateFindingAggregator = "CreateFindingAggregator"
	opGetFindingAggregator    = "GetFindingAggregator"
	opListFindingAggregators  = "ListFindingAggregators"
	opUpdateFindingAggregator = "UpdateFindingAggregator"
	opDeleteFindingAggregator = "DeleteFindingAggregator"

	// Configuration Policy.
	opCreateConfigurationPolicy               = "CreateConfigurationPolicy"
	opGetConfigurationPolicy                  = "GetConfigurationPolicy"
	opUpdateConfigurationPolicy               = "UpdateConfigurationPolicy"
	opDeleteConfigurationPolicy               = "DeleteConfigurationPolicy"
	opListConfigurationPolicies               = "ListConfigurationPolicies"
	opGetConfigurationPolicyAssociation       = "GetConfigurationPolicyAssociation"
	opListConfigurationPolicyAssociations     = "ListConfigurationPolicyAssociations"
	opStartConfigurationPolicyAssociation     = "StartConfigurationPolicyAssociation"
	opStartConfigurationPolicyDisassociation  = "StartConfigurationPolicyDisassociation"
	opBatchGetConfigurationPolicyAssociations = "BatchGetConfigurationPolicyAssociations"

	// Hub V2.
	opEnableSecurityHubV2   = "EnableSecurityHubV2"
	opDisableSecurityHubV2  = "DisableSecurityHubV2"
	opDescribeSecurityHubV2 = "DescribeSecurityHubV2"

	// Hub V2 opt-in features.
	opEnableSecurityHubFeatureV2  = "EnableSecurityHubFeatureV2"
	opDisableSecurityHubFeatureV2 = "DisableSecurityHubFeatureV2"

	// CSPM Connectors (third-party cloud provider connectors -- distinct
	// from the "Connectors V2" ticketing-system family below).
	opCreateConnector = "CreateConnector"
	opGetConnector    = "GetConnector"
	opUpdateConnector = "UpdateConnector"
	opDeleteConnector = "DeleteConnector"
	opListConnectors  = "ListConnectors"

	// Aggregator V2.
	opCreateAggregatorV2 = "CreateAggregatorV2"
	opGetAggregatorV2    = "GetAggregatorV2"
	opListAggregatorsV2  = "ListAggregatorsV2"
	opUpdateAggregatorV2 = "UpdateAggregatorV2"
	opDeleteAggregatorV2 = "DeleteAggregatorV2"

	// Automation Rules V2.
	opCreateAutomationRuleV2 = "CreateAutomationRuleV2"
	opGetAutomationRuleV2    = "GetAutomationRuleV2"
	opListAutomationRulesV2  = "ListAutomationRulesV2"
	opUpdateAutomationRuleV2 = "UpdateAutomationRuleV2"
	opDeleteAutomationRuleV2 = "DeleteAutomationRuleV2"

	// Connectors V2.
	opCreateConnectorV2   = "CreateConnectorV2"
	opGetConnectorV2      = "GetConnectorV2"
	opListConnectorsV2    = "ListConnectorsV2"
	opUpdateConnectorV2   = "UpdateConnectorV2"
	opDeleteConnectorV2   = "DeleteConnectorV2"
	opRegisterConnectorV2 = "RegisterConnectorV2"

	// Tickets V2.
	opCreateTicketV2 = "CreateTicketV2"

	// Findings V2.
	opGetFindingsV2          = "GetFindingsV2"
	opBatchUpdateFindingsV2  = "BatchUpdateFindingsV2"
	opGetFindingStatisticsV2 = "GetFindingStatisticsV2"
	opGetFindingsTrendsV2    = "GetFindingsTrendsV2"

	// Resources V2.
	opGetResourcesV2           = "GetResourcesV2"
	opGetResourcesStatisticsV2 = "GetResourcesStatisticsV2"
	opGetResourcesTrendsV2     = "GetResourcesTrendsV2"

	// Products V2.
	opDescribeProductsV2 = "DescribeProductsV2"

	// Recommended Policy V2.
	opGenerateRecommendedPolicyV2 = "GenerateRecommendedPolicyV2"
	opGetRecommendedPolicyV2      = "GetRecommendedPolicyV2"

	opUnknown = "Unknown"
)

// Handler handles SecurityHub HTTP requests.
type Handler struct {
	Backend StorageBackend
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	return &Handler{Backend: b}
}

// Name returns the service name.
func (h *Handler) Name() string { return "SecurityHub" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// GetSupportedOperations returns all supported operations.
// supportedOperations lists every SecurityHub operation name this handler
// routes. It is a package-level var (not rebuilt on every
// GetSupportedOperations call) purely for allocation efficiency; the order
// matches the historical grouping used across the handler_<family>.go files.
var supportedOperations = []string{ //nolint:gochecknoglobals // read-only lookup data
	opEnableSecurityHub,
	opDisableSecurityHub,
	opDescribeHub,
	opUpdateSecurityHubCfg,
	opGetFindings,
	opBatchImportFindings,
	opBatchUpdateFindings,
	opUpdateFindings,
	opGetFindingHistory,
	opCreateInsight,
	opGetInsights,
	opGetInsightResults,
	opUpdateInsight,
	opDeleteInsight,
	opBatchEnableStandards,
	opBatchDisableStandards,
	opGetEnabledStandards,
	opDescribeStandards,
	opDescribeStdControls,
	opUpdateStdControl,
	opListStdCtlAssocs,
	opBatchGetStdCtlAssocs,
	opBatchUpdateStdCtlAssocs,
	opCreateActionTarget,
	opDescribeActionTargets,
	opUpdateActionTarget,
	opDeleteActionTarget,
	opDescribeProducts,
	opListEnabledProductsForImport,
	opEnableImportFindingsForProduct,
	opDisableImportFindingsForProduct,
	opGetSecurityControlDefinition,
	opListSecurityControlDefinitions,
	opBatchGetSecurityControls,
	opUpdateSecurityControl,
	opListAutomationRules,
	opCreateAutomationRule,
	opBatchGetAutomationRules,
	opBatchDeleteAutoRules,
	opBatchUpdateAutoRules,
	opListTagsForResource,
	opTagResource,
	opUntagResource,
	// Members
	opCreateMembers,
	opDeleteMembers,
	opGetMembers,
	opInviteMembers,
	opListMembers,
	opDisassociateMembers,
	// Invitations / Admin
	opAcceptAdministratorInvitation,
	opAcceptInvitation,
	opDeclineInvitations,
	opDeleteInvitations,
	opGetInvitationsCount,
	opListInvitations,
	opGetAdministratorAccount,
	opGetMasterAccount,
	opDisassociateFromAdministratorAccount,
	opDisassociateFromMasterAccount,
	// Organization
	opDescribeOrganizationConfiguration,
	opUpdateOrganizationConfiguration,
	opEnableOrganizationAdminAccount,
	opDisableOrganizationAdminAccount,
	opListOrganizationAdminAccounts,
	// Finding Aggregator
	opCreateFindingAggregator,
	opGetFindingAggregator,
	opListFindingAggregators,
	opUpdateFindingAggregator,
	opDeleteFindingAggregator,
	// Configuration Policy
	opCreateConfigurationPolicy,
	opGetConfigurationPolicy,
	opUpdateConfigurationPolicy,
	opDeleteConfigurationPolicy,
	opListConfigurationPolicies,
	opGetConfigurationPolicyAssociation,
	opListConfigurationPolicyAssociations,
	opStartConfigurationPolicyAssociation,
	opStartConfigurationPolicyDisassociation,
	opBatchGetConfigurationPolicyAssociations,
	// Hub V2
	opEnableSecurityHubV2,
	opDisableSecurityHubV2,
	opDescribeSecurityHubV2,
	opEnableSecurityHubFeatureV2,
	opDisableSecurityHubFeatureV2,
	// CSPM Connectors
	opCreateConnector,
	opGetConnector,
	opUpdateConnector,
	opDeleteConnector,
	opListConnectors,
	// Aggregator V2
	opCreateAggregatorV2,
	opGetAggregatorV2,
	opListAggregatorsV2,
	opUpdateAggregatorV2,
	opDeleteAggregatorV2,
	// Automation Rules V2
	opCreateAutomationRuleV2,
	opGetAutomationRuleV2,
	opListAutomationRulesV2,
	opUpdateAutomationRuleV2,
	opDeleteAutomationRuleV2,
	// Connectors V2
	opCreateConnectorV2,
	opGetConnectorV2,
	opListConnectorsV2,
	opUpdateConnectorV2,
	opDeleteConnectorV2,
	opRegisterConnectorV2,
	// Tickets V2
	opCreateTicketV2,
	// Findings V2
	opGetFindingsV2,
	opBatchUpdateFindingsV2,
	opGetFindingStatisticsV2,
	opGetFindingsTrendsV2,
	// Resources V2
	opGetResourcesV2,
	opGetResourcesStatisticsV2,
	opGetResourcesTrendsV2,
	// Products V2
	opDescribeProductsV2,
	// Recommended Policy V2
	opGenerateRecommendedPolicyV2,
	opGetRecommendedPolicyV2,
}

// GetSupportedOperations returns every operation name this handler routes.
func (h *Handler) GetSupportedOperations() []string { return supportedOperations }

// ChaosServiceName returns the service name for chaos engineering.
func (h *Handler) ChaosServiceName() string { return securityHubServiceName }

// ChaosOperations returns the operations for chaos engineering.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns the regions for chaos engineering.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// securityHubOnlyPathPrefixes are path prefixes that unambiguously belong to
// SecurityHub (no other service registers them), so RouteMatcher can claim
// them without further disambiguation. pathAccounts is checked separately
// since it is an exact match, not a prefix.
var securityHubOnlyPathPrefixes = []string{ //nolint:gochecknoglobals // read-only lookup data
	"/actionTargets",
	"/insights",
	pathProductSubscriptions,
	"/standards",
	"/associations",
	"/securityControl",
	"/automationrules",
	"/findingHistory",
	pathHubV2,
	pathMembers,
	"/invitations",
	pathAdministrator,
	pathMaster,
	"/organization",
	"/findingAggregator",
	"/configurationPolicy",
	"/configurationPolicyAssociation",
	"/aggregatorv2",
	"/automationrulesv2",
	pathConnectorsV2,
	pathConnectors,
	"/ticketsv2",
	"/findingsv2",
	"/findingsTrendsv2",
	"/resourcesv2",
	"/resourcesTrendsv2",
	"/productsV2",
	"/recommendedPolicyV2",
	"/products",
}

// hasAnyPrefix reports whether s starts with any of prefixes.
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}

	return false
}

// RouteMatcher returns a function that matches SecurityHub requests by path.
// For /findings and /tags paths, uses the Authorization header to disambiguate
// from other services (e.g. Macie2) that share those path prefixes.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		// Unambiguous SecurityHub-only paths. /accounts is exact-match only:
		// the real API (EnableSecurityHub, DisableSecurityHub, DescribeHub,
		// UpdateSecurityHubConfiguration) never binds anything under
		// /accounts/... -- that shape belongs to QuickSight
		// (aws-sdk-go-v2/service/quicksight@v1.123.1, e.g. serializers.go:1023
		// "/accounts/{AwsAccountId}/analyses/{AnalysisId}").
		if path == pathAccounts || hasAnyPrefix(path, securityHubOnlyPathPrefixes) {
			return true
		}

		// /findings — disambiguate using Authorization signing service
		if strings.HasPrefix(path, "/findings") {
			return isSecurityHubRequest(c)
		}

		// /tags/{ARN} — match SecurityHub ARNs
		if after, ok := strings.CutPrefix(path, "/tags/"); ok {
			return strings.Contains(after, ":"+securityHubServiceName+":")
		}

		return false
	}
}

// isSecurityHubRequest checks the Authorization header for the securityhub signing service.
func isSecurityHubRequest(c *echo.Context) bool {
	auth := c.Request().Header.Get("Authorization")

	return strings.Contains(auth, "/"+securityHubServiceName+"/")
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return matchPriority }

// ExtractOperation classifies the request into an operation name.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _ := classifyPath(c.Request().Method, c.Request().URL.Path)

	return op
}

// ExtractResource returns the resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	_, resource := classifyPath(c.Request().Method, c.Request().URL.Path)

	return resource
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return h.handleREST(c)
	}
}

// amznErrorTypeHeader carries the modeled exception type for the restjson1
// protocol. aws-sdk-go-v2's restjson.GetErrorInfo (aws/protocol/restjson/decoder_util.go)
// reads this header before falling back to a body "code"/"__type" field; without it every
// error here deserialized client-side as a generic UnknownError, since this package had no
// central error handler and every call site wrote only a "Message" field.
const amznErrorTypeHeader = "X-Amzn-Errortype"

// typedErrorResponse writes a wire-accurate error response, setting
// X-Amzn-Errortype so a real SDK client can distinguish error kinds. errType
// must be verified against the specific operation's own deserializer error
// list (securityhub@v1.75.4 deserializers.go) before use at a given call
// site -- the exception vocabulary differs between the classic REST API
// (InvalidInputException/InternalException/ResourceConflictException) and
// the newer V2-style operations, including some non-"V2"-suffixed ones like
// Connectors (ValidationException/InternalServerException/ConflictException).
func typedErrorResponse(c *echo.Context, status int, errType, message string) error {
	c.Response().Header().Set(amznErrorTypeHeader, errType)

	return c.JSON(status, map[string]any{keyMessage: message})
}

// errInvalidJSONBody is returned unwritten so handleREST can map and write
// it exactly once. decodeJSONBody used to write the 400 itself via c.JSON
// and return that call's (always-nil) result, so handleREST's
// `if err != nil` check never fired and dispatch proceeded to the matched
// operation with body == nil (gopherstack-3t96, the gopherstack-8haq
// shape).
var errInvalidJSONBody = errors.New("invalid JSON body")

// decodeJSONBody runs before the request is classified to an operation, so
// it can't know whether that operation uses the classic InvalidInputException
// vocabulary or the newer ValidationException one -- handleREST leaves the
// response unheadered (no X-Amzn-Errortype) rather than guessing (same
// reasoning as the ErrHubNotEnabled cases in handler_hub.go).
func decodeJSONBody(c *echo.Context) (map[string]any, error) {
	var body map[string]any

	if c.Request().ContentLength != 0 {
		if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil && err.Error() != "EOF" {
			return nil, errInvalidJSONBody
		}
	}

	if body == nil {
		body = map[string]any{}
	}

	return body, nil
}

// opHandlerGroups returns the per-family operation dispatch tables, each
// built in the matching handler_<family>.go file. handleREST searches them
// in order for the first table containing op.
func (h *Handler) opHandlerGroups(c *echo.Context, resource string, body map[string]any) []map[string]func() error {
	return []map[string]func() error{
		h.hubOpHandlers(c, resource, body),
		h.findingsOpHandlers(c, body),
		h.insightsOpHandlers(c, resource, body),
		h.standardsOpHandlers(c, resource, body),
		h.actionTargetsOpHandlers(c, resource, body),
		h.productsOpHandlers(c, resource, body),
		h.controlsOpHandlers(c, body),
		h.automationRulesOpHandlers(c, resource, body),
		h.tagsOpHandlers(c, resource, body),
		h.membersOpHandlers(c, body),
		h.invitationsOpHandlers(c, body),
		h.organizationsOpHandlers(c, body),
		h.findingAggregatorsOpHandlers(c, resource, body),
		h.configPolicyOpHandlers(c, resource, body),
		h.aggregatorsV2OpHandlers(c, resource, body),
		h.connectorsV2OpHandlers(c, resource, body),
		h.connectorsOpHandlers(c, resource, body),
		h.resourcesV2OpHandlers(c, body),
	}
}

// handleREST dispatches to the appropriate handler function.
func (h *Handler) handleREST(c *echo.Context) error {
	op, resource := classifyPath(c.Request().Method, c.Request().URL.Path)

	body, err := decodeJSONBody(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{keyMessage: err.Error()})
	}

	for _, handlers := range h.opHandlerGroups(c, resource, body) {
		if fn, ok := handlers[op]; ok {
			return fn()
		}
	}

	return typedErrorResponse(c, http.StatusNotFound, "ResourceNotFoundException", "unknown operation")
}

// pathClassifier pairs a path predicate with the classify*Path function to
// delegate to when it matches. classifyPath walks pathClassifiers in order
// and dispatches to the first match, so branching that used to live in one
// giant switch is now data instead of code.
type pathClassifier struct {
	matches  func(path string) bool
	classify func(method, path string) (string, string)
}

// isFindingsV1Path reports whether path is a V1 /findings or /findingHistory
// path, as opposed to the V2 /findingsv2, /findingsTrendsv2, or the unrelated
// /findingAggregator family.
func isFindingsV1Path(path string) bool {
	if strings.HasPrefix(path, "/findingHistory") {
		return true
	}

	return strings.HasPrefix(path, "/findings") &&
		!strings.HasPrefix(path, "/findingsv2") &&
		!strings.HasPrefix(path, "/findingsTrendsv2") &&
		!strings.HasPrefix(path, "/findingAggregator")
}

func isHubPath(path string) bool {
	return path == pathAccounts || strings.HasPrefix(path, pathAccounts+"/")
}

func isStandardsPath(path string) bool {
	return strings.HasPrefix(path, "/standards") ||
		path == pathAssociations ||
		strings.HasPrefix(path, pathAssociations+"/")
}

func isProductsPath(path string) bool {
	return strings.HasPrefix(path, "/products") || strings.HasPrefix(path, pathProductSubscriptions)
}

func isFindingsV2Path(path string) bool {
	return strings.HasPrefix(path, "/findingsv2") || strings.HasPrefix(path, "/findingsTrendsv2")
}

func isResourcesV2Path(path string) bool {
	return strings.HasPrefix(path, "/resourcesv2") || strings.HasPrefix(path, "/resourcesTrendsv2")
}

func hasPathPrefix(prefix string) func(string) bool {
	return func(path string) bool { return strings.HasPrefix(path, prefix) }
}

// pathClassifiers is the ordered (method, path) → operation dispatch table.
// Order matters: more specific prefixes (e.g. "/configurationPolicyAssociation")
// must be checked before the prefixes they themselves start with
// (e.g. "/configurationPolicy").
var pathClassifiers = []pathClassifier{ //nolint:gochecknoglobals // ordered dispatch table
	{isHubPath, classifyHubPath},
	{hasPathPrefix(pathHubV2), classifyHubV2Path},
	{isFindingsV1Path, classifyFindingsPath},
	{isFindingsV2Path, classifyFindingsV2Path},
	{isResourcesV2Path, classifyResourcesV2Path},
	{hasPathPrefix("/insights"), classifyInsightsPath},
	{isStandardsPath, classifyStandardsPath},
	{hasPathPrefix("/actionTargets"), classifyActionTargetsPath},
	{hasPathPrefix("/productsV2"), classifyProductsV2Path},
	{isProductsPath, classifyProductsPath},
	{hasPathPrefix("/securityControl"), classifySecurityControlsPath},
	{hasPathPrefix("/automationrulesv2"), classifyAutomationRulesV2Path},
	{hasPathPrefix("/automationrules"), classifyAutomationRulesPath},
	{hasPathPrefix(pathMembers), classifyMembersPath},
	{hasPathPrefix("/invitations"), classifyInvitationsPath},
	{hasPathPrefix(pathAdministrator), classifyAdministratorPath},
	{hasPathPrefix(pathMaster), classifyMasterPath},
	{hasPathPrefix("/organization"), classifyOrganizationPath},
	{hasPathPrefix("/findingAggregator"), classifyFindingAggregatorPath},
	{hasPathPrefix("/configurationPolicyAssociation"), classifyConfigPolicyAssocPath},
	{hasPathPrefix("/configurationPolicy"), classifyConfigPolicyPath},
	{hasPathPrefix("/aggregatorv2"), classifyAggregatorV2Path},
	{hasPathPrefix(pathConnectorsV2), classifyConnectorsV2Path},
	{hasPathPrefix(pathConnectors), classifyConnectorsPath},
	{hasPathPrefix("/ticketsv2"), classifyTicketsV2Path},
	{hasPathPrefix("/recommendedPolicyV2"), classifyRecommendedPolicyV2Path},
	{hasPathPrefix("/tags/"), classifyTagsPath},
}

// classifyPath maps (method, path) → (operation, resource).
func classifyPath(method, path string) (string, string) {
	for _, pc := range pathClassifiers {
		if pc.matches(path) {
			return pc.classify(method, path)
		}
	}

	return opUnknown, ""
}

func intFromBody(body map[string]any) int {
	if v, ok := body[keyMaxResults].(float64); ok {
		return int(v)
	}

	return 0
}

func queryInt(c *echo.Context) int {
	v, err := strconv.Atoi(c.QueryParam(keyMaxResults))
	if err != nil {
		return 0
	}

	return v
}
