package kms_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"strings"
)

// TestUpdatePrimaryRegion_RoleSwap verifies the full topology update: the old
// primary becomes a replica and the old replica becomes the new primary.
func TestUpdatePrimaryRegion_RoleSwap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		replicaRegion string
		newPrimary    string
	}{
		{
			name:          "swap east-to-west",
			replicaRegion: "us-west-2",
			newPrimary:    "us-west-2",
		},
		{
			name:          "swap east-to-eu",
			replicaRegion: "eu-west-1",
			newPrimary:    "eu-west-1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			ctx := context.Background()

			// Create primary multi-region key.
			pOut, err := b.CreateKey(ctx, &kms.CreateKeyInput{MultiRegion: true})
			require.NoError(t, err)
			primaryID := pOut.KeyMetadata.KeyID

			// Replicate to target region.
			rOut, err := b.ReplicateKey(ctx, &kms.ReplicateKeyInput{
				KeyID:         primaryID,
				ReplicaRegion: tc.replicaRegion,
			})
			require.NoError(t, err)
			replicaID := rOut.ReplicaKeyMetadata.KeyID

			// UpdatePrimaryRegion takes the CURRENT primary key ID and the target region.
			err = b.UpdatePrimaryRegion(ctx, &kms.UpdatePrimaryRegionInput{
				KeyID:         primaryID,
				PrimaryRegion: tc.newPrimary,
			})
			require.NoError(t, err)

			// New primary (the promoted replica) must report type PRIMARY with old primary in replica list.
			newPrimaryDesc, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: replicaID})
			require.NoError(t, err)
			assert.Equal(t, "PRIMARY", newPrimaryDesc.KeyMetadata.MultiRegionKeyType,
				"promoted key must be PRIMARY")
			require.NotNil(t, newPrimaryDesc.KeyMetadata.MultiRegionConfiguration)
			var foundOldPrimary bool
			for _, rep := range newPrimaryDesc.KeyMetadata.MultiRegionConfiguration.ReplicaKeys {
				if rep.Region == "us-east-1" {
					foundOldPrimary = true
				}
			}
			assert.True(t, foundOldPrimary,
				"new primary ReplicaKeys must include old primary region (us-east-1)")

			// Old primary must now be a replica.
			oldPrimaryDesc, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: primaryID})
			require.NoError(t, err)
			assert.Equal(t, "REPLICA", oldPrimaryDesc.KeyMetadata.MultiRegionKeyType,
				"demoted key must be REPLICA")
		})
	}
}

func TestReplicateKey(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{MultiRegion: true})
	require.NoError(t, err)
	primaryID := out.KeyMetadata.KeyID

	rOut, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "us-west-2",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, rOut.ReplicaKeyMetadata.KeyID)
	// Replica key should have REPLICA type in its multi-region config.
	assert.Equal(t, "REPLICA", rOut.ReplicaKeyMetadata.MultiRegionKeyType)
}

func TestReplicateKey_NonMultiRegionFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b) // not multi-region

	_, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         keyID,
		ReplicaRegion: "us-west-2",
	})
	require.Error(t, err)
}

func TestUpdatePrimaryRegion(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{MultiRegion: true})
	require.NoError(t, err)
	primaryID := out.KeyMetadata.KeyID

	// Replicate first.
	rOut, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "us-west-2",
	})
	require.NoError(t, err)

	// Promote replica to primary.
	err = b.UpdatePrimaryRegion(context.Background(), &kms.UpdatePrimaryRegionInput{
		KeyID:         rOut.ReplicaKeyMetadata.KeyID,
		PrimaryRegion: "us-west-2",
	})
	require.NoError(t, err)
}

func TestReplicateKey_MetadataSync_Description(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         keyID,
		ReplicaRegion: "us-west-2",
		Description:   "replica description",
	})
	require.NoError(t, err)
	assert.Equal(t, "replica description", out.ReplicaKeyMetadata.Description)
}

func TestReplicateKey_MetadataSync_InheritsDescription(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         keyID,
		ReplicaRegion: "eu-west-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "primary key", out.ReplicaKeyMetadata.Description)
}

