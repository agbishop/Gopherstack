package cloudwatch

import "time"

// SignPageTokenForTest exposes signPageToken for unit testing.
func SignPageTokenForTest(offset int) string { return signPageToken(offset) }

// ParseSignedPageTokenForTest exposes parseSignedPageToken for unit testing.
func ParseSignedPageTokenForTest(token string) (int, error) { return parseSignedPageToken(token) }

// ValidateMetricDatumForTest exposes validateMetricDatum for unit testing.
func ValidateMetricDatumForTest(d MetricDatum) error {
	return validateMetricDatum(d, time.Now().UTC())
}

// ValidateMetricDatumAtForTest exposes validateMetricDatum with an explicit
// "now" reference, for testing the Timestamp acceptance window.
func ValidateMetricDatumAtForTest(d MetricDatum, now time.Time) error {
	return validateMetricDatum(d, now)
}

// ValidateStorageResolutionForTest exposes validateStorageResolution for unit testing.
func ValidateStorageResolutionForTest(res int32) error { return validateStorageResolution(res) }

// TopoSortExpressionsForTest exposes topoSortExpressions for unit testing.
func TopoSortExpressionsForTest(queries []MetricDataQuery) ([]string, error) {
	return topoSortExpressions(queries)
}

// ExtractExprDepsForTest exposes extractExprDeps for unit testing.
func ExtractExprDepsForTest(expr string, known map[string]bool) []string {
	return extractExprDeps(expr, known)
}

// RollingStatsForTest exposes rollingStats for unit testing.
func RollingStatsForTest(vals []float64) (float64, float64) { return rollingStats(vals) }

// ComputeAnomalyBandForTest exposes computeAnomalyBand for unit testing.
func ComputeAnomalyBandForTest(values []float64) ([]float64, []float64) {
	return computeAnomalyBand(values)
}

// EvalAnomalyDetectionBandForTest exposes evalAnomalyDetectionBand for unit testing.
func EvalAnomalyDetectionBandForTest(
	expr string,
	resolved map[string]MetricDataResult,
) (MetricDataResult, MetricDataResult, bool) {
	return evalAnomalyDetectionBand(expr, resolved)
}

// EvaluateAlarmRuleForTest is a test-visible wrapper around evaluateAlarmRule.
func EvaluateAlarmRuleForTest(rule string, states map[string]string) string {
	return evaluateAlarmRule(rule, func(name string) string {
		if s, ok := states[name]; ok {
			return s
		}

		return alarmStateInsufficientData
	})
}

// CwMaxMetricNamesPerNamespace exposes the constant for tests.
const CwMaxMetricNamesPerNamespace = cwMaxMetricNamesPerNamespace

// CwMetricRetentionDays exposes the constant for tests.
const CwMetricRetentionDays = cwMetricRetentionDays

// GetInsightRuleContributors is a test-visible wrapper that acquires the lock
// and delegates to the unexported implementation.
func (b *InMemoryBackend) GetInsightRuleContributorsForTest(
	ruleName string,
	startTime, endTime time.Time,
	maxContributorCount int,
	orderBy string,
) ([]InsightRuleContributor, error) {
	b.mu.RLock("GetInsightRuleContributorsForTest")
	defer b.mu.RUnlock()

	return b.GetInsightRuleContributors(ruleName, startTime, endTime, maxContributorCount, orderBy)
}

// DimensionSetKeyForTest exposes the internal dimensionSetKey for tests.
func DimensionSetKeyForTest(dims []Dimension) string {
	return dimensionSetKey(dims)
}

// StreamAllowsMetricForTest exposes the stream filter predicate for tests.
func StreamAllowsMetricForTest(s *MetricStream, namespace, metricName string) bool {
	return streamAllowsMetric(s, namespace, metricName)
}

// AlarmHistoryKeyCountForTest returns the number of distinct alarm names that
// currently have retained history, for leak-detection tests.
func (b *InMemoryBackend) AlarmHistoryKeyCountForTest() int {
	b.mu.RLock("AlarmHistoryKeyCountForTest")
	defer b.mu.RUnlock()

	return len(b.alarmHistory)
}

// MetricPointsCapForTest returns the backing-array capacity of the points slice
// for the named metric series in the given namespace and dimension key.
// Used to verify that the cap-and-copy path bounds allocation, not just length.
func (b *InMemoryBackend) MetricPointsCapForTest(namespace, metricKey string) int {
	b.mu.RLock("MetricPointsCapForTest")
	defer b.mu.RUnlock()

	nsMetrics, ok := b.metrics[namespace]
	if !ok {
		return -1
	}

	rec, ok := nsMetrics[metricKey]
	if !ok {
		return -1
	}

	return cap(rec.Points)
}

// CwMaxMetricDataPointsForTest exposes the per-series data-point cap for tests.
const CwMaxMetricDataPointsForTest = cwMaxMetricDataPoints

