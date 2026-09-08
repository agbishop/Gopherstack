package backup

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateRestoreTestingPlan creates a restore testing plan.
// RecoveryPointSelection is required on the real RestoreTestingPlanForCreate
// (validators.go:2400-2419 rejects a nil value) and on RestoreTestingPlanForGet,
// so it must always come out non-nil -- if the caller passed nil (only
// reachable via a raw request bypassing the SDK's own client-side
// validation), a genuinely-empty selection is stored instead of leaving the
// required member absent.
func (b *InMemoryBackend) CreateRestoreTestingPlan(
	name, scheduleExpression string,
	startWindowHours int64,
	recoveryPointSelection *RestoreTestingRecoveryPointSelection,
) (*RestoreTestingPlan, error) {
	b.mu.Lock("CreateRestoreTestingPlan")
	defer b.mu.Unlock()

	if b.restoreTestingPlans.Has(name) {
		return nil, fmt.Errorf("%w: restore testing plan %s already exists", ErrAlreadyExists, name)
	}

	if recoveryPointSelection == nil {
		recoveryPointSelection = &RestoreTestingRecoveryPointSelection{}
	}

	planARN := arn.Build("backup", b.region, b.accountID, "restore-testing-plan:"+name)
	rtp := &RestoreTestingPlan{
		RestoreTestingPlanName: name,
		RestoreTestingPlanArn:  planARN,
		ScheduleExpression:     scheduleExpression,
		RecoveryPointSelection: recoveryPointSelection,
		StartWindowHours:       startWindowHours,
		CreationTime:           time.Now().UTC(),
	}
	b.restoreTestingPlans.Put(rtp)
	cp := *rtp

	return &cp, nil
}

// CreateRestoreTestingSelection creates a selection within a restore
// testing plan. IAMRoleArn and ProtectedResourceType are required by the
// real RestoreTestingSelectionForCreate shape.
func (b *InMemoryBackend) CreateRestoreTestingSelection(
	planName, selectionName string,
	in RestoreTestingSelectionInput,
) (*RestoreTestingSelection, error) {
	b.mu.Lock("CreateRestoreTestingSelection")
	defer b.mu.Unlock()

	if in.IAMRoleArn == "" {
		return nil, fmt.Errorf("%w: IamRoleArn is required", ErrValidation)
	}
	if in.ProtectedResourceType == "" {
		return nil, fmt.Errorf("%w: ProtectedResourceType is required", ErrValidation)
	}

	rtp, found := b.restoreTestingPlans.Get(planName)
	if !found {
		return nil, fmt.Errorf("%w: restore testing plan %s not found", ErrNotFound, planName)
	}

	if b.restoreTestingSelections.Has(restoreTestingSelectionKey(planName, selectionName)) {
		return nil, fmt.Errorf(
			"%w: restore testing selection %s already exists",
			ErrAlreadyExists,
			selectionName,
		)
	}

	sel := &RestoreTestingSelection{
		RestoreTestingPlanName:      planName,
		RestoreTestingSelectionName: selectionName,
		RestoreTestingPlanArn:       rtp.RestoreTestingPlanArn,
		ProtectedResourceType:       in.ProtectedResourceType,
		IAMRoleArn:                  in.IAMRoleArn,
		ProtectedResourceArns:       in.ProtectedResourceArns,
		ProtectedResourceConditions: in.ProtectedResourceConditions,
		RestoreMetadataOverrides:    in.RestoreMetadataOverrides,
		ValidationWindowHours:       in.ValidationWindowHours,
		CreationTime:                time.Now().UTC(),
	}
	b.restoreTestingSelections.Put(sel)
	cp := *sel

	return &cp, nil
}

// GetRestoreTestingPlan returns a restore testing plan by name.
func (b *InMemoryBackend) GetRestoreTestingPlan(planName string) (*RestoreTestingPlan, error) {
	b.mu.RLock("GetRestoreTestingPlan")
	defer b.mu.RUnlock()

	rtp, ok := b.restoreTestingPlans.Get(planName)
	if !ok {
		return nil, fmt.Errorf("%w: restore testing plan %s not found", ErrNotFound, planName)
	}

	cp := *rtp

	return &cp, nil
}

// ListRestoreTestingPlans returns all restore testing plans.
func (b *InMemoryBackend) ListRestoreTestingPlans() []*RestoreTestingPlan {
	b.mu.RLock("ListRestoreTestingPlans")
	defer b.mu.RUnlock()

	all := b.restoreTestingPlans.All()
	list := make([]*RestoreTestingPlan, 0, len(all))
	for _, rtp := range all {
		cp := *rtp
		list = append(list, &cp)
	}

	slices.SortFunc(list, func(a, b *RestoreTestingPlan) int {
		if a.RestoreTestingPlanName < b.RestoreTestingPlanName {
			return -1
		}
		if a.RestoreTestingPlanName > b.RestoreTestingPlanName {
			return 1
		}

		return 0
	})

	return list
}

