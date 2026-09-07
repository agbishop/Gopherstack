package azureservicebus_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

func TestHandler_CreateEntity_AtomXMLKindDetection(t *testing.T) {
	t.Parallel()

	const queueBody = `<entry xmlns="http://www.w3.org/2005/Atom"><content type="application/xml">` +
		`<QueueDescription xmlns="http://schemas.microsoft.com/netservices/2010/10/servicebus/connect">` +
		`<LockDuration>PT2M</LockDuration><MaxDeliveryCount>4</MaxDeliveryCount>` +
		`</QueueDescription></content></entry>`

	const topicBody = `<entry xmlns="http://www.w3.org/2005/Atom"><content type="application/xml">` +
		`<TopicDescription xmlns="http://schemas.microsoft.com/servicebus/2010/10/">` +
		`<DefaultMessageTimeToLive>PT30S</DefaultMessageTimeToLive>` +
		`</TopicDescription></content></entry>`

	const substringSniffTopicBody = `<entry><content><TopicDescription/></content></entry>`

	tests := []struct {
		name      string
		path      string
		body      []byte
		wantTopic bool
	}{
		{name: "atom queue body", path: "/q1", body: []byte(queueBody), wantTopic: false},
		{name: "atom topic body", path: "/t1", body: []byte(topicBody), wantTopic: true},
		{
			name:      "malformed xml falls through to substring sniff",
			path:      "/t2",
			body:      []byte(substringSniffTopicBody),
			wantTopic: true,
		},
		{
			name:      "malformed xml with no sniff hit defaults to queue",
			path:      "/q2",
			body:      []byte("<entry><content>"),
			wantTopic: false,
		},
		{name: "empty body defaults to queue", path: "/q3", body: nil, wantTopic: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPut, tt.path, tt.body, nil)
			require.Equal(t, http.StatusCreated, rec.Code)

			name := tt.path[1:]
			assert.Equal(t, tt.wantTopic, h.Backend.TopicExists(name))
			assert.Equal(t, !tt.wantTopic, h.Backend.QueueExists(name))
		})
	}

	t.Run("query param wins over an atom body that says otherwise", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doRequest(t, h, http.MethodPut, "/q4?type=topic", []byte(queueBody), nil)
		require.Equal(t, http.StatusCreated, rec.Code)
		assert.True(t, h.Backend.TopicExists("q4"))
	})

	t.Run("?type=queue also wins over an atom body that says otherwise", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doRequest(t, h, http.MethodPut, "/q4b?type=queue", []byte(topicBody), nil)
		require.Equal(t, http.StatusCreated, rec.Code)
		assert.True(t, h.Backend.QueueExists("q4b"), "?type=queue must be honored, not just ?type=topic")
		assert.False(t, h.Backend.TopicExists("q4b"))
	})

	t.Run("atom body config is stored on the queue", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doRequest(t, h, http.MethodPut, "/q5", []byte(queueBody), nil)
		require.Equal(t, http.StatusCreated, rec.Code)

		info, err := h.Backend.GetQueueInfo("q5")
		require.NoError(t, err)
		assert.Equal(t, 2*time.Minute, info.LockDuration)
		assert.Equal(t, 4, info.MaxDeliveryCount)
	})
}

// atomQueueBodyWithLockDuration and atomSubscriptionBodyWithLockDuration
// build minimal Atom+XML create bodies carrying only a LockDuration, for
// TestHandler_CreateQueueAndSubscription_LockDurationValidation's
// boundary cases.
func atomQueueBodyWithLockDuration(lockDuration string) []byte {
	return []byte(`<entry xmlns="http://www.w3.org/2005/Atom"><content type="application/xml">` +
		`<QueueDescription xmlns="http://schemas.microsoft.com/netservices/2010/10/servicebus/connect">` +
		`<LockDuration>` + lockDuration + `</LockDuration>` +
		`</QueueDescription></content></entry>`)
}

func atomSubscriptionBodyWithLockDuration(lockDuration string) []byte {
	return []byte(`<entry xmlns="http://www.w3.org/2005/Atom"><content type="application/xml">` +
		`<SubscriptionDescription xmlns="http://schemas.microsoft.com/netservices/2010/10/servicebus/connect">` +
		`<LockDuration>` + lockDuration + `</LockDuration>` +
		`</SubscriptionDescription></content></entry>`)
}

