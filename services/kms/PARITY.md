---
service: kms
sdk_module: aws-sdk-go-v2/service/kms@v1.55.4
last_audit_commit: 13c27883a00454a6e63bc767d096528ecfd6c4b1
last_audit_date: 2026-07-23
overall: A            # Full sweep of the 5 gaps/2 deferred items this file previously
                       # tracked, plus a dedicated leak hunt. Found + fixed 1 real leak
                       # (Handler.tags -- a side map keyed by KeyID, entirely outside
                       # InMemoryBackend -- was never cleaned up when the janitor
                       # permanently purged a key, leaking one *tags.Tags, and the
                       # lockmetrics/Prometheus registration it owns, per tagged key for
                       # the process lifetime, since KMS key IDs are UUIDs and are never
                       # reused). Closed all 3 still-open gaps (GrantConstraints.SourceArn,
                       # CreateGrantInput.GrantTokens, GranteeServicePrincipal/
                       # RetiringServicePrincipal) as real wire-shape/validation fixes.
                       # IMPORTANT CORRECTION: GetKeyLastUsage was mislabeled "not a real
                       # AWS KMS operation" by every prior audit pass on this file --
                       # confirmed against the actual vendored aws-sdk-go-v2/service/
                       # kms@v1.54.0 (api_op_GetKeyLastUsage.go exists, with a real
                       # Client.GetKeyLastUsage method and TestSDKCompleteness already
                       # silently accounting for it). It IS real, and gopherstack's
                       # existing implementation was field-diffed as correct except for
                       # one genuine behavioral gap (fixed this pass): the real API's
                       # KeyId doc comment is explicit that "Alias names are not
                       # supported," but gopherstack's GetKeyLastUsage accepted them like
                       # every other KeyId-taking op. Re-tagged from `deferred` to a normal
                       # `ops` row below.
ops:
  CreateKey: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "invalid KeySpec now classifies as ValidationException (400), not InternalServiceError (500); tags now validated before the key is created (was: orphan-leak on bad tag). 2026-09-07 (gopherstack-e76y): CreateKeyInput/KeyMetadata gained CustomKeyStoreId (real SDK: kms@v1.55.4 api_op_CreateKey.go:228, types/types.go:439) -- see the gaps entry below for the validation implemented."}
  DescribeKey: {wire: fixed, errors: ok, state: ok, persist: ok, note: "DescribeKeyInput was missing the GrantTokens field entirely (real SDK: aws-sdk-go-v2/service/kms@v1.54.0 DescribeKeyInput carries GrantTokens []string; DescribeKey is a valid grant operation and declares InvalidGrantTokenException in its error set). Added the field + validateGrantTokenPresence (existence+TTL, no encryption-context check -- consistent with Sign/Verify/GetPublicKey/DeriveSharedSecret). Empty tokens is a no-op (the only case Terraform exercises)."}
  ListKeys: {wire: ok, errors: ok, state: ok, persist: ok}
  Encrypt: {wire: ok, errors: fixed, state: ok, persist: ok, note: "real AES-256-GCM / RSA-OAEP-SHA-256, AAD-bound encryption context, grant-token constraint check already present; expired imported material now classifies as ExpiredImportTokenException (400), not 500"}
  Decrypt: {wire: ok, errors: fixed, state: ok, persist: ok, note: "key ID embedded in blob prefix; mismatched context fails AES-GCM auth -> InvalidCiphertextException; history fallback for post-rotation ciphertexts; expired imported material now classifies as ExpiredImportTokenException (400), not 500"}
  ReEncrypt: {wire: ok, errors: ok, state: ok, persist: ok}
  GenerateDataKey: {wire: ok, errors: ok, state: ok, persist: ok}
  GenerateDataKeyWithoutPlaintext: {wire: ok, errors: ok, state: ok, persist: ok}
  GenerateDataKeyPair: {wire: fixed, errors: ok, state: ok, persist: ok, note: "GrantTokens field + EncryptionContext size validation + grant-constraint enforcement were all missing; added"}
  GenerateDataKeyPairWithoutPlaintext: {wire: fixed, errors: ok, state: ok, persist: ok, note: "delegates to GenerateDataKeyPair; GrantTokens now threaded through"}
  Sign: {wire: fixed, errors: ok, state: ok, persist: ok, note: "GrantTokens field + grant-token validity check were missing (disguised stub: op is a valid grant operation but the token was silently dropped)"}
  Verify: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same GrantTokens gap as Sign"}
  GetPublicKey: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same GrantTokens gap as Sign"}
  GenerateMac: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same GrantTokens gap as Sign"}
  VerifyMac: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same GrantTokens gap as Sign"}
  DeriveSharedSecret: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same GrantTokens gap as Sign; real ECDH via crypto/ecdh conversion"}
  GenerateRandom: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreateAlias: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAlias: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAlias: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAliases: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableKeyRotation: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableKeyRotation: {wire: ok, errors: ok, state: ok, persist: ok}
  GetKeyRotationStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  RotateKeyOnDemand: {wire: ok, errors: ok, state: ok, persist: ok, note: "10-per-24h on-demand rate limit enforced"}
  ListKeyRotations: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableKey: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableKey: {wire: ok, errors: ok, state: ok, persist: ok}
  ScheduleKeyDeletion: {wire: ok, errors: ok, state: ok, persist: ok, note: "7-30 day window enforced; janitor purges past DeletionDate"}
  CancelKeyDeletion: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateGrant: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "region-resolution fix (see below) PLUS this pass's 3 gap closures: (1) GrantConstraints gained SourceArn (real SDK field; stored/round-tripped through ListGrants/ListRetirableGrants, NOT enforced -- no cross-service request-context plumbing exists anywhere in this mock to carry a caller/resource ARN through crypto calls, same documented scope boundary as grant-token authorization); (2) CreateGrantInput gained GrantTokens (real SDK field; accepted as a no-op, same precedent as CreateKeyInput/ReplicateKeyInput's BypassPolicyLockoutSafetyCheck -- no IAM layer exists to authorize the CreateGrant call itself); (3) CreateGrantInput/Grant gained GranteeServicePrincipal + RetiringServicePrincipal (real SDK fields), WITH real validation: exactly one of GranteePrincipal/GranteeServicePrincipal required, RetiringPrincipal/RetiringServicePrincipal mutually exclusive, and a service grantee requires a SourceArn constraint + a retiring principal of either kind, matching the real CreateGrantInput doc comments exactly. Also added Grant.IssuingAccount (real GrantListEntry field, was entirely absent -- populated from the backend's account ID). See TestCreateGrant_ServicePrincipals, TestGrantConstraint_SourceArn_RoundTrips, TestCreateGrant_IssuingAccount_Populated, TestCreateGrant_GrantTokens_AcceptedAsNoOp in grants_test.go, plus persistence coverage in TestInMemoryBackend_FullStateSnapshotRestoreRoundTrip."}
  ListGrants: {wire: ok, errors: ok, state: fixed, persist: ok, note: "same region-resolution fix as CreateGrant"}
  RevokeGrant: {wire: ok, errors: ok, state: fixed, persist: ok, note: "same region-resolution fix as CreateGrant"}
  RetireGrant: {wire: ok, errors: ok, state: fixed, persist: ok, note: "GrantId+KeyId path now uses the key's own region; GrantId-only (no KeyId, no region hint) now searches all regions instead of only the request region"}
  ListRetirableGrants: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "2026-08-28 write-only-state sweep: ListRetirableGrantsInput had no RetiringServicePrincipal field (real SDK: aws-sdk-go-v2/service/kms@v1.55.4 api_op_ListRetirableGrants.go, ListRetirableGrantsInput carries both RetiringPrincipal and RetiringServicePrincipal -- 'You must specify either ... but not both'), and the backend filtered solely on g.RetiringPrincipal == input.RetiringPrincipal. CreateGrant has always accepted and stored GranteeServicePrincipal/RetiringServicePrincipal on the Grant (see the CreateGrant op row above), so a grant whose only retiring principal was a service principal could be created but never discovered through ListRetirableGrants -- KMS's only real read path for 'which grants can I retire' (RetireGrant itself requires a GrantId/GrantToken you'd otherwise have no way to find). Worth noting for the next auditor: a naive round-trip test here can pass by accident, because both the (dropped) request field and the unset Grant.RetiringPrincipal default to the empty string, so an empty-string == empty-string match looks like a hit; the real test needs a decoy grant with neither retiring-principal field set and an exact-count assertion. Fixed: added RetiringServicePrincipal to ListRetirableGrantsInput and OR'd it into the filter (each side only matches when its own input field is non-empty). See TestListRetirableGrants_RetiringServicePrincipal_RealClient in wire_field_fixes_test.go."}
  PutKeyPolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "same region-resolution fix as CreateGrant -- policy now stored in the key's own region so a cross-region ARN round-trips through GetKeyPolicy"}
  GetKeyPolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "same region-resolution fix as CreateGrant -- reads the policy from the key's own region (ARN-embedded region for an ARN input)"}
  ListKeyPolicies: {wire: ok, errors: ok, state: ok, persist: n/a, note: "already region-aware (routes through lookupKey); confirmed no change needed"}
  GetParametersForImport: {wire: ok, errors: ok, state: ok, persist: ok, note: "real RSA-2048/3072/4096 wrapping keypair generated per call"}
  ImportKeyMaterial: {wire: ok, errors: ok, state: ok, persist: ok, note: "real RSA-OAEP unwrap of caller material. FIXED 2026-08-11 -- the key material field was wire-tagged KeyMaterial; the real ImportKeyMaterialRequest field is EncryptedKeyMaterial, so every real client's material was rejected with 'KeyMaterial must not be empty'. ImportToken remains unmodeled -- resolveKeyMaterial looks up the wrapping key by KeyId alone (set by GetParametersForImport), so the token's value was never load-bearing; Go silently drops the extra unrecognized field, which is harmless"}
  DeleteImportedKeyMaterial: {wire: ok, errors: ok, state: ok, persist: ok}
  ReplicateKey: {wire: fixed, errors: ok, state: ok, persist: ok, note: "tag validation moved before replica creation (was: orphan-leak on bad tag, and tags on ReplicateKey bypassed validateTag entirely); ReplicateKeyInput was ALSO missing the Policy field entirely (confirmed against aws-sdk-go-v2/service/kms@v1.54.0's api_op_ReplicateKey.go) -- an inline replica policy was silently dropped, so GetKeyPolicy on the replica always returned the synthesized default, the exact same bug class (and same Terraform symptom: aws_kms_replica_key's post-apply GetKeyPolicy poll never converges) as the already-fixed CreateKey Policy bug. Fixed by adding Policy (+ BypassPolicyLockoutSafetyCheck, unused like CreateKey's own copy since there is no IAM layer) to ReplicateKeyInput and persisting it into the replica's region-scoped policiesStore, mirroring CreateKey exactly."}
  UpdateKeyDescription: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePrimaryRegion: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCustomKeyStore: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteCustomKeyStore: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "2026-09-07 (gopherstack-e76y): now refuses deletion while any key's CustomKeyStoreId still references the store (real SDK doc: 'The custom key store that you delete cannot contain any KMS keys'; CustomKeyStoreHasCMKsException, confirmed in deserializeOpErrorDeleteCustomKeyStore's error list)."}
  DescribeCustomKeyStores: {wire: ok, errors: ok, state: ok, persist: ok}
  ConnectCustomKeyStore: {wire: ok, errors: ok, state: ok, persist: ok}
  DisconnectCustomKeyStore: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCustomKeyStore: {wire: ok, errors: ok, state: ok, persist: ok}
  GetKeyLastUsage: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "CORRECTION: every prior pass on this file mislabeled this as 'not a real AWS KMS operation' and filed it under deferred -- it IS real (confirmed against the vendored aws-sdk-go-v2/service/kms@v1.54.0's api_op_GetKeyLastUsage.go: a real Client.GetKeyLastUsage method exists, and TestSDKCompleteness already silently accounted for it without complaint, which is what surfaced the mislabel when this pass tried to remove the op from the wire). Field-diffed as correct: GetKeyLastUsageInput/Output shapes match exactly (KeyId/KeyCreationDate/TrackingStartDate/KeyLastUsage with CloudTrailEventId/KmsRequestId/Operation/Timestamp), and the set of operations that record last-usage (recordLastUsage callers in data_keys.go/encryption.go/hmac.go/key_agreement.go/signing.go) matches the real SDK's types.KeyLastUsageTrackingOperation enum values exactly (all 12: Decrypt, DeriveSharedSecret, Encrypt, GenerateDataKey(Pair)(WithoutPlaintext) x3, GenerateMac, ReEncrypt, Sign, Verify, VerifyMac). One real gap found and fixed: the real API's KeyId doc comment is explicit that 'Alias names are not supported' for this one operation (unlike almost every other KeyId-accepting KMS op), but gopherstack's GetKeyLastUsage routed through the general-purpose lookupKey, silently accepting aliases. Fixed with a new isAliasKeyID helper (store.go) called before taking any lock, rejecting alias names/alias ARNs with ValidationException. See TestGetKeyLastUsage_RejectsAliasKeyID in get_key_last_usage_test.go."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: fixed, note: "tags stored via pkgs/tags in a Handler-level side map (Handler.tags, keyed by KeyID), NOT in InMemoryBackend.backendSnapshot -- Handler.Snapshot previously delegated straight to Backend.Snapshot and never serialized Handler.tags at all, so a process restart with persistence enabled silently dropped every key's tags (ListResourceTags stayed correct within a single running process, masking the gap). Fixed: Handler.Snapshot/Restore now wrap the backend snapshot together with a tags map (see persistence.go's handlerSnapshot); a handlerFormat marker distinguishes the new wrapped shape from a legacy pre-fix snapshot (raw backend bytes) so old on-disk snapshots still restore backend state cleanly, just without tags (no worse than before). Re-checked this pass (wrapper-key sweep) against the sfn TagResource map/array bug class: kms's Tags is []types.Tag with TagKey/TagValue fields, not Key/Value (api_op_TagResource.go, serializers.go:3400-3415), matching this emulator's []kmsTagEntry{TagKey,TagValue} exactly -- genuinely clean, confirmed via a real-client round-trip test (tag_resource_sdk_test.go)."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: fixed, note: "same Handler.tags persistence fix as TagResource"}
  ListResourceTags: {wire: ok, errors: ok, state: ok, persist: fixed, note: "same Handler.tags persistence fix as TagResource"}
