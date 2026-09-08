package sqs

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// MetricEmitter emits a CloudWatch metric data point.
// It is implemented by the CloudWatch backend and injected into InMemoryBackend
// so that SQS operations can be forwarded to CloudWatch as metrics.
type MetricEmitter interface {
	EmitMetric(namespace, name string, value float64, unit string) error
}

// MetricEmitterFunc is a function adapter for MetricEmitter.
type MetricEmitterFunc func(namespace, name string, value float64, unit string) error

// EmitMetric implements MetricEmitter.
func (f MetricEmitterFunc) EmitMetric(namespace, name string, value float64, unit string) error {
	return f(namespace, name, value, unit)
}

// sqsMetricNamespace is the CloudWatch namespace used for SQS metrics.
const sqsMetricNamespace = "AWS/SQS"

const sqsMetricUnitCount = "Count"

// InMemoryBackend implements StorageBackend using in-memory maps.
type InMemoryBackend struct {
	metricEmitter MetricEmitter
	svcCtx        context.Context
	// registry lets Reset collapse the queues/moveTasks lifecycle to one call
	// (registry.ResetAll()) instead of hand-rolled re-initialization of each map.
	registry       *store.Registry
	queues         *store.Table[Queue]
	moveTasks      *store.Table[moveTaskState]
	snsUnsubscribe func()
	janitorStop    chan struct{}
	mu             *lockmetrics.RWMutex
	// recentlyDeleted maps a queueKey(region, name) to the time DeleteQueue was
	// called for it, so CreateQueue can enforce AWS's 60-second
	// wait-before-recreate rule (ErrQueueDeletedRecently). Guarded by b.mu, the
	// same lock CreateQueue/DeleteQueue already hold. Entries older than
	// queueDeletedRecentlyWindowSecs are pruned lazily by CreateQueue's own
	// check and swept periodically by pruneState so the map cannot grow
	// unboundedly across a long-lived backend that deletes many queues.
	recentlyDeleted map[string]time.Time
	accountID       string
	region          string
}

// queueTableKey is the [store.Table] key function for b.queues, deriving the
// same (region, name) composite key that queueKey builds from raw input.
func queueTableKey(q *Queue) string {
	return queueKey(q.Region, q.Name)
}

// SetMetricEmitter sets the emitter used to forward SQS operation metrics to CloudWatch.
func (b *InMemoryBackend) SetMetricEmitter(e MetricEmitter) {
	b.mu.Lock("SetMetricEmitter")
	defer b.mu.Unlock()

	b.metricEmitter = e
}

func (b *InMemoryBackend) emitMetric(name string, value float64) {
	var e MetricEmitter
	func() {
		b.mu.RLock("emitMetric")
		defer b.mu.RUnlock()

		e = b.metricEmitter
	}()

	if e == nil {
		return
	}

	// Emit asynchronously without holding the lock.
	go func() {
		_ = e.EmitMetric(sqsMetricNamespace, name, value, sqsMetricUnitCount)
	}()
}

const sqsDefaultMaxResults = 1000

// NewInMemoryBackend creates a new empty InMemoryBackend with default account/region and a background service context.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(config.DefaultAccountID, config.DefaultRegion)
}

// NewInMemoryBackendWithConfig creates a new InMemoryBackend with the given account ID and region
// and a background service context.
func NewInMemoryBackendWithConfig(accountID, region string) *InMemoryBackend {
	return NewInMemoryBackendWithContext(context.Background(), accountID, region)
}

// NewInMemoryBackendWithContext creates a new InMemoryBackend whose background goroutines
// are bounded by svcCtx. If svcCtx is nil, [context.Background] is used.
func NewInMemoryBackendWithContext(svcCtx context.Context, accountID, region string) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	b := &InMemoryBackend{
		registry:        store.NewRegistry(),
		accountID:       accountID,
		region:          region,
		mu:              lockmetrics.New("sqs"),
		svcCtx:          svcCtx,
		recentlyDeleted: make(map[string]time.Time),
	}

	b.queues = store.Register(b.registry, "queues", store.New(queueTableKey))
	b.moveTasks = store.Register(b.registry, "moveTasks", store.New(moveTaskTableKey))

	b.startJanitor()

	return b
}

// Close stops the background janitor goroutine and releases associated
// resources.  It is safe to call Close multiple times; subsequent calls are
// no-ops.  The backend must not be used after Close returns.
func (b *InMemoryBackend) Close() {
	b.stopInternalJanitor()
	b.cancelAllMoveTasks()
}

// queueNameFromInput extracts the queue name from a queue URL.
func queueNameFromInput(queueURL string) string {
	parts := strings.Split(queueURL, "/")
	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
}

// effectiveRegion returns the region to scope a backend lookup against.
// Empty input falls back to the backend's default region so single-region
// callers (and existing tests) continue to work without explicit region threading.
func (b *InMemoryBackend) effectiveRegion(region string) string {
	if region != "" {
		return region
	}

	return b.region
}

// queueKey is the composite map key used by b.queues, formed from
// (region, name). Two queues with the same name in different regions occupy
// distinct keys, matching AWS semantics where queue names are scoped per-region.
func queueKey(region, name string) string {
	return region + "/" + name
}

