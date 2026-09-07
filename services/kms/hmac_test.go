package kms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

func TestGenerateMac_WrongAlgorithm_HMAC256KeyWithSHA512(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateHMACKey(t, b, "HMAC_256")

	_, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
		KeyID:        keyID,
		Message:      []byte("test"),
		MacAlgorithm: "HMAC_SHA_512", // wrong for HMAC_256
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidAlgorithmException")
}

func TestGenerateMac_WrongAlgorithm_HMAC512KeyWithSHA256(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateHMACKey(t, b, "HMAC_512")

	_, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
		KeyID:        keyID,
		Message:      []byte("test"),
		MacAlgorithm: "HMAC_SHA_256", // wrong for HMAC_512
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidAlgorithmException")
}

func TestVerifyMac_WrongAlgorithm(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateHMACKey(t, b, "HMAC_384")

	_, err := b.VerifyMac(context.Background(), &kms.VerifyMacInput{
		KeyID:        keyID,
		Message:      []byte("test"),
		Mac:          []byte("fakemac"),
		MacAlgorithm: "HMAC_SHA_256", // wrong for HMAC_384
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidAlgorithmException")
}

func TestGenerateMac_CorrectAlgorithm_AllSpecs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spec string
		algo string
	}{
		{"HMAC_256", "HMAC_SHA_256"},
		{"HMAC_384", "HMAC_SHA_384"},
		{"HMAC_512", "HMAC_SHA_512"},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			b := newBackend(t)
			keyID := mustCreateHMACKey(t, b, tc.spec)
			msg := []byte("mac test message")

			gOut, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
				KeyID:        keyID,
				Message:      msg,
				MacAlgorithm: tc.algo,
			})
			require.NoError(t, err)
			require.NotEmpty(t, gOut.Mac)

			vOut, err := b.VerifyMac(context.Background(), &kms.VerifyMacInput{
				KeyID:        keyID,
				Message:      msg,
				Mac:          gOut.Mac,
				MacAlgorithm: tc.algo,
			})
			require.NoError(t, err)
			assert.True(t, vOut.MacValid)
		})
	}
}

func TestVerifyMac_WrongMac_Fails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateHMACKey(t, b, "HMAC_256")

	_, err := b.VerifyMac(context.Background(), &kms.VerifyMacInput{
		KeyID:        keyID,
		Message:      []byte("msg"),
		Mac:          []byte("wrong"),
		MacAlgorithm: "HMAC_SHA_256",
	})
	require.Error(t, err)
}

func TestRotateKeyOnDemand_HMACKey_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "HMAC_256",
		KeyUsage: kms.KeyUsageGenerateMac,
	})
	require.NoError(t, err)

	_, err = b.RotateKeyOnDemand(context.Background(), &kms.RotateKeyOnDemandInput{KeyID: out.KeyMetadata.KeyID})
	require.Error(t, err)
}

func TestVerifyMac_Success(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeySpec: "HMAC_256"})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	msg := []byte("test message")
	macOut, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
		KeyID:        keyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      msg,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, macOut.Mac)

	verOut, err := b.VerifyMac(context.Background(), &kms.VerifyMacInput{
		KeyID:        keyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      msg,
		Mac:          macOut.Mac,
	})
	require.NoError(t, err)
	assert.True(t, verOut.MacValid)
	assert.Equal(t, "HMAC_SHA_256", verOut.MacAlgorithm)
}

func TestVerifyMac_WrongMac(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeySpec: "HMAC_256"})
	require.NoError(t, err)

	_, err = b.VerifyMac(context.Background(), &kms.VerifyMacInput{
		KeyID:        out.KeyMetadata.KeyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("message"),
		Mac:          []byte("wrong-mac"),
	})
	// VerifyMac's own deserializeOpError (kms@v1.55.4 deserializers.go) recognizes
	// KMSInvalidMacException, not KMSInvalidSignatureException -- that code belongs to
	// Verify only (gopherstack-8u3f).
	require.ErrorIs(t, err, kms.ErrInvalidMac)
}

func TestVerifyMac_EmptyAlgorithm(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.VerifyMac(context.Background(), &kms.VerifyMacInput{
		KeyID:        "any",
		MacAlgorithm: "",
		Message:      []byte("x"),
		Mac:          []byte("y"),
	})
	require.ErrorIs(t, err, kms.ErrValidation)
}

