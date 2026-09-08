package stepfunctions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions/asl"
)

// PruneExecutions removes executions and history older than the retention period.
func (b *InMemoryBackend) PruneExecutions(_ context.Context) int {
	retention := b.settings.ExecutionRetention
	if retention == 0 {
		retention = defaultExecutionRetention
	}

	cutoff := float64(time.Now().Add(-retention).Unix())

	b.mu.Lock("PruneExecutions")
	defer b.mu.Unlock()

	return b.pruneExecutionsLocked(cutoff)
}

// pruneExecutionsLocked removes finished executions older than cutoff (Unix seconds).
// Must be called with b.mu held for writing.
func (b *InMemoryBackend) pruneExecutionsLocked(cutoff float64) int {
	type execPruneInfo struct {
		smARN  string
		status string
	}

	var toDelete []string

	pruneInfos := make(map[string]execPruneInfo)

	for _, exec := range b.executions.All() {
		if exec.Status != statusRunning && exec.StopDate != nil && *exec.StopDate < cutoff {
			toDelete = append(toDelete, exec.ExecutionArn)
			pruneInfos[exec.ExecutionArn] = execPruneInfo{smARN: exec.StateMachineArn, status: exec.Status}
		}
	}

	for _, arn := range toDelete {
		// Removes the execution from the table and, via the executionsByStateMachine
		// index, from the former smExecutions bookkeeping too -- the execution's
		// inline history goes with it since there is no longer a separate map.
		b.executions.Delete(arn)
		delete(b.executionDefinitions, arn)
		delete(b.historyTruncated, arn)

		if info, ok := pruneInfos[arn]; ok {
			b.removeFromStatusBucket(info.smARN, info.status, arn)
		}
	}

	b.sweepOrphanedTombstonesLocked()

	return len(toDelete)
}

// sweepOrphanedTombstonesLocked removes deletedExecs entries whose goroutines
// have already exited (no longer in cancelFns). Normally a goroutine removes
// its own tombstone in runParsedExecution, but an unusual exit path (panic/
// recover) could leave tombstones behind indefinitely without this sweep.
// Caller must hold b.mu for writing.
func (b *InMemoryBackend) sweepOrphanedTombstonesLocked() {
	for execARN := range b.deletedExecs {
		if _, running := b.cancelFns[execARN]; !running {
			delete(b.deletedExecs, execARN)
		}
	}
}

// StartSyncExecution executes an EXPRESS state machine synchronously and returns the result.
func (b *InMemoryBackend) StartSyncExecution(
	stateMachineArn, name, input string,
) (*SyncExecutionResult, error) {
	if len(input) > maxExecutionInputBytes {
		return nil, fmt.Errorf(
			"%w: input exceeds %d bytes",
			ErrInvalidExecutionInput,
			maxExecutionInputBytes,
		)
	}

	b.mu.RLock("StartSyncExecution")
	resolved, resolveErr := b.resolveExecutionTarget(stateMachineArn)
	if resolveErr != nil {
		b.mu.RUnlock()

		return nil, resolveErr
	}
	sm := resolved.SM

	if sm.Type != "EXPRESS" {
		b.mu.RUnlock()

		// AWS: StartSyncExecution is only supported for EXPRESS state
		// machines and returns StateMachineTypeNotSupported for STANDARD.
		return nil, fmt.Errorf(
			"%w: StartSyncExecution requires an EXPRESS state machine",
			ErrStateMachineTypeNotSupported,
		)
	}

	smName := sm.Name
	definition := sm.Definition
	integrations := b.snapshotIntegrationsLocked()
	b.mu.RUnlock()

	parsedSM, parseErr := asl.Parse(definition)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, parseErr)
	}

	if name == "" {
		name = fmt.Sprintf("sync-%d", time.Now().UnixNano())
	}

	// Execution/MapRun ARNs are always keyed off the base (unqualified) state
	// machine ARN -- AWS never carries a version/alias qualifier into an
	// execution ARN, even when StartSyncExecution was called with one.
	baseSMArn := sm.StateMachineArn

	const millisPerSecond = 1000.0
	startDate := float64(time.Now().UnixMilli()) / millisPerSecond
	execARN := b.execARN(baseSMArn, smName, name)

	// Express Workflows must complete within 5 minutes per AWS spec.
	const expressSyncTimeout = 5 * time.Minute

	syncCtx, syncCancel := context.WithTimeout(b.svcCtx, expressSyncTimeout)
	defer syncCancel()

	// Run synchronously with nil history recorder (sync executions are ephemeral).
	executor := asl.NewExecutor(parsedSM, integrations.lambdaInvoker, nil)
	applyIntegrations(executor, integrations)
	executor.SetActivityInvoker(b)
	executor.SetTaskTokenCallbackInvoker(b)
	executor.SetMapRunNotifier(
		&syncMapRunNotifier{backend: b, execARN: execARN, smARN: baseSMArn},
	)
	executor.SetExecutionContext(
		execARN,
		name,
		sm.RoleArn,
		time.Unix(int64(startDate), 0).UTC().Format(time.RFC3339),
		baseSMArn,
		sm.Name,
	)

	result, execErr := executor.Execute(syncCtx, execARN, input)

	return finalizeSyncExecutionResult(
		execARN,
		baseSMArn,
		name,
		input,
		startDate,
		result,
		execErr,
	), nil
}

