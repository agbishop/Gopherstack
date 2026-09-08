package appconfig

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyMessageField       = "message"
	errInvalidRequestBody = "invalid request body"
)

const (
	opUnknown = "Unknown"
	keyItems  = "Items"
)

const (
	opCreateApplication                = "CreateApplication"
	opCreateConfigurationProfile       = "CreateConfigurationProfile"
	opCreateDeploymentStrategy         = "CreateDeploymentStrategy"
	opCreateEnvironment                = "CreateEnvironment"
	opCreateExperimentDefinition       = "CreateExperimentDefinition"
	opCreateExtension                  = "CreateExtension"
	opCreateExtensionAssociation       = "CreateExtensionAssociation"
	opCreateHostedConfigurationVersion = "CreateHostedConfigurationVersion"
	opDeleteApplication                = "DeleteApplication"
	opDeleteConfigurationProfile       = "DeleteConfigurationProfile"
	opDeleteDeploymentStrategy         = "DeleteDeploymentStrategy"
	opDeleteEnvironment                = "DeleteEnvironment"
	opDeleteExperimentDefinition       = "DeleteExperimentDefinition"
	opDeleteExtension                  = "DeleteExtension"
	opDeleteExtensionAssociation       = "DeleteExtensionAssociation"
	opDeleteHostedConfigurationVersion = "DeleteHostedConfigurationVersion"
	opGetAccountSettings               = "GetAccountSettings"
	opGetApplication                   = "GetApplication"
	opGetConfiguration                 = "GetConfiguration"
	opGetConfigurationProfile          = "GetConfigurationProfile"
	opGetDeployment                    = "GetDeployment"
	opGetDeploymentStrategy            = "GetDeploymentStrategy"
	opGetEnvironment                   = "GetEnvironment"
	opGetExperimentDefinition          = "GetExperimentDefinition"
	opGetExperimentRun                 = "GetExperimentRun"
	opGetExtension                     = "GetExtension"
	opGetExtensionAssociation          = "GetExtensionAssociation"
	opGetHostedConfigurationVersion    = "GetHostedConfigurationVersion"
	opListApplications                 = "ListApplications"
	opListConfigurationProfiles        = "ListConfigurationProfiles"
	opListDeploymentStrategies         = "ListDeploymentStrategies"
	opListDeployments                  = "ListDeployments"
	opListEnvironments                 = "ListEnvironments"
	opListExperimentDefinitions        = "ListExperimentDefinitions"
	opListExperimentRunEvents          = "ListExperimentRunEvents"
	opListExperimentRuns               = "ListExperimentRuns"
	opListExtensionAssociations        = "ListExtensionAssociations"
	opListExtensions                   = "ListExtensions"
	opListHostedConfigurationVersions  = "ListHostedConfigurationVersions"
	opListTagsForResource              = "ListTagsForResource"
	opStartDeployment                  = "StartDeployment"
	opStartExperimentRun               = "StartExperimentRun"
	opStopDeployment                   = "StopDeployment"
	opStopExperimentRun                = "StopExperimentRun"
	opTagResource                      = "TagResource"
	opUntagResource                    = "UntagResource"
	opUpdateAccountSettings            = "UpdateAccountSettings"
	opUpdateApplication                = "UpdateApplication"
	opUpdateConfigurationProfile       = "UpdateConfigurationProfile"
	opUpdateDeploymentStrategy         = "UpdateDeploymentStrategy"
	opUpdateEnvironment                = "UpdateEnvironment"
	opUpdateExperimentDefinition       = "UpdateExperimentDefinition"
	opUpdateExperimentRun              = "UpdateExperimentRun"
	opUpdateExtension                  = "UpdateExtension"
	opUpdateExtensionAssociation       = "UpdateExtensionAssociation"
	opValidateConfiguration            = "ValidateConfiguration"
)

const (
	appConfigMatchPriority = 86

	// pathParts* constants define the expected segment counts for route matching.
	pathPartsBase       = 2 // /resource/{id}
	pathPartsSubLevel   = 3 // /applications/{id}/subresource
	pathPartsSubItem    = 4 // /applications/{id}/subresource/{subId}
	pathPartsDeepLevel  = 5 // /applications/{id}/subresource/{subId}/nested
	pathPartsDeepItem   = 6 // /applications/{id}/subresource/{subId}/nested/{nestedId}
	pathPartsDeeperItem = 7 // /applications/{id}/subresource/{subId}/nested/{nestedId}/action

	// maxHostedConfigurationVersionBytes caps the request-body size accepted by
	// CreateHostedConfigurationVersion. AWS AppConfig allows hosted configuration
	// versions up to 1 MiB; clamp here to prevent unbounded io.ReadAll DoS.
	maxHostedConfigurationVersionBytes = 1 << 20 // 1 MiB
)

