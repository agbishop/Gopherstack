package workspaces

import (
	"cmp"
	"slices"
	"time"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/workspaces/types"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// imagePermissionsPageSize and imagesPageSize are this backend's default
// page sizes; real AWS doesn't document exact defaults for either operation,
// so these are chosen generously (larger than any realistic per-account
// image or permission-sharing list) so pagination only activates when a
// caller explicitly requests a smaller MaxResults.
const (
	imagePermissionsPageSize = 100
	imagesPageSize           = 100
)

// imageImportSpec carries the extra fields ImportWorkspaceImage/
// ImportCustomWorkspaceImage populate on top of createImageLocked's common
// name/description/source/tags -- zero-valued for every other image-creating
// op (CopyWorkspaceImage, CreateWorkspaceImage, CreateUpdatedWorkspaceImage),
// none of which accept them on the real wire.
type imageImportSpec struct {
	ImageSource                    *ImageSource
	ComputeType                    string
	InfrastructureConfigurationArn string
	OsVersion                      string
	Platform                       string
	Protocol                       string
	IngestionProcess               string
}

func (b *InMemoryBackend) createImageLocked(
	name, description, sourceImageID string,
	tags map[string]string,
	spec imageImportSpec,
) *storedImage {
	id := b.nextID("wsi-")
	stored := cloneTags(tags)
	img := &storedImage{
		ImageID:                        id,
		Name:                           name,
		Description:                    description,
		State:                          "AVAILABLE",
		SourceImageID:                  sourceImageID,
		Created:                        time.Now().UTC(),
		Tags:                           stored,
		ImageSource:                    spec.ImageSource,
		ComputeType:                    spec.ComputeType,
		InfrastructureConfigurationArn: spec.InfrastructureConfigurationArn,
		OsVersion:                      spec.OsVersion,
		Platform:                       spec.Platform,
		Protocol:                       spec.Protocol,
		IngestionProcess:               spec.IngestionProcess,
	}
	b.images.Put(img)
	b.tags[id] = stored

	return img
}

// CopyWorkspaceImage copies an image. SourceImageId is checked against
// b.images only when sourceRegion is empty or equals this backend's own
// region: this service instantiates one InMemoryBackend per (account,
// region) (see NewInMemoryBackend/provider.go), and storedImage carries no
// region field, so a genuine cross-region copy's source image legitimately
// lives in a different backend instance this one cannot see -- rejecting it
// would be more restrictive than real AWS. ResourceNotFoundException is in
// this operation's error list (aws-sdk-go-v2/service/workspaces@v1.73.1
// deserializers.go's awsAwsjson11_deserializeOpErrorCopyWorkspaceImage).
func (b *InMemoryBackend) CopyWorkspaceImage(
	name, sourceImageID, sourceRegion, description string,
	tags map[string]string,
) (string, error) {
	b.mu.Lock("CopyWorkspaceImage")
	defer b.mu.Unlock()

	if (sourceRegion == "" || sourceRegion == b.region) && !b.images.Has(sourceImageID) {
		return "", errImageNotFound
	}

	img := b.createImageLocked(name, description, sourceImageID, tags, imageImportSpec{})

	return img.ImageID, nil
}

// CreateWorkspaceImage creates an image from a workspace. Returns
// ErrWorkspaceNotFound for a WorkspaceId that doesn't reference a real
// workspace, matching real AWS (ResourceNotFoundException is in this
// operation's error list; see deserializers.go's
// awsAwsjson11_deserializeOpErrorCreateWorkspaceImage). The real
// CreateWorkspaceImageOutput and WorkspaceImage type carry no source
// workspace reference, so there is nothing to derive from the workspace
// beyond confirming it exists.
func (b *InMemoryBackend) CreateWorkspaceImage(
	name, description, workspaceID string,
	tags map[string]string,
) (*storedImage, error) {
	b.mu.Lock("CreateWorkspaceImage")
	defer b.mu.Unlock()

	if !b.workspaces.Has(workspaceID) {
		return nil, ErrWorkspaceNotFound
	}

	img := b.createImageLocked(name, description, "", tags, imageImportSpec{})

	return img, nil
}

// DeleteWorkspaceImage removes an image. Returns errImageInUse when a bundle
// still references the image: "To delete an image, you must first delete any
// bundles that are associated with the image" (api_op_DeleteWorkspaceImage.go
// doc comment).
func (b *InMemoryBackend) DeleteWorkspaceImage(imageID string) error {
	b.mu.Lock("DeleteWorkspaceImage")
	defer b.mu.Unlock()

	if !b.images.Has(imageID) {
		return errImageNotFound
	}

	for _, bun := range b.customBundles.All() {
		if bun.ImageID == imageID {
			return errImageInUse
		}
	}

	b.images.Delete(imageID)
	delete(b.imagePermissions, imageID)

	return nil
}

// isValidIngestionProcess reports whether process is one of the pinned
// SDK's known WorkspaceImageIngestionProcess values (workspaces@v1.73.1
// types/enums.go:1550-1562).
func isValidIngestionProcess(process string) bool {
	return slices.Contains(
		sdktypes.WorkspaceImageIngestionProcess("").Values(),
		sdktypes.WorkspaceImageIngestionProcess(process),
	)
}

// ImportWorkspaceImage imports an EC2 image as a workspace image.
// IngestionProcess is required on the real ImportWorkspaceImageInput
// (workspaces@v1.73.1 api_op_ImportWorkspaceImage.go:67).
func (b *InMemoryBackend) ImportWorkspaceImage(
	ec2ImageID, name, description, ingestionProcess string, tags map[string]string,
) (string, error) {
	b.mu.Lock("ImportWorkspaceImage")
	defer b.mu.Unlock()

	if ingestionProcess == "" {
		return "", awserr.New("IngestionProcess is required", awserr.ErrInvalidParameter)
	}

	if !isValidIngestionProcess(ingestionProcess) {
		return "", awserr.Newf("invalid IngestionProcess: %q", awserr.ErrInvalidParameter, ingestionProcess)
	}

	img := b.createImageLocked(name, description, ec2ImageID, tags, imageImportSpec{
		IngestionProcess: ingestionProcess,
	})

	return img.ImageID, nil
}

// customWorkspaceImageImportSpec carries ImportCustomWorkspaceImage's
// required members beyond name/description (workspaces@v1.73.1
// api_op_ImportCustomWorkspaceImage.go:33-75: ComputeType, ImageSource,
// InfrastructureConfigurationArn, OsVersion, Platform and Protocol are all
// "This member is required").
type customWorkspaceImageImportSpec struct {
	ImageSource                    *ImageSource
	ComputeType                    string
	InfrastructureConfigurationArn string
	OsVersion                      string
	Platform                       string
	Protocol                       string
}

// ImportCustomWorkspaceImage imports a custom workspace image.
func (b *InMemoryBackend) ImportCustomWorkspaceImage(
	name, description string, spec customWorkspaceImageImportSpec,
) (*storedImage, error) {
	b.mu.Lock("ImportCustomWorkspaceImage")
	defer b.mu.Unlock()

	if spec.ComputeType == "" {
		return nil, awserr.New("ComputeType is required", awserr.ErrInvalidParameter)
	}

	if !slices.Contains(sdktypes.ImageComputeType("").Values(), sdktypes.ImageComputeType(spec.ComputeType)) {
		return nil, awserr.Newf("invalid ComputeType: %q", awserr.ErrInvalidParameter, spec.ComputeType)
	}

	if spec.ImageSource == nil {
		return nil, awserr.New("ImageSource is required", awserr.ErrInvalidParameter)
	}

	if spec.InfrastructureConfigurationArn == "" {
		return nil, awserr.New("InfrastructureConfigurationArn is required", awserr.ErrInvalidParameter)
	}

	if spec.OsVersion == "" {
		return nil, awserr.New("OsVersion is required", awserr.ErrInvalidParameter)
	}

	if !slices.Contains(sdktypes.OSVersion("").Values(), sdktypes.OSVersion(spec.OsVersion)) {
		return nil, awserr.Newf("invalid OsVersion: %q", awserr.ErrInvalidParameter, spec.OsVersion)
	}

	if spec.Platform == "" {
		return nil, awserr.New("Platform is required", awserr.ErrInvalidParameter)
	}

	if !slices.Contains(sdktypes.Platform("").Values(), sdktypes.Platform(spec.Platform)) {
		return nil, awserr.Newf("invalid Platform: %q", awserr.ErrInvalidParameter, spec.Platform)
	}

	if spec.Protocol == "" {
		return nil, awserr.New("Protocol is required", awserr.ErrInvalidParameter)
	}

	if !slices.Contains(sdktypes.CustomImageProtocol("").Values(), sdktypes.CustomImageProtocol(spec.Protocol)) {
		return nil, awserr.Newf("invalid Protocol: %q", awserr.ErrInvalidParameter, spec.Protocol)
	}

	img := b.createImageLocked(name, description, "", nil, imageImportSpec{
		ImageSource:                    spec.ImageSource,
		ComputeType:                    spec.ComputeType,
		InfrastructureConfigurationArn: spec.InfrastructureConfigurationArn,
		OsVersion:                      spec.OsVersion,
		Platform:                       spec.Platform,
		Protocol:                       spec.Protocol,
	})

	return img, nil
}

// CreateUpdatedWorkspaceImage creates an updated version of an existing
// image. Returns errImageNotFound for a SourceImageId that doesn't
// reference a real image, matching real AWS (ResourceNotFoundException is
// in this operation's error list; see deserializers.go's
// awsAwsjson11_deserializeOpErrorCreateUpdatedWorkspaceImage).
func (b *InMemoryBackend) CreateUpdatedWorkspaceImage(
	sourceImageID, name, description string, tags map[string]string,
) (string, error) {
	b.mu.Lock("CreateUpdatedWorkspaceImage")
	defer b.mu.Unlock()

	if !b.images.Has(sourceImageID) {
		return "", errImageNotFound
	}

	img := b.createImageLocked(name, description, sourceImageID, tags, imageImportSpec{})

	return img.ImageID, nil
}

// DescribeWorkspaceImages returns workspace images, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeWorkspaceImages(
	imageIDs []string, _ /*imageType*/ string, maxResults int32, nextToken string,
) ([]*storedImage, string, error) {
	b.mu.RLock("DescribeWorkspaceImages")
	defer b.mu.RUnlock()

	filter := buildFilter(imageIDs)
	all := b.images.All()

	slices.SortFunc(all, func(a, b *storedImage) int {
		return cmp.Compare(a.ImageID, b.ImageID)
	})

	result := make([]*storedImage, 0, len(all))

	for _, img := range all {
		if !matchesFilter(filter, img.ImageID) {
			continue
		}

		cp := *img
		result = append(result, &cp)
	}

	pg := page.New(result, nextToken, int(maxResults), imagesPageSize)

	return pg.Data, pg.Next, nil
}

