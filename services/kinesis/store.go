package kinesis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// contextWithRegion returns ctx with the given region attached under regionContextKey.
func contextWithRegion(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, regionContextKey{}, region)
}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

// regionFromARNOrCtx resolves the region for an ARN-addressed operation: it
// prefers the region embedded in the ARN (so the resource is found in the
// region it actually lives in), then the ctx region, then defaultRegion.
func regionFromARNOrCtx(ctx context.Context, a, defaultRegion string) string {
	if r := regionFromARN(a); r != "" {
		return r
	}

	return getRegion(ctx, defaultRegion)
}

// regionFromARN extracts the region segment from an AWS ARN.
// ARN format: arn:partition:service:region:account:resource. Returns "" when
// the input is not a well-formed ARN with a non-empty region segment.
func regionFromARN(a string) string {
	const (
		arnRegionIdx = 3
		arnMinParts  = 6
	)

	parts := strings.Split(a, ":")
	if len(parts) < arnMinParts {
		return ""
	}

	return parts[arnRegionIdx]
}

// streamNameFromARN extracts the stream name from a Kinesis stream ARN.
// Stream ARN format: arn:aws:kinesis:{region}:{account}:stream/{name}.
func streamNameFromARN(streamARN string) string {
	parts := strings.Split(streamARN, ":")
	const arnResourceIdx = 5
	if len(parts) <= arnResourceIdx {
		return ""
	}

	return strings.TrimPrefix(parts[arnResourceIdx], "stream/")
}

// ContextAndNameFromStreamARN parses a Kinesis stream ARN
// (arn:aws:kinesis:{region}:{account}:stream/{name}) and returns ctx with
// the ARN's region attached alongside the plain stream name, for callers
// outside this package that only hold the ARN -- cli.go's wireKinesisLambda
// (gopherstack-qowd), so Kinesis-to-Lambda event source mappings resolve the
// stream's actual region instead of always the account default.
func ContextAndNameFromStreamARN(ctx context.Context, streamARN string) (context.Context, string) {
	if region := regionFromARN(streamARN); region != "" {
		ctx = contextWithRegion(ctx, region)
	}

	return ctx, streamNameFromARN(streamARN)
}

// resolveStreamNameAndRegion resolves the target stream name and per-request
// region for an op accepting either StreamName or StreamARN. AWS documents,
// on every op in this family (AddTagsToStream/RemoveTagsFromStream/
// ListTagsForStream/IncreaseStreamRetentionPeriod/DecreaseStreamRetentionPeriod/
// ListShards/EnableEnhancedMonitoring/DisableEnhancedMonitoring/
// UpdateShardCount), "you must use either the StreamARN or the StreamName
// parameter, or both... It is recommended that you use the StreamARN input
// parameter" -- StreamARN is authoritative for both when supplied, mirroring
// the ARN-resolution pattern already used at the backend layer by
// MergeShards/SplitShard/StartStreamEncryption/UpdateStreamMode.
func resolveStreamNameAndRegion(
	ctx context.Context,
	streamName, streamARN, defaultRegion string,
) (string, context.Context) {
	if streamName == "" && streamARN != "" {
		streamName = streamNameFromARN(streamARN)
	}

	return streamName, contextWithRegion(ctx, regionFromARNOrCtx(ctx, streamARN, defaultRegion))
}

// kinesisThrottleFault holds the state of an active FIS throughput-exception fault on a stream.
type kinesisThrottleFault struct {
	expiry      time.Time
	probability float64 // fraction of calls to throttle (0.0–1.0); 1.0 = always throttle
}

// InMemoryBackend implements StorageBackend using in-memory maps.
//
// Streams are keyed by a composite "region/name" key (see streamKey) inside a
// single flat [store.Table], with a secondary [store.Index] grouping them by
// region — so same-named streams in different regions remain fully isolated,
// including their shards, records, and consumers, all of which stay inline
// fields on Stream (see pkgs/store's package doc: shards/records are the
// per-stream hot path and are not decomposed into their own tables).
// fisThroughputFaults and resourcePolicies are NOT store.Table candidates:
// their value types (a pointer with no self-describing identity, and a bare
// string) carry no key of their own to hand a Table keyFn, so they remain
// plain nested maps guarded the same way as before.
type InMemoryBackend struct {
	minimumThroughputBillingCommitment MinimumThroughputBillingCommitmentOutput
	kmsValidator                       KMSKeyValidator
	mu                                 *lockmetrics.RWMutex
	fisThroughputFaults                map[string]map[string]*kinesisThrottleFault
	faultsMu                           *lockmetrics.RWMutex
	resourcePolicies                   map[string]map[string]string
	streams                            *store.Table[Stream]
	OnStreamPurged                     func(string)
	registry                           *store.Registry
	streamsByRegion                    *store.Index[Stream]
	accountID                          string
	region                             string
	onDemandStreamCountLimit           int
}

// NewInMemoryBackend creates a new empty InMemoryBackend with default account/region.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(config.DefaultAccountID, config.DefaultRegion)
}

// NewInMemoryBackendWithConfig creates a new InMemoryBackend with the given account ID and region.
func NewInMemoryBackendWithConfig(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		fisThroughputFaults:      make(map[string]map[string]*kinesisThrottleFault),
		faultsMu:                 lockmetrics.New("kinesis.faults"),
		resourcePolicies:         make(map[string]map[string]string),
		accountID:                accountID,
		region:                   region,
		mu:                       lockmetrics.New("kinesis"),
		onDemandStreamCountLimit: defaultOnDemandStreamCountLimit,
		minimumThroughputBillingCommitment: MinimumThroughputBillingCommitmentOutput{
			Status: minimumThroughputBillingCommitmentDisabled,
		},
		registry: store.NewRegistry(),
	}
	b.streams = store.Register(b.registry, "streams", store.New(streamTableKeyFn))
	b.streamsByRegion = b.streams.AddIndex("region", func(v *Stream) string { return v.Region })

	return b
}

