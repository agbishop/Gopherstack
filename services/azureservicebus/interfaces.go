// Package azureservicebus provides a local, in-memory emulation of Azure
// Service Bus's Brokered Messaging REST API: queue and topic/subscription
// CRUD, and the full send/peek-lock/complete/abandon/dead-letter message
// lifecycle over HTTP+SharedAccessSignature auth. It deliberately does NOT
// implement AMQP 1.0 -- sessions and full AMQP compatibility are out of
// scope for this MVP. See AZURE.md section 9 (M5) and PARITY.md for scope
// and known gaps.
package azureservicebus

import (
	"context"
	"time"
)

// Compile-time assertion: InMemoryBackend must implement StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)

// EntityConfig holds the per-entity configuration properties real Service
// Bus accepts on CreateQueue/CreateTopic/CreateSubscription (LockDuration,
// MaxDeliveryCount, DefaultMessageTimeToLive), parsed from the Atom+XML
// create-request body (see atom.go). Which fields are meaningful depends on
// the entity kind: a queue honors all three; a topic honors only
// DefaultMessageTTL (applied as the message-TTL cap at Send time, since a
// topic never itself holds messages); a subscription honors LockDuration and
// MaxDeliveryCount. A zero-valued field falls back to the corresponding
// package-level default (DefaultLockDuration/MaxDeliveryCount/
// DefaultMessageTTL) -- see the lockDuration/maxDeliveryCount/
// defaultMessageTTL accessor methods.
type EntityConfig struct {
	LockDuration      time.Duration
	DefaultMessageTTL time.Duration
	MaxDeliveryCount  int
}

func (c EntityConfig) lockDuration() time.Duration {
	if c.LockDuration <= 0 {
		return DefaultLockDuration
	}

	return c.LockDuration
}

func (c EntityConfig) maxDeliveryCount() int64 {
	if c.MaxDeliveryCount <= 0 {
		return MaxDeliveryCount
	}

	return int64(c.MaxDeliveryCount)
}

func (c EntityConfig) defaultMessageTTL() time.Duration {
	if c.DefaultMessageTTL <= 0 {
		return DefaultMessageTTL
	}

	return c.DefaultMessageTTL
}

// firstConfig returns cfg[0] if present, else a zero-valued EntityConfig
// (every field falling back to its package-level default). Backs the
// variadic-config convenience overload on CreateQueue/CreateTopic/
// CreateSubscription, so every pre-existing call site that passes only a
// name keeps compiling and behaving exactly as before.
func firstConfig(cfg []EntityConfig) EntityConfig {
	if len(cfg) > 0 {
		return cfg[0]
	}

	return EntityConfig{}
}

// QueueInfo is the read-only metadata snapshot returned by GetQueueInfo/
// ListQueues.
type QueueInfo struct {
	CreatedAt         time.Time
	Name              string
	LockDuration      time.Duration
	DefaultMessageTTL time.Duration
	MaxDeliveryCount  int
}

// TopicInfo is the read-only metadata snapshot returned by GetTopicInfo/
// ListTopics.
type TopicInfo struct {
	CreatedAt         time.Time
	Name              string
	DefaultMessageTTL time.Duration
}

// SubscriptionInfo is the read-only metadata snapshot returned by
// GetSubscriptionInfo/ListSubscriptions.
type SubscriptionInfo struct {
	CreatedAt        time.Time
	Name             string
	LockDuration     time.Duration
	MaxDeliveryCount int
}

// EntityRef identifies a brokered-messaging entity that can hold messages:
// either a queue, or one specific subscription of a topic (a topic itself
// never holds messages directly -- sending to a topic fans the message out
// to every one of its subscriptions' own message lists). Exactly one of
// Queue or (Topic, Subscription) is set.
type EntityRef struct {
	Queue        string
	Topic        string
	Subscription string
}

// IsQueue reports whether ref addresses a queue rather than a subscription.
func (r EntityRef) IsQueue() bool { return r.Queue != "" }

// NewMessage is the input shape for sending a message, built from the
// incoming request's body and BrokerProperties header (see message_ops.go).
type NewMessage struct {
	CustomHeaders map[string]string
	ContentType   string
	Label         string
	CorrelationID string
	MessageID     string
	ReplyTo       string
	SessionID     string
	Body          []byte
	TimeToLive    time.Duration
}