// TestHandler_CreateQueueAndSubscription_LockDurationValidation covers real
// Service Bus's documented 5-minute LockDuration maximum (see PARITY.md and
// MaxLockDuration's doc comment): a create request specifying more is
// rejected 400 Bad Request rather than silently persisted.
func TestHandler_CreateQueueAndSubscription_LockDurationValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		lockDuration string
		wantStatus   int
	}{
		{name: "exactly at the 5-minute maximum is accepted", lockDuration: "PT5M", wantStatus: http.StatusCreated},
		{
			name: "one second over the maximum is rejected", lockDuration: "PT5M1S",
			wantStatus: http.StatusBadRequest,
		},
		{name: "6 minutes is rejected", lockDuration: "PT6M", wantStatus: http.StatusBadRequest},
		{name: "well under the maximum is accepted", lockDuration: "PT1M", wantStatus: http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run("queue: "+tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(
				t,
				h,
				http.MethodPut,
				"/q-"+tt.lockDuration,
				atomQueueBodyWithLockDuration(tt.lockDuration),
				nil,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})

		t.Run("subscription: "+tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			topicRec := doRequest(t, h, http.MethodPut, "/t-"+tt.lockDuration+"?type=topic", nil, nil)
			require.Equal(t, http.StatusCreated, topicRec.Code)

			rec := doRequest(t, h, http.MethodPut, "/t-"+tt.lockDuration+"/subscriptions/s",
				atomSubscriptionBodyWithLockDuration(tt.lockDuration), nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_GetAndListEntities(t *testing.T) {
	t.Parallel()

	const queueBody = `<entry xmlns="http://www.w3.org/2005/Atom"><content type="application/xml">` +
		`<QueueDescription xmlns="http://schemas.microsoft.com/netservices/2010/10/servicebus/connect">` +
		`<LockDuration>PT2M</LockDuration><MaxDeliveryCount>4</MaxDeliveryCount>` +
		`</QueueDescription></content></entry>`

	h := newTestHandler(t)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/q1", []byte(queueBody), nil).Code)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/t1?type=topic", nil, nil).Code)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/t1/subscriptions/s1", nil, nil).Code)

	t.Run("get queue", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/q1", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/atom+xml;type=entry;charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Body.String(), "<QueueDescription")
		assert.Contains(t, rec.Body.String(), "PT2M")
	})

	t.Run("get topic", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/t1", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<TopicDescription")
	})

	t.Run("get subscription", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/t1/subscriptions/s1", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<SubscriptionDescription")
	})

	t.Run("get missing entity 404s", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/missing", nil, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("get missing subscription 404s", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/t1/subscriptions/missing", nil, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("list queues via $Resources", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/$Resources/Queues", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/atom+xml;type=feed;charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Body.String(), "<title>q1</title>")
	})

	t.Run("list topics via $Resources", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/$Resources/Topics", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<title>t1</title>")
	})

	t.Run("list subscriptions of a topic", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/t1/subscriptions", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<title>s1</title>")
	})

	t.Run("$Resources with an unknown collection is a bad request", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodGet, "/$Resources/Bogus", nil, nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestHandler_PeekLock_LongPoll(t *testing.T) {
	t.Parallel()

	t.Run("timeout=0 behaves exactly like immediate PeekLock", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/q1", nil, nil).Code)

		rec := doRequest(t, h, http.MethodPost, "/q1/messages/head?timeout=0", nil, nil)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("a message already available is returned immediately even with a timeout", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/q1", nil, nil).Code)
		require.Equal(t, http.StatusCreated,
			doRequest(t, h, http.MethodPost, "/q1/messages", []byte("m1"), nil).Code)

		rec := doRequest(t, h, http.MethodPost, "/q1/messages/head?timeout=5", nil, nil)
		require.Equal(t, http.StatusCreated, rec.Code)
		assert.Equal(t, "m1", rec.Body.String())
	})

	t.Run("an unparseable or negative timeout is treated as zero", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/q1", nil, nil).Code)

		rec := doRequest(t, h, http.MethodPost, "/q1/messages/head?timeout=-5", nil, nil)
		assert.Equal(t, http.StatusNoContent, rec.Code)

		rec = doRequest(t, h, http.MethodPost, "/q1/messages/head?timeout=bogus", nil, nil)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestHandler_Reset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusCreated, doRequest(t, h, http.MethodPut, "/q1", nil, nil).Code)
	require.True(t, h.Backend.QueueExists("q1"))

	h.Reset()

	assert.False(t, h.Backend.QueueExists("q1"))
}
