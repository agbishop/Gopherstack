package eventbridge

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	stateActive           = "ACTIVE"
	replayStateStarting   = "STARTING"
	replayStateCancelling = "CANCELLING"
	replayStateCancelled  = "CANCELLED"
	replayStateCompleted  = "COMPLETED"
)

// regionContextKey is the context key for the per-request AWS region.
type regionContextKey struct{}

// getRegionFromContext extracts the region from context, falling back to defaultRegion.
func getRegionFromContext(ctx context.Context, defaultRegion string) string {
	if region, ok := ctx.Value(regionContextKey{}).(string); ok && region != "" {
		return region
	}

	return defaultRegion
}

// ebBusKey returns the region-free inner key used to store a bus, its rules and
// its rule index within a region-scoped map. The region is the outer map key, so
// this key only needs to disambiguate buses within a single region.
func ebBusKey(busName string) string {
	if busName == "" {
		busName = defaultEventBusName
	}

	return busName
}

var (
	ErrEventBusNotFound       = errors.New("ResourceNotFoundException")
	ErrEventBusAlreadyExists  = errors.New("ResourceAlreadyExistsException")
	ErrRuleNotFound           = errors.New("ResourceNotFoundException")
	ErrCannotDeleteDefaultBus = errors.New("IllegalArgumentException")
	ErrInvalidParameter       = errors.New("InvalidParameterException")
	ErrNotFound               = errors.New("ResourceNotFoundException")
	ErrAlreadyExists          = errors.New("ResourceAlreadyExistsException")
	ErrInvalidState           = errors.New("InvalidStateException")
	// ErrReplayNotCancellable is returned when CancelReplay targets a replay
	// that has already reached a terminal state. CancelReplay's own
	// deserializeOpError switch declares IllegalStatusException, not
	// InvalidStateException (which IS correct for
	// ActivateEventSource/CreateEventBus/DeactivateEventSource, ErrInvalidState's
	// legitimate callers) -- verified against eventbridge deserializers.go.
	ErrReplayNotCancellable  = errors.New("IllegalStatusException")
	ErrResourceLimitExceeded = errors.New("LimitExceededException")
	// ErrForbiddenOperation is returned when an operation is forbidden (e.g., modifying built-in registries).
	ErrForbiddenOperation = errors.New("ForbiddenException")
	// ErrManagedRule is returned when an operation attempts to modify a rule
	// that is owned/managed by an AWS service (Rule.ManagedBy is non-empty).
	// Matches real AWS's ManagedRuleException.
	ErrManagedRule = errors.New("ManagedRuleException")
)

const (
	defaultEventBusName = "default"
	maxEventLogSize     = 1000
	ruleStateEnabled    = "ENABLED"
	ruleStateDisabled   = "DISABLED"
	// ruleStateEnabledAllCloudTrailMgmtEvents is RuleState's third value (AWS
	// SDK types.RuleState.Values(), aws-sdk-go-v2/service/eventbridge
	// types/enums.go): used for a rule that also matches CloudTrail
	// management events on the account's default event bus.
	ruleStateEnabledAllCloudTrailMgmtEvents = "ENABLED_WITH_ALL_CLOUDTRAIL_MANAGEMENT_EVENTS"
	defaultDeliveryWorkers                  = 10
	// defaultShutdownTimeout is the maximum time Close waits for in-flight delivery
	// goroutines to finish after cancelling the lifecycle context.
	defaultShutdownTimeout = 5 * time.Second
	// defaultDeliveryTimeout is the default maximum time allowed for a single target delivery call.
	defaultDeliveryTimeout = 30 * time.Second
	ruleIndexAny           = "\x00"

	// maxEventBusNameLength is the maximum allowed event bus name length (AWS limit).
	maxEventBusNameLength = 256
	// maxArchiveNameLength is the maximum allowed archive name length (AWS limit).
	maxArchiveNameLength = 48
	// maxTargetsPerRule is the maximum number of targets allowed per rule (AWS default limit).
	maxTargetsPerRule = 5
	// maxEventBusesPerAccount is the AWS limit for custom event buses per account.
	maxEventBusesPerAccount = 200
	// maxRulesPerBus is the AWS limit for rules per event bus.
	maxRulesPerBus = 300

	// putTargetsFailedEntryErrorCode is the FailedEntry.ErrorCode PutTargets
	// uses for every per-target validation failure (bad Id, InputTransformer,
	// target-type-specific parameters, RetryPolicy bounds).
	putTargetsFailedEntryErrorCode = "InvalidParameter"
)

