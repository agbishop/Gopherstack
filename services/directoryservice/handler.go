package directoryservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	matchPriority = service.PriorityHeaderPartial

	targetPrefix = "DirectoryService_20150416."

	opCreateDirectory        = "CreateDirectory"
	opCreateMicrosoftAD      = "CreateMicrosoftAD"
	opDeleteDirectory        = "DeleteDirectory"
	opDescribeDirectories    = "DescribeDirectories"
	opCreateAlias            = "CreateAlias"
	opEnableSso              = "EnableSso"
	opDisableSso             = "DisableSso"
	opGetDirectoryLimits     = "GetDirectoryLimits"
	opCreateSnapshot         = "CreateSnapshot"
	opDeleteSnapshot         = "DeleteSnapshot"
	opDescribeSnapshots      = "DescribeSnapshots"
	opGetSnapshotLimits      = "GetSnapshotLimits"
	opRestoreFromSnapshot    = "RestoreFromSnapshot"
	opAddTagsToResource      = "AddTagsToResource"
	opRemoveTagsFromResource = "RemoveTagsFromResource"
	opListTagsForResource    = "ListTagsForResource"
	opUnknown                = "Unknown"

	keyDirectoryID         = "DirectoryId"
	keySnapshotID          = "SnapshotId"
	keyLaunchTime          = "LaunchTime"
	keyStartTime           = "StartTime"
	keyStatus              = "Status"
	keyRegion              = "Region"
	keyRequestID           = "RequestId"
	keyAssessmentID        = "AssessmentId"
	keyLastUpdatedDateTime = "LastUpdatedDateTime"
	keyCreatedDateTime     = "CreatedDateTime"

	keyRemoteDomainName = "RemoteDomainName"
	keyTopicName        = "TopicName"
	keyVpcID            = "VpcId"
	keySubnetIDs        = "SubnetIds"

	opAcceptSharedDirectory                = "AcceptSharedDirectory"
	opAddIpRoutes                          = "AddIpRoutes" //nolint:revive,staticcheck // existing issue.
	opAddRegion                            = "AddRegion"
	opCancelSchemaExtension                = "CancelSchemaExtension"
	opConnectDirectory                     = "ConnectDirectory"
	opCreateComputer                       = "CreateComputer"
	opCreateConditionalForwarder           = "CreateConditionalForwarder"
	opCreateHybridAD                       = "CreateHybridAD"
	opCreateLogSubscription                = "CreateLogSubscription"
	opCreateTrust                          = "CreateTrust"
	opDeleteADAssessment                   = "DeleteADAssessment"
	opDeleteConditionalForwarder           = "DeleteConditionalForwarder"
	opDeleteLogSubscription                = "DeleteLogSubscription"
	opDeleteTrust                          = "DeleteTrust"
	opDeregisterCertificate                = "DeregisterCertificate"
	opDeregisterEventTopic                 = "DeregisterEventTopic"
	opDescribeADAssessment                 = "DescribeADAssessment"
	opDescribeCAEnrollmentPolicy           = "DescribeCAEnrollmentPolicy"
	opDescribeCertificate                  = "DescribeCertificate"
	opDescribeClientAuthenticationSettings = "DescribeClientAuthenticationSettings"
	opDescribeConditionalForwarders        = "DescribeConditionalForwarders"
	opDescribeDirectoryDataAccess          = "DescribeDirectoryDataAccess"
	opDescribeDomainControllers            = "DescribeDomainControllers"
	opDescribeEventTopics                  = "DescribeEventTopics"
	opDescribeHybridADUpdate               = "DescribeHybridADUpdate"
	opDescribeLDAPSSettings                = "DescribeLDAPSSettings"
	opDescribeRegions                      = "DescribeRegions"
	opDescribeSettings                     = "DescribeSettings"
	opDescribeSharedDirectories            = "DescribeSharedDirectories"
	opDescribeTrusts                       = "DescribeTrusts"
	opDescribeUpdateDirectory              = "DescribeUpdateDirectory"
	opDisableCAEnrollmentPolicy            = "DisableCAEnrollmentPolicy"
	opDisableClientAuthentication          = "DisableClientAuthentication"
	opDisableDirectoryDataAccess           = "DisableDirectoryDataAccess"
	opDisableLDAPS                         = "DisableLDAPS"
	opDisableRadius                        = "DisableRadius"
	opEnableCAEnrollmentPolicy             = "EnableCAEnrollmentPolicy"
	opEnableClientAuthentication           = "EnableClientAuthentication"
	opEnableDirectoryDataAccess            = "EnableDirectoryDataAccess"
	opEnableLDAPS                          = "EnableLDAPS"
	opEnableRadius                         = "EnableRadius"
	opListADAssessments                    = "ListADAssessments"
	opListCertificates                     = "ListCertificates"
	opListIpRoutes                         = "ListIpRoutes" //nolint:revive,staticcheck // existing issue.
	opListLogSubscriptions                 = "ListLogSubscriptions"
	opListSchemaExtensions                 = "ListSchemaExtensions"
	opRegisterCertificate                  = "RegisterCertificate"
	opRegisterEventTopic                   = "RegisterEventTopic"
	opRejectSharedDirectory                = "RejectSharedDirectory"
	opRemoveIpRoutes                       = "RemoveIpRoutes" //nolint:revive,staticcheck // existing issue.
	opRemoveRegion                         = "RemoveRegion"
	opResetUserPassword                    = "ResetUserPassword"
	opShareDirectory                       = "ShareDirectory"
	opStartADAssessment                    = "StartADAssessment"
	opStartSchemaExtension                 = "StartSchemaExtension"
	opUnshareDirectory                     = "UnshareDirectory"
	opUpdateConditionalForwarder           = "UpdateConditionalForwarder"
	opUpdateDirectorySetup                 = "UpdateDirectorySetup"
	opUpdateHybridAD                       = "UpdateHybridAD"
	opUpdateNumberOfDomainControllers      = "UpdateNumberOfDomainControllers"
	opUpdateRadius                         = "UpdateRadius"
	opUpdateSettings                       = "UpdateSettings"
	opUpdateTrust                          = "UpdateTrust"
	opVerifyTrust                          = "VerifyTrust"
)

