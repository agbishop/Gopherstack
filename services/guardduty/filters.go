package guardduty

import (
	"maps"
	"slices"
	"time"
)

const filterActionArchive = "ARCHIVE"

// matchesArchiveFilter reports whether any of detectorID's ARCHIVE-action
// filters match f. CreateFilter's Action ("the action that is to be applied
// to the findings that match the filter") was previously stored and echoed
// back but never applied -- CreateSampleFindings is this backend's only
// finding-creation path, so it is the only place Action can take effect.
// Caller must already hold b.mu.
func (b *InMemoryBackend) matchesArchiveFilter(detectorID string, f *Finding) bool {
	for _, filt := range b.filtersByDetector.Get(detectorID) {
		if filt.Action == filterActionArchive && matchesFindingCriteria(f, conditionsFromRaw(filt.FindingCriteria)) {
			return true
		}
	}

	return false
}

// CreateFilter creates a new filter for a detector.
func (b *InMemoryBackend) CreateFilter(
	detectorID, name, description, action string,
	rank int32,
	findingCriteria map[string]any,
	tags map[string]string,
) (*Filter, error) {
	b.mu.Lock("CreateFilter")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	if b.filters.Has(detectorKey(detectorID, name)) {
		return nil, ErrFilterAlreadyExists
	}

	now := time.Now().UTC()
	f := &Filter{
		Name:            name,
		Description:     description,
		Action:          action,
		Rank:            rank,
		FindingCriteria: findingCriteria,
		Tags:            tags,
		DetectorID:      detectorID,
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	}
	b.filters.Put(f)

	arn := b.filterARN(detectorID, name)
	if tags != nil {
		b.tags[arn] = maps.Clone(tags)
	}

	return f, nil
}

// GetFilter retrieves a filter.
func (b *InMemoryBackend) GetFilter(detectorID, filterName string) (*Filter, error) {
	b.mu.RLock("GetFilter")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	f, ok := b.filters.Get(detectorKey(detectorID, filterName))
	if !ok {
		return nil, ErrFilterNotFound
	}

	return f, nil
}

// UpdateFilter updates a filter's configuration.
func (b *InMemoryBackend) UpdateFilter(
	detectorID, filterName, description, action string,
	rank int32,
	findingCriteria map[string]any,
) (*Filter, error) {
	b.mu.Lock("UpdateFilter")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	f, ok := b.filters.Get(detectorKey(detectorID, filterName))
	if !ok {
		return nil, ErrFilterNotFound
	}

	if description != "" {
		f.Description = description
	}

	if action != "" {
		f.Action = action
	}

	if rank > 0 {
		f.Rank = rank
	}

	if findingCriteria != nil {
		f.FindingCriteria = findingCriteria
	}

	f.UpdatedAt = time.Now().UTC()
	f.Version++

	return f, nil
}

// DeleteFilter removes a filter.
func (b *InMemoryBackend) DeleteFilter(detectorID, filterName string) error {
	b.mu.Lock("DeleteFilter")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	if !b.filters.Delete(detectorKey(detectorID, filterName)) {
		return ErrFilterNotFound
	}

	delete(b.tags, b.filterARN(detectorID, filterName))

	return nil
}

// ListFilters returns filter names for a detector.
func (b *InMemoryBackend) ListFilters(detectorID string, maxResults int32, nextToken string) ([]string, string, error) {
	b.mu.RLock("ListFilters")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, "", ErrDetectorNotFound
	}

	items := b.filtersByDetector.Get(detectorID)
	names := make([]string, len(items))

	for i, f := range items {
		names[i] = f.Name
	}

	slices.Sort(names)

	offset, err := decodeToken(nextToken)
	if err != nil {
		return nil, "", ErrValidation
	}

	size := resolvePageSize(int(maxResults))
	page, next := paginate(names, offset, size)

	return page, next, nil
}
