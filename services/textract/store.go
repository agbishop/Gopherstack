package textract

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

// regionFromARN extracts the region component (index 3) from an AWS ARN
// (arn:partition:service:region:account:resource), falling back to defaultRegion.
func regionFromARN(resourceARN, defaultRegion string) string {
	const (
		regionIndex  = 3
		arnMinFields = regionIndex + 2
	)

	parts := strings.SplitN(resourceARN, ":", arnMinFields)
	if len(parts) > regionIndex && parts[regionIndex] != "" {
		return parts[regionIndex]
	}

	return defaultRegion
}

// maxJobHistory is the maximum number of completed jobs retained in memory.
const maxJobHistory = 10000

// MaxJobHistory is the exported value for testing.
const MaxJobHistory = maxJobHistory

// defaultAsyncJobDelay is the default time before a new async job transitions from IN_PROGRESS to SUCCEEDED.
const defaultAsyncJobDelay = 200 * time.Millisecond

// InMemoryBackend is the in-memory store for Textract jobs.
//
// jobs/expenseJobs/lendingJobs/adapters/adapterVersions were formerly nested
// by region (outer key = region) so that same-named resources in different
// regions are fully isolated; each is now a store.Table[T] keyed by a
// region+id composite (see regionKey in store_setup.go) providing the same
// isolation. clientTokenToJobID/adapterClientTokenToID remain plain
// region-nested maps: their values are strings, not *T, so they do not fit
// store.Table's keyed-by-identity-value shape.
type InMemoryBackend struct {
	svcCtx                    context.Context
	s3                        S3Backend
	adapterClientTokenToID    map[string]map[string]string // region → clientToken → adapterID
	clientTokenToJobID        map[string]map[string]string // region → clientToken → jobID
	expenseClientTokenToJobID map[string]map[string]string // region → clientToken → expense jobID
	lendingClientTokenToJobID map[string]map[string]string // region → clientToken → lending jobID
	jobs                      *store.Table[DocumentJob]
	jobsByRegion              *store.Index[DocumentJob]
	expenseJobs               *store.Table[ExpenseJob]
	expenseJobsByRegion       *store.Index[ExpenseJob]
	lendingJobs               *store.Table[LendingJob]
	lendingJobsByRegion       *store.Index[LendingJob]
	adapters                  *store.Table[Adapter]
	adaptersByRegion          *store.Index[Adapter]
	adapterVersions           *store.Table[AdapterVersion]
	adapterVersionsByAdapter  *store.Index[AdapterVersion]
	mu                        *lockmetrics.RWMutex
	cancel                    context.CancelFunc
	accountID                 string
	region                    string // default region
	wg                        sync.WaitGroup
	asyncJobDelay             time.Duration
	maxJobs                   int
}

// NewInMemoryBackend creates a new InMemoryBackend with a background lifecycle
// context. Prefer [NewInMemoryBackendWithContext] when a service context is
// available so delayed job completions are cancelled on shutdown.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	return NewInMemoryBackendWithContext(context.Background(), accountID, region)
}

// NewInMemoryBackendWithContext creates a new InMemoryBackend whose delayed
// job-completion goroutines are tied to svcCtx, so they are cancelled when the
// service shuts down. If svcCtx is nil, [context.Background] is used.
func NewInMemoryBackendWithContext(svcCtx context.Context, accountID, region string) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	ctx, cancel := context.WithCancel(svcCtx)

	b := &InMemoryBackend{
		clientTokenToJobID:        make(map[string]map[string]string),
		adapterClientTokenToID:    make(map[string]map[string]string),
		expenseClientTokenToJobID: make(map[string]map[string]string),
		lendingClientTokenToJobID: make(map[string]map[string]string),
		mu:                        lockmetrics.New("textract"),
		accountID:                 accountID,
		region:                    region,
		maxJobs:                   maxJobHistory,
		asyncJobDelay:             defaultAsyncJobDelay,
		svcCtx:                    ctx,
		cancel:                    cancel,
	}

	registerAllTables(b)

	return b
}

// SetS3Backend wires S3 so a Document/DocumentLocation's S3Object is
// validated against real S3 state instead of only being stored/echoed.
func (b *InMemoryBackend) SetS3Backend(s3 S3Backend) {
	b.s3 = s3
}

// runDelayed runs fn after delay, unless the backend's lifecycle context is
// cancelled first. The goroutine is tracked by b.wg so [InMemoryBackend.Shutdown]
// can wait for it. A zero delay fires fn promptly (time.After(0) is immediate).
func (b *InMemoryBackend) runDelayed(delay time.Duration, fn func()) {
	b.wg.Go(func() {
		select {
		case <-b.svcCtx.Done():
			return
		case <-time.After(delay):
		}

		fn()
	})
}

// Shutdown cancels in-flight delayed job completions and waits for their
// goroutines to exit, bounded by ctx. It implements the service shutdown
// contract used by the handler.
func (b *InMemoryBackend) Shutdown(ctx context.Context) {
	if b.cancel != nil {
		b.cancel()
	}

	done := make(chan struct{})

	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// AccountID returns the AWS account ID for this backend.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the AWS region for this backend.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all stored resources, resetting the backend to its initial state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.resetTablesLocked()
	b.resetClientTokenMapsLocked()
}

// resetClientTokenMapsLocked reinitializes every ClientRequestToken dedup
// map to empty. Caller must hold b.mu for writing.
func (b *InMemoryBackend) resetClientTokenMapsLocked() {
	b.clientTokenToJobID = make(map[string]map[string]string)
	b.adapterClientTokenToID = make(map[string]map[string]string)
	b.expenseClientTokenToJobID = make(map[string]map[string]string)
	b.lendingClientTokenToJobID = make(map[string]map[string]string)
}

// The following lazy per-region store helpers return the resource map for the
// given region, creating it on first use. Callers must hold b.mu.

func (b *InMemoryBackend) clientTokenToJobIDStore(region string) map[string]string {
	if b.clientTokenToJobID[region] == nil {
		b.clientTokenToJobID[region] = make(map[string]string)
	}

	return b.clientTokenToJobID[region]
}

func (b *InMemoryBackend) adapterClientTokenToIDStore(region string) map[string]string {
	if b.adapterClientTokenToID[region] == nil {
		b.adapterClientTokenToID[region] = make(map[string]string)
	}

	return b.adapterClientTokenToID[region]
}

func (b *InMemoryBackend) expenseClientTokenToJobIDStore(region string) map[string]string {
	if b.expenseClientTokenToJobID[region] == nil {
		b.expenseClientTokenToJobID[region] = make(map[string]string)
	}

	return b.expenseClientTokenToJobID[region]
}

func (b *InMemoryBackend) lendingClientTokenToJobIDStore(region string) map[string]string {
	if b.lendingClientTokenToJobID[region] == nil {
		b.lendingClientTokenToJobID[region] = make(map[string]string)
	}

	return b.lendingClientTokenToJobID[region]
}
