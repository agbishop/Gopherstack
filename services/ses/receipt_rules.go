package ses

import (
	"fmt"
	"sort"
	"strings"
)

// findRuleIndex returns the index of the rule with the given name, or -1 if not found.
func findRuleIndex(rules []ReceiptRule, name string) int {
	for i, r := range rules {
		if r.Name == name {
			return i
		}
	}

	return -1
}

// CreateReceiptRule adds a new rule to an existing receipt rule set.
func (b *InMemoryBackend) CreateReceiptRule(ruleSetName string, rule ReceiptRule, after string) error {
	if strings.TrimSpace(ruleSetName) == "" {
		return fmt.Errorf("%w: RuleSetName is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("%w: Rule.Name is required", ErrInvalidParameter)
	}

	if rule.TLSPolicy != "" && rule.TLSPolicy != TLSPolicyOptional && rule.TLSPolicy != TLSPolicyRequire {
		return fmt.Errorf("%w: TlsPolicy must be Optional or Require", ErrInvalidParameter)
	}

	b.mu.Lock("CreateReceiptRule")
	defer b.mu.Unlock()

	rs, exists := b.receiptRuleSets.Get(ruleSetName)
	if !exists {
		return fmt.Errorf("%w: %s", ErrReceiptRuleSetNotFound, ruleSetName)
	}

	for _, r := range rs.Rules {
		if r.Name == rule.Name {
			return fmt.Errorf("%w: rule %s already exists in rule set %s", ErrReceiptRuleExists, rule.Name, ruleSetName)
		}
	}

	if after == "" {
		rs.Rules = append([]ReceiptRule{rule}, rs.Rules...)

		return nil
	}

	idx := findRuleIndex(rs.Rules, after)

	if idx < 0 {
		return fmt.Errorf("%w: after rule %s not found", ErrReceiptRuleNotFound, after)
	}

	newRules := make([]ReceiptRule, 0, len(rs.Rules)+1)
	newRules = append(newRules, rs.Rules[:idx+1]...)
	newRules = append(newRules, rule)
	newRules = append(newRules, rs.Rules[idx+1:]...)
	rs.Rules = newRules

	return nil
}

// CreateReceiptFilter creates a new IP-based receipt filter.
func (b *InMemoryBackend) CreateReceiptFilter(filter ReceiptFilter) error {
	if strings.TrimSpace(filter.Name) == "" {
		return fmt.Errorf("%w: Filter.Name is required", ErrInvalidParameter)
	}

	if filter.Policy != "" && filter.Policy != FilterPolicyAllow && filter.Policy != FilterPolicyBlock {
		return fmt.Errorf("%w: Policy must be Allow or Block", ErrInvalidParameter)
	}

	b.mu.Lock("CreateReceiptFilter")
	defer b.mu.Unlock()

	if b.receiptFilters.Has(filter.Name) {
		return fmt.Errorf("%w: receipt filter %s already exists", ErrReceiptFilterExists, filter.Name)
	}

	f := filter
	b.receiptFilters.Put(&f)

	return nil
}

// ListReceiptFilters returns a sorted slice of all receipt filters.
func (b *InMemoryBackend) ListReceiptFilters() []ReceiptFilter {
	b.mu.RLock("ListReceiptFilters")
	defer b.mu.RUnlock()

	out := make([]ReceiptFilter, 0, b.receiptFilters.Len())
	for _, f := range b.receiptFilters.All() {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// DeleteReceiptFilter removes a receipt filter by name. A missing name is
// idempotent: DeleteReceiptFilter's own deserializer (ses@v1.37.4
// deserializers.go) declares no exception at all, and botocore's
// ses/2010-12-01 service-2.json has no "errors" key on this op whatsoever.
func (b *InMemoryBackend) DeleteReceiptFilter(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: Filter.Name is required", ErrInvalidParameter)
	}
	b.mu.Lock("DeleteReceiptFilter")
	defer b.mu.Unlock()
	b.receiptFilters.Delete(name)

	return nil
}

// DeleteReceiptRule removes a receipt rule from a rule set. The rule set
// itself must exist (RuleSetDoesNotExist), but a missing rule name is
// idempotent: DeleteReceiptRule's own deserializer (ses@v1.37.4
// deserializers.go) declares only RuleSetDoesNotExist, not RuleDoesNotExist,
// unlike Describe/Update on the same resource.
func (b *InMemoryBackend) DeleteReceiptRule(ruleSetName, ruleName string) error {
	if strings.TrimSpace(ruleSetName) == "" {
		return fmt.Errorf("%w: RuleSetName is required", ErrInvalidParameter)
	}
	if strings.TrimSpace(ruleName) == "" {
		return fmt.Errorf("%w: Rule.Name is required", ErrInvalidParameter)
	}
	b.mu.Lock("DeleteReceiptRule")
	defer b.mu.Unlock()
	rs, exists := b.receiptRuleSets.Get(ruleSetName)
	if !exists {
		return fmt.Errorf("%w: %s", ErrReceiptRuleSetNotFound, ruleSetName)
	}
	if idx := findRuleIndex(rs.Rules, ruleName); idx >= 0 {
		rs.Rules = append(rs.Rules[:idx], rs.Rules[idx+1:]...)
	}

	return nil
}

// DescribeReceiptRule returns a named rule from a rule set.
func (b *InMemoryBackend) DescribeReceiptRule(ruleSetName, ruleName string) (ReceiptRule, error) {
	if strings.TrimSpace(ruleSetName) == "" {
		return ReceiptRule{}, fmt.Errorf("%w: RuleSetName is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(ruleName) == "" {
		return ReceiptRule{}, fmt.Errorf("%w: RuleName is required", ErrInvalidParameter)
	}

	b.mu.RLock("DescribeReceiptRule")
	defer b.mu.RUnlock()

	rs, exists := b.receiptRuleSets.Get(ruleSetName)
	if !exists {
		return ReceiptRule{}, fmt.Errorf("%w: %s", ErrReceiptRuleSetNotFound, ruleSetName)
	}

	idx := findRuleIndex(rs.Rules, ruleName)
	if idx < 0 {
		return ReceiptRule{}, fmt.Errorf("%w: %s", ErrReceiptRuleNotFound, ruleName)
	}

	r := rs.Rules[idx]
	recipients := make([]string, len(r.Recipients))
	copy(recipients, r.Recipients)
	r.Recipients = recipients

	return r, nil
}

// UpdateReceiptRule replaces an existing rule in a rule set.
func (b *InMemoryBackend) UpdateReceiptRule(ruleSetName string, rule ReceiptRule) error {
	if strings.TrimSpace(ruleSetName) == "" {
		return fmt.Errorf("%w: RuleSetName is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("%w: Rule.Name is required", ErrInvalidParameter)
	}

	if rule.TLSPolicy != "" && rule.TLSPolicy != TLSPolicyOptional && rule.TLSPolicy != TLSPolicyRequire {
		return fmt.Errorf("%w: TlsPolicy must be Optional or Require", ErrInvalidParameter)
	}

	b.mu.Lock("UpdateReceiptRule")
	defer b.mu.Unlock()

	rs, exists := b.receiptRuleSets.Get(ruleSetName)
	if !exists {
		return fmt.Errorf("%w: %s", ErrReceiptRuleSetNotFound, ruleSetName)
	}

	idx := findRuleIndex(rs.Rules, rule.Name)
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrReceiptRuleNotFound, rule.Name)
	}

	rs.Rules[idx] = rule

	return nil
}

// ReorderReceiptRuleSet reorders the rules in a rule set according to the given ordered name list.
func (b *InMemoryBackend) ReorderReceiptRuleSet(ruleSetName string, ruleNames []string) error {
	if strings.TrimSpace(ruleSetName) == "" {
		return fmt.Errorf("%w: RuleSetName is required", ErrInvalidParameter)
	}

	b.mu.Lock("ReorderReceiptRuleSet")
	defer b.mu.Unlock()

	rs, exists := b.receiptRuleSets.Get(ruleSetName)
	if !exists {
		return fmt.Errorf("%w: %s", ErrReceiptRuleSetNotFound, ruleSetName)
	}

	if len(ruleNames) != len(rs.Rules) {
		return fmt.Errorf(
			"%w: ruleNames length (%d) must match rule set size (%d)",
			ErrInvalidParameter,
			len(ruleNames),
			len(rs.Rules),
		)
	}

	index := make(map[string]ReceiptRule, len(rs.Rules))
	for _, r := range rs.Rules {
		index[r.Name] = r
	}

	reordered := make([]ReceiptRule, 0, len(ruleNames))

	for _, name := range ruleNames {
		r, ok := index[name]
		if !ok {
			return fmt.Errorf("%w: rule %s not found", ErrReceiptRuleNotFound, name)
		}

		reordered = append(reordered, r)
	}

	rs.Rules = reordered

	return nil
}

// SetReceiptRulePosition moves a rule within its rule set. after="" moves the
// rule to the front; otherwise the rule is placed immediately after the named
// rule (SetReceiptRulePositionInput.After -- there is no numeric position on
// the real wire, see api_op_SetReceiptRulePosition.go).
func (b *InMemoryBackend) SetReceiptRulePosition(ruleSetName, ruleName, after string) error {
	if strings.TrimSpace(ruleSetName) == "" {
		return fmt.Errorf("%w: RuleSetName is required", ErrInvalidParameter)
	}

	if strings.TrimSpace(ruleName) == "" {
		return fmt.Errorf("%w: RuleName is required", ErrInvalidParameter)
	}

	if after == ruleName {
		return fmt.Errorf("%w: After cannot reference the rule being moved", ErrInvalidParameter)
	}

	b.mu.Lock("SetReceiptRulePosition")
	defer b.mu.Unlock()

	rs, exists := b.receiptRuleSets.Get(ruleSetName)
	if !exists {
		return fmt.Errorf("%w: %s", ErrReceiptRuleSetNotFound, ruleSetName)
	}

	idx := findRuleIndex(rs.Rules, ruleName)
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrReceiptRuleNotFound, ruleName)
	}

	rule := rs.Rules[idx]
	withoutRule := make([]ReceiptRule, 0, len(rs.Rules)-1)
	withoutRule = append(withoutRule, rs.Rules[:idx]...)
	withoutRule = append(withoutRule, rs.Rules[idx+1:]...)

	if after == "" {
		rs.Rules = append([]ReceiptRule{rule}, withoutRule...)

		return nil
	}

	afterIdx := findRuleIndex(withoutRule, after)
	if afterIdx < 0 {
		return fmt.Errorf("%w: after rule %s not found", ErrReceiptRuleNotFound, after)
	}

	newRules := make([]ReceiptRule, 0, len(withoutRule)+1)
	newRules = append(newRules, withoutRule[:afterIdx+1]...)
	newRules = append(newRules, rule)
	newRules = append(newRules, withoutRule[afterIdx+1:]...)
	rs.Rules = newRules

	return nil
}
