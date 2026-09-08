package appstream

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

const (
	imageStateAvailable  = "AVAILABLE"
	imagePlatformWindows = "WINDOWS_SERVER_2019"

	imageBuilderStateStopped = "STOPPED"
	imageBuilderStateRunning = "RUNNING"

	// exportImageTaskStateCompleted is the real AWS ExportImageTaskState enum
	// value for a finished export (EXPORTING/COMPLETED/FAILED/TIMED_OUT); this
	// in-memory backend completes export tasks synchronously.
	exportImageTaskStateCompleted = "COMPLETED"

	// defaultListExportImageTasksLimit matches real AWS's documented default
	// page size (50) when the caller omits MaxResults.
	defaultListExportImageTasksLimit = 50

	// defaultBuilderStreamingURLValiditySeconds matches real AWS's default for
	// CreateImageBuilderStreamingURL/CreateAppBlockBuilderStreamingURL (3600
	// seconds) when the caller omits Validity.
	defaultBuilderStreamingURLValiditySeconds = 3600
)

type storedImage struct {
	CreatedTime  time.Time         `json:"createdTime"`
	Tags         map[string]string `json:"tags"`
	Name         string            `json:"name"`
	Arn          string            `json:"arn"`
	Description  string            `json:"description"`
	Platform     string            `json:"platform"`
	Visibility   string            `json:"visibility"`
	State        string            `json:"state"`
	BaseImageArn string            `json:"baseImageArn"`
}

func (i *storedImage) toImage() *Image {
	tags := make(map[string]string)
	maps.Copy(tags, i.Tags)

	return &Image{
		CreatedTime:  i.CreatedTime,
		Tags:         tags,
		Name:         i.Name,
		Arn:          i.Arn,
		Description:  i.Description,
		Platform:     i.Platform,
		Visibility:   i.Visibility,
		State:        i.State,
		BaseImageArn: i.BaseImageArn,
	}
}

// storedImagePermissions previously had no identity field of its own -- it
// was always looked up by an external imageName key on the plain map it
// lived in. Converting that map to a *store.Table[storedImagePermissions]
// (see store_setup.go) requires a keyFn, so it gained a real ImageName field
// for that purpose. This type is purely internal storage (DescribeImagePermissions
// builds its own SharedImagePermissions response type rather than marshaling
// this one), so a plain visible json tag is fine -- unlike a wire-shape type,
// there is no AWS response shape this could leak into.
type storedImagePermissions struct {
	SharedAccounts map[string]*ImagePermissions `json:"sharedAccounts"`
	ImageName      string                       `json:"imageName"`
}

type storedImageBuilder struct {
	CreatedTime  time.Time         `json:"createdTime"`
	Tags         map[string]string `json:"tags"`
	Name         string            `json:"name"`
	Arn          string            `json:"arn"`
	Description  string            `json:"description"`
	Platform     string            `json:"platform"`
	InstanceType string            `json:"instanceType"`
	State        string            `json:"state"`
	ImageName    string            `json:"imageName"`
}

func (ib *storedImageBuilder) toImageBuilder() *ImageBuilder {
	tags := make(map[string]string)
	maps.Copy(tags, ib.Tags)

	return &ImageBuilder{
		CreatedTime:  ib.CreatedTime,
		Tags:         tags,
		Name:         ib.Name,
		Arn:          ib.Arn,
		Description:  ib.Description,
		Platform:     ib.Platform,
		InstanceType: ib.InstanceType,
		State:        ib.State,
		ImageName:    ib.ImageName,
	}
}

type storedExportImageTask struct {
	CreatedDate       time.Time         `json:"createdDate"`
	TagSpecifications map[string]string `json:"tagSpecifications"`
	TaskID            string            `json:"taskId"`
	ImageArn          string            `json:"imageArn"`
	AmiName           string            `json:"amiName"`
	AmiDescription    string            `json:"amiDescription"`
	AmiID             string            `json:"amiId"`
	IamRoleArn        string            `json:"iamRoleArn"`
	State             string            `json:"state"`
}

