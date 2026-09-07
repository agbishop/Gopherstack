package pipes

import (
	"encoding/json"
	"fmt"
	"strings"
)

// isKnownPipeMatcher reports whether key is a matcher-object key that pipes'
// filter engine (filter.go's matchesRuleObject) actually evaluates.
// eventbridge's pattern language additionally supports $or, wildcard, and
// equals-ignore-case (standalone and nested in prefix/suffix), and anything-but's
// object forms, but filter.go does not implement any of those yet
// (gopherstack-5eok) -- validateFilterPattern rejects them here rather than
// accepting a pattern that can never match at delivery time.
func isKnownPipeMatcher(key string) bool {
	switch key {
	case "prefix", "suffix", "exists", "numeric", "anything-but", "cidr":
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
// object. Each field value must be an array of matchers or a nested object;
// unlike eventbridge's validatePatternObject, "$or" gets no special
// top-level handling -- pipes' filter.go does not implement $or, so it is
// rejected explicitly (gopherstack-5eok).
func validatePipePatternObject(pattern map[string]any) error {
	for key, val := range pattern {
		if key == "$or" {
			return fmt.Errorf("%w: $or is not supported by pipe filters (gopherstack-5eok)", ErrValidation)
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
			// filter.go's matchesPrefixRule/matchesSuffixRule only decode a
			// plain string; the nested equals-ignore-case form real AWS
			// documents for pipes ({"prefix": {"equals-ignore-case": ...}})
			// is not implemented (gopherstack-5eok).
			if _, ok := val.(string); !ok {
				return fmt.Errorf(
					"%w: %q matcher for field %q must be a string; case-insensitive matching is not"+
						" yet supported by pipe filters (gopherstack-5eok)",
					ErrValidation, key, field,
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
