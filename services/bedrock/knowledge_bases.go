package bedrock

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	kbStatusActive = "ACTIVE"
)

// CreateKnowledgeBase creates a new knowledge base.
func (b *InMemoryBackend) CreateKnowledgeBase(
	name, description, roleArn string,
	kbConfig, storageConfig map[string]any,
	tags map[string]string,
) (*KnowledgeBase, error) {
	b.mu.Lock("CreateKnowledgeBase")
	defer b.mu.Unlock()

	if _, ok := b.kbByName[name]; ok {
		return nil, fmt.Errorf("%w: knowledge base %q already exists", ErrAlreadyExists, name)
	}

	b.kbCounter++
	id := fmt.Sprintf("kb-%08d", b.kbCounter)
	kbArn := arn.Build("bedrock", b.region, b.accountID, "knowledge-base/"+id)
	now := time.Now()

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	kb := &KnowledgeBase{
		CreatedAt:                  now,
		UpdatedAt:                  now,
		KnowledgeBaseID:            id,
		KnowledgeBaseArn:           kbArn,
		Name:                       name,
		Description:                description,
		Status:                     kbStatusActive,
		RoleArn:                    roleArn,
		KnowledgeBaseConfiguration: kbConfig,
		StorageConfiguration:       storageConfig,
		Tags:                       tagsCopy,
	}
	b.knowledgeBases.Put(kb)
	b.kbByName[name] = id
	cp := *kb

	return &cp, nil
}

// GetKnowledgeBase returns a knowledge base by ID.
func (b *InMemoryBackend) GetKnowledgeBase(kbID string) (*KnowledgeBase, error) {
	b.mu.RLock("GetKnowledgeBase")
	defer b.mu.RUnlock()

	kb, ok := b.knowledgeBases.Get(kbID)
	if !ok {
		return nil, fmt.Errorf("%w: knowledge base %q not found", ErrNotFound, kbID)
	}

	cp := *kb

	return &cp, nil
}

// ListKnowledgeBases returns all knowledge bases with pagination.
func (b *InMemoryBackend) ListKnowledgeBases(
	maxResults int,
	nextToken string,
) ([]*KnowledgeBase, string) {
	b.mu.RLock("ListKnowledgeBases")
	defer b.mu.RUnlock()

	list := make([]*KnowledgeBase, 0, b.knowledgeBases.Len())
	for _, kb := range b.knowledgeBases.All() {
		cp := *kb
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return paginate(list, maxResults, nextToken)
}

// UpdateKnowledgeBase updates a knowledge base.
func (b *InMemoryBackend) UpdateKnowledgeBase(
	kbID, name, description, roleArn string,
) (*KnowledgeBase, error) {
	b.mu.Lock("UpdateKnowledgeBase")
	defer b.mu.Unlock()

	kb, ok := b.knowledgeBases.Get(kbID)
	if !ok {
		return nil, fmt.Errorf("%w: knowledge base %q not found", ErrNotFound, kbID)
	}

	if name != "" && name != kb.Name {
		delete(b.kbByName, kb.Name)
		kb.Name = name
		b.kbByName[name] = kbID
	}

	if description != "" {
		kb.Description = description
	}

	if roleArn != "" {
		kb.RoleArn = roleArn
	}

	kb.UpdatedAt = time.Now()
	cp := *kb

	return &cp, nil
}

// DeleteKnowledgeBase deletes a knowledge base and its data sources.
// GetDataSource/ListDataSources otherwise keep returning orphaned rows for a
// KB ID that no longer resolves (gopherstack-wg7i).
func (b *InMemoryBackend) DeleteKnowledgeBase(kbID string) error {
	b.mu.Lock("DeleteKnowledgeBase")
	defer b.mu.Unlock()

	kb, ok := b.knowledgeBases.Get(kbID)
	if !ok {
		return fmt.Errorf("%w: knowledge base %q not found", ErrNotFound, kbID)
	}

	delete(b.kbByName, kb.Name)
	b.knowledgeBases.Delete(kbID)

	b.dataSources.Range(func(ds *DataSource) bool {
		if ds.KnowledgeBaseID == kbID {
			b.dataSources.Delete(ds.KnowledgeBaseID + "/" + ds.DataSourceID)
		}

		return true
	})

	return nil
}
