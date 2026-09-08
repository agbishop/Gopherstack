package omics

import (
	"fmt"
	"slices"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// ────────────────────────────────────────────────────────────────────────────
// SequenceStore
// ────────────────────────────────────────────────────────────────────────────

// sequenceStoreDefaultETagAlgorithm is the ETag algorithm family CreateSequenceStore
// applies when the caller omits eTagAlgorithmFamily (CreateSequenceStoreRequest
// documentation, botocore omics service-2.json).
const sequenceStoreDefaultETagAlgorithm = "MD5up"

// CreateSequenceStore creates a new sequence store.
func (b *InMemoryBackend) CreateSequenceStore(
	name, description, eTagAlgorithmFamily, accessLogLocation string,
	tags map[string]string,
) (*SequenceStore, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	b.mu.Lock("CreateSequenceStore")
	defer b.mu.Unlock()

	if eTagAlgorithmFamily == "" {
		eTagAlgorithmFamily = sequenceStoreDefaultETagAlgorithm
	}

	var s3Access map[string]any
	if accessLogLocation != "" {
		s3Access = map[string]any{"accessLogLocation": accessLogLocation}
	}

	now := time.Now().UTC()
	ss := &SequenceStore{
		ID:            newID(),
		Name:          name,
		Description:   description,
		Status:        statusActive,
		Tags:          copyTags(tags),
		CreationTime:  now,
		UpdateTime:    now,
		ETagAlgorithm: eTagAlgorithmFamily,
		S3Access:      s3Access,
	}
	ss.Arn = arn.Build("omics", b.defaultRegion, b.accountID, "sequenceStore/"+ss.ID)

	b.sequenceStores.Put(ss)
	b.uploadParts[ss.ID] = make(map[string][]*ReadSetUploadPart)
	b.uploadPartData[ss.ID] = make(map[string]map[string]map[int][]byte)
	b.readSetBytes[ss.ID] = make(map[string][]byte)

	if tags != nil {
		b.tags[ss.Arn] = copyTags(tags)
	}

	result := *ss

	return &result, nil
}

// DeleteSequenceStore deletes a sequence store by ID. Real AWS only permits
// this when the store contains no read sets
// (api_op_DeleteSequenceStore.go: "You can only delete a sequence store when
// it does not contain any read sets. Use the BatchDeleteReadSet API
// operation to ensure that all read sets in the sequence store are
// deleted.").
func (b *InMemoryBackend) DeleteSequenceStore(id string) error {
	b.mu.Lock("DeleteSequenceStore")
	defer b.mu.Unlock()

	ss, ok := b.sequenceStores.Get(id)
	if !ok {
		return fmt.Errorf("%w: sequence store %s not found", ErrNotFound, id)
	}

	if readSets := b.readSetsByStore.Get(id); len(readSets) > 0 {
		return fmt.Errorf(
			"%w: sequence store %s still contains %d read set(s), delete them first",
			ErrInvalidState, id, len(readSets),
		)
	}

	delete(b.tags, ss.Arn)
	b.sequenceStores.Delete(id)

	for _, rs := range slices.Clone(b.readSetsByStore.Get(id)) {
		b.readSets.Delete(parentKey(id, rs.ID))
	}

	for _, j := range slices.Clone(b.readSetActivationJobsByStore.Get(id)) {
		b.readSetActivationJobs.Delete(parentKey(id, j.ID))
	}

	for _, j := range slices.Clone(b.readSetExportJobsByStore.Get(id)) {
		b.readSetExportJobs.Delete(parentKey(id, j.ID))
	}

	for _, j := range slices.Clone(b.readSetImportJobsByStore.Get(id)) {
		b.readSetImportJobs.Delete(parentKey(id, j.ID))
	}

	for _, u := range slices.Clone(b.multipartUploadsByStore.Get(id)) {
		b.multipartUploads.Delete(parentKey(id, u.UploadID))
	}

	delete(b.uploadParts, id)
	delete(b.uploadPartData, id)
	delete(b.readSetBytes, id)

	return nil
}

// GetSequenceStore retrieves a sequence store by ID.
func (b *InMemoryBackend) GetSequenceStore(id string) (*SequenceStore, error) {
	b.mu.RLock("GetSequenceStore")
	defer b.mu.RUnlock()

	ss, ok := b.sequenceStores.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: sequence store %s not found", ErrNotFound, id)
	}

	result := *ss

	return &result, nil
}

// ListSequenceStores lists sequence stores.
func (b *InMemoryBackend) ListSequenceStores(
	filter *SequenceStoreFilter,
	maxResults int,
	nextToken string,
) ([]*SequenceStore, string, error) {
	b.mu.RLock("ListSequenceStores")
	defer b.mu.RUnlock()

	all := b.sequenceStores.All()
	ids := make([]string, 0, len(all))

	for _, ss := range all {
		if filter != nil && filter.Name != "" && ss.Name != filter.Name {
			continue
		}

		ids = append(ids, ss.ID)
	}

	result, outToken := paginatedCopies(ids, nextToken, maxResults, b.sequenceStores.Get)

	return result, outToken, nil
}

// UpdateSequenceStore updates a sequence store's name and description.
func (b *InMemoryBackend) UpdateSequenceStore(
	id, name, description string,
) (*SequenceStore, error) {
	b.mu.Lock("UpdateSequenceStore")
	defer b.mu.Unlock()

	ss, ok := b.sequenceStores.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: sequence store %s not found", ErrNotFound, id)
	}

	if name != "" {
		ss.Name = name
	}

	if description != "" {
		ss.Description = description
	}

	ss.UpdateTime = time.Now().UTC()
	result := *ss

	return &result, nil
}
