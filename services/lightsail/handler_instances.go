package lightsail

import "context"

// instanceOps returns the dispatch table for family B (9 ops).
func (h *Handler) instanceOps() map[string]opFunc {
	return map[string]opFunc{
		"CreateInstances":             h.handleCreateInstances,
		"CreateInstancesFromSnapshot": h.handleCreateInstancesFromSnapshot,
		"DeleteInstance":              h.handleDeleteInstance,
		"GetInstance":                 h.handleGetInstance,
		"GetInstances":                h.handleGetInstances,
		"GetInstanceState":            h.handleGetInstanceState,
		"RebootInstance":              h.handleRebootInstance,
		"StartInstance":               h.handleStartInstance,
		"StopInstance":                h.handleStopInstance,
	}
}

// addOnRequestWire mirrors types.AddOnRequest.
type addOnRequestWire struct {
	AutoSnapshotAddOnRequest *struct {
		SnapshotTimeOfDay string `json:"snapshotTimeOfDay,omitempty"`
	} `json:"autoSnapshotAddOnRequest,omitempty"`
	StopInstanceOnIdleRequest *struct {
		Duration  string `json:"duration,omitempty"`
		Threshold string `json:"threshold,omitempty"`
	} `json:"stopInstanceOnIdleRequest,omitempty"`
	AddOnType string `json:"addOnType,omitempty"`
}

func addOnRequestsFromWire(in []addOnRequestWire) []AddOnRequest {
	out := make([]AddOnRequest, 0, len(in))

	for _, a := range in {
		req := AddOnRequest{Type: a.AddOnType}

		if a.AutoSnapshotAddOnRequest != nil {
			req.AutoSnapshotTimeOfDay = a.AutoSnapshotAddOnRequest.SnapshotTimeOfDay
		}

		if a.StopInstanceOnIdleRequest != nil {
			req.StopInstanceOnIdleDuration = a.StopInstanceOnIdleRequest.Duration
			req.StopInstanceOnIdleThreshold = a.StopInstanceOnIdleRequest.Threshold
		}

		out = append(out, req)
	}

	return out
}

// addOnWire mirrors types.AddOn.
type addOnWire struct {
	Duration              string `json:"duration,omitempty"`
	Name                  string `json:"name,omitempty"`
	NextSnapshotTimeOfDay string `json:"nextSnapshotTimeOfDay,omitempty"`
	SnapshotTimeOfDay     string `json:"snapshotTimeOfDay,omitempty"`
	Status                string `json:"status,omitempty"`
	Threshold             string `json:"threshold,omitempty"`
}

func addOnsToWire(in []AddOn) []addOnWire {
	if len(in) == 0 {
		return nil
	}

	out := make([]addOnWire, len(in))
	for i, a := range in {
		out[i] = addOnWire{
			Duration: a.Duration, Name: a.Name, NextSnapshotTimeOfDay: a.NextSnapshotTimeOfDay,
			SnapshotTimeOfDay: a.SnapshotTimeOfDay, Status: a.Status, Threshold: a.Threshold,
		}
	}

	return out
}

// instancePortInfoWire mirrors types.InstancePortInfo.
type instancePortInfoWire struct {
	AccessDirection string   `json:"accessDirection,omitempty"`
	AccessFrom      string   `json:"accessFrom,omitempty"`
	AccessType      string   `json:"accessType,omitempty"`
	CommonName      string   `json:"commonName,omitempty"`
	Protocol        string   `json:"protocol,omitempty"`
	CidrListAliases []string `json:"cidrListAliases,omitempty"`
	Cidrs           []string `json:"cidrs,omitempty"`
	Ipv6Cidrs       []string `json:"ipv6Cidrs,omitempty"`
	FromPort        int32    `json:"fromPort"`
	ToPort          int32    `json:"toPort"`
}

func portInfoFromWire(w instancePortInfoWire) InstancePortInfo {
	return InstancePortInfo{
		FromPort: w.FromPort, ToPort: w.ToPort, Protocol: w.Protocol, AccessFrom: w.AccessFrom,
		AccessType: w.AccessType, CommonName: w.CommonName, AccessDirection: w.AccessDirection,
		Cidrs: w.Cidrs, Ipv6Cidrs: w.Ipv6Cidrs, CidrListAliases: w.CidrListAliases,
	}
}

