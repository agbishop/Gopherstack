package elb

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
)

func (h *Handler) handleModifyLoadBalancerAttributes(ctx context.Context, vals url.Values) (any, error) {
	name := vals.Get("LoadBalancerName")
	if name == "" {
		return nil, fmt.Errorf("%w: LoadBalancerName is required", ErrInvalidParameter)
	}

	attrs, mask := parseLoadBalancerAttributes(vals)

	const minTimeout = 1
	const maxIdleTimeout = 3600
	const maxDrainingTimeout = 3600

	if attrs.IdleTimeout < minTimeout || attrs.IdleTimeout > maxIdleTimeout {
		return nil, fmt.Errorf(
			"%w: IdleTimeout must be between 1 and 3600 seconds",
			ErrInvalidParameter,
		)
	}

	if attrs.ConnectionDraining &&
		(attrs.ConnectionDrainingTimeout < minTimeout || attrs.ConnectionDrainingTimeout > maxDrainingTimeout) {
		return nil, fmt.Errorf(
			"%w: ConnectionDrainingTimeout must be between 1 and 3600 seconds",
			ErrInvalidParameter,
		)
	}

	validDesyncModes := map[string]bool{"defensive": true, "strictest": true, "monitor": true}
	if attrs.DesyncMitigationMode != "" && !validDesyncModes[attrs.DesyncMitigationMode] {
		return nil, fmt.Errorf(
			"%w: DesyncMitigationMode must be one of 'defensive', 'strictest', 'monitor'",
			ErrInvalidParameter,
		)
	}

	// Validate AccessLog: enabled requires S3BucketName; EmitInterval must be 5 or 60.
	if attrs.AccessLog.Enabled && attrs.AccessLog.S3BucketName == "" {
		return nil, fmt.Errorf(
			"%w: AccessLog.S3BucketName is required when AccessLog is enabled",
			ErrInvalidParameter,
		)
	}

	if attrs.AccessLog.EmitInterval != 0 && attrs.AccessLog.EmitInterval != 5 && attrs.AccessLog.EmitInterval != 60 {
		return nil, fmt.Errorf(
			"%w: AccessLog.EmitInterval must be 5 or 60 minutes",
			ErrInvalidParameter,
		)
	}

	result, err := h.Backend.ModifyLoadBalancerAttributes(ctx, name, attrs, mask)
	if err != nil {
		return nil, err
	}

	return &modifyLoadBalancerAttributesResponse{
		Xmlns: elbXMLNS,
		Result: modifyLoadBalancerAttributesResult{
			LoadBalancerAttributes: toXMLLoadBalancerAttributes(result),
		},
		ResponseMetadata: xmlResponseMetadata{RequestID: "elb-modifyattrs-" + name},
	}, nil
}

func (h *Handler) handleDescribeLoadBalancerAttributes(ctx context.Context, vals url.Values) (any, error) {
	name := vals.Get("LoadBalancerName")
	if name == "" {
		return nil, fmt.Errorf("%w: LoadBalancerName is required", ErrInvalidParameter)
	}

	attrs, err := h.Backend.DescribeLoadBalancerAttributes(ctx, name)
	if err != nil {
		return nil, err
	}

	return &describeLoadBalancerAttributesResponse{
		Xmlns: elbXMLNS,
		Result: describeLoadBalancerAttributesResult{
			LoadBalancerAttributes: toXMLLoadBalancerAttributes(attrs),
		},
		ResponseMetadata: xmlResponseMetadata{RequestID: "elb-describeattrs-" + name},
	}, nil
}

// parseLoadBalancerAttributes reads LoadBalancerAttributes.* form values into a
// LoadBalancerAttributes struct, plus a mask marking which independent
// attribute groups were actually present in the request. A group's fields
// default to the service defaults when absent, but the mask tells the caller
// not to apply them — each group is independently settable and an absent
// group must leave the load balancer's current value untouched.
func parseLoadBalancerAttributes(vals url.Values) (LoadBalancerAttributes, LoadBalancerAttributesMask) {
	attrs := defaultLBAttributes()

	var mask LoadBalancerAttributesMask

	if v := vals.Get("LoadBalancerAttributes.CrossZoneLoadBalancing.Enabled"); v != "" {
		attrs.CrossZoneLoadBalancing = v == boolStrTrue
		mask.CrossZoneLoadBalancing = true
	}

	if v := vals.Get("LoadBalancerAttributes.ConnectionDraining.Enabled"); v != "" {
		attrs.ConnectionDraining = v == boolStrTrue
		mask.ConnectionDraining = true
	}

	if v := vals.Get("LoadBalancerAttributes.ConnectionDraining.Timeout"); v != "" {
		if n, err := parseInt32(v); err == nil {
			attrs.ConnectionDrainingTimeout = n
		}
	}

	if v := vals.Get("LoadBalancerAttributes.ConnectionSettings.IdleTimeout"); v != "" {
		if n, err := parseInt32(v); err == nil {
			attrs.IdleTimeout = n
		}

		mask.ConnectionSettings = true
	}

	// The desync mitigation mode is passed as an AdditionalAttribute with
	// key "elb.http.desyncmitigationmode".
	for i := 1; ; i++ {
		k := vals.Get(fmt.Sprintf("LoadBalancerAttributes.AdditionalAttributes.member.%d.Key", i))
		if k == "" {
			break
		}

		v := vals.Get(fmt.Sprintf("LoadBalancerAttributes.AdditionalAttributes.member.%d.Value", i))

		if k == "elb.http.desyncmitigationmode" {
			attrs.DesyncMitigationMode = v
			mask.DesyncMitigationMode = true
		}
	}

	// Parse AccessLog attributes.
	if v := vals.Get("LoadBalancerAttributes.AccessLog.Enabled"); v != "" {
		attrs.AccessLog.Enabled = v == boolStrTrue
		mask.AccessLog = true
	}

	if v := vals.Get("LoadBalancerAttributes.AccessLog.S3BucketName"); v != "" {
		attrs.AccessLog.S3BucketName = v
	}

	if v := vals.Get("LoadBalancerAttributes.AccessLog.S3BucketPrefix"); v != "" {
		attrs.AccessLog.S3BucketPrefix = v
	}

	if v := vals.Get("LoadBalancerAttributes.AccessLog.EmitInterval"); v != "" {
		if n, err := parseInt32(v); err == nil {
			attrs.AccessLog.EmitInterval = n
		}
	}

	return attrs, mask
}

