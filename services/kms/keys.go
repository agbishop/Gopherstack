package kms

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/google/uuid"

	gopherarn "github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// validateKeySpecUsage returns an error when keySpec and keyUsage are incompatible.
// Symmetric specs (SYMMETRIC_DEFAULT) are only valid for ENCRYPT_DECRYPT;
// RSA specs (RSA_*) are valid for SIGN_VERIFY or ENCRYPT_DECRYPT (RSA-OAEP);
// ECC specs (ECC_*) are valid for SIGN_VERIFY or KEY_AGREEMENT;
// HMAC specs (HMAC_*) are only valid for GENERATE_VERIFY_MAC.
func validateKeySpecUsage(keySpec, keyUsage string) error {
	switch keySpec {
	case keySpecSymmetric:
		if keyUsage != "" && keyUsage != KeyUsageEncryptDecrypt {
			return fmt.Errorf(
				"%w: key spec %q is not compatible with key usage %q; symmetric keys require ENCRYPT_DECRYPT",
				ErrInvalidKeyUsage,
				keySpec,
				keyUsage,
			)
		}
	case keySpecRSA2048, keySpecRSA3072, keySpecRSA4096:
		if keyUsage != "" && keyUsage != KeyUsageSignVerify && keyUsage != KeyUsageEncryptDecrypt {
			return fmt.Errorf(
				"%w: key spec %q supports KeyUsage=%s or KeyUsage=%s only",
				ErrInvalidKeyUsage, keySpec, KeyUsageSignVerify, KeyUsageEncryptDecrypt,
			)
		}
	case keySpecECCP256, keySpecECCP384, keySpecECCP521:
		if keyUsage != "" && keyUsage != KeyUsageSignVerify && keyUsage != KeyUsageKeyAgreement {
			return fmt.Errorf(
				"%w: key spec %q is not compatible with key usage %q; ECC keys require SIGN_VERIFY or KEY_AGREEMENT",
				ErrInvalidKeyUsage,
				keySpec,
				keyUsage,
			)
		}
	case keySpecHMAC256, keySpecHMAC384, keySpecHMAC512:
		if keyUsage != "" && keyUsage != KeyUsageGenerateMac {
			return fmt.Errorf(
				"%w: key spec %q is not compatible with key usage %q; HMAC keys require GENERATE_VERIFY_MAC",
				ErrInvalidKeyUsage,
				keySpec,
				keyUsage,
			)
		}
	}

	return nil
}

// validateCustomKeyStoreLink checks a CreateKeyInput.CustomKeyStoreId reference against its
// doc comment (aws-sdk-go-v2/service/kms@v1.55.4 api_op_CreateKey.go:207): the store must
// exist and be CONNECTED, and it is valid only for single-Region symmetric encryption KMS
// keys. External key stores need XksKeyId, which gopherstack does not implement (see
// PARITY.md).
func (b *InMemoryBackend) validateCustomKeyStoreLink(
	region, storeID, keySpec, keyUsage string, multiRegion bool,
) error {
	if storeID == "" {
		return nil
	}

	ks, ok := b.customKeyStoresStore(region).Get(storeID)
	if !ok {
		return fmt.Errorf("%w: custom key store %q not found", ErrCustomKeyStoreNotFound, storeID)
	}

	if ks.ConnectionState != ConnectionStateConnected {
		return fmt.Errorf(
			"%w: custom key store %q is not connected (state: %s)",
			ErrCustomKeyStoreInvalidState, storeID, ks.ConnectionState,
		)
	}

	if ks.CustomKeyStoreType == "EXTERNAL_KEY_STORE" {
		return fmt.Errorf(
			"%w: creating a KMS key in an external key store requires XksKeyId, which gopherstack does not implement",
			ErrUnsupportedParameter,
		)
	}

	if keySpec != keySpecSymmetric || keyUsage != KeyUsageEncryptDecrypt || multiRegion {
		return fmt.Errorf(
			"%w: custom key stores support only single-Region symmetric encryption KMS keys",
			ErrUnsupportedParameter,
		)
	}

	return nil
}

