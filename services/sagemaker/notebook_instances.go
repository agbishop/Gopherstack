package sagemaker

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrNotebookNotFound is returned when a notebook instance does not exist.
	ErrNotebookNotFound = awserr.New("ValidationException", awserr.ErrNotFound)
	// ErrNotebookAlreadyExists is returned when a notebook instance already exists.
	ErrNotebookAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
	// ErrNotebookLifecycleConfigNotFound is returned when a lifecycle config does not exist.
	ErrNotebookLifecycleConfigNotFound = awserr.New("ValidationException", awserr.ErrNotFound)
	// ErrNotebookLifecycleConfigAlreadyExists is returned when a lifecycle config already exists.
	ErrNotebookLifecycleConfigAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
)

type NotebookInstance struct {
	CreationTime                          time.Time         `json:"CreationTime"`
	LastModifiedTime                      time.Time         `json:"LastModifiedTime"`
	Tags                                  map[string]string `json:"Tags,omitempty"`
	RootAccess                            string            `json:"RootAccess,omitempty"`
	KmsKeyID                              string            `json:"KmsKeyId,omitempty"`
	URL                                   string            `json:"Url,omitempty"`
	NotebookInstanceName                  string            `json:"NotebookInstanceName"`
	NotebookInstanceArn                   string            `json:"NotebookInstanceArn"`
	NotebookInstanceStatus                string            `json:"NotebookInstanceStatus"`
	InstanceType                          string            `json:"InstanceType,omitempty"`
	RoleArn                               string            `json:"RoleArn,omitempty"`
	SubnetID                              string            `json:"SubnetId,omitempty"`
	PlatformIdentifier                    string            `json:"PlatformIdentifier,omitempty"`
	LifecycleConfigName                   string            `json:"NotebookInstanceLifecycleConfigName,omitempty"`
	DirectInternetAccess                  string            `json:"DirectInternetAccess,omitempty"`
	DefaultCodeRepository                 string            `json:"DefaultCodeRepository,omitempty"`
	IPAddressType                         string            `json:"IpAddressType,omitempty"`
	MinimumInstanceMetadataServiceVersion string            `json:"MinimumInstanceMetadataServiceVersion,omitempty"`
	SecurityGroupIDs                      []string          `json:"SecurityGroupIds,omitempty"`
	AcceleratorTypes                      []string          `json:"AcceleratorTypes,omitempty"`
	AdditionalCodeRepositories            []string          `json:"AdditionalCodeRepositories,omitempty"`
	VolumeSizeInGB                        int32             `json:"VolumeSizeInGB,omitempty"`
}

// cloneNotebook returns a deep copy of nb.
func cloneNotebook(nb *NotebookInstance) *NotebookInstance {
	cp := *nb
	cp.Tags = maps.Clone(nb.Tags)
	cp.SecurityGroupIDs = append([]string(nil), nb.SecurityGroupIDs...)
	cp.AcceleratorTypes = append([]string(nil), nb.AcceleratorTypes...)
	cp.AdditionalCodeRepositories = append([]string(nil), nb.AdditionalCodeRepositories...)

	return &cp
}

// HyperParameterTuningJob represents a SageMaker hyperparameter tuning job.

// ---------------------------------------------------------------------------
// NotebookInstance
// ---------------------------------------------------------------------------

