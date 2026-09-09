package azureservicebus

import "sort"

// CreateQueue creates a new, empty queue with the given configuration (see
// EntityConfig; cfg is variadic so pre-existing callers passing only a name
// keep compiling -- see the StorageBackend interface's doc comment). If a
// queue with the same name already exists, created is false, err is nil, and
// its existing configuration is left untouched (idempotent, mirroring
// services/azurequeue's CreateQueue). ErrQueueAlreadyExists is reserved for a
// future strict-create variant.
func (b *InMemoryBackend) CreateQueue(name string, cfg ...EntityConfig) (bool, error) {
	b.mu.Lock("CreateQueue")
	defer b.mu.Unlock()

	if _, ok := b.queues[name]; ok {
		return false, nil
	}

	b.queues[name] = &storedQueue{
		Name:         name,
		CreatedAt:    b.now(),
		Config:       firstConfig(cfg),
		messageQueue: newMessageQueue(),
	}

	return true, nil
}

// DeleteQueue removes a queue and all of its messages (including any
// dead-lettered ones). Returns ErrQueueNotFound if the queue does not exist.
func (b *InMemoryBackend) DeleteQueue(name string) error {
	b.mu.Lock("DeleteQueue")
	defer b.mu.Unlock()

	if _, ok := b.queues[name]; !ok {
		return ErrQueueNotFound
	}

	delete(b.queues, name)

	return nil
}

// QueueExists reports whether a queue named name exists.
func (b *InMemoryBackend) QueueExists(name string) bool {
	b.mu.RLock("QueueExists")
	defer b.mu.RUnlock()

	_, ok := b.queues[name]

	return ok
}

// GetQueueInfo returns name's metadata. Returns ErrQueueNotFound if it does
// not exist.
func (b *InMemoryBackend) GetQueueInfo(name string) (QueueInfo, error) {
	b.mu.RLock("GetQueueInfo")
	defer b.mu.RUnlock()

	q, ok := b.queues[name]
	if !ok {
		return QueueInfo{}, ErrQueueNotFound
	}

	return queueInfoLocked(q), nil
}

// ListQueues returns every queue's metadata, sorted by name.
func (b *InMemoryBackend) ListQueues() []QueueInfo {
	b.mu.RLock("ListQueues")
	defer b.mu.RUnlock()

	names := make([]string, 0, len(b.queues))
	for name := range b.queues {
		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]QueueInfo, 0, len(names))
	for _, name := range names {
		out = append(out, queueInfoLocked(b.queues[name]))
	}

	return out
}

func queueInfoLocked(q *storedQueue) QueueInfo {
	return QueueInfo{
		Name:              q.Name,
		CreatedAt:         q.CreatedAt,
		LockDuration:      q.Config.lockDuration(),
		MaxDeliveryCount:  int(q.Config.maxDeliveryCount()),
		DefaultMessageTTL: q.Config.defaultMessageTTL(),
	}
}
