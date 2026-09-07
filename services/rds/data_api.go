package rds

import "fmt"

// EnableHTTPEndpoint enables the HTTP endpoint for an Aurora Serverless cluster.
func (b *InMemoryBackend) EnableHTTPEndpoint(resourceARN string) error {
	b.mu.Lock("EnableHTTPEndpoint")
	defer b.mu.Unlock()
	for _, cluster := range b.clusters.All() {
		if cluster.DBClusterIdentifier == resourceARN ||
			b.rdsARN("cluster", cluster.DBClusterIdentifier) == resourceARN {
			cluster.HTTPEndpointEnabled = true

			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrResourceNotFound, resourceARN)
}

// DisableHTTPEndpoint disables the HTTP endpoint for an Aurora Serverless cluster.
func (b *InMemoryBackend) DisableHTTPEndpoint(resourceARN string) error {
	b.mu.Lock("DisableHTTPEndpoint")
	defer b.mu.Unlock()
	for _, cluster := range b.clusters.All() {
		if cluster.DBClusterIdentifier == resourceARN ||
			b.rdsARN("cluster", cluster.DBClusterIdentifier) == resourceARN {
			cluster.HTTPEndpointEnabled = false

			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrResourceNotFound, resourceARN)
}
