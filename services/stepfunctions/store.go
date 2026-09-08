package stepfunctions

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/services/stepfunctions/asl"
)

const (
	// maxExecutionInputBytes mirrors the AWS Step Functions hard limit on
	// StartExecution / StartSyncExecution input payload size (256 KiB).
	maxExecutionInputBytes    = 256 * 1024
	executionStartedEventID   = int64(1)
	executionSucceededEventID = int64(2)
	maxHistoryEvents          = 25000
	maxPendingActivityTasks   = 1000
	activityPollTimeout       = 60 * time.Second
	activityTokenBytes        = 32

	// maxExecutionNameLen is the AWS limit on execution name length.
	maxExecutionNameLen = 80
	// maxStateMachineNameLen is the AWS limit on state machine name length.
	maxStateMachineNameLen = 80
	// maxActivityNameLen is the AWS limit on activity name length.
	maxActivityNameLen = 80

	// executionPruneSweepThreshold is the number of stored executions above which
	// StartExecution opportunistically prunes finished executions that have aged
	// past the retention period. Keeps the execution map bounded even when the
	// background janitor is disabled.
	executionPruneSweepThreshold = 500

	statusRunning   = "RUNNING"
	statusSucceeded = "SUCCEEDED"
	statusFailed    = "FAILED"
	statusAborted   = "ABORTED"
	statusActive    = "ACTIVE"
	statusDeleting  = "DELETING"

	redriveStatusRedrivable    = "REDRIVABLE"
	redriveStatusNotRedrivable = "NOT_REDRIVABLE"

	// redriveStatusReasonRunning/Succeeded match AWS's documented
	// redriveStatusReason values (aws-sdk-go-v2/service/sfn@v1.45.4
	// api_op_DescribeExecution.go) for the two NOT_REDRIVABLE cases this
	// backend produces.
	redriveStatusReasonRunning   = "Execution is RUNNING and cannot be redriven."
	redriveStatusReasonSucceeded = "Execution is SUCCEEDED and cannot be redriven."
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

// regionFromARN extracts the region component from an ARN string.
// ARN format: arn:{partition}:{service}:{region}:{account}:{resource}.
func regionFromARN(arnStr, fallback string) string {
	const arnRegionIdx = 3

	parts := strings.Split(arnStr, ":")
	if len(parts) > arnRegionIdx && parts[arnRegionIdx] != "" {
		return parts[arnRegionIdx]
	}

	return fallback
}

// InMemoryBackend implements StorageBackend using in-memory maps.
//
// Phase 3.3: the resource maps (stateMachines, executions, activities,
// versions, aliases, mapRuns) are *store.Table[T] registered once on
// registry -- see store_setup.go. Execution history moved from a separate
// map onto Execution.history (inline, unexported); see the doc comment on
// [Execution]. Fields whose key is not a pure, immutable function of the
// stored value's own fields (or that hold non-serializable runtime state:
// channels, cancel funcs, timers) remain plain maps -- see store_setup.go's
// package comment and each field's own comment below for why.
type InMemoryBackend struct {
	lambdaInvoker   asl.LambdaInvoker
	sqsIntegration  asl.SQSIntegration
	snsIntegration  asl.SNSIntegration
	ddbIntegration  asl.DynamoDBIntegration
	ecsIntegration  asl.ECSIntegration
	ecsSyncWaiter   asl.ECSSyncWaiter
	glueIntegration asl.GlueIntegration
	glueSyncWaiter  asl.GlueSyncWaiter
	ebIntegration   asl.EventBridgeIntegration
	s3Reader        asl.S3Reader
	s3ResultWriter  asl.S3Writer
	svcCtx          context.Context
	// tasksByToken maps task token → task entry for SendTaskSuccess/Failure.
	// Left as a plain map (not a store.Table): activityTaskEntry carries
	// live, non-serializable runtime state (timers, result channels) and is
	// never persisted -- an isolated in-flight token store, not an
	// AWS-visible resource collection.
	tasksByToken map[string]*activityTaskEntry
	// nameIndex maps region → name → ARN for O(1) duplicate detection per region.
	nameIndex map[string]map[string]string
	// cancelFns holds the cancel function for each running execution goroutine.
	cancelFns map[string]context.CancelFunc
	// deletedExecs is a tombstone set for executions removed by DeleteStateMachine.
	// historyRecorder and runParsedExecution skip writes for tombstoned ARNs.
	deletedExecs      map[string]bool
	activities        *store.Table[Activity]
	activityNameIndex map[string]map[string]string
	// pendingTaskQueues maps activity ARN → buffered channel of pending tasks.
	pendingTaskQueues map[string]chan *activityTaskEntry
	executions        *store.Table[Execution]
	// executionsByStateMachine groups executions by (immutable) StateMachineArn,
	// replacing the former smExecutions []string index map.
	executionsByStateMachine *store.Index[Execution]
	// versions maps version ARN → version for PublishStateMachineVersion.
	versions *store.Table[StateMachineVersion]
	// versionsByStateMachine groups versions by (immutable) StateMachineArn,
	// replacing the former smVersions []string index map.
	versionsByStateMachine *store.Index[StateMachineVersion]
	// aliases maps alias ARN → alias for CreateStateMachineAlias.
	aliases *store.Table[StateMachineAlias]
	// smAliases maps state machine ARN → list of alias ARNs. Left as a plain
	// map: StateMachineAlias carries no StateMachineArn field of its own (only
	// the composite StateMachineAliasArn), so it is not a pure function of the
	// value's own fields and cannot be a store.Index key -- see store_setup.go.
	smAliases map[string][]string
	// executionDefinitions maps execution ARN → the SM definition that was active at start time.
	executionDefinitions map[string]string
	// historyTruncated tracks executions where the history cap has been reached
	// so we only emit a single warning per execution.
	historyTruncated map[string]bool
	stateMachines    *store.Table[StateMachine]
	mu               *lockmetrics.RWMutex
	// registry lets Reset (via resetLocked) collapse the six resource tables'
	// lifecycle to one registry.ResetAll() call instead of hand-rolled
	// re-initialization of each map. See store_setup.go.
	registry *store.Registry
	// mapRuns stores MapRun records keyed by MapRun ARN.
	mapRuns *store.Table[MapRun]
	// mapRunsByExecution groups MapRuns by (immutable) ExecutionArn, replacing
	// the former execMapRuns []string index map.
	mapRunsByExecution *store.Index[MapRun]
	// smExecsByStatus maps smARN → status → []execARN for O(1) filtered listing.
	// Left as a plain map (not a store.Index): Execution.Status is mutated in
	// place by StopExecution/RedriveExecution/runParsedExecution, which would
	// make a status-keyed Index stale -- see store_setup.go.
	smExecsByStatus map[string]map[string][]string
	accountID       string
	region          string
	settings        Settings
	// historyMu protects each Execution's inline history slice and
	// b.historyTruncated for concurrent cross-execution writes.
	// Lock order: b.mu (read or write) must be acquired before historyMu.
	historyMu sync.RWMutex
}

// activityTaskEntry holds a pending activity task and its result channel.
// Field order is tuned for GC pointer-bitmap scan range (pointer fields first).
type activityTaskEntry struct {
	// heartbeatTimer is reset on each SendTaskHeartbeat call. Nil if no heartbeat timeout.
	heartbeatTimer *time.Timer
	// heartbeatStop signals the heartbeat monitor to stop (on task completion).
	heartbeatStop chan struct{}
	resultCh      chan activityTaskResult
	// createdAt records when the task entry was created, used for TTL-based eviction.
	// time.Time contains a pointer (loc), placed before strings to keep scan range minimal.
	createdAt   time.Time
	activityArn string
	taskToken   string
	input       string
	// heartbeatDuration is the original duration for resetting the heartbeat timer.
	// Non-pointer field placed last to reduce GC scan range.
	heartbeatDuration time.Duration
}

// activityTaskResult holds the result of an activity task.
type activityTaskResult struct {
	output    string
	errCode   string
	cause     string
	succeeded bool
}

// NewInMemoryBackend creates a new InMemoryBackend with default configuration.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(config.DefaultAccountID, config.DefaultRegion)
}

