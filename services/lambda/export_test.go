// This file exports internal types for use in external (_test) packages.
// It is compiled only during testing.

package lambda

import (
	"context"
	"maps"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
)

// ExportedRuntimeServer wraps the internal runtimeServer for white-box testing.
type ExportedRuntimeServer struct {
	inner *runtimeServer
}

// NewExportedRuntimeServer creates an ExportedRuntimeServer on the given port.
// The server is not started until Start is called.
func NewExportedRuntimeServer(port int) *ExportedRuntimeServer {
	return &ExportedRuntimeServer{
		inner: newRuntimeServer(port),
	}
}

// Start begins listening on the configured port.
func (e *ExportedRuntimeServer) Start(ctx context.Context) error {
	return e.inner.start(ctx)
}

// Stop shuts down the server.
func (e *ExportedRuntimeServer) Stop(ctx context.Context) {
	e.inner.stop(ctx)
}

// Invoke enqueues a payload and waits for the container response.
func (e *ExportedRuntimeServer) Invoke(
	ctx context.Context,
	payload []byte,
	clientContext string,
	timeout time.Duration,
) ([]byte, bool, string, error) {
	return e.inner.invoke(ctx, payload, clientContext, timeout)
}

// BaseImageForRuntime exports the internal runtimeBaseImages lookup for testing.
func BaseImageForRuntime(runtime string) string {
	return baseImageForRuntime(runtime)
}

// ExtractZip exports the internal extractZip function for testing.
func ExtractZip(zipData []byte) (string, error) { return extractZip(zipData) }

// StreamNameFromARN exports the internal streamNameFromARN function for testing.
func StreamNameFromARN(arn string) string { return streamNameFromARN(arn) }

// FunctionNameFromARN exports the internal functionNameFromARN function for testing.
func FunctionNameFromARN(arn string) string { return functionNameFromARN(arn) }

// FunctionNameAndQualifierFromARN exports the internal
// functionNameAndQualifierFromARN function for testing.
func FunctionNameAndQualifierFromARN(name string) (string, string) {
	return functionNameAndQualifierFromARN(name)
}

// PollOnce triggers a single poll cycle on the given EventSourcePoller.
func PollOnce(ctx context.Context, p *EventSourcePoller) { p.poll(ctx) }

// BuildURLEventPayload exports buildURLEventPayload for testing.
func BuildURLEventPayload(b *InMemoryBackend, r *http.Request) ([]byte, error) {
	return b.buildURLEventPayload(r)
}

// WriteFunctionURLResponse exports writeFunctionURLResponse for testing.
func WriteFunctionURLResponse(w http.ResponseWriter, result []byte) {
	writeFunctionURLResponse(w, result)
}

// SetDNSRegistrarExported exports SetDNSRegistrar for testing.
func SetDNSRegistrarExported(b *InMemoryBackend, dns DNSRegistrar) {
	b.SetDNSRegistrar(dns)
}

// ParseLayerARN exports parseLayerARN for testing.
func ParseLayerARN(layerVersionARN string) (string, int64) {
	return parseLayerARN(layerVersionARN)
}

// PrepareLayerMount exports prepareLayerMount for testing.
func PrepareLayerMount(b *InMemoryBackend, fn *FunctionConfiguration) (string, []string, error) {
	return b.prepareLayerMount(fn)
}

// IsSQSARN exports the internal isSQSARN function for testing.
func IsSQSARN(resourceARN string) bool { return isSQSARN(resourceARN) }

// IsDynamoDBStreamARN exports the internal isDynamoDBStreamARN function for testing.
func IsDynamoDBStreamARN(resourceARN string) bool { return isDynamoDBStreamARN(resourceARN) }

// SetSQSReaderOnPoller exports SetSQSReader on EventSourcePoller for testing.
func SetSQSReaderOnPoller(p *EventSourcePoller, r SQSReader) { p.SetSQSReader(r) }

// SetDynamoDBStreamsReaderOnPoller exports SetDynamoDBStreamsReader on EventSourcePoller for testing.
func SetDynamoDBStreamsReaderOnPoller(p *EventSourcePoller, r DynamoDBStreamsReader) {
	p.SetDynamoDBStreamsReader(r)
}

// SetSQSInvoker sets a test-only override for the Lambda invocation step in the
// SQS ESM poller. When fn is non-nil it is called instead of InvokeFunction.
// fn returns the raw response body (may be nil) and any error, mirroring
// InvokeFunction. Tests that need to simulate ReportBatchItemFailures can
// return a JSON body containing a batchItemFailures list.
func SetSQSInvoker(
	p *EventSourcePoller,
	fn func(ctx context.Context, fnName string) ([]byte, error),
) {
	p.sqsInvoker = fn
}

