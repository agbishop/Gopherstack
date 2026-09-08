package timestreamquery_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/timestreamquery"
)

// clone deep-copies the top-level string→any map for test isolation.
func clone(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	maps.Copy(out, m)

	return out
}

func TestTimestreamQueryHandler_ScheduledQueryLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sqName   string
		wantCode int
	}{
		{
			name:     "create_describe_delete",
			sqName:   "test-query-1",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()

			createBody := map[string]any{
				"Name":                           tt.sqName,
				"QueryString":                    "SELECT * FROM my_db.my_table",
				"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123456789012:role/test-role",
				"ScheduleConfiguration": map[string]any{
					"ScheduleExpression": "rate(1 hour)",
				},
				"NotificationConfiguration": map[string]any{
					"SnsConfiguration": map[string]any{
						"TopicArn": "arn:aws:sns:us-east-1:123456789012:test-topic",
					},
				},
				"ErrorReportConfiguration": map[string]any{
					"S3Configuration": map[string]any{
						"BucketName": "my-error-bucket",
					},
				},
			}

			// Create
			rec := doRequest(t, h, "CreateScheduledQuery", createBody)
			require.Equal(t, tt.wantCode, rec.Code)
			resp := parseResponse(t, rec)
			arn, ok := resp["Arn"].(string)
			require.True(t, ok, "Arn should be a string")
			require.NotEmpty(t, arn)

			// Describe
			rec = doRequest(t, h, "DescribeScheduledQuery", map[string]any{"ScheduledQueryArn": arn})
			require.Equal(t, http.StatusOK, rec.Code)
			resp = parseResponse(t, rec)
			sq, ok := resp["ScheduledQuery"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.sqName, sq["Name"])

			// List
			rec = doRequest(t, h, "ListScheduledQueries", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)
			resp = parseResponse(t, rec)
			queries, ok := resp["ScheduledQueries"].([]any)
			require.True(t, ok)
			assert.Len(t, queries, 1)

			// Update state
			rec = doRequest(t, h, "UpdateScheduledQuery", map[string]any{
				"ScheduledQueryArn": arn,
				"State":             "DISABLED",
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			// Execute
			rec = doRequest(t, h, "ExecuteScheduledQuery", map[string]any{
				"ScheduledQueryArn": arn,
				"InvocationTime":    float64(1704067200), // 2024-01-01T00:00:00Z
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			// Delete
			rec = doRequest(t, h, "DeleteScheduledQuery", map[string]any{"ScheduledQueryArn": arn})
			assert.Equal(t, http.StatusOK, rec.Code)

			// List after delete - should be empty
			rec = doRequest(t, h, "ListScheduledQueries", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)
			resp = parseResponse(t, rec)
			queries, ok = resp["ScheduledQueries"].([]any)
			require.True(t, ok)
			assert.Empty(t, queries)
		})
	}
}

func TestTimestreamQueryHandler_CreateScheduledQuery_Duplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{
			name:     "duplicate name returns conflict",
			wantCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()

			createBody := map[string]any{
				"Name":                           "dup-query",
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123456789012:role/role",
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
				"NotificationConfiguration": map[string]any{
					"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
				},
				"ErrorReportConfiguration": map[string]any{
					"S3Configuration": map[string]any{"BucketName": "bucket"},
				},
			}

			rec := doRequest(t, h, "CreateScheduledQuery", createBody)
			require.Equal(t, http.StatusOK, rec.Code)

			rec = doRequest(t, h, "CreateScheduledQuery", createBody)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestTimestreamQueryHandler_CreateScheduledQuery_ClientTokenIdempotent verifies
// that a repeated CreateScheduledQuery request carrying the same ClientToken
// replays the original success (same Arn, HTTP 200) instead of returning
// ConflictException. The aws-sdk-go-v2 client auto-generates a ClientToken on
// every call when one isn't supplied, so this is the behavior a real client
// retry would rely on after a lost response.
func TestTimestreamQueryHandler_CreateScheduledQuery_ClientTokenIdempotent(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	createBody := map[string]any{
		"Name":                           "retry-query",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123456789012:role/role",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "bucket"},
		},
		"ClientToken": "retry-token",
	}

	rec := doRequest(t, h, "CreateScheduledQuery", createBody)
	require.Equal(t, http.StatusOK, rec.Code)
	firstArn := parseResponse(t, rec)["Arn"]

	// Same ClientToken, same body: must replay, not conflict.
	rec = doRequest(t, h, "CreateScheduledQuery", createBody)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, firstArn, parseResponse(t, rec)["Arn"])

	// Same Name but no ClientToken at all: genuine duplicate, must conflict.
	withoutToken := map[string]any{}
	maps.Copy(withoutToken, createBody)
	delete(withoutToken, "ClientToken")

	rec = doRequest(t, h, "CreateScheduledQuery", withoutToken)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// TestInMemoryBackend_CreateScheduledQuery_ClientTokenIdempotent verifies that
// CreateScheduledQuery replays the original success when called again with the
// same ClientToken, instead of surfacing ConflictException. The aws-sdk-go-v2
// client auto-generates a ClientToken on every CreateScheduledQuery call (see
// idempotencyToken_initializeOpCreateScheduledQuery), so a retried request
// after a lost response (e.g. a network blip) must not error just because the
// resource already exists under that token.
func TestInMemoryBackend_CreateScheduledQuery_ClientTokenIdempotent(t *testing.T) {
	t.Parallel()

	backend := timestreamquery.NewInMemoryBackend("123456789012", "us-east-1")

	first, err := backend.CreateScheduledQuery(
		t.Context(), "idempotent-sq", "SELECT 1", "rate(1 hour)", "arn:aws:iam::123456789012:role/r",
		"", "", "", "", "retry-token-1", "", "", "", nil, nil,
	)
	require.NoError(t, err)

	// A second call with the SAME ClientToken (and, notably, a different
	// QueryString -- simulating a client that doesn't realise the first
	// attempt already succeeded) must replay the original result rather than
	// erroring.
	second, err := backend.CreateScheduledQuery(
		t.Context(), "idempotent-sq", "SELECT 2", "rate(2 hours)", "arn:aws:iam::123456789012:role/other",
		"", "", "", "", "retry-token-1", "", "", "", nil, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, first.Arn, second.Arn)
	assert.Equal(t, "SELECT 1", second.QueryString, "replay must return the ORIGINAL query, not the retry's")
	assert.Equal(t, 1, timestreamquery.ScheduledQueryCount(backend))

	// A different ClientToken for the same Name is a genuine conflict.
	_, err = backend.CreateScheduledQuery(
		t.Context(), "idempotent-sq", "SELECT 3", "rate(1 hour)", "arn:aws:iam::123456789012:role/r",
		"", "", "", "", "different-token", "", "", "", nil, nil,
	)
	require.Error(t, err)

	// No ClientToken at all: repeated calls are NOT deduplicated and hit the
	// normal "already exists" conflict path.
	_, err = backend.CreateScheduledQuery(
		t.Context(), "no-token-sq", "SELECT 1", "rate(1 hour)", "arn:aws:iam::123456789012:role/r",
		"", "", "", "", "", "", "", "", nil, nil,
	)
	require.NoError(t, err)

	_, err = backend.CreateScheduledQuery(
		t.Context(), "no-token-sq", "SELECT 1", "rate(1 hour)", "arn:aws:iam::123456789012:role/r",
		"", "", "", "", "", "", "", "", nil, nil,
	)
	require.Error(t, err)
}

func TestTimestreamQueryHandler_DescribeScheduledQuery_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		arn      string
		wantCode int
	}{
		{
			name:     "not found",
			arn:      "arn:aws:timestream:us-east-1:123456789012:scheduled-query/nonexistent",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()
			rec := doRequest(t, h, "DescribeScheduledQuery", map[string]any{"ScheduledQueryArn": tt.arn})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestTimestreamQueryBackend_ListScheduledQueriesFull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queries   []string
		wantCount int
	}{
		{
			name:      "empty backend",
			queries:   []string{},
			wantCount: 0,
		},
		{
			name:      "single query",
			queries:   []string{"q1"},
			wantCount: 1,
		},
		{
			name:      "multiple queries sorted by name",
			queries:   []string{"zeta", "alpha", "beta"},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend, h := newTestBackendAndHandler()

			for _, name := range tt.queries {
				createBody := map[string]any{
					"Name":                           name,
					"QueryString":                    "SELECT 1",
					"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
					"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
					"NotificationConfiguration": map[string]any{
						"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:t"},
					},
					"ErrorReportConfiguration": map[string]any{
						"S3Configuration": map[string]any{"BucketName": "b"},
					},
				}

				rec := doRequest(t, h, "CreateScheduledQuery", createBody)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			queries := backend.ListScheduledQueriesFull(t.Context())
			assert.Len(t, queries, tt.wantCount)

			if len(queries) > 1 {
				for i := 1; i < len(queries); i++ {
					assert.LessOrEqual(t, queries[i-1].Name, queries[i].Name)
				}
			}
		})
	}
}

