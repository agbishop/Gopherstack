package ec2

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Errors for network interface (ENI) operations.
var (
	ErrNetworkInterfaceNotFound = errors.New("InvalidNetworkInterfaceID.NotFound")
	ErrNetworkInterfaceInUse    = errors.New("InvalidNetworkInterfaceID.InUse")
	ErrAttachmentNotFound       = errors.New("InvalidAttachmentID.NotFound")
)

// Attribute name and boolean-string constants used when reading/writing ENI attributes.
const (
	attrSourceDest = "sourceDestCheck"
	ec2BooleanTrue = "true"
)

// NetworkInterface represents an EC2 Network Interface (ENI).
type NetworkInterface struct {
	ID                    string   `json:"id,omitempty"`
	SubnetID              string   `json:"subnetID,omitempty"`
	VPCID                 string   `json:"vpcID,omitempty"`
	PrivateIP             string   `json:"privateIP,omitempty"`
	Description           string   `json:"description,omitempty"`
	InstanceID            string   `json:"instanceID,omitempty"`
	AttachmentID          string   `json:"attachmentID,omitempty"`
	Status                string   `json:"status,omitempty"`
	OwnerID               string   `json:"ownerID,omitempty"`
	PublicDNSHostnameType string   `json:"publicDnsHostnameType,omitempty"`
	SecondaryPrivateIPs   []string `json:"secondaryPrivateIPs,omitempty"`
	DeviceIndex           int      `json:"deviceIndex,omitempty"`
	SourceDestCheck       bool     `json:"sourceDestCheck,omitempty"`
	// DeleteOnTermination mirrors real AWS's per-attachment default: true for
	// the primary interface auto-created at instance launch, false for any
	// interface created separately (CreateNetworkInterface) and later attached
	// via AttachNetworkInterface. A real AttachNetworkInterface call never sets
	// this to true - only the launch path and ModifyNetworkInterfaceAttribute's
	// Attachment.DeleteOnTermination can.
	DeleteOnTermination bool `json:"deleteOnTermination,omitempty"`
}

// DescribeNetworkInterfaces returns network interfaces, optionally filtered by IDs.
// When ids are provided, lookups are O(len(ids)) via the ENI map rather than
// scanning every interface in the backend.
func (b *InMemoryBackend) DescribeNetworkInterfaces(ids []string) []*NetworkInterface {
	b.mu.RLock("DescribeNetworkInterfaces")
	defer b.mu.RUnlock()

	if len(ids) > 0 {
		out := make([]*NetworkInterface, 0, len(ids))

		for _, id := range ids {
			eni, ok := b.networkInterfaces.Get(id)
			if !ok {
				continue
			}

			cp := *eni
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*NetworkInterface, 0, b.networkInterfaces.Len())

	for _, eni := range b.networkInterfaces.All() {
		cp := *eni
		out = append(out, &cp)
	}

	return out
}

// ---- network interface full CRUD ----

// CreateNetworkInterface creates a new ENI in the given subnet.
func (b *InMemoryBackend) CreateNetworkInterface(
	subnetID, description string,
) (*NetworkInterface, error) {
	if subnetID == "" {
		return nil, fmt.Errorf("%w: SubnetId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateNetworkInterface")
	defer b.mu.Unlock()

	sub, ok := b.subnets.Get(subnetID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	id := newENIID()
	eni := &NetworkInterface{
		ID:              id,
		SubnetID:        subnetID,
		VPCID:           sub.VPCID,
		PrivateIP:       b.allocPrivateIP(),
		Description:     description,
		Status:          stateAvailable,
		OwnerID:         b.AccountID,
		SourceDestCheck: true,
	}
	b.networkInterfaces.Put(eni)
	b.indexENILocked(id, eni)
	b.indexENIByVPCLocked(id, eni)

	return eni, nil
}

// DeleteNetworkInterface removes a network interface by ID.
// Returns ErrNetworkInterfaceInUse if the ENI is currently attached.
func (b *InMemoryBackend) DeleteNetworkInterface(id string) error {
	b.mu.Lock("DeleteNetworkInterface")
	defer b.mu.Unlock()

	eni, ok := b.networkInterfaces.Get(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, id)
	}

	if eni.InstanceID != "" {
		return fmt.Errorf(
			"%w: %s is currently attached to instance %s",
			ErrNetworkInterfaceInUse,
			id,
			eni.InstanceID,
		)
	}

	b.recycleENIIPsLocked(eni)
	b.deindexENILocked(id, eni)
	b.deindexENIByVPCLocked(id, eni)
	b.networkInterfaces.Delete(id)
	delete(b.tags, id)
	delete(b.niIPv6Addresses, id)

	return nil
}

// AttachNetworkInterface attaches an ENI to an instance and returns the attachment ID.
func (b *InMemoryBackend) AttachNetworkInterface(
	eniID, instanceID string,
	deviceIndex int,
) (string, error) {
	b.mu.Lock("AttachNetworkInterface")
	defer b.mu.Unlock()

	eni, ok := b.networkInterfaces.Get(eniID)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, eniID)
	}

	if eni.AttachmentID != "" {
		return "", fmt.Errorf("%w: %s is already attached", ErrNetworkInterfaceInUse, eniID)
	}

	if _, ok = b.instances.Get(instanceID); !ok {
		return "", fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	attachmentID := newENIAttachmentID()
	b.deindexENILocked(eniID, eni)
	eni.InstanceID = instanceID
	eni.AttachmentID = attachmentID
	eni.DeviceIndex = deviceIndex
	eni.Status = stateInUse
	b.indexENILocked(eniID, eni)

	return attachmentID, nil
}

// DetachNetworkInterface detaches a network interface by attachment ID.
func (b *InMemoryBackend) DetachNetworkInterface(attachmentID string, _ bool) error {
	b.mu.Lock("DetachNetworkInterface")
	defer b.mu.Unlock()

	eniID, ok := b.eniIDByAttachment[attachmentID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAttachmentNotFound, attachmentID)
	}

	eni, ok := b.networkInterfaces.Get(eniID)
	if !ok {
		delete(b.eniIDByAttachment, attachmentID)

		return fmt.Errorf("%w: %s", ErrAttachmentNotFound, attachmentID)
	}

	b.deindexENILocked(eniID, eni)
	eni.InstanceID = ""
	eni.AttachmentID = ""
	eni.DeviceIndex = 0
	eni.Status = stateAvailable
	eni.DeleteOnTermination = false

	return nil
}

