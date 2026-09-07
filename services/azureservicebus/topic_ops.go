package azureservicebus

// CreateTopic creates a new, empty topic (no subscriptions). If a topic with
// the same name already exists, created is false and err is nil, mirroring
// CreateQueue's idempotent-retry semantics.
func (b *InMemoryBackend) CreateTopic(name string) (bool, error) {
	b.mu.Lock("CreateTopic")
	defer b.mu.Unlock()

	if _, ok := b.topics[name]; ok {
		return false, nil
	}

	b.topics[name] = &storedTopic{
		Name:          name,
		CreatedAt:     b.now(),
		Subscriptions: make(map[string]*storedSubscription),
	}

	return true, nil
}

// DeleteTopic removes a topic, all of its subscriptions, and every message
// held by those subscriptions. Returns ErrTopicNotFound if the topic does
// not exist.
func (b *InMemoryBackend) DeleteTopic(name string) error {
	b.mu.Lock("DeleteTopic")
	defer b.mu.Unlock()

	if _, ok := b.topics[name]; !ok {
		return ErrTopicNotFound
	}

	delete(b.topics, name)

	return nil
}

// TopicExists reports whether a topic named name exists.
func (b *InMemoryBackend) TopicExists(name string) bool {
	b.mu.RLock("TopicExists")
	defer b.mu.RUnlock()

	_, ok := b.topics[name]

	return ok
}
