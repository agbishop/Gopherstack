---
service: secretsmanager
sdk_module: aws-sdk-go-v2/service/secretsmanager@v1.48.0
last_audit_commit: 1a7ddc64b  # STALE/WRONG -- this hash resolves to an unrelated build/CI commit, not
                              # a secretsmanager audit; not an ancestor of HEAD on this branch either.
                              # Left uncorrected (this pass made no commit); a future audit should
                              # replace it with the real commit once this pass's fix lands.
last_audit_date: 2026-08-30   # this pass (wrapper-key-sweep-rds-cloudwatch-sqs-sns branch); prior
                               # header value (2026-08-10) was itself stale -- gopherstack-3tpf's
                               # mechanical struct-field diff (see gaps below) actually ran 2026-08-14
                               # and was never reflected up into this header field.
overall: A            # 2026-08-30 pass: two real filter bugs found and fixed, both in the shared
                       # anyMatchPrefix/secretMatchesFilter path ListSecrets and BatchGetSecretValue
                       # both use. (1) types.Filter.Values' documented "!"-negation prefix ("You can
                       # prefix your search value with an exclamation mark ( ! ) in order to perform
                       # negation filters", types/types.go@v1.44.4) was entirely unimplemented --
                       # anyMatchPrefix treated a "!foo" value as a literal (never-matching) prefix,
                       # so a client's negation filter silently returned an EMPTY list instead of
                       # "everything except foo" (the primary-class "silent empty slice" shape).
                       # (2) BatchGetSecretValueInput.Filters is the identical []types.Filter type
                       # ListSecretsInput.Filters uses (same 7-key vocabulary), but batchMatchesFilters
                       # only had switch cases for name/description/tag-key/tag-value -- a filter
                       # keyed primary-region/owning-service/all silently matched every secret (no
                       # case, no default, loop just continues), the "unfiltered full list" shape.
                       # Fixed by making batchMatchesFilters delegate to secretMatchesFilter (a type
                       # conversion, BatchGetSecretValueFilter and SecretFilter are field-identical)
                       # instead of re-implementing a narrower switch that could drift. Proven via
                       # TestListSecrets_FilterNegationExcludes and
                       # TestBatchGetSecretValue_FilterAllKeyIsHonoured (wrapper_key_filter_negation_test.go),
                       # both confirmed failing against unmodified code first. NOT fixed, disclosed
                       # only (semantic ambiguity, not a wrapper-key bug): types.Filter.Values' doc
                       # also states "description"/"all" prefix matches are case-INsensitive while
                       # name/tag-key/tag-value/primary-region/owning-service are case-sensitive --
                       # this mock's anyMatchPrefix is case-sensitive uniformly; and "all" is
                       # documented to "break the filter value string into words" rather than treat
                       # it as one prefix, which this mock also doesn't do. Both are real, doc-cited
                       # divergences but lower-confidence to fix without an authoritative word-split
                       # algorithm, so left as a gap rather than guessed at. Gates
                       # (build/vet/test -race/golangci-lint) clean on services/secretsmanager/...;
                       # assertion count 1656 (was 1648), 0 dropped.
                       #
                       # gopherstack-9wuh sweep: RotateSecret's lenient no-strategy gap (previously
                       # gopherstack-qqq, kept open on the circular "dozens of tests rely on it"
                       # justification) is now closed -- real AWS's InvalidRequestException doc
                       # comment in types/errors.go@v1.44.4 documents the missing-Lambda-ARN
                       # condition verbatim, and the entrenching tests were corrected instead of the
                       # gap being preserved. managed-external-secret fields (Type,
                       # ExternalSecretRotationMetadata/RoleArn) were verified real and wired in;
                       # OwningService verified genuinely unpopulatable by any input in real AWS
                       # either, so its permanent zero value is correct -- but the owning-service
                       # ListSecrets filter's unconditional pass-all was a real, separate, more-
                       # permissive-than-AWS bug and is fixed. Also fixed a second "state mutated
                       # before validation" instance in UpdateSecret (Description/KmsKeyId applied
                       # before a same-call value-update failure was checked). See ops/gaps below.
