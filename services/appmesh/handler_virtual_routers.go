package appmesh

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
)

// ─── VirtualRouter dispatch ───

func (h *Handler) handleVirtualRouters(c *echo.Context, segs []string, meshName string) error {
	if err := h.checkMeshOwner(c); err != nil {
		return h.mapErr(c, err)
	}
	// /meshes/{meshName}/virtualRouters
	if len(segs) == segsSubCollection {
		switch c.Request().Method {
		case http.MethodPut:

			return h.handleCreateVirtualRouter(c, meshName)
		case http.MethodGet:

			return h.handleListVirtualRouters(c, meshName)
		}

		return methodNotAllowed(c)
	}
	name := segs[3]
	// /meshes/{meshName}/virtualRouters/{name}
	switch c.Request().Method {
	case http.MethodGet:

		return h.handleDescribeVirtualRouter(c, meshName, name)
	case http.MethodPut:

		return h.handleUpdateVirtualRouter(c, meshName, name)
	case http.MethodDelete:

		return h.handleDeleteVirtualRouter(c, meshName, name)
	}

	return methodNotAllowed(c)
}

// ─── VirtualRouter handlers ───

func (h *Handler) handleCreateVirtualRouter(c *echo.Context, meshName string) error {
	var body struct {
		VirtualRouterName string          `json:"virtualRouterName"`
		ClientToken       string          `json:"clientToken"`
		Spec              json.RawMessage `json:"spec"`
		Tags              []tagInput      `json:"tags"`
	}
	if err := c.Bind(&body); err != nil || !isValidResourceName(body.VirtualRouterName) {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "virtualRouterName is required"))
	}
	vr, err := h.Backend.CreateVirtualRouter(meshName, body.VirtualRouterName, body.Spec, tagsToMap(body.Tags))
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vrToWire(vr))
}

func (h *Handler) handleDescribeVirtualRouter(c *echo.Context, meshName, name string) error {
	vr, err := h.Backend.DescribeVirtualRouter(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vrToWire(vr))
}

func (h *Handler) handleUpdateVirtualRouter(c *echo.Context, meshName, name string) error {
	var body struct {
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "invalid request body"))
	}
	vr, err := h.Backend.UpdateVirtualRouter(meshName, name, body.Spec)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vrToWire(vr))
}

func (h *Handler) handleDeleteVirtualRouter(c *echo.Context, meshName, name string) error {
	vr, err := h.Backend.DeleteVirtualRouter(meshName, name)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, vrToWire(vr))
}

func (h *Handler) handleListVirtualRouters(c *echo.Context, meshName string) error {
	maxResults, nextToken := listParams(c)
	items, next, err := h.Backend.ListVirtualRouters(meshName, maxResults, nextToken)
	if err != nil {
		return h.mapErr(c, err)
	}
	refs := make([]any, 0, len(items))
	for _, vr := range items {
		refs = append(refs, vrSummaryToWire(vr))
	}

	return c.JSON(http.StatusOK, listResp("virtualRouters", refs, next))
}

// ─── VirtualRouter wire serialization ───

func vrToWire(vr *VirtualRouter) map[string]any {
	return map[string]any{
		keyMeshName:          vr.MeshName,
		keyVirtualRouterName: vr.VirtualRouterName,
		keyMetadata:          metaToWire(vr.Meta),
		keySpec:              specOrEmpty(vr.Spec),
		keyStatus:            map[string]any{keyStatus: vr.Status},
	}
}

func vrSummaryToWire(vr *VirtualRouterSummary) map[string]any {
	return map[string]any{
		keyArn:               vr.Arn,
		keyCreatedAt:         vr.CreatedAt.Unix(),
		keyLastUpdatedAt:     vr.UpdatedAt.Unix(),
		keyMeshName:          vr.MeshName,
		keyVirtualRouterName: vr.VirtualRouterName,
		keyMeshOwner:         vr.MeshOwner,
		keyResourceOwner:     vr.ResourceOwner,
		keyVersion:           vr.Version,
	}
}

// ─── Route dispatch (singular virtualRouter in path) ───

