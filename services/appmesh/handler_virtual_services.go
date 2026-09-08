package appmesh

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
)

// ─── VirtualService dispatch ───

func (h *Handler) handleVirtualServices(c *echo.Context, segs []string, meshName string) error {
	if err := h.checkMeshOwner(c); err != nil {
		return h.mapErr(c, err)
	}
	// /meshes/{meshName}/virtualServices
	if len(segs) == segsSubCollection {
		switch c.Request().Method {
		case http.MethodPut:

			return h.handleCreateVirtualService(c, meshName)
		case http.MethodGet:

			return h.handleListVirtualServices(c, meshName)
		}

		return methodNotAllowed(c)
	}
	name := segs[3]
	// /meshes/{meshName}/virtualServices/{name}
	switch c.Request().Method {
	case http.MethodGet:

		return h.handleDescribeVirtualService(c, meshName, name)
	case http.MethodPut:

		return h.handleUpdateVirtualService(c, meshName, name)
	case http.MethodDelete:

		return h.handleDeleteVirtualService(c, meshName, name)
	}

	return methodNotAllowed(c)
}

// ─── VirtualService handlers ───

func (h *Handler) handleCreateVirtualService(c *echo.Context, meshName string) error {
	var body struct {
		VirtualServiceName string          `json:"virtualServiceName"`
		ClientToken        string          `json:"clientToken"`
		Spec               json.RawMessage `json:"spec"`
		Tags               []tagInput      `json:"tags"`
	}
	if err := c.Bind(&body); err != nil || !isValidResourceName(body.VirtualServiceName) {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "virtualServiceName is required"))
	}
	vs, err := h.Backend.CreateVirtualService(meshName, body.VirtualServiceName, body.Spec, tagsToMap(body.Tags))
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vsToWire(vs))
}

func (h *Handler) handleDescribeVirtualService(c *echo.Context, meshName, name string) error {
	vs, err := h.Backend.DescribeVirtualService(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vsToWire(vs))
}

func (h *Handler) handleUpdateVirtualService(c *echo.Context, meshName, name string) error {
	var body struct {
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "invalid request body"))
	}
	vs, err := h.Backend.UpdateVirtualService(meshName, name, body.Spec)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vsToWire(vs))
}

func (h *Handler) handleDeleteVirtualService(c *echo.Context, meshName, name string) error {
	vs, err := h.Backend.DeleteVirtualService(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vsToWire(vs))
}

func (h *Handler) handleListVirtualServices(c *echo.Context, meshName string) error {
	maxResults, nextToken := listParams(c)
	items, next, err := h.Backend.ListVirtualServices(meshName, maxResults, nextToken)
	if err != nil {
		return h.mapErr(c, err)
	}
	refs := make([]any, 0, len(items))
	for _, vs := range items {
		refs = append(refs, vsSummaryToWire(vs))
	}

	return c.JSON(http.StatusOK, listResp("virtualServices", refs, next))
}

// ─── VirtualService wire serialization ───

func vsToWire(vs *VirtualService) map[string]any {
	return map[string]any{
		keyMeshName:           vs.MeshName,
		keyVirtualServiceName: vs.VirtualServiceName,
		keyMetadata:           metaToWire(vs.Meta),
		keySpec:               specOrEmpty(vs.Spec),
		keyStatus:             map[string]any{keyStatus: vs.Status},
	}
}

func vsSummaryToWire(vs *VirtualServiceSummary) map[string]any {
	return map[string]any{
		keyArn:                vs.Arn,
		keyCreatedAt:          vs.CreatedAt.Unix(),
		keyLastUpdatedAt:      vs.UpdatedAt.Unix(),
		keyMeshName:           vs.MeshName,
		keyVirtualServiceName: vs.VirtualServiceName,
		keyMeshOwner:          vs.MeshOwner,
		keyResourceOwner:      vs.ResourceOwner,
		keyVersion:            vs.Version,
	}
}