// Handler handles DirectoryService HTTP requests.
type Handler struct {
	Backend  StorageBackend
	dispatch map[string]echo.HandlerFunc
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	h := &Handler{Backend: b}
	h.dispatch = map[string]echo.HandlerFunc{
		opCreateDirectory:        h.handleCreateDirectory,
		opCreateMicrosoftAD:      h.handleCreateMicrosoftAD,
		opDeleteDirectory:        h.handleDeleteDirectory,
		opDescribeDirectories:    h.handleDescribeDirectories,
		opCreateAlias:            h.handleCreateAlias,
		opEnableSso:              h.handleEnableSso,
		opDisableSso:             h.handleDisableSso,
		opGetDirectoryLimits:     h.handleGetDirectoryLimits,
		opCreateSnapshot:         h.handleCreateSnapshot,
		opDeleteSnapshot:         h.handleDeleteSnapshot,
		opDescribeSnapshots:      h.handleDescribeSnapshots,
		opGetSnapshotLimits:      h.handleGetSnapshotLimits,
		opRestoreFromSnapshot:    h.handleRestoreFromSnapshot,
		opAddTagsToResource:      h.handleAddTagsToResource,
		opRemoveTagsFromResource: h.handleRemoveTagsFromResource,
		opListTagsForResource:    h.handleListTagsForResource,

		opAcceptSharedDirectory:                h.handleAcceptSharedDirectory,
		opAddIpRoutes:                          h.handleAddIpRoutes,
		opAddRegion:                            h.handleAddRegion,
		opCancelSchemaExtension:                h.handleCancelSchemaExtension,
		opConnectDirectory:                     h.handleConnectDirectory,
		opCreateComputer:                       h.handleCreateComputer,
		opCreateConditionalForwarder:           h.handleCreateConditionalForwarder,
		opCreateHybridAD:                       h.handleCreateHybridAD,
		opCreateLogSubscription:                h.handleCreateLogSubscription,
		opCreateTrust:                          h.handleCreateTrust,
		opDeleteADAssessment:                   h.handleDeleteADAssessment,
		opDeleteConditionalForwarder:           h.handleDeleteConditionalForwarder,
		opDeleteLogSubscription:                h.handleDeleteLogSubscription,
		opDeleteTrust:                          h.handleDeleteTrust,
		opDeregisterCertificate:                h.handleDeregisterCertificate,
		opDeregisterEventTopic:                 h.handleDeregisterEventTopic,
		opDescribeADAssessment:                 h.handleDescribeADAssessment,
		opDescribeCAEnrollmentPolicy:           h.handleDescribeCAEnrollmentPolicy,
		opDescribeCertificate:                  h.handleDescribeCertificate,
		opDescribeClientAuthenticationSettings: h.handleDescribeClientAuthenticationSettings,
		opDescribeConditionalForwarders:        h.handleDescribeConditionalForwarders,
		opDescribeDirectoryDataAccess:          h.handleDescribeDirectoryDataAccess,
		opDescribeDomainControllers:            h.handleDescribeDomainControllers,
		opDescribeEventTopics:                  h.handleDescribeEventTopics,
		opDescribeHybridADUpdate:               h.handleDescribeHybridADUpdate,
		opDescribeLDAPSSettings:                h.handleDescribeLDAPSSettings,
		opDescribeRegions:                      h.handleDescribeRegions,
		opDescribeSettings:                     h.handleDescribeSettings,
		opDescribeSharedDirectories:            h.handleDescribeSharedDirectories,
		opDescribeTrusts:                       h.handleDescribeTrusts,
		opDescribeUpdateDirectory:              h.handleDescribeUpdateDirectory,
		opDisableCAEnrollmentPolicy:            h.handleDisableCAEnrollmentPolicy,
		opDisableClientAuthentication:          h.handleDisableClientAuthentication,
		opDisableDirectoryDataAccess:           h.handleDisableDirectoryDataAccess,
		opDisableLDAPS:                         h.handleDisableLDAPS,
		opDisableRadius:                        h.handleDisableRadius,
		opEnableCAEnrollmentPolicy:             h.handleEnableCAEnrollmentPolicy,
		opEnableClientAuthentication:           h.handleEnableClientAuthentication,
		opEnableDirectoryDataAccess:            h.handleEnableDirectoryDataAccess,
		opEnableLDAPS:                          h.handleEnableLDAPS,
		opEnableRadius:                         h.handleEnableRadius,
		opListADAssessments:                    h.handleListADAssessments,
		opListCertificates:                     h.handleListCertificates,
		opListIpRoutes:                         h.handleListIpRoutes,
		opListLogSubscriptions:                 h.handleListLogSubscriptions,
		opListSchemaExtensions:                 h.handleListSchemaExtensions,
		opRegisterCertificate:                  h.handleRegisterCertificate,
		opRegisterEventTopic:                   h.handleRegisterEventTopic,
		opRejectSharedDirectory:                h.handleRejectSharedDirectory,
		opRemoveIpRoutes:                       h.handleRemoveIpRoutes,
		opRemoveRegion:                         h.handleRemoveRegion,
		opResetUserPassword:                    h.handleResetUserPassword,
		opShareDirectory:                       h.handleShareDirectory,
		opStartADAssessment:                    h.handleStartADAssessment,
		opStartSchemaExtension:                 h.handleStartSchemaExtension,
		opUnshareDirectory:                     h.handleUnshareDirectory,
		opUpdateConditionalForwarder:           h.handleUpdateConditionalForwarder,
		opUpdateDirectorySetup:                 h.handleUpdateDirectorySetup,
		opUpdateHybridAD:                       h.handleUpdateHybridAD,
		opUpdateNumberOfDomainControllers:      h.handleUpdateNumberOfDomainControllers,
		opUpdateRadius:                         h.handleUpdateRadius,
		opUpdateSettings:                       h.handleUpdateSettings,
		opUpdateTrust:                          h.handleUpdateTrust,
		opVerifyTrust:                          h.handleVerifyTrust,
	}

	return h
}

