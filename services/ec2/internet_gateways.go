package ec2

import (
	"errors"
	"fmt"
)

// ErrInternetGatewayNotFound is returned when an Internet Gateway is not found.
var ErrInternetGatewayNotFound = errors.New("InvalidInternetGatewayID.NotFound")

// IGWAttachment represents the attachment of an Internet Gateway to a VPC.
type IGWAttachment struct {
	VPCID string `json:"vpcID,omitempty"`
	State string `json:"state,omitempty"`
}

// InternetGateway represents an EC2 Internet Gateway.
type InternetGateway struct {
	ID          string          `json:"id,omitempty"`
	Attachments []IGWAttachment `json:"attachments,omitempty"`
}

// CreateInternetGateway creates a new Internet Gateway.
func (b *InMemoryBackend) CreateInternetGateway() (*InternetGateway, error) {
	b.mu.Lock("CreateInternetGateway")
	defer b.mu.Unlock()

	id := newInternetGatewayID()
	igw := &InternetGateway{
		ID:          id,
		Attachments: []IGWAttachment{},
	}
	b.internetGateways.Put(igw)

	return igw, nil
}

// DeleteInternetGateway removes an Internet Gateway. Matching real AWS, this
// fails with DependencyViolation while the IGW is still attached to a VPC —
// callers must DetachInternetGateway first.
func (b *InMemoryBackend) DeleteInternetGateway(id string) error {
	b.mu.Lock("DeleteInternetGateway")
	defer b.mu.Unlock()

	igw, ok := b.internetGateways.Get(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInternetGatewayNotFound, id)
	}

	if len(igw.Attachments) > 0 {
		return fmt.Errorf(
			"%w: the internetGateway %s has dependencies (VPC attachment) and cannot be deleted",
			ErrDependencyViolation, id,
		)
	}

	b.internetGateways.Delete(id)
	delete(b.tags, id)

	return nil
}

// DescribeInternetGateways returns IGWs, optionally filtered by IDs.
// When ids are provided, lookups are O(len(ids)) via the IGW map rather than
// scanning every IGW in the backend.
func (b *InMemoryBackend) DescribeInternetGateways(ids []string) []*InternetGateway {
	b.mu.RLock("DescribeInternetGateways")
	defer b.mu.RUnlock()

	if len(ids) > 0 {
		out := make([]*InternetGateway, 0, len(ids))

		for _, id := range ids {
			igw, ok := b.internetGateways.Get(id)
			if !ok {
				continue
			}

			cp := *igw
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*InternetGateway, 0, b.internetGateways.Len())

	for _, igw := range b.internetGateways.All() {
		cp := *igw
		out = append(out, &cp)
	}

	return out
}

// AttachInternetGateway attaches an IGW to a VPC. Matching real AWS, an IGW
// can only be attached to one VPC at a time, and a VPC can only have one IGW
// attached at a time; either conflict returns Resource.AlreadyAssociated.
func (b *InMemoryBackend) AttachInternetGateway(igwID, vpcID string) error {
	b.mu.Lock("AttachInternetGateway")
	defer b.mu.Unlock()

	igw, ok := b.internetGateways.Get(igwID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInternetGatewayNotFound, igwID)
	}

	if _, vpcOK := b.vpcs.Get(vpcID); !vpcOK {
		return fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	if len(igw.Attachments) > 0 {
		return fmt.Errorf(
			"%w: resource %s is already attached to network %s",
			ErrResourceAlreadyAssociated, igwID, igw.Attachments[0].VPCID,
		)
	}

	for _, other := range b.internetGateways.All() {
		for _, att := range other.Attachments {
			if att.VPCID == vpcID {
				return fmt.Errorf(
					"%w: the vpc %s already has an internet gateway attached",
					ErrResourceAlreadyAssociated, vpcID,
				)
			}
		}
	}

	igw.Attachments = append(igw.Attachments, IGWAttachment{VPCID: vpcID, State: stateAvailable})

	return nil
}

// DetachInternetGateway detaches an IGW from a VPC. Matching real AWS, this
// fails with DependencyViolation while vpcID still has a running instance
// with a public IPv4 address or an associated Elastic IP.
func (b *InMemoryBackend) DetachInternetGateway(igwID, vpcID string) error {
	b.mu.Lock("DetachInternetGateway")
	defer b.mu.Unlock()

	igw, ok := b.internetGateways.Get(igwID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInternetGatewayNotFound, igwID)
	}

	for i, att := range igw.Attachments {
		if att.VPCID == vpcID {
			if err := b.igwDetachDependencyViolationLocked(vpcID); err != nil {
				return err
			}

			igw.Attachments = append(igw.Attachments[:i], igw.Attachments[i+1:]...)

			return nil
		}
	}

	return fmt.Errorf("%w: IGW %s is not attached to VPC %s", ErrInvalidParameter, igwID, vpcID)
}

// igwDetachDependencyViolationLocked returns a DependencyViolation error if
// vpcID has a running instance with a public IPv4 address or an associated
// Elastic IP. Must be called with b.mu held.
func (b *InMemoryBackend) igwDetachDependencyViolationLocked(vpcID string) error {
	for _, inst := range b.instances.All() {
		if inst.VPCID != vpcID || inst.State != StateRunning || inst.PublicIPAddress == "" {
			continue
		}

		return fmt.Errorf(
			"%w: the vpc %s has dependencies (instance %s with a public IP address) "+
				"and cannot detach its internet gateway",
			ErrDependencyViolation, vpcID, inst.ID,
		)
	}

	for _, addr := range b.addresses.All() {
		if addr.InstanceID == "" {
			continue
		}

		inst, ok := b.instances.Get(addr.InstanceID)
		if !ok || inst.VPCID != vpcID || inst.State != StateRunning {
			continue
		}

		return fmt.Errorf(
			"%w: the vpc %s has dependencies (elastic IP %s associated with instance %s) "+
				"and cannot detach its internet gateway",
			ErrDependencyViolation, vpcID, addr.PublicIP, inst.ID,
		)
	}

	return nil
}
