package pipes_test

// Covers TargetParameters for StepFunctions, SQS, Kinesis, CloudWatch Logs,
// EventBridge event bus, Redshift Data API, SageMaker Pipelines, Timestream,
// and HTTP (API Gateway / API destination) targets, plus the generic
// InputTemplate override. ECS and Batch targets (deeper, higher test-count
// families) live in targets_ecs_batch_test.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

// --- b2 helpers (us-east-1 / 123456789012 test account) ---

func b2Backend() *pipes.InMemoryBackend {
	return pipes.NewInMemoryBackend("123456789012", "us-east-1")
}

func b2Handler(t *testing.T) *pipes.Handler {
	t.Helper()

	return pipes.NewHandler(b2Backend())
}

func b2Do(t *testing.T, h *pipes.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		require.NoError(t, err)
	}
	e := echo.New()
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20230101/us-east-1/pipes/aws4_request")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetRequest(req)
	require.NoError(t, h.Handler()(c))

	return rec
}

func b2Create(t *testing.T, h *pipes.Handler, name string, body map[string]any) map[string]any {
	t.Helper()
	rec := b2Do(t, h, http.MethodPost, "/v1/pipes/"+name, body)
	require.Equal(t, http.StatusOK, rec.Code, "create pipe %q: %s", name, rec.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	return resp
}

func b2Describe(t *testing.T, h *pipes.Handler, name string) map[string]any {
	t.Helper()
	rec := b2Do(t, h, http.MethodGet, "/v1/pipes/"+name, nil)
	require.Equal(t, http.StatusOK, rec.Code, "describe pipe %q: %s", name, rec.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	return resp
}

func b2Update(t *testing.T, h *pipes.Handler, name string, body map[string]any) map[string]any {
	t.Helper()
	rec := b2Do(t, h, http.MethodPut, "/v1/pipes/"+name, body)
	require.Equal(t, http.StatusOK, rec.Code, "update pipe %q: %s", name, rec.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	return resp
}

const (
	b2SQSSource    = "arn:aws:sqs:us-east-1:123456789012:queue"
	b2LambdaTarget = "arn:aws:lambda:us-east-1:123456789012:function:fn"
	b2SFNTarget    = "arn:aws:states:us-east-1:123456789012:stateMachine:sm"
	b2ECSTarget    = "arn:aws:ecs:us-east-1:123456789012:cluster/cluster"
)

// nestedFloat extracts a float64 from nested map[string]any.
func nestedFloat(t *testing.T, m map[string]any, keys ...string) float64 {
	t.Helper()
	cur := m
	for i, k := range keys {
		if i == len(keys)-1 {
			v, ok := cur[k]
			require.True(t, ok, "key %q missing", k)
			f, ok := v.(float64)
			require.True(t, ok, "key %q is not float64: %T", k, v)

			return f
		}
		sub, ok := cur[k]
		require.True(t, ok, "intermediate key %q missing", k)
		cur, ok = sub.(map[string]any)
		require.True(t, ok, "intermediate key %q not object: %T", k, sub)
	}

	return 0
}

// --- basic target parameter round-trip tests ---

// TestTargetParams_StepFunctions verifies Step Functions target parameters.
func TestTargetParams_StepFunctions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		invocationType string
	}{
		{name: "fire_and_forget", invocationType: "FIRE_AND_FORGET"},
		{name: "request_response", invocationType: "REQUEST_RESPONSE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			resp := auditCreate(t, h, tt.name+"-sfn-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:states:us-west-2:123456789012:stateMachine:sm",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"StepFunctionStateMachineParameters": map[string]any{
						"InvocationType": tt.invocationType,
					},
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			require.NotNil(t, tp, "TargetParameters missing")
			sfn, _ := tp["StepFunctionStateMachineParameters"].(map[string]any)
			require.NotNil(t, sfn, "StepFunctionStateMachineParameters missing")
			assert.Equal(t, tt.invocationType, sfn["InvocationType"])
		})
	}
}

