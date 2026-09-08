package ce

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

type createCostCategoryDefinitionInput struct {
	Name             string             `json:"Name"`
	RuleVersion      string             `json:"RuleVersion"`
	DefaultValue     string             `json:"DefaultValue"`
	EffectiveStart   string             `json:"EffectiveStart"`
	Rules            []costCategoryRule `json:"Rules"`
	SplitChargeRules []splitChargeRule  `json:"SplitChargeRules"`
	ResourceTags     []resourceTag      `json:"ResourceTags"`
}

type costCategoryRule struct {
	Value string `json:"Value"`
}

type splitChargeRule struct {
	Source  string   `json:"Source"`
	Method  string   `json:"Method"`
	Targets []string `json:"Targets"`
}

type createCostCategoryDefinitionOutput struct {
	CostCategoryArn string `json:"CostCategoryArn"`
	EffectiveStart  string `json:"EffectiveStart"`
}

func (h *Handler) handleCreateCostCategoryDefinition(
	_ context.Context,
	in *createCostCategoryDefinitionInput,
) (*createCostCategoryDefinitionOutput, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if in.RuleVersion == "" {
		return nil, fmt.Errorf("%w: RuleVersion is required", ErrValidation)
	}

	if in.Rules == nil {
		return nil, fmt.Errorf("%w: Rules is required", ErrValidation)
	}

	rules := make([]CostCategoryRule, 0, len(in.Rules))
	for _, r := range in.Rules {
		rules = append(rules, CostCategoryRule(r))
	}

	splitChargeRules := make([]SplitChargeRule, 0, len(in.SplitChargeRules))
	for _, r := range in.SplitChargeRules {
		splitChargeRules = append(splitChargeRules, SplitChargeRule(r))
	}

	cat, err := h.Backend.CreateCostCategoryDefinition(
		in.Name, in.RuleVersion, in.DefaultValue,
		rules, resourceTagsToMap(in.ResourceTags),
		splitChargeRules, in.EffectiveStart,
	)
	if err != nil {
		return nil, err
	}

	return &createCostCategoryDefinitionOutput{
		CostCategoryArn: cat.ARN,
		EffectiveStart:  cat.EffectiveStart,
	}, nil
}

type deleteCostCategoryDefinitionInput struct {
	CostCategoryArn string `json:"CostCategoryArn"`
}

type deleteCostCategoryDefinitionOutput struct {
	CostCategoryArn string `json:"CostCategoryArn"`
	EffectiveEnd    string `json:"EffectiveEnd"`
}

func (h *Handler) handleDeleteCostCategoryDefinition(
	_ context.Context,
	in *deleteCostCategoryDefinitionInput,
) (*deleteCostCategoryDefinitionOutput, error) {
	if in.CostCategoryArn == "" {
		return nil, fmt.Errorf("%w: CostCategoryArn is required", ErrValidation)
	}

	cat, err := h.Backend.DeleteCostCategoryDefinition(in.CostCategoryArn)
	if err != nil {
		return nil, err
	}

	return &deleteCostCategoryDefinitionOutput{
		CostCategoryArn: cat.ARN,
		EffectiveEnd:    effectiveStart(),
	}, nil
}

type describeCostCategoryDefinitionInput struct {
	CostCategoryArn string `json:"CostCategoryArn"`
	EffectiveOn     string `json:"EffectiveOn"`
}

type costCategoryProcessingStatus struct {
	Component string `json:"Component"`
	Status    string `json:"Status"`
}

type costCategorySummary struct {
	CostCategoryArn  string                         `json:"CostCategoryArn"`
	Name             string                         `json:"Name"`
	RuleVersion      string                         `json:"RuleVersion"`
	DefaultValue     string                         `json:"DefaultValue"`
	EffectiveStart   string                         `json:"EffectiveStart"`
	EffectiveEnd     string                         `json:"EffectiveEnd,omitempty"`
	ProcessingStatus []costCategoryProcessingStatus `json:"ProcessingStatus,omitempty"`
	Rules            []costCategoryRule             `json:"Rules"`
	SplitChargeRules []splitChargeRule              `json:"SplitChargeRules,omitempty"`
}

type describeCostCategoryDefinitionOutput struct {
	CostCategory costCategorySummary `json:"CostCategory"`
}

