package kms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"strings"
	"time"
)

func TestCreateKey_AllSpecs(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	specs := []struct {
		spec  string
		usage string
	}{
		{"SYMMETRIC_DEFAULT", kms.KeyUsageEncryptDecrypt},
		{"RSA_2048", kms.KeyUsageSignVerify},
		{"RSA_3072", kms.KeyUsageSignVerify},
		{"RSA_4096", kms.KeyUsageSignVerify},
		{"ECC_NIST_P256", kms.KeyUsageSignVerify},
		{"ECC_NIST_P384", kms.KeyUsageSignVerify},
		{"ECC_NIST_P521", kms.KeyUsageSignVerify},
		{"HMAC_256", kms.KeyUsageGenerateMac},
		{"HMAC_384", kms.KeyUsageGenerateMac},
		{"HMAC_512", kms.KeyUsageGenerateMac},
	}

	for _, tc := range specs {
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeySpec: tc.spec, KeyUsage: tc.usage})
			require.NoError(t, err)
			assert.NotEmpty(t, out.KeyMetadata.KeyID)
			assert.Equal(t, tc.spec, out.KeyMetadata.KeySpec)
			assert.Equal(t, kms.KeyStateEnabled, out.KeyMetadata.KeyState)
		})
	}
}

func TestDescribeKey_NotFound(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	_, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: "nonexistent-key-id"})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrKeyNotFound)
}

func TestListKeys_Pagination(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	for range 5 {
		mustCreateSymKey(t, b)
	}

	out, err := b.ListKeys(context.Background(), &kms.ListKeysInput{Limit: ptr[int32](3)})
	require.NoError(t, err)
	assert.Len(t, out.Keys, 3)
	assert.True(t, out.Truncated)
	assert.NotEmpty(t, out.NextMarker)

	out2, err := b.ListKeys(context.Background(), &kms.ListKeysInput{Limit: ptr[int32](3), Marker: out.NextMarker})
	require.NoError(t, err)
	assert.Len(t, out2.Keys, 2)
	assert.False(t, out2.Truncated)
}

func TestScheduleAndCancelDeletion(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	out, err := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)
	assert.NotZero(t, out.DeletionDate)

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingDeletion, desc.KeyMetadata.KeyState)

	_, err = b.CancelKeyDeletion(context.Background(), &kms.CancelKeyDeletionInput{KeyID: keyID})
	require.NoError(t, err)

	desc, err = b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateDisabled, desc.KeyMetadata.KeyState)
}

func TestScheduleDeletion_InvalidWindowRejected(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	_, err := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 3, // below minimum of 7
	})
	require.Error(t, err)
}

func TestUpdateKeyDescription(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	require.NoError(t, b.UpdateKeyDescription(context.Background(), &kms.UpdateKeyDescriptionInput{
		KeyID:       keyID,
		Description: "audit test key",
	}))

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, "audit test key", desc.KeyMetadata.Description)
}

func TestConcurrent_CreateKey(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	const n = 20
	errs := make(chan error, n)

	for range n {
		go func() {
			_, e := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			errs <- e
		}()
	}

	for range n {
		assert.NoError(t, <-errs)
	}
}

func TestErrors_KeyNotFound_Sentinel(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	_, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: "bad"})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrKeyNotFound)
}

func TestScheduleKeyDeletion_MinWindow_7Days(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 6,
	})
	require.Error(t, err)
}

func TestScheduleKeyDeletion_MaxWindow_30Days(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 31,
	})
	require.Error(t, err)
}

func TestScheduleKeyDeletion_DeletionDateInFuture(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	now := float64(time.Now().UnixNano()) / 1e9
	schedOut, err := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)
	assert.Greater(t, schedOut.DeletionDate, now)
	assert.Equal(t, kms.KeyStatePendingDeletion, schedOut.KeyState)
}

