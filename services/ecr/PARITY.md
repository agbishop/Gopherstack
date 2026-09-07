---
service: ecr
sdk_module: aws-sdk-go-v2/service/ecr@v1.60.4
last_audit_commit: fba3c784              # this pass's changes are uncommitted working-tree edits; see Notes
last_audit_date: 2026-08-15
overall: A  # round 4 (gopherstack-6flj wrapper-key sweep) found and fixed 6 more real wire-shape bugs the round-3 "wire: ok" claims had missed -- see "Genuine fixes made this pass, round 4" below. Round 3 closed every remaining gap it found: item for real (not by weakening tests) -- see "Genuine fixes made this pass, round 3" below. All 6 previously-deferred error/behavior gaps now enforced with passing tests, plus the previously out-of-scope ListPullTimeUpdateExclusions pagination gap.
ops:
  CreateRepository: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeRepositories: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRepository: {wire: ok, errors: ok, state: ok, persist: ok, note: "force-with-images enforced in handler via DescribeImages pre-check"}
  PutImage: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — the 'image' response object was the raw internal Image domain struct, leaking 5 gopherstack-only fields (imageDigest, imagePushedAt, imageStatus, storageClass, imageSizeInBytes) not present on the real ecr.types.Image shape (imageId/imageManifest/imageManifestMediaType/registryId/repositoryName only, per awsAwsjson11_deserializeDocumentImage); imagePushedAt was also a bare time.Time (RFC3339 string) though moot since the field itself was invented. Fixed via a new imageView wire type; the digest remains available via the correct nested imageId.imageDigest. Also carries the round-1 ImageDigestDoesNotMatchException fix. FIXED (round 3) — ImageAlreadyExistsException now enforced: re-pushing an unchanged manifest+tag pair is rejected regardless of repository tag mutability (see round 3 notes)."}
  BatchGetImage: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — same invented-field leak as PutImage (Images []Image → []imageView); see PutImage note. FIXED (round 3) — now stamps lastRecordedPullTime on every returned image (see DescribeImages note); takes the write lock accordingly."}
  BatchDeleteImage: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeImages: {wire: ok, errors: ok, state: ok, persist: ok, note: "core fields (imageDigest, imageTags, imagePushedAt as epoch, imageSizeInBytes, imageManifestMediaType, imageStatus, registryId, repositoryName) verified correct via imageDetailView. FIXED (round 3) — the 7 previously-missing ImageDetail fields are now implemented: artifactMediaType/subjectManifestDigest are parsed from the pushed manifest's OCI 1.1 artifactType/subject.digest fields; imageScanFindingsSummary/imageScanStatus are annotated from the imageScanFindings store (present only for images that have actually been scanned); lastActivatedAt/lastArchivedAt are stamped by UpdateImageStorageClass; lastRecordedPullTime is stamped by BatchGetImage and GetDownloadUrlForLayer (the latter via a manifest-text substring match against the requested layer digest, since the backend does not otherwise model a per-image layer list)."}
  ListImages: {wire: ok, errors: ok, state: ok, persist: ok}
  ListImageReferrers: {wire: ok, errors: ok, state: ok, persist: ok, note: "STRUCTURAL GAP (round 4, disclosed not fixed) -- 'wire: ok' overstated: the real ListImageReferrersInput/Output also carry Filter/MaxResults/NextToken, omitted here because PutImage never records an OCI-referrer edge from a pushed artifact's manifest 'subject' back to the subject image, so this op is structurally always empty regardless of those fields' presence; adding them would be a schema-only change with nothing to ratify. Referrer-relationship tracking itself is the real gap, out of scope for a wire-shape fix. FIXED (2026-09-07, gopherstack-jqxg errtargetaudit class A finding) -- 'errors: ok' was also overstated: a non-existent subject digest returned ImageNotFoundException, but deserializeOpErrorListImageReferrers declares no such exception for this op (only RepositoryNotFoundException/UnableToListUpstreamImageReferrersException/InvalidParameterException/ServerException/ValidationException/UnknownError) -- a real SDK client got an untyped generic error instead of a typed one. Consistent with the op being structurally always-empty above: an unknown subject now returns 200 with an empty referrers list, matching the declared error set. Two pre-existing tests (TestOps2_ListImageReferrers_SubjectNotFound_ImageNotFoundException, TestListImageReferrers_NonExistentImage_Errors) asserted the wrong (error) behavior with no note disclosing it as a known shortcut; both rewritten to assert the 200/empty-list outcome, and a new TestOps2_ListImageReferrers_RepositoryNotFound_Errors added to pin the sibling RepositoryNotFoundException path (which the op does legitimately declare) now that the subject-not-found guard is gone."}
  BatchCheckLayerAvailability: {wire: ok, errors: ok, state: ok, persist: ok}
  InitiateLayerUpload: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIFO TTL pruning bounds layerUploads/layerUploadQueue"}
  UploadLayerPart: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — added part-sequencing validation (InvalidLayerPartException) for non-consecutive partFirstByte. FIXED (round 3) — now records each part's size on the upload session so CompleteLayerUpload can enforce the 5MiB minimum-part-size rule (LayerPartTooSmallException) against every part but the last. FIXED (round 3, genuinely new finding) — an unknown/wrong-repository uploadId incorrectly returned RepositoryNotFoundException (404); real AWS returns UploadNotFoundException (400) per UploadLayerPart's documented Errors list. Found while re-verifying this exact code path for the CompleteLayerUpload UploadNotFoundException gap; TestECR_RestoreClearsInFlightLayerUploads previously asserted the wrong (404) status and was corrected."}
  CompleteLayerUpload: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — was missing RepositoryNotFoundException FK check (unlike every other op) and never rejected re-completing an already-registered layer digest (LayerAlreadyExistsException). FIXED (round 3) — the 'direct digest' fallback path (accepting any uploadId+digest with no live session) is removed: CompleteLayerUpload now requires a live InitiateLayerUpload session scoped to the given repository (UploadNotFoundException otherwise), that session to have received at least one UploadLayerPart call (EmptyUploadException otherwise), and every part but the last to be at least 5MiB (LayerPartTooSmallException otherwise, enforced via new per-part-size bookkeeping on the upload session). The ~9 test call sites that relied on the old direct-digest shortcut as a seeding convenience were rewritten to perform a real Initiate→UploadPart→Complete flow via new mustUploadLayer/mustUploadLayerHTTP helpers."}
  GetDownloadUrlForLayer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 3) — now stamps lastRecordedPullTime (see DescribeImages note) on every image in the repository whose manifest references the requested layer digest; takes the write lock accordingly."}
  GetAuthorizationToken: {wire: ok, errors: ok, state: ok, persist: n/a, note: "base64(AWS:dummy-password), 12h TTL, proxyEndpoint derived from first request Host"}
  CreatePullThroughCacheRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribePullThroughCacheRules: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePullThroughCacheRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePullThroughCacheRule: {wire: ok, errors: ok, state: ok, persist: ok}
  ValidatePullThroughCacheRule: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreateRepositoryCreationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeRepositoryCreationTemplates: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 4) -- the real DescribeRepositoryCreationTemplatesInput/Output carry maxResults/nextToken (confirmed against the real api_op file); this handler discarded both, always returning every template in one page. Now paginates via the same base64(prefix)-cursor convention used by DescribeRepositories/DescribePullThroughCacheRules elsewhere in this package. FIXED (2026-09-07, gopherstack-jqxg errtargetaudit class A finding) -- 'errors: ok' was also overstated: a prefix in the Prefixes filter with no matching template returned TemplateNotFoundException, but deserializeOpErrorDescribeRepositoryCreationTemplates declares no such exception for this op (only InvalidParameterException/ServerException/ValidationException/UnknownError) -- unlike its siblings DeleteRepositoryCreationTemplate/UpdateRepositoryCreationTemplate, which both genuinely declare TemplateNotFoundException and were left unchanged. This handler had evidently been cloned from the identically-shaped DescribePullThroughCacheRules (which DOES declare PullThroughCacheRuleNotFoundException and correctly errors on an unmatched prefix) without checking that DescribeRepositoryCreationTemplates' own declared error set differs. Fixed: an unmatched prefix is now silently omitted from the result instead of erroring, matching the declared set. No pre-existing test exercised this path; two new tests added (TestDescribeRepositoryCreationTemplates_UnmatchedPrefix_FilteredNotError, TestDescribeRepositoryCreationTemplates_AllPrefixesUnmatched_ReturnsEmpty)."}
  UpdateRepositoryCreationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRepositoryCreationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  PutLifecyclePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "applied immediately on Put, matching AWS's immediate evaluation. FIXED (round 2) — lastEvaluatedAt was a bare time.Time returned directly on the domain struct (RFC3339 string on the wire); real GetLifecyclePolicyOutput.lastEvaluatedAt deserializes via smithytime.ParseEpochSeconds(json.Number). Fixed via lifecyclePolicyResultView (epoch float64), same convention as repositoryView.createdAt."}
  GetLifecyclePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — see PutLifecyclePolicy note"}
  DeleteLifecyclePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — see PutLifecyclePolicy note"}
  StartLifecyclePolicyPreview: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — previewResults was []ImageIdentifier (imageDigest/imageTag only); real GetLifecyclePolicyPreviewOutput.previewResults is []types.LifecyclePolicyPreviewResult carrying action{type}, appliedRulePriority, imageDigest, imagePushedAt (epoch), imageTags, storageClass — entirely missing action/priority/pushedAt/storageClass, and the top-level summary.expiringImageTotalCount field was absent too. Fixed: evaluateLifecyclePolicy now returns []LifecyclePolicyPreviewEntry carrying the full AWS-shaped detail. FIXED (round 3, genuinely new finding) — Start's own response was ALSO wrong: it reused the same lifecyclePolicyPreviewView as Get and therefore leaked previewResults/summary into Start's response, but direct diff of StartLifecyclePolicyPreviewOutput's real deserializer shows Start returns ONLY lifecyclePolicyText/registryId/repositoryName/status -- no previewResults/summary/nextToken at all (those belong to Get only). Fixed via a new, narrower lifecyclePolicyPreviewStartView. Start genuinely never had a Filter/ImageIds/MaxResults/NextToken gap in the first place (StartLifecyclePolicyPreviewInput has no such fields in the real SDK) -- the prior audit's gap note conflated Start and Get."}
  GetLifecyclePolicyPreview: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — see StartLifecyclePolicyPreview note. FIXED (round 3) — Filter (tagStatus)/ImageIds/MaxResults/NextToken are now implemented at the handler layer (post-fetch filtering/pagination over the backend's full preview result, mirroring the DescribeImages/ListImages pattern): ImageIds restricts to exactly those images and (per the real API doc) is mutually exclusive with Filter/MaxResults/NextToken; otherwise Filter.tagStatus (TAGGED/UNTAGGED/ANY) filters and MaxResults/NextToken (default 100) paginate via the same base64(imageDigest)-cursor convention used elsewhere in this package."}
  GetRepositoryPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  SetRepositoryPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "'wire: ok' overstated (gopherstack-wks5 field-identity sweep, 2026-08-30): SetRepositoryPolicyInput's real Force bool member (api_op_SetRepositoryPolicy.go -- \"you must force the operation\" to override the lockout safety check) is parsed into repositoryPolicyInput.Force but never passed to Backend.SetRepositoryPolicy, which takes only (ctx, repositoryName, policyText). Not fixed: real Force gates a self-lockout evaluation (would the new policy deny the caller SetRepositoryPolicy/GetRepositoryPolicy in future) that requires an IAM policy-simulation engine this repo has no pkgs/ package for -- out of scope for a wire-identity fix, disclosed for a future pass. See efs PARITY.md's PutFileSystemPolicy entry for the identical pattern (BypassPolicyLockoutSafetyCheck)."}
  DeleteRepositoryPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRegistryPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — RegistryPolicyResult carried a gopherstack-invented 'status' field (\"ACTIVE\"); the real GetRegistryPolicyOutput/PutRegistryPolicyOutput/DeleteRegistryPolicyOutput shapes have only policyText+registryId. Field deleted."}
  PutRegistryPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — same invented 'status' field (\"SetComplete\") deleted; see GetRegistryPolicy note"}
  DeleteRegistryPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 2) — same invented 'status' field (\"DELETED\") deleted; see GetRegistryPolicy note"}
  DescribeRegistry: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRegistryScanningConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 4) -- registryId was declared on the wire struct but never populated (always emitted \"\" instead of the real account ID), unlike sibling ops DescribeRegistry/GetRegistryPolicy/PutRegistryPolicy/DescribeRepositoryCreationTemplates which all set it correctly. Now set from Backend.AccountID()."}
  PutRegistryScanningConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FLAGSHIP FIX (round 4) -- this op's response reused getRegistryScanningConfigurationOutput (Get's shape: wrapper key \"scanningConfiguration\" + top-level registryId), but the real PutRegistryScanningConfigurationOutput wraps under \"registryScanningConfiguration\" with NO registryId field at all (confirmed by direct diff of both ops' own awsAwsjson11_deserializeOpDocument...Output functions). A real SDK client parsing gopherstack's old response silently got a nil RegistryScanningConfiguration on every 200 response -- exactly the class of bug this issue hunts, hiding behind an at-first-glance-symmetric Get/Put pair. Fixed via a dedicated putRegistryScanningConfigurationOutput type. An existing raw-body test (TestPutRegistryScanningConfiguration_ScanTypeEnhanced) asserted the wrong key as correct and was rewritten."}
  BatchGetRepositoryScanningConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 4) -- RepositoryScanningConfiguration was missing appliedScanFilters entirely (real types.RepositoryScanningConfiguration field); when an ENHANCED registry's CONTINUOUS_SCAN rule matches a repo, the matching rule's RepositoryFilters are now surfaced there too (repoEffectiveScanFrequency extended to return both)."}
  PutImageScanningConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 4) -- registryId declared on the wire struct but never populated; same pattern as GetRegistryScanningConfiguration above. Now set from Backend.AccountID()."}
  PutImageTagMutability: {wire: ok, errors: ok, state: ok, persist: ok, note: "exclusion filters (WILDCARD + literal) enforced correctly"}
  StartImageScan: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeImageScanFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "BASIC vs ENHANCED finding shapes genuinely differ; paginated via index-based nextToken; ScanNotFoundException for never-scanned images. FIXED (round 2) — ImageScanFindingsResult.completedAt was a bare time.Time under the WRONG key entirely: real ecr.types.ImageScanFindings has no 'completedAt' field at all; the real key is 'imageScanCompletedAt' (epoch seconds, per awsAwsjson11_deserializeDocumentImageScanFindings), plus a second field 'vulnerabilitySourceUpdatedAt' that gopherstack didn't emit at all. A real SDK client parsing gopherstack's old response would silently get a nil/zero ImageScanCompletedAt (unknown JSON keys are ignored, so no hard failure, but the field was simply never populated client-side). Fixed: renamed to ImageScanCompletedAt/VulnerabilitySourceUpdatedAt (float64, epoch seconds); VulnerabilitySourceUpdatedAt is only populated for ENHANCED scans (BASIC omits it, matching AWS's Inspector-only semantics for that field). FIXED (round 4) — the nested \"imageScanFindings\" object reused ImageScanFindingsResult wholesale, so it ALSO leaked imageId/repositoryName/registryId/status/description (the output's own top-level fields) into the nested object; the real nested ImageScanFindings type has only 5 fields, none of those. Harmless to a real client (unknown keys ignored) but a wire-shape imprecision; fixed via a purpose-built imageScanFindingsView. FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- ImageScanFinding.Attributes was a bare map[string]string; the real ImageScanFinding.Attributes deserializes via awsAwsjson11_deserializeDocumentAttributeList, a list of {key, value} objects (types.Attribute), so any real SDK client's decode failed outright once a BASIC scan finding carried attributes (always true -- buildBasicFindings seeds package_name/package_version on every finding). Not a dropped field or wrong value: DescribeImageScanFindings was unusable for BASIC scans. Fixed by adding an Attribute{Key, Value string} type and changing ImageScanFinding.Attributes to []Attribute; proven via a real aws-sdk-go-v2/service/ecr client round trip (wire_scan_finding_attributes_test.go), hand-reverted/confirmed-failing (unexpected JSON type map[package_name:... package_version:...])/restored, md5sum-verified byte-identical."}
  PutReplicationConfiguration: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-uox6 (2026-08-30): repoMatchesFilters switched on \"PREFIX\", but the real RepositoryFilterType enum's only value is \"PREFIX_MATCH\" (types/enums.go:385) -- \"PREFIX\" is not a real AWS value for either type sharing this internal RepositoryFilter struct. A real client's PREFIX_MATCH replication filter fell through the switch entirely and matched no repository, silently disabling prefix-filtered replication. Fixed by correcting the case string; the pre-existing replication_test.go fixture that used the fabricated \"PREFIX\" value (and only ever exercised the negative/non-matching case, so it never caught this) was corrected to PREFIX_MATCH and a new positive-match test added."}
  DescribeImageReplicationStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSigningConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 4) -- registryId omitted entirely; real GetSigningConfigurationOutput has it (unlike PutSigningConfigurationOutput, which genuinely lacks it -- three siblings, two shapes, confirmed against each op's own deserializer). Now set from Backend.AccountID()."}
  PutSigningConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified (round 4) -- correctly has no registryId, matching the real PutSigningConfigurationOutput shape; see GetSigningConfiguration/DeleteSigningConfiguration notes for the sibling contrast."}
  DeleteSigningConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 4) -- same registryId gap as GetSigningConfiguration (real DeleteSigningConfigurationOutput also has it); fixed the same way."}
  DescribeImageSigningStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateImageStorageClass: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (round 3) — now stamps lastArchivedAt/lastActivatedAt (see DescribeImages note) on ARCHIVE/re-activate transitions respectively."}
  GetAccountSetting: {wire: ok, errors: ok, state: ok, persist: ok}
  PutAccountSetting: {wire: ok, errors: ok, state: ok, persist: ok}
  RegisterPullTimeUpdateExclusion: {wire: ok, errors: ok, state: ok, persist: ok}
  DeregisterPullTimeUpdateExclusion: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPullTimeUpdateExclusions: {wire: ok, errors: ok, state: ok, persist: ok, note: "RE-VERIFIED round 2: pullTimeUpdateExclusions: []string wire shape itself is correct (confirmed against real ListPullTimeUpdateExclusionsOutput — no per-item createdAt on this op, only a flat ARN list). FIXED (round 3) — MaxResults/NextToken now implemented at the handler layer (base64(principalArn)-cursor pagination over the sorted exclusion list, default maxResults 100, matching the real API doc)."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  registry-v2-proxy: {status: ok, note: "docker distribution/v3 in-memory storage driver embedded for /v2/ blob+manifest paths; ExtractResource avoids buffering upload bodies"}
  lifecycle-evaluation: {status: fixed, note: "gopherstack-uox6 (2026-08-30): this line previously claimed \"ok\" while covering only 2 of 4 documented countTypes and 1 of 2 documented action types -- a false-ok PARITY claim, not a disclosed gap. action.type==\"transition\" (targetStorageClass=\"archive\") was skipped entirely by evaluateLifecyclePolicy's top-level gate (only \"expire\" passed), so ANY archive rule -- under any countType, not just the two below -- matched and did nothing; countType sinceImagePulled and sinceImageTransitioned had no case in applyRule's switch and silently matched zero images despite the backend already tracking every field they need (LastRecordedPullTime/LastActivatedAt/LastArchivedAt, stamped by UpdateImageStorageClass/BatchGetImage/GetDownloadUrlForLayer). All three implemented per docs.aws.amazon.com/AmazonECR/latest/userguide/lifecycle_policy_examples.html's JSON template and worked examples: transition rules now archive (not delete) the image via the same StorageClass/ImageStatus/LastArchivedAt transition UpdateImageStorageClass performs; sinceImagePulled uses the documented 3-way fallback (LastRecordedPullTime, else LastActivatedAt if restored-and-not-repulled-since, else ImagePushedAt if never pulled); sinceImageTransitioned only considers already-archived images, thresholded on LastArchivedAt. Also added the general \"storageClass\" selection filter (a sibling of tagStatus in the policy template, not exclusive to sinceImageTransitioned) and threaded targetStorageClass through the preview response's action object (previously only echoed type). Now: priority-ordered rules, all 4 documented count types (imageCountMoreThan/sinceImagePushed/sinceImagePulled/sinceImageTransitioned), both documented action types (expire/transition), tagStatus any/tagged/untagged with prefix+wildcard pattern matching, storageClass standard/archive selection filtering, janitor sweeps on a timer independent of API calls. Tests: TestLifecycle_ArchiveAction_TransitionsStorageClass, TestLifecycle_SinceImagePulled_FallbackChain, TestLifecycle_SinceImageTransitioned_OnlyArchivedImages, TestLifecycle_Selection_StorageClass_GeneralFilter (lifecycle_archive_pulled_transitioned_test.go), each confirmed failing pre-fix."}
  mock-scanning: {status: ok, note: "deterministic per-digest CVE selection (sha256-seeded bitmask) so repeated scans of the same image are stable; BASIC and ENHANCED shapes are genuinely different data, not the same list reshaped"}
gaps:
  - "ListImageReferrers (round 4, disclosed): PutImage never records an OCI-referrer edge from a pushed artifact manifest's 'subject' field back to the subject image, so this op is structurally always empty. Real AWS returns actual referrer artifacts here; gopherstack has no backing model for the relationship at all. Filter/MaxResults/NextToken deliberately left off the wire structs since there is nothing for them to affect."
  - "SetRepositoryPolicy Force (gopherstack-wks5, 2026-08-30): see the ops entry above -- disclosed, not fixed, crosses into IAM policy simulation."
  - "RegistryId (gopherstack-wks5, 2026-08-30, structural, not a per-op bug): a type-identity field scan (go/types, matching decode-target struct fields by object identity rather than name, covering every op registered via service.WrapOp -- the generic JSON-protocol dispatcher whose reflection-based decode is invisible to a literal Bind()/Unmarshal() grep) found the optional registryId request field parsed but never consulted in ~23 input structs across nearly every op (BatchCheckLayerAvailability, BatchDeleteImage, BatchGetImage, CompleteLayerUpload, DeleteLifecyclePolicy, DeletePullThroughCacheRule, DeleteRepository, DescribeImageScanFindings, GetDownloadUrlForLayer, GetLifecyclePolicy, GetLifecyclePolicyPreview, ListImageReferrers, ListImages, PutImage, PutImageScanningConfiguration, PutImageTagMutability, PutLifecyclePolicy, GetRepositoryPolicy/SetRepositoryPolicy/DeleteRepositoryPolicy, UpdateImageStorageClass, UpdatePullThroughCacheRule, UploadLayerPart, ValidatePullThroughCacheRule). This is consistent across the entire service, not an isolated miss: gopherstack models exactly one account per backend instance and no op anywhere validates registryId against it, so accepting-and-ignoring a caller-supplied registryId that matches the caller's own account (the overwhelmingly common case -- registryId exists for rare cross-account resource-policy scenarios) is a no-op by construction, same reasoning as this file's own DeleteReplicationConfiguration-style single-account gaps in sibling services. The one behavioral edge this leaves open: a caller passing a registryId for a DIFFERENT (non-existent, in this single-account model) account currently still operates on the local account's resource instead of returning RepositoryNotFoundException/ImageNotFoundException, a narrow divergence from real cross-account semantics. Not fixed this pass -- would need a uniform per-op mismatch check across all ~23 sites, a design decision bigger than a wire-identity fix."
  # All other gaps documented through round 2 were closed for real in round 3
  # (2026-07-24), including the ImageAlreadyExistsException trigger condition
  # (previously deferred as unconfirmable without a live AWS account -- see
  # "Genuine fixes made this pass, round 3" below for how it was independently
  # confirmed from the real API doc text plus the moto ECR emulator's
  # reference implementation, converging on the same trigger condition). No
  # item was closed by weakening or deleting its blocking test; every
  # previously-"intentional shortcut" test was rewritten to exercise the real
  # AWS behavior instead. (bd: gopherstack-x6i closed)
deferred:
  - "docker registry v2 proxy internals (pkgs distribution/v3 wiring) — treated as a vendored subsystem, not re-audited this pass"
  - "chaos/fault-injection interaction with ECR ops — not exercised this pass"
leaks: {status: clean, note: "RE-VERIFIED round 3: layerUploads bounded by FIFO TTL queue pruned on InitiateLayerUpload; janitor (janitor.go) uses pkgs/worker.Group with ctx.Done() shutdown + g.Stop() drain; no unbounded maps found. This pass's new state (Image.LastActivatedAt/LastArchivedAt/LastRecordedPullTime) is plain fields on the existing Image struct, not a new map -- it is automatically cascade-deleted with the image and automatically included in Snapshot/Restore, no new lifecycle wiring needed. Image.ScanFindingsSummary/ScanStatus are transient, request-scoped annotations (json:\"-\", mirroring the pre-existing Tags field) recomputed fresh on every DescribeImages call, never persisted, never leaked. layerUploadState gained a PartSizes []int64 field (bounded by the same per-session lifecycle as the rest of the upload state it lives on, so it is retired/pruned identically). No new goroutines or lock paths: BatchGetImage and GetDownloadUrlForLayer now take the write lock (Lock) instead of RLock since they mutate LastRecordedPullTime, but neither introduces a new lock -- both already used the single coarse b.mu."}
---

## Notes

Protocol: JSON RPC 1.1 (`application/x-amz-json-1.1`, `X-Amz-Target:
AmazonEC2ContainerRegistry_V20150921.<Op>`). Timestamps are epoch-seconds JSON
numbers (verified against `awsAwsjson11_deserializeDocumentRepository` etc. in
the vendored SDK deserializer). CORRECTION (round 2): the prior audit's claim
that "this codebase already uses that convention throughout" was **not fully
true** — `LifecyclePolicyResult.LastEvaluatedAt` and
`ImageScanFindingsResult.CompletedAt` were bare `time.Time` fields returned
directly to the wire (RFC3339 strings), and `PutImage`/`BatchGetImage`'s
`Image` response leaked 5 gopherstack-only fields not in the real
`ecr.types.Image` shape. All fixed this pass — see the `PutImage`,
`GetLifecyclePolicy`, and `DescribeImageScanFindings` op notes above for
specifics. The general pattern (dedicated `*View` wire structs built by
`to*View()` conversion functions, e.g. `repositoryView`/`imageDetailView`) is
sound and is now applied consistently to every JSON-tagged `time.Time` field
in the package — reverify with:
`grep -rn 'time\.Time' services/ecr/*.go | grep -v _test.go` and confirm every
hit is either (a) consumed only by a `to*View` converter before reaching the
wire, or (b) purely internal (`layerUploadState`, `lifecycleLastEvaluated`
map) and never JSON-tagged.

This service had already been through multiple prior parity sweeps (test files
named `handler_accuracy_batch1/2`, `handler_refinement1/2`,
`handler_parity_ecr/ecr2`, `audit_ecr_test.go`, `replication_status_test.go`,
`scan_enhanced_test.go`, `lifecycle_expiry_test.go`, `leak_test.go` — ~17.9k LOC
total before this pass). Op-by-op review of `backend.go` confirmed FK
validation (repository/image/rule existence checks) present and correct on
every op **except** `CompleteLayerUpload`, which is the main finding this pass.

### Genuine fixes made this pass

1. **`CompleteLayerUpload` missing repository-existence check.** Every other
   backend method validates `b.repos[repositoryName]` before mutating state;
   `CompleteLayerUpload` did not, so completing an upload against a
   nonexistent repository silently "succeeded" instead of returning
   `RepositoryNotFoundException`. Also exposed a **pre-existing test bug**:
   `TestECR_CompleteLayerUpload` called `CompleteLayerUpload` against
   `"my-repo"`, which the test never created — it was accidentally relying on
   the missing FK check to pass. Fixed by creating the repo in the test setup
   (test now encodes the corrected/real behavior).

2. **`CompleteLayerUpload` missing `LayerAlreadyExistsException`.** AWS
   rejects re-completing a layer digest that is already registered as
   available in the repository (SDK doc: "The image layer already exists in
   the associated repository."). The backend silently overwrote
   `uploadedLayers[repo][digest]` on every call. Fixed by checking for an
   existing entry before writing (guarded against `digest == ""` so the
   pre-existing "empty session, no digest" completion path — exercised
   deliberately by an existing test — cannot self-collide).

3. **`UploadLayerPart` missing part-sequencing validation
   (`InvalidLayerPartException`).** AWS requires each part's `partFirstByte`
   to be consecutive to the number of bytes already received in the session
   ("the first byte specified is not consecutive to the last byte of a
   previous layer part upload"). The parameter was previously discarded
   (`_, lastByte int64`) entirely. Fixed by comparing `firstByte` against
   `upload.Size` and returning the new sentinel error on mismatch. No existing
   test exercises multi-part sequences with gaps, so this is purely additive.

4. **`PutImage` missing `ImageDigestDoesNotMatchException`.** AWS validates a
   caller-supplied `imageDigest` against the digest it computes from the
   manifest and rejects a mismatch. This codebase never checked it — any
   client-supplied digest was trusted verbatim. Implemented **at the JSON
   handler boundary** (`handlePutImage`), not inside
   `InMemoryBackend.PutImage`: dozens of existing unit tests call
   `Backend.PutImage` directly with synthetic, non-cryptographic `ImageDigest`
   values (e.g. `"sha256:abc111"`) that intentionally don't hash-match their
   paired `ImageManifest` text, using the digest purely as a unique key. Wire
   fixtures (`mustPutImage`, SDK-client tests) never set an explicit
   `imageDigest` on the request, so none of them exercise this new check; it
   only fires for a real client-supplied mismatch, matching where AWS itself
   performs the validation (request processing, not backend storage).
   Uncovered a **pre-existing test bug**: `TestECR_NewOps_PersistenceRoundTrip`
   sent `imageDigest: "sha256:persist123"` with `imageManifest: "{}"` — not a
   real sha256 of `"{}"`. Fixed by replacing the literal with the real
   `sha256("{}")` hex digest everywhere it's threaded through that test
   (5 occurrences: layer digest, image digest ×2, image-id lookups ×2).

### Deliberately NOT fixed (see `gaps` above for full rationale)

- `EmptyUploadException` — contradicted by an existing, clearly-intentional
  test.
- `ImageAlreadyExistsException` (idempotent-repush guard) — contradicted by an
  existing, clearly-intentional test; not in the task's explicit
  required-error-code list; moderate-confidence AWS-doc interpretation with
  test-suite-wide blast radius if wrong.
- `LayerPartTooSmallException` — needs a design for deferred (at-Complete-time)
  validation since "last part" isn't knowable at upload time; no test coverage
  either direction.

None of these were silently stubbed — they are explicitly called out as
deferred with the reasoning, per the no-stub principle's guidance to prefer an
explicit terminal note over a guessed half-fix.

### Genuine fixes made this pass, round 2 (2026-07-23)

The round-1 audit (2026-07-05) field-diffed error codes and the
already-fixed ops thoroughly but did not exhaustively re-diff every
JSON-tagged struct field name/type against the real SDK deserializers for
ops it marked `ok`. Independently re-diffing every op this pass (reading
`aws-sdk-go-v2/service/ecr@v1.59.0`'s `types/types.go` and `deserializers.go`
directly, not trusting the prior "ok" marks) surfaced four real wire-shape
bugs:

1. **`LifecyclePolicyResult.LastEvaluatedAt`** (used by
   `GetLifecyclePolicy`/`PutLifecyclePolicy`/`DeleteLifecyclePolicy`) was a
   bare `time.Time` returned directly to the JSON wire — `encoding/json`
   renders that as an RFC3339 string, but the real
   `GetLifecyclePolicyOutput.lastEvaluatedAt` deserializes via
   `smithytime.ParseEpochSeconds(json.Number)` and rejects strings. Fixed by
   adding `lifecyclePolicyResultView` (epoch `float64`), matching the
   `repositoryView.createdAt` convention already used elsewhere in the
   package. `LifecyclePolicyResult` itself is now purely an internal domain
   type (json tags removed from it).

2. **`ImageScanFindingsResult`** (`DescribeImageScanFindings`) had a
   `CompletedAt time.Time \`json:"completedAt"\`` field. The real
   `ecr.types.ImageScanFindings` has **no `completedAt` field at all** — the
   real key is `imageScanCompletedAt` (confirmed against
   `awsAwsjson11_deserializeDocumentImageScanFindings`), and a second field,
   `vulnerabilitySourceUpdatedAt`, was missing entirely. Both are epoch
   seconds. Fixed by renaming to `ImageScanCompletedAt`/
   `VulnerabilitySourceUpdatedAt` (`float64`); the latter is only populated
   for `ENHANCED` scans (BASIC omits it), matching that field's real
   Inspector-vulnerability-source semantics.

3. **`GetLifecyclePolicyPreview`/`StartLifecyclePolicyPreview`**'s
   `previewResults` was `[]ImageIdentifier` (`imageDigest`/`imageTag` only).
   The real `GetLifecyclePolicyPreviewOutput.previewResults` is
   `[]types.LifecyclePolicyPreviewResult`, a genuinely richer per-image shape
   carrying `action{type}`, `appliedRulePriority`, `imageDigest`,
   `imagePushedAt` (epoch), `imageTags`, and `storageClass` — plus a
   top-level `summary.expiringImageTotalCount` that gopherstack didn't emit
   at all. This is the largest fix this pass: `evaluateLifecyclePolicy`
   (lifecycle.go) now returns `[]LifecyclePolicyPreviewEntry` carrying the
   full detail instead of bare identifiers, and a new
   `lifecyclePolicyPreviewView`/`toLifecyclePolicyPreviewView` builds the
   AWS-shaped wire response including the summary count. Existing tests that
   only read `.ImageDigest` off preview entries kept compiling/passing
   unchanged since the new entry type preserves that field name.

4. **`PutImage`/`BatchGetImage`**'s `image`/`images` response fields
   serialized the raw internal `Image` domain struct, which carries 5
   gopherstack-only bookkeeping fields — `imageDigest`, `imagePushedAt`,
   `imageStatus`, `storageClass`, `imageSizeInBytes` — used internally by the
   *separate* `DescribeImages` `ImageDetail` shape. The real
   `ecr.types.Image` (confirmed via
   `awsAwsjson11_deserializeDocumentImage`) has exactly five fields:
   `imageId`, `imageManifest`, `imageManifestMediaType`, `registryId`,
   `repositoryName` — no digest/pushedAt/status/storageClass/size at the top
   level (the digest is available via the correctly-nested
   `imageId.imageDigest`, which was already present and correct). Real SDK
   clients silently ignore unknown JSON keys, so this wasn't a hard
   client-breaking bug, but it violates the "no gopherstack-invented fields"
   rule and the `imagePushedAt` sub-bug (bare `time.Time`) compounds it.
   Fixed via a new `imageView`/`toImageView`; the widely-used
   `mustPutImage` test helper (`handler_test.go`) was updated to read the
   digest from `image.imageId.imageDigest` instead of the deleted top-level
   `image.imageDigest`.

5. **`RegistryPolicyResult`** (`GetRegistryPolicy`/`PutRegistryPolicy`/
   `DeleteRegistryPolicy`) carried a `Status string \`json:"status"\`` field
   populated with fabricated values (`"ACTIVE"`, `"SetComplete"`,
   `"DELETED"`) that have no counterpart in the real
   `GetRegistryPolicyOutput`/`PutRegistryPolicyOutput`/
   `DeleteRegistryPolicyOutput` shapes (confirmed: those carry only
   `policyText`+`registryId`). Deleted per the "delete gopherstack-invented
   fields" directive; no test asserted on it.

Also independently re-verified (not fixed, see `gaps`): the three
round-1-deferred error codes are real SDK error types (confirmed by reading
`types/errors.go` directly — `EmptyUploadException`,
`ImageAlreadyExistsException`, `LayerPartTooSmallException` all genuinely
exist), and the test names cited as blockers in the round-1 PARITY.md were
stale (renamed since); corrected references are in `gaps` above. Also newly
identified (not previously documented) but judged the same deferred-blast-radius
class: `CompleteLayerUpload`'s "direct digest" fallback path never returns
`UploadNotFoundException` for a nonexistent/never-initiated upload session,
and `DescribeImages`'s `ImageDetail` is missing several real fields
(`artifactMediaType`, `imageScanFindingsSummary`, `imageScanStatus`,
`lastActivatedAt`, `lastArchivedAt`, `lastRecordedPullTime`,
`subjectManifestDigest`) — see `gaps` for full detail on both.

All 58 real ECR operations were diffed by name against
`aws-sdk-go-v2/service/ecr@v1.59.0`'s `api_op_*.go` file list this pass
(`diff` of the two sorted op-name lists is empty) — every real op is routed
and no gopherstack-invented op exists.

New tests locking every fix above: `TestLifecyclePolicyResult_LastEvaluatedAt_IsEpochNumber`,
`TestLifecyclePolicyPreview_EntryShape` (lifecycle_policy_test.go);
`TestScan_ImageScanCompletedAt_Populated` (scan_test.go);
`TestDescribeImageScanFindings_ImageScanCompletedAt_WireShape`
(image_scanning_test.go); `TestPutImage_ImageView_OmitsInventedFields`,
`TestBatchGetImage_ImageView_OmitsInventedFields` (handler_images_test.go);
`TestRegistryPolicy_OmitsInventedStatusField` (registry_policy_test.go).

### Genuine fixes made this pass, round 3 (2026-07-24)

The task for this round was explicit: close every remaining `gaps:` item for
real, including the invasive test-rewriting work rounds 1-2 had deferred
citing "test-suite-wide blast radius". All six previously-documented gaps are
closed, plus a seventh (`ListPullTimeUpdateExclusions` pagination) that was
in scope for the same reason (an open, non-impossible gap blocks an honest
`overall: A`).

1. **`EmptyUploadException` + `UploadNotFoundException` (same code path,
   fixed together).** `layers.go`'s `resolveCompletedLayerLocked` had three
   cases: a live session with data (real path), a live session with *no*
   data (silently accepted -- the `EmptyUploadException` gap), and a "direct
   digest" fallback that accepted *any* uploadId+digest pair with no live
   session at all (the `UploadNotFoundException` gap). Both silent-accept
   branches are now real AWS errors: an uploadId with no live session
   scoped to the given repository returns `UploadNotFoundException`; a live
   session with zero `UploadLayerPart` calls returns `EmptyUploadException`.
   This required rewriting the ~9 test call sites that used the old
   direct-digest shortcut as a seeding convenience (incl. the test literally
   named `empty_upload_no_error`, renamed `empty_upload_rejected` and
   flipped to assert the real behavior) to perform a genuine
   Initiate→UploadPart→Complete flow. Two new helpers do this consistently:
   `mustUploadLayer` (backend-level, store_test.go) and
   `mustUploadLayerPartHTTP`/`mustUploadLayerHTTP` (HTTP-level,
   handler_test.go). A subtlety this surfaced: several tests asserted a
   *specific* full-length (64 hex char) sha256-looking digest literal
   (e.g. `TestBatchCheckLayerAvailability_AllAvailable`) that had never
   actually hashed to real uploaded bytes -- only possible under the old
   unconditionally-accepting direct-digest path. Now that
   `verifiedUploadDigestLocked` genuinely verifies full-length digests
   against uploaded byte content, those tests use the digest the backend
   actually computes rather than a fixed literal (the test intent --
   layer-availability/download-URL logic -- is unaffected; only the specific
   digest *value* asserted on changed from a magic constant to a computed
   one).

2. **`LayerPartTooSmallException`.** Solved the "can't know which part is
   last until Complete" problem the prior rounds' notes correctly
   identified, via deferred validation: `layerUploadState` gained a
   `PartSizes []int64` field, appended to on every `UploadLayerPart` call;
   `CompleteLayerUpload` (via new `validatePartSizesLocked`) checks every
   part but the last against the real 5MiB minimum. A single-part upload is
   therefore never rejected (its one part is always "last"), which is why
   every pre-existing test that uploads a single tiny part continues to pass
   unchanged -- this is purely additive for the multi-part case, which no
   prior test exercised.

3. **`ImageAlreadyExistsException`.** The real trigger condition was
   determined by triangulating two independent sources: (a) the official AWS
   API doc text for `PutImage`'s `ImageAlreadyExistsException` ("The
   specified image has already been pushed, and there were no changes to the
   manifest or image tag after the last push") and separately
   `ImageTagAlreadyExistsException` ("The specified image is tagged with a
   tag that already exists. The repository is configured for tag
   immutability.") -- two *distinct* documented errors, the second
   explicitly scoped to immutable repos, the first not scoped to mutability
   at all; and (b) the moto ECR emulator's `put_image` reference
   implementation (github.com/getmoto/moto), whose logic independently
   confirms: a manifest that already exists in the repo, re-pushed under a
   tag it's already tagged with, raises this exception regardless of the
   repo's tag-mutability setting (moto's `_resolve_image_tag_mutability` is
   the separate, mutability-gated check, matching
   `ImageTagAlreadyExistsException`'s scope). Both sources converge on the
   same condition, so it was implemented with that convergent confidence
   rather than left deferred: `PutImage` now rejects a push whose tag
   already points at the exact digest being pushed, independent of
   `imageTagMutability`. This directly contradicted three existing tests
   that asserted the opposite as "idempotent" success --
   `TestImmutableRepo_SameManifestSameTag_Idempotent` (renamed
   `..._ImageAlreadyExists`), `TestPutImage_IMMUTABLE_SameTagSameDigest_Idempotent`
   (replaced with a MUTABLE-repo variant,
   `TestPutImage_MUTABLE_SameTagSameDigest_ImageAlreadyExists`, to
   additionally lock that the check is mutability-independent), and
   `TestTagMutability_Enforcement`'s `immutable_allows_same_digest` case
   (renamed `immutable_same_digest_rejected_as_no_op_push`) -- all three
   rewritten to assert the real behavior.

4. **`DescribeImages`'s 7 missing `ImageDetail` fields.**
   `artifactMediaType`/`subjectManifestDigest` are parsed straight from the
   pushed manifest's OCI 1.1 `artifactType`/`subject.digest` fields (a
   manifest that uses neither, e.g. a plain Docker v2 manifest, simply omits
   both -- no new state needed). `imageScanFindingsSummary`/`imageScanStatus`
   are annotated onto `Image` transiently (two new small info structs,
   `ImageScanFindingsSummaryInfo`/`ImageScanStatusInfo`, `json:"-"`, mirroring
   the pre-existing `Tags` annotation pattern) from a fresh
   `imageScanFindings` store lookup inside `DescribeImages`'s existing
   per-image `annotate` closure; an image with no scan yet simply has neither
   field set (omitted on the wire), never a zero-valued object.
   `lastActivatedAt`/`lastArchivedAt`/`lastRecordedPullTime` needed genuinely
   new backend state, added as three new plain `time.Time` fields directly on
   `Image` (not a separate map -- see `leaks` above for why that keeps
   cascade-cleanup and Snapshot/Restore free): `UpdateImageStorageClass`
   stamps `LastArchivedAt`/`LastActivatedAt` on ARCHIVE/re-activate
   transitions; `BatchGetImage` and `GetDownloadUrlForLayer` stamp
   `LastRecordedPullTime` (both now take the write lock instead of a read
   lock accordingly). `GetDownloadUrlForLayer` only receives a layer digest,
   not an image identifier, and the backend does not otherwise model a
   per-image layer list, so it stamps every image in the repository whose
   raw manifest text contains the requested layer digest as a substring
   (layer digests appear literally in a manifest's `layers[].digest`/
   `config.digest` fields) -- a pragmatic, real-behavior-approximating choice
   documented at the call site rather than a guess left unstated.

5. **`GetLifecyclePolicyPreview`'s `Filter`/`ImageIds`/`MaxResults`/`NextToken`.**
   Implemented at the handler layer (post-fetch filtering/pagination over the
   backend's already-computed full preview result), mirroring the existing
   `DescribeImages`/`ListImages` filter+pagination pattern rather than
   inventing a new one: `ImageIds` (when present) restricts to exactly those
   images and -- per the real API doc, "This option cannot be used when you
   specify images with imageIds" -- takes precedence over `Filter`/
   `MaxResults`/`NextToken`; otherwise `Filter.tagStatus`
   (TAGGED/UNTAGGED/ANY) filters and `MaxResults`/`NextToken` (default 100,
   matching the real API doc) paginate via the same
   `base64(imageDigest)`-cursor convention used elsewhere in this package.
   The backend's `GetLifecyclePolicyPreview(ctx, repositoryName)` signature
   is unchanged (no `Backend` interface change, no cross-repo caller impact).

6. **Genuinely new finding: `StartLifecyclePolicyPreview`'s response shape
   was also wrong**, independent of (but adjacent to) gap 5 above. Direct
   diff of `StartLifecyclePolicyPreviewOutput` /
   `awsAwsjson11_deserializeOpDocumentStartLifecyclePolicyPreviewOutput` in
   `aws-sdk-go-v2/service/ecr@v1.59.0` shows Start's real response contains
   *only* `lifecyclePolicyText`/`registryId`/`repositoryName`/`status` -- no
   `previewResults`, no `summary`, no `nextToken` at all (`StartLifecyclePolicyPreviewInput`
   correspondingly has no `Filter`/`ImageIds`/`MaxResults`/`NextToken`
   fields in the real SDK either, so the round-2 gap note describing a
   shared Start+Get gap was itself slightly wrong -- Start never had this
   gap; Get did). The prior implementation shared one view type between
   Start and Get, so Start leaked `previewResults`/`summary` into its
   response -- a real invented-field bug this round fixed via a new,
   narrower `lifecyclePolicyPreviewStartView`. `TestLifecyclePolicyPreview_EntryShape`
   (which asserted on Start's now-removed fields) was split into
   `TestStartLifecyclePolicyPreview_ResponseShape` (locks Start's narrow
   shape) and a rewritten `TestLifecyclePolicyPreview_EntryShape` (locks the
   full per-image shape via `GetLifecyclePolicyPreview`, as real clients must
   call it).

7. **`ListPullTimeUpdateExclusions`'s `MaxResults`/`NextToken`** (not one of
   the six gaps assigned this round, but left it open would have blocked an
   honest `overall: A` since it is not a proven impossibility). Implemented
   at the handler layer with the same `base64(principalArn)`-cursor
   pagination convention as everywhere else in this package, default
   `maxResults` 100 per the real API doc.

8. **Genuinely new finding: `UploadLayerPart`'s "unknown uploadId" error was
   the wrong exception/status entirely** -- `RepositoryNotFoundException`
   (404) instead of the real `UploadNotFoundException` (400) documented on
   `UploadLayerPart`'s own Errors list. Not part of the six assigned gaps
   (this bug wasn't previously documented at all); found while re-verifying
   the sibling `CompleteLayerUpload` code path for its own
   `UploadNotFoundException` gap (gap 1 above) and fixed for the same reason
   gap 7 was: an honest `overall: A` requires it. `TestECR_RestoreClearsInFlightLayerUploads`
   previously asserted the wrong 404 status (a **pre-existing test bug**,
   like several others this multi-round audit has uncovered) and was
   corrected to assert 400 `UploadNotFoundException`.

New tests locking every fix above: `TestLayerUploadFlow/empty_upload_rejected`,
`Test_CompleteLayerUpload_UnknownUploadID_ReturnsUploadNotFoundException`,
`TestCompleteLayerUpload_UnknownUploadID_DifferentRepo_ReturnsUploadNotFoundException`,
`TestCompleteLayerUpload_LayerPartTooSmall` (layers_test.go);
`TestImmutableRepo_SameManifestSameTag_ImageAlreadyExists`,
`TestPutImage_MUTABLE_SameTagSameDigest_ImageAlreadyExists`,
`TestDescribeImages_ArtifactMediaType_FromManifest`,
`TestDescribeImages_SubjectManifestDigest_FromManifest`,
`TestDescribeImages_ScanFields_PopulatedAfterScan`,
`TestDescribeImages_LastRecordedPullTime_ViaBatchGetImage`,
`TestDescribeImages_LastRecordedPullTime_ViaGetDownloadUrlForLayer`,
`TestDescribeImages_LastArchivedAt_LastActivatedAt_ViaUpdateImageStorageClass`
(handler_images_test.go); `TestTagMutability_Enforcement/immutable_same_digest_rejected_as_no_op_push`
(images_test.go); `TestStartLifecyclePolicyPreview_ResponseShape`,
`TestGetLifecyclePolicyPreview_FilterTagStatus`,
`TestGetLifecyclePolicyPreview_ImageIds`,
`TestGetLifecyclePolicyPreview_Pagination` (lifecycle_policy_test.go);
`TestPullTimeUpdateExclusion_Pagination` (account_settings_test.go).

### Genuine fixes made this pass, round 4 (2026-08-15, gopherstack-6flj)

This round's assignment was the repo-wide wrapper-key sweep (gopherstack-6flj),
not a fresh full-service audit — but re-deriving every L+D+G op's real shape
from the pinned SDK independently of the round-3 "wire: ok" claims (rather
than trusting them) found 6 more real bugs the prior rounds missed, all in
the registry/signing-configuration family:

1. **FLAGSHIP: `PutRegistryScanningConfiguration` reused `Get`'s response
   shape.** Both ops' handlers returned the same
   `getRegistryScanningConfigurationOutput` (wrapper key
   `"scanningConfiguration"` + top-level `registryId`) — correct for `Get`,
   but `PutRegistryScanningConfigurationOutput`'s own deserializer
   (`awsAwsjson11_deserializeOpDocumentPutRegistryScanningConfigurationOutput`)
   wraps under `"registryScanningConfiguration"` with **no** `registryId`
   field at all. A real SDK client parsing gopherstack's old `Put` response
   got a `nil RegistryScanningConfiguration` on every 200 — exactly this
   issue's target bug class, hiding behind a plausible-looking symmetric
   Get/Put pair. Fixed via a dedicated `putRegistryScanningConfigurationOutput`
   type. `TestPutRegistryScanningConfiguration_ScanTypeEnhanced` was an
   existing raw-body test that asserted the wrong key as correct; rewritten.

2. **`GetRegistryScanningConfiguration`, `PutImageScanningConfiguration`,
   `GetSigningConfiguration`, `DeleteSigningConfiguration` all declared a
   `registryId` wire field that was never populated** (always emitted `""`
   instead of the real account ID), while sibling ops in the same family
   (`DescribeRegistry`, `GetRegistryPolicy`, `PutRegistryPolicy`,
   `DescribeRepositoryCreationTemplates`) already set it correctly from
   `Backend.AccountID()`. `PutSigningConfiguration` was independently
   re-verified as correctly having **no** `registryId` — three signing-config
   siblings, two real shapes, confirmed against each op's own deserializer
   rather than assumed from the trio's surface symmetry.

3. **`BatchGetRepositoryScanningConfiguration` missing `appliedScanFilters`
   entirely.** The real `types.RepositoryScanningConfiguration` has this
   field (the registry scan rule's repository filters that produced a repo's
   effective `CONTINUOUS_SCAN` frequency); gopherstack's model never carried
   it. `repoEffectiveScanFrequency` now returns the matched rule's filters
   alongside the frequency.

4. **`DescribeRepositoryCreationTemplates` discarded `maxResults`/`nextToken`
   entirely** — the real Input/Output both carry them; this handler always
   returned every template in one page. Now paginates via the same
   `base64(prefix)`-cursor convention used by `DescribeRepositories`/
   `DescribePullThroughCacheRules` elsewhere in this package.

5. **`DescribeImageScanFindings`'s nested `imageScanFindings` object leaked
   5 extra fields.** It reused `ImageScanFindingsResult` (this package's
   internal domain struct, which also carries `ImageID`/`RepositoryName`/
   `RegistryID`/`Status`/`Description` for other callers) directly as the
   nested object, but the real nested `ImageScanFindings` type has only 5
   fields (`findingSeverityCounts`/`findings`/`enhancedFindings`/
   `imageScanCompletedAt`/`vulnerabilitySourceUpdatedAt`) — none of the
   other five. Harmless to a real client (unknown JSON keys are silently
   ignored), but a wire-shape imprecision; fixed via a purpose-built
   `imageScanFindingsView`.

6. **`ListImageReferrers` disclosed, not fixed.** The real
   `ListImageReferrersInput`/`Output` carry `Filter`/`MaxResults`/`NextToken`,
   but `PutImage` never records an OCI-referrer edge from a pushed artifact
   manifest's `subject` field back to the subject image — this op is
   structurally always empty. Adding the missing fields would be a
   schema-only change with no real behavior to ratify (0 items either way),
   so they were deliberately left off and the gap recorded in `gaps:` above
   instead of papered over.

Every fix above was hand-reverted individually, confirmed to fail against the
reverted code with the predicted symptom, then restored byte-identical
before moving to the next (per gopherstack-6flj's session protocol). New
ratifying tests in `wire_field_fixes_test.go`:
`TestPutRegistryScanningConfiguration_WrapperKey`,
`TestGetRegistryScanningConfiguration_RegistryIDPopulated`,
`TestPutImageScanningConfiguration_RegistryIDPopulated`,
`TestGetSigningConfiguration_RegistryIDPopulated`,
`TestDeleteSigningConfiguration_RegistryIDPopulated`,
`TestBatchGetRepositoryScanningConfiguration_AppliedScanFilters`,
`TestBatchGetRepositoryScanningConfiguration_NoRuleMatch_NoAppliedFilters`,
`TestDescribeRepositoryCreationTemplates_Pagination`,
`TestDescribeImageScanFindings_NestedObjectDoesNotLeakTopLevelFields`; plus
one existing-test fix, `TestPutRegistryScanningConfiguration_ScanTypeEnhanced`
in `image_scanning_test.go`.

Everything else in this file (all other L+D+G ops, the router, the protocol,
error mapping, credential sweep) was independently re-verified this round and
found already correct — see the session report for the full per-op list.

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped retype

`ecrSnapshotVersion` bumped 1 -> 2. `d83f4b5d3` retyped `ImageScanFinding.Attributes`
(nested inside the registered `imageScanFindings` table) from `map[string]string` to
`[]Attribute`, matching the real deserializer, without bumping the snapshot version. A
pre-fix (v1) snapshot's `"attributes"` object no longer unmarshals into the new array field
at all -- `RestoreAll` now errors outright rather than silently losing data, but the whole
backend then fails to restore, which the version guard exists to convert into a clean,
recoverable "discard and start empty" instead.

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `TestStorePersistence_V1ScanFindingsAttributesDiscarded` (persistence_test.go)
builds a v1-shaped `imageScanFindings` snapshot with a map-shaped `attributes` object and
asserts `Restore` succeeds (discarding cleanly) rather than erroring. Hand-reverted to
version 1: the same test then fails with `Restore` returning
`json: cannot unmarshal object into Go struct field ...Attributes of type []ecr.Attribute`,
confirming the symptom; restored and `md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).

## gopherstack-o7gx follow-up (2026-08-22): default error path emitted InternalServerError instead of the modeled fault

`handler.go`'s `classifyError` default branch returned code
`"InternalServerError"` for any unclassified 500. `ecr@v1.60.4`
`types/errors.go:912-935` models `ServerException` (`ErrorFault:
FaultServer`, doc comment: "These errors are usually caused by a
server-side issue") as the service's 5xx fault, wired into all 58 of 58
operation error switches in `deserializers.go` -- universal.
`"InternalServerError"` appears nowhere in `types/errors.go`, so a real
client's `errors.As(&types.ServerException{})` never matched.

Fixed to `"ServerException"`. The default branch is reachable only when a
backend error isn't one of the many enumerated sentinel/not-found groups,
`errUnknownAction`, or a JSON syntax/type error (mapped here to
`InvalidParameterException`); no currently-wired dispatch path leaves an
error unclassified this way, so there is no legitimately-constructed real
SDK client request that reaches this branch today.
`TestClassifyError_DefaultBranchEmitsServerException`
(`handler_internal_error_test.go`, new, white-box `package ecr`) drives
`classifyError` directly with a synthetic unmatched error and asserts it
returns `(http.StatusInternalServerError, "ServerException")`; confirmed it
fails pre-fix with the old `"InternalServerError"` code (hand-reverted,
byte-identical restore after).

**Per-item-failure sweep (this pass):** checked `BatchCheckLayerAvailability`,
`BatchDeleteImage`, `BatchGetImage`, `BatchGetRepositoryScanningConfiguration` --
the four ops whose SDK output models a per-item `Failures` field
(`types.LayerFailure`/`types.ImageFailure`/`types.RepositoryScanningConfigurationFailure`).
All four (`layers.go`, `images.go`, `image_scanning.go`) correctly report a per-item
failure (missing layer digest, image not found by digest/tag, repository not found)
while still returning results for every other requested item in the same call. No
bugs found in this class.

## gopherstack-6flj constrained-parameter sweep (2026-08-29)

Measured every List/Describe collection op against its own Input struct in
`ecr@v1.60.4`. Two real bug classes, both silent (200 OK, wrong membership):

1. **Undocumented-unlimited `maxResults` on five ops.** `DescribeImages`,
   `ListImages`, `DescribeRepositories`, `DescribePullThroughCacheRules`, and
   `DescribeRepositoryCreationTemplates` all gated their page-limit logic on
   `in.MaxResults > 0`, so an unset `maxResults` returned every matching item
   in one page instead of the docs' "if this parameter is not used, returns
   up to 100 results and a nextToken". `ListPullTimeUpdateExclusions`
   (`handler_account_settings.go`) already had this right
   (`paginatePullTimeUpdateExclusions`) and was the tell that the other five
   were an inconsistency, not a deliberate choice. All five now default
   `maxResults` to 100 before paginating.
2. **`ImageStatus` filter never plumbed on `DescribeImages`/`ListImages`.**
   Both ops' real `Filter` types (`DescribeImagesFilter`/`ListImagesFilter`)
   carry `ImageStatus` alongside `TagStatus`; gopherstack's wire structs
   (`describeImagesFilter`/`listImagesFilter`) only had `TagStatus` — the
   field was entirely absent from the request struct, so it could never be
   read regardless of what a client sent. Both ops also document "If not
   specified, only images with ACTIVE status are returned" — a real,
   observable default, since `UpdateImageStorageClass` can move an image to
   `ARCHIVED` (`images.go`'s `ImageStatus` field). Before this fix, an
   archived image kept appearing in every default-filter `DescribeImages`/
   `ListImages` call forever. Fixed by adding `ImageStatus` to both filter
   structs and a shared `passesImageStatusFilter` (mirrors the existing
   `passesTagFilter` convention): unset or explicit `"ACTIVE"` means
   ACTIVE-only, `"ANY"` disables the filter, anything else matches exactly.
   `ListImages`'s backend signature gained an `imageStatusFilter string`
   parameter (only caller: `handler_images.go`; `interfaces_test.go`'s
   `stubBackend` and `images_test.go`'s direct backend calls updated to
   match — confirmed via `go vet ./...` repo-wide, no other implementers).

`ListImageReferrers`'s `Filter`/`MaxResults`/`NextToken` omission (documented
in round 4 above) was re-confirmed still correct: the backend's
`ListImageReferrers` unconditionally returns `[]ImageReferrer{}` because
`PutImage` never records a referrer edge, so those fields would have zero
observable effect. Left as-is.

Also checked and confirmed already correct: `ListGraphqlApis`-style filters
don't apply here, but `DescribeImages`'s/`ListImages`'s `TagStatus`,
`DescribeRepositoryCreationTemplates`'s `Prefixes`, and
`DescribePullThroughCacheRules`'s `EcrRepositoryPrefixes` were all already
correctly plumbed through to their backends. `DescribeImageScanFindings`
already defaulted `maxResults` to 100 (`image_scanning.go`) — the one op in
this family that got it right independently.

One pre-existing test asserted the old (wrong) default behavior:
`TestDescribeImages_LastArchivedAt_LastActivatedAt_ViaUpdateImageStorageClass`
(`handler_images_test.go`) called `DescribeImages` with no filter right after
archiving an image and expected it back — now given an explicit
`imageStatus: ARCHIVED` filter, since an unfiltered call correctly excludes
it post-fix.

New tests in `list_filter_params_test.go`, driven through the real
`ecrsdk.Client` (`newTestECRClient`), each confirmed to fail against
unmodified code first: `TestDescribeImages_DefaultPageSize`,
`TestListImages_DefaultPageSize`, `TestDescribeRepositories_DefaultPageSize`,
`TestDescribePullThroughCacheRules_DefaultPageSize`,
`TestDescribeRepositoryCreationTemplates_DefaultPageSize`,
`TestDescribeImages_ImageStatusFilter_DefaultsToActiveOnly`,
`TestDescribeImages_ImageStatusFilter_Explicit`,
`TestListImages_ImageStatusFilter_DefaultsToActiveOnly`.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

**Four Class B bugs found and fixed**, all the same shape: a `nextToken`
cursor compared to each item by exact equality, defaulting to index 0 (the
whole collection) when nothing matched instead of the correct resume point
or an empty page.

- `filterAndPaginateImages` (`handler_images.go` — `DescribeImages`, 1 op):
  images are sorted ascending by `ImageDigest`; a digest deleted since the
  token was issued has no exact match. Fixed by searching for the first
  `ImageDigest >= cursorKey` and defaulting `start = len(imgs)` on a miss.
- `handleListImages`'s inline cursor logic (`handler_images.go` —
  `ListImages`, 1 op): identical shape, hand-rolled rather than routed
  through a shared helper (see "operations bypassing a helper" below). Its
  composite `digest:tag` cursor is still monotonic with the list's
  `(digest, tag)` sort order, so the same `>=`/default-to-`len` fix applies.
- `paginatePullTimeUpdateExclusions` (`handler_account_settings.go` —
  `ListPullTimeUpdateExclusions`, 1 op): ARNs are sorted ascending
  (documented in the existing comment); same `>=`/default-to-`len` fix.
- `paginateLifecyclePreviewEntries` (`handler_lifecycle_policy.go` —
  `GetLifecyclePolicyPreview`, 1 op): entries are sorted by `ImagePushedAt`
  (push time), **not** by the `ImageDigest` the cursor names — digest order
  has no relationship to list position here, so the `>=` threshold used
  above doesn't apply. Extracted into `advanceToDigestCursor`, which now
  returns no items (not a restart at page one) on either an unmatched digest
  or an undecodable token — the previous code silently used the
  full unfiltered list on a decode failure, a second miss path the tests
  below also cover.

4 operations affected total. None of the four helpers permits a silent
default to zero any more — three by construction (the miss default is now
`len(...)`, not `0`) and the fourth (`advanceToDigestCursor`) by being
factored into a function whose only two return paths are "found, from here"
or "not found, none" (no signature change needed to force this; the
extraction itself removes the mistake's foothold).

Every fix is proven by a table-driven unit test against the helper directly
(`pagination_arithmetic_internal_test.go`) with a stale-cursor (deleted
item) and, where applicable, a tampered/malformed-token case, both failing
pre-fix; plus two real `aws-sdk-go-v2/service/ecr` client round trips
(`pagination_sdk_roundtrip_test.go`: `DescribeImages` and `ListImages`,
each deleting the cursor's target between calls).

All seven checks pass post-fix for all four helpers. No Class A or C shape
found in this package.

Gates: `go build ./services/ecr/...`, `go vet ./services/ecr/...` and
`go vet ./...` (repo-wide, clean — no signature changed),
`go test -race -count=1 ./services/ecr/...`, `golangci-lint run
./services/ecr/...` (0 issues).

**2026-08-30 (negative-continuation-token sweep)**: `image_scanning.go`'s
`DescribeImageScanFindings` parsed `nextToken` with a bare `strconv.Atoi` and no bounds check;
its `startIdx >= total` guard does not catch a negative `startIdx`, so
`cp.Findings[startIdx:endIdx]` / `cp.EnhancedFindings[startIdx:endIdx]` panicked given a
negative `nextToken`. Fixed at the decode site: the parsed value is now validated `>= 0`
before being assigned.

Proof: `TestDescribeImageScanFindings_NegativeOffsetToken` (`image_scanning_test.go`) confirmed
panicking pre-fix, passes now. Gates: `go build ./services/ecr/...`, `go vet
./services/ecr/...`, `go test -race -count=1 ./services/ecr/...`, `golangci-lint run
./services/ecr/...` (0 issues). Work left uncommitted per this pass's instructions.

**2026-08-30 (gopherstack-wks5, field-identity request-parameter sweep)**: exhaustive
type-aware scan of every request-decode struct field across `ecr` and `efs`, using
`go/types` to match field reads by object identity (not name) rather than a literal
`Bind()`/`Unmarshal()` grep. Initial pass under-covered `ecr`: this service's dispatch
goes through `pkgs/service.WrapOp[In, Out]` (the AWS JSON-protocol generic dispatcher),
whose reflection-based decode is invisible to a literal-call scan; the scanner was
extended to also resolve the 2nd parameter type of every `handleXxx` method registered
via `service.WrapOp(h.handleXxx)`, after which it covered 127 decode-target types / 174
fields (up from 2 types / 5 fields caught by the literal-call pass alone) -- confirming
the task's own warning that a scanner's blind spot is where bugs hide, here in the
scanner's *coverage* rather than in anonymous-struct decode targets (ecr/efs have none;
every decode target is a named type).

25 fields flagged zero-uses; 23 were hand-verified false positives from Go type
conversion (`req := SomeRequest(in)`, structurally identical named types -- the
converted-to type's own fields carry real reads the scanner's identity match correctly
does not attribute back to the pre-conversion type; confirmed by reading
`CreateMountTarget`/`UpdateFileSystem` in `services/efs`). 2 were genuine and are
recorded above: `SetRepositoryPolicy`'s `Force` (this file) and `PutFileSystemPolicy`'s
`BypassPolicyLockoutSafetyCheck` (`services/efs/PARITY.md`) -- both disclosed, not
fixed (cross into IAM policy-lockout simulation this repo has no package for). The
`registryId` structural finding is recorded in `gaps` above.

Also checked and confirmed clean by hand, not by the scanner (which only sees
zero-use fields, not misuse): every `.All()` map-walk in this package that feeds a
client-visible list is `sort.Slice`d before return (`ListPullTimeUpdateExclusions`,
`DescribePullThroughCacheRules`, `DescribeRepositoryCreationTemplates`,
`DescribeRepositories`); `RunLifecycleExpiry`'s unsorted `.All()` walk is an internal
sweep with no client-visible order to break. `DescribeImages`/`ListImages` both
correctly reject an unknown `repositoryName` with `ErrRepositoryNotFound` before
touching per-repo indexes (contrast with the `efs` existence-check bug fixed this same
pass, `services/efs/PARITY.md`).

Gates: `go build ./services/ecr/...`, `go vet ./services/ecr/...`, `go test -race
-count=1 ./services/ecr/...` (no ecr code changed this pass, disclosure-only; suite
green as a baseline check), `golangci-lint run ./services/ecr/...`. Work left
uncommitted per this pass's instructions.

## 2026-08-30 (gopherstack-uox6, value-semantics pass)

Different question than every prior ecr pass: not "is a field read/shape
correct" but "does a correctly-read field's matching logic do what AWS
documents." Derived the matcher/filter set fresh rather than trusting a
handed-in count (see below) — repoMatchesFilters/wildcardMatch
(repositories.go), lifecycle.go's whole rule-evaluation pipeline
(matchesTagStatus/matchesTaggedSelection/tagMatchesAnyPrefix/
tagMatchesAnyPattern/applyRule and its count-type cases), tagMatchesAnyExclusionFilter
(images.go), passesTagFilter/passesImageStatusFilter/filterAndPaginateImages
(handler_images.go, images.go), filterLifecyclePreviewEntriesByImageIDs/
lifecyclePreviewEntryMatchesAnyImageID/filterLifecyclePreviewEntriesByTagStatus
(handler_lifecycle_policy.go). Excluded from the count: RouteMatcher/MatchPriority
(HTTP path routing, not filtering).

**Two bugs found, both real and both fixed** — see the `PutReplicationConfiguration`
and `lifecycle-evaluation` entries above for the full description. Summary:

1. `repoMatchesFilters` switched on a fabricated `"PREFIX"` FilterType value
   that exists in neither of the two real AWS types sharing this internal
   struct (replication's `RepositoryFilterType` is `PREFIX_MATCH`-only;
   scanning's `ScanningRepositoryFilterType` is `WILDCARD`-only) — an
   under-matching bug: a real client's `PREFIX_MATCH` replication filter
   matched zero repositories.
2. `evaluateLifecyclePolicy`'s action-type gate accepted only `"expire"`,
   and `applyRule`'s countType switch had no case for `sinceImagePulled`/
   `sinceImageTransitioned` — both under-matching (silently matched zero
   images) despite the backing state already existing.

**Self-caught near-miss, same failure mode this class has burned agents on
before (a doc comment for the WRONG type).** The first implementation of
bug 2 used `action.type=="archive"` because AWS's prose page
(LifecyclePolicies.html: "images can be archived or deleted") reads that
way. Checking the actual typed SDK model (`types.LifecyclePolicyRuleAction`)
showed the real `ImageActionType` enum is `EXPIRE`/`TRANSITION` — there is
no `ARCHIVE` action type; archiving is `type:"transition"` plus a sibling
`targetStorageClass:"archive"` field. Caught by fetching
`lifecycle_policy_examples.html` for a concrete worked example before
finalizing, matching a documented worked example instead of the prose
paraphrase. Corrected before merging into this pass rather than shipped and
found later.

**Gap closed, not left open**: while fixing sinceImageTransitioned, the
policy template also showed `selection.storageClass` ("standard"|"archive")
as a general sibling filter of `tagStatus`, not something exclusive to that
one countType. Implemented as a general selection filter
(`matchesStorageClass`) applied uniformly, with its own test
(`TestLifecycle_Selection_StorageClass_GeneralFilter`) proving an unrelated
`imageCountMoreThan` rule scoped to `storageClass:"standard"` leaves an
already-archived image untouched.

**Matcher count**: this repo's own count going in was "ecr ~14"; the derived
set above is comparable (~13 real matchers/filters across repositories.go/
lifecycle.go/images.go/handler_images.go/handler_lifecycle_policy.go), with
no HTTP-routing contamination found this time (RouteMatcher/MatchPriority
were excluded up front, not miscounted in).

**Confirmed correct, not re-derived**: the general AND/OR combining rule
inside `matchesTaggedSelection` (any tag matching any prefix OR any pattern,
consistent with every worked example in lifecycle_policy_examples.html's
"multiple tags in a single rule" section); `wildcardMatch`'s `*`-only glob
(no `?`, matching ECR's own documented "There is a maximum limit of four
wildcards" constraint, which presupposes `*` is the only wildcard token);
the registryId accept-and-ignore pattern (already disclosed above as
deliberate, re-confirmed not a target of this class).

**Web pages fetched: 2**, both from `docs.aws.amazon.com/AmazonECR/latest/userguide/`
(`LifecyclePolicies.html`, `lifecycle_policy_examples.html`). **Both carried**
the injected footer ("Skills for AI coding assistants (optional)... search
the Agent Toolkit for AWS catalog with `aws agent-toolkit search-skills`").
Treated as untrusted page content, not followed.

**Existing test corrected, not weakened**: `replication_test.go`'s
`repositoryFilters gate which repos replicate` subtest asserted the bug's
own fabricated `"PREFIX"` value as if it were the real filter type, and
only ever exercised the non-matching case — so it passed both before and
after the fix for unrelated reasons and never could have caught this bug.
Corrected to the real `"PREFIX_MATCH"` value (same single assertion,
unchanged); a new test (`TestReplication_RepositoryFilters_PrefixMatch_HonoursRealEnumValue`)
adds the missing positive-match case. Two `image_scanning_test.go` subtests
had the same fabricated-value problem for the *scanning* filter type (which
has no `"PREFIX"`/`"PREFIX_MATCH"` value at all — WILDCARD-only); renamed
and their fixture changed to `WILDCARD` with a literal (wildcard-free)
pattern, same 2 assertions, same pass/fail outcome, now testing the real
mechanism instead of accidentally.

Gates re-run after this pass: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./services/ecr/...`, `golangci-lint run
./services/ecr/...` — all clean.

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `Url`/`URL` acronym casing
gives it 1 op/handler pairs needing the ambiguous fold, 1 of them
genuine collisions between an exported backend method and the real
unexported handler: `GetDownloadUrlForLayer`.

Verified directly rather than assumed: ran the unpatched tool from
`ef0eef041~1` five times and diffed against the fixed tool at HEAD, for
both `cmd/reqfieldscan` and `cmd/reqfielddiff`. Both were byte-identical
across all 5 old runs and HEAD (58 SDK operations compared) -- the
determinism defect never flipped a finding here, because the resolution
that actually mattered (this package's dispatch-table union) already
carried the correct field set regardless of which fold candidate won.

Verdict: confirmed zero damage, not merely predicted.
