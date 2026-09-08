package fsx

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// storedBackup is the persisted form of a Backup.
// time.Time is first: non-pointer prefix (wall, ext) reduces GC pointer bytes.
//
// FileSystem is a snapshot of the source file system's metadata taken at
// backup-creation time, not a live lookup: real AWS's Backup.FileSystem doc
// states this metadata "is persisted even if the file system is deleted"
// (aws-sdk-go-v2/service/fsx@v1.68.4/types/types.go), and it is a required
// response member (gopherstack-r80d).
type storedBackup struct {
	CreationTime time.Time         `json:"creationTime"`
	Tags         map[string]string `json:"tags"`
	FileSystem   *storedFileSystem `json:"fileSystem,omitempty"`
	FileSystemID string            `json:"fileSystemId"`
	BackupID     string            `json:"backupId"`
	BackupType   string            `json:"backupType"`
	Lifecycle    string            `json:"lifecycle"`
	ResourceARN  string            `json:"resourceArn"`
}

// cloneStoredFileSystem deep-copies fs so a snapshot embedded in a
// storedBackup never aliases the live fileSystems table entry -- store.Table
// returns the live pointer, and UpdateFileSystem mutates it in place.
func cloneStoredFileSystem(fs *storedFileSystem) *storedFileSystem {
	if fs == nil {
		return nil
	}

	clone := *fs
	if fs.Tags != nil {
		clone.Tags = make(map[string]string, len(fs.Tags))
		maps.Copy(clone.Tags, fs.Tags)
	}

	clone.SubnetIDs = append([]string(nil), fs.SubnetIDs...)
	clone.NetworkInterfaceIDs = append([]string(nil), fs.NetworkInterfaceIDs...)

	return &clone
}

func (b *storedBackup) toBackup(fallbackFS *storedFileSystem) *Backup {
	bk := &Backup{
		BackupID:     b.BackupID,
		BackupType:   b.BackupType,
		CreationTime: epochTime(b.CreationTime),
		Lifecycle:    b.Lifecycle,
		ResourceARN:  b.ResourceARN,
		Tags:         tagsMapToSlice(b.Tags),
	}

	switch {
	case b.FileSystem != nil:
		bk.FileSystem = b.FileSystem.toFileSystem()
	case fallbackFS != nil:
		bk.FileSystem = fallbackFS.toFileSystem()
	}

	return bk
}

// createBackupInput holds parameters for CreateBackup.
type createBackupInput struct {
	FileSystemID string `json:"FileSystemId"`
	Tags         []Tag  `json:"Tags,omitempty"`
}

// CreateBackup creates a backup of the specified file system.
func (b *InMemoryBackend) CreateBackup(input *createBackupInput) (*Backup, error) {
	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateBackup")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(input.FileSystemID)
	if !ok {
		return nil, ErrFileSystemNotFound
	}

	id := newFSxBackupID()
	arn := b.backupARN(id)
	now := time.Now().UTC()

	tags := tagsSliceToMap(input.Tags)

	bk := &storedBackup{
		BackupID:     id,
		BackupType:   backupTypeUserInitiated,
		CreationTime: now,
		Lifecycle:    lifecycleAvailable,
		ResourceARN:  arn,
		Tags:         tags,
		FileSystemID: input.FileSystemID,
		FileSystem:   cloneStoredFileSystem(fs),
	}

	b.backups.Put(bk)
	b.tags[arn] = tags

	return bk.toBackup(fs), nil
}

// backupFilterValue resolves the value of a supported DescribeBackups filter
// name (file-system-id, backup-type, file-system-type -- the names
// DescribeBackupsInput's own doc comment documents as supported;
// aws-sdk-go-v2/service/fsx@v1.68.4 api_op_DescribeBackups.go) for bk. Its own
// Volume (real Backup.Volume, for ONTAP/OpenZFS volume backups) isn't tracked
// by this backend's CreateBackup, so volume-id has no honest value to compare
// against and isn't recognized here -- a request setting it matches every
// backup rather than none, same as AWS treating an unset/unsupported filter.
func backupFilterValue(bk *storedBackup, fallbackFS *storedFileSystem, name string) (string, bool) {
	switch name {
	case filterNameFileSystemID:
		return bk.FileSystemID, true
	case "backup-type":
		return bk.BackupType, true
	case "file-system-type":
		switch {
		case bk.FileSystem != nil:
			return bk.FileSystem.FileSystemType, true
		case fallbackFS != nil:
			return fallbackFS.FileSystemType, true
		default:
			return "", true
		}
	default:
		return "", false
	}
}

