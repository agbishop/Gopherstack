package redshiftdata

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ExecuteStatement creates and immediately completes a SQL statement.
//
// Database is NOT validated as required here: unlike ListDatabasesInput/
// ListSchemasInput/ListTablesInput/DescribeTableInput (whose Database member
// carries "This member is required" and a matching client-side
// smithy.NewErrParamRequired check in validators.go), ExecuteStatementInput's
// Database doc comment says only "required when authenticating using either
// Secrets Manager or temporary credentials" -- a conditional, not a hard
// trait -- and validateOpExecuteStatementInput has no Database check at all.
func (b *InMemoryBackend) ExecuteStatement(
	ctx context.Context,
	sql, clusterIdentifier, workgroupName, database, dbUser, secretARN, statementName string,
	withEvent bool, resultFormat string,
	parameters []SQLParameter,
	sessionID string,
) (*Statement, error) {
	if sql == "" {
		return nil, fmt.Errorf("%w: Sql is required", ErrValidation)
	}
	if err := ValidateConnectionTarget(clusterIdentifier, workgroupName); err != nil {
		return nil, err
	}

	resultFormat, err := requestedResultFormat(resultFormat)
	if err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("ExecuteStatement")
	defer b.mu.Unlock()

	hasResultSet := sqlHasResultSet(sql)

	now := time.Now()
	stmt := &Statement{
		ID:                uuid.NewString(),
		QueryString:       sql,
		ClusterIdentifier: clusterIdentifier,
		WorkgroupName:     workgroupName,
		Database:          database,
		DBUser:            dbUser,
		SecretARN:         secretARN,
		StatementName:     statementName,
		ResultFormat:      resultFormat,
		Parameters:        parameters,
		SessionID:         sessionID,
		Status:            statusFinished,
		HasResultSet:      hasResultSet,
		IsBatchStatement:  false,
		WithEvent:         withEvent,
		CreatedAt:         now,
		UpdatedAt:         now,
		// Simulated instant execution: 1 ms so the UI always displays a
		// human-readable duration rather than showing "0ms" which could be
		// mistaken for a failed or uninitialized measurement.
		DurationMs: mockStatementDurationMs,
		ResultRows: demoResultRows,
		ResultSize: demoResultSize,
	}
	b.storeFor(region).addStatement(stmt)

	return cloneStatement(stmt), nil
}

// BatchExecuteStatement creates and immediately completes a batch SQL statement.
func (b *InMemoryBackend) BatchExecuteStatement(
	ctx context.Context,
	sqls []string, clusterIdentifier, workgroupName, database, dbUser, secretARN, statementName string,
	withEvent bool, resultFormat string,
	parameters []SQLParameter,
	sessionID string,
) (*Statement, error) {
	if len(sqls) == 0 {
		return nil, fmt.Errorf("%w: Sqls is required", ErrValidation)
	}

	for i, sql := range sqls {
		if sql == "" {
			return nil, fmt.Errorf("%w: Sqls[%d] must not be empty", ErrValidation, i)
		}
	}
	if err := ValidateConnectionTarget(clusterIdentifier, workgroupName); err != nil {
		return nil, err
	}

	// Database is not validated as required -- see ExecuteStatement's doc
	// comment; BatchExecuteStatementInput.Database has the identical
	// conditional (not hard-required) doc comment and validator absence.
	resultFormat, err := requestedResultFormat(resultFormat)
	if err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("BatchExecuteStatement")
	defer b.mu.Unlock()

	now := time.Now()

	subs := make([]SubStatementData, len(sqls))
	for i, sql := range sqls {
		subs[i] = SubStatementData{
			ID:           uuid.NewString(),
			CreatedAt:    now,
			UpdatedAt:    now,
			QueryString:  sql,
			Status:       statusFinished,
			HasResultSet: false,
			DurationMs:   1,
		}
	}

	stmt := &Statement{
		ID:                uuid.NewString(),
		QueryString:       sqls[0], // AWS sets QueryString to the first SQL in the batch.
		QueryStrings:      append([]string(nil), sqls...),
		SubStatements:     subs,
		ClusterIdentifier: clusterIdentifier,
		WorkgroupName:     workgroupName,
		Database:          database,
		DBUser:            dbUser,
		SecretARN:         secretARN,
		StatementName:     statementName,
		ResultFormat:      resultFormat,
		Parameters:        parameters,
		SessionID:         sessionID,
		Status:            statusFinished,
		HasResultSet:      false,
		IsBatchStatement:  true,
		WithEvent:         withEvent,
		CreatedAt:         now,
		UpdatedAt:         now,
		DurationMs:        1,
	}
	b.storeFor(region).addStatement(stmt)

	return cloneStatement(stmt), nil
}