// CreateNotebookInstance creates a new notebook instance.
func (b *InMemoryBackend) CreateNotebookInstance(
	ctx context.Context,
	name, instanceType, roleArn string,
	tags map[string]string,
) (*NotebookInstance, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: NotebookInstanceName is required", ErrValidation)
	}

	if instanceType == "" {
		return nil, fmt.Errorf("%w: InstanceType is required", ErrValidation)
	}

	if roleArn == "" {
		return nil, fmt.Errorf("%w: RoleArn is required", ErrValidation)
	}

	b.mu.Lock("CreateNotebookInstance")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.notebooksStore(region).Get(name); ok {
		return nil, fmt.Errorf(
			"%w: notebook instance %s already exists",
			ErrNotebookAlreadyExists,
			name,
		)
	}

	nbARN := arn.Build("sagemaker", region, b.accountID, "notebook-instance/"+name)
	now := time.Now()
	nb := &NotebookInstance{
		NotebookInstanceName:   name,
		NotebookInstanceArn:    nbARN,
		URL:                    notebookInstanceURL(nbARN),
		NotebookInstanceStatus: notebookStatusPending,
		InstanceType:           instanceType,
		RoleArn:                roleArn,
		CreationTime:           now,
		LastModifiedTime:       now,
		Tags:                   mergeTags(nil, tags),
	}
	b.notebooksStore(region).Put(nb)
	b.notebookARNIndexStore(region)[nbARN] = name

	return cloneNotebook(nb), nil
}

// DescribeNotebookInstance returns a notebook instance by name.
func (b *InMemoryBackend) DescribeNotebookInstance(ctx context.Context, name string) (*NotebookInstance, error) {
	b.mu.RLock("DescribeNotebookInstance")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	return cloneNotebook(nb), nil
}

// ListNotebookInstancesParams bundles ListNotebookInstances' filter/sort/
// pagination criteria (api_op_ListNotebookInstances.go:31-90,
// sagemaker@v1.263.2). Empty/nil fields are treated as wildcards.
type ListNotebookInstancesParams struct {
	CreationTimeAfter                           *time.Time
	CreationTimeBefore                          *time.Time
	LastModifiedTimeAfter                       *time.Time
	LastModifiedTimeBefore                      *time.Time
	StatusEquals                                string
	NameContains                                string
	AdditionalCodeRepositoryEquals              string
	DefaultCodeRepositoryContains               string
	NotebookInstanceLifecycleConfigNameContains string
	NextToken                                   string
	SortBy                                      string
	SortOrder                                   string
	MaxResults                                  int32
}

