---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: support
sdk_module: aws-sdk-go-v2/service/support@v1.34.4   # version audited against
last_audit_commit: 5400868b3                         # HEAD when this manifest was written
last_audit_date: 2026-07-24
overall: A                # 1 severe wire bug (missing __type on every error) + several missing modeled exceptions fixed
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateCase: {wire: ok, errors: fixed, state: ok, persist: ok, note: "CaseCreationLimitExceeded was entirely unimplemented; added open-case cap"}
  DescribeCases: {wire: ok, errors: ok, state: ok, persist: ok}
  ResolveCase: {wire: ok, errors: fixed, state: ok, persist: ok, note: "CaseIdNotFound wire status fixed 404->400 (see errors family note)"}
  AddCommunicationToCase: {wire: ok, errors: fixed, state: ok, persist: ok, note: "CaseIdNotFound/AttachmentSet* wire status fixed 404->400"}
  DescribeCommunications: {wire: ok, errors: fixed, state: ok, persist: ok, note: "CaseIdNotFound wire status fixed 404->400"}
  AddAttachmentsToSet: {wire: ok, errors: fixed, state: ok, persist: ok, note: "size/count overage previously mapped to generic ValidationError instead of the modeled AttachmentSetSizeLimitExceeded; AttachmentLimitExceeded was entirely unimplemented -- both fixed"}
  DescribeAttachment: {wire: ok, errors: fixed, state: ok, persist: ok, note: "AttachmentIdNotFound wire status fixed 404->400; DescribeAttachmentLimitExceeded was entirely unimplemented -- added"}
  DescribeCreateCaseOptions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "static data, no persistable state"}
  DescribeServices: {wire: ok, errors: ok, state: ok, persist: n/a, note: "static data"}
  DescribeSeverityLevels: {wire: ok, errors: ok, state: ok, persist: n/a, note: "static data"}
  DescribeSupportedLanguages: {wire: ok, errors: ok, state: ok, persist: n/a, note: "categoryCode field fix from prior pass verified still correct"}
  DescribeTrustedAdvisorChecks: {wire: ok, errors: fixed, state: ok, persist: n/a, note: "language validation reused the 4-code Support-case set (en/ja/zh/ko) instead of the 11-code Trusted-Advisor-specific set (adds zh_TW/fr/de/id/it/pt_BR/es); valid TA requests such as language=fr were incorrectly rejected -- fixed with validTALanguage"}
  DescribeTrustedAdvisorCheckRefreshStatuses: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTrustedAdvisorCheckResult: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTrustedAdvisorCheckSummaries: {wire: ok, errors: ok, state: ok, persist: ok}
  RefreshTrustedAdvisorCheck: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  case_lifecycle: {status: ok, note: "field shapes still match deserializers.go; CaseCreationLimitExceeded now enforced (open-case cap, frees on resolve)"}
  attachments: {status: ok, note: "AttachmentSetSizeLimitExceeded/AttachmentLimitExceeded/DescribeAttachmentLimitExceeded now real (sliding-window rate limiters + size/count routing), not stubs"}
  trusted_advisor: {status: ok, note: "language validation now uses the real 11-code Trusted-Advisor set instead of the 4-code case-language set"}
  filter_value_semantics: {status: ok, note: "2026-08-31 (gopherstack-uox6 value-semantics pass, CLEAN -- no bug found): audited every filterable List/Describe op's request-parameter semantics (this service's covledger row was empty; PARITY.md itself had never recorded this axis). DescribeCasesWithOptions/DescribeCommunicationsWithOptions: caseIdList/displayId/language are correct equality filters, afterTime/beforeTime compare against CaseDetails.TimeCreated/Communication.TimeCreated (the only date field either type has -- the doc's 'filtered date search on support case communications' wording on DescribeCases judged a generation artifact, same call as the substring/prefix doc comment dynamodb's pass correctly disbelieved). includeCommunications *bool correctly preserves the omitted-vs-false distinction (in.IncludeCommunications == nil || *in.IncludeCommunications, handler_cases.go:95) matching the documented 'By default, communications are included' -- new regression test TestSupport_DescribeCases_IncludeCommunications added and proven to fail against a temporarily-flattened version, then restored byte-identical. DescribeTrustedAdvisorChecks/CheckResult/CheckSummaries/RefreshCheck: checkIds are direct map lookups, no matcher surface. DescribeServices.serviceCodeList is a simple set-membership filter, verified correct. MaxResults: neither the pinned SDK nor the live AWS_DescribeCases API reference page (fetched, carried the agent-toolkit footer) states a default when omitted, only Valid Range 10-100 -- nothing for the existing defaultPageSize=100 to violate. One item recorded on the OTHER axis, not fixed: DescribeTrustedAdvisorCheckRefreshStatuses does not validate checkIds is non-empty/well-formed the way DescribeTrustedAdvisorCheckSummaries does -- a missing rejection, validation-shaped rather than a wrong algorithm. Another recorded as a gap: DescribeCasesWithOptions silently drops an unknown id from caseIdList rather than raising the documented CaseIdNotFound -- also a missing-rejection/validation gap, not filter semantics, left unfixed per the class's own discrimination rule."}
  errors: {status: fixed, note: "SEVERE: handleError built a bare {\"message\":...} JSON body with NO \"__type\" field and no X-Amzn-ErrorType header. aws-sdk-go-v2/service/support/deserializers.go's resolveProtocolErrorType requires one of those two to identify which exception occurred; without it every error -- regardless of the correct HTTP status/message text -- deserializes client-side as a generic smithy.GenericAPIError{Code:\"UnknownError\"}, never the typed exception (e.g. *types.CaseIdNotFound) a real caller's errors.As would expect. Fixed: handleError now emits service.JSONErrorResponse{Type, Message} (the shared convention also used by codeconnections/athena in this campaign) via a new resolveErrorType(err) switch. Separately, confirmed via the botocore support/2013-04-15/service-2.json model that NONE of support's exception shapes carry an httpStatusCode override, so the awsjson1.1 protocol default applies: HTTP 400 for every client-fault exception (including the '*NotFound'-named ones) and HTTP 500 only for the fault:true InternalServerError shape. gopherstack previously mapped CaseIdNotFound/AttachmentIdNotFound/AttachmentSetIdNotFound to HTTP 404 -- fixed to 400. This __type gap predates and is independent of the HTTP-status gap; both were unit-test-invisible because existing tests only asserted on rec.Code, never decoded the body's __type field (parity-principles.md note 3: unit tests are not full parity proof)."}
