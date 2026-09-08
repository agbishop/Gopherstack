package ec2

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CopySnapshot creates a new snapshot as a copy of an existing one.
func (b *InMemoryBackend) CopySnapshot(
	sourceSnapshotID, description string, encryptOverride bool, kmsKeyID string,
) (*Snapshot, error) {
	if sourceSnapshotID == "" {
		return nil, fmt.Errorf("%w: SourceSnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CopySnapshot")
	defer b.mu.Unlock()

	src, ok := b.snapshots.Get(sourceSnapshotID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotNotFound, sourceSnapshotID)
	}

	desc := description
	if desc == "" {
		desc = "Copy of " + sourceSnapshotID
	}

	// Encrypted/KmsKeyId default to the source's own state when the caller
	// says nothing (ec2@v1.319.1 api_op_CopySnapshot.go's Encrypted doc: "You
	// can encrypt a copy of an unencrypted snapshot, but you cannot create an
	// unencrypted copy of an encrypted snapshot" -- a contingent default, not
	// a fixed one). An explicit Encrypted=true request can additionally
	// encrypt a copy of an unencrypted source.
	encrypted := src.Encrypted
	copyKmsKeyID := src.KmsKeyID

	if encryptOverride {
		encrypted = true
		copyKmsKeyID = kmsKeyID

		if copyKmsKeyID == "" {
			copyKmsKeyID = defaultEBSKmsKeyAlias
		}
	}

	snap := &Snapshot{
		SnapshotID:  newSnapshotID(),
		VolumeID:    src.VolumeID,
		Description: desc,
		State:       stateCompleted,
		Progress:    snapshotProgress100,
		StartTime:   time.Now().UTC(),
		VolumeSize:  src.VolumeSize,
		Encrypted:   encrypted,
		KmsKeyID:    copyKmsKeyID,
		OwnerID:     b.AccountID,
	}
	b.snapshots.Put(snap)

	return snap, nil
}

// ---- CreateSnapshots ----

// SnapshotEntry is used by CreateSnapshots to specify a volume.
type SnapshotEntry struct {
	VolumeID    string `json:"volumeID,omitempty"`
	SnapshotID  string `json:"snapshotID,omitempty"`
	Description string `json:"description,omitempty"`
	State       string `json:"state,omitempty"`
}

