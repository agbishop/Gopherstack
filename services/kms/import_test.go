package kms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"time"
)

// TestGetParametersForImport_ReturnsRealRSAPublicKey verifies that the returned
// PublicKey is a parseable DER-encoded SubjectPublicKeyInfo RSA public key.
func TestGetParametersForImport_ReturnsRealRSAPublicKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wrappingSpec string
		wantBits     int
	}{
		{
			name:         "RSA_2048",
			wrappingSpec: "RSA_2048",
			wantBits:     2048,
		},
		{
			name:         "RSA_3072",
			wrappingSpec: "RSA_3072",
			wantBits:     3072,
		},
		{
			name:         "RSA_4096",
			wrappingSpec: "RSA_4096",
			wantBits:     4096,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			ctx := context.Background()

			keyOut, err := b.CreateKey(ctx, &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
			require.NoError(t, err)
			keyID := keyOut.KeyMetadata.KeyID

			params, err := b.GetParametersForImport(ctx, &kms.GetParametersForImportInput{
				KeyID:             keyID,
				WrappingAlgorithm: "RSAES_OAEP_SHA_256",
				WrappingKeySpec:   tc.wrappingSpec,
			})
			require.NoError(t, err)
			require.NotEmpty(t, params.PublicKey, "PublicKey must be non-empty")

			// Parse as SubjectPublicKeyInfo.
			pub, err := x509.ParsePKIXPublicKey(params.PublicKey)
			require.NoError(t, err, "PublicKey must be valid DER SubjectPublicKeyInfo")

			rsaPub, ok := pub.(*rsa.PublicKey)
			require.True(t, ok, "PublicKey must be RSA")
			assert.Equal(t, tc.wantBits, rsaPub.N.BitLen(),
				"RSA key bit size must match wrapping spec")
		})
	}
}

// TestImportKeyMaterial_RSAOAEPWrapped verifies full end-to-end: wrap AES material
// with the returned RSA public key, import, confirm key becomes Enabled and usable.
func TestImportKeyMaterial_RSAOAEPWrapped(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	ctx := context.Background()

	keyOut, err := b.CreateKey(ctx, &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	params, err := b.GetParametersForImport(ctx, &kms.GetParametersForImportInput{
		KeyID:             keyID,
		WrappingAlgorithm: "RSAES_OAEP_SHA_256",
		WrappingKeySpec:   "RSA_2048",
	})
	require.NoError(t, err)

	// Parse RSA public key.
	pub, err := x509.ParsePKIXPublicKey(params.PublicKey)
	require.NoError(t, err)
	rsaPub := pub.(*rsa.PublicKey)

	// Generate 32-byte AES-256 key material and RSA-OAEP encrypt it.
	rawMaterial := make([]byte, 32)
	_, err = rand.Read(rawMaterial)
	require.NoError(t, err)

	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPub, rawMaterial, nil)
	require.NoError(t, err)

	// Import the RSA-OAEP-wrapped material.
	err = b.ImportKeyMaterial(ctx, &kms.ImportKeyMaterialInput{
		KeyID:           keyID,
		KeyMaterial:     wrapped,
		ExpirationModel: "KEY_MATERIAL_DOES_NOT_EXPIRE",
	})
	require.NoError(t, err)

	// Key must be Enabled.
	desc, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateEnabled, desc.KeyMetadata.KeyState)

	// Must be able to encrypt and decrypt with the imported key.
	plain := []byte("audit-kms-rsa-wrapped-import-test")
	encOut, err := b.Encrypt(ctx, &kms.EncryptInput{KeyID: keyID, Plaintext: plain})
	require.NoError(t, err)

	decOut, err := b.Decrypt(ctx, &kms.DecryptInput{KeyID: keyID, CiphertextBlob: encOut.CiphertextBlob})
	require.NoError(t, err)
	assert.Equal(t, plain, decOut.Plaintext)
}

