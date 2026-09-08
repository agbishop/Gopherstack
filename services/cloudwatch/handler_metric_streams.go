package cloudwatch

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// parseMetricStreamFiltersFromForm parses IncludeFilters.member.N.* or ExcludeFilters.member.N.* form values.
func parseMetricStreamFiltersFromForm(form url.Values, listPrefix string) []MetricStreamFilter {
	var filters []MetricStreamFilter
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("%smember.%d.", listPrefix, i)
		ns := form.Get(prefix + "Namespace")
		if ns == "" {
			return filters
		}
		var metricNames []string
		for j := 1; ; j++ {
			mn := form.Get(fmt.Sprintf("%sMetricNames.member.%d", prefix, j))
			if mn == "" {
				break
			}
			metricNames = append(metricNames, mn)
		}
		filters = append(filters, MetricStreamFilter{Namespace: ns, MetricNames: metricNames})
	}
}

func (h *Handler) putMetricStreamFromForm(form url.Values, c *echo.Context) error {
	name := form.Get("Name")
	if name == "" {
		return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "Name is required")
	}

	if err := h.Backend.PutMetricStream(&MetricStream{
		Name:           name,
		FirehoseArn:    form.Get("FirehoseArn"),
		RoleArn:        form.Get("RoleArn"),
		OutputFormat:   form.Get("OutputFormat"),
		State:          form.Get("State"),
		IncludeFilters: parseMetricStreamFiltersFromForm(form, "IncludeFilters."),
		ExcludeFilters: parseMetricStreamFiltersFromForm(form, "ExcludeFilters."),
	}); err != nil {
		if errors.Is(err, ErrValidation) {
			return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
		}

		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return nil
}

func (h *Handler) handlePutMetricStream(form url.Values, c *echo.Context) error {
	name := form.Get("Name")
	if err := h.putMetricStreamFromForm(form, c); err != nil {
		return err
	}

	stream, err := h.Backend.GetMetricStream(name)
	if err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type putMetricStreamResult struct {
		Arn string `xml:"Arn"`
	}
	type response struct {
		XMLName   xml.Name              `xml:"PutMetricStreamResponse"`
		Xmlns     string                `xml:"xmlns,attr"`
		RequestID string                `xml:"ResponseMetadata>RequestId"`
		Result    putMetricStreamResult `xml:"PutMetricStreamResult"`
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		RequestID: uuid.New().String(),
		Result:    putMetricStreamResult{Arn: stream.Arn},
	})
}

