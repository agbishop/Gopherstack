---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: accessanalyzer
sdk_module: aws-sdk-go-v2/service/accessanalyzer@v1.51.4
last_audit_commit: c79ebf1b5                             # HEAD when this manifest was written
last_audit_date: 2026-08-21
overall: A            # gopherstack-r80d batch 18 (required-output-member cut): Location.Span (nested 3 levels below ValidatePolicyOutput, invisible to a flat per-op required-field scan) was never emitted; fixed. Every other domain struct reachable from an Output field was re-verified end to end against accessanalyzer@v1.51.4/types/types.go and confirmed already correct, mostly by the 2026-08-15 gopherstack-6flj pass and gopherstack-kwht before it (re-confirmed, not re-litigated); prior 2026-08-10 wire-shape audit (19eea66b2) also re-confirmed, not re-litigated
ops:
  CreateAnalyzer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: now accepts+persists the AnalyzerConfiguration union (\"configuration\") and the inline \"archiveRules\" array (each creates a real ArchiveRule via CreateArchiveRule, including its auto-archive-existing-findings side effect), neither of which was previously read from the request body at all."}
  GetAnalyzer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: response now includes \"configuration\" when the analyzer has one (previously never returned, since Configuration was not modeled)."}
  ListAnalyzers: {wire: ok, errors: ok, state: ok, persist: ok, note: "Confirmed correctly omits \"configuration\" per the real API's ListAnalyzers/GetAnalyzer asymmetry (see analyzerToJSON's includeConfiguration param)."}
  DeleteAnalyzer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (leak): now cascade-deletes tags, findingRecommendations (by finding ID), analyzedResources, and accessPreviews for the deleted analyzer's ARN, in addition to the findings/archiveRules cascade that already existed. Previously b.tags[analyzerARN] and finding-recommendation/analyzed-resource/access-preview rows for the analyzer were never cleaned up -- ghost rows that would resurface (e.g. stale tags) if an analyzer of the same name was re-created."}
  UpdateAnalyzer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (was: state: partial): Configuration union is now read from the request body, persisted, and echoed back in the response. Also fixed a real wire-shape bug: the response wrongly included an \"arn\" key -- the real UpdateAnalyzerOutput has ONLY \"configuration\", no arn member. Also upgraded the backend method from RLock to Lock (it now genuinely mutates state instead of being a no-op read)."}
  CreateServiceLinkedAnalyzer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: now accepts configuration + inline archiveRules, same as CreateAnalyzer (CreateServiceLinkedAnalyzerInput has both fields on the real API too)."}
  DeleteServiceLinkedAnalyzer: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateArchiveRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj FIXED (real behavior bug, not wire-shape): previously archived EVERY existing active finding for the analyzer on rule creation, regardless of whether the finding matched the new rule's filter -- real AWS's auto-apply only archives findings matching the rule's own criteria. Now filters via matchesFindingFilter (findings.go) before archiving. A narrow rule (e.g. one resourceType) used to also archive every unrelated active finding."}
  GetArchiveRule: {wire: ok, errors: ok, state: ok, persist: ok}
  ListArchiveRules: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteArchiveRule: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateArchiveRule: {wire: ok, errors: ok, state: ok, persist: ok}
  ApplyArchiveRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj FIXED: RuleName is a required ApplyArchiveRuleInput member (api_op_ApplyArchiveRule.go:37-40) but was previously optional-and-ignored (`if ruleName != \"\"`); now required (empty -> ValidationException) and the named rule is looked up to retrieve ITS OWN filter, applied via matchesFindingFilter, instead of blanket-archiving every active finding regardless of which rule (if any) was named."}
  GetFinding: {wire: ok, errors: ok, state: ok, persist: ok, note: "Routing/resource/resourceOwnerAccount/analyzedAt fixed in a prior pass. FIXED THIS PASS: \"condition\" is a required Finding member (per types.Finding) and was previously omitted whenever a finding had no condition map; now always present (as {} when empty)."}
  ListFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "Same \"condition\" always-present fix as GetFinding (shared findingToJSON). gopherstack-6flj FIXED (discarded input): ListFindingsInput.Filter (map[string]types.Criterion, the real \"filter\" wire key) was decoded from the request body and threaded down to InMemoryBackend.ListFindings, but that method's filter parameter was named `_` -- entirely discarded. A real client's filter criteria were always a silent no-op; every finding for the analyzer came back regardless. Now applied via a new matchesFindingFilter helper (findings.go), which evaluates the Eq operator on the finding attributes this backend tracks as direct fields (status/resourceType/resource/id); Contains/Neq/Exists and any other filter key (principal.*, condition.*, action, isPublic, createdAt, resourceRegion) are still not evaluated -- disclosed below, not silently faked as always-matching-or-excluding. FIXED (constraining-parameter sweep, wrapper-key campaign): ListFindingsInput.Sort (*types.SortCriteria, wire key \"sort\": attributeName/orderBy) was never read from the request body at all -- results were always sorted ascending by ID regardless of what the client requested. Now decoded (FindingSortCriteria) and applied by sortFindings (findings.go), honoring the same attribute set matchesFindingFilter tracks (status/resourceType/resource/id) in ASC/DESC order; any other attributeName (e.g. createdAt, isPublic) falls back to the default ascending-by-ID order, same disclosed-scope convention as the filter fix. Proven via TestListFindings_RealClient_SortDescending (handler_findings_test.go), a real aws-sdk-go-v2 client round trip asserting the actual expected descending order, confirmed failing against the unfixed handler first."}
  UpdateFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (discarded input): UpdateFindingsInput.ResourceArn (serializers.go:3312-3315, an independently optional wire field distinct from ids) was never decoded from the request body at all -- a client selecting findings by resourceArn alone (no ids) silently updated nothing. Now parsed and applied: ids select findings when non-empty, otherwise resourceArn selects all findings for that resource; when both are given, resourceArn further narrows the ids selection. Proven via TestUpdateFindings_ByResourceArn (handler_findings_test.go, real request through the handler) and TestUpdateFindings_SelectionMode (findings_test.go)."}
  GetFindingV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (was: wire: partial): findingDetails now returns a real []types.FindingDetails-shaped array with one ExternalAccessDetails union member (condition/action/principal/isPublic, built from the same Finding fields findingToJSON already used) instead of always []; findingType is now \"ExternalAccess\" instead of absent. InMemoryBackend only ever produces external-access-shaped findings (AddFinding has no unused-access/internal-access modeling anywhere in this service), so reporting findingType=ExternalAccess + one ExternalAccessDetails member is a complete, honest representation of everything this backend can produce -- not a disguised partial stub of the other four union members (InternalAccessDetails/UnusedIamRoleDetails/UnusedIamUserAccessKeyDetails/UnusedIamUserPasswordDetails), which remain correctly unmodeled because InMemoryBackend has zero state to back them."}
  ListFindingsV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: findingType now \"ExternalAccess\" (FindingSummaryV2 has no findingDetails member at all, unlike GetFindingV2Output, so nothing else to add here). gopherstack-6flj FIXED (discarded input, worse than ListFindings' instance): ListFindingsV2Input.Filter was never even decoded from the request body -- the backend method took no filter parameter at all. Added the parameter (interfaces.go, findings.go) and wired matchesFindingFilter through, same scope/limits as ListFindings above. FIXED (constraining-parameter sweep): same missing-Sort bug as ListFindings -- ListFindingsV2Input.Sort was never read; now decoded and applied via the same sortFindings helper. Proven via TestListFindingsV2_RealClient_SortDescending."}
  GetFindingsStatistics: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (real wire-shape bug, not just a gap): types.ExternalAccessFindingsStatistics serializes its three counters as flat integers totalActiveFindings/totalArchivedFindings/totalResolvedFindings (confirmed against awsRestjson1_deserializeDocumentExternalAccessFindingsStatistics in the SDK's deserializers.go) -- gopherstack was emitting a nested {\"activeFindings\":{\"total\":N}} shape that no real deserializer recognizes; a real SDK client would have silently gotten zero counts back. Also added the missing analyzerArn-required validation (matches GetFindingsStatisticsInput's required field, same pattern as ListFindings). gopherstack-6flj FIXED (union wrapper-key bug, flagship of this pass): types.FindingsStatistics is a union keyed by wire name (awsRestjson1_deserializeDocumentFindingsStatistics, deserializers.go ~L9169) -- \"externalAccessFindingsStatistics\" for ACCOUNT/ORGANIZATION analyzers, \"unusedAccessFindingsStatistics\" for ACCOUNT_UNUSED_ACCESS/ORGANIZATION_UNUSED_ACCESS ones (this backend explicitly models all four AnalyzerType values, models.go). The handler always emitted the external-access key regardless of the target analyzer's own Type; a real client's typed union switch on an unused-access analyzer's statistics would decode into the wrong Go type entirely. Now selects the wire key from the looked-up analyzer's Type. unusedAccessFindingsStatistics.TopAccounts/UnusedAccessTypeStatistics are left unset -- DISCLOSED, not synthesized: no per-principal-account aggregation or unused-access-type categorization exists anywhere in this backend's Finding model to derive them from honestly."}
  GenerateFindingRecommendation: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindingRecommendation: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (real wire bugs, kept as gap otherwise): resourceArn and startedAt (both required GetFindingRecommendationOutput members) and completedAt were entirely missing from the response; now populated from the finding record and the recommendation job's own timestamps. recommendationType's wire value was \"UNUSED_PERMISSION\", which does not match the real types.RecommendationType enum's only value, \"UnusedPermissionRecommendation\" (enums.go:579) -- fixed. Also fixed a silent-accept bug: GenerateFindingRecommendation previously created a recommendation record for ANY finding ID, including nonexistent ones, without checking it existed; it now 404s (ResourceNotFoundException) like GetFindingRecommendation already did, and captures the finding's real resourceArn while doing so. recommendedSteps remains always [] -- content generation is still a genuinely separate feature (IAM Access Analyzer's unused-permission-removal recommendation engine) with no state in this backend to derive it from; Status is always SUCCEEDED (synchronous), matching the StartPolicyGeneration convention elsewhere in this service."}
  GetAnalyzedResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (real wire-shape bug): resourceOwnerAccount is a required types.AnalyzedResource member and was entirely missing from the response; now defaults to the backend's own AccountID(), the same convention findingToJSON already used for Finding.resourceOwnerAccount. gopherstack-6flj DISCLOSED (not fixed): types.AnalyzedResource's optional Actions/Error/SharedVia/Status members are still never emitted. AnalyzedResource (models.go) has no state for any of the four, and AddAnalyzedResource/AddFinding are two independent synthetic paths with no enforced link between an analyzed resource and a same-ARN finding in this backend -- deriving Status from a coincidentally-matching Finding.Status would be exactly the adjacent-but-conceptually-different-data derivation parity-principles #1 warns against, not a same-concept aggregation like the GetFindingsStatistics/ArchiveRule fixes above. Also noted: analyzedResourceToJSON emits an extra \"analyzerArn\" key that types.AnalyzedResource does not have on the wire at all -- harmless (a real client's deserializer's `default:` case silently discards unknown keys), not a missing-data bug, left as-is rather than churned."}
  ListAnalyzedResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: same resourceOwnerAccount fix as GetAnalyzedResource -- it's also required on types.AnalyzedResourceSummary and was missing from every list item."}
  StartResourceScan: {wire: ok, errors: ok, state: ok, persist: n/a, note: "verifies analyzer exists by ARN; no actual resource scanning to simulate (matches other AA scan endpoints elsewhere in gopherstack)"}
  StartPolicyGeneration: {wire: ok, errors: ok, state: ok, persist: ok, note: "completes synchronously (SUCCEEDED immediately) rather than modeling async IN_PROGRESS -- acceptable since it still reaches a real terminal state and GetGeneratedPolicy/ListPolicyGenerations reflect it; not a stuck-forever no-op. FIXED THIS PASS (silent drop): the optional cloudTrailDetails member (types.CloudTrailDetails) was parsed from the request but entirely discarded; now stored and echoed back (see GetGeneratedPolicy)."}
  GetGeneratedPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (real wire-shape bug): jobDetails wrongly included \"principalArn\" -- the real types.JobDetails (GetGeneratedPolicyOutput.jobDetails) has NO principalArn member; that value only exists under generatedPolicyResult.properties.principalArn (types.GeneratedPolicyProperties), which was already correct. Split the shared serializer into jobDetailsToJSON (no principalArn) vs policyGenerationToJSON (has principalArn, used by ListPolicyGenerations' types.PolicyGeneration, which DOES carry it) so the two real, differently-shaped types stop being conflated. FIXED THIS PASS: properties.cloudTrailProperties (types.CloudTrailProperties) is now populated from the cloudTrailDetails supplied to StartPolicyGeneration, when present -- previously silently dropped on the floor despite being real, client-supplied, already-available data (same pattern as Analyzer.Configuration from a prior pass). generatedPolicies still always [] -- IAM policy statement synthesis from CloudTrail activity remains a distinct, unimplemented analysis engine with no backing data in this backend."}
  CancelPolicyGeneration: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPolicyGenerations: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (discarded input): ListPolicyGenerationsInput.MaxResults/NextToken (serializers.go:2571-2577, real query params) were never read from the request at all -- every call returned every job in one page with no nextToken, regardless of maxResults. Now parsed and applied (same start/nextToken-by-JobID pagination pattern as ListFindings). Proven via TestListPolicyGenerations_MaxResultsAndNextToken (handler_generated_policies_test.go)."}
  CreateAccessPreview: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-afi1: Configurations, the required access-control configuration being previewed (api_op_CreateAccessPreview.go:39-43, a 13-member types.Configuration union per resource type -- confirmed via awsRestjson1_serializeDocumentConfiguration in serializers.go), was read by neither the handler's decode struct nor the backend method signature at all -- only analyzerArn was ever consulted. Now decoded (map[string]json.RawMessage, \"configurations\" wire key) and validated to contain exactly one element (the doc comment's stated constraint); stored opaquely rather than decoded into the full union, since ListAccessPreviewFindings (this backend's only Configurations-adjacent behavior) reuses the analyzer's existing findings and never interprets Configurations' semantic content -- see AccessPreview.Configurations godoc (models.go) for the full reasoning. Missing/multi-entry Configurations -> ValidationException, following this handler's existing analyzerArn-required convention (this op declares no validation-style exception in its own error switch)."}
  GetAccessPreview: {wire: fixed, errors: ok, state: ok, persist: ok, note: "response now echoes Configurations back (accessPreviewToJSON(ap, true)), matching real GetAccessPreviewOutput.accessPreview (types.AccessPreview, which has a Configurations member) -- see CreateAccessPreview."}
  ListAccessPreviews: {wire: ok, errors: ok, state: ok, persist: ok, note: "unaffected by the CreateAccessPreview fix: real ListAccessPreviewsOutput.accessPreviews is []types.AccessPreviewSummary, which has NO Configurations member (unlike Get's types.AccessPreview) -- accessPreviewToJSON(ap, false) correctly omits it here, same asymmetry as ListAnalyzers/GetAnalyzer's Configuration field above."}
  ListAccessPreviewFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (was: wire: partial): now builds the real types.AccessPreviewFinding shape (id/changeType/resourceOwnerAccount/resourceType/status/createdAt required members, plus action/principal/condition/isPublic when set) via a new accessPreviewFindingToJSON, instead of reusing findingToJSON's v1 Finding/FindingSummary shape (which has analyzerArn and no changeType -- a different, incompatible shape). Every finding is reported as changeType \"NEW\" since access previews here are not diffed against a prior finding set, so existingFindingId/existingFindingStatus are never populated (both are documented as \"provided only for existing findings\"). FIXED (cmd/enumcheck sweep, 1d6e40d1a): changeType was the non-member string \"New\" -- types.FindingChangeType only has NEW/UNCHANGED/CHANGED (types/enums.go:237-244), all-caps -- now emits \"NEW\"; see TestListAccessPreviewFindings_ChangeType_RealSDKClient (wire_field_fixes_test.go). Also added the missing analyzerArn-required validation (ListAccessPreviewFindingsInput requires it). gopherstack-6flj FIXED (discarded input, third instance of the ListFindings/ListFindingsV2 pattern): ListAccessPreviewFindingsInput.Filter was decoded from the body but the backend method took no filter parameter at all -- same fix, same matchesFindingFilter, same disclosed scope."}
  CheckAccessNotGranted: {wire: ok, errors: ok, state: gap, persist: n/a, note: "genuine IAM policy evaluation (policy_analysis.go), not a stub. FIXED 2026-08-30 (gopherstack-4a8v, anonymous-struct sweep): policyType is a required CheckAccessNotGrantedInput member (accessanalyzer@v1.51.4 api_op_CheckAccessNotGranted.go) that was parsed off the wire and never validated or forwarded anywhere -- CheckAccessNotGranted(policyDoc, accesses) takes no policyType param at all. Added a required-field check. NOT fixed (gap, layer-boundary risk): the underlying evaluation still doesn't distinguish IDENTITY_POLICY from RESOURCE_POLICY (e.g. no Principal-aware analysis for resource policies) -- doing so would need new policy-evaluation semantics this pass didn't invent. A real typed SDK client can never omit policyType (validateOpCheckAccessNotGrantedInput rejects it client-side before any request is sent), so this gap is only reachable by a non-SDK/raw HTTP caller; the required-field test therefore drives the raw HTTP path (handler_policy_validation_test.go), not the typed client. FIXED 2026-09-06 (gopherstack-xyu4, deep-read audit): actionAllowed/resourceAllowed (policy_analysis.go) checked only Action/Resource and silently ignored NotAction/NotResource, so a NotAction- or NotResource-only Allow statement (which grants the *complement* of the listed set -- e.g. NotAction:s3:PutObject grants every other action) was treated as granting nothing at all. That produced a confident, wrong PASS for exactly the kind of broad grant this op exists to catch. Fixed by adding NotAction/NotResource complement matching; see TestCheckAccessNotGrantedLogic/not_action_grants_other_actions_fails and .../not_resource_grants_other_resources_fails (policy_analysis_test.go), both proven to fail against the pre-fix code. Still NOT fixed (gap, disclosed below): a policyDocument that isn't valid JSON is silently parsed to an empty policy (parsePolicy swallows the json.Unmarshal error) rather than erroring, so this op reports PASS for garbage input instead of InvalidParameterException/UnprocessableEntityException -- same confident-wrong-answer shape, left for a follow-up issue since fixing it means adding error returns to a currently-infallible function signature shared by all three Check* ops."}
  CheckNoNewAccess: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-30 (gopherstack-4a8v): policyType is a required CheckNoNewAccessInput member, parsed but never validated (CheckNoNewAccess(existingDoc, newDoc) doesn't take it either, same as CheckAccessNotGranted -- see its note for why that deeper gap is left alone). Added a required-field check. FIXED 2026-09-06 (gopherstack-xyu4): same NotAction/NotResource fix as CheckAccessNotGranted (shared stmtGrants/policyGrants helpers) -- an existing policy that grants broadly via NotAction/NotResource was previously invisible to policyGrants, so CheckNoNewAccess reported a false FAIL (\"new access\") for access the existing policy already granted. See TestCheckNoNewAccessLogic/existing_not_action_already_covers_new_grant_passes. NOT fixed (gap, unbounded): the *new* statement's own NotAction/NotResource are still not expanded when walking newAccessReasons -- doing so would require enumerating \"every action/resource except these\" against an unbounded action/resource space, which is the automated-reasoning problem this mock cannot solve; a new statement written with NotAction/NotResource is under-reported (may PASS when it should FAIL). Disclosed, not fixed -- see the deep-read note at the bottom of this file (gopherstack-xyu4)."}
  CheckNoPublicAccess: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-30 (gopherstack-4a8v): resourceType is a required CheckNoPublicAccessInput member, parsed but never validated or used (CheckNoPublicAccess(policyDoc) ignores it -- this mock has no resource-type-specific evaluation, e.g. S3 bucket vs KMS key rules, same structural gap class as CheckAccessNotGranted's policyType). Added a required-field check."}
  ValidatePolicy: {wire: ok, errors: ok, state: partial, persist: n/a, note: "NEW gap noted 2026-08-30 (gopherstack-4a8v): nextToken is parsed off the wire and never used -- ValidatePolicy always returns every finding on one page with no NextToken out. Not fixed: maxResults isn't even parsed (a separate, unflagged wire gap), so there's no natural page size to paginate against without inventing one; this mock's finding set is deterministic and small enough to plausibly fit on one page every time, the same honest-gap shape as GetStatementResult's single-page demo data elsewhere in this repo. gopherstack-6flj FIXED: findingDetails is a required types.ValidatePolicyFinding member (\"a localized message that explains the finding\") and was never emitted at all. Added findingDetailMessages, a static IssueCode->message lookup covering every code this package's validators can produce (locked in by TestValidatePolicy_FindingDetailsPopulated, which fails if any finding is emitted with an empty message). 2026-08-21 gopherstack-r80d batch 18 FIXED (real bug, one level deeper): each types.Location within the required Locations array requires its own Span member (types/types.go:1509-1521, v1.51.4) -- Path was present but Span was never emitted at all (rootLoc/fieldLoc/stmtLoc/stmtFieldLoc, policy_analysis.go, built only \"path\"). A real client's Location.Span decoded to nil for every ValidatePolicy finding ever returned. This is a domain struct invisible to a flat per-op scan of ValidatePolicyOutput (whose own only required member is the top-level Findings array) AND one level deeper than ValidatePolicyFinding's own required Locations (which was already correctly populated) -- it's the Location entries *inside* Locations that were missing their own required member. Fixed with attachSpans/resolveRawAt (policy_analysis.go): each Location's real byte range is recovered from the original policyDocument text via its json.RawMessage bytes (copied verbatim by encoding/json, not re-synthesized), with a step-by-step fallback toward the document root so Span is never dropped even when the specific key a finding is about (e.g. a wholly absent \"Effect\") can't itself be located. Proven via a real aws-sdk-go-v2/service/accessanalyzer client round trip (wire_output_required_r80d_test.go): one test asserts Span/Start/End/Position fields are never nil across 4 finding shapes (root-span, field-span, and a 2-statement case exercising the duplicate-element search), a second asserts the span's byte range exactly bounds the real `\"Permit\"` substring for an INVALID_EFFECT finding. Hand-reverted/confirmed-failing (all 4 subtests + the accuracy test)/restored, md5sum byte-identical."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  route_matcher: {status: ok, note: "FIXED THIS PASS: deleted pathAnalyzedResource (\"analyzedResource\", no hyphen) dead legacy routing -- RouteMatcher claimed it but no parser ever resolved an op for it (always 404'd; no real SDK client sends this path, only the real hyphenated \"/analyzed-resource\" via pathAnalyzedResourceHyph). Removed the RouteMatcher prefix entry and the dead parseRESTPath case; updated TestAccessAnalyzerHandler_RouteMatcher accordingly (now asserts /analyzedResource is NOT claimed and /analyzed-resource IS). All other families re-verified unchanged against aws-sdk-go-v2 serializers.go this pass (archive-rule PUT/GET/DELETE paths+methods, tags GET/POST/DELETE, policy-generation PUT/GET paths, access-preview PUT/GET/POST) -- no further routing bugs found."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "GetFindingRecommendation.recommendedSteps is always [] -- IAM Access Analyzer's actual unused-permission-removal recommendation content generation is a distinct feature with no backing state in InMemoryBackend to derive concrete steps from (RecommendationType/ResourceArn/Status/StartedAt/CompletedAt are ALL real, state-backed, and correctly wire-shaped as of gopherstack-kwht). Not attempted this pass; would need a genuine recommendation-generation model, not a fabricated placeholder. Tracked as bd issue gopherstack-kwht."
  - "GetGeneratedPolicy.generatedPolicyResult.generatedPolicies is always [] -- actual IAM policy generation from CloudTrail activity is a distinct, large feature (statement synthesis from simulated CloudTrail events) with no backing data in this backend. properties (including cloudTrailProperties as of gopherstack-kwht)/jobDetails ARE real, state-backed. Tracked as bd issue gopherstack-kwht."
  - "gopherstack-6flj: ListFindings/ListFindingsV2/ListAccessPreviewFindings filter criteria only evaluate the Eq operator on status/resourceType/resource/id -- Contains/Neq/Exists, and any filter key not backed by a direct Finding field (principal.*, condition.*, action, isPublic, createdAt, resourceRegion), are not evaluated (matchesFindingFilter treats them as satisfied rather than excluding, which is closer to the pre-fix always-match baseline than silently hiding results a real client should see). Same limitation applies to CreateArchiveRule/ApplyArchiveRule's auto-archive matching, which reuses the same helper."
  - "gopherstack-6flj: types.AnalyzedResource's optional Actions/Error/SharedVia/Status members are never emitted by GetAnalyzedResource -- no backing state anywhere in this backend (AnalyzedResource and Finding are two unlinked synthetic-data paths); see the GetAnalyzedResource op note above for why deriving Status from a same-ARN Finding was declined rather than attempted."
  - "gopherstack-r80d batch 18: ValidatePolicy's Locations never use the types.PathElementMemberSubstring path-element variant (\"substring\", pointing at a range within a literal string rather than a whole key/value/array-index) -- none of this package's validators (Version/Effect/Action-NotAction/Resource-NotResource/permissiveness) analyze substrings of a policy value, so the variant is correctly never constructed rather than dropped; a missing-feature gap (this analyzer's checks are coarser than real IAM Access Analyzer's), not a dropped-required-field bug, since PathElement itself (an interface/union) has no required members of its own to drop."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "store.go/store_setup.go/persistence.go internal locking and Table[T]/Index[T] generic implementation (pkgs/store) not re-audited line-by-line this pass beyond the DeleteAnalyzer cascade fix and the Configuration field addition to the Analyzer table's JSON shape (verified generically compatible with store.Table's JSON-marshal-based Snapshot/Restore, no special-casing needed); no correctness issues observed."
leaks: {status: clean, note: "FIXED THIS PASS: DeleteAnalyzer previously left ghost rows in tags/findingRecommendations/analyzedResources/accessPreviews (see DeleteAnalyzer note above) -- these are now cascade-deleted. No goroutines/janitors in this service; all state is synchronous map/store access under lockmetrics.RWMutex, and every lock acquisition uses defer Unlock/RUnlock (re-verified this pass)."}
---

## Notes

**Protocol**: restjson1. Timestamps are ISO8601 strings via `smithytime.ParseDateTime`
on the real deserializer side (NOT epoch-seconds) -- `time.RFC3339` formatting used
throughout gopherstack's handler*.go is correct; do not "fix" this to `awstime.Epoch`
in a future pass.

### 2026-08-21 gopherstack-r80d batch 18 (required OUTPUT member cut)

Re-verified accessanalyzer end to end for this campaign's target class (a
required response member the handler never populates), since the flat
per-op `cmd/requiredoutputfields` count (28 fields / 39 ops / 17
ops-with-required) only reflects each op's own top-level `<Op>Output`
struct. Cross-checked with two independent implementations of a full
`types.go` domain-struct walk (a character-level brace matcher and a
`go/parser`/`go/ast`-based parser) that agreed exactly: 117 structs total,
41 carrying at least one required member, 114 required fields summed --
nearly 4x the flat count. Read every op's response-building code
(`analyzers.go`/`archive_rules.go`/`findings.go`/
`analyzed_resources.go`/`access_previews.go`/`policy_analysis.go`/
`handler_*.go`) against every domain struct actually reachable from an
Output field (cross-checked via a repo-wide grep of every `api_op_*Output`
struct's own `types.X` field types, not just the ones the flat scan
flagged): `AccessPreview(Finding/Summary)`, `AnalyzedResource(Summary)`,
`AnalyzerSummary`, `ArchiveRuleSummary`, `Finding(Summary/SummaryV2)`,
`GeneratedPolicyResult`/`GeneratedPolicyProperties`/`JobDetails`/
`PolicyGeneration`, and `ValidatePolicyFinding`/`Location`/`Span`/
`Position`. Two ops (`GetFinding`, `GetAnalyzedResource`) don't even appear
in the flat scan's 17-op list -- their own top-level `Finding`/`Resource`
fields aren't Smithy-required -- yet nest `Finding`/`AnalyzedResource`
structs that themselves carry 8 and 7 required members respectively; both
were already fully correct (fixed by the 2026-08-15 gopherstack-6flj pass).

**1 bug found and fixed**: `ValidatePolicyFinding.Locations` (required,
already correctly populated) is `[]types.Location`, and `types.Location`
itself requires both `Path` (already correct) and `Span`
(types/types.go:1509-1521) -- `Span` was never emitted at all by
`rootLoc`/`fieldLoc`/`stmtLoc`/`stmtFieldLoc` (policy_analysis.go), so
every `ValidatePolicy` finding a real client has ever decoded had
`Location.Span == nil`. This is one level deeper than the
already-known-good `Locations` array itself -- the *elements inside* it
were missing their own required member, invisible to both the flat per-op
scan and a naive struct-count of `ValidatePolicyFinding` alone. Fixed with
`attachSpans`/`resolveRawAt` (policy_analysis.go): the real byte range is
recovered from the original `policyDocument` text via each value's own
`json.RawMessage` bytes (copied verbatim by `encoding/json`, not
re-synthesized), walking the same path shape `rootLoc`/`fieldLoc`/
`stmtLoc`/`stmtFieldLoc` already built; a step-by-step fallback toward the
document root guarantees `Span` is never dropped even when the specific
key a finding is about (e.g. a wholly absent `"Effect"`) can't itself be
located. Proven via `wire_output_required_r80d_test.go` against a real
`aws-sdk-go-v2/service/accessanalyzer` client: one test asserts
`Span`/`Start`/`End`/`Line`/`Column`/`Offset` are never nil across 4
finding shapes (root-span, field-span, and a 2-statement case exercising
duplicate-element disambiguation), a second asserts the span's byte range
exactly bounds the real `"Permit"` substring for an `INVALID_EFFECT`
finding. Hand-reverted (all subtests fail without the fix, confirmed)/
restored, md5sum byte-identical.

**Everything else reachable from an Output field was already correct**,
mostly from the 2026-08-15 gopherstack-6flj pass and gopherstack-kwht
before it: `AccessPreview`/`AccessPreviewFinding`/`AccessPreviewSummary`
(all 5/6/4 required members present, `Status` always
`AccessPreviewStatusCompleted` synchronously), `AnalyzedResource`/
`AnalyzedResourceSummary` (7/3 required members present, `IsPublic` a
plain non-pointer `bool` so always emitted), `AnalyzerSummary` (5/5
present), `ArchiveRuleSummary` (4/4 present; `Filter` is also required on
*input* to `CreateArchiveRule`, api_op_CreateArchiveRule.go:43-46, so a
real client can never construct one with a nil filter -- structurally
unreachable, same class as ce's commitment-analysis fields from batch 17),
`Finding`/`FindingSummary`/`FindingSummaryV2` (8/8/7 present, matching the
already-fixed `condition`-always-present convention), `GeneratedPolicyResult`/
`GeneratedPolicyProperties`/`JobDetails`/`PolicyGeneration` (1/1/3/4
present, including `CloudTrailProperties.EndTime` -- required on output,
optional on input, already correctly defaulted to now() when the caller
omits it, the same "optional-on-input/required-on-output" shape as efs's
`Destination.Region` from batch 17). The 13-member `AnalyzerConfiguration`/
`Configuration` unions (`CreateAccessPreview`'s `Configurations`,
`CreateAnalyzer`'s `configuration`) are stored and echoed back as opaque
`json.RawMessage`, never decoded field-by-field -- so their constituent
structs' own required members (`KmsGrantConfiguration`,
`S3BucketAclGrantConfiguration`, `S3PublicAccessBlockConfiguration`,
`VpcConfiguration`, `Trail`) are exactly whatever bytes a real client
itself sent, genuinely inapplicable to this bug class rather than merely
unaudited. `ReasonSummary` (`Check*` ops) and `FindingsStatistics`'s
concrete union members have zero required fields at all (confirmed by both
walk implementations), so nothing to check there.

**Disclosed, not fixed** (no new state to derive from, matching this
service's existing gap-disclosure convention -- see `gaps:` above):
`GetFindingRecommendation`'s `RecommendedStep` union member
(`UnusedPermissionsRecommendedStep.RecommendedAction`) is never
constructed since `recommendedSteps` stays `[]` (pre-existing
gopherstack-kwht gap); `JobDetails.JobError`/`GetFindingRecommendationOutput`'s
`RecommendationError` are optional fields never populated since no job in
this backend ever reaches a `FAILED` state (synchronous success only,
consistent with `StartPolicyGeneration`'s existing documented behavior);
`GeneratedPolicyResult.GeneratedPolicies` stays `[]` (pre-existing
gopherstack-kwht gap, so its required `Policy` member is never
constructed, vacuously safe); `types.PathElementMemberSubstring` is never
used by any validator here (see `gaps:` above -- a missing-feature gap,
not a dropped-required-field one, since `PathElement` itself has no
required members).

**This pass's fixes, in order of severity**:

1. **GetFindingsStatistics wire-shape bug** (real bug, not a gap): the real
   `types.ExternalAccessFindingsStatistics` serializes `totalActiveFindings`/
   `totalArchivedFindings`/`totalResolvedFindings` as flat integers (confirmed against
   `awsRestjson1_deserializeDocumentExternalAccessFindingsStatistics` in the SDK's
   `deserializers.go`), not the `{"activeFindings":{"total":N}}` nested-object shape
   gopherstack was emitting. A real SDK client parsing gopherstack's old response would
   have gotten all-zero counts back silently. Fixed in `handleGetFindingsStatistics`
   (handler_findings.go).
2. **GetGeneratedPolicy jobDetails wrongly included `principalArn`**: `types.JobDetails`
   (the real `GetGeneratedPolicyOutput.jobDetails` type) has no such member -- only
   `types.PolicyGeneration` (used by `ListPolicyGenerations`) does. The two were being
   built by one shared function; split into `jobDetailsToJSON`/`policyGenerationToJSON`
   (handler_generated_policies.go).
3. **GetAnalyzedResource/ListAnalyzedResources missing required `resourceOwnerAccount`**:
   both `types.AnalyzedResource` and `types.AnalyzedResourceSummary` require it; neither
   response included it. Fixed in handler_analyzed_resources.go.
4. **ValidatePolicy findingDetails (required) never emitted**: added a static
   IssueCode -> message table (`findingDetailMessages`, policy_analysis.go) covering
   every code the validators produce.
5. **UpdateAnalyzer response wrongly included `arn`**: the real `UpdateAnalyzerOutput`
   has only `configuration`. Fixed alongside implementing the Configuration union for
   real (see gap-closure below).
6. **v1 Finding's required `condition` field was conditionally omitted** instead of
   always present (as `{}` when empty) -- fixed in `findingToJSON`.
7. **DeleteAnalyzer ghost-row leak**: tags, finding recommendations, analyzed
   resources, and access previews for a deleted analyzer's ARN were never cleaned up.
   Fixed with an explicit cascade (see `analyzers.go`); locked in by
   `TestDeleteAnalyzer_CascadesGhostRows`.
8. **Dead route deleted**: `pathAnalyzedResource` ("analyzedResource", no hyphen) --
   see families.route_matcher above.

**Gaps closed for real this pass** (previously listed under `gaps:`, now implemented,
not just narrowed):
- **UpdateAnalyzer/CreateAnalyzer/CreateServiceLinkedAnalyzer Configuration** (the
  `AnalyzerConfiguration` union) is now accepted, persisted (`Analyzer.Configuration
  json.RawMessage`), and echoed back opaquely -- gopherstack does not semantically
  interpret unused-access/internal-access analysis rules, but no longer silently drops
  client-supplied configuration on the floor either.
- **CreateAnalyzer/CreateServiceLinkedAnalyzer inline `archiveRules`**: now creates real
  archive rules (via the existing `CreateArchiveRule`, including its
  auto-archive-existing-findings side effect) instead of being ignored.
- **GetFindingV2/ListFindingsV2 findingDetails/findingType**: `findingType` is now
  always `"ExternalAccess"` and `GetFindingV2`'s `findingDetails` now returns a real
  one-element `[]types.FindingDetails`-shaped array (`externalAccessDetails`, built from
  the same Principal/Condition/Action/IsPublic fields `findingToJSON` already exposes).
  This is not a disguised partial stub: `InMemoryBackend.AddFinding` has no
  internal-access/unused-access modeling anywhere, so external-access is the only
  finding type this backend can honestly report, and it is now reported completely and
  correctly for that one type.
- **ListAccessPreviewFindings wire shape**: now builds the real
  `types.AccessPreviewFinding` shape (`accessPreviewFindingToJSON`) instead of reusing
  the incompatible v1 `Finding`/`FindingSummary` shape.

**Remaining gaps** (see `gaps:` above): `GetFindingRecommendation.recommendedSteps` and
`GetGeneratedPolicy...generatedPolicies` are both always `[]` -- both would require
modeling genuinely separate content-generation features (unused-permission-removal
recommendations; CloudTrail-activity-derived policy statement synthesis) with no
backing state anywhere in this service to derive real content from. Left as explicit
gaps rather than fabricated per parity-principles #1.

**gopherstack-kwht follow-up (this pass)**: re-audited both "always empty" gaps against
their *surrounding* fields rather than accepting the label at face value. Both were
still genuine gaps (no fabricated content added), but each had a real, separately
fixable bug next to it:
- `GetFindingRecommendation` was missing two **required** response members entirely
  (`resourceArn`, `startedAt`) plus the optional `completedAt`, and its
  `recommendationType` wire value (`"UNUSED_PERMISSION"`) didn't match the real
  `types.RecommendationType` enum's only value, `"UnusedPermissionRecommendation"`
  (SDK `types/enums.go:579`, v1.51.4) -- a real client would never recognize the type it
  received. Also: `GenerateFindingRecommendation` created a recommendation record for
  *any* finding ID, including nonexistent ones, with no existence check at all; it now
  resolves the real finding (capturing its `resourceArn`) and 404s
  (`ResourceNotFoundException`) like `GetFindingRecommendation` already did.
- `StartPolicyGeneration` accepted `cloudTrailDetails` (`types.CloudTrailDetails`) from
  the request and silently discarded it -- a client-supplied-and-dropped bug, not a
  missing-analysis-engine one. It's now stored and echoed back by `GetGeneratedPolicy`
  as `properties.cloudTrailProperties` (`types.CloudTrailProperties`,
  `types/types.go:2375`), matching the pattern already established for
  `Analyzer.Configuration`.
- Also fixed in passing: `PolicyGenerationStatusRunning` was declared as `"RUNNING"`,
  which doesn't match the real `types.JobStatus` enum's `"IN_PROGRESS"`
  (`types/enums.go:394`). Currently unassigned (StartPolicyGeneration completes
  synchronously), so not a live bug, but wrong if ever used.
- `sdk_module` pin was stale (`v1.48.0` recorded vs. `v1.51.4` actually pinned in
  `go.mod`); corrected. All wire claims in this file re-checked against v1.51.4 sources
  during this pass; no other drift found.

**Wire-shape trap for future auditors**: `Finding`/`FindingSummary` (used by
GetFinding/ListFindings/GetFindingV2/ListFindingsV2) serialize the resource under the
JSON key **"resource"**, not "resourceArn". "resourceArn" is only correct for the
unrelated `AnalyzedResource` type (used by GetAnalyzedResource/ListAnalyzedResources) --
do NOT conflate the two; they look similar but differ on the wire.
`types.AccessPreviewFinding` (ListAccessPreviewFindings) is a THIRD, still different
shape again (`id`/`changeType`, no `analyzerArn` member at all) -- do not conflate it
with `Finding`/`FindingSummary` either, despite gopherstack modeling all three from the
same underlying `*Finding` record.

**Confirmed NOT stubs** (would look suspicious on a grep-only pass, verified by reading):
`ValidatePolicy`, `CheckAccessNotGranted`, `CheckNoNewAccess`, `CheckNoPublicAccess`
(`policy_analysis.go`) do genuine IAM policy statement evaluation (glob-matching
Action/Resource/Principal, Allow/Deny semantics) -- not always-empty/always-PASS stubs.
`StartPolicyGeneration` completes synchronously to `SUCCEEDED` rather than modeling an
async `IN_PROGRESS` state machine; this is a deliberate simplification (not a
stuck-forever no-op) since `GetGeneratedPolicy`/`ListPolicyGenerations` immediately
reflect the terminal state and a polling client will see it complete on the first call.

**Persistence**: `Handler.Snapshot`/`Handler.Restore` delegate to
`InMemoryBackend.Snapshot`/`Restore` (persistence.go), which round-trips all
`store.Registry`-registered tables (analyzers, findings, analyzedResources,
policyGenerations, accessPreviews, findingRecommendations) plus the "dirty" archiveRules
table via an ephemeral DTO registry, plus the plain `tags` map. The new
`Analyzer.Configuration json.RawMessage` field round-trips for free through the
generic JSON-marshal-based `store.Table[Analyzer]` Snapshot/Restore -- no DTO or special
casing needed (verified via `TestAnalyzerConfiguration`-style round-trip in
persistence_test.go's existing analyzer coverage plus manual review of store.Table's
marshal path).

## gopherstack-6flj wrapper-key sweep (this pass, 2026-08-15)

**PICK.** `go run ./cmd/opcensus` at pickup showed the last tier-18 tier
(`memorydb`, `codedeploy`, `accessanalyzer`) with `memorydb` already
committed and `codedeploy` live-uncommitted (`git status` showed 9 modified
`services/codedeploy/*` files, a concurrent session's in-progress work per
`_WRAPPER_KEY_SWEEP_REMAINDER.md`'s own tail). Occupancy ruled `codedeploy`
out; `accessanalyzer` was the only free service left at that tier (18
L+D+G: 9 List, 0 Describe, 9 Get) -- no surface tie-break was needed since
only one candidate was free.

**PROTOCOL, ROUTER, SECOND CLIENT, EQUALFOLD.** restjson1 (confirmed via
`api_client.go` / `awsRestjson1_*` prefix throughout deserializers.go).
Of 201 `EqualFold` call sites in deserializers.go, ALL 201 match on
`errorCode` for exception dispatch -- zero body-field-key `EqualFold`
calls, so body decode is case-SENSITIVE (Go map-key switch over an
already-`encoding/json`-decoded `map[string]interface{}`). Router is a
path-segment matcher (`RouteMatcher` in handler.go), NOT the flat
`X-Amz-Target` style -- not structurally immune to the router-collision
class, but the existing `/tags/{ARN}` handling already guards against
swallowing other services' tag requests by checking for
`:access-analyzer:` in the ARN (pre-existing, re-verified, no new
collision found). No second SDK client import anywhere outside `_test.go`
files.

**PHANTOM OPS:** zero, both directions. `GetSupportedOperations`'s 39
`op*` constants exact-matched the SDK's 39 `api_op_*.go` files 1:1 (`diff`
after sorted extraction).

**SCRIPTED KEY EXTRACTION: both directions.** A paren-balance-aware Python
walker (gitignored scratch script, not committed) located each op's
`awsRestjson1_deserializeOpDocument*Output` / `serializeOpDocument*Input`
function by matching the signature's parens to balance before searching
for the body's opening brace -- hit the documented `interface{}`-in-signature
trap on the deserializer side (`func …Output(v **T, value interface{}) error {`)
and confirmed the naive first-`{` search breaks on it. Walked transitively
into every nested `awsRestjson1_(de)serializeDocument*` call for all 39
ops (`case "key":` for deserializers, `object.Key("key")` for
serializers), then cross-referenced the full key set against every
`json:"..."` tag in this service's non-test `.go` files. Top-level wrapper
keys came back clean everywhere except the one flagship bug below; the
false-negative candidates the diff surfaced (`accountID`/`region`/`tables`/
`version` in persistence.go; `Action`/`Condition`/`Effect`/`Principal`/
`Resource`/`Sid`/`Statement`/`Version` in policy_analysis.go) were both
confirmed non-wire: the former are `store.Table` snapshot-DTO fields
(persistence, not wire), the latter are IAM policy-*document* fields
parsed out of a `policyDocument` string value, not accessanalyzer's own
API surface.

**FLAGSHIP BUG: `GetFindingsStatistics` union wrapper-key mismatch.**
`types.FindingsStatistics` is a union keyed by wire name
(`awsRestjson1_deserializeDocumentFindingsStatistics`, deserializers.go
~L9169) with three members --
`externalAccessFindingsStatistics`/`internalAccessFindingsStatistics`/
`unusedAccessFindingsStatistics` -- selected purely by which JSON key is
present. This backend explicitly models four `AnalyzerType` values
(`ACCOUNT`/`ORGANIZATION`/`ACCOUNT_UNUSED_ACCESS`/`ORGANIZATION_UNUSED_ACCESS`,
models.go), but `handleGetFindingsStatistics` always emitted the
external-access key regardless of the target analyzer's actual `Type`. A
real client's typed union type-switch on an unused-access analyzer's
statistics would land on the wrong branch (`*types.
FindingsStatisticsMemberExternalAccessFindingsStatistics` instead of
`...MemberUnusedAccessFindingsStatistics`) -- correct byte count, wrong
Go type, same silent-wrong-data class this campaign exists to find, just
one level below the field-name layer most instances of this bug live at.
Fixed by looking up the target analyzer's `Type` and selecting the wire
key accordingly. `unusedAccessFindingsStatistics`'s
`TopAccounts`/`UnusedAccessTypeStatistics` members are left unset and
disclosed above -- no per-principal-account or unused-access-type
categorization exists in this backend's `Finding` model to derive them
from honestly.

**DISCARDED INPUTS (`grep '_ [A-Za-z]*FilterCriterion'` and manual read):
3 instances of the same defect, one degree worse each time.**
`ListFindings`' backend method took a `map[string]FilterCriterion`
parameter literally named `_` -- decoded from the wire's real `filter`
key, then discarded before reaching the filtering logic.
`ListFindingsV2`'s handler didn't even decode `filter` from the request
body; the backend method had no such parameter at all.
`ListAccessPreviewFindings` was the same as `ListFindingsV2`. All three
real client filters were pure no-ops: every finding for the analyzer/
access-preview came back regardless of the criteria sent. Fixed by adding
a shared `matchesFindingFilter` helper (findings.go) evaluating the `Eq`
operator against the finding attributes this backend tracks as direct
scalar fields (`status`/`resourceType`/`resource`/`id`), wired through all
three ops (`ListFindingsV2`'s backend signature gained a `filter`
parameter -- the only exported-interface change this pass, `go build
./...` re-run clean after). `Contains`/`Neq`/`Exists` and any filter key
not backed by a direct field are NOT evaluated (treated as satisfied, not
excluding) -- disclosed under `gaps:` rather than silently faked as full
filter-language support.

**RELATED BEHAVIORAL BUG FOUND WHILE BUILDING THE FILTER HELPER (same
root cause, not itself a wire-shape bug): `CreateArchiveRule`/
`ApplyArchiveRule` ignored their own archive rule's filter entirely.**
Real AWS's archive-rule auto-apply (on creation) and retroactive-apply
(`ApplyArchiveRule`) both archive only the ACTIVE findings matching the
rule's filter criteria -- that's the entire point of an archive rule.
Both ops here instead blanket-archived every active finding for the
analyzer, filter or no filter, rule-specific criteria or not. A real
caller creating a narrowly-scoped archive rule (e.g. one `resourceType`)
would have had every OTHER active finding wrongly archived too. Fixed
both using the same `matchesFindingFilter` helper: `CreateArchiveRule`
now matches its own `filter` parameter before archiving;
`ApplyArchiveRule` now looks up the NAMED rule (previously `ruleName` was
treated as optional and, even when supplied, was validated to exist but
never actually consulted for its filter) and matches against that rule's
stored `Filter`. Also fixed in passing: `RuleName` is a required
`ApplyArchiveRuleInput` member (`api_op_ApplyArchiveRule.go:37-40`);
previously accepted as optional, now empty -> `ValidationException`.

**TESTS:** all real-`aws-sdk-go-v2`-client round-trip tests (not raw-body
hand-decoded structs), added across
`handler_findings_test.go`/`access_preview_sdk_test.go`/
`handler_archive_rules_test.go`:
`TestGetFindingsStatistics_RealClient_UnusedAccessUnion` (type-asserts the
response union member, not just field values -- this is the only way to
observe the union-key bug through the real client),
`TestListFindings_RealClient_FilterByResourceType`,
`TestListFindingsV2_RealClient_FilterByResourceType`,
`TestListAccessPreviewFindings_RealClient_FilterByResourceType`,
`TestCreateArchiveRule_RealClient_OnlyArchivesMatchingFindings`,
`TestApplyArchiveRule_RealClient_OnlyArchivesMatchingFindings`. Every one
of the 6 fixes was hand-reverted individually (git-mutating commands
banned this session, including `git checkout --`; reverts were by hand-
edit back to the exact pre-fix code, since `matchesFindingFilter`/the
union-key `if` needed a compiling-but-wrong intermediate shape for some
reverts, e.g. `_ = rule` cleanup), the corresponding test re-run and
confirmed to fail with the exact predicted symptom (wrong union member
type; extra unfiltered finding in the result; wrongly-archived
non-matching finding), then restored and confirmed **byte-identical**
via `diff` against a saved `git diff` snapshot for each file before and
after the hand-revert/restore cycle.

**NOT reached this pass:** the `store.go`/`persistence.go` internal
locking/generic-Table implementation (unchanged from the 2026-08-10
audit's own `deferred:` note, still not re-litigated); `patch.go`-style
special-shape ops (none exist in this service); `GetFindingRecommendation.
recommendedSteps`/`GetGeneratedPolicy...generatedPolicies` (pre-existing,
disclosed, unrelated content-generation gaps, unchanged this pass).

**GATES:** `go build ./services/accessanalyzer/...` and full `go build
./...` (the `ListFindingsV2` interface signature change requires the full
build; `services/docdb/*` was mid-edit by a concurrent live sibling at
various points this session -- re-ran `go build ./...` after each
`git status` check and it was green both before this session's edits and
again at the end) both clean; `go vet ./services/accessanalyzer/...`
clean; `go test -race -count=1 ./services/accessanalyzer/...` and
`./pkgs/...` both green; `go fix -diff ./services/accessanalyzer/...`
initially flagged one manual loop that `slices.Contains` replaces (in the
new `matchesFindingFilter`) -- applied by hand, re-ran clean.
`golangci-lint run ./services/accessanalyzer/...` found 3
`golines`/`lll` line-length issues in new test code on first pass, fixed
by hand-wrapping (not `--fix`, per this campaign's
`fieldalignment -fix`-strips-`//nolint` hazard note -- these weren't
`fieldalignment` findings, but the same by-hand discipline was applied
regardless); final run: **0 issues**. Zero
`//nolint:cyclop/gocyclo/gocognit/funlen` present before or after
(grep-confirmed).

No subagents used (Read/Grep/Bash/Edit only, per this session's hard
constraint). No git-mutating command run at any point. `git status`
re-checked before every edit batch; only `services/accessanalyzer/*` and
`services/_WRAPPER_KEY_SWEEP_REMAINDER.md` touched by this session --
`services/docdb/*` (the concurrent sibling's files) was never read or
edited.

## 2026-08-30 anonymous-struct-decode sweep (gopherstack-4a8v)

`cmd/reqfieldscan` gained a fifth dispatch shape (handlers implementing
`service.JSONOpFunc` directly, decoding into anonymous inline structs, no
`WrapOp` anywhere) that made real findings newly visible in this service.
Dispatch coverage: 20/39 (51%), both the literal-decode-only and
WrapOp-resolved lines identical; no coverage-guard warning (51% clears the
50% threshold). The 19 unresolved ops (GetAnalyzer, ListAnalyzers,
DeleteAnalyzer, etc.) are legitimately outside this scanner's ground truth,
not a measurement failure: they're REST GET/DELETE handlers keyed by path
(`handleGetAnalyzer(path string)`, no `body []byte` parameter at all), a
structurally different, non-body-decoding dispatch shape this scan doesn't
claim to cover. Confirmed by reading `handleGetAnalyzer` directly.

4 fields flagged in `handler_policy_validation.go`, all hand-verified
against `accessanalyzer@v1.51.4`'s own `Input` structs:

- `CheckAccessNotGranted.policyType`, `CheckNoNewAccess.policyType`,
  `CheckNoPublicAccess.resourceType`: real bugs, all three "This member is
  required" in the SDK and none were validated. Fixed with a required-field
  check (see `ops:` notes above for what was and wasn't fixed -- the
  deeper identity-vs-resource-policy evaluation gap is left as a
  documented gap, not fabricated).
- `ValidatePolicy.nextToken`: real but left as an honest, documented gap
  (see its `ops:` note) rather than fixed -- implementing real pagination
  would require inventing a page size `maxResults` isn't even parsed for.

Tests: `TestCheckPolicyOps_RequiredFieldMissing` (3 new subtests,
`handler_policy_validation_test.go`), driven via raw HTTP
(`doRequest`) rather than the typed SDK client -- the real client's own
`validateOp*Input` rejects an empty policyType/resourceType client-side
before ever sending a request, so the typed client can't reach this bug at
all; only a non-SDK caller can. All hand-confirmed failing (200 instead of
400) against unmodified code before the fix. No existing test assertions
were weakened; 0 dropped.

Gates: `go build`, `go vet` (repo-wide), `go test -race -count=1`,
`golangci-lint run` — all clean (`./services/accessanalyzer/...`).

## 2026-08-31 directed sweep: request-key/silent-empty-default compound bug (gopherstack-uox6 territory), CLEAN

Regenerated the campaign's plural-heuristic candidate list against
`accessanalyzer@v1.51.4/serializers.go`: `action`, `archiveRule`, `region`,
`resource`. All four dismissed: `action`/`archiveRule` are response-output
map keys (`m["action"] = f.Action`, `keyArchiveRule` response wrapper), not
request reads; `region` is an internal persistence-DTO json tag, never on
the wire; `resource` is a `ListFindings`/`ListFindingsV2` `Filter` map key
matching `FindingSummary.Resource`'s real field name (confirmed against
`types.go`'s `Resource *string` and its JSON tag) -- the heuristic flagged
it only because `Filter` is `map[string]types.Criterion`, so the key never
appears as a literal in `serializers.go` at all.

Went beyond the heuristic: read every JSON-body/query-param decode struct in
`handler_findings.go`, `handler_access_previews.go`,
`handler_analyzed_resources.go`, `handler_generated_policies.go`,
`handler_archive_rules.go` against the pinned SDK's
`ListFindings`/`ListFindingsV2`/`ListAccessPreviewFindings`/
`ListAnalyzedResources`/`CreateAccessPreview`/`StartPolicyGeneration`/
`CreateArchiveRule` input structs and their
`awsRestjson1_serializeOpDocument*Input`/`*HttpBindings*Input` functions.
Every field name (`filter`, `sort.attributeName`/`sort.orderBy`,
`analyzerArn`, `resourceType`, `clientToken`, `ruleName`,
`filter[key].{contains,eq,exists,neq}`, `cloudTrailArn`/`allRegions`/
`regions`/`accessRole`/`startTime`/`endTime`/`trails`, `configurations`)
matched exactly.

One dead-but-harmless finding, not fixed: `handleListFindings`/
`handleListFindingsV2` both decode a top-level `Status string
json:"status"` field that the real `ListFindingsInput`/`ListFindingsV2Input`
do not have at all (confirmed: neither struct declares it, and neither
`serializeOpDocumentListFindingsInput` nor its V2 counterpart ever emits
`"status"`) -- a real client can never populate it, so it is permanently
`""`. This is NOT the compound bug: the empty-string case means "no
additional status narrowing," which is exactly correct, because status
filtering for a real client happens entirely through `Filter["status"]`
(already correctly read by `matchesFindingFilter`'s `case "status"`). Dead
code, zero observable effect on any real-client-driven call -- left alone
rather than removed, out of this pass's scope.

Also checked and correctly left as an open gap (not fabricated):
`GetGeneratedPolicy`'s `IncludeResourcePlaceholders`/
`IncludeServiceLevelTemplate` (both real, both unread anywhere in this
package) affect generated-policy *content* detail, not which records a list
operation returns -- a different axis (missing feature) from this
compound's record-filtering shape, so left unimplemented rather than
folded in here.

No code changes this pass -- service verdict is CLEAN on this specific axis.
Gates re-run to confirm no regression from the investigation: `go build`,
`go vet` (repo-wide), `go test -race -count=1`, `golangci-lint run` -- all
clean (`./services/accessanalyzer/...`), 0 diff.

## gopherstack-xyu4: full read of policy_analysis.go + access_previews.go (2026-09-06)

Prior passes (2026-08-30 gopherstack-4a8v, and the "Confirmed NOT stubs" note above)
established that `ValidatePolicy`/`CheckAccessNotGranted`/`CheckNoNewAccess`/
`CheckNoPublicAccess` are real evaluation, not always-PASS/always-empty stubs, but never
did a full line-by-line read of `policy_analysis.go` (687 lines) or `access_previews.go`
beyond the precondition/ghost-row checks. This pass did that read. Real AWS runs an
automated-reasoning engine behind these four ops; this mock obviously cannot, so the
question is honesty about the gap, not reasoning correctness.

**Per-op verdict**:

| Op | SDK return shape | What this backend computes | Verdict |
|---|---|---|---|
| `ValidatePolicy` | `ValidatePolicyOutput.Findings []types.ValidatePolicyFinding` (required) | Real, bounded structural/syntax validation: JSON-parseable, `Version` present/valid, `Effect` in {Allow,Deny}, exactly one of Action/NotAction, exactly one of Resource/NotResource (identity policies only), plus one narrow `PASS_ROLE_WITH_STAR` heuristic (wildcard Action AND wildcard Resource on an Allow). No IAM-grammar semantic reasoning beyond that (e.g. does not validate action names exist, ARN formats, or Condition operators). | **Documented approximation.** Never returns empty findings for a policy with real structural errors; the checks it does perform are correct and match `findingDetailMessages`'s disclosed coverage. |
| `CheckAccessNotGranted` | `CheckAccessNotGrantedOutput.Result types.CheckAccessNotGrantedResult` (`PASS`/`FAIL`, types/enums.go:180-197) | Real glob-based Allow/Deny statement evaluation against the caller-supplied `access` (Actions x Resources). **Bug found and fixed this pass**: NotAction/NotResource were parsed into `iamStatement` but silently ignored by `actionAllowed`/`resourceAllowed`, so a NotAction/NotResource-only Allow statement (which grants the *complement* of the listed set) was scored as granting nothing -- a confident, wrong `PASS` for exactly the broad-grant case this op exists to catch. Fixed (see policy_analysis.go, matchesAny/actionAllowed/resourceAllowed). Residual disclosed gap: invalid-JSON policyDocument silently parses to an empty policy (see below) and IDENTITY_POLICY vs RESOURCE_POLICY is not distinguished (pre-existing, gopherstack-4a8v). | **Real bounded check, with a real bug now fixed; residual gaps disclosed.** |
| `CheckNoNewAccess` | `CheckNoNewAccessOutput.Result types.CheckNoNewAccessResult` (`PASS`/`FAIL`, types/enums.go:199-216) | Same statement-grant machinery as `CheckAccessNotGranted`, diffed old vs. new. Same NotAction/NotResource bug on the *existing*-policy side, fixed this pass (existing grants via NotAction/NotResource are now visible to `policyGrants`). Residual, NOT fixed: the *new* statement's own NotAction/NotResource still can't be expanded into concrete (action,resource) pairs to diff -- doing so needs an action/resource universe this mock doesn't have (the actual reasoning problem). A new statement written with NotAction/NotResource is under-reported and may `PASS` when a real analyzer would `FAIL`. | **Real bounded check; disclosed residual gap on the unbounded (new-statement NotAction/NotResource) side.** |
| `CheckNoPublicAccess` | `CheckNoPublicAccessOutput.Result types.CheckNoPublicAccessResult` | Real check: any Allow statement whose Principal is `"*"` or `{"AWS":"*"}` (`isPublicPrincipal`) fails, regardless of Action/Resource -- correctly independent of the NotAction/NotResource bug above (it never calls `actionAllowed`/`resourceAllowed`). Does not evaluate Condition (e.g. an `aws:SourceIp`-restricted wildcard principal is still flagged public); this is the *safe* direction of error (over-flagging, not a false PASS) and resourceType is parsed but not used for resource-type-specific rules (disclosed pre-existing gap, gopherstack-4a8v). | **Real bounded check, conservative in the direction that matters (never a false-safe PASS from Condition).** |

**access_previews.go findings** (beyond the precondition/ghost-row checks already done):
`AccessPreviewFinding` fields ARE populated from real backend state (id, status,
resourceType, resource, resourceOwnerAccount, createdAt, plus action/principal/condition/
isPublic when set) -- not emitted empty. `GetAccessPreview`'s status never leaves its
initial value (`AccessPreviewStatusCompleted`, set once in `CreateAccessPreview` and never
transitioned through `AccessPreviewStatusCreating`/`Failed`), but this is disclosed, not
silent: `AccessPreview.Configurations` godoc (models.go:161-174) and the
`CreateAccessPreview`/`ListAccessPreviewFindings` PARITY notes above both state plainly
that finding generation reuses the analyzer's existing findings rather than deriving
findings from `Configurations`' semantic content (i.e. this mock does not actually
simulate "what would change" -- it echoes the analyzer's current finding set). Every
finding is reported `changeType: "NEW"` for the same disclosed reason (not diffed against
a prior finding set). Since the backend is synchronous, immediate `COMPLETED` is honest --
it is not modeling an unfinished analysis as done, it genuinely has nothing left to do.
**Verdict: documented approximation, not an undisclosed stub** -- this is the "reuses
existing findings" design already on record, re-confirmed by reading every line of the
file.

**Error extractions** (raw, `awk "/deserializeOpError<Op>\(/,/^}/" deserializers.go | grep
-oE '"[A-Za-z0-9]+"'`, accessanalyzer@v1.51.4):
```
ValidatePolicy:         UnknownError, AccessDeniedException, InternalServerException, ThrottlingException, ValidationException
CheckAccessNotGranted:  UnknownError, AccessDeniedException, InternalServerException, InvalidParameterException, ThrottlingException, UnprocessableEntityException, ValidationException
CheckNoNewAccess:       UnknownError, AccessDeniedException, InternalServerException, InvalidParameterException, ThrottlingException, UnprocessableEntityException, ValidationException
CheckNoPublicAccess:    UnknownError, AccessDeniedException, InternalServerException, InvalidParameterException, ThrottlingException, UnprocessableEntityException, ValidationException
```
This mock only ever wires `ValidationException` (`ErrValidation`, errors.go) for these four
ops; `AccessDeniedException`/`InternalServerException`/`ThrottlingException` are structurally
unimplementable here (no auth/rate-limiting simulation) and unimplemented the same way
across the whole repo, not specific to this audit. `InvalidParameterException`/
`UnprocessableEntityException` on the three Check* ops ARE reachable in real AWS for a
syntactically-invalid `policyDocument` and are NOT wired here -- see the invalid-JSON gap
below.

**Follow-up gap, disclosed not fixed (report as a separate issue)**: `parsePolicy`
(policy_analysis.go) silently discards `json.Unmarshal` errors and returns a zero-value
`iamPolicy{}` (no statements). `ValidatePolicy` is the only one of the four ops that checks
JSON-parseability itself (`INVALID_POLICY_SYNTAX`, via its own `json.Unmarshal` in the
error path before calling `parsePolicy`); `CheckAccessNotGranted`/`CheckNoNewAccess`/
`CheckNoPublicAccess` call `parsePolicy` directly and never check for a parse failure, so a
malformed (non-JSON) `policyDocument` silently becomes an empty policy and all three report
`PASS`/no-public-access instead of an error -- the same confident-wrong-answer shape the
NotAction/NotResource bug was, now on garbage input instead of well-formed input. Not fixed
this pass: `CheckAccessNotGranted`/`CheckNoNewAccess`/`CheckNoPublicAccess`/`policyGrants`/
`stmtGrants` are all currently infallible (`PolicyCheckResult`, no error return); wiring a
parse error through to an HTTP-level `InvalidParameterException`/`UnprocessableEntityException`
touches all three call sites plus their handlers in `handler_policy_validation.go` and is a
big enough surface to deserve its own reviewed change rather than folding into this pass's
NotAction/NotResource fix.

**Files changed this pass**: `policy_analysis.go` (NotAction/NotResource complement
matching in `actionAllowed`/`resourceAllowed`, plus a new `matchesAny` helper);
`policy_analysis_test.go` (4 new regression subtests, proven to fail against pre-fix code
by revert-and-run, see commit history / session record). `access_previews.go` and
`handler_access_previews.go`: read in full, no changes -- both already correctly disclosed
(models.go:161-174, handler_access_previews.go:45-52/202-211).

Gates: `go test -race ./services/accessanalyzer/...` and
`golangci-lint run services/accessanalyzer/...` both clean, 0 issues.
