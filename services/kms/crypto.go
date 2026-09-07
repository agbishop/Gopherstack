package kms

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SHA-1 is required for RSA-OAEP-SHA-1 (AWS compatibility)
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/big"
	"slices"
	"strings"
)

const (
	// rsaBits2048 is the bit size for RSA-2048 key generation.
	rsaBits2048 = 2048
	// rsaBits3072 is the bit size for RSA-3072 key generation.
	rsaBits3072 = 3072
	// rsaBits4096 is the bit size for RSA-4096 key generation.
	rsaBits4096 = 4096
	// keySpecSymmetric is the key spec for symmetric AES-256-GCM keys.
	keySpecSymmetric = "SYMMETRIC_DEFAULT"
	// keySpecRSA2048 is the key spec for RSA-2048 asymmetric keys.
	keySpecRSA2048 = "RSA_2048"
	// keySpecHMAC256 is the key spec for HMAC-SHA-256 keys.
	keySpecHMAC256 = "HMAC_256"
	// keySpecHMAC384 is the key spec for HMAC-SHA-384 keys.
	keySpecHMAC384 = "HMAC_384"
	// keySpecHMAC512 is the key spec for HMAC-SHA-512 keys.
	keySpecHMAC512 = "HMAC_512"
	// hmac256Bytes is the key size in bytes for HMAC-SHA-256.
	hmac256Bytes = 32
	// hmac384Bytes is the key size in bytes for HMAC-SHA-384.
	hmac384Bytes = 48
	// hmac512Bytes is the key size in bytes for HMAC-SHA-512.
	hmac512Bytes = 64
)

var (
	// errUnsupportedKeySpec is returned when an unknown key spec is requested.
	errUnsupportedKeySpec = errors.New("unsupported key spec")
	// errMissingSymmetricKey is returned when symmetric key material is missing.
	errMissingSymmetricKey = errors.New("key material has no symmetric key")
	// errUnsupportedAlgorithm is returned when an unknown signing algorithm is given.
	errUnsupportedAlgorithm = errors.New("unsupported signing algorithm")
	// errUnsupportedHash is returned when an unknown hash algorithm is referenced.
	errUnsupportedHash = errors.New("unsupported hash")
	// errSignatureVerificationFailed is returned when a raw ECDSA verify check fails.
	errSignatureVerificationFailed = errors.New("signature verification failed")
	// errNoAsymmetricKey is returned when no asymmetric key material exists.
	errNoAsymmetricKey = errors.New("no asymmetric key material available")
	// errEmptyKeyMaterial is returned when marshaling empty key material.
	errEmptyKeyMaterial = errors.New("empty key material")
	// errNoKeyMaterialData is returned when deserializing empty serialized material.
	errNoKeyMaterialData = errors.New("no key material to deserialize")
	// errUnsupportedKeyType is returned for unknown private key types.
	errUnsupportedKeyType = errors.New("unsupported private key type")
	// errInvalidMessageType is returned when an unsupported message type is specified.
	errInvalidMessageType = errors.New("invalid message type: must be RAW or DIGEST")
)

// keyMaterial holds the actual cryptographic key bytes or keypairs for a KMS key.
type keyMaterial struct {
	gcm          cipher.AEAD
	rsaKey       *rsa.PrivateKey
	ecKey        *ecdsa.PrivateKey
	symmetricKey []byte
}

// serializedKeyMaterial is the JSON-serializable form of keyMaterial for persistence.
type serializedKeyMaterial struct {
	// SymmetricKey holds raw AES key bytes for symmetric keys.
	SymmetricKey []byte `json:"symmetric_key,omitempty"`
	// PrivKeyDER holds PKCS#8 DER-encoded private key for asymmetric keys.
	PrivKeyDER []byte `json:"priv_key_der,omitempty"`
}

