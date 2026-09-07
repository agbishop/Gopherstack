// Package azureservicebus provides a local, in-memory emulation of Azure
// Service Bus's Brokered Messaging REST API: queue and topic/subscription
// CRUD, and the full send/peek-lock/complete/abandon/dead-letter message
// lifecycle over HTTP+SharedAccessSignature auth. It deliberately does NOT
// implement AMQP 1.0 -- sessions and full AMQP compatibility are out of
// scope for this MVP. See AZURE.md section 9 (M5) and PARITY.md for scope
// and known gaps.
package azureservicebus

import "time"

// Compile-time assertion: InMemoryBackend must implement StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)

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
	CreateQueue(name string) (created bool, err error)
	DeleteQueue(name string) error
	QueueExists(name string) bool

	CreateTopic(name string) (created bool, err error)
	DeleteTopic(name string) error
	TopicExists(name string) bool

	// CreateSubscription creates a subscription of topic. The filter rule (if
	// any) is accepted structurally but not stored/evaluated -- every rule is
	// treated as match-all (see PARITY.md's filter-evaluation gap).
	CreateSubscription(topic, name string) (created bool, err error)
	DeleteSubscription(topic, name string) error
	SubscriptionExists(topic, name string) bool

	// Send enqueues msg on ref. For a queue ref it is appended to that
	// queue's own message list. For a topic ref (Topic set, Subscription
	// empty) it is fanned out: an independent copy is appended to every one
	// of that topic's subscriptions' own message lists, matching real
	// Service Bus's one-to-many topic delivery (see AZURE.md section 9's M5
	// entry and services/sns's topic/subscription fan-out as the structural
	// reference). Returns ErrTopicNotFound if the topic has no
	// subscriptions registered as an error -- fan-out to zero subscriptions
	// is not an error, matching real Service Bus (the message is simply
	// dropped, as no subscription exists to receive it).
	Send(ref EntityRef, msg NewMessage) (MessageInfo, error)

	// PeekLock performs a destructive read: it returns the oldest visible,
	// non-expired message on ref (or its dead-letter sub-queue, if
	// deadLetter is true), locking it for lockDuration and incrementing its
	// delivery count. Returns ErrMessageNotFound if none is available.
	PeekLock(ref EntityRef, deadLetter bool, lockDuration time.Duration) (MessageInfo, error)

	// Complete permanently removes a locked message identified by messageID,
	// after verifying lockToken matches. Returns ErrLockTokenMismatch or
	// ErrMessageNotFound as appropriate.
	Complete(ref EntityRef, deadLetter bool, messageID, lockToken string) error

	// Abandon releases a locked message's lock (making it immediately
	// available again) after verifying lockToken matches, without altering
	// its position. If the message's delivery count has reached
	// MaxDeliveryCount it is moved to the dead-letter sub-queue instead.
	Abandon(ref EntityRef, deadLetter bool, messageID, lockToken string) error

	// Reset clears all in-memory state. Used by the
	// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
	Reset()
}
