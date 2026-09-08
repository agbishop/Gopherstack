package codepipeline

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// findStage returns a pointer into p.Declaration.Stages for the stage named
// stageName, or nil if no such stage exists.
func findStage(p *Pipeline, stageName string) *Stage {
	for i := range p.Declaration.Stages {
		if p.Declaration.Stages[i].Name == stageName {
			return &p.Declaration.Stages[i]
		}
	}

	return nil
}

// findAction returns a pointer into stage.Actions for the action named
// actionName, or nil if no such action exists.
func findAction(stage *Stage, actionName string) *Action {
	for i := range stage.Actions {
		if stage.Actions[i].Name == actionName {
			return &stage.Actions[i]
		}
	}

	return nil
}

// runPipelineActions advances exec from wherever its existing action-execution
// records leave off, executing every action in declaration order,
// synchronously and instantaneously, with two exceptions: a stage whose
// inbound transition is disabled (DisableStageTransition, pipeline_state.go)
// is not entered, and a stage whose outbound transition is disabled blocks
// once its own actions have all succeeded, before the next stage starts --
// both mirroring the transient wait a real AWS client observes until
// EnableStageTransition resumes the run (see EnableStageTransition,
// pipeline_state.go). The other exception is the first unresolved
// Approval-category action, which gates the run the same way: it is recorded
// InProgress with a freshly generated approval token and processing stops
// there, resumed via PutApprovalResult (approvals.go). RetryStageExecution
// and RollbackStage (pipeline_state.go) also call this after resetting the
// action-execution records they mutate, so a resumed run picks up exactly
// where the reset left it.
//
// exec.Status is left at statusInProgress if a gate is hit, statusFailed if
// a previously-recorded Failed action is encountered (a rejected approval
// that was never retried -- the stage is broken and processing does not
// continue past it, matching real AWS's stage-scoped failure semantics), or
// statusSucceeded once every action in the pipeline has succeeded. Callers
// must hold b.mu.Lock.
func (b *InMemoryBackend) runPipelineActions(region string, p *Pipeline, exec *PipelineExecution) {
	actionExecs := b.actionExecutionsStore(region)
	byKey := indexActionExecutions(actionExecs[p.Declaration.Name], exec.PipelineExecutionID)

	for i, stage := range p.Declaration.Stages {
		if !stageStarted(byKey, stage) &&
			b.stageTransitionDisabled(region, p.Declaration.Name, stage.Name, transitionTypeInbound) {
			exec.Status = statusInProgress

			return
		}

		for _, action := range stage.Actions {
			resolved, done := resolvedActionStatus(byKey, stage.Name, action.Name)
			if done {
				switch resolved {
				case statusSucceeded:
					// Already recorded, move on to the next action.
					continue
				case statusInProgress:
					exec.Status = statusInProgress
				default:
					// statusFailed (rejected approval) or
					// statusActionAbandoned (stopped while pending): the
					// stage is broken and processing does not continue
					// past it without an explicit RetryStageExecution.
					exec.Status = statusFailed
				}

				return
			}

			ae := b.runOneAction(region, p.Declaration.Name, exec.PipelineExecutionID, stage.Name, action)

			switch ae.Status {
			case statusSucceeded:
				// Move on to the next action.
			case statusInProgress:
				exec.Status = statusInProgress

				return
			default:
				// statusFailed: a wired CodeBuild/Lambda action reported
				// failure. The stage is broken and processing does not
				// continue past it, matching real AWS's stage-scoped
				// failure semantics (see the doc comment above).
				exec.Status = statusFailed

				return
			}
		}

		// Only a non-final stage's outbound transition can meaningfully
		// gate anything -- there is no "next stage" for the last one to
		// block artifacts from reaching, so a disabled outbound transition
		// there has nothing left to prevent.
		if i < len(p.Declaration.Stages)-1 &&
			b.stageTransitionDisabled(region, p.Declaration.Name, stage.Name, transitionTypeOutbound) {
			exec.Status = statusInProgress

			return
		}
	}

	exec.Status = statusSucceeded
}

// stageStarted reports whether any action in stage already has a recorded
// action execution for this run, i.e. whether the run has already entered
// the stage. Used to scope the inbound-transition gate to stages not yet
// entered -- disabling a transition does not retroactively interrupt a stage
// already in progress, matching real AWS.
func stageStarted(byKey map[string]*ActionExecution, stage Stage) bool {
	for _, action := range stage.Actions {
		if _, ok := byKey[stage.Name+"/"+action.Name]; ok {
			return true
		}
	}

	return false
}