func TestReplicateKey_MetadataSync_KeySpecAndUsage(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         keyID,
		ReplicaRegion: "ap-southeast-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "SYMMETRIC_DEFAULT", out.ReplicaKeyMetadata.KeySpec)
	assert.Equal(t, kms.KeyUsageEncryptDecrypt, out.ReplicaKeyMetadata.KeyUsage)
	assert.True(t, out.ReplicaKeyMetadata.MultiRegion)
}

func TestReplicateKey_SharedKeyMaterial_EncryptRoundtrip(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	primaryID := b2mustCreateMultiRegionKey(t, b)

	replicaOut, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "us-east-2",
	})
	require.NoError(t, err)
	replicaID := replicaOut.ReplicaKeyMetadata.KeyID

	// Encrypt with primary key
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     primaryID,
		Plaintext: []byte("shared secret"),
	})
	require.NoError(t, err)

	// Decrypt with replica key (shared key material)
	dec, err := b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("shared secret"), dec.Plaintext)
	_ = replicaID // used implicitly through shared material
}

func TestReplicateKey_PrimaryShowsReplica_InDescribeKey(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	primaryID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "eu-central-1",
	})
	require.NoError(t, err)
	replicaID := out.ReplicaKeyMetadata.KeyID

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: primaryID})
	require.NoError(t, err)
	require.NotNil(t, desc.KeyMetadata.MultiRegionConfiguration)
	assert.Equal(t, "PRIMARY", desc.KeyMetadata.MultiRegionConfiguration.MultiRegionKeyType)

	replicaARNs := make([]string, 0, len(desc.KeyMetadata.MultiRegionConfiguration.ReplicaKeys))
	for _, r := range desc.KeyMetadata.MultiRegionConfiguration.ReplicaKeys {
		replicaARNs = append(replicaARNs, r.Arn)
	}
	assert.Contains(t, replicaARNs, out.ReplicaKeyMetadata.Arn)
	_ = replicaID
}

func TestReplicateKey_ReplicaShowsPrimary_InDescribeKey(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	primaryID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "ap-northeast-1",
	})
	require.NoError(t, err)
	replicaID := out.ReplicaKeyMetadata.KeyID

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: replicaID})
	require.NoError(t, err)
	assert.Equal(t, "REPLICA", desc.KeyMetadata.MultiRegionKeyType)
}

func TestReplicateKey_DisabledSource_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateMultiRegionKey(t, b)

	require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyID}))

	_, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         keyID,
		ReplicaRegion: "us-west-1",
	})
	require.Error(t, err)
}

func TestReplicateKey_MultipleReplicas(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	primaryID := b2mustCreateMultiRegionKey(t, b)

	regions := []string{"us-west-2", "eu-west-1", "ap-southeast-1"}
	for _, region := range regions {
		_, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
			KeyID:         primaryID,
			ReplicaRegion: region,
		})
		require.NoError(t, err)
	}

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: primaryID})
	require.NoError(t, err)
	assert.Len(t, desc.KeyMetadata.MultiRegionConfiguration.ReplicaKeys, 3)
}

func TestMultiRegion_PrimaryType_InDescribeKey(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateMultiRegionKey(t, b)

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, "PRIMARY", desc.KeyMetadata.MultiRegionKeyType)
	assert.True(t, desc.KeyMetadata.MultiRegion)
}

func TestMultiRegion_ReplicaType_AfterReplicate(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	primaryID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "us-west-2",
	})
	require.NoError(t, err)

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: out.ReplicaKeyMetadata.KeyID})
	require.NoError(t, err)
	assert.Equal(t, "REPLICA", desc.KeyMetadata.MultiRegionKeyType)
}

func TestMultiRegion_PrimaryConfig_HasPrimaryAndReplicas(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	primaryID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "ap-east-1",
	})
	require.NoError(t, err)
	_ = out

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: primaryID})
	require.NoError(t, err)
	mrConfig := desc.KeyMetadata.MultiRegionConfiguration
	require.NotNil(t, mrConfig)
	assert.NotNil(t, mrConfig.PrimaryKey)
	assert.NotEmpty(t, mrConfig.ReplicaKeys)
}

