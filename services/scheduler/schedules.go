package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// ScheduleOption applies an optional field to a schedule during creation or update.
type ScheduleOption func(*Schedule)

// applyScheduleOptions applies all options to the schedule.
func applyScheduleOptions(opts []ScheduleOption, s *Schedule) {
	for _, o := range opts {
		o(s)
	}
}

// WithStartDate sets the optional start date on a schedule.
func WithStartDate(t time.Time) ScheduleOption {
	return func(s *Schedule) { s.StartDate = &t }
}

// WithEndDate sets the optional end date on a schedule.
func WithEndDate(t time.Time) ScheduleOption {
	return func(s *Schedule) { s.EndDate = &t }
}

// WithActionAfterCompletion sets what happens after the schedule completes (DELETE or NONE).
func WithActionAfterCompletion(action string) ScheduleOption {
	return func(s *Schedule) { s.ActionAfterCompletion = action }
}

// WithKmsKeyArn sets an optional customer-managed KMS key ARN on a schedule.
func WithKmsKeyArn(arn string) ScheduleOption {
	return func(s *Schedule) { s.KmsKeyArn = arn }
}

// scheduleKey returns the composite map key for a schedule: "groupName/name".
func scheduleKey(groupName, name string) string {
	return groupName + "/" + name
}

// validateScheduleFields validates the fields shared by CreateSchedule and
// UpdateSchedule: the schedule expression, target, flexible time window,
// state, and timezone. It returns the first validation error encountered, in
// the same order both callers previously checked them.
func validateScheduleFields(
	expr string,
	target Target,
	state string,
	ftw FlexibleTimeWindow,
	timezone string,
) error {
	if expr == "" {
		return fmt.Errorf("%w: ScheduleExpression is required", ErrValidation)
	}

	if err := validateScheduleExpression(expr); err != nil {
		return err
	}

	if target.ARN == "" {
		return fmt.Errorf("%w: Target.Arn is required", ErrValidation)
	}

	if target.RoleARN == "" {
		return fmt.Errorf("%w: Target.RoleArn is required", ErrValidation)
	}

	if ftw.Mode == "" {
		return fmt.Errorf("%w: FlexibleTimeWindow.Mode is required", ErrValidation)
	}

	if err := validateScheduleState(state); err != nil {
		return err
	}

	if err := validateFlexibleTimeWindow(ftw); err != nil {
		return err
	}

	if err := validateTarget(target); err != nil {
		return err
	}

	if err := validateTimezone(timezone); err != nil {
		return err
	}

	return nil
}

