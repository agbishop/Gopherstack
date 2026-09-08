package lambda

import (
	"context"
	"sync"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/container"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// lambdaDefaultMaxItems is the default page size for ListFunctions.
const lambdaDefaultMaxItems = 50

// StorageBackend defines the interface for Lambda backend operations.
type StorageBackend interface {
	CreateFunction(fn *FunctionConfiguration) error
	GetFunction(name string) (*FunctionConfiguration, error)
	ListFunctions(marker string, maxItems int) page.Page[*FunctionConfiguration]
	DeleteFunction(name string) error
	UpdateFunction(fn *FunctionConfiguration) error
	InvokeFunction(
		ctx context.Context,
		name string,
		invocationType InvocationType,
		payload []byte,
	) ([]byte, int, error)
	Purge(ctx context.Context, cutoff time.Time)
}

// QualifierInvoker is an optional extension of StorageBackend that supports qualified invocations.
// Backends implement this to support ?Qualifier= on Invoke (alias or version qualifier).
type QualifierInvoker interface {
	InvokeFunctionWithQualifier(
		ctx context.Context, name, qualifier, clientContext, logType string, invocationType InvocationType, payload []byte,
	) ([]byte, string, string, int, error)
}

// QualifierResolver is an optional extension of StorageBackend that resolves a
// qualifier (version number or alias name) to a function configuration for
// GetFunction/GetFunctionConfiguration. Backends implement this to support
// ?Qualifier= on the read paths.
type QualifierResolver interface {
	GetFunctionByQualifier(name, qualifier string) (*FunctionConfiguration, error)
}

// QualifierDeleter is an optional extension of StorageBackend that supports
// deleting a single published function version via DeleteFunctionInput's
// query-bound Qualifier member, leaving the rest of the function (and any
// other versions/aliases) intact. Backends implement this to support
// ?Qualifier= on DeleteFunction.
type QualifierDeleter interface {
	DeleteFunctionVersion(name, qualifier string) error
}

// ImageURIResolver is an optional extension of StorageBackend that validates
// a Code.ImageUri against a real ECR backend for Image package-type
// functions (see ECRResolver in crossservice.go). InMemoryBackend always
// implements this; ResolveImageURI returns true (accept) when cli.go has
// not wired an ECRResolver in.
type ImageURIResolver interface {
	ResolveImageURI(imageURI string) bool
}

// S3CodeFetcher can retrieve zip bytes from an S3-compatible store.
// It is used by InMemoryBackend to pull Zip Lambda code from S3.
type S3CodeFetcher interface {
	GetObjectBytes(ctx context.Context, bucket, key string) ([]byte, error)
}

// CWLogsBackend is the minimum CloudWatch Logs interface needed by Lambda for log delivery.
type CWLogsBackend interface {
	EnsureLogGroupAndStream(groupName, streamName string) error
	PutLogLines(groupName, streamName string, messages []string) error
}

// DNSRegistrar is an optional interface for registering synthetic DNS hostnames.
type DNSRegistrar interface {
	Register(hostname string)
	Deregister(hostname string)
}

// InMemoryBackend is a concurrency-safe in-memory Lambda backend.
type InMemoryBackend struct {
	cwLogs             CWLogsBackend
	s3Fetcher          S3CodeFetcher
	ecrResolver        ECRResolver
	docker             container.Runtime
	dnsRegistrar       DNSRegistrar
	ctx                context.Context
	logSem             chan struct{}
	fisFaults          map[string]*FISInvocationFault
	versionCounters    map[string]int
	functions          *store.Table[FunctionConfiguration]
	functionURLServers map[string]*functionURLServer
	functionURLConfigs *store.Table[FunctionURLConfig]
	versions           map[string][]*FunctionVersion
	// eventInvokeConfigs stays a plain map (not a store.Table): its only
	// identity field, FunctionArn, is copied from the owning
	// FunctionConfiguration.FunctionArn at Put time, and a large share of this
	// package's own test fixtures construct FunctionConfiguration without
	// ever setting FunctionArn (it is optional unless the FunctionURL/
	// EventInvokeConfig/CapacityProvider surface is being exercised) --
	// keying a Table off it would silently mis-key every such config to "".
	// WAS persisted before this refactor and remains a raw field on
	// backendSnapshot.
	eventInvokeConfigs    map[string]*FunctionEventInvokeConfig
	functionConcurrencies map[string]int
	kinesisPoller         *EventSourcePoller
	pollerCancel          context.CancelFunc
	// provisionedConcurrencies is keyed by FunctionArn (buildAliasARN:
	// function+qualifier composite); provisionedConcurrenciesByFunction
	// indexes it by bare function name for ListProvisionedConcurrencyConfigs
	// and the function-delete cascade. Registered on b.ephemeralRegistry, not
	// b.registry -- see store_setup.go's package doc: this was never
	// persisted before the conversion and must stay that way.
	provisionedConcurrencies           *store.Table[ProvisionedConcurrencyConfig]
	provisionedConcurrenciesByFunction *store.Index[ProvisionedConcurrencyConfig]
	layers                             map[string][]*LayerVersion
	eventSourceMappings                *store.Table[EventSourceMapping]
	esmByFunctionARN                   map[string]map[string]struct{}
	versionIndex                       map[string]map[string]*FunctionVersion
	cleanupSem                         chan struct{}
	layerVersionCounters               map[string]int64
	layerPolicies                      map[string]map[int64]map[string]*LayerVersionStatement
	// aliases is keyed by aliasKey(functionName, aliasName);
	// aliasesByFunction indexes it by bare function name for ListAliases and
	// the function-delete cascade.
	aliases           *store.Table[FunctionAlias]
	aliasesByFunction *store.Index[FunctionAlias]
	// permissions is keyed by permissionKeyFn (permissionMapKey(FunctionName,
	// Qualifier)+"|"+StatementID); permissionsByTarget indexes it by
	// permissionMapKey(FunctionName, Qualifier) for GetPolicy.
	permissions          *store.Table[FunctionPermission]
	permissionsByTarget  *store.Index[FunctionPermission]
	codeSigningConfigs   *store.Table[CodeSigningConfig]
	fnCodeSigningConfigs map[string]string
	capacityProviders    *store.Table[CapacityProvider]
	// registry holds every table that was already persisted before this
	// refactor (see backendSnapshot in persistence.go); ephemeralRegistry
	// holds tables with a pure key function that were NOT previously
	// persisted. Both are swept by Reset(); only registry feeds Snapshot/
	// Restore. See store_setup.go's package doc for the full rationale.
	registry                 *store.Registry
	ephemeralRegistry        *store.Registry
	runtimeManagementConfigs map[string]*RuntimeManagementConfig
	functionRecursionConfigs map[string]*FunctionRecursionConfig
	functionScalingConfigs   map[string]*FunctionScalingConfig
	durableExecs             *durableExecutionStore
	asyncEnqueueWaiters      chan struct{}
	shutdown                 chan struct{}
	mu                       *lockmetrics.RWMutex
	portAlloc                *portalloc.Allocator
	runtimes                 map[string]*functionRuntime
	activeConcurrencies      map[string]int
	asyncDelivery            AsyncDestinationDelivery
	accountID                string
	region                   string
	sigV4Secret              string
	settings                 Settings
	asyncWG                  sync.WaitGroup
	activationDelay          time.Duration
	pcActivationDelay        time.Duration
	cscIDCounter             int
	shutdownOnce             sync.Once
}

// SetActivationDelay configures how long a newly created function stays in the
// Pending state before transitioning to Active. Zero (the default) creates
// functions directly in Active, matching the near-instant activation of ZIP
// functions; a positive delay reproduces the Pending→Active window AWS exhibits
// for VPC and container functions.
func (b *InMemoryBackend) SetActivationDelay(d time.Duration) {
	b.mu.Lock("SetActivationDelay")
	defer b.mu.Unlock()
	b.activationDelay = d
}

// SetProvisionedConcurrencyDelay configures how long a provisioned concurrency
// config stays IN_PROGRESS before transitioning to READY. Zero (the default)
// reports READY immediately.
func (b *InMemoryBackend) SetProvisionedConcurrencyDelay(d time.Duration) {
	b.mu.Lock("SetProvisionedConcurrencyDelay")
	defer b.mu.Unlock()
	b.pcActivationDelay = d
}

// SetSigV4Secret sets the shared secret used to verify SigV4 signatures on
// AWS_IAM Function URL invocations. When empty the default "test" secret is used.
func (b *InMemoryBackend) SetSigV4Secret(secret string) {
	b.mu.Lock("SetSigV4Secret")
	defer b.mu.Unlock()
	b.sigV4Secret = secret
}

// NewInMemoryBackend creates a new Lambda in-memory backend with a background service context.
func NewInMemoryBackend(
	dockerClient container.Runtime,
	portAlloc *portalloc.Allocator,
	settings Settings,
	accountID, region string,
) *InMemoryBackend {
	return NewInMemoryBackendWithContext(context.Background(), dockerClient, portAlloc, settings, accountID, region)
}

// NewInMemoryBackendWithContext creates a new Lambda in-memory backend whose background
// goroutines are bounded by svcCtx. If svcCtx is nil, [context.Background] is used.
func NewInMemoryBackendWithContext(
	svcCtx context.Context,
	dockerClient container.Runtime,
	portAlloc *portalloc.Allocator,
	settings Settings,
	accountID, region string,
) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	b := &InMemoryBackend{
		runtimes:                 make(map[string]*functionRuntime),
		esmByFunctionARN:         make(map[string]map[string]struct{}),
		versionIndex:             make(map[string]map[string]*FunctionVersion),
		cleanupSem:               make(chan struct{}, maxCleanupConcurrency),
		logSem:                   make(chan struct{}, maxConcurrentInvocationLogs),
		functionURLServers:       make(map[string]*functionURLServer),
		versions:                 make(map[string][]*FunctionVersion),
		versionCounters:          make(map[string]int),
		layers:                   make(map[string][]*LayerVersion),
		layerVersionCounters:     make(map[string]int64),
		layerPolicies:            make(map[string]map[int64]map[string]*LayerVersionStatement),
		eventInvokeConfigs:       make(map[string]*FunctionEventInvokeConfig),
		functionConcurrencies:    make(map[string]int),
		activeConcurrencies:      make(map[string]int),
		fisFaults:                make(map[string]*FISInvocationFault),
		fnCodeSigningConfigs:     make(map[string]string),
		runtimeManagementConfigs: make(map[string]*RuntimeManagementConfig),
		functionRecursionConfigs: make(map[string]*FunctionRecursionConfig),
		functionScalingConfigs:   make(map[string]*FunctionScalingConfig),
		durableExecs:             newDurableExecutionStore(),
		asyncEnqueueWaiters:      make(chan struct{}, maxAsyncEnqueueWaiters),
		shutdown:                 make(chan struct{}),
		docker:                   dockerClient,
		portAlloc:                portAlloc,
		settings:                 settings,
		accountID:                accountID,
		region:                   region,
		ctx:                      svcCtx,
		mu:                       lockmetrics.New("lambda"),
		registry:                 store.NewRegistry(),
		ephemeralRegistry:        store.NewRegistry(),
	}

	registerAllTables(b)

	return b
}

