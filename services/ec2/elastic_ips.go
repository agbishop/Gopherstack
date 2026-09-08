package ec2

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Errors for Elastic IP operations.
var (
	ErrAddressNotFound     = errors.New("InvalidAllocationID.NotFound")
	ErrAssociationNotFound = errors.New("InvalidAssociationID.NotFound")
	ErrAddressInUse        = errors.New("InvalidIPAddress.InUse")
)

const elasticIPOctetRange = 254

// Address represents an Elastic IP address.
type Address struct {
	AllocationID  string `json:"allocationID,omitempty"`
	AssociationID string `json:"associationID,omitempty"`
	PublicIP      string `json:"publicIP,omitempty"`
	InstanceID    string `json:"instanceID,omitempty"`
}

// allocElasticIP returns the next 54.230.x.y elastic IP. Must be called with mu held.
func (b *InMemoryBackend) allocElasticIP() string {
	idx := b.nextElasticIPIndex
	b.nextElasticIPIndex++
	third := idx / elasticIPOctetRange
	fourth := idx%elasticIPOctetRange + 1

	return fmt.Sprintf("54.230.%d.%d", third, fourth)
}

// AllocateAddress allocates a new Elastic IP address.
func (b *InMemoryBackend) AllocateAddress() (*Address, error) {
	b.mu.Lock("AllocateAddress")
	defer b.mu.Unlock()

	id := newEIPAllocationID()
	addr := &Address{
		AllocationID: id,
		PublicIP:     b.allocElasticIP(),
	}
	b.addresses.Put(addr)

	return addr, nil
}

// AssociateAddress associates an Elastic IP with an instance.
func (b *InMemoryBackend) AssociateAddress(allocationID, instanceID string) (string, error) {
	b.mu.Lock("AssociateAddress")
	defer b.mu.Unlock()

	addr, ok := b.addresses.Get(allocationID)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrAddressNotFound, allocationID)
	}

	if _, instOK := b.instances.Get(instanceID); !instOK {
		return "", fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	assocID := newEIPAssociationID()
	addr.AssociationID = assocID
	addr.InstanceID = instanceID

	return assocID, nil
}

// DisassociateAddress removes an Elastic IP association.
func (b *InMemoryBackend) DisassociateAddress(associationID string) error {
	b.mu.Lock("DisassociateAddress")
	defer b.mu.Unlock()

	for _, addr := range b.addresses.All() {
		if addr.AssociationID == associationID {
			addr.AssociationID = ""
			addr.InstanceID = ""

			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrAssociationNotFound, associationID)
}

// ReleaseAddress releases an Elastic IP allocation.
func (b *InMemoryBackend) ReleaseAddress(allocationID string) error {
	b.mu.Lock("ReleaseAddress")
	defer b.mu.Unlock()

	addr, ok := b.addresses.Get(allocationID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrAddressNotFound, allocationID)
	}

	if addr.AssociationID != "" {
		return fmt.Errorf("%w: the address %s is associated (%s) and must be disassociated before release",
			ErrAddressInUse, addr.PublicIP, addr.AssociationID)
	}

	b.addresses.Delete(allocationID)
	delete(b.tags, allocationID)
	delete(b.addressTransfers, allocationID)

	return nil
}

// DescribeAddresses returns Elastic IPs, optionally filtered by allocation IDs.
// When allocationIDs are provided, lookups are O(len(allocationIDs)) via the
// address map rather than scanning every address in the backend.
func (b *InMemoryBackend) DescribeAddresses(allocationIDs []string) []*Address {
	b.mu.RLock("DescribeAddresses")
	defer b.mu.RUnlock()

	if len(allocationIDs) > 0 {
		out := make([]*Address, 0, len(allocationIDs))

		for _, id := range allocationIDs {
			addr, ok := b.addresses.Get(id)
			if !ok {
				continue
			}

			cp := *addr
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*Address, 0, b.addresses.Len())

	for _, addr := range b.addresses.All() {
		cp := *addr
		out = append(out, &cp)
	}

	return out
}

// DescribeAddressesAttribute returns domain-name attributes for Elastic IPs.
func (b *InMemoryBackend) DescribeAddressesAttribute(allocationIDs []string) []AddressAttribute {
	b.mu.RLock("DescribeAddressesAttribute")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(allocationIDs))
	for _, id := range allocationIDs {
		filter[id] = true
	}

	var out []AddressAttribute
	for _, addr := range b.addresses.All() {
		if len(filter) > 0 && !filter[addr.AllocationID] {
			continue
		}
		attr := AddressAttribute{
			AllocationID: addr.AllocationID,
			PublicIP:     addr.PublicIP,
		}
		if stored, ok := b.addressAttributes.Get(addr.AllocationID); ok {
			attr.DomainName = stored.DomainName
		}
		out = append(out, attr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AllocationID < out[j].AllocationID })

	return out
}

// ModifyAddressAttribute sets the domain name for an Elastic IP.
func (b *InMemoryBackend) ModifyAddressAttribute(allocationID, domainName string) error {
	if allocationID == "" {
		return fmt.Errorf("%w: AllocationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyAddressAttribute")
	defer b.mu.Unlock()

	addr, ok := b.addresses.Get(allocationID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInvalidParameter, allocationID)
	}
	b.addressAttributes.Put(&AddressAttribute{
		AllocationID: allocationID,
		PublicIP:     addr.PublicIP,
		DomainName:   domainName,
	})

	return nil
}

