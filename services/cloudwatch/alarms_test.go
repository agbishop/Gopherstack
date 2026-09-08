package cloudwatch_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

// ---------------------------------------------------------------------------
// Alarm: DatapointsToAlarm validation
// ---------------------------------------------------------------------------

func TestBackend_PutMetricAlarm_DatapointsToAlarmExceedsEvalPeriods(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	err := b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:          "bad",
		Namespace:          "NS",
		MetricName:         "M",
		EvaluationPeriods:  3,
		DatapointsToAlarm:  5, // > EvaluationPeriods
		ComparisonOperator: "GreaterThanThreshold",
		Threshold:          80,
	})
	assert.Error(t, err, "DatapointsToAlarm > EvaluationPeriods should be rejected")
}

func TestBackend_PutMetricAlarm_DatapointsToAlarmValid(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	err := b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:          "ok",
		Namespace:          "NS",
		MetricName:         "M",
		EvaluationPeriods:  5,
		DatapointsToAlarm:  3,
		ComparisonOperator: "GreaterThanThreshold",
		Threshold:          80,
	})
	assert.NoError(t, err, "DatapointsToAlarm <= EvaluationPeriods is valid")
}

// ---------------------------------------------------------------------------
// Alarm: TreatMissingData field round-trip
// ---------------------------------------------------------------------------

func TestBackend_PutMetricAlarm_TreatMissingData(t *testing.T) {
	t.Parallel()

	for _, tmd := range []string{"missing", "notBreaching", "breaching", "ignore"} {
		t.Run(tmd, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackend()
			err := b.PutMetricAlarm(&cloudwatch.MetricAlarm{
				AlarmName:          "a",
				Namespace:          "NS",
				MetricName:         "M",
				EvaluationPeriods:  1,
				ComparisonOperator: "GreaterThanThreshold",
				Threshold:          50,
				TreatMissingData:   tmd,
			})
			require.NoError(t, err)

			alarms, _, _, err := b.DescribeAlarms([]string{"a"}, nil, "", "", "", 0, "", "", "")
			require.NoError(t, err)
			require.Len(t, alarms.Data, 1)
			assert.Equal(t, tmd, alarms.Data[0].TreatMissingData)
		})
	}
}

// ---------------------------------------------------------------------------
// Alarm: Statistic / ExtendedStatistic mutual exclusion
// ---------------------------------------------------------------------------

func TestBackend_PutMetricAlarm_StatAndExtendedStat_Rejected(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	err := b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:          "a",
		Namespace:          "NS",
		MetricName:         "M",
		EvaluationPeriods:  1,
		ComparisonOperator: "GreaterThanThreshold",
		Threshold:          50,
		Statistic:          "Average",
		ExtendedStatistic:  "p99",
	})
	assert.Error(t, err, "Statistic and ExtendedStatistic are mutually exclusive")
}

// ---------------------------------------------------------------------------
// DescribeAlarms: filters
// ---------------------------------------------------------------------------

func TestBackend_DescribeAlarms_ByNamePrefix(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	for _, name := range []string{"prod-cpu", "prod-mem", "staging-cpu"} {
		require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
			AlarmName: name, Namespace: "NS", MetricName: "M",
			ComparisonOperator: "GreaterThanThreshold", Threshold: 80, EvaluationPeriods: 1,
		}))
	}

	p, _, _, err := b.DescribeAlarms(nil, nil, "prod-", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Len(t, p.Data, 2)
	for _, a := range p.Data {
		assert.True(t, strings.HasPrefix(a.AlarmName, "prod-"))
	}
}

func TestBackend_DescribeAlarms_ByState(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "a1", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 80, EvaluationPeriods: 1,
	}))
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "a2", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 80, EvaluationPeriods: 1,
	}))
	require.NoError(t, b.SetAlarmState(t.Context(), "a1", "ALARM", "test", ""))

	p, _, _, err := b.DescribeAlarms(nil, nil, "", "ALARM", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Len(t, p.Data, 1)
	assert.Equal(t, "a1", p.Data[0].AlarmName)
}

func TestBackend_DescribeAlarmsForMetric_Filters(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	for _, mn := range []string{"CPU", "Memory"} {
		require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
			AlarmName: mn + "-alarm", Namespace: "AWS/EC2", MetricName: mn,
			ComparisonOperator: "GreaterThanThreshold", Threshold: 80, EvaluationPeriods: 1,
		}))
	}

	p, err := b.DescribeAlarmsForMetric("AWS/EC2", "CPU", nil, nil, "", 0)
	require.NoError(t, err)
	require.Len(t, p.Data, 1)
	assert.Equal(t, "CPU-alarm", p.Data[0].AlarmName)
}

