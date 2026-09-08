package neptune

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) clusterEndpointGet(region, id string) (*DBClusterEndpoint, bool) {
	return b.clusterEndpoints.Get(regionKey(region, id))
}

func (b *InMemoryBackend) clusterEndpointHas(region, id string) bool {
	return b.clusterEndpoints.Has(regionKey(region, id))
}

func (b *InMemoryBackend) clusterEndpointPut(v *DBClusterEndpoint) { b.clusterEndpoints.Put(v) }

func (b *InMemoryBackend) clusterEndpointDelete(region, id string) {
	b.clusterEndpoints.Delete(regionKey(region, id))
}

func (b *InMemoryBackend) clusterEndpointsInRegion(region string) []*DBClusterEndpoint {
	return b.clusterEndpointsByRegion.Get(region)
}

// clusterEndpointARN returns the region-scoped ARN for a Neptune DB cluster endpoint.
func (b *InMemoryBackend) clusterEndpointARN(region, id string) string {
	return arn.Build("rds", region, b.accountID, "cluster-endpoint:"+id)
}

// CreateDBClusterEndpoint creates a Neptune DB cluster custom endpoint.
func (b *InMemoryBackend) CreateDBClusterEndpoint(
	ctx context.Context,
	endpointID, clusterID, endpointType string,
) (*DBClusterEndpoint, error) {
	if endpointID == "" {
		return nil, fmt.Errorf("%w: DBClusterEndpointIdentifier is required", ErrInvalidParameter)
	}
	if clusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateDBClusterEndpoint")
	defer b.mu.Unlock()
	if b.clusterEndpointHas(region, endpointID) {
		return nil, fmt.Errorf(
			"%w: cluster endpoint %s already exists",
			ErrClusterEndpointAlreadyExists,
			endpointID,
		)
	}
	if !b.clusterHas(region, clusterID) {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}
	if endpointType == "" {
		endpointType = endpointTypeReader
	}
	switch endpointType {
	case endpointTypeReader, endpointTypeWriter, endpointTypeCustom, endpointTypeAny:
	default:
		return nil, fmt.Errorf(
			"%w: EndpointType must be one of READER, WRITER, CUSTOM, ANY",
			ErrInvalidParameter,
		)
	}
	ep := &DBClusterEndpoint{
		region:                              region,
		DBClusterEndpointIdentifier:         endpointID,
		DBClusterIdentifier:                 clusterID,
		DBClusterEndpointArn:                b.clusterEndpointARN(region, endpointID),
		DBClusterEndpointResourceIdentifier: fmt.Sprintf("cluster-endpoint-%s", endpointID),
		EndpointType:                        endpointType,
		Status:                              clusterStatusAvailable,
		Endpoint: fmt.Sprintf(
			"%s.cluster-custom.neptune.%s.amazonaws.com",
			endpointID,
			region,
		),
		StaticMembers:   []string{},
		ExcludedMembers: []string{},
	}
	b.clusterEndpointPut(ep)
	cp := *ep
	cp.StaticMembers = make([]string, len(ep.StaticMembers))
	copy(cp.StaticMembers, ep.StaticMembers)
	cp.ExcludedMembers = make([]string, len(ep.ExcludedMembers))
	copy(cp.ExcludedMembers, ep.ExcludedMembers)

	return &cp, nil
}

// DeleteDBClusterEndpoint deletes a Neptune DB cluster custom endpoint,
// returning the deleted endpoint's details -- AWS's DeleteDBClusterEndpoint
// response echoes them back (DeleteDBClusterEndpointResult), unlike e.g.
// DeleteDBSubnetGroup which has a genuinely empty output.
func (b *InMemoryBackend) DeleteDBClusterEndpoint(
	ctx context.Context, endpointID string,
) (*DBClusterEndpoint, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteDBClusterEndpoint")
	defer b.mu.Unlock()
	ep, exists := b.clusterEndpointGet(region, endpointID)
	if !exists {
		return nil, fmt.Errorf(
			"%w: cluster endpoint %s not found",
			ErrClusterEndpointNotFound,
			endpointID,
		)
	}
	cp := *ep
	b.clusterEndpointDelete(region, endpointID)
	delete(b.tagsStore(region), b.clusterEndpointARN(region, endpointID))

	return &cp, nil
}

// DescribeDBClusterEndpoints returns all Neptune DB cluster endpoints or a specific one.
func (b *InMemoryBackend) DescribeDBClusterEndpoints(
	ctx context.Context, endpointID, clusterID string,
) ([]DBClusterEndpoint, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBClusterEndpoints")
	defer b.mu.RUnlock()
	if endpointID != "" {
		ep, exists := b.clusterEndpointGet(region, endpointID)
		if !exists {
			return nil, fmt.Errorf(
				"%w: cluster endpoint %s not found",
				ErrClusterEndpointNotFound,
				endpointID,
			)
		}
		cp := *ep

		return []DBClusterEndpoint{cp}, nil
	}
	clusterEndpoints := b.clusterEndpointsInRegion(region)
	result := make([]DBClusterEndpoint, 0, len(clusterEndpoints))
	for _, ep := range clusterEndpoints {
		if clusterID != "" && ep.DBClusterIdentifier != clusterID {
			continue
		}
		result = append(result, *ep)
	}

	return result, nil
}

// ModifyDBClusterEndpoint modifies a Neptune DB cluster custom endpoint.
// staticMembers/excludedMembers replace the endpoint's respective member
// lists when non-nil (an explicitly-empty list, e.g. from
// StaticMembers.member with zero entries, is indistinguishable from "not
// supplied" on this query-protocol wire format -- same nil-vs-empty
// convention CreateDBClusterEndpoint already used for these two fields).
func (b *InMemoryBackend) ModifyDBClusterEndpoint(
	ctx context.Context, endpointID, endpointType string, staticMembers, excludedMembers []string,
) (*DBClusterEndpoint, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyDBClusterEndpoint")
	defer b.mu.Unlock()
	ep, exists := b.clusterEndpointGet(region, endpointID)
	if !exists {
		return nil, fmt.Errorf(
			"%w: cluster endpoint %s not found",
			ErrClusterEndpointNotFound,
			endpointID,
		)
	}
	if endpointType != "" {
		ep.EndpointType = endpointType
	}
	if len(staticMembers) > 0 {
		ep.StaticMembers = append([]string(nil), staticMembers...)
	}
	if len(excludedMembers) > 0 {
		ep.ExcludedMembers = append([]string(nil), excludedMembers...)
	}
	cp := *ep
	cp.StaticMembers = make([]string, len(ep.StaticMembers))
	copy(cp.StaticMembers, ep.StaticMembers)
	cp.ExcludedMembers = make([]string, len(ep.ExcludedMembers))
	copy(cp.ExcludedMembers, ep.ExcludedMembers)

	return &cp, nil
}
