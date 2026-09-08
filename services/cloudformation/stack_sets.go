package cloudformation

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

const (
	statusComplete                   = "COMPLETE"
	statusEnabled                    = "ENABLED"
	resourceScanCompletePercent      = 100
	typeKindResource                 = "RESOURCE"
	typeVisibilityPublic             = "PUBLIC"
	typeVisibilityPrivate            = "PRIVATE"
	typeStatusDeprecated             = "DEPRECATED"
	provisioningTypeFullyMutable     = "FULLY_MUTABLE"
	driftStatusDrifted               = "DRIFTED"
	driftStatusModified              = "MODIFIED"
	driftStatusDeleted               = "DELETED"
	driftStatusNotChecked            = "NOT_CHECKED"
	stackSetPermissionServiceManaged = "SERVICE_MANAGED"
)

// StackSetOptions holds the optional fields accepted by CreateStackSet and
// UpdateStackSet beyond name/description/templateBody, mirroring the shape of
// StackOptions for regular stacks.
type StackSetOptions struct {
	AutoDeployment        *AutoDeployment
	ManagedExecution      *ManagedExecution
	AdministrationRoleARN string
	ExecutionRoleName     string
	PermissionModel       string
	Capabilities          []string
	Parameters            []Parameter
	Tags                  []Tag
	OrganizationalUnitIDs []string
}

func (b *InMemoryBackend) CreateStackSet(
	name, description, templateBody string,
	opts StackSetOptions,
) (*StackSet, error) {
	b.mu.Lock("CreateStackSet")
	defer b.mu.Unlock()
	if b.stackSets.Has(name) {
		return nil, ErrStackSetAlreadyExists
	}
	stackSetID := uuid.New().String()
	ss := &StackSet{
		StackSetID:   stackSetID,
		StackSetName: name,
		Description:  description,
		TemplateBody: templateBody,
		Status:       "ACTIVE",
		StackSetARN: arn.Build(
			"cloudformation", b.region, b.accountID, "stackset/"+name+":"+stackSetID,
		),
		AdministrationRoleARN: opts.AdministrationRoleARN,
		ExecutionRoleName:     opts.ExecutionRoleName,
		PermissionModel:       opts.PermissionModel,
		Capabilities:          opts.Capabilities,
		Parameters:            opts.Parameters,
		Tags:                  opts.Tags,
		OrganizationalUnitIDs: opts.OrganizationalUnitIDs,
		AutoDeployment:        opts.AutoDeployment,
		ManagedExecution:      opts.ManagedExecution,
	}
	b.stackSets.Put(ss)

	return ss, nil
}

func (b *InMemoryBackend) UpdateStackSet(
	name, description, templateBody string,
	opts StackSetOptions,
) (*StackSet, string, error) {
	b.mu.Lock("UpdateStackSet")
	defer b.mu.Unlock()
	ss, ok := b.stackSets.Get(name)
	if !ok {
		return nil, "", ErrStackSetNotFound
	}
	if description != "" {
		ss.Description = description
	}
	if templateBody != "" {
		ss.TemplateBody = templateBody
	}
	if opts.AdministrationRoleARN != "" {
		ss.AdministrationRoleARN = opts.AdministrationRoleARN
	}
	if opts.ExecutionRoleName != "" {
		ss.ExecutionRoleName = opts.ExecutionRoleName
	}
	if opts.PermissionModel != "" {
		ss.PermissionModel = opts.PermissionModel
	}
	if opts.Capabilities != nil {
		ss.Capabilities = opts.Capabilities
	}
	if opts.Parameters != nil {
		ss.Parameters = opts.Parameters
	}
	if opts.Tags != nil {
		ss.Tags = opts.Tags
	}
	if opts.OrganizationalUnitIDs != nil {
		ss.OrganizationalUnitIDs = opts.OrganizationalUnitIDs
	}
	if opts.AutoDeployment != nil {
		ss.AutoDeployment = opts.AutoDeployment
	}
	if opts.ManagedExecution != nil {
		ss.ManagedExecution = opts.ManagedExecution
	}
	opID := b.recordStackSetOperation(name, "UPDATE")

	return ss, opID, nil
}

