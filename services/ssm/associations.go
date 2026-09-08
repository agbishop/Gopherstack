package ssm

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func (b *InMemoryBackend) associationsStore(region string) *store.Table[Association] {
	return getOrCreateTable(b, b.associations, "associations", region, associationKeyFn)
}
func (b *InMemoryBackend) associationExecutionsStore(
	region string,
) map[string][]AssociationExecution {
	return b.associationExecutions[region]
}
func (b *InMemoryBackend) associationExecTargetsStore(
	region string,
) map[string][]AssociationExecutionTarget {
	return b.associationExecTargets[region]
}

// copyAssocParameters deep copies a parameters map for associations.
func copyAssocParameters(src map[string][]string) map[string][]string {
	if src == nil {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for k, v := range src {
		dst[k] = append([]string(nil), v...)
	}

	return dst
}

// copyAssocTargets deep copies a targets slice for associations.
func copyAssocTargets(src []AssociationTarget) []AssociationTarget {
	if src == nil {
		return nil
	}
	dst := make([]AssociationTarget, len(src))
	for i, t := range src {
		dst[i] = AssociationTarget{
			Key:    t.Key,
			Values: append([]string(nil), t.Values...),
		}
	}

	return dst
}

// CreateAssociation creates a new association between a document and targets.
func (b *InMemoryBackend) CreateAssociation(
	ctx context.Context,
	input *CreateAssociationInput,
) (*CreateAssociationOutput, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidationException)
	}

	if err := validateMaxConcurrency(input.MaxConcurrency); err != nil {
		return nil, err
	}

	if err := validateMaxErrors(input.MaxErrors); err != nil {
		return nil, err
	}

	region := getRegion(ctx)
	b.mu.Lock("CreateAssociation")
	defer b.mu.Unlock()

	if !b.documentsStore(region).Has(input.Name) {
		return nil, ErrDocumentNotFound
	}

	assocID := uuid.NewString()
	now := UnixTimeFloat(time.Now())

	assoc := Association{
		AssociationID:                 assocID,
		AssociationName:               input.AssociationName,
		DocumentVersion:               input.DocumentVersion,
		InstanceID:                    input.InstanceID,
		Name:                          input.Name,
		Parameters:                    copyAssocParameters(input.Parameters),
		Targets:                       copyAssocTargets(input.Targets),
		Overview:                      &AssociationOverview{Status: assocStatusSuccess},
		LastUpdateAssociationDate:     now,
		ApplyOnlyAtCronInterval:       input.ApplyOnlyAtCronInterval,
		AssociationDispatchAssumeRole: input.AssociationDispatchAssumeRole,
		AutomationTargetParameterName: input.AutomationTargetParameterName,
		CalendarNames:                 append([]string(nil), input.CalendarNames...),
		ComplianceSeverity:            input.ComplianceSeverity,
		Duration:                      input.Duration,
		MaxConcurrency:                input.MaxConcurrency,
		MaxErrors:                     input.MaxErrors,
		OutputLocation:                copyAssocOutputLocation(input.OutputLocation),
		ScheduleExpression:            input.ScheduleExpression,
		SyncCompliance:                input.SyncCompliance,
		AssociationVersion:            "1",
	}

	b.associationsStore(region).Put(&assoc)
	b.recordAssociationExecutionLocked(region, assoc)

	if len(input.Tags) > 0 {
		if b.miscResourceTags[region] == nil {
			b.miscResourceTags[region] = make(map[string]map[string]string)
		}
		miscTags := b.miscResourceTagsStore(region)
		if miscTags[assocID] == nil {
			miscTags[assocID] = make(map[string]string)
		}
		for _, t := range input.Tags {
			miscTags[assocID][t.Key] = t.Value
		}
	}

	return &CreateAssociationOutput{AssociationDescription: assoc}, nil
}