// ImagePermission is one shared-account entry from
// DescribeWorkspaceImagePermissions.
type ImagePermission struct {
	SharedAccountID string
	AllowCopyImage  bool
}

// DescribeWorkspaceImagePermissions returns a page of sharing permissions for
// an image, sorted by account ID for a stable pagination order.
func (b *InMemoryBackend) DescribeWorkspaceImagePermissions(
	imageID, token string, limit int,
) (string, page.Page[ImagePermission], error) {
	b.mu.RLock("DescribeWorkspaceImagePermissions")
	defer b.mu.RUnlock()

	if !b.images.Has(imageID) {
		return "", page.Page[ImagePermission]{}, errImageNotFound
	}

	perms := b.imagePermissions[imageID]
	accountIDs := make([]string, 0, len(perms))

	for accountID := range perms {
		accountIDs = append(accountIDs, accountID)
	}

	slices.Sort(accountIDs)

	all := make([]ImagePermission, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		all = append(all, ImagePermission{SharedAccountID: accountID, AllowCopyImage: perms[accountID]})
	}

	return imageID, page.New(all, token, limit, imagePermissionsPageSize), nil
}

// UpdateWorkspaceImagePermission sets the sharing permission for an image.
func (b *InMemoryBackend) UpdateWorkspaceImagePermission(
	imageID, sharedAccountID string, allowCopy bool,
) error {
	b.mu.Lock("UpdateWorkspaceImagePermission")
	defer b.mu.Unlock()

	if !b.images.Has(imageID) {
		return errImageNotFound
	}

	if b.imagePermissions[imageID] == nil {
		b.imagePermissions[imageID] = make(map[string]bool)
	}

	b.imagePermissions[imageID][sharedAccountID] = allowCopy

	return nil
}

