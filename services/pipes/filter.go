package pipes

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/eventpattern"
)

// matchesAnyFilter returns true if body passes at least one of the given filters.
// body is the raw event record body/data for any pipe source type (SQS message
// body, Kinesis record data, or a marshalled DynamoDB Streams record) -- the
// matching engine itself is source-agnostic.
//
// Filter.Pattern semantics:
//   - If Pattern is empty, every message matches (pass-through).
//   - If Pattern is valid JSON starting with '{', it is evaluated as a JSON event
//     pattern (see matchesJSONPattern).
//   - Otherwise the pattern is treated as a literal substring and matched against
//     the raw message body (backward-compatible behaviour).
func matchesAnyFilter(body string, filters []Filter) bool {
	for _, f := range filters {
		if matchesSingleFilter(body, f) {
			return true
		}
	}

	return false
}

func matchesSingleFilter(body string, f Filter) bool {
	if f.Pattern == "" {
		return true
	}

	trimmed := strings.TrimSpace(f.Pattern)
	if strings.HasPrefix(trimmed, "{") {
		return matchesJSONPattern(body, trimmed)
	}

	// Backward-compatible substring match for non-JSON patterns.
	return strings.Contains(body, f.Pattern)
}

// matchesJSONPattern tests whether msgBody satisfies the EventBridge-style
// JSON event pattern (eb-event-patterns-content-based-filtering.html).
// A nested pattern object recurses into the matching message field
// (e.g. {"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}); multiple fields at
// one level are ANDed, multiple array entries for one field are ORed.
//
// Supported content filters (array elements shaped as an object): exists,
// prefix, suffix, numeric, anything-but, cidr, equals-ignore-case (standalone
// and nested inside prefix/suffix), and the "$or" combinator (top-level and
// nested). Unsupported operators (wildcard, and anything-but's object-form
// negation, e.g. {"anything-but":{"prefix":"x"}}) and any other unrecognized
// matcher object never match -- see matchesRuleObject -- rather than
// silently matching everything (gopherstack-5eok: real AWS Pipes' own
// operator-support table, eb-create-pattern-operators.html, marks $or and
// equals-ignore-case "Pipe support: Yes" but wildcard and anything-but's
// object forms "Pipe support: No").
//
// Pattern shape:  {"field": ["value1", "value2", ...], "nested": {"field2": [...]}}
// A field's pattern value must be either an array (of exact-match values
// and/or content-filter objects) or a nested object; a bare scalar is not a
// valid EventBridge pattern value and never matches (see isJSONObject).
func matchesJSONPattern(msgBody, pattern string) bool {
	var patternMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pattern), &patternMap); err != nil {
		// Malformed pattern: fall back to substring.
		return strings.Contains(msgBody, pattern)
	}

	if len(patternMap) == 0 {
		return true
	}

	var msgMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(msgBody), &msgMap); err != nil {
		// Message body is not a JSON object; pattern cannot match.
		return false
	}

	return matchesPatternObject(patternMap, msgMap)
}

// matchesPatternObject reports whether every field in patternMap is
// satisfied by msgMap (fields at one level are ANDed). "$or" is a
// combinator, not a message field, so it is dispatched separately.
func matchesPatternObject(patternMap, msgMap map[string]json.RawMessage) bool {
	for field, ruleRaw := range patternMap {
		if field == "$or" {
			if !matchesOrCombinator(ruleRaw, msgMap) {
				return false
			}

			continue
		}

		msgVal, exists := msgMap[field]
		if !fieldMatchesRule(msgVal, exists, ruleRaw) {
			return false
		}
	}

	return true
}

// matchesOrCombinator evaluates a "$or" combinator
// (eb-filtering-complex-example-or): ruleRaw must decode to an array of
// pattern objects, and msgMap matches if any one of them fully matches.
func matchesOrCombinator(ruleRaw json.RawMessage, msgMap map[string]json.RawMessage) bool {
	var alternatives []json.RawMessage
	if err := json.Unmarshal(ruleRaw, &alternatives); err != nil {
		return false
	}

	for _, alt := range alternatives {
		altPattern, ok := decodeJSONObject(alt)
		if !ok {
			continue
		}

		if matchesPatternObject(altPattern, msgMap) {
			return true
		}
	}

	return false
}

// isJSONObject reports whether raw is a JSON object ('{...}'), checked by
// its first non-whitespace byte rather than by attempting to unmarshal it --
// unmarshalling JSON `null` into a map succeeds with a nil map, which would
// otherwise misclassify a literal null rule element as an object.
func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)

	return len(trimmed) > 0 && trimmed[0] == '{'
}

func decodeJSONObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}

	return obj, true
}