// finalizeSyncExecutionResult assembles the SyncExecutionResult based on the
// outcome of the synchronous executor invocation. Extracted to keep
// StartSyncExecution under the funlen threshold.
func finalizeSyncExecutionResult(
	execARN, stateMachineArn, name, input string,
	startDate float64,
	result *asl.ExecutionResult,
	execErr error,
) *SyncExecutionResult {
	stopDate := float64(time.Now().Unix())

	syncResult := &SyncExecutionResult{
		StartDate:       startDate,
		StopDate:        stopDate,
		ExecutionArn:    execARN,
		StateMachineArn: stateMachineArn,
		Name:            name,
		Input:           input,
	}

	if execErr != nil {
		if errors.Is(execErr, context.DeadlineExceeded) {
			syncResult.Status = "TIMED_OUT"
			syncResult.Error = "States.Timeout"
			syncResult.Cause = "Express Workflow exceeded the 5-minute maximum execution time"
		} else {
			syncResult.Status = statusFailed
			syncResult.Error = execErr.Error()
		}

		return syncResult
	}

	if result.Failed {
		syncResult.Status = statusFailed
		syncResult.Error = result.Error
		syncResult.Cause = result.Cause

		return syncResult
	}

	outputBytes, _ := json.Marshal(result.Output)
	syncResult.Status = statusSucceeded
	syncResult.Output = string(outputBytes)

	return syncResult
}

func (b *InMemoryBackend) initializeExecutionRecord(
	smArn, name, execArn, input, def string, now float64, versionArn, aliasArn string,
) *Execution {
	exec := &Execution{
		StartDate:              now,
		ExecutionArn:           execArn,
		StateMachineArn:        smArn,
		StateMachineVersionArn: versionArn,
		StateMachineAliasArn:   aliasArn,
		Name:                   name,
		Status:                 statusRunning,
		Input:                  input,
		InputDetails:           &CloudWatchEventsExecutionDataDetails{Truncated: false},
		RedriveStatus:          redriveStatusNotRedrivable,
		RedriveStatusReason:    redriveStatusReasonRunning,
		history: []*HistoryEvent{
			{Timestamp: now, Type: "ExecutionStarted", ID: executionStartedEventID, PreviousEventID: 0},
		},
	}
	// Put also inserts exec into the executionsByStateMachine index, replacing
	// the former manual b.smExecutions[smArn] append.
	b.executions.Put(exec)
	b.executionDefinitions[execArn] = def
	b.addToStatusBucket(smArn, statusRunning, execArn)

	return exec
}

// startedExecution carries the state produced under lock by startExecutionLocked
// that the caller needs once the lock has been released (the pre-parsed state
// machine, the integration set to attach to the executor, and the context to
// run the ASL interpreter goroutine under).
type startedExecution struct {
	integrations    integrationsSnapshot
	ctx             context.Context
	activityInvoker asl.ActivityInvoker
	exec            *Execution
	parsedSM        *asl.StateMachine
	execArn         string
}

