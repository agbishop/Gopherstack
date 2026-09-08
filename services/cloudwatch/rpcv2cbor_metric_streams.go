package cloudwatch

import (
	"errors"
	"net/http"

	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/labstack/echo/v5"
)

// cborMetricStreamFilters extracts a list of MetricStreamFilter from a CBOR map key.
func cborMetricStreamFilters(input cbor.Map, key string) []MetricStreamFilter {
	listVal, ok := input[key]
	if !ok {
		return nil
	}
	list, isList := listVal.(cbor.List)
	if !isList {
		return nil
	}
	filters := make([]MetricStreamFilter, 0, len(list))
	for _, item := range list {
		fm, isMap := item.(cbor.Map)
		if !isMap {
			continue
		}
		ns := cborStr(fm, keyNamespace)
		if ns == "" {
			continue
		}
		metricNames := cborStrList(fm, "MetricNames")
		filters = append(filters, MetricStreamFilter{Namespace: ns, MetricNames: metricNames})
	}

	return filters
}

// buildMetricStreamFiltersCBOR converts a []MetricStreamFilter to its wire
// shape (cloudwatch@v1.66.3 schemas/schemas.go:3937-3939, MetricStreamFilter).
func buildMetricStreamFiltersCBOR(filters []MetricStreamFilter) cbor.List {
	out := make(cbor.List, 0, len(filters))
	for _, f := range filters {
		out = append(out, cbor.Map{
			keyNamespace:  cbor.String(f.Namespace),
			"MetricNames": cborStringList(f.MetricNames),
		})
	}

	return out
}

// cborMetricStreamStatisticsConfigurations extracts
// []MetricStreamStatisticsConfiguration from the "StatisticsConfigurations"
// key of a CBOR map (aws-sdk-go-v2 cloudwatch@v1.66.3 types/types.go:3270).
func cborMetricStreamStatisticsConfigurations(input cbor.Map, key string) []MetricStreamStatisticsConfiguration {
	listVal, ok := input[key]
	if !ok {
		return nil
	}

	list, isList := listVal.(cbor.List)
	if !isList {
		return nil
	}

	configs := make([]MetricStreamStatisticsConfiguration, 0, len(list))

	for _, item := range list {
		cm, isMap := item.(cbor.Map)
		if !isMap {
			continue
		}

		var metrics []MetricStreamStatisticsMetric

		if metricsVal, hasMetrics := cm["IncludeMetrics"].(cbor.List); hasMetrics {
			metrics = make([]MetricStreamStatisticsMetric, 0, len(metricsVal))
			for _, mv := range metricsVal {
				mm, mmIsMap := mv.(cbor.Map)
				if !mmIsMap {
					continue
				}

				metrics = append(metrics, MetricStreamStatisticsMetric{
					MetricName: cborStr(mm, "MetricName"),
					Namespace:  cborStr(mm, keyNamespace),
				})
			}
		}

		configs = append(configs, MetricStreamStatisticsConfiguration{
			AdditionalStatistics: cborStrList(cm, "AdditionalStatistics"),
			IncludeMetrics:       metrics,
		})
	}

	return configs
}

// buildMetricStreamStatisticsConfigurationsCBOR converts
// []MetricStreamStatisticsConfiguration to its wire shape.
func buildMetricStreamStatisticsConfigurationsCBOR(configs []MetricStreamStatisticsConfiguration) cbor.List {
	out := make(cbor.List, 0, len(configs))

	for _, cfg := range configs {
		metrics := make(cbor.List, 0, len(cfg.IncludeMetrics))
		for _, m := range cfg.IncludeMetrics {
			metrics = append(metrics, cbor.Map{
				"MetricName": cbor.String(m.MetricName),
				keyNamespace: cbor.String(m.Namespace),
			})
		}

		out = append(out, cbor.Map{
			"AdditionalStatistics": cborStringList(cfg.AdditionalStatistics),
			"IncludeMetrics":       metrics,
		})
	}

	return out
}

