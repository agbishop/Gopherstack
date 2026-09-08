package lambda

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// maxConcurrentInvocationLogs bounds the number of in-flight async log delivery
// goroutines spawned by dispatchInvocationLog. When saturated, additional log
// emissions are dropped (with a warn log) rather than queued, preventing
// unbounded goroutine growth under high invocation throughput when the
// CloudWatch Logs backend is slow or unavailable.
const maxConcurrentInvocationLogs = 256

// invocationChainKeyType is the context key type used to track the current Lambda invocation chain.
// Its value is a []string of function names currently in the call stack.
type invocationChainKeyType struct{}

// withInvocationChain returns a context carrying the updated invocation chain.
// Uses a []string instead of a map to avoid per-call heap allocation on the hot invocation path.
// make+copy ensures the new slice never shares backing array with existing.
func withInvocationChain(ctx context.Context, functionName string) context.Context {
	existing, _ := ctx.Value(invocationChainKeyType{}).([]string)
	next := make([]string, len(existing)+1)
	copy(next, existing)
	next[len(existing)] = functionName

	return context.WithValue(ctx, invocationChainKeyType{}, next)
}

// invocationChainContains reports whether functionName is already in the call chain.
func invocationChainContains(ctx context.Context, functionName string) bool {
	chain, _ := ctx.Value(invocationChainKeyType{}).([]string)

	return slices.Contains(chain, functionName)
}

// InvokeFunction invokes a Lambda function without a qualifier (equivalent to "$LATEST").
// For qualified invocations (alias or version number), use InvokeFunctionWithQualifier.
func (b *InMemoryBackend) InvokeFunction(
	ctx context.Context,
	name string,
	invocationType InvocationType,
	payload []byte,
) ([]byte, int, error) {
	result, _, _, statusCode, err := b.InvokeFunctionWithQualifier(ctx, name, "", "", "", invocationType, payload)

	return result, statusCode, err
}

// asyncInvocationEnqueueTimeout is the maximum time a background goroutine will wait
// to place an async (Event) invocation into the runtime queue. If the queue remains
// full after this duration the invocation is dropped with a warning log.
const asyncInvocationEnqueueTimeout = 5 * time.Minute

// maxAsyncEnqueueWaiters bounds the number of goroutines allowed to block while
// waiting for space in a runtime async invocation queue.
const maxAsyncEnqueueWaiters = 128

// checkRecursiveLoop returns an error when fn is already in the invocation chain and
// its RecursiveLoop config is set to "Deny".
func (b *InMemoryBackend) checkRecursiveLoop(ctx context.Context, functionName string) error {
	if !invocationChainContains(ctx, functionName) {
		return nil
	}

	var rc *FunctionRecursionConfig

	func() {
		b.mu.RLock("checkRecursiveLoop")
		defer b.mu.RUnlock()

		rc = b.functionRecursionConfigs[functionName]
	}()

	mode := "Terminate"
	if rc != nil {
		mode = rc.RecursiveLoop
	}

	if mode == "Deny" {
		return fmt.Errorf(
			"%w: recursive invocation detected for function %s with RecursiveLoop=Deny",
			ErrInvalidParameterValue, functionName,
		)
	}

	return nil
}

