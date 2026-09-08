package swf

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// ExecutionFilter holds optional filters for counting/listing executions.
type ExecutionFilter struct {
	OldestDate          *time.Time
	LatestDate          *time.Time
	CloseOldestDate     *time.Time
	CloseLatestDate     *time.Time
	WorkflowID          string
	WorkflowTypeName    string
	WorkflowTypeVersion string
	Tag                 string
	CloseStatus         string
	ReverseOrder        bool
}

func (f ExecutionFilter) matchOpen(e *WorkflowExecution) bool {
	if e.Status != statusRunning {
		return false
	}
	if f.WorkflowID != "" && e.WorkflowID != f.WorkflowID {
		return false
	}
	if f.WorkflowTypeName != "" && e.WorkflowTypeName != f.WorkflowTypeName {
		return false
	}
	if f.WorkflowTypeVersion != "" && e.WorkflowTypeVersion != f.WorkflowTypeVersion {
		return false
	}
	if f.Tag != "" && !slices.Contains(e.TagList, f.Tag) {
		return false
	}
	st := time.Unix(int64(e.StartTimestamp), 0)
	if f.OldestDate != nil && st.Before(*f.OldestDate) {
		return false
	}
	if f.LatestDate != nil && st.After(*f.LatestDate) {
		return false
	}

	return true
}

// matchClosed reports whether a non-RUNNING execution satisfies every filter
// dimension. Each dimension is evaluated by its own predicate; matchClosed
// short-circuits on the first predicate that fails, in the same order the
// checks used to appear inline.
func (f ExecutionFilter) matchClosed(e *WorkflowExecution) bool {
	if e.Status == statusRunning {
		return false
	}

	return f.matchClosedIdentity(e) &&
		f.matchClosedStatus(e) &&
		f.matchStartRange(e) &&
		f.matchCloseRange(e)
}

// matchClosedIdentity checks the workflow ID/type/tag filter dimensions.
func (f ExecutionFilter) matchClosedIdentity(e *WorkflowExecution) bool {
	if f.WorkflowID != "" && e.WorkflowID != f.WorkflowID {
		return false
	}
	if f.WorkflowTypeName != "" && e.WorkflowTypeName != f.WorkflowTypeName {
		return false
	}
	if f.WorkflowTypeVersion != "" && e.WorkflowTypeVersion != f.WorkflowTypeVersion {
		return false
	}
	if f.Tag != "" && !slices.Contains(e.TagList, f.Tag) {
		return false
	}

	return true
}

// matchClosedStatus checks the CloseStatus filter dimension.
func (f ExecutionFilter) matchClosedStatus(e *WorkflowExecution) bool {
	return f.CloseStatus == "" || e.CloseStatus == f.CloseStatus
}

// matchStartRange checks the OldestDate/LatestDate (start time) filter dimension.
func (f ExecutionFilter) matchStartRange(e *WorkflowExecution) bool {
	if f.OldestDate == nil && f.LatestDate == nil {
		return true
	}
	st := time.Unix(int64(e.StartTimestamp), 0)
	if f.OldestDate != nil && st.Before(*f.OldestDate) {
		return false
	}
	if f.LatestDate != nil && st.After(*f.LatestDate) {
		return false
	}

	return true
}

// matchCloseRange checks the CloseOldestDate/CloseLatestDate filter dimension.
func (f ExecutionFilter) matchCloseRange(e *WorkflowExecution) bool {
	if f.CloseOldestDate == nil && f.CloseLatestDate == nil {
		return true
	}
	if e.CloseTimestamp == 0 {
		return false
	}
	ct := time.Unix(int64(e.CloseTimestamp), 0)
	if f.CloseOldestDate != nil && ct.Before(*f.CloseOldestDate) {
		return false
	}
	if f.CloseLatestDate != nil && ct.After(*f.CloseLatestDate) {
		return false
	}

	return true
}

// CountOpenWorkflowExecutions counts RUNNING workflow executions in a domain, applying filters.
func (b *InMemoryBackend) CountOpenWorkflowExecutions(domain string, filter ExecutionFilter) int {
	b.mu.Lock("CountOpenWorkflowExecutions")
	defer b.mu.Unlock()

	b.sweepTimedOutExecutionsLocked(time.Now())

	count := 0
	for _, e := range b.executionsByDomain.Get(domain) {
		if filter.matchOpen(e) {
			count++
		}
	}

	return count
}