type ruleIndexKey struct {
	source     string
	detailType string
}

// StorageBackend is the interface for an EventBridge in-memory store.
type StorageBackend interface {
	CreateEventBus(ctx context.Context, params CreateEventBusParams) (*EventBus, error)
	DeleteEventBus(ctx context.Context, name string) error
	ListEventBuses(
		ctx context.Context,
		namePrefix, nextToken string,
		limit int,
	) ([]EventBus, string, error)
	DescribeEventBus(ctx context.Context, name string) (*EventBus, error)
	PutRule(ctx context.Context, input PutRuleInput) (*Rule, error)
	DeleteRule(ctx context.Context, name, eventBusName string) error
	ListRules(
		ctx context.Context,
		eventBusName, namePrefix, nextToken string,
		limit int,
	) ([]Rule, string, error)
	DescribeRule(ctx context.Context, name, eventBusName string) (*Rule, error)
	EnableRule(ctx context.Context, name, eventBusName string) error
	DisableRule(ctx context.Context, name, eventBusName string) error
	PutTargets(
		ctx context.Context,
		ruleName, eventBusName string,
		targets []Target,
	) ([]FailedEntry, error)
	RemoveTargets(
		ctx context.Context,
		ruleName, eventBusName string,
		ids []string,
	) ([]FailedEntry, error)
	ListTargetsByRule(
		ctx context.Context,
		ruleName, eventBusName, nextToken string,
		limit int,
	) ([]Target, string, error)
	PutEvents(ctx context.Context, entries []EventEntry) ([]EventResultEntry, error)
	GetEventLog(ctx context.Context) []EventLogEntry
	ActivateEventSource(ctx context.Context, name string) error
	DeactivateEventSource(ctx context.Context, name string) error
	CreatePartnerEventSource(ctx context.Context, name, account string) (*PartnerEventSource, error)
	CancelReplay(ctx context.Context, replayName string) (*Replay, error)
	CreateAPIDestination(
		ctx context.Context,
		input CreateAPIDestinationInput,
	) (*APIDestination, error)
	CreateArchive(ctx context.Context, input CreateArchiveInput) (*Archive, error)
	CreateConnection(ctx context.Context, input CreateConnectionInput) (*Connection, error)
	CreateEndpoint(ctx context.Context, input CreateEndpointInput) (*Endpoint, error)
	DeauthorizeConnection(ctx context.Context, name string) (*Connection, error)
	DeleteAPIDestination(ctx context.Context, name string) error
	DeleteArchive(ctx context.Context, name string) error
	DescribeArchive(ctx context.Context, name string) (*Archive, error)
	ListArchives(
		ctx context.Context, namePrefix, eventSourceArn, state, nextToken string, limit int,
	) ([]Archive, string, error)
	UpdateArchive(ctx context.Context, input UpdateArchiveInput) (*Archive, error)
	DeleteConnection(ctx context.Context, name string) error
	DescribeConnection(ctx context.Context, name string) (*Connection, error)
	ListConnections(
		ctx context.Context, namePrefix, connectionState, nextToken string, limit int,
	) ([]Connection, string, error)
	UpdateConnection(ctx context.Context, input UpdateConnectionInput) (*Connection, error)
	DeleteEndpoint(ctx context.Context, name string) error
	DescribeEndpoint(ctx context.Context, name string) (*Endpoint, error)
	ListEndpoints(
		ctx context.Context,
		namePrefix, nextToken string,
		limit int,
	) ([]Endpoint, string, error)
	UpdateEndpoint(ctx context.Context, input UpdateEndpointInput) (*Endpoint, error)
	DescribeAPIDestination(ctx context.Context, name string) (*APIDestination, error)
	ListAPIDestinations(
		ctx context.Context, namePrefix, connectionArn, nextToken string, limit int,
	) ([]APIDestination, string, error)
	UpdateAPIDestination(
		ctx context.Context,
		input UpdateAPIDestinationInput,
	) (*APIDestination, error)
	DescribeEventSource(ctx context.Context, name string) (*EventSource, error)
	ListEventSources(
		ctx context.Context,
		namePrefix, nextToken string,
		limit int,
	) ([]EventSource, string, error)
	DescribePartnerEventSource(ctx context.Context, name string) (*PartnerEventSource, error)
	DeletePartnerEventSource(ctx context.Context, name string) error
	ListPartnerEventSources(
		ctx context.Context,
		namePrefix, nextToken string,
		limit int,
	) ([]PartnerEventSource, string, error)
	ListPartnerEventSourceAccounts(
		ctx context.Context,
		eventSourceName string,
	) ([]PartnerEventSourceAccountInfo, error)
	PutPartnerEvents(ctx context.Context, entries []EventEntry) ([]EventResultEntry, error)
	DescribeReplay(ctx context.Context, name string) (*Replay, error)
	ListReplays(
		ctx context.Context, namePrefix, eventSourceArn, state, nextToken string, limit int,
	) ([]Replay, string, error)
	StartReplay(ctx context.Context, input StartReplayInput) (*Replay, error)
	ListRuleNamesByTarget(
		ctx context.Context, targetARN, eventBusName, nextToken string, limit int,
	) ([]string, string, error)
	TestEventPattern(ctx context.Context, pattern, event string) (bool, error)
	UpdateEventBus(ctx context.Context, input UpdateEventBusInput) (*EventBus, error)
	PutPermission(ctx context.Context, input PutPermissionInput) error
	RemovePermission(ctx context.Context, input RemovePermissionInput) error
	GetEventBusPolicy(ctx context.Context, eventBusName string) (string, error)
	PutEventBusPolicy(ctx context.Context, input PutEventBusPolicyInput) error
	// Schema Registry operations.
	CreateRegistry(ctx context.Context, input CreateRegistryInput) (*SchemaRegistry, error)
	DeleteRegistry(ctx context.Context, registryName string) error
	DescribeRegistry(ctx context.Context, registryName string) (*SchemaRegistry, error)
	ListRegistries(
		ctx context.Context,
		namePrefix, nextToken string,
		limit int,
	) ([]SchemaRegistry, string, error)
	UpdateRegistry(ctx context.Context, input UpdateRegistryInput) (*SchemaRegistry, error)
	CreateSchema(ctx context.Context, input CreateSchemaInput) (*Schema, error)
	DeleteSchema(ctx context.Context, registryName, schemaName string) error
	DescribeSchema(
		ctx context.Context,
		registryName, schemaName, schemaVersion string,
	) (*Schema, error)
	ListSchemas(
		ctx context.Context,
		registryName, namePrefix, nextToken string,
		limit int,
	) ([]Schema, string, error)
	SearchSchemas(
		ctx context.Context,
		registryName, keywords, nextToken string,
		limit int,
	) ([]Schema, string, error)
	UpdateSchema(ctx context.Context, input UpdateSchemaInput) (*Schema, error)
	ListSchemaVersions(
		ctx context.Context, registryName, schemaName, nextToken string, limit int,
	) ([]SchemaVersion, string, error)
	DescribeSchemaVersion(
		ctx context.Context,
		registryName, schemaName, schemaVersion string,
	) (*SchemaVersion, error)
	DeleteSchemaVersion(ctx context.Context, registryName, schemaName, schemaVersion string) error
	GetDiscoveredSchema(ctx context.Context, input GetDiscoveredSchemaInput) (string, error)
	PutCodeBinding(ctx context.Context, input PutCodeBindingInput) (*CodeBinding, error)
	DescribeCodeBinding(ctx context.Context, input DescribeCodeBindingInput) (*CodeBinding, error)
	ListCodeBindings(
		ctx context.Context,
		input ListCodeBindingsInput,
	) ([]CodeBinding, string, error)
	GetCodeBindingSource(
		ctx context.Context,
		registryName, schemaName, language, schemaVersion string,
	) (string, error)
}

