package sagemaker

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

const statusCreating = "Creating"

// ---------------------------------------------------------------------------
// InferenceComponent
// ---------------------------------------------------------------------------

var (
	// ErrInferenceComponentNotFound is returned when an inference component does not exist.
	ErrInferenceComponentNotFound = awserr.New("ValidationException", awserr.ErrNotFound)
	// ErrInferenceComponentAlreadyExists is returned when an inference component already exists.
	ErrInferenceComponentAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
)

// InferenceComponentContainerSpec mirrors the request-side
// InferenceComponentContainerSpecification (types/types.go:11731-11756). Image
// is echoed back as DeployedImage.SpecifiedImage on Describe/List — this
// backend never resolves a registry digest, so DeployedImage.ResolvedImage/
// ResolutionTime are disclosed no-ops.
type InferenceComponentContainerSpec struct {
	Environment            map[string]string `json:"Environment,omitempty"`
	ArtifactURL            string            `json:"ArtifactUrl,omitempty"`
	Image                  string            `json:"Image,omitempty"`
	ContainerMetricsConfig json.RawMessage   `json:"ContainerMetricsConfig,omitempty"`
}

// InferenceComponentSpecification mirrors types.InferenceComponentSpecification
// (types/types.go:11947-11996). ComputeResourceRequirements/DataCacheConfig/
// SchedulingConfig/StartupParameters are carried as opaque json.RawMessage:
// each of those four sub-types is used unchanged by both this request type
// and its response-side Summary counterpart (types/types.go:12002-12034), so
// a byte-for-byte passthrough between Create/Update and Describe/List is
// wire-correct without needing semantic modeling. Only Container needs real
// translation, since the request's Image field becomes the response's
// DeployedImage.
type InferenceComponentSpecification struct {
	Container                   *InferenceComponentContainerSpec `json:"Container,omitempty"`
	BaseInferenceComponentName  string                           `json:"BaseInferenceComponentName,omitempty"`
	InstanceType                string                           `json:"InstanceType,omitempty"`
	ModelName                   string                           `json:"ModelName,omitempty"`
	ComputeResourceRequirements json.RawMessage                  `json:"ComputeResourceRequirements,omitempty"`
	DataCacheConfig             json.RawMessage                  `json:"DataCacheConfig,omitempty"`
	SchedulingConfig            json.RawMessage                  `json:"SchedulingConfig,omitempty"`
	StartupParameters           json.RawMessage                  `json:"StartupParameters,omitempty"`
}

// inferenceComponentSpecificationSummary builds the InferenceComponentSpecificationSummary
// wire shape (types/types.go:12002-12034) from a stored InferenceComponentSpecification.
func inferenceComponentSpecificationSummary(s *InferenceComponentSpecification) map[string]any {
	summary := map[string]any{}

	if s.BaseInferenceComponentName != "" {
		summary["BaseInferenceComponentName"] = s.BaseInferenceComponentName
	}

	if s.ComputeResourceRequirements != nil {
		summary["ComputeResourceRequirements"] = s.ComputeResourceRequirements
	}

	if s.Container != nil {
		container := map[string]any{}

		if s.Container.ArtifactURL != "" {
			container["ArtifactUrl"] = s.Container.ArtifactURL
		}

		if s.Container.ContainerMetricsConfig != nil {
			container["ContainerMetricsConfig"] = s.Container.ContainerMetricsConfig
		}

		if len(s.Container.Environment) > 0 {
			container["Environment"] = s.Container.Environment
		}

		if s.Container.Image != "" {
			container["DeployedImage"] = map[string]any{"SpecifiedImage": s.Container.Image}
		}

		summary["Container"] = container
	}

	if s.DataCacheConfig != nil {
		summary["DataCacheConfig"] = s.DataCacheConfig
	}

	if s.InstanceType != "" {
		summary["InstanceType"] = s.InstanceType
	}

	if s.ModelName != "" {
		summary["ModelName"] = s.ModelName
	}

	if s.SchedulingConfig != nil {
		summary["SchedulingConfig"] = s.SchedulingConfig
	}

	if s.StartupParameters != nil {
		summary["StartupParameters"] = s.StartupParameters
	}

	return summary
}

