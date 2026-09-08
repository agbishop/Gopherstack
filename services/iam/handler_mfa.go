package iam

import (
	"encoding/xml"
	"net/url"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// iamMFALinkDispatch wires EnableMFADevice, DeactivateMFADevice, and ListMFADevices.
func (h *Handler) iamMFALinkDispatch() map[string]iamActionFn {
	return map[string]iamActionFn{
		opEnableMFADevice: func(vals url.Values, reqID string) (any, error) {
			if err := h.Backend.EnableMFADevice(
				vals.Get("UserName"),
				vals.Get("SerialNumber"),
				vals.Get("AuthenticationCode1"),
				vals.Get("AuthenticationCode2"),
			); err != nil {
				return nil, err
			}

			return &iamSimpleTagResponse{
				XMLName:          xmlLocalName("EnableMFADeviceResponse"),
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},

		opDeactivateMFADevice: func(vals url.Values, reqID string) (any, error) {
			if err := h.Backend.DeactivateMFADevice(
				vals.Get("UserName"),
				vals.Get("SerialNumber"),
			); err != nil {
				return nil, err
			}

			return &iamSimpleTagResponse{
				XMLName:          xmlLocalName("DeactivateMFADeviceResponse"),
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},

		opListMFADevices: func(vals url.Values, reqID string) (any, error) {
			userName := vals.Get("UserName")

			var p page.Page[VirtualMFADevice]

			if userName != "" {
				var err error

				marker, maxItems := vals.Get("Marker"), parseMaxItems(vals.Get("MaxItems"))

				p, err = h.Backend.ListMFADevicesForUser(userName, marker, maxItems)
				if err != nil {
					return nil, err
				}
			}

			members := make([]mfaDeviceXML, 0, len(p.Data))

			for _, d := range p.Data {
				members = append(members, mfaDeviceXML{
					UserName:     userName,
					SerialNumber: d.SerialNumber,
					EnableDate:   isoTime(d.CreateDate),
				})
			}

			return &listMFADevicesResponse{
				XMLName: xmlLocalName("ListMFADevicesResponse"),
				Xmlns:   iamXMLNS,
				ListMFADevicesResult: listMFADevicesResult{
					MFADevices:  members,
					Marker:      p.Next,
					IsTruncated: p.Next != "",
				},
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
	}
}

// iamVirtualMFAFullDispatch overrides CreateVirtualMFADevice with QR code and seed support.
func (h *Handler) iamVirtualMFAFullDispatch() map[string]iamActionFn {
	return map[string]iamActionFn{
		opCreateVirtualMFADevice: func(vals url.Values, reqID string) (any, error) {
			device, err := h.Backend.CreateVirtualMFADeviceFull(
				vals.Get("VirtualMFADeviceName"),
				vals.Get("Path"),
			)
			if err != nil {
				return nil, err
			}

			if tags := parseIAMTags(vals); len(tags) > 0 {
				h.setTags("mfa:"+device.SerialNumber, tags)
			}

			return &CreateVirtualMFADeviceResponse{
				Xmlns: iamXMLNS,
				CreateVirtualMFADeviceResult: CreateVirtualMFADeviceResult{
					VirtualMFADevice: VirtualMFADeviceXML{
						SerialNumber:     device.SerialNumber,
						Base32StringSeed: device.Base32StringSeed,
						QRCodePNG:        device.QRCodePNG,
					},
				},
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
	}
}

// iamVirtualMFADispatch adds ListVirtualMFADevices and DeleteVirtualMFADevice.
func (h *Handler) iamVirtualMFADispatch() map[string]iamActionFn {
	return map[string]iamActionFn{
		"ListVirtualMFADevices": func(vals url.Values, reqID string) (any, error) {
			p, err := h.Backend.ListVirtualMFADevices(
				vals.Get("Marker"), parseMaxItems(vals.Get("MaxItems")),
			)
			if err != nil {
				return nil, err
			}

			xmlDevices := make([]VirtualMFADeviceXML, 0, len(p.Data))
			for i := range p.Data {
				dev := VirtualMFADeviceXML{
					SerialNumber: p.Data[i].SerialNumber,
					Tags:         tagsToXML(h.getTags("mfa:" + p.Data[i].SerialNumber)),
				}
				if owner := h.Backend.GetMFADeviceOwner(p.Data[i].SerialNumber); owner != "" {
					if u, uErr := h.Backend.GetUser(owner); uErr == nil {
						x := toUserXML(u)
						dev.User = &x
					}
				}
				xmlDevices = append(xmlDevices, dev)
			}

			return &ListVirtualMFADevicesResponse{
				Xmlns: iamXMLNS,
				ListVirtualMFADevicesResult: ListVirtualMFADevicesResult{
					VirtualMFADevices: xmlDevices,
					IsTruncated:       p.Next != "",
				},
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
		"DeleteVirtualMFADevice": func(vals url.Values, reqID string) (any, error) {
			serial := vals.Get("SerialNumber")
			if err := h.Backend.DeleteVirtualMFADevice(serial); err != nil {
				return nil, err
			}

			h.deleteTags("mfa:" + serial)

			return &DeleteVirtualMFADeviceResponse{
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
	}
}

// iamMFADeviceDispatch's "ListMFADevices" entry is shadowed by
// iamMFALinkDispatch's opListMFADevices (buildDispatchTable merges
// iamComprehensiveDispatchTable last) and never runs.
func (h *Handler) iamMFADeviceDispatch() map[string]iamActionFn {
	return map[string]iamActionFn{
		"ListMFADevices": func(vals url.Values, reqID string) (any, error) {
			userName := vals.Get("UserName")

			p, err := h.Backend.ListMFADevicesForUser(userName, vals.Get("Marker"), parseMaxItems(vals.Get("MaxItems")))
			if err != nil {
				// If user not found, return empty list (matches AWS behavior for optional UserName).
				p = page.Page[VirtualMFADevice]{}
			}

			members := make([]mfaDeviceXML, 0, len(p.Data))
			for _, d := range p.Data {
				members = append(members, mfaDeviceXML{
					UserName:     h.Backend.GetMFADeviceOwner(d.SerialNumber),
					SerialNumber: d.SerialNumber,
					EnableDate:   isoTime(d.CreateDate),
				})
			}

			return &listMFADevicesResponse{
				XMLName: xml.Name{Local: "ListMFADevicesResponse"},
				Xmlns:   iamXMLNS,
				ListMFADevicesResult: listMFADevicesResult{
					MFADevices:  members,
					Marker:      p.Next,
					IsTruncated: p.Next != "",
				},
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
		"ListMFADeviceTags": func(vals url.Values, reqID string) (any, error) {
			serial := vals.Get("SerialNumber")
			members := tagsMapToKV(h.getTags("mfa:" + serial))

			return &iamListTagsResponse{
				XMLName:          xml.Name{Local: "ListMFADeviceTagsResponse"},
				Xmlns:            iamXMLNS,
				Result:           iamListTagsResult{XMLName: xml.Name{Local: "ListMFADeviceTagsResult"}, Tags: members},
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
		"TagMFADevice": func(vals url.Values, reqID string) (any, error) {
			h.setTags("mfa:"+vals.Get("SerialNumber"), parseIAMTags(vals))

			return &iamSimpleTagResponse{
				XMLName:          xml.Name{Local: "TagMFADeviceResponse"},
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
		"UntagMFADevice": func(vals url.Values, reqID string) (any, error) {
			h.removeTags("mfa:"+vals.Get("SerialNumber"), parseIAMTagKeys(vals))

			return &iamSimpleTagResponse{
				XMLName:          xml.Name{Local: "UntagMFADeviceResponse"},
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
		"DeactivateMFADevice": func(vals url.Values, reqID string) (any, error) {
			if err := h.Backend.DeactivateMFADevice(vals.Get("UserName"), vals.Get("SerialNumber")); err != nil {
				return nil, err
			}

			return &iamSimpleTagResponse{
				XMLName:          xml.Name{Local: "DeactivateMFADeviceResponse"},
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
		"EnableMFADevice": func(vals url.Values, reqID string) (any, error) {
			if err := h.Backend.EnableMFADevice(
				vals.Get("UserName"), vals.Get("SerialNumber"),
				vals.Get("AuthenticationCode1"), vals.Get("AuthenticationCode2"),
			); err != nil {
				return nil, err
			}

			return &iamSimpleTagResponse{
				XMLName:          xml.Name{Local: "EnableMFADeviceResponse"},
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
		"ResyncMFADevice": func(vals url.Values, reqID string) (any, error) {
			if err := h.Backend.ResyncMFADevice(
				vals.Get("UserName"), vals.Get("SerialNumber"),
				vals.Get("AuthenticationCode1"), vals.Get("AuthenticationCode2"),
			); err != nil {
				return nil, err
			}

			// AWS returns no body fields for ResyncMFADevice.
			return &iamSimpleTagResponse{
				XMLName:          xml.Name{Local: "ResyncMFADeviceResponse"},
				Xmlns:            iamXMLNS,
				ResponseMetadata: ResponseMetadata{RequestID: reqID},
			}, nil
		},
	}
}