// CreateSchedule creates a new schedule in the named group.
func (b *InMemoryBackend) CreateSchedule(
	ctx context.Context,
	name, groupName, expr, description, timezone string,
	target Target,
	state string,
	ftw FlexibleTimeWindow,
	opts ...ScheduleOption,
) (*Schedule, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	if err := validateScheduleFields(expr, target, state, ftw, timezone); err != nil {
		return nil, err
	}

	if groupName == "" {
		groupName = defaultGroupName
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateSchedule")
	defer b.mu.Unlock()

	b.ensureRegionGroupsSeeded(region)

	if _, ok := b.scheduleGroups.Get(regionKey(region, groupName)); !ok {
		return nil, fmt.Errorf("%w: schedule group %s not found", ErrNotFound, groupName)
	}

	tableKey := regionKey(region, scheduleKey(groupName, name))
	if b.schedules.Has(tableKey) {
		return nil, fmt.Errorf("%w: schedule %s already exists in group %s", ErrAlreadyExists, name, groupName)
	}

	schedARN := arn.Build("scheduler", region, b.accountID, "schedule/"+groupName+"/"+name)
	now := time.Now().UTC()
	s := &Schedule{
		Name:                       name,
		GroupName:                  groupName,
		ARN:                        schedARN,
		ScheduleExpression:         expr,
		ScheduleExpressionTimezone: timezone,
		Description:                description,
		Target:                     target,
		State:                      state,
		FlexibleTimeWindow:         ftw,
		AccountID:                  b.accountID,
		Region:                     region,
		CreationDate:               now,
		LastModificationDate:       now,
		Tags:                       tags.New("scheduler.schedule." + groupName + "." + name + ".tags"),
	}
	applyScheduleOptions(opts, s)
	b.schedules.Put(s)

	return cloneSchedule(s), nil
}

// GetSchedule returns a schedule by name and group.
func (b *InMemoryBackend) GetSchedule(ctx context.Context, name, groupName string) (*Schedule, error) {
	if groupName == "" {
		groupName = defaultGroupName
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("GetSchedule")
	defer b.mu.RUnlock()

	s, ok := b.schedules.Get(regionKey(region, scheduleKey(groupName, name)))
	if !ok {
		return nil, fmt.Errorf("%w: schedule %s not found", ErrNotFound, name)
	}

	return cloneSchedule(s), nil
}

// ListSchedules returns schedules optionally filtered by group name, name prefix, and state.
// When maxResults > 0 and nextToken is non-empty it resumes after the token (last seen name).
// Returns the page of schedules and the next continuation token (empty when no more results).
func (b *InMemoryBackend) ListSchedules(
	ctx context.Context,
	groupName, namePrefix, state, nextToken string,
	maxResults int,
) ([]*Schedule, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListSchedules")
	defer b.mu.RUnlock()

	schedules := b.schedulesByRegion.Get(region)
	list := make([]*Schedule, 0, len(schedules))

	for _, s := range schedules {
		if groupName != "" && s.GroupName != groupName {
			continue
		}

		if namePrefix != "" && !strings.HasPrefix(s.Name, namePrefix) {
			continue
		}

		if state != "" && s.State != state {
			continue
		}

		list = append(list, cloneSchedule(s))
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return paginate(list, func(s *Schedule) string { return s.GroupName + "/" + s.Name }, nextToken, maxResults)
}

// DeleteSchedule removes a schedule by name and group.
func (b *InMemoryBackend) DeleteSchedule(ctx context.Context, name, groupName string) error {
	if groupName == "" {
		groupName = defaultGroupName
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteSchedule")
	defer b.mu.Unlock()

	tableKey := regionKey(region, scheduleKey(groupName, name))

	s, ok := b.schedules.Get(tableKey)
	if !ok {
		return fmt.Errorf("%w: schedule %s not found", ErrNotFound, name)
	}

	b.schedules.Delete(tableKey)
	s.Tags.Close()

	return nil
}

// UpdateSchedule updates an existing schedule.
func (b *InMemoryBackend) UpdateSchedule(
	ctx context.Context,
	name, groupName, expr, description, timezone string,
	target Target,
	state string,
	ftw FlexibleTimeWindow,
	opts ...ScheduleOption,
) (*Schedule, error) {
	if err := validateScheduleFields(expr, target, state, ftw, timezone); err != nil {
		return nil, err
	}

	if groupName == "" {
		groupName = defaultGroupName
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateSchedule")
	defer b.mu.Unlock()

	s, ok := b.schedules.Get(regionKey(region, scheduleKey(groupName, name)))
	if !ok {
		return nil, fmt.Errorf("%w: schedule %s not found", ErrNotFound, name)
	}

	s.ScheduleExpression = expr
	s.ScheduleExpressionTimezone = timezone
	s.Description = description
	s.Target = target
	// State is optional on UpdateSchedule (unlike CreateSchedule, which defaults an
	// omitted State to ENABLED in the handler): the real UpdateScheduleInput document
	// serializer omits the "State" JSON key entirely when it's the zero value, so
	// real clients can update a schedule without touching its enabled/disabled
	// status. Overwriting with "" here would leave the schedule in an invalid state
	// that matches neither ENABLED nor DISABLED and silently stops the runner from
	// ever firing it again (see checkAndFireSchedules's `s.State != "ENABLED"` gate).
	if state != "" {
		s.State = state
	}
	s.FlexibleTimeWindow = ftw
	// UpdateSchedule is a full replacement (api_op_UpdateSchedule.go:16-19: "uses all
	// values, including empty values... if you do not set an optional field in your
	// request, that field will be set to its system-default value"). Unlike State
	// above, StartDate/EndDate/KmsKeyArn are true pointer/optional fields on the wire
	// (not ambiguous zero-value enums), so clear them here and let opts below
	// re-populate whatever the caller did specify.
	s.StartDate = nil
	s.EndDate = nil
	s.KmsKeyArn = ""
	s.LastModificationDate = time.Now().UTC()
	applyScheduleOptions(opts, s)

	return cloneSchedule(s), nil
}

// scheduleByARN resolves a schedule by its ARN within region via the byARN index,
// returning nil when no schedule matches. Callers must hold b.mu. An ARN uniquely
// identifies a schedule, so at most one entry is ever grouped under the index key.
func (b *InMemoryBackend) scheduleByARN(region, resourceARN string) *Schedule {
	if matches := b.schedulesByARN.Get(regionKey(region, resourceARN)); len(matches) > 0 {
		return matches[0]
	}

	return nil
}

// AddScheduleInternal inserts a schedule directly for testing purposes.
// Must only be used from test code.
func (b *InMemoryBackend) AddScheduleInternal(s *Schedule) {
	b.mu.Lock("AddScheduleInternal")
	defer b.mu.Unlock()

	if s.GroupName == "" {
		s.GroupName = defaultGroupName
	}

	if s.Tags == nil {
		s.Tags = tags.New("scheduler.schedule." + s.GroupName + "." + s.Name + ".tags")
	}

	if s.Region == "" {
		s.Region = b.region
	}

	// s.Region is the outer half of the composite table key (see schedulesKeyFn),
	// so it must be set before Put.
	b.schedules.Put(s)
}

// cloneSchedule returns a deep copy of a schedule (including a snapshot of its Tags).
func cloneSchedule(s *Schedule) *Schedule {
	cp := *s
	cp.Tags = nil

	if s.Tags != nil {
		cp.Tags = tags.FromMap("scheduler.schedule."+s.GroupName+"."+s.Name+".tags.clone", s.Tags.Clone())
	}

	// Deep-copy optional time pointer fields.
	if s.StartDate != nil {
		t := *s.StartDate
		cp.StartDate = &t
	}

	if s.EndDate != nil {
		t := *s.EndDate
		cp.EndDate = &t
	}

	return &cp
}

// validateScheduleState returns ErrValidation if state is not a valid value.
// An empty string is allowed (the handler sets a default).
func validateScheduleState(state string) error {
	switch state {
	case scheduleStateEnabled, scheduleStateDisabled, "":
		return nil
	default:
		return fmt.Errorf("%w: State must be ENABLED or DISABLED, got %q", ErrValidation, state)
	}
}

// validateActionAfterCompletion returns ErrValidation if action is not a valid value.
// An empty string is allowed (it means the default NONE behaviour).
func validateActionAfterCompletion(action string) error {
	switch action {
	case actionAfterCompletionNone, actionAfterCompletionDelete, "":
		return nil
	default:
		return fmt.Errorf("%w: ActionAfterCompletion must be NONE or DELETE, got %q", ErrValidation, action)
	}
}

// validateFlexibleTimeWindowMode returns ErrValidation if mode is not OFF or FLEXIBLE.
func validateFlexibleTimeWindowMode(mode string) error {
	switch mode {
	case flexibleTimeWindowModeOff, flexibleTimeWindowModeFlexible:
		return nil
	default:
		return fmt.Errorf("%w: FlexibleTimeWindow.Mode must be OFF or FLEXIBLE, got %q", ErrValidation, mode)
	}
}

// validateFlexibleTimeWindow validates both the mode and the required window size.
// When Mode is FLEXIBLE, MaximumWindowInMinutes must be positive (1–1440).
func validateFlexibleTimeWindow(ftw FlexibleTimeWindow) error {
	if err := validateFlexibleTimeWindowMode(ftw.Mode); err != nil {
		return err
	}

	if ftw.Mode == flexibleTimeWindowModeFlexible && ftw.MaximumWindowInMinutes <= 0 {
		return fmt.Errorf(
			"%w: FlexibleTimeWindow.MaximumWindowInMinutes is required and must be >= 1 when Mode is FLEXIBLE",
			ErrValidation,
		)
	}

	return nil
}

// validateTimezone returns ErrValidation if timezone is set but is not a resolvable
// IANA timezone name. AWS's ScheduleExpressionTimezone is "the timezone in which the
// scheduling expression is evaluated"; an unresolvable name can never be evaluated
// against wall-clock time by the runner (see Runner.cachedLocation), so it is
// rejected at write time rather than silently falling back to UTC. An empty string
// is allowed (defaults to UTC).
func validateTimezone(tz string) error {
	if tz == "" {
		return nil
	}

	if _, err := time.LoadLocation(tz); err != nil {
		return fmt.Errorf("%w: ScheduleExpressionTimezone %q is not a valid timezone", ErrValidation, tz)
	}

	return nil
}

// validateName checks that name is non-empty, matches [0-9a-zA-Z-_.], and is at most 64 chars.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if len(name) > scheduleNameMaxLen {
		return fmt.Errorf("%w: Name must be 1-64 characters, got %d", ErrValidation, len(name))
	}

	if !validNameRE.MatchString(name) {
		return fmt.Errorf("%w: Name must match [0-9a-zA-Z-_.], got %q", ErrValidation, name)
	}

	return nil
}

// validateRetryPolicy validates RetryPolicy field ranges.
func validateRetryPolicy(rp *RetryPolicy) error {
	if rp == nil {
		return nil
	}

	if rp.MaximumEventAgeInSeconds != 0 &&
		(rp.MaximumEventAgeInSeconds < retryPolicyMinEventAge || rp.MaximumEventAgeInSeconds > retryPolicyMaxEventAge) {
		return fmt.Errorf(
			"%w: RetryPolicy.MaximumEventAgeInSeconds must be 60-86400, got %d",
			ErrValidation,
			rp.MaximumEventAgeInSeconds,
		)
	}

	if rp.MaximumRetryAttempts < 0 || rp.MaximumRetryAttempts > retryPolicyMaxAttempts {
		return fmt.Errorf(
			"%w: RetryPolicy.MaximumRetryAttempts must be 0-185, got %d",
			ErrValidation,
			rp.MaximumRetryAttempts,
		)
	}

	return nil
}

// validateTarget validates target-specific parameter constraints.
func validateTarget(target Target) error {
	if err := validateRetryPolicy(target.RetryPolicy); err != nil {
		return err
	}

	if target.DeadLetterConfig != nil && target.DeadLetterConfig.Arn != "" {
		if !strings.HasPrefix(target.DeadLetterConfig.Arn, "arn:aws:sqs:") {
			return fmt.Errorf("%w: DeadLetterConfig.Arn must be an SQS ARN (arn:aws:sqs:...)", ErrValidation)
		}
	}

	if target.KinesisParameters != nil && target.KinesisParameters.PartitionKey == "" {
		return fmt.Errorf("%w: KinesisParameters.PartitionKey is required for Kinesis targets", ErrValidation)
	}

	if target.EventBridgeParameters != nil {
		if target.EventBridgeParameters.DetailType == "" {
			return fmt.Errorf("%w: EventBridgeParameters.DetailType is required", ErrValidation)
		}

		if target.EventBridgeParameters.Source == "" {
			return fmt.Errorf("%w: EventBridgeParameters.Source is required", ErrValidation)
		}
	}

	if target.EcsParameters != nil && target.EcsParameters.TaskDefinitionArn == "" {
		return fmt.Errorf("%w: EcsParameters.TaskDefinitionArn is required", ErrValidation)
	}

	return nil
}
