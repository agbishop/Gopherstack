package redshiftdata_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshiftdata"
)

func TestHandler_ExecuteStatement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name: "success",
			body: map[string]any{
				"Sql":               "SELECT 1",
				"ClusterIdentifier": "my-cluster",
				"Database":          "testdb",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name: "success_with_workgroup",
			body: map[string]any{
				"Sql":           "SELECT 2",
				"WorkgroupName": "my-workgroup",
				"Database":      "testdb",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name:       "missing_sql",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid_json",
			body: nil,
			// empty body is treated as invalid
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "ExecuteStatement", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["Id"])
			}
		})
	}
}

func TestHandler_BatchExecuteStatement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name: "success",
			body: map[string]any{
				"Sqls":              []string{"SELECT 1", "SELECT 2"},
				"ClusterIdentifier": "my-cluster",
				"Database":          "testdb",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name:       "missing_sqls",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "BatchExecuteStatement", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["Id"])
			}
		})
	}
}

func TestHandler_DescribeStatement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*redshiftdata.Handler) string
		name        string
		requestID   string
		wantStatus2 string
		wantErrType string
		wantStatus  int
	}{
		{
			name: "existing_statement",
			setup: func(h *redshiftdata.Handler) string {
				rec := doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql":               "SELECT 1",
					"ClusterIdentifier": "my-cluster",
					"Database":          "testdb",
				})
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["Id"].(string)
			},
			wantStatus:  http.StatusOK,
			wantStatus2: "FINISHED",
		},
		{
			name:        "not_found",
			setup:       func(_ *redshiftdata.Handler) string { return "nonexistent-id" },
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ResourceNotFoundException",
		},
		{
			name:        "missing_id",
			setup:       func(_ *redshiftdata.Handler) string { return "" },
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id := tt.setup(h)
			body := map[string]any{"Id": id}

			if id == "" {
				body = map[string]any{}
			}

			rec := doRequest(t, h, "DescribeStatement", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			if tt.wantStatus2 != "" {
				assert.Equal(t, tt.wantStatus2, resp["Status"])
			}

			if tt.wantErrType != "" {
				assert.Equal(t, tt.wantErrType, resp["__type"])
			}
		})
	}
}

func TestHandler_GetStatementResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*redshiftdata.Handler) string
		name       string
		wantStatus int
	}{
		{
			name: "existing_statement",
			setup: func(h *redshiftdata.Handler) string {
				rec := doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql":               "SELECT 1",
					"ClusterIdentifier": "my-cluster",
					"Database":          "testdb",
				})
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["Id"].(string)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			setup:      func(_ *redshiftdata.Handler) string { return "nonexistent-id" },
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id := tt.setup(h)
			rec := doRequest(t, h, "GetStatementResult", map[string]any{"Id": id})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotNil(t, resp["Records"])
			}
		})
	}
}

func TestHandler_GetStatementResultV2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*redshiftdata.Handler) string
		name         string
		wantErrType  string
		wantStatus   int
		checkRecords bool
	}{
		{
			name: "existing_statement",
			setup: func(h *redshiftdata.Handler) string {
				rec := doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql":               "SELECT 1",
					"ClusterIdentifier": "my-cluster",
					"Database":          "testdb",
					"ResultFormat":      "CSV",
				})
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["Id"].(string)
			},
			wantStatus:   http.StatusOK,
			checkRecords: true,
		},
		{
			name:        "not_found",
			setup:       func(_ *redshiftdata.Handler) string { return "nonexistent-id" },
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ResourceNotFoundException",
		},
		{
			name:        "missing_id",
			setup:       func(_ *redshiftdata.Handler) string { return "" },
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id := tt.setup(h)

			var body map[string]any
			if id != "" {
				body = map[string]any{"Id": id}
			} else {
				body = map[string]any{}
			}

			rec := doRequest(t, h, "GetStatementResultV2", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			if tt.checkRecords {
				assert.NotNil(t, resp["Records"])
				assert.Equal(t, "CSV", resp["ResultFormat"])
				assert.NotNil(t, resp["ColumnMetadata"])
				assert.EqualValues(t, 1, resp["TotalNumRows"])
			} else {
				assert.Equal(t, tt.wantErrType, resp["__type"])
			}
		})
	}
}

