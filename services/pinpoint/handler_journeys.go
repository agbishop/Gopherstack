package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// extractJourneysOp returns the journeys collection operation name.
func extractJourneysOp(method string) string {
	switch method {
	case http.MethodPost:
		return "CreateJourney"
	case http.MethodGet:
		return "ListJourneys"
	}

	return unknownOperation
}

func (h *Handler) extractJourneySubOp(method, rest string) string {
	parts := strings.SplitN(rest, "/", dispatchSplitTwo)
	subPath := ""

	if len(parts) == dispatchSplitTwo {
		subPath = parts[1]
	}

	switch {
	case subPath == "":
		switch method {
		case http.MethodGet:
			return "GetJourney"
		case http.MethodPut:
			return "UpdateJourney"
		case http.MethodDelete:
			return "DeleteJourney"
		}
	case subPath == "state":
		return "UpdateJourneyState"
	case strings.HasPrefix(subPath, "kpis/daterange/"):
		return "GetJourneyDateRangeKpi"
	case subPath == subPathExecutionMetrics:
		return "GetJourneyExecutionMetrics"
	case strings.HasPrefix(subPath, "activities/") && strings.HasSuffix(subPath, "/"+subPathExecutionMetrics):
		return "GetJourneyExecutionActivityMetrics"
	case subPath == "runs":
		return "GetJourneyRuns"
	case strings.HasPrefix(subPath, "runs/"):
		runRest := strings.TrimPrefix(subPath, "runs/")
		if strings.HasSuffix(runRest, "/"+subPathExecutionMetrics) {
			if strings.Contains(runRest, "/activities/") {
				return "GetJourneyRunExecutionActivityMetrics"
			}

			return "GetJourneyRunExecutionMetrics"
		}
	}

	return unknownOperation
}

func (h *Handler) dispatchJourneys(c *echo.Context, appID string) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.handleCreateJourney(c, appID)
	case http.MethodGet:
		return h.handleListJourneys(c, appID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

func (h *Handler) dispatchJourneyByID(c *echo.Context, appID, rest string) error {
	// rest: {id}, {id}/state, {id}/kpis/daterange/{kpi},
	//       {id}/execution-metrics, {id}/activities/{aid}/execution-metrics,
	//       {id}/runs, {id}/runs/{rid}/execution-metrics,
	//       {id}/runs/{rid}/activities/{aid}/execution-metrics
	parts := strings.SplitN(rest, "/", dispatchSplitTwo)
	journeyID := parts[0]
	subPath := ""

	if len(parts) == dispatchSplitTwo {
		subPath = parts[1]
	}

	switch {
	case subPath == "":
		switch c.Request().Method {
		case http.MethodGet:
			return h.handleGetJourney(c, appID, journeyID)
		case http.MethodPut:
			return h.handleUpdateJourney(c, appID, journeyID)
		case http.MethodDelete:
			return h.handleDeleteJourney(c, appID, journeyID)
		}
	case subPath == "state":
		return h.handleUpdateJourneyState(c, appID, journeyID)
	case strings.HasPrefix(subPath, "kpis/daterange/"):
		kpiName := strings.TrimPrefix(subPath, "kpis/daterange/")

		return h.handleGetJourneyDateRangeKpi(c, appID, journeyID, kpiName)
	case subPath == subPathExecutionMetrics:
		return h.handleGetJourneyExecutionMetrics(c, appID, journeyID)
	case strings.HasPrefix(subPath, "activities/") && strings.HasSuffix(subPath, "/"+subPathExecutionMetrics):
		activityID := strings.TrimPrefix(subPath, "activities/")
		activityID = strings.TrimSuffix(activityID, "/"+subPathExecutionMetrics)

		return h.handleGetJourneyExecutionActivityMetrics(c, appID, journeyID, activityID)
	case subPath == "runs":
		return h.handleGetJourneyRuns(c, appID, journeyID)
	case strings.HasPrefix(subPath, "runs/"):
		return h.dispatchJourneyRun(c, appID, journeyID, strings.TrimPrefix(subPath, "runs/"))
	}

	return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", "resource not found")
}

func (h *Handler) dispatchJourneyRun(c *echo.Context, appID, journeyID, rest string) error {
	// rest: {runId}/execution-metrics, {runId}/activities/{aid}/execution-metrics
	parts := strings.SplitN(rest, "/", dispatchSplitTwo)
	runID := parts[0]
	subPath := ""

	if len(parts) == dispatchSplitTwo {
		subPath = parts[1]
	}

	switch {
	case subPath == subPathExecutionMetrics:
		return h.handleGetJourneyRunExecutionMetrics(c, appID, journeyID, runID)
	case strings.HasPrefix(subPath, "activities/") && strings.HasSuffix(subPath, "/"+subPathExecutionMetrics):
		activityID := strings.TrimPrefix(subPath, "activities/")
		activityID = strings.TrimSuffix(activityID, "/"+subPathExecutionMetrics)

		return h.handleGetJourneyRunExecutionActivityMetrics(c, appID, journeyID, runID, activityID)
	}

	return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", "resource not found")
}

// handleCreateJourney handles POST /v1/apps/{appId}/journeys.
func (h *Handler) handleCreateJourney(c *echo.Context, appID string) error {
	return h.handleCreateNamedAppResource(c, appID, func(body []byte, region, appID string) (any, error) {
		var req createJourneyRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, errInvalidRequestBody
		}

		if strings.TrimSpace(req.Name) == "" {
			return nil, errNameRequired
		}

		journey, err := h.Backend.CreateJourney(region, h.AccountID, appID, req)
		if err != nil {
			return nil, err
		}

		return toJourneyResponse(journey), nil
	})
}

