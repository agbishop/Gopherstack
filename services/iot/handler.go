package iot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	iotMatchPriority = 90
	unknownOperation = "Unknown"
	// iotServiceName is the SigV4 signing name real IoT control-plane requests carry.
	iotServiceName = "iot"
	// headerIoTPrincipal is the HTTP header name for the IoT principal (certificate ARN or Cognito identity).
	headerIoTPrincipal = "X-Amzn-Principal"
)

// Handler is the Echo HTTP handler for IoT control-plane operations.
type Handler struct {
	Backend   StorageBackend
	broker    *Broker
	brokerRun worker.SingleRun
}

// NewHandler creates a new IoT Handler.
func NewHandler(backend StorageBackend, broker *Broker) *Handler {
	return &Handler{Backend: backend, broker: broker}
}

// Reset clears all backend state and resets the handler. Used for test isolation.
func (h *Handler) Reset() {
	if r, ok := h.Backend.(Resettable); ok {
		r.Reset()
	}
}

// Broker returns the embedded MQTT broker (used for cross-service wiring).
func (h *Handler) Broker() *Broker { return h.broker }

// Name returns the service name.
func (h *Handler) Name() string { return "IoT" }

// GetSupportedOperations returns the list of supported IoT control-plane operations.
func (h *Handler) GetSupportedOperations() []string {
	core := coreResourceOperationNames()
	core = append(core, extendedResourceOperationNames()...)
	core = append(core, deviceFleetOperationNames()...)
	core = append(core, thingRegistrationAndDeviceDefenderOperationNames()...)
	core = append(core, extendedOperationNames()...)

	return append(core, allStubOps()...)
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return iotServiceName }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this IoT instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path
		if path == pathPolicies || strings.HasPrefix(path, pathPolicies+"/") {
			svc := httputils.ExtractServiceFromRequest(c.Request())

			return svc == "" || svc == iotServiceName
		}

		// "/api/things/shadow/" (ListNamedShadowsForThing/ListThingsWithShadows) and
		// "/things/{name}/shadow..." (Get/Update/DeleteThingShadow) are entirely
		// iotdataplane's real wire paths (signs as "iotdata"); iot has no shadow
		// operations in the real API at all. Scope both by SigV4 so a
		// correctly-signed iotdataplane request isn't swallowed here (see
		// gopherstack-61i8).
		if strings.HasPrefix(path, "/api/things/shadow/") || isThingShadowPath(path) {
			svc := httputils.ExtractServiceFromRequest(c.Request())

			return svc == "" || svc == iotServiceName
		}

		return matchIoTPath(path)
	}
}

// MatchPriority returns the routing priority for the IoT handler.
func (h *Handler) MatchPriority() int { return iotMatchPriority }

// ExtractOperation extracts the IoT operation name from the request method + path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return resolveOperation(c.Request().URL.Path, c.Request().Method)
}

// maxPathSegments is used to split the path into at most 2 segments.
const maxPathSegments = 2

// ExtractResource extracts the resource name (thing/rule/policy) from the URL path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path

	for _, prefix := range []string{"/things/", "/rules/", "/policies/", "/target-policies/"} {
		if after, ok := strings.CutPrefix(path, prefix); ok {
			return strings.SplitN(after, "/", maxPathSegments)[0]
		}
	}

	for _, prefix := range []string{
		"/accept-certificate-transfer/",
		"/security-profiles/",
		"/jobs/",
		"/packages/",
		"/audit/mitigationactions/tasks/",
		"/audit/tasks/",
	} {
		if after, ok := strings.CutPrefix(path, prefix); ok {
			return strings.SplitN(after, "/", maxPathSegments)[0]
		}
	}

	return ""
}

// StartWorker starts the embedded MQTT broker as a background worker.
// Implements service.BackgroundWorker. The broker is run under a
// worker.SingleRun so Shutdown can deterministically wait for its goroutine
// to exit instead of only relying on ctx cancellation propagating.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.broker == nil {
		return nil
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "starting IoT MQTT broker", "port", h.broker.port)

	h.brokerRun.Start(ctx, h.broker)

	return nil
}

// Shutdown stops the embedded MQTT broker and waits for its goroutine to
// exit (or for ctx to be done), so no goroutine outlives the service.
// Invoked on server shutdown via service.Shutdowner.
func (h *Handler) Shutdown(ctx context.Context) {
	h.brokerRun.Stop(ctx)
}

// Ensure Handler implements service.BackgroundWorker and service.Shutdowner
// at compile time.
var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// Handler returns the Echo handler function for IoT operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := logger.Load(c.Request().Context())
		op := resolveOperation(c.Request().URL.Path, c.Request().Method)

		log.Debug("iot request", "operation", op, "path", c.Request().URL.Path)

		if handled, err := h.dispatchCoreOp(c, op); handled {
			return err
		}

		if handled, err := h.dispatchNewOp(c, op); handled {
			return err
		}

		return c.JSON(
			http.StatusBadRequest,
			awsErrBody{errTypeInvalidRequest, "unknown operation: " + op},
		)
	}
}

