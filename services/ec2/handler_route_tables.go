package ec2

import (
	"encoding/xml"
	"fmt"
	"net/url"
)

func (h *Handler) handleReplaceRoute(vals url.Values, reqID string) (any, error) {
	rtID := vals.Get("RouteTableId")
	destCIDR := vals.Get("DestinationCidrBlock")
	gatewayID := vals.Get("GatewayId")
	natGatewayID := vals.Get("NatGatewayId")

	if err := h.Backend.ReplaceRoute(rtID, destCIDR, gatewayID, natGatewayID); err != nil {
		return nil, err
	}

	return &stubResponse{
		XMLName:   xml.Name{Local: "ReplaceRouteResponse"},
		RequestID: reqID,
		Return:    true,
	}, nil
}

// registerRouteTablesOps registers the RouteTables operation handlers.
func registerRouteTablesOps(h *Handler, ops map[string]ec2ActionFn) {
	ops["ReplaceRoute"] = h.handleReplaceRoute
}

// routeTablesSupportedOperations lists the operation names registered by
// registerRouteTablesOps, for GetSupportedOperations().
func routeTablesSupportedOperations() []string {
	return []string{
		"ReplaceRoute",
	}
}

type routeItem struct {
	DestinationCIDR string `xml:"destinationCidrBlock"`
	GatewayID       string `xml:"gatewayId,omitempty"`
	NatGatewayID    string `xml:"natGatewayId,omitempty"`
	State           string `xml:"state"`
}

type routeSet struct {
	Items []routeItem `xml:"item"`
}

type assocItem struct {
	RouteTableAssociationID string `xml:"routeTableAssociationId"`
	RouteTableID            string `xml:"routeTableId"`
	SubnetID                string `xml:"subnetId,omitempty"`
	Main                    bool   `xml:"main"`
}

type assocSet struct {
	Items []assocItem `xml:"item"`
}

type routeTableItem struct {
	RouteTableID   string          `xml:"routeTableId"`
	VPCID          string          `xml:"vpcId"`
	RouteSet       routeSet        `xml:"routeSet"`
	AssociationSet assocSet        `xml:"associationSet"`
	TagSet         []simpleTagItem `xml:"tagSet>item"`
}

type routeTableItemSet struct {
	Items []routeTableItem `xml:"item"`
}

type describeRouteTablesResponse struct {
	XMLName       xml.Name          `xml:"DescribeRouteTablesResponse"`
	Xmlns         string            `xml:"xmlns,attr"`
	RequestID     string            `xml:"requestId"`
	RouteTableSet routeTableItemSet `xml:"routeTableSet"`
}

type createRouteTableResponse struct {
	XMLName    xml.Name       `xml:"CreateRouteTableResponse"`
	Xmlns      string         `xml:"xmlns,attr"`
	RequestID  string         `xml:"requestId"`
	RouteTable routeTableItem `xml:"routeTable"`
}

