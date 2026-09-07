package azureservicebus

// CreateQueue creates a new, empty queue. If a queue with the same name
// already exists, created is false and err is nil (idempotent, mirroring
// services/azurequeue's CreateQueue -- this backend has no queue metadata to
// compare, so any pre-existing queue is "the same"). ErrQueueAlreadyExists is
// reserved for a future metadata-bearing Create.
func (b *InMemoryBackend) CreateQueue(name string) (bool, error) {
	b.mu.Lock("CreateQueue")
	defer b.mu.Unlock()

	if _, ok := b.queues[name]; ok {
		return false, nil
	}

	b.queues[name] = &storedQueue{
		Name:      name,
		CreatedAt: b.now(),
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