families:
  crypto_core: {status: ok, note: "AES-256-GCM (real Seal/Open, AAD = keyID + sorted encryption-context pairs), RSA-OAEP-SHA-256/SHA-1 fallback, RSASSA-PSS/PKCS1v15, ECDSA P-256/384/521, ECDH (crypto/ecdh), HMAC-SHA-256/384/512 — all real crypto/*, no mock byte-flipping anywhere in crypto.go"}
  error_classification: {status: fixed, note: "kmsErrorTable was missing entries for ErrExpiredKeyMaterial and ErrKeyMaterialUnavailable (raised by checkKeyMaterialExpiry/requireKeyMaterial, reachable from every crypto op: Encrypt, Decrypt, ReEncrypt, Sign, Verify, GetPublicKey, GenerateMac, VerifyMac, DeriveSharedSecret, GenerateDataKeyPair(WithoutPlaintext)), so both fell through to the generic 500 default. Also, that generic default itself emitted the type string \"InternalServiceError\", which is not a real KMS exception name (the real SDK's unclassified-server-error type is KMSInternalException) -- a caller's errors.As(&types.KMSInternalException{}) would never match. Fixed: added both sentinels to the table (ExpiredImportTokenException/400 client-fault, KeyUnavailableException/500 server-fault per the real SDK's ErrorFault), and changed the default-branch type string to KMSInternalException."}
  key_state_machine: {status: ok, note: "Enabled/Disabled/PendingDeletion/PendingImport transitions all gated; keyStateError() maps Disabled->DisabledException, everything else->KMSInvalidStateException"}
  multi_region: {status: ok, note: "ReplicateKey/UpdatePrimaryRegion primary<->replica promotion verified by existing TestUpdatePrimaryRegion_RoleSwap; DescribeKey MultiRegionConfiguration built correctly for both primary and replica sides"}
