package iot

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func resolveFleetMetricOps(path, method string) string {
	switch {
	case path == "/fleet-metrics" && method == http.MethodGet:
		return opListFleetMetrics
	case strings.HasPrefix(path, "/fleet-metric/") && method == http.MethodPut:
		return opCreateFleetMetric
	case strings.HasPrefix(path, "/fleet-metric/") && method == http.MethodGet:
		return opDescribeFleetMetric
	case strings.HasPrefix(path, "/fleet-metric/") && method == http.MethodPatch:
		return opUpdateFleetMetric
	case strings.HasPrefix(path, "/fleet-metric/") && method == http.MethodDelete:
		return opDeleteFleetMetric
	}

	return unknownOperation
}

// resolveCustomMetricOps resolves the custom-metric op family.
//
// ListCustomMetrics' real path is GET /custom-metrics (plural, iot@v1.77.4
// serializers.go) -- every other op in this family is correctly singular
// "/custom-metric/{name}", but List was too, unreachable by a real client.
// Found by gopherstack-n1mb's route table. The singular bare path is kept
// too as a non-canonical route wired for this package's own tests.
func resolveCustomMetricOps(path, method string) string {
	switch {
	case path == "/custom-metrics" && method == http.MethodGet:
		return opListCustomMetrics
	case path == "/custom-metric" && method == http.MethodGet:
		return opListCustomMetrics
	case strings.HasPrefix(path, "/custom-metric/") && method == http.MethodPost:
		return opCreateCustomMetric
	case strings.HasPrefix(path, "/custom-metric/") && method == http.MethodGet:
		return opDescribeCustomMetric
	case strings.HasPrefix(path, "/custom-metric/") && method == http.MethodPatch:
		return opUpdateCustomMetric
	case strings.HasPrefix(path, "/custom-metric/") && method == http.MethodDelete:
		return opDeleteCustomMetric
	}

	return unknownOperation
}

func resolveDimensionOps(path, method string) string {
	switch {
	case path == "/dimensions" && method == http.MethodGet:
		return opListDimensions
	case strings.HasPrefix(path, "/dimensions/") && method == http.MethodPost:
		return opCreateDimension
	case strings.HasPrefix(path, "/dimensions/") && method == http.MethodGet:
		return opDescribeDimension
	case strings.HasPrefix(path, "/dimensions/") && method == http.MethodPatch:
		return opUpdateDimension
	case strings.HasPrefix(path, "/dimensions/") && method == http.MethodDelete:
		return opDeleteDimension
	}

	return unknownOperation
}

func (h *Handler) handleCreateFleetMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/fleet-metric/")
	var input CreateFleetMetricInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.MetricName = name
	fm, err := h.Backend.CreateFleetMetric(&input)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyMetricName: fm.MetricName,
		keyMetricARN:  fm.MetricARN,
	})
}

func (h *Handler) handleDescribeFleetMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/fleet-metric/")
	fm, err := h.Backend.DescribeFleetMetric(name)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, fm)
}

func (h *Handler) handleListFleetMetrics(c *echo.Context) error {
	metrics := h.Backend.ListFleetMetrics()
	summaries := make([]map[string]any, len(metrics))
	for i, fm := range metrics {
		summaries[i] = map[string]any{
			keyMetricName: fm.MetricName,
			keyMetricARN:  fm.MetricARN,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"fleetMetrics": summaries})
}

func (h *Handler) handleUpdateFleetMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/fleet-metric/")
	var input UpdateFleetMetricInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	if err := h.Backend.UpdateFleetMetric(name, &input); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleDeleteFleetMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/fleet-metric/")
	expectedVersion := parseExpectedVersionQueryParam(c)
	if err := h.Backend.DeleteFleetMetric(name, expectedVersion); err != nil {
		// DeleteFleetMetric's own deserializeOpError switch declares no
		// ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrResourceNotFound)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleCreateCustomMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/custom-metric/")
	var input CreateCustomMetricInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.MetricName = name
	cm, err := h.Backend.CreateCustomMetric(&input)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyMetricName: cm.MetricName,
		keyMetricARN:  cm.MetricARN,
	})
}

