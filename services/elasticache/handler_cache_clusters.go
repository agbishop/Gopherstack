package elasticache

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
)

// cacheEndpoint is the XML representation of a cache node endpoint.
type cacheEndpoint struct {
	Address string `xml:"Address"`
	Port    int    `xml:"Port"`
}

// cacheNode is the XML representation of a cache node.
type cacheNode struct {
	CacheNodeID              string        `xml:"CacheNodeId"`
	CacheNodeStatus          string        `xml:"CacheNodeStatus"`
	CacheNodeCreateTime      string        `xml:"CacheNodeCreateTime"`
	CustomerAvailabilityZone string        `xml:"CustomerAvailabilityZone"`
	Endpoint                 cacheEndpoint `xml:"Endpoint"`
}

// cacheNodes is the XML container for cache nodes.
type cacheNodes struct {
	CacheNode []cacheNode `xml:"CacheNode"`
}

// cacheClusterXML is the XML representation of a cache cluster.
type cacheClusterXML struct {
	CacheParameterGroupName    string     `xml:"CacheParameterGroup>CacheParameterGroupName,omitempty"`
	PreferredMaintenanceWindow string     `xml:"PreferredMaintenanceWindow,omitempty"`
	CacheNodeType              string     `xml:"CacheNodeType"`
	Engine                     string     `xml:"Engine"`
	EngineVersion              string     `xml:"EngineVersion"`
	ARN                        string     `xml:"ARN"`
	CacheClusterStatus         string     `xml:"CacheClusterStatus"`
	CreatedAt                  string     `xml:"CacheClusterCreateTime,omitempty"`
	CacheClusterID             string     `xml:"CacheClusterId"`
	ReplicationGroupID         string     `xml:"ReplicationGroupId,omitempty"`
	SnapshotWindow             string     `xml:"SnapshotWindow,omitempty"`
	CacheNodes                 cacheNodes `xml:"CacheNodes"`
	NumCacheNodes              int        `xml:"NumCacheNodes"`
	SnapshotRetentionLimit     int        `xml:"SnapshotRetentionLimit,omitempty"`
	TransitEncryptionEnabled   bool       `xml:"TransitEncryptionEnabled"`
	AtRestEncryptionEnabled    bool       `xml:"AtRestEncryptionEnabled"`
}