// generateKeyMaterial creates real cryptographic key material for the given key spec.
func generateKeyMaterial(keySpec string) (*keyMaterial, error) {
	switch keySpec {
	case keySpecSymmetric:
		key := make([]byte, aes256Bytes)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, fmt.Errorf("generating AES key: %w", err)
		}

		return newSymmetricKeyMaterial(key)
	case keySpecRSA2048:
		return generateRSAKeyMaterial(rsaBits2048)
	case keySpecRSA3072:
		return generateRSAKeyMaterial(rsaBits3072)
	case keySpecRSA4096:
		return generateRSAKeyMaterial(rsaBits4096)
	case keySpecECCP256:
		return generateECKeyMaterial(elliptic.P256())
	case keySpecECCP384:
		return generateECKeyMaterial(elliptic.P384())
	case keySpecECCP521:
		return generateECKeyMaterial(elliptic.P521())
	case keySpecHMAC256:
		return generateHMACKeyMaterial(hmac256Bytes)
	case keySpecHMAC384:
		return generateHMACKeyMaterial(hmac384Bytes)
	case keySpecHMAC512:
		return generateHMACKeyMaterial(hmac512Bytes)
	default:
		// Wrap with ErrUnsupportedParameter (in addition to errUnsupportedKeySpec) so
		// an unrecognized KeySpec/KeyPairSpec classifies as UnsupportedOperationException
		// (400) rather than falling through classifyKMSError's default
		// KMSInternalException (500). CreateKey, GenerateDataKeyPair(WithoutPlaintext)
		// and key rotation all route unvalidated spec strings here, and all recognize
		// UnsupportedOperationException in their deserializeOpError.
		return nil, fmt.Errorf("%w: %w: %s", ErrUnsupportedParameter, errUnsupportedKeySpec, keySpec)
	}
}

func generateRSAKeyMaterial(bits int) (*keyMaterial, error) {
	k, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("generating RSA-%d key: %w", bits, err)
	}

	return &keyMaterial{rsaKey: k}, nil
}

func generateECKeyMaterial(curve elliptic.Curve) (*keyMaterial, error) {
	k, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ECDSA key: %w", err)
	}

	return &keyMaterial{ecKey: k}, nil
}

func generateHMACKeyMaterial(size int) (*keyMaterial, error) {
	key := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating HMAC key: %w", err)
	}

	return &keyMaterial{symmetricKey: key}, nil
}

func newSymmetricKeyMaterial(key []byte) (*keyMaterial, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	return &keyMaterial{symmetricKey: key, gcm: gcm}, nil
}

// computeHMAC computes an HMAC tag over message using the key material and algorithm.
// Supported algorithms: HMAC_SHA_256, HMAC_SHA_384, HMAC_SHA_512.
func computeHMAC(message []byte, macAlgorithm string, km *keyMaterial) ([]byte, error) {
	if km.symmetricKey == nil {
		return nil, errMissingSymmetricKey
	}

	newHash, ok := hmacHashFor(macAlgorithm)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnsupportedAlgorithm, macAlgorithm)
	}

	h := hmac.New(newHash, km.symmetricKey)
	h.Write(message)

	return h.Sum(nil), nil
}

// hmacHashFor returns the hash constructor for the given HMAC algorithm name.
func hmacHashFor(macAlgorithm string) (func() hash.Hash, bool) {
	switch macAlgorithm {
	case "HMAC_SHA_256":
		return sha256.New, true
	case "HMAC_SHA_384":
		return sha512.New384, true
	case "HMAC_SHA_512":
		return sha512.New, true
	default:
		return nil, false
	}
}

// deriveECDH performs ECDH key agreement using the stored EC private key and the provided DER-encoded
// peer public key (SubjectPublicKeyInfo). Returns the raw shared secret bytes.
func deriveECDH(peerPublicKeyDER []byte, km *keyMaterial) ([]byte, error) {
	if km.ecKey == nil {
		return nil, fmt.Errorf("%w: no EC key available for ECDH", errNoAsymmetricKey)
	}

	peerPub, err := x509.ParsePKIXPublicKey(peerPublicKeyDER)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing peer public key: %w", ErrInvalidKeyUsage, err)
	}

	peerECPub, ok := peerPub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: peer public key is not an EC key", ErrInvalidKeyUsage)
	}

	// Convert ecdsa.PrivateKey to ecdh.PrivateKey.
	privECDH, convErr := km.ecKey.ECDH()
	if convErr != nil {
		return nil, fmt.Errorf("converting EC private key to ECDH: %w", convErr)
	}

	// Convert ecdsa.PublicKey to ecdh.PublicKey.
	peerECDH, convErr := peerECPub.ECDH()
	if convErr != nil {
		return nil, fmt.Errorf("%w: converting peer EC public key to ECDH: %w", ErrInvalidKeyUsage, convErr)
	}

	secret, err := privECDH.ECDH(peerECDH)
	if err != nil {
		return nil, fmt.Errorf("ECDH key agreement: %w", err)
	}

	return secret, nil
}

