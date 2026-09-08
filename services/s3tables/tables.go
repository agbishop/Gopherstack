package s3tables

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// PutTableReplication sets the replication configuration for a table. When
// expectedVersionToken is non-empty it must match the table's currently
// stored VersionToken (optimistic concurrency, mirroring
// PutTableReplicationInput's optional versionToken query parameter); a
// mismatch, or a token supplied when no configuration yet exists, returns
// ErrTableVersionConflict. Returns the stored config (with a freshly minted
// VersionToken) so the caller can build the {status, versionToken} response
// PutTableReplicationOutput requires.
func (b *InMemoryBackend) PutTableReplication(
	tableArn, role string,
	rules []ReplicationRule,
	expectedVersionToken string,
) (*TableReplicationConfig, error) {
	b.muTables.RLock("PutTableReplication")
	defer b.muTables.RUnlock()

	b.muState.Lock("PutTableReplication")
	defer b.muState.Unlock()

	if !b.tables.Has(tableArn) {
		return nil, fmt.Errorf("%w: table %q not found", ErrTableNotFound, tableArn)
	}

	existing, hasExisting := b.tableReplication.Get(tableArn)
	if expectedVersionToken != "" && (!hasExisting || existing.VersionToken != expectedVersionToken) {
		return nil, fmt.Errorf(
			"%w: stale version token for table replication %q",
			ErrTableVersionConflict,
			tableArn,
		)
	}

	cfg := &TableReplicationConfig{
		TableARN:     tableArn,
		Role:         role,
		Rules:        rules,
		VersionToken: uuid.NewString(),
	}
	b.tableReplication.Put(cfg)

	return cfg, nil
}

// DeleteTableReplication removes the replication configuration for a table.
// versionToken must match the stored VersionToken (mirroring
// DeleteTableReplicationInput, where versionToken is a required field); a
// mismatch returns ErrTableVersionConflict.
func (b *InMemoryBackend) DeleteTableReplication(tableArn, versionToken string) error {
	b.muTables.RLock("DeleteTableReplication")
	defer b.muTables.RUnlock()

	b.muState.Lock("DeleteTableReplication")
	defer b.muState.Unlock()

	if !b.tables.Has(tableArn) {
		return fmt.Errorf("%w: table %q not found", ErrTableNotFound, tableArn)
	}

	existing, hasExisting := b.tableReplication.Get(tableArn)
	if !hasExisting {
		return fmt.Errorf(
			"%w: no replication configuration for table %q",
			ErrTableNotFound,
			tableArn,
		)
	}

	if existing.VersionToken != versionToken {
		return fmt.Errorf(
			"%w: stale version token for table replication %q",
			ErrTableVersionConflict,
			tableArn,
		)
	}

	b.tableReplication.Delete(tableArn)

	return nil
}

// PutTableRecordExpirationConfiguration sets record expiration config for a table.
func (b *InMemoryBackend) PutTableRecordExpirationConfiguration(
	tableArn string,
	cfg *TableRecordExpiryConfig,
) error {
	b.muTables.RLock("PutTableRecordExpirationConfiguration")
	defer b.muTables.RUnlock()

	b.muState.Lock("PutTableRecordExpirationConfiguration")
	defer b.muState.Unlock()

	if !b.tables.Has(tableArn) {
		return fmt.Errorf("%w: table %q not found", ErrTableNotFound, tableArn)
	}

	cfg.TableARN = tableArn
	b.tableRecordExpiry.Put(cfg)

	return nil
}

// GetTableRecordExpirationConfiguration returns record expiry config for a
// table, defaulting to the "disabled" TableRecordExpirationStatus wire value
// when no configuration has been set.
func (b *InMemoryBackend) GetTableRecordExpirationConfiguration(
	tableArn string,
) (*TableRecordExpiryConfig, error) {
	b.muTables.RLock("GetTableRecordExpirationConfiguration")
	defer b.muTables.RUnlock()

	b.muState.RLock("GetTableRecordExpirationConfiguration")
	defer b.muState.RUnlock()

	if !b.tables.Has(tableArn) {
		return nil, fmt.Errorf("%w: table %q not found", ErrTableNotFound, tableArn)
	}

	cfg, ok := b.tableRecordExpiry.Get(tableArn)
	if !ok {
		return &TableRecordExpiryConfig{Status: recordExpirationStatusDisabled}, nil
	}

	return cfg, nil
}

