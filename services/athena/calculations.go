package athena

import (
	"fmt"
	"sort"
)

const (
	calcStateCreating  = "CREATING"
	calcStateCreated   = "CREATED"
	calcStateQueued    = "QUEUED"
	calcStateRunning   = "RUNNING"
	calcStateCanceling = "CANCELING"
	calcStateCompleted = "COMPLETED"
	calcStateFailed    = "FAILED"
	calcStateCanceled  = "CANCELED"
)

// isValidCalculationExecutionState reports whether state is one of the eight
// CalculationExecutionState enum values Athena defines
// (aws-sdk-go-v2/service/athena@v1.60.4 types/enums.go:22-34).
func isValidCalculationExecutionState(state string) bool {
	switch state {
	case calcStateCreating, calcStateCreated, calcStateQueued, calcStateRunning,
		calcStateCanceling, calcStateCanceled, calcStateCompleted, calcStateFailed:
		return true
	default:
		return false
	}
}

// StartCalculationExecution starts a Spark calculation in the given session.
func (b *InMemoryBackend) StartCalculationExecution(
	sessionID, description, codeBlock string,
) (string, string, error) {
	if sessionID == "" {
		return "", "", fmt.Errorf("%w: SessionId is required", ErrValidation)
	}

	b.mu.Lock("StartCalculationExecution")
	defer b.mu.Unlock()

	s, ok := b.sessions.Get(sessionID)
	if !ok {
		return "", "", fmt.Errorf("%w: session %q not found", ErrResourceNotFound, sessionID)
	}

	if s.Status.State == sessionStateTerminated {
		return "", "", fmt.Errorf("%w: session %q is terminated", ErrValidation, sessionID)
	}

	id := randomID()
	now := nowSeconds()
	b.calculations.Put(&CalculationExecution{
		CalculationID: id,
		SessionID:     sessionID,
		Description:   description,
		CodeBlock:     codeBlock,
		Status: CalculationStatus{
			State:              calcStateCompleted,
			SubmissionDateTime: now,
			CompletionDateTime: now,
		},
		Statistics: CalculationStatistics{Progress: "COMPLETED"},
		Result: CalculationResult{
			ResultType:    "JSON",
			ResultS3URI:   fmt.Sprintf("s3://athena-mock/%s/result.json", id),
			StdOutS3URI:   fmt.Sprintf("s3://athena-mock/%s/stdout.log", id),
			StdErrorS3URI: fmt.Sprintf("s3://athena-mock/%s/stderr.log", id),
		},
	})

	return id, calcStateCompleted, nil
}

// GetCalculationExecution returns a calculation execution by ID.
func (b *InMemoryBackend) GetCalculationExecution(id string) (*CalculationExecution, error) {
	b.mu.RLock("GetCalculationExecution")
	defer b.mu.RUnlock()

	c, ok := b.calculations.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: calculation execution %q not found", ErrResourceNotFound, id)
	}

	cp := *c

	return &cp, nil
}

// GetCalculationExecutionStatus returns just the status of a calculation.
func (b *InMemoryBackend) GetCalculationExecutionStatus(id string) (CalculationStatus, CalculationStatistics, error) {
	b.mu.RLock("GetCalculationExecutionStatus")
	defer b.mu.RUnlock()

	c, ok := b.calculations.Get(id)
	if !ok {
		return CalculationStatus{}, CalculationStatistics{},
			fmt.Errorf("%w: calculation execution %q not found", ErrResourceNotFound, id)
	}

	return c.Status, c.Statistics, nil
}

// GetCalculationExecutionCode returns just the code block of a calculation.
func (b *InMemoryBackend) GetCalculationExecutionCode(id string) (string, error) {
	b.mu.RLock("GetCalculationExecutionCode")
	defer b.mu.RUnlock()

	c, ok := b.calculations.Get(id)
	if !ok {
		return "", fmt.Errorf("%w: calculation execution %q not found", ErrResourceNotFound, id)
	}

	return c.CodeBlock, nil
}

// StopCalculationExecution cancels a running calculation. Per AWS's
// documented behavior, "A StopCalculationExecution call on a calculation
// that is already in a terminal state (for example, STOPPED, FAILED, or
// COMPLETED) succeeds but has no effect."
// (aws-sdk-go-v2/service/athena@v1.60.4 api_op_StopCalculationExecution.go).
func (b *InMemoryBackend) StopCalculationExecution(id string) (string, error) {
	b.mu.Lock("StopCalculationExecution")
	defer b.mu.Unlock()

	c, ok := b.calculations.Get(id)
	if !ok {
		return "", fmt.Errorf("%w: calculation execution %q not found", ErrResourceNotFound, id)
	}

	switch c.Status.State {
	case calcStateCompleted, calcStateFailed, calcStateCanceled:
		return c.Status.State, nil
	}

	c.Status.State = calcStateCanceled
	c.Status.CompletionDateTime = nowSeconds()

	return calcStateCanceled, nil
}

// ListCalculationExecutions lists calculations within a session.
func (b *InMemoryBackend) ListCalculationExecutions(sessionID, stateFilter string) ([]CalculationSummary, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: SessionId is required", ErrValidation)
	}

	if stateFilter != "" && !isValidCalculationExecutionState(stateFilter) {
		return nil, fmt.Errorf("%w: StateFilter %q is invalid", ErrValidation, stateFilter)
	}

	b.mu.RLock("ListCalculationExecutions")
	defer b.mu.RUnlock()

	if !b.sessions.Has(sessionID) {
		return nil, fmt.Errorf("%w: session %q not found", ErrResourceNotFound, sessionID)
	}

	out := make([]CalculationSummary, 0, b.calculations.Len())

	for _, c := range b.calculations.All() {
		if c.SessionID != sessionID {
			continue
		}

		if stateFilter != "" && c.Status.State != stateFilter {
			continue
		}

		out = append(out, CalculationSummary{
			CalculationID: c.CalculationID,
			Description:   c.Description,
			Status:        c.Status,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CalculationID < out[j].CalculationID })

	return out, nil
}