// startExecutionLocked validates the state machine, registers the new execution
// record and its cancel function, and returns everything needed to run the ASL
// interpreter asynchronously. Must be called with b.mu unlocked; it takes the
// write lock itself.
func (b *InMemoryBackend) startExecutionLocked(
	stateMachineArn, name, input string,
) (*startedExecution, error) {
	b.mu.Lock("StartExecution")
	defer b.mu.Unlock()

	// Opportunistically prune finished executions that have aged past the retention
	// period so the executions/history maps stay bounded when the janitor is off.
	if b.executions.Len() >= executionPruneSweepThreshold {
		retention := b.settings.ExecutionRetention
		if retention == 0 {
			retention = defaultExecutionRetention
		}

		b.pruneExecutionsLocked(float64(time.Now().Add(-retention).Unix()))
	}

	resolved, resolveErr := b.resolveExecutionTarget(stateMachineArn)
	if resolveErr != nil {
		return nil, resolveErr
	}
	sm := resolved.SM

	// AWS allows StartExecution (asynchronous execution) on EXPRESS state
	// machines too -- only StartSyncExecution is restricted to EXPRESS.
	// See "Asynchronous Express Workflows" in the AWS Step Functions docs.
	//
	// Execution ARNs are always keyed off the base (unqualified) state
	// machine ARN, even when stateMachineArn (the caller-supplied argument)
	// was a version or alias ARN -- see resolveExecutionTarget's doc comment.
	baseSMArn := sm.StateMachineArn
	execArn := b.execARN(baseSMArn, sm.Name, name)
	if sm.Type != "EXPRESS" && b.executions.Has(execArn) {
		return nil, fmt.Errorf("%w: %s", ErrExecutionAlreadyExists, name)
	}

	// Parse the definition before inserting any state, so a bad definition never
	// leaves an orphaned RUNNING execution in the store.
	definition := sm.Definition

	parsedSM, parseErr := asl.Parse(definition)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, parseErr)
	}

	const millisPerSecond = 1000.0
	now := float64(time.Now().UnixMilli()) / millisPerSecond
	exec := b.initializeExecutionRecord(
		baseSMArn, name, execArn, input, definition, now, resolved.VersionArn, resolved.AliasArn,
	)

	// Register the execution in the SM→executions index and store a cancel fn
	// so StopExecution and DeleteStateMachine can cancel the goroutine.
	// The context is derived from b.svcCtx so that all active executions are
	// also cancelled when the server shuts down.

	ctx, cancel := context.WithCancel(b.svcCtx)
	b.cancelFns[execArn] = cancel

	return &startedExecution{
		exec:            exec,
		execArn:         execArn,
		parsedSM:        parsedSM,
		integrations:    b.snapshotIntegrationsLocked(),
		ctx:             ctx,
		activityInvoker: b,
	}, nil
}

// StartExecution creates an execution and runs the ASL interpreter asynchronously.
func (b *InMemoryBackend) StartExecution(stateMachineArn, name, input string) (*Execution, error) {
	return b.StartExecutionWithTrace(stateMachineArn, name, input, "")
}

// StartExecutionWithTrace creates an execution with a trace header and runs the ASL interpreter.
func (b *InMemoryBackend) StartExecutionWithTrace(
	stateMachineArn, name, input, traceHeader string,
) (*Execution, error) {
	if len(input) > maxExecutionInputBytes {
		return nil, fmt.Errorf(
			"%w: input exceeds %d bytes",
			ErrInvalidExecutionInput,
			maxExecutionInputBytes,
		)
	}

	if name != "" {
		if err := validateName(name, maxExecutionNameLen); err != nil {
			return nil, err
		}
	}

	started, err := b.startExecutionLocked(stateMachineArn, name, input)
	if err != nil {
		return nil, err
	}

	if traceHeader != "" {
		started.exec.TraceHeader = traceHeader
	}

	// Run the ASL interpreter asynchronously.
	go b.runParsedExecution(
		started.ctx,
		started.execArn,
		started.parsedSM,
		input,
		started.integrations,
		started.activityInvoker,
	)

	return started.exec, nil
}