func (t *storedExportImageTask) toExportImageTask() *ExportImageTask {
	tags := make(map[string]string)
	maps.Copy(tags, t.TagSpecifications)

	return &ExportImageTask{
		CreatedDate:       t.CreatedDate,
		TagSpecifications: tags,
		TaskID:            t.TaskID,
		ImageArn:          t.ImageArn,
		AmiName:           t.AmiName,
		AmiDescription:    t.AmiDescription,
		AmiID:             t.AmiID,
		State:             t.State,
	}
}

func (b *InMemoryBackend) imageARN(name string) string {
	return arn.Build("appstream", b.region, b.accountID, fmt.Sprintf("image/%s", name))
}

func (b *InMemoryBackend) imageBuilderARN(name string) string {
	return arn.Build("appstream", b.region, b.accountID, fmt.Sprintf("image-builder/%s", name))
}

// CopyImage duplicates an image with a new name.
func (b *InMemoryBackend) CopyImage(
	sourceName, destName, destRegion, description string, //nolint:revive // existing issue.
) (*Image, error) {
	b.mu.Lock("CopyImage")
	defer b.mu.Unlock()

	src, ok := b.images.Get(sourceName)
	if !ok {
		return nil, ErrNotFound
	}

	if b.images.Has(destName) {
		return nil, ErrAlreadyExists
	}

	arn := b.imageARN(destName)
	desc := description
	if desc == "" {
		desc = src.Description
	}

	img := &storedImage{
		CreatedTime:  time.Now().UTC(),
		Tags:         make(map[string]string),
		Name:         destName,
		Arn:          arn,
		Description:  desc,
		Platform:     src.Platform,
		Visibility:   "PRIVATE", //nolint:goconst // existing issue.
		State:        imageStateAvailable,
		BaseImageArn: src.Arn,
	}
	b.images.Put(img)
	b.tags[arn] = make(map[string]string)

	return img.toImage(), nil
}

// CreateImportedImage creates a new image (e.g. imported from S3).
func (b *InMemoryBackend) CreateImportedImage(name, description string, tags map[string]string) (*Image, error) {
	b.mu.Lock("CreateImportedImage")
	defer b.mu.Unlock()

	if b.images.Has(name) {
		return nil, ErrAlreadyExists
	}

	arn := b.imageARN(name)
	storedTags := make(map[string]string)
	maps.Copy(storedTags, tags)

	img := &storedImage{
		CreatedTime: time.Now().UTC(),
		Tags:        storedTags,
		Name:        name,
		Arn:         arn,
		Description: description,
		Platform:    imagePlatformWindows,
		Visibility:  "PRIVATE",
		State:       imageStateAvailable,
	}
	b.images.Put(img)
	b.tags[arn] = storedTags

	return img.toImage(), nil
}

// CreateUpdatedImage creates a new image based on an existing one with updates applied.
func (b *InMemoryBackend) CreateUpdatedImage(imageName, newImageName, description string) (*Image, error) {
	b.mu.Lock("CreateUpdatedImage")
	defer b.mu.Unlock()

	src, ok := b.images.Get(imageName)
	if !ok {
		return nil, ErrNotFound
	}

	if b.images.Has(newImageName) {
		return nil, ErrAlreadyExists
	}

	arn := b.imageARN(newImageName)
	desc := description
	if desc == "" {
		desc = src.Description
	}

	img := &storedImage{
		CreatedTime:  time.Now().UTC(),
		Tags:         make(map[string]string),
		Name:         newImageName,
		Arn:          arn,
		Description:  desc,
		Platform:     src.Platform,
		Visibility:   "PRIVATE",
		State:        imageStateAvailable,
		BaseImageArn: src.Arn,
	}
	b.images.Put(img)
	b.tags[arn] = make(map[string]string)

	return img.toImage(), nil
}

