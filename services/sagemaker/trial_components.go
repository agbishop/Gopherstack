package sagemaker

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// sortTrialComponentsByName is SortTrialComponentsBy's "Name" value
// (types/enums.go:9345-9346); the enum's only sibling is CreationTime, the
// default applied when SortBy is unset.
const sortTrialComponentsByName = "Name"

var (
	// ErrTrialComponentNotFound is returned when a trial component does not exist.
	ErrTrialComponentNotFound = awserr.New("ResourceNotFound", awserr.ErrNotFound)
	// ErrTrialComponentAlreadyExists is returned when a trial component already exists.
	ErrTrialComponentAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
	// ErrTrialComponentInUse is returned when deleting a trial component still associated with a trial.
	ErrTrialComponentInUse = awserr.New("ResourceInUse", awserr.ErrConflict)
)

// TrialComponent represents a SageMaker Trial Component.
type TrialComponent struct {
	CreationTime       time.Time                         `json:"CreationTime"`
	LastModifiedTime   time.Time                         `json:"LastModifiedTime"`
	StartTime          *time.Time                        `json:"StartTime,omitempty"`
	EndTime            *time.Time                        `json:"EndTime,omitempty"`
	Status             *TrialComponentStatus             `json:"Status,omitempty"`
	Tags               map[string]string                 `json:"Tags,omitempty"`
	Parameters         map[string]TrialComponentValue    `json:"Parameters,omitempty"`
	InputArtifacts     map[string]TrialComponentArtifact `json:"InputArtifacts,omitempty"`
	OutputArtifacts    map[string]TrialComponentArtifact `json:"OutputArtifacts,omitempty"`
	MetadataProperties *MetadataProperties               `json:"MetadataProperties,omitempty"`
	TrialComponentName string                            `json:"TrialComponentName"`
	TrialComponentArn  string                            `json:"TrialComponentArn"`
	DisplayName        string                            `json:"DisplayName,omitempty"`
}

// TrialComponentStatus reports a trial component's lifecycle state.
// SDK ref: aws-sdk-go-v2/service/sagemaker/types.TrialComponentStatus
// ({PrimaryStatus, Message}) — DescribeTrialComponentOutput.Status is this
// struct on the wire, never a bare string.
type TrialComponentStatus struct {
	PrimaryStatus string `json:"PrimaryStatus,omitempty"`
	Message       string `json:"Message,omitempty"`
}

// TrialComponentValue is a number or string parameter value.
type TrialComponentValue struct {
	NumberValue *float64 `json:"NumberValue,omitempty"`
	StringValue string   `json:"StringValue,omitempty"`
}

// TrialComponentArtifact is a URI/media-type artifact reference.
type TrialComponentArtifact struct {
	Value     string `json:"Value"`
	MediaType string `json:"MediaType,omitempty"`
}

func cloneTrialComponent(tc *TrialComponent) *TrialComponent {
	cp := *tc
	cp.Tags = maps.Clone(tc.Tags)
	cp.Parameters = maps.Clone(tc.Parameters)
	cp.InputArtifacts = maps.Clone(tc.InputArtifacts)
	cp.OutputArtifacts = maps.Clone(tc.OutputArtifacts)

	if tc.StartTime != nil {
		st := *tc.StartTime
		cp.StartTime = &st
	}

	if tc.EndTime != nil {
		et := *tc.EndTime
		cp.EndTime = &et
	}

	if tc.Status != nil {
		st := *tc.Status
		cp.Status = &st
	}

	if tc.MetadataProperties != nil {
		mp := *tc.MetadataProperties
		cp.MetadataProperties = &mp
	}

	return &cp
}

// CreateTrialComponentOptions holds the parameters CreateTrialComponent accepts.
type CreateTrialComponentOptions struct {
	StartTime          *time.Time
	EndTime            *time.Time
	Status             *TrialComponentStatus
	Parameters         map[string]TrialComponentValue
	InputArtifacts     map[string]TrialComponentArtifact
	OutputArtifacts    map[string]TrialComponentArtifact
	MetadataProperties *MetadataProperties
	Tags               map[string]string
	TrialComponentName string
	DisplayName        string
}

