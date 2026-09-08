package cloudwatch

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// PutMetricAlarm creates or updates an alarm.
func (b *InMemoryBackend) PutMetricAlarm(alarm *MetricAlarm) error {
	if alarm.AlarmName == "" {
		return ErrAlarmNameRequired
	}

	// AWS validation: Statistic and ExtendedStatistic are mutually exclusive.
	if alarm.Statistic != "" && alarm.ExtendedStatistic != "" {
		return fmt.Errorf(
			"%w: Statistic and ExtendedStatistic are mutually exclusive",
			ErrValidation,
		)
	}

	// AWS validation: DatapointsToAlarm must not exceed EvaluationPeriods.
	if alarm.DatapointsToAlarm > 0 && alarm.DatapointsToAlarm > alarm.EvaluationPeriods {
		return fmt.Errorf(
			"%w: DatapointsToAlarm (%d) must not exceed EvaluationPeriods (%d)",
			ErrValidation,
			alarm.DatapointsToAlarm,
			alarm.EvaluationPeriods,
		)
	}

	b.mu.Lock("PutMetricAlarm")
	defer b.mu.Unlock()

	isNew := !b.alarms.Has(alarm.AlarmName)

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
	// Preserve the state-transitioned timestamp from an existing alarm if the state did not change.
	if existing, ok := b.alarms.Get(alarm.AlarmName); ok {
		if existing.StateValue == alarm.StateValue {
			alarm.StateTransitionedTimestamp = existing.StateTransitionedTimestamp
		} else {
			alarm.StateTransitionedTimestamp = now
		}
	} else {
		alarm.StateTransitionedTimestamp = now
	}
	alarm.AlarmConfigurationUpdatedTimestamp = now

	cp := *alarm
	b.alarms.Put(&cp)

	histType := historyTypeConfigurationUpdate
	historySummary := fmt.Sprintf("Alarm %q updated", alarm.AlarmName)
	if isNew {
		historySummary = fmt.Sprintf("Alarm %q created", alarm.AlarmName)
	}
	b.appendHistory(alarm.AlarmName, "MetricAlarm", histType, historySummary, "")

	return nil
}

// DescribeAlarms lists a page of alarms, optionally filtered by name, type, prefix,
// state, action prefix, or alarm-family relationship (children/parents of a
// composite alarm's rule).
// alarmTypes can contain "MetricAlarm", "CompositeAlarm", and/or "LogAlarm".
// Per the real DescribeAlarmsInput.AlarmTypes doc comment ("If you omit this
// parameter, only metric alarms are returned, even if composite alarms or
// log alarms exist in the account" -- confirmed against
// aws-sdk-go-v2/service/cloudwatch@v1.66.3/api_op_DescribeAlarms.go), omitting
// alarmTypes returns ONLY metric alarms: composite and log alarms are both
// included only when explicitly requested via alarmTypes (bd gopherstack-yvb7
// -- composite alarms were previously defaulted-in alongside metric alarms,
// which the LogAlarm family correctly never did).
// MaxRecords applies to the total combined result set (metric + composite + log).
func (b *InMemoryBackend) DescribeAlarms(
	alarmNames []string,
	alarmTypes []string,
	alarmNamePrefix, stateValue, nextToken string,
	maxRecords int,
	actionPrefix, childrenOfAlarmName, parentsOfAlarmName string,
) (page.Page[MetricAlarm], page.Page[CompositeAlarm], page.Page[LogAlarm], error) {
	if err := validateAlarmFamilyFilter(
		alarmNames, alarmTypes, alarmNamePrefix, actionPrefix, stateValue,
		childrenOfAlarmName, parentsOfAlarmName,
	); err != nil {
		return page.Page[MetricAlarm]{}, page.Page[CompositeAlarm]{}, page.Page[LogAlarm]{}, err
	}

	b.mu.RLock("DescribeAlarms")
	defer b.mu.RUnlock()

	var metricResult []MetricAlarm
	var compositeResult []CompositeAlarm
	var logResult []LogAlarm

	switch {
	case childrenOfAlarmName != "":
		metricResult, compositeResult = b.collectChildrenOfAlarm(childrenOfAlarmName)
	case parentsOfAlarmName != "":
		compositeResult = b.collectParentsOfAlarm(parentsOfAlarmName)
	default:
		nameSet := toSet(alarmNames)
		typeSet := toSet(alarmTypes)
		includeMetric := len(typeSet) == 0 || typeSet["MetricAlarm"]
		includeComposite := typeSet["CompositeAlarm"]
		includeLog := typeSet[alarmTypeLogAlarm]

		metricResult = b.collectMetricAlarms(nameSet, alarmNamePrefix, stateValue, actionPrefix, includeMetric)
		compositeResult = b.collectCompositeAlarms(
			nameSet,
			alarmNamePrefix,
			stateValue,
			actionPrefix,
			includeComposite,
		)
		logResult = b.collectLogAlarms(nameSet, alarmNamePrefix, stateValue, actionPrefix, includeLog)
	}

	// Apply a single combined page limit so MaxRecords constrains the total result set.
	limit := maxRecords
	if limit <= 0 {
		limit = cwDefaultDescribeAlarmsLimit
	}
	metricSlice, compositeSlice, logSlice, next := paginateAlarmResults(
		metricResult, compositeResult, logResult, nextToken, limit,
	)

	return page.Page[MetricAlarm]{Data: metricSlice, Next: next},
		page.Page[CompositeAlarm]{Data: compositeSlice, Next: next},
		page.Page[LogAlarm]{Data: logSlice, Next: next},
		nil
}

