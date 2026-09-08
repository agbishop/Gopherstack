package ce

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) buildCostCategoryARN(name string) string {
	return arn.Build("ce", "", b.accountID, fmt.Sprintf("costcategory/%s", name))
}

func effectiveStart() string {
	now := time.Now().UTC()

	return fmt.Sprintf("%d-%02d-01T00:00:00Z", now.Year(), now.Month())
}

// CreateCostCategoryDefinition creates a new cost category and returns it.
// requestedEffectiveStart, when non-empty, overrides the default "first day
// of the current month" real AWS also defaults to when the field is
// omitted (api_op_CreateCostCategoryDefinition.go).
func (b *InMemoryBackend) CreateCostCategoryDefinition(
	name, ruleVersion, defaultValue string,
	rules []CostCategoryRule,
	resourceTags map[string]string,
	splitChargeRules []SplitChargeRule,
	requestedEffectiveStart string,
) (*CostCategory, error) {
	b.mu.Lock("CreateCostCategoryDefinition")
	defer b.mu.Unlock()

	catARN := b.buildCostCategoryARN(name)
	if b.costCategories.Has(catARN) {
		return nil, ErrAlreadyExists
	}

	tagsCopy := make(map[string]string, len(resourceTags))
	maps.Copy(tagsCopy, resourceTags)

	rulesCopy := make([]CostCategoryRule, len(rules))
	copy(rulesCopy, rules)

	start := requestedEffectiveStart
	if start == "" {
		start = effectiveStart()
	}

	cat := &CostCategory{
		ARN:              catARN,
		Name:             name,
		RuleVersion:      ruleVersion,
		DefaultValue:     defaultValue,
		Rules:            rulesCopy,
		SplitChargeRules: copySplitChargeRules(splitChargeRules),
		EffectiveStart:   start,
		CreationDate:     time.Now().UTC(),
		Tags:             tagsCopy,
	}
	b.costCategories.Put(cat)

	out := *cat
	out.Rules = make([]CostCategoryRule, len(cat.Rules))
	copy(out.Rules, cat.Rules)
	out.SplitChargeRules = copySplitChargeRules(cat.SplitChargeRules)

	return &out, nil
}

// copySplitChargeRules deep-copies rules (including each rule's own Targets
// slice) so the caller can never alias backend-owned state.
func copySplitChargeRules(rules []SplitChargeRule) []SplitChargeRule {
	out := make([]SplitChargeRule, len(rules))

	for i, r := range rules {
		rc := r
		if r.Targets != nil {
			rc.Targets = make([]string, len(r.Targets))
			copy(rc.Targets, r.Targets)
		}

		out[i] = rc
	}

	return out
}

// DeleteCostCategoryDefinition removes a cost category by ARN.
func (b *InMemoryBackend) DeleteCostCategoryDefinition(catARN string) (*CostCategory, error) {
	b.mu.Lock("DeleteCostCategoryDefinition")
	defer b.mu.Unlock()

	cat, exists := b.costCategories.Get(catARN)
	if !exists {
		return nil, ErrNotFound
	}

	b.costCategories.Delete(catARN)

	out := *cat

	return &out, nil
}

// DescribeCostCategoryDefinition returns a cost category by ARN.
func (b *InMemoryBackend) DescribeCostCategoryDefinition(catARN string) (*CostCategory, error) {
	b.mu.RLock("DescribeCostCategoryDefinition")
	defer b.mu.RUnlock()

	cat, exists := b.costCategories.Get(catARN)
	if !exists {
		return nil, ErrNotFound
	}

	out := *cat

	return &out, nil
}

