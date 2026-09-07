package kms

import (
	"context"
	"fmt"
)

// Sign creates a digital signature for the specified message using an asymmetric KMS key.
func (b *InMemoryBackend) Sign(ctx context.Context, input *SignInput) (*SignOutput, error) {
	// Validate message size: AWS limits RAW messages to 4096 bytes.
	msgType := input.MessageType
	if msgType == "" {
		msgType = messageTypeRaw
	}

	// gopherstack-i4q8: Sign does not declare LimitExceededException (unlike
	// CreateKey/CreateGrant, which do and use it for this same "exceeds a
	// length" shape); nothing in Sign's declared set fits. Landmine.
	if msgType == messageTypeRaw && len(input.Message) > maxSignMessageBytes {
		return nil, fmt.Errorf(
			"%w: message must not exceed %d bytes for RAW message type, got %d",
			ErrValidation, maxSignMessageBytes, len(input.Message),
		)
	}

	b.mu.RLock("Sign")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageSignVerify {
		return nil, fmt.Errorf(
			"%w: key %q is not usable for signing",
			ErrInvalidKeyUsage,
			key.KeyID,
		)
	}

	if algErr := validateSigningAlgorithm(input.SigningAlgorithm, key.KeySpec); algErr != nil {
		return nil, algErr
	}

	if err = b.validateGrantTokenPresence(input.GrantTokens, "Sign"); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	messageType := input.MessageType
	if messageType == "" {
		messageType = messageTypeRaw
	}

	sig, signErr := signWithKeyMaterial(input.Message, messageType, input.SigningAlgorithm, km)
	if signErr != nil {
		return nil, signErr
	}

	b.recordLastUsage(region, key.KeyID, "Sign")

	return &SignOutput{
		KeyID:            key.Arn,
		Signature:        sig,
		SigningAlgorithm: input.SigningAlgorithm,
	}, nil
}

// Verify verifies a digital signature using an asymmetric KMS key.
func (b *InMemoryBackend) Verify(ctx context.Context, input *VerifyInput) (*VerifyOutput, error) {
	// Validate message size: AWS limits RAW messages to 4096 bytes.
	msgType := input.MessageType
	if msgType == "" {
		msgType = messageTypeRaw
	}

	// gopherstack-i4q8: same declared-set gap as Sign above -- Verify does not
	// declare LimitExceededException either. Landmine.
	if msgType == messageTypeRaw && len(input.Message) > maxSignMessageBytes {
		return nil, fmt.Errorf(
			"%w: message must not exceed %d bytes for RAW message type, got %d",
			ErrValidation, maxSignMessageBytes, len(input.Message),
		)
	}

	b.mu.RLock("Verify")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrKeyNotFound)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageSignVerify {
		return nil, fmt.Errorf(
			"%w: key %q is not usable for verification",
			ErrInvalidKeyUsage,
			key.KeyID,
		)
	}

	if algErr := validateSigningAlgorithm(input.SigningAlgorithm, key.KeySpec); algErr != nil {
		return nil, algErr
	}

	if err = b.validateGrantTokenPresence(input.GrantTokens, "Verify"); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	messageType := input.MessageType
	if messageType == "" {
		messageType = messageTypeRaw
	}

	valid, verifyErr := verifyWithKeyMaterial(
		input.Message,
		input.Signature,
		messageType,
		input.SigningAlgorithm,
		km,
	)
	if verifyErr != nil {
		return nil, verifyErr
	}

	b.recordLastUsage(region, key.KeyID, "Verify")

	return &VerifyOutput{
		KeyID:            key.Arn,
		SignatureValid:   valid,
		SigningAlgorithm: input.SigningAlgorithm,
	}, nil
}

// GetPublicKey returns the public key for an asymmetric KMS key.
func (b *InMemoryBackend) GetPublicKey(
	ctx context.Context,
	input *GetPublicKeyInput,
) (*GetPublicKeyOutput, error) {
	b.mu.RLock("GetPublicKey")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStateEnabled {
		return nil, keyStateError(key)
	}

	if key.KeyUsage != KeyUsageSignVerify && key.KeyUsage != KeyUsageKeyAgreement &&
		key.KeyUsage != KeyUsageEncryptDecrypt {
		return nil, fmt.Errorf(
			"%w: key %q does not have an asymmetric public key (KeyUsage=%s)",
			ErrInvalidKeyUsage,
			key.KeyID,
			key.KeyUsage,
		)
	}

	// Symmetric keys do not have a public key.
	if key.KeySpec == keySpecSymmetric {
		return nil, fmt.Errorf(
			"%w: key %q is a symmetric key and has no public key",
			ErrInvalidKeyUsage,
			key.KeyID,
		)
	}

	if err = b.validateGrantTokenPresence(input.GrantTokens, "GetPublicKey"); err != nil {
		return nil, err
	}

	km, err := b.requireKeyMaterial(region, key)
	if err != nil {
		return nil, err
	}

	der, pubErr := publicKeyDER(km)
	if pubErr != nil {
		return nil, pubErr
	}

	out := &GetPublicKeyOutput{
		KeyID:     key.Arn,
		PublicKey: der,
		KeySpec:   key.KeySpec,
		KeyUsage:  key.KeyUsage,
	}

	switch key.KeyUsage {
	case KeyUsageSignVerify:
		out.SigningAlgorithms = defaultSigningAlgorithmsForUsage(key.KeySpec, key.KeyUsage)
	case KeyUsageKeyAgreement:
		out.KeyAgreementAlgorithms = keyAgreementAlgorithms(key.KeyUsage)
	case KeyUsageEncryptDecrypt:
		out.EncryptionAlgorithms = []string{algoRSAESOAEPSHA1, encryptionAlgorithmRSAOAEP}
	}

	return out, nil
}