// NewInMemoryBackendWithConfig creates a new InMemoryBackend with given account and region.
// Use NewInMemoryBackendWithContext to bind execution goroutines to a parent context.
func NewInMemoryBackendWithConfig(accountID, region string) *InMemoryBackend {
	return newInMemoryBackend(context.Background(), accountID, region)
}

// NewInMemoryBackendWithContext creates a new InMemoryBackend whose execution goroutines
// derive their contexts from svcCtx. When svcCtx is cancelled (e.g. on server shutdown),
// all running executions are also cancelled.
// If svcCtx is nil, [context.Background] is used.
func NewInMemoryBackendWithContext(
	svcCtx context.Context,
	accountID, region string,
) *InMemoryBackend {
	return newInMemoryBackend(svcCtx, accountID, region)
}

func newInMemoryBackend(svcCtx context.Context, accountID, region string) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	b := &InMemoryBackend{
		accountID:            accountID,
		region:               region,
		svcCtx:               svcCtx,
		nameIndex:            make(map[string]map[string]string),
		cancelFns:            make(map[string]context.CancelFunc),
		deletedExecs:         make(map[string]bool),
		activityNameIndex:    make(map[string]map[string]string),
		pendingTaskQueues:    make(map[string]chan *activityTaskEntry),
		tasksByToken:         make(map[string]*activityTaskEntry),
		smAliases:            make(map[string][]string),
		executionDefinitions: make(map[string]string),
		historyTruncated:     make(map[string]bool),
		mu:                   lockmetrics.New("stepfunctions"),
		registry:             store.NewRegistry(),
		settings:             DefaultSettings(),
		smExecsByStatus:      make(map[string]map[string][]string),
	}

	registerAllTables(b)

	return b
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string {
	return b.region
}