// handleGetJourney handles GET /v1/apps/{appId}/journeys/{journeyId}.
func (h *Handler) handleGetJourney(c *echo.Context, appID, journeyID string) error {
	journey, err := h.Backend.GetJourney(appID, journeyID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toJourneyResponse(journey))

	return nil
}

// handleListJourneys handles GET /v1/apps/{appId}/journeys.
func (h *Handler) handleListJourneys(c *echo.Context, appID string) error {
	journeys, err := h.Backend.GetJourneys(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	offset, pageSize := parsePageParams(c)
	start, end, nextToken := applyPageParams(offset, pageSize, len(journeys))

	items := make([]journeyResponse, 0, end-start)

	for _, j := range journeys[start:end] {
		items = append(items, toJourneyResponse(j))
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, pagedJourneysResponse{
		NextToken: nextToken,
		Item:      items,
	})

	return nil
}

// handleUpdateJourney handles PUT /v1/apps/{appId}/journeys/{journeyId}.
func (h *Handler) handleUpdateJourney(c *echo.Context, appID, journeyID string) error {
	var req updateJourneyRequest
	if !unmarshalBody(c, &req) {
		return nil
	}

	journey, backendErr := h.Backend.UpdateJourney(appID, journeyID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrInvalidParameter) {
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", backendErr.Error())
		}

		if errors.Is(backendErr, awserr.ErrConflict) {
			return writeErrorResponse(c, http.StatusConflict, "ConflictException", backendErr.Error())
		}

		return writeNotFoundOrInternal(c, backendErr)
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toJourneyResponse(journey))

	return nil
}

// handleUpdateJourneyState handles PUT /v1/apps/{appId}/journeys/{journeyId}/state.
func (h *Handler) handleUpdateJourneyState(c *echo.Context, appID, journeyID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req updateJourneyStateRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	journey, backendErr := h.Backend.UpdateJourneyState(appID, journeyID, req.State)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", backendErr.Error())
		}

		if errors.Is(backendErr, awserr.ErrInvalidParameter) {
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", backendErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toJourneyResponse(journey))

	return nil
}

// handleDeleteJourney handles DELETE /v1/apps/{appId}/journeys/{journeyId}.
func (h *Handler) handleDeleteJourney(c *echo.Context, appID, journeyID string) error {
	journey, err := h.Backend.DeleteJourney(appID, journeyID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toJourneyResponse(journey))

	return nil
}

// handleGetJourneyDateRangeKpi handles GET /v1/apps/{appId}/journeys/{journeyId}/kpis/daterange/{kpiName}.
func (h *Handler) handleGetJourneyDateRangeKpi(c *echo.Context, appID, journeyID, kpiName string) error {
	start, end := parseKPIDateRange(c)

	resp, err := h.Backend.GetJourneyDateRangeKpi(appID, journeyID, kpiName, start, end)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetJourneyExecutionMetrics handles GET /v1/apps/{appId}/journeys/{journeyId}/execution-metrics.
func (h *Handler) handleGetJourneyExecutionMetrics(c *echo.Context, appID, journeyID string) error {
	resp, err := h.Backend.GetJourneyExecutionMetrics(appID, journeyID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetJourneyExecutionActivityMetrics handles GET journey activity execution metrics.
func (h *Handler) handleGetJourneyExecutionActivityMetrics(c *echo.Context, appID, journeyID, activityID string) error {
	resp, err := h.Backend.GetJourneyExecutionActivityMetrics(appID, journeyID, activityID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetJourneyRuns handles GET /v1/apps/{appId}/journeys/{journeyId}/runs.
func (h *Handler) handleGetJourneyRuns(c *echo.Context, appID, journeyID string) error {
	resp, err := h.Backend.GetJourneyRuns(appID, journeyID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetJourneyRunExecutionMetrics handles GET /v1/apps/{appId}/journeys/{journeyId}/runs/{runId}/execution-metrics.
func (h *Handler) handleGetJourneyRunExecutionMetrics(c *echo.Context, appID, journeyID, runID string) error {
	resp, err := h.Backend.GetJourneyRunExecutionMetrics(appID, journeyID, runID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetJourneyRunExecutionActivityMetrics handles GET journey run activity execution metrics.
func (h *Handler) handleGetJourneyRunExecutionActivityMetrics(
	c *echo.Context,
	appID, journeyID, runID, activityID string,
) error {
	resp, err := h.Backend.GetJourneyRunExecutionActivityMetrics(appID, journeyID, runID, activityID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// ──────────────────────────────────────────────────
// Template GET/UPDATE/DELETE handlers (non-create)
// ──────────────────────────────────────────────────

func toJourneyResponse(j *Journey) journeyResponse {
	return journeyResponse{
		ApplicationID:          j.ApplicationID,
		ARN:                    j.ARN,
		ID:                     j.ID,
		Name:                   j.Name,
		State:                  j.State,
		Activities:             j.Activities,
		StartCondition:         j.StartCondition,
		Schedule:               j.Schedule,
		Limits:                 j.Limits,
		QuietTime:              j.QuietTime,
		OpenHours:              j.OpenHours,
		ClosedDays:             j.ClosedDays,
		StartActivity:          j.StartActivity,
		RefreshFrequency:       j.RefreshFrequency,
		LocalTime:              j.LocalTime,
		WaitForQuietTime:       j.WaitForQuietTime,
		RefreshOnSegmentUpdate: j.RefreshOnSegmentUpdate,
		CreationDate:           j.CreationDate,
		LastModifiedDate:       j.LastModifiedDate,
	}
}
