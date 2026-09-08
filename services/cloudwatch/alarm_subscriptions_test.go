package cloudwatch_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

func alarmArnForTest(t *testing.T, b *cloudwatch.InMemoryBackend, name string) string {
	t.Helper()

	alarms, _, _, err := b.DescribeAlarms([]string{name}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)

	return alarms.Data[0].AlarmArn
}

// TestSubscribeAlarmStateChange_FiresOnTransition is the regression test for
// gopherstack-x842 / gopherstack-9939 at the CloudWatch layer: a subscriber
// registered by ARN must be notified with the new state whenever the alarm
// actually transitions.
func TestSubscribeAlarmStateChange_FiresOnTransition(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "sub-alarm"}))
	alarmArn := alarmArnForTest(t, b, "sub-alarm")

	var got []string
	unsub := b.SubscribeAlarmStateChange(alarmArn, func(newState string) {
		got = append(got, newState)
	})
	defer unsub()

	require.NoError(t, b.SetAlarmState(t.Context(), "sub-alarm", "ALARM", "breach", ""))
	require.NoError(t, b.SetAlarmState(t.Context(), "sub-alarm", "OK", "recovered", ""))

	assert.Equal(t, []string{"ALARM", "OK"}, got)
}

// TestSubscribeAlarmStateChange_NoFireWhenStateUnchanged proves the subscriber
// is only notified on an actual transition, not on every SetAlarmState call.
func TestSubscribeAlarmStateChange_NoFireWhenStateUnchanged(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "steady-alarm", StateValue: "OK",
	}))
	alarmArn := alarmArnForTest(t, b, "steady-alarm")

	calls := 0
	unsub := b.SubscribeAlarmStateChange(alarmArn, func(string) { calls++ })
	defer unsub()

	require.NoError(t, b.SetAlarmState(t.Context(), "steady-alarm", "OK", "still fine", ""))
	assert.Equal(t, 0, calls)
}

// TestSubscribeAlarmStateChange_Unsubscribe proves the returned unsubscribe
// func actually stops delivery.
func TestSubscribeAlarmStateChange_Unsubscribe(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "unsub-alarm"}))
	alarmArn := alarmArnForTest(t, b, "unsub-alarm")

	calls := 0
	unsub := b.SubscribeAlarmStateChange(alarmArn, func(string) { calls++ })

	require.NoError(t, b.SetAlarmState(t.Context(), "unsub-alarm", "ALARM", "breach", ""))
	require.Equal(t, 1, calls)

	unsub()

	require.NoError(t, b.SetAlarmState(t.Context(), "unsub-alarm", "OK", "recovered", ""))
	assert.Equal(t, 1, calls, "no further callback after unsubscribe")
}

// TestSubscribeAlarmStateChange_NoSubscribers_Unaffected proves the unwired
// direction stays permissive: with zero subscribers (the default), SetAlarmState
// behaves exactly as it did before this feature existed.
func TestSubscribeAlarmStateChange_NoSubscribers_Unaffected(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "lonely-alarm"}))

	require.NoError(t, b.SetAlarmState(t.Context(), "lonely-alarm", "ALARM", "breach", ""))

	alarms, _, _, err := b.DescribeAlarms([]string{"lonely-alarm"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)
	assert.Equal(t, "ALARM", alarms.Data[0].StateValue)
}

// TestSubscribeAlarmStateChange_CallbackRunsAfterLockReleased is the regression
// test for the "must not call into another backend while holding cloudwatch's
// lock" design constraint (gopherstack-9939): the callback re-enters the same
// backend, which would deadlock if it ran while b.mu was still held by
// SetAlarmState. SetAlarmState is run on a goroutine and raced against a
// timeout so a regression hangs instead of blocking the test suite forever.
func TestSubscribeAlarmStateChange_CallbackRunsAfterLockReleased(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "reentrant-alarm"}))
	alarmArn := alarmArnForTest(t, b, "reentrant-alarm")

	var reentrantErr error

	unsub := b.SubscribeAlarmStateChange(alarmArn, func(string) {
		_, _, _, err := b.DescribeAlarms([]string{"reentrant-alarm"}, nil, "", "", "", 0, "", "", "")
		reentrantErr = err
	})
	defer unsub()

	done := make(chan error, 1)
	go func() {
		done <- b.SetAlarmState(t.Context(), "reentrant-alarm", "ALARM", "breach", "")
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("SetAlarmState did not return: the subscriber callback likely ran " +
			"while b.mu was still held, deadlocking the re-entrant DescribeAlarms call")
	}

	require.NoError(t, reentrantErr)
}
