package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

const (
	defaultDDBJanitorInterval  = 500 * time.Millisecond
	defaultDDBTTLSweepInterval = 5 * time.Second
	// defaultPITRSnapshotInterval is the cadence at which the janitor takes a new
	// PITR snapshot of every PITR-enabled table, on its own ticker independent of
	// the 500ms housekeeping sweep. maxPITRSnapshots(60) * this interval must equal
	// the ~1 hour of recovery coverage store.go's ring doc comment promises -- at
	// the housekeeping sweep's 500ms cadence the same 60-slot ring would only cover
	// 30s.
	//
	// Memory trade-off: each snapshot deep-copies every item in the table, so
	// retaining 60 of them can grow a PITR-enabled table's footprint up to ~61x its
	// live item size. The 1-minute cadence is what keeps that acceptable.
	defaultPITRSnapshotInterval = time.Minute
	// defaultTTLSweepBatchSize is the maximum number of items checked per lock acquisition
	// in sweepTableTTL. Smaller values reduce lock hold time at the cost of more
	// lock round-trips; 1 000 is a reasonable balance for typical table sizes.
	defaultTTLSweepBatchSize = 1_000
	// txnTokensMaxCap is the maximum number of committed idempotency tokens kept
	// in memory. When the cap is exceeded, the oldest half is evicted immediately
	// rather than waiting for the TTL sweep, preventing unbounded map growth.
	txnTokensMaxCap = 100_000
	// txnPendingMaxCap is the equivalent cap for in-progress idempotency tokens.
	txnPendingMaxCap = 100_000
	// streamExpirySeconds is the age after which stream record images are compacted.
	streamExpirySeconds = 24 * 60 * 60
	// txnCapEvictionFraction is the divisor used to compute how many entries to evict
	// when the hard cap is exceeded. Evicting half (len/2) ensures the map drops
	// well below the cap in one pass.
	txnCapEvictionFraction = 2
)

// Janitor is the DynamoDB background worker that finalises tables queued for
// async deletion and records queue-depth metrics for the live dashboard.
type Janitor struct {
	Backend  *InMemoryDB
	Interval time.Duration
	// ttlSweepBatchSize is the maximum number of items checked per lock
	// acquisition when sweeping TTL-expired items.
	ttlSweepBatchSize int
	// TaskTimeout bounds each individual janitor task (TTL sweep, table cleaner, etc.).
	// When non-zero, each task runs with a child context that expires after this duration,
	// preventing a stalled operation from blocking the janitor loop indefinitely.
	TaskTimeout time.Duration
}

// NewJanitor creates a new DynamoDB Janitor for the given backend.
// The janitor interval is taken from the provided settings;
// if zero, it falls back to defaultDDBJanitorInterval.
func NewJanitor(backend *InMemoryDB, settings Settings) *Janitor {
	interval := settings.JanitorInterval
	if interval == 0 {
		interval = defaultDDBJanitorInterval
	}

	ttlSweepBatchSize := settings.TTLSweepBatchSize
	if ttlSweepBatchSize <= 0 {
		ttlSweepBatchSize = defaultTTLSweepBatchSize
	}

	return &Janitor{
		Backend:           backend,
		Interval:          interval,
		ttlSweepBatchSize: ttlSweepBatchSize,
	}
}

// Run runs the janitor loop until ctx is cancelled.
// Three independent tickers are used:
//   - the main ticker (Interval, default 500ms): housekeeping tasks (table
//     cleanup, txn-token sweeps, expression-cache evictions).
//   - the TTL ticker (defaultDDBTTLSweepInterval, 5s): per-table TTL and
//     stream-record sweeps, which are O(tables × items) and too expensive to run
//     every 500ms.
//   - the PITR ticker (defaultPITRSnapshotInterval, 1 minute): per-table PITR
//     snapshotting, deliberately decoupled from the 500ms main ticker so the
//     snapshot ring's documented ~1-hour coverage window is real -- see
//     defaultPITRSnapshotInterval's doc comment for why.
//
// Each sweep is panic-recovered and bounded by TaskTimeout (if non-zero) by the
// worker primitive.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, "dynamodb")
	g.Ticker("TableCleaner", j.Interval, j.TaskTimeout, j.sweepMain)
	g.Ticker("TTLSweeper", defaultDDBTTLSweepInterval, j.TaskTimeout, j.sweepTTLAndStreams)
	g.Ticker("PITRSnapshotter", defaultPITRSnapshotInterval, j.TaskTimeout, j.snapshotPITRTables)

	<-ctx.Done()
	g.Stop()
}

