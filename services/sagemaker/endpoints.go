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

var (
	// ErrEndpointNotFound is returned when an endpoint does not exist.
	ErrEndpointNotFound = awserr.New("ValidationException", awserr.ErrNotFound)
	// ErrEndpointAlreadyExists is returned when an endpoint already exists.
	ErrEndpointAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
)

// Endpoint represents a SageMaker endpoint. AsyncInferenceConfig/
// DataCaptureConfig/ShadowProductionVariants are copied from the active
// EndpointConfig at Create/Update time and surfaced on Describe
// (api_op_DescribeEndpoint.go:39-97) — types.ExplainerConfig/MetricsConfig
// have no counterpart on this service's EndpointConfig type and are
// disclosed no-ops; PendingDeploymentSummary is not simulated at all.
type Endpoint struct {
	LastModifiedTime         time.Time                  `json:"LastModifiedTime"`
	CreationTime             time.Time                  `json:"CreationTime"`
	Tags                     map[string]string          `json:"Tags,omitempty"`
	AsyncInferenceConfig     *AsyncInferenceConfig      `json:"AsyncInferenceConfig,omitempty"`
	DataCaptureConfig        *DataCaptureConfig         `json:"DataCaptureConfig,omitempty"`
	EndpointArn              string                     `json:"EndpointArn"`
	EndpointName             string                     `json:"EndpointName"`
	EndpointConfigName       string                     `json:"EndpointConfigName"`
	EndpointStatus           string                     `json:"EndpointStatus"`
	FailureReason            string                     `json:"FailureReason,omitempty"`
	ProductionVariants       []ProductionVariantSummary `json:"ProductionVariants,omitempty"`
	ShadowProductionVariants []ProductionVariantSummary `json:"ShadowProductionVariants,omitempty"`
	DeploymentConfig         json.RawMessage            `json:"DeploymentConfig,omitempty"`
}

// ProductionVariantStatus describes the current deployment stage of a
// production variant on a deployed endpoint.
type ProductionVariantStatus struct {
	Status        string `json:"Status"`
	StatusMessage string `json:"StatusMessage,omitempty"`
}

// ProductionVariantSummary describes a production variant as deployed on a
// live endpoint (the shape returned by DescribeEndpoint). This is distinct
// from ProductionVariant, which is the EndpointConfig-time configuration:
// AWS renames Initial* to Desired* and adds Current* fields that reflect
// deployed state once the endpoint has finished (re)deploying.
type ProductionVariantSummary struct {
	CurrentWeight        *float64                  `json:"CurrentWeight,omitempty"`
	DesiredWeight        *float64                  `json:"DesiredWeight,omitempty"`
	CurrentInstanceCount *int32                    `json:"CurrentInstanceCount,omitempty"`
	DesiredInstanceCount *int32                    `json:"DesiredInstanceCount,omitempty"`
	VariantName          string                    `json:"VariantName"`
	VariantStatus        []ProductionVariantStatus `json:"VariantStatus,omitempty"`
}

// newVariantSummaries builds the initial ProductionVariantSummary list for an
// endpoint from an endpoint config's ProductionVariants. Desired* fields are
// populated from the config's Initial* fields; Current* fields are left nil
// until the endpoint (re)reaches InService.
func newVariantSummaries(pvs []ProductionVariant) []ProductionVariantSummary {
	summaries := make([]ProductionVariantSummary, len(pvs))

	for i, pv := range pvs {
		weight := pv.InitialVariantWeight
		count := pv.InitialInstanceCount
		summaries[i] = ProductionVariantSummary{
			VariantName:          pv.VariantName,
			DesiredWeight:        &weight,
			DesiredInstanceCount: &count,
			VariantStatus:        []ProductionVariantStatus{{Status: "Creating"}},
		}
	}

	return summaries
}

