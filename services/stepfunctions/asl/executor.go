package asl

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

// ErrExecutionFailed is returned when a Fail state is reached.
var ErrExecutionFailed = errors.New("ExecutionFailed")

// ErrChoiceNoMatch is returned when a Choice state has no matching rule and no Default.
var ErrChoiceNoMatch = errors.New("States.NoChoiceMatched")

// Sentinel errors for executor internals.
var (
	ErrStateNotFound  = errors.New("state not found")
	ErrMaxTransitions = errors.New(
		"state machine exceeded maximum transitions",
	)
	ErrUnsupportedStateType             = errors.New("unsupported state type")
	ErrChoiceNoNext                     = errors.New("choice rule has no Next")
	ErrLambdaNotConfigured              = errors.New("lambda invoker not configured")
	ErrLambdaStatusError                = errors.New("lambda returned non-2xx status")
	ErrMapRequiresIterator              = errors.New("map state requires Iterator")
	ErrUnsupportedPathExpr              = errors.New("unsupported path expression")
	ErrUnsupportedResultPath            = errors.New("unsupported ResultPath")
	ErrCannotIndexNonObject             = errors.New("cannot index non-object with path")
	ErrFieldNotFound                    = errors.New("field not found")
	ErrMapInputNotArray                 = errors.New("input is not an array for Map state")
	ErrItemsPathNotArray                = errors.New("ItemsPath does not point to an array")
	ErrStatesTimeout                    = errors.New("States.Timeout")
	ErrSecondsPathNotNumber             = errors.New("SecondsPath did not resolve to a number")
	ErrTimestampPathNotString           = errors.New("TimestampPath did not resolve to a string")
	ErrNotAString                       = errors.New("not a string")
	ErrReferenceKeyNotString            = errors.New("value for reference key must be a string")
	ErrSQSIntegrationNotConfigured      = errors.New("SQS integration not configured")
	ErrSNSIntegrationNotConfigured      = errors.New("SNS integration not configured")
	ErrDynamoDBIntegrationNotConfigured = errors.New("DynamoDB integration not configured")
	ErrUnsupportedSQSAction             = errors.New("unsupported SQS action")
	ErrUnsupportedSNSAction             = errors.New("unsupported SNS action")
	ErrUnsupportedDynamoDBAction        = errors.New("unsupported DynamoDB action")
	ErrActivityNotConfigured            = errors.New("activity invoker not configured")
	ErrTaskTokenCallbackNotConfigured   = errors.New(
		"task token callback invoker not configured",
	)
	ErrECSIntegrationNotConfigured         = errors.New("ECS integration not configured")
	ErrGlueIntegrationNotConfigured        = errors.New("glue integration not configured")
	ErrEventBridgeIntegrationNotConfigured = errors.New("EventBridge integration not configured")
	ErrUnsupportedECSAction                = errors.New("unsupported ECS action")
	ErrUnsupportedGlueAction               = errors.New("unsupported Glue action")
	ErrUnsupportedEventBridgeAction        = errors.New("unsupported EventBridge action")
	ErrUnsupportedIntegration              = errors.New("unsupported service integration")
	// ErrSyncPatternUnsupported is returned when a Task state's Resource carries a
	// ".sync" suffix for a service that AWS does not support the "Run a Job" pattern
	// for. Per the Step Functions Developer Guide's integration pattern support table
	// (docs.aws.amazon.com/step-functions/latest/dg/connect-to-resource.html),
	// Lambda, SQS, SNS, DynamoDB, and EventBridge are Request-Response/waitForTaskToken
	// only -- ".sync" is "Not supported" for all of them.
	ErrSyncPatternUnsupported = errors.New(".sync integration pattern is not supported for this resource")
)

const (
	errCodeStatesPermissions                     = "States.Permissions"
	errCodeStatesRuntime                         = "States.Runtime"
	errCodeStatesTimeout                         = "States.Timeout"
	errCodeStatesTaskFailed                      = "States.TaskFailed"
	errCodeStatesExceedToleratedFailureThreshold = "States.ExceedToleratedFailureThreshold"
)

// Sentinel errors for Map state tolerated-failure threshold resolution.
var (
	ErrToleratedFailureCountNotNumber      = errors.New("ToleratedFailureCountPath: value is not a number")
	ErrToleratedFailurePercentageNotNumber = errors.New(
		"ToleratedFailurePercentagePath: value is not a number",
	)
)

// LambdaInvoker can invoke a Lambda function.
type LambdaInvoker interface {
	InvokeFunction(
		ctx context.Context,
		name, invocationType string,
		payload []byte,
	) ([]byte, int, error)
}

// ActivityInvoker can enqueue an activity task and wait for its result.
type ActivityInvoker interface {
	// InvokeActivity enqueues a task and blocks until completed.
	// heartbeatSeconds > 0 enables heartbeat timeout enforcement.
	InvokeActivity(
		ctx context.Context,
		activityArn, input string,
		heartbeatSeconds int,
	) (string, error)
}

// S3Reader reads objects from S3 for Map state ItemReader.
type S3Reader interface {
	// GetObjectBytes returns the raw bytes of an S3 object by bucket and key.
	GetObjectBytes(ctx context.Context, bucket, key string) ([]byte, error)
}

// S3Writer writes objects to S3 for a Distributed Map state's ResultWriter.
type S3Writer interface {
	PutObjectBytes(ctx context.Context, bucket, key string, data []byte) error
}

// SQSIntegration handles Step Functions SQS service integration.
type SQSIntegration interface {
	SFNSendMessage(
		ctx context.Context,
		queueURL, messageBody, groupID, deduplicationID string,
		delaySeconds int,
	) (messageID string, md5 string, err error)
}

// SNSIntegration handles Step Functions SNS service integration.
type SNSIntegration interface {
	SFNPublish(ctx context.Context, topicARN, message, subject string) (messageID string, err error)
}

// DynamoDBIntegration handles Step Functions DynamoDB service integration.
type DynamoDBIntegration interface {
	SFNPutItem(ctx context.Context, input any) (any, error)
	SFNGetItem(ctx context.Context, input any) (any, error)
	SFNDeleteItem(ctx context.Context, input any) (any, error)
	SFNUpdateItem(ctx context.Context, input any) (any, error)
	SFNBatchExecuteStatement(ctx context.Context, input any) (any, error)
	SFNBatchGetItem(ctx context.Context, input any) (any, error)
	SFNBatchWriteItem(ctx context.Context, input any) (any, error)
	SFNCreateBackup(ctx context.Context, input any) (any, error)
	SFNCreateGlobalTable(ctx context.Context, input any) (any, error)
	SFNCreateTable(ctx context.Context, input any) (any, error)
	SFNDeleteBackup(ctx context.Context, input any) (any, error)
	SFNDeleteResourcePolicy(ctx context.Context, input any) (any, error)
	SFNDeleteTable(ctx context.Context, input any) (any, error)
}

// ECSIntegration handles Step Functions ECS service integration.
type ECSIntegration interface {
	// SFNRunTask runs an ECS task and returns the response map (Tasks, Failures).
	SFNRunTask(ctx context.Context, input map[string]any) (any, error)
}

// ECSSyncPoll is one poll observation of a ".sync" ECS RunTask integration.
type ECSSyncPoll struct {
	// Result is ECS's own description of the completed task(s), matching
	// DescribeTasks's response shape rather than RunTask's start-call
	// response -- real Step Functions' .sync pattern returns the service's
	// description of the finished job, not the start response. Only valid
	// once Done is true.
	Result        any
	FailureReason string
	Done          bool
	Failed        bool
}

// ECSSyncWaiter lets a ".sync" ECS Task state poll the task(s) started by
// SFNRunTask until they reach ECS's terminal STOPPED status, instead of
// advancing as soon as RunTask returns (gopherstack-tdp6). It is optional
// and independent of ECSIntegration: when unset, ".sync" ECS Task states
// stay fire-and-forget, matching pre-tdp6 behavior.
type ECSSyncWaiter interface {
	// SFNPollSyncTask reports whether the task(s) started by a prior
	// SFNRunTask call are done. runTaskResult is exactly SFNRunTask's
	// return value.
	SFNPollSyncTask(ctx context.Context, runTaskResult any) (ECSSyncPoll, error)
}

// GlueIntegration handles Step Functions Glue service integration.
type GlueIntegration interface {
	// SFNStartJobRun starts a Glue job run and returns the JobRunId.
	SFNStartJobRun(ctx context.Context, jobName string, arguments map[string]string) (string, error)
}

// GlueSyncPoll is one poll observation of a ".sync" Glue StartJobRun integration.
type GlueSyncPoll struct {
	// Result is Glue's own description of the completed job run, matching
	// GetJobRun's response shape ({"JobRun": ...}) rather than
	// StartJobRun's start-call response. Only valid once Done is true.
	Result        any
	FailureReason string
	Done          bool
	Failed        bool
}

// GlueSyncWaiter lets a ".sync" Glue Task state poll a job run's JobRunState
// until it reaches a terminal state, instead of advancing as soon as
// StartJobRun returns (gopherstack-tdp6). Optional and independent of
// GlueIntegration: when unset, ".sync" Glue Task states stay fire-and-forget.
type GlueSyncWaiter interface {
	SFNPollSyncJobRun(ctx context.Context, jobName, runID string) (GlueSyncPoll, error)
}

// EventBridgeIntegration handles Step Functions EventBridge service integration.
type EventBridgeIntegration interface {
	// SFNPutEvents puts events to an EventBridge bus and returns failed-entry count.
	SFNPutEvents(ctx context.Context, entries []map[string]any) (int, error)
}

// TaskTokenCallbackInvoker waits for SendTaskSuccess/SendTaskFailure callbacks for a task token.
type TaskTokenCallbackInvoker interface {
	WaitForTaskToken(ctx context.Context, taskToken string, heartbeatSeconds int) (string, error)
}

// HistoryRecorder is called during execution to record state transition events.
type HistoryRecorder interface {
	RecordStateEntered(executionARN, stateName, stateType string, input any)
	RecordStateExited(executionARN, stateName, stateType string, output any)
	RecordTaskScheduled(executionARN, stateName, resource string, parameters any)
	RecordTaskSucceeded(executionARN, stateName, resource string, output any)
	RecordTaskFailed(executionARN, stateName, resource, errCode, cause string)
}

// MapRunNotifier receives callbacks when Map state runs start and end.
// Implement this to track Map state execution in a backend.
type MapRunNotifier interface {
	OnMapRunStart(executionARN, stateName string, maxConcurrency, itemCount int) string
	// resultsWritten is the count of items whose result was actually written
	// by a ResultWriter (0 when ResultWriter isn't configured), matching
	// DescribeMapRun's ItemCounts.ResultsWritten semantics.
	OnMapRunEnd(mapRunARN, status string, succeeded, failed, total, resultsWritten int)
}

// ExecutionResult holds the final output and status of a state machine execution.
type ExecutionResult struct {
	Output any
	Error  string
	Cause  string
	// Failed is true iff a Fail state (or an unhandled Task failure) ended
	// the execution. A Fail state's Error/Cause are both optional per the
	// ASL spec, so Error alone cannot distinguish "failed with no error
	// code" from "succeeded" -- callers must check Failed, not Error != "".
	Failed bool
}

const (
	maxConcurrentSubExecutors = 64
	maxJSONPathCacheEntries   = int64(4096)
	maxRetryDelay             = 24 * time.Hour
	maxCacheStoreCASAttempts  = 128
)

type jsonPathCache struct {
	entries    sync.Map
	size       atomic.Int64
	maxEntries int64
}

func newJSONPathCache(maxEntries int64) *jsonPathCache {
	if maxEntries <= 0 {
		maxEntries = 1
	}

	cache := &jsonPathCache{maxEntries: maxEntries}
	cache.size.Store(0)

	return cache
}

func (c *jsonPathCache) load(path string) ([]string, bool) {
	if c == nil {
		return nil, false
	}

	cached, found := c.entries.Load(path)
	if !found {
		return nil, false
	}

	parts, ok := cached.([]string)
	if !ok {
		return nil, false
	}

	return parts, true
}

func (c *jsonPathCache) store(path string, parts []string) {
	if c == nil {
		return
	}

	if _, found := c.entries.Load(path); found {
		return
	}

	reserved := false
	for range maxCacheStoreCASAttempts {
		currentSize := c.size.Load()
		if currentSize >= c.maxEntries {
			return
		}

		if c.size.CompareAndSwap(currentSize, currentSize+1) {
			reserved = true

			break
		}

		runtime.Gosched()
	}
	if !reserved {
		return
	}

	if _, loaded := c.entries.LoadOrStore(path, parts); loaded {
		// Another goroutine inserted the same key after reserve; release reservation.
		c.size.Add(-1)

		return
	}
}

