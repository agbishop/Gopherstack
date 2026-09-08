package eventbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// CreateEventBusParams holds the input for CreateEventBus.
type CreateEventBusParams struct {
	DeadLetterConfig *DeadLetterConfig
	LogConfig        *LogConfig
	Name             string
	Description      string
	KmsKeyIdentifier string
}

// CreateEventBus creates a new event bus.
func (b *InMemoryBackend) CreateEventBus(ctx context.Context, params CreateEventBusParams) (*EventBus, error) {
	name := params.Name
	description := params.Description
	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrInvalidParameter)
	}

	if len(name) > maxEventBusNameLength {
		return nil, fmt.Errorf(
			"%w: Name must be %d characters or fewer",
			ErrInvalidParameter,
			maxEventBusNameLength,
		)
	}

	if strings.HasPrefix(name, "aws.") {
		return nil, fmt.Errorf(
			"%w: Event bus name cannot start with the reserved prefix \"aws.\"",
			ErrInvalidParameter,
		)
	}

	region := getRegionFromContext(ctx, b.region)

	b.mu.Lock("CreateEventBus")
	defer b.mu.Unlock()

	buses := b.busesTable(region)
	if buses.Has(ebBusKey(name)) {
		return nil, fmt.Errorf("%w: Event bus %s already exists", ErrEventBusAlreadyExists, name)
	}

	// Count custom buses across all regions — the AWS limit is per-account, not per-region.
	customBusCount := 0
	for _, regionBuses := range b.buses {
		for _, bus := range regionBuses.All() {
			if bus.Name != defaultEventBusName {
				customBusCount++
			}
		}
	}
	if customBusCount >= maxEventBusesPerAccount {
		return nil, fmt.Errorf(
			"%w: account has reached the maximum of %d custom event buses",
			ErrResourceLimitExceeded,
			maxEventBusesPerAccount,
		)
	}

	now := time.Now()
	bus := &EventBus{
		Name:             name,
		Arn:              b.busARN(region, name),
		Description:      description,
		DeadLetterConfig: params.DeadLetterConfig,
		KmsKeyIdentifier: params.KmsKeyIdentifier,
		LogConfig:        params.LogConfig,
		CreatedTime:      now,
		LastModifiedTime: now,
	}
	buses.Put(bus)

	return bus, nil
}

// DeleteEventBus deletes an event bus by name (default bus cannot be deleted).
// It also removes all rules and targets associated with the bus.
func (b *InMemoryBackend) DeleteEventBus(ctx context.Context, name string) error {
	if name == defaultEventBusName {
		return fmt.Errorf("%w: cannot delete the default event bus", ErrCannotDeleteDefaultBus)
	}

	region := getRegionFromContext(ctx, b.region)

	b.mu.Lock("DeleteEventBus")
	defer b.mu.Unlock()

	busKey := ebBusKey(name)
	buses := b.busesTable(region)
	if !buses.Has(busKey) {
		return fmt.Errorf("%w: Event bus %s not found", ErrEventBusNotFound, name)
	}

	buses.Delete(busKey)
	delete(b.ruleIndexStore(region), busKey)
	delete(b.busePoliciesStore(region), busKey)

	// Clean up all rules and targets for this bus.
	rules := b.rulesStore(region)
	targets := b.targetsStore(region)
	if busRules, ok := rules[busKey]; ok {
		for _, rule := range busRules.All() {
			tk := b.targetKey(name, rule.Name)
			b.arnIndexRemoveRule(region, tk, targets[tk])
			delete(targets, tk)
		}

		delete(rules, busKey)
	}

	return nil
}

