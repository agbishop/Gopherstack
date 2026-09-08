---
service: sts
sdk_module: aws-sdk-go-v2/service/sts@v1.49.0   # version audited against (pinned in go.mod)
last_audit_commit: bfc0729e6                    # HEAD before this pass's changes
last_audit_date: 2026-08-20
overall: A                # 2026-08-29: errcodeaudit ERROR-path sweep. 2 confident findings
                           # (handler.go:335,337 "Sender"/"Receiver"), both verified clean false
                           # positives. These are the Query-protocol XML error envelope's <Type>
                           # field (SOAP-fault-actor classification: Sender=client fault,
                           # Receiver=server fault), not exception codes -- confirmed against
                           # awsxml.GetErrorResponseComponents (deserializers.go), which extracts
                           # only Code/Message/RequestID from the XML body for typed dispatch; Type
                           # is never read for errors.As matching by any op. STS DOES model typed
                           # exceptions elsewhere (12 in types/errors.go: ExpiredTokenException,
                           # MalformedPolicyDocumentException, etc.) -- confirmed correctly mapped
                           # to their real ErrorCode() strings (which differ from the Go type names,
                           # e.g. InvalidIdentityTokenException.ErrorCode()=="InvalidIdentityToken"),
                           # not the type names themselves. No fix needed.
                           # OutboundWebIdentityFederationDisabledException genuinely wired
                           # this pass (see GetWebIdentityToken below); the one remaining gap
                           # (JWTPayloadSizeExceededException) is a proven impossibility --
                           # AWS publishes no byte threshold anywhere searched (SDK doc
                           # comments, generated validators.go, public docs, web search) --
                           # so this service's only open gap is genuine, not deferred effort.
ops:
  AssumeRole: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "trust-policy Principal/Condition/Effect evaluation, ExternalId, role-chaining 1h cap, transitive tags, PackedPolicySize all previously verified. FIXED (gopherstack-41fl): the prior note's 'MFA absent (n/a for this op)' was wrong -- AssumeRoleInput.SerialNumber/TokenCode are real optional string members (aws-sdk-go-v2/service/sts@v1.45.4 api_op_AssumeRole.go:281,349, confirmed via the awsAwsquery serializer at serializers.go:929/946 which wires the literal wire keys SerialNumber/TokenCode -- the query-protocol serializer prefix is awsAwsquery_*, not awsQuery_*), and a trust policy with a Bool aws:MultiFactorAuthPresent condition was silently never enforced. Handler now parses both fields (handler_assume_role.go); validateMFAFields (validation.go, extracted from what was GetSessionToken-only inline logic so both ops share one validator) enforces the same SerialNumber/TokenCode pairing-and-format rules GetSessionToken already had, returning the same ErrMFACodeRequired/ErrTokenCodeWithoutSerial/ErrInvalidMFASerialNumber/ErrInvalidMFATokenCode. MFA presence (both fields well-formed) now feeds trust-policy enforcement two ways: validateMFACondition (trust_policy.go, mirrors validateExternalID's principal-independent OR-across-statements semantics) runs unconditionally whenever a RoleLookup resolves the role, and checkAssumeRoleTrust also threads aws:multifactorauthpresent into the general Principal-aware evaluator's conditionCtx (new Bool operator case in conditionOperatorHolds) so a single statement combining Principal and MFA Condition is evaluated jointly. A caller with no MFA against a policy requiring it now returns AccessDenied (existing ErrAccessDenied -> AccessDenied/403 mapping, unchanged). Does NOT cryptographically verify the TOTP is correct -- like GetSessionToken, there is no shared-secret store to check against, so 'MFA present' means a well-formed SerialNumber+TokenCode pair was supplied, matching GetSessionToken's existing, deliberate scope."}
  AssumeRoleWithSAML: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass: the real AssumeRoleWithSAMLInput (aws-sdk-go-v2/service/sts's api_op_AssumeRoleWithSAML.go) has ONLY PrincipalArn/RoleArn/SAMLAssertion/DurationSeconds/Policy/PolicyArns — RoleSessionName, SourceIdentity, and Tags were gopherstack-invented top-level wire parameters accepted by the handler (not real SDK request members). AWS instead derives these, plus Subject/SubjectType/Issuer/Audience/NameQualifier's issuer component, from the SAMLAssertion's own <NameID>/<Issuer>/<SubjectConfirmationData>/<Attribute> elements. Added saml_attributes.go's extractSAMLAssertionData to parse the assertion for the RoleSessionName/SourceIdentity/PrincipalTag:*/TransitiveTagKeys attributes and NameID/Issuer/Recipient elements per AWS's documented derivations; removed the three invented fields from AssumeRoleWithSAMLInput and stopped parsing them from the request form (handler_saml.go); buildSAMLResponse now sources Subject/SubjectType/Issuer/Audience from the assertion with the previous hardcoded/PrincipalArn-derived values retained only as fallbacks for minimal test assertions carrying none of these elements"}
  AssumeRoleWithWebIdentity: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass: the real AssumeRoleWithWebIdentityInput has no SourceIdentity or Tags request member either (only RoleArn/RoleSessionName/WebIdentityToken/DurationSeconds/Policy/PolicyArns/ProviderId) — AWS's doc comment for AssumeRoleWithWebIdentityOutput.SourceIdentity says explicitly \"You do this by adding a claim to the JSON web token.\" Removed both invented fields from AssumeRoleWithWebIdentityInput; added extractWebIdentitySourceIdentity/extractWebIdentityTags (web_identity.go) which read jwtClaimSourceIdentity (\"https://aws.amazon.com/source_identity\") and jwtClaimTags (\"https://aws.amazon.com/tags\", already used elsewhere in this package by GetWebIdentityToken for the same purpose) custom claims from the WebIdentityToken instead of top-level request params"}
  AssumeRoot: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "approved TaskPolicyArn allowlist, fixed 900s duration, arn:aws:sts::ACCT:assumed-root ARN shape re-verified correct. FIXED this pass: AssumeRootOutput.SourceIdentity (\"the source identity specified by the principal that is calling the AssumeRoot operation\" which \"persists across chained role sessions\") was always empty — AssumeRoot has no SourceIdentity input parameter, so it must inherit from the caller's own STS session; added AssumeRootInput.CallerSession (populated by handler_assume_root.go from the SigV4 Authorization header, mirroring the existing AssumeRole role-chaining pattern) and propagated its SourceIdentity into both the new session and the response"}
  GetCallerIdentity: {wire: ok, errors: ok, state: ok, persist: ok, note: "session-token mismatch -> InvalidClientTokenId (not AccessDenied), expired session -> ExpiredTokenException — no input parameters in the real API either; re-verified correct"}
  GetDelegatedAccessToken: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass: the real GetDelegatedAccessTokenInput has ONLY TradeInToken — no DurationSeconds member (confirmed against both api_op_GetDelegatedAccessToken.go's struct and its awsAwsquery serializer, neither of which reference DurationSeconds for this op). gopherstack had invented a DurationSeconds wire parameter (accepted by handler_delegated_access.go, validated against the AssumeRole 900-43200s range) that does not exist on the real operation. Removed the field from GetDelegatedAccessTokenInput and the handler's form parsing; the backend now always issues DefaultDurationSeconds (3600s) credentials since the caller has no way to influence the lifetime. (Prior-pass note retained: TradeInToken's JWT-shaped exp claim is still checked, returning ExpiredTradeInTokenException for an already-expired token.)"}
  GetFederationToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "federated-user ARN/ID shape, tag/policy-arn/packed-policy validation — re-verified field-for-field against GetFederationTokenInput/Output this pass, no changes needed"}
  GetSessionToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "MFA serial+code pairing and format validation, 900-129600s range — re-verified field-for-field against GetSessionTokenInput/Output this pass, no changes needed"}
  GetWebIdentityToken: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "SigningAlgorithm RS256/ES384 narrowing, SessionDurationEscalationException (CallerSession-based), and SDK-completeness listing from prior passes re-verified correct. FIXED this pass (parity-3 phase 2): OutboundWebIdentityFederationDisabledException is now genuinely wired -- see checkOutboundWebIdentityFederationEnabled in web_identity.go and the new AccountSettingsLookup optional-capability interface in store.go, backed by real (previously no-op-stub) state in services/iam (account.go's EnableOutboundWebIdentityFederation/DisableOutboundWebIdentityFederation/GetOutboundWebIdentityFederationInfo/OutboundWebIdentityFederationEnabled). Defaults to enabled so GetWebIdentityToken keeps working out of the box (matches TestIntegration_STS_GetWebIdentityToken, which never calls the IAM enable API first) -- disabling is an explicit account-setting opt-in, matching real AWS's requirement to explicitly enable the feature but without breaking every existing caller by defaulting to AWS's stricter off-by-default posture. JWTPayloadSizeExceededException remains unmodeled -- proven impossibility, see gaps below."}
  GetAccessKeyInfo: {wire: ok, errors: ok, state: ok, persist: ok, note: "session lookup then well-formed-prefix fallback to backend account ID — re-verified correct"}
  DecodeAuthorizationMessage: {wire: ok, errors: fixed, state: ok, persist: n/a, note: "HMAC-signed self-issued messages verified; foreign base64 blobs decoded permissively for emulator usability — re-verified correct. gopherstack-yatn orphan-code triage FIX: missing-EncodedMessage rejection emitted \"InvalidParameter\", which is not a real STS/AWS-Query code anywhere (absent from sts@v1.45.4 entirely, and absent from the AWS STS Common Errors page, which documents \"MissingParameter\"/\"InvalidParameterValue\" but no bare \"InvalidParameter\"). EncodedMessage is a required member (DecodeAuthorizationMessageInput, api_op_DecodeAuthorizationMessage.go) whose absence is exactly what the doc's \"MissingParameter\" ('A required parameter for the specified action isn't included in the request') describes; reclassified ErrMissingEncodedMessage into the same MissingParameter bucket as every sibling ErrMissingXxx sentinel in mapValidationErrorToCode (control: ErrMissingRoleArn et al.). Malformed-message-content handling (InvalidAuthorizationMessageException, this op's one genuinely declared exception) is untouched. Verified via TestDecodeAuthorizationMessageEmpty (pre-existing test's old \"InvalidParameter\" assertion corrected -- confirmed failing pre-fix)."}