// sweepMain runs the housekeeping pass: txn-token/pending sweeps, cache
// evictions, and finalising tables queued for deletion. PITR snapshotting runs
// on its own slower ticker (see Run) and is not part of this pass.
func (j *Janitor) sweepMain(ctx context.Context) {
	j.sweepTxnTokens(ctx)
	j.sweepTxnPending(ctx)
	j.Backend.exprCache.Sweep()
	j.Backend.iteratorStore.Sweep()
	j.runTableCleaner(ctx)
}

// sweepTTLAndStreams runs the per-table TTL eviction and stream-record
// compaction pass.
func (j *Janitor) sweepTTLAndStreams(ctx context.Context) {
	j.sweepTTL(ctx)
	j.sweepStreamRecords(ctx)
}

// runOnce orchestrates all janitor tasks -- including a PITR snapshot pass,
// which in production runs on its own slower ticker (see Run) -- in a single
// synchronous pass. Called by tests via SweepOnce; production code uses the
// three-ticker Run loop above.
func (j *Janitor) runOnce(ctx context.Context) {
	j.sweepTTL(ctx)
	j.sweepTxnTokens(ctx)
	j.sweepTxnPending(ctx)
	j.sweepStreamRecords(ctx)
	j.Backend.exprCache.Sweep()
	j.Backend.iteratorStore.Sweep()
	j.snapshotPITRTables(ctx)
	j.runTableCleaner(ctx)
}

// snapshotPITRTables captures a point-in-time snapshot of every PITR-enabled
// table's items into its per-table ring buffer. Restore uses these to honour
// RestoreDateTime. Iteration is under db.mu read lock; the per-table read
// happens with table.mu so writers don't see a torn copy.
func (j *Janitor) snapshotPITRTables(_ context.Context) {
	db := j.Backend

	tables := allTablesRLocked(db)

	now := time.Now().UTC()

	for _, t := range tables {
		snapshotTablePITRLocked(t, now)
	}
}

// allTablesRLocked returns every table in db.tables under a defer-protected
// db.mu.RLock.
func allTablesRLocked(db *InMemoryDB) []*Table {
	db.mu.RLock("DDBJanitor.snapshotPITR")
	defer db.mu.RUnlock()

	return db.tables.All()
}

// snapshotTablePITRLocked appends a PITR snapshot of t's items (capped at
// maxPITRSnapshots) under a defer-protected table.mu.Lock. No-op if PITR is
// not enabled on t.
func snapshotTablePITRLocked(t *Table, now time.Time) {
	t.mu.Lock("snapshotPITR")
	defer t.mu.Unlock()

	if !t.PITREnabled {
		return
	}

	snap := pitrSnapshot{Taken: now, Items: deepCopyItems(t.Items)}
	t.PITRSnapshots = append(t.PITRSnapshots, snap)
	if len(t.PITRSnapshots) > maxPITRSnapshots {
		t.PITRSnapshots = t.PITRSnapshots[len(t.PITRSnapshots)-maxPITRSnapshots:]
	}
}

// SweepOnce runs a single full sweep pass. Exposed for testing.
func (j *Janitor) SweepOnce(ctx context.Context) {
	j.runOnce(ctx)
}

// runTableCleaner records the current queue depth and finalises all pending deletions.
func (j *Janitor) runTableCleaner(ctx context.Context) {
	db := j.Backend

	// Snapshot the tables to clean up and remove them from deletingTables under the
	// global lock. The slow work (timer cancellation, mutex close) is done outside the
	// lock so that concurrent DescribeTable / PutItem / Query calls are not stalled
	// while thousands of per-table resources are being released.
	tablesToClose, depth := drainDeletingTablesLocked(db)

	// Release per-table resources outside the global lock.
	for _, table := range tablesToClose {
		stopTableTimers(table)
		if table.Tags != nil {
			table.Tags.Close()
		}
		table.mu.Close()
	}

	telemetry.RecordWorkerQueueDepth("dynamodb", "TableCleaner", depth)
	telemetry.RecordWorkerTask("dynamodb", "TableCleaner", "success")
	telemetry.RecordWorkerItems("dynamodb", "TableCleaner", depth)

	for _, table := range tablesToClose {
		logger.Load(ctx).InfoContext(ctx, "DynamoDB janitor: table deleted", "table", table.Name)
	}
}

