package azureservicebus

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
)

// azureServiceBusVersion is the x-ms-version value echoed on every response,
// picked to be a plausible, well-formed value rather than because gopherstack
// implements that exact API version's full feature set. Mirrors
// services/azurequeue's azureQueueVersion.
const azureServiceBusVersion = "2021-05"

// deadLetterSegment is the reserved path segment Service Bus uses to address
// an entity's dead-letter sub-queue.
const deadLetterSegment = "$DeadLetterQueue"

// brokerPropertiesHeader is the header name carrying send/peek-lock metadata
// (see brokerProperties). Spelled in Go's canonical MIME-header form
// (net/http canonicalizes header names on both Set and Get, and HTTP header
// names are case-insensitive on the wire, so this is functionally identical
// to Service Bus's own "BrokerProperties" casing).
const brokerPropertiesHeader = "Brokerproperties"

// Operation name constants used for metrics (ExtractOperation) and
// GetSupportedOperations.
const (
	opCreateQueue        = "CreateQueue"
	opDeleteQueue        = "DeleteQueue"
	opGetQueue           = "GetQueue"
	opListQueues         = "ListQueues"
	opCreateTopic        = "CreateTopic"
	opDeleteTopic        = "DeleteTopic"
	opGetTopic           = "GetTopic"
	opListTopics         = "ListTopics"
	opCreateSubscription = "CreateSubscription"
	opDeleteSubscription = "DeleteSubscription"
	opGetSubscription    = "GetSubscription"
	opListSubscriptions  = "ListSubscriptions"
	opSendMessage        = "SendMessage"
	opPeekLockMessage    = "PeekLockMessage"
	opCompleteMessage    = "CompleteMessage"
	opAbandonMessage     = "AbandonMessage"
	unknownOperation     = "Unknown"
)

// MaxPeekLockWaitTimeout is the ceiling PeekLock's long-poll "?timeout="
// query parameter is clamped to. Real Service Bus documents 30 seconds as
// its own maximum long-poll/operation timeout, and gopherstack matches that
// value here as a deliberate cap on server-side resource use per in-flight
// long-poll request (one goroutine and one open connection held for up to
// this long). The wait is bounded by this cap plus request-context
// cancellation on client disconnect -- NOT by any http.Server timeout:
// azureServiceBusReadTimeout only bounds reading the request itself
// (headers/body), not handler execution time, and this server sets no
// WriteTimeout or handler deadline, so a long-poll handler blocking well
// past 60s would not be torn down by the server. See PARITY.md.
const MaxPeekLockWaitTimeout = 30 * time.Second

// Handler is the Echo HTTP handler for Azure Service Bus operations.
type Handler struct {
	Backend StorageBackend
	srv     *http.Server
	janitor *Janitor
	// Endpoint is e.g. "http://127.0.0.1:10003".
	Endpoint string
	// SASKeyValue is the base64 key WithSASValidation checks signatures
	// against. Defaults to DefaultKeyValue when empty.
	SASKeyValue string
	// Port is the TCP port StartWorker binds. Set from Settings at Init time
	// (see provider.go); defaults to DefaultPort. Single fixed,
	// protocol-conventional port -- no fallback pool.
	Port int
	// ValidateSAS opts checkAuth into cryptographic SAS verification. See
	// WithSASValidation.
	ValidateSAS bool
}

// NewHandler creates a new Azure Service Bus Handler. Port defaults to
// DefaultPort; callers (typically provider.go) override it from Settings.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend: backend,
		Port:    DefaultPort,
	}
}

// WithJanitor attaches a background lock-expiry/dead-letter janitor to the
// handler, mirroring services/azurequeue's WithJanitor. If Backend is not an
// *InMemoryBackend the call is a no-op.
func (h *Handler) WithJanitor(interval time.Duration) *Handler {
	concrete, ok := h.Backend.(*InMemoryBackend)
	if !ok {
		return h
	}

	h.janitor = NewJanitor(concrete, interval)

	return h
}

// WithSASValidation enables cryptographic verification of SAS
// (SharedAccessSignature) Authorization headers, checking each signature
// against the given key (base64-encoded). A blank key defaults to
// DefaultKeyValue. Mirrors services/s3's WithPresignValidation and
// Blob/Queue/Table's WithSharedKeyValidation: when never called, SAS headers
// are parsed structurally only, never cryptographically rejected.
func (h *Handler) WithSASValidation(key string) *Handler {
	if key == "" {
		key = DefaultKeyValue
	}

	h.SASKeyValue = key
	h.ValidateSAS = true

	return h
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
	_ service.Resettable       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "AzureServiceBus" }

// GetSupportedOperations returns the list of supported Azure Service Bus
// operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateQueue, opDeleteQueue, opGetQueue, opListQueues,
		opCreateTopic, opDeleteTopic, opGetTopic, opListTopics,
		opCreateSubscription, opDeleteSubscription, opGetSubscription, opListSubscriptions,
		opSendMessage, opPeekLockMessage, opCompleteMessage, opAbandonMessage,
	}
}

