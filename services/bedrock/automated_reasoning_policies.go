package bedrock

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// newARPOpaqueHash mints an opaque, time-derived concurrency token, matching
// the convention CreateAutomatedReasoningPolicy already uses for
// DefinitionHash. It is not a content hash -- like AWS's own definitionHash/
// annotationSetHash, this backend does not claim its algorithm, only that it
// changes on every real mutation.
func newARPOpaqueHash() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// newARPID generates a unique automated reasoning policy ID.
func (b *InMemoryBackend) newARPID() string {
	b.arpCounter++

	return fmt.Sprintf("arp-%07d", b.arpCounter)
}

// newARPTestCaseID generates a unique test case ID.
func (b *InMemoryBackend) newARPTestCaseID() string {
	b.arpTestCaseCounter++

	return fmt.Sprintf("tc-%07d", b.arpTestCaseCounter)
}

// newARPVersionNum generates a monotonically increasing version number for a policy.
// Must be called with the write lock held.
func (b *InMemoryBackend) newARPVersionNum(policyARN string) string {
	b.arpVersionCountByPolicy[policyARN]++

	return strconv.Itoa(b.arpVersionCountByPolicy[policyARN])
}

// CreateAutomatedReasoningPolicy creates a new Automated Reasoning policy.
func (b *InMemoryBackend) CreateAutomatedReasoningPolicy(
	name, description string,
	tags []Tag,
) (*AutomatedReasoningPolicy, error) {
	b.mu.Lock("CreateAutomatedReasoningPolicy")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	if _, exists := b.arpByName[name]; exists {
		return nil, fmt.Errorf(
			"%w: automated reasoning policy %s already exists",
			ErrAlreadyExists,
			name,
		)
	}

	id := b.newARPID()
	policyARN := arn.Build("bedrock", b.region, b.accountID, "automated-reasoning-policy/"+id)
	now := time.Now().UTC()

	policy := &AutomatedReasoningPolicy{
		PolicyArn:      policyARN,
		Name:           name,
		Description:    description,
		Status:         "ACTIVE",
		CreatedAt:      now,
		UpdatedAt:      now,
		DefinitionHash: fmt.Sprintf("%x", now.UnixNano()),
		Version:        "DRAFT",
		Tags:           copyTags(tags),
	}
	b.automatedReasoningPolicies.Put(policy)
	b.arpByName[name] = policyARN
	cp := *policy
	cp.Tags = copyTags(policy.Tags)

	return &cp, nil
}

// CancelAutomatedReasoningPolicyBuildWorkflow cancels a running build workflow.
func (b *InMemoryBackend) CancelAutomatedReasoningPolicyBuildWorkflow(
	policyARN, workflowID string,
) error {
	b.mu.Lock("CancelAutomatedReasoningPolicyBuildWorkflow")
	defer b.mu.Unlock()

	if _, ok := b.automatedReasoningPolicies.Get(policyARN); !ok {
		return fmt.Errorf("%w: automated reasoning policy %s not found", ErrNotFound, policyARN)
	}

	wf, ok := b.arpBuildWorkflows.Get(workflowID)
	if !ok {
		return fmt.Errorf("%w: build workflow %s not found", ErrNotFound, workflowID)
	}

	if wf.PolicyArn != policyARN {
		return fmt.Errorf(
			"%w: build workflow %s does not belong to policy %s",
			ErrNotFound,
			workflowID,
			policyARN,
		)
	}

	wf.Status = "Cancelled"
	wf.UpdatedAt = time.Now().UTC()

	return nil
}