gaps:
  - "CaseDetails.Status declares 8 valid values (types.go Status field doc comment: all-open, customer-action-completed, opened, pending-customer-action, reopened, resolved, unassigned, work-in-progress); this backend reaches 3 -- opened (CreateCase cases.go:64, CreateCaseWithOptions cases.go:209), resolved (ResolveCase cases.go:134, ResolveCaseWithStatus cases.go:282), and reopened (AddCommunicationWithOptions reopening a resolved case, communications.go:83). Enumerated every Case.Status write site in services/support/*.go (non-test) and found no others. The other 5 have no client-callable trigger: of the API's 16 operations only CreateCase, AddCommunicationToCase, and ResolveCase can plausibly mutate status, and none of their doc comments (api_op_CreateCase.go, api_op_AddCommunicationToCase.go, api_op_ResolveCase.go) describe a transition into unassigned/work-in-progress/pending-customer-action/customer-action-completed -- those are AWS support-staff-side case-management states (agent assignment, agent work, an agent requesting or receiving further customer input) with no modeled client operation that sets them; DescribeCasesInput (api_op_DescribeCases.go) also has no status filter parameter at all, so all-open is not even reachable as a request value, and reads as a leaked console-filter label in AWS's own doc comment rather than a state CaseDetails.Status is documented to actually hold. Same class as gopherstack-g2eo (directoryservice TrustState/SnapshotStatus): verdict modelling gap, not a defect; no code changed. (gopherstack-hwlt)"
