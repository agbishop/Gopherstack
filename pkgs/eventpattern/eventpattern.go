// Package eventpattern holds the numeric-comparison and CIDR-matching core
// shared by EventBridge's and Pipes' event-pattern content filters
// (eb-event-patterns-content-based-filtering.html). Pipes' Filter.Pattern is
// specified as an EventBridge pattern with no grammar of its own, so the two
// services' matchers are meant to stay aligned; this package holds just the
// pieces that were already identical, so they no longer drift out of sync by
// hand. The two services' broader matcher-dispatch trees (operator support,
// unrecognized-key handling, validation) diverge enough that they are not
// unified here -- see bd gopherstack-amfu.
package eventpattern

import "net"

// CompareNumeric reports whether "num op val" holds for one of EventBridge's
// numeric matcher operators (eb-filtering-numeric-matching): >, >=, <, <=, =.
// An unrecognized op reports false.
func CompareNumeric(op string, num, val float64) bool {
	switch op {
	case ">":
		return num > val
	case ">=":
		return num >= val
	case "<":
		return num < val
	case "<=":
		return num <= val
	case "=":
		return num == val
	default:
		return false
	}
}

// MatchNumericRules applies EventBridge's numeric matcher rules to num.
// Rules come in (operator, value) pairs -- e.g. [">", 5, "<", 10] -- that
// must ALL hold; a malformed pair (wrong types) fails the match.
func MatchNumericRules(num float64, rules []any) bool {
	const pairSize = 2

	for i := 0; i+1 < len(rules); i += pairSize {
		op, opOK := rules[i].(string)
		val, valOK := ToFloat64(rules[i+1])

		if !opOK || !valOK || !CompareNumeric(op, num, val) {
			return false
		}
	}

	return true
}

// ToFloat64 converts a decoded JSON numeric value to float64.
func ToFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// MatchCIDR reports whether ipStr is an IP address inside the cidrStr range
// (eb-filtering-ip-address). Either being malformed reports false.
func MatchCIDR(cidrStr, ipStr string) bool {
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	return ipNet.Contains(ip)
}
