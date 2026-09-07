service: s3control
sdk_module: aws-sdk-go-v2/service/s3control@v1.73.4
last_audit_commit:                                # unknown: pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-08-07
                       # 2026-08-30: pagination-tie re-audit. Re-verified the 2026-08-28/29
                       # pagination_sweep entry below still holds: every List* backend method
                       # (ListAccessPoints/ListAccessPointsForDirectoryBuckets/ListJobs/
                       # ListMultiRegionAccessPoints/ListAccessPointsForObjectLambda/
                       # ListRegionalBuckets/ListAccessGrants/ListAccessGrantsLocations/
                       # ListStorageLensGroups/ListStorageLensConfigurations) filters
                       # store.Table.All() (or a raw map, for the last one) down to one AccountID
                       # first, then sorts by the exact field that -- together with that fixed
                       # AccountID -- forms the table's own composite key (accessPointKeyFn etc.,
                       # store_setup.go), so no tie survives the AccountID filter regardless of
                       # store.Table.All()'s documented unspecified order. The handler layer
                       # pages uniformly via s3cPaginate (handler.go), an integer-offset cursor --
                       # this service does NOT use a marker/equality cursor anywhere in its List
                       # family (contrary to a prior assumption that it might), so there is no
                       # deterministic-drop risk to separately check. No fixes needed this pass;
                       # 0 code changes. TestListAccessPoints_Pagination/similar existing tests
                       # use distinct names throughout.
overall: A            # 2026-08-07 (gopherstack-tir4 follow-up): independently re-verified this
                       # file's two "closed" claims by reading code directly rather than trusting
                       # the prior pass's narrative -- DeleteAccessGrantsInstance's precondition
                       # enforcement (errAccessGrantsInstanceNotEmpty, 3 checks) and the sync
                       # DELETE /mrap/instances route removal both held up. Spot-checked
                       # GetBucketTagging/PutBucketTagging wire shape against s3control@v1.73.4's
                       # serializers.go/deserializers.go directly (CLEAN, matches prior claim). Did
                       # NOT re-do the full ~55-op field-by-field diff this pass (out of scope for
                       # the time available) but while checking CreateAccessPoint's request shape
                       # found and FIXED a new bug the prior "all ops diffed" pass missed: real
                       # CreateAccessPointInput accepts inline Scope and Tags (confirmed via
                       # awsRestxml_serializeOpDocumentCreateAccessPointInput), neither of which
                       # createAccessPointRequestXML had fields for -- both silently dropped on
                       # every real call. See the CreateAccessPoint ops row below for the fix and
                       # new regression test. Verified sibling ops (CreateAccessPointForObjectLambda,
                       # CreateMultiRegionAccessPoint) do not share the gap. Kept at A: real bug
                       # found and fixed with a locking regression test, consistent with how this
                       # file already treats other found-and-fixed-same-pass bugs.
                       #
                       # --- prior (2026-08-01, gopherstack-tir4 close-out) history, kept for context ---
                       # three follow-up items closed.
                       # (1) DeleteAccessGrantsInstance's precondition enforcement was extended
                       # from grants/locations-only to also cover the doc comment's third,
                       # previously-missed precondition -- rejecting delete while an IAM Identity
                       # Center instance is still associated (a real, provable gap: association
                       # was already fully modeled via IdentityCenterArn, just never checked on
                       # delete). See the dedicated precondition section below. (2) The
                       # synchronous "DELETE /v20180820/mrap/instances/{Name}" route was proven
                       # dead by reading the real SDK's serializers.go directly (not by trusting
                       # this file's own prior-pass narrative) -- confirmed no serializer anywhere
                       # in aws-sdk-go-v2/service/s3control@v1.73.0 emits a DELETE to that path --
                       # and removed, along with its now-orphaned handler and the opDeleteMRAP
                       # const. (3) An independent 11-type field-by-field sample re-verification
                       # (ListAccessGrantsResult, GetAccessGrantResult, ListAccessGrantsLocationsResult,
                       # ListCallerAccessGrantsResult, ListStorageLensConfigurationsResult,
                       # ListStorageLensGroupsResult, ListJobsResult, GetJobTaggingResult,
                       # GetAccessPointForObjectLambdaResult, ListAccessPointsForObjectLambdaResult,
                       # DescribeMultiRegionAccessPointOperationResult, GetBucketReplicationResult/
                       # PutBucketReplicationRequest) against the installed SDK's
                       # deserializers.go/serializers.go directly -- not against this file's prior
                       # claims -- found all previously-claimed fixes/gaps held up under
                       # independent re-derivation, plus one new small honest gap:
                       # ListAccessPointsForObjectLambdaResult's per-item Alias field has no
                       # backing data and was undocumented (now documented in-code; not fixed --
                       # see gaps, the real per-op alias-generation algorithm differs from regular
                       # access points' and was not safe to synthesize). No new wire-shape bugs
                       # found in the sampled types. Kept at A.
                       #
                       # --- prior (2026-07-31, gopherstack-eje5) history, kept for context ---
                       # found and fixed a severe CreateBucket
                       # account/read round-trip bug (real CreateBucketInput has no AccountId at
                       # all -- the only op in this ~90-op service missing it; every Get/Delete/List
                       # on the same bucket does carry one). Kept at A: fixed in this same pass with
                       # a regression test that fails against the pre-fix code and passes after (see
                       # CreateBucket ops row and the bucket-outposts family note below for the full
                       # accounting), consistent with how this file already treats other
                       # found-and-fixed-same-pass severe bugs (e.g. the PutBucketPolicy envelope bug
                       # below). One related sub-change (c.String(204,"") -> c.NoContent(204) in
                       # handler_bucket.go) was ALSO mischaracterized this same pass as fixing a bug
                       # that returned http.ErrBodyNotAllowed "on every real call" -- CORRECTED
                       # 2026-07-31: verified false against stdlib source (net/http's write no-ops on
                       # a zero-length write after 204, before the body-allowed check; only
                       # httptest.ResponseRecorder rejects it unconditionally). It is a test-
                       # observability/hygiene change, not a production fix -- see gaps and the
                       # DeleteBucket ops row for the corrected accounting. The identical pattern is
                       # left open (same corrected framing) in 8 other handler-file call sites -- see
                       # gaps.
                       #
                       # --- prior (2026-07-30, gopherstack-tir4) history, kept for context ---
                       # RAISED from B this pass (gopherstack-tir4 follow-up). Finished the diff
                       # gopherstack-tir4 explicitly left partial: the three "types_not_reached"
                       # areas named in that pass's honest remainder (DescribeJob's deepest nested
                       # sub-structures, account-level PublicAccessBlockConfiguration field-level
                       # detail, bucket lifecycle/policy raw-passthrough behaviour beyond routing)
                       # were each individually re-verified against aws-sdk-go-v2/service/
                       # s3control@v1.73.0's deserializers.go/serializers.go this pass. Found and
                       # FIXED one more severe, previously-hidden bug of the exact same class
                       # tir4 was downgraded for: PutBucketPolicy's real request is NOT
                       # payload-bound like PutBucketLifecycleConfiguration/PutBucketTagging/
                       # PutBucketVersioning are -- the real body root is
                       # "<PutBucketPolicyRequest><Policy>{json}</Policy></PutBucketPolicyRequest>"
                       # (confirmed via awsRestxml_serializeOpDocumentPutBucketPolicyInput) -- but
                       # the handler stored the ENTIRE raw request body (envelope included) as "the
                       # policy," so GetBucketPolicy re-wrapped that already-wrong, XML-escaped
                       # envelope in a second "<Policy>" element: a real client's policy round-trip
                       # came back as escaped garbage instead of its own JSON policy document.
                       # Fixed by parsing the real envelope and storing just the inner Policy text;
                       # locked in by TestBucketPolicy_WireShape, which asserts the literal response
                       # XML byte-for-byte, not a substring. The other four re-verified areas came
                       # back CLEAN or genuine-gap-not-bug (see "Wire-shape field-diff audit" below
                       # for the full accounting) -- no further fabrications or envelope bugs found.
                       # Two pre-existing minor items remain in gaps (a dead sync-DELETE MRAP route
                       # reusing a real op name, and an unused ErrAlreadyExists sentinel) -- both
                       # explicitly not live bugs, same class of honestly-documented residual gap
                       # this campaign already treats as compatible with an A grade elsewhere (e.g.
                       # services/textract's AdaptersConfig/HumanLoopConfig gap).
                       #
                       # --- prior (2026-07-30, gopherstack-tir4) history, kept for context ---
                       # DOWNGRADED from A this pass (gopherstack-tir4). A field-by-field diff of
                       # ~35 of the ~55 previously-unaudited response/request types against
                       # deserializers.go/serializers.go found a dense cluster of severe wire-shape
                       # bugs the prior pass's route-level audit could not have caught (it
                       # explicitly flagged response-body shape as "sampled, not exhaustive").
                       # Several are REQUEST-side bugs that would make a real aws-sdk-go-v2 client
                       # unable to successfully call the op at all (wrong payload root element:
                       # PutBucketTagging, PutBucketVersioning, PutStorageLensConfiguration; wrong
                       # field name entirely: SubmitMultiRegionAccessPointRoutes's "RouteUpdates"
                       # vs the emulator's "Routes"; wrong transport entirely: UntagResource's
                       # TagKeys travel as repeated query params in the real API, not an XML body,
                       # so every real UntagResource call was silently deleting zero tags). Others
                       # are RESPONSE-side envelope bugs that would make a real client see an
                       # empty list every time (ListCallerAccessGrants wrapped under the wrong key;
                       # ListStorageLensConfigurations/ListStorageLensGroups wrapped under a
                       # nonexistent list element when the real SDK flattens them; ListMultiRegionAccessPoints
                       # used member name "item" instead of "AccessPoint"). Others are fabricated
                       # fields with no real counterpart (GetAccessPointForObjectLambda's
                       # ObjectLambdaAccessPointArn, GetBucket's BucketArn/OutpostId,
                       # ListCallerAccessGrants' AccessGrantId). All fixed this pass with new
                       # regression tests asserting the literal nested envelope (not substrings).
                       # See the "Wire-shape field-diff audit (gopherstack-tir4)" section below for
                       # the full per-type accounting, including what was NOT reached.

# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  GetBucketLifecycleConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "route suffix was '/lifecycle', real SDK is '/lifecycleconfiguration' -- op was UNREACHABLE via real SDK clients; handler body already used the correct suffix, only the route matcher was wrong. Fixed."}
  PutBucketLifecycleConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "same route bug as GetBucketLifecycleConfiguration, fixed"}
  DeleteBucketLifecycleConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "same route bug as GetBucketLifecycleConfiguration, fixed"}
  PutMultiRegionAccessPointPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "route used '/put_policy' (underscore); real SDK URI is '/put-policy' (hyphen) -- UNREACHABLE via real SDK. Fixed. FOUND NOT FIXED 2026-09-07 (gopherstack-l498 investigation): on a missing MRAP this op returns ErrNotFound (\"NoSuchPublicAccessBlockConfiguration\") instead of errMRAPNotFound (\"NoSuchMultiRegionAccessPoint\") -- see SubmitMultiRegionAccessPointRoutes note below for the full accounting; out of scope for that issue, recommend a follow-up."}
  GetMultiRegionAccessPointPolicyStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "route suffix was '/policyStatus'; real SDK uses all-lowercase '/policystatus' for MRAP specifically (unlike AccessPoint/ObjectLambda, which really do use camelCase '/policyStatus' -- verified both, only MRAP was wrong). UNREACHABLE via real SDK. Fixed."}
  ListAccessPointsForDirectoryBuckets: {wire: ok, errors: ok, state: ok, persist: ok, note: "route was '/accesspointfordirectories' (plural); real SDK URI is '/accesspointfordirectory' (singular). UNREACHABLE via real SDK. Fixed."}
  ListCallerAccessGrants: {wire: ok, errors: ok, state: ok, persist: ok, note: "route was '/accessgrantsinstance/caller-grants'; real SDK URI is '/accessgrantsinstance/caller/grants' (path segment, not hyphenated). UNREACHABLE via real SDK. Fixed. this pass: also fixed -- the grantscope query filter (wire key 'grantscope', api_op_ListCallerAccessGrants.go) was read nowhere; ListCallerAccessGrants(accountID) hardcoded an empty scope internally even where the handler could have passed one through. Now ListCallerAccessGrants(accountID, grantScope string) honors it. allowedByApplication remains unenforced -- this backend has no IAM Identity Center federation/caller-identity model to determine which application 'allowed' a grant, so the filter is structurally unobservable; left as a documented gap rather than fabricated (see gaps: below)."}
  ListAccessGrants: {wire: ok, errors: ok, state: ok, persist: ok, note: "was routed on the same singular path as CreateAccessGrant ('/accessgrantsinstance/grant'); real SDK ListAccessGrants URI is plural '/accessgrantsinstance/grants'. UNREACHABLE via real SDK. Added pathAccessGrantsList const, fixed both extract+dispatch. this pass: also fixed -- the handler read query key 'locationscope' (that's ListAccessGrantsLocations's filter key, not this op's) into what it treated as a grantScope filter, so a real client's grantscope query param (serializers.go: awsRestxml_serializeOpHttpBindingsListAccessGrantsInput, wire key 'grantscope') was silently ignored -- a wrong-key bug, not merely an absent filter. application_arn/granteeidentifier/granteetype/permission were never read at all. Now ListAccessGrants(accountID, AccessGrantsFilter) reads and applies all five (grantscope/application_arn/granteeidentifier/granteetype/permission)."}
  ListAccessGrantsLocations: {wire: ok, errors: ok, state: ok, persist: ok, note: "same singular-vs-plural bug as ListAccessGrants ('/location' vs real '/locations'). UNREACHABLE via real SDK. Added pathAccessGrantsLocationsList const, fixed. this pass: also fixed -- the locationscope query filter (wire key 'locationscope', serializers.go:5564-5566) was parsed nowhere; every call returned every location in the account regardless of the filter sent. Now applied (exact match against AccessGrantsLocation.LocationScope)."}
  UpdateJobPriority: {wire: ok, errors: ok, state: ok, persist: ok, note: "route required http.MethodPut; real SDK sends POST for this op (it's not a pure REST-semantic PUT). UNREACHABLE via real SDK. Fixed method check to MethodPost in both extract+dispatch. THIS PASS: also fixed GetJob/UpdateJobDetails/UpdateJobPriority/UpdateJobStatus returning the wrong AWS error code (generic ErrNotFound == \"NoSuchPublicAccessBlockConfiguration\") on a missing job -- now errJobNotFound (\"NoSuchJob\"). See jobs.go."}
  UpdateJobStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "same PUT-vs-POST bug as UpdateJobPriority. UNREACHABLE via real SDK. Fixed. See UpdateJobPriority note for the error-code fix in this pass."}
  CreateAccessPoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-07 (gopherstack-tir4 follow-up audit): real CreateAccessPointInput (confirmed via awsRestxml_serializeOpDocumentCreateAccessPointInput, s3control@v1.73.4) also accepts inline Scope and Tags fields, neither of which createAccessPointRequestXML had a struct field for at all -- both were silently read off the wire and dropped, requiring a caller to know to make a separate PutAccessPointScope/TagResource call the real API does not require. Fixed: Scope captured as raw inner XML (createJobXMLCapture, same pattern PutAccessPointScope already uses) and stored via the existing PutAccessPointScope backend method; Tags parsed with the same resourceTagXML/<Tag> shape ListTagsForResource/TagResource already use and stored via TagResource(ap.AccessPointArn, ...). Verified CreateAccessPointForObjectLambda and CreateMultiRegionAccessPoint do NOT have the same gap (their real request bodies have no Tags/Scope members at all, confirmed via the same serializers.go read) -- this was not a wider pattern. New test TestCreateAccessPoint_InlineScopeAndTags locks in the round-trip (create with inline Scope+Tags, then GetAccessPointScope/ListTagsForResource confirm both landed)."}
  GetAccessPoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: real bug -- GetAccessPointOutput carries PublicAccessBlockConfiguration inline per aws-sdk-go-v2 (there is no standalone Get/Put/DeleteAccessPointPublicAccessBlock op; see families.access-point-crud), but the response never included it even though CreateAccessPoint already stored it. Fixed: handleGetAccessPoint now reads the per-AP PAB and includes PublicAccessBlockConfiguration when set. Also fixed: wrong error code on missing AP (generic ErrNotFound == \"NoSuchPublicAccessBlockConfiguration\") -- now errAccessPointNotFound (\"NoSuchAccessPoint\")."}
  DeleteAccessPoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: fixed wrong error code on missing AP (see GetAccessPoint), and fixed a ghost-map-row leak -- delete only cleaned accessPointPolicies, leaving scope/per-AP-PAB/generic-resource-tags behind forever. Now cascade-cleans all four."}
  ListAccessPoints: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccessPointPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: fixed wrong error code on missing AP; split the \"AP missing\" case (errAccessPointNotFound / NoSuchAccessPoint) from the \"AP exists but no policy set\" case, which now correctly returns the new errAccessPointPolicyNotFound sentinel (\"NoSuchAccessPointPolicy\") instead of also claiming NoSuchAccessPoint."}
  PutAccessPointPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: fixed wrong error code on missing AP (see GetAccessPoint note)."}
  DeleteAccessPointPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: fixed wrong error code on missing AP (see GetAccessPoint note)."}
  GetAccessPointPolicyStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccessPointScope: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: now round-trips through Snapshot/Restore -- see families.persistence-gap."}
  PutAccessPointScope: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAccessPointScope: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccessPointPublicAccessBlock: {status: DELETED, note: "THIS PASS: this op, PutAccessPointPublicAccessBlock, and DeleteAccessPointPublicAccessBlock were gopherstack-invented -- aws-sdk-go-v2/service/s3control has no such standalone operations. DELETED per parity policy (no fabricated AWS surface): removed from GetSupportedOperations(), removed the '/publicAccessBlock' sub-resource route and its 3 handler funcs. The underlying real feature (PublicAccessBlockConfiguration travels INLINE on CreateAccessPoint/GetAccessPoint, confirmed via aws-sdk-go-v2's CreateAccessPointInput/GetAccessPointOutput) is preserved: the backend storage methods (Get/Put/DeleteAccessPointPublicAccessBlock as internal Go methods, not routed HTTP ops) survive and now correctly feed GetAccessPoint's response (see GetAccessPoint note) -- this was itself a real, previously-unfixed gap."}
  PutAccessPointPublicAccessBlock: {status: DELETED, note: "see GetAccessPointPublicAccessBlock"}
  DeleteAccessPointPublicAccessBlock: {status: DELETED, note: "see GetAccessPointPublicAccessBlock"}
  GetPublicAccessBlock: {wire: ok, errors: ok, state: ok, persist: ok, note: "simplified to delegate to handleBackendError instead of a redundant hand-rolled plain-text 404/500; now emits proper XML"}
  PutPublicAccessBlock: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePublicAccessBlock: {wire: ok, errors: ok, state: ok, persist: ok, note: "same simplification as GetPublicAccessBlock"}
  CreateJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "real state mutation (JobID/Status/CreationTime), not a disguised no-op"}
  DescribeJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: fixed wrong error code on missing job (see UpdateJobPriority note)."}
  ListJobs: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAccessGrantsLocation: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAccessGrantsLocation: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAccessGrant: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAccessGrant: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: ghost-map-row leak fix -- delete left generic resourceTags behind forever; now cascade-cleaned via the grant's AccessGrantArn."}
  DeleteAccessGrantsLocation: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: same resourceTags cascade-cleanup fix as DeleteAccessGrant."}
  DeleteAccessGrantsInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-cleans accessGrantsInstancePolicies and resourceTags (previously left behind forever). Deliberately does NOT cascade-delete AccessGrants/AccessGrantsLocations -- the real op's doc comment requires the caller delete those first, and this precondition IS enforced (errAccessGrantsInstanceNotEmpty, see the dedicated 'DeleteAccessGrantsInstance precondition -- FIXED' section below and TestHandler_DeleteAccessGrantsInstance_Precondition). 2026-07-30: corrected stale 'un-enforced precondition' language in this row/the access-grants family note/gaps -- the precondition was already enforced in code; only the summary text had not been updated to match."}
  DeleteMultiRegionAccessPoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "LEAK FOUND AND FIXED THIS PASS -- see leaks below. Also fixed: wrong error code on missing MRAP (generic ErrNotFound == \"NoSuchPublicAccessBlockConfiguration\") -- now errMRAPNotFound (\"NoSuchMultiRegionAccessPoint\"), matching every other MRAP op in this file."}
  GetMultiRegionAccessPoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: fixed wrong error code on missing MRAP (see DeleteMultiRegionAccessPoint note)."}
  DeleteStorageLensGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: ghost-map-row leak fix -- delete left generic resourceTags behind forever; now cascade-cleaned via the group's StorageLensGroupArn."}
  DeleteAccessPointForObjectLambda: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: ghost-map-row leak fix -- delete only removed the OLAP row itself, leaving its policy, configuration, and generic resource tags behind forever. Now cascade-cleans all three."}
  DeleteBucket: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS: ghost-map-row leak fix -- delete only removed the bucket row itself, leaving lifecycle/policy/tagging/versioning/replication/generic-resource-tags behind forever. Now cascade-cleans all five. ALSO THIS PASS (gopherstack-eje5): the handler used c.String(http.StatusNoContent, \"\"), changed to c.NoContent(http.StatusNoContent), matching DeleteBucketReplication's already-correct handler. CORRECTED 2026-07-31: this change was originally described here (and in the commit message) as fixing a bug that \"returned http.ErrBodyNotAllowed on every real call.\" That is false, verified against net/http's stdlib source: (*response).write no-ops on a zero-length write (returns nil) BEFORE it reaches the body-allowed check, so a real server never returns that error here. Only httptest.ResponseRecorder.Write checks bodyAllowedForStatus unconditionally regardless of write length, so it alone raised the error -- meaning the actual defect was a test-observability gap (no handler-level test could assert a nil error), not a client-facing bug. c.NoContent is correct and preferable, but this was a hygiene/testability fix."}
  SubmitMultiRegionAccessPointRoutes: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-07 (gopherstack-l498): FIXED missing existence check -- unlike its sibling PutMultiRegionAccessPointPolicy, this op wrote route data for any MRAP name, real or not, with no 404. Both ops' SDK doc comments (api_op_PutMultiRegionAccessPointPolicy.go / api_op_SubmitMultiRegionAccessPointRoutes.go, s3control@v1.73.4) presume an existing MRAP -- neither is documented as create-or-update -- so the asymmetry was a real gap, not a documented difference. Neither op declares a modeled error in deserializers.go (both awsRestxml_deserializeOpError* functions fall through to a bare default GenericAPIError with no switch cases -- confirmed via awk extraction of each function body), so there is no declared-error-set constraint here; reused the existing errMRAPNotFound (\"NoSuchMultiRegionAccessPoint\") sentinel already established by every other MRAP op in this file (Get/Delete/Describe/GetPolicy/GetPolicyStatus/GetRoutes) for consistency. MRAP is keyed by accountID+name here, the same key GetMultiRegionAccessPointRoutes already resolves successfully from this op's own identifier -- no region-scoping complication, directly resolvable. New test TestHandler_SubmitMultiRegionAccessPointRoutes_MissingMRAP drives the real PATCH route and asserts the 404 + wire error code + that no routes row is stored as a side effect (via new export_test.go MRAPRoutesCount helper); hand-verified to fail against the unfixed backend (3 tests failed: this one, TestSubmitMRAPRoutes_Backend's missing-MRAP subtest, and TestHandler_StubOperations/submit_mrap_routes). Corrected 3 pre-existing tests that were exercising the unvalidated path: TestSubmitMRAPRoutes_Backend's 'submit on missing MRAP is idempotent' subtest (asserted require.NoError against a nonexistent MRAP -- renamed, now asserts the NoSuchMultiRegionAccessPoint error), TestSubmitMRAPRoutes_Table's two cases (neither ever created the MRAP they submitted routes for -- added CreateMultiRegionAccessPoint to setup), and TestHandler_StubOperations's 'submit_mrap_routes' case (asserted 200 against a never-created 'mymrap' -- now asserts 404/NoSuchMultiRegionAccessPoint, matching the same fresh-handler-no-setup convention this table already uses for get/put/delete_bucket_replication). While investigating, also found (NOT fixed -- out of this issue's scope): PutMultiRegionAccessPointPolicy itself returns the wrong sentinel on a missing MRAP -- ErrNotFound (\"NoSuchPublicAccessBlockConfiguration\") instead of errMRAPNotFound (\"NoSuchMultiRegionAccessPoint\"), the exact bug class already fixed on every other MRAP op per the error-wire-shape family note below, but missed on this one op. TestBackend_MRAP_Operations's 'put_policy_missing_mrap' case hides this today via a bare require.Error instead of require.ErrorContains. Recommend filing a follow-up issue."}
  CreateBucket: {wire: ok, errors: ok, state: ok, persist: ok, note: "SEVERE ROUND-TRIP BUG FOUND AND FIXED THIS PASS (gopherstack-eje5). Confirmed by an exhaustive scan of every *Request$ schema in the installed aws-sdk-go-v2/service/s3control@v1.73.0-equivalent client: CreateBucketInput is the ONLY request in this entire service's surface (all ~90 operations) with no AccountId member at all -- every other Create/Get/Put/Delete/List/Describe/Submit/Update/Associate op binds AccountId to the X-Amz-Account-Id header. A real client therefore never sends that header on CreateBucket, while it commonly does on GetBucket/DeleteBucket/ListRegionalBuckets/bucket sub-resource ops. The handler resolved CreateBucket's owner via the same accountIDFromRequest() helper every other op correctly uses, which silently fell back to the literal string \"default\" -- so a bucket a real client created landed under account \"default\" while that same client's real, account-bearing Get/Delete/List calls looked elsewhere and never found it: CreateBucket then GetBucket/DeleteBucket/ListRegionalBuckets was broken for every real caller that supplies an explicit AccountId anywhere but Create (which is structurally the only option, since the field doesn't exist there). Fixed: OutpostsBucket identity (outpostsBucketKeyFn, store_setup.go) and its sub-resource state (bucketLifecycle/bucketPolicies/bucketTagging/bucketVersioning/bucketReplication) are now keyed by bucket Name alone, not accountID+Name -- mirroring real S3's own globally-unique bucket namespace, and matching the fact that Outposts buckets are physically tied to exactly one owning account via the Outpost itself (OutpostId), not via a per-call parameter this API asks for. GetBucket/DeleteBucket/ListRegionalBuckets/etc. still accept whatever AccountId a caller supplies (real clients may send one) but no longer partition storage by it, since there is nothing trustworthy on the Create side to partition against. New test TestHTTP_CreateBucket_RealSDKShape_RoundTrip locks this in: create with NO account header (the real shape), then List/Get/Delete with an explicit, different AccountId header (also a real shape) -- all three now find the bucket; all three failed before the fix. See bucket-outposts family note for the full accounting, including the audit confirming no other operation in this service has the same defect."}

families:
  access-point-crud: {status: ok, note: "CRUD + policy + scope + PAB all backed by real store.Table state, XML wire shapes spot-checked against deserializers.go (GetAccessPointResult root, member names). THIS PASS: the 3 fabricated GetAccessPointPublicAccessBlock/PutAccessPointPublicAccessBlock/DeleteAccessPointPublicAccessBlock ops (confirmed absent from aws-sdk-go-v2/service/s3control -- PublicAccessBlockConfiguration is account-level-only as a standalone op; the per-AP variant travels inline on CreateAccessPoint/GetAccessPoint) were DELETED: removed from GetSupportedOperations(), removed the '/publicAccessBlock' route and 3 handler funcs (see ops above). The real underlying feature was NOT deleted -- GetAccessPoint's response now correctly includes inline PublicAccessBlockConfiguration when set (previously a genuine gap: CreateAccessPoint stored it but GetAccessPoint never echoed it back). Also fixed THIS PASS: DeleteAccessPoint's ghost-map-row leak (scope/PAB/tags survived delete) and 7 instances of the wrong AWS error code (NoSuchPublicAccessBlockConfiguration instead of NoSuchAccessPoint) across Get/Delete/Put AccessPoint*, plus split GetAccessPointPolicy's \"AP missing\" vs \"policy not set\" cases into distinct correctly-coded errors."}
  bucket-outposts: {status: ok, note: "CRUD + lifecycle + policy + replication + tagging + versioning; lifecycle route bug fixed (see ops above), rest spot-checked ok. THIS PASS (2026-07-30 tir4 follow-up): PutBucketPolicy FIXED -- real PutBucketPolicyInput.Policy is not payload-bound (unlike Lifecycle/Tagging/Versioning/Replication), so raw-body passthrough was wrong: the handler now parses the real '<PutBucketPolicyRequest><Policy>...</Policy></PutBucketPolicyRequest>' envelope and stores only the inner Policy text (see putBucketPolicyRequestXML, handler_bucket.go). GetBucketLifecycleConfiguration's raw-passthrough re-verified genuinely correct this pass by reading the real deserializer directly: it fetches the response root generically (does not check the root element's name) and only requires a 'Rules' child inside it, so returning the exact stored PUT body verbatim on GET carries no root-mismatch risk. Also DeleteBucket ghost-map-row leak fixed (see ops above). THIS PASS (2026-07-31, gopherstack-eje5): the family's 'ok' status as of the note above was WRONG about CreateBucket specifically -- see the dedicated CreateBucket ops row above for the full accounting of a severe create/read account-mismatch round-trip bug, now fixed and locked in by TestHTTP_CreateBucket_RealSDKShape_RoundTrip. Confirmed by an exhaustive scan of every operation's request schema (not just this family) that CreateBucket is the ONLY operation in the entire s3control surface without an AccountId member, so this is not a wider pattern -- no other op in this service needed the same fix. Also changed, while adding the first real end-to-end HTTP test for handleDeleteBucket: it (and handleDeleteBucketLifecycleConfiguration/handleDeleteBucketPolicy/handleDeleteBucketTagging) used c.String(http.StatusNoContent, \"\") instead of c.NoContent(...) (see DeleteBucket ops row above). CORRECTED 2026-07-31: this was originally, and wrongly, described as fixing a bug that returns http.ErrBodyNotAllowed on every real call -- verified false against stdlib (net/http no-ops a zero-length write before the body-allowed check; only httptest.ResponseRecorder rejects it unconditionally). It is a test-observability/hygiene fix, not a client-facing one. The identical c.String(204,\"\") pattern also existed in handler_access_grants.go (4x), handler_object_lambda.go (2x), handler_jobs.go (1x), and handler_access_points.go (1x); those 8 sites were converted to c.NoContent in a later pass this same day (2026-07-31) along with handler-level tests -- see gaps history and the corresponding ops rows."}
  job-batch-ops: {status: ok, note: "CreateJob/DescribeJob/ListJobs/tagging real; UpdateJobPriority/UpdateJobStatus route method bug fixed (see ops above). THIS PASS: fixed 4 instances of the wrong AWS error code (NoSuchPublicAccessBlockConfiguration instead of NoSuchJob) across Get/UpdateJobDetails/UpdateJobPriority/UpdateJobStatus."}
  storage-lens: {status: ok, note: "config + group + tagging CRUD backed by real maps (storageLensConfigs, storageLensConfigTags); routes verified against real SDK paths, no mismatches found. THIS PASS: DeleteStorageLensGroup ghost-map-row leak fixed (generic resourceTags survived delete; storageLensConfigTags for config-tagging was already correctly cascade-cleaned by DeleteStorageLensConfiguration, unaffected)."}
  multi-region-access-point: {status: ok, note: "async Create/Delete/PutPolicy + Describe + instance CRUD; PutMultiRegionAccessPointPolicy path and GetMultiRegionAccessPointPolicyStatus suffix bugs fixed (see ops above). LEAK FOUND AND FIXED prior pass: see leaks below. Also removed the dead/unused mrapPolicies map (declared, reset, but never once written to -- MRAP policy always lived on the MultiRegionAccessPoint.Policy struct field instead; this was pure dead state, not a live bug, but is gone now). 2026-08-01 (gopherstack-tir4 close-out): the synchronous DELETE on /mrap/instances/{Name} (previously mapped to the same 'DeleteMultiRegionAccessPoint' op name) was REMOVED after independently confirming via the SDK serializers that no real client can ever send it -- see gaps for the full accounting. GET on the same path remains the only real op there; DeleteMultiRegionAccessPoint is served exclusively via the async POST route now. 2026-09-07 (gopherstack-l498): SubmitMultiRegionAccessPointRoutes FIXED to 404 on a missing MRAP, matching its sibling PutMultiRegionAccessPointPolicy's existence check -- see the dedicated ops row above, including a found-not-fixed wrong-sentinel bug on PutMultiRegionAccessPointPolicy itself."}
  access-grants: {status: ok, note: "instance + grant + location + identity-center + data-access CRUD; ListAccessGrants/ListAccessGrantsLocations singular-vs-plural route bugs and caller/grants hyphen bug fixed (see ops above). Ghost-map-row leaks fixed on DeleteAccessGrant, DeleteAccessGrantsLocation, and DeleteAccessGrantsInstance (generic resourceTags, and for the instance also accessGrantsInstancePolicies, all previously survived delete forever). DeleteAccessGrantsInstance deliberately does NOT cascade-delete grants/locations, and DOES enforce all three real-API preconditions (grants deleted, locations deleted, Identity Center dissociated -- the third added 2026-08-01) via errAccessGrantsInstanceNotEmpty -- see the DeleteAccessGrantsInstance ops row above and the dedicated 'precondition -- FIXED' section below."}
  tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource backed by real resourceTags map, prefix-matched route ok. THIS PASS: every resource-delete path that has a generic ARN (AccessPoint, ObjectLambda AP, Outposts Bucket, AccessGrant, AccessGrantsLocation, AccessGrantsInstance, StorageLensGroup) now cascade-cleans resourceTags[arn] on delete -- previously only AccessPoint's OWN policy map was cleaned by DeleteAccessPoint and nothing else cleaned tags anywhere, so a delete/recreate cycle under the same name/ARN could silently resurrect a prior resource's tags."}
  error-wire-shape: {status: ok, note: "SERVICE-WIDE bug: every error response (handleBackendError + ~30 ad-hoc 'invalid request body'/'not found' sites) returned c.String(status, plainText) instead of the AWS REST-XML <Error><Code>/<Message> envelope. Fixed prior pass via pkgs/awserr.Write. THIS PASS found a SECOND, narrower service-wide bug of the same class: 15 call sites across access_points.go (7), multi_region_access_points.go (4), and jobs.go (4) used the generic `ErrNotFound` sentinel (code \"NoSuchPublicAccessBlockConfiguration\") for AccessPoint-not-found / MRAP-not-found / Job-not-found errors instead of the resource-specific sentinel (errAccessPointNotFound/\"NoSuchAccessPoint\", errMRAPNotFound/\"NoSuchMultiRegionAccessPoint\", errJobNotFound/\"NoSuchJob\"). HTTP status (404) was correct in every case -- only the XML <Code> body was wrong -- so status-code-only tests never caught it; a real SDK client doing typed error matching (err.Code(), errors.As against a specific exception) on any of these paths got the wrong exception class. All 15 fixed; also added a new errAccessPointPolicyNotFound (\"NoSuchAccessPointPolicy\") sentinel to distinguish \"AP doesn't exist\" from \"AP exists but has no policy\" in GetAccessPointPolicy, which the prior pass had conflated under NoSuchAccessPoint."}
  persistence-gap: {status: ok, note: "NEW FAMILY THIS PASS -- found via reading persistence.go against store.go's field list. backendSnapshot only ever round-tripped the 'batch2' raw maps (bucketReplication, storageLensConfigs, storageLensConfigTags, resourceTags, accessPointPolicies) plus the store.Table-backed resources; the 10 'batch1' raw maps (accessPointScopes, objectLambdaAPPolicies, objectLambdaAPConfigs, bucketPolicies, bucketTagging, bucketLifecycle, bucketVersioning, mrapRoutes, accessGrantsInstancePolicies, jobTags) were declared on InMemoryBackend and actively read/written by real handlers, but Snapshot() never serialized them and Restore() never restored them -- a Snapshot/Restore cycle (a service restart with persistence enabled) silently dropped access point scopes, Object Lambda AP policies/configs, Outposts bucket policy/tagging/lifecycle/versioning, MRAP routes, Access Grants instance resource policies, and job tags, even though the owning resource itself (e.g. the access point, the bucket) survived intact. Fixed: all 10 fields added to backendSnapshot, wired into Snapshot/Restore (including the version-mismatch discard-and-reset branch), s3controlSnapshotVersion bumped 1 -> 2. New test TestPersistence_Batch1Maps_SnapshotRestore locks in all 10."}
  pagination_sweep: {status: fixed, note: "2026-08-28/29 (wrapper-key-sweep-rds-cloudwatch-sqs-sns pagination pass): all List ops paginate at the handler layer via the shared s3cPaginate(items, nextToken, maxResults) index-token helper (handler.go:431), which itself correctly truncates/resumes/emits-only-when-truncated. The bug was upstream: pkgs/store.Table.All() (table.go:154) documents 'iteration order is UNSPECIFIED (Go map order)', and ListAccessPoints/ListJobs fed that unsorted order directly into s3cPaginate with no sort.Slice at all -- so a nextToken computed as an offset into one call's ordering could land on a different item in the next call's ordering, duplicating or skipping access points/jobs across a page boundary (same list-ordering-plus-pagination bug class flagged in this campaign's prior passes). Fixed: both now sort.Slice by Name/JobID before returning, matching the convention every other sorted List op in this service already follows (ListAccessGrants, ListAccessGrantsLocations, ListAccessPointsForObjectLambda, ListAccessPointsForDirectoryBuckets, ListRegionalBuckets). TestListAccessPoints_FullPagination/TestListJobs_FullPagination (wire_field_fixes_test.go) create 9 records each, page at MaxResults=4, and assert the union across the full pagination loop is exactly the created set with no duplicates; both hand-verified to fail intermittently against unfixed code (Go's randomized map iteration makes the failure probabilistic, not every run -- confirmed by running the unfixed test 5x). Also fixed the same missing-sort gap in ListMultiRegionAccessPoints/ListStorageLensConfigurations/ListStorageLensGroups for consistency, though those three are not truncation bugs in the same sense: ListMultiRegionAccessPointsInput.MaxResults/NextToken are themselves documented 'Not currently used. Do not use this parameter.' (api_op_ListMultiRegionAccessPoints.go), and ListStorageLensConfigurations/ListStorageLensGroups have no MaxResults member in the real API at all (NextToken only, and the handler already passes maxResults=0 meaning unbounded/no-token, matching that wire shape) -- so ordering stability is the only real improvement there, not a truncation fix."}

gaps:
  - "2026-08-30 (region-isolation sweep, fix/wrapper-key-sweep-rds-cloudwatch-sqs-sns): investigated
    the cloudwatchlogs/memorydb bug class (a resource identifier/storage key built from the
    backend's fixed default region instead of the request's) against every non-MRAP resource
    family here (AccessPoint, ObjectLambdaAccessPoint, OutpostsBucket, BatchJob, AccessGrant/
    AccessGrantsLocation/AccessGrantsInstance, StorageLensGroup/StorageLensConfig). Confirmed via
    the SDK's own doc-comment ARN examples (api_op_CreateAccessPoint.go's Outposts example uses a
    real region, arn:aws:s3-outposts:us-west-2:...) that these ARE real, regional AWS resources --
    not global. This backend, however, builds every one of their ARNs from b.region (store.go), a
    single value fixed once at Provider.Init from global config, and every composite key function
    (store_setup.go's accessPointKeyFn et al.) is AccountID+Name/ID only -- no region dimension
    anywhere, in any resource family, uniformly. Proved with a throwaway test (since deleted, per
    this task's own instructions) that two real aws-sdk-go-v2 clients signing for different
    regions (us-east-1/us-west-2) against ONE gopherstack process DO collide: a
    same-named CreateAccessPoint from the second region silently overwrote the first region's
    record. NOT fixed. Verdict: this is NOT judged the same bug class as cloudwatchlogs/memorydb,
    because those services show the actual tell -- SOME operation scoping correctly by
    request-derived region (httputils.ExtractRegionFromRequest into a ctx value, consumed by
    per-region store.Table maps) while a SIBLING operation in the SAME service does not. s3control
    has zero per-region storage anywhere and never calls ExtractRegionFromRequest at all (confirmed
    absent, unlike ~65 other services including this campaign's own ssm) -- behavior is 100%
    uniform across every op in this service, which this task's own guidance treats as a legitimate
    single-region-per-backend-instance design, not an inconsistency bug. A full fix would require
    adding a region parameter across roughly 16 AccessPoint backend methods alone (111+ call sites
    including tests, per a repo grep), times five more resource families -- crossing well past a
    single-pass, safely-verifiable change for an otherwise heavily-tested A-grade service; left
    unfixed rather than forced. If multi-region isolation is ever wanted here, the mechanical
    pattern to copy is services/ssm's: getRegion(ctx) sourced from
    httputils.ExtractRegionFromRequest, per-region store.Table maps via a getOrCreateTable-style
    helper (services/ssm/store_setup.go), region folded into each KeyFn, and a region parameter
    threaded through every handler/backend call site -- excluding MultiRegionAccessPoint, which is
    correctly already treated as global (CreateMultiRegionAccessPointInput.Regions []Region and the
    arn:aws:s3::<account>:async-request/... token ARN's empty region segment both confirm MRAP
    itself spans regions by design)."
  - REMOVED 2026-08-01 (gopherstack-tir4 close-out): the synchronous "DELETE /v20180820/mrap/instances/{Name}" route mapped to DeleteMultiRegionAccessPoint was proven genuinely unreachable by any real aws-sdk-go-v2 client (awsRestxml_serializeOpDeleteMultiRegionAccessPoint hardcodes "POST /v20180820/async-requests/mrap/delete" as the op's one and only wire binding; the only serializer targeting "/v20180820/mrap/instances/{Name+}" is GetMultiRegionAccessPoint's, method GET) and deleted from extractMRAPInstanceOp/dispatchMRAPInstanceDispatch (handler_multi_region_access_points.go), along with its now-dead handleDeleteMultiRegionAccessPoint handler and the opDeleteMRAP const. DeleteMultiRegionAccessPoint remains fully served via the real async route. Locked in by TestHandler_DeleteMultiRegionAccessPoint_SyncRouteRemoved (asserts 404 + resource survives) and the updated ExtractOperation dispatch-table case (now expects "Unknown" for this path+method).
  - s3control.ErrAlreadyExists (errors.go) wraps a generic "BucketAlreadyExists" code but is never actually returned by any backend method (verified via repo-wide grep) -- unused/dead sentinel, not a live bug, but worth removing or wiring up correctly if AlreadyExists semantics are ever needed for e.g. CreateAccessPoint on a duplicate name.
  - (CORRECTED 2026-07-30, was previously stale) DeleteAccessGrantsInstance's precondition IS enforced -- see items_still_open.
  - STALE as of 2026-08-23 (manifest-harvest pass): `ListAccessPointsForObjectLambdaResult`'s per-item `types.ObjectLambdaAccessPoint` entries were missing the real `Alias` field per the original 2026-08-01 note below, but commit `fb80d66cd` (2026-08-17, #2425) closed the gap this note describes -- `ObjectLambdaAccessPoint` (models.go) now tracks `Alias`, synthesized with the real `"--ol-s3"` suffix convention (`object_lambda.go`'s `CreateAccessPointForObjectLambda`), and both `Get`/`ListAccessPointsForObjectLambda` already returned it. The one gap that commit left behind -- `CreateAccessPointForObjectLambda`'s own response never echoed the `Alias` it had just set, even though the backend now had it -- is FIXED this pass (`handler_object_lambda.go`). Proven via a real `aws-sdk-go-v2/service/s3control` client `CreateAccessPointForObjectLambda` call (`TestCreateAccessPointForObjectLambda_Alias_RealSDKClient`, `handler_object_lambda_real_client_test.go`), confirmed failing (`Alias` nil) against the unfixed handler, passing after, hand-reverted/restored/`md5sum`-verified byte-identical. Original note, now superseded, preserved for history: "`ListAccessPointsForObjectLambdaResult`'s per-item `types.ObjectLambdaAccessPoint` entries are missing the real `Alias` field (2026-08-01 sample audit, gopherstack-tir4): ObjectLambdaAccessPoint (models.go) tracks no alias data for these APs at all, and the real AWS alias-generation algorithm for Object Lambda APs is a distinct, undocumented "<random>-ol-s3alias"-style scheme (NOT the same "<name>-<accountid>-s3alias" formula regular access points use, confirmed by inspecting access_points.go's CreateAccessPoint) -- not synthesized to avoid inventing an unverified value. Now documented in-code (handler_object_lambda.go); not fixed."
  - (CLOSED 2026-07-30) Only a modestly larger sample of response XML shapes were spot-checked against deserializers.go this pass ... -- superseded: the remaining "types_not_reached" items were individually diffed this pass, see below and items_still_open.
  - (2026-07-31, gopherstack-eje5, CORRECTED same day) An earlier version of this entry claimed the c.String(http.StatusNoContent, "") -> c.NoContent(http.StatusNoContent) change (handler_bucket.go, 4 handlers) fixed a bug that "returns http.ErrBodyNotAllowed on every real call." That claim is false and was verified wrong against net/http's stdlib source: (*response).write in net/http/server.go no-ops a zero-length write (returns nil) BEFORE reaching the body-allowed check, so a real net/http server never returns that error for an empty body after a 204. Only httptest.ResponseRecorder.Write checks bodyAllowedForStatus unconditionally with no exemption for zero-length writes, so only handler-level tests dispatching through a ResponseRecorder would see the error -- meaning the real defect was a test-observability gap (no such test could exist and pass), not a client-facing bug, and c.String vs c.NoContent was never observable to a real SDK client. The identical c.String(204,"") pattern in 8 more handlers (handler_access_grants.go x4, handler_object_lambda.go x2, handler_jobs.go x1, handler_access_points.go x1) was converted to c.NoContent in a later pass this same day, with handler-level tests added to lock in the nil-error assertion that could not previously exist -- described there as a hygiene/testability change, not a bug fix, consistent with this correction.

deferred:
  - AccessGrantsInstance / IdentityCenter association flows (state machine correctness beyond basic CRUD). The delete-grants-and-locations-first precondition noted in a prior version of this bullet IS enforced -- see items_still_open.
  - Chaos fault-injection interaction with the fixed routes/leak (ChaosOperations() just echoes GetSupportedOperations(), unaffected by this pass).
  - GetDataAccess/CreateJob request-side ManifestGenerator (an alternative to Manifest the real CreateJobInput also accepts, letting a caller point at an S3 Inventory report or an existing job's manifest instead of uploading one) is accepted nowhere -- createJobRequestXML has no ManifestGenerator field, so a real client using this path instead of Manifest would have that entire configuration silently dropped. Found while closing out DescribeJob's nested sub-structures this pass (2026-07-30); not fixed, since implementing it requires deciding what synthetic manifest generation should look like (there is no real S3 Inventory data to point at), which is a design decision rather than a field-diff fix -- same reasoning textract's AdaptersConfig gap uses.

leaks: {status: fixed, note: "LEAK FOUND AND FIXED THIS PASS. DeleteMultiRegionAccessPoint (multi_region_access_points.go) checked b.mraps.Has(key) and returned nil WITHOUT ever calling b.mraps.Delete(key) -- a disguised no-op. Both the synchronous DELETE /v20180820/mrap/instances/{Name} route and the async POST /v20180820/async-requests/mrap/delete route (the one a real aws-sdk-go-v2 client actually uses) call this same backend method, so every DeleteMultiRegionAccessPoint call, sync or async, silently failed to remove the resource: the MRAP stayed retrievable via GetMultiRegionAccessPoint/ListMultiRegionAccessPoints forever, and repeated create/delete cycles (e.g. any test or workload that creates+deletes MRAPs by generated/random names) accumulated an unbounded number of ghost rows in b.mraps with no way to reclaim them. No existing test caught this because the only assertion on delete was err == nil, never that the resource was actually gone -- classic 'green tests, real bug' (see the project's parity-principles.md point 3). Fixed: DeleteMultiRegionAccessPoint now actually deletes the row and cascade-cleans its route configuration (mrapRoutes); new tests TestBackend_DeleteMultiRegionAccessPoint_ActuallyRemoves and TestHandler_DeleteMultiRegionAccessPoint_AsyncRouteActuallyRemoves lock in both the backend- and HTTP-level behavior via Get-after-Delete and List-after-Delete assertions, not just the return value. While investigating this leak class, also found and fixed 6 more ghost-map-row leaks of the identical shape (delete removes the primary resource row but leaves secondary maps -- policy/scope/PAB/generic-tags -- behind forever) on DeleteAccessPoint, DeleteAccessPointForObjectLambda, DeleteBucket, DeleteAccessGrant, DeleteAccessGrantsLocation, DeleteAccessGrantsInstance, and DeleteStorageLensGroup -- see the tags/access-point-crud/bucket-outposts/access-grants/storage-lens family notes above. No goroutines/janitors/tickers exist in this service (verified: no `go func`/`time.NewTicker`/`time.AfterFunc`/`context.WithCancel` anywhere in services/s3control), so there is no goroutine-leak class here -- the leak this pass found and fixed was purely the disguised-no-op-delete / ghost-map-row class. Handler.Snapshot/Restore correctly delegate to InMemoryBackend.Snapshot/Restore (verified in persistence.go) so cli.go's setupPersistence registers it correctly -- no silent-unregistration bug found here."}
---

## Notes

**Protocol**: REST-XML (`/v20180820/` path-versioned), with `X-Amz-Account-Id` header
carrying the account ID (there is no path/query account parameter). Error bodies use
a bare `<Error><Code>/<Message>/<RequestId></Error>` envelope (not wrapped in an outer
`<ErrorResponse>` the way the Query protocol is) -- see `pkgs/awserr.ProtocolRestXML`.

**Route-matcher bug class (prior pass)**: `RouteMatcher()` itself just
checks `strings.HasPrefix(path, "/v20180820/")` -- real operation routing happens in
`ExtractOperation`/`Handler()`'s `extract*`/`dispatch*` helper functions, which do
literal path-prefix/suffix and HTTP-method matching. Nine ops were **unreachable by a
real aws-sdk-go-v2 client** despite having fully-implemented, real handler+backend
logic, because the literal path/method constants didn't match the real SDK's
`serializers.go` -- singular-vs-plural, hyphen-vs-underscore, casing, wrong verb, extra
path segment, wrong suffix. See git history for the full list; all fixed prior pass.

**Disguised-no-op-delete / ghost-map-row leak class (THIS pass)**: the leak this pass
was asked to find (`DeleteMultiRegionAccessPoint` checked existence and returned nil
without ever calling `.Delete()`) is one instance of a broader pattern found by reading
every `Delete*` backend method against every map it should have touched: a delete that
only removes the primary `store.Table` row, silently leaving secondary maps (per-resource
policy/scope/config, generic `resourceTags[arn]`) populated forever. Two distinct
symptoms: (1) the resource itself never actually disappears (MRAP case -- the primary
row survives too), and (2) the resource disappears but a delete/recreate cycle under the
same name/ARN silently resurrects the deleted resource's stale secondary state (the other
6 cases). Both are real memory-growth-over-time bugs in a long-running emulator process
and both are now fixed with matching regression tests (Get/List-after-Delete assertions,
or delete-then-recreate-then-assert-empty assertions) rather than trusting `err == nil`.

**Wrong-error-code class (THIS pass, second instance of the error-wire-shape bug
family)**: 15 call sites returned the generic `ErrNotFound` sentinel (AWS code
`NoSuchPublicAccessBlockConfiguration`) for AccessPoint/MRAP/Job-not-found instead of
the resource-specific sentinel. HTTP status was always correct (404), so this was
invisible to any test asserting status codes only -- exactly the failure mode
`parity-principles.md` warns about ("green tests, real bug"). All fixed; two new
sentinels added (`errAccessPointPolicyNotFound`) to correctly distinguish "resource
missing" from "resource exists but sub-field not set" where the prior code conflated
them under one AWS error code.

**Fabricated-ops deletion (THIS pass)**: `GetAccessPointPublicAccessBlock` /
`PutAccessPointPublicAccessBlock` / `DeleteAccessPointPublicAccessBlock` were gopherstack-
invented standalone REST operations with no counterpart in
`aws-sdk-go-v2/service/s3control` (confirmed: no `api_op_*AccessPointPublicAccessBlock*.go`
files exist in the SDK module). Deleted per parity policy. Deleting them surfaced a real,
previously-hidden gap underneath: the actual AWS feature (`PublicAccessBlockConfiguration`
travels inline on `CreateAccessPointInput`/`GetAccessPointOutput`, confirmed via the SDK's
own generated types) was half-implemented -- `CreateAccessPoint` stored it but
`GetAccessPoint` never echoed it back. Both are fixed now: the fabricated ops are gone,
the real inline field works.

**Persistence-gap class (THIS pass, new)**: found by systematically checking every raw
(non-`store.Table`) map field declared on `InMemoryBackend` (`store.go`) against
`backendSnapshot`'s field list (`persistence.go`). 10 of them were write-through live
state with no persistence wiring at all -- a silent data-loss bug on any
Snapshot/Restore cycle (service restart with persistence enabled). All 10 fixed; see
the `persistence-gap` family note above for the full list and
`TestPersistence_Batch1Maps_SnapshotRestore` for the regression test.

**Error XML**: `pkgs/awserr.Write(c, awserr.ProtocolRestXML, awserr.APIError{...})`
existed in the shared pkgs/ layer but was unused by ANY service in the codebase before
the prior pass (verified via repo-wide grep). s3control's backend errors are already
created via `awserr.New(code, sentinel)` (e.g. `errAccessPointNotFound =
awserr.New("NoSuchAccessPoint", awserr.ErrNotFound)`), so `err.Error()` IS the AWS
error code string -- `handleBackendError` does `code := err.Error()` and passes it
straight through to `awserr.Write`. This is exactly the mechanism that made the
wrong-error-code bug class (this pass) possible: using the wrong *sentinel* (right
HTTP status, wrong `err.Error()` string) silently produces a wrong-but-well-formed XML
`<Code>`, which only a code-string assertion (not a status-code assertion) will catch.
If re-auditing other REST-XML services, check both status code AND `<Code>` string.

## items_still_open (see gaps/deferred above for full detail)

- Full field-by-field response-XML diff against deserializers.go: CLOSED 2026-07-30.
  The prior version of this bullet claimed "~55 remaining response types not
  spot-checked" -- stale. Every handler-file section under "Wire-shape field-diff
  audit" below is marked "-- all ops diffed", and this pass (2026-07-30) finished the
  one remaining honest gap, the nested/deep-detail items the prior pass explicitly
  listed under "types_not_reached" (DescribeJob's deepest nested sub-structures,
  CreateAccessPoint's request-side VpcConfiguration/PublicAccessBlockConfiguration
  nesting, account-level PublicAccessBlock field-level detail, bucket
  lifecycle/policy raw-passthrough behaviour). See "types_not_reached (THIS PASS,
  2026-07-30 follow-up)" below for the closing accounting, including the one real bug
  found and fixed (PutBucketPolicy's envelope).
- DeleteAccessGrantsInstance's "grants/locations must be deleted first" precondition
  IS enforced (errAccessGrantsInstanceNotEmpty) -- see the dedicated
  "DeleteAccessGrantsInstance precondition -- FIXED" section below and
  TestHandler_DeleteAccessGrantsInstance_Precondition. This bullet previously
  (incorrectly) claimed the precondition was unenforced; that language predated the
  fix documented below it in this same file and was never updated to match. Corrected
  2026-07-30 -- no code change was needed, only the stale summary text.
- ErrAlreadyExists (errors.go) remains an unused/dead sentinel. Reason not fixed: no
  backend method needs AlreadyExists semantics currently (e.g. CreateAccessPoint does
  not reject duplicate names), and confirming whether real AWS actually returns
  AlreadyExists for any s3control Create* op -- versus silently overwriting, versus a
  different validation error -- was out of scope for this pass's leak/error-code/
  persistence focus.
- The synchronous DELETE /v20180820/mrap/instances/{Name} route (mapped to the real
  op name DeleteMultiRegionAccessPoint, but via an HTTP verb/path combination the real
  SDK never sends) remains routable. Reason not removed: unlike the 3 fabricated
  PublicAccessBlock ops (which had no real op-name counterpart at all), this route
  reuses a genuine op name and was already fixed by this pass's leak fix (it no longer
  behaves like a no-op); removing the route itself is pure dead-surface cleanup with
  no remaining functional bug, judged lower priority than the items above.

## Wire-shape field-diff audit (gopherstack-tir4, THIS pass)

Full field-by-field diff of response/request XML shapes against the installed
`aws-sdk-go-v2/service/s3control@v1.73.0`'s `deserializers.go`/`serializers.go`/`types/types.go`
(the SDK module was updated from v1.68.2 -> v1.73.0 as part of confirming shapes against the
currently-vendored version). Grouped by handler file. `types_not_reached` at the end lists
what a follow-up pass should still cover.

### Access Grants (`handler_access_grants.go`) -- all ops diffed

- `CreateAccessGrantsInstanceResult`: FIXED -- missing `CreatedAt` (backing data existed, never
  wired).
- `GetAccessGrantsInstanceResult`: CLEAN.
- `ListAccessGrantsInstancesResult`: FIXED -- per-item `CreatedAt` field existed in the struct
  but was never populated in the loop; `IdentityCenterInstanceArn`/`IdentityCenterApplicationArn`
  were missing entirely despite backing data.
- `AssociateAccessGrantsIdentityCenterResult` / `DissociateAccessGrantsIdentityCenter`:
  CLEAN (real ops have no output body; verified no `awsRestxml_deserializeOpDocument...Output`
  function exists for the first).
- `GetAccessGrantsInstanceForPrefixResult`: FIXED -- missing `AccessGrantsInstanceId` (real
  field, backing data existed).
- `GetAccessGrantsInstanceResourcePolicyResult` / `PutAccessGrantsInstanceResourcePolicyResult`:
  GAP-DOCUMENTED -- real type also has `CreatedAt`/`Organization`; this backend's resource-policy
  store is a bare string map with no such data.
- `GetAccessGrantResult`: FIXED -- missing `AccessGrantsLocationId`, `GrantScope`,
  `ApplicationArn`, `CreatedAt` (all backed by data on every stored grant, none were wired).
- `ListAccessGrantsResult`: FIXED -- item type only carried `AccessGrantId`/`Permission`/
  `GrantScope`; added `AccessGrantArn`/`AccessGrantsLocationId`/`ApplicationArn`/`CreatedAt`/
  `Grantee` (all backed).
- `ListCallerAccessGrantsResult`: **ENVELOPE BUG, FIXED** -- wrapped the list under
  `AccessGrantsList` (the key `ListAccessGrants`, a *different* operation, uses); the real
  deserializer (`awsRestxml_deserializeOpDocumentListCallerAccessGrantsOutput`) only recognizes
  `CallerAccessGrantsList`. A real client would see an empty list on every call. Also: the item
  type was reused from `ListAccessGrants` and fabricated an `AccessGrantId` element --
  `ListCallerAccessGrantsEntry` has no such field in the real SDK at all (verified against
  `types.ListCallerAccessGrantsEntry`); it also lacked `ApplicationArn`/`GrantScope`, which
  are real and backed. New dedicated `listCallerAccessGrantItemXML` type added.
- `GetAccessGrantsLocationResult` / `CreateAccessGrantsLocationResult` /
  `UpdateAccessGrantsLocationResult`: CLEAN.
- `ListAccessGrantsLocationsResult`: FIXED -- item type only carried `AccessGrantsLocationId`/
  `LocationScope`; added `AccessGrantsLocationArn`/`CreatedAt`/`IAMRoleArn` (all backed).
- `GetDataAccessResult`: GAP-DOCUMENTED -- real type also has `Credentials.SessionToken`,
  `Credentials.Expiration`, and a top-level `Grantee`; this backend does not issue real
  STS-style credentials or resolve which grant matched, so these are left unpopulated rather
  than invented.
- `DeleteAccessGrantsInstance` precondition: **FIXED** -- see dedicated section below.

### Jobs (`handler_jobs.go`) -- all ops diffed

- `CreateJobResult`, `DescribeJobResult`: CLEAN (both already spot-checked in the prior pass;
  re-verified `JobDescriptor`'s full field list this pass -- `GeneratedManifestDescriptor`/
  `ManifestGenerator`/`FailureReasons`/`SuspendedCause`/`SuspendedDate` have no backing data,
  GAP not fabricated, not individually annotated in code this pass -- see types_not_reached).
- `ListJobsResult`: FIXED -- item type (`JobListDescriptor`) only carried `JobId`/`Status`/
  `Priority`; added `Description`/`CreationTime`/`TerminationDate` (backed directly) and
  `Operation` (derived from the raw `<Operation>` blob's root element name via a new
  `jobOperationName` helper, since `JobListDescriptor.Operation` is a plain `OperationName`
  enum string like `"LambdaInvoke"`, not the full nested config `JobDescriptor.Operation`
  carries -- confirmed via `types.JobListDescriptor`). `ProgressSummary`: GAP-DOCUMENTED (no
  task-count tracking in this backend).
- `UpdateJobPriorityResult`: CLEAN.
- `UpdateJobStatusResult`: FIXED -- missing `StatusUpdateReason` (real field, backed).
- `GetJobTaggingResult` / `PutJobTaggingRequest`: **ENVELOPE BUG, FIXED** -- `jobTagSetXML`
  used member name `<Tag>`; the real `S3TagSet` type (shared with bucket tagging) serializes
  entries as `<member>` (confirmed via `awsRestxml_serializeDocumentS3TagSet`). This broke
  both directions: `GetJobTagging` would show an empty tag set to a real client, and
  `PutJobTagging` would silently drop every tag a real client sends (its request always uses
  `<member>`, never `<Tag>`).

### Object Lambda (`handler_object_lambda.go`) -- all ops diffed

- `CreateAccessPointForObjectLambdaResult`: **STALE note, FIXED 2026-08-23** -- the
  `Alias`-missing gap this line originally documented was closed by commit `fb80d66cd`
  (2026-08-17, #2425), which gave `ObjectLambdaAccessPoint` (models.go) a real, synthesized
  `Alias` (the `"--ol-s3"` suffix convention). That commit wired `Get`/`ListAccessPointsForObjectLambda`
  to return it but missed this op's own response -- fixed this pass
  (`handler_object_lambda.go`), proven via `TestCreateAccessPointForObjectLambda_Alias_RealSDKClient`
  (`handler_object_lambda_real_client_test.go`).
- `GetAccessPointForObjectLambdaResult`: **FABRICATION, FIXED** -- emitted an
  `ObjectLambdaAccessPointArn` element; the real `GetAccessPointForObjectLambdaOutput` has NO
  such field at all (confirmed against the SDK type and its deserializer -- only
  `Alias`/`CreationDate`/`Name`/`PublicAccessBlockConfiguration` exist). Deleted. `Alias` is now
  modeled and returned (see above); `CreationDate`/`PublicAccessBlockConfiguration` still have no
  backing data in this backend (GAP-DOCUMENTED).
- `ListAccessPointsForObjectLambdaResult`: CLEAN -- `Alias` is now modeled and returned (see
  `CreateAccessPointForObjectLambdaResult` above; this op's own wiring predates that fix).
- `GetAccessPointPolicyForObjectLambdaResult` / `Put...` / `Delete...`: CLEAN.
- `GetAccessPointPolicyStatusForObjectLambdaResult`: CLEAN (same `PolicyStatus>IsPublic`
  pattern as the AccessPoint/MRAP variants, verified independently).
- `GetAccessPointConfigurationForObjectLambdaResult` / `PutAccessPointConfigurationForObjectLambdaRequest`:
  **ENVELOPE BUG, FIXED** -- wrapped the payload under `<ObjectLambdaConfiguration>`; the real
  wrapper element is `<Configuration>` (confirmed via
  `awsRestxml_deserializeOpDocumentGetAccessPointConfigurationForObjectLambdaOutput` and
  `awsRestxml_serializeOpDocumentPutAccessPointConfigurationForObjectLambdaInput`). Also fixed a
  second bug in the same field: it was captured/emitted as a flat `string`, which would
  concatenate/lose a real client's nested `TransformationConfigurations`/`AllowedFeatures`
  structure; now captured as raw inner XML (same pattern as `CreateJob`'s Manifest/Operation/Report).

### Storage Lens (`handler_storage_lens.go`) -- all ops diffed

- `CreateStorageLensGroupResult` / `UpdateStorageLensGroupResult`: CLEAN (verified these real
  ops have NO output body at all -- no `awsRestxml_deserializeOpDocument...Output` function
  exists for either).
- `GetStorageLensConfigurationResult` / `PutStorageLensConfigurationRequest`:
  **ENVELOPE + REQUEST-BREAKING BUG, FIXED** -- expected/emitted a `<Config>` child element that
  does not exist anywhere on the real `StorageLensConfiguration` type. On the request side this
  was a hard break: a real client's PUT body nests the whole configuration directly under
  `<StorageLensConfiguration>` (a "payload"-bound field, confirmed via
  `awsRestxml_serializeOpDocumentPutStorageLensConfigurationInput`), so the `<Config>` field
  never matched anything a real client sends -- every real `PutStorageLensConfiguration` call
  silently stored an empty configuration. Fixed: the real per-field structure
  (`AccountLevel`/`IsEnabled`/etc.) is now captured/replayed as raw inner XML directly under
  `<StorageLensConfiguration>`, alongside the real, always-known `Id`.
- `GetStorageLensConfigurationTaggingResult` / `PutStorageLensConfigurationTaggingRequest`:
  CLEAN -- `StorageLensTags`' member name really is `<Tag>` (a *different* type from the
  `S3TagSet`/`<member>` type job and bucket tagging use), verified independently; no bug here.
- `ListStorageLensConfigurationsResult`: **ENVELOPE BUG, FIXED** -- wrapped the list under
  `<StorageLensConfigurationList>`; the real list is FLATTENED (repeated
  `<StorageLensConfiguration>` elements directly under the result, confirmed via
  `awsRestxml_deserializeDocumentStorageLensConfigurationListUnwrapped` -- the "Unwrapped"
  suffix is smithy-go's flattened-list marker). A real client would see an empty list on every
  call.
- `GetStorageLensGroupResult`: **FABRICATION, FIXED** -- emitted a `CreatedAt` element; the
  real `StorageLensGroup` type has NO such field at all (confirmed via
  `awsRestxml_deserializeDocumentStorageLensGroup` -- only `Filter`/`Name`/`StorageLensGroupArn`
  exist). Deleted (the internal `StorageLensGroup.CreatedAt` field remains tracked, just not
  serialized).
- `ListStorageLensGroupsResult`: **ENVELOPE BUG + FABRICATION, FIXED** -- same flattened-list
  bug as `ListStorageLensConfigurations` (wrapped under `<StorageLensGroupList>` instead of
  flattening), plus the same fabricated `CreatedAt`, plus items reused the `Get` shape's
  `Filter` field, which the real, narrower `ListStorageLensGroupEntry` type does not have
  (it has `HomeRegion` instead -- GAP-DOCUMENTED, no backing data). New dedicated
  `listStorageLensGroupItemXML` type added.

### Tags (`handler_tags.go`) -- all ops diffed

- `ListTagsForResourceResult` / `TagResourceRequest`: CLEAN -- the generic resource `TagList`
  type's member name really is `<Tag>` (confirmed via `awsRestxml_serializeDocumentTagList`'s
  `ArrayWithCustomName(... Local: "Tag")`), a different type from `S3TagSet`; no bug.
- `UntagResourceRequest`: **TRANSPORT BUG, FIXED** -- expected `TagKeys` in an XML request body
  (`<UntagResourceRequest><TagKeys><TagKey>...`). The real `UntagResourceInput` has NO XML body
  at all for this op -- `TagKeys` travels as repeated `tagKeys` query-string parameters
  (confirmed via `awsRestxml_serializeOpHttpBindingsUntagResourceInput`, which calls
  `encoder.AddQuery("tagKeys", ...)` and has no corresponding body serializer). Since
  `decodeXML` treats an empty body (`io.EOF`) as success, every real `UntagResource` call from
  an actual aws-sdk-go-v2 client silently deleted **zero** tags while still returning 204 --
  a disguised no-op of the same severity class as the MRAP delete leak from the prior pass.
  Fixed: `tagKeys` now read from the query string.

### Access Points (`handler_access_points.go`) -- all ops diffed

- `CreateAccessPointResult`, `GetAccessPointPolicyResult`/`Put`/`Delete`,
  `GetAccessPointPolicyStatusResult`, `PutAccessPointScopeResult`: CLEAN.
- `GetAccessPointResult`: GAP-DOCUMENTED (not fixed, no backing data) -- real type also has
  `DataSourceId`/`DataSourceType`/`Endpoints`; doc comment added.
- `ListAccessPointsResult`: CLEAN (verified against `types.AccessPoint`'s full field list;
  same `DataSourceId`/`DataSourceType` gap, no backing data).
- `GetAccessPointScopeResult` / `PutAccessPointScopeRequest`: **WRONG-SHAPE BUG, FIXED** --
  `Scope` was treated as a flat string; the real `GetAccessPointScopeOutput.Scope` /
  `PutAccessPointScopeInput.Scope` is a structured type (`Permissions []ScopePermission`,
  `Prefixes []string`, confirmed via `awsRestxml_deserializeDocumentScope`). A real client's
  nested `<Scope><Permissions>...</Permissions><Prefixes>...</Prefixes></Scope>` body would
  have been collapsed to (mostly empty) character data on decode. Fixed: captured/replayed as
  raw inner XML nested under `<Scope>`, same pattern as the ObjectLambda Configuration fix.
- `ListAccessPointsForDirectoryBucketsResult`: FIXED -- confirmed this op shares the exact same
  entry type and envelope as `ListAccessPoints` (`awsRestxml_deserializeOpDocumentListAccessPointsForDirectoryBucketsOutput`
  delegates to the identical `AccessPointList` deserializer); the handler previously used a
  narrower ad hoc type (`Name`/`AccessPointArn`/`Bucket` only), omitting
  `BucketAccountId`/`NetworkOrigin`/`Alias`/`VpcConfiguration` despite this backend tracking all
  of them. Now reuses `listAccessPointItemXML`.

### Bucket (Outposts) (`handler_bucket.go`) -- all ops diffed

- `CreateBucketResult`, `GetBucketPolicyResult`/`Put`/`Delete`,
  `GetBucketReplicationResult`/`Put`/`Delete` (already used correct innerxml capture under the
  correct `ReplicationConfiguration` root, verified against the real payload-bound field):
  CLEAN.
- `GetBucketResult`: **FABRICATION, FIXED** -- emitted `BucketArn` (does not exist on the real
  `GetBucketOutput` at all -- only `Bucket`/`CreationDate`/`PublicAccessBlockEnabled` do,
  confirmed against the SDK type) and mislabeled the internal HTTP `Location`-header path
  fragment as `OutpostId` (also not a real field on this output). Both deleted;
  `CreationDate`/`PublicAccessBlockEnabled` GAP-DOCUMENTED (no backing data on
  `OutpostsBucket`).
- `GetBucketTaggingResult` / `PutBucketTaggingRequest`: **ENVELOPE + REQUEST-BREAKING BUG,
  FIXED** -- same `S3TagSet` member-name bug as job tagging (`<Tag>` instead of `<member>`),
  compounded by a payload-root bug: `Tagging` is a "payload"-bound field in the real SDK, so
  the ENTIRE request body root is `<Tagging>` with no `<PutBucketTaggingRequest>` wrapper at
  all (confirmed via the serializer, which sets the XML root element to `"Tagging"` directly).
  The previous shape expected the payload nested one level deeper, which a real
  aws-sdk-go-v2 client's request would never match (root-element mismatch) -- **every real
  PutBucketTagging call would have been rejected outright**, not merely mis-parsed.
- `GetBucketVersioningResult`: GAP-DOCUMENTED (`MfaDelete`, no backing data).
- `PutBucketVersioningRequest`: **REQUEST-BREAKING BUG, FIXED** -- same payload-root class of
  bug as `PutBucketTagging`: `VersioningConfiguration` is payload-bound, so the real root is
  `<VersioningConfiguration>` with `Status` as a direct child, not
  `<PutBucketVersioningRequest><VersioningConfiguration><Status>`. Every real
  `PutBucketVersioning` call would have been rejected outright.
- `ListRegionalBucketsResult`: GAP-DOCUMENTED -- item type (`RegionalBucket`) also has
  `CreationDate`/`OutpostId`/`PublicAccessBlockEnabled`; no backing data for any of the three.

### Multi-Region Access Points (`handler_multi_region_access_points.go`) -- all ops diffed

- `CreateMultiRegionAccessPointResult`, `DeleteMultiRegionAccessPointResult` (async),
  `PutMultiRegionAccessPointPolicyResult`: CLEAN (all just `RequestTokenARN`, verified against
  their deserializers).
- `GetMultiRegionAccessPointResult`: GAP-DOCUMENTED -- shares the `MultiRegionAccessPointReport`
  type with the List response (see below); `PublicAccessBlock` has no backing data at the MRAP
  level, and per-region `Region`/`BucketAccountId` (real `RegionReport` fields) have no backing
  data either (this backend tracks only bucket names per region).
- `ListMultiRegionAccessPointsResult`: **ENVELOPE BUG, FIXED** -- list member name was `item`;
  the real member name is `AccessPoint` (confirmed via
  `awsRestxml_deserializeDocumentMultiRegionAccessPointReportList`) -- a real client would see
  an empty list on every call. Also: items were a narrower ad hoc type
  (`Name`/`Alias`/`Status` only) when the real entry type is the SAME
  `MultiRegionAccessPointReport` `GetMultiRegionAccessPoint` returns; added `CreatedAt`/
  `Regions` (both backed).
- `DescribeMultiRegionAccessPointOperationResult`: GAP-DOCUMENTED -- real `AsyncOperation` also
  carries `CreationTime`/`Operation`/`RequestParameters`/`ResponseDetails`; no audit-trail
  tracking in this backend. `RequestStatus` is hardcoded `"SUCCEEDED"`, which is not fabricated
  per se since every MRAP mutation here completes synchronously.
- `GetMultiRegionAccessPointPolicyResult`: CLEAN (`Policy>Established>Policy` nesting verified;
  `Proposed` GAP-DOCUMENTED, no backing data).
- `GetMultiRegionAccessPointPolicyStatusResult`: CLEAN.
- `GetMultiRegionAccessPointRoutesResult` / `SubmitMultiRegionAccessPointRoutesRequest`:
  **WRONG-SHAPE + REQUEST-BREAKING BUG, FIXED** -- `Routes` was a flat string on both sides,
  and the request field was literally misnamed: the real `SubmitMultiRegionAccessPointRoutesInput`
  field is `RouteUpdates`, not `Routes` (confirmed via
  `awsRestxml_serializeOpDocumentSubmitMultiRegionAccessPointRoutesInput`), wrapping a list of
  `<Route>` entries (`Bucket`/`Region`/`TrafficDialPercentage`). A real client's request would
  never populate the old `Routes` field at all -- every real `SubmitMultiRegionAccessPointRoutes`
  call silently stored an empty routing update. Fixed: renamed to `RouteUpdates`, captured as
  raw inner XML on both request and response sides to preserve the real per-route structure.

### `DeleteAccessGrantsInstance` precondition -- FIXED (grants/locations; extended 2026-08-01 to Identity Center)

Real API doc comment (`api_op_DeleteAccessGrantsInstance.go`, generated from AWS's own Smithy
model): "You must first delete the access grants and locations before S3 Access Grants can
delete the instance. ... If you have associated an IAM Identity Center instance with your S3
Access Grants instance, you must first dissassociate the Identity Center instance from the S3
Access Grants instance before you can delete the S3 Access Grants instance." This is THREE
preconditions, not one. Previously unenforced (gopherstack allowed deleting an instance with
grants/locations still attached); a prior pass fixed the grants/locations half. THIS PASS
(2026-08-01, gopherstack-tir4) found the doc comment's third precondition -- the Identity
Center dissociation requirement -- was still unenforced despite Identity Center association
being fully modeled (`AccessGrantsInstance.IdentityCenterArn`, set by
`AssociateAccessGrantsIdentityCenter`/`CreateAccessGrantsInstance`, cleared by
`DissociateAccessGrantsIdentityCenter`): a real account rejects deleting an instance with a
live Identity Center association, but gopherstack silently allowed it. Fixed:
`DeleteAccessGrantsInstance` now also checks `IdentityCenterArn != ""` on the instance itself
and rejects with the same `errAccessGrantsInstanceNotEmpty` sentinel. All three preconditions
(grants / locations / Identity Center) are now enforced, each independently tested (backend-
and HTTP-level) for both the rejection and the success-once-cleared path. No S3 Control typed
exception is specific to any of the three conflicts (verified against the SDK's full
`types/errors.go` exception list: `BadRequestException`, `BucketAlreadyExists`,
`BucketAlreadyOwnedByYou`, `IdempotencyException`, `InternalServiceException`,
`InvalidNextTokenException`, `InvalidRequestException`, `JobStatusException`,
`NoSuchPublicAccessBlockConfiguration`, `NotFoundException`, `TooManyRequestsException`,
`TooManyTagsException` -- none named for any of them), so this reuses the generic
`BadRequestException` sentinel this codebase already uses for other S3 Access Grants
validation failures, rather than inventing an unverified specific code.

### types_not_reached -- CLOSED 2026-07-30 (follow-up pass, gopherstack-tir4 completion)

Every item below was individually diffed this pass against the installed
`aws-sdk-go-v2/service/s3control@v1.73.0`. None remain unverified.

- `DescribeJob`'s nested sub-structures beyond the top-level fields (`GeneratedManifestDescriptor`,
  `ManifestGenerator`, `FailureReasons`, `SuspendedCause`, `SuspendedDate`): CONFIRMED GAP, not a
  bug. Read `types.JobDescriptor` in full and cross-checked every one of these fields against
  `BatchJob` (models.go) and every write site in jobs.go/handler_jobs.go -- none of the five has
  any backing data anywhere in this backend (no task-execution simulation, no console-confirm
  workflow, no generated-manifest computation exist), so all five are correctly omitted rather
  than fabricated. While checking this, also found (and left as a documented `deferred` gap, not
  fixed): `CreateJobInput.ManifestGenerator` -- a real alternative to `Manifest` a caller can send
  instead -- is accepted nowhere (`createJobRequestXML` has no such field), so a real client using
  it would have that whole configuration silently dropped. See `deferred` above.
- `CreateAccessPoint`'s request-side `VpcConfiguration`/`PublicAccessBlockConfiguration` nesting:
  RE-VERIFIED CLEAN against `awsRestxml_serializeOpDocumentCreateAccessPointInput` directly --
  root `CreateAccessPointRequest`, `VpcConfiguration>VpcId` (single field, matches
  `types.VpcConfiguration`), `PublicAccessBlockConfiguration` with the 4 real bool fields
  (`BlockPublicAcls`/`IgnorePublicAcls`/`BlockPublicPolicy`/`RestrictPublicBuckets`, matches
  `types.PublicAccessBlockConfiguration`). `createAccessPointRequestXML`/`apVpcConfigurationXML`/
  `apPublicAccessBlockXML` (handler_access_points.go) match exactly. No bug.
- Account-level `GetPublicAccessBlock`/`PutPublicAccessBlock`/`DeletePublicAccessBlock`:
  RE-VERIFIED CLEAN field-by-field against `awsRestxml_deserializeOpGetPublicAccessBlock` and
  `awsRestxml_serializeOpPutPublicAccessBlockInput` directly -- both are payload-bound with root
  element `PublicAccessBlockConfiguration` (no outer operation-name wrapper) containing the same 4
  bool fields as above. `publicAccessBlockConfigurationXML` (handler.go) matches exactly,
  including the payload-bound (unwrapped) root on both GET response and PUT request. No bug.
- Bucket lifecycle (`GetBucketLifecycleConfiguration`/`Put`/`Delete`): RE-VERIFIED CLEAN. Read
  `awsRestxml_deserializeOpGetBucketLifecycleConfiguration` directly: it fetches the response's
  root element generically via `smithyxml.FetchRootElement` and never checks that root's NAME --
  it only requires a `Rules` child element inside it. `PutBucketLifecycleConfigurationInput` is
  payload-bound with root `LifecycleConfiguration` (confirmed via
  `awsRestxml_serializeOpDocumentPutBucketLifecycleConfigurationInput`), so gopherstack's
  raw-body PUT-then-GET-verbatim passthrough returns exactly what a real client sent
  (`<LifecycleConfiguration><Rules>...</Rules></LifecycleConfiguration>`) and a real client's
  root-name-agnostic decoder accepts it correctly. No root-mismatch risk, confirmed by reading the
  deserializer rather than inferring it, as the prior pass's version of this bullet only assumed.
- Bucket policy (`GetBucketPolicy`/`Put`/`Delete`): **BUG FOUND AND FIXED.** Unlike Lifecycle
  (above) or Tagging/Versioning/Replication (fixed prior pass), `PutBucketPolicyInput.Policy` is
  **NOT** payload-bound -- the real request root is `PutBucketPolicyRequest` with the policy JSON
  as the text of a nested `Policy` element (confirmed via
  `awsRestxml_serializeOpDocumentPutBucketPolicyInput`). The previous handler applied the
  Lifecycle-style "store the whole raw body verbatim" pattern here too, which was wrong for this
  op: it stored the entire `<PutBucketPolicyRequest><Policy>...</Policy></PutBucketPolicyRequest>`
  envelope as "the policy". `GetBucketPolicy` then wrapped that already-wrong stored string in a
  SECOND `<Policy>` element for its response -- a real client's `GetBucketPolicy` call after
  `PutBucketPolicy` would receive its own policy JSON back double-nested and XML-escaped inside
  someone else's envelope, not the policy it sent. Fixed: `handlePutBucketPolicy`
  (handler_bucket.go) now decodes the real `PutBucketPolicyRequest` envelope via a new
  `putBucketPolicyRequestXML` type and stores only the inner `Policy` text; `GetBucketPolicy`'s
  existing single-wrap response was already correct once given the right input. Locked in by
  `TestBucketPolicy_WireShape` (handler_bucket_test.go), which asserts the literal response XML
  byte-for-byte (not a substring) for both the fixed request parsing and the response shape.
- `DeleteBucketLifecycleConfiguration`/`DeleteBucketPolicy`: no request/response body on either
  side in the real API (confirmed -- both ops have no `awsRestxml_serializeOpDocument...Input`/
  `...Output` functions), and gopherstack's handlers send none. No bug, nothing to diff further.
- Chaos fault-injection interaction with the fix above: unaffected (`ChaosOperations()` just
  echoes `GetSupportedOperations()`).

This closes the entire `types_not_reached` remainder gopherstack-tir4 left open. Combined with
every handler-file section above already being marked "-- all ops diffed", there is no longer a
known-unverified area of this service's request/response wire shapes.

## 2026-08-22 (gopherstack-n3zi): first typed-client round trip, 2 bugs found

s3control was zero of 97 ops covered by any typed `aws-sdk-go-v2` client anywhere in
`test/integration`, and only 7 of those (`CreateAccessGrant`/`CreateAccessGrantsInstance`/
`CreateAccessGrantsLocation`/`CreateAccessPoint`/`CreateJob`/`CreateStorageLensGroup`/
`ListTagsForResource` -- create+tag ops only, no read/update/destroy leg) were covered even by
this package's own in-process typed tests. Added
`test/integration/s3control_access_point_lifecycle_test.go`
(`TestIntegration_S3Control_AccessPointLifecycle`), a real create -> read -> policy attach/detach
-> destroy round trip: `CreateAccessPoint`, `GetAccessPoint`, `ListAccessPoints`,
`PutAccessPointPolicy`, `GetAccessPointPolicy`, `GetAccessPointPolicyStatus`,
`DeleteAccessPointPolicy`, `DeleteAccessPoint`. Every prior "wire: ok" claim in this file was
verified by a raw-body/field-diff read, never by decoding through a real client -- exactly the
blind spot gopherstack-n3zi's measurement (77% of ops repo-wide never driven by a typed client)
predicted, and this pass found two bugs in that blind spot that no prior field diff caught.

**BUG 1 (service-wide, all 97 ops): every s3control error response decoded as a real client's
generic `smithy.GenericAPIError{Code: "UnknownError"}` instead of the real AWS error code.**
`writeXMLErrorCode` (handler.go) wrote `pkgs/awserr`'s `ProtocolRestXML` envelope -- a bare
top-level `<Error><Code>.../Message>...</Error>` document, the real shape for data-plane S3.
But every one of aws-sdk-go-v2/service/s3control@v1.73.4's `awsRestxml_deserializeOpError*`
functions (deserializers.go; grepped all 97, zero exceptions) calls
`s3shared.GetErrorResponseComponents` with `IsWrappedWithErrorTag: true`, which decodes
`Error>Code`/`Error>Message` -- i.e. expects `Code`/`Message` nested one level under a
wrapping root, not at the document root. Confirmed against s3shared's own
`wrappedXMLErrorResponse` fixture (xml_utils_test.go@v1.19.36): the real shape is
`<ErrorResponse><Error><Type>Sender</Type><Code>...</Code><Message>...</Message></Error>
<RequestId>...</RequestId></ErrorResponse>` -- exactly what `pkgs/awserr`'s existing
`ProtocolQueryXML` already emits (queryErrorResponse, respond.go). `ProtocolRestXML` is used
nowhere else in this repo (grepped), so no other service's errors were affected. Fixed by
switching `writeXMLErrorCode`'s single call site from `awserr.ProtocolRestXML` to
`awserr.ProtocolQueryXML`. Every `handleBackendError`-routed error in this service was silently
losing its real code to any typed client before this fix -- not just the two ops the new test
happens to assert on.

**BUG 2: `GetAccessPointPolicyStatus` was a hardcoded stub** (`handler_access_points.go`,
`return writeXML(c, getAccessPointPolicyStatusResponseXML{IsPublic: false})`) that never checked
the access point existed and never looked at whether a policy was attached, unlike its two
siblings covering the same op family (`GetAccessPointPolicyStatusForObjectLambda`,
`GetMultiRegionAccessPointPolicyStatus`), both of which correctly 404 on a nonexistent resource
and derive `IsPublic` from whether a policy string is set. This file's own PARITY table (see the
compact per-op list above) marked `GetAccessPointPolicyStatus: {wire: ok, errors: ok, state: ok,
persist: ok}` -- wrong on `errors` and `state`, both unverified by any client call. Fixed by
adding `InMemoryBackend.GetAccessPointPolicyStatus(accountID, name) (bool, error)`
(access_points.go), matching the existing `policy != ""` heuristic convention used by the two
sibling ops, wired through `interfaces.go` and `handler_access_points.go`. `IsPublic` here is
still the same coarse "a policy is attached" approximation those siblings use, not real IAM
policy evaluation -- consistent with, not a regression from, existing precedent in this file.

**Proof:** both bugs isolated and hand-reverted independently (each against the other's fix
still in place), confirmed to reproduce the original failure
(`TestIntegration_S3Control_AccessPointLifecycle` failing on the exact assertions the bug
predicts: `"UnknownError"` instead of the real code for bug 1, `IsPublic` staying `false` after
`PutAccessPointPolicy` for bug 2), then restored and `md5sum`-verified byte-identical. Also
updated `TestHandler_GetAccessPointPolicyStatus` (handler_access_points_test.go), whose sole
prior case asserted the old stub's wrong behavior (200/not-public on a nonexistent access point)
-- rewritten table-driven to cover no-policy, policy-attached, and nonexistent-access-point.

**Gates:** `go build ./...`, `go vet` (default/e2e/integration via `make build-check`), `gofmt
-l` (clean), `go test -race` (services/s3control, pkgs/awserr, both green), `golangci-lint run`
(0 issues, no nolint added), live `test/integration` run against a real running gopherstack
binary (`GOPHERSTACK_ENDPOINT`) both with and without `-tags integration`: pass.

**0.4-bugs-per-op prior:** held loosely -- 2 bugs surfaced from this one lifecycle test (8 ops
newly exercised at the test/integration layer), but bug 1 is service-wide (all 97 ops), so by op
count this sample is far above the prior, not a counter-example like the lightsail pass. Neither
bug was reachable by any documented "preferred/alternate calling pattern" the way ssm
Targets/kinesis StreamARN were in the same issue's other finds -- both are plain silent-failure
bugs (a swallowed error code, a stub ignoring real state) that only a typed client's decode step
could surface.

## gopherstack-o7gx follow-up (2026-08-22): default error path emitted InternalError instead of the modeled fault

`handler.go`'s `handleBackendError` (default branch) and `writeXML`
(marshal-error branch) both wrote code `"InternalError"` for any
unclassified/unexpected 500. `s3control@v1.73.4` `types/errors.go:114-136`
models `InternalServiceException` (`ErrorFault: FaultServer`) as the
service's 5xx fault. It is wired into only 8 of s3control's 97 operation
error switches in `deserializers.go` -- no single code is dominant for this
service; the other ~89 operations model no 5xx exception of their own at
all, so any code chosen for the default branch falls through to the same
`smithy.GenericAPIError` for them regardless. `InternalServiceException` is
still the correct fix: it is s3control's own real modeled fault (not
borrowed from another service, unlike the servicediscovery/xray follow-ups
in this same pass), and using it strictly improves the 8 operations that do
model it while regressing none.

Fixed both sites to `"InternalServiceException"`. Both of
`handleBackendError`'s and `writeXML`'s default branches are reachable only
when a backend error isn't classified as NotFound/InvalidParameter/
AlreadyExists (or, for `writeXML`, only on an `xml.Marshal` failure); no
currently-wired dispatch path leaves an error unclassified this way, so
there is no legitimately-constructed real SDK client request that reaches
either branch today. `TestHandleBackendError_DefaultBranchEmitsInternalServiceException`
(`handler_internal_error_test.go`, new, white-box `package s3control`)
drives `handleBackendError` directly with a synthetic unmatched error and
asserts the XML response's `<Error><Code>` is `InternalServiceException`;
confirmed it fails pre-fix with the old `"InternalError"` code
(hand-reverted, byte-identical restore after).

## gopherstack-21my per-item typed round-trip pass (2026-08-31)

s3control was one of the eighteen services in gopherstack-21my marked "clean
at wrapper level, never swept per-item." Per that issue's own finding (rds's
DescribeDBInstances read clean by hand the session before a real bug was
found underneath it), this pass writes a typed round-trip test instead of
reading `deserializers.go` by eye.

**Covered**: `CreateJob`/`DescribeJob`/`ListJobs` -- this service's richest
nested item shape, and its most unusual: `Job.Manifest`/`Operation`/`Report`
are stored and re-emitted as the client's own raw inner XML
(`createJobXMLCapture`, handler_jobs.go) rather than individually modeled,
so the only way to prove this actually round-trips through the real SDK's
serializer/deserializer pair -- not merely that the stored bytes look
plausible -- is a typed client test. Seeded two jobs (a `LambdaInvoke` and
an `S3PutObjectCopy` operation, each with a full `Manifest{Location,Spec}`
and `Report`) and asserted via the real client: `ListJobs`' per-item
`Operation` enum (derived from the raw XML's root element name via
`jobOperationName`) for both jobs, and `DescribeJob`'s full decoded
`Manifest.Location`/`Manifest.Spec.Fields`/`Report`/`Operation.LambdaInvoke`
for one of them. `TestSDKRoundTrip_JobManifestOperationReport`
(`sdk_roundtrip_nested_test.go`), 19 `require` calls. **Result: clean** --
the raw-echo strategy holds because CreateJob's raw capture is exactly what
the real client itself serialized, so DescribeJob's deserializer (built for
the same real client) parses it back correctly by construction; this test
is the first to actually exercise that symmetry rather than assume it.

