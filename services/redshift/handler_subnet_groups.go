package redshift

import (
	"encoding/xml"
	"net/url"
	"strconv"

	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// ---- Subnet group XML types ----

type xmlSubnet struct {
	SubnetIdentifier string `xml:"SubnetIdentifier"`
	SubnetStatus     string `xml:"SubnetStatus,omitempty"`
}

type xmlSubnetList struct {
	Members []xmlSubnet `xml:"Subnet"`
}

type xmlClusterSubnetGroup struct {
	ClusterSubnetGroupName string        `xml:"ClusterSubnetGroupName"`
	Description            string        `xml:"Description,omitempty"`
	VpcID                  string        `xml:"VpcId,omitempty"`
	SubnetGroupStatus      string        `xml:"SubnetGroupStatus,omitempty"`
	Subnets                xmlSubnetList `xml:"Subnets"`
	Tags                   []svcTags.KV  `xml:"Tags>Tag,omitempty"`
}

type xmlClusterSubnetGroupList struct {
	Members []xmlClusterSubnetGroup `xml:"ClusterSubnetGroup"`
}

func subnetGroupToXML(sg *ClusterSubnetGroup) xmlClusterSubnetGroup {
	subnets := make([]xmlSubnet, 0, len(sg.Subnets))
	for _, s := range sg.Subnets {
		subnets = append(subnets, xmlSubnet(s))
	}

	return xmlClusterSubnetGroup{
		ClusterSubnetGroupName: sg.ClusterSubnetGroupName,
		Description:            sg.Description,
		VpcID:                  sg.VpcID,
		SubnetGroupStatus:      sg.SubnetGroupStatus,
		Subnets:                xmlSubnetList{Members: subnets},
		Tags:                   tagMapToKVList(sg.Tags),
	}
}

// ---- CreateClusterSubnetGroup ----

type createClusterSubnetGroupResponse struct {
	XMLName     xml.Name              `xml:"CreateClusterSubnetGroupResponse"`
	Xmlns       string                `xml:"xmlns,attr"`
	SubnetGroup xmlClusterSubnetGroup `xml:"CreateClusterSubnetGroupResult>ClusterSubnetGroup"`
}

// handleCreateClusterSubnetGroup implements CreateClusterSubnetGroup. Real
// CreateClusterSubnetGroupInput has no VpcId field (confirmed against
// awsAwsquery_serializeOpDocumentCreateClusterSubnetGroupInput in
// aws-sdk-go-v2/service/redshift@v1.65.4/serializers.go) -- AWS derives the
// response's VpcId from the subnets' own VPC. This backend does not track
// subnet-to-VPC linkage (no EC2 cross-reference), so VpcId is left honestly
// empty here rather than accepting it as a fabricated request param; see
// PARITY.md for the same reasoning applied to EndpointAccess.
func (h *Handler) handleCreateClusterSubnetGroup(vals url.Values) (any, error) {
	name := vals.Get("ClusterSubnetGroupName")
	description := vals.Get("Description")
	subnetIDs := parseStringList(vals, "SubnetIds.SubnetIdentifier.")

	sg, err := h.Backend.CreateClusterSubnetGroup(name, description, "", subnetIDs, parseRedshiftTags(vals))
	if err != nil {
		return nil, err
	}

	return &createClusterSubnetGroupResponse{
		Xmlns:       redshiftXMLNS,
		SubnetGroup: subnetGroupToXML(sg),
	}, nil
}

// ---- DeleteClusterSubnetGroup ----

type deleteClusterSubnetGroupResponse struct {
	XMLName xml.Name `xml:"DeleteClusterSubnetGroupResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
}

func (h *Handler) handleDeleteClusterSubnetGroup(vals url.Values) (any, error) {
	name := vals.Get("ClusterSubnetGroupName")

	if err := h.Backend.DeleteClusterSubnetGroup(name); err != nil {
		return nil, err
	}

	return &deleteClusterSubnetGroupResponse{Xmlns: redshiftXMLNS}, nil
}

// ---- DescribeClusterSubnetGroups ----

type describeClusterSubnetGroupsResponse struct {
	XMLName      xml.Name                  `xml:"DescribeClusterSubnetGroupsResponse"`
	Xmlns        string                    `xml:"xmlns,attr"`
	Marker       string                    `xml:"DescribeClusterSubnetGroupsResult>Marker,omitempty"`
	SubnetGroups xmlClusterSubnetGroupList `xml:"DescribeClusterSubnetGroupsResult>ClusterSubnetGroups"`
}

func (h *Handler) handleDescribeClusterSubnetGroups(vals url.Values) (any, error) {
	name := vals.Get("ClusterSubnetGroupName")
	tagKeys := parseRedshiftTagKeysAt(vals, "TagKeys.TagKey.")
	tagValues := parseRedshiftTagKeysAt(vals, "TagValues.TagValue.")
	marker := vals.Get("Marker")

	maxRecords := 0
	if s := vals.Get("MaxRecords"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxRecords = n
		}
	}

	groups, nextMarker, err := h.Backend.DescribeClusterSubnetGroups(name, marker, maxRecords, tagKeys, tagValues)
	if err != nil {
		return nil, err
	}

	members := make([]xmlClusterSubnetGroup, 0, len(groups))
	for _, g := range groups {
		gp := g
		members = append(members, subnetGroupToXML(&gp))
	}

	return &describeClusterSubnetGroupsResponse{
		Xmlns:        redshiftXMLNS,
		SubnetGroups: xmlClusterSubnetGroupList{Members: members},
		Marker:       nextMarker,
	}, nil
}

// ---- ModifyClusterSubnetGroup ----

type modifyClusterSubnetGroupResponse struct {
	XMLName     xml.Name              `xml:"ModifyClusterSubnetGroupResponse"`
	Xmlns       string                `xml:"xmlns,attr"`
	SubnetGroup xmlClusterSubnetGroup `xml:"ModifyClusterSubnetGroupResult>ClusterSubnetGroup"`
}

func (h *Handler) handleModifyClusterSubnetGroup(vals url.Values) (any, error) {
	name := vals.Get("ClusterSubnetGroupName")
	description := vals.Get("Description")
	subnetIDs := parseStringList(vals, "SubnetIds.SubnetIdentifier.")

	sg, err := h.Backend.ModifyClusterSubnetGroup(name, description, subnetIDs)
	if err != nil {
		return nil, err
	}

	return &modifyClusterSubnetGroupResponse{
		Xmlns:       redshiftXMLNS,
		SubnetGroup: subnetGroupToXML(sg),
	}, nil
}
