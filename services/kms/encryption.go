package kms

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// encryptData encrypts plaintext using the per-key AES-256-GCM material, embedding the key ID.
// Kept as a compatibility shim; callers should use encryptSymmetric directly.
func encryptData(
	plaintext []byte,
	keyID string,
	encCtx map[string]string,
	km *keyMaterial,
) ([]byte, error) {
	return encryptSymmetric(plaintext, keyID, encCtx, km)
}

// decryptData decrypts a ciphertext blob produced by encryptData.
// Returns (plaintext, resolvedKeyID, error).
func decryptData(blob []byte, encCtx map[string]string, km *keyMaterial) ([]byte, string, error) {
	return decryptSymmetric(blob, encCtx, km)
}

// encryptionAlgorithmForSpec returns the EncryptionAlgorithm string for a given key spec.
// Returns encryptionAlgorithmRSAOAEP for RSA keys and "SYMMETRIC_DEFAULT" for symmetric keys.
func encryptionAlgorithmForSpec(keySpec string) string {
	switch keySpec {
	case keySpecRSA2048, keySpecRSA3072, keySpecRSA4096:
		return encryptionAlgorithmRSAOAEP
	default:
		return encryptionAlgorithmSymmetric
	}
}

// Encrypt encrypts the given plaintext using the specified key.
func (b *InMemoryBackend) Encrypt(
	ctx context.Context,
	input *EncryptInput,
) (*EncryptOutput, error) {
	// gopherstack-i4q8: Encrypt does not declare LimitExceededException (unlike
	// CreateKey/CreateGrant, which do and use it for this same "exceeds a
	// length" shape); nothing in Encrypt's declared set fits. Landmine.
	if len(input.Plaintext) > maxPlaintextBytes {
		return nil, fmt.Errorf(
			"%w: plaintext must not exceed %d bytes, got %d",
			ErrValidation, maxPlaintextBytes, len(input.Plaintext),
		)
	}

	if err := validateEncryptionContextSize(input.EncryptionContext); err != nil {
		return nil, err
	}

	b.mu.RLock("Encrypt")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	// Encrypt's deserializeOpError does not recognize InvalidArnException (gopherstack-qxaj).
	key, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageEncryptDecrypt {
		return nil, fmt.Errorf(
			"%w: key %q is not usable for encryption",
			ErrInvalidKeyUsage,
			key.KeyID,
		)
	}

	if err = b.checkKeyMaterialExpiry(key); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	if err = b.validateGrantTokenConstraints(ctx, input.GrantTokens, "Encrypt", input.EncryptionContext); err != nil {
		return nil, err
	}

	blob, err := b.encryptPayload(input.Plaintext, key.KeyID, input.EncryptionContext, km)
	if err != nil {
		return nil, err
	}

	b.recordLastUsage(region, key.KeyID, "Encrypt")

	return &EncryptOutput{
		CiphertextBlob:      blob,
		KeyID:               key.Arn,
		EncryptionAlgorithm: encryptionAlgorithmForSpec(key.KeySpec),
	}, nil
}

// encryptPayload dispatches to RSA-OAEP or symmetric encryption depending on key type.
// Must be called with at least a read lock held.
func (*InMemoryBackend) encryptPayload(
	plaintext []byte,
	keyID string,
	encCtx map[string]string,
	km *keyMaterial,
) ([]byte, error) {
	if km.rsaKey != nil {
		// RSA ENCRYPT_DECRYPT keys use RSA-OAEP-SHA256.
		// Prepend the key ID prefix so Decrypt can identify the key.
		rsaBlob, encErr := encryptRSAOAEP(plaintext, km)
		if encErr != nil {
			return nil, encErr
		}

		full := make([]byte, keyIDPrefixLen+len(rsaBlob))
		copy(full[:keyIDPrefixLen], padKeyID(keyID))
		copy(full[keyIDPrefixLen:], rsaBlob)

		return full, nil
	}

	return encryptData(plaintext, keyID, encCtx, km)
}

