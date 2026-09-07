package azureservicebus

// CreateSubscription creates a new, empty subscription of topic. If a
// subscription with the same name already exists on that topic, created is
// false and err is nil (idempotent, mirroring CreateQueue/CreateTopic).
// Returns ErrTopicNotFound if the topic does not exist.
//
// Real Service Bus subscription creation accepts an optional SQL filter rule
// in the request body; this MVP deliberately does not parse or store it --
// every subscription behaves as if it had Service Bus's default "match all"
// rule (TrueFilter). See PARITY.md's filter-evaluation gap and handler.go's
// createSubscription, which discards the request body after reading it.
func (b *InMemoryBackend) CreateSubscription(topic, name string) (bool, error) {
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
		Name:      name,
		CreatedAt: b.now(),
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
