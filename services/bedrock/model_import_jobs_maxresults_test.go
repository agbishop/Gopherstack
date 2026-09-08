package bedrock_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

// TestListImportedModels_MaxResults locks in gopherstack-kkfs: ListImportedModels
// ignored the real ListImportedModelsInput.MaxResults field entirely and
// always paginated at the fixed bedrockDefaultPageSize via
// paginateBedrockSlice, silently dropping a client's smaller page-size
// request. Mirrors the already-fixed sibling ListModelImportJobs shape.
func TestListImportedModels_MaxResults(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("000000000000", "us-east-1")

	const n = 5

	names := make([]string, 0, n)
	for i := range n {
		name := fmt.Sprintf("import-%d", i)
		_, err := b.CreateModelImportJob(
			name, name+"-model", "arn:aws:iam::000000000000:role/x", "s3://bucket/data/", nil,
		)
		require.NoError(t, err)
		names = append(names, name+"-model")
	}
	require.Len(t, names, n)

	t.Run("smaller page returns exactly that many plus a token", func(t *testing.T) {
		t.Parallel()

		page, next := b.ListImportedModels(&bedrock.ListImportedModelsInput{MaxResults: 2})
		assert.Len(t, page, 2)
		assert.NotEmpty(t, next)
	})

	t.Run("resuming with the token returns the remainder", func(t *testing.T) {
		t.Parallel()

		first, next := b.ListImportedModels(&bedrock.ListImportedModelsInput{MaxResults: 2})
		require.Len(t, first, 2)
		require.NotEmpty(t, next)

		second, next2 := b.ListImportedModels(&bedrock.ListImportedModelsInput{MaxResults: 2, NextToken: next})
		require.Len(t, second, 2)
		require.NotEmpty(t, next2)

		third, next3 := b.ListImportedModels(&bedrock.ListImportedModelsInput{MaxResults: 2, NextToken: next2})
		require.Len(t, third, 1)
		require.Empty(t, next3)

		seen := make(map[string]bool, n)
		for _, m := range first {
			seen[m.ImportedModelArn] = true
		}
		for _, m := range second {
			assert.False(t, seen[m.ImportedModelArn], "duplicate across pages: %s", m.ImportedModelArn)
			seen[m.ImportedModelArn] = true
		}
		for _, m := range third {
			assert.False(t, seen[m.ImportedModelArn], "duplicate across pages: %s", m.ImportedModelArn)
			seen[m.ImportedModelArn] = true
		}
		assert.Len(t, seen, n)
	})

	t.Run("absent maxresults still returns everything", func(t *testing.T) {
		t.Parallel()

		page, next := b.ListImportedModels(&bedrock.ListImportedModelsInput{})
		assert.Len(t, page, n)
		assert.Empty(t, next)
	})

	t.Run("ordering is stable across calls", func(t *testing.T) {
		t.Parallel()

		first, _ := b.ListImportedModels(&bedrock.ListImportedModelsInput{})
		second, _ := b.ListImportedModels(&bedrock.ListImportedModelsInput{})
		require.Len(t, first, n)
		require.Len(t, second, n)

		for i := range first {
			assert.Equal(t, first[i].ImportedModelArn, second[i].ImportedModelArn)
		}
	})
}
