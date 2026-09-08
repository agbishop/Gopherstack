---
service: cloudfront
sdk_module: aws-sdk-go-v2/service/cloudfront@v1.67.4
sibling_sdk_modules: [aws-sdk-go-v2/service/cloudfrontkeyvaluestore@v1.15.4]  # KeyValueStore data-plane ops (GetKey/PutKey/DeleteKey/ListKeys/UpdateKeys/DescribeKeyValueStore) now live in services/cloudfrontkeyvaluestore (gopherstack-4ara, 2026-08-13) -- see that service's own PARITY.md
last_audit_commit:                                # unknown: gopherstack-o31x route-table audit pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-08-14  # gopherstack-7185: response shapes of Create/Delete/Modify ops
                              # swept (the class prior passes only checked for List/Describe).
                              # 2 bugs found (DeleteVpcOrigin empty envelope, UpdateDomainAssociation
                              # wrong output key). See DeleteVpcOrigin/UpdateDomainAssociation op rows.
# XML DECLARATION doubling fixed 2026-08-29 (wrapper-key-sweep pass): xmlResp
# handed bodies that already began with `<?xml version="1.0" encoding="UTF-8"?>`
# (every body builder in this package embeds one) to echo's c.XMLBlob, which
# prepends its own copy of the same declaration -- every single XML response
# this service ever emitted, success AND error path alike, carried two
# back-to-back declarations. A declaration is legal only as the very first
# construct in a document, so strict parsers reject the whole body; confirmed
# with botocore ("Unable to parse response") against ListDistributions. The
# aws-sdk-go-v2 client's own smithy-go XML decoder is lenient about it and
# does NOT fail, which is why no existing test (including ones driving the
# real Go SDK client) ever caught this -- only a raw-response-bytes assertion
# does. Fixed by making xmlResp write bytes directly instead of through
# XMLBlob, so the body's own declaration is the one and only source; the sole
# body that never carried its own declaration (GetDistributionConfig's
# RawConfig passthrough -- RawConfig is stored from either the raw client
# request body or xml.Marshal output, neither of which ever emits one) now
# gets one prepended explicitly at that call site, matching the convention
# GetStreamingDistributionConfig's RawConfig passthrough already used. See
# handler_xml_declaration_test.go.
# ERROR path verified 2026-08-29 (wrapper-key-sweep pass): extracted every
# op's deserializeOpError<Op> switch (cloudfront@v1.67.4 deserializers.go,
# 167 ops N-of-N) against errCodeMapping/notFoundCode (handler_dispatch.go)
# and every backend call site. Systemic finding: ErrConnectionFunctionNotFound,
# ErrConnectionGroupNotFound, ErrDistributionTenantNotFound, ErrTrustStoreNotFound,
# ErrVpcOriginNotFound each carried a fabricated per-resource "NoSuchXxx" code
# that does not exist anywhere in the pinned SDK -- every op in each of those
# 5 families (connection function/group, distribution tenant, trust store,
# VPC origin -- ~20 ops) actually models the shared EntityNotFound code
# instead (already the convention this file used for KVS/resource-policy).
# All 5 sentinels + the errCodeMapping/notFoundCode literals fixed. Also
# fixed 8 more per-op mismatches where a shared sentinel's code didn't match
# the specific op's own modeled set: AssociateDistributionWebACL/
# DisassociateDistributionWebACL and TagResource/UntagResource/
# ListTagsForResource each reused ErrNotFound's NoSuchDistribution instead of
# their own EntityNotFound/NoSuchResource; CreateDistributionTenant/
# UpdateDistributionTenant's domain-conflict case used a fabricated
# "DomainConflictException" (renamed sentinel to ErrCNAMEAlreadyExists, the
# code both ops actually model, shared with CreateDistribution's alias-
# collision case); UpdateDomainAssociation's own domain-conflict and unknown-
# target-distribution paths used the same wrong codes; CreateKeyGroup/
# UpdateKeyGroup's unknown-item-public-key case and UpdateTrustStore's
# rename-collision case each used a code their op doesn't model, corrected
# to the modeled ValidationException-equivalent. See error_sentinel_fixes_test.go
# (real-SDK errors.As assertions, each confirmed failing pre-fix). 10
# pre-existing tests across 6 test files asserted the old wrong codes/status
# as correct; corrected alongside the fix.
# FILTER/PAGINATION PARAMETER audit 2026-08-29 (continuation of the eks/cleanrooms pass,
# commit 9f7b9d67e): read every List op's Input shape against api_op_List*.go/types.go
# (cloudfront@v1.67.4) and checked whether the handler reads AND applies each declared
# filter/sort/status/pagination member. 5 real "declared, never read" bugs fixed:
# ListFunctions.Stage (query-bound), ListConnectionFunctions.Stage (XML-body-bound --
# the sibling op families disagree on binding location, confirmed per-op from
# serializers.go rather than assumed from ListFunctions), ListConnectionGroups
# .AssociationFilter.AnycastIpListId (body-bound nested filter), ListKeyValueStores
# .Status (query-bound; KVS.Status is always "READY" here since provisioning is
# synchronous, so the filter is still correctly implemented as an equality check --
# not a structural gap, just never exercised by any seeded non-READY value),
# ListDistributionTenants.AssociationFilter (body-bound nested filter on
# ConnectionGroupId/DistributionId) -- this last handler didn't read its request body
# AT ALL before the fix, so Marker/MaxItems were silently unhonoured alongside the
# filter. All 5 verified against the real aws-sdk-go-v2 client, confirmed failing
# pre-fix, fixed, and re-verified; see list_filter_params_test.go and the pagination
# cases appended to list_pagination_ignored_test.go.
#   Pagination does NOT go through one shared helper here, unlike eks/cleanrooms:
# paginateByMarkerID (query-string Marker/MaxItems) and the new paginateByMarkerValue
# (XML-body Marker/MaxItems, for ListConnectionGroups/ListConnectionFunctions/
# ListDistributionTenants) are both used, but ~20 further List ops (ListCachePolicies,
# ListOriginRequestPolicies, ListResponseHeadersPolicies, ListOriginAccessControls,
# ListCloudFrontOriginAccessIdentities, ListFieldLevelEncryptionConfigs,
# ListFieldLevelEncryptionProfiles, ListPublicKeys, ListKeyGroups,
# ListRealtimeLogConfigs, ListVpcOrigins, ListContinuousDeploymentPolicies,
# ListStreamingDistributions, ListTrustStores, ListConflictingAliases,
# ListDomainConflicts, and the whole ListDistributionsBy* family of 11) hardcode
# MaxItems in the response and never truncate or emit a marker/NextMarker at all --
# confirmed by reading each handler, NOT fixed this pass (see gaps below). The
# ListDistributionsBy* family additionally has heterogeneous real output shapes
# (DistributionIdList vs DistributionList vs DistributionIdOwnerList depending on
# the specific op) that the current shared marshalDistributionList collapses to one
# shape -- a wire-shape question distinct from parameter-honouring, flagged but not
# investigated further; needs its own dedicated pass reading each op's own Output
# struct and deserializer, not a mechanical pagination patch.
# BOTH GAPS ABOVE CLOSED 2026-08-30 (gopherstack-lkng) -- see "List pagination +
# ListDistributionsBy* shape fix" section near the end of this file.
overall: A            # gopherstack-o31x: first FULL route diff of all 167 real cloudfront
                       # control-plane ops (method+path) against cloudfront@v1.67.4
                       # serializers.go, not just the ops other work happened to touch.
                       # Fixed the 3 known bare-vs-/config Update routes plus 21 further
                       # mismatches this diff surfaced: the entire ListDistributionsBy*
                       # family (12 ops) used a hyphenated path with no real-SDK counterpart;
                       # CreateDistributionWithTags/CreateStreamingDistributionWithTags read
                       # their WithTags flag from a "Resource" query key a real client never
                       # sets (real flag is a bare "?WithTags"); TagResource/UntagResource
                       # were disambiguated by HTTP method (POST/DELETE) when both are really
                       # POST, differing only by an "Operation=Tag|Untag" query value, so
                       # every real UntagResource call landed on TagResource instead; the
                       # monitoring-subscription trio used singular "distribution/" instead of
                       # the real plural "distributions/"; GetManagedCertificateDetails was
                       # nested under distribution-tenant instead of its own top-level
                       # "managed-certificate/{Identifier}"; DisassociateDistributionTenantWebACL
                       # had no route at all; and ListConnectionFunctions/ListConnectionGroups/
                       # GetDistributionTenantByDomain/GetConnectionGroupByRoutingEndpoint were
                       # each swapped with their List/Get sibling. See gopherstack-o31x and the
                       # "Full route-table audit" note below for the complete method and
                       # methodology. go build/vet/test -race/golangci-lint all pass clean.
                       # (gopherstack-a9t managed policies, gopherstack-na4 InUse guards,
                       # gopherstack-mzx CallerReference AlreadyExists), and found three
                       # NEW real wire bugs via field-diff against aws-sdk-go-v2 that were
                       # not previously flagged despite these families being marked "ok":
                       # (1) CachePolicy/OriginRequestPolicy/ResponseHeadersPolicy whitelist
                       # Items lists were silently dropped on parse (CachePolicy only) and on
                       # every read response (all three) -- see "Wire-shape fixes" note below;
                       # (2) UpdateOriginRequestPolicy was routed to require a "/config" URL
                       # suffix that no real SDK client ever sends (real wire is a bare-ID PUT),
                       # so every real UpdateOriginRequestPolicy call 404'd against this
                       # emulator; (3) CreateDistribution/CreateStreamingDistribution treated
                       # CallerReference reuse as unconditionally idempotent, when real AWS
                       # returns *AlreadyExists on ANY reuse regardless of content (stricter
                       # than the previously-filed gopherstack-mzx description, which assumed
                       # a content-comparison rule -- verified against the live API docs).
                       # go build/vet/test -race/golangci-lint all pass clean this pass.
