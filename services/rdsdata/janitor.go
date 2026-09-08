package rdsdata

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	defaultRDSDataJanitorInterval = time.Minute

	// defaultRDSDataTransactionIdleTimeout mirrors BeginTransaction's doc
	// comment (rdsdata@v1.35.4 api_op_BeginTransaction.go): a transaction
	// times out if no calls use its transaction ID in three minutes, and is
	// rolled back automatically.
	defaultRDSDataTransactionIdleTimeout = 3 * time.Minute

	// defaultRDSDataTransactionMaxLifetime mirrors the same doc comment: a
	// transaction can run for a maximum of 24 hours before it's terminated
	// and rolled back automatically.
	defaultRDSDataTransactionMaxLifetime = 24 * time.Hour

	rdsdataWorkerServiceName = "rdsdata"
	txReaperComponent        = "TransactionReaper"
)

// Janitor is the RDS Data background worker that rolls back and evicts
// transactions real AWS would itself have expired by now: idle past
// IdleTimeout, or open past MaxLifetime (see the constants above). Without
// this, a caller that begins a transaction and never commits or rolls it
// back leaks it forever -- both the backend's Transaction record and the
// engine's open *sql.Tx (engine.go's sqlEngine.txs).
type Janitor struct {
	Backend     *InMemoryBackend
	Interval    time.Duration
	IdleTimeout time.Duration
	MaxLifetime time.Duration
	// TaskTimeout bounds each individual sweep. When non-zero, each sweep
	// runs with a child context that expires after this duration, preventing
	// a stalled operation from blocking the janitor loop indefinitely.
	TaskTimeout time.Duration
}

// NewJanitor creates a new RDS Data Janitor for the given backend. Zero
// values for interval, idleTimeout, or maxLifetime fall back to the
// AWS-documented defaults above.
func NewJanitor(backend *InMemoryBackend, interval, idleTimeout, maxLifetime time.Duration) *Janitor {
	if interval == 0 {
		interval = defaultRDSDataJanitorInterval
	}

	if idleTimeout == 0 {
		idleTimeout = defaultRDSDataTransactionIdleTimeout
	}

	if maxLifetime == 0 {
		maxLifetime = defaultRDSDataTransactionMaxLifetime
	}

	return &Janitor{
		Backend:     backend,
		Interval:    interval,
		IdleTimeout: idleTimeout,
		MaxLifetime: maxLifetime,
	}
}

// Run runs the janitor loop until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, rdsdataWorkerServiceName)
	g.Ticker(txReaperComponent, j.Interval, j.TaskTimeout, j.tick)

	<-ctx.Done()
	g.Stop()
}

// SweepOnce runs a single janitor pass. Exposed for testing.
func (j *Janitor) SweepOnce(ctx context.Context) {
	j.tick(ctx)
}

// expiredTxKey identifies one transaction to reap, by the region its id is
// scoped to (transaction ids are only unique within a region -- see
// store_setup.go).
type expiredTxKey struct {
	region string
	id     string
}

// tick sweeps every region's transactions for ones idle past IdleTimeout or
// open past MaxLifetime, rolling each back on the engine and removing its
// Transaction record.
func (j *Janitor) tick(ctx context.Context) {
	now := time.Now()

	var expired []expiredTxKey

	func() {
		j.Backend.mu.Lock("RDSDataJanitor")
		defer j.Backend.mu.Unlock()

		for region, tbl := range j.Backend.transactions {
			for _, tx := range tbl.All() {
				if now.Sub(tx.CreatedAt) > j.MaxLifetime || now.Sub(tx.LastActivityAt) > j.IdleTimeout {
					expired = append(expired, expiredTxKey{region: region, id: tx.TransactionID})
				}
			}
		}

		for _, e := range expired {
			j.Backend.transactionsStore(e.region).Delete(e.id)
			j.Backend.engine.finalizeTx(e.id, false)
		}
	}()

	count := len(expired)

	telemetry.RecordWorkerTask(rdsdataWorkerServiceName, txReaperComponent, "success")

	if count == 0 {
		return
	}

	telemetry.RecordWorkerItems(rdsdataWorkerServiceName, txReaperComponent, count)

	logger.Load(ctx).InfoContext(ctx, "rdsdata janitor: expired transactions rolled back", "count", count)
}
