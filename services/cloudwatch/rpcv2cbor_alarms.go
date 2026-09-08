package cloudwatch

import (
	"errors"
	"net/http"

	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/labstack/echo/v5"
)

func (h *Handler) cborPutMetricAlarm(input cbor.Map, c *echo.Context) error {
	alarmName := cborStr(input, "AlarmName")
	if alarmName == "" {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"AlarmName is required",
		)
	}

	actionsEnabled := true
	if v, ok := input["ActionsEnabled"]; ok {
		if b, isBool := v.(cbor.Bool); isBool {
			actionsEnabled = bool(b)
		}
	}

	alarm := &MetricAlarm{
		AlarmName:               alarmName,
		Namespace:               cborStr(input, keyNamespace),
		MetricName:              cborStr(input, keyMetricName),
		ComparisonOperator:      cborStr(input, "ComparisonOperator"),
		Statistic:               cborStr(input, "Statistic"),
		ExtendedStatistic:       cborStr(input, "ExtendedStatistic"),
		TreatMissingData:        cborStr(input, "TreatMissingData"),
		AlarmDescription:        cborStr(input, "AlarmDescription"),
		Threshold:               cborFloat(input, "Threshold"),
		EvaluationPeriods:       cborInt32(input, "EvaluationPeriods"),
		DatapointsToAlarm:       cborInt32(input, "DatapointsToAlarm"),
		Period:                  cborInt32(input, "Period"),
		ActionsEnabled:          actionsEnabled,
		AlarmActions:            cborStrList(input, "AlarmActions"),
		OKActions:               cborStrList(input, "OKActions"),
		InsufficientDataActions: cborStrList(input, "InsufficientDataActions"),
		Dimensions:              cborDimensions(input),
		Metrics:                 parseMetricDataQueries(input, "Metrics"),
	}

	if err := h.Backend.PutMetricAlarm(alarm); err != nil {
		if errors.Is(err, ErrValidation) {
			return h.cborError(c, http.StatusBadRequest, "InvalidParameterValueException", err.Error())
		}

		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	h.applyCreationTags(input, alarm.AlarmArn)

	return writeCBOR(c, cbor.Map{})
}

func (h *Handler) cborDescribeAlarms(input cbor.Map, c *echo.Context) error {
	alarmNames := cborStrList(input, "AlarmNames")
	alarmTypes := cborStrList(input, "AlarmTypes")
	alarmNamePrefix := cborStr(input, "AlarmNamePrefix")
	stateValue := cborStr(input, keyStateValue)
	nextToken := cborStr(input, "NextToken")
	maxRecords := int(cborInt32(input, "MaxRecords"))
	actionPrefix := cborStr(input, "ActionPrefix")
	childrenOfAlarmName := cborStr(input, "ChildrenOfAlarmName")
	parentsOfAlarmName := cborStr(input, "ParentsOfAlarmName")

	metricPage, compositePage, logPage, err := h.Backend.DescribeAlarms(
		alarmNames,
		alarmTypes,
		alarmNamePrefix,
		stateValue,
		nextToken,
		maxRecords,
		actionPrefix,
		childrenOfAlarmName,
		parentsOfAlarmName,
	)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return h.cborError(c, http.StatusBadRequest, "InvalidParameterValueException", err.Error())
		}

		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	alarmList := make(cbor.List, 0, len(metricPage.Data))
	for i := range metricPage.Data {
		alarmList = append(alarmList, buildMetricAlarmCBOR(&metricPage.Data[i]))
	}

	compositeList := make(cbor.List, 0, len(compositePage.Data))
	for i := range compositePage.Data {
		compositeList = append(compositeList, buildCompositeAlarmCBOR(&compositePage.Data[i]))
	}

	logList := make(cbor.List, 0, len(logPage.Data))
	for i := range logPage.Data {
		logList = append(logList, buildLogAlarmCBOR(&logPage.Data[i]))
	}

	resp := cbor.Map{
		"MetricAlarms":    alarmList,
		"CompositeAlarms": compositeList,
		"LogAlarms":       logList,
	}

	nextTok := metricPage.Next
	if nextTok == "" {
		nextTok = compositePage.Next
	}
	if nextTok == "" {
		nextTok = logPage.Next
	}
	if nextTok != "" {
		resp["NextToken"] = cbor.String(nextTok)
	}

	return writeCBOR(c, resp)
}