// CreateSnapshots creates one crash-consistent snapshot per volume attached
// to instanceID, honouring excludeBootVolume and excludeDataVolumeIDs
// (types.InstanceSpecification, api_op_CreateSnapshots.go). The root/boot
// volume is the one attached at the device matching the instance's AMI
// RootDeviceName; when the AMI can't be resolved, no volume is treated as
// boot (ExcludeBootVolume then excludes nothing, matching "unknown" rather
// than fabricating a root).
func (b *InMemoryBackend) CreateSnapshots(
	instanceID string,
	excludeBootVolume bool,
	excludeDataVolumeIDs []string,
	description string,
) ([]*Snapshot, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("%w: InstanceSpecification.InstanceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateSnapshots")
	defer b.mu.Unlock()

	inst, ok := b.instances.Get(instanceID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	rootDevice := ""
	if img := b.lookupImageLocked(inst.ImageID); img != nil {
		rootDevice = img.RootDeviceName
	}

	targets, err := selectSnapshotVolumes(
		b.attachedVolumesLocked(instanceID), rootDevice, excludeBootVolume, excludeDataVolumeIDs,
	)
	if err != nil {
		return nil, err
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: instance %s has no volumes to snapshot", ErrInvalidParameter, instanceID)
	}

	snaps := make([]*Snapshot, 0, len(targets))
	for _, vol := range targets {
		snap := &Snapshot{
			SnapshotID:  newSnapshotID(),
			VolumeID:    vol.ID,
			Description: description,
			State:       stateCompleted,
			Progress:    snapshotProgress100,
			StartTime:   time.Now().UTC(),
			VolumeSize:  vol.Size,
			Encrypted:   vol.Encrypted,
			KmsKeyID:    vol.KmsKeyID,
			OwnerID:     b.AccountID,
		}
		b.snapshots.Put(snap)
		snaps = append(snaps, snap)
	}

	return snaps, nil
}

// attachedVolumesLocked returns the volumes attached to instanceID, sorted by
// ID for deterministic snapshot ordering. Must be called with b.mu held.
func (b *InMemoryBackend) attachedVolumesLocked(instanceID string) []*Volume {
	var attached []*Volume
	for _, vol := range b.volumes.All() {
		if vol.Attachment != nil && vol.Attachment.InstanceID == instanceID {
			attached = append(attached, vol)
		}
	}
	sort.Slice(attached, func(i, j int) bool { return attached[i].ID < attached[j].ID })

	return attached
}

// selectSnapshotVolumes applies ExcludeBootVolume/ExcludeDataVolumeIds to
// attached. The boot volume is whichever attached volume's Device matches
// rootDevice; an empty rootDevice (AMI unresolved) means no volume is ever
// treated as boot. Naming the root volume in excludeDataVolumeIDs is
// rejected, matching real AWS ("If you specify the ID of the root volume,
// the request fails" -- InstanceSpecification.ExcludeDataVolumeIds doc).
func selectSnapshotVolumes(
	attached []*Volume,
	rootDevice string,
	excludeBootVolume bool,
	excludeDataVolumeIDs []string,
) ([]*Volume, error) {
	exclude := make(map[string]bool, len(excludeDataVolumeIDs))
	for _, id := range excludeDataVolumeIDs {
		exclude[id] = true
	}

	var targets []*Volume
	for _, vol := range attached {
		isBoot := rootDevice != "" && strings.EqualFold(vol.Attachment.Device, rootDevice)

		switch {
		case isBoot && exclude[vol.ID]:
			return nil, fmt.Errorf(
				"%w: %s is the root volume; exclude it with ExcludeBootVolume, not ExcludeDataVolumeIds",
				ErrInvalidParameter, vol.ID,
			)
		case isBoot && excludeBootVolume:
			continue
		case !isBoot && exclude[vol.ID]:
			continue
		}

		targets = append(targets, vol)
	}

	return targets, nil
}

// ---- Snapshot block public access ----

// GetSnapshotBlockPublicAccessState returns the account-level block state.
// Returns "block-all-sharing" (default) if never explicitly set.
func (b *InMemoryBackend) GetSnapshotBlockPublicAccessState() string {
	b.mu.RLock("GetSnapshotBlockPublicAccessState")
	defer b.mu.RUnlock()

	if b.snapshotBlockPublicAccess == "" {
		return stateBlockAllSharing
	}

	return b.snapshotBlockPublicAccess
}

// EnableSnapshotBlockPublicAccess sets the block state to block-all-sharing or block-new-sharing.
func (b *InMemoryBackend) EnableSnapshotBlockPublicAccess(state string) error {
	if state != stateBlockAllSharing && state != "block-new-sharing" {
		return fmt.Errorf(
			"%w: State must be "+stateBlockAllSharing+" or block-new-sharing",
			ErrInvalidParameter,
		)
	}

	b.mu.Lock("EnableSnapshotBlockPublicAccess")
	defer b.mu.Unlock()

	b.snapshotBlockPublicAccess = state

	return nil
}

// DisableSnapshotBlockPublicAccess sets the block state to unblocked.
func (b *InMemoryBackend) DisableSnapshotBlockPublicAccess() {
	b.mu.Lock("DisableSnapshotBlockPublicAccess")
	defer b.mu.Unlock()

	b.snapshotBlockPublicAccess = "unblocked"
}

// ---- Snapshot tier ----

// SnapshotTierItem contains tier info for a single snapshot.
type SnapshotTierItem struct {
	SnapshotID  string `json:"snapshotID,omitempty"`
	VolumeID    string `json:"volumeID,omitempty"`
	StorageTier string `json:"storageTier,omitempty"`
}

// DescribeSnapshotTierStatus returns tier info for given snapshot IDs (or all).
func (b *InMemoryBackend) DescribeSnapshotTierStatus(ids []string) []SnapshotTierItem {
	b.mu.RLock("DescribeSnapshotTierStatus")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	var out []SnapshotTierItem
	for _, snap := range b.snapshots.All() {
		if len(filter) > 0 && !filter[snap.SnapshotID] {
			continue
		}
		tier := b.snapshotTiers[snap.SnapshotID]
		if tier == "" {
			tier = "standard"
		}
		out = append(out, SnapshotTierItem{
			SnapshotID:  snap.SnapshotID,
			VolumeID:    snap.VolumeID,
			StorageTier: tier,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SnapshotID < out[j].SnapshotID })

	return out
}

// ModifySnapshotTier updates the storage tier for a snapshot ("archive" or "standard").
func (b *InMemoryBackend) ModifySnapshotTier(snapshotID, storageTier string) error {
	if snapshotID == "" {
		return fmt.Errorf("%w: SnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifySnapshotTier")
	defer b.mu.Unlock()

	if _, ok := b.snapshots.Get(snapshotID); !ok {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, snapshotID)
	}
	b.snapshotTiers[snapshotID] = storageTier

	return nil
}

// ResetSnapshotAttribute resets the createVolumePermission attribute of a snapshot.
func (b *InMemoryBackend) ResetSnapshotAttribute(snapshotID string) error {
	if snapshotID == "" {
		return fmt.Errorf("%w: SnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ResetSnapshotAttribute")
	defer b.mu.Unlock()

	if _, ok := b.snapshots.Get(snapshotID); !ok {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, snapshotID)
	}
	delete(b.snapshotAttributes, snapshotID)

	return nil
}

// ---- CreateDefaultVpc ----

// LockSnapshot locks a snapshot to prevent deletion.
func (b *InMemoryBackend) LockSnapshot(
	snapshotID, lockMode string,
	durationDays int,
) (*SnapshotLock, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: SnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("LockSnapshot")
	defer b.mu.Unlock()

	if _, ok := b.snapshots.Get(snapshotID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotNotFound, snapshotID)
	}
	if _, locked := b.snapshotLocks.Get(snapshotID); locked {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotAlreadyLocked, snapshotID)
	}

	now := time.Now().UTC()
	lock := &SnapshotLock{
		SnapshotID:       snapshotID,
		LockState:        lockMode,
		LockCreatedOn:    now,
		LockDurationDays: durationDays,
	}
	if durationDays > 0 {
		lock.LockExpiresOn = now.AddDate(0, 0, durationDays)
	}
	b.snapshotLocks.Put(lock)

	return lock, nil
}

