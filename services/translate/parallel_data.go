package translate

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) parallelDataARN(name string) string {
	return arn.Build("translate", b.region, b.accountID, "parallel-data/"+name)
}

// validateParallelDataConfig rejects a ParallelDataConfig.Format outside the
// modeled CSV|TMX|TSV enum. Format is not itself a required member of the
// ParallelDataConfig shape (api-2.json), so an absent Format is left to
// default rather than rejected -- only a present-but-invalid value errors,
// matching ImportTerminology's Directionality precedent.
func validateParallelDataConfig(cfg *ParallelDataConfig) error {
	if cfg == nil || cfg.Format == "" {
		return nil
	}

	if !validDataFormatsTable()[cfg.Format] {
		return fmt.Errorf("%w: ParallelDataConfig.Format must be one of CSV, TMX, TSV", ErrValidation)
	}

	return nil
}

// CreateParallelData creates a new parallel data resource.
func (b *InMemoryBackend) CreateParallelData(
	name, description string,
	cfg *ParallelDataConfig,
	encKey *EncryptionKey,
	tags map[string]string,
) (*ParallelData, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if err := validateParallelDataConfig(cfg); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateParallelData")
	defer b.mu.Unlock()

	if b.parallelData.Has(name) {
		return nil, fmt.Errorf("%w: parallel data %q already exists", ErrConflict, name)
	}

	// CreateParallelData writes a brand-new resource (nothing to merge
	// with), so -- like ImportTerminology -- the 50-tag limit applies to the
	// new set's size directly.
	if len(tags) > maxTagsPerResource {
		return nil, fmt.Errorf(
			"%w: parallel data %q would exceed the %d-tag limit",
			ErrTooManyTags,
			name,
			maxTagsPerResource,
		)
	}

	now := time.Now().UTC()
	resourceARN := b.parallelDataARN(name)

	pd := &ParallelData{
		ARN:                resourceARN,
		Name:               name,
		Description:        description,
		ParallelDataConfig: cfg,
		EncryptionKey:      encKey,
		Tags:               tags,
		CreatedAt:          now,
		LastUpdatedAt:      now,
		// Real CreateParallelData starts the resource at CREATING; it only
		// becomes ACTIVE once GetParallelData observes the transition (see
		// advanceParallelData) -- matching advanceJob's "advance on poll"
		// convention for translation jobs.
		Status:         parallelDataStatusCreating,
		SourceLanguage: "en",
	}
	b.parallelData.Put(pd)

	if tags != nil {
		b.tags[resourceARN] = copyMap(tags)
	}

	return pd, nil
}

// advanceParallelData moves pd one step through its async lifecycle, called
// from GetParallelData so that each poll makes progress -- the same
// "advance on poll" convention text_translation_jobs.go's advanceJob
// establishes for DescribeTextTranslationJob. ListParallelData intentionally
// does not call this (matching ListTextTranslationJobs's pure-read
// convention): real List operations do not mutate state.
func advanceParallelData(pd *ParallelData) {
	switch pd.Status {
	case parallelDataStatusCreating:
		pd.Status = parallelDataStatusActive
	case parallelDataStatusUpdating:
		pd.Status = parallelDataStatusActive
		pd.LatestUpdateAttemptStatus = parallelDataStatusActive
	}
}

// GetParallelData retrieves a parallel data resource by name and advances it
// one step through its async CREATING/UPDATING -> ACTIVE lifecycle. This
// takes the write lock (not RLock) because advanceParallelData mutates state,
// matching DescribeTextTranslationJob's documented precedent in
// text_translation_jobs.go.
func (b *InMemoryBackend) GetParallelData(name string) (*ParallelData, error) {
	b.mu.Lock("GetParallelData")
	defer b.mu.Unlock()

	pd, ok := b.parallelData.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: parallel data %q not found", ErrNotFound, name)
	}

	advanceParallelData(pd)

	return pd, nil
}

// UpdateParallelData updates an existing parallel data resource. Real AWS
// puts the resource into UPDATING (and records a new
// LatestUpdateAttemptStatus/LatestUpdateAttemptAt attempt) until the update
// completes asynchronously; GetParallelData's advanceParallelData call
// carries it to ACTIVE, matching CreateParallelData's CREATING -> ACTIVE
// lifecycle above.
func (b *InMemoryBackend) UpdateParallelData(
	name, description string,
	cfg *ParallelDataConfig,
) (*ParallelData, error) {
	if err := validateParallelDataConfig(cfg); err != nil {
		return nil, err
	}

	b.mu.Lock("UpdateParallelData")
	defer b.mu.Unlock()

	pd, ok := b.parallelData.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: parallel data %q not found", ErrNotFound, name)
	}

	// A prior Create/Update on this resource hasn't been observed to
	// completion yet (GetParallelData's advanceParallelData call is what
	// carries CREATING/UPDATING to ACTIVE) -- exactly the "another
	// modification is being made" case ConcurrentModificationException
	// documents.
	if pd.Status == parallelDataStatusCreating || pd.Status == parallelDataStatusUpdating {
		return nil, fmt.Errorf(
			"%w: parallel data %q has a modification in progress (status: %s)",
			ErrConcurrentModification, name, pd.Status,
		)
	}

	pd.Description = description
	if cfg != nil {
		pd.ParallelDataConfig = cfg
	}

	now := time.Now().UTC()
	pd.LastUpdatedAt = now
	pd.LatestUpdateAttemptAt = now
	pd.Status = parallelDataStatusUpdating
	pd.LatestUpdateAttemptStatus = parallelDataStatusUpdating

	return pd, nil
}

// DeleteParallelData removes a parallel data resource by name.
func (b *InMemoryBackend) DeleteParallelData(name string) (*ParallelData, error) {
	b.mu.Lock("DeleteParallelData")
	defer b.mu.Unlock()

	pd, ok := b.parallelData.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: parallel data %q not found", ErrNotFound, name)
	}

	resourceARN := b.parallelDataARN(name)
	b.parallelData.Delete(name)
	delete(b.tags, resourceARN)

	return pd, nil
}

// ListParallelData returns a paginated list of parallel data resources.
func (b *InMemoryBackend) ListParallelData(maxResults int, nextToken string) ([]*ParallelData, string) {
	b.mu.RLock("ListParallelData")
	defer b.mu.RUnlock()

	names := sortedNames(b.parallelData.All(), func(pd *ParallelData) string { return pd.Name })

	return paginate(names, func(n string) *ParallelData { return tableGet(b.parallelData, n) }, maxResults, nextToken)
}