// CountClosedWorkflowExecutions counts non-RUNNING workflow executions in a domain, applying filters.
func (b *InMemoryBackend) CountClosedWorkflowExecutions(domain string, filter ExecutionFilter) int {
	b.mu.Lock("CountClosedWorkflowExecutions")
	defer b.mu.Unlock()

	b.sweepTimedOutExecutionsLocked(time.Now())

	count := 0
	for _, e := range b.executionsByDomain.Get(domain) {
		if filter.matchClosed(e) {
			count++
		}
	}

	return count
}

// validateStartWorkflowExecutionInput validates the parameters of a
// StartWorkflowExecution request that don't require backend state (no lock
// needed). Checks run in the exact order callers depend on for which error
// surfaces first.
func validateStartWorkflowExecutionInput(input StartWorkflowExecutionInput) error {
	if input.Domain == "" {
		return fmt.Errorf("%w: domain is required", ErrValidation)
	}
	if input.WorkflowID == "" {
		return fmt.Errorf("%w: workflowId is required", ErrValidation)
	}
	if err := validateChildPolicy(input.ChildPolicy); err != nil {
		return err
	}
	if err := validateDuration(input.ExecutionStartToCloseTimeout); err != nil {
		return err
	}
	if err := validateDuration(input.TaskStartToCloseTimeout); err != nil {
		return err
	}

	return nil
}

// startExecutionDefaults holds the effective (post-WorkflowType-defaulting)
// parameters used to construct a new WorkflowExecution.
type startExecutionDefaults struct {
	taskList    string
	childPolicy string
	execTimeout string
	taskTimeout string
	lambdaRole  string
}

// resolveExecutionDefaultsLocked applies WorkflowType defaults (when a
// workflow type is referenced and registered) on top of the caller-supplied
// input, then falls back to "TERMINATE" for an unset childPolicy. Caller must
// hold the write lock.
func (b *InMemoryBackend) resolveExecutionDefaultsLocked(
	input StartWorkflowExecutionInput,
) (startExecutionDefaults, error) {
	d := startExecutionDefaults{
		taskList:    input.TaskList,
		childPolicy: input.ChildPolicy,
		execTimeout: input.ExecutionStartToCloseTimeout,
		taskTimeout: input.TaskStartToCloseTimeout,
		lambdaRole:  input.LambdaRole,
	}

	if input.WorkflowTypeName != "" {
		wt, err := b.lookupRegisteredWorkflowType(input)
		if err != nil {
			return startExecutionDefaults{}, err
		}
		d = d.withWorkflowTypeDefaults(wt.Defaults)
	}

	if d.childPolicy == "" {
		d.childPolicy = childPolicyTerminate
	}

	return d, nil
}

// lookupRegisteredWorkflowType looks up the WorkflowType referenced by input
// and rejects unregistered or deprecated types. Caller must hold at least the
// read lock.
func (b *InMemoryBackend) lookupRegisteredWorkflowType(input StartWorkflowExecutionInput) (*WorkflowType, error) {
	key := input.Domain + ":" + input.WorkflowTypeName + ":" + input.WorkflowTypeVersion
	wt, ok := b.workflows.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: workflow type %s/%s not found",
			ErrNotFound, input.WorkflowTypeName, input.WorkflowTypeVersion)
	}
	if wt.Status == statusDeprecated {
		return nil, fmt.Errorf("%w: workflow type %s/%s is deprecated",
			ErrTypeDeprecated, input.WorkflowTypeName, input.WorkflowTypeVersion)
	}

	return wt, nil
}

// withWorkflowTypeDefaults returns a copy of d with any unset (empty-string)
// fields filled in from the WorkflowType's registered defaults.
func (d startExecutionDefaults) withWorkflowTypeDefaults(wtd WorkflowTypeDefaults) startExecutionDefaults {
	if d.taskList == "" {
		d.taskList = wtd.DefaultTaskList
	}
	if d.childPolicy == "" {
		d.childPolicy = wtd.DefaultChildPolicy
	}
	if d.execTimeout == "" {
		d.execTimeout = wtd.DefaultExecutionStartToCloseTimeout
	}
	if d.taskTimeout == "" {
		d.taskTimeout = wtd.DefaultTaskStartToCloseTimeout
	}
	if d.lambdaRole == "" {
		d.lambdaRole = wtd.DefaultLambdaRole
	}

	return d
}

