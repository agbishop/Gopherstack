package kms_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

func TestCustomKeyStore_CRUD_Basic(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "audit-store",
		CustomKeyStoreType: "EXTERNAL_KEY_STORE",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.CustomKeyStoreID)

	desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	})
	require.NoError(t, err)
	assert.Len(t, desc.CustomKeyStores, 1)

	require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	}))

	require.NoError(t, b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	}))

	require.NoError(t, b.DeleteCustomKeyStore(context.Background(), &kms.DeleteCustomKeyStoreInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	}))

	desc, err = b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{})
	require.NoError(t, err)
	assert.Empty(t, desc.CustomKeyStores)
}

func TestConnectCustomKeyStore_AlreadyConnected_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "my-hsm-store",
	})
	require.NoError(t, err)
	storeID := out.CustomKeyStoreID

	require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	}))

	err = b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrCustomKeyStoreInvalidState)
}

func TestDisconnectCustomKeyStore_AlreadyDisconnected_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "my-hsm-store-2",
	})
	require.NoError(t, err)
	storeID := out.CustomKeyStoreID

	err = b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrCustomKeyStoreInvalidState)
}

func TestConnectCustomKeyStore_NotFound_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	err := b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: "nonexistent-id",
	})
	require.Error(t, err)
}

func TestDisconnectCustomKeyStore_NotFound_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	err := b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{
		CustomKeyStoreID: "nonexistent-id",
	})
	require.Error(t, err)
}

func TestDeleteCustomKeyStore_WhileConnected_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "connected-store-delete-test",
	})
	require.NoError(t, err)
	storeID := out.CustomKeyStoreID

	require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	}))

	err = b.DeleteCustomKeyStore(context.Background(), &kms.DeleteCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrCustomKeyStoreInvalidState)
}

func TestUpdateCustomKeyStore_RenameSucceeds(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "old-name",
	})
	require.NoError(t, err)
	storeID := out.CustomKeyStoreID

	require.NoError(t, b.UpdateCustomKeyStore(context.Background(), &kms.UpdateCustomKeyStoreInput{
		CustomKeyStoreID:      storeID,
		NewCustomKeyStoreName: "new-name",
	}))

	desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreID: storeID,
	})
	require.NoError(t, err)
	require.Len(t, desc.CustomKeyStores, 1)
	assert.Equal(t, "new-name", desc.CustomKeyStores[0].CustomKeyStoreName)
}

func TestXKS_CreateWithExternalKeyStoreType(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "my-xks-store",
		CustomKeyStoreType: "EXTERNAL_KEY_STORE",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.CustomKeyStoreID)
}

func TestXKS_DescribeReturnsCorrectType(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "my-xks-describe",
		CustomKeyStoreType: "EXTERNAL_KEY_STORE",
	})
	require.NoError(t, err)

	desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	})
	require.NoError(t, err)
	require.Len(t, desc.CustomKeyStores, 1)
	assert.Equal(t, "EXTERNAL_KEY_STORE", desc.CustomKeyStores[0].CustomKeyStoreType)
}

func TestXKS_InvalidTypeRejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	_, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "bad-type-store",
		CustomKeyStoreType: "INVALID_TYPE",
	})
	require.Error(t, err)
}

func TestXKS_ConnectDisconnectLifecycle(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "xks-lifecycle",
		CustomKeyStoreType: "EXTERNAL_KEY_STORE",
	})
	require.NoError(t, err)
	storeID := out.CustomKeyStoreID

	// Initial state is DISCONNECTED
	desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreID: storeID,
	})
	require.NoError(t, err)
	assert.Equal(t, kms.ConnectionStateDisconnected, desc.CustomKeyStores[0].ConnectionState)

	require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	}))

	desc2, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreID: storeID,
	})
	require.NoError(t, err)
	assert.Equal(t, kms.ConnectionStateConnected, desc2.CustomKeyStores[0].ConnectionState)

	require.NoError(t, b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	}))

	desc3, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreID: storeID,
	})
	require.NoError(t, err)
	assert.Equal(t, kms.ConnectionStateDisconnected, desc3.CustomKeyStores[0].ConnectionState)
}