// ListNotebookInstances returns notebook instances matching params, sorted by
// params.SortBy (default Name, per api_op_ListNotebookInstances.go:80) /
// params.SortOrder (no default documented, Ascending kept as the disclosed
// fallback per this campaign's ListHubs/ListPipelines precedent), capped at
// params.MaxResults.
func (b *InMemoryBackend) ListNotebookInstances(
	ctx context.Context,
	params ListNotebookInstancesParams,
) ([]*NotebookInstance, string) {
	b.mu.RLock("ListNotebookInstances")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	store := b.notebooksStoreRO(region)
	list := make([]*NotebookInstance, 0, store.Len())
	for _, nb := range store.All() {
		if !matchesNotebookParams(nb, params) {
			continue
		}

		list = append(list, cloneNotebook(nb))
	}

	desc := strings.EqualFold(params.SortOrder, sortOrderDescending)
	sort.Slice(list, func(i, j int) bool {
		less := notebookInstanceSortLess(list[i], list[j], params.SortBy)
		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, params.NextToken, params.MaxResults)
}

// matchesNotebookParams reports whether nb satisfies every filter in params.
func matchesNotebookParams(nb *NotebookInstance, p ListNotebookInstancesParams) bool {
	return matchesNotebookStringFilters(nb, p) && matchesNotebookTimeWindows(nb, p)
}

// matchesNotebookStringFilters checks every string-valued filter in params.
func matchesNotebookStringFilters(nb *NotebookInstance, p ListNotebookInstancesParams) bool {
	if p.StatusEquals != "" && !strings.EqualFold(nb.NotebookInstanceStatus, p.StatusEquals) {
		return false
	}

	if p.NameContains != "" &&
		!strings.Contains(strings.ToLower(nb.NotebookInstanceName), strings.ToLower(p.NameContains)) {
		return false
	}

	if p.AdditionalCodeRepositoryEquals != "" &&
		!slices.Contains(nb.AdditionalCodeRepositories, p.AdditionalCodeRepositoryEquals) {
		return false
	}

	if p.DefaultCodeRepositoryContains != "" &&
		!strings.Contains(nb.DefaultCodeRepository, p.DefaultCodeRepositoryContains) {
		return false
	}

	if p.NotebookInstanceLifecycleConfigNameContains != "" &&
		!strings.Contains(nb.LifecycleConfigName, p.NotebookInstanceLifecycleConfigNameContains) {
		return false
	}

	return true
}

// matchesNotebookTimeWindows checks the creation/last-modified time-window
// filters in params.
func matchesNotebookTimeWindows(nb *NotebookInstance, p ListNotebookInstancesParams) bool {
	if p.CreationTimeAfter != nil && !nb.CreationTime.After(*p.CreationTimeAfter) {
		return false
	}

	if p.CreationTimeBefore != nil && !nb.CreationTime.Before(*p.CreationTimeBefore) {
		return false
	}

	if p.LastModifiedTimeAfter != nil && !nb.LastModifiedTime.After(*p.LastModifiedTimeAfter) {
		return false
	}

	if p.LastModifiedTimeBefore != nil && !nb.LastModifiedTime.Before(*p.LastModifiedTimeBefore) {
		return false
	}

	return true
}

// notebookInstanceSortLess orders two notebook instances by sortBy — one of
// NotebookInstanceSortKey's real values (Name, CreationTime, Status;
// types/enums.go:6492-6498) — falling through to the name tiebreak for a
// stable order.
func notebookInstanceSortLess(a, b *NotebookInstance, sortBy string) bool {
	switch sortBy {
	case "CreationTime":
		if !a.CreationTime.Equal(b.CreationTime) {
			return a.CreationTime.Before(b.CreationTime)
		}
	case keyStatus:
		if a.NotebookInstanceStatus != b.NotebookInstanceStatus {
			return a.NotebookInstanceStatus < b.NotebookInstanceStatus
		}
	default:
		if a.NotebookInstanceName != b.NotebookInstanceName {
			return a.NotebookInstanceName < b.NotebookInstanceName
		}
	}

	return a.NotebookInstanceName < b.NotebookInstanceName
}

// DeleteNotebookInstance removes a notebook instance from the backend.
func (b *InMemoryBackend) DeleteNotebookInstance(ctx context.Context, name string) error {
	b.mu.Lock("DeleteNotebookInstance")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	if nb.NotebookInstanceStatus != notebookStatusStopped {
		return fmt.Errorf(
			"%w: notebook instance %q is in %s status and must be stopped before it can be deleted",
			ErrValidation, name, nb.NotebookInstanceStatus,
		)
	}

	arnIdx := b.notebookARNIndexStore(region)
	delete(arnIdx, nb.NotebookInstanceArn)
	store := b.notebooksStore(region)
	store.Delete(name)

	return nil
}

// StartNotebookInstance transitions a notebook instance to InService.
func (b *InMemoryBackend) StartNotebookInstance(ctx context.Context, name string) error {
	b.mu.Lock("StartNotebookInstance")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	nb.NotebookInstanceStatus = statusInService
	nb.LastModifiedTime = time.Now()

	return nil
}

// StopNotebookInstance transitions a notebook instance to Stopped.
func (b *InMemoryBackend) StopNotebookInstance(ctx context.Context, name string) error {
	b.mu.Lock("StopNotebookInstance")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	nb.NotebookInstanceStatus = notebookStatusStopped
	nb.LastModifiedTime = time.Now()

	return nil
}

// UpdateNotebookInstance updates a notebook instance's instance type.
func (b *InMemoryBackend) UpdateNotebookInstance(ctx context.Context, name, instanceType string) error {
	b.mu.Lock("UpdateNotebookInstance")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	if instanceType != "" {
		nb.InstanceType = instanceType
	}
	nb.LastModifiedTime = time.Now()

	return nil
}

// notebookInstanceURL builds the stable (non-presigned) Jupyter URL reported
// by DescribeNotebookInstance/NotebookInstanceSummary's Url field, and shared
// by CreatePresignedNotebookInstanceURL as the base it would otherwise
// presign.
func notebookInstanceURL(nbARN string) string {
	return "https://" + nbARN + ".notebook.sagemaker.aws/lab"
}

// CreatePresignedNotebookInstanceURL returns a presigned URL for a notebook instance.
func (b *InMemoryBackend) CreatePresignedNotebookInstanceURL(ctx context.Context, name string) (string, error) {
	b.mu.RLock("CreatePresignedNotebookInstanceURL")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStoreRO(region).Get(name)
	if !ok {
		return "", fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	return notebookInstanceURL(nb.NotebookInstanceArn), nil
}

// ---------------------------------------------------------------------------
// NotebookInstanceLifecycleConfig (#3)
// ---------------------------------------------------------------------------

// NotebookLifecycleHook is a single lifecycle script entry.
type NotebookLifecycleHook struct {
	Content string `json:"Content,omitempty"` // base64-encoded shell script
}

// NotebookInstanceLifecycleConfig stores Create/Start lifecycle scripts.
type NotebookInstanceLifecycleConfig struct {
	CreationTime     time.Time               `json:"CreationTime"`
	LastModifiedTime time.Time               `json:"LastModifiedTime"`
	Tags             map[string]string       `json:"Tags,omitempty"`
	Name             string                  `json:"NotebookInstanceLifecycleConfigName"`
	ARN              string                  `json:"NotebookInstanceLifecycleConfigArn"`
	OnCreate         []NotebookLifecycleHook `json:"OnCreate,omitempty"`
	OnStart          []NotebookLifecycleHook `json:"OnStart,omitempty"`
}

// cloneNotebookLifecycleConfig returns a deep copy.
func cloneNotebookLifecycleConfig(
	lc *NotebookInstanceLifecycleConfig,
) *NotebookInstanceLifecycleConfig {
	cp := *lc
	cp.OnCreate = make([]NotebookLifecycleHook, len(lc.OnCreate))
	copy(cp.OnCreate, lc.OnCreate)
	cp.OnStart = make([]NotebookLifecycleHook, len(lc.OnStart))
	copy(cp.OnStart, lc.OnStart)
	cp.Tags = maps.Clone(lc.Tags)

	return &cp
}

// CreateNotebookInstanceLifecycleConfig creates a new lifecycle config.
func (b *InMemoryBackend) CreateNotebookInstanceLifecycleConfig(
	ctx context.Context,
	name string,
	onCreate, onStart []NotebookLifecycleHook,
	tags map[string]string,
) (*NotebookInstanceLifecycleConfig, error) {
	b.mu.Lock("CreateNotebookInstanceLifecycleConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.notebookLifecycleConfigsStore(region).Get(name); ok {
		return nil, fmt.Errorf(
			"%w: notebook lifecycle config %s already exists",
			ErrNotebookLifecycleConfigAlreadyExists,
			name,
		)
	}

	lcARN := arn.Build(
		"sagemaker",
		region,
		b.accountID,
		"notebook-instance-lifecycle-config/"+name,
	)
	now := time.Now()
	lc := &NotebookInstanceLifecycleConfig{
		Name:             name,
		ARN:              lcARN,
		OnCreate:         onCreate,
		OnStart:          onStart,
		CreationTime:     now,
		LastModifiedTime: now,
		Tags:             mergeTags(nil, tags),
	}
	b.notebookLifecycleConfigsStore(region).Put(lc)

	return cloneNotebookLifecycleConfig(lc), nil
}

// DescribeNotebookInstanceLifecycleConfig returns a lifecycle config by name.
func (b *InMemoryBackend) DescribeNotebookInstanceLifecycleConfig(
	ctx context.Context,
	name string,
) (*NotebookInstanceLifecycleConfig, error) {
	b.mu.RLock("DescribeNotebookInstanceLifecycleConfig")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	lc, ok := b.notebookLifecycleConfigsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf(
			"%w: notebook lifecycle config %q not found",
			ErrNotebookLifecycleConfigNotFound,
			name,
		)
	}

	return cloneNotebookLifecycleConfig(lc), nil
}

// UpdateNotebookInstanceLifecycleConfig replaces onCreate/onStart scripts.
func (b *InMemoryBackend) UpdateNotebookInstanceLifecycleConfig(
	ctx context.Context,
	name string,
	onCreate, onStart []NotebookLifecycleHook,
) (*NotebookInstanceLifecycleConfig, error) {
	b.mu.Lock("UpdateNotebookInstanceLifecycleConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	lc, ok := b.notebookLifecycleConfigsStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf(
			"%w: notebook lifecycle config %q not found",
			ErrNotebookLifecycleConfigNotFound,
			name,
		)
	}

	if onCreate != nil {
		lc.OnCreate = onCreate
	}
	if onStart != nil {
		lc.OnStart = onStart
	}
	lc.LastModifiedTime = time.Now()

	return cloneNotebookLifecycleConfig(lc), nil
}