func TestHandler_ListStatements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		stmts      int
		wantStatus int
		wantCount  int
	}{
		{
			name:       "empty_list",
			stmts:      0,
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "with_statements",
			stmts:      2,
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for i := range tt.stmts {
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql":               "SELECT " + string(rune('0'+i)),
					"ClusterIdentifier": "my-cluster",
					"Database":          "testdb",
				})
			}

			rec := doRequest(t, h, "ListStatements", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			stmts, ok := resp["Statements"].([]any)
			require.True(t, ok)
			assert.Len(t, stmts, tt.wantCount)
		})
	}
}

func TestHandler_CancelStatement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*redshiftdata.Handler) string
		name        string
		wantErrType string
		wantStatus  int
	}{
		{
			name:        "not_found",
			setup:       func(_ *redshiftdata.Handler) string { return "nonexistent-id" },
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ResourceNotFoundException",
		},
		{
			name:        "missing_id",
			setup:       func(_ *redshiftdata.Handler) string { return "" },
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id := tt.setup(h)

			var body map[string]any
			if id != "" {
				body = map[string]any{"Id": id}
			} else {
				body = map[string]any{}
			}

			rec := doRequest(t, h, "CancelStatement", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantErrType, resp["__type"])
		})
	}
}

func TestHandler_CancelStatement_AlreadyFinished(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create and immediately finish a statement.
	rec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"ClusterIdentifier": "my-cluster",
		"Database":          "testdb",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	id := resp["Id"].(string)

	// Cancelling a FINISHED statement should return an error.
	cancelRec := doRequest(t, h, "CancelStatement", map[string]any{"Id": id})
	assert.Equal(t, http.StatusBadRequest, cancelRec.Code)

	var cancelResp map[string]any
	require.NoError(t, json.Unmarshal(cancelRec.Body.Bytes(), &cancelResp))
	assert.Equal(t, "ValidationException", cancelResp["__type"])
}

func TestHandler_ListStatements_WithFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create statements for two different clusters.
	doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"ClusterIdentifier": "cluster-a",
		"Database":          "testdb",
	})
	doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 2",
		"ClusterIdentifier": "cluster-b",
		"Database":          "testdb",
	})

	// Filter by cluster-a.
	rec := doRequest(t, h, "ListStatements", map[string]any{
		"ClusterIdentifier": "cluster-a",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts := resp["Statements"].([]any)
	assert.Len(t, stmts, 1)
}

func TestHandler_ListStatements_WithWorkgroupFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":           "SELECT 1",
		"WorkgroupName": "wg-a",
		"Database":      "testdb",
	})
	doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":           "SELECT 2",
		"WorkgroupName": "wg-b",
		"Database":      "testdb",
	})

	rec := doRequest(t, h, "ListStatements", map[string]any{
		"WorkgroupName": "wg-a",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts := resp["Statements"].([]any)
	assert.Len(t, stmts, 1)
}

func TestHandler_DescribeStatement_AllFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a statement with all optional fields set.
	rec := doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"ClusterIdentifier": "my-cluster",
		"Database":          "testdb",
		"DbUser":            "myuser",
		"SecretArn":         "arn:aws:secretsmanager:us-east-1:000000000000:secret:mysecret",
		"StatementName":     "my-statement",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	assert.Equal(t, id, resp["Id"])
	assert.Equal(t, "FINISHED", resp["Status"])
	assert.Equal(t, "my-cluster", resp["ClusterIdentifier"])
	assert.Equal(t, "testdb", resp["Database"])
	assert.Equal(t, "myuser", resp["DbUser"])
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:000000000000:secret:mysecret", resp["SecretArn"])
	// StatementName is a real StatementData (ListStatements item) member, not
	// a DescribeStatementOutput one -- confirmed against
	// aws-sdk-go-v2/service/redshiftdata@v1.43.4's DescribeStatementOutput
	// struct, which has no StatementName field at all.
	assert.NotContains(t, resp, "StatementName")
}

