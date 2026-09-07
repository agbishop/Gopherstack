package codepipeline

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CreatePipeline creates a new CodePipeline pipeline.
func (b *InMemoryBackend) CreatePipeline(
	ctx context.Context,
	decl PipelineDeclaration,
	tags map[string]string,
) (*Pipeline, error) {
	b.mu.Lock("CreatePipeline")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if b.pipelines.Has(regionKey(region, decl.Name)) {
		return nil, fmt.Errorf("%w: pipeline %q already exists", ErrPipelineNameInUse, decl.Name)
	}

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	now := float64(time.Now().Unix())
	if decl.Version == 0 {
		decl.Version = 1
	}

	if decl.PipelineType == "" {
		decl.PipelineType = PipelineTypeV1
	}

	if decl.ExecutionMode == "" {
		decl.ExecutionMode = ExecutionModeSuperseded
	}

	p := &Pipeline{
		region:      region,
		Declaration: decl,
		Metadata: PipelineMetadata{
			PipelineArn: b.buildPipelineARN(region, decl.Name),
			Created:     now,
			Updated:     now,
		},
		Tags: tagsCopy,
	}
	b.pipelines.Put(p)

	return copyPipeline(p), nil
}

// GetPipeline returns the pipeline with the given name.
func (b *InMemoryBackend) GetPipeline(ctx context.Context, name string) (*Pipeline, error) {
	b.mu.RLock("GetPipeline")
	defer b.mu.RUnlock()

	p, ok := b.pipelines.Get(regionKey(getRegion(ctx, b.region), name))
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q", ErrNotFound, name)
	}

	return copyPipeline(p), nil
}

// UpdatePipeline replaces the pipeline declaration. decl.Version, if set by
// the caller, is IGNORED: real AWS's PipelineDeclaration.Version field is
// documented as purely informational/system-managed ("A new pipeline always
// has a version number of 1. This number is incremented when a pipeline is
// updated" -- aws-sdk-go-v2/service/codepipeline/types/types.go), with no
// documented optimistic-concurrency check against it anywhere in
// UpdatePipeline's contract, and the real UpdatePipeline API/CLI docs
// describe updating as always incrementing the version by exactly 1
// regardless of what the caller sent. An earlier revision of this backend
// rejected a mismatched Version with a fabricated ConflictException
// (flagged, but left unfixed, in a prior audit pass -- see PARITY.md); that
// was gopherstack-invented behavior with no basis in the real API and has
// been removed.
func (b *InMemoryBackend) UpdatePipeline(ctx context.Context, decl PipelineDeclaration) (*Pipeline, error) {
	b.mu.Lock("UpdatePipeline")
	defer b.mu.Unlock()

	// gopherstack-3djp (errtargetaudit): PipelineNotFoundException is not in
	// UpdatePipeline's modeled error set (deserializeOpErrorUpdatePipeline:
	// InvalidActionDeclarationException/InvalidBlockerDeclarationException/
	// InvalidStageDeclarationException/InvalidStructureException/
	// LimitExceededException/ValidationException only). Left unfixed:
	// unlike DeletePipeline/DeleteCustomActionType/CreateCustomActionType
	// (also undeclared, but no-op-on-absent is a safe fix for those), an
	// update against a nonexistent pipeline has no benign "success"
	// meaning -- InvalidStructureException is a plausible replacement
	// (updating something that isn't there is arguably a structurally
	// invalid request) but that's inference, not a confirmed AWS behavior.
	p, ok := b.pipelines.Get(regionKey(getRegion(ctx, b.region), decl.Name))
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q", ErrNotFound, decl.Name)
	}

	currentVersion := p.Declaration.Version
	p.Declaration = decl
	p.Declaration.Version = currentVersion + 1
	p.Metadata.Updated = float64(time.Now().Unix())

	return copyPipeline(p), nil
}

// DeletePipeline removes the pipeline with the given name and cleans up associated state.
//
// gopherstack-3djp (errtargetaudit): the guard below emits
// PipelineNotFoundException on a nonexistent name, a code confirmed NOT in
// this op's modeled error set (deserializeOpErrorDeletePipeline:
// ConcurrentModificationException/ValidationException only; live docs
// Errors section agrees). Left unfixed: an earlier pass removed this guard
// citing the doc's "HTTP 200 response with an empty HTTP body" sentence as
// evidence of idempotent delete -- that sentence is generic Response-shape
// boilerplate present verbatim on ops that DO error on not-found (see
// DisableStageTransition: same sentence, models PipelineNotFoundException),
// so by itself it is not evidence of idempotent-delete semantics. No
// declared code is an obvious replacement either.
func (b *InMemoryBackend) DeletePipeline(ctx context.Context, name string) error {
	b.mu.Lock("DeletePipeline")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	key := regionKey(region, name)

	if !b.pipelines.Has(key) {
		return fmt.Errorf("%w: pipeline %q", ErrNotFound, name)
	}

	b.pipelines.Delete(key)
	delete(b.executionsStore(region), name)
	delete(b.actionExecutionsStore(region), name)

	// Cascade: remove disabled stage transitions for this pipeline.
	for _, st := range slices.Clone(b.stageTransitionsByPipeline.Get(regionKey(region, name))) {
		b.stageTransitions.Delete(stageTransitionKeyFn(st))
	}

	// Cascade: remove tracked action revisions (PutActionRevision) for this
	// pipeline, keyed by "pipelineName/stage/action" within the per-region
	// map -- otherwise a same-named pipeline recreated later would
	// resurrect the deleted pipeline's stale CurrentRevision data.
	revisions := b.actionRevisionsStore(region)
	for key := range revisions {
		if pn, _, ok := strings.Cut(key, "/"); ok && pn == name {
			delete(revisions, key)
		}
	}

	return nil
}