// Region returns the AWS region this backend is configured to use as its default.
func (b *InMemoryBackend) Region() string { return b.region }

// streamKey builds the composite primary key ("region/name") a Stream is
// stored under in b.streams, mirroring the region-nested map key it replaces.
func streamKey(region, name string) string {
	return region + "/" + name
}

// streamTableKeyFn is the [store.Table] key function for b.streams.
func streamTableKeyFn(v *Stream) string {
	return streamKey(v.Region, v.Name)
}

// faultsStore returns the FIS throttle-fault map for the given region, lazily
// creating it. Callers must hold b.faultsMu.
func (b *InMemoryBackend) faultsStore(region string) map[string]*kinesisThrottleFault {
	if b.fisThroughputFaults[region] == nil {
		b.fisThroughputFaults[region] = make(map[string]*kinesisThrottleFault)
	}

	return b.fisThroughputFaults[region]
}

// policiesStore returns the resource-policy map for the given region, lazily
// creating it. Callers must hold b.mu.
func (b *InMemoryBackend) policiesStore(region string) map[string]string {
	if b.resourcePolicies[region] == nil {
		b.resourcePolicies[region] = make(map[string]string)
	}

	return b.resourcePolicies[region]
}

func newStreamLock(streamName string) *lockmetrics.RWMutex {
	return lockmetrics.New(fmt.Sprintf("kinesis.stream.%s", streamName))
}

// ListAll returns a snapshot of all streams as StreamInfo values across every
// region. It is used by the dashboard, which presents a global inventory.
func (b *InMemoryBackend) ListAll(_ context.Context) []StreamInfo {
	b.mu.RLock("ListAll")
	defer b.mu.RUnlock()

	result := make([]StreamInfo, 0, b.streams.Len())
	for _, s := range b.streams.All() {
		s.mu.RLock("ListAll.stream")
		result = append(result, StreamInfo{
			Name:       s.Name,
			ARN:        s.ARN,
			Status:     s.Status,
			ShardCount: len(s.Shards),
		})
		s.mu.RUnlock()
	}

	return result
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, stream := range b.streams.All() {
		func() {
			stream.mu.Lock("Reset.stream")
			defer stream.mu.Unlock()

			if stream.Tags != nil {
				stream.Tags.Close()
			}
		}()
	}

	b.registry.ResetAll()
	b.faultsMu.Lock("Reset.faults")
	b.fisThroughputFaults = make(map[string]map[string]*kinesisThrottleFault)
	b.faultsMu.Unlock()
	b.resourcePolicies = make(map[string]map[string]string)
	b.minimumThroughputBillingCommitment = MinimumThroughputBillingCommitmentOutput{
		Status: minimumThroughputBillingCommitmentDisabled,
	}
}

// purgeStreamEntry removes s from the given region's stream map when it predates
// cutoff, or evicts its stale consumers otherwise. Must be called with b.mu held.
// Returns true when the stream was removed.
func (b *InMemoryBackend) purgeStreamEntry(region, name string, s *Stream, cutoff time.Time) bool {
	s.mu.Lock("Purge.stream")
	defer s.mu.Unlock()

	if s.CreatedAt.Before(cutoff) {
		if s.Tags != nil {
			s.Tags.Close()
		}
		b.streams.Delete(streamKey(region, name))
		b.faultsMu.Lock("Purge.faults")
		delete(b.faultsStore(region), name)
		b.faultsMu.Unlock()

		return true
	}

	// Stream is still live; evict stale consumers only.
	for cName, c := range s.Consumers {
		if c.ConsumerCreationTimestamp.Before(cutoff) {
			delete(s.Consumers, cName)
		}
	}

	return false
}

// fireStreamPurgedCallbacks invokes OnStreamPurged for every name, without holding any lock.
func (b *InMemoryBackend) fireStreamPurgedCallbacks(names []string) {
	if b.OnStreamPurged == nil {
		return
	}
	for _, name := range names {
		b.OnStreamPurged(name)
	}
}

// Purge removes all Kinesis streams and consumers created before the cutoff time.
func (b *InMemoryBackend) Purge(ctx context.Context, cutoff time.Time) {
	if ctx.Err() != nil {
		return
	}

	var purgedNames []string

	func() {
		b.mu.Lock("Purge")
		defer b.mu.Unlock()

		for _, s := range b.streams.All() {
			if ctx.Err() != nil {
				break
			}
			if b.purgeStreamEntry(s.Region, s.Name, s, cutoff) {
				purgedNames = append(purgedNames, s.Name)
			}
		}
	}()

	b.fireStreamPurgedCallbacks(purgedNames)
}

// AddStreamInternal seeds a stream directly into the backend for testing.
// Caller must provide a non-nil stream with at least Name and ARN set. The
// stream is placed in the region encoded in its ARN, falling back to the
// backend's default region when the ARN carries none.
func (b *InMemoryBackend) AddStreamInternal(stream *Stream) {
	b.mu.Lock("AddStreamInternal")
	defer b.mu.Unlock()

	initializeStreamRuntime(stream, stream.Name)

	region := regionFromARN(stream.ARN)
	if region == "" {
		region = b.region
	}
	stream.Region = region

	b.streams.Put(stream)
}