// applyExecutorContext populates the ASL executor's `$$` context object with
// data derived from the persisted execution and state machine records.
func (b *InMemoryBackend) applyExecutorContext(executor *asl.Executor, execARN string) {
	b.mu.RLock("applyExecutorContext")
	defer b.mu.RUnlock()

	exec, ok := b.executions.Get(execARN)
	if !ok {
		return
	}

	sm, smOK := b.stateMachines.Get(exec.StateMachineArn)
	if !smOK || sm == nil {
		executor.SetExecutionContext(
			exec.ExecutionArn,
			exec.Name,
			"",
			time.Unix(int64(exec.StartDate), 0).UTC().Format(time.RFC3339),
			exec.StateMachineArn,
			"",
		)

		return
	}

	executor.SetExecutionContext(
		exec.ExecutionArn,
		exec.Name,
		sm.RoleArn,
		time.Unix(int64(exec.StartDate), 0).UTC().Format(time.RFC3339),
		exec.StateMachineArn,
		sm.Name,
	)
}

// runParsedExecution runs the ASL interpreter for a pre-parsed state machine and updates the execution record.
func (b *InMemoryBackend) runParsedExecution(
	ctx context.Context,
	execARN string,
	sm *asl.StateMachine,
	input string,
	integrations integrationsSnapshot,
	activityInvoker asl.ActivityInvoker,
) {
	rec := &historyRecorder{backend: b}
	executor := asl.NewExecutor(sm, integrations.lambdaInvoker, rec)
	applyIntegrations(executor, integrations)
	executor.SetActivityInvoker(activityInvoker)
	executor.SetTaskTokenCallbackInvoker(b)
	executor.SetMapRunNotifier(b)
	b.applyExecutorContext(executor, execARN)
	result, execErr := executor.Execute(ctx, execARN, input)

	b.mu.Lock("runParsedExecution")
	defer b.mu.Unlock()

	// Clean up the cancel function now that the goroutine has exited.
	delete(b.cancelFns, execARN)

	// If the execution was tombstoned by DeleteStateMachine, discard and exit.
	if b.deletedExecs[execARN] {
		delete(b.deletedExecs, execARN)

		return
	}

	exec, ok := b.executions.Get(execARN)
	if !ok {
		return
	}

	// If the execution was already moved to a terminal state (e.g., ABORTED via
	// StopExecution) while the background runner was in flight, do not overwrite it.
	if exec.Status != statusRunning {
		return
	}

	b.finalizeExecutionRecordLocked(exec, execARN, result, execErr)
}

func (b *InMemoryBackend) finalizeExecutionRecordLocked(
	exec *Execution,
	execARN string,
	result *asl.ExecutionResult,
	execErr error,
) {
	now := float64(time.Now().Unix())
	exec.StopDate = &now
	nextID := int64(len(exec.history) + 1)

	if execErr != nil {
		exec.Status = statusFailed
		exec.Error = execErr.Error()
		exec.RedriveStatus = redriveStatusRedrivable
		exec.RedriveStatusReason = ""
		b.removeFromStatusBucket(exec.StateMachineArn, statusRunning, execARN)
		b.addToStatusBucket(exec.StateMachineArn, exec.Status, execARN)
		exec.history = append(exec.history, &HistoryEvent{
			Timestamp: now, Type: "ExecutionFailed", ID: nextID, PreviousEventID: nextID - 1,
		})

		return
	}

	if result.Failed {
		exec.Status = statusFailed
		exec.Error = result.Error
		exec.Cause = result.Cause
		exec.RedriveStatus = redriveStatusRedrivable
		exec.RedriveStatusReason = ""
		b.removeFromStatusBucket(exec.StateMachineArn, statusRunning, execARN)
		b.addToStatusBucket(exec.StateMachineArn, exec.Status, execARN)
		exec.history = append(exec.history, &HistoryEvent{
			Timestamp: now, Type: "ExecutionFailed", ID: nextID, PreviousEventID: nextID - 1,
		})

		return
	}

	outputBytes, _ := json.Marshal(result.Output)
	exec.Status = statusSucceeded
	exec.Output = string(outputBytes)
	exec.OutputDetails = &CloudWatchEventsExecutionDataDetails{Truncated: false}
	exec.RedriveStatus = redriveStatusNotRedrivable
	exec.RedriveStatusReason = redriveStatusReasonSucceeded
	b.removeFromStatusBucket(exec.StateMachineArn, statusRunning, execARN)
	b.addToStatusBucket(exec.StateMachineArn, exec.Status, execARN)
	exec.history = append(exec.history, &HistoryEvent{
		Timestamp: now, Type: "ExecutionSucceeded", ID: nextID, PreviousEventID: nextID - 1,
	})
}