// Executor runs an ASL state machine.
type Executor struct {
	s3             S3Reader
	s3w            S3Writer
	callback       TaskTokenCallbackInvoker
	sqs            SQSIntegration
	sns            SNSIntegration
	dynamodb       DynamoDBIntegration
	ecs            ECSIntegration
	ecsSyncWaiter  ECSSyncWaiter
	glue           GlueIntegration
	glueSyncWaiter GlueSyncWaiter
	eventbridge    EventBridgeIntegration
	history        HistoryRecorder
	mapRunNotifier MapRunNotifier
	lambda         LambdaInvoker
	activity       ActivityInvoker
	mapItemValue   any
	execSem        *semaphore.Weighted
	jsonPathCache  *jsonPathCache
	sm             *StateMachine
	execMeta       executionMeta
	branchName     string
	mapItemIdx     int
	inMapItem      bool
}

// executionMeta is the subset of context object data that ASL exposes via `$$`.
type executionMeta struct {
	ID               string
	Name             string
	RoleArn          string
	StartTime        string
	StateMachineID   string
	StateMachineArn  string
	StateMachineName string
}

// SetExecutionContext sets the metadata that will be exposed via `$$.Execution.*`
// and `$$.StateMachine.*` JSONPath references during evaluation.
func (e *Executor) SetExecutionContext(
	executionARN, executionName, roleArn, startTime, stateMachineARN, stateMachineName string,
) {
	e.execMeta = executionMeta{
		ID:               executionARN,
		Name:             executionName,
		RoleArn:          roleArn,
		StartTime:        startTime,
		StateMachineID:   stateMachineARN,
		StateMachineArn:  stateMachineARN,
		StateMachineName: stateMachineName,
	}
}

// NewExecutor creates an Executor for the given state machine.
func NewExecutor(sm *StateMachine, lambda LambdaInvoker, history HistoryRecorder) *Executor {
	return &Executor{
		sm:            sm,
		lambda:        lambda,
		history:       history,
		execSem:       semaphore.NewWeighted(maxConcurrentSubExecutors),
		jsonPathCache: newJSONPathCache(maxJSONPathCacheEntries),
	}
}

// newSubExecutor creates a sub-executor sharing this executor's semaphore and integrations.
func (e *Executor) newSubExecutor(sm *StateMachine) *Executor {
	return &Executor{
		sm:             sm,
		lambda:         e.lambda,
		sqs:            e.sqs,
		sns:            e.sns,
		dynamodb:       e.dynamodb,
		ecs:            e.ecs,
		ecsSyncWaiter:  e.ecsSyncWaiter,
		glue:           e.glue,
		glueSyncWaiter: e.glueSyncWaiter,
		eventbridge:    e.eventbridge,
		history:        e.history,
		activity:       e.activity,
		callback:       e.callback,
		s3:             e.s3,
		s3w:            e.s3w,
		execSem:        e.execSem,
		jsonPathCache:  e.jsonPathCache,
		execMeta:       e.execMeta,
		branchName:     e.branchName,
		mapRunNotifier: e.mapRunNotifier,
		inMapItem:      e.inMapItem,
		mapItemIdx:     e.mapItemIdx,
		mapItemValue:   e.mapItemValue,
	}
}

// newMapItemExecutor creates a sub-executor for a single Map state iteration.
// It exposes `$$.Map.Item.Index` and `$$.Map.Item.Value` in the context object.
func (e *Executor) newMapItemExecutor(sm *StateMachine, idx int, itemValue any) *Executor {
	sub := e.newSubExecutor(sm)
	sub.inMapItem = true
	sub.mapItemIdx = idx
	sub.mapItemValue = itemValue

	return sub
}

// newBranchExecutor creates a sub-executor whose `$$.Parallel.BranchName` and
// `$$.Execution.BranchName` paths resolve to the supplied name.
func (e *Executor) newBranchExecutor(sm *StateMachine, branchName string) *Executor {
	sub := e.newSubExecutor(sm)
	sub.branchName = branchName

	return sub
}

// buildContextObject returns the JSON-shaped `$$` context object for path
// evaluation. The shape mirrors the AWS Step Functions context object subset
// that is observable via Parameters/ResultSelector templates.
func (e *Executor) buildContextObject() map[string]any {
	exec := map[string]any{}

	if e.execMeta.ID != "" {
		exec["Id"] = e.execMeta.ID
	}

	if e.execMeta.Name != "" {
		exec["Name"] = e.execMeta.Name
	}

	if e.execMeta.RoleArn != "" {
		exec["RoleArn"] = e.execMeta.RoleArn
	}

	if e.execMeta.StartTime != "" {
		exec["StartTime"] = e.execMeta.StartTime
	}

	if e.branchName != "" {
		exec["BranchName"] = e.branchName
	}

	ctx := map[string]any{
		"Execution": exec,
		"StateMachine": map[string]any{
			"Id":   e.execMeta.StateMachineID,
			"Name": e.execMeta.StateMachineName,
		},
	}

	if e.branchName != "" {
		ctx["Parallel"] = map[string]any{"BranchName": e.branchName}
	}

	if e.inMapItem {
		ctx["Map"] = map[string]any{
			"Item": map[string]any{
				"Index": float64(e.mapItemIdx),
				"Value": e.mapItemValue,
			},
		}
	}

	return ctx
}

// SetSQSIntegration configures the SQS integration for Task states.
func (e *Executor) SetSQSIntegration(sqs SQSIntegration) { e.sqs = sqs }

// SetSNSIntegration configures the SNS integration for Task states.
func (e *Executor) SetSNSIntegration(sns SNSIntegration) { e.sns = sns }

// SetDynamoDBIntegration configures the DynamoDB integration for Task states.
func (e *Executor) SetDynamoDBIntegration(ddb DynamoDBIntegration) { e.dynamodb = ddb }

// SetS3Reader configures the S3 reader for Map state ItemReader.
func (e *Executor) SetS3Reader(s3 S3Reader) { e.s3 = s3 }

// SetS3ResultWriter configures the S3 writer for Distributed Map ResultWriter.
func (e *Executor) SetS3ResultWriter(w S3Writer) { e.s3w = w }

// SetActivityInvoker configures the activity invoker for activity Task states.
func (e *Executor) SetActivityInvoker(ai ActivityInvoker) { e.activity = ai }

// SetTaskTokenCallbackInvoker configures callback waiting for .waitForTaskToken integrations.
func (e *Executor) SetTaskTokenCallbackInvoker(invoker TaskTokenCallbackInvoker) {
	e.callback = invoker
}

// SetECSIntegration configures the ECS integration for Task states.
func (e *Executor) SetECSIntegration(ecs ECSIntegration) { e.ecs = ecs }

// SetECSSyncWaiter configures ".sync" pattern polling for ECS Task states.
func (e *Executor) SetECSSyncWaiter(w ECSSyncWaiter) { e.ecsSyncWaiter = w }

// SetGlueIntegration configures the Glue integration for Task states.
func (e *Executor) SetGlueIntegration(glue GlueIntegration) { e.glue = glue }

// SetGlueSyncWaiter configures ".sync" pattern polling for Glue Task states.
func (e *Executor) SetGlueSyncWaiter(w GlueSyncWaiter) { e.glueSyncWaiter = w }

// SetEventBridgeIntegration configures the EventBridge integration for Task states.
func (e *Executor) SetEventBridgeIntegration(eb EventBridgeIntegration) { e.eventbridge = eb }

// SetMapRunNotifier configures the MapRun notifier for Map states.
func (e *Executor) SetMapRunNotifier(n MapRunNotifier) {
	e.mapRunNotifier = n
}

// Execute runs the state machine with the given input JSON and returns the result.
func (e *Executor) Execute(
	ctx context.Context,
	executionARN, inputJSON string,
) (*ExecutionResult, error) {
	var input any
	if inputJSON != "" {
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			return nil, fmt.Errorf("invalid input JSON: %w", err)
		}
	}

	output, err := e.runStates(ctx, executionARN, e.sm.States, e.sm.StartAt, input)
	if err != nil {
		if failErr, ok := errors.AsType[*FailError](err); ok {
			return &ExecutionResult{Error: failErr.ErrCode, Cause: failErr.Cause, Failed: true}, nil
		}

		return nil, err
	}

	return &ExecutionResult{Output: output}, nil
}

// runStates executes states starting from startAt in the provided states map.
func (e *Executor) runStates(
	ctx context.Context,
	executionARN string,
	states map[string]*State,
	startAt string,
	input any,
) (any, error) {
	current := startAt
	value := input

	const maxTransitions = 10000

	for range maxTransitions {
		state, ok := states[current]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrStateNotFound, current)
		}

		if e.history != nil {
			e.history.RecordStateEntered(executionARN, current, state.Type, value)
		}

		// Apply InputPath.
		effectiveInput, err := applyPath(state.InputPath, value, e.jsonPathCache)
		if err != nil {
			return nil, fmt.Errorf("InputPath error in state %q: %w", current, err)
		}

		// Apply Parameters to transform the effective input for this state.
		taskInput := effectiveInput
		if len(state.Parameters) > 0 {
			paramInput := pathEvalInput{data: effectiveInput, context: e.buildContextObject()}
			taskInput, err = applyParametersTemplate(state.Parameters, paramInput)
			if err != nil {
				return nil, fmt.Errorf("parameters error in state %q: %w", current, err)
			}
		}

		var result any
		var nextState string

		nextState, result, err = e.executeState(ctx, executionARN, current, state, effectiveInput, taskInput)
		if err != nil {
			return nil, err
		}

		finalOutput, err := e.applyStateOutputTransforms(state, value, result, current)
		if err != nil {
			return nil, err
		}

		if e.history != nil {
			e.history.RecordStateExited(executionARN, current, state.Type, finalOutput)
		}

		if nextState == "" {
			return finalOutput, nil
		}

		value = finalOutput
		current = nextState
	}

	return nil, ErrMaxTransitions
}

// applyStateOutputTransforms applies ResultSelector, ResultPath, and OutputPath to produce the final state output.
func (e *Executor) applyStateOutputTransforms(
	state *State,
	input, result any,
	stateName string,
) (any, error) {
	// Apply ResultSelector to filter the state output before ResultPath merge.
	if len(state.ResultSelector) > 0 {
		var err error

		rsInput := pathEvalInput{data: result, context: e.buildContextObject()}

		result, err = applyParametersTemplate(state.ResultSelector, rsInput)
		if err != nil {
			return nil, fmt.Errorf("ResultSelector error in state %q: %w", stateName, err)
		}
	}

	// Apply ResultPath: merge result into original input.
	finalOutput, err := applyResultPath(state.ResultPath, input, result)
	if err != nil {
		return nil, fmt.Errorf("ResultPath error in state %q: %w", stateName, err)
	}

	// Apply OutputPath.
	finalOutput, err = applyPath(state.OutputPath, finalOutput)
	if err != nil {
		return nil, fmt.Errorf("OutputPath error in state %q: %w", stateName, err)
	}

	return finalOutput, nil
}

// executeState executes a single state and returns (nextStateName, output, error).
// pathInput is the state's input after InputPath but before Parameters; Task
// and Map thread it separately from input (the post-Parameters payload) so
// their *Path fields (TimeoutSecondsPath, ItemsPath, MaxConcurrencyPath, ...)
// resolve against the pre-Parameters value, per the ASL spec (see
// resolveTaskTimeoutSeconds and resolveMapItems).
func (e *Executor) executeState(
	ctx context.Context,
	executionARN, stateName string,
	state *State,
	pathInput, input any,
) (string, any, error) {
	switch state.Type {
	case "Pass":
		return e.executePass(state, input)
	case "Succeed":
		return "", input, nil
	case "Fail":
		return "", nil, &FailError{ErrCode: state.Error, Cause: state.Cause}
	case "Wait":
		return e.executeWait(ctx, state, input)
	case "Choice":
		return e.executeChoice(state, input)
	case "Task":
		return e.executeTask(ctx, executionARN, stateName, state, pathInput, input)
	case "Parallel":
		return e.executeParallel(ctx, executionARN, stateName, state, input)
	case StateTypeMap:
		return e.executeMap(ctx, executionARN, stateName, state, pathInput, input)
	default:
		return "", nil, fmt.Errorf(
			"%w: %q in state %q",
			ErrUnsupportedStateType,
			state.Type,
			stateName,
		)
	}
}

// executePass handles Pass state.
func (e *Executor) executePass(state *State, input any) (string, any, error) {
	if len(state.Result) > 0 {
		var result any
		if err := json.Unmarshal(state.Result, &result); err != nil {
			return "", nil, fmt.Errorf("invalid Pass Result: %w", err)
		}

		return state.Next, result, nil
	}

	return state.Next, input, nil
}

// executeWait handles Wait state.
//

// executeWait handles Wait state.
func (e *Executor) executeWait(ctx context.Context, state *State, input any) (string, any, error) {
	waitDuration, err := e.resolveWaitDuration(state, input)
	if err != nil {
		return "", nil, err
	}

	if waitDuration > 0 {
		if waitErr := e.waitForDuration(ctx, waitDuration); waitErr != nil {
			return "", nil, waitErr
		}
	}

	return state.Next, input, nil
}

