---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: waf
sdk_module: aws-sdk-go-v2/service/waf@v1.33.4   # WAF Classic (legacy WAF/WAF Regional), distinct from wafv2
last_audit_commit: 8c56f4eb9
last_audit_date: 2026-08-07
overall: A            # 2026-08-29 (cursor-population sweep): all 16 List ops declare a real NextMarker
                      # (from the pinned SDK Output structs directly), and 15 of 16 already read
                      # NextMarker/Limit from the request and set NextMarker on the response through the
                      # shared paginate() helper (handler.go:124). The one exception, ListSubscribedRule-
                      # Groups, is correctly left unpaginated: its backend (rule_groups.go) always returns
                      # an empty slice -- there is no real AWS Marketplace subscription state for this
                      # mock to page over, so the gap is unobservable. No code changed this pass.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  GetChangeToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-06 (gopherstack-url6): now returns the outstanding PROVISIONED token on a repeated call before any mutation, matching api_op_GetChangeToken.go:23-27, instead of minting a fresh UUID every call."}
  GetChangeTokenStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: unknown token returns INSYNC per real AWS behavior (pre-existing, verified not re-broken)}
  GetSampledRequests: {wire: ok, errors: ok, state: ok, persist: n/a, note: "TimeWindow.StartTime/EndTime epoch-seconds shape verified ok; SampledHTTPRequest.Request/.Timestamp fields present for wire-shape completeness. This pass: WebAclId is now validated against real backend state -- unknown WebACL returns WAFNonexistentItemException instead of silently succeeding with an empty sample. RuleId is accepted without existence validation since real AWS defines it as one of three shapes (a Rule's RuleId, a RuleGroup's RuleGroupId, or the literal 'Default_Action'), and gopherstack has no verified AWS source pinning which combination is checked server-side, so validating it risks false rejections; unvalidated is the same behavior real AWS shows for the RuleGroupId/Default_Action cases. The sample list itself stays empty: see structural_gaps."}
  GetRateBasedRuleManagedKeys: {wire: ok, errors: ok, state: ok, persist: n/a, note: "RuleId is validated against real RateBasedRule state (WAFNonexistentItemException for unknown rule, pre-existing). ManagedKeys list itself stays empty: see structural_gaps."}
