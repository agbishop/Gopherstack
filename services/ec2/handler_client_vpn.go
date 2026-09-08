package ec2

import (
	"encoding/xml"
	"net/url"
)

type createClientVpnEndpointResponse struct {
	XMLName             xml.Name                    `xml:"CreateClientVpnEndpointResponse"`
	RequestID           string                      `xml:"requestId"`
	ClientVpnEndpointID string                      `xml:"clientVpnEndpointId"`
	DNSName             string                      `xml:"dnsName"`
	Status              clientVpnEndpointStatusItem `xml:"status"`
}

// describeClientVpnEndpointsResponse wraps the endpoint list directly under
// <clientVpnEndpoint>, matching the real aws-sdk-go-v2 EndpointSet shape
// (there is no <clientVpnEndpointSet> wrapper in the real wire format).

// describeClientVpnEndpointsResponse wraps the endpoint list directly under
// <clientVpnEndpoint>, matching the real aws-sdk-go-v2 EndpointSet shape
// (there is no <clientVpnEndpointSet> wrapper in the real wire format).
type describeClientVpnEndpointsResponse struct {
	XMLName              xml.Name `xml:"DescribeClientVpnEndpointsResponse"`
	RequestID            string   `xml:"requestId"`
	NextToken            string   `xml:"nextToken,omitempty"`
	ClientVpnEndpointSet struct {
		Items []clientVpnEndpointItem `xml:"item"`
	} `xml:"clientVpnEndpoint"`
}

type clientVpnTargetNetworkItem struct {
	AssociationID       string                      `xml:"associationId"`
	TargetNetworkID     string                      `xml:"targetNetworkId"`
	ClientVpnEndpointID string                      `xml:"clientVpnEndpointId"`
	Status              clientVpnEndpointStatusItem `xml:"status"`
	VpcID               string                      `xml:"vpcId,omitempty"`
	SecurityGroups      stringItemSet               `xml:"securityGroups"`
}

type describeClientVpnTargetNetworksResponse struct {
	XMLName                 xml.Name `xml:"DescribeClientVpnTargetNetworksResponse"`
	RequestID               string   `xml:"requestId"`
	NextToken               string   `xml:"nextToken,omitempty"`
	ClientVpnTargetNetworks struct {
		Items []clientVpnTargetNetworkItem `xml:"item"`
	} `xml:"clientVpnTargetNetworks"`
}

type clientVpnRouteItem struct {
	DestinationCidr string                      `xml:"destinationCidr"`
	Status          clientVpnEndpointStatusItem `xml:"status"`
	Description     string                      `xml:"description,omitempty"`
	Origin          string                      `xml:"origin,omitempty"`
	TargetSubnet    string                      `xml:"targetSubnet,omitempty"`
}

// describeClientVpnRoutesResponse wraps routes under <routes>, matching the
// real aws-sdk-go-v2 wire format (this repo previously used the wrong
// <clientVpnRouteSet> wrapper name — see parity.md "EC2 sub-resource ops").

// describeClientVpnRoutesResponse wraps routes under <routes>, matching the
// real aws-sdk-go-v2 wire format (this repo previously used the wrong
// <clientVpnRouteSet> wrapper name — see parity.md "EC2 sub-resource ops").
type describeClientVpnRoutesResponse struct {
	XMLName   xml.Name `xml:"DescribeClientVpnRoutesResponse"`
	RequestID string   `xml:"requestId"`
	NextToken string   `xml:"nextToken,omitempty"`
	Routes    struct {
		Items []clientVpnRouteItem `xml:"item"`
	} `xml:"routes"`
}

type clientVpnAuthRuleItem struct {
	Cidr        string                      `xml:"destinationCidr"`
	Status      clientVpnEndpointStatusItem `xml:"status"`
	Description string                      `xml:"description,omitempty"`
	GroupID     string                      `xml:"groupId,omitempty"`
	AccessAll   bool                        `xml:"accessAll,omitempty"`
}

// describeClientVpnAuthorizationRulesResponse wraps rules under
// <authorizationRule> (singular), matching the real aws-sdk-go-v2
// AuthorizationRuleSet shape.

