package rdsdata

import (
	"context"
	"fmt"
)

// ExecuteStatement executes a SQL statement and returns its result set. Named
// parameters (e.g. ":id") are bound when supplied. The returned generated
// fields are non-empty only for an INSERT into a table with a single INTEGER
// PRIMARY KEY column -- see generatedFieldsFor in engine.go.
func (b *InMemoryBackend) ExecuteStatement(
	ctx context.Context,
	resourceARN, sql, transactionID string,
	parameters ...SQLParameter,
) ([][]Field, []ColumnMetadata, int64, []Field, error) {
	b.mu.Lock("ExecuteStatement")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	if transactionID != "" {
		if !b.transactionsStore(region).Has(transactionID) {
			return nil, nil, 0, nil, fmt.Errorf(
				"%w: transaction %s not found",
				ErrTransactionNotFound,
				transactionID,
			)
		}

		b.touchTransactionLocked(region, transactionID)
	}

	b.appendStatementLocked(region, resourceARN, sql, transactionID)

	// Execute against the real in-memory SQL engine. A genuine result set is
	// returned for well-formed statements; anything the engine rejects (for
	// example DML against a table the caller never created) degrades to the
	// historical empty-success envelope rather than surfacing an error.
	records, columns, updated, generated, err := b.engine.execute(
		ctx, region, resourceARN, sql, transactionID, parameters)
	if err != nil {
		return [][]Field{}, []ColumnMetadata{}, 0, []Field{}, nil
	}

	if generated == nil {
		generated = []Field{}
	}

	return records, columns, updated, generated, nil
}

// BatchExecuteStatement executes a batch of SQL statements and returns results for each.
func (b *InMemoryBackend) BatchExecuteStatement(
	ctx context.Context,
	resourceARN, sql, transactionID string,
	parameterSets [][]SQLParameter,
) ([]UpdateResult, error) {
	b.mu.Lock("BatchExecuteStatement")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	if transactionID != "" {
		if !b.transactionsStore(region).Has(transactionID) {
			return nil, fmt.Errorf(
				"%w: transaction %s not found",
				ErrTransactionNotFound,
				transactionID,
			)
		}

		b.touchTransactionLocked(region, transactionID)
	}

	b.appendStatementLocked(region, resourceARN, sql, transactionID)

	if len(parameterSets) == 0 {
		// A parameterless batch still executes the statement once so DDL such
		// as CREATE TABLE takes effect; the engine error is ignored to keep the
		// historical lenient behaviour.
		_, _, _, _, _ = b.engine.execute(ctx, region, resourceARN, sql, transactionID, nil)

		return []UpdateResult{}, nil
	}

	results := make([]UpdateResult, len(parameterSets))

	for i, params := range parameterSets {
		// Run each parameter set so inserts/updates actually land in the
		// engine; generatedFields carries the same rowid-alias detection as
		// ExecuteStatement (see generatedFieldsFor in engine.go).
		_, _, _, generated, _ := b.engine.execute(ctx, region, resourceARN, sql, transactionID, params)
		if generated == nil {
			generated = []Field{}
		}

		results[i] = UpdateResult{GeneratedFields: generated}
	}

	return results, nil
}

// ListExecutedStatements returns a copy of all executed statements for the request's region.
func (b *InMemoryBackend) ListExecutedStatements(ctx context.Context) []ExecutedStatement {
	b.mu.RLock("ListExecutedStatements")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	stmts := b.executedStatements[region]
	result := make([]ExecutedStatement, len(stmts))
	copy(result, stmts)

	return result
}
