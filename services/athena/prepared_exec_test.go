package athena_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/athena"
)

// TestExecuteStatement_SubstitutesParametersAndFiltersRows proves EXECUTE ...
// USING actually substitutes into the stored QueryStatement and that the
// substituted query takes the normal SELECT path (real row filtering), not an
// empty no-op result set.
func TestExecuteStatement_SubstitutesParametersAndFiltersRows(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("", "")
	b.InsertRows(testCatalog, testDatabase, "widgets", []map[string]any{
		{"name": "alpha"},
		{"name": "beta"},
	})
	require.NoError(t, b.CreatePreparedStatement(
		"find_widget", "", "primary", "SELECT name FROM widgets WHERE name = ?",
	))

	h := athena.NewHandler(b)

	id, err := b.StartQueryExecution(
		"EXECUTE find_widget USING 'beta'", "primary", ddlCtx(), athena.ResultConfiguration{}, nil, nil,
	)
	require.NoError(t, err)

	qe, err := b.GetQueryExecution(id)
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", qe.Status.State)

	rows := fetchResultRows(t, h, id)
	require.Len(t, rows, 1, "only the matching row")
	assert.Equal(t, "beta", rows[0][0])
}

// TestExecuteStatement_UsingQuotedCommaIsOneParameter proves a comma embedded in
// a quoted USING value is not mistaken for a parameter separator (the
// injection-shaped hazard: a value's own punctuation must not corrupt parsing).
// Splitting it into two values would produce a parameter-count mismatch against
// the single "?" in the prepared statement, failing the query instead of
// matching the row.
func TestExecuteStatement_UsingQuotedCommaIsOneParameter(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("", "")
	b.InsertRows(testCatalog, testDatabase, "widgets", []map[string]any{
		{"name": "a,b"},
		{"name": "other"},
	})
	require.NoError(t, b.CreatePreparedStatement(
		"find_widget", "", "primary", "SELECT name FROM widgets WHERE name = ?",
	))

	h := athena.NewHandler(b)

	id, err := b.StartQueryExecution(
		"EXECUTE find_widget USING 'a,b'", "primary", ddlCtx(), athena.ResultConfiguration{}, nil, nil,
	)
	require.NoError(t, err)

	qe, err := b.GetQueryExecution(id)
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", qe.Status.State, qe.Status.StateChangeReason)

	rows := fetchResultRows(t, h, id)
	require.Len(t, rows, 1)
	assert.Equal(t, "a,b", rows[0][0])
}

// TestExecuteStatement_OtherNoOpsStillNoOp guards against a regression in the
// isRecognisedNoOp prefix list: giving EXECUTE real handling must not disturb
// the other statement types that remain successful no-ops.
func TestExecuteStatement_OtherNoOpsStillNoOp(t *testing.T) {
	t.Parallel()

	queries := []string{
		"PREPARE p1 FROM SELECT 1",
		"DEALLOCATE PREPARE p1",
		"ALTER TABLE sample_table ADD COLUMNS (extra string)",
		"MSCK REPAIR TABLE sample_table",
		"SHOW TABLES",
		"DESCRIBE sample_table",
		"EXPLAIN SELECT 1",
		"USE default",
		"SET client_info = 'x'",
		"RESET client_info",
		"UNLOAD (SELECT 1) TO 's3://bucket/' WITH (format = 'JSON')",
		"CALL some_procedure()",
		"GRANT SELECT ON sample_table TO alice",
		"REVOKE SELECT ON sample_table FROM alice",
		"COMMENT ON TABLE sample_table IS 'x'",
		"REFRESH TABLE sample_table",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			t.Parallel()

			b := athena.NewInMemoryBackend("", "")

			id, err := b.StartQueryExecution(query, "primary", ddlCtx(), athena.ResultConfiguration{}, nil, nil)
			require.NoError(t, err)

			qe, err := b.GetQueryExecution(id)
			require.NoError(t, err)
			assert.Equal(t, "SUCCEEDED", qe.Status.State, "%s must remain a no-op", query)
		})
	}
}

// fetchResultRows runs GetQueryResults through the handler and returns the
// string cell values of the data rows (header row excluded).
func fetchResultRows(t *testing.T, h *athena.Handler, id string) [][]string {
	t.Helper()

	rec := doRequest(t, h, "GetQueryResults", `{"QueryExecutionId":"`+id+`"}`)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	rs, _ := resp["ResultSet"].(map[string]any)
	require.NotNil(t, rs)

	allRows, _ := rs["Rows"].([]any)
	if len(allRows) == 0 {
		return nil
	}

	var out [][]string

	for _, r := range allRows[1:] {
		data, _ := r.(map[string]any)["Data"].([]any)

		row := make([]string, len(data))
		for i, cell := range data {
			row[i], _ = cell.(map[string]any)["VarCharValue"].(string)
		}

		out = append(out, row)
	}

	return out
}
