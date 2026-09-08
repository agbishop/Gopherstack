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

// extractCampaignsOp returns the campaigns collection operation name.
func extractCampaignsOp(method string) string {
	switch method {
	case http.MethodPost:
		return "CreateCampaign"
	case http.MethodGet:
		return "GetCampaigns"
	}

	return unknownOperation
}

func (h *Handler) extractCampaignSubOp(method, rest string) string {
	parts := strings.SplitN(rest, "/", dispatchSplitTwo)
	subPath := ""

	if len(parts) == dispatchSplitTwo {
		subPath = parts[1]
	}

	switch {
	case subPath == "":
		switch method {
		case http.MethodGet:
			return "GetCampaign"
		case http.MethodPut:
			return "UpdateCampaign"
		case http.MethodDelete:
			return "DeleteCampaign"
		}
	case subPath == "activities":
		return "GetCampaignActivities"
	case strings.HasPrefix(subPath, "kpis/daterange/"):
		return "GetCampaignDateRangeKpi"
	case subPath == subPathVersions:
		return "GetCampaignVersions"
	case strings.HasPrefix(subPath, subPathVersions+"/"):
		return "GetCampaignVersion"
	}

	return unknownOperation
}

func (h *Handler) dispatchCampaigns(c *echo.Context, appID string) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.handleCreateCampaign(c, appID)
	case http.MethodGet:
		return h.handleGetCampaigns(c, appID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

func (h *Handler) dispatchCampaignByID(c *echo.Context, appID, rest string) error {
	// rest can be: {id}, {id}/activities, {id}/kpis/daterange/{kpi}, {id}/versions, {id}/versions/{v}
	parts := strings.SplitN(rest, "/", dispatchSplitTwo)
	campaignID := parts[0]
	subPath := ""

	if len(parts) == dispatchSplitTwo {
		subPath = parts[1]
	}

	switch {
	case subPath == "":
		switch c.Request().Method {
		case http.MethodGet:
			return h.handleGetCampaign(c, appID, campaignID)
		case http.MethodPut:
			return h.handleUpdateCampaign(c, appID, campaignID)
		case http.MethodDelete:
			return h.handleDeleteCampaign(c, appID, campaignID)
		}
	case subPath == "activities":
		return h.handleGetCampaignActivities(c, appID, campaignID)
	case strings.HasPrefix(subPath, "kpis/daterange/"):
		kpiName := strings.TrimPrefix(subPath, "kpis/daterange/")

		return h.handleGetCampaignDateRangeKpi(c, appID, campaignID, kpiName)
	case subPath == subPathVersions:
		return h.handleGetCampaignVersions(c, appID, campaignID)
	case strings.HasPrefix(subPath, subPathVersions+"/"):
		versionStr := strings.TrimPrefix(subPath, "versions/")
		v, _ := parseVersionParam(versionStr)

		return h.handleGetCampaignVersion(c, appID, campaignID, v)
	}

	return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", "resource not found")
}

func (h *Handler) handleCreateCampaign(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req createCampaignRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	if strings.TrimSpace(req.Name) == "" {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "Name is required")
	}

	region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)

	campaign, backendErr := h.Backend.CreateCampaign(region, h.AccountID, appID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", backendErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusCreated, toCampaignResponse(campaign))

	return nil
}