func portInfoToWire(p InstancePortInfo) instancePortInfoWire {
	return instancePortInfoWire{
		AccessDirection: p.AccessDirection, AccessFrom: p.AccessFrom, AccessType: p.AccessType,
		CidrListAliases: p.CidrListAliases, Cidrs: p.Cidrs, CommonName: p.CommonName,
		FromPort: p.FromPort, Ipv6Cidrs: p.Ipv6Cidrs, Protocol: p.Protocol, ToPort: p.ToPort,
	}
}

func portInfosToWire(in []InstancePortInfo) []instancePortInfoWire {
	if len(in) == 0 {
		return nil
	}

	out := make([]instancePortInfoWire, len(in))
	for i, p := range in {
		out[i] = portInfoToWire(p)
	}

	return out
}

// monthlyTransferWire mirrors types.MonthlyTransfer.
type monthlyTransferWire struct {
	GbPerMonthAllocated int32 `json:"gbPerMonthAllocated,omitempty"`
}

// instanceNetworkingWire mirrors types.InstanceNetworking.
type instanceNetworkingWire struct {
	MonthlyTransfer *monthlyTransferWire   `json:"monthlyTransfer,omitempty"`
	Ports           []instancePortInfoWire `json:"ports,omitempty"`
}

// instanceHardwareWire mirrors types.InstanceHardware.
type instanceHardwareWire struct {
	Disks       []diskWire `json:"disks,omitempty"`
	CPUCount    int32      `json:"cpuCount,omitempty"`
	RAMSizeInGb float32    `json:"ramSizeInGb,omitempty"`
}

// instanceMetadataOptionsWire mirrors types.InstanceMetadataOptions.
type instanceMetadataOptionsWire struct {
	HTTPEndpoint            string `json:"httpEndpoint,omitempty"`
	HTTPProtocolIpv6        string `json:"httpProtocolIpv6,omitempty"`
	HTTPTokens              string `json:"httpTokens,omitempty"`
	State                   string `json:"state,omitempty"`
	HTTPPutResponseHopLimit int32  `json:"httpPutResponseHopLimit,omitempty"`
}

// instanceWire mirrors types.Instance.
type instanceWire struct {
	Location         *resourceLocationWire        `json:"location,omitempty"`
	MetadataOptions  *instanceMetadataOptionsWire `json:"metadataOptions,omitempty"`
	State            *instanceStateWire           `json:"state,omitempty"`
	Networking       *instanceNetworkingWire      `json:"networking,omitempty"`
	Hardware         *instanceHardwareWire        `json:"hardware,omitempty"`
	CreatedAt        *float64                     `json:"createdAt,omitempty"`
	PrivateIPAddress string                       `json:"privateIpAddress,omitempty"`
	BlueprintName    string                       `json:"blueprintName,omitempty"`
	Username         string                       `json:"username,omitempty"`
	Name             string                       `json:"name,omitempty"`
	Arn              string                       `json:"arn,omitempty"`
	SupportCode      string                       `json:"supportCode,omitempty"`
	IPAddressType    string                       `json:"ipAddressType,omitempty"`
	BlueprintID      string                       `json:"blueprintId,omitempty"`
	PublicIPAddress  string                       `json:"publicIpAddress,omitempty"`
	BundleID         string                       `json:"bundleId,omitempty"`
	ResourceType     string                       `json:"resourceType,omitempty"`
	SSHKeyName       string                       `json:"sshKeyName,omitempty"`
	AddOns           []addOnWire                  `json:"addOns,omitempty"`
	Tags             []tagWire                    `json:"tags,omitempty"`
	Ipv6Addresses    []string                     `json:"ipv6Addresses,omitempty"`
	IsStaticIP       bool                         `json:"isStaticIp,omitempty"`
}

type instanceStateWire struct {
	Name string `json:"name,omitempty"`
	Code int32  `json:"code"`
}

