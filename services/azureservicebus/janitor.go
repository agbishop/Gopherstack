package azureservicebus

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	defaultJanitorInterval = 10 * time.Second
	janitorService         = "azureservicebus"
	janitorComponent       = "LockExpiryAndDeadLetterSweeper"
)

// Janitor is the Azure Service Bus background worker responsible for the two
// things real Service Bus does automatically over time: releasing a
// peek-locked message whose lock has expired without being Completed or
// Abandoned (making it available for redelivery, or dead-lettering it if
// that expiry pushed its delivery count past MaxDeliveryCount), and moving a
// TTL-expired message to its entity's dead-letter sub-queue. Mirrors
// services/azurequeue's Janitor/TTL-sweep shape.
type Janitor struct {
	Backend  *InMemoryBackend
	Interval time.Duration
}

// NewJanitor creates a new Azure Service Bus Janitor for the given backend.
func NewJanitor(backend *InMemoryBackend, interval time.Duration) *Janitor {
	if interval == 0 {
		interval = defaultJanitorInterval
	}

	return &Janitor{Backend: backend, Interval: interval}
}

// Run runs the janitor loop until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, janitorService)
	g.Ticker(janitorComponent, j.Interval, 0, j.sweep)

	<-ctx.Done()
	g.Stop()
}

// sweep releases expired locks and dead-letters TTL-expired messages across
// every queue and subscription.
func (j *Janitor) sweep(ctx context.Context) {
	stats := j.Backend.sweepOnce(j.Backend.now())

	if stats.Unlocked > 0 || stats.DeadLettered > 0 || stats.Expired > 0 {
		telemetry.RecordWorkerItems(janitorService, janitorComponent, stats.Unlocked+stats.DeadLettered+stats.Expired)
		logger.Load(ctx).InfoContext(ctx, "azureservicebus janitor: swept",
			"locksReleased", stats.Unlocked, "movedToDeadLetter", stats.DeadLettered,
			"ttlExpiredDeadLettered", stats.Expired)
	}

	telemetry.RecordWorkerTask(janitorService, janitorComponent, "success")
}

// sweepStats tallies what one sweep pass did: Unlocked counts locks released
// back to availability, DeadLettered counts messages moved to dead-letter
// because their delivery count was exhausted while locked, and Expired
// counts live (non-dead-letter) messages moved to dead-letter because their
// TTL elapsed.
type sweepStats struct {
	Unlocked     int
	DeadLettered int
	Expired      int
}

func (s *sweepStats) add(other sweepStats) {
	s.Unlocked += other.Unlocked
	s.DeadLettered += other.DeadLettered
	s.Expired += other.Expired
}

// sweepOnce performs one full sweep pass over every queue and subscription's
// messageQueue.
func (b *InMemoryBackend) sweepOnce(now time.Time) sweepStats {
	b.mu.Lock("sweepOnce")
	defer b.mu.Unlock()

	var total sweepStats

	for _, q := range b.queues {
		total.add(sweepMessageQueueLocked(&q.messageQueue, now))
	}

	for _, t := range b.topics {
		for _, sub := range t.Subscriptions {
			total.add(sweepMessageQueueLocked(&sub.messageQueue, now))
		}
	}

	return total
}

// sweepMessageQueueLocked sweeps one entity's live message list: an
// expired lock is released (and, if that message's delivery count has
// reached MaxDeliveryCount, moved to DeadLetter instead of being released);
// a TTL-expired message is moved to DeadLetter outright, regardless of lock
// state. The dead-letter sub-queue itself is swept only for TTL expiry (a
// dead-lettered message that also expires is simply dropped -- there is
// nowhere further for it to go). Callers must hold b.mu for writing.
func sweepMessageQueueLocked(mq *messageQueue, now time.Time) sweepStats {
	var stats sweepStats

	kept := mq.Messages[:0]

	for _, msg := range mq.Messages {
		switch {
		case msg.isExpired(now):
			msg.LockToken = ""
			msg.LockedUntil = time.Time{}
			// A message moved to dead-letter because its original TTL
			// elapsed must not immediately re-expire out of the dead-letter
			// sub-queue too -- real Service Bus's dead-letter sub-queue is
			// not subject to the entity's message TTL. Give it a fresh
			// window instead.
			msg.ExpiresAt = now.Add(DefaultMessageTTL)
			mq.DeadLetter = append(mq.DeadLetter, msg)

			stats.Expired++
		case msg.isLocked(now):
			kept = append(kept, msg)
		default:
			// Lock already absent or already released; nothing to sweep.
			kept = append(kept, msg)
		}
	}

	// A second pass catches locks that just expired (LockedUntil in the past
	// but LockToken still set): release them, dead-lettering on
	// delivery-count exhaustion.
	final := kept[:0]

	for _, msg := range kept {
		if msg.LockToken != "" && !msg.LockedUntil.After(now) {
			msg.LockToken = ""
			msg.LockedUntil = time.Time{}
			stats.Unlocked++

			if msg.DeliveryCount >= MaxDeliveryCount {
				mq.DeadLetter = append(mq.DeadLetter, msg)
				stats.DeadLettered++

				continue
			}
		}

		final = append(final, msg)
	}

	mq.Messages = final

	// Dead-lettered messages are still subject to their own TTL: drop (not
	// re-dead-letter) any that have expired while sitting in DeadLetter.
	keptDL := mq.DeadLetter[:0]

	for _, msg := range mq.DeadLetter {
		if msg.isExpired(now) {
			continue
		}

		keptDL = append(keptDL, msg)
	}

	mq.DeadLetter = keptDL

	return stats
}
