package redshiftdata_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshiftdata"
)

// TestCancelStatement_SuccessStatusBoolean verifies AWS boolean response shape.
func TestCancelStatement_SuccessStatusBoolean(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)
	redshiftdata.AddStatementInternal(b, testRegion, "stmt-pending", "SELECT 1", "mydb", "STARTED", false)

	h := redshiftdata.NewHandler(b)

	rec := doRequest(t, h, "CancelStatement", map[string]any{"Id": "stmt-pending"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	status, ok := resp["Status"].(bool)
	require.True(t, ok, "Status should be a boolean")
	assert.True(t, status)
}

// TestBatchExecuteStatement_QueryStringIsFirstSQL verifies that
// BatchExecuteStatement sets QueryString to the first SQL in the batch, matching
// real AWS behaviour (used by DescribeStatement and ListStatements).
func TestBatchExecuteStatement_QueryStringIsFirstSQL(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":              []string{"SELECT 1", "SELECT 2", "SELECT 3"},
		"Database":          "testdb",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	assert.Equal(t, "SELECT 1", resp["QueryString"], "QueryString should equal the first SQL")
}

// TestDescribeStatement_ResultSizeAndRows verifies that DescribeStatement
// includes ResultSize and ResultRows for a FINISHED single statement.
func TestDescribeStatement_ResultSizeAndRows(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "testdb",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	assert.NotNil(t, resp["ResultRows"], "ResultRows should be present")
	assert.NotNil(t, resp["ResultSize"], "ResultSize should be present")
	assert.EqualValues(t, 1, resp["ResultRows"])
}

// TestDescribeStatement_SubStatements verifies that DescribeStatement
// for a batch includes a non-empty SubStatements array with per-SQL details.
func TestDescribeStatement_SubStatements(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	sqls := []string{"SELECT 1", "SELECT 2"}
	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":              sqls,
		"Database":          "testdb",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	subs, ok := resp["SubStatements"].([]any)
	require.True(t, ok, "SubStatements should be present")
	assert.Len(t, subs, 2, "should have one sub-statement per SQL")

	first := subs[0].(map[string]any)
	assert.Equal(t, "SELECT 1", first["QueryString"])
	assert.Equal(t, "FINISHED", first["Status"])
}

// TestGetStatementResult_ReturnsDemoRow verifies that GetStatementResult
// returns at least one row and one column in the demo result set.
func TestGetStatementResult_ReturnsDemoRow(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "testdb",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)

	resultRec := doRequest(t, h, "GetStatementResult", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, resultRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(resultRec.Body.Bytes(), &resp))

	records, ok := resp["Records"].([]any)
	require.True(t, ok, "Records should be a slice")
	assert.NotEmpty(t, records, "should return at least one row")

	cols, ok := resp["ColumnMetadata"].([]any)
	require.True(t, ok, "ColumnMetadata should be a slice")
	assert.NotEmpty(t, cols, "should return at least one column")

	assert.EqualValues(t, 1, resp["TotalNumRows"])
}