// AccountID returns the AWS account ID this backend is configured for.
func (b *InMemoryBackend) AccountID() string {
	return b.accountID
}

// SetSettings updates the backend settings.
func (b *InMemoryBackend) SetSettings(s Settings) {
	b.mu.Lock("SetSettings")
	defer b.mu.Unlock()
	b.settings = s
}

// Destroy cancels all running execution goroutines and releases resources.
func (b *InMemoryBackend) Destroy() {
	b.mu.Lock("Destroy")
	defer b.mu.Unlock()

	for execARN, cancel := range b.cancelFns {
		cancel()
		delete(b.cancelFns, execARN)
	}

	// Close activity task queues to unblock any waiting GetActivityTask callers.
	for activityArn, queue := range b.pendingTaskQueues {
		close(queue)
		delete(b.pendingTaskQueues, activityArn)
	}
}

// SetLambdaInvoker configures the Lambda invoker for Task states.
func (b *InMemoryBackend) SetLambdaInvoker(invoker asl.LambdaInvoker) {
	b.mu.Lock("SetLambdaInvoker")
	defer b.mu.Unlock()
	b.lambdaInvoker = invoker
}

// SetSQSIntegration configures the SQS integration for Task states.
func (b *InMemoryBackend) SetSQSIntegration(sqs asl.SQSIntegration) {
	b.mu.Lock("SetSQSIntegration")
	defer b.mu.Unlock()
	b.sqsIntegration = sqs
}

// SetSNSIntegration configures the SNS integration for Task states.
func (b *InMemoryBackend) SetSNSIntegration(sns asl.SNSIntegration) {
	b.mu.Lock("SetSNSIntegration")
	defer b.mu.Unlock()
	b.snsIntegration = sns
}

// SetDynamoDBIntegration configures the DynamoDB integration for Task states.
func (b *InMemoryBackend) SetDynamoDBIntegration(ddb asl.DynamoDBIntegration) {
	b.mu.Lock("SetDynamoDBIntegration")
	defer b.mu.Unlock()
	b.ddbIntegration = ddb
}

// SetECSIntegration configures the ECS integration.
func (b *InMemoryBackend) SetECSIntegration(ecs asl.ECSIntegration) {
	b.mu.Lock("SetECSIntegration")
	defer b.mu.Unlock()
	b.ecsIntegration = ecs
}

// SetECSSyncWaiter configures ".sync" pattern polling for ECS Task states.
func (b *InMemoryBackend) SetECSSyncWaiter(w asl.ECSSyncWaiter) {
	b.mu.Lock("SetECSSyncWaiter")
	defer b.mu.Unlock()
	b.ecsSyncWaiter = w
}

// SetGlueIntegration configures the Glue integration.
func (b *InMemoryBackend) SetGlueIntegration(glue asl.GlueIntegration) {
	b.mu.Lock("SetGlueIntegration")
	defer b.mu.Unlock()
	b.glueIntegration = glue
}