// SetDDBInvoker sets a test-only override for the Lambda invocation step in the
// DynamoDB Streams ESM poller. When fn is non-nil it is called instead of InvokeFunction.
func SetDDBInvoker(
	p *EventSourcePoller,
	fn func(ctx context.Context, fnName string, payload []byte) error,
) {
	p.ddbInvoker = fn
}

// InjectRuntimeEntry inserts a synthetic functionRuntime into the backend's runtimes map
// so that Close() tests can verify runtime cleanup without a real container.
// zipDir and layerDirs will be cleaned up by Close().
func InjectRuntimeEntry(
	b *InMemoryBackend,
	functionName, zipDir string,
	layerDirs []string,
	port int,
) {
	b.mu.Lock("InjectRuntimeEntry")
	defer b.mu.Unlock()

	b.runtimes[functionName] = &functionRuntime{
		mu:        lockmetrics.New("lambda.runtime.test"),
		zipDir:    zipDir,
		layerDirs: layerDirs,
		port:      port,
		started:   true,
		lastUsed:  time.Now(),
	}
}

// InjectRuntimeEntryWithContainer inserts a synthetic functionRuntime with a container ID
// into the backend's runtimes map for testing container-stop behavior.
func InjectRuntimeEntryWithContainer(
	b *InMemoryBackend,
	functionName, zipDir, containerID string,
	layerDirs []string,
	port int,
) {
	b.mu.Lock("InjectRuntimeEntryWithContainer")
	defer b.mu.Unlock()

	b.runtimes[functionName] = &functionRuntime{
		mu:          lockmetrics.New("lambda.runtime.test"),
		zipDir:      zipDir,
		layerDirs:   layerDirs,
		containerID: containerID,
		port:        port,
		started:     true,
		lastUsed:    time.Now(),
	}
}

// InjectRuntimeEntryWithSrv inserts a synthetic functionRuntime with an already-started
// runtime server so that tests can trigger real invocation timeouts via InvokeFunction.
func InjectRuntimeEntryWithSrv(
	b *InMemoryBackend,
	functionName string,
	srv *ExportedRuntimeServer,
) {
	b.mu.Lock("InjectRuntimeEntryWithSrv")
	defer b.mu.Unlock()

	b.runtimes[functionName] = &functionRuntime{
		mu:       lockmetrics.New("lambda.runtime.test"),
		srv:      srv.inner,
		started:  true,
		lastUsed: time.Now(),
	}
}

// InjectRuntimeEntryFull inserts a synthetic functionRuntime with a server, container ID,
// and optional temp directories. This is used in tests that need full runtime state.
func InjectRuntimeEntryFull(
	b *InMemoryBackend,
	functionName string,
	srv *ExportedRuntimeServer,
	containerID string,
) {
	b.mu.Lock("InjectRuntimeEntryFull")
	defer b.mu.Unlock()

	var innerSrv *runtimeServer
	if srv != nil {
		innerSrv = srv.inner
	}

	b.runtimes[functionName] = &functionRuntime{
		mu:          lockmetrics.New("lambda.runtime.test"),
		srv:         innerSrv,
		containerID: containerID,
		started:     true,
		lastUsed:    time.Now(),
	}
}

// RuntimesLen returns the number of entries in the backend's runtimes map.
func RuntimesLen(b *InMemoryBackend) int {
	b.mu.RLock("RuntimesLen")
	defer b.mu.RUnlock()

	return len(b.runtimes)
}

// RuntimeContainerID returns the containerID stored in the named runtime entry, or "" if not found.
func RuntimeContainerID(b *InMemoryBackend, functionName string) string {
	b.mu.RLock("RuntimeContainerID")
	defer b.mu.RUnlock()

	if rt, ok := b.runtimes[functionName]; ok {
		return rt.containerID
	}

	return ""
}

// SetRuntimeLastUsed sets the lastUsed timestamp on a runtime entry.
// It is used by LRU eviction tests to control which runtime is evicted.
func SetRuntimeLastUsed(b *InMemoryBackend, functionName string, t time.Time) {
	b.mu.Lock("SetRuntimeLastUsed")
	defer b.mu.Unlock()

	if rt, ok := b.runtimes[functionName]; ok {
		rt.lastUsed = t
	}
}

