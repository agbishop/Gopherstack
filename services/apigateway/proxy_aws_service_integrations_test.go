package apigateway_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSQSSender records the queue ARN and message body passed to SendMessageToQueue.
type mockSQSSender struct {
	err      error
	queueARN string
	body     string
	called   bool
}

func (m *mockSQSSender) SendMessageToQueue(_ context.Context, queueARN, messageBody string) error {
	m.called = true
	m.queueARN = queueARN
	m.body = messageBody

	return m.err
}

// mockSNSPublisher records the topic ARN and message passed to PublishToTopic.
type mockSNSPublisher struct {
	err      error
	topicARN string
	message  string
	called   bool
}

func (m *mockSNSPublisher) PublishToTopic(_ context.Context, topicARN, message string) error {
	m.called = true
	m.topicARN = topicARN
	m.message = message

	return m.err
}

var errSQSSendFailed = errors.New("sqs send failed")

const (
	sqsIntegrationURI = "arn:aws:apigateway:us-east-1:sqs:path/123456789012/my-queue"
	snsIntegrationURI = "arn:aws:apigateway:us-east-1:sns:action/Publish"
)

// TestHandleAWSIntegration_SQSTarget proves an AWS integration whose URI targets sqs
// reaches the wired SQS hook, not Lambda (gopherstack-is2a).
func TestHandleAWSIntegration_SQSTarget(t *testing.T) {
	t.Parallel()

	h, e, apiID := setupProxyAPIViaHandler(t, "AWS", sqsIntegrationURI)

	sender := &mockSQSSender{}
	h.SetSQSSender(sender)

	rec := proxyReq(t, h, e, apiID, "/items", `{"hello":"world"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, sender.called)
	assert.Equal(t, "arn:aws:sqs:us-east-1:123456789012:my-queue", sender.queueARN)
	assert.JSONEq(t, `{"hello":"world"}`, sender.body)
}

func TestHandleAWSIntegration_SQSTarget_SendError(t *testing.T) {
	t.Parallel()

	h, e, apiID := setupProxyAPIViaHandler(t, "AWS", sqsIntegrationURI)

	sender := &mockSQSSender{err: errSQSSendFailed}
	h.SetSQSSender(sender)

	rec := proxyReq(t, h, e, apiID, "/items", `{}`)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestHandleAWSIntegration_SQSTarget_Unwired proves that an AWS integration pointed at
// sqs with no SQS hook wired behaves exactly as it did before this fix: an unwired hook
// is a silent no-op, not a rejection, so the request falls through to the pre-existing
// Lambda-invoke path unchanged.
func TestHandleAWSIntegration_SQSTarget_Unwired(t *testing.T) {
	t.Parallel()

	t.Run("no_lambda_either", func(t *testing.T) {
		t.Parallel()

		h, e, apiID := setupProxyAPIViaHandler(t, "AWS", sqsIntegrationURI)
		// Neither SetSQSSender nor SetLambdaInvoker called.

		rec := proxyReq(t, h, e, apiID, "/items", `{}`)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("lambda_wired", func(t *testing.T) {
		t.Parallel()

		h, e, apiID := setupProxyAPIViaHandler(t, "AWS", sqsIntegrationURI)

		invoker := &proxyMockInvoker{}
		h.SetLambdaInvoker(invoker)

		rec := proxyReq(t, h, e, apiID, "/items", `{}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, sqsIntegrationURI, invoker.capturedFn)
	})
}

// TestHandleAWSIntegration_SNSTarget proves an AWS integration whose URI targets sns
// action/Publish reaches the wired SNS hook, not Lambda (gopherstack-is2a).
func TestHandleAWSIntegration_SNSTarget(t *testing.T) {
	t.Parallel()

	h, e, apiID := setupProxyAPIViaHandler(t, "AWS", snsIntegrationURI)

	publisher := &mockSNSPublisher{}
	h.SetSNSPublisher(publisher)

	topicARN := "arn:aws:sns:us-east-1:123456789012:my-topic"
	rec := proxyReq(t, h, e, apiID, "/items?TopicArn="+url.QueryEscape(topicARN), `hello world`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, publisher.called)
	assert.Equal(t, topicARN, publisher.topicARN)
	assert.Equal(t, "hello world", publisher.message)
}

