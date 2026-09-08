package athena_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/athena"
)

const (
	testCatalog  = "AwsDataCatalog"
	testDatabase = "default"
)

func ddlCtx() athena.QueryExecutionContext {
	return athena.QueryExecutionContext{Catalog: testCatalog, Database: testDatabase}
}

// runStatement starts a query execution and returns the resulting execution.
func runStatement(t *testing.T, b *athena.InMemoryBackend, query string) *athena.QueryExecution {
	t.Helper()

	id, err := b.StartQueryExecution(query, "primary", ddlCtx(), athena.ResultConfiguration{}, nil, nil)
	require.NoError(t, err)

	qe, err := b.GetQueryExecution(id)
	require.NoError(t, err)

	return qe
}

// TestExecuteStatement_DDLAndDML drives the SQL engine's non-SELECT handling and
// asserts real catalog/state mutations (not a silent empty success) plus
// AWS-shaped FAILED executions for invalid statements.
func TestExecuteStatement_DDLAndDML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(b *athena.InMemoryBackend)
		verify    func(t *testing.T, b *athena.InMemoryBackend)
		name      string
		query     string
		wantState string
	}{
		{
			name:      "create_database",
			query:     "CREATE DATABASE analytics",
			wantState: "SUCCEEDED",
			verify: func(t *testing.T, b *athena.InMemoryBackend) {
				t.Helper()
				assert.True(t, b.HasDatabase(testCatalog, "analytics"))
			},
		},
		{
			name:      "create_database_if_not_exists_existing_ok",
			query:     "CREATE DATABASE IF NOT EXISTS default",
			wantState: "SUCCEEDED",
		},
		{
			name:      "create_database_duplicate_fails",
			query:     "CREATE DATABASE default",
			wantState: "FAILED",
		},
		{
			name: "drop_database",
			setup: func(b *athena.InMemoryBackend) {
				runStatement(t, b, "CREATE DATABASE analytics")
			},
			query:     "DROP DATABASE analytics",
			wantState: "SUCCEEDED",
			verify: func(t *testing.T, b *athena.InMemoryBackend) {
				t.Helper()
				assert.False(t, b.HasDatabase(testCatalog, "analytics"))
			},
		},
		{
			name:      "drop_database_missing_fails",
			query:     "DROP DATABASE ghostdb",
			wantState: "FAILED",
		},
		{
			name:      "drop_database_if_exists_missing_ok",
			query:     "DROP DATABASE IF EXISTS ghostdb",
			wantState: "SUCCEEDED",
		},
		{
			name:      "create_table_with_columns",
			query:     "CREATE EXTERNAL TABLE events (id bigint, name string, amount decimal(10,2))",
			wantState: "SUCCEEDED",
			verify: func(t *testing.T, b *athena.InMemoryBackend) {
				t.Helper()
				require.True(t, b.HasTable(testCatalog, testDatabase, "events"))
				tm, err := b.GetTableMetadata(testCatalog, testDatabase, "events")
				require.NoError(t, err)
				require.Len(t, tm.Columns, 3)
				assert.Equal(t, "id", tm.Columns[0].Name)
				assert.Equal(t, "bigint", tm.Columns[0].Type)
				assert.Equal(t, "amount", tm.Columns[2].Name)
				assert.Equal(t, "decimal(10,2)", tm.Columns[2].Type)
			},
		},
		{
			name:      "create_table_duplicate_fails",
			query:     "CREATE TABLE sample_table (id int)",
			wantState: "FAILED",
		},
		{
			name: "drop_table",
			setup: func(b *athena.InMemoryBackend) {
				runStatement(t, b, "CREATE TABLE tmp (id int)")
			},
			query:     "DROP TABLE tmp",
			wantState: "SUCCEEDED",
			verify: func(t *testing.T, b *athena.InMemoryBackend) {
				t.Helper()
				assert.False(t, b.HasTable(testCatalog, testDatabase, "tmp"))
			},
		},
		{
			name:      "drop_table_missing_fails",
			query:     "DROP TABLE ghost",
			wantState: "FAILED",
		},
		{
			name:      "drop_table_if_exists_missing_ok",
			query:     "DROP TABLE IF EXISTS ghost",
			wantState: "SUCCEEDED",
		},
		{
			name: "insert_values_populates_data",
			setup: func(b *athena.InMemoryBackend) {
				runStatement(t, b, "CREATE TABLE nums (n int)")
			},
			query:     "INSERT INTO nums VALUES (1), (2), (3)",
			wantState: "SUCCEEDED",
			verify: func(t *testing.T, b *athena.InMemoryBackend) {
				t.Helper()
				assert.Equal(t, 3, b.TableRowCount(testCatalog, testDatabase, "nums"))
			},
		},
		{
			name:      "insert_missing_table_fails",
			query:     "INSERT INTO ghost VALUES (1)",
			wantState: "FAILED",
		},
		{
			name: "ctas_materialises_rows",
			setup: func(b *athena.InMemoryBackend) {
				b.InsertRows(testCatalog, testDatabase, "src", []map[string]any{
					{"id": 1, "label": "a"},
					{"id": 2, "label": "b"},
				})
			},
			query:     "CREATE TABLE dst AS SELECT * FROM src",
			wantState: "SUCCEEDED",
			verify: func(t *testing.T, b *athena.InMemoryBackend) {
				t.Helper()
				require.True(t, b.HasTable(testCatalog, testDatabase, "dst"))
				assert.Equal(t, 2, b.TableRowCount(testCatalog, testDatabase, "dst"))
			},
		},
		{
			name:      "unsupported_statement_fails",
			query:     "FROBNICATE something",
			wantState: "FAILED",
		},
		{
			name:      "recognised_noop_show_ok",
			query:     "SHOW TABLES",
			wantState: "SUCCEEDED",
		},
		{
			name:      "recognised_noop_alter_ok",
			query:     "ALTER TABLE sample_table ADD COLUMNS (extra string)",
			wantState: "SUCCEEDED",
		},
		{
			name:      "select_still_succeeds",
			query:     "SELECT 1",
			wantState: "SUCCEEDED",
		},
		{
			name: "execute_no_using_no_params_succeeds",
			setup: func(b *athena.InMemoryBackend) {
				require.NoError(t, b.CreatePreparedStatement("get_all", "", "primary", "SELECT 1"))
			},
			query:     "EXECUTE get_all",
			wantState: "SUCCEEDED",
		},
		{
			name:      "execute_missing_statement_fails",
			query:     "EXECUTE ghost_stmt",
			wantState: "FAILED",
		},
		{
			name: "execute_param_count_mismatch_fails",
			setup: func(b *athena.InMemoryBackend) {
				require.NoError(t, b.CreatePreparedStatement(
					"get_by_id", "", "primary", "SELECT id FROM widgets WHERE id = ?",
				))
			},
			query:     "EXECUTE get_by_id USING 1, 2",
			wantState: "FAILED",
		},
		{
			name: "execute_wrong_workgroup_fails",
			setup: func(b *athena.InMemoryBackend) {
				require.NoError(t, b.CreateWorkGroup("secondary", "", "ENABLED", athena.WorkGroupConfiguration{}, nil))
				require.NoError(t, b.CreatePreparedStatement("wg_stmt", "", "secondary", "SELECT 1"))
			},
			query:     "EXECUTE wg_stmt",
			wantState: "FAILED",
		},
		{
			name: "execute_malformed_trailing_garbage_fails",
			setup: func(b *athena.InMemoryBackend) {
				require.NoError(t, b.CreatePreparedStatement("get_all", "", "primary", "SELECT 1"))
			},
			query:     "EXECUTE get_all bogus",
			wantState: "FAILED",
		},
		{
			name: "execute_nested_execute_fails",
			setup: func(b *athena.InMemoryBackend) {
				require.NoError(t, b.CreatePreparedStatement("inner", "", "primary", "SELECT 1"))
				require.NoError(t, b.CreatePreparedStatement("outer", "", "primary", "EXECUTE inner"))
			},
			query:     "EXECUTE outer",
			wantState: "FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := athena.NewInMemoryBackend("", "")
			if tt.setup != nil {
				tt.setup(b)
			}

			qe := runStatement(t, b, tt.query)
			assert.Equal(t, tt.wantState, qe.Status.State)

			if tt.wantState == "FAILED" {
				require.NotNil(t, qe.Status.AthenaError, "FAILED executions carry an AthenaError")
				assert.NotEmpty(t, qe.Status.StateChangeReason)
				assert.Equal(t, int32(2), qe.Status.AthenaError.ErrorCategory, "user error category")
			}

			if tt.verify != nil {
				tt.verify(t, b)
			}
		})
	}
}

