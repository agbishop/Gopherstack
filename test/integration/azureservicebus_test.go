package integration_test

// This file deliberately does NOT use azure-sdk-for-go's
// sdk/messaging/azservicebus client. That package is built entirely on top
// of github.com/Azure/go-amqp -- it speaks AMQP 1.0 over TLS (or AMQP frames
// over WebSockets) and has no HTTP/REST transport option at all. gopherstack's
// services/azureservicebus deliberately targets Service Bus's Brokered
// Messaging REST API instead (see AZURE.md section 9's M5 rationale: a full
// binary AMQP 1.0 stack is a materially larger and different-shaped effort
// than any other Azure service in this repo implements), so there is no SDK
// client that can be pointed at it unmodified. This is a genuine
// SDK-compatibility gap, documented in services/azureservicebus/PARITY.md's
// families.sdk_compat entry -- not an oversight. The tests below therefore
// exercise the REST surface directly via net/http, mirroring what a
// hand-rolled REST client (or curl) would do against a real Service Bus
// namespace's Brokered Messaging API.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sbBrokerProperties mirrors services/azureservicebus/handler.go's
// brokerProperties wire shape (a private type in that package, so the
// integration test -- which only talks HTTP -- redeclares the subset of
// fields it needs).
type sbBrokerProperties struct {
	MessageID string `json:"MessageId,omitempty"`
	LockToken string `json:"LockToken,omitempty"`
}

// sbRequest performs one Service Bus REST call against azureServiceBusEndpoint
// and returns the response, skipping the test if the endpoint isn't
// available (mirroring createAzureQueueClient's t.Skip pattern).
func sbRequest(t *testing.T, method, path string, body []byte, headers map[string]string) *http.Response {
	t.Helper()

	if azureServiceBusEndpoint == "" {
		t.Skip("Azure Service Bus endpoint not available (mapped port could not be determined)")
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequestWithContext(t.Context(), method, azureServiceBusEndpoint+path, bodyReader)
	require.NoError(t, err)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func TestIntegration_AzureServiceBus_QueueSendPeekLockComplete(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	queue := "test-queue-" + uuid.NewString()

	// CreateQueue.
	resp := sbRequest(t, http.MethodPut, "/"+queue, nil, nil)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Send.
	const messageBody = "hello from gopherstack azureservicebus"

	resp = sbRequest(t, http.MethodPost, "/"+queue+"/messages", []byte(messageBody), map[string]string{
		"Content-Type": "text/plain",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// PeekLock.
	resp = sbRequest(t, http.MethodPost, "/"+queue+"/messages/head", nil, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	gotBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, messageBody, string(gotBody))

	var bp sbBrokerProperties

	require.NoError(t, json.Unmarshal([]byte(resp.Header.Get("Brokerproperties")), &bp))
	require.NotEmpty(t, bp.MessageID)
	require.NotEmpty(t, bp.LockToken)

	location := resp.Header.Get("Location")
	require.NotEmpty(t, location)

	// Complete via the Location header returned by PeekLock.
	resp = sbRequest(t, http.MethodDelete, location, nil, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Queue should now be empty.
	resp = sbRequest(t, http.MethodPost, "/"+queue+"/messages/head", nil, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// DeleteQueue.
	resp = sbRequest(t, http.MethodDelete, "/"+queue, nil, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestIntegration_AzureServiceBus_TopicSubscriptionFanOut(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	topic := "test-topic-" + uuid.NewString()

	// CreateTopic (via the ?type=topic escape hatch -- see
	// services/azureservicebus/PARITY.md's entity_kind_detection note for why
	// a hand-built REST request needs it instead of a proper Atom+XML body).
	resp := sbRequest(t, http.MethodPut, "/"+topic+"?type=topic", nil, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Create two subscriptions.
	for _, sub := range []string{"sub-a", "sub-b"} {
		subResp := sbRequest(t, http.MethodPut, "/"+topic+"/subscriptions/"+sub, nil, nil)
		require.Equal(t, http.StatusCreated, subResp.StatusCode)
	}

	// Send once to the topic.
	const messageBody = "fan-out message"

	resp = sbRequest(t, http.MethodPost, "/"+topic+"/messages", []byte(messageBody), nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Both subscriptions independently receive their own copy.
	for _, sub := range []string{"sub-a", "sub-b"} {
		subResp := sbRequest(t, http.MethodPost, "/"+topic+"/subscriptions/"+sub+"/messages/head", nil, nil)
		require.Equal(t, http.StatusCreated, subResp.StatusCode, "subscription %s should have received a copy", sub)

		gotBody, err := io.ReadAll(subResp.Body)
		require.NoError(t, err)
		assert.Equal(t, messageBody, string(gotBody))

		location := subResp.Header.Get("Location")
		require.NotEmpty(t, location)

		completeResp := sbRequest(t, http.MethodDelete, location, nil, nil)
		assert.Equal(t, http.StatusOK, completeResp.StatusCode)
	}

	// DeleteTopic (cascades to its subscriptions).
	resp = sbRequest(t, http.MethodDelete, "/"+topic, nil, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