// ListEventBuses returns event buses optionally filtered by name prefix, with pagination.
// limit controls the page size; 0 uses the backend default (100).
func (b *InMemoryBackend) ListEventBuses(
	ctx context.Context,
	namePrefix, nextToken string,
	limit int,
) ([]EventBus, string, error) {
	region := getRegionFromContext(ctx, b.region)

	b.mu.RLock("ListEventBuses")
	defer b.mu.RUnlock()

	all := make([]EventBus, 0)
	for _, bus := range b.busesTable(region).All() {
		if namePrefix == "" || strings.HasPrefix(bus.Name, namePrefix) {
			all = append(all, *bus)
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	page, outToken := paginateN(all, nextToken, limit)

	return page, outToken, nil
}

// DescribeEventBus returns details for a single event bus.
func (b *InMemoryBackend) DescribeEventBus(ctx context.Context, name string) (*EventBus, error) {
	if name == "" {
		name = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)

	b.mu.RLock("DescribeEventBus")
	defer b.mu.RUnlock()

	bus, exists := b.busesTable(region).Get(ebBusKey(name))
	if !exists {
		return nil, fmt.Errorf("%w: Event bus %s not found", ErrEventBusNotFound, name)
	}

	cp := *bus

	return &cp, nil
}

// UpdateEventBus updates an existing event bus description.
func (b *InMemoryBackend) UpdateEventBus(ctx context.Context, input UpdateEventBusInput) (*EventBus, error) {
	busName := input.Name
	if busName == "" {
		busName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(busName)

	b.mu.Lock("UpdateEventBus")
	defer b.mu.Unlock()

	bus, exists := b.busesTable(region).Get(busKey)
	if !exists {
		return nil, fmt.Errorf("%w: event bus %s not found", ErrEventBusNotFound, busName)
	}

	bus.Description = input.Description
	bus.DeadLetterConfig = input.DeadLetterConfig
	bus.KmsKeyIdentifier = input.KmsKeyIdentifier
	bus.LogConfig = input.LogConfig
	bus.LastModifiedTime = time.Now()

	cp := *bus

	return &cp, nil
}

// PutPermission adds or replaces a resource-based policy statement on an event bus.
func (b *InMemoryBackend) PutPermission(ctx context.Context, input PutPermissionInput) error {
	busName := input.EventBusName
	if busName == "" {
		busName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(busName)

	b.mu.Lock("PutPermission")
	defer b.mu.Unlock()

	if _, exists := b.busesTable(region).Get(busKey); !exists {
		return fmt.Errorf("%w: event bus %s not found", ErrEventBusNotFound, busName)
	}

	policies := b.busePoliciesStore(region)
	policy := policies[busKey]
	if policy == nil {
		policy = &EventBusPolicy{Statements: make(map[string]*EventBusPolicyStatement)}
		policies[busKey] = policy
	}

	// If a raw Policy JSON is provided it replaces the whole policy.
	if input.Policy != "" {
		var stmts []EventBusPolicyStatement
		if err := json.Unmarshal([]byte(input.Policy), &stmts); err == nil {
			policy.Statements = make(map[string]*EventBusPolicyStatement, len(stmts))
			for i := range stmts {
				s := stmts[i]
				policy.Statements[s.Sid] = &s
			}
		}

		return nil
	}

	if input.StatementID == "" {
		return fmt.Errorf("%w: StatementId is required", ErrInvalidParameter)
	}

	stmt := &EventBusPolicyStatement{
		Sid:       input.StatementID,
		Effect:    "Allow",
		Action:    input.Action,
		Principal: input.Principal,
	}
	if input.Condition != nil {
		stmt.Condition = map[string]map[string]string{
			input.Condition.Type: {input.Condition.Key: input.Condition.Value},
		}
	}
	policy.Statements[input.StatementID] = stmt

	return nil
}

// RemovePermission removes a resource-based policy statement from an event bus.
func (b *InMemoryBackend) RemovePermission(ctx context.Context, input RemovePermissionInput) error {
	busName := input.EventBusName
	if busName == "" {
		busName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(busName)

	b.mu.Lock("RemovePermission")
	defer b.mu.Unlock()

	if _, exists := b.busesTable(region).Get(busKey); !exists {
		return fmt.Errorf("%w: event bus %s not found", ErrEventBusNotFound, busName)
	}

	policies := b.busePoliciesStore(region)
	if input.RemoveAllPermissions {
		delete(policies, busKey)

		return nil
	}

	policy := policies[busKey]
	if policy == nil {
		return nil
	}
	delete(policy.Statements, input.StatementID)

	return nil
}

// GetEventBusPolicy returns the resource-based policy for an event bus as JSON.
func (b *InMemoryBackend) GetEventBusPolicy(ctx context.Context, eventBusName string) (string, error) {
	busName := eventBusName
	if busName == "" {
		busName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(busName)

	b.mu.RLock("GetEventBusPolicy")
	defer b.mu.RUnlock()

	if _, exists := b.busesTable(region).Get(busKey); !exists {
		return "", fmt.Errorf("%w: event bus %s not found", ErrEventBusNotFound, busName)
	}

	policy := b.busePoliciesStoreRO(region)[busKey]
	if policy == nil || len(policy.Statements) == 0 {
		return "", nil
	}

	stmts := make([]*EventBusPolicyStatement, 0, len(policy.Statements))
	for _, s := range policy.Statements {
		stmts = append(stmts, s)
	}
	data, err := json.Marshal(stmts)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// PutEventBusPolicy replaces the resource-based policy on an event bus with raw JSON.
func (b *InMemoryBackend) PutEventBusPolicy(ctx context.Context, input PutEventBusPolicyInput) error {
	busName := input.EventBusName
	if busName == "" {
		busName = defaultEventBusName
	}

	region := getRegionFromContext(ctx, b.region)
	busKey := ebBusKey(busName)

	b.mu.Lock("PutEventBusPolicy")
	defer b.mu.Unlock()

	if _, exists := b.busesTable(region).Get(busKey); !exists {
		return fmt.Errorf("%w: event bus %s not found", ErrEventBusNotFound, busName)
	}

	policies := b.busePoliciesStore(region)
	if input.Policy == "" {
		delete(policies, busKey)

		return nil
	}

	var stmts []EventBusPolicyStatement
	if err := json.Unmarshal([]byte(input.Policy), &stmts); err != nil {
		return fmt.Errorf("%w: Policy must be valid JSON: %w", ErrInvalidParameter, err)
	}

	policy := &EventBusPolicy{Statements: make(map[string]*EventBusPolicyStatement, len(stmts))}
	for i := range stmts {
		s := stmts[i]
		policy.Statements[s.Sid] = &s
	}
	policies[busKey] = policy

	return nil
}