ops:
  CreateDistribution: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED this pass: CallerReference reuse now ALWAYS returns DistributionAlreadyExists (was unconditionally idempotent); real API docs state this happens regardless of DistributionConfig content -- verified against the live CreateDistribution reference page, not just the SDK doc comment"}
  CreateDistributionWithTags: {wire: ok, errors: ok, state: fixed, persist: ok, note: "inherits the CreateDistribution CallerReference fix. FIXED 2026-08-13 (gopherstack-o31x): routing bug. Real request sends a bare \"?WithTags\" query flag with no value (serializers.go: awsRestxml_serializeOpCreateDistributionWithTags's SplitURI on \".../distribution?WithTags\"), never \"?Resource=WithTags\" -- gopherstack read the WithTags signal from a \"Resource\" query value a real client never sends, so every real CreateDistributionWithTags call silently landed on plain CreateDistribution instead (tags dropped, no error). Fixed by a new cfResourceParam helper (handler.go) that checks for the bare \"WithTags\" query key before falling back to \"Resource\". Same bug, same fix, for CreateStreamingDistributionWithTags (see its op row). Verified against the real aws-sdk-go-v2 client (TestCreateDistributionWithTags_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand."}
  GetDistribution: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDistributionConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDistribution: {wire: ok, errors: ok, state: fixed, persist: ok, note: "If-Match/ETag enforced; validateQuantities added. FIXED this pass (gopherstack-k3fi): the InProgress status UpdateDistribution sets now really transitions back to Deployed on its own, via a b.work.After-scheduled async hop (distributions.go's scheduleDistributionDeployed) -- the same pkgs/worker idiom services/mgn/exportimport.go and services/outposts's order lifecycle use. The scheduled hop is re-armed on Restore (rearmPendingDistributionDeploysLocked) so a distribution restored mid-transition still reaches Deployed instead of sticking InProgress forever, unlike a bare timer that would only survive a process restart, not a Snapshot/Restore round trip. Scoped to Distribution only -- see deferred note below for the other 5 resource kinds with their own status semantics."}
  DeleteDistribution: {wire: ok, errors: ok, state: ok, persist: ok, note: "If-Match enforced; DistributionNotDisabled enforced"}
  ListDistributions: {wire: ok, errors: ok, state: ok, persist: ok}
  CopyDistribution: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "FIXED this pass: did not track/enforce CallerReference uniqueness at all (distributionCallerRefs was never populated by CopyDistribution); now returns DistributionAlreadyExists on reuse, matching the real CopyDistribution error list"}
  CreateInvalidation: {wire: ok, errors: ok, state: ok, persist: ok, note: "validateQuantities added for Paths; background reconciler transitions InProgress->Completed"}
  GetInvalidation: {wire: ok, errors: ok, state: ok, persist: ok}
  ListInvalidations: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCachePolicy: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass: request-parsing tags for whitelisted Headers/Cookies/QueryStrings used Headers>Header/Cookies>Cookie/QueryStrings>QueryString, which matches no real CloudFront wire path -- real is Headers>Items>Name (verified against the live CreateCachePolicy/UpdateCachePolicy request syntax); every whitelist/allExcept request silently lost its listed names on unmarshal. Also now returns CachePolicyAlreadyExists; validateQuantities added"}
  UpdateCachePolicy: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "same parse fix as Create; managed policies (Managed-CachingOptimized etc.) now return IllegalUpdate (400) instead of being silently rewritten; If-Match enforced; validateQuantities added"}
  DeleteCachePolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "CachePolicyInUse guard via distribution config token index (prior pass); managed policies now return IllegalDelete (400) instead of being silently removed (this pass)"}
  GetCachePolicy / GetCachePolicyConfig / ListCachePolicies: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass: every response previously omitted the Headers/Cookies/QueryStrings Items lists entirely (only a bare HeaderBehavior/CookieBehavior/QueryStringBehavior, no Quantity, no Items) and GetCachePolicyConfig omitted ParametersInCacheKeyAndForwardedToOrigin altogether -- a real client could never discover which names a policy actually whitelists. Managed-vs-custom Type=managed|custom filter added (gopherstack-a9t, closed) and List summaries now carry the correct <Type> element"}
  CreateOriginRequestPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "now returns OriginRequestPolicyAlreadyExists; validateQuantities added"}
  UpdateOriginRequestPolicy: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass, CLIENT-BREAKING: routing required a PUT to .../origin-request-policy/{id}/config, but the real UpdateOriginRequestPolicy wire is a bare-ID PUT (.../origin-request-policy/{id}, no /config suffix, verified against the live API reference) -- every real SDK client's UpdateOriginRequestPolicy call 404'd with NoSuchOperation against this emulator. Managed policies now return IllegalUpdate (400)"}
  DeleteOriginRequestPolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "OriginRequestPolicyInUse guard (prior pass); managed policies now return IllegalDelete (400) (this pass)"}
  GetOriginRequestPolicy / GetOriginRequestPolicyConfig / ListOriginRequestPolicies: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass: same Items-list omission as CachePolicy -- orpResponseXML emitted only a bare Quantity, and GetOriginRequestPolicyConfig omitted HeadersConfig/CookiesConfig/QueryStringsConfig entirely. Type=managed|custom filter added"}
  CreateResponseHeadersPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "now returns ResponseHeadersPolicyAlreadyExists; validateQuantities added"}
  UpdateResponseHeadersPolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "same as Create; managed policies now return IllegalUpdate (400)"}
  DeleteResponseHeadersPolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "ResponseHeadersPolicyInUse guard (prior pass); managed policies now return IllegalDelete (400) (this pass)"}
  GetResponseHeadersPolicy / GetResponseHeadersPolicyConfig / ListResponseHeadersPolicies: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass: CorsConfig's four list fields (AccessControlAllowOrigins/Headers/Methods, AccessControlExposeHeaders) and SecurityHeadersConfig's ContentTypeOptions/ContentSecurityPolicy were completely absent from every response even though the request parser already captured them. GetResponseHeadersPolicyConfig omitted the whole config body. Type=managed|custom filter added. STILL SIMPLIFIED (see items_still_open): XSSProtection is a single string field, not the real 4-field Override/Protection/ModeBlock/ReportUri struct, and STS/FrameOptions/ReferrerPolicy/ContentSecurityPolicy have no per-header Override flag modeled (only ContentTypeOptions does) -- not restructured this pass"}
  CreateOriginAccessControl: {wire: ok, errors: ok, state: ok, persist: ok, note: "now returns OriginAccessControlAlreadyExists; validateQuantities added"}
  UpdateOriginAccessControl: {wire: ok, errors: ok, state: ok, persist: ok, note: "same as above"}
  DeleteOriginAccessControl: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED this pass (gopherstack-na4): OriginAccessControlInUse guard added via the same token-index pattern as CachePolicy/Function; verified against a distribution whose Origin.OriginAccessControlId references it"}
  GetOriginAccessControl / GetOriginAccessControlConfig / ListOriginAccessControls: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCloudFrontOriginAccessIdentity: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED this pass: CallerReference reuse with an identical Comment is idempotent (correct, per the real CloudFrontOriginAccessIdentityConfig doc), but reuse with a DIFFERENT Comment previously still returned the existing OAI silently instead of CloudFrontOriginAccessIdentityAlreadyExists; validateQuantities added (harmless no-op for this shape)"}
  UpdateCloudFrontOriginAccessIdentity: {wire: ok, errors: ok, state: ok, persist: ok, note: "If-Match enforced"}
  DeleteCloudFrontOriginAccessIdentity: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED this pass (gopherstack-na4): CloudFrontOriginAccessIdentityInUse guard added, matching on the real S3OriginConfig.OriginAccessIdentity wire value \"origin-access-identity/cloudfront/{id}\" (not the bare ID) -- If-Match still enforced"}
  GetCloudFrontOriginAccessIdentity / Config / List: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateFunction: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass: response was missing required FunctionMetadata.FunctionARN/CreatedTime/LastModifiedTime; now returns FunctionAlreadyExists (was DistributionAlreadyExists); validateQuantities added"}
  UpdateFunction: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same wire fix; If-Match enforced; validateQuantities added"}
  PublishFunction: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same wire fix; If-Match enforced; LastModifiedTime now bumped"}
  DeleteFunction: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW: FunctionInUse guard (keyed by FunctionARN, not name)"}
  GetFunction / DescribeFunction / ListFunctions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "share the same FunctionMetadata fix"}
  TestFunction: {wire: fixed, errors: fixed, state: n/a, persist: n/a, note: "CORRECTED 2026-08-13 (gopherstack-3izo): the handler never read the request body at all -- it confirmed the function existed via GetFunction, then returned a hardcoded TestResult with empty FunctionExecutionLogs/FunctionErrorMessage/FunctionOutput regardless of the supplied EventObject (required, base64 body-XML, api_op_TestFunction.go:50, serializers.go:11847) or the function's own code, and never checked If-Match at all despite it being a second required member (api_op_TestFunction.go:56) -- every real client's test call got a successful-looking empty result no matter what it sent. Real execution is out of reach: gopherstack vendors no JavaScript engine (no goja/otto/v8 in go.mod), and the one existing precedent for this exact problem -- appsync's EvaluateCode (services/appsync/jseval.go) -- only covers a narrow return-expression DSL used by AppSync resolver mapping templates (~5 fixed patterns: object literals, context member paths, a handful of util.* helpers), not general-purpose ES5.1 code with loops/variables/string methods/regex that real CloudFront Functions (URL rewrites, header/cookie manipulation, redirects) actually use; a 'faithful subset' evaluator broad enough to be useful would silently misexecute on anything outside its subset and produce a FunctionOutput that looks real but isn't -- worse than an empty one. Lambda's approach (services/lambda/containers.go: real Docker containers running actual AWS runtime images) is genuine execution but is Lambda's own zip/bootstrap/runtime-API protocol, not applicable to CloudFront Functions' edge JS model. Chose the honest option: read and validate the request for real (If-Match checked against the function's current ETag -> InvalidIfMatchVersion if missing/mismatched, matching this op's own declared error, not the PreconditionFailed siblings use; EventObject required, base64-decoded, and validated as well-formed JSON -> InvalidArgument otherwise), then report the real declared TestFunctionFailed error (HTTP 500, 'the CloudFront function failed' per the API reference) for a well-formed request gopherstack cannot execute, instead of fabricating FunctionOutput/logs. One pre-existing test (TestCloudFrontFunctionCRUD/test_function) asserted the canned empty-success TestResult as correct with no If-Match header and no EventObject at all; corrected to expect TestFunctionFailed for a well-formed request. New TestTestFunction covers the full validation matrix (missing/wrong If-Match, missing/non-base64/non-JSON EventObject, unknown function, and the TestFunctionFailed structural-gap response) and fails against the pre-fix handler by reverting by hand."}
  TagResource / UntagResource / ListTagsForResource: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-13 (gopherstack-o31x): routing bug. Real TagResource and UntagResource are BOTH POST /2020-05-31/tagging, disambiguated only by an \"Operation=Tag\"/\"Operation=Untag\" query value (serializers.go: awsRestxml_serializeOp{Tag,Untag}Resource's SplitURI) -- UntagResource is never DELETE. gopherstack routed POST unconditionally to TagResource and DELETE to UntagResource, so every real UntagResource call (POST) landed on the TagResource handler instead, which then 400'd MalformedXML trying to unmarshal an UntagResource body (root TagKeys) as Tags. Fixed by threading the \"Operation\" query value through parseCFPath (new opParam parameter) and switching on it for POST /tagging; a bare POST with no recognized Operation value still defaults to TagResource for backward compatibility with hand-built requests. ListTagsForResource (GET) was unaffected. Verified against the real aws-sdk-go-v2 client (TestTagUntagResource_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand. gopherstack-r80d (required-OUTPUT-member sweep): ListTagsForResourceOutput.Tags is the ONLY required output member in this service's entire 167-op SDK surface (every other op's Output has zero 'This member is required.' fields at struct depth 0) -- not a protocol-wide trait (Route 53, also REST-XML, has 108 required output fields across 58 ops), just how this particular Smithy model was authored. handleListTagsForResource always builds a non-nil Tags element (even when the tag set is empty), so the sole required member is correctly populated. Service is fully settled for this bug class. Re-verified 2026-08-28 (independent re-check after the issue's closure reason was found undocumented): re-ran `go run ./cmd/requiredoutputfields`, still exactly 1 field/1 op (ListTagsForResourceOutput.Tags) across all 167 ops; handler unchanged since, still correctly populated; go build/vet/test -race/golangci-lint all clean. 0 new findings, no regression."}
  AssociateAlias: {wire: ok, errors: ok, state: ok, persist: ok, families: cross-service}
  AssociateDistributionTenantWebACL: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23 (gopherstack-jf8z): response was a bare c.NoContent(200) -- no ETag header, no body at all -- so AssociateDistributionTenantWebACLOutput's ETag/Id/WebACLArn (all *string, api_op_AssociateDistributionTenantWebACL.go) decoded nil for every real client call regardless of backend state. Same bug class as the non-tenant sibling's 2026-08-23 fix (AssociateDistributionWebACL row above), fixed the same way: ETag on the response header, <Id>/<WebACLArn> in the body (root name irrelevant to decode -- awsRestxml_deserializeOpDocumentAssociateDistributionTenantWebACLOutput matches these as direct children of whatever root is sent). This was missed by the 2026-08-13 pass below, whose own commit message asserted this op was \"checked and correct\" -- it was not; only the request-side shape had been fixed, the response side was never driven through a real client that inspected the returned fields (the existing TestAssociateDistributionTenantWebACL_RealClient only asserted err==nil and checked state via a raw HTTP GET, never the SDK response object). Verified against the real aws-sdk-go-v2 client (TestAssociateDistributionTenantWebACL_RealClient_ETag, handler_sdk_route_fixes_test.go) and confirmed to fail against the pre-fix shape by reverting by hand (ETag=<nil> Id=<nil> WebACLArn=<nil> before, all populated after). 2026-08-13 (gopherstack-4ara): request struct root was WebACLAssociation with a WebACLId field; the real root is AssociateDistributionTenantWebACLRequest with a WebACLArn field (an ARN, not an ID; serializers.go: awsRestxml_serializeOpDocumentAssociateDistributionTenantWebACLInput, cloudfront@v1.67.4). Unlike the PutResourcePolicy class of this bug, the handler's xml.Unmarshal error WAS checked (not discarded), so the actual failure mode was every real client's request 400ing MalformedXML outright, not a silent zero-value wipe that returns 200 -- confirmed against the real client both before and after the fix (TestAssociateDistributionTenantWebACL_RealClient, fails against the pre-fix shape by reverting by hand). Also fixed TestAssociateDistributionTenantWebACL, a pre-existing test whose hand-typed request body encoded the exact same invented WebACLAssociation/WebACLId shape the pre-fix handler expected, so it had been passing against broken code indefinitely."}
  AssociateDistributionWebACL: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23: response never set the ETag header and returned an empty 200 body, so AssociateDistributionWebACLOutput's ETag/Id/WebACLArn (all *string, api_op_AssociateDistributionWebACL.go) decoded nil for every real client call regardless of backend state -- distinct from the 2026-08-13 request-shape fix below, which never checked the response side. Fixed by returning ETag on the response header and an <Id>/<WebACLArn> body (root name irrelevant to decode -- confirmed via awsRestxml_deserializeOpDocumentAssociateDistributionWebACLOutput, which matches these as direct children of whatever root is sent, not a nested wrapper). Verified against the real aws-sdk-go-v2 client (TestAssociateDisassociateDistributionWebACL_RealClient_ETag, handler_sdk_route_fixes_test.go) and confirmed to fail against the pre-fix shape by reverting by hand (ETag=<nil> Id=<nil> WebACLArn=<nil> before, all populated after). 2026-08-13 (gopherstack-bhhx): request struct root was WebACLAssociation with a WebACLId field (the same webACLAssociationXML shared type AssociateDistributionTenantWebACL used before its own gopherstack-4ara fix); the real root is AssociateDistributionWebACLRequest with a WebACLArn field (an ARN, not an ID; serializers.go:255, awsRestxml_serializeOpDocumentAssociateDistributionWebACLInput, cloudfront@v1.67.4) -- a DIFFERENT real root from the tenant sibling's AssociateDistributionTenantWebACLRequest despite an identical field shape, so this needed its own dedicated request type (associateDistributionWebACLRequestXML) rather than reusing either the old shared type or the tenant's dedicated one. Same failure-mode class as the tenant fix: the handler's xml.Unmarshal error WAS checked (not discarded), so real clients got a clean 400 MalformedXML rather than a silent zero-value wipe. Surveyed every other shared XML request/response type in this service for the same shared-type-different-real-root risk (invalidationBatchXML used by CreateInvalidation and CreateInvalidationForDistributionTenant, tagXML/tagsXML used by 7+ ops) -- all confirmed safe: the real SDK's own types.InvalidationBatch/types.Tags/types.Tag are themselves canonical shared types reused identically across those ops (types/types.go:6492,6521), unlike the WebACLAssociation/WebACLId shape which never existed on any real op's wire at all. Verified against the real aws-sdk-go-v2 client (TestAssociateDistributionWebACL in handler_distributions_lifecycle_test.go, driven with the real AssociateDistributionWebACLRequest/WebACLArn body, plus a negative case asserting the old WebACLAssociation/WebACLId body now 400s MalformedXML) and confirmed to fail against the pre-fix shape by reverting by hand. Also fixed TestAssociateDistributionWebACL and TestDisassociateWebACL, two pre-existing tests whose hand-typed request bodies encoded the exact same invented WebACLAssociation/WebACLId shape the pre-fix handler expected, so they had been passing against broken code indefinitely."}
  DisassociateDistributionWebACL: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23: this op (distinct from the tenant-scoped DisassociateDistributionTenantWebACL) had never appeared in this file's ops/families before -- routing was already correct, but the response never set the ETag header, so DisassociateDistributionWebACLOutput's ETag (*string, api_op_DisassociateDistributionWebACL.go) decoded nil for every real client call even though Id (from the existing Distribution body) already worked. Fixed by setting ETag on the response header, same as every other If-Match-bearing op in this file. Verified against the real aws-sdk-go-v2 client (TestAssociateDisassociateDistributionWebACL_RealClient_ETag, handler_sdk_route_fixes_test.go) and confirmed to fail against the pre-fix shape by reverting by hand (ETag=<nil> before, populated after)."}
  UpdateDomainAssociation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-14 (gopherstack-7185): response shape bug. The real UpdateDomainAssociationOutput carries a single ResourceId field (whichever of DistributionId/DistributionTenantId was the target -- the union collapses on output) plus ETag as a response header, NOT the input-shaped DistributionId/DistributionTenantId pair (api_op_UpdateDomainAssociation.go:60-68, deserializers.go:23927-23938,23974) -- an input/output key split of the same class the omics CompleteMultipartReadSetUpload/RunCache bugs were, so checking the request side alone would have confirmed the wrong answer. The handler echoed back separate DistributionId/DistributionTenantId elements (neither matching the real ResourceId tag) and never set an ETag header at all, so a real client's ResourceId/ETag were always empty even though the reassignment genuinely happened. Fixed: DomainAssociationResult gained an ETag field (sourced from the target's own ETag) and a ResourceID() accessor collapsing the two IDs; the handler now emits <ResourceId> and sets the ETag header. Verified against the real aws-sdk-go-v2 client (TestUpdateDomainAssociation_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand."}
  ListDistributionTenantsByCustomization: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "FIXED 2026-08-12 (gopherstack-difi): TWO wire bugs, the second more severe than the first. (1) WebACLArn was read from the query string via c.Request().URL.Query(); cloudfront@v1.67.4 serializers.go's HTTP-bindings serializer for this op returns nil (zero HTTP-bound fields), so WebACLArn/CertificateArn/Marker/MaxItems all serialize into the XML body -- the query-string read was always empty against a real client. (2) The route table matched GET /distribution-tenants/by-customization, but the real SDK sends POST /distribution-tenants-by-customization (one hyphenated segment, no slash) -- confirmed by probing the unfixed handler with a real-shaped request, which 404'd NoSuchOperation. Fixed both: request fields now parsed from the XML body (root ListDistributionTenantsByCustomizationRequest), and the route corrected to POST + the hyphenated path. CertificateArn filtering and Marker/MaxItems pagination, previously entirely unimplemented, are now real: CertificateArn matches TenantCertificateArn (the tenant's deterministic CloudFront-managed certificate ARN -- customer-supplied ACM certs via Customizations.Certificate.Arn are not modeled anywhere in this service's Create/UpdateDistributionTenant, so that half of real AWS's certificate model stays out of scope); Marker/MaxItems page through the ID-sorted tenant list the same way ListDistributions already does, with NextMarker returned as a sibling of DistributionTenantList per the real deserializer."}
  PutResourcePolicy: {wire: fixed, errors: fixed, state: fixed, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-nfka): TWO stacked wire bugs. (1) The request struct tagged its policy field xml:\"Policy\" and its root xml:\"ResourcePolicy\"; the real request is root PutResourcePolicyRequest containing PolicyDocument (api_op_PutResourcePolicy.go:27-41, serializers.go:11515-11527) -- since encoding/xml's Unmarshal errors when the root element name doesn't match an XMLName tag, EVERY real client's body failed to parse at all (err was discarded), silently zeroing ResourceArn too, not just the policy text. (2) Routing matched method (GET/POST/DELETE) on a single shared \"resource-policy\" path, but the real SDK POSTs to three distinct RPC-style paths -- /put-resource-policy, /get-resource-policy, /delete-resource-policy -- confirmed by probing the unfixed handler with real-shaped requests, all three 404'd NoSuchOperation. Fixed both: root/field names corrected, ResourceArn parsed from the body (never a query string, matching serializeOpHttpBindings*Input which emits no HTTP bindings for any of the three ops), and routing split into three POST-only suffix matches. Also fixed the not-found error code: ErrResourcePolicyNotFound emitted the invented NoSuchResourcePolicy; the real declared code (deserializeOpError{Get,Put,Delete}ResourcePolicy) is EntityNotFound."}
  GetResourcePolicy: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "Twin of the PutResourcePolicy bug: response element was xml:\"Policy\" instead of PolicyDocument, and ResourceArn was never echoed at all. Both request-side bugs (root-name mismatch discarding ResourceArn, routing) also applied -- see PutResourcePolicy row. Response now emits PolicyDocument and ResourceArn per GetResourcePolicyOutput."}
  DeleteResourcePolicy: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "Same routing + body-vs-query-string bugs as Put/Get; DeleteResourcePolicyInput.ResourceArn now read from the body (root DeleteResourcePolicyRequest)."}
  CreateVpcOrigin: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-13 (gopherstack-nfka): request parsing captured only VpcOriginEndpointConfig.Name and Tags; the other three required members -- Arn (the ARN of the VPC interface endpoint or ALB this origin actually routes to, types/types.go:6989-6992), HTTPPort, HTTPSPort, and OriginProtocolPolicy -- were dropped entirely and never reached backend state. Now parsed, validated (InvalidArgument if any required member is empty/non-positive, matching the op's declared error set), stored, and echoed back inside VpcOriginEndpointConfig in the response (which is a sibling of the resource's own top-level Arn, not nested inside it -- confirmed via CreateVpcOriginOutput's httpPayload-bound VpcOrigin decode)."}
  UpdateVpcOrigin: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "CORRECTED 2026-08-13 (gopherstack-ob1g): the 2026-08-13 (gopherstack-nfka) fix above stopped one field short. UpdateVpcOriginInput's real root element IS VpcOriginEndpointConfig itself (serializers.go: awsRestxml_serializeOpUpdateVpcOrigin's payloadRoot.Local) -- there is no wrapping UpdateVpcOriginRequest element the way Create has one. The struct fixed that pass still used XMLName=\"UpdateVpcOriginRequest\" and nested fields one level under a VpcOriginEndpointConfig>Name-style path, so xml.Unmarshal still errored on the whole body for every real client and the error was discarded (_ = xml.Unmarshal(...)), silently no-opping every real UpdateVpcOrigin call end to end -- this survived because the existing tests hand-crafted bodies matching the same wrong root. Root and field nesting corrected; the unmarshal error is now handled (400 MalformedXML) instead of discarded. Verified against the real aws-sdk-go-v2 client (TestUpdateVpcOrigin_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand."}
  DeleteVpcOrigin: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-14 (gopherstack-7185): empty-envelope bug, the class this issue was opened to find. Unlike every other Delete op in this service (all with a genuinely empty DeleteXOutput -- verified across all 23 real DeleteXOutput structs in the pinned SDK), DeleteVpcOriginOutput uniquely carries ETag (header) and VpcOrigin (body, the just-deleted resource) -- api_op_DeleteVpcOrigin.go:44-53. The handler answered with a bare 204 No Content, so a real client's out.VpcOrigin/out.ETag were always nil even though the delete genuinely happened. Fixed to return 200 with the deleted VpcOrigin body (reusing vpcOriginResponseXML) and the ETag header. Verified against the real aws-sdk-go-v2 client (TestDeleteVpcOrigin_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand."}
  CreateRealtimeLogConfig: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-13 (gopherstack-nfka): THREE stacked wire bugs. (1) Request struct root was xml:\"RealtimeLogConfig\"; the real root is CreateRealtimeLogConfigRequest (api_op_CreateRealtimeLogConfig.go, serializers.go:2489-2609) -- same root-name-mismatch class of bug as PutResourcePolicy, so Name/Fields/SamplingRate were ALSO silently dropped for every real client, not just EndPoints. (2) EndPoints -- the required Kinesis destination (api_op_CreateRealtimeLogConfig.go:37-43) -- was never declared as a struct field at all; now parsed (list wrapped in <member>, matching serializers.go's awsRestxml_serializeDocumentEndPointList) and required (InvalidArgument if empty). (3) The response nested ARN/Name/etc directly under the root; CreateRealtimeLogConfigOutput is NOT httpPayload-bound (unlike VpcOrigin/Distribution) so the real deserializer looks for a child element literally named <RealtimeLogConfig> wrapping the fields (deserializers.go: awsRestxml_deserializeOpDocumentCreateRealtimeLogConfigOutput) -- the old flat response left output.RealtimeLogConfig nil for a real client even once (1) and (2) were fixed. All three verified against the real aws-sdk-go-v2 client via a round-trip test (TestRealtimeLogConfigCRUD_RealClient), and each fails against the pre-fix shape individually (confirmed by temporarily reverting each in turn)."}
  GetRealtimeLogConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Same response double-nesting bug as Create (fixed). ALSO a routing bug: this op is a POST to /2020-05-31/get-realtime-log-config carrying ARN or Name in the body (api_op_GetRealtimeLogConfig.go:33-42), not a GET to /realtime-log-config/{id}; the old route table 404'd NoSuchOperation for every real client. Now POSTs to the correct path and resolves by ARN or Name (preferring Name when both given, per the op's doc comment)."}
  UpdateRealtimeLogConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Same three bugs as Create (missing EndPoints, response nesting) plus the same routing bug as Get: real wire is PUT to the base /2020-05-31/realtime-log-config path with ARN/Name identifying the target in the body (api_op_UpdateRealtimeLogConfig.go:43-67), not a PUT to /realtime-log-config/{id}."}
  DeleteRealtimeLogConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Same routing bug as Get: real wire is POST to /2020-05-31/delete-realtime-log-config with ARN/Name in the body (api_op_DeleteRealtimeLogConfig.go), not a DELETE to /realtime-log-config/{id}."}
  UpdateTrustStore: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "FIXED 2026-08-13 (gopherstack-ob1g): TWO stacked wire bugs, same class as UpdateVpcOrigin above. (1) UpdateTrustStoreInput's real root is CaCertificatesBundleSource, containing CaCertificatesBundleS3Location>Bucket/Key/Region as its only children (serializers.go: awsRestxml_serializeOpUpdateTrustStore's payloadRoot.Local; types.go: CaCertificatesBundleSourceMemberCaCertificatesBundleS3Location) -- UpdateTrustStoreInput has NO Name or Comment member at all, so real AWS can never change either through this operation. The struct here used root TrustStoreConfig with Name/Comment/CertificateAuthorityCertificatesBundle fields, none of which exist on the real wire; xml.Unmarshal errored on the whole body for every real client and the error was discarded, silently no-opping the CA bundle update while ALSO exposing a Name/Comment-update capability real AWS doesn't have. (2) The unmarshal error was discarded (_ = xml.Unmarshal(...)); now handled (400 MalformedXML). Fix: request struct rebuilt to the real CaCertificatesBundleSource>CaCertificatesBundleS3Location shape (Region accepted on the wire but not persisted -- see deferred note), handler now always passes empty name/comment to the backend (never overwritten, matching real AWS), and the old TrustStoreConfig>CertificateAuthorityCertificatesBundle shape is still accepted for backward compatibility. Verified against the real aws-sdk-go-v2 client (TestUpdateTrustStore_RealClient, which reads back the applied bundle via a raw follow-up GET since the real TrustStore output shape has no field for the CA bundle at all) and confirmed to fail against the pre-fix shape by reverting by hand."}
  UpdateDistributionWithStagingConfig: {wire: fixed, errors: fixed, state: ok, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-ob1g): routing bug found while hardening this handler's discarded xml.Unmarshal error. Real wire is PUT /2020-05-31/distribution/{Id}/promote-staging-config with StagingDistributionId as a QUERY parameter, never a body field (serializers.go: awsRestxml_serializeOpUpdateDistributionWithStagingConfig's SplitURI and awsRestxml_serializeOpHttpBindingsUpdateDistributionWithStagingConfigInput's SetQuery call). The route table matched a bare \"/staging\" suffix instead, so every real client's PUT 404'd as NoSuchOperation. Since real clients never send a body, the (now-fixed) discarded xml.Unmarshal error itself was latent rather than an active wipe for real traffic -- the route was the blocking bug. Fixed both: route corrected to the real path, and the unmarshal error is now handled instead of discarded, guarding the pre-existing body-based fallback path some callers may still use for backward compatibility."}
  ListDomainConflicts: {wire: fixed, errors: fixed, state: ok, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-ob1g): routing bug found while hardening this handler's discarded xml.Unmarshal error. Real path is /2020-05-31/domain-conflicts (plural; serializers.go: awsRestxml_serializeOpListDomainConflicts's SplitURI); the route table matched the singular \"domain-conflict\", so every real client's POST 404'd as NoSuchOperation. Root/field names (ListDomainConflictsRequest>Domain) were already correct. Fixed both: route corrected to the plural path, and the unmarshal error is now handled instead of discarded. CORRECTED 2026-08-13 (gopherstack-3izo): that pass's 'Root/field names were already correct' verification only checked Domain -- it missed that ListDomainConflictsInput has a SECOND independently-required member, DomainControlValidationResource (a types.DistributionResourceId identifying the distribution or distribution tenant whose certificate validates control of the domain; api_op_ListDomainConflicts.go:73-77), which the request struct dropped entirely. Real AWS scopes the conflict check to that resource (excludes it from its own conflict list, since it legitimately holds the domain's cert); gopherstack ignored the scope and returned every conflict for the domain globally, including the resource itself when it was the one claiming the domain -- wrong, not merely incomplete. Fixed: DomainControlValidationResource now parsed (nested DistributionId/DistributionTenantId, exactly one required -> InvalidArgument otherwise), both required members validated (missing Domain or missing DomainControlValidationResource -> InvalidArgument), the referenced resource's existence checked (EntityNotFound if neither a real distribution nor tenant, matching this op's own declared error switch, not the per-resource-type NoSuchDistribution/NoSuchDistributionTenant codes other ops use), and findDomainConflicts extended to exclude that resource from the results. Two pre-existing tests (TestListDomainConflicts_RealConflicts, TestListDomainConflicts_TableDriven) never sent DomainControlValidationResource at all (one even used a nonexistent-on-the-real-wire ?Domain= query fallback) and so encoded the global-scope bug as correct; both corrected to send real bodies and now also cover the self-exclusion scoping and the new validation errors. All new/changed cases fail against the pre-fix handler by reverting by hand."}
  UpdatePublicKey: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-13 (gopherstack-o31x, filed by gopherstack-ob1g): real UpdatePublicKey PUTs to /2020-05-31/public-key/{Id}/config (serializers.go: awsRestxml_serializeOpUpdatePublicKey's SplitURI), not the bare /public-key/{Id} path -- every real client call 404'd. parseCFResourcePath's public-key call site (handler_paths.go: parseCFPublicKeyRealtimePath) had updateOp and updateConfigOp backwards (bound to the bare path, left the /config-suffixed PUT unmatched). Fixed by swapping which argument carries the real op. Existing tests asserting the wrong bare-ID path were updated to the real /config path, not preserved -- a test asserting a 404-producing route is negative value. Verified against the real aws-sdk-go-v2 client (TestUpdatePublicKey_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand."}
  UpdateFieldLevelEncryptionConfig: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-13 (gopherstack-o31x, filed by gopherstack-ob1g): same bare-vs-/config bug as UpdatePublicKey. Real path is /2020-05-31/field-level-encryption/{Id}/config (serializers.go SplitURI). Fixed the same way (parseCFFieldLevelEncryptionPath's field-level-encryption call site); existing tests updated to the real path. Verified against the real aws-sdk-go-v2 client (TestUpdateFieldLevelEncryptionConfig_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand. ALSO FIXED (gopherstack-kpk5, see Root cause C below): the CallerReference rename-collision check was removed from UpdateFieldLevelEncryption (field_level_encryption.go) since real AWS's declared error set for this op has no FieldLevelEncryptionConfigAlreadyExists at all (Create-only), so gopherstack was rejecting requests real AWS accepts."}
  UpdateFieldLevelEncryptionProfile: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-13 (gopherstack-o31x, filed by gopherstack-ob1g): same bare-vs-/config bug as UpdatePublicKey. Real path is /2020-05-31/field-level-encryption-profile/{Id}/config (serializers.go SplitURI). Fixed the same way (parseCFFieldLevelEncryptionPath's field-level-encryption-profile call site); existing tests updated to the real path. Verified against the real aws-sdk-go-v2 client (TestUpdateFieldLevelEncryptionProfile_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand."}
families:
  list_distributions_by: {status: fixed, note: "FIXED 2026-08-13 (gopherstack-o31x): all 12 ListDistributionsBy* ops (AnycastIpListId, CachePolicyId, ConnectionFunction, ConnectionMode, KeyGroup, OriginRequestPolicyId, OwnedResource, RealtimeLogConfig, ResponseHeadersPolicyId, TrustStore, VpcOriginId, WebACLId) were routed on a hyphenated \"distributions/by-x-id/{id}\" path with no real-SDK counterpart at all -- every real client call 404'd NoSuchOperation. Real paths are a single camelCase segment with no hyphens, e.g. \"/2020-05-31/distributionsByCachePolicyId/{CachePolicyId}\" (serializers.go SplitURI, verified per-op individually, cloudfront@v1.67.4). Beyond the path shape, several ops also had the wrong ID SOURCE: ByConnectionFunction and ByTrustStore carry their identifier as a query value with no URI label at all (ConnectionFunctionIdentifier/TrustStoreIdentifier), not a path segment; ByConnectionMode and ByOwnedResource carry theirs as a URI label, not a query value (gopherstack previously had this backwards for all four); ByRealtimeLogConfig carries its ARN/Name in the XML body (POST, root ListDistributionsByRealtimeLogConfigRequest), not a query value. Fixed by rewriting parseCFDistributionsByPath (handler_paths.go) to the real per-op path shapes and dispatchStubsDistributionListBy (handler_dispatch.go) to read each op's identifier from its real source; deleted the now-fully-dead hyphenated-path fallback code in parseCFMiscPathSimple/parseCFMiscPathByDistribution that duplicated the wrong shape. Verified against the real aws-sdk-go-v2 client for ByConnectionMode (field-level round-trip, TestListDistributionsByConnectionMode_RealClient) and ByRealtimeLogConfig (TestListDistributionsByRealtimeLogConfig_RealClient); the other 10 are covered by TestExtractOperation_SDKRouteTable's exhaustive method+path diff against every real op (see 'Full route-table audit' note below) but not individually round-tripped through a real client due to this pass's time budget."}
  monitoring_subscription: {status: fixed, note: "FIXED 2026-08-13 (gopherstack-o31x): CreateMonitoringSubscription/GetMonitoringSubscription/DeleteMonitoringSubscription used the singular \"distribution/{Id}/monitoring-subscription\" path; the real path is PLURAL \"distributions/{DistributionId}/monitoring-subscription\" (serializers.go SplitURI, cloudfront@v1.67.4) -- unlike every other distribution sub-path in this service, which is singular. The singular-prefix guard in parseCFDistributionExtPath meant the plural path never even reached the trio's routing logic, so every real call 404'd. Fixed by splitting the trio into its own parseCFMonitoringSubscriptionPath (handler_paths.go) keyed on the plural prefix, and fixing extractMonitoringDistID (handler_monitoring.go) to trim the plural prefix too. Verified against the real aws-sdk-go-v2 client (TestMonitoringSubscription_RealClient, full Create/Get/Delete round trip) and confirmed to fail against the pre-fix shape by reverting by hand."}
  managed_certificate_details: {status: fixed, note: "FIXED 2026-08-13 (gopherstack-o31x): GetManagedCertificateDetails was routed as \"distribution-tenant/{Id}/managed-certificate-details\"; the real path is its own top-level \"/2020-05-31/managed-certificate/{Identifier}\" (serializers.go: awsRestxml_serializeOpGetManagedCertificateDetails's SplitURI), not nested under distribution-tenant at all -- every real client call 404'd. Fixed with a new parseCFManagedCertificatePath (handler_paths.go) and the matching dispatch-layer ID-extraction prefix (handler_dispatch.go); the two duplicate wrong-shape handlers in parseCFDistributionTenantExtOps and parseCFMiscPathByDistribution were removed. Verified against the real aws-sdk-go-v2 client (TestGetManagedCertificateDetails_RealClient) and confirmed to fail against the pre-fix shape by reverting by hand."}
  connection_group_function_swaps: {status: fixed, note: "FIXED 2026-08-13 (gopherstack-o31x): three swapped/wrong-shape routes in the connection-group and connection-function families. (1) GetConnectionGroupByRoutingEndpoint is really the bare GET \"connection-group\" (RoutingEndpoint as a query value); ListConnectionGroups is really POST to the plural \"connection-groups\"; gopherstack had these backwards (bare GET matched List, and a fictional \"connection-group-by-routing-endpoint\" literal path that no real client sends matched GetByRoutingEndpoint). (2) Same swap for GetDistributionTenantByDomain (bare GET \"distribution-tenant\", Domain as a \"?domain=\" query value) vs ListDistributionTenants (POST plural \"distribution-tenants\") -- the bare GET was routed to List instead. (3) ListConnectionFunctions is really POST to the plural \"connection-functions\"; gopherstack matched GET on the bare singular \"connection-function\", which no real client sends for List. All three confirmed by reading serializers.go's SplitURI per op (cloudfront@v1.67.4) and verified against the real aws-sdk-go-v2 client (TestGetConnectionGroupByRoutingEndpoint_RealClient, TestGetDistributionTenantByDomain_RealClient, TestListConnectionFunctions_RealClient); the GetConnectionGroupByRoutingEndpoint and GetDistributionTenantByDomain fixes were each confirmed to fail against the pre-fix shape by reverting by hand."}
  ListConnectionGroups: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-4ara): response wrapped items as <ConnectionGroupList><Items><ConnectionGroupSummary>...(plus a Quantity element); the real ListConnectionGroupsOutput has no Items/Quantity wrapper at all -- it is a direct <ConnectionGroups> element holding repeated <ConnectionGroupSummary> children (deserializers.go: awsRestxml_deserializeOpDocumentListConnectionGroupsOutput/awsRestxml_deserializeDocumentConnectionGroupSummaryList, cloudfront@v1.67.4). smithyxml's decoder only recognizes a direct <ConnectionGroups> child by name and silently skips anything else (including the old <Items> wrapper), so a real client always decoded an empty ConnectionGroups slice regardless of what gopherstack had stored -- worse than the 404 this op gave before gopherstack-o31x's routing fix made it reachable. Fixed by renaming the wrapper element and dropping the fabricated Quantity field the real output doesn't have. Verified against the real aws-sdk-go-v2 client (TestGetConnectionGroupByRoutingEndpoint_RealClient's List assertion, extended this pass to require the created group actually appears in the decoded list -- the SDK cannot populate a list from a wrong wrapper no matter what the raw XML holds, so this is the only test that can catch the bug); confirmed to fail against the pre-fix shape by reverting by hand."}
  ListConnectionFunctions: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-4ara): same bug class as ListConnectionGroups above -- response wrapped items as <ConnectionFunctionList><Items><ConnectionFunctionSummary>...(plus Quantity); real ListConnectionFunctionsOutput is a direct <ConnectionFunctions> element with no Items/Quantity wrapper (deserializers.go: awsRestxml_deserializeOpDocumentListConnectionFunctionsOutput, cloudfront@v1.67.4), so a real client always decoded an empty list. Fixed the same way. Verified against the real aws-sdk-go-v2 client (TestListConnectionFunctions_RealClient, extended this pass to require the created function actually appears in the decoded list); confirmed to fail against the pre-fix shape by reverting by hand."}
  disassociate_distribution_tenant_web_acl: {status: fixed, note: "FIXED 2026-08-23 (gopherstack-jf8z): response never set the ETag header, so DisassociateDistributionTenantWebACLOutput's ETag (*string, api_op_DisassociateDistributionTenantWebACL.go) decoded nil for every real client call even though the body Id (via the existing distributionTenantXML) already worked. handleDisassociateDistributionTenantWebACL already fetched the tenant record before calling the backend but never echoed its ETag onto the response, unlike every sibling If-Match-bearing tenant op in this file (Create/Get/GetByDomain/Update all do). Fixed by setting the header from the same tenant record already in hand. Verified against the real aws-sdk-go-v2 client (extended TestDisassociateDistributionTenantWebACL_RealClient, handler_sdk_route_fixes_test.go, to assert ETag/Id on the SDK response object -- the pre-existing test only asserted err==nil) and confirmed to fail against the pre-fix shape by reverting by hand (ETag=<nil> before, populated after). 2026-08-13 (gopherstack-o31x): DisassociateDistributionTenantWebACL had no route at all -- only the Associate variant was wired in parseCFDistributionCorePath, even though the handler function and dispatch case already existed and were correctly implemented (handleDisassociateDistributionTenantWebACL, handler_dispatch.go's opDisassociateDistributionTenantWebACL case), unreachable purely for lack of a route match. Fixed by adding the \"/disassociate-web-acl\" suffix case alongside the existing \"/associate-web-acl\" one. Verified against the real aws-sdk-go-v2 client (TestDisassociateDistributionTenantWebACL_RealClient, which deliberately does not round-trip through Associate first -- see the AssociateDistributionTenantWebACL gap above)."}
  distribution_tenants_connection_groups: {status: ok, note: "CreateDistributionTenant/UpdateDistributionTenant now run validateQuantities; If-Match enforced on update/delete; audited, no new findings beyond the Quantity gap"}
  field_level_encryption: {status: ok, note: "Create/Update for config + profile now run validateQuantities and return the correct *AlreadyExists code (FieldLevelEncryptionConfigAlreadyExists / FieldLevelEncryptionProfileAlreadyExists) instead of DistributionAlreadyExists; FLEProfileInUse guard on profile delete pre-existed and is correct"}
  public_keys_key_groups: {status: ok, note: "CreatePublicKey/CreateKeyGroup/UpdateKeyGroup return PublicKeyAlreadyExists/KeyGroupAlreadyExists instead of DistributionAlreadyExists; PublicKeyInUse guard on public-key delete pre-existed and is correct; FIXED this pass (gopherstack-na4): DeleteKeyGroup now returns ResourceInUse (matching the real DeleteKeyGroup error list -- there is no dedicated KeyGroupInUse type) when the key group is referenced by a distribution's TrustedKeyGroups"}
  realtime_log_configs: {status: ok, note: "CreateRealtimeLogConfig returns RealtimeLogConfigAlreadyExists instead of DistributionAlreadyExists. See the CreateRealtimeLogConfig/GetRealtimeLogConfig/UpdateRealtimeLogConfig/DeleteRealtimeLogConfig op rows for the 2026-08-13 (gopherstack-nfka) wire and routing fixes -- this family note previously implied these ops were clean when they were not (missed by the 2026-07-23 audit)."}
  key_value_stores: {status: ok, note: "control-plane Create/Update run validateQuantities (no-op, shape has no Quantity/Items pairs). RESOLVED 2026-08-13 (gopherstack-4ara): the data-plane GetKey/PutKey/DeleteKey/ListKeys/UpdateKeys handlers previously living here were routed under this Handler's /2020-05-31/ RouteMatcher, which the real cloudfrontkeyvaluestore.Client never sends a request through -- structurally unreachable (see the removed 'gaps' entry below, and the Notes section's protocol paragraph). Removed from this service (handler_key_value_store.go, handler.go's op consts, handler_dispatch.go, handler_paths.go's parseCFKVSDataPlanePath) and reimplemented with correct routing/wire-shape in the new services/cloudfrontkeyvaluestore, wired via cli.go's wireCloudFrontKeyValueStore directly to this backend's keyValueStoreData/keyValueDataETags (the underlying state was always real; only the HTTP layer was wrong). This service's own control-plane CRUD (Create/Get/List/Delete/UpdateKeyValueStore) is unaffected and stays here. Persistence side-effect: keyValueStoreData/keyValueDataETags -- previously NOT in backendSnapshot at all -- are now persisted (cloudfrontSnapshotVersion bumped 1->2), and KeyValueStore gained a CreatedTime field (needed by the sibling service's DescribeKeyValueStore)."}
  vpc_origins: {status: ok, note: "Create/Update run validateQuantities (no-op for this shape). See the CreateVpcOrigin/UpdateVpcOrigin op rows for the 2026-08-13 (gopherstack-nfka) fix -- Arn/HTTPPort/HTTPSPort/OriginProtocolPolicy were previously dropped entirely, missed by the 2026-07-23 audit."}
  continuous_deployment_policy: {status: ok, note: "Create/Update run validateQuantities; If-Match already enforced"}
  invalidations_realtime_status: {status: ok, note: "background reconciler goroutine (runInvalidationReconciler) has a clean stopCh lifecycle via Close(); no leak"}
  monitoring_subscriptions_public_resource_policy_connection_groups: {status: fixed, note: "audited via handler_new_ops.go/handler_batch2.go dispatch; no Quantity/AlreadyExists-code issues found in these shapes. CORRECTION: this note previously claimed resource-policy was clean, but the 2026-07-23 audit missed that PutResourcePolicy's request never parsed at all against a real client (root/field name mismatch) and all three resource-policy ops were mis-routed (see PutResourcePolicy/GetResourcePolicy/DeleteResourcePolicy op rows, gopherstack-nfka, fixed 2026-08-13)."}
  managed_policies: {status: ok, note: "NEW this pass (gopherstack-a9t): 7 managed cache policies, 8 managed origin request policies, and 5 managed response headers policies seeded at backend construction/Reset/Restore with their real, permanent, verified-against-live-AWS-docs IDs and configs (see managed_policies.go's doc comment for the exact verification method and the deliberately-omitted Amplify-internal policies). Managed=true policies reject Update/Delete with IllegalUpdate/IllegalDelete (400); List* honors the real Type=managed|custom query filter and each summary carries the correct <Type> element"}
  streaming_distributions: {status: ok, note: "FIXED this pass: CreateStreamingDistribution treated non-empty CallerReference reuse as unconditionally idempotent; real AWS returns StreamingDistributionAlreadyExists on any reuse regardless of content (verified against the live CreateStreamingDistribution API reference, same rule as CreateDistribution). FIXED 2026-08-13 (gopherstack-o31x): CreateStreamingDistributionWithTags had the exact same WithTags-flag routing bug as CreateDistributionWithTags (real bare \"?WithTags\" query flag misread as \"Resource=WithTags\") -- see that op row for the fix. Verified via TestCreateStreamingDistributionWithTags_RealClient, confirmed to fail pre-fix by reverting by hand."}