// Handler is the Echo HTTP handler for AppConfig operations.
type Handler struct {
	Backend StorageBackend
}

// NewHandler creates a new AppConfig Handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "AppConfig" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateApplication,
		opGetApplication,
		opListApplications,
		opUpdateApplication,
		opDeleteApplication,
		opCreateEnvironment,
		opGetEnvironment,
		opListEnvironments,
		opUpdateEnvironment,
		opDeleteEnvironment,
		opCreateConfigurationProfile,
		opGetConfigurationProfile,
		opListConfigurationProfiles,
		opUpdateConfigurationProfile,
		opDeleteConfigurationProfile,
		opCreateHostedConfigurationVersion,
		opGetHostedConfigurationVersion,
		opListHostedConfigurationVersions,
		opDeleteHostedConfigurationVersion,
		opCreateDeploymentStrategy,
		opGetDeploymentStrategy,
		opListDeploymentStrategies,
		opUpdateDeploymentStrategy,
		opDeleteDeploymentStrategy,
		opStartDeployment,
		opGetDeployment,
		opListDeployments,
		opStopDeployment,
		opListTagsForResource,
		opTagResource,
		opUntagResource,
		opCreateExtension,
		opGetExtension,
		opListExtensions,
		opUpdateExtension,
		opDeleteExtension,
		opCreateExtensionAssociation,
		opGetExtensionAssociation,
		opListExtensionAssociations,
		opUpdateExtensionAssociation,
		opDeleteExtensionAssociation,
		opGetAccountSettings,
		opUpdateAccountSettings,
		opGetConfiguration,
		opValidateConfiguration,
		opCreateExperimentDefinition,
		opGetExperimentDefinition,
		opListExperimentDefinitions,
		opUpdateExperimentDefinition,
		opDeleteExperimentDefinition,
		opStartExperimentRun,
		opGetExperimentRun,
		opListExperimentRuns,
		opUpdateExperimentRun,
		opStopExperimentRun,
		opListExperimentRunEvents,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "appconfig" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function matching AppConfig REST API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		// EMRServerless and ServerlessRepo also bind "/applications" bare in
		// their real APIs; scope by SigV4 so a correctly-signed request for
		// either isn't swallowed here regardless of registration/priority
		// ordering (see gopherstack-ibeo).
		if strings.HasPrefix(path, "/applications") {
			return httputils.ScopedPrefixMatch(c.Request(), path, "/applications", "appconfig")
		}

		return strings.HasPrefix(path, "/deploymentstrategies") ||
			// The AWS AppConfig API ships a known typo: DeleteDeploymentStrategy
			// uses the misspelled "/deployementstrategies/{Id}" URI while every
			// other deployment-strategy operation uses "/deploymentstrategies".
			// The SDK serializer hard-codes this, so we must match it too.
			strings.HasPrefix(path, "/deployementstrategies") ||
			strings.HasPrefix(path, "/extensions") ||
			strings.HasPrefix(path, "/extensionassociations") ||
			// ListExperimentDefinitions alone is account-wide (not nested
			// under /applications/{id}) -- every other experiment
			// definition/run route IS under /applications and is already
			// covered by the prefix above.
			strings.HasPrefix(path, "/experimentdefinitions") ||
			path == "/settings" ||
			strings.HasPrefix(path, "/tags/arn:aws:appconfig:")
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return appConfigMatchPriority }

// ExtractOperation returns the operation name based on the parsed path and HTTP method.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	route := parseAppConfigPath(c.Request().Method, c.Request().URL.Path)

	return route.operation
}

// ExtractResource extracts the primary resource ID from the URL path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	route := parseAppConfigPath(c.Request().Method, c.Request().URL.Path)
	if route.applicationID != "" {
		return route.applicationID
	}

	return route.strategyID
}

// appConfigRoute holds parsed path segments and the derived operation name.
type appConfigRoute struct {
	applicationID          string
	environmentID          string
	profileID              string
	strategyID             string
	resourceArn            string
	extensionID            string
	extensionAssociationID string
	configurationID        string
	experimentDefinitionID string
	operation              string
	versionNumber          int32
	deploymentNum          int32
	experimentRunNumber    int32
}