// CreateTrialComponent creates a new trial component.
func (b *InMemoryBackend) CreateTrialComponent(
	ctx context.Context,
	opts CreateTrialComponentOptions,
) (*TrialComponent, error) {
	b.mu.Lock("CreateTrialComponent")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.trialComponentsStore(region).Get(opts.TrialComponentName); ok {
		return nil, fmt.Errorf(
			"%w: trial component %s already exists",
			ErrTrialComponentAlreadyExists,
			opts.TrialComponentName,
		)
	}

	tcArn := arn.Build(
		"sagemaker", region, b.accountID, "experiment-trial-component/"+opts.TrialComponentName,
	)
	now := time.Now()

	tc := &TrialComponent{
		TrialComponentName: opts.TrialComponentName,
		TrialComponentArn:  tcArn,
		DisplayName:        opts.DisplayName,
		StartTime:          opts.StartTime,
		EndTime:            opts.EndTime,
		Status:             opts.Status,
		Parameters:         maps.Clone(opts.Parameters),
		InputArtifacts:     maps.Clone(opts.InputArtifacts),
		OutputArtifacts:    maps.Clone(opts.OutputArtifacts),
		MetadataProperties: opts.MetadataProperties,
		CreationTime:       now,
		LastModifiedTime:   now,
		Tags:               mergeTags(nil, opts.Tags),
	}
	b.trialComponentsStore(region).Put(tc)

	return cloneTrialComponent(tc), nil
}

// DescribeTrialComponent returns a trial component by name.
func (b *InMemoryBackend) DescribeTrialComponent(ctx context.Context, name string) (*TrialComponent, error) {
	b.mu.RLock("DescribeTrialComponent")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	tc, ok := b.trialComponentsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: trial component %q not found", ErrTrialComponentNotFound, name)
	}

	return cloneTrialComponent(tc), nil
}

// trialComponentLineageGroupArn returns the ARN of the account's single
// auto-provisioned default lineage group (lineage.go's defaultLineageGroupName
// — SageMaker has no CreateLineageGroup op, confirmed absent from the pinned
// SDK). Every trial component belongs to it, which
// DescribeTrialComponentOutput.LineageGroupArn requires
// (api_op_DescribeTrialComponent.go); TrialComponentSummary does not carry it.
func (b *InMemoryBackend) trialComponentLineageGroupArn(ctx context.Context) string {
	region := getRegion(ctx, b.region)

	return arn.Build("sagemaker", region, b.accountID, "lineage-group/"+defaultLineageGroupName)
}

// DeleteTrialComponent deletes a trial component.
func (b *InMemoryBackend) DeleteTrialComponent(ctx context.Context, name string) (*TrialComponent, error) {
	b.mu.Lock("DeleteTrialComponent")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	store := b.trialComponentsStore(region)

	tc, ok := store.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: trial component %q not found", ErrTrialComponentNotFound, name)
	}

	for _, assoc := range b.trialComponentAssociationsStoreRO(region).All() {
		if assoc.TrialComponentName == name {
			return nil, fmt.Errorf(
				"%w: trial component %q is still associated with trial %q",
				ErrTrialComponentInUse, name, assoc.TrialName,
			)
		}
	}

	cp := cloneTrialComponent(tc)
	store.Delete(name)

	return cp, nil
}

// UpdateTrialComponentOptions holds optional fields for UpdateTrialComponent.
type UpdateTrialComponentOptions struct {
	StartTime               *time.Time
	EndTime                 *time.Time
	Status                  *TrialComponentStatus
	Parameters              map[string]TrialComponentValue
	InputArtifacts          map[string]TrialComponentArtifact
	OutputArtifacts         map[string]TrialComponentArtifact
	DisplayName             string
	ParametersToRemove      []string
	InputArtifactsToRemove  []string
	OutputArtifactsToRemove []string
}

// UpdateTrialComponent mutates DisplayName, Parameters, and Artifacts on a
// trial component. Per api_op_UpdateTrialComponent.go, the *ToRemove lists are
// applied after the corresponding additive map, matching
// UpdateActionInput/UpdateArtifactInput's own additive-then-remove order
// elsewhere in this file's sibling lineage handlers.
func (b *InMemoryBackend) UpdateTrialComponent(
	ctx context.Context,
	name string,
	opts UpdateTrialComponentOptions,
) (*TrialComponent, error) {
	b.mu.Lock("UpdateTrialComponent")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	tc, ok := b.trialComponentsStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: trial component %q not found", ErrTrialComponentNotFound, name)
	}

	if opts.DisplayName != "" {
		tc.DisplayName = opts.DisplayName
	}
	if opts.Status != nil {
		st := *opts.Status
		tc.Status = &st
	}
	if opts.StartTime != nil {
		st := *opts.StartTime
		tc.StartTime = &st
	}
	if opts.EndTime != nil {
		et := *opts.EndTime
		tc.EndTime = &et
	}
	if len(opts.Parameters) > 0 {
		if tc.Parameters == nil {
			tc.Parameters = make(map[string]TrialComponentValue)
		}
		maps.Copy(tc.Parameters, opts.Parameters)
	}
	if len(opts.InputArtifacts) > 0 {
		if tc.InputArtifacts == nil {
			tc.InputArtifacts = make(map[string]TrialComponentArtifact)
		}
		maps.Copy(tc.InputArtifacts, opts.InputArtifacts)
	}
	if len(opts.OutputArtifacts) > 0 {
		if tc.OutputArtifacts == nil {
			tc.OutputArtifacts = make(map[string]TrialComponentArtifact)
		}
		maps.Copy(tc.OutputArtifacts, opts.OutputArtifacts)
	}
	for _, k := range opts.ParametersToRemove {
		delete(tc.Parameters, k)
	}
	for _, k := range opts.InputArtifactsToRemove {
		delete(tc.InputArtifacts, k)
	}
	for _, k := range opts.OutputArtifactsToRemove {
		delete(tc.OutputArtifacts, k)
	}
	tc.LastModifiedTime = time.Now()

	return cloneTrialComponent(tc), nil
}