// InMemoryBackend implements StorageBackend using in-memory maps.
type InMemoryBackend struct {
	ctx context.Context
	mu  *lockmetrics.RWMutex
	// registry is the lifecycle registry for every PERSISTED *store.Table
	// below -- see store_setup.go's package doc for why eventbridge
	// (region-scoped, with rules/targets nested one level deeper still)
	// needs the lazy getOrCreateTable/getOrCreateNestedTable/
	// getOrCreateGlobalTable helpers rather than the flat static
	// registration ec2/sqs use. persistence.go drives Snapshot/Restore
	// through this registry only.
	registry *store.Registry
	// auxRegistry holds registries/schemas: also *store.Table-backed, but
	// deliberately NOT snapshotted (see store_setup.go's package doc) -- they
	// were never part of backendSnapshot before this conversion, and this
	// preserves that byte-for-byte.
	auxRegistry *store.Registry
	// Region-isolated stores. The outer key is the AWS region; the leaf
	// *store.Table is keyed by the resource's own identity field (bus name,
	// rule name, resource name, etc), matching the map key it replaces.
	connections     map[string]*store.Table[Connection]
	rules           map[string]map[string]*store.Table[Rule]
	targets         map[string]map[string]*store.Table[Target]
	eventSources    map[string]*store.Table[EventSource]
	replays         map[string]*store.Table[Replay]
	apiDestinations map[string]*store.Table[APIDestination]
	cancel          context.CancelFunc
	deliveryTargets *DeliveryTargets
	endpoints       map[string]*store.Table[Endpoint]
	buses           map[string]*store.Table[EventBus]
	partnerSources  map[string]*store.Table[PartnerEventSource]
	archives        map[string]*store.Table[Archive]
	archivedEvents  map[string]map[string][]EventEntry
	busePolicies    map[string]map[string]*EventBusPolicy
	// registries is NOT region-scoped -- a single backend holds one global
	// SchemaRegistry catalogue -- so it is a single Table, lazily registered
	// by getOrCreateGlobalTable (see store_setup.go).
	registries *store.Table[SchemaRegistry]
	// schemas is keyed by registryName (also global, not region-scoped, but
	// one dynamic dimension deep like a per-region resource).
	schemas        map[string]*store.Table[Schema]
	schemaVersions map[string][]*SchemaVersion // "registryName/schemaName" → ordered versions
	codeBindings   map[string]*CodeBinding     // "registryName/schemaName/language" → binding
	workerSem      chan struct{}
	ruleIndex      map[string]map[string]map[ruleIndexKey]map[string]*Rule
	// targetsByARN indexes (region → ARN → set of "busKey/ruleName" targetKeys)
	// for O(1) ListRuleNamesByTarget lookups. Kept consistent on PutTargets /
	// RemoveTargets / DeleteRule / DeleteEventBus / Reset.
	targetsByARN map[string]map[string]map[string]struct{}
	patternCache sync.Map
	// apiDestLimiters holds per-destination-ARN rate limiters (*apiDestLimiter)
	// used to honour each API destination's InvocationRateLimitPerSecond.
	apiDestLimiters sync.Map
	region          string
	accountID       string
	eventLog        []EventLogEntry
	wg              sync.WaitGroup
	shutdownTimeout time.Duration
	deliveryTimeout time.Duration
	tableMu         sync.Mutex
	closing         atomic.Bool
}