families:
  trust-policy-evaluation: {status: ok, note: "Principal (AWS/Federated/Service/wildcard), Action (incl. wildcard glob), Effect Allow/Deny, Condition (StringEquals/StringLike/StringEqualsIgnoreCase/StringNotEquals/StringNotLike/Bool/Null/ArnEquals/ArnLike/ArnNotEquals/ArnNotLike + IfExists, case-insensitive keys) implemented in trust_policy.go and verified against the statements in AssumeRole/WithSAML/WithWebIdentity. Bool operator + aws:multifactorauthpresent condition key added (gopherstack-41fl) for AssumeRole only -- AssumeRoleWithSAML/WithWebIdentity have no SerialNumber/TokenCode request members in the real API (federated identities cannot present MFA through those operations), so a Bool MFA condition in a trust policy assumed via those two ops remains unenforced by design, matching AWS's own operation surface, not a gap in this emulator. FIXED (gopherstack-yg95): conditionOperatorHolds's default branch returned true for every operator it did not model, and an unknown condition key also returned true (satisfied) unconditionally -- both meant a restrictive trust policy's condition could be silently ignored. Added Null (tests key presence via conditionValue's known result, not value -- must run before the generic unknown-key fallback or Null:false would always pass through it) and ArnEquals/ArnLike/ArnNotEquals/ArnNotLike (AWS documents ArnEquals/ArnLike as behaving identically, both wildcard-capable; this emulator reuses the same general-purpose glob matcher as StringLike rather than AWS's six-segment-aware ARN matching, since trust-policy ARN conditions in practice wildcard within one segment, e.g. role/prod-*). Confirmed the IfExists suffix stripping in normalizeConditionOp is correct for every operator it runs before: IfExists only changes absent-key handling, which happens uniformly at the single !known check, not per-operator -- Null is the one AWS-documented exception (no IfExists variant), handled by returning before that check. Numeric*/Date*/IpAddress/NotIpAddress/BinaryEquals remain unenforced: STRUCTURAL, not deferred -- this evaluator has no numeric, timestamp, source-IP, or binary-valued request-context anywhere (confirmed by inventory: the only condition keys ever populated are sts:ExternalId, aws:PrincipalArn, aws:MultiFactorAuthPresent, and WebIdentity's per-issuer :aud/:sub claims, all string- or bool-valued) -- there is no value to compare and adding the operator without a real value would be dead plumbing. DECISION: both fallback paths (unmodeled operator, unmodeled/unknown key) remain fail-open (permit) rather than flipping to fail-closed, but now log at WARN (services/sts/trust_policy.go's warnUnmodeledCondition) naming the specific operator/key, closing the 'silent' half of the bug without a behavioral break for existing callers whose trust policies carry a condition on a key this emulator cannot evaluate. This mirrors the file's pre-existing, deliberate 'enforce only what is positively known' design (see evaluateAssumeRoleTrust's and conditionValue's docstrings) rather than reversing it unilaterally; flipping the global default to fail-closed is flagged as a call that would benefit from explicit human sign-off, since the same permissive-mock philosophy is embedded by design throughout this same file (and, per pkgs-catalog.md's shared conventions, plausibly elsewhere in the emulator) rather than being local to this one bug."}
  session-tag-validation: {status: ok, note: "key/value length, charset, aws: reserved prefix, case-insensitive dup detection, MaxTagCount=50, transitive-tag merge on role chaining — verified correct; AssumeRoleWithSAML's TransitiveTagKeys (assertion-derived, previously never wired to the session at all) is now also propagated, closing a related chaining gap"}
  locking: {status: ok, note: "InMemoryBackend.mu is *lockmetrics.RWMutex (New(\"sts\")) per pkgs-catalog.md; every new lock path added this pass (GetWebIdentityToken/AssumeRoot CallerSession lookups, and this pass's checkOutboundWebIdentityFederationEnabled) reuses the existing LookupSession/RLock accessors — no new raw sync.Mutex, no lock ordering changes"}