gaps:
  # RESOLVED 2026-08-13 (gopherstack-4ara): the 5 CloudFront KeyValueStore data-plane ops
  # (GetKey/PutKey/DeleteKey/ListKeys/UpdateKeys), found structurally unreachable here by
  # gopherstack-o31x (see the "Full route-table audit" note below for how it was found),
  # are now implemented with correct routing/wire-shape in services/cloudfrontkeyvaluestore
  # -- see that service's own PARITY.md and the key_value_stores family note above.
  # gopherstack-o31x closed the previous pass's one open gap plus 21 further routing
  # mismatches the full 167-op diff surfaced beyond it -- see the FIXED op rows above
  # (CreateDistributionWithTags, CreateStreamingDistributionWithTags, TagResource,
  # UntagResource, UpdatePublicKey, UpdateFieldLevelEncryptionConfig,
  # UpdateFieldLevelEncryptionProfile, the whole ListDistributionsBy* family,
  # CreateMonitoringSubscription/GetMonitoringSubscription/DeleteMonitoringSubscription,
  # GetManagedCertificateDetails, DisassociateDistributionTenantWebACL,
  # GetDistributionTenantByDomain, GetConnectionGroupByRoutingEndpoint,
  # ListConnectionFunctions, ListConnectionGroups) and the "Full route-table audit" note
  # below for the complete list and methodology.
  # All three gaps filed by the pass before that are closed:
  #  - gopherstack-a9t (managed policies + Type filter): closed, see managed_policies family above.
  #  - gopherstack-na4 (OAI/OAC/KeyGroup delete InUse guards): closed, see the three
  #    "FIXED this pass (gopherstack-na4)" op rows above (DeleteOriginAccessControl,
  #    DeleteCloudFrontOriginAccessIdentity, DeleteKeyGroup via public_keys_key_groups family).
  #  - gopherstack-mzx (CallerReference AlreadyExists): closed, but the actual real-AWS rule
  #    is STRICTER than originally filed -- CreateDistribution/CreateStreamingDistribution
  #    always conflict on CallerReference reuse (content-independent), not just when content
  #    differs. CreateOAI genuinely IS content-comparison idempotent (the filed gap's
  #    assumption was correct for OAI specifically) and was fixed for the differing-content
  #    case. CopyDistribution didn't enforce CallerReference uniqueness at all and was also
  #    fixed. See the CreateDistribution/CopyDistribution/CreateStreamingDistribution/
  #    CreateCloudFrontOriginAccessIdentity op rows above for the exact behavior each has now.