// DeleteImage removes an image and returns the deleted image. Real AWS's
// DeleteImageOutput carries the deleted Image
// (deserializeCBOR_DeleteImageOutput in the pinned appstream SDK's
// deserializers.go), not an empty envelope.
//
// "You cannot delete an image when it is in use" (api_op_DeleteImage.go doc
// comment); a fleet still referencing this image by name or ARN counts as
// in use, matching DeleteImage's modeled ResourceInUseException.
func (b *InMemoryBackend) DeleteImage(name string) (*Image, error) {
	b.mu.Lock("DeleteImage")
	defer b.mu.Unlock()

	img, ok := b.images.Get(name)
	if !ok {
		return nil, ErrNotFound
	}

	for _, f := range b.fleets.All() {
		if f.ImageName == img.Name || f.ImageArn == img.Arn {
			return nil, ErrResourceInUse
		}
	}

	deleted := img.toImage()

	delete(b.tags, img.Arn)
	b.images.Delete(name)
	b.imagePermissions.Delete(name)

	return deleted, nil
}

// findImage resolves id against Name (the primary key used by
// CreateImportedImage/CopyImage/DeleteImage) or Arn. Real AWS's
// DescribeImages accepts either an image Names filter or an Arns filter, so
// callers on that wire path must resolve through this helper rather than
// indexing b.images directly with the caller-supplied identifier.
func (b *InMemoryBackend) findImage(id string) (*storedImage, bool) {
	if img, ok := b.images.Get(id); ok {
		return img, true
	}

	for _, img := range b.images.All() {
		if img.Arn == id {
			return img, true
		}
	}

	return nil, false
}

// DescribeImages returns images, optionally filtered by name or ARN.
// DescribeImages returns images, optionally filtered by name/ARN and by
// visibility type. Every image this backend creates has Visibility
// "PRIVATE" -- it never models AWS-provided base images or images shared
// from another account -- so visibilityType "PUBLIC" or "SHARED" always
// yields an empty result.
func (b *InMemoryBackend) DescribeImages(names []string, visibilityType string) ([]*Image, error) {
	b.mu.RLock("DescribeImages")
	defer b.mu.RUnlock()

	if len(names) > 0 {
		var result []*Image

		for _, name := range names {
			img, ok := b.findImage(name)
			if !ok {
				return nil, ErrNotFound
			}

			if visibilityType != "" && img.Visibility != visibilityType {
				continue
			}

			result = append(result, img.toImage())
		}

		return result, nil
	}

	result := make([]*Image, 0, b.images.Len())
	for _, img := range b.images.All() {
		if visibilityType != "" && img.Visibility != visibilityType {
			continue
		}

		result = append(result, img.toImage())
	}

	return result, nil
}

// UpdateImagePermissions sets sharing permissions for a specific account.
func (b *InMemoryBackend) UpdateImagePermissions(
	imageName, accountID string,
	allowFleet, allowImageBuilder bool,
) error {
	b.mu.Lock("UpdateImagePermissions")
	defer b.mu.Unlock()

	if !b.images.Has(imageName) {
		return ErrNotFound
	}

	perms, ok := b.imagePermissions.Get(imageName)
	if !ok {
		perms = &storedImagePermissions{
			ImageName:      imageName,
			SharedAccounts: make(map[string]*ImagePermissions),
		}
		b.imagePermissions.Put(perms)
	}

	perms.SharedAccounts[accountID] = &ImagePermissions{
		AllowFleet:        allowFleet,
		AllowImageBuilder: allowImageBuilder,
	}

	return nil
}

// DeleteImagePermissions removes sharing permissions for a specific account.
func (b *InMemoryBackend) DeleteImagePermissions(imageName, accountID string) error {
	b.mu.Lock("DeleteImagePermissions")
	defer b.mu.Unlock()

	if !b.images.Has(imageName) {
		return ErrNotFound
	}

	if perms, ok := b.imagePermissions.Get(imageName); ok {
		delete(perms.SharedAccounts, accountID)
	}

	return nil
}

