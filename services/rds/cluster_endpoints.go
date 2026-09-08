package rds

import "fmt"

// CreateDBClusterEndpoint creates a custom endpoint for the given cluster.
func (b *InMemoryBackend) CreateDBClusterEndpoint(
	endpointID, clusterID, endpointType string,
) (*DBClusterEndpoint, error) {
	if endpointID == "" {
		return nil, fmt.Errorf("%w: DBClusterEndpointIdentifier must not be empty", ErrInvalidParameter)
	}
	if clusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier must not be empty", ErrInvalidParameter)
	}
	b.mu.Lock("CreateDBClusterEndpoint")
	defer b.mu.Unlock()
	if _, exists := b.clusterEndpoints.Get(endpointID); exists {
		return nil, fmt.Errorf("%w: cluster endpoint %s already exists", ErrClusterEndpointAlreadyExists, endpointID)
	}
	if _, exists := b.clusters.Get(normalizeID(clusterID)); !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}
	if endpointType == "" {
		endpointType = "ANY"
	}
	ep := &DBClusterEndpoint{
		DBClusterEndpointIdentifier: endpointID,
		DBClusterIdentifier:         clusterID,
		DBClusterEndpointArn:        b.rdsARN("cluster-endpoint", endpointID),
		EndpointType:                endpointType,
		Status:                      instanceStatusAvailable,
		Endpoint: fmt.Sprintf(
			"%s.cluster-custom.%s.%s.rds.amazonaws.com",
			endpointID,
			b.accountID,
			b.region,
		),
	}
	b.clusterEndpoints.Put(ep)
	cp := *ep

	return &cp, nil
}

// DescribeDBClusterEndpoints returns cluster endpoints, filtered by cluster or endpoint ID.
// DBClusterEndpointIdentifier is a filter like DBClusterIdentifier's matching below, not
// an existence check: real AWS declares no not-found error for it. DBClusterIdentifier is
// the opposite (gopherstack-l20u): it's the op's one declared error, DBClusterNotFoundFault,
// so a supplied-but-unknown cluster must fault rather than silently filter to empty; an
// omitted DBClusterIdentifier still lists all endpoints.
func (b *InMemoryBackend) DescribeDBClusterEndpoints(clusterID, endpointID string) ([]DBClusterEndpoint, error) {
	b.mu.RLock("DescribeDBClusterEndpoints")
	defer b.mu.RUnlock()
	if clusterID != "" {
		if _, exists := b.clusters.Get(normalizeID(clusterID)); !exists {
			return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
		}
	}
	result := make([]DBClusterEndpoint, 0, b.clusterEndpoints.Len())
	for _, ep := range b.clusterEndpoints.All() {
		// DBClusterIdentifier is a case-insensitive AWS identifier (see
		// normalizeID) even though DBClusterEndpointIdentifier -- this
		// table's own primary key -- is out of this fix's scope.
		if clusterID != "" && !idEqual(ep.DBClusterIdentifier, clusterID) {
			continue
		}
		if endpointID != "" && ep.DBClusterEndpointIdentifier != endpointID {
			continue
		}
		result = append(result, *ep)
	}

	return result, nil
}

// DeleteDBClusterEndpoint removes the given custom cluster endpoint.
func (b *InMemoryBackend) DeleteDBClusterEndpoint(endpointID string) (*DBClusterEndpoint, error) {
	b.mu.Lock("DeleteDBClusterEndpoint")
	defer b.mu.Unlock()
	ep, exists := b.clusterEndpoints.Get(endpointID)
	if !exists {
		return nil, fmt.Errorf("%w: cluster endpoint %s not found", ErrClusterEndpointNotFound, endpointID)
	}
	cp := *ep
	b.clusterEndpoints.Delete(endpointID)
	delete(b.tags, b.rdsARN("cluster-endpoint", endpointID))

	return &cp, nil
}

// ModifyDBClusterEndpoint modifies a custom DB cluster endpoint.
func (b *InMemoryBackend) ModifyDBClusterEndpoint(endpointID, endpointType string) (*DBClusterEndpoint, error) {
	b.mu.Lock("ModifyDBClusterEndpoint")
	defer b.mu.Unlock()
	ep, exists := b.clusterEndpoints.Get(endpointID)
	if !exists {
		return nil, fmt.Errorf("%w: cluster endpoint %s not found", ErrClusterEndpointNotFound, endpointID)
	}
	if endpointType != "" {
		ep.EndpointType = endpointType
	}
	cp := *ep

	return &cp, nil
}
