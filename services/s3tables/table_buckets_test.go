package s3tables_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/s3tables"
)

// TestBackend_CreateTableBucket_NameValidation verifies CreateTableBucket
// enforces real S3 Tables table-bucket naming rules (field-diffed against
// https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-tables-buckets-naming.html):
// 3-63 chars; lowercase letters, digits, hyphens only; must begin/end with a
// letter or number; no underscores/periods; reserved prefix/suffix denylist.
func TestBackend_CreateTableBucket_NameValidation(t *testing.T) {
	t.Parallel()

	// Subtest names double as the case description: too-short/too-long length
	// bounds, uppercase/underscore/period char-class violations, leading/
	// trailing hyphen, each reserved prefix (xn--, sthree-, amzn-s3-demo-,
	// aws), each reserved suffix (-s3alias, --ol-s3, --x-s3, --table-s3), and
	// two valid names (one at the minimum length).
	tests := []struct {
		name      string
		wantValid bool
	}{
		{name: "va", wantValid: false},
		{
			name:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantValid: false,
		},
		{name: "Valid-Bucket", wantValid: false},
		{name: "valid_bucket", wantValid: false},
		{name: "valid.bucket", wantValid: false},
		{name: "-valid-bucket", wantValid: false},
		{name: "valid-bucket-", wantValid: false},
		{name: "xn--valid-bucket", wantValid: false},
		{name: "sthree-valid-bucket", wantValid: false},
		{name: "amzn-s3-demo-valid-bucket", wantValid: false},
		{name: "aws-valid-bucket", wantValid: false},
		{name: "valid-bucket-s3alias", wantValid: false},
		{name: "valid-bucket--ol-s3", wantValid: false},
		{name: "valid-bucket--x-s3", wantValid: false},
		{name: "valid-bucket--table-s3", wantValid: false},
		{name: "valid-bucket-123", wantValid: true},
		{name: "abc", wantValid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			_, err := b.CreateTableBucket(tt.name, s3tables.CreateTableBucketOptions{})

			if tt.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, s3tables.ErrInvalidBucketName)
			}
		})
	}
}

func TestBackend_CreateTableBucket_AppliesOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		opts             s3tables.CreateTableBucketOptions
		name             string
		wantStorageClass string
		wantEncrypted    bool
	}{
		{
			name:             "no_options_defaults_to_standard",
			opts:             s3tables.CreateTableBucketOptions{},
			wantStorageClass: "STANDARD",
			wantEncrypted:    false,
		},
		{
			name: "encryption_and_storage_class_and_tags_applied",
			opts: s3tables.CreateTableBucketOptions{
				Encryption: map[string]any{
					"sseAlgorithm": "aws:kms",
					"kmsKeyArn":    "arn:aws:kms:us-east-1:000000000000:key/test",
				},
				StorageClass: "INTELLIGENT_TIERING",
				Tags:         map[string]string{"env": "prod"},
			},
			wantStorageClass: "INTELLIGENT_TIERING",
			wantEncrypted:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			tb, err := b.CreateTableBucket("opts-bucket-"+bucketSuffix(tt.name), tt.opts)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStorageClass, tb.StorageClass)

			if tt.wantEncrypted {
				require.NotNil(t, tb.Encryption)
				assert.Equal(t, "aws:kms", tb.Encryption["sseAlgorithm"])
			} else {
				assert.Nil(t, tb.Encryption)
			}

			if len(tt.opts.Tags) > 0 {
				tags, tagErr := b.ListTagsForResource(tb.ARN)
				require.NoError(t, tagErr)
				assert.Equal(t, tt.opts.Tags, tags)
			}
		})
	}
}

func TestBackend_ListTableBuckets_Pagination(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	const total = 5
	for i := range total {
		_, err := b.CreateTableBucket(fmt.Sprintf("page-bucket-%d", i), s3tables.CreateTableBucketOptions{})
		require.NoError(t, err)
	}

	pg, err := b.ListTableBuckets(s3tables.ListTableBucketsParams{MaxBuckets: 2})
	require.NoError(t, err)
	assert.Len(t, pg.Data, 2)
	require.NotEmpty(t, pg.Next, "expected a continuation token when more results remain")

	seen := len(pg.Data)
	token := pg.Next

	for token != "" {
		pg, err = b.ListTableBuckets(s3tables.ListTableBucketsParams{MaxBuckets: 2, ContinuationToken: token})
		require.NoError(t, err)
		seen += len(pg.Data)
		token = pg.Next
	}

	assert.Equal(t, total, seen, "pagination must eventually enumerate every bucket exactly once")
}