// privateKeyPKCS8DER returns the PKCS#8 DER-encoded private key from key material.
func privateKeyPKCS8DER(km *keyMaterial) ([]byte, error) {
	var privKey crypto.PrivateKey

	switch {
	case km.rsaKey != nil:
		privKey = km.rsaKey
	case km.ecKey != nil:
		privKey = km.ecKey
	default:
		return nil, errNoAsymmetricKey
	}

	der, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshaling private key to PKCS8: %w", err)
	}

	return der, nil
}

// padKeyID pads or truncates a key ID to exactly keyIDPrefixLen bytes.
func padKeyID(keyID string) []byte {
	b := make([]byte, keyIDPrefixLen)
	copy(b, keyID)

	return b
}

// buildEncryptionContextAAD constructs the additional authenticated data (AAD) bytes from the key ID
// and an optional encryption context. When ctx is empty, the AAD is just the key ID bytes.
// When ctx is non-empty, each sorted key=value pair is appended with a NUL separator,
// producing a deterministic byte sequence required for authenticated decryption.
func buildEncryptionContextAAD(keyID string, ctx map[string]string) []byte {
	if len(ctx) == 0 {
		return []byte(keyID)
	}

	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	var b strings.Builder

	b.WriteString(keyID)

	for _, k := range keys {
		b.WriteByte(0)
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(ctx[k])
	}

	return []byte(b.String())
}

// encryptSymmetric encrypts plaintext with the key's AES-256-GCM material, embedding the key ID.
// The output format is: [keyIDPrefixLen bytes: padded keyID][nonce][AES-GCM ciphertext+tag].
// encCtx is incorporated into the AAD for authenticated encryption; callers must supply the
// same context during decryption or the authentication check will fail.
func encryptSymmetric(plaintext []byte, keyID string, encCtx map[string]string, km *keyMaterial) ([]byte, error) {
	if km.symmetricKey == nil {
		return nil, errMissingSymmetricKey
	}

	gcm, err := symmetricAEAD(km)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, readErr := io.ReadFull(rand.Reader, nonce); readErr != nil {
		return nil, fmt.Errorf("generating nonce: %w", readErr)
	}

	aad := buildEncryptionContextAAD(keyID, encCtx)
	encrypted := gcm.Seal(nonce, nonce, plaintext, aad)

	result := make([]byte, keyIDPrefixLen+len(encrypted))
	copy(result[:keyIDPrefixLen], padKeyID(keyID))
	copy(result[keyIDPrefixLen:], encrypted)

	return result, nil
}

// decryptSymmetric decrypts a ciphertext blob produced by encryptSymmetric.
// encCtx must match the context that was used during encryption; a mismatch causes
// AES-GCM authentication to fail and ErrInvalidCiphertext is returned.
// Returns (plaintext, keyID, error).
func decryptSymmetric(blob []byte, encCtx map[string]string, km *keyMaterial) ([]byte, string, error) {
	if len(blob) < keyIDPrefixLen {
		return nil, "", ErrCiphertextTooShort
	}

	if km.symmetricKey == nil {
		return nil, "", errMissingSymmetricKey
	}

	keyID := strings.TrimRight(string(blob[:keyIDPrefixLen]), "\x00")
	encrypted := blob[keyIDPrefixLen:]

	gcm, err := symmetricAEAD(km)
	if err != nil {
		return nil, "", fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encrypted) < nonceSize {
		return nil, "", ErrCiphertextTooShort
	}

	nonce, cipherOnly := encrypted[:nonceSize], encrypted[nonceSize:]
	aad := buildEncryptionContextAAD(keyID, encCtx)

	plaintext, openErr := gcm.Open(nil, nonce, cipherOnly, aad)
	if openErr != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrInvalidCiphertext, openErr)
	}

	return plaintext, keyID, nil
}