// deriveKeySpecUsage fills in missing KeySpec and KeyUsage defaults, returning the resolved pair.
// If keyUsage is empty, it is inferred from keySpec; if keySpec is empty it is inferred from keyUsage.
func deriveKeySpecUsage(keySpec, keyUsage string) (string, string) {
	if keyUsage == "" {
		switch keySpec {
		case keySpecSymmetric, "":
			keyUsage = KeyUsageEncryptDecrypt
		case keySpecHMAC256, keySpecHMAC384, keySpecHMAC512:
			keyUsage = KeyUsageGenerateMac
		default:
			// RSA and ECC specs default to SIGN_VERIFY unless the caller specified otherwise.
			keyUsage = KeyUsageSignVerify
		}
	}

	if keySpec == "" {
		switch keyUsage {
		case KeyUsageEncryptDecrypt:
			keySpec = keySpecSymmetric
		case KeyUsageGenerateMac:
			keySpec = keySpecHMAC256
		case KeyUsageSignVerify:
			keySpec = keySpecRSA2048
		case KeyUsageKeyAgreement:
			keySpec = keySpecECCP256
		default:
			keySpec = keySpecSymmetric
		}
	}

	return keySpec, keyUsage
}

// validateCreateKeyLimits checks CreateKeyInput's Description and Tags length limits.
func validateCreateKeyLimits(input *CreateKeyInput) error {
	if len(input.Description) > maxDescriptionLength {
		return fmt.Errorf(
			"%w: Description exceeds maximum length of %d characters",
			ErrValidation, maxDescriptionLength,
		)
	}

	if len(input.Tags) > maxTagsPerKey {
		return fmt.Errorf(
			"%w: number of tags (%d) exceeds the maximum of %d",
			ErrLimitExceeded, len(input.Tags), maxTagsPerKey,
		)
	}

	return nil
}

// CreateKey creates a new KMS key and stores it in the backend.
func (b *InMemoryBackend) CreateKey(
	ctx context.Context,
	input *CreateKeyInput,
) (*CreateKeyOutput, error) {
	if err := validateCreateKeyLimits(input); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateKey")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)
	if input.Region != "" {
		region = input.Region
	}

	// An inline policy, if supplied, must be a well-formed key policy document.
	if input.Policy != "" && !validKeyPolicyDoc(input.Policy) {
		return nil, ErrMalformedPolicyDocument
	}

	keyID := uuid.New().String()
	keyUsage := input.KeyUsage
	keySpec := input.KeySpec

	// Validate that KeySpec and KeyUsage are compatible when both are specified.
	if err := validateKeySpecUsage(keySpec, keyUsage); err != nil {
		return nil, err
	}

	keySpec, keyUsage = deriveKeySpecUsage(keySpec, keyUsage)

	// HMAC keys do not support MultiRegion.
	if input.MultiRegion {
		switch keySpec {
		case keySpecHMAC256, keySpecHMAC384, keySpecHMAC512:
			return nil, fmt.Errorf(
				"%w: HMAC keys (spec %q) do not support MultiRegion",
				ErrInvalidKeyUsage, keySpec,
			)
		}
	}

	if err := b.validateCustomKeyStoreLink(
		region, input.CustomKeyStoreID, keySpec, keyUsage, input.MultiRegion,
	); err != nil {
		return nil, err
	}

	// Resolve origin: EXTERNAL keys require the caller to import key material later.
	origin := input.Origin
	if origin == "" {
		origin = KeyOriginAWSKMS
	}

	keyARN := gopherarn.Build("kms", region, b.accountID, "key/"+keyID)

	// External-origin keys start in PendingImport; no key material is generated.
	keyState := KeyStateEnabled
	if origin == KeyOriginExternal {
		keyState = KeyStatePendingImport
	}

	key := &Key{
		KeyID:            keyID,
		Arn:              keyARN,
		Description:      input.Description,
		KeyState:         keyState,
		KeyUsage:         keyUsage,
		KeySpec:          keySpec,
		Origin:           origin,
		PrimaryRegion:    region,
		CustomKeyStoreID: input.CustomKeyStoreID,
		CreationDate:     UnixTimeFloat(time.Now()),
		Enabled:          keyState == KeyStateEnabled,
		MultiRegion:      input.MultiRegion,
	}

	if origin != KeyOriginExternal {
		km, err := generateKeyMaterial(keySpec)
		if err != nil {
			return nil, fmt.Errorf("generating key material for spec %q: %w", keySpec, err)
		}

		b.keyMaterialsStore(region)[keyID] = km
	}

	b.keysStore(region).Put(key)

	// Persist the caller-supplied policy so GetKeyPolicy returns it verbatim
	// rather than synthesizing the default (Terraform's aws_kms_key polls
	// GetKeyPolicy after create until the configured policy propagates).
	if input.Policy != "" {
		b.policiesStore(region)[keyID] = input.Policy
	}

	out := &CreateKeyOutput{
		KeyMetadata: b.keyToMetadata(key),
	}

	return out, nil
}