// CreateAutomatedReasoningPolicyTestCase creates a test case for an Automated Reasoning policy.
func (b *InMemoryBackend) CreateAutomatedReasoningPolicyTestCase(
	policyARN string,
) (*AutomatedReasoningPolicyTestCase, error) {
	b.mu.Lock("CreateAutomatedReasoningPolicyTestCase")
	defer b.mu.Unlock()

	if _, ok := b.automatedReasoningPolicies.Get(policyARN); !ok {
		return nil, fmt.Errorf(
			"%w: automated reasoning policy %s not found",
			ErrNotFound,
			policyARN,
		)
	}

	id := b.newARPTestCaseID()
	now := time.Now().UTC()
	tc := &AutomatedReasoningPolicyTestCase{
		TestCaseID: id,
		PolicyArn:  policyARN,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	b.arpTestCases.Put(tc)
	cp := *tc

	return &cp, nil
}

// CreateAutomatedReasoningPolicyVersion creates a new version of an Automated Reasoning policy.
func (b *InMemoryBackend) CreateAutomatedReasoningPolicyVersion(
	policyARN, definitionHash string,
	tags []Tag,
) (*AutomatedReasoningPolicyVersion, error) {
	b.mu.Lock("CreateAutomatedReasoningPolicyVersion")
	defer b.mu.Unlock()

	policy, ok := b.automatedReasoningPolicies.Get(policyARN)
	if !ok {
		return nil, fmt.Errorf(
			"%w: automated reasoning policy %s not found",
			ErrNotFound,
			policyARN,
		)
	}

	// Use per-policy version counter for realistic monotonic versioning.
	versionNum := b.newARPVersionNum(policyARN)
	versionedARN := policyARN + "/version/" + versionNum

	version := &AutomatedReasoningPolicyVersion{
		PolicyArn:      versionedARN,
		Name:           policy.Name,
		DefinitionHash: definitionHash,
		Version:        versionNum,
		CreatedAt:      time.Now().UTC(),
		Tags:           copyTags(tags),
	}
	b.arpVersions.Put(version)
	cp := *version

	return &cp, nil
}

const statusRunning = "Running"

// GetAutomatedReasoningPolicy returns a single ARP by ARN.
func (b *InMemoryBackend) GetAutomatedReasoningPolicy(policyARN string) (*AutomatedReasoningPolicy, error) {
	b.mu.RLock("GetAutomatedReasoningPolicy")
	defer b.mu.RUnlock()

	policy, ok := b.automatedReasoningPolicies.Get(policyARN)
	if !ok {
		return nil, fmt.Errorf("%w: automated reasoning policy %s not found", ErrNotFound, policyARN)
	}

	cp := *policy
	cp.Tags = copyTags(policy.Tags)

	return &cp, nil
}

// ListAutomatedReasoningPolicies returns all policies.
func (b *InMemoryBackend) ListAutomatedReasoningPolicies() []*AutomatedReasoningPolicy {
	b.mu.RLock("ListAutomatedReasoningPolicies")
	defer b.mu.RUnlock()

	policies := make([]*AutomatedReasoningPolicy, 0, b.automatedReasoningPolicies.Len())
	for _, p := range b.automatedReasoningPolicies.All() {
		cp := *p
		cp.Tags = copyTags(p.Tags)
		policies = append(policies, &cp)
	}

	sort.Slice(policies, func(i, k int) bool {
		return policies[i].Name < policies[k].Name
	})

	return policies
}

// UpdateAutomatedReasoningPolicy updates a policy's definition (required --
// bedrock@v1.66.4 api_op_UpdateAutomatedReasoningPolicy.go:37-63) and,
// optionally, its name and description. name only renames when non-empty:
// unlike description, policy.Name backs the arpByName secondary index, so an
// unconditional overwrite on every PATCH (including ones that omit name)
// would orphan that index instead of leaving the name unchanged.
func (b *InMemoryBackend) UpdateAutomatedReasoningPolicy(
	policyARN, name, description string,
	policyDefinition json.RawMessage,
) (*AutomatedReasoningPolicy, error) {
	if len(policyDefinition) == 0 {
		return nil, fmt.Errorf("%w: policyDefinition is required", ErrValidation)
	}

	b.mu.Lock("UpdateAutomatedReasoningPolicy")
	defer b.mu.Unlock()

	policy, ok := b.automatedReasoningPolicies.Get(policyARN)
	if !ok {
		return nil, fmt.Errorf("%w: automated reasoning policy %s not found", ErrNotFound, policyARN)
	}

	policy.PolicyDefinition = policyDefinition
	policy.DefinitionHash = newARPOpaqueHash()
	policy.Description = description

	if name != "" && name != policy.Name {
		delete(b.arpByName, policy.Name)
		policy.Name = name
		b.arpByName[name] = policyARN
	}

	policy.UpdatedAt = time.Now().UTC()

	cp := *policy
	cp.Tags = copyTags(policy.Tags)

	return &cp, nil
}

// arpHasTestCases reports whether any test case still belongs to policyARN.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) arpHasTestCases(policyARN string) bool {
	for _, tc := range b.arpTestCases.All() {
		if tc.PolicyArn == policyARN {
			return true
		}
	}

	return false
}