func (h *Handler) cborDeleteAlarms(input cbor.Map, c *echo.Context) error {
	alarmNames := cborStrList(input, "AlarmNames")

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		for _, a := range b.GetAlarmARNs(alarmNames) {
			h.deleteResourceTags(a)
		}
	}

	if err := h.Backend.DeleteAlarms(alarmNames); err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, cbor.Map{})
}

// buildMetricAlarmCBOR converts a MetricAlarm to a cbor.Map.
func buildMetricAlarmCBOR(a *MetricAlarm) cbor.Map {
	m := cbor.Map{
		keyAlarmName:         cbor.String(a.AlarmName),
		keyAlarmArn:          cbor.String(a.AlarmArn),
		keyAlarmType:         cbor.String("MetricAlarm"),
		keyNamespace:         cbor.String(a.Namespace),
		keyMetricName:        cbor.String(a.MetricName),
		"ComparisonOperator": cbor.String(a.ComparisonOperator),
		"Statistic":          cbor.String(a.Statistic),
		keyStateValue:        cbor.String(a.StateValue),
		keyStateReason:       cbor.String(a.StateReason),
		keyAlarmDescription:  cbor.String(a.AlarmDescription),
		"Threshold":          cbor.Float64(a.Threshold),
		"EvaluationPeriods": cbor.Uint(
			uint64(a.EvaluationPeriods), //nolint:gosec // EvaluationPeriods is positive
		),
		"Period": cbor.Uint(
			uint64(a.Period), //nolint:gosec // Period is positive
		),
		keyActionsEnabled: cbor.Bool(a.ActionsEnabled),
	}
	if !a.StateTransitionedTimestamp.IsZero() {
		m["StateTransitionedTimestamp"] = cborFromTime(a.StateTransitionedTimestamp)
	}
	if !a.AlarmConfigurationUpdatedTimestamp.IsZero() {
		m["AlarmConfigurationUpdatedTimestamp"] = cborFromTime(a.AlarmConfigurationUpdatedTimestamp)
	}
	if !a.CreatedAt.IsZero() {
		m["AlarmCreatedAt"] = cborFromTime(a.CreatedAt)
	}
	if a.StateReasonData != "" {
		m["StateReasonData"] = cbor.String(a.StateReasonData)
	}
	if a.DatapointsToAlarm > 0 {
		m["DatapointsToAlarm"] = cbor.Uint(uint64(a.DatapointsToAlarm))
	}
	if a.TreatMissingData != "" {
		m["TreatMissingData"] = cbor.String(a.TreatMissingData)
	}
	if a.ExtendedStatistic != "" {
		m["ExtendedStatistic"] = cbor.String(a.ExtendedStatistic)
	}
	if len(a.Dimensions) > 0 {
		dims := make(cbor.List, 0, len(a.Dimensions))
		for _, d := range a.Dimensions {
			dims = append(dims, cbor.Map{
				keyName:  cbor.String(d.Name),
				keyValue: cbor.String(d.Value),
			})
		}
		m["Dimensions"] = dims
	}
	if len(a.AlarmActions) > 0 {
		m["AlarmActions"] = cborStringList(a.AlarmActions)
	}
	if len(a.OKActions) > 0 {
		m["OKActions"] = cborStringList(a.OKActions)
	}
	if len(a.InsufficientDataActions) > 0 {
		m["InsufficientDataActions"] = cborStringList(a.InsufficientDataActions)
	}
	if len(a.Metrics) > 0 {
		m["Metrics"] = buildMetricDataQueriesCBOR(a.Metrics)
	}

	return m
}

