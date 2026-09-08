package redshiftdata_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshiftdata"
)

func TestValidateListStatementsStatus_ValidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
	}{
		{name: "empty_string", status: ""},
		{name: "ALL", status: "ALL"},
		{name: "ABORTED", status: "ABORTED"},
		{name: "FAILED", status: "FAILED"},
		{name: "FINISHED", status: "FINISHED"},
		{name: "PICKED", status: "PICKED"},
		{name: "STARTED", status: "STARTED"},
		{name: "SUBMITTED", status: "SUBMITTED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := redshiftdata.ValidateListStatementsStatus(tt.status)
			require.NoError(t, err, "status %q should be valid", tt.status)
		})
	}
}

func TestValidateListStatementsStatus_InvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
	}{
		{name: "lowercase_all", status: "all"},
		{name: "partial", status: "FINISH"},
		{name: "unknown", status: "RUNNING"},
		{name: "whitespace", status: " ALL"},
		{name: "mixed_case", status: "Finished"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := redshiftdata.ValidateListStatementsStatus(tt.status)
			require.Error(t, err)
			require.ErrorIs(t, err, redshiftdata.ErrValidation)
			assert.Contains(t, err.Error(), tt.status)
		})
	}
}

func TestHandler_ListStatements_InvalidStatus_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ListStatements", map[string]any{"Status": "INVALID_STATUS"})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ValidationException")
	assert.Contains(t, rec.Body.String(), "INVALID_STATUS")
}

func TestBackend_BatchExecuteStatement_EmptySqlItem_Returns400(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
		sqls []string
	}{
		{
			name: "first_empty",
			sqls: []string{"", "SELECT 2"},
			want: "Sqls[0]",
		},
		{
			name: "second_empty",
			sqls: []string{"SELECT 1", ""},
			want: "Sqls[1]",
		},
		{
			name: "all_empty",
			sqls: []string{"", ""},
			want: "Sqls[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
				"Sqls":              tt.sqls,
				"Database":          "dev",
				"ClusterIdentifier": "cluster-a",
			})

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.want)
		})
	}
}

func TestHandler_BatchExecuteStatement_EmptySqlItem_Returns400(t *testing.T) {
	t.Parallel()

	rec := doRequest(t, newTestHandler(t), "BatchExecuteStatement", map[string]any{
		"Sqls":              []string{"SELECT 1", ""},
		"Database":          "dev",
		"ClusterIdentifier": "cluster-a",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ValidationException")
}

func TestHandler_ExecuteAndBatchExecuteStatement_RejectInvalidConnectionTarget(t *testing.T) {
	t.Parallel()

	t.Run("execute_rejects_both_cluster_and_workgroup", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, newTestHandler(t), "ExecuteStatement", map[string]any{
			"Sql":               "SELECT 1",
			"Database":          "dev",
			"ClusterIdentifier": "my-cluster",
			"WorkgroupName":     "my-workgroup",
		})

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ValidationException")
	})

	t.Run("execute_rejects_neither_cluster_nor_workgroup", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, newTestHandler(t), "ExecuteStatement", map[string]any{
			"Sql":      "SELECT 1",
			"Database": "dev",
		})

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ValidationException")
	})

	t.Run("batch_execute_rejects_neither_cluster_nor_workgroup", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, newTestHandler(t), "BatchExecuteStatement", map[string]any{
			"Sqls":     []string{"SELECT 1", "SELECT 2"},
			"Database": "dev",
		})

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ValidationException")
	})

	t.Run("batch_execute_rejects_both_cluster_and_workgroup", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, newTestHandler(t), "BatchExecuteStatement", map[string]any{
			"Sqls":              []string{"SELECT 1", "SELECT 2"},
			"Database":          "dev",
			"ClusterIdentifier": "my-cluster",
			"WorkgroupName":     "my-workgroup",
		})

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ValidationException")
	})
}

