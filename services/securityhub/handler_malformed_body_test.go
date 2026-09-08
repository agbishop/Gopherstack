package securityhub_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/securityhub"
)

// doRawRequest sends a raw (possibly malformed) body, unlike doRequest which
// only ever sends valid JSON via json.Marshal.
func doRawRequest(t *testing.T, h *securityhub.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKID/20240101/us-east-1/securityhub/aws4_request")
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// TestMalformedJSONBody_DoesNotEnableHubV2 proves a malformed request body is
// rejected with 400 and the matched operation never runs. decodeJSONBody
// (handler.go) used to write the 400 itself via c.JSON and return that
// call's (always-nil) result, so handleREST's `if err != nil` check never
// fired and dispatch proceeded to the matched operation with body == nil.
// EnableSecurityHubV2's only field (Tags) is optional, so a nil body reads
// as a valid, empty request: without this fix, POST /hubv2 with malformed
// JSON still enabled SecurityHub V2 even though the client had already
// received a 400 -- worse than a mere double write, since it's a real,
// unintended state mutation (gopherstack-3t96, the gopherstack-8haq shape).
func TestMalformedJSONBody_DoesNotEnableHubV2(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRawRequest(t, h, http.MethodPost, "/hubv2", []byte(`{"Tags":`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// The hub must not have been enabled by the rejected request: a fresh
	// GET must still see it as not-enabled (404 ResourceNotFoundException),
	// not the 200 it would return had EnableSecurityHubV2 actually run.
	rec = doRequest(t, h, http.MethodGet, "/hubv2", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"SecurityHub V2 must not be enabled after a malformed EnableSecurityHubV2 request")
}

// TestMalformedJSONBody_DoesNotDoubleWrite proves a malformed request body
// to an operation whose fields ARE required (unlike EnableSecurityHubV2)
// does not reach that operation's own validation and write a second
// response on top of the already-committed 400. Before the fix, handleREST
// dispatched CreateActionTarget with body == nil, whose own "Name is
// required" check then wrote typedErrorResponse a second time -- a status
// code alone can't distinguish this from the correct single-write path,
// since both write a 400; the response body two writes produce is what
// gives it away (a real client's JSON decoder sees concatenated garbage).
func TestMalformedJSONBody_DoesNotDoubleWrite(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	enableHub(t, h)

	rec := doRawRequest(t, h, http.MethodPost, "/actionTargets", []byte(`{"Name":`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &body)
	require.NoError(t, err, "a single write must produce one well-formed JSON body, got: %s", rec.Body.String())
}