families:
  WebACL: {status: ok, note: "fixed CreateWebACL: added missing ChangeToken parameter (interface didn't even accept one) + validation on Create/Update/Delete. UpdateWebACL correctly applies INSERT/DELETE ActivatedRule updates and sorts by Priority. This pass: DeleteWebACL now returns WAFNonEmptyEntityException while Rules is non-empty."}
  Rule: {status: ok, note: "fixed Create/Update/Delete to validate ChangeToken; DeleteRule now returns WAFReferencedItemException if still activated in a WebACL or RuleGroup (previously deleted unconditionally, silently orphaning ActivatedRule references). This pass: DeleteRule now also returns WAFNonEmptyEntityException while Predicates is non-empty. 2026-09-06 (gopherstack-zalx): UpdateRule now rejects a redundant INSERT (Predicate already present) or DELETE of an absent Predicate with WAFInvalidOperationException, mirroring the UpdateWebACL/UpdateRuleGroup fix (gopherstack-1z1a)."}
  RateBasedRule: {status: ok, note: "same ChangeToken + ReferencedItem fixes as Rule (a RateBasedRule's RuleId can be activated in a WebACL with Type=RATE_BASED). This pass: DeleteRateBasedRule now also returns WAFNonEmptyEntityException while MatchPredicates is non-empty. 2026-09-06 (gopherstack-zalx): UpdateRateBasedRule now rejects a redundant INSERT/DELETE of a MatchPredicate the same way UpdateRule does."}
  IPSet: {status: ok, note: "ChangeToken validation added; DeleteIPSet now returns WAFReferencedItemException if referenced by a Rule/RateBasedRule Predicate.DataId. This pass: DeleteIPSet now also returns WAFNonEmptyEntityException while IPSetDescriptors is non-empty. 2026-09-06 (gopherstack-zalx): UpdateIPSet now rejects a redundant INSERT (IPSetDescriptor already present) or DELETE of an absent descriptor with WAFInvalidOperationException."}
  ByteMatchSet: {status: ok, note: "ChangeToken validation + ReferencedItem check on delete (same pattern as IPSet). This pass: DeleteByteMatchSet now also returns WAFNonEmptyEntityException while ByteMatchTuples is non-empty. 2026-09-06 (gopherstack-zalx): UpdateByteMatchSet now rejects a redundant INSERT/DELETE of a ByteMatchTuple with WAFInvalidOperationException."}
  SizeConstraintSet: {status: ok, note: "ChangeToken validation + ReferencedItem check on delete. This pass: DeleteSizeConstraintSet now also returns WAFNonEmptyEntityException while SizeConstraints is non-empty. 2026-09-06 (gopherstack-zalx): UpdateSizeConstraintSet now rejects a redundant INSERT/DELETE of a SizeConstraint with WAFInvalidOperationException."}
  SqlInjectionMatchSet: {status: ok, note: "ChangeToken validation + ReferencedItem check on delete. This pass: DeleteSqlInjectionMatchSet now also returns WAFNonEmptyEntityException while SqlInjectionMatchTuples is non-empty. 2026-09-06 (gopherstack-zalx): UpdateSqlInjectionMatchSet now rejects a redundant INSERT/DELETE of a SqlInjectionMatchTuple with WAFInvalidOperationException."}
  XssMatchSet: {status: ok, note: "ChangeToken validation + ReferencedItem check on delete. This pass: DeleteXssMatchSet now also returns WAFNonEmptyEntityException while XssMatchTuples is non-empty. 2026-09-06 (gopherstack-zalx): UpdateXssMatchSet now rejects a redundant INSERT/DELETE of an XssMatchTuple with WAFInvalidOperationException."}
  GeoMatchSet: {status: ok, note: "ChangeToken validation + ReferencedItem check on delete. This pass: DeleteGeoMatchSet now also returns WAFNonEmptyEntityException while GeoMatchConstraints is non-empty. 2026-09-06 (gopherstack-zalx): UpdateGeoMatchSet now rejects a redundant INSERT/DELETE of a GeoMatchConstraint with WAFInvalidOperationException."}
  RegexPatternSet: {status: ok, note: "ChangeToken validation; DeleteRegexPatternSet now returns WAFReferencedItemException if referenced by a RegexMatchSet tuple's RegexPatternSetId. This pass: DeleteRegexPatternSet now also returns WAFNonEmptyEntityException while RegexPatternStrings is non-empty. 2026-09-06 (gopherstack-zalx): UpdateRegexPatternSet now rejects a redundant INSERT/DELETE of a RegexPatternString with WAFInvalidOperationException."}
  RegexMatchSet: {status: ok, note: "ChangeToken validation + ReferencedItem check on delete (a RegexMatchSet is itself a match set referenceable from a Rule Predicate). This pass: DeleteRegexMatchSet now also returns WAFNonEmptyEntityException while RegexMatchTuples is non-empty. 2026-09-06 (gopherstack-zalx): UpdateRegexMatchSet now rejects a redundant INSERT/DELETE of a RegexMatchTuple with WAFInvalidOperationException."}
  RuleGroup: {status: ok, note: "ChangeToken validation; DeleteRuleGroup now returns WAFReferencedItemException if activated in a WebACL with Type=GROUP. DeleteRuleGroup also returns WAFNonEmptyEntityException while it still has activated rules. 2026-08-30 (marker-cursor sweep): UpdateRuleGroup's INSERT action now rejects a RuleId already active in the group (WAFInvalidParameterException) -- previously unchecked, so the same RuleId could be activated twice at different priorities, and since ListActivatedRulesInRuleGroup resumes pagination by matching a RuleId marker (handler_rule_groups.go), a duplicate RuleId broke that resume. Fixed at the mutation boundary rather than the read path, matching this repo's established pattern (e.g. wafv2 rate_based_rules.go rejecting duplicate Name/Priority on write)."}
  Tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource verified against real shapes -- no ChangeToken involved in real AWS, correctly not required here"}
  Logging: {status: ok, note: "PutLoggingConfiguration/GetLoggingConfiguration/DeleteLoggingConfiguration/ListLoggingConfigurations -- no ChangeToken in real AWS, correctly not required"}
  PermissionPolicy: {status: ok, note: "no ChangeToken in real AWS, correctly not required"}
  Migration: {status: ok, note: "CreateWebACLMigrationStack returns a deterministic S3 URL shape; genuinely can't produce a real migration template without wafv2 state, documented as a stub-shape return, not a disguised no-op"}
