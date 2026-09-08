package azurearm_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

// insecureTLSTransport returns an *http.Transport that skips certificate
// verification, for talking to the dedicated ARM listener's self-signed
// certificate in tests (production clients rely on system trust +
// SSL_CERT_FILE instead -- see test/terraform/azure).
func insecureTLSTransport() *http.Transport {
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // test-only
}

// newTestHandler builds a Handler wired the same way Provider.Init does,
// without going through cli.go/service.AppContext.
func newTestHandler(t *testing.T) *azurearm.Handler {
	t.Helper()

	backend, registry := azurearm.NewTestRegistryWithStorage()
	issuer, err := aadauth.NewIssuer()
	require.NoError(t, err)

	settings := azurearm.DefaultSettings()

	return azurearm.NewHandler(backend, registry, issuer, settings)
}

// doRequest issues method/path (and optional body) against h.Handler() via
// httptest, returning the decoded JSON response body and status.
func doRequest(t *testing.T, h *azurearm.Handler, method, path string, body []byte) (int, map[string]any) {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Host = "host:10006"

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()

	e := newEchoServer(h)
	e.ServeHTTP(rec, req)

	var decoded map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}

	return rec.Code, decoded
}

func TestHandler_MetadataEndpoints(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	status, body := doRequestRaw(t, h, http.MethodGet, "/metadata/endpoints?api-version=2022-09-01", nil)
	require.Equal(t, http.StatusOK, status)

	var docs []map[string]any

	require.NoError(t, json.Unmarshal(body, &docs))
	require.Len(t, docs, 1)
	assert.Equal(t, "gopherstack", docs[0]["name"])
}

func TestHandler_OpenIDConfigurationAndInstanceDiscovery(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	tenant := h.Settings.TenantID

	status, body := doRequest(t, h, http.MethodGet, "/"+tenant+"/v2.0/.well-known/openid-configuration", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, body["issuer"], tenant)

	status, body = doRequest(t, h, http.MethodGet, "/common/discovery/instance?api-version=1.1", nil)
	require.Equal(t, http.StatusOK, status)
	assert.NotEmpty(t, body["tenant_discovery_endpoint"])
}

func TestHandler_TokenEndpoint(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	tenant := h.Settings.TenantID

	tests := []struct {
		name string
		path string
	}{
		{name: "v1 token endpoint", path: "/" + tenant + "/oauth2/token"},
		{name: "v2 token endpoint", path: "/" + tenant + "/oauth2/v2.0/token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			form := url.Values{
				"grant_type":    {"client_credentials"},
				"client_id":     {h.Settings.ClientID},
				"client_secret": {h.Settings.ClientSecret},
				"scope":         {"https://management.azure.com/.default"},
			}

			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(form.Encode()))
			req.Host = "host:10006"
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rec := httptest.NewRecorder()
			newEchoServer(h).ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "Bearer", resp["token_type"])
			assert.NotEmpty(t, resp["access_token"])
		})
	}
}

func TestHandler_ResourceGroupCRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	sub := h.Settings.SubscriptionID
	base := "/subscriptions/" + sub

	status, body := doRequest(t, h, http.MethodPut, base+"/resourcegroups/rg1",
		[]byte(`{"location":"westus","tags":{"env":"dev"}}`))
	require.Equal(t, http.StatusCreated, status)
	assert.Equal(t, "westus", body["location"])

	// Case-insensitive re-fetch via "resourceGroups".
	status, body = doRequest(t, h, http.MethodGet, base+"/resourceGroups/rg1", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "rg1", body["name"])

	// PUT again is an update, not a create.
	status, _ = doRequest(t, h, http.MethodPut, base+"/resourcegroups/rg1", []byte(`{"location":"eastus"}`))
	require.Equal(t, http.StatusOK, status)

	status, listBody := doRequestList(t, h, http.MethodGet, base+"/resourcegroups")
	require.Equal(t, http.StatusOK, status)
	values, ok := listBody["value"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 1)

	status, _ = doRequest(t, h, http.MethodDelete, base+"/resourcegroups/rg1", nil)
	assert.Equal(t, http.StatusOK, status)

	status, _ = doRequest(t, h, http.MethodGet, base+"/resourcegroups/rg1", nil)
	assert.Equal(t, http.StatusNotFound, status)

	// DELETE is idempotent.
	status, _ = doRequest(t, h, http.MethodDelete, base+"/resourcegroups/rg1", nil)
	assert.Equal(t, http.StatusNoContent, status)
}

func TestHandler_GenericResourceAndListKeys(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	sub := h.Settings.SubscriptionID
	base := "/subscriptions/" + sub

	_, _ = doRequest(t, h, http.MethodPut, base+"/resourcegroups/rg1", []byte(`{"location":"westus"}`))

	resourcePath := base + "/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/acct1"

	status, body := doRequest(t, h, http.MethodPut, resourcePath, []byte(`{"location":"westus","sku":{"name":"Standard_LRS"}}`))
	require.Equal(t, http.StatusCreated, status)
	assert.Equal(t, "acct1", body["name"])

	status, body = doRequest(t, h, http.MethodGet, resourcePath, nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "acct1", body["name"])

	status, body = doRequest(t, h, http.MethodPost, resourcePath+"/listKeys", nil)
	require.Equal(t, http.StatusOK, status)

	keys, ok := body["keys"].([]any)
	require.True(t, ok)
	assert.Len(t, keys, 2)

	status, listBody := doRequestList(t, h, http.MethodGet,
		base+"/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts")
	require.Equal(t, http.StatusOK, status)
	values, ok := listBody["value"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 1)

	status, _ = doRequest(t, h, http.MethodDelete, resourcePath, nil)
	assert.Equal(t, http.StatusOK, status)
}