// TestGenerateMac verifies HMAC computation with HMAC KMS keys.
func TestGenerateMac(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		keySpec      string
		macAlgorithm string
		wantErr      bool
	}{
		{name: "hmac_256", keySpec: "HMAC_256", macAlgorithm: "HMAC_SHA_256", wantErr: false},
		{name: "hmac_384", keySpec: "HMAC_384", macAlgorithm: "HMAC_SHA_384", wantErr: false},
		{name: "hmac_512", keySpec: "HMAC_512", macAlgorithm: "HMAC_SHA_512", wantErr: false},
		{name: "wrong_usage_symmetric", keySpec: "SYMMETRIC_DEFAULT", macAlgorithm: "HMAC_SHA_256", wantErr: true},
		{name: "unsupported_algorithm", keySpec: "HMAC_256", macAlgorithm: "UNKNOWN_ALG", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()

			key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeySpec: tt.keySpec})
			require.NoError(t, err)

			out, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
				KeyID:        key.KeyMetadata.KeyID,
				MacAlgorithm: tt.macAlgorithm,
				Message:      []byte("hello world"),
			})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, out.Mac)
			assert.Equal(t, tt.macAlgorithm, out.MacAlgorithm)
			assert.NotEmpty(t, out.KeyID)

			// Verify determinism: same message with same key produces same MAC.
			out2, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
				KeyID:        key.KeyMetadata.KeyID,
				MacAlgorithm: tt.macAlgorithm,
				Message:      []byte("hello world"),
			})
			require.NoError(t, err)
			assert.Equal(t, out.Mac, out2.Mac)

			// Different message produces different MAC.
			out3, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
				KeyID:        key.KeyMetadata.KeyID,
				MacAlgorithm: tt.macAlgorithm,
				Message:      []byte("different message"),
			})
			require.NoError(t, err)
			assert.NotEqual(t, out.Mac, out3.Mac)
		})
	}
}

// TestCreateKey_HMACSpec verifies that HMAC key specs are accepted by CreateKey.
func TestCreateKey_HMACSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keySpec  string
		keyUsage string
	}{
		{name: "hmac_256_default_usage", keySpec: "HMAC_256"},
		{name: "hmac_384_default_usage", keySpec: "HMAC_384"},
		{name: "hmac_512_default_usage", keySpec: "HMAC_512"},
		{name: "hmac_256_explicit_usage", keySpec: "HMAC_256", keyUsage: kms.KeyUsageGenerateMac},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
				KeySpec:  tt.keySpec,
				KeyUsage: tt.keyUsage,
			})
			require.NoError(t, err)
			assert.Equal(t, kms.KeyUsageGenerateMac, out.KeyMetadata.KeyUsage)
			assert.Equal(t, tt.keySpec, out.KeyMetadata.KeySpec)
		})
	}
}

// TestGenerateMac_DisabledKey verifies that GenerateMac fails for a disabled key.
func TestGenerateMac_DisabledKey(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()

	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeySpec: "HMAC_256"})
	require.NoError(t, err)

	require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: key.KeyMetadata.KeyID}))

	_, err = b.GenerateMac(context.Background(), &kms.GenerateMacInput{
		KeyID:        key.KeyMetadata.KeyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("test"),
	})
	require.Error(t, err)
}

func TestVerifyMacRoundTrip(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "HMAC_256",
		KeyUsage: "GENERATE_VERIFY_MAC",
	})
	require.NoError(t, err)

	macOut, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
		KeyID:        key.KeyMetadata.KeyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("msg"),
	})
	require.NoError(t, err)

	verOut, err := b.VerifyMac(context.Background(), &kms.VerifyMacInput{
		KeyID:        key.KeyMetadata.KeyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("msg"),
		Mac:          macOut.Mac,
	})
	require.NoError(t, err)
	assert.True(t, verOut.MacValid)

	// Wrong message should fail.
	_, verErr := b.VerifyMac(context.Background(), &kms.VerifyMacInput{
		KeyID:        key.KeyMetadata.KeyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("wrong"),
		Mac:          macOut.Mac,
	})
	require.Error(t, verErr)
}