// Decrypt decrypts the given ciphertext blob.
// verifyKeyIDHint validates a caller-supplied key identifier (Decrypt's KeyId or
// ReEncrypt's SourceKeyId) against the key ID embedded in the ciphertext blob.
// When the hint is empty it is a no-op (AWS reads the key from the symmetric blob
// metadata). When the hint resolves to a different key, AWS KMS rejects the request
// with IncorrectKeyException rather than silently using the embedded key.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) verifyKeyIDHint(
	ctx context.Context,
	hint, embeddedKeyID, paramName string,
) error {
	if hint == "" {
		return nil
	}

	// Neither Decrypt nor ReEncrypt (the two callers of this hint check) recognizes
	// InvalidArnException (gopherstack-qxaj).
	hintResolved, _, err := b.resolveKeyID(ctx, hint, ErrKeyNotFound)
	if err != nil {
		return err
	}

	if hintResolved != embeddedKeyID {
		return fmt.Errorf(
			"%w: provided %s %q does not match the key that encrypted the ciphertext",
			ErrIncorrectKey, paramName, hint,
		)
	}

	return nil
}

func (b *InMemoryBackend) Decrypt(
	ctx context.Context,
	input *DecryptInput,
) (*DecryptOutput, error) {
	if err := validateEncryptionContextSize(input.EncryptionContext); err != nil {
		return nil, err
	}

	b.mu.RLock("Decrypt")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	// Extract the key ID from the blob prefix first, then look up material.
	if len(input.CiphertextBlob) < keyIDPrefixLen {
		return nil, ErrCiphertextTooShort
	}

	keyID := strings.TrimRight(string(input.CiphertextBlob[:keyIDPrefixLen]), "\x00")

	// If the caller provided a KeyId hint, verify it matches the embedded key ID.
	if err := b.verifyKeyIDHint(ctx, input.KeyID, keyID, "KeyId"); err != nil {
		return nil, err
	}

	key, lookupErr := b.lookupKey(ctx, keyID, ErrKeyNotFound)
	if lookupErr != nil {
		return nil, lookupErr
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageEncryptDecrypt {
		return nil, fmt.Errorf(
			"%w: key %q is not usable for decryption",
			ErrInvalidKeyUsage,
			key.KeyID,
		)
	}

	if err := b.checkKeyMaterialExpiry(key); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	if err = b.validateGrantTokenConstraints(ctx, input.GrantTokens, "Decrypt", input.EncryptionContext); err != nil {
		return nil, err
	}

	cipherPayload := input.CiphertextBlob[keyIDPrefixLen:]

	plaintext, err := b.decryptPayload(
		region,
		input.CiphertextBlob,
		cipherPayload,
		input.EncryptionContext,
		key,
		km,
	)
	if err != nil {
		return nil, err
	}

	// Defense-in-depth: a tampered ciphertext that decrypts could in theory exceed
	// the maximum plaintext that AWS allows on encrypt. Reject to mirror behavior.
	if len(plaintext) > maxPlaintextBytes {
		return nil, fmt.Errorf(
			"%w: decrypted plaintext exceeds %d bytes",
			ErrInvalidCiphertext, maxPlaintextBytes,
		)
	}

	b.recordLastUsage(region, key.KeyID, "Decrypt")

	return &DecryptOutput{
		Plaintext:           plaintext,
		KeyID:               key.Arn,
		EncryptionAlgorithm: encryptionAlgorithmForSpec(key.KeySpec),
	}, nil
}

// decryptPayload dispatches to RSA-OAEP or symmetric decryption depending on key type.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) decryptPayload(
	region string,
	fullBlob, rsaPayload []byte,
	encCtx map[string]string,
	key *Key,
	km *keyMaterial,
) ([]byte, error) {
	if km.rsaKey != nil {
		return decryptRSAOAEP(rsaPayload, km)
	}

	plaintext, _, err := decryptData(fullBlob, encCtx, km)
	if err == nil {
		return plaintext, nil
	}

	// Try previous key material versions (produced by key rotation).
	return b.decryptWithHistory(region, fullBlob, encCtx, key.KeyID)
}

// decryptWithHistory attempts to decrypt a blob using previous key material versions.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) decryptWithHistory(
	region string,
	blob []byte,
	encCtx map[string]string,
	keyID string,
) ([]byte, error) {
	history := b.keyMaterialHistoryStore(region)[keyID]
	for _, v := range slices.Backward(history) {
		plaintext, _, err := decryptData(blob, encCtx, v)
		if err == nil {
			return plaintext, nil
		}
	}

	return nil, ErrInvalidCiphertext
}

