package neptune

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) eventSubscriptionGet(region, name string) (*EventSubscription, bool) {
	return b.eventSubscriptions.Get(regionKey(region, name))
}

func (b *InMemoryBackend) eventSubscriptionHas(region, name string) bool {
	return b.eventSubscriptions.Has(regionKey(region, name))
}

func (b *InMemoryBackend) eventSubscriptionPut(v *EventSubscription) { b.eventSubscriptions.Put(v) }

func (b *InMemoryBackend) eventSubscriptionDelete(region, name string) {
	b.eventSubscriptions.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) eventSubscriptionsInRegion(region string) []*EventSubscription {
	return b.eventSubscriptionsByRegion.Get(region)
}

// cloneEventSubscription returns a deep copy of an event subscription (with its slices copied).
func cloneEventSubscription(sub *EventSubscription) EventSubscription {
	cp := *sub
	cp.SourceIDs = make([]string, len(sub.SourceIDs))
	copy(cp.SourceIDs, sub.SourceIDs)
	cp.EventCategoriesList = make([]string, len(sub.EventCategoriesList))
	copy(cp.EventCategoriesList, sub.EventCategoriesList)

	return cp
}

// eventSubscriptionARN returns the region-scoped ARN for a Neptune event subscription.
func (b *InMemoryBackend) eventSubscriptionARN(region, name string) string {
	return arn.Build("rds", region, b.accountID, "es:"+name)
}

// AddSourceIdentifierToSubscription adds a source identifier to an event subscription.
func (b *InMemoryBackend) AddSourceIdentifierToSubscription(
	ctx context.Context, name, sourceID string,
) (*EventSubscription, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	if sourceID == "" {
		return nil, fmt.Errorf("%w: SourceIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("AddSourceIdentifierToSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrSubscriptionNotFound, name)
	}
	if !slices.Contains(sub.SourceIDs, sourceID) {
		sub.SourceIDs = append(sub.SourceIDs, sourceID)
	}
	cp := *sub
	cp.SourceIDs = make([]string, len(sub.SourceIDs))
	copy(cp.SourceIDs, sub.SourceIDs)

	return &cp, nil
}

// CreateEventSubscription creates a Neptune event notification subscription.
func (b *InMemoryBackend) CreateEventSubscription(
	ctx context.Context,
	name, snsTopicARN, sourceType string,
	sourceIDs, eventCategories []string,
	enabled bool,
) (*EventSubscription, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	if snsTopicARN == "" {
		return nil, fmt.Errorf("%w: SnsTopicArn is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateEventSubscription")
	defer b.mu.Unlock()
	if b.eventSubscriptionHas(region, name) {
		return nil, fmt.Errorf(
			"%w: subscription %s already exists",
			ErrSubscriptionAlreadyExists,
			name,
		)
	}
	ids := make([]string, len(sourceIDs))
	copy(ids, sourceIDs)
	cats := make([]string, len(eventCategories))
	copy(cats, eventCategories)
	sub := &EventSubscription{
		region:               region,
		CustSubscriptionID:   name,
		SnsTopicARN:          snsTopicARN,
		EventSubscriptionArn: b.eventSubscriptionARN(region, name),
		Status:               subscriptionStatusActive,
		SourceType:           sourceType,
		SourceIDs:            ids,
		EventCategoriesList:  cats,
		Enabled:              enabled,
		CustomerAwsID:        b.accountID,
	}
	b.eventSubscriptionPut(sub)
	cp := cloneEventSubscription(sub)

	return &cp, nil
}

// DeleteEventSubscription deletes a Neptune event subscription.
func (b *InMemoryBackend) DeleteEventSubscription(
	ctx context.Context,
	name string,
) (*EventSubscription, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteEventSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrSubscriptionNotFound, name)
	}
	cp := *sub
	cp.SourceIDs = make([]string, len(sub.SourceIDs))
	copy(cp.SourceIDs, sub.SourceIDs)
	b.eventSubscriptionDelete(region, name)
	delete(b.tagsStore(region), b.eventSubscriptionARN(region, name))

	return &cp, nil
}

// DescribeEventSubscriptions returns all event subscriptions or a specific one.
func (b *InMemoryBackend) DescribeEventSubscriptions(
	ctx context.Context,
	name string,
) ([]EventSubscription, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeEventSubscriptions")
	defer b.mu.RUnlock()
	if name != "" {
		sub, exists := b.eventSubscriptionGet(region, name)
		if !exists {
			return nil, fmt.Errorf("%w: subscription %s not found", ErrSubscriptionNotFound, name)
		}

		return []EventSubscription{cloneEventSubscription(sub)}, nil
	}
	subs := b.eventSubscriptionsInRegion(region)
	result := make([]EventSubscription, 0, len(subs))
	for _, sub := range subs {
		result = append(result, cloneEventSubscription(sub))
	}
	slices.SortFunc(result, func(a, b EventSubscription) int {
		return strings.Compare(a.CustSubscriptionID, b.CustSubscriptionID)
	})

	return result, nil
}

// ModifyEventSubscription modifies a Neptune event subscription.
func (b *InMemoryBackend) ModifyEventSubscription(
	ctx context.Context,
	name, snsTopicARN, sourceType, enabled string,
	eventCategories []string,
) (*EventSubscription, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyEventSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrSubscriptionNotFound, name)
	}
	if snsTopicARN != "" {
		sub.SnsTopicARN = snsTopicARN
	}
	if sourceType != "" {
		sub.SourceType = sourceType
	}
	switch enabled {
	case "true":
		sub.Enabled = true
	case "false":
		sub.Enabled = false
	}
	if len(eventCategories) > 0 {
		cats := make([]string, len(eventCategories))
		copy(cats, eventCategories)
		sub.EventCategoriesList = cats
	}
	cp := cloneEventSubscription(sub)

	return &cp, nil
}

// RemoveSourceIdentifierFromSubscription removes a source identifier from a Neptune event subscription.
func (b *InMemoryBackend) RemoveSourceIdentifierFromSubscription(
	ctx context.Context, name, sourceID string,
) (*EventSubscription, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	if sourceID == "" {
		return nil, fmt.Errorf("%w: SourceIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("RemoveSourceIdentifierFromSubscription")
	defer b.mu.Unlock()
	sub, exists := b.eventSubscriptionGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrSubscriptionNotFound, name)
	}
	kept := make([]string, 0, len(sub.SourceIDs))
	for _, id := range sub.SourceIDs {
		if id != sourceID {
			kept = append(kept, id)
		}
	}
	sub.SourceIDs = kept
	cp := *sub
	cp.SourceIDs = make([]string, len(sub.SourceIDs))
	copy(cp.SourceIDs, sub.SourceIDs)

	return &cp, nil
}

// AddEventSubscriptionInternal creates an event subscription directly. Used for seeding tests.
func (b *InMemoryBackend) AddEventSubscriptionInternal(
	name, snsTopicARN string,
) *EventSubscription {
	b.mu.Lock("AddEventSubscriptionInternal")
	defer b.mu.Unlock()
	sub := &EventSubscription{
		region:             b.region,
		CustSubscriptionID: name,
		SnsTopicARN:        snsTopicARN,
		Status:             subscriptionStatusActive,
	}
	b.eventSubscriptionPut(sub)
	cp := *sub

	return &cp
}