gaps:
  - "2026-08-14 (gopherstack-3tpf): independently re-confirmed via a mechanical struct-field diff (cmd/structfielddiff), not by re-reading this file's prior claims -- every Input/Output/nested struct across all 11 ops, expanded through AssumedRoleUser/Credentials/FederatedUser/PolicyDescriptorType/ProvidedContext/Tag, diffed field-by-field against aws-sdk-go-v2/service/sts@v1.45.4. Zero new gaps: every real field this SDK declares has a matching gopherstack field (Go casing differences like AssumedRoleID/AssumedRoleId excluded as known noise), matching this manifest's own A grade. No code changes made to this service this pass."
  - "STRUCTURAL (gopherstack-yg95): Numeric*/Date*/IpAddress/NotIpAddress/BinaryEquals trust-policy condition operators are unenforced for every condition key this evaluator carries, because none of those keys are numeric-, timestamp-, IP-, or binary-valued -- see the trust-policy-evaluation family note above for the full key inventory and the fail-open-plus-WARN-log decision. Not a deferred-effort gap: implementing any of these operators today would have nothing real to compare against."
  - "IMPOSSIBLE (re-confirmed gopherstack-yewt): JWTPayloadSizeExceededException (aws-sdk-go-v2/service/sts/types, dispatched specifically on GetWebIdentityToken's error branch) has no discoverable numeric threshold anywhere searched: (1) the generated SDK doc comment on the type itself says only 'The requested token payload size exceeds the maximum allowed size. Reduce the number of request tags...' -- no byte number; (2) aws-sdk-go-v2/service/sts@v1.44.0's validators.go's validateOpGetWebIdentityTokenInput only checks Audience/SigningAlgorithm required-ness and delegates Tags to validateTagListType (per-tag key/value length limits, not an aggregate payload-size limit) -- no length/size constraint of any kind is client-side-enforced for this op; (3) no botocore/smithy api-2.json model with a `length` trait for this newer STS operation was found in any locally-vendored SDK (aws-sdk-go v1.55.5's models/apis/sts predates GetWebIdentityToken entirely -- confirmed via `ls .../models/apis/sts` finding no api-2.json referencing this op); (4) WebSearch for 'JWTPayloadSizeExceededException STS GetWebIdentityToken maximum size bytes' returned only the same threshold-free doc comment, restated by boto3/re:Post/awsfundamentals.com sources, plus AWS's general (unrelated) guidance that STS credential/token sizes should never be assumed fixed. Implementing a threshold here would mean inventing an arbitrary number with no spec to verify it against -- the opposite of parity. Genuinely unimplementable without an undocumented number AWS does not publish. (bd: gopherstack-p05, follow-up -- OutboundWebIdentityFederationDisabledException, the other half of this original gap entry, WAS closed this pass, see GetWebIdentityToken above)"
  - "STALE ISSUE PREMISE (gopherstack-yewt re-triage): the follow-up issue's item (2), 'OutboundWebIdentityFederationDisabledException -- needs account-level settings model gopherstack lacks + no API to toggle,' is already fully resolved as of this same PARITY.md's GetWebIdentityToken row above (parity-3 phase 2) -- re-confirmed this pass by reading the actual code, not just this file: web_identity.go's checkOutboundWebIdentityFederationEnabled (called from GetWebIdentityToken, web_identity.go:365) gates on real state via services/iam/account.go's EnableOutboundWebIdentityFederation/DisableOutboundWebIdentityFederation/GetOutboundWebIdentityFederationInfo/OutboundWebIdentityFederationEnabled (all real methods, not stubs -- confirmed by reading their bodies), and both handler_test.go and web_identity_test.go carry OutboundWebIdentityFederationDisabledException regression coverage. No code change needed; the bd issue's premise predates the fix that already landed in this same file."
deferred:
  - "SESSION-POLICY EVALUATION: session Policy/PolicyArns are validated for shape/size (MalformedPolicyDocument, PackedPolicyTooLarge) and PackedPolicySize is computed, but the policy document's *content* is not enforced against subsequent API calls (no IAM policy-evaluation engine wired to session credentials). This mirrors the rest of the emulator's authz model and is out of scope for a service-local sts audit."
leaks: {status: clean, note: "Sessions map is bounded by (a) the background Janitor (sweepExpiredSessions, default 30s tick) when WithJanitor is configured, and (b) an opportunistic sweep on every storeSession once the map reaches sessionEvictThreshold=256, so unbounded growth cannot occur even with the janitor disabled. Janitor.Run selects on ctx.Done() and its worker.Group is Stop()'d on cancellation — no goroutine leak. No unbounded slices/maps found elsewhere in the package. New CallerSession lookups (AssumeRoot, GetWebIdentityToken) reuse the existing LookupSession path and add no new state. The new accountSettingsLookup field (parity-3 phase 2) is a single interface-valued pointer set once via SetOIDCLookup's optional-capability type assertion, read under the existing b.mu.RLock in checkOutboundWebIdentityFederationEnabled -- no new lock, no new goroutine."}
---

## Notes

Freeform findings and traps for the next auditor.

### Wire-format / protocol
STS is the AWS **query (XML)** protocol (`Version=2011-06-15`), not JSON. Every
response envelope in `models.go` is `XMLName` + `xmlns,attr` + `<Op>Result` +
`ResponseMetadata` in that field order — **this order is load-bearing for XML
marshaling** (Go's `encoding/xml` serialises struct fields in declaration order,
and `<Op>Result` must precede `<ResponseMetadata>` on the wire to match AWS).
**Trap for future `fieldalignment`/govet-driven cleanups**: running
`fieldalignment -fix` (or any tool that reorders struct fields for memory
packing) across the whole package will silently swap `<Op>Result` and
`ResponseMetadata` in every response struct in `models.go`, breaking wire
compatibility while still compiling and passing `go vet`/`golangci-lint`
(fieldalignment has no concept of XML field order). This was caught during
this sweep (five `TestBatch2_*_ResultBeforeMetadata` tests failed after a
package-wide `fieldalignment -fix` run) and reverted; **never run a
whole-package `fieldalignment -fix` here — scope it to a single non-wire file
(e.g. `backend.go`) or hand-edit the target struct instead.**