// cloneEndpoint returns a deep copy of ep.
func cloneEndpoint(ep *Endpoint) *Endpoint {
	cp := *ep
	cp.Tags = maps.Clone(ep.Tags)
	cp.ProductionVariants = make([]ProductionVariantSummary, len(ep.ProductionVariants))
	for i, pv := range ep.ProductionVariants {
		cp.ProductionVariants[i] = cloneProductionVariantSummary(pv)
	}
	cp.ShadowProductionVariants = make([]ProductionVariantSummary, len(ep.ShadowProductionVariants))
	for i, pv := range ep.ShadowProductionVariants {
		cp.ShadowProductionVariants[i] = cloneProductionVariantSummary(pv)
	}

	if ep.DataCaptureConfig != nil {
		dcc := *ep.DataCaptureConfig
		cp.DataCaptureConfig = &dcc
	}

	if ep.AsyncInferenceConfig != nil {
		aic := *ep.AsyncInferenceConfig
		cp.AsyncInferenceConfig = &aic
	}

	return &cp
}

// cloneProductionVariantSummary returns a deep copy of a ProductionVariantSummary.
func cloneProductionVariantSummary(pv ProductionVariantSummary) ProductionVariantSummary {
	if pv.CurrentWeight != nil {
		w := *pv.CurrentWeight
		pv.CurrentWeight = &w
	}
	if pv.DesiredWeight != nil {
		w := *pv.DesiredWeight
		pv.DesiredWeight = &w
	}
	if pv.CurrentInstanceCount != nil {
		c := *pv.CurrentInstanceCount
		pv.CurrentInstanceCount = &c
	}
	if pv.DesiredInstanceCount != nil {
		c := *pv.DesiredInstanceCount
		pv.DesiredInstanceCount = &c
	}
	pv.VariantStatus = append([]ProductionVariantStatus(nil), pv.VariantStatus...)

	return pv
}

// TrainingJob represents a SageMaker training job.

// ---------------------------------------------------------------------------
// Backend field additions via a separate map — we extend InMemoryBackend with
// a parallel struct to avoid modifying the original large file.
// ---------------------------------------------------------------------------

// extendedState holds the new resource maps added in this file.
// It is embedded into InMemoryBackend via the ext field initialised in
// NewInMemoryBackendExtended (called from NewInMemoryBackend after init).
// We keep it simple: InMemoryBackend itself is extended with four new fields.

// CreateEndpointOptions holds input fields for CreateEndpoint
// (api_op_CreateEndpoint.go:1-49).
type CreateEndpointOptions struct {
	Tags               map[string]string
	Name               string
	EndpointConfigName string
	DeploymentConfig   json.RawMessage
}

// CreateEndpoint creates a new SageMaker endpoint.
func (b *InMemoryBackend) CreateEndpoint(
	ctx context.Context,
	opts CreateEndpointOptions,
) (*Endpoint, error) {
	b.mu.Lock("CreateEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := opts.Name

	if _, ok := b.endpointsStore(region).Get(name); ok {
		return nil, fmt.Errorf("%w: endpoint %s already exists", ErrEndpointAlreadyExists, name)
	}

	ec, ok := b.endpointConfigsStore(region).Get(opts.EndpointConfigName)
	if !ok {
		return nil, fmt.Errorf(
			"%w: could not find endpoint configuration %q",
			ErrEndpointConfigNotFound,
			opts.EndpointConfigName,
		)
	}

	epARN := arn.Build("sagemaker", region, b.accountID, "endpoint/"+name)
	now := time.Now()
	ep := &Endpoint{
		EndpointName:             name,
		EndpointArn:              epARN,
		EndpointConfigName:       opts.EndpointConfigName,
		EndpointStatus:           "Creating",
		CreationTime:             now,
		LastModifiedTime:         now,
		Tags:                     mergeTags(nil, opts.Tags),
		ProductionVariants:       newVariantSummaries(ec.ProductionVariants),
		ShadowProductionVariants: newVariantSummaries(ec.ShadowProductionVariants),
		DataCaptureConfig:        ec.DataCaptureConfig,
		AsyncInferenceConfig:     ec.AsyncInferenceConfig,
		DeploymentConfig:         opts.DeploymentConfig,
	}
	b.endpointsStore(region).Put(ep)
	b.endpointARNIndexStore(region)[epARN] = name

	return cloneEndpoint(ep), nil
}

// DescribeEndpoint returns an endpoint by name.
func (b *InMemoryBackend) DescribeEndpoint(ctx context.Context, name string) (*Endpoint, error) {
	b.mu.RLock("DescribeEndpoint")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	ep, ok := b.endpointsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: endpoint %q not found", ErrEndpointNotFound, name)
	}

	return cloneEndpoint(ep), nil
}

