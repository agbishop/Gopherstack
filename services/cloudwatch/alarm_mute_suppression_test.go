package cloudwatch_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

// TestSetAlarmState_MutedAlarm_SuppressesActions drives the backend directly:
// a mute rule targeting the alarm and covering the current instant must stop
// SetAlarmState from dispatching SNS/Lambda actions, while still applying the
// state transition itself ("the targeted alarms continue to evaluate metrics
// and transition between states, but their configured actions are muted",
// botocore cloudwatch 2010-08-01 service-2.json MuteTargets shape).
func TestSetAlarmState_MutedAlarm_SuppressesActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		muteTargets    []string
		wantSuppressed bool
	}{
		{name: "targeted_alarm_is_muted", muteTargets: []string{"cpu-alarm"}, wantSuppressed: true},
		{name: "other_alarm_is_not_muted", muteTargets: []string{"unrelated-alarm"}, wantSuppressed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			sns := &mockSNSPublisher{}
			b.SetSNSPublisher(sns)

			require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
				AlarmName:      "cpu-alarm",
				StateValue:     "OK",
				ActionsEnabled: true,
				AlarmActions:   []string{"arn:aws:sns:us-east-1:123456789012:topic"},
			}))

			// A one-time window covering "now": current wall-clock minute plus
			// a generous duration, so the rule is active without any sleep.
			atExpr := "at(" + time.Now().UTC().Format("2006-01-02T15:04") + ")"
			require.NoError(t, b.PutAlarmMuteRule(&cloudwatch.AlarmMuteRule{
				Name:       "maintenance",
				AlarmNames: tt.muteTargets,
				Schedule:   cloudwatch.AlarmMuteRuleSchedule{Expression: atExpr, Duration: "PT1H"},
			}))

			require.NoError(t, b.SetAlarmState(t.Context(), "cpu-alarm", "ALARM", "test", ""))

			alarms, _, _, err := b.DescribeAlarms([]string{"cpu-alarm"}, nil, "", "", "", 0, "", "", "")
			require.NoError(t, err)
			require.Len(t, alarms.Data, 1)
			assert.Equal(t, "ALARM", alarms.Data[0].StateValue, "state transition must apply regardless of muting")

			if tt.wantSuppressed {
				assert.Empty(t, sns.messages, "muted alarm must not dispatch actions")
			} else {
				assert.Len(t, sns.messages, 1, "unmuted alarm must dispatch actions")
			}
		})
	}
}

// TestSDK_SetAlarmState_MutedAlarm_SuppressesActions drives PutAlarmMuteRule
// and SetAlarmState through a real aws-sdk-go-v2 client (rpc-v2 CBOR, the
// only protocol this SDK's cloudwatch client speaks) and checks the wired
// SNS fake directly, since the client itself cannot observe suppression.
func TestSDK_SetAlarmState_MutedAlarm_SuppressesActions(t *testing.T) {
	t.Parallel()

	client, backend := newTestHandlerAndClientWithBackend(t)
	sns := &mockSNSPublisher{}
	backend.SetSNSPublisher(sns)

	ctx := t.Context()

	require.NoError(t, backend.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:      "sdk-cpu-alarm",
		StateValue:     "OK",
		ActionsEnabled: true,
		AlarmActions:   []string{"arn:aws:sns:us-east-1:123456789012:topic"},
	}))

	atExpr := "at(" + time.Now().UTC().Format("2006-01-02T15:04") + ")"

	_, err := client.PutAlarmMuteRule(ctx, &cwsdk.PutAlarmMuteRuleInput{
		Name: aws.String("sdk-maintenance"),
		Rule: &cwtypes.Rule{
			Schedule: &cwtypes.Schedule{
				Expression: aws.String(atExpr),
				Duration:   aws.String("PT1H"),
			},
		},
		MuteTargets: &cwtypes.MuteTargets{AlarmNames: []string{"sdk-cpu-alarm"}},
	})
	require.NoError(t, err)

	_, err = client.SetAlarmState(ctx, &cwsdk.SetAlarmStateInput{
		AlarmName:   aws.String("sdk-cpu-alarm"),
		StateValue:  cwtypes.StateValueAlarm,
		StateReason: aws.String("integration test"),
	})
	require.NoError(t, err)

	out, err := client.DescribeAlarms(ctx, &cwsdk.DescribeAlarmsInput{
		AlarmNames: []string{"sdk-cpu-alarm"},
	})
	require.NoError(t, err)
	require.Len(t, out.MetricAlarms, 1)
	assert.Equal(t, cwtypes.StateValueAlarm, out.MetricAlarms[0].StateValue,
		"state transition must apply regardless of muting")

	assert.Empty(t, sns.messages, "muted alarm reached through the real SDK client must not dispatch actions")
}

// TestQueryProtocol_SetAlarmState_MutedAlarm_SuppressesActions drives
// PutAlarmMuteRule and SetAlarmState over the form-encoded query protocol
// botocore still advertises for cloudwatch alongside rpc-v2 CBOR and json --
// no real Go SDK client can speak it for this service, so postForm (this
// package's established stand-in, see handler_alarm_mute_rules_test.go) is
// the real-surface proxy.
func TestQueryProtocol_SetAlarmState_MutedAlarm_SuppressesActions(t *testing.T) {
	t.Parallel()

	h, backend := newCWHandlerWithBackend()
	sns := &mockSNSPublisher{}
	backend.SetSNSPublisher(sns)

	require.NoError(t, backend.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:      "query-cpu-alarm",
		StateValue:     "OK",
		ActionsEnabled: true,
		AlarmActions:   []string{"arn:aws:sns:us-east-1:123456789012:topic"},
	}))

	atExpr := "at(" + time.Now().UTC().Format("2006-01-02T15:04") + ")"

	putRec := postForm(t, h, url.Values{
		"Action":                          []string{"PutAlarmMuteRule"},
		"Name":                            []string{"query-maintenance"},
		"Rule.Schedule.Expression":        []string{atExpr},
		"Rule.Schedule.Duration":          []string{"PT1H"},
		"MuteTargets.AlarmNames.member.1": []string{"query-cpu-alarm"},
	}.Encode())
	require.Equal(t, 200, putRec.Code, putRec.Body.String())

	setRec := postForm(t, h, url.Values{
		"Action":      []string{"SetAlarmState"},
		"AlarmName":   []string{"query-cpu-alarm"},
		"StateValue":  []string{"ALARM"},
		"StateReason": []string{"integration test"},
	}.Encode())
	require.Equal(t, 200, setRec.Code, setRec.Body.String())

	alarms, _, _, err := backend.DescribeAlarms([]string{"query-cpu-alarm"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)
	assert.Equal(t, "ALARM", alarms.Data[0].StateValue, "state transition must apply regardless of muting")

	assert.Empty(t, sns.messages, "muted alarm reached through the query protocol must not dispatch actions")
}
