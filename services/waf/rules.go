package waf

import (
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) ruleARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("rule/%s", id))
}

// CreateRule creates a new Rule.
func (b *InMemoryBackend) CreateRule(
	name, metricName, changeToken string,
	tags map[string]string,
) (*Rule, error) {
	b.mu.Lock("CreateRule")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	rule := &Rule{
		RuleId:     id,
		Name:       name,
		MetricName: metricName,
		Predicates: []Predicate{},
	}
	b.rules.Put(rule)

	if len(tags) > 0 {
		b.tags[b.ruleARN(id)] = maps.Clone(tags)
	}

	return rule, nil
}

// GetRule retrieves a Rule by ID.
func (b *InMemoryBackend) GetRule(id string) (*Rule, error) {
	b.mu.RLock("GetRule")
	defer b.mu.RUnlock()

	rule, ok := b.rules.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return rule, nil
}

// UpdateRule updates a Rule's predicates.
func (b *InMemoryBackend) UpdateRule(id, changeToken string, updates []RuleUpdate) error {
	b.mu.Lock("UpdateRule")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rule, ok := b.rules.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		predicates, err := applyEntryUpdate(rule.Predicates, u.Action, u.Predicate,
			func(a, b Predicate) bool { return a.DataId == b.DataId && a.Type == b.Type })
		if err != nil {
			return err
		}

		rule.Predicates = predicates
	}

	return nil
}

// DeleteRule deletes a Rule. Real AWS rejects deletion while the Rule is
// still activated in a WebACL/RuleGroup (WAFReferencedItemException) or
// still contains any Predicates (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteRule(id, changeToken string) error {
	b.mu.Lock("DeleteRule")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rule, ok := b.rules.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.ruleReferenced(id) {
		return ErrReferencedItem
	}

	if len(rule.Predicates) > 0 {
		return ErrNonEmptyEntity
	}

	b.rules.Delete(id)
	delete(b.tags, b.ruleARN(id))

	return nil
}

// ListRules returns summaries of all Rules.
func (b *InMemoryBackend) ListRules() []RuleSummary {
	b.mu.RLock("ListRules")
	defer b.mu.RUnlock()

	all := b.rules.All()
	result := make([]RuleSummary, 0, len(all))
	for _, r := range all {
		result = append(result, RuleSummary{RuleId: r.RuleId, Name: r.Name})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].RuleId < result[j].RuleId })

	return result
}