// RouteMatcher exists only to satisfy service.Registerable's interface
// contract: AzureServiceBus never matches on the shared AWS single-port
// Router (see Provider's doc comment).
func (h *Handler) RouteMatcher() service.Matcher {
	return func(*echo.Context) bool { return false }
}

// MatchPriority returns the routing priority for the AzureServiceBus
// handler. Irrelevant in practice since RouteMatcher never matches.
func (h *Handler) MatchPriority() int { return 0 }

// ExtractOperation extracts the Azure Service Bus operation name from the
// request, for metrics labeling.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	req, err := parseRequestPath(c.Request().URL.Path)
	if err != nil {
		return unknownOperation
	}

	return operationFor(c.Request().Method, req)
}

// ExtractResource extracts the entity resource identifier from the request
// path, for metrics labeling.
func (h *Handler) ExtractResource(c *echo.Context) string {
	req, err := parseRequestPath(c.Request().URL.Path)
	if err != nil {
		return ""
	}

	if req.Subscription != "" {
		return req.Entity + "/subscriptions/" + req.Subscription
	}

	return req.Entity
}

// Reset clears all in-memory state. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Handler returns the Echo handler function for Azure Service Bus
// operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()

		h.setCommonHeaders(c)

		if !h.checkAuth(r) {
			return h.writeError(c, http.StatusUnauthorized, "Unauthorized", "The SAS token signature is invalid.")
		}

		req, err := parseRequestPath(r.URL.Path)
		if err != nil {
			return h.writeError(c, http.StatusBadRequest, "BadRequest",
				"The requested URI does not represent any resource on the server.")
		}

		return h.dispatch(c, req)
	}
}

// dispatch routes a parsed request to the correct operation handler based on
// method and the parsed path's segment kind.
func (h *Handler) dispatch(c *echo.Context, req parsedRequest) error {
	switch req.Segment {
	case segEntity:
		return h.handleEntityLevel(c, req)
	case segSubscription:
		return h.handleSubscriptionLevel(c, req)
	case segMessages:
		return h.handleSendLevel(c, req)
	case segMessagesHead:
		return h.handlePeekLockLevel(c, req)
	case segMessageByID:
		return h.handleCompleteAbandonLevel(c, req)
	case segListSubscriptions:
		return h.handleListSubscriptionsLevel(c, req)
	case segResourcesQueues:
		return h.handleResourcesLevel(c, http.MethodGet, h.listQueues)
	case segResourcesTopics:
		return h.handleResourcesLevel(c, http.MethodGet, h.listTopics)
	default:
		return h.writeError(c, http.StatusBadRequest, "BadRequest",
			"The requested URI does not represent any resource on the server.")
	}
}

// handleResourcesLevel serves a $Resources/* listing endpoint (see
// segResourcesQueues/segResourcesTopics), rejecting anything but GET.
func (h *Handler) handleResourcesLevel(c *echo.Context, allowedMethod string, fn func(*echo.Context) error) error {
	if c.Request().Method != allowedMethod {
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}

	return fn(c)
}

// setCommonHeaders sets the headers real Azure SDKs/clients expect on every
// response, success or error.
func (h *Handler) setCommonHeaders(c *echo.Context) {
	hdr := c.Response().Header()
	hdr.Set("Server", "Microsoft-HTTPAPI/2.0")
	hdr.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	hdr.Set("X-Ms-Version", azureServiceBusVersion)
}

// newRequestID generates a plausible request-id (UUID-shaped, not
// cryptographically meaningful).
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}

	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// ---- path parsing ----

// segmentKind identifies which shape of Service Bus REST path a request
// targets.
type segmentKind int

const (
	segEntity segmentKind = iota
	segSubscription
	segMessages
	segMessagesHead
	segMessageByID
	// segListSubscriptions is GET /<topic>/subscriptions (list). Deliberately
	// distinct from segSubscription (which addresses one named subscription)
	// since it has no Subscription name segment at all.
	segListSubscriptions
	// segResourcesQueues/segResourcesTopics are GET /$Resources/Queues and
	// GET /$Resources/Topics respectively -- real Service Bus's
	// entity-listing management endpoints. Parsed and routed before any
	// generic /<entity> handling (see parseRequestPath/dispatch) so a
	// literal "$Resources" is never mistaken for an entity name.
	segResourcesQueues
	segResourcesTopics
)

// parsedRequest is the decomposed shape of a Service Bus REST path:
//
//	/<entity>                                              (segEntity)
//	/<entity>/subscriptions/<sub>                          (segSubscription)
//	/<entity>[/subscriptions/<sub>][/$DeadLetterQueue]/messages       (segMessages)
//	/<entity>[/subscriptions/<sub>][/$DeadLetterQueue]/messages/head  (segMessagesHead)
//	/<entity>[/subscriptions/<sub>][/$DeadLetterQueue]/messages/<id>/<token> (segMessageByID)
type parsedRequest struct {
	Entity       string
	Subscription string
	MessageID    string
	LockToken    string
	DeadLetter   bool
	Segment      segmentKind
}