// fieldMatchesRule checks whether msgVal satisfies a pattern field's rule
// value: either a nested object (recurse a level deeper into the event) or
// an array of exact-match values / content-filter objects, ORed together.
// exists reports whether the field was present in the message at all --
// needed because {"exists": false} matches precisely when the field is
// absent, so evaluation cannot short-circuit on absence the way every other
// operator does.
func fieldMatchesRule(msgVal json.RawMessage, exists bool, ruleRaw json.RawMessage) bool {
	if isJSONObject(ruleRaw) {
		return matchesNestedRule(msgVal, exists, ruleRaw)
	}

	var rules []json.RawMessage
	if err := json.Unmarshal(ruleRaw, &rules); err != nil {
		return false
	}

	for _, rule := range rules {
		if matchesRule(msgVal, exists, rule) {
			return true
		}
	}

	return false
}

// matchesNestedRule descends one level into the event for a nested pattern
// object. A nested pattern can only match a present, JSON-object-shaped
// field.
func matchesNestedRule(msgVal json.RawMessage, exists bool, ruleRaw json.RawMessage) bool {
	if !exists {
		return false
	}

	nestedPattern, ok := decodeJSONObject(ruleRaw)
	if !ok {
		return false
	}

	nestedMsg, ok := decodeJSONObject(msgVal)
	if !ok {
		return false
	}

	return matchesPatternObject(nestedPattern, nestedMsg)
}

// matchesRule evaluates a single rule element from a field's match array
// against msgVal. Supported shapes:
//   - "string"/number/bool/null   — exact equality (type-sensitive)
//   - {"prefix": "pfx"}           — string prefix match
//   - {"suffix": "sfx"}           — string suffix match
//   - {"numeric": [">", 5]}       — numeric comparison
//   - {"anything-but": "a"}       — negation (single value)
//   - {"anything-but": ["a","b"]} — negation (list)
//   - {"cidr": "10.0.0.0/24"}     — CIDR IP range match
//   - {"exists": true/false}      — field presence
//   - {"equals-ignore-case": "a"} — case-insensitive string equality
//   - {"prefix": {"equals-ignore-case": "a"}} — case-insensitive prefix
//   - {"suffix": {"equals-ignore-case": "a"}} — case-insensitive suffix
//
// Any other matcher object key is unrecognized and never matches -- fails
// closed rather than risking a false positive for an unsupported operator.
// msgExists is false whenever the field was absent from the message; every
// operator besides exists requires a value to compare against, so absence
// fails them all except an explicit {"exists": false}.
func matchesRule(msgVal json.RawMessage, msgExists bool, rule json.RawMessage) bool {
	if isJSONObject(rule) {
		ruleObj, ok := decodeJSONObject(rule)
		if !ok {
			return false
		}

		if want, hasExists := existsWant(ruleObj); hasExists {
			return msgExists == want
		}

		if !msgExists {
			return false
		}

		return matchesRuleObject(msgVal, ruleObj)
	}

	if !msgExists {
		return false
	}

	return matchesExactRule(msgVal, rule)
}

// matchesExactRule reports whether msgVal and rule decode to the identical
// JSON scalar (string, number, bool, or null) -- type-sensitive, matching
// EventBridge's exact-match semantics (a string rule does not match a
// numerically-equal event number). reflect.DeepEqual (not ==) is required
// because a malformed pattern or event can decode to a non-comparable type
// (a JSON array or object), which would panic under ==.
func matchesExactRule(msgVal, rule json.RawMessage) bool {
	var ruleAny, msgAny any
	if err := json.Unmarshal(rule, &ruleAny); err != nil {
		return false
	}

	if err := json.Unmarshal(msgVal, &msgAny); err != nil {
		return false
	}

	return reflect.DeepEqual(msgAny, ruleAny)
}

// matchesRuleObject dispatches a single-key content-filter object to its
// matcher. Supported: prefix, suffix, numeric, cidr, anything-but,
// equals-ignore-case.
func matchesRuleObject(msgVal json.RawMessage, ruleObj map[string]json.RawMessage) bool {
	if prefixRaw, ok := ruleObj["prefix"]; ok {
		return matchesPrefixRule(msgVal, prefixRaw)
	}

	if suffixRaw, ok := ruleObj["suffix"]; ok {
		return matchesSuffixRule(msgVal, suffixRaw)
	}

	if numericRaw, ok := ruleObj["numeric"]; ok {
		return matchesNumericRule(msgVal, numericRaw)
	}

	if cidrRaw, ok := ruleObj["cidr"]; ok {
		return matchesCIDRRule(msgVal, cidrRaw)
	}

	if anythingButRaw, ok := ruleObj["anything-but"]; ok {
		return !matchesAnythingBut(anythingButRaw, msgVal)
	}

	if eqIgnoreCaseRaw, ok := ruleObj["equals-ignore-case"]; ok {
		return matchesEqualsIgnoreCaseRule(msgVal, eqIgnoreCaseRaw)
	}

	return false
}

func decodeString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}

	return s, true
}

func decodeFloat(raw json.RawMessage) (float64, bool) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}

	return f, true
}