// drainDeletingTablesLocked snapshots every table queued in db.deletingTables
// and resets the queue, under a defer-protected db.mu.Lock.
func drainDeletingTablesLocked(db *InMemoryDB) ([]*Table, int) {
	db.mu.Lock("DDBJanitor")
	defer db.mu.Unlock()

	tablesToClose := db.deletingTables.All()
	depth := len(tablesToClose)
	db.deletingTables.Reset()

	return tablesToClose, depth
}

// sweepTTL iterates over all tables, finds those with TTL enabled,
// and evicts expired items based on the configured TTL attribute.
func (j *Janitor) sweepTTL(ctx context.Context) {
	db := j.Backend
	tables := db.ListAllTables()
	totalEvicted := 0

	replicationQueue := make([]ttlReplicationEntry, 0, len(tables))

	for _, table := range tables {
		count, pending := j.sweepTableTTL(ctx, db, table)
		totalEvicted += count
		replicationQueue = append(replicationQueue, pending...)
	}

	for _, entry := range replicationQueue {
		db.replicateItemMutation(
			entry.tableName,
			entry.globalTableName,
			entry.region,
			entry.item,
			"DELETE",
		)
	}

	if totalEvicted > 0 {
		telemetry.RecordWorkerItems("dynamodb", "TTLSweeper", totalEvicted)
	}

	telemetry.RecordWorkerTask("dynamodb", "TTLSweeper", "success")
}

type ttlReplicationEntry struct {
	item            map[string]any
	tableName       string
	globalTableName string
	region          string
}

// sweepTableTTL evicts TTL-expired items from a single table and returns
// the number evicted plus any global-table replication entries to process.
//
// Items are processed in batches of ttlSweepBatchSize so the table write lock is
// not held for the full scan. Between batches the lock is released, giving other
// goroutines (reads, writes) a chance to acquire it. The sweep is also aborted
// early when ctx is cancelled (e.g. on shutdown or task timeout).
func (j *Janitor) sweepTableTTL(
	ctx context.Context,
	db *InMemoryDB,
	table *Table,
) (int, []ttlReplicationEntry) {
	ttlAttr, gtName, tableARN := ttlSweepMetaRLocked(table)

	if ttlAttr == "" {
		return 0, nil
	}

	region := db.regionFromARN(tableARN)

	var pending []ttlReplicationEntry
	totalEvicted := 0
	start := time.Now()

	// Process in batches: acquire the write lock, scan up to batchSize items,
	// release the lock, then repeat until the full slice has been covered.
	// Scanning backwards keeps index arithmetic correct after deleteItemAtIndex.
	i := -1 // sentinel: start from last element on first batch

	for ctx.Err() == nil {
		var batchEvicted int
		var batchPending []ttlReplicationEntry

		i, batchEvicted, batchPending = j.sweepTTLBatchLocked(db, table, ttlAttr, gtName, region, i)
		pending = append(pending, batchPending...)
		totalEvicted += batchEvicted

		// All items scanned.
		if i < 0 {
			break
		}
	}

	if totalEvicted > 0 {
		logger.Load(ctx).InfoContext(ctx, "DynamoDB janitor: TTL items evicted",
			"table", table.Name,
			"count", totalEvicted,
			"duration", time.Since(start))
	}

	return totalEvicted, pending
}

// ttlSweepMetaRLocked returns the TTL attribute, global-table name, and table
// ARN needed by sweepTableTTL, under a defer-protected table.mu.RLock.
func ttlSweepMetaRLocked(table *Table) (string, string, string) {
	table.mu.RLock("TTLSweepCheck")
	defer table.mu.RUnlock()

	return table.TTLAttribute, table.GlobalTableName, table.TableArn
}