// ListCostCategoryDefinitions returns cost categories sorted by name with
// opaque pagination, narrowed to categories whose EffectiveStart is on or
// before effectiveOn -- see DescribeCostCategoryDefinition's EffectiveOn
// handling for why this backend can only honor "existed by this date", not
// real AWS's full historical-version lookup. Per
// ListCostCategoryDefinitionsInput.EffectiveOn's doc comment, an empty
// effectiveOn defaults to the current date rather than disabling the filter.
func (b *InMemoryBackend) ListCostCategoryDefinitions(
	maxResults int, nextPageToken, effectiveOn string,
) ([]*CostCategory, string) {
	b.mu.RLock("ListCostCategoryDefinitions")
	defer b.mu.RUnlock()

	on := effectiveOn
	if on == "" {
		on = time.Now().UTC().Format(time.RFC3339)
	}

	all := b.costCategories.All()
	result := make([]*CostCategory, 0, len(all))

	for _, cat := range all {
		if on < cat.EffectiveStart {
			continue
		}

		out := *cat
		result = append(result, &out)
	}

	return paginateList(result, maxResults, nextPageToken, func(c *CostCategory) string {
		return c.Name
	})
}

// UpdateCostCategoryDefinition updates an existing cost category.
// requestedEffectiveStart, when non-empty, overrides the default "first day of the
// current month" -- same optional-field default as CreateCostCategoryDefinition
// (api_op_UpdateCostCategoryDefinition.go's EffectiveStart doc is identical to
// Create's: "If the date isn't provided, it's the first day of the current month").
func (b *InMemoryBackend) UpdateCostCategoryDefinition(
	catARN, ruleVersion, defaultValue string,
	rules []CostCategoryRule,
	splitChargeRules []SplitChargeRule,
	requestedEffectiveStart string,
) (*CostCategory, error) {
	b.mu.Lock("UpdateCostCategoryDefinition")
	defer b.mu.Unlock()

	cat, exists := b.costCategories.Get(catARN)
	if !exists {
		return nil, ErrNotFound
	}

	cat.RuleVersion = ruleVersion
	cat.DefaultValue = defaultValue
	// Deep-copy both slices so the caller cannot alias backend-owned state.
	rulesCopy := make([]CostCategoryRule, len(rules))
	copy(rulesCopy, rules)
	cat.Rules = rulesCopy

	cat.SplitChargeRules = copySplitChargeRules(splitChargeRules)

	start := requestedEffectiveStart
	if start == "" {
		start = effectiveStart()
	}

	cat.EffectiveStart = start

	out := *cat
	out.Rules = make([]CostCategoryRule, len(cat.Rules))
	copy(out.Rules, cat.Rules)
	out.SplitChargeRules = copySplitChargeRules(cat.SplitChargeRules)

	return &out, nil
}

// GetCostCategories returns the distinct cost category values stored in the
// backend, optionally filtered by cost category name. Values are sorted alphabetically.
func (b *InMemoryBackend) GetCostCategories(costCategoryName string) []string {
	b.mu.RLock("GetCostCategories")
	defer b.mu.RUnlock()

	seen := make(map[string]struct{})
	var values []string

	for _, cat := range b.costCategories.All() {
		if costCategoryName != "" && cat.Name != costCategoryName {
			continue
		}

		for _, rule := range cat.Rules {
			if _, exists := seen[rule.Value]; !exists && rule.Value != "" {
				seen[rule.Value] = struct{}{}
				values = append(values, rule.Value)
			}
		}
	}

	sort.Strings(values)

	return values
}

// GetCostCategoryNames returns the distinct cost category names stored in the
// backend, sorted alphabetically. Real GetCostCategories emits this list
// instead of CostCategoryValues when the request omits CostCategoryName (see
// api_op_GetCostCategories.go: "If the CostCategoryName key isn't specified
// in the request, the CostCategoryValues fields aren't returned").
func (b *InMemoryBackend) GetCostCategoryNames() []string {
	b.mu.RLock("GetCostCategoryNames")
	defer b.mu.RUnlock()

	names := make([]string, 0, b.costCategories.Len())
	for _, cat := range b.costCategories.All() {
		names = append(names, cat.Name)
	}

	sort.Strings(names)

	return names
}
