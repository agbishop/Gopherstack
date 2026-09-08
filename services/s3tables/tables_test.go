package s3tables_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/s3tables"
)

// TestBackend_CreateTable_NameValidation verifies CreateTable enforces real
// S3 Tables table naming rules (field-diffed against
// https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-tables-buckets-naming.html):
// 1-255 chars; lowercase letters, digits, underscores only; must begin with
// a letter or number; no hyphens/periods. Unlike namespaces, table names
// have no reserved "aws" prefix restriction.
func TestBackend_CreateTable_NameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantValid bool
	}{
		{name: "", wantValid: false},             // too short (< 1 char)
		{name: "Valid_table", wantValid: false},  // uppercase not allowed
		{name: "valid-table", wantValid: false},  // hyphen not allowed
		{name: "valid.table", wantValid: false},  // period not allowed
		{name: "_valid_table", wantValid: false}, // must begin with letter/number
		{name: "awsreserved", wantValid: true},   // "aws" prefix IS allowed for tables
		{name: "valid_table_123", wantValid: true},
		{name: "t", wantValid: true}, // exactly minimum length
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			tb, err := b.CreateTableBucket("tbl-validation-bucket", s3tables.CreateTableBucketOptions{})
			require.NoError(t, err)
			_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
			require.NoError(t, err)

			_, err = b.CreateTable(tb.ARN, []string{"ns1"}, tt.name, "ICEBERG", s3tables.CreateTableOptions{})

			if tt.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, s3tables.ErrInvalidName)
			}
		})
	}
}

func TestBackend_CreateTable_AppliesOptions(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	tb, err := b.CreateTableBucket("table-opts-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	table, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{
		Encryption: map[string]any{
			"sseAlgorithm": "aws:kms",
			"kmsKeyArn":    "arn:aws:kms:us-east-1:000000000000:key/tbl",
		},
		StorageClass: "INTELLIGENT_TIERING",
		Tags:         map[string]string{"team": "data"},
	})
	require.NoError(t, err)

	assert.Equal(t, "INTELLIGENT_TIERING", table.StorageClass)
	require.NotNil(t, table.Encryption)
	assert.Equal(t, "aws:kms", table.Encryption["sseAlgorithm"])

	tags, err := b.ListTagsForResource(table.ARN)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"team": "data"}, tags)
}

// ----------------------------------------
// GetTableByARN
// ----------------------------------------

func TestBackend_GetTableByARN(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	tb, err := b.CreateTableBucket("by-arn-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	created, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
	require.NoError(t, err)

	got, err := b.GetTableByARN(created.ARN)
	require.NoError(t, err)
	assert.Equal(t, created.Name, got.Name)
	assert.Equal(t, created.ARN, got.ARN)

	_, err = b.GetTableByARN("arn:aws:s3tables:us-east-1:000000000000:bucket/nope/table/ns1/nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, s3tables.ErrTableNotFound)
}

// ----------------------------------------
// GetTableEncryption inheritance: table override > bucket default > AES256
// ----------------------------------------

func TestBackend_GetTableEncryption_Inheritance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		bucketEnc map[string]any
		tableEnc  map[string]any
		wantSSE   string
	}{
		{
			name:    "no_config_anywhere_defaults_to_AES256",
			wantSSE: "AES256",
		},
		{
			name: "bucket_default_inherited_when_table_has_no_override",
			bucketEnc: map[string]any{
				"sseAlgorithm": "aws:kms",
				"kmsKeyArn":    "arn:aws:kms:us-east-1:000000000000:key/b",
			},
			wantSSE: "aws:kms",
		},
		{
			name: "table_override_wins_over_bucket_default",
			bucketEnc: map[string]any{
				"sseAlgorithm": "aws:kms",
				"kmsKeyArn":    "arn:aws:kms:us-east-1:000000000000:key/b",
			},
			tableEnc: map[string]any{"sseAlgorithm": "AES256"},
			wantSSE:  "AES256",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			bucketOpts := s3tables.CreateTableBucketOptions{}
			if tt.bucketEnc != nil {
				bucketOpts.Encryption = tt.bucketEnc
			}

			tb, err := b.CreateTableBucket("enc-inherit-"+bucketSuffix(tt.name), bucketOpts)
			require.NoError(t, err)

			_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
			require.NoError(t, err)

			tableOpts := s3tables.CreateTableOptions{}
			if tt.tableEnc != nil {
				tableOpts.Encryption = tt.tableEnc
			}

			_, err = b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", tableOpts)
			require.NoError(t, err)

			cfg, err := b.GetTableEncryption(tb.ARN, []string{"ns1"}, "t1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantSSE, cfg["sseAlgorithm"])
		})
	}
}

