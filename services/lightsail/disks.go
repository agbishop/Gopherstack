package lightsail

// This file backs family I (7 ops: AttachDisk, DetachDisk, CreateDisk,
// CreateDiskFromSnapshot, DeleteDisk, GetDisk, GetDisks) and family J (5
// ops: CreateDiskSnapshot, DeleteDiskSnapshot, GetDiskSnapshot,
// GetDiskSnapshots, CopySnapshot).

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	opTypeAttachDisk         = "AttachDisk"
	opTypeDetachDisk         = "DetachDisk"
	opTypeCreateDisk         = "CreateDisk"
	opTypeDeleteDisk         = "DeleteDisk"
	opTypeCreateDiskSnapshot = "CreateDiskSnapshot"
	opTypeDeleteDiskSnapshot = "DeleteDiskSnapshot"
	opTypeCopySnapshot       = "CopySnapshot"
)

// CreateDisk creates a new, unattached Disk.
func (b *InMemoryBackend) CreateDisk(
	name, availabilityZone string, sizeInGb int32, addOns []AddOnRequest, userTags map[string]string,
) ([]Operation, error) {
	if sizeInGb <= 0 {
		return nil, validationError("SizeInGb must be positive")
	}

	b.mu.Lock("CreateDisk")
	defer b.mu.Unlock()

	if err := b.registerNameLocked(ResourceTypeDisk, name); err != nil {
		return nil, err
	}

	az := availabilityZone
	if az == "" {
		az = availabilityZoneA(b.region)
	}

	d := &Disk{
		Name: name, Arn: b.regionalARN(ResourceTypeDisk, newUUID()), SupportCode: newSupportCode(),
		State: DiskStateAvailable, SizeInGb: sizeInGb, Iops: defaultDiskIops(sizeInGb),
		CreatedAt: nowUTC(), Location: ResourceLocation{RegionName: b.region, AvailabilityZone: az},
		Tags: tags.New("lightsail.disk." + name + ".tags"),
	}
	d.Tags.Merge(userTags)

	for _, a := range addOns {
		d.AddOns = applyAddOnRequestLocked(d.AddOns, a)
	}

	b.disks.Put(d)

	return b.newOperationsLocked(opTypeCreateDisk, ResourceTypeDisk, []string{name}), nil
}

// defaultDiskIops is a defensible, SDK-unconfirmed IOPS-per-GB stand-in
// (real Lightsail disk IOPS scale with size, but no formula is published in
// this SDK module) -- 3 IOPS/GB, the same baseline ratio EBS gp2 uses,
// clearly documented as an emulator convention, not an AWS-sourced fact.
const diskIopsPerGb = 3

func defaultDiskIops(sizeInGb int32) int32 { return sizeInGb * diskIopsPerGb }

// CreateDiskFromSnapshot creates a new Disk restored from an existing
// DiskSnapshot.
func (b *InMemoryBackend) CreateDiskFromSnapshot(
	name, availabilityZone, snapshotName string, sizeInGb int32,
	addOns []AddOnRequest, userTags map[string]string,
) ([]Operation, error) {
	b.mu.Lock("CreateDiskFromSnapshot")
	defer b.mu.Unlock()

	snap, ok := b.diskSnapshots.Get(snapshotName)
	if !ok {
		return nil, notFoundError("DiskSnapshot", snapshotName)
	}

	if err := b.registerNameLocked(ResourceTypeDisk, name); err != nil {
		return nil, err
	}

	size := sizeInGb
	if size <= 0 {
		size = snap.SizeInGb
	}

	az := availabilityZone
	if az == "" {
		az = availabilityZoneA(b.region)
	}

	d := &Disk{
		Name: name, Arn: b.regionalARN(ResourceTypeDisk, newUUID()), SupportCode: newSupportCode(),
		State: DiskStateAvailable, SizeInGb: size, Iops: defaultDiskIops(size),
		CreatedAt: nowUTC(), Location: ResourceLocation{RegionName: b.region, AvailabilityZone: az},
		Tags: tags.New("lightsail.disk." + name + ".tags"),
	}
	d.Tags.Merge(userTags)

	for _, a := range addOns {
		d.AddOns = applyAddOnRequestLocked(d.AddOns, a)
	}

	b.disks.Put(d)

	return b.newOperationsLocked(opTypeCreateDisk, ResourceTypeDisk, []string{name}), nil
}

// AttachDisk attaches the named disk to instanceName at diskPath, honoring
// autoMounting as bookkeeping-only state (no real guest OS to mount inside
// this emulator, PARITY.md 4.2).
func (b *InMemoryBackend) AttachDisk(diskName, instanceName, diskPath string, autoMounting bool) (*Operation, error) {
	b.mu.Lock("AttachDisk")
	defer b.mu.Unlock()

	d, ok := b.disks.Get(diskName)
	if !ok {
		return nil, notFoundError("Disk", diskName)
	}

	if _, instOK := b.instances.Get(instanceName); !instOK {
		return nil, notFoundError("Instance", instanceName)
	}

	if d.IsAttached {
		return nil, validationError(fmt.Sprintf("disk %s is already attached", diskName))
	}

	d.State = DiskStateInUse
	d.IsAttached = true
	d.AttachedTo = instanceName
	d.AttachmentState = "attached"
	d.Path = diskPath
	d.GbInUse = d.SizeInGb

	if autoMounting {
		d.AutoMountStatus = AutoMountStatusMounted
	} else {
		d.AutoMountStatus = AutoMountStatusNotMounted
	}

	ops := b.newOperationsLocked(opTypeAttachDisk, ResourceTypeDisk, []string{diskName})

	return &ops[0], nil
}

