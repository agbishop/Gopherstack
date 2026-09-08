package stepfunctions

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// aliasParentStateMachineLocked returns the state machine that owns
// aliasARN, or (nil, false) if no state machine currently claims it (e.g.
// DeleteStateMachine already removed the mapping). Caller must hold b.mu.
func (b *InMemoryBackend) aliasParentStateMachineLocked(aliasARN string) (*StateMachine, bool) {
	for smARN, aARNs := range b.smAliases {
		if slices.Contains(aARNs, aliasARN) {
			return b.stateMachines.Get(smARN)
		}
	}

	return nil, false
}

// validateRoutingConfig enforces AWS alias routing constraints:
// 1-2 entries, each weight 0-100, total weight = 100.
func validateRoutingConfig(routing []AliasRoutingConfig) error {
	if len(routing) == 0 || len(routing) > 2 {
		return fmt.Errorf(
			"%w: routing configuration must have 1 or 2 entries",
			ErrInvalidRoutingConfiguration,
		)
	}

	total := 0

	for _, r := range routing {
		if r.Weight < 0 || r.Weight > 100 {
			return fmt.Errorf(
				"%w: each routing weight must be between 0 and 100",
				ErrInvalidRoutingConfiguration,
			)
		}

		total += r.Weight
	}

	const totalWeight = 100
	if total != totalWeight {
		return fmt.Errorf(
			"%w: routing weights must sum to 100, got %d",
			ErrInvalidRoutingConfiguration,
			total,
		)
	}

	return nil
}

// CreateStateMachineAlias creates a named routing alias for one or more state machine versions.
func (b *InMemoryBackend) CreateStateMachineAlias(
	smARN, name, description string,
	routing []AliasRoutingConfig,
) (*StateMachineAlias, error) {
	if err := validateRoutingConfig(routing); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateStateMachineAlias")
	defer b.mu.Unlock()

	sm, exists := b.stateMachines.Get(smARN)
	if !exists {
		// AWS: CreateStateMachineAlias's own error switch models ResourceNotFound
		// for a missing state machine, not StateMachineDoesNotExist -- reuse the
		// alias family's not-found sentinel, which already maps there.
		return nil, fmt.Errorf("%w: %s", ErrStateMachineAliasDoesNotExist, smARN)
	}

	if sm.Status == statusDeleting {
		return nil, fmt.Errorf("%w: %s", ErrStateMachineDeleting, smARN)
	}

	aARN := b.aliasARN(smARN, sm.Name, name)

	if b.aliases.Has(aARN) {
		return nil, fmt.Errorf("%w: %s", ErrStateMachineAliasAlreadyExists, name)
	}

	now := float64(time.Now().Unix())
	alias := &StateMachineAlias{
		StateMachineAliasArn: aARN,
		Name:                 name,
		Description:          description,
		RoutingConfiguration: routing,
		CreationDate:         now,
		// AWS: "the date the state machine alias was last updated. For a
		// newly created state machine, this is the same as the creation
		// date" (DescribeStateMachineAliasOutput.UpdateDate).
		UpdatedDate: now,
	}

	b.aliases.Put(alias)
	b.smAliases[smARN] = append(b.smAliases[smARN], aARN)

	cp := *alias

	return &cp, nil
}

// UpdateStateMachineAlias updates an alias's description and/or routing configuration.
func (b *InMemoryBackend) UpdateStateMachineAlias(
	aliasARN, description string,
	routing []AliasRoutingConfig,
) (*StateMachineAlias, error) {
	b.mu.Lock("UpdateStateMachineAlias")
	defer b.mu.Unlock()

	alias, exists := b.aliases.Get(aliasARN)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrStateMachineAliasDoesNotExist, aliasARN)
	}

	if sm, ok := b.aliasParentStateMachineLocked(aliasARN); ok && sm.Status == statusDeleting {
		return nil, fmt.Errorf("%w: %s", ErrStateMachineDeleting, aliasARN)
	}

	if description != "" {
		alias.Description = description
	}

	if len(routing) > 0 {
		if err := validateRoutingConfig(routing); err != nil {
			return nil, err
		}

		alias.RoutingConfiguration = routing
	}

	alias.UpdatedDate = float64(time.Now().Unix())

	cp := *alias

	return &cp, nil
}

// DeleteStateMachineAlias removes a state machine alias.
func (b *InMemoryBackend) DeleteStateMachineAlias(aliasARN string) error {
	b.mu.Lock("DeleteStateMachineAlias")
	defer b.mu.Unlock()

	if !b.aliases.Has(aliasARN) {
		return fmt.Errorf("%w: %s", ErrStateMachineAliasDoesNotExist, aliasARN)
	}

	// Remove from the SM's alias list — find parent SM ARN via alias's routing or by scanning smAliases.
	for smARN, aARNs := range b.smAliases {
		updated := make([]string, 0, len(aARNs))
		for _, aARN := range aARNs {
			if aARN != aliasARN {
				updated = append(updated, aARN)
			}
		}
		if len(updated) != len(aARNs) {
			b.smAliases[smARN] = updated
		}
	}

	b.aliases.Delete(aliasARN)

	return nil
}

// DescribeStateMachineAlias returns details for a state machine alias.
func (b *InMemoryBackend) DescribeStateMachineAlias(aliasARN string) (*StateMachineAlias, error) {
	b.mu.RLock("DescribeStateMachineAlias")
	defer b.mu.RUnlock()

	alias, exists := b.aliases.Get(aliasARN)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrStateMachineAliasDoesNotExist, aliasARN)
	}

	cp := *alias

	return &cp, nil
}

// ListStateMachineAliases returns all aliases for a state machine.
func (b *InMemoryBackend) ListStateMachineAliases(
	smARN, nextToken string, maxResults int,
) ([]StateMachineAlias, string, error) {
	b.mu.RLock("ListStateMachineAliases")
	defer b.mu.RUnlock()

	sm, exists := b.stateMachines.Get(smARN)
	if !exists {
		return nil, "", fmt.Errorf("%w: %s", ErrStateMachineDoesNotExist, smARN)
	}

	if sm.Status == statusDeleting {
		return nil, "", fmt.Errorf("%w: %s", ErrStateMachineDeleting, smARN)
	}

	aARNs := b.smAliases[smARN]
	all := make([]StateMachineAlias, 0, len(aARNs))
	for _, aARN := range aARNs {
		if a, ok := b.aliases.Get(aARN); ok {
			all = append(all, *a)
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	aliases, token := paginate(all, nextToken, maxResults)

	return aliases, token, nil
}