// resolveWaitDuration determines the wait duration from the state configuration.
func (e *Executor) resolveWaitDuration(state *State, input any) (time.Duration, error) {
	switch {
	case state.Seconds > 0:
		return time.Duration(state.Seconds) * time.Second, nil

	case state.SecondsPath != "":
		return e.resolveSecondsPath(state.SecondsPath, input)

	case state.Timestamp != "":
		return resolveTimestampDuration(state.Timestamp)

	case state.TimestampPath != "":
		return e.resolveTimestampPath(state.TimestampPath, input)
	}

	return 0, nil
}

// resolveSecondsPath resolves the wait duration from a path to a numeric value.
func (e *Executor) resolveSecondsPath(path string, input any) (time.Duration, error) {
	val, err := applyPath(path, input, e.jsonPathCache)
	if err != nil {
		return 0, fmt.Errorf("SecondsPath error: %w", err)
	}

	secs, ok := toFloat(val)
	if !ok {
		return 0, ErrSecondsPathNotNumber
	}

	if secs > 0 {
		return time.Duration(secs * float64(time.Second)), nil
	}

	return 0, nil
}

// resolveTimestampDuration resolves the wait duration from a timestamp string.
func resolveTimestampDuration(timestamp string) (time.Duration, error) {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return 0, fmt.Errorf("timestamp parse error: %w", err)
	}

	if d := time.Until(t); d > 0 {
		return d, nil
	}

	return 0, nil
}

// resolveTimestampPath resolves the wait duration from a path to a timestamp string.
func (e *Executor) resolveTimestampPath(path string, input any) (time.Duration, error) {
	val, err := applyPath(path, input, e.jsonPathCache)
	if err != nil {
		return 0, fmt.Errorf("TimestampPath error: %w", err)
	}

	tsStr, ok := val.(string)
	if !ok {
		return 0, ErrTimestampPathNotString
	}

	t, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		return 0, fmt.Errorf("TimestampPath value parse error: %w", err)
	}

	if d := time.Until(t); d > 0 {
		return d, nil
	}

	return 0, nil
}

// waitForDuration waits for the specified duration or context cancellation.
func (e *Executor) waitForDuration(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// executeChoice handles Choice state.
func (e *Executor) executeChoice(state *State, input any) (string, any, error) {
	for _, rule := range state.Choices {
		if evaluateChoiceRule(&rule, input, e.jsonPathCache) {
			if rule.Next == "" {
				return "", nil, ErrChoiceNoNext
			}

			return rule.Next, input, nil
		}
	}

	if state.Default != "" {
		return state.Default, input, nil
	}

	return "", nil, &FailError{
		ErrCode: ErrChoiceNoMatch.Error(),
		Cause:   "No choice matched and no Default was provided",
	}
}

// executeTask handles Task state (Lambda integration and placeholder for other types).
func (e *Executor) executeTask(
	ctx context.Context,
	executionARN, stateName string,
	state *State,
	pathInput, input any,
) (string, any, error) {
	timeoutSeconds, err := e.resolveTaskTimeoutSeconds(state, pathInput)
	if err != nil {
		return "", nil, err
	}

	heartbeatSeconds, err := e.resolveTaskHeartbeatSeconds(state, pathInput)
	if err != nil {
		return "", nil, err
	}

	if e.history != nil {
		e.history.RecordTaskScheduled(executionARN, stateName, state.Resource, input)
	}

	// retryAttempts tracks how many times each retrier entry has been used.
	retryAttempts := make([]int, len(state.Retry))
	waitForTaskToken := isWaitForTaskTokenResource(state.Resource)

	for {
		result, taskErr := e.runTaskAttempt(ctx, state, input, waitForTaskToken, timeoutSeconds, heartbeatSeconds)
		if taskErr == nil {
			e.recordTaskSucceeded(executionARN, stateName, state.Resource, result)

			return state.Next, result, nil
		}

		if errors.Is(taskErr, context.DeadlineExceeded) {
			taskErr = ErrStatesTimeout
		} else if errors.Is(taskErr, context.Canceled) {
			// External cancellation: propagate immediately without retrying.
			return "", nil, taskErr
		}

		retried, retryErr := tryRetry(ctx, state, retryAttempts, taskErr)
		if retryErr != nil {
			if !errors.Is(retryErr, ErrStatesTimeout) {
				return "", nil, retryErr
			}

			taskErr = ErrStatesTimeout
		} else if retried {
			continue
		}

		if next, out, matched := e.checkCatchers(executionARN, stateName, state, input, taskErr); matched {
			return next, out, nil
		}

		e.recordTaskFailed(
			executionARN, stateName, state.Resource,
			stepFunctionsErrorCode(taskErr), stepFunctionsErrorCause(taskErr),
		)

		return "", nil, &FailError{ErrCode: "TaskFailed", Cause: taskErr.Error()}
	}
}

// runTaskAttempt invokes a single Task attempt under its own TimeoutSeconds
// deadline, freshly derived from ctx. AWS docs (amazon-states-language-task-state.html)
// on TimeoutSeconds: "the timeout count begins when the start event is
// executed, such as when TaskStarted ... events are logged" -- a new start
// event is logged per retry attempt, so the deadline resets on every
// attempt rather than being shared across the whole state including
// retries. tryRetry (called by executeTask with the un-wrapped ctx) is
// therefore not starved by a prior attempt's expired deadline.
func (e *Executor) runTaskAttempt(
	ctx context.Context,
	state *State,
	input any,
	waitForTaskToken bool,
	timeoutSeconds, heartbeatSeconds int,
) (any, error) {
	attemptCtx := ctx

	if timeoutSeconds > 0 {
		var cancel context.CancelFunc

		attemptCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
	}

	return e.invokeTaskAttempt(attemptCtx, state, input, waitForTaskToken, heartbeatSeconds)
}

func (e *Executor) invokeTaskAttempt(
	ctx context.Context,
	state *State,
	input any,
	waitForTaskToken bool,
	heartbeatSeconds int,
) (any, error) {
	if !waitForTaskToken {
		return e.invokeTask(ctx, state, input, heartbeatSeconds)
	}

	if e.callback == nil {
		return nil, ErrTaskTokenCallbackNotConfigured
	}

	taskToken, err := generateTaskToken()
	if err != nil {
		return nil, err
	}

	taskInput := injectTaskToken(input, taskToken)
	invokeResult, invokeErr := e.invokeTask(ctx, state, taskInput, heartbeatSeconds)
	if invokeErr != nil {
		return nil, invokeErr
	}

	callbackOutput, err := e.callback.WaitForTaskToken(ctx, taskToken, heartbeatSeconds)
	if err != nil {
		return nil, err
	}
	if callbackOutput == "" {
		return invokeResult, nil
	}

	return parseCallbackOutput(callbackOutput), nil
}

// parseCallbackOutput tries to decode callback output as JSON and falls back to raw string.
// AWS callback outputs are often JSON, but plain strings are allowed.
func parseCallbackOutput(output string) any {
	var result any
	if output != "" {
		if err := json.Unmarshal([]byte(output), &result); err == nil {
			return result
		}
	}

	return output
}

func injectTaskToken(input any, taskToken string) any {
	inputMap, ok := input.(map[string]any)
	if !ok {
		return map[string]any{
			"Input":     input,
			"TaskToken": taskToken,
		}
	}

	copyMap := make(map[string]any, mapCopyCapacity(len(inputMap)))
	maps.Copy(copyMap, inputMap)
	copyMap["TaskToken"] = taskToken

	return copyMap
}

func mapCopyCapacity(size int) int {
	if size > math.MaxInt-1 {
		return 0
	}

	return size + 1
}

// generateTaskToken returns a cryptographically random task token.
// Token format is URL-safe base64-encoded 32 random bytes.
func generateTaskToken() (string, error) {
	// 32 random bytes provide 256-bit entropy, matching strong token requirements.
	const taskTokenBytes = 32
	tokenBytes := make([]byte, taskTokenBytes)
	if _, err := cryptorand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate task token: %w", err)
	}

	return base64.URLEncoding.EncodeToString(tokenBytes), nil
}

// waitForRetry waits for the retry delay, or returns ctx.Err() if the context is done.
// It checks for immediate cancellation first to avoid non-deterministic select behavior.
func waitForRetry(ctx context.Context, delay time.Duration) error {
	// Fail fast if context is already done.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// tryRetry attempts to schedule a retry for the failed task.
// Returns (scheduled=true, nil) if a retry delay was served; the caller should retry the task.
// Returns (false, ErrStatesTimeout) if the context deadline fired during the delay.
// Returns (false, ctx.Err()) for other context cancellations.
// Returns (false, nil) if no matching, non-exhausted retrier was found.
func tryRetry(ctx context.Context, state *State, retryAttempts []int, taskErr error) (bool, error) {
	const defaultMaxAttempts = 3
	const defaultIntervalSeconds = 1
	const defaultBackoffRate = 2.0

	for i := range state.Retry {
		retrier := &state.Retry[i]

		if !catchesError(retrier.ErrorEquals, taskErr) {
			continue
		}

		maxAttempts := defaultMaxAttempts
		if retrier.MaxAttempts != nil {
			maxAttempts = *retrier.MaxAttempts
		}

		if retryAttempts[i] >= maxAttempts {
			continue
		}

		intervalSeconds := defaultIntervalSeconds
		if retrier.IntervalSeconds != nil {
			intervalSeconds = *retrier.IntervalSeconds
		}

		backoffRate := retrier.BackoffRate
		if backoffRate <= 0 {
			backoffRate = defaultBackoffRate
		}

		delay := computeRetryDelay(intervalSeconds, backoffRate, retryAttempts[i])
		delay = applyRetryDelayCapAndJitter(delay, retrier)
		retryAttempts[i]++

		if err := waitForRetry(ctx, delay); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return false, ErrStatesTimeout
			}

			return false, err
		}

		return true, nil
	}

	return false, nil
}

func computeRetryDelay(intervalSeconds int, backoffRate float64, attempts int) time.Duration {
	delaySeconds := float64(intervalSeconds) * math.Pow(backoffRate, float64(attempts))
	if math.IsNaN(delaySeconds) || math.IsInf(delaySeconds, 0) {
		return 0
	}

	if delaySeconds > maxRetryDelay.Seconds() {
		return maxRetryDelay
	}

	if delaySeconds < 0 {
		return 0
	}

	return time.Duration(delaySeconds * float64(time.Second))
}

// applyRetryDelayCapAndJitter applies a Retrier's MaxDelaySeconds cap and
// JitterStrategy to a computed backoff delay. AWS's default JitterStrategy is
// "NONE" (delay used as-is); "FULL" randomizes the delay uniformly in
// [0, delay] to spread out simultaneous retries across concurrent executions.
func applyRetryDelayCapAndJitter(delay time.Duration, retrier *Retrier) time.Duration {
	if retrier.MaxDelaySeconds != nil && *retrier.MaxDelaySeconds > 0 {
		maxDelay := time.Duration(*retrier.MaxDelaySeconds) * time.Second
		if delay > maxDelay {
			delay = maxDelay
		}
	}

	if retrier.JitterStrategy == jitterStrategyFull {
		delay = time.Duration(rand.Float64() * float64(delay)) //nolint:gosec // non-cryptographic per ASL spec
	}

	return delay
}

// checkCatchers checks Catch clauses and returns (nextState, output, matched).
// If a catcher matches, the task failure is recorded in the history.
func (e *Executor) checkCatchers(
	executionARN, stateName string,
	state *State,
	input any,
	taskErr error,
) (string, any, bool) {
	for _, catcher := range state.Catch {
		if catchesError(catcher.ErrorEquals, taskErr) {
			errCode := stepFunctionsErrorCode(taskErr)
			cause := stepFunctionsErrorCause(taskErr)

			// AWS's error output has separate "Error" and "Cause" fields (the
			// error code and a human-readable description), not one combined
			// string. See "Handling errors in Step Functions workflows".
			errorResult := map[string]any{
				"Error": errCode,
			}
			if cause != "" {
				errorResult["Cause"] = cause
			}

			out, _ := applyResultPath(catcher.ResultPath, input, errorResult)

			e.recordTaskFailed(executionARN, stateName, state.Resource, errCode, cause)

			return catcher.Next, out, true
		}
	}

	return "", nil, false
}

// recordTaskSucceeded records a task success event if a history recorder is configured.
func (e *Executor) recordTaskSucceeded(executionARN, stateName, resource string, result any) {
	if e.history != nil {
		e.history.RecordTaskSucceeded(executionARN, stateName, resource, result)
	}
}

// recordTaskFailed records a task failure event if a history recorder is configured.
func (e *Executor) recordTaskFailed(executionARN, stateName, resource, errCode, cause string) {
	if e.history != nil {
		e.history.RecordTaskFailed(executionARN, stateName, resource, errCode, cause)
	}
}