// lookupQueueByName returns the queue stored under (region, name), or false if
// no queue exists in that region with that name.
func (b *InMemoryBackend) lookupQueueByName(region, name string) (*Queue, bool) {
	return b.queues.Get(queueKey(b.effectiveRegion(region), name))
}

// lookupQueueByURL finds a queue by its URL.
//
// The queue MUST live in the supplied region; a queue with the same name in a
// different region is treated as not found, matching real AWS where the SigV4
// region must match the regional endpoint. Lookup uses the queue name extracted
// from the URL plus effectiveRegion(region) — we do NOT require the stored
// q.URL to be byte-identical to the caller's queueURL because SDKs and proxy
// hops may rewrite the host/port (e.g. host.docker.internal vs localhost).
//
// When region is empty, effectiveRegion falls back to the backend's default
// region so single-region callers continue to work without explicit threading.
// The previous O(n) URL-string scan across all regions has been removed because
// it defeated region isolation: a caller in us-east-1 could accidentally find a
// queue created in us-west-2 if the URL strings happened to match.
func (b *InMemoryBackend) lookupQueueByURL(region, queueURL string) (*Queue, bool) {
	name := queueNameFromInput(queueURL)

	return b.queues.Get(queueKey(b.effectiveRegion(region), name))
}

// ListAll returns a snapshot of all queues as QueueInfo values.
// The returned slice contains value copies of the immutable queue metadata, safe for
// concurrent use after the lock is released.
func (b *InMemoryBackend) ListAll() []QueueInfo {
	b.mu.RLock("ListAll")
	defer b.mu.RUnlock()

	result := make([]QueueInfo, 0, b.queues.Len())

	for _, q := range b.queues.All() {
		result = append(result, QueueInfo{Name: q.Name, URL: q.URL, IsFIFO: q.IsFIFO})
	}

	return result
}

// findQueueByARN scans for the queue whose QueueArn attribute equals queueARN.
// Must be called with b.mu held (either read or write).
func (b *InMemoryBackend) findQueueByARN(queueARN string) (*Queue, bool) {
	for _, q := range b.queues.All() {
		if q.Attributes[attrQueueArn] == queueARN {
			return q, true
		}
	}

	return nil, false
}

// QueueExists reports whether a queue with the given ARN exists. Used by
// cross-service callers (e.g. SNS's RedrivePolicy.deadLetterTargetArn check)
// to verify a referenced queue is real.
func (b *InMemoryBackend) QueueExists(queueARN string) bool {
	b.mu.RLock("QueueExists")
	defer b.mu.RUnlock()

	_, ok := b.findQueueByARN(queueARN)

	return ok
}

// totalMessages returns the total in-memory message count across all queues
// (visible + in-flight + DLQ retained).
func (b *InMemoryBackend) totalMessages() int {
	b.mu.RLock("totalMessages")
	defer b.mu.RUnlock()

	total := 0
	for _, q := range b.queues.All() {
		total += len(q.messages) + len(q.inFlightMessages)
	}

	return total
}

// Purge removes all queues created before the given cutoff time.
func (b *InMemoryBackend) Purge(ctx context.Context, cutoff time.Time) {
	if ctx.Err() != nil {
		return
	}

	b.mu.Lock("Purge")
	defer b.mu.Unlock()

	for _, q := range b.queues.All() {
		if ctx.Err() != nil {
			return
		}

		createdStr, ok := q.Attributes[attrCreatedTimestamp]
		if !ok {
			continue
		}

		createdUnix, err := strconv.ParseInt(createdStr, 10, 64)
		if err != nil {
			continue
		}

		if time.Unix(createdUnix, 0).Before(cutoff) {
			b.purgeQueue(q)
		}
	}
}

// purgeQueue closes and removes a single queue and cancels any move tasks that involve it.
// Caller must hold b.mu.
func (b *InMemoryBackend) purgeQueue(q *Queue) {
	close(q.notify)
	if q.Tags != nil {
		q.Tags.Close()
	}
	b.queues.Delete(queueTableKey(q))

	queueARN := q.Attributes[attrQueueArn]
	for _, task := range b.moveTasks.All() {
		b.cancelMoveTaskIfInvolved(task, queueARN)
	}
}

// Reset clears all in-memory state from the database. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
// The active SNS subscription listener is kept intact so that SNS→SQS delivery
// continues to work after a reset (wireSNSToSQS is only wired at startup and is
// not re-run after reset).
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	// Close all queue tag stores to prevent resource leaks.
	for _, q := range b.queues.All() {
		if q.Tags != nil {
			q.Tags.Close()
		}
	}

	// Cancel any running move tasks.
	for _, task := range b.moveTasks.All() {
		task.cancel()
	}

	b.registry.ResetAll()
	clear(b.recentlyDeleted)
}

// queueURLFromARNLocked returns the URL and ARN of the queue with the given ARN.
// Must be called with b.mu held (either read or write).
func (b *InMemoryBackend) queueURLFromARNLocked(queueARN string) (string, bool) {
	q, ok := b.findQueueByARN(queueARN)
	if !ok {
		return "", false
	}

	return q.URL, true
}
