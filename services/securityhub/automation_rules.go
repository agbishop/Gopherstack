package securityhub

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// errCodeAutomationRuleNotFound is an HTTP status code: UnprocessedAutomationRule.ErrorCode
// is *int32 (types/types.go:19904), like the identically-shaped
// cloudfront CustomErrorResponse.ErrorCode ("The HTTP status code").
const errCodeAutomationRuleNotFound = int32(http.StatusNotFound)

func (b *InMemoryBackend) automationRuleARN(seq int) string {
	return arn.Build("securityhub", b.region, b.accountID, fmt.Sprintf("automation-rule/%d", seq))
}

// clone deep-copies r's map/slice fields. BatchUpdateAutomationRules mutates
// a live *AutomationRule's fields in place under lock; BatchGetAutomationRules
// used to hand that same pointer straight back to callers with no copy at
// all, racing a caller reading it after RUnlock against a concurrent update.
func (r *AutomationRule) clone() *AutomationRule {
	cp := *r
	cp.Criteria = maps.Clone(r.Criteria)
	cp.Actions = slices.Clone(r.Actions)

	return &cp
}

// clone deep-copies r's map/slice fields; see AutomationRule.clone.
func (r *AutomationRuleV2) clone() *AutomationRuleV2 {
	cp := *r
	cp.Criteria = maps.Clone(r.Criteria)
	cp.Actions = slices.Clone(r.Actions)

	return &cp
}

// applyAutomationRules evaluates every ENABLED automation rule (ascending
// RuleOrder, ties broken by RuleArn for determinism) against finding and
// applies each match's FINDING_FIELDS_UPDATE action in place, stopping at
// the first terminal match -- gopherstack-1qf: automation rules were pure
// CRUD, never evaluated against imported findings. AWS documents that
// BatchImportFindings itself cannot set Note/UserDefinedFields/
// VerificationState/Workflow "since they're managed by Security Hub
// customers/automation rules, not finding providers" (findings.go's
// findingCustomerManagedFields) -- automation rules are the only mechanism
// that manages them, so this must run for that documented architecture to
// have any real effect.
//
// Criteria is matched via matchesFindingFilters against the same
// field-name-mapped subset it already supports (AwsAccountId/GeneratorId/
// Title/Description/RecordState/Type/ResourceType/ResourceId/Id/ProductArn/
// SeverityLabel/WorkflowStatus/ComplianceStatus) -- AutomationRulesFindingFilters
// (securityhub@v1.75.4 types/types.go:575) has additional NumberFilter/
// DateFilter/MapFilter members (Confidence, CreatedAt, NoteText,
// ResourceTags, ...) with no evaluator in this file; deliberately left
// unevaluated per the no-fabrication rule rather than guessed at.
//
// Caller must already hold b.mu (Lock) -- this reads b.automationRules
// under the same coarse lock ImportFindings already acquires.
func (b *InMemoryBackend) applyAutomationRules(finding map[string]any) {
	rules := b.automationRules.Snapshot()

	sort.Slice(rules, func(i, j int) bool {
		if rules[i].RuleOrder != rules[j].RuleOrder {
			return rules[i].RuleOrder < rules[j].RuleOrder
		}

		return rules[i].RuleArn < rules[j].RuleArn
	})

	for _, rule := range rules {
		if rule.RuleStatus != statusEnabled {
			continue
		}

		if !matchesFindingFilters(finding, rule.Criteria) {
			continue
		}

		for _, action := range rule.Actions {
			if t, _ := action["Type"].(string); t != "FINDING_FIELDS_UPDATE" {
				continue
			}

			if update, ok := action["FindingFieldsUpdate"].(map[string]any); ok {
				maps.Copy(finding, update)
			}
		}

		if rule.IsTerminal {
			break
		}
	}
}

func (b *InMemoryBackend) CreateAutomationRule(rule map[string]any) (string, string) {
	b.mu.Lock("CreateAutomationRule")
	defer b.mu.Unlock()

	b.automationRuleSeq++
	ruleArn := b.automationRuleARN(b.automationRuleSeq)
	now := time.Now().UTC().Format(time.RFC3339)

	criteria, _ := rule["Criteria"].(map[string]any)
	actions, _ := rule["Actions"].([]any)

	var actionMaps []map[string]any

	for _, a := range actions {
		if m, ok := a.(map[string]any); ok {
			actionMaps = append(actionMaps, m)
		}
	}

	ruleName, _ := rule["RuleName"].(string)
	desc, _ := rule["Description"].(string)
	ruleStatus, _ := rule["RuleStatus"].(string)
	isTerminal, _ := rule["IsTerminal"].(bool)

	ruleOrder := int32(0)
	if ro, ok := rule["RuleOrder"].(float64); ok {
		ruleOrder = int32(ro)
	}

	if ruleStatus == "" {
		ruleStatus = statusEnabled
	}

	b.automationRules.Put(&AutomationRule{
		RuleArn:     ruleArn,
		RuleStatus:  ruleStatus,
		RuleOrder:   ruleOrder,
		RuleName:    ruleName,
		Description: desc,
		IsTerminal:  isTerminal,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   b.accountID,
		Criteria:    criteria,
		Actions:     actionMaps,
	})

	if t, ok := rule["Tags"].(map[string]any); ok && len(t) > 0 {
		tags := make(map[string]string, len(t))
		for k, v := range t {
			tags[k], _ = v.(string)
		}

		b.tags[ruleArn] = tags
	}

	return ruleArn, now
}

