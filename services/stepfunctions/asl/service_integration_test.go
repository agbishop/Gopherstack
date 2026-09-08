package asl_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions/asl"
)

// errDynamoDBIntegrationNotConfigured is the sentinel used in DynamoDB integration error tests.
var errDynamoDBIntegrationNotConfigured = asl.ErrDynamoDBIntegrationNotConfigured

// --- Mock implementations ---

type mockSQS struct {
	returnErr               error
	capturedQueueURL        string
	capturedMessageBody     string
	capturedGroupID         string
	capturedDeduplicationID string
	returnMsgID             string
	returnMD5               string
	capturedDelaySeconds    int
}

func (m *mockSQS) SFNSendMessage(
	_ context.Context,
	queueURL, messageBody, groupID, deduplicationID string,
	delaySeconds int,
) (string, string, error) {
	m.capturedQueueURL = queueURL
	m.capturedMessageBody = messageBody
	m.capturedGroupID = groupID
	m.capturedDeduplicationID = deduplicationID
	m.capturedDelaySeconds = delaySeconds

	return m.returnMsgID, m.returnMD5, m.returnErr
}

type mockSNS struct {
	returnErr        error
	capturedTopicARN string
	capturedMessage  string
	capturedSubject  string
	returnMsgID      string
}

func (m *mockSNS) SFNPublish(_ context.Context, topicARN, message, subject string) (string, error) {
	m.capturedTopicARN = topicARN
	m.capturedMessage = message
	m.capturedSubject = subject

	return m.returnMsgID, m.returnErr
}

type mockTaskTokenCallback struct {
	returnErr     error
	returnOutput  string
	lastToken     string
	called        int
	lastHeartbeat int
}

func (m *mockTaskTokenCallback) WaitForTaskToken(
	_ context.Context,
	taskToken string,
	heartbeatSeconds int,
) (string, error) {
	m.called++
	m.lastToken = taskToken
	m.lastHeartbeat = heartbeatSeconds

	return m.returnOutput, m.returnErr
}

type mockDynamoDB struct {
	returnOutput               any
	returnErr                  error
	calledPut                  bool
	calledGet                  bool
	calledDelete               bool
	calledUpdate               bool
	calledBatchExecuteStmt     bool
	calledBatchGet             bool
	calledBatchWrite           bool
	calledCreateBackup         bool
	calledCreateGlobalTable    bool
	calledCreateTable          bool
	calledDeleteBackup         bool
	calledDeleteResourcePolicy bool
	calledDeleteTable          bool
}

