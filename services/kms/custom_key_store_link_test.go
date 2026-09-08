package kms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

func mustCreateConnectedStore(t *testing.T, b *kms.InMemoryBackend, name string) string {
	t.Helper()

	out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
		CustomKeyStoreName: name,
	})
	require.NoError(t, err)

	require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
		CustomKeyStoreID: out.CustomKeyStoreID,
	}))

	return out.CustomKeyStoreID
}

func TestCreateKey_CustomKeyStore_ConnectedStore_LinksAndReports(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	storeID := mustCreateConnectedStore(t, b, "link-store")

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{CustomKeyStoreID: storeID})
	require.NoError(t, err)
	assert.Equal(t, storeID, out.KeyMetadata.CustomKeyStoreID)

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: out.KeyMetadata.KeyID})
	require.NoError(t, err)
	assert.Equal(t, storeID, desc.KeyMetadata.CustomKeyStoreID)
}

func TestCreateKey_CustomKeyStore_NoStoreID_OmitsField(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	assert.Empty(t, out.KeyMetadata.CustomKeyStoreID)
}

func TestCreateKey_CustomKeyStore_Rejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		build   func(t *testing.T, b *kms.InMemoryBackend) kms.CreateKeyInput
		name    string
	}{
		{
			name: "nonexistent store",
			build: func(_ *testing.T, _ *kms.InMemoryBackend) kms.CreateKeyInput {
				return kms.CreateKeyInput{CustomKeyStoreID: "does-not-exist"}
			},
			wantErr: kms.ErrCustomKeyStoreNotFound,
		},
		{
			name: "disconnected store",
			build: func(t *testing.T, b *kms.InMemoryBackend) kms.CreateKeyInput {
				t.Helper()
				out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "disconnected-store",
				})
				require.NoError(t, err)

				return kms.CreateKeyInput{CustomKeyStoreID: out.CustomKeyStoreID}
			},
			wantErr: kms.ErrCustomKeyStoreInvalidState,
		},
		{
			name: "non-symmetric key spec",
			build: func(t *testing.T, b *kms.InMemoryBackend) kms.CreateKeyInput {
				t.Helper()
				storeID := mustCreateConnectedStore(t, b, "rsa-store")

				return kms.CreateKeyInput{
					CustomKeyStoreID: storeID,
					KeySpec:          "RSA_2048",
					KeyUsage:         kms.KeyUsageSignVerify,
				}
			},
			wantErr: kms.ErrUnsupportedParameter,
		},
		{
			name: "multi-region key",
			build: func(t *testing.T, b *kms.InMemoryBackend) kms.CreateKeyInput {
				t.Helper()
				storeID := mustCreateConnectedStore(t, b, "mr-store")

				return kms.CreateKeyInput{CustomKeyStoreID: storeID, MultiRegion: true}
			},
			wantErr: kms.ErrUnsupportedParameter,
		},
		{
			name: "external key store type",
			build: func(t *testing.T, b *kms.InMemoryBackend) kms.CreateKeyInput {
				t.Helper()
				out, err := b.CreateCustomKeyStore(context.Background(), &kms.CreateCustomKeyStoreInput{
					CustomKeyStoreName: "xks-store",
					CustomKeyStoreType: "EXTERNAL_KEY_STORE",
				})
				require.NoError(t, err)
				require.NoError(t, b.ConnectCustomKeyStore(context.Background(), &kms.ConnectCustomKeyStoreInput{
					CustomKeyStoreID: out.CustomKeyStoreID,
				}))

				return kms.CreateKeyInput{CustomKeyStoreID: out.CustomKeyStoreID}
			},
			wantErr: kms.ErrUnsupportedParameter,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend(t)
			input := tc.build(t, b)

			_, err := b.CreateKey(context.Background(), &input)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestDeleteCustomKeyStore_WithLinkedKey_Rejected(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	storeID := mustCreateConnectedStore(t, b, "occupied-store")

	_, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{CustomKeyStoreID: storeID})
	require.NoError(t, err)

	require.NoError(t, b.DisconnectCustomKeyStore(context.Background(), &kms.DisconnectCustomKeyStoreInput{
		CustomKeyStoreID: storeID,
	}))

	err = b.DeleteCustomKeyStore(context.Background(), &kms.DeleteCustomKeyStoreInput{CustomKeyStoreID: storeID})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrCustomKeyStoreHasKeys)
}
