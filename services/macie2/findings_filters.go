package macie2

import (
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) findingsFilterARN(id string) string {
	return arn.Build("macie2", b.region, b.accountID, fmt.Sprintf("findings-filter/%s", id))
}

// filterActionArchive is FindingsFilterAction's ARCHIVE value: "suppress (automatically
// archive) the findings" that match the filter criteria.
const filterActionArchive = "ARCHIVE"

// matchesArchiveFilter reports whether any ARCHIVE-action findings filter matches f.
// CreateFindingsFilter's Action was previously stored and echoed back but never applied --
// CreateSampleFindings is this backend's only finding-creation path, so it is the only
// place Action can take effect. Mirrors guardduty's identical fix (matchesArchiveFilter in
// services/guardduty/filters.go). Caller must already hold b.mu.
func (b *InMemoryBackend) matchesArchiveFilter(f *storedFinding) bool {
	for _, ff := range b.findingsFilters.All() {
		if ff.Action == filterActionArchive && matchesFindingCriteria(f, ff.FindingCriteria) {
			return true
		}
	}

	return false
}

// CreateFindingsFilter creates a new findings filter.
func (b *InMemoryBackend) CreateFindingsFilter(
	name, description, action string,
	position *int32,
	criteria map[string]any,
	tags map[string]string,
) (*FindingsFilterSummary, error) {
	b.mu.Lock("CreateFindingsFilter")
	defer b.mu.Unlock()

	id := uuid.New().String()

	pos := int32(1)
	if position != nil {
		pos = *position
	}

	ff := &storedFindingsFilter{
		ID:              id,
		Arn:             b.findingsFilterARN(id),
		Name:            name,
		Description:     description,
		Action:          action,
		Position:        pos,
		FindingCriteria: criteria,
		Tags:            maps.Clone(tags),
	}

	b.findingsFilters.Put(ff)
	if len(tags) > 0 {
		b.tags[ff.Arn] = maps.Clone(tags)
	}

	return &FindingsFilterSummary{
		Action:      ff.Action,
		Arn:         ff.Arn,
		Description: ff.Description,
		ID:          ff.ID,
		Name:        ff.Name,
		Position:    ff.Position,
		Tags:        maps.Clone(ff.Tags),
	}, nil
}

// GetFindingsFilter retrieves a findings filter.
func (b *InMemoryBackend) GetFindingsFilter(id string) (*FindingsFilterDetail, error) {
	b.mu.RLock("GetFindingsFilter")
	defer b.mu.RUnlock()

	ff, ok := b.findingsFilters.Get(id)
	if !ok {
		return nil, ErrFindingsFilterNotFound
	}

	cp := ff.FindingsFilterDetail
	cp.Tags = maps.Clone(ff.Tags)

	return &cp, nil
}

// UpdateFindingsFilter updates an existing findings filter.
func (b *InMemoryBackend) UpdateFindingsFilter(
	id, name, description, action string,
	position *int32,
	criteria map[string]any,
) (*FindingsFilterSummary, error) {
	b.mu.Lock("UpdateFindingsFilter")
	defer b.mu.Unlock()

	ff, ok := b.findingsFilters.Get(id)
	if !ok {
		return nil, ErrFindingsFilterNotFound
	}

	if name != "" {
		ff.Name = name
	}

	ff.Description = description

	if action != "" {
		ff.Action = action
	}

	if position != nil {
		ff.Position = *position
	}

	if criteria != nil {
		ff.FindingCriteria = criteria
	}

	return &FindingsFilterSummary{
		Action:      ff.Action,
		Arn:         ff.Arn,
		Description: ff.Description,
		ID:          ff.ID,
		Name:        ff.Name,
		Position:    ff.Position,
		Tags:        maps.Clone(ff.Tags),
	}, nil
}

// DeleteFindingsFilter deletes a findings filter.
func (b *InMemoryBackend) DeleteFindingsFilter(id string) error {
	b.mu.Lock("DeleteFindingsFilter")
	defer b.mu.Unlock()

	if !b.findingsFilters.Delete(id) {
		return ErrFindingsFilterNotFound
	}
	delete(b.tags, b.findingsFilterARN(id))

	return nil
}

// ListFindingsFilters returns summaries of all findings filters.
//
//nolint:dupl // structurally identical to ListAllowLists but operates on a different type
func (b *InMemoryBackend) ListFindingsFilters(limit int, token string) ([]*FindingsFilterSummary, string, error) {
	return listPaginated(
		b, "ListFindingsFilters", b.findingsFilters.All(),
		func(ff *storedFindingsFilter) (*FindingsFilterSummary, bool) {
			return &FindingsFilterSummary{
				Action:      ff.Action,
				Arn:         ff.Arn,
				Description: ff.Description,
				ID:          ff.ID,
				Name:        ff.Name,
				Position:    ff.Position,
				Tags:        maps.Clone(ff.Tags),
			}, true
		},
		func(result []*FindingsFilterSummary) {
			// Position defaults to 1 for every filter that doesn't specify one
			// (CreateFindingsFilter), so it's not unique; ID as a tiebreaker keeps a
			// total order so the offset-based page.NewHMAC cursor can't drop or
			// duplicate filters that tie on Position.
			sort.Slice(result, func(i, j int) bool {
				if result[i].Position != result[j].Position {
					return result[i].Position < result[j].Position
				}

				return result[i].ID < result[j].ID
			})
		},
		token, limit,
	)
}
