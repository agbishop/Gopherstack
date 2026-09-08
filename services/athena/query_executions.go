package athena

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// athenaErrorCategoryUser is the AthenaError.ErrorCategory value AWS uses for
// failures caused by the submitted query (as opposed to platform errors).
const athenaErrorCategoryUser = 2

// StartQueryExecution records a new query execution and returns its ID.
func (b *InMemoryBackend) StartQueryExecution(
	query, workGroup string,
	ctx QueryExecutionContext,
	rc ResultConfiguration,
	execParams []string,
	reuseCfg *ResultReuseConfiguration,
) (string, error) {
	if query == "" {
		return "", fmt.Errorf("%w: QueryString is required", ErrValidation)
	}

	if workGroup == "" {
		workGroup = defaultWorkGroup
	}

	var id string

	var startErr error

	func() {
		b.mu.Lock("StartQueryExecution")
		defer b.mu.Unlock()

		wg, ok := b.workGroups.Get(workGroup)
		if !ok {
			startErr = fmt.Errorf("%w: workgroup %q not found", ErrNotFound, workGroup)

			return
		}

		if wg.State == "DISABLED" {
			startErr = fmt.Errorf("%w: workgroup %q is disabled", ErrValidation, workGroup)

			return
		}

		// EnforceWorkGroupConfiguration: workgroup settings override per-query settings.
		rc = applyWorkGroupResultConfig(wg, rc)

		reused := b.hasReusableResult(workGroup, query, reuseCfg)

		id = randomID()
		qe := newQueryExecution(id, query, workGroup, ctx, rc, execParams, reuseCfg, reused)

		b.queryExecutions.Put(qe)
	}()

	if startErr != nil {
		return "", startErr
	}

	// Execute the statement outside the write-lock. executeStatement acquires
	// its own locks: an RLock for SELECT projection and a write-lock for DDL/DML
	// catalog mutations. It returns the (possibly nil) result set plus an outcome
	// describing catalog effects or a failure reason.
	result, outcome := b.executeStatement(query, workGroup, ctx)

	b.mu.Lock("StartQueryExecution-storeResult")
	b.finalizeExecution(id, result, outcome)
	b.mu.Unlock()

	return id, nil
}

// applyWorkGroupResultConfig overrides rc with the workgroup's result
// configuration when EnforceWorkGroupConfiguration is set.
func applyWorkGroupResultConfig(wg *WorkGroup, rc ResultConfiguration) ResultConfiguration {
	if !wg.Configuration.EnforceWGCfg {
		return rc
	}

	wgRC := wg.Configuration.ResultConfiguration
	if wgRC.OutputLocation != "" {
		rc.OutputLocation = wgRC.OutputLocation
	}

	if wgRC.EncryptionConfiguration.EncryptionOption != "" {
		rc.EncryptionConfiguration = wgRC.EncryptionConfiguration
	}

	return rc
}

// hasReusableResult reports whether a recent succeeded execution of query in
// workGroup is fresh enough, per reuseCfg, to mark the new execution as having
// reused its previous result. Callers must hold b.mu.
func (b *InMemoryBackend) hasReusableResult(
	workGroup, query string,
	reuseCfg *ResultReuseConfiguration,
) bool {
	if reuseCfg == nil ||
		reuseCfg.ResultReuseByAgeConfiguration == nil ||
		!reuseCfg.ResultReuseByAgeConfiguration.Enabled {
		return false
	}

	maxAge := reuseCfg.ResultReuseByAgeConfiguration.MaxAgeInMinutes
	cutoff := float64(
		time.Now().Add(-time.Duration(maxAge)*time.Minute).UnixMilli(),
	) / millisToSeconds

	for _, prev := range b.queryExecutionsByWorkGroup.Get(workGroup) {
		if prev.Query == query &&
			prev.Status.State == stateSucceeded &&
			prev.Status.CompletionDateTime >= cutoff &&
			(prev.Statistics.ResultReuseInformation == nil ||
				!prev.Statistics.ResultReuseInformation.ReusedPreviousResult) {
			return true
		}
	}

	return false
}

// newQueryExecution builds the initial QueryExecution record for a just-started query.
func newQueryExecution(
	id, query, workGroup string,
	ctx QueryExecutionContext,
	rc ResultConfiguration,
	execParams []string,
	reuseCfg *ResultReuseConfiguration,
	reused bool,
) *QueryExecution {
	now := float64(time.Now().UnixMilli()) / millisToSeconds

	const mockEngineMs int64 = 100

	return &QueryExecution{
		QueryExecutionID:         id,
		Query:                    query,
		ResultConfiguration:      rc,
		QueryExecutionContext:    ctx,
		ResultReuseConfiguration: reuseCfg,
		WorkGroup:                workGroup,
		StatementType:            inferStatementType(query),
		ExecutionParameters:      execParams,
		EngineVersion: &EngineVersion{
			SelectedEngineVersion:  stateAuto,
			EffectiveEngineVersion: athenaEngineV3,
		},
		Status: QueryExecutionStatus{
			State:              stateSucceeded,
			SubmissionDateTime: now,
			CompletionDateTime: now,
		},
		Statistics: QueryExecutionStatistics{
			EngineExecutionTimeInMillis:   mockEngineMs,
			TotalExecutionTimeInMillis:    mockEngineMs,
			ServiceProcessingTimeInMillis: 1,
			DataScannedInBytes:            0,
			ResultReuseInformation:        &ResultReuseInformation{ReusedPreviousResult: reused},
		},
	}
}

