package mediatailor

import (
	"maps"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	defaultMaxResults = 20

	channelStateRunning = "RUNNING"
	channelStateStopped = "STOPPED"

	playbackModeLinear = "LINEAR"
	playbackModeLoop   = "LOOP"

	tierBasic    = "BASIC"
	tierStandard = "STANDARD"

	logTypeAsRun = "AS_RUN"

	loggingStrategyVendedLogs       = "VENDED_LOGS"
	loggingStrategyLegacyCloudwatch = "LEGACY_CLOUDWATCH"

	resourceTypePlaybackConfiguration = "playbackConfiguration"
	resourceTypeChannel               = "channel"
	resourceTypeSourceLocation        = "sourceLocation"
	resourceTypeVodSource             = "vodSource"
)

// InMemoryBackend is an in-memory implementation of StorageBackend.
type InMemoryBackend struct {
	mu       *lockmetrics.RWMutex
	registry *store.Registry

	playbackConfigurations *store.Table[storedPlaybackConfiguration]
	channels               *store.Table[storedChannel]
	channelPolicies        map[string]string
	sourceLocations        *store.Table[storedSourceLocation]

	vodSources           *store.Table[storedVodSource]
	vodSourcesByLocation *store.Index[storedVodSource]

	liveSources           *store.Table[LiveSource]
	liveSourcesByLocation *store.Index[LiveSource]

	prefetchSchedules         *store.Table[PrefetchSchedule]
	prefetchSchedulesByConfig *store.Index[PrefetchSchedule]

	programs          *store.Table[Program]
	programsByChannel *store.Index[Program]

	functions *store.Table[Function]

	tags map[string]map[string]string

	accountID string
	region    string
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		mu:              lockmetrics.New("mediatailor"),
		registry:        store.NewRegistry(),
		channelPolicies: make(map[string]string),
		tags:            make(map[string]map[string]string),
		accountID:       accountID,
		region:          region,
	}
	registerAllTables(b)

	return b
}

// AccountID returns the configured account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the configured region.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all stored data.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.channelPolicies = make(map[string]string)
	b.tags = make(map[string]map[string]string)
}

func (b *InMemoryBackend) playbackConfigARN(name string) string {
	return arn.Build("mediatailor", b.region, b.accountID, resourceTypePlaybackConfiguration+"/"+name)
}

func (b *InMemoryBackend) channelARN(name string) string {
	return arn.Build("mediatailor", b.region, b.accountID, resourceTypeChannel+"/"+name)
}

func (b *InMemoryBackend) sourceLocationARN(name string) string {
	return arn.Build("mediatailor", b.region, b.accountID, resourceTypeSourceLocation+"/"+name)
}

func (b *InMemoryBackend) vodSourceARN(sourceLocationName, vodSourceName string) string {
	return arn.Build(
		"mediatailor",
		b.region,
		b.accountID,
		resourceTypeSourceLocation+"/"+sourceLocationName+"/"+resourceTypeVodSource+"/"+vodSourceName,
	)
}

func copyTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return make(map[string]string)
	}

	result := make(map[string]string, len(tags))
	maps.Copy(result, tags)

	return result
}