// parseAppConfigPath parses an HTTP method and URL path into an appConfigRoute,
// identifying the resource type, IDs, and operation name for AppConfig REST API requests.
// It maps REST path segments to their corresponding CRUD operations.
func parseAppConfigPath(method, path string) appConfigRoute {
	// Trim leading slash and split.
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(parts) == 0 {
		return appConfigRoute{operation: opUnknown}
	}

	switch parts[0] {
	case "deploymentstrategies", "deployementstrategies":
		// "deployementstrategies" is the misspelled URI segment the AWS SDK
		// hard-codes for DeleteDeploymentStrategy; treat it identically.
		return parseDeploymentStrategyRoute(method, parts)
	case "applications":
		return parseApplicationRoute(method, parts)
	case "extensions":
		return parseExtensionRoute(method, parts)
	case "extensionassociations":
		return parseExtensionAssociationRoute(method, parts)
	case "experimentdefinitions":
		// The account-wide ListExperimentDefinitions route
		// ("/experimentdefinitions", no application prefix) -- every other
		// experiment definition/run route is nested under
		// "/applications/{id}/experimentdefinitions" and is handled by
		// parseApplicationRoute -> parseExperimentDefinitionRoute instead.
		if len(parts) == 1 && method == http.MethodGet {
			return appConfigRoute{operation: opListExperimentDefinitions}
		}
	case "settings":
		if len(parts) == 1 {
			switch method {
			case http.MethodGet:
				return appConfigRoute{operation: opGetAccountSettings}
			case http.MethodPatch:
				return appConfigRoute{operation: opUpdateAccountSettings}
			}
		}
	case "tags":
		// ARN spans all remaining path segments joined by "/"
		return parseTagRoute(method, strings.Join(parts[1:], "/"))
	}

	return appConfigRoute{operation: opUnknown}
}

func parseTagRoute(method, resourceArn string) appConfigRoute {
	base := appConfigRoute{resourceArn: resourceArn}

	switch method {
	case http.MethodGet:
		base.operation = opListTagsForResource
	case http.MethodPost:
		base.operation = opTagResource
	case http.MethodDelete:
		base.operation = opUntagResource
	default:
		base.operation = opUnknown
	}

	return base
}

func parseExtensionRoute(method string, parts []string) appConfigRoute {
	if len(parts) == 1 {
		switch method {
		case http.MethodPost:
			return appConfigRoute{operation: opCreateExtension}
		case http.MethodGet:
			return appConfigRoute{operation: opListExtensions}
		}

		return appConfigRoute{operation: opUnknown}
	}

	extID := parts[1]

	switch method {
	case http.MethodGet:
		return appConfigRoute{extensionID: extID, operation: opGetExtension}
	case http.MethodPatch:
		return appConfigRoute{extensionID: extID, operation: opUpdateExtension}
	case http.MethodDelete:
		return appConfigRoute{extensionID: extID, operation: opDeleteExtension}
	}

	return appConfigRoute{extensionID: extID, operation: opUnknown}
}

func parseExtensionAssociationRoute(method string, parts []string) appConfigRoute {
	if len(parts) == 1 {
		switch method {
		case http.MethodPost:
			return appConfigRoute{operation: opCreateExtensionAssociation}
		case http.MethodGet:
			return appConfigRoute{operation: opListExtensionAssociations}
		}

		return appConfigRoute{operation: opUnknown}
	}

	assocID := parts[1]

	switch method {
	case http.MethodGet:
		return appConfigRoute{extensionAssociationID: assocID, operation: opGetExtensionAssociation}
	case http.MethodPatch:
		return appConfigRoute{
			extensionAssociationID: assocID,
			operation:              opUpdateExtensionAssociation,
		}
	case http.MethodDelete:
		return appConfigRoute{
			extensionAssociationID: assocID,
			operation:              opDeleteExtensionAssociation,
		}
	}

	return appConfigRoute{extensionAssociationID: assocID, operation: opUnknown}
}

func parseDeploymentStrategyRoute(method string, parts []string) appConfigRoute {
	if len(parts) == 1 {
		switch method {
		case http.MethodPost:
			return appConfigRoute{operation: opCreateDeploymentStrategy}
		case http.MethodGet:
			return appConfigRoute{operation: opListDeploymentStrategies}
		}

		return appConfigRoute{operation: opUnknown}
	}

	strategyID := parts[1]

	switch method {
	case http.MethodGet:
		return appConfigRoute{strategyID: strategyID, operation: opGetDeploymentStrategy}
	case http.MethodPatch:
		return appConfigRoute{strategyID: strategyID, operation: opUpdateDeploymentStrategy}
	case http.MethodDelete:
		return appConfigRoute{strategyID: strategyID, operation: opDeleteDeploymentStrategy}
	}

	return appConfigRoute{strategyID: strategyID, operation: opUnknown}
}