// CreateAssociationBatch creates multiple associations in a batch.
func (b *InMemoryBackend) CreateAssociationBatch(
	ctx context.Context,
	input *CreateAssociationBatchInput,
) (*CreateAssociationBatchOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("CreateAssociationBatch")
	defer b.mu.Unlock()

	output := &CreateAssociationBatchOutput{
		Successful: make([]Association, 0, len(input.Entries)),
		Failed:     make([]FailedCreateAssociation, 0, len(input.Entries)),
	}

	now := UnixTimeFloat(time.Now())
	docs := b.documentsStore(region)
	assocs := b.associationsStore(region)

	for _, entry := range input.Entries {
		if !docs.Has(entry.Name) {
			output.Failed = append(output.Failed, FailedCreateAssociation{
				Entry:   entry,
				Message: ErrDocumentNotFound.Error(),
				Fault:   faultClient,
			})

			continue
		}

		assocID := uuid.NewString()
		assoc := Association{
			AssociationID:                 assocID,
			AssociationName:               entry.AssociationName,
			DocumentVersion:               entry.DocumentVersion,
			InstanceID:                    entry.InstanceID,
			Name:                          entry.Name,
			Parameters:                    copyAssocParameters(entry.Parameters),
			Targets:                       copyAssocTargets(entry.Targets),
			Overview:                      &AssociationOverview{Status: assocStatusSuccess},
			LastUpdateAssociationDate:     now,
			ApplyOnlyAtCronInterval:       entry.ApplyOnlyAtCronInterval,
			AssociationDispatchAssumeRole: entry.AssociationDispatchAssumeRole,
			AutomationTargetParameterName: entry.AutomationTargetParameterName,
			CalendarNames:                 append([]string(nil), entry.CalendarNames...),
			ComplianceSeverity:            entry.ComplianceSeverity,
			Duration:                      entry.Duration,
			MaxConcurrency:                entry.MaxConcurrency,
			MaxErrors:                     entry.MaxErrors,
			OutputLocation:                copyAssocOutputLocation(entry.OutputLocation),
			ScheduleExpression:            entry.ScheduleExpression,
			SyncCompliance:                entry.SyncCompliance,
			AssociationVersion:            "1",
		}

		assocs.Put(&assoc)
		b.recordAssociationExecutionLocked(region, assoc)
		output.Successful = append(output.Successful, assoc)
	}

	return output, nil
}

// UpdateAssociationStatus updates the status of an association.
func (b *InMemoryBackend) UpdateAssociationStatus(
	ctx context.Context,
	input *UpdateAssociationStatusInput,
) (*UpdateAssociationStatusOutputFull, error) {
	if input.AssociationStatus.Name == "" || input.AssociationStatus.Message == "" ||
		input.AssociationStatus.Date == 0 {
		return nil, fmt.Errorf(
			"%w: AssociationStatus.Name, AssociationStatus.Message and AssociationStatus.Date are required",
			ErrValidationException,
		)
	}

	region := getRegion(ctx)
	b.mu.Lock("UpdateAssociationStatus")
	defer b.mu.Unlock()

	associations := b.associationsStore(region)
	for _, assocPtr := range associations.All() {
		assoc := *assocPtr
		if assoc.InstanceID == input.InstanceID && assoc.Name == input.Name {
			if assoc.Overview == nil {
				assoc.Overview = &AssociationOverview{}
			}

			assoc.Overview.Status = input.AssociationStatus.Name
			assoc.Status = &AssociationStatusInfo{
				Date:           input.AssociationStatus.Date,
				Message:        input.AssociationStatus.Message,
				Name:           input.AssociationStatus.Name,
				AdditionalInfo: input.AssociationStatus.AdditionalInfo,
			}
			associations.Put(&assoc)

			return &UpdateAssociationStatusOutputFull{AssociationDescription: assoc}, nil
		}
	}

	return nil, fmt.Errorf(
		"%w: instance %q / name %q",
		ErrAssociationNotFound,
		input.InstanceID,
		input.Name,
	)
}