func (h *Handler) createCacheCluster(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("CacheClusterId")
	if id == "" {
		return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "CacheClusterId is required")
	}

	engine := form.Get("Engine")
	nodeType := form.Get("CacheNodeType")
	paramGroupName := form.Get("CacheParameterGroupName")
	maintenanceWindow := form.Get("PreferredMaintenanceWindow")
	snapshotWindow := form.Get("SnapshotWindow")
	numCacheNodes := 1

	if s := form.Get("NumCacheNodes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			numCacheNodes = n
		}
	}

	var restoreErr error

	engine, nodeType, restoreErr = h.applySnapshotDefaults(ctx, c, form.Get("SnapshotName"), engine, nodeType)
	if restoreErr != nil {
		return restoreErr
	}

	cluster, err := h.Backend.CreateClusterWithOptions(ctx,
		id,
		engine,
		nodeType,
		paramGroupName,
		maintenanceWindow,
		snapshotWindow,
		numCacheNodes,
		0,
	)
	if err != nil {
		if errors.Is(err, ErrClusterAlreadyExists) {
			return xmlError(c, http.StatusBadRequest, "CacheClusterAlreadyExists", "Cache cluster already exists")
		}
		if errors.Is(err, ErrParameterGroupNotFound) {
			return xmlError(c, http.StatusNotFound, "CacheParameterGroupNotFound", "Cache parameter group not found")
		}
		if errors.Is(err, ErrInvalidParameterGroupFamily) {
			return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	h.applyCreateTimeTags(ctx, form, cluster.ARN)

	if sgErr := h.applyClusterSubnetGroup(ctx, c, form, id, cluster); sgErr != nil {
		return sgErr
	}

	if srErr := h.applyClusterSnapshotRetentionLimit(ctx, c, form, id, cluster); srErr != nil {
		return srErr
	}

	type result struct {
		XMLName      xml.Name        `xml:"CreateCacheClusterResponse"`
		Xmlns        string          `xml:"xmlns,attr"`
		CacheCluster cacheClusterXML `xml:"CreateCacheClusterResult>CacheCluster"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:        elasticacheNS,
		CacheCluster: clusterToXML(cluster, cluster.Status),
	})
}

// applyClusterSubnetGroup records CacheSubnetGroupName on a just-created cluster,
// if the caller supplied one. Split out of createCacheCluster to keep its
// cognitive complexity down.
func (h *Handler) applyClusterSubnetGroup(
	ctx context.Context, c *echo.Context, form url.Values, id string, cluster *Cluster,
) error {
	subnetGroupName := form.Get("CacheSubnetGroupName")
	if subnetGroupName == "" {
		return nil
	}

	if err := h.Backend.SetClusterSubnetGroupName(ctx, id, subnetGroupName); err != nil {
		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	cluster.SubnetGroupName = subnetGroupName

	return nil
}

// applyClusterSnapshotRetentionLimit records SnapshotRetentionLimit on a
// just-created cluster, if the caller supplied it. AWS documents 0 as a
// meaningful explicit value ("automatic backups are disabled"), so presence
// is checked on the raw form value, not on the parsed int being non-zero.
// Split out of createCacheCluster to keep its cognitive complexity down.
func (h *Handler) applyClusterSnapshotRetentionLimit(
	ctx context.Context, c *echo.Context, form url.Values, id string, cluster *Cluster,
) error {
	s := form.Get("SnapshotRetentionLimit")
	if s == "" {
		return nil
	}

	n, parseErr := strconv.Atoi(s)
	if parseErr != nil {
		return xmlError(c, http.StatusBadRequest, "InvalidParameterValue", "SnapshotRetentionLimit must be an integer")
	}

	if err := h.Backend.SetClusterSnapshotRetentionLimit(ctx, id, &n); err != nil {
		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	cluster.SnapshotRetentionLimit = n

	return nil
}

// applySnapshotDefaults implements AWS's CreateCacheCluster-from-snapshot restore:
// when snapshotName is set, the snapshot must exist and its engine/node type
// become the defaults for whichever of engine/nodeType the caller left blank.
// Split out of createCacheCluster to keep its cognitive complexity down.
func (h *Handler) applySnapshotDefaults(
	ctx context.Context, c *echo.Context, snapshotName, engine, nodeType string,
) (string, string, error) {
	if snapshotName == "" {
		return engine, nodeType, nil
	}

	// SnapshotNotFoundFault isn't in CreateCacheCluster's modeled error list
	// (api-2.json), so aws-sdk-go-v2 has no case for it in this operation's
	// error deserializer and would fall back to a generic error; AWS instead
	// surfaces a missing/invalid snapshot here as InvalidParameterValue, which
	// the SDK does model for this op.
	snaps, snapErr := h.Backend.DescribeSnapshots(ctx, snapshotName, "", "", "", "", 0)
	if snapErr != nil || len(snaps.Data) == 0 {
		return "", "", xmlError(c, http.StatusBadRequest, "InvalidParameterValue",
			fmt.Sprintf("Cache cluster snapshot not found: %s", snapshotName))
	}

	src := snaps.Data[0]
	if engine == "" {
		engine = src.Engine
	}

	if nodeType == "" {
		nodeType = src.NodeType
	}

	return engine, nodeType, nil
}

func (h *Handler) deleteCacheCluster(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("CacheClusterId")
	clusters, descErr := h.Backend.DescribeClusters(ctx, id, "", 0, false)
	if descErr != nil {
		if errors.Is(descErr, ErrClusterNotFound) {
			return xmlError(c, http.StatusNotFound, "CacheClusterNotFound", "Cache cluster not found")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", descErr.Error())
	}
	if len(clusters.Data) == 0 {
		return xmlError(c, http.StatusNotFound, "CacheClusterNotFound", "Cache cluster not found")
	}

	cl := clusters.Data[0]

	// api-2.json SnapshotFeatureNotSupportedFault: "Creating a snapshot of a
	// cluster that is running Memcached rather than Valkey or Redis OSS" is
	// unsupported, and DeleteCacheCluster takes the final snapshot as part
	// of the delete.
	if form.Get("FinalSnapshotIdentifier") != "" && cl.Engine == engineMemcached {
		return xmlError(c, http.StatusBadRequest, "SnapshotFeatureNotSupportedFault",
			"Snapshots not supported for Memcached engine")
	}

	if err := h.Backend.DeleteCluster(ctx, id); err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return xmlError(c, http.StatusNotFound, "CacheClusterNotFound", "Cache cluster not found")
		}
		if errors.Is(err, ErrClusterNotAvailable) || errors.Is(err, ErrClusterInReplicationGroup) {
			return xmlError(c, http.StatusBadRequest, "InvalidCacheClusterState", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName      xml.Name        `xml:"DeleteCacheClusterResponse"`
		Xmlns        string          `xml:"xmlns,attr"`
		CacheCluster cacheClusterXML `xml:"DeleteCacheClusterResult>CacheCluster"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:        elasticacheNS,
		CacheCluster: clusterToXML(&cl, "deleting"),
	})
}