func (b *InMemoryBackend) DeleteStackSet(name string) error {
	b.mu.Lock("DeleteStackSet")
	defer b.mu.Unlock()
	if !b.stackSets.Has(name) {
		// DeleteStackSet's modeled error set (OperationInProgressException,
		// StackSetNotEmptyException) has no "not found" case — like DeleteStack,
		// deleting a StackSet that doesn't exist (or was already deleted) is a
		// silent no-op in real AWS, not an error.
		return nil
	}
	if len(b.stackInstances[name]) > 0 {
		return ErrStackSetNotEmpty
	}
	b.stackSets.Delete(name)
	delete(b.stackInstances, name)
	delete(b.stackSetOperations, name)
	delete(b.stackSetOpResults, name)

	return nil
}

func (b *InMemoryBackend) DescribeStackSet(name string) (*StackSet, error) {
	b.mu.RLock("DescribeStackSet")
	defer b.mu.RUnlock()
	ss, ok := b.stackSets.Get(name)
	if !ok {
		return nil, ErrStackSetNotFound
	}

	return ss, nil
}

// StackSetRegions returns the deduplicated, sorted list of Amazon Web
// Services Regions the given StackSet currently has stack instances deployed
// in, matching real DescribeStackSetResult.StackSet.Regions. This is derived
// live from b.stackInstances rather than stored on the StackSet record itself
// -- storing it directly would create a second source of truth that could
// drift out of sync with the actual instances (same rationale as the
// driftByStackID reverse index rebuilt in Restore).
func (b *InMemoryBackend) StackSetRegions(name string) []string {
	b.mu.RLock("StackSetRegions")
	defer b.mu.RUnlock()

	seen := make(map[string]bool)
	var regions []string
	for _, inst := range b.stackInstances[name] {
		if !seen[inst.Region] {
			seen[inst.Region] = true
			regions = append(regions, inst.Region)
		}
	}
	sort.Strings(regions)

	return regions
}

func (b *InMemoryBackend) ListStackSets(nextToken, status string) (page.Page[StackSetSummary], error) {
	b.mu.RLock("ListStackSets")
	defer b.mu.RUnlock()
	result := make([]StackSetSummary, 0, b.stackSets.Len())
	for _, ss := range b.stackSets.All() {
		if status != "" && ss.Status != status {
			continue
		}

		result = append(result, StackSetSummary{
			StackSetID:   ss.StackSetID,
			StackSetName: ss.StackSetName,
			Status:       ss.Status,
			Description:  ss.Description,
		})
	}
	sort.Slice(
		result,
		func(i, j int) bool { return result[i].StackSetName < result[j].StackSetName },
	)

	return page.New(result, nextToken, 0, cfnDefaultPageSize), nil
}

func (b *InMemoryBackend) DetectStackSetDrift(stackSetName string) (string, error) {
	b.mu.Lock("DetectStackSetDrift")
	defer b.mu.Unlock()
	if !b.stackSets.Has(stackSetName) {
		return "", ErrStackSetNotFound
	}
	opID := b.recordStackSetOperation(stackSetName, "DETECT_DRIFT")
	b.detectStackInstanceDrift(stackSetName)

	return opID, nil
}

// detectStackInstanceDrift runs real per-resource drift comparison (the same
// compareStackResources logic DetectStackDrift uses for a standalone stack)
// against every stack instance's provisioned child stack, updating each
// instance's DriftStatus in place. Previously DetectStackSetDrift only
// recorded a SUCCEEDED operation without ever touching instance DriftStatus
// at all -- a disguised stub (looks real, records a real operation, but the
// actual per-instance drift diff never ran). Caller must hold b.mu.Lock.
func (b *InMemoryBackend) detectStackInstanceDrift(stackSetName string) {
	instances := b.stackInstances[stackSetName]
	for i := range instances {
		stackName, ok := b.stackIDIndex[instances[i].StackID]
		if !ok {
			instances[i].DriftStatus = driftStatusNotChecked

			continue
		}
		stack, ok := b.stacks.Get(stackName)
		if !ok {
			instances[i].DriftStatus = driftStatusNotChecked

			continue
		}

		instances[i].DriftStatus = driftStatusInSync
		for _, status := range b.compareStackResources(stack) {
			if status != driftStatusInSync {
				instances[i].DriftStatus = driftStatusDrifted

				break
			}
		}
	}
}