// UnlockSnapshot removes the lock from a snapshot.
func (b *InMemoryBackend) UnlockSnapshot(snapshotID string) error {
	if snapshotID == "" {
		return fmt.Errorf("%w: SnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("UnlockSnapshot")
	defer b.mu.Unlock()

	if _, ok := b.snapshots.Get(snapshotID); !ok {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, snapshotID)
	}
	if _, locked := b.snapshotLocks.Get(snapshotID); !locked {
		return fmt.Errorf("%w: %s", ErrSnapshotNotLocked, snapshotID)
	}
	b.snapshotLocks.Delete(snapshotID)

	return nil
}

// DescribeLockedSnapshots returns locked snapshots, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeLockedSnapshots(ids []string) []*SnapshotLock {
	b.mu.RLock("DescribeLockedSnapshots")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	var out []*SnapshotLock
	for _, lock := range b.snapshotLocks.All() {
		if len(filter) > 0 && !filter[lock.SnapshotID] {
			continue
		}
		cp := *lock
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SnapshotID < out[j].SnapshotID })

	return out
}

// ListSnapshotsInRecycleBin returns soft-deleted snapshots.
func (b *InMemoryBackend) ListSnapshotsInRecycleBin(snapshotIDs []string) []*Snapshot {
	b.mu.RLock("ListSnapshotsInRecycleBin")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(snapshotIDs))
	for _, id := range snapshotIDs {
		filter[id] = true
	}

	var out []*Snapshot
	for _, snap := range b.recycleBinSnapshots.All() {
		if len(filter) > 0 && !filter[snap.SnapshotID] {
			continue
		}
		cp := *snap
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SnapshotID < out[j].SnapshotID })

	return out
}

