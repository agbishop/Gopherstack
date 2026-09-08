package apprunner

import (
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateVpcConnector creates a new VPC connector.
func (b *InMemoryBackend) CreateVpcConnector(
	name string,
	subnets, securityGroups []string,
	tags map[string]string,
) (*VpcConnector, error) {
	b.mu.Lock("CreateVpcConnector")
	defer b.mu.Unlock()

	existing := 0
	b.vpcConnectors.Range(func(vc *storedVpcConnector) bool {
		if vc.VpcConnectorName == name {
			existing++
		}

		return true
	})

	revision := int32(existing + 1)
	id := newID()
	vcArn := b.vpcConnectorARN(name, revision, id)
	now := time.Now().UTC()

	sg := make([]string, len(securityGroups))
	copy(sg, securityGroups)
	sn := make([]string, len(subnets))
	copy(sn, subnets)

	vc := &storedVpcConnector{
		VpcConnectorArn:      vcArn,
		VpcConnectorName:     name,
		VpcConnectorRevision: revision,
		Status:               vpcConnStatusActive,
		Subnets:              sn,
		SecurityGroups:       sg,
		CreatedAt:            now,
	}

	b.vpcConnectors.Put(vc)

	if len(tags) > 0 {
		b.tags[vcArn] = make(map[string]string)
		maps.Copy(b.tags[vcArn], tags)
	}

	cp := vc.toVpcConnector()

	return &cp, nil
}

// DescribeVpcConnector returns a VPC connector by ARN.
func (b *InMemoryBackend) DescribeVpcConnector(vcArn string) (*VpcConnector, error) {
	b.mu.RLock("DescribeVpcConnector")
	defer b.mu.RUnlock()

	vc, ok := b.vpcConnectors.Get(vcArn)
	if !ok {
		return nil, fmt.Errorf("vpc connector %s not found: %w", vcArn, ErrNotFound)
	}

	cp := vc.toVpcConnector()

	return &cp, nil
}

// DeleteVpcConnector deletes a VPC connector.
func (b *InMemoryBackend) DeleteVpcConnector(vcArn string) (*VpcConnector, error) {
	b.mu.Lock("DeleteVpcConnector")
	defer b.mu.Unlock()

	vc, ok := b.vpcConnectors.Get(vcArn)
	if !ok {
		return nil, fmt.Errorf("vpc connector %s not found: %w", vcArn, ErrNotFound)
	}

	// DeleteVpcConnector doc (api_op_DeleteVpcConnector.go): "You can't
	// delete a connector that's used by one or more App Runner services."
	if b.serviceUsesVpcConnector(vcArn) {
		return nil, fmt.Errorf("vpc connector %s is used by one or more services: %w", vcArn, ErrInvalidParameter)
	}

	vc.Status = vpcConnStatusInactive
	vc.DeletedAt = time.Now().UTC()
	cp := vc.toVpcConnector()

	b.vpcConnectors.Delete(vcArn)
	delete(b.tags, vcArn)

	return &cp, nil
}

// ListVpcConnectors returns VPC connectors with pagination.
func (b *InMemoryBackend) ListVpcConnectors(maxResults int32, nextToken string) ([]*VpcConnector, string, error) {
	b.mu.RLock("ListVpcConnectors")
	defer b.mu.RUnlock()

	items := b.vpcConnectors.Snapshot()

	all := make([]*VpcConnector, 0, len(items))
	for _, vc := range items {
		cp := vc.toVpcConnector()
		all = append(all, &cp)
	}

	limit := int(maxResults)
	pg := page.New(all, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}
