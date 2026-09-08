package lambda

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// --- Capacity provider handlers ---

// handleCapacityProviderRoute dispatches /2025-11-30/capacity-providers routes.
func (h *Handler) handleCapacityProviderRoute(c *echo.Context, path, method string) error {
	lambdaBk, ok := h.Backend.(*InMemoryBackend)
	if !ok {
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", "backend not available")
	}

	rest := strings.TrimPrefix(path, lambdaCapacityPathPrefix)
	rest = strings.TrimPrefix(rest, "/")

	// /2025-11-30/capacity-providers → Create / List
	if rest == "" {
		switch method {
		case http.MethodPost:
			return h.handleCreateCapacityProvider(c, lambdaBk)
		case http.MethodGet:
			return h.handleListCapacityProviders(c, lambdaBk)
		default:
			return h.writeError(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
		}
	}

	// /2025-11-30/capacity-providers/{name}/function-versions → ListFunctionVersionsByCapacityProvider
	if strings.HasSuffix(rest, "/function-versions") && method == http.MethodGet {
		cpName := strings.TrimSuffix(rest, "/function-versions")

		return h.handleListFunctionVersionsByCapacityProvider(c, lambdaBk, cpName)
	}

	// /2025-11-30/capacity-providers/{name} → Get / Delete / Update
	name, _, _ := strings.Cut(rest, "/")

	switch method {
	case http.MethodGet:
		return h.handleGetCapacityProvider(c, lambdaBk, name)
	case http.MethodDelete:
		return h.handleDeleteCapacityProvider(c, lambdaBk, name)
	case http.MethodPut:
		return h.handleUpdateCapacityProvider(c, lambdaBk, name)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
	}
}

// handleCreateCapacityProvider handles POST /2025-11-30/capacity-providers.
func (h *Handler) handleCreateCapacityProvider(c *echo.Context, bk *InMemoryBackend) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "failed to read body")
	}

	var input CreateCapacityProviderInput
	if len(body) > 0 {
		if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
			return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "invalid JSON")
		}
	}

	if input.CapacityProviderName == "" {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			"CapacityProviderName is required")
	}

	if input.PermissionsConfig == nil || input.PermissionsConfig.CapacityProviderOperatorRoleArn == "" {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			"PermissionsConfig.CapacityProviderOperatorRoleArn is required")
	}

	if input.VpcConfig == nil || len(input.VpcConfig.SubnetIDs) == 0 || len(input.VpcConfig.SecurityGroupIDs) == 0 {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException",
			"VpcConfig.SubnetIds and VpcConfig.SecurityGroupIds are required")
	}

	cp, createErr := bk.CreateCapacityProvider(&input)
	if createErr != nil {
		if errors.Is(createErr, ErrFunctionAlreadyExists) {
			return h.writeError(c, http.StatusConflict, "ResourceConflictException",
				"Capacity provider already exists: "+input.CapacityProviderName)
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", createErr.Error())
	}

	return c.JSON(http.StatusCreated, &CreateCapacityProviderOutput{CapacityProvider: cp})
}