gaps:
  - "2026-08-29 (constrain-not-honoured sweep, confirmed clean): every List op's Limit/NextMarker is applied via the shared paginate() chokepoint (handler.go) except opListSubscribedRuleGroups, which ignores its request body entirely. Not fixed: ListSubscribedRuleGroups' backend (rule_groups.go) always returns an empty slice (structural_gaps: no marketplace-subscription simulation), so there is never more than zero items to paginate -- Limit/NextMarker have no observable effect either way. GetRateBasedRuleManagedKeys.NextMarker is documented on the SDK itself as \"not currently used\" (api_op_GetRateBasedRuleManagedKeys.go), correctly unread. No other List/Get op in this service accepts a filter/selector parameter beyond Limit/NextMarker on the pinned v1.33.4 SDK -- verified by reading every api_op_List*.go/api_op_Get*ManagedKeys.go input struct."
  - "2026-09-06 (gopherstack-y6ok, deliberately NOT fixed): ListTagsForResource performs no existence check on ResourceARN. WAFNonexistentItemException IS declared on ListTagsForResource's error list (deserializers.go awk recipe confirms it), but so is it declared, with the identical generic doc text (\"The operation failed because the referenced object doesn't exist.\", types/errors.go:440) on TagResource, UntagResource, and every Get/Delete op that unambiguously does existence-check by resource ID (e.g. GetIPSet, DeleteIPSet) -- the doc text is boilerplate on the exception type, not operation-specific behavior, and no sentence anywhere in api_op_ListTagsForResource.go (operation doc, ResourceARN field doc, or the error type doc) pins down that ListTagsForResource itself validates ARN existence. Declaring an error in a Smithy operation's error trait is not proof the emulated operation reaches it. Left unimplemented rather than guessing; if a doc source surfaces later that pins this down, revisit."
structural_gaps:
  - "GetSampledRequests always returns an empty SampledRequests list: real AWS randomly samples from actual HTTP requests evaluated against the WebACL's rules. Gopherstack has no request-proxying subsystem -- it never sees or evaluates real client traffic through WAF rules, so there is no request data to sample from, ever. Producing non-empty samples would mean fabricating fictitious HTTP requests, exactly the failure mode this parity campaign exists to remove. (WebAclId existence validation IS buildable from real state and was added this pass; the sample content is not.) (bd: gopherstack-smld)"
  - "GetRateBasedRuleManagedKeys always returns an empty ManagedKeys list: real AWS derives it from live request-rate tracking against the rule's RateLimit over a trailing 5-minute window, which requires the same real-traffic evaluation GetSampledRequests lacks. Nothing in InMemoryBackend's state (RateBasedRule config, WebACL associations) encodes request rates, so there is no rate to threshold against. (RuleId existence validation IS buildable and already present.) (bd: gopherstack-smld)"
deferred: []
leaks: {status: clean, note: "no goroutines/timers/background workers in this service; InMemoryBackend is plain locked maps + store.Table, no leak surface. New non-empty checks only read already-locked in-memory slices/maps under the existing coarse b.mu -- no new lock paths, no new persisted state. FIXED (gopherstack-cq0z, 2026-09-06): DeleteIPSet, DeleteRateBasedRule, DeleteRuleGroup, DeleteWebACL and DeleteRule all left their entry in the tags map. FIXED (gopherstack-y6ok, 2026-09-06): the remaining seven of twelve -- DeleteByteMatchSet, DeleteSizeConstraintSet, DeleteSqlInjectionMatchSet, DeleteXssMatchSet, DeleteGeoMatchSet, DeleteRegexPatternSet, DeleteRegexMatchSet -- had no ARN helper at all and never cleared their tags entry either; added byteMatchSetARN/sizeConstraintSetARN/sqlInjectionMatchSetARN/xssMatchSetARN/geoMatchSetARN/regexPatternSetARN/regexMatchSetARN (same arn.Build(\"waf\", \"\", accountID, \"<type>/<id>\") pattern as the five already-fixed families) and wired delete(b.tags, ...) into all seven. All twelve Delete ops now clear their tags entry. ListTagsForResource still has no existence check at all -- see gaps for why that half was deliberately left alone. See TestWAF_Delete_ClearsTags (now one subtest per family, all twelve)."}
---

## Notes