// GetTableStorageClass returns the storage class for a table.
func (b *InMemoryBackend) GetTableStorageClass(
	bucketARN string,
	namespace []string,
	name string,
) (string, error) {
	b.muTables.RLock("GetTableStorageClass")
	defer b.muTables.RUnlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(bucketARN, nsStr, name)
	if !ok {
		return "", fmt.Errorf(
			"%w: table %q not found in namespace %s",
			ErrTableNotFound,
			name,
			nsStr,
		)
	}

	sc := t.StorageClass
	if sc == "" {
		sc = storageClassStandard
	}

	return sc, nil
}

// GetTableReplicationConfig returns the replication config for a table.
func (b *InMemoryBackend) GetTableReplicationConfig(tableArn string) (*TableReplicationConfig, error) {
	b.muTables.RLock("GetTableReplicationConfig")
	defer b.muTables.RUnlock()

	b.muState.RLock("GetTableReplicationConfig")
	defer b.muState.RUnlock()

	if !b.tables.Has(tableArn) {
		return nil, fmt.Errorf("%w: table %q not found", ErrTableNotFound, tableArn)
	}

	cfg, ok := b.tableReplication.Get(tableArn)
	if !ok {
		return nil, fmt.Errorf(
			"%w: no replication configuration for table %q",
			ErrTableNotFound,
			tableArn,
		)
	}

	return cfg, nil
}

// ValidateTableExists checks that a table ARN exists in the backend.
func (b *InMemoryBackend) ValidateTableExists(tableArn string) error {
	b.muTables.RLock("ValidateTableExists")
	defer b.muTables.RUnlock()

	if !b.tables.Has(tableArn) {
		return fmt.Errorf("%w: table %q not found", ErrTableNotFound, tableArn)
	}

	return nil
}

// CreateTable creates a new table within a namespace.
func (b *InMemoryBackend) CreateTable(
	tableBucketARN string,
	namespace []string,
	name, format string,
	opts CreateTableOptions,
) (*Table, error) {
	if err := validateTableOrNamespaceName(name); err != nil {
		return nil, err
	}

	b.muBuckets.RLock("CreateTable")
	defer b.muBuckets.RUnlock()

	b.muNamespaces.RLock("CreateTable")
	defer b.muNamespaces.RUnlock()

	b.muTables.Lock("CreateTable")
	defer b.muTables.Unlock()

	tb, ok := b.tableBuckets.Get(tableBucketARN)
	if !ok {
		return nil, fmt.Errorf(
			"%w: table bucket %q not found",
			ErrTableBucketNotFound,
			tableBucketARN,
		)
	}

	nsStr := joinNamespace(namespace)
	nsKey := namespaceKey(tableBucketARN, nsStr)

	if !b.namespaces.Has(nsKey) {
		return nil, fmt.Errorf(
			"%w: namespace %q not found in bucket %s",
			ErrNamespaceNotFound,
			nsStr,
			tableBucketARN,
		)
	}

	if _, exists := b.tableByComposite(tableBucketARN, nsStr, name); exists {
		return nil, fmt.Errorf(
			"%w: table %q already exists in namespace %s",
			ErrTableAlreadyExists,
			name,
			nsStr,
		)
	}

	tableARN := b.TableARN(tb.Name, nsStr, name)

	storageClass := opts.StorageClass

	now := time.Now().UTC()
	table := &Table{
		ARN:               tableARN,
		Name:              name,
		Namespace:         cloneStringSlice(namespace),
		TableBucketARN:    tableBucketARN,
		TableBucketID:     tb.BucketID,
		Format:            format,
		VersionToken:      uuid.NewString(),
		WarehouseLocation: "s3://" + tb.Name + "/" + nsStr + "/" + name,
		CreatedAt:         now,
		ModifiedAt:        now,
		OwnerAccountID:    b.accountID,
		StorageClass:      storageClass,
		Encryption:        cloneAnyMap(opts.Encryption),
		MaintenanceConfiguration: map[string]any{
			maintenanceTypeIcebergCompaction: map[string]any{
				keyStatusField: statusEnabled,
				keySettings: map[string]any{
					maintenanceTypeIcebergCompaction: map[string]any{
						"targetFileSizeMB": float64(512), //nolint:mnd // AWS default: 512 MB target file size
						"strategy":         "binpack",
					},
				},
			},
			maintenanceTypeIcebergSnapshotManagement: map[string]any{
				keyStatusField: statusEnabled,
				keySettings: map[string]any{
					maintenanceTypeIcebergSnapshotManagement: map[string]any{
						"maxSnapshotAgeHours": float64(120), //nolint:mnd // AWS default: 120 hours (5 days)
						"minSnapshotsToKeep":  float64(1),
					},
				},
			},
		},
	}
	b.tables.Put(table)

	// TagResource only takes muState, which sits after muTables in the
	// documented lock order (muBuckets -> muNamespaces -> muTables ->
	// muState), so acquiring it here while muBuckets/muNamespaces/muTables
	// are already held is safe.
	if len(opts.Tags) > 0 {
		_ = b.TagResource(tableARN, opts.Tags)
	}

	return cloneTable(table), nil
}