// sweepTTLBatchLocked scans and evicts up to j.ttlSweepBatchSize expired items
// starting at index i (scanning backwards), under a single defer-protected
// table.mu.Lock -- released as soon as this call returns, so the lock is held
// only for one batch, not the whole multi-batch sweep loop, while still
// guaranteeing a panic partway through a batch can never leave table.mu locked
// forever.
func (j *Janitor) sweepTTLBatchLocked(
	db *InMemoryDB,
	table *Table,
	ttlAttr, gtName, region string,
	i int,
) (int, int, []ttlReplicationEntry) {
	table.mu.Lock("TTLSweep")
	defer table.mu.Unlock()

	batchEvicted := 0

	var pending []ttlReplicationEntry

	// Clamp i in case concurrent deletes shrank table.Items between batches.
	if n := len(table.Items) - 1; i > n {
		i = n
	}

	if i == -1 {
		i = len(table.Items) - 1
	}

	batchEnd := i - j.ttlSweepBatchSize
	if batchEnd < 0 {
		batchEnd = -1
	}

	for ; i > batchEnd; i-- {
		item := table.Items[i]

		if !isItemExpiredWithGrace(item, ttlAttr, TTLGracePeriod) {
			continue
		}

		// Copy the item once; the stream record and replication entry each
		// need their own copy so they can be mutated independently.
		itemCopy := deepCopyItem(item)
		table.appendStreamRecord(streamEventRemove, itemCopy, nil, "dynamodb.amazonaws.com", "Service")
		batchEvicted++

		if gtName != "" {
			pending = append(pending, ttlReplicationEntry{
				tableName:       table.Name,
				globalTableName: gtName,
				region:          region,
				item:            deepCopyItem(item),
			})
		}

		db.deleteItemAtIndex(table, i)
	}

	return i, batchEvicted, pending
}

// sweepTxnTokens removes committed idempotency tokens that have exceeded their TTL.
// AWS DynamoDB expires tokens after 10 minutes; this prevents unbounded map growth.
// If the map exceeds txnTokensMaxCap entries the oldest half is evicted immediately.
//
// Optimised two-phase sweep: expired keys are identified under a read lock (allowing
// concurrent reads to proceed), then deleted under a write lock. The write lock is
// only acquired when there is actual work to do, keeping contention minimal on the
// common "nothing expired" path.
func (j *Janitor) sweepTxnTokens(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	db := j.Backend
	now := time.Now()

	// Phase 1: identify expired keys under read lock.
	expired, capExceeded := scanExpiredTxnTokensRLocked(db, now)

	// Fast path: nothing to do.
	if len(expired) == 0 && !capExceeded {
		return
	}

	// Phase 2: apply deletions under write lock.
	db.mu.Lock("sweepTxnTokens.delete")
	defer db.mu.Unlock()

	for _, token := range expired {
		delete(db.txnTokens, token)
	}

	// Hard cap: if still over limit, evict the oldest half to prevent OOM.
	if len(db.txnTokens) > txnTokensMaxCap {
		evictOldestTokens(db.txnTokens, len(db.txnTokens)/txnCapEvictionFraction)
	}
}

// scanExpiredTxnTokensRLocked identifies committed idempotency tokens past
// their expiry (and whether the cap is exceeded) under a defer-protected
// db.mu.RLock.
func scanExpiredTxnTokensRLocked(db *InMemoryDB, now time.Time) ([]string, bool) {
	db.mu.RLock("sweepTxnTokens.scan")
	defer db.mu.RUnlock()

	var expired []string

	for token, rec := range db.txnTokens {
		if now.After(rec.expiry) {
			expired = append(expired, token)
		}
	}

	return expired, len(db.txnTokens) > txnTokensMaxCap
}

// sweepTxnPending removes in-progress idempotency tokens that have exceeded txnPendingTTL.
// Under normal operation the defer in TransactWriteItems cleans up pending entries.
// This sweep is a safety net for orphaned entries (e.g. from a crashed goroutine).
// If the map exceeds txnPendingMaxCap entries the oldest half is evicted immediately.
//
// Uses the same two-phase snapshot approach as sweepTxnTokens.
func (j *Janitor) sweepTxnPending(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	db := j.Backend
	now := time.Now()

	// Phase 1: identify stale keys under read lock.
	stale, capExceeded := scanStaleTxnPendingRLocked(db, now)

	// Fast path: nothing to do.
	if len(stale) == 0 && !capExceeded {
		return
	}

	// Phase 2: apply deletions under write lock.
	db.mu.Lock("sweepTxnPending.delete")
	defer db.mu.Unlock()

	for _, token := range stale {
		delete(db.txnPending, token)
	}

	// Hard cap: if still over limit, evict the oldest half to prevent OOM.
	if len(db.txnPending) > txnPendingMaxCap {
		evictOldestPending(db.txnPending, len(db.txnPending)/txnCapEvictionFraction)
	}
}