// ComputeExtendedStatsForTest exposes computeExtendedStats for unit testing.
func ComputeExtendedStatsForTest(sortedVals []float64, stats []string) map[string]float64 {
	return computeExtendedStats(sortedVals, stats)
}

// ComputeExtendedStatForTest exposes computeExtendedStat for unit testing.
func ComputeExtendedStatForTest(sortedVals []float64, stat string) (float64, bool) {
	return computeExtendedStat(sortedVals, stat)
}

// ExpandDatumValuesForTest exposes expandDatumValues for unit testing.
func ExpandDatumValuesForTest(d MetricDatum) []float64 { return expandDatumValues(d) }

// PaginateMetricDataForTest exposes paginateMetricData for unit testing.
func PaginateMetricDataForTest(
	all []MetricDataResult,
	maxDatapoints int,
	nextToken string,
) GetMetricDataPage {
	return paginateMetricData(all, maxDatapoints, nextToken)
}

// AnnotateArithmeticMessagesForTest exposes annotateArithmeticMessages for testing.
func AnnotateArithmeticMessagesForTest(r *MetricDataResult) { annotateArithmeticMessages(r) }

// ParseEC2AutomateVerbForTest exposes parseEC2AutomateVerb for unit testing.
func ParseEC2AutomateVerbForTest(action string) (string, bool) {
	return parseEC2AutomateVerb(action)
}

// ParseScalingPolicyARNForTest exposes parseScalingPolicyARN for unit testing.
func ParseScalingPolicyARNForTest(action string) (string, string, bool) {
	return parseScalingPolicyARN(action)
}

// ExtractAlarmRuleRefsForTest exposes extractAlarmRuleRefs, flattened to parallel
// func/name slices for the external test package.
func ExtractAlarmRuleRefsForTest(rule string) ([]string, []string) {
	refs := extractAlarmRuleRefs(rule)
	funcs := make([]string, 0, len(refs))
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		funcs = append(funcs, r.Func)
		names = append(names, r.Name)
	}

	return funcs, names
}

// ParseRelativeDurationForTest exposes parseRelativeDuration for unit testing.
func ParseRelativeDurationForTest(s string) (time.Duration, bool) {
	return parseRelativeDuration(s)
}

// RenderMetricWidgetPNGForTest exposes renderMetricWidgetPNG for unit testing.
func RenderMetricWidgetPNGForTest(
	b *InMemoryBackend,
	widgetJSON string,
	now time.Time,
) ([]byte, error) {
	return renderMetricWidgetPNG(b, widgetJSON, now)
}

// RecentTestAnchor returns a fixed point in time, safely inside PutMetricData's
// write-time Timestamp acceptance window (two weeks past / two hours future),
// truncated to the minute so period-bucket-alignment tests built on offsets
// from it behave the same as they did with a fixed calendar-date anchor.
func RecentTestAnchor() time.Time {
	return time.Now().UTC().Add(-time.Hour).Truncate(time.Minute)
}

// legacyTestEpoch is the fixed calendar date historically used throughout the
// cloudwatch test suite as a form-encoded Timestamp/StartTime/EndTime literal,
// before PutMetricData enforced its write-time Timestamp acceptance window.
//
//nolint:gochecknoglobals // test-only fixed reference point
var legacyTestEpoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// ShiftTestTimestampForTest remaps an RFC3339 literal expressed relative to
// legacyTestEpoch onto one relative to anchor, preserving the offset (which
// may cross a day boundary). It exists so form-encoded query strings built
// from literal date constants (readable, and easy to keep multiple related
// timestamps consistent with each other) keep working now that PutMetricData
// rejects Timestamps outside its acceptance window. Panics on malformed input
// since it is only ever called with literal constants in test source.
func ShiftTestTimestampForTest(anchor time.Time, legacy string) string {
	t, err := time.Parse(time.RFC3339, legacy)
	if err != nil {
		panic("ShiftTestTimestampForTest: " + err.Error())
	}

	return anchor.Add(t.Sub(legacyTestEpoch)).Format(time.RFC3339)
}

// TotalMetricsForTest exposes the running totalMetrics counter (#60) for tests.
func (b *InMemoryBackend) TotalMetricsForTest() int {
	b.mu.RLock("TotalMetricsForTest")
	defer b.mu.RUnlock()

	return b.totalMetrics
}

// StoreDatumForTest stores a MetricDatum directly, bypassing PutMetricData's
// write-time Timestamp-acceptance-window check (real CloudWatch only enforces
// that window at write time; a datapoint legitimately written within the
// window ages past retention exactly like any other, so tests exercising
// SweepExpiredMetrics need a way to seed already-aged data without going
// through the now-enforced window check on PutMetricData itself).
func (b *InMemoryBackend) StoreDatumForTest(namespace string, d MetricDatum) {
	b.mu.Lock("StoreDatumForTest")
	defer b.mu.Unlock()

	if b.metrics[namespace] == nil {
		b.metrics[namespace] = make(map[string]*metricRecord)
	}

	d.Namespace = namespace
	b.storeDatum(namespace, d)
}