// InvokeFunctionWithQualifier invokes a Lambda function using an optional qualifier.
func (b *InMemoryBackend) InvokeFunctionWithQualifier(
	ctx context.Context,
	name, qualifier, clientContext, logType string,
	invocationType InvocationType,
	payload []byte,
) ([]byte, string, string, int, error) {
	fn, err := b.resolveQualifier(name, qualifier)
	if err != nil {
		return nil, "", "", http.StatusNotFound, err
	}

	if invocationType == InvocationTypeDryRun {
		return nil, "", "", http.StatusNoContent, nil
	}

	// Enforce RecursiveLoop=Deny: reject self-invocations when the function name
	// is already in the current invocation chain.
	if loopErr := b.checkRecursiveLoop(ctx, fn.FunctionName); loopErr != nil {
		return nil, "", "", http.StatusBadRequest, loopErr
	}

	// Propagate the invocation chain to nested Lambda calls.
	ctx = withInvocationChain(ctx, fn.FunctionName)

	// Check FIS fault injection state for this function.
	fisPayload, fisStatus, fisErr := b.applyFISFaultToInvocation(ctx, fn.FunctionName)
	if fisPayload != nil || fisErr != nil {
		return fisPayload, "", "", fisStatus, fisErr
	}

	// Enforce reserved concurrency limits for all invocation types.
	// Reserved concurrency of 0 blocks all invocations; non-zero limits are enforced
	// for both synchronous (RequestResponse) and asynchronous (Event) invocations.
	trackConcurrency, concErr := b.acquireConcurrencySlot(fn.FunctionName)
	if concErr != nil {
		return nil, "", "", http.StatusTooManyRequests, concErr
	}

	// For synchronous invocations, release the concurrency slot when this function returns.
	// For async (Event) invocations, enqueueAsyncInvocation releases the slot after the
	// invocation completes or times out.
	if trackConcurrency && invocationType != InvocationTypeEvent {
		defer b.releaseConcurrencySlot(fn.FunctionName)
	}

	srv, srvErr := b.getOrCreateRuntime(ctx, fn)
	if srvErr != nil {
		// Release the slot on error regardless of invocation type.
		if trackConcurrency {
			b.releaseConcurrencySlot(fn.FunctionName)
		}

		return nil, "", "", http.StatusInternalServerError, srvErr
	}

	timeout := time.Duration(fn.Timeout) * time.Second
	if timeout <= 0 {
		timeout = defaultFunctionTimeout
	}

	if invocationType == InvocationTypeEvent {
		b.invokeEvent(ctx, fn, srv, payload, clientContext, timeout, trackConcurrency)

		return nil, "", "", http.StatusAccepted, nil
	}

	return b.invokeSync(ctx, fn, srv, payload, clientContext, logType, timeout)
}

// classifyFunctionError maps an invocation outcome to the AWS X-Amz-Function-Error
// header value. A runtime that reports via the /error endpoint (isError) is an
// Unhandled error; a normal /response whose payload merely has the Lambda error
// shape is a Handled error; anything else is not a function error.
func classifyFunctionError(isError bool, result []byte) string {
	if isError {
		return "Unhandled"
	}

	if isLambdaFunctionErrorPayload(result) {
		return "Handled"
	}

	return ""
}

func (b *InMemoryBackend) invokeSync(
	ctx context.Context,
	fn *FunctionConfiguration,
	srv *runtimeServer,
	payload []byte,
	clientContext, logType string,
	timeout time.Duration,
) ([]byte, string, string, int, error) {
	result, isError, reqID, invokeErr := srv.invoke(ctx, payload, clientContext, timeout)
	if invokeErr != nil {
		if errors.Is(invokeErr, ErrInvocationTimeout) {
			b.cleanupTimedOutRuntime(fn.FunctionName)
		}

		return nil, "", "", http.StatusInternalServerError, invokeErr
	}

	functionError := classifyFunctionError(isError, result)

	b.dispatchInvocationLog(context.WithoutCancel(ctx), fn.FunctionName, payload, result)

	var logResult string
	if logType == LogTypeTail {
		logResult = base64.StdEncoding.EncodeToString([]byte(buildTailLog(reqID)))
	}

	return result, logResult, functionError, http.StatusOK, nil
}

// buildTailLog produces the base64-worthy CloudWatch-style log tail returned in
// the X-Amz-Log-Result header when LogType=Tail is requested.
func buildTailLog(reqID string) string {
	return fmt.Sprintf(
		"START RequestId: %s Version: $LATEST\nEND RequestId: %s\n"+
			"REPORT RequestId: %s\tDuration: 1.00 ms\tBilled Duration: 1 ms\tMemory Size: 128 MB\tMax Memory Used: 64 MB\n",
		reqID, reqID, reqID,
	)
}