// StartAssociationsOnce triggers a one-time run of the given associations.
func (b *InMemoryBackend) StartAssociationsOnce(
	ctx context.Context,
	input *StartAssociationsOnceInput,
) (*StartAssociationsOnceOutput, error) {
	if len(input.AssociationIDs) == 0 {
		return nil, fmt.Errorf("%w: AssociationIds is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("StartAssociationsOnce")
	defer b.mu.Unlock()

	now := time.Now().UTC()

	associations := b.associationsStore(region)
	for _, assocID := range input.AssociationIDs {
		if assocPtr, exists := associations.Get(assocID); exists {
			assoc := *assocPtr
			assoc.LastUpdateAssociationDate = float64(now.Unix())
			associations.Put(&assoc)
			// A one-time run produces a fresh, stable execution record.
			b.recordAssociationExecutionLocked(region, assoc)
		}
	}

	return &StartAssociationsOnceOutput{}, nil
}

// ListAssociationVersions returns the version history of an association.
func (b *InMemoryBackend) ListAssociationVersions(
	ctx context.Context,
	input *ListAssociationVersionsInput,
) (*ListAssociationVersionsOutputFull, error) {
	if input.AssociationID == "" {
		return nil, fmt.Errorf("%w: AssociationId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.RLock("ListAssociationVersions")
	defer b.mu.RUnlock()

	assoc, exists := b.associationsStore(region).Get(input.AssociationID)
	if !exists {
		return &ListAssociationVersionsOutputFull{AssociationVersions: []AssociationVersionInfo{}}, nil
	}

	return &ListAssociationVersionsOutputFull{
		AssociationVersions: []AssociationVersionInfo{associationToVersionInfo(assoc)},
	}, nil
}

func associationToVersionInfo(a *Association) AssociationVersionInfo {
	if a == nil {
		return AssociationVersionInfo{}
	}

	version := a.DocumentVersion
	if version == "" {
		version = "1"
	}

	return AssociationVersionInfo{
		ApplyOnlyAtCronInterval:       a.ApplyOnlyAtCronInterval,
		AssociationDispatchAssumeRole: a.AssociationDispatchAssumeRole,
		AssociationID:                 a.AssociationID,
		AssociationName:               a.AssociationName,
		AssociationVersion:            version,
		CalendarNames:                 a.CalendarNames,
		ComplianceSeverity:            a.ComplianceSeverity,
		CreatedDate:                   a.LastUpdateAssociationDate,
		DocumentVersion:               a.DocumentVersion,
		Duration:                      a.Duration,
		MaxConcurrency:                a.MaxConcurrency,
		MaxErrors:                     a.MaxErrors,
		Name:                          a.Name,
		OutputLocation:                copyAssocOutputLocation(a.OutputLocation),
		Parameters:                    a.Parameters,
		ScheduleExpression:            a.ScheduleExpression,
		SyncCompliance:                a.SyncCompliance,
		Targets:                       a.Targets,
	}
}

const assocExecIDPrefix = "aexec-"

// assocExecStatus returns the effective status of an association's executions.
func assocExecStatus(assoc Association) string {
	if assoc.Overview != nil && assoc.Overview.Status != "" {
		return assoc.Overview.Status
	}

	return commandStatusSuccess
}

// buildAssocExecTargets derives the per-resource execution targets for an
// association execution from the association's InstanceId and Targets.
func buildAssocExecTargets(
	assoc Association,
	execID, status string,
) []AssociationExecutionTarget {
	targets := make([]AssociationExecutionTarget, 0)
	if assoc.InstanceID != "" {
		targets = append(targets, AssociationExecutionTarget{
			AssociationID: assoc.AssociationID,
			ExecutionID:   execID,
			ResourceID:    assoc.InstanceID,
			ResourceType:  "ManagedInstance",
			Status:        status,
		})
	}

	for _, t := range assoc.Targets {
		for _, v := range t.Values {
			targets = append(targets, AssociationExecutionTarget{
				AssociationID: assoc.AssociationID,
				ExecutionID:   execID,
				ResourceID:    v,
				ResourceType:  "ManagedInstance",
				Status:        status,
			})
		}
	}

	return targets
}

// recordAssociationExecutionLocked appends a new stable execution record (and
// its target set) for the association and returns the new execution ID. Records
// are stored newest-first. Must be called with b.mu held for writing.
func (b *InMemoryBackend) recordAssociationExecutionLocked(
	region string,
	assoc Association,
) string {
	execID := assocExecIDPrefix + uuid.NewString()
	status := assocExecStatus(assoc)

	exec := AssociationExecution{
		AssociationID: assoc.AssociationID,
		ExecutionID:   execID,
		Status:        status,
		ExecutionDate: UnixTimeFloat(time.Now()),
	}

	if b.associationExecutions[region] == nil {
		b.associationExecutions[region] = make(map[string][]AssociationExecution)
	}
	execStore := b.associationExecutionsStore(region)
	execStore[assoc.AssociationID] = append(
		[]AssociationExecution{exec},
		execStore[assoc.AssociationID]...,
	)

	if b.associationExecTargets[region] == nil {
		b.associationExecTargets[region] = make(map[string][]AssociationExecutionTarget)
	}
	b.associationExecTargetsStore(region)[execID] = buildAssocExecTargets(assoc, execID, status)

	return execID
}

// DescribeAssociationExecutions returns the stored execution history for an
// association. Records are stable across calls (unlike a freshly minted UUID
// per request). When the association has no recorded executions yet, one is
// created lazily so a valid association is never reported as never-run.
func (b *InMemoryBackend) DescribeAssociationExecutions(
	ctx context.Context,
	input *DescribeAssociationExecutionsInput,
) (*DescribeAssociationExecutionsOutputFull, error) {
	if input.AssociationID == "" {
		return nil, fmt.Errorf("%w: AssociationId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("DescribeAssociationExecutions")
	defer b.mu.Unlock()

	assocPtr, exists := b.associationsStore(region).Get(input.AssociationID)
	if !exists {
		return &DescribeAssociationExecutionsOutputFull{
			AssociationExecutions: []AssociationExecution{},
		}, nil
	}

	execs := b.associationExecutionsStore(region)[input.AssociationID]
	if len(execs) == 0 {
		b.recordAssociationExecutionLocked(region, *assocPtr)
		execs = b.associationExecutionsStore(region)[input.AssociationID]
	}

	out := make([]AssociationExecution, len(execs))
	copy(out, execs)

	maxResults := 0
	if input.MaxResults != nil {
		maxResults = int(*input.MaxResults)
	}

	page, next := paginateSlice(out, input.NextToken, maxResults, defaultDescribeMaxResults)

	return &DescribeAssociationExecutionsOutputFull{
		AssociationExecutions: page,
		NextToken:             next,
	}, nil
}

// DescribeAssociationExecutionTargets returns the stored targets for a specific
// association execution. When no ExecutionId is supplied the latest execution
// is used; both the execution and its targets are stable across calls.
func (b *InMemoryBackend) DescribeAssociationExecutionTargets(
	ctx context.Context,
	input *DescribeAssociationExecutionTargetsInput,
) (*DescribeAssociationExecutionTargetsOutputFull, error) {
	if input.AssociationID == "" {
		return nil, fmt.Errorf("%w: AssociationId is required", ErrValidationException)
	}

	region := getRegion(ctx)
	b.mu.Lock("DescribeAssociationExecutionTargets")
	defer b.mu.Unlock()

	assocPtr, exists := b.associationsStore(region).Get(input.AssociationID)
	if !exists {
		return &DescribeAssociationExecutionTargetsOutputFull{
			AssociationExecutionTargets: []AssociationExecutionTarget{},
		}, nil
	}

	assoc := *assocPtr

	execID := input.ExecutionID
	if execID == "" {
		execs := b.associationExecutionsStore(region)[input.AssociationID]
		if len(execs) == 0 {
			execID = b.recordAssociationExecutionLocked(region, assoc)
		} else {
			execID = execs[0].ExecutionID
		}
	}

	targets := b.associationExecTargetsStore(region)[execID]
	if targets == nil {
		// A caller-supplied ExecutionId we have not seen: materialise and store
		// a stable target set for it derived from the association.
		targets = buildAssocExecTargets(assoc, execID, assocExecStatus(assoc))
		if b.associationExecTargets[region] == nil {
			b.associationExecTargets[region] = make(map[string][]AssociationExecutionTarget)
		}
		b.associationExecTargetsStore(region)[execID] = targets
	}

	out := make([]AssociationExecutionTarget, len(targets))
	copy(out, targets)

	maxResults := 0
	if input.MaxResults != nil {
		maxResults = int(*input.MaxResults)
	}

	page, next := paginateSlice(out, input.NextToken, maxResults, defaultDescribeMaxResults)

	return &DescribeAssociationExecutionTargetsOutputFull{
		AssociationExecutionTargets: page,
		NextToken:                   next,
	}, nil
}

// DeleteAssociation removes a stored association by ID.
func (b *InMemoryBackend) DeleteAssociation(
	ctx context.Context,
	input *DeleteAssociationInput,
) (*DeleteAssociationOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("DeleteAssociation")
	defer b.mu.Unlock()

	associations := b.associationsStore(region)
	if !associations.Has(input.AssociationID) {
		return nil, ErrAssociationNotFound
	}

	associations.Delete(input.AssociationID)

	// Evict the association's execution records and their per-execution targets
	// so deleted associations do not leak execution state.
	if execs := b.associationExecutionsStore(region); execs != nil {
		for _, e := range execs[input.AssociationID] {
			delete(b.associationExecTargetsStore(region), e.ExecutionID)
		}

		delete(execs, input.AssociationID)
	}

	delete(b.miscResourceTagsStore(region), input.AssociationID)

	cleanupEmptyInnerMap(b.associationExecutions, region)
	cleanupEmptyInnerMap(b.associationExecTargets, region)
	cleanupEmptyInnerMap(b.miscResourceTags, region)

	return &DeleteAssociationOutput{}, nil
}

// DescribeAssociation retrieves an association by name or ID.
func (b *InMemoryBackend) DescribeAssociation(
	ctx context.Context,
	input *DescribeAssociationInput,
) (*DescribeAssociationOutput, error) {
	region := getRegion(ctx)
	b.mu.RLock("DescribeAssociation")
	defer b.mu.RUnlock()

	for _, assocPtr := range b.associationsStore(region).All() {
		assoc := *assocPtr
		if (input.AssociationID != "" && assoc.AssociationID == input.AssociationID) ||
			(input.Name != "" && assoc.Name == input.Name && (input.InstanceID == "" || assoc.InstanceID == input.InstanceID)) {
			return &DescribeAssociationOutput{AssociationDescription: assoc}, nil
		}
	}

	return nil, ErrAssociationNotFound
}

// associationAttr returns the value of an Association attribute by its
// AssociationFilterKey name. Only the attributes Association itself tracks
// can be meaningfully filtered; every other key
// (LastExecutedBefore/LastExecutedAfter/ResourceGroupName/CloudConnectorId)
// returns "" and is treated as untracked (see matchesAssociationFilter).
func associationAttr(a Association, key string) (string, bool) {
	switch key {
	case filterKeyInstanceID:
		return a.InstanceID, true
	case filterKeyName:
		return a.Name, true
	case "AssociationId":
		return a.AssociationID, true
	case "AssociationName":
		return a.AssociationName, true
	case "AssociationStatusName":
		if a.Overview != nil {
			return a.Overview.Status, true
		}

		return "", true
	default:
		return "", false
	}
}

// matchesAssociationFilter reports whether an association satisfies a single
// key/value filter. Unrecognized keys match every association
// (accept-and-echo, mirroring ListNodes' unknown-key handling,
// instances.go).
func matchesAssociationFilter(a Association, f AssociationFilterEntry) bool {
	value, tracked := associationAttr(a, f.Key)
	if !tracked {
		return true
	}

	return value == f.Value
}

// ListAssociations lists stored associations, filtered by
// input.AssociationFilterList and paginated by input.MaxResults/NextToken --
// real, optional ListAssociationsInput members (api_op_ListAssociations.go)
// a literal struct{} input previously discarded from every request.
//
//nolint:dupl // mirrors ListOpsMetadata's filter/sort/paginate shape inherently, not by copy-paste
func (b *InMemoryBackend) ListAssociations(
	ctx context.Context,
	input *ListAssociationsInput,
) (*ListAssociationsOutputFull, error) {
	region := getRegion(ctx)
	b.mu.RLock("ListAssociations")
	defer b.mu.RUnlock()

	associations := b.associationsStore(region)
	list := make([]Association, 0, associations.Len())

	for _, a := range associations.All() {
		matched := true

		for _, f := range input.AssociationFilterList {
			if !matchesAssociationFilter(*a, f) {
				matched = false

				break
			}
		}

		if matched {
			list = append(list, *a)
		}
	}

	sort.Slice(list, func(i, j int) bool { return list[i].AssociationID < list[j].AssociationID })

	var maxResults int
	if input.MaxResults != nil {
		maxResults = int(*input.MaxResults)
	}

	page, next := paginateSlice(list, input.NextToken, maxResults, defaultDescribeMaxResults)

	return &ListAssociationsOutputFull{Associations: page, NextToken: next}, nil
}

// applyAssociationCoreUpdates applies UpdateAssociationInput's original
// (pre-extended-fields) settable properties to assoc in place.
func applyAssociationCoreUpdates(assoc *Association, input *UpdateAssociationInput) {
	if input.AssociationName != "" {
		assoc.AssociationName = input.AssociationName
	}

	if input.DocumentVersion != "" {
		assoc.DocumentVersion = input.DocumentVersion
	}

	if input.Parameters != nil {
		assoc.Parameters = copyAssocParameters(input.Parameters)
	}

	if input.Targets != nil {
		assoc.Targets = copyAssocTargets(input.Targets)
	}
}

// applyAssociationExtendedUpdates applies the State Manager fields added
// alongside CreateAssociationInput (ApplyOnlyAtCronInterval/
// AssociationDispatchAssumeRole/AutomationTargetParameterName/CalendarNames/
// ComplianceSeverity/Duration/MaxConcurrency/MaxErrors/OutputLocation/
// ScheduleExpression/SyncCompliance) to assoc in place. Split out of
// UpdateAssociation to keep its cyclomatic complexity under the package
// limit.
func applyAssociationExtendedUpdates(assoc *Association, input *UpdateAssociationInput) {
	if input.ApplyOnlyAtCronInterval {
		assoc.ApplyOnlyAtCronInterval = input.ApplyOnlyAtCronInterval
	}

	if input.AssociationDispatchAssumeRole != "" {
		assoc.AssociationDispatchAssumeRole = input.AssociationDispatchAssumeRole
	}

	if input.AutomationTargetParameterName != "" {
		assoc.AutomationTargetParameterName = input.AutomationTargetParameterName
	}

	if input.CalendarNames != nil {
		assoc.CalendarNames = append([]string(nil), input.CalendarNames...)
	}

	if input.ComplianceSeverity != "" {
		assoc.ComplianceSeverity = input.ComplianceSeverity
	}

	if input.Duration != nil {
		assoc.Duration = input.Duration
	}

	if input.MaxConcurrency != "" {
		assoc.MaxConcurrency = input.MaxConcurrency
	}

	if input.MaxErrors != "" {
		assoc.MaxErrors = input.MaxErrors
	}

	if input.OutputLocation != nil {
		assoc.OutputLocation = copyAssocOutputLocation(input.OutputLocation)
	}

	if input.ScheduleExpression != "" {
		assoc.ScheduleExpression = input.ScheduleExpression
	}

	if input.SyncCompliance != "" {
		assoc.SyncCompliance = input.SyncCompliance
	}
}

// UpdateAssociation updates an existing association.
func (b *InMemoryBackend) UpdateAssociation(
	ctx context.Context,
	input *UpdateAssociationInput,
) (*UpdateAssociationOutput, error) {
	region := getRegion(ctx)
	b.mu.Lock("UpdateAssociation")
	defer b.mu.Unlock()

	associations := b.associationsStore(region)
	assocPtr, exists := associations.Get(input.AssociationID)
	if !exists {
		return nil, ErrAssociationNotFound
	}

	assoc := *assocPtr

	applyAssociationCoreUpdates(&assoc, input)
	applyAssociationExtendedUpdates(&assoc, input)

	assoc.LastUpdateAssociationDate = UnixTimeFloat(timeNow())
	associations.Put(&assoc)

	return &UpdateAssociationOutput{AssociationDescription: assoc}, nil
}