func (h *Handler) handleDeleteMetricStream(form url.Values, c *echo.Context) error {
	name := form.Get("Name")
	if name == "" {
		return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "Name is required")
	}

	// Look up the real (region+account qualified) ARN before deleting so tag
	// cleanup targets the same key TagResource was called with -- a
	// hardcoded "arn:aws:cloudwatch::metric-stream/"+name (no region/account)
	// previously never matched, leaving tags orphaned after delete.
	stream, getErr := h.Backend.GetMetricStream(name)

	if err := h.Backend.DeleteMetricStream(name); err != nil {
		return h.xmlError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	if getErr == nil {
		h.deleteResourceTags(stream.Arn)
	}

	type response struct {
		XMLName   xml.Name       `xml:"DeleteMetricStreamResponse"`
		Result    xmlEmptyResult `xml:"DeleteMetricStreamResult"`
		Xmlns     string         `xml:"xmlns,attr"`
		RequestID string         `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}

func (h *Handler) handleListMetricStreams(form url.Values, c *echo.Context) error {
	nextToken := form.Get("NextToken")
	maxResults, _ := strconv.Atoi(form.Get("MaxResults"))

	p, err := h.Backend.ListMetricStreams(nextToken, maxResults)
	if err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type entryXML struct {
		Name           string `xml:"Name"`
		Arn            string `xml:"Arn"`
		FirehoseArn    string `xml:"FirehoseArn"`
		State          string `xml:"State"`
		OutputFormat   string `xml:"OutputFormat"`
		CreationDate   string `xml:"CreationDate,omitempty"`
		LastUpdateDate string `xml:"LastUpdateDate,omitempty"`
	}
	members := make([]entryXML, 0, len(p.Data))
	for _, s := range p.Data {
		members = append(members, entryXML{
			Name:           s.Name,
			Arn:            s.Arn,
			FirehoseArn:    s.FirehoseArn,
			State:          s.State,
			OutputFormat:   s.OutputFormat,
			CreationDate:   formatTimeOmitZero(s.CreationDate),
			LastUpdateDate: formatTimeOmitZero(s.LastUpdateDate),
		})
	}

	type listResult struct {
		NextToken     string     `xml:"NextToken,omitempty"`
		MetricStreams []entryXML `xml:"Entries>member"`
	}
	type response struct {
		XMLName   xml.Name   `xml:"ListMetricStreamsResponse"`
		Xmlns     string     `xml:"xmlns,attr"`
		RequestID string     `xml:"ResponseMetadata>RequestId"`
		Result    listResult `xml:"ListMetricStreamsResult"`
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		RequestID: uuid.New().String(),
		Result:    listResult{MetricStreams: members, NextToken: p.Next},
	})
}

func (h *Handler) handleGetMetricStream(form url.Values, c *echo.Context) error {
	name := form.Get("Name")
	if name == "" {
		return h.xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "Name is required")
	}

	stream, err := h.Backend.GetMetricStream(name)
	if err != nil {
		return h.xmlError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	type filterXML struct {
		Namespace   string   `xml:"Namespace"`
		MetricNames []string `xml:"MetricNames>member,omitempty"`
	}
	type result struct {
		Name           string      `xml:"Name"`
		Arn            string      `xml:"Arn"`
		FirehoseArn    string      `xml:"FirehoseArn"`
		RoleArn        string      `xml:"RoleArn"`
		State          string      `xml:"State"`
		OutputFormat   string      `xml:"OutputFormat"`
		CreationDate   string      `xml:"CreationDate,omitempty"`
		LastUpdateDate string      `xml:"LastUpdateDate,omitempty"`
		IncludeFilters []filterXML `xml:"IncludeFilters>member,omitempty"`
		ExcludeFilters []filterXML `xml:"ExcludeFilters>member,omitempty"`
	}
	type response struct {
		XMLName   xml.Name `xml:"GetMetricStreamResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
		Result    result   `xml:"GetMetricStreamResult"`
	}

	toFilterXML := func(filters []MetricStreamFilter) []filterXML {
		out := make([]filterXML, 0, len(filters))
		for _, f := range filters {
			out = append(out, filterXML(f))
		}

		return out
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		RequestID: uuid.New().String(),
		Result: result{
			Name:           stream.Name,
			Arn:            stream.Arn,
			FirehoseArn:    stream.FirehoseArn,
			RoleArn:        stream.RoleArn,
			State:          stream.State,
			OutputFormat:   stream.OutputFormat,
			CreationDate:   formatTimeOmitZero(stream.CreationDate),
			LastUpdateDate: formatTimeOmitZero(stream.LastUpdateDate),
			IncludeFilters: toFilterXML(stream.IncludeFilters),
			ExcludeFilters: toFilterXML(stream.ExcludeFilters),
		},
	})
}

func (h *Handler) handleStartMetricStreams(form url.Values, c *echo.Context) error {
	names := parseMemberList(form, "Names.")
	if err := h.Backend.StartMetricStreams(names); err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name       `xml:"StartMetricStreamsResponse"`
		Result    xmlEmptyResult `xml:"StartMetricStreamsResult"`
		Xmlns     string         `xml:"xmlns,attr"`
		RequestID string         `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}

func (h *Handler) handleStopMetricStreams(form url.Values, c *echo.Context) error {
	names := parseMemberList(form, "Names.")
	if err := h.Backend.StopMetricStreams(names); err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type response struct {
		XMLName   xml.Name       `xml:"StopMetricStreamsResponse"`
		Result    xmlEmptyResult `xml:"StopMetricStreamsResult"`
		Xmlns     string         `xml:"xmlns,attr"`
		RequestID string         `xml:"ResponseMetadata>RequestId"`
	}

	return writeXML(c, response{Xmlns: cloudwatchNS, RequestID: uuid.New().String()})
}
