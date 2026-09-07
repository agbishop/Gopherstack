package azureservicebus_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

func newTestHandler(t *testing.T) *azureservicebus.Handler {
	t.Helper()

	return azureservicebus.NewHandler(azureservicebus.NewInMemoryBackend())
}

func doRequest(
	t *testing.T, h *azureservicebus.Handler, method, path string, body []byte, headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, http.NoBody)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))

	return rec
}

func TestHandler_EntityLevel(t *testing.T) {
	t.Parallel()

	t.Run("create and delete queue", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, http.MethodPut, "/q1", nil, nil)
		assert.Equal(t, http.StatusCreated, rec.Code)

		// Idempotent retry.
		rec = doRequest(t, h, http.MethodPut, "/q1", nil, nil)
		assert.Equal(t, http.StatusOK, rec.Code)

		rec = doRequest(t, h, http.MethodDelete, "/q1", nil, nil)
		assert.Equal(t, http.StatusOK, rec.Code)

		rec = doRequest(t, h, http.MethodDelete, "/q1", nil, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("create topic via body sniff", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		body := []byte(`<entry><content><TopicDescription/></content></entry>`)
		rec := doRequest(t, h, http.MethodPut, "/t1", body, nil)
		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.True(t, h.Backend.TopicExists("t1"))
	})

	t.Run("create topic via query param", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, http.MethodPut, "/t1?type=topic", nil, nil)
		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.True(t, h.Backend.TopicExists("t1"))
	})

	t.Run("unsupported verb", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doRequest(t, h, http.MethodPatch, "/q1", nil, nil)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestHandler_SubscriptionLevel(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPut, "/t1?type=topic", nil, nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = doRequest(t, h, http.MethodPut, "/t1/subscriptions/s1", nil, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, h.Backend.SubscriptionExists("t1", "s1"))

	rec = doRequest(t, h, http.MethodPut, "/missing-topic/subscriptions/s1", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = doRequest(t, h, http.MethodDelete, "/t1/subscriptions/s1", nil, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, h.Backend.SubscriptionExists("t1", "s1"))

	rec = doRequest(t, h, http.MethodDelete, "/t1/subscriptions/s1", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_MessageLifecycle_Queue(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPut, "/q1", nil, nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Send.
	rec = doRequest(t, h, http.MethodPost, "/q1/messages", []byte("hello"), nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Sending to a subscription path is invalid.
	rec = doRequest(t, h, http.MethodPost, "/q1/subscriptions/s1/messages", []byte("x"), nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	// Peek-lock.
	rec = doRequest(t, h, http.MethodPost, "/q1/messages/head", nil, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())

	bpHeader := rec.Header().Get("Brokerproperties")
	require.NotEmpty(t, bpHeader)

	var bp map[string]any

	require.NoError(t, json.Unmarshal([]byte(bpHeader), &bp))
	messageID, _ := bp["MessageId"].(string)
	lockToken, _ := bp["LockToken"].(string)
	require.NotEmpty(t, messageID)
	require.NotEmpty(t, lockToken)

	location := rec.Header().Get("Location")
	assert.Equal(t, "/q1/messages/"+messageID+"/"+lockToken, location)

	// No more messages available.
	rec = doRequest(t, h, http.MethodPost, "/q1/messages/head", nil, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Complete with wrong lock token fails.
	rec = doRequest(t, h, http.MethodDelete, "/q1/messages/"+messageID+"/wrong-token", nil, nil)
	assert.Equal(t, http.StatusGone, rec.Code)

	// Complete with correct token succeeds.
	rec = doRequest(t, h, http.MethodDelete, location, nil, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_MessageLifecycle_Abandon(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPut, "/q1", nil, nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = doRequest(t, h, http.MethodPost, "/q1/messages", []byte("hello"), nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = doRequest(t, h, http.MethodPost, "/q1/messages/head", nil, nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	location := rec.Header().Get("Location")
	require.NotEmpty(t, location)

	rec = doRequest(t, h, http.MethodPut, location, nil, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Message is immediately available again after Abandon.
	rec = doRequest(t, h, http.MethodPost, "/q1/messages/head", nil, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_TopicFanOut(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/t1?type=topic", nil, nil).Code)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/t1/subscriptions/s1", nil, nil).Code)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/t1/subscriptions/s2", nil, nil).Code)

	sendRec := doRequest(t, h, http.MethodPost, "/t1/messages", []byte("fanout"), nil)
	require.Equal(t, http.StatusCreated, sendRec.Code)

	for _, sub := range []string{"s1", "s2"} {
		subRec := doRequest(t, h, http.MethodPost, "/t1/subscriptions/"+sub+"/messages/head", nil, nil)
		require.Equal(t, http.StatusCreated, subRec.Code, "subscription %s", sub)
		assert.Equal(t, "fanout", subRec.Body.String())
	}
}

func TestHandler_DeadLetterQueuePath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/q1", nil, nil).Code)
	require.Equal(t, http.StatusCreated,
		doRequest(t, h, http.MethodPost, "/q1/messages", []byte("m1"), nil).Code)

	for range azureservicebus.MaxDeliveryCount {
		rec := doRequest(t, h, http.MethodPost, "/q1/messages/head", nil, nil)
		require.Equal(t, http.StatusCreated, rec.Code)

		location := rec.Header().Get("Location")
		rec = doRequest(t, h, http.MethodPut, location, nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// The main queue is now empty; the message should be in the dead-letter
	// sub-queue.
	rec := doRequest(t, h, http.MethodPost, "/q1/messages/head", nil, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = doRequest(t, h, http.MethodPost, "/q1/$DeadLetterQueue/messages/head", nil, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "m1", rec.Body.String())
}

func TestHandler_SASValidation(t *testing.T) {
	t.Parallel()

	const key = "dGVzdC1rZXktdmFsdWU="

	h := azureservicebus.NewHandler(azureservicebus.NewInMemoryBackend()).WithSASValidation(key)

	sig := azureservicebus.SignSAS("q1", 9999999999, key)
	validAuth := "SharedAccessSignature sr=q1&sig=" + url.QueryEscape(
		sig,
	) + "&se=9999999999&skn=RootManageSharedAccessKey"

	rec := doRequest(t, h, http.MethodPut, "/q1", nil, map[string]string{"Authorization": validAuth})
	assert.Equal(t, http.StatusCreated, rec.Code, "valid SAS signature should be accepted")

	rec = doRequest(t, h, http.MethodPut, "/q2", nil, map[string]string{
		"Authorization": "SharedAccessSignature sr=q2&sig=bad&se=9999999999&skn=RootManageSharedAccessKey",
	})
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "invalid SAS signature should be rejected when validation is on")

	// No Authorization header at all is still accepted (anonymous), matching
	// this repo's permissive-by-default auth stance.
	rec = doRequest(t, h, http.MethodPut, "/q3", nil, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_Reset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/q1", nil, nil).Code)
	require.True(t, h.Backend.QueueExists("q1"))

	h.Reset()

	assert.False(t, h.Backend.QueueExists("q1"))
}
