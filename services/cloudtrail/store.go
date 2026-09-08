package cloudtrail

import (
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// InMemoryBackend is the in-memory store for CloudTrail resources.
type InMemoryBackend struct {
	s3               S3Backend
	dashboardsByARN  *store.Index[Dashboard]
	eventConfigs     map[string]*EventConfiguration
	trailsByARN      *store.Index[Trail]
	channels         *store.Table[Channel]
	channelsByARN    *store.Index[Channel]
	channelsByName   *store.Index[Channel]
	dashboards       *store.Table[Dashboard]
	queries          *store.Table[Query]
	queriesByAlias   *store.Index[Query]
	edsByName        *store.Index[EventDataStore]
	eventDataStores  *store.Table[EventDataStore]
	trails           *store.Table[Trail]
	mu               *lockmetrics.RWMutex
	dashboardsByName *store.Index[Dashboard]
	resourcePolicies *store.Table[ResourcePolicy]
	edsByARN         *store.Index[EventDataStore]
	registry         *store.Registry
	imports          *store.Table[Import]
	region           string
	accountID        string
	events           []Event
	channelCounter   int
	dashboardCounter int
	edsCounter       int
	queryCounter     int
	importCounter    int
}

// NewInMemoryBackend creates a new in-memory CloudTrail backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		accountID:    accountID,
		region:       region,
		mu:           lockmetrics.New("cloudtrail"),
		registry:     store.NewRegistry(),
		eventConfigs: make(map[string]*EventConfiguration),
	}

	registerAllTables(b)

	return b
}

// SetS3Backend wires S3 so CreateTrail/UpdateTrail validate the configured
// bucket exists and recorded management events are actually delivered as
// log files, instead of S3BucketName being stored/echoed with no
// validation and no delivery (gopherstack-g9b4).
func (b *InMemoryBackend) SetS3Backend(s3 S3Backend) {
	b.s3 = s3
}

// Reset clears all state in the backend.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, t := range b.trails.All() {
		t.Tags.Close()
	}
	for _, ch := range b.channels.All() {
		ch.Tags.Close()
	}
	for _, d := range b.dashboards.All() {
		d.Tags.Close()
	}
	for _, eds := range b.eventDataStores.All() {
		eds.Tags.Close()
	}

	b.registry.ResetAll()
	b.events = nil
	b.eventConfigs = make(map[string]*EventConfiguration)
	b.channelCounter = 0
	b.dashboardCounter = 0
	b.edsCounter = 0
	b.queryCounter = 0
	b.importCounter = 0
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns the AWS account ID this backend is configured for.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// findByNameOrARNLocked looks up a trail by name or ARN without locking.
func (b *InMemoryBackend) findByNameOrARNLocked(nameOrARN string) *Trail {
	if t, ok := b.trails.Get(nameOrARN); ok {
		return t
	}
	if matches := b.trailsByARN.Get(nameOrARN); len(matches) > 0 {
		return matches[0]
	}

	return nil
}

// findChannelLocked looks up a channel by ID or ARN without locking.
func (b *InMemoryBackend) findChannelLocked(idOrARN string) *Channel {
	if ch, ok := b.channels.Get(idOrARN); ok {
		return ch
	}
	if matches := b.channelsByARN.Get(idOrARN); len(matches) > 0 {
		return matches[0]
	}

	return nil
}

// findDashboardLocked looks up a dashboard by ID or ARN without locking.
func (b *InMemoryBackend) findDashboardLocked(idOrARN string) *Dashboard {
	if d, ok := b.dashboards.Get(idOrARN); ok {
		return d
	}
	if matches := b.dashboardsByARN.Get(idOrARN); len(matches) > 0 {
		return matches[0]
	}

	return nil
}

// findEventDataStoreLocked looks up an event data store by ID or ARN without locking.
func (b *InMemoryBackend) findEventDataStoreLocked(idOrARN string) *EventDataStore {
	if eds, ok := b.eventDataStores.Get(idOrARN); ok {
		return eds
	}
	if matches := b.edsByARN.Get(idOrARN); len(matches) > 0 {
		return matches[0]
	}

	return nil
}

func copyEventSelectors(in []EventSelector) []EventSelector {
	if len(in) == 0 {
		return nil
	}
	out := make([]EventSelector, len(in))
	copy(out, in)
	for i, es := range in {
		if es.DataResources != nil {
			out[i].DataResources = make([]DataResource, len(es.DataResources))
			copy(out[i].DataResources, es.DataResources)
		}
	}

	return out
}

func copyAdvancedEventSelectors(in []AdvancedEventSelector) []AdvancedEventSelector {
	if len(in) == 0 {
		return nil
	}
	out := make([]AdvancedEventSelector, len(in))
	for i, aes := range in {
		out[i].Name = aes.Name
		if aes.FieldSelectors != nil {
			out[i].FieldSelectors = make([]AdvancedFieldSelector, len(aes.FieldSelectors))
			for j, fs := range aes.FieldSelectors {
				out[i].FieldSelectors[j] = AdvancedFieldSelector{
					Field:         fs.Field,
					Equals:        copyStringSlice(fs.Equals),
					StartsWith:    copyStringSlice(fs.StartsWith),
					EndsWith:      copyStringSlice(fs.EndsWith),
					NotEquals:     copyStringSlice(fs.NotEquals),
					NotStartsWith: copyStringSlice(fs.NotStartsWith),
					NotEndsWith:   copyStringSlice(fs.NotEndsWith),
				}
			}
		}
	}

	return out
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)

	return out
}