// InferenceComponent represents a SageMaker inference component.
type InferenceComponent struct {
	CreationTime             time.Time
	LastModifiedTime         time.Time
	Specification            *InferenceComponentSpecification
	Tags                     map[string]string
	InferenceComponentName   string
	InferenceComponentArn    string
	EndpointName             string
	EndpointArn              string
	VariantName              string
	InferenceComponentStatus string
	FailureReason            string
	DeploymentConfig         json.RawMessage
	Specifications           []InferenceComponentSpecification
	CopyCount                int32
	CurrentCopyCount         int32
}

func cloneInferenceComponent(c *InferenceComponent) *InferenceComponent {
	cp := *c
	cp.Tags = maps.Clone(c.Tags)
	cp.Specifications = append([]InferenceComponentSpecification(nil), c.Specifications...)

	return &cp
}

// CreateInferenceComponentOptions holds input fields for CreateInferenceComponent.
type CreateInferenceComponentOptions struct {
	Tags                   map[string]string
	Specification          *InferenceComponentSpecification
	InferenceComponentName string
	EndpointName           string
	VariantName            string
	Specifications         []InferenceComponentSpecification
	CopyCount              int32
}

// CreateInferenceComponent creates a SageMaker inference component and
// schedules its Creating -> InService transition.
func (b *InMemoryBackend) CreateInferenceComponent(
	ctx context.Context,
	opts CreateInferenceComponentOptions,
) (*InferenceComponent, error) {
	b.mu.Lock("CreateInferenceComponent")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if opts.InferenceComponentName == "" {
		return nil, fmt.Errorf("%w: InferenceComponentName is required", ErrValidation)
	}

	if _, ok := b.inferenceComponentsStore(region).Get(opts.InferenceComponentName); ok {
		return nil, fmt.Errorf(
			"%w: inference component %q already exists",
			ErrInferenceComponentAlreadyExists,
			opts.InferenceComponentName,
		)
	}

	compARN := arn.Build(
		"sagemaker",
		region,
		b.accountID,
		"inference-component/"+opts.InferenceComponentName,
	)
	endpointARN := arn.Build("sagemaker", region, b.accountID, "endpoint/"+opts.EndpointName)
	now := time.Now()

	c := &InferenceComponent{
		InferenceComponentName:   opts.InferenceComponentName,
		InferenceComponentArn:    compARN,
		EndpointName:             opts.EndpointName,
		EndpointArn:              endpointARN,
		VariantName:              opts.VariantName,
		InferenceComponentStatus: statusCreating,
		CopyCount:                opts.CopyCount,
		Specification:            opts.Specification,
		Specifications:           opts.Specifications,
		Tags:                     mergeTags(nil, opts.Tags),
		CreationTime:             now,
		LastModifiedTime:         now,
	}
	b.inferenceComponentsStore(region).Put(c)

	b.scheduleInferenceComponentTransition(
		b.lifecycleCtx, region, opts.InferenceComponentName, statusCreating, statusInService,
		inferenceComponentCreatingToInService,
	)

	return cloneInferenceComponent(c), nil
}

// scheduleInferenceComponentTransition drives an inference component from
// fromStatus to nextStatus after delay, and — when nextStatus is InService —
// catches CurrentCopyCount up to the desired CopyCount. A no-op if the
// component has since been deleted or has moved to a status other than
// fromStatus (e.g. a later overlapping transition already completed it).
// ctx must be b.lifecycleCtx captured by the caller while holding b.mu.
func (b *InMemoryBackend) scheduleInferenceComponentTransition(
	ctx context.Context,
	region, name, fromStatus, nextStatus string,
	delay time.Duration,
) {
	b.runDelayed(ctx, delay, func() {
		b.mu.Lock("scheduleInferenceComponentTransition.goroutine")
		defer b.mu.Unlock()

		c, ok := b.inferenceComponentsStore(region).Get(name)
		if !ok || c.InferenceComponentStatus != fromStatus {
			return
		}

		c.InferenceComponentStatus = nextStatus
		c.LastModifiedTime = time.Now()

		if nextStatus == statusInService {
			c.CurrentCopyCount = c.CopyCount
		}
	})
}

