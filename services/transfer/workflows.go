package transfer

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// isValidWorkflowStepType reports whether t is a step Type accepted by real AWS Transfer.
func isValidWorkflowStepType(t string) bool {
	switch t {
	case "COPY", "CUSTOM", "DELETE", "TAG", "DECRYPT":
		return true
	}

	return false
}

// validateWorkflowSteps returns an error if any step has an unrecognised Type.
func validateWorkflowSteps(steps []WorkflowStep) error {
	for i, s := range steps {
		if !isValidWorkflowStepType(s.Type) {
			return fmt.Errorf(
				"%w: step %d has invalid Type %q; must be one of COPY, CUSTOM, DELETE, TAG, DECRYPT",
				ErrValidation, i, s.Type,
			)
		}
	}

	return nil
}

// CreateWorkflow creates a Transfer workflow.
func (b *InMemoryBackend) CreateWorkflow(
	description string,
	steps []WorkflowStep,
	onExceptionSteps []WorkflowStep,
	tags map[string]string,
) (*Workflow, error) {
	if err := validateWorkflowSteps(steps); err != nil {
		return nil, err
	}

	if err := validateWorkflowSteps(onExceptionSteps); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateWorkflow")
	defer b.mu.Unlock()

	workflowID := "w-" + uuid.NewString()[:20]

	merged := make(map[string]string, len(tags))
	maps.Copy(merged, tags)

	wf := &Workflow{
		WorkflowID:       workflowID,
		Description:      description,
		Steps:            cloneWorkflowSteps(steps),
		OnExceptionSteps: cloneWorkflowSteps(onExceptionSteps),
		CreatedAt:        time.Now(),
		Tags:             merged,
		AccountID:        b.accountID,
		Region:           b.region,
	}
	b.workflows.Put(wf)
	b.initTagsStore(arn.Build("transfer", b.region, b.accountID, "workflow/"+workflowID), merged)

	return cloneWorkflow(wf), nil
}

// DeleteWorkflow removes a workflow by ID.
func (b *InMemoryBackend) DeleteWorkflow(workflowID string) error {
	b.mu.Lock("DeleteWorkflow")
	defer b.mu.Unlock()

	if !b.workflows.Has(workflowID) {
		return fmt.Errorf("%w: workflow %s not found", ErrWorkflowNotFound, workflowID)
	}

	b.workflows.Delete(workflowID)
	delete(b.tagsStore, workflowARN(b.accountID, b.region, workflowID))

	return nil
}

// DescribeWorkflow returns a workflow by ID.
func (b *InMemoryBackend) DescribeWorkflow(workflowID string) (*Workflow, error) {
	b.mu.RLock("DescribeWorkflow")
	defer b.mu.RUnlock()

	wf, ok := b.workflows.Get(workflowID)
	if !ok {
		return nil, fmt.Errorf("%w: workflow %s not found", ErrWorkflowNotFound, workflowID)
	}

	return cloneWorkflow(wf), nil
}

// ListWorkflows returns all workflows sorted by workflowID.
func (b *InMemoryBackend) ListWorkflows() []*Workflow {
	b.mu.RLock("ListWorkflows")
	defer b.mu.RUnlock()

	all := b.workflows.All()
	out := make([]*Workflow, 0, len(all))

	for _, wf := range all {
		out = append(out, cloneWorkflow(wf))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].WorkflowID < out[j].WorkflowID
	})

	return out
}

// SendWorkflowStepStateRecord advances an execution by matching a token.
// Status must be "SUCCESS" or "FAILURE" per AWS API contract
// (types.CustomStepStatus).
func (b *InMemoryBackend) SendWorkflowStepStateRecord(workflowID, executionID, token, status string) error {
	// Validate status before acquiring the lock.
	switch status {
	case workflowStepStatusSuccess, workflowStepStatusFailure:
		// valid
	default:
		return fmt.Errorf(
			"%w: Status must be SUCCESS or FAILURE, got %q",
			ErrValidation,
			status,
		)
	}

	b.mu.Lock("SendWorkflowStepState")
	defer b.mu.Unlock()

	if len(b.executionsByWorkflow.Get(workflowID)) == 0 {
		return fmt.Errorf("%w: workflow %s not found", ErrWorkflowNotFound, workflowID)
	}

	e, ok := b.executions.Get(executionKey(workflowID, executionID))
	if !ok {
		return fmt.Errorf("%w: execution %s not found", ErrWorkflowNotFound, executionID)
	}

	if e.PendingTokens == nil {
		e.PendingTokens = make(map[string]string)
	}

	// Mark token as processed.
	e.PendingTokens[token] = status

	switch status {
	case workflowStepStatusSuccess:
		e.Status = "COMPLETED"
	case workflowStepStatusFailure:
		e.Status = "EXCEPTION"
	}

	return nil
}

// AddWorkflowInternal seeds a workflow for testing purposes.
func (b *InMemoryBackend) AddWorkflowInternal(workflowID string) {
	b.mu.Lock("AddWorkflowInternal")
	defer b.mu.Unlock()

	b.workflows.Put(&Workflow{
		WorkflowID: workflowID,
		CreatedAt:  time.Now(),
		Tags:       make(map[string]string),
		AccountID:  b.accountID,
		Region:     b.region,
	})
}

// CreateExecution creates a new execution for a workflow.
func (b *InMemoryBackend) CreateExecution(workflowID string) (*Execution, error) {
	b.mu.Lock("CreateExecution")
	defer b.mu.Unlock()

	if !b.workflows.Has(workflowID) {
		return nil, fmt.Errorf("%w: workflow %s not found", ErrWorkflowNotFound, workflowID)
	}

	executionID := "exec-" + uuid.NewString()[:8]

	e := &Execution{
		ExecutionID: executionID,
		WorkflowID:  workflowID,
		Status:      "IN_PROGRESS",
		CreatedAt:   time.Now(),
	}
	b.executions.Put(e)

	cp := *e

	return &cp, nil
}

// DescribeExecution returns an execution for a workflow.
func (b *InMemoryBackend) DescribeExecution(workflowID, executionID string) (*Execution, error) {
	b.mu.RLock("DescribeExecution")
	defer b.mu.RUnlock()

	e, ok := b.executions.Get(executionKey(workflowID, executionID))
	if !ok {
		return nil, fmt.Errorf(
			"%w: execution %s not found for workflow %s",
			ErrWorkflowNotFound,
			executionID,
			workflowID,
		)
	}

	cp := *e

	return &cp, nil
}

// ListExecutions returns all executions for a workflow sorted by executionID.
func (b *InMemoryBackend) ListExecutions(workflowID string) ([]*Execution, error) {
	b.mu.RLock("ListExecutions")
	defer b.mu.RUnlock()

	if !b.workflows.Has(workflowID) {
		return nil, fmt.Errorf("%w: workflow %s not found", ErrWorkflowNotFound, workflowID)
	}

	workflowExecs := b.executionsByWorkflow.Get(workflowID)
	out := make([]*Execution, 0, len(workflowExecs))

	for _, e := range workflowExecs {
		cp := *e
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ExecutionID < out[j].ExecutionID
	})

	return out, nil
}
