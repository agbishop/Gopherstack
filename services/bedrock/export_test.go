package bedrock

import (
	"encoding/json"
	"strconv"
	"time"
)

// SeedCustomModelForTest inserts m directly into the backend, bypassing
// CreateCustomModel's time.Now() CreationTime stamp so tests can construct
// an exact tie between two models' CreationTime.
func (b *InMemoryBackend) SeedCustomModelForTest(m *CustomModel) {
	b.mu.Lock("SeedCustomModelForTest")
	defer b.mu.Unlock()
	b.customModels.Put(m)
}

// SeedEvaluationJobForTest inserts j directly into the backend, bypassing
// CreateEvaluationJob's time.Now() CreationTime stamp.
func (b *InMemoryBackend) SeedEvaluationJobForTest(j *EvaluationJob) {
	b.mu.Lock("SeedEvaluationJobForTest")
	defer b.mu.Unlock()
	b.evaluationJobs.Put(j)
}

// SeedCustomModelDeploymentForTest inserts d directly into the backend,
// bypassing CreateCustomModelDeployment's time.Now() CreationTime stamp.
func (b *InMemoryBackend) SeedCustomModelDeploymentForTest(d *CustomModelDeployment) {
	b.mu.Lock("SeedCustomModelDeploymentForTest")
	defer b.mu.Unlock()
	b.customModelDeployments.Put(d)
}

// SeedModelCopyJobForTest inserts j directly into the backend, bypassing
// CopyModel's time.Now() CreationTime stamp.
func (b *InMemoryBackend) SeedModelCopyJobForTest(j *ModelCopyJob) {
	b.mu.Lock("SeedModelCopyJobForTest")
	defer b.mu.Unlock()
	b.modelCopyJobs.Put(j)
}

// SeedModelInvocationJobForTest inserts j directly into the backend,
// bypassing CreateModelInvocationJob's time.Now() CreationTime stamp.
func (b *InMemoryBackend) SeedModelInvocationJobForTest(j *ModelInvocationJob) {
	b.mu.Lock("SeedModelInvocationJobForTest")
	defer b.mu.Unlock()
	b.modelInvocationJobs.Put(j)
}

// SeedModelImportJobForTest inserts j directly into the backend, bypassing
// CreateModelImportJob's time.Now() CreationTime stamp.
func (b *InMemoryBackend) SeedModelImportJobForTest(j *ModelImportJob) {
	b.mu.Lock("SeedModelImportJobForTest")
	defer b.mu.Unlock()
	b.modelImportJobs.Put(j)
}

// SeedModelCustomizationJobForTest inserts j directly into the backend,
// bypassing CreateModelCustomizationJob's time.Now() CreationTime stamp.
func (b *InMemoryBackend) SeedModelCustomizationJobForTest(j *ModelCustomizationJob) {
	b.mu.Lock("SeedModelCustomizationJobForTest")
	defer b.mu.Unlock()
	b.modelCustomizationJobs.Put(j)
}

// SeedAdvancedPromptOptimizationJobForTest inserts j directly into the
// backend, bypassing CreateAdvancedPromptOptimizationJob's time.Now()
// CreationTime stamp.
func (b *InMemoryBackend) SeedAdvancedPromptOptimizationJobForTest(j *AdvancedPromptOptimizationJob) {
	b.mu.Lock("SeedAdvancedPromptOptimizationJobForTest")
	defer b.mu.Unlock()
	b.advancedPromptOptimizationJobs.Put(j)
}

// SeedProvisionedModelThroughputForTest inserts p directly into the backend,
// bypassing CreateProvisionedModelThroughput's time.Now() CreationTime stamp.
func (b *InMemoryBackend) SeedProvisionedModelThroughputForTest(p *ProvisionedModelThroughput) {
	b.mu.Lock("SeedProvisionedModelThroughputForTest")
	defer b.mu.Unlock()
	b.provisionedModelThroughputs.Put(p)
}

// AppendFoundationModelsForTest appends additional foundation models to the backend.
// This is only used in tests to populate beyond the default seeded models.
func (b *InMemoryBackend) AppendFoundationModelsForTest(models []*FoundationModelSummary) {
	b.mu.Lock("AppendFoundationModelsForTest")
	defer b.mu.Unlock()
	b.foundationModels = append(b.foundationModels, models...)
}

// AddBuildWorkflowForTest adds a build workflow to the backend for testing.
func (b *InMemoryBackend) AddBuildWorkflowForTest(policyARN string) *AutomatedReasoningPolicyBuildWorkflow {
	b.mu.Lock("AddBuildWorkflowForTest")
	defer b.mu.Unlock()

	b.arpWorkflowCounter++
	id := "bw-" + strconv.Itoa(b.arpWorkflowCounter)

	wf := &AutomatedReasoningPolicyBuildWorkflow{
		BuildWorkflowID: id,
		PolicyArn:       policyARN,
		Status:          "Running",
	}
	b.arpBuildWorkflows.Put(wf)

	return wf
}