// validateAlarmFamilyFilter enforces DescribeAlarmsInput's documented
// restriction on ChildrenOfAlarmName and ParentsOfAlarmName: per the doc
// comment on both fields (aws-sdk-go-v2/service/cloudwatch@v1.66.3/
// api_op_DescribeAlarms.go), specifying either one with any parameter other
// than MaxRecords/NextToken (which this backend does not see here) is a
// validation error, and the two are mutually exclusive with each other.
func validateAlarmFamilyFilter(
	alarmNames, alarmTypes []string,
	alarmNamePrefix, actionPrefix, stateValue, childrenOfAlarmName, parentsOfAlarmName string,
) error {
	if childrenOfAlarmName == "" && parentsOfAlarmName == "" {
		return nil
	}
	if childrenOfAlarmName != "" && parentsOfAlarmName != "" {
		return ErrAlarmFamilyFilterExclusive
	}
	if len(alarmNames) > 0 || len(alarmTypes) > 0 || alarmNamePrefix != "" ||
		actionPrefix != "" || stateValue != "" {
		return ErrAlarmFamilyFilterExclusive
	}

	return nil
}

// collectChildrenOfAlarm returns the metric and composite alarms referenced
// by the named composite alarm's AlarmRule (its "children"), abbreviated per
// the ChildrenOfAlarmName doc comment: only Name, ARN, StateValue, and
// StateUpdatedTimestamp are returned. This model does not track
// StateUpdatedTimestamp separately from StateTransitionedTimestamp for
// MetricAlarm/CompositeAlarm, so StateTransitionedTimestamp stands in for it.
// Caller must hold b.mu (read lock).
func (b *InMemoryBackend) collectChildrenOfAlarm(name string) ([]MetricAlarm, []CompositeAlarm) {
	parent, isComposite := b.compositeAlarms.Get(name)
	if !isComposite {
		return nil, nil
	}

	seen := make(map[string]bool)
	var metrics []MetricAlarm
	var composites []CompositeAlarm

	for _, ref := range extractAlarmRuleRefs(parent.AlarmRule) {
		if ref.Name == name || seen[ref.Name] {
			continue
		}
		seen[ref.Name] = true

		if m, found := b.alarms.Get(ref.Name); found {
			metrics = append(metrics, abbreviateMetricAlarmForFamily(*m))
		}
		if c, found := b.compositeAlarms.Get(ref.Name); found {
			composites = append(composites, abbreviateCompositeAlarmForFamily(*c))
		}
	}

	sort.Slice(metrics, func(i, j int) bool { return metrics[i].AlarmName < metrics[j].AlarmName })
	sort.Slice(composites, func(i, j int) bool { return composites[i].AlarmName < composites[j].AlarmName })

	return metrics, composites
}

// collectParentsOfAlarm returns the composite alarms whose AlarmRule
// references the named alarm (its "parents"), abbreviated per the
// ParentsOfAlarmName doc comment: only Name and ARN are returned.
// Caller must hold b.mu (read lock).
func (b *InMemoryBackend) collectParentsOfAlarm(name string) []CompositeAlarm {
	var composites []CompositeAlarm

	for _, ca := range b.compositeAlarms.All() {
		if ca.AlarmName == name {
			continue
		}
		for _, ref := range extractAlarmRuleRefs(ca.AlarmRule) {
			if ref.Name == name {
				composites = append(composites, CompositeAlarm{
					AlarmName: ca.AlarmName,
					AlarmArn:  ca.AlarmArn,
				})

				break
			}
		}
	}

	sort.Slice(composites, func(i, j int) bool { return composites[i].AlarmName < composites[j].AlarmName })

	return composites
}