// GetTableByARN returns a table by its ARN directly, without needing the
// caller to know its bucket/namespace/name. Real GetTable accepts either
// tableArn alone or the tableBucketARN+namespace+name triple (see
// GetTableInput's optional TableArn field) -- this backs the former.
func (b *InMemoryBackend) GetTableByARN(tableArn string) (*Table, error) {
	b.muTables.RLock("GetTableByARN")
	defer b.muTables.RUnlock()

	t, ok := b.tables.Get(tableArn)
	if !ok {
		return nil, fmt.Errorf("%w: table %q not found", ErrTableNotFound, tableArn)
	}

	return cloneTable(t), nil
}

// GetTableEncryption returns the effective encryption configuration for a
// table: the table's own override if CreateTable set one, else the owning
// bucket's configuration, else the AWS default (SSE-S3/AES256). There is no
// PutTableEncryption operation for individual tables in the real API, so
// GetTableEncryption never returns NotFound the way GetTableBucketEncryption
// can -- every table has an effective encryption configuration.
func (b *InMemoryBackend) GetTableEncryption(
	tableBucketARN string,
	namespace []string,
	name string,
) (map[string]any, error) {
	b.muBuckets.RLock("GetTableEncryption")
	defer b.muBuckets.RUnlock()

	b.muTables.RLock("GetTableEncryption")
	defer b.muTables.RUnlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return nil, fmt.Errorf(
			"%w: table %q not found in namespace %s",
			ErrTableNotFound,
			name,
			nsStr,
		)
	}

	if t.Encryption != nil {
		return cloneAnyMap(t.Encryption), nil
	}

	if tb, tbOK := b.tableBuckets.Get(tableBucketARN); tbOK && tb.Encryption != nil {
		return cloneAnyMap(tb.Encryption), nil
	}

	return map[string]any{"sseAlgorithm": defaultSSEAlgorithm}, nil
}

// GetTable returns a table by bucket ARN, namespace, and name.
func (b *InMemoryBackend) GetTable(
	tableBucketARN string,
	namespace []string,
	name string,
) (*Table, error) {
	b.muTables.RLock("GetTable")
	defer b.muTables.RUnlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return nil, fmt.Errorf(
			"%w: table %q not found in namespace %s",
			ErrTableNotFound,
			name,
			nsStr,
		)
	}

	return cloneTable(t), nil
}

// DeleteTable deletes a table by bucket ARN, namespace, and name. When
// versionToken is non-empty it must match the table's currently stored
// VersionToken (optimistic concurrency, mirroring DeleteTableInput's
// optional versionToken query parameter); a mismatch returns
// ErrTableVersionConflict.
func (b *InMemoryBackend) DeleteTable(
	tableBucketARN string,
	namespace []string,
	name, versionToken string,
) error {
	b.muTables.Lock("DeleteTable")
	defer b.muTables.Unlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return fmt.Errorf("%w: table %q not found in namespace %s", ErrTableNotFound, name, nsStr)
	}

	if versionToken != "" && versionToken != t.VersionToken {
		return fmt.Errorf("%w: stale version token for table %q", ErrTableVersionConflict, name)
	}

	b.tables.Delete(t.ARN)

	return nil
}

