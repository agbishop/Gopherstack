package cloudformation

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

func (b *InMemoryBackend) DeleteStack(ctx context.Context, nameOrID string) error {
	b.mu.Lock("DeleteStack")
	defer b.mu.Unlock()

	return b.deleteStackLocked(ctx, nameOrID)
}

// DescribeStack returns details for a single stack.
func (b *InMemoryBackend) DescribeStack(nameOrID string) (*Stack, error) {
	b.mu.RLock("DescribeStack")
	defer b.mu.RUnlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return nil, ErrStackNotFound
	}

	return stack, nil
}

const cfnDefaultPageSize = 100

// ListStacks returns paginated stack summaries, optionally filtered by status.
func (b *InMemoryBackend) ListStacks(
	statusFilter []string,
	nextToken string,
) (page.Page[StackSummary], error) {
	b.mu.RLock("ListStacks")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(statusFilter))
	for _, s := range statusFilter {
		filter[s] = true
	}

	summaries := make([]StackSummary, 0, b.stacks.Len())
	for _, stack := range b.stacks.All() {
		if len(filter) > 0 && !filter[stack.StackStatus] {
			continue
		}
		summaries = append(summaries, StackSummary{
			StackID:           stack.StackID,
			StackName:         stack.StackName,
			StackStatus:       stack.StackStatus,
			StackStatusReason: stack.StackStatusReason,
			CreationTime:      stack.CreationTime,
			DeletionTime:      stack.DeletionTime,
			LastUpdatedTime:   stack.LastUpdatedTime,
		})
	}

	sort.Slice(
		summaries,
		func(i, j int) bool { return summaries[i].StackName < summaries[j].StackName },
	)

	return page.New(summaries, nextToken, 0, cfnDefaultPageSize), nil
}

// DescribeStackEvents returns paginated events for a stack, most recent first.
func (b *InMemoryBackend) DescribeStackEvents(
	nameOrID, nextToken string,
) (page.Page[StackEvent], error) {
	b.mu.RLock("DescribeStackEvents")
	defer b.mu.RUnlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return page.Page[StackEvent]{}, ErrStackNotFound
	}

	evts := b.events[stack.StackID]
	result := make([]StackEvent, len(evts))
	// Return in reverse chronological order (most recent first).
	for i, e := range evts {
		result[len(evts)-1-i] = e
	}

	return page.New(result, nextToken, 0, cfnDefaultPageSize), nil
}

// ListAll returns all stacks (for dashboard).
func (b *InMemoryBackend) ListAll() []*Stack {
	b.mu.RLock("ListAll")
	defer b.mu.RUnlock()

	return b.stacks.All()
}

// ContinueUpdateRollback continues the rollback for a stack that is in ROLLBACK_IN_PROGRESS
// or UPDATE_ROLLBACK_IN_PROGRESS state.
func (b *InMemoryBackend) ContinueUpdateRollback(_ context.Context, nameOrID string) error {
	b.mu.Lock("ContinueUpdateRollback")
	defer b.mu.Unlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return ErrStackNotFound
	}

	switch stack.StackStatus {
	case statusRollbackInProgress:
		stack.StackStatus = statusRollbackComplete
		b.addEvent(stack.StackID, stack.StackName, stack.StackName, stack.StackID,
			cfnStackType, statusRollbackComplete, "")
	case statusUpdateRollbackInProgress:
		stack.StackStatus = statusUpdateRollbackComplete
		b.addEvent(stack.StackID, stack.StackName, stack.StackName, stack.StackID,
			cfnStackType, statusUpdateRollbackComplete, "")
	}

	return nil
}

