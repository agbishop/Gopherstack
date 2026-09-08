package omics

import (
	"fmt"
	"slices"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// ────────────────────────────────────────────────────────────────────────────
// ReferenceStore
// ────────────────────────────────────────────────────────────────────────────

// CreateReferenceStore creates a new reference store.
func (b *InMemoryBackend) CreateReferenceStore(
	name, description string,
	tags map[string]string,
) (*ReferenceStore, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	b.mu.Lock("CreateReferenceStore")
	defer b.mu.Unlock()

	rs := &ReferenceStore{
		ID:           newID(),
		Name:         name,
		Description:  description,
		Tags:         copyTags(tags),
		CreationTime: time.Now().UTC(),
	}
	rs.Arn = arn.Build("omics", b.defaultRegion, b.accountID, "referenceStore/"+rs.ID)

	b.referenceStores.Put(rs)
	b.referenceBytes[rs.ID] = make(map[string][]byte)

	if tags != nil {
		b.tags[rs.Arn] = copyTags(tags)
	}

	result := *rs

	return &result, nil
}

// DeleteReferenceStore deletes a reference store by ID. Real AWS only
// permits this when the store contains no reference genomes
// (api_op_DeleteReferenceStore.go: "You can only delete a reference store
// when it does not contain any reference genomes. To empty a reference
// store, use DeleteReference.").
func (b *InMemoryBackend) DeleteReferenceStore(id string) error {
	b.mu.Lock("DeleteReferenceStore")
	defer b.mu.Unlock()

	rs, ok := b.referenceStores.Get(id)
	if !ok {
		return fmt.Errorf("%w: reference store %s not found", ErrNotFound, id)
	}

	if refs := b.referencesByStore.Get(id); len(refs) > 0 {
		return fmt.Errorf(
			"%w: reference store %s still contains %d reference(s), delete them first",
			ErrInvalidState, id, len(refs),
		)
	}

	delete(b.tags, rs.Arn)
	b.referenceStores.Delete(id)

	for _, ref := range slices.Clone(b.referencesByStore.Get(id)) {
		b.references.Delete(parentKey(id, ref.ID))
	}

	for _, job := range slices.Clone(b.referenceImportJobsByStore.Get(id)) {
		b.referenceImportJobs.Delete(parentKey(id, job.ID))
	}

	delete(b.referenceBytes, id)

	return nil
}

// GetReferenceStore retrieves a reference store by ID.
func (b *InMemoryBackend) GetReferenceStore(id string) (*ReferenceStore, error) {
	b.mu.RLock("GetReferenceStore")
	defer b.mu.RUnlock()

	rs, ok := b.referenceStores.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: reference store %s not found", ErrNotFound, id)
	}

	result := *rs

	return &result, nil
}

