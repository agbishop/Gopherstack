package efs

import (
	"context"
	"fmt"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
)

// isValidTransitionToIA derives its answer from types.TransitionToIARules.Values()
// so it cannot drift from the real enum -- the previous hand-copied list was
// missing AFTER_1_DAY and wrongly accepted "NONE", which isn't a TransitionToIARules
// member at all (LifecyclePolicy.TransitionToIA is simply omitted, not set to "NONE").
func isValidTransitionToIA(v string) bool {
	for _, e := range sdktypes.TransitionToIARules("").Values() {
		if string(e) == v {
			return true
		}
	}

	return false
}

func isValidTransitionToPrimary(v string) bool {
	return v == "AFTER_1_ACCESS"
}

// isValidTransitionToArchive derives its answer from
// types.TransitionToArchiveRules.Values() so it cannot drift from the real
// enum -- the previous hand-copied list was missing AFTER_1_DAY and wrongly
// accepted "AFTER_1_ACCESS" (that value belongs to the separate
// TransitionToPrimaryStorageClassRules enum) and a nonexistent "AFTER_90_DAYS_1".
func isValidTransitionToArchive(v string) bool {
	for _, e := range sdktypes.TransitionToArchiveRules("").Values() {
		if string(e) == v {
			return true
		}
	}

	return false
}

// DescribeLifecycleConfiguration returns lifecycle policies for a file system.
func (b *InMemoryBackend) DescribeLifecycleConfiguration(
	ctx context.Context,
	fileSystemID string,
) ([]LifecyclePolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeLifecycleConfiguration")
	defer b.mu.RUnlock()

	if _, ok := b.fileSystems.Get(regionKey(region, fileSystemID)); !ok {
		return nil, fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}

	policies := b.lifecycleStoreRO(region)[fileSystemID]
	if policies == nil {
		return []LifecyclePolicy{}, nil
	}

	result := make([]LifecyclePolicy, len(policies))
	copy(result, policies)

	return result, nil
}

// validateLifecyclePolicies checks that each policy's transition fields are valid AWS enum
// values. PutLifecycleConfiguration (this function's only caller) declares BadRequest,
// never ValidationException, for malformed input (efs@v1.44.4 deserializers.go).
func validateLifecyclePolicies(policies []LifecyclePolicy) error {
	for i, p := range policies {
		if p.TransitionToIA != "" && !isValidTransitionToIA(p.TransitionToIA) {
			return fmt.Errorf(
				"%w: invalid TransitionToIA value %q at index %d",
				ErrBadRequest,
				p.TransitionToIA,
				i,
			)
		}
		if p.TransitionToPrimaryStorageClass != "" &&
			!isValidTransitionToPrimary(p.TransitionToPrimaryStorageClass) {
			return fmt.Errorf(
				"%w: invalid TransitionToPrimaryStorageClass value %q at index %d",
				ErrBadRequest,
				p.TransitionToPrimaryStorageClass,
				i,
			)
		}
		if p.TransitionToArchive != "" && !isValidTransitionToArchive(p.TransitionToArchive) {
			return fmt.Errorf(
				"%w: invalid TransitionToArchive value %q at index %d",
				ErrBadRequest,
				p.TransitionToArchive,
				i,
			)
		}
	}

	return nil
}

// PutLifecycleConfiguration sets lifecycle policies for a file system.
func (b *InMemoryBackend) PutLifecycleConfiguration(
	ctx context.Context,
	fileSystemID string,
	policies []LifecyclePolicy,
) ([]LifecyclePolicy, error) {
	if err := validateLifecyclePolicies(policies); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("PutLifecycleConfiguration")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return nil, fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return nil, err
	}

	stored := make([]LifecyclePolicy, len(policies))
	copy(stored, policies)
	b.lifecycleStore(region)[fileSystemID] = stored

	result := make([]LifecyclePolicy, len(stored))
	copy(result, stored)

	return result, nil
}