// ErrBadPath is returned by parseRequestPath for a path that doesn't match
// any recognized Service Bus REST shape.
var ErrBadPath = errors.New("azureservicebus: unrecognized path")

// messagesSegment is the fixed path segment every message-scoped operation
// is nested beneath, mirroring services/azurequeue's identical constant.
const messagesSegment = "messages"

// headSegment is the fixed path segment identifying a peek-lock request
// (POST .../messages/head).
const headSegment = "head"

// subscriptionsSegmentLen and messageByIDSegmentLen are the rest-slice
// lengths parseRequestPath's switch matches on for
// ".../messages/head" and ".../messages/<id>/<token>" respectively.
const (
	messagesHeadSegmentLen = 2
	messageByIDSegmentLen  = 3
)

// resourcesSegment is the reserved path prefix for Service Bus's entity-list
// management endpoints ($Resources/Queues, $Resources/Topics). Checked
// before any generic /<entity> parsing -- see parseRequestPath.
const resourcesSegment = "$Resources"

// resourcesQueuesSegment/resourcesTopicsSegment are the two collection names
// recognized under resourcesSegment -- see parseResourcesPath.
const (
	resourcesQueuesSegment = "Queues"
	resourcesTopicsSegment = "Topics"
)

// subscriptionsSegment is the fixed path segment naming a topic's
// subscriptions collection, used both for /<topic>/subscriptions/<name> and
// (with no further segment) /<topic>/subscriptions (list).
const subscriptionsSegment = "subscriptions"

// parseRequestPath decomposes an Azure Service Bus REST request path. Unlike
// Azure Storage's paths (services/azurequeue's splitPath), Service Bus has no
// leading /<account> segment -- the namespace is addressed by host, not path.
func parseRequestPath(p string) (parsedRequest, error) {
	parts := splitNonEmpty(p)
	if len(parts) == 0 {
		return parsedRequest{}, ErrBadPath
	}

	// $Resources/Queues and $Resources/Topics are checked first and
	// explicitly, before "parts[0]" is ever treated as an entity name --
	// a queue or topic literally named "$Resources" is not a real-world
	// concern, but this ordering keeps the precedence unambiguous and
	// documented rather than accidental.
	if parts[0] == resourcesSegment {
		return parseResourcesPath(parts[1:])
	}

	req := parsedRequest{Entity: parts[0]}
	rest := parts[1:]

	if len(rest) == 1 && rest[0] == subscriptionsSegment {
		req.Segment = segListSubscriptions

		return req, nil
	}

	rest = consumeSubscriptionAndDeadLetterSegments(&req, rest)

	return parseMessageSegment(req, rest)
}

// parseResourcesPath classifies the path segment(s) following "$Resources".
func parseResourcesPath(rest []string) (parsedRequest, error) {
	if len(rest) != 1 {
		return parsedRequest{}, ErrBadPath
	}

	switch rest[0] {
	case resourcesQueuesSegment:
		return parsedRequest{Segment: segResourcesQueues}, nil
	case resourcesTopicsSegment:
		return parsedRequest{Segment: segResourcesTopics}, nil
	default:
		return parsedRequest{}, ErrBadPath
	}
}

// consumeSubscriptionAndDeadLetterSegments strips a leading
// "subscriptions/<name>" segment pair and/or "$DeadLetterQueue" segment from
// rest, recording them on req, and returns whatever remains.
func consumeSubscriptionAndDeadLetterSegments(req *parsedRequest, rest []string) []string {
	const subscriptionsSegmentLen = 2

	if len(rest) >= subscriptionsSegmentLen && rest[0] == subscriptionsSegment {
		req.Subscription = rest[1]
		rest = rest[subscriptionsSegmentLen:]
	}

	if len(rest) >= 1 && rest[0] == deadLetterSegment {
		req.DeadLetter = true
		rest = rest[1:]
	}

	return rest
}

// parseMessageSegment classifies whatever path remains after entity/
// subscription/dead-letter segments have been consumed, and finishes
// populating req accordingly.
func parseMessageSegment(req parsedRequest, rest []string) (parsedRequest, error) {
	switch {
	case len(rest) == 0:
		if req.Subscription != "" {
			req.Segment = segSubscription
		} else {
			req.Segment = segEntity
		}
	case len(rest) == 1 && rest[0] == messagesSegment:
		req.Segment = segMessages
	case len(rest) == messagesHeadSegmentLen && rest[0] == messagesSegment && rest[1] == headSegment:
		req.Segment = segMessagesHead
	case len(rest) == messageByIDSegmentLen && rest[0] == messagesSegment:
		req.MessageID = rest[1]
		req.LockToken = rest[2]
		req.Segment = segMessageByID
	default:
		return parsedRequest{}, ErrBadPath
	}

	return req, nil
}

