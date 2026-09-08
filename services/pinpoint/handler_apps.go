package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// extractAppsCollectionOp returns the operation for the apps collection.
func extractAppsCollectionOp(method string) string {
	switch method {
	case http.MethodPost:
		return "CreateApp"
	case http.MethodGet:
		return "GetApps"
	}

	return unknownOperation
}

// extractAppsResourceOp returns the operation for a specific app resource.
func extractAppsResourceOp(h *Handler, method, path string) string {
	suffix := strings.TrimPrefix(path, "/v1/apps/")
	if strings.Contains(suffix, "/") {
		return h.extractAppSubOperation(method, suffix)
	}

	switch method {
	case http.MethodGet:
		return "GetApp"
	case http.MethodDelete:
		return "DeleteApp"
	}

	return unknownOperation
}

func (h *Handler) dispatchApps(c *echo.Context) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.handleCreateApp(c)
	case http.MethodGet:
		return h.handleGetApps(c)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

func (h *Handler) dispatchApp(c *echo.Context, appID string) error {
	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetApp(c, appID)
	case http.MethodDelete:
		return h.handleDeleteApp(c, appID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

// dispatchAppSubPath handles paths under /v1/apps/{appId}/ (e.g. settings).
//

func (h *Handler) handleCreateApp(c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req createAppRequest

	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	if strings.TrimSpace(req.Name) == "" {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "Name is required")
	}

	region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)

	app, err := h.Backend.CreateApp(region, h.AccountID, req.Name, req.Tags)
	if err != nil {
		switch {
		case errors.Is(err, awserr.ErrInvalidParameter):
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", err.Error())
		case errors.Is(err, awserr.ErrConflict):
			return writeErrorResponse(c, http.StatusConflict, "ConflictException", err.Error())
		case errors.Is(err, awserr.ErrNotFound):
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		default:
			return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
		}
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusCreated, toAppResponse(app))

	return nil
}

func (h *Handler) handleGetApp(c *echo.Context, appID string) error {
	app, err := h.Backend.GetApp(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toAppResponse(app))

	return nil
}

func (h *Handler) handleDeleteApp(c *echo.Context, appID string) error {
	app, err := h.Backend.DeleteApp(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toAppResponse(app))

	return nil
}

func (h *Handler) handleGetApps(c *echo.Context) error {
	apps, err := h.Backend.GetApps()
	if err != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	items := make([]appResponse, 0, len(apps))

	for _, app := range apps {
		items = append(items, toAppResponse(app))
	}

	// Support pageSize and token query parameters for cursor-based pagination.
	// The Pinpoint REST API uses ?pageSize=N&token=<cursor>.
	q := c.Request().URL.Query()
	token := q.Get("token")

	var limit int

	if ps := q.Get("pageSize"); ps != "" {
		if n, parseErr := strconv.Atoi(ps); parseErr == nil && n > 0 {
			limit = n
		}
	}

	p := page.New(items, token, limit, pinpointDefaultPageSize)

	resp := appsResponse{Item: p.Data}
	if p.Next != "" {
		resp.NextToken = &p.Next
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}

// toAppResponse converts an App to the JSON wire format.
func toAppResponse(app *App) appResponse {
	return appResponse{
		ARN:          app.ARN,
		ID:           app.ID,
		Name:         app.Name,
		CreationDate: app.CreationDate,
		Tags:         app.Tags,
	}
}