func (b *InMemoryBackend) invokeEvent(
	ctx context.Context,
	fn *FunctionConfiguration,
	srv *runtimeServer,
	payload []byte,
	clientContext string,
	timeout time.Duration,
	trackConcurrency bool,
) {
	inv := &pendingInvocation{
		requestID:     uuid.New().String(),
		payload:       payload,
		clientContext: clientContext,
		deadline:      time.Now().Add(timeout),
		createdAt:     time.Now(),
		result:        make(chan invocationResult, 1),
	}

	b.enqueueAsyncInvocation(ctx, srv, fn.FunctionName, inv, timeout, trackConcurrency)
}

// enqueueAsyncInvocation places inv into the runtime queue and then waits for the
// container to respond. The wait serves two purposes:
//  1. Hold the concurrency slot for the full execution duration when trackConcurrency is true.
//  2. Remove any stale srv.pending entry when a container picks up the invocation via
//     /next but never calls /response or /error (e.g., crash), preventing a memory leak.
//
// The enqueue attempts a non-blocking fast path first. If the queue is full a background
// goroutine blocks for up to asyncInvocationEnqueueTimeout before giving up.
// [context.WithoutCancel] detaches the goroutine from the caller's HTTP-request context
// so cancellation of the 202 response does not abort the background work.
func (b *InMemoryBackend) enqueueAsyncInvocation(
	ctx context.Context,
	srv *runtimeServer,
	functionName string,
	inv *pendingInvocation,
	timeout time.Duration,
	trackConcurrency bool,
) {
	log := logger.Load(ctx)

	// Fast path: try a non-blocking enqueue without spawning a goroutine.
	// Even on the fast path we still need a goroutine to clean up srv.pending on
	// container timeout, so only skip the goroutine when there's nothing to track
	// and the queue has immediate space.
	if !trackConcurrency {
		select {
		case srv.queue <- inv:
			// Invocation queued; spawn a minimal goroutine only to clean up srv.pending
			// if the container picks up the invocation but never responds.
			b.asyncWG.Go(func() {
				b.waitAndCleanPending(log, srv, inv, timeout, false, functionName)
			})

			return
		default:
		}
	}

	// Slow path: queue was full (or a slot is held); block until space is available.
	select {
	case b.asyncEnqueueWaiters <- struct{}{}:
	default:
		log.WarnContext(ctx, "lambda: async invocation dropped: enqueue waiters saturated",
			"function", functionName, "requestID", inv.requestID)

		if trackConcurrency {
			b.releaseConcurrencySlot(functionName)
		}

		return
	}

	b.asyncWG.Go(func() {
		defer func() {
			<-b.asyncEnqueueWaiters
		}()

		enqueueCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			asyncInvocationEnqueueTimeout,
		)
		defer cancel()

		select {
		case srv.queue <- inv:
			b.waitAndCleanPending(log, srv, inv, timeout, trackConcurrency, functionName)

		case <-enqueueCtx.Done():
			log.WarnContext(ctx, "lambda: async invocation dropped: queue full",
				"function", functionName, "requestID", inv.requestID)

			if trackConcurrency {
				b.releaseConcurrencySlot(functionName)
			}

		case <-b.shutdown:
			// Backend is shutting down; drop the still-queueing invocation.
			if trackConcurrency {
				b.releaseConcurrencySlot(functionName)
			}
		}
	})
}

// defaultAsyncMaxRetryAttempts is the number of automatic retries AWS Lambda performs
// for async (Event) invocations that fail with a function error. This matches the AWS default.
const defaultAsyncMaxRetryAttempts = 2