// splitNonEmpty splits an URL path on "/" and drops empty segments (leading,
// trailing, or repeated slashes).
func splitNonEmpty(p string) []string {
	raw := strings.Split(p, "/")
	out := make([]string, 0, len(raw))

	for _, s := range raw {
		if s != "" {
			out = append(out, s)
		}
	}

	return out
}

// entityRefFor builds the EntityRef a parsedRequest addresses. Queue-shaped
// requests (no Subscription) resolve to a queue reference; the caller is
// responsible for entity-CRUD paths, which don't need an EntityRef at all.
func entityRefFor(req parsedRequest) EntityRef {
	if req.Subscription != "" {
		return EntityRef{Topic: req.Entity, Subscription: req.Subscription}
	}

	return EntityRef{Queue: req.Entity}
}

// operationFor determines the Azure Service Bus operation name for a
// request, for metrics labeling. Best-effort: does not distinguish queue vs.
// topic (that requires a backend lookup) for entity-level Create/Delete.
func operationFor(method string, req parsedRequest) string {
	switch req.Segment {
	case segEntity:
		return entityOperationFor(method, false)
	case segSubscription:
		return entityOperationFor(method, true)
	case segMessages:
		if method == http.MethodPost {
			return opSendMessage
		}

		return unknownOperation
	case segMessagesHead:
		if method == http.MethodPost {
			return opPeekLockMessage
		}

		return unknownOperation
	case segMessageByID:
		switch method {
		case http.MethodDelete:
			return opCompleteMessage
		case http.MethodPut:
			// operationFor sees only method+path, never headers -- it cannot
			// tell an Abandon PUT from a client-initiated DeadLetter PUT
			// (real Service Bus's BrokerProperties-based DeadLetterReason
			// wire shape, which this MVP does not implement pending further
			// research -- see PARITY.md's DeadLetter note) apart on this
			// information alone, so it always reports AbandonMessage.
			return opAbandonMessage
		default:
			return unknownOperation
		}
	case segListSubscriptions:
		return getOnlyOperationFor(method, opListSubscriptions)
	case segResourcesQueues:
		return getOnlyOperationFor(method, opListQueues)
	case segResourcesTopics:
		return getOnlyOperationFor(method, opListTopics)
	default:
		return unknownOperation
	}
}

// getOnlyOperationFor returns op if method is GET, else unknownOperation --
// every list-style segment (segListSubscriptions/segResourcesQueues/
// segResourcesTopics) only ever supports GET.
func getOnlyOperationFor(method, op string) string {
	if method == http.MethodGet {
		return op
	}

	return unknownOperation
}

// entityOperationFor determines the Create/Delete/Get operation name for an
// entity-level (segEntity) or subscription-level (segSubscription) request.
// Best-effort, like segEntity's Create/Delete case above: a GET on segEntity
// is always reported as GetQueue even when the entity is actually a topic,
// since telling them apart would require a backend lookup this metrics-only
// path deliberately avoids (matching the pre-existing Create/Delete
// ambiguity this function already had).
func entityOperationFor(method string, isSubscription bool) string {
	switch {
	case method == http.MethodPut && isSubscription:
		return opCreateSubscription
	case method == http.MethodDelete && isSubscription:
		return opDeleteSubscription
	case method == http.MethodGet && isSubscription:
		return opGetSubscription
	case method == http.MethodPut:
		return opCreateQueue
	case method == http.MethodDelete:
		return opDeleteQueue
	case method == http.MethodGet:
		return opGetQueue
	default:
		return unknownOperation
	}
}

// ---- entity-level (queue/topic) handlers ----

// looksLikeTopicBody reports whether body appears to describe a
// TopicDescription rather than a QueueDescription. This is the last-resort
// fallback in resolveEntityKind's resolution order: a simple
// case-insensitive substring sniff of the raw body, kept around because the
// REST integration test (and any other hand-built, deliberately simplified
// request body) may not be well-formed enough for parseAtomEntityBody's real
// Atom+XML parse to succeed. See PARITY.md's entity_kind_detection note.
func looksLikeTopicBody(body []byte) bool {
	return strings.Contains(strings.ToLower(string(body)), "topicdescription")
}

// typeQueryParam is the ?type= escape hatch's query-parameter name, and
// typeQueryValueTopic/typeQueryValueQueue its two recognized values -- see
// resolveEntityKind.
const (
	typeQueryParam      = "type"
	typeQueryValueTopic = "topic"
	typeQueryValueQueue = "queue"
)

// resolveEntityKind determines whether a PUT /<name> create-request
// describes a queue or a topic, and any EntityConfig properties that came
// with it, using -- in order -- (1) an explicit ?type=topic/?type=queue
// query param (either value wins outright, matching this function's own doc
// contract rather than only ever recognizing "topic"), (2) a successful
// Atom+XML parse of body, (3) looksLikeTopicBody's substring sniff, and (4)
// defaulting to a queue. Malformed XML never fails the create; it simply
// falls through to (3)/(4), matching this repo's permissive-by-default
// philosophy. See PARITY.md.
func resolveEntityKind(c *echo.Context, body []byte) (entityKind, EntityConfig) {
	switch c.QueryParam(typeQueryParam) {
	case typeQueryValueTopic:
		return entityKindTopic, EntityConfig{}
	case typeQueryValueQueue:
		return entityKindQueue, EntityConfig{}
	}

	if parsed, ok := parseAtomEntityBody(body); ok && parsed.Kind != entityKindSubscription {
		return parsed.Kind, parsed.Config
	}

	if looksLikeTopicBody(body) {
		return entityKindTopic, EntityConfig{}
	}

	return entityKindQueue, EntityConfig{}
}