// describeClientVpnAuthorizationRulesResponse wraps rules under
// <authorizationRule> (singular), matching the real aws-sdk-go-v2
// AuthorizationRuleSet shape.
type describeClientVpnAuthorizationRulesResponse struct {
	XMLName            xml.Name `xml:"DescribeClientVpnAuthorizationRulesResponse"`
	RequestID          string   `xml:"requestId"`
	NextToken          string   `xml:"nextToken,omitempty"`
	AuthorizationRules struct {
		Items []clientVpnAuthRuleItem `xml:"item"`
	} `xml:"authorizationRule"`
}

// clientVpnConnectionItem mirrors AWS's ClientVpnConnection shape. No API in
// this backend creates connections, so DescribeClientVpnConnections always
// returns an empty <connections> set — the correct AWS shape for a Client
// VPN endpoint with no active clients.

// clientVpnConnectionItem mirrors AWS's ClientVpnConnection shape. No API in
// this backend creates connections, so DescribeClientVpnConnections always
// returns an empty <connections> set — the correct AWS shape for a Client
// VPN endpoint with no active clients.
type clientVpnConnectionItem struct {
	ConnectionID              string                      `xml:"connectionId"`
	ClientVpnEndpointID       string                      `xml:"clientVpnEndpointId"`
	Username                  string                      `xml:"username,omitempty"`
	ClientIP                  string                      `xml:"clientIp,omitempty"`
	CommonName                string                      `xml:"commonName,omitempty"`
	ConnectionEstablishedTime string                      `xml:"connectionEstablishedTime,omitempty"`
	Status                    clientVpnEndpointStatusItem `xml:"status,omitempty"`
}

type describeClientVpnConnectionsResponse struct {
	XMLName     xml.Name `xml:"DescribeClientVpnConnectionsResponse"`
	RequestID   string   `xml:"requestId"`
	NextToken   string   `xml:"nextToken,omitempty"`
	Connections struct {
		Items []clientVpnConnectionItem `xml:"item"`
	} `xml:"connections"`
}

// terminateConnectionStatusItem mirrors AWS's TerminateConnectionStatus shape.

// terminateConnectionStatusItem mirrors AWS's TerminateConnectionStatus shape.
type terminateConnectionStatusItem struct {
	ConnectionID   string `xml:"connectionId"`
	PreviousStatus string `xml:"previousStatus,omitempty"`
	CurrentStatus  string `xml:"currentStatus,omitempty"`
}

// terminateClientVpnConnectionsResponse matches the real
// TerminateClientVpnConnectionsOutput shape: it has no top-level "return"
// field, only the endpoint/username echoed back plus the (always empty, since
// no connections exist) list of per-connection termination statuses.

// terminateClientVpnConnectionsResponse matches the real
// TerminateClientVpnConnectionsOutput shape: it has no top-level "return"
// field, only the endpoint/username echoed back plus the (always empty, since
// no connections exist) list of per-connection termination statuses.
type terminateClientVpnConnectionsResponse struct {
	XMLName             xml.Name `xml:"TerminateClientVpnConnectionsResponse"`
	RequestID           string   `xml:"requestId"`
	ClientVpnEndpointID string   `xml:"clientVpnEndpointId"`
	Username            string   `xml:"username,omitempty"`
	ConnectionStatuses  struct {
		Items []terminateConnectionStatusItem `xml:"item"`
	} `xml:"connectionStatuses"`
}

type exportClientVpnClientConfigurationResponse struct {
	XMLName             xml.Name `xml:"ExportClientVpnClientConfigurationResponse"`
	RequestID           string   `xml:"requestId"`
	ClientConfiguration string   `xml:"clientConfiguration"`
}

type exportClientVpnClientCertificateRevocationListResponse struct {
	XMLName                   xml.Name                    `xml:"ExportClientVpnClientCertificateRevocationListResponse"`
	RequestID                 string                      `xml:"requestId"`
	CertificateRevocationList string                      `xml:"certificateRevocationList"`
	Status                    clientVpnEndpointStatusItem `xml:"status"`
}

