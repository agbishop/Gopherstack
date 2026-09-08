package appsync_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestCreateAPIKey_ExpiryDefaulted_WhenZero(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// expires=0 → backend assigns default expiry (7 days from now).
	key, err := b.CreateAPIKey(api.APIID, "test key", 0)
	require.NoError(t, err)
	assert.Positive(t, key.Expires, "expiry should be defaulted to a future timestamp")
}

func TestCreateAPIKey_ExpiryOutOfBounds_TooFarInFuture(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// expires far in the future (> 365 days) → ApiKeyValidityOutOfBoundsException.
	_, err = b.CreateAPIKey(api.APIID, "test key", 9999999999)
	require.ErrorIs(t, err, appsync.ErrAPIKeyValidityOutOfBounds)
}

func TestUpdateAPIKey_ExpiryRoundTrip(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	initial := time.Now().AddDate(0, 0, 30).Unix()
	key, err := b.CreateAPIKey(api.APIID, "initial desc", initial)
	require.NoError(t, err)

	updatedExpiry := time.Now().AddDate(0, 0, 60).Unix()
	updated, err := b.UpdateAPIKey(api.APIID, key.ID, "updated desc", updatedExpiry)
	require.NoError(t, err)
	assert.Equal(t, "updated desc", updated.Description)
	assert.Equal(t, updatedExpiry, updated.Expires)
}

func TestInMemoryBackend_CreateAPIKey_DefaultExpiry(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key, err := b.CreateAPIKey(api.APIID, "test", 0)
	require.NoError(t, err)

	// Expires should be in the future.
	assert.Positive(t, key.Expires)
}

func TestInMemoryBackend_CreateAPIKey_Da2Prefix(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key, err := b.CreateAPIKey(api.APIID, "test", 0)
	require.NoError(t, err)

	assert.Greater(t, len(key.ID), 4, "key ID should be longer than the prefix")
	assert.Equal(t, "da2-", key.ID[:4])
}

