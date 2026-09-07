package azureservicebus

import (
	"context"
	"errors"
	"time"
)

// Send enqueues msg on ref. See the StorageBackend.Send doc comment for the
// queue-vs-topic fan-out distinction and the per-entity TTL cap.
func (b *InMemoryBackend) Send(ref EntityRef, msg NewMessage) (MessageInfo, error) {
	b.mu.Lock("Send")
	defer b.mu.Unlock()

	now := b.now()

	if ref.IsQueue() {
		q, ok := b.queues[ref.Queue]
		if !ok {
			return MessageInfo{}, ErrQueueNotFound
		}

		stored := b.newStoredMessageLocked(msg, q.Config, now)
		q.Messages = append(q.Messages, stored)
		q.broadcastLocked()

		return stored.info(), nil
	}

	if ref.Subscription != "" {
		mq, cfg, err := b.resolveEntityLocked(ref)
		if err != nil {
			return MessageInfo{}, err
		}

		stored := b.newStoredMessageLocked(msg, cfg, now)
		mq.Messages = append(mq.Messages, stored)
		mq.broadcastLocked()

		return stored.info(), nil
	}

	// Topic-level send: fan out an independent copy to every subscription.
	t, ok := b.topics[ref.Topic]
	if !ok {
		return MessageInfo{}, ErrTopicNotFound
	}

	var first MessageInfo

	for i, sub := range sortedSubscriptions(t) {
		stored := b.newStoredMessageLocked(msg, t.Config, now)
		sub.Messages = append(sub.Messages, stored)
		sub.broadcastLocked()

		if i == 0 {
			first = stored.info()
		}
	}

	if len(t.Subscriptions) == 0 {
		// No subscriptions to deliver to: still report success (matches real
		// Service Bus -- the message is accepted and simply has nowhere to
		// go), with a synthesized info reflecting what would have been sent.
		stored := b.newStoredMessageLocked(msg, t.Config, now)
		first = stored.info()
	}

	return first, nil
}

// sortedSubscriptions returns t's subscriptions in a stable (name-sorted)
// order purely so tests that send to multi-subscription topics get
// deterministic sequence-number assignment; delivery to each subscription is
// otherwise independent.
func sortedSubscriptions(t *storedTopic) []*storedSubscription {
	names := make([]string, 0, len(t.Subscriptions))
	for name := range t.Subscriptions {
		names = append(names, name)
	}

	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}

	out := make([]*storedSubscription, 0, len(names))
	for _, name := range names {
		out = append(out, t.Subscriptions[name])
	}

	return out
}

// newStoredMessageLocked builds a storedMessage from msg. If msg.TimeToLive
// is unset (<= 0) the target entity's configured DefaultMessageTimeToLive
// (cfg.defaultMessageTTL(), falling back to the package-level
// DefaultMessageTTL) is used outright; if it is set, it is capped at that
// same value, matching real Service Bus's per-message-TTL-cap semantics (see
// PARITY.md). Callers must hold b.mu for writing.
func (b *InMemoryBackend) newStoredMessageLocked(msg NewMessage, cfg EntityConfig, now time.Time) *storedMessage {
	entityTTL := cfg.defaultMessageTTL()

	ttl := msg.TimeToLive
	if ttl <= 0 || ttl > entityTTL {
		ttl = entityTTL
	}

	id := msg.MessageID
	if id == "" {
		id = b.idFunc()
	}

	return &storedMessage{
		MessageID:      id,
		SequenceNumber: b.nextSequenceNumberLocked(),
		Body:           append([]byte(nil), msg.Body...),
		ContentType:    msg.ContentType,
		Label:          msg.Label,
		CorrelationID:  msg.CorrelationID,
		ReplyTo:        msg.ReplyTo,
		SessionID:      msg.SessionID,
		CustomHeaders:  msg.CustomHeaders,
		EnqueuedTime:   now,
		ExpiresAt:      now.Add(ttl),
	}
}

// PeekLock returns the oldest visible (unlocked, non-expired) message on ref
// -- or its dead-letter sub-queue, if deadLetter is true -- locking it for
// lockDuration and incrementing its delivery count. Returns
// ErrMessageNotFound if none is available. See the StorageBackend doc
// comment for lockDuration <= 0's fallback behavior.
func (b *InMemoryBackend) PeekLock(ref EntityRef, deadLetter bool, lockDuration time.Duration) (MessageInfo, error) {
	info, _, err := b.peekLockOnce(ref, deadLetter, lockDuration)

	return info, err
}

// PeekLockWait is PeekLock's long-poll variant. See the StorageBackend doc
// comment. Mirrors services/sqs's ReceiveMessage/pollReceive/receiveOnce
// long-poll shape (a broadcast notify channel plus a 1-second recheck-timer
// backstop), adapted to take a context so a disconnecting client releases
// the waiting goroutine immediately -- a deliberate improvement over the SQS
// precedent, which has no ctx parameter (see PARITY.md). Deadline/backstop
// timing uses the real wall clock (time.Now/time.Timer), not the backend's
// mockable nowFunc -- matching services/sqs's identical choice and making
// this method exercisable under testing/synctest's fake clock.
func (b *InMemoryBackend) PeekLockWait(
	ctx context.Context, ref EntityRef, deadLetter bool, lockDuration, timeout time.Duration,
) (MessageInfo, error) {
	info, notifyCh, err := b.peekLockOnce(ref, deadLetter, lockDuration)
	if !errors.Is(err, ErrMessageNotFound) || timeout <= 0 {
		return info, err
	}

	deadline := time.Now().Add(timeout)

	const recheckInterval = time.Second

	timer := time.NewTimer(recheckInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return MessageInfo{}, ErrMessageNotFound
		case <-notifyCh:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}

		info, notifyCh, err = b.peekLockOnce(ref, deadLetter, lockDuration)
		if !errors.Is(err, ErrMessageNotFound) {
			return info, err
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return MessageInfo{}, ErrMessageNotFound
		}

		timer.Reset(min(remaining, recheckInterval))
	}
}