// registerExecutionOrderLocked records key (one run's full
// domain+workflowID+runID key) in the LRU execution order and evicts the
// oldest run once the cache reaches maxWorkflowExecutions. Caller must hold
// the write lock.
func (b *InMemoryBackend) registerExecutionOrderLocked(key string) {
	b.executionOrder = append(b.executionOrder, key)
	if len(b.executionOrder) >= maxWorkflowExecutions {
		oldest := b.executionOrder[0]
		b.executionOrder = b.executionOrder[1:]
		b.evictExecutionLocked(oldest)
	}
}

// evictExecutionLocked removes an LRU-evicted run's execution row and
// history, plus any pending/active task rows still referencing it
// (gopherstack-jsi8: previously these were left behind as "ghost" rows -- a
// pending decisionQueues/activityQueues entry, or an active task token in
// activeDecisionTasks/activeActivityTasks, could still reference a run whose
// execution/history had already been evicted, so a later poll or respond
// call would silently operate against data that no longer existed). Caller
// must hold the write lock.
func (b *InMemoryBackend) evictExecutionLocked(key string) {
	exec, ok := b.executions.Get(key)
	b.executions.Delete(key)
	delete(b.history, key)

	if !ok {
		return
	}

	belongsToEvicted := func(workflowID, runID string) bool {
		return workflowID == exec.WorkflowID && runID == exec.RunID
	}

	for qkey, q := range b.decisionQueues {
		b.decisionQueues[qkey] = slices.DeleteFunc(q, func(t *DecisionTask) bool {
			return belongsToEvicted(t.WorkflowID, t.RunID)
		})
	}
	for qkey, q := range b.activityQueues {
		b.activityQueues[qkey] = slices.DeleteFunc(q, func(t *ActivityTask) bool {
			return belongsToEvicted(t.WorkflowID, t.RunID)
		})
	}
	for _, rec := range b.activeDecisionTasks.All() {
		if rec.Domain == exec.Domain && belongsToEvicted(rec.WorkflowID, rec.RunID) {
			b.activeDecisionTasks.Delete(rec.TaskToken)
		}
	}
	for _, rec := range b.activeActivityTasks.All() {
		if rec.Domain == exec.Domain && belongsToEvicted(rec.WorkflowID, rec.RunID) {
			b.activeActivityTasks.Delete(rec.TaskToken)
		}
	}
}

// childLink identifies the parent execution/decision when a new execution is
// being created as the child of a StartChildWorkflowExecution decision (see
// decision_orchestration.go). nil for a top-level (non-child) execution.
type childLink struct {
	parentWorkflowID       string
	parentRunID            string
	parentInitiatedEventID int64
}