deferred:
  - "Distribution status InProgress->Deployed transition timer: FIXED this pass (gopherstack-k3fi) for Distribution specifically -- see UpdateDistribution's op row above. The other 5 resource kinds with their own InProgress/Deployed-shaped status semantics (DistributionTenant, StreamingDistribution, ConnectionGroup/ConnectionFunction, AnycastIPList, TrustStore) still persist InProgress indefinitely; still deferred, now for a narrower, more honest reason -- extending the same worker.Group timer to each is straightforward but out of this pass's scope, not blocked on anything."
  - "Full per-op audit of DistributionConfig nested shape correctness (Origins/OriginGroups/CacheBehaviors/ViewerCertificate/Restrictions field-by-field) beyond the Quantity/Items validation and the pre-existing minimal-parse (RawConfig) model. This pass verified the specific sub-fields needed for the InUse-guard fixes (S3OriginConfig.OriginAccessIdentity path format, Origin.OriginAccessControlId, TrustedKeyGroups.Items) are correct, but a full field-by-field audit of the rest of DistributionConfig's ~60 nested types was not attempted -- RawConfig storage design predates this pass and was not restructured."
  - "ResponseHeadersPolicySecurityHeadersConfig is a flattened simplification of the real 5-sub-struct shape: XSSProtection is stored/emitted as a single string (matches only the real ReportUri sub-field) instead of the real ResponseHeadersPolicyXSSProtection{Override, Protection, ModeBlock, ReportUri} struct, and only ContentTypeOptions has a per-header Override flag modeled (STS/FrameOptions/ReferrerPolicy/ContentSecurityPolicy hardcode Override=false in every response, which happens to match every seeded managed policy's real Override:No default but is not read from request input for those four). Restructuring RHPSecurityHeaders to the full real shape is a breaking model change (cascades to persistence JSON tags and every existing test that constructs one) out of proportion to fix alongside this pass's other work; the CORS list fields and ContentTypeOptions/ContentSecurityPolicy value (the parts client code actually round-trips today) were fixed."
  - "2026-08-29 filter/pagination audit: ~20 List ops (see the header note above for the full list) hardcode MaxItems/Quantity and never apply Marker/MaxItems truncation or emit a NextMarker, unlike the ops fixed this pass and the handful already using paginateByMarkerID (ListDistributions, ListFunctions, ListInvalidations*, ListAnycastIPLists, ListDistributionTenantsByCustomization). Left unfixed: the fix is mechanical (route each through paginateByMarkerID/paginateByMarkerValue) but the volume (~20 handlers, each needing its own before/after real-SDK pagination test) was out of this pass's budget after the higher-value never-honoured-filter bugs. The ListDistributionsBy* family (11 ops) additionally has per-op output shape questions (DistributionIdList vs DistributionList vs DistributionIdOwnerList -- confirmed heterogeneous by reading 3 of the 11 Output structs) that a mechanical pagination patch alone would not resolve; that family needs a dedicated wire-shape read of each op's own Output/deserializer before touching its pagination, not a copy of the fix used elsewhere in this pass."
leaks: {status: clean, note: "runInvalidationReconciler goroutine has a proper stopCh + Close() lifecycle; no unbounded maps found. This pass added b.work (*pkgs/worker.Group), the mgn/outposts-style scheduled-timer idiom used by scheduleDistributionDeployed -- Close() now also calls b.work.Stop(), which cancels every pending timer and joins its goroutines, so nothing outlives the backend. seedManagedPoliciesLocked (prior pass) does no allocation beyond the fixed ~20-entry seed tables and is called only at construction/Reset/Restore, never per-request."}
---

## Notes

**gopherstack-bhhx (2026-08-13)**: fixed the `AssociateDistributionWebACL` gap `gopherstack-4ara`
confirmed but left open (see that entry below). Same bug class and same actual failure mode as the
tenant fix -- wrong request root/field (`WebACLAssociation`/`WebACLId` instead of the real
`AssociateDistributionWebACLRequest`/`WebACLArn`), `xml.Unmarshal` error checked rather than
discarded, so real clients got `400 MalformedXML` rather than a silent wipe -- but a DIFFERENT
real root than the tenant sibling's `AssociateDistributionTenantWebACLRequest`, confirmed against
`cloudfront@v1.67.4` `serializers.go:255`, so the fix needed its own dedicated request type rather
than reusing either the old shared type or the tenant op's. Surveyed every other XML type shared
across 2+ cloudfront ops for the same shared-type-different-real-root risk (`invalidationBatchXML`,
`tagXML`, `tagsXML`) -- all confirmed safe against the SDK's own canonical `types.InvalidationBatch`/
`types.Tags`/`types.Tag`, unlike `WebACLAssociation`/`WebACLId`, which never matched any real op's
wire. Verified against the real aws-sdk-go-v2 client and confirmed to fail against the pre-fix shape
by hand-reverting. Two pre-existing tests (`TestAssociateDistributionWebACL`, `TestDisassociateWebACL`)
encoded the same invented request shape the pre-fix handler expected and had been passing against
broken code indefinitely; both corrected to the real shape rather than preserved.

**gopherstack-4ara (2026-08-13)**: fixed the two wire-shape gaps `gopherstack-o31x` deliberately
left open (the KeyValueStore structural gap in the same issue was out of scope for that pass;
now RESOLVED in a follow-up pass the same day -- see the `key_value_stores` family note above
and services/cloudfrontkeyvaluestore/PARITY.md). (1) `AssociateDistributionTenantWebACL`'s request root/field
were wrong (`WebACLAssociation`/`WebACLId` instead of the real `AssociateDistributionTenantWebACLRequest`/
`WebACLArn`); the ACTUAL failure mode was every real client's request 400ing `MalformedXML`
outright, not the silent-200-with-empty-state pattern the filing bd issue described by analogy to
`PutResourcePolicy` -- gopherstack's `xml.Unmarshal` error here was checked, not discarded, unlike
the `PutResourcePolicy` precedent. The bug (every real call fails) was still real and still fixed;
only the exact mechanism differed from the filed premise, confirmed by driving the real
aws-sdk-go-v2 client both before and after the fix rather than trusting the filed description.
Also confirmed (not fixed here -- see the `gopherstack-bhhx` entry above for the follow-up fix)
that the non-tenant sibling `AssociateDistributionWebACL` shares the identical bug class, though
with a different real root name. (2) `ListConnectionGroups`/`ListConnectionFunctions` responses wrapped items under
an invented `<X><Items>` element with a fabricated `<Quantity>`; the real deserializers read a
direct `<ConnectionGroups>`/`<ConnectionFunctions>` element with no wrapper at all, so a real
client always decoded an empty list -- fixed by matching the real element names and dropping
`Quantity`. Both fixes verified against the real aws-sdk-go-v2 client, each confirmed to fail
against the pre-fix shape by hand-reverting. A pre-existing test, `TestAssociateDistributionTenantWebACL`,
encoded the exact same invented request shape the pre-fix handler expected and so had been passing
against broken code indefinitely; its body was corrected to the real shape rather than preserved.

**ETag/IfMatch** (proven, not touched this pass): Update/Delete for Distribution, CachePolicy,
OriginRequestPolicy, ResponseHeadersPolicy, OriginAccessControl, OAI, CloudFront Function,
ContinuousDeploymentPolicy, and DistributionTenant all require an `If-Match` header equal to
the resource's current ETag, else `412 PreconditionFailed`. This was already correct across
the board before this sweep; verified op-by-op, no gaps found.

**InconsistentQuantities (the headline fix this pass)**: CloudFront's wire format pairs a
caller-supplied `<Quantity>N</Quantity>` with an `<Items>...</Items>` list virtually
everywhere in the schema (57 distinct SDK types carry a `Quantity *int32` field). Real
AWS rejects a request where `N` disagrees with the actual number of items with
`InconsistentQuantities` (400). Before this pass, the emulator had **zero** occurrences of
this validation anywhere in the codebase -- `grep -rn InconsistentQuantities` was empty.
Root cause: `DistributionConfig` (and most other configs) is parsed into either a minimal
typed struct or stored as opaque `RawConfig` bytes; nothing ever re-derived the caller's
stated `Quantity` and compared it to the real list length, because Go slices don't need an
explicit count. Fix: `services/cloudfront/quantity_validation.go` adds a generic recursive
XML-tree walker (`validateQuantities`) that finds every `<X><Quantity>..</Quantity>
<Items>..</Items></X>` pairing in an arbitrary config body and flags a mismatch --
no per-resource schema modeling required, and provably safe against false positives
because it only fires when both `Quantity` and `Items` siblings are actually present
(verified against `KeyGroupConfig`/`PublicKeyConfig`/`RealtimeLogConfig`/`VpcOriginConfig`,
none of which use this pattern in the real SDK, via the smithy serializers). Wired into
all ~58 Create/Update body-parsing call sites across `handler.go`, `handler_batch2.go`,
and `handler_new_ops.go`.