// DescribeImagePermissions returns sharing permissions for an image.
func (b *InMemoryBackend) DescribeImagePermissions(
	imageName string, sharedAwsAccountIDs []string,
) ([]*SharedImagePermissions, error) {
	b.mu.RLock("DescribeImagePermissions")
	defer b.mu.RUnlock()

	if !b.images.Has(imageName) {
		return nil, ErrNotFound
	}

	perms, ok := b.imagePermissions.Get(imageName)
	if !ok {
		return []*SharedImagePermissions{}, nil
	}

	allowed := make(map[string]bool, len(sharedAwsAccountIDs))
	for _, id := range sharedAwsAccountIDs {
		allowed[id] = true
	}

	result := make([]*SharedImagePermissions, 0, len(perms.SharedAccounts))
	for accID, p := range perms.SharedAccounts {
		if len(allowed) > 0 && !allowed[accID] {
			continue
		}

		pCopy := *p
		result = append(result, &SharedImagePermissions{
			SharedAccountID:  accID,
			ImagePermissions: &pCopy,
		})
	}

	return result, nil
}

// CreateImageBuilder creates a new image builder.
func (b *InMemoryBackend) CreateImageBuilder(
	name, description, platform, instanceType string,
	tags map[string]string,
) (*ImageBuilder, error) {
	if instanceType == "" {
		return nil, fmt.Errorf("%w: InstanceType is required", awserr.ErrInvalidParameter)
	}

	b.mu.Lock("CreateImageBuilder")
	defer b.mu.Unlock()

	if b.imageBuilders.Has(name) {
		return nil, ErrAlreadyExists
	}

	arn := b.imageBuilderARN(name)
	storedTags := make(map[string]string)
	maps.Copy(storedTags, tags)

	plat := platform
	if plat == "" {
		plat = imagePlatformWindows
	}

	ib := &storedImageBuilder{
		CreatedTime:  time.Now().UTC(),
		Tags:         storedTags,
		Name:         name,
		Arn:          arn,
		Description:  description,
		Platform:     plat,
		InstanceType: instanceType,
		State:        imageBuilderStateStopped,
	}
	b.imageBuilders.Put(ib)
	b.tags[arn] = storedTags

	return ib.toImageBuilder(), nil
}

// DeleteImageBuilder removes an image builder and returns the deleted image
// builder. Real AWS's DeleteImageBuilderOutput carries the deleted
// ImageBuilder (deserializeCBOR_DeleteImageBuilderOutput in the pinned
// appstream SDK's deserializers.go), not an empty envelope or a stripped-down
// Name/ImageName-only shape.
func (b *InMemoryBackend) DeleteImageBuilder(name string) (*ImageBuilder, error) {
	b.mu.Lock("DeleteImageBuilder")
	defer b.mu.Unlock()

	ib, ok := b.imageBuilders.Get(name)
	if !ok {
		return nil, ErrNotFound
	}

	deleted := ib.toImageBuilder()

	delete(b.tags, ib.Arn)
	b.imageBuilders.Delete(name)
	delete(b.softwareAssoc, name)

	return deleted, nil
}

// DescribeImageBuilders returns image builders, optionally filtered by name.
func (b *InMemoryBackend) DescribeImageBuilders(names []string) ([]*ImageBuilder, error) {
	b.mu.RLock("DescribeImageBuilders")
	defer b.mu.RUnlock()

	if len(names) > 0 {
		var result []*ImageBuilder

		for _, name := range names {
			ib, ok := b.imageBuilders.Get(name)
			if !ok {
				return nil, ErrNotFound
			}

			result = append(result, ib.toImageBuilder())
		}

		return result, nil
	}

	result := make([]*ImageBuilder, 0, b.imageBuilders.Len())
	for _, ib := range b.imageBuilders.All() {
		result = append(result, ib.toImageBuilder())
	}

	return result, nil
}