// peekLockOnce attempts one immediate PeekLock and, on ErrMessageNotFound,
// also returns the entity's current notify channel captured under the same
// lock as the read attempt -- avoiding a lost wakeup between checking for a
// message and starting to wait on the channel (mirrors services/sqs's
// receiveOnce).
func (b *InMemoryBackend) peekLockOnce(
	ref EntityRef, deadLetter bool, lockDuration time.Duration,
) (MessageInfo, chan struct{}, error) {
	b.mu.Lock("PeekLock")
	defer b.mu.Unlock()

	mq, cfg, err := b.resolveEntityLocked(ref)
	if err != nil {
		return MessageInfo{}, nil, err
	}

	if lockDuration <= 0 {
		lockDuration = cfg.lockDuration()
	}

	now := b.now()
	list := mq.liveList(deadLetter)

	for _, msg := range list {
		if msg.isExpired(now) || msg.isLocked(now) {
			continue
		}

		msg.DeliveryCount++
		msg.LockToken = b.idFunc()
		msg.LockedUntil = now.Add(lockDuration)

		return msg.info(), nil, nil
	}

	mq.ensureNotifyLocked()

	return MessageInfo{}, mq.notify, ErrMessageNotFound
}

// Complete permanently removes a locked message identified by messageID from
// ref (or its dead-letter sub-queue), after verifying lockToken matches.
func (b *InMemoryBackend) Complete(ref EntityRef, deadLetter bool, messageID, lockToken string) error {
	b.mu.Lock("Complete")
	defer b.mu.Unlock()

	mq, err := b.resolveMessageQueueLocked(ref)
	if err != nil {
		return err
	}

	now := b.now()

	idx, msg, err := findLockedMessageLocked(mq.liveList(deadLetter), messageID, now)
	if err != nil {
		return err
	}

	if msg.LockToken != lockToken {
		return ErrLockTokenMismatch
	}

	mq.removeAt(deadLetter, idx)

	return nil
}

// Abandon releases a locked message's lock (making it immediately available
// again), after verifying lockToken matches. If DeliveryCount has reached
// ref's configured MaxDeliveryCount, the message is moved to the entity's
// dead-letter sub-queue instead of being made available, matching real
// Service Bus's automatic dead-lettering on delivery-count exhaustion. A
// release back to availability wakes any PeekLockWait waiter on ref.
func (b *InMemoryBackend) Abandon(ref EntityRef, deadLetter bool, messageID, lockToken string) error {
	b.mu.Lock("Abandon")
	defer b.mu.Unlock()

	mq, cfg, err := b.resolveEntityLocked(ref)
	if err != nil {
		return err
	}

	now := b.now()

	idx, msg, err := findLockedMessageLocked(mq.liveList(deadLetter), messageID, now)
	if err != nil {
		return err
	}

	if msg.LockToken != lockToken {
		return ErrLockTokenMismatch
	}

	msg.LockToken = ""
	msg.LockedUntil = time.Time{}

	if !deadLetter && msg.DeliveryCount >= cfg.maxDeliveryCount() {
		mq.removeAt(false, idx)
		mq.DeadLetter = append(mq.DeadLetter, msg)

		return nil
	}

	mq.broadcastLocked()

	return nil
}

// liveList returns the live-message or dead-letter slice, selected by
// deadLetter.
func (mq *messageQueue) liveList(deadLetter bool) []*storedMessage {
	if deadLetter {
		return mq.DeadLetter
	}

	return mq.Messages
}

// removeAt removes the message at index idx from the selected slice.
func (mq *messageQueue) removeAt(deadLetter bool, idx int) {
	if deadLetter {
		mq.DeadLetter = append(mq.DeadLetter[:idx], mq.DeadLetter[idx+1:]...)

		return
	}

	mq.Messages = append(mq.Messages[:idx], mq.Messages[idx+1:]...)
}

// findLockedMessageLocked resolves a message by ID within list, as of now, requiring
// it to currently be locked (a message can only be Completed/Abandoned while
// locked, matching real Service Bus's lock-token requirement). An expired
// message is treated as not found even if the Janitor has not yet swept it.
func findLockedMessageLocked(list []*storedMessage, messageID string, now time.Time) (int, *storedMessage, error) {
	for i, msg := range list {
		if msg.MessageID != messageID || msg.isExpired(now) {
			continue
		}

		if !msg.isLocked(now) {
			return 0, nil, ErrMessageNotLocked
		}

		return i, msg, nil
	}

	return 0, nil, ErrMessageNotFound
}