func TestHandler_ListStatements_MaxResults_Paginates(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)

	ids := []string{"parity-stmt-1", "parity-stmt-2", "parity-stmt-3", "parity-stmt-4", "parity-stmt-5"}
	for i, id := range ids {
		redshiftdata.AddStatementInternal(b, testRegion, id, "SELECT "+string(rune('1'+i)), "dev", "FINISHED", true)
	}

	h := redshiftdata.NewHandler(b)

	rec := doRequest(t, h, "ListStatements", map[string]any{
		"Status":     "ALL",
		"MaxResults": 2,
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts, _ := resp["Statements"].([]any)
	assert.Len(t, stmts, 2)

	token, _ := resp["NextToken"].(string)
	assert.NotEmpty(t, token, "NextToken should be set when more results exist")
}

func TestHandler_ListStatements_MaxResultsTooHigh_Returns400(t *testing.T) {
	t.Parallel()

	rec := doRequest(t, newTestHandler(t), "ListStatements", map[string]any{
		"MaxResults": 9999,
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ValidationException")
}

// TestHandler_ListStatements_RejectsBothClusterAndWorkgroup guards
// ListStatementsInput's documented mutual-exclusivity constraint ("When
// providing ClusterIdentifier, then WorkgroupName can't be specified",
// api_op_ListStatements.go, aws-sdk-go-v2/service/redshiftdata@v1.43.4) --
// unlike ExecuteStatement/BatchExecuteStatement, neither field is required,
// so only the both-set case is invalid.
func TestHandler_ListStatements_RejectsBothClusterAndWorkgroup(t *testing.T) {
	t.Parallel()

	t.Run("both_set_returns_400", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, newTestHandler(t), "ListStatements", map[string]any{
			"ClusterIdentifier": "my-cluster",
			"WorkgroupName":     "my-workgroup",
		})

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ValidationException")
	})

	t.Run("neither_set_returns_200", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, newTestHandler(t), "ListStatements", map[string]any{})

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("only_cluster_set_returns_200", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, newTestHandler(t), "ListStatements", map[string]any{
			"ClusterIdentifier": "my-cluster",
		})

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestAddStatementInternal verifies the seed helper bypasses UUID generation.
func TestAddStatementInternal(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)
	redshiftdata.AddStatementInternal(b, testRegion, "fixed-id", "SELECT 1", "mydb", "FINISHED", true)

	stmt, err := b.DescribeStatement(context.Background(), "fixed-id")
	require.NoError(t, err)
	assert.Equal(t, "fixed-id", stmt.ID)
	assert.Equal(t, "SELECT 1", stmt.QueryString)
	assert.Equal(t, "FINISHED", stmt.Status)
	assert.True(t, stmt.HasResultSet)
}

// TestExecuteStatement_DatabaseOptional verifies Database is NOT required on
// ExecuteStatement: unlike ListDatabases/ListSchemas/ListTables/DescribeTable
// (whose Database is a hard @required member, enforced client-side in
// validateOpListDatabasesInput et al.), ExecuteStatementInput.Database's doc
// comment states it is only "required when authenticating using either
// Secrets Manager or temporary credentials," and
// validateOpExecuteStatementInput (aws-sdk-go-v2/service/redshiftdata@
// v1.43.4's validators.go) has no Database check at all. Previously this
// package rejected every Database-less request with ValidationException,
// which was stricter than real AWS.
func TestExecuteStatement_DatabaseOptional(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"ClusterIdentifier": "my-cluster",
	})
	assert.Equal(t, http.StatusOK, rec.Code, "Database is conditionally, not unconditionally, required")
}

// TestBatchExecuteStatement_DatabaseOptional mirrors
// TestExecuteStatement_DatabaseOptional for BatchExecuteStatement, whose
// Database member carries the identical conditional doc comment and lacks a
// validator entry too.
func TestBatchExecuteStatement_DatabaseOptional(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":              []string{"SELECT 1", "SELECT 2"},
		"ClusterIdentifier": "my-cluster",
	})
	assert.Equal(t, http.StatusOK, rec.Code, "Database is conditionally, not unconditionally, required")
}

// TestGetStatementResult_NoResultSet verifies that GetStatementResult
// returns ValidationException when the statement has no result set (e.g. batch).
func TestGetStatementResult_NoResultSet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":              []string{"INSERT INTO t VALUES (1)"},
		"Database":          "testdb",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var batchResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &batchResp))

	id := batchResp["Id"].(string)
	rec = doRequest(t, h, "GetStatementResult", map[string]any{"Id": id})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

// TestGetStatementResultV2_NoResultSet verifies that GetStatementResultV2
// returns ValidationException when the statement has no result set.
func TestGetStatementResultV2_NoResultSet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":              []string{"INSERT INTO t VALUES (1)"},
		"Database":          "testdb",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var batchResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &batchResp))

	id := batchResp["Id"].(string)
	rec = doRequest(t, h, "GetStatementResultV2", map[string]any{"Id": id})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