// NewInMemoryBackend creates a new InMemoryBackend with default configuration.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(config.DefaultAccountID, config.DefaultRegion)
}

// NewInMemoryBackendWithConfig creates a new InMemoryBackend with given account and region.
// The backend's lifecycle context is derived from [context.Background]; use
// NewInMemoryBackendWithContext to bind it to a parent service context instead.
func NewInMemoryBackendWithConfig(accountID, region string) *InMemoryBackend {
	return NewInMemoryBackendWithContext(context.Background(), accountID, region)
}

// NewInMemoryBackendWithContext creates a new InMemoryBackend whose lifecycle context
// is derived from the provided parent. When the parent is cancelled (e.g. on server
// shutdown), all in-flight delivery workers are also cancelled.
// If svcCtx is nil, [context.Background] is used.
func NewInMemoryBackendWithContext(
	svcCtx context.Context,
	accountID, region string,
) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	ctx, cancel := context.WithCancel(svcCtx)
	b := &InMemoryBackend{
		accountID:       accountID,
		region:          region,
		registry:        store.NewRegistry(),
		auxRegistry:     store.NewRegistry(),
		buses:           make(map[string]*store.Table[EventBus]),
		rules:           make(map[string]map[string]*store.Table[Rule]),
		targets:         make(map[string]map[string]*store.Table[Target]),
		eventSources:    make(map[string]*store.Table[EventSource]),
		replays:         make(map[string]*store.Table[Replay]),
		apiDestinations: make(map[string]*store.Table[APIDestination]),
		archives:        make(map[string]*store.Table[Archive]),
		archivedEvents:  make(map[string]map[string][]EventEntry),
		connections:     make(map[string]*store.Table[Connection]),
		endpoints:       make(map[string]*store.Table[Endpoint]),
		partnerSources:  make(map[string]*store.Table[PartnerEventSource]),
		busePolicies:    make(map[string]map[string]*EventBusPolicy),
		schemas:         make(map[string]*store.Table[Schema]),
		schemaVersions:  make(map[string][]*SchemaVersion),
		codeBindings:    make(map[string]*CodeBinding),
		deliveryTargets: &DeliveryTargets{},
		mu:              lockmetrics.New("eventbridge"),
		ctx:             ctx,
		cancel:          cancel,
		workerSem:       make(chan struct{}, defaultDeliveryWorkers),
		shutdownTimeout: defaultShutdownTimeout,
		deliveryTimeout: defaultDeliveryTimeout,
		ruleIndex:       make(map[string]map[string]map[ruleIndexKey]map[string]*Rule),
		targetsByARN:    make(map[string]map[string]map[string]struct{}),
	}
	// Create the default event bus in the backend's own region.
	now := time.Now()
	b.busesTable(b.region).Put(&EventBus{
		Name:             defaultEventBusName,
		Arn:              b.busARN(b.region, defaultEventBusName),
		CreatedTime:      now,
		LastModifiedTime: now,
	})

	return b
}

