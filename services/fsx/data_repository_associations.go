package fsx

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

type storedDataRepositoryAssoc struct {
	CreationTime       time.Time         `json:"creationTime"`
	Tags               map[string]string `json:"tags"`
	AssociationID      string            `json:"associationId"`
	FileSystemID       string            `json:"fileSystemId"`
	FileSystemPath     string            `json:"fileSystemPath"`
	DataRepositoryPath string            `json:"dataRepositoryPath"`
	Lifecycle          string            `json:"lifecycle"`
	ResourceARN        string            `json:"resourceArn"`
}

func (a *storedDataRepositoryAssoc) toPublic() *DataRepositoryAssociation {
	return &DataRepositoryAssociation{
		CreationTime:       epochTime(a.CreationTime),
		AssociationID:      a.AssociationID,
		FileSystemID:       a.FileSystemID,
		FileSystemPath:     a.FileSystemPath,
		DataRepositoryPath: a.DataRepositoryPath,
		Lifecycle:          a.Lifecycle,
		ResourceARN:        a.ResourceARN,
		Tags:               tagsMapToSlice(a.Tags),
	}
}

type createDataRepositoryAssociationInput struct {
	FileSystemID       string `json:"FileSystemId"`
	FileSystemPath     string `json:"FileSystemPath"`
	DataRepositoryPath string `json:"DataRepositoryPath"`
	Tags               []Tag  `json:"Tags,omitempty"`
}

// CreateDataRepositoryAssociation creates a data repository association.
func (b *InMemoryBackend) CreateDataRepositoryAssociation(
	input *createDataRepositoryAssociationInput,
) (*DataRepositoryAssociation, error) {
	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateDataRepositoryAssociation")
	defer b.mu.Unlock()

	if !b.fileSystems.Has(input.FileSystemID) {
		return nil, ErrFileSystemNotFound
	}

	id := newDataRepositoryAssociationID()
	arn := b.draARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	a := &storedDataRepositoryAssoc{
		CreationTime:       now,
		Tags:               tags,
		AssociationID:      id,
		FileSystemID:       input.FileSystemID,
		FileSystemPath:     input.FileSystemPath,
		DataRepositoryPath: input.DataRepositoryPath,
		Lifecycle:          lifecycleAvailable,
		ResourceARN:        arn,
	}

	b.dataRepositoryAssocs.Put(a)
	b.tags[arn] = tags

	return a.toPublic(), nil
}

// DeleteDataRepositoryAssociation removes a data repository association.
func (b *InMemoryBackend) DeleteDataRepositoryAssociation(associationID string) error {
	b.mu.Lock("DeleteDataRepositoryAssociation")
	defer b.mu.Unlock()

	a, ok := b.dataRepositoryAssocs.Get(associationID)
	if !ok {
		return ErrDataRepositoryAssociationNotFound
	}

	b.dataRepositoryAssocs.Delete(associationID)
	delete(b.tags, a.ResourceARN)

	return nil
}

// DescribeDataRepositoryAssociations returns DRAs, optionally filtered by ID
// or Filters. Real DescribeDataRepositoryAssociationsInput.Filters
// (aws-sdk-go-v2/service/fsx@v1.68.4 api_op_DescribeDataRepositoryAssociations.go)
// reuses the same types.Filter/FilterName as DescribeBackups, but a DRA has
// no backup-type/volume-id/file-cache-* concept of its own -- only
// file-system-id is recognized here; the other enum values are honored as
// documented-but-unsupported (matches everything, same as an unset filter).
func (b *InMemoryBackend) DescribeDataRepositoryAssociations( //nolint:dupl // existing issue.
	ids []string,
	filters []wireFilter,
	maxResults int32,
	nextToken string,
) ([]*DataRepositoryAssociation, string, error) {
	b.mu.RLock("DescribeDataRepositoryAssociations")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedDataRepositoryAssoc

	if len(ids) > 0 {
		for _, id := range ids {
			a, ok := b.dataRepositoryAssocs.Get(id)
			if !ok {
				return nil, "", ErrDataRepositoryAssociationNotFound
			}

			all = append(all, a)
		}
	} else {
		for _, a := range b.dataRepositoryAssocs.All() {
			if matchesFilters(filters, func(name string) (string, bool) {
				if name == filterNameFileSystemID {
					return a.FileSystemID, true
				}

				return "", false
			}) {
				all = append(all, a)
			}
		}

		sort.Slice(all, func(i, j int) bool { return all[i].AssociationID < all[j].AssociationID })
	}

	start, end, next := paginate(len(all), int(maxResults), nextToken, func(i int) string {
		return all[i].AssociationID
	})

	result := make([]*DataRepositoryAssociation, end-start)
	for i, a := range all[start:end] {
		result[i] = a.toPublic()
	}

	return result, next, nil
}

type updateDataRepositoryAssociationInput struct {
	AssociationID      string `json:"AssociationId"`
	FileSystemPath     string `json:"FileSystemPath,omitempty"`
	DataRepositoryPath string `json:"DataRepositoryPath,omitempty"`
}

// UpdateDataRepositoryAssociation updates a DRA's paths.
func (b *InMemoryBackend) UpdateDataRepositoryAssociation(
	input *updateDataRepositoryAssociationInput,
) (*DataRepositoryAssociation, error) {
	b.mu.Lock("UpdateDataRepositoryAssociation")
	defer b.mu.Unlock()

	a, ok := b.dataRepositoryAssocs.Get(input.AssociationID)
	if !ok {
		return nil, ErrDataRepositoryAssociationNotFound
	}

	if input.FileSystemPath != "" {
		a.FileSystemPath = input.FileSystemPath
	}

	if input.DataRepositoryPath != "" {
		a.DataRepositoryPath = input.DataRepositoryPath
	}

	return a.toPublic(), nil
}

func (b *InMemoryBackend) draARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("association/%s", id))
}