// RestoreSnapshotFromRecycleBin restores a snapshot from recycle bin.
func (b *InMemoryBackend) RestoreSnapshotFromRecycleBin(snapshotID string) error {
	if snapshotID == "" {
		return fmt.Errorf("%w: SnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("RestoreSnapshotFromRecycleBin")
	defer b.mu.Unlock()

	snap, ok := b.recycleBinSnapshots.Get(snapshotID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, snapshotID)
	}
	b.snapshots.Put(snap)
	b.recycleBinSnapshots.Delete(snapshotID)

	return nil
}

// RestoreSnapshotTier restores a snapshot from archive tier to standard.
func (b *InMemoryBackend) RestoreSnapshotTier(snapshotID string) error {
	if snapshotID == "" {
		return fmt.Errorf("%w: SnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("RestoreSnapshotTier")
	defer b.mu.Unlock()

	if _, ok := b.snapshots.Get(snapshotID); !ok {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, snapshotID)
	}
	b.snapshotTiers[snapshotID] = stateDefaultCredit

	return nil
}

// ---- Import snapshot ----

// ImportSnapshot creates an import task for a snapshot.
func (b *InMemoryBackend) ImportSnapshot(
	description string, encrypted bool, kmsKeyID string,
) (*SnapshotImportTask, error) {
	b.mu.Lock("ImportSnapshot")
	defer b.mu.Unlock()

	if encrypted && kmsKeyID == "" {
		kmsKeyID = defaultEBSKmsKeyAlias
	}

	task := &SnapshotImportTask{
		ImportTaskID: "import-snap-" + uuid.New().String()[:8],
		Description:  description,
		Status:       stateTaskCompleted,
		Encrypted:    encrypted,
		KmsKeyID:     kmsKeyID,
	}
	b.snapshotImportTasks.Put(task)

	return task, nil
}

// DescribeImportSnapshotTasks returns import snapshot tasks.
func (b *InMemoryBackend) DescribeImportSnapshotTasks(taskIDs []string) []*SnapshotImportTask {
	b.mu.RLock("DescribeImportSnapshotTasks")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(taskIDs))
	for _, id := range taskIDs {
		filter[id] = true
	}

	var out []*SnapshotImportTask
	for _, t := range b.snapshotImportTasks.All() {
		if len(filter) > 0 && !filter[t.ImportTaskID] {
			continue
		}
		cp := *t
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ImportTaskID < out[j].ImportTaskID })

	return out
}

// ---- Fast launch / fast snapshot restores ----

// EnableFastSnapshotRestores enables fast snapshot restores for given AZ/snapshot combos.
func (b *InMemoryBackend) EnableFastSnapshotRestores(
	snapshotIDs, availabilityZones []string,
) error {
	b.mu.Lock("EnableFastSnapshotRestores")
	defer b.mu.Unlock()

	for _, snap := range snapshotIDs {
		for _, az := range availabilityZones {
			b.fastSnapshotRestores[snap+":"+az] = true
		}
	}

	return nil
}

// DisableFastSnapshotRestores disables fast snapshot restores.
func (b *InMemoryBackend) DisableFastSnapshotRestores(
	snapshotIDs, availabilityZones []string,
) error {
	b.mu.Lock("DisableFastSnapshotRestores")
	defer b.mu.Unlock()

	for _, snap := range snapshotIDs {
		for _, az := range availabilityZones {
			delete(b.fastSnapshotRestores, snap+":"+az)
		}
	}

	return nil
}

