package fsx

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

type storedFileCache struct {
	CreationTime                     time.Time         `json:"creationTime"`
	Tags                             map[string]string `json:"tags"`
	FileCacheID                      string            `json:"fileCacheId"`
	FileCacheType                    string            `json:"fileCacheType"`
	FileCacheTypeVersion             string            `json:"fileCacheTypeVersion,omitempty"`
	Lifecycle                        string            `json:"lifecycle"`
	ResourceARN                      string            `json:"resourceArn"`
	LustreDeploymentType             string            `json:"lustreDeploymentType,omitempty"`
	LustreMountName                  string            `json:"lustreMountName,omitempty"`
	LustreWeeklyMaintenanceStartTime string            `json:"lustreWeeklyMaintenanceStartTime,omitempty"`
	SubnetIDs                        []string          `json:"subnetIds,omitempty"`
	StorageCapacityGiB               int32             `json:"storageCapacityGiB,omitempty"`
	LustreMetadataStorageCapacity    int32             `json:"lustreMetadataStorageCapacity,omitempty"`
	LustrePerUnitStorageThroughput   int32             `json:"lustrePerUnitStorageThroughput,omitempty"`
}

// lustreConfiguration builds the FileCacheLustreConfiguration response block.
// CreateFileCache requires LustreConfiguration (see applyFileCacheLustreConfig),
// so this is always non-nil once a file cache exists.
func (c *storedFileCache) lustreConfiguration() *FileCacheLustreConfiguration {
	return &FileCacheLustreConfiguration{
		MetadataConfiguration: &FileCacheLustreMetadataConfiguration{
			StorageCapacity: c.LustreMetadataStorageCapacity,
		},
		DeploymentType:             c.LustreDeploymentType,
		MountName:                  c.LustreMountName,
		WeeklyMaintenanceStartTime: c.LustreWeeklyMaintenanceStartTime,
		PerUnitStorageThroughput:   c.LustrePerUnitStorageThroughput,
	}
}

func (c *storedFileCache) toPublic() *FileCache {
	return &FileCache{
		CreationTime:         epochTime(c.CreationTime),
		LustreConfiguration:  c.lustreConfiguration(),
		FileCacheID:          c.FileCacheID,
		FileCacheType:        c.FileCacheType,
		FileCacheTypeVersion: c.FileCacheTypeVersion,
		Lifecycle:            c.Lifecycle,
		ResourceARN:          c.ResourceARN,
		SubnetIDs:            c.SubnetIDs,
		StorageCapacityGiB:   c.StorageCapacityGiB,
	}
}

// toPublicCreating renders the CreateFileCache response shape, which -- unlike
// DescribeFileCaches/UpdateFileCache's toPublic() above -- includes Tags (see
// FileCacheCreating in interfaces.go).
func (c *storedFileCache) toPublicCreating() *FileCacheCreating {
	return &FileCacheCreating{
		CreationTime:         epochTime(c.CreationTime),
		LustreConfiguration:  c.lustreConfiguration(),
		FileCacheID:          c.FileCacheID,
		FileCacheType:        c.FileCacheType,
		FileCacheTypeVersion: c.FileCacheTypeVersion,
		Lifecycle:            c.Lifecycle,
		ResourceARN:          c.ResourceARN,
		SubnetIDs:            c.SubnetIDs,
		StorageCapacityGiB:   c.StorageCapacityGiB,
		Tags:                 tagsMapToSlice(c.Tags),
	}
}

// createFileCacheLustreConfigurationInput mirrors
// CreateFileCacheLustreConfiguration (fsx@v1.68.4 types/types.go:574).
// DeploymentType, MetadataConfiguration, and PerUnitStorageThroughput are
// required members on the real SDK type.
type createFileCacheLustreConfigurationInput struct {
	MetadataConfiguration      *createFileCacheLustreMetadataInput `json:"MetadataConfiguration"`
	PerUnitStorageThroughput   *int32                              `json:"PerUnitStorageThroughput,omitempty"`
	DeploymentType             string                              `json:"DeploymentType,omitempty"`
	WeeklyMaintenanceStartTime string                              `json:"WeeklyMaintenanceStartTime,omitempty"`
}

// createFileCacheLustreMetadataInput mirrors
// FileCacheLustreMetadataConfiguration (fsx@v1.68.4 types/types.go:2550).
// StorageCapacity is a required member on the real SDK type.
type createFileCacheLustreMetadataInput struct {
	StorageCapacity *int32 `json:"StorageCapacity,omitempty"`
}

type createFileCacheInput struct {
	LustreConfiguration  *createFileCacheLustreConfigurationInput `json:"LustreConfiguration"`
	FileCacheType        string                                   `json:"FileCacheType"`
	FileCacheTypeVersion string                                   `json:"FileCacheTypeVersion"`
	Tags                 []Tag                                    `json:"Tags,omitempty"`
	SubnetIDs            []string                                 `json:"SubnetIds"`
	StorageCapacityGiB   int32                                    `json:"StorageCapacity,omitempty"`
}

