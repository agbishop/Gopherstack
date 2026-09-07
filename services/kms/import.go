package kms

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"io"
	"time"
)

// GetParametersForImport returns wrapping parameters for EXTERNAL-origin key material import.
// Returns a real RSA public key (DER-encoded SubjectPublicKeyInfo) that callers can use to
// RSA-OAEP-wrap their key material before calling ImportKeyMaterial.
func (b *InMemoryBackend) GetParametersForImport(
	ctx context.Context, input *GetParametersForImportInput,
) (*GetParametersForImportOutput, error) {
	validWrappingAlgorithms := map[string]struct{}{
		"RSAES_PKCS1_V1_5":         {},
		"RSAES_OAEP_SHA_1":         {},
		encryptionAlgorithmRSAOAEP: {},
		"RSA_AES_KEY_WRAP_SHA_1":   {},
		"RSA_AES_KEY_WRAP_SHA_256": {},
	}

	if input.WrappingAlgorithm != "" {
		if _, ok := validWrappingAlgorithms[input.WrappingAlgorithm]; !ok {
			return nil, fmt.Errorf(
				"%w: WrappingAlgorithm %q is not valid; must be one of RSAES_PKCS1_V1_5, "+
					"RSAES_OAEP_SHA_1, RSAES_OAEP_SHA_256, RSA_AES_KEY_WRAP_SHA_1, or RSA_AES_KEY_WRAP_SHA_256",
				ErrUnsupportedParameter,
				input.WrappingAlgorithm,
			)
		}
	}

	validWrappingKeySpecs := map[string]struct{}{
		"RSA_2048": {},
		"RSA_3072": {},
		"RSA_4096": {},
	}

	if input.WrappingKeySpec != "" {
		if _, ok := validWrappingKeySpecs[input.WrappingKeySpec]; !ok {
			return nil, fmt.Errorf(
				"%w: WrappingKeySpec %q is not valid; must be RSA_2048, RSA_3072, or RSA_4096",
				ErrUnsupportedParameter, input.WrappingKeySpec,
			)
		}
	}

	// Generate import token and RSA key pair BEFORE acquiring the lock.
	importToken := make([]byte, aes256Bytes)
	if _, readErr := io.ReadFull(rand.Reader, importToken); readErr != nil {
		return nil, fmt.Errorf("generating import token: %w", readErr)
	}

	rsaBits := rsaBits2048
	switch input.WrappingKeySpec {
	case "RSA_3072":
		rsaBits = rsaBits3072
	case "RSA_4096":
		rsaBits = rsaBits4096
	}

	privKey, genErr := rsa.GenerateKey(rand.Reader, rsaBits)
	if genErr != nil {
		return nil, fmt.Errorf("generating wrapping RSA key: %w", genErr)
	}

	pubKeyDER, marshalErr := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshaling wrapping public key: %w", marshalErr)
	}

	b.mu.RLock("GetParametersForImport")
	defer b.mu.RUnlock()

	key, err := b.lookupKey(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	if key.Origin != KeyOriginExternal {
		return nil, fmt.Errorf(
			"%w: GetParametersForImport is only valid for keys with Origin=%s",
			ErrUnsupportedOrigin,
			KeyOriginExternal,
		)
	}

	// Store private key (via sync.Map, no write lock needed) so ImportKeyMaterial
	// can unwrap RSA-OAEP-encrypted material from this caller.
	b.importWrappingKeys.Store(key.KeyID, privKey)

	return &GetParametersForImportOutput{
		KeyID:             key.KeyID,
		ImportToken:       importToken,
		PublicKey:         pubKeyDER,
		ParametersValidTo: UnixTimeFloat(time.Now().Add(getParametersValidityWindow)),
	}, nil
}

// resolveExpirationModel normalises the (expirationModel, validTo) pair from an
// ImportKeyMaterial request and returns the validated expiration model and ValidTo.
// gopherstack-i4q8: ImportKeyMaterial declares UnsupportedOperationException,
// but its doc ("a specified parameter is not supported") covers an unsupported
// VALUE for a parameter (as used for KeySpec elsewhere), not this
// ExpirationModel/ValidTo cross-field consistency rule -- rejected as a
// name-similarity trap, not a fit. Nothing else fits either. Landmine.
func resolveExpirationModel(expModel string, validTo float64) (string, float64, error) {
	if expModel == "" {
		if validTo > 0 {
			expModel = expirationModelExpires
		} else {
			expModel = expirationModelNoExpiry
		}
	}

	if expModel == expirationModelExpires && validTo == 0 {
		return "", 0, fmt.Errorf(
			"%w: ExpirationModel=%s requires ValidTo to be set",
			ErrValidation, expirationModelExpires,
		)
	}

	if expModel == expirationModelNoExpiry && validTo > 0 {
		return "", 0, fmt.Errorf(
			"%w: ExpirationModel=%s must not include ValidTo",
			ErrValidation, expirationModelNoExpiry,
		)
	}

	return expModel, validTo, nil
}