// DefaultMaxRuntimes exposes the defaultMaxRuntimes constant for tests.
const DefaultMaxRuntimes = defaultMaxRuntimes

// FunctionNamesFromARNs exports functionNamesFromARNs for testing.
func FunctionNamesFromARNs(arns []string) []string { return functionNamesFromARNs(arns) }

// ParseInvocationPercentage exports parseInvocationPercentage for testing.
func ParseInvocationPercentage(s string) float64 { return parseInvocationPercentage(s) }

// ParseInvocationDelayMs exports parseInvocationDelayMs for testing.
func ParseInvocationDelayMs(s string) int { return parseInvocationDelayMs(s) }

// ParseIntSafe exports parseIntSafe for testing.
func ParseIntSafe(s string, out *int) error { return parseIntSafe(s, out) }

// ExpiryFromDuration exports expiryFromDuration for testing.
func ExpiryFromDuration(d time.Duration) time.Time { return expiryFromDuration(d) }

// SetFISFault exports setFISFault for testing.
func SetFISFault(b *InMemoryBackend, name string, fault *FISInvocationFault) {
	b.setFISFault(name, fault)
}

// ClearFISFault exports clearFISFault for testing.
func ClearFISFault(b *InMemoryBackend, name string) { b.clearFISFault(name) }

// CheckFISFault exports checkFISFault for testing.
func CheckFISFault(b *InMemoryBackend, name string) *FISInvocationFault {
	return b.checkFISFault(name)
}

// ReleaseConcurrencySlot exports releaseConcurrencySlot for testing.
func ReleaseConcurrencySlot(b *InMemoryBackend, functionName string) {
	b.releaseConcurrencySlot(functionName)
}

// AcquireConcurrencySlot exports acquireConcurrencySlot for testing.
func AcquireConcurrencySlot(b *InMemoryBackend, functionName string) (bool, error) {
	return b.acquireConcurrencySlot(functionName)
}

// MinEventAgeInSeconds exports minEventAgeInSeconds for use in tests.
const MinEventAgeInSeconds = minEventAgeInSeconds

func ShardIteratorsLen(p *EventSourcePoller) int {
	p.mu.RLock("ShardIteratorsLen")
	defer p.mu.RUnlock()

	return len(p.shardIterators)
}

// RuntimeQueueSize exposes the internal runtimeQueueSize constant for test use.
const RuntimeQueueSize = runtimeQueueSize

// MaxAsyncEnqueueWaiters exposes maxAsyncEnqueueWaiters for test use.
const MaxAsyncEnqueueWaiters = maxAsyncEnqueueWaiters

// PendingLen returns the number of entries in the runtime server's pending invocations map.
// Intended for use in unit tests to verify stale-pending cleanup.
func PendingLen(s *ExportedRuntimeServer) int {
	count := 0
	s.inner.pending.Range(func(_, _ any) bool {
		count++

		return true
	})

	return count
}

// QueueLen returns the current number of items in the runtime server's invocation queue.
func QueueLen(s *ExportedRuntimeServer) int {
	return len(s.inner.queue)
}

// AsyncEnqueueWaitersLen returns the number of currently active slow-path async enqueue waiters.
func AsyncEnqueueWaitersLen(b *InMemoryBackend) int {
	return len(b.asyncEnqueueWaiters)
}

// FillQueue fills the runtime server's queue with dummy placeholder entries up to n items.
// Returns the actual number added.
func FillQueue(s *ExportedRuntimeServer, n int) int {
	added := 0

	for range n {
		select {
		case s.inner.queue <- &pendingInvocation{}:
			added++
		default:
			return added
		}
	}

	return added
}

// DrainQueue removes all items currently in the queue and returns the count drained.
func DrainQueue(s *ExportedRuntimeServer) int {
	n := 0

	for {
		select {
		case <-s.inner.queue:
			n++
		default:
			return n
		}
	}
}

// DrainN removes at most n items from the runtime server's invocation queue,
// returning the actual number removed. Unlike DrainQueue, it stops after n
// dequeues so that items inserted concurrently by background goroutines (e.g.
// the slow-path in enqueueAsyncInvocation) are not inadvertently consumed.
func DrainN(s *ExportedRuntimeServer, n int) int {
	removed := 0

	for removed < n {
		select {
		case <-s.inner.queue:
			removed++
		default:
			return removed
		}
	}

	return removed
}

