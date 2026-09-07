package appmesh

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
)

// ─── VirtualGateway dispatch ───

func (h *Handler) handleVirtualGateways(c *echo.Context, segs []string, meshName string) error {
	if err := h.checkMeshOwner(c); err != nil {
		return h.mapErr(c, err)
	}
	// /meshes/{meshName}/virtualGateways
	if len(segs) == segsSubCollection {
		switch c.Request().Method {
		case http.MethodPut:

			return h.handleCreateVirtualGateway(c, meshName)
		case http.MethodGet:

			return h.handleListVirtualGateways(c, meshName)
		}

		return methodNotAllowed(c)
	}
	name := segs[3]
	// /meshes/{meshName}/virtualGateways/{name}
	switch c.Request().Method {
	case http.MethodGet:

		return h.handleDescribeVirtualGateway(c, meshName, name)
	case http.MethodPut:

		return h.handleUpdateVirtualGateway(c, meshName, name)
	case http.MethodDelete:

		return h.handleDeleteVirtualGateway(c, meshName, name)
	}

	return methodNotAllowed(c)
}

// ─── VirtualGateway handlers ───

func (h *Handler) handleCreateVirtualGateway(c *echo.Context, meshName string) error {
	var body struct {
		VirtualGatewayName string          `json:"virtualGatewayName"`
		ClientToken        string          `json:"clientToken"`
		Spec               json.RawMessage `json:"spec"`
		Tags               []tagInput      `json:"tags"`
	}
	if err := c.Bind(&body); err != nil || !isValidResourceName(body.VirtualGatewayName) {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "virtualGatewayName is required"))
	}
	vg, err := h.Backend.CreateVirtualGateway(meshName, body.VirtualGatewayName, body.Spec, tagsToMap(body.Tags))
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vgToWire(vg))
}

func (h *Handler) handleDescribeVirtualGateway(c *echo.Context, meshName, name string) error {
	vg, err := h.Backend.DescribeVirtualGateway(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vgToWire(vg))
}

func (h *Handler) handleUpdateVirtualGateway(c *echo.Context, meshName, name string) error {
	var body struct {
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "invalid request body"))
	}
	vg, err := h.Backend.UpdateVirtualGateway(meshName, name, body.Spec)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vgToWire(vg))
}

func (h *Handler) handleDeleteVirtualGateway(c *echo.Context, meshName, name string) error {
	vg, err := h.Backend.DeleteVirtualGateway(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vgToWire(vg))
}

func (h *Handler) handleListVirtualGateways(c *echo.Context, meshName string) error {
	maxResults, nextToken := listParams(c)
	items, next, err := h.Backend.ListVirtualGateways(meshName, maxResults, nextToken)
	if err != nil {
		return h.mapErr(c, err)
	}
	refs := make([]any, 0, len(items))
	for _, vg := range items {
		refs = append(refs, vgSummaryToWire(vg))
	}

	return c.JSON(http.StatusOK, listResp("virtualGateways", refs, next))
}

// ─── VirtualGateway wire serialization ───

func vgToWire(vg *VirtualGateway) map[string]any {
	return map[string]any{
		keyMeshName:           vg.MeshName,
		keyVirtualGatewayName: vg.VirtualGatewayName,
		keyMetadata:           metaToWire(vg.Meta),
		keySpec:               specOrEmpty(vg.Spec),
		keyStatus:             map[string]any{keyStatus: vg.Status},
	}
}

func vgSummaryToWire(vg *VirtualGatewaySummary) map[string]any {
	return map[string]any{
		keyArn:                vg.Arn,
		keyCreatedAt:          vg.CreatedAt.Unix(),
		keyLastUpdatedAt:      vg.UpdatedAt.Unix(),
		keyMeshName:           vg.MeshName,
		keyVirtualGatewayName: vg.VirtualGatewayName,
		keyMeshOwner:          vg.MeshOwner,
		keyResourceOwner:      vg.ResourceOwner,
		keyVersion:            vg.Version,
	}
}

// ─── GatewayRoute dispatch (singular virtualGateway in path) ───