// Name returns the service name.
func (h *Handler) Name() string { return "DirectoryService" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// Snapshot returns a JSON snapshot of the backend state for persistence.
// Delegating this (and Restore below) is what makes the Handler satisfy the
// unexported persistence.Persistable-shaped interface cli.go's
// setupPersistence type-asserts services against; without it the backend's
// otherwise-complete BackendSnapshot/Restore implementation is never
// registered with the persistence manager and directoryservice state
// silently fails to survive a snapshot/restore cycle.
func (h *Handler) Snapshot(_ context.Context) []byte { return h.Backend.BackendSnapshot() }

// Restore restores the backend state from a snapshot.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateDirectory,
		opCreateMicrosoftAD,
		opDeleteDirectory,
		opDescribeDirectories,
		opCreateAlias,
		opEnableSso,
		opDisableSso,
		opGetDirectoryLimits,
		opCreateSnapshot,
		opDeleteSnapshot,
		opDescribeSnapshots,
		opGetSnapshotLimits,
		opRestoreFromSnapshot,
		opAddTagsToResource,
		opRemoveTagsFromResource,
		opListTagsForResource,

		opAcceptSharedDirectory,
		opAddIpRoutes,
		opAddRegion,
		opCancelSchemaExtension,
		opConnectDirectory,
		opCreateComputer,
		opCreateConditionalForwarder,
		opCreateHybridAD,
		opCreateLogSubscription,
		opCreateTrust,
		opDeleteADAssessment,
		opDeleteConditionalForwarder,
		opDeleteLogSubscription,
		opDeleteTrust,
		opDeregisterCertificate,
		opDeregisterEventTopic,
		opDescribeADAssessment,
		opDescribeCAEnrollmentPolicy,
		opDescribeCertificate,
		opDescribeClientAuthenticationSettings,
		opDescribeConditionalForwarders,
		opDescribeDirectoryDataAccess,
		opDescribeDomainControllers,
		opDescribeEventTopics,
		opDescribeHybridADUpdate,
		opDescribeLDAPSSettings,
		opDescribeRegions,
		opDescribeSettings,
		opDescribeSharedDirectories,
		opDescribeTrusts,
		opDescribeUpdateDirectory,
		opDisableCAEnrollmentPolicy,
		opDisableClientAuthentication,
		opDisableDirectoryDataAccess,
		opDisableLDAPS,
		opDisableRadius,
		opEnableCAEnrollmentPolicy,
		opEnableClientAuthentication,
		opEnableDirectoryDataAccess,
		opEnableLDAPS,
		opEnableRadius,
		opListADAssessments,
		opListCertificates,
		opListIpRoutes,
		opListLogSubscriptions,
		opListSchemaExtensions,
		opRegisterCertificate,
		opRegisterEventTopic,
		opRejectSharedDirectory,
		opRemoveIpRoutes,
		opRemoveRegion,
		opResetUserPassword,
		opShareDirectory,
		opStartADAssessment,
		opStartSchemaExtension,
		opUnshareDirectory,
		opUpdateConditionalForwarder,
		opUpdateDirectorySetup,
		opUpdateHybridAD,
		opUpdateNumberOfDomainControllers,
		opUpdateRadius,
		opUpdateSettings,
		opUpdateTrust,
		opVerifyTrust,
	}
}