// ListTables returns all tables in a table bucket, optionally filtered by namespace.
func (b *InMemoryBackend) ListTables(
	tableBucketARN, namespace string,
	p ListTablesParams,
) (page.Page[*Table], error) {
	if err := page.ValidateToken(p.ContinuationToken); err != nil {
		return page.Page[*Table]{}, fmt.Errorf(
			"%w: invalid continuationToken",
			ErrInvalidContinuationToken,
		)
	}

	b.muBuckets.RLock("ListTables")
	defer b.muBuckets.RUnlock()

	b.muTables.RLock("ListTables")
	defer b.muTables.RUnlock()

	if !b.tableBuckets.Has(tableBucketARN) {
		return page.Page[*Table]{}, fmt.Errorf(
			"%w: table bucket %q not found",
			ErrTableBucketNotFound,
			tableBucketARN,
		)
	}

	items := b.tablesByBucket.Get(tableBucketARN)
	list := make([]*Table, 0, len(items))

	for _, t := range items {
		if namespace != "" && joinNamespace(t.Namespace) != namespace {
			continue
		}

		if p.Prefix != "" && !strings.HasPrefix(t.Name, p.Prefix) {
			continue
		}

		list = append(list, cloneTable(t))
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})

	return page.New(list, p.ContinuationToken, p.MaxTables, s3tablesDefaultMaxTables), nil
}

// RenameTable renames a table or moves it to a different namespace.
func (b *InMemoryBackend) RenameTable(
	tableBucketARN string,
	namespace []string,
	name, newNamespace, newName, versionToken string,
) error {
	b.muBuckets.RLock("RenameTable")
	defer b.muBuckets.RUnlock()

	b.muNamespaces.RLock("RenameTable")
	defer b.muNamespaces.RUnlock()

	b.muTables.Lock("RenameTable")
	defer b.muTables.Unlock()

	nsStr := joinNamespace(namespace)

	found, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return fmt.Errorf("%w: table %q not found in namespace %s", ErrTableNotFound, name, nsStr)
	}

	if versionToken != "" && versionToken != found.VersionToken {
		return fmt.Errorf("%w: stale version token for table %q", ErrTableVersionConflict, name)
	}

	if newName == "" {
		newName = name
	}

	if newNamespace == "" {
		newNamespace = nsStr
	}

	if !b.namespaces.Has(namespaceKey(tableBucketARN, newNamespace)) {
		return fmt.Errorf(
			"%w: namespace %q not found in bucket %s",
			ErrNamespaceNotFound,
			newNamespace,
			tableBucketARN,
		)
	}

	tb, _ := b.tableBuckets.Get(tableBucketARN)
	newARN := b.TableARN(tb.Name, newNamespace, newName)

	if _, exists := b.tableByComposite(tableBucketARN, newNamespace, newName); exists {
		return fmt.Errorf(
			"%w: table %q already exists in namespace %s",
			ErrTableAlreadyExists,
			newName,
			newNamespace,
		)
	}

	// found.ARN (the primary key) and its composite/bucket index keys are
	// all about to change, so the old entry must be explicitly removed
	// before the mutated value is re-inserted -- Put alone would leave the
	// stale entry under the old ARN behind, since Put only knows how to
	// replace whatever is already stored at the NEW key it derives.
	b.tables.Delete(found.ARN)

	found.Name = newName
	found.Namespace = splitNamespace(newNamespace)
	found.ARN = newARN
	found.ModifiedAt = time.Now().UTC()
	found.VersionToken = uuid.NewString()

	b.tables.Put(found)

	return nil
}

