package secretsmanager_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/secretsmanager"
)

// newSMHandler builds a handler backed by a fresh in-memory backend.
func newSMHandler() *secretsmanager.Handler {
	return secretsmanager.NewHandler(secretsmanager.NewInMemoryBackend())
}

// doR1Request invokes an SM handler via an X-Amz-Target action, setting the
// JSON-1.1 content type.
func doR1Request(t *testing.T, h *secretsmanager.Handler, action, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", action)

	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))

	return rec
}

// doSMRequest sends a POST with the given X-Amz-Target and JSON body.
func doSMRequest(t *testing.T, h *secretsmanager.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Amz-Target", target)
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))

	return rec
}

// doSMRequestInRegion is doSMRequest with an explicit AWS region, via the
// same X-Amz-Region header httputils.ExtractRegionFromRequest reads.
func doSMRequestInRegion(
	t *testing.T, h *secretsmanager.Handler, region, target, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Amz-Target", target)
	req.Header.Set("X-Amz-Region", region)
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))

	return rec
}

// ptr64 returns the address of an int64 literal.
func ptr64(v int64) *int64 {
	p := new(int64)
	*p = v

	return p
}

// contains wraps slices.Contains for readable test assertions.
func contains(s []string, target string) bool {
	return slices.Contains(s, target)
}

// mockLambdaInvoker is a test mock for the LambdaInvoker interface.
type mockLambdaInvoker struct {
	calls      []lambdaCall
	invokeErr  error
	invokeResp []byte
}

type lambdaCall struct {
	name           string
	invocationType string
	payload        []byte
}

func (m *mockLambdaInvoker) InvokeFunction(
	_ context.Context,
	name, invocationType string,
	payload []byte,
) ([]byte, int, error) {
	m.calls = append(m.calls, lambdaCall{name: name, invocationType: invocationType, payload: payload})
	if m.invokeErr != nil {
		return nil, 500, m.invokeErr
	}
	if m.invokeResp != nil {
		return m.invokeResp, 200, nil
	}

	return []byte(`{}`), 200, nil
}