func (h *Handler) handleDescribeCustomMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/custom-metric/")
	cm, err := h.Backend.DescribeCustomMetric(name)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, cm)
}

func (h *Handler) handleListCustomMetrics(c *echo.Context) error {
	metrics := h.Backend.ListCustomMetrics()
	names := make([]string, len(metrics))
	for i, cm := range metrics {
		names[i] = cm.MetricName
	}

	return c.JSON(http.StatusOK, map[string]any{"metricNames": names})
}

func (h *Handler) handleUpdateCustomMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/custom-metric/")
	var input UpdateCustomMetricInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	cm, err := h.Backend.UpdateCustomMetric(name, input.DisplayName)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyMetricName: cm.MetricName,
		keyMetricARN:  cm.MetricARN,
	})
}

func (h *Handler) handleDeleteCustomMetric(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/custom-metric/")
	if err := h.Backend.DeleteCustomMetric(name); err != nil {
		// DeleteCustomMetric's own deserializeOpError switch declares no
		// ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrResourceNotFound)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleCreateDimension(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/dimensions/")
	var input CreateDimensionInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.Name = name
	d, err := h.Backend.CreateDimension(&input)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyName: d.Name,
		"arn":   d.ARN,
	})
}

func (h *Handler) handleDescribeDimension(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/dimensions/")
	d, err := h.Backend.DescribeDimension(name)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, d)
}

func (h *Handler) handleListDimensions(c *echo.Context) error {
	dims := h.Backend.ListDimensions()
	names := make([]string, len(dims))
	for i, d := range dims {
		names[i] = d.Name
	}

	return c.JSON(http.StatusOK, map[string]any{"dimensionNames": names})
}

func (h *Handler) handleUpdateDimension(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/dimensions/")
	var input UpdateDimensionInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	d, err := h.Backend.UpdateDimension(name, input.StringValues)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyName: d.Name,
		"arn":   d.ARN,
	})
}

func (h *Handler) handleDeleteDimension(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/dimensions/")
	if err := h.Backend.DeleteDimension(name); err != nil {
		// DeleteDimension's own deserializeOpError switch declares no
		// ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrResourceNotFound)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleListMetricValues(c *echo.Context) error {
	thingName := c.QueryParam(keyThingName)
	metricName := c.QueryParam("metricName")
	startTime := parseIoTEpochQueryParam(c, "startTime")
	endTime := parseIoTEpochQueryParam(c, "endTime")
	maxResults := parseInt32QueryParam(c, "maxResults")
	nextToken := c.QueryParam("nextToken")

	values, next, err := h.Backend.ListMetricValues(thingName, metricName, startTime, endTime, maxResults, nextToken)
	if err != nil {
		return h.handleError(c, err)
	}

	resp := map[string]any{"metricDatumList": values}
	if next != "" {
		resp["nextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) dispatchFleetMetricOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateFleetMetric:
		return true, h.handleCreateFleetMetric(c)
	case opDescribeFleetMetric:
		return true, h.handleDescribeFleetMetric(c)
	case opListFleetMetrics:
		return true, h.handleListFleetMetrics(c)
	case opUpdateFleetMetric:
		return true, h.handleUpdateFleetMetric(c)
	case opDeleteFleetMetric:
		return true, h.handleDeleteFleetMetric(c)
	}

	return false, nil
}

func (h *Handler) dispatchCustomMetricOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateCustomMetric:
		return true, h.handleCreateCustomMetric(c)
	case opDescribeCustomMetric:
		return true, h.handleDescribeCustomMetric(c)
	case opListCustomMetrics:
		return true, h.handleListCustomMetrics(c)
	case opUpdateCustomMetric:
		return true, h.handleUpdateCustomMetric(c)
	case opDeleteCustomMetric:
		return true, h.handleDeleteCustomMetric(c)
	}

	return false, nil
}

func (h *Handler) dispatchDimensionOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateDimension:
		return true, h.handleCreateDimension(c)
	case opDescribeDimension:
		return true, h.handleDescribeDimension(c)
	case opListDimensions:
		return true, h.handleListDimensions(c)
	case opUpdateDimension:
		return true, h.handleUpdateDimension(c)
	case opDeleteDimension:
		return true, h.handleDeleteDimension(c)
	}

	return false, nil
}
