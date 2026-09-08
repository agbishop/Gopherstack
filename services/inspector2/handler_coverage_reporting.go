package inspector2

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

const (
	opListCoverage           = "ListCoverage"
	opListCoverageStatistics = "ListCoverageStatistics"

	pathCoverageList           = "/coverage/list"
	pathCoverageStatisticsList = "/coverage/statistics/list"
)

func (h *Handler) handleListCoverage(c *echo.Context) error {
	req, ok := decodeFilterListRequest(c)
	if !ok {
		return nil
	}

	entries, nextToken, listErr := h.Backend.ListCoverage(req.FilterCriteria, req.MaxResults, req.NextToken)
	if listErr != nil {
		return h.mapError(c, listErr)
	}

	wire := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		wire = append(wire, coverageEntryToWire(e))
	}

	resp := map[string]any{"coveredResources": wire}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// coverageEntryToWire renders a CoverageEntry in its real CoveredResource
// wire shape. lastScannedAt is a DateTimeTimestamp member (see
// findingToWire's doc comment for why time.Time must never be marshaled
// directly for a restjson1 timestamp field).
func coverageEntryToWire(e *CoverageEntry) map[string]any {
	entry := map[string]any{
		keyAccountID:   e.AccountID,
		keyResourceID:  e.ResourceID,
		"resourceType": e.ResourceType,
		"scanType":     e.ScanType,
	}

	if !e.LastScannedAt.IsZero() {
		entry["lastScannedAt"] = awstime.Epoch(e.LastScannedAt)
	}

	if e.ScanMode != "" {
		entry["scanMode"] = e.ScanMode
	}

	if e.ScanStatus != nil {
		status := map[string]any{"statusCode": e.ScanStatus.StatusCode}
		if e.ScanStatus.Reason != "" {
			status["reason"] = e.ScanStatus.Reason
		}

		entry["scanStatus"] = status
	}

	return entry
}

func (h *Handler) handleListCoverageStatistics(c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid body"))
	}

	var req struct {
		FilterCriteria map[string]any `json:"filterCriteria"`
		GroupBy        string         `json:"groupBy"`
	}

	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
			return c.JSON(
				http.StatusBadRequest,
				errorResponse("ValidationException", "invalid JSON"),
			)
		}
	}

	stats, statsErr := h.Backend.ListCoverageStatistics(req.FilterCriteria, req.GroupBy)
	if statsErr != nil {
		return h.mapError(c, statsErr)
	}

	return c.JSON(http.StatusOK, stats)
}
