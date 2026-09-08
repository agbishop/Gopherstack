package kms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

func TestGenerateDataKey_AES128(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	out, err := b.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{KeyID: keyID, KeySpec: "AES_128"})
	require.NoError(t, err)
	assert.Len(t, out.Plaintext, 16)
	assert.NotEmpty(t, out.CiphertextBlob)
}

func TestGenerateDataKey_AES256(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	out, err := b.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{KeyID: keyID, KeySpec: "AES_256"})
	require.NoError(t, err)
	assert.Len(t, out.Plaintext, 32)
}

func TestGenerateDataKey_NumberOfBytes(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)
	n := int32(64)

	out, err := b.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
		KeyID: keyID, NumberOfBytes: &n,
	})
	require.NoError(t, err)
	assert.Len(t, out.Plaintext, 64)
}

func TestGenerateDataKey_BothSpecAndBytes_Rejected(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)
	n := int32(32)

	_, err := b.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
		KeyID:         keyID,
		KeySpec:       "AES_256",
		NumberOfBytes: &n,
	})
	require.Error(t, err)
}

func TestGenerateDataKeyWithoutPlaintext(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	out, err := b.GenerateDataKeyWithoutPlaintext(context.Background(), &kms.GenerateDataKeyWithoutPlaintextInput{
		KeyID:   keyID,
		KeySpec: "AES_256",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.CiphertextBlob)
	// Plaintext field should be absent (nil or empty).
}

func TestGenerateDataKeyPair_RSA(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	out, err := b.GenerateDataKeyPair(context.Background(), &kms.GenerateDataKeyPairInput{
		KeyID:       keyID,
		KeyPairSpec: "RSA_2048",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.PublicKey)
	assert.NotEmpty(t, out.PrivateKeyCiphertextBlob)
	assert.NotEmpty(t, out.PrivateKeyPlaintext)
}

func TestGenerateDataKeyPairWithoutPlaintext_Basic(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	out, err := b.GenerateDataKeyPairWithoutPlaintext(
		context.Background(),
		&kms.GenerateDataKeyPairWithoutPlaintextInput{
			KeyID:       keyID,
			KeyPairSpec: "RSA_2048",
		},
	)
	require.NoError(t, err)
	assert.NotEmpty(t, out.PublicKey)
	assert.NotEmpty(t, out.PrivateKeyCiphertextBlob)
}

func TestGenerateDataKeyPair_AllSpecs(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	specs := []string{"RSA_2048", "RSA_3072", "RSA_4096", "ECC_NIST_P256", "ECC_NIST_P384", "ECC_NIST_P521"}
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {
			t.Parallel()
			pairOut, err := b.GenerateDataKeyPair(context.Background(), &kms.GenerateDataKeyPairInput{
				KeyID:       keyID,
				KeyPairSpec: spec,
			})
			require.NoError(t, err)
			assert.NotEmpty(t, pairOut.PublicKey)
			assert.NotEmpty(t, pairOut.PrivateKeyPlaintext)
			assert.NotEmpty(t, pairOut.PrivateKeyCiphertextBlob)
			assert.Equal(t, spec, pairOut.KeyPairSpec)
		})
	}
}

func TestGenerateDataKeyPair_EmptySpec(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)

	_, err = b.GenerateDataKeyPair(context.Background(), &kms.GenerateDataKeyPairInput{
		KeyID:       key.KeyMetadata.KeyID,
		KeyPairSpec: "",
	})
	require.ErrorIs(t, err, kms.ErrUnsupportedParameter)
}

// TestKMSBackendGenerateDataKey verifies data key generation.
func TestKMSBackendGenerateDataKey(t *testing.T) {
	t.Parallel()

	t.Run("AES256", func(t *testing.T) {
		t.Parallel()

		backend := kms.NewInMemoryBackend()
		key, _ := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})

		out, err := backend.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
			KeyID:   key.KeyMetadata.KeyID,
			KeySpec: "AES_256",
		})
		require.NoError(t, err)
		assert.Len(t, out.Plaintext, 32)
		assert.NotEmpty(t, out.CiphertextBlob)
		assert.Equal(t, key.KeyMetadata.Arn, out.KeyID)
	})

	t.Run("AES128", func(t *testing.T) {
		t.Parallel()

		backend := kms.NewInMemoryBackend()
		key, _ := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})

		out, err := backend.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
			KeyID:   key.KeyMetadata.KeyID,
			KeySpec: "AES_128",
		})
		require.NoError(t, err)
		assert.Len(t, out.Plaintext, 16)
	})

	t.Run("NumberOfBytes", func(t *testing.T) {
		t.Parallel()

		backend := kms.NewInMemoryBackend()
		key, _ := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})

		n := int32(24)
		out, err := backend.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
			KeyID:         key.KeyMetadata.KeyID,
			NumberOfBytes: &n,
		})
		require.NoError(t, err)
		assert.Len(t, out.Plaintext, 24)
	})
}