### Credentials / ID shapes (verified correct, no changes)
- Access key IDs: `ASIA` + 16 upper-alnum chars (`generateAccessKeyID`).
- Assumed-role ARN: `arn:aws:sts::ACCOUNT:assumed-role/ROLE_NAME/SESSION` — note
  that an IAM path is **stripped**: `role/team/dev/MyRole` → `assumed-role/MyRole/SESSION`,
  only the final path segment survives (`buildAssumedRoleArn`/`roleNameFromResource`).
  This is a common trap: naively keeping the full path produces a wire-invalid ARN.
- AssumedRoleId: `AROA` + 16-char derived suffix + `:` + session name
  (`deriveRoleID`) — deterministic from the role ARN so repeated `AssumeRole`
  calls for the same role produce a stable role-ID prefix.
- `Expiration` fields are RFC3339 strings (`time.RFC3339`), which is correct for
  the query/XML protocol (unlike JSON-protocol services, which need
  `pkgs/awstime.Epoch()` — STS does **not** want epoch numbers here).

### GetWebIdentityToken / GetDelegatedAccessToken are real ops, not gopherstack extensions
The pinned `aws-sdk-go-v2/service/sts@v1.43.5` ships
`api_op_GetWebIdentityToken.go` and `api_op_GetDelegatedAccessToken.go` — both
are genuine, documented AWS STS actions (apparently added to the API after an
earlier audit pass assumed otherwise). This sweep found and corrected three
places that encoded the stale "GetWebIdentityToken is not real" belief:
`Handler.GetSupportedOperations()`'s doc comment + missing list entry,
`sdk_completeness_test.go`'s `notImplemented` acknowledgement list, and
`TestParity_GetWebIdentityToken_NotInSupportedOps` (inverted to
`TestParity_GetWebIdentityToken_InSupportedOps`). **If a future SDK bump adds
more ops, re-run `TestSDKCompleteness` first** — it is the authoritative
sentinel for this class of drift (it diffs `GetSupportedOperations()` against
the real SDK client's method set via reflection).

### GetWebIdentityToken SigningAlgorithm — only two values are valid
`aws-sdk-go-v2/service/sts@v1.43.5`'s doc comment for `SigningAlgorithm` is
explicit: *"Valid values are RS256 (RSA with SHA-256) and ES384 (ECDSA using
P-384 curve with SHA-384)."* The emulator previously accepted a 9-value JOSE
allowlist (RS256/RS384/RS512/ES256/ES384/ES512/PS256/PS384/PS512) inherited
from generic JWT-library conventions rather than the STS API's actual
constraint — narrowed to `{RS256, ES384}` this pass. A test
(`TestRefinement2_GetWebIdentityTokenValidSigningAlgorithms`, renamed to
`TestRefinement2_GetWebIdentityTokenSigningAlgorithms`) had asserted the wrong
(permissive) behaviour as correct; it is now a table of accept/reject cases.

