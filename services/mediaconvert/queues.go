package mediaconvert

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// AddQueueInternal inserts a queue directly into the backend, bypassing business logic.
func (b *InMemoryBackend) AddQueueInternal(q *Queue) {
	b.mu.Lock("AddQueueInternal")
	defer b.mu.Unlock()

	b.queues.Put(q)
}

// CreateQueue creates a new MediaConvert queue.
func (b *InMemoryBackend) CreateQueue(
	name, description, pricingPlan, status string,
	tags map[string]string,
) (*Queue, error) {
	return b.CreateQueueFull(name, description, pricingPlan, status, tags, nil, nil)
}

// QueueCreateExtras carries newer optional CreateQueue fields
// (MaximumConcurrentFeeds) added to the CreateQueueFull signature after its
// long positional parameter list was already established. CreateQueueFull
// accepts it as a variadic trailing parameter (pass zero or one) so existing
// call sites keep compiling unchanged.
type QueueCreateExtras struct {
	MaximumConcurrentFeeds *int
}

// CreateQueueFull creates a queue with all optional fields.
func (b *InMemoryBackend) CreateQueueFull(
	name, description, pricingPlan, status string,
	tags map[string]string,
	concurrentJobs *int,
	reservationPlan *ReservationPlan,
	extras ...QueueCreateExtras,
) (*Queue, error) {
	b.mu.Lock("CreateQueue")
	defer b.mu.Unlock()

	var extra QueueCreateExtras
	if len(extras) > 0 {
		extra = extras[0]
	}

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	if b.queues.Has(name) {
		return nil, fmt.Errorf("%w: queue %s already exists", ErrAlreadyExists, name)
	}

	if pricingPlan == "" {
		pricingPlan = pricingPlanOnDemand
	}

	if status == "" {
		status = statusActive
	}

	if status != statusActive && status != statusPaused {
		return nil, fmt.Errorf("%w: status must be ACTIVE or PAUSED", ErrValidation)
	}

	now := epochSeconds(time.Now())
	q := &Queue{
		Arn:                    arn.Build("mediaconvert", b.region, b.accountID, "queues/"+name),
		Name:                   name,
		Description:            description,
		PricingPlan:            pricingPlan,
		Status:                 status,
		Type:                   presetCustom,
		Tags:                   nonNilTagsCopy(tags),
		CreatedAt:              now,
		LastUpdated:            now,
		ConcurrentJobs:         cloneIntPtr(concurrentJobs),
		ReservationPlan:        cloneReservationPlan(reservationPlan),
		MaximumConcurrentFeeds: cloneIntPtr(extra.MaximumConcurrentFeeds),
	}
	b.queues.Put(q)
	b.initQueueCounterLocked(q.Arn)

	if len(tags) > 0 {
		b.storeTagsLocked(q.Arn, tags)
	}

	cp := cloneQueue(q)

	return cp, nil
}

// GetQueue returns a queue by name.
func (b *InMemoryBackend) GetQueue(name string) (*Queue, error) {
	b.mu.RLock("GetQueue")
	defer b.mu.RUnlock()

	q, ok := b.queues.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: queue %s not found", ErrNotFound, name)
	}

	cp := cloneQueue(q)
	cp.ProgressingJobsCount, cp.SubmittedJobsCount = b.getQueueCounterLocked(q.Arn)

	return cp, nil
}