// ListEndpointsFilter narrows the results of ListEndpoints
// (api_op_ListEndpoints.go:30-64). SortBy defaults to CreationTime, SortOrder
// to Descending — both documented defaults for this op.
type ListEndpointsFilter struct {
	CreationTimeAfter      *time.Time
	CreationTimeBefore     *time.Time
	LastModifiedTimeAfter  *time.Time
	LastModifiedTimeBefore *time.Time
	NameContains           string
	StatusEquals           string
	SortBy                 string
	SortOrder              string
	NextToken              string
	MaxResults             int32
}

// endpointMatchesFilter reports whether ep satisfies every set field of filter.
func endpointMatchesFilter(ep *Endpoint, filter ListEndpointsFilter) bool {
	if filter.StatusEquals != "" && ep.EndpointStatus != filter.StatusEquals {
		return false
	}

	if filter.NameContains != "" &&
		!strings.Contains(strings.ToLower(ep.EndpointName), strings.ToLower(filter.NameContains)) {
		return false
	}

	if filter.CreationTimeAfter != nil && !ep.CreationTime.After(*filter.CreationTimeAfter) {
		return false
	}

	if filter.CreationTimeBefore != nil && !ep.CreationTime.Before(*filter.CreationTimeBefore) {
		return false
	}

	if filter.LastModifiedTimeAfter != nil && !ep.LastModifiedTime.After(*filter.LastModifiedTimeAfter) {
		return false
	}

	if filter.LastModifiedTimeBefore != nil && !ep.LastModifiedTime.Before(*filter.LastModifiedTimeBefore) {
		return false
	}

	return true
}

// lessEndpoint orders a before b by sortBy (Name/Status/default CreationTime).
func lessEndpoint(a, b *Endpoint, sortBy string) bool {
	switch sortBy {
	case keyGenericName:
		return a.EndpointName < b.EndpointName
	case keyStatus:
		return a.EndpointStatus < b.EndpointStatus
	default:
		return a.CreationTime.Before(b.CreationTime)
	}
}

// ListEndpoints returns endpoints matching filter, sorted by filter.SortBy
// (default CreationTime) / filter.SortOrder (default Descending).
func (b *InMemoryBackend) ListEndpoints(ctx context.Context, filter ListEndpointsFilter) ([]*Endpoint, string) {
	b.mu.RLock("ListEndpoints")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	store := b.endpointsStoreRO(region)

	list := make([]*Endpoint, 0, store.Len())

	for _, ep := range store.All() {
		if endpointMatchesFilter(ep, filter) {
			list = append(list, cloneEndpoint(ep))
		}
	}

	desc := !strings.EqualFold(filter.SortOrder, "Ascending")
	sort.Slice(list, func(i, k int) bool {
		less := lessEndpoint(list[i], list[k], filter.SortBy)
		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, filter.NextToken, filter.MaxResults)
}

// DeleteEndpoint deletes an endpoint by name.
func (b *InMemoryBackend) DeleteEndpoint(ctx context.Context, name string) error {
	b.mu.Lock("DeleteEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	ep, ok := b.endpointsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: endpoint %q not found", ErrEndpointNotFound, name)
	}

	arnIdx := b.endpointARNIndexStore(region)
	delete(arnIdx, ep.EndpointArn)
	endpoints := b.endpointsStore(region)
	endpoints.Delete(name)

	return nil
}

