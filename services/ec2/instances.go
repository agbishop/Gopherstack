package ec2

import (
	"encoding/base64"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// privateIPOctetRange bounds the third octet used by allocPrivateIP.
const privateIPOctetRange = 253

// allocPrivateIP returns the next 172.31.x.y private IP. Must be called with mu held.
// Recycled IPs from UnassignPrivateIPAddresses are reused before allocating fresh ones.
func (b *InMemoryBackend) allocPrivateIP() string {
	if len(b.freePrivateIPs) > 0 {
		last := len(b.freePrivateIPs) - 1
		ip := b.freePrivateIPs[last]
		b.freePrivateIPs = b.freePrivateIPs[:last]

		return ip
	}

	idx := b.nextPrivateIPIndex
	b.nextPrivateIPIndex++
	third := idx / privateIPOctetRange
	fourth := idx%privateIPOctetRange + 1

	return fmt.Sprintf("172.31.%d.%d", third, fourth)
}

// StartInstances transitions stopped instances to running.
// Returns ErrInvalidInstanceState if any instance is not in the stopped state.
func (b *InMemoryBackend) StartInstances(ids []string) ([]*InstanceStateChange, error) {
	b.mu.Lock("StartInstances")
	defer b.mu.Unlock()

	var result []*InstanceStateChange

	for _, id := range ids {
		inst, ok := b.instances.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
		}

		if inst.State != StateStopped {
			return nil, fmt.Errorf(
				"%w: instance %s is in state %s, expected stopped",
				ErrInvalidInstanceState,
				id,
				inst.State.Name,
			)
		}

		prev := inst.State
		// AWS state machine: stopped → pending → running (reconciler advances pending→running).
		inst.State = StatePending
		// AWS clears the state reason once the instance starts back up.
		inst.StateReasonCode = ""
		inst.StateReasonMessage = ""
		inst.StateTransitionReason = ""
		result = append(result, &InstanceStateChange{
			InstanceID:    id,
			PreviousState: prev,
			CurrentState:  inst.State,
		})
	}

	return result, nil
}

// StopInstances transitions running instances to stopped.
// Returns ErrInvalidInstanceState if any instance is not in the running state.
func (b *InMemoryBackend) StopInstances(ids []string) ([]*InstanceStateChange, error) {
	b.mu.Lock("StopInstances")
	defer b.mu.Unlock()

	var result []*InstanceStateChange

	for _, id := range ids {
		inst, ok := b.instances.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
		}

		// AWS allows stopping instances in running or pending states.
		if inst.State != StateRunning && inst.State != StatePending {
			return nil, fmt.Errorf(
				"%w: instance %s is in state %s, cannot stop",
				ErrInvalidInstanceState,
				id,
				inst.State.Name,
			)
		}

		if inst.DisableAPIStop {
			return nil, fmt.Errorf(
				"%w: the instance %s may not be stopped. "+
					"Modify its 'disableApiStop' instance attribute and try again",
				ErrOperationNotPermitted, id)
		}

		prev := inst.State
		// AWS state machine: running/pending → stopping → stopped (reconciler advances stopping→stopped).
		inst.State = StateStopping
		inst.StateReasonCode = "Client.UserInitiatedShutdown"
		inst.StateReasonMessage = "Client.UserInitiatedShutdown: User initiated shutdown"
		inst.StateTransitionReason = fmt.Sprintf(
			"User initiated (%s)", time.Now().UTC().Format("2006-01-02 15:04:05 GMT"),
		)
		result = append(result, &InstanceStateChange{
			InstanceID:    id,
			PreviousState: prev,
			CurrentState:  inst.State,
		})
	}

	return result, nil
}

// RebootInstances validates that all given instances exist (no state change).
func (b *InMemoryBackend) RebootInstances(ids []string) error {
	b.mu.RLock("RebootInstances")
	defer b.mu.RUnlock()

	for _, id := range ids {
		if _, ok := b.instances.Get(id); !ok {
			return fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
		}
	}

	return nil
}

