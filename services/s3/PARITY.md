---
service: s3
sdk_module: aws-sdk-go-v2/service/s3@v1.111.0   # version audited against (go.mod pin)
last_audit_commit:                                # unknown: gopherstack-6flj wrapper-key sweep pass ran without git access at write time, never backfilled -- gopherstack-33in; see 2026-08-15 section below
last_audit_date: 2026-08-15
overall: A   # gopherstack-3dqa: found+fixed 4 real bugs incl. a race-detector-confirmed data race and a real (not disguised) over-replication bug. gopherstack-zi7k (2026-08-14): implemented the 5-op Object Annotations family that gopherstack-3dqa found entirely missing. gopherstack-3dqa follow-up (2026-08-14b): mechanical struct-field diff (the method that closed the dynamodb sibling pass) found 2 real absent-but-tracked wire bugs; a benchmark-verified ListObjectsV2 allocation fix closed the one axis (optimization) the prior four rounds left as "inspected, not profiled". gopherstack-6flj (2026-08-15): full List/Describe/Get wrapper-key sweep (45 ops), 2 more real bugs fixed (ListObjects/V2 Owner, GetBucketVersioning MFADelete), 1 severe wrong-response-shape finding flagged not fixed (GetBucketMetadataConfiguration/GetBucketMetadataTableConfiguration) -- see families/ops/gaps below.
protocol: REST-XML
families:
  multipart:    {status: ok, note: part-order InvalidPartOrder, non-last EntityTooSmall, ETag=MD5(concat part-MD5s)-N, SSE sealing}
  conditionals: {status: ok, note: If-Match/None-Match/(Un)Modified-Since 412/304 precedence; If-Range; Range 206/416 InvalidRange w/ ActualObjectSize}
  versioning:   {status: ok, note: delete markers, null-version, suspended vs never-configured, object-lock/legal-hold}
  pagination:   {status: ok, note: v1 NextMarker only w/ delimiter; v2 KeyCount, ContinuationToken/StartAfter, encoding-type on keys not tokens; List*Configurations (analytics/inventory/metrics/intelligent-tiering) do NOT paginate (IsTruncated always false) — documented gap, see below}
  errors:       {status: ok, note: full errorTable, no missing-lookup->500; HEAD bodiless}
  copy:         {status: ok, note: "FIXED 2026-07-24 (phase 2): CopyObjectResult now carries destination checksums (ChecksumCRC32/CRC32C/SHA1/SHA256/CRC64NVME, either recomputed via x-amz-checksum-algorithm or carried forward from the source); LastModified now reads back the real stored value via HeadObject instead of a second independent time.Now(); destination SSE (SSE-S3/SSE-KMS) request headers are now honored + echoed in the response (previously silently dropped); copy-source SSE-C headers are now read and validated (previously a source object encrypted with SSE-C would silently copy raw CIPHERTEXT as if it were plaintext — see errors below)"}
  bucket_delete: {status: ok, note: "FIXED 2026-07-24: DeleteBucket previously accepted ANY bucket (objects, versions, delete markers, and even incomplete multipart uploads) and silently queued an async janitor drain — real S3 rejects with 409 BucketNotEmpty until the caller empties it. Now checked synchronously under b.mu before marking DeletePending; janitor drain loop kept as a no-op-in-practice safety net (bucket is already empty by the time it's marked pending)."}
  bucket_config_lists: {status: ok, note: "FIXED 2026-07-24 (phase 2, SEVERE): writeConfigListXML (shared by ListBucketAnalyticsConfigurations/ListBucketIntelligentTieringConfigurations/ListBucketInventoryConfigurations/ListBucketMetricsConfigurations) was wrapping each already-XML-rooted stored config (Put*Configuration's request body IS the full `<XConfiguration>...</XConfiguration>` document per the real SDK's serializer) in ANOTHER copy of the same root element, producing doubly-nested XML no real SDK client could parse Id/Filter/etc back out of. Fixed by emitting each stored config verbatim (unwrapped) — matches the real deserializer's *ListUnwrapped decode logic. Regression test decodes the real element shape and fails if double-nesting regresses."}
