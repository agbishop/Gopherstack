// Package codepipeline provides an in-memory implementation of the AWS CodePipeline service.
package codepipeline

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
// CodePipeline resources are isolated per region: every backend operation resolves
// the caller's region from the request context and operates only on that region's
// nested store. Pipelines, action types, jobs, webhooks, executions, and stage
// transitions are all region-scoped in AWS, so cross-region references never occur
// and isolation is always safe.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

const (
	// statusInProgress is the status for an in-progress job or execution.
	statusInProgress = "InProgress"

	// PipelineTypeV1 and PipelineTypeV2 are the valid PipelineType values.
	PipelineTypeV1 = "V1"
	PipelineTypeV2 = "V2"

	// ExecutionModeQueued is the QUEUED execution mode.
	ExecutionModeQueued = "QUEUED"
	// ExecutionModeSuperseded is the SUPERSEDED execution mode.
	ExecutionModeSuperseded = "SUPERSEDED"
	// ExecutionModeParallel is the PARALLEL execution mode.
	ExecutionModeParallel = "PARALLEL"

	// WebhookAuthGitHubHMAC is the GITHUB_HMAC authentication type for webhooks.
	WebhookAuthGitHubHMAC = "GITHUB_HMAC"
	// WebhookAuthIP is the IP authentication type for webhooks.
	WebhookAuthIP = "IP"
	// WebhookAuthUnauthenticated is the UNAUTHENTICATED authentication type for webhooks.
	WebhookAuthUnauthenticated = "UNAUTHENTICATED"

	// kindPipeline is the resource kind string for pipelines.
	kindPipeline = "pipeline"
	// kindWebhook is the resource kind string for webhooks.
	kindWebhook = "webhook"
	// kindActionType is the resource kind string for custom action types.
	kindActionType = "actiontype"

	// keyPipelineExecutionID and keyStatus are JSON keys shared across the
	// execution-detail response maps.
	keyPipelineExecutionID = "pipelineExecutionId"
	keyStatus              = "status"

	// statusSucceeded is the terminal success status for executions and actions.
	statusSucceeded = "Succeeded"

	// statusStopped is the terminal status for a manually stopped pipeline execution.
	statusStopped = "Stopped"

	// statusFailed is the terminal status for a failed pipeline execution or
	// action execution (reached only via a rejected manual approval in this
	// backend, since every action otherwise completes synchronously).
	statusFailed = "Failed"

	// statusActionAbandoned is the terminal status for an action execution
	// abandoned by StopPipelineExecution while awaiting manual approval.
	statusActionAbandoned = "Abandoned"

	// ruleOwnerAWS is the owner value for AWS-managed CodePipeline rule types.
	ruleOwnerAWS = "AWS"

	// actionCategoryApproval is the ActionTypeID.Category value for manual
	// approval actions; it is the only category this backend treats
	// specially (as a gate on StartPipelineExecution/PutApprovalResult),
	// matching real AWS's built-in Approval/AWS/Manual/1 action type.
	actionCategoryApproval = "Approval"

	// actionOwnerAWS is the ActionTypeID.Owner value for AWS-managed built-in
	// action types, as opposed to "ThirdParty" or "Custom".
	actionOwnerAWS = "AWS"

	// actionProviderCodeBuild and actionProviderLambda are the
	// ActionTypeID.Provider values for the built-in Build/CodeBuild and
	// Invoke/Lambda action types, wired via SetCodeBuildBackend/
	// SetLambdaBackend when a real backend is available (action_engine.go).
	actionProviderCodeBuild = "CodeBuild"
	actionProviderLambda    = "Lambda"

	// configKeyProjectName and configKeyFunctionName are the
	// Action.Configuration keys real AWS documents for the built-in
	// Build/CodeBuild and Invoke/Lambda action types (not part of the SDK's
	// Action/ActionTypeId shape, which leaves Configuration an opaque
	// map[string]string).
	configKeyProjectName  = "ProjectName"
	configKeyFunctionName = "FunctionName"

	// approvalStatusApproved and approvalStatusRejected are the valid
	// ApprovalResult.Status values for PutApprovalResult.
	approvalStatusApproved = "Approved"
	approvalStatusRejected = "Rejected"

	// stageRetryModeAllActions is the StageRetryMode value that resets every
	// action in the stage, not just the failed ones (FAILED_ACTIONS, the
	// default/only other mode, is handled without a named constant since it
	// is simply "not ALL_ACTIONS").
	stageRetryModeAllActions = "ALL_ACTIONS"

	// executionTypeStandard and executionTypeRollback are the valid
	// PipelineExecution ExecutionType values.
	executionTypeStandard = "STANDARD"
	executionTypeRollback = "ROLLBACK"

	// triggerTypeManualRollback and triggerTypePutActionRevision are
	// ExecutionTrigger.TriggerType values this backend can produce, beyond
	// triggerTypeStartExecution (handler_pipeline_executions.go).
	triggerTypeManualRollback    = "ManualRollback"
	triggerTypePutActionRevision = "PutActionRevision"
)

