package efs

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// validateProvisionedThroughput checks provisioned throughput constraints.
func validateProvisionedThroughput(mode string, mib float64) error {
	if mode == throughputModeProvisioned {
		if mib < 1 || mib > 1024 {
			return fmt.Errorf(
				"%w: ProvisionedThroughputInMibps must be between 1 and 1024 when ThroughputMode is provisioned, got %g",
				ErrValidation,
				mib,
			)
		}
	} else if mib != 0 {
		return fmt.Errorf(
			"%w: ProvisionedThroughputInMibps is only valid when ThroughputMode is provisioned",
			ErrValidation,
		)
	}

	return nil
}

// validateCreateFSRequest validates and normalizes a CreateFileSystemRequest,
// returning the resolved KMS key ID on success.
func validateCreateFSRequest(req *CreateFileSystemRequest) (string, error) {
	if len(req.CreationToken) > maxCreationTokenLen {
		return "", fmt.Errorf(
			"%w: CreationToken length must be 1-%d, got %d",
			ErrValidation,
			maxCreationTokenLen,
			len(req.CreationToken),
		)
	}

	if err := validateTags(req.Tags); err != nil {
		return "", err
	}

	if req.PerformanceMode == "" {
		req.PerformanceMode = performanceModeGeneral
	}
	if req.ThroughputMode == "" {
		req.ThroughputMode = throughputModeBursting
	}

	if req.PerformanceMode != performanceModeGeneral &&
		req.PerformanceMode != performanceModeMaxIO {
		return "", fmt.Errorf(
			"%w: invalid PerformanceMode %q, must be generalPurpose or maxIO",
			ErrValidation,
			req.PerformanceMode,
		)
	}
	if req.ThroughputMode != throughputModeBursting &&
		req.ThroughputMode != throughputModeProvisioned &&
		req.ThroughputMode != throughputModeElastic {
		return "", fmt.Errorf(
			"%w: invalid ThroughputMode %q, must be bursting, provisioned, or elastic",
			ErrValidation,
			req.ThroughputMode,
		)
	}

	if err := validateProvisionedThroughput(req.ThroughputMode, req.ProvisionedThroughputMib); err != nil {
		return "", err
	}

	kmsKeyID := req.KmsKeyID
	if req.Encrypted && kmsKeyID == "" {
		kmsKeyID = managedKMSKeyARN
	}
	if !req.Encrypted && kmsKeyID != "" {
		return "", fmt.Errorf(
			"%w: KmsKeyID can only be specified when Encrypted is true",
			ErrValidation,
		)
	}

	return kmsKeyID, nil
}

// applyInitialBackupPolicy sets the backup policy a newly created file
// system starts with, per CreateFileSystemInput.Backup's documented default:
// false, or true when AvailabilityZoneName is set (One Zone). Must be called
// while holding b.mu.
func (b *InMemoryBackend) applyInitialBackupPolicy(region, id string, req CreateFileSystemRequest) {
	enableBackup := req.AvailabilityZoneName != ""
	if req.Backup != nil {
		enableBackup = *req.Backup
	}
	if enableBackup {
		b.backupStore(region)[id] = backupStatusEnabled
	}
}

