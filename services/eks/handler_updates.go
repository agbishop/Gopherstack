package eks

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// dispatchUpdateOps handles cluster-level config/version update and
// encryption-config association operations.
func (h *Handler) dispatchUpdateOps(c *echo.Context, route eksRoute, body []byte) (bool, error) {
	switch route.operation {
	case opAssociateEncryptionConfig:
		return true, h.handleAssociateEncryptionConfig(c, route.clusterName, body)
	case opUpdateClusterConfig:
		return true, h.handleUpdateClusterConfig(c, route.clusterName, body)
	case opUpdateClusterVersion:
		return true, h.handleUpdateClusterVersion(c, route.clusterName, body)
	case opDescribeUpdate:
		return true, h.handleDescribeUpdate(c, route.clusterName, route.nodegroupName)
	case opListUpdates:
		return true, h.handleListUpdates(c, route.clusterName)
	case opCancelUpdate:
		return true, h.handleCancelUpdate(c, route.clusterName, route.nodegroupName, body)
	}

	return false, nil
}

// parseUpdatesRoute returns the route for
// /clusters/{name}/updates[/{id}[/cancel-update]].
func parseUpdatesRoute(method, clusterName string, parts []string) eksRoute {
	const updatesParts = 2

	// GET lists updates; POST on the same path starts a new
	// UpdateClusterVersion (there is no separate "/update-version" cluster
	// path in the real API) -- verified against the SDK serializer.
	if len(parts) == updatesParts {
		switch method {
		case http.MethodGet:
			return eksRoute{operation: opListUpdates, clusterName: clusterName}
		case http.MethodPost:
			return eksRoute{operation: opUpdateClusterVersion, clusterName: clusterName}
		}

		return eksRoute{operation: opUnknown}
	}

	tail := parts[2]

	// Real path is /clusters/{name}/updates/{updateId}/cancel-update (POST)
	// -- verified against the SDK serializer.
	if before, ok := strings.CutSuffix(tail, "/cancel-update"); ok {
		if method == http.MethodPost {
			return eksRoute{operation: opCancelUpdate, clusterName: clusterName, nodegroupName: before}
		}

		return eksRoute{operation: opUnknown}
	}

	updateID := tail

	if method == http.MethodGet {
		return eksRoute{operation: opDescribeUpdate, clusterName: clusterName, nodegroupName: updateID}
	}

	return eksRoute{operation: opUnknown}
}

type encryptionConfigItem struct {
	Provider  map[string]string `json:"provider"`
	Resources []string          `json:"resources"`
}

type associateEncryptionConfigBody struct {
	EncryptionConfig []encryptionConfigItem `json:"encryptionConfig"`
}

func (h *Handler) handleAssociateEncryptionConfig(c *echo.Context, clusterName string, body []byte) error {
	var in associateEncryptionConfigBody
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", "invalid request body"))
	}

	configs := make([]EncryptionConfig, len(in.EncryptionConfig))
	for i, ec := range in.EncryptionConfig {
		configs[i] = EncryptionConfig(ec)
	}

	result, err := h.Backend.AssociateEncryptionConfig(clusterName, configs)
	if err != nil {
		return h.handleError(c, err)
	}

	encryptionConfigJSON, err := json.Marshal(result)
	if err != nil {
		return h.handleError(c, err)
	}

	// Update.Params is an array of {type, value} pairs -- confirmed against
	// aws-sdk-go-v2/service/eks@v1.90.4's deserializers.go
	// (awsRestjson1_deserializeDocumentUpdate, case "params":
	// awsRestjson1_deserializeDocumentUpdateParams, an array) and the
	// UpdateParamTypeEncryptionConfig = "EncryptionConfig" enum value
	// (types/enums.go). A nested {"encryptionConfig": ...} object failed
	// decoding outright for every real client.
	return c.JSON(http.StatusOK, map[string]any{
		keyUpdate: map[string]any{
			"id":           uuid.NewString()[:8],
			keyStatusField: statusInProgress,
			keyType:        opAssociateEncryptionConfig,
			keyClusterName: clusterName,
			"params": []UpdateParam{
				{Type: "EncryptionConfig", Value: string(encryptionConfigJSON)},
			},
		},
	})
}

