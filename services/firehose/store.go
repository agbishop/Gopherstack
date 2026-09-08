package firehose

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key for the per-request AWS region.
type regionContextKey struct{}

// InMemoryBackend is the in-memory store for Firehose resources.
type InMemoryBackend struct {
	s3             S3Storer
	lambda         LambdaInvoker
	kinesisBackend KinesisReader
	redshiftData   RedshiftDataExecutor
	cwLogs         CWLogsBackend
	registry       *store.Registry
	// streams is a single flat table of every delivery stream, composite-keyed by
	// "region|name" (see regionKey/deliveryStreamKeyFn in store_setup.go) so that
	// same-named streams stay isolated across regions exactly as the old
	// map[region]map[name]*DeliveryStream nesting did.
	streams *store.Table[DeliveryStream]
	// streamsByRegion groups streams entries have registered exactly like the old
	// per-region outer map key did, so a region-scoped "list everything" (what
	// ListDeliveryStreamsByType and FlushAll need) stays an O(region size) index
	// lookup instead of an O(all regions) scan.
	streamsByRegion *store.Index[DeliveryStream]
	// pollerCancel maps region → stream name → cancel func for active Kinesis source pollers.
	pollerCancel map[string]map[string]context.CancelFunc
	// sortedNamesCache caches the alphabetically sorted stream names per region so
	// ListDeliveryStreams does not re-sort on every call. Invalidated on create/delete.
	sortedNamesCache map[string][]string
	// pendingFlush tracks the set of streams (region → name) that hold buffered records
	// eligible for an interval-based flush, so intervalFlusher only inspects streams that
	// could actually need flushing rather than scanning every stream each tick.
	pendingFlush map[string]map[string]struct{}
	mu           *lockmetrics.RWMutex
	// svcCtx is the service lifecycle context; delivery operations use it so
	// they are cancelled when the server shuts down rather than blocking indefinitely.
	svcCtx    context.Context
	accountID string
	region    string
}

var _ StorageBackend = (*InMemoryBackend)(nil)

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	return NewInMemoryBackendWithContext(context.Background(), accountID, region)
}

// NewInMemoryBackendWithContext creates a new InMemoryBackend whose delivery
// operations are bounded by the provided parent context. Use this in production
// to ensure in-flight deliveries are cancelled on server shutdown.
// If svcCtx is nil, [context.Background] is used.
func NewInMemoryBackendWithContext(svcCtx context.Context, accountID, region string) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	b := &InMemoryBackend{
		registry:         store.NewRegistry(),
		pollerCancel:     make(map[string]map[string]context.CancelFunc),
		sortedNamesCache: make(map[string][]string),
		pendingFlush:     make(map[string]map[string]struct{}),
		accountID:        accountID,
		region:           region,
		mu:               lockmetrics.New("firehose"),
		svcCtx:           svcCtx,
	}
	registerAllTables(b)

	return b
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// getRegionFromContext extracts the per-request AWS region from ctx, falling
// back to the backend's configured region when none is present.
func getRegionFromContext(ctx context.Context, b *InMemoryBackend) string {
	if region, ok := ctx.Value(regionContextKey{}).(string); ok && region != "" {
		return region
	}

	return b.region
}

// pollerStore returns the poller-cancel map for region, lazily creating it.
// Must be called with the lock held.
func (b *InMemoryBackend) pollerStore(region string) map[string]context.CancelFunc {
	if b.pollerCancel[region] == nil {
		b.pollerCancel[region] = make(map[string]context.CancelFunc)
	}

	return b.pollerCancel[region]
}

// invalidateNamesCacheLocked drops the cached sorted-name slice for region so the next
// ListDeliveryStreams rebuilds it. Must be called with the write lock held.
func (b *InMemoryBackend) invalidateNamesCacheLocked(region string) {
	delete(b.sortedNamesCache, region)
}

// markPendingFlushLocked records that region/name holds buffered records that may need an
// interval flush. Must be called with the write lock held.
func (b *InMemoryBackend) markPendingFlushLocked(region, name string) {
	if b.pendingFlush[region] == nil {
		b.pendingFlush[region] = make(map[string]struct{})
	}
	b.pendingFlush[region][name] = struct{}{}
}

