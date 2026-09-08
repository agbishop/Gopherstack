package directoryservice

import (
	"context"
	"sort"
	"time"
)

// CreateSnapshot creates a manual snapshot for a directory.
func (b *InMemoryBackend) CreateSnapshot(
	ctx context.Context,
	directoryID, name string,
) (*Snapshot, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateSnapshot")
	defer b.mu.Unlock()

	d, ok := b.directoryGet(region, directoryID)
	if !ok {
		return nil, ErrDirectoryNotFound
	}
	if DirectoryType(d.DirType) == DirectoryTypeADConnector {
		return nil, ErrSnapshotUnsupportedForADConnector
	}

	var count int32
	for _, s := range b.snapshotsInRegion(region) {
		if s.DirectoryID == directoryID && s.SnapType == string(SnapshotTypeManual) {
			count++
		}
	}
	if count >= defaultSnapshotLimit {
		return nil, ErrSnapshotLimitExceeded
	}

	id := b.newSnapshotID()
	now := time.Now().UTC()

	s := &storedSnapshot{
		region:      region,
		StartTime:   now,
		SnapshotID:  id,
		DirectoryID: directoryID,
		Name:        name,
		Status:      string(SnapshotStatusCompleted),
		SnapType:    string(SnapshotTypeManual),
	}
	b.snapshotPut(s)

	cp := s.toSnapshot()

	return &cp, nil
}

// newAutoSnapshot stores an Auto-type snapshot for directoryID, AWS's own type for a
// snapshot taken automatically ahead of another operation (e.g. StartSchemaExtension's
// createSnapshotBeforeSchemaExtension) rather than requested directly via CreateSnapshot.
// Callers must already hold b.mu and have confirmed directoryID exists.
func (b *InMemoryBackend) newAutoSnapshot(region, directoryID, name string) {
	b.snapshotPut(&storedSnapshot{
		region:      region,
		StartTime:   time.Now().UTC(),
		SnapshotID:  b.newSnapshotID(),
		DirectoryID: directoryID,
		Name:        name,
		Status:      string(SnapshotStatusCompleted),
		SnapType:    string(SnapshotTypeAuto),
	})
}

// DeleteSnapshot deletes a snapshot.
func (b *InMemoryBackend) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteSnapshot")
	defer b.mu.Unlock()

	if _, ok := b.snapshotGet(region, snapshotID); !ok {
		return ErrSnapshotNotFound
	}

	b.snapshotDelete(region, snapshotID)

	return nil
}

// DescribeSnapshots returns snapshots filtered by directory and/or snapshot IDs.
func (b *InMemoryBackend) DescribeSnapshots(
	ctx context.Context,
	directoryID string,
	snapshotIDs []string,
	limit int32,
	nextToken string,
) ([]*Snapshot, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeSnapshots")
	defer b.mu.RUnlock()

	// Build filter set for snapshot IDs.
	filterIDs := make(map[string]bool, len(snapshotIDs))
	for _, id := range snapshotIDs {
		filterIDs[id] = true
	}

	ids := make([]string, 0, len(b.snapshotsInRegion(region)))
	for _, snap := range b.snapshotsInRegion(region) {
		if directoryID != "" && snap.DirectoryID != directoryID {
			continue
		}
		if len(filterIDs) > 0 && !filterIDs[snap.SnapshotID] {
			continue
		}
		ids = append(ids, snap.SnapshotID)
	}
	sort.Strings(ids)

	start := 0
	if nextToken != "" {
		if n, err := decodePageToken(nextToken); err == nil && n > 0 {
			start = n
		}
	}

	pageSize := int(limit)
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 1000
	}

	end := min(start+pageSize, len(ids))

	result := make([]*Snapshot, 0, end-start)
	for _, id := range ids[start:end] {
		s, _ := b.snapshotGet(region, id)
		cp := s.toSnapshot()
		result = append(result, &cp)
	}

	var outToken string
	if end < len(ids) {
		outToken = encodePageToken(end)
	}

	return result, outToken, nil
}

// GetSnapshotLimits returns snapshot limits for a directory.
func (b *InMemoryBackend) GetSnapshotLimits(
	ctx context.Context,
	directoryID string,
) (*SnapshotLimits, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetSnapshotLimits")
	defer b.mu.RUnlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return nil, ErrDirectoryNotFound
	}

	var count int32
	for _, snap := range b.snapshotsInRegion(region) {
		if snap.DirectoryID == directoryID && snap.SnapType == string(SnapshotTypeManual) {
			count++
		}
	}

	return &SnapshotLimits{
		ManualSnapshotsCurrentCount: count,
		ManualSnapshotsLimit:        defaultSnapshotLimit,
		ManualSnapshotsLimitReached: count >= defaultSnapshotLimit,
	}, nil
}

// RestoreFromSnapshot simulates restoring a directory from a snapshot.
func (b *InMemoryBackend) RestoreFromSnapshot(ctx context.Context, snapshotID string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("RestoreFromSnapshot")
	defer b.mu.Unlock()

	snap, ok := b.snapshotGet(region, snapshotID)
	if !ok {
		return ErrSnapshotNotFound
	}

	dir, ok := b.directoryGet(region, snap.DirectoryID)
	if !ok {
		return ErrDirectoryNotFound
	}

	setStage(dir, DirectoryStageRestoring)

	dirID := dir.DirectoryID

	go func(region, id string) {
		time.Sleep(restoreLifecycleDelay)

		b.mu.Lock("RestoreFromSnapshot:active")
		if d, exists := b.directoryGet(region, id); exists && d.Stage == string(DirectoryStageRestoring) {
			setStage(d, DirectoryStageActive)
		}
		b.mu.Unlock()
	}(region, dirID)

	return nil
}
