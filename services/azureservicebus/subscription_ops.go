package azureservicebus

// CreateSubscription creates a new, empty subscription of topic with the
// given configuration (see EntityConfig -- LockDuration and MaxDeliveryCount
// are meaningful for a subscription; cfg is variadic so pre-existing callers
// passing only topic/name keep compiling). If a subscription with the same
// name already exists on that topic, created is false and err is nil
// (idempotent, mirroring CreateQueue/CreateTopic). Returns ErrTopicNotFound
// if the topic does not exist.
//
// Real Service Bus subscription creation accepts an optional SQL filter rule
// in the request body; this MVP deliberately does not parse or store it --
// every subscription behaves as if it had Service Bus's default "match all"
// rule (TrueFilter). See PARITY.md's filter-evaluation gap and handler.go's
// createSubscription, which discards the request body's filter rule after
// extracting any EntityConfig properties it also carries.
func (b *InMemoryBackend) CreateSubscription(topic, name string, cfg ...EntityConfig) (bool, error) {
	b.mu.Lock("CreateSubscription")
	defer b.mu.Unlock()

	t, ok := b.topics[topic]
	if !ok {
		return false, ErrTopicNotFound
	}

	if _, exists := t.Subscriptions[name]; exists {
		return false, nil
	}

	t.Subscriptions[name] = &storedSubscription{
		Name:         name,
		CreatedAt:    b.now(),
		Config:       firstConfig(cfg),
		messageQueue: newMessageQueue(),
	}

	return true, nil
}

// DeleteSubscription removes a subscription and all of its messages
// (including any dead-lettered ones). Returns ErrTopicNotFound or
// ErrSubscriptionNotFound as appropriate.
func (b *InMemoryBackend) DeleteSubscription(topic, name string) error {
	b.mu.Lock("DeleteSubscription")
	defer b.mu.Unlock()

	t, ok := b.topics[topic]
	if !ok {
		return ErrTopicNotFound
	}

	if _, exists := t.Subscriptions[name]; !exists {
		return ErrSubscriptionNotFound
	}

	delete(t.Subscriptions, name)

	return nil
}

// SubscriptionExists reports whether topic has a subscription named name.
// Returns false (not an error) if topic itself does not exist.
func (b *InMemoryBackend) SubscriptionExists(topic, name string) bool {
	b.mu.RLock("SubscriptionExists")
	defer b.mu.RUnlock()

	t, ok := b.topics[topic]
	if !ok {
		return false
	}

	_, ok = t.Subscriptions[name]

	return ok
}

// GetSubscriptionInfo returns name's metadata within topic. Returns
// ErrTopicNotFound or ErrSubscriptionNotFound as appropriate.
func (b *InMemoryBackend) GetSubscriptionInfo(topic, name string) (SubscriptionInfo, error) {
	b.mu.RLock("GetSubscriptionInfo")
	defer b.mu.RUnlock()

	t, ok := b.topics[topic]
	if !ok {
		return SubscriptionInfo{}, ErrTopicNotFound
	}

	sub, ok := t.Subscriptions[name]
	if !ok {
		return SubscriptionInfo{}, ErrSubscriptionNotFound
	}

	return subscriptionInfoLocked(sub), nil
}

func subscriptionInfoLocked(sub *storedSubscription) SubscriptionInfo {
	return SubscriptionInfo{
		Name:             sub.Name,
		CreatedAt:        sub.CreatedAt,
		LockDuration:     sub.Config.lockDuration(),
		MaxDeliveryCount: int(sub.Config.maxDeliveryCount()),
	}
}