func (h *Handler) handleEntityLevel(c *echo.Context, req parsedRequest) error {
	switch c.Request().Method {
	case http.MethodPut:
		return h.createEntity(c, req.Entity)
	case http.MethodDelete:
		return h.deleteEntity(c, req.Entity)
	case http.MethodGet:
		return h.getEntity(c, req.Entity)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

func (h *Handler) createEntity(c *echo.Context, name string) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return h.writeError(c, http.StatusBadRequest, "BadRequest", "Unable to read request body.")
	}

	kind, cfg := resolveEntityKind(c, body)

	// LockDuration/MaxDeliveryCount are queue-only properties (a
	// TopicDescription never carries them -- see entityDescriptionIn's
	// shared-struct doc comment), so validation only applies on the queue
	// path; a topic create with a spuriously-parsed value would have nothing
	// to validate against in the first place.
	if kind == entityKindQueue {
		if validateErr := validateEntityConfig(cfg); validateErr != nil {
			return h.writeError(c, http.StatusBadRequest, "BadRequest", validateErr.Error())
		}
	}

	var created bool

	if kind == entityKindTopic {
		created, err = h.Backend.CreateTopic(name, cfg)
	} else {
		created, err = h.Backend.CreateQueue(name, cfg)
	}

	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}

	return h.writeEntityCreated(c, created)
}

// getEntity serves GET /<name>: real Service Bus's management API GET for a
// single queue or topic. Queue is tried first (matching deleteEntity's
// queue-then-topic order); 404 if it exists as neither.
func (h *Handler) getEntity(c *echo.Context, name string) error {
	if info, err := h.Backend.GetQueueInfo(name); err == nil {
		return h.writeAtomEntry(c, http.StatusOK, queueEntryXML(info))
	}

	if info, err := h.Backend.GetTopicInfo(name); err == nil {
		return h.writeAtomEntry(c, http.StatusOK, topicEntryXML(info))
	}

	return h.writeError(c, http.StatusNotFound, "NotFound", "The messaging entity could not be found.")
}

// listQueues serves GET /$Resources/Queues.
func (h *Handler) listQueues(c *echo.Context) error {
	return h.writeAtomFeed(c, queueFeedXML(resourcesQueuesSegment, h.Backend.ListQueues()))
}

// listTopics serves GET /$Resources/Topics.
func (h *Handler) listTopics(c *echo.Context) error {
	return h.writeAtomFeed(c, topicFeedXML(resourcesTopicsSegment, h.Backend.ListTopics()))
}

// writeAtomEntry writes a single Get response in real Service Bus's
// Atom+XML entry shape.
func (h *Handler) writeAtomEntry(c *echo.Context, status int, entry atomEntryOut) error {
	body, err := xml.Marshal(entry)
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", "Failed to marshal entity metadata.")
	}

	return c.Blob(status, atomEntryContentType, append([]byte(xml.Header), body...))
}

// writeAtomFeed writes a List response in real Service Bus's Atom+XML feed
// shape.
func (h *Handler) writeAtomFeed(c *echo.Context, feed atomFeedOut) error {
	body, err := xml.Marshal(feed)
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", "Failed to marshal entity list.")
	}

	return c.Blob(http.StatusOK, atomFeedContentType, append([]byte(xml.Header), body...))
}

func (h *Handler) writeEntityCreated(c *echo.Context, created bool) error {
	if created {
		return c.String(http.StatusCreated, "")
	}
	// Pre-existing entity: treated as an idempotent success, matching
	// services/azurequeue's CreateQueue stance.
	return c.NoContent(http.StatusOK)
}

func (h *Handler) deleteEntity(c *echo.Context, name string) error {
	if err := h.Backend.DeleteQueue(name); err == nil {
		return c.NoContent(http.StatusOK)
	}

	if err := h.Backend.DeleteTopic(name); err == nil {
		return c.NoContent(http.StatusOK)
	}

	return h.writeError(c, http.StatusNotFound, "NotFound", "The messaging entity could not be found.")
}

// ---- subscription-level handlers ----

