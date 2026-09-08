package cloudwatch_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

// ---------------------------------------------------------------------------
// CompositeAlarm: state evaluation
// ---------------------------------------------------------------------------

func TestBackend_CompositeAlarm_EvaluatesOnCreate(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()

	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "child", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
	}))
	require.NoError(t, b.SetAlarmState(t.Context(), "child", "ALARM", "test", ""))

	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "composite",
		AlarmRule: "ALARM(child)",
	}))

	_, compositeAlarms, _, err := b.DescribeAlarms(
		[]string{"composite"},
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
	require.Len(t, compositeAlarms.Data, 1)
	assert.Equal(t, "ALARM", compositeAlarms.Data[0].StateValue,
		"composite alarm should reflect child alarm state on create")
}

func TestBackend_CompositeAlarm_UpdatesOnChildChange(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()

	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "child", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "composite", AlarmRule: "ALARM(child)",
	}))

	// Child initially INSUFFICIENT_DATA → composite should be non-ALARM.
	_, compPages, _, _ := b.DescribeAlarms([]string{"composite"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	require.Len(t, compPages.Data, 1)
	initialState := compPages.Data[0].StateValue

	// Trigger child alarm.
	require.NoError(t, b.SetAlarmState(t.Context(), "child", "ALARM", "trigger", ""))

	_, compPages, _, _ = b.DescribeAlarms([]string{"composite"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	require.Len(t, compPages.Data, 1)
	afterState := compPages.Data[0].StateValue

	assert.NotEqual(t, initialState, afterState, "composite should transition when child changes")
	assert.Equal(t, "ALARM", afterState)
}

// ---------------------------------------------------------------------------
// EnableAlarmActions / DisableAlarmActions
// ---------------------------------------------------------------------------

func TestBackend_EnableDisableAlarmActions_Composite(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "c1", AlarmRule: "ALARM(nonexistent)",
	}))

	require.NoError(t, b.DisableAlarmActions([]string{"c1"}))
	_, cp, _, _ := b.DescribeAlarms([]string{"c1"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	require.Len(t, cp.Data, 1)
	assert.False(t, cp.Data[0].ActionsEnabled)

	require.NoError(t, b.EnableAlarmActions([]string{"c1"}))
	_, cp, _, _ = b.DescribeAlarms([]string{"c1"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	assert.True(t, cp.Data[0].ActionsEnabled)
}

// ---------------------------------------------------------------------------
// Fix 3: CompositeAlarm missing StateTransitionedTimestamp
// ---------------------------------------------------------------------------

func TestCompositeAlarm_StateTransitionedTimestamp_SetAlarmState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stateValue string
	}{
		{name: "transition to ALARM", stateValue: "ALARM"},
		{name: "transition to OK", stateValue: "OK"},
		{name: "transition to INSUFFICIENT_DATA", stateValue: "INSUFFICIENT_DATA"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackend()
			require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
				AlarmName: "comp",
				AlarmRule: "FALSE",
			}))

			require.NoError(t, b.SetAlarmState(t.Context(), "comp", tc.stateValue, "manual", ""))

			_, p, _, err := b.DescribeAlarms([]string{"comp"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
			require.NoError(t, err)
			require.Len(t, p.Data, 1)

			assert.False(t, p.Data[0].StateTransitionedTimestamp.IsZero(),
				"StateTransitionedTimestamp must be set after SetAlarmState")
		})
	}
}