// DescribeStatement returns the details of a statement by ID.
func (b *InMemoryBackend) DescribeStatement(ctx context.Context, id string) (*Statement, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("DescribeStatement")
	defer b.mu.RUnlock()

	store := b.storeForRead(region)
	if store == nil {
		return nil, fmt.Errorf("%w: statement %s not found", ErrNotFound, id)
	}

	stmt, ok := store.statements[id]

	if !ok {
		return nil, fmt.Errorf("%w: statement %s not found", ErrNotFound, id)
	}

	return cloneStatement(stmt), nil
}

// CancelStatement marks a statement as aborted.
func (b *InMemoryBackend) CancelStatement(ctx context.Context, id string) error {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("CancelStatement")
	defer b.mu.Unlock()

	store := b.storeFor(region)
	stmt, ok := store.statements[id]

	if !ok {
		return fmt.Errorf("%w: statement %s not found", ErrNotFound, id)
	}

	if stmt.Status == statusFinished || stmt.Status == statusFailed || stmt.Status == statusAborted {
		return fmt.Errorf("%w: statement %s is already in terminal state %s", ErrTerminalState, id, stmt.Status)
	}

	now := time.Now()
	stmt.Status = statusAborted
	stmt.UpdatedAt = now
	stmt.DurationMs = now.Sub(stmt.CreatedAt).Milliseconds()

	return nil
}

// statementMatchesFilter reports whether stmt satisfies every set field of filter.
func statementMatchesFilter(stmt *Statement, filter ListStatementsFilter) bool {
	if filter.ClusterIdentifier != "" && stmt.ClusterIdentifier != filter.ClusterIdentifier {
		return false
	}

	if filter.WorkgroupName != "" && stmt.WorkgroupName != filter.WorkgroupName {
		return false
	}

	if filter.Database != "" && stmt.Database != filter.Database {
		return false
	}

	if filter.StatementName != "" && !strings.HasPrefix(stmt.StatementName, filter.StatementName) {
		return false
	}

	return matchesStatementStatus(stmt.Status, filter.Status)
}

// ListStatements returns statements sorted by creation time (newest first).
// An omitted Status matches AWS by returning only finished statements.
// Returns the page slice and a next-token string (non-empty when more pages exist).
func (b *InMemoryBackend) ListStatements(
	ctx context.Context,
	filter ListStatementsFilter,
) ([]*Statement, string, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("ListStatements")
	defer b.mu.RUnlock()

	store := b.storeForRead(region)
	if store == nil {
		return nil, "", nil
	}

	result := make([]*Statement, 0, len(store.statements))

	for _, stmt := range store.statements {
		if statementMatchesFilter(stmt, filter) {
			result = append(result, cloneStatement(stmt))
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}

		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	start, err := statementPageStart(result, filter.NextToken)
	if err != nil {
		return nil, "", err
	}

	result = result[start:]

	limit := filter.MaxResults
	if limit <= 0 {
		limit = defaultListStatementsResults
	}

	if len(result) <= limit {
		return result, "", nil
	}

	return result[:limit], result[limit].ID, nil
}

func requestedResultFormat(format string) (string, error) {
	if format == "" {
		return resultFormatJSON, nil
	}

	switch format {
	case resultFormatJSON, resultFormatCSV:
		return format, nil
	default:
		return "", fmt.Errorf("%w: ResultFormat must be JSON or CSV", ErrValidation)
	}
}

func statementResultFormat(stmt *Statement) string {
	if stmt.ResultFormat == "" {
		return resultFormatJSON
	}

	return stmt.ResultFormat
}

func matchesStatementStatus(actual, requested string) bool {
	if requested == "" {
		return actual == statusFinished
	}

	return requested == statusAll || actual == requested
}

func statementPageStart(statements []*Statement, nextToken string) (int, error) {
	if nextToken == "" {
		return 0, nil
	}

	for i, stmt := range statements {
		if stmt.ID == nextToken {
			return i, nil
		}
	}

	return 0, fmt.Errorf("%w: invalid NextToken", ErrValidation)
}

// EvictExpiredStatements removes terminal statements whose UpdatedAt is older
// than the given cutoff across all regions. Returns the number of evicted statements.
// Only terminal states (FINISHED, FAILED, ABORTED) are eligible for eviction.
func (b *InMemoryBackend) EvictExpiredStatements(cutoff time.Time) int {
	b.mu.Lock("EvictExpiredStatements")
	defer b.mu.Unlock()

	total := 0

	for _, store := range b.stores {
		var toDelete []string

		for id, stmt := range store.statements {
			terminal := stmt.Status == statusFinished ||
				stmt.Status == statusFailed ||
				stmt.Status == statusAborted
			if terminal && stmt.UpdatedAt.Before(cutoff) {
				toDelete = append(toDelete, id)
			}
		}

		for _, id := range toDelete {
			delete(store.statements, id)
		}

		if len(toDelete) > 0 {
			store.compactRingBuffer()
		}

		total += len(toDelete)
	}

	return total
}
