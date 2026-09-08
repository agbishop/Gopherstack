---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: codeartifact
sdk_module: aws-sdk-go-v2/service/codeartifact@v1.41.4   # version audited against
last_audit_commit:                                # unknown: pass (2026-08-15, gopherstack-6flj) ran without git access at write time, never backfilled -- gopherstack-33in; this pass fixed a total-outage array-vs-map bug on 4 package-version ops, a DeletePackage sibling-trap, 2 ignored filters, 2 required-field gaps, and 2 backend-tracked-but-unemitted fields
last_audit_date: 2026-08-15
overall: A            # this pass: package-group "weak match" (casefold + dash/dot/underscore-run
                      # normalization, per AWS's documented dependency-confusion-protection
                      # algorithm) implemented and wired into GetAssociatedPackageGroup/
                      # ListAssociatedPackages' associationType (previously hardcoded "STRONG").
                      # Confusable-character normalization remains unimplemented (needs the full
                      # Unicode confusables table). Prior pass: real package-group pattern-matching
                      # algorithm implemented, readme/dependency extraction implemented (npm
                      # package.json scope), UpdatePackageGroupOriginConfiguration/
                      # ListAllowedRepositoriesForGroup made real, a domain-delete package-group
                      # leak fixed. See ops table + gaps below.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateDomain: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDomain: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDomain: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED — api_op_DeleteDomain.go: 'You cannot delete a domain that contains repositories. If you want to delete a domain with repositories, first delete its repositories.' DeleteDomain models ConflictException for exactly this; the emulator instead cascade-deleted every repository/package/version in the domain unconditionally. Now rejects with ConflictException when the domain has any repository, leaving it and its contents intact (package-group + domain-policy cascade for an empty domain is unaffected)"}
  ListDomains: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — maxResults/nextToken are JSON body fields (POST), not query params, unlike every other List op; was reading query only (always empty)"}
  CreateRepository: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — real, always-present RepositoryDescription.CreatedTime (deserializers.go) was never emitted despite the backend already tracking it; shared by Create/Describe/Delete/Associate/Disassociate/UpdateRepository via repoToMap. Prior: request body field renamed upstreamRepositories -> upstreams (real wire key)"}
  DescribeRepository: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — see CreateRepository's CreatedTime note"}
  UpdateRepository: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — see CreateRepository's CreatedTime note. Prior: request body field upstreamRepositories -> upstreams"}
  DeleteRepository: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — see CreateRepository's CreatedTime note"}
  ListRepositories: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (gopherstack-6flj) — real repository-prefix query filter (serializers.go's SetQuery) was silently discarded; also RepositorySummary was a hand-built 4-field map missing 3 real members (administratorAccount/createdTime/description), consolidated into repositorySummaryToMap. Prior: maxResults/nextToken query params are max-results/next-token (kebab), not camelCase"}
  ListRepositoriesInDomain: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (gopherstack-6flj) — same repository-prefix filter + RepositorySummary gaps as ListRepositories. Prior: same kebab-case pagination bug"}
  GetRepositoryEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAuthorizationToken: {wire: partial, errors: ok, state: ok, persist: n/a, note: "token is a fabricated string (codeartifact-stub-token-<domain>), not a real signed/opaque credential — acceptable for an emulator since no downstream auth check consumes it, but flagged for awareness"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDomainPermissionsPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutDomainPermissionsPolicy: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "FIXED (gopherstack-6flj) — PolicyDocument is 'This member is required.' on the real Input (confirmed via the real SDK's own generated client-side validator, validators.go) but was silently defaulted to an empty-statement policy instead of rejected with ValidationException; a raw caller (not a real SDK client, which can't send this request at all) could reach the old lenient behavior. ALSO FIXED (this pass) — PolicyRevision ('This revision is used for optimistic locking, which prevents others from overwriting your changes to the domain's resource policy.') was accepted and parsed nowhere: any caller could silently overwrite another caller's policy update. Now enforced against the stored policy's revision (ConflictException on mismatch); an omitted revision is still accepted unconditionally, matching the real optional field"}
  DeleteDomainPermissionsPolicy: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "FIXED (this pass) — same unenforced PolicyRevision optimistic-locking gap as PutDomainPermissionsPolicy, on the delete side (policy-revision query param)"}
  GetRepositoryPermissionsPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutRepositoryPermissionsPolicy: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "FIXED (gopherstack-6flj) — same required-PolicyDocument gap as PutDomainPermissionsPolicy. ALSO FIXED (this pass) — same unenforced PolicyRevision optimistic-locking gap as PutDomainPermissionsPolicy"}
  DeleteRepositoryPermissionsPolicy: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "FIXED route-matcher bug — real path is plural /v1/repository/permissions/policies, DELETE-only; was sharing the singular /v1/repository/permissions/policy path with Get/Put, which real AWS does NOT serve DELETE on. ALSO FIXED (this pass) — same unenforced PolicyRevision optimistic-locking gap as DeleteDomainPermissionsPolicy"}
  AssociateExternalConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — query param externalConnection -> external-connection (kebab)"}
  DisassociateExternalConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — same externalConnection -> external-connection"}
  CreatePackageGroup: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FOUND AND FIXED THIS PASS (gopherstack-u9e5, via the new SDK-driven integration test) — SEVERE: the request-body JSON key was 'pattern'; the real wire key (verified against serializers.go's awsRestjson1_serializeOpDocumentCreatePackageGroupInput) is 'packageGroup'. Every unit test constructed its request body by hand using 'pattern' (matching this bug, not the real wire — the same trap parity-principles.md rule 3 warns about), so a real aws-sdk-go-v2 client's CreatePackageGroup call ALWAYS failed with a spurious 'pattern is required' ValidationException against every prior build of this emulator, even though this op was graded ok by two prior audits. 16 unit-test call sites across 3 test files updated to the real key alongside the fix."}
  DescribePackageGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — query param packageGroup -> package-group (kebab); was always empty for real clients -> spurious ValidationException"}
  DeletePackageGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — same packageGroup -> package-group"}
  UpdatePackageGroup: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — PackageGroup ('This member is required.' on the real Input) was never validated, unlike its Create/Describe/Delete siblings; an empty pattern fell through to the backend and surfaced as a misleading 404 instead of the real 400 ValidationException. Prior: packageGroup is a JSON body field here (matches real wire), the (wrong) query fallback was dead code for real traffic but harmless"}
  ListPackageGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED pagination casing; createdTime was missing from response (real field, was tracked but never serialized) — now added"}
  ListSubPackageGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — real parent/child hierarchy: a group's children are every OTHER domain group whose immediate (most-specific) proper-superset pattern is exactly this one, computed via package_group_pattern.go's isProperSubsetPattern; replaced the old string-prefix heuristic. Verified direct-children-only against the ListSubPackageGroups API reference (not the full descendant subtree)."}
  GetAssociatedPackageGroup: {wire: ok, errors: ok, state: partial, persist: n/a, note: "Real most-specific-pattern matching (package_group_pattern.go) replaces the always-nil stub (prior pass); response includes associationType. FIXED this pass: associationType now genuinely computed as STRONG or WEAK (was hardcoded 'STRONG' — see package_group_pattern_matching family note) via casefold + dash/dot/underscore-run normalization, matching AWS's documented dependency-confusion-protection algorithm. state gap: confusable-character normalization (the third weak-match rule) is not implemented (needs the full Unicode confusables table), and this backend does not auto-create the implicit root '/*' group every real domain has — see gaps."}
  ListAssociatedPackages: {wire: ok, errors: ok, state: partial, persist: n/a, note: "Real domain-wide matching (prior pass): for each package (deduped by format/namespace/name across repos), computes its most-specific matching group and includes it only if that group is the requested pattern. Pagination (max-results/next-token, kebab) added prior pass. FIXED this pass: associationType per package is now genuinely STRONG or WEAK (was hardcoded 'STRONG'), same algorithm as GetAssociatedPackageGroup — a package that only weak-matches the requested group's pattern is still included (weak match doesn't roll up to a broader group, per AWS's documented behavior) but reported WEAK. state gap: Preview flag (compute association without creating the group) not read/supported."}
  ListAllowedRepositoriesForGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — the previously-unread required originRestrictionType query param (camelCase, NOT kebab — verified against serializers.go, an exception to this service's usual kebab-case query convention) is now read/validated and used to look up the real per-restriction-type AllowedRepositories list set via UpdatePackageGroupOriginConfiguration; added pagination. FIXED missing 404: real AWS 404s when the package group doesn't exist, this op never checked."}
  UpdatePackageGroupOriginConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — request body now real (restrictions map[type]mode, addAllowedRepositories/removeAllowedRepositories []{originRestrictionType,repositoryName}), validated against the real 4-value mode / 3-value type enums; response now returns the real allowedRepositoryUpdates map[type]map[ADDED|REMOVED][]repoName shape (verified against the API reference's response syntax) plus the updated packageGroup with a real originConfiguration.restrictions block (mode/effectiveMode/repositoriesCount/inheritedFrom, resolved by walking the pattern-hierarchy's INHERIT chain up to the nearest explicit ancestor, defaulting to ALLOW at the top like real AWS's root group). FIXED missing repository-existence check on add/remove entries."}
  DescribePackage: {wire: fixed, errors: ok, state: partial, persist: ok, note: "auto-creates a stub package on first Describe if absent (pre-existing behavior, not touched this pass — see gaps); now surfaces originConfiguration when set. gopherstack-g479 (2026-08-21): CORRECTION to the DeletePackage row's own 'Describe shape' framing below -- packageToMap itself was wrong, not just misapplied. Real types.PackageDescription (aws-sdk-go-v2/service/codeartifact@v1.41.4's deserializers.go AND types/types.go, both checked) declares only format/name/namespace/originConfiguration; domainName, domainOwner and repository are not members of it at all and were leaking onto the wire with no real field to correspond to. Found via a new go/types-based map-literal kind scanner; proven via a raw-response-body assertion (a typed real client silently ignores unknown keys, so decode-based proof can't show this class -- same precedent as ssm's Patch.State fix)."}
  DeletePackage: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — SIBLING-TRAP: DeletePackageOutput.DeletedPackage is real *types.PackageSummary (format/namespace/originConfiguration/package), NOT *types.PackageDescription; the handler reused packageToMap (the Describe shape) instead of packageSummaryToMap (the List/Delete shape, already split out for ListPackages under gopherstack-tuh5 but missed here) — dropped the identifier entirely (PackageSummary has no 'name' key) and leaked domainName/domainOwner/repository, three keys real PackageDescription itself doesn't have either (see DescribePackage row, gopherstack-g479 correction)"}
  ListPackages: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tuh5: was reusing packageToMap (the full DescribePackage converter) unscoped, leaking domainName/domainOwner/repository, none of which types.PackageSummary declares. Same function ALSO had an inverse bug: it emitted the package identifier under key \"name\", but the real deserializer (awsRestjson1_deserializeDocumentPackageSummary, deserializers.go:10044) only recognises \"package\" -- so the identifier was silently dropped for every real client, on top of the leak. Now emits types.PackageSummary (format/namespace/originConfiguration/package) via a dedicated packageSummaryToMap. Regression: raw-body test for the leak (SDK clients discard unrecognised keys and can't see it), real aws-sdk-go-v2 client test for the wrong-key loss (a raw-body assertion is weak here -- only a typed caller shows the identifier actually reaching PackageSummary.Package). Prior: FIXED pagination casing"}
  PutPackageOriginConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED disguised no-op — backend built a Package literal but never called packages.Put (state was discarded); FIXED route-matcher bug — real op has no path of its own, it is POST on the shared /v1/package path (was GET/DELETE only, PUT on a nonexistent /v1/package/origin-configuration path); FIXED response shape — real output is flat {originConfiguration:{restrictions:{publish,upstream}}}, was wrapping in {package:...} and not reading the request body's restrictions at all"}
  DescribePackageVersion: {wire: ok, errors: ok, state: partial, persist: ok, note: "FIXED wire bug — publish-time field key is publishedTime, was publishedAt (real SDK deserializer never populated PublishedTime). auto-creates a stub version on first Describe if absent (pre-existing, not touched — see gaps)"}
  ListPackageVersions: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (gopherstack-6flj) — 2 real filter/ordering members (status, sortBy=PUBLISHED_TIME) were silently discarded, and the real namespace echo + defaultDisplayVersion member (computed as most-recently-published, matching AWS's own doc fallback since this backend has no npm dist-tag concept) were entirely absent. originType is also real but has no backend field to source from — see gaps. gopherstack-tuh5: was reusing packageVersionToMap (the full DescribePackageVersion converter) unscoped, leaking format/packageName/publishedTime/namespace, none of which types.PackageVersionSummary declares. Now emits types.PackageVersionSummary (status/version/origin/revision, confirmed against awsRestjson1_deserializeDocumentPackageVersionSummary) via a dedicated packageVersionSummaryToMap; origin is a real Summary member but the backend's PackageVersion model has no source for it, so it stays absent rather than fabricated. Regression: raw-body assertion (SDK clients discard unrecognised keys and can't see the leak). Prior: FIXED pagination casing"}
  PublishPackageVersion: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED wire bug (prior pass) — real response is FLAT {format,namespace,package,status,version,versionRevision,asset}, was nesting under packageVersionToMap with wrong field names (packageName not package, revision not versionRevision) and no asset field; FIXED disguised no-op (prior pass) — the uploaded asset's raw octet-stream body was discarded (Handler() only ever attempted a JSON decode, which fails silently on binary content) and the asset query param was never read; now stores the asset (name/size/sha256/content) on the PackageVersion and GetPackageVersionAsset/ListPackageVersionAssets serve it back; FIXED missing repository-existence check (prior pass, real API 404s if the repo doesn't exist, this op never checked). FOUND AND FIXED THIS PASS (gopherstack-u9e5, via the new SDK-driven integration test) — SEVERE route-matcher bug: the registered path was /v1/package/versions/publish (plural 'versions'); the real path (verified against serializers.go's SplitURI) is /v1/package/version/publish (singular, matching this service's own convention that single-version ops use singular 'version' and only the batch ops use plural 'versions'). A real aws-sdk-go-v2 client's PublishPackageVersion call 404'd (UnknownOperationException) against every prior build of this emulator — every one of the extensive fixes/features listed above for this op (asset storage, wire shape, npm-package.json readme/dependency extraction) was unreachable by any real SDK client the entire time, despite this op having been through 3+ prior audit passes and a dedicated route_matcher family audit that claimed 'all other op paths/methods verified correct'. 25+ unit-test call sites across 4 test files updated to the real path alongside the fix. FIXED THIS PASS (gopherstack-h910): the required AssetSHA256 (sent as the X-Amz-Content-Sha256 header, verified against serializers.go's awsRestjson1_serializeOpHttpBindingsPublishPackageVersionInput -- not a body field) was decoded nowhere; the handler silently computed its own SHA256 from the uploaded body and ignored whatever the client sent, so a corrupted-in-transit upload could never be detected. Now required and checked against the computed hash. Note: the bd issue that flagged this cited a MismatchedSha256Exception, but the pinned SDK (codeartifact@v1.41.4) declares no such exception for this op -- its deserializer's error switch is only AccessDeniedException/ConflictException/InternalServerException/ResourceNotFoundException/ServiceQuotaExceededException/ThrottlingException/ValidationException, so a mismatch now returns ValidationException instead."}
  DeletePackageVersions: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — SEVERE, total-outage: failedVersions/successfulVersions were built as a JSON ARRAY; the real Output members are map[string]types.PackageVersionError / map[string]types.SuccessfulPackageVersionInfo, a JSON OBJECT keyed by version string (deserializers.go's ...PackageVersionErrorMap/...SuccessfulPackageVersionInfoMap, which hard-error on a non-object) — every real SDK client's call to this op failed outright with a deserialization error, reproduced verbatim against unfixed code. Also fixed an invented errorCode ('RESOURCE_NOT_FOUND', real value is NOT_FOUND). New PackageVersionOutcome{Revision,Status} type + shared packageVersionOutcomesToWire helper across all 4 ops below."}
  CopyPackageVersions: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — same array-vs-map total-outage bug as DeletePackageVersions, plus a fabricated successful-entry status literal ('Copied', not a real PackageVersionStatus enum value) replaced with the copied version's actual tracked status. Prior: query params sourceRepository/destinationRepository -> source-repository/destination-repository (kebab)"}
  DisposePackageVersions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — same array-vs-map total-outage bug; this op's errorCode ('NOT_FOUND') was already correct, a sibling-trap-in-reverse against Delete/Copy's wrong 'RESOURCE_NOT_FOUND'"}
  UpdatePackageVersionsStatus: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6flj) — same array-vs-map total-outage bug, plus a fabricated successful-entry status literal ('SUCCESS', not a real PackageVersionStatus enum value) replaced with the real in.TargetStatus"}
  GetPackageVersionAsset: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED disguised no-op — always returned empty 200 regardless of asset name/existence; now returns real stored content or 404 for an asset that was never published"}
  GetPackageVersionReadme: {wire: ok, errors: ok, state: partial, persist: n/a, note: "FIXED (this pass) — response is now the real flat shape (format/namespace/package/readme/version/versionRevision, verified against deserializers.go), and readme is populated for real when the caller published an asset literally named package.json whose JSON content has a readme field (npm convention). state gap: without such an asset, still returns empty (this backend doesn't unpack full tarballs/POMs) — see gaps. gopherstack-7opw (2026-09-08): validatePackageVersionParams wrote its rejection via c.JSON and returned that (always-nil) result; this handler and its two siblings below tested it and fell through to call the real (read-only) Backend method with an empty param, writing a second body on top of the committed one (gopherstack-8haq shape). No mutation was possible (both calls are read-only), only a spurious concatenated response. Fixed to return the raw ErrValidation-wrapped error; each call site now maps and writes it exactly once via handleError."}
  ListPackageVersionAssets: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED disguised no-op — always returned [] regardless of what was published; now lists real stored AssetSummary entries (name/size/hashes). gopherstack-7opw (2026-09-08): same validatePackageVersionParams rejection-bypass fix as GetPackageVersionReadme."}
  ListPackageVersionDependencies: {wire: ok, errors: ok, state: partial, persist: n/a, note: "FIXED (this pass) — response is now the real shape (dependencies[]/format/namespace/package/version, verified against deserializers.go), and dependencies are populated for real from a published package.json's dependencies/devDependencies/peerDependencies/optionalDependencies maps (dependencyType regular/dev/peer/optional per types.PackageDependency's doc comment). state gap: same package.json-only scope as GetPackageVersionReadme — see gaps. gopherstack-7opw (2026-09-08): same validatePackageVersionParams rejection-bypass fix as GetPackageVersionReadme."}
families:
  route_matcher: {status: ok, note: "Audited every op's path+method against aws-sdk-go-v2 serializers.go SplitURI/request.Method. Found and fixed 5 path/method bugs in a prior pass: DeleteRepositoryPermissionsPolicy (wrong shared path), GetAssociatedPackageGroup (wrong path), ListAssociatedPackages (wrong path), ListSubPackageGroups (wrong path), PutPackageOriginConfiguration (wrong path — real op has none, shares POST /v1/package). FOUND AND FIXED THIS PASS (gopherstack-u9e5, via the new SDK-driven integration test, NOT the original manual serializers.go audit that claimed 'all other op paths/methods verified correct'): PublishPackageVersion's path was /v1/package/versions/publish (plural 'versions') — real path (verified against serializers.go's SplitURI) is /v1/package/version/publish (singular, like the other single-package-version ops asset/readme/assets/dependencies; only the batch ops copy/delete/dispose/update_status use plural). A real aws-sdk-go-v2 client's PublishPackageVersion call 404'd against every prior build of this emulator. Every unit test built its own request path by hand matching the (wrong) constant, so this was invisible without a real SDK client — exactly the trap this manual path audit was supposed to catch and didn't."}
  query_param_casing: {status: ok, note: "Audited every op's query-string parameter names against aws-sdk-go-v2 serializers.go SetQuery(...) calls. Found and fixed a service-wide pattern: List-op pagination (maxResults/nextToken) and several other params (packageGroup, externalConnection, sourceRepository/destinationRepository) use kebab-case on the wire (max-results, next-token, package-group, external-connection, source-repository, destination-repository) but the handler read camelCase query keys — meaning pagination and several ops were silently broken for any real AWS SDK client (worked only in unit tests that construct query strings by hand). ListDomains is the sole exception: its pagination is JSON-body, not query, distinguishing it from every other List op. This pass found one more exception: ListAllowedRepositoriesForGroup's originRestrictionType is genuinely camelCase on the wire (verified against serializers.go), unlike every other param in this family."}
  package_group_pattern_matching: {status: ok, note: "Implemented (prior pass) AWS's package-group pattern-matching algorithm (package_group_pattern.go): pattern parsing (format[/namespace[/name]] + $/~/ * suffix), matching, word-boundary prefix matching, and the specificity/subset ordering that defines the group hierarchy (parent/child, most-specific-match). Wired into GetAssociatedPackageGroup, ListAssociatedPackages, ListSubPackageGroups, and UpdatePackageGroupOriginConfiguration's INHERIT-chain resolution. NEW this pass: matchesWeak/normalizeWeak implement the casefold + dash/dot/underscore-run-collapse half of 'weak match' (dependency-confusion protection); bestMatchingGroup now selects the most-specific group via the weak-match (superset) space and classifies STRONG vs WEAK by re-checking the strong (exact) match, matching AWS's documented 'weak match doesn't roll up to a broader group' behavior. Still NOT implemented: confusable-character normalization (needs the full Unicode confusables table — real, external data this pass didn't have room to vendor) and the implicit root '/*' group auto-creation — see gaps."}
  list_summary_shape: {status: fixed, note: "gopherstack-tuh5: ListPackages/ListPackageVersions each reused their Describe sibling's full converter (packageToMap/packageVersionToMap) unscoped, leaking Get-only members (see ops). ListPackages' packageToMap also had a wrong-key inverse bug: the package identifier was emitted as \"name\" where types.PackageSummary's real deserializer only recognises \"package\" — a distinct bug class from the leak (a real client loses the field rather than merely receiving extras it ignores), found and fixed in the same function. Both now have a dedicated *SummaryToMap converter built by reading that op's own types.*Summary struct and deserializer individually. Regression coverage in handler_list_summary_test.go: raw-body assertions for both leaks (an SDK client discards unrecognised keys and can't observe an over-wide response), plus a real aws-sdk-go-v2 client test for the wrong-key loss specifically (a raw-body assertion is weak there — only a typed caller shows PackageSummary.Package actually reaching the caller)."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "Package-group 'weak match' confusable-character normalization (the third rule of AWS's dependency-confusion-protection algorithm, alongside casefolding and dash/dot/underscore-run collapsing — both of which ARE implemented this pass, see package_group_pattern_matching family note) is not implemented. It requires the full Unicode confusables table (real, external data — genuinely buildable, not structural, but this pass didn't have room to vendor and verify it faithfully). A package that differs from a group's exact pattern only by a confusable-character substitution (e.g. a Cyrillic look-alike) will not be detected as either a strong or weak match by this backend. (bd: gopherstack-u9e5 follow-up)"
  - "Origin-restriction configuration (PackageGroupOriginRestriction mode/ALLOW-BLOCK, weak-match blocking) is fully modeled and returned by the API (CreatePackageGroup/DescribePackageGroup/UpdatePackageGroupOriginConfiguration/GetAssociatedPackageGroup's associationType) but is NOT enforced anywhere: PublishPackageVersion and package-version ingestion never consult a package's associated group's origin restrictions, for either STRONG or WEAK-matched packages. Real AWS's core dependency-confusion protection is precisely this enforcement (\"the package is blocked instead of applying the group's origin control configuration\" for a WEAK match) — this backend computes the classification but does not act on it. Found this pass while implementing weak-match classification; pre-existing (not introduced this pass), and a materially larger feature (wiring restriction checks into the publish/ingestion path) than the classification logic itself. (bd: gopherstack-u9e5 follow-up)"
  - "This backend does not auto-create the implicit root package group ('/*') that real AWS attaches to every domain and forbids deleting. Adding it would change GetAssociatedPackageGroup/ListPackageGroups behavior on a domain with zero explicitly-created groups (several existing tests assert 'no groups yet' -> empty list / no match), so it was deliberately left out this pass rather than rewriting that test surface; flagged for a future pass. (bd: gopherstack-u9e5 follow-up)"
  - "DescribePackage / DescribePackageVersion auto-create a stub record when the resource doesn't exist, instead of returning ResourceNotFoundException like real AWS. This is pre-existing, intentionally-documented behavior, reconfirmed this pass to be extremely load-bearing test-seeding infrastructure (60+ call sites across handler_package_versions_test.go, handler_package_versions_assets_test.go, persistence_test.go, handler_packages_test.go use GET as a seed operation), so ripping it out remains a large, independently-scoped migration — not touched this pass either. Real behavioral divergence from AWS. (bd: gopherstack-u9e5 follow-up)"
  - "GetPackageVersionReadme / ListPackageVersionDependencies now parse real content from a published package.json asset (npm convention — see the ops table), but still return empty for any format/publish that doesn't include a standalone package.json asset (e.g. a real npm tarball, a Maven POM, or any non-npm format) — this backend's single-asset-per-call publish model doesn't unpack archives."
  - "GetAuthorizationToken returns a fabricated token string rather than any real credential material; acceptable since nothing validates it downstream, but flagged in case a future op starts checking it."
  - "domain-owner / cross-account query param is accepted by real AWS on nearly every op (for cross-account domain access) but is not read anywhere in this backend; single-account-only is assumed throughout."
  - "ListPackageVersionsInput.OriginType (real filter member, serializers.go's SetQuery(\"originType\")) is not honored -- this backend's PackageVersion model has no per-version origin concept at all (unlike status/sortBy, both fixed this pass, gopherstack-6flj) to filter on; fabricating one would be worse than the current no-op. (bd: gopherstack-6flj follow-up)"
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "Package-group weak-match confusable-character normalization and origin-restriction enforcement against publish/ingestion (see gaps above)"
  - "Root package-group auto-creation (see gaps above)"
  - "store_setup.go was read but not modified — no bugs found, not exhaustively re-audited"
leaks: {status: clean, note: "FIXED (this pass) — DeleteDomain never cascade-deleted the domain's package groups (ghost store rows) or closed their Tags (a pkgs/tags leak), despite deleting everything else the domain owned; every other resource path (repositories/packages/versions/policies/external-connections) was already covered by pre-existing cascade logic. Re-verified: no goroutines/janitors in this service; store.Table-backed state is snapshot/restored via existing Handler.Snapshot/Restore delegation to InMemoryBackend; new Restrictions/Assets/OriginConfig fields are plain JSON-tagged struct fields and round-trip automatically."}
---

## Notes

**2026-08-13 (gopherstack-jqh2 pass 3):** re-extracted all 48 ops' real
method+path directly from `codeartifact@v1.41.4` serializers.go and drove
them through `ExtractOperation` via the new
`handler_sdk_route_table_test.go` (`TestExtractOperation_SDKRouteTable`, one
subtest per op, `t.Parallel()`). All 48 resolved correctly, including the
real AWS `DeleteRepositoryPermissionsPolicy` singular/plural path quirk
(already deliberately handled with a doc comment) and every
same-path/different-method collision (`/v1/domain`, `/v1/repository`,
`/v1/package` each serve three methods). None of this service's ops carry a
URI-label path parameter, so this table needed no PLACEHOLDER substitution
at all. No pre-existing table existed to check, and no new routing bugs
found — this pass's earlier route-matcher audits (see below) already
covered this ground. This test is now the permanent regression guard for
route-table drift.

### 2026-08-07 pass, addendum: two severe bugs found only by adding SDK-driven integration tests

While implementing the weak-match feature (below), `test/integration/codeartifact_test.go` gained
a new `TestIntegration_CodeArtifact_PackageGroupWeakMatch` test that drives `CreatePackageGroup`
and `PublishPackageVersion` through the real `aws-sdk-go-v2` client rather than this package's
own hand-built-request unit tests. It immediately failed twice, in sequence, surfacing two
client-breaking bugs that **three prior audit passes** (including a dedicated line-by-line
route-matcher audit that explicitly claimed "all other op paths/methods verified correct") had
missed entirely:

1. **`CreatePackageGroup`'s request body read the wrong JSON key.** The handler expected
   `{"pattern": "..."}`; the real wire key (confirmed in `serializers.go`) is
   `{"packageGroup": "..."}`. A real client's `CreatePackageGroup` call always failed with
   `pattern is required`, for every domain, unconditionally — since the very first byte of the
   request body onward, this op could never have been called successfully by anything but this
   package's own unit tests (which built requests using the same wrong key).
2. **`PublishPackageVersion`'s registered path was pluralized wrong**:
   `/v1/package/versions/publish` instead of the real `/v1/package/version/publish`. A real
   client's call 404'd outright (`UnknownOperationException`) before ever reaching the handler —
   meaning every one of this op's many previously-audited "fixes" (flat response shape, real
   asset storage, npm `package.json` readme/dependency extraction) were sitting behind a route
   that no real SDK client could ever reach.

Both are now exactly the class of bug `parity-principles.md` rule 3 warns about: invisible to
`go test` because every unit test in this package builds its own request body/path by hand and
happened to match the bug, not the real wire. Fixed both (see `CreatePackageGroup`/
`PublishPackageVersion` ops rows) and updated ~40 unit-test call sites across
`handler_package_groups_test.go`, `handler_package_groups_list_test.go`,
`handler_packages_test.go`, `handler_package_versions_test.go`,
`handler_package_versions_assets_test.go`, and `persistence_test.go` to the real wire shapes.
This is the strongest argument yet in this file for keeping (and growing) the SDK-driven
integration suite rather than trusting a manual line-by-line audit alone — the manual audit
that specifically covered this exact path/method surface still missed both.

### 2026-08-07 pass: package-group weak match (gopherstack-u9e5)

Implemented the "weak match" half of AWS's package-group matching algorithm that the
2026-07-23 pass explicitly deferred (see [Strong and weak
match](https://docs.aws.amazon.com/codeartifact/latest/ug/package-group-definition-syntax-matching-behavior.html#package-group-strong-and-weak-match)
and "Additional variations" on the same page, fetched this pass for the exact documented
rules and worked examples). Two of the three documented weak-match rules are implemented:

1. **Casefolding** (`normalizeWeak` in `package_group_pattern.go`, via `strings.ToLower` —
   AWS's own docs frame its casefolding as "similar to converting to lowercase", not full
   Unicode case-fold, so this matches the documented behavior for every worked example).
2. **Dash/dot/underscore-run collapsing**: any run of `-`, `.`, `_` normalizes to a single
   `.`, so `foo-bar`/`foo.bar`/`foo..bar`/`foo_bar` all normalize identically while `foobar`
   (no separator) stays distinct — matches the doc's own worked example set exactly.

`bestMatchingGroup` (`package_groups.go`) now selects the most-specific matching group using
the *weak*-match space (a strict superset of the strong-match space, since
`normalizeWeak(x) == normalizeWeak(x)` trivially), then classifies the result as `STRONG`
(the package's literal coordinate strong-matches the winning pattern) or `WEAK` (it only
matches after normalization). This mirrors AWS's documented behavior that a weak-matching
package variant stays attached to the *same* most-specific group rather than rolling up to
a broader, less specific one. `GetAssociatedPackageGroup` and `ListAssociatedPackages` both
now report the real classification (previously hardcoded `"STRONG"` unconditionally, a
disguised-stub field found while scoping this work — see the `gaps` entry it corresponds to
being closed on the ops table).

**Not implemented**: the third weak-match rule, confusable-character normalization (e.g. a
Cyrillic character that renders identically to a Latin one). This needs the real Unicode
confusables table (`confusables.txt`), sizeable external data this pass did not have room to
vendor and verify correctly — genuinely buildable (not structural), left in `gaps` rather
than promoted to `structural_gaps`, per this repo's classification rule that "large" and
"structural" are not the same thing.

**Found while scoping (bug, not this pass's assigned feature)**: origin-restriction
enforcement — the actual point of computing STRONG/WEAK in the first place, per AWS's docs
("the package is blocked instead of applying the group's origin control configuration" for
a WEAK match) — does not exist anywhere in this backend. `PublishPackageVersion` and package
ingestion never look up a package's associated group or consult its restrictions at all, for
either association type. The classification is now real; the enforcement it exists to drive
is a separate, materially larger gap (see `gaps`).

### 2026-07-23 pass: package-group pattern matching + readme/dependency extraction + a leak fix

The prior audit (2026-07-13) explicitly deferred the package-group pattern-matching algorithm as
"a real feature, not a quick wire fix — needs its own pass." That pass happened this round:
`package_group_pattern.go` implements AWS's documented pattern syntax/matching/hierarchy algorithm
(see [Package group definition syntax and matching
behavior](https://docs.aws.amazon.com/codeartifact/latest/ug/package-group-definition-syntax-matching-behavior.html)),
including the non-obvious parts — word-boundary prefix matching (`~`), the target-position/suffix
parsing model, and the proper-subset relation that defines "most specific match" and parent/child
hierarchy. It's covered by its own white-box test file
(`package_group_pattern_test.go`, `package codeartifact` with a `//nolint:testpackage`, same
convention as `isolation_test.go`) pinning every documented example pattern plus edge cases. This
powers real (non-stub) `GetAssociatedPackageGroup`, `ListAssociatedPackages`, `ListSubPackageGroups`,
and `UpdatePackageGroupOriginConfiguration`'s INHERIT-chain resolution — see the ops table for what
changed in each. Deliberately NOT implemented: the "weak match" half of the algorithm
(case-folding/separator-equivalence used for dependency-confusion protection) and the implicit
root `/*` group every real domain has — both are real, scoped, documented gaps (see `gaps:`), not
silently dropped.

`GetPackageVersionReadme`/`ListPackageVersionDependencies` also went from permanent stubs to real
extraction from a published `package.json` asset (npm convention) — see the ops table. This is
intentionally scoped to that one file name/format rather than attempting to unpack arbitrary
tarballs/POMs, which is out of reach for this backend's single-asset-per-call publish model.

**Leak found and fixed**: `DeleteDomain`'s cascade-delete never touched package groups — every
other owned resource (repositories, packages, versions, policies, external connections) was
cleaned up, but package groups (keyed by `domainName+pattern`, not by repository, so outside the
existing per-repository cascade loop) were left behind as ghost store rows with their `Tags` never
`Close()`d. Fixed by adding an explicit package-group cascade loop to `DeleteDomain` (see
`domains.go`).

**Traps for the next auditor (new this pass)**:
- `ListAllowedRepositoriesForGroup`'s `originRestrictionType` query param is genuinely camelCase
  on the wire (verified against `serializers.go`'s `SetQuery("originRestrictionType")`), unlike
  every other package-group query param in this service (which are kebab-case). Do not "fix" this
  to kebab-case.
- The package-group hierarchy (parent/child, effective-mode inheritance) is derived purely from
  pattern structure (`isProperSubsetPattern`/`specificityRank` in `package_group_pattern.go`), not
  from any stored parent pointer — a group's parent can change implicitly when a sibling group is
  created or deleted. This matches real AWS (the hierarchy is defined by pattern specificity, not
  an explicit tree), but means `DescribeOriginInfo` re-scans the domain's groups on every call
  rather than caching a parent reference.
- `DescribePackage`/`DescribePackageVersion`'s auto-create-on-Describe divergence was
  re-investigated (not just re-asserted) this pass specifically to see if it was now safe to remove
  — it is not: 60+ existing test call sites across five files rely on `GET .../package/version` as
  their primary seeding mechanism, not `PublishPackageVersion`. Removing it is real work (rewrite
  every one of those call sites to publish first) that deserves its own pass, not a footnote in this
  one.

Protocol: **restjson1**. Timestamps are epoch-seconds JSON numbers (`awstime`-style, hand-rolled
here via `epochSeconds`), not ISO8601 strings — this was already correct except for the
`publishedAt`→`publishedTime` key-name bug (see ops table).

**The big finding this pass**: query-string parameter casing. AWS's CodeArtifact Smithy model
uses kebab-case (`max-results`, `next-token`, `package-group`, `external-connection`,
`source-repository`, `destination-repository`) for httpQuery-bound members on most ops, but this
service's handlers read camelCase (`maxResults`, `nextToken`, `packageGroup`, ...). Every unit
test in this package constructs its own query strings by hand and matched the handler's (wrong)
camelCase expectation, so the bug was invisible to `go test` — it only breaks when driven by a
real `aws-sdk-go-v2` client, exactly the trap `parity-principles.md` rule 3 warns about
("unit tests are not parity proof"). `ListDomains` is the one op where pagination is a JSON body
field instead of a query param at all — an easy thing to miss if you pattern-match against the
other List ops.

**Route-matcher bugs** (rule explicitly named in the audit brief): 5 ops had incorrect path
and/or method wiring, meaning a real SDK request would never reach the intended handler (falling
through to `opUnknown` → 404 "unknown operation", or in `PutPackageOriginConfiguration`'s case,
being silently unroutable since `/v1/package/origin-configuration` never existed in the real API
at all — it's `POST /v1/package`, sharing a path with `DescribePackage`/`DeletePackage`).
Unit tests calling `h.Handler()(c)` directly with hand-built paths did not exercise
`RouteMatcher()`/`parseCodeArtifactPath` against the real paths, so this was invisible too.

**Disguised no-ops** (rule 4 / rule 1 no-stub rule): `PutPackageOriginConfiguration`'s backend
method built a `*Package` return value but never called `b.packages.Put(...)` — every call
looked like it succeeded but the origin configuration was never actually stored, so a subsequent
`DescribePackage` would never reflect it. `PublishPackageVersion`'s asset upload was silently
discarded twice over: the HTTP layer's blanket "try to JSON-decode every body" logic errors out
(and is swallowed) on binary octet-stream content, and even if it hadn't, the handler never read
the `asset` query param or passed the body through. `GetPackageVersionAsset`/
`ListPackageVersionAssets` were pure stubs returning empty regardless of what (if anything) had
been published. All four are fixed together as one coherent asset-storage feature (see ops table).

**Traps for the next auditor** (looks-wrong-but-correct):
- `UpdatePackageGroup`'s query-string fallback for `packageGroup` (still camelCase, technically
  wrong per the real wire format) is *harmless* dead code for real traffic: the real SDK always
  sends `packageGroup` in the JSON body for this op specifically (verified — it's the one
  package-group op where the identifier is a body field, not query), and the handler already
  falls back to the body value when the query lookup misses. Do not "fix" this to kebab-case;
  that would break nothing further but is pointless — leave it as documented.
- `DescribePackage`/`DescribePackageVersion` auto-creating a stub entry on first read is
  intentional pre-existing behavior (explicit comments: "stub entries are created on demand"),
  not a bug introduced this pass. It IS a real divergence from AWS (which 404s), logged as a gap,
  but ripping it out would be a large, risky behavioral change affecting many existing tests and
  was out of scope for this pass.
- `ListPackages` derives its package list by scanning `packageVersions`, not the `packages` table
  directly — this is intentional (a "package" only meaningfully exists once it has a version) and
  is why `PublishPackageVersion` inserts into both tables.

## 2026-08-29 pass: campaign class audit (constraining parameter never honoured)

Measured 12 List operations against the pinned SDK (codeartifact@v1.41.4).
Most were already correctly filtered from prior passes (ListPackageVersions's
status/sortBy fix is noted in an earlier section of this file). One real
finding: **ListPackages** declares `packagePrefix`/`publish`/`upstream` as
query-bound filters (serializers.go's
`awsRestjson1_serializeOpHttpBindingsListPackagesInput`), none of which
`handleListPackages` read -- always returned every package in the repository
regardless of what was requested. Fixed by threading all three through to
`InMemoryBackend.ListPackages`, which now also looks up each package's real
stored `PackageOriginConfiguration` (previously synthesized fresh `Package`
values from `PackageVersion` records alone, with `OriginConfigPublish`/
`OriginConfigUpstream` always blank even when `PutPackageOriginConfiguration`
had set them) so publish/upstream filtering has real data to match against;
an unset origin config defaults to ALLOW/ALLOW, matching
`PackageOriginRestrictions`'s real default.

Decomposed into `listPackagesFilters` (packages.go) rather than adding a
`//nolint:gocognit` — matches this campaign's established pattern of a
per-op filter type over a complexity suppression.

Tests: `list_filter_params_test.go`, driven through the real SDK client
(`newTestCodeArtifactClient`) -- `TestListPackages_Filters` covers
packagePrefix and publish. Fails against pre-fix code (confirmed by
reverting packages.go/handler_packages.go only).

## 2026-08-31 directed sweep: request-key/silent-empty-default compound bug (gopherstack-uox6 territory)

Regenerated the campaign's plural-heuristic candidate list against
`codeartifact@v1.41.4/serializers.go` (quoted lowercase identifiers in
non-test `.go` files whose plural form appears in `serializers.go` while the
singular does not): only `versionRevision`/`versionRevisions`. Both hits
(`handler_package_versions.go:343,605`) are response-side output keys, not
request reads, and independently verified correct against
`deserializers.go`'s `PackageVersionSummary`/`PackageVersion` cases -- not a
bug, wrong axis for this heuristic.

Went beyond the heuristic: read every query-parameter and JSON-body decode
site in `handler.go`/`handler_*.go` against each operation's own
`awsRestjson1_serializeOpHttpBindings*Input`/`serializeOpDocument*Input` in
the pinned SDK (`ListPackages`, `ListPackageVersions`, `ListPackageGroups`,
`ListAllowedRepositoriesForGroup`, `ListRepositories`,
`ListRepositoriesInDomain`, `CreateArchiveRule`-equivalent domain/repo ops,
`CreateAccessPreview`-style body fields N/A to this service). One real
finding:

**`ListRepositoriesInDomain`'s `AdministratorAccount` filter was declared by
the real SDK (`serializers.go`'s `SetQuery("administrator-account")`,
`api_op_ListRepositoriesInDomain.go`: "Filter the list of repositories to
only include those that are managed by the Amazon Web Services account ID")
but never read at all** -- `handleListRepositoriesInDomain` decoded only
`max-results`/`next-token`/`repository-prefix`. Every repository this
backend creates is administered by the backend's own single account ID
(`repositories.go`'s `CreateRepository` always sets
`AdministratorAccount: b.accountID`), so a real client filtering by any
*other* account ID should get zero repositories back; the unfiltered handler
returned every repository in the domain regardless. Same shape as this
service's earlier `ListPackages`/`ListPackageVersions`/
`ListRepositoriesInDomain.RepositoryPrefix` fixes (a real, documented filter
member silently dropped, narrowing request returns everything instead of
the narrowed/empty set) -- not the wrong-key variant, the field-never-wired
variant of the same compound bug. Fixed by threading `administratorAccount`
through `InMemoryBackend.ListRepositoriesInDomain` (new fourth parameter)
and comparing it against each repository's stored `AdministratorAccount`
when non-empty.

**Checked and correctly left alone (not fabricated):** `ListFindings`/
policy-generation-style optional detail flags don't apply here; this
service's `ListRuleTypes`-equivalent has no analogue.
`ListGuardrails`/`PollForJobs`-style backend-data-gaps don't apply to this
service either (see bedrock/codepipeline notes in this campaign for the
same restraint pattern). `DomainOwner` filters across
`ListPackages`/`ListPackageVersions`/`ListPackageGroups`/
`ListRepositoriesInDomain`/`ListAllowedRepositoriesForGroup` are cross-account
domain-sharing fields this single-account backend has no data model for --
same class of honest gap as this file's pre-existing `originType` note, not
newly introduced.

Tests: `list_filter_params_test.go`'s new
`TestListRepositoriesInDomain_AdministratorAccountFilter`, driven through the
real SDK client, asserts both the matching-account case (1 repository) and
the non-matching-account case (0 repositories) -- the second assertion is
exactly what an unfiltered handler cannot pass. Confirmed failing against
unmodified code first (`require.Empty(t, nonMatching.Repositories)` failed,
returning the one repository) before implementing the fix. 1 test added, 0
existing assertions dropped or weakened.

Gates: `go build`, repo-wide `go vet` (clean, no cross-service callers of
`ListRepositoriesInDomain` outside this package), `go test -race -count=1`,
`go fix -diff` (no diff), `golangci-lint run` (0 issues after wrapping one
golines-flagged call to the new four-argument signature) -- all clean
(`./services/codeartifact/...`). No `//nolint:cyclop/gocyclo/gocognit/funlen`
added (repo-wide grep confirms 0 across all four services in this session's
scope).
