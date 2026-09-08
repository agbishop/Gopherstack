package athena

import "strings"

// execExecute runs a prepared statement: EXECUTE statement_name [USING v1, v2, ...].
// Athena docs: "EXECUTE {{statement_name}} [USING {{value1}} [ ,{{value2}}, ... ] ]"
// (https://docs.aws.amazon.com/athena/latest/ug/querying-with-prepared-statements-execute.html).
// Prepared statements are workgroup-scoped, so lookup uses the workgroup the query
// was submitted to; a name that exists only in a different workgroup is
// indistinguishable from a name that does not exist at all, matching AWS.
func (b *InMemoryBackend) execExecute(
	query, workGroup string, ctx QueryExecutionContext,
) (*sqlResult, statementOutcome) {
	name, values, ok := parseExecute(query)
	if !ok || name == "" {
		return nil, stmtFail(athenaErrTypeSyntax, "line 1:1: could not parse EXECUTE statement")
	}

	ps, err := b.GetPreparedStatement(name, workGroup)
	if err != nil {
		return nil, stmtFail(athenaErrTypeEntityMiss, "Prepared statement does not exist: %s", name)
	}

	substituted, expected, found, subOK := substitutePlaceholders(ps.QueryStatement, values)
	if !subOK {
		return nil, stmtFail(athenaErrTypeSyntax,
			"line 1:1: Incorrect number of parameters: expected %d but found %d", expected, found)
	}

	if hasKeyword(strings.ToUpper(strings.TrimSpace(substituted)), "EXECUTE") {
		return nil, stmtFail(athenaErrTypeSyntax,
			"line 1:1: a prepared statement cannot EXECUTE another prepared statement")
	}

	return b.executeStatement(substituted, workGroup, ctx)
}

// parseExecute splits an EXECUTE statement into the prepared statement name and
// the (possibly empty) list of USING values. USING is optional when the
// prepared statement has no parameters, e.g. "EXECUTE my_select1".
func parseExecute(query string) (string, []string, bool) {
	rest, matched := stripLeadingKeyword(query, []string{"EXECUTE"})
	if !matched {
		return "", nil, false
	}

	rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")

	nameEnd := strings.IndexAny(rest, " \t\n")
	if nameEnd < 0 {
		return unquoteIdent(rest), nil, rest != ""
	}

	name := unquoteIdent(rest[:nameEnd])

	tail := strings.TrimSpace(rest[nameEnd:])
	if !strings.HasPrefix(strings.ToUpper(tail), "USING") {
		return "", nil, false
	}

	payload := strings.TrimSpace(tail[len("USING"):])

	return name, splitUsingValues(payload), name != ""
}

// splitUsingValues splits an EXECUTE ... USING payload on top-level commas,
// respecting parenthesised expressions and single-quoted string literals (a
// doubled quote is the escaped quote) so a comma inside a literal does not
// split the value.
func splitUsingValues(s string) []string {
	var (
		parts    []string
		buf      strings.Builder
		depth    int
		inSingle bool
	)

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case inSingle:
			buf.WriteByte(c)

			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					buf.WriteByte(s[i+1])
					i++

					continue
				}

				inSingle = false
			}
		case c == '\'':
			inSingle = true

			buf.WriteByte(c)
		case c == '(':
			depth++

			buf.WriteByte(c)
		case c == ')':
			if depth > 0 {
				depth--
			}

			buf.WriteByte(c)
		case c == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(buf.String()))
			buf.Reset()
		default:
			buf.WriteByte(c)
		}
	}

	if strings.TrimSpace(buf.String()) != "" {
		parts = append(parts, strings.TrimSpace(buf.String()))
	}

	return parts
}

// countPlaceholders counts '?' parameter markers in query, skipping any '?'
// that appears inside a single-quoted string literal (a doubled quote is the
// escaped quote). Athena's docs state question-mark placeholders can never
// themselves be quoted, so a quoted '?' is ordinary string content, not a
// parameter.
func countPlaceholders(query string) int {
	var (
		n        int
		inSingle bool
	)

	for i := 0; i < len(query); i++ {
		c := query[i]

		switch {
		case inSingle:
			if c == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++

					continue
				}

				inSingle = false
			}
		case c == '\'':
			inSingle = true
		case c == '?':
			n++
		}
	}

	return n
}

// substitutePlaceholders replaces each unquoted '?' in query, in order, with the
// corresponding value from vals. vals are copied in verbatim: the EXECUTE ...
// USING clause already carries correctly quoted/escaped SQL literals, so no
// further escaping is applied here — doing so would risk double-escaping.
// Returns ok=false with the expected/found counts on a parameter-count
// mismatch, without performing any substitution.
func substitutePlaceholders(query string, vals []string) (string, int, int, bool) {
	expected := countPlaceholders(query)
	found := len(vals)

	if expected != found {
		return "", expected, found, false
	}

	var (
		out      strings.Builder
		inSingle bool
		vi       int
	)

	for i := 0; i < len(query); i++ {
		c := query[i]

		switch {
		case inSingle:
			out.WriteByte(c)

			if c == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					out.WriteByte(query[i+1])
					i++

					continue
				}

				inSingle = false
			}
		case c == '\'':
			inSingle = true

			out.WriteByte(c)
		case c == '?':
			out.WriteString(vals[vi])
			vi++
		default:
			out.WriteByte(c)
		}
	}

	return out.String(), expected, found, true
}