// DescribeKey returns metadata for the specified key.
func (b *InMemoryBackend) DescribeKey(
	ctx context.Context,
	input *DescribeKeyInput,
) (*DescribeKeyOutput, error) {
	b.mu.RLock("DescribeKey")
	defer b.mu.RUnlock()

	key, err := b.lookupKey(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	// DescribeKey is a grant operation with no encryption context, so validate
	// grant-token presence only (existence + TTL) -- consistent with Sign/Verify/
	// GetPublicKey/DeriveSharedSecret. Empty GrantTokens is a no-op, which is the
	// only case Terraform ever exercises.
	if err = b.validateGrantTokenPresence(input.GrantTokens, "DescribeKey"); err != nil {
		return nil, err
	}

	meta := b.keyToMetadata(key)
	meta.MultiRegionConfiguration = b.buildMultiRegionConfig(ctx, key)

	return &DescribeKeyOutput{KeyMetadata: meta}, nil
}

// ListKeys returns a paginated list of all keys.
func (b *InMemoryBackend) ListKeys(
	ctx context.Context,
	input *ListKeysInput,
) (*ListKeysOutput, error) {
	b.mu.RLock("ListKeys")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	entries := make([]KeyListEntry, 0, b.keysStore(region).Len())

	for _, k := range b.keysStore(region).All() {
		entries = append(
			entries,
			KeyListEntry{KeyID: k.KeyID, KeyArn: k.Arn, Description: k.Description},
		)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].KeyID < entries[j].KeyID
	})

	startIdx := parseMarker(input.Marker)
	limit := int32(defaultListLimit)

	if input.Limit != nil {
		if *input.Limit < 1 || *input.Limit > 1000 {
			return nil, fmt.Errorf("%w: Limit must be between 1 and 1000", ErrValidation)
		}

		limit = *input.Limit
	}

	if startIdx >= len(entries) {
		return &ListKeysOutput{Keys: []KeyListEntry{}}, nil
	}

	end := startIdx + int(limit)

	var nextMarker string

	if end < len(entries) {
		nextMarker = strconv.Itoa(end)
	} else {
		end = len(entries)
	}

	return &ListKeysOutput{
		Keys:       entries[startIdx:end],
		NextMarker: nextMarker,
		Truncated:  nextMarker != "",
	}, nil
}

// DisableKey disables the specified key.
// AWS raises KMSInvalidStateException for keys pending deletion or import.
func (b *InMemoryBackend) DisableKey(ctx context.Context, input *DisableKeyInput) error {
	b.mu.Lock("DisableKey")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	if key.KeyState == KeyStatePendingDeletion || key.KeyState == KeyStatePendingImport ||
		key.KeyState == KeyStatePendingReplicaDeletion {
		return keyStateError(key)
	}

	key.KeyState = KeyStateDisabled
	key.Enabled = false
	b.evictAliasesFromCache(region, key.KeyID)

	return nil
}