// scanStaleTxnPendingRLocked identifies in-progress idempotency tokens older
// than txnPendingTTL (and whether the cap is exceeded) under a
// defer-protected db.mu.RLock.
func scanStaleTxnPendingRLocked(db *InMemoryDB, now time.Time) ([]string, bool) {
	db.mu.RLock("sweepTxnPending.scan")
	defer db.mu.RUnlock()

	var stale []string

	for token, startTime := range db.txnPending {
		if now.Sub(startTime) > txnPendingTTL {
			stale = append(stale, token)
		}
	}

	return stale, len(db.txnPending) > txnPendingMaxCap
}

// evictOldestTokens removes the n oldest entries from m (oldest = earliest expiry time).
// Must be called with db.mu held.
func evictOldestTokens(m map[string]txnTokenRecord, n int) {
	if n <= 0 {
		return
	}

	// Find the nth smallest expiry time using partial selection — O(len(m)) space.
	times := make([]time.Time, 0, len(m))
	for _, rec := range m {
		times = append(times, rec.expiry)
	}

	threshold := nthSmallest(times, n)

	evicted := 0

	for k, rec := range m {
		if evicted >= n {
			break
		}

		if !rec.expiry.After(threshold) {
			delete(m, k)
			evicted++
		}
	}
}

// evictOldestPending removes the n oldest entries from m (oldest = earliest start time).
// Must be called with db.mu held.
func evictOldestPending(m map[string]time.Time, n int) {
	if n <= 0 {
		return
	}

	times := make([]time.Time, 0, len(m))
	for _, t := range m {
		times = append(times, t)
	}

	threshold := nthSmallest(times, n)

	evicted := 0

	for k, t := range m {
		if evicted >= n {
			break
		}

		if !t.After(threshold) {
			delete(m, k)
			evicted++
		}
	}
}

// nthSmallest returns the nth smallest [time.Time] in ts (1-indexed) using a
// Floyd-Rivest / introselect-style quickselect algorithm — O(n) average, O(n²)
// worst case. For the token-eviction call site n is always len(ts)/2, so the
// average complexity is the relevant bound.
//
// If n >= len(ts) it returns the maximum value. ts is mutated in place; callers
// must not rely on its order after the call.
func nthSmallest(ts []time.Time, n int) time.Time {
	if n <= 0 || len(ts) == 0 {
		return time.Time{}
	}

	if n >= len(ts) {
		return sliceMax(ts)
	}

	// Quickselect: partition around a pivot until the kth element is in place.
	// k is 0-indexed here (n is 1-indexed from the caller).
	k := n - 1
	lo, hi := 0, len(ts)-1

	for lo < hi {
		p := partition(ts, lo, hi)

		if p <= k {
			lo = p + 1
		}

		if p >= k {
			hi = p - 1
		}
	}

	return ts[k]
}

// sliceMax returns the largest time.Time in ts. ts must not be empty.
func sliceMax(ts []time.Time) time.Time {
	latest := ts[0]

	for _, t := range ts[1:] {
		if t.After(latest) {
			latest = t
		}
	}

	return latest
}

// partition runs a Hoare-variant partition on ts[lo..hi] using a median-of-three
// pivot. Returns the final pivot index after partitioning.
// Extracted from nthSmallest to keep per-function cognitive complexity below 20.
func partition(ts []time.Time, lo, hi int) int {
	// minPartitionWindow is the window size below which sortThree has already fully
	// sorted the elements and further pivot placement is unnecessary.
	const minPartitionWindow = 2

	mid := lo + (hi-lo)/minPartitionWindow
	sortThree(ts, lo, mid, hi)

	// Window of 2 or fewer elements is already sorted by sortThree; return mid.
	if hi-lo < minPartitionWindow {
		return mid
	}

	pivot := ts[mid]

	// Place pivot at hi-1 so both scan directions can proceed inward.
	ts[mid], ts[hi-1] = ts[hi-1], ts[mid]

	i, j := lo, hi-1

	for {
		i++
		for ts[i].Before(pivot) {
			i++
		}

		j--
		for pivot.Before(ts[j]) {
			j--
		}

		if i >= j {
			break
		}

		ts[i], ts[j] = ts[j], ts[i]
	}

	ts[i], ts[hi-1] = ts[hi-1], ts[i]

	return i
}

// sortThree sorts the three elements at positions a, b, c in ts into ascending order.
func sortThree(ts []time.Time, a, b, c int) {
	if ts[c].Before(ts[a]) {
		ts[a], ts[c] = ts[c], ts[a]
	}

	if ts[b].Before(ts[a]) {
		ts[a], ts[b] = ts[b], ts[a]
	}

	if ts[c].Before(ts[b]) {
		ts[b], ts[c] = ts[c], ts[b]
	}
}

