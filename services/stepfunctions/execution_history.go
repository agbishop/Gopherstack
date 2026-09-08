package stepfunctions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// historyRecorder adapts InMemoryBackend to the asl.HistoryRecorder interface.
type historyRecorder struct {
	backend *InMemoryBackend
}

// stateEnteredEventType returns the AWS event type name for the state-entered event
// for each state type.
func stateEnteredEventType(stateType string) string {
	switch stateType {
	case "Task":
		return "TaskStateEntered"
	case "Pass":
		return "PassStateEntered"
	case "Choice":
		return "ChoiceStateEntered"
	case "Wait":
		return "WaitStateEntered"
	case "Succeed":
		return "SucceedStateEntered"
	case "Fail":
		return "FailStateEntered"
	case "Parallel":
		return "ParallelStateEntered"
	case "Map":
		return "MapStateEntered"
	default:
		return stateType + "StateEntered"
	}
}

// stateExitedEventType returns the AWS event type name for the state-exited event
// for each state type.
func stateExitedEventType(stateType string) string {
	switch stateType {
	case "Task":
		return "TaskStateExited"
	case "Pass":
		return "PassStateExited"
	case "Choice":
		return "ChoiceStateExited"
	case "Wait":
		return "WaitStateExited"
	case "Succeed":
		return "SucceedStateExited"
	case "Fail":
		return "FailStateExited"
	case "Parallel":
		return "ParallelStateExited"
	case "Map":
		return "MapStateExited"
	default:
		return stateType + "StateExited"
	}
}

// appendHistory appends event to the per-execution history without the global
// write lock on the hot path. Lock order: b.mu (RLock) then b.historyMu (Lock).
// Holding b.mu.RLock for the entire call ensures b.mu write-lock holders (e.g.
// StartExecution, runParsedExecution) cannot race with concurrent map writes here.
func (b *InMemoryBackend) appendHistory(execARN string, event *HistoryEvent) {
	b.mu.RLock("appendHistory")
	defer b.mu.RUnlock()

	if b.deletedExecs[execARN] {
		return
	}

	exec, ok := b.executions.Get(execARN)
	if !ok {
		return
	}

	b.historyMu.Lock()
	defer b.historyMu.Unlock()

	if !b.checkHistoryCapacity(exec) {
		return
	}

	nextID := int64(len(exec.history) + 1)
	event.ID = nextID
	event.PreviousEventID = nextID - 1
	exec.history = append(exec.history, event)
}

func (r *historyRecorder) RecordStateEntered(execARN, stateName, stateType string, input any) {
	r.backend.appendHistory(execARN, &HistoryEvent{
		Timestamp: float64(time.Now().Unix()),
		Type:      stateEnteredEventType(stateType),
		StateEnteredEventDetails: &StateEnteredEventDetails{
			Name:  stateName,
			Input: historyValueToJSON(input),
		},
	})
}

func (r *historyRecorder) RecordStateExited(execARN, stateName, stateType string, output any) {
	r.backend.appendHistory(execARN, &HistoryEvent{
		Timestamp: float64(time.Now().Unix()),
		Type:      stateExitedEventType(stateType),
		StateExitedEventDetails: &StateExitedEventDetails{
			Name:   stateName,
			Output: historyValueToJSON(output),
		},
	})
}

func (r *historyRecorder) RecordTaskScheduled(execARN, _ /* stateName */, resource string, parameters any) {
	r.backend.appendHistory(execARN, &HistoryEvent{
		Timestamp: float64(time.Now().Unix()),
		Type:      "TaskScheduled",
		TaskScheduledEventDetails: &TaskScheduledEventDetails{
			Resource:     resource,
			ResourceType: resourceTypeFromResource(resource),
			Region:       regionFromARN(resource, r.backend.region),
			Parameters:   historyValueToJSON(parameters),
		},
	})
}