func (b *InMemoryBackend) ListAutomationRules(nextToken string, maxResults int) ([]*AutomationRuleMetadata, string) {
	b.mu.RLock("ListAutomationRules")
	defer b.mu.RUnlock()

	all := b.automationRules.All()
	results := make([]*AutomationRuleMetadata, 0, len(all))

	for _, rule := range all {
		results = append(results, &AutomationRuleMetadata{
			RuleArn:     rule.RuleArn,
			RuleStatus:  rule.RuleStatus,
			RuleOrder:   rule.RuleOrder,
			RuleName:    rule.RuleName,
			Description: rule.Description,
			IsTerminal:  rule.IsTerminal,
			CreatedAt:   rule.CreatedAt,
			UpdatedAt:   rule.UpdatedAt,
			CreatedBy:   rule.CreatedBy,
		})
	}

	if maxResults <= 0 || maxResults > 100 {
		maxResults = 100
	}

	start := decodeToken(nextToken)
	if start >= len(results) {
		return []*AutomationRuleMetadata{}, ""
	}

	end := start + maxResults
	end = min(end, len(results))

	page := results[start:end]
	nextOut := ""

	if end < len(results) {
		nextOut = encodeToken(end)
	}

	return page, nextOut
}

func (b *InMemoryBackend) BatchGetAutomationRules(automationRulesArns []string) ([]*AutomationRule, []map[string]any) {
	b.mu.RLock("BatchGetAutomationRules")
	defer b.mu.RUnlock()

	var rules []*AutomationRule
	var unprocessed []map[string]any

	for _, arn := range automationRulesArns {
		rule, ok := b.automationRules.Get(arn)
		if !ok {
			unprocessed = append(unprocessed, map[string]any{
				keyRuleArn:      arn,
				keyErrorCode:    errCodeAutomationRuleNotFound,
				keyErrorMessage: msgRuleNotFound,
			})

			continue
		}

		rules = append(rules, rule.clone())
	}

	if rules == nil {
		rules = []*AutomationRule{}
	}

	if unprocessed == nil {
		unprocessed = []map[string]any{}
	}

	return rules, unprocessed
}

func (b *InMemoryBackend) BatchDeleteAutomationRules(automationRulesArns []string) ([]string, []map[string]any) {
	b.mu.Lock("BatchDeleteAutomationRules")
	defer b.mu.Unlock()

	var deleted []string
	var unprocessed []map[string]any

	for _, arn := range automationRulesArns {
		if !b.automationRules.Delete(arn) {
			unprocessed = append(unprocessed, map[string]any{
				keyRuleArn:      arn,
				keyErrorCode:    errCodeAutomationRuleNotFound,
				keyErrorMessage: msgRuleNotFound,
			})

			continue
		}

		deleted = append(deleted, arn)
	}

	if deleted == nil {
		deleted = []string{}
	}

	if unprocessed == nil {
		unprocessed = []map[string]any{}
	}

	return deleted, unprocessed
}

// applyAutomationRuleUpdate copies each optional field present in u onto
// rule, leaving fields the caller omitted untouched. Split out of
// BatchUpdateAutomationRules purely to keep that function's cognitive
// complexity down -- the seven independent optional fields are inherently
// sequential, not nested, logic.
func applyAutomationRuleUpdate(rule *AutomationRule, u map[string]any) {
	if name, hasName := u["RuleName"].(string); hasName {
		rule.RuleName = name
	}

	if desc, hasDesc := u["Description"].(string); hasDesc {
		rule.Description = desc
	}

	if status, hasStatus := u["RuleStatus"].(string); hasStatus {
		rule.RuleStatus = status
	}

	if order, hasOrder := u["RuleOrder"].(float64); hasOrder {
		rule.RuleOrder = int32(order)
	}

	if terminal, hasTerminal := u["IsTerminal"].(bool); hasTerminal {
		rule.IsTerminal = terminal
	}

	if criteria, hasCriteria := u["Criteria"].(map[string]any); hasCriteria {
		rule.Criteria = criteria
	}

	if rawActions, hasActions := u["Actions"].([]any); hasActions {
		actionMaps := make([]map[string]any, 0, len(rawActions))

		for _, a := range rawActions {
			if m, isMap := a.(map[string]any); isMap {
				actionMaps = append(actionMaps, m)
			}
		}

		rule.Actions = actionMaps
	}
}