// UpdateRestoreTestingPlan updates a restore testing plan.
// RecoveryPointSelection is optional on the real RestoreTestingPlanForUpdate
// (types.go:2431-2456, no "required" marker), so an omitted (nil) value
// leaves the plan's existing selection unchanged rather than clearing it.
func (b *InMemoryBackend) UpdateRestoreTestingPlan(
	planName, scheduleExpression string,
	startWindowHours int64,
	recoveryPointSelection *RestoreTestingRecoveryPointSelection,
) (*RestoreTestingPlan, error) {
	b.mu.Lock("UpdateRestoreTestingPlan")
	defer b.mu.Unlock()

	rtp, ok := b.restoreTestingPlans.Get(planName)
	if !ok {
		return nil, fmt.Errorf("%w: restore testing plan %s not found", ErrNotFound, planName)
	}

	rtp.ScheduleExpression = scheduleExpression
	if startWindowHours > 0 {
		rtp.StartWindowHours = startWindowHours
	}
	if recoveryPointSelection != nil {
		rtp.RecoveryPointSelection = recoveryPointSelection
	}
	cp := *rtp

	return &cp, nil
}

// DeleteRestoreTestingPlan deletes a restore testing plan. Real AWS
// (api_op_DeleteRestoreTestingPlan.go) says deletion can only succeed if all
// associated restore testing selections are deleted first.
func (b *InMemoryBackend) DeleteRestoreTestingPlan(planName string) error {
	b.mu.Lock("DeleteRestoreTestingPlan")
	defer b.mu.Unlock()

	if !b.restoreTestingPlans.Has(planName) {
		// DeleteRestoreTestingPlan's own deserializeOpError switch (unlike
		// almost every sibling op) declares no ResourceNotFoundException
		// case at all -- InvalidRequestException is the only client-fault
		// type available for this operation.
		return fmt.Errorf("%w: restore testing plan %s not found", ErrInvalidRequest, planName)
	}

	if sels := b.restoreTestingSelectionsByPlan.Get(planName); len(sels) > 0 {
		return fmt.Errorf(
			"%w: restore testing plan %s has %d active selection(s); delete them first",
			ErrInvalidRequest, planName, len(sels),
		)
	}

	b.restoreTestingPlans.Delete(planName)

	return nil
}

// GetRestoreTestingSelection returns a specific restore testing selection.
func (b *InMemoryBackend) GetRestoreTestingSelection(
	planName, selectionName string,
) (*RestoreTestingSelection, error) {
	b.mu.RLock("GetRestoreTestingSelection")
	defer b.mu.RUnlock()

	if !b.restoreTestingPlans.Has(planName) {
		return nil, fmt.Errorf("%w: restore testing plan %s not found", ErrNotFound, planName)
	}

	sel, ok := b.restoreTestingSelections.Get(restoreTestingSelectionKey(planName, selectionName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: restore testing selection %s not found",
			ErrNotFound,
			selectionName,
		)
	}

	cp := *sel

	return &cp, nil
}

// ListRestoreTestingSelections returns all selections for a restore testing plan.
func (b *InMemoryBackend) ListRestoreTestingSelections(
	planName string,
) ([]*RestoreTestingSelection, error) {
	b.mu.RLock("ListRestoreTestingSelections")
	defer b.mu.RUnlock()

	if !b.restoreTestingPlans.Has(planName) {
		return nil, fmt.Errorf("%w: restore testing plan %s not found", ErrNotFound, planName)
	}

	sels := b.restoreTestingSelectionsByPlan.Get(planName)
	list := make([]*RestoreTestingSelection, 0, len(sels))
	for _, sel := range sels {
		cp := *sel
		list = append(list, &cp)
	}

	slices.SortFunc(list, func(a, b *RestoreTestingSelection) int {
		if a.RestoreTestingSelectionName < b.RestoreTestingSelectionName {
			return -1
		}
		if a.RestoreTestingSelectionName > b.RestoreTestingSelectionName {
			return 1
		}

		return 0
	})

	return list, nil
}