func toClientVpnEndpointItem(ep *ClientVpnEndpoint) clientVpnEndpointItem {
	return clientVpnEndpointItem{
		ClientVpnEndpointID:  ep.ClientVpnEndpointID,
		DNSName:              ep.DNSName,
		Status:               clientVpnEndpointStatusItem{Code: ep.Status},
		Description:          ep.Description,
		ClientCidrBlock:      ep.ClientCidrBlock,
		DNSServers:           stringItemSet{Items: ep.DNSServers},
		VpnProtocol:          ep.VpnProtocol,
		TransportProtocol:    ep.TransportProtocol,
		VpcID:                ep.VPCID,
		VpnPort:              ep.VpnPort,
		SplitTunnel:          ep.SplitTunnel,
		SecurityGroupIDSet:   stringItemSet{Items: ep.SecurityGroupIDs},
		ServerCertificateArn: ep.ServerCertificateArn,
		SessionTimeoutHours:  ep.SessionTimeoutHours,
		SelfServicePortalURL: ep.SelfServicePortalURL,
		CreationTime:         ep.CreationTime,
	}
}

// parseClientVpnEndpointOptions extracts the optional advanced Client VPN
// endpoint fields shared by CreateClientVpnEndpoint and ModifyClientVpnEndpoint.

// parseClientVpnEndpointOptions extracts the optional advanced Client VPN
// endpoint fields shared by CreateClientVpnEndpoint and ModifyClientVpnEndpoint.
func parseClientVpnEndpointOptions(vals url.Values) ClientVpnEndpointOptions {
	opts := ClientVpnEndpointOptions{
		ServerCertificateArn: vals.Get("ServerCertificateArn"),
		TransportProtocol:    vals.Get("TransportProtocol"),
		VpcID:                vals.Get("VpcId"),
		SecurityGroupIDs:     parseMemberList(vals, "SecurityGroupId"),
		SelfServicePortalURL: vals.Get("SelfServicePortal"),
		TransitGatewayID:     vals.Get("TransitGatewayConfiguration.TransitGatewayId"),

		VpnPort:             parseInt32Value(vals.Get("VpnPort")),
		SessionTimeoutHours: parseInt32Value(vals.Get("SessionTimeoutHours"))}

	if v := vals.Get("SplitTunnel"); v != "" {
		splitTunnel := v == ec2BooleanTrue
		opts.SplitTunnel = &splitTunnel
	}

	return opts
}

func (h *Handler) handleCreateClientVpnEndpoint(vals url.Values, reqID string) (any, error) {
	cidr := vals.Get("ClientCidrBlock")
	description := vals.Get("Description")
	dnsServers := parseMemberList(vals, "DnsServers")

	ep, err := h.Backend.CreateClientVpnEndpointWithOptions(
		cidr, description, dnsServers, parseClientVpnEndpointOptions(vals),
	)
	if err != nil {
		return nil, err
	}

	return &createClientVpnEndpointResponse{
		RequestID:           reqID,
		ClientVpnEndpointID: ep.ClientVpnEndpointID,
		DNSName:             ep.DNSName,
		Status:              clientVpnEndpointStatusItem{Code: ep.Status},
	}, nil
}

type deleteClientVpnEndpointResponse struct {
	XMLName   xml.Name                    `xml:"DeleteClientVpnEndpointResponse"`
	RequestID string                      `xml:"requestId"`
	Status    clientVpnEndpointStatusItem `xml:"status"`
}

func (h *Handler) handleDeleteClientVpnEndpoint(vals url.Values, reqID string) (any, error) {
	id := vals.Get("ClientVpnEndpointId")
	if err := h.Backend.DeleteClientVpnEndpoint(id); err != nil {
		return nil, err
	}

	return &deleteClientVpnEndpointResponse{
		RequestID: reqID,
		Status:    clientVpnEndpointStatusItem{Code: stateDeleting},
	}, nil
}

func (h *Handler) handleDescribeClientVpnEndpoints(vals url.Values, reqID string) (any, error) {
	ids := parseMemberList(vals, "ClientVpnEndpointId")
	eps := h.Backend.DescribeClientVpnEndpoints(ids)

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	eps, nextToken = pageSlice(eps, offset, maxResults)

	resp := &describeClientVpnEndpointsResponse{RequestID: reqID, NextToken: nextToken}
	for _, ep := range eps {
		resp.ClientVpnEndpointSet.Items = append(resp.ClientVpnEndpointSet.Items, toClientVpnEndpointItem(ep))
	}

	return resp, nil
}