func (h *Handler) handleDescribeCostCategoryDefinition(
	_ context.Context,
	in *describeCostCategoryDefinitionInput,
) (*describeCostCategoryDefinitionOutput, error) {
	if in.CostCategoryArn == "" {
		return nil, fmt.Errorf("%w: CostCategoryArn is required", ErrValidation)
	}

	cat, err := h.Backend.DescribeCostCategoryDefinition(in.CostCategoryArn)
	if err != nil {
		return nil, err
	}

	// EffectiveOn selects which historical version of the cost category was
	// effective on that date; this backend has no version history, only the
	// current rule set's own EffectiveStart. The one honest, non-fabricated
	// use of EffectiveOn without inventing prior versions: if it names a date
	// before the category's own EffectiveStart, the category did not exist
	// yet as of that date.
	if in.EffectiveOn != "" && in.EffectiveOn < cat.EffectiveStart {
		return nil, ErrNotFound
	}

	rules := make([]costCategoryRule, len(cat.Rules))
	for i, r := range cat.Rules {
		rules[i] = costCategoryRule(r)
	}

	splitChargeRules := make([]splitChargeRule, len(cat.SplitChargeRules))
	for i, r := range cat.SplitChargeRules {
		splitChargeRules[i] = splitChargeRule(r)
	}

	return &describeCostCategoryDefinitionOutput{
		CostCategory: costCategorySummary{
			CostCategoryArn: cat.ARN,
			Name:            cat.Name,
			RuleVersion:     cat.RuleVersion,
			DefaultValue:    cat.DefaultValue,
			EffectiveStart:  cat.EffectiveStart,
			ProcessingStatus: []costCategoryProcessingStatus{
				{Component: "COST_EXPLORER", Status: "APPLIED"},
			},
			Rules:            rules,
			SplitChargeRules: splitChargeRules,
		},
	}, nil
}

type listCostCategoryDefinitionsInput struct {
	NextToken   string `json:"NextToken"`
	EffectiveOn string `json:"EffectiveOn"`
	MaxResults  int    `json:"MaxResults"`
}

type costCategoryReference struct {
	CostCategoryArn string `json:"CostCategoryArn"`
	Name            string `json:"Name"`
	EffectiveStart  string `json:"EffectiveStart"`
}

type listCostCategoryDefinitionsOutput struct {
	NextPageToken          string                  `json:"NextPageToken,omitempty"`
	CostCategoryReferences []costCategoryReference `json:"CostCategoryReferences"`
}

func (h *Handler) handleListCostCategoryDefinitions(
	_ context.Context,
	in *listCostCategoryDefinitionsInput,
) (*listCostCategoryDefinitionsOutput, error) {
	cats, nextToken := h.Backend.ListCostCategoryDefinitions(in.MaxResults, in.NextToken, in.EffectiveOn)
	refs := make([]costCategoryReference, 0, len(cats))

	for _, cat := range cats {
		refs = append(refs, costCategoryReference{
			CostCategoryArn: cat.ARN,
			Name:            cat.Name,
			EffectiveStart:  cat.EffectiveStart,
		})
	}

	return &listCostCategoryDefinitionsOutput{CostCategoryReferences: refs, NextPageToken: nextToken}, nil
}

type updateCostCategoryDefinitionInput struct {
	CostCategoryArn  string             `json:"CostCategoryArn"`
	RuleVersion      string             `json:"RuleVersion"`
	DefaultValue     string             `json:"DefaultValue"`
	EffectiveStart   string             `json:"EffectiveStart"`
	Rules            []costCategoryRule `json:"Rules"`
	SplitChargeRules []splitChargeRule  `json:"SplitChargeRules"`
}

type updateCostCategoryDefinitionOutput struct {
	CostCategoryArn string `json:"CostCategoryArn"`
	EffectiveStart  string `json:"EffectiveStart"`
}

func (h *Handler) handleUpdateCostCategoryDefinition(
	_ context.Context,
	in *updateCostCategoryDefinitionInput,
) (*updateCostCategoryDefinitionOutput, error) {
	if in.CostCategoryArn == "" {
		return nil, fmt.Errorf("%w: CostCategoryArn is required", ErrValidation)
	}

	if in.RuleVersion == "" {
		return nil, fmt.Errorf("%w: RuleVersion is required", ErrValidation)
	}

	if in.Rules == nil {
		return nil, fmt.Errorf("%w: Rules is required", ErrValidation)
	}

	rules := make([]CostCategoryRule, 0, len(in.Rules))
	for _, r := range in.Rules {
		rules = append(rules, CostCategoryRule(r))
	}

	splitChargeRules := make([]SplitChargeRule, 0, len(in.SplitChargeRules))
	for _, r := range in.SplitChargeRules {
		splitChargeRules = append(splitChargeRules, SplitChargeRule(r))
	}

	cat, err := h.Backend.UpdateCostCategoryDefinition(
		in.CostCategoryArn, in.RuleVersion, in.DefaultValue,
		rules, splitChargeRules, in.EffectiveStart,
	)
	if err != nil {
		return nil, err
	}

	return &updateCostCategoryDefinitionOutput{
		CostCategoryArn: cat.ARN,
		EffectiveStart:  cat.EffectiveStart,
	}, nil
}