func (h *Handler) handleGatewayRoutes(c *echo.Context, segs []string, meshName string) error {
	if err := h.checkMeshOwner(c); err != nil {
		return h.mapErr(c, err)
	}
	// /meshes/{meshName}/virtualGateway/{vgName}/gatewayRoutes[/{routeName}]
	if len(segs) < segsSubSingle {
		return c.JSON(http.StatusNotFound, errResp("NotFoundException", "not found"))
	}
	vgName := segs[3]
	if len(segs) == segsSubSingle || segs[4] != pathSegGatewayRoutes {
		return c.JSON(http.StatusNotFound, errResp("NotFoundException", "not found"))
	}
	// /meshes/{meshName}/virtualGateway/{vgName}/gatewayRoutes
	if len(segs) == segsNestedCollection {
		switch c.Request().Method {
		case http.MethodPut:

			return h.handleCreateGatewayRoute(c, meshName, vgName)
		case http.MethodGet:

			return h.handleListGatewayRoutes(c, meshName, vgName)
		}

		return methodNotAllowed(c)
	}
	routeName := segs[segsNestedSingle-1]
	// /meshes/{meshName}/virtualGateway/{vgName}/gatewayRoutes/{routeName}
	switch c.Request().Method {
	case http.MethodGet:

		return h.handleDescribeGatewayRoute(c, meshName, vgName, routeName)
	case http.MethodPut:

		return h.handleUpdateGatewayRoute(c, meshName, vgName, routeName)
	case http.MethodDelete:

		return h.handleDeleteGatewayRoute(c, meshName, vgName, routeName)
	}

	return methodNotAllowed(c)
}

// ─── GatewayRoute handlers ───

func (h *Handler) handleCreateGatewayRoute(c *echo.Context, meshName, vgName string) error {
	var body struct {
		GatewayRouteName string          `json:"gatewayRouteName"`
		ClientToken      string          `json:"clientToken"`
		Spec             json.RawMessage `json:"spec"`
		Tags             []tagInput      `json:"tags"`
	}
	if err := c.Bind(&body); err != nil || !isValidResourceName(body.GatewayRouteName) {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "gatewayRouteName is required"))
	}
	gr, err := h.Backend.CreateGatewayRoute(meshName, vgName, body.GatewayRouteName, body.Spec, tagsToMap(body.Tags))
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, grToWire(gr))
}

func (h *Handler) handleDescribeGatewayRoute(c *echo.Context, meshName, vgName, routeName string) error {
	gr, err := h.Backend.DescribeGatewayRoute(meshName, vgName, routeName)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, grToWire(gr))
}

func (h *Handler) handleUpdateGatewayRoute(c *echo.Context, meshName, vgName, routeName string) error {
	var body struct {
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "invalid request body"))
	}
	gr, err := h.Backend.UpdateGatewayRoute(meshName, vgName, routeName, body.Spec)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, grToWire(gr))
}

func (h *Handler) handleDeleteGatewayRoute(c *echo.Context, meshName, vgName, routeName string) error {
	gr, err := h.Backend.DeleteGatewayRoute(meshName, vgName, routeName)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, grToWire(gr))
}

func (h *Handler) handleListGatewayRoutes(c *echo.Context, meshName, vgName string) error {
	maxResults, nextToken := listParams(c)
	items, next, err := h.Backend.ListGatewayRoutes(meshName, vgName, maxResults, nextToken)
	if err != nil {
		return h.mapErr(c, err)
	}
	refs := make([]any, 0, len(items))
	for _, gr := range items {
		refs = append(refs, grSummaryToWire(gr))
	}

	return c.JSON(http.StatusOK, listResp("gatewayRoutes", refs, next))
}

// ─── GatewayRoute wire serialization ───

func grToWire(gr *GatewayRoute) map[string]any {
	return map[string]any{
		keyMeshName:           gr.MeshName,
		keyVirtualGatewayName: gr.VirtualGatewayName,
		keyGatewayRouteName:   gr.GatewayRouteName,
		keyMetadata:           metaToWire(gr.Meta),
		keySpec:               specOrEmpty(gr.Spec),
		keyStatus:             map[string]any{keyStatus: gr.Status},
	}
}

func grSummaryToWire(gr *GatewayRouteSummary) map[string]any {
	return map[string]any{
		keyArn:                gr.Arn,
		keyCreatedAt:          gr.CreatedAt.Unix(),
		keyLastUpdatedAt:      gr.UpdatedAt.Unix(),
		keyMeshName:           gr.MeshName,
		keyVirtualGatewayName: gr.VirtualGatewayName,
		keyGatewayRouteName:   gr.GatewayRouteName,
		keyMeshOwner:          gr.MeshOwner,
		keyResourceOwner:      gr.ResourceOwner,
		keyVersion:            gr.Version,
	}
}