// matchesPrefixRule matches a prefix rule value against msgVal. AWS supports
// both a plain string prefix and a case-insensitive nested form
// (eb-filtering-prefix-matching-ignore-case): {"prefix": {"equals-ignore-case": "foo"}}.
func matchesPrefixRule(msgVal, prefixRaw json.RawMessage) bool {
	msgStr, ok := decodeString(msgVal)
	if !ok {
		return false
	}

	if prefix, isString := decodeString(prefixRaw); isString {
		return strings.HasPrefix(msgStr, prefix)
	}

	nested, ok := decodeJSONObject(prefixRaw)
	if !ok {
		return false
	}

	ci, ok := nestedEqualsIgnoreCase(nested)
	if !ok {
		return false
	}

	return len(msgStr) >= len(ci) && strings.EqualFold(msgStr[:len(ci)], ci)
}

// matchesSuffixRule matches a suffix rule value against msgVal. AWS supports
// both a plain string suffix and a case-insensitive nested form
// (eb-filtering-suffix-matching-ignore-case): {"suffix": {"equals-ignore-case": "foo"}}.
func matchesSuffixRule(msgVal, suffixRaw json.RawMessage) bool {
	msgStr, ok := decodeString(msgVal)
	if !ok {
		return false
	}

	if suffix, isString := decodeString(suffixRaw); isString {
		return strings.HasSuffix(msgStr, suffix)
	}

	nested, ok := decodeJSONObject(suffixRaw)
	if !ok {
		return false
	}

	ci, ok := nestedEqualsIgnoreCase(nested)
	if !ok {
		return false
	}

	return len(msgStr) >= len(ci) && strings.EqualFold(msgStr[len(msgStr)-len(ci):], ci)
}

// nestedEqualsIgnoreCase extracts the string value of a nested
// {"equals-ignore-case": "..."} object, as used inside prefix/suffix rules.
func nestedEqualsIgnoreCase(nested map[string]json.RawMessage) (string, bool) {
	ciRaw, ok := nested["equals-ignore-case"]
	if !ok {
		return "", false
	}

	return decodeString(ciRaw)
}

// matchesEqualsIgnoreCaseRule matches a standalone
// {"equals-ignore-case": "..."} rule (eb-filtering-equals-ignore-case-matching).
func matchesEqualsIgnoreCaseRule(msgVal, valRaw json.RawMessage) bool {
	msgStr, ok := decodeString(msgVal)
	if !ok {
		return false
	}

	want, ok := decodeString(valRaw)
	if !ok {
		return false
	}

	return strings.EqualFold(msgStr, want)
}

// matchesNumericRule applies numeric comparison rules like [">", 5, "<", 10]
// (eb-filtering-numeric-matching) via pkgs/eventpattern, shared with
// services/eventbridge/pattern.go's matchNumeric. msgVal must decode as a
// JSON number; a string-encoded number (as DynamoDB Streams' "N" attribute
// wrapper produces) does not match.
func matchesNumericRule(msgVal, rulesRaw json.RawMessage) bool {
	var ruleList []any
	if err := json.Unmarshal(rulesRaw, &ruleList); err != nil {
		return false
	}

	num, ok := decodeFloat(msgVal)
	if !ok {
		return false
	}

	return eventpattern.MatchNumericRules(num, ruleList)
}

// matchesCIDRRule reports whether the string msgVal is an IP address inside
// the given CIDR range (eb-filtering-ip-address), via pkgs/eventpattern,
// shared with services/eventbridge/pattern.go's matchCIDR.
func matchesCIDRRule(msgVal, cidrRaw json.RawMessage) bool {
	ipStr, ok := decodeString(msgVal)
	if !ok {
		return false
	}

	cidrStr, ok := decodeString(cidrRaw)
	if !ok {
		return false
	}

	return eventpattern.MatchCIDR(cidrStr, ipStr)
}

// existsWant extracts {"exists": true/false}'s boolean, ok=false if the key
// is absent or not a bool.
func existsWant(ruleObj map[string]json.RawMessage) (bool, bool) {
	existsRaw, hasKey := ruleObj["exists"]
	if !hasKey {
		return false, false
	}

	var want bool
	if err := json.Unmarshal(existsRaw, &want); err != nil {
		return false, false
	}

	return want, true
}

// matchesAnythingBut reports whether msgVal equals the anything-but rule
// value, which per the docs (eb-filtering-anything-but) may be a single
// string or a list of strings -- "state": [ { "anything-but": "initializing" } ]
// is the documented single-value form, distinct from the list form. Only
// string comparison is supported (matching the values DynamoDB Streams
// records use, where every AttributeValue is string-wrapped); a non-string
// msgVal is never equal to a string exclusion value, so it always counts as
// "anything but" (matchesRuleObject negates this function's result).
func matchesAnythingBut(anythingButRaw, msgVal json.RawMessage) bool {
	msgStr, ok := decodeString(msgVal)
	if !ok {
		return false
	}

	var single string
	if err := json.Unmarshal(anythingButRaw, &single); err == nil {
		return msgStr == single
	}

	var excluded []string
	if err := json.Unmarshal(anythingButRaw, &excluded); err == nil {
		return slices.Contains(excluded, msgStr)
	}

	return false
}