// waitAndCleanPending is the exit point for every async invocation goroutine. It runs
// the retry loop and, once all attempts are exhausted or completed, releases the
// concurrency slot if one was acquired.
func (b *InMemoryBackend) waitAndCleanPending(
	log *slog.Logger,
	srv *runtimeServer,
	inv *pendingInvocation,
	timeout time.Duration,
	trackConcurrency bool,
	functionName string,
) {
	b.runAsyncInvocationRetryLoop(log, srv, inv, timeout, functionName)

	if trackConcurrency {
		b.releaseConcurrencySlot(functionName)
	}
}

// runAsyncInvocationRetryLoop executes the async invocation and retries on function errors
// according to the function's event invoke configuration (MaximumRetryAttempts,
// MaximumEventAgeInSeconds). Default retry count mirrors AWS Lambda: 2 retries.
func (b *InMemoryBackend) runAsyncInvocationRetryLoop(
	log *slog.Logger,
	srv *runtimeServer,
	inv *pendingInvocation,
	timeout time.Duration,
	functionName string,
) {
	maxRetries, maxEventAgeDL := b.readAsyncRetryConfig(functionName, inv.createdAt)
	currentInv := inv

	for attempt := range maxRetries + 1 {
		result, ok, containerTimedOut := b.waitForAsyncResult(srv, currentInv, timeout)
		if !ok && !containerTimedOut {
			// Backend shutdown: abandon without retry or destination delivery.
			return
		}

		// AWS treats a runtime timeout as a function error for async retry/destination
		// purposes: "Function errors include errors returned by the function's code and
		// errors returned by the function's runtime, such as timeouts"
		// (docs.aws.amazon.com/lambda/latest/dg/invocation-async-error-handling.html).
		isError := result.isError
		if !ok {
			// A container timeout means the process is hung; evict it so the next
			// invocation gets a fresh container, matching the synchronous timeout path.
			b.cleanupTimedOutRuntime(functionName)
			isError = true
		}

		if !isError || attempt == maxRetries {
			outcome := asyncOutcome{
				functionName:    functionName,
				requestID:       inv.requestID,
				requestPayload:  inv.payload,
				responsePayload: result.payload,
				functionError:   classifyFunctionError(isError, result.payload),
				invokeCount:     attempt + 1,
				statusCode:      result.statusCode,
				success:         !isError,
			}

			if !isError {
				b.dispatchInvocationLog(
					b.ctx,
					functionName,
					inv.payload,
					result.payload,
				)
			} else {
				log.Warn("lambda: async invocation failed after retries",
					"function", functionName, "attempts", attempt+1)
			}

			b.dispatchAsyncOutcome(context.WithoutCancel(b.ctx), outcome)

			return
		}

		newInv := scheduleAsyncRetry(b.ctx, log, srv, inv, timeout, maxEventAgeDL, attempt+1, functionName)
		if newInv == nil {
			return // retry dropped (queue full or event too old)
		}

		currentInv = newInv
	}
}

// readAsyncRetryConfig returns the effective maximum retry attempts and the event-age deadline
// for an async invocation. If no event invoke configuration exists, the AWS defaults are used
// (2 retries, no age limit).
func (b *InMemoryBackend) readAsyncRetryConfig(
	functionName string,
	createdAt time.Time,
) (int, time.Time) {
	b.mu.RLock("readAsyncRetryConfig")
	defer b.mu.RUnlock()

	maxRetries := defaultAsyncMaxRetryAttempts

	cfg, ok := b.eventInvokeConfigs[functionName]
	if !ok {
		return maxRetries, time.Time{}
	}

	if cfg.MaximumRetryAttempts != nil {
		maxRetries = *cfg.MaximumRetryAttempts
	}

	var maxEventAgeDL time.Time

	if cfg.MaximumEventAgeInSeconds != nil {
		maxEventAgeDL = createdAt.Add(time.Duration(*cfg.MaximumEventAgeInSeconds) * time.Second)
	}

	return maxRetries, maxEventAgeDL
}