// TestImportKeyMaterial_RawBytesBackwardCompat verifies that raw 32-byte key
// material (no RSA wrapping) is still accepted after the RSA-OAEP upgrade.
func TestImportKeyMaterial_RawBytesBackwardCompat(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	ctx := context.Background()

	keyOut, err := b.CreateKey(ctx, &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	// GetParametersForImport to satisfy any internal state (import token etc.).
	_, err = b.GetParametersForImport(ctx, &kms.GetParametersForImportInput{
		KeyID:             keyID,
		WrappingAlgorithm: "RSAES_OAEP_SHA_256",
		WrappingKeySpec:   "RSA_2048",
	})
	require.NoError(t, err)

	// Pass raw 32 bytes — backward compat path.
	rawMaterial := make([]byte, 32)
	err = b.ImportKeyMaterial(ctx, &kms.ImportKeyMaterialInput{
		KeyID:           keyID,
		KeyMaterial:     rawMaterial,
		ExpirationModel: "KEY_MATERIAL_DOES_NOT_EXPIRE",
	})
	require.NoError(t, err)

	desc, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateEnabled, desc.KeyMetadata.KeyState,
		"raw 32-byte import must still enable the key")
}

func TestImportKeyMaterial_EnablesExternalKey(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	// GetParametersForImport.
	params, err := b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID:             keyID,
		WrappingAlgorithm: "RSAES_OAEP_SHA_256",
		WrappingKeySpec:   "RSA_2048",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, params.PublicKey)
	assert.NotEmpty(t, params.ImportToken)

	// Import 32-byte key material (symmetric key must be exactly 32 bytes).
	keyMaterial := make([]byte, 32)
	err = b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:           keyID,
		KeyMaterial:     keyMaterial,
		ExpirationModel: "KEY_MATERIAL_DOES_NOT_EXPIRE",
	})
	require.NoError(t, err)

	// Key should now be enabled.
	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateEnabled, desc.KeyMetadata.KeyState)
}

func TestDeleteImportedKeyMaterial(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	_, err = b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID:             keyID,
		WrappingAlgorithm: "RSAES_OAEP_SHA_256",
		WrappingKeySpec:   "RSA_2048",
	})
	require.NoError(t, err)

	err = b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:           keyID,
		KeyMaterial:     make([]byte, 32),
		ExpirationModel: "KEY_MATERIAL_DOES_NOT_EXPIRE",
	})
	require.NoError(t, err)

	err = b.DeleteImportedKeyMaterial(context.Background(), &kms.DeleteImportedKeyMaterialInput{KeyID: keyID})
	require.NoError(t, err)

	// Key should return to PendingImport.
	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState)
}

func TestImportKeyMaterial_NonExternalOriginFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:           keyID,
		KeyMaterial:     []byte("fake"),
		ExpirationModel: "KEY_MATERIAL_DOES_NOT_EXPIRE",
	})
	require.Error(t, err)
}

func TestImportKeyMaterial_ExpiresModel_RequiresValidTo(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	mat := make([]byte, 32)
	err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:           keyID,
		KeyMaterial:     mat,
		ExpirationModel: "KEY_MATERIAL_EXPIRES",
		ValidTo:         0, // missing
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ValidationException")
}

func TestImportKeyMaterial_NoExpiryModel_RejectsValidTo(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	mat := make([]byte, 32)
	futureTS := float64(time.Now().Add(24*time.Hour).UnixNano()) / 1e9
	err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:           keyID,
		KeyMaterial:     mat,
		ExpirationModel: "KEY_MATERIAL_DOES_NOT_EXPIRE",
		ValidTo:         futureTS,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ValidationException")
}

func TestImportKeyMaterial_WithValidTo_ExpiresModel(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	mat := make([]byte, 32)
	futureTS := float64(time.Now().Add(24*time.Hour).UnixNano()) / 1e9
	require.NoError(t, b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: mat,
		ValidTo:     futureTS,
	}))

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, "KEY_MATERIAL_EXPIRES", desc.KeyMetadata.ExpirationModel)
	assert.Greater(t, desc.KeyMetadata.ValidTo, float64(0))
}