// associateClientVpnTargetNetworkResponse mirrors the real
// AssociateClientVpnTargetNetworkOutput shape: associationId and status are
// direct children of the response root, not nested under an
// "associationStatus" wrapper.

// associateClientVpnTargetNetworkResponse mirrors the real
// AssociateClientVpnTargetNetworkOutput shape: associationId and status are
// direct children of the response root, not nested under an
// "associationStatus" wrapper.
type associateClientVpnTargetNetworkResponse struct {
	XMLName       xml.Name                    `xml:"AssociateClientVpnTargetNetworkResponse"`
	RequestID     string                      `xml:"requestId"`
	AssociationID string                      `xml:"associationId"`
	Status        clientVpnEndpointStatusItem `xml:"status"`
}

func (h *Handler) handleAssociateClientVpnTargetNetwork(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	subnetID := vals.Get("SubnetId")
	assocID, err := h.Backend.AssociateClientVpnTargetNetwork(endpointID, subnetID)
	if err != nil {
		return nil, err
	}

	return &associateClientVpnTargetNetworkResponse{
		RequestID:     reqID,
		AssociationID: assocID,
		Status:        clientVpnEndpointStatusItem{Code: "associating"},
	}, nil
}

// disassociateClientVpnTargetNetworkResponse mirrors the real
// DisassociateClientVpnTargetNetworkOutput shape: associationId and status,
// not a bare Return bool.
type disassociateClientVpnTargetNetworkResponse struct {
	XMLName       xml.Name                    `xml:"DisassociateClientVpnTargetNetworkResponse"`
	RequestID     string                      `xml:"requestId"`
	AssociationID string                      `xml:"associationId"`
	Status        clientVpnEndpointStatusItem `xml:"status"`
}

func (h *Handler) handleDisassociateClientVpnTargetNetwork(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	assocID := vals.Get("AssociationId")
	if err := h.Backend.DisassociateClientVpnTargetNetwork(endpointID, assocID); err != nil {
		return nil, err
	}

	return &disassociateClientVpnTargetNetworkResponse{
		RequestID:     reqID,
		AssociationID: assocID,
		Status:        clientVpnEndpointStatusItem{Code: "disassociating"},
	}, nil
}

func (h *Handler) handleDescribeClientVpnTargetNetworks(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	networks, err := h.Backend.DescribeClientVpnTargetNetworks(endpointID)
	if err != nil {
		return nil, err
	}

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	networks, nextToken = pageSlice(networks, offset, maxResults)

	resp := &describeClientVpnTargetNetworksResponse{RequestID: reqID, NextToken: nextToken}
	for _, tn := range networks {
		resp.ClientVpnTargetNetworks.Items = append(
			resp.ClientVpnTargetNetworks.Items,
			clientVpnTargetNetworkItem{
				AssociationID:       tn.AssociationID,
				TargetNetworkID:     tn.SubnetID,
				ClientVpnEndpointID: tn.ClientVpnEndpointID,
				Status:              clientVpnEndpointStatusItem{Code: tn.Status},
				VpcID:               tn.VPCID,
				SecurityGroups:      stringItemSet{Items: tn.SecurityGroups},
			},
		)
	}

	return resp, nil
}

type clientVpnRouteStatusItem struct {
	Code string `xml:"code"`
}

type createClientVpnRouteResponse struct {
	XMLName   xml.Name                 `xml:"CreateClientVpnRouteResponse"`
	RequestID string                   `xml:"requestId"`
	Status    clientVpnRouteStatusItem `xml:"status"`
}

func (h *Handler) handleCreateClientVpnRoute(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	cidr := vals.Get("DestinationCidrBlock")
	description := vals.Get("Description")
	if err := h.Backend.CreateClientVpnRoute(endpointID, cidr, description); err != nil {
		return nil, err
	}

	return &createClientVpnRouteResponse{
		RequestID: reqID,
		Status:    clientVpnRouteStatusItem{Code: "creating"},
	}, nil
}

type deleteClientVpnRouteResponse struct {
	XMLName   xml.Name                 `xml:"DeleteClientVpnRouteResponse"`
	RequestID string                   `xml:"requestId"`
	Status    clientVpnRouteStatusItem `xml:"status"`
}

