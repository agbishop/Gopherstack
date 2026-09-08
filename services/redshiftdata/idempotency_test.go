package redshiftdata_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteStatement_ClientToken_ReplaysOnRetry verifies that retrying an
// ExecuteStatement call with the same ClientToken (simulating a client
// resending a request whose response was lost in transit) replays the
// original statement Id instead of creating a second statement. ClientToken
// is documented as "a unique, case-sensitive identifier that you provide to
// ensure the idempotency of the request" (api_op_ExecuteStatement.go), and
// the SDK client auto-generates one when the caller omits it specifically to
// make its own retry-after-lost-response logic safe -- so a caller-supplied
// token must dedupe the same way, matching the precedent already established
// in services/scheduler/idempotency.go for the identical trait.
func TestExecuteStatement_ClientToken_ReplaysOnRetry(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	req := map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "dev",
		"ClientToken":       "retry-token-1",
		"ClusterIdentifier": "my-cluster",
	}

	rec1 := doRequest(t, h, "ExecuteStatement", req)
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))

	rec2 := doRequest(t, h, "ExecuteStatement", req)
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))

	assert.Equal(t, out1["Id"], out2["Id"], "retry with the same ClientToken must replay the same statement Id")

	listRec := doRequest(t, h, "ListStatements", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)

	var list struct {
		Statements []map[string]any `json:"Statements"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	assert.Len(t, list.Statements, 1, "exactly one statement should actually be created")
}

// TestExecuteStatement_NoClientToken_CreatesDistinctStatements verifies the
// idempotency cache does not change behavior for the common case: repeating
// an identical request without a ClientToken still creates a new statement
// each time.
func TestExecuteStatement_NoClientToken_CreatesDistinctStatements(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	req := map[string]any{
		"Sql":               "SELECT 1",
		"Database":          "dev",
		"ClusterIdentifier": "my-cluster",
	}

	rec1 := doRequest(t, h, "ExecuteStatement", req)
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))

	rec2 := doRequest(t, h, "ExecuteStatement", req)
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))

	assert.NotEqual(t, out1["Id"], out2["Id"], "without a ClientToken each call must create a distinct statement")
}

// TestBatchExecuteStatement_ClientToken_ReplaysOnRetry mirrors the
// ExecuteStatement idempotency behavior for BatchExecuteStatement, which
// carries the identical ClientToken member on the wire
// (api_op_BatchExecuteStatement.go).
func TestBatchExecuteStatement_ClientToken_ReplaysOnRetry(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	req := map[string]any{
		"Sqls":              []string{"SELECT 1", "SELECT 2"},
		"Database":          "dev",
		"ClientToken":       "batch-retry-token-1",
		"ClusterIdentifier": "my-cluster",
	}

	rec1 := doRequest(t, h, "BatchExecuteStatement", req)
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))

	rec2 := doRequest(t, h, "BatchExecuteStatement", req)
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))

	assert.Equal(t, out1["Id"], out2["Id"], "retry with the same ClientToken must replay the same batch statement Id")
}