func (h *Handler) dispatchCoreOp(c *echo.Context, op string) (bool, error) {
	if handled, err := h.dispatchThingOps(c, op); handled {
		return true, err
	}

	if handled, err := h.dispatchRuleOps(c, op); handled {
		return true, err
	}

	return h.dispatchPolicyOps(c, op)
}

func (h *Handler) dispatchThingOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateThing:

		return true, h.handleCreateThing(c)
	case opDescribeThing:

		return true, h.handleDescribeThing(c)
	case opDeleteThing:

		return true, h.handleDeleteThing(c)
	case opUpdateThing:

		return true, h.handleUpdateThing(c)
	case opListThings:

		return true, h.handleListThings(c)
	case opListThingPrincipals:

		return true, h.handleListThingPrincipals(c)
	case opGetThingShadow:

		return true, h.handleGetThingShadow(c)
	case opUpdateThingShadow:

		return true, h.handleUpdateThingShadow(c)
	case opDeleteThingShadow:

		return true, h.handleDeleteThingShadow(c)
	case opListNamedShadowsForThing:

		return true, h.handleListNamedShadowsForThing(c)
	case opDetachThingPrincipal:

		return true, h.handleDetachThingPrincipal(c)
	}

	return false, nil
}

// handleError maps backend errors to appropriate HTTP responses.
// handleError maps a backend sentinel error to the AWS IoT restjson1 error
// shape and HTTP status. See writeIoTError (handler_helpers.go) for the
// canonical mapping shared with respondErr.
func (h *Handler) handleError(c *echo.Context, err error) error {
	return writeIoTError(c, err)
}

func (h *Handler) handleCreateThing(c *echo.Context) error {
	thingName := strings.TrimPrefix(c.Request().URL.Path, "/things/")

	var body struct {
		AttributePayload *AttributePayload `json:"attributePayload"`
		ThingTypeName    string            `json:"thingTypeName"`
		BillingGroupName string            `json:"billingGroupName"`
	}

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	out, err := h.Backend.CreateThing(&CreateThingInput{
		ThingName:        thingName,
		ThingTypeName:    body.ThingTypeName,
		AttributePayload: body.AttributePayload,
		BillingGroupName: body.BillingGroupName,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		keyThingName: out.ThingName,
		keyThingArn:  out.ThingARN,
		"thingId":    out.ThingID,
	})
}