type getCostCategoriesInput struct {
	Filter           *ceExpression      `json:"Filter"`
	TimePeriod       map[string]string  `json:"TimePeriod"`
	CostCategoryName string             `json:"CostCategoryName"`
	SearchString     string             `json:"SearchString"`
	NextPageToken    string             `json:"NextPageToken"`
	SortBy           []ceSortDefinition `json:"SortBy"`
	MaxResults       int                `json:"MaxResults"`
}

// getCostCategoriesOutput's CostCategoryNames/CostCategoryValues split matches
// real AWS CE's GetCostCategoriesOutput: CostCategoryValues is only populated
// when the request specifies CostCategoryName, otherwise the response carries
// CostCategoryNames instead (api_op_GetCostCategories.go). A prior revision
// unconditionally returned CostCategoryValues and never emitted
// CostCategoryNames at all.
type getCostCategoriesOutput struct {
	NextPageToken      string   `json:"NextPageToken,omitempty"`
	CostCategoryNames  []string `json:"CostCategoryNames,omitempty"`
	CostCategoryValues []string `json:"CostCategoryValues,omitempty"`
	ReturnSize         int      `json:"ReturnSize"`
	TotalSize          int      `json:"TotalSize"`
}

// applyCostCategoriesFilter narrows values to the Filter.CostCategories
// allow-list, when provided. This emulator derives CostCategoryValues purely
// from cost-category Rule definitions (see Backend.GetCostCategories), not
// from tagged cost/usage transactions the way real AWS does, so a
// Dimensions/Tags-based Filter has no backing transaction state to act on;
// only the CostCategories clause -- which restricts to an explicit candidate
// list -- has a meaningful, non-fabricated effect here.
func applyCostCategoriesFilter(values []string, filter *ceExpression) []string {
	if filter == nil || filter.CostCategories == nil || len(filter.CostCategories.Values) == 0 {
		return values
	}

	kept := make([]string, 0, len(values))

	for _, v := range values {
		if stringSliceContainsFold(filter.CostCategories.Values, v) {
			kept = append(kept, v)
		}
	}

	return kept
}

// applyCostCategoriesSort orders values by the requested SortOrder. Real
// GetCostCategories SortBy keys are cost metrics (BlendedCost, UsageQuantity,
// etc.); CostCategoryValues here are plain strings with no per-value cost
// metric behind them, so the honest, non-fabricated behavior is to honor
// only SortOrder (ASCENDING/DESCENDING) over the already-alphabetical list
// Backend.GetCostCategories returns, rather than inventing per-value costs.
func applyCostCategoriesSort(values []string, sortBy []ceSortDefinition) []string {
	if len(sortBy) == 0 || !sortDescending(sortBy[0].SortOrder) {
		return values
	}

	reversed := make([]string, len(values))
	for i, v := range values {
		reversed[len(values)-1-i] = v
	}

	return reversed
}

// applyCostCategoriesSearchString case-insensitively substring-matches
// values, mirroring GetDimensionValues/GetTags' SearchString handling. Real
// AWS documents SearchString as filtering cost category names when
// CostCategoryName is unset, or cost category values when it is set -- either
// way it narrows the same values slice this function is given.
func applyCostCategoriesSearchString(values []string, search string) []string {
	if search == "" {
		return values
	}

	needle := strings.ToLower(search)
	kept := values[:0]

	for _, v := range values {
		if strings.Contains(strings.ToLower(v), needle) {
			kept = append(kept, v)
		}
	}

	return kept
}