type updateClusterConfigLogging struct {
	ClusterLogging []struct {
		Types   []string `json:"types"`
		Enabled bool     `json:"enabled"`
	} `json:"clusterLogging"`
}

type updateClusterConfigVpcConfig struct {
	SubnetIDs             []string `json:"subnetIds"`
	EndpointPublicAccess  *bool    `json:"endpointPublicAccess"`
	EndpointPrivateAccess *bool    `json:"endpointPrivateAccess"`
	PublicAccessCidrs     []string `json:"publicAccessCidrs"`
}

type updateClusterConfigBody struct {
	Logging            *updateClusterConfigLogging   `json:"logging"`
	ResourcesVpcConfig *updateClusterConfigVpcConfig `json:"resourcesVpcConfig"`
	AccessConfig       *accessConfigJSON             `json:"accessConfig"`
	ComputeConfig      *computeConfigJSON            `json:"computeConfig"`
	StorageConfig      *storageConfigJSON            `json:"storageConfig"`
}

func (h *Handler) handleUpdateClusterConfig(c *echo.Context, clusterName string, body []byte) error {
	var in updateClusterConfigBody

	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", err.Error()))
		}
	}

	cfgUpd := buildClusterConfigUpdate(in)

	update, err := h.Backend.UpdateClusterConfig(clusterName, cfgUpd)
	if err != nil {
		return h.handleError(c, err)
	}

	if vpcErr := h.applyVpcEndpointUpdate(clusterName, in.ResourcesVpcConfig, update); vpcErr != nil {
		return h.handleError(c, vpcErr)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyUpdate: updateToJSON(update),
	})
}

func buildClusterConfigUpdate(in updateClusterConfigBody) ClusterConfigUpdate {
	cfgUpd := ClusterConfigUpdate{}

	if in.Logging != nil {
		cfgUpd.LogEntries = make([]ClusterLogEntry, len(in.Logging.ClusterLogging))
		for i, entry := range in.Logging.ClusterLogging {
			cfgUpd.LogEntries[i] = ClusterLogEntry{Types: entry.Types, Enabled: entry.Enabled}
		}
	}

	if in.ResourcesVpcConfig != nil && len(in.ResourcesVpcConfig.SubnetIDs) > 0 {
		cfgUpd.SubnetIDs = in.ResourcesVpcConfig.SubnetIDs
	}

	// types.UpdateAccessConfigRequest carries only AuthenticationMode --
	// BootstrapClusterCreatorAdminPermissions is create-only and not part of
	// this op's wire shape, so it is deliberately not read here.
	if in.AccessConfig != nil {
		cfgUpd.AccessConfig = &AccessConfig{AuthenticationMode: in.AccessConfig.AuthenticationMode}
	}

	if in.ComputeConfig != nil {
		cc := &ComputeConfig{NodeRoleARN: in.ComputeConfig.NodeRoleArn, NodePools: in.ComputeConfig.NodePools}
		if in.ComputeConfig.Enabled != nil {
			cc.Enabled = *in.ComputeConfig.Enabled
		}
		cfgUpd.ComputeConfig = cc
	}

	if in.StorageConfig != nil && in.StorageConfig.BlockStorage != nil {
		sc := &StorageConfig{BlockStorage: &BlockStorageConfig{}}
		if in.StorageConfig.BlockStorage.Enabled != nil {
			sc.BlockStorage.Enabled = *in.StorageConfig.BlockStorage.Enabled
		}
		cfgUpd.StorageConfig = sc
	}

	return cfgUpd
}