func (h *Handler) handleDescribeThing(c *echo.Context) error {
	thingName := strings.TrimPrefix(c.Request().URL.Path, "/things/")

	t, err := h.Backend.DescribeThing(thingName)
	if err != nil {
		return h.handleError(c, err)
	}

	resp := map[string]any{
		keyThingName:      t.ThingName,
		keyThingArn:       t.ARN,
		"thingId":         t.ThingID,
		keyThingTypeName:  t.ThingTypeName,
		keyAttributes:     t.Attributes,
		keyVersion:        t.Version,
		"defaultClientId": t.ThingName,
	}
	if t.BillingGroupName != "" {
		resp["billingGroupName"] = t.BillingGroupName
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleDeleteThing(c *echo.Context) error {
	thingName := strings.TrimPrefix(c.Request().URL.Path, "/things/")
	expectedVersion := parseExpectedVersionQueryParam(c)

	if err := h.Backend.DeleteThing(thingName, expectedVersion); err != nil {
		// DeleteThing's own deserializeOpError switch declares no
		// DeleteConflictException case -- InvalidRequestException is the
		// real type. Its ResourceNotFoundException case IS declared, so
		// only ErrDeleteConflict needs the override.
		return respondAsInvalidRequest(c, err, ErrDeleteConflict)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleDescribeEndpoint(c *echo.Context) error {
	endpointType := c.QueryParam("endpointType")

	out, err := h.Backend.DescribeEndpoint(endpointType)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"endpointAddress": out.EndpointAddress,
	})
}

func (h *Handler) handleAcceptCertificateTransfer(c *echo.Context) error {
	certID := strings.TrimPrefix(c.Request().URL.Path, "/accept-certificate-transfer/")

	var body struct {
		SetAsActive bool `json:"setAsActive"`
	}

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	if err := h.Backend.AcceptCertificateTransfer(&AcceptCertificateTransferInput{
		CertificateID: certID,
		SetAsActive:   body.SetAsActive,
	}); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleAttachThingPrincipal(c *echo.Context) error {
	// Path: /things/{thingName}/principals
	after := strings.TrimPrefix(c.Request().URL.Path, "/things/")
	thingName, _, _ := strings.Cut(after, "/")
	principal := c.Request().Header.Get(headerIoTPrincipal)

	if err := h.Backend.AttachThingPrincipal(&AttachThingPrincipalInput{
		ThingName:          thingName,
		Principal:          principal,
		ThingPrincipalType: c.QueryParam("thingPrincipalType"),
	}); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// iotDefaultPageSize is the AWS default/maximum page size for IoT list
// operations that accept maxResults (ListThings, ListPolicies, ListTopicRules).
const iotDefaultPageSize = 250

// parseIoTPagination reads the maxResults and nextToken query parameters,
// returning the page size (clamped to [1, iotDefaultPageSize]) and the decoded
// start offset. An invalid or absent nextToken starts at offset 0.
func parseIoTPagination(c *echo.Context) (int, int) {
	pageSize := iotDefaultPageSize
	if v := c.QueryParam("maxResults"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < pageSize {
			pageSize = n
		}
	}

	start := 0
	if tok := c.QueryParam("nextToken"); tok != "" {
		if n, err := strconv.Atoi(tok); err == nil && n > 0 {
			start = n
		}
	}

	return pageSize, start
}

// parseIoTMarkerPagination reads the pageSize and marker query parameters --
// the wire names List ops like ListDomainConfigurations/
// ListOutgoingCertificates/ListPolicyPrincipals/ListPrincipalPolicies/
// ListTargetsForPolicy use instead of maxResults/nextToken (iot@v1.77.4
// serializers.go) -- returning the page size (clamped to
// [1, iotDefaultPageSize]) and the decoded start offset.
func parseIoTMarkerPagination(c *echo.Context) (int, int) {
	pageSize := iotDefaultPageSize
	if v := parseInt32QueryParam(c, "pageSize"); v > 0 && int(v) < pageSize {
		pageSize = int(v)
	}

	start := 0
	if marker := c.QueryParam("marker"); marker != "" {
		if n, err := strconv.Atoi(marker); err == nil && n > 0 {
			start = n
		}
	}

	return pageSize, start
}

// reverseSlice reverses items in-place.
func reverseSlice[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

// paginateMaps applies offset-based pagination to a list of result maps,
// returning the page and an opaque nextToken (the next start offset as a
// string). An empty token indicates the last page.
func paginateMaps[T any](items []T, pageSize, start int) ([]T, string) {
	if start >= len(items) {
		return items[len(items):], ""
	}

	items = items[start:]

	nextToken := ""
	if len(items) > pageSize {
		nextToken = strconv.Itoa(start + pageSize)
		items = items[:pageSize]
	}

	return items, nextToken
}

func (h *Handler) handleListThings(c *echo.Context) error {
	things := h.Backend.ListThings()

	out := make([]map[string]any, 0, len(things))
	for _, t := range things {
		out = append(out, map[string]any{
			keyThingName:     t.ThingName,
			keyThingArn:      t.ARN,
			keyThingTypeName: t.ThingTypeName,
			keyAttributes:    t.Attributes,
			keyVersion:       t.Version,
		})
	}

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(out, pageSize, start)

	resp := map[string]any{"things": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleUpdateThing(c *echo.Context) error {
	thingName := strings.TrimPrefix(c.Request().URL.Path, "/things/")

	var body UpdateThingInput

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	body.ThingName = thingName

	if err := h.Backend.UpdateThing(&body); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleListThingPrincipals(c *echo.Context) error {
	// Path: /things/{thingName}/principals
	after := strings.TrimPrefix(c.Request().URL.Path, "/things/")
	thingName := strings.TrimSuffix(after, "/principals")

	principals, err := h.Backend.ListThingPrincipals(thingName)
	if err != nil {
		return h.handleError(c, err)
	}

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(principals, pageSize, start)

	resp := map[string]any{"principals": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleDetachThingPrincipal(c *echo.Context) error {
	// DELETE /things/{thingName}/principals
	thingName := extractThingName(c.Request().URL.Path)
	principal := c.Request().Header.Get(headerIoTPrincipal)
	if err := h.Backend.DetachThingPrincipal(thingName, principal); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func extractThingName(path string) string {
	trimmed := strings.TrimPrefix(path, "/things/")

	return strings.SplitN(trimmed, "/", 2)[0] //nolint:mnd // split into at most 2 parts
}

func (h *Handler) handleListThingPrincipalsV2(c *echo.Context) error {
	after := strings.TrimPrefix(c.Request().URL.Path, "/things/")
	thingName := strings.TrimSuffix(after, "/principals-v2")
	thingPrincipalType := c.QueryParam("thingPrincipalType")

	principals, err := h.Backend.ListThingPrincipalsV2(thingName)
	if err != nil {
		return h.handleError(c, err)
	}

	out := make([]map[string]any, 0, len(principals))
	for _, p := range principals {
		if thingPrincipalType != "" && p.ThingPrincipalType != thingPrincipalType {
			continue
		}
		out = append(out, map[string]any{
			"principal":          p.Principal,
			"thingPrincipalType": p.ThingPrincipalType,
		})
	}

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(out, pageSize, start)

	resp := map[string]any{"thingPrincipalObjects": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleGetThingConnectivityData(c *echo.Context) error {
	thingName := strings.TrimSuffix(strings.TrimPrefix(c.Request().URL.Path, "/things/"), "/connectivity-data")

	data, err := h.Backend.GetThingConnectivityData(thingName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyThingName:       thingName,
		"connected":        data.Connected,
		"timestamp":        data.Timestamp,
		"disconnectReason": data.DisconnectReason,
	})
}
