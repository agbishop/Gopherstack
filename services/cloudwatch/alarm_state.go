package cloudwatch

import (
	"context"
	"fmt"
	"time"
)

// alarmStateUpdate carries the state produced under lock by setAlarmStateLocked
// that SetAlarmState needs once the lock has been released to fire alarm
// actions and composite-alarm transitions.
type alarmStateUpdate struct {
	oldState               string
	alarmArn               string
	alarmDesc              string
	histAlarmType          string
	deps                   alarmActionDeps
	alarmActions           []string
	okActions              []string
	insuffActions          []string
	compositeTransitions   []compositeAlarmTransition
	stateChangeSubscribers []func(newState string)
	actionsEnabled         bool
}

// applyMetricAlarmStateLocked copies a metric alarm's pre-change fields into
// update and applies the new state. Caller must hold b.mu (write lock).
func applyMetricAlarmStateLocked(
	update *alarmStateUpdate,
	a *MetricAlarm,
	stateValue, stateReason, stateReasonData string,
	now time.Time,
) []string {
	update.oldState = a.StateValue
	update.alarmArn = a.AlarmArn
	update.alarmDesc = a.AlarmDescription
	update.alarmActions = a.AlarmActions
	update.okActions = a.OKActions
	update.insuffActions = a.InsufficientDataActions
	update.actionsEnabled = a.ActionsEnabled
	instanceIDs := instanceIDsFromDimensions(a.Dimensions)

	a.StateValue = stateValue
	a.StateReason = stateReason
	a.StateReasonData = stateReasonData
	if update.oldState != stateValue {
		a.StateTransitionedTimestamp = now
	}

	return instanceIDs
}

// applyCompositeAlarmStateLocked copies a composite alarm's pre-change fields
// into update and applies the new state. Caller must hold b.mu (write lock).
func applyCompositeAlarmStateLocked(
	update *alarmStateUpdate,
	a *CompositeAlarm,
	stateValue, stateReason string,
	now time.Time,
) {
	update.oldState = a.StateValue
	update.alarmArn = a.AlarmArn
	update.alarmDesc = a.AlarmDescription
	update.alarmActions = a.AlarmActions
	update.okActions = a.OKActions
	update.insuffActions = a.InsufficientDataActions
	update.actionsEnabled = a.ActionsEnabled

	a.StateValue = stateValue
	a.StateReason = stateReason
	if update.oldState != stateValue {
		a.StateTransitionedTimestamp = now
	}
}

// applyLogAlarmStateLocked copies a log alarm's pre-change fields into update
// and applies the new state. gopherstack has no scheduled-query engine to
// drive a log alarm's state automatically, so this is reached only via an
// explicit SetAlarmState call. Caller must hold b.mu (write lock).
func applyLogAlarmStateLocked(
	update *alarmStateUpdate,
	a *LogAlarm,
	stateValue, stateReason, stateReasonData string,
	now time.Time,
) {
	update.oldState = a.StateValue
	update.alarmArn = a.AlarmArn
	update.alarmDesc = a.AlarmDescription
	update.alarmActions = a.AlarmActions
	update.okActions = a.OKActions
	update.insuffActions = a.InsufficientDataActions
	update.actionsEnabled = a.ActionsEnabled

	a.StateValue = stateValue
	a.StateReason = stateReason
	a.StateReasonData = stateReasonData
	if update.oldState != stateValue {
		a.StateTransitionedTimestamp = now
		a.StateUpdatedTimestamp = now
	}
}

// setAlarmStateLocked applies the new state to the named metric, composite,
// or log alarm, records history, and re-evaluates dependent composite alarms.
// Must be called with b.mu unlocked; it takes the write lock itself.
// Extracted from SetAlarmState to keep the locked region out of the parent's
// funlen count.
func (b *InMemoryBackend) setAlarmStateLocked(
	alarmName, stateValue, stateReason, stateReasonData string,
) (*alarmStateUpdate, error) {
	b.mu.Lock("SetAlarmState")
	defer b.mu.Unlock()

	metricAlarm, hasMetric := b.alarms.Get(alarmName)
	compositeAlarm, hasComposite := b.compositeAlarms.Get(alarmName)
	logAlarm, hasLog := b.logAlarms.Get(alarmName)

	if !hasMetric && !hasComposite && !hasLog {
		return nil, fmt.Errorf("%w: %s", ErrAlarmNotFound, alarmName)
	}

	update := &alarmStateUpdate{}
	now := time.Now().UTC()

	var instanceIDs []string
	switch {
	case hasMetric:
		update.histAlarmType = "MetricAlarm"
		instanceIDs = applyMetricAlarmStateLocked(
			update, metricAlarm, stateValue, stateReason, stateReasonData, now,
		)
	case hasComposite:
		update.histAlarmType = "CompositeAlarm"
		applyCompositeAlarmStateLocked(update, compositeAlarm, stateValue, stateReason, now)
	default:
		update.histAlarmType = alarmTypeLogAlarm
		applyLogAlarmStateLocked(update, logAlarm, stateValue, stateReason, stateReasonData, now)
	}

	summary := fmt.Sprintf("Alarm %q changed from %s to %s", alarmName, update.oldState, stateValue)
	histData := b.stateChangeHistoryData(alarmName, update.oldState, stateValue, stateReason)
	b.appendHistory(alarmName, update.histAlarmType, historyTypeStateUpdate, summary, histData)

	// re-evaluate composite alarms that may reference this alarm, collecting any transitions
	update.compositeTransitions = b.reevaluateCompositeAlarms()

	// Alarm-state-change subscribers (e.g. FIS stop conditions) watch the alarm's
	// state itself, independent of ActionsEnabled/muting, which only gate the
	// alarm's own AlarmActions/OKActions/InsufficientDataActions below.
	if update.oldState != stateValue {
		update.stateChangeSubscribers = b.alarmStateChangeCallbacksLocked(update.alarmArn)
	}

	update.deps = alarmActionDeps{
		snsPub:      b.snsPublisher,
		lambdaInv:   b.lambdaInvoker,
		ec2:         b.ec2Actioner,
		asg:         b.asgExecutor,
		instanceIDs: instanceIDs,
	}

	return update, nil
}

// SetAlarmState manually sets the state of an alarm and fires the corresponding actions.
func (b *InMemoryBackend) SetAlarmState(
	ctx context.Context,
	alarmName, stateValue, stateReason, stateReasonData string,
) error {
	update, err := b.setAlarmStateLocked(alarmName, stateValue, stateReason, stateReasonData)
	if err != nil {
		return err
	}

	if update.actionsEnabled && stateValue != update.oldState {
		var actions []string
		switch stateValue {
		case alarmStateAlarm:
			actions = update.alarmActions
		case alarmStateOK:
			actions = update.okActions
		case alarmStateInsufficientData:
			actions = update.insuffActions
		}

		if muteRule, muted := b.activeMuteRule(alarmName, time.Now().UTC()); muted {
			b.appendMutedActionHistory(alarmName, update.histAlarmType, muteRule)
		} else {
			payload := b.buildAlarmActionPayload(
				alarmName,
				update.alarmDesc,
				update.alarmArn,
				update.oldState,
				stateValue,
				stateReason,
			)
			b.executeActions(ctx, actions, alarmName, update.histAlarmType, payload, update.deps)
		}
	}

	b.fireCompositeTransitions(ctx, update.compositeTransitions, update.deps)

	for _, cb := range update.stateChangeSubscribers {
		cb(stateValue)
	}

	return nil
}