// SetNetworkInterfaceDeleteOnTermination updates the DeleteOnTermination flag
// for the attachment identified by attachmentID, matching real AWS's
// ModifyNetworkInterfaceAttribute Attachment.DeleteOnTermination parameter.
func (b *InMemoryBackend) SetNetworkInterfaceDeleteOnTermination(
	attachmentID string,
	del bool,
) error {
	b.mu.Lock("SetNetworkInterfaceDeleteOnTermination")
	defer b.mu.Unlock()

	eniID, ok := b.eniIDByAttachment[attachmentID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAttachmentNotFound, attachmentID)
	}

	eni, ok := b.networkInterfaces.Get(eniID)
	if !ok {
		delete(b.eniIDByAttachment, attachmentID)

		return fmt.Errorf("%w: %s", ErrAttachmentNotFound, attachmentID)
	}

	eni.DeleteOnTermination = del

	return nil
}

// AssignPrivateIPAddresses adds secondary private IP addresses to an ENI.
// count is used when ips is empty; otherwise the supplied IPs are assigned.
// Returns the IPs actually assigned.
func (b *InMemoryBackend) AssignPrivateIPAddresses(eniID string, count int, ips []string) ([]string, error) {
	b.mu.Lock("AssignPrivateIPAddresses")
	defer b.mu.Unlock()

	eni, ok := b.networkInterfaces.Get(eniID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, eniID)
	}

	if len(ips) > 0 {
		eni.SecondaryPrivateIPs = append(eni.SecondaryPrivateIPs, ips...)

		return ips, nil
	}

	assigned := make([]string, 0, count)
	for range count {
		ip := b.allocPrivateIP()
		eni.SecondaryPrivateIPs = append(eni.SecondaryPrivateIPs, ip)
		assigned = append(assigned, ip)
	}

	return assigned, nil
}

// UnassignPrivateIPAddresses removes secondary private IP addresses from an ENI.
// Auto-allocated IPs (172.31.x.y) are returned to the free list so they can be
// reused by subsequent allocations.
func (b *InMemoryBackend) UnassignPrivateIPAddresses(eniID string, ips []string) error {
	b.mu.Lock("UnassignPrivateIPAddresses")
	defer b.mu.Unlock()

	eni, ok := b.networkInterfaces.Get(eniID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, eniID)
	}

	remove := make(map[string]bool, len(ips))
	for _, ip := range ips {
		remove[ip] = true
	}

	kept := eni.SecondaryPrivateIPs[:0]

	for _, ip := range eni.SecondaryPrivateIPs {
		if !remove[ip] {
			kept = append(kept, ip)
		} else {
			// Return auto-allocated IPs to the free list for reuse, up to the cap.
			b.recycleIPLocked(ip)
		}
	}

	eni.SecondaryPrivateIPs = kept

	return nil
}

