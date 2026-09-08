package fsx

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

type storedSnapshot struct {
	CreationTime time.Time         `json:"creationTime"`
	Tags         map[string]string `json:"tags"`
	SnapshotID   string            `json:"snapshotId"`
	VolumeID     string            `json:"volumeId"`
	Name         string            `json:"name"`
	Lifecycle    string            `json:"lifecycle"`
	ResourceARN  string            `json:"resourceArn"`
}

func (s *storedSnapshot) toPublic() *Snapshot {
	return &Snapshot{
		CreationTime: epochTime(s.CreationTime),
		SnapshotID:   s.SnapshotID,
		VolumeID:     s.VolumeID,
		Name:         s.Name,
		Lifecycle:    s.Lifecycle,
		ResourceARN:  s.ResourceARN,
		Tags:         tagsMapToSlice(s.Tags),
	}
}

type createSnapshotInput struct {
	VolumeID string `json:"VolumeId"`
	Name     string `json:"Name"`
	Tags     []Tag  `json:"Tags,omitempty"`
}

// CreateSnapshot creates a snapshot of a volume.
func (b *InMemoryBackend) CreateSnapshot(input *createSnapshotInput) (*Snapshot, error) {
	if input.VolumeID == "" {
		return nil, ErrValidation
	}

	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateSnapshot")
	defer b.mu.Unlock()

	if !b.volumes.Has(input.VolumeID) {
		return nil, ErrVolumeNotFound
	}

	id := newFSxVolumeSnapshotID()
	arn := b.snapshotARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	s := &storedSnapshot{
		CreationTime: now,
		Tags:         tags,
		SnapshotID:   id,
		VolumeID:     input.VolumeID,
		Name:         input.Name,
		Lifecycle:    lifecycleAvailable,
		ResourceARN:  arn,
	}

	b.snapshots.Put(s)
	b.tags[arn] = tags

	return s.toPublic(), nil
}

// DeleteSnapshot removes a snapshot.
func (b *InMemoryBackend) DeleteSnapshot(snapshotID string) error {
	b.mu.Lock("DeleteSnapshot")
	defer b.mu.Unlock()

	s, ok := b.snapshots.Get(snapshotID)
	if !ok {
		return ErrSnapshotNotFound
	}

	b.snapshots.Delete(snapshotID)
	delete(b.tags, s.ResourceARN)

	return nil
}

// DescribeSnapshots returns snapshots, optionally filtered by ID or Filters.
// Real SnapshotFilterName (aws-sdk-go-v2/service/fsx@v1.68.4 types/enums.go)
// has 2 values: file-system-id, volume-id. file-system-id requires resolving
// the snapshot's owning volume -- storedSnapshot only tracks VolumeID
// directly. IncludeShared (real DescribeSnapshotsInput member) is not
// modeled: this backend is single-account/single-tenant, so every snapshot
// is definitionally "owned" regardless of that flag -- there is no honest
// cross-account snapshot to differ on.
func (b *InMemoryBackend) DescribeSnapshots(
	ids []string,
	filters []wireFilter,
	maxResults int32,
	nextToken string,
) ([]*Snapshot, string, error) {
	b.mu.RLock("DescribeSnapshots")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedSnapshot

	if len(ids) > 0 {
		for _, id := range ids {
			s, ok := b.snapshots.Get(id)
			if !ok {
				return nil, "", ErrSnapshotNotFound
			}

			all = append(all, s)
		}
	} else {
		for _, s := range b.snapshots.All() {
			if matchesFilters(filters, func(name string) (string, bool) {
				switch name {
				case "volume-id":
					return s.VolumeID, true
				case filterNameFileSystemID:
					if vol, ok := b.volumes.Get(s.VolumeID); ok {
						return vol.FileSystemID, true
					}

					return "", true
				default:
					return "", false
				}
			}) {
				all = append(all, s)
			}
		}

		sort.Slice(all, func(i, j int) bool { return all[i].SnapshotID < all[j].SnapshotID })
	}

	start, end, next := paginate(len(all), int(maxResults), nextToken, func(i int) string {
		return all[i].SnapshotID
	})

	result := make([]*Snapshot, end-start)
	for i, s := range all[start:end] {
		result[i] = s.toPublic()
	}

	return result, next, nil
}

type updateSnapshotInput struct {
	SnapshotID string `json:"SnapshotId"`
	Name       string `json:"Name,omitempty"`
}

// UpdateSnapshot updates snapshot metadata.
func (b *InMemoryBackend) UpdateSnapshot(input *updateSnapshotInput) (*Snapshot, error) {
	b.mu.Lock("UpdateSnapshot")
	defer b.mu.Unlock()

	s, ok := b.snapshots.Get(input.SnapshotID)
	if !ok {
		return nil, ErrSnapshotNotFound
	}

	if input.Name != "" {
		s.Name = input.Name
	}

	return s.toPublic(), nil
}

type copySnapshotAndUpdateVolumeInput struct {
	VolumeID         string `json:"VolumeId"`
	SourceSnapshotID string `json:"SourceSnapshotARN"`
}

// snapshotIDFromARN extracts the trailing "snapshot/<id>" resource ID from a
// snapshot ARN, matching the format snapshotARN builds.
func snapshotIDFromARN(snapshotARN string) string {
	_, id, found := strings.Cut(snapshotARN, "snapshot/")
	if !found {
		return snapshotARN
	}

	return id
}

// CopySnapshotAndUpdateVolume restores a volume to the state of a snapshot.
// SourceSnapshotARN is a required real CopySnapshotAndUpdateVolumeInput
// member (api_op_CopySnapshotAndUpdateVolume.go) that was previously decoded
// but never read anywhere: any ARN, including one naming a nonexistent
// snapshot, silently succeeded. Now resolved and existence-checked like
// RestoreVolumeFromSnapshot's sibling SnapshotId parameter.
//
// Neither not-found check below uses ErrVolumeNotFound/ErrSnapshotNotFound:
// this op's own switch (fsx@v1.68.4 deserializers.go
// deserializeOpErrorCopySnapshotAndUpdateVolume) is exactly [BadRequest,
// IncompatibleParameterError, InternalServerError, ServiceLimitExceeded] --
// neither NotFound type is declared here, unlike their legitimate declarers
// elsewhere in this service. ErrValidation (BadRequest, "a generic error
// indicating a failure with a client request") is this op's own declared
// generic-client-error type and is the correct substitution, not an
// invented code (gopherstack-6flj/uox6 error-envelope sweep).
func (b *InMemoryBackend) CopySnapshotAndUpdateVolume(input *copySnapshotAndUpdateVolumeInput) (*Volume, error) {
	if input.SourceSnapshotID == "" {
		return nil, fmt.Errorf("%w: SourceSnapshotARN is required", ErrValidation)
	}

	b.mu.Lock("CopySnapshotAndUpdateVolume")
	defer b.mu.Unlock()

	v, ok := b.volumes.Get(input.VolumeID)
	if !ok {
		return nil, fmt.Errorf("%w: volume %q not found", ErrValidation, input.VolumeID)
	}

	if !b.snapshots.Has(snapshotIDFromARN(input.SourceSnapshotID)) {
		return nil, fmt.Errorf("%w: snapshot %q not found", ErrValidation, input.SourceSnapshotID)
	}

	return v.toPublic(), nil
}

func (b *InMemoryBackend) snapshotARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("snapshot/%s", id))
}
