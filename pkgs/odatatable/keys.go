package odatatable

import "strings"

// UnquoteODataString unquotes a single '...'-delimited OData string literal
// (escaped by doubling: a single quote written twice in a row means one
// literal quote), such as the table-name literal in DELETE
// /<account>/Tables('foo'). Returns ("", false) for anything else (missing/
// mismatched quotes, an unescaped quote inside).
func UnquoteODataString(s string) (string, bool) {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", false
	}

	inner := s[1 : len(s)-1]

	var b strings.Builder

	i := 0
	for i < len(inner) {
		if inner[i] == '\'' {
			if i+1 < len(inner) && inner[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2

				continue
			}

			return "", false
		}

		b.WriteByte(inner[i])
		i++
	}

	return b.String(), true
}

// EscapeODataKey escapes a key value for embedding back into a
// single-quoted OData literal (the inverse of UnquoteODataString).
func EscapeODataKey(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// splitTopLevelCommas splits s on commas that are not inside a single-quoted
// literal. Quote-parity tracking (not full escape-aware parsing) is
// sufficient here: an escaped (doubled) quote toggles parity twice, correctly
// leaving the parser "still inside" the literal it started in.
func splitTopLevelCommas(s string) []string {
	var parts []string

	var b strings.Builder

	inQuote := false

	for i := range len(s) {
		c := s[i]

		switch {
		case c == '\'':
			inQuote = !inQuote

			b.WriteByte(c)
		case c == ',' && !inQuote:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}

	parts = append(parts, b.String())

	return parts
}

// entityKeyPredicateParts is the fixed arity of a "PartitionKey='p',RowKey='r'"
// predicate: exactly PartitionKey and RowKey, in either order.
const entityKeyPredicateParts = 2

// ParseEntityKeyPredicate parses an entity key predicate
// ("PartitionKey='p',RowKey='r'", in either key order) into its two values.
// Returns ok=false for anything malformed.
func ParseEntityKeyPredicate(predicate string) (string, string, bool) {
	parts := splitTopLevelCommas(predicate)
	if len(parts) != entityKeyPredicateParts {
		return "", "", false
	}

	var partitionKey, rowKey string

	var havePK, haveRK bool

	for _, part := range parts {
		key, value, splitOK := strings.Cut(part, "=")
		if !splitOK {
			return "", "", false
		}

		key = strings.TrimSpace(key)

		unquoted, unquoteOK := UnquoteODataString(strings.TrimSpace(value))
		if !unquoteOK {
			return "", "", false
		}

		switch key {
		case PartitionKeyProperty:
			partitionKey, havePK = unquoted, true
		case RowKeyProperty:
			rowKey, haveRK = unquoted, true
		default:
			return "", "", false
		}
	}

	if !havePK || !haveRK {
		return "", "", false
	}

	return partitionKey, rowKey, true
}
