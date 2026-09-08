package efs

import (
	"context"
	"fmt"
)

// DescribeBackupPolicy returns the backup policy for a file system.
func (b *InMemoryBackend) DescribeBackupPolicy(ctx context.Context, fileSystemID string) (string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeBackupPolicy")
	defer b.mu.RUnlock()

	if _, ok := b.fileSystems.Get(regionKey(region, fileSystemID)); !ok {
		return "", fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}

	status, ok := b.backupStoreRO(region)[fileSystemID]
	if !ok {
		return backupStatusDisabled, nil
	}

	return status, nil
}

// PutBackupPolicy sets the backup policy status for a file system.
// Valid values: ENABLED, ENABLING, DISABLED, DISABLING.
func (b *InMemoryBackend) PutBackupPolicy(ctx context.Context, fileSystemID, status string) error {
	switch status {
	case backupStatusEnabled, backupStatusEnabling, backupStatusDisabled, "DISABLING":
		// valid
	default:
		return fmt.Errorf(
			"%w: invalid backup policy status %q, must be ENABLED, ENABLING, DISABLED, or DISABLING",
			ErrValidation,
			status,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("PutBackupPolicy")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return err
	}

	b.backupStore(region)[fileSystemID] = status

	return nil
}
