package efs

import (
	"context"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateAccessPoint creates an access point for a file system.
// Supports ClientToken idempotency.
func (b *InMemoryBackend) CreateAccessPoint(
	ctx context.Context,
	req CreateAccessPointRequest,
) (*AccessPoint, error) {
	if err := validateTags(req.Tags); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateAccessPoint")
	defer b.mu.Unlock()

	// ClientToken idempotency.
	if req.ClientToken != "" {
		if existing, ok := b.accessPointsByClientToken.Get(regionKey(region, req.ClientToken)); ok {
			cp := copyAccessPoint(existing)

			return cp, nil
		}
	}

	fs, ok := b.fileSystems.Get(regionKey(region, req.FileSystemID))
	if !ok {
		return nil, fmt.Errorf("%w: file system %s not found", ErrNotFound, req.FileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return nil, err
	}

	// Validate RootDirectory: require CreationInfo when path != "/".
	if req.RootDirectory != nil && req.RootDirectory.Path != "" && req.RootDirectory.Path != "/" {
		if req.RootDirectory.CreationInfo == nil {
			return nil, fmt.Errorf(
				"%w: CreationInfo is required when RootDirectory.Path is not /",
				ErrBadRequest,
			)
		}
	}

	id := "fsap-" + uuid.NewString()[:8]
	apARN := arn.Build("elasticfilesystem", region, b.accountID, "access-point/"+id)
	t := tags.New("efs.accesspoint." + id + ".tags")

	tagCopy := make(map[string]string, len(req.Tags))
	maps.Copy(tagCopy, req.Tags)

	if len(tagCopy) > 0 {
		t.Merge(tagCopy)
	}
	name := req.Tags["Name"]

	ap := &AccessPoint{
		region:         region,
		AccessPointID:  id,
		AccessPointArn: apARN,
		FileSystemID:   req.FileSystemID,
		ClientToken:    req.ClientToken,
		Name:           name,
		LifeCycleState: statusAvailable,
		Tags:           t,
		PosixUser:      req.PosixUser,
		RootDirectory:  req.RootDirectory,
		OwnerID:        b.accountID,
	}
	b.accessPoints.Put(ap)
	b.accessPointsByARN.Put(ap)
	if req.ClientToken != "" {
		b.accessPointsByClientToken.Put(ap)
	}
	b.apFSStore(region, req.FileSystemID)[id] = struct{}{}
	cp := copyAccessPoint(ap)

	return cp, nil
}

func copyAccessPoint(ap *AccessPoint) *AccessPoint {
	cp := *ap

	if ap.PosixUser != nil {
		pu := *ap.PosixUser
		if len(ap.PosixUser.SecondaryGids) > 0 {
			pu.SecondaryGids = make([]int64, len(ap.PosixUser.SecondaryGids))
			copy(pu.SecondaryGids, ap.PosixUser.SecondaryGids)
		}
		cp.PosixUser = &pu
	}

	if ap.RootDirectory != nil {
		rd := *ap.RootDirectory
		if ap.RootDirectory.CreationInfo != nil {
			ci := *ap.RootDirectory.CreationInfo
			rd.CreationInfo = &ci
		}
		cp.RootDirectory = &rd
	}

	return &cp
}

// DescribeAccessPoints returns access points, optionally filtered by file system ID or access point ID.
func (b *InMemoryBackend) DescribeAccessPoints(
	ctx context.Context,
	fileSystemID, accessPointID, marker string, maxItems int,
) ([]*AccessPoint, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeAccessPoints")
	defer b.mu.RUnlock()

	return describeByIDOrFilter(
		func(id string) (*AccessPoint, bool) { return b.accessPoints.Get(regionKey(region, id)) },
		b.accessPointsByRegion.Get(region),
		accessPointID, ErrAccessPointNotFound,
		fileSystemID,
		func(fsID string) error { return b.requireFileSystem(region, fsID) },
		func(ap *AccessPoint) string { return ap.FileSystemID },
		copyAccessPoint,
		func(ap *AccessPoint) string { return ap.AccessPointID },
		marker, maxItems,
	)
}

// DeleteAccessPoint deletes an access point by ID.
func (b *InMemoryBackend) DeleteAccessPoint(ctx context.Context, accessPointID string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteAccessPoint")
	defer b.mu.Unlock()

	ap, ok := b.accessPoints.Get(regionKey(region, accessPointID))
	if !ok {
		return fmt.Errorf("%w: access point %s not found", ErrAccessPointNotFound, accessPointID)
	}
	b.accessPointsByARN.Delete(regionKey(region, ap.AccessPointArn))
	if ap.ClientToken != "" {
		b.accessPointsByClientToken.Delete(regionKey(region, ap.ClientToken))
	}
	// Clean up apByFS index.
	if b.apByFS[region] != nil && b.apByFS[region][ap.FileSystemID] != nil {
		delete(b.apByFS[region][ap.FileSystemID], accessPointID)
	}

	ap.Tags.Close()
	b.accessPoints.Delete(regionKey(region, accessPointID))

	return nil
}

// AddAccessPointInternal inserts a pre-built AccessPoint directly into the backend (test seed helper).
func (b *InMemoryBackend) AddAccessPointInternal(ap *AccessPoint) {
	b.mu.Lock("AddAccessPointInternal")
	defer b.mu.Unlock()

	ap.region = regionFromARN(ap.AccessPointArn, b.region)
	b.accessPoints.Put(ap)
	b.accessPointsByARN.Put(ap)
}