// StopExecution marks a RUNNING execution as ABORTED.
// AWS behaviour: idempotent on already-terminal executions — returns success without mutation.
func (b *InMemoryBackend) StopExecution(executionArn, errCode, cause string) error {
	b.mu.Lock("StopExecution")
	defer b.mu.Unlock()

	exec, exists := b.executions.Get(executionArn)
	if !exists {
		return fmt.Errorf("%w: %s", ErrExecutionDoesNotExist, executionArn)
	}

	// Already in a terminal state — no-op per AWS semantics.
	if exec.Status != statusRunning {
		return nil
	}

	now := float64(time.Now().Unix())
	exec.Status = statusAborted
	exec.StopDate = &now
	exec.Error = errCode
	exec.Cause = cause
	exec.RedriveStatus = redriveStatusRedrivable
	exec.RedriveStatusReason = ""
	b.removeFromStatusBucket(exec.StateMachineArn, statusRunning, executionArn)
	b.addToStatusBucket(exec.StateMachineArn, statusAborted, executionArn)

	// Cancel the running goroutine for this execution.
	if cancelFn, ok := b.cancelFns[executionArn]; ok {
		cancelFn()
		delete(b.cancelFns, executionArn)
	}

	nextID := int64(len(exec.history) + 1)
	exec.history = append(exec.history, &HistoryEvent{
		Timestamp: now, Type: "ExecutionAborted", ID: nextID, PreviousEventID: nextID - 1,
	})

	return nil
}

// DescribeExecution returns details for a single execution.
func (b *InMemoryBackend) DescribeExecution(executionArn string) (*Execution, error) {
	b.mu.RLock("DescribeExecution")
	defer b.mu.RUnlock()

	exec, exists := b.executions.Get(executionArn)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrExecutionDoesNotExist, executionArn)
	}

	// exec.history is written by appendHistory under only b.mu.RLock +
	// b.historyMu.Lock (a deliberate hot-path optimization, see appendHistory),
	// so a whole-struct copy here -- which touches every field, including
	// history -- must also take historyMu to avoid racing that write; b.mu's
	// RLock alone does not exclude it. Lock order: b.mu before historyMu.
	b.historyMu.RLock()
	cp := *exec
	b.historyMu.RUnlock()

	return &cp, nil
}

// ListExecutions returns executions for a state machine with optional pagination.
// AWS: ListExecutions models StateMachineDoesNotExist for an unknown
// stateMachineArn.
func (b *InMemoryBackend) ListExecutions(
	stateMachineArn, statusFilter, nextToken string, maxResults int,
) ([]Execution, string, error) {
	b.mu.RLock("ListExecutions")
	defer b.mu.RUnlock()

	if !b.stateMachines.Has(stateMachineArn) {
		return nil, "", fmt.Errorf("%w: %s", ErrStateMachineDoesNotExist, stateMachineArn)
	}

	// When a status filter is given, use the O(1) status bucket index instead
	// of scanning the full executionsByStateMachine index.
	var execs []*Execution

	if statusFilter != "" {
		if bucket := b.smExecsByStatus[stateMachineArn]; bucket != nil {
			for _, execARN := range bucket[statusFilter] {
				if exec, ok := b.executions.Get(execARN); ok {
					execs = append(execs, exec)
				}
				// Defensive guard: indexes should be consistent with
				// b.executions, but skip any stale references just in case.
			}
		}
	} else {
		execs = b.executionsByStateMachine.Get(stateMachineArn)
	}

	// See the comment in DescribeExecution: whole-struct copies of *Execution
	// touch history, which appendHistory writes under historyMu rather than
	// b.mu's write lock, so copying it here needs the same guard.
	all := make([]Execution, 0, len(execs))
	b.historyMu.RLock()
	for _, exec := range execs {
		all = append(all, *exec)
	}
	b.historyMu.RUnlock()

	sort.Slice(all, func(i, j int) bool { return all[i].StartDate > all[j].StartDate })

	page, token := paginate(all, nextToken, maxResults)

	return page, token, nil
}

