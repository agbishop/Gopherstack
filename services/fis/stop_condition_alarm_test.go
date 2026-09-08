package fis_test

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
	"github.com/blackbirdworks/gopherstack/services/fis"
)

// newStopConditionExperiment creates a template whose single stop condition
// names alarmArn, plus one with no actions or targets, starts an experiment
// from it, and returns the experiment's ID. It advances the synctest fake
// clock just enough for the experiment to reach "running".
func newStopConditionExperiment(t *testing.T, h *fis.Handler, alarmArn string) string {
	t.Helper()

	body := map[string]any{
		"roleArn": "arn:aws:iam::000000000000:role/FISRole",
		"stopConditions": []map[string]any{
			{"source": "aws:cloudwatch:alarm", "value": alarmArn},
		},
		"targets": map[string]any{},
		"actions": map[string]any{},
	}

	rec := doRequest(t, h, http.MethodPost, "/experimentTemplates", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	var tplResp struct {
		ExperimentTemplate struct {
			ID string `json:"id"`
		} `json:"experimentTemplate"`
	}
	mustJSON(t, rec, &tplResp)

	rec2 := doRequest(t, h, http.MethodPost, "/experiments", map[string]any{
		"experimentTemplateId": tplResp.ExperimentTemplate.ID,
	})
	require.Equal(t, http.StatusCreated, rec2.Code)

	var expResp struct {
		Experiment struct {
			ID string `json:"id"`
		} `json:"experiment"`
	}
	mustJSON(t, rec2, &expResp)

	// Advance past the pending -> initiating -> running transition. Strictly
	// longer than the transition delay: two timers due at the same fake
	// instant have no guaranteed fire order.
	time.Sleep(fis.LifecycleDelayForTest + time.Millisecond)

	return expResp.Experiment.ID
}

func newTestHandlerWithAlarm(t *testing.T) (*fis.Handler, *fis.InMemoryBackend, *cloudwatch.InMemoryBackend, string) {
	t.Helper()

	backend := fis.NewTestBackend()
	h := fis.NewHandler(backend)
	h.DefaultRegion = "us-east-1"
	h.AccountID = "000000000000"

	cw := cloudwatch.NewInMemoryBackend()
	require.NoError(t, cw.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "stop-alarm"}))

	alarms, _, _, err := cw.DescribeAlarms([]string{"stop-alarm"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)

	return h, backend, cw, alarms.Data[0].AlarmArn
}

// TestFISStopCondition_AlarmToAlarm_StopsExperiment is the regression test for
// gopherstack-x842 / gopherstack-9939: an experiment with a stop condition
// naming a CloudWatch alarm must stop once that alarm transitions to ALARM.
func TestFISStopCondition_AlarmToAlarm_StopsExperiment(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h, backend, cw, alarmArn := newTestHandlerWithAlarm(t)
		backend.SetAlarmStateSubscriber(cw)

		expID := newStopConditionExperiment(t, h, alarmArn)

		exp, err := backend.GetExperiment(expID)
		require.NoError(t, err)
		require.Equal(t, "running", exp.Status.Status,
			"must be running before the alarm fires, or stopping it proves nothing")

		require.NoError(t, cw.SetAlarmState(t.Context(), "stop-alarm", "ALARM", "breach", ""))

		synctest.Wait()

		exp, err = backend.GetExperiment(expID)
		require.NoError(t, err)
		assert.Equal(t, "stopped", exp.Status.Status)
	})
}

// TestFISStopCondition_AlarmToOK_DoesNotStopExperiment proves an alarm
// transition to a state other than ALARM does not stop the experiment --
// otherwise any state change would look like a working stop condition.
func TestFISStopCondition_AlarmToOK_DoesNotStopExperiment(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h, backend, cw, alarmArn := newTestHandlerWithAlarm(t)
		backend.SetAlarmStateSubscriber(cw)

		expID := newStopConditionExperiment(t, h, alarmArn)

		// The alarm starts INSUFFICIENT_DATA (PutMetricAlarm's default); OK is a
		// real transition, exercising the subscription without it being ALARM.
		require.NoError(t, cw.SetAlarmState(t.Context(), "stop-alarm", "OK", "resolved", ""))

		exp, err := backend.GetExperiment(expID)
		require.NoError(t, err)
		assert.Equal(t, "running", exp.Status.Status)

		// synctest requires every goroutine the bubble started to have exited
		// before the bubble's root function returns; stop the experiment we
		// deliberately left running to end its background goroutine cleanly.
		_, err = backend.StopExperiment(expID)
		require.NoError(t, err)
		synctest.Wait()
	})
}

// TestFISStopCondition_CloudWatchUnwired_ExperimentUnaffected proves the
// unwired direction stays permissive: when nothing calls
// SetAlarmStateSubscriber, an "aws:cloudwatch:alarm" stop condition is
// accepted and the experiment runs exactly as it did before this feature.
func TestFISStopCondition_CloudWatchUnwired_ExperimentUnaffected(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h, backend, _, alarmArn := newTestHandlerWithAlarm(t)
		// Deliberately never call backend.SetAlarmStateSubscriber.

		expID := newStopConditionExperiment(t, h, alarmArn)

		exp, err := backend.GetExperiment(expID)
		require.NoError(t, err)
		assert.Equal(t, "running", exp.Status.Status)

		// Advance past the no-action grace period: the experiment completes
		// normally, undisturbed by the alarm it can never hear about.
		time.Sleep(fis.LifecycleDelayForTest + time.Millisecond)
		synctest.Wait()

		exp, err = backend.GetExperiment(expID)
		require.NoError(t, err)
		assert.Equal(t, "completed", exp.Status.Status)
	})
}