// buildMetricDataQueriesCBOR converts a MetricDataQuery list to the wire
// shape PutMetricAlarmInput's "Metrics" member shares with GetMetricDataInput's
// "MetricDataQueries" member (both the _MetricDataQueries shape in
// cloudwatch@v1.66.3 schemas.go), for DescribeAlarms to echo back what
// PutMetricAlarm stored.
func buildMetricDataQueriesCBOR(queries []MetricDataQuery) cbor.List {
	list := make(cbor.List, 0, len(queries))

	for _, q := range queries {
		qm := cbor.Map{
			"Id":         cbor.String(q.ID),
			"ReturnData": cbor.Bool(q.ReturnData),
		}
		if q.Label != "" {
			qm["Label"] = cbor.String(q.Label)
		}
		if q.Expression != "" {
			qm["Expression"] = cbor.String(q.Expression)
		}
		if q.AccountID != "" {
			qm["AccountId"] = cbor.String(q.AccountID)
		}
		if q.MetricStat.MetricName != "" || q.MetricStat.Namespace != "" {
			metric := cbor.Map{
				keyNamespace:  cbor.String(q.MetricStat.Namespace),
				keyMetricName: cbor.String(q.MetricStat.MetricName),
			}
			if len(q.MetricStat.Dimensions) > 0 {
				dims := make(cbor.List, 0, len(q.MetricStat.Dimensions))
				for _, d := range q.MetricStat.Dimensions {
					dims = append(dims, cbor.Map{
						keyName:  cbor.String(d.Name),
						keyValue: cbor.String(d.Value),
					})
				}
				metric["Dimensions"] = dims
			}
			qm["MetricStat"] = cbor.Map{
				"Metric": metric,
				"Period": cbor.Uint(uint64(q.MetricStat.Period)), //nolint:gosec // Period is positive
				"Stat":   cbor.String(q.MetricStat.Stat),
			}
		}

		list = append(list, qm)
	}

	return list
}

func (h *Handler) cborDescribeAlarmsForMetric(input cbor.Map, c *echo.Context) error {
	namespace := cborStr(input, keyNamespace)
	metricName := cborStr(input, keyMetricName)
	dimensions := cborDimensions(input)
	alarmNames := cborStrList(input, "AlarmNames")
	nextToken := cborStr(input, "NextToken")
	maxRecords := int(cborInt32(input, "MaxRecords"))

	p, err := h.Backend.DescribeAlarmsForMetric(
		namespace,
		metricName,
		dimensions,
		alarmNames,
		nextToken,
		maxRecords,
	)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	alarmList := make(cbor.List, 0, len(p.Data))
	for i := range p.Data {
		alarmList = append(alarmList, buildMetricAlarmCBOR(&p.Data[i]))
	}

	resp := cbor.Map{"MetricAlarms": alarmList}
	if p.Next != "" {
		resp["NextToken"] = cbor.String(p.Next)
	}

	return writeCBOR(c, resp)
}

func (h *Handler) cborSetAlarmState(input cbor.Map, c *echo.Context) error {
	alarmName := cborStr(input, "AlarmName")
	if alarmName == "" {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"AlarmName is required",
		)
	}

	if err := h.Backend.SetAlarmState(
		c.Request().Context(),
		alarmName,
		cborStr(input, keyStateValue),
		cborStr(input, "StateReason"),
		cborStr(input, "StateReasonData"),
	); err != nil {
		return h.cborError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	return writeCBOR(c, cbor.Map{})
}

func (h *Handler) cborEnableAlarmActions(input cbor.Map, c *echo.Context) error {
	if err := h.Backend.EnableAlarmActions(cborStrList(input, "AlarmNames")); err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, cbor.Map{})
}

func (h *Handler) cborDisableAlarmActions(input cbor.Map, c *echo.Context) error {
	if err := h.Backend.DisableAlarmActions(cborStrList(input, "AlarmNames")); err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, cbor.Map{})
}