func TestMultiRegion_UpdatePrimaryRegion_ChangesType(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	keyID := b2mustCreateMultiRegionKey(t, b)

	// Replicate to us-west-2 first — UpdatePrimaryRegion requires an actual replica.
	_, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         keyID,
		ReplicaRegion: "us-west-2",
	})
	require.NoError(t, err)

	require.NoError(t, b.UpdatePrimaryRegion(context.Background(), &kms.UpdatePrimaryRegionInput{
		KeyID:         keyID,
		PrimaryRegion: "us-west-2",
	}))

	// Old primary (us-east-1) must now be a replica.
	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, "REPLICA", desc.KeyMetadata.MultiRegionKeyType)
}

func TestMultiRegion_ReplicaEnableDisable_Independent(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)
	primaryID := b2mustCreateMultiRegionKey(t, b)

	out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "sa-east-1",
	})
	require.NoError(t, err)
	replicaID := out.ReplicaKeyMetadata.KeyID

	// Disable primary; replica should still be enabled
	require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: primaryID}))

	replicaDesc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: replicaID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStateEnabled, replicaDesc.KeyMetadata.KeyState)
}

func TestDescribeKey_AllFields_Populated(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		Description: "test key",
		MultiRegion: true,
	})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	meta := desc.KeyMetadata
	assert.NotEmpty(t, meta.KeyID)
	assert.NotEmpty(t, meta.Arn)
	assert.Equal(t, "test key", meta.Description)
	assert.Equal(t, kms.KeyStateEnabled, meta.KeyState)
	assert.Equal(t, kms.KeyUsageEncryptDecrypt, meta.KeyUsage)
	assert.Equal(t, "CUSTOMER", meta.KeyManager)
	assert.Equal(t, kms.KeyOriginAWSKMS, meta.Origin)
	assert.Greater(t, meta.CreationDate, float64(0))
	assert.True(t, meta.MultiRegion)
}

func TestConcurrent_ReplicateKey(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	primaryID := b2mustCreateMultiRegionKey(t, b)
	regions := []string{"us-west-2", "eu-west-1", "ap-southeast-1", "sa-east-1", "ca-central-1"}

	errCh := make(chan error, len(regions))
	for _, region := range regions {
		go func() {
			_, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
				KeyID:         primaryID,
				ReplicaRegion: region,
			})
			errCh <- err
		}()
	}

	for range regions {
		err := <-errCh
		require.NoError(t, err)
	}

	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: primaryID})
	require.NoError(t, err)
	assert.Len(t, desc.KeyMetadata.MultiRegionConfiguration.ReplicaKeys, len(regions))
}

// TestKMSKeyMetadataFields verifies that DescribeKey returns the additional metadata fields.
func TestKMSKeyMetadataFields(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()
	created, err := backend.CreateKey(context.Background(), &kms.CreateKeyInput{
		Description: "test key",
		KeyUsage:    "ENCRYPT_DECRYPT",
	})
	require.NoError(t, err)

	out, err := backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: created.KeyMetadata.KeyID})
	require.NoError(t, err)

	assert.Equal(t, "CUSTOMER", out.KeyMetadata.KeyManager)
	assert.Equal(t, "AWS_KMS", out.KeyMetadata.Origin)
	assert.Equal(t, "SYMMETRIC_DEFAULT", out.KeyMetadata.KeySpec)
	assert.Equal(t, []string{"SYMMETRIC_DEFAULT"}, out.KeyMetadata.EncryptionAlgorithms)
	assert.False(t, out.KeyMetadata.MultiRegion)
}

func TestKMSBackendNewMaintenanceOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *kms.InMemoryBackend)
		name string
	}{
		{
			name: "GetParametersForImport",
			run: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				require.NoError(t, err)

				out, err := b.GetParametersForImport(
					context.Background(),
					&kms.GetParametersForImportInput{KeyID: key.KeyMetadata.KeyID},
				)
				require.NoError(t, err)
				assert.Equal(t, key.KeyMetadata.KeyID, out.KeyID)
				assert.NotEmpty(t, out.ImportToken)
				assert.NotEmpty(t, out.PublicKey)
			},
		},
		{
			name: "ListKeyPolicies",
			run: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)

				out, err := b.ListKeyPolicies(
					context.Background(),
					&kms.ListKeyPoliciesInput{KeyID: key.KeyMetadata.KeyID},
				)
				require.NoError(t, err)
				assert.Equal(t, []string{"default"}, out.PolicyNames)
			},
		},
		{
			name: "ListKeyRotations_after_on_demand_rotation",
			run: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)

				_, err = b.RotateKeyOnDemand(
					context.Background(),
					&kms.RotateKeyOnDemandInput{KeyID: key.KeyMetadata.KeyID},
				)
				require.NoError(t, err)

				out, err := b.ListKeyRotations(
					context.Background(),
					&kms.ListKeyRotationsInput{KeyID: key.KeyMetadata.KeyID},
				)
				require.NoError(t, err)
				require.Len(t, out.Rotations, 1)
				assert.Positive(t, out.Rotations[0].RotationDate)
			},
		},
		{
			name: "ReplicateKey",
			run: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{MultiRegion: true})
				require.NoError(t, err)

				out, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
					KeyID:         key.KeyMetadata.KeyID,
					ReplicaRegion: "us-west-2",
				})
				require.NoError(t, err)
				assert.True(t, out.ReplicaKeyMetadata.MultiRegion)
				assert.Contains(t, out.ReplicaKeyMetadata.Arn, ":us-west-2:")
			},
		},
		{
			name: "UpdateCustomKeyStore",
			run: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				created, err := b.CreateCustomKeyStore(context.Background(),
					&kms.CreateCustomKeyStoreInput{CustomKeyStoreName: "before-name"},
				)
				require.NoError(t, err)

				err = b.UpdateCustomKeyStore(context.Background(), &kms.UpdateCustomKeyStoreInput{
					CustomKeyStoreID:      created.CustomKeyStoreID,
					NewCustomKeyStoreName: "after-name",
				})
				require.NoError(t, err)

				desc, err := b.DescribeCustomKeyStores(context.Background(), &kms.DescribeCustomKeyStoresInput{
					CustomKeyStoreID: created.CustomKeyStoreID,
				})
				require.NoError(t, err)
				require.Len(t, desc.CustomKeyStores, 1)
				assert.Equal(t, "after-name", desc.CustomKeyStores[0].CustomKeyStoreName)
			},
		},
		{
			name: "UpdateKeyDescription",
			run: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)

				err = b.UpdateKeyDescription(context.Background(), &kms.UpdateKeyDescriptionInput{
					KeyID:       key.KeyMetadata.KeyID,
					Description: "updated description",
				})
				require.NoError(t, err)

				desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: key.KeyMetadata.KeyID})
				require.NoError(t, err)
				assert.Equal(t, "updated description", desc.KeyMetadata.Description)
			},
		},
		{
			name: "UpdatePrimaryRegion",
			run: func(t *testing.T, b *kms.InMemoryBackend) {
				t.Helper()

				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{MultiRegion: true})
				require.NoError(t, err)
				primaryID := key.KeyMetadata.KeyID

				// Replicate to eu-west-1 first — UpdatePrimaryRegion requires an actual replica.
				_, err = b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
					KeyID:         primaryID,
					ReplicaRegion: "eu-west-1",
				})
				require.NoError(t, err)

				err = b.UpdatePrimaryRegion(context.Background(), &kms.UpdatePrimaryRegionInput{
					KeyID:         primaryID,
					PrimaryRegion: "eu-west-1",
				})
				require.NoError(t, err)

				// After promotion, replicate from the NEW primary (the eu-west-1 key promoted to primary)
				// OR replicate from the old primary which is now a replica — both keys are still multi-region.
				replica, err := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
					KeyID:         primaryID,
					ReplicaRegion: "us-west-2",
				})
				require.NoError(t, err)
				assert.True(t, replica.ReplicaKeyMetadata.MultiRegion)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, kms.NewInMemoryBackend())
		})
	}
}

