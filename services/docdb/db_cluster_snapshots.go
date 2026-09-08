package docdb

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func (b *InMemoryBackend) CreateDBClusterSnapshot(
	ctx context.Context,
	snapshotID, clusterID string,
	tags map[string]string,
) (*DBClusterSnapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: DBClusterSnapshotIdentifier is required", ErrInvalidParameter)
	}
	if clusterID == "" {
		return nil, fmt.Errorf("%w: DBClusterIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateDBClusterSnapshot")
	defer b.mu.Unlock()
	if b.clusterSnapshotHas(region, snapshotID) {
		return nil, fmt.Errorf("%w: cluster snapshot %s already exists", ErrClusterSnapshotAlreadyExists, snapshotID)
	}
	c, exists := b.clusterGet(region, clusterID)
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}
	snapArn := b.clusterSnapshotARN(region, snapshotID)
	azs := make([]string, len(c.AvailabilityZones))
	copy(azs, c.AvailabilityZones)
	snap := &DBClusterSnapshot{
		region:                      region,
		DBClusterSnapshotIdentifier: snapshotID,
		DBClusterIdentifier:         clusterID,
		Engine:                      c.Engine,
		Status:                      statusAvailable,
		EngineVersion:               c.EngineVersion,
		StorageEncrypted:            c.StorageEncrypted,
		SnapshotType:                "manual",
		PercentProgress:             snapshotPercentageComplete,
		SnapshotCreateTime:          time.Now().UTC().Format(time.RFC3339),
		ClusterCreateTime:           c.ClusterCreateTime,
		KmsKeyID:                    c.KmsKeyID,
		MasterUsername:              c.MasterUsername,
		Port:                        c.Port,
		AvailabilityZones:           azs,
		DBClusterArn:                b.clusterARN(region, clusterID),
		DBClusterSnapshotArn:        snapArn,
		Tags:                        copyTags(tags),
	}
	b.clusterSnapshotPut(snap)
	if len(tags) > 0 {
		b.tagsStore(region)[snapArn] = tagsFromMap(tags)
	}
	b.recordEvent(
		region, snapshotID, sourceTypeDBClusterSnapshot, snapArn,
		"DB cluster snapshot created", eventCatBackup, eventCatCreate,
	)
	cp := *snap
	cp.Tags = copyTags(snap.Tags)

	return &cp, nil
}

func (b *InMemoryBackend) DescribeDBClusterSnapshots(
	ctx context.Context,
	snapshotID, clusterID, snapshotType string,
) ([]DBClusterSnapshot, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBClusterSnapshots")
	defer b.mu.RUnlock()
	if snapshotID != "" {
		snap, exists := b.clusterSnapshotGet(region, snapshotID)
		if !exists {
			return nil, fmt.Errorf("%w: cluster snapshot %s not found", ErrClusterSnapshotNotFound, snapshotID)
		}
		cp := *snap
		cp.Tags = copyTags(snap.Tags)

		return []DBClusterSnapshot{cp}, nil
	}
	snapshots := b.clusterSnapshotsInRegion(region)
	result := make([]DBClusterSnapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		if clusterID != "" && snap.DBClusterIdentifier != clusterID {
			continue
		}
		if snapshotType != "" && snap.SnapshotType != snapshotType {
			continue
		}
		cp := *snap
		cp.Tags = copyTags(snap.Tags)
		result = append(result, cp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].DBClusterSnapshotIdentifier < result[j].DBClusterSnapshotIdentifier
	})

	return result, nil
}

func (b *InMemoryBackend) DeleteDBClusterSnapshot(ctx context.Context, snapshotID string) (*DBClusterSnapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: DBClusterSnapshotIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteDBClusterSnapshot")
	defer b.mu.Unlock()
	snap, exists := b.clusterSnapshotGet(region, snapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: cluster snapshot %s not found", ErrClusterSnapshotNotFound, snapshotID)
	}
	cp := *snap
	cp.Tags = copyTags(snap.Tags)
	snapArn := b.clusterSnapshotARN(region, snapshotID)
	b.clusterSnapshotDelete(region, snapshotID)
	delete(b.tagsStore(region), snapArn)
	b.snapshotAttributesDelete(region, snapshotID)
	b.recordEvent(
		region, snapshotID, sourceTypeDBClusterSnapshot, snapArn,
		"DB cluster snapshot deleted", eventCatBackup, eventCatDelete,
	)

	return &cp, nil
}

