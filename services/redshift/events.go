package redshift

import (
	"fmt"
	"time"
	"unicode"
)

// LoggingStatus represents the logging status for a Redshift cluster.
type LoggingStatus struct {
	LastFailureTime time.Time `json:"lastFailureTime"`
	LastSuccessTime time.Time `json:"lastSuccessTime"`
	BucketName      string    `json:"bucketName"`
	S3KeyPrefix     string    `json:"s3KeyPrefix"`
	LoggingEnabled  bool      `json:"loggingEnabled"`
}

// Event represents a Redshift event.
type Event struct {
	Date             time.Time `json:"date"`
	SourceIdentifier string    `json:"sourceIdentifier"`
	SourceType       string    `json:"sourceType"`
	Message          string    `json:"message"`
	EventID          string    `json:"eventId"`
	Severity         string    `json:"severity"`
}

// EventSubscription represents a Redshift event subscription.
type EventSubscription struct {
	SubscriptionCreated time.Time `json:"subscriptionCreated"`
	CustomerAwsID       string    `json:"customerAwsId"`
	CustSubscriptionID  string    `json:"custSubscriptionId"`
	SnsTopicArn         string    `json:"snsTopicArn"`
	Status              string    `json:"status"`
	SourceType          string    `json:"sourceType"`
	Severity            string    `json:"severity"`
	SourceIDs           []string  `json:"sourceIds"`
	EventCategories     []string  `json:"eventCategories"`
	Enabled             bool      `json:"enabled"`
}

// invalidS3KeyPrefixRune returns the first rune in s that falls outside the
// character set documented on EnableLoggingInput.S3KeyPrefix (redshift@v1.65.4
// api_op_EnableLogging.go): "Valid characters are any letter from any
// language, any whitespace character, any numeric character, and the
// following characters: underscore ( _ ), period ( . ), colon ( : ), slash (
// / ), equal ( = ), plus ( + ), backslash ( \ ), hyphen ( - ), at symbol ( @
// )." Returns 0 if every rune is valid.
func invalidS3KeyPrefixRune(s string) rune {
	for _, r := range s {
		switch r {
		case '_', '.', ':', '/', '=', '+', '\\', '-', '@':
			continue
		}
		if unicode.IsLetter(r) || unicode.IsSpace(r) || unicode.IsNumber(r) {
			continue
		}

		return r
	}

	return 0
}

// EnableLogging enables audit logging for the specified cluster.
func (b *InMemoryBackend) EnableLogging(clusterID, bucketName, s3KeyPrefix string) (*LoggingStatus, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}
	if bucketName == "" {
		return nil, fmt.Errorf("%w: BucketName is required", ErrInvalidParameter)
	}
	if r := invalidS3KeyPrefixRune(s3KeyPrefix); r != 0 {
		return nil, fmt.Errorf("%w: S3KeyPrefix contains invalid character %q", ErrInvalidS3KeyPrefix, r)
	}

	b.mu.Lock("EnableLogging")
	defer b.mu.Unlock()

	if _, exists := b.clusters.Get(clusterID); !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	status := &LoggingStatus{
		LoggingEnabled:  true,
		BucketName:      bucketName,
		S3KeyPrefix:     s3KeyPrefix,
		LastSuccessTime: time.Now(),
	}
	b.loggingStatuses[clusterID] = status

	cp := *status

	return &cp, nil
}

// DisableLogging disables audit logging for the specified cluster.
func (b *InMemoryBackend) DisableLogging(clusterID string) (*LoggingStatus, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.Lock("DisableLogging")
	defer b.mu.Unlock()

	if _, exists := b.clusters.Get(clusterID); !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	status := &LoggingStatus{
		LoggingEnabled: false,
	}
	b.loggingStatuses[clusterID] = status

	cp := *status

	return &cp, nil
}

// GetLoggingStatus returns the current logging status for the specified cluster.
func (b *InMemoryBackend) GetLoggingStatus(clusterID string) (*LoggingStatus, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("%w: ClusterIdentifier is required", ErrInvalidParameter)
	}

	b.mu.RLock("GetLoggingStatus")
	defer b.mu.RUnlock()

	if _, exists := b.clusters.Get(clusterID); !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	status, ok := b.loggingStatuses[clusterID]
	if !ok {
		return &LoggingStatus{}, nil
	}

	cp := *status

	return &cp, nil
}

// DescribeEvents returns events for a Redshift resource.
// This in-memory implementation returns an empty list since events are not tracked.
func (b *InMemoryBackend) DescribeEvents(sourceIdentifier, sourceType string) ([]Event, error) {
	b.mu.RLock("DescribeEvents")
	defer b.mu.RUnlock()

	var result []Event

	for _, e := range b.events.All() {
		if sourceIdentifier != "" && e.SourceIdentifier != sourceIdentifier {
			continue
		}

		if sourceType != "" && e.SourceType != sourceType {
			continue
		}

		result = append(result, *e)
	}

	return result, nil
}

