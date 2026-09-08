package fsx

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

type storedVolume struct {
	CreationTime            time.Time         `json:"creationTime"`
	Tags                    map[string]string `json:"tags"`
	VolumeID                string            `json:"volumeId"`
	VolumeType              string            `json:"volumeType"`
	FileSystemID            string            `json:"fileSystemId"`
	StorageVirtualMachineID string            `json:"storageVirtualMachineId,omitempty"`
	Name                    string            `json:"name"`
	Lifecycle               string            `json:"lifecycle"`
	ResourceARN             string            `json:"resourceArn"`
}

// toPublic renders v's wire shape. OntapConfiguration is only populated for
// ONTAP volumes: real AWS's OpenZFS volumes have no StorageVirtualMachineId
// concept at all (OpenZFS volumes nest under a parent volume, not an SVM).
func (v *storedVolume) toPublic() *Volume {
	vol := &Volume{
		CreationTime: epochTime(v.CreationTime),
		VolumeID:     v.VolumeID,
		VolumeType:   v.VolumeType,
		FileSystemID: v.FileSystemID,
		Name:         v.Name,
		Lifecycle:    v.Lifecycle,
		ResourceARN:  v.ResourceARN,
		Tags:         tagsMapToSlice(v.Tags),
	}

	if v.StorageVirtualMachineID != "" {
		vol.OntapConfiguration = &OntapVolumeConfiguration{StorageVirtualMachineID: v.StorageVirtualMachineID}
	}

	return vol
}

// createOntapVolumeConfigInput is the real CreateVolumeInput.OntapConfiguration
// shape (fsx@v1.68.4 types.CreateOntapVolumeConfiguration) -- only
// StorageVirtualMachineId (the sole required member) is modeled; the rest is
// out of this fix's scope (Volume's response has no OntapVolumeConfiguration
// at all, a pre-existing, disclosed Layer-3 gap).
type createOntapVolumeConfigInput struct {
	StorageVirtualMachineID string `json:"StorageVirtualMachineId"`
}

// createOpenZFSVolumeConfigInput is the real
// CreateVolumeInput.OpenZFSConfiguration shape (fsx@v1.68.4
// types.CreateOpenZFSVolumeConfiguration) -- only ParentVolumeId (the sole
// required member) is modeled, same scoping rationale as
// createOntapVolumeConfigInput.
type createOpenZFSVolumeConfigInput struct {
	ParentVolumeID string `json:"ParentVolumeId"`
}

// createVolumeInput mirrors the real CreateVolumeInput wire shape
// (fsx@v1.68.4 api_op_CreateVolume.go): there is no top-level FileSystemId
// or StorageVirtualMachineId at all -- StorageVirtualMachineId lives nested
// under OntapConfiguration (required for VolumeType=ONTAP) and the
// equivalent OpenZFS anchor is OpenZFSConfiguration.ParentVolumeId (required
// for VolumeType=OPENZFS); FileSystemId is derived server-side from
// whichever anchor resolves.
type createVolumeInput struct {
	VolumeType           string                          `json:"VolumeType"`
	Name                 string                          `json:"Name"`
	OntapConfiguration   *createOntapVolumeConfigInput   `json:"OntapConfiguration,omitempty"`
	OpenZFSConfiguration *createOpenZFSVolumeConfigInput `json:"OpenZFSConfiguration,omitempty"`
	Tags                 []Tag                           `json:"Tags,omitempty"`
}

// CreateVolume creates a volume. Real AWS requires OntapConfiguration for
// VolumeType=ONTAP and OpenZFSConfiguration for VolumeType=OPENZFS
// (types.MissingVolumeConfiguration, "A volume configuration is required for
// this operation.") -- there is no other way for a real client to name the
// parent file system, since CreateVolumeInput carries neither a
// FileSystemId nor a top-level StorageVirtualMachineId.
func (b *InMemoryBackend) CreateVolume(input *createVolumeInput) (*Volume, error) {
	if input.VolumeType == "" {
		return nil, ErrValidation
	}

	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateVolume")
	defer b.mu.Unlock()

	fileSystemID, svmID, err := b.resolveVolumeParentLocked(input)
	if err != nil {
		return nil, err
	}

	id := newFSxVolumeID()
	arn := b.volumeARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	v := &storedVolume{
		CreationTime:            now,
		Tags:                    tags,
		VolumeID:                id,
		VolumeType:              input.VolumeType,
		FileSystemID:            fileSystemID,
		StorageVirtualMachineID: svmID,
		Name:                    input.Name,
		Lifecycle:               lifecycleAvailable,
		ResourceARN:             arn,
	}

	b.volumes.Put(v)
	b.tags[arn] = tags

	return v.toPublic(), nil
}

