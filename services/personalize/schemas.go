package personalize

import (
	"fmt"
	"time"
)

// --- Schema ---

// CreateSchema creates a new schema.
func (b *InMemoryBackend) CreateSchema(name, schema, domain string) (*Schema, error) {
	b.mu.Lock("CreateSchema")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if b.schemas.Has(name) {
		return nil, fmt.Errorf("%w: schema %q already exists", ErrAlreadyExists, name)
	}
	if !validDomain(domain) {
		return nil, fmt.Errorf("%w: domain %q is invalid", ErrValidation, domain)
	}

	now := time.Now().UTC()
	s := &Schema{
		SchemaArn:           b.personalizeARN("schema", name),
		Name:                name,
		Schema:              schema,
		Domain:              domain,
		CreationDateTime:    now,
		LastUpdatedDateTime: now,
	}
	b.schemas.Put(s)

	return s, nil
}

// DescribeSchema returns a schema by name or ARN.
func (b *InMemoryBackend) DescribeSchema(nameOrArn string) (*Schema, error) {
	b.mu.RLock("DescribeSchema")
	defer b.mu.RUnlock()

	if s := b.findSchema(nameOrArn); s != nil {
		return s, nil
	}

	return nil, fmt.Errorf("%w: schema %q not found", ErrNotFound, nameOrArn)
}

// DeleteSchema removes a schema. Per api_op_DeleteSchema.go's doc comment,
// the caller must first delete all datasets referencing the schema.
func (b *InMemoryBackend) DeleteSchema(nameOrArn string) error {
	b.mu.Lock("DeleteSchema")
	defer b.mu.Unlock()

	s := b.findSchema(nameOrArn)
	if s == nil {
		return fmt.Errorf("%w: schema %q not found", ErrNotFound, nameOrArn)
	}
	for _, ds := range b.datasets.All() {
		if ds.SchemaArn == s.SchemaArn {
			return fmt.Errorf("%w: schema %q still has datasets referencing it", ErrInUse, nameOrArn)
		}
	}
	b.schemas.Delete(s.Name)

	return nil
}

// ListSchemas returns all schemas.
func (b *InMemoryBackend) ListSchemas(maxResults int, nextToken string) ([]*Schema, string) {
	b.mu.RLock("ListSchemas")
	defer b.mu.RUnlock()

	return paginateItems(b.schemas.Snapshot(), schemaKeyFn, maxResults, nextToken)
}

func (b *InMemoryBackend) findSchema(nameOrArn string) *Schema {
	if s, ok := b.schemas.Get(nameOrArn); ok {
		return s
	}
	for _, s := range b.schemas.All() {
		if s.SchemaArn == nameOrArn {
			return s
		}
	}

	return nil
}