func (b *InMemoryBackend) resetExecutionForRedrive(exec *Execution, executionARN, smARN string, now float64) {
	oldStatus := exec.Status
	exec.Status = statusRunning
	exec.Output = ""
	exec.Error = ""
	exec.Cause = ""
	exec.StopDate = nil
	exec.StartDate = now
	exec.RedriveCount++
	exec.RedriveDate = &now
	exec.RedriveStatus = redriveStatusNotRedrivable
	exec.RedriveStatusReason = redriveStatusReasonRunning
	exec.OutputDetails = nil
	b.removeFromStatusBucket(smARN, oldStatus, executionARN)
	b.addToStatusBucket(smARN, statusRunning, executionARN)
	exec.history = []*HistoryEvent{
		{Timestamp: now, Type: "ExecutionStarted", ID: executionStartedEventID, PreviousEventID: 0},
	}
}

// redrivenExecution carries the state produced under lock by redriveExecutionLocked
// that the caller needs once the lock has been released, mirroring startedExecution.
type redrivenExecution struct {
	integrations    integrationsSnapshot
	ctx             context.Context
	activityInvoker asl.ActivityInvoker
	parsedSM        *asl.StateMachine
	originalInput   string
}

// redriveExecutionLocked validates the execution is redrivable, resets it to
// RUNNING, and returns everything needed to run the ASL interpreter
// asynchronously. Must be called with b.mu unlocked; it takes the write lock
// itself.
func (b *InMemoryBackend) redriveExecutionLocked(executionARN string) (*redrivenExecution, error) {
	b.mu.Lock("RedriveExecution")
	defer b.mu.Unlock()

	exec, exists := b.executions.Get(executionARN)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrExecutionDoesNotExist, executionARN)
	}

	if exec.Status != statusFailed && exec.Status != statusAborted {
		return nil, fmt.Errorf(
			"%w: execution %s is in status %s; only FAILED or ABORTED executions can be redriven",
			ErrExecutionNotRedrivable,
			executionARN,
			exec.Status,
		)
	}

	smARN := exec.StateMachineArn
	originalInput := exec.Input

	sm, smExists := b.stateMachines.Get(smARN)
	if !smExists {
		return nil, fmt.Errorf(
			"%w: state machine %s no longer exists",
			ErrStateMachineDoesNotExist,
			smARN,
		)
	}

	definition := sm.Definition

	parsedSM, parseErr := asl.Parse(definition)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, parseErr)
	}

	// Reset the execution to RUNNING.
	now := float64(time.Now().Unix())
	b.resetExecutionForRedrive(exec, executionARN, smARN, now)

	// Snapshot the (possibly-updated) definition.
	b.executionDefinitions[executionARN] = definition

	ctx, cancel := context.WithCancel(b.svcCtx)
	b.cancelFns[executionARN] = cancel

	// No manual "ensure execution is tracked under the SM" step is needed here
	// (unlike the former smExecutions []string index): exec was already
	// inserted into the executions table -- and, via the immutable
	// StateMachineArn field, into the executionsByStateMachine index -- when
	// the execution was first created, and neither Redrive nor any other path
	// ever removes it from the table without also deleting it outright.
	return &redrivenExecution{
		originalInput:   originalInput,
		parsedSM:        parsedSM,
		integrations:    b.snapshotIntegrationsLocked(),
		ctx:             ctx,
		activityInvoker: b,
	}, nil
}