func (r *historyRecorder) RecordTaskSucceeded(execARN, _ /* stateName */, resource string, output any) {
	r.backend.appendHistory(execARN, &HistoryEvent{
		Timestamp: float64(time.Now().Unix()),
		Type:      "TaskSucceeded",
		TaskSucceededEventDetails: &TaskSucceededEventDetails{
			Resource:      resource,
			ResourceType:  resourceTypeFromResource(resource),
			Output:        historyValueToJSON(output),
			OutputDetails: &HistoryEventExecutionDataDetails{Truncated: false},
		},
	})
}

func (r *historyRecorder) RecordTaskFailed(
	execARN, _ /* stateName */, resource, errCode, cause string,
) {
	r.backend.appendHistory(execARN, &HistoryEvent{
		Timestamp: float64(time.Now().Unix()),
		Type:      "TaskFailed",
		TaskFailedEventDetails: &TaskFailedEventDetails{
			Resource:     resource,
			ResourceType: resourceTypeFromResource(resource),
			Error:        errCode,
			Cause:        cause,
		},
	})
}

// resourceTypeFromResource derives AWS's TaskScheduled/TaskSucceeded/TaskFailed
// ResourceType from a Task state's Resource ARN: for a direct-service ARN
// (e.g. "arn:aws:lambda:...:function:fn") it's that ARN's own service; for a
// States service-integration ARN ("arn:aws:states:::sqs:sendMessage", optionally
// "arn:aws:states:::aws-sdk:sqs:sendMessage") it's the integration's service
// name; for an activity ARN it's "activity".
func resourceTypeFromResource(resource string) string {
	parts := strings.Split(resource, ":")
	if len(parts) < 6 || parts[0] != "arn" {
		return ""
	}

	service := parts[2]
	if service != "states" {
		return service
	}

	if parts[5] == "activity" {
		return "activity"
	}

	if parts[5] == "aws-sdk" && len(parts) > 6 {
		return parts[6]
	}

	return parts[5]
}

// historyValueToJSON marshals a state input/output value to its JSON string
// representation for execution-history detail fields, matching AWS's
// convention of embedding input/output as a JSON string rather than a nested
// object. Marshal failures (which should not occur for values that already
// round-tripped through the ASL interpreter) fall back to an empty string
// rather than corrupting the history event.
func historyValueToJSON(v any) string {
	if v == nil {
		return ""
	}

	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}

	return string(b)
}

// checkHistoryCapacity reports whether there is room to append another event
// to exec.history. On the first refusal per execution it logs a warning so
// that silent truncation is observable. Caller must hold b.historyMu write lock.
func (b *InMemoryBackend) checkHistoryCapacity(exec *Execution) bool {
	if len(exec.history) < maxHistoryEvents {
		return true
	}

	if !b.historyTruncated[exec.ExecutionArn] {
		b.historyTruncated[exec.ExecutionArn] = true
		logger.Load(b.svcCtx).Warn(
			"stepfunctions: execution history truncated at maxHistoryEvents",
			"executionArn", exec.ExecutionArn,
			"maxHistoryEvents", maxHistoryEvents,
		)
	}

	return false
}

// GetExecutionHistory returns history events for an execution.
func (b *InMemoryBackend) GetExecutionHistory(
	executionArn, nextToken string, maxResults int, reverseOrder bool,
) ([]HistoryEvent, string, error) {
	b.mu.RLock("GetExecutionHistory")
	defer b.mu.RUnlock()

	exec, exists := b.executions.Get(executionArn)
	if !exists {
		return nil, "", fmt.Errorf("%w: %s", ErrExecutionDoesNotExist, executionArn)
	}

	b.historyMu.RLock()
	defer b.historyMu.RUnlock()

	raw := exec.history
	all := make([]HistoryEvent, 0, len(raw))
	for _, e := range raw {
		all = append(all, *e)
	}

	if reverseOrder {
		sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	}

	events, token := paginate(all, nextToken, maxResults)

	return events, token, nil
}