// applyVpcEndpointUpdate applies the VPC endpoint sub-update, returning the
// raw backend error unwritten so handleUpdateClusterConfig can map and write
// it exactly once. It used to call h.handleError itself and return that
// (always-nil, per c.JSON on success) result directly; handleUpdateClusterConfig
// tested that nil and fell through to write a second 200 on top of the
// already-committed error body (gopherstack-7opw, the gopherstack-8haq shape).
func (h *Handler) applyVpcEndpointUpdate(
	clusterName string,
	vpcIn *updateClusterConfigVpcConfig, update *Update,
) error {
	if vpcIn == nil {
		return nil
	}
	vpcUpd := VpcEndpointUpdate{
		EndpointPublicAccess:  vpcIn.EndpointPublicAccess,
		EndpointPrivateAccess: vpcIn.EndpointPrivateAccess,
		PublicAccessCIDRs:     vpcIn.PublicAccessCidrs,
	}
	if vpcUpd.EndpointPublicAccess == nil && vpcUpd.EndpointPrivateAccess == nil && vpcUpd.PublicAccessCIDRs == nil {
		return nil
	}
	vpcUpdate, err := h.Backend.UpdateClusterVpcEndpoint(clusterName, vpcUpd)
	if err != nil {
		return err
	}
	update.Params = append(update.Params, vpcUpdate.Params...)

	return nil
}

type updateClusterVersionBody struct {
	Version string `json:"version"`
}

func (h *Handler) handleUpdateClusterVersion(c *echo.Context, clusterName string, body []byte) error {
	var in updateClusterVersionBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", err.Error()))
		}
	}

	update, err := h.Backend.UpdateClusterVersion(clusterName, in.Version)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyUpdate: updateToJSON(update),
	})
}

func (h *Handler) handleDescribeUpdate(c *echo.Context, clusterName, updateID string) error {
	update, err := h.Backend.DescribeUpdate(clusterName, updateID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyUpdate: updateToJSON(update),
	})
}

func (h *Handler) handleListUpdates(c *echo.Context, clusterName string) error {
	ids, err := h.Backend.ListUpdates(clusterName)
	if err != nil {
		return h.handleError(c, err)
	}

	if nodegroupName := c.Request().URL.Query().Get("nodegroupName"); nodegroupName != "" {
		ids = slices.DeleteFunc(ids, func(id string) bool {
			u, descErr := h.Backend.DescribeUpdate(clusterName, id)

			return descErr != nil || u.NodegroupName != nodegroupName
		})
	}

	maxResults, nextToken := eksPaginationParams(c)
	p := page.New(ids, nextToken, maxResults, eksDefaultPageSize)

	return c.JSON(http.StatusOK, eksPageResponse("updateIds", p))
}

func updateToJSON(u *Update) map[string]any {
	m := map[string]any{
		"id":           u.ID,
		keyStatusField: u.Status,
		keyType:        u.Type,
		keyCreatedAt:   float64(u.CreatedAt.Unix()),
		"params":       u.Params,
		"errors":       u.Errors,
	}
	if u.Params == nil {
		m["params"] = []UpdateParam{}
	}
	if u.Errors == nil {
		m["errors"] = []UpdateError{}
	}
	if u.Cancellation != nil {
		m["cancellation"] = u.Cancellation
	}

	return m
}

type cancelUpdateBody struct {
	ClientRequestToken string `json:"clientRequestToken"`
}

// handleCancelUpdate implements CancelUpdate. clientRequestToken is accepted
// for wire-shape parity but not tracked for idempotency (matching the
// in-memory, non-durable nature of this backend).
func (h *Handler) handleCancelUpdate(c *echo.Context, clusterName, updateID string, body []byte) error {
	var in cancelUpdateBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("InvalidParameterException", err.Error()))
		}
	}

	update, err := h.Backend.CancelUpdate(clusterName, updateID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyUpdate: updateToJSON(update),
	})
}