// ListPipelines returns a sorted summary of all pipelines in the request region.
func (b *InMemoryBackend) ListPipelines(ctx context.Context) []PipelineSummary {
	b.mu.RLock("ListPipelines")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	entries := b.pipelinesByRegion.Get(region)

	summaries := make([]PipelineSummary, 0, len(entries))
	for _, p := range entries {
		summaries = append(summaries, PipelineSummary{
			Name:          p.Declaration.Name,
			Version:       p.Declaration.Version,
			PipelineType:  p.Declaration.PipelineType,
			ExecutionMode: p.Declaration.ExecutionMode,
			Created:       p.Metadata.Created,
			Updated:       p.Metadata.Updated,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})

	return summaries
}

func copyPipeline(p *Pipeline) *Pipeline {
	tagsCopy := make(map[string]string, len(p.Tags))
	maps.Copy(tagsCopy, p.Tags)

	out := *p
	out.Tags = tagsCopy
	out.Declaration = copyDeclaration(p.Declaration)

	return &out
}

// AddPipelineInternal seeds a pipeline directly into the backend's default region (for testing).
func (b *InMemoryBackend) AddPipelineInternal(decl PipelineDeclaration, tags map[string]string) *Pipeline {
	b.mu.Lock("AddPipelineInternal")
	defer b.mu.Unlock()

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	now := float64(time.Now().Unix())
	if decl.Version == 0 {
		decl.Version = 1
	}

	p := &Pipeline{
		region:      b.region,
		Declaration: decl,
		Metadata: PipelineMetadata{
			PipelineArn: b.buildPipelineARN(b.region, decl.Name),
			Created:     now,
			Updated:     now,
		},
		Tags: tagsCopy,
	}
	b.pipelines.Put(p)

	return copyPipeline(p)
}

// copyDeclaration deep-copies a PipelineDeclaration so callers cannot mutate
// the backend's stored stages, actions, or configuration maps.
func copyDeclaration(d PipelineDeclaration) PipelineDeclaration {
	out := d
	out.Stages = copyStages(d.Stages)
	out.Variables = copyVariables(d.Variables)
	out.Triggers = copyTriggers(d.Triggers)

	if d.ArtifactStores != nil {
		out.ArtifactStores = make(map[string]ArtifactStore, len(d.ArtifactStores))
		maps.Copy(out.ArtifactStores, d.ArtifactStores)
	}

	return out
}

func copyVariables(vars []PipelineVariable) []PipelineVariable {
	if vars == nil {
		return nil
	}

	out := make([]PipelineVariable, len(vars))
	copy(out, vars)

	return out
}

func copyTriggers(triggers []Trigger) []Trigger {
	if triggers == nil {
		return nil
	}

	out := make([]Trigger, len(triggers))
	copy(out, triggers)

	return out
}

func copyStages(stages []Stage) []Stage {
	if stages == nil {
		return nil
	}

	out := make([]Stage, len(stages))
	for i, s := range stages {
		out[i] = Stage{
			Name:        s.Name,
			Type:        s.Type,
			Actions:     copyActions(s.Actions),
			BeforeEntry: copyCondition(s.BeforeEntry),
			OnFailure:   copyCondition(s.OnFailure),
			OnSuccess:   copyCondition(s.OnSuccess),
		}
	}

	return out
}

func copyCondition(c *Condition) *Condition {
	if c == nil {
		return nil
	}

	cp := *c
	if c.Rules != nil {
		cp.Rules = make([]Rule, len(c.Rules))
		copy(cp.Rules, c.Rules)
	}

	return &cp
}

func copyActions(actions []Action) []Action {
	if actions == nil {
		return nil
	}

	out := make([]Action, len(actions))
	for i, a := range actions {
		actionCopy := a
		actionCopy.Configuration = copyStringMap(a.Configuration)
		actionCopy.InputArtifacts = copyArtifactRefs(a.InputArtifacts)
		actionCopy.OutputArtifacts = copyArtifactRefs(a.OutputArtifacts)
		out[i] = actionCopy
	}

	return out
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}

	out := make(map[string]string, len(m))
	maps.Copy(out, m)

	return out
}

