package codepipeline

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// GetStageTransitionState returns the disabled state for a stage transition, or nil if enabled.
func (b *InMemoryBackend) GetStageTransitionState(
	ctx context.Context,
	pipelineName, stageName, transitionType string,
) *StageTransitionState {
	b.mu.RLock("GetStageTransitionState")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	key := regionKey(region, stageTransitionKey{
		PipelineName:   pipelineName,
		StageName:      stageName,
		TransitionType: transitionType,
	}.String())

	state, ok := b.stageTransitions.Get(key)
	if !ok {
		return nil
	}

	cp := *state

	return &cp
}

// DisableStageTransition disables a stage transition and records the reason.
// Returns StageNotFoundException if stageName does not exist in the pipeline.
func (b *InMemoryBackend) DisableStageTransition(
	ctx context.Context,
	pipelineName, stageName, transitionType, reason string,
) error {
	b.mu.Lock("DisableStageTransition")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelines.Get(regionKey(region, pipelineName))
	if !ok {
		return fmt.Errorf("%w: pipeline %q", ErrNotFound, pipelineName)
	}

	if !pipelineHasStage(p, stageName) {
		return fmt.Errorf("%w: stage %q not found in pipeline %q", ErrStageNotFound, stageName, pipelineName)
	}

	b.stageTransitions.Put(&StageTransitionState{
		region:         region,
		PipelineName:   pipelineName,
		StageName:      stageName,
		TransitionType: transitionType,
		Reason:         reason,
		Disabled:       true,
	})

	return nil
}

// EnableStageTransition re-enables a stage transition and, since real AWS
// resumes any artifacts that were waiting on it without requiring a further
// client call, re-drives runPipelineActions for every InProgress execution
// of the pipeline so a run parked at this gate (see runPipelineActions in
// action_engine.go) can proceed.
func (b *InMemoryBackend) EnableStageTransition(
	ctx context.Context,
	pipelineName, stageName, transitionType string,
) error {
	b.mu.Lock("EnableStageTransition")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelines.Get(regionKey(region, pipelineName))
	if !ok {
		return fmt.Errorf("%w: pipeline %q", ErrNotFound, pipelineName)
	}

	key := regionKey(region, stageTransitionKey{
		PipelineName: pipelineName, StageName: stageName, TransitionType: transitionType,
	}.String())
	b.stageTransitions.Delete(key)

	now := time.Now().UTC()

	for _, exec := range b.executionsStore(region)[pipelineName] {
		if exec.Status != statusInProgress {
			continue
		}

		b.runPipelineActions(region, p, exec)
		exec.LastUpdateTime = now
	}

	return nil
}

// stageTransitionDisabled reports whether stageName's inbound or outbound
// transition (per transitionType) is currently disabled. Callers must hold
// b.mu (RLock or Lock).
func (b *InMemoryBackend) stageTransitionDisabled(region, pipelineName, stageName, transitionType string) bool {
	key := regionKey(region, stageTransitionKey{
		PipelineName: pipelineName, StageName: stageName, TransitionType: transitionType,
	}.String())

	state, ok := b.stageTransitions.Get(key)

	return ok && state.Disabled
}

// pipelineHasStage returns true if the pipeline contains a stage with the given name.
func pipelineHasStage(p *Pipeline, stageName string) bool {
	for _, s := range p.Declaration.Stages {
		if s.Name == stageName {
			return true
		}
	}

	return false
}

// GetPipelineState returns the current state of each stage in a pipeline.
func (b *InMemoryBackend) GetPipelineState(ctx context.Context, pipelineName string) ([]StageState, error) {
	b.mu.RLock("GetPipelineState")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelines.Get(regionKey(region, pipelineName))
	if !ok {
		return nil, ErrNotFound
	}

	states := make([]StageState, len(p.Declaration.Stages))
	for i, stage := range p.Declaration.Stages {
		inKey := regionKey(region, stageTransitionKey{
			PipelineName: pipelineName, StageName: stage.Name, TransitionType: transitionTypeInbound,
		}.String())

		var inState *StageTransitionState
		if ts, found := b.stageTransitions.Get(inKey); found {
			tsCopy := *ts
			inState = &tsCopy
		}

		actionExecs := b.actionExecutionsStoreRO(region)[pipelineName]
		revisions := b.actionRevisionsStoreRO(region)
		actionStates := make([]map[string]any, len(stage.Actions))

		for j, action := range stage.Actions {
			actionStates[j] = buildActionState(actionExecs, revisions, pipelineName, stage.Name, action.Name)
		}

		states[i] = StageState{
			StageName:              stage.Name,
			InboundTransitionState: inState,
			ActionStates:           actionStates,
		}
	}

	return states, nil
}

