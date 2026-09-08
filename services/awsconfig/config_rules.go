package awsconfig

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// PutConfigRule creates or updates a config rule with full metadata.
func (b *InMemoryBackend) PutConfigRule(input *ConfigRule) error {
	if input == nil || input.ConfigRuleName == "" {
		return fmt.Errorf("%w: ConfigRuleName is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("PutConfigRule")
	defer b.mu.Unlock()

	b.putConfigRuleLocked(input)

	return nil
}

// putConfigRuleLocked creates or updates a config rule with full metadata.
// Callers must already hold the write lock -- factored out of PutConfigRule so
// PutConformancePack can register the config rules a conformance pack template
// deploys within its own single lock acquisition, matching real AWS Config
// where a conformance pack literally creates managed config rules on the
// account.
func (b *InMemoryBackend) putConfigRuleLocked(input *ConfigRule) {
	existing, ok := b.configRules.Get(input.ConfigRuleName)
	if ok {
		// Preserve ARN and ID on update.
		input.ConfigRuleArn = existing.ConfigRuleArn
		input.ConfigRuleID = existing.ConfigRuleID
	} else {
		b.ruleCounter++
		input.ConfigRuleArn = fmt.Sprintf(
			"arn:aws:config:%s:%s:config-rule/config-rule-%08d",
			b.region, b.accountID, b.ruleCounter,
		)
		input.ConfigRuleID = fmt.Sprintf("config-rule-%08d", b.ruleCounter)
	}

	if input.ConfigRuleState == "" {
		input.ConfigRuleState = "ACTIVE"
	}

	cp := *input
	// Deep-copy Source to avoid shared pointer.
	if input.Source != nil {
		srcCopy := *input.Source
		cp.Source = &srcCopy
	}

	b.configRules.Put(&cp)
}

// DescribeConfigRules returns config rules optionally filtered by name list, sorted
// by name. An unknown name in a non-empty filter list errors NoSuchConfigRuleException,
// matching real AWS Config (verified against aws-sdk-go-v2/service/configservice's
// DescribeConfigRules deserializer, which declares NoSuchConfigRuleException).
func (b *InMemoryBackend) DescribeConfigRules(names []string) ([]ConfigRule, error) {
	b.mu.RLock("DescribeConfigRules")
	defer b.mu.RUnlock()

	out := make([]ConfigRule, 0, b.configRules.Len())

	if len(names) == 0 {
		for _, r := range b.configRules.All() {
			out = append(out, *r)
		}
	} else {
		for _, n := range names {
			r, ok := b.configRules.Get(n)
			if !ok {
				return nil, fmt.Errorf("%w: %s", ErrNoSuchConfigRule, n)
			}

			out = append(out, *r)
		}
	}

	slices.SortFunc(out, func(a, b ConfigRule) int {
		if a.ConfigRuleName < b.ConfigRuleName {
			return -1
		}

		if a.ConfigRuleName > b.ConfigRuleName {
			return 1
		}

		return 0
	})

	return out, nil
}

// DeleteConfigRule deletes a config rule by name.
func (b *InMemoryBackend) DeleteConfigRule(name string) error {
	if name == "" {
		// DeleteConfigRule declares only NoSuchConfigRuleException/ResourceInUseException --
		// no validation-shaped code fits an empty name (configservice@v1.68.4 deserializers.go).
		return fmt.Errorf("%w: ConfigRuleName is required", ErrValidation)
	}

	b.mu.Lock("DeleteConfigRule")
	defer b.mu.Unlock()

	if !b.configRules.Has(name) {
		return fmt.Errorf("%w: %s", ErrNoSuchConfigRule, name)
	}

	b.configRules.Delete(name)
	b.clearRuleEvaluationsLocked(name)
	delete(b.remediationExceptions, name)

	return nil
}

// clearRuleEvaluationsLocked removes every stored evaluation (rollup and
// per-resource) for ruleName. The caller must hold the write lock.
//
// ruleResourceEvals has no bulk "delete everything under this rule" operation
// (unlike the old map[string]map[string]*T's single outer-map delete);
// snapshot the rule's entries via slices.Clone first since Table.Delete
// mutates the very index slice ruleResourceEvalsByRule.Get returns.
func (b *InMemoryBackend) clearRuleEvaluationsLocked(ruleName string) {
	delete(b.ruleEvaluations, ruleName)

	for _, e := range slices.Clone(b.ruleResourceEvalsByRule.Get(ruleName)) {
		b.ruleResourceEvals.Delete(storedEvaluationKeyFn(e))
	}
}

// DeleteEvaluationResults clears the rollup and per-resource evaluation results
// recorded for a config rule (so a subsequent StartConfigRulesEvaluation starts
// from a clean slate), matching real AWS Config which errors
// NoSuchConfigRuleException for an unknown rule (verified against
// aws-sdk-go-v2/service/configservice's DeleteEvaluationResults deserializer).
func (b *InMemoryBackend) DeleteEvaluationResults(ruleName string) error {
	if ruleName == "" {
		// Same as DeleteConfigRule: declared set is NoSuchConfigRuleException/
		// ResourceInUseException only, no fitting validation code.
		return fmt.Errorf("%w: ConfigRuleName is required", ErrValidation)
	}

	b.mu.Lock("DeleteEvaluationResults")
	defer b.mu.Unlock()

	if !b.configRules.Has(ruleName) {
		return fmt.Errorf("%w: %s", ErrNoSuchConfigRule, ruleName)
	}

	b.clearRuleEvaluationsLocked(ruleName)

	return nil
}

// GetConfigRuleComplianceType returns the rolled-up compliance type for a config
// rule after evaluation, or empty string if no evaluation has run for that rule yet.
func (b *InMemoryBackend) GetConfigRuleComplianceType(ruleName string) string {
	b.mu.RLock("GetConfigRuleComplianceType")
	defer b.mu.RUnlock()

	return b.ruleEvaluations[ruleName]
}

// DescribeConfigRuleEvaluationStatus returns evaluation statuses for config rules.
// If names is empty, all rules are returned.
func (b *InMemoryBackend) DescribeConfigRuleEvaluationStatus(names []string) []ConfigRuleEvaluationStatus {
	b.mu.RLock("DescribeConfigRuleEvaluationStatus")
	defer b.mu.RUnlock()

	if len(names) == 0 {
		out := make([]ConfigRuleEvaluationStatus, 0, len(b.ruleEvaluations))
		for name := range b.ruleEvaluations {
			out = append(out, ConfigRuleEvaluationStatus{ConfigRuleName: name})
		}

		return out
	}

	out := make([]ConfigRuleEvaluationStatus, 0, len(names))

	for _, name := range names {
		if _, ok := b.ruleEvaluations[name]; ok {
			out = append(out, ConfigRuleEvaluationStatus{ConfigRuleName: name})
		}
	}

	return out
}

// DescribeComplianceByConfigRule returns compliance info for the given rule names.
// If names is empty, all rules are returned.
func (b *InMemoryBackend) DescribeComplianceByConfigRule(names []string) []ComplianceByConfigRule {
	b.mu.RLock("DescribeComplianceByConfigRule")
	defer b.mu.RUnlock()

	if len(names) == 0 {
		out := make([]ComplianceByConfigRule, 0, len(b.ruleEvaluations))
		for name, ct := range b.ruleEvaluations {
			out = append(out, ComplianceByConfigRule{
				ConfigRuleName: name,
				Compliance:     ComplianceResult{ComplianceType: ct},
			})
		}

		return out
	}

	out := make([]ComplianceByConfigRule, 0, len(names))

	for _, name := range names {
		ct := b.ruleEvaluations[name]
		if ct == "" {
			ct = complianceNotApplicable
		}

		out = append(out, ComplianceByConfigRule{
			ConfigRuleName: name,
			Compliance:     ComplianceResult{ComplianceType: ct},
		})
	}

	return out
}

// GetComplianceSummaryByConfigRule returns a compliance summary aggregated
// from the recorded rule evaluations. Real GetComplianceSummaryByConfigRuleOutput
// carries a single ComplianceSummary object, not a list (confirmed at
// aws-sdk-go-v2/service/configservice's api_op_GetComplianceSummaryByConfigRule.go);
// counts are derived from the stored per-rule compliance types populated via
// PutEvaluation(s)/PutExternalEvaluation. When no evaluations have been
// recorded both counts are zero.
func (b *InMemoryBackend) GetComplianceSummaryByConfigRule() ComplianceSummary {
	b.mu.RLock("GetComplianceSummaryByConfigRule")
	defer b.mu.RUnlock()

	var compliant, nonCompliant int32

	for _, ct := range b.ruleEvaluations {
		switch ct {
		case "COMPLIANT":
			compliant++
		case "NON_COMPLIANT":
			nonCompliant++
		}
	}

	return ComplianceSummary{
		CompliantResourceCount:    ResourceCount{CappedCount: compliant},
		NonCompliantResourceCount: ResourceCount{CappedCount: nonCompliant},
	}
}

// PutEvaluations stores evaluation results from an AWS Lambda function for a
// config rule. Each result is retained per-(rule, resource) so the compliance
// detail APIs can return real per-resource outcomes.
func (b *InMemoryBackend) PutEvaluations(results []EvaluationResult) error {
	b.mu.Lock("PutEvaluations")
	defer b.mu.Unlock()

	now := float64(time.Now().Unix())

	for _, r := range results {
		b.recordEvaluationLocked(r.ConfigRuleName, r.ResourceType, r.ResourceID, r.ComplianceType, r.Annotation, now)
	}

	return nil
}

// PutExternalEvaluation stores a single external evaluation result per-resource.
func (b *InMemoryBackend) PutExternalEvaluation(result EvaluationResult) error {
	b.mu.Lock("PutExternalEvaluation")
	defer b.mu.Unlock()

	b.recordEvaluationLocked(
		result.ConfigRuleName,
		result.ResourceType,
		result.ResourceID,
		result.ComplianceType,
		result.Annotation,
		float64(time.Now().Unix()),
	)

	return nil
}

// GetCustomRulePolicy returns the policy text for the given custom rule.
func (b *InMemoryBackend) GetCustomRulePolicy(ruleName string) string {
	b.mu.RLock("GetCustomRulePolicy")
	defer b.mu.RUnlock()

	return b.customRulePolicies[ruleName]
}

// GetAggregateComplianceDetailsByConfigRule returns per-resource evaluation
// results for ruleName as seen through aggregatorName, echoing the requested
// accountID/awsRegion into each result. This emulator has no real
// multi-account data source, so (mirroring DescribeAggregateComplianceByConfigRules,
// already-established for this same reason) it reuses the local account's own
// per-rule evaluation state rather than returning an empty stub; only the
// aggregator's existence is genuinely validated (NoSuchConfigurationAggregatorException).
func (b *InMemoryBackend) GetAggregateComplianceDetailsByConfigRule(
	aggregatorName, ruleName, accountID, awsRegion string,
	complianceTypes []string,
) ([]AggregateEvaluationResult, error) {
	b.mu.RLock("GetAggregateComplianceDetailsByConfigRule")
	defer b.mu.RUnlock()

	if err := b.requireAggregatorLocked(aggregatorName); err != nil {
		return nil, err
	}

	details := buildDetailedResults(ruleName, b.ruleResourceEvalsByRule.Get(ruleName), complianceTypes)
	out := make([]AggregateEvaluationResult, 0, len(details))

	for _, d := range details {
		out = append(out, AggregateEvaluationResult{
			EvaluationResultIdentifier: d.EvaluationResultIdentifier,
			ComplianceType:             d.ComplianceType,
			AccountID:                  accountID,
			AwsRegion:                  awsRegion,
			Annotation:                 d.Annotation,
			ResultRecordedTime:         d.ResultRecordedTime,
			ConfigRuleInvokedTime:      d.ConfigRuleInvokedTime,
		})
	}

	return out, nil
}

// GetAggregateConfigRuleComplianceSummary returns compliant/non-compliant rule
// counts grouped by account ID or AWS region (groupByKey; ACCOUNT_ID when
// empty). Since this emulator only ever has one local account/region as its
// aggregated source, the result is a single group -- mirroring
// GetComplianceSummaryByConfigRule's rollup logic -- once the aggregator's
// existence is validated (NoSuchConfigurationAggregatorException).
func (b *InMemoryBackend) GetAggregateConfigRuleComplianceSummary(
	aggregatorName, groupByKey string,
) ([]AggregateComplianceCount, error) {
	b.mu.RLock("GetAggregateConfigRuleComplianceSummary")
	defer b.mu.RUnlock()

	if err := b.requireAggregatorLocked(aggregatorName); err != nil {
		return nil, err
	}

	if len(b.ruleEvaluations) == 0 {
		return []AggregateComplianceCount{}, nil
	}

	groupName := b.accountID
	if groupByKey == "AWS_REGION" {
		groupName = b.region
	}

	var compliant, nonCompliant int32

	for _, ct := range b.ruleEvaluations {
		switch ct {
		case complianceCompliant:
			compliant++
		case complianceNonCompliant:
			nonCompliant++
		}
	}

	return []AggregateComplianceCount{{
		GroupName: groupName,
		ComplianceSummary: ComplianceSummary{
			CompliantResourceCount:    ResourceCount{CappedCount: compliant},
			NonCompliantResourceCount: ResourceCount{CappedCount: nonCompliant},
		},
	}}, nil
}

// DescribeAggregateComplianceByConfigRules returns compliance by rule using ruleEvaluations.
func (b *InMemoryBackend) DescribeAggregateComplianceByConfigRules() []any {
	b.mu.RLock("DescribeAggregateComplianceByConfigRules")
	defer b.mu.RUnlock()

	out := make([]any, 0, len(b.ruleEvaluations))

	for name, ct := range b.ruleEvaluations {
		out = append(out, ComplianceByConfigRule{
			ConfigRuleName: name,
			Compliance:     ComplianceResult{ComplianceType: ct},
		})
	}

	return out
}

// GetComplianceSummaryByResourceType returns compliant/non-compliant resource
// counts grouped by resource type, derived from the same per-(rule, resource)
// evaluation state (b.ruleResourceEvals) that rolls up into b.ruleEvaluations
// for DescribeAggregateComplianceByConfigRules. A resource counts as
// NON_COMPLIANT if any rule evaluated it as such, else COMPLIANT. When
// resourceTypes is non-empty, only those types are included.
func (b *InMemoryBackend) GetComplianceSummaryByResourceType(
	resourceTypes []string,
) []ComplianceSummaryByResourceType {
	b.mu.RLock("GetComplianceSummaryByResourceType")
	defer b.mu.RUnlock()

	nonCompliant := resourceComplianceByType(b.ruleResourceEvals.All(), resourceTypes)

	resourceTypesSeen := make([]string, 0, len(nonCompliant))
	for rt := range nonCompliant {
		resourceTypesSeen = append(resourceTypesSeen, rt)
	}

	sort.Strings(resourceTypesSeen)

	out := make([]ComplianceSummaryByResourceType, 0, len(resourceTypesSeen))
	for _, rt := range resourceTypesSeen {
		out = append(out, summarizeResourceType(rt, nonCompliant[rt]))
	}

	return out
}

// resourceComplianceByType walks per-(rule, resource) evaluations and, for
// every resource matching the optional resourceTypes filter, records whether
// any rule found it NON_COMPLIANT (true) or only COMPLIANT/other (false).
// evals is the flat contents of b.ruleResourceEvals (formerly a nested
// map[string]map[string]*StoredEvaluation keyed by rule then resource; the
// flattened store.Table has no rule-name grouping at this call site's level,
// but nothing here ever used the rule name, only ResourceType/ResourceID/
// ComplianceType off each StoredEvaluation, so a flat slice is equivalent).
func resourceComplianceByType(
	evals []*StoredEvaluation,
	resourceTypes []string,
) map[string]map[string]bool {
	filter := make(map[string]struct{}, len(resourceTypes))
	for _, rt := range resourceTypes {
		filter[rt] = struct{}{}
	}

	nonCompliant := make(map[string]map[string]bool)

	for _, e := range evals {
		if len(filter) > 0 {
			if _, ok := filter[e.ResourceType]; !ok {
				continue
			}
		}

		if nonCompliant[e.ResourceType] == nil {
			nonCompliant[e.ResourceType] = make(map[string]bool)
		}

		nonCompliant[e.ResourceType][e.ResourceID] =
			nonCompliant[e.ResourceType][e.ResourceID] || e.ComplianceType == complianceNonCompliant
	}

	return nonCompliant
}

// summarizeResourceType counts compliant/non-compliant resources for one
// resource type into the wire-shaped summary.
func summarizeResourceType(resourceType string, resources map[string]bool) ComplianceSummaryByResourceType {
	var compliantCount, nonCompliantCount int32

	for _, isNonCompliant := range resources {
		if isNonCompliant {
			nonCompliantCount++
		} else {
			compliantCount++
		}
	}

	return ComplianceSummaryByResourceType{
		ResourceType: resourceType,
		ComplianceSummary: ComplianceSummaryDetail{
			CompliantResourceCount:    ResourceCount{CappedCount: compliantCount},
			NonCompliantResourceCount: ResourceCount{CappedCount: nonCompliantCount},
		},
	}
}

// DescribeComplianceByResource returns compliance rollups for discovered
// resources, derived from the same per-(rule, resource) evaluation state
// (b.ruleResourceEvals) that DescribeComplianceByConfigRule/
// GetComplianceSummaryByResourceType roll up from -- mirroring their approach
// instead of the previous intentional empty-list stub. A resource is
// NON_COMPLIANT if any rule evaluated it as such, COMPLIANT if every rule that
// evaluated it found it compliant, else INSUFFICIENT_DATA. resourceType/
// resourceID (both optional; resourceID is only meaningful alongside a
// resourceType) and complianceTypes narrow the result set, matching real AWS
// Config's DescribeComplianceByResource filters (verified against
// aws-sdk-go-v2/service/configservice's DescribeComplianceByResourceInput).
func (b *InMemoryBackend) DescribeComplianceByResource(
	resourceType, resourceID string,
	complianceTypes []string,
) []ComplianceByResource {
	b.mu.RLock("DescribeComplianceByResource")
	defer b.mu.RUnlock()

	return rollupComplianceByResource(b.ruleResourceEvals.All(), resourceType, resourceID, complianceTypes)
}

// rollupComplianceByResource groups per-(rule, resource) evaluations by
// resource, rolls each group up to a single compliance result, and applies the
// optional resourceType/resourceID/complianceTypes filters.
func rollupComplianceByResource(
	evals []*StoredEvaluation,
	resourceType, resourceID string,
	complianceTypes []string,
) []ComplianceByResource {
	type resourceKey struct{ resourceType, resourceID string }

	grouped := make(map[resourceKey][]*StoredEvaluation)

	for _, e := range evals {
		if resourceType != "" && e.ResourceType != resourceType {
			continue
		}

		if resourceID != "" && e.ResourceID != resourceID {
			continue
		}

		key := resourceKey{e.ResourceType, e.ResourceID}
		grouped[key] = append(grouped[key], e)
	}

	out := make([]ComplianceByResource, 0, len(grouped))

	for key, group := range grouped {
		item := complianceByResourceItem(key.resourceType, key.resourceID, group)
		if len(complianceTypes) > 0 && !slices.Contains(complianceTypes, item.Compliance.ComplianceType) {
			continue
		}

		out = append(out, item)
	}

	slices.SortFunc(out, func(a, c ComplianceByResource) int {
		if a.ResourceType != c.ResourceType {
			return strings.Compare(a.ResourceType, c.ResourceType)
		}

		return strings.Compare(a.ResourceID, c.ResourceID)
	})

	return out
}

// complianceByResourceItem rolls a single resource's per-rule evaluations up
// into one ComplianceByResource entry. ComplianceContributorCount reflects the
// number of evaluations that drove the rollup outcome: NON_COMPLIANT
// evaluations when the resource is NON_COMPLIANT, COMPLIANT evaluations when it
// is COMPLIANT, zero otherwise.
func complianceByResourceItem(resourceType, resourceID string, group []*StoredEvaluation) ComplianceByResource {
	rollup := rollupCompliance(group)

	var contributing int32

	for _, e := range group {
		if e.ComplianceType == rollup {
			contributing++
		}
	}

	return ComplianceByResource{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Compliance: ComplianceResult{
			ComplianceType:             rollup,
			ComplianceContributorCount: &ResourceCount{CappedCount: contributing},
		},
	}
}