// InMemoryBackend is a thread-safe in-memory store for CodePipeline resources.
//
// pipelines, customActionTypes, jobs, webhooks, and stageTransitions are flat
// store.Table collections keyed by a composite "region|id" string (see
// regionKey below), replacing the old map[string]map[K]*V nesting (outer key
// = region) that isolated same-named resources across regions. Each table's
// companion *store.Index values replace the old per-region
// iteration/reverse-ARN-map lookups -- see store_setup.go. executions and
// actionExecutions remain plain region-nested maps: their values are bare
// []*T slices with no identity of their own, so they are not candidates for
// store.Table (see pkgs/store's package doc). Callers must hold b.mu while
// accessing any of these collections.
type InMemoryBackend struct {
	pipelines                  *store.Table[Pipeline]
	pipelinesByRegion          *store.Index[Pipeline]
	pipelinesByARN             *store.Index[Pipeline]
	customActionTypes          *store.Table[CustomActionType]
	customActionTypesByRegion  *store.Index[CustomActionType]
	customActionTypesByARN     *store.Index[CustomActionType]
	jobs                       *store.Table[Job]
	jobsByRegion               *store.Index[Job]
	webhooks                   *store.Table[Webhook]
	webhooksByRegion           *store.Index[Webhook]
	webhooksByARN              *store.Index[Webhook]
	stageTransitions           *store.Table[StageTransitionState]
	stageTransitionsByPipeline *store.Index[StageTransitionState]
	registry                   *store.Registry
	executions                 map[string]map[string][]*PipelineExecution  // region → pipelineName → executions
	actionExecutions           map[string]map[string][]*ActionExecution    // region → pipelineName → action executions
	actionRevisions            map[string]map[string]*ActionRevisionRecord // region → "pipeline/stage/action" → revision
	// codeBuildBackend and lambdaBackend back a Build/CodeBuild and an
	// Invoke/Lambda action respectively (see runOneAction, action_engine.go).
	// Nil until wired via SetCodeBuildBackend/SetLambdaBackend, in which case
	// those actions complete instantly with no cross-service call, matching
	// this backend's original behavior.
	codeBuildBackend CodeBuildStarter
	lambdaBackend    LambdaInvoker
	mu               *lockmetrics.RWMutex
	accountID        string
	region           string
}

// NewInMemoryBackend creates a new backend for the given account and region.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:         store.NewRegistry(),
		executions:       make(map[string]map[string][]*PipelineExecution),
		actionExecutions: make(map[string]map[string][]*ActionExecution),
		actionRevisions:  make(map[string]map[string]*ActionRevisionRecord),
		accountID:        accountID,
		region:           region,
		mu:               lockmetrics.New("codepipeline-" + region),
	}

	registerAllTables(b)

	return b
}

// SetCodeBuildBackend wires CodeBuild so a Build/CodeBuild action's
// ProjectName actually starts a build, instead of every action completing
// unconditionally with no cross-service call (gopherstack-cb9l).
func (b *InMemoryBackend) SetCodeBuildBackend(codeBuild CodeBuildStarter) {
	b.mu.Lock("SetCodeBuildBackend")
	defer b.mu.Unlock()

	b.codeBuildBackend = codeBuild
}

