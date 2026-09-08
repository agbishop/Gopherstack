package appmesh

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
)

// ─── Mesh dispatch ───

func (h *Handler) handleMeshes(c *echo.Context, segs []string) error {
	// /meshes
	if len(segs) == segsCollection {
		switch c.Request().Method {
		case http.MethodPut:

			return h.handleCreateMesh(c)
		case http.MethodGet:

			return h.handleListMeshes(c)
		}

		return methodNotAllowed(c)
	}
	meshName := segs[1]

	// /meshes/{meshName}
	if len(segs) == segsSingle {
		switch c.Request().Method {
		case http.MethodGet:

			return h.handleDescribeMesh(c, meshName)
		case http.MethodPut:

			return h.handleUpdateMesh(c, meshName)
		case http.MethodDelete:

			return h.handleDeleteMesh(c, meshName)
		}

		return methodNotAllowed(c)
	}

	resource := segs[2]
	switch resource {
	case pathSegVirtualNodes:

		return h.handleVirtualNodes(c, segs, meshName)
	case pathSegVirtualRouters:

		return h.handleVirtualRouters(c, segs, meshName)
	case pathSegVirtualRouter:

		return h.handleRoutes(c, segs, meshName)
	case pathSegVirtualSvcs:

		return h.handleVirtualServices(c, segs, meshName)
	case pathSegVirtualGWs:

		return h.handleVirtualGateways(c, segs, meshName)
	case pathSegVirtualGW:

		return h.handleGatewayRoutes(c, segs, meshName)
	}

	return c.JSON(http.StatusNotFound, errResp("NotFoundException", "not found"))
}

// ─── Mesh handlers ───

func (h *Handler) handleCreateMesh(c *echo.Context) error {
	var body struct {
		MeshName    string          `json:"meshName"`
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
		Tags        []tagInput      `json:"tags"`
	}
	if err := c.Bind(&body); err != nil || !isValidResourceName(body.MeshName) {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "meshName is required"))
	}
	m, err := h.Backend.CreateMesh(body.MeshName, body.Spec, tagsToMap(body.Tags))
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, meshToWire(m))
}

func (h *Handler) handleDescribeMesh(c *echo.Context, meshName string) error {
	if err := h.checkMeshOwner(c); err != nil {
		return h.mapErr(c, err)
	}
	m, err := h.Backend.DescribeMesh(meshName)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, meshToWire(m))
}

func (h *Handler) handleUpdateMesh(c *echo.Context, meshName string) error {
	var body struct {
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "invalid request body"))
	}
	m, err := h.Backend.UpdateMesh(meshName, body.Spec)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, meshToWire(m))
}

func (h *Handler) handleDeleteMesh(c *echo.Context, meshName string) error {
	m, err := h.Backend.DeleteMesh(meshName)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, meshToWire(m))
}

func (h *Handler) handleListMeshes(c *echo.Context) error {
	maxResults, nextToken := listParams(c)
	items, next, err := h.Backend.ListMeshes(maxResults, nextToken)
	if err != nil {
		return h.mapErr(c, err)
	}
	refs := make([]any, 0, len(items))
	for _, ms := range items {
		refs = append(refs, meshSummaryToWire(ms))
	}

	return c.JSON(http.StatusOK, listResp("meshes", refs, next))
}

// ─── Mesh wire serialization ───

func meshToWire(m *Mesh) map[string]any {
	return map[string]any{
		keyMeshName: m.Name,
		keyMetadata: metaToWire(m.Meta),
		keySpec:     specOrEmpty(m.Spec),
		keyStatus:   map[string]any{keyStatus: m.Status},
	}
}

func meshSummaryToWire(ms *MeshSummary) map[string]any {
	return map[string]any{
		keyArn:           ms.Arn,
		keyCreatedAt:     ms.CreatedAt.Unix(),
		keyLastUpdatedAt: ms.UpdatedAt.Unix(),
		keyMeshName:      ms.Name,
		keyMeshOwner:     ms.MeshOwner,
		keyResourceOwner: ms.ResourceOwner,
		keyVersion:       ms.Version,
	}
}