- **2026-08-14 (gopherstack-dv4s batch five): over-wide List-response audit, 13/13
  candidate ops verified clean, zero leaks.** All 13 List ops flagged by
  matching real SDK Output element type names against
  `Summary|Item|Brief|Entry|Ref|Preview|Metadata|Info`
  (`ListByteMatchSets`/`ListGeoMatchSets`/`ListIPSets`/`ListRateBasedRules`/
  `ListRegexMatchSets`/`ListRegexPatternSets`/`ListRuleGroups`/`ListRules`/
  `ListSizeConstraintSets`/`ListSqlInjectionMatchSets`/
  `ListSubscribedRuleGroups`/`ListWebACLs`/`ListXssMatchSets`) already use a
  dedicated `*Summary` Go type per family, each hand-verified field-for-field
  against the pinned `waf@v1.33.4` `types/types.go` declaration — every one
  is an exact `{Id, Name}` pair (`SubscribedRuleGroupSummary` also carries
  `MetricName`, matching `types.SubscribedRuleGroupSummary` exactly).
  `ListRateBasedRules` is the one case worth naming: gopherstack's
  `RateBasedRuleSummary` is a distinct Go type name, but the real
  `ListRateBasedRulesOutput.Rules` is `[]types.RuleSummary` — the same
  `{RuleId, Name}` shape WAF Classic reuses for plain Rules, confirmed by
  reading `api_op_ListRateBasedRules.go` directly rather than assuming a
  name match implied a type match. No shared Get/List converter exists
  anywhere in this family (`handler_match_sets.go`'s file-level comment
  explains the seven near-identical families were deliberately merged into
  one file for a `dupl` lint reason, not a shared-conversion reason) — the
  structural signal that has predicted a clean result everywhere else it
  held (cleanrooms' membership-scope ops, most of the dv4s pass-4 services)
  held again here.

- **ChangeToken workflow was a complete no-op before this pass.** Every mutating backend
  method (Create/Update/Delete across all 12 resource families) accepted a `changeToken`
  parameter but discarded it via `_`; `CreateWebACL`'s interface didn't even have the
  parameter. `ErrStaleToken` (WAFStaleDataException) was defined and wired into
  `Handler.handleError` but never returned by anything — any string, including `""` or a
  token from a completely different backend, worked as a "change token." Fixed by adding
  `InMemoryBackend.validateChangeToken(token string) error`, called at the top of every
  Create/Update/Delete method (after acquiring the write lock, before the existence/mutation
  logic): rejects unless `token` was returned by an earlier `GetChangeToken` call and is
  still `PROVISIONED` (not yet consumed). It does **not** itself consume the token — token
  consumption (`PROVISIONED` → `INSYNC`) stays where it already was, in
  `Handler.dispatch`'s post-success `MarkChangeTokenUsed` sniff of the request body. This
  split matters: `InMemoryBackend` methods are also called directly (bypassing the HTTP
  handler) by tests and potentially other code, and those callers never go through
  `dispatch`, so a token obtained via `GetChangeToken()` and reused directly against the
  backend stays `PROVISIONED` indefinitely unless `MarkChangeTokenUsed` is called
  explicitly. `persistence_test.go`'s full-state round-trip test relied on exactly this
  reuse pattern (one token, ~20 backend calls) and needed a small fix: it had been calling
  `MarkChangeTokenUsed` on its one token *before* reusing it for unrelated resource
  creation, which now correctly fails — split into a dedicated `staleToken` (marked used,
  asserted INSYNC after snapshot/restore) and a separate `token` that stays PROVISIONED for
  the actual resource-creation calls.

- **WAFReferencedItemException was defined and wired into the error-code switch but never
  returned.** Real AWS WAF Classic rejects deleting an object that is still referenced by
  another object: "you tried to delete a ByteMatchSet that is still referenced by a Rule,"
  "you tried to delete a Rule that is still referenced by a WebACL." Before this pass, every
  `Delete*` method deleted unconditionally once the target existed, silently leaving
  dangling `RuleId`/`DataId` references in whatever WebACL/RuleGroup/Rule/RateBasedRule
  still pointed at the deleted object — a `GetWebACL` after such a delete would return a
  `Rules[]` entry whose `RuleId` no longer resolves via `GetRule`, which real AWS makes
  structurally impossible. Fixed with three small reference-scan helpers on
  `InMemoryBackend` (all O(n) over the small in-memory resource sets, called with `b.mu`
  already held):
  - `ruleReferenced(id)` — true if `id` appears as `ActivatedRule.RuleId` in any WebACL's
    `Rules` or any RuleGroup's activated-rules list. Covers Rule, RateBasedRule, and
    RuleGroup deletion (WAF Classic's `WafRuleType` enum is `REGULAR | RATE_BASED | GROUP`,
    and all three share the same `RuleId`-in-`ActivatedRule` reference shape).
  - `matchSetReferenced(id)` — true if `id` appears as `Predicate.DataId` in any Rule's or
    RateBasedRule's predicates. Covers IPSet, ByteMatchSet, SqlInjectionMatchSet,
    XssMatchSet, SizeConstraintSet, GeoMatchSet, and RegexMatchSet deletion (a
    `Predicate.DataId` is a "unique identifier ... such as ByteMatchSetId or IPSetId,"
    untyped by resource kind on the wire, and UUID collision across kinds is not a
    practical concern).
  - `regexPatternSetReferenced(id)` — true if `id` appears as a
    `RegexMatchTuple.RegexPatternSetId` in any RegexMatchSet.

  Verified against every existing lifecycle test (`handler_audit1_test.go`,
  `handler_audit2_test.go`, `parity_b_test.go`) before writing the fix: none of them
  exercise a delete-while-referenced path, so no existing test assertions needed to change
  for this half of the fix (unlike the ChangeToken fix, which did require the
  `persistence_test.go` restructuring described above). New coverage in
  `backend_test.go` exercises both the rejection and the success-after-reference-removed
  path for each of the five reference shapes (Rule↔WebACL, Rule↔RuleGroup,
  RuleGroup↔WebACL via Type=GROUP, RateBasedRule↔WebACL via Type=RATE_BASED,
  IPSet↔Rule-predicate, RegexPatternSet↔RegexMatchSet).

- **GetSampledRequests wire-shape bug**: `TimeWindow.StartTime`/`EndTime` were modeled as
  JSON strings, but `aws-sdk-go-v2/service/waf`'s `serializeDocumentTimeWindow` always
  sends (and the deserializer always expects) a JSON **number** of seconds since the Unix
  epoch (`types.TimeWindow.{Start,End}Time` are `*time.Time`, protocol shape
  `unixTimestamp`) — this is the one and only timestamp-bearing shape in the entire WAF
  Classic API (`grep -rl time.Time` across `types/types.go` and every `api_op_*.go` in the
  SDK module confirms no other operation has this issue). A real SDK client's
  `GetSampledRequests` request would have failed to unmarshal server-side under the old
  code (`json: cannot unmarshal number into Go struct field ... of type string`). Fixed by
  changing the request-parsing struct's `StartTime`/`EndTime` fields to `float64`, which
  also fixes the response encoding for free (the same `float64` values are echoed back into
  the response map, and `encoding/json` renders a `float64` as a JSON number automatically).
  `parity_a_test.go`'s `TestParity_GetSampledRequestsReturnsTimeWindow` and
  `handler_audit1_test.go`'s `TestWAF_GetSampledRequests_EmptyStub` both asserted the old
  (wrong) ISO8601-string shape and were updated to send/expect epoch-seconds numbers.

- **`TargetString` in `ByteMatchTuple` is intentionally plain `string`, not base64** — this
  looks wrong at first glance (real `types.ByteMatchTuple.TargetString` is `[]byte`,
  serialized via `Base64EncodeBytes`) but is actually correct: gopherstack never decodes or
  re-encodes the field, just stores and echoes back whatever base64 text the SDK sent
  verbatim, so a real SDK client's own base64 encode-on-request / decode-on-response
  round-trips correctly through the opaque string. Confirmed by reading
  `serializers.go`/`deserializers.go` — do not "fix" this to `[]byte` in a future pass
  without checking whether that changes the stored representation in a way that breaks the
  round-trip (it currently doesn't, precisely because nothing on the gopherstack side ever
  interprets the bytes).

- **WAFNonEmptyEntityException was not modeled at all before this pass** (tracked as an
  explicit gap in the 2026-07-13 audit). Real AWS WAF Classic rejects deleting a container
  object while it still holds child entities, distinct from `WAFReferencedItemException`
  (which rejects deleting an object still referenced *by* something else): "You can't
  delete a WebACL if it still contains any Rules," "You can't delete a Rule if ... it
  still includes any predicates," and the equivalent doc comment on every other
  `Delete*` operation in `aws-sdk-go-v2/service/waf/api_op_Delete*.go` (confirmed by
  reading all twelve). Before this pass every `Delete*` method deleted unconditionally
  once the reference check passed, even with a non-empty child slice/map still attached —
  a `DeleteWebACL` on a WebACL with `Rules` still populated, or a `DeleteRule` on a Rule
  with `Predicates` still populated, both silently succeeded. Fixed by changing every
  `Delete*` method from `!b.<table>.Has(id)` to `<table>.Get(id)` (needed the value
  anyway to check its length) and adding `if len(<child slice/map>) > 0 { return
  ErrNonEmptyEntity }` after the existing `ErrReferencedItem` check, for all twelve
  families: WebACL (`Rules`), Rule/RateBasedRule (`Predicates`/`MatchPredicates`),
  RuleGroup (`ruleGroupRules[id]`), IPSet (`IPSetDescriptors`), ByteMatchSet
  (`ByteMatchTuples`), SizeConstraintSet (`SizeConstraints`), SqlInjectionMatchSet
  (`SqlInjectionMatchTuples`), XssMatchSet (`XssMatchTuples`), GeoMatchSet
  (`GeoMatchConstraints`), RegexMatchSet (`RegexMatchTuples`), and RegexPatternSet
  (`RegexPatternStrings`). New `ErrNonEmptyEntity` sentinel added to `errors.go`
  (`WAFNonEmptyEntityException`, HTTP 400, same `awserr.ErrConflict` class as
  `ErrReferencedItem`) and wired into `Handler.handleError`. Six pre-existing lifecycle
  tests (`byte_match_sets_test.go`, `size_constraint_sets_test.go`,
  `sql_injection_match_sets_test.go`, `xss_match_sets_test.go`, `geo_match_sets_test.go`,
  `regex_pattern_sets_test.go`) deleted a populated set directly and needed updating to
  first remove the tuple/pattern (matching real AWS) before the delete now correctly
  succeeds; every other existing lifecycle test already removed children before deleting
  (no change needed), which is itself evidence the bug had gone unexercised. New dedicated
  coverage in `non_empty_entity_test.go` (one test per family, all twelve) asserts both the
  blocked-while-non-empty case and the succeeds-after-removal case, following the same
  create → populate → blocked-delete → depopulate → delete pattern as the existing
  `referenced_item_test.go`.

- **SampledHTTPRequest wire-shape completeness**: the model was missing the `Request`
  (`HTTPRequest`) and `Timestamp` fields present on the real
  `types.SampledHTTPRequest` (`Request` is even marked "This member is required" in the
  SDK doc comment). Added `HTTPRequest`/`HTTPHeader` types and the two missing fields to
  `models.go`. Does not change any test-observable behavior today because
  `GetSampledRequests` always returns an empty `SampledRequests` list (the pre-existing,
  documented traffic-inspection stub) — this is forward-looking wire-shape correctness for
  if/when real sample data is ever populated, not a currently-reachable bug fix.

- **GetChangeTokenStatus's "unknown token → INSYNC" behavior** (comment + dedicated test
  `TestParity_ChangeTokenStatus_UnknownReturnsINSYNC`) predates this audit and was not
  touched. It is orthogonal to the `validateChangeToken` fix added this pass: the new
  validation lives in the *write* path (Create/Update/Delete reject an unknown/reused
  token), while `GetChangeTokenStatus` is a pure *read* that intentionally still returns
  `INSYNC` for a token it has never seen, matching the pre-existing, already-verified
  parity finding.