gaps:
  - "RESOLVED 2026-07-23: GrantConstraints had no SourceArn field (real SDK: GrantConstraints.SourceArn). Added; round-trips through CreateGrant -> ListGrants/ListRetirableGrants/Snapshot-Restore. NOT enforced -- no operation in this mock threads a caller/resource ARN through crypto calls to check against it, and no other service adapter currently supplies one either; enforcement remains cross-cutting request-context plumbing, not a KMS-local fix (bd: gopherstack-w3k, still open for the enforcement half only)."
  - "RESOLVED 2026-07-23: CreateGrantInput had no GrantTokens field (real SDK: authorizes the CreateGrant call itself via an existing not-yet-consistent grant). Added; accepted as a no-op (no IAM/authorization layer exists anywhere in this mock to authorize against), same precedent as CreateKeyInput/ReplicateKeyInput's BypassPolicyLockoutSafetyCheck."
  - "RESOLVED 2026-07-23: GranteeServicePrincipal / RetiringServicePrincipal (AWS-service grantees) were not modeled on CreateGrantInput. Added, WITH real validation (exactly one of GranteePrincipal/GranteeServicePrincipal; RetiringPrincipal/RetiringServicePrincipal mutually exclusive; a service grantee requires a SourceArn constraint + a retiring principal), matching the real CreateGrantInput doc comments. No AWS-service-principal *simulation* exists (still no IAM layer), but the wire shape and its documented validation rules are both real and enforced now, unlike SourceArn's constraint-enforcement half above which has nothing local to check against."
  - "RESOLVED 2026-07-12: DescribeKeyInput was missing the GrantTokens field -- added + wired validateGrantTokenPresence (see the DescribeKey op row and describe_key_grant_tokens_test.go). Unlike the CreateGrant/CreateGrantInput GrantTokens gap above (which authorizes the CreateGrant call itself and has nothing to validate without an IAM layer), DescribeKey's GrantTokens resolve to real existing grants, so validation is meaningful and AWS-accurate here (DescribeKey declares InvalidGrantTokenException)."
  - "RESOLVED 2026-07-12: the region-scoped KeyId resolution inconsistency (GetKeyPolicy/PutKeyPolicy/CreateGrant/ListGrants/RevokeGrant/RetireGrant indexed their policiesStore/grantsRegion using the request region instead of an ARN's embedded region). Root cause was two-fold and both fixed at source: (1) these ops discarded the region resolveKeyID returned and re-used getRegion(ctx) -- fixed by adding a shared resolveKeyAndRegion helper (lookupKey now delegates to it too) that returns the key's actual region, and routing all six ops through it; (2) resolveKeyID's resolution cache stored only the resolved UUID and returned the REQUEST region on every cache hit, so even the region resolveKeyID returned was wrong for any ARN resolved more than once -- fixed by caching a {keyID, region} pair (region=\"\" sentinel for aliases means 'derive from request context', so alias behavior is unchanged; ARN caches its own embedded region, which is safe because the region is part of the ARN cache key). Verified by region_scoped_resolution_test.go (cross-region ReplicateKey -> replica-ARN Put/GetKeyPolicy round-trip + full grant lifecycle by ARN, all while ctx defaults to the primary's region)."
  - "RESOLVED 2026-09-07 (gopherstack-e76y): CreateKeyInput/KeyMetadata gained CustomKeyStoreID (real SDK: aws-sdk-go-v2/service/kms@v1.55.4 api_op_CreateKey.go:228, types/types.go:439). CreateKeyInput.CustomKeyStoreId's own doc comment (quoted in full): 'Creates the KMS key in the specified custom key store. The ConnectionState of the custom key store must be CONNECTED. To find the CustomKeyStoreID and ConnectionState use the DescribeCustomKeyStores operation. This parameter is valid only for symmetric encryption KMS keys in a single Region. You cannot create any other type of KMS key in a custom key store. When you create a KMS key in an CloudHSM key store, KMS generates a non-exportable 256-bit symmetric key in its associated CloudHSM cluster and associates it with the KMS key. When you create a KMS key in an external key store, you must use the XksKeyId parameter to specify an external key that serves as key material for the KMS key.' Implemented exactly what it states, no more: (1) the store must exist (CustomKeyStoreNotFoundException, extraction-confirmed in deserializeOpErrorCreateKey's error list); (2) its ConnectionState must be CONNECTED (CustomKeyStoreInvalidStateException, also extraction-confirmed, and its own doc comment names 'You requested the CreateKey operation in a custom key store that is not connected' as one of its own reasons for existing); (3) 'valid only for symmetric encryption KMS keys in a single Region' -- resolved KeySpec must be SYMMETRIC_DEFAULT, KeyUsage must be ENCRYPT_DECRYPT, and MultiRegion must be false, or the request is rejected with UnsupportedOperationException (ErrUnsupportedParameter, same sentinel already used elsewhere in this file for 'a specified parameter is not supported'). Declined only the external-key-store half: rather than accept a CustomKeyStoreId for an EXTERNAL_KEY_STORE-type store and silently ignore the doc's 'you must use the XksKeyId parameter' requirement, CreateKey rejects it outright (also UnsupportedOperationException) since XksKeyId itself is unmodeled -- see the new XksKeyId gap entry below (gopherstack-ufvn). The association persists on Key.CustomKeyStoreID and round-trips through DescribeKey."
  - "OPEN 2026-09-07 (gopherstack-ufvn, filed alongside gopherstack-e76y): XksKeyId (external-key-store variant of the CustomKeyStoreId linkage, CreateKeyInput field, api_op_CreateKey.go:505) remains entirely unmodeled -- no field, no per-store external-key uniqueness tracking, none of XksKeyNotFoundException/XksKeyAlreadyInUseException/XksKeyInvalidConfigurationException (all three appear in CreateKey's own error list). CreateKey rejects any attempt to create a key in an EXTERNAL_KEY_STORE-type custom key store with UnsupportedOperationException rather than accepting a request it cannot honor. Deliberate scope decision, not an oversight -- implementing this is a real feature (see gopherstack-ufvn), not a linkage bug."
  - "RESOLVED 2026-09-07 (gopherstack-o3rp): confirmed and fixed. DisconnectCustomKeyStore's own doc comment (kms@v1.55.4 api_op_DisconnectCustomKeyStore.go, quoted in full): 'While a custom key store is disconnected, all attempts to create KMS keys in the custom key store or to use existing KMS keys in [cryptographic operations] will fail.' Error choice was NOT CustomKeyStoreInvalidStateException (that type's own doc enumerates only CreateKey/ConnectCustomKeyStore/DisconnectCustomKeyStore/UpdateCustomKeyStore/DeleteCustomKeyStore/GenerateRandom preconditions -- no crypto op) but KMSInvalidStateException, whose own doc says explicitly: 'For cryptographic operations on KMS keys in custom key stores, this exception represents a general failure with many possible causes.' Extraction of all twelve crypto ops' deserializeOpError functions (Encrypt, Decrypt, ReEncrypt, GenerateDataKey/WithoutPlaintext/Pair/PairWithoutPlaintext, Sign, Verify, GetPublicKey, GenerateMac, VerifyMac, DeriveSharedSecret -- kms@v1.55.4 deserializers.go) confirms every one declares KMSInvalidStateException and none declares CustomKeyStoreInvalidStateException, so the existing ErrKeyInvalidState sentinel (already mapped in kmsErrorTable, no new row needed) was reused rather than adding a new one. Confirmed DisconnectCustomKeyStore itself is unguarded against live keys (only DeleteCustomKeyStore refuses while keys reference the store), so the disconnected-with-live-keys case this issue depends on is reachable. Fix: all twelve crypto ops fetch key material through one shared helper, requireKeyMaterial(region, key) in store.go -- confirmed by grep before touching anything, not assumed -- so the guard was added there once (check Key.CustomKeyStoreID's backing store ConnectionState before returning material) rather than duplicated at each of the twelve call sites; those call sites only changed to pass the already-in-scope *Key instead of a bare keyID string. Regression test TestEncrypt_DisconnectedCustomKeyStore_WireErrorType drives the full HTTP handler (create+connect a store, create a key in it, disconnect, Encrypt) and asserts the JSON Type field is KMSInvalidStateException; confirmed pre-fix by neutering the new guard block in requireKeyMaterial, which makes the test fail with 200 OK instead of 400/KMSInvalidStateException."
  - "RESOLVED 2026-09-07 (gopherstack-akm2): confirmed. ConnectCustomKeyStore/DisconnectCustomKeyStore/DeleteCustomKeyStore's own pre-existing state-transition guards (custom_key_stores.go) all returned ErrKeyInvalidState (KMSInvalidStateException), but raw error extraction of all four ops' deserializeOpError functions (kms@v1.55.4 deserializers.go) shows CustomKeyStoreInvalidStateException declared and KMSInvalidStateException declared by NONE of them -- confirmed by types/errors.go's CustomKeyStoreInvalidStateException doc comment, which lists exactly ConnectCustomKeyStore/DisconnectCustomKeyStore/UpdateCustomKeyStore/DeleteCustomKeyStore's non-connected/non-disconnected preconditions as its own reasons for existing. Fixed: all three call sites now use the existing ErrCustomKeyStoreInvalidState sentinel (added by gopherstack-e76y for CreateKey's check, previously unused by these three pre-existing guards). Also fixed a second, dependent gap this exposed: ErrCustomKeyStoreInvalidState was itself absent from handler.go's kmsErrorTable, so even CreateKey's existing use of it fell through to the generic 500 KMSInternalException default instead of the real 400 CustomKeyStoreInvalidStateException -- added the missing table entry (both CustomKeyStoreInvalidStateException and KMSInvalidStateException are ErrorFault: Client in the real SDK, so the HTTP status stays 400 either way; only the JSON error Type field was wrong). UpdateCustomKeyStore is NOT touched by this fix -- it has no ConnectionState guard at all in this backend (a separate, pre-existing feature gap, not a wrong-code bug; not added here to stay in scope). ErrCustomKeyStoreHasKeys (DeleteCustomKeyStore's separate 'store still has keys' check) is also absent from kmsErrorTable, same failure mode as the CreateKey gap above -- left unfixed, out of this issue's scope, noted for a future pass."
  - "RESOLVED 2026-09-07 (gopherstack-ylkc): the ErrCustomKeyStoreHasKeys gap flagged (but left unfixed) by gopherstack-akm2 above. DeleteCustomKeyStore's still-has-keys guard (custom_key_stores.go) returns ErrCustomKeyStoreHasKeys (CustomKeyStoreHasCMKsException, added by gopherstack-e76y) but had no row in handler.go's kmsErrorTable, so it fell through the linear errors.Is scan to the generic 500 KMSInternalException default instead of the real 400 CustomKeyStoreHasCMKsException. Extraction of deserializeOpErrorDeleteCustomKeyStore (kms@v1.55.4 deserializers.go) confirms the op declares it: UnknownError, CustomKeyStoreHasCMKsException, CustomKeyStoreInvalidStateException, CustomKeyStoreNotFoundException, KMSInternalException. types/errors.go confirms CustomKeyStoreHasCMKsException.ErrorFault() is smithy.FaultClient, so the fix is a 400, not the pre-fix 500. Fixed: added the missing table row. A full sweep of every sentinel in errors.go against kmsErrorTable (comm -23 on both lists) found exactly this one omission and no others -- the akm2 CustomKeyStoreInvalidState gap it fixed earlier this session was the only sibling, and is already resolved above. Regression test TestDeleteCustomKeyStore_HasKeys_WireErrorType drives the full HTTP handler and asserts the JSON Type field (not just ErrorIs on the sentinel -- an ErrorIs-only assertion is exactly how both this gap and the akm2 one went undetected: TestDeleteCustomKeyStore_WithLinkedKey_Rejected in custom_key_store_link_test.go calls the backend directly and asserts only require.Error + assert.ErrorIs, which passes identically whether or not the table row exists). Confirmed pre-fix: reverting the table row makes the new test fail with 500/KMSInternalException instead of 400/CustomKeyStoreHasCMKsException."
deferred:
  - Custom key store cryptographic connection/HSM simulation (ConnectCustomKeyStore is a pure state-machine transition; no CloudHSM cluster or XKS proxy is modeled, matching pre-existing scope). Re-audited 2026-07-23, still accurate -- no change.
  - "REMOVED 2026-07-23: GetKeyLastUsage was listed here as 'not a real AWS KMS operation'. That was wrong on every prior pass -- see the GetKeyLastUsage ops row above. It is now field-diffed and current."
