package apprunner

import (
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateConnection creates a new App Runner connection.
func (b *InMemoryBackend) CreateConnection(name, providerType string, tags map[string]string) (*Connection, error) {
	b.mu.Lock("CreateConnection")
	defer b.mu.Unlock()

	if existing := b.connByName.Get(name); len(existing) > 0 {
		return nil, fmt.Errorf("connection %s already exists: %w", name, ErrAlreadyExists)
	}

	id := newID()
	connArn := b.connectionARN(name, id)
	now := time.Now().UTC()

	conn := &storedConnection{
		ConnectionArn:  connArn,
		ConnectionName: name,
		ProviderType:   providerType,
		Status:         connStatusAvailable,
		CreatedAt:      now,
	}

	b.connections.Put(conn)

	if len(tags) > 0 {
		b.tags[connArn] = make(map[string]string)
		maps.Copy(b.tags[connArn], tags)
	}

	cp := conn.toConnection()

	return &cp, nil
}

// DeleteConnection deletes a connection.
func (b *InMemoryBackend) DeleteConnection(connArn string) (*Connection, error) {
	b.mu.Lock("DeleteConnection")
	defer b.mu.Unlock()

	conn, ok := b.connections.Get(connArn)
	if !ok {
		return nil, fmt.Errorf("connection %s not found: %w", connArn, ErrNotFound)
	}

	// DeleteConnection doc (api_op_DeleteConnection.go): "You must first
	// ensure that there are no running App Runner services that use this
	// connection. If there are any, the DeleteConnection action fails."
	if b.serviceUsesConnection(connArn) {
		return nil, fmt.Errorf("connection %s is used by one or more services: %w", connArn, ErrInvalidParameter)
	}

	conn.Status = connStatusDeleted
	cp := conn.toConnection()

	b.connections.Delete(connArn)
	delete(b.tags, connArn)

	return &cp, nil
}

// ListConnections returns connections with optional name filter.
func (b *InMemoryBackend) ListConnections(
	nameFilter string,
	maxResults int32,
	nextToken string,
) ([]*ConnectionSummary, string, error) {
	b.mu.RLock("ListConnections")
	defer b.mu.RUnlock()

	items := b.connections.Snapshot()

	all := make([]*ConnectionSummary, 0, len(items))
	for _, conn := range items {
		if nameFilter != "" && conn.ConnectionName != nameFilter {
			continue
		}
		s := conn.toSummary()
		all = append(all, &s)
	}

	limit := int(maxResults)
	pg := page.New(all, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}