// DescribeInferenceComponent returns an inference component by name.
func (b *InMemoryBackend) DescribeInferenceComponent(ctx context.Context, name string) (*InferenceComponent, error) {
	b.mu.RLock("DescribeInferenceComponent")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	c, ok := b.inferenceComponentsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: inference component %q", ErrInferenceComponentNotFound, name)
	}

	return cloneInferenceComponent(c), nil
}

// ListInferenceComponentsFilter narrows the results of ListInferenceComponents
// (api_op_ListInferenceComponents.go:30-71). SortBy defaults to CreationTime,
// SortOrder to Descending — both documented defaults for this op.
type ListInferenceComponentsFilter struct {
	CreationTimeAfter      *time.Time
	CreationTimeBefore     *time.Time
	LastModifiedTimeAfter  *time.Time
	LastModifiedTimeBefore *time.Time
	EndpointNameEquals     string
	NameContains           string
	StatusEquals           string
	VariantNameEquals      string
	SortBy                 string
	SortOrder              string
	NextToken              string
	MaxResults             int32
}

// inferenceComponentMatchesIdentityFilters reports whether c satisfies filter's
// name/endpoint/variant/status fields.
func inferenceComponentMatchesIdentityFilters(c *InferenceComponent, filter ListInferenceComponentsFilter) bool {
	if filter.EndpointNameEquals != "" && c.EndpointName != filter.EndpointNameEquals {
		return false
	}

	if filter.VariantNameEquals != "" && c.VariantName != filter.VariantNameEquals {
		return false
	}

	if filter.StatusEquals != "" && c.InferenceComponentStatus != filter.StatusEquals {
		return false
	}

	return filter.NameContains == "" ||
		strings.Contains(strings.ToLower(c.InferenceComponentName), strings.ToLower(filter.NameContains))
}

// inferenceComponentMatchesTimeFilters reports whether c satisfies filter's
// creation/last-modified time-window fields.
func inferenceComponentMatchesTimeFilters(c *InferenceComponent, filter ListInferenceComponentsFilter) bool {
	if filter.CreationTimeAfter != nil && !c.CreationTime.After(*filter.CreationTimeAfter) {
		return false
	}

	if filter.CreationTimeBefore != nil && !c.CreationTime.Before(*filter.CreationTimeBefore) {
		return false
	}

	if filter.LastModifiedTimeAfter != nil && !c.LastModifiedTime.After(*filter.LastModifiedTimeAfter) {
		return false
	}

	if filter.LastModifiedTimeBefore != nil && !c.LastModifiedTime.Before(*filter.LastModifiedTimeBefore) {
		return false
	}

	return true
}

// inferenceComponentMatchesFilter reports whether c satisfies every set field of filter.
func inferenceComponentMatchesFilter(c *InferenceComponent, filter ListInferenceComponentsFilter) bool {
	return inferenceComponentMatchesIdentityFilters(c, filter) && inferenceComponentMatchesTimeFilters(c, filter)
}

// lessInferenceComponent orders a before b by sortBy (Name/Status/default CreationTime).
func lessInferenceComponent(a, b *InferenceComponent, sortBy string) bool {
	switch sortBy {
	case keyGenericName:
		return a.InferenceComponentName < b.InferenceComponentName
	case keyStatus:
		return a.InferenceComponentStatus < b.InferenceComponentStatus
	default:
		return a.CreationTime.Before(b.CreationTime)
	}
}