// TestTargetParams_SQS verifies SQS target parameters.
func TestTargetParams_SQS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		messageGroupID         string
		messageDeduplicationID string
	}{
		{name: "plain_sqs", messageGroupID: "", messageDeduplicationID: ""},
		{name: "fifo_sqs", messageGroupID: "group-1", messageDeduplicationID: "dedup-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			sqsParams := map[string]any{}
			if tt.messageGroupID != "" {
				sqsParams["MessageGroupId"] = tt.messageGroupID
			}
			if tt.messageDeduplicationID != "" {
				sqsParams["MessageDeduplicationId"] = tt.messageDeduplicationID
			}

			resp := auditCreate(t, h, tt.name+"-sqs-target-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:src",
				"Target":       "arn:aws:sqs:us-west-2:123456789012:dst.fifo",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"SqsQueueParameters": sqsParams,
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			sqsp, _ := tp["SqsQueueParameters"].(map[string]any)
			require.NotNil(t, sqsp, "SqsQueueParameters missing")
			if tt.messageGroupID != "" {
				assert.Equal(t, tt.messageGroupID, sqsp["MessageGroupId"])
			}
		})
	}
}

// TestTargetParams_Kinesis verifies Kinesis stream target parameters.
func TestTargetParams_Kinesis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		partitionKey string
	}{
		{name: "static_partition_key", partitionKey: "static"},
		{name: "dynamic_partition_key", partitionKey: "$.id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			resp := auditCreate(t, h, tt.name+"-kinesis-target-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:kinesis:us-west-2:123456789012:stream/out",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"KinesisStreamParameters": map[string]any{
						"PartitionKey": tt.partitionKey,
					},
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			kp, _ := tp["KinesisStreamParameters"].(map[string]any)
			require.NotNil(t, kp, "KinesisStreamParameters missing")
			assert.Equal(t, tt.partitionKey, kp["PartitionKey"])
		})
	}
}

// TestTargetPartitionKey_Required verifies that CreatePipe and UpdatePipe
// reject a Kinesis target with no PartitionKey, matching aws-sdk-go-v2 pipes
// validators.go's validatePipeTargetKinesisStreamParameters (PartitionKey
// required). Unlike source StartingPosition, this applies to both ops: Create
// and Update route TargetParameters through the same validator.
func TestTargetPartitionKey_Required(t *testing.T) {
	t.Parallel()

	t.Run("create_missing_partition_key_rejected", func(t *testing.T) {
		t.Parallel()

		b := b2Backend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "create-missing-pk-pipe",
			RoleARN:      "arn:aws:iam::123456789012:role/r",
			Source:       "arn:aws:sqs:us-east-1:123456789012:q",
			Target:       "arn:aws:kinesis:us-east-1:123456789012:stream/out",
			DesiredState: "RUNNING",
			TargetParameters: &pipes.TargetParameters{
				KinesisStreamParameters: &pipes.KinesisStreamTargetParameters{},
			},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})

	t.Run("create_with_partition_key_accepted", func(t *testing.T) {
		t.Parallel()

		b := b2Backend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "create-with-pk-pipe",
			RoleARN:      "arn:aws:iam::123456789012:role/r",
			Source:       "arn:aws:sqs:us-east-1:123456789012:q",
			Target:       "arn:aws:kinesis:us-east-1:123456789012:stream/out",
			DesiredState: "RUNNING",
			TargetParameters: &pipes.TargetParameters{
				KinesisStreamParameters: &pipes.KinesisStreamTargetParameters{PartitionKey: "pk"},
			},
		})
		assert.NoError(t, err)
	})

	t.Run("update_missing_partition_key_rejected", func(t *testing.T) {
		t.Parallel()

		b := b2Backend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "update-missing-pk-pipe",
			RoleARN:      "arn:aws:iam::123456789012:role/r",
			Source:       "arn:aws:sqs:us-east-1:123456789012:q",
			Target:       "arn:aws:lambda:us-east-1:123456789012:function:fn",
			DesiredState: "RUNNING",
		})
		require.NoError(t, err)

		_, err = b.UpdatePipe(context.Background(), "update-missing-pk-pipe", pipes.UpdatePipeInput{
			RoleARN: "arn:aws:iam::123456789012:role/r",
			TargetParameters: &pipes.TargetParameters{
				KinesisStreamParameters: &pipes.KinesisStreamTargetParameters{},
			},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})
}

