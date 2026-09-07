package kms

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
)

// GenerateDataKey generates a random data key, returning both plaintext and encrypted forms.
func (b *InMemoryBackend) GenerateDataKey(
	ctx context.Context,
	input *GenerateDataKeyInput,
) (*GenerateDataKeyOutput, error) {
	return b.generateDataKey(ctx, input, "GenerateDataKey")
}

// generateDataKey implements GenerateDataKey and GenerateDataKeyWithoutPlaintext, which share
// identical validation and key material but must be checked against the grant Operations list
// under their own (distinct) grant-operation names.
func (b *InMemoryBackend) generateDataKey(
	ctx context.Context,
	input *GenerateDataKeyInput,
	operation string,
) (*GenerateDataKeyOutput, error) {
	if err := validateGenerateDataKeyInput(input); err != nil {
		return nil, err
	}

	b.mu.RLock("GenerateDataKey")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageEncryptDecrypt {
		return nil, fmt.Errorf(
			"%w: key %q is not usable for data key generation",
			ErrInvalidKeyUsage,
			key.KeyID,
		)
	}

	// Validate requested data key size to prevent excessive memory allocation.
	if input.NumberOfBytes != nil {
		if *input.NumberOfBytes <= 0 || *input.NumberOfBytes > maxDataKeyBytes {
			return nil, ErrInvalidDataKeySize
		}
	}

	keyBytes := min(dataKeySize(input.KeySpec, input.NumberOfBytes), maxDataKeyBytes)

	plaintextKey := make([]byte, keyBytes)
	if _, randErr := io.ReadFull(rand.Reader, plaintextKey); randErr != nil {
		return nil, randErr
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	if err = b.validateGrantTokenConstraints(ctx, input.GrantTokens, operation, input.EncryptionContext); err != nil {
		return nil, err
	}

	blob, encErr := encryptData(plaintextKey, key.KeyID, input.EncryptionContext, km)
	if encErr != nil {
		return nil, encErr
	}

	b.recordLastUsage(region, key.KeyID, operation)

	return &GenerateDataKeyOutput{
		CiphertextBlob: blob,
		Plaintext:      plaintextKey,
		KeyID:          key.Arn,
	}, nil
}

// dataKeySize returns the number of bytes for a data key based on spec and override.
func dataKeySize(keySpec string, numBytes *int32) int {
	if numBytes != nil && *numBytes > 0 {
		return int(*numBytes)
	}

	if keySpec == "AES_128" {
		return aes128Bytes
	}

	return aes256Bytes
}

// GenerateDataKeyWithoutPlaintext generates a data key but returns only the encrypted copy.
func (b *InMemoryBackend) GenerateDataKeyWithoutPlaintext(
	ctx context.Context, input *GenerateDataKeyWithoutPlaintextInput,
) (*GenerateDataKeyWithoutPlaintextOutput, error) {
	out, err := b.generateDataKey(ctx, &GenerateDataKeyInput{
		KeyID:             input.KeyID,
		KeySpec:           input.KeySpec,
		NumberOfBytes:     input.NumberOfBytes,
		EncryptionContext: input.EncryptionContext,
		GrantTokens:       input.GrantTokens,
	}, "GenerateDataKeyWithoutPlaintext")
	if err != nil {
		return nil, err
	}

	return &GenerateDataKeyWithoutPlaintextOutput{
		KeyID:          out.KeyID,
		CiphertextBlob: out.CiphertextBlob,
	}, nil
}

// GenerateDataKeyPair generates a new ephemeral asymmetric key pair, returning the public key,
// plaintext private key (DER-encoded PKCS#8), and the private key encrypted under the specified
// KMS wrapping key.
func (b *InMemoryBackend) GenerateDataKeyPair(
	ctx context.Context, input *GenerateDataKeyPairInput,
) (*GenerateDataKeyPairOutput, error) {
	return b.generateDataKeyPair(ctx, input, "GenerateDataKeyPair")
}

// generateDataKeyPair implements GenerateDataKeyPair and GenerateDataKeyPairWithoutPlaintext,
// which share identical validation and key material but must be checked against the grant
// Operations list under their own (distinct) grant-operation names.
func (b *InMemoryBackend) generateDataKeyPair(
	ctx context.Context, input *GenerateDataKeyPairInput, operation string,
) (*GenerateDataKeyPairOutput, error) {
	if input.KeyPairSpec == "" {
		return nil, fmt.Errorf("%w: KeyPairSpec must not be empty", ErrUnsupportedParameter)
	}

	if err := validateEncryptionContextSize(input.EncryptionContext); err != nil {
		return nil, err
	}

	b.mu.RLock("GenerateDataKeyPair")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	wrapKey, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if wrapKey.KeyState != KeyStateEnabled {
		return nil, keyStateError(wrapKey)
	}

	if wrapKey.KeyUsage != KeyUsageEncryptDecrypt {
		return nil, fmt.Errorf(
			"%w: wrapping key %q must have ENCRYPT_DECRYPT usage",
			ErrInvalidKeyUsage,
			wrapKey.KeyID,
		)
	}

	if err = b.validateGrantTokenConstraints(ctx, input.GrantTokens, operation, input.EncryptionContext); err != nil {
		return nil, err
	}

	wrapKM, err := b.requireKeyMaterial(region, wrapKey)
	if err != nil {
		return nil, err
	}

	pairKM, err := generateKeyMaterial(input.KeyPairSpec)
	if err != nil {
		return nil, fmt.Errorf("generating key pair for spec %q: %w", input.KeyPairSpec, err)
	}

	privDER, err := privateKeyPKCS8DER(pairKM)
	if err != nil {
		return nil, err
	}

	pubDER, err := publicKeyDER(pairKM)
	if err != nil {
		return nil, err
	}

	encPriv, err := encryptData(privDER, wrapKey.KeyID, input.EncryptionContext, wrapKM)
	if err != nil {
		return nil, fmt.Errorf("encrypting private key: %w", err)
	}

	b.recordLastUsage(region, wrapKey.KeyID, operation)

	return &GenerateDataKeyPairOutput{
		KeyID:                    wrapKey.Arn,
		KeyPairSpec:              input.KeyPairSpec,
		PrivateKeyCiphertextBlob: encPriv,
		PrivateKeyPlaintext:      privDER,
		PublicKey:                pubDER,
	}, nil
}

// GenerateDataKeyPairWithoutPlaintext generates an asymmetric key pair but omits the plaintext
// private key from the response.
func (b *InMemoryBackend) GenerateDataKeyPairWithoutPlaintext(
	ctx context.Context, input *GenerateDataKeyPairWithoutPlaintextInput,
) (*GenerateDataKeyPairWithoutPlaintextOutput, error) {
	out, err := b.generateDataKeyPair(ctx, &GenerateDataKeyPairInput{
		KeyID:             input.KeyID,
		KeyPairSpec:       input.KeyPairSpec,
		EncryptionContext: input.EncryptionContext,
		GrantTokens:       input.GrantTokens,
	}, "GenerateDataKeyPairWithoutPlaintext")
	if err != nil {
		return nil, err
	}

	return &GenerateDataKeyPairWithoutPlaintextOutput{
		KeyID:                    out.KeyID,
		KeyPairSpec:              out.KeyPairSpec,
		PrivateKeyCiphertextBlob: out.PrivateKeyCiphertextBlob,
		PublicKey:                out.PublicKey,
	}, nil
}