func (h *Handler) handleRoutes(c *echo.Context, segs []string, meshName string) error {
	if err := h.checkMeshOwner(c); err != nil {
		return h.mapErr(c, err)
	}
	// /meshes/{meshName}/virtualRouter/{vrName}/routes
	if len(segs) < segsSubSingle {
		return c.JSON(http.StatusNotFound, errResp("NotFoundException", "not found"))
	}
	vrName := segs[3]
	if len(segs) == segsSubSingle || segs[4] != pathSegRoutes {
		return c.JSON(http.StatusNotFound, errResp("NotFoundException", "not found"))
	}
	// /meshes/{meshName}/virtualRouter/{vrName}/routes
	if len(segs) == segsNestedCollection {
		switch c.Request().Method {
		case http.MethodPut:

			return h.handleCreateRoute(c, meshName, vrName)
		case http.MethodGet:

			return h.handleListRoutes(c, meshName, vrName)
		}

		return methodNotAllowed(c)
	}
	routeName := segs[segsNestedSingle-1]
	// /meshes/{meshName}/virtualRouter/{vrName}/routes/{routeName}
	switch c.Request().Method {
	case http.MethodGet:

		return h.handleDescribeRoute(c, meshName, vrName, routeName)
	case http.MethodPut:

		return h.handleUpdateRoute(c, meshName, vrName, routeName)
	case http.MethodDelete:

		return h.handleDeleteRoute(c, meshName, vrName, routeName)
	}

	return methodNotAllowed(c)
}

// ─── Route handlers ───

func (h *Handler) handleCreateRoute(c *echo.Context, meshName, vrName string) error {
	var body struct {
		RouteName   string          `json:"routeName"`
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
		Tags        []tagInput      `json:"tags"`
	}
	if err := c.Bind(&body); err != nil || !isValidResourceName(body.RouteName) {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "routeName is required"))
	}
	r, err := h.Backend.CreateRoute(meshName, vrName, body.RouteName, body.Spec, tagsToMap(body.Tags))
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, routeToWire(r))
}

func (h *Handler) handleDescribeRoute(c *echo.Context, meshName, vrName, routeName string) error {
	r, err := h.Backend.DescribeRoute(meshName, vrName, routeName)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, routeToWire(r))
}

func (h *Handler) handleUpdateRoute(c *echo.Context, meshName, vrName, routeName string) error {
	var body struct {
		ClientToken string          `json:"clientToken"`
		Spec        json.RawMessage `json:"spec"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", "invalid request body"))
	}
	r, err := h.Backend.UpdateRoute(meshName, vrName, routeName, body.Spec)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, routeToWire(r))
}

func (h *Handler) handleDeleteRoute(c *echo.Context, meshName, vrName, routeName string) error {
	r, err := h.Backend.DeleteRoute(meshName, vrName, routeName)
	if err != nil {
		return h.mapErr(c, err)
	}

	return c.JSON(http.StatusOK, routeToWire(r))
}

func (h *Handler) handleListRoutes(c *echo.Context, meshName, vrName string) error {
	maxResults, nextToken := listParams(c)
	items, next, err := h.Backend.ListRoutes(meshName, vrName, maxResults, nextToken)
	if err != nil {
		return h.mapErr(c, err)
	}
	refs := make([]any, 0, len(items))
	for _, r := range items {
		refs = append(refs, routeSummaryToWire(r))
	}

	return c.JSON(http.StatusOK, listResp("routes", refs, next))
}

// ─── Route wire serialization ───

func routeToWire(r *Route) map[string]any {
	return map[string]any{
		keyMeshName:          r.MeshName,
		keyVirtualRouterName: r.VirtualRouterName,
		keyRouteName:         r.RouteName,
		keyMetadata:          metaToWire(r.Meta),
		keySpec:              specOrEmpty(r.Spec),
		keyStatus:            map[string]any{keyStatus: r.Status},
	}
}

func routeSummaryToWire(r *RouteSummary) map[string]any {
	return map[string]any{
		keyArn:               r.Arn,
		keyCreatedAt:         r.CreatedAt.Unix(),
		keyLastUpdatedAt:     r.UpdatedAt.Unix(),
		keyMeshName:          r.MeshName,
		keyVirtualRouterName: r.VirtualRouterName,
		keyRouteName:         r.RouteName,
		keyMeshOwner:         r.MeshOwner,
		keyResourceOwner:     r.ResourceOwner,
		keyVersion:           r.Version,
	}
}
