package elasticache

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// parseSubnetIDs extracts a list of subnet IDs from query form values.
func parseSubnetIDs(form url.Values) []string {
	var ids []string
	for i := 1; ; i++ {
		id := form.Get(fmt.Sprintf("SubnetIds.SubnetIdentifier.%d", i))
		if id == "" {
			break
		}
		ids = append(ids, id)
	}

	return ids
}

// cacheSubnetGroupXML is the XML representation of a cache subnet group.
type cacheSubnetGroupXML struct {
	ARN                         string     `xml:"ARN"`
	CacheSubnetGroupName        string     `xml:"CacheSubnetGroupName"`
	CacheSubnetGroupDescription string     `xml:"CacheSubnetGroupDescription"`
	VpcID                       string     `xml:"VpcId"`
	Subnets                     subnetsXML `xml:"Subnets"`
}

type subnetXML struct {
	SubnetIdentifier string `xml:"SubnetIdentifier"`
}

type subnetsXML struct {
	Subnet []subnetXML `xml:"Subnet"`
}

func subnetGroupToXML(sg *CacheSubnetGroup) cacheSubnetGroupXML {
	subnets := make([]subnetXML, 0, len(sg.SubnetIDs))
	for _, id := range sg.SubnetIDs {
		subnets = append(subnets, subnetXML{SubnetIdentifier: id})
	}

	return cacheSubnetGroupXML{
		ARN:                         sg.ARN,
		CacheSubnetGroupName:        sg.Name,
		CacheSubnetGroupDescription: sg.Description,
		VpcID:                       sg.VpcID,
		Subnets:                     subnetsXML{Subnet: subnets},
	}
}

func (h *Handler) createCacheSubnetGroup(ctx context.Context, c *echo.Context, form url.Values) error {
	name := form.Get("CacheSubnetGroupName")
	desc := form.Get("CacheSubnetGroupDescription")
	subnetIDs := parseSubnetIDs(form)

	sg, err := h.Backend.CreateSubnetGroup(ctx, name, desc, subnetIDs)
	if err != nil {
		if errors.Is(err, ErrSubnetGroupAlreadyExists) {
			return xmlError(
				c,
				http.StatusBadRequest,
				"CacheSubnetGroupAlreadyExists",
				"Cache subnet group already exists",
			)
		}

		if errors.Is(err, ErrCacheSubnetGroupQuotaExceeded) {
			return xmlError(c, http.StatusBadRequest, "CacheSubnetGroupQuotaExceeded", err.Error())
		}

		if errors.Is(err, ErrCacheSubnetQuotaExceeded) {
			return xmlError(c, http.StatusBadRequest, "CacheSubnetQuotaExceededFault", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	h.applyCreateTimeTags(ctx, form, sg.ARN)

	type result struct {
		XMLName          xml.Name            `xml:"CreateCacheSubnetGroupResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		CacheSubnetGroup cacheSubnetGroupXML `xml:"CreateCacheSubnetGroupResult>CacheSubnetGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		CacheSubnetGroup: subnetGroupToXML(sg),
	})
}

func (h *Handler) deleteCacheSubnetGroup(ctx context.Context, c *echo.Context, form url.Values) error {
	name := form.Get("CacheSubnetGroupName")

	if err := h.Backend.DeleteSubnetGroup(ctx, name); err != nil {
		if errors.Is(err, ErrSubnetGroupNotFound) {
			return xmlError(c, http.StatusBadRequest, "CacheSubnetGroupNotFoundFault", "Cache subnet group not found")
		}
		if errors.Is(err, ErrSubnetGroupInUse) {
			return xmlError(c, http.StatusBadRequest, "CacheSubnetGroupInUse", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName   xml.Name `xml:"DeleteCacheSubnetGroupResponse"`
		Xmlns     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"ResponseMetadata>RequestId"`
	}

	return xmlResp(c, http.StatusOK, result{Xmlns: elasticacheNS, RequestID: newRequestID()})
}

// describeCacheSubnetGroupsResultXML is the XML result for DescribeCacheSubnetGroups.
type describeCacheSubnetGroupsResultXML struct {
	XMLName           xml.Name                 `xml:"DescribeCacheSubnetGroupsResponse"`
	Xmlns             string                   `xml:"xmlns,attr"`
	Marker            string                   `xml:"DescribeCacheSubnetGroupsResult>Marker,omitempty"`
	CacheSubnetGroups cacheSubnetGroupsListXML `xml:"DescribeCacheSubnetGroupsResult>CacheSubnetGroups"`
}

// cacheSubnetGroupsListXML holds the list of cache subnet groups.
type cacheSubnetGroupsListXML struct {
	CacheSubnetGroup []cacheSubnetGroupXML `xml:"CacheSubnetGroup"`
}

func (h *Handler) describeCacheSubnetGroups(ctx context.Context, c *echo.Context, form url.Values) error {
	name := form.Get("CacheSubnetGroupName")

	p, err := describeListChecked(c, form,
		func(marker string, maxRecords int) (page.Page[CacheSubnetGroup], error) {
			return h.Backend.DescribeSubnetGroups(ctx, name, marker, maxRecords)
		},
		ErrSubnetGroupNotFound, http.StatusBadRequest, "CacheSubnetGroupNotFoundFault", "Cache subnet group not found")
	if err != nil {
		return err
	}

	items := make([]cacheSubnetGroupXML, 0, len(p.Data))
	for i := range p.Data {
		items = append(items, subnetGroupToXML(&p.Data[i]))
	}

	return xmlResp(c, http.StatusOK, describeCacheSubnetGroupsResultXML{
		Xmlns:             elasticacheNS,
		Marker:            p.Next,
		CacheSubnetGroups: cacheSubnetGroupsListXML{CacheSubnetGroup: items},
	})
}

func (h *Handler) modifyCacheSubnetGroup(ctx context.Context, c *echo.Context, form url.Values) error {
	name := form.Get("CacheSubnetGroupName")
	desc := form.Get("CacheSubnetGroupDescription")
	subnetIDs := parseSubnetIDs(form)

	sg, err := h.Backend.ModifySubnetGroup(ctx, name, desc, subnetIDs)
	if err != nil {
		if errors.Is(err, ErrSubnetGroupNotFound) {
			return xmlError(c, http.StatusBadRequest, "CacheSubnetGroupNotFoundFault", "Cache subnet group not found")
		}

		if errors.Is(err, ErrCacheSubnetQuotaExceeded) {
			return xmlError(c, http.StatusBadRequest, "CacheSubnetQuotaExceededFault", err.Error())
		}

		return xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type result struct {
		XMLName          xml.Name            `xml:"ModifyCacheSubnetGroupResponse"`
		Xmlns            string              `xml:"xmlns,attr"`
		CacheSubnetGroup cacheSubnetGroupXML `xml:"ModifyCacheSubnetGroupResult>CacheSubnetGroup"`
	}

	return xmlResp(c, http.StatusOK, result{
		Xmlns:            elasticacheNS,
		CacheSubnetGroup: subnetGroupToXML(sg),
	})
}