// SetLambdaBackend wires Lambda so an Invoke/Lambda action's FunctionName is
// actually invoked, instead of every action completing unconditionally with
// no cross-service call (gopherstack-cb9l).
func (b *InMemoryBackend) SetLambdaBackend(lambda LambdaInvoker) {
	b.mu.Lock("SetLambdaBackend")
	defer b.mu.Unlock()

	b.lambdaBackend = lambda
}

// regionKey builds the composite store.Table primary key ("region|id") shared
// by every resource table this backend owns.
func regionKey(region, id string) string { return region + "|" + id }

// executionsStore returns the per-region execution-history map for region,
// lazily creating it. Callers must hold b.mu.
func (b *InMemoryBackend) executionsStore(region string) map[string][]*PipelineExecution {
	if b.executions[region] == nil {
		b.executions[region] = make(map[string][]*PipelineExecution)
	}

	return b.executions[region]
}

// executionsStoreRO returns the per-region execution-history map for region
// without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) executionsStoreRO(region string) map[string][]*PipelineExecution {
	if v := b.executions[region]; v != nil {
		return v
	}

	return map[string][]*PipelineExecution{}
}

// actionExecutionsStore returns the per-region action-execution map for
// region, lazily creating it. Callers must hold b.mu.
func (b *InMemoryBackend) actionExecutionsStore(region string) map[string][]*ActionExecution {
	if b.actionExecutions[region] == nil {
		b.actionExecutions[region] = make(map[string][]*ActionExecution)
	}

	return b.actionExecutions[region]
}

// actionExecutionsStoreRO returns the per-region action-execution map for
// region without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) actionExecutionsStoreRO(region string) map[string][]*ActionExecution {
	if v := b.actionExecutions[region]; v != nil {
		return v
	}

	return map[string][]*ActionExecution{}
}

// actionRevisionKey builds the composite key PutActionRevision/GetPipelineState
// use to identify a single pipeline/stage/action within actionRevisionsStore.
func actionRevisionKey(pipelineName, stageName, actionName string) string {
	return pipelineName + "/" + stageName + "/" + actionName
}

// actionRevisionsStore returns the per-region action-revision map for
// region, lazily creating it. Callers must hold b.mu.
func (b *InMemoryBackend) actionRevisionsStore(region string) map[string]*ActionRevisionRecord {
	if b.actionRevisions[region] == nil {
		b.actionRevisions[region] = make(map[string]*ActionRevisionRecord)
	}

	return b.actionRevisions[region]
}

// actionRevisionsStoreRO returns the per-region action-revision map for
// region without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) actionRevisionsStoreRO(region string) map[string]*ActionRevisionRecord {
	if v := b.actionRevisions[region]; v != nil {
		return v
	}

	return map[string]*ActionRevisionRecord{}
}

// Region returns the default region for this backend instance.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all state in the backend, resetting it to a pristine empty state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.executions = make(map[string]map[string][]*PipelineExecution)
	b.actionExecutions = make(map[string]map[string][]*ActionExecution)
	b.actionRevisions = make(map[string]map[string]*ActionRevisionRecord)
}

func (b *InMemoryBackend) buildPipelineARN(region, name string) string {
	return arn.Build("codepipeline", region, b.accountID, name)
}

func (b *InMemoryBackend) buildWebhookARN(region, name string) string {
	return arn.Build("codepipeline", region, b.accountID, "webhook:"+name)
}

// buildActionTypeARN builds a custom action type's ARN:
// arn:aws:codepipeline:{region}:{account}:actiontype:{owner}/{category}/{provider}/{version}.
// CreateCustomActionTypeOutput carries no ARN field (real ActionType has
// none), so a real caller constructs it from this documented pattern -- same
// as ListTagsForResource must, since custom action types aren't in a
// caller-visible index otherwise.
func (b *InMemoryBackend) buildActionTypeARN(region string, cat *CustomActionType) string {
	owner := cat.Owner
	if owner == "" {
		owner = keyOwnerCustom
	}

	return arn.Build("codepipeline", region, b.accountID,
		"actiontype:"+owner+"/"+cat.Category+"/"+cat.Provider+"/"+cat.Version)
}
