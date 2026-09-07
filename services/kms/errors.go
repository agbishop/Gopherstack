package kms

import (
	"errors"
)

var (
	// ErrKeyNotFound is returned when the specified key does not exist.
	ErrKeyNotFound = errors.New("NotFoundException")

	// ErrMalformedPolicyDocument is returned when the provided policy is invalid.
	ErrMalformedPolicyDocument = errors.New("MalformedPolicyDocumentException")
	// ErrAliasNotFound is returned when the specified alias does not exist.
	ErrAliasNotFound = errors.New("NotFoundException")
	// ErrAliasAlreadyExists is returned when an alias with the given name already exists.
	ErrAliasAlreadyExists = errors.New("AlreadyExistsException")
	// ErrInvalidAliasName is returned by CreateAlias when AliasName doesn't start with
	// "alias/", is reserved ("alias/aws/"), exceeds the length limit, or contains
	// disallowed characters. CreateAlias's own deserializeOpError (kms@v1.55.4
	// deserializers.go) recognizes InvalidAliasNameException for exactly this.
	ErrInvalidAliasName = errors.New("InvalidAliasNameException")
	// ErrCustomKeyStoreAlreadyExists is returned when a custom key store with the given name already exists.
	ErrCustomKeyStoreAlreadyExists = errors.New("CustomKeyStoreNameInUseException")
	// ErrCustomKeyStoreNotFound is returned when a custom key store ID does not exist.
	ErrCustomKeyStoreNotFound = errors.New("CustomKeyStoreNotFoundException")
	// ErrCustomKeyStoreInvalidState is returned by CreateKey when the target custom
	// key store's ConnectionState is not CONNECTED, and by Connect/Disconnect/
	// DeleteCustomKeyStore for their own state preconditions (UpdateCustomKeyStore
	// has no ConnectionState guard in this backend; see PARITY.md gopherstack-akm2).
	// CreateKey's own deserializeOpError (kms@v1.55.4 deserializers.go) recognizes
	// CustomKeyStoreInvalidStateException for exactly this.
	ErrCustomKeyStoreInvalidState = errors.New("CustomKeyStoreInvalidStateException")
	// ErrCustomKeyStoreHasKeys is returned by DeleteCustomKeyStore when the store
	// still contains KMS keys. DeleteCustomKeyStore's own deserializeOpError
	// recognizes CustomKeyStoreHasCMKsException for exactly this ("The custom key
	// store that you delete cannot contain any KMS keys").
	ErrCustomKeyStoreHasKeys = errors.New("CustomKeyStoreHasCMKsException")
	// ErrKeyDisabled is returned when an operation is attempted on a disabled key.
	ErrKeyDisabled = errors.New("DisabledException")
	// ErrKeyInvalidState is returned when a key is in a state that does not allow the requested
	// operation (e.g. PendingDeletion).
	ErrKeyInvalidState = errors.New("KMSInvalidStateException")
	// ErrInvalidKeyUsage is returned when the key is used for an operation incompatible with its
	// KeyUsage (e.g. encrypting with a SIGN_VERIFY key).
	ErrInvalidKeyUsage = errors.New("InvalidKeyUsageException")
	// ErrInvalidCiphertext is returned when the ciphertext cannot be decrypted.
	ErrInvalidCiphertext = errors.New("InvalidCiphertextException")
	// ErrIncorrectKey is returned when the KMS key identified by a caller-supplied KeyId
	// (Decrypt) or SourceKeyId (ReEncrypt) is not the key that encrypted the ciphertext.
	ErrIncorrectKey = errors.New("IncorrectKeyException")
	// ErrIncorrectKeyMaterial is returned by ImportKeyMaterial when the supplied key
	// material does not meet expectations (e.g. wrong length for the target key spec).
	// IncorrectKeyMaterialException's doc (kms@v1.55.4 types/errors.go): "the key
	// material in the request is, expired, invalid, or does not meet expectations".
	ErrIncorrectKeyMaterial = errors.New("IncorrectKeyMaterialException")
	// ErrGrantNotFound is returned when the specified grant does not exist.
	ErrGrantNotFound = errors.New("NotFoundException: grant not found")
	// ErrCiphertextTooShort is returned when the ciphertext is too short.
	ErrCiphertextTooShort = errors.New("ciphertext too short")
	// ErrInvalidDataKeySize is returned when a data key size is invalid or too large.
	ErrInvalidDataKeySize = errors.New("ValidationException: invalid data key size")
	// ErrInvalidSignature is returned when a signature verification fails.
	ErrInvalidSignature = errors.New("KMSInvalidSignatureException")
	// ErrInvalidMac is returned when a VerifyMac HMAC comparison fails. Distinct from
	// ErrInvalidSignature: VerifyMac's own deserializeOpError (kms@v1.55.4
	// deserializers.go) recognizes KMSInvalidMacException, not KMSInvalidSignatureException
	// (that code belongs to Verify only).
	ErrInvalidMac = errors.New("KMSInvalidMacException")
	// ErrKeyMaterialUnavailable is returned when key material is missing (e.g. restored from
	// an older snapshot that predates key material persistence).
	ErrKeyMaterialUnavailable = errors.New("key material unavailable for this key")
	// ErrUnsupportedOrigin is returned when an operation is incompatible with the key's origin.
	ErrUnsupportedOrigin = errors.New("UnsupportedOperationException")
	// ErrValidation is returned for invalid request parameters (maps to ValidationException).
	// "ValidationException" names no per-operation typed exception in kms@v1.55.4 (not in
	// types/errors.go, not in any op's deserializeOpError, and absent from the full
	// api-2.json model too) -- it is real for KMS at the pre-dispatch, protocol level
	// regardless: KMS's own GetPublicKey doc quotes a live wire ValidationException for a
	// malformed PublicKey that no op declares, and deserializeOpError's default case
	// preserves an unmodeled wire code rather than rejecting it. Settled by
	// gopherstack-q9bs; the sites below using it are correct, not landmines.
	ErrValidation = errors.New("ValidationException")
	// ErrExpiredKeyMaterial is returned when a key's imported material has passed its ValidTo date.
	ErrExpiredKeyMaterial = errors.New("ExpiredImportTokenException")
	// ErrInvalidGrantToken is returned when a grant token is expired or malformed.
	ErrInvalidGrantToken = errors.New("InvalidGrantTokenException")
	// ErrLimitExceeded is returned when a service limit is exceeded (e.g. grants per key)
	// or, per LimitExceededException's own doc ("a length constraint or quota was
	// exceeded"), a length constraint -- CreateKey's Description and CreateGrant's
	// Name length checks reuse it for exactly that (gopherstack-i4q8).
	ErrLimitExceeded = errors.New("LimitExceededException")
	// ErrAccessDenied is returned when a grant token is valid but its Operations list
	// does not authorize the operation being performed.
	ErrAccessDenied = errors.New("AccessDeniedException")
	// ErrInvalidTag is returned when a tag key/value fails KMS's format constraints
	// (empty key, length limit, reserved "aws:" prefix). TagResource, CreateKey and
	// ReplicateKey's deserializeOpError all recognize TagException for this.
	ErrInvalidTag = errors.New("TagException")
	// ErrUnsupportedParameter is returned when a KeySpec/KeyPairSpec/WrappingAlgorithm/
	// WrappingKeySpec/PolicyName value is not one this operation supports. CreateKey,
	// GenerateDataKeyPair(WithoutPlaintext), GetParametersForImport, the rotation
	// ops and PutKeyPolicy all recognize UnsupportedOperationException for an
	// unsupported parameter value, per its doc ("a specified parameter is not
	// supported") -- gopherstack-i4q8 added the PutKeyPolicy reuse.
	ErrUnsupportedParameter = errors.New("UnsupportedOperationException")
	// ErrInvalidImportToken is returned when ImportKeyMaterial's wrapped key material
	// cannot be unwrapped because no GetParametersForImport wrapping key is on record
	// for the target KMS key (stale or skipped GetParametersForImport call).
	// ImportKeyMaterial's deserializeOpError recognizes InvalidImportTokenException.
	ErrInvalidImportToken = errors.New("InvalidImportTokenException")
	// ErrInvalidArn is returned by resolveKeyID/resolveARNKeyID for a malformed KeyId
	// ARN, for the KeyId-accepting operations whose own deserializeOpError recognizes
	// InvalidArnException (gopherstack-qxaj). Crypto ops (Encrypt, Decrypt, Sign,
	// GenerateDataKey, ...) do not model it -- those callers pass ErrKeyNotFound
	// instead, the only resource-shaped code they do recognize.
	ErrInvalidArn = errors.New("InvalidArnException")
)
