---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: acmpca
sdk_module: aws-sdk-go-v2/service/acmpca@v1.50.0   # version audited against
last_audit_commit: 3cec3729                          # HEAD when this manifest was written
last_audit_date: 2026-08-20
overall: A            # wrapper-key/nested-shape re-audit this pass: zero new wire bugs found (see Notes)
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateCertificateAuthority: {wire: ok, errors: ok, state: ok, persist: ok, note: "ROOT auto-signs+activates; SUBORDINATE -> PENDING_CERTIFICATE. FIXED THIS PASS: IdempotencyToken now deduplicated (5-min window); KeyStorageSecurityStandard/UsageMode/RevocationConfiguration now accepted, validated, stored, and echoed (previously entirely absent from the model -- a gap not listed in the prior manifest, found via full field-diff)."}
  DescribeCertificateAuthority: {wire: ok, errors: ok, state: ok, persist: ok, note: "reports RestorableUntil, LastStateChangeAt (new field, fixed this pass), KeyStorageSecurityStandard, UsageMode, RevocationConfiguration (omitted entirely when unconfigured, matching a nil *types.RevocationConfiguration). A CA past its RestorableUntil deadline now correctly returns ResourceNotFoundException (fixed this pass -- see gaps)."}
  ListCertificateAuthorities: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: ResourceOwner now validated and enforced -- SELF/empty lists this account's CAs, OTHER_ACCOUNTS returns an empty page (no cross-account sharing modeled), anything else is InvalidArgsException (corrected gopherstack-r3pr, 2026-08-30 -- was previously the fabricated InvalidParameterException). Also now filters out CAs past their RestorableUntil deadline. gopherstack-wksw (2026-08-29, constraint-not-honoured sweep): MaxResults' documented ceiling (api_op_ListCertificateAuthorities.go: 'Although the maximum value is 1000, the action only returns a maximum of 100 items.') was not applied -- a caller-requested MaxResults above 100 (up to the accepted max of 1000) returned that many items in one page instead of AWS's hard 100-item page cap. Fixed: certificate_authorities.go's ListCertificateAuthorities now clamps to defaultMaxItems (100) whenever the requested value is <=0 or >100, matching the doc comment exactly (not just the omitted-parameter default). TestInMemoryBackend_ListCertificateAuthorities_MaxResultsCappedAt100 (list_certificate_authorities_maxresults_test.go) confirmed failing pre-fix for MaxResults=500 and MaxResults=1000 (both returned the full requested count against 105 seeded CAs)."}
  DeleteCertificateAuthority: {wire: ok, errors: ok, state: ok, persist: ok, note: "tracks RestorableUntil (default 30d) and sets LastStateChangeAt (new field, fixed this pass)."}
  UpdateCertificateAuthority: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (gopherstack-5xc, state-machine sweep): the op's own doc comment ('Your private CA must be in the ACTIVE or DISABLED state before you can update it') was never enforced -- a CA in CREATING/PENDING_CERTIFICATE/DELETED could be flipped straight to ACTIVE via UpdateCertificateAuthority, bypassing ImportCertificateAuthorityCertificate/RestoreCertificateAuthority entirely. Now rejects with InvalidStateException unless the CA's current status is ACTIVE or DISABLED. Prior pass: accepts RevocationConfiguration (omitting the field leaves the CA's existing configuration unchanged, matching the real API's documented semantics -- distinguished from an explicit null via a custom UnmarshalJSON tracking which wire keys were present); sets LastStateChangeAt on status change."}
  RestoreCertificateAuthority: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: clears RestorableUntil and now correctly rejects a restore attempted after the RestorableUntil deadline (ResourceNotFoundException, matching real AWS permanently removing the CA once its restoration window ends) -- see caGet/casInRegion in store.go, the single choke point every CA read/write goes through."}
  GetCertificateAuthorityCsr: {wire: ok, errors: ok, state: ok, persist: ok}
  ImportCertificateAuthorityCertificate: {wire: ok, errors: ok, state: ok, persist: ok, note: "sets LastStateChangeAt (fixed this pass)."}
  GetCertificateAuthorityCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  IssueCertificate: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (severe wire bug, found via field-diff): the certificate ARN's final path segment must be the certificate's own serial number in decimal (see IssueCertificateOutput's doc example) -- gopherstack instead appended an unrelated crypto/rand ID, meaning every issued cert ARN was wrong-shaped. Also FIXED: IdempotencyToken deduplication (5-min window); TemplateArn now gates ApiPassthrough per the real API's documented 'ignored unless an APIPassthrough/APICSRPassthrough template variant is selected' rule; ApiPassthrough now really applies Subject/KeyUsage/ExtendedKeyUsage/SubjectAlternativeNames(DNS+IP+email)/CustomExtensions overrides to the issued cert (previously silently ignored entirely). UsageMode=SHORT_LIVED_CERTIFICATE now enforces the real API's 7-day validity cap. Still not implemented: ApiPassthrough.Extensions.CertificatePolicies, the ASN1Subject RDN types beyond CommonName/Country/Organization/OrganizationalUnit/State/Locality/SerialNumber, and the GeneralName variants beyond DnsName/IpAddress/Rfc822Name -- all explicitly REJECTED (InvalidArgsException, corrected gopherstack-r3pr) rather than silently dropped when a caller sets them; TemplateArn's per-template default extension profile (e.g. SubordinateCACertificate_PathLenN's path-length constraint) is not modeled beyond the APIPassthrough-gating behavior. END_DATE validity type is still treated as epoch seconds like ABSOLUTE rather than true UTCTime/GeneralizedTime -- pre-existing intentional simplification, unchanged this pass (see Traps)."}
  GetCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  RevokeCertificate: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (gopherstack-5xc, state-machine sweep): RevokeCertificate's own deserializeOpError uniquely models RequestAlreadyProcessedException ('Your request has already been completed') among every op in this service -- no other op declares it -- evidence a repeat revocation must be rejected. Previously a second RevokeCertificate call on an already-REVOKED certificate silently succeeded and overwrote RevokedAt/RevocationReason; now rejected. Prior pass CORRECTED: the older manifest's gap note ('does not require CRL/OCSP to be enabled before revoking') was a misdiagnosis -- re-checked against the real SDK's RevokeCertificate doc comment, which describes CRL/OCSP as purely optional side-effects of revocation, not a precondition for it. No such requirement exists in the real API; that was never actually a gap and no fix was needed there."}
  ListPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePermission: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: Principal now validated to be exactly 'acm.amazonaws.com', the only value the real API accepts per CreatePermissionInput.Principal's doc comment ('At this time, the only valid principal is acm.amazonaws.com')."}
  DeletePermission: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCertificateAuthorityAuditReport: {wire: ok, errors: ok, state: ok, persist: ok, note: "synchronous SUCCESS; real AWS is async (CREATING->SUCCESS/FAILED) but this is a reasonable simplification for an emulator."}
  DescribeCertificateAuthorityAuditReport: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  TagCertificateAuthority: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: enforces the real API's 50-tag-per-CA limit, returning TooManyTagsException (mapped to the real exception's ErrorCode) when exceeded."}
  UntagCertificateAuthority: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: MaxResults/NextToken now paginate for real (via pkgs/page, same pattern as every other list op in this service) instead of always returning the full tag set in one page. The invented 'ListTagsForCertificateAuthority' op alias was DELETED (see Notes) -- it does not exist anywhere in aws-sdk-go-v2; the real op is ListTags only."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "NEW (found this pass): CertificateAuthority.FailureReason (types.FailureReason: REQUEST_TIMED_OUT/UNSUPPORTED_ALGORITHM/OTHER) and CertificateAuthorityStatus's FAILED/EXPIRED enum values are entirely unmodeled -- CreateCertificateAuthority is synchronous and always succeeds or returns an immediate validation error, so no CA ever reaches FAILED, and no expiry-driven ACTIVE->EXPIRED transition is simulated. FailureReason is correctly never emitted (matching the real API omitting it whenever Status != FAILED), so this is a state-machine depth gap, not a wire-shape bug -- disclosed, not fixed (would need a new terminal status + expiry sweep, out of scope for a wrapper-key/nesting sweep)."
  - "NEW (found this pass): CertificateAuthorityConfiguration.CsrExtensions (nested CsrExtensions{KeyUsage, SubjectInformationAccess->AccessDescription{AccessMethod,GeneralName}}) is accepted by neither CreateCertificateAuthority's input decoding (caConfigInput has no CsrExtensions field) nor echoed by Describe/List -- silently dropped on the request side rather than rejected. Real AWS would echo a caller-supplied CsrExtensions back on every subsequent Describe/List; gopherstack never stores it, so a caller setting it gets no error but also never sees it round-trip. Disclosed, not fixed -- same class of gap as the already-documented ASN1Subject exotic RDN types, but this one lacks the explicit-rejection treatment those get in decodeASN1Subject/decodeExtensions (handler_certificates.go); a caller has no signal the field was ignored."
  - ApiPassthrough.Extensions.CertificatePolicies is rejected (InvalidArgsException, corrected gopherstack-r3pr) rather than implemented -- would require arbitrary OID/PolicyQualifier ASN.1 encoding beyond a simple pkix.Extension passthrough
  - ApiPassthrough.Subject's exotic RDN types (DistinguishedNameQualifier, GenerationQualifier, Initials, Pseudonym, Surname, Title, CustomAttributes) are rejected rather than implemented -- crypto/x509's pkix.Name has no direct fields for most of these
  - ApiPassthrough.Extensions.SubjectAlternativeNames' exotic GeneralName variants (OtherName, DirectoryName, EdiPartyName, UniformResourceIdentifier, RegisteredId) are rejected rather than implemented -- only DnsName/IpAddress/Rfc822Name (the three Terraform's aws_acmpca_certificate resource actually exposes) are modeled
  - TemplateArn's per-template default X.509 extension profile (e.g. SubordinateCACertificate_PathLenN's CA path-length constraint, OCSPSigningCertificate/CodeSigningCertificate's preset KeyUsage/ExtendedKeyUsage) is not modeled; only the documented APIPassthrough/APICSRPassthrough-gating behavior (whether ApiPassthrough is honored at all) is implemented -- every issued cert uses the same flat extension baseline (optionally overridden by ApiPassthrough) regardless of TemplateArn's specific value
  - RevocationConfiguration.CrlConfiguration/OcspConfiguration's CNAME fields (CustomCname, OcspCustomCname) and S3BucketName are accepted as any non-empty string; the real API's RFC2396/S3-bucket-naming-rule validation is not enforced
  - IssueCertificate's END_DATE validity type is still treated as Unix epoch seconds (same as ABSOLUTE) rather than true UTCTime/GeneralizedTime -- pre-existing intentional simplification, not touched this pass (see Traps)
  - DELETED CAs past their RestorableUntil deadline are hidden from every read path (Describe/List/Get/Issue/etc. all treat them as not-found, matching real AWS's user-visible behavior) and RestoreCertificateAuthority correctly rejects them, but the row is not physically freed from the in-memory store.Table until the next process Reset() -- consistent with how every other terminal-state resource in this backend (revoked certs, etc.) is retained rather than garbage-collected; not a new leak, just not a true memory-reclaiming sweep
deferred: []              # both prior deferred items (ApiPassthrough, TemplateArn) now substantially implemented -- remaining edges tracked under gaps above
leaks: {status: clean, note: "no goroutines/janitors in this service; all state lives in store.Table/store.Index behind the coarse b.mu lockmetrics.RWMutex, matching pkgs-catalog guidance. The RestorableUntil-deadline enforcement added this pass is a lazy read-time filter (caGet/casInRegion in store.go), not a background sweep -- no new goroutine, no new lock, no new leak surface."}
---

## Notes

### 2026-09-04 state-machine sweep (gopherstack-5xc)

Targeted the CA state machine and revocation-semantics bug classes specifically
(brief patterns #1 and #3), per-op doc comments read in full against
`acmpca@v1.50.0`. **2 real bugs found and fixed**, both confirmed by
neutering the fix and observing the new regression test fail with the
predicted error before restoring.

1. **`UpdateCertificateAuthority` never enforced its own documented
   precondition.** `api_op_UpdateCertificateAuthority.go`: "Your private CA
   must be in the ACTIVE or DISABLED state before you can update it. You can
   disable a private CA that is in the ACTIVE state or make a CA that is in
   the DISABLED state active again." The backend checked only that the
   *target* status was ACTIVE/DISABLED, never the CA's *current* status --
   so a SUBORDINATE CA still in PENDING_CERTIFICATE (no certificate ever
   imported) could be flipped straight to ACTIVE via
   `UpdateCertificateAuthority(status=ACTIVE)`, and a DELETED-but-restorable
   CA could likewise be resurrected to ACTIVE directly, bypassing
   `RestoreCertificateAuthority` and leaving `RestorableUntil` stale. Fixed
   in `certificate_authorities.go`'s `UpdateCertificateAuthority`: now
   rejects with `InvalidStateException` (modeled by this op's own
   deserializer) unless the CA's current status is ACTIVE or DISABLED --
   applies to every update (status change or RevocationConfiguration-only
   change), matching the doc comment's unqualified "before you can update
   it". Regression tests:
   `TestInMemoryBackend_CertificateAuthorityValidation/updateCA_on_a_PENDING_CERTIFICATE_CA_returns_InvalidStateException`
   and `.../updateCA_on_a_DELETED-but-restorable_CA_returns_InvalidStateException`
   (`certificate_authorities_test.go`), both confirmed failing pre-fix.

2. **`RevokeCertificate` allowed silent double-revocation.** Its own
   `awsAwsjson11_deserializeOpErrorRevokeCertificate` uniquely models
   `RequestAlreadyProcessedException` ("Your request has already been
   completed") among all 23 ops in this service -- grepped every other op's
   deserializer, none declare it -- direct evidence a repeat
   `RevokeCertificate` call on an already-revoked certificate must be
   rejected, per this campaign's "a modelled error is evidence of a
   precondition" rule. The backend previously re-applied the revocation
   unconditionally, silently overwriting `RevokedAt`/`RevocationReason` on a
   second call. Fixed in `certificates.go`'s `RevokeCertificate`: returns
   the new `ErrRequestAlreadyProcessed` sentinel
   (`RequestAlreadyProcessedException`, wired into `handleOpError`) when the
   certificate's status is already `REVOKED`, before mutating state.
   Regression test:
   `TestInMemoryBackend_CertificateValidation/revoking_an_already-revoked_certificate_returns_RequestAlreadyProcessedException`
   (`certificates_test.go`), confirmed failing pre-fix and confirming
   `RevocationReason` is not overwritten by the rejected second call.

**Investigated, no fix (déjà-vu of an already-disclosed gap)**:
`DeleteCertificateAuthority`'s doc comment ("A private CA can be deleted if
it is in the PENDING_CERTIFICATE, CREATING, EXPIRED, DISABLED, or FAILED
state... A private CA deleted in the CREATING or FAILED state has no
assigned restoration period and cannot be restored") implies CREATING/FAILED
deletions should skip setting `RestorableUntil`. `DeleteCertificateAuthority`
does allow deleting a CREATING-status CA and always sets `RestorableUntil`
regardless -- but this is dead code, not a live bug: `CreateCertificateAuthority`
holds `b.mu.Lock()` for its entire duration and transitions every CA out of
CREATING (to ACTIVE for ROOT, PENDING_CERTIFICATE for SUBORDINATE) before
releasing it, so no caller can ever observe or act on a CA in CREATING
status. `FAILED`/`EXPIRED` remain entirely unmodeled (already disclosed
under `gaps`, unchanged this pass -- no CA ever reaches either). Not fixed:
fixing dead code adds no observable behavior change and risks the "reverting
a file that defines a sentinel another file uses" trap for no benefit; left
as a documented trap instead (see below).

**Also checked, confirmed correct, no fix needed**: `IssueCertificate`'s
`ca.Status != caStatusActive` gate (matches "must be ACTIVE"); ghost-row
risk on CA deletion for permissions/policies/tags (`permissions.go`,
`ca_policy.go`) -- all correctly remain queryable through a soft-deleted
(DELETED-but-restorable) CA via `caGet`, matching real
`DescribeCertificateAuthority`'s documented behavior of still reporting on a
DELETED CA within its restoration window, and correctly become unreachable
once the CA passes `RestorableUntil` (same `caGet` choke point everything
else goes through) -- not a new ghost-row leak, `CreatePermission`/`PutPolicy`
on a DELETED-but-restorable CA is a lower-confidence question left **NOT
CHECKED** (no explicit doc-comment precondition found for those two ops
specifically, unlike `UpdateCertificateAuthority`'s unambiguous text).
`ListCertificateAuthorities`/`ListCertificates`/`ListPermissions` sort keys
(`ARN`, `ARN`, `Principal+SourceAccount`) re-confirmed unique
(ARNs embed a fresh UUID/serial per resource; a CA's permission key is one
per (CA, principal, sourceAccount) tuple by construction) -- no unstable-sort
bug.

Gates: `GOTOOLCHAIN=go1.26.6 go build ./services/acmpca/...` clean;
`go test -race -count=1 ./services/acmpca/...` pass (all pre-existing
assertions unchanged, 2 new regression tests added); `golangci-lint run
./services/acmpca/...` → `0 issues.`; dependents
`go test -race -count=1 ./services/cloudformation/... ./services/acm/...`
pass.


Protocol: awsjson1.1 (single POST, `X-Amz-Target: ACMPrivateCA.<Op>`; RouteMatcher prefix
`"ACMPrivateCA."` confirmed against the SDK's `ServiceID`/operation names — correct).

### 2026-08-30 fabricated-error-code sweep (gopherstack-r3pr)

**Real bug, confirmed and fixed**: every "invalid parameter" path in this service emitted
the wire code `InvalidParameterException` via a single sentinel (`ErrInvalidParameter`)
and one central `handleOpError` mapping. `InvalidParameterException` names no type in the
pinned SDK (`acmpca@v1.50.0`) -- grepping every `awsAwsjson11_deserializeOpError*` switch
in `deserializers.go` confirms it appears in none of the 23 operations' modeled error sets.
A typed client's `errors.As` against any acm-pca exception type therefore always missed,
falling through to `*smithy.GenericAPIError` -- confirmed both by reading the deserializers
and by 4 new SDK-driven tests (`error_code_fixes_test.go`) that fail against the unmodified
code with exactly that fallthrough.

Fix: replaced the one flat sentinel with per-operation-correct sentinels, chosen by reading
each emitting operation's own `deserializeOpError` (not a sibling's): `ErrInvalidArgs`
("InvalidArgsException", CreateCertificateAuthority/UpdateCertificateAuthority business-rule
validation and CreateCertificateAuthorityAuditReport/DescribeCertificateAuthorityAuditReport
non-ARN fields), `ErrInvalidArn` ("InvalidArnException", every `*Arn`-field required-check --
modeled by every op except CreateCertificateAuthority/ListCertificateAuthorities),
`ErrInvalidRequest` ("InvalidRequestException", RevokeCertificate's RevocationReason),
`ErrInvalidPolicy` ("InvalidPolicyException", PutPolicy's empty-Policy check),
`ErrMalformedCertificate` ("MalformedCertificateException", ImportCertificateAuthorityCertificate's
PEM decode/parse failures), `ErrMalformedCSR` ("MalformedCSRException", IssueCertificate's Csr
field). A few call sites (CreatePermission's Principal/Actions checks, DeletePermission's
Principal check) have no matching code in their own operation's modeled set at all -- best
effort `ErrInvalidArgs` used there since it is at minimum a real acm-pca exception type,
flagged here as unconfirmed against that specific operation's deserializer.

Also corrected: `DeleteCertificateAuthority`'s out-of-range `PermanentDeletionTimeInDays`
and `ListCertificateAuthorities`'s invalid `ResourceOwner` both map to `ErrInvalidArgs` as a
best-effort choice (neither operation's own deserializer models `InvalidArgsException` or
any other "bad argument" code -- `DeleteCertificateAuthority` models only
`ConcurrentModificationException`/`InvalidArnException`/`InvalidStateException`/
`ResourceNotFoundException`, `ListCertificateAuthorities` only `InvalidNextTokenException`).
25 existing test assertions across 15 files, asserting the fabricated `InvalidParameterException`
(some via `acmpca.ErrInvalidParameter`, some via the literal wire string), were updated to the
corrected code. 4 new SDK-driven tests were added (`error_code_fixes_test.go`), each confirmed
to fail against the unmodified code before this fix.

### 2026-08-29 constraint-not-honoured sweep (gopherstack-wksw)

New bug class for this campaign: a parameter constraining a result (filter/page-limit)
present in the real Input but not correctly honoured -- distinct from the wire-shape bugs
the 2026-08-20 sweep covered. All 3 collection-returning ops (`ListCertificateAuthorities`,
`ListPermissions`, `ListTags`) re-read against their own `api_op_List*.go` in
`acmpca@v1.50.0`. **1 real bug found and fixed** -- `ListCertificateAuthorities`'s
100-item hard page cap (see its ops entry above for detail). `ListPermissions` and
`ListTags` were re-confirmed clean: both share the same `pkgs/page.New(items, nextToken,
maxItems, defaultMaxItems)` call shape, but neither op's own doc comment documents a
lower-than-requested actual-return ceiling the way `ListCertificateAuthorities`'s does (each
just says "specify the maximum number of items to return" with no "only returns a maximum
of N" caveat), so `page.New`'s plain `limit <= 0 -> defaultLimit` fallback with no upper
clamp is correct behavior for those two, not a second instance of the same bug. This also
confirms the "family is never the unit of truth" rule directly: three ops share one
pagination helper and one `defaultMaxItems` constant, but only one of the three has a
documented ceiling below what a caller can request.

`ListCertificateAuthorities.ResourceOwner` (already fixed by a prior pass, per its own ops
entry) re-verified still correct -- SELF/empty scope to the account, OTHER_ACCOUNTS empty,
anything else rejected.

Test style: real backend method call (`b.ListCertificateAuthorities`), not a hand-built
request, since `MaxResults` already decodes correctly as a plain Go int at the handler --
the bug is entirely in the backend's page-size resolution, the narrow exception this
campaign's brief allows for skipping a full SDK-client round trip. Seeded via
`CreateCertificateAuthority` (105 real CAs, EC key gen, ~30ms total) rather than fabricating
`CertificateAuthority` structs directly, since acmpca has no existing whitebox test file for
that pattern and the real creation path is fast enough here.

### 2026-08-20 re-audit: wrapper-key / nested-shape sweep (zero new wire bugs)

Scope: this pass targeted the wrapper-key/nesting-level/JSON-type/enum-value bug class
specifically (not a full re-audit of state/errors/persist, which the 2026-07-23 pass
already covered in depth). All 23 ops in `GetSupportedOperations()` were enumerated and
cross-checked 1:1 against `ls $(go env GOMODCACHE)/.../acmpca@v1.50.0/api_op_*.go` (23
files, exact match, no drift since the last pass).

**Protocol reconfirmed independently**: `grep -c '^func awsAwsjson11_deserializeOp'
deserializers.go` → 35 (defined and called; JSON-RPC, not the restjson flat-body false-
positive trap the brief warned about — that trap does not apply here, confirmed rather
than assumed).

**Every op's Input/Output struct** (`api_op_*.go`) was read directly and diffed field-by-
field against gopherstack's wire structs in `handler_certificate_authorities.go`,
`handler_certificates.go`, `handler_audit_reports.go`, `handler_ca_policy.go`,
`handler_permissions.go`, `handler_tags.go`. Every wrapper key, nesting level, and JSON
type matched exactly. Every emitted enum value (`CertificateAuthorityStatus`,
`CertificateAuthorityType`, `KeyStorageSecurityStandard`, `CertificateAuthorityUsageMode`,
`CrlType`, `S3ObjectAcl`, `ResourceOwner`, `RevocationReason`, `AuditReportStatus`,
`AuditReportResponseFormat`, `ActionType`) was grepped in `models.go`/`permissions.go` and
compared byte-for-byte against `types/enums.go` — all exact matches, no invented values,
no case mismatches.

**GeneralName** (the 8-member mutually-exclusive union: `DnsName`, `IpAddress`,
`Rfc822Name`, `DirectoryName`, `EdiPartyName`, `OtherName`, `UniformResourceIdentifier`,
`RegisteredId` — confirmed 8, not 9, against `types.go:594-628`) never appears on any
response shape in the real SDK; it exists only inside
`IssueCertificateInput.ApiPassthrough.Extensions.SubjectAlternativeNames`, which
gopherstack never echoes back anywhere. So the "request-only field leaking into a
response" bug class does not apply to `GeneralName` here — verified by grep, not assumed.
On the request side (`generalNameWire`/`decodeGeneralName`, `handler_certificates.go`),
all 8 variants are represented in the wire struct; the 3 Terraform actually uses
(`DnsName`/`IpAddress`/`Rfc822Name`) are implemented, the other 5 are explicitly rejected
with `InvalidArgsException` (corrected gopherstack-r3pr, was the fabricated `InvalidParameterException`)
rather than silently dropped — correct treatment, no change needed.

**Request-only-field-in-response check**: `ApiPassthrough` (the other main
request/response-shared-shape risk named in the brief) is `IssueCertificateInput`-only —
`IssueCertificateOutput` has just `CertificateArn`, confirmed by reading the real
`api_op_IssueCertificate.go` struct directly — and gopherstack's `issueCertificateOutput`
correctly has no `ApiPassthrough`-derived fields. Clean.

**Two new (small, pre-existing) gaps found and disclosed, not fixed** — see `gaps` above:
`FailureReason`/`CertificateAuthorityStatus`'s `FAILED`/`EXPIRED` values (state-machine
depth, not a wire bug — the field is correctly never emitted since no CA ever reaches
those states), and `CertificateAuthorityConfiguration.CsrExtensions` (silently dropped on
input rather than explicitly rejected like the sibling exotic-field gaps get). Both are
Layer 3 in nature (missing feature depth), surfaced incidentally while diffing
`CertificateAuthority`'s and `CertificateAuthorityConfiguration`'s full field lists against
`types/types.go:160-266`, not from a dedicated Layer-3 hunt.

**Existing tests**: every wire-key assertion in `handler_certificate_authorities_test.go`,
`handler_certificates_test.go`, `handler_tags_test.go`, `handler_permissions_test.go`,
`handler_audit_reports_test.go`, `handler_ca_policy_test.go`, `handler_sdk_route_table_test.go`,
and `api_passthrough_test.go` was spot-checked against the real SDK structs; none assert a
wrong key/nesting/type/value. No test correction was needed this pass (contrast with
redshiftdata's three wrong-key tests the same session).

**Added**: `wire_sdk_roundtrip_test.go` —
`TestDescribeCertificateAuthority_SDKRoundTrip`, this service's first *typed* real-SDK-
client round-trip test (prior coverage was raw-JSON-map assertions via
`handler_*_test.go`'s `doACMPCARequest`, which can't detect a case-sensitive key miss the
way a typed client's deserializer can). It creates a CA with a full
`CertificateAuthorityConfiguration.Subject` and `RevocationConfiguration`
(`CrlConfiguration`+`OcspConfiguration`), describes it back through the real
`acmpcasdk.Client`, and asserts every nested field survived. **Proven meaningful by hand-
revert**: changed `certAuthorityOutput.RevocationConfiguration`'s JSON tag from
`"RevocationConfiguration"` to `"revocationConfiguration"` (lowercase r) — the test failed
exactly as predicted (`ca.RevocationConfiguration` came back `nil` through the real
client, since `awsAwsjson11_deserializeOpDocumentDescribeCertificateAuthorityOutput`'s case
switch is case-sensitive and only matches the capitalized key); reverted, confirmed
`git diff` byte-identical and the test green again.

**last_audit_commit provenance re-checked**: the prior manifest's `last_audit_commit:
1c4ee34e` is dated `Sun Jul 19 14:07:07 2026` (`git show -s --format=%ad`), 4 days before
`last_audit_date: 2026-07-23` — not the days-to-weeks-stale pattern the re-audit protocol
warns about; `git diff 1c4ee34e..<pre-this-pass-HEAD> --stat -- services/acmpca/` showed
~2,658 insertions across 28 files, matching the prior manifest's own extensive "FIXED THIS
PASS" narrative line-for-line. Provenance is genuine, not a copy-paste sha. No prose/header
SDK-version mismatch found (`sdk_module` header says `v1.50.0`; `go.mod` pins the identical
`v1.50.0`; no other version number appears anywhere in this file's prose). Every prior
"FIXED THIS PASS" claim in `ops:` was spot-verified still true by reading the current code
(`RevocationConfiguration` round-trips, `KeyStorageSecurityStandard`/`UsageMode` echo
correctly, `LastStateChangeAt` present, issued-cert ARN uses the decimal serial, etc.) — no
regressions, no stale claims.

### Bugs fixed this pass (real, high-impact)

1. **Issued certificate ARNs embedded the wrong ID.** aws-sdk-go-v2's
   `IssueCertificateOutput` doc comment gives a concrete example ARN ending in
   `.../certificate/286535153982981100925020015808220737245` — that trailing
   number is the certificate's own serial number in decimal. gopherstack instead
   generated an unrelated 16-byte `crypto/rand` ID for that path segment, so
   every issued certificate's ARN was wrong-shaped relative to real AWS (a
   client parsing the ARN to recover the serial, or comparing it against a
   value obtained elsewhere, would get a mismatch). Fixed in
   `certificates.go` (`signAndStoreCertificateLocked`): the ARN is now built
   from `big.Int` decimal formatting of the same hex serial already stored on
   `IssuedCertificate.Serial`.

2. **`ListTagsForCertificateAuthority` was an invented operation** — it does
   not exist anywhere in `aws-sdk-go-v2/service/acmpca` (no
   `api_op_ListTagsForCertificateAuthority.go`; only `ListTags` is real).
   Deleted from `GetSupportedOperations()` and the dispatch switch in
   `handler.go`; a request naming it now correctly gets `InvalidAction` like
   any other unrecognized op, instead of silently succeeding as an alias for
   `ListTags`.

3. **`CertificateAuthority` was missing three real SDK fields entirely**
   (`KeyStorageSecurityStandard`, `UsageMode`, `LastStateChangeAt`) — none of
   the create/describe/list wire shapes carried them at all, which is a gap
   beyond what the prior manifest tracked (that pass's field-diff didn't reach
   these). Added: `KeyStorageSecurityStandard` (validated against the 3-value
   enum, default `FIPS_140_2_LEVEL_3_OR_HIGHER`), `UsageMode` (validated
   against the 2-value enum, default `GENERAL_PURPOSE`, and now enforces the
   real API's 7-day certificate-validity cap for
   `SHORT_LIVED_CERTIFICATE`-mode CAs on `IssueCertificate`), and
   `LastStateChangeAt` (set on every status/certificate-material transition:
   Create, self-sign-activate, Import, Update, Delete, Restore).

4. **`RevocationConfiguration` (CRL/OCSP) was entirely unmodeled** — the
   biggest of the 8 pre-existing gaps. `CreateCertificateAuthority` and
   `UpdateCertificateAuthority` now accept a real `RevocationConfiguration`
   input (validated per the documented constraints: a disabled
   `CrlConfiguration`/`OcspConfiguration` must set only `Enabled=false`; an
   *enabled* `CrlConfiguration` must specify `S3BucketName`; `CrlType` and
   `S3ObjectAcl` are validated against their enums), and
   `DescribeCertificateAuthority`/`ListCertificateAuthorities` now report it
   back — omitting the field entirely when unconfigured, matching how the real
   SDK omits a nil `*types.RevocationConfiguration` rather than emitting an
   empty object. `UpdateCertificateAuthority`'s "omit the field to leave
   existing config unchanged" semantics required a custom `UnmarshalJSON` on
   the wire input type to distinguish "key absent" from "key present with a
   null/zero value", since both unmarshal a Go pointer field to `nil`.

5. **`ApiPassthrough`/`TemplateArn` were both silently ignored** (the two
   prior deferred items). Both are now substantially implemented:
   `TemplateArn` gates `ApiPassthrough` exactly as the real API's doc comment
   describes ("An APIPassthrough or APICSRPassthrough template variant must be
   selected, or else this parameter is ignored"); when gated-in,
   `ApiPassthrough.Subject` (the 6 common X.500 fields + `SerialNumber`) and
   `ApiPassthrough.Extensions` (`KeyUsage`'s 9 bits, `ExtendedKeyUsage` —
   standard types via `crypto/x509` constants where they exist,
   Microsoft/CT-EKU OIDs via `UnknownExtKeyUsage` where they don't, plus
   arbitrary custom OIDs; `SubjectAlternativeNames` for DNS/IP/email;
   `CustomExtensions` via raw `pkix.Extension` passthrough) now really alter
   the signed certificate (see `crypto.go`'s `applyAPIPassthrough`). The
   sub-fields not implemented (`CertificatePolicies`, exotic `ASN1Subject` RDN
   types, exotic `GeneralName` variants) are explicitly **rejected** with
   `InvalidArgsException` (corrected gopherstack-r3pr, was the fabricated
   `InvalidParameterException`) when a caller sets them, rather than silently
   dropped — see `handler_certificates.go`'s `decodeASN1Subject`/
   `decodeExtensions`/`decodeGeneralName`, and parity-principles.md's
   no-silent-gaps rule.

6. **`RestorableUntil` was tracked but never enforced.** A DELETED CA now
   becomes invisible (`ResourceNotFoundException`) to every read path once its
   restoration window passes, and `RestoreCertificateAuthority` correctly
   rejects a restore attempted after the deadline — both via a single choke
   point, `caGet`/`casInRegion` in `store.go`, rather than scattered checks
   across every CA-touching function. This is a lazy read-time filter, not a
   background sweep (see leaks note above): no new goroutine was introduced.

7. **`CreateCertificateAuthority`/`IssueCertificate` `IdempotencyToken` was
   accepted but never deduplicated.** Both now recognize repeated calls
   bearing the same token within a 5-minute window (matching the real API's
   documented behavior) and return the original resource's ARN instead of
   creating a duplicate, via a small `(region, op, token) -> (resourceARN,
   expiresAt)` cache on the backend (`store.go`'s `idempotentResourceARN`/
   `rememberIdempotency`) — deliberately not persisted through Snapshot/Restore,
   since it's a short-lived dedup cache, not durable resource state.

8. **`ListCertificateAuthorities`'s `ResourceOwner` was accepted but
   ignored.** Now validated against the real 2-value enum: `SELF`/empty lists
   this account's CAs (unchanged behavior), `OTHER_ACCOUNTS` returns an empty
   page (no cross-account CA sharing is modeled, so no CA is ever owned by
   another account), and any other value is `InvalidArgsException` (corrected gopherstack-r3pr,
   best-effort -- ListCertificateAuthorities' own deserializer does not model
   InvalidArgsException; was the fabricated `InvalidParameterException`).

9. **`TagCertificateAuthority` never enforced the 50-tag-per-CA limit.** Now
   returns `TooManyTagsException` when tagging would exceed it (checked
   without mutating state first, via `tagCountAfterMerge` in
   `handler_tags.go`).

10. **`ListTags` never paginated.** `MaxResults`/`NextToken` now behave like
    every other list op in this service (`pkgs/page`), instead of always
    returning the full tag set in one page.

11. **`CreatePermission` accepted any `Principal` string.** Now validated to
    be exactly `"acm.amazonaws.com"`, the only value
    `CreatePermissionInput.Principal`'s doc comment says the real API accepts.

12. **CA/audit-report resource IDs used a flat 32-char hex string with no
    dashes.** `newRandomID` now formats the same entropy as a dashed UUID
    (8-4-4-4-12), matching the shape of real ACM PCA resource IDs (see
    `CreateCertificateAuthorityOutput`'s doc comment example).

### Traps for the next auditor (looks-wrong-but-intentional)

- `IssueCertificate`'s `Validity.Type == "END_DATE"` is handled identically to
  `"ABSOLUTE"` (both treated as Unix epoch seconds via `time.Unix`). Per AWS docs,
  END_DATE is technically UTCTime/GeneralizedTime (`YYMMDDHHMMSS`/`YYYYMMDDHHMMSS`
  as a decimal integer), a different encoding from ABSOLUTE's Unix epoch. This is a
  deliberate prior-sweep simplification (see `TestACMPCA_IssueCertificate_ValidityTypeAliases`),
  left as-is again this pass — implementing true UTCTime parsing for a rarely-used
  validity type wasn't judged worth the risk of breaking existing intentional-design
  tests without a dedicated pass. Flagged here (and in gaps) instead of silently
  left off the manifest.
- `RevokeCertificate` does **not** check whether CRL/OCSP is enabled on the CA
  before revoking — this was flagged as a gap in the prior manifest under the
  theory that real AWS requires it, but re-reading the real SDK's doc comment
  this pass found no such precondition: CRL/OCSP are described purely as
  optional side-effects of revocation (the CRL gets updated, OCSP responses
  change), never as a requirement for the `RevokeCertificate` call to succeed.
  That prior gap note was a misdiagnosis; no fix was needed.
- `DeleteCertificateAuthority`'s `caStatusCreating` branch (allowed states:
  DISABLED, CREATING, PENDING_CERTIFICATE) is unreachable dead code, not a
  bug: `CreateCertificateAuthority` holds `b.mu.Lock()` for its whole
  duration and always transitions a new CA out of CREATING before releasing
  the lock, so no caller can ever call `DeleteCertificateAuthority` while a
  CA is actually in that status. Consequently the doc comment's "CREATING
  deletions get no restoration period" nuance (`api_op_DeleteCertificateAuthority.go`)
  can never manifest either -- confirmed 2026-09-04, left as-is (gopherstack-5xc).
- `RestorableUntil`-past-deadline CAs are hidden by `caGet`/`casInRegion`
  filtering, not physically deleted from `b.cas`/`b.casByRegion` — see the
  `leaks` note above for why this is an accepted tradeoff, not a regression.
- Outbound blob fields the SDK models as `*string` (e.g.
  `GetCertificate`/`GetCertificateAuthorityCertificate`/`GetCertificateAuthorityCsr`'s
  `Certificate`/`CertificateChain`/`Csr` **output** fields) are intentionally
  plain PEM strings, not base64 — this asymmetry (blob on input, string on
  output) is a real AWS API quirk, confirmed against `deserializers.go`, not a
  gopherstack bug. (Carried over from the prior pass's notes; still accurate.)

### Follow-ups for bd (not fixed — out of scope / lower value this pass)

- `ApiPassthrough.Extensions.CertificatePolicies` (OID/PolicyQualifier ASN.1 encoding).
- `ApiPassthrough.Subject`'s exotic RDN types and `SubjectAlternativeNames`'s exotic
  `GeneralName` variants (both explicitly rejected rather than silently dropped).
- `TemplateArn`'s per-template default X.509 extension profiles (path-length
  constraints, OCSP/code-signing presets) beyond the APIPassthrough-gating behavior.
- `RevocationConfiguration`'s CNAME/S3-bucket-name format validation (RFC2396,
  S3 bucket naming rules) — currently any non-empty string is accepted.
- True UTCTime/GeneralizedTime parsing for `Validity.Type == "END_DATE"`.
- NEW (2026-08-20 pass): `CertificateAuthorityStatus`'s `FAILED`/`EXPIRED` values and
  `CertificateAuthority.FailureReason` are entirely unmodeled (no CA ever fails creation
  or expires).
- NEW (2026-08-20 pass): `CertificateAuthorityConfiguration.CsrExtensions` is silently
  dropped on `CreateCertificateAuthority` input rather than stored/echoed or explicitly
  rejected like its `ASN1Subject`/`Extensions` siblings.

## 2026-08-31 Error-envelope sweep (gopherstack-6flj/uox6, errtargetaudit)

`errtargetaudit -dir acmpca` reported 6 class-A findings, all resolving to
3 distinct call sites (`CreatePermission`'s 4 validation checks share one
finding per domain; `DeleteCertificateAuthority`; `ListCertificateAuthorities`).
Verified each against the pinned SDK's own per-op `deserializeOpError`
switch (acmpca@v1.50.0 deserializers.go) — all 3 are real: a
correctly-declared-elsewhere code (`InvalidArgsException`) reaching an
operation whose own switch does not include it.

**No fix applied to any of the three** — recorded, not substituted, per the
no-invented-code rule:

- `CreatePermission` (Principal-required / Principal-must-be-acm.amazonaws.com
  / Actions-required / unsupported-action checks, `permissions.go`): declares
  `InvalidArn`, `InvalidState`, `LimitExceeded`, `PermissionAlreadyExists`,
  `RequestFailed`, `ResourceNotFound` — no validation-shaped exception at all.
- `DeleteCertificateAuthority` (`PermanentDeletionTimeInDays` 7–30 range
  check, `certificate_authorities.go`): declares `ConcurrentModification`,
  `InvalidArn`, `InvalidState`, `ResourceNotFound`. `InvalidArnException`'s
  own doc ("does not refer to an existing resource") does not describe a
  day-count range violation, so it was not substituted despite being the
  closest-sounding declared type.
- `ListCertificateAuthorities` (bad `ResourceOwner` enum value,
  `certificate_authorities.go`): declares only `InvalidNextTokenException`.

All three: no `ValidationException` type exists anywhere in this SDK
module (grepped), so there is no generic fallback either — reason is "the
operation's own model declares no type for this condition", not a
reachability or infrastructure gap.

Gates: `go build ./services/acmpca/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/acmpca/...` (pass, unchanged assertion
counts), `golangci-lint run ./services/acmpca/...` (0 issues). No code
changed in this pass — comments only.

### 2026-08-31 Error-envelope re-run (gopherstack-uox6, post-reachability-fix)

`errtargetaudit -dir acmpca` re-run after the tool's unreachable-branch
false-positive fix (773bfa8b7): still 6 class-A findings, identical to the
2026-08-31 sweep above (`CreatePermission` x4 sites, `DeleteCertificateAuthority`,
`ListCertificateAuthorities`) -- confirming this service's findings were
never the reachability-defect shape. Not re-derived from scratch: this is
the same previously-recorded refusal (all 3 distinct sites, none fixed, per
the earlier entry's own per-op `deserializeOpError` verification and its
"no `ValidationException` type exists anywhere in this SDK module" finding).
No code changed.

### 2026-09-07 Error-envelope re-run, third confirmation (gopherstack-qrnq, errtargetaudit)

`errtargetaudit` filed 6 fresh class-A findings for acmpca; same 3 call
sites as the two entries above (`CreatePermission` x4 sites,
`DeleteCertificateAuthority`, `ListCertificateAuthorities`), same
`InvalidArgsException` code, same `sentinel reference` mechanism. Re-verified
independently rather than trusting the prior write-up:

- Re-extracted each op's declared set straight from
  `aws-sdk-go-v2/service/acmpca@v1.50.0/deserializers.go`
  (`awk '/deserializeOpError<Op>\(/,/^}/'` + code-string grep):
  `CreatePermission` -> `InvalidArnException, InvalidStateException,
  LimitExceededException, PermissionAlreadyExistsException,
  RequestFailedException, ResourceNotFoundException`;
  `DeleteCertificateAuthority` -> `ConcurrentModificationException,
  InvalidArnException, InvalidStateException, ResourceNotFoundException`;
  `ListCertificateAuthorities` -> `InvalidNextTokenException`. None include
  `InvalidArgsException` (`errors.go`'s `ErrInvalidArgs` sentinel) --
  confirms all 3 findings are real declared-set mismatches, not tool noise.
- Confirmed the two false-positive classes this repeat sweep would most
  likely hit both don't apply: no shared helper with `services/acm/`
  (`acm/` and `acmpca/` import neither package, confirmed by grep -- acmpca's
  parity issue gopherstack-ftkd covered a structurally unrelated SDK
  module) and no handler-level override -- `jsonCreatePermission`,
  `jsonDeleteCA`, `jsonListCAs` (`handler_permissions.go`,
  `handler_certificate_authorities.go`) pass backend errors straight to
  `handleOpError`'s `errors.Is(opErr, ErrInvalidArgs) -> "InvalidArgsException"`
  switch (`handler.go`) with no call-site remap.
- Protocol: awsjson1.1. `handleOpError` writes `{"__type": code, "message":
  ...}` via `writeJSONError` at HTTP 400 (500 only for the unmapped-sentinel
  `InternalFailure` default case) -- confirmed by reading `handler.go`
  directly, not assumed.
- Pre-existing tests (`certificate_authorities_test.go`,
  `list_certificate_authorities_resource_owner_test.go`,
  `permissions_test.go`) already assert `require.ErrorIs(t, err,
  acmpca.ErrInvalidArgs)` for these three call sites -- correct, since
  `ErrInvalidArgs`/`InvalidArgsException` is what the handler actually
  emits and no better code exists; none needed correcting.

**No fix applied** -- unchanged from the two prior entries. This is the
third independent audit pass (`gopherstack-6flj`/`uox6`, then a
reachability re-run, now `gopherstack-qrnq`) to land on the same
conclusion: the operation's own declared error set has no member that
fits the validation failure, so no code substitution is correct. Left as a
landmine (see `permissions.go:26-33`, `certificate_authorities.go:384-389`,
`certificate_authorities.go:427-433`) rather than guessed at a fourth time.

Gates: `go test -race -count=1 ./services/acmpca/...` pass (unchanged
assertion counts, no new tests needed since no behavior changed);
`golangci-lint run services/acmpca/...` 0 issues. `errtargetaudit` re-run
post-pass: still 6 class-A findings, identical sites -- expected, since no
code changed.