// TestListStatements_MaxResults limits the page size.
func TestListStatements_MaxResults(t *testing.T) {
	t.Parallel()

	// testPaginationStatements is the number of statements to create to test pagination.
	// It must be greater than testMaxResultsLimit so that results are truncated.
	const (
		testPaginationStatements = 5
		testMaxResultsLimit      = 3
	)

	h := newTestHandler(t)

	for range testPaginationStatements {
		doRequest(t, h, "ExecuteStatement", map[string]any{
			"Sql":               "SELECT 1",
			"Database":          "testdb",
			"ClusterIdentifier": "my-cluster",
		})
	}

	rec := doRequest(t, h, "ListStatements", map[string]any{
		"MaxResults": testMaxResultsLimit,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts := resp["Statements"].([]any)
	assert.Len(t, stmts, testMaxResultsLimit, "should be truncated to MaxResults")

	// A next-token should be present when there are more results.
	assert.NotEmpty(t, resp["NextToken"], "NextToken should be set when results are truncated")
}

// TestListStatements_MaxResultsTooLarge returns ValidationException.
func TestListStatements_MaxResultsTooLarge(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ListStatements", map[string]any{
		"MaxResults": 101,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

// TestWithEvent_AcceptedButNotEchoed verifies that WithEvent is accepted on
// the ExecuteStatement request without error, but never appears on
// DescribeStatement's response. WithEvent has no response counterpart
// anywhere in the SDK -- confirmed against
// aws-sdk-go-v2/service/redshiftdata@v1.43.4: it's a request-only field on
// ExecuteStatementInput/BatchExecuteStatementInput
// (api_op_ExecuteStatement.go, api_op_BatchExecuteStatement.go), and none of
// ExecuteStatementOutput/BatchExecuteStatementOutput/DescribeStatementOutput/
// StatementData declare it.
func TestWithEvent_AcceptedButNotEchoed(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "testdb",
		"WithEvent":         true,
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	assert.NotContains(t, createResp, "WithEvent")

	id := createResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	assert.NotContains(t, resp, "WithEvent")
}

// TestListStatements_StatusFilter verifies that ListStatements
// properly filters by Status.
func TestListStatements_StatusFilter(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)
	redshiftdata.AddStatementInternal(b, testRegion, "stmt-finished", "SELECT 1", "mydb", "FINISHED", true)
	redshiftdata.AddStatementInternal(b, testRegion, "stmt-failed", "SELECT 2", "mydb", "FAILED", false)

	h := redshiftdata.NewHandler(b)

	rec := doRequest(t, h, "ListStatements", map[string]any{
		"Status": "FAILED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts := resp["Statements"].([]any)
	require.Len(t, stmts, 1)
	assert.Equal(t, "FAILED", stmts[0].(map[string]any)["Status"])
}

// TestRingBuffer_Overflow verifies that adding more than
// maxStatementHistory statements correctly evicts the oldest.
func TestRingBuffer_Overflow(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)

	// Add exactly maxStatementHistory + 5 statements.
	maxCap := redshiftdata.MaxStatementHistoryForTest
	overCount := 5

	for i := range maxCap + overCount {
		redshiftdata.AddStatementInternal(b, testRegion, generateID(i), "SELECT 1", "dev", "FINISHED", false)
	}

	// The backend should never exceed the cap.
	assert.Equal(t, maxCap, b.StatementCount(), "statement count must not exceed ring buffer capacity")
}

// generateID produces a deterministic test ID from an integer index.
func generateID(i int) string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, 8)
	n := i

	for j := 7; j >= 0; j-- {
		b[j] = hexChars[n%16]
		n /= 16
	}

	return "test-" + string(b)
}

// TestConcurrent_AccessSafe tests concurrent reads and writes to
// the InMemoryBackend to catch data races (run with -race).
func TestConcurrent_AccessSafe(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)

	const goroutines = 20

	var wg sync.WaitGroup

	// Concurrent writes
	for range goroutines {
		wg.Go(func() {
			_, _ = b.ExecuteStatement(
				context.Background(), "SELECT 1", "my-cluster", "", "dev", "", "", "", false, "", nil, "",
			)
		})
	}

	// Concurrent reads interleaved
	for range goroutines {
		wg.Go(func() {
			stmts, _, _ := b.ListStatements(
				context.Background(), redshiftdata.ListStatementsFilter{Status: "ALL", MaxResults: 100},
			)
			_ = stmts
		})
	}

	wg.Wait()

	// No panic or race = pass.
	assert.Positive(t, b.StatementCount())
}

