package medialive

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// --- SdiSource operations ---

// CreateSdiSource creates a new SDI source. Unlike the resource kinds
// taggableResourceTags covers, real DescribeSdiSourceResult has no Tags
// field (verified: types.SdiSource, medialive@v1.101.4 types/types.go:7622),
// so tags go into the legacy per-ARN b.tags store, not the resource struct.
func (b *InMemoryBackend) CreateSdiSource(
	name, sdiType, mode string,
	tags map[string]string,
) (*SdiSource, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name required", ErrInvalidParameter)
	}

	if sdiType == "" {
		sdiType = sdiSourceTypeSingle
	}

	if mode == "" {
		mode = sdiSourceModeQuadrant
	}

	id := newID()
	s := &storedSdiSource{
		ARN:    b.sdiSourceARN(id),
		ID:     id,
		Name:   name,
		Type:   sdiType,
		Mode:   mode,
		State:  sdiSourceStateIdle,
		Inputs: []string{},
	}

	b.mu.Lock("CreateSdiSource")
	defer b.mu.Unlock()

	b.sdiSources.Put(s)
	b.tags[s.ARN] = copyTags(tags)

	return s.toSdiSource(), nil
}

// DescribeSdiSource returns an SDI source by ID.
func (b *InMemoryBackend) DescribeSdiSource(sdiSourceID string) (*SdiSource, error) {
	b.mu.RLock("DescribeSdiSource")
	defer b.mu.RUnlock()

	s, ok := b.sdiSources.Get(sdiSourceID)
	if !ok {
		return nil, fmt.Errorf("%w: sdiSource %s not found", ErrNotFound, sdiSourceID)
	}

	return s.toSdiSource(), nil
}

// UpdateSdiSource updates an SDI source's mutable fields.
func (b *InMemoryBackend) UpdateSdiSource(
	sdiSourceID, name, sdiType, mode string,
) (*SdiSource, error) {
	b.mu.Lock("UpdateSdiSource")
	defer b.mu.Unlock()

	s, ok := b.sdiSources.Get(sdiSourceID)
	if !ok {
		return nil, fmt.Errorf("%w: sdiSource %s not found", ErrNotFound, sdiSourceID)
	}

	if name != "" {
		s.Name = name
	}

	if sdiType != "" {
		s.Type = sdiType
	}

	if mode != "" {
		s.Mode = mode
	}

	return s.toSdiSource(), nil
}

// DeleteSdiSource deletes an SDI source. api_op_DeleteSdiSource.go's doc
// comment ("The SdiSource must not be part of any SidSourceMapping and must
// not be attached to any input.") requires the attachment check below; the
// SidSourceMapping half is not enforced -- gopherstack's Node model has no
// SdiSourceMappings field to check against (structural gap, not this bug).
func (b *InMemoryBackend) DeleteSdiSource(sdiSourceID string) (*SdiSource, error) {
	b.mu.Lock("DeleteSdiSource")
	defer b.mu.Unlock()

	s, ok := b.sdiSources.Get(sdiSourceID)
	if !ok {
		return nil, fmt.Errorf("%w: sdiSource %s not found", ErrNotFound, sdiSourceID)
	}

	if len(s.Inputs) > 0 {
		return nil, fmt.Errorf("%w: sdiSource %s is attached to an input", ErrConflict, sdiSourceID)
	}

	s.State = sdiSourceStateDeleted
	out := s.toSdiSource()
	b.sdiSources.Delete(sdiSourceID)
	delete(b.tags, s.ARN)

	return out, nil
}

// ListSdiSources returns a paginated list of SDI sources.
func (b *InMemoryBackend) ListSdiSources(
	maxResults int,
	nextToken string,
) ([]*SdiSource, string, error) {
	b.mu.RLock("ListSdiSources")
	defer b.mu.RUnlock()

	all := b.sdiSources.All()

	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	pg := page.New(all, nextToken, maxResults, defaultMaxResults)

	out := make([]*SdiSource, 0, len(pg.Data))
	for _, s := range pg.Data {
		out = append(out, s.toSdiSource())
	}

	return out, pg.Next, nil
}