// CreateEventSubscription creates a new event notification subscription.
func (b *InMemoryBackend) CreateEventSubscription(
	subscriptionName, snsTopicArn, sourceType, severity string,
	sourceIDs, eventCategories []string,
	enabled bool,
) (*EventSubscription, error) {
	if subscriptionName == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}
	if snsTopicArn == "" {
		return nil, fmt.Errorf("%w: SnsTopicArn is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateEventSubscription")
	defer b.mu.Unlock()

	if _, exists := b.eventSubscriptions.Get(subscriptionName); exists {
		return nil, fmt.Errorf(
			"%w: subscription %s already exists",
			ErrEventSubscriptionAlreadyExists,
			subscriptionName,
		)
	}

	srcIDs := make([]string, len(sourceIDs))
	copy(srcIDs, sourceIDs)

	evtCats := make([]string, len(eventCategories))
	copy(evtCats, eventCategories)

	sub := &EventSubscription{
		CustomerAwsID:       b.accountID,
		CustSubscriptionID:  subscriptionName,
		SnsTopicArn:         snsTopicArn,
		Status:              endpointStatusActive,
		SubscriptionCreated: time.Now(),
		SourceType:          sourceType,
		SourceIDs:           srcIDs,
		EventCategories:     evtCats,
		Severity:            severity,
		Enabled:             enabled,
	}
	b.eventSubscriptions.Put(sub)

	cp := cloneEventSubscription(sub)

	return cp, nil
}

// DeleteEventSubscription removes an event subscription.
func (b *InMemoryBackend) DeleteEventSubscription(subscriptionName string) error {
	if subscriptionName == "" {
		return fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteEventSubscription")
	defer b.mu.Unlock()

	if _, exists := b.eventSubscriptions.Get(subscriptionName); !exists {
		return fmt.Errorf("%w: subscription %s not found", ErrEventSubscriptionNotFound, subscriptionName)
	}

	b.eventSubscriptions.Delete(subscriptionName)

	return nil
}

// DescribeEventSubscriptions returns event subscriptions, optionally filtered by name.
func (b *InMemoryBackend) DescribeEventSubscriptions(subscriptionName string) ([]EventSubscription, error) {
	b.mu.RLock("DescribeEventSubscriptions")
	defer b.mu.RUnlock()

	if subscriptionName != "" {
		sub, exists := b.eventSubscriptions.Get(subscriptionName)
		if !exists {
			return nil, fmt.Errorf("%w: subscription %s not found", ErrEventSubscriptionNotFound, subscriptionName)
		}

		return []EventSubscription{*cloneEventSubscription(sub)}, nil
	}

	result := make([]EventSubscription, 0, b.eventSubscriptions.Len())
	for _, sub := range b.eventSubscriptions.All() {
		result = append(result, *cloneEventSubscription(sub))
	}

	return result, nil
}

// ModifyEventSubscription modifies an existing event subscription.
func (b *InMemoryBackend) ModifyEventSubscription(
	subscriptionName, snsTopicArn, sourceType, severity string,
	sourceIDs, eventCategories []string,
	enabled *bool,
) (*EventSubscription, error) {
	if subscriptionName == "" {
		return nil, fmt.Errorf("%w: SubscriptionName is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyEventSubscription")
	defer b.mu.Unlock()

	sub, exists := b.eventSubscriptions.Get(subscriptionName)
	if !exists {
		return nil, fmt.Errorf("%w: subscription %s not found", ErrEventSubscriptionNotFound, subscriptionName)
	}

	if snsTopicArn != "" {
		sub.SnsTopicArn = snsTopicArn
	}

	if sourceType != "" {
		sub.SourceType = sourceType
	}

	if severity != "" {
		sub.Severity = severity
	}

	if len(sourceIDs) > 0 {
		sub.SourceIDs = make([]string, len(sourceIDs))
		copy(sub.SourceIDs, sourceIDs)
	}

	if len(eventCategories) > 0 {
		sub.EventCategories = make([]string, len(eventCategories))
		copy(sub.EventCategories, eventCategories)
	}

	if enabled != nil {
		sub.Enabled = *enabled
	}

	return cloneEventSubscription(sub), nil
}

// cloneEventSubscription returns a deep copy of an EventSubscription.
func cloneEventSubscription(sub *EventSubscription) *EventSubscription {
	cp := *sub
	cp.SourceIDs = make([]string, len(sub.SourceIDs))
	copy(cp.SourceIDs, sub.SourceIDs)
	cp.EventCategories = make([]string, len(sub.EventCategories))
	copy(cp.EventCategories, sub.EventCategories)

	return &cp
}
