package fsx

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

type storedStorageVirtualMachine struct {
	CreationTime            time.Time         `json:"creationTime"`
	Tags                    map[string]string `json:"tags"`
	StorageVirtualMachineID string            `json:"storageVirtualMachineId"`
	FileSystemID            string            `json:"fileSystemId"`
	Name                    string            `json:"name"`
	Lifecycle               string            `json:"lifecycle"`
	ResourceARN             string            `json:"resourceArn"`
	Subtype                 string            `json:"subtype,omitempty"`
	RootVolumeSecurityStyle string            `json:"rootVolumeSecurityStyle,omitempty"`
}

func (s *storedStorageVirtualMachine) toPublic() *StorageVirtualMachine {
	return &StorageVirtualMachine{
		CreationTime:            epochTime(s.CreationTime),
		StorageVirtualMachineID: s.StorageVirtualMachineID,
		FileSystemID:            s.FileSystemID,
		Name:                    s.Name,
		Lifecycle:               s.Lifecycle,
		ResourceARN:             s.ResourceARN,
		Subtype:                 s.Subtype,
		RootVolumeSecurityStyle: s.RootVolumeSecurityStyle,
		Tags:                    tagsMapToSlice(s.Tags),
	}
}

type createStorageVirtualMachineInput struct {
	FileSystemID            string `json:"FileSystemId"`
	Name                    string `json:"Name"`
	Subtype                 string `json:"Subtype,omitempty"`
	RootVolumeSecurityStyle string `json:"RootVolumeSecurityStyle,omitempty"`
	Tags                    []Tag  `json:"Tags,omitempty"`
}

// CreateStorageVirtualMachine creates an SVM on an ONTAP file system.
func (b *InMemoryBackend) CreateStorageVirtualMachine(
	input *createStorageVirtualMachineInput,
) (*StorageVirtualMachine, error) {
	if input.FileSystemID == "" {
		return nil, ErrValidation
	}

	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateStorageVirtualMachine")
	defer b.mu.Unlock()

	if !b.fileSystems.Has(input.FileSystemID) {
		return nil, ErrFileSystemNotFound
	}

	id := newStorageVirtualMachineID()
	arn := b.svmARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	svm := &storedStorageVirtualMachine{
		CreationTime:            now,
		Tags:                    tags,
		StorageVirtualMachineID: id,
		FileSystemID:            input.FileSystemID,
		Name:                    input.Name,
		Lifecycle:               lifecycleAvailable,
		ResourceARN:             arn,
		Subtype:                 input.Subtype,
		RootVolumeSecurityStyle: input.RootVolumeSecurityStyle,
	}

	b.storageVirtualMachines.Put(svm)
	b.tags[arn] = tags

	return svm.toPublic(), nil
}

// DeleteStorageVirtualMachine removes an SVM. Real AWS: "Prior to deleting
// an SVM, you must delete all non-root volumes in the SVM, otherwise the
// operation will fail".
func (b *InMemoryBackend) DeleteStorageVirtualMachine(svmID string) error {
	b.mu.Lock("DeleteStorageVirtualMachine")
	defer b.mu.Unlock()

	if !b.storageVirtualMachines.Has(svmID) {
		return ErrStorageVirtualMachineNotFound
	}

	if err := b.requireNoSVMVolumesLocked(svmID); err != nil {
		return err
	}

	b.deleteStorageVirtualMachineLocked(svmID)

	return nil
}

// requireNoSVMVolumesLocked returns ErrValidation if svmID still hosts any
// volumes. Caller must already hold b.mu.
func (b *InMemoryBackend) requireNoSVMVolumesLocked(svmID string) error {
	var hasVolume bool

	b.volumes.Range(func(v *storedVolume) bool {
		if v.StorageVirtualMachineID == svmID {
			hasVolume = true

			return false
		}

		return true
	})

	if hasVolume {
		return fmt.Errorf("%w: storage virtual machine %s has volumes; delete them first", ErrValidation, svmID)
	}

	return nil
}

// deleteStorageVirtualMachineLocked removes an SVM and cascades to every
// volume hosted on it (which in turn cascades to that volume's snapshots),
// so no ghost Volume/Snapshot rows survive the SVM's deletion. Caller must
// already hold b.mu and have verified the SVM exists.
func (b *InMemoryBackend) deleteStorageVirtualMachineLocked(svmID string) {
	svm, ok := b.storageVirtualMachines.Get(svmID)
	if !ok {
		return
	}

	var volumeIDs []string

	b.volumes.Range(func(v *storedVolume) bool {
		if v.StorageVirtualMachineID == svmID {
			volumeIDs = append(volumeIDs, v.VolumeID)
		}

		return true
	})

	for _, id := range volumeIDs {
		b.deleteVolumeLocked(id)
	}

	b.storageVirtualMachines.Delete(svmID)
	delete(b.tags, svm.ResourceARN)
}

// DescribeStorageVirtualMachines returns SVMs, optionally filtered by ID or
// Filters. Real StorageVirtualMachineFilterName (aws-sdk-go-v2/service/fsx@v1.68.4
// types/enums.go) has exactly one value, file-system-id.
func (b *InMemoryBackend) DescribeStorageVirtualMachines( //nolint:dupl // existing issue.
	ids []string,
	filters []wireFilter,
	maxResults int32,
	nextToken string,
) ([]*StorageVirtualMachine, string, error) {
	b.mu.RLock("DescribeStorageVirtualMachines")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedStorageVirtualMachine

	if len(ids) > 0 {
		for _, id := range ids {
			svm, ok := b.storageVirtualMachines.Get(id)
			if !ok {
				return nil, "", ErrStorageVirtualMachineNotFound
			}

			all = append(all, svm)
		}
	} else {
		for _, svm := range b.storageVirtualMachines.All() {
			if matchesFilters(filters, func(name string) (string, bool) {
				if name == filterNameFileSystemID {
					return svm.FileSystemID, true
				}

				return "", false
			}) {
				all = append(all, svm)
			}
		}

		sort.Slice(all, func(i, j int) bool {
			return all[i].StorageVirtualMachineID < all[j].StorageVirtualMachineID
		})
	}

	start, end, next := paginate(len(all), int(maxResults), nextToken, func(i int) string {
		return all[i].StorageVirtualMachineID
	})

	result := make([]*StorageVirtualMachine, end-start)
	for i, svm := range all[start:end] {
		result[i] = svm.toPublic()
	}

	return result, next, nil
}

type updateStorageVirtualMachineInput struct {
	StorageVirtualMachineID string `json:"StorageVirtualMachineId"`
	Subtype                 string `json:"Subtype,omitempty"`
}

// UpdateStorageVirtualMachine updates an SVM.
func (b *InMemoryBackend) UpdateStorageVirtualMachine(
	input *updateStorageVirtualMachineInput,
) (*StorageVirtualMachine, error) {
	b.mu.Lock("UpdateStorageVirtualMachine")
	defer b.mu.Unlock()

	svm, ok := b.storageVirtualMachines.Get(input.StorageVirtualMachineID)
	if !ok {
		return nil, ErrStorageVirtualMachineNotFound
	}

	if input.Subtype != "" {
		svm.Subtype = input.Subtype
	}

	return svm.toPublic(), nil
}

func (b *InMemoryBackend) svmARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("storage-virtual-machine/%s", id))
}