// finalizeExecution stores the result of a query execution and, when the
// statement failed, transitions the execution to FAILED with an AWS-shaped
// AthenaError. Failed queries carry no result set. Callers must hold b.mu.
func (b *InMemoryBackend) finalizeExecution(id string, result *sqlResult, outcome statementOutcome) {
	qe, ok := b.queryExecutions.Get(id)
	if !ok {
		// Evicted between dispatch and finalize; nothing to store.
		return
	}

	if outcome.failed {
		now := float64(time.Now().UnixMilli()) / millisToSeconds
		qe.Status.State = stateFailed
		qe.Status.StateChangeReason = outcome.reason
		qe.Status.CompletionDateTime = now
		qe.Status.AthenaError = &QueryExecutionError{
			ErrorCategory: athenaErrorCategoryUser,
			ErrorType:     outcome.errorType,
			ErrorMessage:  outcome.reason,
		}

		delete(b.queryResults, id)

		return
	}

	b.queryResults[id] = result

	b.writeResultObject(id, qe.ResultConfiguration, result)
}

// inferStatementType returns the Athena StatementType for a query string.
func inferStatementType(query string) string {
	q := strings.ToUpper(strings.TrimSpace(query))
	switch {
	case strings.HasPrefix(q, "SELECT"),
		strings.HasPrefix(q, "INSERT"),
		strings.HasPrefix(q, "CREATE TABLE AS"),
		strings.HasPrefix(q, "UNLOAD"):
		return "DML"
	case strings.HasPrefix(q, "CREATE"),
		strings.HasPrefix(q, "DROP"),
		strings.HasPrefix(q, "ALTER"),
		strings.HasPrefix(q, "MSCK"):
		return "DDL"
	default:
		return "UTILITY"
	}
}

// GetQueryExecution retrieves a query execution by ID.
func (b *InMemoryBackend) GetQueryExecution(id string) (*QueryExecution, error) {
	b.mu.RLock("GetQueryExecution")
	defer b.mu.RUnlock()

	qe, ok := b.queryExecutions.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: query execution %q not found", ErrNotFound, id)
	}

	cp := *qe

	return &cp, nil
}

// ListQueryExecutions returns query execution IDs, optionally filtered by workgroup.
func (b *InMemoryBackend) ListQueryExecutions(workGroup string) ([]string, error) {
	b.mu.RLock("ListQueryExecutions")
	defer b.mu.RUnlock()

	var matches []*QueryExecution
	if workGroup == "" {
		matches = b.queryExecutions.All()
	} else {
		matches = b.queryExecutionsByWorkGroup.Get(workGroup)
	}

	ids := make([]string, 0, len(matches))
	for _, qe := range matches {
		ids = append(ids, qe.QueryExecutionID)
	}

	sort.Strings(ids)

	return ids, nil
}

// isTerminalState reports whether a query execution state is terminal (cannot be stopped).
func isTerminalState(state string) bool {
	switch state {
	case stateSucceeded, stateFailed, stateCancelled:
		return true
	default:
		return false
	}
}

// StopQueryExecution marks a query execution as cancelled.
func (b *InMemoryBackend) StopQueryExecution(id string) error {
	b.mu.Lock("StopQueryExecution")
	defer b.mu.Unlock()

	qe, ok := b.queryExecutions.Get(id)
	if !ok {
		return fmt.Errorf("%w: query execution %q not found", ErrNotFound, id)
	}

	if isTerminalState(qe.Status.State) {
		return fmt.Errorf(
			"%w: query execution %q is already in terminal state %q",
			ErrValidation,
			id,
			qe.Status.State,
		)
	}

	qe.Status.State = stateCancelled

	return nil
}

// BatchGetQueryExecution retrieves multiple query executions by ID.
func (b *InMemoryBackend) BatchGetQueryExecution(
	ids []string,
) ([]QueryExecution, []UnprocessedQueryExecutionID) {
	b.mu.RLock("BatchGetQueryExecution")
	defer b.mu.RUnlock()

	found := make([]QueryExecution, 0, len(ids))
	unprocessed := make([]UnprocessedQueryExecutionID, 0, len(ids))

	for _, id := range ids {
		qe, ok := b.queryExecutions.Get(id)
		if ok {
			found = append(found, *qe)
		} else {
			unprocessed = append(unprocessed, UnprocessedQueryExecutionID{
				QueryExecutionID: id,
				ErrorCode:        "InvalidRequestException",
				ErrorMessage:     fmt.Sprintf("query execution %q not found", id),
			})
		}
	}

	return found, unprocessed
}

// GetQueryRuntimeStatistics returns runtime statistics for a query execution.
func (b *InMemoryBackend) GetQueryRuntimeStatistics(id string) (*QueryRuntimeStatistics, error) {
	b.mu.RLock("GetQueryRuntimeStatistics")
	defer b.mu.RUnlock()

	qe, ok := b.queryExecutions.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: query execution %q not found", ErrNotFound, id)
	}

	engineMs := qe.Statistics.EngineExecutionTimeInMillis

	return &QueryRuntimeStatistics{
		Timeline: QueryRuntimeStatisticsTimeline{
			EngineExecutionTimeInMillis: engineMs,
			TotalExecutionTimeInMillis:  engineMs,
		},
		Rows: QueryRuntimeStatisticsRows{
			InputBytes: qe.Statistics.DataScannedInBytes,
		},
		OutputStage: QueryStage{
			StageID: 0,
			State:   qe.Status.State,
		},
	}, nil
}