**AlreadyExists error codes were all wrong (second major finding)**: `handleError`'s
`ErrAlreadyExists` sentinel had `code = "DistributionAlreadyExists"` and was reused
verbatim for CachePolicy, OriginRequestPolicy, ResponseHeadersPolicy, OriginAccessControl,
CloudFront Function, FieldLevelEncryptionConfig, FieldLevelEncryptionProfile, PublicKey,
KeyGroup, and RealtimeLogConfig name/CallerReference collisions -- i.e. creating a second
cache policy with a taken name returned the literal string `DistributionAlreadyExists`,
which is CloudFront's *distribution*-specific error code and was never even triggered by
an actual distribution collision (`CreateDistribution` doesn't use this sentinel at all;
it's fully idempotent on CallerReference, see gap above). Two existing tests
(`TestRefinement1_CachePolicyUniqueness`, `TestRefinement1_ErrorMapping`) asserted this
wrong code as if it were correct -- both fixed with justification comments pointing at the
real `aws-sdk-go-v2/service/cloudfront/types` error type names. Fix: 11 new distinct
sentinel errors (one per resource, matching the real SDK's dedicated error type where one
exists, falling back to the real generic `EntityAlreadyExists` where the SDK has no
resource-specific type -- e.g. Anycast IP lists, key value stores, trust stores). The
`handleError` switch (which had grown to cyclomatic complexity 23) was refactored into a
data-driven `errCodeMapping` table (pattern already established by EC2's `errCodeLookup`),
fixing a `cyclop` lint violation as a side effect.

**Function responses were missing FunctionARN/CreatedTime/LastModifiedTime (third
finding)**: `FunctionMetadata` requires `FunctionARN` and `LastModifiedTime` per the real
SDK (`CreatedTime`/`Stage` too). The emulator's `Function` backend struct *did* compute
and store an ARN (`b.functionARN(name)`) on create, but `functionResponseXML` (shared by
Create/Get/Describe/Publish/Update) and the inline `FunctionSummary` builder in
`handleListFunctions` never emitted it -- a real SDK caller had no way to get a function's
ARN back from any read operation, which makes attaching the function to a distribution's
`FunctionAssociations` (which require the ARN, not the name) impossible. Fixed by adding
`CreatedTime`/`LastModifiedTime` fields to `Function`, populating them on
Create/Update/Publish, and emitting all four `FunctionMetadata` fields from both XML
builders.

**InUse-on-delete guards (fourth finding)**: `DeleteCachePolicy`, `DeleteOriginRequestPolicy`,
`DeleteResponseHeadersPolicy`, and `DeleteFunction` had **no** check for whether the
resource was still referenced by a distribution -- real AWS returns `CachePolicyInUse` /
`OriginRequestPolicyInUse` / `ResponseHeadersPolicyInUse` / `FunctionInUse` (409) in that
case. (`PublicKeyInUse` and `FieldLevelEncryptionProfileInUse` already existed and are
correct -- not touched.) Fixed by adding `tokenReferencedByAnyDistribution` to
`backend_search_index.go`, reusing the pre-existing inverted token index that already
backs `ListDistributionsByCachePolicyID` etc. (built for the `ListDistributionsBy*`
control-plane ops) -- an O(1) check with no new scanning logic.

**gopherstack-na4 closed this pass: `KeyGroup`/`OAI`/`OriginAccessControl` InUse guards.**
`DeleteKeyGroup`, `DeleteOAI`, and `DeleteOriginAccessControl` had the same missing-guard gap
as the fourth finding above, deferred previously because each needed a slightly different
search token than the bare-ID case `tokenReferencedByAnyDistribution` already handled:
- `KeyGroup`: bare ID, referenced via `TrustedKeyGroups.Items` -- same pattern as
  CachePolicy, just a drop-in `tokenReferencedByAnyDistribution(id)` call. Returns
  `ResourceInUse` (409) on conflict: real `DeleteKeyGroup` has no dedicated `KeyGroupInUse`
  type, `ResourceInUse` is the actual documented error (verified against the live API
  reference), matching the existing `ErrKeyGroupNotFound` -> `NoSuchResource` precedent.
- `OAI`: referenced via `S3OriginConfig.OriginAccessIdentity`, whose real wire value is the
  literal path string `"origin-access-identity/cloudfront/{id}"`, not the bare ID (verified
  against the real `S3OriginConfig.OriginAccessIdentity` doc comment). Added
  `oaiReferencePath(id)` (also now shared by `oaiARN`) and check
  `tokenReferencedByAnyDistribution(oaiReferencePath(id))`. Returns
  `CloudFrontOriginAccessIdentityInUse` (409).
- `OriginAccessControl`: referenced via `Origin.OriginAccessControlId`, a bare ID like
  CachePolicyId -- same drop-in pattern as KeyGroup. Returns `OriginAccessControlInUse`
  (409).

All three verified end-to-end via `Test_ResourceInUse_BlocksDelete` in
`resource_in_use_test.go` (extended this pass): create the resource, attach it to a
distribution's raw config, assert delete is blocked with the correct code, disable+delete
the distribution, assert delete now succeeds.

**InconsistentQuantities trap for the next auditor**: don't add per-resource Quantity
validation by hand if you find a new Create/Update body-parsing handler missing the
`validateQuantities(body)` call -- just add the one-line call. The generic walker already
covers any shape with a `<Quantity>`/`<Items>` sibling pair; it is a no-op (returns nil)
for shapes that don't use the pattern, so it is always safe to add defensively.

**"Looks wrong but is correct" traps**:
- `ErrKeyGroupNotFound`'s wire code is `NoSuchResource`, not `NoSuchKeyGroup` -- this
  matches the real SDK (`types.NoSuchResource` is what CloudFront actually returns for a
  missing key group; there is no dedicated `NoSuchKeyGroup` type). Don't "fix" this.
- `ErrKeyValueStoreNotFound`/the new fallback `ErrAlreadyExists` both use `EntityNotFound`/
  `EntityAlreadyExists` -- also correct; the real SDK has no KVS-specific *NotFound/
  *AlreadyExists type either.
- `CreateAnycastIPList`/`CreateKeyValueStore`/`CreateTrustStore` intentionally still use the
  generic `ErrAlreadyExists` (now `EntityAlreadyExists`) sentinel rather than a dedicated
  one -- there is no `AnycastIpListAlreadyExists`/`KeyValueStoreAlreadyExists` type in
  `aws-sdk-go-v2/service/cloudfront/types@v1.60.2` to match; this is the AWS-accurate
  fallback, not an oversight.

**Protocol**: REST-XML throughout (control plane only, as of gopherstack-4ara 2026-08-13).
KeyValueStore's data plane (GetKey/PutKey/DeleteKey/ListKeys/UpdateKeys/DescribeKeyValueStore)
uses a genuinely separate REST-JSON protocol and SDK client (`cloudfrontkeyvaluestore`) with its
own unversioned path family (`/key-value-stores/...`, no `/2020-05-31/` prefix) -- it now lives
entirely in services/cloudfrontkeyvaluestore, not here. Do not re-add data-plane handlers to
this service; this Handler's RouteMatcher is anchored on `/2020-05-31/`, which the real
cloudfrontkeyvaluestore client never sends a request through, so anything added here would be
unreachable again (the exact bug gopherstack-4ara fixed).

---

## This pass's findings (2026-07-23 re-audit)

**Fifth finding: CachePolicy/OriginRequestPolicy/ResponseHeadersPolicy whitelist Items
lists, both directions.** Field-diffing these three families against the real SDK request
syntax (not just the Go struct field names, which matched) turned up the same bug class as
the second finding, but worse because it hit both parse AND serialize:
- **Parse (CachePolicy only)**: `cachePolicyHeadersConfigXML`/`CookiesConfigXML`/
  `QueryStringsConfigXML` used `xml:"Headers>Header"` / `"Cookies>Cookie"` /
  `"QueryStrings>QueryString"`. The real wire path (verified against the live
  `CreateCachePolicy`/`UpdateCachePolicy` request syntax) is `Headers>Items>Name` /
  `Cookies>Items>Name` / `QueryStrings>Items>Name`. Every whitelist/allExcept request a real
  SDK client sent had its listed names silently discarded on unmarshal -- `Headers` came back
  an empty slice with no error. (OriginRequestPolicy's parse-side tags were already correct;
  only its response side was broken -- see next bullet.)
- **Serialize (all three families, every read op)**: `cachePolicyResponseXML`,
  `orpResponseXML`, and `rhpResponseXML` either omitted the Items list entirely (emitting a
  bare `<Quantity>N</Quantity>` with no `<Items>`/no wrapper element at all) or, for
  `ResponseHeadersPolicy`'s CORS config, dropped all four list fields
  (`AccessControlAllowOrigins`/`AccessControlAllowHeaders`/`AccessControlAllowMethods`/
  `AccessControlExposeHeaders`) and two `SecurityHeadersConfig` fields
  (`ContentTypeOptions`, `ContentSecurityPolicy`) completely, even though the request parser
  already captured all of them correctly. `GetCachePolicyConfig`/
  `GetOriginRequestPolicyConfig`/`GetResponseHeadersPolicyConfig` omitted the entire nested
  config block (`ParametersInCacheKeyAndForwardedToOrigin`/`HeadersConfig+CookiesConfig+
  QueryStringsConfig`/`CorsConfig+SecurityHeadersConfig`) -- not just the lists. A real SDK
  caller had no way to discover which headers/cookies/query-strings/origins/methods a policy
  actually configures via any read op.

  Fix: added `xmlNameItems`/`xmlPluralItems` shared helpers (`handler_cache_policies.go`) and
  `cachePolicyConfigXMLBlock`/`orpConfigXMLBlock`/`rhpConfigXMLBlock` builders reused across
  each family's full response, config-only response, and List summary -- eliminating the
  triplicated, inconsistent hand-built XML that let the three call sites drift out of sync
  with each other in the first place. Locked in by
  `TestCachePolicyWhitelistItems_WireRoundTrip`,
  `TestOriginRequestPolicyWhitelistItems_WireRoundTrip`, and
  `TestResponseHeadersPolicyCORSItems_WireRoundTrip`.

**Sixth finding, CLIENT-BREAKING: `UpdateOriginRequestPolicy` routed to the wrong path.**
`parseCFOriginRequestPolicyPath` only matched `PUT` when the URL suffix ended in `/config`.
The real `UpdateOriginRequestPolicy` wire request is `PUT /2020-05-31/origin-request-policy/
{Id}` -- the bare-ID path, exactly like `UpdateCachePolicy` and `UpdateResponseHeadersPolicy`
(verified against the live API reference request syntax for all three). No real SDK client
ever sends `/config` on a PUT; `/config` is GET-only (`GetOriginRequestPolicyConfig`). Every
real `UpdateOriginRequestPolicy` call against this emulator 404'd with `NoSuchOperation:
unknown operation: Unknown`. An existing test (`TestOriginRequestPolicyCRUD/update_orp`) had
encoded this wrong path as correct and was fixed alongside the route.

**Seventh finding: `CreateDistribution`/`CopyDistribution`/`CreateStreamingDistribution`
CallerReference semantics.** The previously-filed gopherstack-mzx gap assumed a
content-comparison rule (idempotent if identical, conflict if different) by analogy with OAI.
Re-verified against the live API reference pages, not just the SDK's terser doc comments:
`CreateDistribution`'s docs state CallerReference reuse returns `DistributionAlreadyExists`
"regardless of the content of the DistributionConfig object" -- i.e. it NEVER treats reuse as
idempotent, even for byte-identical bodies. Same wording for `CreateStreamingDistribution`
(-> `StreamingDistributionAlreadyExists`) and `CopyDistribution` (which additionally wasn't
tracking CallerReference uniqueness at all before this pass -- `distributionCallerRefs` was
never populated by `CopyDistribution`). `CreateOAI` is the one family where the SDK doc's
content-comparison language is accurate and was implemented as such (identical `Comment` ->
idempotent return; different `Comment` -> `CloudFrontOriginAccessIdentityAlreadyExists`).
Existing tests asserting the old (wrong) always-idempotent behavior for Distribution and
StreamingDistribution were fixed: `TestCallerReferenceReuse` (renamed from
`TestCallerReferenceIdempotency`), `TestPersistenceRoundTrip_IndexesRebuilt`,
`TestStreamingDistributionSnapshotRestore`, `TestInMemoryBackend_StreamingDistribution`.

**Managed policies (gopherstack-a9t, closed)**: see the `managed_policies` family row above
and `managed_policies.go`'s doc comment for the full rationale, verification method, and the
deliberately-omitted Amplify-internal policy set. Every ID was cross-checked against the live
AWS documentation pages (not invented, not guessed) via `WebFetch`, since a wrong ID posing as
a real managed-policy ID would be worse than not seeding one at all.

---

## Discarded xml.Unmarshal errors sweep (2026-08-13, gopherstack-ob1g)

`encoding/xml` returns an error when a document's root element doesn't match the target
struct's `XMLName` tag, and leaves the struct **zeroed**, not partially filled -- so
`_ = xml.Unmarshal(body, &req)` with a wrong root silently discards the entire request and
proceeds on zero values, exactly the mechanism behind the `PutResourcePolicy`/
`CreateRealtimeLogConfig` bugs fixed by gopherstack-nfka (e5fbae252) above. This pass swept
every remaining non-test `_ = xml.Unmarshal(...)` call in `services/cloudfront/` (28
occurrences) and re-verified each struct's `XMLName` against the pinned SDK's serializer.

**Two more genuine whole-request wipes found and fixed** (each verified to fail against the
pre-fix code by reverting by hand, and covered by a real aws-sdk-go-v2 client round-trip
test): `UpdateVpcOrigin` (root was `UpdateVpcOriginRequest`, real root is
`VpcOriginEndpointConfig` itself -- see the corrected `UpdateVpcOrigin` op row above) and
`UpdateTrustStore` (root was `TrustStoreConfig` with Name/Comment fields that don't exist on
the real wire at all; real root is `CaCertificatesBundleSource` -- see the `UpdateTrustStore`
op row above).

**Two routing bugs found as a second layer behind hardening fixes**, per this pass's mandate
to check routability whenever touching one of these handlers (`UpdateDistributionWithStagingConfig`
matched a `/staging` suffix no real client sends; `ListDomainConflicts` matched the singular
`domain-conflict` instead of the real plural `domain-conflicts`) -- both fixed, see their op
rows above. **One more routing bug found but NOT fixed** (`UpdatePublicKey`/
`UpdateFieldLevelEncryptionConfig`/`UpdateFieldLevelEncryptionProfile` bound to the wrong path
shape) -- see `gaps` above for why it was left for a follow-up pass.

**The remaining 26 occurrences (23 in cloudfront: all but the two above; 3 in s3) had a
correct `XMLName` already** -- the discarded error was hardening, not a live bug, since no
real client body could ever hit a mismatched root there. Each now returns the service's
`MalformedXML` error (matching the pattern already established elsewhere in both codebases,
e.g. `handler_anycast_ip_lists.go`) instead of silently discarding the error, which is what
would have made the two genuine wipes above immediately findable instead of surviving three
prior audit passes. Covered by `TestXMLUnmarshalErrorHandled` (cloudfront) and
`TestXMLUnmarshalErrorHandled`/`TestRestoreObject_MalformedBodyHandled` (s3), each of which
fails against the pre-fix `_ = xml.Unmarshal(...)` form (spot-verified by reverting one
representative case, `CreateMonitoringSubscription`, by hand).

The matching s3 sweep (4 occurrences, one genuine wipe -- `GetBucketAbac`'s re-parse of its
own stored `PutBucketAbac` body used root `AbacConfiguration` where the real root is
`AbacStatus`, discovered to ALSO have a response-nesting bug once fixed: `GetBucketAbacOutput`
is httpPayload-bound so the response body itself must be the bare `AbacStatus` document, not
`AbacStatus` nested under an `AbacConfiguration` envelope -- the real deserializer function
that looks for a nested child is dead/unused generated code, not evidence of an envelope
shape) is recorded in `services/s3/PARITY.md`.

---

## Full route-table audit (2026-08-13, gopherstack-o31x)

cloudfront had produced eleven routing bugs across three prior passes (six in
gopherstack-nfka, two in gopherstack-ob1g, three filed-but-unfixed also in gopherstack-ob1g)
without ever getting a full diff of all its ops against the SDK -- only the ops other work
happened to touch were ever checked. A fleet-wide sweep (gopherstack-4nek) had to skip
cloudfront entirely because it was mid-edit at the time, and confirmed the fleet-wide finding
of zero mismatches held for the 76 services actually swept -- cloudfront was flagged as the
one confirmed hotspot and the right next target.

**Method**: extracted every real cloudfront op's method and path template from
`cloudfront@v1.67.4` serializers.go directly -- for each `awsRestxml_serializeOp<Op>`, its
`request.Method` assignment and the literal string passed to `httpbinding.SplitURI(...)`, both
in the same `HandleSerialize` function body. This is authoritative by construction: it's the
same code path the real SDK client runs to build a request, not a description of it. Extracted
167 ops this way (all of cloudfront's control-plane operations; excludes the 5 KeyValueStore
data-plane ops, which live in a structurally separate SDK client/protocol -- resolved
2026-08-13 in services/cloudfrontkeyvaluestore, see the `key_value_stores` family note above).

Then, instead of eyeballing the ~1000-line `handler_paths.go` route table by hand against that
list, built `TestExtractOperation_SDKRouteTable` (`handler_paths_sdk_diff_test.go`): a
table-driven test that builds a real `httptest.Request` for every one of the 167 extracted
(method, path) pairs and asserts `Handler.ExtractOperation` resolves it to the right op name.
Run against the pre-fix code (after Part 1's three known bugs were already fixed, but before
any of the fixes below), it reported exactly 21 mismatches -- either resolving to `"Unknown"`
(no route matched at all) or to a plausible-but-wrong sibling op. Every one of the 21 is now
fixed; see the op rows and family notes above for each (`list_distributions_by` accounts for
12, `monitoring_subscription` for 3, and one each for `GetManagedCertificateDetails`,
`DisassociateDistributionTenantWebACL`, `GetConnectionGroupByRoutingEndpoint`,
`ListConnectionGroups`, `GetDistributionTenantByDomain`, `ListConnectionFunctions`). Re-run
after all fixes, `TestExtractOperation_SDKRouteTable` reports zero mismatches across all 167
ops -- kept as a permanent regression test against this exact bug class recurring.

Two further bugs were NOT caught by the mechanical diff, because they don't manifest as a
wrong *op name* -- they're wrong signals feeding the SAME correct-looking dispatch: (1)
`CreateDistributionWithTags`/`CreateStreamingDistributionWithTags` silently resolved to their
non-tagged sibling because the WithTags flag was read from the wrong query key (a real client
never sends `Resource=WithTags`, only a bare `?WithTags`) -- found by writing a real-client
test and noticing tags never applied; (2) `TagResource`/`UntagResource` were disambiguated by
HTTP method instead of the real `Operation=Tag|Untag` query value, so `UntagResource` (always
POST on the real wire) landed on the `TagResource` handler -- found the same way. Both fixed;
see their op rows above.

**Verification**: every fix in this pass has a real `aws-sdk-go-v2` client test proving
reachability (`newTestCloudFrontClient`, driving the real router, not a hand-built request
that could encode the same wrong assumption the handler makes), and every fix was confirmed to
fail against its pre-fix shape by reverting the change by hand and re-running the test before
restoring it -- the same discipline this pass's mandate required for Part 1. Two response-body
wire-shape bugs (`AssociateDistributionTenantWebACL`'s request root/field names,
`ListConnectionGroups`/`ListConnectionFunctions`' response list wrapper) and one structural gap
(the KeyValueStore data-plane ops' host/protocol mismatch) were found as a second layer behind
these routing fixes and were deliberately NOT fixed here -- wire-shape and structural-routing
bugs are a different class of work than the method+path diff this pass's mandate scoped to. All
three were resolved in follow-up passes the same day: the two wire-shape bugs by gopherstack-4ara
(see the Notes entry above), the KeyValueStore structural gap by a further gopherstack-4ara pass
that split the data plane into services/cloudfrontkeyvaluestore.

## 2026-08-23: pagination bug sweep (ListInvalidations, ListInvalidationsForDistributionTenant, ListFunctions)

Discovered while auditing the pagination bug class found in medialive.
`handleListInvalidations`, `handleListInvalidationsForTenant`, and
`handleListFunctions` all ignored the real, per-op `Marker`/`MaxItems`
request members (cloudfront@v1.67.4: every List op's Input has
`Marker *string, MaxItems *int32`) and always returned every item in one
unbounded page — `handleListInvalidations` even hardcoded
`<IsTruncated>false</IsTruncated>`. Fixed all three using a new shared
`paginateByMarkerID` helper (`pagination_helper.go`, generalizing the
existing correct pattern from `handleListDistributions`/
`handleListAnycastIPLists`), sorted by `Invalidation.ID`/`Function.Name`.
Proven with `TestListInvalidations_SDKRoundTrip_Pagination`,
`TestListInvalidationsForDistributionTenant_SDKRoundTrip_Pagination`, and
`TestListFunctions_SDKRoundTrip_Pagination`
(`list_pagination_ignored_test.go`), each driving the real SDK client across
two 10-item pages of 25 seeded items and asserting the pages are disjoint;
all three fail against the unfixed handlers (`should have 10 item(s), but
has 25`), hand-reverted and confirmed.

Audited but NOT fixed this pass, same ignored-pagination pattern, all
low blast radius given CloudFront's own low per-resource-type account quotas
(cache policies, origin request policies, response headers policies ~20;
key groups ~10; public keys ~100; real-time log configs ~20; field-level
encryption configs/profiles, streaming distributions (legacy), trust stores,
connection groups/functions, key value stores, VPC origins, origin access
controls — all similarly small): `handleListCachePolicies`,
`handleListContinuousDeploymentPolicies`, `handleListConnectionGroups`,
`handleListConnectionFunctions`, `handleListFieldLevelEncryptions`,
`handleListFieldLevelEncryptionProfiles`, `handleListPublicKeys`,
`handleListKeyGroups`, `handleListKeyValueStores`, `handleListOAIs`,
`handleListOriginAccessControls`, `handleListOriginRequestPolicies`,
`handleListRealtimeLogConfigs`, `handleListResponseHeadersPolicies`,
`handleListStreamingDistributions`, `handleListTrustStores`,
`handleListVpcOrigins`. None of these apply an artificial default cap while
discarding the client's token (the medialive/true-loop bug shape) — they
simply return every item unbounded, so no data loss and no infinite loop,
just a spec deviation. Follow-up: apply the same `paginateByMarkerID`
helper to each.

## 2026-08-23: structured ops/families diff audit (clientcoverage said 68/167)

`clientcoverage` reported cloudfront at 68/167 (40.7%), but the actual number of
control-plane ops never covered by an `ops:`/`families:` entry above -- diffing
the 169 op-name string constants this service's `handler.go` actually declares
(167 real control-plane ops + the 2 already-superseded KeyValueStore-era consts
still present as identifiers) against this file's structured entries, not prose --
was **1**: `DisassociateDistributionWebACL` (see its new op row above; its tenant
sibling `DisassociateDistributionTenantWebACL` already had one). Everything else
that looked missing on a first name-diff resolved to a family/pagination-note
match once checked: all 12 `ListDistributionsBy*` (`list_distributions_by`
family), `ListCloudFrontOriginAccessIdentities`/`GetCloudFrontOriginAccessIdentityConfig`
(the combined `GetCloudFrontOriginAccessIdentity / Config / List` ops entry),
10 of the 17 ops named in the 2026-08-23 pagination-sweep note above as
"audited but not fixed" (`ListRealtimeLogConfigs`, `ListStreamingDistributions`,
`ListTrustStores`, `ListVpcOrigins`, `ListKeyValueStores`, `ListPublicKeys`,
`ListKeyGroups`, `ListFieldLevelEncryptionConfigs`, `ListFieldLevelEncryptionProfiles`,
`ListContinuousDeploymentPolicies`), and `CreateContinuousDeploymentPolicy`/
`UpdateContinuousDeploymentPolicy`/`CreateFieldLevelEncryptionConfig`/
`CreateFieldLevelEncryptionProfile`/`DeletePublicKey`/`DeleteKeyValueStore`
(named in the `continuous_deployment_policy`/`field_level_encryption`/
`public_keys_key_groups`/`key_value_stores` family notes).

**Found and fixed**: `AssociateDistributionWebACL` and `DisassociateDistributionWebACL`
both never set the `ETag` response header, and `AssociateDistributionWebACL`
returned an empty body -- see their op rows above.

**False positive ruled out by hand, not just by pattern-match**: on first read,
`connection.go`/`handler_connection.go`'s `ConnectionGroup`/`ConnectionFunctionSummary`/
`ConnectionFunctionTestResult` responses looked like the same missing-envelope bug
class (their real Output structs wrap a named nested type, e.g.
`CreateConnectionGroupOutput.ConnectionGroup *types.ConnectionGroup`, and gopherstack's
response root is that same type's fields directly, with no `<ConnectionGroup>` wrapper
child). Checking which deserializer function each op's `HandleDeserialize` *actually
calls* (not just grepping for a plausibly-named `awsRestxml_deserializeOpDocument<Op>Output`
function, which can be dead/unused generated code -- exactly the trap the s3
`GetBucketAbac` note above already documents) showed every one of these ops calls the
type's own `awsRestxml_deserializeDocument<Type>` function directly on the unwrapped
response body, not the nested-child-scanning `OpDocument` function. So the real wire
*is* the flat, unwrapped shape gopherstack already sends. Confirmed empirically, not just
by reading source: a real-client round trip (`TestReproConnectionGroupRaw`, written to
verify then deleted -- not a permanent test since it found no bug) decoded
`CreateConnectionGroupOutput.ConnectionGroup` fully populated against unmodified code.
No fix needed; `connection.go`/`anycast_ip_lists.go` show the same care elsewhere
(serializer/deserializer line-number citations already in their doc comments) despite
never having had a dedicated PARITY.md ops/families entry.

**Not reached** (no ops/families entry either way -- spot-checked by reading their
handler bodies for the stub/wrong-required-field/sentinel-collision shapes this pass's
mandate named, found non-stub and structurally sound, but not individually field-diffed
against the SDK response deserializers the way the ops above were): `CreateAnycastIPList`,
`GetAnycastIPList`, `UpdateAnycastIPList`, `DeleteAnycastIPList`, `ListAnycastIPLists`,
`CreateConnectionFunction`, `GetConnectionFunction`, `DescribeConnectionFunction`,
`UpdateConnectionFunction`, `DeleteConnectionFunction`, `PublishConnectionFunction`,
`TestConnectionFunction`, `CreateConnectionGroup`, `UpdateConnectionGroup`,
`DeleteConnectionGroup`, `DeleteContinuousDeploymentPolicy`, `GetContinuousDeploymentPolicy`,
`GetContinuousDeploymentPolicyConfig`, `DeleteFieldLevelEncryptionConfig`,
`GetFieldLevelEncryption`, `GetFieldLevelEncryptionConfig`, `GetFieldLevelEncryptionProfile`,
`GetFieldLevelEncryptionProfileConfig`, `GetDistributionTenant`, `GetFunctionAssociations`,
`SetFunctionAssociations`, `GetInvalidationForDistributionTenant`,
`CreateInvalidationForDistributionTenant` (its shared `invalidationBatchXML` request type was
confirmed safe by the 2026-08-13 gopherstack-bhhx sweep, but its own response/routing was not),
`ListConflictingAliases`, `VerifyDNSConfiguration`, `GetKeyGroup`, `GetKeyGroupConfig`,
`GetPublicKey`, `GetPublicKeyConfig`, `GetStreamingDistribution`,
`GetStreamingDistributionConfig`, `DeleteStreamingDistribution`, `UpdateStreamingDistribution`,
`GetTrustStore`, `DeleteTrustStore`, `GetVpcOrigin`. Files: `anycast_ip_lists.go`,
`handler_anycast_ip_lists.go`, `connection.go`, `handler_connection.go`,
`continuous_deployment.go`, `field_level_encryption.go`, `handler_field_level_encryption.go`,
`distribution_tenants.go`, `handler_distribution_tenants.go`, `handler_distributions.go`,
`invalidations.go`, `handler_invalidations.go`, `key_groups.go`, `handler_key_groups.go`,
`streaming_distributions.go`, `handler_streaming_distributions.go`, `trust_stores.go`,
`handler_trust_stores.go`, `vpc_origins.go`, `handler_vpc_origins.go`.

Gate output (this pass, `services/cloudfront/` only): `go build ./services/cloudfront/...`
clean; `go test ./services/cloudfront/...` -- `ok github.com/blackbirdworks/gopherstack/services/cloudfront 0.187s`.

## 2026-08-23: empty-body sweep (AssociateDistributionTenantWebACL) and gaps: re-check (gopherstack-jf8z)

Scoped to two classes: (1) ops returning an empty body where the real Output declares members,
(2) recorded `gaps:` entries that turn out to be state the backend already tracks but never
surfaces.

**Class 1**: `grep -n "return nil, nil$"` (the ec2 `DeregisterImage` pattern) found nothing here --
this service's handlers never return an untyped nil through `xml.Marshal`. Broadened the search to
every `c.NoContent(http.StatusOK)` call site (200-with-no-body, as opposed to the correct
`c.NoContent(http.StatusNoContent)` used by ~25 real Delete/Tag/Untag ops whose real Outputs are
genuinely empty -- spot-checked `DeleteDistributionOutput`, `TagResourceOutput`,
`DeleteResourcePolicyOutput`, `DeleteMonitoringSubscriptionOutput` against the SDK, all confirmed
empty). Of the `StatusOK`-with-no-body call sites: `handleAssociateAlias` is correct
(`AssociateAliasOutput` has no members beyond `ResultMetadata`); `handleSetFunctionAssociations`
and `handleGetFunctionAssociations` are gopherstack-internal routes, not real CloudFront ops (no
`GetFunctionAssociations`/`SetFunctionAssociations` exist in `cloudfront@v1.67.4`'s `api_op_*.go`
files), out of scope; `handleAssociateDistributionTenantWebACL` was the one genuine bug -- see the
op row above. One genuinely wrong out of the ones checked.

Also caught, not from the empty-body grep but from re-verifying the 2026-08-13 commit's own claim
that `AssociateDistributionTenantWebACL`/`DisassociateDistributionTenantWebACL` were "checked and
correct": `DisassociateDistributionTenantWebACL` was missing its `ETag` response header (body `Id`
was already right). The existing real-client test for it only asserted `err == nil` and checked
state via a raw HTTP GET, never the SDK response object's fields, so the header gap had no test
that could catch it. See the op row above for both.

**Class 2**: redshift's `gaps:` is `[]` -- empty, nothing to re-check. cloudfront's `gaps:` block
(front matter, this file) contains zero live entries -- every line under it is a `#`-commented
historical note about a gap already RESOLVED in an earlier pass; no `- "..."` list item exists.
Same for any op/family row with a `status`/`wire`/etc. of literally `gap` -- none exist in either
service's structured `ops:`/`families:` sections. Extended the check to the three `deferred:`
entries below (structural, not `gaps:`, but the closest thing to a live open item in this file) and
applied the same test -- does the state exist in the backend already, just unsurfaced -- to each:
the InProgress->Deployed timer for 5 non-Distribution resource kinds requires new timer machinery
those backends don't have (not a surfacing bug); the DistributionConfig field audit is a
not-yet-done-audit note, not a specific claim; the `ResponseHeadersPolicySecurityHeadersConfig`
Override flattening was checked directly against `rhpSecurityHeadersXML`
(`handler_response_headers_policies.go`) -- the four hardcoded-false sub-fields
(STS/FrameOptions/ReferrerPolicy/ContentSecurityPolicy Override) aren't even parsed from the
request XML, so there is no stored value being withheld; the deferred note's own description was
accurate. All three left as recorded.

Gate output (this pass, `services/cloudfront/` only): `go build ./services/cloudfront/...` clean;
`golangci-lint run services/cloudfront/...` -- `0 issues.`; `go test ./services/cloudfront/... -count=1`
-- `ok github.com/blackbirdworks/gopherstack/services/cloudfront 0.170s`.

## 2026-08-30: paginated-listing reproducibility sweep (unstable page-boundary drop)

Targeted class: a Marker/MaxItems (or offset) cursor over a listing whose sort order isn't
reproducible between calls -- a record dropped or duplicated at a page boundary with
nothing changed in between. Read every `sort.Slice` (24 sites) feeding a `paginateByMarkerID`/
`paginateByMarkerValue` call plus every direct caller of those two helpers.

**Found and fixed**: `ListConnectionFunctions` (`connection.go`, `handler_connection.go`).
`CreateConnectionFunctionWithCode`'s own comment says "AWS allows multiple connection
functions to share the same Name -- they are keyed and uniqued by ID, not by name," yet
`ListConnectionFunctions` sorted solely by `Name` and `handleListConnectionFunctions`'
cursor used `getID(item) = fn.Name` -- once a group of same-named functions straddled a
`MaxItems` boundary, page 2's `getID(item) <= marker` cutoff silently discarded the rest
of the tied group forever (deterministic once a tie spans a boundary, not merely a
map-iteration flake). Proven with `TestListConnectionFunctions_DuplicateNames_NoDropAcrossPages`
(`list_pagination_ignored_test.go`, looped 30x for extra confidence though the drop
reproduces on the first iteration too) -- confirmed failing against unmodified code (2 of
5 same-named functions survived pagination), passing after. Fixed by (1) sorting on
`(Name, ID)` in `ListConnectionFunctions`, and (2) changing the cursor's `getID` and the
emitted `NextMarker` to `Name + "\t" + ID` (tab, not NUL -- Marker round-trips through the
XML request/response body and NUL is not a valid XML 1.0 character) so the cutoff can no
longer land mid-tie-group. `Marker`/`NextMarker` are documented opaque tokens
(`api_op_ListConnectionFunctions.go`), so exposing the composite key on the wire is safe;
no existing test asserted the literal Marker content.

**Confirmed safe, every other `sort.Slice` site checked**: all 23 remaining sort keys are
either the sorted table's own `store.Table` key (`distributions`, `oais`,
`anycastIPLists`, `cachePolicies`, `connectionGroups`, `continuousDeploymentPolicies`,
`originAccessControls`, `responseHeadersPolicies`, `functions` (keyed by Name),
`originRequestPolicies`, `fieldLevelEncryptions` x2, `publicKeys`, `keyGroups`,
`realtimeLogConfigs` (keyed by ARN, sorted by Name -- see next), `vpcOrigins`,
`trustStores`, `streamingDistributions`, `distributionTenants` x2, `invalidations`
(composite `distID#ID`, filtered to one distribution so `ID` alone is unique in that
subset)) or a field independently enforced unique at creation (`KeyValueStore.Name` --
`CreateKeyValueStore` checks `keyValueStoreByName` and returns `AlreadyExists`;
`RealtimeLogConfig.Name` -- same pattern via `realtimeLogConfigByName`). `ListKVSValues`
sorts by `Key`, which is literally the underlying Go map's own key -- immune by
construction. No "no sort at all" sites found (every truncating listing sorts first).

**Confirmed ignoring MaxItems/Marker entirely** (re-verified, not re-trusted from the
existing note -- see the sweep-methodology warning already on this file about a prior
false "already correct" claim): the ~20 `List*` ops the 2026-08-29 filter/pagination audit
already disclosed as hardcoding `MaxItems`/`Quantity` and never truncating are confirmed
accurate on inspection -- since they never truncate, they can't drop or duplicate a record
at a page boundary (a different, already-tracked completeness gap, not this pass's
target); left as previously disclosed rather than re-fixed here.

Gate output (this pass, `services/cloudfront/` only): `go build ./services/cloudfront/...`
clean; `go vet ./services/cloudfront/...` clean; `go test ./services/cloudfront/... -race
-count=1` -- `ok`; `golangci-lint run ./services/cloudfront/...` -- `0 issues.`

## 2026-08-30 (part 2): List pagination + ListDistributionsBy* shape fix (gopherstack-lkng)

Closes both gaps the 2026-08-29 filter/pagination audit disclosed and explicitly left unfixed
(see the header note above, now marked closed).

**16 single-shape listings wired to real Marker/MaxItems pagination**, each verified with its
own `TestList*_SDKRoundTrip_Pagination` test in `list_pagination_ignored_more_test.go` (25
records seeded, MaxItems=10, asserts page 1 is full + carries a cursor, the remainder comes
back exactly once with no duplicates, confirmed failing against the pre-fix handler via a
scoped `git stash` of only the source files, tests reapplied after):
`ListCachePolicies`, `ListOriginRequestPolicies`, `ListResponseHeadersPolicies` (query-bound,
`paginateByMarkerID`, `Type` filter applied before pagination -- already correct, not moved);
`ListOAIs` (`ListCloudFrontOriginAccessIdentities`), `ListOriginAccessControls`,
`ListFieldLevelEncryptionConfigs`, `ListFieldLevelEncryptionProfiles`, `ListPublicKeys`,
`ListKeyGroups`, `ListVpcOrigins`, `ListContinuousDeploymentPolicies`,
`ListStreamingDistributions` (all query-bound, `paginateByMarkerID`, sort key = the backend's
own unique ID); `ListRealtimeLogConfigs` (query-bound, sort/cursor key = `Name`, unique per
`CreateRealtimeLogConfig`'s own uniqueness check -- left un-retouched, matches the "one such
sort was correctly left alone" pattern); `ListTrustStores` (body-bound --
`awsRestxml_serializeOpDocumentListTrustStoresInput`, `paginateByMarkerValue`; real
`ListTrustStoresOutput.NextMarker` is a sibling of `TrustStoreList`, not a field on it, and
`TrustStoreList` itself has no `MaxItems` -- both preserved); `ListConflictingAliases`
(query-bound, `paginateByMarkerID`; `ListConflictingAliasesByDomain` ranged
`b.distributionAliases` -- a map -- with no sort, now sorted by distribution ID, its own
unique key); `ListDomainConflicts` (body-bound alongside `Domain`/
`DomainControlValidationResource`, `paginateByMarkerValue` keyed on `ResourceID`;
`findDomainConflicts` builds its result as one tenant match followed by a separately-sorted
list of distribution IDs -- two orderings concatenated, not one total order -- so a final
`sort.Slice` by `ResourceID` was added to give the pagination cursor a single stable order
across both halves).

Real wire-shape check for each (`go doc`/pinned SDK `types/types.go`): 8 of the 16
(`CachePolicyList`, `OriginRequestPolicyList`, `ResponseHeadersPolicyList`,
`FieldLevelEncryptionList`, `FieldLevelEncryptionProfileList`, `PublicKeyList`,
`KeyGroupList`, `ContinuousDeploymentPolicyList`) have **no `IsTruncated` field at all** --
`NextMarker`'s presence alone signals truncation -- so the handlers were rewritten to that
shape rather than keeping the previous always-`false` `IsTruncated` element every one of them
carried (harmless to a real client, which ignores unknown elements, but not wire-accurate);
`ConflictingAliasesList` is the same no-`IsTruncated` shape. The other 5
(`OriginAccessControlList`, `CloudFrontOriginAccessIdentityList`, `RealtimeLogConfigs`,
`VpcOriginList`, `StreamingDistributionList`) do carry `IsTruncated`, now populated for real.
`RealtimeLogConfigs` additionally has no `Quantity` field in the real type (`Items`/
`IsTruncated`/`MaxItems`/`NextMarker` only) -- the handler's phantom `Quantity` element was
dropped to match. None of the 16 echo the request's `Marker` value back on the response
(a `Marker` field the real Group-B types also carry) -- deliberately, to match this file's own
two pre-existing reference implementations (`handleListDistributions`,
`handleListAnycastIPLists`), which already omit it.

**`ListDistributionsBy*` family (12 ops, not 11 -- `ls` on the pinned SDK's
`api_op_ListDistributionsBy*.go` files gives 12: Anycast­IpListId, CachePolicyId,
ConnectionFunction, ConnectionMode, KeyGroup, OriginRequestPolicyId, OwnedResource,
RealtimeLogConfig, ResponseHeadersPolicyId, TrustStore, VpcOriginId, WebACLId) now marshal
through the correct one of three real output shapes instead of the one shared
`marshalDistributionList` every op previously used regardless of its actual `Output` struct:
- **`DistributionIdList`** (bare `Items []string` of distribution IDs) --
  `ByCachePolicyId`, `ByKeyGroup`, `ByOriginRequestPolicyId`, `ByResponseHeadersPolicyId`,
  `ByVpcOriginId`. New `marshalDistributionIDList`.
- **`DistributionList`** (full `DistributionSummary` objects, the shape every op previously
  used) -- `ByAnycastIpListId`, `ByConnectionFunction`, `ByConnectionMode`, `ByTrustStore`,
  `ByWebACLId`, `ByRealtimeLogConfig`. Existing `marshalDistributionList`, now paginated
  (previously hardcoded `MaxItems`/never truncated here too).
- **`DistributionIdOwnerList`** (`Items []DistributionIdOwner`, pairing a distribution ID with
  an owning account ID) -- `ByOwnedResource` only. New `marshalDistributionIDOwnerList`;
  `OwnerAccountId` is always this backend's own account (single-account emulator), read via a
  new `(*InMemoryBackend).AccountID()` accessor (`store.go`, mirrors the existing `Region()`).

Confirmed each op's real binding and Output type by reading its own
`awsRestxml_serializeOpHttpBindings*Input`/`serializeOpDocument*Input` and `*Output` struct in
the pinned SDK rather than assuming the family is uniform: 11 of the 12 bind Marker/MaxItems to
the query string (`paginateByMarkerID`); `ByRealtimeLogConfig` alone binds them in the XML
request body alongside `RealtimeLogConfigArn` (`paginateByMarkerValue`) -- the existing
`extractRealtimeLogConfigArn` body-reader was replaced with
`decodeListDistributionsByRealtimeLogConfigBody`, since the old one only read the ARN and the
body can be read exactly once; the `handler_dispatch.go` call site updated accordingly (its
signature change is internal to this package, no repo-root call-site fix needed).
`distributionsByConfigSearch` (`search_index.go`, backs 9 of these 12 plus
`ListDistributionsByCachePolicyID`/`OriginRequestPolicyID`/`ResponseHeadersPolicyID` used
elsewhere) and `ListDistributionsByWebACLID` (`distributions.go`) both range a map with no
sort -- added `sort.Slice` by distribution ID (the map's own key, already unique) to both.

Two pre-existing tests (`TestListDistributionsByPolicyID_RoundTrip`,
`TestListDistributionsByKeyGroup`) asserted `strings.Contains(resp, "DistributionList")` for
ops that actually return `DistributionIdList` -- passed only because the DistributionList-shape
handler these ops previously shared happened to satisfy that substring check by coincidence,
not because the shape was right (a real client decoding these fields against `DistributionIdList`
would read `Items` as bare ID strings vs `DistributionSummary` structs -- silently wrong data,
not a decode error). Both updated to assert `DistributionIdList` instead, matching the corrected
shape; this is exactly the "existing tests that could not have caught these" class the task
description warned about.

All 12 family ops covered by their own `TestListDistributionsBy*_SDKRoundTrip_Pagination` test
(same 25-record/MaxItems=10 pattern as above), including a positive assertion on the correct
shape's `Items` field (`DistributionIdList.Items []string` vs `DistributionList.Items
[]types.DistributionSummary` vs `DistributionIdOwnerList.Items []types.DistributionIdOwner`) so
a future shape regression fails a type-check, not just a substring check.

No AWS documentation was fetched for this pass (all wire-shape facts came from the pinned
`aws-sdk-go-v2` module in the local Go module cache, not the web), so the security note about
an injected `aws agent-toolkit search-skills` footer in fetched docs (flagged elsewhere in this
campaign) does not apply here.

Gate output (this pass, `services/cloudfront/` only): `go build ./services/cloudfront/...`
clean; `go vet ./services/cloudfront/...` clean (repo-wide `go vet ./...` also clean -- no
call-site fix needed in any root `cli_*_test.go`); `go test ./services/cloudfront/... -race
-count=1 -shuffle=on` -- `ok`; `golangci-lint run ./services/cloudfront/...` -- `0 issues`
(after restoring `//nolint:dupl` on four handlers whose doc-comment rewrite had dropped the
existing directive, and adding it to two newly-`dupl`-flagged pairs --
`ListOriginRequestPolicies`/`ListResponseHeadersPolicies` and, in `services/autoscaling`,
`DescribeLoadBalancers`/`DescribeLoadBalancerTargetGroups` -- confirmed these are pre-existing
"different resource types sharing the same list-XML shape" duplication, not new debt, before
adding the suppression).

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in `ef0eef041`
(`cmd/reqfieldscan`/`cmd/reqfielddiff` used to break ties among
case-insensitive handler-name candidates by Go's randomized map iteration
order, so they could read the wrong function body). Built the unpatched
tools from `ef0eef041~1` in a worktree, ran both five times against this
package, and diffed against HEAD.

`cmd/reqfieldscan`: byte-identical JSON across all 5 old runs and HEAD.
`cmd/reqfielddiff`: 155 findings in every one of the 5 old runs and at
HEAD, and the op.field key sets are identical, not merely equal in count.
ZERO DAMAGE -- confirmed by the actual diff, not inferred from collision
count (not separately re-measured this pass; the prior campaign already
established collisions don't predict damage).

## 2026-08-31 per-item exact-case sweep (gopherstack-21my continuation)

Byte-for-byte item-level check against cloudfront@v1.67.4 deserializers.go for
List ops not yet covered by the 2026-08-14 two-layer batch (970162d1c):
ListCachePolicies (incl. nested ParametersInCacheKeyAndForwardedToOrigin ->
HeadersConfig/CookiesConfig/QueryStringsConfig>Items>Name), ListOriginRequestPolicies
(same nested shape), ListResponseHeadersPolicies (CorsConfig incl. all four
Items>Header/Method/Origin lists, SecurityHeadersConfig incl.
StrictTransportSecurity/FrameOptions/ReferrerPolicy/ContentTypeOptions,
CustomHeadersConfig>Items>ResponseHeadersPolicyCustomHeader, RemoveHeadersConfig>
Items>ResponseHeadersPolicyRemoveHeader), ListRealtimeLogConfigs, ListVpcOrigins.
Confirmed all wrapper keys and every checked field name are exact-case matches to
the deserializer's `strings.EqualFold` literal, and every list is `Items`-wrapped
with the item type name (or `member` for ListRealtimeLogConfigs) as the direct
child -- no unwrapped-list-deserializer call site exists for any of these ops in
the pinned SDK (grepped `*ListUnwrapped`/`*SummaryListUnwrapped` by name; zero call
sites outside their own func definitions).

**BUG (fixed): `ListRealtimeLogConfigs`' item struct (`handler_realtime_log_configs.go`,
`rlcItemXML`) emitted only ARN/Name/SamplingRate, dropping Fields and EndPoints
entirely from every item** -- absent, not wrong-named. The real per-item
deserializer (`awsRestxml_deserializeDocumentRealtimeLogConfig`) reads both, and
the sibling `GetRealtimeLogConfig` (`realtimeLogConfigResponseXML`) already emits
them correctly from the same backend `RealtimeLogConfig.Fields`/`.EndPoints`
fields -- the exact "Get right, List wrong" trap this issue tracks. Right item
count, permanently blank Fields/EndPoints for every config returned by List
regardless of backend state. Fixed by adding both fields to `rlcItemXML`,
converting `RealtimeLogConfig.EndPoints` to the existing `endPointXML` request
type for reuse on the response side. Test: `TestListRealtimeLogConfigs_ItemShape_RealClient`
(`handler_realtime_log_configs_test.go`), seeds two configs with distinguishable
Fields/EndPoints via the real SDK client and asserts both round-trip correctly
matched by ARN. Verified failing pre-fix by hand-revert (Fields/EndPoints decode
empty).

**BUG (fixed): `ListVpcOrigins`' item struct (`handler_vpc_origins.go`,
`vpcSummaryXML`) tagged its ARN field `xml:"ARN"`, but the real `VpcOriginSummary`
deserializer matches on `"Arn"`** -- a case-only mismatch (decodes today only
because the XML decoder folds case) and inconsistent with this same service's
`vpcOriginResponseXML` (Get), which already used the correct `"Arn"` casing.
**Also missing entirely: OriginEndpointArn and AccountId**, both real
`VpcOriginSummary` members, both backed by real state (`origin.EndpointArn`,
already used correctly in the Get response's nested
`VpcOriginEndpointConfig.Arn`; and `(*InMemoryBackend).AccountID()`, the same
accessor added for `ListDistributionsByOwnedResource`'s `DistributionIdOwner.
OwnerAccountId`). Fixed all three. Status/CreatedTime/LastModifiedTime remain
genuine gaps -- `VpcOrigin` tracks no timestamp or deployment-state field to back
them. Test: `TestListVpcOrigins_ItemShape_RealClient`
(`handler_vpc_origins_test.go`), seeds two origins with distinguishable endpoint
ARNs; verified failing pre-fix by hand-revert (the absent-field assertions fail
outright -- the case-only Arn mismatch alone would NOT have failed this test,
since the real decoder tolerates it; this is recorded to illustrate why the
case-only class needs the byte-for-byte deserializer read, not just a green
round-trip test).

NOT REACHED at item level this pass: ListPublicKeys, ListKeyGroups (re-verify
post-2026-08-14 fix), ListFieldLevelEncryptionConfigs/Profiles,
ListContinuousDeploymentPolicies (re-verify post-2026-08-14 fix),
ListDistributionTenants (re-verify post-2026-08-14 fix), ListTrustStores,
ListAnycastIPLists, ListConnectionGroups/Functions (already deep-audited
2026-08-13, see connection_group_function_swaps row), the ListDistributionsBy*
family (12 ops), ListInvalidations*, ListStreamingDistributions,
ListCloudFrontOriginAccessIdentities, ListDistributions itself (Distribution is
the densest single item shape in this service and was not re-walked field-by-field
this pass).

Gates: `go build ./services/cloudfront/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/cloudfront/...` (pass), `golangci-lint run
./services/cloudfront/...` (0 issues after `fieldalignment -fix
./services/cloudfront/...` reordered the new `rlcItemXML` fields).

## 2026-08-31 per-item exact-case sweep, batch 2 (gopherstack-21my continuation)

Byte-for-byte item-level check against cloudfront@v1.67.4 deserializers.go for the
remainder of this issue's cloudfront "not reached" list: `ListDistributions` itself
and the full twelve-operation `ListDistributionsBy*` family, `ListPublicKeys`,
`ListKeyGroups` (re-verified, no changes needed -- the 2026-08-14 fix holds),
`ListFieldLevelEncryptionConfigs`, `ListFieldLevelEncryptionProfiles`,
`ListDistributionTenants`, `ListTrustStores`, `ListAnycastIpLists`. That completes
every op named in this issue's cloudfront queue.

**BUG (fixed): `ListDistributions`' `distributionSummaryXML`
(`handler_distributions.go`) omitted `ETag` and `Aliases.Items` entirely** --
`ETag` is a real, required `DistributionSummary` member and is backed by
`Distribution.ETag`; `Aliases.Items` (`Items>CNAME`) is backed by
`h.Backend.ListAliases(d.ID)`, which the handler already called to compute
`Aliases.Quantity` but never emitted the underlying strings. Both absent, not
wrong-named.

**BUG (fixed), the sibling-disagreement class in its purest form this pass:
six `ListDistributionsBy*` operations (`ByAnycastIpListId`,
`ByConnectionFunction`, `ByConnectionMode`, `ByTrustStore`, `ByWebACLId`,
`ByRealtimeLogConfig`) share `marshalDistributionList`/`writeDistributionList`,
which built its own separate, far more minimal `DistributionSummary` item
(`ID`/`ARN`/`Status`/`DomainName` only) than the identical wire type
`ListDistributions` builds via `distributionSummaryXML`** -- both real ops
return the exact same `DistributionSummary` shape (confirmed against
`awsRestxml_deserializeDocumentDistributionSummary`), so these six were
missing `Comment`, `Enabled`, `PriceClass`, `HttpVersion`, `LastModifiedTime`,
`IsIPV6Enabled`, `Restrictions`, `ViewerCertificate`, `ETag`, and `Aliases`
entirely -- right item count, drastically impoverished contents, and
inconsistent with this service's own `ListDistributions`. Fixed by factoring
`toDistributionSummaryXML` out of the `ListDistributions` handler and reusing
it in `writeDistributionList`, so both paths build the identical rich shape.
The other six `ListDistributionsBy*` ops (`ByCachePolicyId`, `ByKeyGroup`,
`ByOriginRequestPolicyId`, `ByResponseHeadersPolicyId`, `ByVpcOriginId`,
`ByOwnedResource`) return `DistributionIdList`/`DistributionIdOwnerList`
(bare IDs, not `DistributionSummary`) per their own real deserializers --
re-verified clean, no change needed.

Test: `TestListDistributionsByWebACLId_ItemShape_RealClient`
(`handler_distributions_test.go`), seeds two distributions with distinguishable
Comment/PriceClass/HttpVersion/Aliases, associates both with a web ACL, and
asserts every field round-trips through `ListDistributionsByWebACLId` (the fix
is shared code, so this one op's test covers all six). Verified failing
pre-fix by hand-revert (Comment/PriceClass/HttpVersion/ETag/Aliases all decode
empty). Two pre-existing raw-body substring tests
(`TestListDistributionsByTrustStore`, `TestListDistributionsByConnectionFunction`)
asserted `!strings.Contains(resp, "<Quantity>0</Quantity>")` as their
non-empty-list check; the richer item shape now legitimately contains several
nested zero `Quantity` fields (Origins, Restrictions, Aliases), so both were
narrowed to `<Quantity>0</Quantity><IsTruncated>` (the outer list Quantity is
the only one immediately followed by `IsTruncated` in field order) -- fixed,
not disabled, since the underlying non-empty-list property they check is still
real and still worth checking.

**DIFFERENT AXIS, found but not fixed here (routing bug, not a wire-shape naming
bug): `extractResourceID` (`handler.go`) cuts a URI-label identifier at its
first `/` via `strings.Cut(trimmed, "/")`.** A WAFV2-style `WebACLId` (an ARN,
which contains slashes) passed to `ListDistributionsByWebACLId` gets truncated
to everything before the first slash, so the list silently returns zero
results for a real ARN-shaped ID. Classic (non-ARN) `WebACLId` values are
unaffected, and `ListDistributionsByOwnedResource`'s resource ARN is presumably
exposed to the same bug via the same helper. Verified by reproduction (see
session notes); not fixed here since it is a request-path parsing defect, not
a response-shape naming mismatch -- worth a dedicated issue.

**BUG (fixed): `ListPublicKeys`' `pkSummaryXML` (`handler_key_groups.go`)
omitted `EncodedKey` entirely** -- absent, not wrong-named. The real
`PublicKeySummary` deserializer reads it, and the sibling `GetPublicKey`
(`publicKeyResponseXML`) already emits it correctly from the same backing
`PublicKey.EncodedKey` field. `CreatedTime` remains a genuine gap -- `PublicKey`
tracks no timestamp. Test: `TestListPublicKeys_ItemShape_RealClient`, seeds two
keys with distinguishable Name/Comment, asserts `EncodedKey` round-trips for
both. Verified failing pre-fix by hand-revert.

**BUG (fixed): `ListFieldLevelEncryptionConfigs`' `fleSummaryXML`
(`handler_field_level_encryption.go`) omitted `QueryArgProfileConfig`
entirely** -- absent, not wrong-named. The real `FieldLevelEncryptionSummary`
deserializer reads it (nested `ForwardWhenQueryArgProfileIsUnknown` +
`QueryArgProfiles>Items>QueryArgProfile{QueryArg,ProfileId}` +
`QueryArgProfiles>Quantity`), and the sibling `GetFieldLevelEncryptionConfig`
(`fleConfigInnerXML`) already emits it correctly from the same backing
`FieldLevelEncryption.QueryArgProfiles`/`.ForwardWhenQueryArgProfileIsUnknown`
fields. `ContentTypeProfileConfig` and `LastModifiedTime` remain genuine gaps
-- no backing state. Test:
`TestListFieldLevelEncryptionConfigs_ItemShape_RealClient`, seeds two configs
each referencing a real FLE profile with a distinguishable query-arg, asserts
both round-trip. Verified failing pre-fix by hand-revert (nil-pointer on the
now-absent field).

**BUG (fixed): `ListFieldLevelEncryptionProfiles`' `flePSummaryXML`
(`handler_field_level_encryption.go`) omitted `EncryptionEntities`
entirely** -- same shape as the config-list bug above, against
`FieldLevelEncryptionProfileSummary`'s deserializer, sibling
`GetFieldLevelEncryptionProfile` (`fleProfileConfigInnerXML`) already correct.
`LastModifiedTime` remains a genuine gap. Test:
`TestListFieldLevelEncryptionProfiles_ItemShape_RealClient`, seeds two
profiles with distinguishable encryption entities, asserts both round-trip.
Verified failing pre-fix by hand-revert -- this one failed as a nil-pointer
panic (`item1.EncryptionEntities` decoded as a nil struct pointer on the real
SDK type, not merely an empty slice), a harder failure signature than the
usual empty-slice case, worth noting since it is closer to the "hard decode
error" class than the usual "silent blank" one even though the client itself
did not error.

**BUG (fixed): `ListTrustStores`' `tsSummary` (`handler_trust_stores.go`)
omitted `ETag`, `Status`, and `LastModifiedTime` entirely, and tagged the ARN
field `xml:"ARN"` where the real deserializer matches `"Arn"`** -- a case-only
mismatch (decodes today only because the XML decoder folds case) on top of
three absent-entirely fields, all backed by real state
(`TrustStore.ETag`/`.Status`/`.LastModifiedTime`) and all emitted correctly by
the sibling `GetTrustStore` (`trustStoreXML`). Fixed the case and added all
three fields. Test: `TestListTrustStores_ItemShape_RealClient`, seeds two
trust stores, asserts ARN/ETag/Status/LastModifiedTime all round-trip.
Verified failing pre-fix by hand-revert.

**BUG (fixed): `ListAnycastIpLists`' `ailSummary`
(`handler_anycast_ip_lists.go`) omitted `ETag` and `IpamConfig` entirely** --
both real `AnycastIpListSummary` members; `ETag` is backed by
`AnycastIPList.ETag`, `IpamConfig` by `.IpamCidrConfigs`, and the sibling
`GetAnycastIpList` (`anycastIPListXML`) already emits `IpamConfig` correctly
via the shared `anycastIPListIpamConfigXML` string builder. `IpAddressType`
remains a genuine gap -- `CreateAnycastIpList`'s backend method never accepts
or sets it, so it is always empty regardless of the wire tag now being
present (added anyway, `omitempty`, for when that gap closes). Test:
`TestListAnycastIPLists_ItemShape_RealClient`, seeds two lists with
distinguishable IPAM CIDR configs, asserts ETag and IpamConfig round-trip for
both. Verified failing pre-fix by hand-revert.

**BUG (fixed): `ListDistributionTenants`' `tenantSummaryXML`
(`handler_distribution_tenants.go`) omitted `ETag`, `CreatedTime`, and
`LastModifiedTime` entirely** -- all three real `DistributionTenantSummary`
members, all backed by `DistributionTenant.ETag`/`.CreationTime`/`.LastModifiedTime`,
set at `CreateDistributionTenant`. Unlike every other bug this pass, this one
is **not** a Get-vs-List disagreement -- the singular `distributionTenantXML`
(used by Create/Get/Update/AssociateWebACL) omits all three too, so this is a
pre-existing, service-wide gap on this field set rather than the sibling trap.
Fixing the singular response as well was judged out of this pass's
list-item-shape scope and is recorded here as a related, still-open finding
for the next pass; `Customizations` also remains unaddressed on both sides --
a complex nested union type, deliberately not attempted without deeper
verification of its real shape. Test:
`TestListDistributionTenants_ItemShape_RealClient`, seeds two tenants, asserts
ETag/CreatedTime/LastModifiedTime all round-trip. Verified failing pre-fix by
hand-revert.

**RE-VERIFIED CLEAN, no changes needed:** `ListKeyGroups` (already fixed
2026-08-14, `KeyGroupSummary`/`KeyGroup`/`KeyGroupConfig` field names and
`Items>PublicKey` wrapping all still exact-case correct; `LastModifiedTime` is
a genuine gap -- `KeyGroup` tracks no timestamp).

Wrapping shape checked for every op above, as well as the six
`DistributionIdList`/`DistributionIdOwnerList`-shaped `ListDistributionsBy*`
ops: no call site of any unwrapped-list-deserializer variant exists for any of
them in the pinned SDK.

Gates: `go build ./services/cloudfront/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/cloudfront/...` (pass, including
all seven new real-client tests above), `golangci-lint run
./services/cloudfront/...` (0 issues after `fieldalignment -fix` reordered
`ailSummary` and `queryArgProfileConfigXML`, and two now-stale
`//nolint:dupl` directives on `handleListPublicKeys` and
`handleListFieldLevelEncryptionProfiles` were removed as unused by
`nolintlint` once those two functions' item shapes grew enough to no longer
duplicate their neighbors).