func TestCancelKeyDeletion_RestoresToDisabled(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)

	cancelOut, err := b.CancelKeyDeletion(context.Background(), &kms.CancelKeyDeletionInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateDisabled, cancelOut.KeyState)
}

func TestScheduleKeyDeletion_DefaultWindow_30Days(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	schedOut, err := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID: keyID,
		// No PendingWindowInDays → defaults to 30
	})
	require.NoError(t, err)
	assert.Equal(t, 30, schedOut.PendingWindowInDays)
}

func TestUpdateKeyDescription_EmptyDescriptionAllowed(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Description: "original"})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	require.NoError(t, b.UpdateKeyDescription(context.Background(), &kms.UpdateKeyDescriptionInput{
		KeyID:       keyID,
		Description: "",
	}))

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Empty(t, desc.KeyMetadata.Description)
}

func TestUpdateKeyDescription_MaxLength_Accepted(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	maxDesc := make([]byte, 8192)
	for i := range maxDesc {
		maxDesc[i] = 'a'
	}
	require.NoError(t, b.UpdateKeyDescription(context.Background(), &kms.UpdateKeyDescriptionInput{
		KeyID:       keyID,
		Description: string(maxDesc),
	}))
}

func TestUpdateKeyDescription_ExceedsMaxLength_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	tooLong := make([]byte, 8193)
	err = b.UpdateKeyDescription(context.Background(), &kms.UpdateKeyDescriptionInput{
		KeyID:       keyID,
		Description: string(tooLong),
	})
	require.Error(t, err)
}

func TestUpdateKeyDescription_DescribeKeyReturnsUpdated(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	require.NoError(t, b.UpdateKeyDescription(context.Background(), &kms.UpdateKeyDescriptionInput{
		KeyID:       keyID,
		Description: "updated description",
	}))

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, "updated description", desc.KeyMetadata.Description)
}

func TestCreateKey_ECC_P384_KeyAgreement(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "ECC_NIST_P384",
		KeyUsage: kms.KeyUsageKeyAgreement,
	})
	require.NoError(t, err)
	assert.Equal(t, "ECC_NIST_P384", out.KeyMetadata.KeySpec)
	assert.Equal(t, kms.KeyUsageKeyAgreement, out.KeyMetadata.KeyUsage)
}

func TestCreateKey_ECC_P521_KeyAgreement(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "ECC_NIST_P521",
		KeyUsage: kms.KeyUsageKeyAgreement,
	})
	require.NoError(t, err)
	assert.Equal(t, "ECC_NIST_P521", out.KeyMetadata.KeySpec)
}

func TestListKeys_Pagination_MultiplePages(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	for range 5 {
		_, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
		require.NoError(t, err)
	}

	limit := int32(2)
	first, err := b.ListKeys(context.Background(), &kms.ListKeysInput{Limit: &limit})
	require.NoError(t, err)
	assert.Len(t, first.Keys, 2)
	assert.True(t, first.Truncated)

	second, err := b.ListKeys(context.Background(), &kms.ListKeysInput{Limit: &limit, Marker: first.NextMarker})
	require.NoError(t, err)
	assert.NotEmpty(t, second.Keys)
}

func TestListKeyPolicies_ReturnsDefault(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	policies, err := b.ListKeyPolicies(context.Background(), &kms.ListKeyPoliciesInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Contains(t, policies.PolicyNames, "default")
}

func TestKeyMetadata_Enabled(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	assert.True(t, out.KeyMetadata.Enabled, "newly created key should be Enabled=true in metadata")

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: out.KeyMetadata.KeyID})
	require.NoError(t, err)
	assert.True(t, desc.KeyMetadata.Enabled)

	require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: out.KeyMetadata.KeyID}))

	desc2, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: out.KeyMetadata.KeyID})
	require.NoError(t, err)
	assert.False(t, desc2.KeyMetadata.Enabled)
}

func TestKeyMetadata_CustomerMasterKeySpec(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	assert.Equal(t, out.KeyMetadata.KeySpec, out.KeyMetadata.CustomerMasterKeySpec)
}