func (h *Handler) handleSubscriptionLevel(c *echo.Context, req parsedRequest) error {
	switch c.Request().Method {
	case http.MethodPut:
		return h.createSubscription(c, req.Entity, req.Subscription)
	case http.MethodDelete:
		return h.deleteSubscription(c, req.Entity, req.Subscription)
	case http.MethodGet:
		return h.getSubscription(c, req.Entity, req.Subscription)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

// createSubscription creates a subscription. The request body may contain a
// SQL filter rule (real Service Bus's RuleDescription/SqlFilter shape); this
// MVP reads it only far enough to extract EntityConfig's LockDuration/
// MaxDeliveryCount properties (parseAtomEntityBody) and otherwise discards
// it -- every subscription behaves as match-all. See CreateSubscription's
// doc comment and PARITY.md.
func (h *Handler) createSubscription(c *echo.Context, topic, name string) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return h.writeError(c, http.StatusBadRequest, "BadRequest", "Unable to read request body.")
	}

	var cfg EntityConfig
	if parsed, ok := parseAtomEntityBody(body); ok {
		cfg = parsed.Config
	}

	if validateErr := validateEntityConfig(cfg); validateErr != nil {
		return h.writeError(c, http.StatusBadRequest, "BadRequest", validateErr.Error())
	}

	created, err := h.Backend.CreateSubscription(topic, name, cfg)
	if err != nil {
		if errors.Is(err, ErrTopicNotFound) {
			return h.writeError(c, http.StatusNotFound, "NotFound", "The topic could not be found.")
		}

		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}

	return h.writeEntityCreated(c, created)
}

func (h *Handler) deleteSubscription(c *echo.Context, topic, name string) error {
	if err := h.Backend.DeleteSubscription(topic, name); err != nil {
		return h.writeError(c, http.StatusNotFound, "NotFound", "The messaging entity could not be found.")
	}

	return c.NoContent(http.StatusOK)
}

// getSubscription serves GET /<topic>/subscriptions/<name>.
func (h *Handler) getSubscription(c *echo.Context, topic, name string) error {
	info, err := h.Backend.GetSubscriptionInfo(topic, name)
	if err != nil {
		return h.writeError(c, http.StatusNotFound, "NotFound", "The messaging entity could not be found.")
	}

	return h.writeAtomEntry(c, http.StatusOK, subscriptionEntryXML(info))
}

// handleListSubscriptionsLevel serves GET /<topic>/subscriptions (list).
func (h *Handler) handleListSubscriptionsLevel(c *echo.Context, req parsedRequest) error {
	if c.Request().Method != http.MethodGet {
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}

	infos, err := h.Backend.ListSubscriptions(req.Entity)
	if err != nil {
		return h.writeError(c, http.StatusNotFound, "NotFound", "The topic could not be found.")
	}

	return h.writeAtomFeed(c, subscriptionFeedXML(req.Entity+"/subscriptions", infos))
}

// ---- message-level handlers ----

// brokerProperties is the JSON shape carried in the BrokerProperties header
// on send (subset populated by the client) and returned on peek-lock (full
// set populated by gopherstack), matching real Service Bus's REST wire
// format for this header.
type brokerProperties struct {
	MessageID       string  `json:"MessageId,omitempty"`
	CorrelationID   string  `json:"CorrelationId,omitempty"`
	SessionID       string  `json:"SessionId,omitempty"`
	Label           string  `json:"Label,omitempty"`
	ReplyTo         string  `json:"ReplyTo,omitempty"`
	LockToken       string  `json:"LockToken,omitempty"`
	LockedUntilUtc  string  `json:"LockedUntilUtc,omitempty"`
	EnqueuedTimeUtc string  `json:"EnqueuedTimeUtc,omitempty"`
	TimeToLive      float64 `json:"TimeToLive,omitempty"`
	SequenceNumber  int64   `json:"SequenceNumber,omitempty"`
	DeliveryCount   int64   `json:"DeliveryCount,omitempty"`
}

// brokerPropertiesFromHeader parses the BrokerProperties request header, if
// present. A missing or malformed header yields a zero-value (empty)
// brokerProperties -- send still succeeds with gopherstack-generated
// defaults, matching this repo's permissive-by-default philosophy.
func brokerPropertiesFromHeader(r *http.Request) brokerProperties {
	raw := r.Header.Get(brokerPropertiesHeader)
	if raw == "" {
		return brokerProperties{}
	}

	var bp brokerProperties

	_ = json.Unmarshal([]byte(raw), &bp)

	return bp
}

// handleSendLevel serves POST /<entity>/messages (send). The target entity
// must already exist as either a queue or a topic; sending directly to a
// subscription path is not a valid Service Bus operation (subscriptions are
// receive-only) and is rejected with 405.
func (h *Handler) handleSendLevel(c *echo.Context, req parsedRequest) error {
	if c.Request().Method != http.MethodPost {
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}

	if req.Subscription != "" {
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"Cannot send directly to a subscription.")
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return h.writeError(c, http.StatusBadRequest, "BadRequest", "Unable to read request body.")
	}

	bp := brokerPropertiesFromHeader(c.Request())
	newMsg := NewMessage{
		Body:          body,
		ContentType:   c.Request().Header.Get("Content-Type"),
		MessageID:     bp.MessageID,
		Label:         bp.Label,
		CorrelationID: bp.CorrelationID,
		ReplyTo:       bp.ReplyTo,
		SessionID:     bp.SessionID,
		TimeToLive:    time.Duration(bp.TimeToLive * float64(time.Second)),
	}

	var ref EntityRef

	switch {
	case h.Backend.QueueExists(req.Entity):
		ref = EntityRef{Queue: req.Entity}
	case h.Backend.TopicExists(req.Entity):
		ref = EntityRef{Topic: req.Entity}
	default:
		return h.writeError(c, http.StatusNotFound, "NotFound", "The messaging entity could not be found.")
	}

	if _, sendErr := h.Backend.Send(ref, newMsg); sendErr != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", sendErr.Error())
	}

	return c.NoContent(http.StatusCreated)
}