// TestTargetParams_CloudWatchLogs verifies CloudWatch Logs target parameters.
func TestTargetParams_CloudWatchLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		logStreamName string
		timestamp     string
	}{
		{name: "stream_only", logStreamName: "my-stream"},
		{name: "stream_and_timestamp", logStreamName: "my-stream", timestamp: "$.time"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			cwlParams := map[string]any{
				"LogStreamName": tt.logStreamName,
			}
			if tt.timestamp != "" {
				cwlParams["Timestamp"] = tt.timestamp
			}

			resp := auditCreate(t, h, tt.name+"-cwl-target-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:logs:us-west-2:123456789012:log-group:/pipes/out",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"CloudWatchLogsParameters": cwlParams,
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			cwp, _ := tp["CloudWatchLogsParameters"].(map[string]any)
			require.NotNil(t, cwp, "CloudWatchLogsParameters missing")
			assert.Equal(t, tt.logStreamName, cwp["LogStreamName"])
		})
	}
}

// TestTargetParams_EventBridgeEventBus verifies EventBridge event bus target parameters.
func TestTargetParams_EventBridgeEventBus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		detailType string
		endpointID string
	}{
		{
			name:       "basic_event_bus",
			source:     "my.app",
			detailType: "OrderCreated",
		},
		{
			name:       "with_endpoint",
			source:     "my.app",
			detailType: "PaymentProcessed",
			endpointID: "ep-12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			ebParams := map[string]any{
				"Source":     tt.source,
				"DetailType": tt.detailType,
			}
			if tt.endpointID != "" {
				ebParams["EndpointId"] = tt.endpointID
			}

			resp := auditCreate(t, h, tt.name+"-eb-target-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:events:us-west-2:123456789012:event-bus/my-bus",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"EventBridgeEventBusParameters": ebParams,
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			ebp, _ := tp["EventBridgeEventBusParameters"].(map[string]any)
			require.NotNil(t, ebp, "EventBridgeEventBusParameters missing")
			assert.Equal(t, tt.source, ebp["Source"])
			assert.Equal(t, tt.detailType, ebp["DetailType"])
		})
	}
}

// TestTargetParams_Redshift verifies Redshift Data API target parameters.
func TestTargetParams_Redshift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		database  string
		dbUser    string
		sqls      []string
		withEvent bool
	}{
		{
			name:     "basic_redshift",
			database: "mydb",
			dbUser:   "admin",
			sqls:     []string{"INSERT INTO events VALUES (1)"},
		},
		{
			name:      "redshift_with_event",
			database:  "analytics",
			dbUser:    "writer",
			sqls:      []string{"INSERT INTO raw VALUES (1)", "UPDATE summary SET cnt=cnt+1"},
			withEvent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			resp := auditCreate(t, h, tt.name+"-redshift-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:redshift:us-west-2:123456789012:cluster:mycluster",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"RedshiftDataParameters": map[string]any{
						"Database":  tt.database,
						"DbUser":    tt.dbUser,
						"Sqls":      tt.sqls,
						"WithEvent": tt.withEvent,
					},
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			rp, _ := tp["RedshiftDataParameters"].(map[string]any)
			require.NotNil(t, rp, "RedshiftDataParameters missing")
			assert.Equal(t, tt.database, rp["Database"])
			assert.Equal(t, tt.dbUser, rp["DbUser"])
		})
	}
}

