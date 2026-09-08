package cloudformation

import (
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"
)

// pruneDriftDetections removes all drift detection entries associated with a stack
// using the reverse index for O(1) lookup instead of O(n) scan.
func (b *InMemoryBackend) pruneDriftDetections(stackID string) {
	for _, detectionID := range b.driftByStackID[stackID] {
		b.driftDetections.Delete(detectionID)
	}
	delete(b.driftByStackID, stackID)
	delete(b.resourceDriftStatus, stackID)
	delete(b.resourceDriftDetail, stackID)
}

const (
	driftStatusInSync  = "IN_SYNC"
	detectionComplete  = "DETECTION_COMPLETE"
	cfnEstimateCostURL = "https://calculator.s3.amazonaws.com/calc5.html?key=mock-estimate"
)

// DetectStackDrift initiates drift detection for all resources in a stack.
// It compares deployed resource state against the current template and returns
// DRIFTED/MODIFIED/DELETED when divergence is found (#12).
func (b *InMemoryBackend) DetectStackDrift(nameOrID string) (string, error) {
	b.mu.Lock("DetectStackDrift")
	defer b.mu.Unlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return "", ErrStackNotFound
	}

	resourceStatuses := b.compareStackResources(stack)

	driftedCount := 0
	overallStatus := driftStatusInSync
	for _, status := range resourceStatuses {
		if status != driftStatusInSync {
			driftedCount++
			overallStatus = driftStatusDrifted
		}
	}

	b.resourceDriftStatus[stack.StackID] = resourceStatuses

	detectionID := uuid.New().String()
	b.driftDetections.Put(&DriftDetectionStatus{
		StackID:                   stack.StackID,
		StackDriftDetectionID:     detectionID,
		StackDriftStatus:          overallStatus,
		DetectionStatus:           detectionComplete,
		DriftedStackResourceCount: driftedCount,
		Timestamp:                 time.Now(),
	})
	b.driftByStackID[stack.StackID] = append(b.driftByStackID[stack.StackID], detectionID)

	return detectionID, nil
}

// DetectStackResourceDrift synchronously detects drift for a single resource in a
// stack and returns its full StackResourceDrift record. Unlike DetectStackDrift,
// this operation has no asynchronous detection-ID phase: AWS returns the drift
// record directly (api_op_DetectStackResourceDrift.go, aws-sdk-go-v2 v1.76.1).
func (b *InMemoryBackend) DetectStackResourceDrift(nameOrID, logicalID string) (*StackResourceDrift, error) {
	b.mu.Lock("DetectStackResourceDrift")
	defer b.mu.Unlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return nil, ErrStackNotFound
	}

	deployedRes, exists := b.resources[stack.StackID][logicalID]
	if !exists {
		return nil, ErrResourceNotFound
	}

	b.compareStackResources(stack)

	detail, ok2 := b.resourceDriftDetail[stack.StackID][logicalID]
	if !ok2 {
		// Template failed to parse: fall back to reporting the live state as
		// in-sync rather than fabricating a diff we can't compute.
		detail = driftDetailFor(stack, deployedRes, nil, driftStatusInSync, nil)
		if b.resourceDriftDetail[stack.StackID] == nil {
			b.resourceDriftDetail[stack.StackID] = make(map[string]StackResourceDrift)
		}
		b.resourceDriftDetail[stack.StackID][logicalID] = detail
	}

	if b.resourceDriftStatus[stack.StackID] == nil {
		b.resourceDriftStatus[stack.StackID] = make(map[string]string)
	}
	b.resourceDriftStatus[stack.StackID][logicalID] = detail.StackResourceDriftStatus

	return &detail, nil
}

// RecordResourceMutation records an out-of-band change to a deployed resource's
// live configuration. This models a resource whose actual state was modified
// outside CloudFormation (for example a direct call to the underlying service).
// After this call, DetectStackDrift reports the resource as MODIFIED with the
// precise property differences between the template (expected) and the recorded
// live state (actual).
func (b *InMemoryBackend) RecordResourceMutation(
	nameOrID, logicalID string,
	liveProps map[string]any,
) error {
	b.mu.Lock("RecordResourceMutation")
	defer b.mu.Unlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return ErrStackNotFound
	}
	res, exists := b.resources[stack.StackID][logicalID]
	if !exists {
		return ErrResourceNotFound
	}
	res.Properties = deepCopyProps(liveProps)

	return nil
}

// compareStackResources compares each resource's live (recorded) backend state
// against the properties declared in the stack template, detecting out-of-band
// mutations (MODIFIED) and resources removed from the template (DELETED). It
// populates per-resource drift detail (property-level differences plus the
// expected/actual property JSON) and returns a map of logicalID → drift status.
// Must be called with b.mu held.
func (b *InMemoryBackend) compareStackResources(stack *Stack) map[string]string {
	statuses := make(map[string]string)
	details := make(map[string]StackResourceDrift)

	deployedResources := b.resources[stack.StackID]
	if len(deployedResources) == 0 {
		return statuses
	}

	tmpl, err := ParseTemplate(stack.TemplateBody)
	if err != nil {
		for logicalID := range deployedResources {
			statuses[logicalID] = driftStatusInSync
		}

		return statuses
	}

	for logicalID, deployedRes := range deployedResources {
		tmplRes, inTemplate := tmpl.Resources[logicalID]
		if !inTemplate {
			statuses[logicalID] = driftStatusDeleted
			details[logicalID] = driftDetailFor(stack, deployedRes, nil, driftStatusDeleted, nil)

			continue
		}

		// Expected = template properties; Actual = live recorded properties.
		expected := tmplRes.Properties
		actual := deployedRes.Properties
		diffs := computePropertyDifferences(expected, actual)

		status := driftStatusInSync
		if len(diffs) > 0 {
			status = driftStatusModified
		}
		statuses[logicalID] = status
		details[logicalID] = driftDetailFor(stack, deployedRes, expected, status, diffs)
	}

	if b.resourceDriftDetail[stack.StackID] == nil {
		b.resourceDriftDetail[stack.StackID] = make(map[string]StackResourceDrift)
	}
	maps.Copy(b.resourceDriftDetail[stack.StackID], details)

	return statuses
}

