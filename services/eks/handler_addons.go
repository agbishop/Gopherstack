package eks

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// dispatchAddonOps handles addon CRUD and static addon metadata operations.
func (h *Handler) dispatchAddonOps(c *echo.Context, route eksRoute, body []byte) (bool, error) {
	switch route.operation {
	case opCreateAddon:
		return true, h.handleCreateAddon(c, route.clusterName, body)
	case opDeleteAddon:
		return true, h.handleDeleteAddon(c, route.clusterName, route.nodegroupName)
	case opDescribeAddon:
		return true, h.handleDescribeAddon(c, route.clusterName, route.nodegroupName)
	case opListAddons:
		return true, h.handleListAddons(c, route.clusterName)
	case opUpdateAddon:
		return true, h.handleUpdateAddon(c, route.clusterName, route.nodegroupName, body)
	case opDescribeAddonVersions:
		return true, h.handleDescribeAddonVersions(c)
	case opDescribeAddonConfiguration:
		return true, h.handleDescribeAddonConfiguration(c)
	}

	return false, nil
}

// parseAddonRoute returns the route for /clusters/{name}/addons[/{addonName}] paths.
func parseAddonRoute(method, clusterName string, parts []string) eksRoute {
	const addonParts = 2

	if len(parts) == addonParts {
		switch method {
		case http.MethodPost:
			return eksRoute{operation: opCreateAddon, clusterName: clusterName}
		case http.MethodGet:
			return eksRoute{operation: opListAddons, clusterName: clusterName}
		}

		return eksRoute{operation: opUnknown}
	}

	tail := parts[2]

	// Real path is /clusters/{clusterName}/addons/{addonName}/update (POST),
	// NOT a bare-path PUT -- verified against the SDK serializer.
	if before, ok := strings.CutSuffix(tail, "/update"); ok {
		if method == http.MethodPost {
			return eksRoute{operation: opUpdateAddon, clusterName: clusterName, nodegroupName: before}
		}

		return eksRoute{operation: opUnknown}
	}

	switch method {
	case http.MethodGet:
		return eksRoute{operation: opDescribeAddon, clusterName: clusterName, nodegroupName: tail}
	case http.MethodDelete:
		return eksRoute{operation: opDeleteAddon, clusterName: clusterName, nodegroupName: tail}
	}

	return eksRoute{operation: opUnknown}
}

func addonToJSON(a *Addon) map[string]any {
	m := map[string]any{
		keyClusterName: a.ClusterName,
		"addonName":    a.AddonName,
		"addonArn":     a.ARN,
		keyStatusField: a.Status,
		keyCreatedAt:   a.CreatedAt.Unix(),
		"addonVersion": a.AddonVersion,
	}

	if a.Tags != nil {
		m["tags"] = a.Tags.Clone()
	} else {
		m["tags"] = map[string]string{}
	}

	if a.ServiceAccountRoleARN != "" {
		m["serviceAccountRoleArn"] = a.ServiceAccountRoleARN
	}

	// No "marketplaceVersion" or "resolveConflicts" keys: types.Addon
	// (eks@v1.90.4 deserializers.go's awsRestjson1_deserializeDocumentAddon)
	// has neither -- the real Marketplace field is the nested
	// "marketplaceInformation" object (productId/productUrl), which this
	// backend does not track, and resolveConflicts is a CreateAddon/
	// UpdateAddon request-only member, never echoed back.

	if a.Health != nil {
		m["health"] = map[string]any{
			"issues": a.Health.Issues,
		}
	}

	if a.Configuration != "" {
		m["configurationValues"] = a.Configuration
	}

	podAssocs := a.PodIdentityAssociations
	if podAssocs == nil {
		podAssocs = []string{}
	}

	m["podIdentityAssociations"] = podAssocs

	return m
}

type createAddonBody struct {
	Tags                  map[string]string `json:"tags"`
	AddonName             string            `json:"addonName"`
	AddonVersion          string            `json:"addonVersion"`
	ServiceAccountRoleArn string            `json:"serviceAccountRoleArn"`
	ConfigurationValues   string            `json:"configurationValues"`
	ResolveConflicts      string            `json:"resolveConflicts"`
}

