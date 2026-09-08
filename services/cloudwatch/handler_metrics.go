package cloudwatch

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// parseMetricDataFromForm parses MetricData.member.N.* form values.
// Supports the Value field, the StatisticSet (pre-aggregated) path, and the
// Values/Counts array path. Also parses Dimensions and StorageResolution.
// Each datum's Has* flags record which shape (if any) was present on the wire;
// validateMetricDatum uses those flags to reject ambiguous combinations rather
// than guessing from zero values.
func parseMetricDataFromForm(form url.Values) []MetricDatum {
	var data []MetricDatum
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("MetricData.member.%d.", i)
		name := form.Get(prefix + "MetricName")
		if name == "" {
			return data
		}

		datum := parseMetricDatumBaseFields(form, prefix, name)
		applyMetricDatumShape(form, prefix, &datum)
		data = append(data, datum)
	}
}

// parseMetricDatumBaseFields parses the shape-independent fields of a single
// MetricData.member.N entry: MetricName, Unit, Timestamp, StorageResolution,
// and Dimensions.
func parseMetricDatumBaseFields(form url.Values, prefix, name string) MetricDatum {
	unit := form.Get(prefix + "Unit")

	// Parse optional Timestamp (fall back to now).
	ts := time.Now().UTC()
	if tsStr := form.Get(prefix + "Timestamp"); tsStr != "" {
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			ts = t.UTC()
		}
	}

	storageRes, _ := strconv.ParseInt(form.Get(prefix+"StorageResolution"), 10, 32)
	dims := parseDimensionsFromForm(form, prefix+"Dimensions.")

	return MetricDatum{
		MetricName:        name,
		Unit:              unit,
		Timestamp:         ts,
		Dimensions:        dims,
		StorageResolution: int32(storageRes),
	}
}

// applyMetricDatumShape parses whichever of the three mutually-exclusive
// PutMetricData input shapes (Values/Counts, StatisticValues, or Value) is
// present in the form for this entry and populates datum accordingly.
func applyMetricDatumShape(form url.Values, prefix string, datum *MetricDatum) {
	if values, hasValues := parseFloatMemberList(form, prefix+"Values."); hasValues {
		applyValuesArrayShape(form, prefix, datum, values)

		return
	}

	if ssCount := form.Get(prefix + "StatisticValues.SampleCount"); ssCount != "" {
		applyStatisticSetShape(form, prefix, datum, ssCount)

		return
	}

	rawValue := form.Get(prefix + "Value")
	val, _ := strconv.ParseFloat(rawValue, 64)
	datum.HasValue = rawValue != ""
	datum.Value = val
	datum.Count = 1
	datum.Sum = val
	datum.Min = val
	datum.Max = val
}

// applyValuesArrayShape populates datum from a Values/Counts array pair,
// defaulting Counts to all-1s when the caller omits it. The backend
// (storeDatum) derives Sum/SampleCount/Min/Max from Values/Counts at store
// time, so this only needs to carry the raw arrays through.
func applyValuesArrayShape(form url.Values, prefix string, datum *MetricDatum, values []float64) {
	counts, hasCounts := parseFloatMemberList(form, prefix+"Counts.")
	if !hasCounts {
		counts = make([]float64, len(values))
		for j := range counts {
			counts[j] = 1
		}
	}

	datum.HasValuesArray = true
	datum.Values = values
	datum.Counts = counts
}

// applyStatisticSetShape populates datum from a StatisticValues input. A
// caller may also (invalidly) supply Value alongside it; that is preserved so
// validateMetricDatum can reject the combination.
func applyStatisticSetShape(form url.Values, prefix string, datum *MetricDatum, ssCount string) {
	count, _ := strconv.ParseFloat(ssCount, 64)
	sum, _ := strconv.ParseFloat(form.Get(prefix+"StatisticValues.Sum"), 64)
	minimum, _ := strconv.ParseFloat(form.Get(prefix+"StatisticValues.Minimum"), 64)
	maximum, _ := strconv.ParseFloat(form.Get(prefix+"StatisticValues.Maximum"), 64)
	datum.HasStatisticSet = true
	datum.Count = count
	datum.Sum = sum
	datum.Min = minimum
	datum.Max = maximum

	if rawValue := form.Get(prefix + "Value"); rawValue != "" {
		datum.HasValue = true
		datum.Value, _ = strconv.ParseFloat(rawValue, 64)
	}
}