// SnapshotTablesForTest returns a snapshot of every store.Table registered on
// b.registry, keyed by table name (including per-parent lazy tables such as
// "flowVersions:<flowID>"). It is a test-only bridge to the unexported
// registry so blackbox tests can exercise the Phase 3.3 pkgs/store
// conversion's registry mechanics directly, independent of the production
// Snapshot/Restore pair added in persistence.go (see persistence_test.go for
// black-box tests of that production path).
func (b *InMemoryBackend) SnapshotTablesForTest() (map[string]json.RawMessage, error) {
	b.mu.RLock("SnapshotTablesForTest")
	defer b.mu.RUnlock()

	return b.registry.SnapshotAll()
}

// RestoreTablesForTest replaces every store.Table registered on b.registry
// with the contents of data, as produced by SnapshotTablesForTest.
func (b *InMemoryBackend) RestoreTablesForTest(data map[string]json.RawMessage) error {
	b.mu.Lock("RestoreTablesForTest")
	defer b.mu.Unlock()

	return b.registry.RestoreAll(data)
}

// ResetTablesForTest clears every store.Table registered on b.registry in
// place, leaving secondary-index maps (guardrailsByName, etc.) and counters
// untouched. Mirrors the registry.ResetAll call inside Reset.
func (b *InMemoryBackend) ResetTablesForTest() {
	b.mu.Lock("ResetTablesForTest")
	defer b.mu.Unlock()

	b.registry.ResetAll()
}

// GuardrailVersionCounterForTest returns guardrailID's current versionCounter
// (0 if the guardrail is absent). Guardrail.versionCounter is unexported --
// see persistence.go's backendSnapshot doc comment for why -- so black-box
// tests need this bridge to assert it survives a Snapshot/Restore round trip.
func (b *InMemoryBackend) GuardrailVersionCounterForTest(guardrailID string) int {
	b.mu.RLock("GuardrailVersionCounterForTest")
	defer b.mu.RUnlock()

	g, ok := b.guardrails.Get(guardrailID)
	if !ok {
		return 0
	}

	return g.versionCounter
}

// AgentPreparationDueAtForTest returns agentID's current preparationDueAt
// (the zero Time if the agent is absent). Agent.preparationDueAt is
// unexported -- see persistence.go's backendSnapshot doc comment for why --
// so black-box tests need this bridge to assert it survives a
// Snapshot/Restore round trip.
func (b *InMemoryBackend) AgentPreparationDueAtForTest(agentID string) time.Time {
	b.mu.RLock("AgentPreparationDueAtForTest")
	defer b.mu.RUnlock()

	a, ok := b.agents.Get(agentID)
	if !ok {
		return time.Time{}
	}

	return a.preparationDueAt
}

// FlowVersionCounterForTest returns the current per-flow version counter for
// flowID (0 if absent). flowVersionCounters is unexported plain-map state --
// see store_setup.go's registerAllTables doc comment -- so black-box tests
// need this bridge to assert it is pruned on flow delete.
func (b *InMemoryBackend) FlowVersionCounterForTest(flowID string) int {
	b.mu.RLock("FlowVersionCounterForTest")
	defer b.mu.RUnlock()

	return b.flowVersionCounters[flowID]
}

// PromptVersionCounterForTest returns the current per-prompt version counter
// for promptID (0 if absent).
func (b *InMemoryBackend) PromptVersionCounterForTest(promptID string) int {
	b.mu.RLock("PromptVersionCounterForTest")
	defer b.mu.RUnlock()

	return b.promptVersionCounters[promptID]
}

// ARPVersionCountForTest returns the current per-policy version counter for
// policyARN (0 if absent).
func (b *InMemoryBackend) ARPVersionCountForTest(policyARN string) int {
	b.mu.RLock("ARPVersionCountForTest")
	defer b.mu.RUnlock()

	return b.arpVersionCountByPolicy[policyARN]
}

// ARPAnnotationStateExistsForTest reports whether any of the annotation-state
// maps (arpAnnotations, arpAnnotationSetHash, arpAnnotationsUpdatedAt) still
// holds an entry for (policyARN, buildWorkflowID).
func (b *InMemoryBackend) ARPAnnotationStateExistsForTest(policyARN, buildWorkflowID string) bool {
	b.mu.RLock("ARPAnnotationStateExistsForTest")
	defer b.mu.RUnlock()

	key := arpAnnotationsKey(policyARN, buildWorkflowID)
	_, hasAnn := b.arpAnnotations[key]
	_, hasHash := b.arpAnnotationSetHash[key]
	_, hasUpdated := b.arpAnnotationsUpdatedAt[key]

	return hasAnn || hasHash || hasUpdated
}

// IngestionJobCompletionDueAtForTest returns the current completionDueAt for
// the ingestion job identified by (kbID, dsID, jobID) (the zero Time if
// absent). IngestionJob.completionDueAt is unexported -- see persistence.go's
// backendSnapshot doc comment for why -- so black-box tests need this bridge
// to assert it survives a Snapshot/Restore round trip.
func (b *InMemoryBackend) IngestionJobCompletionDueAtForTest(kbID, dsID, jobID string) time.Time {
	b.mu.RLock("IngestionJobCompletionDueAtForTest")
	defer b.mu.RUnlock()

	j, ok := b.ingestionJobs.Get(ingestionJobKey(kbID, dsID, jobID))
	if !ok {
		return time.Time{}
	}

	return j.completionDueAt
}