// TestTargetParams_SageMaker verifies SageMaker Pipeline target parameters.
func TestTargetParams_SageMaker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params []map[string]string
	}{
		{
			name: "no_params",
		},
		{
			name: "single_param",
			params: []map[string]string{
				{"Name": "input-path", "Value": "s3://bucket/data"},
			},
		},
		{
			name: "multiple_params",
			params: []map[string]string{
				{"Name": "batch-size", "Value": "100"},
				{"Name": "output-path", "Value": "s3://bucket/output"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			smParams := map[string]any{}
			if len(tt.params) > 0 {
				smParams["PipelineParameterList"] = tt.params
			}

			resp := auditCreate(t, h, tt.name+"-sm-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:sagemaker:us-west-2:123456789012:pipeline/my-pipeline",
				"DesiredState": "RUNNING",
				"TargetParameters": map[string]any{
					"SageMakerPipelineParameters": smParams,
				},
			})

			tp, _ := resp["TargetParameters"].(map[string]any)
			smp, _ := tp["SageMakerPipelineParameters"].(map[string]any)
			require.NotNil(t, smp, "SageMakerPipelineParameters missing")
			if len(tt.params) > 0 {
				list, _ := smp["PipelineParameterList"].([]any)
				assert.Len(t, list, len(tt.params))
			}
		})
	}
}

// --- enrichment / target InputTemplate ---

// TestInputTemplate_RoundTrip verifies input template is stored and returned.
func TestInputTemplate_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
	}{
		{name: "simple_template", template: `{"key": "value"}`},
		{name: "jsonpath_template", template: `{"id": "<$.id>", "ts": "<$.timestamp>"}`},
		{name: "empty_template", template: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			body := map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
			}
			if tt.template != "" {
				body["TargetParameters"] = map[string]any{
					"InputTemplate": tt.template,
				}
			}

			resp := auditCreate(t, h, tt.name+"-pipe", body)

			if tt.template != "" {
				tp, _ := resp["TargetParameters"].(map[string]any)
				assert.Equal(t, tt.template, tp["InputTemplate"])
			}
		})
	}
}

// --- richer target parameter tests (StepFunctions/EventBridge/Redshift/SageMaker) ---

// TestStepFunctions_InvocationTypes verifies FIRE_AND_FORGET and REQUEST_RESPONSE.
func TestStepFunctions_InvocationTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		invocationType string
	}{
		{name: "fire_and_forget", invocationType: "FIRE_AND_FORGET"},
		{name: "request_response", invocationType: "REQUEST_RESPONSE"},
		{name: "empty_defaults_preserved", invocationType: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			sfnParams := map[string]any{}
			if tt.invocationType != "" {
				sfnParams["InvocationType"] = tt.invocationType
			}
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2SFNTarget,
				"TargetParameters": map[string]any{
					"StepFunctionStateMachineParameters": sfnParams,
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			sfn, ok := tp["StepFunctionStateMachineParameters"].(map[string]any)
			require.True(t, ok, "StepFunctionStateMachineParameters missing")

			if tt.invocationType != "" {
				assert.Equal(t, tt.invocationType, sfn["InvocationType"])
			}
		})
	}
}

// TestEventBridge_Params verifies EventBridge event bus parameters.
func TestEventBridge_Params(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		detailType string
		source     string
		endpointID string
		resources  []any
	}{
		{
			name:       "basic_event",
			detailType: "my.event.type",
			source:     "my.source",
		},
		{
			name:       "with_endpoint",
			detailType: "type",
			source:     "src",
			endpointID: "endpoint-id-abc",
		},
		{
			name:       "with_resources",
			detailType: "type",
			source:     "src",
			resources:  []any{"arn:aws:s3:::my-bucket", "arn:aws:s3:::my-bucket-2"},
		},
		{
			name:       "full_event_bus",
			detailType: "order.placed",
			source:     "com.myapp.orders",
			endpointID: "ep-xyz",
			resources:  []any{"arn:aws:s3:::bucket"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			ebParams := map[string]any{
				"DetailType": tt.detailType,
				"Source":     tt.source,
			}
			if tt.endpointID != "" {
				ebParams["EndpointId"] = tt.endpointID
			}
			if tt.resources != nil {
				ebParams["Resources"] = tt.resources
			}

			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:events:us-east-1:123456789012:event-bus/mybus",
				"TargetParameters": map[string]any{
					"EventBridgeEventBusParameters": ebParams,
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			eb, ok := tp["EventBridgeEventBusParameters"].(map[string]any)
			require.True(t, ok, "EventBridgeEventBusParameters missing")

			assert.Equal(t, tt.detailType, eb["DetailType"])
			assert.Equal(t, tt.source, eb["Source"])
			if tt.endpointID != "" {
				assert.Equal(t, tt.endpointID, eb["EndpointId"])
			}
			if tt.resources != nil {
				res, resOK := eb["Resources"].([]any)
				require.True(t, resOK)
				assert.Len(t, res, len(tt.resources))
			}
		})
	}
}

