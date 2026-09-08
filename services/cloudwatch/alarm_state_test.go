package cloudwatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

// ---------------------------------------------------------------------------
// SetAlarmState: state reasons, history
// ---------------------------------------------------------------------------

func TestBackend_SetAlarmState_ToAlarm_RecordsHistory(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	err := b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "cpu-alarm", Namespace: "AWS/EC2", MetricName: "CPUUtilization",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 80, EvaluationPeriods: 1,
	})
	require.NoError(t, err)

	err = b.SetAlarmState(t.Context(), "cpu-alarm", "ALARM", "CPU exceeded 80%", "")
	require.NoError(t, err)

	hist, err := b.DescribeAlarmHistory("cpu-alarm", nil, "", "", "", time.Time{}, time.Time{}, 0)
	require.NoError(t, err)
	require.NotEmpty(t, hist.Data)

	found := false
	for _, item := range hist.Data {
		if item.HistoryItemType == "StateUpdate" {
			found = true
			assert.Contains(t, item.HistorySummary, "ALARM")
		}
	}
	assert.True(t, found, "SetAlarmState should create a StateUpdate history item")
}

func TestBackend_SetAlarmState_Reason_Stored(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "a1", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
	}))

	require.NoError(t, b.SetAlarmState(t.Context(), "a1", "ALARM", "Threshold breach detected", ""))

	alarms, _, _, err := b.DescribeAlarms([]string{"a1"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)
	assert.Equal(t, "ALARM", alarms.Data[0].StateValue)
	assert.Equal(t, "Threshold breach detected", alarms.Data[0].StateReason)
}

func TestBackend_SetAlarmState_StateTransitions(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "a1", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
	}))

	// Initial state is INSUFFICIENT_DATA.
	alarms, _, _, err := b.DescribeAlarms([]string{"a1"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "INSUFFICIENT_DATA", alarms.Data[0].StateValue)

	// Transition to ALARM.
	require.NoError(t, b.SetAlarmState(t.Context(), "a1", "ALARM", "breach", ""))
	alarms, _, _, err = b.DescribeAlarms([]string{"a1"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "ALARM", alarms.Data[0].StateValue)

	// Transition to OK.
	require.NoError(t, b.SetAlarmState(t.Context(), "a1", "OK", "resolved", ""))
	alarms, _, _, err = b.DescribeAlarms([]string{"a1"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "OK", alarms.Data[0].StateValue)
}

func TestBackend_SetAlarmState_NotFound_Error(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	err := b.SetAlarmState(t.Context(), "nonexistent", "ALARM", "reason", "")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Fix 2: SetAlarmState ignores StateReasonData form parameter
// ---------------------------------------------------------------------------

func TestSetAlarmState_StateReasonData_StoredAndReturned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		stateReasonData string
	}{
		{
			name:            "JSON blob stored and returned",
			stateReasonData: `{"version":"1.0","queryDate":"2026-06-01T00:00:00Z","statistic":"Average","period":300}`,
		},
		{
			name:            "empty string — field absent",
			stateReasonData: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackend()
			require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
				AlarmName:          "a",
				Namespace:          "NS",
				MetricName:         "M",
				ComparisonOperator: "GreaterThanThreshold",
				EvaluationPeriods:  1,
				Period:             60,
				Statistic:          "Average",
			}))

			require.NoError(t, b.SetAlarmState(t.Context(), "a", "ALARM", "manual", tc.stateReasonData))

			p, _, _, err := b.DescribeAlarms([]string{"a"}, nil, "", "", "", 0, "", "", "")
			require.NoError(t, err)
			require.Len(t, p.Data, 1)
			assert.Equal(t, tc.stateReasonData, p.Data[0].StateReasonData)
		})
	}
}

// ---------------------------------------------------------------------------
// SetAlarmState — manual override persists
// ---------------------------------------------------------------------------

