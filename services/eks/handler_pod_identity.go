package eks

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// dispatchPodIdentityOps handles pod identity association CRUD operations.
func (h *Handler) dispatchPodIdentityOps(c *echo.Context, route eksRoute, body []byte) (bool, error) {
	switch route.operation {
	case opCreatePodIdentityAssociation:
		return true, h.handleCreatePodIdentityAssociation(c, route.clusterName, body)
	case opDeletePodIdentityAssociation:
		return true, h.handleDeletePodIdentityAssociation(c, route.clusterName, route.nodegroupName)
	case opDescribePodIdentityAssociation:
		return true, h.handleDescribePodIdentityAssociation(c, route.clusterName, route.nodegroupName)
	case opListPodIdentityAssociations:
		return true, h.handleListPodIdentityAssociations(c, route.clusterName)
	case opUpdatePodIdentityAssociation:
		return true, h.handleUpdatePodIdentityAssociation(c, route.clusterName, route.nodegroupName, body)
	}

	return false, nil
}

// parsePodIdentityRoute returns the route for /clusters/{name}/pod-identity-associations[/{id}] paths.
func parsePodIdentityRoute(method, clusterName string, parts []string) eksRoute {
	const podIdentityParts = 2

	if len(parts) == podIdentityParts {
		switch method {
		case http.MethodPost:
			return eksRoute{operation: opCreatePodIdentityAssociation, clusterName: clusterName}
		case http.MethodGet:
			return eksRoute{operation: opListPodIdentityAssociations, clusterName: clusterName}
		}

		return eksRoute{operation: opUnknown}
	}

	assocID := parts[2]

	// UpdatePodIdentityAssociation is POST to this same path (not PUT) --
	// verified against the SDK serializer.
	switch method {
	case http.MethodGet:
		return eksRoute{operation: opDescribePodIdentityAssociation, clusterName: clusterName, nodegroupName: assocID}
	case http.MethodDelete:
		return eksRoute{operation: opDeletePodIdentityAssociation, clusterName: clusterName, nodegroupName: assocID}
	case http.MethodPost:
		return eksRoute{operation: opUpdatePodIdentityAssociation, clusterName: clusterName, nodegroupName: assocID}
	}

	return eksRoute{operation: opUnknown}
}

func podIdentityToJSON(a *PodIdentityAssociation) map[string]any {
	modifiedAt := a.ModifiedAt
	if modifiedAt.IsZero() {
		modifiedAt = a.CreatedAt
	}

	m := map[string]any{
		keyClusterName:       a.ClusterName,
		"associationId":      a.AssociationID,
		"associationArn":     a.ARN,
		keyNamespace:         a.Namespace,
		"serviceAccount":     a.ServiceAccount,
		"roleArn":            a.RoleARN,
		keyCreatedAt:         a.CreatedAt.Unix(),
		keyModifiedAt:        modifiedAt.Unix(),
		"disableSessionTags": a.DisableSessionTags,
	}

	if a.OwnerARN != "" {
		m["ownerArn"] = a.OwnerARN
	}

	if a.ExternalID != "" {
		m["externalId"] = a.ExternalID
	}

	if a.Policy != "" {
		m["policy"] = a.Policy
	}

	if a.Tags != nil {
		m["tags"] = a.Tags.Clone()
	} else {
		m["tags"] = map[string]string{}
	}

	return m
}

// podIdentitySummaryToJSON converts a PodIdentityAssociation into the
// PodIdentityAssociationSummary shape used by ListPodIdentityAssociations --
// verified against aws-sdk-go-v2/service/eks/types.PodIdentityAssociationSummary,
// which deliberately omits roleArn/createdAt/modifiedAt/tags/externalId/policy
// present on the full PodIdentityAssociation.
func podIdentitySummaryToJSON(a *PodIdentityAssociation) map[string]any {
	m := map[string]any{
		keyClusterName:   a.ClusterName,
		"associationId":  a.AssociationID,
		"associationArn": a.ARN,
		keyNamespace:     a.Namespace,
		"serviceAccount": a.ServiceAccount,
	}

	if a.OwnerARN != "" {
		m["ownerArn"] = a.OwnerARN
	}

	return m
}

type createPodIdentityAssociationBody struct {
	Tags               map[string]string `json:"tags"`
	Namespace          string            `json:"namespace"`
	ServiceAccount     string            `json:"serviceAccount"`
	RoleArn            string            `json:"roleArn"`
	Policy             string            `json:"policy"`
	DisableSessionTags bool              `json:"disableSessionTags"`
}

func (h *Handler) handleCreatePodIdentityAssociation(c *echo.Context, clusterName string, body []byte) error {
	var in createPodIdentityAssociationBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", "invalid request body"))
	}

	if in.Namespace == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", "namespace is required"))
	}

	if in.ServiceAccount == "" {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", "serviceAccount is required"))
	}

	assoc, err := h.Backend.CreatePodIdentityAssociation(
		clusterName,
		in.Namespace,
		in.ServiceAccount,
		in.RoleArn,
		in.Tags,
		PodIdentityAssociationInput{Policy: in.Policy, DisableSessionTags: in.DisableSessionTags},
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyAssociation: podIdentityToJSON(assoc),
	})
}

func (h *Handler) handleDeletePodIdentityAssociation(c *echo.Context, clusterName, assocID string) error {
	assoc, err := h.Backend.DeletePodIdentityAssociation(clusterName, assocID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyAssociation: podIdentityToJSON(assoc),
	})
}

func (h *Handler) handleDescribePodIdentityAssociation(c *echo.Context, clusterName, assocID string) error {
	assoc, err := h.Backend.DescribePodIdentityAssociation(clusterName, assocID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyAssociation: podIdentityToJSON(assoc),
	})
}

func (h *Handler) handleListPodIdentityAssociations(c *echo.Context, clusterName string) error {
	assocs, err := h.Backend.ListPodIdentityAssociations(clusterName)
	if err != nil {
		return h.handleError(c, err)
	}

	q := c.Request().URL.Query()
	if namespace := q.Get("namespace"); namespace != "" {
		assocs = slices.DeleteFunc(assocs, func(a *PodIdentityAssociation) bool {
			return a.Namespace != namespace
		})
	}

	if sa := q.Get("serviceAccount"); sa != "" {
		assocs = slices.DeleteFunc(assocs, func(a *PodIdentityAssociation) bool {
			return a.ServiceAccount != sa
		})
	}

	result := make([]map[string]any, len(assocs))
	for i, a := range assocs {
		result[i] = podIdentitySummaryToJSON(a)
	}

	maxResults, nextToken := eksPaginationParams(c)
	p := page.New(result, nextToken, maxResults, eksDefaultPageSize)

	return c.JSON(http.StatusOK, eksPageResponse("associations", p))
}

type updatePodIdentityBody struct {
	Policy             *string `json:"policy"`
	DisableSessionTags *bool   `json:"disableSessionTags"`
	RoleArn            string  `json:"roleArn"`
}

func (h *Handler) handleUpdatePodIdentityAssociation(c *echo.Context, clusterName, assocID string, body []byte) error {
	var in updatePodIdentityBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", err.Error()))
		}
	}

	assoc, err := h.Backend.UpdatePodIdentityAssociation(clusterName, assocID, PodIdentityAssociationUpdate{
		RoleARN:            in.RoleArn,
		Policy:             in.Policy,
		DisableSessionTags: in.DisableSessionTags,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyAssociation: podIdentityToJSON(assoc),
	})
}
