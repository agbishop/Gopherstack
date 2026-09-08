package guardduty

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Condition represents a single finding-criteria condition, matching
// aws-sdk-go-v2/service/guardduty/types.Condition field-for-field (including
// the deprecated Eq/Neq/Gt/Gte/Lt/Lte aliases real clients may still send).
type Condition struct {
	GreaterThan        *int64   `json:"greaterThan,omitempty"`
	GreaterThanOrEqual *int64   `json:"greaterThanOrEqual,omitempty"`
	LessThan           *int64   `json:"lessThan,omitempty"`
	LessThanOrEqual    *int64   `json:"lessThanOrEqual,omitempty"`
	Gt                 *int32   `json:"gt,omitempty"`
	Gte                *int32   `json:"gte,omitempty"`
	Lt                 *int32   `json:"lt,omitempty"`
	Lte                *int32   `json:"lte,omitempty"`
	Equals             []string `json:"equals,omitempty"`
	NotEquals          []string `json:"notEquals,omitempty"`
	Eq                 []string `json:"eq,omitempty"`
	Neq                []string `json:"neq,omitempty"`
	Matches            []string `json:"matches,omitempty"`
	NotMatches         []string `json:"notMatches,omitempty"`
}

// FindingCriteria represents the criteria used for querying findings,
// matching types.FindingCriteria.
type FindingCriteria struct {
	Criterion map[string]Condition `json:"criterion,omitempty"`
}

// conditionsFromRaw converts a Filter's stored raw findingCriteria (kept as
// map[string]any for echo-back fidelity) into the typed map[string]Condition
// matchesFindingCriteria expects, by round-tripping it through the same
// {"criterion": {...}} wire shape ListFindings' typed FindingCriteria parses
// directly.
func conditionsFromRaw(raw map[string]any) map[string]Condition {
	if len(raw) == 0 {
		return nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}

	var fc FindingCriteria
	if unmarshalErr := json.Unmarshal(data, &fc); unmarshalErr != nil {
		return nil
	}

	return fc.Criterion
}

// SortCriteria represents the sort ordering for ListFindings, matching
// types.SortCriteria.
type SortCriteria struct {
	AttributeName string `json:"attributeName,omitempty"`
	OrderBy       string `json:"orderBy,omitempty"`
}

// findingToFieldMap flattens a Finding into a generic map keyed by its wire
// field names (via its existing json tags), so FindingCriteria's dot-path
// attribute names ("service.archived", "resource.resourceType", ...) can be
// resolved the same way a real GuardDuty backend would resolve them against
// the finding document.
func findingToFieldMap(f *Finding) map[string]any {
	data, marshalErr := json.Marshal(f)
	if marshalErr != nil {
		return map[string]any{}
	}

	var m map[string]any
	if unmarshalErr := json.Unmarshal(data, &m); unmarshalErr != nil {
		return map[string]any{}
	}

	return m
}

// fieldByPath resolves a dot-separated attribute path ("service.archived")
// against a flattened finding map.
func fieldByPath(m map[string]any, path string) (any, bool) {
	cur := any(m)

	for part := range strings.SplitSeq(path, ".") {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}

		v, ok := asMap[part]
		if !ok {
			return nil, false
		}

		cur = v
	}

	return cur, true
}

// matchesFindingCriteria reports whether f satisfies every condition in
// criteria. AWS ANDs every criterion entry together; an unset/empty criteria
// map matches everything.
func matchesFindingCriteria(f *Finding, criteria map[string]Condition) bool {
	if len(criteria) == 0 {
		return true
	}

	fields := findingToFieldMap(f)

	for attr, cond := range criteria {
		val, ok := fieldByPath(fields, attr)
		if !ok || !conditionMatches(val, cond) {
			return false
		}
	}

	return true
}

func conditionMatches(val any, c Condition) bool {
	s := valueAsString(val)

	return equalsMatch(s, c.Equals) &&
		equalsMatch(s, c.Eq) &&
		notEqualsMatch(s, c.NotEquals) &&
		notEqualsMatch(s, c.Neq) &&
		numericMatch(val, c) &&
		matchesPatterns(s, c.Matches, true) &&
		matchesPatterns(s, c.NotMatches, false)
}