// invokeTask performs the actual task invocation.
func (e *Executor) invokeTask(ctx context.Context, state *State, input any, heartbeatSeconds int) (any, error) {
	if err := checkSyncPatternSupported(state.Resource); err != nil {
		return nil, err
	}

	if isActivityResource(state.Resource) {
		return e.invokeActivityTask(ctx, state, input, heartbeatSeconds)
	}
	if isLambdaResource(state.Resource) {
		return e.invokeLambdaTask(ctx, state, input)
	}
	if isSQSResource(state.Resource) {
		return e.invokeSQSTask(ctx, state, input)
	}
	if isSNSResource(state.Resource) {
		return e.invokeSNSTask(ctx, state, input)
	}
	if isDynamoDBResource(state.Resource) {
		return e.invokeDynamoDBTask(ctx, state, input)
	}
	if isECSResource(state.Resource) {
		return e.invokeECSTask(ctx, state, input)
	}
	if isGlueResource(state.Resource) {
		return e.invokeGlueTask(ctx, state, input)
	}
	if isEventBridgeResource(state.Resource) {
		return e.invokeEventBridgeTask(ctx, state, input)
	}
	if isAPIGatewayResource(state.Resource) || isEMRResource(state.Resource) {
		return nil, &FailError{
			ErrCode: errCodeStatesTaskFailed,
			Cause:   fmt.Sprintf("unsupported service integration: %s", state.Resource),
		}
	}

	return nil, &FailError{
		ErrCode: errCodeStatesTaskFailed,
		Cause:   fmt.Sprintf("unsupported service integration: %s", state.Resource),
	}
}

// invokeActivityTask enqueues a task for an activity worker and waits for the result.
func (e *Executor) invokeActivityTask(ctx context.Context, state *State, input any, heartbeatSeconds int) (any, error) {
	if e.activity == nil {
		return nil, ErrActivityNotConfigured
	}

	output, err := e.activity.InvokeActivity(
		ctx,
		state.Resource,
		marshalInput(input),
		heartbeatSeconds,
	)
	if err != nil {
		return nil, err
	}

	var result any
	if unmarshalErr := json.Unmarshal([]byte(output), &result); unmarshalErr != nil {
		// Non-JSON output is valid; return the raw string. Intentionally ignoring unmarshalErr.
		return output, nil //nolint:nilerr // non-JSON activity output is valid; return as string
	}

	return result, nil
}

// invokeLambdaTask invokes a Lambda function as a Task state.
func (e *Executor) invokeLambdaTask(ctx context.Context, state *State, input any) (any, error) {
	if e.lambda == nil {
		return nil, ErrLambdaNotConfigured
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task input: %w", err)
	}

	respBytes, statusCode, err := e.lambda.InvokeFunction(
		ctx,
		state.Resource,
		"RequestResponse",
		payload,
	)
	if err != nil {
		return nil, err
	}

	const statusOK = 200
	if statusCode >= 400 || statusCode < statusOK {
		return nil, fmt.Errorf("%w: %d", ErrLambdaStatusError, statusCode)
	}

	var result any
	// Non-JSON Lambda responses are valid; fall back to raw string.
	if json.Unmarshal(respBytes, &result) != nil {
		result = string(respBytes)
	}

	return result, nil
}

// invokeSQSTask invokes an SQS action as a Task state.
func (e *Executor) invokeSQSTask(ctx context.Context, state *State, input any) (any, error) {
	if e.sqs == nil {
		return nil, ErrSQSIntegrationNotConfigured
	}

	action := serviceAction(state.Resource)
	m, _ := input.(map[string]any)

	switch action {
	case "sendMessage":
		queueURL, _ := m["QueueUrl"].(string)
		messageBody, _ := m["MessageBody"].(string)
		groupID, _ := m["MessageGroupId"].(string)
		dedupID, _ := m["MessageDeduplicationId"].(string)
		var delaySeconds int
		if d, ok := toFloat(m["DelaySeconds"]); ok {
			delaySeconds = int(d)
		}
		msgID, md5, err := e.sqs.SFNSendMessage(
			ctx,
			queueURL,
			messageBody,
			groupID,
			dedupID,
			delaySeconds,
		)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"MD5OfMessageBody": md5,
			"MessageId":        msgID,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedSQSAction, action)
	}
}

// invokeSNSTask invokes an SNS action as a Task state.
func (e *Executor) invokeSNSTask(ctx context.Context, state *State, input any) (any, error) {
	if e.sns == nil {
		return nil, ErrSNSIntegrationNotConfigured
	}

	action := serviceAction(state.Resource)
	m, _ := input.(map[string]any)

	switch action {
	case "publish":
		topicARN, _ := m["TopicArn"].(string)
		message, _ := m["Message"].(string)
		subject, _ := m["Subject"].(string)
		msgID, err := e.sns.SFNPublish(ctx, topicARN, message, subject)
		if err != nil {
			return nil, err
		}

		return map[string]any{"MessageId": msgID}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedSNSAction, action)
	}
}

// invokeDynamoDBTask invokes a DynamoDB action as a Task state.
func (e *Executor) invokeDynamoDBTask(ctx context.Context, state *State, input any) (any, error) {
	if e.dynamodb == nil {
		return nil, ErrDynamoDBIntegrationNotConfigured
	}

	action := serviceAction(state.Resource)

	if out, ok, err := e.invokeDynamoDBItemOps(ctx, action, input); ok {
		return out, err
	}

	if out, ok, err := e.invokeDynamoDBTableOps(ctx, action, input); ok {
		return out, err
	}

	return nil, fmt.Errorf("%w: %q", ErrUnsupportedDynamoDBAction, action)
}

// invokeDynamoDBItemOps handles per-item and batch DynamoDB actions.
// Returns (result, true, err) on a match, or (nil, false, nil) when no action matches.
func (e *Executor) invokeDynamoDBItemOps(
	ctx context.Context,
	action string,
	input any,
) (any, bool, error) {
	switch action {
	case "putItem":
		out, err := e.dynamodb.SFNPutItem(ctx, input)

		return out, true, err
	case "getItem":
		out, err := e.dynamodb.SFNGetItem(ctx, input)

		return out, true, err
	case "deleteItem":
		out, err := e.dynamodb.SFNDeleteItem(ctx, input)

		return out, true, err
	case "updateItem":
		out, err := e.dynamodb.SFNUpdateItem(ctx, input)

		return out, true, err
	case "batchExecuteStatement":
		out, err := e.dynamodb.SFNBatchExecuteStatement(ctx, input)

		return out, true, err
	case "batchGetItem":
		out, err := e.dynamodb.SFNBatchGetItem(ctx, input)

		return out, true, err
	case "batchWriteItem":
		out, err := e.dynamodb.SFNBatchWriteItem(ctx, input)

		return out, true, err
	}

	return nil, false, nil
}

// invokeDynamoDBTableOps handles table-level and backup/policy DynamoDB actions.
// Returns (result, true, err) on a match, or (nil, false, nil) when no action matches.
func (e *Executor) invokeDynamoDBTableOps(
	ctx context.Context,
	action string,
	input any,
) (any, bool, error) {
	switch action {
	case "createTable":
		out, err := e.dynamodb.SFNCreateTable(ctx, input)

		return out, true, err
	case "deleteTable":
		out, err := e.dynamodb.SFNDeleteTable(ctx, input)

		return out, true, err
	case "createGlobalTable":
		out, err := e.dynamodb.SFNCreateGlobalTable(ctx, input)

		return out, true, err
	case "createBackup":
		out, err := e.dynamodb.SFNCreateBackup(ctx, input)

		return out, true, err
	case "deleteBackup":
		out, err := e.dynamodb.SFNDeleteBackup(ctx, input)

		return out, true, err
	case "deleteResourcePolicy":
		out, err := e.dynamodb.SFNDeleteResourcePolicy(ctx, input)

		return out, true, err
	}

	return nil, false, nil
}

// invokeECSTask invokes an ECS RunTask action as a Task state.
func (e *Executor) invokeECSTask(ctx context.Context, state *State, input any) (any, error) {
	if e.ecs == nil {
		return nil, ErrECSIntegrationNotConfigured
	}

	action := serviceAction(state.Resource)

	switch action {
	case "runTask":
		m, _ := input.(map[string]any)

		result, err := e.ecs.SFNRunTask(ctx, m)
		if err != nil {
			return nil, err
		}

		if !isSyncPattern(state.Resource) || e.ecsSyncWaiter == nil {
			return result, nil
		}

		return e.waitForECSSync(ctx, result)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedECSAction, action)
	}
}

// waitForECSSync polls e.ecsSyncWaiter until the task(s) started by runTask
// reach a terminal state, or ctx ends first (its own cancellation, or the
// Task state's TimeoutSeconds deadline -- runTaskAttempt already derives
// ctx's deadline from that, the same bound every other Task type uses). A
// job that never completes and has no TimeoutSeconds set blocks only as
// long as ctx allows, exactly like every other unbounded Task integration
// in this executor.
func (e *Executor) waitForECSSync(ctx context.Context, runTaskResult any) (any, error) {
	var poll ECSSyncPoll

	err := pollUntilDone(ctx, func() (bool, error) {
		p, pollErr := e.ecsSyncWaiter.SFNPollSyncTask(ctx, runTaskResult)
		if pollErr != nil {
			return false, pollErr
		}

		poll = p

		return p.Done, nil
	})
	if err != nil {
		return nil, err
	}

	if poll.Failed {
		return nil, &FailError{ErrCode: errCodeStatesTaskFailed, Cause: poll.FailureReason}
	}

	return poll.Result, nil
}

// invokeGlueTask invokes a Glue action as a Task state.
func (e *Executor) invokeGlueTask(ctx context.Context, state *State, input any) (any, error) {
	if e.glue == nil {
		return nil, ErrGlueIntegrationNotConfigured
	}

	action := serviceAction(state.Resource)

	switch action {
	case "startJobRun":
		m, _ := input.(map[string]any)
		jobName, _ := m["JobName"].(string)

		var arguments map[string]string
		if rawArgs, ok := m["Arguments"].(map[string]any); ok {
			arguments = make(map[string]string, len(rawArgs))
			for k, v := range rawArgs {
				arguments[k], _ = v.(string)
			}
		}

		runID, err := e.glue.SFNStartJobRun(ctx, jobName, arguments)
		if err != nil {
			return nil, err
		}

		result := map[string]any{"JobRunId": runID}

		if !isSyncPattern(state.Resource) || e.glueSyncWaiter == nil {
			return result, nil
		}

		return e.waitForGlueSync(ctx, jobName, runID)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedGlueAction, action)
	}
}

// waitForGlueSync polls e.glueSyncWaiter until the job run reaches a
// terminal JobRunState; see waitForECSSync for the bounding rationale.
func (e *Executor) waitForGlueSync(ctx context.Context, jobName, runID string) (any, error) {
	var poll GlueSyncPoll

	err := pollUntilDone(ctx, func() (bool, error) {
		p, pollErr := e.glueSyncWaiter.SFNPollSyncJobRun(ctx, jobName, runID)
		if pollErr != nil {
			return false, pollErr
		}

		poll = p

		return p.Done, nil
	})
	if err != nil {
		return nil, err
	}

	if poll.Failed {
		return nil, &FailError{ErrCode: errCodeStatesTaskFailed, Cause: poll.FailureReason}
	}

	return poll.Result, nil
}

// syncPollInterval is how often a ".sync" Task state re-checks job status.
const syncPollInterval = 25 * time.Millisecond

// pollUntilDone calls poll every syncPollInterval until it reports done, or
// ctx is done first. The first check happens immediately, before any wait.
func pollUntilDone(ctx context.Context, poll func() (done bool, err error)) error {
	ticker := time.NewTicker(syncPollInterval)
	defer ticker.Stop()

	for {
		done, err := poll()
		if err != nil {
			return err
		}

		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// invokeEventBridgeTask invokes an EventBridge action as a Task state.
func (e *Executor) invokeEventBridgeTask(
	ctx context.Context,
	state *State,
	input any,
) (any, error) {
	if e.eventbridge == nil {
		return nil, ErrEventBridgeIntegrationNotConfigured
	}

	action := serviceAction(state.Resource)

	switch action {
	case "putEvents":
		m, _ := input.(map[string]any)

		var entries []map[string]any
		if rawEntries, ok := m["Entries"].([]any); ok {
			entries = make([]map[string]any, 0, len(rawEntries))
			for _, e := range rawEntries {
				if entry, ok2 := e.(map[string]any); ok2 {
					entries = append(entries, entry)
				}
			}
		}

		failedCount, err := e.eventbridge.SFNPutEvents(ctx, entries)
		if err != nil {
			return nil, err
		}

		return map[string]any{"FailedEntryCount": failedCount, "Entries": []any{}}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedEventBridgeAction, action)
	}
}

// serviceAction extracts the action name from a States service integration ARN,
// stripping any .sync or .waitForTaskToken suffix.
// Example: "arn:aws:states:::sqs:sendMessage.sync" → "sendMessage".
func serviceAction(resource string) string {
	action, _ := parseServiceIntegrationResource(resource)

	return action
}

func isWaitForTaskTokenResource(resource string) bool {
	_, pattern := parseServiceIntegrationResource(resource)

	return pattern == "waitForTaskToken"
}

func isSyncPattern(resource string) bool {
	_, pattern := parseServiceIntegrationResource(resource)

	return pattern == "sync"
}

// checkSyncPatternSupported rejects a ".sync" Resource for a service that AWS
// documents as not supporting the "Run a Job" pattern (see
// ErrSyncPatternUnsupported). Real Step Functions rejects these at
// CreateStateMachine; gopherstack does not validate definitions at creation
// time, so this is enforced at Task execution instead, alongside the existing
// per-service "unsupported action" checks.
func checkSyncPatternSupported(resource string) error {
	_, pattern := parseServiceIntegrationResource(resource)
	if pattern != "sync" {
		return nil
	}

	if isLambdaResource(resource) || isSQSResource(resource) || isSNSResource(resource) ||
		isDynamoDBResource(resource) || isEventBridgeResource(resource) {
		return fmt.Errorf("%w: %q", ErrSyncPatternUnsupported, resource)
	}

	return nil
}

func parseServiceIntegrationResource(resource string) (string, string) {
	parts := strings.Split(resource, ":")
	action := parts[len(parts)-1]
	pattern := ""
	if i := strings.IndexByte(action, '.'); i >= 0 {
		pattern = action[i+1:]
		action = action[:i]
	}

	return action, pattern
}

func (e *Executor) executeParallel(
	ctx context.Context,
	executionARN, stateName string,
	state *State,
	input any,
) (string, any, error) {
	return e.executeWithStateRetryAndCatch(ctx, executionARN, stateName, state, input,
		func(ctx context.Context) (any, error) {
			results := make([]any, len(state.Branches))
			errs := make([]error, len(state.Branches))

			var wg sync.WaitGroup
			for i, branch := range state.Branches {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}

				wg.Go(func() { e.runParallelBranch(ctx, executionARN, branch, input, results, errs, i) })
			}

			wg.Wait()

			if err := ctx.Err(); err != nil {
				return nil, err
			}

			for _, branchErr := range errs {
				if branchErr != nil {
					return nil, branchErr
				}
			}

			return results, nil
		},
	)
}