// hashAndAlgorithm returns the hash value for the message and the [crypto.Hash] to use
// based on the signing algorithm. messageType must be "RAW" or "DIGEST".
func hashAndAlgorithm(
	message []byte,
	messageType, signingAlgorithm string,
) ([]byte, crypto.Hash, error) {
	hashAlg, err := signingAlgorithmHash(signingAlgorithm)
	if err != nil {
		return nil, 0, err
	}

	switch messageType {
	case "DIGEST":
		return message, hashAlg, nil
	case "", "RAW":
		digest, hashErr := computeHash(message, hashAlg)
		if hashErr != nil {
			return nil, 0, hashErr
		}

		return digest, hashAlg, nil
	default:
		return nil, 0, fmt.Errorf("%w: %q", errInvalidMessageType, messageType)
	}
}

// signingAlgorithmHash returns the [crypto.Hash] for a signing algorithm string.
func signingAlgorithmHash(signingAlgorithm string) (crypto.Hash, error) {
	switch {
	case strings.HasSuffix(signingAlgorithm, "_SHA_256"):
		return crypto.SHA256, nil
	case strings.HasSuffix(signingAlgorithm, "_SHA_384"):
		return crypto.SHA384, nil
	case strings.HasSuffix(signingAlgorithm, "_SHA_512"):
		return crypto.SHA512, nil
	default:
		return 0, fmt.Errorf("%w: %s", errUnsupportedAlgorithm, signingAlgorithm)
	}
}

// computeHash returns the hash digest of message using the given hash algorithm.
func computeHash(message []byte, h crypto.Hash) ([]byte, error) {
	switch h {
	case crypto.SHA256:
		d := sha256.Sum256(message)

		return d[:], nil
	case crypto.SHA384:
		d := sha512.Sum384(message)

		return d[:], nil
	case crypto.SHA512:
		d := sha512.Sum512(message)

		return d[:], nil
	default:
		return nil, fmt.Errorf("%w: %v", errUnsupportedHash, h)
	}
}

// signWithKeyMaterial signs a message using the key material and specified algorithm.
func signWithKeyMaterial(
	message []byte,
	messageType, signingAlgorithm string,
	km *keyMaterial,
) ([]byte, error) {
	digest, hashAlg, err := hashAndAlgorithm(message, messageType, signingAlgorithm)
	if err != nil {
		return nil, err
	}

	switch {
	case strings.HasPrefix(signingAlgorithm, "RSASSA_PSS_"):
		if km.rsaKey == nil {
			return nil, fmt.Errorf("%w: not an RSA key", ErrInvalidKeyUsage)
		}

		pssOpts := &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
			Hash:       hashAlg,
		}

		return rsa.SignPSS(rand.Reader, km.rsaKey, hashAlg, digest, pssOpts)

	case strings.HasPrefix(signingAlgorithm, "RSASSA_PKCS1_V1_5_"):
		if km.rsaKey == nil {
			return nil, fmt.Errorf("%w: not an RSA key", ErrInvalidKeyUsage)
		}

		return rsa.SignPKCS1v15(rand.Reader, km.rsaKey, hashAlg, digest)

	case strings.HasPrefix(signingAlgorithm, "ECDSA_"):
		if km.ecKey == nil {
			return nil, fmt.Errorf("%w: not an EC key", ErrInvalidKeyUsage)
		}

		return signECDSA(digest, km.ecKey)

	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedAlgorithm, signingAlgorithm)
	}
}

// ecdsaSignature is the ASN.1 structure for DER-encoded ECDSA signatures.
type ecdsaSignature struct {
	R, S *big.Int
}

// signECDSA signs a digest with an ECDSA key and returns a DER-encoded signature.
func signECDSA(digest []byte, key *ecdsa.PrivateKey) ([]byte, error) {
	r, s, err := ecdsa.Sign(rand.Reader, key, digest)
	if err != nil {
		return nil, fmt.Errorf("ECDSA signing: %w", err)
	}

	return asn1.Marshal(ecdsaSignature{R: r, S: s})
}