- **gopherstack-smld follow-up**: `GetSampledRequests` now validates `WebAclId` against
  `InMemoryBackend.webACLs` before returning, since that is real state the backend already
  holds — an unknown WebACL now gets `WAFNonexistentItemException` instead of a silent
  empty-sample 200. `GetRateBasedRuleManagedKeys` already validated `RuleId` the same way
  (pre-existing, unchanged). The actual sampled-request/managed-key *content* stays
  unimplemented and is now recorded in `structural_gaps` rather than `gaps`: gopherstack
  has no subsystem that proxies or evaluates real HTTP traffic through WAF rules, so there
  is no request/rate data for either op to report — inventing sample requests or blocked
  IPs would be fabrication, not emulation.

- **2026-08-28 wrapper-key/layer-2 re-sweep (bug class gopherstack-6flj/21my), no bugs
  found.** Protocol re-confirmed against the pinned `waf@v1.33.4` module:
  `awsAwsjson11_*` serializer prefix (JSON-RPC), not WAFv2's protocol — read directly, not
  assumed from `_PROTOCOLS.md`. Per-op manifest-mention check found the match-set
  families' individual Create/Get/Update op names (ByteMatchSet/IPSet/SizeConstraintSet/
  SqlInjectionMatchSet/XssMatchSet/GeoMatchSet/RegexPatternSet/RegexMatchSet),
  CreateRule/CreateRuleGroup/GetRuleGroup/UpdateRuleGroup,
  CreateRateBasedRule/GetRateBasedRule/UpdateRateBasedRule,
  ListActivatedRulesInRuleGroup, and PutPermissionPolicy/GetPermissionPolicy/
  DeletePermissionPolicy at zero literal mentions (this manifest tracks status by
  *family*, e.g. `IPSet:`, not by individual op name) — swept each against its own
  `api_op_*.go` Output struct and `deserializers.go` document-deserializer field-for-field
  at both the wrapper-key and nested-tuple/type layer. All confirmed clean: wrapper keys
  (`IPSet`/`ByteMatchSet`/.../`Rule` for GetRateBasedRule, `Rules` for
  ListRateBasedRules) match; `ByteMatchTuple.TargetString` was checked as a possible
  base64/[]byte type mismatch (real deserializer base64-decodes it,
  `deserializers.go:10420`) but gopherstack passes the wire-format base64 string through
  verbatim on both the accept and echo path (never decoding), so a real client's own
  base64 round trip still produces the original bytes -- not a bug, just an internal
  representation choice. `ActivatedRule`/`WafAction`/`WafOverrideAction`/`ExcludedRule`/
  `LoggingConfiguration`/`RedactedFields` also spot-checked clean. No source changes this
  pass.