// executeWithStateRetryAndCatch runs attempt (the full body of a Map or
// Parallel state) applying that state's own Retry and Catch clauses, exactly
// as AWS Step Functions supports Retry/Catch directly on Map and Parallel
// states (not just Task). Per AWS semantics, a retry re-executes the ENTIRE
// state body (all branches / all iterations), not just the failed portion.
func (e *Executor) executeWithStateRetryAndCatch(
	ctx context.Context,
	executionARN, stateName string,
	state *State,
	input any,
	attempt func(ctx context.Context) (any, error),
) (string, any, error) {
	retryAttempts := make([]int, len(state.Retry))

	for {
		result, err := attempt(ctx)
		if err == nil {
			return state.Next, result, nil
		}

		if errors.Is(err, context.Canceled) {
			return "", nil, err
		}

		retried, retryErr := tryRetry(ctx, state, retryAttempts, err)
		if retryErr != nil {
			if !errors.Is(retryErr, ErrStatesTimeout) {
				return "", nil, retryErr
			}

			err = ErrStatesTimeout
		} else if retried {
			continue
		}

		if next, out, matched := e.checkCatchers(executionARN, stateName, state, input, err); matched {
			return next, out, nil
		}

		return "", nil, err
	}
}

// runParallelBranch executes a single Parallel state branch in a goroutine.
func (e *Executor) runParallelBranch(
	ctx context.Context,
	executionARN string,
	branch Branch,
	input any,
	results []any,
	errs []error,
	idx int,
) {
	if err := e.execSem.Acquire(ctx, 1); err != nil {
		return
	}
	defer e.execSem.Release(1)

	branchSM := &StateMachine{StartAt: branch.StartAt, States: branch.States}
	exec := e.newBranchExecutor(branchSM, fmt.Sprintf("Branch-%d", idx))
	res, err := exec.Execute(ctx, executionARN, marshalInput(input))
	if err != nil {
		errs[idx] = err

		return
	}

	if res.Failed {
		errs[idx] = &FailError{ErrCode: res.Error, Cause: res.Cause}

		return
	}

	results[idx] = res.Output
}

const maxMapConcurrencyLimit = 40

// percentToFractionDivisor converts a 0-100 percentage into a 0-1 fraction.
const percentToFractionDivisor = 100

// jitterStrategyFull is the ASL Retry.JitterStrategy value that randomizes
// the computed backoff delay uniformly in [0, delay]. AWS's default, when
// JitterStrategy is omitted, is "NONE" (no randomization).
const jitterStrategyFull = "FULL"

// executeMap handles Map state: iterates over an array. Like Task and
// Parallel, Map supports Retry/Catch on the Map state itself (in addition to
// per-iteration failures counted against ToleratedFailure*); a retry re-runs
// every item from scratch, matching AWS Map/Distributed-Map semantics.
// pathInput is the pre-Parameters input (see executeState); every *Path
// field (ItemsPath, ItemBatcher's and ReaderConfig's *Path siblings,
// MaxConcurrencyPath, ToleratedFailure*Path) resolves against it, matching
// the ASL Map State spec section, which resolves ItemsPath/ItemReader
// against "the effective input" (post-InputPath, pre-Parameters) -- the
// state's own Parameters field is the deprecated ItemSelector alias applied
// per item, not to this scope.
func (e *Executor) executeMap(
	ctx context.Context,
	executionARN string,
	stateName string,
	state *State,
	pathInput, input any,
) (string, any, error) {
	iterator, iterErr := e.getMapIterator(state)
	if iterErr != nil {
		return "", nil, iterErr
	}

	return e.executeWithStateRetryAndCatch(ctx, executionARN, stateName, state, input,
		func(ctx context.Context) (any, error) {
			items, err := e.resolveMapItems(ctx, state, pathInput)
			if err != nil {
				return nil, err
			}

			if len(state.ItemSelector) > 0 {
				items, err = applyMapItemSelector(state.ItemSelector, items)
				if err != nil {
					return nil, err
				}
			}

			// Apply ItemBatcher: wrap items into batches; each batch is one Map iteration.
			if state.ItemBatcher != nil {
				maxItemsPerBatch, maxInputBytesPerBatch, batcherErr := e.resolveItemBatcherLimits(
					state.ItemBatcher, pathInput,
				)
				if batcherErr != nil {
					return nil, batcherErr
				}

				rawBatches := batchItems(items, maxItemsPerBatch, maxInputBytesPerBatch)

				batched, wrapErr := wrapItemBatcherBatches(rawBatches, state.ItemBatcher.BatchInput)
				if wrapErr != nil {
					return nil, wrapErr
				}

				return e.runMapItemsAndFinalize(ctx, executionARN, stateName, iterator, state, pathInput, batched)
			}

			return e.runMapItemsAndFinalize(ctx, executionARN, stateName, iterator, state, pathInput, items)
		},
	)
}

// runMapItemsAndFinalize runs iterator over items (or pre-built batches) at
// the resolved concurrency, notifies the MapRunNotifier (if configured), and
// finalizes the result, applying any ToleratedFailure* threshold.
func (e *Executor) runMapItemsAndFinalize(
	ctx context.Context,
	executionARN, stateName string,
	iterator *StateMachine,
	state *State,
	mapInput any,
	items []any,
) (any, error) {
	results := make([]any, len(items))
	errs := make([]error, len(items))

	maxConcurrency, err := e.resolveMaxConcurrency(state, mapInput)
	if err != nil {
		return nil, err
	}

	concurrency := resolveMapConcurrency(maxConcurrency, len(items))

	var mapRunARN string
	if e.mapRunNotifier != nil {
		mapRunARN = e.mapRunNotifier.OnMapRunStart(executionARN, stateName, maxConcurrency, len(items))
	}

	e.runMapTasks(ctx, executionARN, iterator, items, results, errs, concurrency)

	out, finalErr := e.finalizeMap(ctx, state, mapInput, results, errs)

	resultsWritten := 0
	if finalErr == nil && state.ResultWriter != nil {
		out, resultsWritten, finalErr = e.exportMapResults(ctx, state, stateName, mapRunARN, items, results, errs)
	}

	if e.mapRunNotifier != nil && mapRunARN != "" {
		succeeded, failed := countMapResults(errs)
		status := "SUCCEEDED"

		if finalErr != nil {
			status = "FAILED"
		}

		e.mapRunNotifier.OnMapRunEnd(mapRunARN, status, succeeded, failed, len(items), resultsWritten)
	}

	return out, finalErr
}

// ErrMaxItemsPerBatchPathNotNumber is returned when ItemBatcher.MaxItemsPerBatchPath
// does not resolve to a number.
var ErrMaxItemsPerBatchPathNotNumber = errors.New("MaxItemsPerBatchPath: value is not a number")

// ErrMaxInputBytesPerBatchPathNotNumber is returned when
// ItemBatcher.MaxInputBytesPerBatchPath does not resolve to a number.
var ErrMaxInputBytesPerBatchPathNotNumber = errors.New(
	"MaxInputBytesPerBatchPath: value is not a number",
)

// resolveItemBatcherLimits resolves ItemBatcher's MaxItemsPerBatch(Path) and
// MaxInputBytesPerBatch(Path) against the Map state's pre-Parameters input,
// the same way resolveToleratedFailureCount resolves ToleratedFailureCountPath.
func (e *Executor) resolveItemBatcherLimits(batcher *ItemBatcher, mapInput any) (int, int, error) {
	maxItems := batcher.MaxItemsPerBatch

	if batcher.MaxItemsPerBatchPath != "" {
		val, err := applyPath(batcher.MaxItemsPerBatchPath, mapInput, e.jsonPathCache)
		if err != nil {
			return 0, 0, fmt.Errorf("MaxItemsPerBatchPath error: %w", err)
		}

		f, ok := toFloat(val)
		if !ok {
			return 0, 0, ErrMaxItemsPerBatchPathNotNumber
		}

		maxItems = int(f)
	}

	maxBytes := batcher.MaxInputBytesPerBatch

	if batcher.MaxInputBytesPerBatchPath != "" {
		val, err := applyPath(batcher.MaxInputBytesPerBatchPath, mapInput, e.jsonPathCache)
		if err != nil {
			return 0, 0, fmt.Errorf("MaxInputBytesPerBatchPath error: %w", err)
		}

		f, ok := toFloat(val)
		if !ok {
			return 0, 0, ErrMaxInputBytesPerBatchPathNotNumber
		}

		maxBytes = int(f)
	}

	return maxItems, maxBytes, nil
}

// batchItems groups items into batches of at most maxItemsPerBatch items and
// maxInputBytesPerBatch bytes (either limit <= 0 means unlimited). Each batch
// is returned as []any and becomes one iteration input.
func batchItems(items []any, maxItemsPerBatch, maxInputBytesPerBatch int) []any {
	if maxItemsPerBatch <= 0 && maxInputBytesPerBatch <= 0 {
		result := make([]any, len(items))
		for i, it := range items {
			result[i] = []any{it}
		}

		return result
	}

	var batches []any

	var current []any

	currentBytes := 0

	for _, item := range items {
		itemBytes := len(marshalInput(item))

		itemsFull := maxItemsPerBatch > 0 && len(current) >= maxItemsPerBatch
		bytesFull := maxInputBytesPerBatch > 0 &&
			len(current) > 0 &&
			currentBytes+itemBytes > maxInputBytesPerBatch

		if itemsFull || bytesFull {
			batches = append(batches, current)
			current = nil
			currentBytes = 0
		}

		current = append(current, item)
		currentBytes += itemBytes
	}

	if len(current) > 0 {
		batches = append(batches, current)
	}

	return batches
}

// wrapItemBatcherBatches converts raw item batches into the AWS Map
// iteration-input shape for ItemBatcher: {"Items": [...]}, plus a
// "BatchInput" field merged in when ItemBatcher.BatchInput is set (AWS docs:
// input-output-itembatcher.html, "Batch input"). Previously each batch was
// passed through as a bare array, which is not the shape AWS sends to child
// executions.
func wrapItemBatcherBatches(rawBatches []any, batchInput json.RawMessage) ([]any, error) {
	var decodedBatchInput any
	if len(batchInput) > 0 {
		if err := json.Unmarshal(batchInput, &decodedBatchInput); err != nil {
			return nil, fmt.Errorf("ItemBatcher.BatchInput error: %w", err)
		}
	}

	wrapped := make([]any, len(rawBatches))

	for i, batch := range rawBatches {
		entry := map[string]any{"Items": batch}
		if len(batchInput) > 0 {
			entry["BatchInput"] = decodedBatchInput
		}

		wrapped[i] = entry
	}

	return wrapped, nil
}

func applyMapItemSelector(itemSelector json.RawMessage, items []any) ([]any, error) {
	selectedItems := make([]any, len(items))
	for idx, item := range items {
		contextInput := pathEvalInput{
			data: item,
			context: map[string]any{
				"Map": map[string]any{
					"Item": map[string]any{
						"Index": float64(idx),
						"Value": item,
					},
				},
			},
		}

		selected, err := applyParametersTemplate(itemSelector, contextInput)
		if err != nil {
			return nil, fmt.Errorf("map ItemSelector error: %w", err)
		}

		selectedItems[idx] = selected
	}

	return selectedItems, nil
}