func (h *Handler) describeCacheClusters(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("CacheClusterId")
	marker, maxRecords, err := parsePaginationChecked(c, form)
	if err != nil {
		return err
	}
	notInRG := strings.EqualFold(form.Get("ShowCacheClustersNotInReplicationGroups"), "true")

	p, err := h.Backend.DescribeClusters(ctx, id, marker, maxRecords, notInRG)
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return xmlError(c, http.StatusNotFound, "CacheClusterNotFound", "Cache cluster not found")
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type cacheClusters struct {
		CacheCluster []cacheClusterXML `xml:"CacheCluster"`
	}
	type result struct {
		XMLName       xml.Name      `xml:"DescribeCacheClustersResponse"`
		Xmlns         string        `xml:"xmlns,attr"`
		Marker        string        `xml:"DescribeCacheClustersResult>Marker,omitempty"`
		CacheClusters cacheClusters `xml:"DescribeCacheClustersResult>CacheClusters"`
	}

	items := make([]cacheClusterXML, 0, len(p.Data))
	for i := range p.Data {
		items = append(items, clusterToXML(&p.Data[i], p.Data[i].Status))
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:         elasticacheNS,
		Marker:        p.Next,
		CacheClusters: cacheClusters{CacheCluster: items},
	})
}

// clusterToXML converts a Cluster to its XML representation with the given status.
func clusterToXML(cl *Cluster, status string) cacheClusterXML {
	n := cl.NumCacheNodes
	if n <= 0 {
		n = 1
	}

	region := cl.Region
	if region == "" {
		region = config.DefaultRegion
	}

	nodes := make([]cacheNode, 0, n)
	for i := range n {
		nodeID := fmt.Sprintf("%04d", i+1)
		nodes = append(nodes, cacheNode{
			CacheNodeID:              nodeID,
			CacheNodeStatus:          status,
			CacheNodeCreateTime:      cl.CreatedAt.UTC().Format(time.RFC3339),
			CustomerAvailabilityZone: region + "a",
			Endpoint: cacheEndpoint{
				Address: cl.Endpoint,
				Port:    cl.Port,
			},
		})
	}

	return cacheClusterXML{
		CacheClusterID:             cl.ClusterID,
		CacheClusterStatus:         status,
		CacheNodeType:              cl.NodeType,
		Engine:                     cl.Engine,
		EngineVersion:              cl.EngineVersion,
		NumCacheNodes:              n,
		ARN:                        cl.ARN,
		CacheParameterGroupName:    cl.CacheParameterGroupName,
		ReplicationGroupID:         cl.ReplicationGroupID,
		PreferredMaintenanceWindow: cl.PreferredMaintenanceWindow,
		SnapshotWindow:             cl.SnapshotWindow,
		SnapshotRetentionLimit:     cl.SnapshotRetentionLimit,
		TransitEncryptionEnabled:   cl.TransitEncryptionEnabled,
		AtRestEncryptionEnabled:    cl.AtRestEncryptionEnabled,
		CreatedAt:                  cl.CreatedAt.UTC().Format(time.RFC3339),
		CacheNodes: cacheNodes{
			CacheNode: nodes,
		},
	}
}

