package apigatewaymanagementapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigatewaymanagementapi"
)

func TestAdmin_ListConnections(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("alpha", "10.0.0.1", "ua-a", nil)
	require.NoError(t, err)

	_, err = h.Backend.CreateConnection("beta", "10.0.0.2", "ua-b", nil)
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodGet, "/_gopherstack/apigwmgmt/connections", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Connections []apigatewaymanagementapi.Connection `json:"connections"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Connections, 2)
}

func TestAdmin_ListConnections_QueryFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("alpha-1", "10.0.0.1", "Chromium", nil)
	require.NoError(t, err)

	_, err = h.Backend.CreateConnection("beta-2", "192.168.1.5", "Firefox", nil)
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodGet, "/_gopherstack/apigwmgmt/connections?q=192", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Connections []apigatewaymanagementapi.Connection `json:"connections"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Connections, 1)
	assert.Equal(t, "beta-2", resp.Connections[0].ConnectionID)
}

func TestAdmin_SimulateConnection(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body, _ := json.Marshal(map[string]string{
		"connectionId": "sim-1",
		"sourceIp":     "1.2.3.4",
		"userAgent":    "ua",
	})

	rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/connections", body)
	assert.Equal(t, http.StatusCreated, rec.Code)

	conns := h.Backend.ListConnections()
	require.Len(t, conns, 1)
	assert.Equal(t, "sim-1", conns[0].ConnectionID)
}

func TestAdmin_SimulateDuplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("dupe", "1.1.1.1", "ua", nil)
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]string{"connectionId": "dupe"})
	rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/connections", body)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestAdmin_SimulateEmptyConnectionID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body, _ := json.Marshal(map[string]string{"connectionId": ""})
	rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/connections", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdmin_GetMessages(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("conn-msgs", "1.1.1.1", "ua", nil)
	require.NoError(t, err)
	require.NoError(t, h.Backend.PostToConnection("conn-msgs", []byte("hello")))
	require.NoError(t, h.Backend.PostToConnection("conn-msgs", []byte("world")))

	rec := doRequest(t, h, http.MethodGet, "/_gopherstack/apigwmgmt/connections/conn-msgs/messages", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Messages []struct {
			Data  string `json:"data"`
			Bytes int    `json:"bytes"`
		} `json:"messages"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Messages, 2)
	assert.Equal(t, "hello", resp.Messages[0].Data)
	assert.Equal(t, 5, resp.Messages[0].Bytes)
	assert.Equal(t, "world", resp.Messages[1].Data)
}

func TestAdmin_GetMessagesNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/_gopherstack/apigwmgmt/connections/missing/messages", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAdmin_ClearMessages(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("clr", "1.1.1.1", "ua", nil)
	require.NoError(t, err)
	require.NoError(t, h.Backend.PostToConnection("clr", []byte("data")))

	rec := doRequest(t, h, http.MethodDelete, "/_gopherstack/apigwmgmt/connections/clr/messages", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, h.Backend.GetMessages("clr"))
}

func TestAdmin_GetTimeline(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("tl", "1.1.1.1", "ua", nil)
	require.NoError(t, err)
	require.NoError(t, h.Backend.PostToConnection("tl", []byte("data")))
	require.NoError(t, h.Backend.PingConnection("tl"))

	rec := doRequest(t, h, http.MethodGet, "/_gopherstack/apigwmgmt/connections/tl/timeline", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Events []apigatewaymanagementapi.LifecycleEvent `json:"events"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Events, 3)
	assert.Equal(t, apigatewaymanagementapi.EventConnected, resp.Events[0].Type)
	assert.Equal(t, apigatewaymanagementapi.EventMessage, resp.Events[1].Type)
	assert.Equal(t, apigatewaymanagementapi.EventPing, resp.Events[2].Type)
}