func (h *Handler) handleDeleteClientVpnRoute(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	cidr := vals.Get("DestinationCidrBlock")
	if err := h.Backend.DeleteClientVpnRoute(endpointID, cidr); err != nil {
		return nil, err
	}

	return &deleteClientVpnRouteResponse{
		RequestID: reqID,
		Status:    clientVpnRouteStatusItem{Code: stateDeleting},
	}, nil
}

func (h *Handler) handleDescribeClientVpnRoutes(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	routes, err := h.Backend.DescribeClientVpnRoutes(endpointID)
	if err != nil {
		return nil, err
	}

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	routes, nextToken = pageSlice(routes, offset, maxResults)

	resp := &describeClientVpnRoutesResponse{RequestID: reqID, NextToken: nextToken}
	for _, r := range routes {
		resp.Routes.Items = append(resp.Routes.Items, clientVpnRouteItem{
			DestinationCidr: r.DestinationCidr,
			Status:          clientVpnEndpointStatusItem{Code: r.Status},
			Description:     r.Description,
			Origin:          r.Origin,
			TargetSubnet:    r.TargetSubnet,
		})
	}

	return resp, nil
}

// authorizeClientVpnIngressResponse matches the real
// AuthorizeClientVpnIngressOutput shape: a single nested Status, no
// top-level "return" field at all (ec2@v1.319.1 deserializers.go:
// awsEc2query_deserializeOpDocumentAuthorizeClientVpnIngressOutput).
type authorizeClientVpnIngressResponse struct {
	XMLName   xml.Name                    `xml:"AuthorizeClientVpnIngressResponse"`
	RequestID string                      `xml:"requestId"`
	Status    clientVpnEndpointStatusItem `xml:"status"`
}

// revokeClientVpnIngressResponse mirrors authorizeClientVpnIngressResponse
// (ec2@v1.319.1 deserializers.go:
// awsEc2query_deserializeOpDocumentRevokeClientVpnIngressOutput).
type revokeClientVpnIngressResponse struct {
	XMLName   xml.Name                    `xml:"RevokeClientVpnIngressResponse"`
	RequestID string                      `xml:"requestId"`
	Status    clientVpnEndpointStatusItem `xml:"status"`
}

func (h *Handler) handleAuthorizeClientVpnIngress(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	// TargetNetworkCidr is the real AWS request field for the destination CIDR.
	// AuthorizeAllGroups is an unrelated boolean flag — a previous version of
	// this handler incorrectly read it as if it held the CIDR value.
	cidr := vals.Get("TargetNetworkCidr")
	description := vals.Get("Description")
	if err := h.Backend.AuthorizeClientVpnIngress(endpointID, cidr, description); err != nil {
		return nil, err
	}

	return &authorizeClientVpnIngressResponse{
		RequestID: reqID,
		Status:    clientVpnEndpointStatusItem{Code: "authorizing"},
	}, nil
}

func (h *Handler) handleRevokeClientVpnIngress(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	cidr := vals.Get("TargetNetworkCidr")
	if err := h.Backend.RevokeClientVpnIngress(endpointID, cidr); err != nil {
		return nil, err
	}

	return &revokeClientVpnIngressResponse{
		RequestID: reqID,
		Status:    clientVpnEndpointStatusItem{Code: "revoking"},
	}, nil
}

func (h *Handler) handleDescribeClientVpnAuthorizationRules(
	vals url.Values,
	reqID string,
) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	rules, err := h.Backend.DescribeClientVpnAuthorizationRules(endpointID)
	if err != nil {
		return nil, err
	}

	maxResults, offset, err := parseEC2Pagination(vals, ec2PageMinDefault, ec2PageMaxDefault, ec2PageMaxDefault)
	if err != nil {
		return nil, err
	}

	var nextToken string
	rules, nextToken = pageSlice(rules, offset, maxResults)

	resp := &describeClientVpnAuthorizationRulesResponse{RequestID: reqID, NextToken: nextToken}
	for _, r := range rules {
		resp.AuthorizationRules.Items = append(resp.AuthorizationRules.Items, clientVpnAuthRuleItem{
			Cidr:        r.Cidr,
			Status:      clientVpnEndpointStatusItem{Code: r.Status},
			Description: r.Description,
			GroupID:     r.GroupID,
			AccessAll:   r.AccessAll,
		})
	}

	return resp, nil
}

