package scheduler

import "context"

// StorageBackend defines the interface for EventBridge Scheduler backend implementations.
// All mutating methods must be safe for concurrent use. The region for each operation is
// resolved from the supplied context (falling back to the backend's default region).
type StorageBackend interface {
	// Schedule operations
	CreateSchedule(
		ctx context.Context,
		name, groupName, expr, description, timezone string,
		target Target,
		state string,
		ftw FlexibleTimeWindow,
		opts ...ScheduleOption,
	) (*Schedule, error)
	GetSchedule(ctx context.Context, name, groupName string) (*Schedule, error)
	ListSchedules(
		ctx context.Context,
		groupName, namePrefix, state, nextToken string,
		maxResults int,
	) ([]*Schedule, string)
	DeleteSchedule(ctx context.Context, name, groupName string) error
	UpdateSchedule(
		ctx context.Context,
		name, groupName, expr, description, timezone string,
		target Target,
		state string,
		ftw FlexibleTimeWindow,
		opts ...ScheduleOption,
	) (*Schedule, error)

	// Schedule group operations
	CreateScheduleGroup(
		ctx context.Context,
		name string,
		initialTags map[string]string,
	) (*ScheduleGroup, error)
	GetScheduleGroup(ctx context.Context, name string) (*ScheduleGroup, error)
	DeleteScheduleGroup(ctx context.Context, name string) error
	ListScheduleGroups(ctx context.Context, namePrefix, nextToken string, maxResults int) ([]*ScheduleGroup, string)

	// Tag operations
	TagResource(ctx context.Context, resourceARN string, kv map[string]string) error
	UntagResource(ctx context.Context, resourceARN string, tagKeys []string) error
	ListTagsForResource(ctx context.Context, resourceARN string) (map[string]string, error)

	// Lifecycle
	Reset()
	Region() string
	AccountID() string
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// compile-time assertion that InMemoryBackend satisfies StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)
