package ce

import "strings"

// ceDimensionValues mirrors the wire shape of costexplorer's types.DimensionValues:
// a dimension Key plus the Values OR'd together for that dimension.
type ceDimensionValues struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values"`
}

// ceTagValues mirrors costexplorer's types.TagValues.
type ceTagValues struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values"`
}

// ceCostCategoryValues mirrors costexplorer's types.CostCategoryValues.
type ceCostCategoryValues struct {
	Key    string   `json:"Key"`
	Values []string `json:"Values"`
}

// ceExpression mirrors costexplorer's types.Expression, including the And/Or/Not
// boolean composition (api_op_GetCostAndUsage.go:85-86: "You can nest Expression
// objects to define any combination of dimension filters"). matchesExpression below
// evaluates the full tree for GetCostAndUsage; most other call sites in this package
// still only look at the top-level Dimensions/Tags clause via narrower per-op shims
// (serviceDimensionFilter and friends) tied to their own backend's data model.
type ceExpression struct {
	Not            *ceExpression         `json:"Not,omitempty"`
	Dimensions     *ceDimensionValues    `json:"Dimensions,omitempty"`
	Tags           *ceTagValues          `json:"Tags,omitempty"`
	CostCategories *ceCostCategoryValues `json:"CostCategories,omitempty"`
	And            []ceExpression        `json:"And,omitempty"`
	Or             []ceExpression        `json:"Or,omitempty"`
}

// matchesExpression evaluates the full Dimensions/Tags/And/Or/Not tree against a
// cost ledger entry. CostCategories clauses are not modeled per-entry (no
// per-usage cost-category assignment exists in this ledger) so they don't narrow.
// An unmodeled Dimensions key (dimensionFieldValue's ok=false) also doesn't narrow,
// rather than spuriously matching nothing.
func matchesExpression(e CostEntry, expr *ceExpression) bool {
	if expr == nil {
		return true
	}

	switch {
	case len(expr.And) > 0:
		for i := range expr.And {
			if !matchesExpression(e, &expr.And[i]) {
				return false
			}
		}

		return true
	case len(expr.Or) > 0:
		for i := range expr.Or {
			if matchesExpression(e, &expr.Or[i]) {
				return true
			}
		}

		return false
	case expr.Not != nil:
		return !matchesExpression(e, expr.Not)
	case expr.Dimensions != nil:
		val, ok := dimensionFieldValue(e, expr.Dimensions.Key)
		if !ok {
			return true
		}

		return stringSliceContainsFold(expr.Dimensions.Values, val)
	case expr.Tags != nil:
		return stringSliceContainsFold(expr.Tags.Values, e.Tags[expr.Tags.Key])
	default:
		return true
	}
}

// ceSortDefinition mirrors costexplorer's types.SortDefinition.
type ceSortDefinition struct {
	Key       string `json:"Key"`
	SortOrder string `json:"SortOrder,omitempty"`
}

func sortDescending(order string) bool {
	return strings.EqualFold(order, "DESCENDING")
}

// stringSliceContainsFold reports whether values contains s, case-insensitively.
func stringSliceContainsFold(values []string, s string) bool {
	for _, v := range values {
		if strings.EqualFold(v, s) {
			return true
		}
	}

	return false
}