// resolveVolumeParentLocked validates and resolves CreateVolume's per-type
// configuration block into (FileSystemId, StorageVirtualMachineId). Caller
// must already hold b.mu.
func (b *InMemoryBackend) resolveVolumeParentLocked(input *createVolumeInput) (string, string, error) {
	switch input.VolumeType {
	case fileSystemTypeONTAP:
		if input.OntapConfiguration == nil || input.OntapConfiguration.StorageVirtualMachineID == "" {
			return "", "", ErrMissingVolumeConfiguration
		}

		svm, ok := b.storageVirtualMachines.Get(input.OntapConfiguration.StorageVirtualMachineID)
		if !ok {
			return "", "", ErrStorageVirtualMachineNotFound
		}

		return svm.FileSystemID, svm.StorageVirtualMachineID, nil
	case fileSystemTypeOpenZFS:
		if input.OpenZFSConfiguration == nil || input.OpenZFSConfiguration.ParentVolumeID == "" {
			return "", "", ErrMissingVolumeConfiguration
		}

		parent, ok := b.volumes.Get(input.OpenZFSConfiguration.ParentVolumeID)
		if !ok {
			return "", "", ErrVolumeNotFound
		}

		return parent.FileSystemID, "", nil
	default:
		return "", "", nil
	}
}

// createVolumeFromBackupInput mirrors the real CreateVolumeFromBackupInput
// wire shape (fsx@v1.68.4 api_op_CreateVolumeFromBackup.go): there is no
// top-level VolumeType or StorageVirtualMachineId at all -- the operation is
// ONTAP-only, and the SVM anchor lives nested under
// OntapConfiguration.StorageVirtualMachineId, exactly like CreateVolume's own
// OntapConfiguration (see createOntapVolumeConfigInput, shared here).
type createVolumeFromBackupInput struct {
	BackupID           string                        `json:"BackupId"`
	Name               string                        `json:"Name"`
	OntapConfiguration *createOntapVolumeConfigInput `json:"OntapConfiguration,omitempty"`
	Tags               []Tag                         `json:"Tags,omitempty"`
}

// CreateVolumeFromBackup creates an ONTAP volume from a backup. Real AWS
// requires OntapConfiguration.StorageVirtualMachineId to name the target SVM
// (types.MissingVolumeConfiguration otherwise) -- there is no other way for a
// real client to specify it, since CreateVolumeFromBackupInput carries no
// top-level StorageVirtualMachineId.
func (b *InMemoryBackend) CreateVolumeFromBackup(input *createVolumeFromBackupInput) (*Volume, error) {
	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	if input.OntapConfiguration == nil || input.OntapConfiguration.StorageVirtualMachineID == "" {
		return nil, ErrMissingVolumeConfiguration
	}

	b.mu.Lock("CreateVolumeFromBackup")
	defer b.mu.Unlock()

	if !b.backups.Has(input.BackupID) {
		return nil, ErrBackupNotFound
	}

	svm, ok := b.storageVirtualMachines.Get(input.OntapConfiguration.StorageVirtualMachineID)
	if !ok {
		return nil, ErrStorageVirtualMachineNotFound
	}

	id := newFSxVolumeID()
	arn := b.volumeARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	v := &storedVolume{
		CreationTime:            now,
		Tags:                    tags,
		VolumeID:                id,
		VolumeType:              fileSystemTypeONTAP,
		FileSystemID:            svm.FileSystemID,
		StorageVirtualMachineID: svm.StorageVirtualMachineID,
		Name:                    input.Name,
		Lifecycle:               lifecycleAvailable,
		ResourceARN:             arn,
	}

	b.volumes.Put(v)
	b.tags[arn] = tags

	return v.toPublic(), nil
}

// DeleteVolume removes a volume.
func (b *InMemoryBackend) DeleteVolume(volumeID string) error {
	b.mu.Lock("DeleteVolume")
	defer b.mu.Unlock()

	if !b.volumes.Has(volumeID) {
		return ErrVolumeNotFound
	}

	b.deleteVolumeLocked(volumeID)

	return nil
}

// deleteVolumeLocked removes a volume and cascades to its snapshots, so no
// ghost Snapshot rows (pointing at a now-nonexistent VolumeId) survive the
// volume's deletion. Caller must already hold b.mu and have verified the
// volume exists.
func (b *InMemoryBackend) deleteVolumeLocked(volumeID string) {
	v, ok := b.volumes.Get(volumeID)
	if !ok {
		return
	}

	var snapshotIDs []string

	b.snapshots.Range(func(s *storedSnapshot) bool {
		if s.VolumeID == volumeID {
			snapshotIDs = append(snapshotIDs, s.SnapshotID)
		}

		return true
	})

	for _, id := range snapshotIDs {
		if snap, found := b.snapshots.Get(id); found {
			delete(b.tags, snap.ResourceARN)
		}

		b.snapshots.Delete(id)
	}

	b.volumes.Delete(volumeID)
	delete(b.tags, v.ResourceARN)
}