ops:
  CreateSecret: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added missing ClientRequestToken idempotency contract (matches/mismatches an existing version's content on name collision). Fixed 2026-08-10 (gopherstack-9wuh): Type (api_op_CreateSecret.go's CreateSecretInput.Type, 'the exact string that identifies the partner that holds the external secret') was a real settable input field with no corresponding struct field in gopherstack's CreateSecretInput -- accepted on the wire then silently dropped by json.Unmarshal (unknown-field-ignored), not even a stub. Added the field, wired into secret.Type, echoed by DescribeSecret/ListSecrets."}
  GetSecretValue: {wire: ok, errors: ok, state: ok, persist: ok, note: "VersionId+VersionStage resolution correct; access-day clock now uses injectable b.now()"}
  PutSecretValue: {wire: ok, errors: ok, state: fixed, persist: ok, note: "AWSCURRENT/AWSPREVIOUS rotation on staging labels correct; clock consistency fixed. Fixed 2026-09-06 (gopherstack-ngkw): a replica secret was writable directly via PutSecretValue, so it could diverge from its primary instead of only receiving values through replication. Secrets Manager User Guide ('Promote a replica secret to a standalone secret'): 'A replica secret can't be updated independently from its primary secret, except for its encryption key.' PutSecretValue has no KMS-key parameter, so it is always rejected on a replica now (InvalidRequestException via new ErrReplicaNotWritable -- modeled: PutSecretValue's deserializeOpError in aws-sdk-go-v2/service/secretsmanager@v1.44.4 deserializers.go includes InvalidRequestException). A primary/standalone secret is unaffected. See TestReplicaWriteGuard_BlockedOnReplica/putsecretvalue, TestReplicaWriteGuard_PrimaryUnaffectedByReplicas/putsecretvalue."}
  DeleteSecret: {wire: ok, errors: ok, state: ok, persist: ok, note: "force-delete vs 7-30d recovery window, mutual exclusivity with RecoveryWindowInDays, already correct"}
  RestoreSecret: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSecrets: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "IncludeDeleted field name was wrong (real key IncludePlannedDeletion); SortBy was entirely unsupported; NextRotationDate was missing from SecretListEntry. All three fixed. RLock no longer lazily mutates the region map (see leaks). Fixed 2026-08-10 (gopherstack-9wuh): the 'owning-service' filter (FilterNameStringType, a prefix match against DescribeSecretOutput.OwningService) unconditionally returned true for every secret regardless of the filter value -- more permissive than real AWS, which would match zero secrets here since no CreateSecret/UpdateSecret input field can ever set OwningService (verified: absent from both api_op_CreateSecret.go's and api_op_UpdateSecret.go's Input structs; only AWS itself sets it, for service-linked secrets like RDS-managed rotation, which this mock does not model). A real client filtering ListSecrets by owning-service=rds.amazonaws.com would have wrongly gotten back every user-created secret. Fixed to match against the (always-empty) field, which now correctly matches nothing for any non-empty filter value; three tests that asserted the old always-pass behavior as correct were corrected (TestListSecrets_FilterOwningServicePassesAll -> FilterOwningServiceMatchesNone, FilterOwningServiceWithOtherFilters, OwningServiceHTTP). FIXED 2026-08-30 -- Filter.Values' documented '!' negation prefix (types/types.go@v1.44.4) was unimplemented in anyMatchPrefix, so a negated value never matched anything (silent empty-list bug, not silent pass-all); see the overall header note for full citation."}
  ListSecretVersionIds: {wire: ok, errors: ok, state: ok, persist: ok, note: "RLock no longer lazily mutates the region map (see leaks)"}
  DescribeSecret: {wire: fixed, errors: ok, state: ok, persist: ok, note: "RLock no longer lazily mutates the region/replication maps (see leaks); fabricated OwnerAccountId field DELETED 2026-07-23 (confirmed absent from types.DescribeSecretOutput and types.SecretListEntry in aws-sdk-go-v2/service/secretsmanager@v1.43.0/api_op_DescribeSecret.go and types/types.go) — closes gopherstack-pct's OwnerAccountId half; PrimaryRegion verified real (both structs), kept. Fixed 2026-08-10 (gopherstack-9wuh): added Type/ExternalSecretRotationRoleArn/ExternalSecretRotationMetadata echo (all three confirmed real DescribeSecretOutput fields in api_op_DescribeSecret.go@v1.44.4); OwningService remains genuinely never populated (see ListSecrets note) and is correctly always omitted (zero value), not fabricated."}
  UpdateSecret: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "clock consistency fixed (was time.Now(), now b.now()). Fixed 2026-08-10 (gopherstack-9wuh): (1) added the same Type field CreateSecret was missing (api_op_UpdateSecret.go's UpdateSecretInput.Type); (2) found a 'state mutated before validation' bug (parity-principles.md bug class, found twice elsewhere in this campaign, now three times counting this one): Description and KmsKeyId were written directly onto the live secret BEFORE attempting a same-call SecretString/SecretBinary update, so a request that also changed the value but failed partway (e.g. a KMS encryption error) still left the Description/KmsKeyId mutations applied even though the overall call returned an error. Reordered so Description applies only after a successful value update, and KmsKeyId -- which sealVersion must read to pick the encryption key for the new version, so it can't simply be deferred -- is applied optimistically and rolled back on failure. write-only-state sweep (this pass): KmsKeyId was a plain string guarded by != \"\" (not *string like the real UpdateSecretInput.KmsKeyId, api_op_UpdateSecret.go), whose doc says \"If you set this to an empty string, Secrets Manager uses the Amazon Web Services managed key aws/secretsmanager\" -- a client's documented, explicit revert-to-default was silently dropped. Now *string with a nil check (secrets.go). Response side (DescribeSecretOutput.KmsKeyID, models.go) intentionally kept omitempty -- real Secrets Manager only returns KmsKeyId when a customer-managed key is set, omitting it for the (far more common) default-managed-key case; the internal Secret.KmsKeyID field is a plain string, so 'cleared' and 'never set' are indistinguishable in storage regardless, and stripping omitempty would put a spurious empty key on every DescribeSecret using the default key. Round-trip test: wire_field_fixes_test.go (TestUpdateSecret_KmsKeyIDCanBeCleared). Fixed 2026-09-06 (gopherstack-ngkw): UpdateSecret against a replica secret with SecretString/SecretBinary/Description/Type set was applied directly, letting the replica diverge from its primary. Per the same User Guide quote as PutSecretValue's note ('except for its encryption key'), a replica now rejects UpdateSecret (InvalidRequestException/ErrReplicaNotWritable, modeled per UpdateSecret's deserializeOpError) when any of those four fields is set, but a KmsKeyId-only call still succeeds -- the one field the guide documents as independently updatable on a replica, matching the real API's own KmsKeyId-only example request. See TestReplicaWriteGuard_BlockedOnReplica/updatesecret_* and TestReplicaWriteGuard_StillAllowedOnReplica/updatesecret_kmskeyid_only."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-ngkw (2026-09-06) investigated whether this, UntagResource, CancelRotateSecret, and RestoreSecret should also reject a replica secret the way PutSecretValue/UpdateSecret/RotateSecret now do. No AWS source (API reference Errors list, SDK doc comments, or User Guide) states that tagging a replica directly is disallowed -- the User Guide only says tags are part of what gets replicated FROM the primary, not that TagResource against a replica ARN itself errors -- so left unguarded rather than fabricating a rejection; see TestReplicaWriteGuard_StillAllowedOnReplica/tagresource."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  RotateSecret: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "immediate rotation + Lambda 4-step invocation + AWSPENDING->AWSCURRENT promotion correct; RotateImmediately=false now runs the testSecret probe (fixed 2026-07-23, closes gopherstack-avt): backend.BeginRotationTestProbe creates a transient AWSPENDING version, handler.runRotationTestProbe invokes only the testSecret Lambda step (resolving RotationLambdaARN from the request or, per AWS doc text 'starts a rotation with the values already stored in the secret', falling back to the secret's stored ARN via DescribeSecret), then the version is unconditionally removed via a defer'd AbortRotation regardless of invocation outcome — verified with TestRotateSecret_RotateImmediatelyFalseWithLambdaRunsTestSecretProbe (success path: exactly 1 Lambda call, step=testSecret, VersionID empty, no leftover AWSPENDING label, AWSCURRENT unchanged) and TestRotateSecret_RotateImmediatelyFalseWithLambdaProbeFails (failure path: error surfaced, probe version still removed). No Lambda configured / no invoker wired: unchanged no-op, as before. Fixed 2026-08-07 (gopherstack-9wuh, part 1): the RotateImmediately=true immediate-rotation path (both handler.rotateSecret and backend.RotateSecret) checked the *request's* RotationLambdaARN field to decide whether to invoke the configured Lambda, so a RotateSecret call that omitted RotationLambdaARN -- the normal case once EnableRotation/an earlier RotateSecret has already stored one on the secret -- silently skipped the Lambda entirely and auto-promoted to AWSCURRENT, even though a Lambda was in fact configured. The RotateImmediately=false testSecret probe already resolved the ARN correctly (request, else DescribeSecret's stored value, per handler.resolveRotationLambdaARN); the immediate-rotation path now shares that same resolution instead of trusting the request field alone. Verified with TestRotateSecret_OmittedARNUsesStoredLambda (configure via one RotateSecret call, then a second call with SecretId only still drives all 4 Lambda steps). Fixed 2026-08-10 (gopherstack-9wuh, part 2 — closes the remaining lenient-no-strategy gap): established from the pinned SDK, not prose, that real AWS rejects the operation entirely when it is not given a rotation strategy. aws-sdk-go-v2/service/secretsmanager@v1.44.4 validators.go's validateOpRotateSecretInput only requires SecretId client-side (no client-side check of RotationLambdaARN), but types/errors.go's InvalidRequestException doc comment enumerates the server-side condition verbatim: 'You tried to enable rotation on a secret that doesn't already have a Lambda function ARN configured and you didn't include such an ARN as a parameter in this call.' deserializers.go's awsAwsjson11_deserializeOpErrorRotateSecret (matched via strings.EqualFold, not literal case labels) confirms InvalidRequestException is in RotateSecret's modelled error set alongside InternalServiceError/InvalidParameterException/ResourceNotFoundException. Added ErrRotationStrategyRequired, checked in InMemoryBackend.RotateSecret BEFORE any mutation (effective ARN = request's RotationLambdaARN, else the secret's already-stored one) so a rejected call leaves RotationEnabled/RotationRules/the version set untouched — this is also a 'state mutated before validation' fix, the same bug class flagged elsewhere in this campaign. Did NOT exempt ExternalSecretRotationRoleArn-only calls from this check: the SDK's InvalidRequestException text does not say a managed-external-secret role ARN substitutes for a Lambda ARN, and gopherstack does not implement any managed-external-secret behavior for this to unlock, so inventing that exemption would be exactly the kind of unverified formula this campaign was warned against fabricating — left conservative and cited. The dozens-of-tests justification that had kept this gap open was circular (see gopherstack-9wuh): those tests asserted the lenient behavior as correct, then that assertion was cited as the reason not to fix it. Corrected ~21 test functions/subtests across 8 files (rotatesecret_test.go, cancelrotatesecret_test.go, describesecret_test.go, handler_dispatch_test.go, getsecretvalue_test.go, kms_test.go, listsecrets_test.go, persistence_test.go — none of which needed store_conversion_test.go, which already configured a Lambda ARN) to supply a RotationLambdaARN; one test (TestKMSEncryptor_RotateSecret_NoLambda_CarriesValueForward) tested a scenario — successful rotation with no strategy ever configured — that cannot happen in real AWS at all, so it was rewritten as TestKMSEncryptor_RotateSecret_NoLambda_RejectsAndLeavesValueUnchanged (asserts the new rejection plus that the KMS encryptor is never touched by the rejected call). Nothing in this service depends on RotateSecret succeeding without a strategy: CancelRotateSecret/scheduler/persistence/replication all operate on whatever rotation state already exists and don't require a fresh RotateSecret call to have gone through, unlike the codedeploy case cited as a caution — that one was correctly left permissive because deployments there depend on a prior step succeeding; nothing here does. New regression test TestRotateSecret_NoRotationStrategyConfigured_Rejected; wire/error additions (Type, ExternalSecretRotationRoleArn, ExternalSecretRotationMetadata — see gaps for the classification of each) covered by TestRotateSecret_ExternalSecretRotationFieldsAcceptedAndEchoed and TestSnapshotRestore_ManagedExternalSecretFields. Fixed 2026-09-06 (gopherstack-ngkw): RotateSecret against a replica secret ran rotation independently instead of being rejected. Secrets Manager User Guide ('Replicate AWS Secrets Manager secrets across Regions'): 'If you turn on rotation for your primary secret, Secrets Manager rotates the secret in the primary Region, and the new secret value propagates to all of the associated replica secrets'; and ('Promote a replica secret to a standalone secret'): you might promote a replica to standalone 'if you want to turn on rotation for the replica' -- both establish that rotation is a primary-only operation. A replica secret now rejects RotateSecret before the rotation-strategy check (InvalidRequestException/ErrReplicaNotWritable, modeled per RotateSecret's deserializeOpError); a primary/standalone secret with replicas configured is unaffected. See TestReplicaWriteGuard_BlockedOnReplica/rotatesecret, TestReplicaWriteGuard_PrimaryUnaffectedByReplicas/rotatesecret."}
  GetRandomPassword: {wire: ok, errors: ok, state: ok, note: "length bounds, exclude-chars, require-each-type, crypto/rand rejection sampling all correct"}
  ListAll: {wire: n/a, state: ok, note: "internal dashboard helper, not a wire op"}
  BatchGetSecretValue: {wire: ok, errors: ok, state: fixed, persist: ok, note: "clock consistency fixed for LastAccessedDate. FIXED 2026-08-30 -- Filters is []types.Filter (api_op_BatchGetSecretValue.go), the identical shared type ListSecretsInput.Filters uses (same 7-key vocabulary: name/description/tag-key/tag-value/primary-region/owning-service/all), but batchMatchesFilters only had switch cases for 4 of the 7 keys -- a primary-region/owning-service/all filter silently matched every secret (no case, no default). Fixed by delegating to secretMatchesFilter (SecretFilter(f) conversion -- BatchGetSecretValueFilter and SecretFilter are field-identical) instead of a separate, narrower switch. See TestBatchGetSecretValue_FilterAllKeyIsHonoured (wrapper_key_filter_negation_test.go), confirmed failing pre-fix (2 results instead of 1)."}
  CancelRotateSecret: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "RLock no longer lazily mutates the region map (see leaks)"}
  PutResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "BlockPublicPolicy default-true + wildcard-principal detection correct"}
  DeleteResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  ReplicateSecretToRegions: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveRegionsFromReplication: {wire: ok, errors: ok, state: ok, persist: ok}
  StopReplicationToReplica: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSecretVersionStage: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "was silently stripping a staging label from wherever it happened to be attached, regardless of RemoveFromVersionId; real API requires RemoveFromVersionId to name the current holder or the call fails. Fixed + regression tests added; one existing test (TestRefinement1_UpdateSecretVersionStageAutoStrip) encoded the old wrong behavior and was corrected."}
  ValidateResourcePolicy: {wire: ok, errors: ok, state: ok, note: "RLock no longer lazily mutates the region map (see leaks)"}