// parseApplicationRoute parses routes starting with /applications.
//

func parseApplicationRoute(method string, parts []string) appConfigRoute {
	if len(parts) == 1 {
		switch method {
		case http.MethodPost:
			return appConfigRoute{operation: opCreateApplication}
		case http.MethodGet:
			return appConfigRoute{operation: opListApplications}
		}

		return appConfigRoute{operation: opUnknown}
	}

	appID := parts[1]

	if len(parts) == pathPartsBase {
		return parseAppIDRoute(method, appID)
	}

	switch parts[2] {
	case "environments":
		return parseEnvironmentRoute(method, appID, parts)
	case "configurationprofiles":
		return parseConfigProfileRoute(method, appID, parts)
	case "experimentdefinitions":
		return parseExperimentDefinitionRoute(method, appID, parts)
	}

	return appConfigRoute{applicationID: appID, operation: opUnknown}
}

func parseAppIDRoute(method, appID string) appConfigRoute {
	switch method {
	case http.MethodGet:
		return appConfigRoute{applicationID: appID, operation: opGetApplication}
	case http.MethodPatch:
		return appConfigRoute{applicationID: appID, operation: opUpdateApplication}
	case http.MethodDelete:
		return appConfigRoute{applicationID: appID, operation: opDeleteApplication}
	}

	return appConfigRoute{applicationID: appID, operation: opUnknown}
}

func parseEnvironmentRoute(method, appID string, parts []string) appConfigRoute {
	if len(parts) == pathPartsSubLevel {
		switch method {
		case http.MethodPost:
			return appConfigRoute{applicationID: appID, operation: opCreateEnvironment}
		case http.MethodGet:
			return appConfigRoute{applicationID: appID, operation: opListEnvironments}
		}

		return appConfigRoute{applicationID: appID, operation: opUnknown}
	}

	envID := parts[3]

	if len(parts) == pathPartsSubItem {
		return parseEnvIDRoute(method, appID, envID)
	}

	if len(parts) >= pathPartsDeepLevel && parts[4] == "deployments" {
		return parseDeploymentRoute(method, appID, envID, parts)
	}

	if len(parts) == pathPartsDeepItem && parts[4] == "configurations" && method == http.MethodGet {
		return appConfigRoute{
			applicationID:   appID,
			environmentID:   envID,
			configurationID: parts[5],
			operation:       opGetConfiguration,
		}
	}

	return appConfigRoute{applicationID: appID, environmentID: envID, operation: opUnknown}
}

func parseEnvIDRoute(method, appID, envID string) appConfigRoute {
	switch method {
	case http.MethodGet:
		return appConfigRoute{
			applicationID: appID,
			environmentID: envID,
			operation:     opGetEnvironment,
		}
	case http.MethodPatch:
		return appConfigRoute{
			applicationID: appID,
			environmentID: envID,
			operation:     opUpdateEnvironment,
		}
	case http.MethodDelete:
		return appConfigRoute{
			applicationID: appID,
			environmentID: envID,
			operation:     opDeleteEnvironment,
		}
	}

	return appConfigRoute{applicationID: appID, environmentID: envID, operation: opUnknown}
}

// parseDeploymentRoute parses deployment routes under /environments/{envId}/deployments.
//
//nolint:dupl // similar structure to parseHostedVersionRoute by design; different resource fields
func parseDeploymentRoute(method, appID, envID string, parts []string) appConfigRoute {
	if len(parts) == pathPartsDeepLevel {
		switch method {
		case http.MethodPost:
			return appConfigRoute{
				applicationID: appID,
				environmentID: envID,
				operation:     opStartDeployment,
			}
		case http.MethodGet:
			return appConfigRoute{
				applicationID: appID,
				environmentID: envID,
				operation:     opListDeployments,
			}
		}

		return appConfigRoute{applicationID: appID, environmentID: envID, operation: opUnknown}
	}

	depNum, err := strconv.ParseInt(parts[5], 10, 32)
	if err != nil {
		return appConfigRoute{applicationID: appID, environmentID: envID, operation: opUnknown}
	}

	num := int32(depNum)

	switch method {
	case http.MethodGet:
		return appConfigRoute{
			applicationID: appID,
			environmentID: envID,
			deploymentNum: num,
			operation:     opGetDeployment,
		}
	case http.MethodDelete:
		return appConfigRoute{
			applicationID: appID,
			environmentID: envID,
			deploymentNum: num,
			operation:     opStopDeployment,
		}
	}

	return appConfigRoute{
		applicationID: appID,
		environmentID: envID,
		deploymentNum: num,
		operation:     opUnknown,
	}
}