func instanceToWire(i *Instance, disks []*Disk) instanceWire {
	diskWires := make([]diskWire, len(disks))
	for idx, d := range disks {
		diskWires[idx] = diskToWire(d)
	}

	return instanceWire{
		AddOns: addOnsToWire(i.AddOns), Arn: i.Arn, BlueprintID: i.BlueprintID, BlueprintName: i.BlueprintName,
		BundleID: i.BundleID, CreatedAt: epochPtr(i.CreatedAt),
		Hardware:      &instanceHardwareWire{CPUCount: i.CPUCount, RAMSizeInGb: i.RAMSizeInGb, Disks: diskWires},
		IPAddressType: i.IPAddressType, Ipv6Addresses: i.IPv6Addresses, IsStaticIP: i.IsStaticIP,
		Location: locationToWire(i.Location),
		MetadataOptions: &instanceMetadataOptionsWire{
			HTTPEndpoint:            i.MetadataOptions.HTTPEndpoint,
			HTTPProtocolIpv6:        i.MetadataOptions.HTTPProtocolIpv6,
			HTTPPutResponseHopLimit: i.MetadataOptions.HTTPPutResponseHopLimit,
			HTTPTokens:              i.MetadataOptions.HTTPTokens,
			State:                   i.MetadataOptions.State,
		},
		Name: i.Name,
		Networking: &instanceNetworkingWire{
			MonthlyTransfer: &monthlyTransferWire{GbPerMonthAllocated: i.MonthlyTransfer},
			Ports:           portInfosToWire(i.Ports),
		},
		PrivateIPAddress: i.PrivateIPAddress, PublicIPAddress: i.PublicIPAddress, ResourceType: ResourceTypeInstance,
		SSHKeyName: i.SSHKeyName, State: &instanceStateWire{Code: i.StateCode, Name: i.StateName},
		SupportCode: i.SupportCode, Tags: mapFromTags(i.Tags), Username: i.Username,
	}
}

type createInstancesRequestWire struct {
	AvailabilityZone string             `json:"availabilityZone,omitempty"`
	BlueprintID      string             `json:"blueprintId"`
	BundleID         string             `json:"bundleId"`
	CustomImageName  string             `json:"customImageName,omitempty"`
	IPAddressType    string             `json:"ipAddressType,omitempty"`
	KeyPairName      string             `json:"keyPairName,omitempty"`
	UserData         string             `json:"userData,omitempty"`
	InstanceNames    []string           `json:"instanceNames"`
	AddOns           []addOnRequestWire `json:"addOns,omitempty"`
	Tags             []tagWire          `json:"tags,omitempty"`
}

func (h *Handler) handleCreateInstances(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[createInstancesRequestWire](body)
	if err != nil {
		return nil, err
	}

	ops, createErr := h.Backend.CreateInstances(CreateInstancesRequest{
		Names: req.InstanceNames, AvailabilityZone: req.AvailabilityZone, BlueprintID: req.BlueprintID,
		BundleID: req.BundleID, KeyPairName: req.KeyPairName, IPAddressType: req.IPAddressType,
		AddOns: addOnRequestsFromWire(req.AddOns), Tags: tagsFromWire(req.Tags),
	})
	if createErr != nil {
		return nil, createErr
	}

	return marshalResponse(opsEnvelope(ops))
}

type createInstancesFromSnapshotRequestWire struct {
	AttachedDiskMapping map[string][]struct {
		NewDiskName      string `json:"newDiskName,omitempty"`
		OriginalDiskPath string `json:"originalDiskPath,omitempty"`
	} `json:"attachedDiskMapping,omitempty"`
	UserData                        string             `json:"userData,omitempty"`
	AvailabilityZone                string             `json:"availabilityZone,omitempty"`
	BundleID                        string             `json:"bundleId"`
	InstanceSnapshotName            string             `json:"instanceSnapshotName,omitempty"`
	IPAddressType                   string             `json:"ipAddressType,omitempty"`
	KeyPairName                     string             `json:"keyPairName,omitempty"`
	RestoreDate                     string             `json:"restoreDate,omitempty"`
	SourceInstanceName              string             `json:"sourceInstanceName,omitempty"`
	InstanceNames                   []string           `json:"instanceNames"`
	AddOns                          []addOnRequestWire `json:"addOns,omitempty"`
	Tags                            []tagWire          `json:"tags,omitempty"`
	UseLatestRestorableAutoSnapshot bool               `json:"useLatestRestorableAutoSnapshot,omitempty"`
}

