package appsync_test

// Shared test scaffolding used across the appsync_test family test files:
// backend/handler constructors, HTTP request helpers, and mock service doubles
// (Lambda invoker, DynamoDB backend) used by the GraphQL execution tests.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func newTestBackend() *appsync.InMemoryBackend {
	return appsync.NewInMemoryBackend("000000000000", "us-east-1", "http://localhost:8000")
}

func newTestHandler() (*appsync.Handler, *appsync.InMemoryBackend) {
	b := appsync.NewInMemoryBackend("000000000000", "us-east-1", "http://localhost:8000")
	h := appsync.NewHandler(b)

	return h, b
}

func doRequest(t *testing.T, handler *appsync.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf *bytes.Buffer

	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	err := handler.Handler()(c)
	require.NoError(t, err)

	return rec
}

// doRequestWithHeaders is doRequest plus caller-supplied headers (e.g.
// x-api-key), for endpoints -- like /graphql -- that check auth. Every
// current caller POSTs, so the method isn't parameterized.
func doRequestWithHeaders(
	t *testing.T, handler *appsync.Handler, path string, body any, headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	var buf *bytes.Buffer

	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(http.MethodPost, path, buf)
	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	err := handler.Handler()(c)
	require.NoError(t, err)

	return rec
}

// doV2Request performs a request with the appsync User-Agent set so the v2 router matches.
func doV2Request(t *testing.T, handler *appsync.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aws-sdk-go-v2/1.0 api/appsync/1.53.5")

	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	err := handler.Handler()(c)
	require.NoError(t, err)

	return rec
}

// mockLambdaInvoker is a test double for Lambda invocations.
type mockLambdaInvoker struct {
	payload []byte
	calls   []invocationCall
}

type invocationCall struct {
	name    string
	invType string
	payload []byte
}

func (m *mockLambdaInvoker) InvokeFunction(
	_ context.Context,
	name, invType string,
	payload []byte,
) ([]byte, int, error) {
	m.calls = append(m.calls, invocationCall{name: name, invType: invType, payload: payload})

	if m.payload != nil {
		return m.payload, 200, nil
	}

	return []byte(`{"result":"ok"}`), 200, nil
}

// mockDynamoDB is a test double for DynamoDB operations.
type mockDynamoDB struct {
	items map[string]map[string]any
}

func (m *mockDynamoDB) GetItemRaw(_ context.Context, tableName string, key map[string]any) (map[string]any, error) {
	tableKey := fmt.Sprintf("%s::%v", tableName, key)
	if item, ok := m.items[tableKey]; ok {
		return item, nil
	}

	return map[string]any{}, nil
}

func (m *mockDynamoDB) PutItemRaw(_ context.Context, tableName string, item map[string]any) error {
	if m.items == nil {
		m.items = make(map[string]map[string]any)
	}

	m.items[fmt.Sprintf("%s::%v", tableName, item)] = item

	return nil
}