// MessageInfo is the public, read-only snapshot of a stored message returned
// by Send/PeekLock. LockToken/LockedUntil are populated only for a
// peek-locked read; Send returns them zero.
type MessageInfo struct {
	EnqueuedTime   time.Time
	LockedUntil    time.Time
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

// StorageBackend defines the interface for an Azure Service Bus backend.
// Shaped after services/azurequeue's StorageBackend: a narrow, testable seam
// between the wire handler and storage, so handler tests can substitute a
// fake.
type StorageBackend interface {
	// CreateQueue creates a queue. cfg is variadic purely so every
	// pre-existing call site that only ever passed a name keeps compiling;
	// at most cfg[0] is used (see EntityConfig, firstConfig).
	CreateQueue(name string, cfg ...EntityConfig) (created bool, err error)
	DeleteQueue(name string) error
	QueueExists(name string) bool
	GetQueueInfo(name string) (QueueInfo, error)
	ListQueues() []QueueInfo

	// CreateTopic creates a topic. Only cfg's DefaultMessageTTL field is
	// meaningful (see EntityConfig's doc comment); see CreateQueue's doc
	// comment for why cfg is variadic.
	CreateTopic(name string, cfg ...EntityConfig) (created bool, err error)
	DeleteTopic(name string) error
	TopicExists(name string) bool
	GetTopicInfo(name string) (TopicInfo, error)
	ListTopics() []TopicInfo

	// CreateSubscription creates a subscription of topic. The filter rule
	// (if any) is accepted structurally but not stored/evaluated -- every
	// rule is treated as match-all (see PARITY.md's filter-evaluation gap).
	// Only cfg's LockDuration/MaxDeliveryCount fields are meaningful; see
	// CreateQueue's doc comment for why cfg is variadic.
	CreateSubscription(topic, name string, cfg ...EntityConfig) (created bool, err error)
	DeleteSubscription(topic, name string) error
	SubscriptionExists(topic, name string) bool
	GetSubscriptionInfo(topic, name string) (SubscriptionInfo, error)
	ListSubscriptions(topic string) ([]SubscriptionInfo, error)

	// Send enqueues msg on ref. For a queue ref it is appended to that
	// queue's own message list. For a topic ref (Topic set, Subscription
	// empty) it is fanned out: an independent copy is appended to every one
	// of that topic's subscriptions' own message lists, matching real
	// Service Bus's one-to-many topic delivery (see AZURE.md section 9's M5
	// entry and services/sns's topic/subscription fan-out as the structural
	// reference). Returns ErrTopicNotFound if the topic has no
	// subscriptions registered as an error -- fan-out to zero subscriptions
	// is not an error, matching real Service Bus (the message is simply
	// dropped, as no subscription exists to receive it). msg.TimeToLive, if
	// set, is capped at the target entity's configured
	// DefaultMessageTimeToLive (real Service Bus semantics); if unset, the
	// entity's DefaultMessageTimeToLive is used outright. Send also wakes
	// any goroutine blocked in PeekLockWait on the affected entity/entities.
	Send(ref EntityRef, msg NewMessage) (MessageInfo, error)

	// PeekLock performs a destructive read: it returns the oldest visible,
	// non-expired message on ref (or its dead-letter sub-queue, if
	// deadLetter is true), locking it for lockDuration and incrementing its
	// delivery count. lockDuration <= 0 resolves to ref's own configured
	// LockDuration (falling back to DefaultLockDuration if ref has none
	// configured) rather than a caller-supplied value -- see EntityConfig.
	// Returns ErrMessageNotFound if none is available.
	PeekLock(ref EntityRef, deadLetter bool, lockDuration time.Duration) (MessageInfo, error)

	// PeekLockWait is PeekLock's long-poll variant: if no message is
	// immediately visible, it waits up to timeout for one to arrive (a
	// message becoming visible via Send, Abandon's release path, or the
	// Janitor's lock-release path all wake a waiter), or until ctx is
	// cancelled, whichever comes first. timeout <= 0 behaves exactly like an
	// immediate PeekLock call. Callers are expected to have already clamped
	// timeout to a sane maximum (see handler.go's MaxPeekLockWaitTimeout);
	// PeekLockWait itself does not enforce a cap.
	PeekLockWait(
		ctx context.Context,
		ref EntityRef,
		deadLetter bool,
		lockDuration, timeout time.Duration,
	) (MessageInfo, error)

	// Complete permanently removes a locked message identified by messageID,
	// after verifying lockToken matches. Returns ErrLockTokenMismatch or
	// ErrMessageNotFound as appropriate.
	Complete(ref EntityRef, deadLetter bool, messageID, lockToken string) error

	// Abandon releases a locked message's lock (making it immediately
	// available again) after verifying lockToken matches, without altering
	// its position. If the message's delivery count has reached ref's
	// configured MaxDeliveryCount (see EntityConfig) it is moved to the
	// dead-letter sub-queue instead.
	Abandon(ref EntityRef, deadLetter bool, messageID, lockToken string) error

	// Reset clears all in-memory state. Used by the
	// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
	Reset()
}