// EnableKey enables the specified key.
// AWS raises KMSInvalidStateException for keys pending deletion or import.
func (b *InMemoryBackend) EnableKey(ctx context.Context, input *EnableKeyInput) error {
	b.mu.Lock("EnableKey")
	defer b.mu.Unlock()

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	if key.KeyState == KeyStatePendingDeletion || key.KeyState == KeyStatePendingImport ||
		key.KeyState == KeyStatePendingReplicaDeletion {
		return keyStateError(key)
	}

	key.KeyState = KeyStateEnabled
	key.Enabled = true

	return nil
}

// ScheduleKeyDeletion schedules a key for deletion.
// PendingWindowInDays must be in the range [7, 30]; values outside this range are rejected.
// AWS raises ValidationException for out-of-range values and KMSInvalidStateException
// for keys already in PendingDeletion.
func (b *InMemoryBackend) ScheduleKeyDeletion(
	ctx context.Context,
	input *ScheduleKeyDeletionInput,
) (*ScheduleKeyDeletionOutput, error) {
	b.mu.Lock("ScheduleKeyDeletion")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	if key.KeyState == KeyStatePendingDeletion {
		return nil, keyStateError(key)
	}

	days := input.PendingWindowInDays
	if days == 0 {
		days = defaultPendingWindowDays
	}

	if days < minPendingWindowDays || days > maxPendingWindowDays {
		return nil, fmt.Errorf(
			"%w: PendingWindowInDays must be between %d and %d, got %d",
			ErrValidation, minPendingWindowDays, maxPendingWindowDays, days,
		)
	}

	key.Enabled = false
	key.PendingWindowInDays = days

	// KMS will not delete a multi-Region primary key with existing replica
	// keys: it moves to the non-final PendingReplicaDeletion state instead,
	// with no DeletionDate yet -- the waiting period only starts once the
	// last replica is actually deleted (see the janitor's promotion logic).
	if b.isMultiRegionPrimaryWithReplicasLocked(key) {
		key.KeyState = KeyStatePendingReplicaDeletion
		key.DeletionDate = 0
		b.evictAliasesFromCache(region, key.KeyID)

		return &ScheduleKeyDeletionOutput{
			KeyID:               key.KeyID,
			KeyState:            key.KeyState,
			PendingWindowInDays: days,
		}, nil
	}

	deletionDate := time.Now().UTC().AddDate(0, 0, days)
	key.KeyState = KeyStatePendingDeletion
	key.DeletionDate = UnixTimeFloat(deletionDate)
	b.evictAliasesFromCache(region, key.KeyID)

	return &ScheduleKeyDeletionOutput{
		KeyID:               key.KeyID,
		DeletionDate:        key.DeletionDate,
		KeyState:            key.KeyState,
		PendingWindowInDays: days,
	}, nil
}

// isMultiRegionPrimaryWithReplicasLocked reports whether key is a multi-Region
// primary key that still has at least one replica key in existence. Must be
// called with the backend write lock held.
func (b *InMemoryBackend) isMultiRegionPrimaryWithReplicasLocked(key *Key) bool {
	if !key.MultiRegion {
		return false
	}

	if key.PrimaryRegion != "" && key.PrimaryRegion != extractRegionFromARN(key.Arn) {
		return false // key is a replica, not a primary
	}

	for _, replicaID := range key.ReplicaKeyIDs {
		if b.findKeyInAnyRegion(replicaID) != nil {
			return true
		}
	}

	return false
}

// CancelKeyDeletion cancels a pending key deletion and sets the key to Disabled.
// AWS raises KMSInvalidStateException if the key is not pending deletion
// (KeyStatePendingDeletion or KeyStatePendingReplicaDeletion).
func (b *InMemoryBackend) CancelKeyDeletion(
	ctx context.Context,
	input *CancelKeyDeletionInput,
) (*CancelKeyDeletionOutput, error) {
	b.mu.Lock("CancelKeyDeletion")
	defer b.mu.Unlock()

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	if key.KeyState != KeyStatePendingDeletion && key.KeyState != KeyStatePendingReplicaDeletion {
		return nil, keyStateError(key)
	}

	key.KeyState = KeyStateDisabled
	key.Enabled = false
	key.DeletionDate = 0

	return &CancelKeyDeletionOutput{KeyID: key.KeyID, KeyState: key.KeyState}, nil
}