// CopyDBClusterSnapshot copies a DB cluster snapshot. tags is the target's
// own explicit "Tags" request member; copyTagsFromSource mirrors the
// request's "CopyTags" flag ("Set to true to copy all tags from the source
// cluster snapshot to the target cluster snapshot", CopyDBClusterSnapshotInput
// doc comment). Neither was read from the request at all before this fix,
// so a real client's CopyTags=true/Tags request was silently a no-op. The
// SDK doc comment doesn't state a precedence rule for tags+CopyTags used
// together, so this takes the more-specific explicit tags when given and
// falls back to the source's tags only when tags is empty -- an
// interpretation, not a confirmed AWS rule.
func (b *InMemoryBackend) CopyDBClusterSnapshot(
	ctx context.Context,
	sourceSnapshotID, targetSnapshotID string,
	tags map[string]string,
	copyTagsFromSource bool,
) (*DBClusterSnapshot, error) {
	if sourceSnapshotID == "" {
		return nil, fmt.Errorf("%w: SourceDBClusterSnapshotIdentifier is required", ErrInvalidParameter)
	}
	if targetSnapshotID == "" {
		return nil, fmt.Errorf("%w: TargetDBClusterSnapshotIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CopyDBClusterSnapshot")
	defer b.mu.Unlock()
	src, exists := b.clusterSnapshotGet(region, sourceSnapshotID)
	if !exists {
		return nil, fmt.Errorf("%w: cluster snapshot %s not found", ErrClusterSnapshotNotFound, sourceSnapshotID)
	}
	if b.clusterSnapshotHas(region, targetSnapshotID) {
		return nil, fmt.Errorf(
			"%w: cluster snapshot %s already exists",
			ErrClusterSnapshotAlreadyExists,
			targetSnapshotID,
		)
	}
	azs := make([]string, len(src.AvailabilityZones))
	copy(azs, src.AvailabilityZones)

	snapTags := tags
	if len(snapTags) == 0 && copyTagsFromSource {
		snapTags = src.Tags
	}

	snap := &DBClusterSnapshot{
		region:                      region,
		DBClusterSnapshotIdentifier: targetSnapshotID,
		DBClusterIdentifier:         src.DBClusterIdentifier,
		DBClusterArn:                src.DBClusterArn,
		DBClusterSnapshotArn:        b.clusterSnapshotARN(region, targetSnapshotID),
		SourceDBClusterSnapshotArn:  src.DBClusterSnapshotArn,
		Engine:                      src.Engine,
		Status:                      statusAvailable,
		EngineVersion:               src.EngineVersion,
		StorageEncrypted:            src.StorageEncrypted,
		SnapshotType:                src.SnapshotType,
		PercentProgress:             src.PercentProgress,
		ClusterCreateTime:           src.ClusterCreateTime,
		KmsKeyID:                    src.KmsKeyID,
		MasterUsername:              src.MasterUsername,
		Port:                        src.Port,
		AvailabilityZones:           azs,
		Tags:                        copyTags(snapTags),
		// SnapshotCreateTime is stamped fresh at copy time, not copied from
		// src: real AWS's CopyDBClusterSnapshot creates a genuinely new
		// snapshot resource with its own creation timestamp, distinct from
		// the source's.
		SnapshotCreateTime: time.Now().UTC().Format(time.RFC3339),
	}
	b.clusterSnapshotPut(snap)
	if len(snap.Tags) > 0 {
		b.tagsStore(region)[snap.DBClusterSnapshotArn] = tagsFromMap(snap.Tags)
	}
	cp := *snap
	cp.Tags = copyTags(snap.Tags)

	return &cp, nil
}

// DescribeDBClusterSnapshotAttributes returns attributes for a cluster snapshot.
func (b *InMemoryBackend) DescribeDBClusterSnapshotAttributes(
	ctx context.Context,
	snapshotID string,
) (*DBClusterSnapshotAttributesResult, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: DBClusterSnapshotIdentifier is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBClusterSnapshotAttributes")
	defer b.mu.RUnlock()
	if !b.clusterSnapshotHas(region, snapshotID) {
		return nil, fmt.Errorf("%w: cluster snapshot %s not found", ErrClusterSnapshotNotFound, snapshotID)
	}
	result, ok := b.snapshotAttributesGet(region, snapshotID)
	if !ok {
		return &DBClusterSnapshotAttributesResult{
			DBClusterSnapshotIdentifier: snapshotID,
			Attributes:                  []DBClusterSnapshotAttribute{},
		}, nil
	}
	cp := &DBClusterSnapshotAttributesResult{
		DBClusterSnapshotIdentifier: result.DBClusterSnapshotIdentifier,
		Attributes:                  make([]DBClusterSnapshotAttribute, len(result.Attributes)),
	}
	for i, a := range result.Attributes {
		vals := make([]string, len(a.AttributeValues))
		copy(vals, a.AttributeValues)
		cp.Attributes[i] = DBClusterSnapshotAttribute{
			AttributeName:   a.AttributeName,
			AttributeValues: vals,
		}
	}

	return cp, nil
}

// ModifyDBClusterSnapshotAttribute modifies an attribute on a cluster snapshot.
// findOrCreateAttribute finds an existing attribute by name in the result, or creates and appends a new one.
func findOrCreateAttribute(
	result *DBClusterSnapshotAttributesResult,
	attributeName string,
) *DBClusterSnapshotAttribute {
	for i := range result.Attributes {
		if result.Attributes[i].AttributeName == attributeName {
			return &result.Attributes[i]
		}
	}
	result.Attributes = append(result.Attributes, DBClusterSnapshotAttribute{
		AttributeName:   attributeName,
		AttributeValues: []string{},
	})

	return &result.Attributes[len(result.Attributes)-1]
}

// applySnapshotAttributeChanges adds and removes values from an attribute in place.
func applySnapshotAttributeChanges(attr *DBClusterSnapshotAttribute, valuesToAdd, valuesToRemove []string) {
	existing := make(map[string]struct{}, len(attr.AttributeValues))
	for _, v := range attr.AttributeValues {
		existing[v] = struct{}{}
	}
	for _, v := range valuesToAdd {
		if _, ok := existing[v]; !ok {
			attr.AttributeValues = append(attr.AttributeValues, v)
			existing[v] = struct{}{}
		}
	}
	if len(valuesToRemove) == 0 {
		return
	}
	removeSet := make(map[string]bool, len(valuesToRemove))
	for _, v := range valuesToRemove {
		removeSet[v] = true
	}
	kept := attr.AttributeValues[:0]
	for _, v := range attr.AttributeValues {
		if !removeSet[v] {
			kept = append(kept, v)
		}
	}
	attr.AttributeValues = kept
}

// copySnapshotAttributesResult returns a deep copy of a DBClusterSnapshotAttributesResult.
func copySnapshotAttributesResult(result *DBClusterSnapshotAttributesResult) *DBClusterSnapshotAttributesResult {
	cp := &DBClusterSnapshotAttributesResult{
		DBClusterSnapshotIdentifier: result.DBClusterSnapshotIdentifier,
		Attributes:                  make([]DBClusterSnapshotAttribute, len(result.Attributes)),
	}
	for i, a := range result.Attributes {
		vals := make([]string, len(a.AttributeValues))
		copy(vals, a.AttributeValues)
		cp.Attributes[i] = DBClusterSnapshotAttribute{
			AttributeName:   a.AttributeName,
			AttributeValues: vals,
		}
	}

	return cp
}

func (b *InMemoryBackend) ModifyDBClusterSnapshotAttribute(
	ctx context.Context,
	snapshotID, attributeName string,
	valuesToAdd, valuesToRemove []string,
) (*DBClusterSnapshotAttributesResult, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("%w: DBClusterSnapshotIdentifier is required", ErrInvalidParameter)
	}
	if attributeName == "" {
		return nil, fmt.Errorf("%w: AttributeName is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyDBClusterSnapshotAttribute")
	defer b.mu.Unlock()
	if !b.clusterSnapshotHas(region, snapshotID) {
		return nil, fmt.Errorf("%w: cluster snapshot %s not found", ErrClusterSnapshotNotFound, snapshotID)
	}
	result, ok := b.snapshotAttributesGet(region, snapshotID)
	if !ok {
		result = &DBClusterSnapshotAttributesResult{
			region:                      region,
			DBClusterSnapshotIdentifier: snapshotID,
			Attributes:                  []DBClusterSnapshotAttribute{},
		}
	}
	attr := findOrCreateAttribute(result, attributeName)
	applySnapshotAttributeChanges(attr, valuesToAdd, valuesToRemove)
	b.snapshotAttributesPut(result)

	return copySnapshotAttributesResult(result), nil
}