// clearPendingFlushLocked removes region/name from the interval-flush watch set. Must be
// called with the write lock held.
func (b *InMemoryBackend) clearPendingFlushLocked(region, name string) {
	if set := b.pendingFlush[region]; set != nil {
		delete(set, name)
		if len(set) == 0 {
			delete(b.pendingFlush, region)
		}
	}
}

// SetS3Backend wires the S3 backend for actual record delivery.
func (b *InMemoryBackend) SetS3Backend(s3 S3Storer) {
	b.s3 = s3
}

// SetLambdaBackend wires the Lambda backend for record transformation.
func (b *InMemoryBackend) SetLambdaBackend(lambda LambdaInvoker) {
	b.lambda = lambda
}

// SetKinesisBackend wires the Kinesis backend for polling KinesisStreamAsSource streams.
func (b *InMemoryBackend) SetKinesisBackend(k KinesisReader) {
	b.kinesisBackend = k
}

// SetRedshiftDataBackend wires the Redshift Data API executor used to issue the COPY
// command after S3 staging for Redshift destinations.
func (b *InMemoryBackend) SetRedshiftDataBackend(rd RedshiftDataExecutor) {
	b.redshiftData = rd
}

// SetCWLogsBackend wires CloudWatch Logs so destinations with CloudWatchLoggingOptions
// enabled actually deliver their error-log events, instead of only logging locally.
func (b *InMemoryBackend) SetCWLogsBackend(cw CWLogsBackend) {
	b.cwLogs = cw
}

// Reset clears all delivery streams, closing their tag registries and cancelling any
// running Kinesis source pollers to prevent leaks.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, s := range b.streams.All() {
		if s.Tags != nil {
			s.Tags.Close()
		}
	}

	for region, pollers := range b.pollerCancel {
		for name, cancel := range pollers {
			cancel()
			delete(pollers, name)
		}

		delete(b.pollerCancel, region)
	}

	b.pendingFlush = make(map[string]map[string]struct{})
	b.sortedNamesCache = make(map[string][]string)

	b.registry.ResetAll()
}

// AddStreamInternal deep-copies s into the backend, used for seeding test data.
func (b *InMemoryBackend) AddStreamInternal(s *DeliveryStream) {
	b.mu.Lock("AddStreamInternal")
	defer b.mu.Unlock()

	cp := streamCopy(s)
	if cp.Region == "" {
		// A store.Table key is a pure function of the value (see store_setup.go's
		// deliveryStreamKeyFn), so -- unlike the old map[region]map[name]*DeliveryStream,
		// where a caller-omitted Region only affected which outer-map bucket the entry
		// landed in -- cp.Region itself must be defaulted here for the entry to be
		// keyed (and therefore later found by DescribeDeliveryStream et al.) under the
		// backend's own region.
		cp.Region = b.region
	}
	b.streams.Put(cp)
}

// streamCopy returns a shallow copy of s with pointer fields independently copied.
func streamCopy(s *DeliveryStream) *DeliveryStream {
	cp := *s
	if s.S3Destination != nil {
		dest := *s.S3Destination
		cp.S3Destination = &dest
	}

	if s.HTTPEndpointDestination != nil {
		ep := *s.HTTPEndpointDestination
		cp.HTTPEndpointDestination = &ep
	}

	if s.RedshiftDestination != nil {
		rs := *s.RedshiftDestination
		cp.RedshiftDestination = &rs
	}

	if s.OpenSearchDestination != nil {
		os := *s.OpenSearchDestination
		cp.OpenSearchDestination = &os
	}

	if s.ElasticsearchDestination != nil {
		es := *s.ElasticsearchDestination
		cp.ElasticsearchDestination = &es
	}

	if s.SplunkDestination != nil {
		sp := *s.SplunkDestination
		cp.SplunkDestination = &sp
	}

	if s.IcebergDestination != nil {
		ic := *s.IcebergDestination
		cp.IcebergDestination = &ic
	}

	if s.SnowflakeDestination != nil {
		sf := *s.SnowflakeDestination
		cp.SnowflakeDestination = &sf
	}

	if s.Encryption != nil {
		enc := *s.Encryption
		cp.Encryption = &enc
	}

	if s.Source != nil {
		src := *s.Source
		cp.Source = &src
	}

	cp.Records = nil
	cp.BackupRecords = nil

	return &cp
}