func TestInMemoryBackend_ListAndDeleteAPIKeys(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key1, err := b.CreateAPIKey(api.APIID, "k1", 0)
	require.NoError(t, err)
	_, err = b.CreateAPIKey(api.APIID, "k2", 0)
	require.NoError(t, err)

	// List returns 2 keys.
	keys, err := b.ListAPIKeys(api.APIID)
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Delete one.
	err = b.DeleteAPIKey(api.APIID, key1.ID)
	require.NoError(t, err)

	// List returns 1 key.
	keys, err = b.ListAPIKeys(api.APIID)
	require.NoError(t, err)
	assert.Len(t, keys, 1)

	// Delete non-existent returns error.
	err = b.DeleteAPIKey(api.APIID, "nonexistent")
	require.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestInMemoryBackend_UpdateAPIKey(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key, err := b.CreateAPIKey(api.APIID, "original", 0)
	require.NoError(t, err)

	updated, err := b.UpdateAPIKey(api.APIID, key.ID, "updated", 0)
	require.NoError(t, err)
	assert.Equal(t, "updated", updated.Description)

	// Not found key returns error.
	_, err = b.UpdateAPIKey(api.APIID, "nonexistent", "x", 0)
	require.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestInMemoryBackend_ListAPIKeys_APINotFound(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	_, err := b.ListAPIKeys("nonexistent")
	require.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestInMemoryBackend_DeleteAPIKey_APINotFound(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	err := b.DeleteAPIKey("nonexistent", "key-id")
	require.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestInMemoryBackend_CreateAPIKey_MaxKeysLimit(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Create up to the limit (50).
	for i := range 50 {
		_, err = b.CreateAPIKey(api.APIID, fmt.Sprintf("key%d", i+1), 0)
		require.NoError(t, err, "key %d should succeed", i+1)
	}

	// 51st key should fail with the real ApiKeyLimitExceededException, not a
	// generic BadRequestException.
	_, err = b.CreateAPIKey(api.APIID, "key51", 0)
	require.ErrorIs(t, err, appsync.ErrAPIKeyLimitExceeded)
}

func TestInMemoryBackend_CreateAPIKey_ExpiresOutOfBounds_TooFarInFuture(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Set expires to 2 years from now (> 365 days) -> ApiKeyValidityOutOfBoundsException.
	farFuture := time.Now().AddDate(2, 0, 0).Unix()
	_, err = b.CreateAPIKey(api.APIID, "key1", farFuture)
	require.ErrorIs(t, err, appsync.ErrAPIKeyValidityOutOfBounds)
}

func TestInMemoryBackend_CreateAPIKey_ExpiresOutOfBounds_TooSoon(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Less than 1 day from now -> ApiKeyValidityOutOfBoundsException.
	tooSoon := time.Now().Add(1 * time.Hour).Unix()
	_, err = b.CreateAPIKey(api.APIID, "key1", tooSoon)
	require.ErrorIs(t, err, appsync.ErrAPIKeyValidityOutOfBounds)
}

func TestInMemoryBackend_ListAPIKeys_FilterExpired(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Real AWS's ApiKeyValidityOutOfBoundsException makes it impossible to
	// create an already-expired key through the public API; simulate a key
	// that has since aged past its expiry via the test-only setter.
	expired, err := b.CreateAPIKey(api.APIID, "expired", 0)
	require.NoError(t, err)
	b.SetAPIKeyExpiry(api.APIID, expired.ID, time.Now().Add(-24*time.Hour).Unix())

	// Create a valid key.
	_, err = b.CreateAPIKey(api.APIID, "valid", 0)
	require.NoError(t, err)

	keys, err := b.ListAPIKeys(api.APIID)
	require.NoError(t, err)
	// Only the non-expired key should be returned.
	assert.Len(t, keys, 1)
	assert.Equal(t, "valid", keys[0].Description)
}

func TestInMemoryBackend_UpdateAPIKey_ExpiryOutOfBounds(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("MyAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key, err := b.CreateAPIKey(api.APIID, "desc", 0)
	require.NoError(t, err)

	// Attempt to update with an expiry far in the future (10 years).
	farFuture := time.Now().AddDate(10, 0, 0).Unix()
	_, err = b.UpdateAPIKey(api.APIID, key.ID, "", farFuture)
	require.ErrorIs(t, err, appsync.ErrAPIKeyValidityOutOfBounds)
}

func TestInMemoryBackend_UpdateAPIKey_ValidExpiryUnchanged(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("MyAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	key, err := b.CreateAPIKey(api.APIID, "desc", 0)
	require.NoError(t, err)

	// Set a valid expiry within the cap.
	validExpiry := time.Now().AddDate(0, 0, 30).Unix()
	updated, err := b.UpdateAPIKey(api.APIID, key.ID, "", validExpiry)
	require.NoError(t, err)
	assert.Equal(t, validExpiry, updated.Expires, "valid expiry should be stored as-is")
}

func TestBackend_SweepExpiredAPIKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(b *appsync.InMemoryBackend) string
		name          string
		wantEvicted   int
		wantKeyExists bool
	}{
		{
			name: "no_keys",
			setup: func(b *appsync.InMemoryBackend) string {
				api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				require.NoError(t, err)

				return api.APIID
			},
			wantEvicted:   0,
			wantKeyExists: false,
		},
		{
			// A key can organically age past its expiry after creation, even
			// though real AWS's ApiKeyValidityOutOfBoundsException makes it
			// impossible to create one already expired via the public API --
			// SetAPIKeyExpiry simulates that aging without waiting.
			name: "expired_key_is_swept",
			setup: func(b *appsync.InMemoryBackend) string {
				api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				require.NoError(t, err)

				key, err := b.CreateAPIKey(api.APIID, "expired", 0)
				require.NoError(t, err)
				b.SetAPIKeyExpiry(api.APIID, key.ID, time.Now().Add(-1*time.Hour).Unix())

				return api.APIID
			},
			wantEvicted:   1,
			wantKeyExists: false,
		},
		{
			name: "valid_key_not_swept",
			setup: func(b *appsync.InMemoryBackend) string {
				api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				require.NoError(t, err)
				// Create a key that expires far in the future.
				_, err = b.CreateAPIKey(api.APIID, "valid", time.Now().AddDate(0, 0, 2).Unix())
				require.NoError(t, err)

				return api.APIID
			},
			wantEvicted:   0,
			wantKeyExists: true,
		},
		{
			name: "mixed_keys",
			setup: func(b *appsync.InMemoryBackend) string {
				api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				require.NoError(t, err)

				expired, err := b.CreateAPIKey(api.APIID, "expired", 0)
				require.NoError(t, err)
				b.SetAPIKeyExpiry(api.APIID, expired.ID, time.Now().Add(-1*time.Hour).Unix())

				_, err = b.CreateAPIKey(api.APIID, "valid", time.Now().AddDate(0, 0, 2).Unix())
				require.NoError(t, err)

				return api.APIID
			},
			wantEvicted:   1,
			wantKeyExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			apiID := tt.setup(b)

			evicted := b.SweepExpiredAPIKeys()
			assert.Equal(t, tt.wantEvicted, evicted)

			keys, err := b.ListAPIKeys(apiID)
			require.NoError(t, err)

			if tt.wantKeyExists {
				assert.NotEmpty(t, keys)
			} else {
				// Either no keys or only non-expired ones remain.
				for _, k := range keys {
					assert.True(t, k.Expires == 0 || k.Expires > time.Now().Unix())
				}
			}
		})
	}
}

// TestListAPIKeys_Pagination verifies maxResults/nextToken on Listapikeys.
func TestListAPIKeys_Pagination(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/v1/apis", map[string]any{
		"name":               "key-api",
		"authenticationType": "API_KEY",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var apiOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &apiOut))
	apiID := apiOut["graphqlApi"].(map[string]any)["apiId"].(string)

	for range 4 {
		rec = doRequest(t, h, http.MethodPost, fmt.Sprintf("/v1/apis/%s/apikeys", apiID), map[string]any{
			"description": "test key",
		})
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	tests := []struct {
		name          string
		path          string
		wantLen       int
		wantNextToken bool
	}{
		{
			name:          "no_limit_returns_all",
			path:          fmt.Sprintf("/v1/apis/%s/apikeys", apiID),
			wantLen:       4,
			wantNextToken: false,
		},
		{
			name:          "page1_two_items",
			path:          fmt.Sprintf("/v1/apis/%s/apikeys?maxResults=2", apiID),
			wantLen:       2,
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			listRec := doRequest(t, h, http.MethodGet, tt.path, nil)
			require.Equal(t, http.StatusOK, listRec.Code)

			var out struct {
				NextToken string           `json:"nextToken"`
				APIKeys   []map[string]any `json:"apiKeys"`
			}
			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &out))
			assert.Len(t, out.APIKeys, tt.wantLen)
			if tt.wantNextToken {
				assert.NotEmpty(t, out.NextToken)
			} else {
				assert.Empty(t, out.NextToken)
			}
		})
	}
}
