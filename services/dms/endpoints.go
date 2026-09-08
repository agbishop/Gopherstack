package dms

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// mustDescribeEndpoints returns all endpoints without error (for internal use).
func (b *InMemoryBackend) mustDescribeEndpoints(ctx context.Context) []*Endpoint {
	list, _ := b.DescribeEndpoints(ctx, "")

	return list
}

// EndpointConnectionSettings carries the top-level (non-engine-specific)
// connection settings CreateEndpoint/ModifyEndpoint accept beyond the
// original identifier/engine/server/database/username/password/port set --
// see api_op_CreateEndpoint.go / api_op_ModifyEndpoint.go,
// databasemigrationservice@v1.66.4. KmsKeyID is create-only (real
// ModifyEndpointInput has no KmsKeyId member -- encryption key can't change
// post-creation) and is ignored by ModifyEndpoint.
type EndpointConnectionSettings struct {
	CertificateArn            string
	ExtraConnectionAttributes string
	KmsKeyID                  string
	ServiceAccessRoleArn      string
	SslMode                   string
	ExternalTableDefinition   string
}

// CreateEndpoint creates a new DMS endpoint.
func (b *InMemoryBackend) CreateEndpoint(
	ctx context.Context,
	identifier, endpointType, engineName, serverName, databaseName, username, password string,
	port int32,
	kv map[string]string,
	settings EndpointConnectionSettings,
) (*Endpoint, error) {
	b.mu.Lock("CreateEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if b.endpoints.Has(regionKey(region, identifier)) {
		return nil, fmt.Errorf("%w: endpoint %s already exists", ErrAlreadyExists, identifier)
	}

	endpointID := uuid.NewString()
	endpointARN := arn.Build("dms", region, b.accountID, "endpoint:"+endpointID)
	t := tags.New("dms.endpoint." + identifier + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}

	if settings.SslMode == "" {
		settings.SslMode = "none"
	}

	ep := &Endpoint{
		EndpointIdentifier:        identifier,
		EndpointArn:               endpointARN,
		EndpointType:              endpointType,
		EngineName:                engineName,
		ServerName:                serverName,
		DatabaseName:              databaseName,
		Username:                  username,
		Password:                  password,
		Port:                      port,
		Status:                    statusActive,
		AccountID:                 b.accountID,
		Region:                    region,
		CreationTime:              time.Now().UTC(),
		Tags:                      t,
		CertificateArn:            settings.CertificateArn,
		ExtraConnectionAttributes: settings.ExtraConnectionAttributes,
		KmsKeyID:                  settings.KmsKeyID,
		ServiceAccessRoleArn:      settings.ServiceAccessRoleArn,
		SslMode:                   settings.SslMode,
		ExternalTableDefinition:   settings.ExternalTableDefinition,
	}
	b.endpoints.Put(ep)
	b.appendEvent(
		region, endpointARN, "replication-instance",
		"Endpoint "+identifier+" created", []string{eventCategoryCreation},
	)
	cp := *ep

	return &cp, nil
}

// DescribeEndpoints returns endpoints, optionally filtered by identifier or ARN.
func (b *InMemoryBackend) DescribeEndpoints(ctx context.Context, identifierOrArn string) ([]*Endpoint, error) {
	b.mu.RLock("DescribeEndpoints")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	return describeByIdentifierOrARN(b.endpoints, b.endpointsByARN, b.endpointsByRegion, region, identifierOrArn), nil
}

// DeleteEndpoint deletes an endpoint by ARN or identifier.
// Real AWS rejects deletion if the endpoint is still referenced by any replication task.
func (b *InMemoryBackend) DeleteEndpoint(ctx context.Context, arnOrID string) (*Endpoint, error) {
	b.mu.Lock("DeleteEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	deleteEndpoint := func(ep *Endpoint, id string) (*Endpoint, error) {
		// O(1) check using tasksByEndpointARN index (#performance).
		if tasks := b.tasksByEndpointARN[ep.EndpointArn]; len(tasks) > 0 {
			for taskARN := range tasks {
				taskID := taskARN
				if rt, ok := lookupUnique(b.replicationTasksByARN, regionKey(region, taskARN)); ok {
					taskID = rt.ReplicationTaskIdentifier
				}

				return nil, fmt.Errorf(
					"%w: endpoint %s is in use by replication task %s; delete the task first",
					ErrInvalidState,
					arnOrID,
					taskID,
				)
			}
		}
		cp := *ep
		ep.Tags.Close()
		b.endpoints.Delete(regionKey(region, id))
		delete(b.endpointSchemasStore(region), ep.EndpointArn)
		b.appendEvent(
			region, ep.EndpointArn, "replication-instance",
			"Endpoint "+id+" deleted", []string{eventCategoryDeletion},
		)

		return &cp, nil
	}

	// Try by identifier first.
	if ep, ok := b.endpoints.Get(regionKey(region, arnOrID)); ok {
		return deleteEndpoint(ep, arnOrID)
	}
	// Try by ARN index.
	if ep, ok := lookupUnique(b.endpointsByARN, regionKey(region, arnOrID)); ok {
		return deleteEndpoint(ep, ep.EndpointIdentifier)
	}

	return nil, fmt.Errorf("%w: endpoint %s not found", ErrNotFound, arnOrID)
}

