package kms

import (
	"context"
	"fmt"
)

// GenerateMac computes an HMAC tag over the provided message using an HMAC KMS key.
func (b *InMemoryBackend) GenerateMac(
	ctx context.Context,
	input *GenerateMacInput,
) (*GenerateMacOutput, error) {
	if input.MacAlgorithm == "" {
		return nil, fmt.Errorf("%w: MacAlgorithm must not be empty", ErrValidation)
	}

	b.mu.RLock("GenerateMac")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageGenerateMac {
		return nil, fmt.Errorf(
			"%w: key %q must have KeyUsage=%s for GenerateMac",
			ErrInvalidKeyUsage, key.KeyID, KeyUsageGenerateMac,
		)
	}

	if algErr := validateMacAlgorithm(input.MacAlgorithm, key.KeySpec); algErr != nil {
		return nil, algErr
	}

	if err = b.validateGrantTokenPresence(input.GrantTokens, "GenerateMac"); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	mac, err := computeHMAC(input.Message, input.MacAlgorithm, km)
	if err != nil {
		return nil, err
	}

	b.recordLastUsage(region, key.KeyID, "GenerateMac")

	return &GenerateMacOutput{
		KeyID:        key.Arn,
		Mac:          mac,
		MacAlgorithm: input.MacAlgorithm,
	}, nil
}

// VerifyMac verifies an HMAC tag over the provided message using an HMAC KMS key.
// Returns an error if the MAC does not match; on success returns the key ARN and algorithm.
func (b *InMemoryBackend) VerifyMac(
	ctx context.Context,
	input *VerifyMacInput,
) (*VerifyMacOutput, error) {
	if input.MacAlgorithm == "" {
		return nil, fmt.Errorf("%w: MacAlgorithm must not be empty", ErrValidation)
	}

	b.mu.RLock("VerifyMac")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageGenerateMac {
		return nil, fmt.Errorf(
			"%w: key %q must have KeyUsage=%s for VerifyMac",
			ErrInvalidKeyUsage, key.KeyID, KeyUsageGenerateMac,
		)
	}

	if algErr := validateMacAlgorithm(input.MacAlgorithm, key.KeySpec); algErr != nil {
		return nil, algErr
	}

	if err = b.validateGrantTokenPresence(input.GrantTokens, "VerifyMac"); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	expected, err := computeHMAC(input.Message, input.MacAlgorithm, km)
	if err != nil {
		return nil, err
	}

	if !hmacEqual(expected, input.Mac) {
		return nil, fmt.Errorf("%w: MAC verification failed", ErrInvalidSignature)
	}

	b.recordLastUsage(region, key.KeyID, "VerifyMac")

	return &VerifyMacOutput{
		KeyID:        key.Arn,
		MacAlgorithm: input.MacAlgorithm,
		MacValid:     true,
	}, nil
}