func TestSetAlarmState_ManualOverride_AllStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setState  string
		wantState string
	}{
		{name: "set_alarm", setState: "ALARM", wantState: "ALARM"},
		{name: "set_ok", setState: "OK", wantState: "OK"},
		{name: "set_insufficient", setState: "INSUFFICIENT_DATA", wantState: "INSUFFICIENT_DATA"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackend()
			aName := "sa-alarm-" + tc.name

			require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
				AlarmName:          aName,
				Namespace:          "NS",
				MetricName:         "M",
				Statistic:          "Average",
				ComparisonOperator: "GreaterThanThreshold",
				EvaluationPeriods:  1,
				Period:             60,
				Threshold:          50,
			}))

			require.NoError(t, b.SetAlarmState(context.Background(), aName, tc.setState, "manual override", ""))

			got := describeMetricAlarm(t, b, aName)
			assert.Equal(t, tc.wantState, got.StateValue)
		})
	}
}

func TestCloudWatchBackend_SetAlarmState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T, b *cloudwatch.InMemoryBackend)
		alarmName   string
		stateValue  string
		stateReason string
		wantErr     bool
	}{
		{
			name: "metric_alarm_state_change",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "test-alarm"}),
				)
			},
			alarmName:   "test-alarm",
			stateValue:  "ALARM",
			stateReason: "Test triggered",
		},
		{
			name: "composite_alarm_state_change",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "child", StateValue: "OK"}),
				)
				require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
					AlarmName: "comp", AlarmRule: `ALARM("child")`,
				}))
			},
			alarmName:   "comp",
			stateValue:  "ALARM",
			stateReason: "Manual override",
		},
		{
			name:       "nonexistent_alarm",
			alarmName:  "no-alarm",
			stateValue: "ALARM",
			wantErr:    true,
		},
		{
			name: "log_alarm_state_change",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutLogAlarm(validLogAlarmForTest("log-state")))
			},
			alarmName:   "log-state",
			stateValue:  "ALARM",
			stateReason: "manual log alarm override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := b.SetAlarmState(t.Context(), tt.alarmName, tt.stateValue, tt.stateReason, "")
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestCloudWatchBackend_SetAlarmState_ChildTriggersCompositeReevaluation(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	// Child alarm starts in ALARM state, composite rule evaluates to ALARM.
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "child-direct", StateValue: "ALARM"}),
	)
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "direct-composite",
		AlarmRule: `ALARM("child-direct")`,
	}))

	// Composite should be ALARM since child is ALARM.
	_, composites0, _, err0 := b.DescribeAlarms(
		[]string{"direct-composite"},
		[]string{"CompositeAlarm"},
		"",
		"",
		"",
		0,
		"",
		"",
		"",
	)
	require.NoError(t, err0)
	require.Len(t, composites0.Data, 1)
	assert.Equal(t, "ALARM", composites0.Data[0].StateValue)

	// SetAlarmState on child to OK; composite should re-evaluate to OK.
	require.NoError(t, b.SetAlarmState(t.Context(), "child-direct", "OK", "recovered", ""))

	_, composites, _, err := b.DescribeAlarms(
		[]string{"direct-composite"},
		[]string{"CompositeAlarm"},
		"",
		"",
		"",
		0,
		"",
		"",
		"",
	)
	require.NoError(t, err)
	require.Len(t, composites.Data, 1)
	assert.Equal(t, "OK", composites.Data[0].StateValue)
}

func TestCloudWatchBackend_SetAlarmState_OKAndInsufficientData(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	pub := &mockSNSPublisher{}
	b.SetSNSPublisher(pub)

	topicARN := "arn:aws:sns:us-east-1:123456789012:test-topic-2"

	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:               "state-cycle",
		StateValue:              "ALARM",
		ActionsEnabled:          true,
		OKActions:               []string{topicARN},
		InsufficientDataActions: []string{topicARN},
	}))

	// Transition to OK — should fire OKActions.
	require.NoError(t, b.SetAlarmState(t.Context(), "state-cycle", "OK", "recovered", ""))
	assert.Len(t, pub.messages, 1)

	// Transition to INSUFFICIENT_DATA — should fire InsufficientDataActions.
	require.NoError(t, b.SetAlarmState(t.Context(), "state-cycle", "INSUFFICIENT_DATA", "no data", ""))
	assert.Len(t, pub.messages, 2)
}