func TestScheduleKeyDeletion_UsesErrValidation(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)

	_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               key.KeyMetadata.KeyID,
		PendingWindowInDays: 3,
	})
	require.ErrorIs(t, err, kms.ErrValidation)
}

func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	for range 3 {
		_, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
		require.NoError(t, err)
		b.Reset()
		assert.Equal(t, 0, kms.KeyCount(b))
	}
}

// TestKMSBackendDescribeKey verifies key lookup.
func TestKMSBackendDescribeKey(t *testing.T) {
	t.Parallel()

	t.Run("Found", func(t *testing.T) {
		t.Parallel()

		backend := kms.NewInMemoryBackend()
		created, _ := backend.CreateKey(context.Background(), &kms.CreateKeyInput{Description: "my key"})

		out, err := backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: created.KeyMetadata.KeyID})
		require.NoError(t, err)
		assert.Equal(t, created.KeyMetadata.KeyID, out.KeyMetadata.KeyID)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		backend := kms.NewInMemoryBackend()
		_, err := backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: "does-not-exist"})
		require.ErrorIs(t, err, kms.ErrKeyNotFound)
	})
}

// TestKMSBackendListKeys verifies listing keys.
func TestKMSBackendListKeys(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()

	for range 3 {
		_, _ = backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	}

	out, err := backend.ListKeys(context.Background(), &kms.ListKeysInput{})
	require.NoError(t, err)
	assert.Len(t, out.Keys, 3)
	assert.False(t, out.Truncated)
}

// TestKMSBackendListKeysPagination verifies pagination in ListKeys.
func TestKMSBackendListKeysPagination(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()

	for range 5 {
		_, _ = backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	}

	limit := int32(2)
	out, err := backend.ListKeys(context.Background(), &kms.ListKeysInput{Limit: &limit})
	require.NoError(t, err)
	assert.Len(t, out.Keys, 2)
	assert.True(t, out.Truncated)
	assert.NotEmpty(t, out.NextMarker)

	// Get next page
	out2, err := backend.ListKeys(context.Background(), &kms.ListKeysInput{Limit: &limit, Marker: out.NextMarker})
	require.NoError(t, err)
	assert.Len(t, out2.Keys, 2)
}

// TestKMSScheduleAndCancelKeyDeletion verifies schedule and cancel key deletion.
func TestKMSScheduleAndCancelKeyDeletion(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()
	out, err := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	schedOut, err := backend.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingDeletion, schedOut.KeyState)
	assert.NotZero(t, schedOut.DeletionDate)

	// Cancel deletion — key should become Disabled
	cancelOut, cancelErr := backend.CancelKeyDeletion(context.Background(), &kms.CancelKeyDeletionInput{KeyID: keyID})
	require.NoError(t, cancelErr)
	assert.Equal(t, kms.KeyStateDisabled, cancelOut.KeyState)

	desc, err := backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateDisabled, desc.KeyMetadata.KeyState)
}

// TestKMSBackendUnsupportedKeySpec verifies that CreateKey fails with an unknown key spec.
func TestKMSBackendUnsupportedKeySpec(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	_, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec: "UNSUPPORTED_SPEC",
	})
	require.Error(t, err)
}

