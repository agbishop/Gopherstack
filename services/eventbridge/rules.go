package eventbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// checkManagedRule returns ManagedRuleException if rule is owned by an AWS
// service (Rule.ManagedBy non-empty). Real AWS rejects PutRule (on an existing
// managed rule), DeleteRule, EnableRule, DisableRule, PutTargets and
// RemoveTargets against service-managed rules (e.g. rules EventBridge
// Scheduler creates on a caller's behalf) with this error.
func checkManagedRule(rule *Rule) error {
	if rule.ManagedBy != "" {
		return fmt.Errorf(
			"%w: rule %s is managed by %s and cannot be modified directly",
			ErrManagedRule,
			rule.Name,
			rule.ManagedBy,
		)
	}

	return nil
}

// validatePutRuleInput validates the rule input fields before any locking.
func validatePutRuleInput(input PutRuleInput) error {
	if input.Name == "" {
		return fmt.Errorf("%w: Name is required", ErrInvalidParameter)
	}

	const maxRuleNameLength = 64
	if len(input.Name) > maxRuleNameLength {
		return fmt.Errorf(
			"%w: Name must not exceed %d characters",
			ErrInvalidParameter,
			maxRuleNameLength,
		)
	}

	if input.EventPattern != "" && input.ScheduleExpression != "" {
		return fmt.Errorf(
			"%w: ScheduleExpression and EventPattern are mutually exclusive",
			ErrInvalidParameter,
		)
	}

	if input.EventPattern == "" && input.ScheduleExpression == "" {
		return fmt.Errorf(
			"%w: either EventPattern or ScheduleExpression must be provided",
			ErrInvalidParameter,
		)
	}

	if input.State != "" && !isValidRuleState(input.State) {
		return fmt.Errorf(
			"%w: State must be one of ENABLED, DISABLED, %s",
			ErrInvalidParameter,
			ruleStateEnabledAllCloudTrailMgmtEvents,
		)
	}

	if input.ScheduleExpression != "" {
		if _, err := parseScheduleExpression(input.ScheduleExpression); err != nil {
			return fmt.Errorf(
				"%w: invalid ScheduleExpression %q: %w",
				ErrInvalidParameter,
				input.ScheduleExpression,
				err,
			)
		}
	}

	return nil
}

// isValidRuleState reports whether state is one of RuleState's modeled
// values (see ruleStateEnabledAllCloudTrailMgmtEvents).
func isValidRuleState(state string) bool {
	switch state {
	case ruleStateEnabled, ruleStateDisabled, ruleStateEnabledAllCloudTrailMgmtEvents:
		return true
	default:
		return false
	}
}

// PutRule creates or updates a rule on an event bus.
func (b *InMemoryBackend) PutRule(ctx context.Context, input PutRuleInput) (*Rule, error) {
	if err := validatePutRuleInput(input); err != nil {
		return nil, err
	}

	region := getRegionFromContext(ctx, b.region)

	busName := input.EventBusName
	if busName == "" {
		busName = defaultEventBusName
	}

	var compiled *compiledPattern
	if input.EventPattern != "" {
		var err error
		compiled, err = b.getOrCompilePattern(input.EventPattern)
		if err != nil {
			return nil, fmt.Errorf("%w: EventPattern is not valid JSON", ErrInvalidParameter)
		}
	}

	busKey := ebBusKey(busName)

	b.mu.Lock("PutRule")
	defer b.mu.Unlock()

	if !b.busesTable(region).Has(busKey) {
		return nil, fmt.Errorf("%w: Event bus %s not found", ErrEventBusNotFound, busName)
	}

	state := input.State
	if state == "" {
		state = ruleStateEnabled
	}

	ruleTable := b.ruleTableFor(region, busKey)

	existing, exists := ruleTable.Get(input.Name)

	// Enforce per-bus rule limit only for new rules (not updates).
	if !exists {
		if ruleTable.Len() >= maxRulesPerBus {
			return nil, fmt.Errorf(
				"%w: event bus %s has reached the maximum of %d rules",
				ErrResourceLimitExceeded,
				busName,
				maxRulesPerBus,
			)
		}
	} else if err := checkManagedRule(existing); err != nil {
		return nil, err
	}

	createdBy := b.accountID
	if exists {
		createdBy = existing.CreatedBy
	}

	rule := &Rule{
		Name:               input.Name,
		Arn:                b.ruleARN(region, busName, input.Name),
		EventBusName:       busName,
		EventPattern:       input.EventPattern,
		State:              state,
		Description:        input.Description,
		ScheduleExpression: input.ScheduleExpression,
		RoleArn:            input.RoleArn,
		ManagedBy:          input.ManagedBy,
		CreatedBy:          createdBy,
		compiledPattern:    compiled,
	}

	if exists {
		b.removeRuleFromIndex(region, busKey, existing)
	}
	ruleTable.Put(rule)
	b.addRuleToIndex(region, busKey, rule)

	return rule, nil
}

