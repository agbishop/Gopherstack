package cloudwatch

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func (h *Handler) handlePutMetricAlarm(form url.Values, c *echo.Context) error {
	alarmName := form.Get("AlarmName")
	if alarmName == "" {
		return h.xmlError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"AlarmName is required",
		)
	}

	threshold, _ := strconv.ParseFloat(form.Get("Threshold"), 64)
	evalPeriods, _ := strconv.ParseInt(form.Get("EvaluationPeriods"), 10, 32)
	datapointsToAlarm, _ := strconv.ParseInt(form.Get("DatapointsToAlarm"), 10, 32)
	period, _ := strconv.ParseInt(form.Get("Period"), 10, 32)
	actionsEnabled := form.Get("ActionsEnabled") != formFalse

	alarm := &MetricAlarm{
		AlarmName:               alarmName,
		Namespace:               form.Get("Namespace"),
		MetricName:              form.Get("MetricName"),
		ComparisonOperator:      form.Get("ComparisonOperator"),
		Statistic:               form.Get("Statistic"),
		ExtendedStatistic:       form.Get("ExtendedStatistic"),
		TreatMissingData:        form.Get("TreatMissingData"),
		AlarmDescription:        form.Get("AlarmDescription"),
		ThresholdMetricID:       form.Get("ThresholdMetricId"),
		Threshold:               threshold,
		EvaluationPeriods:       int32(evalPeriods),
		DatapointsToAlarm:       int32(datapointsToAlarm),
		Period:                  int32(period),
		ActionsEnabled:          actionsEnabled,
		AlarmActions:            parseMemberList(form, "AlarmActions."),
		OKActions:               parseMemberList(form, "OKActions."),
		InsufficientDataActions: parseMemberList(form, "InsufficientDataActions."),
		Dimensions:              parseDimensionsFromForm(form, "Dimensions."),
		Metrics:                 parseMetricDataQueriesFromForm(form),
	}
	if err := h.Backend.PutMetricAlarm(alarm); err != nil {
		if errors.Is(err, ErrValidation) {
			return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
		}

		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name `xml:"PutMetricAlarmResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}

// metricAlarmToXML converts a MetricAlarm to its XML representation.
func metricAlarmToXML(a MetricAlarm) metricAlarmXML {
	x := metricAlarmXML{
		AlarmName:               a.AlarmName,
		AlarmArn:                a.AlarmArn,
		Namespace:               a.Namespace,
		MetricName:              a.MetricName,
		ComparisonOperator:      a.ComparisonOperator,
		EvaluationPeriods:       a.EvaluationPeriods,
		DatapointsToAlarm:       a.DatapointsToAlarm,
		Period:                  a.Period,
		Statistic:               a.Statistic,
		ExtendedStatistic:       a.ExtendedStatistic,
		TreatMissingData:        a.TreatMissingData,
		ThresholdMetricID:       a.ThresholdMetricID,
		Threshold:               a.Threshold,
		StateValue:              a.StateValue,
		StateReason:             a.StateReason,
		StateReasonData:         a.StateReasonData,
		AlarmDescription:        a.AlarmDescription,
		AlarmActions:            a.AlarmActions,
		OKActions:               a.OKActions,
		InsufficientDataActions: a.InsufficientDataActions,
		ActionsEnabled:          a.ActionsEnabled,
	}
	if !a.StateTransitionedTimestamp.IsZero() {
		x.StateTransitionedTimestamp = a.StateTransitionedTimestamp.UTC().Format(time.RFC3339)
	}
	if !a.AlarmConfigurationUpdatedTimestamp.IsZero() {
		x.AlarmConfigurationUpdatedTimestamp = a.AlarmConfigurationUpdatedTimestamp.UTC().
			Format(time.RFC3339)
	}
	for _, d := range a.Dimensions {
		x.Dimensions = append(x.Dimensions, struct {
			Name  string `xml:"Name"`
			Value string `xml:"Value"`
		}{Name: d.Name, Value: d.Value})
	}

	return x
}

// metricAlarmXML is the XML representation of a MetricAlarm.
type metricAlarmXML struct {
	AlarmConfigurationUpdatedTimestamp string   `xml:"AlarmConfigurationUpdatedTimestamp,omitempty"`
	StateTransitionedTimestamp         string   `xml:"StateTransitionedTimestamp,omitempty"`
	AlarmDescription                   string   `xml:"AlarmDescription,omitempty"`
	Namespace                          string   `xml:"Namespace,omitempty"`
	MetricName                         string   `xml:"MetricName,omitempty"`
	ComparisonOperator                 string   `xml:"ComparisonOperator"`
	Statistic                          string   `xml:"Statistic,omitempty"`
	ExtendedStatistic                  string   `xml:"ExtendedStatistic,omitempty"`
	TreatMissingData                   string   `xml:"TreatMissingData,omitempty"`
	ThresholdMetricID                  string   `xml:"ThresholdMetricId,omitempty"`
	AlarmArn                           string   `xml:"AlarmArn"`
	StateValue                         string   `xml:"StateValue"`
	AlarmName                          string   `xml:"AlarmName"`
	StateReason                        string   `xml:"StateReason,omitempty"`
	StateReasonData                    string   `xml:"StateReasonData,omitempty"`
	AlarmActions                       []string `xml:"AlarmActions>member,omitempty"`
	InsufficientDataActions            []string `xml:"InsufficientDataActions>member,omitempty"`
	OKActions                          []string `xml:"OKActions>member,omitempty"`
	Dimensions                         []struct {
		Name  string `xml:"Name"`
		Value string `xml:"Value"`
	} `xml:"Dimensions>member,omitempty"`
	Threshold         float64 `xml:"Threshold"`
	Period            int32   `xml:"Period,omitempty"`
	EvaluationPeriods int32   `xml:"EvaluationPeriods"`
	DatapointsToAlarm int32   `xml:"DatapointsToAlarm,omitempty"`
	ActionsEnabled    bool    `xml:"ActionsEnabled"`
}

func (h *Handler) handleDescribeAlarms(form url.Values, c *echo.Context) error {
	alarmNames := parseMemberList(form, "AlarmNames.")
	alarmTypes := parseMemberList(form, "AlarmTypes.")
	alarmNamePrefix := form.Get("AlarmNamePrefix")
	stateValue := form.Get("StateValue")
	nextToken := form.Get("NextToken")
	maxRecords, _ := strconv.Atoi(form.Get("MaxRecords"))
	actionPrefix := form.Get("ActionPrefix")
	childrenOfAlarmName := form.Get("ChildrenOfAlarmName")
	parentsOfAlarmName := form.Get("ParentsOfAlarmName")

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
			return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
		}

		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	metricMembers := make([]metricAlarmXML, 0, len(metricPage.Data))
	for _, a := range metricPage.Data {
		metricMembers = append(metricMembers, metricAlarmToXML(a))
	}

	compositeMembers := make([]compositeAlarmXMLType, 0, len(compositePage.Data))
	for _, a := range compositePage.Data {
		compositeMembers = append(compositeMembers, compositeAlarmToXML(a))
	}

	logMembers := make([]logAlarmXML, 0, len(logPage.Data))
	for _, a := range logPage.Data {
		logMembers = append(logMembers, logAlarmToXML(a))
	}

	nextTok := metricPage.Next
	if nextTok == "" {
		nextTok = compositePage.Next
	}
	if nextTok == "" {
		nextTok = logPage.Next
	}

	type descResult struct {
		NextToken       string                  `xml:"NextToken,omitempty"`
		MetricAlarms    []metricAlarmXML        `xml:"MetricAlarms>member"`
		CompositeAlarms []compositeAlarmXMLType `xml:"CompositeAlarms>member"`
		LogAlarms       []logAlarmXML           `xml:"LogAlarms>member"`
	}
	type response struct {
		XMLName   xml.Name   `xml:"DescribeAlarmsResponse"`
		Xmlns     string     `xml:"xmlns,attr"`
		RequestID string     `xml:"ResponseMetadata>RequestId"`
		Result    descResult `xml:"DescribeAlarmsResult"`
	}

	return writeXML(c, response{
		Xmlns: cloudwatchNS,
		Result: descResult{
			MetricAlarms:    metricMembers,
			CompositeAlarms: compositeMembers,
			LogAlarms:       logMembers,
			NextToken:       nextTok,
		},
		RequestID: uuid.New().String(),
	})
}

func (h *Handler) handleDeleteAlarms(form url.Values, c *echo.Context) error {
	alarmNames := parseMemberList(form, "AlarmNames.")

	// Collect alarm ARNs before deleting so we can clean up their tag entries.
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		for _, arn := range b.GetAlarmARNs(alarmNames) {
			h.deleteResourceTags(arn)
		}
	}

	if err := h.Backend.DeleteAlarms(alarmNames); err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name `xml:"DeleteAlarmsResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}

func (h *Handler) handleDescribeAlarmsForMetric(form url.Values, c *echo.Context) error {
	namespace := form.Get("Namespace")
	metricName := form.Get("MetricName")
	dimensions := parseDimensionsFromForm(form, "Dimensions.")
	alarmNames := parseMemberList(form, "AlarmNames.")
	nextToken := form.Get("NextToken")
	maxRecords, _ := strconv.Atoi(form.Get("MaxRecords"))

	p, err := h.Backend.DescribeAlarmsForMetric(
		namespace,
		metricName,
		dimensions,
		alarmNames,
		nextToken,
		maxRecords,
	)
	if err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	members := make([]metricAlarmXML, 0, len(p.Data))
	for _, a := range p.Data {
		members = append(members, metricAlarmToXML(a))
	}

	type descResult struct {
		NextToken    string           `xml:"NextToken,omitempty"`
		MetricAlarms []metricAlarmXML `xml:"MetricAlarms>member"`
	}
	type response struct {
		XMLName   xml.Name   `xml:"DescribeAlarmsForMetricResponse"`
		Xmlns     string     `xml:"xmlns,attr"`
		RequestID string     `xml:"ResponseMetadata>RequestId"`
		Result    descResult `xml:"DescribeAlarmsForMetricResult"`
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		Result:    descResult{MetricAlarms: members, NextToken: p.Next},
		RequestID: uuid.New().String(),
	})
}

func (h *Handler) handleSetAlarmState(form url.Values, c *echo.Context) error {
	alarmName := form.Get("AlarmName")
	if alarmName == "" {
		return h.xmlError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"AlarmName is required",
		)
	}
	stateValue := form.Get("StateValue")
	stateReason := form.Get("StateReason")
	stateReasonData := form.Get("StateReasonData")

	if err := h.Backend.SetAlarmState(
		c.Request().Context(),
		alarmName,
		stateValue,
		stateReason,
		stateReasonData,
	); err != nil {
		return h.xmlError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	type response struct {
		XMLName   xml.Name `xml:"SetAlarmStateResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}

func (h *Handler) handleEnableAlarmActions(form url.Values, c *echo.Context) error {
	alarmNames := parseMemberList(form, "AlarmNames.")
	if err := h.Backend.EnableAlarmActions(alarmNames); err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name `xml:"EnableAlarmActionsResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}

func (h *Handler) handleDisableAlarmActions(form url.Values, c *echo.Context) error {
	alarmNames := parseMemberList(form, "AlarmNames.")
	if err := h.Backend.DisableAlarmActions(alarmNames); err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name `xml:"DisableAlarmActionsResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}