// DescribeInstanceStatus returns instances for status reporting.
func (b *InMemoryBackend) DescribeInstanceStatus(ids []string) []*Instance {
	b.mu.RLock("DescribeInstanceStatus")
	defer b.mu.RUnlock()

	var out []*Instance
	if len(ids) > 0 {
		for _, id := range ids {
			inst, ok := b.instances.Get(id)
			if !ok {
				continue
			}

			cp := *inst
			out = append(out, &cp)
		}

		return out
	}

	for _, inst := range b.instances.All() {
		cp := *inst
		out = append(out, &cp)
	}

	return out
}

// GetConsoleOutput returns a base64-encoded console output for an instance.
// Instances in this mock don't generate real console output; returns empty string.
func (b *InMemoryBackend) GetConsoleOutput(instanceID string) (string, time.Time, error) {
	if instanceID == "" {
		return "", time.Time{}, fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	b.mu.RLock("GetConsoleOutput")
	defer b.mu.RUnlock()

	if _, ok := b.instances.Get(instanceID); !ok {
		return "", time.Time{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	output := base64.StdEncoding.EncodeToString([]byte("[ EC2 mock console ]\n"))

	return output, time.Now().UTC(), nil
}

// ---- ModifyInstanceMetadataOptions ----

// IMDSOptions holds per-instance IMDS configuration.
type IMDSOptions struct {
	HTTPTokens              string `json:"httpTokens,omitempty"`
	HTTPEndpoint            string `json:"httpEndpoint,omitempty"`
	InstanceMetadataTags    string `json:"instanceMetadataTags,omitempty"`
	State                   string `json:"state,omitempty"`
	HTTPPutResponseHopLimit int    `json:"httpPutResponseHopLimit,omitempty"`
}

// ModifyInstanceMetadataOptions updates IMDS settings for an instance.
func (b *InMemoryBackend) ModifyInstanceMetadataOptions(
	instanceID, httpTokens, httpEndpoint, instanceMetadataTags string,
	hopLimit int,
) (*IMDSOptions, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyInstanceMetadataOptions")
	defer b.mu.Unlock()

	inst, ok := b.instances.Get(instanceID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	// Persist tokens setting on instance for DescribeInstances accuracy.
	if httpTokens != "" {
		inst.MetadataOptionsTokens = httpTokens
	}
	if httpEndpoint != "" {
		inst.MetadataOptionsState = httpEndpoint
	}

	opts := &IMDSOptions{
		HTTPTokens:              inst.MetadataOptionsTokens,
		HTTPEndpoint:            inst.MetadataOptionsState,
		HTTPPutResponseHopLimit: hopLimit,
		InstanceMetadataTags:    instanceMetadataTags,
		State:                   "applied",
	}
	if opts.HTTPTokens == "" {
		opts.HTTPTokens = "optional"
	}
	if opts.HTTPEndpoint == "" {
		opts.HTTPEndpoint = "enabled"
	}
	if opts.HTTPPutResponseHopLimit == 0 {
		opts.HTTPPutResponseHopLimit = 1
	}

	return opts, nil
}

// ---- InstanceMetadata account defaults ----

// GetInstanceMetadataDefaults returns account-level IMDS defaults.
func (b *InMemoryBackend) GetInstanceMetadataDefaults() *InstanceMetadataDefaults {
	b.mu.RLock("GetInstanceMetadataDefaults")
	defer b.mu.RUnlock()

	if b.instanceMetadataDefaults == nil {
		return &InstanceMetadataDefaults{
			HTTPTokens:              "no-preference",
			HTTPEndpoint:            "enabled",
			HTTPPutResponseHopLimit: 0,
			InstanceMetadataTags:    "no-preference",
		}
	}
	cp := *b.instanceMetadataDefaults

	return &cp
}

// ModifyInstanceMetadataDefaults updates account-level IMDS defaults.
func (b *InMemoryBackend) ModifyInstanceMetadataDefaults(
	httpTokens, httpEndpoint, instanceMetadataTags string,
	hopLimit int,
) error {
	b.mu.Lock("ModifyInstanceMetadataDefaults")
	defer b.mu.Unlock()

	d := &InstanceMetadataDefaults{}
	if b.instanceMetadataDefaults != nil {
		*d = *b.instanceMetadataDefaults
	}
	if httpTokens != "" {
		d.HTTPTokens = httpTokens
	}
	if httpEndpoint != "" {
		d.HTTPEndpoint = httpEndpoint
	}
	if instanceMetadataTags != "" {
		d.InstanceMetadataTags = instanceMetadataTags
	}
	if hopLimit > 0 {
		d.HTTPPutResponseHopLimit = hopLimit
	}
	b.instanceMetadataDefaults = d

	return nil
}

// ---- Instance credit specifications ----

// InstanceCreditSpec holds the CPU credit configuration for burstable instances.
type InstanceCreditSpec struct {
	InstanceID string `json:"instanceID,omitempty"`
	CPUCredits string `json:"cpuCredits,omitempty"` // "standard" or "unlimited"
}

// DescribeInstanceCreditSpecifications returns CPU credit specs for the given instances.
func (b *InMemoryBackend) DescribeInstanceCreditSpecifications(ids []string) []InstanceCreditSpec {
	b.mu.RLock("DescribeInstanceCreditSpecifications")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	var out []InstanceCreditSpec
	for _, inst := range b.instances.All() {
		if len(filter) > 0 && !filter[inst.ID] {
			continue
		}
		credits := b.instanceCreditSpecs[inst.ID]
		if credits == "" {
			// Default: burstable types use "standard", others return nothing in real AWS
			// but we return "standard" to keep responses non-empty.
			credits = "standard"
		}
		out = append(out, InstanceCreditSpec{InstanceID: inst.ID, CPUCredits: credits})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceID < out[j].InstanceID })

	return out
}

// ModifyInstanceCreditSpecification updates the CPU credit spec for each given
// instance, reporting per-instance success/failure rather than aborting the
// whole batch on one bad ID (ec2@v1.319.1 serializers.go:87727,
// InstanceCreditSpecifications is a required, unbounded list serialized as
// flat InstanceCreditSpecification.N; ModifyInstanceCreditSpecificationOutput
// reports per-item results via Successful/UnsuccessfulInstanceCreditSpecifications).
func (b *InMemoryBackend) ModifyInstanceCreditSpecification(
	specs []InstanceCreditSpec,
) ([]InstanceCreditSpec, []InstanceCreditSpec) {
	b.mu.Lock("ModifyInstanceCreditSpecification")
	defer b.mu.Unlock()

	var successful, unsuccessful []InstanceCreditSpec

	for _, spec := range specs {
		if _, ok := b.instances.Get(spec.InstanceID); !ok {
			unsuccessful = append(unsuccessful, spec)

			continue
		}
		b.instanceCreditSpecs[spec.InstanceID] = spec.CPUCredits
		successful = append(successful, spec)
	}

	return successful, unsuccessful
}

// ---- DescribeInstanceTopology ----

// InstanceTopologyItem holds topology info for an instance.
type InstanceTopologyItem struct {
	InstanceID       string   `json:"instanceID,omitempty"`
	InstanceType     string   `json:"instanceType,omitempty"`
	GroupName        string   `json:"groupName,omitempty"`
	AvailabilityZone string   `json:"availabilityZone,omitempty"`
	ZoneID           string   `json:"zoneID,omitempty"`
	NetworkNodes     []string `json:"networkNodes,omitempty"`
}

// DescribeInstanceTopology returns placement topology info for instances.
func (b *InMemoryBackend) DescribeInstanceTopology(ids []string) []InstanceTopologyItem {
	b.mu.RLock("DescribeInstanceTopology")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	var out []InstanceTopologyItem
	for _, inst := range b.instances.All() {
		if len(filter) > 0 && !filter[inst.ID] {
			continue
		}
		az := b.Region + "a"
		if sub, ok := b.subnets.Get(inst.SubnetID); ok && sub.AvailabilityZone != "" {
			az = sub.AvailabilityZone
		}
		out = append(out, InstanceTopologyItem{
			InstanceID:       inst.ID,
			InstanceType:     inst.InstanceType,
			GroupName:        inst.Placement.GroupName,
			AvailabilityZone: az,
			ZoneID:           az + "1",
			NetworkNodes:     []string{"nn-" + inst.ID[:8]},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceID < out[j].InstanceID })

	return out
}

// ---- MonitorInstances / UnmonitorInstances ----

// MonitoringState holds the monitoring state for an instance.
type MonitoringState struct {
	InstanceID string `json:"instanceID,omitempty"`
	State      string `json:"state,omitempty"` // "pending" | "enabled" | "disabling" | "disabled"
}

// MonitorInstances enables detailed monitoring for the given instances.
func (b *InMemoryBackend) MonitorInstances(instanceIDs []string) ([]MonitoringState, error) {
	if len(instanceIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one InstanceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("MonitorInstances")
	defer b.mu.Unlock()

	var out []MonitoringState
	for _, id := range instanceIDs {
		inst, ok := b.instances.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
		}
		inst.MonitoringState = stateMonitoringEnabled
		out = append(out, MonitoringState{InstanceID: id, State: stateMonitoringEnabled})
	}

	return out, nil
}

// UnmonitorInstances disables detailed monitoring for the given instances.
func (b *InMemoryBackend) UnmonitorInstances(instanceIDs []string) ([]MonitoringState, error) {
	if len(instanceIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one InstanceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("UnmonitorInstances")
	defer b.mu.Unlock()

	var out []MonitoringState
	for _, id := range instanceIDs {
		inst, ok := b.instances.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
		}
		inst.MonitoringState = stateMonitoringDisabled
		out = append(out, MonitoringState{InstanceID: id, State: stateMonitoringDisabled})
	}

	return out, nil
}

// ---- Network Interface attribute ----

// NIAttributeResult holds the result of DescribeNetworkInterfaceAttribute.
type NIAttributeResult struct {
	NetworkInterfaceID string   `json:"networkInterfaceID,omitempty"`
	Description        string   `json:"description,omitempty"`
	AttachmentID       string   `json:"attachmentID,omitempty"`
	AttachInstanceID   string   `json:"attachInstanceID,omitempty"`
	AttachStatus       string   `json:"attachStatus,omitempty"`
	GroupIDs           []string `json:"groupIDs,omitempty"`
	AttachDeviceIndex  int      `json:"attachDeviceIndex,omitempty"`
	SourceDestCheck    bool     `json:"sourceDestCheck,omitempty"`
	AttachDeleteOnTerm bool     `json:"attachDeleteOnTermination,omitempty"`
	HasAttachment      bool     `json:"hasAttachment,omitempty"`
}

// EnableSerialConsoleAccess enables serial console access for the account.
func (b *InMemoryBackend) EnableSerialConsoleAccess() {
	b.mu.Lock("EnableSerialConsoleAccess")
	defer b.mu.Unlock()

	b.serialConsoleAccess = true
}

// DisableSerialConsoleAccess disables serial console access for the account.
func (b *InMemoryBackend) DisableSerialConsoleAccess() {
	b.mu.Lock("DisableSerialConsoleAccess")
	defer b.mu.Unlock()

	b.serialConsoleAccess = false
}

// GetSerialConsoleAccessStatus returns whether serial console access is enabled.
func (b *InMemoryBackend) GetSerialConsoleAccessStatus() bool {
	b.mu.RLock("GetSerialConsoleAccessStatus")
	defer b.mu.RUnlock()

	return b.serialConsoleAccess
}

// ---- VGW route propagation ----

// GetDefaultCreditSpecification returns the account-level default CPU credit spec.
func (b *InMemoryBackend) GetDefaultCreditSpecification() string {
	b.mu.RLock("GetDefaultCreditSpecification")
	defer b.mu.RUnlock()

	if b.defaultCreditSpec == "" {
		return stateDefaultCredit
	}

	return b.defaultCreditSpec
}

// ModifyDefaultCreditSpecification sets the account-level default CPU credit spec.
func (b *InMemoryBackend) ModifyDefaultCreditSpecification(cpuCredits string) error {
	if cpuCredits == "" {
		return fmt.Errorf("%w: CpuCredits is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyDefaultCreditSpecification")
	defer b.mu.Unlock()

	b.defaultCreditSpec = cpuCredits

	return nil
}

// ---- Replace root volume tasks ----

// CreateInstanceConnectEndpoint creates a new Instance Connect Endpoint.
func (b *InMemoryBackend) CreateInstanceConnectEndpoint(
	subnetID string,
	securityGroupIDs []string,
	preserveClientIP bool,
) (*InstanceConnectEndpoint, error) {
	if subnetID == "" {
		return nil, fmt.Errorf("%w: SubnetId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateInstanceConnectEndpoint")
	defer b.mu.Unlock()

	subnet, ok := b.subnets.Get(subnetID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	id := "eice-" + uuid.New().String()[:8]
	ep := &InstanceConnectEndpoint{
		InstanceConnectEndpointID:  id,
		InstanceConnectEndpointARN: "arn:aws:ec2:" + b.Region + ":" + b.AccountID + ":instance-connect-endpoint/" + id,
		SubnetID:                   subnetID,
		VPCID:                      subnet.VPCID,
		SecurityGroupIDs:           securityGroupIDs,
		State:                      stateActive,
		PreserveClientIP:           preserveClientIP,
		CreateTime:                 time.Now().UTC(),
	}
	b.instanceConnectEndpoints.Put(ep)

	return ep, nil
}

// DeleteInstanceConnectEndpoint removes an Instance Connect Endpoint.
func (b *InMemoryBackend) DeleteInstanceConnectEndpoint(id string) (*InstanceConnectEndpoint, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: InstanceConnectEndpointId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteInstanceConnectEndpoint")
	defer b.mu.Unlock()

	ep, ok := b.instanceConnectEndpoints.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInstanceConnectEndpointNotFound, id)
	}
	cp := *ep
	cp.State = "delete-complete"
	b.instanceConnectEndpoints.Delete(id)
	delete(b.tags, id)

	return &cp, nil
}

// DescribeInstanceConnectEndpoints returns endpoints, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeInstanceConnectEndpoints(
	ids []string,
) []*InstanceConnectEndpoint {
	b.mu.RLock("DescribeInstanceConnectEndpoints")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	var out []*InstanceConnectEndpoint
	for _, ep := range b.instanceConnectEndpoints.All() {
		if len(filter) > 0 && !filter[ep.InstanceConnectEndpointID] {
			continue
		}
		cp := *ep
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].InstanceConnectEndpointID < out[j].InstanceConnectEndpointID
	})

	return out
}

// ModifyInstanceConnectEndpoint modifies preserveClientIP for an endpoint.
func (b *InMemoryBackend) ModifyInstanceConnectEndpoint(id string, preserveClientIP bool) error {
	if id == "" {
		return fmt.Errorf("%w: InstanceConnectEndpointId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyInstanceConnectEndpoint")
	defer b.mu.Unlock()

	ep, ok := b.instanceConnectEndpoints.Get(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInstanceConnectEndpointNotFound, id)
	}
	ep.PreserveClientIP = preserveClientIP

	return nil
}

// ---- Instance Event Windows ----

// CreateInstanceEventWindow creates a maintenance event window.
func (b *InMemoryBackend) CreateInstanceEventWindow(
	name, cronExpression string,
) (*InstanceEventWindow, error) {
	b.mu.Lock("CreateInstanceEventWindow")
	defer b.mu.Unlock()

	ew := &InstanceEventWindow{
		InstanceEventWindowID: "iew-" + uuid.New().String()[:8],
		Name:                  name,
		CronExpression:        cronExpression,
		State:                 stateActive,
	}
	b.instanceEventWindows.Put(ew)

	return ew, nil
}

// DeleteInstanceEventWindow removes an event window.
func (b *InMemoryBackend) DeleteInstanceEventWindow(id string) error {
	if id == "" {
		return fmt.Errorf("%w: InstanceEventWindowId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteInstanceEventWindow")
	defer b.mu.Unlock()

	if _, ok := b.instanceEventWindows.Get(id); !ok {
		return fmt.Errorf("%w: %s", ErrInstanceEventWindowNotFound, id)
	}
	b.instanceEventWindows.Delete(id)
	delete(b.tags, id)

	return nil
}

// DescribeInstanceEventWindows returns event windows, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeInstanceEventWindows(ids []string) []*InstanceEventWindow {
	b.mu.RLock("DescribeInstanceEventWindows")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	var out []*InstanceEventWindow
	for _, ew := range b.instanceEventWindows.All() {
		if len(filter) > 0 && !filter[ew.InstanceEventWindowID] {
			continue
		}
		cp := *ew
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].InstanceEventWindowID < out[j].InstanceEventWindowID
	})

	return out
}

