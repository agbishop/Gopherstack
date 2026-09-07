package azureservicebus

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// azureServiceBusSnapshotVersion identifies the shape of backendSnapshot.
// Must be bumped whenever a change to storedQueue/storedTopic/
// storedSubscription/storedMessage would make an older snapshot unsafe to
// decode as the current shape; Restore discards (rather than partially
// decodes) any mismatch, mirroring services/azurequeue.
const azureServiceBusSnapshotVersion = 1

// backendSnapshot is the top-level on-disk shape for the Azure Service Bus
// backend. Queues/Topics serialise directly (no DTO layer).
type backendSnapshot struct {
	Queues  map[string]*storedQueue `json:"queues"`
	Topics  map[string]*storedTopic `json:"topics"`
	Version int                     `json:"version"`
	Seq     int64                   `json:"seq"`
}

// Snapshot serialises the backend state to JSON. It implements
// persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	snap := backendSnapshot{
		Version: azureServiceBusSnapshotVersion,
		Queues:  b.queues,
		Topics:  b.topics,
		Seq:     b.seq,
	}

	return persistence.MarshalSnapshot(ctx, "azureservicebus", snap)
}

// Restore loads backend state from a JSON snapshot. It implements
// persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "azureservicebus", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != azureServiceBusSnapshotVersion {
		logger.Load(ctx).WarnContext(ctx,
			"azureservicebus: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", azureServiceBusSnapshotVersion)

		b.queues = make(map[string]*storedQueue)
		b.topics = make(map[string]*storedTopic)
		b.seq = 0

		return nil
	}

	if err := validateQueueSnapshot(snap.Queues); err != nil {
		return err
	}

	if err := validateTopicSnapshot(snap.Topics); err != nil {
		return err
	}

	if snap.Queues == nil {
		snap.Queues = make(map[string]*storedQueue)
	}

	if snap.Topics == nil {
		snap.Topics = make(map[string]*storedTopic)
	}

	b.queues = snap.Queues
	b.topics = snap.Topics
	b.seq = snap.Seq

	return nil
}

// validateQueueSnapshot rejects a queues map containing null entries, name
// mismatches, or null messages -- any of which would panic later or let two
// views of the same entity disagree, mirroring services/azurequeue's
// identical Restore validation.
func validateQueueSnapshot(queues map[string]*storedQueue) error {
	for name, q := range queues {
		if q == nil {
			return fmt.Errorf("%w: %q", ErrSnapshotQueueNull, name)
		}

		if q.Name != name {
			return fmt.Errorf("%w: map key %q, Name %q", ErrSnapshotQueueNull, name, q.Name)
		}

		if err := validateMessageSlices(q.Messages, q.DeadLetter, "queue", name); err != nil {
			return err
		}
	}

	return nil
}

// validateTopicSnapshot rejects a topics map containing null entries, name
// mismatches, or null subscriptions/messages.
func validateTopicSnapshot(topics map[string]*storedTopic) error {
	for name, t := range topics {
		if t == nil {
			return fmt.Errorf("%w: %q", ErrSnapshotTopicNull, name)
		}

		if t.Name != name {
			return fmt.Errorf("%w: map key %q, Name %q", ErrSnapshotTopicNull, name, t.Name)
		}

		for subName, sub := range t.Subscriptions {
			if sub == nil {
				return fmt.Errorf("%w: topic %q, %q", ErrSnapshotSubscriptionNull, name, subName)
			}

			if sub.Name != subName {
				return fmt.Errorf("%w: topic %q, map key %q, Name %q",
					ErrSnapshotSubscriptionNull, name, subName, sub.Name)
			}

			if err := validateMessageSlices(sub.Messages, sub.DeadLetter, "subscription", subName); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateMessageSlices(live, deadLetter []*storedMessage, kind, name string) error {
	for i, msg := range live {
		if msg == nil {
			return fmt.Errorf("%w: index %d in %s %q", ErrSnapshotMessageNull, i, kind, name)
		}
	}

	for i, msg := range deadLetter {
		if msg == nil {
			return fmt.Errorf("%w: dead-letter index %d in %s %q", ErrSnapshotMessageNull, i, kind, name)
		}
	}

	return nil
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}
	if s, ok := h.Backend.(snapshotter); ok {
		return s.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}
	if r, ok := h.Backend.(restorer); ok {
		if err := r.Restore(ctx, data); err != nil {
			return fmt.Errorf("azureservicebus: restore snapshot: %w", err)
		}
	}

	return nil
}