// EnqueueAsync calls enqueueAsyncInvocation on b with a synthetic pendingInvocation.
// It returns the requestID of the created invocation so tests can simulate container
// responses or verify pending-map cleanup. createdAt sets the event creation time; pass
// a zero [time.Time] to default to [time.Now].
func EnqueueAsync(
	ctx context.Context,
	b *InMemoryBackend,
	srv *ExportedRuntimeServer,
	functionName string,
	payload []byte,
	timeout time.Duration,
	trackConcurrency bool,
	createdAt ...time.Time,
) string {
	eventTime := time.Now()
	if len(createdAt) > 0 && !createdAt[0].IsZero() {
		eventTime = createdAt[0]
	}

	inv := &pendingInvocation{
		requestID: uuid.New().String(),
		payload:   payload,
		deadline:  eventTime.Add(timeout),
		createdAt: eventTime,
		result:    make(chan invocationResult, 1),
	}

	b.enqueueAsyncInvocation(ctx, srv.inner, functionName, inv, timeout, trackConcurrency)

	return inv.requestID
}

// CleanupSemLen returns the number of cleanup goroutines currently holding a slot in cleanupSem.
func CleanupSemLen(b *InMemoryBackend) int { return len(b.cleanupSem) }

// CleanupTimedOutRuntimeForTest exports cleanupTimedOutRuntime so tests can drive
// its cleanupSem-saturated fallback path directly and deterministically.
func CleanupTimedOutRuntimeForTest(b *InMemoryBackend, functionName string) {
	b.cleanupTimedOutRuntime(functionName)
}

// FillCleanupSem saturates b.cleanupSem to capacity and returns a function that
// drains it back to empty. Used to test the "semaphore full" fallback path of the
// various runtime-cleanup call sites.
func FillCleanupSem(b *InMemoryBackend) func() {
	n := cap(b.cleanupSem)
	for range n {
		b.cleanupSem <- struct{}{}
	}

	return func() {
		for range n {
			<-b.cleanupSem
		}
	}
}

// PollerNotifyC returns the notify channel of an EventSourcePoller for testing.
func PollerNotifyC(p *EventSourcePoller) chan struct{} { return p.notifyC }

// PushInvocationLog exports pushInvocationLog for testing.
func PushInvocationLog(
	ctx context.Context,
	b *InMemoryBackend,
	functionName string,
	payload, result []byte,
) {
	b.pushInvocationLog(ctx, functionName, payload, result)
}

// EventFilterMatches exports eventFilterMatches for testing FilterCriteria evaluation.
func EventFilterMatches(fc *FilterCriteria, record map[string]any) bool {
	return eventFilterMatches(fc, record)
}

// SQSFilterView exports sqsFilterView for testing.
func SQSFilterView(msg *SQSMessage) map[string]any { return sqsFilterView(msg) }

// ClassifyFunctionError exports classifyFunctionError for testing X-Amz-Function-Error.
func ClassifyFunctionError(isError bool, result []byte) string {
	return classifyFunctionError(isError, result)
}

// SQSRegionFromARN exports sqsRegionFromARN for testing.
func SQSRegionFromARN(arn string) string { return sqsRegionFromARN(arn) }

// SplitByRecordAge exports splitByRecordAge for testing MaximumRecordAge enforcement.
func SplitByRecordAge(msgs []*SQSMessage, maxAgeSecs int) ([]*SQSMessage, []*SQSMessage) {
	return splitByRecordAge(msgs, maxAgeSecs)
}

// SplitByFilter exports splitByFilter for testing FilterCriteria enforcement.
func SplitByFilter(fc *FilterCriteria, msgs []*SQSMessage) ([]*SQSMessage, []*SQSMessage) {
	return splitByFilter(fc, msgs)
}

// BuildSQSEventPayload exports buildSQSEventPayload for testing the event record shape.
func BuildSQSEventPayload(region, esmARN string, msgs []*SQSMessage) ([]byte, error) {
	return buildSQSEventPayload(region, esmARN, msgs)
}

// AccumulateSQSBatch exports the poller's batching-window accumulation for testing.
func AccumulateSQSBatch(p *EventSourcePoller, m *EventSourceMapping, msgs []*SQSMessage) ([]*SQSMessage, bool) {
	return p.accumulateSQSBatch(m, msgs)
}

// BuildFunctionURLHandler exports buildFunctionURLHandler for testing auth/CORS.
func BuildFunctionURLHandler(b *InMemoryBackend, functionName string) http.HandlerFunc {
	return b.buildFunctionURLHandler(functionName)
}