func TestXKS_DescribeByName(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	_, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "my-xks-by-name",
		CustomKeyStoreType: "EXTERNAL_KEY_STORE",
	})
	require.NoError(t, err)

	desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreName: "my-xks-by-name",
	})
	require.NoError(t, err)
	require.Len(t, desc.CustomKeyStores, 1)
	assert.Equal(t, "my-xks-by-name", desc.CustomKeyStores[0].CustomKeyStoreName)
}

func TestBackendReset(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	assert.Equal(t, 1, kms.KeyCount(b))

	b.CreateAlias(context.Background(), &kms.CreateAliasInput{
		AliasName:   "alias/test-reset",
		TargetKeyID: key.KeyMetadata.KeyID,
	})
	assert.Equal(t, 1, kms.AliasCount(b))

	b.Reset()
	assert.Equal(t, 0, kms.KeyCount(b))
	assert.Equal(t, 0, kms.AliasCount(b))
	assert.Equal(t, 0, kms.GrantCount(b))
	assert.Equal(t, 0, kms.CustomKeyStoreCount(b))
}

func TestCreateCustomKeyStore_EmptyName(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "",
	})
	require.ErrorIs(t, err, kms.ErrValidation)
}

func TestCreateCustomKeyStore_InvalidType(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "my-store",
		CustomKeyStoreType: "INVALID_TYPE",
	})
	require.ErrorIs(t, err, kms.ErrValidation)
}

func TestConnectCustomKeyStore_EmptyID(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	err := b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{CustomKeyStoreID: ""})
	require.ErrorIs(t, err, kms.ErrCustomKeyStoreNotFound)
}

func TestDisconnectCustomKeyStore_EmptyID(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	err := b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{CustomKeyStoreID: ""})
	require.ErrorIs(t, err, kms.ErrCustomKeyStoreNotFound)
}

func TestDeleteCustomKeyStore_EmptyID(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	err := b.DeleteCustomKeyStore(context.Background(), &kms.DeleteCustomKeyStoreInput{CustomKeyStoreID: ""})
	require.ErrorIs(t, err, kms.ErrCustomKeyStoreNotFound)
}

func TestErrCustomKeyStoreNotFound_Sentinel(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	err := b.ConnectCustomKeyStore(
		context.Background(),
		&kms.ConnectCustomKeyStoreInput{CustomKeyStoreID: "no-such-id"},
	)
	require.ErrorIs(t, err, kms.ErrCustomKeyStoreNotFound)
}

// TestCustomKeyStore_CRUD verifies Create/Describe/Connect/Disconnect/Delete lifecycle.
func TestCustomKeyStore_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		steps   func(t *testing.T, b *kms.InMemoryBackend)
		name    string
		wantErr bool
	}{
		{
			name: "create_describe_connect_disconnect_delete",
			steps: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				// Create
				out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "test-store",
				})
				require.NoError(t, err)
				assert.NotEmpty(t, out.CustomKeyStoreID)
				storeID := out.CustomKeyStoreID

				// Describe – should find it
				desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
					CustomKeyStoreID: storeID,
				})
				require.NoError(t, err)
				require.Len(t, desc.CustomKeyStores, 1)
				assert.Equal(t, kms.ConnectionStateDisconnected, desc.CustomKeyStores[0].ConnectionState)

				// Connect
				require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
					CustomKeyStoreID: storeID,
				}))

				// Describe – should be CONNECTED
				desc2, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
					CustomKeyStoreID: storeID,
				})
				require.NoError(t, err)
				assert.Equal(t, kms.ConnectionStateConnected, desc2.CustomKeyStores[0].ConnectionState)

				// Disconnect
				require.NoError(t, b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{
					CustomKeyStoreID: storeID,
				}))

				// Delete
				require.NoError(t, b.DeleteCustomKeyStore(context.Background(), &kms.DeleteCustomKeyStoreInput{
					CustomKeyStoreID: storeID,
				}))

				// Describe – should be empty
				desc3, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{})
				require.NoError(t, err)
				assert.Empty(t, desc3.CustomKeyStores)
			},
		},
		{
			name: "delete_connected_returns_error",
			steps: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "connected-store",
				})
				require.NoError(t, err)

				require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
					CustomKeyStoreID: out.CustomKeyStoreID,
				}))

				err = b.DeleteCustomKeyStore(context.Background(), &kms.DeleteCustomKeyStoreInput{
					CustomKeyStoreID: out.CustomKeyStoreID,
				})
				require.Error(t, err)
			},
		},
		{
			name: "duplicate_name_returns_error",
			steps: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				_, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "dup-store",
				})
				require.NoError(t, err)

				_, err = b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "dup-store",
				})
				require.Error(t, err)
			},
		},
		{
			name: "connect_not_found_returns_error",
			steps: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				err := b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
					CustomKeyStoreID: "nonexistent-id",
				})
				require.Error(t, err)
			},
		},
		{
			name: "disconnect_already_disconnected_returns_error",
			steps: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "already-disconnected",
				})
				require.NoError(t, err)

				err = b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{
					CustomKeyStoreID: out.CustomKeyStoreID,
				})
				require.Error(t, err)
			},
		},
		{
			name: "describe_by_name",
			steps: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				_, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "named-store",
				})
				require.NoError(t, err)

				desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
					CustomKeyStoreName: "named-store",
				})
				require.NoError(t, err)
				require.Len(t, desc.CustomKeyStores, 1)
				assert.Equal(t, "named-store", desc.CustomKeyStores[0].CustomKeyStoreName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			tt.steps(t, b)
		})
	}
}

