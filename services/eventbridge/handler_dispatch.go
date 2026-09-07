package eventbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

var errUnknownOperation = errors.New("UnknownOperationException")

// Handler is the Echo HTTP service handler for EventBridge operations.
type Handler struct {
	Backend        StorageBackend
	ops            map[string]actionFn
	scheduler      *Scheduler
	archiveJanitor *ArchiveJanitor
	tags           map[string]*svcTags.Tags
	tagsMu         *lockmetrics.RWMutex
	cancelWorkers  func()
	DefaultRegion  string
}

// NewHandler creates a new EventBridge handler.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{
		Backend:       backend,
		DefaultRegion: config.DefaultRegion,
		tags:          make(map[string]*svcTags.Tags),
		tagsMu:        lockmetrics.New("eb.tags"),
		cancelWorkers: func() {},
	}
	h.ops = h.buildOps()

	return h
}

func (h *Handler) setTags(resourceID string, kv map[string]string) {
	h.tagsMu.Lock("setTags")
	defer h.tagsMu.Unlock()
	if h.tags[resourceID] == nil {
		h.tags[resourceID] = svcTags.New("eb." + resourceID + ".tags")
	}
	h.tags[resourceID].Merge(kv)
}

func (h *Handler) removeTags(resourceID string, keys []string) {
	h.tagsMu.RLock("removeTags")
	t := h.tags[resourceID]
	h.tagsMu.RUnlock()
	if t != nil {
		t.DeleteKeys(keys)
	}
}

func (h *Handler) getTags(resourceID string) map[string]string {
	h.tagsMu.RLock("getTags")
	t := h.tags[resourceID]
	h.tagsMu.RUnlock()
	if t == nil {
		return map[string]string{}
	}

	return t.Clone()
}

// clearResourceTags removes all tag state for a resource ARN from the handler's
// tag map. Called when a bus or rule is deleted so the map doesn't grow unbounded.
func (h *Handler) clearResourceTags(resourceARN string) {
	h.tagsMu.Lock("clearResourceTags")
	defer h.tagsMu.Unlock()
	delete(h.tags, resourceARN)
}

// SetScheduler attaches a Scheduler to the handler. The scheduler is started as a
// background worker when StartWorker is called (which satisfies service.BackgroundWorker).
func (h *Handler) SetScheduler(s *Scheduler) {
	h.scheduler = s
}

// SetArchiveJanitor attaches an archive janitor to the handler.
func (h *Handler) SetArchiveJanitor(j *ArchiveJanitor) {
	h.archiveJanitor = j
}

// StartWorker implements service.BackgroundWorker.
// It starts the EventBridge scheduled-rules scheduler and archive janitor as
// background goroutines. A derived context is stored so Shutdown can cancel
// both goroutines independently of the backend lifecycle.
func (h *Handler) StartWorker(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	h.cancelWorkers = cancel

	if h.scheduler != nil {
		go h.scheduler.Run(workerCtx)
	}

	if h.archiveJanitor != nil {
		go h.archiveJanitor.Run(workerCtx)
	}

	return nil
}

