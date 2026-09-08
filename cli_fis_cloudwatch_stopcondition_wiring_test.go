package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cwbackend "github.com/blackbirdworks/gopherstack/services/cloudwatch"
	fisbackend "github.com/blackbirdworks/gopherstack/services/fis"
)

// fisWiringRequest drives fisbackend.Handler exactly the way the real router
// does, without needing package fis's unexported request DTOs.
func fisWiringRequest(t *testing.T, h *fisbackend.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte

	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyBytes = b
	}

	e := echo.New()
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))

	return rec
}

// TestWireFISStopConditions_AlarmStopsRealExperiment exercises the exact
// composition-root wiring cli.go's setup uses (wireFISStopConditions) against
// REAL FIS and CloudWatch backends -- not test doubles -- proving that an
// experiment's "aws:cloudwatch:alarm" stop condition actually stops the
// experiment once the wired CloudWatch alarm transitions to ALARM
// (gopherstack-x842, gopherstack-9939).
func TestWireFISStopConditions_AlarmStopsRealExperiment(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		fisBk := fisbackend.NewInMemoryBackend("000000000000", "us-east-1")
		fisH := fisbackend.NewHandler(fisBk)
		fisH.DefaultRegion = "us-east-1"
		fisH.AccountID = "000000000000"

		cwBk := cwbackend.NewInMemoryBackend()
		cwH := cwbackend.NewHandler(cwBk)

		// This is the exact call cli.go's setup makes.
		wireFISStopConditions(fisH, cwH)

		require.NoError(t, cwBk.PutMetricAlarm(&cwbackend.MetricAlarm{AlarmName: "wiring-alarm"}))

		alarms, _, _, err := cwBk.DescribeAlarms([]string{"wiring-alarm"}, nil, "", "", "", 0, "", "", "")
		require.NoError(t, err)
		require.Len(t, alarms.Data, 1)
		alarmArn := alarms.Data[0].AlarmArn

		tplBody := map[string]any{
			"roleArn": "arn:aws:iam::000000000000:role/FISRole",
			"stopConditions": []map[string]any{
				{"source": "aws:cloudwatch:alarm", "value": alarmArn},
			},
			"targets": map[string]any{},
			// A long-running action keeps the experiment "running" well past the
			// 1-second sleep below, instead of racing fis's own internal
			// no-action grace period (not exported outside its own test binary).
			"actions": map[string]any{
				"wait": map[string]any{
					"actionId":   "aws:fis:wait",
					"parameters": map[string]string{"duration": "PT30S"},
				},
			},
		}

		tplRec := fisWiringRequest(t, fisH, http.MethodPost, "/experimentTemplates", tplBody)
		require.Equal(t, http.StatusCreated, tplRec.Code)

		var tplResp struct {
			ExperimentTemplate struct {
				ID string `json:"id"`
			} `json:"experimentTemplate"`
		}
		require.NoError(t, json.Unmarshal(tplRec.Body.Bytes(), &tplResp))

		expRec := fisWiringRequest(t, fisH, http.MethodPost, "/experiments", map[string]any{
			"experimentTemplateId": tplResp.ExperimentTemplate.ID,
		})
		require.Equal(t, http.StatusCreated, expRec.Code)

		var expResp struct {
			Experiment struct {
				ID string `json:"id"`
			} `json:"experiment"`
		}
		require.NoError(t, json.Unmarshal(expRec.Body.Bytes(), &expResp))
		expID := expResp.Experiment.ID

		// Advance past the pending -> initiating -> running transition. This
		// duration is virtual (inside the synctest bubble) and costs no real
		// wall-clock time; it only needs to comfortably exceed fis's internal
		// lifecycleDelay, which is not exported outside the fis test binary.
		time.Sleep(time.Second)

		exp, err := fisBk.GetExperiment(expID)
		require.NoError(t, err)
		require.Equal(t, "running", exp.Status.Status,
			"must be running before the alarm fires, or stopping it proves nothing")

		require.NoError(t, cwBk.SetAlarmState(t.Context(), "wiring-alarm", "ALARM", "breach", ""))

		synctest.Wait()

		exp, err = fisBk.GetExperiment(expID)
		require.NoError(t, err)
		assert.Equal(t, "stopped", exp.Status.Status,
			"wireFISStopConditions must connect the real CloudWatch alarm to the real FIS experiment")
	})
}

// TestWireFISStopConditions_MissingCloudWatch_NoOp proves wireFISStopConditions
// is a silent no-op when CloudWatch isn't registered, leaving FIS unaffected --
// the same permissive-unwired-direction guarantee every other optional wiring
// call in cli.go makes.
func TestWireFISStopConditions_MissingCloudWatch_NoOp(t *testing.T) {
	t.Parallel()

	fisBk := fisbackend.NewInMemoryBackend("000000000000", "us-east-1")
	fisH := fisbackend.NewHandler(fisBk)

	require.NotPanics(t, func() {
		wireFISStopConditions(fisH, nil)
	})
}