// CreateFileSystem creates a new EFS file system.
func (b *InMemoryBackend) CreateFileSystem(
	ctx context.Context,
	req CreateFileSystemRequest,
) (*FileSystem, error) {
	kmsKeyID, err := validateCreateFSRequest(&req)
	if err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateFileSystem")
	defer b.mu.Unlock()

	tokenIdx := b.tokenIdxStore(region)

	// O(1) idempotency check via creation-token index.
	if existingID, ok := tokenIdx[req.CreationToken]; ok {
		fs, _ := b.fileSystems.Get(regionKey(region, existingID))
		cp := *fs

		if fs.PerformanceMode == req.PerformanceMode &&
			fs.ThroughputMode == req.ThroughputMode &&
			fs.Encrypted == req.Encrypted &&
			fs.KmsKeyID == req.KmsKeyID &&
			fs.AvailabilityZoneName == req.AvailabilityZoneName {
			return &cp, fmt.Errorf(
				"%w: file system with token %s already exists (identical args)",
				ErrCreationTokenExists,
				req.CreationToken,
			)
		}

		return &cp, fmt.Errorf(
			"%w: file system with token %s already exists with different parameters (FileSystemId: %s)",
			ErrAlreadyExists,
			req.CreationToken,
			fs.FileSystemID,
		)
	}

	id := "fs-" + uuid.NewString()[:8]
	fsARN := arn.Build("elasticfilesystem", region, b.accountID, "file-system/"+id)
	t := tags.New("efs.filesystem." + id + ".tags")

	tagCopy := make(map[string]string, len(req.Tags))
	maps.Copy(tagCopy, req.Tags)

	if len(tagCopy) > 0 {
		t.Merge(tagCopy)
	}

	name := req.Tags["Name"]

	initialState := statusAvailable
	if b.fsActivationDelay > 0 {
		initialState = statusCreating
	}

	fs := &FileSystem{
		FileSystemID:                   id,
		FileSystemArn:                  fsARN,
		CreationToken:                  req.CreationToken,
		Name:                           name,
		PerformanceMode:                req.PerformanceMode,
		ThroughputMode:                 req.ThroughputMode,
		LifeCycleState:                 initialState,
		Encrypted:                      req.Encrypted,
		KmsKeyID:                       kmsKeyID,
		AvailabilityZoneName:           req.AvailabilityZoneName,
		ProvisionedThroughputMib:       req.ProvisionedThroughputMib,
		ReplicationOverwriteProtection: protectionDisabled,
		AccountID:                      b.accountID,
		Region:                         region,
		CreationTime:                   time.Now().UTC(),
		Tags:                           t,
	}
	b.fileSystems.Put(fs)
	b.fileSystemsByARN.Put(fs)
	tokenIdx[req.CreationToken] = id

	b.applyInitialBackupPolicy(region, id, req)

	// When a non-zero activation delay is configured, simulate the AWS
	// "creating" → "available" lifecycle transition asynchronously.
	// The goroutine is self-terminating and guards against concurrent deletion.
	if b.fsActivationDelay > 0 {
		delay := b.fsActivationDelay

		go func() {
			time.Sleep(delay)
			b.mu.Lock("CreateFileSystem.activate")
			defer b.mu.Unlock()
			if cur, ok := b.fileSystems.Get(regionKey(region, id)); ok && cur.LifeCycleState == statusCreating {
				cur.LifeCycleState = statusAvailable
			}
		}()
	}

	cp := *fs

	return &cp, nil
}

// DescribeFileSystems returns file systems, optionally filtered by ID or creation token, with pagination support.
func (b *InMemoryBackend) DescribeFileSystems(
	ctx context.Context,
	fileSystemID, creationToken, marker string,
	maxItems int,
) ([]*FileSystem, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeFileSystems")
	defer b.mu.RUnlock()

	if fileSystemID != "" {
		fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
		if !ok {
			return nil, "", fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
		}
		cp := *fs

		return []*FileSystem{&cp}, "", nil
	}

	regionFS := b.fileSystemsByRegion.Get(region)

	if creationToken != "" {
		for _, fs := range regionFS {
			if fs.CreationToken == creationToken {
				cp := *fs

				return []*FileSystem{&cp}, "", nil
			}
		}

		return []*FileSystem{}, "", nil
	}

	all := make([]*FileSystem, 0, len(regionFS))
	for _, fs := range regionFS {
		cp := *fs
		all = append(all, &cp)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].FileSystemID < all[j].FileSystemID })

	return paginate(all, marker, maxItems, func(fs *FileSystem) string { return fs.FileSystemID })
}

// DeleteFileSystem deletes a file system by ID.
// Returns ErrFileSystemInUse if mount targets, access points, or a
// replication configuration exist for it.
func (b *InMemoryBackend) DeleteFileSystem(ctx context.Context, fileSystemID string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteFileSystem")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}

	// O(1) conflict check via indexes: reject delete if mount targets or access points exist.
	if b.mtSubnetIdx[region] != nil && len(b.mtSubnetIdx[region][fileSystemID]) > 0 {
		return fmt.Errorf(
			"%w: file system %s has existing mount targets",
			ErrFileSystemInUse,
			fileSystemID,
		)
	}

	if b.apByFS[region] != nil && len(b.apByFS[region][fileSystemID]) > 0 {
		return fmt.Errorf(
			"%w: file system %s has existing access points",
			ErrFileSystemInUse,
			fileSystemID,
		)
	}

	if _, exists := b.replicationConfigs.Get(regionKey(region, fileSystemID)); exists {
		return fmt.Errorf(
			"%w: file system %s is part of an EFS replication configuration; delete the replication "+
				"configuration first",
			ErrFileSystemInUse,
			fileSystemID,
		)
	}

	b.fileSystemsByARN.Delete(regionKey(region, fs.FileSystemArn))
	// Remove from creation-token index so the token can be reused.
	if b.creationTokenIdx[region] != nil {
		delete(b.creationTokenIdx[region], fs.CreationToken)
	}

	fs.Tags.Close()
	b.fileSystems.Delete(regionKey(region, fileSystemID))
	delete(b.lifecycleStore(region), fileSystemID)
	delete(b.backupStore(region), fileSystemID)
	delete(b.fsPolicyStore(region), fileSystemID)

	return nil
}

