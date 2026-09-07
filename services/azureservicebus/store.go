package azureservicebus

import (
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
)

// Default and bound values, matching real Service Bus's documented defaults.
const (
	// DefaultLockDuration is applied to a PeekLock when the caller specifies
	// no timeout query parameter.
	DefaultLockDuration = 60 * time.Second
	// DefaultMessageTTL is applied to Send when the caller supplies no
	// TimeToLive in BrokerProperties.
	DefaultMessageTTL = 14 * 24 * time.Hour
	// MaxDeliveryCount is how many times a message may be
	// peek-locked-and-abandoned before it is automatically moved to its
	// entity's dead-letter sub-queue, matching real Service Bus's own
	// default MaxDeliveryCount.
	MaxDeliveryCount = 10
)

// storedMessage is the backend's internal representation of one message
// instance. A message sent to a topic is fanned out into one independent
// storedMessage per subscription (see message_ops.go's Send) so each
// subscription's delivery/lock/dead-letter state evolves independently, the
// same way real Service Bus subscriptions behave.
type storedMessage struct {
	EnqueuedTime   time.Time
	LockedUntil    time.Time // zero value = not locked
	ExpiresAt      time.Time
	CustomHeaders  map[string]string
	ContentType    string
	Label          string
	CorrelationID  string
	MessageID      string
	ReplyTo        string
	SessionID      string
	LockToken      string
	Body           []byte
	SequenceNumber int64
	DeliveryCount  int64
}

func (m *storedMessage) isExpired(now time.Time) bool { return !now.Before(m.ExpiresAt) }
func (m *storedMessage) isLocked(now time.Time) bool  { return m.LockedUntil.After(now) }

func (m *storedMessage) info() MessageInfo {
	return MessageInfo{
		MessageID:      m.MessageID,
		SequenceNumber: m.SequenceNumber,
		Body:           append([]byte(nil), m.Body...),
		ContentType:    m.ContentType,
		Label:          m.Label,
		CorrelationID:  m.CorrelationID,
		ReplyTo:        m.ReplyTo,
		SessionID:      m.SessionID,
		EnqueuedTime:   m.EnqueuedTime,
		DeliveryCount:  m.DeliveryCount,
		LockToken:      m.LockToken,
		LockedUntil:    m.LockedUntil,
		CustomHeaders:  m.CustomHeaders,
	}
}

// messageQueue holds the ordered live messages and dead-lettered messages
// for one brokered-messaging entity (a queue, or a single topic
// subscription). Both storedQueue and storedSubscription embed one.
type messageQueue struct {
	Messages   []*storedMessage
	DeadLetter []*storedMessage
}

type storedQueue struct {
	CreatedAt time.Time
	Name      string
	messageQueue
}

type storedTopic struct {
	Subscriptions map[string]*storedSubscription
	CreatedAt     time.Time
	Name          string
}

type storedSubscription struct {
	CreatedAt time.Time
	Name      string
	messageQueue
}

// InMemoryBackend implements StorageBackend using in-memory maps guarded by
// a single RWMutex. Shaped after services/azurequeue's InMemoryBackend.
type InMemoryBackend struct {
	mu     *lockmetrics.RWMutex
	queues map[string]*storedQueue
	topics map[string]*storedTopic
	// nowFunc is the backend's time source, overridable in tests so
	// lock-expiry/TTL/dead-letter logic can be exercised deterministically
	// instead of via real sleeps.
	nowFunc func() time.Time
	// idFunc generates message IDs and lock tokens, overridable in tests for
	// deterministic assertions.
	idFunc func() string
	// seq is the monotonically increasing SequenceNumber counter, shared
	// across every queue/subscription in the namespace (matching real
	// Service Bus, where SequenceNumber is unique per namespace).
	seq int64
}

// NewInMemoryBackend creates a new empty InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{
		mu:      lockmetrics.New("azureservicebus"),
		queues:  make(map[string]*storedQueue),
		topics:  make(map[string]*storedTopic),
		nowFunc: time.Now,
		idFunc:  uuid.NewString,
	}
}

func (b *InMemoryBackend) now() time.Time { return b.nowFunc().UTC() }

func (b *InMemoryBackend) nextSequenceNumberLocked() int64 {
	b.seq++

	return b.seq
}

// Reset clears all in-memory state. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.queues = make(map[string]*storedQueue)
	b.topics = make(map[string]*storedTopic)
	b.seq = 0
}

// resolveMessageQueueLocked resolves ref to its underlying *messageQueue.
// Callers must hold b.mu (either read or write). Returns ErrQueueNotFound,
// ErrTopicNotFound, ErrSubscriptionNotFound, or ErrInvalidEntityRef.
func (b *InMemoryBackend) resolveMessageQueueLocked(ref EntityRef) (*messageQueue, error) {
	if ref.IsQueue() {
		q, ok := b.queues[ref.Queue]
		if !ok {
			return nil, ErrQueueNotFound
		}

		return &q.messageQueue, nil
	}

	if ref.Topic == "" || ref.Subscription == "" {
		return nil, ErrInvalidEntityRef
	}

	t, ok := b.topics[ref.Topic]
	if !ok {
		return nil, ErrTopicNotFound
	}

	sub, ok := t.Subscriptions[ref.Subscription]
	if !ok {
		return nil, ErrSubscriptionNotFound
	}

	return &sub.messageQueue, nil
}