### GetDelegatedAccessToken TradeInToken — was an unvalidated pass-through
`types.ExpiredTradeInTokenException` exists in the SDK ("The trade-in token
provided in the request has expired and can no longer be exchanged for
credentials") and the input doc comment says the token "must be valid and
unexpired at the time of the request" — yet the handler accepted **any**
non-empty string forever. Fixed by adding `validateTradeInTokenExpiry`
(`tokenvalidation.go`), which — mirroring the existing JWT-exp handling already
used for `WebIdentityToken` — checks the `exp` claim only when the token is
JWT-shaped (three non-empty dot-separated segments); opaque test-fixture tokens
remain accepted unchanged (no external issuer keys are available to verify a
signature either way, matching the emulator's existing stance on
WebIdentityToken/SAMLAssertion). New sentinel `ErrExpiredTradeInToken` maps to
`ExpiredTradeInTokenException` / HTTP 400 in `mapErrorToCode`.

### Locking
`InMemoryBackend.mu` was a raw `sync.Mutex` (violates
`.claude/memories/pkgs-catalog.md`'s "never scatter raw sync.Mutex — use
lockmetrics.RWMutex" rule); every other audited service backend already uses
`lockmetrics.RWMutex`. Converted with per-call-site operation labels; read-only
accessors use `RLock`/`RUnlock`. No behavioural change — `Lock`/`Unlock` stayed
write-exclusive everywhere a map mutation (session store/delete) occurs.

### Not touched (shared files / out of scope)
`cli.go`'s `stsBk.SetRoleLookup(...)` / `stsBk.SetOIDCLookup(...)` wiring was
read but not modified — the `RoleLookup`/`OIDCLookup` interfaces in
`backend.go` are unchanged (additive-safe; no signature changes were needed).

### Re-audit 2026-07-11 (HEAD eb94f3c3, no changes made)
Ran the standard re-audit protocol before touching any code:
- `git diff ce30166a..eb94f3c3 -- services/sts/` (the commit that actually
  authored/committed this ledger, since `0407b38d` predates the sweep-3 squash
  merge and is not an ancestor of any commit on this branch) — **empty**, no
  local drift in `services/sts/` since the last audit.
- `go.mod` bumped `aws-sdk-go-v2/service/sts` v1.43.5 → v1.44.0 in the interim
  (dependency-upgrade commit `e51c0de9`, unrelated to sts specifically). Diffed
  the two module versions on disk
  (`go/pkg/mod/github.com/aws/aws-sdk-go-v2/service/sts@{v1.43.5,v1.44.0}`):
  zero `api_op_*.go` or `types/` differences — the only changes upstream were
  added `serde_snapshot`/`serde_snapshot_test.go` test-infrastructure files.
  No new/changed operations, no new error types to model.
- No `TODO`/`FIXME`/`XXX`/`HACK` markers in non-test source.
- All five gates re-verified green with zero changes:
  `go build`, `go vet`, `go test -race` (ok, 2.956s), `go fix -diff` (empty),
  `golangci-lint run` (0 issues).

Conclusion: nothing to fix this pass. All `ops`/`families` rows above are
carried forward unchanged from the 2026-07-05 audit; `gaps`/`deferred` items
remain open (still no reliable spec for the three unimplemented exception
types; session-policy-content enforcement still correctly out of scope for a
service-local audit).

### Re-audit 2026-07-24: AssumeRoleWithSAML/AssumeRoleWithWebIdentity had invented wire params
The 2026-07-11 (and earlier) audits repeatedly marked `AssumeRoleWithSAML` and
`AssumeRoleWithWebIdentity` `wire: ok` on the strength of "no stub" behavioral
checks — real state mutation, correct-looking response shapes — without
actually diffing `AssumeRoleWithSAMLInput`/`AssumeRoleWithWebIdentityInput`
field-for-field against the pinned SDK's `api_op_*.go` struct definitions.
Doing that diff this pass found both were accepting **top-level request
parameters that do not exist in the real API**:

- `AssumeRoleWithSAMLInput` (real fields: `PrincipalArn`, `RoleArn`,
  `SAMLAssertion`, `DurationSeconds`, `Policy`, `PolicyArns` — confirmed
  against `api_op_AssumeRoleWithSAML.go`) — gopherstack additionally accepted
  `RoleSessionName`, `SourceIdentity`, and `Tags` as if they were separate
  wire parameters, parsed straight off `r.FormValue(...)` /
  `Tags.member.N.*` in `handler_saml.go`.
- `AssumeRoleWithWebIdentityInput` (real fields: `RoleArn`,
  `RoleSessionName`, `WebIdentityToken`, `DurationSeconds`, `Policy`,
  `PolicyArns`, `ProviderId`) — gopherstack additionally accepted
  `SourceIdentity` and `Tags` the same way.

This is not a cosmetic gap: a real `aws-sdk-go-v2` client calling either
operation has **no way to set these fields on the wire** — the generated
`AssumeRoleWithSAMLInput`/`AssumeRoleWithWebIdentityInput` Go structs the SDK
serializes simply don't have the fields, so the emulator's prior behavior
(accepting them as if AWS did) could never be exercised by a real SDK client
and would only work with hand-rolled form-encoded requests bypassing the SDK
entirely — the opposite of parity.

The real mechanism, per both operations' `Output` doc comments, is that AWS
derives these values **server-side** from the credential material itself:
`AssumeRoleWithSAMLOutput.SourceIdentity` / `.Subject` / `.SubjectType` /
`.Issuer` / `.Audience` come from the SAML assertion's `<Attribute>` /
`<NameID>` / `<Issuer>` / `<SubjectConfirmationData>` elements (see
`saml_attributes.go`'s `extractSAMLAssertionData`), and
`AssumeRoleWithWebIdentityOutput.SourceIdentity` (and, by the same convention
this package already used for `GetWebIdentityToken`, session tags) come from
custom claims in the `WebIdentityToken` JWT (see `web_identity.go`'s
`extractWebIdentitySourceIdentity`/`extractWebIdentityTags` and
`jwtClaimSourceIdentity`/`jwtClaimTags` in `token_validation.go`).

**Lesson for future auditors**: "no stub" (real state mutation, plausible
response shape) is necessary but not sufficient for `wire: ok`. An op can look
completely real — correct XML envelope, correct error codes, a session
actually stored — while still accepting invented request parameters a real
SDK client could never send. The only way to catch this class of bug is a
literal field list diff against the SDK's generated `Input`/`Output` structs
(or, better, its `serializers.go`/`deserializers.go`, which are authoritative
for what actually goes on the wire).

### Re-audit 2026-07-24: GetDelegatedAccessToken had an invented DurationSeconds parameter
Same root cause as above, smaller blast radius: `GetDelegatedAccessTokenInput`
has exactly one field in the real SDK (`TradeInToken`) — confirmed against
both the struct definition in `api_op_GetDelegatedAccessToken.go` and its
`awsAwsquery_serializeOpDocumentGetDelegatedAccessTokenInput` serializer
(neither references `DurationSeconds`). gopherstack had invented a
`DurationSeconds` wire parameter, accepted by `handler_delegated_access.go`
and validated against the same 900-43200s range as `AssumeRole`. Removed;
the backend now always issues `DefaultDurationSeconds` (3600s) credentials.

### Re-audit 2026-07-24: SessionDurationEscalationException and AssumeRoot SourceIdentity
Two smaller closable gaps found by reading the SDK's per-operation error
dispatch tables (`deserializers.go`'s `awsAwsquery_deserializeOpError<Op>`
functions) and `Output` doc comments rather than just the `types/errors.go`
doc comments in isolation:

- `SessionDurationEscalationException` is dispatched specifically in
  `GetWebIdentityToken`'s error branch (alongside
  `JWTPayloadSizeExceededException` and
  `OutboundWebIdentityFederationDisabledException`, which remain unmodeled —
  see `gaps`). Its doc comment ("you cannot use this operation to extend the
  lifetime of a session beyond what was granted when the session was
  originally created") maps cleanly onto a caller using temporary STS
  credentials to request a `GetWebIdentityToken` JWT whose `DurationSeconds`
  would outlive their own session — implemented via a new
  `GetWebIdentityTokenInput.CallerSession` populated from the SigV4
  Authorization header, mirroring the existing `AssumeRole` role-chaining
  `CallerSession` pattern.
- `AssumeRootOutput.SourceIdentity` was always empty — `AssumeRootInput` has
  no `SourceIdentity` parameter, and the doc comment says source identity
  "persists across chained role sessions", so it must be inherited from the
  calling principal's own session. Added the same `CallerSession` pattern to
  `AssumeRoot`.

### Re-audit 2026-07-24 (parity-3 phase 2): OutboundWebIdentityFederationDisabledException wired, JWTPayloadSizeExceededException proven unimplementable

The final gap from the prior pass bundled two unrelated error types together;
each was re-investigated independently this pass.

**OutboundWebIdentityFederationDisabledException — closed.** The prior gap
entry said this "would need a cross-service account-settings flag gopherstack
does not currently model... with no other API in this codebase to toggle it."
That premise was checked, not re-asserted: `aws-sdk-go-v2/service/iam@v1.55.0`
turns out to genuinely ship `EnableOutboundWebIdentityFederation`,
`DisableOutboundWebIdentityFederation`, and
`GetOutboundWebIdentityFederationInfo` (real, documented IAM account-settings
operations — `IssuerIdentifier`/`JwtVendingEnabled` fields confirmed against
their `api_op_*.go` structs), and `services/iam/handler.go`/`handler_account.go`
already routed all three actions — but as **no-op stubs**: no backend state,
wrong response shape (a bare ack instead of the real
`IssuerIdentifier`/`JwtVendingEnabled` fields). So the account-settings
surface this gap needed did exist, just disconnected and half-broken.
Fixed by:
1. `services/iam/account.go`/`store.go`/`models_account.go`/`persistence.go`:
   gave the three ops real state (`outboundFederationEnabled bool`, default
   `true`; `outboundFederationIssuerURL(accountID)` computed on demand, not
   persisted since it's a pure function of `accountID`) and the correct wire
   shape. New `OutboundWebIdentityFederationEnabled() bool` accessor.
   (Flagged per this session's instructions: this is a real fix to
   `services/iam`, outside this task's three assigned services — kept minimal
   and covered by new tests in `account_test.go`/`handler_account_config_test.go`,
   but it has not received a full `services/iam` parity audit as part of this
   pass; `services/iam/PARITY.md` was deliberately NOT touched/re-graded.)
2. `services/sts/store.go`: added a new `AccountSettingsLookup` interface
   (`OutboundWebIdentityFederationEnabled() bool`) and an optional-capability
   type assertion inside the *existing* `SetOIDCLookup` method — when the
   value passed to `SetOIDCLookup` also implements `AccountSettingsLookup`
   (the real IAM backend now does), it's captured as
   `b.accountSettingsLookup` too. This was a deliberate design constraint:
   this session was expressly forbidden from editing `cli.go`, and `grep`
   confirms `cli.go`'s `stsBk.SetOIDCLookup(iamBk)` (in `wireIAMToSTS`) is the
   **only** call site anywhere in the codebase that cross-wires IAM into STS
   — so a conventional new `SetAccountSettingsLookup` call would have
   required a `cli.go` edit. Piggybacking the capability check onto the
   existing call means `cli.go` needed zero changes (verified:
   `git diff --stat cli.go` is empty) while still giving STS a live,
   correctly-defaulted link to IAM's real state.
3. `services/sts/web_identity.go`: `GetWebIdentityToken` now calls
   `checkOutboundWebIdentityFederationEnabled()` first, returning the new
   `ErrOutboundWebIdentityFederationDisabled` (mapped to
   `OutboundWebIdentityFederationDisabledException`/400 in `handler.go`) when
   an `AccountSettingsLookup` is wired AND reports the feature disabled. When
   no lookup is wired (every existing unit test that builds an isolated
   `sts.NewInMemoryBackend()` without calling `SetOIDCLookup`), the check is
   a no-op — permissive by default, matching `validateOIDCProvider`'s existing
   convention for the same reason. IAM's default (`true`/enabled) was chosen
   specifically so `test/integration/sts_test.go`'s
   `TestIntegration_STS_GetWebIdentityToken` (which calls `GetWebIdentityToken`
   with no setup) keeps passing against the full `cli.go`-wired stack —
   defaulting to AWS's real off-by-default posture would have broken that
   test and every other zero-setup caller.

**JWTPayloadSizeExceededException — proven impossibility, not fixed.** See
the `gaps` entry above for the specific four things checked (SDK doc comment,
`validators.go`, absence of a botocore/smithy model for this newer op in any
locally-vendored SDK, and a live web search) — none surfaced a byte-size
number. Implementing a threshold here would mean inventing a number with
nothing to verify it against.

### Re-audit 2026-08-13 (gopherstack-yg95): trust-policy conditions failing open, general shape behind the MFA bug

gopherstack-41fl (fixed in `89726ecb1`) turned out to be one instance of a
broader bug in `conditionOperatorHolds`: its `default` case returned `true`
for *any* operator the switch did not model, and a separate `!known` check
earlier in the function returned `true` for *any* condition key
`conditionValue` did not resolve. Both meant a trust policy written to
restrict `AssumeRole` could have its restricting condition silently ignored.

**Inventory first.** Grepped every place this evaluator ever populates a
condition value (`conditionValue`'s two hardcoded cases plus every
`conditionCtx` map literal in `assume_role.go`/`saml.go`/`web_identity.go`):
`sts:ExternalId` (string, AssumeRole only), `aws:PrincipalArn` (string ARN,
AssumeRole only), `aws:MultiFactorAuthPresent` (bool string, AssumeRole
only), and per-issuer `{host}:aud`/`{host}:sub` (string, WebIdentity only,
only added to the map when the JWT claim was actually present — this is the
one place in the evaluator that already models true request-dependent key
absence, not merely "unmodeled key type"). No key anywhere is numeric,
timestamp, source-IP, or binary-valued, because nothing threads a request
timestamp or client IP into `trustEval` at all — confirmed by grep, zero
hits for `SourceIp`/`RemoteAddr`/`CurrentTime` in the package.

**Implemented:** `Null` (presence-only, evaluated before the generic
`!known` fallback so `Null:false` — key must be present — isn't defeated by
that same fallback) and the `Arn*` family (`ArnEquals`/`ArnLike` identical
and wildcard-capable per AWS docs, `ArnNotEquals`/`ArnNotLike` negated),
reusing the existing glob matcher rather than AWS's six-segment-aware ARN
matching. **Deliberately not implemented:** `Numeric*`, `Date*`,
`IpAddress`/`NotIpAddress`, `BinaryEquals` — structural, no key of that
value type exists to compare against; adding the operator without a real
value to feed it would be dead plumbing that looks like enforcement and
isn't.

**Fail-open kept, not flipped, but no longer silent.** Both remaining
fallback paths (truly unmodeled operator; unmodeled/unknown key) still
permit by default — reversing that default repo-wide is a bigger call than
this bug fix, since the same "enforce only what is positively known"
posture is *documented as deliberate* elsewhere in this same file
(`evaluateAssumeRoleTrust`'s and `conditionValue`'s existing doc comments),
and flipping it would silently start denying `AssumeRole` calls for any
existing caller whose policy happens to carry a condition on a
structurally-unsupported key, even one unrelated to what that caller's test
actually cares about. Instead, both paths now log at `WARN`
(`warnUnmodeledCondition`, via `logger.Load(context.Background())` — no
request-scoped context reaches this deep into trust-policy evaluation, same
pattern used in `services/autoscaling/elbv2_targets.go`) naming the specific
operator and key, so the gap is discoverable at runtime instead of silent —
that was the actual harm named in the bug report. **Flagged for human
review:** whether the emulator's trust-policy evaluator (and any sibling
evaluator elsewhere in the repo built on the same permissive-mock
philosophy) should default to fail-closed is a product-level tradeoff
between "safe by default" and "doesn't silently break existing working
setups," not a purely technical one — this pass implemented what could be
implemented and made the remaining gap loud rather than making that call
unilaterally.

Also confirmed `normalizeConditionOp`'s `IfExists`-suffix stripping is
correct for every operator that reaches it: `IfExists` only changes
absent-key behavior, decided once at the single `!known` check rather than
per-operator, so stripping the suffix before the per-operator switch cannot
change any operator's present-key comparison semantics. `Null` is AWS's one
documented exception (no `IfExists` variant — presence-testing is already
its job) and is special-cased to run before that check entirely.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed InternalFailure**: same survey/rationale as
gopherstack-3a8t (see autoscaling's entry). `RouteMatcher` now falls back to
`service.MatchesUserAgentMarker(c.Request().Header, "api/sts")` (verified against the
pinned `sts@v1.45.4/api_client.go:651` `AddSDKAgentKeyValue` call) only on the `ReadBody`
failure branch, leaving the existing `Version`-substring matching untouched.

**`dispatch()`'s single `r.ParseForm()` call was left as-is, verified safe rather than
migrated.** `ExtractOperation`/`ExtractResource` already use `httputils.ReadBody` (via a
manual `parseFormValues` helper), not `r.ParseForm()`, so `dispatch()`'s call is the
*only* `ParseForm()` call for a given request -- the docdb/neptune double-call landmine
(a second call silently seeing a cached-empty, non-nil `r.PostForm` instead of the real
read error) requires a *second* call to manifest, and there isn't one here. Confirmed by
the oversized-body test: it surfaces `InternalFailure` directly, with no intermediate
`MissingAction` step.

Proof: `TestHandler_OversizedBodySurfacesInternalFailure` in `handler_oversized_body_test.go`
adds a new registry-routed test client (`newTestSTSRoutedClient`) -- this package's other
helpers (`newTestHandler`+`postForm`, `buildSTSClient`+`newEchoServer`) all mount
`Handler()` directly, bypassing `RouteMatcher` entirely, so none of them could prove the
routing half of this fix. Confirmed failing pre-fix with `UnknownError`; passes now with
`InternalFailure`. `TestHandler_NormalSizedBodyStillRoutes` is the regression guard.
Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race ./services/sts/...`
(pass), `golangci-lint run ./services/sts/...` (0 issues).
### Re-audit 2026-08-20: wrapper-key / nested-shape wire sweep — zero live bugs found

Campaign-wide sweep for response-envelope-name, nesting, and per-member shape
bugs (the class that made up ~62 real bugs across 31 other services this
session). Protocol reconfirmed from the pinned SDK, not assumed: `sts` is
`awsAwsquery_*` (query/XML), confirmed at
`aws-sdk-go-v2/service/sts@v1.45.4/deserializers.go:26` and `api_client.go:12`
(`aws/protocol/query` import). Per the campaign brief's own caveat, XML
element-name matching is case-insensitive (`strings.EqualFold`, verified at
every `case strings.EqualFold(...)` site in `deserializers.go`) for both
member decoding **and** error-code routing (`deserializeOpError<Op>`), so
casing near-misses are structurally impossible to matter here and were not
hunted.

**Method**: for each of the 11 ops, diffed gopherstack's `models.go`
Input/Output/Result structs field-for-field against the pinned SDK's
`api_op_*.go` `*Output` structs and `types/types.go` (`Credentials`,
`AssumedRoleUser`, `FederatedUser`, `PolicyDescriptorType`), then verified
every `<Op>Result` envelope element name against the live
`decoder.GetElement("<Op>Result")` call in each op's `HandleDeserialize` in
`deserializers.go` (all 11 confirmed exact-match, see table below), then
cross-checked the full per-op typed-error switch list in each
`deserializeOpError<Op>` function against `handler.go`'s `mapErrorToCode`.

**Result: zero new wire-shape bugs.** Every Input/Output field, every
`<Op>Result` envelope name, every list/struct nesting path
(`PolicyArns.member.N.arn`, `ProvidedContexts.member.N.ProviderArn`/
`.ContextAssertion`, `Tags.member.N.Key`/`.Value`, `TransitiveTagKeys.member.N`,
`Audience.member.N`, and — notably — `AssumeRootInput.TaskPolicyArn`, which is
**not** a flat string on the wire but a nested `PolicyDescriptorType` object
serialized as `TaskPolicyArn.arn=...`, confirmed via
`serializers.go:1058-1063` + `serializers.go:796-806`) already matches the
pinned SDK exactly. `handler_assume_root.go` already reads
`TaskPolicyArn.arn` first (with a documented flat-key fallback for
non-SDK callers) — this looked like a live candidate for the brief's pattern
(b) "wrong nesting level" bug class and turned out to be a prior pass's
correct fix, verified clean, not a new bug.

**Credentials**: real member names re-confirmed at
`aws-sdk-go-v2/service/sts@v1.45.4/types/types.go:34-56` —
`AccessKeyId`/`Expiration`/`SecretAccessKey`/`SessionToken`, all four
`This member is required`. Note this is **`SecretAccessKey`**, not
`SecretKey` (the cognitoidentity trap named in this campaign's brief does not
apply here — gopherstack's `Credentials.SecretAccessKey`
(`models.go:154`) already uses the correct name). Expiration is parsed via
`smithytime.ParseDateTime` (`deserializers.go:1950`), the smithy `date-time`
(RFC3339) format; gopherstack emits `expiration.Format(time.RFC3339)`
everywhere a `Credentials` value is built (`assume_role.go`, `assume_root.go`,
`delegated_access.go`, `federation.go`, `saml.go`, `session_tokens.go`,
`web_identity.go`) — format matches.

**Three assume-role variants**: confirmed gopherstack uses three **separate**
Go response structs (`AssumeRoleResult`, `AssumeRoleWithSAMLResult`,
`AssumeRoleWithWebIdentityResult`), not one shared struct — so the pattern-(a)
leak risk the brief calls out (a SAML-only or web-identity-only member
leaking into plain `AssumeRole`) is structurally prevented. Field-by-field
diff against each op's own `*Output` struct
(`api_op_AssumeRole.go:369`, `api_op_AssumeRoleWithSAML.go:269`,
`api_op_AssumeRoleWithWebIdentity.go:299`) confirms no cross-op leak in
either direction: `AssumeRoleResult` has exactly
{AssumedRoleUser,Credentials,PackedPolicySize,SourceIdentity}; SAML adds
{Audience,Issuer,NameQualifier,Subject,SubjectType}; WebIdentity adds
{Audience,Provider,SubjectFromWebIdentityToken} instead — matching each op's
real `Output` exactly, no overlap fabricated.

**Envelope / `ResponseMetadata`**: every op's `<Op>Result` child-element name
matches the live `decoder.GetElement(...)` call byte-for-byte:
AssumeRole/AssumeRoleWithSAML/AssumeRoleWithWebIdentity/AssumeRoot/
DecodeAuthorizationMessage/GetAccessKeyInfo/GetCallerIdentity/
GetDelegatedAccessToken/GetFederationToken/GetSessionToken/
GetWebIdentityToken — all 11 confirmed against `deserializers.go` lines
74/195/322/452/567/679/788/897/1015/1133/1245 respectively. `ResponseMetadata`
+ `RequestId` is present and correctly nested after `<Op>Result>` in every
`models.go` response struct (field order verified load-bearing per this
file's own prior "Wire-format / protocol" note above; re-confirmed
`gofmt`/`golangci-lint` don't reorder it — see gates below). Note: the real
`aws-sdk-go-v2` client actually reads the request ID from the
`RequestIDRetriever` deserialize-middleware (HTTP-header-driven on the
success path — no `ResponseMetadata` field exists on any success `*Output`
Go struct in the pinned SDK) rather than from the XML body's
`<ResponseMetadata><RequestId>` on success; gopherstack does not set an
`X-Amzn-Requestid` HTTP header on STS success responses (confirmed by grep —
no such `Header().Set` call anywhere in `services/sts/`, unlike e.g.
`services/glacier`/`services/emrserverless`). This does not affect any
response body field a client reads, and setting response headers is
infrastructure shared across the whole router, not an sts-local wire-shape
concern, so it is disclosed here but intentionally not touched by this
service-scoped pass.

**One disclosed, NOT fixed, finding** (error-taxonomy, not
wrapper-key/nesting, so kept minimal per the brief's error-shape callout):
`errors.go`'s `ErrIDPRejectedClaim` sentinel (doc comment: "returned when the
identity provider rejects the claim") is coalesced in
`handler.go:305`'s `mapNamedExceptionToCode` into the same case as
`ErrAccessDenied`, both emitting wire `Code: "AccessDenied"`. The real STS
`IDPRejectedClaimException.ErrorCode()` is `"IDPRejectedClaim"`
(`types/errors.go:116-119`), HTTP 403 (confirmed against the live AWS API
reference for `AssumeRoleWithWebIdentity`/`AssumeRoleWithSAML` — both list
`IDPRejectedClaim`, HTTP Status Code 403, as a **typed** error distinct from
the untyped `AccessDenied` bucket in each op's
`deserializeOpError<Op>` switch, `deserializers.go`: AssumeRoleWithSAML
func at line 221, `IDPRejectedClaim` case at line 249; AssumeRoleWithWebIdentity
func at line 348, `IDPRejectedClaim` case at line 379). If
`ErrIDPRejectedClaim` were ever wired to a live code path, an SDK client
would currently get a generic `smithy.GenericAPIError` instead of the typed
`*types.IDPRejectedClaimException`. **Not fixed this pass**: grep confirms
`ErrIDPRejectedClaim` is declared but never constructed/returned anywhere in
`services/sts/*.go` today — it is dead code, so there is no live wire
response to hand-revert-and-reprove a fix against, and this package's tests
are all external (`package sts_test`; `export_test.go` is off-limits per this
session's constraints), so `mapErrorToCode` cannot be unit-tested directly
without a real caller. Flagged here with citations so a future pass wiring
up IDP-rejection detection uses the right code from the start.

**Genuinely unimplementable / structural, not re-litigated this pass** (already
correctly disclosed above, re-confirmed still accurate):
`JWTPayloadSizeExceededException` (no published threshold),
`RegionDisabledException` (no per-region STS activation concept in this
emulator — same class of structural gap, not previously called out by name in
this file; `IDPCommunicationError` similarly has no live path since this
emulator never makes an outbound call to a real external IdP to fail).

**Provenance correction**: this file's header previously read
`last_audit_commit: 2d47b51d4` / `last_audit_date: 2026-07-29`, but
`git log --oneline 2d47b51d4..HEAD -- services/sts/` shows four further
commits touching this service (`69bbb940a` 2026-08-15, `d39bf33e4`
2026-08-11, `e22eb6be1` 2026-07-30, `7abc9be9a` 2026-08-16) — the last of
which (`7abc9be9a`) is what actually authored this file's current `AssumeRole`
MFA note and the `strictConditions`/`UserLookup` code in `assume_role.go`/
`store.go`/`interfaces.go`/`provider.go`. The header was never updated to
match, so it understated the true last-touched date by three weeks. Corrected
above to `bfc0729e6` / 2026-08-20 (this session's starting HEAD).

## gopherstack-wlo1 (2026-08-22): Handler()'s method-not-allowed branch was untyped

`Handler()`'s own `if c.Request().Method != http.MethodPost { return
c.String(http.StatusMethodNotAllowed, "Method not allowed") }` guard
(handler.go) wrote a bare text/plain 405. STS is AWS Query/XML
(`sts@v1.45.4` `awsAwsquery_` prefix), whose deserializer expects the
wrapped `<ErrorResponse><Error>` document; plain text doesn't decode, so a
real client saw a raw XML-unmarshal failure rather than a typed API error.

Reachability: `RouteMatcher` (handler.go) matches purely on Content-Type
and whether the body contains `Version=2011-06-15` -- it never inspects the
HTTP method -- so a request with any other method still routes to
`Handler()`. (GET is separately special-cased into a 200
`GetSupportedOperations` response above this check, so the proof uses PUT,
not GET.)

Fixed: routes through the existing `handleError(ctx, c,
fmt.Errorf("%w: method not allowed", ErrValidation))` -- `ErrValidation`
already maps to `invalidParamValue` ("InvalidParameterValue") at 400 in
`mapErrorToCode`'s switch, so no new exception vocabulary was introduced.
The status code changes from the old 405 to 400 as a result (matching every
other validation-class STS error); `TestHandler_MethodNotAllowed`
(handler_test.go) updated accordingly.

Proof: `TestHandler_WrongMethodSurfacesInvalidParameterValue`
(`handler_dispatch_malformed_test.go`) drives a real STS client's
`GetCallerIdentity` through a Finalize-stage middleware that rewrites the
request's HTTP method to PUT post-signing, keeping the form-encoded body
and Content-Type intact. Hand-reverted `handler.go` to `git show HEAD`,
confirmed the test fails with `apiErr.ErrorCode() == "UnknownError"`,
restored the fix, `md5sum`-confirmed byte-identical.

## 2026-08-29: error-path re-verification (failure-side wire shape) -- no new findings

Independent re-run of this session's error-path campaign (HTTP status / AWS
error code / whether an operation actually models that code, per its own
`awsAwsquery_deserializeOpError<Op>` switch in `deserializers.go`,
sts@v1.45.4). This class was already fully audited by the 2026-08-20
wrapper-key/nested-shape sweep above ("cross-checked the full per-op
typed-error switch list in each `deserializeOpError<Op>` function against
`handler.go`'s `mapErrorToCode`"); `git log --since=2026-08-20 -- services/sts/`
shows no commits touching error-path logic since. Independently re-extracted
all 11 ops' declared code sets from the pinned SDK and re-diffed against
`handler.go`'s `mapValidationErrorToCode`/`mapNamedExceptionToCode` --
confirms the prior finding: zero live bugs, and the one previously-disclosed
gap (`ErrIDPRejectedClaim` coalesced into `AccessDenied` in
`mapNamedExceptionToCode`, `handler.go:310`) remains dead code -- still
never constructed anywhere in `services/sts/*.go` (grep-confirmed), so
there is no live wire response for it to be a bug in yet. No changes made.
