package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// extractSettingsOp returns the settings operation name.
func extractSettingsOp(method string) string {
	switch method {
	case http.MethodGet:
		return "GetApplicationSettings"
	case http.MethodPut:
		return "UpdateApplicationSettings"
	}

	return unknownOperation
}

// dispatchAppSettings handles GET/PUT /v1/apps/{appId}/settings.
func (h *Handler) dispatchAppSettings(c *echo.Context, appID string) error {
	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetApplicationSettings(c, appID)
	case http.MethodPut:
		return h.handleUpdateApplicationSettings(c, appID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

// handleGetApplicationSettings handles GET /v1/apps/{appId}/settings.
func (h *Handler) handleGetApplicationSettings(c *echo.Context, appID string) error {
	settings, err := h.Backend.GetApplicationSettings(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	resp := toAppSettingsResponse(appID, settings)

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleUpdateApplicationSettings handles PUT /v1/apps/{appId}/settings.
func (h *Handler) handleUpdateApplicationSettings(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var incoming struct {
		CampaignHook        map[string]any `json:"CampaignHook"`
		Limits              map[string]any `json:"Limits"`
		QuietTime           map[string]any `json:"QuietTime"`
		JourneyLimits       map[string]any `json:"JourneyLimits"`
		CloudWatchMetrics   bool           `json:"CloudWatchMetricsEnabled"`
		EventTaggingEnabled bool           `json:"EventTaggingEnabled"`
	}

	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &incoming); jsonErr != nil {
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
		}
	}

	settingsToStore := &StoredAppSettings{
		CampaignHook:        incoming.CampaignHook,
		Limits:              incoming.Limits,
		QuietTime:           incoming.QuietTime,
		JourneyLimits:       incoming.JourneyLimits,
		CloudWatchMetrics:   incoming.CloudWatchMetrics,
		EventTaggingEnabled: incoming.EventTaggingEnabled,
	}

	settings, updateErr := h.Backend.UpdateApplicationSettings(appID, settingsToStore)
	if updateErr != nil {
		if errors.Is(updateErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", updateErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", updateErr.Error())
	}

	resp := toAppSettingsResponse(appID, settings)

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// toAppSettingsResponse converts stored settings to the wire format, filling
// CampaignHook/Limits/QuietTime/JourneyLimits with non-nil empty objects.
func toAppSettingsResponse(appID string, settings *StoredAppSettings) appSettingsResponse {
	resp := appSettingsResponse{
		ApplicationID:            appID,
		LastModifiedDate:         settings.LastModifiedDate,
		CampaignHook:             settings.CampaignHook,
		Limits:                   settings.Limits,
		QuietTime:                settings.QuietTime,
		JourneyLimits:            settings.JourneyLimits,
		CloudWatchMetricsEnabled: settings.CloudWatchMetrics,
		EventTaggingEnabled:      settings.EventTaggingEnabled,
	}

	if resp.CampaignHook == nil {
		resp.CampaignHook = map[string]any{}
	}

	if resp.Limits == nil {
		resp.Limits = map[string]any{}
	}

	if resp.QuietTime == nil {
		resp.QuietTime = map[string]any{}
	}

	if resp.JourneyLimits == nil {
		resp.JourneyLimits = map[string]any{}
	}

	return resp
}

// handleGetApplicationDateRangeKpi handles GET /v1/apps/{appId}/kpis/daterange/{kpiName}.
func (h *Handler) handleGetApplicationDateRangeKpi(c *echo.Context, appID, kpiName string) error {
	start, end := parseKPIDateRange(c)

	resp, err := h.Backend.GetApplicationDateRangeKpi(appID, kpiName, start, end)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}