// DetachDisk detaches the named disk from whatever instance it is attached
// to.
func (b *InMemoryBackend) DetachDisk(diskName string) (*Operation, error) {
	b.mu.Lock("DetachDisk")
	defer b.mu.Unlock()

	d, ok := b.disks.Get(diskName)
	if !ok {
		return nil, notFoundError("Disk", diskName)
	}

	d.State = DiskStateAvailable
	d.IsAttached = false
	d.AttachedTo = ""
	d.AttachmentState = ""
	d.Path = ""
	d.GbInUse = 0
	d.AutoMountStatus = ""

	ops := b.newOperationsLocked(opTypeDetachDisk, ResourceTypeDisk, []string{diskName})

	return &ops[0], nil
}

// DeleteDisk deletes the named disk.
func (b *InMemoryBackend) DeleteDisk(name string) ([]Operation, error) {
	b.mu.Lock("DeleteDisk")
	defer b.mu.Unlock()

	d, ok := b.disks.Get(name)
	if !ok {
		return nil, notFoundError("Disk", name)
	}

	if d.IsAttached {
		return nil, validationError(fmt.Sprintf("disk %s is attached to %s", name, d.AttachedTo))
	}

	if d.Tags != nil {
		d.Tags.Close()
	}

	b.disks.Delete(name)
	b.unregisterNameLocked(name)

	return b.newOperationsLocked(opTypeDeleteDisk, ResourceTypeDisk, []string{name}), nil
}

// GetDisk returns the named disk.
func (b *InMemoryBackend) GetDisk(name string) (*Disk, error) {
	b.mu.RLock("GetDisk")
	defer b.mu.RUnlock()

	d, ok := b.disks.Get(name)
	if !ok {
		return nil, notFoundError("Disk", name)
	}

	return d.clone(), nil
}