// UpdateTableMetadataLocation updates the metadata location of a table.
func (b *InMemoryBackend) UpdateTableMetadataLocation(
	tableBucketARN string,
	namespace []string,
	name, metadataLocation, versionToken string,
) (*Table, error) {
	b.muTables.Lock("UpdateTableMetadataLocation")
	defer b.muTables.Unlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return nil, fmt.Errorf(
			"%w: table %q not found in namespace %s",
			ErrTableNotFound,
			name,
			nsStr,
		)
	}

	if versionToken != t.VersionToken {
		return nil, fmt.Errorf(
			"%w: stale version token for table %q",
			ErrTableVersionConflict,
			name,
		)
	}

	if !validMetadataLocation(t.WarehouseLocation, metadataLocation) {
		return nil, fmt.Errorf(
			"%w: metadata location %q is outside table warehouse or invalid",
			ErrInvalidTableMetadataLocation,
			metadataLocation,
		)
	}

	t.MetadataLocation = metadataLocation
	t.VersionToken = uuid.NewString()
	t.ModifiedAt = time.Now().UTC()

	return cloneTable(t), nil
}

func validMetadataLocation(_, metadataLocation string) bool {
	if !strings.HasPrefix(metadataLocation, "s3://") {
		return false
	}

	return strings.HasSuffix(metadataLocation, ".json") ||
		strings.HasSuffix(metadataLocation, ".json.gz")
}

// GetTableMaintenanceConfiguration returns the maintenance config for a table.
func (b *InMemoryBackend) GetTableMaintenanceConfiguration(
	tableBucketARN string,
	namespace []string,
	name string,
) (map[string]any, string, error) {
	b.muTables.RLock("GetTableMaintenanceConfiguration")
	defer b.muTables.RUnlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return nil, "", fmt.Errorf(
			"%w: table %q not found in namespace %s",
			ErrTableNotFound,
			name,
			nsStr,
		)
	}

	return cloneAnyMap(t.MaintenanceConfiguration), t.ARN, nil
}

// PutTableMaintenanceConfiguration sets maintenance config for a table.
func (b *InMemoryBackend) PutTableMaintenanceConfiguration(
	tableBucketARN string,
	namespace []string,
	name, maintenanceType string,
	value map[string]any,
) error {
	b.muTables.Lock("PutTableMaintenanceConfiguration")
	defer b.muTables.Unlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return fmt.Errorf("%w: table %q not found in namespace %s", ErrTableNotFound, name, nsStr)
	}

	if t.MaintenanceConfiguration == nil {
		t.MaintenanceConfiguration = make(map[string]any)
	}

	t.MaintenanceConfiguration[maintenanceType] = value

	return nil
}

// GetTablePolicy returns the resource policy for a table.
func (b *InMemoryBackend) GetTablePolicy(
	tableBucketARN string,
	namespace []string,
	name string,
) (string, error) {
	b.muTables.RLock("GetTablePolicy")
	defer b.muTables.RUnlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return "", fmt.Errorf(
			"%w: table %q not found in namespace %s",
			ErrTableNotFound,
			name,
			nsStr,
		)
	}

	if t.Policy == "" {
		return "", fmt.Errorf("%w: no policy for table %q", ErrTableNotFound, name)
	}

	return t.Policy, nil
}

// PutTablePolicy sets the resource policy for a table.
func (b *InMemoryBackend) PutTablePolicy(
	tableBucketARN string,
	namespace []string,
	name, policy string,
) error {
	b.muTables.Lock("PutTablePolicy")
	defer b.muTables.Unlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return fmt.Errorf("%w: table %q not found in namespace %s", ErrTableNotFound, name, nsStr)
	}

	t.Policy = policy

	return nil
}

// DeleteTablePolicy removes the resource policy from a table.
func (b *InMemoryBackend) DeleteTablePolicy(
	tableBucketARN string,
	namespace []string,
	name string,
) error {
	b.muTables.Lock("DeleteTablePolicy")
	defer b.muTables.Unlock()

	nsStr := joinNamespace(namespace)

	t, ok := b.tableByComposite(tableBucketARN, nsStr, name)
	if !ok {
		return fmt.Errorf("%w: table %q not found in namespace %s", ErrTableNotFound, name, nsStr)
	}

	t.Policy = ""

	return nil
}

func cloneTable(t *Table) *Table {
	cp := *t
	cp.Namespace = cloneStringSlice(t.Namespace)
	cp.MaintenanceConfiguration = cloneAnyMap(t.MaintenanceConfiguration)
	cp.Encryption = cloneAnyMap(t.Encryption)

	return &cp
}