// RouteMatcher returns a matcher that accepts DirectoryService requests by X-Amz-Target header.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, targetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return matchPriority }

// ExtractOperation extracts the operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	op, _ := strings.CutPrefix(target, targetPrefix)

	if op == "" {
		return opUnknown
	}

	return op
}

// ExtractResource extracts the directory ID from the request body when present.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	for _, key := range []string{keyDirectoryID, "ResourceId"} {
		if v, ok := data[key].(string); ok && v != "" {
			return v
		}
	}

	return ""
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return h.doDispatch(c)
	}
}

func (h *Handler) doDispatch(c *echo.Context) error {
	op := h.ExtractOperation(c)
	if fn, ok := h.dispatch[op]; ok {
		return fn(c)
	}

	// AWS rejects an unrecognised operation with a 400 client error rather than 501.
	return c.JSON(http.StatusBadRequest, errResp("InvalidRequestException", "unrecognized operation: "+op))
}

// contextWithRegion returns the request context with the resolved AWS region attached
// under regionContextKey so that backend operations are routed to the correct region.
// The SigV4 credential-scope region in the Authorization header (extracted by
// httputils.ExtractRegionFromRequest) takes precedence over the backend default.
func (h *Handler) contextWithRegion(c *echo.Context) context.Context {
	region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())

	return context.WithValue(c.Request().Context(), regionContextKey{}, region)
}

// twoFieldOp describes an operation that takes a directory ID plus one secondary
// string field and returns only an error. It centralises the identical request
// parsing, validation and error mapping shared by several handlers across
// resource families.
type twoFieldOp struct {
	invoke    func(ctx context.Context, dirID, second string) error
	secondKey string // JSON key (and human label) of the secondary field
}

// handleTwoFieldOp parses {DirectoryId, <secondKey>} from the request body,
// validates both are present, resolves the request region and invokes op.
func (h *Handler) handleTwoFieldOp(c *echo.Context, op twoFieldOp) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errResp("ClientException", "invalid body"))
	}

	var raw map[string]json.RawMessage
	if jsonErr := json.Unmarshal(body, &raw); jsonErr != nil {
		return c.JSON(http.StatusBadRequest, errResp("ClientException", "invalid JSON"))
	}

	dirID := jsonString(raw, keyDirectoryID)
	second := jsonString(raw, op.secondKey)

	if dirID == "" || second == "" {
		msg := "DirectoryId and " + op.secondKey + " are required"

		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", msg))
	}

	if opErr := op.invoke(h.contextWithRegion(c), dirID, second); opErr != nil {
		return h.mapError(c, opErr)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

// jsonString returns the string value stored under key in raw, or "" if absent
// or not a JSON string.
func jsonString(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}

	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}

	return s
}