// keyToMetadata converts a Key to its KeyMetadata representation.
func (b *InMemoryBackend) keyToMetadata(k *Key) KeyMetadata {
	origin := k.Origin
	if origin == "" {
		origin = KeyOriginAWSKMS
	}

	meta := KeyMetadata{
		KeyID:                 k.KeyID,
		AWSAccountID:          b.accountID,
		Arn:                   k.Arn,
		Description:           k.Description,
		KeyState:              k.KeyState,
		KeyUsage:              k.KeyUsage,
		KeySpec:               k.KeySpec,
		CustomerMasterKeySpec: k.KeySpec,
		CreationDate:          k.CreationDate,
		KeyManager:            "CUSTOMER",
		Origin:                origin,
		MultiRegion:           k.MultiRegion,
		PrimaryRegion:         k.PrimaryRegion,
		CustomKeyStoreID:      k.CustomKeyStoreID,
		Enabled:               k.Enabled,
	}

	// DeletionDate and PendingWindowInDays are only meaningful for PendingDeletion keys.
	if k.KeyState == KeyStatePendingDeletion {
		meta.DeletionDate = k.DeletionDate
		meta.PendingDeletionWindowInDays = k.PendingWindowInDays
	}

	applyExpirationFields(k, &meta)
	applyMultiRegionType(k, &meta)
	applyAlgorithmFields(k, &meta)

	return meta
}

// applyExpirationFields sets ValidTo and ExpirationModel on meta for EXTERNAL keys.
func applyExpirationFields(k *Key, meta *KeyMetadata) {
	if k.Origin != KeyOriginExternal {
		return
	}

	if k.ValidTo > 0 {
		meta.ValidTo = k.ValidTo
		meta.ExpirationModel = expirationModelExpires

		return
	}

	if k.ExpirationModel == expirationModelNoExpiry {
		meta.ExpirationModel = expirationModelNoExpiry

		return
	}

	meta.ExpirationModel = k.ExpirationModel
}

// applyMultiRegionType sets MultiRegionKeyType on meta for multi-region keys.
func applyMultiRegionType(k *Key, meta *KeyMetadata) {
	if !k.MultiRegion || k.PrimaryRegion == "" {
		return
	}

	if k.PrimaryRegion == extractRegionFromARN(k.Arn) {
		meta.MultiRegionKeyType = "PRIMARY"
	} else {
		meta.MultiRegionKeyType = "REPLICA"
	}
}

// buildMultiRegionConfig constructs the MultiRegionConfiguration for a key, following
// the same PRIMARY/REPLICA logic used by AWS DescribeKey. Returns nil for non-multi-region keys.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) buildMultiRegionConfig(
	_ context.Context,
	key *Key,
) *MultiRegionConfiguration {
	if !key.MultiRegion {
		return nil
	}

	keyRegion := extractRegionFromARN(key.Arn)

	if key.PrimaryRegion == "" || key.PrimaryRegion == keyRegion {
		return b.buildPrimaryMultiRegionConfig(key, keyRegion)
	}

	return b.buildReplicaMultiRegionConfig(key)
}

// buildPrimaryMultiRegionConfig returns the MultiRegionConfiguration for a primary key.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) buildPrimaryMultiRegionConfig(
	key *Key,
	keyRegion string,
) *MultiRegionConfiguration {
	cfg := &MultiRegionConfiguration{
		MultiRegionKeyType: "PRIMARY",
		PrimaryKey:         &MultiRegionKeyRef{Arn: key.Arn, Region: keyRegion},
	}

	for _, replicaID := range key.ReplicaKeyIDs {
		// replica keys may live in any region — search all regions
		if rk := b.findKeyInAnyRegion(replicaID); rk != nil {
			cfg.ReplicaKeys = append(cfg.ReplicaKeys, MultiRegionKeyRef{
				Arn:    rk.Arn,
				Region: extractRegionFromARN(rk.Arn),
			})
		}
	}

	return cfg
}