// ResetAddressAttribute clears the domain name for an Elastic IP.
func (b *InMemoryBackend) ResetAddressAttribute(allocationID string) (*Address, error) {
	if allocationID == "" {
		return nil, fmt.Errorf("%w: AllocationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ResetAddressAttribute")
	defer b.mu.Unlock()

	addr, ok := b.addresses.Get(allocationID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidParameter, allocationID)
	}
	b.addressAttributes.Delete(allocationID)

	cp := *addr

	return &cp, nil
}

// ---- Instance console output ----

// EnableAddressTransfer enables EIP transfer to another account.
func (b *InMemoryBackend) EnableAddressTransfer(
	allocationID, transferAccountID string,
) (*AddressTransfer, error) {
	if allocationID == "" {
		return nil, fmt.Errorf("%w: AllocationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("EnableAddressTransfer")
	defer b.mu.Unlock()

	addr, ok := b.addresses.Get(allocationID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidParameter, allocationID)
	}

	transfer := &AddressTransfer{
		AllocationID:        allocationID,
		PublicIP:            addr.PublicIP,
		TransferAccountID:   transferAccountID,
		TransferOfferStatus: "pending",
		TransferOfferExpiry: time.Now().UTC().AddDate(0, 0, addressTransferOfferDays),
	}
	b.addressTransfers[allocationID] = transfer

	return transfer, nil
}

// DisableAddressTransfer cancels an EIP transfer.
func (b *InMemoryBackend) DisableAddressTransfer(allocationID string) (*AddressTransfer, error) {
	if allocationID == "" {
		return nil, fmt.Errorf("%w: AllocationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DisableAddressTransfer")
	defer b.mu.Unlock()

	addr, ok := b.addresses.Get(allocationID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidParameter, allocationID)
	}

	transfer, existed := b.addressTransfers[allocationID]
	if !existed {
		transfer = &AddressTransfer{AllocationID: allocationID, PublicIP: addr.PublicIP}
	}

	cp := *transfer
	delete(b.addressTransfers, allocationID)

	return &cp, nil
}

// DescribeAddressTransfers returns address transfers, optionally filtered by allocation ID.
func (b *InMemoryBackend) DescribeAddressTransfers(allocationIDs []string) []*AddressTransfer {
	b.mu.RLock("DescribeAddressTransfers")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(allocationIDs))
	for _, id := range allocationIDs {
		filter[id] = true
	}

	var out []*AddressTransfer
	for _, t := range b.addressTransfers {
		if len(filter) > 0 && !filter[t.AllocationID] {
			continue
		}
		cp := *t
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AllocationID < out[j].AllocationID })

	return out
}

// ---- Subnet CIDR reservations ----

// RestoreAddressToClassic moves an EIP from VPC to EC2-Classic (no-op in mock).
func (b *InMemoryBackend) RestoreAddressToClassic(publicIP string) error {
	if publicIP == "" {
		return fmt.Errorf("%w: PublicIp is required", ErrInvalidParameter)
	}

	return nil
}

// ---- ReportInstanceStatus ----

// findAddressByPublicIPLocked scans the addresses map for an EIP by public IP
// address. Must be called with b.mu held.
func (b *InMemoryBackend) findAddressByPublicIPLocked(publicIP string) *Address {
	for _, addr := range b.addresses.All() {
		if addr.PublicIP == publicIP {
			return addr
		}
	}

	return nil
}

// MoveAddressToVpc marks an existing Elastic IP as moving into the VPC
// platform, overlaying EC2-Classic move status on top of the existing
// address state.
func (b *InMemoryBackend) MoveAddressToVpc(publicIP string) (*Address, error) {
	if publicIP == "" {
		return nil, fmt.Errorf("%w: PublicIp is required", ErrInvalidParameter)
	}

	b.mu.Lock("MoveAddressToVpc")
	defer b.mu.Unlock()

	addr := b.findAddressByPublicIPLocked(publicIP)
	if addr == nil {
		return nil, fmt.Errorf("%w: %s", ErrPublicIPNotFound, publicIP)
	}
	b.movingAddresses.Put(&MovingAddressStatus{
		PublicIP:   publicIP,
		MoveStatus: moveStatusMovingToVpc,
	})

	cp := *addr

	return &cp, nil
}

// DescribeMovingAddresses returns the moving-address status entries created
// by MoveAddressToVpc, optionally filtered by public IP.
func (b *InMemoryBackend) DescribeMovingAddresses(publicIPs []string) []*MovingAddressStatus {
	b.mu.RLock("DescribeMovingAddresses")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(publicIPs))
	for _, ip := range publicIPs {
		filter[ip] = true
	}

	out := make([]*MovingAddressStatus, 0, b.movingAddresses.Len())

	for _, st := range b.movingAddresses.All() {
		if len(filter) > 0 && !filter[st.PublicIP] {
			continue
		}

		cp := *st
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].PublicIP < out[j].PublicIP })

	return out
}

const moveStatusMovingToVpc = "movingToVpc"

// MovingAddressStatus represents the EC2-Classic move state of an Elastic IP,
// as tracked by MoveAddressToVpc/DescribeMovingAddresses.
type MovingAddressStatus struct {
	PublicIP   string `json:"publicIP,omitempty"`
	MoveStatus string `json:"moveStatus,omitempty"`
}