// CancelUpdateStack cancels an in-progress stack update, transitioning it to
// UPDATE_ROLLBACK_COMPLETE. Real AWS: "You can cancel only stacks that are
// in the UPDATE_IN_PROGRESS state".
func (b *InMemoryBackend) CancelUpdateStack(_ context.Context, nameOrID string) error {
	b.mu.Lock("CancelUpdateStack")
	defer b.mu.Unlock()

	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return ErrStackNotFound
	}

	if stack.StackStatus != statusUpdateInProgress {
		return ErrCancelUpdateStackInvalidState
	}

	stack.StackStatus = statusUpdateRollbackInProgress
	b.addEvent(stack.StackID, stack.StackName, stack.StackName, stack.StackID,
		cfnStackType, statusUpdateRollbackInProgress, reasonUserInitiated)
	stack.StackStatus = statusUpdateRollbackComplete
	b.addEvent(stack.StackID, stack.StackName, stack.StackName, stack.StackID,
		cfnStackType, statusUpdateRollbackComplete, "")

	return nil
}

const cfnDefaultAccountLimitCount = 200

// DescribeAccountLimits returns the CloudFormation account limits for this mock.
func (b *InMemoryBackend) DescribeAccountLimits() []AccountLimit {
	return []AccountLimit{
		{Name: "stackCount", Value: cfnDefaultAccountLimitCount},
		{Name: "stackOutputsCount", Value: cfnDefaultAccountLimitCount},
	}
}

func (b *InMemoryBackend) RollbackStack(_ context.Context, nameOrID string) (*Stack, error) {
	b.mu.Lock("RollbackStack")
	defer b.mu.Unlock()
	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStackNotFound, nameOrID)
	}
	stack.StackStatus = statusRollbackComplete

	return stack, nil
}

// isFailedResourceStatus reports whether status is one of CloudFormation's
// failure states -- CREATE_FAILED/UPDATE_FAILED/DELETE_FAILED/
// UPDATE_ROLLBACK_FAILED/ROLLBACK_FAILED/IMPORT_FAILED/
// IMPORT_ROLLBACK_FAILED all follow the same "_FAILED" suffix convention.
func isFailedResourceStatus(status string) bool {
	return strings.HasSuffix(status, "_FAILED")
}

// filterFailedEvents applies DescribeEventsInput's Filters.FailedEvents
// member (cloudformation@v1.76.1 types.EventFilter) when failedOnly is set.
func filterFailedEvents(events []StackEvent, failedOnly bool) []StackEvent {
	if !failedOnly {
		return events
	}
	filtered := make([]StackEvent, 0, len(events))
	for _, e := range events {
		if isFailedResourceStatus(e.ResourceStatus) {
			filtered = append(filtered, e)
		}
	}

	return filtered
}

func (b *InMemoryBackend) DescribeEvents(
	stackName, nextToken string,
	failedOnly bool,
) (page.Page[StackEvent], error) {
	b.mu.RLock("DescribeEvents")
	defer b.mu.RUnlock()
	if stackName != "" {
		stack, ok := b.resolveStack(stackName)
		if !ok {
			return page.Page[StackEvent]{}, fmt.Errorf("%w: %s", ErrStackNotFound, stackName)
		}
		evts := b.events[stack.StackID]
		all := make([]StackEvent, len(evts))
		copy(all, evts)
		// AWS returns events newest-first.
		sort.Slice(all, func(i, j int) bool {
			return all[i].Timestamp.After(all[j].Timestamp)
		})
		all = filterFailedEvents(all, failedOnly)

		return page.New(all, nextToken, 0, cfnDefaultPageSize), nil
	}
	// No filter: collect events across all stacks.
	var total int
	for _, evts := range b.events {
		total += len(evts)
	}
	all := make([]StackEvent, 0, total)
	for _, evts := range b.events {
		all = append(all, evts...)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].Timestamp.Equal(all[j].Timestamp) {
			return all[i].Timestamp.After(all[j].Timestamp)
		}

		return all[i].EventID < all[j].EventID
	})
	all = filterFailedEvents(all, failedOnly)

	return page.New(all, nextToken, 0, cfnDefaultPageSize), nil
}