// handlePeekLockLevel serves POST /<entity>[/subscriptions/<sub>][/$DeadLetterQueue]/messages/head
// (peek-lock, a destructive read with a lock timeout). A "?timeout=<seconds>"
// query parameter makes this a long-poll: gopherstack waits up to that many
// seconds (clamped to MaxPeekLockWaitTimeout) for a message to arrive rather
// than immediately returning 204. The wait honors the request context, so a
// disconnecting client releases the handler goroutine right away. See
// PARITY.md.
func (h *Handler) handlePeekLockLevel(c *echo.Context, req parsedRequest) error {
	if c.Request().Method != http.MethodPost {
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}

	if err := h.checkEntityExists(req); err != nil {
		return h.writeEntityLookupError(c, err)
	}

	timeout := parsePeekLockTimeout(c.QueryParam("timeout"))

	info, err := h.Backend.PeekLockWait(c.Request().Context(), entityRefFor(req), req.DeadLetter, 0, timeout)
	if err != nil {
		if errors.Is(err, ErrMessageNotFound) {
			return c.NoContent(http.StatusNoContent)
		}

		return h.writeEntityLookupError(c, err)
	}

	return h.writeLockedMessage(c, req, info)
}

// parsePeekLockTimeout parses the "?timeout=" query parameter as a whole
// number of seconds. A missing, negative, zero, or unparseable value yields
// 0 (immediate return, matching this MVP's pre-long-poll behavior); a value
// above MaxPeekLockWaitTimeout is clamped down to it rather than rejected,
// matching this repo's permissive-by-default philosophy.
func parsePeekLockTimeout(raw string) time.Duration {
	if raw == "" {
		return 0
	}

	secs, err := strconv.Atoi(raw)
	if err != nil || secs <= 0 {
		return 0
	}

	d := time.Duration(secs) * time.Second
	if d > MaxPeekLockWaitTimeout {
		return MaxPeekLockWaitTimeout
	}

	return d
}

// writeLockedMessage writes a successful peek-lock response: 201 Created,
// BrokerProperties header describing the lock, a Location header giving the
// complete/abandon URI, and the message body.
func (h *Handler) writeLockedMessage(c *echo.Context, req parsedRequest, info MessageInfo) error {
	bp := brokerProperties{
		MessageID:       info.MessageID,
		CorrelationID:   info.CorrelationID,
		SessionID:       info.SessionID,
		Label:           info.Label,
		ReplyTo:         info.ReplyTo,
		LockToken:       info.LockToken,
		LockedUntilUtc:  info.LockedUntil.UTC().Format(http.TimeFormat),
		EnqueuedTimeUtc: info.EnqueuedTime.UTC().Format(http.TimeFormat),
		SequenceNumber:  info.SequenceNumber,
		DeliveryCount:   info.DeliveryCount,
	}

	bpJSON, err := json.Marshal(bp)
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", "Failed to marshal BrokerProperties.")
	}

	hdr := c.Response().Header()
	hdr.Set(brokerPropertiesHeader, string(bpJSON))
	hdr.Set("Location", completeMessagePath(req, info.MessageID, info.LockToken))
	hdr.Set("X-Ms-Request-Id", newRequestID())

	contentType := info.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return c.Blob(http.StatusCreated, contentType, info.Body)
}

// completeMessagePath builds the path a client uses to Complete/Abandon the
// message just peek-locked, mirroring real Service Bus's Location header.
func completeMessagePath(req parsedRequest, messageID, lockToken string) string {
	var b strings.Builder

	b.WriteString("/")
	b.WriteString(req.Entity)

	if req.Subscription != "" {
		b.WriteString("/subscriptions/")
		b.WriteString(req.Subscription)
	}

	if req.DeadLetter {
		b.WriteString("/")
		b.WriteString(deadLetterSegment)
	}

	b.WriteString("/messages/")
	b.WriteString(messageID)
	b.WriteString("/")
	b.WriteString(lockToken)

	return b.String()
}