// parseFloatMemberList parses a "Prefix.member.N" list of floats (e.g.
// "MetricData.member.1.Values.member.1"). Returns ok=false when no members are
// present so callers can distinguish an absent list from an explicit empty one.
func parseFloatMemberList(form url.Values, prefix string) ([]float64, bool) {
	var vals []float64

	for i := 1; ; i++ {
		raw := form.Get(fmt.Sprintf("%smember.%d", prefix, i))
		if raw == "" {
			return vals, len(vals) > 0
		}

		f, _ := strconv.ParseFloat(raw, 64)
		vals = append(vals, f)
	}
}

// parseMetricDataQueriesFromForm reads MetricDataQueries.member.N.* values from the form.
func parseMetricDataQueriesFromForm(form url.Values) []MetricDataQuery {
	var queries []MetricDataQuery
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("MetricDataQueries.member.%d.", i)
		id := form.Get(prefix + "Id")
		if id == "" {
			return queries
		}

		period, _ := strconv.ParseInt(form.Get(prefix+"MetricStat.Period"), 10, 32)
		if period <= 0 {
			period = 60
		}

		// ReturnData defaults to true; only set false when caller passes "false".
		returnData := form.Get(prefix+"ReturnData") != formFalse

		dims := parseDimensionsFromForm(form, prefix+"MetricStat.Metric.Dimensions.")

		queries = append(queries, MetricDataQuery{
			ID:         id,
			Label:      form.Get(prefix + "Label"),
			Expression: form.Get(prefix + "Expression"),
			AccountID:  form.Get(prefix + "AccountId"),
			ReturnData: returnData,
			MetricStat: MetricStat{
				Namespace:  form.Get(prefix + "MetricStat.Metric.Namespace"),
				MetricName: form.Get(prefix + "MetricStat.Metric.MetricName"),
				Stat:       form.Get(prefix + "MetricStat.Stat"),
				Period:     int32(period),
				Dimensions: dims,
			},
		})
	}
}