func TestCompositeAlarm_StateTransitionedTimestamp_ReEvalOnChildChange(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:          "child",
		Namespace:          "NS",
		MetricName:         "M",
		ComparisonOperator: "GreaterThanThreshold",
		EvaluationPeriods:  1,
		Period:             60,
		Statistic:          "Average",
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "parent",
		AlarmRule: "ALARM(child)",
	}))

	_, before, _, err := b.DescribeAlarms([]string{"parent"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, before.Data, 1)
	assert.False(t, before.Data[0].StateTransitionedTimestamp.IsZero(),
		"StateTransitionedTimestamp must be initialized on creation")

	// Trigger composite re-evaluation by setting child to ALARM.
	require.NoError(t, b.SetAlarmState(t.Context(), "child", "ALARM", "test", ""))

	_, after, _, err := b.DescribeAlarms([]string{"parent"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, after.Data, 1)
	assert.Equal(t, "ALARM", after.Data[0].StateValue)
	assert.False(t, after.Data[0].StateTransitionedTimestamp.IsZero(),
		"StateTransitionedTimestamp must be updated after composite re-evaluation")
	assert.True(t, after.Data[0].StateTransitionedTimestamp.After(before.Data[0].StateTransitionedTimestamp),
		"StateTransitionedTimestamp must advance when state changes via re-evaluation")
}

func TestCloudWatchBackend_PutCompositeAlarm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T, b *cloudwatch.InMemoryBackend)
		alarm     *cloudwatch.CompositeAlarm
		wantState string
		wantErr   bool
	}{
		{
			name: "alarm_in_alarm_state",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(
						&cloudwatch.MetricAlarm{AlarmName: "child", StateValue: "ALARM"},
					),
				)
			},
			alarm: &cloudwatch.CompositeAlarm{
				AlarmName: "composite",
				AlarmRule: `ALARM("child")`,
			},
			wantState: "ALARM",
		},
		{
			name: "alarm_in_ok_state",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "child", StateValue: "OK"}),
				)
			},
			alarm: &cloudwatch.CompositeAlarm{
				AlarmName: "composite",
				AlarmRule: `ALARM("child")`,
			},
			wantState: "OK",
		},
		{
			name: "and_rule",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "a", StateValue: "ALARM"}),
				)
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "b", StateValue: "ALARM"}),
				)
			},
			alarm: &cloudwatch.CompositeAlarm{
				AlarmName: "composite",
				AlarmRule: `ALARM("a") AND ALARM("b")`,
			},
			wantState: "ALARM",
		},
		{
			name: "or_rule_one_ok",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "a", StateValue: "ALARM"}),
				)
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "b", StateValue: "OK"}),
				)
			},
			alarm: &cloudwatch.CompositeAlarm{
				AlarmName: "composite",
				AlarmRule: `ALARM("a") OR ALARM("b")`,
			},
			wantState: "ALARM",
		},
		{
			name:    "missing_name",
			alarm:   &cloudwatch.CompositeAlarm{AlarmRule: `ALARM("x")`},
			wantErr: true,
		},
		{
			name:    "missing_rule",
			alarm:   &cloudwatch.CompositeAlarm{AlarmName: "c"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := b.PutCompositeAlarm(tt.alarm)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			_, compositeAlarms, _, err2 := b.DescribeAlarms(
				[]string{tt.alarm.AlarmName},
				[]string{"CompositeAlarm"},
				"",
				"",
				"",
				0,
				"", "", "")
			require.NoError(t, err2)
			require.Len(t, compositeAlarms.Data, 1)
			assert.Equal(t, tt.wantState, compositeAlarms.Data[0].StateValue)
		})
	}
}

func TestCloudWatchBackend_CompositeAlarmReevalOnChildChange(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "child", StateValue: "OK"}),
	)
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "parent", AlarmRule: `ALARM("child")`,
	}))

	// Initially composite should be OK since child is OK
	_, compositeAlarms, _, err := b.DescribeAlarms(
		[]string{"parent"},
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
	assert.Equal(t, "OK", compositeAlarms.Data[0].StateValue)

	// Change child to ALARM; composite should re-evaluate
	require.NoError(t, b.SetAlarmState(t.Context(), "child", "ALARM", "test", ""))
	_, compositeAlarms2, _, err2 := b.DescribeAlarms(
		[]string{"parent"},
		[]string{"CompositeAlarm"},
		"",
		"",
		"",
		0,
		"",
		"",
		"",
	)
	require.NoError(t, err2)
	assert.Equal(t, "ALARM", compositeAlarms2.Data[0].StateValue)
}

func TestCloudWatchBackend_CompositeAlarmActionsFireOnChildChange(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	pub := &mockSNSPublisher{}
	b.SetSNSPublisher(pub)

	topicARN := "arn:aws:sns:us-east-1:123456789012:test-topic"

	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "child2", StateValue: "OK"}),
	)
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName:      "parent2",
		AlarmRule:      `ALARM("child2")`,
		ActionsEnabled: true,
		AlarmActions:   []string{topicARN},
	}))

	// No SNS publish yet — child is OK, composite is OK.
	assert.Empty(t, pub.messages)

	// Transition child to ALARM; composite should re-evaluate and fire its AlarmActions.
	require.NoError(t, b.SetAlarmState(t.Context(), "child2", "ALARM", "test trigger", ""))

	assert.Len(t, pub.messages, 1, "composite alarm action should have been fired")
	assert.Contains(t, pub.messages[0], "parent2")

	// The fired action's history entry must be tagged AlarmType=CompositeAlarm
	// (previously hardcoded to "MetricAlarm" for every action-fired entry,
	// regardless of which kind of alarm actually fired it), so
	// DescribeAlarmHistory's AlarmType filter can find it.
	composite, err := b.DescribeAlarmHistory(
		"parent2", []string{"CompositeAlarm"}, "Action", "", "", time.Time{}, time.Time{}, 0,
	)
	require.NoError(t, err)
	require.NotEmpty(t, composite.Data, "composite alarm's Action history should be tagged CompositeAlarm")

	metricTyped, err := b.DescribeAlarmHistory(
		"parent2", []string{"MetricAlarm"}, "Action", "", "", time.Time{}, time.Time{}, 0,
	)
	require.NoError(t, err)
	assert.Empty(t, metricTyped.Data,
		"a composite alarm's Action history must not be mistagged as MetricAlarm")
}