// verifyWithKeyMaterial verifies a signature using the key material and specified algorithm.
func verifyWithKeyMaterial(
	message, signature []byte,
	messageType, signingAlgorithm string,
	km *keyMaterial,
) (bool, error) {
	digest, hashAlg, err := hashAndAlgorithm(message, messageType, signingAlgorithm)
	if err != nil {
		return false, err
	}

	switch {
	case strings.HasPrefix(signingAlgorithm, "RSASSA_PSS_"):
		if km.rsaKey == nil {
			return false, fmt.Errorf("%w: not an RSA key", ErrInvalidKeyUsage)
		}

		pssOpts := &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
			Hash:       hashAlg,
		}

		err = rsa.VerifyPSS(&km.rsaKey.PublicKey, hashAlg, digest, signature, pssOpts)

	case strings.HasPrefix(signingAlgorithm, "RSASSA_PKCS1_V1_5_"):
		if km.rsaKey == nil {
			return false, fmt.Errorf("%w: not an RSA key", ErrInvalidKeyUsage)
		}

		err = rsa.VerifyPKCS1v15(&km.rsaKey.PublicKey, hashAlg, digest, signature)

	case strings.HasPrefix(signingAlgorithm, "ECDSA_"):
		if km.ecKey == nil {
			return false, fmt.Errorf("%w: not an EC key", ErrInvalidKeyUsage)
		}

		err = verifyECDSA(digest, signature, &km.ecKey.PublicKey)

	default:
		return false, fmt.Errorf("%w: %s", errUnsupportedAlgorithm, signingAlgorithm)
	}

	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrInvalidSignature, err)
	}

	return true, nil
}

// verifyECDSA verifies a DER-encoded ECDSA signature against a digest.
func verifyECDSA(digest, signature []byte, pub *ecdsa.PublicKey) error {
	var sig ecdsaSignature
	if _, err := asn1.Unmarshal(signature, &sig); err != nil {
		return fmt.Errorf("parsing ECDSA signature: %w", err)
	}

	if !ecdsa.Verify(pub, digest, sig.R, sig.S) {
		return errSignatureVerificationFailed
	}

	return nil
}

// publicKeyDER returns the DER-encoded SubjectPublicKeyInfo for the key material's public key.
func publicKeyDER(km *keyMaterial) ([]byte, error) {
	var pub crypto.PublicKey

	switch {
	case km.rsaKey != nil:
		pub = &km.rsaKey.PublicKey
	case km.ecKey != nil:
		pub = &km.ecKey.PublicKey
	default:
		return nil, errNoAsymmetricKey
	}

	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("marshaling public key: %w", err)
	}

	return der, nil
}

// marshalKeyMaterial serializes key material for JSON persistence.
func marshalKeyMaterial(km *keyMaterial) (serializedKeyMaterial, error) {
	if km.symmetricKey != nil {
		return serializedKeyMaterial{SymmetricKey: km.symmetricKey}, nil
	}

	var privKey crypto.PrivateKey

	switch {
	case km.rsaKey != nil:
		privKey = km.rsaKey
	case km.ecKey != nil:
		privKey = km.ecKey
	default:
		return serializedKeyMaterial{}, errEmptyKeyMaterial
	}

	der, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return serializedKeyMaterial{}, fmt.Errorf("marshaling private key: %w", err)
	}

	return serializedKeyMaterial{PrivKeyDER: der}, nil
}

// unmarshalKeyMaterial deserializes key material from a serializedKeyMaterial.
func unmarshalKeyMaterial(s serializedKeyMaterial) (*keyMaterial, error) {
	if len(s.SymmetricKey) > 0 {
		if len(s.SymmetricKey) == aes256Bytes {
			symmetricKM, err := newSymmetricKeyMaterial(s.SymmetricKey)
			if err != nil {
				return nil, err
			}

			return symmetricKM, nil
		}

		return &keyMaterial{symmetricKey: s.SymmetricKey}, nil
	}

	if len(s.PrivKeyDER) == 0 {
		return nil, errNoKeyMaterialData
	}

	priv, err := x509.ParsePKCS8PrivateKey(s.PrivKeyDER)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &keyMaterial{rsaKey: k}, nil
	case *ecdsa.PrivateKey:
		return &keyMaterial{ecKey: k}, nil
	default:
		return nil, fmt.Errorf("%w: %T", errUnsupportedKeyType, priv)
	}
}