func equalsMatch(s string, list []string) bool {
	return len(list) == 0 || slices.Contains(list, s)
}

func notEqualsMatch(s string, list []string) bool {
	return !slices.Contains(list, s)
}

func numericMatch(val any, c Condition) bool {
	return numGTE(val, c.GreaterThan, false) &&
		numGTE(val, c.GreaterThanOrEqual, true) &&
		numLTE(val, c.LessThan, false) &&
		numLTE(val, c.LessThanOrEqual, true) &&
		numGTE32(val, c.Gt, false) &&
		numGTE32(val, c.Gte, true) &&
		numLTE32(val, c.Lt, false) &&
		numLTE32(val, c.Lte, true)
}

func numGTE(val any, threshold *int64, orEqual bool) bool {
	if threshold == nil {
		return true
	}

	f, ok := valueAsFloat64(val)
	if !ok {
		return false
	}

	if orEqual {
		return f >= float64(*threshold)
	}

	return f > float64(*threshold)
}

func numLTE(val any, threshold *int64, orEqual bool) bool {
	if threshold == nil {
		return true
	}

	f, ok := valueAsFloat64(val)
	if !ok {
		return false
	}

	if orEqual {
		return f <= float64(*threshold)
	}

	return f < float64(*threshold)
}

func numGTE32(val any, threshold *int32, orEqual bool) bool {
	if threshold == nil {
		return true
	}

	t := int64(*threshold)

	return numGTE(val, &t, orEqual)
}

func numLTE32(val any, threshold *int32, orEqual bool) bool {
	if threshold == nil {
		return true
	}

	t := int64(*threshold)

	return numLTE(val, &t, orEqual)
}

// matchesPatterns reports whether s matches at least one of patterns
// (want=true, used for "matches") or matches none of them (want=false, used
// for "notMatches"). An empty pattern list always satisfies the condition,
// since the caller never supplied it. Patterns may contain "*" wildcards, as
// real GuardDuty finding-criteria matches support.
func matchesPatterns(s string, patterns []string, want bool) bool {
	if len(patterns) == 0 {
		return true
	}

	anyMatch := false

	for _, p := range patterns {
		if globMatch(s, p) {
			anyMatch = true

			break
		}
	}

	return anyMatch == want
}

func globMatch(s, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return s == pattern
	}

	quoted := strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*")

	re, err := regexp.Compile("^" + quoted + "$")
	if err != nil {
		return s == pattern
	}

	return re.MatchString(s)
}

// valueAsString renders a flattened field value the way a real finding
// document attribute would be compared for equals/notEquals/matches: numbers
// use Go's default (shortest round-trip) formatting, which agrees with how
// GuardDuty's own docs show severity comparisons (e.g. "8" not "8.000000").
func valueAsString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

// valueAsFloat64 coerces a flattened field value to a float64 for numeric
// comparisons. RFC3339 timestamp strings (Finding.CreatedAt/UpdatedAt are
// wire-shape strings, not JSON numbers) are converted to epoch
// milliseconds, matching how GuardDuty documents numeric conditions against
// timestamp attributes.
func valueAsFloat64(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return float64(t.UnixMilli()), true
		}

		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}

	return 0, false
}

// sortFindings orders findings in place by attr ("severity", "updatedAt",
// "type", ...; empty defaults to "id") according to orderBy ("ASC"/"DESC";
// empty defaults to "ASC", matching types.OrderBy's documented default).
func sortFindings(items []*Finding, attr, orderBy string) {
	if attr == "" {
		attr = "id"
	}

	desc := strings.EqualFold(orderBy, "DESC")

	slices.SortFunc(items, func(a, b *Finding) int {
		av, _ := fieldByPath(findingToFieldMap(a), attr)
		bv, _ := fieldByPath(findingToFieldMap(b), attr)

		c := compareFieldValues(av, bv)
		if desc {
			c = -c
		}

		return c
	})
}

func compareFieldValues(a, b any) int {
	if af, aok := valueAsFloat64(a); aok {
		if bf, bok := valueAsFloat64(b); bok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}

	return strings.Compare(valueAsString(a), valueAsString(b))
}