func (h *Handler) mapError(c *echo.Context, err error) error {
	logger.Load(c.Request().Context()).Error("directoryservice error", "error", err)

	switch {
	case errors.Is(err, ErrInvalidCertificate):
		return c.JSON(http.StatusBadRequest, errResp("InvalidCertificateException", err.Error()))
	case errors.Is(err, ErrDirectoryLimitExceeded):
		return c.JSON(http.StatusBadRequest, errResp("DirectoryLimitExceededException", err.Error()))
	case errors.Is(err, ErrSnapshotLimitExceeded):
		return c.JSON(http.StatusBadRequest, errResp("SnapshotLimitExceededException", err.Error()))
	case errors.Is(err, ErrUnsupportedOperation):
		return c.JSON(http.StatusBadRequest, errResp("UnsupportedOperationException", err.Error()))
	case errors.Is(err, ErrDirectoryNotFoundDDNE):
		return c.JSON(http.StatusBadRequest, errResp("DirectoryDoesNotExistException", err.Error()))
	case errors.Is(err, ErrDirectoryAlreadyInRegion):
		return c.JSON(http.StatusBadRequest, errResp("DirectoryAlreadyInRegionException", err.Error()))
	case errors.Is(err, ErrCertificateDoesNotExist):
		return c.JSON(http.StatusBadRequest, errResp("CertificateDoesNotExistException", err.Error()))
	case errors.Is(err, ErrInvalidLDAPSStatus):
		return c.JSON(http.StatusBadRequest, errResp("InvalidLDAPSStatusException", err.Error()))
	case errors.Is(err, ErrInvalidClientAuthStatus):
		return c.JSON(http.StatusBadRequest, errResp("InvalidClientAuthStatusException", err.Error()))
	case errors.Is(err, awserr.ErrNotFound):
		return c.JSON(http.StatusBadRequest, errResp("EntityDoesNotExistException", err.Error()))
	case errors.Is(err, awserr.ErrAlreadyExists):
		return c.JSON(http.StatusBadRequest, errResp("EntityAlreadyExistsException", err.Error()))
	case errors.Is(err, awserr.ErrInvalidParameter):
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", err.Error()))
	case errors.Is(err, awserr.ErrConflict):
		return c.JSON(http.StatusBadRequest, errResp("ClientException", err.Error()))
	default:
		return c.JSON(http.StatusInternalServerError, errResp("ServiceException", err.Error()))
	}
}

// validEnum reports whether value is empty or matches one of allowed. It is used
// to validate free-form request fields that correspond to a constrained AWS enum
// (e.g. TrustDirection, LDAPSType) before invoking backend logic, matching the
// InvalidParameterException AWS returns for out-of-range enum values.
func validEnum(value string, allowed ...string) bool {
	if value == "" {
		return true
	}

	return slices.Contains(allowed, value)
}

func errResp(code, message string) map[string]string {
	return map[string]string{
		"__type":  code,
		"message": message,
	}
}

func directoryVpcSettingsJSON(vs *DirectoryVpcSettings) map[string]any {
	if vs == nil {
		return nil
	}

	secGroupID := vs.SecurityGroupID
	if secGroupID == "" && len(vs.SecurityGroupIDs) > 0 {
		secGroupID = vs.SecurityGroupIDs[0]
	}
	azs := vs.AvailabilityZones
	if azs == nil {
		azs = []string{}
	}

	res := map[string]any{
		keyVpcID:            vs.VpcID,
		keySubnetIDs:        vs.SubnetIDs,
		"AvailabilityZones": azs,
	}
	if secGroupID != "" {
		res["SecurityGroupId"] = secGroupID
	}

	return res
}

func directoryConnectSettingsJSON(cs *DirectoryConnectSettingsDescription) map[string]any {
	if cs == nil {
		return nil
	}

	return map[string]any{
		"CustomerUserName":  cs.CustomerUserName,
		keyVpcID:            cs.VpcID,
		keySubnetIDs:        cs.SubnetIDs,
		"AvailabilityZones": cs.AvailabilityZones,
		"ConnectIps":        cs.ConnectIPs,
		"ConnectIpsV6":      cs.ConnectIPsV6,
	}
}