// buildActionState builds the ActionState wire map for a single stage/action
// pair: the most recent action-execution record (if any), and the most
// recently tracked ActionRevision (if any, from PutActionRevision).
func buildActionState(
	actionExecs []*ActionExecution,
	revisions map[string]*ActionRevisionRecord,
	pipelineName, stageName, actionName string,
) map[string]any {
	state := map[string]any{"actionName": actionName}

	// Walk backwards to find the most recent execution for this stage/action pair.
	for _, ae := range slices.Backward(actionExecs) {
		if ae.StageName != stageName || ae.ActionName != actionName {
			continue
		}

		latest := map[string]any{
			"actionExecutionId": ae.ActionExecutionID,
			keyStatus:           ae.Status,
			"startTime":         float64(ae.StartTime.Unix()),
			"lastUpdateTime":    float64(ae.LastUpdateTime.Unix()),
			"lastStatusChange":  float64(ae.LastUpdateTime.Unix()),
		}

		if ae.Summary != "" {
			latest["summary"] = ae.Summary
		}

		if ae.Token != "" {
			latest["token"] = ae.Token
		}

		state["latestExecution"] = latest

		break
	}

	if rev, ok := revisions[actionRevisionKey(pipelineName, stageName, actionName)]; ok {
		state["currentRevision"] = map[string]any{
			"revisionId":       rev.RevisionID,
			"revisionChangeId": rev.RevisionChangeID,
			"created":          rev.Created,
		}
	}

	return state
}

// RetryStageExecution retries the failed actions of a stage within a
// specific pipeline execution. RetryMode FAILED_ACTIONS (the common case)
// resets only actions currently Failed/Abandoned in that stage; ALL_ACTIONS
// resets every action in the stage regardless of status. Either way, at
// least one action in the stage must currently be Failed/Abandoned for this
// execution or StageNotRetryableException is returned (matching real AWS,
// which only allows retrying a stage that actually failed). The reset
// actions are then re-run synchronously via runPipelineActions, resuming the
// SAME pipeline execution from that point -- unlike RollbackStage, retry
// never creates a new execution.
func (b *InMemoryBackend) RetryStageExecution(
	ctx context.Context,
	pipelineName, stageName, executionID, retryMode string,
) (*PipelineExecution, error) {
	b.mu.Lock("RetryStageExecution")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelines.Get(regionKey(region, pipelineName))
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q", ErrNotFound, pipelineName)
	}

	if findStage(p, stageName) == nil {
		return nil, fmt.Errorf("%w: stage %q not found in pipeline %q", ErrStageNotFound, stageName, pipelineName)
	}

	// gopherstack-wlab: same undeclared-code issue as OverrideStageCondition
	// below -- PipelineExecutionNotFoundException is not in
	// RetryStageExecution's declared error set per botocore
	// codepipeline/2015-07-09/service-2.json (PipelineNotFoundException/
	// StageNotFoundException/StageNotRetryableException/
	// NotLatestPipelineExecutionException/
	// ConcurrentPipelineExecutionsLimitExceededException/ConflictException/
	// ValidationException only). Left unfixed: StageNotRetryableException's
	// doc text ("stage state might have changed ... or the stage contains
	// no failed actions") describes a stage-condition mismatch on a real
	// execution, not a nonexistent executionID -- doesn't fit. NOTE: this
	// does not break errors.As for a real caller -- see
	// undeclared_error_codes_test.go.
	exec := findExecution(b.executionsStore(region)[pipelineName], executionID)
	if exec == nil {
		return nil, fmt.Errorf("%w: pipeline %q execution %q", ErrExecutionNotFound, pipelineName, executionID)
	}

	actionExecs := b.actionExecutionsStore(region)[pipelineName]
	if !resetStageActions(actionExecs, executionID, stageName, retryMode == stageRetryModeAllActions) {
		return nil, fmt.Errorf("%w: stage %q of execution %q has no failed action to retry",
			ErrStageNotRetryable, stageName, executionID)
	}

	exec.Status = statusInProgress
	b.runPipelineActions(region, p, exec)
	exec.LastUpdateTime = time.Now().UTC()

	cp := *exec

	return &cp, nil
}