// handleGetCapacityProvider handles GET /2025-11-30/capacity-providers/{name}.
func (h *Handler) handleGetCapacityProvider(c *echo.Context, bk *InMemoryBackend, name string) error {
	cp, err := bk.GetCapacityProvider(name)
	if err != nil {
		if errors.Is(err, ErrFunctionNotFound) {
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Capacity provider not found: "+name)
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", err.Error())
	}

	return c.JSON(http.StatusOK, &CreateCapacityProviderOutput{CapacityProvider: cp})
}

// handleDeleteCapacityProvider handles DELETE /2025-11-30/capacity-providers/{name}.
// Real AWS returns 200 with the deleted provider's state (CapacityProvider is a
// required output member), not an empty 204.
func (h *Handler) handleDeleteCapacityProvider(c *echo.Context, bk *InMemoryBackend, name string) error {
	cp, err := bk.DeleteCapacityProvider(name)
	if err != nil {
		if errors.Is(err, ErrFunctionNotFound) {
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Capacity provider not found: "+name)
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", err.Error())
	}

	return c.JSON(http.StatusOK, &DeleteCapacityProviderOutput{CapacityProvider: cp})
}

// handleUpdateCapacityProvider handles PUT /2025-11-30/capacity-providers/{name}.
//
//nolint:dupl // similar update-handler structure shared with handleUpdateCodeSigningConfig by design
func (h *Handler) handleUpdateCapacityProvider(c *echo.Context, bk *InMemoryBackend, name string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "failed to read body")
	}

	var input UpdateCapacityProviderInput
	if len(body) > 0 {
		if unmarshalErr := json.Unmarshal(body, &input); unmarshalErr != nil {
			return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", "invalid JSON")
		}
	}

	cp, updateErr := bk.UpdateCapacityProvider(name, &input)
	if updateErr != nil {
		if errors.Is(updateErr, ErrFunctionNotFound) {
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Capacity provider not found: "+name)
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", updateErr.Error())
	}

	return c.JSON(http.StatusOK, &UpdateCapacityProviderOutput{CapacityProvider: cp})
}

// handleListCapacityProviders handles GET /2025-11-30/capacity-providers.
func (h *Handler) handleListCapacityProviders(c *echo.Context, bk *InMemoryBackend) error {
	cps := bk.ListCapacityProviders()

	return c.JSON(http.StatusOK, &ListCapacityProvidersOutput{CapacityProviders: cps})
}

// --- ListFunctionVersionsByCapacityProvider ---

// functionVersionsByCapacityProviderListItem mirrors
// types.FunctionVersionsByCapacityProviderListItem: each entry on the wire is
// an object with FunctionArn and State, not a bare ARN string.
type functionVersionsByCapacityProviderListItem struct {
	FunctionArn string `json:"FunctionArn"`
	State       string `json:"State"`
}

type listFunctionVersionsByCapacityProviderOutput struct {
	NextMarker          string                                       `json:"NextMarker,omitempty"`
	CapacityProviderArn string                                       `json:"CapacityProviderArn"`
	FunctionVersions    []functionVersionsByCapacityProviderListItem `json:"FunctionVersions"`
}

// handleListFunctionVersionsByCapacityProvider returns the function-version ARNs
// assigned to the named capacity provider, with Marker/MaxItems pagination. It
// returns ResourceNotFoundException when the provider does not exist.
//
// AWS exposes no public API to assign function versions to a capacity provider in
// this emulator's surface, so assignments are populated only via the internal
// SeedCapacityProviderFunctionVersions helper (used by tests). When no versions
// have been seeded, an empty list is returned for a valid provider. This backend
// doesn't track per-assignment lifecycle state, so every seeded version reports
// State "Active" -- the value real ECS-managed function versions settle into.
func (h *Handler) handleListFunctionVersionsByCapacityProvider(
	c *echo.Context, bk *InMemoryBackend, name string,
) error {
	marker, maxItems := parsePaginationParams(c.Request())

	p, err := bk.ListFunctionVersionsByCapacityProvider(name, marker, maxItems)
	if err != nil {
		return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
			"Capacity provider not found: "+name)
	}

	cp, err := bk.GetCapacityProvider(name)
	if err != nil {
		return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
			"Capacity provider not found: "+name)
	}

	items := make([]functionVersionsByCapacityProviderListItem, 0, len(p.Data))
	for _, versionArn := range p.Data {
		items = append(items, functionVersionsByCapacityProviderListItem{
			FunctionArn: versionArn,
			State:       string(FunctionStateActive),
		})
	}

	return c.JSON(http.StatusOK, &listFunctionVersionsByCapacityProviderOutput{
		FunctionVersions:    items,
		NextMarker:          p.Next,
		CapacityProviderArn: cp.CapacityProviderArn,
	})
}