func (h *Handler) modifyCacheCluster(ctx context.Context, c *echo.Context, form url.Values) error {
	id := form.Get("CacheClusterId")
	nodeType := form.Get("CacheNodeType")
	paramGroupName := form.Get("CacheParameterGroupName")
	engineVersion := form.Get("EngineVersion")
	maintenanceWindow := form.Get("PreferredMaintenanceWindow")
	snapshotWindow := form.Get("SnapshotWindow")
	numCacheNodes := 0

	if s := form.Get("NumCacheNodes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			numCacheNodes = n
		}
	}

	cluster, err := h.Backend.ModifyCluster(ctx,
		id,
		nodeType,
		paramGroupName,
		engineVersion,
		maintenanceWindow,
		snapshotWindow,
		numCacheNodes,
	)
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return xmlError(c, http.StatusNotFound, "CacheClusterNotFound", "Cache cluster not found")
		}
		if errors.Is(err, ErrParameterGroupNotFound) {
			return xmlError(c, http.StatusNotFound, "CacheParameterGroupNotFound", "Cache parameter group not found")
		}
		if errors.Is(err, ErrClusterNotAvailable) {
			return xmlError(c, http.StatusBadRequest, "InvalidCacheClusterState", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	if srErr := h.applyClusterSnapshotRetentionLimit(ctx, c, form, id, cluster); srErr != nil {
		return srErr
	}

	type result struct {
		XMLName      xml.Name        `xml:"ModifyCacheClusterResponse"`
		Xmlns        string          `xml:"xmlns,attr"`
		CacheCluster cacheClusterXML `xml:"ModifyCacheClusterResult>CacheCluster"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:        elasticacheNS,
		CacheCluster: clusterToXML(cluster, cluster.Status),
	})
}

func (h *Handler) rebootCacheCluster(ctx context.Context, c *echo.Context, form url.Values) error {
	clusterID := form.Get("CacheClusterId")
	nodeIDs := parseRepeatedField(form, "CacheNodeIdsToReboot.CacheNodeId")

	cl, err := h.Backend.RebootCacheCluster(ctx, clusterID, nodeIDs)
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			return xmlError(c, http.StatusNotFound, "CacheClusterNotFound", "Cache cluster not found")
		}
		if errors.Is(err, ErrClusterNotAvailable) {
			return xmlError(c, http.StatusBadRequest, "InvalidCacheClusterState", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName      xml.Name        `xml:"RebootCacheClusterResponse"`
		Xmlns        string          `xml:"xmlns,attr"`
		CacheCluster cacheClusterXML `xml:"RebootCacheClusterResult>CacheCluster"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:        elasticacheNS,
		CacheCluster: clusterToXML(cl, cl.Status),
	})
}

func (h *Handler) listAllowedNodeTypeModifications(ctx context.Context, c *echo.Context, form url.Values) error {
	clusterID := form.Get("CacheClusterId")
	replicationGroupID := form.Get("ReplicationGroupId")

	mods, err := h.Backend.ListAllowedNodeTypeModifications(ctx, clusterID, replicationGroupID)
	if err != nil {
		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type scaleModsXML struct {
		Member []string `xml:"member"`
	}

	type result struct {
		XMLName                xml.Name     `xml:"ListAllowedNodeTypeModificationsResponse"`
		Xmlns                  string       `xml:"xmlns,attr"`
		ScaleUpModifications   scaleModsXML `xml:"ListAllowedNodeTypeModificationsResult>ScaleUpModifications"`
		ScaleDownModifications scaleModsXML `xml:"ListAllowedNodeTypeModificationsResult>ScaleDownModifications"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:                elasticacheNS,
		ScaleUpModifications: scaleModsXML{Member: mods},
	})
}
