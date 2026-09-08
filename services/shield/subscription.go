package shield

import (
	"fmt"
	"time"
)

// subscriptionCommitmentDays is the default Shield Advanced subscription commitment period.
const subscriptionCommitmentDays int64 = 365

// CreateSubscription enables Shield Advanced. Returns an error if already subscribed.
func (b *InMemoryBackend) CreateSubscription() error {
	b.mu.Lock("CreateSubscription")
	defer b.mu.Unlock()

	if b.subscription != nil {
		return fmt.Errorf("%w: subscription already exists", ErrSubscriptionAlreadyExists)
	}

	now := time.Now()
	b.subscription = &Subscription{
		StartTime:            now,
		EndTime:              now.AddDate(1, 0, 0),
		AutoRenew:            "ENABLED",
		TimeCommitmentInDays: subscriptionCommitmentDays,
	}

	if b.proactiveEngagementStatus == "" {
		b.proactiveEngagementStatus = ProactiveEngagementDisabled
	}

	return nil
}

// DescribeSubscription returns the current Shield Advanced subscription.
func (b *InMemoryBackend) DescribeSubscription() (*Subscription, error) {
	b.mu.RLock("DescribeSubscription")
	defer b.mu.RUnlock()

	if b.subscription == nil {
		return nil, fmt.Errorf("%w: no subscription found", ErrSubscriptionNotFound)
	}

	s := *b.subscription

	return &s, nil
}

// GetSubscriptionState returns ACTIVE or INACTIVE.
func (b *InMemoryBackend) GetSubscriptionState() string {
	b.mu.RLock("GetSubscriptionState")
	defer b.mu.RUnlock()

	if b.subscription != nil {
		return "ACTIVE"
	}

	return "INACTIVE"
}

// AddSubscriptionInternal creates a subscription directly (for tests).
func (b *InMemoryBackend) AddSubscriptionInternal() {
	b.mu.Lock("AddSubscriptionInternal")
	defer b.mu.Unlock()

	now := time.Now()
	b.subscription = &Subscription{
		StartTime:            now,
		EndTime:              now.AddDate(1, 0, 0),
		AutoRenew:            AutoRenewEnabled,
		TimeCommitmentInDays: subscriptionCommitmentDays,
	}
}

// DeleteSubscription cancels the active Shield Advanced subscription.
func (b *InMemoryBackend) DeleteSubscription() error {
	b.mu.Lock("DeleteSubscription")
	defer b.mu.Unlock()

	if b.subscription == nil {
		return fmt.Errorf("%w: no active subscription found", ErrSubscriptionNotFound)
	}

	b.subscription = nil

	return nil
}

// UpdateSubscription updates the auto-renew setting of the active subscription.
// An empty autoRenew leaves the existing value unchanged, per api_op_UpdateSubscription.go's
// doc comment: if the request does not include a value for AutoRenew, the existing value
// remains unchanged.
func (b *InMemoryBackend) UpdateSubscription(autoRenew string) error {
	b.mu.Lock("UpdateSubscription")
	defer b.mu.Unlock()

	if b.subscription == nil {
		return fmt.Errorf("%w: no active subscription found", ErrSubscriptionNotFound)
	}

	if autoRenew != "" {
		b.subscription.AutoRenew = autoRenew
	}

	return nil
}