// waitForAsyncResult waits for a pending invocation to receive a container response or for
// the function timeout to elapse. On timeout it removes the stale srv.pending entry that
// handleNext stored (preventing a memory leak).
// Returns:
//   - (result, true, false)  — container responded in time
//   - (zero, false, true)    — container timed out; the caller should clean up the runtime
func (b *InMemoryBackend) waitForAsyncResult(
	srv *runtimeServer,
	inv *pendingInvocation,
	timeout time.Duration,
) (invocationResult, bool, bool) {
	waitTimer := time.NewTimer(timeout + containerResponseGracePeriod)
	defer func() {
		if !waitTimer.Stop() {
			select {
			case <-waitTimer.C:
			default:
			}
		}
	}()

	select {
	case result := <-inv.result:
		return result, true, false
	case <-waitTimer.C:
		// Container timed out; remove the stale pending entry to prevent a memory leak.
		srv.pending.LoadAndDelete(inv.requestID)

		return invocationResult{}, false, true
	case <-b.shutdown:
		// Backend is shutting down; abandon the wait and remove the stale pending
		// entry. Treated like a timeout (not a container timeout) so the caller
		// stops retrying without evicting a runtime.
		srv.pending.LoadAndDelete(inv.requestID)

		return invocationResult{}, false, false
	}
}

// scheduleAsyncRetry creates a new pendingInvocation for a retry attempt and enqueues it.
// It returns the new invocation on success or nil if the event is too old or the queue
// remains full after asyncInvocationEnqueueTimeout.
func scheduleAsyncRetry(
	ctx context.Context,
	log *slog.Logger,
	srv *runtimeServer,
	original *pendingInvocation,
	timeout time.Duration,
	maxEventAgeDL time.Time,
	attempt int,
	functionName string,
) *pendingInvocation {
	if !maxEventAgeDL.IsZero() && time.Now().After(maxEventAgeDL) {
		log.WarnContext(ctx, "lambda: async retry dropped: event age exceeded",
			"function", functionName, "attempt", attempt)

		return nil
	}

	newInv := &pendingInvocation{
		requestID: uuid.New().String(),
		payload:   original.payload,
		deadline:  time.Now().Add(timeout),
		result:    make(chan invocationResult, 1),
		createdAt: original.createdAt,
	}

	ctx, cancel := context.WithTimeout(ctx, asyncInvocationEnqueueTimeout)
	defer cancel()

	select {
	case srv.queue <- newInv:
		return newInv
	case <-ctx.Done():
		log.WarnContext(ctx, "lambda: async retry dropped: queue full",
			"function", functionName, "requestID", newInv.requestID, "attempt", attempt)

		return nil
	}
}

// acquireConcurrencySlot checks and optionally increments the active concurrency counter
// for a function. It returns (true, nil) when a slot was acquired (caller must release),
// (false, nil) when the function has no reserved concurrency limit, or (false, err) when
// the limit is already exhausted. Must not be called with b.mu held.
func (b *InMemoryBackend) acquireConcurrencySlot(functionName string) (bool, error) {
	b.mu.Lock("acquireConcurrencySlot")
	defer b.mu.Unlock()

	reserved, hasLimit := b.functionConcurrencies[functionName]
	if !hasLimit {
		// No reserved concurrency limit — check scaling config MaxExecutionEnvironments instead.
		if sc, ok := b.functionScalingConfigs[functionName]; ok && sc.MaxExecutionEnvironments != nil {
			active := b.activeConcurrencies[functionName]
			if active >= int(*sc.MaxExecutionEnvironments) {
				return false, fmt.Errorf(
					"%w: scaling concurrency limit reached for function %s",
					ErrTooManyRequests,
					functionName,
				)
			}

			b.activeConcurrencies[functionName]++

			return true, nil
		}

		return false, nil
	}

	// Reserved concurrency of 0 disables all invocations regardless of type.
	if reserved == 0 {
		return false, fmt.Errorf(
			"%w: reserved concurrency is 0 for function %s",
			ErrTooManyRequests,
			functionName,
		)
	}

	active := b.activeConcurrencies[functionName]
	if active >= reserved {
		return false, fmt.Errorf(
			"%w: concurrent execution limit reached for function %s",
			ErrTooManyRequests,
			functionName,
		)
	}

	// Also enforce MaxExecutionEnvironments from scaling config when set.
	if sc, ok := b.functionScalingConfigs[functionName]; ok && sc.MaxExecutionEnvironments != nil {
		if active >= int(*sc.MaxExecutionEnvironments) {
			return false, fmt.Errorf(
				"%w: scaling concurrency limit reached for function %s",
				ErrTooManyRequests,
				functionName,
			)
		}
	}

	b.activeConcurrencies[functionName]++

	return true, nil
}