func (h *Handler) handleCreateAddon(c *echo.Context, clusterName string, body []byte) error {
	var in createAddonBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", "invalid request body"))
	}

	if in.AddonName == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", "addonName is required"))
	}

	addon, err := h.Backend.CreateAddon(
		clusterName, in.AddonName, in.AddonVersion, in.ServiceAccountRoleArn,
		in.ConfigurationValues, in.ResolveConflicts,
		in.Tags,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyAddon: addonToJSON(addon),
	})
}

func (h *Handler) handleDeleteAddon(c *echo.Context, clusterName, addonName string) error {
	addon, err := h.Backend.DeleteAddon(clusterName, addonName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"addon": addonToJSON(addon),
	})
}

func (h *Handler) handleDescribeAddon(c *echo.Context, clusterName, addonName string) error {
	addon, err := h.Backend.DescribeAddon(clusterName, addonName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"addon": addonToJSON(addon),
	})
}

func (h *Handler) handleListAddons(c *echo.Context, clusterName string) error {
	names, err := h.Backend.ListAddons(clusterName)
	if err != nil {
		return h.handleError(c, err)
	}

	maxResults, nextToken := eksPaginationParams(c)
	p := page.New(names, nextToken, maxResults, eksDefaultPageSize)

	return c.JSON(http.StatusOK, eksPageResponse("addons", p))
}

// addonPodIdentityAssociationBody mirrors types.AddonPodIdentityAssociations
// (roleArn + serviceAccount only, no namespace).
type addonPodIdentityAssociationBody struct {
	RoleArn        string `json:"roleArn"`
	ServiceAccount string `json:"serviceAccount"`
}

type updateAddonBody struct {
	AddonVersion            string                            `json:"addonVersion"`
	ServiceAccountRoleArn   string                            `json:"serviceAccountRoleArn"`
	ConfigurationValues     string                            `json:"configurationValues"`
	ResolveConflicts        string                            `json:"resolveConflicts"`
	PodIdentityAssociations []addonPodIdentityAssociationBody `json:"podIdentityAssociations"`
}

func (h *Handler) handleUpdateAddon(c *echo.Context, clusterName, addonName string, body []byte) error {
	var in updateAddonBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", err.Error()))
		}
	}

	// in.PodIdentityAssociations is nil iff the JSON key was absent -- Go's
	// json.Unmarshal leaves a []T field untouched (nil) when its key is
	// missing, but sets it to a non-nil empty slice for `[]` (verified in
	// this session; same tri-state idiom as services/backup/handler_frameworks.go's
	// UpdateFramework/FrameworkControls).
	var podIdentityAssociations *[]PodIdentityAssociationSpec
	if in.PodIdentityAssociations != nil {
		specs := make([]PodIdentityAssociationSpec, len(in.PodIdentityAssociations))
		for i, a := range in.PodIdentityAssociations {
			specs[i] = PodIdentityAssociationSpec{RoleARN: a.RoleArn, ServiceAccount: a.ServiceAccount}
		}

		podIdentityAssociations = &specs
	}

	addon, err := h.Backend.UpdateAddon(
		clusterName, addonName, in.AddonVersion, in.ServiceAccountRoleArn,
		in.ConfigurationValues, in.ResolveConflicts, podIdentityAssociations,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyUpdate: map[string]any{
			"id":           uuid.NewString()[:8],
			keyStatusField: statusInProgress,
			keyType:        "AddonUpdate",
			keyClusterName: clusterName,
			"addonName":    addon.AddonName,
			keyCreatedAt:   float64(time.Now().Unix()),
		},
	})
}

func (h *Handler) handleDescribeAddonVersions(c *echo.Context) error {
	versions := h.Backend.DescribeAddonVersions()

	return c.JSON(http.StatusOK, map[string]any{
		"addons": versions,
	})
}

func (h *Handler) handleDescribeAddonConfiguration(c *echo.Context) error {
	addonName := c.Request().URL.Query().Get("addonName")
	addonVersion := c.Request().URL.Query().Get("addonVersion")

	if addonName == "" {
		addonName = addonVPCCNI
	}

	result := h.Backend.DescribeAddonConfiguration(addonName, addonVersion)

	return c.JSON(http.StatusOK, result)
}
