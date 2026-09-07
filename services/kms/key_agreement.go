package kms

import (
	"context"
	"fmt"
)

// DeriveSharedSecret computes an ECDH shared secret using an ECC KEY_AGREEMENT KMS key
// and the provided DER-encoded peer public key.
func (b *InMemoryBackend) DeriveSharedSecret(
	ctx context.Context, input *DeriveSharedSecretInput,
) (*DeriveSharedSecretOutput, error) {
	if input.KeyAgreementAlgorithm != "" && input.KeyAgreementAlgorithm != algoECDH {
		return nil, fmt.Errorf(
			"%w: KeyAgreementAlgorithm must be ECDH, got %q",
			ErrValidation, input.KeyAgreementAlgorithm,
		)
	}

	if len(input.PublicKey) == 0 {
		return nil, fmt.Errorf("%w: PublicKey must not be empty", ErrValidation)
	}

	b.mu.RLock("DeriveSharedSecret")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageKeyAgreement {
		return nil, fmt.Errorf(
			"%w: key %q must have KeyUsage=%s for DeriveSharedSecret",
			ErrInvalidKeyUsage, key.KeyID, KeyUsageKeyAgreement,
		)
	}

	if err = b.validateGrantTokenPresence(input.GrantTokens, "DeriveSharedSecret"); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	sharedSecret, err := deriveECDH(input.PublicKey, km)
	if err != nil {
		return nil, err
	}

	algo := input.KeyAgreementAlgorithm
	if algo == "" {
		algo = algoECDH
	}

	b.recordLastUsage(region, key.KeyID, "DeriveSharedSecret")

	return &DeriveSharedSecretOutput{
		KeyID:                 key.Arn,
		SharedSecret:          sharedSecret,
		KeyAgreementAlgorithm: algo,
	}, nil
}