func TestBackend_ListTableBuckets_PrefixAndTypeFilter(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	_, err := b.CreateTableBucket("alpha-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)
	_, err = b.CreateTableBucket("beta-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	pg, err := b.ListTableBuckets(s3tables.ListTableBucketsParams{Prefix: "alpha"})
	require.NoError(t, err)
	require.Len(t, pg.Data, 1)
	assert.Equal(t, "alpha-bucket", pg.Data[0].Name)

	// Every bucket this backend creates is "customer" type; filtering by
	// "aws" must match nothing, matching real AWS semantics.
	pg, err = b.ListTableBuckets(s3tables.ListTableBucketsParams{Type: "aws"})
	require.NoError(t, err)
	assert.Empty(t, pg.Data)

	pg, err = b.ListTableBuckets(s3tables.ListTableBucketsParams{Type: "customer"})
	require.NoError(t, err)
	assert.Len(t, pg.Data, 2)
}

func TestBackend_ListTableBuckets_InvalidToken(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	_, err := b.ListTableBuckets(s3tables.ListTableBucketsParams{ContinuationToken: "not-a-valid-token!!"})
	require.Error(t, err)
	assert.ErrorIs(t, err, s3tables.ErrInvalidContinuationToken)
}

func TestBackend_BucketReplicationRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		destARN   string
		wantCount int
	}{
		{
			name:      "single destination",
			destARN:   "arn:aws:s3tables:us-east-1:000000000000:bucket/dest1",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
			tb, err := b.CreateTableBucket("rr-bucket", s3tables.CreateTableBucketOptions{})
			require.NoError(t, err)

			rules := []s3tables.ReplicationRule{
				{Destinations: []s3tables.ReplicationDestination{{DestinationTableBucketARN: tt.destARN}}},
			}

			putCfg, err := b.PutTableBucketReplication(tb.ARN, "arn:aws:iam::000000000000:role/repl", rules, "")
			require.NoError(t, err)
			assert.NotEmpty(t, putCfg.VersionToken)

			got, err := b.GetTableBucketReplication(tb.ARN)
			require.NoError(t, err)
			require.Len(t, got.Rules, tt.wantCount)
			assert.Equal(t, tt.destARN, got.Rules[0].Destinations[0].DestinationTableBucketARN)
			assert.Equal(t, putCfg.VersionToken, got.VersionToken)

			require.NoError(t, b.DeleteTableBucketReplication(tb.ARN, got.VersionToken))
			assert.Equal(t, 0, s3tables.BucketReplicationCount(b))
		})
	}
}

func TestBackend_DeleteTableBucketReplication_NotFound(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	err := b.DeleteTableBucketReplication("arn:aws:s3tables:us-east-1:000000000000:bucket/nonexistent", "")
	require.Error(t, err)
}

func TestBackend_GetTableBucketReplication_BucketNotFound(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.GetTableBucketReplication("arn:aws:s3tables:us-east-1:000000000000:bucket/nonexistent")
	require.Error(t, err)
}

func TestBackend_PutTableBucketReplication_BucketNotFound(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.PutTableBucketReplication(
		"arn:aws:s3tables:us-east-1:000000000000:bucket/nonexistent",
		"arn:aws:iam::000000000000:role/repl", nil, "",
	)
	require.Error(t, err)
}

func TestBackend_PutTableBucketReplication_StaleVersionToken(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	tb, err := b.CreateTableBucket("rr-stale-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.PutTableBucketReplication(tb.ARN, "arn:aws:iam::000000000000:role/repl", nil, "")
	require.NoError(t, err)

	_, err = b.PutTableBucketReplication(tb.ARN, "arn:aws:iam::000000000000:role/repl2", nil, "stale-token")
	require.ErrorIs(t, err, s3tables.ErrTableVersionConflict)
}

func TestBackend_DeleteTableBucketReplication_StaleVersionToken(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	tb, err := b.CreateTableBucket("rr-del-stale-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.PutTableBucketReplication(tb.ARN, "arn:aws:iam::000000000000:role/repl", nil, "")
	require.NoError(t, err)

	err = b.DeleteTableBucketReplication(tb.ARN, "stale-token")
	require.ErrorIs(t, err, s3tables.ErrTableVersionConflict)
}

// TestBackend_DeleteTableBucket_NotEmpty proves DeleteTableBucket rejects
// deleting a bucket that still contains a namespace, per AWS docs
// (s3-tables-buckets-delete.html): "Before you delete a table bucket, you
// must first delete all namespaces and tables within the bucket." Once the
// namespace (and its table) are gone, the same bucket deletes cleanly.
func TestBackend_DeleteTableBucket_NotEmpty(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend("000000000000", "us-east-1")
	tb, err := b.CreateTableBucket("del-nonempty-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	err = b.DeleteTableBucket(tb.ARN)
	require.ErrorIs(t, err, s3tables.ErrTableBucketNotEmpty)

	require.NoError(t, b.DeleteNamespace(tb.ARN, []string{"ns1"}))
	require.NoError(t, b.DeleteTableBucket(tb.ARN))
}