## 2026-08-31: PARITY-gap targeting, batch 5 (gopherstack-6flj/21my)

Queue derivation for this pass: real `List*`/`Describe*` ops in cloudfront@v1.67.4 (42
total) whose full name never appears verbatim anywhere in this file. Mechanical grep gave
5 (the entire `ListDistributionsBy{AnycastIpListId,CachePolicyId,OriginRequestPolicyId,
ResponseHeadersPolicyId,VpcOriginId}` set) — all 5 turned out to be false positives: the
"2026-08-31 per-item exact-case sweep, batch 2" section above already re-verified all 12
`ListDistributionsBy*` ops (just under abbreviated `By*` names, never the full contiguous
op name), and explicitly states it completes this issue's cloudfront queue. Re-derived by
hand instead: read every op still marked "not reached at item level" or only
pagination/spot-checked, cross-referenced against later passes. Genuinely-unswept-at-item-
level ops covered this batch: `ListStreamingDistributions`, `ListCloudFrontOriginAccessIdentities`,
`ListOriginAccessControls`, `ListConflictingAliases`, `ListInvalidations`,
`ListInvalidationsForDistributionTenant`, `DescribeConnectionFunction`. All checked
byte-for-byte against cloudfront@v1.67.4 deserializers.go (`awsRestxml_deserializeDocument*`
switch cases). `ListStreamingDistributions`, `ListCloudFrontOriginAccessIdentities`,
`ListOriginAccessControls`, `ListInvalidations`, `ListInvalidationsForDistributionTenant`,
`DescribeConnectionFunction` came back clean — every emitted field name, nesting, and
LastModifiedTime/CreatedTime timestamp format matched.