func TestImportKeyMaterial_NoValidTo_NoExpiryModel(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	b2importKeyMaterial(t, b, keyID)

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, "KEY_MATERIAL_DOES_NOT_EXPIRE", desc.KeyMetadata.ExpirationModel)
	assert.InDelta(t, float64(0), desc.KeyMetadata.ValidTo, 0)
}

func TestImportKeyMaterial_ReImport_ReplacesExisting(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	mat1 := make([]byte, 32)
	for i := range mat1 {
		mat1[i] = byte(i + 1)
	}
	require.NoError(t, b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: mat1,
	}))

	// Delete imported material
	require.NoError(t, b.DeleteImportedKeyMaterial(context.Background(), &kms.DeleteImportedKeyMaterialInput{
		KeyID: keyID,
	}))

	// Re-import with different material
	mat2 := make([]byte, 32)
	for i := range mat2 {
		mat2[i] = byte(i + 100)
	}
	require.NoError(t, b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: mat2,
	}))

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateEnabled, desc.KeyMetadata.KeyState)
}

func TestImportKeyMaterial_ExpiredMaterial_BlocksEncrypt(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	// Import with past ValidTo (already expired)
	pastTS := float64(time.Now().Add(-1*time.Hour).UnixNano()) / 1e9

	// Import must succeed (we allow setting past ValidTo for testing)
	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 1)
	}
	// The backend's expiry check is inline on Encrypt, not on Import.
	// So first import successfully with a past-time ValidTo via direct struct manipulation.
	// AWS validates ValidTo > now on import; our mock doesn't to stay testable.
	// We test the inline expiry check by first importing with future ts, then time-rolling.
	// Instead: import with explicit expires model + past ValidTo to verify Encrypt blocks.

	// Use backdated timestamp so it looks expired
	require.NoError(t, b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: mat,
		ValidTo:     pastTS,
	}))

	// Encrypt should fail because the material is past its ValidTo
	_, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     keyID,
		Plaintext: []byte("test"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestImportKeyMaterial_NonExternalOrigin_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	mat := make([]byte, 32)
	err = b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: mat,
	})
	require.Error(t, err)
}

func TestImportKeyMaterial_WrongSize_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	mat := make([]byte, 16) // too short, need 32
	err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: mat,
	})
	require.Error(t, err)
}

func TestDeleteImportedKeyMaterial_TransitionsToPendingImport(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	b2importKeyMaterial(t, b, keyID)

	require.NoError(t, b.DeleteImportedKeyMaterial(context.Background(), &kms.DeleteImportedKeyMaterialInput{
		KeyID: keyID,
	}))

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState)
	assert.False(t, desc.KeyMetadata.Enabled)
}

func TestGetParametersForImport_ReturnsTokenAndKey(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	out, err := b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID: keyID,
	})
	require.NoError(t, err)
	assert.Equal(t, keyID, out.KeyID)
	assert.NotEmpty(t, out.ImportToken)
	assert.NotEmpty(t, out.PublicKey)
	assert.Greater(t, out.ParametersValidTo, float64(0))
}

func TestGetParametersForImport_AWSOriginKey_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	_, err = b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID: keyID,
	})
	require.Error(t, err)
}

func TestGetParametersForImport_InvalidWrappingAlgorithm_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	_, err := b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID:             keyID,
		WrappingAlgorithm: "INVALID_ALGO",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UnsupportedOperationException")
}

func TestGetParametersForImport_InvalidWrappingKeySpec_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	_, err := b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID:           keyID,
		WrappingKeySpec: "ECC_NIST_P256",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UnsupportedOperationException")
}