func copyArtifactRefs(refs []ArtifactRef) []ArtifactRef {
	if refs == nil {
		return nil
	}

	out := make([]ArtifactRef, len(refs))
	copy(out, refs)

	return out
}

// StartPipelineExecution starts and stores a new execution of a pipeline.
//
// gopherstack runs every action synchronously via runPipelineActions
// (action_engine.go): ordinary actions complete instantly, but an
// Approval-category action gates the run and leaves the execution
// InProgress until PutApprovalResult resolves it (see PutApprovalResult in
// approvals.go). Leaving Status at statusInProgress unconditionally here
// (the pre-fix behavior) left every execution stuck InProgress forever, even
// when it contained no approval gate at all: GetPipelineExecution/
// ListPipelineExecutions would never report a terminal status, so any client
// polling for completion (as the real, asynchronous AWS service expects
// callers to do) would spin indefinitely.
func (b *InMemoryBackend) StartPipelineExecution(ctx context.Context, pipelineName string) (*PipelineExecution, error) {
	b.mu.Lock("StartPipelineExecution")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelines.Get(regionKey(region, pipelineName))
	if !ok {
		return nil, ErrNotFound
	}

	now := time.Now().UTC()
	exec := &PipelineExecution{
		PipelineName:        pipelineName,
		PipelineExecutionID: uuid.NewString(),
		Status:              statusInProgress,
		PipelineVersion:     p.Declaration.Version,
		ExecutionMode:       p.Declaration.ExecutionMode,
		ExecutionType:       executionTypeStandard,
		Trigger:             triggerTypeStartExecution,
		StartTime:           now,
		LastUpdateTime:      now,
	}

	execs := b.executionsStore(region)
	execs[pipelineName] = append(execs[pipelineName], exec)

	b.runPipelineActions(region, p, exec)
	exec.LastUpdateTime = time.Now().UTC()

	cp := *exec

	return &cp, nil
}

// StopPipelineExecution stops a pipeline execution. Real AWS transitions
// through a transient "Stopping" state while in-progress actions finish (or
// are abandoned, if abandon is true) before reaching the terminal "Stopped"
// state. gopherstack runs every ordinary action synchronously and
// instantaneously (see StartPipelineExecution), so there is never an
// in-progress *ordinary* action left to wait for -- the execution goes
// straight to "Stopped" regardless of abandon. Leaving it at "Stopping" left
// every stopped execution stuck there forever, indistinguishable (to a
// polling client) from a stop request that never completed.
//
// An execution can, however, still be genuinely InProgress here if it is
// gated on a manual approval (see runPipelineActions in action_engine.go):
// stopping such an execution abandons that pending approval action (marks it
// Abandoned and clears its token) so a subsequent PutApprovalResult against
// a stopped execution correctly fails instead of silently resurrecting it.
func (b *InMemoryBackend) StopPipelineExecution(
	ctx context.Context,
	pipelineName, executionID, reason string,
	abandon bool,
) (*PipelineExecution, error) {
	b.mu.Lock("StopPipelineExecution")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if !b.pipelines.Has(regionKey(region, pipelineName)) {
		return nil, ErrNotFound
	}

	// abandon has no independent effect in this synchronous backend: there is
	// never an in-progress *ordinary* action to wait out (see doc comment),
	// so both abandon=true and abandon=false immediately abandon any pending
	// approval gate and reach the terminal Stopped state.
	_, _ = reason, abandon

	for _, exec := range b.executionsStore(region)[pipelineName] {
		if exec.PipelineExecutionID != executionID {
			continue
		}

		now := time.Now().UTC()

		for _, ae := range b.actionExecutionsStore(region)[pipelineName] {
			if ae.PipelineExecutionID == executionID && ae.Status == statusInProgress {
				ae.Status = statusActionAbandoned
				ae.Token = ""
				ae.LastUpdateTime = now
			}
		}

		exec.Status = statusStopped
		exec.LastUpdateTime = now
		cp := *exec

		return &cp, nil
	}

	// gopherstack-3djp (errtargetaudit): same undeclared-code issue as
	// OverrideStageCondition/RetryStageExecution -- PipelineExecutionNotFoundException
	// is not in StopPipelineExecution's modeled error set
	// (deserializeOpErrorStopPipelineExecution: PipelineNotFoundException/
	// PipelineExecutionNotStoppableException/DuplicatedStopRequestException/
	// ConflictException/ValidationException only). Left unfixed: no
	// declared code obviously means "this executionID doesn't exist".
	return nil, fmt.Errorf("%w: pipeline %q execution %q", ErrExecutionNotFound, pipelineName, executionID)
}