// DeleteNotebookInstanceLifecycleConfig removes a lifecycle config.
func (b *InMemoryBackend) DeleteNotebookInstanceLifecycleConfig(ctx context.Context, name string) error {
	b.mu.Lock("DeleteNotebookInstanceLifecycleConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	store := b.notebookLifecycleConfigsStore(region)

	if _, ok := store.Get(name); !ok {
		return fmt.Errorf(
			"%w: notebook lifecycle config %q not found",
			ErrNotebookLifecycleConfigNotFound,
			name,
		)
	}

	store.Delete(name)

	return nil
}

// ListNotebookInstanceLifecycleConfigsParams bundles
// ListNotebookInstanceLifecycleConfigs' filter/sort/pagination criteria
// (api_op_ListNotebookInstanceLifecycleConfigs.go:32-69, sagemaker@v1.263.2).
type ListNotebookInstanceLifecycleConfigsParams struct {
	CreationTimeAfter      *time.Time
	CreationTimeBefore     *time.Time
	LastModifiedTimeAfter  *time.Time
	LastModifiedTimeBefore *time.Time
	NameContains           string
	NextToken              string
	SortBy                 string
	SortOrder              string
	MaxResults             int32
}

// ListNotebookInstanceLifecycleConfigs returns lifecycle configs matching
// params, sorted by params.SortBy (default CreationTime, per
// api_op_ListNotebookInstanceLifecycleConfigs.go:62) / params.SortOrder (no
// default documented, Ascending kept as the disclosed fallback), capped at
// params.MaxResults.
func (b *InMemoryBackend) ListNotebookInstanceLifecycleConfigs(
	ctx context.Context,
	params ListNotebookInstanceLifecycleConfigsParams,
) ([]*NotebookInstanceLifecycleConfig, string) {
	b.mu.RLock("ListNotebookInstanceLifecycleConfigs")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	store := b.notebookLifecycleConfigsStoreRO(region)
	list := make([]*NotebookInstanceLifecycleConfig, 0, store.Len())
	for _, lc := range store.All() {
		if !matchesLifecycleConfigParams(lc, params) {
			continue
		}

		list = append(list, cloneNotebookLifecycleConfig(lc))
	}

	desc := strings.EqualFold(params.SortOrder, sortOrderDescending)
	sort.Slice(list, func(i, j int) bool {
		less := lifecycleConfigSortLess(list[i], list[j], params.SortBy)
		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, params.NextToken, params.MaxResults)
}

// matchesLifecycleConfigParams reports whether lc satisfies every filter in params.
func matchesLifecycleConfigParams(
	lc *NotebookInstanceLifecycleConfig,
	p ListNotebookInstanceLifecycleConfigsParams,
) bool {
	if p.NameContains != "" && !strings.Contains(lc.Name, p.NameContains) {
		return false
	}

	if p.CreationTimeAfter != nil && !lc.CreationTime.After(*p.CreationTimeAfter) {
		return false
	}

	if p.CreationTimeBefore != nil && !lc.CreationTime.Before(*p.CreationTimeBefore) {
		return false
	}

	if p.LastModifiedTimeAfter != nil && !lc.LastModifiedTime.After(*p.LastModifiedTimeAfter) {
		return false
	}

	if p.LastModifiedTimeBefore != nil && !lc.LastModifiedTime.Before(*p.LastModifiedTimeBefore) {
		return false
	}

	return true
}

// lifecycleConfigSortLess orders two lifecycle configs by sortBy — one of
// NotebookInstanceLifecycleConfigSortKey's real values (Name, CreationTime,
// LastModifiedTime; types/enums.go:6450-6456) — falling through to the name
// tiebreak for a stable order.
func lifecycleConfigSortLess(a, b *NotebookInstanceLifecycleConfig, sortBy string) bool {
	switch sortBy {
	case keyGenericName:
		if a.Name != b.Name {
			return a.Name < b.Name
		}
	case keyLastModifiedTime:
		if !a.LastModifiedTime.Equal(b.LastModifiedTime) {
			return a.LastModifiedTime.Before(b.LastModifiedTime)
		}
	default:
		if !a.CreationTime.Equal(b.CreationTime) {
			return a.CreationTime.Before(b.CreationTime)
		}
	}

	return a.Name < b.Name
}

// ---------------------------------------------------------------------------
// Notebook lifecycle FSM simulator (#2)
// ---------------------------------------------------------------------------

// scheduleNotebookTransition asynchronously transitions a notebook to nextStatus after delay.
// Must be called while holding b.mu (runDelayed captures the lifecycle context).
func (b *InMemoryBackend) scheduleNotebookTransition(
	ctx context.Context,
	name, nextStatus string,
	delay time.Duration,
) {
	region := getRegion(ctx, b.region)
	b.runDelayed(ctx, delay, func() {
		b.mu.Lock("scheduleNotebookTransition.goroutine")
		defer b.mu.Unlock()

		if nb, ok := b.notebooksStore(region).Get(name); ok {
			nb.NotebookInstanceStatus = nextStatus
			nb.LastModifiedTime = time.Now()
		}
	})
}

// StartNotebookInstanceFSM transitions: Stopped → Pending, then Pending → InService.
func (b *InMemoryBackend) StartNotebookInstanceFSM(ctx context.Context, name string) error {
	b.mu.Lock("StartNotebookInstanceFSM")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	if nb.NotebookInstanceStatus != notebookStatusStopped {
		return fmt.Errorf(
			"%w: notebook %q is not Stopped (status=%s)",
			ErrValidation,
			name,
			nb.NotebookInstanceStatus,
		)
	}

	nb.NotebookInstanceStatus = notebookStatusPending
	nb.LastModifiedTime = time.Now()
	b.scheduleNotebookTransition(
		b.lifecycleCtx,
		name,
		statusInService,
		notebookPendingToInServiceDelay,
	)

	return nil
}

// StopNotebookInstanceFSM transitions: InService → Stopping, then Stopping → Stopped.
func (b *InMemoryBackend) StopNotebookInstanceFSM(ctx context.Context, name string) error {
	b.mu.Lock("StopNotebookInstanceFSM")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	if nb.NotebookInstanceStatus != statusInService {
		return fmt.Errorf(
			"%w: notebook %q is not InService (status=%s)",
			ErrValidation,
			name,
			nb.NotebookInstanceStatus,
		)
	}

	nb.NotebookInstanceStatus = notebookStatusStopping
	nb.LastModifiedTime = time.Now()
	b.scheduleNotebookTransition(b.lifecycleCtx, name, notebookStatusStopped, notebookStoppingToStoppedDelay)

	return nil
}

// CreateNotebookInstanceFSM creates a notebook and immediately schedules Pending → InService.
func (b *InMemoryBackend) CreateNotebookInstanceFSM(
	ctx context.Context,
	opts NotebookInstanceOptions,
) (*NotebookInstance, error) {
	b.mu.RLock("CreateNotebookInstanceFSM.ctx")
	lifecycleCtx := b.lifecycleCtx
	b.mu.RUnlock()

	nb, err := b.CreateNotebookInstanceFull(ctx, opts)
	if err != nil {
		return nil, err
	}
	b.scheduleNotebookTransition(lifecycleCtx, opts.Name, statusInService, notebookPendingToInServiceDelay)

	return nb, nil
}

// UpdateNotebookInstanceFull updates all mutable fields on a notebook.
func (b *InMemoryBackend) UpdateNotebookInstanceFull(
	ctx context.Context,
	name string,
	opts NotebookUpdateOptions,
) error {
	b.mu.Lock("UpdateNotebookInstanceFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	nb, ok := b.notebooksStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: notebook instance %q not found", ErrNotebookNotFound, name)
	}

	if nb.NotebookInstanceStatus != notebookStatusStopped {
		return fmt.Errorf(
			"%w: notebook instance %q is in %s status and cannot be updated",
			ErrValidation, name, nb.NotebookInstanceStatus,
		)
	}

	applyNotebookUpdateOptions(nb, opts)
	nb.LastModifiedTime = time.Now()

	return nil
}

// applyNotebookUpdateOptions mutates nb in place per opts, following
// UpdateNotebookInstanceInput's own semantics: a Disassociate* flag clears
// its field, otherwise a non-empty/non-nil value replaces it and a
// zero-value one leaves the existing field unchanged.
func applyNotebookUpdateOptions(nb *NotebookInstance, opts NotebookUpdateOptions) {
	if opts.InstanceType != "" {
		nb.InstanceType = opts.InstanceType
	}
	if opts.RoleArn != "" {
		nb.RoleArn = opts.RoleArn
	}
	if opts.LifecycleConfigName != "" {
		nb.LifecycleConfigName = opts.LifecycleConfigName
	}
	if opts.DisassociateLifecycleConfig {
		nb.LifecycleConfigName = ""
	}
	if opts.VolumeSizeInGB > 0 {
		nb.VolumeSizeInGB = opts.VolumeSizeInGB
	}
	if opts.DefaultCodeRepository != "" {
		nb.DefaultCodeRepository = opts.DefaultCodeRepository
	}
	if opts.DisassociateDefaultCodeRepository {
		nb.DefaultCodeRepository = ""
	}
	if opts.AdditionalCodeRepositories != nil {
		nb.AdditionalCodeRepositories = opts.AdditionalCodeRepositories
	}
	if opts.DisassociateAdditionalCodeRepositories {
		nb.AdditionalCodeRepositories = nil
	}
	if opts.PlatformIdentifier != "" {
		nb.PlatformIdentifier = opts.PlatformIdentifier
	}
	if opts.RootAccess != "" {
		nb.RootAccess = opts.RootAccess
	}
	if opts.IPAddressType != "" {
		nb.IPAddressType = opts.IPAddressType
	}
	if opts.MinimumInstanceMetadataServiceVersion != "" {
		nb.MinimumInstanceMetadataServiceVersion = opts.MinimumInstanceMetadataServiceVersion
	}
}

// NotebookUpdateOptions holds mutable fields for UpdateNotebookInstance.
type NotebookUpdateOptions struct {
	InstanceType                           string
	RoleArn                                string
	LifecycleConfigName                    string
	DefaultCodeRepository                  string
	PlatformIdentifier                     string
	RootAccess                             string
	IPAddressType                          string
	MinimumInstanceMetadataServiceVersion  string
	AdditionalCodeRepositories             []string
	VolumeSizeInGB                         int32
	DisassociateLifecycleConfig            bool
	DisassociateDefaultCodeRepository      bool
	DisassociateAdditionalCodeRepositories bool
}

// ---------------------------------------------------------------------------
// NotebookInstanceOptions for gap #1 (full field set)
// ---------------------------------------------------------------------------

// NotebookInstanceOptions holds all CreateNotebookInstance request fields.
type NotebookInstanceOptions struct {
	Tags                                  map[string]string `json:"Tags,omitempty"`
	SubnetID                              string            `json:"SubnetId,omitempty"`
	LifecycleConfigName                   string            `json:"LifecycleConfigName,omitempty"`
	Name                                  string            `json:"NotebookInstanceName"`
	InstanceType                          string            `json:"InstanceType"`
	RoleArn                               string            `json:"RoleArn"`
	RootAccess                            string            `json:"RootAccess,omitempty"`
	KmsKeyID                              string            `json:"KmsKeyId,omitempty"`
	DirectInternetAccess                  string            `json:"DirectInternetAccess,omitempty"`
	DefaultCodeRepository                 string            `json:"DefaultCodeRepository,omitempty"`
	PlatformIdentifier                    string            `json:"PlatformIdentifier,omitempty"`
	IPAddressType                         string            `json:"IpAddressType,omitempty"`
	MinimumInstanceMetadataServiceVersion string            `json:"MinimumInstanceMetadataServiceVersion,omitempty"`
	AcceleratorTypes                      []string          `json:"AcceleratorTypes,omitempty"`
	AdditionalCodeRepositories            []string          `json:"AdditionalCodeRepositories,omitempty"`
	SecurityGroupIDs                      []string          `json:"SecurityGroupIds,omitempty"`
	VolumeSizeInGB                        int32             `json:"VolumeSizeInGB,omitempty"`
}

// CreateNotebookInstanceFull persists all NotebookInstanceOptions fields.
func (b *InMemoryBackend) CreateNotebookInstanceFull(
	ctx context.Context,
	opts NotebookInstanceOptions,
) (*NotebookInstance, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("%w: NotebookInstanceName is required", ErrValidation)
	}
	if opts.InstanceType == "" {
		return nil, fmt.Errorf("%w: InstanceType is required", ErrValidation)
	}
	if opts.RoleArn == "" {
		return nil, fmt.Errorf("%w: RoleArn is required", ErrValidation)
	}

	b.mu.Lock("CreateNotebookInstanceFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.notebooksStore(region).Get(opts.Name); ok {
		return nil, fmt.Errorf(
			"%w: notebook instance %s already exists",
			ErrNotebookAlreadyExists,
			opts.Name,
		)
	}

	nbARN := arn.Build("sagemaker", region, b.accountID, "notebook-instance/"+opts.Name)
	now := time.Now()
	nb := &NotebookInstance{
		NotebookInstanceName:                  opts.Name,
		NotebookInstanceArn:                   nbARN,
		URL:                                   notebookInstanceURL(nbARN),
		NotebookInstanceStatus:                "Pending",
		InstanceType:                          opts.InstanceType,
		RoleArn:                               opts.RoleArn,
		SubnetID:                              opts.SubnetID,
		SecurityGroupIDs:                      append([]string(nil), opts.SecurityGroupIDs...),
		KmsKeyID:                              opts.KmsKeyID,
		LifecycleConfigName:                   opts.LifecycleConfigName,
		DirectInternetAccess:                  opts.DirectInternetAccess,
		RootAccess:                            opts.RootAccess,
		AcceleratorTypes:                      append([]string(nil), opts.AcceleratorTypes...),
		AdditionalCodeRepositories:            append([]string(nil), opts.AdditionalCodeRepositories...),
		DefaultCodeRepository:                 opts.DefaultCodeRepository,
		VolumeSizeInGB:                        opts.VolumeSizeInGB,
		PlatformIdentifier:                    opts.PlatformIdentifier,
		IPAddressType:                         opts.IPAddressType,
		MinimumInstanceMetadataServiceVersion: opts.MinimumInstanceMetadataServiceVersion,
		CreationTime:                          now,
		LastModifiedTime:                      now,
		Tags:                                  mergeTags(nil, opts.Tags),
	}
	b.notebooksStore(region).Put(nb)
	b.notebookARNIndexStore(region)[nbARN] = opts.Name

	return cloneNotebook(nb), nil
}