// UpdateRestoreTestingSelection updates a restore testing selection.
// RestoreTestingSelectionForUpdate has no required fields beyond identity,
// so every field here is set from in verbatim (a full-replace PUT, matching
// how PutBackupVaultAccessPolicy/PutBackupVaultLockConfiguration etc. treat
// their bodies elsewhere in this service) -- ProtectedResourceType itself
// is immutable on Update per the real API, so it is intentionally not
// touched here.
func (b *InMemoryBackend) UpdateRestoreTestingSelection(
	planName, selectionName string,
	in RestoreTestingSelectionInput,
) (*RestoreTestingSelection, error) {
	b.mu.Lock("UpdateRestoreTestingSelection")
	defer b.mu.Unlock()

	if !b.restoreTestingPlans.Has(planName) {
		return nil, fmt.Errorf("%w: restore testing plan %s not found", ErrNotFound, planName)
	}

	sel, ok := b.restoreTestingSelections.Get(restoreTestingSelectionKey(planName, selectionName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: restore testing selection %s not found",
			ErrNotFound,
			selectionName,
		)
	}

	sel.IAMRoleArn = in.IAMRoleArn
	sel.ProtectedResourceArns = in.ProtectedResourceArns
	sel.ProtectedResourceConditions = in.ProtectedResourceConditions
	sel.RestoreMetadataOverrides = in.RestoreMetadataOverrides
	sel.ValidationWindowHours = in.ValidationWindowHours
	cp := *sel

	return &cp, nil
}

// DeleteRestoreTestingSelection deletes a restore testing selection.
func (b *InMemoryBackend) DeleteRestoreTestingSelection(planName, selectionName string) error {
	b.mu.Lock("DeleteRestoreTestingSelection")
	defer b.mu.Unlock()

	if !b.restoreTestingPlans.Has(planName) {
		return fmt.Errorf("%w: restore testing plan %s not found", ErrNotFound, planName)
	}

	key := restoreTestingSelectionKey(planName, selectionName)
	if !b.restoreTestingSelections.Has(key) {
		return fmt.Errorf("%w: restore testing selection %s not found", ErrNotFound, selectionName)
	}

	b.restoreTestingSelections.Delete(key)

	return nil
}

// --- Framework read/update/delete methods ---

// scanJobResourceName derives ResourceName from a resource ARN's trailing
// segment (after the last "/" or ":") -- the same non-fabricating,
// derive-from-already-stored-state approach this campaign used for
// bedrock's SourceAccountId. ResourceName itself is not tracked anywhere
// else in this backend.
func scanJobResourceName(resourceArn string) string {
	if i := strings.LastIndexAny(resourceArn, "/:"); i >= 0 {
		return resourceArn[i+1:]
	}

	return resourceArn
}

// StartScanJob creates a new scan job for a backup vault.
// ResourceArn/ResourceType are derived from the recovery point
// input.RecoveryPointArn identifies, when this backend is tracking one --
// left absent (not fabricated) when it isn't.
func (b *InMemoryBackend) StartScanJob(backupVaultArn string, input StartScanJobInput) *ScanJob {
	b.mu.Lock("StartScanJob")
	defer b.mu.Unlock()

	var resourceArn, resourceType string
	if rp, ok := b.findRecoveryPointByArn(input.RecoveryPointArn); ok {
		resourceArn = rp.ResourceArn
		resourceType = rp.ResourceType
	}

	now := time.Now().UTC()
	done := now
	job := &ScanJob{
		ScanJobID:                "scan-job-" + uuid.NewString()[:8],
		BackupVaultArn:           backupVaultArn,
		BackupVaultName:          input.BackupVaultName,
		Status:                   statusCompleted,
		CreationTime:             now,
		CompletionTime:           &done,
		IamRoleArn:               input.IamRoleArn,
		MalwareScanner:           input.MalwareScanner,
		RecoveryPointArn:         input.RecoveryPointArn,
		ResourceArn:              resourceArn,
		ResourceName:             scanJobResourceName(resourceArn),
		ResourceType:             resourceType,
		ScanMode:                 input.ScanMode,
		ScannerRoleArn:           input.ScannerRoleArn,
		AccountID:                b.accountID,
		ContinuousScanEndTime:    input.ContinuousScanEndTime,
		IdempotencyToken:         input.IdempotencyToken,
		ScanBaseRecoveryPointArn: input.ScanBaseRecoveryPointArn,
	}
	b.scanJobs.Put(job)

	return job
}

// DescribeScanJob returns a scan job by ID.
func (b *InMemoryBackend) DescribeScanJob(scanJobID string) (*ScanJob, error) {
	b.mu.RLock("DescribeScanJob")
	defer b.mu.RUnlock()

	job, ok := b.scanJobs.Get(scanJobID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errScanJobNotFound, scanJobID)
	}

	return job, nil
}