// SetFunctionURLConfigForTest inserts a function URL config directly for testing.
// The store.Table backing b.functionURLConfigs derives its key from
// cfg.FunctionArn (see functionURLConfigsKeyFn in store_setup.go), so callers
// that omit FunctionArn get it defaulted here to match functionName -- exactly
// what CreateFunctionURLConfig itself would have set via buildURLARN.
func SetFunctionURLConfigForTest(b *InMemoryBackend, functionName string, cfg *FunctionURLConfig) {
	b.mu.Lock("SetFunctionURLConfigForTest")
	defer b.mu.Unlock()

	if cfg.FunctionArn == "" {
		cfg.FunctionArn = buildURLARN(b.region, b.accountID, functionName)
	}

	b.functionURLConfigs.Put(cfg)
}

// AsyncOutcomeForTest describes a completed async invocation for delivery testing.
type AsyncOutcomeForTest struct {
	FunctionName    string
	RequestID       string
	FunctionError   string
	RequestPayload  []byte
	ResponsePayload []byte
	InvokeCount     int
	StatusCode      int
	Success         bool
}

// DispatchAsyncOutcomeForTest exports dispatchAsyncOutcome for testing DLQ/destination delivery.
func DispatchAsyncOutcomeForTest(ctx context.Context, b *InMemoryBackend, o AsyncOutcomeForTest) {
	b.dispatchAsyncOutcome(ctx, asyncOutcome{
		functionName:    o.FunctionName,
		requestID:       o.RequestID,
		requestPayload:  o.RequestPayload,
		responsePayload: o.ResponsePayload,
		functionError:   o.FunctionError,
		invokeCount:     o.InvokeCount,
		statusCode:      o.StatusCode,
		success:         o.Success,
	})
}

// GetFunctionStateForTest returns the stored State for a function.
func GetFunctionStateForTest(b *InMemoryBackend, name string) FunctionState {
	b.mu.RLock("GetFunctionStateForTest")
	defer b.mu.RUnlock()

	if fn, ok := b.functions.Get(name); ok {
		return fn.State
	}

	return ""
}

// WithInvocationChainForTest injects a function name into the context's invocation chain.
// This allows tests to simulate recursive self-invocations without a real container.
func WithInvocationChainForTest(ctx context.Context, functionName string) context.Context {
	return withInvocationChain(ctx, functionName)
}

// NewPollerWithCancelSignal creates an EventSourcePoller that closes doneCh when its
// context is cancelled. Used by Close() lifecycle tests.
func NewPollerWithCancelSignal(doneCh chan struct{}) *EventSourcePoller {
	p := &EventSourcePoller{
		mu:           lockmetrics.New("lambda.poller.test"),
		notifyC:      make(chan struct{}, 1),
		cancelSignal: doneCh,
	}

	return p
}

// ActiveConcurrenciesSnapshot returns a copy of the activeConcurrencies map for testing.
func ActiveConcurrenciesSnapshot(b *InMemoryBackend) map[string]int {
	b.mu.RLock("ActiveConcurrenciesSnapshot")
	defer b.mu.RUnlock()

	result := make(map[string]int, len(b.activeConcurrencies))
	maps.Copy(result, b.activeConcurrencies)

	return result
}

// CleanupSemCap returns the capacity of the cleanupSem channel.
// Used to verify Reset() replaces it with a fresh channel of the correct capacity.
func CleanupSemCap(b *InMemoryBackend) int { return cap(b.cleanupSem) }

// LogSemCap returns the capacity of the logSem channel.
func LogSemCap(b *InMemoryBackend) int { return cap(b.logSem) }

// SweepESMsForTest calls sweepESMs on the janitor for white-box testing.
func SweepESMsForTest(ctx context.Context, j *Janitor) { j.sweepESMs(ctx) }

// InvocationChainContainsForTest reports whether functionName is in the context's invocation chain.
func InvocationChainContainsForTest(ctx context.Context, functionName string) bool {
	return invocationChainContains(ctx, functionName)
}

// InvocationChainLenForTest returns the length of the invocation chain stored in ctx.
func InvocationChainLenForTest(ctx context.Context) int {
	chain, _ := ctx.Value(invocationChainKeyType{}).([]string)

	return len(chain)
}

// WithInvocationChainBatchForTest builds an invocation chain by appending all names in one
// context value, avoiding a context-in-loop pattern. Intended for table-driven tests only.
func WithInvocationChainBatchForTest(ctx context.Context, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	existing, _ := ctx.Value(invocationChainKeyType{}).([]string)
	next := make([]string, len(existing)+len(names))
	copy(next, existing)
	copy(next[len(existing):], names)

	return context.WithValue(ctx, invocationChainKeyType{}, next)
}