// DescribeCustomWorkspaceImageImport returns import state for a custom image.
func (b *InMemoryBackend) DescribeCustomWorkspaceImageImport(imageID string) (*storedImage, error) {
	b.mu.RLock("DescribeCustomWorkspaceImageImport")
	defer b.mu.RUnlock()

	img, ok := b.images.Get(imageID)
	if !ok {
		return nil, errImageNotFound
	}

	cp := *img

	return &cp, nil
}

// DescribeImageAssociations returns application associations for an image.
// Real AWS's WorkSpaces Application Manager exposes no public API to create
// an image<->application association (only AssociateWorkspaceApplication,
// which associates an application directly with a WorkSpace, and
// DeployWorkspaceApplications, neither of which touch an image or bundle) --
// so a freshly emulated account always has an empty association list. This
// still performs the real required-field and existence validation a live
// call would enforce, matching the pattern used by RestoreWorkspace for an
// otherwise-no-op operation.
func (b *InMemoryBackend) DescribeImageAssociations(
	imageID string, resourceTypes []string,
) ([]ImageResourceAssociation, error) {
	b.mu.RLock("DescribeImageAssociations")
	defer b.mu.RUnlock()

	if imageID == "" {
		return nil, awserr.New("ImageId is required", awserr.ErrInvalidParameter)
	}

	if !b.images.Has(imageID) {
		return nil, errImageNotFound
	}

	if err := validateAssociatedResourceTypes(resourceTypes); err != nil {
		return nil, err
	}

	return []ImageResourceAssociation{}, nil
}