// TestListStatements_WorkgroupFilter verifies that ListStatements
// filters by WorkgroupName when provided. WorkgroupName is a request filter
// only, not a real ListStatements response field (types.StatementData has
// no WorkgroupName member), so the assertion checks which statement was
// returned by StatementName rather than reading a fabricated response key.
func TestListStatements_WorkgroupFilter(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)
	h := redshiftdata.NewHandler(b)

	_, err := b.ExecuteStatement(
		context.Background(), "SELECT 1", "", "wg-a", "dev", "", "", "wg-a-stmt", false, "", nil, "",
	)
	require.NoError(t, err)

	_, err = b.ExecuteStatement(
		context.Background(), "SELECT 2", "", "wg-b", "dev", "", "", "wg-b-stmt", false, "", nil, "",
	)
	require.NoError(t, err)

	rec := doRequest(t, h, "ListStatements", map[string]any{
		"WorkgroupName": "wg-a",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts := resp["Statements"].([]any)
	require.Len(t, stmts, 1, "should only return statements for wg-a")
	assert.Equal(t, "wg-a-stmt", stmts[0].(map[string]any)["StatementName"])
}

// TestGetStatementResultV2_DemoRow verifies that GetStatementResultV2
// returns at least one row in CSV format and a ResultFormat field.
func TestGetStatementResultV2_DemoRow(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	execRec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "dev",
		"ResultFormat":      "CSV",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, execRec.Code)

	var execResp map[string]any
	require.NoError(t, json.Unmarshal(execRec.Body.Bytes(), &execResp))

	id := execResp["Id"].(string)

	rec := doRequest(t, h, "GetStatementResultV2", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	records, ok := resp["Records"].([]any)
	require.True(t, ok, "Records should be present")
	assert.NotEmpty(t, records, "should return at least one row")
	assert.Equal(t, "CSV", resp["ResultFormat"], "ResultFormat should be CSV")

	cols, ok := resp["ColumnMetadata"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, cols)
}

// TestCancelStatement_NotFound verifies that CancelStatement on a
// non-existent ID returns a ResourceNotFoundException (HTTP 400).
func TestCancelStatement_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CancelStatement", map[string]any{"Id": "does-not-exist"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestGetStatementResult_NotFound verifies that GetStatementResult
// on a non-existent statement returns a ResourceNotFoundException.
func TestGetStatementResult_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "GetStatementResult", map[string]any{"Id": "does-not-exist"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestBatchExecuteStatement_EmptySqls verifies that submitting an
// empty Sqls array returns a ValidationException.
func TestBatchExecuteStatement_EmptySqls(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":     []string{},
		"Database": "dev",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

// TestDescribeStatement_MissingID verifies that DescribeStatement
// with no Id field returns a ValidationException.
func TestDescribeStatement_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DescribeStatement", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

// TestDescribeStatement_RedshiftQueryId verifies that DescribeStatement
// returns the RedshiftQueryId field (set to 0 in the mock).
func TestDescribeStatement_RedshiftQueryId(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	execRec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "dev",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, execRec.Code)

	var execResp map[string]any
	require.NoError(t, json.Unmarshal(execRec.Body.Bytes(), &execResp))

	id := execResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	_, ok := resp["RedshiftQueryId"]
	assert.True(t, ok, "RedshiftQueryId should be present in DescribeStatement response")
}

// TestEvictExpiredStatements_UpdatesRingBuffer verifies that after
// eviction the ring buffer is compacted and the backend is consistent.
func TestEvictExpiredStatements_UpdatesRingBuffer(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)

	redshiftdata.AddStatementInternal(b, testRegion, "s1", "SELECT 1", "dev", "FINISHED", true)
	redshiftdata.AddStatementInternal(b, testRegion, "s2", "SELECT 2", "dev", "FINISHED", true)
	redshiftdata.AddStatementInternal(b, testRegion, "s3", "SELECT 3", "dev", "STARTED", false) // must not be evicted

	evicted := b.EvictExpiredStatements(time.Now().Add(time.Hour))

	assert.Equal(t, 2, evicted)
	assert.Equal(t, 1, b.StatementCount())

	// The remaining statement should still be fetchable via ListStatements.
	stmts, _, _ := b.ListStatements(
		context.Background(), redshiftdata.ListStatementsFilter{Status: "ALL", MaxResults: 100},
	)
	require.Len(t, stmts, 1)
	assert.Equal(t, "STARTED", stmts[0].Status)
}

// TestGetStatementResult_HasNextToken verifies that GetStatementResult
// returns a NextToken field (empty string = no more pages in the mock).
func TestGetStatementResult_HasNextToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	execRec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "dev",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, execRec.Code)

	var execResp map[string]any
	require.NoError(t, json.Unmarshal(execRec.Body.Bytes(), &execResp))

	id := execResp["Id"].(string)

	rec := doRequest(t, h, "GetStatementResult", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	_, ok := resp["NextToken"]
	assert.True(t, ok, "NextToken field should always be present in GetStatementResult response")
}