// TestRedshift_Params verifies all Redshift Data API parameters.
func TestRedshift_Params(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		database         string
		dbUser           string
		secretManagerArn string
		statementName    string
		sqls             []any
		withEvent        bool
	}{
		{
			name:     "basic_query",
			database: "mydb",
			dbUser:   "admin",
			sqls:     []any{"SELECT 1"},
		},
		{
			name:             "with_secret",
			database:         "mydb",
			secretManagerArn: "arn:aws:secretsmanager:us-east-1:123456789012:secret:mysecret",
			sqls:             []any{"INSERT INTO table1 VALUES (1)"},
		},
		{
			name:          "named_statement",
			database:      "mydb",
			dbUser:        "user1",
			statementName: "my-statement",
			sqls:          []any{"SELECT count(*) FROM orders"},
		},
		{
			name:      "with_event_multiple_sqls",
			database:  "prod",
			dbUser:    "etl",
			sqls:      []any{"TRUNCATE staging", "INSERT INTO prod SELECT * FROM staging"},
			withEvent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			rdParams := map[string]any{
				"Database": tt.database,
				"Sqls":     tt.sqls,
			}
			if tt.dbUser != "" {
				rdParams["DbUser"] = tt.dbUser
			}
			if tt.secretManagerArn != "" {
				rdParams["SecretManagerArn"] = tt.secretManagerArn
			}
			if tt.statementName != "" {
				rdParams["StatementName"] = tt.statementName
			}
			if tt.withEvent {
				rdParams["WithEvent"] = true
			}

			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:redshift:us-east-1:123456789012:cluster:mycluster",
				"TargetParameters": map[string]any{
					"RedshiftDataParameters": rdParams,
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			rd, ok := tp["RedshiftDataParameters"].(map[string]any)
			require.True(t, ok, "RedshiftDataParameters missing")

			assert.Equal(t, tt.database, rd["Database"])
			sqls, ok := rd["Sqls"].([]any)
			require.True(t, ok)
			assert.Len(t, sqls, len(tt.sqls))

			if tt.dbUser != "" {
				assert.Equal(t, tt.dbUser, rd["DbUser"])
			}
			if tt.secretManagerArn != "" {
				assert.Equal(t, tt.secretManagerArn, rd["SecretManagerArn"])
			}
			if tt.statementName != "" {
				assert.Equal(t, tt.statementName, rd["StatementName"])
			}
			if tt.withEvent {
				assert.Equal(t, true, rd["WithEvent"])
			}
		})
	}
}