// ModifyInstanceEventWindow updates the cron expression for an event window.
func (b *InMemoryBackend) ModifyInstanceEventWindow(id, name, cronExpression string) (*InstanceEventWindow, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: InstanceEventWindowId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyInstanceEventWindow")
	defer b.mu.Unlock()

	ew, ok := b.instanceEventWindows.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInstanceEventWindowNotFound, id)
	}
	if name != "" {
		ew.Name = name
	}
	if cronExpression != "" {
		ew.CronExpression = cronExpression
	}

	cp := *ew

	return &cp, nil
}

// ---- Spot Datafeed ----

// GetPasswordData returns the password data for a Windows instance (empty in mock).
func (b *InMemoryBackend) GetPasswordData(instanceID string) (string, time.Time, error) {
	if instanceID == "" {
		return "", time.Time{}, fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	b.mu.RLock("GetPasswordData")
	defer b.mu.RUnlock()

	if _, ok := b.instances.Get(instanceID); !ok {
		return "", time.Time{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	return "", time.Now().UTC(), nil
}

// ---- GetConsoleScreenshot ----

// GetConsoleScreenshot returns an empty base64-encoded console screenshot.
func (b *InMemoryBackend) GetConsoleScreenshot(instanceID string) (string, error) {
	if instanceID == "" {
		return "", fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	b.mu.RLock("GetConsoleScreenshot")
	defer b.mu.RUnlock()

	if _, ok := b.instances.Get(instanceID); !ok {
		return "", fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	return "", nil
}

// ---- GetInstanceTypesFromInstanceRequirements ----

// GetInstanceTypesFromInstanceRequirements returns a static list of instance types
// matching the given requirements (simplified mock).
func (b *InMemoryBackend) GetInstanceTypesFromInstanceRequirements() []string {
	return []string{
		instanceTypeT3Micro, instanceTypeT3Small, instanceTypeT3Medium, "m5.large",
		instanceTypeM5Xlarge,
	}
}

// ---- GetSubnetCidrReservations ----

// ReportInstanceStatus records a custom status for the given instances. Real
// AWS validates every listed instance, not just the first (ec2@v1.319.1
// serializers.go:91277, ReportInstanceStatusInput.Instances is a required,
// unbounded list serialized as flat InstanceId.N).
func (b *InMemoryBackend) ReportInstanceStatus(instanceIDs []string, _ string, _ string) error {
	if len(instanceIDs) == 0 {
		return fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	b.mu.RLock("ReportInstanceStatus")
	defer b.mu.RUnlock()

	for _, instanceID := range instanceIDs {
		if _, ok := b.instances.Get(instanceID); !ok {
			return fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
		}
	}

	return nil
}

// ---- VPN operations ----

// DescribeInstancesByVPC returns running instances in a given VPC using the secondary index.
func (b *InMemoryBackend) DescribeInstancesByVPC(vpcID string) []*Instance {
	b.mu.RLock("DescribeInstancesByVPC")
	defer b.mu.RUnlock()

	ids, ok := b.instanceIDsByVPC[vpcID]
	if !ok {
		return nil
	}

	out := make([]*Instance, 0, len(ids))

	for id := range ids {
		if inst, exists := b.instances.Get(id); exists {
			cp := *inst
			out = append(out, &cp)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

// DescribeInstanceTypeOfferings returns a static list of instance type / AZ pairs.
func (b *InMemoryBackend) DescribeInstanceTypeOfferings() []InstanceTypeOffering {
	azs := b.DescribeAvailabilityZones(b.Region)
	types := []string{
		"t3.nano", instanceTypeT3Micro, "t3.small", instanceTypeT3Medium, "t3.large",
		"t3.xlarge", "t3.2xlarge",
		"m5.large", instanceTypeM5Xlarge, "m5.2xlarge", "m5.4xlarge",
		"c5.large", "c5.xlarge", "c5.2xlarge",
		"r5.large", "r5.xlarge",
		"p3.2xlarge",
		"inf1.xlarge",
	}

	out := make([]InstanceTypeOffering, 0, len(types)*len(azs))

	for _, t := range types {
		for _, az := range azs {
			out = append(out, InstanceTypeOffering{
				InstanceType:     t,
				AvailabilityZone: az,
				Location:         az,
				LocationType:     "availability-zone",
			})
		}
	}

	return out
}

// ---- VPC peering: create / delete (accept is in accept_ops.go) ----

// SendDiagnosticInterrupt validates that the instance exists. Real AWS sends
// a diagnostic NMI/QMP interrupt to the underlying hypervisor guest, which
// has no observable effect this backend can emulate beyond the existence
// check every real call also performs.
func (b *InMemoryBackend) SendDiagnosticInterrupt(instanceID string) error {
	if instanceID == "" {
		return fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	b.mu.RLock("SendDiagnosticInterrupt")
	defer b.mu.RUnlock()

	if _, ok := b.instances.Get(instanceID); !ok {
		return fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	return nil
}

// DescribeElasticGpus returns Elastic Graphics accelerators. No API in this
// backend attaches Elastic Graphics accelerators to instances (AWS retired
// the feature in 2023), so this always returns an empty, correctly shaped
// list — the same "no API populates this" precedent already established for
// ClientVpnConnection elsewhere in this package.
func (b *InMemoryBackend) DescribeElasticGpus(_ []string) []*ElasticGpuStub {
	return []*ElasticGpuStub{}
}

// ElasticGpuStub is the wire shape for DescribeElasticGpus entries. No API in
// this backend attaches Elastic Graphics accelerators to instances (AWS
// retired the feature in 2023), so DescribeElasticGpus never populates one —
// this type exists purely so the response stays correctly shaped, mirroring
// the ClientVpnConnection precedent elsewhere in this package.
type ElasticGpuStub struct {
	ElasticGpuID   string
	InstanceID     string
	ElasticGpuType string
}

// DescribeInstances returns instances, optionally filtered by IDs or state.
// When ids are provided, lookups are O(len(ids)) via the instance map rather
// than scanning every instance in the backend.
func (b *InMemoryBackend) DescribeInstances(ids []string, state string) []*Instance {
	b.mu.RLock("DescribeInstances")
	defer b.mu.RUnlock()

	if len(ids) > 0 {
		out := make([]*Instance, 0, len(ids))

		for _, id := range ids {
			inst, ok := b.instances.Get(id)
			if !ok {
				continue
			}

			if state != "" && inst.State.Name != state {
				continue
			}

			cp := *inst
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*Instance, 0, b.instances.Len())
	for _, inst := range b.instances.All() {
		if state == "" || inst.State.Name == state {
			cp := *inst
			out = append(out, &cp)
		}
	}

	return out
}

// TerminateInstances transitions instances to shutting-down then terminated.
// Returns the previous and current state for each instance.
// Terminated instances remain visible (matching AWS ~1 hour grace period)
// until the janitor sweeps them.
func (b *InMemoryBackend) TerminateInstances(ids []string) ([]*InstanceStateChange, error) {
	b.mu.Lock("TerminateInstances")
	defer b.mu.Unlock()

	var result []*InstanceStateChange

	for _, id := range ids {
		inst, ok := b.instances.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
		}

		if inst.DisableAPITermination {
			return nil, fmt.Errorf(
				"%w: the instance %s may not be terminated. "+
					"Modify its 'disableApiTermination' instance attribute and try again",
				ErrOperationNotPermitted, id)
		}

		prev := inst.State
		// AWS state machine: any state → shutting-down → terminated (reconciler advances).
		// Resource cleanup (ENIs, EIPs, volumes) happens immediately so callers
		// do not observe dangling attachments, but state advances asynchronously.
		inst.State = StateShuttingDown
		inst.TerminatedAt = time.Now()
		inst.StateReasonCode = "Client.UserInitiatedShutdown"
		inst.StateReasonMessage = "Client.UserInitiatedShutdown: User initiated shutdown"
		inst.StateTransitionReason = fmt.Sprintf(
			"User initiated (%s)", time.Now().UTC().Format("2006-01-02 15:04:05 GMT"),
		)
		result = append(result, &InstanceStateChange{
			InstanceID:    id,
			PreviousState: prev,
			CurrentState:  inst.State,
		})

		b.releaseOutpostCapacityIfFirstTermination(inst, id, prev)

		// Mirror AWS behaviour: when the backing instance of a spot request is
		// terminated, the request transitions to "closed" (not stateCancelled).
		for _, req := range b.spotRequests.All() {
			if req.InstanceID == id && req.State == stateActive {
				req.State = "closed"
				req.CancelledAt = time.Now()
			}
		}

		// Per real AWS's per-attachment DeleteOnTermination flag: the
		// launch-created primary ENI (true) is deleted; an ENI attached later via
		// CreateNetworkInterface (false, the real default) only detaches and
		// survives termination in "available" state ("leftover ENI" behaviour).
		eniIDs := b.eniIDsByInstance[id]
		for eniID := range eniIDs {
			eni, exists := b.networkInterfaces.Get(eniID)
			if !exists {
				continue
			}

			b.deindexENILocked(eniID, eni)

			if eni.DeleteOnTermination {
				b.deindexENIByVPCLocked(eniID, eni)
				b.recycleENIIPsLocked(eni)
				b.networkInterfaces.Delete(eniID)
				delete(b.tags, eniID)

				continue
			}

			eni.InstanceID = ""
			eni.AttachmentID = ""
			eni.DeviceIndex = 0
			eni.Status = stateAvailable
		}
		b.deindexInstanceLocked(inst)

		// Detach any EBS volumes whose attachment refers to this instance so they
		// return to "available" state. AWS deletes the root volume by default
		// (deleteOnTermination=true) but detaches additional volumes; since we do
		// not track the deleteOnTermination flag per attachment, detaching all of
		// them is the safe, non-destructive equivalent.
		// Also disassociate any Elastic IPs associated with the terminated instance.
		b.detachVolumesAndEIPsLocked(id)

		// A terminated instance can't hold an IAM instance profile association
		// (gopherstack-hmfm): without this the association outlives the
		// instance and DescribeIamInstanceProfileAssociations returns it forever.
		b.disassociateIamInstanceProfilesLocked(id)
	}

	return result, nil
}

// SetInstanceLaunchConfig sets the key pair name and security groups on an instance.
func (b *InMemoryBackend) SetInstanceLaunchConfig(instanceID, keyName string, securityGroups []string) error {
	b.mu.Lock("SetInstanceLaunchConfig")
	defer b.mu.Unlock()

	inst, ok := b.instances.Get(instanceID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	if keyName != "" {
		inst.KeyName = keyName
	}

	if len(securityGroups) > 0 {
		inst.SecurityGroups = append([]string(nil), securityGroups...)
	}

	return nil
}
