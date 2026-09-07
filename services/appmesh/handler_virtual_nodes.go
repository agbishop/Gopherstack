package appmesh

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
)

// ─── VirtualNode dispatch ───

func (h *Handler) handleVirtualNodes(c *echo.Context, segs []string, meshName string) error {
	if err := h.checkMeshOwner(c); err != nil {
		return h.mapErr(c, err)
	}
	// /meshes/{meshName}/virtualNodes
	if len(segs) == segsSubCollection {
		switch c.Request().Method {
		case http.MethodPut:

			return h.handleCreateVirtualNode(c, meshName)
		case http.MethodGet:

			return h.handleListVirtualNodes(c, meshName)
		}

		return methodNotAllowed(c)
	}
	name := segs[3]
	// /meshes/{meshName}/virtualNodes/{name}
	switch c.Request().Method {
	case http.MethodGet:

		return h.handleDescribeVirtualNode(c, meshName, name)
	case http.MethodPut:

		return h.handleUpdateVirtualNode(c, meshName, name)
	case http.MethodDelete:

		return h.handleDeleteVirtualNode(c, meshName, name)
	}

	return methodNotAllowed(c)
}

// ─── VirtualNode handlers ───

func (h *Handler) handleCreateVirtualNode(c *echo.Context, meshName string) error {
	var body struct {
		VirtualNodeName string          `json:"virtualNodeName"`
		ClientToken     string          `json:"clientToken"`
		Spec            json.RawMessage `json:"spec"`
		Tags            []tagInput      `json:"tags"`
	}
	if err := c.Bind(&body); err != nil || !isValidResourceName(body.VirtualNodeName) {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "virtualNodeName is required"))
	}
	vn, err := h.Backend.CreateVirtualNode(meshName, body.VirtualNodeName, body.Spec, tagsToMap(body.Tags))
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vnToWire(vn))
}

func (h *Handler) handleDescribeVirtualNode(c *echo.Context, meshName, name string) error {
	vn, err := h.Backend.DescribeVirtualNode(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vnToWire(vn))
}

func (h *Handler) handleUpdateVirtualNode(c *echo.Context, meshName, name string) error {
	var body struct {
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "invalid request body"))
	}
	vn, err := h.Backend.UpdateVirtualNode(meshName, name, body.Spec)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vnToWire(vn))
}

func (h *Handler) handleDeleteVirtualNode(c *echo.Context, meshName, name string) error {
	vn, err := h.Backend.DeleteVirtualNode(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vnToWire(vn))
}

func (h *Handler) handleListVirtualNodes(c *echo.Context, meshName string) error {
	maxResults, nextToken := listParams(c)
	items, next, err := h.Backend.ListVirtualNodes(meshName, maxResults, nextToken)
	if err != nil {
		return h.mapErr(c, err)
	}
	refs := make([]any, 0, len(items))
	for _, vn := range items {
		refs = append(refs, vnSummaryToWire(vn))
	}

	return c.JSON(http.StatusOK, listResp("virtualNodes", refs, next))
}

// ─── VirtualNode wire serialization ───

func vnToWire(vn *VirtualNode) map[string]any {
	return map[string]any{
		keyMeshName:        vn.MeshName,
		keyVirtualNodeName: vn.VirtualNodeName,
		keyMetadata:        metaToWire(vn.Meta),
		keySpec:            specOrEmpty(vn.Spec),
		keyStatus:          map[string]any{keyStatus: vn.Status},
	}
}

func vnSummaryToWire(vn *VirtualNodeSummary) map[string]any {
	return map[string]any{
		keyArn:             vn.Arn,
		keyCreatedAt:       vn.CreatedAt.Unix(),
		keyLastUpdatedAt:   vn.UpdatedAt.Unix(),
		keyMeshName:        vn.MeshName,
		keyVirtualNodeName: vn.VirtualNodeName,
		keyMeshOwner:       vn.MeshOwner,
		keyResourceOwner:   vn.ResourceOwner,
		keyVersion:         vn.Version,
	}
}