leaks: {status: fixed, note: "Handler.tags (a side map of *tags.Tags keyed by KeyID, entirely outside InMemoryBackend -- see the TagResource/UntagResource/ListResourceTags ops rows for why tags live at the handler layer here) was never cleaned up when the janitor permanently purged a key. Every other per-key index the janitor purges (aliases, grants, lastUsage, and the grantsByKey secondary index fixed in a prior pass) lives inside InMemoryBackend and was already cascade-cleaned by purgeKey; Handler.tags structurally could not be, since Janitor only holds a *InMemoryBackend, not a *Handler. Since KMS key IDs are UUIDs that are never reused, an unfixed regression here leaks one *tags.Tags -- AND the lockmetrics/Prometheus collector registration it owns (see pkgs/tags.Tags.Close's doc comment: 'prevent unbounded growth of the global collector') -- per tagged-then-deleted key for the remaining lifetime of any long-running gopherstack process. Fixed by adding a Janitor.OnKeyPurged(region, keyID string) callback, invoked synchronously at the end of purgeKey (still under the backend's write lock; safe because Handler.tagsMu is never held while calling back into Backend anywhere in this package -- verified by reading every tagsMu-holding code path in handler_tags.go and persistence.go), and wired in Handler.WithJanitor to h.purgeTags, which Close()s and deletes the map entry. Verified by TestTagsLeak_PurgeKey in leak_test.go with a negative-control run (test fails with the exact leaked-tag symptom when the OnKeyPurged wiring is disabled, passes with it restored). All other maps confirmed still bounded (keyMaterialHistory capped at 100 entries/key, janitor sweeps PendingDeletion keys, resolution cache cleared via evictAliasesFromCache, grantsByKey dropped in purgeKey)."}
---

## Notes

Freeform findings from the 2026-07-05 sweep (bd: gopherstack-42s), for the next auditor.

### Fixed this pass

1. **Grant-token wire gap on 7 operations (severe, disguised stub).** `SignInput`,
   `VerifyInput`, `GetPublicKeyInput`, `DeriveSharedSecretInput`, `GenerateDataKeyPairInput`,
   `GenerateDataKeyPairWithoutPlaintextInput`, `GenerateMacInput`, and `VerifyMacInput` had no
   `GrantTokens` field at all, even though `isValidGrantOperation` in backend.go already lists
   `Sign`, `Verify`, `GetPublicKey`, `GenerateMac`, `VerifyMac`, `DeriveSharedSecret`,
   `GenerateDataKeyPair`, and `GenerateDataKeyPairWithoutPlaintext` as grantable operations.
   Since dispatch does a bare `json.Unmarshal(body, &input)`, a caller-supplied `GrantTokens`
   array was silently dropped on the floor — the grant system modeled these operations as
   grantable but never actually validated a grant token for them. Confirmed against
   `aws-sdk-go-v2/service/kms` that all 8 real `*Input` structs carry `GrantTokens []string`.
   Fixed by adding the field to all 8 structs and wiring validation into the backend:
   - Per AWS docs (`kms/types.GrantConstraints` doc comment), `EncryptionContextEquals`/
     `EncryptionContextSubset` constraints apply ONLY to operations that support an
     encryption context (Encrypt, Decrypt, GenerateDataKey(WithoutPlaintext),
     GenerateDataKeyPair(WithoutPlaintext), ReEncryptFrom/To) — NOT to Sign, Verify,
     GetPublicKey, GenerateMac, VerifyMac, or DeriveSharedSecret. So the fix adds two
     different helpers: `validateGrantTokenPresence` (token must exist + be unexpired; no
     constraint check) for the six non-context ops, and reuses the existing
     `validateGrantTokenConstraints` (token + TTL + EncryptionContext match) for
     GenerateDataKeyPair(WithoutPlaintext), matching the pattern already used by
     Encrypt/Decrypt/GenerateDataKey/ReEncrypt.
   - **Trap for the next auditor:** do not "simplify" by reusing
     `validateGrantTokenConstraints` with a nil encryption context for Sign/Verify/etc. — if
     a grant happens to have `EncryptionContextEquals` set (unusual but not rejected at
     CreateGrant time), that would spuriously reject Sign/Verify calls that AWS would allow,
     since AWS never evaluates that constraint for those operations.
   - Also added `validateEncryptionContextSize` to `GenerateDataKeyPair`, which was missing
     it (Encrypt/Decrypt/GenerateDataKey/ReEncrypt already had it).

2. **Invalid KeySpec/KeyPairSpec misclassified as 500, not 400 (moderate).**
   `generateKeyMaterial`'s `default:` case returned an error wrapping only
   `errUnsupportedKeySpec`, never `ErrValidation`. `classifyKMSError` only matches via
   `errors.Is` against the sentinels in `kmsErrorTable()`, so `CreateKey` with a garbage
   `KeySpec` (e.g. a typo) and `GenerateDataKeyPair` with a garbage `KeyPairSpec` both fell
   through to the default `InternalServiceError` / 500 branch instead of
   `ValidationException` / 400 — exactly bug class #2 from `parity-principles.md`
   ("missing errCodeLookup entries"). Fixed with a single-point fix: wrap with both
   `ErrValidation` and `errUnsupportedKeySpec` (Go 1.20+ multi-`%w`) at the source, so every
   current and future caller of `generateKeyMaterial` gets correct classification for free.
   Proven with an HTTP-level test (`TestKMS_InvalidKeySpec_Returns400UnsupportedOperationException`)
   that exercises the full `Handler().Handler()` echo path and checks both status code and
   `ErrorResponse.Type`.
   **Update (gopherstack-e3yu):** `ValidationException` names no type in any KMS
   operation's `deserializeOpError` (CreateKey and GenerateDataKeyPair included), so the
   wrap now uses `ErrUnsupportedParameter` (`UnsupportedOperationException`, which both
   operations' real deserializers do recognize) instead of `ErrValidation`; still 400.

3. **`purgeKey` leaked the `grantsByKey` secondary-index submap (moderate, matches the
   "unbounded key/grant maps" pattern called out in the audit brief).** When the janitor
   permanently purges a key past its `ScheduleKeyDeletion` window, it deleted the key's
   grants from `grants` and `grantsByToken` but never removed
   `grantsByKey[region][keyID]`. Since the purged keyID can never be looked up again, that
   submap (and any residual per-grant map beneath it) is unreachable for the remainder of
   the process's lifetime — a genuine memory leak in any long-running instance that
   repeatedly creates keys with grants and lets them expire. Fixed by dropping the submap in
   `purgeKey`; `grantsByKeyStore` lazily recreates it on next access if the key ID is ever
   reused (it won't be, since key IDs are UUIDs, but the accessor is already written to
   tolerate that). Note `rebuildGrantIndexesLocked` (used by `Restore`) already rebuilds
   `grantsByKey` from scratch, confirming it's a pure derived index safe to drop.

4. **`CreateKey` and `ReplicateKey` created a real, permanent resource before validating
   tags (moderate, orphan-resource leak).** `createKeyAction` called
   `Backend.CreateKey` (allocating a UUID, real key material, and a backend map entry)
   and only validated `input.Tags` afterward via `applyInputTags`. If a tag was malformed
   (empty key, reserved `aws:` prefix, over-length), the handler returned an error to the
   caller — who never receives a `KeyId` — while the key remained permanently resident in
   the backend, discoverable only via `ListKeys`, with no route to ever tag, use, or delete
   it by the caller that "created" it. `ReplicateKey`'s dispatch closure was worse: it never
   validated tags at all (`copyTagsToReplica` calls `setTags` directly, bypassing
   `validateTag` entirely), so a malformed replica tag would just silently apply. Real AWS
   validates the whole request shape before creating any resource. Fixed by extracting a
   shared `validateTags` helper and calling it before `Backend.CreateKey` /
   `Backend.ReplicateKey` in both dispatch paths (the `ReplicateKey` closure was also
   extracted to `replicateKeyAction` to keep `buildReplicationAndMaintenanceActions` under
   the gocognit complexity gate).

### Traps / already-correct patterns confirmed (do not re-flag)

- `Decrypt`/`ReEncrypt` returning an error via `decryptWithHistory` after the primary
  `decryptData` attempt fails is NOT a stub — it's the real post-rotation fallback path that
  tries prior key-material versions (capped at `maxKeyMaterialHistoryEntries` = 100) before
  giving up with `InvalidCiphertextException`.
- `GetKeyLastUsage` is not a real AWS KMS API (AWS doesn't expose per-key last-usage via a
  named operation); it was added in an earlier pass as an internal telemetry accessor and is
  kept as deferred/non-blocking scope.
- `errCodeLookup`-equivalent (`kmsErrorTable()` + `classifyKMSError`) returns HTTP 400 for
  every matched sentinel including `NotFoundException` — this matches real AWS KMS
  (a `json-1.1` protocol service), which returns 400 Bad Request for `NotFoundException`
  too, not 404.
- `CreateGrant`'s own `GrantTokens` field (authorizing the CreateGrant call via an existing
  grant) is intentionally NOT modeled — there is no IAM/authorization layer anywhere in this
  mock, so it would be a no-op; this is a scope boundary, not a bug.

### Verification method

Every fix in this pass was proven with a negative control: the corresponding fix commit was
reverted locally, the new test was confirmed to fail (or fail to compile, for the wire-shape
field additions) against the reverted code, then the fix was reapplied and the full suite
re-run green. See `parity_sweep3_test.go`.

## 2026-07-11 re-audit (bd: none filed yet — see report)

Per the re-audit protocol: `git diff ede7169a..eb94f3c3 -- services/kms/` showed only the
sweep-3 commit itself touching this service (no drift since the ledger above was written),
and `aws-sdk-go-v2/service/kms` bumped v1.53.6 -> v1.54.0 with only "Add request
serialization snapshot tests" in the changelog (no new ops/fields). So this pass audited the
`kmsErrorTable()` / `classifyKMSError` machinery specifically (an area the sweep-3 notes
already flagged as a recurring bug class) rather than re-walking every already-`ok` row.

### Fixed this pass

1. **Two sentinel errors missing from `kmsErrorTable()` (moderate, same bug class as
   sweep 3's `ValidationException` gap).** `ErrExpiredKeyMaterial` (raised by
   `checkKeyMaterialExpiry`, reachable from `Encrypt`/`Decrypt`/etc. on a key whose
   imported material has passed its `ValidTo`) and `ErrKeyMaterialUnavailable` (raised by
   `requireKeyMaterial` when key material is absent, e.g. after restoring an old-format
   snapshot) both had no entry in the table, so `classifyKMSError` fell through to the
   generic default branch and returned a 500 for both — even though `ErrExpiredKeyMaterial`
   is a genuine 400 client-fault scenario (bad/expired import, not a server problem). Fixed
   by adding both to the table: `ErrExpiredKeyMaterial` -> `ExpiredImportTokenException`
   (400, confirmed a client-fault exception in the real SDK), `ErrKeyMaterialUnavailable`
   -> `KeyUnavailableException` (500, confirmed `ErrorFault: Server` in the real SDK).
   Required extending `kmsErrorEntry` with an optional `httpStatus` field (0 = default 400)
   since this is the first table entry that isn't a plain client-fault 400.