families:
  version-staging: {status: ok, note: "AWSCURRENT/AWSPENDING/AWSPREVIOUS transitions, auto-demotion of AWSCURRENT->AWSPREVIOUS on PutSecretValue/UpdateSecret/rotation, max 100 versions with unlabeled-oldest-first pruning — all verified against real semantics"}
  rotation: {status: partial, note: "Lambda 4-step invocation (createSecret/setSecret/testSecret/finishSecret), rate()/cron() schedule parsing and due-date computation, scheduler goroutine with ctx-bounded lifecycle, RotateImmediately=false testSecret probe (fixed 2026-07-23, gopherstack-avt), RotateImmediately=true now resolves a request-omitted RotationLambdaARN from the secret's stored value too (fixed 2026-08-07, gopherstack-9wuh) — all correct. Remaining gap: missing-rotation-function validation (gopherstack-qqq, intentional)"}
  replication: {status: ok, note: "ReplicateSecretToRegions/RemoveRegionsFromReplication/StopReplicationToReplica + status sync on version change all verified"}
  resource-policy: {status: ok, note: "Get/Put/Delete/Validate + BlockPublicPolicy + MalformedPolicyDocumentException/PublicPolicyException verified"}
  error-codes: {status: ok, note: "ResourceNotFoundException/ResourceExistsException/InvalidRequestException/InvalidParameterException/MalformedPolicyDocumentException/PublicPolicyException all verified against types/errors.go. Re-audit 2026-07-11: fetched the live AWS API reference for TagResource and BatchGetSecretValue — neither operation's documented Errors list includes LimitExceededException (TagResource: InternalServiceError/InvalidParameterException/InvalidRequestException/ResourceNotFoundException only; BatchGetSecretValue adds DecryptionFailure/InvalidNextTokenException, still no LimitExceededException). The previous gopherstack-gvw gap note asserting these ops should return LimitExceededException on tag/SecretIdList limit overflow was an unverified assumption and was WRONG; current InvalidParameterException behavior on both ops is correct AWS parity. CreateSecret's Errors list DOES include LimitExceededException, but AWS doesn't document which specific validation maps to it and CreateSecret's InvalidParameterException-on-tag-overflow is equally consistent with its documented error set, so left as-is (no evidence of a bug, would be speculative to change). gopherstack-gvw should be closed as invalid/works-as-intended."}
  persistence: {status: ok, note: "Snapshot/Restore round-trips all fields including json:\"-\" internal fields via secretSnapshot; Tags.Close() called on replace to avoid Prometheus registry leaks; rotation scheduler re-armed on restore when RotationEnabled"}
  concurrency-locking: {status: fixed, note: "see leaks — RLock-guarded reads were lazily mutating the coarse per-region maps; fixed with non-mutating *StoreRO accessors"}