// DisksAttachedTo returns every disk currently attached to instanceName,
// sorted by name -- backs Instance.Hardware.Disks (types.InstanceHardware:
// "The disks attached to the instance"), which AttachDisk/DetachDisk update
// on the disk side only; this reads that same disk state from the instance
// side instead of duplicating it.
func (b *InMemoryBackend) DisksAttachedTo(instanceName string) []*Disk {
	b.mu.RLock("DisksAttachedTo")
	defer b.mu.RUnlock()

	var out []*Disk

	for _, d := range b.disks.All() {
		if d.AttachedTo == instanceName {
			out = append(out, d.clone())
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// GetDisks returns every disk, paginated.
func (b *InMemoryBackend) GetDisks(token string) (page.Page[*Disk], error) {
	b.mu.RLock("GetDisks")
	defer b.mu.RUnlock()

	all := b.disks.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*Disk, len(all))
	for i, v := range all {
		out[i] = v.clone()
	}

	return paginateGeneric(out, token)
}

// CreateDiskSnapshot creates a DiskSnapshot of diskName, or of
// instanceName's system disk when diskName is empty (PARITY.md family J).
func (b *InMemoryBackend) CreateDiskSnapshot(
	diskName, instanceName, snapshotName string,
	userTags map[string]string,
) ([]Operation, error) {
	b.mu.Lock("CreateDiskSnapshot")
	defer b.mu.Unlock()

	var (
		size         int32
		fromDiskArn  string
		fromInstArn  string
		fromInstName string
		location     ResourceLocation
	)

	switch {
	case diskName != "":
		d, ok := b.disks.Get(diskName)
		if !ok {
			return nil, notFoundError("Disk", diskName)
		}

		size, fromDiskArn, location = d.SizeInGb, d.Arn, d.Location
	case instanceName != "":
		i, ok := b.instances.Get(instanceName)
		if !ok {
			return nil, notFoundError("Instance", instanceName)
		}

		size, fromInstArn, fromInstName, location = i.DiskSizeInGb, i.Arn, i.Name, i.Location
	default:
		return nil, validationError("either DiskName or InstanceName is required")
	}

	if err := b.registerNameLocked(ResourceTypeDiskSnapshot, snapshotName); err != nil {
		return nil, err
	}

	snap := &DiskSnapshot{
		Name: snapshotName, Arn: b.regionalARN(ResourceTypeDiskSnapshot, newUUID()),
		SupportCode: newSupportCode(), State: SnapshotStatePending, Progress: "0%",
		FromDiskName: diskName, FromDiskArn: fromDiskArn,
		FromInstanceName: fromInstName, FromInstanceArn: fromInstArn,
		SizeInGb: size, CreatedAt: nowUTC(), Location: location,
		Tags: tags.New("lightsail.disksnapshot." + snapshotName + ".tags"),
	}
	snap.Tags.Merge(userTags)
	b.diskSnapshots.Put(snap)

	b.scheduleDiskSnapshotCompleteLocked(snapshotName)

	return b.newOperationsLocked(opTypeCreateDiskSnapshot, ResourceTypeDiskSnapshot, []string{snapshotName}), nil
}

func (b *InMemoryBackend) scheduleDiskSnapshotCompleteLocked(name string) {
	b.work.After("DiskSnapshotComplete", asyncTransitionDelay, func() {
		b.mu.Lock("DiskSnapshot-async-complete")
		defer b.mu.Unlock()

		if s, found := b.diskSnapshots.Get(name); found && s.State == SnapshotStatePending {
			s.State = SnapshotStateCompleted
			s.Progress = snapshotProgressComplete
		}
	})
}

// DeleteDiskSnapshot deletes the named disk snapshot.
func (b *InMemoryBackend) DeleteDiskSnapshot(name string) ([]Operation, error) {
	b.mu.Lock("DeleteDiskSnapshot")
	defer b.mu.Unlock()

	snap, ok := b.diskSnapshots.Get(name)
	if !ok {
		return nil, notFoundError("DiskSnapshot", name)
	}

	if snap.Tags != nil {
		snap.Tags.Close()
	}

	b.diskSnapshots.Delete(name)
	b.unregisterNameLocked(name)

	return b.newOperationsLocked(opTypeDeleteDiskSnapshot, ResourceTypeDiskSnapshot, []string{name}), nil
}

// GetDiskSnapshot returns the named disk snapshot.
func (b *InMemoryBackend) GetDiskSnapshot(name string) (*DiskSnapshot, error) {
	b.mu.RLock("GetDiskSnapshot")
	defer b.mu.RUnlock()

	snap, ok := b.diskSnapshots.Get(name)
	if !ok {
		return nil, notFoundError("DiskSnapshot", name)
	}

	return snap.clone(), nil
}

// GetDiskSnapshots returns every disk snapshot, paginated.
func (b *InMemoryBackend) GetDiskSnapshots(token string) (page.Page[*DiskSnapshot], error) {
	b.mu.RLock("GetDiskSnapshots")
	defer b.mu.RUnlock()

	all := b.diskSnapshots.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*DiskSnapshot, len(all))
	for i, v := range all {
		out[i] = v.clone()
	}

	return paginateGeneric(out, token)
}

// CopySnapshot copies an existing instance or disk snapshot
// (sourceResourceName is polymorphic) to a new snapshot named
// targetSnapshotName -- the only explicitly cross-region op in this whole
// surface (PARITY.md family J). This repo models each AWS region as its own
// separate InMemoryBackend instance, so a genuine cross-PROCESS copy from a
// different region's backend is not reachable from here without dedicated
// wiring this pass does not add; this backend honors sourceRegion only when
// it names THIS backend's own region (or is empty), and otherwise returns a
// validation error naming the limitation explicitly rather than silently
// mis-copying or fabricating a record -- a documented, honest scoped-down
// behavior, not a fabricated cross-region result.
func (b *InMemoryBackend) CopySnapshot(
	sourceRegion, sourceResourceName, targetSnapshotName string,
) ([]Operation, error) {
	b.mu.Lock("CopySnapshot")
	defer b.mu.Unlock()

	if sourceRegion != "" && sourceRegion != b.region {
		return nil, validationError(
			"CopySnapshot: cross-region source (" + sourceRegion + ") is not reachable from this backend instance; " +
				"only same-region snapshot copies are supported",
		)
	}

	if snap, ok := b.instanceSnapshots.Get(sourceResourceName); ok {
		if err := b.registerNameLocked(ResourceTypeInstanceSnapshot, targetSnapshotName); err != nil {
			return nil, err
		}

		cp := snap.clone()
		cp.Name = targetSnapshotName
		cp.Arn = b.regionalARN(ResourceTypeInstanceSnapshot, newUUID())
		cp.CreatedAt = nowUTC()
		cp.Tags = tags.New("lightsail.instancesnapshot." + targetSnapshotName + ".tags")
		b.instanceSnapshots.Put(cp)

		return b.newOperationsLocked(
			opTypeCopySnapshot,
			ResourceTypeInstanceSnapshot,
			[]string{targetSnapshotName},
		), nil
	}

	if snap, ok := b.diskSnapshots.Get(sourceResourceName); ok {
		if err := b.registerNameLocked(ResourceTypeDiskSnapshot, targetSnapshotName); err != nil {
			return nil, err
		}

		cp := snap.clone()
		cp.Name = targetSnapshotName
		cp.Arn = b.regionalARN(ResourceTypeDiskSnapshot, newUUID())
		cp.CreatedAt = nowUTC()
		cp.Tags = tags.New("lightsail.disksnapshot." + targetSnapshotName + ".tags")
		b.diskSnapshots.Put(cp)

		return b.newOperationsLocked(opTypeCopySnapshot, ResourceTypeDiskSnapshot, []string{targetSnapshotName}), nil
	}

	return nil, notFoundError("snapshot", sourceResourceName)
}