// ---------------------------------------------------------------------------
// Fix 6: DescribeAlarmsForMetric ignores Dimensions filter
// ---------------------------------------------------------------------------

func TestDescribeAlarmsForMetric_DimensionFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filterDims []cloudwatch.Dimension
		wantNames  []string
	}{
		{
			name:       "filter by prod dimension — returns only prod alarm",
			filterDims: []cloudwatch.Dimension{{Name: "Env", Value: "prod"}},
			wantNames:  []string{"prod-alarm"},
		},
		{
			name:       "filter by staging dimension — returns only staging alarm",
			filterDims: []cloudwatch.Dimension{{Name: "Env", Value: "staging"}},
			wantNames:  []string{"staging-alarm"},
		},
		{
			name:       "no dimension filter — returns both alarms",
			filterDims: nil,
			wantNames:  []string{"prod-alarm", "staging-alarm"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackend()
			require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
				AlarmName:          "prod-alarm",
				Namespace:          "NS",
				MetricName:         "M",
				ComparisonOperator: "GreaterThanThreshold",
				EvaluationPeriods:  1,
				Period:             60,
				Dimensions:         []cloudwatch.Dimension{{Name: "Env", Value: "prod"}},
			}))
			require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
				AlarmName:          "staging-alarm",
				Namespace:          "NS",
				MetricName:         "M",
				ComparisonOperator: "GreaterThanThreshold",
				EvaluationPeriods:  1,
				Period:             60,
				Dimensions:         []cloudwatch.Dimension{{Name: "Env", Value: "staging"}},
			}))

			p, err := b.DescribeAlarmsForMetric("NS", "M", tc.filterDims, nil, "", 0)
			require.NoError(t, err)

			gotNames := make([]string, 0, len(p.Data))
			for _, a := range p.Data {
				gotNames = append(gotNames, a.AlarmName)
			}
			assert.ElementsMatch(t, tc.wantNames, gotNames,
				"DescribeAlarmsForMetric must filter by Dimensions when provided")
		})
	}
}

func TestCloudWatchBackend_PutAndDescribeAlarms(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	alarm := &cloudwatch.MetricAlarm{
		AlarmName:          "high-cpu",
		Namespace:          "AWS/EC2",
		MetricName:         "CPUUtilization",
		ComparisonOperator: "GreaterThanThreshold",
		Threshold:          80.0,
		EvaluationPeriods:  1,
		Period:             60,
		Statistic:          "Average",
	}
	require.NoError(t, b.PutMetricAlarm(alarm))

	alarms, _, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)
	assert.Equal(t, "high-cpu", alarms.Data[0].AlarmName)
	assert.Contains(t, alarms.Data[0].AlarmArn, "high-cpu")
	assert.Equal(t, "INSUFFICIENT_DATA", alarms.Data[0].StateValue)
}

func TestCloudWatchBackend_DescribeAlarms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, b *cloudwatch.InMemoryBackend)
		name       string
		stateValue string
		alarmNames []string
		alarmTypes []string
		wantCount  int
	}{
		{
			name: "filter_by_name",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				for _, name := range []string{"alarm-a", "alarm-b", "alarm-c"} {
					require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: name}))
				}
			},
			alarmNames: []string{"alarm-a", "alarm-c"},
			wantCount:  2,
		},
		{
			name: "filter_by_state",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "a1", StateValue: "OK"}),
				)
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "a2", StateValue: "ALARM"}),
				)
			},
			stateValue: "OK",
			wantCount:  1,
		},
		{
			// Real DescribeAlarms defaults to metric alarms only when AlarmTypes is
			// omitted -- a log alarm must not appear unless explicitly requested.
			name: "log_alarm_excluded_by_default",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "m1"}))
				require.NoError(t, b.PutLogAlarm(validLogAlarmForTest("log1")))
			},
			wantCount: 1,
		},
		{
			name: "log_alarm_included_when_requested",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "m1"}))
				require.NoError(t, b.PutLogAlarm(validLogAlarmForTest("log1")))
			},
			alarmTypes: []string{"LogAlarm"},
			wantCount:  1,
		},
		{
			// bd gopherstack-yvb7: a composite alarm must not appear unless
			// explicitly requested, matching the log_alarm_excluded_by_default
			// case above -- real DescribeAlarms defaults to metric alarms only.
			name: "composite_alarm_excluded_by_default",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "m1"}))
				require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
					AlarmName: "comp1", AlarmRule: `ALARM("m1")`,
				}))
			},
			wantCount: 1,
		},
		{
			name: "composite_alarm_included_when_requested",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "m1"}))
				require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
					AlarmName: "comp1", AlarmRule: `ALARM("m1")`,
				}))
			},
			alarmTypes: []string{"CompositeAlarm"},
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			if tt.setup != nil {
				tt.setup(t, b)
			}

			metric, composite, logAlarms, err := b.DescribeAlarms(
				tt.alarmNames, tt.alarmTypes, "", tt.stateValue, "", 0,
				"", "", "")
			require.NoError(t, err)
			gotCount := len(metric.Data) + len(composite.Data) + len(logAlarms.Data)
			assert.Equal(t, tt.wantCount, gotCount)
		})
	}
}