// UpdateEndpointOptions holds input fields for UpdateEndpoint
// (api_op_UpdateEndpoint.go:1-56). ExcludeRetainedVariantProperties is
// restricted to the two VariantPropertyType values this backend tracks per
// variant (DesiredInstanceCount/DesiredWeight, types/enums.go:10837-10844) —
// DataCaptureConfig is endpoint-wide here, not per-variant, so that
// VariantPropertyType value has no effect to exclude.
type UpdateEndpointOptions struct {
	DeploymentConfig                 json.RawMessage
	EndpointConfigName               string
	ExcludeRetainedVariantProperties []string
	RetainAllVariantProperties       bool
	RetainDeploymentConfig           bool
}

// carryOverVariantProperties applies UpdateEndpoint's retention semantics to
// newVariants (freshly built from the new EndpointConfig): Current* always
// carries over from the same-named old variant, since traffic keeps flowing
// on the old counts until the update finishes rolling out regardless of
// RetainAllVariantProperties (which governs Desired* — the new
// EndpointConfig's targets — instead).
func carryOverVariantProperties(
	newVariants, oldVariants []ProductionVariantSummary,
	opts UpdateEndpointOptions,
) []ProductionVariantSummary {
	excluded := make(map[string]bool, len(opts.ExcludeRetainedVariantProperties))
	for _, p := range opts.ExcludeRetainedVariantProperties {
		excluded[p] = true
	}

	for i := range newVariants {
		for _, old := range oldVariants {
			if old.VariantName != newVariants[i].VariantName {
				continue
			}

			newVariants[i].CurrentWeight = old.CurrentWeight
			newVariants[i].CurrentInstanceCount = old.CurrentInstanceCount

			if opts.RetainAllVariantProperties {
				if !excluded["DesiredWeight"] {
					newVariants[i].DesiredWeight = old.DesiredWeight
				}

				if !excluded["DesiredInstanceCount"] {
					newVariants[i].DesiredInstanceCount = old.DesiredInstanceCount
				}
			}

			break
		}
	}

	return newVariants
}

// UpdateEndpoint updates the endpoint config for an existing endpoint.
func (b *InMemoryBackend) UpdateEndpoint(
	ctx context.Context,
	name string,
	opts UpdateEndpointOptions,
) (*Endpoint, error) {
	b.mu.Lock("UpdateEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	ep, ok := b.endpointsStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: endpoint %q not found", ErrEndpointNotFound, name)
	}

	ec, ok := b.endpointConfigsStore(region).Get(opts.EndpointConfigName)
	if !ok {
		return nil, fmt.Errorf(
			"%w: could not find endpoint configuration %q",
			ErrEndpointConfigNotFound,
			opts.EndpointConfigName,
		)
	}

	newVariants := carryOverVariantProperties(newVariantSummaries(ec.ProductionVariants), ep.ProductionVariants, opts)

	ep.EndpointConfigName = opts.EndpointConfigName
	ep.EndpointStatus = statusUpdating
	ep.LastModifiedTime = time.Now()
	ep.ProductionVariants = newVariants
	ep.ShadowProductionVariants = newVariantSummaries(ec.ShadowProductionVariants)
	ep.DataCaptureConfig = ec.DataCaptureConfig
	ep.AsyncInferenceConfig = ec.AsyncInferenceConfig

	if !opts.RetainDeploymentConfig {
		ep.DeploymentConfig = opts.DeploymentConfig
	}

	return cloneEndpoint(ep), nil
}

// ---------------------------------------------------------------------------
// Endpoint lifecycle FSM (#9)
// ---------------------------------------------------------------------------