func (h *Handler) handleModifyClientVpnEndpoint(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	description := vals.Get("Description")
	dnsServers := parseMemberList(vals, "DnsServers.CustomDnsServers")
	if err := h.Backend.ModifyClientVpnEndpointWithOptions(
		endpointID, description, dnsServers, parseClientVpnEndpointOptions(vals),
	); err != nil {
		return nil, err
	}

	return &stubResponse{
		XMLName:   xml.Name{Local: "ModifyClientVpnEndpointResponse"},
		RequestID: reqID,
		Return:    true,
	}, nil
}

// applySecurityGroupsToClientVpnTargetNetworkResponse matches
// ApplySecurityGroupsToClientVpnTargetNetworkOutput (ec2@v1.319.1
// api_op_ApplySecurityGroupsToClientVpnTargetNetwork.go): SecurityGroupIds,
// no Return member.
type applySecurityGroupsToClientVpnTargetNetworkResponse struct {
	XMLName         xml.Name `xml:"ApplySecurityGroupsToClientVpnTargetNetworkResponse"`
	RequestID       string   `xml:"requestId"`
	SecurityGroupID []string `xml:"securityGroupIds>item"`
}

func (h *Handler) handleApplySecurityGroupsToClientVpnTargetNetwork(
	vals url.Values,
	reqID string,
) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	sgs := parseMemberList(vals, "SecurityGroupId")
	if err := h.Backend.ApplySecurityGroupsToClientVpnTargetNetwork(endpointID, sgs); err != nil {
		return nil, err
	}

	return &applySecurityGroupsToClientVpnTargetNetworkResponse{
		RequestID:       reqID,
		SecurityGroupID: sgs,
	}, nil
}

func (h *Handler) handleDescribeClientVpnConnections(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	_, err := h.Backend.DescribeClientVpnConnections(endpointID)
	if err != nil {
		return nil, err
	}

	return &describeClientVpnConnectionsResponse{RequestID: reqID}, nil
}

func (h *Handler) handleTerminateClientVpnConnections(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	if err := h.Backend.TerminateClientVpnConnections(endpointID); err != nil {
		return nil, err
	}

	// No connections ever exist in this backend (nothing establishes a real
	// OpenVPN session), so ConnectionStatuses is always empty — the correct
	// AWS shape for terminating connections on an endpoint with no clients.
	return &terminateClientVpnConnectionsResponse{
		RequestID:           reqID,
		ClientVpnEndpointID: endpointID,
		Username:            vals.Get("Username"),
	}, nil
}

func (h *Handler) handleExportClientVpnClientConfiguration(vals url.Values, reqID string) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	config, err := h.Backend.ExportClientVpnClientConfiguration(endpointID)
	if err != nil {
		return nil, err
	}

	return &exportClientVpnClientConfigurationResponse{
		RequestID:           reqID,
		ClientConfiguration: config,
	}, nil
}

func (h *Handler) handleExportClientVpnClientCertificateRevocationList(
	vals url.Values,
	reqID string,
) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	crl, err := h.Backend.ExportClientVpnClientCertificateRevocationList(endpointID)
	if err != nil {
		return nil, err
	}

	return &exportClientVpnClientCertificateRevocationListResponse{
		RequestID:                 reqID,
		CertificateRevocationList: crl,
		Status:                    clientVpnEndpointStatusItem{Code: stateActive},
	}, nil
}

func (h *Handler) handleImportClientVpnClientCertificateRevocationList(
	vals url.Values,
	reqID string,
) (any, error) {
	endpointID := vals.Get("ClientVpnEndpointId")
	crl := vals.Get("CertificateRevocationList")
	if err := h.Backend.ImportClientVpnClientCertificateRevocationList(endpointID, crl); err != nil {
		return nil, err
	}

	return &stubResponse{
		XMLName:   xml.Name{Local: "ImportClientVpnClientCertificateRevocationListResponse"},
		RequestID: reqID,
		Return:    true,
	}, nil
}

// ---- Transit Gateway Client VPN Attachment handlers ----