func (h *Handler) handlePutMetricData(form url.Values, c *echo.Context) error {
	namespace := form.Get("Namespace")
	if namespace == "" {
		return h.xmlError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"Namespace is required",
		)
	}
	data := parseMetricDataFromForm(form)
	if err := h.Backend.PutMetricData(namespace, data); err != nil {
		return h.xmlError(c, putMetricDataErrorStatus(err), putMetricDataErrorCode(err), err.Error())
	}

	// PutMetricDataOutput has no members besides the request ID: CloudWatch has
	// no partial-success concept for this operation, so a 200 always means every
	// datum in the request was accepted.
	type response struct {
		XMLName   xml.Name `xml:"PutMetricDataResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}

// putMetricDataErrorCode maps a PutMetricData validation error to its
// Query-protocol (XML, handlePutMetricData) AWS error code. Order matters:
// more specific sentinels must be checked before the generic ErrValidation
// they may also match via errors.Is chains.
func putMetricDataErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrValueAndStatisticSet):
		return "InvalidParameterCombination"
	case errors.Is(err, ErrMetricSeriesLimitExceeded):
		return "LimitExceeded"
	case errors.Is(err, ErrValuesCountsLengthMismatch),
		errors.Is(err, ErrTooManyValues),
		errors.Is(err, ErrInvalidMetricValue),
		errors.Is(err, ErrMetricTimestampOutOfRange),
		errors.Is(err, ErrValidation):
		return "InvalidParameterValue"
	default:
		return errCodeInternalFailure
	}
}

// putMetricDataCBORErrorCode maps a PutMetricData validation error to its
// rpc-v2-cbor exception shape name (cborPutMetricData). "InvalidParameterCombination",
// "LimitExceeded", and "InvalidParameterValue" are the AWSQueryError
// compatibility aliases cloudwatch's schemas.go embeds on
// InvalidParameterCombinationException/LimitExceededFault/InvalidParameterValueException
// for query-compatible callers; a non-query-compatible rpc-v2-cbor client
// (aws-sdk-go-v2's NewCBOR protocol, which cloudwatch uses exclusively)
// resolves the __type body field against the shape's own name instead, so
// this must return the Exception/Fault-suffixed names, not the aliases.
func putMetricDataCBORErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrValueAndStatisticSet):
		return "InvalidParameterCombinationException"
	case errors.Is(err, ErrMetricSeriesLimitExceeded):
		return "LimitExceededFault"
	case errors.Is(err, ErrValuesCountsLengthMismatch),
		errors.Is(err, ErrTooManyValues),
		errors.Is(err, ErrInvalidMetricValue),
		errors.Is(err, ErrMetricTimestampOutOfRange),
		errors.Is(err, ErrValidation):
		return "InvalidParameterValueException"
	default:
		return errCodeInternalFailure
	}
}

// putMetricDataErrorStatus maps a PutMetricData validation error to its HTTP status.
func putMetricDataErrorStatus(err error) int {
	if putMetricDataErrorCode(err) == errCodeInternalFailure {
		return http.StatusInternalServerError
	}

	return http.StatusBadRequest
}

func (h *Handler) handleGetMetricStatistics(form url.Values, c *echo.Context) error {
	namespace := form.Get("Namespace")
	metricName := form.Get("MetricName")
	startStr := form.Get("StartTime")
	endStr := form.Get("EndTime")
	periodStr := form.Get("Period")

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "invalid StartTime")
	}
	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "invalid EndTime")
	}
	period, err := strconv.ParseInt(periodStr, 10, 32)
	if err != nil || period <= 0 {
		return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "invalid Period")
	}

	dimensions := parseDimensionsFromForm(form, "Dimensions.")
	statistics := parseMemberList(form, "Statistics.")
	extendedStatistics := parseMemberList(form, "ExtendedStatistics.")
	dps, berr := h.Backend.GetMetricStatistics(
		namespace,
		metricName,
		dimensions,
		startTime,
		endTime,
		int32(period),
		statistics,
		extendedStatistics,
	)
	if berr != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", berr.Error())
	}

	return writeXML(c, buildGetMetricStatisticsResponse(metricName, dps))
}

func buildGetMetricStatisticsResponse(metricName string, dps []Datapoint) any {
	type extStatXML struct {
		Key   string  `xml:"Name"`
		Value float64 `xml:"Value"`
	}
	type dpXML struct {
		Average            *float64     `xml:"Average,omitempty"`
		Sum                *float64     `xml:"Sum,omitempty"`
		Minimum            *float64     `xml:"Minimum,omitempty"`
		Maximum            *float64     `xml:"Maximum,omitempty"`
		SampleCount        *float64     `xml:"SampleCount,omitempty"`
		Timestamp          string       `xml:"Timestamp"`
		Unit               string       `xml:"Unit,omitempty"`
		ExtendedStatistics []extStatXML `xml:"ExtendedStatistics>member,omitempty"`
	}
	members := make([]dpXML, 0, len(dps))
	for _, dp := range dps {
		d := dpXML{
			Timestamp:   dp.Timestamp.UTC().Format(time.RFC3339),
			Unit:        dp.Unit,
			Average:     dp.Average,
			Sum:         dp.Sum,
			Minimum:     dp.Minimum,
			Maximum:     dp.Maximum,
			SampleCount: dp.SampleCount,
		}
		for k, v := range dp.ExtendedStatistics {
			d.ExtendedStatistics = append(d.ExtendedStatistics, extStatXML{Key: k, Value: v})
		}
		members = append(members, d)
	}
	type result struct {
		Label      string  `xml:"Label"`
		Datapoints []dpXML `xml:"Datapoints>member"`
	}
	type response struct {
		XMLName   xml.Name `xml:"GetMetricStatisticsResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		Result    result   `xml:"GetMetricStatisticsResult"`
	}

	return response{
		Xmlns:     cloudwatchNS,
		Result:    result{Datapoints: members, Label: metricName},
		RequestID: uuid.New().String(),
	}
}