func (b *InMemoryBackend) UpdateTerminationProtection(nameOrID string, enable bool) error {
	b.mu.Lock("UpdateTerminationProtection")
	defer b.mu.Unlock()
	stack, ok := b.resolveStack(nameOrID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrStackNotFound, nameOrID)
	}
	stack.EnableTerminationProtection = enable

	return nil
}

// iamRoleARNPattern matches valid IAM role ARNs:
// arn:aws[-gov|-cn]*:iam::<accountID>:role[/path]/<roleName>
var iamRoleARNPattern = regexp.MustCompile(
	`^arn:aws(?:-cn|-gov|-iso|-iso-b)?:iam::\d{12}:role(/[^/]+)*/[^/\s]+$`,
)

// ValidateRoleARN checks that the provided role ARN is syntactically valid.
// Returns ErrInvalidRoleARN if the ARN does not match the expected IAM role format.
func ValidateRoleARN(roleARN string) error {
	if roleARN == "" {
		return nil
	}
	if !iamRoleARNPattern.MatchString(roleARN) {
		return fmt.Errorf("%w: %q must match arn:aws:iam::<account>:role/<name>", ErrInvalidRoleARN, roleARN)
	}

	return nil
}

// requireIAMCapability checks whether the template references IAM resources and
// the caller declared the appropriate capability. It returns ErrInsufficientCapabilities
// if the template creates IAM resources but the capability is absent.
func requireIAMCapability(templateBody string, capabilities []string) error {
	if templateBody == "" {
		return nil
	}
	hasIAM := strings.Contains(templateBody, "AWS::IAM::")
	if !hasIAM {
		return nil
	}
	// Note: CAPABILITY_AUTO_EXPAND only authorizes macro/transform expansion
	// (e.g. SAM); it does not grant permission to create IAM resources
	// declared directly in the template, so it must NOT satisfy this check.
	for _, c := range capabilities {
		if c == "CAPABILITY_IAM" || c == "CAPABILITY_NAMED_IAM" {
			return nil
		}
	}

	return ErrInsufficientCapabilities
}

// requireAutoExpandCapability checks whether the template declares a top-level
// Transform (macro/SAM expansion) and the caller declared CAPABILITY_AUTO_EXPAND.
// Per AWS docs, CAPABILITY_AUTO_EXPAND is required for any stack whose template
// uses a Transform, since macro expansion can silently add or modify resources
// that were not visible in the submitted template. It returns
// ErrInsufficientCapabilities if the capability is absent. A template that fails
// to parse is not this function's concern -- the caller's own ParseTemplate call
// surfaces that error through its normal path.
func requireAutoExpandCapability(templateBody string, capabilities []string) error {
	if templateBody == "" {
		return nil
	}
	tmpl, err := ParseTemplate(templateBody)
	if err != nil {
		// Deliberately swallowed: a malformed template is the caller's own
		// ParseTemplate call's problem to surface (createStackFromTemplate /
		// applyTemplateToStack), not this capability pre-flight check's.
		return nil //nolint:nilerr // malformed-template errors are surfaced by the caller's own parse, not here
	}
	if len(tmpl.Transform) == 0 {
		return nil
	}
	if slices.Contains(capabilities, "CAPABILITY_AUTO_EXPAND") {
		return nil
	}

	return ErrInsufficientCapabilities
}

// validateStackOptions validates the options for CreateStack and UpdateStack.
// It checks RoleARN format, IAM capability requirements, and (for templates
// with a top-level Transform) the CAPABILITY_AUTO_EXPAND requirement.
func validateStackOptions(templateBody string, opts StackOptions) error {
	if err := ValidateRoleARN(opts.RoleARN); err != nil {
		return err
	}

	if err := requireIAMCapability(templateBody, opts.Capabilities); err != nil {
		return err
	}

	return requireAutoExpandCapability(templateBody, opts.Capabilities)
}
