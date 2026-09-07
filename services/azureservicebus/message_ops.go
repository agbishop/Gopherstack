package azureservicebus

import "time"

// Send enqueues msg on ref. See the StorageBackend.Send doc comment for the
// queue-vs-topic fan-out distinction.
func (b *InMemoryBackend) Send(ref EntityRef, msg NewMessage) (MessageInfo, error) {
	b.mu.Lock("Send")
	defer b.mu.Unlock()

	now := b.now()

	if ref.IsQueue() {
		q, ok := b.queues[ref.Queue]
		if !ok {
			return MessageInfo{}, ErrQueueNotFound
		}

		stored := b.newStoredMessageLocked(msg, now)
		q.Messages = append(q.Messages, stored)

		return stored.info(), nil
	}

	if ref.Subscription != "" {
		mq, err := b.resolveMessageQueueLocked(ref)
		if err != nil {
			return MessageInfo{}, err
		}

		stored := b.newStoredMessageLocked(msg, now)
		mq.Messages = append(mq.Messages, stored)

		return stored.info(), nil
	}

	// Topic-level send: fan out an independent copy to every subscription.
	t, ok := b.topics[ref.Topic]
	if !ok {
		return MessageInfo{}, ErrTopicNotFound
	}

	var first MessageInfo

	for i, sub := range sortedSubscriptions(t) {
		stored := b.newStoredMessageLocked(msg, now)
		sub.Messages = append(sub.Messages, stored)

		if i == 0 {
			first = stored.info()
		}
	}

	if len(t.Subscriptions) == 0 {
		// No subscriptions to deliver to: still report success (matches real
		// Service Bus -- the message is accepted and simply has nowhere to
		// go), with a synthesized info reflecting what would have been sent.
		stored := b.newStoredMessageLocked(msg, now)
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

// newStoredMessageLocked builds a storedMessage from msg. Callers must hold
// b.mu for writing.
func (b *InMemoryBackend) newStoredMessageLocked(msg NewMessage, now time.Time) *storedMessage {
	ttl := msg.TimeToLive
	if ttl <= 0 {
		ttl = DefaultMessageTTL
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
// ErrMessageNotFound if none is available.
func (b *InMemoryBackend) PeekLock(ref EntityRef, deadLetter bool, lockDuration time.Duration) (MessageInfo, error) {
	b.mu.Lock("PeekLock")
	defer b.mu.Unlock()

	mq, err := b.resolveMessageQueueLocked(ref)
	if err != nil {
		return MessageInfo{}, err
	}

	if lockDuration <= 0 {
		lockDuration = DefaultLockDuration
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

		return msg.info(), nil
	}

	return MessageInfo{}, ErrMessageNotFound
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
// MaxDeliveryCount, the message is moved to the entity's dead-letter
// sub-queue instead of being made available, matching real Service Bus's
// automatic dead-lettering on delivery-count exhaustion.
func (b *InMemoryBackend) Abandon(ref EntityRef, deadLetter bool, messageID, lockToken string) error {
	b.mu.Lock("Abandon")
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

	msg.LockToken = ""
	msg.LockedUntil = time.Time{}

	if !deadLetter && msg.DeliveryCount >= MaxDeliveryCount {
		mq.removeAt(false, idx)
		mq.DeadLetter = append(mq.DeadLetter, msg)
	}

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