2. **The default/unclassified-error branch emitted a type string that isn't a real KMS
   exception (minor but structural).** `classifyKMSError`'s fallback returned
   `"InternalServiceError"`, which does not appear anywhere in
   `aws-sdk-go-v2/service/kms/types/errors.go` — the real type for an unclassified
   server-side failure is `KMSInternalException`. A caller doing
   `errors.As(err, &types.KMSInternalException{})` (a real, documented pattern for
   distinguishing retryable server errors) would never match against this emulator's
   output. Fixed by changing the fallback's type string to `"KMSInternalException"`.
   Verified no test asserted the literal string `"InternalServiceError"` (only comments
   did) before making the change.

Both fixes proven with the same negative-control method as sweep 3: see
`Test_KMS_ErrorClassification_MissingTableEntries` in `parity_sweep3_test.go` — reverting
`handler.go` alone (via `git stash`) was confirmed to fail both subtests with
`InternalServiceError` in place of the expected exception type, then the fix was reapplied
and the full suite re-run green.

No other gaps found this pass. The three deferred items and the previously-fixed leak in the
`ops`/`gaps`/`leaks` block above are unchanged and still accurate.

## 2026-07-12 Terraform-lifecycle-focused re-audit (bd: none filed yet — see report)

Per the re-audit protocol: `git diff eb94f3c3..HEAD -- services/kms/` showed two commits
since the last ledger entry: `d9cb5d10` (the error-classification fix already recorded
above) and `42cff5ce` ("fix(kms): CreateKey now honors inline Policy (Terraform
GetKeyPolicy hang)" — CreateKey dropped the inline `Policy` field entirely, so
`GetKeyPolicy` always returned the synthesized default and Terraform's `aws_kms_key`
polled to a 10-minute timeout; already fixed before this pass started, per the task
background). This pass's brief: hunt for *more* bugs in the same two classes (AWS wire
parity, and LocalStack/Terraform read-after-write behavioral parity) that would break a
full `terraform apply`/`plan`/`destroy` cycle, since the CreateKey bug proved more exist.

### Fixed this pass

1. **`ReplicateKeyInput` was missing the `Policy` field entirely — the exact same bug
   class as the already-fixed CreateKey regression (severe, confirmed Terraform-breaking).**
   Confirmed against the real `aws-sdk-go-v2/service/kms@v1.54.0` vendored source
   (`api_op_ReplicateKey.go`, found via the Go module cache): the real `ReplicateKeyInput`
   carries `Policy *string`, and the field's doc comment is explicit that "The key policy
   is not a shared property of multi-Region keys... KMS does not synchronize this
   property" — i.e. a replica does NOT inherit the primary's policy; it needs its own,
   supplied via this field or defaulted. gopherstack's `ReplicateKeyInput` had no `Policy`
   field at all, so `Backend.ReplicateKey` silently dropped any inline policy a caller
   supplied, and `GetKeyPolicy` on the replica always returned the synthesized default.
   Since Terraform's `aws_kms_replica_key` resource sets an inline `policy` argument via
   `ReplicateKeyInput.Policy` and then polls `GetKeyPolicy` after apply until the
   configured policy propagates (identical read-after-write pattern to `aws_kms_key`),
   this would have caused the **exact same 10-minute poll-timeout hang** as the
   already-fixed CreateKey bug, just on the replica-key resource instead of the primary.
   Fixed by adding `Policy` (plus `BypassPolicyLockoutSafetyCheck`, present on the real
   input but a no-op here just like CreateKey's own copy of that field, since there is no
   IAM layer) to `ReplicateKeyInput`, validating it with the same `validKeyPolicyDoc`
   helper CreateKey/PutKeyPolicy already share (rejecting a malformed policy with
   `MalformedPolicyDocumentException` *before* creating the replica, consistent with the
   orphan-resource-leak fix from sweep 3), and persisting it into the replica's
   region-scoped `policiesStore` exactly like CreateKey does. See
   `backend.go`'s `ReplicateKey` and `models.go`'s `ReplicateKeyInput`.
   Proven by `Test_ReplicateKey_InlinePolicy` in `replicate_key_policy_test.go`, routed
   through the full HTTP handler with per-region requests (the replica lives in a
   different region than the primary) — table-driven: policy round-trips verbatim,
   defaults correctly when omitted, is rejected as malformed before any replica is
   created, and does not leak onto the primary's own policy (proving the "not a shared
   property" semantics).

2. **KMS resource tags were never included in `Handler.Snapshot`/`Restore` — a real
   persistence gap, exactly the perpetual-`terraform-plan`-drift bug class this sweep's
   brief called out as highest priority (moderate; only manifests across a process
   restart with persistence enabled).** Unlike most other gopherstack services, which
   embed `*tags.Tags` directly in their resource struct (see
   `.claude/memories/pkgs-catalog.md`'s tags entry), KMS applies tags at the *handler*
   layer: `createKeyAction`/`tagResource`/`replicateKeyAction` all write into
   `Handler.tags`, a side map keyed by KeyID, entirely separate from
   `InMemoryBackend`'s own state. `Handler.Snapshot` previously delegated straight to
   `Backend.Snapshot(ctx)` and returned those bytes verbatim — `Handler.tags` was never
   serialized at all. `ListResourceTags` kept returning tags correctly within a single
   running process (masking the gap in every same-process test, including the existing
   `create_key_policy_test.go`-style tests), but any gopherstack restart with persistence
   enabled would silently drop every KMS key's tags — the next `terraform plan` after a
   restart would show a permanent diff on the `tags` attribute of every `aws_kms_key`/
   `aws_kms_replica_key` resource, forever, since there'd be no way to reconcile it short
   of a real `TagResource` call. This is precisely the kind of gap the audit brief
   highlighted ("verify these newly-touched fields are included in the snapshot/restore
   round trip") applied retroactively to an *existing*, previously undetected gap rather
   than a field I was actively adding. Fixed: `Handler.Snapshot` now wraps the backend's
   own snapshot bytes together with a tags map into a small `handlerSnapshot` envelope
   (`persistence.go`), stamped with a `handlerFormat` marker. `Handler.Restore` peeks that
   marker to distinguish the new wrapped shape from a *legacy* snapshot (raw backend
   bytes with no wrapper at all — exactly what `Handler.Snapshot` produced before this
   fix) so an existing on-disk snapshot taken before this fix still restores backend
   state cleanly instead of erroring out; it just won't have tags to restore (no worse
   than the pre-fix status quo). Proven by `TestHandlerSnapshotRestore_TagsRoundTrip`,
   `TestHandlerSnapshotRestore_EmptyTagsOmitted`, and
   `TestHandlerRestore_LegacyBackendOnlySnapshot` in `handler_tags_persistence_test.go`.

### Audited and confirmed correct against aws-sdk-go-v2/service/kms@v1.44.7 (verified via tests in handler_tags_persistence_test.go, handler_keys_test.go)

- **CreateKey's inline `Policy` fix (the task's background bug) is solid**: verified the
  fix persists the policy into `policiesStore` before returning, and that `GetKeyPolicy`
  returns it verbatim on every subsequent call (not regenerated), including across two
  consecutive `DescribeKey`/`GetKeyPolicy`/`ListResourceTags`/`GetKeyRotationStatus`
  calls in the new `TestTerraformLifecycle_KMSKey` integration test (`assert.JSONEq`
  on the raw HTTP response bytes of back-to-back calls) — no drift.
- **The synthesized default key policy is deterministic**: built from a static string
  template (`policiesStore` miss branch in `GetKeyPolicy`), not regenerated with
  randomized/timestamped content — confirmed identical across repeated calls.
- **Tags round-trip immediately after CreateKey** (within a single process, the common
  case): `createKeyAction` validates tags *before* creating the key (sweep-3 fix, still
  correct) and applies them synchronously in the same request; `ListResourceTags`
  reflects them immediately, no async lag.
- **Aliases**: full `CreateAlias -> ListAliases -> UpdateAlias -> DeleteAlias` round trip
  confirmed correct. `ListAliases` does NOT synthesize `alias/aws/*` AWS-managed entries
  — confirmed intentional (no AWS-managed-key simulation anywhere in this mock, a
  pre-existing scope boundary, not a gap) rather than an oversight. `DeleteAlias` is
  correctly non-idempotent (`NotFoundException` on double-delete), matching real AWS.