// validLogAlarmForTest builds a LogAlarm that satisfies PutLogAlarm's
// required-field validation, for tests that only care about the alarm
// existing and being named alarmName.
func validLogAlarmForTest(alarmName string) *cloudwatch.LogAlarm {
	return &cloudwatch.LogAlarm{
		AlarmName:              alarmName,
		ComparisonOperator:     "GreaterThanThreshold",
		Threshold:              1,
		QueryResultsToAlarm:    1,
		QueryResultsToEvaluate: 1,
		ScheduledQueryConfiguration: cloudwatch.ScheduledQueryConfiguration{
			AggregationExpression: "count(*)",
			QueryString:           "fields @message",
			ScheduledQueryRoleARN: "arn:aws:iam::123456789012:role/cw-log-alarm",
			ScheduleConfiguration: cloudwatch.ScheduleConfiguration{
				ScheduleExpression: "rate(5 minutes)",
				StartTimeOffset:    300,
			},
		},
	}
}

func TestCloudWatchBackend_DeleteAlarms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setup           func(t *testing.T, b *cloudwatch.InMemoryBackend)
		names           []string
		checkAlarmTypes []string
		wantRemaining   int
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "to-delete"}),
				)
			},
			names:         []string{"to-delete"},
			wantRemaining: 0,
		},
		{
			name:          "nonexistent",
			names:         []string{"no-such-alarm"},
			wantRemaining: 0,
		},
		{
			// DeleteAlarms must remove log alarms too -- it is not observable
			// via DescribeAlarms without AlarmTypes=LogAlarm, which is what
			// checkAlarmTypes exercises here.
			name: "log_alarm",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutLogAlarm(validLogAlarmForTest("log-to-delete")))
			},
			names:           []string{"log-to-delete"},
			checkAlarmTypes: []string{"LogAlarm"},
			wantRemaining:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			if tt.setup != nil {
				tt.setup(t, b)
			}

			require.NoError(t, b.DeleteAlarms(tt.names))

			metric, _, logAlarms, err := b.DescribeAlarms(nil, tt.checkAlarmTypes, "", "", "", 0, "", "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.wantRemaining, len(metric.Data)+len(logAlarms.Data))
		})
	}
}

func TestCloudWatchBackend_PutMetricAlarm_MissingName(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	err := b.PutMetricAlarm(&cloudwatch.MetricAlarm{})
	require.Error(t, err)
}

func TestCloudWatchBackend_PutMetricAlarm_UpdateExisting(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "upd", Threshold: 10}))
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "upd", Threshold: 20}))
	alarms, _, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Len(t, alarms.Data, 1)
	assert.InDelta(t, 20.0, alarms.Data[0].Threshold, 0.01)
}

func TestCloudWatchBackend_EnableDisableAlarmActions(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "test", ActionsEnabled: true}),
	)

	require.NoError(t, b.DisableAlarmActions([]string{"test"}))
	alarms, _, _, err := b.DescribeAlarms([]string{"test"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)
	assert.False(t, alarms.Data[0].ActionsEnabled)

	require.NoError(t, b.EnableAlarmActions([]string{"test"}))
	alarms2, _, _, err2 := b.DescribeAlarms([]string{"test"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err2)
	require.Len(t, alarms2.Data, 1)
	assert.True(t, alarms2.Data[0].ActionsEnabled)
}

func TestCloudWatchBackend_DescribeAlarmsForMetric(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "cpu-alarm", Namespace: "AWS/EC2", MetricName: "CPUUtilization",
	}))
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "mem-alarm", Namespace: "AWS/EC2", MetricName: "MemoryUtilization",
	}))

	p, err := b.DescribeAlarmsForMetric("AWS/EC2", "CPUUtilization", nil, nil, "", 0)
	require.NoError(t, err)
	require.Len(t, p.Data, 1)
	assert.Equal(t, "cpu-alarm", p.Data[0].AlarmName)
}