// TestCustomKeyStorePersistence verifies that custom key stores survive a snapshot/restore cycle.
func TestCustomKeyStorePersistence(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: "persist-store",
	})
	require.NoError(t, err)

	require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	}))

	snap := b.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := kms.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))

	desc, err := b2.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	})
	require.NoError(t, err)
	require.Len(t, desc.CustomKeyStores, 1)
	assert.Equal(t, kms.ConnectionStateConnected, desc.CustomKeyStores[0].ConnectionState)
	assert.Equal(t, "persist-store", desc.CustomKeyStores[0].CustomKeyStoreName)
}

// TestCustomKeyStore_StateGuards_WireErrorType drives the full HTTP handler path for
// gopherstack-akm2: ConnectCustomKeyStore/DisconnectCustomKeyStore/DeleteCustomKeyStore's
// own state-transition guards must surface as the real AWS CustomKeyStoreInvalidStateException
// (extraction-confirmed in each op's deserializeOpError, kms@v1.55.4 deserializers.go), not
// KMSInvalidStateException -- neither op declares that type.
func TestCustomKeyStore_StateGuards_WireErrorType(t *testing.T) {
	t.Parallel()

	t.Run("connect_already_connected", func(t *testing.T) {
		t.Parallel()

		h := ab2NewHandler(t)

		createRec := doKMSRequest(t, h, "CreateCustomKeyStore", `{"CustomKeyStoreName":"wire-connect"}`)
		require.Equal(t, http.StatusOK, createRec.Code)

		var createOut kms.CreateCustomKeyStoreOutput
		require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

		connectBody := `{"CustomKeyStoreId":"` + createOut.CustomKeyStoreID + `"}`
		require.Equal(t, http.StatusOK, doKMSRequest(t, h, "ConnectCustomKeyStore", connectBody).Code)

		rec := doKMSRequest(t, h, "ConnectCustomKeyStore", connectBody)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp kms.ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "CustomKeyStoreInvalidStateException", errResp.Type)
	})

	t.Run("disconnect_already_disconnected", func(t *testing.T) {
		t.Parallel()

		h := ab2NewHandler(t)

		createRec := doKMSRequest(t, h, "CreateCustomKeyStore", `{"CustomKeyStoreName":"wire-disconnect"}`)
		require.Equal(t, http.StatusOK, createRec.Code)

		var createOut kms.CreateCustomKeyStoreOutput
		require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

		disconnectBody := `{"CustomKeyStoreId":"` + createOut.CustomKeyStoreID + `"}`
		rec := doKMSRequest(t, h, "DisconnectCustomKeyStore", disconnectBody)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp kms.ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "CustomKeyStoreInvalidStateException", errResp.Type)
	})

	t.Run("delete_while_connected", func(t *testing.T) {
		t.Parallel()

		h := ab2NewHandler(t)

		createRec := doKMSRequest(t, h, "CreateCustomKeyStore", `{"CustomKeyStoreName":"wire-delete"}`)
		require.Equal(t, http.StatusOK, createRec.Code)

		var createOut kms.CreateCustomKeyStoreOutput
		require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

		idBody := `{"CustomKeyStoreId":"` + createOut.CustomKeyStoreID + `"}`
		require.Equal(t, http.StatusOK, doKMSRequest(t, h, "ConnectCustomKeyStore", idBody).Code)

		rec := doKMSRequest(t, h, "DeleteCustomKeyStore", idBody)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp kms.ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "CustomKeyStoreInvalidStateException", errResp.Type)
	})
}