// createExecutionLocked is the shared core behind StartWorkflowExecution,
// ContinueAsNewWorkflowExecution, and StartChildWorkflowExecution: it rejects
// a workflowId that already has an open run, stores the new WorkflowExecution,
// appends its WorkflowExecutionStarted history event, and enqueues its
// initial decision task. continuedFromRunID and parent are both optional and
// mutually exclusive in practice (continuation vs. child-start); pass "" /
// nil when neither applies (the plain StartWorkflowExecution case).
// Caller must hold the write lock.
func (b *InMemoryBackend) createExecutionLocked(
	input StartWorkflowExecutionInput,
	defaults startExecutionDefaults,
	continuedFromRunID string,
	parent *childLink,
) (*WorkflowExecution, error) {
	if _, open := b.openExecutionLocked(input.Domain, input.WorkflowID); open {
		return nil, fmt.Errorf("%w: %s", ErrWorkflowAlreadyStarted, input.WorkflowID)
	}

	runID := input.RunID
	if runID == "" {
		runID = uuid.New().String()
	}

	b.registerExecutionOrderLocked(executionKey(input.Domain, input.WorkflowID, runID))

	now := float64(time.Now().UnixMilli()) / milliDivisor
	exec := &WorkflowExecution{
		Domain:                       input.Domain,
		WorkflowID:                   input.WorkflowID,
		RunID:                        runID,
		Status:                       statusRunning,
		StartTimestamp:               now,
		WorkflowTypeName:             input.WorkflowTypeName,
		WorkflowTypeVersion:          input.WorkflowTypeVersion,
		TaskList:                     defaults.taskList,
		Input:                        input.Input,
		TagList:                      input.TagList,
		ChildPolicy:                  defaults.childPolicy,
		LambdaRole:                   defaults.lambdaRole,
		ExecutionStartToCloseTimeout: defaults.execTimeout,
		TaskStartToCloseTimeout:      defaults.taskTimeout,
		TaskPriority:                 input.TaskPriority,
	}
	if parent != nil {
		exec.ParentWorkflowID = parent.parentWorkflowID
		exec.ParentRunID = parent.parentRunID
		exec.ParentInitiatedEventID = parent.parentInitiatedEventID
	}
	b.executions.Put(exec)

	startedAttrs := map[string]any{
		attrInput:       input.Input,
		attrChildPolicy: defaults.childPolicy,
		attrTaskList:    map[string]any{attrName: defaults.taskList},
		attrWorkflowType: map[string]any{
			attrName:    input.WorkflowTypeName,
			attrVersion: input.WorkflowTypeVersion,
		},
		attrExecToCloseTO: defaults.execTimeout,
		attrTaskToCloseTO: defaults.taskTimeout,
		attrLambdaRole:    defaults.lambdaRole,
		attrTagList:       input.TagList,
	}
	if continuedFromRunID != "" {
		startedAttrs["continuedExecutionRunId"] = continuedFromRunID
	}
	if parent != nil {
		startedAttrs["parentInitiatedEventId"] = parent.parentInitiatedEventID
		startedAttrs["parentWorkflowExecution"] = map[string]any{
			attrWorkflowID: parent.parentWorkflowID,
			attrRunID:      parent.parentRunID,
		}
	}
	b.appendHistoryEventLocked(input.Domain, input.WorkflowID, runID, "WorkflowExecutionStarted", map[string]any{
		eventAttrKey("WorkflowExecutionStarted"): startedAttrs,
	})

	// Real AWS schedules the first decision task immediately after starting an
	// execution, so a decider can PollForDecisionTask and see the
	// WorkflowExecutionStarted event without any other event (signal, cancel
	// request, activity completion) first triggering one. Without this, a
	// freshly started workflow with no other stimulus never gets its first
	// decision task and stays OPEN forever.
	b.enqueueDecisionTaskLocked(input.Domain, input.WorkflowID, runID)

	cp := *exec

	return &cp, nil
}

// StartWorkflowExecution starts a new workflow execution.
// It validates that the referenced WorkflowType exists and is REGISTERED.
func (b *InMemoryBackend) StartWorkflowExecution(
	input StartWorkflowExecutionInput,
) (*WorkflowExecution, error) {
	if err := validateStartWorkflowExecutionInput(input); err != nil {
		return nil, err
	}

	b.mu.Lock("StartWorkflowExecution")
	defer b.mu.Unlock()

	// A prior run under this workflowId may have timed out but not yet been
	// swept; without this, its stale RUNNING status would falsely trip
	// createExecutionLocked's already-open guard below.
	b.sweepTimedOutExecutionsLocked(time.Now())

	if err := b.requireActiveDomainLocked(input.Domain); err != nil {
		return nil, err
	}

	defaults, err := b.resolveExecutionDefaultsLocked(input)
	if err != nil {
		return nil, err
	}

	return b.createExecutionLocked(input, defaults, "", nil)
}