// Shutdown implements service.Shutdowner.
// It cancels the scheduler and archive janitor goroutines, then cancels the
// backend's internal lifecycle context and waits for all in-flight delivery
// goroutines to finish. If ctx expires before Close returns, Shutdown returns
// immediately so the process shutdown is not blocked.
func (h *Handler) Shutdown(ctx context.Context) {
	h.cancelWorkers()

	type closer interface{ Close() }

	b, ok := h.Backend.(closer)
	if !ok {
		return
	}

	done := make(chan struct{})

	go func() {
		b.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Ensure Handler implements service.BackgroundWorker and service.Shutdowner at compile time.
var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "EventBridge" }

// GetSupportedOperations returns all mocked EventBridge operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateEventBus",
		"DeleteEventBus",
		"ListEventBuses",
		"DescribeEventBus",
		"UpdateEventBus",
		"PutRule",
		"DeleteRule",
		"ListRules",
		"DescribeRule",
		"EnableRule",
		"DisableRule",
		"PutTargets",
		"RemoveTargets",
		"ListTargetsByRule",
		"ListRuleNamesByTarget",
		"PutEvents",
		"PutPartnerEvents",
		"ListTagsForResource",
		"TagResource",
		"UntagResource",
		"ActivateEventSource",
		"DeactivateEventSource",
		"DescribeEventSource",
		"ListEventSources",
		"CancelReplay",
		"DescribeReplay",
		"ListReplays",
		"StartReplay",
		"CreateApiDestination",
		"DeleteApiDestination",
		"DescribeApiDestination",
		"ListApiDestinations",
		"UpdateApiDestination",
		"CreateArchive",
		"DeleteArchive",
		"DescribeArchive",
		"ListArchives",
		"UpdateArchive",
		"CreateConnection",
		"DeleteConnection",
		"DescribeConnection",
		"ListConnections",
		"UpdateConnection",
		"DeauthorizeConnection",
		"CreateEndpoint",
		"DeleteEndpoint",
		"DescribeEndpoint",
		"ListEndpoints",
		"UpdateEndpoint",
		"CreatePartnerEventSource",
		"DeletePartnerEventSource",
		"DescribePartnerEventSource",
		"ListPartnerEventSources",
		"ListPartnerEventSourceAccounts",
		"TestEventPattern",
		"PutPermission",
		"RemovePermission",
		// GetEventBusPolicy/PutEventBusPolicy are deliberately NOT listed here:
		// they are not real EventBridge SDK operations (no such methods on
		// aws-sdk-go-v2/service/eventbridge.Client at any version -- the real
		// wire path for reading a bus policy is DescribeEventBus's Policy
		// field, populated via eventBusToResponse). They remain reachable
		// internal-only via policyActions() in the dispatch table below for
		// any existing direct callers, but no real AWS SDK client can invoke
		// them, so they must not be advertised as supported here.
		// Schema Registry operations.
		opCreateRegistry,
		opDeleteRegistry,
		opDescribeRegistry,
		opListRegistries,
		opUpdateRegistry,
		opCreateSchema,
		opDeleteSchema,
		opDescribeSchema,
		opListSchemas,
		opSearchSchemas,
		opUpdateSchema,
		opListSchemaVersions,
		// DescribeSchemaVersion is deliberately NOT listed here: it is not a
		// real Schemas SDK operation (no such method on
		// aws-sdk-go-v2/service/schemas.Client at any version -- the real
		// wire path for reading a specific version's content is
		// DescribeSchema's optional SchemaVersion request field). It remains
		// reachable internal-only via schemaVersionActions() in the dispatch
		// table below, but no real AWS SDK client can invoke it under this
		// name, so it must not be advertised as supported here.
		opDeleteSchemaVersion,
		opGetDiscoveredSchema,
		opPutCodeBinding,
		opDescribeCodeBinding,
		// ListCodeBindings is deliberately NOT listed here: it is not a real
		// Schemas SDK operation (no such method on
		// aws-sdk-go-v2/service/schemas.Client at any version -- checking a
		// binding's status is DescribeCodeBinding, per-language, one at a
		// time; there is no list-all-bindings operation in the real API). It
		// remains reachable internal-only via codeBindingActions() in the
		// dispatch table below, but no real AWS SDK client can invoke it
		// under this name, so it must not be advertised as supported here.
		opGetCodeBindingSource,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "events" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this EventBridge instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a matcher for EventBridge requests. It matches
// EventBridge's own JSON-RPC 1.1 X-Amz-Target prefixes (including the
// fabricated "AWSSchemas." internal convention no real client sends) plus
// the real schemas@v1.37.4 REST-JSON1 method+path templates (gopherstack-92ft)
// -- schemasRESTOpForRequest.
//
// Path alone does not textually discriminate: Batch's RouteMatcher also
// matches any "/v1/..." path (services/batch/handler.go's v1Prefix), and it
// does not exclude these Schemas templates the way it already excludes
// AppSync/CodeArtifact/Kafka's own "/v1/" paths. No SigV4 scoping is needed
// to resolve this, though, unlike iot/iotdataplane's genuine same-path
// collision (gopherstack-61i8): this handler's MatchPriority is already
// PriorityHeaderExact (100), strictly above Batch's PriorityPathVersioned
// (85), and pkgs/service's Router evaluates matchers in descending-priority
// order, calling the first one that returns true (pkgs/service/router.go).
// So this handler's matcher is always checked -- and wins -- before Batch's
// ever runs for these literal paths, deterministically, the same way
// codeartifact's higher-than-Batch priority already resolves an identical
// "/v1/" overlap (services/codeartifact/handler.go's
// codeartifactMatchPriority comment).
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		if strings.HasPrefix(target, "AmazonEventBridge.") ||
			strings.HasPrefix(target, "AWSEvents.") ||
			strings.HasPrefix(target, "AWSSchemas.") {
			return true
		}

		return schemasRESTOpForRequest(c.Request()) != ""
	}
}

// MatchPriority returns the routing priority for the EventBridge handler.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the operation name from the X-Amz-Target header
// for EventBridge's own JSON-RPC 1.1 requests, or (for a real Schemas
// REST-JSON1 request, which carries no X-Amz-Target at all) from the
// method+path via schemasRESTOpForRequest.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		const targetParts = 2
		if len(parts) == targetParts {
			return parts[1]
		}

		return "Unknown"
	}

	if op := schemasRESTOpForRequest(c.Request()); op != "" {
		return op
	}

	return "Unknown"
}

// ExtractResource extracts the resource name from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	for _, key := range []string{"Name", "Rule", "EventBusName", "ReplayName", "ArchiveName"} {
		if v, ok := data[key].(string); ok && v != "" {
			return v
		}
	}

	return ""
}