// createOpenZFSRootVolumeLocked creates the backing root volume that real
// AWS auto-creates for every FSx for OpenZFS file system, and returns its
// VolumeId. Caller must already hold b.mu.
func (b *InMemoryBackend) createOpenZFSRootVolumeLocked(fs *storedFileSystem) string {
	id := newFSxVolumeID()
	arn := b.volumeARN(id)
	tags := make(map[string]string)

	v := &storedVolume{
		CreationTime: fs.CreationTime,
		Tags:         tags,
		VolumeID:     id,
		VolumeType:   fileSystemTypeOpenZFS,
		FileSystemID: fs.FileSystemID,
		Name:         openZFSRootVolumeName,
		Lifecycle:    lifecycleAvailable,
		ResourceARN:  arn,
	}

	b.volumes.Put(v)
	b.tags[arn] = tags

	return id
}

// DescribeVolumes returns volumes, optionally filtered by ID or Filters.
// Real VolumeFilterName (aws-sdk-go-v2/service/fsx@v1.68.4 types/enums.go)
// has 2 values: file-system-id, storage-virtual-machine-id -- both tracked
// directly on storedVolume.
func (b *InMemoryBackend) DescribeVolumes( //nolint:dupl // existing issue.
	ids []string,
	filters []wireFilter,
	maxResults int32,
	nextToken string,
) ([]*Volume, string, error) {
	b.mu.RLock("DescribeVolumes")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedVolume

	if len(ids) > 0 {
		for _, id := range ids {
			v, ok := b.volumes.Get(id)
			if !ok {
				return nil, "", ErrVolumeNotFound
			}

			all = append(all, v)
		}
	} else {
		for _, v := range b.volumes.All() {
			if matchesFilters(filters, func(name string) (string, bool) {
				switch name {
				case filterNameFileSystemID:
					return v.FileSystemID, true
				case "storage-virtual-machine-id":
					return v.StorageVirtualMachineID, true
				default:
					return "", false
				}
			}) {
				all = append(all, v)
			}
		}

		sort.Slice(all, func(i, j int) bool { return all[i].VolumeID < all[j].VolumeID })
	}

	start, end, next := paginate(len(all), int(maxResults), nextToken, func(i int) string {
		return all[i].VolumeID
	})

	result := make([]*Volume, end-start)
	for i, v := range all[start:end] {
		result[i] = v.toPublic()
	}

	return result, next, nil
}

type restoreVolumeFromSnapshotInput struct {
	VolumeID   string `json:"VolumeId"`
	SnapshotID string `json:"SnapshotId"`
}

// RestoreVolumeFromSnapshot restores a volume to a snapshot state.
//
// The VolumeId check below stays on ErrVolumeNotFound -- this op's own
// switch (fsx@v1.68.4 deserializers.go
// deserializeOpErrorRestoreVolumeFromSnapshot) is [BadRequest,
// InternalServerError, VolumeNotFound], which declares it. SnapshotNotFound
// is not declared here (unlike its legitimate declarers
// DeleteSnapshot/DescribeSnapshots/UpdateSnapshot), so the SnapshotId check
// uses ErrValidation (BadRequest) instead, this op's own declared
// generic-client-error type (gopherstack-6flj/uox6 error-envelope sweep).
func (b *InMemoryBackend) RestoreVolumeFromSnapshot(input *restoreVolumeFromSnapshotInput) (*Volume, error) {
	b.mu.Lock("RestoreVolumeFromSnapshot")
	defer b.mu.Unlock()

	v, ok := b.volumes.Get(input.VolumeID)
	if !ok {
		return nil, ErrVolumeNotFound
	}

	if !b.snapshots.Has(input.SnapshotID) {
		return nil, fmt.Errorf("%w: snapshot %q not found", ErrValidation, input.SnapshotID)
	}

	return v.toPublic(), nil
}

type updateVolumeInput struct {
	VolumeID string `json:"VolumeId"`
	Name     string `json:"Name,omitempty"`
}

// UpdateVolume updates volume metadata.
func (b *InMemoryBackend) UpdateVolume(input *updateVolumeInput) (*Volume, error) {
	b.mu.Lock("UpdateVolume")
	defer b.mu.Unlock()

	v, ok := b.volumes.Get(input.VolumeID)
	if !ok {
		return nil, ErrVolumeNotFound
	}

	if input.Name != "" {
		v.Name = input.Name
	}

	return v.toPublic(), nil
}

func (b *InMemoryBackend) volumeARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("volume/%s", id))
}
