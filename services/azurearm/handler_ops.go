package azurearm

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
)

// handleMetadataEndpoints serves GET /metadata/endpoints?api-version=2022-09-01.
func (h *Handler) handleMetadataEndpoints(c *echo.Context) error {
	baseURL := h.baseURLFor(c.Request())
	docs := BuildMetadataEndpoints(baseURL, h.Settings)

	return h.writeJSON(c, http.StatusOK, docs)
}

// handleOpenIDConfiguration serves GET /{tenant}/v2.0/.well-known/openid-configuration.
func (h *Handler) handleOpenIDConfiguration(c *echo.Context, tenant string) error {
	baseURL := h.baseURLFor(c.Request())
	cfg := aadauth.BuildOpenIDConfiguration(baseURL, tenant)

	return h.writeJSON(c, http.StatusOK, cfg)
}

// handleInstanceDiscovery serves GET /common/discovery/instance?api-version=1.1.
func (h *Handler) handleInstanceDiscovery(c *echo.Context) error {
	baseURL := h.baseURLFor(c.Request())
	resp := aadauth.BuildInstanceDiscoveryResponse(baseURL, h.Settings.TenantID)

	return h.writeJSON(c, http.StatusOK, resp)
}

// handleJWKS serves GET /{tenant}/discovery/v2.0/keys.
func (h *Handler) handleJWKS(c *echo.Context) error {
	return h.writeJSON(c, http.StatusOK, h.Issuer.JWKS())
}

// handleToken serves POST /{tenant}/oauth2/token and
// POST /{tenant}/oauth2/v2.0/token -- both return the identical body shape
// (AZURE.md section 10.1).
func (h *Handler) handleToken(c *echo.Context, tenant string) error {
	r := c.Request()

	form, err := parseFormOrJSONBody(r)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	clientID := form.Get("client_id")
	if clientID == "" {
		clientID = h.Settings.ClientID
	}

	scope := form.Get("scope")
	if scope == "" {
		scope = form.Get("resource") // v1 shape
	}

	baseURL := h.baseURLFor(r)

	resp, err := IssueToken(h.Issuer, baseURL, tenant, clientID, scope)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	return h.writeJSON(c, http.StatusOK, resp)
}

// handleListTenants serves GET /tenants.
func (h *Handler) handleListTenants(c *echo.Context) error {
	return h.writeJSON(c, http.StatusOK, map[string]any{
		"value": []map[string]any{TenantBody(h.Settings.TenantID)},
	})
}

// handleListSubscriptions serves GET /subscriptions.
func (h *Handler) handleListSubscriptions(c *echo.Context) error {
	return h.writeJSON(c, http.StatusOK, map[string]any{
		"value": []map[string]any{SubscriptionBody(h.Settings.SubscriptionID, "gopherstack")},
	})
}

// handleGetSubscription serves GET /subscriptions/{sub}.
func (h *Handler) handleGetSubscription(c *echo.Context, sub string) error {
	return h.writeJSON(c, http.StatusOK, SubscriptionBody(sub, "gopherstack"))
}

// handleListProviders serves GET /subscriptions/{sub}/providers.
func (h *Handler) handleListProviders(c *echo.Context, sub string) error {
	values := make([]map[string]any, 0, len(h.Registry.Providers()))

	for _, ns := range h.Registry.Providers() {
		p := h.Registry.providers[ns]
		state := h.Backend.ProviderRegistrationState(sub, ns)
		values = append(values, ProviderBody(sub, ns, state, p.ResourceTypes()))
	}

	return h.writeJSON(c, http.StatusOK, map[string]any{"value": values})
}

// handleGetProvider serves GET /subscriptions/{sub}/providers/{ns}.
func (h *Handler) handleGetProvider(c *echo.Context, sub, ns string) error {
	p, ok := h.Registry.providers[ns]
	if !ok {
		return h.writeAPIError(c, ErrProviderNotFound)
	}

	state := h.Backend.ProviderRegistrationState(sub, ns)

	return h.writeJSON(c, http.StatusOK, ProviderBody(sub, ns, state, p.ResourceTypes()))
}

// handleRegisterProvider serves POST /subscriptions/{sub}/providers/{ns}/register.
func (h *Handler) handleRegisterProvider(c *echo.Context, sub, ns string) error {
	p, ok := h.Registry.providers[ns]
	if !ok {
		return h.writeAPIError(c, ErrProviderNotFound)
	}

	h.Backend.RegisterProvider(sub, ns)

	return h.writeJSON(c, http.StatusOK, ProviderBody(sub, ns, "Registered", p.ResourceTypes()))
}

// handleListResourceGroups serves GET /subscriptions/{sub}/resourcegroups.
func (h *Handler) handleListResourceGroups(c *echo.Context, sub string) error {
	groups := h.Backend.ListResourceGroups()
	values := make([]map[string]any, 0, len(groups))

	for _, g := range groups {
		values = append(values, g.Body(sub))
	}

	return h.writeJSON(c, http.StatusOK, map[string]any{"value": values})
}