// arpHasVersions reports whether any version still belongs to policyARN.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) arpHasVersions(policyARN string) bool {
	for _, v := range b.arpVersions.All() {
		if base, _, versioned := splitVersionedARN(v.PolicyArn); versioned && base == policyARN {
			return true
		}
	}

	return false
}

// deleteARPAnnotationState removes the annotation-state entries
// (arpAnnotations/arpAnnotationSetHash/arpAnnotationsUpdatedAt) minted for
// one build workflow. Plain maps, not store.Table -- delete() is correct
// (see store_setup.go's registerAllTables doc comment). Caller must hold the
// write lock.
func (b *InMemoryBackend) deleteARPAnnotationState(policyARN, buildWorkflowID string) {
	key := arpAnnotationsKey(policyARN, buildWorkflowID)
	delete(b.arpAnnotations, key)
	delete(b.arpAnnotationSetHash, key)
	delete(b.arpAnnotationsUpdatedAt, key)
}

// deleteARPArtifacts removes every build workflow (and its annotation
// state), test case, and version belonging to policyARN. Caller must hold
// the write lock.
func (b *InMemoryBackend) deleteARPArtifacts(policyARN string) {
	for _, wf := range b.arpBuildWorkflows.All() {
		if wf.PolicyArn == policyARN {
			b.deleteARPAnnotationState(policyARN, wf.BuildWorkflowID)
			b.arpBuildWorkflows.Delete(wf.BuildWorkflowID)
		}
	}

	for _, tc := range b.arpTestCases.All() {
		if tc.PolicyArn == policyARN {
			b.arpTestCases.Delete(tc.TestCaseID)
		}
	}

	for _, v := range b.arpVersions.All() {
		if base, _, versioned := splitVersionedARN(v.PolicyArn); versioned && base == policyARN {
			b.arpVersions.Delete(arpVersionsKeyFn(v))
		}
	}
}

// DeleteAutomatedReasoningPolicy removes a policy and its related resources.
// When force is false, AWS validates that all artifacts (policy versions,
// test cases) have already been deleted before allowing the policy itself
// to be removed (bedrock@v1.66.4 api_op_DeleteAutomatedReasoningPolicy.go:36-41).
func (b *InMemoryBackend) DeleteAutomatedReasoningPolicy(policyARN string, force bool) error {
	b.mu.Lock("DeleteAutomatedReasoningPolicy")
	defer b.mu.Unlock()

	policy, ok := b.automatedReasoningPolicies.Get(policyARN)
	if !ok {
		return fmt.Errorf("%w: automated reasoning policy %s not found", ErrNotFound, policyARN)
	}

	if !force && b.arpHasTestCases(policyARN) {
		return fmt.Errorf(
			"%w: automated reasoning policy %s still has test cases; delete them or pass force",
			ErrResourceInUse, policyARN,
		)
	}

	if !force && b.arpHasVersions(policyARN) {
		return fmt.Errorf(
			"%w: automated reasoning policy %s still has versions; delete them or pass force",
			ErrResourceInUse, policyARN,
		)
	}

	delete(b.arpByName, policy.Name)
	delete(b.arpVersionCountByPolicy, policyARN)
	b.automatedReasoningPolicies.Delete(policyARN)
	b.deleteARPArtifacts(policyARN)

	return nil
}

