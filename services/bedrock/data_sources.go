package bedrock

import (
	"fmt"
	"maps"
	"sort"
	"time"
)

const (
	dsStatusAvailable = "AVAILABLE"
)

// CreateDataSource creates a data source for a knowledge base.
func (b *InMemoryBackend) CreateDataSource(
	kbID, name, description string,
	dsConfig map[string]any,
) (*DataSource, error) {
	return b.CreateDataSourceWithConfiguration(kbID, name, description, "", dsConfig, nil)
}

// CreateDataSourceWithConfiguration creates a data source with vector ingestion settings.
func (b *InMemoryBackend) CreateDataSourceWithConfiguration(
	kbID, name, description, deletionPolicy string,
	dsConfig, vectorConfig map[string]any,
) (*DataSource, error) {
	b.mu.Lock("CreateDataSource")
	defer b.mu.Unlock()

	if _, ok := b.knowledgeBases.Get(kbID); !ok {
		return nil, fmt.Errorf("%w: knowledge base %q not found", ErrNotFound, kbID)
	}

	b.dataSourceCounter++
	id := fmt.Sprintf("ds-%08d", b.dataSourceCounter)
	now := time.Now()

	ds := &DataSource{
		CreatedAt:               now,
		UpdatedAt:               now,
		DataSourceID:            id,
		DataSourceStatus:        dsStatusAvailable,
		KnowledgeBaseID:         kbID,
		Name:                    name,
		Description:             description,
		DataDeletionPolicy:      deletionPolicy,
		DataSourceConfiguration: maps.Clone(dsConfig),
		VectorIngestionConfig:   maps.Clone(vectorConfig),
	}
	b.dataSources.Put(ds)
	cp := *ds

	return &cp, nil
}

// GetDataSource returns a data source by ID.
func (b *InMemoryBackend) GetDataSource(kbID, dsID string) (*DataSource, error) {
	b.mu.RLock("GetDataSource")
	defer b.mu.RUnlock()

	ds, ok := b.dataSources.Get(kbID + "/" + dsID)
	if !ok {
		return nil, fmt.Errorf("%w: data source %q not found", ErrNotFound, dsID)
	}

	cp := *ds

	return &cp, nil
}

// ListDataSources lists data sources for a knowledge base.
func (b *InMemoryBackend) ListDataSources(
	kbID string,
	maxResults int,
	nextToken string,
) ([]*DataSource, string) {
	b.mu.RLock("ListDataSources")
	defer b.mu.RUnlock()

	list := make([]*DataSource, 0, b.dataSources.Len())

	for _, ds := range b.dataSources.All() {
		if ds.KnowledgeBaseID == kbID {
			cp := *ds
			list = append(list, &cp)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}

		return list[i].DataSourceID < list[j].DataSourceID
	})

	return paginate(list, maxResults, nextToken)
}

// UpdateDataSource updates a data source.
func (b *InMemoryBackend) UpdateDataSource(
	kbID, dsID, name, description string,
) (*DataSource, error) {
	return b.UpdateDataSourceWithConfiguration(kbID, dsID, name, description, "", nil, nil)
}

// UpdateDataSourceWithConfiguration updates data source ingestion and connector settings.
func (b *InMemoryBackend) UpdateDataSourceWithConfiguration(
	kbID, dsID, name, description, deletionPolicy string,
	dsConfig, vectorConfig map[string]any,
) (*DataSource, error) {
	b.mu.Lock("UpdateDataSource")
	defer b.mu.Unlock()

	key := kbID + "/" + dsID

	ds, ok := b.dataSources.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: data source %q not found", ErrNotFound, dsID)
	}

	if name != "" {
		ds.Name = name
	}

	if description != "" {
		ds.Description = description
	}
	if deletionPolicy != "" {
		ds.DataDeletionPolicy = deletionPolicy
	}
	if dsConfig != nil {
		ds.DataSourceConfiguration = maps.Clone(dsConfig)
	}
	if vectorConfig != nil {
		ds.VectorIngestionConfig = maps.Clone(vectorConfig)
	}

	ds.UpdatedAt = time.Now()
	cp := *ds

	return &cp, nil
}

// DeleteDataSource deletes a data source and its ingestion jobs and
// documents. GetIngestionJob/ListIngestionJobs/ListKnowledgeBaseDocuments
// otherwise keep returning orphaned rows for a data source ID that no
// longer resolves, the same gap gopherstack-jkiu closed for
// DeleteKnowledgeBase (gopherstack-y0to).
func (b *InMemoryBackend) DeleteDataSource(kbID, dsID string) error {
	b.mu.Lock("DeleteDataSource")
	defer b.mu.Unlock()

	key := kbID + "/" + dsID

	if _, ok := b.dataSources.Get(key); !ok {
		return fmt.Errorf("%w: data source %q not found", ErrNotFound, dsID)
	}

	b.dataSources.Delete(key)

	b.ingestionJobs.Range(func(job *IngestionJob) bool {
		if job.KnowledgeBaseID == kbID && job.DataSourceID == dsID {
			b.ingestionJobs.Delete(ingestionJobKey(job.KnowledgeBaseID, job.DataSourceID, job.IngestionJobID))
		}

		return true
	})

	b.kbDocuments.Range(func(doc *KnowledgeBaseDocument) bool {
		if doc.KnowledgeBaseID == kbID && doc.DataSourceID == dsID {
			b.kbDocuments.Delete(kbDocKey(doc.KnowledgeBaseID, doc.DataSourceID, doc.DocumentID))
		}

		return true
	})

	return nil
}
