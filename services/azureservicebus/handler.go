package azureservicebus

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	opCreateTopic        = "CreateTopic"
	opDeleteTopic        = "DeleteTopic"
	opCreateSubscription = "CreateSubscription"
	opDeleteSubscription = "DeleteSubscription"
	opSendMessage        = "SendMessage"
	opPeekLockMessage    = "PeekLockMessage"
	opCompleteMessage    = "CompleteMessage"
	opAbandonMessage     = "AbandonMessage"
	unknownOperation     = "Unknown"
)

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
		opCreateQueue, opDeleteQueue,
		opCreateTopic, opDeleteTopic,
		opCreateSubscription, opDeleteSubscription,
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
	default:
		return h.writeError(c, http.StatusBadRequest, "BadRequest",
			"The requested URI does not represent any resource on the server.")
	}
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

// parseRequestPath decomposes an Azure Service Bus REST request path. Unlike
// Azure Storage's paths (services/azurequeue's splitPath), Service Bus has no
// leading /<account> segment -- the namespace is addressed by host, not path.
func parseRequestPath(p string) (parsedRequest, error) {
	parts := splitNonEmpty(p)
	if len(parts) == 0 {
		return parsedRequest{}, ErrBadPath
	}

	req := parsedRequest{Entity: parts[0]}
	rest := consumeSubscriptionAndDeadLetterSegments(&req, parts[1:])

	return parseMessageSegment(req, rest)
}

// consumeSubscriptionAndDeadLetterSegments strips a leading
// "subscriptions/<name>" segment pair and/or "$DeadLetterQueue" segment from
// rest, recording them on req, and returns whatever remains.
func consumeSubscriptionAndDeadLetterSegments(req *parsedRequest, rest []string) []string {
	const subscriptionsSegmentLen = 2

	if len(rest) >= subscriptionsSegmentLen && rest[0] == "subscriptions" {
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
			return opAbandonMessage
		default:
			return unknownOperation
		}
	default:
		return unknownOperation
	}
}

func entityOperationFor(method string, isSubscription bool) string {
	switch {
	case method == http.MethodPut && isSubscription:
		return opCreateSubscription
	case method == http.MethodDelete && isSubscription:
		return opDeleteSubscription
	case method == http.MethodPut:
		return opCreateQueue
	case method == http.MethodDelete:
		return opDeleteQueue
	default:
		return unknownOperation
	}
}

// ---- entity-level (queue/topic) handlers ----

// looksLikeTopicBody reports whether body appears to describe a
// TopicDescription rather than a QueueDescription. Real Service Bus
// disambiguates by the Atom entry's XML element name
// (<TopicDescription>/<QueueDescription>); this MVP does a simple
// case-insensitive substring sniff of the raw body instead of a full
// atom+xml parse, which is sufficient to support azure-sdk-for-go's admin
// client and hand-built test requests alike. See PARITY.md.
func looksLikeTopicBody(body []byte) bool {
	return strings.Contains(strings.ToLower(string(body)), "topicdescription")
}

func (h *Handler) handleEntityLevel(c *echo.Context, req parsedRequest) error {
	switch c.Request().Method {
	case http.MethodPut:
		return h.createEntity(c, req.Entity)
	case http.MethodDelete:
		return h.deleteEntity(c, req.Entity)
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

	var created bool

	if looksLikeTopicBody(body) || c.QueryParam("type") == "topic" {
		created, err = h.Backend.CreateTopic(name)
	} else {
		created, err = h.Backend.CreateQueue(name)
	}

	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}

	return h.writeEntityCreated(c, created)
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
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

// createSubscription creates a subscription. The request body may contain a
// SQL filter rule (real Service Bus's RuleDescription/SqlFilter shape); this
// MVP reads and discards it -- every subscription behaves as match-all. See
// CreateSubscription's doc comment and PARITY.md.
func (h *Handler) createSubscription(c *echo.Context, topic, name string) error {
	if _, err := io.ReadAll(c.Request().Body); err != nil {
		return h.writeError(c, http.StatusBadRequest, "BadRequest", "Unable to read request body.")
	}

	created, err := h.Backend.CreateSubscription(topic, name)
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
// (peek-lock, a destructive read with a lock timeout). This MVP does not
// implement the long-poll "timeout" query parameter real clients use to wait
// for a message to arrive -- it returns immediately, 204 if none is
// available (see PARITY.md).
func (h *Handler) handlePeekLockLevel(c *echo.Context, req parsedRequest) error {
	if c.Request().Method != http.MethodPost {
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}

	if err := h.checkEntityExists(req); err != nil {
		return h.writeEntityLookupError(c, err)
	}

	info, err := h.Backend.PeekLock(entityRefFor(req), req.DeadLetter, DefaultLockDuration)
	if err != nil {
		if errors.Is(err, ErrMessageNotFound) {
			return c.NoContent(http.StatusNoContent)
		}

		return h.writeEntityLookupError(c, err)
	}

	return h.writeLockedMessage(c, req, info)
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