func (h *Handler) handleCreateInstancesFromSnapshot(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[createInstancesFromSnapshotRequestWire](body)
	if err != nil {
		return nil, err
	}

	ops, createErr := h.Backend.CreateInstancesFromSnapshot(CreateInstancesFromSnapshotRequest{
		InstanceNames: req.InstanceNames, AvailabilityZone: req.AvailabilityZone, BundleID: req.BundleID,
		InstanceSnapshotName: req.InstanceSnapshotName, KeyPairName: req.KeyPairName, IPAddressType: req.IPAddressType,
		AddOns: addOnRequestsFromWire(req.AddOns), Tags: tagsFromWire(req.Tags),
	})
	if createErr != nil {
		return nil, createErr
	}

	return marshalResponse(opsEnvelope(ops))
}

type instanceNameRequest struct {
	InstanceName string `json:"instanceName"`
}

type deleteInstanceRequest struct {
	InstanceName      string `json:"instanceName"`
	ForceDeleteAddOns bool   `json:"forceDeleteAddOns,omitempty"`
}

func (h *Handler) handleDeleteInstance(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[deleteInstanceRequest](body)
	if err != nil {
		return nil, err
	}

	ops, delErr := h.Backend.DeleteInstance(req.InstanceName)
	if delErr != nil {
		return nil, delErr
	}

	return marshalResponse(opsEnvelope(ops))
}

type instanceEnvelope struct {
	Instance *instanceWire `json:"instance,omitempty"`
}

func (h *Handler) handleGetInstance(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[instanceNameRequest](body)
	if err != nil {
		return nil, err
	}

	inst, getErr := h.Backend.GetInstance(req.InstanceName)
	if getErr != nil {
		return nil, getErr
	}

	w := instanceToWire(inst, h.Backend.DisksAttachedTo(inst.Name))

	return marshalResponse(instanceEnvelope{Instance: &w})
}

type instancesListResponse struct {
	NextPageToken string         `json:"nextPageToken,omitempty"`
	Instances     []instanceWire `json:"instances,omitempty"`
}

func (h *Handler) handleGetInstances(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[pageTokenRequest](body)
	if err != nil {
		return nil, err
	}

	pg, pgErr := h.Backend.GetInstances(req.PageToken)
	if pgErr != nil {
		return nil, pgErr
	}

	out := make([]instanceWire, len(pg.Data))
	for i, inst := range pg.Data {
		out[i] = instanceToWire(inst, h.Backend.DisksAttachedTo(inst.Name))
	}

	return marshalResponse(instancesListResponse{Instances: out, NextPageToken: pg.Next})
}

type instanceStateResponse struct {
	State *instanceStateWire `json:"state,omitempty"`
}

func (h *Handler) handleGetInstanceState(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[instanceNameRequest](body)
	if err != nil {
		return nil, err
	}

	code, name, getErr := h.Backend.GetInstanceState(req.InstanceName)
	if getErr != nil {
		return nil, getErr
	}

	return marshalResponse(instanceStateResponse{State: &instanceStateWire{Code: code, Name: name}})
}

func (h *Handler) handleRebootInstance(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[instanceNameRequest](body)
	if err != nil {
		return nil, err
	}

	ops, rebootErr := h.Backend.RebootInstance(req.InstanceName)
	if rebootErr != nil {
		return nil, rebootErr
	}

	return marshalResponse(opsEnvelope(ops))
}

func (h *Handler) handleStartInstance(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[instanceNameRequest](body)
	if err != nil {
		return nil, err
	}

	ops, startErr := h.Backend.StartInstance(req.InstanceName)
	if startErr != nil {
		return nil, startErr
	}

	return marshalResponse(opsEnvelope(ops))
}

type stopInstanceRequest struct {
	InstanceName string `json:"instanceName"`
	Force        bool   `json:"force,omitempty"`
}

func (h *Handler) handleStopInstance(_ context.Context, body []byte) ([]byte, error) {
	req, err := decodeBody[stopInstanceRequest](body)
	if err != nil {
		return nil, err
	}

	ops, stopErr := h.Backend.StopInstance(req.InstanceName)
	if stopErr != nil {
		return nil, stopErr
	}

	return marshalResponse(opsEnvelope(ops))
}