// StartAutomatedReasoningPolicyBuildWorkflow creates a new build workflow for
// a policy. buildWorkflowType and sourceContent are both required
// (bedrock@v1.66.4 api_op_StartAutomatedReasoningPolicyBuildWorkflow.go:37-53);
// buildWorkflowType arrives as a URI label, sourceContent as the entire JSON
// request body (serializers.go:8008-8058).
func (b *InMemoryBackend) StartAutomatedReasoningPolicyBuildWorkflow(
	policyARN, buildWorkflowType string,
	sourceContent json.RawMessage,
) (*AutomatedReasoningPolicyBuildWorkflow, error) {
	if buildWorkflowType == "" {
		return nil, fmt.Errorf("%w: buildWorkflowType is required", ErrValidation)
	}

	b.mu.Lock("StartAutomatedReasoningPolicyBuildWorkflow")
	defer b.mu.Unlock()

	if _, ok := b.automatedReasoningPolicies.Get(policyARN); !ok {
		return nil, fmt.Errorf("%w: automated reasoning policy %s not found", ErrNotFound, policyARN)
	}

	b.arpWorkflowCounter++
	id := "bw-" + strconv.Itoa(b.arpWorkflowCounter)
	now := time.Now().UTC()

	wf := &AutomatedReasoningPolicyBuildWorkflow{
		BuildWorkflowID:   id,
		PolicyArn:         policyARN,
		Status:            statusRunning,
		BuildWorkflowType: buildWorkflowType,
		SourceContent:     sourceContent,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	b.arpBuildWorkflows.Put(wf)
	cp := *wf

	return &cp, nil
}

// GetAutomatedReasoningPolicyBuildWorkflow returns a workflow by policy ARN and workflow ID.
func (b *InMemoryBackend) GetAutomatedReasoningPolicyBuildWorkflow(
	policyARN, workflowID string,
) (*AutomatedReasoningPolicyBuildWorkflow, error) {
	b.mu.RLock("GetAutomatedReasoningPolicyBuildWorkflow")
	defer b.mu.RUnlock()

	wf, ok := b.arpBuildWorkflows.Get(workflowID)
	if !ok || wf.PolicyArn != policyARN {
		return nil, fmt.Errorf("%w: build workflow %s not found", ErrNotFound, workflowID)
	}

	cp := *wf

	return &cp, nil
}

// ListAutomatedReasoningPolicyBuildWorkflows returns all workflows for a policy.
func (b *InMemoryBackend) ListAutomatedReasoningPolicyBuildWorkflows(
	policyARN string,
) []*AutomatedReasoningPolicyBuildWorkflow {
	b.mu.RLock("ListAutomatedReasoningPolicyBuildWorkflows")
	defer b.mu.RUnlock()

	var workflows []*AutomatedReasoningPolicyBuildWorkflow
	for _, wf := range b.arpBuildWorkflows.All() {
		if wf.PolicyArn == policyARN {
			cp := *wf
			workflows = append(workflows, &cp)
		}
	}

	sort.Slice(workflows, func(i, k int) bool {
		return workflows[i].BuildWorkflowID < workflows[k].BuildWorkflowID
	})

	return workflows
}

// DeleteAutomatedReasoningPolicyBuildWorkflow removes a build workflow.
func (b *InMemoryBackend) DeleteAutomatedReasoningPolicyBuildWorkflow(policyARN, workflowID string) error {
	b.mu.Lock("DeleteAutomatedReasoningPolicyBuildWorkflow")
	defer b.mu.Unlock()

	wf, ok := b.arpBuildWorkflows.Get(workflowID)
	if !ok || wf.PolicyArn != policyARN {
		return fmt.Errorf("%w: build workflow %s not found", ErrNotFound, workflowID)
	}

	b.deleteARPAnnotationState(policyARN, workflowID)
	b.arpBuildWorkflows.Delete(workflowID)

	return nil
}

// GetAutomatedReasoningPolicyTestCase returns a test case by ID.
func (b *InMemoryBackend) GetAutomatedReasoningPolicyTestCase(
	policyARN, testCaseID string,
) (*AutomatedReasoningPolicyTestCase, error) {
	b.mu.RLock("GetAutomatedReasoningPolicyTestCase")
	defer b.mu.RUnlock()

	tc, ok := b.arpTestCases.Get(testCaseID)
	if !ok || tc.PolicyArn != policyARN {
		return nil, fmt.Errorf("%w: test case %s not found", ErrNotFound, testCaseID)
	}

	cp := *tc

	return &cp, nil
}

// ListAutomatedReasoningPolicyTestCases returns all test cases for a policy.
func (b *InMemoryBackend) ListAutomatedReasoningPolicyTestCases(policyARN string) []*AutomatedReasoningPolicyTestCase {
	b.mu.RLock("ListAutomatedReasoningPolicyTestCases")
	defer b.mu.RUnlock()

	var cases []*AutomatedReasoningPolicyTestCase
	for _, tc := range b.arpTestCases.All() {
		if tc.PolicyArn == policyARN {
			cp := *tc
			cases = append(cases, &cp)
		}
	}

	sort.Slice(cases, func(i, k int) bool {
		return cases[i].TestCaseID < cases[k].TestCaseID
	})

	return cases
}

// UpdateAutomatedReasoningPolicyTestCase updates a test case's content, query,
// expected result, and confidence threshold
// (aws-sdk-go-v2 api_op_UpdateAutomatedReasoningPolicyTestCase.go:34-71).
func (b *InMemoryBackend) UpdateAutomatedReasoningPolicyTestCase(
	policyARN, testCaseID, guardContent, queryContent, expectedResult string,
	confidenceThreshold *float64,
) (*AutomatedReasoningPolicyTestCase, error) {
	b.mu.Lock("UpdateAutomatedReasoningPolicyTestCase")
	defer b.mu.Unlock()

	tc, ok := b.arpTestCases.Get(testCaseID)
	if !ok || tc.PolicyArn != policyARN {
		return nil, fmt.Errorf("%w: test case %s not found", ErrNotFound, testCaseID)
	}

	if guardContent == "" {
		return nil, fmt.Errorf("%w: guardContent is required", ErrValidation)
	}

	if expectedResult == "" {
		return nil, fmt.Errorf("%w: expectedAggregatedFindingsResult is required", ErrValidation)
	}

	tc.GuardContent = guardContent
	tc.QueryContent = queryContent
	tc.ExpectedAggregatedFindingsResult = expectedResult
	tc.ConfidenceThreshold = confidenceThreshold
	tc.UpdatedAt = time.Now().UTC()

	cp := *tc

	return &cp, nil
}

// DeleteAutomatedReasoningPolicyTestCase removes a test case.
func (b *InMemoryBackend) DeleteAutomatedReasoningPolicyTestCase(policyARN, testCaseID string) error {
	b.mu.Lock("DeleteAutomatedReasoningPolicyTestCase")
	defer b.mu.Unlock()

	tc, ok := b.arpTestCases.Get(testCaseID)
	if !ok || tc.PolicyArn != policyARN {
		return fmt.Errorf("%w: test case %s not found", ErrNotFound, testCaseID)
	}

	b.arpTestCases.Delete(testCaseID)

	return nil
}

// mustGetARPBuildWorkflow errors unless buildWorkflowID exists and belongs to
// policyARN. Caller must hold b.mu.
func (b *InMemoryBackend) mustGetARPBuildWorkflow(policyARN, buildWorkflowID string) error {
	wf, ok := b.arpBuildWorkflows.Get(buildWorkflowID)
	if !ok || wf.PolicyArn != policyARN {
		return fmt.Errorf("%w: build workflow %s not found", ErrNotFound, buildWorkflowID)
	}

	return nil
}

func arpAnnotationsKey(policyARN, buildWorkflowID string) string {
	return policyARN + ":" + buildWorkflowID
}

// GetAutomatedReasoningPolicyAnnotations returns annotations for a build workflow
// (bedrock@v1.66.4 serializers.go:3874 — build-workflow-scoped, not policy-scoped).
// annotationSetHash is required on the real output
// (api_op_GetAutomatedReasoningPolicyAnnotations.go:54) and doubles as the
// token UpdateAutomatedReasoningPolicyAnnotations's required
// lastUpdatedAnnotationSetHash checks against, so it is minted here on first
// read rather than left absent. buildWorkflowId/name/policyArn/updatedAt are
// also required (api_op_GetAutomatedReasoningPolicyAnnotations.go:62-80) and
// were previously dropped entirely; updatedAt is lazily minted alongside the
// hash for the same reason. Uses the write lock because that lazy mint
// mutates arpAnnotationSetHash/arpAnnotationsUpdatedAt.
func (b *InMemoryBackend) GetAutomatedReasoningPolicyAnnotations(
	policyARN, buildWorkflowID string,
) (map[string]any, error) {
	b.mu.Lock("GetAutomatedReasoningPolicyAnnotations")
	defer b.mu.Unlock()

	if err := b.mustGetARPBuildWorkflow(policyARN, buildWorkflowID); err != nil {
		return nil, err
	}

	policy, ok := b.automatedReasoningPolicies.Get(policyARN)
	if !ok {
		return nil, fmt.Errorf("%w: automated reasoning policy %s not found", ErrNotFound, policyARN)
	}

	key := arpAnnotationsKey(policyARN, buildWorkflowID)

	anns := b.arpAnnotations[key]
	if anns == nil {
		anns = []any{}
	}

	hash, ok := b.arpAnnotationSetHash[key]
	if !ok {
		hash = newARPOpaqueHash()
		b.arpAnnotationSetHash[key] = hash
	}

	updatedAt, ok := b.arpAnnotationsUpdatedAt[key]
	if !ok {
		updatedAt = time.Now().UTC()
		b.arpAnnotationsUpdatedAt[key] = updatedAt
	}

	return map[string]any{
		"annotations":       anns,
		"annotationSetHash": hash,
		keyBuildWorkflowID:  buildWorkflowID,
		keyName:             policy.Name,
		keyPolicyArn:        policyARN,
		keyUpdatedAt:        isoTime{updatedAt},
	}, nil
}

// UpdateAutomatedReasoningPolicyAnnotations stores the caller's real annotations
// for a build workflow (bedrock@v1.66.4
// api_op_UpdateAutomatedReasoningPolicyAnnotations.go: both annotations and
// lastUpdatedAnnotationSetHash are required). lastUpdatedAnnotationSetHash is
// validated as present but not matched against the stored hash -- this
// backend does not enforce optimistic-concurrency conflicts, the same
// approach already taken for CreateAutomatedReasoningPolicyVersion's
// lastUpdatedDefinitionHash. Response fields (annotationSetHash,
// buildWorkflowId, policyArn, updatedAt) are all required on the real output.
func (b *InMemoryBackend) UpdateAutomatedReasoningPolicyAnnotations(
	policyARN, buildWorkflowID string,
	annotations []any,
	lastUpdatedAnnotationSetHash string,
) (map[string]any, error) {
	if annotations == nil {
		return nil, fmt.Errorf("%w: annotations is required", ErrValidation)
	}

	if lastUpdatedAnnotationSetHash == "" {
		return nil, fmt.Errorf("%w: lastUpdatedAnnotationSetHash is required", ErrValidation)
	}

	b.mu.Lock("UpdateAutomatedReasoningPolicyAnnotations")
	defer b.mu.Unlock()

	if err := b.mustGetARPBuildWorkflow(policyARN, buildWorkflowID); err != nil {
		return nil, err
	}

	key := arpAnnotationsKey(policyARN, buildWorkflowID)
	b.arpAnnotations[key] = annotations
	hash := newARPOpaqueHash()
	b.arpAnnotationSetHash[key] = hash
	now := time.Now().UTC()
	b.arpAnnotationsUpdatedAt[key] = now

	return map[string]any{
		"annotationSetHash": hash,
		keyBuildWorkflowID:  buildWorkflowID,
		keyPolicyArn:        policyARN,
		keyUpdatedAt:        isoTime{now},
	}, nil
}

// GetAutomatedReasoningPolicyNextScenario returns the next scenario for active-learning
// (bedrock@v1.66.4 serializers.go:4122 — real segment is "scenarios", build-workflow-scoped).
func (b *InMemoryBackend) GetAutomatedReasoningPolicyNextScenario(
	policyARN, buildWorkflowID string,
) (map[string]any, error) {
	b.mu.RLock("GetAutomatedReasoningPolicyNextScenario")
	defer b.mu.RUnlock()

	if err := b.mustGetARPBuildWorkflow(policyARN, buildWorkflowID); err != nil {
		return nil, err
	}

	return map[string]any{"scenario": nil, keyPolicyArn: policyARN}, nil
}

// GetAutomatedReasoningPolicyBuildWorkflowResultAssets returns result asset
// URLs for a workflow. Ignores the real, required AssetType filter
// (bedrock@v1.66.4 api_op_GetAutomatedReasoningPolicyBuildWorkflowResultAssets.go)
// deliberately rather than fixing it: this backend never generates
// result-asset content (build workflows here don't run a real
// document-ingestion/policy-generation pipeline), so buildWorkflowAssets is
// always omitted. Threading AssetType through to filter an always-absent
// union can't be observed by any test, real-client or otherwise
// (gopherstack-4sov). Revisit only if/when this backend starts producing
// real result-asset content.
//
// buildWorkflowAssets (types.AutomatedReasoningPolicyBuildResultAssets,
// deserializers.go's awsRestjson1_deserializeDocumentAutomatedReasoningPolicyBuildResultAssets)
// is a union object keyed by asset kind (assetManifest/buildLog/...), not a
// list -- emitting it as [] previously fed a real client's deserializer a
// JSON array where it type-switches on a JSON object, an outright decode
// failure. BuildWorkflowAssets is optional on the output, so the honest
// empty state is to omit the key entirely, matching Go's nil zero value.
func (b *InMemoryBackend) GetAutomatedReasoningPolicyBuildWorkflowResultAssets(
	policyARN, workflowID string,
) (map[string]any, error) {
	b.mu.RLock("GetAutomatedReasoningPolicyBuildWorkflowResultAssets")
	defer b.mu.RUnlock()

	wf, ok := b.arpBuildWorkflows.Get(workflowID)
	if !ok || wf.PolicyArn != policyARN {
		return nil, fmt.Errorf("%w: build workflow %s not found", ErrNotFound, workflowID)
	}

	// PolicyArn is required on the real output
	// (api_op_GetAutomatedReasoningPolicyBuildWorkflowResultAssets.go:70) and
	// was previously dropped entirely.
	return map[string]any{
		keyBuildWorkflowID: workflowID,
		keyPolicyArn:       policyARN,
	}, nil
}

// splitVersionedARN splits a versioned ARN (policyARN + "/version/" + version,
// the shape CreateAutomatedReasoningPolicyVersion builds) back into its parts.
func splitVersionedARN(versionedARN string) (string, string, bool) {
	idx := strings.LastIndex(versionedARN, "/version/")
	if idx < 0 {
		return "", "", false
	}

	return versionedARN[:idx], versionedARN[idx+len("/version/"):], true
}

// ExportAutomatedReasoningPolicyVersion exports a policy version definition. arnParam
// may be a versioned ARN or the bare policy ARN (bedrock@v1.66.4 serializers.go:3603 —
// no separate {version} path segment; the version, if any, is embedded in the ARN).
func (b *InMemoryBackend) ExportAutomatedReasoningPolicyVersion(arnParam string) (map[string]any, error) {
	b.mu.RLock("ExportAutomatedReasoningPolicyVersion")
	defer b.mu.RUnlock()

	base, version, ok := splitVersionedARN(arnParam)
	if !ok {
		if _, exists := b.automatedReasoningPolicies.Get(arnParam); !exists {
			return nil, fmt.Errorf("%w: automated reasoning policy %s not found", ErrNotFound, arnParam)
		}
		// Real AWS exports the working draft definition for a bare ARN; gopherstack
		// does not track a separate draft policy definition, so only versioned
		// exports (below) are supported.
		return nil, fmt.Errorf("%w: policy %s has no exportable version", ErrNotFound, arnParam)
	}

	v, found := b.arpVersions.Get(base + ":" + version)
	if !found {
		return nil, fmt.Errorf("%w: version %s of policy %s not found", ErrNotFound, version, base)
	}

	return map[string]any{
		keyPolicyArn:      v.PolicyArn,
		keyVersion:        v.Version,
		keyDefinitionHash: v.DefinitionHash,
		keyCreatedAt:      v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// StartAutomatedReasoningPolicyTestWorkflow starts a test workflow for a build workflow,
// optionally scoped to testCaseIDs (bedrock@v1.66.4 serializers.go:8117 —
// .../build-workflows/{id}/test-workflows, not per-test-case).
func (b *InMemoryBackend) StartAutomatedReasoningPolicyTestWorkflow(
	policyARN, buildWorkflowID string,
	testCaseIDs []string,
) (map[string]any, error) {
	b.mu.Lock("StartAutomatedReasoningPolicyTestWorkflow")
	defer b.mu.Unlock()

	if err := b.mustGetARPBuildWorkflow(policyARN, buildWorkflowID); err != nil {
		return nil, err
	}

	for _, id := range testCaseIDs {
		tc, ok := b.arpTestCases.Get(id)
		if !ok || tc.PolicyArn != policyARN {
			return nil, fmt.Errorf("%w: test case %s not found", ErrNotFound, id)
		}
	}

	return map[string]any{keyPolicyArn: policyARN}, nil
}

// arpTestResultTestRunStatus is the only AutomatedReasoningPolicyTestRunStatus
// value this backend produces: it fakes every test run as immediately
// complete (bedrock@v1.66.4 types/enums.go:352), the same simplification
// already made by GetAutomatedReasoningPolicyTestResult/
// ListAutomatedReasoningPolicyTestResults before this fix.
const arpTestResultTestRunStatus = "COMPLETED"

// arpTestResultToMap builds the real AutomatedReasoningPolicyTestResult shape
// (bedrock@v1.66.4 types.go:2055-2092: policyArn/testCase/testRunStatus/
// updatedAt required; aggregatedTestFindingsResult/testFindings/
// testRunResult omitted rather than fabricated -- this backend runs no real
// validation). updatedAt reuses the test case's own UpdatedAt: real, already
// tracked state, not a fresh fabricated timestamp.
func arpTestResultToMap(tc *AutomatedReasoningPolicyTestCase, policyARN string) map[string]any {
	return map[string]any{
		keyPolicyArn:    policyARN,
		"testCase":      arpTestCaseToMap(tc),
		"testRunStatus": arpTestResultTestRunStatus,
		keyUpdatedAt:    isoTime{tc.UpdatedAt},
	}
}

// GetAutomatedReasoningPolicyTestResult returns the result for a test case execution
// (bedrock@v1.66.4 serializers.go:4282 — build-workflow-scoped). The required
// top-level member is "testResult" (deserializers.go:
// awsRestjson1_deserializeOpDocumentGetAutomatedReasoningPolicyTestResultOutput) --
// previously returned a flat, differently-keyed object with no "testResult"
// wrapper at all, so a real client's required TestResult decoded nil.
func (b *InMemoryBackend) GetAutomatedReasoningPolicyTestResult(
	policyARN, buildWorkflowID, testCaseID string,
) (map[string]any, error) {
	b.mu.RLock("GetAutomatedReasoningPolicyTestResult")
	defer b.mu.RUnlock()

	if err := b.mustGetARPBuildWorkflow(policyARN, buildWorkflowID); err != nil {
		return nil, err
	}

	tc, ok := b.arpTestCases.Get(testCaseID)
	if !ok || tc.PolicyArn != policyARN {
		return nil, fmt.Errorf("%w: test case %s not found", ErrNotFound, testCaseID)
	}

	return map[string]any{"testResult": arpTestResultToMap(tc, policyARN)}, nil
}

// ListAutomatedReasoningPolicyTestResults returns test results for a build workflow
// (bedrock@v1.66.4 serializers.go:5937 — build-workflow-scoped). Each item is
// the same real AutomatedReasoningPolicyTestResult shape as
// GetAutomatedReasoningPolicyTestResult (types.go:2055-2092), not the flat
// shape used before this fix.
func (b *InMemoryBackend) ListAutomatedReasoningPolicyTestResults(
	policyARN, buildWorkflowID string,
) ([]map[string]any, error) {
	b.mu.RLock("ListAutomatedReasoningPolicyTestResults")
	defer b.mu.RUnlock()

	if err := b.mustGetARPBuildWorkflow(policyARN, buildWorkflowID); err != nil {
		return nil, err
	}

	type resultWithID struct {
		wire       map[string]any
		testCaseID string
	}

	var results []resultWithID
	for _, tc := range b.arpTestCases.All() {
		if tc.PolicyArn == policyARN {
			results = append(results, resultWithID{
				testCaseID: tc.TestCaseID,
				wire:       arpTestResultToMap(tc, policyARN),
			})
		}
	}

	sort.Slice(results, func(i, k int) bool {
		return results[i].testCaseID < results[k].testCaseID
	})

	wire := make([]map[string]any, 0, len(results))
	for _, r := range results {
		wire = append(wire, r.wire)
	}

	return wire, nil
}