func TestHandler_BatchExecuteStatement_AllFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "BatchExecuteStatement", map[string]any{
		"Sqls":              []string{"SELECT 1", "SELECT 2"},
		"ClusterIdentifier": "my-cluster",
		"Database":          "testdb",
		"StatementName":     "my-batch",
		"SecretArn":         "arn:aws:secretsmanager:us-east-1:000000000000:secret:mysecret",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	id := createResp["Id"].(string)

	descRec := doRequest(t, h, "DescribeStatement", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))

	assert.Equal(t, id, resp["Id"])
	// IsBatchStatement/QueryStrings are real StatementData (ListStatements
	// item) members, not DescribeStatementOutput ones -- confirmed against
	// aws-sdk-go-v2/service/redshiftdata@v1.43.4's DescribeStatementOutput
	// struct, which has neither field.
	assert.NotContains(t, resp, "IsBatchStatement")
	assert.NotContains(t, resp, "QueryStrings")

	listRec := doRequest(t, h, "ListStatements", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp struct {
		Statements []map[string]any `json:"Statements"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	require.NotEmpty(t, listResp.Statements)

	assert.Equal(t, true, listResp.Statements[0]["IsBatchStatement"])
	queryStrings, ok := listResp.Statements[0]["QueryStrings"].([]any)
	require.True(t, ok)
	assert.Len(t, queryStrings, 2)
}

func TestHandler_ListStatements_WithSecretARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "ExecuteStatement", map[string]any{
		"Sql":               "SELECT 1",
		"ClusterIdentifier": "my-cluster",
		"Database":          "testdb",
		"SecretArn":         "arn:aws:secretsmanager:us-east-1:000000000000:secret:mysecret",
		"StatementName":     "named-stmt",
	})

	rec := doRequest(t, h, "ListStatements", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	stmts := resp["Statements"].([]any)
	require.Len(t, stmts, 1)

	stmt := stmts[0].(map[string]any)
	assert.Equal(t, "named-stmt", stmt["StatementName"])
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:000000000000:secret:mysecret", stmt["SecretArn"])
}

func TestHandler_ListSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*testing.T, *redshiftdata.Handler)
		check       func(*testing.T, []any)
		body        map[string]any
		name        string
		wantErrType string
		wantStatus  int
		wantCount   int
	}{
		{
			name:       "no_sessions",
			setup:      func(*testing.T, *redshiftdata.Handler) {},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "statement_without_session_id_is_not_a_session",
			setup: func(t *testing.T, h *redshiftdata.Handler) {
				t.Helper()
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 1", "ClusterIdentifier": "my-cluster", "Database": "testdb",
				})
			},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "groups_statements_by_session_id",
			setup: func(t *testing.T, h *redshiftdata.Handler) {
				t.Helper()
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 1", "ClusterIdentifier": "my-cluster", "Database": "testdb", "SessionId": "sess-1",
				})
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 2", "ClusterIdentifier": "my-cluster", "Database": "testdb", "SessionId": "sess-1",
				})
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql":               "SELECT 3",
					"ClusterIdentifier": "other-cluster",
					"Database":          "testdb",
					"SessionId":         "sess-2",
				})
			},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  2,
			check: func(t *testing.T, sessions []any) {
				t.Helper()

				for _, raw := range sessions {
					item, ok := raw.(map[string]any)
					require.True(t, ok)

					if item["SessionId"] != "sess-1" {
						continue
					}

					assert.Equal(t, "AVAILABLE", item["Status"])
					assert.Equal(t, "my-cluster", item["ClusterIdentifier"])
					assert.Equal(t, "testdb", item["Database"])
					assert.NotNil(t, item["CreatedAt"])
					assert.NotNil(t, item["UpdatedAt"])
					assert.Nil(t, item["WorkgroupName"])
				}
			},
		},
		{
			name: "filter_by_session_id",
			setup: func(t *testing.T, h *redshiftdata.Handler) {
				t.Helper()
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 1", "Database": "testdb", "ClusterIdentifier": "c1", "SessionId": "sess-a",
				})
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 2", "Database": "testdb", "ClusterIdentifier": "c2", "SessionId": "sess-b",
				})
			},
			body:       map[string]any{"SessionId": "sess-a"},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "filter_by_cluster_identifier",
			setup: func(t *testing.T, h *redshiftdata.Handler) {
				t.Helper()
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 1", "Database": "testdb", "ClusterIdentifier": "cluster-a", "SessionId": "sess-a",
				})
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 2", "Database": "testdb", "ClusterIdentifier": "cluster-b", "SessionId": "sess-b",
				})
			},
			body:       map[string]any{"ClusterIdentifier": "cluster-a"},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "filter_by_workgroup_name",
			setup: func(t *testing.T, h *redshiftdata.Handler) {
				t.Helper()
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 1", "Database": "testdb", "WorkgroupName": "wg-a", "SessionId": "sess-a",
				})
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 2", "Database": "testdb", "WorkgroupName": "wg-b", "SessionId": "sess-b",
				})
			},
			body:       map[string]any{"WorkgroupName": "wg-a"},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "filter_by_database",
			setup: func(t *testing.T, h *redshiftdata.Handler) {
				t.Helper()
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 1", "Database": "db-a", "ClusterIdentifier": "c", "SessionId": "sess-a",
				})
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 2", "Database": "db-b", "ClusterIdentifier": "c", "SessionId": "sess-b",
				})
			},
			body:       map[string]any{"Database": "db-a"},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:        "session_id_combined_with_status_rejected",
			setup:       func(*testing.T, *redshiftdata.Handler) {},
			body:        map[string]any{"SessionId": "sess-a", "Status": "AVAILABLE"},
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ValidationException",
		},
		{
			name:        "cluster_identifier_and_workgroup_both_set_rejected",
			setup:       func(*testing.T, *redshiftdata.Handler) {},
			body:        map[string]any{"ClusterIdentifier": "c1", "WorkgroupName": "wg1"},
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ValidationException",
		},
		{
			name:        "invalid_status_rejected",
			setup:       func(*testing.T, *redshiftdata.Handler) {},
			body:        map[string]any{"Status": "BOGUS"},
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ValidationException",
		},
		{
			name: "invalid_next_token_rejected",
			setup: func(t *testing.T, h *redshiftdata.Handler) {
				t.Helper()
				doRequest(t, h, "ExecuteStatement", map[string]any{
					"Sql": "SELECT 1", "Database": "testdb", "ClusterIdentifier": "c", "SessionId": "sess-a",
				})
			},
			body:        map[string]any{"NextToken": "does-not-exist"},
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(t, h)

			rec := doRequest(t, h, "ListSessions", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			if tt.wantErrType != "" {
				assert.Equal(t, tt.wantErrType, resp["__type"])

				return
			}

			sessions, ok := resp["Sessions"].([]any)
			require.True(t, ok)
			assert.Len(t, sessions, tt.wantCount)

			if tt.check != nil {
				tt.check(t, sessions)
			}
		})
	}
}

func TestInMemoryBackend_StatementCap_OldestEvicted(t *testing.T) {
	t.Parallel()

	backend := redshiftdata.NewInMemoryBackend(testAccountID, testRegion)

	// Create exactly the cap worth of statements.
	var firstID string
	for i := range redshiftdata.MaxStatementHistoryForTest {
		stmt, err := backend.ExecuteStatement(context.Background(),
			"SELECT 1", "cluster", "", "db", "", "", "", false,
			"", nil, "",
		)
		require.NoError(t, err)
		if i == 0 {
			firstID = stmt.ID
		}
	}

	require.Equal(t, redshiftdata.MaxStatementHistoryForTest, backend.StatementCount())

	// The first statement is still present before overflow.
	_, err := backend.DescribeStatement(context.Background(), firstID)
	require.NoError(t, err)

	// One more statement pushes the oldest out.
	_, err = backend.ExecuteStatement(
		context.Background(), "SELECT 2", "cluster", "", "db", "", "", "", false, "", nil, "",
	)
	require.NoError(t, err)

	assert.LessOrEqual(t, backend.StatementCount(), redshiftdata.MaxStatementHistoryForTest)

	// The first statement is now evicted.
	_, err = backend.DescribeStatement(context.Background(), firstID)
	require.Error(t, err)
}