// Close shuts down all active function URL servers and runtime API servers.
// It is safe to call concurrently and should be called when the backend is no longer needed.
func (b *InMemoryBackend) Close(ctx context.Context) {
	// Signal in-flight async (Event) invocation goroutines to stop waiting on a
	// container response so they exit instead of lingering until their per-event
	// timeout. shutdownOnce keeps Close idempotent and safe to call concurrently.
	b.shutdownOnce.Do(func() {
		close(b.shutdown)
	})

	var (
		urlServers []*functionURLServer
		rts        []*functionRuntime
		cancel     context.CancelFunc
	)

	func() {
		b.mu.Lock("Close")
		defer b.mu.Unlock()

		urlServers = make([]*functionURLServer, 0, len(b.functionURLServers))
		for _, srv := range b.functionURLServers {
			urlServers = append(urlServers, srv)
		}

		rts = make([]*functionRuntime, 0, len(b.runtimes))
		for _, rt := range b.runtimes {
			rts = append(rts, rt)
		}

		cancel = b.pollerCancel
		b.pollerCancel = nil
	}()

	// Stop the event-source poller goroutine if it was started.
	if cancel != nil {
		cancel()
	}

	var wg sync.WaitGroup

	for _, srv := range urlServers {
		wg.Go(func() {
			_ = srv.server.Shutdown(ctx)

			if b.portAlloc != nil {
				_ = b.portAlloc.Release(srv.port)
			}
		})
	}

	for _, rt := range rts {
		wg.Go(func() {
			b.cleanupRuntime(ctx, rt)
		})
	}

	wg.Wait()

	// Wait for async invocation goroutines (unblocked by closing b.shutdown above)
	// to finish so no background work outlives the backend.
	b.asyncWG.Wait()
}

