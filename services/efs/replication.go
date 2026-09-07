package efs

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateReplicationConfiguration creates a replication configuration for a file system.
func (b *InMemoryBackend) CreateReplicationConfiguration(
	ctx context.Context,
	sourceFileSystemID string,
	destinations []ReplicationDestination,
) (*ReplicationConfiguration, error) {
	if len(destinations) != maxReplicationDestinations {
		return nil, fmt.Errorf(
			"%w: exactly %d destination is required, got %d",
			ErrValidation,
			maxReplicationDestinations,
			len(destinations),
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateReplicationConfiguration")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, sourceFileSystemID))
	if !ok {
		return nil, fmt.Errorf("%w: file system %s not found", ErrNotFound, sourceFileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return nil, err
	}

	if _, exists := b.replicationConfigs.Get(regionKey(region, sourceFileSystemID)); exists {
		return nil, fmt.Errorf(
			"%w: replication configuration already exists for file system %s",
			ErrReplicationConfigExists,
			sourceFileSystemID,
		)
	}

	creationTime := time.Now().UTC()

	dests := make([]ReplicationDestination, len(destinations))
	copy(dests, destinations)
	for i := range dests {
		if dests[i].Status == "" {
			dests[i].Status = "ENABLED"
		}
		if dests[i].OwnerID == "" {
			dests[i].OwnerID = b.accountID
		}
		// Region is required on the response Destination but optional on the
		// request DestinationToCreate (same-region replication omits it) --
		// default to the source region rather than leave it empty, or the
		// required "Region" key vanishes under its own omitempty tag.
		if dests[i].Region == "" {
			dests[i].Region = region
		}
		// The mock completes the initial sync synchronously, so the destination is
		// immediately caught up as of creation time. Real AWS leaves this unset until
		// the first background sync completes, which can take longer.
		dests[i].LastReplicatedTimestamp = creationTime.Unix()
		// Assign a destination file-system ID and ARN when not provided by the caller.
		// Real AWS creates a read-only replica; we record a synthetic ID here.
		if dests[i].FileSystemID == "" {
			destFSID := "fs-" + uuid.NewString()[:8]
			dests[i].FileSystemID = destFSID
			dests[i].FileSystemArn = arn.Build(
				"elasticfilesystem", dests[i].Region, b.accountID, "file-system/"+destFSID,
			)
		} else if dests[i].FileSystemArn == "" {
			dests[i].FileSystemArn = arn.Build(
				"elasticfilesystem", dests[i].Region, b.accountID, "file-system/"+dests[i].FileSystemID,
			)
		}
	}

	rc := &ReplicationConfiguration{
		OriginalSourceFileSystemARN: fs.FileSystemArn,
		SourceFileSystemARN:         fs.FileSystemArn,
		SourceFileSystemID:          sourceFileSystemID,
		SourceFileSystemOwnerID:     b.accountID,
		SourceFileSystemRegion:      region,
		CreationTime:                creationTime.Unix(),
		Destinations:                dests,
	}
	b.replicationConfigs.Put(rc)

	// Mark source file system as replicating.
	fs.ReplicationOverwriteProtection = protectionReplicating

	cp := *rc
	cp.Destinations = make([]ReplicationDestination, len(rc.Destinations))
	copy(cp.Destinations, rc.Destinations)

	return &cp, nil
}

// DeleteReplicationConfiguration deletes the replication configuration for a file system.
// The destination file system (if tracked) becomes a standalone writable file system
// with ReplicationOverwriteProtection set to ENABLED.
func (b *InMemoryBackend) DeleteReplicationConfiguration(
	ctx context.Context,
	sourceFileSystemID string,
) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteReplicationConfiguration")
	defer b.mu.Unlock()

	if _, ok := b.fileSystems.Get(regionKey(region, sourceFileSystemID)); !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, sourceFileSystemID)
	}

	if _, exists := b.replicationConfigs.Get(regionKey(region, sourceFileSystemID)); !exists {
		return fmt.Errorf(
			"%w: replication configuration not found for file system %s",
			ErrNotFound,
			sourceFileSystemID,
		)
	}

	b.replicationConfigs.Delete(regionKey(region, sourceFileSystemID))

	// Reset source protection to DISABLED.
	if fs, ok := b.fileSystems.Get(regionKey(region, sourceFileSystemID)); ok {
		fs.ReplicationOverwriteProtection = protectionDisabled
	}

	return nil
}

// DescribeReplicationConfigurations returns replication configurations, optionally
// filtered by file system ID. marker/maxItems apply cursor-based pagination
// (wire names NextToken/MaxResults) over the unfiltered list, matching the
// pagination convention used by DescribeFileSystems/DescribeMountTargets/
// DescribeAccessPoints (see paginate in store.go). A single-ID match (whether by
// source or destination file system) is never paginated, same as the
// describeByIDOrFilter convention those ops use.
func (b *InMemoryBackend) DescribeReplicationConfigurations(
	ctx context.Context,
	fileSystemID, marker string,
	maxItems int,
) ([]*ReplicationConfiguration, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeReplicationConfigurations")
	defer b.mu.RUnlock()

	regionRCs := b.replicationConfigsByRegion.Get(region)

	if fileSystemID != "" {
		// Match source OR destination file system ID, matching real AWS behaviour.
		if rc, ok := b.replicationConfigs.Get(regionKey(region, fileSystemID)); ok {
			cp := *rc
			cp.Destinations = make([]ReplicationDestination, len(rc.Destinations))
			copy(cp.Destinations, rc.Destinations)

			return []*ReplicationConfiguration{&cp}, "", nil
		}

		for _, rc := range regionRCs {
			for _, d := range rc.Destinations {
				if d.FileSystemID == fileSystemID {
					cp := *rc
					cp.Destinations = make([]ReplicationDestination, len(rc.Destinations))
					copy(cp.Destinations, rc.Destinations)

					return []*ReplicationConfiguration{&cp}, "", nil
				}
			}
		}

		return []*ReplicationConfiguration{}, "", nil
	}

	list := make([]*ReplicationConfiguration, 0, len(regionRCs))
	for _, rc := range regionRCs {
		cp := *rc
		cp.Destinations = make([]ReplicationDestination, len(rc.Destinations))
		copy(cp.Destinations, rc.Destinations)
		list = append(list, &cp)
	}
	sort.Slice(
		list,
		func(i, j int) bool { return list[i].SourceFileSystemID < list[j].SourceFileSystemID },
	)

	return paginate(list, marker, maxItems, func(rc *ReplicationConfiguration) string {
		return rc.SourceFileSystemID
	})
}

// UpdateFileSystemProtection sets the replication overwrite protection for a file system.
func (b *InMemoryBackend) UpdateFileSystemProtection(
	ctx context.Context,
	fileSystemID, replicationOverwriteProtection string,
) error {
	switch replicationOverwriteProtection {
	case protectionEnabled, protectionDisabled, protectionReplicating:
		// valid
	default:
		// UpdateFileSystemProtection declares BadRequest, never ValidationException, for
		// malformed input (efs@v1.44.4 deserializers.go).
		return fmt.Errorf(
			"%w: invalid ReplicationOverwriteProtection value %q, must be ENABLED, DISABLED, or REPLICATING",
			ErrBadRequest,
			replicationOverwriteProtection,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateFileSystemProtection")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return err
	}

	fs.ReplicationOverwriteProtection = replicationOverwriteProtection

	return nil
}