Two real bugs found and fixed, both layer-2 (correct wrapper key, wrong/missing per-item
shape), found while diffing `ListConnectionFunctions`/`ListConnectionGroups` against their
already-fixed wrapper-level history from 2026-08-13 (gopherstack-4ara) -- those fixes only
addressed the wrapper, never the per-item field set, and neither had a "not reached"
marker anywhere in this file, so the naive queue-derivation step above would have skipped
them entirely:

1. **`ListConnectionFunctions`' `cfnSummary` (`handler_connection.go`) omitted `CreatedTime`
   and `LastModifiedTime` entirely** -- both real, required `ConnectionFunctionSummary`
   members (cloudfront@v1.67.4 deserializers.go, 8-of-8 case match otherwise), both backed
   by real state (`ConnectionFunction.CreatedTime`/`.LastModifiedTime`), and both already
   emitted correctly by the sibling `DescribeConnectionFunction`
   (`connectionFunctionSummaryXML`) from the same fields -- the "Get right, List wrong"
   trap. Test: `TestListConnectionFunctions_ItemShape_RealClient`
   (`handler_sdk_route_fixes_test.go`), verified failing pre-fix by hand-revert
   (`CreatedTime`/`LastModifiedTime` decode nil).

2. **`ListConnectionGroups`' `cgSummary` (`handler_connection.go`) omitted `AnycastIpListId`,
   `CreatedTime`, `Enabled`, `IsDefault`, and `LastModifiedTime` entirely** -- 5 of the real
   11-member `ConnectionGroupSummary`'s fields, all backed by real state
   (`ConnectionGroup.AnycastIPListID`/`.CreatedTime`/`.LastModifiedTime`/`.Enabled`/
   `.IsDefault`), all already emitted correctly by `GetConnectionGroup`
   (`connectionGroupXML`) from the same fields. Same trap as above, worse: right item
   count, 5 of 11 fields permanently blank/false/zero for every group regardless of
   backend state. Test: `TestListConnectionGroups_ItemShape_RealClient`
   (`handler_sdk_route_fixes_test.go`), verified failing pre-fix by hand-revert.
   **Case-only mismatch fixed alongside** (not independently observable, folded into the
   same struct edit): `cgSummary.ARN` was tagged `xml:"ARN"`; the real
   `ConnectionGroupSummary` deserializer matches on `"Arn"` -- decoded fine either way
   (smithyxml folds case), retagged to match the real casing for consistency with
   `connectionGroupXML`'s own `"ARN"` tag being independently harmless (Get's tag was never
   checked against the real deserializer this pass; not re-verified).

One more real bug found, also layer-2 but a "state tracked, never surfaced" absence rather
than a Get/List sibling gap (`ListConflictingAliases` has no singular `Get` sibling to
compare against):

3. **`ListConflictingAliases`' `conflictingSummary.AccountID` (`handler_distributions.go`)
   was hardcoded to `""`**, despite `h.Backend.AccountID()` already existing and already
   used correctly for the identical real `AccountId` field on `ListVpcOrigins` and
   `ListDistributionsByOwnedResource`'s `DistributionIdOwner.OwnerAccountId`. Real
   `ConflictingAlias.AccountId` (cloudfront@v1.67.4 deserializers.go, 3-of-3 case match
   otherwise: `AccountId`/`Alias`/`DistributionId`) permanently blank regardless of backend
   state. Test: `TestListConflictingAliases_AccountID_RealClient`
   (`handler_distributions_test.go`), verified failing pre-fix by hand-revert.

No hard-decode-error or panic findings this batch. No wrapper-key mismatches this batch
(all 3 fixes are per-item, layer 2). No transpositions, no elements absent from the real
type, no fields existing both nested and top-level. Pages fetched this batch: 0 (module
cache used throughout; no live AWS docs fetched, so no footer-injection risk to report).

