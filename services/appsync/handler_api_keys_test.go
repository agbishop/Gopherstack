package appsync_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestHandler_CreateApiKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*appsync.InMemoryBackend) string
		body       map[string]any
		name       string
		wantStatus int
		wantKeyID  bool
	}{
		{
			name: "creates_api_key_successfully",
			setup: func(b *appsync.InMemoryBackend) string {
				api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

				return api.APIID
			},
			body:       map[string]any{"description": "test key", "expires": time.Now().AddDate(0, 0, 30).Unix()},
			wantStatus: http.StatusCreated,
			wantKeyID:  true,
		},
		{
			name: "returns_404_for_missing_api",
			setup: func(_ *appsync.InMemoryBackend) string {
				return "nonexistent"
			},
			body:       map[string]any{"description": "test key"},
			wantStatus: http.StatusNotFound,
			wantKeyID:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			apiID := tt.setup(b)

			rec := doRequest(t, h, http.MethodPost, "/v1/apis/"+apiID+"/apikeys", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantKeyID {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				key, ok := resp["apiKey"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, key["id"])
			}
		})
	}
}

func TestHandler_CreateAPIKey_Da2Prefix(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()
	// Create an API.
	body := map[string]any{"name": "TestAPI", "authenticationType": "API_KEY"}
	rec := doRequest(t, h, http.MethodPost, "/v1/apis", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	gqlAPI := resp["graphqlApi"].(map[string]any)
	apiID := gqlAPI["apiId"].(string)

	keyBody := map[string]any{"description": "test key"}
	rec2 := doRequest(t, h, http.MethodPost, "/v1/apis/"+apiID+"/apikeys", keyBody)
	require.Equal(t, http.StatusCreated, rec2.Code)

	var keyResp map[string]any
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&keyResp))
	apiKey := keyResp["apiKey"].(map[string]any)
	keyID := apiKey["id"].(string)
	require.GreaterOrEqual(t, len(keyID), 4, "key ID must be at least 4 characters")
	assert.Equal(t, "da2-", keyID[:4])
}

func TestHandler_ListApiKeys(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAPIKey(api.APIID, "key1", 0)
	require.NoError(t, err)
	_, err = b.CreateAPIKey(api.APIID, "key2", 0)
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/apikeys", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	keys := resp["apiKeys"].([]any)
	assert.Len(t, keys, 2)
}

func TestHandler_DeleteApiKey(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key, err := b.CreateAPIKey(api.APIID, "test", 0)
	require.NoError(t, err)

	// Delete the key.
	rec := doRequest(t, h, http.MethodDelete, "/v1/apis/"+api.APIID+"/apikeys/"+key.ID, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Second delete returns 404.
	rec2 := doRequest(t, h, http.MethodDelete, "/v1/apis/"+api.APIID+"/apikeys/"+key.ID, nil)
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

func TestHandler_UpdateApiKey(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key, err := b.CreateAPIKey(api.APIID, "original", 0)
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPut, "/v1/apis/"+api.APIID+"/apikeys/"+key.ID,
		map[string]any{"description": "updated"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	apiKey := resp["apiKey"].(map[string]any)
	assert.Equal(t, "updated", apiKey["description"])
}

func TestHandler_UpdateApiKey_NotFound(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPut, "/v1/apis/"+api.APIID+"/apikeys/nonexistent",
		map[string]any{"description": "x"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_CreateAPIKey_MaxLimit(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

	keyBody := map[string]any{"description": "k1"}

	// Create up to the limit (50).
	for i := range 50 {
		rec := doRequest(t, h, http.MethodPost, "/v1/apis/"+api.APIID+"/apikeys", keyBody)
		require.Equal(t, http.StatusCreated, rec.Code, "key %d should succeed", i+1)
	}

	// 51st key exceeds limit.
	rec := doRequest(t, h, http.MethodPost, "/v1/apis/"+api.APIID+"/apikeys", keyBody)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
