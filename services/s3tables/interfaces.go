package s3tables

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// StorageBackend defines the interface for S3 Tables backend implementations.
// All mutating methods must be safe for concurrent use.
type StorageBackend interface {
	// TableBucket operations
	TableBucketARN(name string) string
	TableARN(bucketName, namespaceName, tableName string) string
	Region() string
	AccountID() string
	CreateTableBucket(name string, opts CreateTableBucketOptions) (*TableBucket, error)
	GetTableBucket(bucketARN string) (*TableBucket, error)
	DeleteTableBucket(bucketARN string) error
	ListTableBuckets(p ListTableBucketsParams) (page.Page[*TableBucket], error)
	GetTableBucketMaintenanceConfiguration(bucketARN string) (map[string]any, error)
	PutTableBucketMaintenanceConfiguration(bucketARN, maintenanceType string, value map[string]any) error
	GetTableBucketPolicy(bucketARN string) (string, error)
	PutTableBucketPolicy(bucketARN, policy string) error
	DeleteTableBucketPolicy(bucketARN string) error

	DeleteTableBucketEncryption(bucketARN string) error

	// BucketReplication operations
	PutTableBucketReplication(
		bucketARN, role string,
		rules []ReplicationRule,
		expectedVersionToken string,
	) (*BucketReplicationConfig, error)
	GetTableBucketReplication(bucketARN string) (*BucketReplicationConfig, error)
	DeleteTableBucketReplication(bucketARN, expectedVersionToken string) error

	// TableReplication operations
	PutTableReplication(
		tableArn, role string,
		rules []ReplicationRule,
		expectedVersionToken string,
	) (*TableReplicationConfig, error)
	GetTableReplicationConfig(tableArn string) (*TableReplicationConfig, error)
	DeleteTableReplication(tableArn, versionToken string) error

	// TableRecordExpiry operations
	PutTableRecordExpirationConfiguration(tableArn string, cfg *TableRecordExpiryConfig) error
	GetTableRecordExpirationConfiguration(tableArn string) (*TableRecordExpiryConfig, error)

	// Namespace operations
	CreateNamespace(tableBucketARN string, namespace []string) (*Namespace, error)
	GetNamespace(tableBucketARN string, namespace []string) (*Namespace, error)
	DeleteNamespace(tableBucketARN string, namespace []string) error
	ListNamespaces(tableBucketARN string, p ListNamespacesParams) (page.Page[*Namespace], error)

	// Table operations
	CreateTable(tableBucketARN string, namespace []string, name, format string, opts CreateTableOptions) (*Table, error)
	GetTable(tableBucketARN string, namespace []string, name string) (*Table, error)
	GetTableByARN(tableArn string) (*Table, error)
	GetTableEncryption(tableBucketARN string, namespace []string, name string) (map[string]any, error)
	DeleteTable(tableBucketARN string, namespace []string, name, versionToken string) error
	ListTables(tableBucketARN, namespace string, p ListTablesParams) (page.Page[*Table], error)
	RenameTable(tableBucketARN string, namespace []string, name, newNamespace, newName, versionToken string) error
	UpdateTableMetadataLocation(
		tableBucketARN string,
		namespace []string,
		name, metadataLocation, versionToken string,
	) (*Table, error)
	GetTableMaintenanceConfiguration(
		tableBucketARN string,
		namespace []string,
		name string,
	) (map[string]any, string, error)
	PutTableMaintenanceConfiguration(
		tableBucketARN string,
		namespace []string,
		name, maintenanceType string,
		value map[string]any,
	) error
	GetTablePolicy(tableBucketARN string, namespace []string, name string) (string, error)
	PutTablePolicy(tableBucketARN string, namespace []string, name, policy string) error
	DeleteTablePolicy(tableBucketARN string, namespace []string, name string) error

	// Lifecycle
	Reset()
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
}

// compile-time assertion that InMemoryBackend satisfies StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)