// TestKMSScheduleKeyDeletion_Validation verifies that ScheduleKeyDeletion enforces the
// pending window range [7, 30] days and rejects keys already in PendingDeletion.
func TestKMSScheduleKeyDeletion_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		setup       func(b *kms.InMemoryBackend, keyID string)
		name        string
		pendingDays int
	}{
		{
			name:        "valid_7_days",
			pendingDays: 7,
		},
		{
			name:        "valid_30_days",
			pendingDays: 30,
		},
		{
			name:        "too_few_days_6",
			pendingDays: 6,
			wantErr:     kms.ErrValidation,
		},
		{
			name:        "too_many_days_31",
			pendingDays: 31,
			wantErr:     kms.ErrValidation,
		},
		{
			name:        "default_when_zero",
			pendingDays: 0,
		},
		{
			name:        "already_pending_deletion",
			pendingDays: 7,
			setup: func(b *kms.InMemoryBackend, keyID string) {
				_, err := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
					KeyID:               keyID,
					PendingWindowInDays: 7,
				})
				require.NoError(t, err)
			},
			wantErr: kms.ErrKeyInvalidState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)
			keyID := out.KeyMetadata.KeyID

			if tt.setup != nil {
				tt.setup(b, keyID)
			}

			_, schedErr := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
				KeyID:               keyID,
				PendingWindowInDays: tt.pendingDays,
			})

			if tt.wantErr != nil {
				require.ErrorIs(t, schedErr, tt.wantErr)

				return
			}

			require.NoError(t, schedErr)
		})
	}
}

// TestKMSCancelKeyDeletion_RequiresPendingDeletion verifies that CancelKeyDeletion only
// succeeds for keys in PendingDeletion state.
func TestKMSCancelKeyDeletion_RequiresPendingDeletion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(b *kms.InMemoryBackend, keyID string)
		name    string
	}{
		{
			name: "key_in_pending_deletion_succeeds",
			setup: func(b *kms.InMemoryBackend, keyID string) {
				_, err := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
					KeyID:               keyID,
					PendingWindowInDays: 7,
				})
				require.NoError(t, err)
			},
		},
		{
			name:    "enabled_key_fails",
			wantErr: kms.ErrKeyInvalidState,
		},
		{
			name: "disabled_key_fails",
			setup: func(b *kms.InMemoryBackend, keyID string) {
				require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyID}))
			},
			// CancelKeyDeletion's own deserializeOpError (kms@v1.55.4 deserializers.go)
			// does not recognize DisabledException, only KMSInvalidStateException
			// (gopherstack-8u3f) -- "not pending deletion" is the same defect regardless
			// of which other state the key happens to be in.
			wantErr: kms.ErrKeyInvalidState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)
			keyID := out.KeyMetadata.KeyID

			if tt.setup != nil {
				tt.setup(b, keyID)
			}

			_, cancelErr := b.CancelKeyDeletion(context.Background(), &kms.CancelKeyDeletionInput{KeyID: keyID})

			if tt.wantErr != nil {
				require.ErrorIs(t, cancelErr, tt.wantErr)

				return
			}

			require.NoError(t, cancelErr)
		})
	}
}

// TestKMSEnableDisableKey_StateValidation verifies that EnableKey and DisableKey
// reject keys in PendingDeletion and PendingImport states.
func TestKMSEnableDisableKey_StateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(b *kms.InMemoryBackend) string
		name    string
		op      string
	}{
		{
			name: "disable_pending_deletion_fails",
			op:   "disable",
			setup: func(b *kms.InMemoryBackend) string {
				out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)
				keyID := out.KeyMetadata.KeyID
				_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
					KeyID:               keyID,
					PendingWindowInDays: 7,
				})
				require.NoError(t, err)

				return keyID
			},
			wantErr: kms.ErrKeyInvalidState,
		},
		{
			name: "enable_pending_deletion_fails",
			op:   "enable",
			setup: func(b *kms.InMemoryBackend) string {
				out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)
				keyID := out.KeyMetadata.KeyID
				_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
					KeyID:               keyID,
					PendingWindowInDays: 7,
				})
				require.NoError(t, err)

				return keyID
			},
			wantErr: kms.ErrKeyInvalidState,
		},
		{
			name: "disable_pending_import_fails",
			op:   "disable",
			setup: func(b *kms.InMemoryBackend) string {
				out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				require.NoError(t, err)

				return out.KeyMetadata.KeyID
			},
			wantErr: kms.ErrKeyInvalidState,
		},
		{
			name: "enable_pending_import_fails",
			op:   "enable",
			setup: func(b *kms.InMemoryBackend) string {
				out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				require.NoError(t, err)

				return out.KeyMetadata.KeyID
			},
			wantErr: kms.ErrKeyInvalidState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			targetKeyID := tt.setup(b)

			var opErr error

			switch tt.op {
			case "disable":
				opErr = b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: targetKeyID})
			case "enable":
				opErr = b.EnableKey(context.Background(), &kms.EnableKeyInput{KeyID: targetKeyID})
			}

			if tt.wantErr != nil {
				require.ErrorIs(t, opErr, tt.wantErr)

				return
			}

			require.NoError(t, opErr)
		})
	}
}

