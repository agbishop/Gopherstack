package identitystore //nolint:testpackage // pins errResponseWritten + unexported return sites (gopherstack-n7nk)

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
)

const internalTestStoreID = "d-1234567890"

func newInternalTestHandler() *Handler {
	return NewHandler(NewInMemoryBackend("123456789012", config.DefaultRegion))
}

func newInternalTestContext(t *testing.T, body []byte) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e := echo.New()

	return e.NewContext(req, rec), rec
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	require.NoError(t, err)

	return b
}

// TestRequireIdentityStoreIDReturnsSentinel pins BOTH of
// requireIdentityStoreID's return sites (validation.go): the empty-id branch
// and the pattern-mismatch branch must each return errResponseWritten after
// writing the 400, not the writer's nil -- that is the entire fix for
// gopherstack-n7nk. All ~18 direct call sites only check `err != nil`, so a
// nil here silently lets the caller's mutation or second write proceed.
func TestRequireIdentityStoreIDReturnsSentinel(t *testing.T) {
	t.Parallel()

	h := newInternalTestHandler()

	tests := []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "malformed", id: "not-a-valid-store-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, rec := newInternalTestContext(t, nil)

			err := h.requireIdentityStoreID(c, tt.id)
			require.ErrorIs(t, err, errResponseWritten)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}

	t.Run("valid_returns_nil", func(t *testing.T) {
		t.Parallel()

		c, _ := newInternalTestContext(t, nil)
		assert.NoError(t, h.requireIdentityStoreID(c, internalTestStoreID))
	})
}

// TestParseAlternateIDRequestReturnsSentinel pins every one of
// parseAlternateIDRequest's own write-and-return points (handler.go): a
// malformed body, a missing AlternateIdentifier, an invalid
// UniqueAttribute.AttributePath, and an invalid ExternalId Issuer/Id must
// each return errResponseWritten, not the writer's nil -- this helper has
// the exact same bug shape as requireIdentityStoreID and feeds
// handleGetGroupID/handleGetUserID.
func TestParseAlternateIDRequestReturnsSentinel(t *testing.T) {
	t.Parallel()

	h := newInternalTestHandler()

	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "invalid_json_body",
			body: []byte(`{bad`),
		},
		{
			name: "missing_alternate_identifier",
			body: mustJSON(t, map[string]any{"IdentityStoreId": internalTestStoreID}),
		},
		{
			name: "invalid_unique_attribute_path",
			body: mustJSON(t, map[string]any{
				"IdentityStoreId": internalTestStoreID,
				"AlternateIdentifier": map[string]any{
					"UniqueAttribute": map[string]any{
						"AttributePath":  "not a valid path!!",
						"AttributeValue": "x",
					},
				},
			}),
		},
		{
			name: "invalid_external_id",
			body: mustJSON(t, map[string]any{
				"IdentityStoreId": internalTestStoreID,
				"AlternateIdentifier": map[string]any{
					"ExternalId": map[string]any{
						"Issuer": "has space",
						"Id":     "x",
					},
				},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, rec := newInternalTestContext(t, tt.body)

			_, err := h.parseAlternateIDRequest(c, tt.body)
			require.ErrorIs(t, err, errResponseWritten)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}

	t.Run("valid_returns_nil_error", func(t *testing.T) {
		t.Parallel()

		body := mustJSON(t, map[string]any{
			"IdentityStoreId": internalTestStoreID,
			"AlternateIdentifier": map[string]any{
				"UniqueAttribute": map[string]any{
					"AttributePath":  "userName",
					"AttributeValue": "someone",
				},
			},
		})
		c, _ := newInternalTestContext(t, body)

		result, err := h.parseAlternateIDRequest(c, body)
		require.NoError(t, err)
		assert.Equal(t, internalTestStoreID, result.storeID)
	})
}

// TestHandlerTranslatesSentinelToNil pins the single translation point
// (Handler(), handler.go): dispatch's returned error must satisfy
// errors.Is(err, errResponseWritten) for a rejected request (proving the
// sentinel actually reaches the top of the chain unmodified through every
// intermediate `if err != nil { return err }`), and Handler() must convert
// that into a nil return -- otherwise the sentinel escapes to echo as a
// genuine handler error on a response that already committed a 400,
// producing a spurious second write/log from echo's own
// "response already written" guard. This was the load-bearing piece
// elasticache's fix initially left unpinned (gopherstack-8haq).
func TestHandlerTranslatesSentinelToNil(t *testing.T) {
	t.Parallel()

	h := newInternalTestHandler()
	body := mustJSON(t, map[string]any{
		"IdentityStoreId": "not-a-valid-store-id",
		"DisplayName":     "X",
	})

	c, rec := newInternalTestContext(t, body)

	dispatchErr := h.dispatch(t.Context(), c, opCreateGroup, body)
	require.ErrorIs(
		t,
		dispatchErr,
		errResponseWritten,
		"requireIdentityStoreID's sentinel must propagate unmodified through handleCreateGroup's `if err != nil` check",
	)

	c2, rec2 := newInternalTestContext(t, body)
	c2.Request().Header.Set("X-Amz-Target", targetPrefix+opCreateGroup)

	err := h.Handler()(c2)
	require.NoError(t, err, "Handler must translate errResponseWritten back to nil at the top of the dispatch chain")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}
