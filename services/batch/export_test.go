package batch

import (
	"fmt"
	"time"
)

// DefaultJanitorInterval exposes the package default janitor interval for testing.
const DefaultJanitorInterval = defaultBatchJanitorInterval

// DefaultInactiveJobDefTTL exposes the package default inactive job def TTL for testing.
const DefaultInactiveJobDefTTL = defaultBatchInactiveJobDefTTL

// DefaultCompletedJobTTL exposes the package default completed job TTL for testing.
const DefaultCompletedJobTTL = defaultBatchCompletedJobTTL

// JobDefinitionCount returns the number of job definitions stored in the backend.
// Used only in tests.
func (b *InMemoryBackend) JobDefinitionCount() int {
	b.mu.RLock("JobDefinitionCount")
	defer b.mu.RUnlock()

	return len(b.jobDefinitionsByRegion.Get(b.region))
}

// RevisionFor returns the current revision counter for the given job definition name.
// Returns 0 if the name has no counter (deleted or never registered).
// Used only in tests.
func (b *InMemoryBackend) RevisionFor(name string) int32 {
	b.mu.RLock("RevisionFor")
	defer b.mu.RUnlock()

	return b.jobDefRevisionsStoreRO(b.region)[name]
}

// HasRevisionCounter reports whether a revision counter exists for name.
// Used only in tests to verify cleanup.
func (b *InMemoryBackend) HasRevisionCounter(name string) bool {
	b.mu.RLock("HasRevisionCounter")
	defer b.mu.RUnlock()

	_, ok := b.jobDefRevisionsStoreRO(b.region)[name]

	return ok
}

// SetJobDefinitionDeregisteredAt overrides the DeregisteredAt timestamp for a job definition.
// Used only in tests to simulate TTL expiry.
func (b *InMemoryBackend) SetJobDefinitionDeregisteredAt(arnOrNameRev string, timestamp time.Time) {
	b.mu.Lock("SetJobDefinitionDeregisteredAt")
	defer b.mu.Unlock()

	if jd, ok := b.jobDefinitions.Get(regionKey(b.region, arnOrNameRev)); ok {
		jd.DeregisteredAt = &timestamp

		return
	}

	for _, jd := range b.jobDefinitionsByRegion.Get(b.region) {
		nameRev := fmt.Sprintf("%s:%d", jd.JobDefinitionName, jd.Revision)
		if nameRev == arnOrNameRev {
			jd.DeregisteredAt = &timestamp

			return
		}
	}
}

// GetJanitorTaskTimeout returns the TaskTimeout configured on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the timeout.
func (h *Handler) GetJanitorTaskTimeout() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.TaskTimeout
}

// GetJanitorInactiveJobDefTTL returns the InactiveJobDefTTL on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the TTL.
func (h *Handler) GetJanitorInactiveJobDefTTL() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.InactiveJobDefTTL
}

// GetJanitorCompletedJobTTL returns the CompletedJobTTL on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the TTL.
func (h *Handler) GetJanitorCompletedJobTTL() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.CompletedJobTTL
}

// GetJanitorInterval returns the Interval configured on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the interval.
func (h *Handler) GetJanitorInterval() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.Interval
}

// SetJobStoppedAtForTest sets a job's status and StoppedAt timestamp for testing.
// Used to simulate a job that stopped at a specific time for TTL sweep tests.
func (b *InMemoryBackend) SetJobStoppedAtForTest(jobID, status string, stoppedAt time.Time) {
	b.mu.Lock("SetJobStoppedAtForTest")
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(regionKey(b.region, jobID))
	if !ok {
		return
	}

	j.Status = status
	ms := stoppedAt.UnixMilli()
	j.StoppedAt = &ms
}

// ForceJobStatus directly sets a job's status, bypassing normal transitions.
// For use in tests only.
func (b *InMemoryBackend) ForceJobStatus(jobID, status string) {
	b.mu.Lock("ForceJobStatus")
	defer b.mu.Unlock()
	if j, ok := b.jobs.Get(regionKey(b.region, jobID)); ok {
		j.Status = status
	}
}

// SetJobStartedAtForTest backdates a job's StartedAt timestamp. Used to
// simulate a RUNNING job whose attempt has run longer than its JobTimeout
// without a real sleep.
func (b *InMemoryBackend) SetJobStartedAtForTest(jobID string, startedAt time.Time) {
	b.mu.Lock("SetJobStartedAtForTest")
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(regionKey(b.region, jobID))
	if !ok {
		return
	}

	ms := startedAt.UnixMilli()
	j.StartedAt = &ms
}