// ModifyNetworkInterfaceAttribute updates a single attribute of an ENI.
// Supported attributes: description, sourceDestCheck.
func (b *InMemoryBackend) ModifyNetworkInterfaceAttribute(eniID, attr, value string) error {
	b.mu.Lock("ModifyNetworkInterfaceAttribute")
	defer b.mu.Unlock()

	eni, ok := b.networkInterfaces.Get(eniID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, eniID)
	}

	switch attr {
	case filterKeyDescription:
		eni.Description = value
	case attrSourceDest:
		eni.SourceDestCheck = value == ec2BooleanTrue
	default:
		return fmt.Errorf("%w: unsupported attribute %q", ErrInvalidParameter, attr)
	}

	return nil
}

// PrimaryNetworkInterfaceSourceDestCheck returns the sourceDestCheck flag of
// instanceID's primary network interface. AWS defaults this to true for VPC
// instances; unknown instances/interfaces also report true to match that
// default rather than a zero-value false.
func (b *InMemoryBackend) PrimaryNetworkInterfaceSourceDestCheck(instanceID string) bool {
	b.mu.RLock("PrimaryNetworkInterfaceSourceDestCheck")
	defer b.mu.RUnlock()

	if eni := b.primaryNetworkInterfaceLocked(instanceID); eni != nil {
		return eni.SourceDestCheck
	}

	return true
}

// DescribeNetworkInterfaceAttribute returns a requested attribute for a network interface.
func (b *InMemoryBackend) DescribeNetworkInterfaceAttribute(
	niID string, _ string,
) (*NIAttributeResult, error) {
	if niID == "" {
		return nil, fmt.Errorf("%w: NetworkInterfaceId is required", ErrInvalidParameter)
	}

	b.mu.RLock("DescribeNetworkInterfaceAttribute")
	defer b.mu.RUnlock()

	ni, ok := b.networkInterfaces.Get(niID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, niID)
	}

	result := &NIAttributeResult{
		NetworkInterfaceID: niID,
		Description:        ni.Description,
		SourceDestCheck:    ni.SourceDestCheck,
	}
	if ni.InstanceID != "" {
		result.HasAttachment = true
		result.AttachmentID = ni.AttachmentID
		result.AttachInstanceID = ni.InstanceID
		result.AttachDeviceIndex = ni.DeviceIndex
		result.AttachStatus = attachmentStateAttached
		result.AttachDeleteOnTerm = ni.DeleteOnTermination
	}

	return result, nil
}

// ResetNetworkInterfaceAttribute resets sourceDestCheck to true for a network interface.
func (b *InMemoryBackend) ResetNetworkInterfaceAttribute(niID string) error {
	if niID == "" {
		return fmt.Errorf("%w: NetworkInterfaceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ResetNetworkInterfaceAttribute")
	defer b.mu.Unlock()

	ni, ok := b.networkInterfaces.Get(niID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, niID)
	}
	ni.SourceDestCheck = true

	return nil
}

// ---- Network Interface permissions ----

// DescribeNetworkInterfacePermissions returns permissions for the given NIDs (or all).
func (b *InMemoryBackend) DescribeNetworkInterfacePermissions(
	niIDs []string,
) []*NetworkInterfacePermission {
	b.mu.RLock("DescribeNetworkInterfacePermissions")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(niIDs))
	for _, id := range niIDs {
		filter[id] = true
	}

	var out []*NetworkInterfacePermission
	for _, perm := range b.niPermissions.All() {
		if len(filter) > 0 && !filter[perm.NetworkInterfaceID] {
			continue
		}
		cp := *perm
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PermissionID < out[j].PermissionID })

	return out
}

// CreateNetworkInterfacePermission creates a new permission on a network interface.
func (b *InMemoryBackend) CreateNetworkInterfacePermission(
	niID, awsAccountID, awsService, permission string,
) (*NetworkInterfacePermission, error) {
	if niID == "" {
		return nil, fmt.Errorf("%w: NetworkInterfaceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateNetworkInterfacePermission")
	defer b.mu.Unlock()

	if _, ok := b.networkInterfaces.Get(niID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, niID)
	}

	perm := &NetworkInterfacePermission{
		PermissionID:       "eni-perm-" + uuid.New().String()[:8],
		NetworkInterfaceID: niID,
		AwsAccountID:       awsAccountID,
		AwsService:         awsService,
		Permission:         permission,
		State:              "granted",
	}
	b.niPermissions.Put(perm)

	return perm, nil
}

