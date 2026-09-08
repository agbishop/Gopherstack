package iot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// resolveIndexOps maps fleet-indexing REST paths to their operation names.
// Real AWS IoT REST paths: GET/POST /indices/config, POST /indices/search,
// GET /indices, GET /indices/{indexName}, POST /indices/cardinality,
// POST /indices/percentiles, POST /indices/statistics, POST /indices/buckets.
func resolveIndexOps(path, method string) string {
	if op := resolveIndexConfigOps(path, method); op != unknownOperation {
		return op
	}

	return resolveIndexQueryOps(path, method)
}

// resolveIndexConfigOps handles the configuration/discovery surface: listing
// and describing indices, and getting/updating the indexing configuration.
//
// The real AWS IoT SDK sends Get/UpdateIndexingConfiguration to GET/POST
// /indexing/config (not /indices/config); /indices/config is also accepted for
// backward compatibility with older callers of this emulator.
func resolveIndexConfigOps(path, method string) string {
	switch {
	case path == pathIndices && method == http.MethodGet:
		return opListIndices
	case (path == pathIndexingConfig || path == pathIndices+"/config") && method == http.MethodGet:
		return opGetIndexingConfiguration
	case (path == pathIndexingConfig || path == pathIndices+"/config") && method == http.MethodPost:
		return opUpdateIndexingConfiguration
	case strings.HasPrefix(path, pathIndices+"/") && method == http.MethodGet:
		return opDescribeIndex
	}

	return unknownOperation
}

// resolveIndexQueryOps handles the search/aggregation surface, all POST.
func resolveIndexQueryOps(path, method string) string {
	if method != http.MethodPost {
		return unknownOperation
	}

	switch path {
	case pathIndices + "/search":
		return opSearchIndex
	case pathIndices + "/cardinality":
		return opGetCardinality
	case pathIndices + "/percentiles":
		return opGetPercentiles
	case pathIndices + "/statistics":
		return opGetStatistics
	case pathIndices + "/buckets":
		return opGetBucketsAggregation
	}

	return unknownOperation
}

// dispatchIndexOps routes fleet-indexing / search / aggregation operations to
// their real handlers.
func (h *Handler) dispatchIndexOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opUpdateIndexingConfiguration:
		return true, h.handleUpdateIndexingConfiguration(c)
	case opGetIndexingConfiguration:
		return true, h.handleGetIndexingConfiguration(c)
	case opListIndices:
		return true, h.handleListIndices(c)
	case opDescribeIndex:
		return true, h.handleDescribeIndex(c)
	case opSearchIndex:
		return true, h.handleSearchIndex(c)
	case opGetCardinality:
		return true, h.handleGetCardinality(c)
	case opGetPercentiles:
		return true, h.handleGetPercentiles(c)
	case opGetStatistics:
		return true, h.handleGetStatistics(c)
	case opGetBucketsAggregation:
		return true, h.handleGetBucketsAggregation(c)
	}

	return false, nil
}

// decodeJSONBody decodes the request body into v, tolerating an empty body.
func decodeJSONBody(c *echo.Context, v any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	return nil
}