// releaseConcurrencySlot decrements the active concurrency counter for a function.
// Entries are deleted when the count reaches zero to prevent unbounded map growth.
// Must not be called with b.mu held.
func (b *InMemoryBackend) releaseConcurrencySlot(functionName string) {
	b.mu.Lock("releaseConcurrencySlot")
	defer b.mu.Unlock()

	if b.activeConcurrencies[functionName] > 0 {
		b.activeConcurrencies[functionName]--
		if b.activeConcurrencies[functionName] == 0 {
			delete(b.activeConcurrencies, functionName)
		}
	}
}

// dispatchInvocationLog asynchronously emits an invocation log entry. The
// goroutine count is bounded by b.logSem; when saturated, the log is dropped
// (best-effort observability) so a slow CloudWatch Logs backend cannot leak
// goroutines under high invocation throughput.
func (b *InMemoryBackend) dispatchInvocationLog(
	ctx context.Context,
	functionName string,
	payload, result []byte,
) {
	// Capture the semaphore channel under the read lock so that a concurrent Reset()
	// cannot replace b.logSem between the send and the goroutine's deferred release.
	var sem chan struct{}

	func() {
		b.mu.RLock("dispatchInvocationLog.sem")
		defer b.mu.RUnlock()

		sem = b.logSem
	}()

	select {
	case sem <- struct{}{}:
	default:
		logger.Load(ctx).WarnContext(ctx, "lambda: invocation log dropped: logSem saturated",
			"function", functionName)

		return
	}

	go func() {
		defer func() { <-sem }()
		b.pushInvocationLog(ctx, functionName, payload, result)
	}()
}

// pushInvocationLog writes a minimal invocation log entry to CloudWatch Logs when a backend is set.
func (b *InMemoryBackend) pushInvocationLog(
	ctx context.Context,
	functionName string,
	_ []byte,
	result []byte,
) {
	var cwl CWLogsBackend

	func() {
		b.mu.RLock("pushInvocationLog")
		defer b.mu.RUnlock()

		cwl = b.cwLogs
	}()

	if cwl == nil {
		return
	}

	groupName := "/aws/lambda/" + functionName
	streamName := time.Now().UTC().Format("2006/01/02") + "/[$LATEST]" + uuid.New().String()[:8]

	if err := cwl.EnsureLogGroupAndStream(groupName, streamName); err != nil {
		logger.Load(ctx).WarnContext(ctx, "pushInvocationLog: failed to ensure log group/stream",
			"function", functionName, "error", err)

		return
	}

	requestID := uuid.New().String()
	report := "REPORT RequestId: " + requestID +
		"\tDuration: 0.00 ms\tBilled Duration: 1 ms\tMemory Size: 128 MB\tMax Memory Used: 128 MB"
	messages := []string{
		"START RequestId: " + requestID + " Version: $LATEST",
	}
	if len(result) > 0 {
		messages = append(messages, string(result))
	}
	messages = append(
		messages,
		"END RequestId: "+requestID,
		report,
	)

	if err := cwl.PutLogLines(groupName, streamName, messages); err != nil {
		logger.Load(ctx).WarnContext(ctx, "pushInvocationLog: failed to put log lines",
			"function", functionName, "error", err)
	}
}

// defaultFunctionTimeout is used when the function has no timeout configured.
const defaultFunctionTimeout = 3 * time.Second