- **2026-08-30 marker-cursor-over-a-tie-prone-key sweep.** Audited all 16 List ops'
  marker/sort key for duplicate-admission. All 12 `store.Table`-keyed listings
  (WebACLs/Rules/RateBasedRules/IPSets/ByteMatchSets/SizeConstraintSets/
  SqlInjectionMatchSets/XssMatchSets/GeoMatchSets/RegexPatternSets/RegexMatchSets/
  RuleGroups) sort/mark by their own `store.Table` key (`store_setup.go` `*KeyFn`
  functions) — duplicates structurally impossible. `ListLoggingConfigurations` marks by
  `ResourceArn`, also the table key. `ListTagsForResource` marks by `Tag.Key`, unique by
  Go map-key construction. `ListSubscribedRuleGroups` is unpaginated (always empty,
  documented in `structural_gaps`). The one exception: **`ListActivatedRulesInRuleGroup`**
  marks by `ActivatedRule.RuleId`, a field of a *side slice* (`b.ruleGroupRules[id]`), not
  a `store.Table` entry — `UpdateRuleGroup`'s INSERT action never checked for a
  already-active RuleId, so two `ActivatedRule` entries could share the same RuleId and
  break marker resume deterministically once a page boundary landed inside that pair.
  Fixed (see `RuleGroup` family note above); reproduced first in
  `rule_groups_test.go::TestUpdateRuleGroup_RejectsDuplicateRuleId` (fails against
  unmodified code, passes after the fix). All existing pagination fixtures
  (`pagination_test.go`) use distinct names/IDs throughout and could not have caught this.