func (h *Handler) handleListMetrics(form url.Values, c *echo.Context) error {
	namespace := form.Get("Namespace")
	metricName := form.Get("MetricName")
	nextToken := form.Get("NextToken")
	recentlyActive := form.Get("RecentlyActive")
	maxResults, _ := strconv.Atoi(form.Get("MaxResults"))
	dimensions := parseDimensionsFromForm(form, "Dimensions.")

	p, err := h.Backend.ListMetrics(namespace, metricName, dimensions, recentlyActive, nextToken, maxResults)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
		}

		return h.xmlError(c, http.StatusInternalServerError, errCodeInternalFailure, err.Error())
	}

	type dimXML struct {
		Name  string `xml:"Name"`
		Value string `xml:"Value"`
	}
	type metricXML struct {
		Namespace  string   `xml:"Namespace"`
		MetricName string   `xml:"MetricName"`
		Dimensions []dimXML `xml:"Dimensions>member,omitempty"`
	}
	members := make([]metricXML, 0, len(p.Data))
	for _, m := range p.Data {
		dims := make([]dimXML, 0, len(m.Dimensions))
		for _, d := range m.Dimensions {
			dims = append(dims, dimXML(d))
		}
		members = append(
			members,
			metricXML{Namespace: m.Namespace, MetricName: m.MetricName, Dimensions: dims},
		)
	}

	type listResult struct {
		NextToken string      `xml:"NextToken,omitempty"`
		Metrics   []metricXML `xml:"Metrics>member"`
	}
	type response struct {
		XMLName   xml.Name   `xml:"ListMetricsResponse"`
		Xmlns     string     `xml:"xmlns,attr"`
		RequestID string     `xml:"ResponseMetadata>RequestId"`
		Result    listResult `xml:"ListMetricsResult"`
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		Result:    listResult{Metrics: members, NextToken: p.Next},
		RequestID: uuid.New().String(),
	})
}

func (h *Handler) handleGetMetricData(form url.Values, c *echo.Context) error {
	startStr := form.Get("StartTime")
	endStr := form.Get("EndTime")

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		startTime = time.Now().UTC().Add(-time.Hour)
	}

	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		endTime = time.Now().UTC()
	}

	scanBy := form.Get("ScanBy")
	nextToken := form.Get("NextToken")
	maxDatapoints, _ := strconv.Atoi(form.Get("MaxDatapoints"))
	queries := parseMetricDataQueriesFromForm(form)

	var pageResult GetMetricDataPage
	var berr error
	if bk, ok := h.Backend.(*InMemoryBackend); ok {
		pageResult, berr = bk.GetMetricDataPaged(
			queries, startTime, endTime, scanBy, nextToken, maxDatapoints,
		)
	} else {
		var results []MetricDataResult
		results, berr = h.Backend.GetMetricData(queries, startTime, endTime)
		pageResult.Results = results
	}
	if berr != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", berr.Error())
	}

	type messageXML struct {
		Code  string `xml:"Code"`
		Value string `xml:"Value"`
	}
	// resultEntry has no XMLName override: one previously set to "member" won for
	// the repeating element name over the parent field's own tag (Go's
	// encoding/xml gives a child XMLName tag priority), which silently dropped
	// the <MetricDataResults> wrapping level a real client needs
	// (schemas.go: GetMetricDataOutput_MetricDataResults = AddMember("MetricDataResults", ...)).
	type resultEntry struct {
		ID         string       `xml:"Id"`
		Label      string       `xml:"Label,omitempty"`
		StatusCode string       `xml:"StatusCode"`
		Timestamps []string     `xml:"Timestamps>member"`
		Values     []float64    `xml:"Values>member"`
		Messages   []messageXML `xml:"Messages>member,omitempty"`
	}

	type response struct {
		XMLName           xml.Name      `xml:"GetMetricDataResponse"`
		Xmlns             string        `xml:"xmlns,attr"`
		RequestID         string        `xml:"ResponseMetadata>RequestId"`
		NextToken         string        `xml:"GetMetricDataResult>NextToken,omitempty"`
		MetricDataResults []resultEntry `xml:"GetMetricDataResult>MetricDataResults>member"`
		Messages          []messageXML  `xml:"GetMetricDataResult>Messages>member,omitempty"`
	}

	resp := response{
		Xmlns:     cloudwatchNS,
		RequestID: uuid.New().String(),
		NextToken: pageResult.NextToken,
	}

	for _, r := range pageResult.Results {
		entry := resultEntry{
			ID:         r.ID,
			Label:      r.Label,
			StatusCode: r.StatusCode,
			Values:     r.Values,
		}
		for _, ts := range r.Timestamps {
			entry.Timestamps = append(entry.Timestamps, ts.UTC().Format(time.RFC3339))
		}
		for _, m := range r.Messages {
			entry.Messages = append(entry.Messages, messageXML(m))
		}

		resp.MetricDataResults = append(resp.MetricDataResults, entry)
	}

	for _, m := range pageResult.Messages {
		resp.Messages = append(resp.Messages, messageXML(m))
	}

	return writeXML(c, resp)
}