// TestKMSGenerateDataKeyErrors tests error paths in GenerateDataKey.
func TestKMSGenerateDataKeyErrors(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()

	// Key not found
	_, err := backend.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
		KeyID:   "nonexistent-key",
		KeySpec: "AES_256",
	})
	require.ErrorIs(t, err, kms.ErrKeyNotFound)
}

// TestKMSBackendInvalidKeyUsage verifies that using a SIGN_VERIFY key for encryption returns
// InvalidKeyUsageException matching the AWS SDK v2 *types.InvalidKeyUsageException error.
func TestKMSBackendInvalidKeyUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		operation func(b *kms.InMemoryBackend, keyID string) error
		name      string
	}{
		{
			name: "Encrypt_with_sign_verify_key",
			operation: func(b *kms.InMemoryBackend, keyID string) error {
				_, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("test")})

				return err
			},
		},
		{
			name: "GenerateDataKey_with_sign_verify_key",
			operation: func(b *kms.InMemoryBackend, keyID string) error {
				_, err := b.GenerateDataKey(
					context.Background(),
					&kms.GenerateDataKeyInput{KeyID: keyID, KeySpec: "AES_256"},
				)

				return err
			},
		},
		{
			name: "ReEncrypt_with_sign_verify_dest_key",
			operation: func(b *kms.InMemoryBackend, keyID string) error {
				encKey, createErr := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				if createErr != nil {
					return createErr
				}

				encOut, encErr := b.Encrypt(context.Background(), &kms.EncryptInput{
					KeyID:     encKey.KeyMetadata.KeyID,
					Plaintext: []byte("test"),
				})
				if encErr != nil {
					return encErr
				}

				_, err := b.ReEncrypt(context.Background(), &kms.ReEncryptInput{
					DestinationKeyID: keyID,
					CiphertextBlob:   encOut.CiphertextBlob,
				})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			createOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeyUsage: kms.KeyUsageSignVerify})
			require.NoError(t, err)

			err = tt.operation(b, createOut.KeyMetadata.KeyID)

			require.ErrorIs(t, err, kms.ErrInvalidKeyUsage)
		})
	}
}

// TestKMSGenerateDataKey_MaxBytes verifies that GenerateDataKey enforces the AWS 1024-byte
// maximum for NumberOfBytes.
func TestKMSGenerateDataKey_MaxBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		numberOfBytes int32
		wantErr       bool
	}{
		{name: "max_allowed_1024", numberOfBytes: 1024},
		{name: "exceeds_max_1025", numberOfBytes: 1025, wantErr: true},
		{name: "zero_uses_default", numberOfBytes: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)

			inp := &kms.GenerateDataKeyInput{KeyID: key.KeyMetadata.KeyID}
			if tt.numberOfBytes != 0 {
				inp.NumberOfBytes = &tt.numberOfBytes
			}

			out, genErr := b.GenerateDataKey(context.Background(), inp)

			if tt.wantErr {
				require.ErrorIs(t, genErr, kms.ErrInvalidDataKeySize)

				return
			}

			require.NoError(t, genErr)
			assert.NotEmpty(t, out.Plaintext)
		})
	}
}

// TestKMSGenerateDataKeyEncryptionContext verifies that encryption context is
// passed through GenerateDataKey and must match when decrypting the data key.
func TestKMSGenerateDataKeyEncryptionContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		genCtx        map[string]string
		decryptCtx    map[string]string
		name          string
		wantDecryptOK bool
	}{
		{
			name:          "matching_context",
			genCtx:        map[string]string{"app": "myapp"},
			decryptCtx:    map[string]string{"app": "myapp"},
			wantDecryptOK: true,
		},
		{
			name:          "no_context",
			genCtx:        nil,
			decryptCtx:    nil,
			wantDecryptOK: true,
		},
		{
			name:          "mismatched_context",
			genCtx:        map[string]string{"app": "myapp"},
			decryptCtx:    nil,
			wantDecryptOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)

			genOut, err := b.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
				KeyID:             key.KeyMetadata.KeyID,
				EncryptionContext: tt.genCtx,
			})
			require.NoError(t, err)

			decOut, decErr := b.Decrypt(context.Background(), &kms.DecryptInput{
				CiphertextBlob:    genOut.CiphertextBlob,
				EncryptionContext: tt.decryptCtx,
			})

			if tt.wantDecryptOK {
				require.NoError(t, decErr)
				assert.Equal(t, genOut.Plaintext, decOut.Plaintext)
			} else {
				require.Error(t, decErr)
			}
		})
	}
}