// ListQueues returns all queues sorted by name.
func (b *InMemoryBackend) ListQueues() []*Queue {
	b.mu.RLock("ListQueues")
	defer b.mu.RUnlock()

	list := make([]*Queue, 0, b.queues.Len())
	for _, q := range b.queues.All() {
		cp := cloneQueue(q)
		cp.ProgressingJobsCount, cp.SubmittedJobsCount = b.getQueueCounterLocked(q.Arn)
		list = append(list, cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return list
}

// initQueueCounterLocked creates a counter entry for queueArn if it does not exist.
// Caller must hold the write lock.
func (b *InMemoryBackend) initQueueCounterLocked(queueArn string) {
	if !b.queueCounters.Has(queueArn) {
		b.queueCounters.Put(&queueJobCounter{queueArn: queueArn})
	}
}

// getQueueCounterLocked returns (progressing, submitted) counts for queueArn.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) getQueueCounterLocked(queueArn string) (int, int) {
	c, ok := b.queueCounters.Get(queueArn)
	if !ok {
		return 0, 0
	}

	return c.progressing, c.submitted
}

// adjustQueueCounterLocked adds delta to the counter field corresponding to status.
// Caller must hold the write lock.
func (b *InMemoryBackend) adjustQueueCounterLocked(queueArn, status string, delta int) {
	if queueArn == "" {
		return
	}

	b.initQueueCounterLocked(queueArn)
	c, _ := b.queueCounters.Get(queueArn)

	switch status {
	case jobStatusSubmitted:
		c.submitted += delta
	case jobStatusProgressing:
		c.progressing += delta
	}
}

// UpdateQueue updates a queue's description, status, concurrent-job limit,
// reservation plan settings, and Elemental Inference feed concurrency.
// concurrentJobs, reservationPlanSettings, and maximumConcurrentFeeds are nil
// when the caller doesn't want to change that field (matches the real
// UpdateQueueInput, whose members are all optional).
func (b *InMemoryBackend) UpdateQueue(
	name, description, status string,
	concurrentJobs *int,
	reservationPlanSettings *ReservationPlan,
	maximumConcurrentFeeds *int,
) (*Queue, error) {
	b.mu.Lock("UpdateQueue")
	defer b.mu.Unlock()

	q, ok := b.queues.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: queue %s not found", ErrNotFound, name)
	}

	if status != "" && status != statusActive && status != statusPaused {
		return nil, fmt.Errorf("%w: status must be ACTIVE or PAUSED", ErrValidation)
	}

	if description != "" {
		q.Description = description
	}

	if status != "" {
		q.Status = status
	}

	if concurrentJobs != nil {
		q.ConcurrentJobs = cloneIntPtr(concurrentJobs)
	}

	if reservationPlanSettings != nil {
		q.ReservationPlan = cloneReservationPlan(reservationPlanSettings)
	}

	if maximumConcurrentFeeds != nil {
		q.MaximumConcurrentFeeds = cloneIntPtr(maximumConcurrentFeeds)
	}

	q.LastUpdated = epochSeconds(time.Now())

	return cloneQueue(q), nil
}

// DeleteQueue removes a queue by name.
func (b *InMemoryBackend) DeleteQueue(name string) error {
	b.mu.Lock("DeleteQueue")
	defer b.mu.Unlock()

	q, ok := b.queues.Get(name)
	if !ok {
		return fmt.Errorf("%w: queue %s not found", ErrNotFound, name)
	}
	delete(b.tags, q.Arn)
	b.queueCounters.Delete(q.Arn)
	b.queues.Delete(name) // also removes q from the queuesByArn index

	return nil
}

// resolveQueueLocked looks up a queue by name or ARN.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) resolveQueueLocked(queue string) (*Queue, error) {
	if strings.HasPrefix(queue, "arn:") {
		if matches := b.queuesByArn.Get(queue); len(matches) > 0 {
			return matches[0], nil
		}
	} else if q, ok := b.queues.Get(queue); ok {
		return q, nil
	}

	return nil, fmt.Errorf("%w: queue %s not found", ErrNotFound, queue)
}

// --- Deep clone helpers ---

func cloneReservationPlan(rp *ReservationPlan) *ReservationPlan {
	if rp == nil {
		return nil
	}

	cp := *rp

	return &cp
}

func cloneQueue(q *Queue) *Queue {
	cp := *q
	cp.Tags = nonNilTagsCopy(q.Tags)
	cp.MaximumConcurrentFeeds = cloneIntPtr(q.MaximumConcurrentFeeds)
	cp.ConcurrentJobs = cloneIntPtr(q.ConcurrentJobs)

	if q.ReservationPlan != nil {
		rp := *q.ReservationPlan
		cp.ReservationPlan = &rp
	}

	return &cp
}

// cloneIntPtr returns a copy of p so the returned Queue can't alias backend
// state through a shared pointer.
func cloneIntPtr(p *int) *int {
	if p == nil {
		return nil
	}

	v := *p

	return &v
}