func directoryRadiusSettingsJSON(rs *RadiusSettingsDescription) map[string]any {
	if rs == nil {
		return nil
	}

	return map[string]any{
		"AuthenticationProtocol": rs.AuthenticationProtocol,
		"DisplayLabel":           rs.DisplayLabel,
		"SharedSecret":           rs.SharedSecret,
		"RadiusServers":          rs.RadiusServers,
		"RadiusServersIpv6":      rs.RadiusServersIPv6,
		"RadiusPort":             rs.RadiusPort,
		"RadiusRetries":          rs.RadiusRetries,
		"RadiusTimeout":          rs.RadiusTimeout,
		"UseSameUsername":        rs.UseSameUsername,
	}
}

func directoryRegionsInfoJSON(ri *RegionsInfo) map[string]any {
	if ri == nil {
		return nil
	}

	additional := ri.AdditionalRegions
	if additional == nil {
		additional = []string{}
	}

	return map[string]any{
		"PrimaryRegion":     ri.PrimaryRegion,
		"AdditionalRegions": additional,
	}
}

func directoryHybridSettingsJSON(hs *HybridSettingsDescription) map[string]any {
	if hs == nil {
		return nil
	}

	dnsIPs := hs.SelfManagedDNSIPAddrs
	if dnsIPs == nil {
		dnsIPs = []string{}
	}
	instanceIDs := hs.SelfManagedInstanceIDs
	if instanceIDs == nil {
		instanceIDs = []string{}
	}

	return map[string]any{
		"SelfManagedDnsIpAddrs":  dnsIPs,
		"SelfManagedInstanceIds": instanceIDs,
	}
}

func directoryToJSON(d *Directory) map[string]any {
	dnsIPAddrs := d.DNSIPAddrs
	if dnsIPAddrs == nil {
		dnsIPAddrs = []string{}
	}
	dnsIPv6Addrs := d.DNSIPv6Addrs
	if dnsIPv6Addrs == nil {
		dnsIPv6Addrs = []string{}
	}

	out := map[string]any{
		keyDirectoryID:             d.DirectoryID,
		"Name":                     d.Name, //nolint:goconst // existing issue.
		"ShortName":                d.ShortName,
		"Description":              d.Description, //nolint:goconst // existing issue.
		"Alias":                    d.Alias,
		"AccessUrl":                d.AccessURL,
		"Type":                     string(d.Type), //nolint:goconst // existing issue.
		"Stage":                    string(d.Stage),
		"Size":                     string(d.Size),
		"Edition":                  string(d.Edition),
		"NetworkType":              string(d.NetworkType),
		"SsoEnabled":               d.SsoEnabled,
		keyLaunchTime:              awstime.Epoch(d.LaunchTime),
		"StageLastUpdatedDateTime": awstime.Epoch(d.StageLastUpdatedDateTime),
		"DnsIpAddrs":               dnsIPAddrs,
		"DnsIpv6Addrs":             dnsIPv6Addrs,
	}
	if vs := directoryVpcSettingsJSON(d.VpcSettings); vs != nil {
		out["VpcSettings"] = vs
	}
	if cs := directoryConnectSettingsJSON(d.ConnectSettings); cs != nil {
		out["ConnectSettings"] = cs
	}
	if ri := directoryRegionsInfoJSON(d.RegionsInfo); ri != nil {
		out["RegionsInfo"] = ri
	}
	if rs := directoryRadiusSettingsJSON(d.RadiusSettings); rs != nil {
		out["RadiusSettings"] = rs
		out["RadiusStatus"] = string(d.RadiusStatus)
	}
	if d.DesiredNumberOfDomainControllers != nil {
		out["DesiredNumberOfDomainControllers"] = *d.DesiredNumberOfDomainControllers
	}
	if hs := directoryHybridSettingsJSON(d.HybridSettings); hs != nil {
		out["HybridSettings"] = hs
	}

	return out
}

func snapshotToJSON(s *Snapshot) map[string]any {
	return map[string]any{
		keySnapshotID:  s.SnapshotID,
		keyDirectoryID: s.DirectoryID,
		"Name":         s.Name,
		keyStatus:      string(s.Status),
		"Type":         string(s.Type),
		keyStartTime:   awstime.Epoch(s.StartTime),
	}
}

func reqTagsToTags(raw []struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
},
) []Tag {
	tags := make([]Tag, 0, len(raw))
	for _, t := range raw {
		tags = append(tags, Tag{Key: t.Key, Value: t.Value})
	}

	return tags
}