- **Grants**: `CreateGrant` returns `GrantId`+`GrantToken`; `ListGrants` (with its
  `GrantId` filter, matching the real SDK's `ListGrantsInput.GrantId`) shows the new
  grant immediately, matching how `aws_kms_grant`'s refresh re-reads a specific grant;
  `RetireGrant`/`RevokeGrant` remove it immediately (verified via the pre-existing
  `GrantIndexesConsistent` test helper).
- **ScheduleKeyDeletion/CancelKeyDeletion/EnableKey/DisableKey**: all state transitions
  (`KeyState`, `Enabled`, `DeletionDate`, `PendingDeletionWindowInDays`) apply
  synchronously with no async lag; `DescribeKey` reflects them on the very next call.
  Proven end-to-end (not just at the backend layer) by
  `TestTerraformLifecycle_KMSKey`'s `ScheduleKeyDeletion -> DescribeKey` sequence.
- **EnableKeyRotation/DisableKeyRotation/GetKeyRotationStatus**: round-trip exactly;
  default `KeyRotationEnabled` is `false` and stable across repeated
  `GetKeyRotationStatus` calls (byte-identical JSON, asserted via `assert.JSONEq` in the
  new lifecycle test).
- **Replica keys (`ReplicateKey`/`DescribeKey`)**: `MultiRegionConfiguration`
  (`PRIMARY`/`REPLICA` role + `PrimaryKey`/`ReplicaKeys` list) already correct on both
  sides prior to this pass (per sweep-3's `TestUpdatePrimaryRegion_RoleSwap`); this
  pass's only replica-side finding was the missing `Policy` field (fixed above).
- **DescribeKey's `KeyMetadata` shape**: `KeyState`, `KeyManager` (hardcoded `"CUSTOMER"`,
  correct — this mock never creates AWS-managed keys), `MultiRegion`,
  `MultiRegionConfiguration`, `PendingDeletionWindowInDays`, `DeletionDate`, `ValidTo`,
  `Origin`, `KeySpec` AND the deprecated `CustomerMasterKeySpec` alias (always mirrors
  `KeySpec`), `EncryptionAlgorithms`, `SigningAlgorithms`, `Enabled` — all present and
  correct; cross-checked field-by-field against the real vendored
  `aws-sdk-go-v2/service/kms@v1.54.0/types.KeyMetadata`.
- **Tag/grant/policy wire shapes** (`TagResource`/`UntagResource`/`ListResourceTags`
  inputs+outputs, `ListGrantsInput`/`CreateGrantInput`/`GrantListEntry`) — all
  field-for-field checked against the vendored real SDK; no casing or shape gaps.
  gopherstack-ioxy: CORRECTED — this entry previously argued that `ListGrants`
  echoing `GrantToken` on every entry was a "harmless superset (unknown extra JSON
  fields are ignored)". The premise is true but the conclusion was wrong: this
  isn't an inert extra field, it's a bearer credential. Real AWS's `GrantListEntry`
  (kms@v1.55.4 `types/types.go:308`, deserialized field-by-field in
  `deserializers.go:9430` `awsAwsjson11_deserializeDocumentGrantListEntry`) has no
  `GrantToken` member at all, by design: a grant token is minted once, in
  `CreateGrantOutput` (`api_op_CreateGrant.go:278`, confirmed same field/value as
  `Grant.GrantToken` set at `CreateGrant` time), specifically so it can't be
  re-read later — it lets the holder exercise a grant's permissions before
  eventual consistency settles. Emitting it from `ListGrants`/`ListRetirableGrants`
  handed out that bearer credential to anyone who could list grants, regardless of
  SDK-client tolerance for unknown keys. Fixed: `ListGrantsOutput.Grants` is now
  `[]GrantListEntry`, a dedicated wire type built by `toGrantListEntry` that drops
  both `GrantToken` and the internal `TokenIssuedAt` bookkeeping field (also not
  part of real `GrantListEntry`); the internal `Grant` storage struct keeps both,
  since `TokenIssuedAt`/`GrantToken` are needed for TTL and token-lookup indexing.
  No consumer in this repo (tests, `cli.go`, UI) read `GrantToken` off a
  `ListGrants`/`ListRetirableGrants` response — every existing reference reads it
  off `CreateGrant`'s own output — so this was a pure emulator-side leak with no
  in-repo caller depending on it.

### Cross-service KMS integration punch-list (Step 4, report-only — no edits made)

**CORRECTION (gopherstack-utfj, verified against current `cli.go`):** Secrets Manager
was listed below as unwired; `wireSecretsManagerKMS` (`cli.go:4551`, called from
`cli.go:3385`) has wired it since commit `efc42cbc4` (2026-07-13), after this Step 4
punch-list and the 2026-07-12 pass's closing paragraph were written and never updated.
Moved to the wired list below. Also missing from this punch-list entirely: **Kinesis**
(`wireKinesisKMS`, `cli.go:4502`, called from `cli.go:2876`) real-validates
`StartStreamEncryption`'s `KeyId` via `kms.InMemoryBackend.DescribeKey` — this Step 4
search's `KmsKeyId`/`KMSMasterKeyId`/`SSEKMSKeyId` field-name grep didn't match
Kinesis's bare `KeyId` field, so it was never enumerated either way. Added below. The
remaining "not wired" entries (S3, SQS, SNS, DynamoDB, RDS, EC2/EBS, CloudWatch Logs)
were re-checked against current `cli.go` and still have no `wire*KMS` counterpart —
still accurate.

Searched the full non-KMS codebase (read-only) for `KmsKeyId`/`KMSMasterKeyId`/
`SSEKMSKeyId`-style fields. **Already wired in `cli.go` (no gap):**

- **SSM** (`wireSSMKMS` in `cli.go:3542`): `ssm.InMemoryBackend.WithKMS` +
  `ssm.KMSEncryptor` interface + a `cli.go`-local `ssmKMSAdapter` calling
  `kms.InMemoryBackend.Encrypt`/`Decrypt` directly. SecureString `Parameter`s whose
  `KeyId` is set get *real* KMS encryption already. No action needed.
- **Resource Groups Tagging API** (`wireTaggingKMS` in `cli.go:5218`): uses
  `Handler.TaggedKeys`/`TagKeyByARN`/`UntagKeyByARN` (already exported on
  `kms.Handler` specifically for this). No action needed.
- **Kinesis** (`wireKinesisKMS` in `cli.go:4502`): `kinesis.InMemoryBackend.WithKMSValidator`
  + `kinesis.KMSKeyValidator` interface + a `cli.go`-local `kinesisKMSAdapter` calling
  `kms.InMemoryBackend.DescribeKey` to real-validate `StartStreamEncryption`'s `KeyId`
  (existence + `Enabled` state). No action needed.
- **Secrets Manager** (`wireSecretsManagerKMS` in `cli.go:4551`, called from `cli.go:3385`):
  `secretsmanager.InMemoryBackend.SetKMSEncryptor` + `secretsmanager.KMSEncryptor`
  interface (`services/secretsmanager/kms.go`) + a `cli.go`-local
  `secretsManagerKMSAdapter` calling `kms.InMemoryBackend.Encrypt`/`Decrypt`, mirroring
  the SSM precedent. `SecretString`/`SecretBinary` are now *really* encrypted/decrypted
  through KMS when a secret has a `KmsKeyId` (or the default alias). No action needed —
  see the corrected entry below, formerly listed under "Not wired."
- **CloudFormation** (`services/cloudformation/resources.go`,
  `resources_phase5.go`/`resources_phase6.go`): `AWS::KMS::Key`/`AWS::KMS::Alias`/
  `AWS::KMS::ReplicaKey` CFN resource types call `rc.backends.KMS.Backend.CreateKey`/
  `CreateAlias`/`DeleteAlias`/`ReplicateKey`/`ScheduleKeyDeletion` directly via the
  already-exported `Handler.Backend` (`StorageBackend`) field. This is CFN driving KMS
  natively (not really a "consumer" relationship) but confirms the integration point
  already works end-to-end. No action needed.

