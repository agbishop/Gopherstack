package bedrockruntime

import "time"

// maxInvocationHistory is the maximum number of invocations retained in memory.
const maxInvocationHistory = 1000

// MaxInvocationHistory is the exported value for testing.
const MaxInvocationHistory = maxInvocationHistory

// Async invoke status values.
const (
	AsyncInvokeStatusInProgress = "InProgress"
	AsyncInvokeStatusCompleted  = "Completed"
	AsyncInvokeStatusFailed     = "Failed"
)

// Invocation records a single model invocation.
type Invocation struct {
	CreatedAt time.Time
	ModelID   string
	Operation string
	Input     string
	Output    string
}

// AsyncInvoke records an asynchronous model invocation.
type AsyncInvoke struct {
	SubmitTime         time.Time
	LastModifiedTime   time.Time
	EndTime            *time.Time
	FailureMessage     *string
	ClientRequestToken *string
	Tags               map[string]string
	InvocationArn      string
	ModelArn           string
	OutputS3URI        string
	Status             string
}

// asyncInvokeSortOrderDescending is the ListAsyncInvokesInput.SortOrder value
// (types.SortOrder) that reverses the default ascending-by-submit-time order.
const asyncInvokeSortOrderDescending = "Descending"

// ListAsyncInvokesFilter holds optional filter criteria for listing async invocations.
type ListAsyncInvokesFilter struct {
	// SubmitTimeAfter, if set, excludes invocations submitted at or before this time.
	SubmitTimeAfter *time.Time
	// SubmitTimeBefore, if set, excludes invocations submitted at or after this time.
	SubmitTimeBefore *time.Time
	// StatusEquals filters to invocations with the given status; empty means no filter.
	StatusEquals string
	// SortOrder reverses SubmitTime ordering when set to
	// asyncInvokeSortOrderDescending; any other value (including empty) sorts ascending.
	SortOrder string
}

// invocationRing is a fixed-capacity circular buffer for Invocation records.
// Once full, new writes overwrite the oldest entry (FIFO eviction).
type invocationRing struct {
	buf       []*Invocation
	head      int // index of the oldest entry
	count     int // number of valid entries (0 ≤ count ≤ sz)
	evictions int // total evictions since last reset
}

// newInvocationRing allocates a ring with the given capacity.
func newInvocationRing(sz int) invocationRing {
	return invocationRing{buf: make([]*Invocation, sz)}
}

// push appends inv, evicting the oldest entry when the ring is full.
func (r *invocationRing) push(inv *Invocation) {
	sz := len(r.buf)
	if r.count < sz {
		r.buf[(r.head+r.count)%sz] = inv
		r.count++
	} else {
		// Overwrite oldest slot and advance head — oldest entry is evicted.
		r.evictions++
		r.buf[r.head] = inv
		r.head = (r.head + 1) % sz
	}
}

// snapshot returns all entries in insertion order (oldest first).
func (r *invocationRing) snapshot() []*Invocation {
	out := make([]*Invocation, r.count)
	sz := len(r.buf)

	for i := range r.count {
		out[i] = r.buf[(r.head+i)%sz]
	}

	return out
}

// reset discards all entries without reallocating the buffer.
func (r *invocationRing) reset() {
	r.head = 0
	r.count = 0
}
