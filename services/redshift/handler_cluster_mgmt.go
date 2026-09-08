package redshift

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
)

// ---- ModifyCluster ----

type modifyClusterResponse struct {
	XMLName xml.Name   `xml:"ModifyClusterResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"ModifyClusterResult>Cluster"`
}

func (h *Handler) handleModifyCluster(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	numberOfNodesStr := vals.Get("NumberOfNodes")
	applyImmediatelyStr := vals.Get("ApplyImmediately")
	portStr := vals.Get("Port")

	// Encrypted/EnhancedVpcRouting/PubliclyAccessible are tri-state on the wire
	// (real ModifyClusterInput fields are *bool): only build a pointer when the
	// form actually included the key, so "not specified" is distinguishable
	// from "explicitly false" (e.g. decrypting a cluster).
	var encrypted *bool
	if v := vals.Get("Encrypted"); v != "" {
		b := v == paramValueTrue
		encrypted = &b
	}

	var enhancedVpcRouting *bool
	if v := vals.Get("EnhancedVpcRouting"); v != "" {
		b := v == paramValueTrue
		enhancedVpcRouting = &b
	}

	var publiclyAccessible *bool
	if v := vals.Get("PubliclyAccessible"); v != "" {
		b := v == paramValueTrue
		publiclyAccessible = &b
	}

	numberOfNodes := 0

	if numberOfNodesStr != "" {
		n, err := strconv.Atoi(numberOfNodesStr)
		if err != nil {
			return nil, fmt.Errorf("%w: NumberOfNodes must be an integer", ErrInvalidParameter)
		}

		numberOfNodes = n
	}

	port := 0

	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("%w: Port must be an integer", ErrInvalidParameter)
		}

		port = p
	}

	cluster, err := h.Backend.ModifyCluster(id, ModifyClusterOptions{
		NodeType:            vals.Get("NodeType"),
		MasterUserPassword:  vals.Get("MasterUserPassword"),
		ClusterVersion:      vals.Get("ClusterVersion"),
		VpcSecurityGroupIDs: parseStringList(vals, "VpcSecurityGroupIds.VpcSecurityGroupId."),
		NumberOfNodes:       numberOfNodes,
		Port:                port,
		Encrypted:           encrypted,
		EnhancedVpcRouting:  enhancedVpcRouting,
		PubliclyAccessible:  publiclyAccessible,
		ApplyImmediately:    applyImmediatelyStr != "false",
	})
	if err != nil {
		return nil, err
	}

	return &modifyClusterResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- RebootCluster ----

type rebootClusterResponse struct {
	XMLName xml.Name   `xml:"RebootClusterResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"RebootClusterResult>Cluster"`
}

