package cloudwatch

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// logAlarmComparisonOperators is the restricted set of ComparisonOperator
// values PutLogAlarmInput documents as valid -- a strict subset of the seven
// values MetricAlarm accepts (no anomaly-detection band operators, since a
// log alarm compares a single aggregated query result against a scalar
// Threshold, not a metric time series against a dynamic band).
//
//nolint:gochecknoglobals // fixed enum lookup table, analogous to tableRegistrations elsewhere
var logAlarmComparisonOperators = []string{
	"GreaterThanThreshold",
	"GreaterThanOrEqualToThreshold",
	"LessThanThreshold",
	"LessThanOrEqualToThreshold",
}

const (
	cwMinQueryResultsToEvaluate = 1
	cwMaxQueryResultsToEvaluate = 100
	cwMaxActionLogLineCount     = 50
)

// validateLogAlarm checks the fields PutLogAlarmInput documents as required
// or range-constrained. gopherstack has no CloudWatch Logs query engine, so
// this stops at structural/range validation -- it does not attempt to run or
// otherwise validate the query string itself.
func validateLogAlarm(alarm *LogAlarm) error {
	if alarm.AlarmName == "" {
		return ErrAlarmNameRequired
	}
	if !slices.Contains(logAlarmComparisonOperators, alarm.ComparisonOperator) {
		return fmt.Errorf(
			"%w: ComparisonOperator must be one of %s",
			ErrValidation,
			strings.Join(logAlarmComparisonOperators, ", "),
		)
	}
	if alarm.QueryResultsToEvaluate < cwMinQueryResultsToEvaluate ||
		alarm.QueryResultsToEvaluate > cwMaxQueryResultsToEvaluate {
		return fmt.Errorf(
			"%w: QueryResultsToEvaluate must be between %d and %d",
			ErrValidation, cwMinQueryResultsToEvaluate, cwMaxQueryResultsToEvaluate,
		)
	}
	if alarm.QueryResultsToAlarm < 1 || alarm.QueryResultsToAlarm > alarm.QueryResultsToEvaluate {
		return fmt.Errorf(
			"%w: QueryResultsToAlarm (%d) must be between 1 and QueryResultsToEvaluate (%d)",
			ErrValidation, alarm.QueryResultsToAlarm, alarm.QueryResultsToEvaluate,
		)
	}
	if alarm.ActionLogLineCount < 0 || alarm.ActionLogLineCount > cwMaxActionLogLineCount {
		return fmt.Errorf(
			"%w: ActionLogLineCount must be between 0 and %d",
			ErrValidation, cwMaxActionLogLineCount,
		)
	}
	if alarm.ActionLogLineCount > 0 && alarm.ActionLogLineRoleArn == "" {
		return fmt.Errorf(
			"%w: ActionLogLineRoleArn is required when ActionLogLineCount is greater than 0",
			ErrValidation,
		)
	}

	return validateScheduledQueryConfiguration(alarm.ScheduledQueryConfiguration)
}

// validateScheduledQueryConfiguration checks the required members of the
// underlying scheduled query configuration a log alarm evaluates.
func validateScheduledQueryConfiguration(cfg ScheduledQueryConfiguration) error {
	if cfg.QueryString == "" {
		return fmt.Errorf("%w: ScheduledQueryConfiguration.QueryString is required", ErrValidation)
	}
	if cfg.AggregationExpression == "" {
		return fmt.Errorf(
			"%w: ScheduledQueryConfiguration.AggregationExpression is required",
			ErrValidation,
		)
	}
	if cfg.ScheduledQueryRoleARN == "" {
		return fmt.Errorf(
			"%w: ScheduledQueryConfiguration.ScheduledQueryRoleARN is required",
			ErrValidation,
		)
	}
	if cfg.ScheduleConfiguration.ScheduleExpression == "" {
		return fmt.Errorf(
			"%w: ScheduledQueryConfiguration.ScheduleConfiguration.ScheduleExpression is required",
			ErrValidation,
		)
	}

	return nil
}

// PutLogAlarm creates or updates a log alarm. Like PutMetricAlarm/
// PutCompositeAlarm, calling it again with the same AlarmName replaces the
// previous configuration in place (matches the SDK doc comment: "If you call
// this operation with the name of an existing log alarm, the operation
// replaces the previous configuration of that alarm").
func (b *InMemoryBackend) PutLogAlarm(alarm *LogAlarm) error {
	if err := validateLogAlarm(alarm); err != nil {
		return err
	}

	b.mu.Lock("PutLogAlarm")
	defer b.mu.Unlock()

	isNew := !b.logAlarms.Has(alarm.AlarmName)

	if alarm.AlarmArn == "" {
		alarm.AlarmArn = arn.Build("cloudwatch", b.region, b.accountID, "alarm:"+alarm.AlarmName)
	}
	if alarm.StateValue == "" {
		alarm.StateValue = alarmStateInsufficientData
	}
	now := time.Now().UTC()
	if alarm.CreatedAt.IsZero() {
		alarm.CreatedAt = now
	}
	if existing, ok := b.logAlarms.Get(alarm.AlarmName); ok {
		if existing.StateValue == alarm.StateValue {
			alarm.StateTransitionedTimestamp = existing.StateTransitionedTimestamp
			alarm.StateUpdatedTimestamp = existing.StateUpdatedTimestamp
		} else {
			alarm.StateTransitionedTimestamp = now
			alarm.StateUpdatedTimestamp = now
		}
	} else {
		alarm.StateTransitionedTimestamp = now
		alarm.StateUpdatedTimestamp = now
	}
	alarm.AlarmConfigurationUpdatedTimestamp = now

	cp := *alarm
	b.logAlarms.Put(&cp)

	histType := historyTypeConfigurationUpdate
	historySummary := fmt.Sprintf("Log alarm %q updated", alarm.AlarmName)
	if isNew {
		historySummary = fmt.Sprintf("Log alarm %q created", alarm.AlarmName)
	}
	b.appendHistory(alarm.AlarmName, alarmTypeLogAlarm, histType, historySummary, "")

	return nil
}

// collectLogAlarms returns filtered and sorted log alarms.
// Caller must hold b.mu (read lock).
func (b *InMemoryBackend) collectLogAlarms(
	nameSet map[string]bool,
	alarmNamePrefix, stateValue, actionPrefix string,
	include bool,
) []LogAlarm {
	if !include {
		return nil
	}

	var result []LogAlarm

	for _, alarm := range b.logAlarms.All() {
		if !alarmMatchesDescribeFilter(
			alarm.AlarmName, alarm.StateValue, nameSet, alarmNamePrefix, stateValue, actionPrefix,
			alarm.AlarmActions, alarm.OKActions, alarm.InsufficientDataActions,
		) {
			continue
		}
		result = append(result, *alarm)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].AlarmName < result[j].AlarmName
	})

	return result
}