// tgwClientVpnAttachmentItem is the wire shape of a
// TransitGatewayClientVpnAttachment, field-diffed against
// aws-sdk-go-v2/service/ec2/types.TransitGatewayClientVpnAttachment.
type tgwClientVpnAttachmentItem struct {
	TransitGatewayAttachmentID string `xml:"transitGatewayAttachmentId,omitempty"`
	TransitGatewayID           string `xml:"transitGatewayId,omitempty"`
	ClientVpnEndpointID        string `xml:"clientVpnEndpointId,omitempty"`
	ClientVpnOwnerID           string `xml:"clientVpnOwnerId,omitempty"`
	State                      string `xml:"state,omitempty"`
	CreationTime               string `xml:"creationTime,omitempty"`
}

func toTGWClientVpnAttachmentItem(att *TransitGatewayClientVpnAttachment) tgwClientVpnAttachmentItem {
	return tgwClientVpnAttachmentItem{
		TransitGatewayAttachmentID: att.TransitGatewayAttachmentID,
		TransitGatewayID:           att.TransitGatewayID,
		ClientVpnEndpointID:        att.ClientVpnEndpointID,
		ClientVpnOwnerID:           att.ClientVpnOwnerID,
		State:                      att.State,
		CreationTime:               att.CreationTime,
	}
}

type acceptTGWClientVpnAttachmentResponse struct {
	XMLName                           xml.Name                   `xml:"AcceptTransitGatewayClientVpnAttachmentResponse"`
	Xmlns                             string                     `xml:"xmlns,attr"`
	RequestID                         string                     `xml:"requestId"`
	TransitGatewayClientVpnAttachment tgwClientVpnAttachmentItem `xml:"transitGatewayClientVpnAttachment"`
}

func (h *Handler) handleAcceptTransitGatewayClientVpnAttachment(vals url.Values, reqID string) (any, error) {
	attachmentID := vals.Get("TransitGatewayAttachmentId")

	att, err := h.Backend.AcceptTransitGatewayClientVpnAttachment(attachmentID)
	if err != nil {
		return nil, err
	}

	return &acceptTGWClientVpnAttachmentResponse{
		Xmlns:                             ec2XMLNS,
		RequestID:                         reqID,
		TransitGatewayClientVpnAttachment: toTGWClientVpnAttachmentItem(att),
	}, nil
}

type rejectTGWClientVpnAttachmentResponse struct {
	XMLName                           xml.Name                   `xml:"RejectTransitGatewayClientVpnAttachmentResponse"`
	Xmlns                             string                     `xml:"xmlns,attr"`
	RequestID                         string                     `xml:"requestId"`
	TransitGatewayClientVpnAttachment tgwClientVpnAttachmentItem `xml:"transitGatewayClientVpnAttachment"`
}

func (h *Handler) handleRejectTransitGatewayClientVpnAttachment(vals url.Values, reqID string) (any, error) {
	attachmentID := vals.Get("TransitGatewayAttachmentId")

	att, err := h.Backend.RejectTransitGatewayClientVpnAttachment(attachmentID)
	if err != nil {
		return nil, err
	}

	return &rejectTGWClientVpnAttachmentResponse{
		Xmlns:                             ec2XMLNS,
		RequestID:                         reqID,
		TransitGatewayClientVpnAttachment: toTGWClientVpnAttachmentItem(att),
	}, nil
}

type deleteTGWClientVpnAttachmentResponse struct {
	XMLName                           xml.Name                   `xml:"DeleteTransitGatewayClientVpnAttachmentResponse"`
	Xmlns                             string                     `xml:"xmlns,attr"`
	RequestID                         string                     `xml:"requestId"`
	TransitGatewayClientVpnAttachment tgwClientVpnAttachmentItem `xml:"transitGatewayClientVpnAttachment"`
}

func (h *Handler) handleDeleteTransitGatewayClientVpnAttachment(vals url.Values, reqID string) (any, error) {
	attachmentID := vals.Get("TransitGatewayAttachmentId")

	att, err := h.Backend.DeleteTransitGatewayClientVpnAttachment(attachmentID)
	if err != nil {
		return nil, err
	}

	return &deleteTGWClientVpnAttachmentResponse{
		Xmlns:                             ec2XMLNS,
		RequestID:                         reqID,
		TransitGatewayClientVpnAttachment: toTGWClientVpnAttachmentItem(att),
	}, nil
}