// TestCloudWatchBackend_DescribeAlarms_WithComposite verifies that a
// composite alarm is returned when AlarmTypes explicitly requests
// "CompositeAlarm", but -- per DescribeAlarmsInput.AlarmTypes's own doc
// comment ("If you omit this parameter, only metric alarms are returned,
// even if composite alarms or log alarms exist in the account", confirmed
// against aws-sdk-go-v2/service/cloudwatch@v1.66.3/api_op_DescribeAlarms.go)
// -- is NOT returned when AlarmTypes is omitted (bd gopherstack-yvb7: this
// backend previously defaulted composite alarms in alongside metric alarms,
// contradicting the documented default that only LogAlarm already honored
// correctly). See also the composite_alarm_excluded_by_default/
// composite_alarm_included_when_requested cases in
// TestCloudWatchBackend_DescribeAlarms below.
func TestCloudWatchBackend_DescribeAlarms_WithComposite(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "metric1", StateValue: "ALARM"}),
	)
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "comp1", AlarmRule: `ALARM("metric1")`,
	}))

	// AlarmTypes omitted: only the metric alarm comes back.
	metricPage, compositePage, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Len(t, metricPage.Data, 1)
	assert.Empty(t, compositePage.Data, "composite alarms must NOT be returned when AlarmTypes is omitted")

	// AlarmTypes=["CompositeAlarm"] explicitly requested: composite alarm comes back.
	metricPage2, compositePage2, _, err2 := b.DescribeAlarms(nil, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	require.NoError(t, err2)
	assert.Empty(t, metricPage2.Data, "metric alarms must NOT be returned when only CompositeAlarm is requested")
	require.Len(t, compositePage2.Data, 1)
	assert.Equal(t, "ALARM", compositePage2.Data[0].StateValue)
}

func TestCloudWatchBackend_DescribeAlarmsForMetric_WithAlarmNames(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:  "match-name",
		Namespace:  "NS",
		MetricName: "M",
	}))
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:  "other-name",
		Namespace:  "NS",
		MetricName: "M",
	}))

	// Filter by both namespace+metric AND alarm name.
	p, err := b.DescribeAlarmsForMetric("NS", "M", nil, []string{"match-name"}, "", 0)
	require.NoError(t, err)
	require.Len(t, p.Data, 1)
	assert.Equal(t, "match-name", p.Data[0].AlarmName)
}

func TestCloudWatchBackend_DescribeAlarms_StateFilter(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "a-ok", StateValue: "OK"}),
	)
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "a-alarm", StateValue: "ALARM"}),
	)

	p, _, _, err := b.DescribeAlarms(nil, nil, "", "OK", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, p.Data, 1)
	assert.Equal(t, "a-ok", p.Data[0].AlarmName)
}

func TestCloudWatchBackend_DescribeAlarms_AlarmNamePrefix(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	for _, name := range []string{"prod-cpu", "prod-mem", "staging-cpu"} {
		require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
			AlarmName:          name,
			ComparisonOperator: "GreaterThanThreshold",
			EvaluationPeriods:  1,
			Period:             60,
		}))
	}

	p, _, _, err := b.DescribeAlarms(nil, nil, "prod-", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, p.Data, 2)
	for _, a := range p.Data {
		assert.Contains(t, a.AlarmName, "prod-")
	}
}

func TestCloudWatchBackend_StateTransitionedTimestamp(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:          "ts-alarm",
		ComparisonOperator: "GreaterThanThreshold",
		EvaluationPeriods:  1,
		Period:             60,
	}))

	p, _, _, err := b.DescribeAlarms([]string{"ts-alarm"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, p.Data, 1)
	assert.False(
		t,
		p.Data[0].StateTransitionedTimestamp.IsZero(),
		"StateTransitionedTimestamp should be set on creation",
	)

	// Change state — timestamp should update.
	prevTS := p.Data[0].StateTransitionedTimestamp
	require.NoError(t, b.SetAlarmState(t.Context(), "ts-alarm", "ALARM", "manual", ""))

	p2, _, _, err2 := b.DescribeAlarms([]string{"ts-alarm"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err2)
	require.Len(t, p2.Data, 1)

	newTS := p2.Data[0].StateTransitionedTimestamp
	assert.True(
		t,
		newTS.After(prevTS) || newTS.Equal(prevTS),
		"timestamp should not go backwards",
	)
}

func TestCloudWatchBackend_GetAlarmARNs(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:          "arn-alarm",
		ComparisonOperator: "GreaterThanThreshold",
		EvaluationPeriods:  1,
		Period:             60,
	}))

	arns := b.GetAlarmARNs([]string{"arn-alarm", "nonexistent"})
	require.Len(t, arns, 1)
	assert.Contains(t, arns[0], "arn-alarm")
}