// SetDNSRegistrar sets the optional DNS registrar used to register function URL hostnames.
func (b *InMemoryBackend) SetDNSRegistrar(r DNSRegistrar) {
	b.mu.Lock("SetDNSRegistrar")
	defer b.mu.Unlock()
	b.dnsRegistrar = r
}

// SetS3CodeFetcher sets the S3CodeFetcher for fetching Zip Lambda code from S3.
func (b *InMemoryBackend) SetS3CodeFetcher(f S3CodeFetcher) {
	b.mu.Lock("SetS3CodeFetcher")
	defer b.mu.Unlock()
	b.s3Fetcher = f
}

// SetCWLogsBackend sets the CloudWatch Logs backend for Lambda log delivery.
func (b *InMemoryBackend) SetCWLogsBackend(cwl CWLogsBackend) {
	b.mu.Lock("SetCWLogsBackend")
	defer b.mu.Unlock()
	b.cwLogs = cwl
}

// SetECRResolver wires the backend to validate Code.ImageUri against the
// real services/ecr backend -- see ECRResolver's doc comment. Called by
// cli.go's wireLambdaECR; nil (the default) skips validation.
func (b *InMemoryBackend) SetECRResolver(r ECRResolver) {
	b.mu.Lock("SetECRResolver")
	defer b.mu.Unlock()
	b.ecrResolver = r
}