// ReEncrypt decrypts a ciphertext and re-encrypts it under a different key.
func (b *InMemoryBackend) ReEncrypt(
	ctx context.Context,
	input *ReEncryptInput,
) (*ReEncryptOutput, error) {
	if err := validateReEncryptInput(input); err != nil {
		return nil, err
	}

	b.mu.RLock("ReEncrypt")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	plaintext, sourceKey, err := b.reEncryptDecrypt(ctx, region, input)
	if err != nil {
		return nil, err
	}

	blob, destKey, err := b.reEncryptEncrypt(ctx, region, plaintext, input)
	if err != nil {
		return nil, err
	}

	b.recordLastUsage(region, sourceKey.KeyID, "ReEncrypt")
	b.recordLastUsage(region, destKey.KeyID, "ReEncrypt")

	return &ReEncryptOutput{
		CiphertextBlob:                 blob,
		KeyID:                          destKey.Arn,
		SourceKeyID:                    sourceKey.Arn,
		SourceEncryptionAlgorithm:      encryptionAlgorithmForSpec(sourceKey.KeySpec),
		DestinationEncryptionAlgorithm: encryptionAlgorithmForSpec(destKey.KeySpec),
	}, nil
}

func (b *InMemoryBackend) reEncryptDecrypt(
	ctx context.Context,
	region string,
	input *ReEncryptInput,
) ([]byte, *Key, error) {
	if len(input.CiphertextBlob) < keyIDPrefixLen {
		return nil, nil, ErrCiphertextTooShort
	}

	sourceKeyID := strings.TrimRight(string(input.CiphertextBlob[:keyIDPrefixLen]), "\x00")

	if err := b.verifyKeyIDHint(ctx, input.SourceKeyID, sourceKeyID, "SourceKeyId"); err != nil {
		return nil, nil, err
	}

	sourceKey, err := b.lookupKey(ctx, sourceKeyID, ErrKeyNotFound)
	if err != nil {
		return nil, nil, err
	}

	if sourceKey.KeyState != KeyStateEnabled {
		return nil, nil, keyStateError(sourceKey)
	}

	if sourceKey.KeyUsage != KeyUsageEncryptDecrypt {
		return nil, nil, fmt.Errorf(
			"%w: source key %q is not usable for decryption",
			ErrInvalidKeyUsage,
			sourceKey.KeyID,
		)
	}

	sourceKM, err := b.requireKeyMaterial(region, sourceKey)
	if err != nil {
		return nil, nil, err
	}

	plaintext, _, decErr := decryptData(
		input.CiphertextBlob,
		input.SourceEncryptionContext,
		sourceKM,
	)
	if decErr != nil {
		plaintext, decErr = b.decryptWithHistory(
			region,
			input.CiphertextBlob,
			input.SourceEncryptionContext,
			sourceKey.KeyID,
		)
		if decErr != nil {
			return nil, nil, decErr
		}
	}

	return plaintext, sourceKey, nil
}

func (b *InMemoryBackend) reEncryptEncrypt(
	ctx context.Context,
	region string,
	plaintext []byte,
	input *ReEncryptInput,
) ([]byte, *Key, error) {
	destKey, err := b.lookupKey(ctx, input.DestinationKeyID, ErrKeyNotFound)
	if err != nil {
		return nil, nil, err
	}

	if destKey.KeyState != KeyStateEnabled {
		return nil, nil, keyStateError(destKey)
	}

	if destKey.KeyUsage != KeyUsageEncryptDecrypt {
		return nil, nil, fmt.Errorf(
			"%w: destination key %q is not usable for encryption",
			ErrInvalidKeyUsage,
			destKey.KeyID,
		)
	}

	destKM, err := b.requireKeyMaterial(region, destKey)
	if err != nil {
		return nil, nil, err
	}

	blob, err := encryptData(
		plaintext,
		destKey.KeyID,
		input.DestinationEncryptionContext,
		destKM,
	)
	if err != nil {
		return nil, nil, err
	}

	return blob, destKey, nil
}
