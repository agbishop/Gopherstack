package redshift

import (
	"encoding/xml"
	"net/url"
	"strconv"

	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// ---- CreateClusterSecurityGroup ----

type createClusterSecurityGroupResponse struct {
	XMLName       xml.Name                `xml:"CreateClusterSecurityGroupResponse"`
	Xmlns         string                  `xml:"xmlns,attr"`
	SecurityGroup xmlClusterSecurityGroup `xml:"CreateClusterSecurityGroupResult>ClusterSecurityGroup"`
}

func (h *Handler) handleCreateClusterSecurityGroup(vals url.Values) (any, error) {
	name := vals.Get("ClusterSecurityGroupName")
	description := vals.Get("Description")

	sg, err := h.Backend.CreateClusterSecurityGroup(name, description, parseRedshiftTags(vals))
	if err != nil {
		return nil, err
	}

	return &createClusterSecurityGroupResponse{
		Xmlns:         redshiftXMLNS,
		SecurityGroup: securityGroupToXML(sg),
	}, nil
}

// ---- DeleteClusterSecurityGroup ----

type deleteClusterSecurityGroupResponse struct {
	XMLName xml.Name `xml:"DeleteClusterSecurityGroupResponse"`
	Xmlns   string   `xml:"xmlns,attr"`
}

func (h *Handler) handleDeleteClusterSecurityGroup(vals url.Values) (any, error) {
	name := vals.Get("ClusterSecurityGroupName")

	if err := h.Backend.DeleteClusterSecurityGroup(name); err != nil {
		return nil, err
	}

	return &deleteClusterSecurityGroupResponse{Xmlns: redshiftXMLNS}, nil
}

// ---- DescribeClusterSecurityGroups ----

type xmlClusterSecurityGroupList struct {
	Members []xmlClusterSecurityGroup `xml:"ClusterSecurityGroup"`
}

type describeClusterSecurityGroupsResponse struct {
	XMLName        xml.Name                    `xml:"DescribeClusterSecurityGroupsResponse"`
	Xmlns          string                      `xml:"xmlns,attr"`
	Marker         string                      `xml:"DescribeClusterSecurityGroupsResult>Marker,omitempty"`
	SecurityGroups xmlClusterSecurityGroupList `xml:"DescribeClusterSecurityGroupsResult>ClusterSecurityGroups"`
}

func (h *Handler) handleDescribeClusterSecurityGroups(vals url.Values) (any, error) {
	name := vals.Get("ClusterSecurityGroupName")
	tagKeys := parseRedshiftTagKeysAt(vals, "TagKeys.TagKey.")
	tagValues := parseRedshiftTagKeysAt(vals, "TagValues.TagValue.")
	marker := vals.Get("Marker")

	maxRecords := 0
	if s := vals.Get("MaxRecords"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxRecords = n
		}
	}

	groups, nextMarker, err := h.Backend.DescribeClusterSecurityGroups(name, marker, maxRecords, tagKeys, tagValues)
	if err != nil {
		return nil, err
	}

	members := make([]xmlClusterSecurityGroup, 0, len(groups))
	for _, g := range groups {
		gp := g
		members = append(members, securityGroupToXML(&gp))
	}

	return &describeClusterSecurityGroupsResponse{
		Xmlns:          redshiftXMLNS,
		SecurityGroups: xmlClusterSecurityGroupList{Members: members},
		Marker:         nextMarker,
	}, nil
}

// ---- RevokeClusterSecurityGroupIngress ----

type revokeClusterSecurityGroupIngressResponse struct {
	XMLName       xml.Name                `xml:"RevokeClusterSecurityGroupIngressResponse"`
	Xmlns         string                  `xml:"xmlns,attr"`
	SecurityGroup xmlClusterSecurityGroup `xml:"RevokeClusterSecurityGroupIngressResult>ClusterSecurityGroup"`
}

func (h *Handler) handleRevokeClusterSecurityGroupIngress(vals url.Values) (any, error) {
	groupName := vals.Get("ClusterSecurityGroupName")
	cidrIP := vals.Get("CIDRIP")
	ec2GroupName := vals.Get("EC2SecurityGroupName")
	ec2GroupOwnerID := vals.Get("EC2SecurityGroupOwnerId")

	sg, err := h.Backend.RevokeClusterSecurityGroupIngress(groupName, cidrIP, ec2GroupName, ec2GroupOwnerID)
	if err != nil {
		return nil, err
	}

	return &revokeClusterSecurityGroupIngressResponse{
		Xmlns:         redshiftXMLNS,
		SecurityGroup: securityGroupToXML(sg),
	}, nil
}

// ---- AuthorizeClusterSecurityGroupIngress ----

type xmlIPRange struct {
	CIDRIP string `xml:"CIDRIP"`
	Status string `xml:"Status"`
}

type xmlEC2SecurityGroup struct {
	EC2SecurityGroupName    string `xml:"EC2SecurityGroupName"`
	EC2SecurityGroupOwnerID string `xml:"EC2SecurityGroupOwnerId"`
	Status                  string `xml:"Status"`
}

type xmlClusterSecurityGroup struct {
	ClusterSecurityGroupName string                `xml:"ClusterSecurityGroupName"`
	Description              string                `xml:"Description,omitempty"`
	IPRanges                 []xmlIPRange          `xml:"IPRanges>IPRange,omitempty"`
	EC2SecurityGroups        []xmlEC2SecurityGroup `xml:"EC2SecurityGroups>EC2SecurityGroup,omitempty"`
	Tags                     []svcTags.KV          `xml:"Tags>Tag,omitempty"`
}

type authorizeClusterSecurityGroupIngressResponse struct {
	XMLName xml.Name                `xml:"AuthorizeClusterSecurityGroupIngressResponse"`
	Xmlns   string                  `xml:"xmlns,attr"`
	Result  xmlClusterSecurityGroup `xml:"AuthorizeClusterSecurityGroupIngressResult>ClusterSecurityGroup"`
}

func securityGroupToXML(sg *ClusterSecurityGroup) xmlClusterSecurityGroup {
	ipRanges := make([]xmlIPRange, 0, len(sg.IPRanges))
	for _, r := range sg.IPRanges {
		ipRanges = append(ipRanges, xmlIPRange(r))
	}

	ec2Groups := make([]xmlEC2SecurityGroup, 0, len(sg.EC2SecurityGroups))
	for _, g := range sg.EC2SecurityGroups {
		ec2Groups = append(ec2Groups, xmlEC2SecurityGroup(g))
	}

	return xmlClusterSecurityGroup{
		ClusterSecurityGroupName: sg.ClusterSecurityGroupName,
		Description:              sg.Description,
		IPRanges:                 ipRanges,
		EC2SecurityGroups:        ec2Groups,
		Tags:                     tagMapToKVList(sg.Tags),
	}
}

func (h *Handler) handleAuthorizeClusterSecurityGroupIngress(vals url.Values) (any, error) {
	groupName := vals.Get("ClusterSecurityGroupName")
	cidrIP := vals.Get("CIDRIP")
	ec2GroupName := vals.Get("EC2SecurityGroupName")
	ec2GroupOwnerID := vals.Get("EC2SecurityGroupOwnerId")

	sg, err := h.Backend.AuthorizeClusterSecurityGroupIngress(groupName, cidrIP, ec2GroupName, ec2GroupOwnerID)
	if err != nil {
		return nil, err
	}

	return &authorizeClusterSecurityGroupIngressResponse{
		Xmlns:  redshiftXMLNS,
		Result: securityGroupToXML(sg),
	}, nil
}
