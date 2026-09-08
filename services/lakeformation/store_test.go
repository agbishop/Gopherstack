package lakeformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/lakeformation"
)

func TestPaginate_NextToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxResults int
		wantCount  int
		wantToken  bool
	}{
		{
			name:       "paginate returns next token when more items exist",
			maxResults: 1,
			wantCount:  1,
			wantToken:  true,
		},
		{
			name:       "paginate returns all items when max is 0",
			maxResults: 0,
			wantCount:  2,
			wantToken:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()

			require.NoError(
				t,
				b.RegisterResource(
					"arn:aws:s3:::bucket-a",
					"arn:aws:iam::123:role/r",
					lakeformation.RegisterResourceOptions{},
				),
			)
			require.NoError(
				t,
				b.RegisterResource(
					"arn:aws:s3:::bucket-b",
					"arn:aws:iam::123:role/r",
					lakeformation.RegisterResourceOptions{},
				),
			)

			resources, token := b.ListResources(nil, tt.maxResults, "")
			assert.Len(t, resources, tt.wantCount)

			if tt.wantToken {
				assert.NotEmpty(t, token)
			} else {
				assert.Empty(t, token)
			}
		})
	}
}

func TestPaginate_InvalidNextToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		nextToken string
		wantCount int
	}{
		{
			name:      "invalid next token falls back to start",
			nextToken: "not-a-number",
			wantCount: 2,
		},
		{
			name:      "negative next token falls back to start",
			nextToken: "-1",
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lakeformation.NewInMemoryBackend()

			require.NoError(
				t,
				b.RegisterResource("arn:aws:s3:::bucket-x", "arn:role", lakeformation.RegisterResourceOptions{}),
			)
			require.NoError(
				t,
				b.RegisterResource("arn:aws:s3:::bucket-y", "arn:role", lakeformation.RegisterResourceOptions{}),
			)

			resources, _ := b.ListResources(nil, 0, tt.nextToken)
			assert.Len(t, resources, tt.wantCount)
		})
	}
}

func TestReset(t *testing.T) {
	t.Parallel()

	b := lakeformation.NewInMemoryBackend()
	b.AddLFTagInternal("123456789012", "env", []string{"dev", "prod"})
	b.AddResourceInternal("arn:aws:s3:::my-bucket", "arn:aws:iam::123456789012:role/MyRole")
	require.Equal(t, 1, b.TagCount())
	require.Equal(t, 1, b.ResourceCount())

	b.Reset()

	assert.Equal(t, 0, b.TagCount())
	assert.Equal(t, 0, b.ResourceCount())
	assert.Equal(t, 0, b.PermissionCount())
}

// TestReset_TableObjectsCleared verifies that Reset() clears governed table
// object state written via UpdateTableObjects.
func TestReset_TableObjectsCleared(t *testing.T) {
	t.Parallel()

	b := lakeformation.NewInMemoryBackend()

	txnID := b.StartTransaction("READ_AND_WRITE")

	size := int64(42)
	err := b.UpdateTableObjects("123456789012", "mydb", "mytable", txnID, []lakeformation.WriteOperation{
		{AddObject: &lakeformation.TableObject{URI: "s3://bucket/obj1", Size: &size}},
	})
	require.NoError(t, err)

	objectsBefore, _ := b.GetTableObjects("123456789012", "mydb", "mytable", "", 0, "")
	require.Len(t, objectsBefore, 1, "sanity: table object should be recorded")

	b.Reset()

	objectsAfter, _ := b.GetTableObjects("123456789012", "mydb", "mytable", "", 0, "")
	assert.Empty(t, objectsAfter, "tableObjects must be cleared by Reset")
}

func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	b := lakeformation.NewInMemoryBackend()

	for i := range 3 {
		_ = i
		b.AddLFTagInternal("cat", "key", []string{"v"})
		b.Reset()
		assert.Equal(t, 0, b.TagCount())
	}
}