**Not covered this pass**: `ListMultiRegionAccessPoints` (`Regions[]` nested
list), `ListStorageLensConfigurations`/`ListStorageLensGroups`,
`ListAccessGrants`/`ListAccessGrantsLocations` (already has real-client
pagination coverage via `wire_field_fixes_test.go` and prior
`ListAccessGrants` filter-key fixes, but not a nested-list nested-field
sweep), `ListRegionalBuckets`. `ListAccessPoints` already has real-client
pagination coverage (`TestListAccessPoints_FullPagination`) but its item
fields (`apVpcConfigurationXML`, `apPublicAccessBlockXML`) were not
re-verified in this pass.

**Test-file exposure**: of 23 `*_test.go` files in this service, only 4 (5
counting the new one) drive a real typed `aws-sdk-go-v2` client
(`NewFromConfig`) -- the remaining ~83% assert on raw XML/HTTP responses,
which cannot see a wrong-element-name or dropped-nested-field bug of this
class.

Gates: `go build ./services/s3control/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/s3control/...` (pass),
`golangci-lint run ./services/s3control/...` (0 issues, `golines -w -m 120`
applied then re-verified with plain `golangci-lint run`).

## gopherstack-21my continuation (2026-08-31, s3control+neptune pair)

Confirmed protocol from `deserializers.go` before starting: s3control is
`awsRestxml_`, so the case-only class (smithy-go's XML decoder folds
element-name case, `encoding/xml/xml_decoder.go:92`) is live here.