// parseConfigProfileRoute parses configuration profile routes.
//

func parseConfigProfileRoute(method, appID string, parts []string) appConfigRoute {
	if len(parts) == pathPartsSubLevel {
		switch method {
		case http.MethodPost:
			return appConfigRoute{applicationID: appID, operation: opCreateConfigurationProfile}
		case http.MethodGet:
			return appConfigRoute{applicationID: appID, operation: opListConfigurationProfiles}
		}

		return appConfigRoute{applicationID: appID, operation: opUnknown}
	}

	profileID := parts[3]

	if len(parts) == pathPartsSubItem {
		return parseProfileIDRoute(method, appID, profileID)
	}

	if len(parts) >= pathPartsDeepLevel && parts[4] == "hostedconfigurationversions" {
		return parseHostedVersionRoute(method, appID, profileID, parts)
	}

	if len(parts) == pathPartsDeepLevel && parts[4] == "validators" && method == http.MethodPost {
		return appConfigRoute{
			applicationID: appID,
			profileID:     profileID,
			operation:     opValidateConfiguration,
		}
	}

	return appConfigRoute{applicationID: appID, profileID: profileID, operation: opUnknown}
}

func parseProfileIDRoute(method, appID, profileID string) appConfigRoute {
	switch method {
	case http.MethodGet:
		return appConfigRoute{
			applicationID: appID,
			profileID:     profileID,
			operation:     opGetConfigurationProfile,
		}
	case http.MethodPatch:
		return appConfigRoute{
			applicationID: appID,
			profileID:     profileID,
			operation:     opUpdateConfigurationProfile,
		}
	case http.MethodDelete:
		return appConfigRoute{
			applicationID: appID,
			profileID:     profileID,
			operation:     opDeleteConfigurationProfile,
		}
	}

	return appConfigRoute{applicationID: appID, profileID: profileID, operation: opUnknown}
}

// parseHostedVersionRoute parses hosted configuration version routes.
//
//nolint:dupl // similar structure to parseDeploymentRoute by design; different resource fields
func parseHostedVersionRoute(method, appID, profileID string, parts []string) appConfigRoute {
	if len(parts) == pathPartsDeepLevel {
		switch method {
		case http.MethodPost:
			return appConfigRoute{
				applicationID: appID,
				profileID:     profileID,
				operation:     opCreateHostedConfigurationVersion,
			}
		case http.MethodGet:
			return appConfigRoute{
				applicationID: appID,
				profileID:     profileID,
				operation:     opListHostedConfigurationVersions,
			}
		}

		return appConfigRoute{applicationID: appID, profileID: profileID, operation: opUnknown}
	}

	verNum, err := strconv.ParseInt(parts[5], 10, 32)
	if err != nil {
		return appConfigRoute{applicationID: appID, profileID: profileID, operation: opUnknown}
	}

	num := int32(verNum)

	switch method {
	case http.MethodGet:
		return appConfigRoute{
			applicationID: appID,
			profileID:     profileID,
			versionNumber: num,
			operation:     opGetHostedConfigurationVersion,
		}
	case http.MethodDelete:
		return appConfigRoute{
			applicationID: appID,
			profileID:     profileID,
			versionNumber: num,
			operation:     opDeleteHostedConfigurationVersion,
		}
	}

	return appConfigRoute{
		applicationID: appID,
		profileID:     profileID,
		versionNumber: num,
		operation:     opUnknown,
	}
}

// parseExperimentDefinitionRoute parses routes under
// /applications/{appId}/experimentdefinitions. Unlike every sibling
// sub-resource in this file there is no GET at the collection level here:
// real ListExperimentDefinitions is account-wide ("/experimentdefinitions",
// no application prefix -- see the top-level "experimentdefinitions" case
// in parseAppConfigPath), so only POST (create) is valid at len(parts)==3.
func parseExperimentDefinitionRoute(method, appID string, parts []string) appConfigRoute {
	if len(parts) == pathPartsSubLevel {
		if method == http.MethodPost {
			return appConfigRoute{applicationID: appID, operation: opCreateExperimentDefinition}
		}

		return appConfigRoute{applicationID: appID, operation: opUnknown}
	}

	defID := parts[3]

	if len(parts) == pathPartsSubItem {
		return parseExperimentDefinitionIDRoute(method, appID, defID)
	}

	if len(parts) >= pathPartsDeepLevel && parts[4] == "experimentruns" {
		return parseExperimentRunRoute(method, appID, defID, parts)
	}

	return appConfigRoute{applicationID: appID, experimentDefinitionID: defID, operation: opUnknown}
}