// resetStageActions resets the action executions belonging to executionID's
// stageName back to Succeeded so runPipelineActions can re-run them. It
// first requires (and reports via its bool return) that at least one such
// action is currently Failed or Abandoned -- a stage with no failed action
// is not retryable in real AWS. When allActions is true (StageRetryMode
// ALL_ACTIONS) every action in the stage is reset, not just the failed ones.
func resetStageActions(actionExecs []*ActionExecution, executionID, stageName string, allActions bool) bool {
	failedFound := false

	for _, ae := range actionExecs {
		if ae.PipelineExecutionID != executionID || ae.StageName != stageName {
			continue
		}

		if ae.Status == statusFailed || ae.Status == statusActionAbandoned {
			failedFound = true
		}
	}

	if !failedFound {
		return false
	}

	now := time.Now().UTC()

	for _, ae := range actionExecs {
		if ae.PipelineExecutionID != executionID || ae.StageName != stageName {
			continue
		}

		if allActions || ae.Status == statusFailed || ae.Status == statusActionAbandoned {
			ae.Status = statusSucceeded
			ae.StartTime = now
			ae.LastUpdateTime = now
			ae.Token = ""
			ae.Summary = ""
		}
	}

	return true
}

// RollbackStage rolls back stageName to the state it was in after
// targetExecutionID last completed it successfully, creating (and
// persisting) a new ROLLBACK-type PipelineExecution rather than mutating the
// target execution. Real AWS requires the target execution to have actually
// succeeded through that stage; this backend enforces the same precondition
// via stageSucceededInExecution and returns UnableToRollbackStageException
// otherwise.
func (b *InMemoryBackend) RollbackStage(
	ctx context.Context,
	pipelineName, stageName, targetExecutionID string,
) (*PipelineExecution, error) {
	b.mu.Lock("RollbackStage")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelines.Get(regionKey(region, pipelineName))
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q", ErrNotFound, pipelineName)
	}

	stage := findStage(p, stageName)
	if stage == nil {
		return nil, fmt.Errorf("%w: stage %q not found in pipeline %q", ErrStageNotFound, stageName, pipelineName)
	}

	actionExecs := b.actionExecutionsStore(region)[pipelineName]
	if !stageSucceededInExecution(actionExecs, stage, targetExecutionID) {
		return nil, fmt.Errorf(
			"%w: pipeline %q execution %q did not complete stage %q successfully",
			ErrUnableToRollbackStage, pipelineName, targetExecutionID, stageName,
		)
	}

	now := time.Now().UTC()
	exec := &PipelineExecution{
		PipelineName:              pipelineName,
		PipelineExecutionID:       uuid.NewString(),
		Status:                    statusSucceeded,
		PipelineVersion:           p.Declaration.Version,
		ExecutionMode:             p.Declaration.ExecutionMode,
		ExecutionType:             executionTypeRollback,
		Trigger:                   triggerTypeManualRollback,
		RollbackTargetExecutionID: targetExecutionID,
		StartTime:                 now,
		LastUpdateTime:            now,
	}

	execs := b.executionsStore(region)
	execs[pipelineName] = append(execs[pipelineName], exec)

	actionExecStore := b.actionExecutionsStore(region)
	for _, action := range stage.Actions {
		actionExecStore[pipelineName] = append(actionExecStore[pipelineName], &ActionExecution{
			PipelineExecutionID: exec.PipelineExecutionID,
			ActionExecutionID:   uuid.NewString(),
			StageName:           stage.Name,
			ActionName:          action.Name,
			Status:              statusSucceeded,
			StartTime:           now,
			LastUpdateTime:      now,
		})
	}

	cp := *exec

	return &cp, nil
}