func symmetricAEAD(km *keyMaterial) (cipher.AEAD, error) {
	if km.gcm != nil {
		return km.gcm, nil
	}

	block, err := aes.NewCipher(km.symmetricKey)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	return gcm, nil
}

// defaultSigningAlgorithms returns the supported signing algorithms for a key spec.
func defaultSigningAlgorithms(keySpec string) []string {
	switch keySpec {
	case keySpecRSA2048, keySpecRSA3072, keySpecRSA4096:
		return []string{
			"RSASSA_PSS_SHA_256",
			"RSASSA_PSS_SHA_384",
			"RSASSA_PSS_SHA_512",
			"RSASSA_PKCS1_V1_5_SHA_256",
			"RSASSA_PKCS1_V1_5_SHA_384",
			"RSASSA_PKCS1_V1_5_SHA_512",
		}
	case keySpecECCP256:
		return []string{"ECDSA_SHA_256"}
	case keySpecECCP384:
		return []string{"ECDSA_SHA_384"}
	case keySpecECCP521:
		return []string{"ECDSA_SHA_512"}
	default:
		return nil
	}
}

// defaultMacAlgorithms returns the HMAC algorithms supported by a key spec.
func defaultMacAlgorithms(keySpec string) []string {
	switch keySpec {
	case keySpecHMAC256:
		return []string{"HMAC_SHA_256"}
	case keySpecHMAC384:
		return []string{"HMAC_SHA_384"}
	case keySpecHMAC512:
		return []string{"HMAC_SHA_512"}
	default:
		return nil
	}
}

// keyAgreementAlgorithms returns the key agreement algorithms supported by a key usage.
func keyAgreementAlgorithms(keyUsage string) []string {
	if keyUsage == KeyUsageKeyAgreement {
		return []string{algoECDH}
	}

	return nil
}

// defaultSigningAlgorithmsForUsage returns signing algorithms for the given key spec and usage.
// For KEY_AGREEMENT keys, signing algorithms are not applicable and nil is returned.
func defaultSigningAlgorithmsForUsage(keySpec, keyUsage string) []string {
	if keyUsage == KeyUsageKeyAgreement {
		return nil
	}

	return defaultSigningAlgorithms(keySpec)
}

// encryptRSAOAEP encrypts plaintext using RSA-OAEP-SHA-256 with the given key material.
func encryptRSAOAEP(plaintext []byte, km *keyMaterial) ([]byte, error) {
	if km.rsaKey == nil {
		return nil, fmt.Errorf("%w: not an RSA key", ErrInvalidKeyUsage)
	}

	return rsa.EncryptOAEP(sha256.New(), rand.Reader, &km.rsaKey.PublicKey, plaintext, nil)
}

// decryptRSAOAEP decrypts ciphertext using RSA-OAEP-SHA-256 with the given key material.
// It first tries SHA-256 (primary) then SHA-1 for backward compatibility with RSAES_OAEP_SHA_1 blobs.
func decryptRSAOAEP(ciphertext []byte, km *keyMaterial) ([]byte, error) {
	if km.rsaKey == nil {
		return nil, fmt.Errorf("%w: not an RSA key", ErrInvalidKeyUsage)
	}

	// Try SHA-256 first (RSAES_OAEP_SHA_256).
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, km.rsaKey, ciphertext, nil)
	if err == nil {
		return plaintext, nil
	}

	// Fall back to SHA-1 (RSAES_OAEP_SHA_1) for AWS compatibility.
	plaintext, err = tryDecryptRSAOAEPSHA1(ciphertext, km)
	if err != nil {
		return nil, fmt.Errorf("%w: RSA-OAEP decryption failed", ErrInvalidCiphertext)
	}

	return plaintext, nil
}

// tryDecryptRSAOAEPSHA1 attempts RSA-OAEP decryption with SHA-1 hash for RSAES_OAEP_SHA_1 blobs.
// SHA-1 is used here solely for AWS SDK compatibility; it is not used for hashing security-sensitive data.
//
//nolint:gosec // SHA-1 is required for AWS RSAES_OAEP_SHA_1 compatibility, not for security hashing
func tryDecryptRSAOAEPSHA1(ciphertext []byte, km *keyMaterial) ([]byte, error) {
	return rsa.DecryptOAEP(sha1.New(), rand.Reader, km.rsaKey, ciphertext, nil)
}