gaps:
  - 2026-08-30 (this pass): types.Filter.Values' doc comment (types/types.go@v1.44.4) says "description"
    and "all" keys are prefix-matched case-INsensitively, while name/tag-key/tag-value/primary-region/
    owning-service are case-sensitive; this mock's anyMatchPrefix is case-sensitive uniformly. The same
    doc also says "all" "breaks the filter value string into words and then searches all attributes",
    not a single whole-string prefix match, which is what this mock's "all" case does instead. Both are
    real, doc-cited divergences from documented AWS behavior, DISCLOSED not fixed -- the exact
    word-splitting algorithm isn't specified precisely enough in the SDK's doc comment to implement with
    confidence, and inventing one would be exactly the fabrication this campaign warns against; case-
    insensitivity alone could be fixed cheaply but was left alongside the word-breaking gap rather than
    partially fixed, since a client relying on "all" is already getting whole-string-not-word prefix
    matching regardless of case.
  - CLOSED 2026-08-10 (gopherstack-9wuh, part 2): RotateSecret no longer accepts rotation with no RotationLambdaARN ever configured — see the RotateSecret ops entry above for the full citation and fix. The "dozens of tests depend on it" justification was circular (those tests were the artifact of the gap, not independent evidence for keeping it) and has been corrected rather than preserved.
  - managed-external-secret fields, reclassified 2026-08-10 (gopherstack-9wuh, part 3 — three-way split per field, verified against aws-sdk-go-v2/service/secretsmanager@v1.44.4 api_op_*.go, not assumed):
    - Type and ExternalSecretRotationMetadata/ExternalSecretRotationRoleArn were **accepted then silently dropped**: all three are real settable input fields (Type on CreateSecretInput and UpdateSecretInput; ExternalSecretRotationMetadata/ExternalSecretRotationRoleArn on RotateSecretInput) that gopherstack's wire structs simply had no field for, so json.Unmarshal silently discarded them — not even a stub, the data never existed past the HTTP boundary. FIXED: added to CreateSecretInput/UpdateSecretInput/RotateSecretInput, stored on Secret, echoed by DescribeSecretOutput and SecretListEntry, round-trips through Snapshot/Restore (additive omitempty fields, no snapshot version bump). RotateSecret's Lambda-ARN-required check (see above) was deliberately NOT relaxed for a request that only supplies ExternalSecretRotationRoleArn — the SDK's InvalidRequestException text doesn't document that as an alternative, and gopherstack has no managed-external-secret invocation behavior for it to unlock, so leaving it required is the conservative, citable choice, not a gap.
    - OwningService is **genuinely absent from any input this mock could wire it from**: confirmed absent from both CreateSecretInput and UpdateSecretInput in api_op_CreateSecret.go/api_op_UpdateSecret.go@v1.44.4 — in real AWS it is set only by AWS itself, for service-linked/managed secrets (e.g. RDS-managed rotation), which this mock does not model at all (see deferred). This one really does require a managed-service model that doesn't exist here, so it stays permanently unset — that's correct, not a gap. What WAS a gap: the "owning-service" ListSecrets filter used to unconditionally return true regardless of filter value, which is more permissive than AWS (a real client filtering by owning-service=rds.amazonaws.com would wrongly get back every secret instead of none). FIXED — see ListSecrets ops entry above.
  - 2026-08-14 (gopherstack-3tpf mechanical struct-field diff, cmd/structfielddiff, all 23 ops against aws-sdk-go-v2/service/secretsmanager@v1.44.4 -- wire-complete otherwise, every Input/Output/nested field matched): two more real request members silently dropped, same class as the Type fix above, both DISCLOSED not fixed -- see gopherstack-zurl for the full citation and why each is unsafe to enforce today rather than a two-line add:
    - CreateSecretInput.ForceOverwriteReplicaSecret (bool) -- attempting a real fix surfaced that gopherstack's replication status never distinguishes a destination-name-collision Failed from syncReplicationStatusLocked's own no-current-version Failed, so a naive fix's Failed status gets silently promoted to InSync by the very next sync call. Reverted rather than shipped half-working.
    - PutSecretValueInput.RotationToken (string) -- a cross-account rotation identity token with nothing in gopherstack's rotation model to validate it against (no session/trust engine), structurally the same as sts's disclosed JWTPayloadSizeExceededException gap.