// TerminateWorkflowExecution terminates a running workflow execution.
// runID is optional; if empty, targets the currently open run (real AWS's
// convention for this op's optional RunId). reason and details are
// stored in history. childPolicyOverride, if non-empty, is real SWF's
// per-call override of the child policy applied to this execution's open
// child executions -- it takes precedence over the policy stored on exec
// (set at StartWorkflowExecution time) for this call only, and never
// mutates that stored value. An empty override falls back to the stored
// policy, exactly as if the field had been omitted on the wire.
func (b *InMemoryBackend) TerminateWorkflowExecution(
	domain, workflowID, runID, reason, details, childPolicyOverride string,
) error {
	b.mu.Lock("TerminateWorkflowExecution")
	defer b.mu.Unlock()

	if err := validateChildPolicy(childPolicyOverride); err != nil {
		return err
	}

	b.sweepTimedOutExecutionsLocked(time.Now())

	exec, ok := b.resolveExecutionLocked(domain, workflowID, runID)
	if !ok {
		return fmt.Errorf("%w: execution %s/%s not found", ErrNotFound, domain, workflowID)
	}
	if exec.Status != statusRunning {
		return fmt.Errorf("%w: execution %s/%s is not open", ErrNotFound, domain, workflowID)
	}

	effectivePolicy := exec.ChildPolicy
	if childPolicyOverride != "" {
		effectivePolicy = childPolicyOverride
	}

	b.terminateExecutionLocked(domain, exec, reason, details, causeOperatorInitiated, effectivePolicy)

	return nil
}

// terminateExecutionLocked marks exec terminated, records its
// WorkflowExecutionTerminated history event, notifies exec's own parent (if
// still open) that this child has closed, and cascades policy onto exec's
// own open children per applyChildPolicyLocked. cause is "OPERATOR_INITIATED"
// for a direct TerminateWorkflowExecution call, or "CHILD_POLICY_APPLIED"
// when this termination is itself the result of an ancestor's TERMINATE
// child-policy cascade -- in which case reason/details are empty, matching
// real AWS (a cascade-triggered termination carries no operator-supplied
// reason). Caller must hold the write lock.
func (b *InMemoryBackend) terminateExecutionLocked(
	domain string, exec *WorkflowExecution, reason, details, cause, policy string,
) {
	exec.Status = statusTerminated
	exec.CloseStatus = statusTerminated
	exec.CloseTimestamp = float64(time.Now().UnixMilli()) / milliDivisor

	attrKey := eventAttrKey("WorkflowExecutionTerminated")
	attrs := map[string]any{
		attrKey: map[string]any{
			attrReason:      reason,
			attrDetails:     details,
			attrCause:       cause,
			attrChildPolicy: policy,
		},
	}
	b.appendHistoryEventLocked(domain, exec.WorkflowID, exec.RunID, "WorkflowExecutionTerminated", attrs)
	b.propagateChildClosureLocked(domain, exec, "ChildWorkflowExecutionTerminated", nil)
	b.applyChildPolicyLocked(domain, exec, policy)
}

// applyChildPolicyLocked cascades policy from a just-closed parent execution
// onto its still-open child executions (those with ParentWorkflowID/
// ParentRunID pointing at parent), matching real SWF's automatic
// child-closure handling:
//
//   - TERMINATE: each open child is itself terminated (cause
//     CHILD_POLICY_APPLIED), which in turn cascades that child's own stored
//     ChildPolicy onto its children, and so on down the tree.
//   - REQUEST_CANCEL: a WorkflowExecutionCancelRequested event (cause
//     CHILD_POLICY_APPLIED) is recorded on each open child and it is
//     enqueued a fresh decision task, exactly as RequestCancelWorkflowExecution
//     does for a direct call -- it is up to that child's own decider to act.
//   - ABANDON, or any other value: no action; children continue running
//     untouched.
//
// The child snapshot is taken before mutating anything so that terminating
// (and thereby further cascading into) one child cannot perturb iteration
// over its siblings. Caller must hold the write lock.
func (b *InMemoryBackend) applyChildPolicyLocked(domain string, parent *WorkflowExecution, policy string) {
	var children []*WorkflowExecution
	for _, e := range b.executionsByDomain.Get(domain) {
		if e.Status == statusRunning &&
			e.ParentWorkflowID == parent.WorkflowID &&
			e.ParentRunID == parent.RunID {
			children = append(children, e)
		}
	}

	for _, child := range children {
		switch policy {
		case childPolicyTerminate:
			b.terminateExecutionLocked(domain, child, "", "", causeChildPolicyApplied, child.ChildPolicy)
		case childPolicyRequestCancel:
			b.cascadeCancelRequestLocked(domain, child)
		}
	}
}