// toXMLLoadBalancerAttributes converts a LoadBalancerAttributes to its XML wire representation.
func toXMLLoadBalancerAttributes(attrs *LoadBalancerAttributes) xmlLoadBalancerAttributes {
	additionalAttrs := []xmlAdditionalAttribute{
		{Key: "elb.http.desyncmitigationmode", Value: attrs.DesyncMitigationMode},
	}

	return xmlLoadBalancerAttributes{
		CrossZoneLoadBalancing: xmlBoolAttribute{Enabled: attrs.CrossZoneLoadBalancing},
		ConnectionDraining: xmlConnectionDraining{
			Enabled: attrs.ConnectionDraining,
			Timeout: attrs.ConnectionDrainingTimeout,
		},
		ConnectionSettings: xmlConnectionSettings{IdleTimeout: attrs.IdleTimeout},
		AccessLog: xmlAccessLog{
			Enabled:        attrs.AccessLog.Enabled,
			S3BucketName:   attrs.AccessLog.S3BucketName,
			S3BucketPrefix: attrs.AccessLog.S3BucketPrefix,
			EmitInterval:   attrs.AccessLog.EmitInterval,
		},
		AdditionalAttributes: xmlAdditionalAttributeList{
			Members: additionalAttrs,
		},
	}
}

type xmlBoolAttribute struct {
	Enabled bool `xml:"Enabled"`
}

type xmlConnectionDraining struct {
	Enabled bool  `xml:"Enabled"`
	Timeout int32 `xml:"Timeout"`
}

type xmlConnectionSettings struct {
	IdleTimeout int32 `xml:"IdleTimeout"`
}

type xmlAdditionalAttribute struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

type xmlAdditionalAttributeList struct {
	Members []xmlAdditionalAttribute `xml:"member"`
}

type xmlAccessLog struct {
	S3BucketName   string `xml:"S3BucketName,omitempty"`
	S3BucketPrefix string `xml:"S3BucketPrefix,omitempty"`
	EmitInterval   int32  `xml:"EmitInterval,omitempty"`
	Enabled        bool   `xml:"Enabled"`
}

type xmlLoadBalancerAttributes struct {
	AdditionalAttributes   xmlAdditionalAttributeList `xml:"AdditionalAttributes"`
	AccessLog              xmlAccessLog               `xml:"AccessLog"`
	ConnectionDraining     xmlConnectionDraining      `xml:"ConnectionDraining"`
	ConnectionSettings     xmlConnectionSettings      `xml:"ConnectionSettings"`
	CrossZoneLoadBalancing xmlBoolAttribute           `xml:"CrossZoneLoadBalancing"`
}

type modifyLoadBalancerAttributesResult struct {
	LoadBalancerAttributes xmlLoadBalancerAttributes `xml:"LoadBalancerAttributes"`
}

type modifyLoadBalancerAttributesResponse struct {
	XMLName          xml.Name                           `xml:"ModifyLoadBalancerAttributesResponse"`
	Xmlns            string                             `xml:"xmlns,attr"`
	ResponseMetadata xmlResponseMetadata                `xml:"ResponseMetadata"`
	Result           modifyLoadBalancerAttributesResult `xml:"ModifyLoadBalancerAttributesResult"`
}

type describeLoadBalancerAttributesResult struct {
	LoadBalancerAttributes xmlLoadBalancerAttributes `xml:"LoadBalancerAttributes"`
}

type describeLoadBalancerAttributesResponse struct {
	XMLName          xml.Name                             `xml:"DescribeLoadBalancerAttributesResponse"`
	Xmlns            string                               `xml:"xmlns,attr"`
	ResponseMetadata xmlResponseMetadata                  `xml:"ResponseMetadata"`
	Result           describeLoadBalancerAttributesResult `xml:"DescribeLoadBalancerAttributesResult"`
}