Gates (`services/cloudfront/` only, plus repo-wide `go vet`): `go build ./...` clean;
`go vet ./...` clean; `go test -race -count=1 ./services/cloudfront/...` clean;
`golangci-lint run ./services/cloudfront/...` 0 issues. No `nolint` directives in any file
touched this batch (`handler_connection.go`, `handler_distributions.go`,
`handler_distributions_test.go`, `handler_sdk_route_fixes_test.go`).

## 2026-09-07: errtargetaudit class A sweep, 32 findings (gopherstack-lmkr)

`cmd/errtargetaudit` flagged 32 class A findings for cloudfront (real code, present
elsewhere in the SDK, emitted by an op whose own `deserializeOpError<Op>` never declares
it) -- the second largest block in that campaign's corpus. All 32 traced to two root
causes; only one was a real, fixable bug.

**Error-response shape** (verified before touching anything, per this campaign's own
mandate that CloudFront's protocol needs checking, not assumed): cloudfront is REST-XML.
`awsRestxml_deserializeOpError<Op>` (cloudfront@v1.67.4 deserializers.go) reads the HTTP
body via `awsxml.GetErrorResponseComponents`, then switches on `errorCode` against a
`strings.EqualFold("<Code>", errorCode)` case list that is genuinely PER-OPERATION here
(unlike this campaign's JSON-protocol services, where a thin per-op case list was the
norm, cloudfront's real per-op case lists run into the dozens -- e.g. `CreateDistribution`
declares 62 codes). The extraction pattern (`awk "/deserializeOpError<Op>\(/,/^}/" |
grep -oE '"[A-Za-z0-9]+"'`) needed no adjustment for this protocol; scripted per op via a
bash loop over the 32 op names, pasted below per finding. This repo's own error path
(`services/cloudfront/handler_dispatch.go`'s `handleError`) renders
`<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Code></Error></ErrorResponse>`
via `cfErrorXML`, matched against the SDK's `awsxml.GetErrorResponseComponents` shape.

**Root cause A (27 findings, FALSE POSITIVE -- new class, not one of the 3 already known
from sqs/kms): `checkQuantityNode`'s generic Quantity/Items walker
(`services/cloudfront/quantity_validation.go`) runs against these ops' raw request bodies,
but their real cloudfront@v1.67.4 wire shape never contains a `<X><Quantity>..</Quantity>
<Items>..</Items></X>` pattern anywhere -- confirmed by mechanically walking every
`awsRestxml_serialize(Op)?Document*` function transitively reachable from each op's own
request serializer (depth-8 BFS over `serializers.go`, script below) and finding zero
`Local: "Quantity"` elements at all, paired or not, for all 27. `validateQuantities` is
therefore dead code for real client traffic to these ops -- unlike
`CreateCloudFrontOriginAccessIdentity`'s pre-existing "harmless no-op for this shape" note
above (same mechanism, already accepted convention in this file), just not yet
individually called out for these 27. Left unchanged; no fix possible or needed since no
real input can reach the mismatch branch.

  Ops (27): `AssociateDistributionTenantWebACL AssociateDistributionWebACL
  CreateAnycastIpList CreateConnectionGroup CreateDistributionTenant CreateKeyGroup
  CreateKeyValueStore CreateMonitoringSubscription CreateOriginAccessControl
  CreatePublicKey CreateRealtimeLogConfig CreateTrustStore ListDomainConflicts
  PutResourcePolicy TagResource TestConnectionFunction UpdateAnycastIpList
  UpdateConnectionGroup UpdateDistributionTenant UpdateDomainAssociation UpdateKeyGroup
  UpdateKeyValueStore UpdateOriginAccessControl UpdatePublicKey UpdateRealtimeLogConfig
  UpdateTrustStore VerifyDnsConfiguration`. (The tool's original single-cause 31-count
  also included `CreateFunction UpdateFunction CreateConnectionFunction
  UpdateConnectionFunction`, reclassified into root cause B below once the reachability
  check distinguished them; 27 + 4 = 31, the tool's pre-fix total, and 27 + 1 (root cause
  C) = 28, its post-fix total.)

**Root cause B (4 findings, CONFIRMED and FIXED):** `CreateFunction`, `UpdateFunction`,
`CreateConnectionFunction`, `UpdateConnectionFunction` all carry a real, genuinely
reachable Quantity/Items pair -- `FunctionConfig.KeyValueStoreAssociations`
(cloudfront@v1.67.4 types.go/serializers.go's
`awsRestxml_serializeDocumentKeyValueStoreAssociations`: `Quantity *int32` +
`Items []KeyValueStoreAssociation`) -- but none of the four ops' own declared error sets
include `InconsistentQuantities`, only `InvalidArgument` (verified per-op, raw extraction
below). A real client sending a mismatched `KeyValueStoreAssociations` therefore got the
wrong wire code from `validateQuantities`' unconditional `ErrInconsistentQuantities`.
Fixed: `services/cloudfront/quantity_validation.go`'s mismatch-finding logic was split out
of the sentinel-wrapping (`findQuantityMismatch`/`quantityMismatchError`, unchanged
behavior for all other ~40 `validateQuantities` call sites), and a new
`validateFunctionConfigQuantities` wraps the same mismatch with `ErrValidation`
("InvalidArgument") instead. The 4 handlers (`handler_functions.go:50,210`,
`handler_connection.go:84,519`) now call it instead of `validateQuantities`; no other call
site changed.

**Root cause C (1 finding, CONFIRMED, FIXED 2026-09-08, gopherstack-kpk5):**
`UpdateFieldLevelEncryptionConfig`'s rename-collision path (`field_level_encryption.go:182`
pre-fix, `renameInIndex` failing) returned `FieldLevelEncryptionConfigAlreadyExists`, but
that op's own declared error set (`AccessDenied IllegalUpdate InconsistentQuantities
InvalidArgument InvalidIfMatchVersion NoSuchFieldLevelEncryptionConfig
NoSuchFieldLevelEncryptionProfile PreconditionFailed QueryArgProfileEmpty
TooManyFieldLevelEncryptionContentTypeProfiles TooManyFieldLevelEncryptionQueryArgProfiles
UnknownError`) never includes it -- only `CreateFieldLevelEncryptionConfig` does. This IS
reachable by a legitimate client (update an FLE config's `CallerReference` to one already
used by another FLE config), so it was not a false positive -- and gopherstack-kpk5 (filed
title-only as "emits IllegalUpdate for a Name collision that has no real-AWS analogue")
turned out to be wrong about *which* code was emitted -- the code path actually emitted
`FieldLevelEncryptionConfigAlreadyExists`, never `IllegalUpdate` -- but right about the
conclusion: real AWS has no analogue for rejecting this Update at all, under any code.

The original writeup here posed two candidate fixes ("silently allow it" vs. `IllegalUpdate`,
this file's standing code for "not allowed on update") as an unresolved judgement call. A
decisive cross-op comparison resolves it: `CreateDistribution`/`UpdateDistribution` declare
`DistributionAlreadyExists` on Create only, and `CreateStreamingDistribution`/
`UpdateStreamingDistribution` declare `StreamingDistributionAlreadyExists` on Create only --
both real AWS ops with a `CallerReference`-bearing config, both showing the same
Create-checks/Update-doesn't split as `FieldLevelEncryptionConfig`. The contrasting case,
`UpdateFieldLevelEncryptionProfile`, *does* declare `FieldLevelEncryptionProfileAlreadyExists`
in its own error set (botocore `cloudfront/2020-05-31/service-2.json`), and gopherstack's
`renameInIndex` rejection for FLE *profiles* (`field_level_encryption.go:320`) was left
untouched -- it is a correct, declared rejection, not this bug. Four real ops line up
consistently: the two Distribution pairs and FLE profile's Update all confirm Create-time
`CallerReference` collision detection is real but does not carry over to Update; only FLE
*config's* Update was checking it anyway, wrongly. Fix: `UpdateFieldLevelEncryption`
(`field_level_encryption.go`) no longer rejects a `CallerReference` rename that collides
with another config -- it moves the `fieldLevelEncryptionByName` index entry unconditionally
instead of failing when the target name is taken. `IllegalUpdate` was never a fit either:
its own doc comment ("The update contains modifications that are not allowed.",
aws-sdk-go-v2 cloudfront@v1.67.4 `types/errors.go:795`) describes disallowed *field*
mutations (e.g. changing an immutable public key field), not a collision with another
resource's identity -- so neither the code gopherstack was actually emitting nor the code
its own title alleged was the right fix; removing the rejection was.

Regression: `TestUpdateFieldLevelEncryptionConfig_CallerReferenceCollisionAllowed`
(`handler_field_level_encryption_test.go`) creates two FLE configs, renames the second's
`CallerReference` onto the first's, and pins the actual wire response -- `200 OK` with the
new `CallerReference` echoed back, and asserts the body contains neither
`FieldLevelEncryptionConfigAlreadyExists` nor `IllegalUpdate` -- not merely "no error".
Confirmed to fail with `409 FieldLevelEncryptionConfigAlreadyExists` against the pre-fix
code before the fix landed.

**Verdict table** (all 32; raw extraction is `awk "/deserializeOpError<Op>\(/,/^}/"
deserializers.go | grep -oE '"[A-Za-z0-9]+"' | sort -u`, scripted per op in a bash loop):

| op | code | verdict | class |
|---|---|---|---|
| AssociateDistributionTenantWebACL | InconsistentQuantities | FALSE POSITIVE | A: unreachable, no Quantity/Items in real wire shape (declared: AccessDenied, EntityLimitExceeded, EntityNotFound, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, UnknownError) |
| AssociateDistributionWebACL | InconsistentQuantities | FALSE POSITIVE | A (same declared set as above) |
| CreateAnycastIpList | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityAlreadyExists, EntityLimitExceeded, InvalidArgument, InvalidTagging, UnknownError, UnsupportedOperation) |
| CreateConnectionGroup | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityAlreadyExists, EntityLimitExceeded, EntityNotFound, InvalidArgument, InvalidTagging, UnknownError) |
| CreateConnectionFunction | InconsistentQuantities | CONFIRMED, FIXED | B: real KeyValueStoreAssociations pair, wrong code -> now InvalidArgument (declared: AccessDenied, EntityAlreadyExists, EntityLimitExceeded, EntitySizeLimitExceeded, InvalidArgument, InvalidTagging, UnknownError, UnsupportedOperation) |
| CreateDistributionTenant | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, CNAMEAlreadyExists, EntityAlreadyExists, EntityLimitExceeded, EntityNotFound, InvalidArgument, InvalidAssociation, InvalidTagging, UnknownError) |
| CreateFunction | InconsistentQuantities | CONFIRMED, FIXED | B (declared: FunctionAlreadyExists, FunctionSizeLimitExceeded, InvalidArgument, TooManyFunctions, UnsupportedOperation, UnknownError) |
| CreateKeyGroup | InconsistentQuantities | FALSE POSITIVE | A: KeyGroupConfig has Items but no Quantity element at all (declared: InvalidArgument, KeyGroupAlreadyExists, TooManyKeyGroups, TooManyPublicKeysInKeyGroup, UnknownError) |
| CreateKeyValueStore | InconsistentQuantities | FALSE POSITIVE | A: no Quantity/Items in shape (declared: AccessDenied, EntityAlreadyExists, EntityLimitExceeded, EntitySizeLimitExceeded, InvalidArgument, UnknownError, UnsupportedOperation) |
| CreateMonitoringSubscription | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, MonitoringSubscriptionAlreadyExists, NoSuchDistribution, UnknownError, UnsupportedOperation) |
| CreateOriginAccessControl | InconsistentQuantities | FALSE POSITIVE | A (declared: InvalidArgument, OriginAccessControlAlreadyExists, TooManyOriginAccessControls, UnknownError) |
| CreatePublicKey | InconsistentQuantities | FALSE POSITIVE | A (declared: InvalidArgument, PublicKeyAlreadyExists, TooManyPublicKeys, UnknownError) |
| CreateRealtimeLogConfig | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, InvalidArgument, RealtimeLogConfigAlreadyExists, TooManyRealtimeLogConfigs, UnknownError) |
| CreateTrustStore | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityAlreadyExists, EntityLimitExceeded, EntityNotFound, InvalidArgument, InvalidTagging, UnknownError) |
| ListDomainConflicts | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityNotFound, InvalidArgument, UnknownError) |
| PutResourcePolicy | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityNotFound, IllegalUpdate, InvalidArgument, PreconditionFailed, UnknownError, UnsupportedOperation) |
| TagResource | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, InvalidArgument, InvalidTagging, NoSuchResource, UnknownError) |
| TestConnectionFunction | InconsistentQuantities | FALSE POSITIVE | A: request is Stage/EventObject, no FunctionConfig at all (declared: EntityNotFound, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, TestFunctionFailed, UnknownError, UnsupportedOperation) |
| UpdateAnycastIpList | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityNotFound, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, UnknownError, UnsupportedOperation) |
| UpdateConnectionFunction | InconsistentQuantities | CONFIRMED, FIXED | B (declared: AccessDenied, EntityNotFound, EntitySizeLimitExceeded, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, UnknownError, UnsupportedOperation) |
| UpdateConnectionGroup | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityAlreadyExists, EntityLimitExceeded, EntityNotFound, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, ResourceInUse, UnknownError) |
| UpdateDistributionTenant | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, CNAMEAlreadyExists, EntityAlreadyExists, EntityLimitExceeded, EntityNotFound, InvalidArgument, InvalidAssociation, InvalidIfMatchVersion, PreconditionFailed, UnknownError) |
| UpdateDomainAssociation | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityNotFound, IllegalUpdate, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, UnknownError) |
| UpdateFieldLevelEncryptionConfig | FieldLevelEncryptionConfigAlreadyExists | CONFIRMED, FIXED (gopherstack-kpk5) | C: reachable, rejection removed -- real AWS has no CallerReference-collision check on Update at all (declared: AccessDenied, IllegalUpdate, InconsistentQuantities, InvalidArgument, InvalidIfMatchVersion, NoSuchFieldLevelEncryptionConfig, NoSuchFieldLevelEncryptionProfile, PreconditionFailed, QueryArgProfileEmpty, TooManyFieldLevelEncryptionContentTypeProfiles, TooManyFieldLevelEncryptionQueryArgProfiles, UnknownError) |
| UpdateFunction | InconsistentQuantities | CONFIRMED, FIXED | B (declared: FunctionSizeLimitExceeded, InvalidArgument, InvalidIfMatchVersion, NoSuchFunctionExists, PreconditionFailed, UnknownError, UnsupportedOperation) |
| UpdateKeyGroup | InconsistentQuantities | FALSE POSITIVE | A (declared: InvalidArgument, InvalidIfMatchVersion, KeyGroupAlreadyExists, NoSuchResource, PreconditionFailed, TooManyPublicKeysInKeyGroup, UnknownError) |
| UpdateKeyValueStore | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityNotFound, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, UnknownError, UnsupportedOperation) |
| UpdateOriginAccessControl | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, IllegalUpdate, InvalidArgument, InvalidIfMatchVersion, NoSuchOriginAccessControl, OriginAccessControlAlreadyExists, PreconditionFailed, UnknownError) |
| UpdatePublicKey | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, CannotChangeImmutablePublicKeyFields, IllegalUpdate, InvalidArgument, InvalidIfMatchVersion, NoSuchPublicKey, PreconditionFailed, UnknownError) |
| UpdateRealtimeLogConfig | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, InvalidArgument, NoSuchRealtimeLogConfig, UnknownError) |
| UpdateTrustStore | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityNotFound, InvalidArgument, InvalidIfMatchVersion, PreconditionFailed, UnknownError) |
| VerifyDnsConfiguration | InconsistentQuantities | FALSE POSITIVE | A (declared: AccessDenied, EntityNotFound, InvalidArgument, UnknownError) |

**Files changed:** `services/cloudfront/quantity_validation.go` (split
`findQuantityMismatch`/`quantityMismatchError` out of `validateQuantities`; added
`validateFunctionConfigQuantities`), `services/cloudfront/handler_functions.go` (2 call
sites: `handleCreateFunction`, `handleUpdateFunction`),
`services/cloudfront/handler_connection.go` (2 call sites:
`handleCreateConnectionFunction`, `handleUpdateConnectionFunction`), new
`services/cloudfront/function_config_quantities_test.go`.

**Tests** (`Test_FunctionConfigQuantities_WrongCode`, table-driven, 5 subtests): each
mismatch subtest drives the real HTTP handler with a `KeyValueStoreAssociations`
`Quantity`/`Items` mismatch, asserts the XML response `<Code>` is `InvalidArgument` (not
`InconsistentQuantities`), and asserts the resource was neither created (`ListFunctions`/
`ListConnectionFunctions` empty) nor mutated (`GetFunction`/`GetConnectionFunction` ETag
and Comment unchanged); one control subtest (`create_function_match_control_created`)
proves a consistent `Quantity`/`Items` pair still succeeds. `Test_InconsistentQuantities_
EndToEnd` (pre-existing, unmodified) already covers the "an op that legitimately still
emits `InconsistentQuantities`" side for this shared mechanism (`CreateDistribution`,
`CreateCachePolicy`, `CreateInvalidation`, `CreateResponseHeadersPolicy`) -- none of those
ops are in this fix's scope, so no pre-existing test needed correcting.

**Neuter results** (each change reverted individually, confirmed to compile, confirmed a
test then fails, then restored):
| line | compiles reverted? | failing test |
|---|---|---|
| `handler_functions.go:50` (`validateFunctionConfigQuantities`->`validateQuantities`) | yes | `create_function_mismatch_invalid_argument_not_created` |
| `handler_functions.go:210` (same swap) | yes | `update_function_mismatch_invalid_argument_not_mutated` |
| `handler_connection.go:84` (same swap) | yes | `create_connection_function_mismatch_invalid_argument_not_created` |
| `handler_connection.go:519` (same swap) | yes | `update_connection_function_mismatch_invalid_argument_not_mutated` |
| `quantity_validation.go`'s `validateFunctionConfigQuantities` sentinel (`ErrValidation`->`ErrInconsistentQuantities`) | yes | all 3 mismatch subtests (control subtest still passes, as expected) |

Pre-existing tests corrected: none (`Test_InconsistentQuantities_EndToEnd` was already
driving the HTTP handler and asserting on the rendered `<Code>`, not `errors.Is`, and
covers only ops outside this fix's scope).

Gates: `go test -race -count=1 ./services/cloudfront/...` ok (1.5s); `golangci-lint run
services/cloudfront/...` 0 issues (after renaming `quantityMismatch`->
`quantityMismatchError` for `errname` and switching both mismatch wraps to `%w: %w` for
`errorlint` -- `fmt.Errorf` support for multiple `%w` verbs, Go 1.20+, confirmed unchanged
`errors.Is` behavior against `errCodeMapping` by re-running the full suite).
`cmd/errtargetaudit`'s cloudfront count dropped from 32 to 28 after the fix (27 root-cause-A
false positives + the 1 root-cause-C ambiguous finding remain, exactly as expected -- the 4
root-cause-B ops no longer appear).

Fragile: root cause A's "unreachable" verdict rests on the BFS script's depth-8 traversal
of `serializers.go`'s call graph; a future SDK bump that adds a genuine Quantity/Items
field to one of those 27 ops' request shapes would silently re-arm `validateQuantities`
for it with the correct code already in place (harmless), but would NOT itself flip that
op's `deserializeOpError` case list to declare `InconsistentQuantities` -- worth re-running
this audit after any cloudfront SDK version bump, not just trusting this file's snapshot.