// buildReplicaMultiRegionConfig returns the MultiRegionConfiguration for a replica key by
// scanning keys to locate the primary. Must be called with at least a read lock held.
func (b *InMemoryBackend) buildReplicaMultiRegionConfig(key *Key) *MultiRegionConfiguration {
	cfg := &MultiRegionConfiguration{
		MultiRegionKeyType: "REPLICA",
	}

	primaryKey := b.findPrimaryKeyForReplica(key)
	if primaryKey != nil {
		cfg.PrimaryKey = &MultiRegionKeyRef{
			Arn:    primaryKey.Arn,
			Region: key.PrimaryRegion,
		}

		for _, replicaID := range primaryKey.ReplicaKeyIDs {
			if rk := b.findKeyInAnyRegion(replicaID); rk != nil {
				cfg.ReplicaKeys = append(cfg.ReplicaKeys, MultiRegionKeyRef{
					Arn:    rk.Arn,
					Region: extractRegionFromARN(rk.Arn),
				})
			}
		}
	} else {
		cfg.PrimaryKey = &MultiRegionKeyRef{Region: key.PrimaryRegion}
	}

	return cfg
}

// findKeyInAnyRegion searches all region stores for a key with the given keyID.
// Must be called with at least a read lock held.
func (b *InMemoryBackend) findKeyInAnyRegion(keyID string) *Key {
	for _, t := range b.keys {
		if k, ok := t.Get(keyID); ok {
			return k
		}
	}

	return nil
}

// promoteMultiRegionPrimaryAfterReplicaPurgeLocked checks whether purgedKey was the
// last surviving replica of a primary key in KeyStatePendingReplicaDeletion, and if
// so, moves that primary to KeyStatePendingDeletion and starts its waiting period
// now, using the PendingWindowInDays recorded by its original ScheduleKeyDeletion
// call. Matches real AWS: "When the last of its replicas keys is deleted (not just
// scheduled), the key state of the primary key changes to PendingDeletion and its
// waiting period begins." Must be called with the backend write lock held, just
// before purgedKey is removed from its store.
func (b *InMemoryBackend) promoteMultiRegionPrimaryAfterReplicaPurgeLocked(purgedKey *Key) {
	if !purgedKey.MultiRegion || purgedKey.PrimaryRegion == "" {
		return
	}

	primary := b.findPrimaryKeyForReplica(purgedKey)
	if primary == nil || primary.KeyState != KeyStatePendingReplicaDeletion {
		return
	}

	for _, replicaID := range primary.ReplicaKeyIDs {
		if replicaID == purgedKey.KeyID {
			continue
		}

		if b.findKeyInAnyRegion(replicaID) != nil {
			return // another replica still exists
		}
	}

	deletionDate := time.Now().UTC().AddDate(0, 0, primary.PendingWindowInDays)
	primary.KeyState = KeyStatePendingDeletion
	primary.DeletionDate = UnixTimeFloat(deletionDate)
}

// findPrimaryKeyForReplica locates the primary key that lists replicaKey.KeyID in its
// ReplicaKeyIDs. Must be called with at least a read lock held.
func (b *InMemoryBackend) findPrimaryKeyForReplica(replicaKey *Key) *Key {
	for _, t := range b.keys {
		for _, k := range t.All() {
			if !k.MultiRegion || extractRegionFromARN(k.Arn) != replicaKey.PrimaryRegion {
				continue
			}

			if slices.Contains(k.ReplicaKeyIDs, replicaKey.KeyID) {
				return k
			}
		}
	}

	return nil
}