// resolveKeyMaterial detects whether material is RSA-OAEP-wrapped (≥ minRSAWrappedMaterialBytes)
// and decrypts it using the stored wrapping key, or returns it unchanged (raw AES-256 path).
func (b *InMemoryBackend) resolveKeyMaterial(keyID string, material []byte) ([]byte, error) {
	if len(material) < minRSAWrappedMaterialBytes {
		return material, nil
	}

	privKeyAny, loaded := b.importWrappingKeys.Load(keyID)
	if !loaded {
		return nil, fmt.Errorf(
			"%w: no wrapping key found for %s; call GetParametersForImport first",
			ErrInvalidImportToken, keyID,
		)
	}

	privKey, ok := privKeyAny.(*rsa.PrivateKey)
	if !ok {
		// gopherstack-i4q8: defensive only -- importWrappingKeys only ever
		// stores *rsa.PrivateKey (see GetParametersForImport), so no request
		// can hit this. Not a real per-op validation, so there's no op-declared
		// code to pick between; landmine.
		return nil, fmt.Errorf("%w: internal: wrapping key type assertion failed", ErrValidation)
	}

	raw, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, material, nil)
	if err != nil {
		// InvalidCiphertextException's doc (kms@v1.55.4 types/errors.go) covers this
		// exactly for ImportKeyMaterial: "KMS could not decrypt the encrypted (wrapped)
		// key material."
		return nil, fmt.Errorf("%w: RSA-OAEP decrypt of key material failed: %w", ErrInvalidCiphertext, err)
	}

	b.importWrappingKeys.Delete(keyID)

	return raw, nil
}

// ImportKeyMaterial imports externally supplied key material into a key created with
// Origin=EXTERNAL. The key must be in PendingImport state. On success the key transitions
// to Enabled. Only SYMMETRIC_DEFAULT keys are supported; asymmetric EXTERNAL keys are
// not modeled by this mock.
func (b *InMemoryBackend) ImportKeyMaterial(
	ctx context.Context,
	input *ImportKeyMaterialInput,
) error {
	b.mu.Lock("ImportKeyMaterial")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	if key.Origin != KeyOriginExternal {
		return fmt.Errorf(
			"%w: ImportKeyMaterial is only valid for keys with Origin=%s",
			ErrUnsupportedOrigin,
			KeyOriginExternal,
		)
	}

	// Only allow import when the key is awaiting material.
	if key.KeyState != KeyStatePendingImport {
		return fmt.Errorf("%w: key %q is not awaiting key material", ErrKeyInvalidState, key.KeyID)
	}

	// Only symmetric (AES-256) key material is supported for external import.
	// UnsupportedOperationException's doc covers this: "a specified ... resource is
	// not valid for this operation" -- the key's own KeySpec, not a request parameter.
	if key.KeySpec != keySpecSymmetric {
		return fmt.Errorf(
			"%w: ImportKeyMaterial only supports SYMMETRIC_DEFAULT keys; got %s",
			ErrUnsupportedParameter, key.KeySpec,
		)
	}

	// IncorrectKeyMaterialException's own doc disjunction -- "is, expired, invalid, or
	// does not meet expectations" -- covers empty material under "invalid", same
	// declared code as the wrong-length check below under "does not meet expectations".
	if len(input.KeyMaterial) == 0 {
		return fmt.Errorf("%w: KeyMaterial must not be empty", ErrIncorrectKeyMaterial)
	}

	rawMaterial, err := b.resolveKeyMaterial(key.KeyID, input.KeyMaterial)
	if err != nil {
		return err
	}

	if len(rawMaterial) != aes256Bytes {
		return fmt.Errorf(
			"%w: symmetric key material must be exactly %d bytes, got %d",
			ErrIncorrectKeyMaterial, aes256Bytes, len(rawMaterial),
		)
	}

	// Copy the material bytes so the caller cannot mutate the key's internal state.
	mat := make([]byte, aes256Bytes)
	copy(mat, rawMaterial)

	km, kmErr := newSymmetricKeyMaterial(mat)
	if kmErr != nil {
		return fmt.Errorf("creating imported symmetric key material: %w", kmErr)
	}

	b.keyMaterialsStore(region)[key.KeyID] = km
	key.KeyState = KeyStateEnabled
	key.Enabled = true

	expModel, validTo, err := resolveExpirationModel(input.ExpirationModel, input.ValidTo)
	if err != nil {
		return err
	}

	key.ValidTo = validTo
	key.ExpirationModel = expModel

	return nil
}

// DeleteImportedKeyMaterial removes the imported key material from an EXTERNAL-origin key.
// The key transitions to PendingImport; it can receive new material via ImportKeyMaterial.
func (b *InMemoryBackend) DeleteImportedKeyMaterial(
	ctx context.Context,
	input *DeleteImportedKeyMaterialInput,
) error {
	b.mu.Lock("DeleteImportedKeyMaterial")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	if key.Origin != KeyOriginExternal {
		return fmt.Errorf(
			"%w: DeleteImportedKeyMaterial is only valid for keys with Origin=%s",
			ErrUnsupportedOrigin,
			KeyOriginExternal,
		)
	}

	delete(b.keyMaterialsStore(region), key.KeyID)
	delete(b.keyMaterialHistoryStore(region), key.KeyID)
	key.KeyState = KeyStatePendingImport
	key.Enabled = false
	key.ValidTo = 0
	key.ExpirationModel = ""

	return nil
}
