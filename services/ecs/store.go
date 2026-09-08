package ecs

import (
	"sync"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	statusRunning           = "RUNNING"
	statusStopped           = "STOPPED"
	statusActive            = "ACTIVE"
	statusInactive          = "INACTIVE"
	statusProvisioning      = "PROVISIONING"
	statusPending           = "PENDING"
	statusDeactivating      = "DEACTIVATING"
	statusStopping          = "STOPPING"
	statusDeprovisioning    = "DEPROVISIONING"
	launchTypeFargate       = "FARGATE"
	defaultCluster          = "default"
	deploymentStatusPrimary = "PRIMARY"

	azRebalancingEnabled  = "ENABLED"
	azRebalancingDisabled = "DISABLED"

	// maxTaskDefinitionRevisions is the maximum number of revisions retained per
	// task definition family. Older INACTIVE revisions beyond this cap are
	// removed to prevent unbounded memory growth.
	maxTaskDefinitionRevisions = 100
)

// compile-time assertion.
var _ Backend = (*InMemoryBackend)(nil)

// svcRef is a composite key identifying a service by its cluster key and service name.
type svcRef struct {
	cluster string
	name    string
}

// InMemoryBackend stores ECS state in memory.
//
// Field ordering is optimised for pointer-byte alignment (govet fieldalignment);
// keep new fields grouped with their kind rather than by logical concern.
type InMemoryBackend struct {
	runner TaskRunner
	// cwLogs, when set (see SetCWLogsBackend), makes an awslogs-driver
	// container's log group/stream discoverable in CloudWatch Logs. Nil
	// preserves the historical behavior of LogConfiguration being stored
	// and echoed with no effect on CloudWatch Logs.
	cwLogs CWLogsBackend
	// elbv2Registrar, when set (see SetELBv2Registrar), registers/deregisters
	// real ELBv2 targets as tasks belonging to a service with LoadBalancers
	// reach/leave RUNNING. Nil preserves the historical behavior of
	// Service.LoadBalancers being stored and echoed with no effect on ELBv2.
	elbv2Registrar ELBv2TargetRegistrar
	// registry is the Phase 3.3 datalayer lifecycle registry: every *store.Table
	// below (except taskDefByArn/daemonTaskDefByArn, which are derived caches --
	// see store_setup.go) is registered on it exactly once at construction, so
	// Reset/Snapshot/Restore collapse to one registry call each instead of one
	// hand-written block per map. See pkgs/store's package doc.
	registry                    *store.Registry
	serviceDeployments          *store.Table[ServiceDeployment]
	taskSets                    *store.Table[TaskSet]
	taskSetsByService           *store.Index[TaskSet]
	taskDefByArn                *store.Table[TaskDefinition]
	services                    *store.Table[Service]
	servicesByCluster           *store.Index[Service]
	tasks                       *store.Table[Task]
	tasksByCluster              *store.Index[Task]
	containerInstances          *store.Table[ContainerInstance]
	containerInstancesByCluster *store.Index[ContainerInstance]
	clusters                    *store.Table[Cluster]
	taskProtections             *store.Table[TaskProtection]
	capacityProviders           *store.Table[CapacityProvider]
	accountSettings             *store.Table[AccountSetting]
	taskDefinitions             map[string][]*TaskDefinition
	attributes                  map[string]map[string]*Attribute
	mu                          *lockmetrics.RWMutex
	resourceTags                map[string][]Tag
	tasksByInstance             map[string]map[string]map[string]bool
	serviceIndex                map[svcRef]bool
	expressGatewayServices      *store.Table[ExpressGatewayService]
	daemonRevisions             *store.Table[DaemonRevision]
	daemonDeployments           *store.Table[DaemonDeployment]
	daemons                     *store.Table[Daemon] // composite "clusterName/daemonName" key; see daemonsByCluster
	daemonsByCluster            *store.Index[Daemon]
	daemonTaskDefinitions       map[string][]*DaemonTaskDefinition
	daemonTaskDefByArn          *store.Table[DaemonTaskDefinition]
	// daemonTaskDefs exists only for structural compatibility with purge.go's
	// per-cluster daemon cleanup (purgeDaemonsLocked). Real daemon task
	// definitions are registered independently by family, like ordinary task
	// definitions — not owned by a single daemon — matching the real
	// RegisterDaemonTaskDefinition/DescribeDaemonTaskDefinition API (see
	// daemonTaskDefinitions/daemonTaskDefByArn and daemon.go), so this
	// map is intentionally never populated.
	daemonTaskDefs map[string][]*DaemonTaskDefinition
	// lifecycle tracks tasks transitioning through observable intermediate states
	// (RUNNING→DEACTIVATING→STOPPING→DEPROVISIONING→STOPPED on stop,
	// PROVISIONING→PENDING→RUNNING on start), keyed by task ARN. Entries exist
	// only while stopDelay/startDelay > 0; the default fast path finalizes inline.
	lifecycle map[string]*taskLifecycle
	// serviceRevisions and serviceRevisionsByArn exist only for structural
	// compatibility with purge.go's per-service cleanup. This backend derives
	// ServiceRevision snapshots on demand from each Service's Deployments (see
	// DescribeServiceRevisions below and buildServiceRevision in
	// services.go) instead of persisting them separately, so these maps
	// are intentionally never populated.
	serviceRevisions      map[string][]*ServiceRevision
	serviceRevisionsByArn map[string]*ServiceRevision
	region                string
	accountID             string
	// clusterDeleteHooks are invoked (outside the backend lock) with each cluster
	// key removed by DeleteCluster or Purge, so external components such as the
	// Reconciler can release per-cluster resources and avoid unbounded growth.
	clusterDeleteHooks []func(clusterName string)
	// stopDelay is the per-phase delay applied to the task stop lifecycle. Zero
	// (the default) finalizes to STOPPED immediately; positive makes the task pass
	// through DEACTIVATING/STOPPING/DEPROVISIONING so SDK waiters observe them.
	stopDelay time.Duration
	// startDelay is the per-phase delay applied to the no-runner task start
	// lifecycle (PROVISIONING→PENDING→RUNNING). Zero finalizes immediately.
	startDelay time.Duration
	hooksMu    sync.Mutex
}