// Handler returns the Echo handler function for EventBridge requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)
		ctx := context.WithValue(c.Request().Context(), regionContextKey{}, region)
		c.SetRequest(c.Request().WithContext(ctx))

		if op := schemasRESTOpForRequest(c.Request()); op != "" {
			return h.handleSchemasREST(c, op)
		}

		return service.HandleTarget(
			c, logger.Load(ctx),
			"EventBridge", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

type actionFn func(context.Context, []byte) (any, error)

// timeToEpochSeconds converts a time.Time to a float64 Unix epoch seconds value,
// as required by the AWS JSON protocol for timestamp fields.
func timeToEpochSeconds(t time.Time) float64 {
	return float64(t.Unix())
}

func (h *Handler) newOpsActions() map[string]actionFn {
	table := make(map[string]actionFn)
	maps.Copy(table, h.eventSourceActions())
	maps.Copy(table, h.extendedEventSourceActions())
	maps.Copy(table, h.partnerSourceActions())
	maps.Copy(table, h.extendedPartnerSourceActions())
	maps.Copy(table, h.replayActions())
	maps.Copy(table, h.extendedReplayActions())
	maps.Copy(table, h.connectionActions())
	maps.Copy(table, h.extendedConnectionActions())
	maps.Copy(table, h.apiDestinationActions())
	maps.Copy(table, h.extendedAPIDestinationActions())
	maps.Copy(table, h.archiveActions())
	maps.Copy(table, h.extendedArchiveActions())
	maps.Copy(table, h.endpointActions())
	maps.Copy(table, h.extendedEndpointActions())
	maps.Copy(table, h.eventBusManagementActions())
	maps.Copy(table, h.policyActions())
	maps.Copy(table, h.registryActions())
	maps.Copy(table, h.schemaActions())
	maps.Copy(table, h.schemaVersionActions())
	maps.Copy(table, h.codeBindingActions())

	return table
}

func (h *Handler) buildOps() map[string]actionFn {
	table := make(map[string]actionFn)
	maps.Copy(table, h.eventBusActions())
	maps.Copy(table, h.ruleActions())
	maps.Copy(table, h.ruleStateActions())
	maps.Copy(table, h.ruleQueryActions())
	maps.Copy(table, h.targetActions())
	maps.Copy(table, h.eventsActions())
	maps.Copy(table, h.tagActions())
	maps.Copy(table, h.newOpsActions())

	return table
}

// dispatch routes the action to the correct handler function.
func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w:%s", errUnknownOperation, action)
	}

	response, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(response)
}

// handleError writes a standardized JSON error response.
func (h *Handler) handleError(
	ctx context.Context,
	c *echo.Context,
	action string,
	reqErr error,
) error {
	log := logger.Load(ctx)
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.1")

	var errType string
	var statusCode int

	switch {
	case errors.Is(reqErr, ErrEventBusNotFound),
		errors.Is(reqErr, ErrRuleNotFound),
		errors.Is(reqErr, ErrNotFound):
		errType = "ResourceNotFoundException"
		statusCode = http.StatusNotFound
	case errors.Is(reqErr, ErrEventBusAlreadyExists), errors.Is(reqErr, ErrAlreadyExists):
		errType = "ResourceAlreadyExistsException"
		statusCode = http.StatusConflict
	case errors.Is(reqErr, ErrCannotDeleteDefaultBus), errors.Is(reqErr, ErrReplayNotCancellable):
		errType = "IllegalStatusException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrInvalidParameter):
		errType = "InvalidParameterException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrInvalidState):
		errType = "InvalidStateException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrResourceLimitExceeded):
		// AWS: PutRule and CreateEventBus (this sentinel's only two raisers)
		// both model LimitExceededException -- "ResourceLimitExceededException"
		// names no type anywhere in eventbridge@v1.48.4.
		errType = "LimitExceededException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrForbiddenOperation):
		errType = "ForbiddenException"
		statusCode = http.StatusForbidden
	case errors.Is(reqErr, ErrManagedRule):
		errType = "ManagedRuleException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, errUnknownOperation):
		errType = "UnknownOperationException"
		statusCode = http.StatusBadRequest
	default:
		errType = "InternalException"
		statusCode = http.StatusInternalServerError
	}

	if statusCode == http.StatusInternalServerError {
		log.ErrorContext(ctx, "EventBridge internal error", "error", reqErr, "action", action)
	} else {
		log.WarnContext(ctx, "EventBridge request error", "error", reqErr, "action", action)
	}

	errResp := service.JSONErrorResponse{
		Type:    errType,
		Message: reqErr.Error(),
	}

	payload, _ := json.Marshal(errResp)

	return c.JSONBlob(statusCode, payload)
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}

	// Clear handler-level tag state so that tags don't bleed across test runs.
	h.tagsMu.Lock("Reset")
	defer h.tagsMu.Unlock()

	for _, t := range h.tags {
		t.Close()
	}

	h.tags = make(map[string]*svcTags.Tags)
}