func TestGetParametersForImport_ValidWrappingAlgorithms(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	algorithms := []string{
		"RSAES_PKCS1_V1_5",
		"RSAES_OAEP_SHA_1",
		"RSAES_OAEP_SHA_256",
		"RSA_AES_KEY_WRAP_SHA_1",
		"RSA_AES_KEY_WRAP_SHA_256",
	}

	for _, algo := range algorithms {
		t.Run(algo, func(t *testing.T) {
			t.Parallel()
			keyID := b2mustCreateExternalKey(t, b)
			out, err := b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
				KeyID:             keyID,
				WrappingAlgorithm: algo,
			})
			require.NoError(t, err)
			assert.NotEmpty(t, out.ImportToken)
		})
	}
}

func TestGetParametersForImport_ValidWrappingKeySpecs(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	specs := []string{"RSA_2048", "RSA_3072", "RSA_4096"}
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {
			t.Parallel()
			keyID := b2mustCreateExternalKey(t, b)
			out, err := b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
				KeyID:           keyID,
				WrappingKeySpec: spec,
			})
			require.NoError(t, err)
			assert.NotEmpty(t, out.PublicKey)
		})
	}
}

func TestGetParametersForImport_ParametersValidToIsFuture(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateExternalKey(t, b)

	now := float64(time.Now().UnixNano()) / 1e9
	out, err := b.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID: keyID,
	})
	require.NoError(t, err)
	assert.Greater(t, out.ParametersValidTo, now)
}

// TestKMSKeyRotation_AsymmetricUnsupported verifies that EnableKeyRotation and
// DisableKeyRotation return errors for asymmetric and EXTERNAL-origin keys.
func TestKMSKeyRotation_AsymmetricUnsupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keySpec string
		origin  string
		op      string
	}{
		{name: "enable_rsa_2048", keySpec: "RSA_2048", op: "enable"},
		{name: "disable_rsa_2048", keySpec: "RSA_2048", op: "disable"},
		{name: "enable_ecc_p256", keySpec: "ECC_NIST_P256", op: "enable"},
		{name: "disable_ecc_p256", keySpec: "ECC_NIST_P256", op: "disable"},
		{name: "enable_external_origin", origin: kms.KeyOriginExternal, op: "enable"},
		{name: "disable_external_origin_after_import", origin: kms.KeyOriginExternal, op: "disable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			inp := &kms.CreateKeyInput{KeySpec: tt.keySpec, Origin: tt.origin}

			out, err := b.CreateKey(context.Background(), inp)
			require.NoError(t, err)
			keyID := out.KeyMetadata.KeyID

			// For EXTERNAL keys in PendingImport, enable them first by importing material.
			if tt.origin == kms.KeyOriginExternal {
				mat := make([]byte, 32)
				require.NoError(t, b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
					KeyID:       keyID,
					KeyMaterial: mat,
				}))
			}

			switch tt.op {
			case "enable":
				rotErr := b.EnableKeyRotation(context.Background(), &kms.EnableKeyRotationInput{KeyID: keyID})
				require.ErrorIs(t, rotErr, kms.ErrUnsupportedOrigin)
			case "disable":
				rotErr := b.DisableKeyRotation(context.Background(), &kms.DisableKeyRotationInput{KeyID: keyID})
				require.ErrorIs(t, rotErr, kms.ErrUnsupportedOrigin)
			}
		})
	}
}