// runMapItem executes a single map iteration item in the sub-executor.
// The sub-executor exposes `$$.Map.Item.Index` and `$$.Map.Item.Value` via the context.
func (e *Executor) runMapItem(
	ctx context.Context,
	executionARN string,
	iterator *StateMachine,
	idx int,
	it any,
	results []any,
	errs []error,
) {
	exec := e.newMapItemExecutor(iterator, idx, it)

	res, execErr := exec.Execute(ctx, executionARN, marshalInput(it))
	if execErr != nil {
		errs[idx] = execErr

		return
	}

	// A Fail state (or an unhandled Task failure) inside the iterator
	// surfaces as a successful Execute() call with res.Failed set rather
	// than a Go error — mirror runParallelBranch's handling so Map
	// iteration failures propagate to the Map state's own Catch/Retry
	// instead of silently producing a nil result for the failed item.
	if res.Failed {
		errs[idx] = &FailError{ErrCode: res.Error, Cause: res.Cause}

		return
	}

	results[idx] = res.Output
}

func (e *Executor) getMapIterator(state *State) (*StateMachine, error) {
	iterator := state.Iterator
	if iterator == nil {
		iterator = state.ItemProcessor
	}

	if iterator == nil {
		return nil, ErrMapRequiresIterator
	}

	return iterator, nil
}

// ErrS3ReaderNotConfigured is returned when ItemReader requires S3 but no S3Reader is set.
var ErrS3ReaderNotConfigured = errors.New("S3 reader not configured for Map state ItemReader")

// ErrItemReaderInvalidData is returned when ItemReader S3 object cannot be parsed as items.
var ErrItemReaderInvalidData = errors.New(
	"ItemReader: unable to parse S3 object as JSON array or JSON lines",
)

func (e *Executor) resolveMapItems(ctx context.Context, state *State, input any) ([]any, error) {
	if state.ItemReader != nil {
		items, err := e.resolveItemsFromReader(ctx, state.ItemReader)
		if err != nil {
			return nil, err
		}

		return e.truncateReaderItems(items, state.ItemReader.ReaderConfig, input)
	}

	items, err := resolveItems(state.ItemsPath, input)
	if err != nil {
		return nil, fmt.Errorf("map ItemsPath error: %w", err)
	}

	return items, nil
}

// ErrMaxItemsPathNotNumber is returned when ReaderConfig.MaxItemsPath does
// not resolve to a number.
var ErrMaxItemsPathNotNumber = errors.New("MaxItemsPath: value is not a number")

// truncateReaderItems applies ReaderConfig's MaxItems(Path) cap, resolving
// MaxItemsPath against the Map state's pre-Parameters input the same way
// resolveToleratedFailureCount resolves ToleratedFailureCountPath.
func (e *Executor) truncateReaderItems(items []any, cfg *ReaderConfig, mapInput any) ([]any, error) {
	if cfg == nil {
		return items, nil
	}

	maxItems := cfg.MaxItems

	if cfg.MaxItemsPath != "" {
		val, err := applyPath(cfg.MaxItemsPath, mapInput, e.jsonPathCache)
		if err != nil {
			return nil, fmt.Errorf("MaxItemsPath error: %w", err)
		}

		f, ok := toFloat(val)
		if !ok {
			return nil, ErrMaxItemsPathNotNumber
		}

		maxItems = int(f)
	}

	if maxItems > 0 && len(items) > maxItems {
		items = items[:maxItems]
	}

	return items, nil
}

// resolveItemsFromReader reads items from S3 using the ItemReader configuration.
// Supports JSON arrays, newline-delimited JSON (JSON Lines), and CSV.
func (e *Executor) resolveItemsFromReader(ctx context.Context, reader *ItemReader) ([]any, error) {
	if e.s3 == nil {
		return nil, ErrS3ReaderNotConfigured
	}

	bucket, _ := reader.Parameters["Bucket"].(string)
	key, _ := reader.Parameters["Key"].(string)

	data, err := e.s3.GetObjectBytes(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("ItemReader S3 get error: %w", err)
	}

	return decodeReaderItems(data, reader.ReaderConfig)
}

// decodeReaderItems parses S3 object bytes into Map items based on the
// ReaderConfig's InputType. Defaults to JSON-array-then-JSON-lines auto-detection.
func decodeReaderItems(data []byte, cfg *ReaderConfig) ([]any, error) {
	inputType := ""
	if cfg != nil {
		inputType = strings.ToUpper(cfg.InputType)
	}

	switch inputType {
	case "CSV":
		return decodeCSVItems(data, cfg)
	case "JSONL", "JSON_LINES":
		return decodeJSONLines(data)
	case "", "JSON":
		return decodeJSONAuto(data)
	default:
		return nil, fmt.Errorf("%w: unsupported InputType %q", ErrItemReaderInvalidData, inputType)
	}
}

func decodeJSONAuto(data []byte) ([]any, error) {
	var arr []any
	if jsonErr := json.Unmarshal(data, &arr); jsonErr == nil {
		return arr, nil
	}

	return decodeJSONLines(data)
}

func decodeJSONLines(data []byte) ([]any, error) {
	lines := strings.SplitSeq(strings.TrimSpace(string(data)), "\n")

	items := []any{}

	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var item any
		if jsonErr := json.Unmarshal([]byte(line), &item); jsonErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrItemReaderInvalidData, jsonErr)
		}

		items = append(items, item)
	}

	return items, nil
}

func decodeCSVItems(data []byte, cfg *ReaderConfig) ([]any, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrItemReaderInvalidData, err)
	}

	headers, dataRows, err := resolveCSVHeaders(rows, cfg)
	if err != nil {
		return nil, err
	}

	items := make([]any, 0, len(dataRows))
	for _, row := range dataRows {
		obj := make(map[string]any, len(headers))

		for i, header := range headers {
			if i < len(row) {
				obj[header] = row[i]
			} else {
				obj[header] = ""
			}
		}

		items = append(items, obj)
	}

	return items, nil
}

func resolveCSVHeaders(rows [][]string, cfg *ReaderConfig) ([]string, [][]string, error) {
	loc := ""
	if cfg != nil {
		loc = strings.ToUpper(cfg.CSVHeaderLocation)
	}

	switch loc {
	case "GIVEN":
		if cfg == nil || len(cfg.CSVHeaders) == 0 {
			return nil, nil, fmt.Errorf(
				"%w: CSVHeaders required when CSVHeaderLocation=GIVEN",
				ErrItemReaderInvalidData,
			)
		}

		return cfg.CSVHeaders, rows, nil
	case "", "FIRST_ROW":
		if len(rows) == 0 {
			return nil, nil, nil
		}

		return rows[0], rows[1:], nil
	default:
		return nil, nil, fmt.Errorf(
			"%w: unsupported CSVHeaderLocation %q",
			ErrItemReaderInvalidData,
			loc,
		)
	}
}

func (e *Executor) runMapTasks(
	ctx context.Context,
	executionARN string,
	iterator *StateMachine,
	items []any,
	results []any,
	errs []error,
	concurrency int,
) {
	sem := semaphore.NewWeighted(int64(concurrency))
	var wg sync.WaitGroup

	for i, item := range items {
		if ctx.Err() != nil {
			break
		}

		e.spawnMapTask(ctx, executionARN, iterator, i, item, results, errs, sem, &wg)
	}

	wg.Wait()
}

func (e *Executor) spawnMapTask(
	ctx context.Context,
	executionARN string,
	iterator *StateMachine,
	idx int,
	item any,
	results []any,
	errs []error,
	sem *semaphore.Weighted,
	wg *sync.WaitGroup,
) {
	wg.Go(func() {
		if err := sem.Acquire(ctx, 1); err != nil {
			return
		}
		defer sem.Release(1)

		e.runMapItem(ctx, executionARN, iterator, idx, item, results, errs)
	})
}

func (e *Executor) finalizeMap(
	ctx context.Context,
	state *State,
	mapInput any,
	results []any,
	errs []error,
) (any, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	failedCount := 0

	var firstErr error

	for _, iterErr := range errs {
		if iterErr != nil {
			failedCount++

			if firstErr == nil {
				firstErr = iterErr
			}
		}
	}

	if failedCount == 0 {
		return results, nil
	}

	threshold, err := e.resolveToleratedFailureThreshold(state, mapInput, len(errs))
	if err != nil {
		return nil, err
	}

	if failedCount <= threshold {
		return results, nil
	}

	// No tolerance configured (the common case): preserve the original
	// per-item error so Catch/Retry on the Map state can match on it.
	if threshold == 0 && state.ToleratedFailureCount == nil &&
		state.ToleratedFailureCountPath == "" &&
		state.ToleratedFailurePercentage == nil &&
		state.ToleratedFailurePercentagePath == "" {
		return nil, firstErr
	}

	return nil, &FailError{
		ErrCode: errCodeStatesExceedToleratedFailureThreshold,
		Cause: fmt.Sprintf(
			"Map Run failed: %d of %d items failed, exceeding the tolerated failure threshold of %d",
			failedCount, len(errs), threshold,
		),
	}
}

// resolveToleratedFailureThreshold computes the maximum number of failed Map
// iterations tolerated before the Map state fails. Returns 0 (no tolerance)
// when neither ToleratedFailureCount(Path) nor ToleratedFailurePercentage(Path)
// is configured. When both are configured, the more restrictive (lower)
// threshold wins, matching AWS "fails when either is crossed" semantics.
func (e *Executor) resolveToleratedFailureThreshold(
	state *State,
	mapInput any,
	totalCount int,
) (int, error) {
	threshold := totalCount
	hasThreshold := false

	countThreshold, countConfigured, err := e.resolveToleratedFailureCount(state, mapInput)
	if err != nil {
		return 0, err
	}

	if countConfigured {
		threshold = countThreshold
		hasThreshold = true
	}

	pctThreshold, pctConfigured, err := e.resolveToleratedFailurePercentageThreshold(state, mapInput, totalCount)
	if err != nil {
		return 0, err
	}

	if pctConfigured && (!hasThreshold || pctThreshold < threshold) {
		threshold = pctThreshold
		hasThreshold = true
	}

	if !hasThreshold {
		return 0, nil
	}

	return threshold, nil
}

// resolveToleratedFailureCount resolves ToleratedFailureCount(Path). The
// second return value reports whether either field was configured.
func (e *Executor) resolveToleratedFailureCount(state *State, mapInput any) (int, bool, error) {
	switch {
	case state.ToleratedFailureCountPath != "":
		val, err := applyPath(state.ToleratedFailureCountPath, mapInput, e.jsonPathCache)
		if err != nil {
			return 0, false, fmt.Errorf("ToleratedFailureCountPath error: %w", err)
		}

		f, ok := toFloat(val)
		if !ok {
			return 0, false, ErrToleratedFailureCountNotNumber
		}

		return int(f), true, nil
	case state.ToleratedFailureCount != nil:
		return *state.ToleratedFailureCount, true, nil
	default:
		return 0, false, nil
	}
}

// resolveToleratedFailurePercentageThreshold resolves ToleratedFailurePercentage(Path)
// into an absolute failure-count threshold for totalCount items. The second
// return value reports whether either field was configured.
func (e *Executor) resolveToleratedFailurePercentageThreshold(
	state *State,
	mapInput any,
	totalCount int,
) (int, bool, error) {
	var pct float64

	switch {
	case state.ToleratedFailurePercentagePath != "":
		val, err := applyPath(state.ToleratedFailurePercentagePath, mapInput, e.jsonPathCache)
		if err != nil {
			return 0, false, fmt.Errorf("ToleratedFailurePercentagePath error: %w", err)
		}

		f, ok := toFloat(val)
		if !ok {
			return 0, false, ErrToleratedFailurePercentageNotNumber
		}

		pct = f
	case state.ToleratedFailurePercentage != nil:
		pct = *state.ToleratedFailurePercentage
	default:
		return 0, false, nil
	}

	return int(math.Floor(float64(totalCount) * pct / percentToFractionDivisor)), true, nil
}

// ErrMaxConcurrencyPathNotNumber is returned when State.MaxConcurrencyPath
// does not resolve to a number.
var ErrMaxConcurrencyPathNotNumber = errors.New("MaxConcurrencyPath: value is not a number")

// resolveMaxConcurrency resolves MaxConcurrency(Path) against the Map
// state's pre-Parameters input, the same way resolveToleratedFailureCount
// resolves ToleratedFailureCountPath.
func (e *Executor) resolveMaxConcurrency(state *State, mapInput any) (int, error) {
	if state.MaxConcurrencyPath == "" {
		return state.MaxConcurrency, nil
	}

	val, err := applyPath(state.MaxConcurrencyPath, mapInput, e.jsonPathCache)
	if err != nil {
		return 0, fmt.Errorf("MaxConcurrencyPath error: %w", err)
	}

	f, ok := toFloat(val)
	if !ok {
		return 0, ErrMaxConcurrencyPathNotNumber
	}

	return int(f), nil
}

// ErrTimeoutSecondsPathNotNumber is returned when State.TimeoutSecondsPath
// does not resolve to a number.
var ErrTimeoutSecondsPathNotNumber = errors.New("TimeoutSecondsPath: value is not a number")