func TestCloudWatchBackend_EvalCompositeRule_NestedComposite(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "leaf", StateValue: "ALARM"}),
	)
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "mid",
		AlarmRule: `ALARM("leaf")`,
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "top",
		AlarmRule: `ALARM("mid")`,
	}))

	_, composites, _, err := b.DescribeAlarms([]string{"top"}, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, composites.Data, 1)
	// top should be ALARM since leaf is ALARM -> mid is ALARM -> top is ALARM
	assert.Equal(t, "ALARM", composites.Data[0].StateValue)
}

func TestCloudWatchBackend_PutCompositeAlarm_UpdateExisting(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(
		t,
		b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "child-update", StateValue: "ALARM"}),
	)

	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "comp-update",
		AlarmRule: `ALARM("child-update")`,
	}))

	// Update with a new description.
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName:        "comp-update",
		AlarmRule:        `ALARM("child-update")`,
		AlarmDescription: "updated",
	}))

	_, composites, _, err := b.DescribeAlarms(
		[]string{"comp-update"},
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
	assert.Equal(t, "updated", composites.Data[0].AlarmDescription)
}

func TestCloudWatchBackend_EnableDisableAlarmActions_CompositeAlarm(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName:      "comp-enable-disable",
		AlarmRule:      `ALARM("x")`,
		ActionsEnabled: false,
	}))

	require.NoError(t, b.EnableAlarmActions([]string{"comp-enable-disable"}))

	_, composites, _, err := b.DescribeAlarms(
		[]string{"comp-enable-disable"}, []string{"CompositeAlarm"}, "", "", "", 0,
		"", "", "")
	require.NoError(t, err)
	require.Len(t, composites.Data, 1)
	assert.True(t, composites.Data[0].ActionsEnabled)

	require.NoError(t, b.DisableAlarmActions([]string{"comp-enable-disable"}))

	_, composites2, _, err2 := b.DescribeAlarms(
		[]string{"comp-enable-disable"}, []string{"CompositeAlarm"}, "", "", "", 0,
		"", "", "")
	require.NoError(t, err2)
	require.Len(t, composites2.Data, 1)
	assert.False(t, composites2.Data[0].ActionsEnabled)
}

func TestCloudWatchBackend_CompositeAlarm_CircularDependency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T, b *cloudwatch.InMemoryBackend)
		alarmName string
		wantState string
	}{
		{
			name: "self_reference",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				// Composite alarm referencing itself: A -> ALARM("A")
				require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
					AlarmName: "self-ref",
					AlarmRule: `ALARM("self-ref")`,
				}))
			},
			alarmName: "self-ref",
			// Self-reference: the alarm doesn't exist when first evaluated, so rule
			// evaluates to OK. The cycle guard prevents infinite recursion on reevaluation.
			wantState: "OK",
		},
		{
			name: "mutual_dependency",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				// A -> ALARM("B"), B -> ALARM("A")
				require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
					AlarmName: "cycle-a",
					AlarmRule: `ALARM("cycle-b")`,
				}))
				require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
					AlarmName: "cycle-b",
					AlarmRule: `ALARM("cycle-a")`,
				}))
			},
			alarmName: "cycle-a",
			// Mutual dependency: cycle-b doesn't exist when cycle-a is first evaluated,
			// so the rule evaluates to OK. The cycle guard prevents infinite recursion.
			wantState: "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			tt.setup(t, b)

			_, composites, _, err := b.DescribeAlarms(
				[]string{tt.alarmName},
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
			assert.Equal(t, tt.wantState, composites.Data[0].StateValue)
		})
	}
}

func TestCloudWatchBackend_CompositeAlarm_CircularDependency_ReevaluationNoPanic(t *testing.T) {
	t.Parallel()

	// Verify that reevaluateCompositeAlarms triggered by SetAlarmState does not
	// panic or deadlock when composite alarms contain circular references.
	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:  "trigger",
		StateValue: "OK",
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "cycle-x",
		AlarmRule: `ALARM("cycle-y")`,
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "cycle-y",
		AlarmRule: `ALARM("cycle-x")`,
	}))

	// SetAlarmState triggers reevaluateCompositeAlarms; must not hang.
	require.NoError(t, b.SetAlarmState(t.Context(), "trigger", "ALARM", "test", ""))
}