// ResolveImageURI implements ImageURIResolver. It returns true (accept)
// when no ECRResolver has been wired in.
func (b *InMemoryBackend) ResolveImageURI(imageURI string) bool {
	b.mu.RLock("ResolveImageURI")
	resolver := b.ecrResolver
	b.mu.RUnlock()

	if resolver == nil {
		return true
	}

	return resolver.ResolveImage(imageURI)
}

// SetKinesisPoller sets the event source poller for Kinesis stream polling.
func (b *InMemoryBackend) SetKinesisPoller(p *EventSourcePoller) {
	b.mu.Lock("SetKinesisPoller")
	defer b.mu.Unlock()
	b.kinesisPoller = p
}

// StartKinesisPoller starts the Kinesis event source poller if one has been set.
// It stores a cancel function so Close() can stop the poller gracefully.
func (b *InMemoryBackend) StartKinesisPoller(ctx context.Context) {
	p := b.kinesisPollerLocked()

	if p == nil {
		return
	}

	pollerCtx, cancel := context.WithCancel(ctx)
	b.storePollerCancel(cancel)

	p.Start(pollerCtx)
}

// kinesisPollerLocked returns the configured Kinesis event source poller, if any.
func (b *InMemoryBackend) kinesisPollerLocked() *EventSourcePoller {
	b.mu.Lock("StartKinesisPoller")
	defer b.mu.Unlock()

	return b.kinesisPoller
}

// storePollerCancel stores the cancel function for the running Kinesis poller so
// Close() can stop it gracefully.
func (b *InMemoryBackend) storePollerCancel(cancel context.CancelFunc) {
	b.mu.Lock("StartKinesisPoller.storeCancel")
	defer b.mu.Unlock()

	b.pollerCancel = cancel
}

// SetSQSReader sets the SQS reader on the event source poller so that SQS
// queues can trigger Lambda functions via event source mappings.
func (b *InMemoryBackend) SetSQSReader(r SQSReader) {
	var p *EventSourcePoller

	func() {
		b.mu.RLock("SetSQSReader")
		defer b.mu.RUnlock()

		p = b.kinesisPoller
	}()

	if p != nil {
		p.SetSQSReader(r)
	}
}

// SetDynamoDBStreamsReader sets the DynamoDB Streams reader on the event source poller so
// that DynamoDB stream records can trigger Lambda functions via event source mappings.
func (b *InMemoryBackend) SetDynamoDBStreamsReader(r DynamoDBStreamsReader) {
	var p *EventSourcePoller

	func() {
		b.mu.RLock("SetDynamoDBStreamsReader")
		defer b.mu.RUnlock()

		p = b.kinesisPoller
	}()

	if p != nil {
		p.SetDynamoDBStreamsReader(r)
	}
}