// startQueryBody builds a StartQueryExecution request body for the default
// catalog/database, keeping test lines within the line-length limit.
func startQueryBody(query string) string {
	body, _ := json.Marshal(map[string]any{
		"QueryString": query,
		"WorkGroup":   "primary",
		"QueryExecutionContext": map[string]any{
			"Catalog":  testCatalog,
			"Database": testDatabase,
		},
	})

	return string(body)
}

// runHandlerQuery starts a query through the handler and returns its execution ID.
func runHandlerQuery(t *testing.T, h *athena.Handler, query string) string {
	t.Helper()

	rec := doRequest(t, h, "StartQueryExecution", startQueryBody(query))
	require.Equal(t, http.StatusOK, rec.Code)

	return jsonField(t, rec.Body.Bytes(), "QueryExecutionId")
}

// TestExecuteStatement_InsertThenSelect proves DML mutations are observable: rows
// written by INSERT are returned by a subsequent SELECT through the full handler.
func TestExecuteStatement_InsertThenSelect(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	runHandlerQuery(t, h, "CREATE TABLE inventory (sku string, qty int)")

	insID := runHandlerQuery(t, h, "INSERT INTO inventory VALUES ('abc', 5), ('def', 9)")
	insExec := doRequest(t, h, "GetQueryExecution", `{"QueryExecutionId":"`+insID+`"}`)

	var insResp map[string]any
	require.NoError(t, json.Unmarshal(insExec.Body.Bytes(), &insResp))
	status := insResp["QueryExecution"].(map[string]any)["Status"].(map[string]any)
	assert.Equal(t, "SUCCEEDED", status["State"])

	selID := runHandlerQuery(t, h, "SELECT sku, qty FROM inventory")

	results := doRequest(t, h, "GetQueryResults", `{"QueryExecutionId":"`+selID+`"}`)
	require.Equal(t, http.StatusOK, results.Code)

	var resResp map[string]any
	require.NoError(t, json.Unmarshal(results.Body.Bytes(), &resResp))

	rs := resResp["ResultSet"].(map[string]any)
	rows := rs["Rows"].([]any)
	// Header row + 2 data rows.
	assert.Len(t, rows, 3)
}