deferred:
  - integration test suite (test/integration/*_parity_test.go) was not run this pass — only unit tests plus static comparison against the real SDK's deserializers.go/serializers.go and the botocore support/2013-04-15/service-2.json model (stronger than typical unit-test-only audits, but still not a live SDK-client round trip). test/integration/support_test.go exists and covers the happy-path case lifecycle only; it does not exercise error paths, so it would not have caught the missing __type field either (the Go SDK client would have surfaced a generic/wrong error type, but the existing test never triggers an error path).
leaks: {status: clean, note: "janitor.go RunJanitor/sweepExpiredResources reviewed; ticker-based, stops cleanly on ctx cancellation via worker.Group; StartWorker/Shutdown wire it into service.BackgroundWorker/Shutdowner correctly. New rate-limit timestamp slices (attachmentSetCreationTimes/describeAttachmentCallTimes) are pruned on every access and capped at their threshold by construction (no unbounded growth), guarded by the existing b.mu -- no new goroutines/tickers introduced."}
---

## Notes (this pass, 2026-07-24)

### 1. SEVERE: every error response was missing the wire "__type" field

`handleError` in `handler.go` built error bodies as a bare
`map[string]string{"message": err.Error()}` with no `__type` field and no
`X-Amzn-ErrorType` header. A real `aws-sdk-go-v2` client's error deserializer
(`awsAwsjson11_deserializeOpError<Op>` -> `resolveProtocolErrorType` in
`deserializers.go`) requires one of those two to identify which exception
occurred; lacking both, `errorCode` stays the literal string `"UnknownError"`
and the switch in every op's error deserializer falls through to `default:`,
returning a `smithy.GenericAPIError{Code:"UnknownError"}` — **never** the
correct typed exception. Every single support error response was, from a real
SDK client's point of view, an unidentifiable generic error, no matter how
correct the HTTP status or message text was. This is exactly the class of bug
`parity-principles.md` warns unit tests cannot catch (they only asserted on
`rec.Code`, never decoded `__type`).

Fixed: `handleError` now marshals `service.JSONErrorResponse{Type, Message}`
(the same shared envelope + convention already used by `codeconnections` and
`athena` in this campaign) via a new `resolveErrorType(err) (string, int)`
that maps each backend sentinel to its real AWS exception name.

### 2. HTTP 404 used for "*NotFound"-named exceptions; real AWS uses 400

Confirmed via the botocore `support/2013-04-15/service-2.json` model: none of
support's exception shapes (`CaseIdNotFound`, `AttachmentIdNotFound`,
`AttachmentSetIdNotFound`, etc.) carry an `error.httpStatusCode` override.
Support is a JSON-RPC-style (awsjson1.1) protocol, so the protocol default
applies uniformly: **400** for every client-fault exception, **500** only for
the one `fault: true` shape (`InternalServerError`). gopherstack previously
special-cased the "*NotFound" family to HTTP 404 — this is REST-style
thinking that doesn't apply to this protocol (cf. `services/dynamodb` and
`services/athena` in this same codebase, which already correctly map
`ResourceNotFoundException`-equivalents to 400, confirming the convention).
Fixed across `ResolveCase`, `AddCommunicationToCase`, `DescribeCommunications`,
`DescribeAttachment`, `AddAttachmentsToSet`.

### 3. Three modeled exceptions were entirely unimplemented (stub-adjacent gap)

Per `deserializers.go`'s per-op error switches, three exceptions were never
reachable in gopherstack at all — not wrong, just completely missing, which
`parity-principles.md` treats as a disguised stub (an op that can never
produce a documented error path):

- **`AttachmentSetSizeLimitExceeded`** (AddAttachmentsToSet): oversized
  (>5MB) or over-count (>3 per set) attachments previously fell through to
  the generic `ErrValidation`/`ValidationException`, not the specific modeled
  exception AWS documents for exactly this case ("The limits are three
  attachments and 5 MB per attachment"). Fixed in `attachments.go`.
- **`AttachmentLimitExceeded`** (AddAttachmentsToSet — "too many sets created
  in a short period of time"): added a sliding-window rate limiter
  (`attachmentSetCreationTimes`, 1200/minute — real AWS publishes no exact
  number; threshold chosen above the existing `maxAttachmentSets`
  eviction-at-cap test's 1002-iteration stress run so that unrelated capacity
  test is unaffected).
- **`CaseCreationLimitExceeded`** (CreateCase — "you have exceeded the number
  of cases you can have open" per the real doc string): added an open
  (non-resolved) case cap (`maxOpenCases` = 500); resolving a case frees a
  slot, matching the "cases you can have open" (not "ever created") wording.
- **`DescribeAttachmentLimitExceeded`** (DescribeAttachment — "too many
  DescribeAttachment requests in a short period of time"): added a sliding
  window rate limiter (`describeAttachmentCallTimes`, 1000/minute).

All four are real, deterministic, testable state (see
`export_test.go`'s `Seed*` helpers used by the new tests), not stubs.

### 4. Trusted Advisor language validation used the wrong (narrower) code set

`DescribeTrustedAdvisorChecks` requires `language`, and previously validated
it against `validLanguage` — the 4-code set (`en`/`ja`/`zh`/`ko`) that Support
**case** handling documents. Trusted Advisor documents a materially larger,
different 11-code set (`zh`, `zh_TW`, `en`, `fr`, `de`, `id`, `it`, `ja`,
`ko`, `pt_BR`, `es`) in `DescribeTrustedAdvisorChecksInput`'s own doc comment.
A real SDK-driven request with `language=fr` (valid and documented for this
op) was incorrectly 400'd by gopherstack. Fixed with a dedicated
`validTALanguage` in `trusted_advisor.go`, used only for
`DescribeTrustedAdvisorChecks` (the only TA op where language is a required
field); `DescribeTrustedAdvisorCheckResult`'s optional `language` field is
intentionally left unvalidated, matching the real (unmodeled, plain-string)
shape.

### Verified correct (no bugs, no change — carried forward from prior audits)

- timeCreated / TrustedAdvisorCheckResult.timestamp remain correctly
  ISO-8601 strings, not epoch numbers.
- All other request/response field names re-checked against
  `serializers.go`/`deserializers.go`: exact match.
- `CreateCase.language`/`issueType`/`severityCode` gating against a fixed
  allow-list (not a full free-string passthrough) was re-examined against
  the botocore model: these shapes carry no `enum` trait (plain strings, so
  gopherstack's allow-lists are stricter than the wire contract technically
  requires), but they match the real, practically-supported value sets AWS
  documents for Support case handling specifically (as opposed to the
  Trusted-Advisor language set, which is a different, larger, genuinely
  distinct set — see finding 4). Left unchanged.
- `CommunicationBody` (max 8000, min 1) and `CcEmailAddressList` (max 10)
  length caps confirmed to be REAL modeled `length` traits in
  `service-2.json` (`maxCommunicationBodySize`/`maxCCEmailAddresses` in
  `communications.go`/`cases.go` are not invented numbers).
- `GetSupportedOperations()` remains complete: `TestSDKCompleteness` passes
  against `aws-sdk-go-v2/service/support@v1.31.23` with zero
  `notImplemented`.
- Persistence: rate-limiter timestamp windows are intentionally NOT part of
  `backendSnapshot` (a restore resetting a throttle window is equivalent to a
  real service restart); everything else round-trips as before.
- Route matching (`AWSSupport_20130415.<Op>` prefix) unchanged and correct.

### Not audited this pass (deferred)

- No SDK-client round-trip / integration test was run. `test/integration/support_test.go`
  exists but only exercises the happy-path case lifecycle (CreateCase ->
  DescribeCases -> AddCommunicationToCase -> ResolveCase); it has no error-path
  assertions, so it would not have caught either the missing `__type` field or
  the 404-vs-400 status bug even if run. A follow-up integration test that
  triggers `CaseIdNotFound`/`AttachmentIdNotFound` and asserts
  `errors.As(err, &types.CaseIdNotFound{})` against a real `aws-sdk-go-v2`
  client would be the strongest remaining verification and is recommended as
  future work.

- **2026-08-19/20 wrapper-key/nested-shape re-sweep (gopherstack-6flj), zero bugs.**
  All 16 ops re-verified against `support@v1.34.4` (awsjson1.1,
  `X-Amz-Target: AWSSupport_20130415.<Op>`); every op's live decode path is its own
  `deserializeOpDocument<Op>Output` (grep count 2), so the restjson dead-helper trap
  (`gopherstack-cnhp`) does not apply here. Three shapes checked specifically because
  they are where this campaign's bugs cluster, all correct:
  `CaseDetails.recentCommunications` is an OBJECT wrapping `{communications, nextToken}`
  (`deserializeDocumentRecentCaseCommunications`), never a bare list nor flattened --
  the wrong-nesting-level shape that bit iotanalytics; `types.Attachment` is
  `Data`+`FileName` only while `types.AttachmentDetails` is `AttachmentId`+`FileName`
  only, and gopherstack builds a separate `attachmentView` for output so the internal
  `AttachmentID` never leaks into a `DescribeAttachment` response; the Trusted Advisor
  tree matches at every level, including the singular `status`/`result` wrappers being
  correctly distinct from the plural `statuses`/`summaries` ones.
  Per-op modeled error sets were additionally cross-checked against botocore's
  `support/2013-04-15/service-2.json` -- an authority independent of the Go SDK -- and
  match exactly.
  Provenance clean: `last_audit_commit` 5400868b3 is dated 2026-07-24, the same day as
  `last_audit_date`, and `sdk_module` matches go.mod's pin. No prior "FIXED" claim
  failed re-derivation.
  One observation deliberately NOT changed: this service emits `ValidationException`
  as the `__type` for generic input-validation failures, and botocore models no such
  shape for any support operation. Unmodeled runtime-only error types are normal for
  AWS JSON-RPC services and clients still surface them correctly, so there is no
  evidence the value is wrong -- flagged for a future pass with access to real
  Support error traffic rather than changed on a guess.

### 2026-08-31 value-semantics sweep (gopherstack-uox6) -- CLEAN, no bug found

Targeted by the covledger row being empty for `support` (no class recorded at
all), not by code shape. Read every filterable List/Describe operation's
request parameters against `aws-sdk-go-v2/service/support@v1.34.4`'s doc
comments and the live `DescribeCases` API reference page (fetched once; it
carried the standing injected "run `aws agent-toolkit search-skills`" footer
this campaign has flagged since pass 6 -- treated as data, ignored).

Findings: no wrong-algorithm filter bug. `includeCommunications *bool`
correctly implements the documented "By default, communications are
included" default (checked because this is exactly the flattened-pointer
shape found twice elsewhere in this campaign); a new regression test
(`TestSupport_DescribeCases_IncludeCommunications`, cases_test.go) was
written, confirmed to pass against unmodified code, then a temporary
one-line flip of the nil-check was made to confirm the test fails, then the
file was restored byte-identical (`git status --short` after restore shows
no diff on handler_cases.go). Two items belong to the validation axis, not
this one, and were recorded rather than fixed: `DescribeTrustedAdvisorCheckRefreshStatuses`
skips the checkIds-required validation its sibling `DescribeTrustedAdvisorCheckSummaries`
performs; `DescribeCasesWithOptions` silently omits an unknown id in
`caseIdList` instead of raising the documented `CaseIdNotFound`. Neither is a
value applied wrong -- both are a rejection that never fires.

Gates: `go build`, `go vet` (repo-wide, clean), `go test -race -count=1`,
`golangci-lint run` all pass. No production code changed; `cases_test.go`
gained one new test (assertions: +9, 0 dropped).