// ---------------------------------------------------------------------------
// PutLogAlarm: a third alarm type (alongside MetricAlarm/CompositeAlarm)
// ---------------------------------------------------------------------------

func TestCloudWatchBackend_PutLogAlarm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mutate  func(a *cloudwatch.LogAlarm)
		name    string
		wantErr bool
	}{
		{
			name: "valid",
		},
		{
			name:    "missing_name",
			mutate:  func(a *cloudwatch.LogAlarm) { a.AlarmName = "" },
			wantErr: true,
		},
		{
			name:    "invalid_comparison_operator",
			mutate:  func(a *cloudwatch.LogAlarm) { a.ComparisonOperator = "LessThanLowerThreshold" },
			wantErr: true,
		},
		{
			name: "query_results_to_alarm_exceeds_evaluate",
			mutate: func(a *cloudwatch.LogAlarm) {
				a.QueryResultsToAlarm = 5
				a.QueryResultsToEvaluate = 3
			},
			wantErr: true,
		},
		{
			name:    "query_results_to_evaluate_out_of_range",
			mutate:  func(a *cloudwatch.LogAlarm) { a.QueryResultsToEvaluate = 0 },
			wantErr: true,
		},
		{
			name:    "missing_query_string",
			mutate:  func(a *cloudwatch.LogAlarm) { a.ScheduledQueryConfiguration.QueryString = "" },
			wantErr: true,
		},
		{
			name: "missing_schedule_expression",
			mutate: func(a *cloudwatch.LogAlarm) {
				a.ScheduledQueryConfiguration.ScheduleConfiguration.ScheduleExpression = ""
			},
			wantErr: true,
		},
		{
			name: "action_log_line_count_without_role",
			mutate: func(a *cloudwatch.LogAlarm) {
				a.ActionLogLineCount = 5
				a.ActionLogLineRoleArn = ""
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			alarm := validLogAlarmForTest("log-validation")
			if tt.mutate != nil {
				tt.mutate(alarm)
			}

			err := b.PutLogAlarm(alarm)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCloudWatchBackend_PutLogAlarm_CreateOrUpdate(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	first := validLogAlarmForTest("log-upd")
	first.Threshold = 10
	require.NoError(t, b.PutLogAlarm(first))

	_, _, logAlarms, err := b.DescribeAlarms(nil, []string{"LogAlarm"}, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, logAlarms.Data, 1)
	assert.InDelta(t, 10.0, logAlarms.Data[0].Threshold, 0.01)
	assert.Equal(t, "INSUFFICIENT_DATA", logAlarms.Data[0].StateValue)
	assert.NotEmpty(t, logAlarms.Data[0].AlarmArn)
	assert.False(t, logAlarms.Data[0].StateTransitionedTimestamp.IsZero())

	second := validLogAlarmForTest("log-upd")
	second.Threshold = 20
	require.NoError(t, b.PutLogAlarm(second))

	_, _, logAlarms2, err2 := b.DescribeAlarms(nil, []string{"LogAlarm"}, "", "", "", 0, "", "", "")
	require.NoError(t, err2)
	require.Len(t, logAlarms2.Data, 1, "PutLogAlarm must update in place, not duplicate")
	assert.InDelta(t, 20.0, logAlarms2.Data[0].Threshold, 0.01)

	// GetAlarmARNs and DeleteAlarms both observe log alarms.
	arns := b.GetAlarmARNs([]string{"log-upd"})
	require.Len(t, arns, 1)
	assert.Contains(t, arns[0], "log-upd")

	require.NoError(t, b.DeleteAlarms([]string{"log-upd"}))
	_, _, logAlarms3, err3 := b.DescribeAlarms(nil, []string{"LogAlarm"}, "", "", "", 0, "", "", "")
	require.NoError(t, err3)
	assert.Empty(t, logAlarms3.Data)
}