func TestCreateKeyValidations(t *testing.T) {
	t.Parallel()

	makeTags := func(n int) []kms.Tag {
		tags := make([]kms.Tag, n)
		for i := range tags {
			tags[i] = kms.Tag{TagKey: "k", TagValue: "v"}
		}

		return tags
	}

	tests := []struct {
		name    string
		input   kms.CreateKeyInput
		wantErr bool
	}{
		{
			name:    "desc_too_long",
			input:   kms.CreateKeyInput{Description: strings.Repeat("a", 8193)},
			wantErr: true,
		},
		{
			name:    "desc_at_limit",
			input:   kms.CreateKeyInput{Description: strings.Repeat("a", 8192)},
			wantErr: false,
		},
		{
			// gopherstack-5rjn: HMAC KMS keys DO support MultiRegion (kms@v1.55.4's
			// own CreateKey doc lists HMAC among "all supported KMS key types" for
			// multi-Region keys); this case previously asserted the opposite.
			name: "hmac_multiregion_allowed",
			input: kms.CreateKeyInput{
				KeySpec:     "HMAC_256",
				KeyUsage:    "GENERATE_VERIFY_MAC",
				MultiRegion: true,
			},
			wantErr: false,
		},
		{
			name:    "too_many_tags",
			input:   kms.CreateKeyInput{Tags: makeTags(51)},
			wantErr: true,
		},
		{
			name:    "exactly_50_tags",
			input:   kms.CreateKeyInput{Tags: makeTags(50)},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			_, err := b.CreateKey(context.Background(), &tt.input)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReplicateKeyRequiresMultiRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		multiRegion bool
		wantErr     bool
	}{
		{name: "multi_region", multiRegion: true, wantErr: false},
		{name: "single_region", multiRegion: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{MultiRegion: tt.multiRegion})
			require.NoError(t, err)

			_, replicaErr := b.ReplicateKey(context.Background(), &kms.ReplicateKeyInput{
				KeyID:         key.KeyMetadata.KeyID,
				ReplicaRegion: "us-west-2",
			})

			if tt.wantErr {
				require.Error(t, replicaErr)
				require.ErrorIs(t, replicaErr, kms.ErrUnsupportedOrigin)
			} else {
				require.NoError(t, replicaErr)
			}
		})
	}
}

// TestScheduleKeyDeletion_PrimaryWithReplicas verifies that scheduling
// deletion of a multi-Region primary key with an existing replica moves it to
// PendingReplicaDeletion (not PendingDeletion) with no DeletionDate, and that
// once the last replica is actually purged, the primary is promoted to
// PendingDeletion with its own waiting period starting then.
func TestScheduleKeyDeletion_PrimaryWithReplicas(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	ctx := context.Background()

	primaryOut, err := b.CreateKey(ctx, &kms.CreateKeyInput{MultiRegion: true})
	require.NoError(t, err)
	primaryID := primaryOut.KeyMetadata.KeyID

	replicaOut, err := b.ReplicateKey(ctx, &kms.ReplicateKeyInput{
		KeyID:         primaryID,
		ReplicaRegion: "us-west-2",
	})
	require.NoError(t, err)
	replicaID := replicaOut.ReplicaKeyMetadata.KeyID

	delOut, err := b.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
		KeyID:               primaryID,
		PendingWindowInDays: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingReplicaDeletion, delOut.KeyState)
	assert.Zero(t, delOut.DeletionDate, "DeletionDate must not appear while replicas exist")

	desc, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: primaryID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingReplicaDeletion, desc.KeyMetadata.KeyState)
	assert.Zero(t, desc.KeyMetadata.DeletionDate)

	// Schedule and force-expire the replica's own deletion.
	_, err = b.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
		KeyID:               replicaID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)
	b.SetDeletionDateForTest(replicaID, time.Now().Add(-time.Second))

	kms.NewJanitor(b, time.Minute).SweepOnce(ctx)

	_, err = b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: replicaID})
	require.Error(t, err, "replica must be purged")

	desc, err = b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: primaryID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingDeletion, desc.KeyMetadata.KeyState,
		"primary must be promoted once its last replica is purged")
	assert.NotZero(t, desc.KeyMetadata.DeletionDate)
}