// hmacEqual is a constant-time comparison of two HMAC byte slices.
func hmacEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}

// validateSigningAlgorithm returns an error if signingAlgorithm is not in the set of
// algorithms supported by keySpec, preventing key-spec/algorithm mismatches.
func validateSigningAlgorithm(signingAlgorithm, keySpec string) error {
	supported := defaultSigningAlgorithms(keySpec)
	if slices.Contains(supported, signingAlgorithm) {
		return nil
	}

	return fmt.Errorf(
		"%w: signing algorithm %q is not supported for key spec %q; supported: %v",
		errUnsupportedAlgorithm, signingAlgorithm, keySpec, supported,
	)
}

// validateEncryptionContextSize enforces the AWS KMS 4096-byte cap on the
// canonical encoded EncryptionContext. The encoded form is the same one used
// by buildEncryptionContextAAD: sorted "key=value" pairs separated by NUL
// bytes (we omit the leading keyID and treat keyID separator weight as 1 byte
// per pair, matching the AAD shape minus the prefix).
// gopherstack-i4q8: reached by GenerateDataKey(WithoutPlaintext) (via
// validateGenerateDataKeyInput below), GenerateDataKeyPair(WithoutPlaintext),
// Encrypt, Decrypt and ReEncrypt (via validateReEncryptInput below) --
// none of their declared sets has an EncryptionContext-size code. Landmine.
func validateEncryptionContextSize(ctx map[string]string) error {
	if len(ctx) == 0 {
		return nil
	}

	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	total := 0

	for _, k := range keys {
		// 1 separator + key + '=' + value
		total += 1 + len(k) + 1 + len(ctx[k])
		if total > maxEncryptionContextBytes {
			return fmt.Errorf(
				"%w: encoded EncryptionContext exceeds %d bytes",
				ErrValidation, maxEncryptionContextBytes,
			)
		}
	}

	return nil
}

// validateGenerateDataKeyInput enforces the same shape rules as the AWS KMS API.
// gopherstack-i4q8: reached by both GenerateDataKey and
// GenerateDataKeyWithoutPlaintext, whose declared sets are identical and
// contain nothing for either check below. Landmine.
func validateGenerateDataKeyInput(input *GenerateDataKeyInput) error {
	if input.KeySpec != "" && input.NumberOfBytes != nil {
		return fmt.Errorf(
			"%w: specify either KeySpec or NumberOfBytes, not both",
			ErrValidation,
		)
	}

	if input.KeySpec != "" && input.KeySpec != "AES_128" && input.KeySpec != "AES_256" {
		return fmt.Errorf(
			"%w: KeySpec for GenerateDataKey must be AES_128 or AES_256, got %q",
			ErrValidation, input.KeySpec,
		)
	}

	return validateEncryptionContextSize(input.EncryptionContext)
}

// validateMacAlgorithm returns an error if macAlgorithm is not supported by the given keySpec.
func validateMacAlgorithm(macAlgorithm, keySpec string) error {
	supported := defaultMacAlgorithms(keySpec)
	if slices.Contains(supported, macAlgorithm) {
		return nil
	}

	return fmt.Errorf(
		"%w: MAC algorithm %q is not supported for key spec %q; supported: %v",
		ErrInvalidKeyUsage, macAlgorithm, keySpec, supported,
	)
}

// validateReEncryptInput enforces non-empty destination key and encryption context size limits.
// gopherstack-i4q8: reached only by ReEncrypt, which declares nothing for a
// missing DestinationKeyId. Landmine.
func validateReEncryptInput(input *ReEncryptInput) error {
	if strings.TrimSpace(input.DestinationKeyID) == "" {
		return fmt.Errorf("%w: DestinationKeyId must not be empty", ErrValidation)
	}

	if err := validateEncryptionContextSize(input.SourceEncryptionContext); err != nil {
		return err
	}

	return validateEncryptionContextSize(input.DestinationEncryptionContext)
}