// cascadeCancelRequestLocked applies a REQUEST_CANCEL child-policy cascade
// to a single open child: sets CancelRequested, records a
// WorkflowExecutionCancelRequested event with cause CHILD_POLICY_APPLIED --
// the only cause value real SWF's WorkflowExecutionCancelRequestedCause enum
// defines, reserved for exactly this automatic-cascade case -- and enqueues
// the child a fresh decision task so its decider learns of the request.
// Caller must hold the write lock.
//
// RequestCancelWorkflowExecution (a direct, operator-initiated call) leaves
// Cause unset on this same event (fixed 2026-08-20, gopherstack wire-parity
// sweep): "OPERATOR_INITIATED" is not a value WorkflowExecutionCancelRequestedCause
// defines at all.
func (b *InMemoryBackend) cascadeCancelRequestLocked(domain string, exec *WorkflowExecution) {
	exec.CancelRequested = true

	attrKey := eventAttrKey("WorkflowExecutionCancelRequested")
	attrs := map[string]any{
		attrKey: map[string]any{
			attrCause: causeChildPolicyApplied,
		},
	}
	b.appendHistoryEventLocked(domain, exec.WorkflowID, exec.RunID, "WorkflowExecutionCancelRequested", attrs)

	if exec.TaskList != "" {
		qkey := domain + ":" + exec.TaskList
		b.decisionQueues[qkey] = append(b.decisionQueues[qkey], &DecisionTask{
			WorkflowID: exec.WorkflowID,
			RunID:      exec.RunID,
		})
	}
}

// DescribeWorkflowExecution returns a specific run of a workflow execution.
// runID is optional; if empty, targets the currently open run. Real AWS
// marks the wire equivalent (Execution.RunId) as required, but this backend
// stays lenient for callers (including its own internal callers, and
// existing tests) that omit it -- see resolveExecutionLocked.
func (b *InMemoryBackend) DescribeWorkflowExecution(
	domain, workflowID, runID string,
) (*WorkflowExecution, error) {
	b.mu.Lock("DescribeWorkflowExecution")
	defer b.mu.Unlock()

	b.sweepTimedOutExecutionsLocked(time.Now())

	exec, ok := b.resolveExecutionLocked(domain, workflowID, runID)
	if !ok {
		return nil, fmt.Errorf("%w: execution %s/%s not found", ErrNotFound, domain, workflowID)
	}
	cp := *exec

	return &cp, nil
}

// openCountsLocked returns open activity/decision/timer/child-workflow counts
// for one specific run. Caller must hold at least RLock.
func (b *InMemoryBackend) openCountsLocked(domain, workflowID, runID string) map[string]int {
	activityCount := 0
	for _, rec := range b.activeActivityTasks.All() {
		if rec.Domain == domain && rec.WorkflowID == workflowID && rec.RunID == runID {
			activityCount++
		}
	}
	decisionCount := 0
	for _, q := range b.decisionQueues {
		for _, t := range q {
			if t.WorkflowID == workflowID && t.RunID == runID {
				decisionCount++
			}
		}
	}

	timerCount := 0
	childCount := 0
	if exec, ok := b.executions.Get(executionKey(domain, workflowID, runID)); ok {
		timerCount = len(exec.OpenTimerIDs)
		for _, e := range b.executionsByDomain.Get(domain) {
			if e.Status == statusRunning && e.ParentWorkflowID == workflowID && e.ParentRunID == exec.RunID {
				childCount++
			}
		}
	}

	return map[string]int{
		"openActivityTasks":           activityCount,
		"openDecisionTasks":           decisionCount,
		"openTimers":                  timerCount,
		"openChildWorkflowExecutions": childCount,
	}
}

// ListOpenWorkflowExecutions returns all running executions in a domain matching the filter.
func (b *InMemoryBackend) ListOpenWorkflowExecutions(
	domain string,
	filter ExecutionFilter,
) []WorkflowExecution {
	b.mu.Lock("ListOpenWorkflowExecutions")
	defer b.mu.Unlock()

	b.sweepTimedOutExecutionsLocked(time.Now())

	byDomain := b.executionsByDomain.Get(domain)
	out := make([]WorkflowExecution, 0, len(byDomain))

	for _, e := range byDomain {
		if filter.matchOpen(e) {
			out = append(out, *e)
		}
	}
	sortExecutionsByTimestamp(out, false, filter.ReverseOrder)

	return out
}

