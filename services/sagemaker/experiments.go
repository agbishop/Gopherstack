package sagemaker

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrExperimentNotFound is returned when an experiment does not exist.
	ErrExperimentNotFound = awserr.New("ResourceNotFound", awserr.ErrNotFound)
	// ErrExperimentAlreadyExists is returned when an experiment already exists.
	ErrExperimentAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
	// ErrExperimentInUse is returned when deleting an experiment that still has trials.
	ErrExperimentInUse = awserr.New("ResourceInUse", awserr.ErrConflict)
)

// Experiment represents a SageMaker Experiment. CreatedBy/LastModifiedBy
// (types.UserContext) and Source (types.ExperimentSource) are disclosed
// absent -- this service models no caller-identity concept, and experiments
// here are always created directly rather than derived from another
// resource (e.g. a Pipeline execution).
type Experiment struct {
	CreationTime     time.Time         `json:"CreationTime"`
	LastModifiedTime time.Time         `json:"LastModifiedTime"`
	Tags             map[string]string `json:"Tags,omitempty"`
	ExperimentName   string            `json:"ExperimentName"`
	ExperimentArn    string            `json:"ExperimentArn"`
	DisplayName      string            `json:"DisplayName,omitempty"`
	Description      string            `json:"Description,omitempty"`
}

func cloneExperiment(e *Experiment) *Experiment {
	cp := *e
	cp.Tags = maps.Clone(e.Tags)

	return &cp
}

// CreateExperiment creates a new experiment.
func (b *InMemoryBackend) CreateExperiment(
	ctx context.Context,
	name, displayName, description string,
	tags map[string]string,
) (*Experiment, error) {
	b.mu.Lock("CreateExperiment")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.experimentsStore(region).Get(name); ok {
		return nil, fmt.Errorf("%w: experiment %s already exists", ErrExperimentAlreadyExists, name)
	}

	expArn := arn.Build("sagemaker", region, b.accountID, "experiment/"+name)
	now := time.Now()

	e := &Experiment{
		ExperimentName:   name,
		ExperimentArn:    expArn,
		DisplayName:      displayName,
		Description:      description,
		CreationTime:     now,
		LastModifiedTime: now,
		Tags:             mergeTags(nil, tags),
	}
	b.experimentsStore(region).Put(e)

	return cloneExperiment(e), nil
}

// DescribeExperiment returns an experiment by name. CreatedBy/LastModifiedBy
// and Source are absent from the response -- see the disclosure on
// [Experiment] above.
func (b *InMemoryBackend) DescribeExperiment(ctx context.Context, name string) (*Experiment, error) {
	b.mu.RLock("DescribeExperiment")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	e, ok := b.experimentsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: experiment %q not found", ErrExperimentNotFound, name)
	}

	return cloneExperiment(e), nil
}

// ListExperimentsFilter bundles the filter/sort criteria for ListExperiments
// (api_op_ListExperiments.go:32-55). Previously this decoded only NextToken
// and dropped every filter and sort control the op's own request shape
// declares.
type ListExperimentsFilter struct {
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	SortBy        string
	SortOrder     string
	MaxResults    int32
}

// ListExperiments returns experiments matching f, sorted and paginated.
//
// api_op_ListExperiments.go:48,51: real defaults are CreationTime/Descending.
func (b *InMemoryBackend) ListExperiments(
	ctx context.Context,
	nextToken string,
	f ListExperimentsFilter,
) ([]*Experiment, string) {
	b.mu.RLock("ListExperiments")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	list := make([]*Experiment, 0, b.experimentsStoreRO(region).Len())

	for _, e := range b.experimentsStoreRO(region).All() {
		if !timeWindowOK(e.CreationTime, f.CreatedAfter, f.CreatedBefore) {
			continue
		}

		list = append(list, cloneExperiment(e))
	}

	desc := f.SortOrder != sortOrderAscending
	sort.SliceStable(list, func(i, k int) bool {
		var less bool

		switch f.SortBy {
		case keyGenericName:
			less = list[i].ExperimentName < list[k].ExperimentName
		default:
			less = list[i].CreationTime.Before(list[k].CreationTime)
		}

		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, nextToken, f.MaxResults)
}

// DeleteExperiment deletes an experiment.
func (b *InMemoryBackend) DeleteExperiment(ctx context.Context, name string) (*Experiment, error) {
	b.mu.Lock("DeleteExperiment")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	store := b.experimentsStore(region)

	e, ok := store.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: experiment %q not found", ErrExperimentNotFound, name)
	}

	for _, tr := range b.trialsStore(region).All() {
		if tr.ExperimentName == name {
			return nil, fmt.Errorf(
				"%w: experiment %q still has trial %q associated with it",
				ErrExperimentInUse, name, tr.TrialName,
			)
		}
	}

	cp := cloneExperiment(e)
	store.Delete(name)

	return cp, nil
}

// UpdateExperiment mutates DisplayName and Description on an experiment.
// Both are *string, matching UpdateExperimentInput
// (api_op_UpdateExperiment.go:28-43): nil means "leave unchanged", a
// present-but-empty string means "clear" -- the op's own doc says it
// "adds, updates, or removes the description", but a plain (non-pointer)
// string field can never distinguish an omitted key from an explicit "",
// making removal unreachable. That was the previous bug.
func (b *InMemoryBackend) UpdateExperiment(
	ctx context.Context,
	name string,
	displayName, description *string,
) (*Experiment, error) {
	b.mu.Lock("UpdateExperiment")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	e, ok := b.experimentsStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: experiment %q not found", ErrExperimentNotFound, name)
	}

	if displayName != nil {
		e.DisplayName = *displayName
	}
	if description != nil {
		e.Description = *description
	}
	e.LastModifiedTime = time.Now()

	return cloneExperiment(e), nil
}
