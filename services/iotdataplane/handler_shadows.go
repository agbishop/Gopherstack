package iotdataplane

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

// handleListThingsWithShadows processes GET /api/things/shadow/ListThingsWithShadows.
// Pagination: pageSize (primary) or maxResults (alias); default 25, max 100.
func (h *Handler) handleListThingsWithShadows(c *echo.Context) error {
	if c.Request().Method != http.MethodGet {
		return c.JSON(http.StatusMethodNotAllowed, map[string]string{keyError: errMethodNotAllowed})
	}

	things := h.Backend.ListThingsWithShadows()

	q := c.Request().URL.Query()
	nextTokenIn := q.Get("nextToken")
	pageSize := parsePageSize(q, defaultPageSize)

	startIdx := findCursorIndex(things, nextTokenIn)
	end := min(startIdx+pageSize, len(things))
	page := things[startIdx:end]

	if page == nil {
		page = []string{}
	}

	resp := map[string]any{
		"things":     page,
		keyTimestamp: time.Now().Unix(),
	}

	if end < len(things) {
		resp["nextToken"] = things[end]
	}

	return c.JSON(http.StatusOK, resp)
}

// handleShadow dispatches GET/POST/DELETE /things/{thingName}/shadow requests.
// Supports named shadows via path (/shadow/name/{name}) and query (?name=).
func (h *Handler) handleShadow(c *echo.Context) error {
	path := c.Request().URL.Path
	thingName, shadowNameFromPath := parseShadowPath(path)

	if thingName == "" {
		return invalidRequestResponse(c, "thingName is required")
	}

	// Path-style takes precedence over query-param style.
	shadowName := shadowNameFromPath
	if shadowName == "" {
		shadowName = c.Request().URL.Query().Get("name")
	}

	if err := validateShadowName(shadowName); err != nil {
		return h.handleError(c, err)
	}

	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetThingShadow(c, thingName, shadowName)
	case http.MethodPost:
		return h.handleUpdateThingShadow(c, thingName, shadowName)
	case http.MethodDelete:
		return h.handleDeleteThingShadow(c, thingName, shadowName)
	default:
		return methodNotAllowedResponse(c)
	}
}

// handleGetThingShadow processes GET /things/{thingName}/shadow.
func (h *Handler) handleGetThingShadow(c *echo.Context, thingName, shadowName string) error {
	doc, err := h.Backend.GetThingShadow(thingName, shadowName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Blob(http.StatusOK, "application/json", doc)
}

// handleUpdateThingShadow processes POST /things/{thingName}/shadow.
func (h *Handler) handleUpdateThingShadow(c *echo.Context, thingName, shadowName string) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxShadowBodyBytes)

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		tooLargeErr := fmt.Errorf("%w: shadow document exceeds %d bytes", ErrRequestTooLarge, maxShadowBodyBytes)

		return h.handleError(c, tooLargeErr)
	}

	updated, updateErr := h.Backend.UpdateThingShadow(thingName, shadowName, body)
	if updateErr != nil {
		return h.handleError(c, updateErr)
	}

	return c.Blob(http.StatusOK, "application/json", updated)
}

// handleDeleteThingShadow processes DELETE /things/{thingName}/shadow.
func (h *Handler) handleDeleteThingShadow(c *echo.Context, thingName, shadowName string) error {
	payload, err := h.Backend.DeleteThingShadow(thingName, shadowName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Blob(http.StatusOK, "application/json", payload)
}

// handleListNamedShadows processes GET /api/things/shadow/ListNamedShadowsForThing/{thingName}.
// Pagination: pageSize (primary) or maxResults (alias); default 25, max 100.
func (h *Handler) handleListNamedShadows(c *echo.Context) error {
	if c.Request().Method != http.MethodGet {
		return methodNotAllowedResponse(c)
	}

	thingName := strings.TrimPrefix(c.Request().URL.Path, listNamedShadowsPrefix)
	if thingName == "" {
		return invalidRequestResponse(c, "thingName is required")
	}

	if err := validateThingName(thingName); err != nil {
		return h.handleError(c, err)
	}

	names, err := h.Backend.ListNamedShadowsForThing(thingName)
	if err != nil {
		return h.handleError(c, err)
	}

	q := c.Request().URL.Query()
	nextTokenIn := q.Get("nextToken")
	pageSize := parsePageSize(q, defaultPageSize)

	startIdx := findCursorIndex(names, nextTokenIn)
	end := min(startIdx+pageSize, len(names))

	page := names[startIdx:end]
	if page == nil {
		page = []string{}
	}

	resp := map[string]any{
		"results":    page,
		keyTimestamp: time.Now().Unix(),
	}

	if end < len(names) {
		resp["nextToken"] = names[end]
	}

	return c.JSON(http.StatusOK, resp)
}