// ListInferenceComponents returns inference components matching filter,
// sorted by filter.SortBy (default CreationTime) / filter.SortOrder (default
// Descending).
func (b *InMemoryBackend) ListInferenceComponents(
	ctx context.Context,
	filter ListInferenceComponentsFilter,
) ([]*InferenceComponent, string) {
	b.mu.RLock("ListInferenceComponents")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	store := b.inferenceComponentsStoreRO(region)

	list := make([]*InferenceComponent, 0, store.Len())

	for _, c := range store.All() {
		if inferenceComponentMatchesFilter(c, filter) {
			list = append(list, cloneInferenceComponent(c))
		}
	}

	desc := !strings.EqualFold(filter.SortOrder, "Ascending")
	sort.Slice(list, func(i, k int) bool {
		less := lessInferenceComponent(list[i], list[k], filter.SortBy)
		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, filter.NextToken, filter.MaxResults)
}

// UpdateInferenceComponentOptions holds the fields UpdateInferenceComponent
// (api_op_UpdateInferenceComponent.go:24-53) can change. There is no
// VariantName member on the real input — a production variant is fixed at
// Create time and cannot be moved via Update.
type UpdateInferenceComponentOptions struct {
	DeploymentConfig json.RawMessage
	RuntimeConfig    *InferenceComponentRuntimeConfigInput
	Specification    *InferenceComponentSpecification
	Specifications   []InferenceComponentSpecification
}

// InferenceComponentRuntimeConfigInput mirrors types.InferenceComponentRuntimeConfig
// (types/types.go:11893-11902).
type InferenceComponentRuntimeConfigInput struct {
	CopyCount int32 `json:"CopyCount"`
}

// UpdateInferenceComponent applies opts and schedules an Updating -> InService
// transition (real AWS behavior while the new configuration rolls out).
func (b *InMemoryBackend) UpdateInferenceComponent(
	ctx context.Context,
	name string,
	opts UpdateInferenceComponentOptions,
) error {
	b.mu.Lock("UpdateInferenceComponent")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	c, ok := b.inferenceComponentsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: inference component %q", ErrInferenceComponentNotFound, name)
	}

	if opts.RuntimeConfig != nil {
		c.CopyCount = opts.RuntimeConfig.CopyCount
	}

	if opts.Specification != nil {
		c.Specification = opts.Specification
	}

	if opts.Specifications != nil {
		c.Specifications = opts.Specifications
	}

	if opts.DeploymentConfig != nil {
		c.DeploymentConfig = opts.DeploymentConfig
	}

	c.InferenceComponentStatus = statusUpdating
	c.LastModifiedTime = time.Now()

	b.scheduleInferenceComponentTransition(
		b.lifecycleCtx, region, name, statusUpdating, statusInService, inferenceComponentUpdatingToInService,
	)

	return nil
}

// UpdateInferenceComponentRuntimeConfig updates the desired copy count for an
// inference component and schedules an Updating -> InService transition.
func (b *InMemoryBackend) UpdateInferenceComponentRuntimeConfig(
	ctx context.Context,
	name string,
	copyCount int32,
) error {
	b.mu.Lock("UpdateInferenceComponentRuntimeConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	c, ok := b.inferenceComponentsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: inference component %q", ErrInferenceComponentNotFound, name)
	}

	c.CopyCount = copyCount
	c.InferenceComponentStatus = statusUpdating
	c.LastModifiedTime = time.Now()

	b.scheduleInferenceComponentTransition(
		b.lifecycleCtx, region, name, statusUpdating, statusInService, inferenceComponentUpdatingToInService,
	)

	return nil
}

// DeleteInferenceComponent deletes an inference component by name.
func (b *InMemoryBackend) DeleteInferenceComponent(ctx context.Context, name string) error {
	b.mu.Lock("DeleteInferenceComponent")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	store := b.inferenceComponentsStore(region)

	if _, ok := store.Get(name); !ok {
		return fmt.Errorf("%w: inference component %q", ErrInferenceComponentNotFound, name)
	}

	store.Delete(name)

	return nil
}