func TestHandleAWSIntegration_SNSTarget_TopicUnresolved(t *testing.T) {
	t.Parallel()

	h, e, apiID := setupProxyAPIViaHandler(t, "AWS", snsIntegrationURI)

	publisher := &mockSNSPublisher{}
	h.SetSNSPublisher(publisher)

	rec := proxyReq(t, h, e, apiID, "/items", `hello world`)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.False(t, publisher.called)
}

// TestHandleAWSIntegration_SNSTarget_Unwired proves that an AWS integration pointed at
// sns action/Publish with no SNS hook wired falls through to the pre-existing
// Lambda-invoke path unchanged.
func TestHandleAWSIntegration_SNSTarget_Unwired(t *testing.T) {
	t.Parallel()

	h, e, apiID := setupProxyAPIViaHandler(t, "AWS", snsIntegrationURI)
	// SetSNSPublisher not called.

	rec := proxyReq(t, h, e, apiID, "/items", `hello world`)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestHandleAWSIntegration_SQSTarget_MalformedPath proves a malformed sqs path-style
// service_api is rejected by sqsQueuePathValid and falls through to the pre-existing
// Lambda path, not a silent SQS send (gopherstack-8mge).
func TestHandleAWSIntegration_SQSTarget_MalformedPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec string
	}{
		{"one_segment", "123456789012"},
		{"three_segments", "123456789012/my-queue/extra"},
		{"empty_account_id", "/my-queue"},
		{"empty_queue_name", "123456789012/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uri := "arn:aws:apigateway:us-east-1:sqs:path/" + tt.spec
			h, e, apiID := setupProxyAPIViaHandler(t, "AWS", uri)

			sender := &mockSQSSender{}
			h.SetSQSSender(sender)
			invoker := &proxyMockInvoker{}
			h.SetLambdaInvoker(invoker)

			rec := proxyReq(t, h, e, apiID, "/items", `{}`)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.False(t, sender.called)
			assert.Equal(t, uri, invoker.capturedFn)
		})
	}
}

// TestHandleAWSIntegration_ShortURINotParsedAsServiceTarget proves that an integration
// URI too short for the arn:aws:apigateway:{region}:{service}:path|action/{service_api}
// shape is not parsed as an sqs/sns target and still routes to Lambda (gopherstack-8mge).
func TestHandleAWSIntegration_ShortURINotParsedAsServiceTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		uri        string
		wantFnName string
	}{
		{"bare_function_name", "my-function", "my-function"},
		{
			"plain_lambda_arn",
			"arn:aws:lambda:us-east-1:123456789012:function:my-function",
			"my-function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := setupProxyAPIViaHandler(t, "AWS", tt.uri)

			sender := &mockSQSSender{}
			h.SetSQSSender(sender)
			invoker := &proxyMockInvoker{}
			h.SetLambdaInvoker(invoker)

			rec := proxyReq(t, h, e, apiID, "/items", `{}`)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.False(t, sender.called)
			assert.Equal(t, tt.wantFnName, invoker.capturedFn)
		})
	}
}

// TestHandleAWSIntegration_LambdaTarget_StillRoutesToLambda proves that an AWS
// integration whose URI matches the full apigateway path-style Lambda grammar
// (arn:aws:apigateway:{region}:lambda:path/.../functions/{arn}/invocations) is
// unaffected by the sqs/sns dispatch added for gopherstack-is2a and still invokes
// Lambda.
func TestHandleAWSIntegration_LambdaTarget_StillRoutesToLambda(t *testing.T) {
	t.Parallel()

	uri := "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/" +
		"arn:aws:lambda:us-east-1:123456789012:function:my-function/invocations"
	h, e, apiID := setupProxyAPIViaHandler(t, "AWS", uri)

	invoker := &proxyMockInvoker{}
	h.SetLambdaInvoker(invoker)

	rec := proxyReq(t, h, e, apiID, "/items", `{}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "my-function", invoker.capturedFn)
}