// recordStackSetOperation creates a StackSetOperation record and returns its ID.
// Caller must hold b.mu.Lock.
func (b *InMemoryBackend) recordStackSetOperation(stackSetName, action string) string {
	opID := uuid.New().String()
	if b.stackSetOperations[stackSetName] == nil {
		b.stackSetOperations[stackSetName] = make(map[string]*StackSetOperation)
	}
	b.stackSetOperations[stackSetName][opID] = &StackSetOperation{
		OperationID:  opID,
		StackSetName: stackSetName,
		Action:       action,
		Status:       "SUCCEEDED",
		CreatedAt:    time.Now(),
	}
	if b.stackSetOpResults[stackSetName] == nil {
		b.stackSetOpResults[stackSetName] = make(map[string][]StackSetOperationResult)
	}
	b.trimStackSetOperations(stackSetName)

	return opID
}

// recordOpResults records per-account/region operation results. Caller must hold b.mu.Lock.
func (b *InMemoryBackend) recordOpResults(
	stackSetName, opID string,
	accounts, regions []string,
	status string,
) {
	if b.stackSetOpResults[stackSetName] == nil {
		b.stackSetOpResults[stackSetName] = make(map[string][]StackSetOperationResult)
	}
	for _, acct := range accounts {
		for _, region := range regions {
			b.stackSetOpResults[stackSetName][opID] = append(
				b.stackSetOpResults[stackSetName][opID],
				StackSetOperationResult{
					Account: acct,
					Region:  region,
					Status:  status,
				},
			)
		}
	}
}

const maxOpsPerStackSet = 1000

func (b *InMemoryBackend) ListStackSetOperations(
	stackSetName, nextToken string,
) (page.Page[StackSetOperationSummary], error) {
	b.mu.RLock("ListStackSetOperations")
	defer b.mu.RUnlock()
	ops := b.stackSetOperations[stackSetName]
	sorted := make([]*StackSetOperation, 0, len(ops))
	for _, op := range ops {
		sorted = append(sorted, op)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
		}

		return sorted[i].OperationID < sorted[j].OperationID
	})
	summaries := make([]StackSetOperationSummary, 0, len(sorted))
	for _, op := range sorted {
		summaries = append(summaries, StackSetOperationSummary{
			OperationID:  op.OperationID,
			Action:       op.Action,
			Status:       op.Status,
			CreationTime: op.CreatedAt,
		})
	}

	return page.New(summaries, nextToken, 0, cfnDefaultPageSize), nil
}

// trimStackSetOperations evicts the oldest entries when a stack set exceeds maxOpsPerStackSet.
// Caller must hold b.mu.Lock.
func (b *InMemoryBackend) trimStackSetOperations(stackSetName string) {
	ops := b.stackSetOperations[stackSetName]
	if len(ops) <= maxOpsPerStackSet {
		return
	}
	sorted := make([]*StackSetOperation, 0, len(ops))
	for _, op := range ops {
		sorted = append(sorted, op)
	}
	sort.Slice(
		sorted,
		func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) },
	)
	evict := len(sorted) - maxOpsPerStackSet
	for _, op := range sorted[:evict] {
		delete(ops, op.OperationID)
		delete(b.stackSetOpResults[stackSetName], op.OperationID)
	}
}

func (b *InMemoryBackend) DescribeStackSetOperation(
	stackSetName, operationID string,
) (*StackSetOperation, error) {
	b.mu.RLock("DescribeStackSetOperation")
	defer b.mu.RUnlock()

	// The SDK's DescribeStackSetOperation error model has a distinct
	// StackSetNotFoundException case (unlike this op's siblings), so an
	// unknown StackSetName must surface that instead of the generic
	// OperationNotFoundException used when the StackSet exists but the
	// operation ID doesn't.
	if !b.stackSets.Has(stackSetName) {
		return nil, fmt.Errorf("%w: %s", ErrStackSetNotFound, stackSetName)
	}

	ops := b.stackSetOperations[stackSetName]
	if ops == nil {
		return nil, fmt.Errorf("%w: %s in %s", ErrOperationNotFound, operationID, stackSetName)
	}
	op, ok := ops[operationID]
	if !ok {
		return nil, fmt.Errorf("%w: %s in %s", ErrOperationNotFound, operationID, stackSetName)
	}

	return op, nil
}

