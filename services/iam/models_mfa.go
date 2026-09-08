package iam

import (
	"encoding/xml"
	"time"
)

// MFA device status constants mirror AWS IAM virtual MFA device states.
const (
	MFAStatusNotAssigned = "not_assigned" // PENDING_ENABLE / unlinked
	MFAStatusEnabled     = "Active"       // linked via EnableMFADevice
	MFAStatusDeactivated = "Deactivated"  // unlinked via DeactivateMFADevice
)

// VirtualMFADevice represents an IAM virtual MFA device.
type VirtualMFADevice struct {
	CreateDate           time.Time `json:"CreateDate"`
	SerialNumber         string    `json:"SerialNumber,omitempty"`
	VirtualMFADeviceName string    `json:"VirtualMFADeviceName,omitempty"`
	Path                 string    `json:"Path,omitempty"`
	// Status tracks the MFA device lifecycle: not_assigned → Active → Deactivated.
	Status           string `json:"Status,omitempty"`
	Base32StringSeed string `json:"Base32StringSeed,omitempty"`
	QRCodePNG        string `json:"QRCodePNG,omitempty"`
}

// VirtualMFADeviceXML is the XML representation of a virtual MFA device.
type VirtualMFADeviceXML struct {
	User             *UserXML `xml:"User,omitempty"`
	SerialNumber     string   `xml:"SerialNumber"`
	Base32StringSeed string   `xml:"Base32StringSeed,omitempty"`
	QRCodePNG        string   `xml:"QRCodePNG,omitempty"`
	Tags             []TagXML `xml:"Tags>member,omitempty"`
}

// CreateVirtualMFADeviceResult wraps the created MFA device.
type CreateVirtualMFADeviceResult struct {
	VirtualMFADevice VirtualMFADeviceXML `xml:"VirtualMFADevice"`
}

// CreateVirtualMFADeviceResponse is the XML response for CreateVirtualMFADevice.
type CreateVirtualMFADeviceResponse struct {
	XMLName                      xml.Name                     `xml:"CreateVirtualMFADeviceResponse"`
	Xmlns                        string                       `xml:"xmlns,attr"`
	ResponseMetadata             ResponseMetadata             `xml:"ResponseMetadata"`
	CreateVirtualMFADeviceResult CreateVirtualMFADeviceResult `xml:"CreateVirtualMFADeviceResult"`
}

// ListVirtualMFADevicesResult contains the list of virtual MFA devices.
type ListVirtualMFADevicesResult struct {
	VirtualMFADevices []VirtualMFADeviceXML `xml:"VirtualMFADevices>member"`
	IsTruncated       bool                  `xml:"IsTruncated"`
}

// ListVirtualMFADevicesResponse is the XML response for ListVirtualMFADevices.
type ListVirtualMFADevicesResponse struct {
	XMLName                     xml.Name                    `xml:"ListVirtualMFADevicesResponse"`
	Xmlns                       string                      `xml:"xmlns,attr"`
	ResponseMetadata            ResponseMetadata            `xml:"ResponseMetadata"`
	ListVirtualMFADevicesResult ListVirtualMFADevicesResult `xml:"ListVirtualMFADevicesResult"`
}

// DeleteVirtualMFADeviceResponse is the XML response for DeleteVirtualMFADevice.
type DeleteVirtualMFADeviceResponse struct {
	XMLName          xml.Name         `xml:"DeleteVirtualMFADeviceResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// mfaDeviceXML is the XML representation of an MFA device entry in ListMFADevices.
type mfaDeviceXML struct {
	UserName     string `xml:"UserName"`
	SerialNumber string `xml:"SerialNumber"`
	EnableDate   string `xml:"EnableDate"`
}

// listMFADevicesResult contains the list of MFA devices.
type listMFADevicesResult struct {
	Marker      string         `xml:"Marker,omitempty"`
	MFADevices  []mfaDeviceXML `xml:"MFADevices>member"`
	IsTruncated bool           `xml:"IsTruncated"`
}

// listMFADevicesResponse is the XML response for ListMFADevices.
type listMFADevicesResponse struct {
	XMLName              xml.Name             `xml:"ListMFADevicesResponse"`
	Xmlns                string               `xml:"xmlns,attr"`
	ResponseMetadata     ResponseMetadata     `xml:"ResponseMetadata"`
	ListMFADevicesResult listMFADevicesResult `xml:"ListMFADevicesResult"`
}

// GetMFADeviceResult contains the details of a single MFA device.
type GetMFADeviceResult struct {
	UserName     string `xml:"UserName,omitempty"`
	SerialNumber string `xml:"SerialNumber"`
	EnableDate   string `xml:"EnableDate"`
}

// GetMFADeviceResponse is the XML response for GetMFADevice.
type GetMFADeviceResponse struct {
	XMLName            xml.Name           `xml:"GetMFADeviceResponse"`
	Xmlns              string             `xml:"xmlns,attr"`
	GetMFADeviceResult GetMFADeviceResult `xml:"GetMFADeviceResult"`
	ResponseMetadata   ResponseMetadata   `xml:"ResponseMetadata"`
}