// TestSageMaker_Params verifies SageMaker PipelineParameterList depth.
func TestSageMaker_Params(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantFirst string
		paramList []map[string]any
		wantLen   int
	}{
		{
			name:      "single_param",
			paramList: []map[string]any{{"Name": "LearningRate", "Value": "0.01"}},
			wantLen:   1,
			wantFirst: "LearningRate",
		},
		{
			name: "multi_params",
			paramList: []map[string]any{
				{"Name": "Epochs", "Value": "100"},
				{"Name": "BatchSize", "Value": "32"},
				{"Name": "LearningRate", "Value": "0.001"},
			},
			wantLen:   3,
			wantFirst: "Epochs",
		},
		{
			name: "special_value_types",
			paramList: []map[string]any{
				{"Name": "S3Uri", "Value": "s3://bucket/prefix/"},
				{"Name": "ModelArn", "Value": "arn:aws:sagemaker:us-east-1:123456789012:model/my-model"},
			},
			wantLen:   2,
			wantFirst: "S3Uri",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  "arn:aws:sagemaker:us-east-1:123456789012:pipeline/my-pipeline",
				"TargetParameters": map[string]any{
					"SageMakerPipelineParameters": map[string]any{
						"PipelineParameterList": tt.paramList,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			tp := resp["TargetParameters"].(map[string]any)
			sm, ok := tp["SageMakerPipelineParameters"].(map[string]any)
			require.True(t, ok, "SageMakerPipelineParameters missing")
			pl, ok := sm["PipelineParameterList"].([]any)
			require.True(t, ok)
			assert.Len(t, pl, tt.wantLen)

			first := pl[0].(map[string]any)
			assert.Equal(t, tt.wantFirst, first["Name"])
		})
	}
}

// --- Timestream target ---

// TestTargetParams_Timestream verifies that TimestreamParameters roundtrip
// through create/describe/update unchanged.
func TestTargetParams_Timestream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params *pipes.TimestreamParameters
		name   string
	}{
		{
			name: "dimension_and_single_measure",
			params: &pipes.TimestreamParameters{
				TimeValue:     "$.timestamp",
				TimeFieldType: "EPOCH",
				EpochTimeUnit: "MILLISECONDS",
				VersionValue:  "1",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionName: "region", DimensionValue: "$.region", DimensionValueType: "VARCHAR"},
				},
				SingleMeasureMappings: []pipes.TimestreamSingleMeasureMapping{
					{MeasureName: "cpu", MeasureValue: "$.cpu", MeasureValueType: "DOUBLE"},
				},
			},
		},
		{
			name: "multi_measure_mapping",
			params: &pipes.TimestreamParameters{
				TimeValue:       "$.eventTime",
				TimeFieldType:   "TIMESTAMP_FORMAT",
				TimestampFormat: "YYYY-MM-DD'T'HH:mm:ss",
				VersionValue:    "1",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionName: "host", DimensionValue: "$.host", DimensionValueType: "VARCHAR"},
				},
				MultiMeasureMappings: []pipes.TimestreamMultiMeasureMapping{
					{
						MultiMeasureName: "metrics",
						MultiMeasureAttributeMappings: []pipes.TimestreamMultiMeasureAttributeMapping{
							{MeasureValue: "$.cpu", MeasureValueType: "DOUBLE", MultiMeasureAttributeName: "cpu"},
							{MeasureValue: "$.mem", MeasureValueType: "DOUBLE", MultiMeasureAttributeName: "mem"},
						},
					},
				},
			},
		},
		{
			name: "minimal_timestream",
			params: &pipes.TimestreamParameters{
				TimeValue:     "$.ts",
				TimeFieldType: "UNIX_TIME",
				VersionValue:  "1",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionName: "id", DimensionValue: "$.id", DimensionValueType: "VARCHAR"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b4Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:    tt.name + "-ts-pipe",
				RoleARN: "arn:aws:iam::111122223333:role/r",
				Source:  b4SQSSource,
				Target:  "arn:aws:timestream:eu-west-1:111122223333:database/db/table/tbl",
				TargetParameters: &pipes.TargetParameters{
					TimestreamParameters: tt.params,
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name+"-ts-pipe")
			require.NoError(t, err)
			require.NotNil(t, p.TargetParameters)
			require.NotNil(t, p.TargetParameters.TimestreamParameters)

			ts := p.TargetParameters.TimestreamParameters
			assert.Equal(t, tt.params.TimeValue, ts.TimeValue)
			assert.Equal(t, tt.params.TimeFieldType, ts.TimeFieldType)
			assert.Equal(t, tt.params.EpochTimeUnit, ts.EpochTimeUnit)
			assert.Equal(t, tt.params.TimestampFormat, ts.TimestampFormat)
			assert.Equal(t, tt.params.VersionValue, ts.VersionValue)
			assert.Len(t, ts.DimensionMappings, len(tt.params.DimensionMappings))
			if len(tt.params.DimensionMappings) > 0 {
				assert.Equal(t, tt.params.DimensionMappings[0].DimensionName, ts.DimensionMappings[0].DimensionName)
				assert.Equal(t, tt.params.DimensionMappings[0].DimensionValue, ts.DimensionMappings[0].DimensionValue)
			}
		})
	}
}