// StartImageBuilder transitions an image builder to RUNNING. Real AWS's
// StartImageBuilderOutput carries only the ImageBuilder shape -- no streaming
// URL; callers must fetch one separately via CreateImageBuilderStreamingURL.
func (b *InMemoryBackend) StartImageBuilder(
	name, appstreamAgentVersion string, //nolint:revive // existing issue.
) error {
	b.mu.Lock("StartImageBuilder")
	defer b.mu.Unlock()

	ib, ok := b.imageBuilders.Get(name)
	if !ok {
		return ErrNotFound
	}

	if ib.State == imageBuilderStateRunning {
		return ErrFleetNotStopped
	}

	ib.State = imageBuilderStateRunning

	return nil
}

// StopImageBuilder transitions an image builder to STOPPED. Idempotent:
// stopping an already-stopped builder succeeds (real AWS's StopImageBuilder
// has no state-conflict exception -- only ConcurrentModificationException,
// OperationNotPermittedException, and ResourceNotFoundException).
func (b *InMemoryBackend) StopImageBuilder(name string) (*ImageBuilder, error) {
	b.mu.Lock("StopImageBuilder")
	defer b.mu.Unlock()

	ib, ok := b.imageBuilders.Get(name)
	if !ok {
		return nil, ErrNotFound
	}

	ib.State = imageBuilderStateStopped

	return ib.toImageBuilder(), nil
}

// CreateImageBuilderStreamingURL returns a streaming URL for an image builder
// along with its expiry time. validitySeconds <= 0 falls back to the real AWS
// default of 3600 seconds.
func (b *InMemoryBackend) CreateImageBuilderStreamingURL(
	name string,
	validitySeconds int64,
) (string, time.Time, error) {
	b.mu.RLock("CreateImageBuilderStreamingURL")
	defer b.mu.RUnlock()

	if !b.imageBuilders.Has(name) {
		return "", time.Time{}, ErrNotFound
	}

	validity := validitySeconds
	if validity <= 0 {
		validity = defaultBuilderStreamingURLValiditySeconds
	}

	expires := time.Now().UTC().Add(time.Duration(validity) * time.Second)

	url := fmt.Sprintf(
		"https://appstream2.%s.aws.amazon.com/authenticate?param=imagebuilder-url-%s", b.region, name,
	)

	return url, expires, nil
}

// AssociateSoftwareToImageBuilder adds software packages to an image builder.
func (b *InMemoryBackend) AssociateSoftwareToImageBuilder(imageBuilderName string, software []string) error {
	b.mu.Lock("AssociateSoftwareToImageBuilder")
	defer b.mu.Unlock()

	if !b.imageBuilders.Has(imageBuilderName) {
		return ErrNotFound
	}

	if b.softwareAssoc[imageBuilderName] == nil {
		b.softwareAssoc[imageBuilderName] = make(map[string]bool)
	}

	for _, sw := range software {
		b.softwareAssoc[imageBuilderName][sw] = true
	}

	return nil
}

// DisassociateSoftwareFromImageBuilder removes software packages from an image builder.
func (b *InMemoryBackend) DisassociateSoftwareFromImageBuilder(imageBuilderName string, software []string) error {
	b.mu.Lock("DisassociateSoftwareFromImageBuilder")
	defer b.mu.Unlock()

	if !b.imageBuilders.Has(imageBuilderName) {
		return ErrNotFound
	}

	for _, sw := range software {
		if b.softwareAssoc[imageBuilderName] != nil {
			delete(b.softwareAssoc[imageBuilderName], sw)
		}
	}

	return nil
}