// Close marks the backend as closing, cancels the lifecycle context, and waits
// for all in-flight delivery goroutines to finish. It returns after at most
// shutdownTimeout to prevent a hung target service from blocking service
// shutdown indefinitely. Once Close is called, PutEvents will no longer spawn
// new delivery goroutines. The internal wg.Wait goroutine completes on its own
// once all delivery goroutines exit — either because the lifecycle context was
// cancelled (propagated to each delivery) or because the per-delivery deadline
// fired.
func (b *InMemoryBackend) Close() {
	// Mark as closing before cancelling so PutEvents stops scheduling new work.
	b.closing.Store(true)

	// Read shutdownTimeout under the same lock used by SetShutdownTimeout so
	// there is no data race between a concurrent setter and Close.
	var timeout time.Duration

	func() {
		b.mu.RLock("Close")
		defer b.mu.RUnlock()

		timeout = b.shutdownTimeout
	}()

	b.cancel()

	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
	}
}

// SetShutdownTimeout overrides the maximum time Close waits for in-flight goroutines.
// Primarily intended for tests.
func (b *InMemoryBackend) SetShutdownTimeout(d time.Duration) {
	b.mu.Lock("SetShutdownTimeout")
	defer b.mu.Unlock()
	b.shutdownTimeout = d
}

