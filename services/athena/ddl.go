package athena

import (
	"fmt"
	"slices"
	"strings"
)

// statementOutcome describes the side effects of executing a non-result
// statement (DDL/DML) or the reason a statement failed. A zero statementOutcome
// means "succeeded with no catalog effect".
type statementOutcome struct {
	// reason is the human-readable failure reason (StateChangeReason) when
	// failed is true.
	reason string
	// rowsAffected is the number of rows written by a successful DML statement
	// (INSERT / CTAS). It is informational; Athena reports affected rows via the
	// data manifest rather than GetQueryResults.
	rowsAffected int64
	// errorType is the numeric AthenaError.ErrorType surfaced on failure.
	errorType int32
	// failed reports whether the statement failed. A failed statement transitions
	// the query execution to FAILED with an AthenaError.
	failed bool
}

// Athena AthenaError.ErrorType values used for the failure reasons this engine
// can produce. These fall in AWS's documented "user" band (query is malformed or
// references a missing object).
const (
	athenaErrTypeSyntax      int32 = 100
	athenaErrTypeEntityFound int32 = 130 // object already exists
	athenaErrTypeEntityMiss  int32 = 131 // object not found
)

// stmtOK is the success outcome with no catalog effect.
func stmtOK() statementOutcome { return statementOutcome{} }

// stmtWrote reports a successful DML statement that wrote n rows.
func stmtWrote(n int64) statementOutcome { return statementOutcome{rowsAffected: n} }

// stmtFail builds a failure outcome with the given AthenaError type and reason.
func stmtFail(errType int32, format string, args ...any) statementOutcome {
	return statementOutcome{
		failed:    true,
		errorType: errType,
		reason:    fmt.Sprintf(format, args...),
	}
}

// executeStatement classifies query and executes it. SELECT queries return a
// result set with no catalog effect. DDL statements (CREATE/DROP DATABASE|TABLE)
// mutate the emulated Glue catalog. DML statements (INSERT, CREATE TABLE AS
// SELECT) mutate table data. Recognised-but-unemulated statements (SHOW,
// DESCRIBE, ALTER, MSCK, ...) succeed as no-ops. Anything else fails the
// execution with an AWS-shaped AthenaError instead of the previous silent empty
// success.
//
// executeStatement must be called without holding b.mu; the DDL/DML helpers take
// the write lock themselves, and the SELECT path takes its own read lock.
func (b *InMemoryBackend) executeStatement(
	query, workGroup string, ctx QueryExecutionContext,
) (*sqlResult, statementOutcome) {
	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)

	switch {
	case hasKeyword(upper, "SELECT"), hasKeyword(upper, "WITH"):
		return b.executeSQL(query, ctx), stmtOK()
	case hasKeyword(upper, "CREATE DATABASE"), hasKeyword(upper, "CREATE SCHEMA"):
		return &sqlResult{}, b.execCreateDatabase(trimmed, ctx)
	case hasKeyword(upper, "DROP DATABASE"), hasKeyword(upper, "DROP SCHEMA"):
		return &sqlResult{}, b.execDropDatabase(trimmed, ctx)
	case isCTAS(upper):
		return b.execCTAS(trimmed, ctx)
	case hasKeyword(upper, "CREATE TABLE"), hasKeyword(upper, "CREATE EXTERNAL TABLE"):
		return &sqlResult{}, b.execCreateTable(trimmed, ctx)
	case hasKeyword(upper, "DROP TABLE"), hasKeyword(upper, "DROP VIEW"):
		return &sqlResult{}, b.execDropTable(trimmed, ctx)
	case hasKeyword(upper, "INSERT INTO"):
		return b.execInsert(trimmed, ctx)
	case hasKeyword(upper, "EXECUTE"):
		return b.execExecute(trimmed, workGroup, ctx)
	case isRecognisedNoOp(upper):
		// Valid Athena statement types we accept but do not fully emulate. AWS
		// runs them successfully; returning an empty result set is the closest
		// faithful behaviour without a full engine.
		return &sqlResult{}, stmtOK()
	default:
		return nil, stmtFail(athenaErrTypeSyntax,
			"line 1:1: mismatched input %q: unsupported statement", firstToken(trimmed))
	}
}

