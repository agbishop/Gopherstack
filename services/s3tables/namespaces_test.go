package s3tables_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/s3tables"
)

// TestBackend_CreateNamespace_NameValidation verifies CreateNamespace
// enforces real S3 Tables namespace naming rules (field-diffed against
// https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-tables-buckets-naming.html):
// 1-255 chars; lowercase letters, digits, underscores only; must begin with
// a letter or number; no hyphens/periods; must not start with reserved
// prefix "aws".
func TestBackend_CreateNamespace_NameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantValid bool
	}{
		{name: "", wantValid: false},            // too short (< 1 char)
		{name: "Valid_ns", wantValid: false},    // uppercase not allowed
		{name: "valid-ns", wantValid: false},    // hyphen not allowed
		{name: "valid.ns", wantValid: false},    // period not allowed
		{name: "_valid_ns", wantValid: false},   // must begin with letter/number
		{name: "awsreserved", wantValid: false}, // reserved "aws" prefix
		{name: "aws_ns", wantValid: false},      // reserved "aws" prefix
		{name: "valid_ns_123", wantValid: true},
		{name: "a", wantValid: true}, // exactly minimum length
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)
			tb, err := b.CreateTableBucket("ns-validation-bucket", s3tables.CreateTableBucketOptions{})
			require.NoError(t, err)

			_, err = b.CreateNamespace(tb.ARN, []string{tt.name})

			if tt.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, s3tables.ErrInvalidName)
			}
		})
	}
}

func TestBackend_ListNamespaces_PaginationAndPrefix(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	tb, err := b.CreateTableBucket("ns-page-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	for _, ns := range []string{"alpha", "beta", "gamma"} {
		_, err = b.CreateNamespace(tb.ARN, []string{ns})
		require.NoError(t, err)
	}

	pg, err := b.ListNamespaces(tb.ARN, s3tables.ListNamespacesParams{MaxNamespaces: 1})
	require.NoError(t, err)
	require.Len(t, pg.Data, 1)
	require.NotEmpty(t, pg.Next)

	pg, err = b.ListNamespaces(tb.ARN, s3tables.ListNamespacesParams{Prefix: "al"})
	require.NoError(t, err)
	require.Len(t, pg.Data, 1)
	assert.Equal(t, []string{"alpha"}, pg.Data[0].Namespace)
}

// TestBackend_DeleteNamespace_NotEmpty proves DeleteNamespace rejects
// deleting a namespace that still contains a table, per AWS docs
// (s3-tables-namespace-delete.html): "Before you delete a table namespace
// ... you must delete all tables within the namespace, or move them under
// another namespace." Once the table is gone, the same namespace deletes
// cleanly.
func TestBackend_DeleteNamespace_NotEmpty(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	tb, err := b.CreateTableBucket("del-ns-nonempty-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"ns1"})
	require.NoError(t, err)

	_, err = b.CreateTable(tb.ARN, []string{"ns1"}, "t1", "ICEBERG", s3tables.CreateTableOptions{})
	require.NoError(t, err)

	err = b.DeleteNamespace(tb.ARN, []string{"ns1"})
	require.ErrorIs(t, err, s3tables.ErrNamespaceNotEmpty)

	require.NoError(t, b.DeleteTable(tb.ARN, []string{"ns1"}, "t1", ""))
	require.NoError(t, b.DeleteNamespace(tb.ARN, []string{"ns1"}))
}