// stageSucceededInExecution reports whether every action in stage has a
// Succeeded action-execution record under executionID -- the real-AWS
// precondition for RollbackStage's target execution.
func stageSucceededInExecution(actionExecs []*ActionExecution, stage *Stage, executionID string) bool {
	if len(stage.Actions) == 0 {
		return false
	}

	succeeded := make(map[string]bool, len(stage.Actions))

	for _, ae := range actionExecs {
		if ae.PipelineExecutionID == executionID && ae.StageName == stage.Name && ae.Status == statusSucceeded {
			succeeded[ae.ActionName] = true
		}
	}

	for _, action := range stage.Actions {
		if !succeeded[action.Name] {
			return false
		}
	}

	return true
}

// OverrideStageCondition overrides a stage's BEFORE_ENTRY or ON_SUCCESS
// condition result (types.ConditionType), letting a pipeline execution
// proceed past a blocking condition. In real AWS this flips the relevant
// StageState.{BeforeEntryConditionState,OnSuccessConditionState}
// .LatestExecution.Status to ConditionExecutionStatusOverridden
// ("Overridden").
//
// This backend cannot perform that mutation because it has no condition-rule
// engine anywhere: StageDeclaration here has no BeforeEntry/OnFailure/
// OnSuccess members at all (the real SDK's StageDeclaration.BeforeEntry
// /OnFailure/OnSuccess -- BeforeEntryConditions/FailureConditions/
// SuccessConditions, each wrapping []Condition/[]RuleDeclaration -- is
// entirely unimplemented: CreatePipeline never parses it, and
// StageState here has no BeforeEntryConditionState/OnSuccessConditionState/
// OnFailureConditionState to flip in the first place). Building real
// condition-rule evaluation (parsing stage Conditions, gating stage entry
// on rule results, tracking per-execution ConditionState/RuleState) would be
// a new subsystem, not a field patch, and is out of scope here -- see
// PARITY.md for the explicit gap writeup. ListRuleExecutions (rules.go) is
// correspondingly always empty for the same underlying reason: there is
// never a rule execution to report because rules never run.
//
// Given that, this validates the pipeline, stage, and pipeline execution
// referenced by the request all exist (real backend logic, not a stub) and
// otherwise performs no additional mutation, which is the AWS-correct wire
// shape for this op regardless (OverrideStageConditionOutput carries no
// fields) -- but the *effect* AWS documents (unblocking a waiting stage) is
// an honest no-op here because there is nothing to unblock.
func (b *InMemoryBackend) OverrideStageCondition(
	ctx context.Context,
	pipelineName, stageName, executionID string,
) error {
	b.mu.RLock("OverrideStageCondition")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelines.Get(regionKey(region, pipelineName))
	if !ok {
		return fmt.Errorf("%w: pipeline %q", ErrNotFound, pipelineName)
	}

	if findStage(p, stageName) == nil {
		return fmt.Errorf("%w: stage %q not found in pipeline %q", ErrStageNotFound, stageName, pipelineName)
	}

	// gopherstack-wlab: PipelineExecutionNotFoundException is not in
	// OverrideStageCondition's declared error set per botocore
	// codepipeline/2015-07-09/service-2.json (PipelineNotFoundException/
	// StageNotFoundException/ConditionNotOverridableException/
	// NotLatestPipelineExecutionException/
	// ConcurrentPipelineExecutionsLimitExceededException/ConflictException/
	// ValidationException only; NOTE: this does not break errors.As for a
	// real caller -- see undeclared_error_codes_test.go). Left unfixed: no
	// single declared code is an obvious replacement for "this executionID
	// doesn't exist" -- NotLatestPipelineExecutionException is the closest
	// semantic fit, but its doc text ("the pipelineExecutionId ... is out
	// of date") describes a stale-but-once-valid ID, not a never-existed
	// one, and this backend cannot tell the two cases apart from AWS docs
	// alone.
	if findExecution(b.executionsStoreRO(region)[pipelineName], executionID) == nil {
		return fmt.Errorf("%w: pipeline %q execution %q", ErrExecutionNotFound, pipelineName, executionID)
	}

	return nil
}