**Covered this pass**: `ListAccessPoints` item fields (re-verified against
`types.AccessPoint`, clean), `ListMultiRegionAccessPoints` (shares its item
struct with `GetMultiRegionAccessPoint`, verified against
`types.MultiRegionAccessPointReport`, clean), `ListAccessGrants`/
`ListCallerAccessGrants`/`ListAccessGrantsLocations` (verified byte-for-byte
against `types.ListAccessGrantEntry`/`ListCallerAccessGrantsEntry`/
`ListAccessGrantsLocationsEntry`, clean), `ListRegionalBuckets` (verified
against `types.RegionalBucket` -- see gap below), `ListStorageLensConfigurations`
and `ListStorageLensGroups` (both had real bugs, see below).

**BUG (fixed): `ListStorageLensConfigurations`' per-item shape emitted only
`Id`.** The real item type, `types.ListStorageLensConfigurationEntry`
(s3control@v1.73.4 types/types.go:1389, confirmed via
`awsRestxml_deserializeDocumentListStorageLensConfigurationEntry`), has four
required members: `HomeRegion`, `Id`, `StorageLensArn`, `IsEnabled`. Three
were dropped entirely -- right config count, `IsEnabled` always false and
`HomeRegion`/`StorageLensArn` always empty regardless of what was actually
configured. This backend stores each config as the client's own raw
`<StorageLensConfiguration>` inner XML (`storage_lens.go`), so `IsEnabled`
is genuinely present in that raw blob but was never parsed back out for the
list response; `HomeRegion` and `StorageLensArn` are synthesized from the
backend's own region/account (new `arnFmtStorageLensConfig` in store.go,
matching the real `arn:aws:s3:<region>:<account-id>:storage-lens/<config-id>`
format documented at types/types.go:3029). Fixed in
`handler_storage_lens.go` (`listStorageLensConfigItemXML`, new
`storageLensConfigIsEnabled` raw-XML-fragment parser, same technique as the
existing `jobOperationName` helper). Test:
`TestSDKRoundTrip_ListStorageLensConfigurations_ItemFields`
(`sdk_roundtrip_nested_test.go`), seeds two enabled configs via a real
client and asserts `IsEnabled`/`HomeRegion`/`StorageLensArn` on both;
confirmed failing pre-fix (`IsEnabled` false when the client sent true).
Silent-blank failure signature, not a hard decode error (`IsEnabled` uses
`strconv.ParseBool` only when the element is present).

