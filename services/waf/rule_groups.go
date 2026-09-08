package waf

import (
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) ruleGroupARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("rulegroup/%s", id))
}

// CreateRuleGroup creates a new RuleGroup.
func (b *InMemoryBackend) CreateRuleGroup(
	name, metricName, changeToken string,
	tags map[string]string,
) (*RuleGroup, error) {
	b.mu.Lock("CreateRuleGroup")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	rg := &RuleGroup{
		RuleGroupId: id,
		Name:        name,
		MetricName:  metricName,
	}
	b.ruleGroups.Put(rg)
	b.ruleGroupRules[id] = []ActivatedRule{}

	if len(tags) > 0 {
		b.tags[b.ruleGroupARN(id)] = maps.Clone(tags)
	}

	return rg, nil
}

// GetRuleGroup retrieves a RuleGroup by ID.
func (b *InMemoryBackend) GetRuleGroup(id string) (*RuleGroup, error) {
	b.mu.RLock("GetRuleGroup")
	defer b.mu.RUnlock()

	rg, ok := b.ruleGroups.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return rg, nil
}

// UpdateRuleGroup updates a RuleGroup's activated rules.
func (b *InMemoryBackend) UpdateRuleGroup(id, changeToken string, updates []ActivatedRuleUpdate) error {
	b.mu.Lock("UpdateRuleGroup")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	if !b.ruleGroups.Has(id) {
		return ErrNotFound
	}

	rules := b.ruleGroupRules[id]
	active := make(map[string]bool, len(rules))
	for _, r := range rules {
		active[r.RuleId] = true
	}

	for _, u := range updates {
		switch u.Action {
		case updateInsert:
			if active[u.ActivatedRule.RuleId] {
				return fmt.Errorf("%w: rule %q is already activated in this RuleGroup",
					ErrInvalidOperation, u.ActivatedRule.RuleId)
			}

			active[u.ActivatedRule.RuleId] = true
			rules = append(rules, u.ActivatedRule)
		case updateDelete:
			if !active[u.ActivatedRule.RuleId] {
				return fmt.Errorf("%w: rule %q isn't activated in this RuleGroup",
					ErrInvalidOperation, u.ActivatedRule.RuleId)
			}

			delete(active, u.ActivatedRule.RuleId)

			filtered := rules[:0]
			for _, r := range rules {
				if r.RuleId != u.ActivatedRule.RuleId {
					filtered = append(filtered, r)
				}
			}
			rules = filtered
		}
	}

	sort.Slice(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	b.ruleGroupRules[id] = rules

	return nil
}

// DeleteRuleGroup deletes a RuleGroup. Real AWS rejects deletion while the
// RuleGroup is still activated in a WebACL (WAFReferencedItemException) or
// still contains any activated rules (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteRuleGroup(id, changeToken string) error {
	b.mu.Lock("DeleteRuleGroup")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	if !b.ruleGroups.Has(id) {
		return ErrNotFound
	}

	if b.ruleReferenced(id) {
		return ErrReferencedItem
	}

	if len(b.ruleGroupRules[id]) > 0 {
		return ErrNonEmptyEntity
	}

	b.ruleGroups.Delete(id)
	delete(b.ruleGroupRules, id)
	delete(b.tags, b.ruleGroupARN(id))

	return nil
}

// ListRuleGroups returns summaries of all RuleGroups.
func (b *InMemoryBackend) ListRuleGroups() []RuleGroupSummary {
	b.mu.RLock("ListRuleGroups")
	defer b.mu.RUnlock()

	all := b.ruleGroups.All()
	result := make([]RuleGroupSummary, 0, len(all))
	for _, rg := range all {
		result = append(result, RuleGroupSummary{RuleGroupId: rg.RuleGroupId, Name: rg.Name})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].RuleGroupId < result[j].RuleGroupId })

	return result
}

// ListActivatedRulesInRuleGroup returns the activated rules for a RuleGroup.
func (b *InMemoryBackend) ListActivatedRulesInRuleGroup(id string) ([]ActivatedRule, error) {
	b.mu.RLock("ListActivatedRulesInRuleGroup")
	defer b.mu.RUnlock()

	if !b.ruleGroups.Has(id) {
		return nil, ErrNotFound
	}

	rules := b.ruleGroupRules[id]
	result := make([]ActivatedRule, len(rules))
	copy(result, rules)

	return result, nil
}

// ListSubscribedRuleGroups returns subscribed rule groups (always empty in mock).
func (b *InMemoryBackend) ListSubscribedRuleGroups() []SubscribedRuleGroupSummary {
	return []SubscribedRuleGroupSummary{}
}
