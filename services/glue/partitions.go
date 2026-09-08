package glue

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// maxBatchCreatePartitions is the maximum number of partitions per BatchCreatePartition call.
const maxBatchCreatePartitions = 100

// partitionKey returns a map key for a partition.
func partitionKey(dbName, tableName string, values []string) string {
	return fmt.Sprintf("%s|%s|%s", dbName, tableName, strings.Join(values, "#"))
}

// deleteTablePartitionsLocked removes all partitions, versions, column
// statistics, and table optimizers for a table. Must be called with b.mu
// held for writing.
func (b *InMemoryBackend) deleteTablePartitionsLocked(dbName, tableName string) {
	prefix := dbName + "|" + tableName + "|"

	for _, p := range b.partitions.Snapshot() {
		if k := partitionKey(p.DatabaseName, p.TableName, p.Values); strings.HasPrefix(k, prefix) {
			b.partitions.Delete(k)
		}
	}

	for k := range b.partitionIndexes {
		if strings.HasPrefix(k, prefix) {
			delete(b.partitionIndexes, k)
		}
	}

	for _, tv := range b.tableVersions.Snapshot() {
		if k := tableVersionEntryKeyFn(tv); strings.HasPrefix(k, prefix) {
			b.tableVersions.Delete(k)
		}
	}

	for k := range b.tableColumnStats {
		if strings.HasPrefix(k, prefix) {
			delete(b.tableColumnStats, k)
		}
	}

	for k := range b.partitionColumnStats {
		if strings.HasPrefix(k, prefix) {
			delete(b.partitionColumnStats, k)
		}
	}

	for _, rec := range b.tableOptimizers.Snapshot() {
		if k := tableOptimizerEntryKeyFn(rec); strings.HasPrefix(k, prefix) {
			b.tableOptimizers.Delete(k)
		}
	}
}

// BatchCreatePartition creates multiple partitions for a table.
func (b *InMemoryBackend) BatchCreatePartition(
	dbName, tableName string,
	inputs []PartitionInput,
) ([]*Partition, []PartitionError) {
	b.mu.Lock("BatchCreatePartition")
	defer b.mu.Unlock()

	created := make([]*Partition, 0, len(inputs))
	errs := make([]PartitionError, 0, len(inputs))

	// AWS's BatchCreatePartition (and the single-partition CreatePartition, which
	// is implemented in terms of this method) reject partitions for a table that
	// does not exist with EntityNotFoundException. This was previously
	// unchecked, so CreatePartition/BatchCreatePartition against a nonexistent
	// database/table silently "succeeded" and stored orphaned partitions with no
	// owning table — a disguised stub.
	if !b.tables.Has(tableKey(dbName, tableName)) {
		for _, input := range inputs {
			errs = append(errs, PartitionError{
				PartitionValues: input.Values,
				ErrorDetail: ErrorDetail{
					ErrorCode:    errEntityNotFoundCode,
					ErrorMessage: fmt.Sprintf("table %s.%s not found", dbName, tableName),
				},
			})
		}

		return created, errs
	}

	now := float64(time.Now().Unix())

	for _, input := range inputs {
		key := partitionKey(dbName, tableName, input.Values)
		if b.partitions.Has(key) {
			errs = append(errs, PartitionError{
				PartitionValues: input.Values,
				ErrorDetail: ErrorDetail{
					ErrorCode:    "AlreadyExistsException",
					ErrorMessage: "partition already exists",
				},
			})

			continue
		}

		p := &Partition{
			DatabaseName:      dbName,
			TableName:         tableName,
			CatalogID:         b.accountID,
			Values:            append([]string(nil), input.Values...),
			StorageDescriptor: input.StorageDescriptor,
			Parameters:        maps.Clone(input.Parameters),
			CreationTime:      now,
		}
		b.partitions.Put(p)
		created = append(created, p)
	}

	return created, errs
}

// BatchDeletePartition deletes multiple partitions for a table.
func (b *InMemoryBackend) BatchDeletePartition(
	dbName, tableName string,
	values []PartitionValueList,
) []PartitionError {
	b.mu.Lock("BatchDeletePartition")
	defer b.mu.Unlock()

	errs := make([]PartitionError, 0, len(values))

	for _, pvl := range values {
		key := partitionKey(dbName, tableName, pvl.Values)
		if !b.partitions.Has(key) {
			errs = append(errs, PartitionError{
				PartitionValues: pvl.Values,
				ErrorDetail: ErrorDetail{
					ErrorCode:    errEntityNotFoundCode,
					ErrorMessage: "partition not found",
				},
			})

			continue
		}

		b.partitions.Delete(key)
		b.deletePartitionColumnStatsLocked(key)
	}

	return errs
}