// handleResourceGroup serves PUT/GET/DELETE
// /subscriptions/{sub}/resourcegroups/{name} (case-insensitive on the
// "resourcegroups"/"resourceGroups" segment -- already normalized by the
// caller's EqualFold match in dispatchARMResource).
func (h *Handler) handleResourceGroup(c *echo.Context, sub, name string) error {
	switch c.Request().Method {
	case http.MethodPut:
		return h.putResourceGroup(c, sub, name)
	case http.MethodGet:
		return h.getResourceGroup(c, sub, name)
	case http.MethodDelete:
		return h.deleteResourceGroup(c, name)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed.")
	}
}

func (h *Handler) putResourceGroup(c *echo.Context, sub, name string) error {
	body, err := decodeJSONBody(c.Request())
	if err != nil {
		return h.writeAPIError(c, err)
	}

	location, _ := body["location"].(string)
	if location == "" {
		location = h.Settings.Location
	}

	group, created := h.Backend.PutResourceGroup(name, location, stringTags(body["tags"]))

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	return h.writeJSON(c, status, group.Body(sub))
}

func (h *Handler) getResourceGroup(c *echo.Context, sub, name string) error {
	group, err := h.Backend.GetResourceGroup(name)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	return h.writeJSON(c, http.StatusOK, group.Body(sub))
}

func (h *Handler) deleteResourceGroup(c *echo.Context, name string) error {
	if err := h.Backend.DeleteResourceGroup(name); err != nil {
		if isNotFoundOnDelete(err) {
			// ARM's DELETE is idempotent: deleting an already-absent
			// resource group still returns success.
			return c.NoContent(http.StatusNoContent)
		}

		return h.writeAPIError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// isNotFoundOnDelete reports whether err is the "not found" sentinel that
// DELETE should treat as an idempotent success rather than an error.
func isNotFoundOnDelete(err error) bool {
	return errors.Is(err, ErrResourceGroupNotFound) ||
		errors.Is(err, ErrResourceNotFound) ||
		errors.Is(err, ErrStorageAccountNotFound)
}

// handleListKeys serves POST .../{resourceId}/listKeys.
func (h *Handler) handleListKeys(c *echo.Context, resourceSegs []string) error {
	id, err := ParseGenericResourcePath("/" + joinSegs(resourceSegs))
	if err != nil {
		return h.writeAPIError(c, err)
	}

	resp, err := h.Registry.ListKeys(c.Request().Context(), id)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	return h.writeJSON(c, http.StatusOK, resp)
}

// handleListResources serves both list forms:
// GET /subscriptions/{sub}/providers/{ns}/{type} and
// GET /subscriptions/{sub}/resourceGroups/{rg}/providers/{ns}/{type}.
func (h *Handler) handleListResources(c *echo.Context, segs []string) error {
	id, _, err := ParseGenericResourceListPath("/" + joinSegs(segs))
	if err != nil {
		return h.writeAPIError(c, err)
	}

	values, err := h.Registry.List(c.Request().Context(), id)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	return h.writeJSON(c, http.StatusOK, map[string]any{"value": values})
}

// handleGenericResource serves PUT/GET/DELETE
// /subscriptions/{sub}/resourceGroups/{rg}/providers/{ns}/{type}/{name}[/{childType}/{childName}...],
// dispatched to the ResourceProvider registered for {ns} (or the generic
// pass-through store if none is registered) -- AZURE.md section 10.1's
// single generic path walker.
func (h *Handler) handleGenericResource(c *echo.Context) error {
	id, err := ParseGenericResourcePath(c.Request().URL.Path)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	switch c.Request().Method {
	case http.MethodPut:
		return h.putGenericResource(c, id)
	case http.MethodGet:
		return h.getGenericResource(c, id)
	case http.MethodDelete:
		return h.deleteGenericResourceHTTP(c, id)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed.")
	}
}

func (h *Handler) putGenericResource(c *echo.Context, id ResourceID) error {
	body, err := decodeJSONBody(c.Request())
	if err != nil {
		return h.writeAPIError(c, err)
	}

	respBody, created, err := h.Registry.Put(c.Request().Context(), id, body)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	return h.writeJSON(c, status, respBody)
}

func (h *Handler) getGenericResource(c *echo.Context, id ResourceID) error {
	respBody, err := h.Registry.Get(c.Request().Context(), id)
	if err != nil {
		return h.writeAPIError(c, err)
	}

	return h.writeJSON(c, http.StatusOK, respBody)
}

func (h *Handler) deleteGenericResourceHTTP(c *echo.Context, id ResourceID) error {
	if err := h.Registry.Delete(c.Request().Context(), id); err != nil {
		if isNotFoundOnDelete(err) {
			return c.NoContent(http.StatusNoContent)
		}

		return h.writeAPIError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// joinSegs re-joins path segments with "/", the inverse of splitARMPath.
func joinSegs(segs []string) string {
	out := ""

	for i, s := range segs {
		if i > 0 {
			out += "/"
		}

		out += s
	}

	return out
}