func TestAdmin_PingConnection(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		conn, err := h.Backend.CreateConnection("ping-conn", "1.1.1.1", "ua", nil)
		require.NoError(t, err)

		originalActive := conn.LastActiveAt
		time.Sleep(2 * time.Millisecond)

		rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/connections/ping-conn/ping", nil)
		assert.Equal(t, http.StatusNoContent, rec.Code)

		updated, err := h.Backend.GetConnection("ping-conn")
		require.NoError(t, err)
		assert.True(t, updated.LastActiveAt.After(originalActive))
	})
}

func TestAdmin_Broadcast(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, id := range []string{"a", "b", "c"} {
		_, err := h.Backend.CreateConnection(id, "1.1.1.1", "ua", nil)
		require.NoError(t, err)
	}

	body, _ := json.Marshal(map[string]string{"data": "broadcast-payload"})

	rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/broadcast", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Delivered int `json:"delivered"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 3, resp.Delivered)

	for _, id := range []string{"a", "b", "c"} {
		msgs := h.Backend.GetMessages(id)
		require.Len(t, msgs, 1)
		assert.Equal(t, []byte("broadcast-payload"), msgs[0].Data)
	}
}

func TestAdmin_BroadcastTooLarge(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("c", "1.1.1.1", "ua", nil)
	require.NoError(t, err)

	big := bytes.Repeat([]byte("x"), 130*1024)
	body, _ := json.Marshal(map[string]string{"data": string(big)})

	rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/broadcast", body)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestAdmin_Stats(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateConnection("s1", "1.1.1.1", "ua", nil)
	require.NoError(t, err)
	require.NoError(t, h.Backend.PostToConnection("s1", []byte("hi")))

	rec := doRequest(t, h, http.MethodGet, "/_gopherstack/apigwmgmt/stats", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var stats apigatewaymanagementapi.Stats
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats))
	assert.Equal(t, 1, stats.ActiveConnections)
	assert.Equal(t, int64(1), stats.TotalConnections)
	assert.Equal(t, int64(1), stats.TotalMessages)
	assert.Equal(t, int64(2), stats.TotalBytesSent)
	assert.Equal(t, 1, stats.BufferedMessages)
}

func TestAdmin_Prune(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		_, err := h.Backend.CreateConnection("old", "1.1.1.1", "ua", nil)
		require.NoError(t, err)

		// Gap must exceed the prune threshold below.
		time.Sleep(500 * time.Millisecond)

		_, err = h.Backend.CreateConnection("new", "2.2.2.2", "ua", nil)
		require.NoError(t, err)

		rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/prune", []byte(`{"idleSeconds": 0}`))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Len(t, h.Backend.ListConnections(), 2)

		pruned := h.Backend.PruneIdle(200 * time.Millisecond)
		assert.Equal(t, []string{"old"}, pruned)
		assert.Len(t, h.Backend.ListConnections(), 1)
	})
}

func TestAdmin_Prune_ClosesDownstream(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		downstream := make(chan []byte, 1)
		_, err := h.Backend.CreateConnection("idle-conn", "1.1.1.1", "ua", downstream)
		require.NoError(t, err)

		// Gap must exceed the prune threshold below.
		time.Sleep(300 * time.Millisecond)

		pruned := h.Backend.PruneIdle(100 * time.Millisecond)
		assert.Equal(t, []string{"idle-conn"}, pruned)

		select {
		case _, open := <-downstream:
			assert.False(t, open, "downstream channel must be closed after PruneIdle")
		default:
			t.Fatal("downstream channel must be closed (readable as closed) after PruneIdle")
		}
	})
}

func TestAdmin_PruneZeroThreshold_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/prune", []byte(`{"idleSeconds": 0}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the response body serialises as [] not null so the frontend can safely call .length
	assert.Contains(t, rec.Body.String(), `"pruned":[]`)
}

func TestAdmin_BadJSON(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/_gopherstack/apigwmgmt/connections", []byte("not json"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdmin_RouteMatcher_Matches(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/_gopherstack/apigwmgmt/connections", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.True(t, h.RouteMatcher()(c))
	assert.Equal(t, "Admin:ListConnections", h.ExtractOperation(c))
}