// SetGlueSyncWaiter configures ".sync" pattern polling for Glue Task states.
func (b *InMemoryBackend) SetGlueSyncWaiter(w asl.GlueSyncWaiter) {
	b.mu.Lock("SetGlueSyncWaiter")
	defer b.mu.Unlock()
	b.glueSyncWaiter = w
}

// SetEventBridgeIntegration configures the EventBridge integration.
func (b *InMemoryBackend) SetEventBridgeIntegration(eb asl.EventBridgeIntegration) {
	b.mu.Lock("SetEventBridgeIntegration")
	defer b.mu.Unlock()
	b.ebIntegration = eb
}

// SetS3Reader configures the S3 reader used to resolve Map state ItemReader
// (Distributed Map) items from S3 objects.
func (b *InMemoryBackend) SetS3Reader(s3Reader asl.S3Reader) {
	b.mu.Lock("SetS3Reader")
	defer b.mu.Unlock()
	b.s3Reader = s3Reader
}

// SetS3ResultWriter configures the S3 writer used to export Distributed Map
// ResultWriter results and manifest.
func (b *InMemoryBackend) SetS3ResultWriter(w asl.S3Writer) {
	b.mu.Lock("SetS3ResultWriter")
	defer b.mu.Unlock()
	b.s3ResultWriter = w
}

// integrationsSnapshot holds the Task-state service integrations configured
// on the backend, copied out under lock so a goroutine building an
// asl.Executor doesn't need to hold b.mu for the executor's lifetime.
type integrationsSnapshot struct {
	lambdaInvoker   asl.LambdaInvoker
	sqsIntegration  asl.SQSIntegration
	snsIntegration  asl.SNSIntegration
	ddbIntegration  asl.DynamoDBIntegration
	ecsIntegration  asl.ECSIntegration
	ecsSyncWaiter   asl.ECSSyncWaiter
	glueIntegration asl.GlueIntegration
	glueSyncWaiter  asl.GlueSyncWaiter
	ebIntegration   asl.EventBridgeIntegration
	s3Reader        asl.S3Reader
	s3ResultWriter  asl.S3Writer
}

// snapshotIntegrationsLocked copies the configured integrations. Must be
// called with at least b.mu's read lock held.
func (b *InMemoryBackend) snapshotIntegrationsLocked() integrationsSnapshot {
	return integrationsSnapshot{
		lambdaInvoker:   b.lambdaInvoker,
		sqsIntegration:  b.sqsIntegration,
		snsIntegration:  b.snsIntegration,
		ddbIntegration:  b.ddbIntegration,
		ecsIntegration:  b.ecsIntegration,
		ecsSyncWaiter:   b.ecsSyncWaiter,
		glueIntegration: b.glueIntegration,
		glueSyncWaiter:  b.glueSyncWaiter,
		ebIntegration:   b.ebIntegration,
		s3Reader:        b.s3Reader,
		s3ResultWriter:  b.s3ResultWriter,
	}
}

// applyIntegrations wires a snapshot's integrations onto executor. Callers
// still set their own ActivityInvoker, TaskTokenCallbackInvoker, and
// MapRunNotifier, which vary per execution path.
func applyIntegrations(executor *asl.Executor, s integrationsSnapshot) {
	executor.SetSQSIntegration(s.sqsIntegration)
	executor.SetSNSIntegration(s.snsIntegration)
	executor.SetDynamoDBIntegration(s.ddbIntegration)
	executor.SetECSIntegration(s.ecsIntegration)
	executor.SetECSSyncWaiter(s.ecsSyncWaiter)
	executor.SetGlueIntegration(s.glueIntegration)
	executor.SetGlueSyncWaiter(s.glueSyncWaiter)
	executor.SetEventBridgeIntegration(s.ebIntegration)
	executor.SetS3Reader(s.s3Reader)
	executor.SetS3ResultWriter(s.s3ResultWriter)
}

func (b *InMemoryBackend) smARN(region, name string) string {
	return arn.Build("states", region, b.accountID, "stateMachine:"+name)
}

func (b *InMemoryBackend) execARN(stateMachineARN, smName, execName string) string {
	region := regionFromARN(stateMachineARN, b.region)

	return arn.Build("states", region, b.accountID, "execution:"+smName+":"+execName)
}

func (b *InMemoryBackend) activityARN(region, name string) string {
	return arn.Build("states", region, b.accountID, "activity:"+name)
}