// applyFileCacheLustreConfig validates and applies cfg onto c.
// LustreConfiguration is a required CreateFileCacheInput member in effect
// (FileCacheType is always LUSTRE, and real AWS's MissingFileCacheConfiguration
// exception -- "A cache configuration is required for this operation." --
// covers exactly this case), matching the CreateFileSystem
// per-type-config-block-required pattern (see applyWindowsConfig et al. in
// file_systems.go): an absent block returns MissingFileCacheConfiguration, a
// present-but-incomplete block returns BadRequest.
func applyFileCacheLustreConfig(c *storedFileCache, cfg *createFileCacheLustreConfigurationInput) error {
	if cfg == nil {
		return ErrMissingFileCacheConfiguration
	}

	if cfg.DeploymentType == "" {
		return fmt.Errorf("%w: LustreConfiguration.DeploymentType is required", ErrValidation)
	}

	if cfg.MetadataConfiguration == nil || cfg.MetadataConfiguration.StorageCapacity == nil {
		return fmt.Errorf(
			"%w: LustreConfiguration.MetadataConfiguration.StorageCapacity is required", ErrValidation,
		)
	}

	if cfg.PerUnitStorageThroughput == nil {
		return fmt.Errorf("%w: LustreConfiguration.PerUnitStorageThroughput is required", ErrValidation)
	}

	c.LustreDeploymentType = cfg.DeploymentType
	c.LustreWeeklyMaintenanceStartTime = cfg.WeeklyMaintenanceStartTime
	c.LustreMetadataStorageCapacity = *cfg.MetadataConfiguration.StorageCapacity
	c.LustrePerUnitStorageThroughput = *cfg.PerUnitStorageThroughput
	c.LustreMountName = generateLustreMountName()

	return nil
}

// CreateFileCache creates a file cache. FileCacheTypeVersion and SubnetIds
// are, along with FileCacheType/StorageCapacity, required
// CreateFileCacheInput members (verified against
// validateOpCreateFileCacheInput, validators.go) that the pre-fix request
// never read at all -- StorageCapacity was already wired.
func (b *InMemoryBackend) CreateFileCache(input *createFileCacheInput) (*FileCacheCreating, error) {
	if input.FileCacheType == "" {
		return nil, ErrValidation
	}

	if input.FileCacheTypeVersion == "" {
		return nil, fmt.Errorf("%w: FileCacheTypeVersion is required", ErrValidation)
	}

	if input.StorageCapacityGiB == 0 {
		return nil, fmt.Errorf("%w: StorageCapacity is required", ErrValidation)
	}

	if len(input.SubnetIDs) == 0 {
		return nil, fmt.Errorf("%w: SubnetIds is required", ErrValidation)
	}

	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateFileCache")
	defer b.mu.Unlock()

	id := newFileCacheID()
	arn := b.fcARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	c := &storedFileCache{
		CreationTime:         now,
		Tags:                 tags,
		FileCacheID:          id,
		FileCacheType:        input.FileCacheType,
		FileCacheTypeVersion: input.FileCacheTypeVersion,
		Lifecycle:            lifecycleAvailable,
		ResourceARN:          arn,
		SubnetIDs:            input.SubnetIDs,
		StorageCapacityGiB:   input.StorageCapacityGiB,
	}

	if err := applyFileCacheLustreConfig(c, input.LustreConfiguration); err != nil {
		return nil, err
	}

	b.fileCaches.Put(c)
	b.tags[arn] = tags

	return c.toPublicCreating(), nil
}

// DeleteFileCache removes a file cache.
func (b *InMemoryBackend) DeleteFileCache(fileCacheID string) error {
	b.mu.Lock("DeleteFileCache")
	defer b.mu.Unlock()

	c, ok := b.fileCaches.Get(fileCacheID)
	if !ok {
		return ErrFileCacheNotFound
	}

	b.fileCaches.Delete(fileCacheID)
	delete(b.tags, c.ResourceARN)

	return nil
}

// DescribeFileCaches returns file caches, optionally filtered by ID.
func (b *InMemoryBackend) DescribeFileCaches(
	ids []string,
	maxResults int32,
	nextToken string,
) ([]*FileCache, string, error) {
	b.mu.RLock("DescribeFileCaches")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedFileCache

	if len(ids) > 0 {
		for _, id := range ids {
			c, ok := b.fileCaches.Get(id)
			if !ok {
				return nil, "", ErrFileCacheNotFound
			}

			all = append(all, c)
		}
	} else {
		all = b.fileCaches.All()

		sort.Slice(all, func(i, j int) bool { return all[i].FileCacheID < all[j].FileCacheID })
	}

	start, end, next := paginate(len(all), int(maxResults), nextToken, func(i int) string {
		return all[i].FileCacheID
	})

	result := make([]*FileCache, end-start)
	for i, c := range all[start:end] {
		result[i] = c.toPublic()
	}

	return result, next, nil
}

type updateFileCacheInput struct {
	FileCacheID        string `json:"FileCacheId"`
	StorageCapacityGiB int32  `json:"StorageCapacityGiB,omitempty"`
}

// UpdateFileCache updates a file cache.
func (b *InMemoryBackend) UpdateFileCache(input *updateFileCacheInput) (*FileCache, error) {
	b.mu.Lock("UpdateFileCache")
	defer b.mu.Unlock()

	c, ok := b.fileCaches.Get(input.FileCacheID)
	if !ok {
		return nil, ErrFileCacheNotFound
	}

	if input.StorageCapacityGiB > 0 {
		c.StorageCapacityGiB = input.StorageCapacityGiB
	}

	return c.toPublic(), nil
}

func (b *InMemoryBackend) fcARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("file-cache/%s", id))
}