// TestKMSImportKeyMaterial verifies the ImportKeyMaterial and DeleteImportedKeyMaterial lifecycle.
func TestKMSImportKeyMaterial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(b *kms.InMemoryBackend) (string, []byte)
		verify  func(t *testing.T, b *kms.InMemoryBackend, keyID string, mat []byte)
		name    string
		wantErr bool
	}{
		{
			name: "import_enables_key",
			setup: func(b *kms.InMemoryBackend) (string, []byte) {
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				if err != nil {
					return "", nil
				}

				mat := make([]byte, 32)
				for i := range mat {
					mat[i] = byte(i)
				}

				return key.KeyMetadata.KeyID, mat
			},
			verify: func(t *testing.T, b *kms.InMemoryBackend, keyID string, mat []byte) {
				t.Helper()

				err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
					KeyID:       keyID,
					KeyMaterial: mat,
				})
				require.NoError(t, err)

				// Key should now be Enabled.
				desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
				require.NoError(t, err)
				assert.Equal(t, kms.KeyStateEnabled, desc.KeyMetadata.KeyState)
				assert.Equal(t, kms.KeyOriginExternal, desc.KeyMetadata.Origin)

				// Should be able to encrypt/decrypt.
				encOut, err := b.Encrypt(context.Background(), &kms.EncryptInput{
					KeyID:     keyID,
					Plaintext: []byte("external-key-data"),
				})
				require.NoError(t, err)

				decOut, err := b.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: encOut.CiphertextBlob})
				require.NoError(t, err)
				assert.Equal(t, []byte("external-key-data"), decOut.Plaintext)
			},
		},
		{
			name: "delete_imported_material_returns_to_pending_import",
			setup: func(b *kms.InMemoryBackend) (string, []byte) {
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				if err != nil {
					return "", nil
				}

				mat := make([]byte, 32)
				for i := range mat {
					mat[i] = byte(i + 1)
				}

				if err = b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
					KeyID:       key.KeyMetadata.KeyID,
					KeyMaterial: mat,
				}); err != nil {
					return "", nil
				}

				return key.KeyMetadata.KeyID, mat
			},
			verify: func(t *testing.T, b *kms.InMemoryBackend, keyID string, _ []byte) {
				t.Helper()

				err := b.DeleteImportedKeyMaterial(
					context.Background(),
					&kms.DeleteImportedKeyMaterialInput{KeyID: keyID},
				)
				require.NoError(t, err)

				desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
				require.NoError(t, err)
				assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState)

				// Encrypt should fail now (no key material).
				_, encErr := b.Encrypt(context.Background(), &kms.EncryptInput{
					KeyID:     keyID,
					Plaintext: []byte("should-fail"),
				})
				require.Error(t, encErr)
			},
		},
		{
			name: "import_on_aws_kms_origin_fails",
			setup: func(b *kms.InMemoryBackend) (string, []byte) {
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				if err != nil {
					return "", nil
				}

				return key.KeyMetadata.KeyID, make([]byte, 32)
			},
			verify: func(t *testing.T, b *kms.InMemoryBackend, keyID string, mat []byte) {
				t.Helper()

				err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
					KeyID:       keyID,
					KeyMaterial: mat,
				})
				require.Error(t, err)
				assert.ErrorIs(t, err, kms.ErrUnsupportedOrigin)
			},
		},
		{
			name: "delete_imported_on_aws_kms_origin_fails",
			setup: func(b *kms.InMemoryBackend) (string, []byte) {
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				if err != nil {
					return "", nil
				}

				return key.KeyMetadata.KeyID, nil
			},
			verify: func(t *testing.T, b *kms.InMemoryBackend, keyID string, _ []byte) {
				t.Helper()

				err := b.DeleteImportedKeyMaterial(
					context.Background(),
					&kms.DeleteImportedKeyMaterialInput{KeyID: keyID},
				)
				require.Error(t, err)
				assert.ErrorIs(t, err, kms.ErrUnsupportedOrigin)
			},
		},
		{
			name: "import_wrong_size_material_fails",
			setup: func(b *kms.InMemoryBackend) (string, []byte) {
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				if err != nil {
					return "", nil
				}

				return key.KeyMetadata.KeyID, make([]byte, 16) // too short
			},
			verify: func(t *testing.T, b *kms.InMemoryBackend, keyID string, mat []byte) {
				t.Helper()

				err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
					KeyID:       keyID,
					KeyMaterial: mat,
				})
				require.Error(t, err)
			},
		},
		{
			name: "external_key_starts_as_pending_import",
			setup: func(b *kms.InMemoryBackend) (string, []byte) {
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				if err != nil {
					return "", nil
				}

				return key.KeyMetadata.KeyID, nil
			},
			verify: func(t *testing.T, b *kms.InMemoryBackend, keyID string, _ []byte) {
				t.Helper()

				desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
				require.NoError(t, err)
				assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState)
				assert.Equal(t, kms.KeyOriginExternal, desc.KeyMetadata.Origin)
			},
		},
		{
			name: "import_on_already_enabled_external_key_fails",
			setup: func(b *kms.InMemoryBackend) (string, []byte) {
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				if err != nil {
					return "", nil
				}

				mat := make([]byte, 32)
				for i := range mat {
					mat[i] = byte(i + 2)
				}

				if err = b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
					KeyID:       key.KeyMetadata.KeyID,
					KeyMaterial: mat,
				}); err != nil {
					return "", nil
				}

				return key.KeyMetadata.KeyID, mat
			},
			verify: func(t *testing.T, b *kms.InMemoryBackend, keyID string, mat []byte) {
				t.Helper()

				// Attempting to re-import while the key is already Enabled must fail.
				err := b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
					KeyID:       keyID,
					KeyMaterial: mat,
				})
				require.Error(t, err)
				assert.ErrorIs(t, err, kms.ErrKeyInvalidState)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			keyID, mat := tt.setup(b)
			require.NotEmpty(t, keyID, "setup should return a key ID")
			tt.verify(t, b, keyID, mat)
		})
	}
}

