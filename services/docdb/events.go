package docdb

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"
)

// AddSourceIdentifierToSubscription adds a source identifier to an event subscription.
func (b *InMemoryBackend) AddSourceIdentifierToSubscription(
	ctx context.Context,
	subscriptionName, sourceID string,
) (*EventSubscription, error) {
	if subscriptionName == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	if sourceID == "" {
		return nil, fmt.Errorf("%w: SourceIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("AddSourceIdentifierToSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, subscriptionName)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrEventSubscriptionNotFound, subscriptionName)
	}
	if slices.Contains(sub.SourceIDs, sourceID) {
		return copyEventSubscription(sub), nil
	}
	sub.SourceIDs = append(sub.SourceIDs, sourceID)

	return copyEventSubscription(sub), nil
}

// CreateEventSubscription creates an event subscription. enabled mirrors
// CreateEventSubscriptionInput.Enabled (a real request field this backend
// previously dropped on the floor -- never parsed by the handler, never
// stored, never echoed back); when the caller doesn't specify it, AWS
// activates new subscriptions by default, so a nil enabled defaults to true.
func (b *InMemoryBackend) CreateEventSubscription(
	ctx context.Context,
	name, snsTopicARN, sourceType string,
	eventCategories, sourceIDs []string,
	enabled *bool,
	tags map[string]string,
) (*EventSubscription, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateEventSubscription")
	defer b.mu.Unlock()
	if b.eventSubscriptionHas(region, name) {
		return nil, fmt.Errorf("%w: subscription %s already exists", ErrEventSubscriptionAlreadyExists, name)
	}
	cats := make([]string, len(eventCategories))
	copy(cats, eventCategories)
	ids := make([]string, len(sourceIDs))
	copy(ids, sourceIDs)
	isEnabled := true
	if enabled != nil {
		isEnabled = *enabled
	}
	subArn := b.eventSubscriptionARN(region, name)
	sub := &EventSubscription{
		region:                   region,
		SubscriptionName:         name,
		SnsTopicARN:              snsTopicARN,
		Status:                   "active",
		SourceType:               sourceType,
		EventCategories:          cats,
		SourceIDs:                ids,
		Enabled:                  isEnabled,
		EventSubscriptionArn:     subArn,
		CustomerAwsID:            b.accountID,
		SubscriptionCreationTime: time.Now().UTC().Format(time.RFC3339),
	}
	b.eventSubscriptionPut(sub)
	if len(tags) > 0 {
		b.tagsStore(region)[subArn] = tagsFromMap(tags)
	}

	return copyEventSubscription(sub), nil
}

// DeleteEventSubscription deletes an event subscription.
func (b *InMemoryBackend) DeleteEventSubscription(ctx context.Context, name string) (*EventSubscription, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteEventSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrEventSubscriptionNotFound, name)
	}
	cp := copyEventSubscription(sub)
	b.eventSubscriptionDelete(region, name)
	delete(b.tagsStore(region), b.eventSubscriptionARN(region, name))

	return cp, nil
}

// DescribeEventSubscriptions returns event subscriptions, optionally filtered by name.
func (b *InMemoryBackend) DescribeEventSubscriptions(ctx context.Context, name string) []EventSubscription {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeEventSubscriptions")
	defer b.mu.RUnlock()
	if name != "" {
		sub, exists := b.eventSubscriptionGet(region, name)
		if !exists {
			return []EventSubscription{}
		}

		return []EventSubscription{*copyEventSubscription(sub)}
	}
	subStore := b.eventSubscriptionsInRegion(region)
	result := make([]EventSubscription, 0, len(subStore))
	for _, sub := range subStore {
		result = append(result, *copyEventSubscription(sub))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SubscriptionName < result[j].SubscriptionName
	})

	return result
}

// ModifyEventSubscription modifies an event subscription. enabled mirrors
// ModifyEventSubscriptionInput.Enabled -- nil means the caller didn't
// specify it (leave the existing value alone), matching this backend's
// existing convention for other optional-bool fields (see e.g.
// ModifyDBCluster's deletionProtection *bool).
func (b *InMemoryBackend) ModifyEventSubscription(
	ctx context.Context,
	name, snsTopicARN, sourceType string,
	eventCategories []string,
	enabled *bool,
) (*EventSubscription, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyEventSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrEventSubscriptionNotFound, name)
	}
	if snsTopicARN != "" {
		sub.SnsTopicARN = snsTopicARN
	}
	if sourceType != "" {
		sub.SourceType = sourceType
	}
	if len(eventCategories) > 0 {
		cats := make([]string, len(eventCategories))
		copy(cats, eventCategories)
		sub.EventCategories = cats
	}
	if enabled != nil {
		sub.Enabled = *enabled
	}

	return copyEventSubscription(sub), nil
}

// RemoveSourceIdentifierFromSubscription removes a source identifier from an event subscription.
func (b *InMemoryBackend) RemoveSourceIdentifierFromSubscription(
	ctx context.Context,
	subscriptionName, sourceID string,
) (*EventSubscription, error) {
	if subscriptionName == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	if sourceID == "" {
		return nil, fmt.Errorf("%w: SourceIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("RemoveSourceIdentifierFromSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, subscriptionName)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrEventSubscriptionNotFound, subscriptionName)
	}
	ids := make([]string, 0, len(sub.SourceIDs))
	for _, id := range sub.SourceIDs {
		if id != sourceID {
			ids = append(ids, id)
		}
	}
	sub.SourceIDs = ids

	return copyEventSubscription(sub), nil
}

// DescribeEventCategories returns the event categories for DocDB.
func (b *InMemoryBackend) DescribeEventCategories(_ context.Context, sourceType string) []EventCategoryMap {
	clusterCategories := []string{
		"availability", eventCatBackup, "configuration change",
		eventCatCreate, eventCatDelete, "failover", "maintenance", eventCatNotify,
	}
	instanceCategories := []string{
		"availability", eventCatBackup, "configuration change",
		eventCatCreate, eventCatDelete, "failover", "maintenance",
		eventCatNotify, "recovery", "restoration",
	}
	snapshotCategories := []string{
		eventCatBackup, eventCatCreate, eventCatDelete, eventCatNotify, "restoration",
	}
	all := []EventCategoryMap{
		{SourceType: "db-cluster", EventCategories: clusterCategories},
		{SourceType: "db-instance", EventCategories: instanceCategories},
		{SourceType: "db-cluster-snapshot", EventCategories: snapshotCategories},
	}
	if sourceType == "" {
		return all
	}
	for _, m := range all {
		if m.SourceType == sourceType {
			return []EventCategoryMap{m}
		}
	}

	return []EventCategoryMap{}
}