// filteredBackupsLocked returns every backup matching filters, sorted by
// BackupID. Caller must already hold b.mu (read or write).
func (b *InMemoryBackend) filteredBackupsLocked(filters []wireFilter) []*storedBackup {
	var all []*storedBackup

	for _, bk := range b.backups.All() {
		var fallbackFS *storedFileSystem
		if bk.FileSystem == nil && bk.FileSystemID != "" {
			fallbackFS, _ = b.fileSystems.Get(bk.FileSystemID)
		}

		if matchesFilters(filters, func(name string) (string, bool) {
			return backupFilterValue(bk, fallbackFS, name)
		}) {
			all = append(all, bk)
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].BackupID < all[j].BackupID })

	return all
}

// DescribeBackups returns backups, optionally filtered by IDs or Filters.
// Per DescribeBackupsInput's own doc comment, BackupIds overrides Filters
// entirely when both are set.
func (b *InMemoryBackend) DescribeBackups(
	backupIDs []string,
	filters []wireFilter,
	maxResults int32,
	nextToken string,
) ([]*Backup, string, error) {
	b.mu.RLock("DescribeBackups")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedBackup

	if len(backupIDs) > 0 {
		for _, id := range backupIDs {
			bk, ok := b.backups.Get(id)
			if !ok {
				return nil, "", ErrBackupNotFound
			}

			all = append(all, bk)
		}
	} else {
		all = b.filteredBackupsLocked(filters)
	}

	start := 0
	if nextToken != "" {
		for i, bk := range all {
			if bk.BackupID == nextToken {
				start = i

				break
			}
		}
	}

	end := min(start+int(maxResults), len(all))
	page := all[start:end]

	var next string
	if end < len(all) {
		next = all[end].BackupID
	}

	result := make([]*Backup, len(page))
	for i, bk := range page {
		var fs *storedFileSystem
		if bk.FileSystemID != "" {
			fs, _ = b.fileSystems.Get(bk.FileSystemID)
		}

		result[i] = bk.toBackup(fs)
	}

	return result, next, nil
}

// DeleteBackup removes a backup.
func (b *InMemoryBackend) DeleteBackup(backupID string) error {
	b.mu.Lock("DeleteBackup")
	defer b.mu.Unlock()

	bk, ok := b.backups.Get(backupID)
	if !ok {
		return ErrBackupNotFound
	}

	b.backups.Delete(backupID)
	delete(b.tags, bk.ResourceARN)

	return nil
}

// copyBackupInput holds parameters for CopyBackup.
type copyBackupInput struct {
	SourceBackupID string `json:"SourceBackupId"`
	Tags           []Tag  `json:"Tags,omitempty"`
}

// CopyBackup creates a copy of an existing backup.
func (b *InMemoryBackend) CopyBackup(input *copyBackupInput) (*Backup, error) {
	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CopyBackup")
	defer b.mu.Unlock()

	src, ok := b.backups.Get(input.SourceBackupID)
	if !ok {
		return nil, ErrBackupNotFound
	}

	id := newFSxBackupID()
	arn := b.backupARN(id)
	now := time.Now().UTC()

	tags := tagsSliceToMap(input.Tags)

	// Prefer the source backup's own frozen snapshot over a fresh live lookup:
	// src's source file system may itself have been deleted since src was
	// created, and src.FileSystem is the correct metadata to propagate either
	// way (CopyBackup does not re-derive metadata from current live state).
	var fs *storedFileSystem

	switch {
	case src.FileSystem != nil:
		fs = cloneStoredFileSystem(src.FileSystem)
	case src.FileSystemID != "":
		if live, found := b.fileSystems.Get(src.FileSystemID); found {
			fs = cloneStoredFileSystem(live)
		}
	}

	bk := &storedBackup{
		BackupID:     id,
		BackupType:   src.BackupType,
		CreationTime: now,
		Lifecycle:    lifecycleAvailable,
		ResourceARN:  arn,
		Tags:         tags,
		FileSystemID: src.FileSystemID,
		FileSystem:   fs,
	}

	b.backups.Put(bk)
	b.tags[arn] = tags

	return bk.toBackup(fs), nil
}

func (b *InMemoryBackend) backupARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("backup/%s", id))
}