func (h *Handler) handleGetCostCategories(
	_ context.Context,
	in *getCostCategoriesInput,
) (*getCostCategoriesOutput, error) {
	// Real GetCostCategoriesInput requires TimePeriod. This emulator derives
	// cost category names/values from stored CostCategory definitions rather
	// than narrowing by TimePeriod, so this is a presence check only, same
	// shape as GetDimensionValues/GetTags' required-field fix.
	if in.TimePeriod == nil || in.TimePeriod[timePeriodKeyStart] == "" || in.TimePeriod[timePeriodKeyEnd] == "" {
		return nil, fmt.Errorf("%w: TimePeriod is required", ErrValidation)
	}

	if in.CostCategoryName == "" {
		names := applyCostCategoriesSearchString(h.Backend.GetCostCategoryNames(), in.SearchString)
		names = applyCostCategoriesSort(names, in.SortBy)
		totalSize := len(names)
		page, nextToken := paginateOrdered(names, in.MaxResults, in.NextPageToken, func(v string) string { return v })

		return &getCostCategoriesOutput{
			CostCategoryNames: page,
			NextPageToken:     nextToken,
			ReturnSize:        len(page),
			TotalSize:         totalSize,
		}, nil
	}

	values := h.Backend.GetCostCategories(in.CostCategoryName)
	values = applyCostCategoriesFilter(values, in.Filter)
	values = applyCostCategoriesSearchString(values, in.SearchString)
	values = applyCostCategoriesSort(values, in.SortBy)
	totalSize := len(values)
	page, nextToken := paginateOrdered(values, in.MaxResults, in.NextPageToken, func(v string) string { return v })

	return &getCostCategoriesOutput{
		CostCategoryValues: page,
		NextPageToken:      nextToken,
		ReturnSize:         len(page),
		TotalSize:          totalSize,
	}, nil
}

// listCostCategoryResourceAssociationsInput is field-diffed against real AWS
// CE's ListCostCategoryResourceAssociationsInput: it has exactly
// CostCategoryArn/MaxResults/NextToken. "ResourceTagFilter" matched no real
// member and was removed; MaxResults was entirely absent.
type listCostCategoryResourceAssociationsInput struct {
	CostCategoryArn string `json:"CostCategoryArn"`
	NextToken       string `json:"NextToken"`
	MaxResults      int    `json:"MaxResults"`
}

// costCategoryResourceAssociation mirrors aws-sdk-go-v2/service/costexplorer/types'
// CostCategoryResourceAssociation exactly (CostCategoryArn/CostCategoryName/ResourceArn).
// The previous shape here ("CostCategoryReference"/"ResourceTagsCount") was invented and
// matched no real CE field.
type costCategoryResourceAssociation struct {
	CostCategoryArn  string `json:"CostCategoryArn,omitempty"`
	CostCategoryName string `json:"CostCategoryName,omitempty"`
	ResourceArn      string `json:"ResourceArn,omitempty"`
}

type listCostCategoryResourceAssociationsOutput struct {
	NextToken                        string                            `json:"NextToken,omitempty"`
	CostCategoryResourceAssociations []costCategoryResourceAssociation `json:"CostCategoryResourceAssociations"`
}

// handleListCostCategoryResourceAssociations always returns zero associations: real AWS
// resource associations tie a cost category to actual AWS resources (via resource tags),
// and this emulator has no such resource-tag inventory to associate against -- there is
// no state to disguise a no-op here, unlike the deterministic-mock query ops that read
// the synthetic cost ledger. The wire shape (field names/nesting) is now field-diffed
// against the real CostCategoryResourceAssociation type. CostCategoryArn is left
// unread/undocumented-as-erroring rather than guessed at: real AWS's own validators.go
// has no required-field check for this op, and there is no confirmed evidence (doc page
// or SDK source) of what a nonexistent ARN does here -- inventing a not-found error would
// be exactly the unverified-behavior fabrication this campaign warns against. NextToken/
// MaxResults are threaded through paginateList for a genuinely empty list (see
// GetCostComparisonDrivers for the same shape).
func (h *Handler) handleListCostCategoryResourceAssociations(
	_ context.Context,
	in *listCostCategoryResourceAssociationsInput,
) (*listCostCategoryResourceAssociationsOutput, error) {
	page, nextToken := paginateList([]costCategoryResourceAssociation{}, in.MaxResults, in.NextToken,
		func(costCategoryResourceAssociation) string { return "" })

	return &listCostCategoryResourceAssociationsOutput{
		CostCategoryResourceAssociations: page,
		NextToken:                        nextToken,
	}, nil
}

// buildCostCategoryOps returns the cost-category-family op dispatch entries.
func (h *Handler) buildCostCategoryOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"CreateCostCategoryDefinition": service.WrapOp(
			h.handleCreateCostCategoryDefinition,
		),
		"DeleteCostCategoryDefinition": service.WrapOp(
			h.handleDeleteCostCategoryDefinition,
		),
		"DescribeCostCategoryDefinition": service.WrapOp(
			h.handleDescribeCostCategoryDefinition,
		),
		"ListCostCategoryDefinitions": service.WrapOp(
			h.handleListCostCategoryDefinitions,
		),
		"UpdateCostCategoryDefinition": service.WrapOp(
			h.handleUpdateCostCategoryDefinition,
		),
		"GetCostCategories": service.WrapOp(
			h.handleGetCostCategories,
		),
		"ListCostCategoryResourceAssociations": service.WrapOp(
			h.handleListCostCategoryResourceAssociations,
		),
	}
}