// indexActionExecutions builds a "stageName/actionName" lookup of the action
// executions already recorded for executionID, so runPipelineActions can
// resume from wherever a prior pass (or a retry/rollback reset) left off.
func indexActionExecutions(all []*ActionExecution, executionID string) map[string]*ActionExecution {
	byKey := make(map[string]*ActionExecution, len(all))

	for _, ae := range all {
		if ae.PipelineExecutionID == executionID {
			byKey[ae.StageName+"/"+ae.ActionName] = ae
		}
	}

	return byKey
}

// resolvedActionStatus reports the status of an already-recorded action
// execution for stageName/actionName, if one exists.
func resolvedActionStatus(byKey map[string]*ActionExecution, stageName, actionName string) (string, bool) {
	ae, ok := byKey[stageName+"/"+actionName]
	if !ok {
		return "", false
	}

	return ae.Status, true
}

// runOneAction records and executes a single action: Approval-category
// actions gate the run (InProgress + a fresh token); a built-in Build/
// CodeBuild or Invoke/Lambda action calls its wired backend and fails the
// action if that call does (runCodeBuildAction/runLambdaAction); every other
// action, and either of those two when unwired, completes immediately
// (Succeeded). Callers must hold b.mu.Lock.
func (b *InMemoryBackend) runOneAction(
	region, pipelineName, executionID, stageName string,
	action Action,
) *ActionExecution {
	now := time.Now().UTC()

	ae := &ActionExecution{
		PipelineExecutionID: executionID,
		ActionExecutionID:   uuid.NewString(),
		StageName:           stageName,
		ActionName:          action.Name,
		Status:              statusSucceeded,
		StartTime:           now,
		LastUpdateTime:      now,
	}

	switch {
	case action.ActionTypeID.Category == actionCategoryApproval:
		ae.Status = statusInProgress
		ae.Token = uuid.NewString()
	case isBuiltinAction(action, actionProviderCodeBuild) && b.codeBuildBackend != nil:
		ae.Status = b.runCodeBuildAction(action)
	case isBuiltinAction(action, actionProviderLambda) && b.lambdaBackend != nil:
		ae.Status = b.runLambdaAction(action)
	}

	store := b.actionExecutionsStore(region)
	store[pipelineName] = append(store[pipelineName], ae)

	return ae
}

// isBuiltinAction reports whether action is the AWS-owned built-in action
// type identified by provider (e.g. "CodeBuild", "Lambda"), as opposed to a
// custom or third-party action type that happens to share the same category.
func isBuiltinAction(action Action, provider string) bool {
	return action.ActionTypeID.Owner == actionOwnerAWS && action.ActionTypeID.Provider == provider
}

// runCodeBuildAction starts a build for a Build/CodeBuild action's
// configured ProjectName. A missing ProjectName is left to succeed
// (nothing to call); a project StartBuild can't find fails the action,
// matching real AWS's StartBuild ResourceNotFoundException. The emulator's
// CodeBuild backend always eventually completes an accepted build (see
// codebuild's janitor), so acceptance alone is this synchronous engine's
// success signal -- it does not wait for the build to finish.
func (b *InMemoryBackend) runCodeBuildAction(action Action) string {
	projectName := action.Configuration[configKeyProjectName]
	if projectName == "" {
		return statusSucceeded
	}

	if err := b.codeBuildBackend.StartBuild(projectName); err != nil {
		return statusFailed
	}

	return statusSucceeded
}

// runLambdaAction synchronously invokes an Invoke/Lambda action's configured
// FunctionName. A missing FunctionName is left to succeed (nothing to call);
// an invocation error (e.g. the function does not exist) fails the action.
// This does not model real AWS's asynchronous PutJobSuccessResult/
// PutJobFailureResult callback protocol for this action type -- see
// PARITY.md.
func (b *InMemoryBackend) runLambdaAction(action Action) string {
	functionName := action.Configuration[configKeyFunctionName]
	if functionName == "" {
		return statusSucceeded
	}

	_, _, err := b.lambdaBackend.InvokeFunction(
		context.Background(), functionName, "RequestResponse", []byte("{}"),
	)
	if err != nil {
		return statusFailed
	}

	return statusSucceeded
}