// applyAlgorithmFields sets the algorithm lists on meta based on key usage and spec.
func applyAlgorithmFields(k *Key, meta *KeyMetadata) {
	switch k.KeyUsage {
	case KeyUsageEncryptDecrypt:
		if k.KeySpec == keySpecRSA2048 || k.KeySpec == keySpecRSA3072 ||
			k.KeySpec == keySpecRSA4096 {
			meta.EncryptionAlgorithms = []string{algoRSAESOAEPSHA1, encryptionAlgorithmRSAOAEP}
		} else {
			meta.EncryptionAlgorithms = []string{"SYMMETRIC_DEFAULT"}
		}
	case KeyUsageSignVerify:
		meta.SigningAlgorithms = defaultSigningAlgorithms(k.KeySpec)
	case KeyUsageGenerateMac:
		meta.MacAlgorithms = defaultMacAlgorithms(k.KeySpec)
	case KeyUsageKeyAgreement:
		meta.KeyAgreementAlgorithms = []string{algoECDH}
	}
}

// extractRegionFromARN parses the region component from a KMS ARN.
func extractRegionFromARN(arnStr string) string {
	parsed, err := awsarn.Parse(arnStr)
	if err != nil {
		return ""
	}

	return parsed.Region
}

// UpdateKeyDescription updates a key's description field.
func (b *InMemoryBackend) UpdateKeyDescription(
	ctx context.Context,
	input *UpdateKeyDescriptionInput,
) error {
	if len(input.Description) > maxDescriptionLength {
		return fmt.Errorf(
			"%w: Description exceeds maximum length of %d characters",
			ErrValidation, maxDescriptionLength,
		)
	}

	b.mu.Lock("UpdateKeyDescription")
	defer b.mu.Unlock()

	key, err := b.lookupKeyWrite(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return err
	}

	key.Description = input.Description

	return nil
}

// recordLastUsage stores the last successful cryptographic operation for the given key.
// It is safe to call concurrently without holding any lock.
func (b *InMemoryBackend) recordLastUsage(region, canonicalKeyID, operation string) {
	b.lastUsage.Store(region+":"+canonicalKeyID, &KeyLastUsageData{
		Operation:         operation,
		Timestamp:         UnixTimeFloat(time.Now()),
		CloudTrailEventID: uuid.New().String(),
		KmsRequestID:      uuid.New().String(),
	})
}

// GetKeyLastUsage returns the last successful cryptographic operation performed with the specified key.
//
// Unlike almost every other KeyId-accepting KMS operation, the real
// aws-sdk-go-v2/service/kms@v1.55.4 GetKeyLastUsageInput doc comment is
// explicit that "Alias names are not supported" here -- a key ID or key ARN
// only. Rejected before taking any lock, matching validKeyPolicyDoc-style
// input validation elsewhere in this file.
func (b *InMemoryBackend) GetKeyLastUsage(
	ctx context.Context,
	input *GetKeyLastUsageInput,
) (*GetKeyLastUsageOutput, error) {
	if isAliasKeyID(input.KeyID) {
		// GetKeyLastUsage's own deserializeOpError recognizes NotFoundException,
		// not ValidationException -- an alias name is a KeyId shape this op
		// doesn't resolve, so it is classified the same as an unresolvable KeyId.
		return nil, fmt.Errorf(
			"%w: GetKeyLastUsage does not support alias names; specify a key ID or key ARN",
			ErrKeyNotFound,
		)
	}

	b.mu.RLock("GetKeyLastUsage")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	key, err := b.lookupKey(ctx, input.KeyID, ErrInvalidArn)
	if err != nil {
		return nil, err
	}

	out := &GetKeyLastUsageOutput{
		KeyID:             key.KeyID,
		KeyCreationDate:   key.CreationDate,
		TrackingStartDate: key.CreationDate,
	}

	if v, loaded := b.lastUsage.Load(region + ":" + key.KeyID); loaded {
		if lu, ok := v.(*KeyLastUsageData); ok {
			out.KeyLastUsage = lu
		}
	}

	return out, nil
}