**Not wired (gap, but confirmed pro-tier-enforcement-only, not needed for a basic
`terraform apply`/`plan`/`destroy` to succeed):** none of the below call into the `kms`
package at all today; each just stores/echoes the KMS key ID string on its own resource
with no existence check, no alias resolution beyond what the field already is, and no
real encrypt/decrypt. For every one of these, **Terraform's own resource lifecycle only
needs the field to round-trip on the owning service's Create/Describe/Update calls**
(that service's own attribute-round-trip correctness is that service's PARITY.md
concern, not KMS's) — actually calling into KMS is additive Pro-tier realism, not a
correctness requirement for `apply`/`plan`/`destroy` to succeed:

  - **S3** (`aws_s3_bucket_server_side_encryption_configuration`,
    `kms_master_key_id`/`SSEKMSKeyId` in `backend_memory.go`/`multipart_ops.go`/
    `object_ops.go`/`types.go`): stored per-object/version, never validated or used to
    actually encrypt object bytes at rest. (a) existence check: no. (b) alias
    resolution: no (stores whatever string was given). (c) real encrypt/decrypt: no. (d)
    needed for basic apply: no.
  - **SQS** (`aws_sqs_queue`, `KmsMasterKeyId`/`KmsDataKeyReusePeriodSeconds` queue
    attributes in `types.go`/`backend.go`): stored/echoed only, format-unvalidated. Same
    (a)-(d) answers as S3.
  - **SNS** (`aws_sns_topic`, `KmsMasterKeyId` topic attribute in `backend.go`): DOES
    format-validate the value (alias name / alias ARN / key ID / key ARN shape) but
    never checks it against a live KMS key and never actually encrypts messages. (a) no
    existence check (format-only). (b)/(c) no. (d) not needed for basic apply.
  - ~~**Secrets Manager**~~ **WIRED as of `efc42cbc4` (2026-07-13) — see the corrected
    "Already wired" entry above (gopherstack-utfj).** This bullet's original claim
    (stored/echoed only, never validated or encrypted) is stale; left struck through
    rather than deleted so this pass's punch-list stays legible as a historical record.
  - **DynamoDB** (`aws_dynamodb_table`, `SSESpecification.KMSMasterKeyId` ->
    `SSEKMSMasterKeyArn` in `table_ops.go`): stored/echoed only. Same (a)-(d) as S3.
  - **RDS** (`aws_db_instance`/`aws_db_snapshot`, `KmsKeyId` throughout
    `handler.go`/`batch1.go`): stored/echoed only. Same (a)-(d) as S3.
  - **EC2/EBS** (`aws_ebs_volume`/`aws_ebs_default_kms_key`, `KmsKeyID` in
    `backend_accuracy.go`/`backend_batch1.go`/`backend_batch3.go`): stored/echoed only,
    plus an account-level default-KMS-key setting (`GetEbsDefaultKmsKeyID`/
    `ResetEbsDefaultKmsKeyID`) that's also just a stored string. Same (a)-(d) as S3.
  - **CloudWatch Logs** (`aws_cloudwatch_log_group`, `KmsKeyId` in
    `backend.go`/`handler.go`/`models.go`): stored/echoed only. Same (a)-(d) as S3.

**No new KMS-side export is needed for any future cli.go wiring of the services above.**
The integration surface already exists and is already exercised by the SSM/CloudFormation
precedents: `kms.Handler.Backend` is already a public field typed as the `StorageBackend`
interface (exactly how `wireSSMKMS` and the CloudFormation resource handlers obtain a
concrete `*kms.InMemoryBackend` today), and that interface's already-public
`DescribeKey`/`Encrypt`/`Decrypt` methods are sufficient for a future adapter to (a)
check key existence (`DescribeKey` + `errors.Is(err, kms.ErrKeyNotFound)`, both exported),
(b) resolve an alias or ARN to a key ID (`DescribeKey` already accepts `alias/...`/
`arn:...`/bare-UUID interchangeably in its `KeyId` field and returns the canonical
`KeyMetadata.KeyID`), and (c) perform real encrypt/decrypt passthrough (`Encrypt`/
`Decrypt`), exactly mirroring `ssmKMSAdapter`'s three-method shape. Wiring any of the
services above is pure `cli.go` + that service's own backend work, per
`PARITY_PHASE4_KICKOFF.md`'s "cross-service interconnect wired in cli.go, main-thread
work" rule — out of scope for this KMS-only pass.

## 2026-07-12 gap-closure pass (in-scope follow-ups from the lifecycle re-audit)

Closed the two remaining in-scope gaps the Terraform-lifecycle re-audit above had
identified and deferred. Both are now `RESOLVED` in the `gaps` block; the op rows carry
the details.

1. **`DescribeKeyInput.GrantTokens` added (wire parity + AWS-accurate validation).** The
   real `aws-sdk-go-v2/service/kms@v1.54.0` `DescribeKeyInput` carries `GrantTokens
   []string` and the op declares `InvalidGrantTokenException` in its error set, so
   validation here is meaningful (unlike the still-deferred `CreateGrantInput.GrantTokens`,
   which authorizes the CreateGrant call itself and has nothing to check without an IAM
   layer). Wired `validateGrantTokenPresence` (existence + TTL, no encryption-context
   check), consistent with Sign/Verify/GetPublicKey/DeriveSharedSecret. Empty tokens is a
   no-op — the only case Terraform exercises. Tests: `describe_key_grant_tokens_test.go`.

2. **Region-scoped KeyId resolution made consistent across all region-partitioned ops.**
   `GetKeyPolicy`/`PutKeyPolicy`/`CreateGrant`/`ListGrants`/`RevokeGrant`/`RetireGrant` now
   index their `policiesStore`/`grantsRegion` using the key's OWN region (an ARN's embedded
   region for an ARN input), the same region-awareness `DescribeKey`/`Encrypt`/`Decrypt`
   already had via `lookupKey`. Two root causes, both fixed at source in `backend.go`:
   - Added `resolveKeyAndRegion(ctx, keyID) (*Key, region, error)` — resolves the key AND
     reports the region it actually lives in (with the same bare-UUID all-region fallback
     `lookupKey` had). `lookupKey` now delegates to it (returns just the key). The six ops
     route through it instead of `getRegion(ctx)` + `resolveKeyID` + `keysStore(region)`.
   - **Deeper bug the cross-region test surfaced:** `resolveKeyID`'s resolution cache stored
     only the resolved UUID and returned the *request* region on every cache hit — so the
     2nd+ resolution of any ARN lost the ARN's embedded region even for callers that did
     use it. Fixed by caching a `{keyID, region}` pair: `region=""` sentinel for aliases
     (means "derive from request context" — alias behavior unchanged, and safe because a
     bare `alias/name` cache key carries no region), and the ARN's own embedded region for
     ARN inputs (safe because the region is part of the ARN cache key). This is the piece
     that makes the fix hold across repeated calls, which is exactly the read-after-write
     pattern Terraform drives.
   Alias ops (`CreateAlias`/`UpdateAlias`/`DeleteAlias`/`ListAliases`) were audited and
   deliberately left request-region-scoped: an AWS alias must target a key in its own
   region, so resolving the target/filter against the request region is correct AWS
   behavior, not the same bug. Tests: `region_scoped_resolution_test.go` (cross-region
   `ReplicateKey` → replica-ARN policy round-trip + full grant lifecycle by ARN, table-
   driven over Revoke/Retire-by-ARN/Retire-by-token/Retire-by-GrantId, all while ctx
   defaults to the primary's region).

**UPDATE (gopherstack-utfj, 2026-09-06): done, not remaining.** The cross-service
Secrets Manager → KMS wiring described below landed in `efc42cbc4` (2026-07-13), two
days after this paragraph was written — `wireSecretsManagerKMS` (`cli.go:4551`) is
called from `cli.go:3385`, and `services/secretsmanager/kms.go` implements the real
`KMSEncryptor` adapter (`secretsManagerKMSAdapter` in `cli.go`) exactly as predicted
below. This paragraph was never updated after that commit landed; left in place,
struck through, as a historical record rather than deleted.

~~**Only remaining KMS-related follow-up is external (out of this service's scope):**
the cross-service Secrets Manager → KMS wiring (real `SecretString`/`SecretBinary`
encryption via a `secretsmanager.KMSEncryptor`-style adapter, mirroring the existing
SSM precedent). That is `cli.go` + Secrets Manager backend work — main-thread /
cross-service territory per `PARITY_PHASE4_KICKOFF.md`, and needs no new KMS-side
export (`Handler.Backend`'s `Encrypt`/`Decrypt`/`DescribeKey` already suffice, as
documented in the cross-service punch-list above). No KMS-local gaps remain.~~

## 2026-07-23 gap-closure + leak-hunt pass (bd: none filed yet — see report)

Worked the 5 `gaps` + 2 `deferred` entries this file tracked at the start of the pass,
plus a dedicated leak hunt (this file's `leaks` block said `status: found`, which on a
literal reading meant "not yet fixed" even though its note described an already-applied
fix from a prior pass — resolved by treating it as an instruction to re-audit for a
*current*, unfixed leak, which turned up a real one; see the `leaks` block above).

### Corrected this pass (process finding, not a code bug)

**`GetKeyLastUsage` was mislabeled "not a real AWS KMS operation" by every prior audit
pass on this file, going back to whichever pass first added it.** Caught only because
this pass initially trusted that label and removed the op from the HTTP dispatch table
(`GetSupportedOperations`/`buildDispatchTable` in handler.go) — `TestSDKCompleteness`
immediately failed with `SDK methods found that are neither in GetSupportedOperations()
nor in the notImplemented list: [GetKeyLastUsage]`, which is the completeness test
correctly noticing the real SDK client (`aws-sdk-go-v2/service/kms@v1.54.0`) has a real
`Client.GetKeyLastUsage` method backed by `api_op_GetKeyLastUsage.go`. Reverted the
removal and field-diffed the real op properly instead (see the `ops` row above) — this
is the exact "field-diff wire shapes against the real SDK types... do NOT mark a family
ok on a no-stub basis alone" failure mode this campaign's task brief warns about, just
inverted: trusting an old *removal* note instead of trusting an old *stub* note. Lesson
for the next auditor: `notImplemented`/`deferred` labels claiming an op "isn't real AWS"
are exactly as load-bearing as any other claim in this file and must be independently
re-verified against the vendored SDK, not propagated forward pass after pass.

### Fixed this pass

1. **Leak: `Handler.tags` never cleaned up on permanent key purge.** See the `leaks`
   block above for the full writeup. `janitor.go`'s `Janitor.OnKeyPurged` callback +
   `handler_tags.go`'s `Handler.purgeTags`, wired in `Handler.WithJanitor`. Test:
   `TestTagsLeak_PurgeKey` in `leak_test.go`, with a negative-control run.
2. **`GrantConstraints.SourceArn`, `CreateGrantInput.GrantTokens`,
   `GranteeServicePrincipal`/`RetiringServicePrincipal`** — all three previously-deferred
   gaps closed as real wire-shape + validation fixes (not stubs: the two principal
   fields have genuine, enforced validation rules taken directly from the real SDK's doc
   comments). See the `CreateGrant` ops row above and `gaps` block for the full
   before/after reasoning on why these are closeable without the cross-cutting IAM layer
   that blocks *enforcement* of grant permissions generally. Also added
   `Grant.IssuingAccount` (real `GrantListEntry` field that was entirely absent).
   Tests: `TestCreateGrant_ServicePrincipals`, `TestGrantConstraint_SourceArn_RoundTrips`,
   `TestCreateGrant_IssuingAccount_Populated`, `TestCreateGrant_GrantTokens_AcceptedAsNoOp`
   in `grants_test.go`; persistence coverage added to the existing
   `TestInMemoryBackend_FullStateSnapshotRestoreRoundTrip`.
3. **`GetKeyLastUsage` accepted alias-form `KeyId`, which the real API's doc comment
   explicitly forbids** ("Specify the key ID or key ARN of the KMS key... Alias names
   are not supported" — the one KeyId-accepting KMS op in this codebase with that
   restriction; every other one accepts alias/ARN/bare-ID interchangeably). Fixed with a
   new `isAliasKeyID` helper (`store.go`). Test: `TestGetKeyLastUsage_RejectsAliasKeyID`.

### Newly found, NOT fixed this pass (items_still_open)

1. **`CreateGrantInput.Name`-based retry idempotency is entirely unimplemented.** The
   real SDK's doc comment on `CreateGrantInput.Name` is explicit: "When this value is
   present, you can retry a CreateGrant request with identical parameters; if the grant
   already exists, the original GrantId is returned without creating a new grant...
   the returned grant token is unique with every CreateGrant request, even when a
   duplicate GrantId is returned." gopherstack's `CreateGrant` (`grants.go`) creates a
   brand-new `GrantID` and `GrantToken` on every call regardless of `Name`, with no
   duplicate-detection at all. Not fixed this pass: correctly implementing "same
   GrantId, fresh GrantToken every retry" requires either (a) letting a single stored
   `Grant` hold multiple valid tokens, or (b) some other mechanism to keep an old,
   already-issued token valid across a retry that mints a new one for the same grant —
   both are real storage-model changes to `Grant`/the grants `store.Table`, not a
   same-shape field addition like this pass's other three gap closures, and risked
   destabilizing the grant-token expiry/constraint-checking logic
   (`validateGrantTokenConstraints`/`validateGrantTokenPresence`) under time pressure.
   Left for a dedicated follow-up pass.
2. **`DryRun` is not implemented on any KMS operation.** The real SDK has a `DryRun
   *bool` field on `CreateGrantInput` (and several other KMS inputs). gopherstack
   implements `DryRun` for EC2 (`ec2/handler.go`: validate-then-`ErrDryRunOperation`/412
   pattern) but nowhere in KMS. This is a broad, multi-op feature addition (every
   DryRun-capable KMS op, not just CreateGrant) rather than a single documented gap this
   file was already tracking, so it's out of scope for this pass's 5-gaps/2-deferred
   closure brief. Noted for a future KMS pass; not a regression (nothing broke — DryRun
   was already absent).

Both `items_still_open` above are genuinely new findings (not previously tracked
anywhere in this file), surfaced by the same real-SDK field-diffing this pass applied to
the 5 gaps it was scoped to close — noted here rather than silently left out, per this
campaign's "no bad tests, no reclassifying an unfinished item as ok" rule.

RE-VERIFIED 2026-08-23 (manifest-harvest pass, ranked items 1 and 4 of that pass's queue):
re-read `grants.go`/`store.go` fresh rather than trusting this note. Both deferrals still
hold. (1) `CreateGrant` still mints a brand-new `GrantID`/`GrantToken` on every call
regardless of `Name` -- no dedup lookup by `Name` exists in `CreateGrant` (`grants.go`).
`Grant` (models.go) and its `byToken` index (`store.go`) still store exactly one
`GrantToken` string per grant, confirmed by re-reading both -- "same GrantId, fresh token,
old token still valid" genuinely needs a storage-model change (multiple tokens per grant),
not a quick fix. (2) `grep -rn DryRun services/kms/*.go` (excluding tests) returns nothing
-- still entirely absent, still a broad multi-op feature addition. No code changed for
either item this pass.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

Audited this package's marker-based pagination for the Class A (panic)/B/C
(stale-cursor-resets-to-zero) shapes found in five services during this
campaign's first pass. No bug found — this pattern is correct, verified
directly rather than assumed from reading.

`paginateTagList` (`handler_tags.go`) and `parseMarker` (`store.go`) back
this package's single pagination shape, duplicated inline (not via a shared
function) across `custom_key_stores.go`, `aliases.go`, `grants.go` (x2),
`keys.go`, `key_policies.go` and `rotation.go` — 8 operations total sharing
the identical `startIdx`/`end`/`NextMarker` structure. It's an offset-token
paginator matching `pkgs/page`'s algorithm exactly (this package hand-rolls
it rather than importing `pkgs/page`): `parseMarker` returns 0 on
empty/invalid/negative input (never a raw, unclamped index), and every call
site checks `startIdx >= len(...)` before slicing.

All seven checks pass, including the stale/tampered-marker case (a marker
past the current count safely returns an empty, non-truncated page — proven
directly against `paginateTagList` and, through the real
`aws-sdk-go-v2/service/kms` client, against `ListAliases`) — see
`pagination_arithmetic_internal_test.go` and
`pagination_sdk_roundtrip_test.go`.

Gates: `go build ./services/kms/...`, `go vet ./services/kms/...` and
`go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/kms/...`, `golangci-lint run ./services/kms/...` (0 issues). No
production code changed this pass — test-only additions confirming
correctness.

## 2026-08-30 value-semantics filter/default sweep (gopherstack-uox6's class, first pass on this axis)

No prior pass had checked kms for the class this bd issue tracks: a
documented filter/default semantic that is read and applied but wrong,
invisible to field-shape or enum-legality scans. Checked every List/Describe
op's optional filters and defaults against `aws-sdk-go-v2/service/kms@v1.55.4`'s
own doc comments.

### 1 bug found and fixed: wrong default `Limit` on 3 of 7 shared-constant list ops

`defaultListLimit = 100` was used uniformly by all 7 paginated list ops
(`ListAliases`, `ListGrants`, `ListRetirableGrants`, `ListKeys`,
`ListKeyPolicies`, `ListKeyRotations`, `DescribeCustomKeyStores`). The SDK's
own doc comments give a *different* documented default per op, not a single
value:

| op | doc'd default | doc'd max | gopherstack before | verdict |
|---|---|---|---|---|
| `ListAliases` | 50 | 100 | 100 | **wrong — fixed** |
| `ListGrants` | 50 | 100 | 100 | **wrong — fixed** |
| `ListRetirableGrants` | 50 | 100 | 100 | **wrong — fixed** |
| `ListKeys` | 100 | 1000 | 100 | correct |
| `ListKeyPolicies` | 100 | 1000 | 100 | correct |
| `ListKeyRotations` | 100 | 1000 | 100 | correct |
| `DescribeCustomKeyStores` | undocumented | undocumented | 100 | not contradicted |

`ListResourceTags` (`handler_tags.go`) already used its own, correct
`defaultKMSTagsLimit = 50` — proof this exact discrepancy had already been
gotten right once and simply wasn't propagated to the other 50-default ops.
A real client calling `ListAliases`/`ListGrants`/`ListRetirableGrants` with
no `Limit` got up to twice as many results per page, and a different
`NextMarker`/`Truncated` boundary, than real AWS would ever return.

Fixed: added `default50ListLimit = 50` (`store.go`) alongside the existing
`defaultListLimit = 100`, and switched `aliases.go`'s `ListAliases`,
`grants.go`'s `ListGrants` and `ListRetirableGrants` to it.
`ListKeys`/`ListKeyPolicies`/`ListKeyRotations`/`DescribeCustomKeyStores`
are unchanged (already correct/undocumented).

Tests (new): `TestListAliases_DefaultLimit_Is50` (`aliases_test.go`),
`TestKMSBackendListGrants_DefaultLimit_Is50`,
`TestKMSBackendListRetirableGrants_DefaultLimit_Is50`
(`grants_internal_test.go`) — each creates 51 items and confirms exactly 50
come back unbounded, `Truncated=true`, `NextMarker="50"`. All three
hand-confirmed failing against unmodified code (51 items returned,
`Truncated=false`, empty `NextMarker`) before the fix.

### Other filters/defaults checked, no bug

- `ListAliases`' `KeyId` (absent ⇒ "returns all aliases in the account and
  Region", per doc) and `DescribeCustomKeyStores`' `CustomKeyStoreId`/`Name`
  (absent ⇒ "returns information about all custom key stores") both
  correctly return everything when omitted — verified by reading the
  empty-filter branch in each.
- `CreateKey`'s `Origin` (absent ⇒ `AWS_KMS`, per doc: "The default is
  AWS_KMS") — correct (`keys.go`).
- `ImportKeyMaterial`'s `ExpirationModel` (absent ⇒ `KEY_MATERIAL_EXPIRES`
  per doc, which in turn requires `ValidTo`) is a genuine discrepancy: this
  backend's `resolveExpirationModel` (`import.go`) infers `NO_EXPIRY`
  instead when both `ExpirationModel` and `ValidTo` are omitted, silently
  accepting a request the real API's own documented default would reject
  with a `ValidTo`-required validation error. **Deliberately not fixed**:
  at least 10 existing tests across `import_test.go` and other files
  construct `ImportKeyMaterialInput` with neither field set, expecting
  success — a strong signal this was a considered prior design choice, not
  an oversight, and "fix" here means inventing the exact
  `ValidationException` shape for a combination no live-AWS evidence in
  this repo confirms, which this class's own restraint guidance (discard
  under-verified corrections; a large blast radius against deliberately
  authored tests outweighs a documentation reading) argues against.
  Recorded as a gap rather than guessed.