// abbreviateMetricAlarmForFamily returns the ChildrenOfAlarmName field subset
// of a MetricAlarm: Name, ARN, StateValue, and StateUpdatedTimestamp.
func abbreviateMetricAlarmForFamily(a MetricAlarm) MetricAlarm {
	return MetricAlarm{
		AlarmName:                  a.AlarmName,
		AlarmArn:                   a.AlarmArn,
		StateValue:                 a.StateValue,
		StateTransitionedTimestamp: a.StateTransitionedTimestamp,
	}
}

// abbreviateCompositeAlarmForFamily returns the ChildrenOfAlarmName field
// subset of a CompositeAlarm: Name, ARN, StateValue, and StateUpdatedTimestamp.
func abbreviateCompositeAlarmForFamily(a CompositeAlarm) CompositeAlarm {
	return CompositeAlarm{
		AlarmName:                  a.AlarmName,
		AlarmArn:                   a.AlarmArn,
		StateValue:                 a.StateValue,
		StateTransitionedTimestamp: a.StateTransitionedTimestamp,
	}
}

// actionListsHavePrefix reports whether any action ARN across the given
// action lists starts with prefix. ActionPrefix's doc comment ("filter the
// results ... to only those alarms that use a certain alarm action")
// does not name a specific action list, so this checks AlarmActions,
// OKActions, and InsufficientDataActions alike.
func actionListsHavePrefix(prefix string, lists ...[]string) bool {
	for _, list := range lists {
		for _, action := range list {
			if strings.HasPrefix(action, prefix) {
				return true
			}
		}
	}

	return false
}

// alarmMatchesDescribeFilter applies DescribeAlarms' name-set, name-prefix,
// state, and action-prefix filters shared by metric, composite, and log
// alarms. actionLists is that alarm's AlarmActions/OKActions/
// InsufficientDataActions, in that order.
func alarmMatchesDescribeFilter(
	alarmName, alarmState string,
	nameSet map[string]bool,
	alarmNamePrefix, stateValue, actionPrefix string,
	actionLists ...[]string,
) bool {
	if len(nameSet) > 0 && !nameSet[alarmName] {
		return false
	}
	if alarmNamePrefix != "" && !strings.HasPrefix(alarmName, alarmNamePrefix) {
		return false
	}
	if stateValue != "" && alarmState != stateValue {
		return false
	}
	if actionPrefix != "" && !actionListsHavePrefix(actionPrefix, actionLists...) {
		return false
	}

	return true
}

// paginateAlarmResults applies a single combined page window across the three
// alarm-type result lists (already filtered and sorted by the caller) so that
// MaxRecords/NextToken constrain and page through the total combined result
// set the same way real DescribeAlarms pagination does.
func paginateAlarmResults(
	metricResult []MetricAlarm,
	compositeResult []CompositeAlarm,
	logResult []LogAlarm,
	nextToken string,
	limit int,
) ([]MetricAlarm, []CompositeAlarm, []LogAlarm, string) {
	combinedTotal := len(metricResult) + len(compositeResult) + len(logResult)
	start := min(page.DecodeToken(nextToken), combinedTotal)
	end := start + limit
	var next string
	if end < combinedTotal {
		next = page.EncodeToken(end)
	} else {
		end = combinedTotal
	}

	var metricSlice []MetricAlarm
	var compositeSlice []CompositeAlarm
	var logSlice []LogAlarm
	for i := start; i < end; i++ {
		switch {
		case i < len(metricResult):
			metricSlice = append(metricSlice, metricResult[i])
		case i < len(metricResult)+len(compositeResult):
			compositeSlice = append(compositeSlice, compositeResult[i-len(metricResult)])
		default:
			logSlice = append(logSlice, logResult[i-len(metricResult)-len(compositeResult)])
		}
	}
	if metricSlice == nil {
		metricSlice = []MetricAlarm{}
	}
	if compositeSlice == nil {
		compositeSlice = []CompositeAlarm{}
	}
	if logSlice == nil {
		logSlice = []LogAlarm{}
	}

	return metricSlice, compositeSlice, logSlice, next
}

// toSet converts a string slice to a set (map[string]bool).
func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}

	return m
}