func TestBackend_GetTableEncryption_TableNotFound(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	tb, err := b.CreateTableBucket("enc-notfound-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.GetTableEncryption(tb.ARN, []string{"ns1"}, "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, s3tables.ErrTableNotFound)
}

// ----------------------------------------
// List pagination (ListTableBuckets, ListNamespaces, ListTables)
// ----------------------------------------

func TestBackend_ListTables_PaginationAndPrefix(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	tb, err := b.CreateTableBucket("tbl-page-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		_, err = b.CreateTable(tb.ARN, []string{"ns1"}, name, "ICEBERG", s3tables.CreateTableOptions{})
		require.NoError(t, err)
	}

	pg, err := b.ListTables(tb.ARN, "", s3tables.ListTablesParams{MaxTables: 1})
	require.NoError(t, err)
	require.Len(t, pg.Data, 1)
	require.NotEmpty(t, pg.Next)

	pg, err = b.ListTables(tb.ARN, "", s3tables.ListTablesParams{Prefix: "be"})
	require.NoError(t, err)
	require.Len(t, pg.Data, 1)
	assert.Equal(t, "beta", pg.Data[0].Name)
}

func TestBackend_TableReplicationRoundTrip(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	tb, err := b.CreateTableBucket("tr-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	table, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
	require.NoError(t, err)

	rules := []s3tables.ReplicationRule{
		{Destinations: []s3tables.ReplicationDestination{
			{DestinationTableBucketARN: "arn:aws:s3tables:us-east-1:000000000000:bucket/dest"},
		}},
	}

	cfg, err := b.PutTableReplication(table.ARN, "arn:aws:iam::000000000000:role/repl", rules, "")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::000000000000:role/repl", cfg.Role)
	assert.NotEmpty(t, cfg.VersionToken)
	assert.Equal(t, 1, s3tables.TableReplicationCount(b))

	got, err := b.GetTableReplicationConfig(table.ARN)
	require.NoError(t, err)
	assert.Equal(t, cfg.VersionToken, got.VersionToken)
	require.Len(t, got.Rules, 1)
	require.Len(t, got.Rules[0].Destinations, 1)
	assert.Equal(t,
		"arn:aws:s3tables:us-east-1:000000000000:bucket/dest",
		got.Rules[0].Destinations[0].DestinationTableBucketARN,
	)

	require.NoError(t, b.DeleteTableReplication(table.ARN, cfg.VersionToken))
	assert.Equal(t, 0, s3tables.TableReplicationCount(b))
}

func TestBackend_PutTableReplication_StaleVersionToken(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	tb, err := b.CreateTableBucket("tr-stale-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	table, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
	require.NoError(t, err)

	_, err = b.PutTableReplication(table.ARN, "arn:aws:iam::000000000000:role/repl", nil, "")
	require.NoError(t, err)

	_, err = b.PutTableReplication(table.ARN, "arn:aws:iam::000000000000:role/repl2", nil, "stale-token")
	require.ErrorIs(t, err, s3tables.ErrTableVersionConflict)
}

func TestBackend_DeleteTableReplication_StaleVersionToken(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	tb, err := b.CreateTableBucket("tr-del-stale-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	table, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
	require.NoError(t, err)

	_, err = b.PutTableReplication(table.ARN, "arn:aws:iam::000000000000:role/repl", nil, "")
	require.NoError(t, err)

	err = b.DeleteTableReplication(table.ARN, "stale-token")
	require.ErrorIs(t, err, s3tables.ErrTableVersionConflict)
}

// TestBackend_DeleteTable_VersionToken proves DeleteTable enforces the
// optional versionToken: a stale token is rejected with
// ErrTableVersionConflict and the table survives; an omitted or matching
// token deletes the table, per DeleteTableInput's versionToken being
// optional (aws-sdk-go-v2/service/s3tables@v1.18.4 api_op_DeleteTable.go).
func TestBackend_DeleteTable_VersionToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr      error
		versionToken func(actual string) string
		name         string
		bucket       string
		wantDeleted  bool
	}{
		{
			name:         "stale token rejected",
			bucket:       "del-token-stale",
			versionToken: func(string) string { return "stale-token" },
			wantErr:      s3tables.ErrTableVersionConflict,
			wantDeleted:  false,
		},
		{
			name:         "matching token deletes",
			bucket:       "del-token-match",
			versionToken: func(actual string) string { return actual },
			wantDeleted:  true,
		},
		{
			name:         "omitted token deletes",
			bucket:       "del-token-omit",
			versionToken: func(string) string { return "" },
			wantDeleted:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
			tb, err := b.CreateTableBucket(tt.bucket, s3tables.CreateTableBucketOptions{})
			require.NoError(t, err)

			_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
			require.NoError(t, err)

			table, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
			require.NoError(t, err)

			err = b.DeleteTable(tb.ARN, []string{"ns1"}, "t1", tt.versionToken(table.VersionToken))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			_, err = b.GetTableByARN(table.ARN)
			if tt.wantDeleted {
				assert.ErrorIs(t, err, s3tables.ErrTableNotFound)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBackend_TableRecordExpiryRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     string
		wantStatus string
	}{
		{
			name:       "set enabled",
			status:     "enabled",
			wantStatus: "enabled",
		},
		{
			name:       "set disabled",
			status:     "disabled",
			wantStatus: "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
			tb, err := b.CreateTableBucket("exp-bucket", s3tables.CreateTableBucketOptions{})
			require.NoError(t, err)

			_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
			require.NoError(t, err)

			table, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
			require.NoError(t, err)

			cfg := &s3tables.TableRecordExpiryConfig{Status: tt.status}
			require.NoError(t, b.PutTableRecordExpirationConfiguration(table.ARN, cfg))

			got, err := b.GetTableRecordExpirationConfiguration(table.ARN)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
		})
	}
}

func TestBackend_TableRecordExpiryDefaultDisabled(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	tb, err := b.CreateTableBucket("exp-default-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	table, err := b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
	require.NoError(t, err)

	got, err := b.GetTableRecordExpirationConfiguration(table.ARN)
	require.NoError(t, err)
	assert.Equal(t, "disabled", got.Status)
}

func TestBackend_PutTableReplication_NotFound(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.PutTableReplication("arn:aws:s3tables:us-east-1:000000000000:bucket/b/table/ns/t",
		"arn:aws:iam::000000000000:role/repl", nil, "")
	require.Error(t, err)
}

func TestBackend_DeleteTableReplication_NotFound(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	err := b.DeleteTableReplication("arn:aws:s3tables:us-east-1:000000000000:bucket/b/table/ns/t", "")
	require.Error(t, err)
}

func TestBackend_PutTableRecordExpirationConfiguration_NotFound(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	err := b.PutTableRecordExpirationConfiguration(
		"arn:aws:s3tables:us-east-1:000000000000:bucket/b/table/ns/t",
		&s3tables.TableRecordExpiryConfig{Status: "ENABLED"},
	)
	require.Error(t, err)
}

func TestBackend_GetTableRecordExpirationConfiguration_NotFound(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.GetTableRecordExpirationConfiguration(
		"arn:aws:s3tables:us-east-1:000000000000:bucket/b/table/ns/t",
	)
	require.Error(t, err)
}