- `GetParametersForImport`/`ImportKeyMaterial`'s `ImportType` (conditional
  default: `NEW_KEY_MATERIAL` vs `EXISTING_KEY_MATERIAL` depending on prior
  import state) is not declared anywhere in `models.go` — the OTHER axis
  (field never read at all), not this class's bug; recorded, not fixed
  here.
- `ListKeyRotations`' `IncludeKeyMaterial` (default `ROTATIONS_ONLY`,
  narrower than `ALL_KEY_MATERIAL`) is likewise never declared in
  `ListKeyRotationsInput` (`models.go`) — the OTHER axis; the feature it
  gates (surfacing first/pending-import key material entries, not just
  rotation events) isn't modeled by this backend's `RotationRecord` at all,
  so there's also nothing to filter yet. Recorded, not fixed.
- `ListAliases`' `Limit` max-bound validation (`aliases.go`) accepts up to
  1000, where the SDK documents a max of 100 for this op specifically (only
  `ListKeys` documents 1000) — a missing rejection (validation-shaped, this
  service accepts a value real AWS would reject), not a wrong algorithm.
  Recorded separately per this class's own validation/semantics split, not
  fixed here. `ListGrants`/`ListRetirableGrants` have no max-bound
  validation at all (same axis).
- `GenerateRandom`'s `CustomKeyStoreId` (absent ⇒ "the random byte string is
  generated in KMS", per doc) is never declared in this backend — the OTHER
  axis; recorded, not fixed.

No web pages fetched this pass — everything resolved from the pinned
`aws-sdk-go-v2/service/kms@v1.55.4` module cache doc comments.

Gates: `go build ./services/kms/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/kms/...`, `golangci-lint run
./services/kms/...` (0 issues). Work left uncommitted per this pass's
instructions.