// deletePartitionColumnStatsLocked removes every column-statistics entry for
// the partition identified by partKey (as built by partitionKey). Must be
// called with b.mu held for writing.
func (b *InMemoryBackend) deletePartitionColumnStatsLocked(partKey string) {
	prefix := partKey + "|"
	for k := range b.partitionColumnStats {
		if strings.HasPrefix(k, prefix) {
			delete(b.partitionColumnStats, k)
		}
	}
}

// AddPartitionInternal adds a partition directly to the backend without
// validation. dbName/tableName are stamped onto the stored copy (rather than
// trusted from the caller-supplied p, which the existing test-seed callers
// often leave zero-valued) so the store.Table key -- derived purely from the
// value via partitionEntryKeyFn -- matches the dbName/tableName this entry is
// filed under, exactly as the previous raw map (keyed externally on the same
// two parameters) did.
func (b *InMemoryBackend) AddPartitionInternal(dbName, tableName string, p *Partition) {
	b.mu.Lock("AddPartitionInternal")
	defer b.mu.Unlock()

	cp := *p
	cp.DatabaseName = dbName
	cp.TableName = tableName
	cp.Values = append([]string(nil), p.Values...)
	b.partitions.Put(&cp)
}

// GetPartition retrieves a single partition by its values.
func (b *InMemoryBackend) GetPartition(dbName, tableName string, values []string) (*Partition, error) {
	b.mu.RLock("GetPartition")
	defer b.mu.RUnlock()

	key := partitionKey(dbName, tableName, values)
	p, ok := b.partitions.Get(key)
	if !ok {
		return nil, ErrNotFound
	}

	cp := clonePartition(p)

	return cp, nil
}

// GetPartitions returns all partitions for a table, sorted by key.
func (b *InMemoryBackend) GetPartitions(dbName, tableName string) ([]*Partition, error) {
	b.mu.RLock("GetPartitions")
	defer b.mu.RUnlock()

	if !b.tables.Has(tableKey(dbName, tableName)) {
		return nil, ErrNotFound
	}

	group := b.partitionsByTable.Get(tableKey(dbName, tableName))
	out := make([]*Partition, 0, len(group))

	for _, p := range group {
		out = append(out, clonePartition(p))
	}

	slices.SortFunc(out, func(a, c *Partition) int {
		return cmp.Compare(partitionEntryKeyFn(a), partitionEntryKeyFn(c))
	})

	return out, nil
}

// UpdatePartition updates an existing partition's storage descriptor and optionally renames it.
func (b *InMemoryBackend) UpdatePartition(
	dbName, tableName string,
	partitionValues []string,
	input PartitionInput,
) error {
	b.mu.Lock("UpdatePartition")
	defer b.mu.Unlock()

	return b.updatePartitionLocked(dbName, tableName, partitionValues, input)
}

// updatePartitionLocked performs the update while the caller holds the write lock.
func (b *InMemoryBackend) updatePartitionLocked(
	dbName, tableName string,
	partitionValues []string,
	input PartitionInput,
) error {
	oldKey := partitionKey(dbName, tableName, partitionValues)
	p, ok := b.partitions.Get(oldKey)
	if !ok {
		return ErrNotFound
	}

	if len(input.Values) > 0 && !stringSliceEqual(input.Values, partitionValues) {
		return b.renamePartitionLocked(p, dbName, tableName, oldKey, input)
	}

	p.StorageDescriptor = input.StorageDescriptor
	p.Parameters = maps.Clone(input.Parameters)

	return nil
}

// renamePartitionLocked moves a partition to a new key based on new values.
// It deletes the old entry and re-inserts p under its new key (rather than
// mutating p.Values in place and leaving it under oldKey) because
// store.Table derives a value's key from its own fields via
// partitionEntryKeyFn -- mutating the identity field without re-Put would
// leave the table indexed under a stale key for this value.
func (b *InMemoryBackend) renamePartitionLocked(
	p *Partition, dbName, tableName, oldKey string, input PartitionInput,
) error {
	newKey := partitionKey(dbName, tableName, input.Values)
	if b.partitions.Has(newKey) {
		return ErrAlreadyExists
	}

	b.partitions.Delete(oldKey)
	b.deletePartitionColumnStatsLocked(oldKey)
	p.Values = append([]string(nil), input.Values...)
	p.StorageDescriptor = input.StorageDescriptor
	p.Parameters = maps.Clone(input.Parameters)
	b.partitions.Put(p)

	return nil
}

// clonePartition returns a deep copy of a Partition.
func clonePartition(p *Partition) *Partition {
	cp := *p
	cp.Values = append([]string(nil), p.Values...)
	cp.StorageDescriptor = cloneStorageDescriptor(p.StorageDescriptor)
	cp.Parameters = maps.Clone(p.Parameters)

	return &cp
}

// stringSliceEqual reports whether two string slices are equal.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