// sweepStreamRecords compacts stream records that are older than 24 hours.
// For each table the function does two passes under the table write lock:
//  1. Nil out OldImage/NewImage on expired records (saves heap) and count fully
//     expired records (both images already nil and timestamp is stale).
//  2. If the proportion of compacted records is high, compact the ring buffer
//     by removing tombstone entries so the slice does not grow monotonically.
func (j *Janitor) sweepStreamRecords(ctx context.Context) {
	db := j.Backend
	tables := db.ListAllTables()
	now := time.Now().Unix()

	for _, t := range tables {
		if ctx.Err() != nil {
			return
		}

		cleared := sweepTableStreamRecordsLocked(t, now)

		if cleared > 0 {
			telemetry.RecordWorkerItems("dynamodb", "StreamSweeper", cleared)
		}
	}

	telemetry.RecordWorkerTask("dynamodb", "StreamSweeper", "success")
}

// sweepTableStreamRecordsLocked nils expired stream-record images and, when
// more than half the ring is expired tombstones, compacts the ring buffer,
// all under a single defer-protected table.mu.Lock. Returns the number of
// records whose images were cleared.
func sweepTableStreamRecordsLocked(t *Table, now int64) int {
	t.mu.Lock("SweepStreamRecords")
	defer t.mu.Unlock()

	cleared := 0
	tombstones := 0

	for i := range t.StreamRecords {
		r := &t.StreamRecords[i]
		if r.ApproximateCreationDateTime <= 0 {
			continue
		}

		age := now - r.ApproximateCreationDateTime
		if age <= streamExpirySeconds {
			continue
		}

		// Nil images to release heap memory.
		if r.OldImage != nil || r.NewImage != nil {
			r.OldImage = nil
			r.NewImage = nil
			cleared++
		}

		tombstones++
	}

	// Compact the ring buffer when more than half the slots are expired tombstones.
	if len(t.StreamRecords) > 0 && tombstones*2 >= len(t.StreamRecords) {
		t.compactExpiredStreamRecordsLocked(now)
	}

	return cleared
}

// compactExpiredStreamRecordsLocked drops only the stream records older than
// streamExpirySeconds and advances streamTrimSeq past them, preserving every
// record still within DynamoDB Streams' 24-hour retention window. A prior
// version discarded the entire ring (StreamRecords = nil, streamTrimSeq =
// streamSeq+1) here, which also destroyed records under 24h old and made
// them wrongly appear as trimmed to GetRecords/GetShardIterator callers.
// Must be called with table.mu held (write lock).
func (t *Table) compactExpiredStreamRecordsLocked(now int64) {
	tail, head := t.streamRecordsInOrder()
	ordered := make([]models.StreamRecord, 0, len(tail)+len(head))
	ordered = append(ordered, tail...)
	ordered = append(ordered, head...)

	kept := ordered[:0]
	trimSeq := t.streamTrimSeq

	for _, r := range ordered {
		if r.ApproximateCreationDateTime > 0 && now-r.ApproximateCreationDateTime > streamExpirySeconds {
			if seq, err := parseSeqNum(r.SequenceNumber); err == nil && seq+1 > trimSeq {
				trimSeq = seq + 1
			}

			continue
		}

		kept = append(kept, r)
	}

	t.StreamRecords = kept
	t.StreamHead = 0
	t.streamTrimSeq = trimSeq
}

// TTLGracePeriod is the extra time added after an item's TTL timestamp before it
// is actually evicted. AWS DynamoDB documents a 48-hour grace period in production.
// Tests should pass 0 to avoid timing dependencies.
var TTLGracePeriod = 0 * time.Second //nolint:gochecknoglobals // intentional package-level default

// isItemExpiredWithGrace reports whether an item's TTL attribute has expired,
// accounting for the configured grace period.
func isItemExpiredWithGrace(item map[string]any, ttlAttr string, gracePeriod time.Duration) bool {
	if ttlAttr == "" {
		return false
	}

	val, ok := item[ttlAttr]
	if !ok {
		return false
	}

	m, ok := val.(map[string]any)
	if !ok {
		return false
	}

	nStr, ok := m["N"].(string)
	if !ok {
		return false
	}

	var ts float64
	if _, err := fmt.Sscanf(nStr, "%f", &ts); err != nil {
		return false
	}

	expiry := time.Unix(int64(ts), 0).Add(gracePeriod)

	return time.Now().After(expiry)
}