func (h *Handler) handleRebootCluster(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")

	cluster, err := h.Backend.RebootCluster(id)
	if err != nil {
		return nil, err
	}

	return &rebootClusterResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- PauseCluster ----

type pauseClusterResponse struct {
	XMLName xml.Name   `xml:"PauseClusterResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"PauseClusterResult>Cluster"`
}

func (h *Handler) handlePauseCluster(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")

	cluster, err := h.Backend.PauseCluster(id)
	if err != nil {
		return nil, err
	}

	return &pauseClusterResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- ResumeCluster ----

type resumeClusterResponse struct {
	XMLName xml.Name   `xml:"ResumeClusterResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"ResumeClusterResult>Cluster"`
}

func (h *Handler) handleResumeCluster(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")

	cluster, err := h.Backend.ResumeCluster(id)
	if err != nil {
		return nil, err
	}

	return &resumeClusterResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- ResizeCluster ----

type resizeClusterResponse struct {
	XMLName xml.Name   `xml:"ResizeClusterResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"ResizeClusterResult>Cluster"`
}

func (h *Handler) handleResizeCluster(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	nodeType := vals.Get("NodeType")
	clusterType := vals.Get("ClusterType")
	classic := vals.Get("Classic") == paramValueTrue
	numberOfNodesStr := vals.Get("NumberOfNodes")

	numberOfNodes := 0

	if numberOfNodesStr != "" {
		n, err := strconv.Atoi(numberOfNodesStr)
		if err != nil {
			return nil, fmt.Errorf("%w: NumberOfNodes must be an integer", ErrInvalidParameter)
		}

		numberOfNodes = n
	}

	cluster, err := h.Backend.ResizeCluster(id, nodeType, clusterType, numberOfNodes, classic)
	if err != nil {
		return nil, err
	}

	return &resizeClusterResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- RotateEncryptionKey ----

type rotateEncryptionKeyResponse struct {
	XMLName xml.Name   `xml:"RotateEncryptionKeyResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"RotateEncryptionKeyResult>Cluster"`
}

func (h *Handler) handleRotateEncryptionKey(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")

	cluster, err := h.Backend.RotateEncryptionKey(id)
	if err != nil {
		return nil, err
	}

	return &rotateEncryptionKeyResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- ModifyClusterIamRoles ----

type modifyClusterIamRolesResponse struct {
	XMLName xml.Name   `xml:"ModifyClusterIamRolesResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"ModifyClusterIamRolesResult>Cluster"`
}

func (h *Handler) handleModifyClusterIamRoles(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	addRoles := parseStringList(vals, "AddIamRoles.IamRoleArn.")
	removeRoles := parseStringList(vals, "RemoveIamRoles.IamRoleArn.")

	cluster, err := h.Backend.ModifyClusterIamRoles(id, addRoles, removeRoles)
	if err != nil {
		return nil, err
	}

	return &modifyClusterIamRolesResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- ModifyClusterMaintenance ----

type modifyClusterMaintenanceResponse struct {
	XMLName xml.Name   `xml:"ModifyClusterMaintenanceResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"ModifyClusterMaintenanceResult>Cluster"`
}

func (h *Handler) handleModifyClusterMaintenance(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	maintenanceTrack := vals.Get("MaintenanceTrackName")
	deferMaintenance := vals.Get("DeferMaintenance") == paramValueTrue

	cluster, err := h.Backend.ModifyClusterMaintenance(id, maintenanceTrack, deferMaintenance)
	if err != nil {
		return nil, err
	}

	return &modifyClusterMaintenanceResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}

// ---- DescribeClusterDBRevisions ----

type clusterDBRevisionXML struct {
	ClusterIdentifier       string `xml:"ClusterIdentifier"`
	CurrentDatabaseRevision string `xml:"CurrentDatabaseRevision"`
}

type describeClusterDBRevisionsResponse struct {
	XMLName xml.Name `xml:"DescribeClusterDBRevisionsResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
	Result  struct {
		ClusterDBRevisions []clusterDBRevisionXML `xml:"ClusterDBRevisions>ClusterDbRevision"`
	} `xml:"DescribeClusterDBRevisionsResult"`
}

func (h *Handler) handleDescribeClusterDBRevisions(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	resp := &describeClusterDBRevisionsResponse{Xmlns: redshiftXMLNS}

	if id != "" {
		resp.Result.ClusterDBRevisions = []clusterDBRevisionXML{
			{ClusterIdentifier: id, CurrentDatabaseRevision: "1"},
		}
	}

	return resp, nil
}

type modifyClusterDBRevisionResponse struct {
	XMLName xml.Name   `xml:"ModifyClusterDbRevisionResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"ModifyClusterDbRevisionResult>Cluster"`
}

func (h *Handler) handleModifyClusterDBRevision(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")
	clusters, _, err := h.Backend.DescribeClusters(id, "", 0, nil, nil)
	if err != nil {
		return nil, err
	}

	if len(clusters) == 0 {
		return &modifyClusterDBRevisionResponse{Xmlns: redshiftXMLNS}, nil
	}

	return &modifyClusterDBRevisionResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(&clusters[0]),
	}, nil
}

// ---- ModifyAquaConfiguration ----

type aquaConfigurationResponse struct {
	XMLName xml.Name      `xml:"ModifyAquaConfigurationResponse"`
	Xmlns   string        `xml:"xmlns,attr"`
	Result  xmlAquaConfig `xml:"ModifyAquaConfigurationResult>AquaConfiguration"`
}

func (h *Handler) handleModifyAquaConfiguration(vals url.Values) (any, error) {
	id := vals.Get("ClusterIdentifier")

	if _, err := h.Backend.ModifyAquaConfiguration(id); err != nil {
		return nil, err
	}

	return &aquaConfigurationResponse{
		Xmlns:  redshiftXMLNS,
		Result: defaultAquaConfig(),
	}, nil
}

// ---- ModifyLakehouseConfiguration ----

type modifyLakehouseConfigurationResponse struct {
	XMLName xml.Name `xml:"ModifyLakehouseConfigurationResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
	Result  struct {
		ClusterIdentifier           string `xml:"ClusterIdentifier,omitempty"`
		CatalogArn                  string `xml:"CatalogArn,omitempty"`
		LakehouseIdcApplicationArn  string `xml:"LakehouseIdcApplicationArn,omitempty"`
		LakehouseRegistrationStatus string `xml:"LakehouseRegistrationStatus,omitempty"`
	} `xml:"ModifyLakehouseConfigurationResult"`
}

func (h *Handler) handleModifyLakehouseConfiguration(vals url.Values) (any, error) {
	params := ModifyLakehouseConfigParams{
		ClusterIdentifier:          vals.Get("ClusterIdentifier"),
		CatalogName:                vals.Get("CatalogName"),
		LakehouseIdcApplicationArn: vals.Get("LakehouseIdcApplicationArn"),
		LakehouseIdcRegistration:   vals.Get("LakehouseIdcRegistration"),
		LakehouseRegistration:      vals.Get("LakehouseRegistration"),
		DryRun:                     vals.Get("DryRun") == paramValueTrue,
	}

	result, err := h.Backend.ModifyLakehouseConfiguration(params)
	if err != nil {
		return nil, err
	}

	resp := &modifyLakehouseConfigurationResponse{Xmlns: redshiftXMLNS}
	resp.Result.ClusterIdentifier = result.ClusterIdentifier
	resp.Result.CatalogArn = result.CatalogArn
	resp.Result.LakehouseIdcApplicationArn = result.LakehouseIdcApplicationArn
	resp.Result.LakehouseRegistrationStatus = result.LakehouseRegistrationStatus

	return resp, nil
}

// ---- FailoverPrimaryCompute ----

type failoverPrimaryComputeResponse struct {
	XMLName xml.Name   `xml:"FailoverPrimaryComputeResponse"`
	Xmlns   string     `xml:"xmlns,attr"`
	Cluster xmlCluster `xml:"FailoverPrimaryComputeResult>Cluster"`
}

func (h *Handler) handleFailoverPrimaryCompute(vals url.Values) (any, error) {
	clusterID := vals.Get("ClusterIdentifier")

	cluster, err := h.Backend.FailoverPrimaryCompute(clusterID)
	if err != nil {
		return nil, err
	}

	return &failoverPrimaryComputeResponse{
		Xmlns:   redshiftXMLNS,
		Cluster: h.toXMLCluster(cluster),
	}, nil
}