// ---- TGW Peering handlers ----

// registerClientVpnOps registers the ClientVpn operation handlers.
func registerClientVpnOps(h *Handler, ops map[string]ec2ActionFn) {
	ops["CreateClientVpnEndpoint"] = h.handleCreateClientVpnEndpoint
	ops["DeleteClientVpnEndpoint"] = h.handleDeleteClientVpnEndpoint
	ops["DescribeClientVpnEndpoints"] = h.handleDescribeClientVpnEndpoints
	ops["AssociateClientVpnTargetNetwork"] = h.handleAssociateClientVpnTargetNetwork
	ops["DescribeClientVpnTargetNetworks"] = h.handleDescribeClientVpnTargetNetworks
	ops["CreateClientVpnRoute"] = h.handleCreateClientVpnRoute
	ops["DeleteClientVpnRoute"] = h.handleDeleteClientVpnRoute
	ops["DescribeClientVpnRoutes"] = h.handleDescribeClientVpnRoutes
	ops["AuthorizeClientVpnIngress"] = h.handleAuthorizeClientVpnIngress
	ops["DescribeClientVpnAuthorizationRules"] = h.handleDescribeClientVpnAuthorizationRules
	ops["TerminateClientVpnConnections"] = h.handleTerminateClientVpnConnections
	ops["DescribeClientVpnConnections"] = h.handleDescribeClientVpnConnections
	ops["ApplySecurityGroupsToClientVpnTargetNetwork"] = h.handleApplySecurityGroupsToClientVpnTargetNetwork
	ops["RevokeClientVpnIngress"] = h.handleRevokeClientVpnIngress
	ops["ModifyClientVpnEndpoint"] = h.handleModifyClientVpnEndpoint
	ops["DisassociateClientVpnTargetNetwork"] = h.handleDisassociateClientVpnTargetNetwork
	ops["ExportClientVpnClientConfiguration"] = h.handleExportClientVpnClientConfiguration
	ops["ExportClientVpnClientCertificateRevocationList"] = h.handleExportClientVpnClientCertificateRevocationList
	ops["ImportClientVpnClientCertificateRevocationList"] = h.handleImportClientVpnClientCertificateRevocationList

	registerTGWClientVpnAttachmentOps(h, ops)
}

// registerTGWClientVpnAttachmentOps registers the Transit Gateway Client VPN
// attachment operation handlers (parity-4). Split out from
// registerClientVpnOps so each stays a distinct, independently-readable unit.
func registerTGWClientVpnAttachmentOps(h *Handler, ops map[string]ec2ActionFn) {
	ops["AcceptTransitGatewayClientVpnAttachment"] = h.handleAcceptTransitGatewayClientVpnAttachment
	ops["RejectTransitGatewayClientVpnAttachment"] = h.handleRejectTransitGatewayClientVpnAttachment
	ops["DeleteTransitGatewayClientVpnAttachment"] = h.handleDeleteTransitGatewayClientVpnAttachment
}

// clientVpnSupportedOperations lists the operation names registered by
// registerClientVpnOps, for GetSupportedOperations().
func clientVpnSupportedOperations() []string {
	return []string{
		"CreateClientVpnEndpoint",
		"DeleteClientVpnEndpoint",
		"DescribeClientVpnEndpoints",
		"AssociateClientVpnTargetNetwork",
		"DescribeClientVpnTargetNetworks",
		"CreateClientVpnRoute",
		"DeleteClientVpnRoute",
		"DescribeClientVpnRoutes",
		"AuthorizeClientVpnIngress",
		"DescribeClientVpnAuthorizationRules",
		"TerminateClientVpnConnections",
		"DescribeClientVpnConnections",
		"ApplySecurityGroupsToClientVpnTargetNetwork",
		"RevokeClientVpnIngress",
		"ModifyClientVpnEndpoint",
		"DisassociateClientVpnTargetNetwork",
		"ExportClientVpnClientConfiguration",
		"ExportClientVpnClientCertificateRevocationList",
		"ImportClientVpnClientCertificateRevocationList",
		"AcceptTransitGatewayClientVpnAttachment",
		"RejectTransitGatewayClientVpnAttachment",
		"DeleteTransitGatewayClientVpnAttachment",
	}
}