// ErrHeartbeatSecondsPathNotNumber is returned when State.HeartbeatSecondsPath
// does not resolve to a number.
var ErrHeartbeatSecondsPathNotNumber = errors.New("HeartbeatSecondsPath: value is not a number")

// resolveTaskTimeoutSeconds resolves TimeoutSeconds(Path) against the Task
// state's pre-Parameters input (input after InputPath, before Parameters is
// applied) -- AWS docs amazon-states-language-task-state.html's GlueJobTask
// example pairs a Parameters block that replaces the whole payload with a
// TimeoutSecondsPath that still reads the pre-Parameters field. Retries
// reuse this same input (ASL never re-evaluates a Task's input between
// retry attempts), so resolving once before the retry loop gives the same
// value every attempt.
func (e *Executor) resolveTaskTimeoutSeconds(state *State, input any) (int, error) {
	if state.TimeoutSecondsPath == "" {
		return state.TimeoutSeconds, nil
	}

	val, err := applyPath(state.TimeoutSecondsPath, input, e.jsonPathCache)
	if err != nil {
		return 0, fmt.Errorf("TimeoutSecondsPath error: %w", err)
	}

	f, ok := toFloat(val)
	if !ok {
		return 0, ErrTimeoutSecondsPathNotNumber
	}

	return int(f), nil
}

// resolveTaskHeartbeatSeconds resolves HeartbeatSeconds(Path) against the
// Task state's own input; see resolveTaskTimeoutSeconds.
func (e *Executor) resolveTaskHeartbeatSeconds(state *State, input any) (int, error) {
	if state.HeartbeatSecondsPath == "" {
		return state.HeartbeatSeconds, nil
	}

	val, err := applyPath(state.HeartbeatSecondsPath, input, e.jsonPathCache)
	if err != nil {
		return 0, fmt.Errorf("HeartbeatSecondsPath error: %w", err)
	}

	f, ok := toFloat(val)
	if !ok {
		return 0, ErrHeartbeatSecondsPathNotNumber
	}

	return int(f), nil
}

// resolveMapConcurrency determines the effective concurrency for a Map state.
func resolveMapConcurrency(maxConcurrency, itemCount int) int {
	if maxConcurrency > 0 {
		return maxConcurrency
	}

	c := min(itemCount, maxMapConcurrencyLimit)
	if c == 0 {
		return 1
	}

	return c
}

// FailError represents the error from a Fail state.
type FailError struct {
	ErrCode string
	Cause   string
}

func (e *FailError) Error() string {
	if e.Cause != "" {
		return fmt.Sprintf("%s: %s", e.ErrCode, e.Cause)
	}

	return e.ErrCode
}

// pathEvalInput wraps path evaluation scope.
// data is target for "$" paths; context is target for "$$" paths.
type pathEvalInput struct {
	data    any
	context any
}

// applyPath applies an InputPath or OutputPath to a value.
// If path is "" or "$", returns value unchanged.
// If path starts with "$.", it's a JSONPath reference.
func applyPath(path string, value any, pathCache ...*jsonPathCache) (any, error) {
	var cache *jsonPathCache
	if len(pathCache) > 0 {
		cache = pathCache[0]
	}

	pathInput, ok := value.(pathEvalInput)
	if !ok {
		pathInput = pathEvalInput{
			data:    value,
			context: value,
		}
	}

	if path == "" || path == "$" {
		return pathInput.data, nil
	}

	if path == "$$" {
		return pathInput.context, nil
	}

	// Simple dot-notation JSONPath: $.field.subfield
	if strings.HasPrefix(path, "$.") {
		return jsonPathGet(path[2:], pathInput.data, cache)
	}

	if strings.HasPrefix(path, "$$.") {
		return jsonPathGet(path[3:], pathInput.context, cache)
	}

	return nil, fmt.Errorf("%w: %q", ErrUnsupportedPathExpr, path)
}

// applyResultPath merges result into input according to ResultPath.
// If ResultPath is "", result replaces input entirely.
// If ResultPath is "$", result replaces input entirely (default).
// If ResultPath is "$.field", result is written to input[field].
// If ResultPath is "null", result is discarded (input passes through).
func applyResultPath(resultPath string, input, result any) (any, error) {
	if resultPath == "null" {
		return input, nil
	}

	if resultPath == "" || resultPath == "$" {
		return result, nil
	}

	if !strings.HasPrefix(resultPath, "$.") {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedResultPath, resultPath)
	}

	field := resultPath[2:]
	inputMap, ok := input.(map[string]any)
	if !ok {
		// If input is not a map, create a new one.
		inputMap = make(map[string]any)
	} else {
		// Copy to avoid mutation.
		inputCopy := make(map[string]any, len(inputMap))
		maps.Copy(inputCopy, inputMap)
		inputMap = inputCopy
	}

	// Support nested paths like $.a.b.
	const splitFieldParts = 2
	parts := strings.SplitN(field, ".", splitFieldParts)
	if len(parts) == 1 {
		inputMap[field] = result
	} else {
		sub, subErr := applyResultPath("$."+parts[1], inputMap[parts[0]], result)
		if subErr != nil {
			return nil, subErr
		}
		inputMap[parts[0]] = sub
	}

	return inputMap, nil
}

// jsonPathGet performs a simple dot-notation JSONPath get on a value.
func jsonPathGet(path string, value any, pathCache *jsonPathCache) (any, error) {
	if path == "" {
		return value, nil
	}

	parts := getCachedJSONPathParts(path, pathCache)
	for remainingIndex, part := range parts {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"%w: %q",
				ErrCannotIndexNonObject,
				strings.Join(parts[remainingIndex:], "."),
			)
		}

		child, ok := m[part]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrFieldNotFound, part)
		}

		value = child
	}

	return value, nil
}

// getCachedJSONPathParts returns cached JSONPath segments or computes and stores
// them when unavailable.
func getCachedJSONPathParts(path string, pathCache *jsonPathCache) []string {
	if pathCache == nil {
		return strings.Split(path, ".")
	}

	if parts, found := pathCache.load(path); found {
		return parts
	}

	parts := strings.Split(path, ".")
	pathCache.store(path, parts)

	return parts
}

// evaluateChoiceRule checks whether a ChoiceRule matches the given input.
func evaluateChoiceRule(rule *ChoiceRule, input any, pathCache *jsonPathCache) bool {
	if result, handled := evaluateLogicalOp(rule, input, pathCache); handled {
		return result
	}

	if rule.Variable == "" {
		return false
	}

	return evaluateVariableComparison(rule, input, pathCache)
}

// evaluateLogicalOp handles And/Or/Not logical operators.
// Returns (result, true) if a logical operator was found, or (false, false) otherwise.
func evaluateLogicalOp(rule *ChoiceRule, input any, pathCache *jsonPathCache) (bool, bool) {
	if len(rule.And) > 0 {
		for i := range rule.And {
			if !evaluateChoiceRule(&rule.And[i], input, pathCache) {
				return false, true
			}
		}

		return true, true
	}

	if len(rule.Or) > 0 {
		for i := range rule.Or {
			if evaluateChoiceRule(&rule.Or[i], input, pathCache) {
				return true, true
			}
		}

		return false, true
	}

	if rule.Not != nil {
		return !evaluateChoiceRule(rule.Not, input, pathCache), true
	}

	return false, false
}

// evaluateVariableComparison resolves the variable and checks condition comparisons.
func evaluateVariableComparison(rule *ChoiceRule, input any, pathCache *jsonPathCache) bool {
	varVal, err := applyPath(rule.Variable, input, pathCache)
	if err != nil {
		if rule.IsPresent != nil {
			return !*rule.IsPresent
		}

		return false
	}

	if rule.IsPresent != nil {
		return *rule.IsPresent
	}

	if rule.IsNull != nil {
		return (varVal == nil) == *rule.IsNull
	}

	// Type checks do not require IsNull handling above.
	if rule.IsString != nil {
		_, ok := varVal.(string)

		return ok == *rule.IsString
	}

	if rule.IsNumeric != nil {
		_, ok := toFloat(varVal)

		return ok == *rule.IsNumeric
	}

	if rule.IsBoolean != nil {
		_, ok := varVal.(bool)

		return ok == *rule.IsBoolean
	}

	if rule.IsTimestamp != nil {
		s, ok := varVal.(string)
		if !ok {
			return !*rule.IsTimestamp
		}
		_, parseErr := time.Parse(time.RFC3339, s)

		return (parseErr == nil) == *rule.IsTimestamp
	}

	return matchVariableCondition(rule, varVal, input, pathCache)
}

// matchVariableCondition checks all value-comparison conditions by delegating to
// type-specific sub-functions.
func matchVariableCondition(rule *ChoiceRule, varVal, input any, pathCache *jsonPathCache) bool {
	if result, matched := matchStringCondition(rule, varVal, input, pathCache); matched {
		return result
	}

	if result, matched := matchNumericCondition(rule, varVal, input, pathCache); matched {
		return result
	}

	if result, matched := matchBooleanCondition(rule, varVal, input, pathCache); matched {
		return result
	}

	if result, matched := matchTimestampCondition(rule, varVal, input, pathCache); matched {
		return result
	}

	return false
}

// matchStringCondition checks string comparison conditions.
func matchStringCondition(
	rule *ChoiceRule,
	varVal, input any,
	pathCache *jsonPathCache,
) (bool, bool) {
	if result, matched := matchStringLiteralCondition(rule, varVal); matched {
		return result, matched
	}

	return matchStringPathCondition(rule, varVal, input, pathCache)
}

// matchStringLiteralCondition checks direct string comparison conditions.
func matchStringLiteralCondition(rule *ChoiceRule, varVal any) (bool, bool) {
	s, ok := varVal.(string)

	switch {
	case rule.StringEquals != nil:
		return ok && s == *rule.StringEquals, true
	case rule.StringLessThan != nil:
		return ok && s < *rule.StringLessThan, true
	case rule.StringGreaterThan != nil:
		return ok && s > *rule.StringGreaterThan, true
	case rule.StringLessThanEquals != nil:
		return ok && s <= *rule.StringLessThanEquals, true
	case rule.StringGreaterThanEquals != nil:
		return ok && s >= *rule.StringGreaterThanEquals, true
	case rule.StringMatches != nil:
		return ok && stringMatchesPattern(s, *rule.StringMatches), true
	}

	return false, false
}

// stringMatchesPattern implements the AWS Step Functions StringMatches glob
// comparator. The wildcard '*' matches zero or more characters. A backslash
// escapes the next character, so "\\*" matches a literal asterisk and "\\\\"
// matches a literal backslash. The match is anchored at both ends.
func stringMatchesPattern(s, pattern string) bool {
	return globMatch(s, pattern)
}

// globMatch performs anchored wildcard matching with backslash escaping using a
// two-pointer algorithm with backtracking. This is linear in practice and avoids
// catastrophic backtracking on patterns with many wildcards.
func globMatch(s, pattern string) bool {
	si, pi := 0, 0
	starPi, starSi := -1, 0

	for si < len(s) {
		switch {
		case pi < len(pattern) && pattern[pi] == '\\' && pi+1 < len(pattern):
			// Escaped literal: the character after the backslash must match exactly.
			if s[si] == pattern[pi+1] {
				si++
				pi += 2

				continue
			}
		case pi < len(pattern) && pattern[pi] == '*':
			// Record the wildcard position so we can backtrack and consume more of s.
			starPi = pi
			starSi = si
			pi++

			continue
		case pi < len(pattern) && pattern[pi] == s[si]:
			si++
			pi++

			continue
		}

		// Mismatch: backtrack to the last '*', expanding what it consumes by one.
		if starPi == -1 {
			return false
		}

		pi = starPi + 1
		starSi++
		si = starSi
	}

	// Consume any trailing wildcards in the pattern.
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}

	return pi == len(pattern)
}

// matchStringPathCondition checks path-based string comparison conditions.
func matchStringPathCondition(
	rule *ChoiceRule,
	varVal, input any,
	pathCache *jsonPathCache,
) (bool, bool) {
	comparePath := func(path *string, cmp func(s1, s2 string) bool) (bool, bool) {
		if path == nil {
			return false, false
		}

		ref, err := applyPath(*path, input, pathCache)
		if err != nil {
			return false, true
		}

		s1, ok1 := varVal.(string)
		s2, ok2 := ref.(string)

		return ok1 && ok2 && cmp(s1, s2), true
	}

	if r, m := comparePath(rule.StringEqualsPath, func(a, b string) bool { return a == b }); m {
		return r, m
	}

	if r, m := comparePath(rule.StringLessThanPath, func(a, b string) bool { return a < b }); m {
		return r, m
	}

	if r, m := comparePath(rule.StringGreaterThanPath, func(a, b string) bool { return a > b }); m {
		return r, m
	}

	if r, m := comparePath(rule.StringLessThanEqualsPath, func(a, b string) bool { return a <= b }); m {
		return r, m
	}

	if r, m := comparePath(rule.StringGreaterThanEqualsPath, func(a, b string) bool { return a >= b }); m {
		return r, m
	}

	return false, false
}

