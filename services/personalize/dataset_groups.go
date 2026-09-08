package personalize

import (
	"fmt"
	"time"
)

// --- DatasetGroup ---

// validDomain reports whether domain is a value the real types.Domain enum
// accepts. An empty domain is valid (it creates a Custom dataset group /
// schema rather than a Domain one) -- only a non-empty, unrecognized value is
// rejected. Shared with CreateSchema, which takes the same Domain-typed
// field.
func validDomain(domain string) bool {
	switch domain {
	case "", "ECOMMERCE", "VIDEO_ON_DEMAND":
		return true
	default:
		return false
	}
}

// CreateDatasetGroup creates a new dataset group.
func (b *InMemoryBackend) CreateDatasetGroup(
	name, domain, kmsKeyArn, roleArn string,
	tags map[string]string,
) (*DatasetGroup, error) {
	b.mu.Lock("CreateDatasetGroup")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if b.datasetGroups.Has(name) {
		return nil, fmt.Errorf("%w: dataset group %q already exists", ErrAlreadyExists, name)
	}
	if !validDomain(domain) {
		return nil, fmt.Errorf("%w: domain %q is invalid", ErrValidation, domain)
	}

	now := time.Now().UTC()
	dg := &DatasetGroup{
		DatasetGroupArn:     b.personalizeARN("dataset-group", name),
		Name:                name,
		Domain:              domain,
		KmsKeyArn:           kmsKeyArn,
		RoleArn:             roleArn,
		Status:              statusActive,
		CreationDateTime:    now,
		LastUpdatedDateTime: now,
	}
	b.datasetGroups.Put(dg)
	if len(tags) > 0 {
		b.tags[dg.DatasetGroupArn] = copyStringMap(tags)
	}

	return dg, nil
}

// DescribeDatasetGroup returns a dataset group by name or ARN.
func (b *InMemoryBackend) DescribeDatasetGroup(nameOrArn string) (*DatasetGroup, error) {
	b.mu.RLock("DescribeDatasetGroup")
	defer b.mu.RUnlock()

	if dg := b.findDatasetGroup(nameOrArn); dg != nil {
		return dg, nil
	}

	return nil, fmt.Errorf("%w: dataset group %q not found", ErrNotFound, nameOrArn)
}

// DeleteDatasetGroup removes a dataset group. Per api_op_DeleteDatasetGroup.go's
// doc comment, the caller must first delete all associated event trackers,
// solutions, and datasets.
func (b *InMemoryBackend) DeleteDatasetGroup(nameOrArn string) error {
	b.mu.Lock("DeleteDatasetGroup")
	defer b.mu.Unlock()

	dg := b.findDatasetGroup(nameOrArn)
	if dg == nil {
		return fmt.Errorf("%w: dataset group %q not found", ErrNotFound, nameOrArn)
	}
	for _, et := range b.eventTrackers.All() {
		if et.DatasetGroupArn == dg.DatasetGroupArn {
			return fmt.Errorf("%w: dataset group %q still has event trackers", ErrInUse, nameOrArn)
		}
	}
	for _, sol := range b.solutions.All() {
		if sol.DatasetGroupArn == dg.DatasetGroupArn {
			return fmt.Errorf("%w: dataset group %q still has solutions", ErrInUse, nameOrArn)
		}
	}
	for _, ds := range b.datasets.All() {
		if ds.DatasetGroupArn == dg.DatasetGroupArn {
			return fmt.Errorf("%w: dataset group %q still has datasets", ErrInUse, nameOrArn)
		}
	}
	b.datasetGroups.Delete(dg.Name)
	delete(b.tags, dg.DatasetGroupArn)

	return nil
}

// ListDatasetGroups returns all dataset groups.
func (b *InMemoryBackend) ListDatasetGroups(maxResults int, nextToken string) ([]*DatasetGroup, string) {
	b.mu.RLock("ListDatasetGroups")
	defer b.mu.RUnlock()

	return paginateItems(b.datasetGroups.Snapshot(), datasetGroupKeyFn, maxResults, nextToken)
}

func (b *InMemoryBackend) findDatasetGroup(nameOrArn string) *DatasetGroup {
	if dg, ok := b.datasetGroups.Get(nameOrArn); ok {
		return dg
	}
	for _, dg := range b.datasetGroups.All() {
		if dg.DatasetGroupArn == nameOrArn {
			return dg
		}
	}

	return nil
}