// ListReferenceStores lists reference stores with optional name filter.
func (b *InMemoryBackend) ListReferenceStores(
	filter *ReferenceStoreFilter,
	maxResults int,
	nextToken string,
) ([]*ReferenceStore, string, error) {
	b.mu.RLock("ListReferenceStores")
	defer b.mu.RUnlock()

	all := b.referenceStores.All()
	ids := make([]string, 0, len(all))

	for _, rs := range all {
		if filter != nil && filter.Name != "" && rs.Name != filter.Name {
			continue
		}

		ids = append(ids, rs.ID)
	}

	result, outToken := paginatedCopies(ids, nextToken, maxResults, b.referenceStores.Get)

	return result, outToken, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Reference
// ────────────────────────────────────────────────────────────────────────────

// DeleteReference deletes a reference by store ID and reference ID. Real AWS
// requires any read set associated with the reference genome to be deleted
// first (api_op_DeleteReference.go: "The read set associated with the
// reference genome must first be deleted before deleting the reference
// genome.").
func (b *InMemoryBackend) DeleteReference(referenceStoreID, id string) error {
	b.mu.Lock("DeleteReference")
	defer b.mu.Unlock()

	if !b.referenceStores.Has(referenceStoreID) {
		return fmt.Errorf("%w: reference store %s not found", ErrNotFound, referenceStoreID)
	}

	ref, ok := b.references.Get(parentKey(referenceStoreID, id))
	if !ok {
		return fmt.Errorf("%w: reference %s not found", ErrNotFound, id)
	}

	for _, rs := range b.readSets.All() {
		if rs.ReferenceARN == ref.Arn {
			return fmt.Errorf(
				"%w: reference %s still has read set %s associated with it, delete it first",
				ErrInvalidState, id, rs.ID,
			)
		}
	}

	delete(b.tags, ref.Arn)
	b.references.Delete(parentKey(referenceStoreID, id))

	return nil
}

// GetReferenceMetadata retrieves reference metadata.
func (b *InMemoryBackend) GetReferenceMetadata(
	referenceStoreID, id string,
) (*ReferenceMetadata, error) {
	b.mu.RLock("GetReferenceMetadata")
	defer b.mu.RUnlock()

	if !b.referenceStores.Has(referenceStoreID) {
		return nil, fmt.Errorf("%w: reference store %s not found", ErrNotFound, referenceStoreID)
	}

	ref, ok := b.references.Get(parentKey(referenceStoreID, id))
	if !ok {
		return nil, fmt.Errorf("%w: reference %s not found", ErrNotFound, id)
	}

	result := *ref

	return &result, nil
}

// ListReferences lists references in a reference store.
//
//nolint:dupl // structurally-identical parent-scoped List op (already deduped via listChildFiltered)
func (b *InMemoryBackend) ListReferences(
	referenceStoreID string,
	filter *ReferenceFilter,
	maxResults int,
	nextToken string,
) ([]*ReferenceMetadata, string, error) {
	b.mu.RLock("ListReferences")
	defer b.mu.RUnlock()

	if !b.referenceStores.Has(referenceStoreID) {
		return nil, "", fmt.Errorf(
			"%w: reference store %s not found",
			ErrNotFound,
			referenceStoreID,
		)
	}

	group := b.referencesByStore.Get(referenceStoreID)
	result, outToken := listChildFiltered(
		group,
		func(ref *ReferenceMetadata) string { return ref.ID },
		func(ref *ReferenceMetadata) bool {
			return filter == nil || filter.Name == "" || ref.Name == filter.Name
		},
		nextToken, maxResults,
		func(id string) (*ReferenceMetadata, bool) { return b.references.Get(parentKey(referenceStoreID, id)) },
	)

	return result, outToken, nil
}

// StartReferenceImportJob creates a reference import job.
func (b *InMemoryBackend) StartReferenceImportJob(
	referenceStoreID, roleARN string,
	sources []ReferenceImportJobSource,
) (*ReferenceImportJob, error) {
	b.mu.Lock("StartReferenceImportJob")
	defer b.mu.Unlock()

	if !b.referenceStores.Has(referenceStoreID) {
		return nil, fmt.Errorf("%w: reference store %s not found", ErrNotFound, referenceStoreID)
	}

	job := &ReferenceImportJob{
		ID:               newID(),
		ReferenceStoreID: referenceStoreID,
		RoleARN:          roleARN,
		Sources:          sources,
		Status:           statusCompleted,
		CreationTime:     time.Now().UTC(),
	}
	now := time.Now().UTC()
	job.CompletionTime = &now

	// Create reference entries for each source
	for _, src := range sources {
		refID := newID()
		ref := &ReferenceMetadata{
			ID:               refID,
			ReferenceStoreID: referenceStoreID,
			Arn: arn.Build(
				"omics",
				b.defaultRegion,
				b.accountID,
				fmt.Sprintf("referenceStore/%s/reference/%s", referenceStoreID, refID),
			),
			Name:        src.Name,
			Description: src.Description,
			Status:      statusActive,
			// Files.source.contentLength reflects the empty body this backend actually
			// stores for imported references (see referenceBytes below, and
			// GetReferenceBytes) -- real AWS reads and hashes the S3 source file, which
			// this emulator has no way to do (real omics@v1.49.5 FileInformation has no
			// required members, so an honest partial value is valid here).
			Files: map[string]any{
				"source": map[string]any{keyContentLength: int64(0)},
			},
			CreationTime: time.Now().UTC(),
			UpdateTime:   time.Now().UTC(),
		}
		b.references.Put(ref)
		b.referenceBytes[referenceStoreID][refID] = []byte{}
	}

	b.referenceImportJobs.Put(job)

	result := *job

	return &result, nil
}

// GetReferenceImportJob retrieves a reference import job.
func (b *InMemoryBackend) GetReferenceImportJob(
	referenceStoreID, jobID string,
) (*ReferenceImportJob, error) {
	b.mu.RLock("GetReferenceImportJob")
	defer b.mu.RUnlock()

	if !b.referenceStores.Has(referenceStoreID) {
		return nil, fmt.Errorf("%w: reference store %s not found", ErrNotFound, referenceStoreID)
	}

	job, ok := b.referenceImportJobs.Get(parentKey(referenceStoreID, jobID))
	if !ok {
		return nil, fmt.Errorf("%w: reference import job %s not found", ErrNotFound, jobID)
	}

	result := *job

	return &result, nil
}

// ListReferenceImportJobs lists reference import jobs for a store, optionally
// filtered by status (real AWS ListReferenceImportJobsInput body "filter").
//
//nolint:dupl // structurally-identical parent-scoped List op (already deduped via listChildFiltered)
func (b *InMemoryBackend) ListReferenceImportJobs(
	referenceStoreID string,
	filter *ReferenceImportJobFilter,
	maxResults int,
	nextToken string,
) ([]*ReferenceImportJob, string, error) {
	b.mu.RLock("ListReferenceImportJobs")
	defer b.mu.RUnlock()

	if !b.referenceStores.Has(referenceStoreID) {
		return nil, "", fmt.Errorf(
			"%w: reference store %s not found",
			ErrNotFound,
			referenceStoreID,
		)
	}

	group := b.referenceImportJobsByStore.Get(referenceStoreID)
	result, outToken := listChildFiltered(
		group,
		func(j *ReferenceImportJob) string { return j.ID },
		func(j *ReferenceImportJob) bool {
			return filter == nil || filter.Status == "" || j.Status == filter.Status
		},
		nextToken, maxResults,
		func(id string) (*ReferenceImportJob, bool) {
			return b.referenceImportJobs.Get(parentKey(referenceStoreID, id))
		},
	)

	return result, outToken, nil
}

// GetReferenceBytes returns the stored binary body for a reference.
func (b *InMemoryBackend) GetReferenceBytes(referenceStoreID, id string) ([]byte, error) {
	b.mu.RLock("GetReferenceBytes")
	defer b.mu.RUnlock()

	if !b.references.Has(parentKey(referenceStoreID, id)) {
		return nil, fmt.Errorf("%w: reference %s not found", ErrNotFound, id)
	}

	return b.referenceBytes[referenceStoreID][id], nil
}