// applyThroughputModeChange validates and applies a throughput mode change to
// a file system. Must be called under b.mu write lock. UpdateFileSystem (this
// helper's only caller) declares BadRequest, never ValidationException, for
// malformed input (efs@v1.44.4 deserializers.go).
func (b *InMemoryBackend) applyThroughputModeChange(
	fs *FileSystem,
	req UpdateFileSystemRequest,
) error {
	if req.ThroughputMode != throughputModeBursting &&
		req.ThroughputMode != throughputModeProvisioned &&
		req.ThroughputMode != throughputModeElastic {
		return fmt.Errorf(
			"%w: invalid ThroughputMode %q, must be bursting, provisioned, or elastic",
			ErrBadRequest,
			req.ThroughputMode,
		)
	}

	if !fs.LastThroughputChange.IsZero() &&
		time.Since(fs.LastThroughputChange) < throughputCooldown {
		return fmt.Errorf(
			"%w: throughput mode was last changed at %s; must wait 24 hours between changes",
			ErrTooManyRequests,
			fs.LastThroughputChange.Format(time.RFC3339),
		)
	}

	if req.ThroughputMode == throughputModeProvisioned {
		if req.ProvisionedThroughputMib < 1 || req.ProvisionedThroughputMib > 1024 {
			return fmt.Errorf(
				"%w: ProvisionedThroughputInMibps must be between 1 and 1024 when ThroughputMode is provisioned, got %g",
				ErrBadRequest,
				req.ProvisionedThroughputMib,
			)
		}
	}

	fs.ThroughputMode = req.ThroughputMode
	fs.LastThroughputChange = time.Now().UTC()

	return nil
}

// checkFileSystemAvailable returns ErrIncorrectFileSystemLifeCycleState unless fs is in
// the "available" state, per the CreateMountTarget precondition
// (api_op_CreateMountTarget.go:29-30) shared by every op that declares the same error.
// Callers must hold b.mu.
func checkFileSystemAvailable(fs *FileSystem) error {
	if fs.LifeCycleState != statusAvailable {
		return fmt.Errorf(
			"%w: file system %s is in lifecycle state %q, not %q",
			ErrIncorrectFileSystemLifeCycleState,
			fs.FileSystemID,
			fs.LifeCycleState,
			statusAvailable,
		)
	}

	return nil
}

// UpdateFileSystem updates throughput settings for a file system.
// Enforces a 24-hour cooldown between throughput mode changes.
func (b *InMemoryBackend) UpdateFileSystem(
	ctx context.Context,
	fileSystemID string,
	req UpdateFileSystemRequest,
) (*FileSystem, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateFileSystem")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(regionKey(region, fileSystemID))
	if !ok {
		return nil, fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}
	if err := checkFileSystemAvailable(fs); err != nil {
		return nil, err
	}

	if req.ThroughputMode != "" {
		if err := b.applyThroughputModeChange(fs, req); err != nil {
			return nil, err
		}
	}

	if req.ProvisionedThroughputMib != 0 {
		if fs.ThroughputMode != throughputModeProvisioned {
			return nil, fmt.Errorf(
				"%w: ProvisionedThroughputInMibps is only valid when ThroughputMode is provisioned",
				ErrBadRequest,
			)
		}
		if req.ProvisionedThroughputMib < 1 || req.ProvisionedThroughputMib > 1024 {
			return nil, fmt.Errorf(
				"%w: ProvisionedThroughputInMibps must be between 1 and 1024, got %g",
				ErrBadRequest,
				req.ProvisionedThroughputMib,
			)
		}
		fs.ProvisionedThroughputMib = req.ProvisionedThroughputMib
	}

	cp := *fs

	return &cp, nil
}

// AddFileSystemInternal inserts a pre-built FileSystem directly into the backend (test seed helper).
func (b *InMemoryBackend) AddFileSystemInternal(fs *FileSystem) {
	b.mu.Lock("AddFileSystemInternal")
	defer b.mu.Unlock()

	region := fs.Region
	if region == "" {
		region = regionFromARN(fs.FileSystemArn, b.region)
		fs.Region = region
	}

	b.fileSystems.Put(fs)
	b.fileSystemsByARN.Put(fs)
}