// TestGenerateDataKeyPair verifies key pair generation with plaintext private key.
func TestGenerateDataKeyPair(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		keyPairSpec string
		wantErr     bool
	}{
		{name: "rsa_2048", keyPairSpec: "RSA_2048", wantErr: false},
		{name: "rsa_3072", keyPairSpec: "RSA_3072", wantErr: false},
		{name: "rsa_4096", keyPairSpec: "RSA_4096", wantErr: false},
		{name: "ecc_p256", keyPairSpec: "ECC_NIST_P256", wantErr: false},
		{name: "ecc_p384", keyPairSpec: "ECC_NIST_P384", wantErr: false},
		{name: "ecc_p521", keyPairSpec: "ECC_NIST_P521", wantErr: false},
		{name: "invalid_spec", keyPairSpec: "INVALID", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()

			// Create a symmetric wrapping key.
			wrapKey, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)

			out, err := b.GenerateDataKeyPair(context.Background(), &kms.GenerateDataKeyPairInput{
				KeyID:       wrapKey.KeyMetadata.KeyID,
				KeyPairSpec: tt.keyPairSpec,
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, out.PublicKey)
			assert.NotEmpty(t, out.PrivateKeyPlaintext)
			assert.NotEmpty(t, out.PrivateKeyCiphertextBlob)
			assert.Equal(t, tt.keyPairSpec, out.KeyPairSpec)
			assert.NotEmpty(t, out.KeyID)

			// Verify the encrypted private key can be decrypted back to the plaintext.
			decOut, err := b.Decrypt(context.Background(), &kms.DecryptInput{
				CiphertextBlob: out.PrivateKeyCiphertextBlob,
			})
			require.NoError(t, err)
			assert.Equal(t, out.PrivateKeyPlaintext, decOut.Plaintext)
		})
	}
}

// TestGenerateDataKeyPairWithoutPlaintext verifies key pair generation without plaintext private key.
func TestGenerateDataKeyPairWithoutPlaintext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		keyPairSpec string
		wantErr     bool
	}{
		{name: "rsa_2048", keyPairSpec: "RSA_2048", wantErr: false},
		{name: "ecc_p256", keyPairSpec: "ECC_NIST_P256", wantErr: false},
		{name: "invalid_spec", keyPairSpec: "INVALID", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()

			wrapKey, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)

			out, err := b.GenerateDataKeyPairWithoutPlaintext(
				context.Background(),
				&kms.GenerateDataKeyPairWithoutPlaintextInput{
					KeyID:       wrapKey.KeyMetadata.KeyID,
					KeyPairSpec: tt.keyPairSpec,
				},
			)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, out.PublicKey)
			assert.NotEmpty(t, out.PrivateKeyCiphertextBlob)
			assert.Equal(t, tt.keyPairSpec, out.KeyPairSpec)
		})
	}
}

// TestGenerateDataKeyPair_WrongKeyUsage verifies that a non-ENCRYPT_DECRYPT wrapping key returns an error.
func TestGenerateDataKeyPair_WrongKeyUsage(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()

	// Create a SIGN_VERIFY key – should not be usable as a wrapping key.
	signKey, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec: "RSA_2048",
	})
	require.NoError(t, err)

	_, err = b.GenerateDataKeyPair(context.Background(), &kms.GenerateDataKeyPairInput{
		KeyID:       signKey.KeyMetadata.KeyID,
		KeyPairSpec: "ECC_NIST_P256",
	})
	require.Error(t, err)
}

func TestGenerateDataKey_GrantTokens_Accepted(t *testing.T) {
	t.Parallel()
	b := ops2NewBackend(t)
	keyID := ops2MustCreateSymKey(t, b)

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"GenerateDataKey"},
	})
	require.NoError(t, err)

	out, err := b.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
		KeyID:       keyID,
		KeySpec:     "AES_256",
		GrantTokens: []string{grantOut.GrantToken},
	})
	require.NoError(t, err)
	assert.Len(t, out.Plaintext, 32, "AES_256 data key should be 32 bytes")
	assert.NotEmpty(t, out.CiphertextBlob)
}

func TestGenerateDataKey_ExpiredGrantToken_Rejected(t *testing.T) {
	t.Parallel()
	b := ops2NewBackend(t)
	keyID := ops2MustCreateSymKey(t, b)

	_, err := b.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
		KeyID:       keyID,
		KeySpec:     "AES_256",
		GrantTokens: []string{"not-a-real-grant-token"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrInvalidGrantToken)
}