// TestGetStatementResultV2_ResultFormat verifies that GetStatementResultV2
// returns ResultFormat=CSV in the response body.
func TestGetStatementResultV2_ResultFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "testdb",
		"ResultFormat":      "CSV",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)
	rec = doRequest(t, h, "GetStatementResultV2", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "CSV", resp["ResultFormat"])
	assert.EqualValues(t, 1, resp["TotalNumRows"])
	assert.NotNil(t, resp["Records"])
	assert.NotNil(t, resp["ColumnMetadata"])
}

// TestCancelStatement_AbortedIsTerminal verifies that an already-aborted
// statement cannot be cancelled again.
func TestCancelStatement_AbortedIsTerminal(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)
	redshiftdata.AddStatementInternal(b, testRegion, "stmt-1", "SELECT 1", "mydb", "ABORTED", false)

	h := redshiftdata.NewHandler(b)

	rec := doRequest(t, h, "CancelStatement", map[string]any{"Id": "stmt-1"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

// TestCancelStatement_FailedIsTerminal verifies that a failed statement
// cannot be cancelled.
func TestCancelStatement_FailedIsTerminal(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)
	redshiftdata.AddStatementInternal(b, testRegion, "stmt-1", "SELECT 1", "mydb", "FAILED", false)

	h := redshiftdata.NewHandler(b)

	rec := doRequest(t, h, "CancelStatement", map[string]any{"Id": "stmt-1"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

// TestListStatements_SortedNewestFirst verifies that ListStatements returns
// results sorted by creation time with the newest statement first.
func TestListStatements_SortedNewestFirst(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 3 {
		doRequest(t, h, "ExecuteStatement", map[string]any{
			"Sql":               fmt.Sprintf("SELECT %d", i+1),
			"Database":          "testdb",
			"StatementName":     fmt.Sprintf("stmt-%d", i),
			"ClusterIdentifier": "my-cluster",
		})
	}

	rec := doRequest(t, h, "ListStatements", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts := resp["Statements"].([]any)
	require.Len(t, stmts, 3)

	// Verify they are ordered newest → oldest by checking CreatedAt is non-increasing.
	first := stmts[0].(map[string]any)["CreatedAt"].(float64)
	second := stmts[1].(map[string]any)["CreatedAt"].(float64)
	third := stmts[2].(map[string]any)["CreatedAt"].(float64)

	assert.GreaterOrEqual(t, first, second)
	assert.GreaterOrEqual(t, second, third)
}

// TestExecuteStatement_HasResultSet verifies that ExecuteStatement
// sets HasResultSet to true for SELECT statements.
func TestExecuteStatement_HasResultSet(t *testing.T) {
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

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	assert.Equal(t, true, descResp["HasResultSet"])
}

// TestBatchExecuteStatement_HasNoResultSet verifies that BatchExecuteStatement
// always sets HasResultSet to false.
func TestBatchExecuteStatement_HasNoResultSet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":              []string{"SELECT 1"},
		"Database":          "testdb",
		"ClusterIdentifier": "my-cluster",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	assert.Equal(t, false, descResp["HasResultSet"])
}

// TestDescribeStatement_CloneDoesNotMutate verifies that modifying
// the returned Statement from DescribeStatement does not affect the backend store.
func TestDescribeStatement_CloneDoesNotMutate(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)
	stmt, err := b.ExecuteStatement(
		context.Background(), "SELECT 1", "cluster", "", "mydb", "", "", "", false, "", nil, "",
	)
	require.NoError(t, err)

	got, err := b.DescribeStatement(context.Background(), stmt.ID)
	require.NoError(t, err)

	// Mutate the returned copy.
	got.Status = "MUTATED"
	got.QueryStrings = append(got.QueryStrings, "injected")

	// Original should be unaffected.
	original, err := b.DescribeStatement(context.Background(), stmt.ID)
	require.NoError(t, err)
	assert.Equal(t, "FINISHED", original.Status)
	assert.Empty(t, original.QueryStrings)
}

// TestStatementCount_RaceCondition verifies concurrent access is safe.
func TestStatementCount_RaceCondition(t *testing.T) {
	t.Parallel()

	b := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)

	done := make(chan struct{})

	go func() {
		for range 50 {
			_, _ = b.ExecuteStatement(context.Background(), "SELECT 1", "", "", "mydb", "", "", "", false, "", nil, "")
		}

		close(done)
	}()

	for range 50 {
		_ = b.StatementCount()
		_, _, _ = b.ListStatements(context.Background(), redshiftdata.ListStatementsFilter{Status: "ALL"})
	}

	<-done
}