// ListClosedWorkflowExecutions returns all closed executions in a domain matching the filter.
func (b *InMemoryBackend) ListClosedWorkflowExecutions(
	domain string,
	filter ExecutionFilter,
) []WorkflowExecution {
	b.mu.Lock("ListClosedWorkflowExecutions")
	defer b.mu.Unlock()

	b.sweepTimedOutExecutionsLocked(time.Now())

	byDomain := b.executionsByDomain.Get(domain)
	out := make([]WorkflowExecution, 0, len(byDomain))

	for _, e := range byDomain {
		if filter.matchClosed(e) {
			out = append(out, *e)
		}
	}
	// Real AWS orders by close time when closeTimeFilter was the caller's
	// selector, else by start time (ListClosedWorkflowExecutionsInput doc:
	// "the returned results are ordered by their close times"/"start times"
	// depending on which of the mutually-exclusive filters was given).
	sortExecutionsByTimestamp(out, filter.CloseOldestDate != nil, filter.ReverseOrder)

	return out
}

// sortExecutionsByTimestamp orders execs by StartTimestamp (or CloseTimestamp
// when byCloseTime is set), descending by default -- matching real AWS's
// documented default ("descending order of the start [or close] time") --
// or ascending when reverseOrder is set (ListOpen/ListClosedWorkflowExecutionsInput.ReverseOrder).
func sortExecutionsByTimestamp(execs []WorkflowExecution, byCloseTime, reverseOrder bool) {
	slices.SortFunc(execs, func(a, b WorkflowExecution) int {
		ak, bk := a.StartTimestamp, b.StartTimestamp
		if byCloseTime {
			ak, bk = a.CloseTimestamp, b.CloseTimestamp
		}

		c := 0
		switch {
		case ak < bk:
			c = -1
		case ak > bk:
			c = 1
		}

		if !reverseOrder {
			c = -c
		}

		return c
	})
}

// RequestCancelWorkflowExecution requests cancellation of a running execution.
// runID is optional; if empty, targets the currently open run.
func (b *InMemoryBackend) RequestCancelWorkflowExecution(domain, workflowID, runID string) error {
	b.mu.Lock("RequestCancelWorkflowExecution")
	defer b.mu.Unlock()

	b.sweepTimedOutExecutionsLocked(time.Now())

	exec, ok := b.resolveExecutionLocked(domain, workflowID, runID)
	if !ok {
		return fmt.Errorf("%w: execution %s/%s not found", ErrNotFound, domain, workflowID)
	}
	// Real AWS: "If the specified workflow execution isn't open, this method
	// fails with UnknownResource." (see RequestCancelWorkflowExecution doc) --
	// not ValidationException, which isn't even in this op's fault model.
	if exec.Status != statusRunning {
		return fmt.Errorf("%w: execution %s/%s is not open", ErrNotFound, domain, workflowID)
	}

	exec.CancelRequested = true

	// Real SWF's WorkflowExecutionCancelRequestedCause enum defines exactly one
	// value, CHILD_POLICY_APPLIED (types/enums.go), reserved for the automatic
	// child-policy cascade in cascadeCancelRequestLocked above -- a direct,
	// operator-initiated call like this one leaves Cause unset entirely, not
	// "OPERATOR_INITIATED" (not a value this enum defines at all).
	attrKey := eventAttrKey("WorkflowExecutionCancelRequested")
	attrs := map[string]any{
		attrKey: map[string]any{},
	}
	b.appendHistoryEventLocked(domain, workflowID, exec.RunID, "WorkflowExecutionCancelRequested", attrs)

	// Enqueue a decision task so the workflow decider can react.
	if exec.TaskList != "" {
		qkey := domain + ":" + exec.TaskList
		b.decisionQueues[qkey] = append(b.decisionQueues[qkey], &DecisionTask{
			WorkflowID: workflowID,
			RunID:      exec.RunID,
		})
	}

	return nil
}