deferred:
  - Managed rotation (AWS-service-owned secrets, e.g. RDS-managed rotation) — out of scope, not modeled at all
  - Cross-account resource-policy principal evaluation beyond the wildcard-principal BlockPublicPolicy heuristic
leaks: {status: fixed, note: "Found a real data race: ListSecrets/ListSecretVersionIds/DescribeSecret/GetResourcePolicy/ValidateResourcePolicy held only an RLock (shared reader lock) but called the lazily-creating *Store(region) helper, which does `b.secrets[region] = make(...)` on first touch of a region — a concurrent map write happening under a read lock. Confirmed with a regression test (concurrency_race_test.go) that reproduces the `go test -race` data race pre-fix and passes clean post-fix. Fixed by adding non-mutating *StoreRO(region) accessors for read-only call sites (a nil-map read/range is well-defined in Go, so no mutation is needed there). Rotation scheduler goroutine, janitor, and StopRotationScheduler/Shutdown ctx-cancellation lifecycle were already clean (verified, no changes needed)."}
---

## Notes

- **2026-08-30 (wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)**: audit-recency check first --
  `last_audit_date` header said 2026-08-10, but this file's own `gaps:` list already documented a later
  2026-08-14 mechanical struct-field diff (`cmd/structfielddiff`, gopherstack-3tpf, all 23 ops) that
  found the service "wire-complete otherwise" with two disclosed-only gaps
  (`ForceOverwriteReplicaSecret`, `RotationToken`). Header field was simply never bumped after that pass
  -- corrected here, not a "newer note sorts below an older one" case, just a stale top-level field.
  Also: `last_audit_commit` (`1a7ddc64b`) resolves to `build: enforce the pin check in CI...`, an
  unrelated commit not touching this service and not an ancestor of this branch's HEAD -- left
  uncorrected pending this pass's own commit (not made by this pass; see the header comment).
  SDK pin unchanged (`v1.44.4`, matches `go.mod`/module cache, confirmed by `ls api_op_*.go` = 23,
  exact match with `GetSupportedOperations()`'s 23-entry literal list). Op count verified 23/23, not
  assumed. Given the genuinely thorough, recent (16-day-old) mechanical field-diff on record, did not
  redo a full struct-by-struct rescan; instead grepped for anonymous decode-target structs across
  `services/secretsmanager/*.go` (zero hits -- this service has no `cmd/structfielddiff`-style blind
  spot to check by hand) and hand-audited the two hand-rolled list-filter code paths
  (`ListSecrets`/`secretMatchesFilter`, `BatchGetSecretValue`/`batchMatchesFilters`) against
  `types.Filter`'s full doc comment in `types/types.go`, since a mechanical field-name/type diff cannot
  catch a documented *value-semantics* gap (the field exists, is read, and is even applied -- just with
  the wrong algorithm). Found and fixed 2 real bugs this way (see `overall`/`ops` above); confirmed no
  other `anyMatchPrefix`/`secretMatchesFilter` call sites existed to have the same gap independently.
  `ListSecrets`'s own pagination path re-checked while in the file: filter is applied before sort/slice
  (no filter-after-pagination bug), and the default (no-`SortBy`) case ties on `Name`, so a map-derived
  input list still produces a stable order -- no ordering bug. Gates clean; did not touch `dms`/`batch`
  in this file's own service beyond this note (see their own PARITY.md for their fresher 2026-08-29
  sweeps on this same branch, spot-verified but not re-audited from scratch this pass).