func parseExperimentDefinitionIDRoute(method, appID, defID string) appConfigRoute {
	base := appConfigRoute{applicationID: appID, experimentDefinitionID: defID}

	switch method {
	case http.MethodGet:
		base.operation = opGetExperimentDefinition
	case http.MethodPatch:
		base.operation = opUpdateExperimentDefinition
	case http.MethodDelete:
		base.operation = opDeleteExperimentDefinition
	default:
		base.operation = opUnknown
	}

	return base
}

// parseExperimentRunRoute parses routes under
// /applications/{appId}/experimentdefinitions/{defId}/experimentruns.
func parseExperimentRunRoute(method, appID, defID string, parts []string) appConfigRoute {
	base := appConfigRoute{applicationID: appID, experimentDefinitionID: defID}

	if len(parts) == pathPartsDeepLevel {
		return parseExperimentRunListRoute(method, base)
	}

	runNum, err := strconv.ParseInt(parts[5], 10, 32)
	if err != nil {
		base.operation = opUnknown

		return base
	}

	base.experimentRunNumber = int32(runNum)

	switch {
	case len(parts) == pathPartsDeepItem:
		return parseExperimentRunIDRoute(method, base)
	case len(parts) == pathPartsDeeperItem:
		return parseExperimentRunActionRoute(method, parts[6], base)
	default:
		base.operation = opUnknown

		return base
	}
}

// parseExperimentRunListRoute parses
// .../experimentruns (no run number): POST starts a new run, GET lists them.
func parseExperimentRunListRoute(method string, base appConfigRoute) appConfigRoute {
	switch method {
	case http.MethodPost:
		base.operation = opStartExperimentRun
	case http.MethodGet:
		base.operation = opListExperimentRuns
	default:
		base.operation = opUnknown
	}

	return base
}

// parseExperimentRunIDRoute parses .../experimentruns/{run} (GET only --
// stop/update are separate action-suffixed paths, see
// parseExperimentRunActionRoute).
func parseExperimentRunIDRoute(method string, base appConfigRoute) appConfigRoute {
	if method == http.MethodGet {
		base.operation = opGetExperimentRun
	} else {
		base.operation = opUnknown
	}

	return base
}

// parseExperimentRunActionRoute parses
// .../experimentruns/{run}/{stop|update|events}.
func parseExperimentRunActionRoute(method, action string, base appConfigRoute) appConfigRoute {
	switch {
	case method == http.MethodPatch && action == "stop":
		base.operation = opStopExperimentRun
	case method == http.MethodPatch && action == "update":
		base.operation = opUpdateExperimentRun
	case method == http.MethodGet && action == "events":
		base.operation = opListExperimentRunEvents
	default:
		base.operation = opUnknown
	}

	return base
}

// appConfigDispatchFn is the function signature for AppConfig dispatch handlers.
type appConfigDispatchFn func(*Handler, *echo.Context, appConfigRoute) error