func (b *InMemoryBackend) StopStackSetOperation(stackSetName, operationID string) error {
	b.mu.Lock("StopStackSetOperation")
	defer b.mu.Unlock()
	ops := b.stackSetOperations[stackSetName]
	if ops == nil {
		return fmt.Errorf("%w: %s in %s", ErrOperationNotFound, operationID, stackSetName)
	}
	op, ok := ops[operationID]
	if !ok {
		return fmt.Errorf("%w: %s in %s", ErrOperationNotFound, operationID, stackSetName)
	}
	if op.Status != "RUNNING" {
		return fmt.Errorf("%w: %s (current: %s)", ErrOperationNotRunning, operationID, op.Status)
	}
	op.Status = "STOPPED"

	return nil
}

func (b *InMemoryBackend) ListStackSetOperationResults(
	stackSetName, operationID, _ string,
) ([]StackSetOperationResult, error) {
	b.mu.RLock("ListStackSetOperationResults")
	defer b.mu.RUnlock()
	if !b.stackSets.Has(stackSetName) {
		return nil, fmt.Errorf("%w: %s", ErrStackSetNotFound, stackSetName)
	}
	if _, ok := b.stackSetOperations[stackSetName][operationID]; !ok {
		return nil, fmt.Errorf("%w: %s in %s", ErrOperationNotFound, operationID, stackSetName)
	}
	results := b.stackSetOpResults[stackSetName][operationID]
	out := make([]StackSetOperationResult, len(results))
	copy(out, results)

	return out, nil
}

func (b *InMemoryBackend) ListStackSetAutoDeploymentTargets(
	stackSetName string,
) ([]AutoDeploymentTarget, error) {
	b.mu.RLock("ListStackSetAutoDeploymentTargets")
	defer b.mu.RUnlock()
	if !b.stackSets.Has(stackSetName) {
		return nil, ErrStackSetNotFound
	}

	byOU := make(map[string]int) // OU ID -> index in targets
	targets := make([]AutoDeploymentTarget, 0)
	for _, inst := range b.stackInstances[stackSetName] {
		// SERVICE_MANAGED instances carry the OU they were deployed through
		// (see resolveInstanceTargets); self-managed instances have none, so
		// fall back to one synthetic target per account.
		ouID := inst.OrganizationalUnitID
		if ouID == "" {
			ouID = inst.Account
		}
		if idx, ok := byOU[ouID]; ok {
			if !slices.Contains(targets[idx].Regions, inst.Region) {
				targets[idx].Regions = append(targets[idx].Regions, inst.Region)
			}

			continue
		}
		byOU[ouID] = len(targets)
		targets = append(targets, AutoDeploymentTarget{
			OrganizationalUnitID: ouID,
			Regions:              []string{inst.Region},
		})
	}

	return targets, nil
}

func (b *InMemoryBackend) ImportStacksToStackSet(stackSetName string, stackIDs []string) (string, error) {
	b.mu.Lock("ImportStacksToStackSet")
	defer b.mu.Unlock()
	ss, ok := b.stackSets.Get(stackSetName)
	if !ok {
		return "", ErrStackSetNotFound
	}
	opID := b.recordStackSetOperation(stackSetName, "IMPORT")
	for _, stackID := range stackIDs {
		// Skip duplicates.
		already := false
		for _, inst := range b.stackInstances[stackSetName] {
			if inst.StackID == stackID {
				already = true

				break
			}
		}
		if already {
			continue
		}
		account, region := parseStackARN(stackID)
		b.stackInstances[stackSetName] = append(b.stackInstances[stackSetName], StackInstance{
			StackSetID:      ss.StackSetID,
			StackSetName:    stackSetName,
			StackID:         stackID,
			Account:         account,
			Region:          region,
			Status:          "CURRENT",
			DriftStatus:     driftStatusNotChecked,
			LastOperationID: opID,
		})
	}

	return opID, nil
}

func (b *InMemoryBackend) ActivateOrganizationsAccess() error {
	b.mu.Lock("ActivateOrganizationsAccess")
	defer b.mu.Unlock()
	b.orgAccessEnabled = true

	return nil
}

func (b *InMemoryBackend) DeactivateOrganizationsAccess() error {
	b.mu.Lock("DeactivateOrganizationsAccess")
	defer b.mu.Unlock()
	b.orgAccessEnabled = false

	return nil
}

func (b *InMemoryBackend) DescribeOrganizationsAccess() (string, error) {
	b.mu.RLock("DescribeOrganizationsAccess")
	defer b.mu.RUnlock()
	if b.orgAccessEnabled {
		return statusEnabled, nil
	}

	return "DISABLED", nil
}