// TestTargetParams_Timestream_Update verifies that UpdatePipe replaces
// TimestreamParameters when new params are provided.
func TestTargetParams_Timestream_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialParams *pipes.TimestreamParameters
		updateParams  *pipes.TimestreamParameters
		wantTimeValue string
	}{
		{
			name: "update_replaces_time_value",
			initialParams: &pipes.TimestreamParameters{
				TimeValue:     "$.oldTs",
				TimeFieldType: "EPOCH",
				EpochTimeUnit: "SECONDS",
				VersionValue:  "1",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionName: "d", DimensionValue: "$.d", DimensionValueType: "VARCHAR"},
				},
			},
			updateParams: &pipes.TimestreamParameters{
				TimeValue:     "$.newTs",
				TimeFieldType: "UNIX_TIME",
				VersionValue:  "1",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionName: "d", DimensionValue: "$.d", DimensionValueType: "VARCHAR"},
				},
			},
			wantTimeValue: "$.newTs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b4Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:    tt.name + "-pipe",
				RoleARN: "arn:aws:iam::111122223333:role/r",
				Source:  b4SQSSource,
				Target:  "arn:aws:timestream:eu-west-1:111122223333:database/db/table/t",
				TargetParameters: &pipes.TargetParameters{
					TimestreamParameters: tt.initialParams,
				},
			})
			require.NoError(t, err)

			_, err = b.UpdatePipe(context.Background(), tt.name+"-pipe", pipes.UpdatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				TargetParameters: &pipes.TargetParameters{
					TimestreamParameters: tt.updateParams,
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			require.NotNil(t, p.TargetParameters.TimestreamParameters)
			assert.Equal(t, tt.wantTimeValue, p.TargetParameters.TimestreamParameters.TimeValue)
		})
	}
}

// TestClone_TimestreamIsolation verifies that clonePipe deep-copies
// TimestreamParameters so mutations of the clone do not affect the original.
func TestClone_TimestreamIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "dimension_mappings_isolated"},
		{name: "multi_measure_isolated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b4Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:    tt.name + "-pipe",
				RoleARN: "arn:aws:iam::111122223333:role/r",
				Source:  b4SQSSource,
				Target:  "arn:aws:timestream:eu-west-1:111122223333:database/db/table/t",
				TargetParameters: &pipes.TargetParameters{
					TimestreamParameters: &pipes.TimestreamParameters{
						TimeValue:     "$.ts",
						TimeFieldType: "UNIX_TIME",
						VersionValue:  "1",
						DimensionMappings: []pipes.TimestreamDimensionMapping{
							{DimensionName: "region", DimensionValue: "$.region", DimensionValueType: "VARCHAR"},
						},
					},
				},
			})
			require.NoError(t, err)

			// GetPipe returns a clone; mutate the clone's dimension mapping.
			clone, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			require.NotNil(t, clone.TargetParameters.TimestreamParameters)
			clone.TargetParameters.TimestreamParameters.DimensionMappings[0].DimensionName = "mutated"

			// Re-read from backend; original should be unchanged.
			orig, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			assert.Equal(t, "region",
				orig.TargetParameters.TimestreamParameters.DimensionMappings[0].DimensionName,
				"mutating clone dimension should not affect stored pipe")
		})
	}
}

// --- HTTP (API Gateway / API destination) target ---