// DisassociateTrialComponent removes the association between a trial and a
// trial component, if one exists. It is idempotent: disassociating a
// component that was never associated succeeds and simply returns the
// resources' ARNs (mirroring AssociateTrialComponent's non-strict existence
// checks).
func (b *InMemoryBackend) DisassociateTrialComponent(
	ctx context.Context,
	trialName, trialComponentName string,
) (string, string, error) {
	b.mu.Lock("DisassociateTrialComponent")
	defer b.mu.Unlock()

	if trialName == "" {
		return "", "", fmt.Errorf("%w: TrialName is required", ErrValidation)
	}

	if trialComponentName == "" {
		return "", "", fmt.Errorf("%w: TrialComponentName is required", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	trialArn := arn.Build("sagemaker", region, b.accountID, "experiment-trial/"+trialName)
	trialComponentArn := arn.Build(
		"sagemaker", region, b.accountID, "experiment-trial-component/"+trialComponentName,
	)

	key := trialComponentKey(trialName, trialComponentName)
	b.trialComponentAssociationsStore(region).Delete(key)

	return trialArn, trialComponentArn, nil
}

// ListTrialComponentsParams bundles ListTrialComponents' filter/sort/
// pagination criteria (api_op_ListTrialComponents.go:30-71). SourceArn is
// decoded by the handler for wire-shape fidelity but is a disclosed no-op
// here: CreateTrialComponentInput has no Source field at all (a trial
// component's TrialComponentSource is only ever populated when SageMaker
// auto-tracks a processing/training job, which this backend never does), so
// no trial component ever has a source ARN to filter by.
type ListTrialComponentsParams struct {
	ExperimentName string
	TrialName      string
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	SortBy         string
	SortOrder      string
	NextToken      string
	MaxResults     int32
}

// ListTrialComponents returns trial components, optionally filtered by the
// trial they're associated with, the experiment their trial belongs to, and a
// creation-time window, sorted by params.SortBy (default CreationTime) /
// params.SortOrder (default Descending — both documented defaults, per
// api_op_ListTrialComponents.go).
func (b *InMemoryBackend) ListTrialComponents(
	ctx context.Context,
	params ListTrialComponentsParams,
) ([]*TrialComponent, string) {
	b.mu.RLock("ListTrialComponents")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	allowed := b.trialComponentNameFilterLocked(region, params.ExperimentName, params.TrialName)

	list := make([]*TrialComponent, 0, b.trialComponentsStoreRO(region).Len())

	for _, tc := range b.trialComponentsStoreRO(region).All() {
		if allowed != nil {
			if _, ok := allowed[tc.TrialComponentName]; !ok {
				continue
			}
		}
		if params.CreatedAfter != nil && !tc.CreationTime.After(*params.CreatedAfter) {
			continue
		}
		if params.CreatedBefore != nil && !tc.CreationTime.Before(*params.CreatedBefore) {
			continue
		}

		list = append(list, cloneTrialComponent(tc))
	}

	desc := !strings.EqualFold(params.SortOrder, "Ascending")
	sort.Slice(list, func(i, j int) bool {
		var less bool
		if params.SortBy == sortTrialComponentsByName {
			less = list[i].TrialComponentName < list[j].TrialComponentName
		} else {
			less = list[i].CreationTime.Before(list[j].CreationTime)
		}

		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, params.NextToken, params.MaxResults)
}

// trialComponentNameFilterLocked returns the set of trial component names
// allowed by the given TrialName/ExperimentName filters, or nil if neither
// filter is set (meaning: no filtering). Callers must hold b.mu.
func (b *InMemoryBackend) trialComponentNameFilterLocked(
	region, experimentName, trialName string,
) map[string]bool {
	if trialName == "" && experimentName == "" {
		return nil
	}

	trialNames := map[string]bool{}

	if trialName != "" {
		trialNames[trialName] = true
	} else {
		for _, t := range b.trialsStoreRO(region).All() {
			if t.ExperimentName == experimentName {
				trialNames[t.TrialName] = true
			}
		}
	}

	allowed := map[string]bool{}

	for _, assoc := range b.trialComponentAssociationsStoreRO(region).All() {
		if trialNames[assoc.TrialName] {
			allowed[assoc.TrialComponentName] = true
		}
	}

	return allowed
}