// ListScanJobs returns all scan jobs.
func (b *InMemoryBackend) ListScanJobs() []*ScanJob {
	b.mu.RLock("ListScanJobs")
	defer b.mu.RUnlock()

	all := b.scanJobs.All()
	out := make([]*ScanJob, 0, len(all))
	for _, j := range all {
		cp := *j
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ScanJobID < out[j].ScanJobID })

	return out
}

// ListScanJobSummaries returns scan job counts grouped by State, real
// ScanJobSummary's own required grouping key (backup@v1.59.4
// api_op_ListScanJobSummaries.go, ScanJobSummary: AccountId, Count, Region,
// ResourceType, ScanResultStatus, State, StartTime, EndTime).
// AggregationPeriod (per-day/per-week time-bucketed counts),
// ResourceType-level grouping, and MalwareScanner/ScanResultStatus (this
// backend's ScanJob never tracks a scan result outcome, see the ScanJob
// type doc) are not modeled -- kept consistent with the same State-only
// grouping precedent ListBackupJobSummaries/ListCopyJobSummaries already
// use for their own sibling ops.
func (b *InMemoryBackend) ListScanJobSummaries() []map[string]any {
	b.mu.RLock("ListScanJobSummaries")
	defer b.mu.RUnlock()

	counts := make(map[string]int)
	for _, j := range b.scanJobs.All() {
		counts[j.Status]++
	}

	summaries := make([]map[string]any, 0, len(counts))
	for state, count := range counts {
		summaries = append(summaries, map[string]any{
			keyState:         state,
			keySummaryCount:  count,
			keySummaryRegion: b.region,
			keyAccountID:     b.accountID,
		})
	}

	return summaries
}

// ListScanJobsFilter contains optional filter parameters for listing scan
// jobs, mirroring ListScanJobsInput (api_op_ListScanJobs.go, backup@v1.59.4).
// ByScanResultStatus is not included: this backend's ScanJob has no field
// to hold a scan result status (StartScanJob never receives or fabricates
// one).
type ListScanJobsFilter struct {
	CompleteAfter    *time.Time
	CompleteBefore   *time.Time
	AccountID        string
	BackupVaultName  string
	MalwareScanner   string
	RecoveryPointArn string
	ResourceArn      string
	ResourceType     string
	State            string
	NextToken        string
	MaxResults       int
}

// scanJobAccountMatches implements ByAccountId (api_op_ListScanJobs.go):
// "If used from an Amazon Web Services Organizations management account,
// passing * returns all jobs across the organization" -- "*" is a wildcard,
// not a literal account ID.
func scanJobAccountMatches(j *ScanJob, f ListScanJobsFilter) bool {
	return f.AccountID == "" || f.AccountID == wildcardAccountID || j.AccountID == f.AccountID
}

func scanJobMatchesFieldFilters(j *ScanJob, f ListScanJobsFilter) bool {
	if !scanJobAccountMatches(j, f) {
		return false
	}

	switch {
	case f.BackupVaultName != "" && j.BackupVaultName != f.BackupVaultName:
		return false
	case f.MalwareScanner != "" && j.MalwareScanner != f.MalwareScanner:
		return false
	case f.RecoveryPointArn != "" && j.RecoveryPointArn != f.RecoveryPointArn:
		return false
	case f.ResourceArn != "" && j.ResourceArn != f.ResourceArn:
		return false
	case f.ResourceType != "" && j.ResourceType != f.ResourceType:
		return false
	case f.State != "" && j.Status != f.State:
		return false
	}

	return true
}

func scanJobMatchesFilter(j *ScanJob, f ListScanJobsFilter) bool {
	if !scanJobMatchesFieldFilters(j, f) {
		return false
	}

	if j.CompletionTime == nil {
		return f.CompleteAfter == nil && f.CompleteBefore == nil
	}

	return inTimeRange(*j.CompletionTime, f.CompleteAfter, f.CompleteBefore)
}

// ListScanJobsFiltered returns scan jobs matching the filter.
func (b *InMemoryBackend) ListScanJobsFiltered(f ListScanJobsFilter) ([]*ScanJob, string) {
	b.mu.RLock("ListScanJobsFiltered")
	defer b.mu.RUnlock()

	all := b.scanJobs.All()
	out := make([]*ScanJob, 0, len(all))
	for _, j := range all {
		if !scanJobMatchesFilter(j, f) {
			continue
		}
		cp := *j
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ScanJobID < out[j].ScanJobID })

	return paginateByID(out, func(j *ScanJob) string { return j.ScanJobID }, f.MaxResults, f.NextToken)
}

// ---- Legal Holds ----
