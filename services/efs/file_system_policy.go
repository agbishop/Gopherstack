package efs

import (
	"context"
	"encoding/json"
	"fmt"
)

// DescribeFileSystemPolicy returns the resource-based policy for a file system.
func (b *InMemoryBackend) DescribeFileSystemPolicy(ctx context.Context, fileSystemID string) (string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeFileSystemPolicy")
	defer b.mu.RUnlock()

	if _, ok := b.fileSystems.Get(regionKey(region, fileSystemID)); !ok {
		return "", fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}

	policy, ok := b.fsPolicyStoreRO(region)[fileSystemID]
	if !ok {
		return "", fmt.Errorf("%w: no policy found for file system %s", ErrPolicyNotFound, fileSystemID)
	}

	return policy, nil
}

// DeleteFileSystemPolicy removes the resource-based policy from a file system.
func (b *InMemoryBackend) DeleteFileSystemPolicy(ctx context.Context, fileSystemID string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteFileSystemPolicy")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return err
	}

	delete(b.fsPolicyStore(region), fileSystemID)

	return nil
}

// PutFileSystemPolicy sets the resource-based policy for a file system.
// The policy must be valid JSON and no larger than 20 KB.
func (b *InMemoryBackend) PutFileSystemPolicy(ctx context.Context, fileSystemID, policy string) error {
	if !json.Valid([]byte(policy)) {
		return fmt.Errorf("%w: FileSystemPolicy is not valid JSON", ErrInvalidPolicy)
	}
	if len(policy) > maxFileSystemPolicyBytes {
		return fmt.Errorf(
			"%w: FileSystemPolicy exceeds maximum size of %d bytes, got %d",
			ErrInvalidPolicy,
			maxFileSystemPolicyBytes,
			len(policy),
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("PutFileSystemPolicy")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return err
	}

	b.fsPolicyStore(region)[fileSystemID] = policy

	return nil
}