// hasKeyword reports whether the uppercased query begins with keyword, treating
// keyword as a whitespace-delimited prefix (so "CREATE TABLE" does not match
// "CREATE TABLES...").
func hasKeyword(upper, keyword string) bool {
	if !strings.HasPrefix(upper, keyword) {
		return false
	}

	rest := upper[len(keyword):]

	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n' || rest[0] == '('
}

// isCTAS reports whether the statement is a CREATE TABLE ... AS SELECT.
func isCTAS(upper string) bool {
	if !hasKeyword(upper, "CREATE TABLE") && !hasKeyword(upper, "CREATE EXTERNAL TABLE") {
		return false
	}

	return strings.Contains(upper, " AS SELECT") || strings.Contains(upper, " AS WITH") ||
		strings.HasSuffix(strings.TrimSpace(upper), " AS") ||
		containsTopLevelAs(upper)
}

// containsTopLevelAs reports whether the statement contains an " AS " token that
// is not inside a parenthesised column list — the CTAS marker.
func containsTopLevelAs(upper string) bool {
	depth := 0
	for i := 0; i+4 <= len(upper); i++ {
		switch upper[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}

		if depth == 0 && upper[i:i+4] == " AS " {
			return true
		}
	}

	return false
}

// isRecognisedNoOp reports whether the uppercased statement is a supported
// Athena statement that this engine treats as a successful no-op. These are
// statement leaders Athena runs successfully but that this emulator does not
// fully model.
func isRecognisedNoOp(upper string) bool {
	prefixes := []string{
		"ALTER", "MSCK", "SHOW", "DESCRIBE", "DESC", "EXPLAIN",
		"USE", "SET", "RESET", "UNLOAD", "CALL", "GRANT", "REVOKE",
		"COMMENT", "PREPARE", "DEALLOCATE", "REFRESH",
	}

	for _, p := range prefixes {
		if hasKeyword(upper, p) {
			return true
		}
	}

	return false
}

// firstToken returns the first whitespace-delimited token of s (for diagnostics).
func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return s[:i]
	}

	return s
}

// --- CREATE / DROP DATABASE ---

func (b *InMemoryBackend) execCreateDatabase(query string, ctx QueryExecutionContext) statementOutcome {
	name, ifNotExists := parseDatabaseTarget(query, []string{"CREATE DATABASE", "CREATE SCHEMA"})
	if name == "" {
		return stmtFail(athenaErrTypeSyntax, "line 1:1: missing database name in CREATE DATABASE")
	}

	catalog, database := resolveDatabaseRef(name, ctx)

	b.mu.Lock("execCreateDatabase")
	defer b.mu.Unlock()

	key := databaseKey(catalog, database)
	if b.databases.Has(key) {
		if ifNotExists {
			return stmtOK()
		}

		return stmtFail(athenaErrTypeEntityFound, "Database %s already exists", database)
	}

	b.databases.Put(&Database{Catalog: catalog, Name: database})

	return stmtOK()
}

func (b *InMemoryBackend) execDropDatabase(query string, ctx QueryExecutionContext) statementOutcome {
	name, ifExists := parseDatabaseTarget(query, []string{"DROP DATABASE", "DROP SCHEMA"})
	if name == "" {
		return stmtFail(athenaErrTypeSyntax, "line 1:1: missing database name in DROP DATABASE")
	}

	catalog, database := resolveDatabaseRef(name, ctx)

	b.mu.Lock("execDropDatabase")
	defer b.mu.Unlock()

	key := databaseKey(catalog, database)
	if !b.databases.Has(key) {
		if ifExists {
			return stmtOK()
		}

		return stmtFail(athenaErrTypeEntityMiss, "Database does not exist: %s", database)
	}

	b.databases.Delete(key)

	// Drop the database's tables and their data. tablesByDatabase.Get returns
	// a slice owned by the index that Table.Delete mutates in place, so it
	// must be cloned before iterating and deleting from it.
	for _, tm := range slices.Clone(b.tablesByDatabase.Get(key)) {
		b.tables.Delete(tableMetadataKeyFn(tm))
	}

	prefix := catalog + "/" + database

	for tdKey := range b.tableData {
		if strings.HasPrefix(tdKey, prefix+"/") {
			delete(b.tableData, tdKey)
		}
	}

	return stmtOK()
}

// --- CREATE TABLE (with explicit columns) ---

func (b *InMemoryBackend) execCreateTable(query string, ctx QueryExecutionContext) statementOutcome {
	name, cols, ifNotExists, ok := parseCreateTable(query)
	if !ok || name == "" {
		return stmtFail(athenaErrTypeSyntax, "line 1:1: could not parse CREATE TABLE statement")
	}

	catalog, database, table := resolveTableRef(name, ctx)

	b.mu.Lock("execCreateTable")
	defer b.mu.Unlock()

	key := tableMetadataKey(catalog, database, table)

	if b.tables.Has(key) {
		if ifNotExists {
			return stmtOK()
		}

		return stmtFail(athenaErrTypeEntityFound, "Table %s already exists", table)
	}

	b.ensureDatabaseLocked(catalog, database)

	b.tables.Put(&TableMetadata{
		Catalog:   catalog,
		Database:  database,
		Name:      table,
		TableType: tableTypeExternal,
		Columns:   cols,
	})

	return stmtOK()
}