// TestKMSImportKeyMaterialSnapshotRestore verifies that EXTERNAL-origin key state
// is correctly preserved through snapshot/restore.
func TestKMSImportKeyMaterialSnapshotRestore(t *testing.T) {
	t.Parallel()

	orig := kms.NewInMemoryBackend()
	key, err := orig.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingImport, key.KeyMetadata.KeyState)

	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 5)
	}

	require.NoError(t, orig.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       key.KeyMetadata.KeyID,
		KeyMaterial: mat,
	}))

	snap := orig.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	fresh := kms.NewInMemoryBackendWithConfig(kms.MockAccountID, kms.MockRegion)
	require.NoError(t, fresh.Restore(t.Context(), snap))

	desc, err := fresh.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: key.KeyMetadata.KeyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateEnabled, desc.KeyMetadata.KeyState)
	assert.Equal(t, kms.KeyOriginExternal, desc.KeyMetadata.Origin)

	// Encrypt on original, decrypt on restored.
	encOut, err := orig.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     key.KeyMetadata.KeyID,
		Plaintext: []byte("external"),
	})
	require.NoError(t, err)

	decOut, err := fresh.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: encOut.CiphertextBlob})
	require.NoError(t, err)
	assert.Equal(t, []byte("external"), decOut.Plaintext)
}

// TestKMSExternalKeyEncryptionContext verifies encryption context is correctly applied
// when using an EXTERNAL-origin key.
func TestKMSExternalKeyEncryptionContext(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)

	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 10)
	}

	require.NoError(t, b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       key.KeyMetadata.KeyID,
		KeyMaterial: mat,
	}))

	ctx := map[string]string{"service": "myservice"}

	encOut, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:             key.KeyMetadata.KeyID,
		Plaintext:         []byte("external-ctx-test"),
		EncryptionContext: ctx,
	})
	require.NoError(t, err)

	// Decrypt with correct context.
	decOut, err := b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob:    encOut.CiphertextBlob,
		EncryptionContext: ctx,
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("external-ctx-test"), decOut.Plaintext)

	// Decrypt without context must fail.
	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: encOut.CiphertextBlob})
	require.Error(t, err)
}

func TestRotateKeyOnDemandExternalOriginRejected(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)

	// Import material so key transitions to Enabled state.
	err = b.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       key.KeyMetadata.KeyID,
		KeyMaterial: make([]byte, 32),
	})
	require.NoError(t, err)

	_, rotErr := b.RotateKeyOnDemand(context.Background(), &kms.RotateKeyOnDemandInput{KeyID: key.KeyMetadata.KeyID})
	require.Error(t, rotErr)
	require.ErrorIs(t, rotErr, kms.ErrUnsupportedOrigin)
}
