package bedrock_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

func TestDeleteDataSource_PrunesIngestionJobsAndDocuments(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	kb, err := b.CreateKnowledgeBase(
		"kb1",
		"",
		"arn:aws:iam::000000000000:role/test",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	ds, err := b.CreateDataSource(kb.KnowledgeBaseID, "ds1", "", nil)
	require.NoError(t, err)

	job, err := b.StartIngestionJob(kb.KnowledgeBaseID, ds.DataSourceID, "")
	require.NoError(t, err)

	_, err = b.IngestKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds.DataSourceID, []string{"doc1"})
	require.NoError(t, err)

	require.NoError(t, b.DeleteDataSource(kb.KnowledgeBaseID, ds.DataSourceID))

	_, err = b.GetIngestionJob(kb.KnowledgeBaseID, ds.DataSourceID, job.IngestionJobID)
	require.Error(t, err, "ingestion job must not survive its parent data source's deletion")

	jobs, _ := b.ListIngestionJobs(kb.KnowledgeBaseID, ds.DataSourceID, 0, "")
	assert.Empty(
		t,
		jobs,
		"ListIngestionJobs must not return ghost rows for a deleted data source",
	)

	docs, _ := b.ListKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds.DataSourceID, 0, "")
	assert.Empty(
		t,
		docs,
		"ListKnowledgeBaseDocuments must not return ghost rows for a deleted data source",
	)
}

// TestDeleteDataSource_DoesNotAffectSiblingDataSource proves the DataSourceID
// filter in the cascade is exact, not a KB-wide prune -- a sibling data
// source's ingestion jobs and documents must survive.
func TestDeleteDataSource_DoesNotAffectSiblingDataSource(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	kb, err := b.CreateKnowledgeBase(
		"kb1",
		"",
		"arn:aws:iam::000000000000:role/test",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	ds1, err := b.CreateDataSource(kb.KnowledgeBaseID, "ds1", "", nil)
	require.NoError(t, err)

	ds2, err := b.CreateDataSource(kb.KnowledgeBaseID, "ds2", "", nil)
	require.NoError(t, err)

	job2, err := b.StartIngestionJob(kb.KnowledgeBaseID, ds2.DataSourceID, "")
	require.NoError(t, err)

	_, err = b.IngestKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds2.DataSourceID, []string{"doc1"})
	require.NoError(t, err)

	require.NoError(t, b.DeleteDataSource(kb.KnowledgeBaseID, ds1.DataSourceID))

	_, err = b.GetIngestionJob(kb.KnowledgeBaseID, ds2.DataSourceID, job2.IngestionJobID)
	require.NoError(t, err, "sibling data source's ingestion job must survive")

	docs, _ := b.ListKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds2.DataSourceID, 0, "")
	assert.Len(t, docs, 1, "sibling data source's documents must survive")
}

// TestDeleteKnowledgeBase_StillCascadesAfterDataSourceCascadeFix proves the
// DeleteDataSource cascade added for gopherstack-y0to composes with the
// DeleteKnowledgeBase cascade gopherstack-jkiu added in the same file:
// deleting the parent KB directly (without deleting the data source first)
// still removes its ingestion jobs and documents.
func TestDeleteKnowledgeBase_StillCascadesAfterDataSourceCascadeFix(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	kb, err := b.CreateKnowledgeBase(
		"kb1",
		"",
		"arn:aws:iam::000000000000:role/test",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	ds, err := b.CreateDataSource(kb.KnowledgeBaseID, "ds1", "", nil)
	require.NoError(t, err)

	job, err := b.StartIngestionJob(kb.KnowledgeBaseID, ds.DataSourceID, "")
	require.NoError(t, err)

	_, err = b.IngestKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds.DataSourceID, []string{"doc1"})
	require.NoError(t, err)

	require.NoError(t, b.DeleteKnowledgeBase(kb.KnowledgeBaseID))

	_, err = b.GetIngestionJob(kb.KnowledgeBaseID, ds.DataSourceID, job.IngestionJobID)
	require.Error(t, err, "ingestion job must not survive its parent knowledge base's deletion")

	docs, _ := b.ListKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds.DataSourceID, 0, "")
	assert.Empty(t, docs, "ListKnowledgeBaseDocuments must not return ghost rows for a deleted knowledge base")
}