// TestKMSDescribeKey_DeletionDateInResponse verifies that DescribeKey returns the
// DeletionDate field for keys in PendingDeletion state.
func TestKMSDescribeKey_DeletionDateInResponse(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	// No DeletionDate before scheduling.
	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Zero(t, desc.KeyMetadata.DeletionDate)

	_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)

	desc, err = b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.NotZero(t, desc.KeyMetadata.DeletionDate)
	assert.Equal(t, kms.KeyStatePendingDeletion, desc.KeyMetadata.KeyState)
}

// TestCreateKey_KeyAgreementSpec verifies that ECC KEY_AGREEMENT keys are accepted by CreateKey.
func TestCreateKey_KeyAgreementSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keySpec  string
		keyUsage string
	}{
		{name: "ecc_p256_key_agreement", keySpec: "ECC_NIST_P256", keyUsage: kms.KeyUsageKeyAgreement},
		{name: "ecc_p384_key_agreement", keySpec: "ECC_NIST_P384", keyUsage: kms.KeyUsageKeyAgreement},
		{name: "ecc_p521_key_agreement", keySpec: "ECC_NIST_P521", keyUsage: kms.KeyUsageKeyAgreement},
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
			assert.Equal(t, kms.KeyUsageKeyAgreement, out.KeyMetadata.KeyUsage)
			assert.Equal(t, tt.keySpec, out.KeyMetadata.KeySpec)
		})
	}
}

// TestListKeys_LimitBound verifies that ListKeys rejects an out-of-range Limit
// (AWS bound: 1–1000) with ValidationException, and accepts in-range values.
func TestListKeys_LimitBound(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	i32 := func(v int32) *int32 { return &v }

	tests := []struct {
		limit   *int32
		name    string
		wantErr bool
	}{
		{name: "nil ok", limit: nil, wantErr: false},
		{name: "min ok", limit: i32(1), wantErr: false},
		{name: "max ok", limit: i32(1000), wantErr: false},
		{name: "zero rejected", limit: i32(0), wantErr: true},
		{name: "over cap rejected", limit: i32(1001), wantErr: true},
		{name: "negative rejected", limit: i32(-5), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := b.ListKeys(context.Background(), &kms.ListKeysInput{Limit: tc.limit})
			if tc.wantErr {
				require.ErrorIs(t, err, kms.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateKeyDescriptionMaxLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		descLen int
		wantErr bool
	}{
		{name: "at_limit", descLen: 8192, wantErr: false},
		{name: "under_limit", descLen: 100, wantErr: false},
		{name: "over_limit", descLen: 8193, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)

			desc := strings.Repeat("x", tt.descLen)
			updateErr := b.UpdateKeyDescription(context.Background(), &kms.UpdateKeyDescriptionInput{
				KeyID:       key.KeyMetadata.KeyID,
				Description: desc,
			})

			if tt.wantErr {
				require.Error(t, updateErr)
				require.ErrorIs(t, updateErr, kms.ErrValidation)
			} else {
				require.NoError(t, updateErr)
			}
		})
	}
}

func TestCancelKeyDeletionOutput(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)

	keyID := key.KeyMetadata.KeyID

	_, err = b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)

	out, err := b.CancelKeyDeletion(context.Background(), &kms.CancelKeyDeletionInput{KeyID: keyID})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, keyID, out.KeyID)
	assert.Equal(t, kms.KeyStateDisabled, out.KeyState)
}
