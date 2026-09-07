package pipes

import (
	"context"
	"testing"
	"time"
)

// PollAllPipesOnce triggers a single synchronous poll cycle for tests.
func PollAllPipesOnce(ctx context.Context, r *Runner) {
	pipes := r.backend.allRunningPipes()

	for _, p := range pipes {
		r.pollPipe(ctx, p)
	}
}

// CreatePipeSimple is a test helper that creates a pipe using positional args.
func (b *InMemoryBackend) CreatePipeSimple(
	name, roleARN, source, target, description, desiredState string,
	tags map[string]string,
) (*Pipe, error) {
	return b.CreatePipe(context.Background(), CreatePipeInput{
		Name:         name,
		RoleARN:      roleARN,
		Source:       source,
		Target:       target,
		Description:  description,
		DesiredState: desiredState,
		Tags:         tags,
	})
}

// ListPipesAll returns all pipes without filtering (test convenience).
func (b *InMemoryBackend) ListPipesAll() []*Pipe {
	res, _ := b.ListPipes(context.Background(), ListPipesFilter{})

	return res.Pipes
}

// EpochMillisForTest exposes epochMillis for direct unit testing of timestamp precision.
func EpochMillisForTest(t time.Time) float64 {
	return epochMillis(t)
}

// WaitPipeRunning waits up to 500ms for a pipe to reach RUNNING state.
func WaitPipeRunning(t *testing.T, b *InMemoryBackend, name string) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		p, err := b.GetPipe(context.Background(), name)
		if err == nil && p.CurrentState == stateRunning {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("pipe %q did not reach RUNNING state within 500ms", name)
}

// WaitPipeStopped waits up to 500ms for a pipe to reach STOPPED state.
func WaitPipeStopped(t *testing.T, b *InMemoryBackend, name string) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		p, err := b.GetPipe(context.Background(), name)
		if err == nil && p.CurrentState == stateStopped {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("pipe %q did not reach STOPPED state within 500ms", name)
}

// SetFilterPatternForTest overwrites a pipe's first filter pattern directly
// in the backend, bypassing CreatePipe/UpdatePipe's Filter.Pattern
// validation (gopherstack-sphp). For pinning matchesAnyFilter's runtime
// fail-closed behavior on patterns creation-time validation now rejects.
func (b *InMemoryBackend) SetFilterPatternForTest(name, pattern string) {
	b.mu.Lock("SetFilterPatternForTest")
	defer b.mu.Unlock()

	p, ok := b.pipesTable(b.region).Get(name)
	if !ok {
		return
	}

	p.SourceParameters.FilterCriteria.Filters[0].Pattern = pattern
}

// EnrichmentCallCountForTest returns the enrichment call count for a pipe in the default region.
func (b *InMemoryBackend) EnrichmentCallCountForTest(pipeName string) int64 {
	return b.GetEnrichmentCallCount(context.Background(), pipeName)
}

// EnrichmentIndexSizeForTest returns the total number of entries in the enrichment counter map
// for the default region (used to verify that deleted pipes are pruned).
func (b *InMemoryBackend) EnrichmentIndexSizeForTest() int {
	b.mu.RLock("EnrichmentIndexSizeForTest")
	defer b.mu.RUnlock()

	if t, ok := b.enrichmentCounts[b.region]; ok {
		return t.Len()
	}

	return 0
}
