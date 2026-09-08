package codestarconnections

import (
	"context"
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateConnection creates a new CodeStar connection.
//
// The name/ProviderType/tag validation below (ErrValidation, wire
// InvalidInputException) has no correct declared type to send: this op's
// own switch (codestarconnections@v1.38.4 deserializers.go
// deserializeOpErrorCreateConnection) is exactly [LimitExceededException,
// ResourceNotFoundException, ResourceUnavailableException] -- no
// InvalidInputException, and no ValidationException equivalent exists
// anywhere in this SDK module. Recorded, not fixed (gopherstack-6flj/uox6
// error-envelope sweep).
func (b *InMemoryBackend) CreateConnection(
	ctx context.Context,
	name, providerType, hostArn string,
	tags map[string]string,
) (*Connection, error) {
	if err := validateConnectionName(name); err != nil {
		return nil, err
	}

	if providerType != "" && !validProviderTypes()[providerType] {
		return nil, fmt.Errorf("%w: invalid ProviderType %q", ErrValidation, providerType)
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("CreateConnection")
	defer b.mu.Unlock()

	// A duplicate ConnectionName is NOT rejected: CreateConnection's real
	// error list is exactly [LimitExceededException, ResourceNotFoundException,
	// ResourceUnavailableException] (codestarconnections@v1.38.4
	// deserializers.go's awsAwsjson10_deserializeOpErrorCreateConnection
	// switch) -- no InvalidInputException/ResourceAlreadyExistsException case
	// for a name collision -- so a real client's second create for the same
	// name gets a distinct ARN, not an error (same behavior as sibling
	// codeconnections@v1.13.4, confirmed independently against its own
	// identical switch).

	// CreateConnection's real error list is [LimitExceededException,
	// ResourceNotFoundException, ResourceUnavailableException] (botocore
	// codestar-connections/2019-12-01/service-2.json) -- a HostArn referencing
	// a host that does not exist in the caller's region maps to
	// ResourceNotFoundException, the same real type GetHost/DeleteHost use
	// for a missing host. hosts is keyed directly by HostArn (which already
	// embeds its own region -- see store_setup.go), so no region-scoped
	// lookup is needed here.
	if hostArn != "" && !b.hosts.Has(hostArn) {
		return nil, fmt.Errorf("%w: host %q does not exist", ErrNotFound, hostArn)
	}

	id := uuid.NewString()
	connArn := arn.Build("codestar-connections", region, b.accountID, "connection/"+id)

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	conn := &Connection{
		ConnectionName:   name,
		ConnectionArn:    connArn,
		ConnectionStatus: ConnectionStatusAvailable,
		OwnerAccountID:   b.accountID,
		ProviderType:     providerType,
		HostArn:          hostArn,
		Tags:             tagsCopy,
	}
	b.connections.Put(conn)

	cp := *conn
	cp.Tags = make(map[string]string, len(conn.Tags))
	maps.Copy(cp.Tags, conn.Tags)

	return &cp, nil
}

// GetConnection returns a connection by ARN.
func (b *InMemoryBackend) GetConnection(_ context.Context, connectionArn string) (*Connection, error) {
	b.mu.RLock("GetConnection")
	defer b.mu.RUnlock()

	conn, ok := b.connections.Get(connectionArn)
	if !ok {
		return nil, fmt.Errorf("%w: connection not found: %s", ErrNotFound, connectionArn)
	}

	cp := *conn
	cp.Tags = make(map[string]string, len(conn.Tags))
	maps.Copy(cp.Tags, conn.Tags)

	return &cp, nil
}

// ListConnections returns all connections sorted by name, optionally filtered by provider type or host ARN.
func (b *InMemoryBackend) ListConnections(ctx context.Context, providerTypeFilter, hostArnFilter string) []*Connection {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("ListConnections")
	defer b.mu.RUnlock()

	conns := b.connectionsByRegion.Get(region)
	result := make([]*Connection, 0, len(conns))

	for _, conn := range conns {
		if providerTypeFilter != "" && conn.ProviderType != providerTypeFilter {
			continue
		}

		if hostArnFilter != "" && conn.HostArn != hostArnFilter {
			continue
		}

		cp := *conn
		cp.Tags = make(map[string]string, len(conn.Tags))
		maps.Copy(cp.Tags, conn.Tags)
		result = append(result, &cp)
	}

	// ConnectionName is not unique (CreateConnection has no
	// ResourceAlreadyExistsException for a duplicate name, see errors.go),
	// so ConnectionArn (always unique) breaks ties -- sort.Slice is not
	// stable, and without a tie-break two connections sharing a name have no
	// total order between them.
	sort.Slice(result, func(i, j int) bool {
		if result[i].ConnectionName != result[j].ConnectionName {
			return result[i].ConnectionName < result[j].ConnectionName
		}

		return result[i].ConnectionArn < result[j].ConnectionArn
	})

	return result
}

// DeleteConnection removes a connection by ARN.
func (b *InMemoryBackend) DeleteConnection(_ context.Context, connectionArn string) error {
	b.mu.Lock("DeleteConnection")
	defer b.mu.Unlock()

	if !b.connections.Has(connectionArn) {
		return fmt.Errorf("%w: connection not found: %s", ErrNotFound, connectionArn)
	}

	b.connections.Delete(connectionArn)

	return nil
}
