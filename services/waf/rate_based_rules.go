package waf

import (
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) rateBasedRuleARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("ratebasedrule/%s", id))
}

// CreateRateBasedRule creates a new RateBasedRule.
func (b *InMemoryBackend) CreateRateBasedRule(
	name, metricName, rateKey string,
	rateLimit int64,
	changeToken string,
	tags map[string]string,
) (*RateBasedRule, error) {
	b.mu.Lock("CreateRateBasedRule")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	rule := &RateBasedRule{
		RuleId:          id,
		Name:            name,
		MetricName:      metricName,
		RateKey:         rateKey,
		RateLimit:       rateLimit,
		MatchPredicates: []Predicate{},
	}
	b.rateBasedRules.Put(rule)

	if len(tags) > 0 {
		b.tags[b.rateBasedRuleARN(id)] = maps.Clone(tags)
	}

	return rule, nil
}

// GetRateBasedRule retrieves a RateBasedRule by ID.
func (b *InMemoryBackend) GetRateBasedRule(id string) (*RateBasedRule, error) {
	b.mu.RLock("GetRateBasedRule")
	defer b.mu.RUnlock()

	rule, ok := b.rateBasedRules.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return rule, nil
}

// UpdateRateBasedRule updates a RateBasedRule's predicates and rate limit.
func (b *InMemoryBackend) UpdateRateBasedRule(
	id, changeToken string,
	rateLimit int64,
	updates []RuleUpdate,
) error {
	b.mu.Lock("UpdateRateBasedRule")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rule, ok := b.rateBasedRules.Get(id)
	if !ok {
		return ErrNotFound
	}

	if rateLimit > 0 {
		rule.RateLimit = rateLimit
	}

	for _, u := range updates {
		predicates, err := applyEntryUpdate(rule.MatchPredicates, u.Action, u.Predicate,
			func(a, b Predicate) bool { return a.DataId == b.DataId && a.Type == b.Type })
		if err != nil {
			return err
		}

		rule.MatchPredicates = predicates
	}

	return nil
}

// DeleteRateBasedRule deletes a RateBasedRule. Real AWS rejects deletion
// while the rule is still activated in a WebACL (WAFReferencedItemException)
// or still contains any MatchPredicates (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteRateBasedRule(id, changeToken string) error {
	b.mu.Lock("DeleteRateBasedRule")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rule, ok := b.rateBasedRules.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.ruleReferenced(id) {
		return ErrReferencedItem
	}

	if len(rule.MatchPredicates) > 0 {
		return ErrNonEmptyEntity
	}

	b.rateBasedRules.Delete(id)
	delete(b.tags, b.rateBasedRuleARN(id))

	return nil
}

// ListRateBasedRules returns summaries of all RateBasedRules.
func (b *InMemoryBackend) ListRateBasedRules() []RateBasedRuleSummary {
	b.mu.RLock("ListRateBasedRules")
	defer b.mu.RUnlock()

	all := b.rateBasedRules.All()
	result := make([]RateBasedRuleSummary, 0, len(all))
	for _, r := range all {
		result = append(result, RateBasedRuleSummary{RuleId: r.RuleId, Name: r.Name})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].RuleId < result[j].RuleId })

	return result
}

// GetRateBasedRuleManagedKeys returns the IP addresses currently blocked by a rate-based rule (stub).
func (b *InMemoryBackend) GetRateBasedRuleManagedKeys(id string) ([]string, error) {
	b.mu.RLock("GetRateBasedRuleManagedKeys")
	defer b.mu.RUnlock()

	if !b.rateBasedRules.Has(id) {
		return nil, ErrNotFound
	}

	return []string{}, nil
}