func (h *Handler) cborPutMetricStream(input cbor.Map, c *echo.Context) error {
	name := cborStr(input, keyName)
	if name == "" {
		return h.cborError(c, http.StatusBadRequest, "InvalidParameterValue", "Name is required")
	}

	if err := h.Backend.PutMetricStream(&MetricStream{
		Name:                     name,
		FirehoseArn:              cborStr(input, "FirehoseArn"),
		RoleArn:                  cborStr(input, "RoleArn"),
		OutputFormat:             cborStr(input, "OutputFormat"),
		State:                    cborStr(input, keyState),
		IncludeFilters:           cborMetricStreamFilters(input, "IncludeFilters"),
		ExcludeFilters:           cborMetricStreamFilters(input, "ExcludeFilters"),
		StatisticsConfigurations: cborMetricStreamStatisticsConfigurations(input, "StatisticsConfigurations"),
	}); err != nil {
		if errors.Is(err, ErrValidation) {
			return h.cborError(c, http.StatusBadRequest, "InvalidParameterValueException", err.Error())
		}

		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	stream, err := h.Backend.GetMetricStream(name)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	h.applyCreationTags(input, stream.Arn)

	return writeCBOR(c, cbor.Map{"Arn": cbor.String(stream.Arn)})
}

func (h *Handler) cborListMetricStreams(input cbor.Map, c *echo.Context) error {
	nextToken := cborStr(input, "NextToken")
	maxResults := int(cborInt32(input, "MaxResults"))

	p, err := h.Backend.ListMetricStreams(nextToken, maxResults)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	entries := make(cbor.List, 0, len(p.Data))
	for _, s := range p.Data {
		entry := cbor.Map{
			keyName:        cbor.String(s.Name),
			keyArn:         cbor.String(s.Arn),
			"FirehoseArn":  cbor.String(s.FirehoseArn),
			keyState:       cbor.String(s.State),
			"OutputFormat": cbor.String(s.OutputFormat),
		}
		if !s.CreationDate.IsZero() {
			entry["CreationDate"] = cborFromTime(s.CreationDate)
		}
		if !s.LastUpdateDate.IsZero() {
			entry["LastUpdateDate"] = cborFromTime(s.LastUpdateDate)
		}
		entries = append(entries, entry)
	}

	out := cbor.Map{
		"Entries": entries,
	}
	if p.Next != "" {
		out["NextToken"] = cbor.String(p.Next)
	}

	return writeCBOR(c, out)
}

func (h *Handler) cborGetMetricStream(input cbor.Map, c *echo.Context) error {
	name := cborStr(input, keyName)
	if name == "" {
		return h.cborError(c, http.StatusBadRequest, "InvalidParameterValue", "Name is required")
	}

	stream, err := h.Backend.GetMetricStream(name)
	if err != nil {
		return h.cborError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	out := cbor.Map{
		keyName:        cbor.String(stream.Name),
		"Arn":          cbor.String(stream.Arn),
		"FirehoseArn":  cbor.String(stream.FirehoseArn),
		"RoleArn":      cbor.String(stream.RoleArn),
		keyState:       cbor.String(stream.State),
		"OutputFormat": cbor.String(stream.OutputFormat),
	}
	if !stream.CreationDate.IsZero() {
		out["CreationDate"] = cborFromTime(stream.CreationDate)
	}
	if !stream.LastUpdateDate.IsZero() {
		out["LastUpdateDate"] = cborFromTime(stream.LastUpdateDate)
	}
	if len(stream.IncludeFilters) > 0 {
		out["IncludeFilters"] = buildMetricStreamFiltersCBOR(stream.IncludeFilters)
	}
	if len(stream.ExcludeFilters) > 0 {
		out["ExcludeFilters"] = buildMetricStreamFiltersCBOR(stream.ExcludeFilters)
	}
	if len(stream.StatisticsConfigurations) > 0 {
		out["StatisticsConfigurations"] = buildMetricStreamStatisticsConfigurationsCBOR(stream.StatisticsConfigurations)
	}

	return writeCBOR(c, out)
}

func (h *Handler) cborDeleteMetricStream(input cbor.Map, c *echo.Context) error {
	name := cborStr(input, keyName)
	if name == "" {
		return h.cborError(c, http.StatusBadRequest, "InvalidParameterValue", "Name is required")
	}

	stream, getErr := h.Backend.GetMetricStream(name)

	if err := h.Backend.DeleteMetricStream(name); err != nil {
		return h.cborError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	if getErr == nil {
		h.deleteResourceTags(stream.Arn)
	}

	return writeCBOR(c, cbor.Map{})
}

func (h *Handler) cborStartMetricStreams(input cbor.Map, c *echo.Context) error {
	if err := h.Backend.StartMetricStreams(cborStrList(input, "Names")); err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, cbor.Map{})
}

func (h *Handler) cborStopMetricStreams(input cbor.Map, c *echo.Context) error {
	if err := h.Backend.StopMetricStreams(cborStrList(input, "Names")); err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, cbor.Map{})
}