func (b *InMemoryBackend) BatchUpdateAutomationRules(updates []map[string]any) ([]string, []map[string]any) {
	b.mu.Lock("BatchUpdateAutomationRules")
	defer b.mu.Unlock()

	var processed []string
	var unprocessed []map[string]any

	for _, u := range updates {
		arn, _ := u[keyRuleArn].(string)

		rule, exists := b.automationRules.Get(arn)
		if !exists {
			unprocessed = append(unprocessed, map[string]any{
				keyRuleArn:      arn,
				keyErrorCode:    errCodeAutomationRuleNotFound,
				keyErrorMessage: msgRuleNotFound,
			})

			continue
		}

		applyAutomationRuleUpdate(rule, u)

		rule.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		processed = append(processed, arn)
	}

	if processed == nil {
		processed = []string{}
	}

	if unprocessed == nil {
		unprocessed = []map[string]any{}
	}

	return processed, unprocessed
}

func (b *InMemoryBackend) automationRuleV2ARN(id string) string {
	return arn.Build("securityhub", b.region, b.accountID, fmt.Sprintf("automation-rule-v2/%s", id))
}

func (b *InMemoryBackend) CreateAutomationRuleV2(
	ruleName, ruleStatus, description string,
	criteria map[string]any,
	actions []map[string]any,
	ruleOrder float64,
	tags map[string]string,
) (*AutomationRuleV2, error) {
	b.mu.Lock("CreateAutomationRuleV2")
	defer b.mu.Unlock()

	b.automationRuleV2Seq++
	id := fmt.Sprintf("rule-v2-%d", b.automationRuleV2Seq)
	arn := b.automationRuleV2ARN(id)
	now := time.Now().UTC().Format(time.RFC3339)

	rule := &AutomationRuleV2{
		Identifier:  id,
		RuleArn:     arn,
		RuleName:    ruleName,
		RuleStatus:  ruleStatus,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Criteria:    criteria,
		Actions:     actions,
		RuleOrder:   ruleOrder,
	}
	b.automationRulesV2.Put(rule)

	if len(tags) > 0 {
		b.tags[arn] = tags
	}

	return rule.clone(), nil
}

func (b *InMemoryBackend) GetAutomationRuleV2(identifier string) (*AutomationRuleV2, error) {
	b.mu.RLock("GetAutomationRuleV2")
	defer b.mu.RUnlock()

	rule, ok := b.automationRulesV2.Get(identifier)
	if !ok {
		for _, r := range b.automationRulesV2.All() {
			if r.RuleArn == identifier {
				return r.clone(), nil
			}
		}

		return nil, ErrNotFound
	}

	return rule.clone(), nil
}

func (b *InMemoryBackend) ListAutomationRulesV2(nextToken string, maxResults int) ([]*AutomationRuleV2, string) {
	b.mu.RLock("ListAutomationRulesV2")
	defer b.mu.RUnlock()

	snap := b.automationRulesV2.Snapshot()
	all := make([]*AutomationRuleV2, 0, len(snap))

	for _, rule := range snap {
		all = append(all, rule.clone())
	}

	return paginateSlice(all, nextToken, maxResults, maxDefaultResults)
}

func (b *InMemoryBackend) UpdateAutomationRuleV2(
	identifier string,
	updates map[string]any,
) (*AutomationRuleV2, error) {
	b.mu.Lock("UpdateAutomationRuleV2")
	defer b.mu.Unlock()

	var target *AutomationRuleV2

	if rule, ok := b.automationRulesV2.Get(identifier); ok {
		target = rule
	} else {
		for _, r := range b.automationRulesV2.All() {
			if r.RuleArn == identifier {
				target = r

				break
			}
		}
	}

	if target == nil {
		return nil, ErrNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if v, ok := updates["RuleName"].(string); ok {
		target.RuleName = v
	}

	if v, ok := updates["RuleStatus"].(string); ok {
		target.RuleStatus = v
	}

	if v, ok := updates["Description"].(string); ok {
		target.Description = v
	}

	if v, ok := updates["Criteria"].(map[string]any); ok {
		target.Criteria = v
	}

	// Actions decodes from JSON as []any (each element map[string]any), not
	// []map[string]any -- a direct type assertion to []map[string]any always
	// fails and silently drops every Actions update.
	if raw, ok := updates["Actions"].([]any); ok {
		actionMaps := make([]map[string]any, 0, len(raw))

		for _, a := range raw {
			if m, isMap := a.(map[string]any); isMap {
				actionMaps = append(actionMaps, m)
			}
		}

		target.Actions = actionMaps
	}

	if v, ok := updates["RuleOrder"].(float64); ok {
		target.RuleOrder = v
	}

	target.UpdatedAt = now

	return target.clone(), nil
}

func (b *InMemoryBackend) DeleteAutomationRuleV2(identifier string) error {
	b.mu.Lock("DeleteAutomationRuleV2")
	defer b.mu.Unlock()

	if b.automationRulesV2.Delete(identifier) {
		return nil
	}

	for _, r := range b.automationRulesV2.All() {
		if r.RuleArn == identifier {
			b.automationRulesV2.Delete(r.Identifier)

			return nil
		}
	}

	return ErrNotFound
}