// DeleteRule removes a rule from an event bus.
func (b *InMemoryBackend) DeleteRule(ctx context.Context, name, eventBusName string) error {
	if eventBusName == "" {
		eventBusName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(eventBusName)

	b.mu.Lock("DeleteRule")
	defer b.mu.Unlock()

	busRules, exists := b.rulesStore(region)[busKey]
	if !exists {
		return fmt.Errorf("%w: Rule %s not found", ErrRuleNotFound, name)
	}

	rule, ruleExists := busRules.Get(name)
	if !ruleExists {
		return fmt.Errorf("%w: Rule %s not found", ErrRuleNotFound, name)
	}

	if err := checkManagedRule(rule); err != nil {
		return err
	}

	b.removeRuleFromIndex(region, busKey, rule)
	busRules.Delete(name)
	// Also remove targets for this rule.
	targetKey := b.targetKey(eventBusName, name)
	b.arnIndexRemoveRule(region, targetKey, b.targetsStore(region)[targetKey])
	delete(b.targetsStore(region), targetKey)

	return nil
}

// ListRules returns rules for an event bus optionally filtered by name prefix.
// limit caps the page size (0 uses the default); AWS EventBridge honours the
// Limit request parameter, so it is threaded through to pagination here.
func (b *InMemoryBackend) ListRules(ctx context.Context,
	eventBusName, namePrefix, nextToken string, limit int,
) ([]Rule, string, error) {
	if eventBusName == "" {
		eventBusName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(eventBusName)

	b.mu.RLock("ListRules")
	defer b.mu.RUnlock()

	busRules := b.rulesStore(region)[busKey]
	var busRulesAll []*Rule
	if busRules != nil {
		busRulesAll = busRules.All()
	}
	all := make([]Rule, 0, len(busRulesAll))
	for _, r := range busRulesAll {
		if namePrefix == "" || strings.HasPrefix(r.Name, namePrefix) {
			all = append(all, *r)
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	page, outToken := paginateN(all, nextToken, limit)

	return page, outToken, nil
}

// DescribeRule returns a single rule.
func (b *InMemoryBackend) DescribeRule(ctx context.Context, name, eventBusName string) (*Rule, error) {
	if eventBusName == "" {
		eventBusName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(eventBusName)

	b.mu.RLock("DescribeRule")
	defer b.mu.RUnlock()

	busRules, exists := b.rulesStore(region)[busKey]
	if !exists {
		return nil, fmt.Errorf("%w: Rule %s not found", ErrRuleNotFound, name)
	}

	rule, exists := busRules.Get(name)
	if !exists {
		return nil, fmt.Errorf("%w: Rule %s not found", ErrRuleNotFound, name)
	}

	cp := *rule

	return &cp, nil
}

// EnableRule sets a rule's state to ENABLED.
func (b *InMemoryBackend) EnableRule(ctx context.Context, name, eventBusName string) error {
	return b.setRuleState(ctx, name, eventBusName, ruleStateEnabled)
}

// DisableRule sets a rule's state to DISABLED.
func (b *InMemoryBackend) DisableRule(ctx context.Context, name, eventBusName string) error {
	return b.setRuleState(ctx, name, eventBusName, ruleStateDisabled)
}

func (b *InMemoryBackend) setRuleState(ctx context.Context, name, eventBusName, state string) error {
	if eventBusName == "" {
		eventBusName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(eventBusName)

	b.mu.Lock("setRuleState")
	defer b.mu.Unlock()

	busRules, exists := b.rulesStore(region)[busKey]
	if !exists {
		return fmt.Errorf("%w: Rule %s not found", ErrRuleNotFound, name)
	}

	rule, exists := busRules.Get(name)
	if !exists {
		return fmt.Errorf("%w: Rule %s not found", ErrRuleNotFound, name)
	}

	if err := checkManagedRule(rule); err != nil {
		return err
	}

	rule.State = state

	return nil
}

// ListRuleNamesByTarget returns rule names that have a target matching the given ARN.
func (b *InMemoryBackend) ListRuleNamesByTarget(ctx context.Context,
	targetARN, eventBusName, nextToken string, limit int,
) ([]string, string, error) {
	if eventBusName == "" {
		eventBusName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)

	b.mu.RLock("ListRuleNamesByTarget")
	defer b.mu.RUnlock()

	// Use the ARN index for O(matched-rules) lookup instead of O(all-rules×all-targets).
	prefix := ebBusKey(eventBusName) + "/"
	var names []string
	for targetKey := range b.targetsByARN[region][targetARN] {
		if after, ok := strings.CutPrefix(targetKey, prefix); ok {
			names = append(names, after)
		}
	}

	sort.Strings(names)

	page, outToken := paginateN(names, nextToken, limit)

	return page, outToken, nil
}

// TestEventPattern tests an event pattern against an event JSON string.
func (b *InMemoryBackend) TestEventPattern(
	ctx context.Context, //nolint:revive // existing issue.
	pattern, event string,
) (bool, error) {
	if pattern == "" {
		return false, fmt.Errorf("%w: EventPattern is required", ErrInvalidParameter)
	}

	if event == "" {
		return false, fmt.Errorf("%w: Event is required", ErrInvalidParameter)
	}

	if !isValidJSON(event) {
		return false, fmt.Errorf("%w: Event must be valid JSON", ErrInvalidParameter)
	}

	compiled, err := b.getOrCompilePattern(pattern)
	if err != nil {
		return false, fmt.Errorf("%w: EventPattern is not valid JSON", ErrInvalidParameter)
	}

	return matchCompiledPattern(compiled, event), nil
}

// isValidJSON reports whether s is valid JSON.
func isValidJSON(s string) bool {
	var v any

	return json.Unmarshal([]byte(s), &v) == nil
}