// TestDeleteCustomKeyStore_HasKeys_WireErrorType drives the full HTTP handler path for
// gopherstack-ylkc: DeleteCustomKeyStore's still-has-keys precondition (custom_key_stores.go)
// must surface as the real AWS CustomKeyStoreHasCMKsException (extraction-confirmed in
// deserializeOpErrorDeleteCustomKeyStore, kms@v1.55.4 deserializers.go), not the generic
// KMSInternalException 500 default that kmsErrorTable fell through to when this sentinel
// had no row.
func TestDeleteCustomKeyStore_HasKeys_WireErrorType(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)

	createRec := doKMSRequest(t, h, "CreateCustomKeyStore", `{"CustomKeyStoreName":"wire-has-keys"}`)
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut kms.CreateCustomKeyStoreOutput
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

	idBody := `{"CustomKeyStoreId":"` + createOut.CustomKeyStoreID + `"}`
	require.Equal(t, http.StatusOK, doKMSRequest(t, h, "ConnectCustomKeyStore", idBody).Code)

	createKeyBody := `{"CustomKeyStoreId":"` + createOut.CustomKeyStoreID + `"}`
	require.Equal(t, http.StatusOK, doKMSRequest(t, h, "CreateKey", createKeyBody).Code)

	require.Equal(t, http.StatusOK, doKMSRequest(t, h, "DisconnectCustomKeyStore", idBody).Code)

	rec := doKMSRequest(t, h, "DeleteCustomKeyStore", idBody)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "CustomKeyStoreHasCMKsException", errResp.Type)
}

// TestEncrypt_DisconnectedCustomKeyStore_WireErrorType drives the full HTTP handler path for
// gopherstack-o3rp: DisconnectCustomKeyStore's own doc says "all attempts to ... use existing
// KMS keys in cryptographic operations will fail" while the store is disconnected
// (kms@v1.55.4 api_op_DisconnectCustomKeyStore.go). Encrypt's own deserializeOpError
// recognizes KMSInvalidStateException, not CustomKeyStoreInvalidStateException -- the latter
// is CreateKey/GenerateRandom's error, confirmed absent from Encrypt's declared list.
func TestEncrypt_DisconnectedCustomKeyStore_WireErrorType(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)

	createRec := doKMSRequest(t, h, "CreateCustomKeyStore", `{"CustomKeyStoreName":"wire-crypto-disconnected"}`)
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut kms.CreateCustomKeyStoreOutput
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

	idBody := `{"CustomKeyStoreId":"` + createOut.CustomKeyStoreID + `"}`
	require.Equal(t, http.StatusOK, doKMSRequest(t, h, "ConnectCustomKeyStore", idBody).Code)

	createKeyBody := `{"CustomKeyStoreId":"` + createOut.CustomKeyStoreID + `"}`
	createKeyRec := doKMSRequest(t, h, "CreateKey", createKeyBody)
	require.Equal(t, http.StatusOK, createKeyRec.Code)

	var createKeyOut kms.CreateKeyOutput
	require.NoError(t, json.Unmarshal(createKeyRec.Body.Bytes(), &createKeyOut))

	require.Equal(t, http.StatusOK, doKMSRequest(t, h, "DisconnectCustomKeyStore", idBody).Code)

	encryptBody := `{"KeyId":"` + createKeyOut.KeyMetadata.KeyID + `","Plaintext":"aGVsbG8="}`
	rec := doKMSRequest(t, h, "Encrypt", encryptBody)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "KMSInvalidStateException", errResp.Type)
}