func (m *mockDynamoDB) SFNPutItem(_ context.Context, _ any) (any, error) {
	m.calledPut = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNGetItem(_ context.Context, _ any) (any, error) {
	m.calledGet = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNDeleteItem(_ context.Context, _ any) (any, error) {
	m.calledDelete = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNUpdateItem(_ context.Context, _ any) (any, error) {
	m.calledUpdate = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNBatchExecuteStatement(_ context.Context, _ any) (any, error) {
	m.calledBatchExecuteStmt = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNBatchGetItem(_ context.Context, _ any) (any, error) {
	m.calledBatchGet = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNBatchWriteItem(_ context.Context, _ any) (any, error) {
	m.calledBatchWrite = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNCreateBackup(_ context.Context, _ any) (any, error) {
	m.calledCreateBackup = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNCreateGlobalTable(_ context.Context, _ any) (any, error) {
	m.calledCreateGlobalTable = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNCreateTable(_ context.Context, _ any) (any, error) {
	m.calledCreateTable = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNDeleteBackup(_ context.Context, _ any) (any, error) {
	m.calledDeleteBackup = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNDeleteResourcePolicy(_ context.Context, _ any) (any, error) {
	m.calledDeleteResourcePolicy = true

	return m.returnOutput, m.returnErr
}

func (m *mockDynamoDB) SFNDeleteTable(_ context.Context, _ any) (any, error) {
	m.calledDeleteTable = true

	return m.returnOutput, m.returnErr
}

// --- SQS helpers ---

type sqsCaptureAssertion struct {
	wantCapturedQueueURL        string
	wantCapturedMessageBody     string
	wantCapturedGroupID         string
	wantCapturedDeduplicationID string
	wantCapturedDelaySeconds    int
}

func assertSQSCaptures(t *testing.T, mock *mockSQS, tt sqsCaptureAssertion) {
	t.Helper()

	if tt.wantCapturedQueueURL != "" {
		assert.Equal(t, tt.wantCapturedQueueURL, mock.capturedQueueURL)
	}

	if tt.wantCapturedMessageBody != "" {
		assert.Equal(t, tt.wantCapturedMessageBody, mock.capturedMessageBody)
	}

	if tt.wantCapturedGroupID != "" {
		assert.Equal(t, tt.wantCapturedGroupID, mock.capturedGroupID)
	}

	if tt.wantCapturedDeduplicationID != "" {
		assert.Equal(t, tt.wantCapturedDeduplicationID, mock.capturedDeduplicationID)
	}

	if tt.wantCapturedDelaySeconds != 0 {
		assert.Equal(t, tt.wantCapturedDelaySeconds, mock.capturedDelaySeconds)
	}
}

// --- SQS tests ---

func TestExecutor_SQS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantCauseContains           string
		wantCapturedDeduplicationID string
		wantOutputMsgID             string
		mockReturnMsgID             string
		mockReturnMD5               string
		wantError                   string
		wantOutputMD5               string
		name                        string
		def                         string
		wantCapturedQueueURL        string
		wantCapturedMessageBody     string
		wantCapturedGroupID         string
		wantCapturedDelaySeconds    int
		setMock                     bool
	}{
		{
			name: "send_message",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:sendMessage",
						"Parameters": {
							"QueueUrl": "https://sqs.us-east-1.amazonaws.com/123456789012/myqueue",
							"MessageBody": "hello world"
						},
						"End": true
					}
				}
			}`,
			setMock:                 true,
			mockReturnMsgID:         "msg-1",
			mockReturnMD5:           "abc123",
			wantOutputMsgID:         "msg-1",
			wantOutputMD5:           "abc123",
			wantCapturedQueueURL:    "https://sqs.us-east-1.amazonaws.com/123456789012/myqueue",
			wantCapturedMessageBody: "hello world",
		},
		{
			name: "send_message_with_fifo_fields",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:sendMessage",
						"Parameters": {
							"QueueUrl": "https://sqs.us-east-1.amazonaws.com/123/myqueue.fifo",
							"MessageBody": "fifo msg",
							"MessageGroupId": "group1",
							"MessageDeduplicationId": "dedup1",
							"DelaySeconds": 5
						},
						"End": true
					}
				}
			}`,
			setMock:                     true,
			mockReturnMsgID:             "msg-2",
			mockReturnMD5:               "def456",
			wantCapturedGroupID:         "group1",
			wantCapturedDeduplicationID: "dedup1",
			wantCapturedDelaySeconds:    5,
		},
		{
			name: "not_configured",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:sendMessage",
						"End": true
					}
				}
			}`,
			setMock:           false,
			wantError:         "TaskFailed",
			wantCauseContains: "SQS integration not configured",
		},
		{
			name: "unsupported_action",
			def: `{
				"StartAt": "Recv",
				"States": {
					"Recv": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:receiveMessage",
						"End": true
					}
				}
			}`,
			setMock:           true,
			wantError:         "TaskFailed",
			wantCauseContains: "unsupported SQS action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)

			var mock *mockSQS
			if tt.setMock {
				mock = &mockSQS{returnMsgID: tt.mockReturnMsgID, returnMD5: tt.mockReturnMD5}
				exec.SetSQSIntegration(mock)
			}

			result, err := exec.Execute(t.Context(), "test-exec", `{}`)
			require.NoError(t, err)

			assert.Equal(t, tt.wantError, result.Error)

			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			if tt.wantOutputMsgID != "" {
				out, ok := result.Output.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOutputMsgID, out["MessageId"])
				assert.Equal(t, tt.wantOutputMD5, out["MD5OfMessageBody"])
			}

			if mock != nil {
				assertSQSCaptures(t, mock, sqsCaptureAssertion{
					wantCapturedQueueURL:        tt.wantCapturedQueueURL,
					wantCapturedMessageBody:     tt.wantCapturedMessageBody,
					wantCapturedGroupID:         tt.wantCapturedGroupID,
					wantCapturedDeduplicationID: tt.wantCapturedDeduplicationID,
					wantCapturedDelaySeconds:    tt.wantCapturedDelaySeconds,
				})
			}
		})
	}
}

func TestExecutor_SQS_IntegrationPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		callbackOutput    string
		callbackErr       error
		wantCauseContains string
		wantOutputType    string
		name              string
		def               string
		wantError         string
		setCallback       bool
		wantCallbackCalls int
		wantHeartbeat     int
	}{
		{
			name: "wait_for_task_token_uses_callback_output",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:sendMessage.waitForTaskToken",
						"HeartbeatSeconds": 7,
						"Parameters": {
							"QueueUrl": "https://sqs.us-east-1.amazonaws.com/123456789012/myqueue",
							"MessageBody": "hello world"
						},
						"End": true
					}
				}
			}`,
			setCallback:       true,
			callbackOutput:    `{"callback":"ok"}`,
			wantOutputType:    "map",
			wantCallbackCalls: 1,
			wantHeartbeat:     7,
		},
		{
			name: "async_pattern_returns_send_message_output",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:sendMessage.async",
						"Parameters": {
							"QueueUrl": "https://sqs.us-east-1.amazonaws.com/123456789012/myqueue",
							"MessageBody": "hello world"
						},
						"End": true
					}
				}
			}`,
			setCallback:       true,
			callbackOutput:    `{"ignored":true}`,
			wantOutputType:    "sendMessage",
			wantCallbackCalls: 0,
		},
		{
			name: "wait_for_task_token_requires_callback_invoker",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:sendMessage.waitForTaskToken",
						"Parameters": {
							"QueueUrl": "https://sqs.us-east-1.amazonaws.com/123456789012/myqueue",
							"MessageBody": "hello world"
						},
						"End": true
					}
				}
			}`,
			wantError:         "TaskFailed",
			wantCauseContains: "task token callback invoker not configured",
			wantCallbackCalls: 0,
		},
		{
			name: "wait_for_task_token_callback_error",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sqs:sendMessage.waitForTaskToken",
						"Parameters": {
							"QueueUrl": "https://sqs.us-east-1.amazonaws.com/123456789012/myqueue",
							"MessageBody": "hello world"
						},
						"End": true
					}
				}
			}`,
			setCallback:       true,
			callbackErr:       assert.AnError,
			wantError:         "TaskFailed",
			wantCauseContains: assert.AnError.Error(),
			wantCallbackCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			sqsMock := &mockSQS{returnMsgID: "msg-id", returnMD5: "md5"}
			exec.SetSQSIntegration(sqsMock)

			callbackMock := &mockTaskTokenCallback{
				returnOutput: tt.callbackOutput,
				returnErr:    tt.callbackErr,
			}
			if tt.setCallback {
				exec.SetTaskTokenCallbackInvoker(callbackMock)
			}

			result, err := exec.Execute(t.Context(), "test-exec", `{}`)
			require.NoError(t, err)
			assert.Equal(t, tt.wantError, result.Error)
			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			switch tt.wantOutputType {
			case "map":
				outputMap, ok := result.Output.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "ok", outputMap["callback"])
			case "sendMessage":
				outputMap, ok := result.Output.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "msg-id", outputMap["MessageId"])
			}

			assert.Equal(t, tt.wantCallbackCalls, callbackMock.called)
			if tt.wantCallbackCalls > 0 {
				assert.NotEmpty(t, callbackMock.lastToken)
				assert.Equal(t, tt.wantHeartbeat, callbackMock.lastHeartbeat)
			}
		})
	}
}

// --- SNS tests ---

func TestExecutor_SNS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		def                  string
		mockReturnMsgID      string
		wantError            string
		wantCauseContains    string
		wantOutputMsgID      string
		wantCapturedTopicARN string
		wantCapturedMessage  string
		wantCapturedSubject  string
		setMock              bool
	}{
		{
			name: "publish",
			def: `{
				"StartAt": "Publish",
				"States": {
					"Publish": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sns:publish",
						"Parameters": {
							"TopicArn": "arn:aws:sns:us-east-1:123456789012:MyTopic",
							"Message": "test message",
							"Subject": "test subject"
						},
						"End": true
					}
				}
			}`,
			setMock:              true,
			mockReturnMsgID:      "sns-msg-1",
			wantOutputMsgID:      "sns-msg-1",
			wantCapturedTopicARN: "arn:aws:sns:us-east-1:123456789012:MyTopic",
			wantCapturedMessage:  "test message",
			wantCapturedSubject:  "test subject",
		},
		{
			name: "not_configured",
			def: `{
				"StartAt": "Publish",
				"States": {
					"Publish": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sns:publish",
						"End": true
					}
				}
			}`,
			setMock:           false,
			wantError:         "TaskFailed",
			wantCauseContains: "SNS integration not configured",
		},
		{
			name: "unsupported_action",
			def: `{
				"StartAt": "Sub",
				"States": {
					"Sub": {
						"Type": "Task",
						"Resource": "arn:aws:states:::sns:subscribe",
						"End": true
					}
				}
			}`,
			setMock:           true,
			wantError:         "TaskFailed",
			wantCauseContains: "unsupported SNS action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)

			var mock *mockSNS
			if tt.setMock {
				mock = &mockSNS{returnMsgID: tt.mockReturnMsgID}
				exec.SetSNSIntegration(mock)
			}

			result, err := exec.Execute(t.Context(), "test-exec", `{}`)
			require.NoError(t, err)

			assert.Equal(t, tt.wantError, result.Error)

			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			if tt.wantOutputMsgID != "" {
				out, ok := result.Output.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOutputMsgID, out["MessageId"])
			}

			if mock != nil {
				if tt.wantCapturedTopicARN != "" {
					assert.Equal(t, tt.wantCapturedTopicARN, mock.capturedTopicARN)
				}
				if tt.wantCapturedMessage != "" {
					assert.Equal(t, tt.wantCapturedMessage, mock.capturedMessage)
				}
				if tt.wantCapturedSubject != "" {
					assert.Equal(t, tt.wantCapturedSubject, mock.capturedSubject)
				}
			}
		})
	}
}

// --- aws-sdk integration pattern tests ---

func TestExecutor_AwsSDKIntegrationPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mockClient    any
		setOnExecutor func(exec *asl.Executor, mockClient any)
		name          string
		def           string
	}{
		{
			name: "sqs_aws-sdk_prefix",
			def: `{
				"StartAt": "Send",
				"States": {
					"Send": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:sqs:sendMessage",
						"Parameters": {
							"QueueUrl": "https://sqs.us-east-1.amazonaws.com/123/myqueue",
							"MessageBody": "hello"
						},
						"End": true
					}
				}
			}`,
			mockClient: &mockSQS{returnMsgID: "m1", returnMD5: "md5"},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetSQSIntegration(m.(*mockSQS))
			},
		},
		{
			name: "sns_aws-sdk_prefix",
			def: `{
				"StartAt": "Pub",
				"States": {
					"Pub": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:sns:publish",
						"Parameters": {
							"TopicArn": "arn:aws:sns:us-east-1:123:MyTopic",
							"Message": "hello"
						},
						"End": true
					}
				}
			}`,
			mockClient: &mockSNS{returnMsgID: "s1"},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetSNSIntegration(m.(*mockSNS))
			},
		},
		{
			name: "dynamodb_aws-sdk_prefix",
			def: `{
				"StartAt": "Put",
				"States": {
					"Put": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:putItem",
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_batch_execute_statement_aws-sdk_prefix",
			def: `{
				"StartAt": "Batch",
				"States": {
					"Batch": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:batchExecuteStatement",
						"Parameters": {"Statements": []},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{"Responses": []any{}}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_batch_get_item_aws-sdk_prefix",
			def: `{
				"StartAt": "BatchGet",
				"States": {
					"BatchGet": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:batchGetItem",
						"Parameters": {"RequestItems": {}},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{"Responses": map[string]any{}}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_batch_write_item_aws-sdk_prefix",
			def: `{
				"StartAt": "BatchWrite",
				"States": {
					"BatchWrite": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:batchWriteItem",
						"Parameters": {"RequestItems": {}},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_create_backup_aws-sdk_prefix",
			def: `{
				"StartAt": "CreateBkp",
				"States": {
					"CreateBkp": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:createBackup",
						"Parameters": {"TableName": "T", "BackupName": "B"},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{"BackupDetails": map[string]any{}}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_create_global_table_aws-sdk_prefix",
			def: `{
				"StartAt": "GT",
				"States": {
					"GT": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:createGlobalTable",
						"Parameters": {
							"GlobalTableName": "G",
							"ReplicationGroup": [{"RegionName": "us-east-1"}]
						},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{"GlobalTableDescription": map[string]any{}}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_create_table_aws-sdk_prefix",
			def: `{
				"StartAt": "Create",
				"States": {
					"Create": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:createTable",
						"Parameters": {"TableName": "T", "BillingMode": "PAY_PER_REQUEST"},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{"TableDescription": map[string]any{}}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_delete_backup_aws-sdk_prefix",
			def: `{
				"StartAt": "DelBkp",
				"States": {
					"DelBkp": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:deleteBackup",
						"Parameters": {"BackupArn": "arn:aws:dynamodb:us-east-1:123:backup/T"},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{"BackupDescription": map[string]any{}}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_delete_resource_policy_aws-sdk_prefix",
			def: `{
				"StartAt": "DelPolicy",
				"States": {
					"DelPolicy": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:deleteResourcePolicy",
						"Parameters": {"ResourceArn": "arn:aws:dynamodb:us-east-1:123:table/T"},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
		{
			name: "dynamodb_delete_table_aws-sdk_prefix",
			def: `{
				"StartAt": "DelTbl",
				"States": {
					"DelTbl": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:dynamodb:deleteTable",
						"Parameters": {"TableName": "OldTable"},
						"End": true
					}
				}
			}`,
			mockClient: &mockDynamoDB{returnOutput: map[string]any{"TableDescription": map[string]any{}}},
			setOnExecutor: func(exec *asl.Executor, m any) {
				exec.SetDynamoDBIntegration(m.(*mockDynamoDB))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			tt.setOnExecutor(exec, tt.mockClient)

			result, err := exec.Execute(t.Context(), "exec-1", `{}`)
			require.NoError(t, err)
			assert.Empty(t, result.Error, "execution should succeed via aws-sdk prefix")
		})
	}
}

func TestExecutor_DynamoDB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		def               string
		mock              *mockDynamoDB
		wantError         string
		wantCauseContains string
		wantCalledPut     bool
		wantCalledGet     bool
		wantCalledDelete  bool
		wantCalledUpdate  bool
	}{
		{
			name: "put_item",
			def: `{
				"StartAt": "Put",
				"States": {
					"Put": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:putItem",
						"Parameters": {
							"TableName": "MyTable",
							"Item": {"pk": {"S": "val"}}
						},
						"End": true
					}
				}
			}`,
			mock:          &mockDynamoDB{returnOutput: map[string]any{}},
			wantCalledPut: true,
		},
		{
			name: "get_item",
			def: `{
				"StartAt": "Get",
				"States": {
					"Get": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:getItem",
						"Parameters": {
							"TableName": "MyTable",
							"Key": {"pk": {"S": "val"}}
						},
						"End": true
					}
				}
			}`,
			mock:          &mockDynamoDB{returnOutput: map[string]any{"Item": map[string]any{}}},
			wantCalledGet: true,
		},
		{
			name: "delete_item",
			def: `{
				"StartAt": "Delete",
				"States": {
					"Delete": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:deleteItem",
						"Parameters": {
							"TableName": "MyTable",
							"Key": {"pk": {"S": "val"}}
						},
						"End": true
					}
				}
			}`,
			mock:             &mockDynamoDB{returnOutput: map[string]any{}},
			wantCalledDelete: true,
		},
		{
			name: "update_item",
			def: `{
				"StartAt": "Update",
				"States": {
					"Update": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:updateItem",
						"Parameters": {
							"TableName": "MyTable",
							"Key": {"pk": {"S": "val"}}
						},
						"End": true
					}
				}
			}`,
			mock:             &mockDynamoDB{returnOutput: map[string]any{}},
			wantCalledUpdate: true,
		},
		{
			name: "not_configured",
			def: `{
				"StartAt": "Put",
				"States": {
					"Put": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:putItem",
						"End": true
					}
				}
			}`,
			mock:              nil,
			wantError:         "TaskFailed",
			wantCauseContains: "DynamoDB integration not configured",
		},
		{
			name: "unsupported_action",
			def: `{
				"StartAt": "Scan",
				"States": {
					"Scan": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:scan",
						"End": true
					}
				}
			}`,
			mock:              &mockDynamoDB{},
			wantError:         "TaskFailed",
			wantCauseContains: "unsupported DynamoDB action",
		},
		{
			name: "integration_error",
			def: `{
				"StartAt": "Put",
				"States": {
					"Put": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:putItem",
						"End": true
					}
				}
			}`,
			mock:              &mockDynamoDB{returnErr: errDynamoDBIntegrationNotConfigured},
			wantError:         "TaskFailed",
			wantCauseContains: errDynamoDBIntegrationNotConfigured.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			if tt.mock != nil {
				exec.SetDynamoDBIntegration(tt.mock)
			}

			result, err := exec.Execute(t.Context(), "test-exec", `{}`)
			require.NoError(t, err)

			assert.Equal(t, tt.wantError, result.Error)

			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			if tt.mock != nil && tt.wantError == "" {
				assert.Equal(t, tt.wantCalledPut, tt.mock.calledPut)
				assert.Equal(t, tt.wantCalledGet, tt.mock.calledGet)
				assert.Equal(t, tt.wantCalledDelete, tt.mock.calledDelete)
				assert.Equal(t, tt.wantCalledUpdate, tt.mock.calledUpdate)
			}
		})
	}
}

func TestExecutor_DynamoDB_NewOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                        string
		def                         string
		mock                        *mockDynamoDB
		wantError                   string
		wantCauseContains           string
		wantCalledBatchExecuteStmt  bool
		wantCalledBatchGet          bool
		wantCalledBatchWrite        bool
		wantCalledCreateBackup      bool
		wantCalledCreateGlobalTable bool
		wantCalledCreateTable       bool
		wantCalledDeleteBackup      bool
		wantCalledDeleteResPolicy   bool
		wantCalledDeleteTable       bool
	}{
		{
			name: "batch_execute_statement",
			def: `{
				"StartAt": "Batch",
				"States": {
					"Batch": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:batchExecuteStatement",
						"Parameters": {
							"Statements": [{"Statement": "SELECT * FROM MyTable"}]
						},
						"End": true
					}
				}
			}`,
			mock:                       &mockDynamoDB{returnOutput: map[string]any{"Responses": []any{}}},
			wantCalledBatchExecuteStmt: true,
		},
		{
			name: "batch_get_item",
			def: `{
				"StartAt": "BatchGet",
				"States": {
					"BatchGet": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:batchGetItem",
						"Parameters": {
							"RequestItems": {}
						},
						"End": true
					}
				}
			}`,
			mock:               &mockDynamoDB{returnOutput: map[string]any{"Responses": map[string]any{}}},
			wantCalledBatchGet: true,
		},
		{
			name: "batch_write_item",
			def: `{
				"StartAt": "BatchWrite",
				"States": {
					"BatchWrite": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:batchWriteItem",
						"Parameters": {
							"RequestItems": {}
						},
						"End": true
					}
				}
			}`,
			mock:                 &mockDynamoDB{returnOutput: map[string]any{}},
			wantCalledBatchWrite: true,
		},
		{
			name: "create_backup",
			def: `{
				"StartAt": "CreateBkp",
				"States": {
					"CreateBkp": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:createBackup",
						"Parameters": {
							"TableName": "MyTable",
							"BackupName": "my-backup"
						},
						"End": true
					}
				}
			}`,
			mock:                   &mockDynamoDB{returnOutput: map[string]any{"BackupDetails": map[string]any{}}},
			wantCalledCreateBackup: true,
		},
		{
			name: "create_global_table",
			def: `{
				"StartAt": "CreateGT",
				"States": {
					"CreateGT": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:createGlobalTable",
						"Parameters": {
							"GlobalTableName": "MyGlobalTable",
							"ReplicationGroup": [{"RegionName": "us-east-1"}]
						},
						"End": true
					}
				}
			}`,
			mock: &mockDynamoDB{
				returnOutput: map[string]any{"GlobalTableDescription": map[string]any{}},
			},
			wantCalledCreateGlobalTable: true,
		},
		{
			name: "create_table",
			def: `{
				"StartAt": "Create",
				"States": {
					"Create": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:createTable",
						"Parameters": {
							"TableName": "NewTable",
							"AttributeDefinitions": [{"AttributeName": "pk", "AttributeType": "S"}],
							"KeySchema": [{"AttributeName": "pk", "KeyType": "HASH"}],
							"BillingMode": "PAY_PER_REQUEST"
						},
						"End": true
					}
				}
			}`,
			mock:                  &mockDynamoDB{returnOutput: map[string]any{"TableDescription": map[string]any{}}},
			wantCalledCreateTable: true,
		},
		{
			name: "delete_backup",
			def: `{
				"StartAt": "DeleteBkp",
				"States": {
					"DeleteBkp": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:deleteBackup",
						"Parameters": {
							"BackupArn": "arn:aws:dynamodb:us-east-1:123:backup/TestTable-backup"
						},
						"End": true
					}
				}
			}`,
			mock:                   &mockDynamoDB{returnOutput: map[string]any{"BackupDescription": map[string]any{}}},
			wantCalledDeleteBackup: true,
		},
		{
			name: "delete_resource_policy",
			def: `{
				"StartAt": "DeletePolicy",
				"States": {
					"DeletePolicy": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:deleteResourcePolicy",
						"Parameters": {
							"ResourceArn": "arn:aws:dynamodb:us-east-1:123:table/MyTable"
						},
						"End": true
					}
				}
			}`,
			mock:                      &mockDynamoDB{returnOutput: map[string]any{}},
			wantCalledDeleteResPolicy: true,
		},
		{
			name: "delete_table",
			def: `{
				"StartAt": "DeleteTbl",
				"States": {
					"DeleteTbl": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:deleteTable",
						"Parameters": {
							"TableName": "OldTable"
						},
						"End": true
					}
				}
			}`,
			mock:                  &mockDynamoDB{returnOutput: map[string]any{"TableDescription": map[string]any{}}},
			wantCalledDeleteTable: true,
		},
		{
			name: "not_configured",
			def: `{
				"StartAt": "CreateTbl",
				"States": {
					"CreateTbl": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:createTable",
						"End": true
					}
				}
			}`,
			mock:              nil,
			wantError:         "TaskFailed",
			wantCauseContains: "DynamoDB integration not configured",
		},
		{
			name: "integration_error_create_backup",
			def: `{
				"StartAt": "CreateBkp",
				"States": {
					"CreateBkp": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:createBackup",
						"End": true
					}
				}
			}`,
			mock:              &mockDynamoDB{returnErr: errDynamoDBIntegrationNotConfigured},
			wantError:         "TaskFailed",
			wantCauseContains: errDynamoDBIntegrationNotConfigured.Error(),
		},
		{
			name: "integration_error_delete_backup",
			def: `{
				"StartAt": "DelBkp",
				"States": {
					"DelBkp": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:deleteBackup",
						"End": true
					}
				}
			}`,
			mock:              &mockDynamoDB{returnErr: errDynamoDBIntegrationNotConfigured},
			wantError:         "TaskFailed",
			wantCauseContains: errDynamoDBIntegrationNotConfigured.Error(),
		},
		{
			name: "integration_error_batch_execute_statement",
			def: `{
				"StartAt": "Batch",
				"States": {
					"Batch": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:batchExecuteStatement",
						"End": true
					}
				}
			}`,
			mock:              &mockDynamoDB{returnErr: errDynamoDBIntegrationNotConfigured},
			wantError:         "TaskFailed",
			wantCauseContains: errDynamoDBIntegrationNotConfigured.Error(),
		},
		{
			name: "integration_error_delete_table",
			def: `{
				"StartAt": "DelTbl",
				"States": {
					"DelTbl": {
						"Type": "Task",
						"Resource": "arn:aws:states:::dynamodb:deleteTable",
						"End": true
					}
				}
			}`,
			mock:              &mockDynamoDB{returnErr: errDynamoDBIntegrationNotConfigured},
			wantError:         "TaskFailed",
			wantCauseContains: errDynamoDBIntegrationNotConfigured.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			if tt.mock != nil {
				exec.SetDynamoDBIntegration(tt.mock)
			}

			result, err := exec.Execute(t.Context(), "test-exec", `{}`)
			require.NoError(t, err)

			assert.Equal(t, tt.wantError, result.Error)

			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			if tt.mock != nil && tt.wantError == "" {
				assert.Equal(t, tt.wantCalledBatchExecuteStmt, tt.mock.calledBatchExecuteStmt)
				assert.Equal(t, tt.wantCalledBatchGet, tt.mock.calledBatchGet)
				assert.Equal(t, tt.wantCalledBatchWrite, tt.mock.calledBatchWrite)
				assert.Equal(t, tt.wantCalledCreateBackup, tt.mock.calledCreateBackup)
				assert.Equal(t, tt.wantCalledCreateGlobalTable, tt.mock.calledCreateGlobalTable)
				assert.Equal(t, tt.wantCalledCreateTable, tt.mock.calledCreateTable)
				assert.Equal(t, tt.wantCalledDeleteBackup, tt.mock.calledDeleteBackup)
				assert.Equal(t, tt.wantCalledDeleteResPolicy, tt.mock.calledDeleteResourcePolicy)
				assert.Equal(t, tt.wantCalledDeleteTable, tt.mock.calledDeleteTable)
			}
		})
	}
}