// SetDeliveryTimeout overrides the per-target delivery timeout.
// Primarily intended for tests.
func (b *InMemoryBackend) SetDeliveryTimeout(d time.Duration) {
	b.mu.Lock("SetDeliveryTimeout")
	defer b.mu.Unlock()
	b.deliveryTimeout = d
}

// SetDeliveryTargets configures the service references used for fan-out delivery.
// The backend registers itself as the API-destination resolver and the
// event-bus router (unless the caller supplied one) so outbound HTTP delivery
// and cross-bus routing work without a separate wiring step.
func (b *InMemoryBackend) SetDeliveryTargets(dt *DeliveryTargets) {
	b.mu.Lock("SetDeliveryTargets")
	defer b.mu.Unlock()
	if dt != nil && dt.APIDestinations == nil {
		dt.APIDestinations = b
	}
	if dt != nil && dt.EventBusRouter == nil {
		dt.EventBusRouter = b
	}
	b.deliveryTargets = dt
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	// b.registry is recreated from scratch rather than reused: every
	// currently-registered *store.Table[V] is about to be orphaned anyway
	// because every map field below is also reallocated to a brand-new
	// map[string]*store.Table[V] -- the old *store.Table[V] instances become
	// garbage, but that's fine, each will be recreated (and re-registered) the
	// next time its region/parent is touched (store.Register panics on a
	// duplicate name, so reusing the old registry here would panic on that
	// re-registration). Mirrors the services/ssm conversion.
	b.registry = store.NewRegistry()
	b.auxRegistry = store.NewRegistry()

	b.buses = make(map[string]*store.Table[EventBus])
	b.rules = make(map[string]map[string]*store.Table[Rule])
	b.targets = make(map[string]map[string]*store.Table[Target])
	b.eventLog = nil
	b.eventSources = make(map[string]*store.Table[EventSource])
	b.replays = make(map[string]*store.Table[Replay])
	b.apiDestinations = make(map[string]*store.Table[APIDestination])
	b.archives = make(map[string]*store.Table[Archive])
	b.archivedEvents = make(map[string]map[string][]EventEntry)
	b.connections = make(map[string]*store.Table[Connection])
	b.endpoints = make(map[string]*store.Table[Endpoint])
	b.partnerSources = make(map[string]*store.Table[PartnerEventSource])
	b.busePolicies = make(map[string]map[string]*EventBusPolicy)
	b.registries = nil
	b.schemas = make(map[string]*store.Table[Schema])
	b.schemaVersions = make(map[string][]*SchemaVersion)
	b.codeBindings = make(map[string]*CodeBinding)
	b.ruleIndex = make(map[string]map[string]map[ruleIndexKey]map[string]*Rule)
	b.targetsByARN = make(map[string]map[string]map[string]struct{})
	b.patternCache = sync.Map{}
	b.apiDestLimiters = sync.Map{}

	// Re-create the default event bus so it is always available after reset.
	now := time.Now()
	b.busesTable(b.region).Put(&EventBus{
		Name:             defaultEventBusName,
		Arn:              b.busARN(b.region, defaultEventBusName),
		CreatedTime:      now,
		LastModifiedTime: now,
	})
}