func TestTimestreamQueryHandler_ScheduledQueryValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		op       string
		wantCode int
	}{
		{
			name:     "create - missing name",
			op:       "CreateScheduledQuery",
			body:     map[string]any{"QueryString": "SELECT 1"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "delete - missing arn",
			op:       "DeleteScheduledQuery",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "describe - missing arn",
			op:       "DescribeScheduledQuery",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "execute - missing arn",
			op:       "ExecuteScheduledQuery",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "execute - missing invocation time",
			op:       "ExecuteScheduledQuery",
			body:     map[string]any{"ScheduledQueryArn": "arn:aws:timestream:us-east-1:123:scheduled-query/q"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "update - missing arn",
			op:       "UpdateScheduledQuery",
			body:     map[string]any{"State": "ENABLED"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "update - missing state",
			op:       "UpdateScheduledQuery",
			body:     map[string]any{"ScheduledQueryArn": "arn:aws:timestream:us-east-1:123:scheduled-query/q"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()
			rec := doRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestTimestreamQueryHandler_DeleteAndExecute_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		op       string
		arn      string
		wantCode int
	}{
		{
			name:     "delete not found",
			op:       "DeleteScheduledQuery",
			arn:      "arn:aws:timestream:us-east-1:123:scheduled-query/nonexistent",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "execute not found",
			op:       "ExecuteScheduledQuery",
			arn:      "arn:aws:timestream:us-east-1:123:scheduled-query/nonexistent",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "update not found",
			op:       "UpdateScheduledQuery",
			arn:      "arn:aws:timestream:us-east-1:123:scheduled-query/nonexistent",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()

			body := map[string]any{"ScheduledQueryArn": tt.arn}
			if tt.op == "UpdateScheduledQuery" {
				body["State"] = "DISABLED"
			}

			if tt.op == "ExecuteScheduledQuery" {
				body["InvocationTime"] = float64(1704067200)
			}

			rec := doRequest(t, h, tt.op, body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestCreateScheduledQuery_ScheduleExpressionValidation — gap #23.
func TestCreateScheduledQuery_ScheduleExpressionValidation(t *testing.T) {
	t.Parallel()

	base := map[string]any{
		"Name":                           "test-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration": map[string]any{
			"ScheduleExpression": "PLACEHOLDER",
		},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	}

	tests := []struct {
		name     string
		expr     string
		wantCode int
	}{
		{"rate minutes valid", "rate(5 minutes)", http.StatusOK},
		{"rate hour valid", "rate(1 hour)", http.StatusOK},
		{"rate days valid", "rate(3 days)", http.StatusOK},
		{"cron 6 fields valid", "cron(0 12 * * ? *)", http.StatusOK},
		{"cron 5 fields invalid", "cron(0 12 * * ?)", http.StatusBadRequest},
		{"arbitrary string invalid", "every 5 minutes", http.StatusBadRequest},
		{"empty invalid", "", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()
			body := clone(base)
			if tt.expr == "" {
				body["ScheduleConfiguration"] = map[string]any{"ScheduleExpression": ""}
			} else {
				body["ScheduleConfiguration"] = map[string]any{"ScheduleExpression": tt.expr}
			}
			rec := doRequest(t, h, "CreateScheduledQuery", body)
			assert.Equal(t, tt.wantCode, rec.Code, "expr=%q", tt.expr)
		})
	}
}

// TestValidateScheduleExpression exercises the backend-level validator
// directly across the full matrix of valid/invalid schedule expressions.
func TestValidateScheduleExpression(t *testing.T) {
	t.Parallel()

	backend := timestreamquery.NewInMemoryBackend("123", "us-east-1")

	validExprs := []string{
		"rate(1 minute)",
		"rate(5 minutes)",
		"rate(2 hours)",
		"rate(7 days)",
		"cron(0 12 * * ? *)",
		"cron(0/5 * * * ? *)",
	}
	invalidExprs := []string{
		"",
		"every 5 minutes",
		"cron(0 12 * * ?)", // only 5 fields
		"cron()",           // empty
		"rate()",
		"rate(five minutes)",
	}

	for _, expr := range validExprs {
		_, err := backend.CreateScheduledQuery(
			t.Context(), "valid-"+expr[:4], "SELECT 1", expr, "arn", "", "", "", "", "", "", "", "", nil, nil,
		)
		require.NoError(t, err, "valid expr %q should be accepted", expr)
		_ = backend.DeleteScheduledQuery(
			t.Context(),
			"arn:aws:timestream:us-east-1:123:scheduled-query/valid-"+expr[:4],
		)
	}

	for _, expr := range invalidExprs {
		_, err := backend.CreateScheduledQuery(
			t.Context(), "inv", "SELECT 1", expr, "arn", "", "", "", "", "", "", "", "", nil, nil,
		)
		require.Error(t, err, "invalid expr %q should be rejected", expr)
	}
}

// TestCreateScheduledQuery_ConflictReturns409 — gap #25.
func TestCreateScheduledQuery_ConflictReturns409(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	body := map[string]any{
		"Name":                           "dup-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	}

	rec1 := doRequest(t, h, "CreateScheduledQuery", body)
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doRequest(t, h, "CreateScheduledQuery", body)
	assert.Equal(t, http.StatusConflict, rec2.Code)

	var errBody map[string]string
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &errBody))
	assert.Equal(t, "ConflictException", errBody["__type"])
}

// TestCreateScheduledQuery_RequiresNotificationAndErrorReport verifies that
// CreateScheduledQuery rejects requests missing NotificationConfiguration
// or ErrorReportConfiguration. Real AWS enforces both as required fields;
// the emulator previously accepted them as optional and stored empty values.
func TestCreateScheduledQuery_RequiresNotificationAndErrorReport(t *testing.T) {
	t.Parallel()

	fullBody := map[string]any{
		"Name":                           "parity-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123456789012:role/role",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	}

	tests := []struct {
		name     string
		omitKey  string
		wantCode int
	}{
		{
			name:     "missing_notification_configuration",
			omitKey:  "NotificationConfiguration",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing_error_report_configuration",
			omitKey:  "ErrorReportConfiguration",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "all_required_fields_present",
			omitKey:  "",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			body := clone(fullBody)

			if tt.omitKey != "" {
				delete(body, tt.omitKey)
			}

			rec := doRequest(t, h, "CreateScheduledQuery", body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateScheduledQuery status for case %q", tt.name)

			if tt.wantCode == http.StatusBadRequest {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.NotEmpty(t, errResp["__type"], "error response must include __type")
			}
		})
	}
}

// TestCreateScheduledQuery_RequiredFields verifies required field validation.
func TestCreateScheduledQuery_RequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "missing name",
			body: map[string]any{
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:...",
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing query string",
			body: map[string]any{
				"Name":                           "q",
				"ScheduledQueryExecutionRoleArn": "arn:...",
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing execution role arn",
			body: map[string]any{
				"Name":                  "q",
				"QueryString":           "SELECT 1",
				"ScheduleConfiguration": map[string]any{"ScheduleExpression": "rate(1 hour)"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing schedule expression",
			body: map[string]any{
				"Name":                           "q",
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:...",
				"ScheduleConfiguration":          map[string]any{},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing notification configuration",
			body: map[string]any{
				"Name":                           "q",
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
				"ErrorReportConfiguration": map[string]any{
					"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing error report configuration",
			body: map[string]any{
				"Name":                           "q",
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
				"NotificationConfiguration": map[string]any{
					"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "all required fields provided",
			body: map[string]any{
				"Name":                           "q",
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
				"NotificationConfiguration": map[string]any{
					"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
				},
				"ErrorReportConfiguration": map[string]any{
					"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
				},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doRequest(t, h, "CreateScheduledQuery", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestUpdateScheduledQuery_InvalidState verifies state validation.
func TestUpdateScheduledQuery_InvalidState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		state     string
		wantCode  int
		wantError bool
	}{
		{name: "ENABLED is valid", state: "ENABLED", wantCode: http.StatusOK},
		{name: "DISABLED is valid", state: "DISABLED", wantCode: http.StatusOK},
		{name: "PAUSED is invalid", state: "PAUSED", wantCode: http.StatusBadRequest, wantError: true},
		{name: "lowercase is invalid", state: "enabled", wantCode: http.StatusBadRequest, wantError: true},
		{name: "empty is invalid", state: "", wantCode: http.StatusBadRequest, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, h := newTestBackendAndHandler()

			// Create a scheduled query first.
			rec := doRequest(t, h, "CreateScheduledQuery", map[string]any{
				"Name":                           "state-test",
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
				"NotificationConfiguration": map[string]any{
					"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
				},
				"ErrorReportConfiguration": map[string]any{
					"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			arn := parseResponse(t, rec)["Arn"].(string)

			rec = doRequest(t, h, "UpdateScheduledQuery", map[string]any{
				"ScheduledQueryArn": arn,
				"State":             tt.state,
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestDescribeScheduledQuery_DeepCopy verifies that mutating the returned
// ScheduledQuery does not affect the stored copy.
func TestDescribeScheduledQuery_DeepCopy(t *testing.T) {
	t.Parallel()

	backend, h := newTestBackendAndHandler()

	rec := doRequest(t, h, "CreateScheduledQuery", map[string]any{
		"Name":                           "copy-test",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"Tags":                           []map[string]string{{"Key": "env", "Value": "prod"}},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	arn := parseResponse(t, rec)["Arn"].(string)

	// Get a copy and mutate its Tags.
	sq, err := backend.DescribeScheduledQuery(t.Context(), arn)
	require.NoError(t, err)
	sq.Tags["env"] = "mutated"

	// The stored query should be unaffected.
	sq2, err := backend.DescribeScheduledQuery(t.Context(), arn)
	require.NoError(t, err)
	assert.Equal(t, "prod", sq2.Tags["env"], "mutation of returned copy must not affect stored state")
}

// TestScheduledQueryToView_OmitsTags verifies DescribeScheduledQuery does NOT
// echo a Tags field: types.ScheduledQueryDescription (timestreamquery@v1.39.4
// types/types.go:620) has no Tags member, and
// awsAwsjson10_deserializeDocumentScheduledQueryDescription's case list
// (deserializers.go) has no "Tags" case either -- a real client can never
// receive one on this response.
func TestScheduledQueryToView_OmitsTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	rec := doRequest(t, h, "CreateScheduledQuery", map[string]any{
		"Name":                           "tagged-describe",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"Tags": []map[string]string{
			{"Key": "team", "Value": "data"},
			{"Key": "env", "Value": "test"},
		},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	arn := parseResponse(t, rec)["Arn"].(string)

	rec = doRequest(t, h, "DescribeScheduledQuery", map[string]any{"ScheduledQueryArn": arn})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResponse(t, rec)
	sq := resp["ScheduledQuery"].(map[string]any)
	_, ok := sq["Tags"]
	assert.False(t, ok, "ScheduledQueryDescription has no Tags member; DescribeScheduledQuery must not echo one")
}

// TestListScheduledQueries_EnrichedResponse — gaps #18, #19.
func TestListScheduledQueries_EnrichedResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	// Create a query so there's something to list.
	createBody := map[string]any{
		"Name":                           "enriched-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"TargetConfiguration": map[string]any{
			"TimestreamConfiguration": map[string]any{
				"DatabaseName": "mydb",
				"TableName":    "mytable",
			},
		},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	}
	rec := doRequest(t, h, "CreateScheduledQuery", createBody)
	require.Equal(t, http.StatusOK, rec.Code)

	// List and verify enriched fields.
	listRec := doRequest(t, h, "ListScheduledQueries", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)

	listResp := parseResponse(t, listRec)
	items, ok := listResp["ScheduledQueries"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)

	item := items[0].(map[string]any)
	assert.Equal(t, "enriched-sq", item["Name"])
	assert.Equal(t, "ENABLED", item["State"])
	assert.NotEmpty(t, item["CreationTime"], "CreationTime should be populated")
	assert.NotEmpty(t, item["NextInvocationTime"], "NextInvocationTime should be derived from schedule")

	// Target destination should be populated.
	dest, hasDest := item["TargetDestination"]
	assert.True(t, hasDest)
	assert.NotNil(t, dest)
}

func TestListScheduledQueries_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	// Create 3 queries.
	for i := range 3 {
		body := map[string]any{
			"Name":                           "paged-sq-" + string(rune('a'+i)),
			"QueryString":                    "SELECT 1",
			"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
			"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
			"NotificationConfiguration": map[string]any{
				"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
			},
			"ErrorReportConfiguration": map[string]any{
				"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
			},
		}
		rec := doRequest(t, h, "CreateScheduledQuery", body)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Page 1: MaxResults=2.
	listRec1 := doRequest(t, h, "ListScheduledQueries", map[string]any{"MaxResults": 2})
	require.Equal(t, http.StatusOK, listRec1.Code)
	resp1 := parseResponse(t, listRec1)

	items1 := resp1["ScheduledQueries"].([]any)
	assert.Len(t, items1, 2)
	assert.NotEmpty(t, resp1["NextToken"], "NextToken must be set when more items remain")

	// Page 2 using NextToken.
	listRec2 := doRequest(t, h, "ListScheduledQueries", map[string]any{
		"NextToken": resp1["NextToken"],
	})
	require.Equal(t, http.StatusOK, listRec2.Code)
	resp2 := parseResponse(t, listRec2)

	items2 := resp2["ScheduledQueries"].([]any)
	assert.Len(t, items2, 1)
	_, hasNext := resp2["NextToken"]
	assert.False(t, hasNext, "No NextToken on last page")
}

// TestListScheduledQueries_TimestampsAreNumbers verifies that the
// ListScheduledQueries response encodes timestamps as JSON numbers (Unix epoch
// seconds), not strings. Real AWS JSON protocol 1.0 always sends timestamps as
// floating-point numbers; the emulator previously serialised them as strings.
func TestListScheduledQueries_TimestampsAreNumbers(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createRec := doRequest(t, h, "CreateScheduledQuery", map[string]any{
		"Name":                           "parity-ts-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	listRec := doRequest(t, h, "ListScheduledQueries", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)

	resp := parseResponse(t, listRec)
	items, ok := resp["ScheduledQueries"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)

	item := items[0].(map[string]any)

	// JSON numbers unmarshal to float64 in Go's map[string]any.
	// Strings would unmarshal to string; either would be non-nil, so we
	// assert the concrete type is float64 (not string).
	ct, hasCT := item["CreationTime"]
	assert.True(t, hasCT, "CreationTime must be present")
	_, ctIsFloat := ct.(float64)
	assert.True(t, ctIsFloat, "CreationTime must be a JSON number (float64), got %T", ct)

	nit, hasNIT := item["NextInvocationTime"]
	assert.True(t, hasNIT, "NextInvocationTime must be present")
	_, nitIsFloat := nit.(float64)
	assert.True(t, nitIsFloat, "NextInvocationTime must be a JSON number (float64), got %T", nit)
}

// TestDescribeScheduledQuery_NextPreviousInvocationTime — gaps #20, #21.
func TestDescribeScheduledQuery_NextPreviousInvocationTime(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	backend := timestreamquery.ExportBackend(h)
	backend.AddScheduledQueryInternal(&timestreamquery.ScheduledQuery{
		Arn:                "arn:aws:timestream:us-east-1:123:scheduled-query/inv-test",
		Name:               "inv-test",
		State:              "ENABLED",
		ScheduleExpression: "rate(1 hour)",
		QueryString:        "SELECT 1",
	})

	rec := doRequest(t, h, "DescribeScheduledQuery", map[string]any{
		"ScheduledQueryArn": "arn:aws:timestream:us-east-1:123:scheduled-query/inv-test",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)

	sq, ok := resp["ScheduledQuery"].(map[string]any)
	require.True(t, ok, "response must have ScheduledQuery key")

	assert.NotEmpty(t, sq["NextInvocationTime"], "NextInvocationTime must be derived from schedule")
	// No previous invocation if never run.
	_, hasPrev := sq["PreviousInvocationTime"]
	assert.False(t, hasPrev)
}

func TestDescribeScheduledQuery_LastRunSummaryFullFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	backend := timestreamquery.ExportBackend(h)

	sq := &timestreamquery.ScheduledQuery{
		Arn:                "arn:aws:timestream:us-east-1:123:scheduled-query/run-test",
		Name:               "run-test",
		State:              "ENABLED",
		ScheduleExpression: "rate(30 minutes)",
		QueryString:        "SELECT 1",
	}
	backend.AddScheduledQueryInternal(sq)

	// Trigger an execution.
	doRequest(t, h, "ExecuteScheduledQuery", map[string]any{
		"ScheduledQueryArn": sq.Arn,
		"InvocationTime":    1715000000.0,
	})

	rec := doRequest(t, h, "DescribeScheduledQuery", map[string]any{
		"ScheduledQueryArn": sq.Arn,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)

	sqView, ok := resp["ScheduledQuery"].(map[string]any)
	require.True(t, ok)

	lastRun, ok := sqView["LastRunSummary"].(map[string]any)
	require.True(t, ok, "LastRunSummary must be present after execution")
	assert.NotEmpty(t, lastRun["RunStatus"])
	assert.NotEmpty(t, lastRun["InvocationTime"])
	assert.NotEmpty(t, lastRun["TriggerTime"])
	assert.NotEmpty(t, sqView["PreviousInvocationTime"])
	assert.NotEmpty(t, sqView["NextInvocationTime"])

	// ExecutionStats' wire field is "ExecutionTimeInMillis" (no trailing
	// "ecs"), per the real aws-sdk-go-v2 deserializer
	// (awsAwsjson10_deserializeDocumentExecutionStats). A prior version of
	// this emulator used "ExecutionTimeInMillisecs", which a real client
	// would silently fail to populate.
	stats, ok := lastRun["ExecutionStats"].(map[string]any)
	require.True(t, ok, "ExecutionStats must be present")
	assert.NotEmpty(t, stats["ExecutionTimeInMillis"])
	_, hasMisspelled := stats["ExecutionTimeInMillisecs"]
	assert.False(t, hasMisspelled, "must not emit the misspelled ExecutionTimeInMillisecs field")
}

// TestDescribeScheduledQuery_LastRunSummaryTimestampsAreNumbers verifies
// that InvocationTime and TriggerTime in LastRunSummary are JSON numbers, not
// strings, matching the AWS JSON protocol 1.0 wire format.
func TestDescribeScheduledQuery_LastRunSummaryTimestampsAreNumbers(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createRec := doRequest(t, h, "CreateScheduledQuery", map[string]any{
		"Name":                           "parity-lrs-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	arnResp := parseResponse(t, createRec)
	arn, _ := arnResp["Arn"].(string)
	require.NotEmpty(t, arn)

	// Execute so that LastRunSummary is populated.
	execRec := doRequest(t, h, "ExecuteScheduledQuery", map[string]any{
		"ScheduledQueryArn": arn,
		"InvocationTime":    1715000000.0,
	})
	require.Equal(t, http.StatusOK, execRec.Code)

	descRec := doRequest(t, h, "DescribeScheduledQuery", map[string]any{
		"ScheduledQueryArn": arn,
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	resp := parseResponse(t, descRec)
	sq, ok := resp["ScheduledQuery"].(map[string]any)
	require.True(t, ok)

	lrs, ok := sq["LastRunSummary"].(map[string]any)
	require.True(t, ok, "LastRunSummary must be present after execution")

	invTime, hasInv := lrs["InvocationTime"]
	assert.True(t, hasInv, "InvocationTime must be present")
	_, invIsFloat := invTime.(float64)
	assert.True(t, invIsFloat, "InvocationTime must be a JSON number (float64), got %T", invTime)

	trigTime, hasTrig := lrs["TriggerTime"]
	assert.True(t, hasTrig, "TriggerTime must be present")
	_, trigIsFloat := trigTime.(float64)
	assert.True(t, trigIsFloat, "TriggerTime must be a JSON number (float64), got %T", trigTime)
}

// TestScheduledQueryCountTrack verifies ScheduledQueryCount increments and decrements.
func TestScheduledQueryCountTrack(t *testing.T) {
	t.Parallel()

	backend, h := newTestBackendAndHandler()

	assert.Equal(t, 0, timestreamquery.ScheduledQueryCount(backend))

	rec := doRequest(t, h, "CreateScheduledQuery", map[string]any{
		"Name":                           "count-track",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, timestreamquery.ScheduledQueryCount(backend))

	arn := parseResponse(t, rec)["Arn"].(string)
	doRequest(t, h, "DeleteScheduledQuery", map[string]any{"ScheduledQueryArn": arn})
	assert.Equal(t, 0, timestreamquery.ScheduledQueryCount(backend))
}

// TestScheduledQueryCount_Export verifies the exported ScheduledQueryCount helper.
func TestScheduledQueryCount_Export(t *testing.T) {
	t.Parallel()

	b := timestreamquery.NewInMemoryBackend("000000000000", "us-east-1")
	assert.Equal(t, 0, timestreamquery.ScheduledQueryCount(b))
}

// TestScheduledQuery_AddInternal verifies the seed helper AddScheduledQueryInternal.
func TestScheduledQuery_AddInternal(t *testing.T) {
	t.Parallel()

	backend := timestreamquery.NewInMemoryBackend("000000000000", "us-east-1")

	sq := &timestreamquery.ScheduledQuery{
		Name:         "seeded-query",
		Arn:          "arn:aws:timestream:us-east-1:000000000000:scheduled-query/seeded-query",
		State:        "ENABLED",
		QueryString:  "SELECT 1",
		CreationTime: time.Now(),
	}

	backend.AddScheduledQueryInternal(sq)
	assert.Equal(t, 1, timestreamquery.ScheduledQueryCount(backend))

	result, err := backend.DescribeScheduledQuery(t.Context(), sq.Arn)
	require.NoError(t, err)
	assert.Equal(t, "seeded-query", result.Name)
}

// TestCreateScheduledQuery_KmsKeyId verifies KmsKeyId (real
// CreateScheduledQueryInput.KmsKeyId / ScheduledQueryDescription.KmsKeyId per
// timestreamquery@v1.39.4's api_op_CreateScheduledQuery.go and types/types.go)
// is accepted on create and echoed back on describe rather than silently
// dropped.
func TestCreateScheduledQuery_KmsKeyId(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantKmsKey any
		name       string
		kmsKeyID   string
	}{
		{
			name:       "kms key id round trips through describe",
			kmsKeyID:   "arn:aws:kms:us-east-1:123456789012:key/test-key",
			wantKmsKey: "arn:aws:kms:us-east-1:123456789012:key/test-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()

			createBody := map[string]any{
				"Name":                           "kms-sq-" + tt.name,
				"QueryString":                    "SELECT 1",
				"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123456789012:role/r",
				"KmsKeyId":                       tt.kmsKeyID,
				"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
				"NotificationConfiguration": map[string]any{
					"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123456789012:topic"},
				},
				"ErrorReportConfiguration": map[string]any{
					"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
				},
			}

			rec := doRequest(t, h, "CreateScheduledQuery", createBody)
			require.Equal(t, http.StatusOK, rec.Code)
			arn, _ := parseResponse(t, rec)["Arn"].(string)
			require.NotEmpty(t, arn)

			descRec := doRequest(t, h, "DescribeScheduledQuery", map[string]any{"ScheduledQueryArn": arn})
			require.Equal(t, http.StatusOK, descRec.Code)
			sq, ok := parseResponse(t, descRec)["ScheduledQuery"].(map[string]any)
			require.True(t, ok)

			assert.Equal(t, tt.wantKmsKey, sq["KmsKeyId"], "KmsKeyId must be echoed back on describe, not dropped")
		})
	}
}

// TestCreateScheduledQuery_KmsKeyIdOmittedWhenAbsent verifies that a
// scheduled query created without a KmsKeyId omits the field on describe
// (wire-safe: ScheduledQueryDescription.KmsKeyId is optional).
func TestCreateScheduledQuery_KmsKeyIdOmittedWhenAbsent(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	createBody := map[string]any{
		"Name":                           "no-kms-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123456789012:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123456789012:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	}

	rec := doRequest(t, h, "CreateScheduledQuery", createBody)
	require.Equal(t, http.StatusOK, rec.Code)
	arn, _ := parseResponse(t, rec)["Arn"].(string)
	require.NotEmpty(t, arn)

	descRec := doRequest(t, h, "DescribeScheduledQuery", map[string]any{"ScheduledQueryArn": arn})
	require.Equal(t, http.StatusOK, descRec.Code)
	sq, ok := parseResponse(t, descRec)["ScheduledQuery"].(map[string]any)
	require.True(t, ok)

	_, present := sq["KmsKeyId"]
	assert.False(t, present, "KmsKeyId must be omitted, not emitted empty, when never configured")
}

// TestExecuteScheduledQuery_RunStatusIsManual verifies that a run produced by
// ExecuteScheduledQuery reports MANUAL_TRIGGER_SUCCESS, not
// AUTO_TRIGGER_SUCCESS. Real AWS's ExecuteScheduledQuery is documented "You
// can use this API to run a scheduled query manually"
// (timestreamquery@v1.39.4 api_op_ExecuteScheduledQuery.go), and this
// emulator has no automatic scheduler -- ExecuteScheduledQuery is the only
// code path that ever populates a run, so claiming AUTO_TRIGGER_SUCCESS
// asserts a background trigger that never happened.
func TestExecuteScheduledQuery_RunStatusIsManual(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	createBody := map[string]any{
		"Name":                           "manual-run-sq",
		"QueryString":                    "SELECT 1",
		"ScheduledQueryExecutionRoleArn": "arn:aws:iam::123456789012:role/r",
		"ScheduleConfiguration":          map[string]any{"ScheduleExpression": "rate(1 hour)"},
		"NotificationConfiguration": map[string]any{
			"SnsConfiguration": map[string]any{"TopicArn": "arn:aws:sns:us-east-1:123456789012:topic"},
		},
		"ErrorReportConfiguration": map[string]any{
			"S3Configuration": map[string]any{"BucketName": "my-errors-bucket"},
		},
	}
	rec := doRequest(t, h, "CreateScheduledQuery", createBody)
	require.Equal(t, http.StatusOK, rec.Code)
	arn, _ := parseResponse(t, rec)["Arn"].(string)
	require.NotEmpty(t, arn)

	execRec := doRequest(t, h, "ExecuteScheduledQuery", map[string]any{
		"ScheduledQueryArn": arn,
		"InvocationTime":    1715000000.0,
	})
	require.Equal(t, http.StatusOK, execRec.Code)

	descRec := doRequest(t, h, "DescribeScheduledQuery", map[string]any{"ScheduledQueryArn": arn})
	require.Equal(t, http.StatusOK, descRec.Code)
	sq, ok := parseResponse(t, descRec)["ScheduledQuery"].(map[string]any)
	require.True(t, ok)

	lastRun, ok := sq["LastRunSummary"].(map[string]any)
	require.True(t, ok, "LastRunSummary must be present after execution")
	assert.Equal(t, "MANUAL_TRIGGER_SUCCESS", lastRun["RunStatus"])

	listRec := doRequest(t, h, "ListScheduledQueries", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)
	items, ok := parseResponse(t, listRec)["ScheduledQueries"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "MANUAL_TRIGGER_SUCCESS", item["LastRunStatus"])
}
