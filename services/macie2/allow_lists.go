package macie2

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) allowListARN(id string) string {
	return arn.Build("macie2", b.region, b.accountID, fmt.Sprintf("allow-list/%s", id))
}

// CreateAllowList creates a new allow list.
func (b *InMemoryBackend) CreateAllowList(
	name, description string,
	criteria AllowListCriteria,
	tags map[string]string,
) (*AllowListSummary, error) {
	b.mu.Lock("CreateAllowList")
	defer b.mu.Unlock()

	id := uuid.New().String()
	now := time.Now().UTC()
	al := &storedAllowList{
		ID:          id,
		Arn:         b.allowListARN(id),
		Name:        name,
		Description: description,
		Criteria:    criteria,
		CreatedAt:   now,
		UpdatedAt:   now,
		Status:      AllowListStatus{Code: "OK"},
		Tags:        maps.Clone(tags),
	}

	b.allowLists.Put(al)
	if len(tags) > 0 {
		b.tags[al.Arn] = maps.Clone(tags)
	}

	return &AllowListSummary{
		Arn:         al.Arn,
		CreatedAt:   al.CreatedAt,
		Description: al.Description,
		ID:          al.ID,
		Name:        al.Name,
		UpdatedAt:   al.UpdatedAt,
		Tags:        al.Tags,
	}, nil
}

// GetAllowList retrieves an allow list by ID.
func (b *InMemoryBackend) GetAllowList(id string) (*AllowListDetail, error) {
	b.mu.RLock("GetAllowList")
	defer b.mu.RUnlock()

	al, ok := b.allowLists.Get(id)
	if !ok {
		return nil, ErrAllowListNotFound
	}

	cp := al.AllowListDetail
	cp.Tags = maps.Clone(al.Tags)

	return &cp, nil
}

// UpdateAllowList updates an existing allow list.
func (b *InMemoryBackend) UpdateAllowList(
	id, name, description string,
	criteria AllowListCriteria,
) (*AllowListSummary, error) {
	b.mu.Lock("UpdateAllowList")
	defer b.mu.Unlock()

	al, ok := b.allowLists.Get(id)
	if !ok {
		return nil, ErrAllowListNotFound
	}

	if name != "" {
		al.Name = name
	}

	al.Description = description
	al.Criteria = criteria
	al.UpdatedAt = time.Now().UTC()

	return &AllowListSummary{
		Arn:         al.Arn,
		CreatedAt:   al.CreatedAt,
		Description: al.Description,
		ID:          al.ID,
		Name:        al.Name,
		UpdatedAt:   al.UpdatedAt,
		Tags:        maps.Clone(al.Tags),
	}, nil
}

// DeleteAllowList deletes an allow list.
func (b *InMemoryBackend) DeleteAllowList(id string) error {
	b.mu.Lock("DeleteAllowList")
	defer b.mu.Unlock()

	if !b.allowLists.Delete(id) {
		return ErrAllowListNotFound
	}
	delete(b.tags, b.allowListARN(id))

	return nil
}

// ListAllowLists returns summaries of all allow lists.
//
//nolint:dupl // structurally identical to ListFindingsFilters but operates on a different type
func (b *InMemoryBackend) ListAllowLists(limit int, token string) ([]*AllowListSummary, string, error) {
	return listPaginated(
		b, "ListAllowLists", b.allowLists.All(),
		func(al *storedAllowList) (*AllowListSummary, bool) {
			return &AllowListSummary{
				Arn:         al.Arn,
				CreatedAt:   al.CreatedAt,
				Description: al.Description,
				ID:          al.ID,
				Name:        al.Name,
				UpdatedAt:   al.UpdatedAt,
				Tags:        maps.Clone(al.Tags),
			}, true
		},
		func(result []*AllowListSummary) {
			// Name has no uniqueness constraint (CreateAllowList never checks for an
			// existing Name); ID as a tiebreaker keeps a total order so the
			// offset-based page.NewHMAC cursor can't drop or duplicate allow lists
			// that tie on Name.
			sort.Slice(result, func(i, j int) bool {
				if result[i].Name != result[j].Name {
					return result[i].Name < result[j].Name
				}

				return result[i].ID < result[j].ID
			})
		},
		token, limit,
	)
}