// TestTargetParams_HTTP verifies that HTTPParameters roundtrip
// for API Gateway and API Destination targets.
func TestTargetParams_HTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params *pipes.TargetHTTPParameters
		name   string
	}{
		{
			name: "headers_and_query_string",
			params: &pipes.TargetHTTPParameters{
				HeaderParameters:      map[string]string{"X-Custom": "value", "Content-Type": "application/json"},
				QueryStringParameters: map[string]string{"version": "2", "format": "json"},
				PathParameterValues:   []string{"orders", "123"},
			},
		},
		{
			name: "headers_only",
			params: &pipes.TargetHTTPParameters{
				HeaderParameters: map[string]string{"Authorization": "Bearer token"},
			},
		},
		{
			name: "path_params_only",
			params: &pipes.TargetHTTPParameters{
				PathParameterValues: []string{"v1", "users"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b4Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:    tt.name + "-pipe",
				RoleARN: "arn:aws:iam::111122223333:role/r",
				Source:  b4SQSSource,
				Target:  "arn:aws:execute-api:eu-west-1:111122223333:api/stage/route",
				TargetParameters: &pipes.TargetParameters{
					HTTPParameters: tt.params,
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			require.NotNil(t, p.TargetParameters)
			require.NotNil(t, p.TargetParameters.HTTPParameters)

			hp := p.TargetParameters.HTTPParameters
			assert.Equal(t, tt.params.HeaderParameters, hp.HeaderParameters)
			assert.Equal(t, tt.params.QueryStringParameters, hp.QueryStringParameters)
			assert.Equal(t, tt.params.PathParameterValues, hp.PathParameterValues)
		})
	}
}

// TestClone_HTTPParamsIsolation verifies that HTTPParameters are deep-copied
// in clonePipe so mutations of one clone do not affect others.
func TestClone_HTTPParamsIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "headers_isolated"},
		{name: "path_params_isolated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b4Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:    tt.name + "-pipe",
				RoleARN: "arn:aws:iam::111122223333:role/r",
				Source:  b4SQSSource,
				Target:  "arn:aws:execute-api:eu-west-1:111122223333:api/stage/route",
				TargetParameters: &pipes.TargetParameters{
					HTTPParameters: &pipes.TargetHTTPParameters{
						HeaderParameters:    map[string]string{"X-Original": "yes"},
						PathParameterValues: []string{"original"},
					},
				},
			})
			require.NoError(t, err)

			clone, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			clone.TargetParameters.HTTPParameters.HeaderParameters["X-Original"] = "mutated"
			clone.TargetParameters.HTTPParameters.PathParameterValues[0] = "mutated"

			orig, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			assert.Equal(t, "yes", orig.TargetParameters.HTTPParameters.HeaderParameters["X-Original"],
				"mutating clone headers should not affect stored pipe")
			assert.Equal(t, "original", orig.TargetParameters.HTTPParameters.PathParameterValues[0],
				"mutating clone path params should not affect stored pipe")
		})
	}
}

// TestTargetParams_HTTP_Roundtrip verifies that HTTPParameters survive
// a create → describe HTTP roundtrip via the handler.
func TestTargetParams_HTTP_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		body          map[string]any
		wantHeaderKey string
		wantHeaderVal string
	}{
		{
			name: "headers_survive_roundtrip",
			body: map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b4SQSSource,
				"Target":  "arn:aws:execute-api:eu-west-1:111122223333:api/stage/route",
				"TargetParameters": map[string]any{
					"HttpParameters": map[string]any{
						"HeaderParameters": map[string]any{
							"X-Pipe-Id": "batch4-test",
						},
					},
				},
			},
			wantHeaderKey: "X-Pipe-Id",
			wantHeaderVal: "batch4-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b4Handler(t)
			b4Create(t, h, tt.name+"-pipe", tt.body)

			resp := b4Describe(t, h, tt.name+"-pipe")
			tp, ok := resp["TargetParameters"].(map[string]any)
			require.True(t, ok, "TargetParameters should be present")
			hp, ok := tp["HttpParameters"].(map[string]any)
			require.True(t, ok, "HTTPParameters should be present")
			headers, ok := hp["HeaderParameters"].(map[string]any)
			require.True(t, ok, "HeaderParameters should be present")
			assert.Equal(t, tt.wantHeaderVal, headers[tt.wantHeaderKey])
		})
	}
}