func TestHandler_ProviderRegistration(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	sub := h.Settings.SubscriptionID
	base := "/subscriptions/" + sub

	status, body := doRequest(t, h, http.MethodGet, base+"/providers/Microsoft.Storage", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "NotRegistered", body["registrationState"])

	status, body = doRequest(t, h, http.MethodPost, base+"/providers/Microsoft.Storage/register", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "Registered", body["registrationState"])

	status, listBody := doRequestList(t, h, http.MethodGet, base+"/providers")
	require.Equal(t, http.StatusOK, status)
	values, ok := listBody["value"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, values)

	status, _ = doRequest(t, h, http.MethodGet, base+"/providers/Microsoft.NotRegistered", nil)
	assert.Equal(t, http.StatusNotFound, status)
}

func TestHandler_SubscriptionsAndTenants(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	sub := h.Settings.SubscriptionID

	status, listBody := doRequestList(t, h, http.MethodGet, "/subscriptions")
	require.Equal(t, http.StatusOK, status)
	values, ok := listBody["value"].([]any)
	require.True(t, ok)
	require.Len(t, values, 1)

	status, body := doRequest(t, h, http.MethodGet, "/subscriptions/"+sub, nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, sub, body["subscriptionId"])

	status, listBody = doRequestList(t, h, http.MethodGet, "/tenants")
	require.Equal(t, http.StatusOK, status)
	values, ok = listBody["value"].([]any)
	require.True(t, ok)
	require.Len(t, values, 1)
}

func TestHandler_NotFoundPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	status, body := doRequest(t, h, http.MethodGet, "/nonsense/path", nil)
	assert.Equal(t, http.StatusNotFound, status)
	assert.NotEmpty(t, body["error"])
}

func TestHandler_ResetClearsState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	sub := h.Settings.SubscriptionID

	_, _ = doRequest(t, h, http.MethodPut, "/subscriptions/"+sub+"/resourcegroups/rg1", []byte(`{"location":"westus"}`))

	h.Reset()

	status, _ := doRequest(t, h, http.MethodGet, "/subscriptions/"+sub+"/resourcegroups/rg1", nil)
	assert.Equal(t, http.StatusNotFound, status)
}

// --- test helpers below ---

// newEchoServer wraps h.Handler() in a bare *echo.Echo (no telemetry
// middleware -- these tests exercise wire behavior, not observability), so
// httptest.NewRecorder-based requests can be dispatched exactly like
// StartWorker's real listener would.
func newEchoServer(h *azurearm.Handler) http.Handler {
	e := echo.New()
	e.Any("/*", h.Handler())

	return e
}

func doRequestRaw(t *testing.T, h *azurearm.Handler, method, path string, body []byte) (int, []byte) {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Host = "host:10006"

	rec := httptest.NewRecorder()
	newEchoServer(h).ServeHTTP(rec, req)

	return rec.Code, rec.Body.Bytes()
}

func doRequestList(t *testing.T, h *azurearm.Handler, method, path string) (int, map[string]any) {
	t.Helper()

	return doRequest(t, h, method, path, nil)
}

// TestHandler_StartWorker_BindsServesHTTPS proves the dedicated listener
// binds, serves TLS, and responds -- covering AZURE.md section 10.8's "serve
// the whole services/azurearm listener over HTTPS from the first commit"
// requirement -- then shuts down cleanly, leaving no leaked goroutine (see
// leak_test.go's TestMain).
func TestHandler_StartWorker_BindsServesHTTPS(t *testing.T) {
	t.Parallel()

	port := freeEphemeralPort(t)

	h := newTestHandler(t)
	h.Port = port

	ctx := t.Context()
	require.NoError(t, h.StartWorker(ctx))

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.Shutdown(shutdownCtx)
	})

	client := &http.Client{Transport: insecureTLSTransport()}
	reqURL := fmt.Sprintf("https://127.0.0.1:%d/metadata/endpoints?api-version=2022-09-01", port)

	require.Eventually(t, func() bool {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
		if reqErr != nil {
			return false
		}

		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond, "dedicated HTTPS listener should become reachable")
}

func TestHandler_StartWorker_BindFailureIsSynchronous(t *testing.T) {
	t.Parallel()

	port := reserveEphemeralPort(t)

	h := newTestHandler(t)
	h.Port = port

	err := h.StartWorker(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bind port")
}

func TestHandler_Shutdown_NilServerIsNoop(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	assert.NotPanics(t, func() {
		h.Shutdown(t.Context())
	})
}

func TestHandler_RouteMatcherAndMetadata(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	assert.False(t, h.RouteMatcher()(nil))
	assert.Equal(t, 0, h.MatchPriority())
	assert.Equal(t, "AzureARM", h.Name())
	assert.NotEmpty(t, h.GetSupportedOperations())
}

func freeEphemeralPort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.NoError(t, l.Close())

	return addr.Port
}

func reserveEphemeralPort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return addr.Port
}