func (h *Handler) handleUpdateIndexingConfiguration(c *echo.Context) error {
	var body struct {
		ThingIndexingConfiguration      *ThingIndexingConfiguration      `json:"thingIndexingConfiguration"`
		ThingGroupIndexingConfiguration *ThingGroupIndexingConfiguration `json:"thingGroupIndexingConfiguration"`
	}

	if err := decodeJSONBody(c, &body); err != nil {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	if err := h.Backend.UpdateIndexingConfiguration(&UpdateIndexingConfigurationInput{
		ThingIndexingConfiguration:      body.ThingIndexingConfiguration,
		ThingGroupIndexingConfiguration: body.ThingGroupIndexingConfiguration,
	}); err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleGetIndexingConfiguration(c *echo.Context) error {
	out := h.Backend.GetIndexingConfiguration()

	return c.JSON(http.StatusOK, map[string]any{
		"thingIndexingConfiguration":      out.ThingIndexingConfiguration,
		"thingGroupIndexingConfiguration": out.ThingGroupIndexingConfiguration,
	})
}

func (h *Handler) handleListIndices(c *echo.Context) error {
	names := h.Backend.ListIndices()

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(names, pageSize, start)

	resp := map[string]any{"indexNames": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleDescribeIndex(c *echo.Context) error {
	indexName := strings.TrimPrefix(c.Request().URL.Path, pathIndices+"/")

	idx, err := h.Backend.DescribeIndex(indexName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"indexName":   idx.IndexName,
		"indexStatus": idx.IndexStatus,
		"schema":      idx.Schema,
	})
}

func (h *Handler) handleSearchIndex(c *echo.Context) error {
	var body struct {
		IndexName   string `json:"indexName"`
		QueryString string `json:"queryString"`
		NextToken   string `json:"nextToken"`
		MaxResults  int32  `json:"maxResults"`
	}

	if err := decodeJSONBody(c, &body); err != nil {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	out, err := h.Backend.SearchIndex(&SearchIndexInput{
		IndexName:   body.IndexName,
		QueryString: body.QueryString,
		NextToken:   body.NextToken,
		MaxResults:  body.MaxResults,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	resp := map[string]any{}
	if out.Things != nil {
		resp["things"] = out.Things
	}

	if out.ThingGroups != nil {
		resp["thingGroups"] = out.ThingGroups
	}

	if out.NextToken != "" {
		resp["nextToken"] = out.NextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// aggregationRequestBody is the common JSON request body shape shared by
// GetCardinality, GetStatistics, GetPercentiles and GetBucketsAggregation.
type aggregationRequestBody struct {
	IndexName              string    `json:"indexName"`
	QueryString            string    `json:"queryString"`
	AggregationField       string    `json:"aggregationField"`
	Percents               []float64 `json:"percents"`
	BucketsAggregationType struct {
		TermsAggregation struct {
			MaxBuckets int32 `json:"maxBuckets"`
		} `json:"termsAggregation"`
	} `json:"bucketsAggregationType"`
}

func (h *Handler) handleGetCardinality(c *echo.Context) error {
	var body aggregationRequestBody
	if err := decodeJSONBody(c, &body); err != nil {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	cardinality, err := h.Backend.GetCardinality(&AggregationInput{
		IndexName:        body.IndexName,
		QueryString:      body.QueryString,
		AggregationField: body.AggregationField,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"cardinality": cardinality})
}

func (h *Handler) handleGetStatistics(c *echo.Context) error {
	var body aggregationRequestBody
	if err := decodeJSONBody(c, &body); err != nil {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	stats, err := h.Backend.GetStatistics(&AggregationInput{
		IndexName:        body.IndexName,
		QueryString:      body.QueryString,
		AggregationField: body.AggregationField,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"statistics": map[string]any{
			"count":        stats.Count,
			"average":      stats.Average,
			"sum":          stats.Sum,
			"sumOfSquares": stats.SumOfSquares,
			"minimum":      stats.Minimum,
			"maximum":      stats.Maximum,
			"variance":     stats.Variance,
			"stdDeviation": stats.StdDeviation,
		},
	})
}

func (h *Handler) handleGetPercentiles(c *echo.Context) error {
	var body aggregationRequestBody
	if err := decodeJSONBody(c, &body); err != nil {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	percentiles, err := h.Backend.GetPercentiles(&PercentilesInput{
		IndexName:        body.IndexName,
		QueryString:      body.QueryString,
		AggregationField: body.AggregationField,
		Percents:         body.Percents,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	out := make([]map[string]float64, 0, len(percentiles))
	for _, p := range percentiles {
		out = append(out, map[string]float64{"percent": p.Percent, "value": p.Value})
	}

	return c.JSON(http.StatusOK, map[string]any{"percentiles": out})
}

func (h *Handler) handleGetBucketsAggregation(c *echo.Context) error {
	var body aggregationRequestBody
	if err := decodeJSONBody(c, &body); err != nil {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	buckets, err := h.Backend.GetBucketsAggregation(&BucketsAggregationInput{
		IndexName:        body.IndexName,
		QueryString:      body.QueryString,
		AggregationField: body.AggregationField,
		MaxBuckets:       body.BucketsAggregationType.TermsAggregation.MaxBuckets,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	out := make([]map[string]any, 0, len(buckets))

	var total int64

	for _, b := range buckets {
		out = append(out, map[string]any{"keyValue": b.KeyValue, "count": b.Count})
		total += b.Count
	}

	return c.JSON(http.StatusOK, map[string]any{"buckets": out, "totalCount": total})
}