// DescribeSoftwareAssociations returns software associated with an image builder or image.
func (b *InMemoryBackend) DescribeSoftwareAssociations(resource string) ([]SoftwareAssociation, error) {
	b.mu.RLock("DescribeSoftwareAssociations")
	defer b.mu.RUnlock()

	if !b.imageBuilders.Has(resource) && !b.images.Has(resource) {
		return nil, ErrNotFound
	}

	sw := b.softwareAssoc[resource]
	result := make([]SoftwareAssociation, 0, len(sw))

	for name := range sw {
		result = append(result, SoftwareAssociation{
			ImageBuilderName: resource,
			Software:         name,
		})
	}

	return result, nil
}

// StartSoftwareDeploymentToImageBuilder triggers a deployment (no-op for in-memory).
func (b *InMemoryBackend) StartSoftwareDeploymentToImageBuilder(imageBuilderName string) error {
	b.mu.RLock("StartSoftwareDeploymentToImageBuilder")
	defer b.mu.RUnlock()

	if !b.imageBuilders.Has(imageBuilderName) {
		return ErrNotFound
	}

	return nil
}

func (b *InMemoryBackend) nextExportTaskID() string {
	b.exportTaskSeq++

	return fmt.Sprintf("export-task-%05d", b.exportTaskSeq)
}

// CreateExportImageTask creates a task exporting imageName to an EC2 AMI.
// Real AWS's CreateExportImageTaskInput requires ImageName, AmiName, and
// IamRoleArn (no S3 destination -- this is an AMI export, not an S3 export);
// TagSpecifications and AmiDescription are optional. This in-memory backend
// completes the export synchronously, minting a deterministic AMI ID.
func (b *InMemoryBackend) CreateExportImageTask(
	imageName, amiName, amiDescription, iamRoleArn string,
	tagSpecifications map[string]string,
) (*ExportImageTask, error) {
	b.mu.Lock("CreateExportImageTask")
	defer b.mu.Unlock()

	img, ok := b.images.Get(imageName)
	if !ok {
		return nil, ErrNotFound
	}

	taskID := b.nextExportTaskID()
	amiID := fmt.Sprintf("ami-%017d", b.exportTaskSeq)

	tags := make(map[string]string)
	maps.Copy(tags, tagSpecifications)

	task := &storedExportImageTask{
		CreatedDate:       time.Now().UTC(),
		TagSpecifications: tags,
		TaskID:            taskID,
		ImageArn:          img.Arn,
		AmiName:           amiName,
		AmiDescription:    amiDescription,
		AmiID:             amiID,
		IamRoleArn:        iamRoleArn,
		State:             exportImageTaskStateCompleted,
	}
	b.exportTasks.Put(task)

	return task.toExportImageTask(), nil
}

// GetExportImageTask retrieves an export task by ID.
func (b *InMemoryBackend) GetExportImageTask(taskID string) (*ExportImageTask, error) {
	b.mu.RLock("GetExportImageTask")
	defer b.mu.RUnlock()

	task, ok := b.exportTasks.Get(taskID)
	if !ok {
		return nil, ErrNotFound
	}

	return task.toExportImageTask(), nil
}

// ListExportImageTasks returns a page of export tasks ordered by TaskID.
// Real AWS also accepts a generic Filters parameter (opaque Name/Values
// pairs whose matching semantics are not part of the published service
// model); this emulator does not evaluate it, only MaxResults/NextToken
// pagination.
func (b *InMemoryBackend) ListExportImageTasks(maxResults int32, nextToken string) ([]*ExportImageTask, string, error) {
	b.mu.RLock("ListExportImageTasks")
	defer b.mu.RUnlock()

	all := make([]*ExportImageTask, 0, b.exportTasks.Len())
	for _, task := range b.exportTasks.All() {
		all = append(all, task.toExportImageTask())
	}

	sort.Slice(all, func(i, j int) bool { return all[i].TaskID < all[j].TaskID })

	p := page.New(all, nextToken, int(maxResults), defaultListExportImageTasksLimit)

	return p.Data, p.Next, nil
}