func (b *InMemoryBackend) versionARN(stateMachineARN, smName string, version int) string {
	region := regionFromARN(stateMachineARN, b.region)

	return arn.Build(
		"states",
		region,
		b.accountID,
		fmt.Sprintf("stateMachine:%s:%d", smName, version),
	)
}

func (b *InMemoryBackend) aliasARN(stateMachineARN, smName, aliasName string) string {
	region := regionFromARN(stateMachineARN, b.region)

	return arn.Build("states", region, b.accountID, "stateMachine:"+smName+":"+aliasName)
}

// regionNameIndex lazily initialises and returns the name→ARN map for region.
// Caller must hold b.mu write lock.
func (b *InMemoryBackend) regionNameIndex(region string) map[string]string {
	if b.nameIndex[region] == nil {
		b.nameIndex[region] = make(map[string]string)
	}

	return b.nameIndex[region]
}

// regionActivityIndex lazily initialises and returns the name→ARN map for region.
// Caller must hold b.mu write lock.
func (b *InMemoryBackend) regionActivityIndex(region string) map[string]string {
	if b.activityNameIndex[region] == nil {
		b.activityNameIndex[region] = make(map[string]string)
	}

	return b.activityNameIndex[region]
}

// namePattern is the AWS-allowed character set for state machine, execution, and activity names.
// AWS allows: letters, digits, and [-+/=_.@ ].
var namePattern = regexp.MustCompile(`^[-a-zA-Z0-9+/=_.@ ]+$`)

// validateName checks that a resource name meets AWS length and character constraints.
func validateName(name string, maxLen int) error {
	if name == "" || len(name) > maxLen {
		return fmt.Errorf("%w: name must be 1-%d characters", ErrInvalidName, maxLen)
	}

	if !namePattern.MatchString(name) {
		return fmt.Errorf(
			"%w: name must contain only letters, digits, and [-+/=_.@ ]",
			ErrInvalidName,
		)
	}

	return nil
}

// paginate applies token-based pagination to a sorted slice.
func paginate[T any](all []T, nextToken string, maxResults int) ([]T, string) {
	const defaultLimit = 100

	startIdx := parseNextToken(nextToken)
	if startIdx >= len(all) {
		return []T{}, ""
	}

	limit := defaultLimit
	if maxResults > 0 {
		limit = maxResults
	}

	end := startIdx + limit

	var outToken string
	if end < len(all) {
		outToken = strconv.Itoa(end)
	} else {
		end = len(all)
	}

	return all[startIdx:end], outToken
}

func parseNextToken(token string) int {
	if token == "" {
		return 0
	}
	idx, err := strconv.Atoi(token)
	if err != nil || idx < 0 {
		return 0
	}

	return idx
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
// Running executions are cancelled before state is cleared.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.resetLocked()
}

// resetLocked performs the actual reset. It is shared by Reset and by
// Restore's incompatible-snapshot-version path (see persistence.go), both of
// which already hold b.mu for writing when they call it.
func (b *InMemoryBackend) resetLocked() {
	// Cancel all running execution goroutines.
	for _, cancel := range b.cancelFns {
		cancel()
	}

	// Close activity task queues to unblock any waiting GetActivityTask callers.
	for _, queue := range b.pendingTaskQueues {
		close(queue)
	}

	// Tombstone all current execution ARNs so any in-flight goroutines that
	// exit after the reset see the tombstone and discard their state writes.
	newDeleted := make(map[string]bool, b.executions.Len())
	for _, exec := range b.executions.All() {
		newDeleted[exec.ExecutionArn] = true
	}

	// Clears stateMachines, executions (and their inline history),
	// activities, versions, aliases, and mapRuns in one call, including
	// their store.Index entries.
	b.registry.ResetAll()

	b.nameIndex = make(map[string]map[string]string)
	b.cancelFns = make(map[string]context.CancelFunc)
	b.deletedExecs = newDeleted
	b.activityNameIndex = make(map[string]map[string]string)
	b.pendingTaskQueues = make(map[string]chan *activityTaskEntry)
	b.tasksByToken = make(map[string]*activityTaskEntry)
	b.smAliases = make(map[string][]string)
	b.executionDefinitions = make(map[string]string)
	b.historyTruncated = make(map[string]bool)
	b.historyMu = sync.RWMutex{}
	b.smExecsByStatus = make(map[string]map[string][]string)
}
