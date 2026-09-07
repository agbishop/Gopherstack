package azureservicebus

import "sort"

// CreateTopic creates a new, empty topic (no subscriptions) with the given
// configuration (see EntityConfig -- only DefaultMessageTTL is meaningful
// for a topic; cfg is variadic so pre-existing callers passing only a name
// keep compiling). If a topic with the same name already exists, created is
// false and err is nil, mirroring CreateQueue's idempotent-retry semantics.
func (b *InMemoryBackend) CreateTopic(name string, cfg ...EntityConfig) (bool, error) {
	b.mu.Lock("CreateTopic")
	defer b.mu.Unlock()

	if _, ok := b.topics[name]; ok {
		return false, nil
	}

	b.topics[name] = &storedTopic{
		Name:          name,
		CreatedAt:     b.now(),
		Config:        firstConfig(cfg),
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

// GetTopicInfo returns name's metadata. Returns ErrTopicNotFound if it does
// not exist.
func (b *InMemoryBackend) GetTopicInfo(name string) (TopicInfo, error) {
	b.mu.RLock("GetTopicInfo")
	defer b.mu.RUnlock()

	t, ok := b.topics[name]
	if !ok {
		return TopicInfo{}, ErrTopicNotFound
	}

	return topicInfoLocked(t), nil
}

// ListTopics returns every topic's metadata, sorted by name.
func (b *InMemoryBackend) ListTopics() []TopicInfo {
	b.mu.RLock("ListTopics")
	defer b.mu.RUnlock()

	names := make([]string, 0, len(b.topics))
	for name := range b.topics {
		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]TopicInfo, 0, len(names))
	for _, name := range names {
		out = append(out, topicInfoLocked(b.topics[name]))
	}

	return out
}

// ListSubscriptions returns every subscription of topic's metadata, sorted
// by name. Returns ErrTopicNotFound if topic does not exist.
func (b *InMemoryBackend) ListSubscriptions(topic string) ([]SubscriptionInfo, error) {
	b.mu.RLock("ListSubscriptions")
	defer b.mu.RUnlock()

	t, ok := b.topics[topic]
	if !ok {
		return nil, ErrTopicNotFound
	}

	names := make([]string, 0, len(t.Subscriptions))
	for name := range t.Subscriptions {
		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]SubscriptionInfo, 0, len(names))
	for _, name := range names {
		out = append(out, subscriptionInfoLocked(t.Subscriptions[name]))
	}

	return out, nil
}

func topicInfoLocked(t *storedTopic) TopicInfo {
	return TopicInfo{
		Name:              t.Name,
		CreatedAt:         t.CreatedAt,
		DefaultMessageTTL: t.Config.defaultMessageTTL(),
	}
}