func (b *InMemoryBackend) execDropTable(query string, ctx QueryExecutionContext) statementOutcome {
	name, ifExists := parseDropTable(query)
	if name == "" {
		return stmtFail(athenaErrTypeSyntax, "line 1:1: missing table name in DROP TABLE")
	}

	catalog, database, table := resolveTableRef(name, ctx)

	b.mu.Lock("execDropTable")
	defer b.mu.Unlock()

	key := tableMetadataKey(catalog, database, table)

	if !b.tables.Has(key) {
		if ifExists {
			return stmtOK()
		}
		// Hive/Athena DROP TABLE without IF EXISTS on a missing table fails.
		return stmtFail(athenaErrTypeEntityMiss, "Table does not exist: %s", table)
	}

	b.tables.Delete(key)
	delete(b.tableData, catalog+"/"+database+"/"+table)

	return stmtOK()
}

// --- CREATE TABLE AS SELECT (CTAS) ---

func (b *InMemoryBackend) execCTAS(query string, ctx QueryExecutionContext) (*sqlResult, statementOutcome) {
	name, selectSQL, ok := parseCTAS(query)
	if !ok || name == "" || selectSQL == "" {
		return nil, stmtFail(athenaErrTypeSyntax, "line 1:1: could not parse CREATE TABLE AS SELECT")
	}

	// Run the SELECT first (its own read lock) so we do not hold the write lock
	// across the projection.
	selected := b.executeSQL(selectSQL, ctx)

	catalog, database, table := resolveTableRef(name, ctx)

	b.mu.Lock("execCTAS")
	defer b.mu.Unlock()

	key := tableMetadataKey(catalog, database, table)
	if b.tables.Has(key) {
		return nil, stmtFail(athenaErrTypeEntityFound, "Table %s already exists", table)
	}

	b.ensureDatabaseLocked(catalog, database)

	cols := make([]Column, 0, len(selected.columns))
	for _, c := range selected.columns {
		typ := c.typ
		if typ == "" {
			typ = columnTypeString
		}

		cols = append(cols, Column{Name: c.name, Type: typ})
	}

	b.tables.Put(&TableMetadata{
		Catalog:   catalog,
		Database:  database,
		Name:      table,
		TableType: tableTypeExternal,
		Columns:   cols,
	})

	// Materialise the SELECT rows into the new table's data so a subsequent
	// SELECT observes them.
	dataKey := catalog + "/" + database + "/" + table
	for _, row := range selected.rows {
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if i < len(row) {
				m[c.Name] = row[i]
			}
		}

		b.tableData[dataKey] = append(b.tableData[dataKey], m)
	}

	// CTAS returns no result set from GetQueryResults.
	return &sqlResult{}, stmtWrote(int64(len(selected.rows)))
}

// --- INSERT ---

func (b *InMemoryBackend) execInsert(query string, ctx QueryExecutionContext) (*sqlResult, statementOutcome) {
	target, cols, valuesPart, selectSQL, ok := parseInsert(query)
	if !ok || target == "" {
		return nil, stmtFail(athenaErrTypeSyntax, "line 1:1: could not parse INSERT statement")
	}

	catalog, database, table := resolveTableRef(target, ctx)
	dataKey := catalog + "/" + database + "/" + table
	key := tableMetadataKey(catalog, database, table)

	// Source rows as positional string values, from either VALUES or SELECT.
	var srcRows [][]string

	if selectSQL != "" {
		srcRows = b.executeSQL(selectSQL, ctx).rows
	} else {
		tuples, perr := parseValuesTuples(valuesPart)
		if perr != nil {
			return nil, stmtFail(athenaErrTypeSyntax, "line 1:1: %s", perr.Error())
		}

		srcRows = tuples
	}

	b.mu.Lock("execInsert")
	defer b.mu.Unlock()

	tm, exists := b.tables.Get(key)
	if !exists {
		return nil, stmtFail(athenaErrTypeEntityMiss, "Table does not exist: %s", table)
	}

	// Determine the destination column names: an explicit column list if the
	// INSERT provided one, otherwise the table's declared column order.
	destCols := columnNames(tm.Columns)
	if len(cols) > 0 {
		destCols = cols
	}

	rows := make([]map[string]any, 0, len(srcRows))

	for _, src := range srcRows {
		m := make(map[string]any, len(destCols))
		for i, name := range destCols {
			if i < len(src) {
				m[name] = src[i]
			}
		}

		rows = append(rows, m)
	}

	b.tableData[dataKey] = append(b.tableData[dataKey], rows...)

	// INSERT returns an empty result set from GetQueryResults, matching AWS.
	return &sqlResult{}, stmtWrote(int64(len(rows)))
}

// ensureDatabaseLocked creates the database entry if it does not yet exist.
// Callers must hold b.mu.
func (b *InMemoryBackend) ensureDatabaseLocked(catalog, database string) {
	key := databaseKey(catalog, database)
	if !b.databases.Has(key) {
		b.databases.Put(&Database{Catalog: catalog, Name: database})
	}
}