ops:
  GetObject/HeadObject: {wire: ok, errors: ok, state: ok, persist: ok, note: FIXED response-* override query params (content-type/disposition/expires/cache-control)}
  PutBucketAcl:         {wire: ok, errors: ok, state: ok, persist: ok, note: FIXED reject object-only canned ACLs; read AccessControlPolicy body}
  PutBucketReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: FIXED require versioning=Enabled}
  GetObjectAttributes:  {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED ObjectSize 0-byte, Last-Modified, and ObjectParts (types.GetObjectAttributesParts multipart breakdown)"}
  DeleteBucket:         {wire: ok, errors: ok, state: ok, persist: ok, note: FIXED 409 BucketNotEmpty for objects/versions/delete-markers AND incomplete multipart uploads (real AWS gotcha: MPUs block deletion despite not appearing in ListObjects)}
  PutBucketPolicy:      {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-07-24 400 MalformedPolicy for non-JSON body; FIXED 2026-07-24 (phase 2) full IAM-policy-grammar shape validation (Version/Statement/Effect/Principal/Action/Resource presence+shape) per https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_grammar.html — bucket policies are resource-based so Principal/NotPrincipal is required (real S3 error confirmed: 'MalformedPolicy: Missing required field Principal cannot be empty!')"}
  PostObject:           {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-07-24 (phase 2): POST form fields x-amz-storage-class, x-amz-server-side-encryption(-aws-kms-key-id), and x-amz-checksum-algorithm are now applied to the uploaded object (previously silently ignored — a presigned-POST upload requesting SSE-KMS was stored unencrypted, defeating the caller's intent)"}
  SelectObjectContent:  {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-07-24 (phase 2): SSECustomerAlgorithm/-Key/-KeyMD5 are HTTP-header-bound per the real SDK's serializer (not XML body fields, despite living in SelectObjectContentInput) — now extracted and validated so selecting against an SSE-C source requires (and correctly uses) the same headers GetObject requires; previously ignored entirely, so a query against an SSE-C object would either silently query raw ciphertext or fail opaquely"}
  PutObjectLockConfiguration: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "FIXED 2026-08-07 (gopherstack-pzth): real S3 returns 409 InvalidBucketState for PutObjectLockConfiguration on a bucket not created with x-amz-bucket-object-lock-enabled: true (confirmed against types.go's documented error table: Code InvalidBucketState, 409 Conflict), and CreateBucket did not even read that header so there was no stored flag to check against — the emulator was strictly more permissive than real AWS. CreateBucket now reads input.ObjectLockEnabledForBucket (already the real aws-sdk-go-v2 CreateBucketInput field, no new struct needed) onto StoredBucket.ObjectLockEnabled; PutObjectLockConfiguration now rejects with the new ErrObjectLockNotEnabled sentinel when unset. GetObjectLockConfiguration's existing ObjectLockConfigurationNotFoundError path needed no change: an object-lock-disabled bucket can now never have a stored config to find, so it already falls through to the same NotFound response correctly."}
  CreateBucket:         {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-12 (gopherstack-difi): CreateBucketConfiguration.Tags (types.go:890+, s3@v1.106.5 -- real payload member, TagSet shape) was never parsed from the request XML body, so a client-specified initial bucket tag set was silently discarded. Now parsed alongside LocationConstraint and threaded through to the same StoredBucket.Tags field PutBucketTagging/GetBucketTagging already read/write -- no parallel store."}
  ListBuckets:          {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-03: each Bucket element now carries BucketRegion (sourced from the same per-bucket StoredBucket.Region that enforceBucketRegion already gates cross-region access on), for dashboard visibility into a bucket's real region — ListBuckets is account-global so the bucket list always includes buckets from every region regardless of the caller's signed region, and previously nothing in the response said so. Unlike GetBucketLocation's LocationConstraint (blanked to \"\" for us-east-1), BucketRegion reports the literal region string including \"us-east-1\" — confirmed against the real ListBuckets API docs' paginated response examples. Deliberate gap: real S3 only echoes BucketRegion when the request carries bucket-region/prefix/continuation-token/max-buckets (the unpaginated doc example omits it); this backend doesn't implement ListBuckets pagination/filtering at all, so BucketRegion is simply always populated rather than gated on a request-shape nuance with no pagination behavior behind it."}
  PutBucketAbac/GetBucketAbac: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-08-13 (gopherstack-ob1g): CORRECTS the 2026-07-24 (phase 2) note below that had spot-checked these as fully implemented -- they were not. TWO stacked wire bugs discovered while hardening GetBucketAbac's discarded xml.Unmarshal error (PutBucketAbac stores the raw request body verbatim with no parsing, so GetBucketAbac's re-parse of that stored body on every Get is really parsing the original client request). (1) The real root is AbacStatus, not AbacConfiguration (serializers.go: awsRestxml_serializeOpPutBucketAbac's payloadRoot.Local) -- a real client's PUT body never matched, so xml.Unmarshal errored on the whole thing (err discarded) and every GetBucketAbac after a real PutBucketAbac silently returned an empty status. (2) Once the root was fixed, GetBucketAbacOutput.AbacStatus turned out to be httpPayload-bound: the real deserializer ((*awsRestxml_deserializeOpGetBucketAbac).HandleDeserialize) parses the response's ROOT element directly as the AbacStatus document via awsRestxml_deserializeDocumentAbacStatus, not a nested AbacStatus child of some other envelope root -- a same-named awsRestxml_deserializeOpDocumentGetBucketAbacOutput function exists in the SDK source but is dead code the real deserializer never calls, and trusting it as authoritative (as this pass initially did) reproduces the exact bug class this ticket exists to fix. Caught immediately by driving both PUT and GET through the real aws-sdk-go-v2 client (TestGetBucketAbac_RealClient), which is what surfaced the dead-code trap: the client returned no error and a non-nil AbacStatus, just with an empty Status, because the SDK refuses to populate a field from a response shape it doesn't recognize regardless of what the raw XML contains. Response is now the bare AbacStatus document; request parsing fixed to the same root. Both confirmed to fail against the pre-fix code by reverting by hand."}
  CreateMultipartUpload/CompleteMultipartUpload/ListMultipartUploads: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-13 (gopherstack-3dqa): CreateMultipartUploadInput.StorageClass (real header-bound field, confirmed serializers.go:1122 X-Amz-Storage-Class) was declared and never read anywhere -- neither the HTTP handler (multipart_ops.go's createMultipartUpload built the backend input without it at all) nor the backend (StoredMultipartUpload had no field for it). A multipart upload requesting e.g. GLACIER or STANDARD_IA silently landed as STANDARD on CompleteMultipartUpload, and ListMultipartUploads never populated StorageClass/Owner/Initiator at all (confirmed real fields via deserializers.go's awsRestxml_deserializeDocumentMultipartUpload). Fixed by threading StorageClass through CreateMultipartUpload -> StoredMultipartUpload -> CompleteMultipartUpload's commitMultipartObject -> the new object version, and populating StorageClass/Owner/Initiator on ListMultipartUploads' response entries. TestMultipartUpload_StorageClassAppliedToObject drives the real SDK client end-to-end (Create with StorageClass -> List asserts it back -> Complete -> HeadObject asserts the completed object's StorageClass), confirmed to fail against the pre-fix code (both the missing handler wiring and the missing backend field, found in that order) by hand-reverting."}
  PutBucketReplication (object-put replication matching): {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-13 (gopherstack-3dqa): the async replication matcher (replication.go's triggerReplication/triggerDeleteMarkerReplication) only checked the deprecated top-level Rule>Prefix element (types.ReplicationRule.Prefix is documented 'Deprecated' in the real SDK), never the modern Rule>Filter>Prefix (types.ReplicationRuleFilter, confirmed real element names via types/types.go:4367+). model.go's ReplicationRule Go struct had no Filter field at all. Net effect: a replication rule written with only <Filter><Prefix>images/</Prefix></Filter> (the form AWS's own docs recommend) parsed a legacy Prefix of \"\", which the matcher treats as 'no filter, match everything' -- so a rule scoped to images/ silently replicated every object in the bucket, an over-replication data-exposure bug, not a mere miss. Fixed by adding ReplicationRuleFilter{Prefix, Tag} to model.go and a shared matchesReplicationRule helper that prefers Filter.Prefix over the legacy field. Filter>Tag and Filter>And (tag-based/composite filters) are deliberately NOT evaluated -- such a rule is now treated as non-matching (skip, under-replicate) rather than over-matching, since silently replicating what a real filter would exclude is the more harmful failure mode; see gaps. TestS3BucketReplication_FilterPrefix drives two PutObjects through a Filter>Prefix-scoped rule, uses the exported InMemoryBackend.DrainReplicationGoroutines() for a deterministic happens-before boundary (no require.Eventually/sleep races), and confirmed to fail against the pre-fix matcher by hand-reverting (the non-matching key was incorrectly replicated too)."}
  RenameObject: {wire: ok, errors: ok, state: fixed, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-3dqa): real DATA RACE, confirmed with `go test -race`, not a theoretical gap. When the rename target key already had an object ('existing', a different *StoredObject than the source), RenameObject (objects.go) wrote existing.LatestVersionID and existing.Versions[...] while holding only bucket.mu (exclusive) and srcObj.mu -- never existing.mu. A concurrent GetObject/HeadObject on the target key takes bucket.mu only briefly to fetch the object pointer, releases it, then reads existing.Versions/LatestVersionID under existing.mu.RLock alone -- the writer and that reader shared no common lock. -race reproduced an unsynchronized map write (mapassign_faststr) racing a concurrent map read (findLatestVersion) within ~0.3s of concurrent RenameObject+GetObject traffic. Fixed by taking existing.mu.Lock() around the mutation. RenameObject had zero prior test coverage of any kind (no existing _test.go referenced it before this pass). TestRenameObject_ConcurrentGetOnExistingTarget_NoRace confirmed to fail (i.e. reproduce the -race report) against the pre-fix code by hand-reverting."}
  WriteGetObjectResponse: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "FIXED 2026-08-13 (gopherstack-3dqa): WriteGetObjectResponseInput.StatusCode is header-bound to X-Amz-Fwd-Status (confirmed serializers.go:11069, awsRestxml_serializeOpHttpBindingsWriteGetObjectResponseInput). handleWriteGetObjectResponse (object_lambda.go) hardcoded statusCode: http.StatusOK on every call regardless of what the Lambda sent, discarding the header entirely -- an Object Lambda that calls WriteGetObjectResponse with a non-200 status (e.g. an access-control Lambda returning 403, confirmed a real, documented use of this field) had that status silently downgraded to 200 for the original GetObject caller. The downstream forwarding mechanism (resp.statusCode, handled at line ~160) already existed and worked correctly -- only the header read feeding it was missing. Fixed by parsing X-Amz-Fwd-Status. TestS3ObjectLambda_WriteGetObjectResponse_ForwardsStatus (a Lambda stub returning 403) confirmed to fail (asserted 200 instead of 403) against the pre-fix code by hand-reverting."}
  PutObjectAnnotation/GetObjectAnnotation/DeleteObjectAnnotation/ListObjectAnnotations/UpdateBucketMetadataAnnotationTableConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "IMPLEMENTED 2026-08-14 (gopherstack-zi7k): the whole family was previously entirely absent (gopherstack-3dqa). Routes verified from s3@v1.106.5 serializers.go's httpbinding.SplitURI calls, not by pattern: PUT/GET/DELETE /{Key+}?annotation (query keys annotationName/versionId), bucket-level PUT /?metadataAnnotationTable. GetObjectAnnotation and ListObjectAnnotations share the identical GET route template -- routed on whether the annotationName query param is present, since only GetObjectAnnotation's own HttpBindings function binds it. Annotations are real per-object-version state (StoredObjectVersion.Annotations, additive/omitempty field, no snapshot version bump), proven with a lifecycle test through the real aws-sdk-go-v2 client (put->get->list->delete->list, asserting non-empty/exact-count results at each step) plus a dedicated Snapshot/Restore round-trip test. See gaps for what's deliberately not enforced (payload size cap, ObjectIfMatch) and the one validation rule sourced from a doc comment rather than wire code (reserved name prefix)."}
  ListObjects/ListObjectsV2: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-15 (gopherstack-6flj wrapper-key sweep): Object.Owner (s3@v1.106.5 deserializers.go's awsRestxml_deserializeDocumentObject, case \"Owner\") is a real per-item member the shared ObjectXML struct had NO field for at all -- every real client's Contents[].Owner was nil regardless of backend state, for both ops. ListObjects (V1) has no FetchOwner request member (confirmed absent from ListObjectsInput) so Owner is unconditionally present on every item; ListObjectsV2 only includes it when FetchOwner=true (a near-duplicate-shape pair that genuinely differs, not a copy-paste mismatch). Fixed by adding ObjectXML.Owner and an includeOwner bool threaded through the shared mapObjectsToXML (true for V1, q.Get(\"fetch-owner\")==\"true\" for V2)."}
  GetBucketVersioning/PutBucketVersioning: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-15 (gopherstack-6flj wrapper-key sweep): GetBucketVersioningOutput.MFADelete (deserializers.go's awsRestxml_deserializeOpDocumentGetBucketVersioningOutput, case \"MfaDelete\", sibling to \"Status\") was read from no request, stored nowhere, and echoed by no response -- a real client's PutBucketVersioning({MFADelete: Enabled}) had the value silently dropped, and GetBucketVersioning's MFADelete was always empty regardless. Real request-side type is types.MFADelete; real response-side type is the DIFFERENT types.MFADeleteStatus (same \"Enabled\"/\"Disabled\" strings, two distinct SDK enums) -- stored as a plain string in StoredBucket to avoid coupling to either. Only emitted once ever configured (omitempty), matching the real doc: \"This element is only returned if the bucket has been configured with MFA delete.\""}
  GetBucketMetadataConfiguration/GetBucketMetadataTableConfiguration: {wire: gap, errors: n/a, state: n/a, persist: ok, note: "FOUND, NOT FIXED 2026-08-15 (gopherstack-6flj wrapper-key sweep) -- the most severe finding this pass, deliberately left unfixed. Unlike every other Get*Configuration op in this file (CORS/lifecycle/notification/encryption/logging/replication/analytics/inventory/metrics/intelligent-tiering), where the real GET deserializer parses the response ROOT element directly as the same struct the PUT request root already is (confirmed per-op against deserializers.go), these two do NOT: awsRestxml_deserializeOpGetBucketMetadataConfiguration.HandleDeserialize (deserializers.go) parses the response root directly as types.GetBucketMetadataConfigurationResult, which requires a CHILD element named exactly \"MetadataConfigurationResult\" (types.MetadataConfigurationResult{DestinationResult (required, TableBucketArn/TableBucketType/TableNamespace), AnnotationTableConfigurationResult, InventoryTableConfigurationResult, JournalTableConfigurationResult}) -- a server-computed RESULT shape, structurally different from the client's CreateBucketMetadataConfiguration request body (types.MetadataConfiguration{JournalTableConfiguration, AnnotationTableConfiguration, InventoryTableConfiguration}, no ARNs/status at all). gopherstack's getBucketMetadataConfiguration/getBucketMetadataTableConfiguration (bucket_ops_metadata_table.go) echo the raw stored CREATE request body verbatim -- which has no \"MetadataConfigurationResult\"/\"MetadataTableConfigurationResult\" child element anywhere, so a real typed client's GetBucketMetadataConfigurationOutput.GetBucketMetadataConfigurationResult.MetadataConfigurationResult (and the Table variant's equivalent) decodes to nil regardless of what was created. The same OpDocument...Output wrapper function with a matching case IS present in generated code but is dead -- HandleDeserialize never calls it, the same trap gopherstack-ob1g already found and fixed once on GetBucketAbac -- so this is not a simple 'wrong root name' rename. NOT FIXED: producing a real DestinationResult requires an S3 Tables table-bucket ARN/namespace/provisioning-status concept this backend has no model for at all (no CreateBucketMetadataConfiguration path allocates a table bucket or generates an ARN); fabricating plausible-looking ARNs/status would be invented data, not a shape fix. Flagged per this campaign's own precedent for genuinely-unmodeled response shapes (matches securityhub's GetRecommendedPolicyV2 finding) rather than attempted."}
gaps:
  - "GetBucketMetadataConfiguration/GetBucketMetadataTableConfiguration return the wrong response shape entirely for any real typed client -- see the ops row above (gopherstack-6flj, 2026-08-15). Fixing this for real requires modeling S3 Tables table-bucket provisioning (ARN/namespace/status), which this backend has no concept of anywhere; CreateBucketMetadataConfiguration/CreateBucketMetadataTableConfiguration would also need the same new state. Left flagged rather than fabricated."
  - "Object Annotations (gopherstack-zi7k, 2026-08-14): implemented -- PutObjectAnnotation/GetObjectAnnotation/DeleteObjectAnnotation/ListObjectAnnotations store real per-object-version state (StoredObjectVersion.Annotations, additive/omitempty, survives Snapshot/Restore) and UpdateBucketMetadataAnnotationTableConfiguration persists its config XML the same way its metadataInventoryTable/metadataJournalTable siblings do. Routes verified from the pinned serializer, not by pattern: PUT/GET/DELETE /{Key+}?annotation, and GET is shared byte-for-byte between GetObjectAnnotation and ListObjectAnnotations (both httpbinding.SplitURI to the same path+query) -- disambiguated on the presence of the annotationName query param, which only GetObjectAnnotation's HttpBindings function binds. Bucket-level route key metadataAnnotationTable was independently re-verified (matches the bd issue's note). Deliberately NOT enforced: the documented 1-byte-to-1-MiB payload size window (no error code for it appears in any of these ops' own deserializeOpError switches, so inventing one would violate the same rule that caught the invented metadataTableConfiguration/exception bugs this same sweep found elsewhere) and DeleteObjectAnnotation/PutObjectAnnotation's ObjectIfMatch conditional header (also absent from every relevant switch in this pinned SDK version). DeleteObjectAnnotation deliberately does NOT return NoSuchAnnotation for a missing name -- its error switch declares only NoSuchBucket/NoSuchKey, matching real S3's idempotent-delete semantics. The annotation-name reserved-prefix rule ('cannot start with aws or s3') is enforced from DeleteObjectAnnotation's doc comment in the pinned source, not a serializer/deserializer fact -- flagged here as the one validation rule in this pass that rests on prose rather than wire code. UpdateBucketMetadataAnnotationTableConfiguration's own error switch declares no typed error cases at all (every failure decodes as smithy.GenericAPIError) -- confirmed by reading it directly, not assumed. No dedicated Get op exists for the bucket-level annotation-table config in the pinned SDK, so (like its inventory/journal siblings) persistence there is provable by store/restore but not independently readable over the wire."
  - "CreateSession (S3 Express One Zone) is a disguised stub beyond its own doc comment's disclosure: buckets.go's CreateSession returns a hardcoded fake SessionToken/AccessKeyId/SecretAccessKey for ANY bucket (it doesn't check IsDirectoryBucket, doesn't validate the bucket is actually a directory bucket the way real S3 requires), completely ignores the request's SessionMode (ReadOnly/ReadWrite), and the returned session token has no effect anywhere else in this package -- it isn't wired into sigv4 validation or any subsequent request's authorization, so a caller that authenticates via the returned session credentials would not actually get S3-Express-scoped access semantics. Consistent with the broader disclosed gap that this emulator does not model directory buckets/S3-Express as a distinct bucket type at all; a real fix is a full S3 Express feature addition, not scoped for this pass. (gopherstack-3dqa)"
  - "RenameObject is applied uniformly to any bucket (general-purpose or directory), but real S3 restricts RenameObject to directory buckets only (api_op_RenameObject.go's Bucket doc: 'The bucket name of the directory bucket containing the object... Path-style requests are not supported'). This emulator has no directory-bucket-vs-general-purpose distinction anywhere (see CreateSession gap above), so RenameObject working on any bucket is a permissive superset rather than a wire-shape bug reachable by a real client hitting a real endpoint shape. Also: RenameObjectInput's DestinationIfMatch/DestinationIfNoneMatch/DestinationIfModifiedSince/DestinationIfUnmodifiedSince conditional-header preconditions are declared on the real input but not read/enforced by handleRenameObject (object_ops_copy.go) -- a caller relying on If-None-Match:* to prevent clobbering an existing destination gets a silent unconditional overwrite instead of a 412. Not fixed this pass (scoped feature, not a one-line diff); flagged honestly rather than silently left. (gopherstack-3dqa)"
  - "SelectObjectContent ScanRange (partial-object byte-range selection) is not implemented — requests with a ScanRange element are accepted but the range is ignored and the full object is scanned. Real semantics require record-boundary-aware slicing (a record is included if its first byte falls in [Start,End]) that's entangled with evaluateCSVQuery/evaluateJSONQuery's own record-splitting logic — implementing it correctly is a real feature addition, not a diff-and-fix, so it's left as an honest gap rather than a rushed subtly-wrong implementation."
  - "List*Configurations (analytics/inventory/metrics/intelligent-tiering) do not implement ContinuationToken-based pagination — IsTruncated is always false and all stored configs for a bucket are returned in one response. Real S3 caps at 100 entries per page; this only matters for buckets with >100 configs of one type, an edge case unlikely to be exercised by any realistic test."
  - "object_lambda: CreateAccessPointForObjectLambda and the whole Object Lambda *access point resource* (policy, configuration, ARN) genuinely belong to and ARE already fully implemented in services/s3control (object_lambda.go + handler_object_lambda.go + handler_object_lambda_test.go — verified: CreateAccessPointForObjectLambda, Get/Delete/List, Get/Put/DeleteAccessPointPolicyForObjectLambda, policy-status, and configuration are all real backend-state ops, not stubs). services/s3's own object_lambda.go (SetObjectLambdaConfig + WriteGetObjectResponse) is legitimately s3 DATA-PLANE surface — confirmed WriteGetObjectResponse is an aws-sdk-go-v2/service/s3 operation, not service/s3control — so it is NOT mis-scoped. What IS a real, disclosed limitation: GetObject only recognizes a Lambda wired in via the Go-only SetObjectLambdaConfig test hook, not via genuine access-point-ARN routing (calling GetObject with Bucket=<object-lambda-access-point-ARN>). Wiring that would require access-point-ARN parsing on every object route PLUS a live cross-service lookup into s3control's backend — and regular (non-Lambda) S3 Access Points have zero ARN-as-bucket routing support anywhere in this service either (grepped: no accesspoint/AccessPointARN handling exists in services/s3), so Object Lambda access points would be building ARN routing on a foundation that doesn't exist yet. This is a real, larger cross-service feature, not a diff-and-fix; left honestly open with the evidence above rather than attempted as a rushed partial wiring."
  - "SelectObjectContent's SQL engine internals (select_sql_parser.go/select_sql_tokenizer.go/select_sql_expr.go) were not re-diffed against the S3 Select SQL dialect spec this pass — only the request-handling wrapper (SSE-C headers) was fixed. The engine's existing extensive test coverage (select_test.go, select_advanced_test.go) was re-run and passes; no correctness re-audit of parser/expression-evaluator edge cases was performed."
  - "ListBuckets does not implement the bucket-region/prefix/continuation-token/max-buckets request parameters (filtering or pagination) — ListBucketsInput is always passed empty to the backend, and every bucket the account owns is always returned in one response. A deliberate, disclosed gap: real S3 also gates whether BucketRegion appears in the response on the request being 'paginated' (see the ListBuckets ops note above), which only matters once pagination exists. Adding real filtering/pagination here is a separate feature, not part of the BucketRegion display fix."
  - "ListDirectoryBuckets is structurally unreachable from any real, unmodified aws-sdk-go-v2 client pointed at gopherstack's single local endpoint (gopherstack-0bq8, 2026-08-14). Confirmed against s3@v1.106.5: ListDirectoryBucketsInput.bindEndpointParams sets UseS3ExpressControlEndpoint, and real AWS distinguishes ListDirectoryBuckets from ListBuckets purely by literal hostname (s3express-control.<region>.amazonaws.com vs s3.<region>.amazonaws.com) — the request itself carries no query param, path segment, or header that differs (HttpBindings only sets continuation-token/max-directory-buckets, same shape family as ListBuckets' own params; the addIsExpressUserAgent middleware that tags S3-Express traffic keys off a Bucket field this op doesn't have). The router's isListDirectoryBucketsRequest checks a ?list-type=directory query key no real client ever sends — dead code, confirmed by cross-referencing every query-setting line in the op's own HttpBindings function — so every real ListDirectoryBuckets() call silently falls through to listBuckets (200 success, wrong bucket set: general-purpose buckets instead of directory buckets). Not fixed: there is no real discriminator available to key on when serving both operations from one endpoint (same structural class as the cloudfront KeyValueStore data-plane ops, which structurally can never reach that service's Handler either). Left as documented dead code rather than deleted or silently 'fixed' with another fabricated key, since it's the only way any test (including buckets_test.go's existing TestListDirectoryBuckets) can reach the op at all."
leaks: {status: clean, note: janitor ctx-parented w/ <-ctx.Done() stop; replication goroutines WaitGroup-drained; Shutdown() cancels; object_lambda config now cleared on DeleteBucket (was previously leaking across bucket-name reuse — see 2026-07-24 section)}
---

## Notes
- **CRITICAL prior bug (fixed here):** SSE-S3/SSE-KMS EncryptionDEK/Nonce were `json:"-"` while ciphertext persisted → every encrypted object undecryptable after snapshot/restore (silent data loss). Multipart SSE same. Now persisted base64; SSE-C key stays request-scoped (`SSECKeyB64` json:"-").
- Persistence: backend converted (parity sweep 3, ce30166a) to `pkgs/store.Registry` — `buckets`/`uploads` are now `store.Table`s keyed by name/UploadID (bucket `Region` moved onto `StoredBucket`, replacing the old `map[region]map[name]` nesting + separate `bucketIndex`; `uploadsByBucket` is a `store.Index` replacing the old `map[bucket]map[uploadID]` nesting). Snapshot format bumped to `{"version":1,"tables":{...}}`; older/versionless snapshots are discarded cleanly on Restore (not partially decoded) — a deliberate one-time break, matching services/ec2 and services/sqs. `tags` stays a plain map (composite key, no identity field on the value).
- Trap: `x-amz-storage-class` header is OMITTED for STANDARD objects (AWS behavior) — don't re-add it.

## 2026-07-11 re-audit (a007ec3e, since 708d1961)
Only local drift was ce30166a's `pkgs/store` conversion of `backend_memory.go`/`persistence.go`/`janitor.go` (region-nested bucket maps + `bucketIndex` → `store.Table[StoredBucket]` keyed by name; `uploads` nesting → `store.Table[StoredMultipartUpload]` + `uploadsByBucket` `store.Index`) plus 3 no-op lint/formatting touches (bucket_ops.go, post_object.go, presign.go). Traced every call site touched by the refactor (getBucket/DeleteBucket/ListBuckets/Regions/BucketsByRegion/GetBucketMetadata/CreateMultipartUpload/CompleteMultipartUpload/AbortMultipartUpload/ListParts/janitor sweeps/Purge/Reset/Snapshot/Restore) against pkgs/store's documented semantics (no internal locking — still guarded by the single `b.mu`; `Index.Get` returns an index-owned slice — callers correctly copy IDs out before `Delete` in `purgeUploadsForBucketLocked`/`abortStaleMultipartUploads`). No wire-shape, error-code, state, or persistence regressions found — behavior is a faithful 1:1 port. `go build/vet/test -race/fix/golangci-lint` all clean (0 issues). SDK bumped v1.104.2→v1.105.0 (e51c0de9): CHANGELOG shows serializer-test-only change, `api_op_*.go` file set identical — no new ops to audit.
No fixes were required this sweep.

## 2026-07-24 parity-3 pass
Focused, deep-dive pass (not a full re-diff of every family — s3 is too large for one sitting; see `items_still_open`). Two real, wire-visible bugs found and fixed with regression tests, plus one stale-doc/gap cleanup:

1. **DeleteBucket accepted non-empty buckets (real bug).** `InMemoryBackend.DeleteBucket` (buckets.go) unconditionally marked any existing bucket `DeletePending` and let the janitor asynchronously drain its objects in the background — `ErrBucketNotEmpty` was defined and wired into the handler's error-check path but the backend never actually returned it. Real S3 rejects `DeleteBucket` on a non-empty bucket (any object, object version, or delete marker) with 409 `BucketNotEmpty` — the caller must empty it first; there is no async/best-effort deletion. Also added the well-documented real-AWS gotcha where **incomplete multipart uploads** block deletion even though they never show up in `ListObjects`/`ListObjectsV2`. Fixed by checking `len(bucket.Objects) > 0` (under `bucket.mu.RLock`, nested inside the exclusive `b.mu.Lock` — safe because lock order is always b.mu→bucket.mu everywhere else in this package, and `getBucket()` already routes all object-mutating ops through `b.mu.RLock` first, so no writer can race in between the check and marking `DeletePending`) and `len(b.uploadsByBucket.Get(bucketName)) > 0`, returning `ErrBucketNotEmpty` for either. The chunked drain machinery in `janitor.go` (`processBucket`/`drainChunkSize`) is kept as-is — it's now a fast no-op safety net since a bucket can only reach `DeletePending` while already empty, but ripping it out was unnecessary extra blast radius for this pass. Updated 4 tests across `store_test.go`, `janitor_test.go`, and `buckets_test.go` that had encoded the old (buggy) "async deletion" behavior as intentional; added 2 new regression cases (object-present and MPU-present → BucketNotEmpty). Also fixed a real caller: `services/cloudformation/resources.go`'s `deleteS3Bucket` now correctly surfaces the error instead of silently accepting non-empty-bucket deletes — this actually *improves* CFN accuracy too, since real CloudFormation is well known to fail stack deletion on non-empty S3 buckets without `DeletionPolicy: Retain`.
2. **Object Lambda config leaked across bucket-name reuse.** `S3Handler.objectLambdaConfigs` (object_lambda.go) is keyed by bucket name on the *handler* (not the backend's per-bucket state), and was never cleared on `DeleteBucket`. A bucket deleted and recreated under the same name would silently inherit the previous incarnation's Lambda ARN wiring on `GetObject`. This matches the stale `gaps:` entry "object_lambda ... no delete-on-bucket-delete" from the 2026-07-11 audit. Fixed with a new `clearObjectLambdaConfig` helper called from the handler's `deleteBucket`. (The other half of that old gap note, "dual-lock", was investigated and not reproducible — `objectLambdaMu` is never nested with any other lock in the current code; treating it as resolved/stale.)
3. **PutBucketPolicy accepted any string as a "policy".** No JSON validation at all — real S3 returns 400 `MalformedPolicy` for a non-JSON body. Added `json.Valid()` check before persisting, with a new `ErrMalformedPolicy` error wired into `configErrorTable`. Full IAM-policy-shape semantic validation (Version/Statement/Effect/Action/Resource) is NOT implemented — flagged honestly in `gaps`.
4. **Stale gap: multipart-upload TTL.** The 2026-07-11 `gaps:` entry "abandoned multipart uploads have no per-upload TTL" is no longer accurate — `janitor.go`'s `cleanupDefaultMultipart` (24h unconditional floor, independent of any lifecycle `AbortIncompleteMultipartUpload` rule) already existed and handles this. Removed from `gaps`.
5. **Doc-only: `s3StubOperations` naming.** Verified every operation in that list (`handler_operations.go`) is fully implemented (real backend state mutation, not canned responses) — spot-checked `RestoreObject`, `GetBucketAbac`/`PutBucketAbac`, `GetBucketPolicyStatus`, `UpdateBucketMetadataInventoryTableConfiguration`/`UpdateBucketMetadataJournalTableConfiguration`. The "stub" label was misleading (functionally harmless since `GetSupportedOperations()` merges both lists); corrected the doc comment in place. **CORRECTION (2026-08-13, gopherstack-ob1g): this spot-check confirmed `GetBucketAbac`/`PutBucketAbac` were wired to real backend state, which was true, but missed that the wire shape itself was wrong end-to-end — see the `PutBucketAbac/GetBucketAbac` op row above.** "Implemented" and "wire-correct" are different claims; this note conflated them.

`go build ./...` (full tree), `go test -race ./services/s3/...`, `go vet`, `gofmt -l`, and `golangci-lint run ./services/s3/...` all clean. No banned nolints found (none existed before or after this pass). `git diff --stat go.mod go.sum` empty.

Not re-diffed this pass (time-boxed): SelectObjectContent SQL engine, full bucket-config families (logging/notification/metadata-table/analytics/inventory/intelligent-tiering/metrics/replication details), presign signature internals, chunked/streaming upload, checksum/compression paths — these carry forward the 2026-07-11 audit's "ok" status un-re-verified this round.

## 2026-07-24 (phase 2) parity-3 pass — closed every tracked gap

Follow-up to the same-day pass above. Goal: close every item the first pass had deferred to `gaps:` (PutBucketPolicy semantic validation, object_lambda scoping, `s3StubOperations` naming, and the "whole families never re-diffed" catch-all). Also fixed the CI-failing integration test.

1. **Integration test fix (`test/integration/s3_test.go`).** `TestIntegration_S3_BucketLifecycle/delete_non-empty_bucket_succeeds_(async)` asserted the OLD pre-fix async-delete behavior that the same-day pass above had already made obsolete in the emulator (see `bucket_delete` fix). Verified against the real SDK: `aws-sdk-go-v2/service/s3/api_op_DeleteBucket.go`'s doc comment lists `BucketNotEmpty` / `409 Conflict` as a documented real error. Renamed the case to `delete non-empty bucket fails with BucketNotEmpty` and rewrote it to assert the 409 `BucketNotEmpty` `smithy.APIError`, then empty the bucket and retry successfully — this is real S3 behavior, so the emulator (post-first-pass) was already right and only the test was stale.

2. **PutBucketPolicy: implemented full IAM-policy-grammar shape validation** (new file `bucket_policy_validation.go`). Validates Version (optional; if present must be `2008-10-17`/`2012-10-17`), Statement (required, non-empty, single-object-or-array per IAM convention), and per-statement Effect (`Allow`/`Deny`), Principal/NotPrincipal (required — bucket policies are resource-based, unlike identity policies), Action/NotAction, Resource/NotResource — grammar sourced from https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_grammar.html. The "missing Principal" message (`Missing required field Principal cannot be empty!`) is a confirmed real-world S3 error string (cross-checked via a public bug report using that exact text). 12 new table-driven test cases in `acl_policy_test.go` (`TestS3PutBucketPolicy_SemanticValidation`); the existing `TestS3BucketPolicyCRUD` fixture (`Statement:[]`) was rewritten to a well-formed statement since empty-Statement is now correctly rejected.

3. **object_lambda: investigated and resolved with evidence, not deferred.** Confirmed `services/s3control` already fully implements `CreateAccessPointForObjectLambda` and the entire Object Lambda access-point *resource* surface (`object_lambda.go`, `handler_object_lambda.go`, tests) — that part of the gap was already correctly scoped to s3control, not a real s3 gap. Also confirmed `WriteGetObjectResponse` is a genuine `aws-sdk-go-v2/service/s3` (not `s3control`) operation, so `services/s3`'s existing Lambda-invocation/WriteGetObjectResponse plumbing is NOT mis-scoped either. The one real, disclosed limitation — GetObject only recognizes a Lambda wired via the Go-only `SetObjectLambdaConfig` test hook, not real access-point-ARN routing — is left open with full evidence in `gaps` (regular, non-Lambda Access Points have zero ARN-as-bucket support anywhere in this service either, confirmed by grep; this is a real, larger cross-service feature).

4. **`s3StubOperations` renamed to `s3ExtendedOperations`** (`handler_operations.go`), with an updated doc comment restating which ops were spot-verified as fully implemented. Only 2 call sites, both internal to the package (`handler.go`'s `GetSupportedOperations` and the function's own definition) — no external callers, no test manifests referencing the old name.

5. **CopyObject — 4 real bugs found and fixed** (`object_ops_copy.go`, `sse_crypto.go`, `object_ops_headers.go`):
   - `CopyObjectResult` (`model.go`) was missing the checksum fields real `types.CopyObjectResult` carries (ChecksumCRC32/CRC32C/SHA1/SHA256/CRC64NVME) — added, and wired a `copyChecksumAlgorithm` helper that recomputes using the source's checksum algorithm (or an explicit `x-amz-checksum-algorithm` override on the copy request, both real S3 behaviors).
   - `LastModified` in the response was `time.Now().UTC()` computed independently of (and after) the timestamp the backend actually stamped on the new version — could disagree with a subsequent HeadObject. Now reads back the real value via `h.Backend.HeadObject`.
   - Destination SSE-S3/SSE-KMS request headers (`X-Amz-Server-Side-Encryption*`) were never read at all — a CopyObject requesting encryption for the new object was silently stored unencrypted with no SSE response headers. Now extracted via `extractSSEInfo` and threaded through exactly like PutObject.
   - **Severe:** copy-source SSE-C headers (`X-Amz-Copy-Source-Server-Side-Encryption-Customer-*`) were never read. Copying an SSE-C encrypted source without them didn't error — `decryptVersionForGet` silently returned raw ciphertext, which got copied to the destination as if it were plaintext (silent data corruption). Added `extractCopySourceSSECInfo`/`validateCopySourceSSECOnRead` (mirroring the existing GetObject SSE-C validation) so the copy now correctly fails with 400 `InvalidRequest`/`BadDigest` when the key is missing/wrong, and decrypts correctly when it's supplied. This also transparently fixes `UploadPartCopy`, which shares `copySourceData`.
   - Also added the `X-Amz-Expiration` response header (destination object is subject to the destination bucket's lifecycle rules, same as PutObject).
   - New/rewritten tests in `object_ops_copy_test.go`: `TestCopyObject_ChecksumPropagatedToResult`, `TestCopyObject_DestinationSSE_KMS`, `TestCopyObject_SSECSource_RequiresCopySourceKey`.

6. **PostObject — 3 form fields were silently ignored** (`post_object.go`): `x-amz-storage-class`, `x-amz-server-side-encryption`/`-aws-kms-key-id`, and `x-amz-checksum-algorithm` are all real, documented POST-policy fields (same semantics as their PutObject header equivalents) that were never read. A presigned-POST upload requesting SSE-KMS was silently stored unencrypted — defeats the whole point of specifying it. Fixed by wiring the same `extractSSEInfo`/checksum machinery PutObject uses, sourced from form fields instead of headers (refactored `extractChecksumPointers` into a shared `extractChecksumValues(get func(string) string, algo string)` so both header- and field-based lookups share one implementation). New tests: `TestHandler_PostObject_SSE`, `TestHandler_PostObject_StorageClass`, `TestHandler_PostObject_ChecksumAlgorithm`.

7. **SelectObjectContent — SSE-C headers never read** (`select.go`). Confirmed via the real SDK's serializer that `SelectObjectContentInput.SSECustomerAlgorithm/-Key/-KeyMD5` are HTTP-header-bound (`awsRestxml_serializeOpHttpBindingsSelectObjectContentInput`), not XML body fields despite living on the same Go struct as the body-bound fields. `readSelectRequest` now extracts and validates them exactly like GetObject. New test: `TestHandler_SelectObjectContent_SSEC`.

8. **SEVERE, highest-value find: doubly-nested XML in all 4 `List*Configurations` ops** (`bucket_ops_analytics.go`). `writeConfigListXML` — shared by `ListBucketAnalyticsConfigurations`, `ListBucketIntelligentTieringConfigurations`, `ListBucketInventoryConfigurations`, `ListBucketMetricsConfigurations` — wrapped each stored config string in `<ElementTag>...</ElementTag>` before emitting it. But the stored string is the RAW PUT request body, which (confirmed against the real SDK's serializer, e.g. `awsRestxml_serializeOpPutBucketAnalyticsConfiguration`) is *already* a complete `<AnalyticsConfiguration>...</AnalyticsConfiguration>` document — the whole XML body root, not just its inner fields (same pattern confirmed for Inventory/Metrics/IntelligentTiering). The result was doubly-nested XML (`<AnalyticsConfiguration><AnalyticsConfiguration>...`) that the real SDK's unwrapped-list deserializer (`awsRestxml_deserializeDocumentAnalyticsConfigurationListUnwrapped` et al — confirmed each treats a top-level `<AnalyticsConfiguration>` element itself as one list entry) could not have parsed correctly: every `Id`/`Filter`/etc field would have decoded as empty. The existing tests never caught this because they only asserted loose substring containment (`wantBody: "AnalyticsConfiguration"`), which is true either way. Fixed by emitting each stored config verbatim (dropped the now-dead `elementTag` parameter from `writeConfigListXML`, updated all 4 call sites). New regression test `TestS3_ListBucketConfigurations_NoDoubleNesting` (all 4 families) structurally walks the response with `encoding/xml` the same way the real deserializer does and asserts `Id` decodes correctly one level deep, plus a substring check that the doubled-tag pattern never appears.
   - Also field-diffed `logging`/`notification`/`metadata-table` (raw request-body passthrough on Put+Get, no reconstruction — verified this pattern carries no wire-shape risk since the stored bytes ARE the wire bytes) and confirmed no equivalent bug; `versioning`/`tagging` reconstruct via Go structs from real `types.*` shapes and were spot-checked against the SDK, no issues found.

`go build ./...` (full tree), `go vet ./...` (full tree), `go test -race -count=1 ./services/s3/...`, `go test -race -count=1 -run '^TestIntegration_S3_BucketLifecycle$' ./test/integration/...` (after building `bin/gopherstack`), and the broader `TestIntegration_S3*` integration suite all pass. `gofmt -l services/s3/` and `golangci-lint run ./services/s3/...` both clean (0 issues). No banned `nolint`s. `git diff --stat go.mod go.sum` empty — no dependency changes.

Genuinely not re-diffed this pass (disclosed in `gaps` with evidence, not silently carried forward): SelectObjectContent's ScanRange partial-selection and its SQL-engine internals (parser/tokenizer/expression evaluator), and List*Configurations pagination.

## 2026-08-03 ListBuckets: BucketRegion for dashboard cross-region visibility

User report: buckets created (e.g. by tests) in `ap-southeast-1` while the dashboard's region selector is on `us-east-1` correctly still appear in the bucket list (ListBuckets is account-global, both in real S3 and here — confirmed no regression to that in `TestDashboardRegionScoping_ListBucketsIgnoresSelectedRegion`), but nothing on screen said those buckets actually live in a different region than the one selected.

Fix: `services/s3/buckets.go`'s `ListBuckets` now populates `types.Bucket.BucketRegion` for every entry from the same `StoredBucket.Region` that `enforceBucketRegion` (`handler.go`) already gates cross-region object/bucket access on — one source of truth, can't drift. Verified against the real SDK (`aws-sdk-go-v2/service/s3@v1.106.0/types/types.go:355-358`) and the real `ListBuckets` API docs (`docs.aws.amazon.com/AmazonS3/latest/API/API_ListBuckets.html`): the paginated response examples show `<BucketRegion>us-east-1</BucketRegion>` explicitly — confirming BucketRegion does NOT carry GetBucketLocation's legacy empty-string-for-us-east-1 quirk (`LocationConstraint`, `bucket_ops.go:584-591`), so no special-casing was added. `services/s3/model.go`'s hand-rolled `BucketXML` (this handler doesn't use the SDK's own XML serializer for ListBuckets) gained a matching `BucketRegion` field; `bucket_ops.go`'s `listBuckets` now copies it through.

Deliberate, disclosed scope decision (also in `gaps`): real S3 only echoes BucketRegion when the request carries at least one of bucket-region/prefix/continuation-token/max-buckets (the *unpaginated* doc example omits it entirely). This backend has never implemented ListBuckets pagination or filtering — `ListBucketsInput` is passed empty to the backend regardless of query string — so there's no paginated/unpaginated distinction to gate on, and the field's whole purpose here is dashboard visibility, not byte-exact request-shape mimicry of an AWS nuance nobody would notice. BucketRegion is therefore always populated. `ListDirectoryBuckets` was deliberately left untouched — its own doc explicitly excludes BucketRegion from that response shape.

UI: `ui/src/routes/s3/+page.svelte`'s bucket-card grid (this page predates the `defineColumns` convention used elsewhere and is hand-rolled `<table>`/card markup, matched rather than converted) gained a "Region: <value>" line per card, plus a subtle amber "different region" badge (with an explanatory `title` tooltip) when a bucket's `BucketRegion` differs from `currentRegion()` — informational styling, not an error state, and the list is never filtered by it.

New tests: `TestDashboardRegionScoping_ListBucketsReportsEachBucketsTrueRegion` (Go, `dashboard_region_scoping_test.go` — us-east-1 reports explicitly, a second bucket's true `ap-southeast-1` is reported regardless of the request's signed region) and two cases in `ui/src/routes/s3/page.test.ts` (matching-region shows no marker; differing-region shows the marker with the real region in both the label and the tooltip).

`go build ./...`, `go vet ./...`, `go test -race -count=1 ./services/s3/...` (x2), `gofmt -l services/s3/`, `golangci-lint run ./services/s3/...` all clean. UI: `npm run lint`, `npm run fmt:check`, `npm run check`, `npm run test` all clean, suite count only grew (see git history for exact before/after counts).

## 2026-08-13 discarded xml.Unmarshal errors sweep (gopherstack-ob1g)

`encoding/xml` returns an error when a document's root element doesn't match the target
struct's `XMLName` tag, and leaves the struct **zeroed**, not partially filled — so
`_ = xml.Unmarshal(body, &req)` with a wrong root silently discards the entire request and
proceeds on zero values. Swept all 4 remaining non-test `_ = xml.Unmarshal(...)` call sites in
`services/s3/` and re-verified each struct's `XMLName` against the pinned SDK
(`aws-sdk-go-v2/service/s3@v1.106.5`) serializer.

**One genuine whole-request wipe found and fixed**: `GetBucketAbac`'s re-parse of its own
stored `PutBucketAbac` body used root `AbacConfiguration`, where the real root is `AbacStatus`
— see the `PutBucketAbac/GetBucketAbac` op row above for the full two-bug writeup (a response-
shape bug was hiding directly behind the root-mismatch one, caught only by driving both ends
through the real aws-sdk-go-v2 client). Confirmed to fail against the pre-fix code by
reverting by hand; corrects the 2026-07-24 (phase 2) `s3StubOperations` spot-check above, which
verified these ops were backend-wired but not that the wire shape was correct.

**The remaining 3 occurrences (`PutBucketRequestPayment`, `RestoreObject`,
`PutBucketAccelerateConfiguration`) had a correct `XMLName` already** — hardening, not a live
bug. Each now returns the service's `MalformedXML` error (`errMalformedXML`/
`errMalformedXMLMsg`, the pattern already established elsewhere in this package, e.g.
`bucket_ops_cors.go`) instead of silently discarding the error. Covered by
`TestXMLUnmarshalErrorHandled` and `TestRestoreObject_MalformedBodyHandled`
(`xml_unmarshal_error_handling_test.go`), which fail against the pre-fix
`_ = xml.Unmarshal(...)` form.

`go build`, `go vet`, `gofmt -l`, `go fix -diff`, `go test -race`, and
`golangci-lint run` all clean for `./services/s3/...`. The matching cloudfront sweep (28
occurrences, two more genuine wipes plus two routing bugs found as a second layer) is recorded
in `services/cloudfront/PARITY.md`.

## 2026-08-13 deep pass: correctness/completeness/optimization (gopherstack-3dqa)

User-directed priority pass (s3 + dynamodb, tracked separately) applying this session's
confirmed bug classes (wrong element names/wrapper shapes, required members never
read/populated, timestamp decode breaks, over-wide List entries) to s3 specifically, plus a
concurrency and an optimization pass. Not a full re-diff of every op (s3 is ~44.6k lines
across 87 non-test files) -- see "not reached" below for what a follow-up should cover.

**Four real bugs found and fixed, each verified to fail against the pre-fix code by hand-reverting
(not just "looks fixed"), driven through either the real aws-sdk-go-v2 client or a direct backend
call plus `-race`:**

1. **CreateMultipartUpload/CompleteMultipartUpload/ListMultipartUploads StorageClass** --
   silently dropped end-to-end (handler never read the header; backend had nowhere to store it;
   list response never reported it or Owner/Initiator). See the ops table row above.
2. **PutBucketReplication Filter>Prefix ignored, causing over-replication** -- the modern,
   AWS-docs-recommended replication rule shape was silently treated as "no filter, replicate
   everything." A real correctness/data-exposure bug, not a mere miss. See the ops table row.
3. **RenameObject: real data race**, confirmed with `-race`, not a theoretical lock-ordering
   concern -- an unsynchronized map write racing a concurrent map read. RenameObject had zero
   prior test coverage. See the ops table row.
4. **WriteGetObjectResponse: X-Amz-Fwd-Status discarded**, always reporting 200 to the original
   GetObject caller regardless of what an Object Lambda function actually returned (e.g. a 403
   from an access-control Lambda). See the ops table row.

**Completeness**: the Object Annotations operation family (`PutObjectAnnotation`,
`GetObjectAnnotation`, `DeleteObjectAnnotation`, `ListObjectAnnotations`,
`UpdateBucketMetadataAnnotationTableConfiguration`) -- 5 real, current SDK operations, previously
missing entirely -- was implemented 2026-08-14 (gopherstack-zi7k); see the families row and gaps
entry above for routing citations and the deliberately-unenforced edges. `CreateSession` (S3
Express) was re-examined beyond its existing "stub" doc comment: it ignores `SessionMode` and
doesn't check `IsDirectoryBucket`, on top of returning canned credentials, consistent with this
emulator having no directory-bucket modeling at all. Disclosed in `gaps` rather than attempted
as a rushed partial feature.

**Optimization**: inspected lock scope on the hot write/read paths (`PutObject`/`GetObject`
via `checkPutObjectAuthAndLock`/`saveObjectVersion`, `ListObjects`/`ListObjectsV2` via
`processListObjects`, async `triggerReplication`, `TaggedResources`, the lifecycle janitor
sweep) -- compression/checksumming/encryption already run outside any lock (existing code,
confirmed by reading, not assumed), `b.mu`/`bucket.mu` are held only for pointer lookups and
map mutations, and no lock is held across anything IO-bound (there is no real IO in this
in-memory backend; the closest analog, async replication's cross-object `PutObject`/`GetObject`
calls, already runs after `bucket.mu` is released). No quadratic, whole-object-copy, or
lock-across-IO pattern was found with hard evidence in the code paths inspected. This was a
targeted inspection, not a profiling run with wall-clock numbers -- no benchmark was added
because no candidate bug was found to benchmark; a genuine profiling pass (pprof under
realistic object/version counts) is not something this pass did and would be needed to make a
stronger claim than "nothing obviously wrong was found by reading."

**Existing tests that encoded a bug as correct**: none found this pass beyond what
gopherstack-ob1g already documented (raw-body substring assertions that can't distinguish a
correct key from a wrong one) -- this pass added net-new coverage (`RenameObject` had none at
all) rather than finding mis-asserting tests to correct.

**What this pass did NOT reach** (honest scope, not silently carried forward as "ok"): full
per-op wire-shape re-verification of ACL grants beyond what's cited above, CORS, lifecycle
transitions, notification configurations, website configuration, the metadata-table family
beyond confirming raw-passthrough carries no wire risk, presign/sigv4 internals, chunked
upload internals, the SelectObjectContent SQL engine (carried forward from 2026-07-24 as
un-re-diffed), and persistence round-trip fuzzing beyond the fields this pass touched
(StorageClass on multipart uploads round-trips via existing JSON-tag machinery, spot-verified
by reading, not by a dedicated snapshot/restore test). `go build ./...`, `go vet
./services/s3/...`, `go test -race -count=1 ./services/s3/...`, `go fix -diff
./services/s3/...` (no diff), `golangci-lint run ./services/s3/...` (0 issues, no new
`//nolint`s), and `go test -race -count=1 ./pkgs/...` all clean.

## 2026-08-14 method/path-parameter route audit (gopherstack-0bq8)

Scope: cross-checked all 112 s3@v1.106.5 operations' real HTTP method + path
against the router's dispatch (`handler.go`'s method switch in
handleBucketOperation/handleObjectOperation, then the query-param switches in
`bucket_ops.go`/`object_ops.go`), extracting each op's `request.Method` and
`httpbinding.SplitURI` argument straight from `serializers.go`. This is new
ground beyond gopherstack-zr2u, which already swept every query-param
subresource discriminator (and fixed 7 there) — this pass targeted HTTP
method dispatch and path-parameter shape specifically, since s3's dispatch
tree is method-switch-first (Go `switch` on `r.Method`, immune to
cross-method bugs by construction) and s3 uses one path-parameter name
(`Key`) throughout, so there is no GeneratedTemplateId/Name-class mismatch
possible here.

Hand-verified as already correct (worth recording so it isn't re-derived):
CopyObject/UploadPartCopy are disambiguated from PutObject/UploadPart by the
`X-Amz-Copy-Source` header (`uploadPart` in multipart_ops.go checks the
header and delegates), not a fourth routing mechanism gone wrong;
GetObjectAnnotation vs ListObjectAnnotations' shared GET route is correctly
gated on the real `annotationName` query param; CreateBucketMetadataConfiguration/
CreateBucketMetadataTableConfiguration are correctly POST (not PUT); the
Get-vs-List split for analytics/intelligent-tiering/inventory/metrics
sub-resources is correctly gated on `id` presence.

One finding, structural rather than fixable — see the ListDirectoryBuckets
gap entry above: the router's `list-type=directory` discriminator is dead
code no real client ever sends, and ListDirectoryBuckets silently falls
through to ListBuckets. Not a routing bug with a real fix, since real AWS's
only discriminator (hostname) doesn't exist in gopherstack's single-endpoint
architecture. Documented, not patched with another fake key.

Not reached this pass: s3control (a separate SDK client/package,
out of scope), and no attempt was made to re-verify the query-discriminator
space zr2u already covered (per gopherstack-0bq8's explicit instruction not
to re-derive that result).

Gates: `go build ./services/s3/...`, `go vet ./services/s3/...`, `go test
-race ./services/s3/...`, `go fix -diff ./services/s3/...` (no diff),
`golangci-lint run ./services/s3/...` (0 issues) all clean.

## 2026-08-14b mechanical struct-field diff + optimization follow-up (gopherstack-3dqa)

User-directed priority pass, the s3 sibling of the dynamodb pass closed in 89eac08ea.
Before touching code: checked `bd show gopherstack-3dqa` (already closed, four prior
rounds, 21 bugs) and `git log --oneline -- services/s3/PARITY.md`, which matched this
document's own close-reason text commit-for-commit (`02bccc3d1`, `22eea2bab`) -- no stale
claim found here this time, unlike dynamodb's six-commits-stale GSI note. What genuinely
had not been done for s3 yet: the mechanical struct-field diff method (only ever applied
to dynamodb, confirmed by `git log --all --grep="struct-field diff"` returning exactly one
commit), and real profiling numbers for the optimization axis, which the last round
explicitly recorded as "inspected, not profiled."

**Method**: s3 has no single generated wire-model file the way dynamodb does --
responses are hand-rolled XML structs (`model.go`, `types.go`) plus headers written
directly in handlers. So the diff was per-family: read each response struct next to the
matching real SDK `types.*`/`api_op_*.go` Output struct (`aws-sdk-go-v2/service/s3@v1.106.5`
under `$(go env GOMODCACHE)`), then hand-verify every hit against the real
serializer/deserializer before treating it as a bug -- per the standing warning, the diff
over-reports on SDK-internal fields (`noSmithyDocumentSerde`) and Go-vs-XML naming.
Checked: `Object`/`ObjectVersion`/`DeleteMarkerEntry`/`Part`/`MultipartUpload`/`Grant`/
`Grantee` against `ObjectXML`/`ObjectVersionXML`/`DeleteMarkerXML`/`PartXML`/
`MultipartUpload`/`Grant`/`Grantee`.

**Two real, hand-verified bugs found and fixed, each half proven load-bearing by
independent hand-revert (not just "looks fixed")**:

1. **UploadPart never echoed the checksum it computes and verifies, in either the
   response headers or ListParts.** `Part.ChecksumCRC32/-CRC32C/-SHA1/-SHA256`
   (`types/types.go:3904+`) and `UploadPartOutput`'s same fields are header-bound
   (confirmed `deserializers.go:14957`,
   `awsRestxml_deserializeOpHttpBindingsUploadPartOutput` reads
   `x-amz-checksum-crc32` etc from the response). `multipart.go`'s backend `UploadPart`
   already computes and verifies these checksums (`verifyChecksum`) and returns them on
   `s3.UploadPartOutput` -- but `multipart_ops.go`'s HTTP handler only ever wrote the
   `ETag` header, discarding the computed values entirely, and `StoredPart` (`types.go`)
   had no fields to persist them on, so `ListParts` could never report them either even
   if the handler had. Two stacked gaps, same shape as dynamodb's `AttributesToGet`
   finding this session: fixed the handler (writes `x-amz-checksum-*` via the existing
   `setChecksumHeaders` helper already used by GetObject/PutObject) and the backend
   (added `ChecksumCRC32/-CRC32C/-SHA1/-SHA256 *string` to `StoredPart`, threaded into
   `ListParts`' `types.Part` and the handler's `PartXML`). `TestUploadPart_
   ChecksumEchoedInResponseAndListParts` drives the real SDK client end-to-end
   (UploadPart with `ChecksumAlgorithm: CRC32` -> asserts the response's `ChecksumCRC32`
   -> ListParts -> asserts the same value comes back). Hand-reverted the handler half
   alone (response nil) and the persistence half alone (ListParts nil) -- each failed
   independently, proving both are load-bearing, then restored byte-identical (`diff`
   confirmed against the pre-edit copy).
2. **ListObjectVersions never carried ChecksumAlgorithm, though ListObjectsV2 already
   does for the exact same underlying data.** `types.ObjectVersion.ChecksumAlgorithm`
   (`types/types.go:3775`) is real and already threaded through `ListObjectsV2`'s
   `ObjectXML` (`processObjectSnapshots`/`objectFromVersion` in `listing.go`) --
   `StoredObjectVersion.ChecksumAlgorithm` (`types.go`) is the same field on the same
   struct either op reads. But `versionSnapshot` (the intermediate type
   `ListObjectVersions` uses) never captured it, `buildVersionPage` never set it on
   `types.ObjectVersion`, and `ObjectVersionXML` had no field at all -- so a versioned
   object's checksum algorithm silently disappeared on the one API most likely to be
   called on a versioned bucket. Fixed by threading `checksumAlgorithm` through
   `versionSnapshot` -> `buildVersionPage` -> the new `ObjectVersionXML.ChecksumAlgorithm`
   field -> `mapListVersionsOutput`. **Explicitly NOT touched**: `ObjectVersionXML`'s
   existing `StorageClass` field, which the diff also flagged as a mismatch (backend
   tracks the object's real storage class; the field is hardcoded to `"STANDARD"`) --
   verified against `types/enums.go:1134-1149`,
   `ObjectVersionStorageClass` has exactly ONE valid enum value, `"STANDARD"`, so the
   existing hardcode is real-AWS-correct and the apparent mismatch was a false positive
   from the diff over-matching Go field names across two different-shaped enums (the
   exact "hand-verify against the real serializer" warning this campaign carries).
   `TestListObjectVersions_ChecksumAlgorithmPopulated` drives the real client
   (PutObject with `ChecksumAlgorithm: SHA256` on a versioned bucket -> ListObjectVersions
   -> asserts `Versions[0].ChecksumAlgorithm`); both the backend-threading half and the
   XML-mapping half were hand-reverted independently and each failed alone, then restored
   byte-identical.

**Header-bound member sweep beyond the two bugs above**: cross-checked `GetObject`/
`HeadObject`/`PutObject`/`CopyObject`'s existing `setChecksumHeaders`/`setSSEHeaders`/
`setCommonHeaders` call sites (`object_ops_headers.go`) against every header binding in
`GetObjectOutput`/`HeadObjectOutput`/`PutObjectOutput`'s real `HttpBindings` functions --
no further gaps found; these were already correctly wired going into this pass.

**Optimization -- measured, not just inspected, closing the one axis the prior four
rounds left as "inspected, not profiled"**: added `BenchmarkListObjectsV2` (`bench_test.go`)
against a 50,000-object bucket, three shapes (flat/no-prefix, prefix+delimiter,
delimiter-only). Before: `flat_maxkeys1000` cost 56.0ms/op, 12.0MB/op, 350,015 allocs/op
for a 1000-key page -- `processListObjects` (`listing.go`) built a full `types.Object`
(with its `Owner` pointer, `ChecksumAlgorithm` slice, four `aws.String`/`aws.Time` boxed
allocations) for every one of the 50,000 matching objects, sorted all 50,000, and only
THEN truncated to the 1,000 actually returned -- i.e. ~49,000 wasted per-object
allocations on every single page of a paginated walk over a large, unprefixed bucket
(the exact "hot loop" case this pass was told to check, since s3 is one of the two
services most likely to be hit that way by tests using this emulator). No lock was held
across this cost (confirmed unchanged from prior rounds' inspection) -- the cost was pure
unnecessary allocation, not a concurrency bug.

Fixed by splitting the no-delimiter path (`CommonPrefixes` is always empty there, so
truncation is a plain sorted-slice cut) to sort/marker-seek/truncate on lightweight
`*StoredObjectVersion` pointers first, and defer the `types.Object` conversion
(`objectFromVersion`, new) to only the page actually returned. The delimiter path is
untouched -- grouping into `CommonPrefixes` genuinely needs every matching key's full
`types.Object` up front, so changing it carried the regression risk this campaign has
seen before (a dynamodb GSI "optimization" that copied under lock and regressed to
O(table), caught only by its benchmark) for no measured benefit; left as-is rather than
risked. After: `flat_maxkeys1000` is 39.9ms/op, 1.04MB/op, 7,015 allocs/op -- a 92%
allocation-count reduction (350,015 -> 7,015) and ~11x reduction in bytes/op, with the
delimiter paths' numbers unchanged (confirming no regression there;
`common_prefix_only` stayed ~56ms/12.4MB/350k allocs, `prefix_delimiter` stayed
~2-3ms/650KB/3,527 allocs across both runs). The remaining ~40ms is the per-object
`obj.mu.RLock()` + map-lookup cost of resolving each of the 50,000 objects' latest
version, which is inherent to an unindexed `map[string]*StoredObject` and not
attempted this pass (a sorted-key index would be a larger structural change with its
own regression risk, better suited to a dedicated pass if this cost is ever shown to
matter in practice). `TestListObjectsV2_PaginationConsistency_NoDelimiter`
(`store_listing_test.go`) walks 253 objects in pages of 37 through the new fast path
and reconstructs the full sorted key set, asserting no key is dropped, duplicated, or
misordered, and that each page's `Owner`/`StorageClass` are still populated correctly;
hand-verified to catch an injected off-by-one in the truncation boundary before being
restored to the correct version.

**Not reached this pass**: `services/s3control` (separate package, out of scope for this
timebox; sibling issue if a dedicated pass is warranted); a sorted-key index for
`ListObjectsV2`'s remaining per-object lock cost; re-diffing `Grant`/`Grantee`,
`MultipartUpload`, and `DeleteMarkerEntry` beyond the read-and-compare above (no
mismatches found, but not independently regression-tested); presign/sigv4 internals and
the SelectObjectContent SQL engine (both carried forward un-re-diffed from prior rounds,
as already disclosed above).

Gates, all clean: `go build ./...`, `go build ./services/s3/...`, `go vet
./services/s3/...`, `go test -race -count=1 ./services/s3/...`, `go fix -diff
./services/s3/...` (no diff), `gofmt -l services/s3/` (no output), `golangci-lint run
./services/s3/...` (0 issues, no new `//nolint`), `go test -race -count=1 ./pkgs/...`.

## 2026-08-15 wrapper-key sweep (gopherstack-6flj)

s3 was named across five prior sessions of this issue's own remainder tracking
(`services/_WRAPPER_KEY_SWEEP_REMAINDER.md`) as "needs its own dedicated
session" -- 45 List/Describe/Get ops (12 List, 0 Describe, 33 Get), and this
was that session. Read the doc's method section, the prior s3 sessions above
in this file (five prior passes, 21 bugs, none of them a 6flj-scoped
wrapper-key sweep), and `git log -- services/s3` before starting, per this
issue's "check, don't trust PARITY claims" standing instruction -- s3's own
notes held up under that check this time.

**Protocol**: REST-XML (`awsRestxml_`, the sole prefix in
s3@v1.106.5/deserializers.go). Confirmed (established under gopherstack-7185,
not re-derived here) that this service's deserializers make zero `GetElement`
calls -- the empty-result-on-mismatched-root class this campaign otherwise
watches for structurally cannot happen here. What *can* happen, and did once
this pass: smithy-go's `NodeDecoder.Value`/child-element decode expects a
specific child element name and silently produces a zero-value struct when
that child is absent, which is a different failure mode from an XMLName
mismatch (see the GetBucketMetadataConfiguration finding below) --
confirmed by tracing `HandleDeserialize` (not just an `OpDocument*` function
name) for a dozen ops spanning every op family before trusting any single
one as reached.

**Header-bound members checked**: this session did not find a new
header-bound-member drop -- the known prior instance (`UploadPart`'s
checksums, `deserializers.go:14957`, fixed under gopherstack-3dqa) was
re-verified still fixed and correct, and the config-echo ops swept this
session (Content-Type/Content-Length on GetBucketCors/GetBucketWebsite/etc.)
carry no other header-bound response members per their own `HttpBindings`
functions.

**Method**: for each of the 45 ops, read the real
`awsRestxml_deserializeOp<Op>.HandleDeserialize` in full (not an
`OpDocument*Output` function name in isolation) to find which struct the
response root itself decodes into, then compared field-for-field against
gopherstack's handler/model. Grouped by shape family rather than op-by-op
where a shared pattern applied:

- **Raw-passthrough config echoes** (CORS, Lifecycle, Notification, Website,
  Encryption, Logging, Replication, OwnershipControls, PublicAccessBlock,
  the four Analytics/IntelligentTiering/Inventory/Metrics singular Get ops,
  RequestPayment, Accelerate, PolicyStatus, Abac): for each, confirmed the
  real GET deserializer decodes the response ROOT element directly as the
  same struct the PUT/Create request's payload root already is (traced
  individually, not assumed from the pattern) -- so gopherstack's
  echo-the-stored-PUT-body-verbatim implementation is wire-correct by
  construction for all of these. **One doesn't share this shape** -- see the
  metadata-configuration finding below, found precisely because this
  category assumption was checked per-op instead of extended by pattern.
- **Simple flat-field Get ops** (GetBucketVersioning, GetBucketLocation,
  GetBucketTagging, GetObjectTagging, GetBucketPolicy): field-for-field
  against their own deserializer case lists. Found the GetBucketVersioning
  MFADelete gap here (see below).
- **List*Configurations family**: top-level wrapper keys and the
  double-nesting fix already carry a citing regression test from the
  2026-07-24 (phase 2) pass (structural XML walk, not substring) --
  re-verified still correct via the same real-deserializer read, not
  re-tested.
- **ListObjects/ListObjectsV2/ListObjectVersions/ListMultipartUploads/
  ListParts**: `Object`/`ObjectVersion`/`Part`/`MultipartUpload` shapes were
  mechanically diffed against the real SDK types under gopherstack-3dqa
  (2026-08-14b) -- re-verified via the same deserializer case lists that
  StorageClass/Owner/Initiator/checksum fixes already landed correctly. This
  pass's own read of `awsRestxml_deserializeDocumentObject` found the one gap
  that mechanical diff missed: `Owner` (see below).
- **Object-lock family** (GetObjectRetention, GetObjectLegalHold,
  GetPublicAccessBlock, GetObjectLockConfiguration): field names
  (Mode/RetainUntilDate/Status) and root elements (Retention/LegalHold)
  checked directly against `awsRestxml_deserializeDocumentObjectLockRetention`/
  `ObjectLockLegalHold` -- correct.
- **GetObjectAttributes, GetObjectAcl, GetBucketAcl, GetObjectTorrent**:
  spot-checked; GetObjectTorrent correctly returns `NotImplemented` matching
  real AWS's 2022 deprecation of the op. GetObjectAttributes never emits the
  real, optional `ObjectParts` member -- newly disclosed in `gaps`, not
  fixed (this backend has no per-part breakdown for a completed multipart
  object).
- **Object Annotations family** (PutObjectAnnotation, GetObjectAnnotation,
  DeleteObjectAnnotation, ListObjectAnnotations): implemented one session ago
  (gopherstack-zi7k) with detailed per-field citing comments already in
  `object_ops_annotations.go` against the exact deserializer case lists;
  re-read those citations against the pinned SDK directly rather than
  re-deriving, found no discrepancy.

**Sibling/near-duplicate shapes checked** (this issue's first lead
question): `ListObjects` (V1) vs `ListObjectsV2` is the clearest pair in this
service -- V1 has no `FetchOwner` concept at all (Owner always present); V2
gates it on the request. Both were broken the same way (Owner missing
entirely) before this pass, not a case of "one got it right" -- a shared-bug
variant of the sibling-trap pattern, not a copy-paste-only-one-fixed one.
`GetAdministratorAccount`/`GetMasterAccount`-style Invitation mixups (the
shape securityhub and macie2 both hit this campaign) do not exist in s3 --
no analogous shared-name-field type pair was found.

**Values the backend already held that never reached the wire** (this
issue's second lead question): `Object.Owner` on ListObjects/ListObjectsV2 --
`gopherstackName` is the same constant already emitted correctly by
ListBuckets, GetBucketAcl, ListObjectVersions, and ListMultipartUploads, just
never wired into the one shared `mapObjectsToXML` converter both List ops use.
`GetBucketVersioning`'s `MFADelete` was NOT an already-held value (the
backend tracked nothing for it before this pass); fixed by adding the
storage slot AND the wire threading in the same change, since a value with
nowhere to live is not fixable by rewiring alone.

**2 real bugs found and fixed:**

1. **`ListObjects`/`ListObjectsV2` — `Object.Owner` never emitted, backend
   value never wired.** Confirmed real via
   `awsRestxml_deserializeDocumentObject`'s `case strings.EqualFold("Owner",
   ...)` (deserializers.go), shared by both ops' `Contents` items.
   `ObjectXML` (model.go) had no `Owner` field at all -- a real client's
   typed `Contents[i].Owner` was always nil regardless of backend state, for
   both ops, unconditionally. `ListObjectsInput` has no `FetchOwner` member
   (confirmed absent from `api_op_ListObjects.go`) so V1 must always include
   it; `ListObjectsV2Input.FetchOwner` (already read into the backend input,
   `handler_list_v2.go:68`, but never wired to anything) gates it for V2.
   Fixed by adding `ObjectXML.Owner *Owner` and an `includeOwner bool`
   parameter threaded through the shared `mapObjectsToXML`
   (`bucket_ops_listing.go`), `true` unconditionally for V1's call site,
   `q.Get("fetch-owner") == "true"` for V2's.
2. **`GetBucketVersioning`/`PutBucketVersioning` — `MFADelete` read nowhere,
   stored nowhere, echoed nowhere.** Confirmed real and required-sibling-to-Status
   via `awsRestxml_deserializeOpDocumentGetBucketVersioningOutput`'s
   `case strings.EqualFold("MfaDelete", ...)` sitting directly beside the
   already-correct `Status` case. Real request-side type
   (`types.VersioningConfiguration.MFADelete`) is `types.MFADelete`; real
   response-side type (`GetBucketVersioningOutput.MFADelete`) is the
   **different** Go type `types.MFADeleteStatus` (same string values, two
   distinct SDK enums) -- stored as a plain string on `StoredBucket` to avoid
   coupling the backend model to either. `VersioningConfiguration` (model.go)
   gained a `MfaDelete string` field with `omitempty`, matching the real
   doc's "only returned if the bucket has been configured with MFA delete."

**1 severe finding, flagged and NOT fixed** (structural, would require new
backend modeling, not a rename) -- see the ops-table row and gaps entry
above for the full writeup: **`GetBucketMetadataConfiguration`/
`GetBucketMetadataTableConfiguration` return the wrong response shape
entirely.** Unlike every other config-echo op in this file, these two real
GET deserializers require a `MetadataConfigurationResult`/
`MetadataTableConfigurationResult` child element containing
server-*computed* fields (table bucket ARN/namespace/provisioning status)
structurally absent from the client's CREATE request body that gopherstack
currently echoes back verbatim. A real typed client's response fields
decode to nil/zero regardless of backend state today, for both ops. Not
fixed because a correct fix requires modeling S3 Tables table-bucket
provisioning end to end, which does not exist anywhere in this backend;
fabricating ARNs/status would be invented data, the exact failure mode this
campaign exists to catch elsewhere.

**Wrong-value check**: none found beyond the two fixes above (both are
missing-field bugs, not same-key-wrong-value bugs).

**Casing near-misses**: none. REST-XML decodes case-insensitively
(`strings.EqualFold` throughout `deserializers.go`'s body-field switches,
confirmed, not assumed, per this issue's own standing s3 threat-model note)
-- every finding this pass was a genuinely different/absent element name,
never a casing-only difference.

**Ratifying tests**: none found needing correction. Neither `Owner` nor
`MFADelete` had any prior assertion in either direction in this service's
existing test suite (`bucket_listing_test.go`, `store_listing_test.go`,
`bucket_versioning_test.go`) -- both gaps had zero prior coverage, not a
wrong assertion staying green.

**Phantom ops**: none. Cross-referenced all 115 op-name string literals from
`s3CoreOperations()`/`s3ExtendedOperations()` (`handler_operations.go`)
against `api_op_*.go` files in s3@v1.106.5; every one exists as a real op.

**False-positive rate**: 0 among reported bugs -- every finding cites the
real `awsRestxml_deserializeOp<Op>.HandleDeserialize`/
`awsRestxml_deserializeDocument<Type>` function actually reached, file+line
where cited, never a doc comment, an `OpDocument*` function name in
isolation, or an assumption extended from a sibling op's shape.

**Tests**: 2 real-`aws-sdk-go-v2`-client tests added in the new
`services/s3/wire_field_fixes_test.go`
(`TestListObjects_OwnerPopulated` -- table-driven across V1-always/
V2-default-omits/V2-fetch-owner-true; `TestBucketVersioning_MfaDeleteEcho`).
Every fix hand-reverted individually (no git, per this session's hard
no-git-mutation constraint): the `ObjectXML.Owner` struct field removal was
a compile error (`unknown field Owner in struct literal`), proving it
load-bearing at the type level; the V1/V2 `includeOwner` wiring and both
halves of the `MFADelete` request/response threading each independently
reverted to the exact predicted runtime failure (`Contents[].Owner` nil
where expected non-nil; `MFADelete` empty where `"Enabled"` expected,
quoted from the actual test failure output above) -- then restored and
diffed byte-identical against the pre-revert file before moving to the next.

**Untestable-but-fixed**: none this pass -- both fixes are directly
observable through a real SDK client round-trip, unlike some other
services' backend-tracks-nothing-yet gaps.

Gates: `go build`/`go vet`/`go test -race` (scoped to `services/s3`), `go fix
-diff` (no diff), `fieldalignment` (0 findings), `golangci-lint run` (1
finding -- a `goimports` formatting nit on the new `StoredBucket.MFADelete`
field's comment, fixed with `gofmt -w`; 0 issues after, no
cyclop/gocyclo/gocognit/funlen nolints added) all green. `go test -race
./pkgs/...` green.

Per this session's hard constraints: no subagents used (Read/Grep/Bash
only), no git-mutating commands run (all changes uncommitted -- orchestrator
must commit/push), `cmd/routecollisions/`/`services/_ROUTE_COLLISIONS.md`/
`services/apigateway/handler.go`/`services/appconfigdata/`/
`services/inspector2/`/`test/integration/tag_routing_test.go`/
`test/integration/apigateway_quicksight_account_test.go` (the live sibling
RouteMatcher sweep's in-progress work) confirmed untouched via `git status`
both before starting and again at the end, no `gendocs`/`make docs` run.

s3's List/Describe/Get families are now fully swept for this issue (45/45
ops verified against the real deserializer, one op family's finding flagged
rather than fixed). `services/_WRAPPER_KEY_SWEEP_REMAINDER.md` updated: 65
of 162 services swept, 97 remain.

## 2026-08-23: ListBuckets pagination bug

The handler never parsed `continuation-token`/`max-buckets`/`prefix` off the
request, and the backend's `ListBuckets` documented its own lack of
pagination ("this backend implements no pagination/filtering") — both real
`ListBucketsInput` members (httpQuery-bound,
`awsRestxml_serializeOpHttpBindingsListBucketsInput`), discovered while
auditing the pagination bug class found in medialive. Fixed: the handler now
parses all three query params; the backend applies `Prefix` filtering and
`pkgs/page`-style `ContinuationToken`/`MaxBuckets` pagination (default page
size 10,000, matching the real API's documented default), echoing
`ContinuationToken` in the XML response when truncated. No exported
signature change (`ListBuckets(ctx, *s3.ListBucketsInput)` already matched
the real SDK shape). Proven with `TestListBuckets_SDKRoundTrip_Pagination`
(`handler_list_buckets_pagination_test.go`), which drives the real SDK
client across two 10-item pages of 25 seeded buckets and asserts the pages
are disjoint; fails against the unfixed handler (`should have 10 item(s),
but has 25`), hand-reverted and confirmed.

## 2026-08-23: X-Amz-Bypass-Governance-Retention never read (DeleteObject/DeleteObjects)

Header sweep of the object-lock/retention family (s3@v1.106.5 serializers.go
`awsRestxml_serializeOpHttpBindingsDeleteObjectInput`,
`...DeleteObjectsInput`, `...PutObjectRetentionInput`), triggered by a grep
for `BypassGovernance` across the package returning zero hits anywhere in
`services/s3`. `checkObjectLockForDelete` (`objects_delete.go`) already
tracked and enforced `StoredObjectVersion.RetentionMode` (GOVERNANCE vs
COMPLIANCE, set by `PutObjectRetention`) but blocked deletion of *any*
retained object unconditionally — real AWS lets a caller delete a
GOVERNANCE-mode-retained object by sending
`X-Amz-Bypass-Governance-Retention: true` (with `s3:BypassGovernanceRetention`
permission, not separately modeled here); COMPLIANCE mode can never be
bypassed by anyone. The header is a real, always-present member on
`DeleteObjectInput`/`DeleteObjectsInput` (and `PutObjectRetentionInput`, not
touched this pass) but was discarded end to end: never read off the request,
never threaded into the backend, never checked against `RetentionMode`.

Fixed: `deleteObject`/`deleteObjects` (`object_ops_delete.go`) now parse
`X-Amz-Bypass-Governance-Retention` via a new `bypassGovernanceRetentionHeader`
helper and set it on the constructed `DeleteObjectInput`/`DeleteObjectsInput`.
`checkObjectLockForDelete` (`objects_delete.go`) gained a `bypassGovernance
bool` parameter, threaded through `deleteObjectLocked`/`deleteSingleObject`:
a GOVERNANCE-mode retention is now bypassable, a COMPLIANCE-mode retention
and a legal hold are not — matching real AWS's documented rule that the
bypass header only ever applies to GOVERNANCE-mode retention, never
COMPLIANCE and never a legal hold.

`TestDeleteObject_BypassGovernanceRetention` (`wire_field_fixes_test.go`)
drives the real `aws-sdk-go-v2` S3 client through
`CreateBucket(ObjectLockEnabledForBucket)` → `PutObject` →
`PutObjectRetention` → `DeleteObject`, table-driven across
governance-without-bypass (blocked), governance-with-bypass (allowed), and
compliance-with-bypass (still blocked). Hand-reverted just the bypass branch
in `checkObjectLockForDelete` back to an unconditional block: the
governance-with-bypass subtest failed exactly as predicted (`InvalidObjectState`
409 where AWS would allow the bypassed delete); restored and confirmed
byte-identical via `md5sum`.

**Not fixed this pass, flagged as a related but separate gap**:
`PutObjectRetention` itself has zero enforcement — it unconditionally
overwrites `RetentionMode`/`RetainUntil` regardless of the object's existing
retention state, so a caller can shorten or remove even a COMPLIANCE-mode
retention today (real AWS forbids this unconditionally, and forbids
shortening/removing a GOVERNANCE retention without the same bypass header).
Implementing this correctly needs old-vs-new retention comparison logic that
doesn't exist yet anywhere in this file; deferred rather than rushed, per
this campaign's standing "don't ship a rushed partial feature" rule.

Gates: `go build ./...`, `go vet ./services/s3/...`, `go test -race -count=1
./services/s3/...`, `go fix -diff ./services/s3/...` (no diff), `gofmt -l
services/s3/` (no output), `golangci-lint run ./services/s3/...` (0 issues,
required a field-reorder on the new test's table struct to clear
`fieldalignment`, no `//nolint` added), `go test ./pkgs/persistence/...`
(no persisted struct changed — `RetentionMode`/`RetainUntil` were already
persisted fields — run anyway per this campaign's standing rule) all clean.

## 2026-08-23: PutObjectRetention enforced nothing (COMPLIANCE could be shortened/removed)

Follow-up to the entry immediately above, which flagged but deliberately
deferred this: `PutObjectRetention` (`object_lock.go`) unconditionally
overwrote `StoredObjectVersion.RetentionMode`/`RetainUntil` with whatever the
caller sent, with no comparison against the object's existing retention
state. `checkObjectLockForDelete`/`checkObjectLockForOverwrite`
(`objects_delete.go`) correctly *read* `RetentionMode` to block deletes and
overwrites, but nothing ever protected that state from being weakened in the
first place — the strongest guarantee Object Lock exists to provide.

Verified rules (docs.aws.amazon.com/AmazonS3/latest/userguide/
object-lock-overview.html "Retention modes" and
object-lock-managing.html "Bypassing governance mode" — these confirm the
task brief's assumptions, no correction needed):

- **COMPLIANCE**: retain-until date may only be **extended**; the mode can
  **never** be changed once set, for any principal — no bypass exists ("When
  an object is locked in compliance mode, its retention mode can't be
  changed, and its retention period can't be shortened").
- **GOVERNANCE**: retain-until date may be shortened or removed, or the mode
  upgraded to COMPLIANCE, only with `x-amz-bypass-governance-retention: true`
  (plus `s3:BypassGovernanceRetention` on real AWS — gopherstack has no IAM
  evaluator, so the header alone is the gate, matching the existing
  `checkObjectLockForDelete` precedent).
- Real S3's own error text for a rejected shorten (confirmed via AWS
  documentation search, since s3@v1.106.5's
  `awsRestxml_deserializeOpErrorPutObjectRetention` has no op-specific
  error cases — it's a generic `smithy.GenericAPIError{Code, Message}` off
  whatever the server sent): `AccessDenied` / HTTP 403, message "proposed
  retain-until date shortens an existing retention period and governance
  bypass check failed". Used verbatim for the shorten/remove case; the
  mode-downgrade message ("The retention mode may not be changed once an
  object is locked in COMPLIANCE mode.") is this agent's own phrasing of the
  documented rule, not an independently confirmed literal AWS string — the
  behavior (AccessDenied/403) is confirmed, the exact wording is not.
- One point **not** fully confirmed either way: whether upgrading
  GOVERNANCE→COMPLIANCE itself additionally requires the bypass header even
  when the date isn't shortened. AWS's docs are ambiguous (one passage says
  extending only needs `s3:PutObjectRetention`; another says altering
  GOVERNANCE lock settings "unless [caller has] special permissions" is
  broader). Implemented as **not** requiring bypass for a pure mode upgrade
  with a non-shortened date, matching the "use governance mode to test
  before creating a compliance-mode retention period" workflow AWS's own
  docs describe. Flagging this as the one place a stricter interpretation is
  plausible.
- `PutObjectLockConfiguration` has **no** equivalent restriction tied to
  existing objects (confirmed via API_PutObjectLockConfiguration.html): it
  only sets the bucket's *default* retention applied to *future* objects,
  never touches objects that already have retention. gopherstack's existing
  `ErrObjectLockNotEnabled` gate (bucket must have Object Lock enabled) was
  already correct and untouched.

Fixed: `PutObjectRetention` (`object_lock.go`) now calls a new
`validateRetentionChange(oldMode, oldUntil, newMode, newUntil,
bypassGovernance) error` before applying the new state, implementing the
ratchet above. Two new sentinel errors, `ErrRetentionPeriodShortened` and
`ErrRetentionModeDowngrade` (`errors.go`), both map to `AccessDenied`/403
via the existing `errorTable()` mechanism. `PutObjectRetention`'s interface
and backend signature (`interfaces.go`, `object_lock.go`) gained a trailing
`bypassGovernance bool` parameter — `putObjectRetention`
(`object_ops_retention.go`) now passes
`aws.ToBool(bypassGovernanceRetentionHeader(r))`, reusing the header parser
`DeleteObject`/`DeleteObjects` already added. `make build-check` confirmed
no call sites outside `services/s3/`; the four in-package test call sites
(`object_lock_test.go` x2, `janitor_lifecycle_test.go` x2) were updated to
pass `false` — none of them mutate a pre-existing retention, so the new
parameter is inert there.

**Family check**: `PutObjectLegalHold` was already correct as-is — real S3
lets any caller with `s3:PutObjectLegalHold` freely set/clear a legal hold
regardless of Object Lock mode (independent of retention), and gopherstack's
existing unconditional `ver.LegalHold = status == "ON"` already matches
that; no bug there. `GetObjectRetention`/`GetObjectLegalHold` are read-only,
unaffected.

Proof: `TestPutObjectRetention_Ratchet`
(`object_retention_ratchet_test.go`), table-driven across 8 real-SDK-client
subtests (compliance extend/shorten/shorten-with-bypass-still-denied/
downgrade-to-governance, governance extend/shorten-denied/
shorten-with-bypass/upgrade-to-compliance), each asserting either success or
`AccessDenied`/403 via `smithy.APIError`/`*smithyhttp.ResponseError`.
Hand-reverted by replacing the `validateRetentionChange` call in
`object_lock.go` with a no-op (`_ = bypassGovernance`) to reproduce the
named bug exactly: the 4 rejection subtests failed with "An error is
expected but got nil" (`compliance_shorten_denied`,
`compliance_shorten_denied_even_with_bypass`,
`compliance_downgrade_to_governance_denied`,
`governance_shorten_denied_without_bypass`); the 3 already-passing
"allowed" subtests stayed green, confirming the test isolates exactly the
enforcement gap. Restored via `cp` from a scratchpad copy and confirmed
`md5sum`-identical to the fixed version before restoring.

**Not enforced, for lack of state**: none — the object version already
tracks everything the ratchet needs (`RetentionMode`, `RetainUntil`). The
one real gap this pass did *not* touch: gopherstack's XML decode step in
`putObjectRetention` (`object_ops_retention.go`) rejects any request whose
`RetainUntilDate` fails to parse, including an empty/omitted one — so real
S3's documented "remove retention by sending `PutObjectRetention` with
empty parameters" path 400s here before ever reaching the new ratchet
check. That's a pre-existing, orthogonal request-parsing gap (not an
old-vs-new comparison issue), out of this task's scope; the practical
exploit this task targets — shortening via an earlier date, which does not
require an empty body — is fully covered.

Gates: `go build ./...`, `go vet ./services/s3/...`, `go test -race -count=1
./services/s3/...` (all pass, including the new suite), `golangci-lint run
./services/s3/...` (0 issues after `goimports -w errors.go` re-aligned the
var block and swapping a raw `errors.As`+`require.True` for
`require.ErrorAs` per testifylint), `go test ./pkgs/persistence/...` (no
persisted struct changed — pass anyway per standing rule), `make
build-check` (0 external call sites), banned-nolint grep (0 hits, unchanged).

## 2026-08-29: error-path sweep (failure-side wire shape) -- no live bugs found

Hunt for the class distinct from the wrapper-key/nesting sweeps above: HTTP
status, AWS error code, and whether a given operation's own
`awsRestxml_deserializeOpError<Op>` (s3@v1.106.5, `deserializers.go`) models
that code -- most S3 ops declare *zero* typed exceptions (fall through to
`smithy.GenericAPIError` with whatever `Code`/status the server actually
sent), so the load-bearing check for this service is mostly "is the exact
code string and HTTP status correct", not "is a specific Go type produced".

**Error path**: single centralized `errorTable()`/`WriteError()`
(`errors.go`) mapping typed Go sentinels to `{code, message, status}` via
`errors.Is`, used uniformly by every handler -- same shared-helper shape as
sts/iam. Spot-checked the handful of ops that *do* declare typed exceptions:
`GetObject` (`NoSuchKey`, `InvalidObjectState`), `CreateBucket`
(`BucketAlreadyExists`/`BucketAlreadyOwnedByYou`), `AbortMultipartUpload`
(`NoSuchUpload`), `CopyObject` (`ObjectNotInActiveTierError`) -- all confirmed
correct code+404/409/404/403-class status in `errorTable()`.

**HeadObject/HeadBucket confirmed not a bug**: both ops' own deserializers
pass `UseStatusCode: true` to `s3shared.GetErrorResponseComponents`, which
only synthesizes a code from the HTTP status when the body carries no
`Code`/`Message` at all -- irrelevant here since Go's own `net/http` server
already suppresses the response body on `HEAD` requests
(`net/http/server.go`'s `chunkWriter`, `req.Method == "HEAD"` check), so
gopherstack's XML error body is never actually sent for these two ops
regardless of what `WriteError` writes; only the status code (already 404
for both `NoSuchBucket`/`NoSuchKey`) reaches the client. No gopherstack-side
special case is needed or missing.

**Structural gap disclosed, not fixed**: `CopyObject` never checks a source
object's storage class before copying -- real S3 raises
`ObjectNotInActiveTierError` (403, modeled on this op) when the source is in
GLACIER/DEEP_ARCHIVE and hasn't been restored. This emulator has no
archival-tier/restore-state model at all (`StorageClass` is stored as a
label with no enforcement), so implementing this one error code alone would
mean building the entire Glacier restore state machine as a side effect --
out of scope for an error-code-shape pass; genuinely unimplementable without
that larger feature.

No live bugs found this pass; no code changes made to this service.

Gates: `go build`, `go vet ./services/s3/...`, `go test -race -count=1
./services/s3/...` (pass, unchanged), `golangci-lint run ./services/s3/...`
(0 issues, unchanged).

## 2026-08-29 constraint-parameter sweep (filters/pagination never applied) -- 1 operation fixed

s3 is REST-XML with bucket/key routing; per the campaign brief this service's "filters" are
prefix/delimiter/marker/max-keys rather than a `Filter` object, so the audit unit was each List op's
own Input struct in the pinned SDK (`s3@v1.106.5`), read directly rather than assumed from a sibling:
`ListObjects`, `ListObjectsV2`, `ListObjectVersions`, `ListMultipartUploads`, `ListParts`, `ListBuckets`.

**Confirmed already correct** (read every constraint field's handler + backend code path, not just
grepped for its name): `ListObjects`/`ListObjectsV2` (`prefix`/`delimiter`/`marker`/`max-keys`/
`encoding-type`, V2's `continuation-token`/`start-after`), `ListObjectVersions` (`prefix`/`delimiter`/
`key-marker`/`version-id-marker`/`max-keys`, correctly combining key-marker+version-id-marker for the
seek per the documented semantics), `ListMultipartUploads` (`prefix`/`delimiter`/`key-marker`/
`upload-id-marker`/`max-uploads`), `ListParts` (`part-number-marker`/`max-parts`). All apply their
documented constraints and truncate/paginate correctly (verified in `listing.go`/`multipart.go`/
`bucket_ops_listing.go`/`multipart_ops.go`, not inferred).

- **`ListBuckets`** (`bucket_ops.go`/`buckets.go`): `BucketRegion` (`api_op_ListBuckets.go`: "Limits the
  response to buckets that are located in the specified Amazon Web Services Region", query-bound as
  `bucket-region` per `awsRestxml_serializeOpHttpBindingsListBucketsInput`) was never read by the HTTP
  handler at all, and the backend's `ListBuckets` never filtered on it even had it been set -- every
  call returned buckets from every region. `Prefix`/`MaxBuckets`/`ContinuationToken` were already
  correctly applied (including the documented 10,000 default page size). Fixed: `bucket-region` is now
  read in `listBuckets` (`bucket_ops.go`) and applied against each `StoredBucket.Region` in
  `InMemoryBackend.ListBuckets` (`buckets.go`).

**Restraint, not pursued**: `ListBucketAnalyticsConfigurations`/`ListBucketInventoryConfigurations`/
`ListBucketMetricsConfigurations` also carry a `ContinuationToken`-only pagination parameter, but each
bucket's configuration count is realistically small (these are admin-configured, not per-object) and
AWS itself caps them at low three-digit counts -- an unbounded page here is not the observable bug this
class targets. Left unaudited in depth this pass.

Gates: `go build ./services/s3/...`, `go vet ./...` (repo-wide; no signature changes, so no other
package needed updating), `go test ./services/s3/... -race -count=1` (pass), `golangci-lint run
./services/s3/...` (0 issues). New test in `list_filter_params_test.go` drives the real typed SDK
client (`sdk_s3.Client`, path-style) via the existing `newRealS3ClientTest` helper.

## 2026-08-30 pagination arithmetic sweep

s3 is structurally different from the other services audited in this sweep:
every real listing here is prefix/delimiter/marker-based (no equality-matched
opaque cursor), so Classes B and C (miss-defaults-to-zero infinite loop) do
not apply to any site found. Census: `ListObjects`/`ListObjectsV2`
(`listing.go`), `ListObjectVersions` (`listing.go`), `ListMultipartUploads`
(`multipart.go`), `ListParts` (`multipart.go`), `ListBuckets` (`buckets.go`,
via `pkgs/page`), `ListObjectAnnotations` (`annotations.go`, a
gopherstack-only API, not real AWS), and `GetObjectAttributes`'s embedded
Parts list (`objects.go`). No inline `for i, x := range all { if x.ID ==
token { start = i } }` site exists — every marker seek in this service is
either a `sort.Search` threshold search or a linear scan with a `>`
comparison, both safe-by-construction against a stale marker.

**Found and fixed 3 real bugs, all in the delimiter-truncation path — a bug
shape not on the A–E list.** `ListObjects`, `ListObjectVersions`, and
`ListMultipartUploads` each computed `Contents`/`Versions`/`Uploads` and
`CommonPrefixes` as **two independently truncated lists** instead of one
list cut in true lexicographic order:

- `ListObjects` (`truncateVersionResults`) filled the page from
  non-grouped keys first and only padded with CommonPrefixes if room
  remained. A CommonPrefix whose flat neighbors on both sides fit within
  MaxKeys got skipped entirely, and because the resulting NextMarker landed
  past the CommonPrefix's own key range, every later page's `key > marker`
  seek excluded it too — the CommonPrefix was **dropped from the listing
  permanently**, not merely reordered. Reproduced with `{a, b/x, c}` at
  MaxKeys=2 (page 1 = `{a, c}`, `b/` never returned) and a 10-key/3-group
  case where all 3 groups vanished.
- `ListObjectVersions` had the same shape but worse: `CommonPrefixes` were
  **never truncated or counted toward MaxKeys at all** (`buildVersionPage`'s
  `count` loop only ran over the non-grouped snapshot list), so a response
  could silently exceed MaxKeys, and `NextKeyMarker` — derived purely from
  the truncated non-grouped list — again ignored where a CommonPrefix fell
  in true order.
- `ListMultipartUploads` had the identical CommonPrefix-truncation gap
  (`groupUploadsByDelimiter`'s CommonPrefixes were never passed to
  `truncateUploads` at all) **plus a separate, independent bug covered
  below.**

Fixed all three the same way: built one ordered sequence of
tagged entries (`listObjectEntry` / `versionListEntry` / `uploadListEntry`
— an object-or-delete-marker-or-CommonPrefix union, in the same sorted
order the raw key list was already in), cut that single sequence at
MaxKeys/MaxUploads, and derived NextMarker from the last entry actually
included, whichever kind it was. Also fixed the marker-seek side to match:
a NextMarker that is itself a CommonPrefix (recognizable because every
CommonPrefix this package emits ends with `delimiter`, by construction) now
excludes every key sharing that prefix (`key > marker && !HasPrefix(key,
marker)`), not just keys greater than the bare prefix string — without this,
a plain `key > marker` resumes *inside* the very subtree the prior page
already summarized and the client sees the same CommonPrefix duplicated on
the next page (confirmed as the failure mode once the truncation-order fix
alone was applied and tested).

**Fourth bug, `ListMultipartUploads`, Class D — textbook shape, no delimiter
needed.** `truncateUploads` derived `NextKeyMarker`/`NextUploadIdMarker`
from `uploads[maxUploads]` — the first upload **not** returned on the page,
i.e. the token names the first item of the next page — while
`seekMultipartMarker`'s decoder resumes strictly after the item matching
the marker. Naming the next page's first item and then skipping past
whatever matches it drops that exact upload on *every* truncation boundary.
Reproduced with 23 uploads at MaxUploads=5: `upload-05`, `upload-11`,
`upload-17` (indices 5, 11, 17 — exactly the marker-named item at each
boundary) silently vanished on a plain walk, no delimiter, deletion, or
tampering involved. Fixed by switching the encoder to name the *last
included* item (matching `ListParts`' and the fixed `ListObjects`'
convention), folded into the same combined-entries rewrite above.

**Safe-by-construction patterns confirmed already correct, no bug:**
`ListParts` (threshold search `partNumbers[i] > partNumberMarker`,
NextMarker = last item on page); `GetObjectAttributes`'s parts list
(`objects.go`, same shape); `ListBuckets` (sorts by Name, then delegates to
`pkgs/page.New` — out of this pass's scope to re-audit `pkgs/`);
`ListObjectAnnotations` (a gopherstack-only API: sorted names, threshold
search, NextContinuationToken = last name on page — clean end to end).

Seven checks run via real boundary walks through the exported SDK-shaped
backend methods (not synthetic unit tests of an isolated helper): boundary
walk with a non-dividing page size (10 keys / 3 groups at MaxKeys=2),
delimiter interleaving of flat keys and groups, cursor round trip, and the
Class-D drop reproduction above. All failed against the pre-fix code first
(shown in the diffs above) and pass post-fix. Full existing `services/s3`
suite (multipart list/round-trip tests, `ListObjects`/`ListObjectVersions`
unit and HTTP-driven tests) re-run with `-race` and shows no regression.

New tests: `services/s3/pagination_arithmetic_test.go`.

**Left unaudited, unchanged from the 2026-08-15 note above:**
`ListBucketAnalyticsConfigurations`/`ListBucketInventoryConfigurations`/
`ListBucketMetricsConfigurations`'s `ContinuationToken`-only pagination —
still not pursued this pass either, same restraint reasoning (small,
admin-configured collections; AWS itself caps them low). `pkgs/page`
(backing `ListBuckets`) is outside this pass's scope (`pkgs/` is off limits)
and was not independently re-verified.

Gates: `go build ./services/s3/...` (clean), `go vet ./services/s3/...`
(clean, no exported signature changed — `truncateVersionResults`,
`applyDelimiterToVersions`, `seekVersionMarker`, `applyVersionDelimiter`,
`buildVersionPage`, `seekMultipartMarker`, `groupUploadsByDelimiter`,
`truncateUploads` are all unexported), `go test -race -count=1
./services/s3/...` (pass, full package). Work left uncommitted per this
pass's instructions.

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD.
`cmd/reqfielddiff`: 738-739 findings across the 5 old runs, 740 at HEAD
(this service trips the tool's own coverage guard in every run, old and
new alike -- "only 31/109 (28%) of resolved handlers show ANY declared
field at all", pre-existing and unrelated to this defect: most s3 handlers
read `url.Values`/headers directly with no intervening named struct for
the scan to see, the same structural gap gopherstack-id70's parent issue
already named as why "s3 did not improve" from the query-form-shape
recognizer).

One key moved in the direction worth naming: `PutBucketAcl.ACL` is flagged
at HEAD but in NO old run. Traced it: `PutBucketAcl`/`PutBucketACL` and
`PutObjectAcl`/`PutObjectACL` both collide between the real handler
(`bucket_ops_acl_policy.go:29`, `object_ops_acl.go:70`) and a same-named
exported `*InMemoryBackend` method (`acl_policy_store.go:8,267`). Under
misresolution, the old tool sometimes landed on the backend method, whose
body happens to reference an identifier named `acl` (`bucket.ACL = acl`,
`acl_policy_store.go:19`) -- enough to satisfy the tool's naive read
check and suppress the finding, even though that is not where the field
is actually read. The real handler reads the field correctly via
`r.Header.Get("X-Amz-Acl")` (`bucket_ops_acl_policy.go:42`,
`object_ops_acl.go:82`) -- genuinely handled -- but `cmd/reqfielddiff` has
no HTTP-header-read recognizer at all (confirmed: no `Header.Get` handling
anywhere in `cmd/reqfielddiff/main.go`), so once resolution is fixed and
correctly lands on the real handler, it flags ACL as a false positive for
an unrelated reason (the header blind spot, not the collision).

This is exactly the "field reported read because the wrong body happened
to read it" risk gopherstack-id70 asked to watch for, and it did occur
here -- but it did not conceal a bug: the field is genuinely applied by
the real code, just through a mechanism (`r.Header.Get`) this scan cannot
see, independent of and unimproved by the resolution fix. `PutObjectAcl.ACL`
shows the same pattern (flickered within the 5 old runs, 2/5, and is
flagged at HEAD like `PutBucketAcl.ACL`). No bug, no code changed. The
740 pre-existing findings themselves (identical wire-name-collision misses,
`omitempty`-swallowed defaults, etc.) are outside this pass's scope --
`cmd/reqfielddiff`'s own coverage guard already marks that count
unverified for this package independent of the resolution defect.

## 2026-09-08: lock-metrics naming audit (gopherstack-lf8p) -- not a defect, but found a real drift bug nearby

Filed title-only: "every object lock is named s3.object, so lock metrics
have no per-object resolution." Traced the four `lockmetrics.New("s3.object")`
call sites (`objects.go:43` `newStoredObject`, `objects.go:1028`
`saveObjectVersion`, `multipart.go:483` `commitMultipartObject`,
`persistence.go:153` `reinitSingleBucket`) plus every `obj.mu.Lock`/`RLock`
call site (~30, across acl_policy_store.go, annotations.go, objects.go,
objects_delete.go, listing.go, janitor_lifecycle.go, multipart.go).

**Verdict: (b), working as designed -- the title describes intended
behavior, not a bug.** `pkgs/lockmetrics/lockmetrics.go`'s `Collect` doc
comment names this exact case explicitly: "Many [RWMutex] instances can
share the same name by design (e.g. every S3 object lock is named
`s3.object`), so instances are aggregated per label tuple before emitting."
The `name` passed to `lockmetrics.New` identifies a *lock/resource class*
for Prometheus aggregation, not a per-instance identity -- per-operation
attribution already exists via the separate `operation` label every
`Lock`/`RLock` call carries (`"PutObjectACL"`, `"RestoreObject"`,
`"deleteSpecificVersion"`, `"ListObjects"`, etc. -- see the ~30 call sites
above). Giving every object its own lock name (e.g.
`"s3.object." + key`) would mean one Prometheus series per S3 object key --
unbounded cardinality on a resource that can run to millions of keys per
bucket, which Prometheus documents as a cardinality anti-pattern and which
this package's own aggregation logic exists specifically to avoid.
Contrast with buckets, which *do* get per-instance names
(`"s3.bucket." + bucketName`, `buckets.go:58`) -- bucket counts are orders
of magnitude smaller than object counts, so per-bucket resolution is cheap
and buckets get it; per-object resolution would not be, and objects don't
get it. This is a deliberate, cardinality-aware design choice, not an
oversight -- per-object lock names would misrepresent granularity the
locking (and the metrics pipeline) does not, and should not, have.

**Real bug found nearby and fixed**: `reinitSingleBucket`
(`persistence.go:127-153`, run from `Restore` on every snapshot reload)
named the mutexes it re-creates after deserialization `"s3-bucket"` and
`"s3-object"` (hyphens) -- diverging from every live-creation call site
(`buckets.go:58`, `objects.go:43,1028`, `multipart.go:483`), which use
`"s3.bucket." + bucketName` and `"s3.object"` (dots). `git log -S` traces
the drift to `9d7e36e00` ("Go refactoring 2"), which introduced the
dotted/per-bucket naming at the live-creation sites but never touched
`persistence.go`'s reinit path. Effect: a bucket or object rehydrated from
a snapshot (i.e. after any process restart with persistence enabled)
reports lock-contention metrics under a *different* Prometheus series than
one created fresh in the same process -- fragmenting the exact
same-name aggregation the `Collect` doc comment above relies on -- and for
buckets specifically, silently discards the per-bucket name resolution
`buckets.go` deliberately provides (every restored bucket collapsed onto
one shared `"s3-bucket"` label instead of its own `"s3.bucket.<name>"`).
Fixed by changing both `lockmetrics.New` calls in `reinitSingleBucket` to
match the live-creation strings exactly. No locking behavior changed --
same coarse per-bucket/shared-per-object structure as before, only the
label strings now agree.

Regression test: `TestInMemoryBackend_RestoredLockNamesMatchFreshlyCreated`
(`persistence_lockname_test.go`) creates a bucket+object, snapshots,
restores into a fresh backend, touches the restored bucket/object through
the existing `PeekStoredBytes`/`BackdateObjectForTest` test helpers (whose
lock calls carry known operation labels), and asserts via
`prometheus.DefaultGatherer.Gather()` that the resulting
`gopherstack_lock_hold_seconds`/`gopherstack_lock_wait_seconds` samples
land under the dotted names, not the hyphenated ones. Confirmed failing
against unmodified code (both deltas 0 instead of 1); passes after the fix.

Gates: `golangci-lint run ./services/s3/...` (0 issues), `go test -race
-count=1 ./services/s3/...` (pass, repeated 8x clean plus one `-shuffle=on`
run to rule out flake), `go test ./services/...` (full suite, pass --
`persistence.go` is shared infrastructure so blast radius was checked
repo-wide even though the change is s3-local).
