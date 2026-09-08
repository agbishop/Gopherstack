package redshiftdata

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	// statusFinished is the FINISHED status for a SQL statement.
	statusFinished = "FINISHED"
	// statusFailed is the FAILED status for a SQL statement.
	statusFailed = "FAILED"
	// statusAborted is the ABORTED status for a SQL statement (cancelled).
	statusAborted = "ABORTED"
	// maxStatementHistory is the maximum number of statements to retain in memory per region.
	maxStatementHistory = 1000
	// resultFormatCSV is the CSV result format returned by GetStatementResultV2.
	resultFormatCSV = "CSV"
	// resultFormatJSON is the default result format returned by GetStatementResult.
	resultFormatJSON = "JSON"
	// maxListStatementsResults is the maximum number of statements AWS allows per ListStatements page.
	maxListStatementsResults = 100
	// defaultListStatementsResults is the default page size for ListStatements when MaxResults is 0.
	defaultListStatementsResults = 100
	// mockColumnSize is the VARCHAR column size used in demo result metadata.
	mockColumnSize = int64(256)
	// mockColumnNullable indicates the demo column allows NULL.
	mockColumnNullable = int64(1)
	// mockStatementDurationMs is the simulated execution duration for demo statements.
	mockStatementDurationMs = int64(1)
	// demoResultRows is the simulated row count returned for FINISHED single statements.
	demoResultRows = int64(1)
	// demoResultSize is the simulated result payload size in bytes for FINISHED statements.
	demoResultSize = int64(64)
	// statusAll matches all statement statuses in ListStatements.
	statusAll = "ALL"

	// maxListDatabasesResults is the maximum page size for ListDatabases.
	maxListDatabasesResults = 60
	// defaultListDatabasesResults is the default page size for ListDatabases.
	defaultListDatabasesResults = 60
	// maxListSchemasResults is the maximum page size for ListSchemas.
	maxListSchemasResults = 1000
	// defaultListSchemasResults is the default page size for ListSchemas.
	defaultListSchemasResults = 1000
	// maxListTablesResults is the maximum page size for ListTables.
	maxListTablesResults = 1000
	// defaultListTablesResults is the default page size for ListTables.
	defaultListTablesResults = 1000
)

// ValidateListStatementsStatus returns ErrValidation if status is not a known value.
// An empty string is also accepted (matches FINISHED per AWS default).
func ValidateListStatementsStatus(status string) error {
	if status == "" {
		return nil
	}

	switch status {
	case statusAll, statusAborted, statusFailed, statusFinished, "PICKED", "STARTED", "SUBMITTED":
		return nil
	default:
		return fmt.Errorf(
			"%w: Status %q is invalid; valid values are ALL, ABORTED, FAILED, FINISHED, PICKED, STARTED, SUBMITTED",
			ErrValidation, status,
		)
	}
}

// ValidateListStatementsConnectionTarget enforces the mutual-exclusivity
// constraint documented on ListStatementsInput.ClusterIdentifier / .WorkgroupName
// (aws-sdk-go-v2/service/redshiftdata@v1.43.4's api_op_ListStatements.go: "When
// providing ClusterIdentifier, then WorkgroupName can't be specified" and the
// mirrored sentence on WorkgroupName). Unlike ExecuteStatement/
// BatchExecuteStatement (ValidateConnectionTarget), neither field is required
// here: ListStatements' own doc comment only requires one "when you use
// identity-enhanced role sessions," so specifying neither must still list
// across all targets, matching the identical precedent in ValidateListSessionsRequest.
func ValidateListStatementsConnectionTarget(clusterIdentifier, workgroupName string) error {
	if clusterIdentifier != "" && workgroupName != "" {
		return fmt.Errorf(
			"%w: specify either ClusterIdentifier or WorkgroupName, not both",
			ErrValidation,
		)
	}

	return nil
}

// ValidateConnectionTarget verifies that exactly one of clusterIdentifier or
// workgroupName is provided, matching the AWS constraint.
func ValidateConnectionTarget(clusterIdentifier, workgroupName string) error {
	hasBoth := clusterIdentifier != "" && workgroupName != ""
	hasNeither := clusterIdentifier == "" && workgroupName == ""

	if hasBoth {
		return fmt.Errorf(
			"%w: specify either ClusterIdentifier or WorkgroupName, not both",
			ErrValidation,
		)
	}

	if hasNeither {
		return fmt.Errorf(
			"%w: either ClusterIdentifier or WorkgroupName is required",
			ErrValidation,
		)
	}

	return nil
}

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