// appConfigDispatch maps operation names to their handler wrappers.
//
//nolint:gochecknoglobals // read-only dispatch table initialized once at startup
var appConfigDispatch = map[string]appConfigDispatchFn{
	opCreateApplication: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleCreateApplication(c)
	},
	opGetApplication: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetApplication(c, r.applicationID)
	},
	opListApplications: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleListApplications(c)
	},
	opUpdateApplication: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateApplication(c, r.applicationID)
	},
	opDeleteApplication: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteApplication(c, r.applicationID)
	},
	opCreateEnvironment: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleCreateEnvironment(c, r.applicationID)
	},
	opGetEnvironment: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetEnvironment(c, r.applicationID, r.environmentID)
	},
	opListEnvironments: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleListEnvironments(c, r.applicationID)
	},
	opUpdateEnvironment: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateEnvironment(c, r.applicationID, r.environmentID)
	},
	opDeleteEnvironment: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteEnvironment(c, r.applicationID, r.environmentID)
	},
	opCreateConfigurationProfile: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleCreateConfigurationProfile(c, r.applicationID)
	},
	opGetConfigurationProfile: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetConfigurationProfile(c, r.applicationID, r.profileID)
	},
	opListConfigurationProfiles: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleListConfigurationProfiles(c, r.applicationID)
	},
	opUpdateConfigurationProfile: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateConfigurationProfile(c, r.applicationID, r.profileID)
	},
	opDeleteConfigurationProfile: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteConfigurationProfile(c, r.applicationID, r.profileID)
	},
	opCreateHostedConfigurationVersion: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleCreateHostedConfigurationVersion(c, r.applicationID, r.profileID)
	},
	opGetHostedConfigurationVersion: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetHostedConfigurationVersion(c, r.applicationID, r.profileID, r.versionNumber)
	},
	opListHostedConfigurationVersions: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleListHostedConfigurationVersions(c, r.applicationID, r.profileID)
	},
	opDeleteHostedConfigurationVersion: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteHostedConfigurationVersion(c, r.applicationID, r.profileID, r.versionNumber)
	},
	opCreateDeploymentStrategy: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleCreateDeploymentStrategy(c)
	},
	opGetDeploymentStrategy: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetDeploymentStrategy(c, r.strategyID)
	},
	opListDeploymentStrategies: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleListDeploymentStrategies(c)
	},
	opUpdateDeploymentStrategy: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateDeploymentStrategy(c, r.strategyID)
	},
	opDeleteDeploymentStrategy: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteDeploymentStrategy(c, r.strategyID)
	},
	opStartDeployment: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleStartDeployment(c, r.applicationID, r.environmentID)
	},
	opGetDeployment: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetDeployment(c, r.applicationID, r.environmentID, r.deploymentNum)
	},
	opListDeployments: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleListDeployments(c, r.applicationID, r.environmentID)
	},
	opStopDeployment: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleStopDeployment(c, r.applicationID, r.environmentID, r.deploymentNum)
	},
	opListTagsForResource: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleListTagsForResource(c, r.resourceArn)
	},
	opTagResource: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleTagResource(c, r.resourceArn)
	},
	opUntagResource: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUntagResource(c, r.resourceArn)
	},
	opCreateExtension: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleCreateExtension(c)
	},
	opGetExtension: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetExtension(c, r.extensionID)
	},
	opListExtensions: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleListExtensions(c)
	},
	opUpdateExtension: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateExtension(c, r.extensionID)
	},
	opDeleteExtension: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteExtension(c, r.extensionID)
	},
	opCreateExtensionAssociation: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleCreateExtensionAssociation(c)
	},
	opGetExtensionAssociation: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetExtensionAssociation(c, r.extensionAssociationID)
	},
	opListExtensionAssociations: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleListExtensionAssociations(c)
	},
	opUpdateExtensionAssociation: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateExtensionAssociation(c, r.extensionAssociationID)
	},
	opDeleteExtensionAssociation: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteExtensionAssociation(c, r.extensionAssociationID)
	},
	opGetAccountSettings: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleGetAccountSettings(c)
	},
	opUpdateAccountSettings: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleUpdateAccountSettings(c)
	},
	opGetConfiguration: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetConfiguration(c, r.applicationID, r.environmentID, r.configurationID)
	},
	opValidateConfiguration: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleValidateConfiguration(c, r.applicationID, r.profileID)
	},
	opCreateExperimentDefinition: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleCreateExperimentDefinition(c, r.applicationID)
	},
	opGetExperimentDefinition: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetExperimentDefinition(c, r.applicationID, r.experimentDefinitionID)
	},
	opListExperimentDefinitions: func(h *Handler, c *echo.Context, _ appConfigRoute) error {
		return h.handleListExperimentDefinitions(c)
	},
	opUpdateExperimentDefinition: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateExperimentDefinition(c, r.applicationID, r.experimentDefinitionID)
	},
	opDeleteExperimentDefinition: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleDeleteExperimentDefinition(c, r.applicationID, r.experimentDefinitionID)
	},
	opStartExperimentRun: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleStartExperimentRun(c, r.applicationID, r.experimentDefinitionID)
	},
	opGetExperimentRun: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleGetExperimentRun(c, r.applicationID, r.experimentDefinitionID, r.experimentRunNumber)
	},
	opListExperimentRuns: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleListExperimentRuns(c, r.applicationID, r.experimentDefinitionID)
	},
	opUpdateExperimentRun: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleUpdateExperimentRun(c, r.applicationID, r.experimentDefinitionID, r.experimentRunNumber)
	},
	opStopExperimentRun: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleStopExperimentRun(c, r.applicationID, r.experimentDefinitionID, r.experimentRunNumber)
	},
	opListExperimentRunEvents: func(h *Handler, c *echo.Context, r appConfigRoute) error {
		return h.handleListExperimentRunEvents(c, r.applicationID, r.experimentDefinitionID, r.experimentRunNumber)
	},
}