// handleGetCampaign handles GET /v1/apps/{appId}/campaigns/{campaignId}.
func (h *Handler) handleGetCampaign(c *echo.Context, appID, campaignID string) error {
	campaign, err := h.Backend.GetCampaign(appID, campaignID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	resp := toCampaignResponse(campaign)
	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetCampaigns handles GET /v1/apps/{appId}/campaigns.
func (h *Handler) handleGetCampaigns(c *echo.Context, appID string) error {
	campaigns, err := h.Backend.GetCampaigns(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	offset, pageSize := parsePageParams(c)
	start, end, nextToken := applyPageParams(offset, pageSize, len(campaigns))

	items := make([]campaignResponse, 0, end-start)

	for _, c2 := range campaigns[start:end] {
		items = append(items, toCampaignResponse(c2))
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, pagedCampaignsResponse{
		NextToken: nextToken,
		Item:      items,
	})

	return nil
}

// handleUpdateCampaign handles PUT /v1/apps/{appId}/campaigns/{campaignId}.
func (h *Handler) handleUpdateCampaign(c *echo.Context, appID, campaignID string) error {
	var req updateCampaignRequest
	if !unmarshalBody(c, &req) {
		return nil
	}

	campaign, backendErr := h.Backend.UpdateCampaign(appID, campaignID, req)
	if backendErr != nil {
		return writeNotFoundOrInternal(c, backendErr)
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toCampaignResponse(campaign))

	return nil
}

// handleDeleteCampaign handles DELETE /v1/apps/{appId}/campaigns/{campaignId}.
func (h *Handler) handleDeleteCampaign(c *echo.Context, appID, campaignID string) error {
	campaign, err := h.Backend.DeleteCampaign(appID, campaignID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toCampaignResponse(campaign))

	return nil
}

// handleGetCampaignActivities handles GET /v1/apps/{appId}/campaigns/{campaignId}/activities.
func (h *Handler) handleGetCampaignActivities(c *echo.Context, appID, campaignID string) error {
	resp, err := h.Backend.GetCampaignActivities(appID, campaignID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetCampaignDateRangeKpi handles GET /v1/apps/{appId}/campaigns/{campaignId}/kpis/daterange/{kpiName}.
func (h *Handler) handleGetCampaignDateRangeKpi(c *echo.Context, appID, campaignID, kpiName string) error {
	start, end := parseKPIDateRange(c)

	resp, err := h.Backend.GetCampaignDateRangeKpi(appID, campaignID, kpiName, start, end)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// handleGetCampaignVersion handles GET /v1/apps/{appId}/campaigns/{campaignId}/versions/{version}.
func (h *Handler) handleGetCampaignVersion(c *echo.Context, appID, campaignID string, version int) error {
	campaign, err := h.Backend.GetCampaignVersion(appID, campaignID, version)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toCampaignResponse(campaign))

	return nil
}

// handleGetCampaignVersions handles GET /v1/apps/{appId}/campaigns/{campaignId}/versions.
func (h *Handler) handleGetCampaignVersions(c *echo.Context, appID, campaignID string) error {
	campaigns, err := h.Backend.GetCampaignVersions(appID, campaignID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	items := make([]campaignResponse, 0, len(campaigns))

	for _, c2 := range campaigns {
		items = append(items, toCampaignResponse(c2))
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, campaignVersionsResponse{Item: items})

	return nil
}

// ──────────────────────────────────────────────────
// Segment handlers
// ──────────────────────────────────────────────────

func toCampaignResponse(c *Campaign) campaignResponse {
	status := c.Status
	if status == "" {
		status = campaignStatusScheduled
	}

	return campaignResponse{
		ApplicationID:               c.ApplicationID,
		ARN:                         c.ARN,
		ID:                          c.ID,
		Name:                        c.Name,
		SegmentID:                   c.SegmentID,
		SegmentVersion:              c.SegmentVersion,
		Tags:                        c.Tags,
		MessageConfiguration:        c.MessageConfiguration,
		Schedule:                    c.Schedule,
		Hook:                        c.Hook,
		Limits:                      c.Limits,
		TemplateConfiguration:       c.TemplateConfiguration,
		CustomDeliveryConfiguration: c.CustomDeliveryConfiguration,
		TreatmentDescription:        c.TreatmentDescription,
		TreatmentName:               c.TreatmentName,
		AdditionalTreatments:        c.AdditionalTreatments,
		Priority:                    c.Priority,
		IsPaused:                    c.IsPaused,
		Version:                     c.Version,
		CreationDate:                c.CreationDate,
		LastModifiedDate:            c.LastModifiedDate,
		State:                       campaignState{CampaignStatus: status},
	}
}
