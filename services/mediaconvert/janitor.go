package mediaconvert

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	// janitorInterval is how often the janitor tick fires.
	janitorInterval = 500 * time.Millisecond
)

const (
	simOutputDurationMs = 60000
	simVideoWidthPx     = 1920
	simVideoHeightPx    = 1080
)

// StartJanitor launches the background goroutine that advances job phases and
// sweeps expired tokens. It runs until ctx is cancelled.
func StartJanitor(ctx context.Context, b *InMemoryBackend) {
	// Group.Ticker spawns the sweep goroutine itself; it exits when ctx is
	// cancelled, matching the previous fire-and-forget behaviour.
	g := worker.NewGroup(ctx, "mediaconvert")
	g.Ticker("JobPhaseAdvancer", janitorInterval, 0,
		func(ctx context.Context) { janitorTick(ctx, b) })
}

// janitorTick performs one cycle: advance job phases and sweep tokens.
func janitorTick(ctx context.Context, b *InMemoryBackend) {
	advanced := b.AdvanceJobPhase()
	b.SweepExpiredTokens()

	if advanced {
		logger.Load(ctx).DebugContext(ctx, "mediaconvert: janitor advanced job phase(s)")
	}
}

// SweepExpiredTokens removes token entries that have exceeded the TTL.
// Called by the janitor; safe to call externally for testing.
func (b *InMemoryBackend) SweepExpiredTokens() {
	b.mu.RLock("SweepExpiredTokens")

	var expired []string
	for _, entry := range b.tokenIndex.All() {
		if time.Since(entry.createdAt) >= tokenTTL {
			expired = append(expired, entry.token)
		}
	}

	b.mu.RUnlock()

	if len(expired) == 0 {
		return
	}

	b.mu.Lock("SweepExpiredTokens.delete")
	defer b.mu.Unlock()

	for _, token := range expired {
		b.tokenIndex.Delete(token)
	}
}

// AdvanceJobPhase advances job phase/status for the janitor.
// Returns true if any job was advanced.
func (b *InMemoryBackend) AdvanceJobPhase() bool {
	b.mu.RLock("AdvanceJobPhase")

	var eligible []string
	for _, j := range b.jobs.All() {
		switch j.Status {
		case jobStatusSubmitted, jobStatusProgressing:
			eligible = append(eligible, j.ID)
		}
	}

	b.mu.RUnlock()

	if len(eligible) == 0 {
		return false
	}

	b.mu.Lock("AdvanceJobPhase.advance")
	defer b.mu.Unlock()

	advanced := false
	now := epochSeconds(time.Now())

	for _, id := range eligible {
		j, ok := b.jobs.Get(id)
		if !ok {
			continue
		}
		if b.advanceOneJobLocked(j, now) {
			advanced = true
		}
	}

	return advanced
}

// advanceOneJobLocked advances the state machine for a single job.
// Caller must hold the write lock. Returns true if the job changed.
func (b *InMemoryBackend) advanceOneJobLocked(j *Job, now float64) bool {
	switch j.Status {
	case jobStatusSubmitted:
		return b.advanceSubmittedLocked(j, now)
	case jobStatusProgressing:
		return b.advanceProgressingLocked(j, now)
	case jobStatusError, jobStatusComplete, jobStatusCanceled:
		return false
	}

	return false
}

// queueAdmitsLocked reports whether queueArn is willing to start another job
// right now. Queue.Status doc (aws-sdk-go-v2 mediaconvert types.go): "If you
// pause a queue, the service won't begin processing jobs in that queue."
// ConcurrentJobs doc (gopherstack-7bxb; CreateQueueInput/UpdateQueueInput,
// api_op_CreateQueue.go:38-42): "the maximum number of jobs your queue can
// process concurrently" -- a queue already running that many PROGRESSING
// jobs also does not admit. An empty queueArn (no queue assigned) always
// admits. Caller must hold at least a read lock.
func (b *InMemoryBackend) queueAdmitsLocked(queueArn string) bool {
	if queueArn == "" {
		return true
	}

	matches := b.queuesByArn.Get(queueArn)
	if len(matches) == 0 {
		return true
	}

	q := matches[0]
	if q.Status == statusPaused {
		return false
	}

	if q.ConcurrentJobs != nil {
		progressing, _ := b.getQueueCounterLocked(queueArn)
		if progressing >= *q.ConcurrentJobs {
			return false
		}
	}

	return true
}

// advanceSubmittedLocked transitions a SUBMITTED job to PROGRESSING/PROBING,
// unless queueAdmitsLocked says its queue isn't ready for another job. A job
// that doesn't advance stays SUBMITTED until a later tick.
func (b *InMemoryBackend) advanceSubmittedLocked(j *Job, now float64) bool {
	if !b.queueAdmitsLocked(j.QueueArn) {
		return false
	}

	// Decrement SUBMITTED counter, increment PROGRESSING counter.
	b.adjustQueueCounterLocked(j.QueueArn, jobStatusSubmitted, -1)
	b.adjustQueueCounterLocked(j.QueueArn, jobStatusProgressing, +1)

	j.Status = jobStatusProgressing
	j.CurrentPhase = jobPhaseProbing

	if j.Timing == nil {
		j.Timing = &JobTiming{}
	}

	j.Timing.StartTime = now

	return true
}

// advanceProgressingLocked advances phase within PROGRESSING, eventually to COMPLETE.
func (b *InMemoryBackend) advanceProgressingLocked(j *Job, now float64) bool {
	switch j.CurrentPhase {
	case jobPhaseProbing:
		j.CurrentPhase = jobPhaseTranscoding

		return true
	case jobPhaseTranscoding:
		j.CurrentPhase = jobPhaseUploading

		return true
	case jobPhaseUploading:
		return b.completeJobLocked(j, now)
	}

	return false
}

// completeJobLocked transitions a job to COMPLETE and fills output details.
func (b *InMemoryBackend) completeJobLocked(j *Job, now float64) bool {
	b.adjustQueueCounterLocked(j.QueueArn, jobStatusProgressing, -1)

	j.Status = jobStatusComplete
	j.CurrentPhase = ""
	j.JobPercentComplete = 100

	if j.Timing == nil {
		j.Timing = &JobTiming{}
	}

	j.Timing.FinishTime = now

	j.OutputGroupDetails = []OutputGroupDetail{
		{
			OutputDetails: []OutputDetail{
				{
					DurationInMs: simOutputDurationMs,
					VideoDetails: &VideoDetail{WidthInPx: simVideoWidthPx, HeightInPx: simVideoHeightPx},
				},
			},
		},
	}

	return true
}

// rebuildCountersLocked rebuilds queueCounters from the current jobs table.
// Used when restoring a snapshot that predates per-queue counters. Mirrors
// the pre-Phase-3.3 maps.Copy merge semantics: only queue ARNs with at least
// one SUBMITTED/PROGRESSING job are (re)written; any other pre-existing
// queueCounters entry is left untouched. Caller must hold the write lock.
func (b *InMemoryBackend) rebuildCountersLocked() {
	counters := make(map[string]*queueJobCounter)

	for _, j := range b.jobs.All() {
		if j.QueueArn == "" {
			continue
		}

		c := counters[j.QueueArn]
		if c == nil {
			c = &queueJobCounter{queueArn: j.QueueArn}
			counters[j.QueueArn] = c
		}

		switch j.Status {
		case jobStatusProgressing:
			c.progressing++
		case jobStatusSubmitted:
			c.submitted++
		}
	}

	for _, c := range counters {
		b.queueCounters.Put(c)
	}
}