**BUG (fixed): `ListStorageLensGroups`' per-item shape dropped `HomeRegion`.**
`types.ListStorageLensGroupEntry` (types/types.go:1417) requires
`HomeRegion`/`Name`/`StorageLensGroupArn`; `listStorageLensGroupItemXML`
emitted only the latter two. The backend already computes the region-scoped
ARN at `CreateStorageLensGroup` time (`arnFmtStorageLensGroup` with
`b.region`), so the same region value was trivially available and simply
never threaded into the list item. Fixed: `buildListSLGItem` now takes the
backend's `Region()` and sets `HomeRegion`. Test:
`TestSDKRoundTrip_ListStorageLensGroups_HomeRegion`
(`sdk_roundtrip_nested_test.go`), confirmed failing pre-fix (`HomeRegion`
empty against `"us-east-1"`).

**Gap recorded, not fixed**: `ListRegionalBuckets`' per-item
`types.RegionalBucket` requires `CreationDate` (a `*time.Time`) and also
carries `PublicAccessBlockEnabled`/`OutpostId` -- all three are absent from
`OutpostsBucket` (models.go), so `listRegionalBucketItemXML` correctly omits
what it cannot honestly populate. Verified this is NOT a sibling
disagreement: `GetBucket` shares the identical gap (its own doc comment
already says so), so there is no naming mismatch to fix, only backend state
that was never modeled. Fixing it would mean adding a real `CreationDate` to
`OutpostsBucket` and threading it through `CreateBucket`, the same shape as
neptune's `ClusterCreateTime`/`InstanceCreateTime` fixes elsewhere in this
campaign -- worth a dedicated follow-up, not folded into this per-item
naming sweep.

Wrapping shape re-checked for every op above: no call site of any
`*Unwrapped` deserializer variant in this service outside the two
already-documented flattened Storage Lens lists (which are correctly
flattened per their own doc comments). No case-only mismatch found this
pass.

Gates: `go build ./services/s3control/... ./services/neptune/...`, `go vet
./services/s3control/... ./services/neptune/...` (clean; repo-wide `go vet
./...` currently fails in `services/ec2`, out of scope, held by a different
agent's concurrent in-progress work -- confirmed via `git status --short
services/ec2` showing unrelated uncommitted changes), `go test -race
-count=1 ./services/s3control/... ./services/neptune/...` (pass),
`golangci-lint run ./services/s3control/... ./services/neptune/...` (0
issues, `golines -w -m 120` applied then re-verified).