type deleteRouteTableResponse struct {
	XMLName   xml.Name `xml:"DeleteRouteTableResponse"`
	Xmlns     string   `xml:"xmlns,attr"`
	RequestID string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

type createRouteResponse struct {
	XMLName   xml.Name `xml:"CreateRouteResponse"`
	Xmlns     string   `xml:"xmlns,attr"`
	RequestID string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

type deleteRouteResponse struct {
	XMLName   xml.Name `xml:"DeleteRouteResponse"`
	Xmlns     string   `xml:"xmlns,attr"`
	RequestID string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

type associateRouteTableResponse struct {
	XMLName       xml.Name `xml:"AssociateRouteTableResponse"`
	Xmlns         string   `xml:"xmlns,attr"`
	RequestID     string   `xml:"requestId"`
	AssociationID string   `xml:"associationId"`
}

type disassociateRouteTableResponse struct {
	XMLName   xml.Name `xml:"DisassociateRouteTableResponse"`
	Xmlns     string   `xml:"xmlns,attr"`
	RequestID string   `xml:"requestId"`
	Return    bool     `xml:"return"`
}

func toRouteTableItem(rt *RouteTable, tags map[string]string) routeTableItem {
	routes := make([]routeItem, 0, len(rt.Routes))
	for _, r := range rt.Routes {
		routes = append(routes, routeItem(r))
	}

	assocs := make([]assocItem, 0, len(rt.Associations))
	for _, a := range rt.Associations {
		assocs = append(assocs, assocItem{
			RouteTableAssociationID: a.ID,
			RouteTableID:            a.RouteTableID,
			SubnetID:                a.SubnetID,
			Main:                    a.Main,
		})
	}

	return routeTableItem{
		RouteTableID:   rt.ID,
		VPCID:          rt.VPCID,
		RouteSet:       routeSet{Items: routes},
		AssociationSet: assocSet{Items: assocs},
		TagSet:         tagItemsFromMap(tags),
	}
}

func (h *Handler) handleCreateRouteTable(vals url.Values, reqID string) (any, error) {
	vpcID := vals.Get("VpcId")
	if vpcID == "" {
		return nil, fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	rt, err := h.Backend.CreateRouteTable(vpcID)
	if err != nil {
		return nil, err
	}

	return &createRouteTableResponse{
		Xmlns:      ec2XMLNS,
		RequestID:  reqID,
		RouteTable: toRouteTableItem(rt, nil),
	}, nil
}

func (h *Handler) handleDeleteRouteTable(vals url.Values, reqID string) (any, error) {
	id := vals.Get("RouteTableId")
	if id == "" {
		return nil, fmt.Errorf("%w: RouteTableId is required", ErrInvalidParameter)
	}

	if err := h.Backend.DeleteRouteTable(id); err != nil {
		return nil, err
	}

	return &deleteRouteTableResponse{
		Xmlns:     ec2XMLNS,
		RequestID: reqID,
		Return:    true,
	}, nil
}

func (h *Handler) handleDescribeRouteTables(vals url.Values, reqID string) (any, error) {
	ids := parseMemberList(vals, "RouteTableId")
	rts := h.Backend.DescribeRouteTables(ids)

	filters := parseEC2Filters(vals)
	rts = applyRouteTableFilters(rts, filters, h.Backend)

	items := make([]routeTableItem, 0, len(rts))
	for _, rt := range rts {
		items = append(items, toRouteTableItem(rt, h.Backend.TagsForResource(rt.ID)))
	}

	return &describeRouteTablesResponse{
		Xmlns:         ec2XMLNS,
		RequestID:     reqID,
		RouteTableSet: routeTableItemSet{Items: items},
	}, nil
}

func (h *Handler) handleCreateRoute(vals url.Values, reqID string) (any, error) {
	rtID := vals.Get("RouteTableId")
	destCIDR := vals.Get("DestinationCidrBlock")
	gatewayID := vals.Get("GatewayId")
	natGatewayID := vals.Get("NatGatewayId")

	if rtID == "" || destCIDR == "" {
		return nil, fmt.Errorf(
			"%w: RouteTableId and DestinationCidrBlock are required",
			ErrInvalidParameter,
		)
	}

	if err := h.Backend.CreateRoute(rtID, destCIDR, gatewayID, natGatewayID); err != nil {
		return nil, err
	}

	return &createRouteResponse{
		Xmlns:     ec2XMLNS,
		RequestID: reqID,
		Return:    true,
	}, nil
}

func (h *Handler) handleDeleteRoute(vals url.Values, reqID string) (any, error) {
	rtID := vals.Get("RouteTableId")
	destCIDR := vals.Get("DestinationCidrBlock")

	if rtID == "" || destCIDR == "" {
		return nil, fmt.Errorf(
			"%w: RouteTableId and DestinationCidrBlock are required",
			ErrInvalidParameter,
		)
	}

	if err := h.Backend.DeleteRoute(rtID, destCIDR); err != nil {
		return nil, err
	}

	return &deleteRouteResponse{
		Xmlns:     ec2XMLNS,
		RequestID: reqID,
		Return:    true,
	}, nil
}

func (h *Handler) handleAssociateRouteTable(vals url.Values, reqID string) (any, error) {
	rtID := vals.Get("RouteTableId")
	subnetID := vals.Get("SubnetId")

	if rtID == "" || subnetID == "" {
		return nil, fmt.Errorf("%w: RouteTableId and SubnetId are required", ErrInvalidParameter)
	}

	assocID, err := h.Backend.AssociateRouteTable(rtID, subnetID)
	if err != nil {
		return nil, err
	}

	return &associateRouteTableResponse{
		Xmlns:         ec2XMLNS,
		RequestID:     reqID,
		AssociationID: assocID,
	}, nil
}

func (h *Handler) handleDisassociateRouteTable(vals url.Values, reqID string) (any, error) {
	assocID := vals.Get("AssociationId")
	if assocID == "" {
		return nil, fmt.Errorf("%w: AssociationId is required", ErrInvalidParameter)
	}

	if err := h.Backend.DisassociateRouteTable(assocID); err != nil {
		return nil, err
	}

	return &disassociateRouteTableResponse{
		Xmlns:     ec2XMLNS,
		RequestID: reqID,
		Return:    true,
	}, nil
}