// DeleteNetworkInterfacePermission removes a network interface permission.
func (b *InMemoryBackend) DeleteNetworkInterfacePermission(permissionID string) error {
	if permissionID == "" {
		return fmt.Errorf("%w: NetworkInterfacePermissionId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteNetworkInterfacePermission")
	defer b.mu.Unlock()

	if _, ok := b.niPermissions.Get(permissionID); !ok {
		return fmt.Errorf("%w: %s", ErrNetworkInterfacePermissionNotFound, permissionID)
	}
	b.niPermissions.Delete(permissionID)

	return nil
}

// ---- IPv6 address assignment ----

// AssignIpv6Addresses assigns IPv6 addresses to a network interface.
func (b *InMemoryBackend) AssignIpv6Addresses(niID string, count int) ([]string, error) {
	if niID == "" {
		return nil, fmt.Errorf("%w: NetworkInterfaceId is required", ErrInvalidParameter)
	}
	if count < 1 {
		count = 1
	}

	b.mu.Lock("AssignIpv6Addresses")
	defer b.mu.Unlock()

	if _, ok := b.networkInterfaces.Get(niID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, niID)
	}

	assigned := make([]string, 0, count)
	for range count {
		addr := fmt.Sprintf("2001:db8::%s", uuid.New().String()[:4])
		b.niIPv6Addresses[niID] = append(b.niIPv6Addresses[niID], addr)
		assigned = append(assigned, addr)
	}

	return assigned, nil
}

// UnassignIpv6Addresses removes IPv6 addresses from a network interface.
func (b *InMemoryBackend) UnassignIpv6Addresses(niID string, addresses []string) error {
	if niID == "" {
		return fmt.Errorf("%w: NetworkInterfaceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("UnassignIpv6Addresses")
	defer b.mu.Unlock()

	if _, ok := b.networkInterfaces.Get(niID); !ok {
		return fmt.Errorf("%w: %s", ErrNetworkInterfaceNotFound, niID)
	}

	toRemove := make(map[string]bool, len(addresses))
	for _, a := range addresses {
		toRemove[a] = true
	}

	filtered := b.niIPv6Addresses[niID][:0]
	for _, a := range b.niIPv6Addresses[niID] {
		if !toRemove[a] {
			filtered = append(filtered, a)
		}
	}
	b.niIPv6Addresses[niID] = filtered

	return nil
}

// ---- DescribeAccountAttributes ----

// AccountAttribute represents a single EC2 account attribute.
type AccountAttribute struct {
	Name   string   `json:"name,omitempty"`
	Values []string `json:"values,omitempty"`
}

// maxFreePrivateIPs caps the recycled private-IP free list to avoid
// unbounded memory growth in long-running tests.
const maxFreePrivateIPs = 1000

// recycleIPLocked adds ip to the free list if it is an auto-allocated
// 172.31.x.y address and the free list has capacity remaining.
// Must be called with b.mu held.
func (b *InMemoryBackend) recycleIPLocked(ip string) {
	if strings.HasPrefix(ip, "172.31.") && len(b.freePrivateIPs) < maxFreePrivateIPs {
		b.freePrivateIPs = append(b.freePrivateIPs, ip)
	}
}

// recycleENIIPsLocked returns the primary and secondary auto-allocated private
// IPs from an ENI back to the free list so they can be reused.
// Must be called with b.mu held.
func (b *InMemoryBackend) recycleENIIPsLocked(eni *NetworkInterface) {
	b.recycleIPLocked(eni.PrivateIP)

	for _, ip := range eni.SecondaryPrivateIPs {
		b.recycleIPLocked(ip)
	}
}

// detachVolumesAndEIPsLocked clears volume attachments and Elastic IP
// associations that reference instanceID so that no dangling references remain
// after the instance transitions to the terminated state.
// Must be called with b.mu held.
func (b *InMemoryBackend) detachVolumesAndEIPsLocked(instanceID string) {
	for _, vol := range b.volumes.All() {
		if vol.Attachment != nil && vol.Attachment.InstanceID == instanceID {
			vol.Attachment = nil
			vol.State = stateAvailable
		}
	}

	for _, addr := range b.addresses.All() {
		if addr.InstanceID == instanceID {
			addr.AssociationID = ""
			addr.InstanceID = ""
		}
	}
}