// endpointSchemasStore returns the per-region inner map, lazily creating it.
// endpointSchemas is deliberately left a plain map (not converted to
// store.Table) because its values are []string, not *T -- see
// store_setup.go's registerAllTables doc comment. Callers must hold b.mu.
func (b *InMemoryBackend) endpointSchemasStore(region string) map[string][]string {
	if b.endpointSchemas[region] == nil {
		b.endpointSchemas[region] = make(map[string][]string)
	}

	return b.endpointSchemas[region]
}

// endpointSchemasStoreRO returns the region-scoped endpointSchemas map for
// region without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) endpointSchemasStoreRO(region string) map[string][]string {
	if v := b.endpointSchemas[region]; v != nil {
		return v
	}

	return map[string][]string{}
}

// DescribeSchemas returns the schema names available on an endpoint.
func (b *InMemoryBackend) DescribeSchemas(ctx context.Context, endpointARN string) ([]string, error) {
	b.mu.RLock("DescribeSchemas")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	if _, ok := lookupUnique(b.endpointsByARN, regionKey(region, endpointARN)); !ok {
		return nil, fmt.Errorf("%w: endpoint %s not found", ErrNotFound, endpointARN)
	}

	schemas := b.endpointSchemasStoreRO(region)[endpointARN]

	if schemas == nil {
		return []string{}, nil
	}

	result := make([]string, len(schemas))
	copy(result, schemas)

	return result, nil
}

// RefreshSchemas seeds schema discovery for an endpoint (emulates async refresh).
func (b *InMemoryBackend) RefreshSchemas(ctx context.Context, endpointARN string) error {
	b.mu.Lock("RefreshSchemas")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	ep, ok := lookupUnique(b.endpointsByARN, regionKey(region, endpointARN))

	if !ok {
		return fmt.Errorf("%w: endpoint %s not found", ErrNotFound, endpointARN)
	}

	b.endpointSchemasStore(region)[endpointARN] = defaultSchemasForEngine(ep.EngineName)

	return nil
}

// defaultSchemasForEngine returns a realistic default schema list for a given engine.
func defaultSchemasForEngine(engine string) []string {
	switch engine {
	case engineNamePostgres, engineNameAuroraPostgreSQL:
		return []string{"public", "information_schema", "pg_catalog"}
	case engineNameOracle:
		return []string{"SYS", "SYSTEM", "HR", "OE"}
	case engineNameSQLServer:
		return []string{"dbo", "sys", "INFORMATION_SCHEMA"}
	default:
		return []string{"main", "information_schema"}
	}
}

// AddEndpointInternal seeds an endpoint directly without HTTP.
func (b *InMemoryBackend) AddEndpointInternal(identifier, endpointType, engineName string) {
	b.mu.Lock("AddEndpointInternal")
	defer b.mu.Unlock()
	epID := uuid.NewString()
	epARN := arn.Build("dms", b.region, b.accountID, "endpoint:"+epID)
	t := tags.New("dms.endpoint." + identifier + ".tags")
	ep := &Endpoint{
		EndpointIdentifier: identifier,
		EndpointArn:        epARN,
		EndpointType:       endpointType,
		EngineName:         engineName,
		Status:             statusActive,
		AccountID:          b.accountID,
		Region:             b.region,
		CreationTime:       time.Now().UTC(),
		Tags:               t,
	}
	b.endpoints.Put(ep)
}

// ModifyEndpoint updates endpoint settings.
func (b *InMemoryBackend) ModifyEndpoint(
	ctx context.Context,
	arnOrID, endpointType, engineName, serverName, databaseName, username, password string,
	port int32,
	settings EndpointConnectionSettings,
) (*Endpoint, error) {
	b.mu.Lock("ModifyEndpoint")
	defer b.mu.Unlock()

	ep := b.findEndpoint(ctx, arnOrID)
	if ep == nil {
		return nil, fmt.Errorf("%w: endpoint %s not found", ErrNotFound, arnOrID)
	}

	if endpointType != "" {
		ep.EndpointType = endpointType
	}

	if engineName != "" {
		ep.EngineName = engineName
	}

	if serverName != "" {
		ep.ServerName = serverName
	}

	if databaseName != "" {
		ep.DatabaseName = databaseName
	}

	if username != "" {
		ep.Username = username
	}

	if password != "" {
		ep.Password = password
	}

	if port != 0 {
		ep.Port = port
	}

	if settings.CertificateArn != "" {
		ep.CertificateArn = settings.CertificateArn
	}

	if settings.ExtraConnectionAttributes != "" {
		ep.ExtraConnectionAttributes = settings.ExtraConnectionAttributes
	}

	if settings.ServiceAccessRoleArn != "" {
		ep.ServiceAccessRoleArn = settings.ServiceAccessRoleArn
	}

	if settings.SslMode != "" {
		ep.SslMode = settings.SslMode
	}

	if settings.ExternalTableDefinition != "" {
		ep.ExternalTableDefinition = settings.ExternalTableDefinition
	}

	cp := *ep

	return &cp, nil
}

// findEndpoint locates an endpoint by identifier or ARN within the request
// region (must hold a lock).
func (b *InMemoryBackend) findEndpoint(ctx context.Context, arnOrID string) *Endpoint {
	region := getRegion(ctx, b.region)
	if ep, ok := b.endpoints.Get(regionKey(region, arnOrID)); ok {
		return ep
	}

	if ep, ok := lookupUnique(b.endpointsByARN, regionKey(region, arnOrID)); ok {
		return ep
	}

	return nil
}
