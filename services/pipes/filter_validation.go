package pipes

import (
	"encoding/json"
	"fmt"
	"strings"
)

// isKnownPipeMatcher reports whether key is a matcher-object key that pipes'
// filter engine (filter.go's matchesRuleObject) actually evaluates.
// eventbridge's pattern language additionally supports wildcard and
// anything-but's object forms, but real AWS Pipes does not support those
// either (eb-create-pattern-operators.html's "Pipe support" column marks
// both "No") -- validateFilterPattern rejects them here rather than
// accepting a pattern that can never match at delivery time (gopherstack-5eok).
func isKnownPipeMatcher(key string) bool {
	switch key {
	case "prefix", "suffix", "exists", "numeric", "anything-but", "cidr", "equals-ignore-case":
		return true
	}

	return false
}

// validateFilterCriteria validates every Filter.Pattern in fc. Real AWS
// Pipes throws a ValidationException for a structurally invalid filter
// pattern at CreatePipe/UpdatePipe time (eb-pipes-event-filtering.html's
// stream-filtering table: "EventBridge throws an exception at the time of
// Pipe creation or update. The filter pattern must be valid JSON format.").
func validateFilterCriteria(fc *FilterCriteria) error {
	if fc == nil {
		return nil
	}

	for i, f := range fc.Filters {
		if err := validateFilterPattern(f.Pattern); err != nil {
			return fmt.Errorf("FilterCriteria.Filters[%d]: %w", i, err)
		}
	}

	return nil
}

// validateFilterPattern validates a single Filter.Pattern string.
//
// Only patterns filter.go's matchesSingleFilter treats as JSON event
// patterns -- those starting with '{' -- are structurally validated here;
// everything else, including the empty pattern, is filter.go's documented
// backward-compatible literal substring match and needs no shape validation.
func validateFilterPattern(pattern string) error {
	trimmed := strings.TrimSpace(pattern)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}

	var patternMap map[string]any
	if err := json.Unmarshal([]byte(trimmed), &patternMap); err != nil {
		return fmt.Errorf("pattern: %w: not valid JSON: %w", ErrValidation, err)
	}

	return validatePipePatternObject(patternMap)
}

// validatePipePatternObject validates the structure of a pipe event pattern
// object. Each field value must be an array of matchers or a nested object,
// except "$or" (eb-create-pattern-operators.html: "Pipe support: Yes"),
// which is a combinator whose value is an array of pattern objects
// (gopherstack-5eok).
func validatePipePatternObject(pattern map[string]any) error {
	for key, val := range pattern {
		if key == "$or" {
			if err := validatePipeOrCombinator(val); err != nil {
				return err
			}

			continue
		}

		switch v := val.(type) {
		case map[string]any:
			if err := validatePipePatternObject(v); err != nil {
				return err
			}
		case []any:
			if err := validatePipeMatcherArray(key, v); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: value for field %q must be an array or object, got scalar", ErrValidation, key)
		}
	}

	return nil
}

// validatePipeOrCombinator validates the "$or" combinator value: an array of
// pattern objects, each independently valid (eb-filtering-complex-example-or).
func validatePipeOrCombinator(val any) error {
	alts, ok := val.([]any)
	if !ok {
		return fmt.Errorf("%w: $or must be an array", ErrValidation)
	}

	for _, alt := range alts {
		subPat, isMap := alt.(map[string]any)
		if !isMap {
			return fmt.Errorf("%w: $or elements must be objects", ErrValidation)
		}

		if err := validatePipePatternObject(subPat); err != nil {
			return err
		}
	}

	return nil
}

// validatePipeMatcherArray validates a pattern array for a single field.
func validatePipeMatcherArray(field string, matchers []any) error {
	for _, m := range matchers {
		switch mv := m.(type) {
		case string, float64, bool, nil:
			// Exact-match primitives are always valid.
		case map[string]any:
			if err := validatePipeMatcherObject(field, mv); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: matcher for field %q has unsupported type", ErrValidation, field)
		}
	}

	return nil
}

// validatePipeMatcherObject validates a single matcher object (e.g.
// {"prefix": "foo"}) against the operator subset filter.go implements.
func validatePipeMatcherObject(field string, m map[string]any) error {
	for key, val := range m {
		if !isKnownPipeMatcher(key) {
			return fmt.Errorf(
				"%w: matcher %q for field %q is not supported by pipe filters (gopherstack-5eok)",
				ErrValidation, key, field,
			)
		}

		switch key {
		case "prefix", "suffix":
			if err := validatePipePrefixSuffixValue(field, key, val); err != nil {
				return err
			}
		case "equals-ignore-case":
			if _, ok := val.(string); !ok {
				return fmt.Errorf(
					"%w: equals-ignore-case value for field %q must be a string", ErrValidation, field,
				)
			}
		case "anything-but":
			if err := validatePipeAnythingButValue(field, val); err != nil {
				return err
			}
		}
	}

	return nil
}

// validatePipePrefixSuffixValue validates a prefix/suffix matcher's value:
// either a plain string, or the nested case-insensitive form real AWS
// documents for pipes ({"prefix": {"equals-ignore-case": "..."}}) --
// eb-create-pattern-operators.html's "Begins with (ignore case)"/"Ends with
// (ignore case)" rows are both "Pipe support: Yes".
func validatePipePrefixSuffixValue(field, key string, val any) error {
	if _, ok := val.(string); ok {
		return nil
	}

	nested, ok := val.(map[string]any)
	if !ok {
		return fmt.Errorf(
			"%w: %q matcher for field %q must be a string or {\"equals-ignore-case\": <string>}",
			ErrValidation, key, field,
		)
	}

	ci, hasKey := nested["equals-ignore-case"]
	if !hasKey || len(nested) != 1 {
		return fmt.Errorf(
			"%w: nested %q matcher for field %q only supports equals-ignore-case", ErrValidation, key, field,
		)
	}

	if _, isString := ci.(string); !isString {
		return fmt.Errorf("%w: equals-ignore-case value for field %q must be a string", ErrValidation, field)
	}

	return nil
}

// validatePipeAnythingButValue validates the value of an "anything-but"
// matcher against filter.go's matchesAnythingBut, which only decodes a
// single string or a list of strings -- narrower than eventbridge's
// anything-but (no numbers, no object-form negated prefix/suffix/wildcard/
// equals-ignore-case, gopherstack-5eok).
func validatePipeAnythingButValue(field string, v any) error {
	switch ab := v.(type) {
	case string:
		return nil
	case []any:
		for _, elem := range ab {
			if _, ok := elem.(string); !ok {
				return fmt.Errorf(
					"%w: anything-but list for field %q must contain only strings (pipe filters do not"+
						" yet support numeric or object-form anything-but, gopherstack-5eok)",
					ErrValidation, field,
				)
			}
		}

		return nil
	default:
		return fmt.Errorf(
			"%w: anything-but value for field %q must be a string or a list of strings (pipe filters do"+
				" not yet support numeric or object-form anything-but, gopherstack-5eok)",
			ErrValidation, field,
		)
	}
}