- **2026-09-06 (gopherstack-url6): GetChangeToken minted a fresh token on every call,
  contradicting real AWS.** `api_op_GetChangeToken.go:23-27`: "Each create, update, or
  delete request must use a unique change token. If your application submits a
  GetChangeToken request and then submits a second GetChangeToken request before
  submitting a create, update, or delete request, the second GetChangeToken request
  returns the same value as the first GetChangeToken request." `GetChangeTokenStatus`
  (`PROVISIONED`/`PENDING`/`INSYNC`, `types/enums.go:24-30`) and `GetChangeTokenStatus`
  the operation were already modeled in this codebase pre-existing this pass, along with a
  PROVISIONED->INSYNC state machine and `validateChangeToken`/`MarkChangeTokenUsed` -- only
  `PENDING` is unmodeled, and that is an existing, orthogonal simplification (this is a
  synchronous single-process emulator with nothing to "propagate," so there is no
  intermediate state to occupy between a mutation succeeding and the token reaching INSYNC;
  not touched this pass). Fixed by adding a single `InMemoryBackend.outstandingChangeToken
  string` field (guarded by the same coarse `b.mu` as every other backend map, per this
  service's existing lock discipline): `GetChangeToken` returns it if non-empty instead of
  minting a new UUID, and `MarkChangeTokenUsed` clears it once the token it names is
  consumed by a successful mutation (dispatch only calls `MarkChangeTokenUsed` after `fn`
  returns no error, so a rejected create/update/delete never clears the outstanding token
  either -- matches real AWS: only a request that actually goes through starts a new
  provisioning cycle). Added to `backendSnapshot` as `OutstandingChangeToken
  string \`json:"outstandingChangeToken,omitempty"\`` -- purely additive, `wafSnapshotVersion`
  intentionally NOT bumped (`TestSnapshotVersionGuard` in `pkgs/persistence` hard-fails any
  version bump paired with a purely-additive field; ran with `-update` to refresh
  `pkgs/persistence/testdata/snapshot_inventory.json`, diff is exactly the one new field
  line, nothing else touched).

  `TestWAF_ChangeToken_Unique` was a **pre-existing test that encoded the bug**: it asserted
  two back-to-back `GetChangeToken` calls always differ. Rewritten to assert the real
  contract instead -- same token before any mutation, a genuinely new token only after the
  first is consumed by a successful `CreateRule` -- rather than deleted or weakened; see
  `change_token_test.go`. Also touched: `TestChangeTokenStatus_Lifecycle/DeleteWebACL_transitions_to_INSYNC`
  broke under the fix (not a target of this bug, but exposed by it) because its `mutate`
  closure called `wafCreateWebACL`, which internally fetches its own `GetChangeToken` --
  under the old bug that always returned an independent token, but under the fix it stole
  the SAME outstanding token the test had already fetched and was holding for the delete,
  leaving that captured token already-consumed (INSYNC) by the time `DeleteWebACL` tried to
  use it, and turning a should-succeed delete into a `WAFStaleDataException`. Restructured
  with a `setup` step that runs (and fully consumes its own token) before the outer token
  fetch, so ACL creation and the reserved delete token no longer compete for the same
  outstanding slot. A misleading comment in `persistence_test.go` ("a second, still-
  PROVISIONED token") was also updated -- true before the fix (two independent tokens),
  and still true in effect after it (same token returned twice, still PROVISIONED), but the
  old wording implied distinctness that no longer holds.

- **2026-09-06 (gopherstack-y6ok): tag leak on delete, real for 7 of the 12 families, and
  a `ListTagsForResource` existence check with insufficient evidence to add.** The filed
  issue said "none of the twelve Delete ops" clears `b.tags[arn]` -- checking each of the
  twelve directly found that overstated: `DeleteIPSet`, `DeleteRateBasedRule`,
  `DeleteRuleGroup`, `DeleteWebACL`, and `DeleteRule` already called
  `delete(b.tags, ...ARN(id))` (landed in the gopherstack-cq0z pass earlier the same day).
  The other seven -- `DeleteByteMatchSet`, `DeleteSizeConstraintSet`,
  `DeleteSqlInjectionMatchSet`, `DeleteXssMatchSet`, `DeleteGeoMatchSet`,
  `DeleteRegexPatternSet`, `DeleteRegexMatchSet` -- genuinely leaked: no ARN helper existed
  for any of the seven match-set families, and none cleared its tags entry, so a deleted
  set's tags persisted for the process lifetime and a set recreated under the same ARN
  would inherit them (Snapshot() also persists the leaked entries verbatim, so the leak
  grows the on-disk snapshot without bound). Fixed by adding the same
  `arn.Build("waf", "", accountID, "<lowercase-type>/<id>")` helper each of the five
  already-fixed families already uses (mechanically derived: `bytematchset/`,
  `sizeconstraintset/`, `sqlinjectionmatchset/`, `xssmatchset/`, `geomatchset/`,
  `regexpatternset/`, `regexmatchset/`, following the same lowercase-concatenation pattern
  as the existing `ipset/`, `ratebasedrule/`, `rulegroup/`, `rule/`, `webacl/`) and wiring
  `delete(b.tags, ...ARN(id))` into all seven `Delete*` methods. `TestWAF_Delete_ClearsTags`
  (`tag_leak_test.go`) extended from 5 to all 12 table cases, one subtest per family.

  The `ListTagsForResource` existence-check half was left alone. `WAFNonexistentItemException`
  IS declared on `ListTagsForResource`'s error list -- raw
  `deserializeOpErrorListTagsForResource` output: `"UnknownError" "WAFBadRequestException"
  "WAFInternalErrorException" "WAFInvalidParameterException" "WAFNonexistentItemException"
  "WAFTagOperationException" "WAFTagOperationInternalErrorException"` -- but the same
  exception, with the identical generic doc string ("The operation failed because the
  referenced object doesn't exist.", `types/errors.go:440`), is declared on every op that
  unambiguously existence-checks by resource ID (`GetIPSet`, `DeleteIPSet`, etc.) as well
  as on `TagResource`/`UntagResource`. That doc text is boilerplate attached to the
  exception *type*, not an operation-specific behavioral description, and nothing in
  `api_op_ListTagsForResource.go` -- not the operation doc, not the `ResourceARN` field
  doc -- says the op validates ARN existence. An error appearing in a Smithy operation's
  declared error set is not proof the emulated operation is meant to reach it; also worth
  noting real AWS's own `TagResource`/`UntagResource`/`ListTagsForResource` doc
  ("You can use this action to tag the AWS resources that you manage through AWS WAF
  Classic: web ACLs, rule groups, and rules" -- `api_op_TagResource.go:26-28`) describes a
  narrower taggable surface than what this generic ARN-keyed tag store already accepts for
  all twelve families (a pre-existing design choice, not touched this pass). Left
  unimplemented; recorded in `gaps`.