func TestGenerateDataKeyWithoutPlaintext_GrantTokens_Accepted(t *testing.T) {
	t.Parallel()
	b := ops2NewBackend(t)
	keyID := ops2MustCreateSymKey(t, b)

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"GenerateDataKeyWithoutPlaintext"},
	})
	require.NoError(t, err)

	nb := int32(16)
	out, err := b.GenerateDataKeyWithoutPlaintext(context.Background(), &kms.GenerateDataKeyWithoutPlaintextInput{
		KeyID:         keyID,
		NumberOfBytes: &nb,
		GrantTokens:   []string{grantOut.GrantToken},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.CiphertextBlob)
}

func TestGenerateDataKeyWithoutPlaintext_ExpiredGrantToken_Rejected(t *testing.T) {
	t.Parallel()
	b := ops2NewBackend(t)
	keyID := ops2MustCreateSymKey(t, b)

	_, err := b.GenerateDataKeyWithoutPlaintext(context.Background(), &kms.GenerateDataKeyWithoutPlaintextInput{
		KeyID:       keyID,
		KeySpec:     "AES_256",
		GrantTokens: []string{"not-a-real-grant-token"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrInvalidGrantToken)
}

// Test_GenerateDataKeyPair_GrantTokenConstraints verifies that GenerateDataKeyPair
// (and its WithoutPlaintext sibling) enforce a grant's EncryptionContextEquals
// constraint against the caller-supplied EncryptionContext, mirroring the behavior
// already proven for Encrypt/Decrypt/GenerateDataKey.
func TestGenerateDataKeyPair_GrantTokenConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr       error
		callerContext map[string]string
		name          string
		withoutPT     bool
	}{
		{
			name:          "matching_context_succeeds",
			callerContext: map[string]string{"purpose": "test"},
		},
		{
			name:          "mismatched_context_rejected",
			callerContext: map[string]string{"purpose": "wrong"},
			wantErr:       kms.ErrKeyInvalidState,
		},
		{
			name:          "matching_context_succeeds_without_plaintext",
			callerContext: map[string]string{"purpose": "test"},
			withoutPT:     true,
		},
		{
			name:          "mismatched_context_rejected_without_plaintext",
			callerContext: map[string]string{"purpose": "wrong"},
			withoutPT:     true,
			wantErr:       kms.ErrKeyInvalidState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			b := kms.NewInMemoryBackend()

			wrapKeyOut, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
			require.NoError(t, err)

			grantOut, err := b.CreateGrant(ctx, &kms.CreateGrantInput{
				KeyID:            wrapKeyOut.KeyMetadata.KeyID,
				GranteePrincipal: "arn:aws:iam::000000000000:role/test",
				Operations:       []string{"GenerateDataKeyPair", "GenerateDataKeyPairWithoutPlaintext"},
				Constraints: &kms.GrantConstraints{
					EncryptionContextEquals: map[string]string{"purpose": "test"},
				},
			})
			require.NoError(t, err)

			var opErr error

			if tt.withoutPT {
				_, opErr = b.GenerateDataKeyPairWithoutPlaintext(ctx, &kms.GenerateDataKeyPairWithoutPlaintextInput{
					KeyID:             wrapKeyOut.KeyMetadata.KeyID,
					KeyPairSpec:       "ECC_NIST_P256",
					EncryptionContext: tt.callerContext,
					GrantTokens:       []string{grantOut.GrantToken},
				})
			} else {
				_, opErr = b.GenerateDataKeyPair(ctx, &kms.GenerateDataKeyPairInput{
					KeyID:             wrapKeyOut.KeyMetadata.KeyID,
					KeyPairSpec:       "ECC_NIST_P256",
					EncryptionContext: tt.callerContext,
					GrantTokens:       []string{grantOut.GrantToken},
				})
			}

			if tt.wantErr != nil {
				require.Error(t, opErr)
				assert.ErrorIs(t, opErr, tt.wantErr, "got %v, want %v", opErr, tt.wantErr)

				return
			}

			require.NoError(t, opErr)
		})
	}
}

func TestGenerateDataKeyWithoutPlaintextNumberOfBytes(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)

	numBytes := int32(64)
	out, err := b.GenerateDataKeyWithoutPlaintext(context.Background(), &kms.GenerateDataKeyWithoutPlaintextInput{
		KeyID:         key.KeyMetadata.KeyID,
		NumberOfBytes: &numBytes,
	})
	require.NoError(t, err)
	assert.NotNil(t, out.CiphertextBlob)
}