// scheduleEndpointTransition drives an endpoint from fromStatus to nextStatus
// after delay, a no-op if the endpoint has since been deleted or has moved to
// a status other than fromStatus (e.g. a later overlapping transition already
// completed it). ctx must be b.lifecycleCtx captured by the caller while
// holding b.mu. region must be captured by the caller before the lock is
// released.
func (b *InMemoryBackend) scheduleEndpointTransition(
	ctx context.Context,
	region, name, fromStatus, nextStatus string,
	delay time.Duration,
) {
	b.runDelayed(ctx, delay, func() {
		b.mu.Lock("scheduleEndpointTransition.goroutine")
		defer b.mu.Unlock()

		ep, ok := b.endpointsStore(region).Get(name)
		if !ok || ep.EndpointStatus != fromStatus {
			return
		}

		ep.EndpointStatus = nextStatus
		ep.LastModifiedTime = time.Now()

		if nextStatus == statusInService {
			for i := range ep.ProductionVariants {
				ep.ProductionVariants[i].CurrentWeight = ep.ProductionVariants[i].DesiredWeight
				ep.ProductionVariants[i].CurrentInstanceCount = ep.ProductionVariants[i].DesiredInstanceCount
				ep.ProductionVariants[i].VariantStatus = []ProductionVariantStatus{{Status: statusInService}}
			}
		}
	})
}

// CreateEndpointFSM creates an endpoint and schedules Creating → InService.
func (b *InMemoryBackend) CreateEndpointFSM(ctx context.Context, opts CreateEndpointOptions) (*Endpoint, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("CreateEndpointFSM.ctx")
	lifecycleCtx := b.lifecycleCtx
	b.mu.RUnlock()

	ep, err := b.CreateEndpoint(ctx, opts)
	if err != nil {
		return nil, err
	}
	b.scheduleEndpointTransition(
		lifecycleCtx, region, opts.Name, statusCreating, statusInService, endpointCreatingToInService,
	)

	return ep, nil
}

// UpdateEndpointFSM updates config and drives InService → Updating → InService.
func (b *InMemoryBackend) UpdateEndpointFSM(
	ctx context.Context,
	name string,
	opts UpdateEndpointOptions,
) (*Endpoint, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("UpdateEndpointFSM.ctx")
	lifecycleCtx := b.lifecycleCtx
	b.mu.RUnlock()

	ep, err := b.UpdateEndpoint(ctx, name, opts)
	if err != nil {
		return nil, err
	}
	b.scheduleEndpointTransition(
		lifecycleCtx, region, name, statusUpdating, statusInService, endpointUpdatingToInService,
	)

	return ep, nil
}

// UpdateEndpointWeightsAndCapacitiesFull applies weight/capacity changes and drives Updating → InService.
func (b *InMemoryBackend) UpdateEndpointWeightsAndCapacitiesFull(
	ctx context.Context,
	name string,
	changes []DesiredWeightAndCapacity,
) (*Endpoint, error) {
	b.mu.Lock("UpdateEndpointWeightsAndCapacitiesFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	ep, ok := b.endpointsStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: endpoint %q not found", ErrEndpointNotFound, name)
	}

	// Apply weight/capacity changes to the endpoint's variant snapshots.
	for _, change := range changes {
		found := false
		for i := range ep.ProductionVariants {
			if ep.ProductionVariants[i].VariantName == change.VariantName {
				if change.DesiredWeight != nil {
					w := *change.DesiredWeight
					ep.ProductionVariants[i].DesiredWeight = &w
				}
				if change.DesiredInstanceCount != nil {
					c := *change.DesiredInstanceCount
					ep.ProductionVariants[i].DesiredInstanceCount = &c
				}
				found = true

				break
			}
		}
		if !found {
			return nil, fmt.Errorf(
				"%w: variant %q not found in endpoint %q",
				ErrValidation,
				change.VariantName,
				name,
			)
		}
	}

	ep.EndpointStatus = statusUpdating
	ep.LastModifiedTime = time.Now()

	cp := cloneEndpoint(ep)
	b.scheduleEndpointTransition(
		b.lifecycleCtx, region, name, statusUpdating, statusInService, endpointUpdatingToInService,
	)

	return cp, nil
}

// DesiredWeightAndCapacity is one entry in UpdateEndpointWeightsAndCapacities.
type DesiredWeightAndCapacity struct {
	DesiredWeight        *float64 `json:"DesiredWeight,omitempty"`
	DesiredInstanceCount *int32   `json:"DesiredInstanceCount,omitempty"`
	VariantName          string   `json:"VariantName"`
}