// collectMetricAlarms returns filtered and sorted metric alarms.
// Caller must hold b.mu (read lock).
func (b *InMemoryBackend) collectMetricAlarms(
	nameSet map[string]bool,
	alarmNamePrefix, stateValue, actionPrefix string,
	include bool,
) []MetricAlarm {
	if !include {
		return nil
	}

	var result []MetricAlarm

	for _, alarm := range b.alarms.All() {
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

// collectCompositeAlarms returns filtered and sorted composite alarms.
// Caller must hold b.mu (read lock).
func (b *InMemoryBackend) collectCompositeAlarms(
	nameSet map[string]bool,
	alarmNamePrefix, stateValue, actionPrefix string,
	include bool,
) []CompositeAlarm {
	if !include {
		return nil
	}

	var result []CompositeAlarm

	for _, alarm := range b.compositeAlarms.All() {
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

// DescribeAlarmsForMetric returns metric alarms associated with a specific metric.
func (b *InMemoryBackend) DescribeAlarmsForMetric(
	namespace, metricName string,
	dimensions []Dimension,
	alarmNames []string,
	nextToken string,
	maxRecords int,
) (page.Page[MetricAlarm], error) {
	b.mu.RLock("DescribeAlarmsForMetric")
	defer b.mu.RUnlock()

	nameSet := make(map[string]bool, len(alarmNames))
	for _, n := range alarmNames {
		nameSet[n] = true
	}

	var result []MetricAlarm
	for _, alarm := range b.alarms.All() {
		if namespace != "" && alarm.Namespace != namespace {
			continue
		}
		if metricName != "" && alarm.MetricName != metricName {
			continue
		}
		if len(nameSet) > 0 && !nameSet[alarm.AlarmName] {
			continue
		}
		if len(dimensions) > 0 && !dimsContainAll(alarm.Dimensions, dimensions) {
			continue
		}
		result = append(result, *alarm)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].AlarmName < result[j].AlarmName
	})

	return page.New(result, nextToken, maxRecords, cwDefaultDescribeForMetricLimit), nil
}

// DeleteAlarms removes alarms by name (metric and composite).
func (b *InMemoryBackend) DeleteAlarms(alarmNames []string) error {
	b.mu.Lock("DeleteAlarms")
	defer b.mu.Unlock()

	for _, name := range alarmNames {
		b.alarms.Delete(name)
		b.compositeAlarms.Delete(name)
		b.logAlarms.Delete(name)
		// Release the per-alarm history so it cannot accumulate across the
		// lifetime of the backend once the alarm itself is gone.
		delete(b.alarmHistory, name)
	}

	return nil
}

// EnableAlarmActions enables action execution for the given alarms.
func (b *InMemoryBackend) EnableAlarmActions(alarmNames []string) error {
	b.mu.Lock("EnableAlarmActions")
	defer b.mu.Unlock()

	for _, name := range alarmNames {
		if a, ok := b.alarms.Get(name); ok {
			a.ActionsEnabled = true
		}
		if ca, ok := b.compositeAlarms.Get(name); ok {
			ca.ActionsEnabled = true
		}
		if la, ok := b.logAlarms.Get(name); ok {
			la.ActionsEnabled = true
		}
	}

	return nil
}

// DisableAlarmActions disables action execution for the given alarms.
func (b *InMemoryBackend) DisableAlarmActions(alarmNames []string) error {
	b.mu.Lock("DisableAlarmActions")
	defer b.mu.Unlock()

	for _, name := range alarmNames {
		if a, ok := b.alarms.Get(name); ok {
			a.ActionsEnabled = false
		}
		if ca, ok := b.compositeAlarms.Get(name); ok {
			ca.ActionsEnabled = false
		}
		if la, ok := b.logAlarms.Get(name); ok {
			la.ActionsEnabled = false
		}
	}

	return nil
}

// GetAlarmARNs returns the ARNs for the given alarm names (metric + composite + log).
// Used by the HTTP handler to clean up tag entries on delete.
func (b *InMemoryBackend) GetAlarmARNs(names []string) []string {
	b.mu.RLock("GetAlarmARNs")
	defer b.mu.RUnlock()

	arns := make([]string, 0, len(names))
	for _, name := range names {
		if a, ok := b.alarms.Get(name); ok && a.AlarmArn != "" {
			arns = append(arns, a.AlarmArn)
		}
		if ca, ok := b.compositeAlarms.Get(name); ok && ca.AlarmArn != "" {
			arns = append(arns, ca.AlarmArn)
		}
		if la, ok := b.logAlarms.Get(name); ok && la.AlarmArn != "" {
			arns = append(arns, la.AlarmArn)
		}
	}

	return arns
}