// matchNumericCondition checks numeric comparison conditions.
func matchNumericCondition(
	rule *ChoiceRule,
	varVal, input any,
	pathCache *jsonPathCache,
) (bool, bool) {
	if result, matched := matchNumericLiteralCondition(rule, varVal); matched {
		return result, matched
	}

	return matchNumericPathCondition(rule, varVal, input, pathCache)
}

// matchNumericLiteralCondition checks direct numeric comparison conditions.
func matchNumericLiteralCondition(rule *ChoiceRule, varVal any) (bool, bool) {
	n, ok := toFloat(varVal)

	switch {
	case rule.NumericEquals != nil:
		return ok && n == *rule.NumericEquals, true
	case rule.NumericLessThan != nil:
		return ok && n < *rule.NumericLessThan, true
	case rule.NumericGreaterThan != nil:
		return ok && n > *rule.NumericGreaterThan, true
	case rule.NumericLessThanEquals != nil:
		return ok && n <= *rule.NumericLessThanEquals, true
	case rule.NumericGreaterThanEquals != nil:
		return ok && n >= *rule.NumericGreaterThanEquals, true
	}

	return false, false
}

// matchNumericPathCondition checks path-based numeric comparison conditions.
func matchNumericPathCondition(
	rule *ChoiceRule,
	varVal, input any,
	pathCache *jsonPathCache,
) (bool, bool) {
	compareNumPath := func(path *string, cmp func(n1, n2 float64) bool) (bool, bool) {
		if path == nil {
			return false, false
		}

		ref, err := applyPath(*path, input, pathCache)
		if err != nil {
			return false, true
		}

		n1, ok1 := toFloat(varVal)
		n2, ok2 := toFloat(ref)

		return ok1 && ok2 && cmp(n1, n2), true
	}

	if r, m := compareNumPath(rule.NumericEqualsPath, func(a, b float64) bool { return a == b }); m {
		return r, m
	}

	if r, m := compareNumPath(rule.NumericLessThanPath, func(a, b float64) bool { return a < b }); m {
		return r, m
	}

	if r, m := compareNumPath(rule.NumericGreaterThanPath, func(a, b float64) bool { return a > b }); m {
		return r, m
	}

	if r, m := compareNumPath(rule.NumericLessThanEqualsPath, func(a, b float64) bool { return a <= b }); m {
		return r, m
	}

	if r, m := compareNumPath(rule.NumericGreaterThanEqualsPath, func(a, b float64) bool { return a >= b }); m {
		return r, m
	}

	return false, false
}

// matchBooleanCondition checks boolean comparison conditions.
func matchBooleanCondition(
	rule *ChoiceRule,
	varVal, input any,
	pathCache *jsonPathCache,
) (bool, bool) {
	if rule.BooleanEquals != nil {
		b, ok := varVal.(bool)

		return ok && b == *rule.BooleanEquals, true
	}

	if rule.BooleanEqualsPath != nil {
		ref, err := applyPath(*rule.BooleanEqualsPath, input, pathCache)
		if err != nil {
			return false, true
		}

		b1, ok1 := varVal.(bool)
		b2, ok2 := ref.(bool)

		return ok1 && ok2 && b1 == b2, true
	}

	return false, false
}

// matchTimestampCondition checks timestamp comparison conditions.
func matchTimestampCondition(
	rule *ChoiceRule,
	varVal, input any,
	pathCache *jsonPathCache,
) (bool, bool) {
	if result, matched := matchTimestampLiteralCondition(rule, varVal); matched {
		return result, matched
	}

	return matchTimestampPathCondition(rule, varVal, input, pathCache)
}

// matchTimestampLiteralCondition checks direct timestamp comparison conditions.
func matchTimestampLiteralCondition(rule *ChoiceRule, varVal any) (bool, bool) {
	compareLiteral := func(tsPtr *string, cmp func(t1, t2 time.Time) bool) (bool, bool) {
		if tsPtr == nil {
			return false, false
		}

		t1, err1 := parseTimestamp(varVal)
		t2, err2 := time.Parse(time.RFC3339, *tsPtr)

		return err1 == nil && err2 == nil && cmp(t1, t2), true
	}

	if r, m := compareLiteral(rule.TimestampEquals, time.Time.Equal); m {
		return r, m
	}

	if r, m := compareLiteral(rule.TimestampLessThan, time.Time.Before); m {
		return r, m
	}

	if r, m := compareLiteral(rule.TimestampGreaterThan, time.Time.After); m {
		return r, m
	}

	if r, m := compareLiteral(rule.TimestampLessThanEquals, func(t1, t2 time.Time) bool { return !t1.After(t2) }); m {
		return r, m
	}

	if r, m := compareLiteral(rule.TimestampGreaterThanEquals,
		func(t1, t2 time.Time) bool { return !t1.Before(t2) }); m {
		return r, m
	}

	return false, false
}

// matchTimestampPathCondition checks path-based timestamp comparison conditions.
func matchTimestampPathCondition(
	rule *ChoiceRule,
	varVal, input any,
	pathCache *jsonPathCache,
) (bool, bool) {
	comparePath := func(path *string, cmp func(t1, t2 time.Time) bool) (bool, bool) {
		if path == nil {
			return false, false
		}

		t1, t2, err := resolveTimestampPath(varVal, *path, input, pathCache)

		return err == nil && cmp(t1, t2), true
	}

	if r, m := comparePath(rule.TimestampEqualsPath, time.Time.Equal); m {
		return r, m
	}

	if r, m := comparePath(rule.TimestampLessThanPath, time.Time.Before); m {
		return r, m
	}

	if r, m := comparePath(rule.TimestampGreaterThanPath, time.Time.After); m {
		return r, m
	}

	if r, m := comparePath(rule.TimestampLessThanEqualsPath, func(t1, t2 time.Time) bool { return !t1.After(t2) }); m {
		return r, m
	}

	if r, m := comparePath(rule.TimestampGreaterThanEqualsPath,
		func(t1, t2 time.Time) bool { return !t1.Before(t2) }); m {
		return r, m
	}

	return false, false
}

// parseTimestamp converts any value to a [time.Time] by expecting a RFC3339 string.
func parseTimestamp(v any) (time.Time, error) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, ErrNotAString
	}

	return time.Parse(time.RFC3339, s)
}

// resolveTimestampPath resolves a path reference and returns both timestamps.
func resolveTimestampPath(
	varVal any,
	path string,
	input any,
	pathCache *jsonPathCache,
) (time.Time, time.Time, error) {
	t1, err := parseTimestamp(varVal)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	ref, err := applyPath(path, input, pathCache)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	t2, err := parseTimestamp(ref)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	return t1, t2, nil
}

// toFloat converts a numeric value to float64.
func toFloat(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}

	switch n := v.(type) {
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}

		return f, true
	default:
		return toFloatInteger(v)
	}
}

func toFloatInteger(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

// catchesError checks if a set of error equals values matches the given error.
func catchesError(errorEquals []string, err error) bool {
	if err == nil {
		return false
	}

	errCode := stepFunctionsErrorCode(err)
	for _, e := range errorEquals {
		switch e {
		case "States.ALL":
			return true
		case "States.Timeout":
			if errCode == errCodeStatesTimeout {
				return true
			}
		case errCodeStatesTaskFailed:
			if errCode != errCodeStatesTimeout &&
				errCode != errCodeStatesRuntime &&
				errCode != errCodeStatesPermissions {
				return true
			}
		case "States.Runtime":
			if errCode == errCodeStatesRuntime {
				return true
			}
		case "States.Permissions":
			if errCode == errCodeStatesPermissions {
				return true
			}
		case errCode, err.Error():
			return true
		}
	}

	return false
}

func stepFunctionsErrorCode(err error) string {
	if failErr, ok := errors.AsType[*FailError](err); ok {
		return failErr.ErrCode
	}

	if errors.Is(err, ErrStatesTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return errCodeStatesTimeout
	}

	if errors.Is(err, ErrUnsupportedStateType) ||
		errors.Is(err, ErrStateNotFound) ||
		errors.Is(err, ErrMaxTransitions) ||
		errors.Is(err, ErrChoiceNoMatch) ||
		errors.Is(err, ErrChoiceNoNext) ||
		errors.Is(err, ErrUnsupportedPathExpr) ||
		errors.Is(err, ErrUnsupportedResultPath) {
		return errCodeStatesRuntime
	}

	return err.Error()
}

// stepFunctionsErrorCause extracts the human-readable cause for an error, for
// population into a Catch's error output (the "Cause" field) and into
// TaskFailed history events. FailError carries an explicit Cause (e.g. from a
// Fail state or a wrapped service-integration failure); other errors have no
// separate code/cause split, so the full message is used as the cause.
func stepFunctionsErrorCause(err error) string {
	if failErr, ok := errors.AsType[*FailError](err); ok {
		return failErr.Cause
	}

	return err.Error()
}

// isActivityResource returns true if the resource ARN refers to an activity.
func isActivityResource(resource string) bool {
	return strings.Contains(resource, ":activity:")
}

// isLambdaResource returns true if the resource ARN is a Lambda function.
func isLambdaResource(resource string) bool {
	return strings.Contains(resource, ":lambda:") ||
		strings.HasPrefix(resource, "arn:aws:lambda:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:lambda:")
}

func isSQSResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::sqs:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:sqs:")
}

func isSNSResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::sns:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:sns:")
}

func isDynamoDBResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::dynamodb:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:dynamodb:")
}

func isECSResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::ecs:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:ecs:")
}

func isGlueResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::glue:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:glue:")
}

func isEventBridgeResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::events:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:events:")
}

func isAPIGatewayResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::apigateway:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:api-gateway:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:apigateway:")
}

func isEMRResource(resource string) bool {
	return strings.HasPrefix(resource, "arn:aws:states:::elasticmapreduce:") ||
		strings.HasPrefix(resource, "arn:aws:states:::emr-serverless:") ||
		strings.HasPrefix(resource, "arn:aws:states:::aws-sdk:emr:")
}

// resolveItems returns the array of items for a Map state.
func resolveItems(itemsPath string, input any) ([]any, error) {
	if itemsPath == "" || itemsPath == "$" {
		arr, ok := input.([]any)
		if !ok {
			return nil, ErrMapInputNotArray
		}

		return arr, nil
	}

	val, err := applyPath(itemsPath, input)
	if err != nil {
		return nil, err
	}

	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrItemsPathNotArray, itemsPath)
	}

	return arr, nil
}

// countMapResults counts succeeded and failed items from the error slice.
func countMapResults(errs []error) (int, int) {
	succeeded, failed := 0, 0
	for _, err := range errs {
		if err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	return succeeded, failed
}

// marshalInput marshals a value to JSON string for sub-execution input.
func marshalInput(v any) string {
	if v == nil {
		return "{}"
	}

	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}

	return string(b)
}

// applyParametersTemplate evaluates a Parameters or ResultSelector template
// (json.RawMessage) against the given input context.
// Keys ending in ".$" are evaluated as JSONPath or intrinsic function references.
func applyParametersTemplate(template json.RawMessage, input any) (any, error) {
	var tmpl any
	if err := json.Unmarshal(template, &tmpl); err != nil {
		return nil, fmt.Errorf("invalid template: %w", err)
	}

	return evalTemplate(tmpl, input)
}

// evalTemplate recursively evaluates a template structure against the input context.
//

// evalTemplate recursively evaluates a template against input.
func evalTemplate(tmpl, input any) (any, error) {
	switch v := tmpl.(type) {
	case map[string]any:
		return evalTemplateMap(v, input)
	case []any:
		return evalTemplateSlice(v, input)
	default:
		return tmpl, nil
	}
}

// evalTemplateMap evaluates a map template, resolving reference keys (ending in ".$").
func evalTemplateMap(v map[string]any, input any) (map[string]any, error) {
	result := make(map[string]any, len(v))

	for key, val := range v {
		if strings.HasSuffix(key, ".$") {
			evaluated, err := evalTemplateRefKey(key, val, input)
			if err != nil {
				return nil, err
			}

			result[key[:len(key)-2]] = evaluated
		} else {
			sub, err := evalTemplate(val, input)
			if err != nil {
				return nil, err
			}

			result[key] = sub
		}
	}

	return result, nil
}

// evalTemplateRefKey evaluates a reference key (ending in ".$").
func evalTemplateRefKey(key string, val, input any) (any, error) {
	strVal, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrReferenceKeyNotString, key)
	}

	evaluated, err := evaluateReference(strVal, input)
	if err != nil {
		return nil, fmt.Errorf("evaluating reference %q: %w", key, err)
	}

	return evaluated, nil
}

// evalTemplateSlice evaluates a slice template.
func evalTemplateSlice(v []any, input any) ([]any, error) {
	result := make([]any, len(v))

	for i, item := range v {
		sub, err := evalTemplate(item, input)
		if err != nil {
			return nil, err
		}

		result[i] = sub
	}

	return result, nil
}

// evaluateReference resolves a JSONPath expression or intrinsic function call.
func evaluateReference(ref string, input any) (any, error) {
	if strings.HasPrefix(ref, "States.") {
		return evaluateIntrinsicFunction(ref, input)
	}

	return applyPath(ref, input)
}