// TaskRunner is the interface for launching container tasks.
// The no-op implementation is used when no runtime is configured.
type TaskRunner interface {
	RunTask(task *Task, td *TaskDefinition) error
	StopTask(task *Task) error
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string, runner TaskRunner) *InMemoryBackend {
	b := &InMemoryBackend{
		taskDefinitions:       make(map[string][]*TaskDefinition),
		attributes:            make(map[string]map[string]*Attribute),
		resourceTags:          make(map[string][]Tag),
		tasksByInstance:       make(map[string]map[string]map[string]bool),
		serviceIndex:          make(map[svcRef]bool),
		lifecycle:             make(map[string]*taskLifecycle),
		mu:                    lockmetrics.New("ecs"),
		accountID:             accountID,
		region:                region,
		runner:                runner,
		daemonTaskDefinitions: make(map[string][]*DaemonTaskDefinition),
		daemonTaskDefs:        make(map[string][]*DaemonTaskDefinition),
		serviceRevisions:      make(map[string][]*ServiceRevision),
		serviceRevisionsByArn: make(map[string]*ServiceRevision),
		registry:              store.NewRegistry(),
	}

	registerAllTables(b)

	return b
}

// Reset zeroes all backend state for test isolation.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.taskDefByArn.Reset()
	b.daemonTaskDefByArn.Reset()

	b.taskDefinitions = make(map[string][]*TaskDefinition)
	b.attributes = make(map[string]map[string]*Attribute)
	b.resourceTags = make(map[string][]Tag)
	b.tasksByInstance = make(map[string]map[string]map[string]bool)
	b.serviceIndex = make(map[svcRef]bool)
	b.daemonTaskDefinitions = make(map[string][]*DaemonTaskDefinition)
	b.daemonTaskDefs = make(map[string][]*DaemonTaskDefinition)
	b.serviceRevisions = make(map[string][]*ServiceRevision)
	b.serviceRevisionsByArn = make(map[string]*ServiceRevision)
	b.lifecycle = make(map[string]*taskLifecycle)
}

// RegisterClusterDeleteHook registers a callback invoked (outside the backend
// lock) with the key of each cluster removed by DeleteCluster or Purge. It lets
// external components — such as the Reconciler's per-cluster launch semaphores —
// release cluster-scoped resources and avoid unbounded growth.
func (b *InMemoryBackend) RegisterClusterDeleteHook(fn func(clusterName string)) {
	if fn == nil {
		return
	}

	b.hooksMu.Lock()
	defer b.hooksMu.Unlock()

	b.clusterDeleteHooks = append(b.clusterDeleteHooks, fn)
}

// fireClusterDeleteHooks invokes every registered cluster-delete hook for each
// removed cluster key. Must be called without the backend lock held (hooks may
// take their own locks).
func (b *InMemoryBackend) fireClusterDeleteHooks(clusterNames ...string) {
	if len(clusterNames) == 0 {
		return
	}

	var hooks []func(string)

	func() {
		b.hooksMu.Lock()
		defer b.hooksMu.Unlock()

		hooks = make([]func(string), len(b.clusterDeleteHooks))
		copy(hooks, b.clusterDeleteHooks)
	}()

	for _, name := range clusterNames {
		for _, h := range hooks {
			h(name)
		}
	}
}

// GetRegion returns the AWS region this backend is configured for.
func (b *InMemoryBackend) GetRegion() string {
	return b.region
}

// noopRunner is a TaskRunner that does nothing (used when no runtime is configured).
type noopRunner struct{}

func (noopRunner) RunTask(_ *Task, _ *TaskDefinition) error { return nil }
func (noopRunner) StopTask(_ *Task) error                   { return nil }

// NewNoopRunner returns a TaskRunner that does nothing.
func NewNoopRunner() TaskRunner { return noopRunner{} }