// driftDetailFor assembles the StackResourceDrift record for a resource,
// including the property-level differences and the expected/actual property JSON.
func driftDetailFor(
	stack *Stack,
	deployedRes *StackResource,
	expected map[string]any,
	status string,
	diffs []PropertyDifference,
) StackResourceDrift {
	d := StackResourceDrift{
		StackID:                  stack.StackID,
		LogicalResourceID:        deployedRes.LogicalID,
		PhysicalResourceID:       deployedRes.PhysicalID,
		ResourceType:             deployedRes.Type,
		StackResourceDriftStatus: status,
		Timestamp:                time.Now(),
		PropertyDifferences:      diffs,
	}
	if expected != nil {
		d.ExpectedProperties = toJSONString(expected)
	}
	if status != driftStatusDeleted {
		d.ActualProperties = toJSONString(deployedRes.Properties)
	}

	return d
}

// deepCopyProps returns a JSON round-trip deep copy of a property map so a
// recorded live state cannot alias the template properties.
func deepCopyProps(props map[string]any) map[string]any {
	if props == nil {
		return nil
	}
	data, err := json.Marshal(props)
	if err != nil {
		return props
	}
	var out map[string]any
	if uerr := json.Unmarshal(data, &out); uerr != nil {
		return props
	}

	return out
}

// DescribeStackDriftDetectionStatus returns the status of a drift detection operation.
func (b *InMemoryBackend) DescribeStackDriftDetectionStatus(detectionID string) (*DriftDetectionStatus, error) {
	b.mu.RLock("DescribeStackDriftDetectionStatus")
	defer b.mu.RUnlock()

	status, ok := b.driftDetections.Get(detectionID)
	if !ok {
		return nil, ErrDriftDetectionNotFound
	}

	return status, nil
}

// SimulateDrift marks all resources of a stack as DRIFTED for testing purposes.
// This is a test-only helper not part of the StorageBackend interface.
func (b *InMemoryBackend) SimulateDrift(stackName string) error {
	b.mu.Lock("SimulateDrift")
	defer b.mu.Unlock()

	stack, ok := b.resolveStack(stackName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrStackNotFound, stackName)
	}

	resMap := b.resources[stack.StackID]
	if len(resMap) == 0 {
		return nil
	}

	// Create a drift detection record that reflects drifted resources.
	detectionID := uuid.New().String()
	b.driftDetections.Put(&DriftDetectionStatus{
		StackID:                   stack.StackID,
		StackDriftDetectionID:     detectionID,
		StackDriftStatus:          driftStatusDrifted,
		DetectionStatus:           "DETECTION_COMPLETE",
		DriftedStackResourceCount: len(resMap),
		Timestamp:                 time.Now(),
	})

	return nil
}

// DescribeStackResourceDrifts returns drift information for all resources in a stack.
// Uses per-resource drift statuses from DetectStackDrift when available;
// falls back to the legacy SimulateDrift DRIFTED flag for backward compatibility.
func (b *InMemoryBackend) DescribeStackResourceDrifts(nameOrID string) ([]StackResourceDrift, error) {
	b.mu.RLock("DescribeStackResourceDrifts")
	defer b.mu.RUnlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return nil, ErrStackNotFound
	}

	// Use per-resource drift statuses if populated by DetectStackDrift.
	perResourceStatuses := b.resourceDriftStatus[stack.StackID]

	// Legacy SimulateDrift fallback: if any detection has DRIFTED status and
	// no per-resource data exists, mark all as DRIFTED.
	legacyDrifted := false
	if len(perResourceStatuses) == 0 {
		for _, det := range b.driftDetections.All() {
			if det.StackID == stack.StackID && det.StackDriftStatus == driftStatusDrifted {
				legacyDrifted = true

				break
			}
		}
	}

	resMap := b.resources[stack.StackID]
	detailMap := b.resourceDriftDetail[stack.StackID]
	drifts := make([]StackResourceDrift, 0, len(resMap))
	for _, res := range resMap {
		// Prefer the full drift detail captured by DetectStackDrift, which
		// carries property-level differences and expected/actual state.
		if detailMap != nil {
			if d, ok2 := detailMap[res.LogicalID]; ok2 {
				drifts = append(drifts, d)

				continue
			}
		}

		status := driftStatusInSync
		if len(perResourceStatuses) > 0 {
			if s, ok2 := perResourceStatuses[res.LogicalID]; ok2 {
				status = s
			}
		} else if legacyDrifted {
			status = driftStatusDrifted
		}
		drifts = append(drifts, StackResourceDrift{
			StackID:                  stack.StackID,
			LogicalResourceID:        res.LogicalID,
			PhysicalResourceID:       res.PhysicalID,
			ResourceType:             res.Type,
			StackResourceDriftStatus: status,
			Timestamp:                res.Timestamp,
		})
	}

	sortResourceDrifts(drifts)

	return drifts, nil
}

func sortResourceDrifts(drifts []StackResourceDrift) {
	n := len(drifts)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && drifts[j].LogicalResourceID < drifts[j-1].LogicalResourceID; j-- {
			drifts[j], drifts[j-1] = drifts[j-1], drifts[j]
		}
	}
}