// handleCompleteAbandonLevel serves the two lock-scoped operations:
// DELETE .../messages/<id>/<locktoken> (complete) and
// PUT .../messages/<id>/<locktoken> (abandon/unlock).
func (h *Handler) handleCompleteAbandonLevel(c *echo.Context, req parsedRequest) error {
	ref := entityRefFor(req)

	switch c.Request().Method {
	case http.MethodDelete:
		if err := h.Backend.Complete(ref, req.DeadLetter, req.MessageID, req.LockToken); err != nil {
			return h.writeLockOpError(c, err)
		}

		return c.NoContent(http.StatusOK)
	case http.MethodPut:
		if err := h.Backend.Abandon(ref, req.DeadLetter, req.MessageID, req.LockToken); err != nil {
			return h.writeLockOpError(c, err)
		}

		return c.NoContent(http.StatusOK)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

// checkEntityExists verifies the entity (queue, or topic+subscription) a
// parsedRequest addresses actually exists, without touching its messages.
func (h *Handler) checkEntityExists(req parsedRequest) error {
	if req.Subscription == "" {
		if !h.Backend.QueueExists(req.Entity) {
			return ErrQueueNotFound
		}

		return nil
	}

	if !h.Backend.SubscriptionExists(req.Entity, req.Subscription) {
		return ErrSubscriptionNotFound
	}

	return nil
}

// writeEntityLookupError maps a queue/topic/subscription lookup failure to
// the corresponding Service Bus error response.
func (h *Handler) writeEntityLookupError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrQueueNotFound), errors.Is(err, ErrTopicNotFound), errors.Is(err, ErrSubscriptionNotFound):
		return h.writeError(c, http.StatusNotFound, "NotFound", "The messaging entity could not be found.")
	default:
		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}
}

// writeLockOpError maps a Complete/Abandon failure to the corresponding
// Service Bus error response. Real Service Bus returns 410 Gone for both an
// unknown message and a lock-token mismatch/expiry ("MessageLockLostException"),
// since from the client's perspective both mean "this lock is no longer
// valid" -- gopherstack mirrors that single status code for both cases.
func (h *Handler) writeLockOpError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrMessageNotFound), errors.Is(err, ErrLockTokenMismatch), errors.Is(err, ErrMessageNotLocked):
		return h.writeError(c, http.StatusGone, "MessageLockLost", "The lock supplied is invalid or has expired.")
	case errors.Is(err, ErrQueueNotFound), errors.Is(err, ErrTopicNotFound), errors.Is(err, ErrSubscriptionNotFound):
		return h.writeError(c, http.StatusNotFound, "NotFound", "The messaging entity could not be found.")
	default:
		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}
}

// StartWorker binds the dedicated Service Bus listener, starts serving on
// it, and -- if WithJanitor was called -- starts the background
// lock-expiry/dead-letter sweep. Mirrors services/azurequeue's StartWorker
// exactly.
func (h *Handler) StartWorker(ctx context.Context) error {
	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", h.Port))
	if err != nil {
		return fmt.Errorf("azureservicebus: bind port %d: %w", h.Port, err)
	}

	e := echo.New()
	e.Use(logger.EchoMiddleware(logger.Load(ctx)))
	e.Any("/*", telemetry.WrapEchoHandler("AzureServiceBus", h.Handler(), h))

	srv := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: azureServiceBusReadHeaderTimeout,
		ReadTimeout:       azureServiceBusReadTimeout,
		IdleTimeout:       azureServiceBusIdleTimeout,
	}

	h.srv = srv

	workerCtx := logger.WithWorker(ctx, "azureservicebus", "listener")
	log := logger.Load(workerCtx)

	log.InfoContext(workerCtx, "azureservicebus: starting dedicated listener", "port", h.Port)

	go func() {
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.ErrorContext(workerCtx, "azureservicebus: listener stopped", "error", serveErr)
		}
	}()

	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Timeouts for the dedicated Service Bus http.Server. Mirrors
// services/azurequeue's identical constants and Slowloris rationale.
const (
	azureServiceBusReadHeaderTimeout = 10 * time.Second
	azureServiceBusReadTimeout       = 60 * time.Second
	azureServiceBusIdleTimeout       = 120 * time.Second
)

// Shutdown stops the dedicated Service Bus listener. Mirrors
// services/azurequeue's Shutdown.
func (h *Handler) Shutdown(ctx context.Context) {
	srv := h.srv
	h.srv = nil

	if srv == nil {
		return
	}

	log := logger.Load(ctx)

	if err := srv.Shutdown(ctx); err != nil {
		log.ErrorContext(ctx, "azureservicebus: graceful shutdown failed, forcing close", "error", err)

		if closeErr := srv.Close(); closeErr != nil {
			log.ErrorContext(ctx, "azureservicebus: forced close also failed", "error", closeErr)
		}
	}
}

// azureError is the standard error body shape (matches services/azureblob's
// azureError, just reused here as JSON instead of XML since Service Bus's
// REST error bodies are JSON, not Atom/XML).
type azureError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// writeError writes a standard Service Bus REST error body.
func (h *Handler) writeError(c *echo.Context, status int, code, message string) error {
	body, err := json.Marshal(azureError{Code: code, Message: message})
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal error")
	}

	return c.Blob(status, "application/json", body)
}