// Handler returns the Echo handler function for AppConfig operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := logger.Load(c.Request().Context())
		route := parseAppConfigPath(c.Request().Method, c.Request().URL.Path)

		if fn, ok := appConfigDispatch[route.operation]; ok {
			return fn(h, c, route)
		}

		log.Warn(
			"appconfig: unmatched route",
			"method",
			c.Request().Method,
			"path",
			c.Request().URL.Path,
		)

		return c.JSON(http.StatusNotFound, map[string]string{keyMessageField: "not found"})
	}
}

// amznErrorTypeHeader carries the modeled exception type for the restjson1
// protocol. aws-sdk-go-v2's restjson.GetErrorInfo (aws/protocol/restjson/decoder_util.go)
// reads this header before falling back to a body "code"/"__type" field; without it every
// error here deserialized client-side as a generic UnknownError.
const amznErrorTypeHeader = "X-Amzn-Errortype"

// Wire types below are verified per call site against this service's own
// deserializer error lists (appconfig@v1.48.4 deserializers.go), which use
// strings.EqualFold comparisons rather than literal case labels. Not every
// operation models every one of these codes -- see the callers of
// conflictResponse for the operations that don't model ConflictException.
func notFoundResponse(c *echo.Context, err error) error {
	c.Response().Header().Set(amznErrorTypeHeader, "ResourceNotFoundException")

	return c.JSON(http.StatusNotFound, map[string]string{keyMessageField: err.Error()})
}

// deletionProtectionCheckHeader is where DeleteEnvironmentInput/
// DeleteConfigurationProfileInput.DeletionProtectionCheck actually lives on
// the wire: a header, not the body or query string (appconfig@v1.48.4
// serializers.go:1121 and :1268).
const deletionProtectionCheckHeader = "X-Amzn-Deletion-Protection-Check"

// validDeletionProtectionChecks are the only values types.DeletionProtectionCheck
// accepts (appconfig@v1.48.4 types/enums.go:93-95).
var validDeletionProtectionChecks = map[string]bool{ //nolint:gochecknoglobals // compile-time constant map
	"BYPASS":          true,
	"APPLY":           true,
	"ACCOUNT_DEFAULT": true,
}

// rejectInvalidDeletionProtectionCheck validates the DeletionProtectionCheck
// header on DeleteEnvironment/DeleteConfigurationProfile, if present, writing
// a BadRequestException response and reporting rejected=true for a value
// outside the enum.
//
// It does not enforce deletion protection itself: doing so needs a record of
// recent appconfigdata GetLatestConfiguration calls this backend has no way
// to produce (see PARITY.md's deletion_protection_check gap). BYPASS, APPLY,
// and ACCOUNT_DEFAULT are therefore all accepted and all behave like an
// absent header -- deletes are never blocked -- but a value real AppConfig
// would reject is no longer silently accepted.
func rejectInvalidDeletionProtectionCheck(c *echo.Context) (bool, error) {
	value := c.Request().Header.Get(deletionProtectionCheckHeader)
	if value == "" || validDeletionProtectionChecks[value] {
		return false, nil
	}

	respErr := badRequestResponse(c, fmt.Errorf(
		"%w: DeletionProtectionCheck %q is not a recognized value", awserr.ErrInvalidParameter, value,
	))

	return true, respErr
}

func badRequestResponse(c *echo.Context, err error) error {
	c.Response().Header().Set(amznErrorTypeHeader, "BadRequestException")

	return c.JSON(http.StatusBadRequest, map[string]string{keyMessageField: err.Error()})
}

func conflictResponse(c *echo.Context, err error) error {
	c.Response().Header().Set(amznErrorTypeHeader, "ConflictException")

	return c.JSON(http.StatusConflict, map[string]string{keyMessageField: err.Error()})
}

// payloadTooLargeResponse is only valid on operations that model
// PayloadTooLargeException (CreateHostedConfigurationVersion).
func payloadTooLargeResponse(c *echo.Context, err error) error {
	c.Response().Header().Set(amznErrorTypeHeader, "PayloadTooLargeException")

	return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{keyMessageField: err.Error()})
}

// internalServerErrorResponse is valid on every AppConfig operation --
// InternalServerException is modeled on all of them.
func internalServerErrorResponse(c *echo.Context, err error) error {
	c.Response().Header().Set(amznErrorTypeHeader, "InternalServerException")

	return c.JSON(http.StatusInternalServerError, map[string]string{keyMessageField: err.Error()})
}

// appConfigPaginationParams reads the next_token and max_results query parameters.
func appConfigPaginationParams(c *echo.Context) (string, int) {
	q := c.Request().URL.Query()
	nextToken := q.Get("next_token")

	maxResults := 0

	if s := q.Get("max_results"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxResults = n
		}
	}

	return nextToken, maxResults
}