- **2026-08-22 (gopherstack-urw6) — rotation scheduler audited for the
  "getter hands out a live pointer / shallow copy read outside the lock while
  deferred work writes it" race class (the class fixed in services/securityhub,
  services/eks, services/s3, services/amplify): `rotationSchedulerLoop` ->
  `runScheduledRotations` mutates secret/version state (`finishRotationLocked`,
  `abortRotationLocked`, `rotateStagingLabels`) exclusively through
  `b.mu.Lock()`, and every reader (`GetSecretValue`, `DescribeSecret`,
  `ListSecretVersionIDs`, `BatchGetSecretValue`, etc.) holds `b.mu` (R or W)
  for its entire field-touching duration via `defer`. `secretGet` does return
  the live stored `*Secret`, but no call site lets that pointer (or a field
  off it) escape past the lock the way the fixed instances did. Every write to
  a shared slice field (`StagingLabels`) reassigns a freshly allocated slice
  (`rotateStagingLabels`, `resolveStagingLabels`, `removeLabel`) rather than
  mutating the existing backing array in place, so even the few call sites
  that alias a slice into an output DTO without a defensive copy
  (`GetSecretValue`'s `VersionStages: version.StagingLabels`,
  `secretVersionEntry`'s `VersionStages: ver.StagingLabels`) are safe: no
  writer ever touches the old backing array again once it's replaced. No bug
  found; no code changed here. (services/ecs *did* have a live instance of
  this class in `getServicesForReconciler`'s `Deployments` slice — see
  services/ecs/PARITY.md's 2026-08-22 entry.)
- **Protocol**: `application/x-amz-json-1.1` (awsJson1_1), matches `secretsmanager.<Operation>`
  `X-Amz-Target` routing already in place in `handler.go`.
- **Wire field names verified directly against `aws-sdk-go-v2/service/secretsmanager@v1.42.5`
  serializers.go/deserializers.go** (not just the Go struct tags), which is how the
  `IncludeDeleted` → `IncludePlannedDeletion` and `owned-by-me` → `owning-service` bugs were
  caught: both were plausible-looking names that don't exist on the wire. A previous audit
  pass invented "owned-by-me" for account-ownership semantics, but the real
  `FilterNameStringType` enum has no such value — the real "owning-service" key is about
  AWS-service-managed secrets (e.g. RDS-managed rotation), a different concept entirely.
  Renamed to the real key while preserving pass-all semantics, since this mock never
  tracks AWS-service ownership of secrets.
- **Timestamps** are epoch-seconds JSON numbers (`float64` via `UnixTimeFloat`), matching
  `smithytime.ParseEpochSeconds` in the real deserializers — already correct throughout.
- **Clock consistency**: `InMemoryBackend.now` is an injectable clock (`SetNowForTest` in
  `export_test.go`) used correctly by `DeleteSecret`/rotation, but `CreateSecret`,
  `seedInitialVersion`, `PutSecretValue`, `UpdateSecret`, `GetSecretValue`, and
  `BatchGetSecretValue`'s access-day tracking all called `time.Now()` directly, bypassing
  the injected clock. Fixed for internal consistency and testability — production behavior
  is unchanged since the default `now` is `time.Now`.
- **RemoveFromVersionId semantics** (`UpdateSecretVersionStage`): the real API requires the
  caller to name the version that currently holds a staging label before it can be moved
  elsewhere — "if the label is attached and you either do not specify [RemoveFromVersionId],
  or the version ID does not match, then the operation fails." This mock silently stripped
  the label from wherever it was, which is a **looks-wrong-but-was-actually-a-bug** case: an
  existing test (`TestRefinement1_UpdateSecretVersionStageAutoStrip`) explicitly asserted the
  permissive (wrong) behavior as correct. Fixed the backend and corrected that test rather
  than working around it.
- **CreateSecret idempotency**: the real API's `ClientRequestToken` doc text is explicit
  about the three-way branch (new version / matching retry ignored / mismatched content
  fails) — this was previously entirely unimplemented for the CreateSecret-level name
  collision case (it existed for `PutSecretValue`/`UpdateSecret`, just not `CreateSecret`).
- **Region-nested maps**: `InMemoryBackend.secrets`/`resourcePolicies`/`replicationConfigs`
  are `map[string]map[string]T]` (outer key = region), lazily created per-region by
  `*Store(region)` helpers. Those helpers **must only be called under `b.mu.Lock()`** (write
  lock) — read paths must use the new `*StoreRO(region)` accessors instead. This is the kind
  of thing that's easy to reintroduce; grep for `RLock(` and confirm no `*Store(` (non-RO)
  calls appear before the matching `RUnlock`/`defer`.
- **`GetSupportedOperations`/dispatch-table op-name strings**: several operation names are
  used in three or more places (the supported-ops list, the dispatch map key, and a
  `lockmetrics` label in `backend.go`) — collapsed the four that tripped `goconst`
  (`DescribeSecret`, `GetResourcePolicy`, `ListSecrets`, `ValidateResourcePolicy`) into shared
  `opXxx` constants. Not exhaustive for every op name (only fixes what already tripped the
  linter); a future full pass could do this for all ~20 ops for consistency.
- **Test files in this package are numerous** (from several prior sweeps —
  `batch1_audit_test.go`, `accuracy_audit_test.go`, `parity_a_test.go`,
  `parity_deepen_test.go`, `handler_refinement1/2_test.go`, etc.). Before adding a new test,
  grep first — there is a good chance similar coverage already exists under a different name.
- **2026-07-11 re-audit**: the ledger's stated `last_audit_commit` (`f093a929`) turned out not
  to be an ancestor of current HEAD (rebased/squashed history elsewhere in the repo); the real
  prior audit commit for this package is `ce30166a` ("Parity sweep 3"), which is the commit
  that wrote this PARITY.md. `git diff ce30166a..HEAD -- services/secretsmanager/` is empty —
  **zero code drift** in this package since that audit. The SDK bumped
  `aws-sdk-go-v2/service/secretsmanager` v1.42.5 → v1.43.0 in the interim
  (`e51c0de9`); diffed the two module versions on disk and confirmed the only changes are
  `CHANGELOG.md`/`generated.json`/`go_module_metadata.go` plus new snapshot-test fixtures —
  no operation or shape changes. Per the re-audit protocol this meant auditing only the ledger's
  non-`ok` rows: fetched the live AWS API reference pages for `TagResource`,
  `BatchGetSecretValue`, `CreateSecret`, and `RotateSecret` to check the two `partial` gaps.
  Result: the `error-codes`/gopherstack-gvw gap was a **false positive from a prior audit** —
  neither `TagResource` nor `BatchGetSecretValue` documents `LimitExceededException` as a
  possible error, so the existing `InvalidParameterException` behavior on tag/SecretIdList
  limit overflow is correct AWS parity and needed no code change (closed the gap in the ledger,
  should close bd `gopherstack-gvw` as invalid). The `RotateSecret` gaps
  (gopherstack-avt/gopherstack-qqq) were re-verified against the live docs and are accurately
  described — left as-is, still open. No code changes were made this pass; gates
  (`build`/`vet`/`test -race`/`go fix -diff`/`golangci-lint`) all pass clean on the unmodified
  tree.
- **2026-07-23 parity-3 sweep**: field-diffed against `aws-sdk-go-v2/service/secretsmanager@v1.43.0`
  on disk (`types/types.go`, `types/enums.go`, `api_op_RotateSecret.go`, `api_op_DescribeSecret.go`)
  rather than the previously-installed v1.42.5 (no shape changes between the two, confirmed in the
  prior audit note above). Two fixes:
  1. **Deleted the fabricated `OwnerAccountId` field** (`DescribeSecretOutput.OwnerAccountID` in
     `models.go`, populated in `secrets.go`'s `DescribeSecret`). Confirmed via
     `grep -n OwnerAccountId types/types.go` that the real SDK's `DescribeSecretOutput` and
     `types.SecretListEntry` have no such field (`SecretListEntry` never had it in gopherstack
     either — only `DescribeSecretOutput` did). `PrimaryRegion` was re-verified as a genuine field on
     both real structs and was kept. Updated/renamed the two tests that asserted the fabricated
     field (`TestDescribeSecret_OwnerAccountIDAndPrimaryRegion` → `TestDescribeSecret_PrimaryRegion`;
     dropped the `OwnerAccountID` assertion from `TestDescribeSecret_AllMetadataFields`).
  2. **Implemented the `RotateImmediately=false` testSecret probe** (gopherstack-avt). Added
     `InMemoryBackend.BeginRotationTestProbe` (rotation.go) — creates a transient AWSPENDING version
     under lock, mirroring `FinishRotation`/`AbortRotation`'s existing exported-wrapper pattern — and
     `Handler.runRotationTestProbe` (handler_rotation.go), which resolves the effective
     `RotationLambdaARN` (request field, else the secret's already-stored ARN via `DescribeSecret`,
     matching the AWS doc text "if you don't include the configuration parameters, the operation
     starts a rotation with the values already stored in the secret"), invokes only the `testSecret`
     Lambda step, and unconditionally removes the transient version via a `defer`'d `AbortRotation` —
     success or failure. Matches the `RotateSecretInput.RotateImmediately` doc comment verbatim: "If
     you set RotateImmediately to false, Secrets Manager tests the rotation configuration by running
     the testSecret step... This test creates an AWSPENDING version of the secret and then removes
     it." Extracted a shared `buildRotationStepEvent` helper (handler_rotation.go) used by
     `invokeLambdaRotationSteps`, `runRotationTestProbe`, and `rotation.go`'s scheduler-side
     `runLambdaRotationSteps` to avoid `goconst`-flagged duplication of the `SecretId`/
     `ClientRequestToken`/`Step` JSON keys across three call sites. Regression tests:
     `TestRotateSecret_RotateImmediatelyFalseWithLambdaRunsTestSecretProbe` (exactly one Lambda call,
     step=testSecret, output VersionID empty, AWSCURRENT unchanged, no leftover AWSPENDING label) and
     `TestRotateSecret_RotateImmediatelyFalseWithLambdaProbeFails` (Lambda failure surfaces as an
     error and the transient version is still removed). Direct backend callers (bypassing `Handler`)
     are unaffected by design — Lambda invocation has always been a `Handler`-layer concern in this
     package (see `invokeLambdaRotationSteps` vs. `b.RotateSecret`'s immediate-with-Lambda path,
     which also defers the actual 4-step invocation to the handler).
  `gopherstack-qqq` (lenient no-Lambda-ever-configured rotation) was re-examined: the real SDK's
  generated code doesn't enumerate which specific documented error maps to this case (that lives only
  in prose API docs, not in `errors.go`/`deserializers.go`), and the existing ledger note already
  reflects a live-docs check from the 2026-07-11 pass. Left as-is per the existing tradeoff — dozens
  of tests depend on the lenient behavior and gopherstack does not model AWS managed rotation.
  Spot-checked `FilterNameStringType` (7 values, all handled in `secretMatchesFilter`), `SortByType`
  (4 values, all handled in `sortSecretListEntries`), and `RotationRulesType`
  (`AutomaticallyAfterDays`/`Duration`/`ScheduleExpression`) against `types/enums.go`/`types/types.go`
  — all match exactly, no further drift found. Gates
  (`build`/`vet`/`test -race`/`gofmt`/`golangci-lint`/banned-nolint grep) all pass clean.
- **2026-08-10 sweep (gopherstack-9wuh)**: re-audited the SDK against `v1.44.4` on disk (pin was
  just corrected repo-wide; confirmed `go.mod`'s `v1.44.4` matches what's in the module cache).
  Closed the RotateSecret lenient-no-strategy gap that three prior audits (2026-07-11, 2026-07-23,
  earlier this pass) had each re-confirmed as an intentional tradeoff without ever checking whether
  the justification was sound: `validators.go`'s `validateOpRotateSecretInput` only requires
  `SecretId` client-side, but `types/errors.go`'s `InvalidRequestException` doc comment states the
  server-side condition in plain English, and `deserializers.go`'s
  `awsAwsjson11_deserializeOpErrorRotateSecret` (matched via `strings.EqualFold`, confirmed by
  grep) lists `InvalidRequestException` as one of RotateSecret's four modelled errors. Corrected
  ~21 test functions/subtests across 8 files rather than re-preserving the gap; one test whose
  entire premise (successful no-strategy rotation) can't happen in real AWS was rewritten to assert
  the new rejection instead. Modeled `Type`/`ExternalSecretRotationMetadata`/
  `ExternalSecretRotationRoleArn` (confirmed real, settable, echoed fields) and fixed the
  `owning-service` ListSecrets filter's more-permissive-than-AWS always-pass behavior (confirmed
  `OwningService` itself is correctly never populated — no input sets it in real AWS either).
  Found and fixed a second "state mutated before validation" instance, in `UpdateSecret`
  (`Description`/`KmsKeyId` applied before a same-call value update could fail). Did not touch
  `RotationRulesType`/`FilterNameStringType`/`SortByType` (already verified exhaustively above) or
  `primary-region`'s always-pass filter (single-region-scoped `ListSecrets` makes that a separate,
  lower-confidence question deferred rather than changed speculatively). Gates
  (`build`/`test -race`/`golangci-lint`/`go vet .` at repo root) all pass clean; no snapshot version
  bump (new fields are additive `omitempty`, verified round-tripping through `Snapshot`/`Restore`).