// FastSnapshotRestoreItem holds fast snapshot restore config for a snapshot/AZ combo.
type FastSnapshotRestoreItem struct {
	SnapshotID       string `json:"snapshotID,omitempty"`
	AvailabilityZone string `json:"availabilityZone,omitempty"`
	State            string `json:"state,omitempty"`
}

// DescribeFastSnapshotRestores returns fast snapshot restore enabled items.
func (b *InMemoryBackend) DescribeFastSnapshotRestores() []FastSnapshotRestoreItem {
	b.mu.RLock("DescribeFastSnapshotRestores")
	defer b.mu.RUnlock()

	var out []FastSnapshotRestoreItem
	for key := range b.fastSnapshotRestores {
		// Key is "snapshotID:az"
		parts := splitKey(key)
		if len(parts) == magicSplitLen {
			out = append(out, FastSnapshotRestoreItem{
				SnapshotID:       parts[0],
				AvailabilityZone: parts[1],
				State:            stateEnabledFastLaunch,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SnapshotID != out[j].SnapshotID {
			return out[i].SnapshotID < out[j].SnapshotID
		}

		return out[i].AvailabilityZone < out[j].AvailabilityZone
	})

	return out
}

// splitKey splits a colon-separated key string into two parts.
func splitKey(key string) []string {
	for i, c := range key {
		if c == ':' {
			return []string{key[:i], key[i+1:]}
		}
	}

	return []string{key}
}

// ---- GetPasswordData ----

// CreateSnapshot creates an EBS snapshot from a volume.
func (b *InMemoryBackend) CreateSnapshot(volumeID, description string) (*Snapshot, error) {
	if volumeID == "" {
		return nil, fmt.Errorf("%w: VolumeId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateSnapshot")
	defer b.mu.Unlock()

	vol, ok := b.volumes.Get(volumeID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrVolumeNotFound, volumeID)
	}

	snap := &Snapshot{
		SnapshotID:  newSnapshotID(),
		VolumeID:    volumeID,
		Description: description,
		State:       "completed",
		Progress:    "100%",
		StartTime:   time.Now().UTC(),
		VolumeSize:  vol.Size,
		Encrypted:   vol.Encrypted,
		KmsKeyID:    vol.KmsKeyID,
		OwnerID:     b.AccountID,
	}
	b.snapshots.Put(snap)

	cp := *snap

	return &cp, nil
}

// DescribeSnapshots returns snapshots, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeSnapshots(ids []string) []*Snapshot {
	b.mu.RLock("DescribeSnapshots")
	defer b.mu.RUnlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	out := make([]*Snapshot, 0, b.snapshots.Len())
	for _, snap := range b.snapshots.All() {
		if len(idSet) > 0 && !idSet[snap.SnapshotID] {
			continue
		}

		cp := *snap
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].SnapshotID < out[j].SnapshotID
	})

	return out
}

// DeleteSnapshot removes a snapshot.
func (b *InMemoryBackend) DeleteSnapshot(id string) error {
	if id == "" {
		return fmt.Errorf("%w: SnapshotId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteSnapshot")
	defer b.mu.Unlock()

	if _, ok := b.snapshots.Get(id); !ok {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, id)
	}
	b.snapshots.Delete(id)
	delete(b.tags, id)
	delete(b.snapshotAttributes, id)
	delete(b.snapshotTiers, id)

	prefix := id + ":"
	for key := range b.fastSnapshotRestores {
		if strings.HasPrefix(key, prefix) {
			delete(b.fastSnapshotRestores, key)
		}
	}

	return nil
}

// ---- AMI lifecycle ----

// DescribeSnapshotsSorted returns snapshots sorted by snapshot ID.
func (b *InMemoryBackend) DescribeSnapshotsSorted(ids []string) []*Snapshot {
	return b.DescribeSnapshots(ids)
}