// SQLParameter is a named SQL parameter for use in parameterized queries, matching
// the SQLParameter type in the AWS Redshift Data API.
type SQLParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// sqlHasResultSet reports whether a SQL statement produces a result set.
// Real AWS sets HasResultSet=true only for read-only statements (SELECT, SHOW,
// EXPLAIN, DESCRIBE, WITH, VALUES, TABLE). DML and DDL return false.
func sqlHasResultSet(sql string) bool {
	fields := strings.Fields(strings.ToUpper(sql))
	if len(fields) == 0 {
		return false
	}

	switch fields[0] {
	case "SELECT", "SHOW", "EXPLAIN", "DESCRIBE", "WITH", "VALUES", "TABLE":
		return true
	}

	return false
}

// SubStatementData represents a single sub-statement within a batch, matching
// the SubStatementData shape returned by AWS DescribeStatement for batch runs.
type SubStatementData struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	QueryString  string    `json:"queryString"`
	Status       string    `json:"status"`
	Error        string    `json:"error"`
	HasResultSet bool      `json:"hasResultSet"`
	ResultRows   int64     `json:"resultRows"`
	ResultSize   int64     `json:"resultSize"`
	DurationMs   int64     `json:"durationMs"`
}

// Statement represents an AWS Redshift Data API SQL statement.
type Statement struct {
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	Database          string    `json:"database"`
	ID                string    `json:"id"`
	ClusterIdentifier string    `json:"clusterIdentifier"`
	WorkgroupName     string    `json:"workgroupName"`
	QueryString       string    `json:"queryString"`
	DBUser            string    `json:"dbUser"`
	SecretARN         string    `json:"secretARN"`
	StatementName     string    `json:"statementName"`
	ResultFormat      string    `json:"resultFormat"`
	// SessionID is the session identifier echoed back from the ExecuteStatement/
	// BatchExecuteStatement request that created this statement (StatementData.SessionId
	// / DescribeStatementOutput.SessionId in the real API). Empty when the caller did
	// not supply one -- this mock does not mint new session ids on the caller's behalf,
	// it only threads through what was provided (see handleExecuteStatement).
	SessionID     string             `json:"sessionID,omitempty"`
	Status        string             `json:"status"`
	Error         string             `json:"error"`
	QueryStrings  []string           `json:"queryStrings"`
	Parameters    []SQLParameter     `json:"parameters,omitempty"`
	SubStatements []SubStatementData `json:"subStatements,omitempty"`
	// DurationMs is the total wall-clock execution time in milliseconds. Populated
	// when the statement reaches a terminal state (FINISHED / FAILED / ABORTED).
	DurationMs       int64 `json:"durationMs"`
	ResultRows       int64 `json:"resultRows"`
	ResultSize       int64 `json:"resultSize"`
	HasResultSet     bool  `json:"hasResultSet"`
	IsBatchStatement bool  `json:"isBatchStatement"`
	// WithEvent indicates whether an EventBridge event is generated on completion.
	WithEvent bool `json:"withEvent"`
}

// ListStatementsFilter controls statement filtering and pagination.
type ListStatementsFilter struct {
	ClusterIdentifier string
	WorkgroupName     string
	Database          string
	StatementName     string
	Status            string
	NextToken         string
	MaxResults        int
}

// SessionData represents an AWS Redshift Data API session, matching the
// SessionData shape returned by ListSessions. This backend does not model
// sessions as a first-class stored resource -- there is no explicit
// CreateSession/CloseSession API to persist against. Instead, a session is
// derived by grouping stored Statement records that share a non-empty
// SessionID (see groupSessions in sessions.go): the session's connection
// target and timestamps come from the statements that ran within it.
//
// SessionAliveSeconds and SessionTTL are intentionally omitted (both optional
// wire members): tracking them behaviorally would require the same
// SessionKeepAliveSeconds plumbing that ExecuteStatement/BatchExecuteStatement
// already accept-but-ignore (see handleExecuteStatement's doc comment) --
// adding real semantics for one op without the other would be inconsistent,
// and this pass only implements ListSessions.
type SessionData struct {
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	SessionID         string    `json:"sessionId"`
	ClusterIdentifier string    `json:"clusterIdentifier,omitempty"`
	WorkgroupName     string    `json:"workgroupName,omitempty"`
	Database          string    `json:"database,omitempty"`
	DBUser            string    `json:"dbUser,omitempty"`
	Status            string    `json:"status"`
}

// ListSessionsFilter controls session filtering and pagination.
type ListSessionsFilter struct {
	ClusterIdentifier string
	WorkgroupName     string
	Database          string
	SessionID         string
	Status            string
	NextToken         string
	MaxResults        int
}