// describeExecutionAfterRedrive returns a copy of the execution record for
// executionARN after RedriveExecution has reset it to RUNNING. Extracted from
// RedriveExecution to keep the locked region out of the parent's funlen count.
func (b *InMemoryBackend) describeExecutionAfterRedrive(executionARN string) Execution {
	b.mu.RLock("RedriveExecution.result")
	defer b.mu.RUnlock()

	execAfter, _ := b.executions.Get(executionARN)

	// See the comment in DescribeExecution: guard the whole-struct copy
	// against a concurrent appendHistory write, which b.mu's RLock alone
	// does not exclude.
	b.historyMu.RLock()
	defer b.historyMu.RUnlock()

	return *execAfter
}

// RedriveExecution re-runs a FAILED or ABORTED execution starting from its last known state.
// AWS Step Functions re-runs from the last state that was reached before failure.
// In this implementation we restart the entire execution with the original input (AWS parity for STANDARD executions).
func (b *InMemoryBackend) RedriveExecution(executionARN string) (*Execution, error) {
	redrive, err := b.redriveExecutionLocked(executionARN)
	if err != nil {
		return nil, err
	}

	go b.runParsedExecution(
		redrive.ctx,
		executionARN,
		redrive.parsedSM,
		redrive.originalInput,
		redrive.integrations,
		redrive.activityInvoker,
	)

	cp := b.describeExecutionAfterRedrive(executionARN)

	return &cp, nil
}

// DescribeStateMachineForExecution returns the state machine definition that was active
// when the given execution was started.
func (b *InMemoryBackend) DescribeStateMachineForExecution(
	executionARN string,
) (*StateMachine, error) {
	b.mu.RLock("DescribeStateMachineForExecution")
	defer b.mu.RUnlock()

	exec, exists := b.executions.Get(executionARN)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrExecutionDoesNotExist, executionARN)
	}

	definition, hasSnapshot := b.executionDefinitions[executionARN]
	if !hasSnapshot {
		// Fall back to the current definition if no snapshot was taken (pre-snapshot executions).
		sm, smExists := b.stateMachines.Get(exec.StateMachineArn)
		if !smExists {
			// BUG: DescribeStateMachineForExecution's deserializer declares no
			// StateMachineDoesNotExist (only ExecutionDoesNotExist/InvalidArn/Kms*/
			// UnknownError) -- this leaks an undeclared code. !hasSnapshot fires on any
			// execution started before the last persistence restore (executionDefinitions
			// is deliberately not persisted, see persistence.go's Phase 3.3 comment), so
			// this is reachable, not just theoretical. No declared code fits "execution
			// exists, SM gone." The smExists==false branch below (hasSnapshot==true)
			// answers the identical real condition with a synthetic 200 -- candidate fix:
			// return &StateMachine{StateMachineArn: exec.StateMachineArn} here too instead
			// of erroring. Left unfixed pending evidence for that remedy (gopherstack-2hdk).
			return nil, fmt.Errorf(
				"%w: state machine %s no longer exists",
				ErrStateMachineDoesNotExist,
				exec.StateMachineArn,
			)
		}

		cp := *sm

		return &cp, nil
	}

	sm, smExists := b.stateMachines.Get(exec.StateMachineArn)
	if !smExists {
		// SM was deleted but execution still exists — return a synthetic SM with the snapshot.
		return &StateMachine{
			StateMachineArn: exec.StateMachineArn,
			Definition:      definition,
		}, nil
	}

	cp := *sm
	cp.Definition = definition

	return &cp, nil
}

// removeFromStatusBucket removes execARN from the smExecsByStatus bucket for smARN/status.
// Must be called with b.mu write lock held.
func (b *InMemoryBackend) removeFromStatusBucket(smARN, status, execARN string) {
	bucket := b.smExecsByStatus[smARN]
	if bucket == nil {
		return
	}
	arns := bucket[status]
	for i, a := range arns {
		if a == execARN {
			bucket[status] = append(arns[:i], arns[i+1:]...)

			return
		}
	}
}

// addToStatusBucket adds execARN to the smExecsByStatus bucket for smARN/status.
// Must be called with b.mu write lock held.
func (b *InMemoryBackend) addToStatusBucket(smARN, status, execARN string) {
	if b.smExecsByStatus[smARN] == nil {
		b.smExecsByStatus[smARN] = make(map[string][]string)
	}
	b.smExecsByStatus[smARN][status] = append(b.smExecsByStatus[smARN][status], execARN)
}
