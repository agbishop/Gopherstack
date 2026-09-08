---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: omics
sdk_module: aws-sdk-go-v2/service/omics@v1.49.5
last_audit_commit:                                # unknown: pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-09-04
overall: A            # 2026-08-07 (gopherstack-hnhk): RunBatch's real body shape is now modeled.
                       # StartRunBatch takes real BatchRunSettings (inlineSettings, field-diffed
                       # against awsRestjson1_serializeDocumentBatchRunSettings/InlineSetting) +
                       # DefaultRunSetting (subset -- see the RunBatch family note for what's not
                       # modeled) instead of a flat {workflowId,roleArn,name} shape a real client
                       # never sends, and now actually creates the batch's constituent Run records
                       # synchronously (previously it created zero runs regardless of what a
                       # caller sent). GetBatch now returns real runSummary/submissionSummary/
                       # totalRuns/uuid/submittedTime/processedTime, with runSummary computed live
                       # from surviving Run rows rather than fabricated. ListRunsInBatch's
                       # runSettingId filter is now real (previously accepted-but-ignored). See the
                       # RunBatch family note below for what's still not modeled (s3UriSettings,
                       # most optional DefaultRunSetting fields, RequestId idempotency). This pass
                       # did NOT re-walk the rest of hnhk's stated scope (ListAnnotationStores/
                       # VariantStores/ShareVersions/Shares filters; ReferenceMetadata/
                       # ReadSetMetadata optional sub-objects) -- left open, see gaps below.
                       #
                       # --- prior (2026-07-23) history, kept for context ---
                       # this pass: closed all 3 tracked gaps (jxc5/x7qq/fedo), killed all 6 banned
                       # nolints via a table-based route/dispatch refactor, and field-diffing the
                       # request/response shapes turned up and fixed 4 more real wire bugs: a wrong
                       # JSON key on Run's batch association (runBatchId -> batchId), an invented
                       # field name on ReadSetMetadata/MultipartReadSetUpload (sequenceType, which
                       # appears nowhere in the real API -> fileType/sourceFileType), an invented
                       # "status" field on MultipartReadSetUpload (no such field in the real API,
                       # removed) with two real required fields (sampleId/subjectId) that were
                       # missing entirely, and a wrong JSON key on S3AccessPolicy's policy document
                       # (policy -> s3AccessPolicy, the key real SDK clients actually read).
families:
  ReferenceStore: {status: ok, note: "CRUD + List; pagination now reads maxResults/nextToken from the query string (was body). FIXED 2026-09-04 (gopherstack-42g): DeleteReferenceStore deleted a store unconditionally even when it still contained references, contradicting its own doc comment (\"You can only delete a reference store when it does not contain any reference genomes.\", api_op_DeleteReferenceStore.go). Now rejects with ConflictException (ErrInvalidState) when referencesByStore is non-empty."}
  Reference: {status: ok, note: "Get/List/Delete + GetReferenceBytes/GetReferenceMetadata; pagination fixed same as ReferenceStore. FIXED 2026-09-04 (gopherstack-42g): DeleteReference deleted a reference even when a read set's ReferenceARN pointed to it, contradicting its own doc comment (\"The read set associated with the reference genome must first be deleted before deleting the reference genome.\", api_op_DeleteReference.go). Now rejects with ConflictException when any ReadSet.ReferenceARN matches the reference's ARN."}
  ReferenceImportJob: {status: ok, note: "completes synchronously (Status=COMPLETED at creation) -- no waiter-hang risk since Get never needs to transition; pagination fixed; ListReferenceImportJobs now applies its status body filter (was gap jxc5)"}
  SequenceStore: {status: ok, note: "CRUD + List; created ACTIVE immediately (no CREATING phase in the real API for this resource); pagination fixed. FIXED (gopherstack-5wj0): CreateSequenceStore accepted no eTagAlgorithmFamily/s3AccessConfig fields at all even though the SequenceStore struct already reserved ETagAlgorithm/S3Access fields for them -- both were always zero-valued. eTagAlgorithmFamily now defaults to MD5up (real API default) when omitted and is stored as given otherwise; s3AccessConfig.accessLogLocation is echoed into the response's s3Access object. s3Access.s3Uri/s3AccessPointArn (server-synthesized, no honest source in this in-memory backend) remain absent rather than fabricated. FIXED 2026-09-04 (gopherstack-42g): DeleteSequenceStore deleted a store unconditionally even when it still contained read sets, contradicting its own doc comment (\"You can only delete a sequence store when it does not contain any read sets.\", api_op_DeleteSequenceStore.go). Now rejects with ConflictException when readSetsByStore is non-empty."}
  ReadSet: {status: ok, note: "Get/List/BatchDelete/GetReadSetBytes; pagination fixed; ListReadSets already filtered by name/status. FIXED wire bug: ReadSetMetadata's file-type field was serialized as the invented key \"sequenceType\" (appears nowhere in GetReadSetMetadataOutput/ReadSetListItem) -- renamed to \"fileType\", the real key confirmed against the SDK deserializer. Files/CreationJobId/CreationType/Etag/SequenceInformation sub-objects remain unpopulated (deferred, optional/pointer-safe)"}
  ReadSetActivationJob: {status: ok, note: "completes synchronously; pagination fixed"}
  ReadSetExportJob: {status: ok, note: "completes synchronously; pagination fixed"}
  ReadSetImportJob: {status: ok, note: "completes synchronously; pagination fixed"}
  MultipartReadSetUpload: {status: ok, note: "FIXED (field-diffed against CreateMultipartReadSetUploadInput/Output and MultipartReadSetUploadListItem): the file-type field was serialized as the invented key \"sequenceType\" -- renamed to the real key \"sourceFileType\"; SampleID/SubjectID are real required fields that were missing entirely -- added and threaded through CreateMultipartReadSetUpload's signature; there is no real \"status\" field on this resource at all -- the invented one was deleted. GeneratedFrom/ReferenceARN/Description (real optional fields) also added. 2026-08-14 (gopherstack-7185, mutating-op sweep): CompleteMultipartReadSetUploadOutput's real (and only) member is \"readSetId\" (deserializers.go's awsRestjson1_deserializeOpDocumentCompleteMultipartReadSetUploadOutput) -- a different key from GetReadSetMetadataOutput's \"id\" for the same resource, same split-response class as the AnnotationImportJob/VariantImportJob start-vs-get bugs fixed elsewhere in this file. The handler previously marshaled the full ReadSetMetadata struct (tagged \"id\") as the Complete response, so a real client's ReadSetId was always nil -- the next call in the natural create-then-GetReadSetMetadata chain would silently receive a zero-value ID. Fixed to emit the dedicated {\"readSetId\": ...} shape. FIXED 2026-08-21 (gopherstack-r80d batch 7): CreateMultipartReadSetUploadOutput requires \"referenceArn\" (api_op_CreateMultipartReadSetUpload.go:82-85) despite CreateMultipartReadSetUploadInput.ReferenceArn being optional -- the struct field was tagged \"referenceArn,omitempty\", so an upload created without a reference dropped the key entirely instead of emitting an empty string; a real client's *string decodes nil for a field the wire contract says is always present. Removed omitempty (present-but-empty is correct for a required field with nothing to report, same convention as StatusMessage elsewhere in this file). Also removed omitempty from GeneratedFrom, which MultipartReadSetUploadListItem (the List element, types.go) requires too even though CreateMultipartReadSetUploadOutput itself does not -- same struct backs both Create and List responses, so the fix closes both. Proven via Test_SDKRoundTrip_CreateMultipartReadSetUpload_ReferenceArn (wire_field_additions_test.go); hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
  RunGroup: {status: ok, note: "CRUD + List; already used correct maxResults+startingToken query params; ListRunGroups now applies its name query filter (bonus find alongside gap jxc5, real AWS ListRunGroupsInput has a \"name\" query param the backend previously ignored)"}
  Run: {status: ok, note: "FIXED: GetRun advances PENDING->RUNNING->COMPLETED across polls (waiter-hang fix, prior pass). This pass: (1) ListRuns now applies its name/runGroupId/batchId/status query filters (gap jxc5); (2) the run's batch association was serialized under the invented JSON key \"runBatchId\" -- real GetRunOutput/RunListItem use \"batchId\" (confirmed against the SDK deserializer) -- renamed; (3) added the real (previously entirely absent) RunGroupID field, threaded through StartRun so ListRuns' runGroupId filter has something real to match against; (4) StartRun/GetRun responses now include the optional uuid/networkingMode/runOutputUri/configuration fields real StartRunOutput/GetRunOutput have (gap fedo) -- networkingMode/outputUri are accepted from the request body (real StartRunInput field names, note outputUri on input vs runOutputUri on output). FIXED 2026-09-04 (gopherstack-42g): DeleteRun deleted a run unconditionally regardless of Status, contradicting its own doc comment (\"You can only delete a run that has reached a COMPLETED, FAILED, or CANCELLED stage.\", api_op_DeleteRun.go) -- the exact same precondition class this file's own DeleteRunBatch (DeleteBatch semantics, ErrInvalidState/isRunBatchTerminal) already enforced correctly, just missed on Run itself. Now rejects with ConflictException via a new runDeletableStatuses set, mirroring the existing DeleteRunBatch pattern."}
  RunTask: {status: ok, note: "FIXED: GetRunTask advances PENDING->RUNNING->COMPLETED across polls, same waiter-hang fix as Run. This pass: ListRunTasks now applies its status query filter (gap jxc5)"}
  Workflow: {status: ok, note: "FIXED: GetWorkflow advances CREATING->ACTIVE on first poll (waiter-hang fix, prior pass). This pass: (1) ListWorkflows now applies its name/type query filters (gap jxc5); (2) CreateWorkflow's response now includes the optional uuid field real CreateWorkflowOutput has (gap fedo)"}
  WorkflowVersion: {status: ok, note: "FIXED: GetWorkflowVersion advances CREATING->ACTIVE on first poll (waiter-hang fix, prior pass); pagination already correct. This pass: ListWorkflowVersions now applies its type query filter (gap jxc5)"}
  AnnotationStore: {status: ok, note: "FIXED: GetAnnotationStore advances CREATING->ACTIVE on first poll (real AnnotationStoreCreatedWaiter previously hung forever); pagination fixed to query maxResults+nextToken. ListAnnotationStores' own status/ids filter still not applied (see deferred). 2026-08-13 (gopherstack-lx5h/kb66): the ARN was tagged json:\"arn\" -- real GetAnnotationStoreOutput/AnnotationStoreItem wire key is \"storeArn\" (deserializers.go:6266) -- renamed to StoreArn/storeArn. Added NumVersions (real required \"numVersions\", deserializers.go:6225), computed live from annotationVersionsByStore at Get/List/Update time rather than stored, since a stored counter would drift as versions are added/deleted. Added StoreSizeBytes (real required \"storeSizeBytes\", deserializers.go:6289) -- this backend does not track actual stored bytes, so it is always 0 (modeled honestly, not fabricated) rather than omitted, since the field is required on the real wire. 2026-08-13 (gopherstack-7s8r): added StatusMessage (real required GetAnnotationStoreOutput field, deserializers.go) -- always empty, no error state tracked. 2026-08-14 (gopherstack-dv4s batch five): FIXED the over-share deferred above -- ListAnnotationStores now builds a dedicated AnnotationStoreSummary instead of marshaling AnnotationStore directly, so NumVersions/Tags/StoreOptions (absent from the real List element, AnnotationStoreItem, types.go:152-211) no longer leak. AnnotationStoreItem does declare sseConfig, unlike VariantStoreItem (see VariantStore note) -- verified separately rather than assumed by analogy, and correctly kept in the new summary. 2026-08-20 (gopherstack-dv4s re-verification): re-read types.AnnotationStoreItem independently against omics@v1.49.5 (types.go:152-215, not 152-211 as previously cited -- corrected) and its deserializer (deserializers.go:21641-21783, case list: creationTime/description/id/name/reference/sseConfig/status/statusMessage/storeArn/storeFormat/storeSizeBytes/updateTime, 12 keys) against AnnotationStoreSummary's 12 emitted fields: exact match, no leak, no missing member. Raw-body proof in TestOmicsStoreLists_OmitGetOnlyFields (wire_field_additions_test.go) hand-reverted to confirm it fails against the pre-fix marshal-stores-directly code and passes against this fix, byte-identical restore verified via md5sum. FIXED 2026-08-21 (gopherstack-r80d batch 7): CreateAnnotationStoreOutput requires \"versionName\" (deserializers.go:1290) -- a real, optional CreateAnnotationStoreInput field (api_op_CreateAnnotationStore.go:61-62) the handler didn't read at all, and no field on this struct backed it. CreateAnnotationStoreOutput is also genuinely narrower than GetAnnotationStoreOutput (only creationTime/id/name/status/versionName, api_op_CreateAnnotationStore.go:68-95) -- previously the handler over-shared the full AnnotationStore struct (storeArn/reference/tags/etc, none of which are real CreateAnnotationStoreOutput members) as the Create response. Fixed with a dedicated CreateAnnotationStoreResponse type (models.go) built from the created store plus the echoed request VersionName (present-but-empty when the caller doesn't supply one, never fabricated) -- same technique already used for Start*ImportJob's jobId-only responses. Two pre-existing tests (TestOmics_AnnotationStore/CreateAnnotationStore_returns_201, TestCreateAnnotationStoreStoresReference) asserted the old over-share shape (storeArn/reference on Create's own response) -- updated to check those via a follow-up GetAnnotationStore instead, and a new TestCreateAnnotationStore_ResponseShape locks down the narrower real shape. Proven via Test_SDKRoundTrip_CreateAnnotationStore_VersionName (wire_field_additions_test.go); hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
  AnnotationStoreVersion: {status: ok, note: "created ACTIVE immediately (no waiter-hang risk); pagination fixed. ListAnnotationStoreVersions' own status filter still not applied (see deferred). 2026-08-13 (gopherstack-lx5h/kb66): the ARN was tagged json:\"arn\" -- real GetAnnotationStoreVersionOutput/AnnotationStoreVersionItem wire key is \"versionArn\" (deserializers.go:6564) -- renamed to VersionArn/versionArn. Added VersionSizeBytes (real required \"versionSizeBytes\", deserializers.go:6587) -- always 0, same not-tracked rationale as AnnotationStore.StoreSizeBytes. 2026-08-13 (gopherstack-7s8r): added StatusMessage (real required GetAnnotationStoreVersionOutput field) -- always empty, no error state tracked. 2026-08-14 (gopherstack-dv4s batch five): FIXED an over-share found while auditing this class -- ListAnnotationStoreVersions marshaled this same domain struct, leaking Tags and StoreName; real AnnotationStoreVersionItem (types.go) declares neither. Now builds a dedicated AnnotationStoreVersionSummary. NOT fixed, found in the same pass and out of this pass's scope: StoreName is a phantom field on Get too -- no real GetAnnotationStoreVersionOutput member of that name exists at all, confirmed against its deserializer -- so it is still emitted (wrongly) by the Get response; and the real type also requires an Id and a plain Name distinct from VersionName that this domain struct has never tracked. Both are missing/phantom-field bugs, the opposite class from what this pass targets -- worth a follow-up issue. 2026-08-20 (gopherstack-dv4s re-verification): re-read types.AnnotationStoreVersionItem independently against omics@v1.49.5 (types.go:218-276) and its deserializer (deserializers.go:21818-21939, case list: creationTime/description/id/name/status/statusMessage/storeId/updateTime/versionArn/versionName/versionSizeBytes, 11 keys) against AnnotationStoreVersionSummary's 9 emitted fields: no leak, confirms the tags/storeName omission is correct and independently reconfirms the already-noted Id/plain-Name gap above (2 real required members this domain struct still doesn't track -- unchanged from the prior finding, not fixed here, same rationale). Raw-body proof hand-reverted and restore verified byte-identical via md5sum. FIXED 2026-08-21 (gopherstack-r80d batch 7): closed the Id/plain-Name gap two prior passes flagged and deliberately left open as \"the opposite class\" from their own scope -- it is exactly r80d's \"member with no struct field at all\" class. Get/Update/CreateAnnotationStoreVersionOutput each require \"id\" (a generated version ID, deserializers.go:6501/1478/1465-1505) and \"name\" (the parent store's name, deserializers.go:6510/1487) that this struct never tracked: added ID (json:\"id\", generated via newID() at creation, the same convention every other resource in this file already uses) and retagged StoreName from the invented key \"storeName\" to the real \"name\" -- the value was already correct (the parent store's Name), only the wire key was wrong. Applied to AnnotationStoreVersionSummary (the List element, which requires the identical two members per types.AnnotationStoreVersionItem) too. Proven via Test_SDKRoundTrip_AnnotationStoreVersion_IdAndName (wire_field_additions_test.go), covering Create/Get/Update/List in one round trip; hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
  AnnotationImportJob: {status: ok, note: "completes synchronously; pagination fixed. This pass: ListAnnotationImportJobs now applies its status/storeName body filter and explicit ids list (gap jxc5). 2026-08-13 (gopherstack-lx5h/kb66): StartAnnotationImportJob's response was built by marshaling this same domain struct with its ID field tagged json:\"id\" -- correct for GetAnnotationImportJobOutput/AnnotationImportJobItem (deserializers.go:5954/21500s) but WRONG for StartAnnotationImportJobOutput, whose only member is \"jobId\" (deserializers.go:17434). The two ops don't share a response shape in the real API, so this needed splitting rather than a rename: the start handler now builds its own {\"jobId\": ...} response and leaves the shared struct's \"id\" tag alone. Also added FormatOptions/RunLeftNormalization/VersionName/StatusMessage/UpdateTime/AnnotationFields -- real GetAnnotationImportJobOutput required members (deserializers.go:5949-6015) and real StartAnnotationImportJobInput optional members (serializers.go:7892-7935) that were entirely absent from this struct before -- a schema gap, not a dropped key, on both the request and response sides. FormatOptions is modeled as a passthrough map (same convention as Reference/SseConfig/StoreOptions elsewhere in this service); StatusMessage is always empty (no error state to describe -- this backend completes synchronously). 2026-08-13 (gopherstack-7s8r): fixed the deferred item-level JobStatus gap -- Items is now []AnnotationImportItemDetail (real GetAnnotationImportJobOutput.Items shape, JobStatus+Source, types.go:75-89) instead of reusing the Start-request-only ItemSource shape (Source only, types.go:91-99, still what AnnotationImportItem models and StartAnnotationImportJobInput.Items correctly uses). JobStatus is stamped once from the job's own Status at Start time, since this backend completes synchronously in one step so that is each item's true final state. The originating issue also assumed ListAnnotationImportJobs returns ItemDetail-shaped Items; verified false against the pinned SDK -- the real List element (AnnotationImportJobItem, types.go:102-146) has no items/formatOptions/statusMessage member at all, narrower than Get, so this backend's prior habit of marshaling the Get-shaped struct for List leaked all three. List now builds a dedicated AnnotationImportJobSummary"}
  VariantStore: {status: ok, note: "FIXED: GetVariantStore advances CREATING->ACTIVE on first poll (real VariantStoreCreatedWaiter previously hung forever); pagination fixed. ListVariantStores' own status/ids filter still not applied (see deferred). 2026-08-13 (gopherstack-lx5h/kb66): the ARN was tagged json:\"arn\" -- real GetVariantStoreOutput/VariantStoreItem wire key is \"storeArn\" (deserializers.go:11673) -- renamed to StoreArn/storeArn. Added StoreSizeBytes (real required \"storeSizeBytes\", deserializers.go:11682) -- always 0, not tracked (see AnnotationStore note). VariantStore has no NumVersions concept in the real API (confirmed: GetVariantStoreOutput/VariantStoreItem have no such field) -- correctly not added. 2026-08-13 (gopherstack-7s8r): added StatusMessage (real required GetVariantStoreOutput field) -- always empty, no error state tracked. 2026-08-14 (gopherstack-dv4s batch five): FIXED the deferred over-share -- ListVariantStores now builds a dedicated VariantStoreSummary instead of marshaling VariantStore directly, so Tags (absent from the real List element, VariantStoreItem) no longer leaks. NOT fixed, found in the same pass: VariantStoreItem also declares a required sseConfig member that neither GetVariantStoreOutput nor this domain struct has ever tracked -- CreateVariantStore has no request field for it at all. Missing-member gap on both Get and List, the opposite class from what this pass targets; left absent rather than fabricated, worth a follow-up issue. 2026-08-20 (gopherstack-dv4s re-verification): re-read types.VariantStoreItem independently against omics@v1.49.5 (types.go:2135-2193) and its deserializer (deserializers.go:27828-27950, case list: creationTime/description/id/name/reference/sseConfig/status/statusMessage/storeArn/storeSizeBytes/updateTime, 11 keys) against VariantStoreSummary's 10 emitted fields: no leak beyond tags (already fixed); independently reconfirms the already-noted sseConfig gap above (unchanged, not fixed here). Raw-body proof hand-reverted and restore verified byte-identical via md5sum. FIXED 2026-08-21 (gopherstack-r80d batch 7): closed the sseConfig gap two prior passes flagged and deliberately left open as \"the opposite class\" from their own scope -- it is exactly r80d's \"member with no struct field at all\" class (like iam's JobCompletionDate). Added SseConfig (map[string]any, same passthrough-union convention AnnotationStore already uses) to VariantStore and VariantStoreSummary; CreateVariantStore now reads the real (optional) CreateVariantStoreInput.SseConfig, which the handler previously didn't even declare in its request struct. Description remains a separate, NOT-fixed gap noted here for a future input-sweep pass: CreateVariantStoreInput.Description is also real and optional but the handler still doesn't read it either -- unlike sseConfig this doesn't break the required-output contract (Description is already present-but-empty, satisfying the required field), so it's an input-fidelity bug, not this cut's target class. Proven via Test_SDKRoundTrip_VariantStore_SseConfig (wire_field_additions_test.go), covering Create/Get/List; hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical. Changed the StorageBackend.CreateVariantStore interface signature (added sseConfig map[string]any) -- go build ./..., go vet -tags e2e ./... and go vet -tags integration ./... all re-run repo-wide and clean (excluding the unrelated, already-broken services/ssm concurrent-agent WIP)."}
  VariantImportJob: {status: ok, note: "completes synchronously; pagination fixed. This pass: ListVariantImportJobs now applies its status/storeName body filter and explicit ids list (gap jxc5). 2026-08-13 (gopherstack-lx5h/kb66): same StartVariantImportJobOutput \"jobId\" (deserializers.go:18893) vs GetVariantImportJobOutput \"id\" (deserializers.go:11383) split-response bug as AnnotationImportJob above -- found by reading the whole Start/Get operation pair, not itemized in either originating bd issue, fixed the same way (dedicated {\"jobId\": ...} start response). Added RunLeftNormalization/StatusMessage/UpdateTime/AnnotationFields (real GetVariantImportJobOutput required members, deserializers.go:11406-11444, and StartVariantImportJobInput optional members, serializers.go:8737-8767) -- previously absent entirely. Unlike AnnotationImportJob, variant import jobs have NO FormatOptions or VersionName field anywhere in the real API (confirmed against both StartVariantImportJobInput and GetVariantImportJobOutput) -- correctly not added, verified rather than assumed from the annotation sibling. 2026-08-13 (gopherstack-7s8r): fixed the deferred item-level JobStatus gap, same treatment as AnnotationImportJob -- Items is now []VariantImportItemDetail (JobStatus+Source+optional StatusMessage, types.go:2060-2071); StartVariantImportJobInput.Items keeps using VariantImportItemSource (Source only, types.go:2079-2087) via the unchanged VariantImportItem type. ListVariantImportJobs also over-shared Items/StatusMessage vs the real narrower List element (VariantImportJobItem, types.go:2090-2132) -- same false List==Get premise as AnnotationImportJob, fixed the same way with a dedicated VariantImportJobSummary"}
  Share: {status: fixed, note: "Create/Accept/Delete/Get/List; ACCEPTING/DELETED transient statuses returned synchronously, unchanged this pass; pagination fixed. ListShares' own resourceArns/status/resourceTypes filter still not applied (see deferred). 2026-08-14 (gopherstack-7185, mutating-op sweep): the shared Share model's Name field was tagged json:\"name\" -- real CreateShareOutput and ShareDetails (used by Get/List) both use the wire key \"shareName\" (deserializers.go:3062 and :26670) -- confirmed against the request side of the same handler, which already read the input as \"shareName\". A real client's ShareName was always empty on every op that returns a Share. Fixed by retagging the field. CORRECTED (2026-08-20, gopherstack-dv4s): this note previously argued CreateShareOutput/AcceptShareOutput/DeleteShareOutput being narrower than the full Share struct on the real wire (confirmed: AcceptShareOutput/DeleteShareOutput carry only \"status\"; CreateShareOutput carries only shareId/shareName/status, api_op_AcceptShare.go/api_op_DeleteShare.go/api_op_CreateShare.go) was fine to leak past because unknown keys are silently dropped by real deserializers. The premise is true but the conclusion was wrong, the same false-rationale shape this issue exists to correct: a raw-body or non-SDK caller sees the leak regardless of SDK-client tolerance. handleAcceptShare/handleDeleteShare/handleCreateShare (handler_shares.go) still marshal the full Share struct today -- CONFIRMED, NOT FIXED, same bug class as the three List ops this pass fixed, out of this pass's scope (dv4s scoped fixes to ListAnnotationStores/ListVariantStores/ListAnnotationStoreVersions only) -- worth a follow-up issue. FIXED (2026-08-20, gopherstack-80bz): re-confirmed all four output shapes independently against omics@v1.49.5 -- AcceptShareOutput carries only Status (api_op_AcceptShare.go:39-48), DeleteShareOutput only Status (api_op_DeleteShare.go:41-50), CreateShareOutput ShareId/ShareName/Status (api_op_CreateShare.go:58-73), GetShareOutput the whole *types.ShareDetails under \"share\" (api_op_GetShare.go:39-49, deserializers.go:11157-11191) -- GetShare correctly left unchanged. handleAcceptShare/handleDeleteShare now emit {\"status\": ...} only (wire key confirmed via deserializers.go:290/5340); handleCreateShare now emits {\"shareId\", \"shareName\", \"status\"} only (deserializers.go:3065-3083). Raw-body proof in TestOmics_Share_ResponseShape (handler_shares_test.go) plus a TestOmics_GetShare_FullObjectFields regression guard; hand-reverted to confirm the three narrow-response subtests fail with the extra keys present against the pre-fix marshal, byte-identical restore verified via md5sum. Also checked types.ShareStatus both directions per the campaign's standing rule: SDK declares PENDING/ACTIVATING/ACTIVE/DELETING/DELETED/FAILED (enums.go:903-926); this backend's shares.go only ever stores PENDING (CreateShare), ACTIVATING (AcceptShare), and DELETED (DeleteShare) -- all three are legal SDK constants, no invented values, nothing to fix. FIXED (2026-08-21, gopherstack-muzq): 'nothing to fix' checked only that ACTIVATING/PENDING/DELETED are legal enum values, not whether ACTIVATING ever advances -- it doesn't. AcceptShare stamps Status ACTIVATING and nothing else in this backend ever writes to a Share's Status again (DeleteShare stamps DELETED on a copy after removing the record, not a live write); a client's GetShare-polling waiter never sees a terminal status. PENDING is correctly left alone -- that wait is for a client to call AcceptShare/RejectShare, not a stall. Confirmed no async mechanism anywhere in the package advances it (no ticker/goroutine/janitor/work.After/runDelayed; grepped all non-test .go files in services/omics). Fixed by mirroring the reap-on-read pattern GetWorkflow/GetAnnotationStore/GetVariantStore/GetWorkflowVersion already use in this same package (added Aug/Jul, predates this sweep) -- GetShare now advances ACTIVATING->ACTIVE on first poll via a new unexported Share.pollCount field, no new infrastructure. Proof: TestOmics_AcceptShare_ReachesActive (handler_shares_test.go), a real aws-sdk-go-v2 client test that creates+accepts a share and asserts GetShare returns ACTIVE; hand-reverted shares.go+models.go to git show HEAD, confirmed the test fails with Status stuck at ACTIVATING, restored, md5sum byte-identical."}
  RunCache: {status: ok, note: "CRUD + List; already used correct query params. 2026-08-14 (gopherstack-7185, mutating-op sweep): RunCache.CacheS3Location was tagged json:\"cacheS3Location\" -- that key is real only for CreateRunCacheInput's request body (serializers.go:1334); every RESPONSE shape (CreateRunCacheOutput, GetRunCacheOutput, ListRunCaches' element) uses the different key \"cacheS3Uri\" (deserializers.go:9853). A real client's CacheS3Uri was always nil on every read of a run cache. Fixed by retagging the model field (the handler's separate request-parsing struct already correctly used \"cacheS3Location\" and was untouched)."}
  RunBatch: {status: ok, note: "2026-08-07 (gopherstack-hnhk): body-shape re-architecture. StartRunBatch's real wire shape ({requestId, batchName, batchRunSettings:{inlineSettings|s3UriSettings}, defaultRunSetting:{roleArn,workflowId,...}, tags} -- field-diffed against awsRestjson1_serializeOpDocumentStartRunBatchInput/DefaultRunSetting/BatchRunSettings/InlineSetting) replaces the old flat {workflowId,roleArn,name} shape a real client never sends. Each inlineSettings entry (merged with defaultRunSetting per the documented per-run-override semantics) now creates a real constituent Run via the new startRunLocked helper shared with StartRun -- previously StartRunBatch created zero runs regardless of what a caller sent. GetBatch's real response shape (arn/creationTime/defaultRunSetting/id/name/runSummary/status/submissionSummary/submittedTime/processedTime/tags/totalRuns/uuid -- field-diffed against awsRestjson1_deserializeOpDocumentGetBatchOutput) is now built by a dedicated handler response, separate from ListBatch's smaller BatchListItem shape (arn/createdAt/id/name/status/totalRuns/workflowId) which was previously (and remains, now correctly) served by marshaling the same struct -- a latent leak risk this pass closed by giving each its own wire type instead of widening the shared one. runSummary's pending/running/completed/cancelled/failed counts are computed LIVE from surviving Run rows (summarizeRunBatchLocked) rather than stored, since this backend creates/completes runs synchronously and a stored counter would drift; deletedRunCount and submissionSummary's success/failure counts ARE stored, since DeleteRunsInBatch actually removes the Run rows they'd otherwise be computed from. ListRunsInBatch's runSettingId filter is now real (previously accepted-but-ignored; SubmissionStatus remains accepted-but-ignored -- this backend has no async submission-status state machine, batches complete synchronously). NOT modeled, see gaps: s3UriSettings (rejected with a clear ValidationException rather than silently creating zero runs -- reading real S3 object content synchronously is not something this backend can honestly simulate), most optional DefaultRunSetting fields (cacheBehavior/cacheId/configurationName/engineSettings/logLevel/networkingMode/outputBucketOwnerId/parameters/retentionMode/scratchStorageMode/storageCapacity/storageType/workflowOwnerId), and RequestId idempotency (accepted and required, matching the real API, but not deduplicated against retries)."}
  Configuration: {status: fixed, note: "gopherstack-4ggy: CreateConfiguration's RunConfigurations (a required CreateConfigurationInput member, api_op_CreateConfiguration.go:30-55) was dropped entirely, and the response was a near-total fabrication -- Configuration previously had only {creationTime,name,description,value}, where \"value\" is not a real field anywhere in the API at all (invented) and Arn/Status/Tags/Uuid/RunConfigurations (all real CreateConfigurationOutput/GetConfigurationOutput members) were simply absent. Rebuilt to the real shape: RunConfigurations now required and validated, ARN synthesized via pkgs/arn (arn:aws:omics:<region>:<account>:configuration/<uuid>, matching this service's existing workflow/run-group ARN convention), Status set to ACTIVE immediately (this resource has no async provisioning to model), Tags stored and echoed, Uuid populated. RunConfigurations.VpcConfig models SecurityGroupIds/SubnetIds; the response-only computed VpcId (types.VpcConfigResponse) is left empty rather than fabricated -- this backend does no real VPC/subnet resolution. RequestId (also client-side-required, but auto-filled by the SDK's IdempotencyTokenAutoFill middleware before validation runs, so a real client never omits it) is accepted but not enforced or deduplicated server-side -- out of scope for this fix, same category as RunBatch's RequestId gap noted below."}
  S3AccessPolicy: {status: ok, note: "FIXED (field-diffed against PutS3AccessPolicyInput/Output and GetS3AccessPolicyOutput, closing the prior deferred item): the policy document was serialized under the invented key \"policy\" -- real GetS3AccessPolicyOutput uses \"s3AccessPolicy\" (confirmed against the SDK deserializer) -- renamed; PutS3AccessPolicy's response now echoes s3AccessPointArn (was an empty {}); added StoreID/StoreType/UpdateTime fields to the model (StoreID/StoreType left empty -- this backend has no S3-access-point-to-store association to derive them from, but they're optional/pointer-safe on the wire)"}
  Tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource; RouteMatcher correctly scopes /tags/{arn} to arn containing \":omics:\" so FIS's /tags/{arn} isn't stolen"}
gaps:
  - "CLOSED 2026-08-07 (gopherstack-hnhk): RunBatch's real body shape is now modeled and StartRunBatch creates its constituent runs -- see the RunBatch family note above for the full accounting, including what's still not modeled (s3UriSettings, most optional DefaultRunSetting fields, RequestId idempotency dedup)."
  - "STALE CLAIM, CORRECTED 2026-08-23: this entry previously said ListAnnotationStores/ListVariantStores (status + ids), ListAnnotationStoreVersions (status), and ListShares (resourceArns/status/resourceTypes) don't apply their own real AWS filter/ids body fields. Re-verified against the pinned SDK and against this file's OWN neighboring op notes (which already document the fix, e.g. ListAnnotationStores's family note: 'ListAnnotationStores now applies its own status/ids filter... -- see AnnotationStoreVersion note'): all four filters are real, landed in PR #2417 (commit 69bbb940a, 2026-08-15, part of the same merge that fixed RAM/mq's accepted-then-ignored params) -- annotation_stores.go's ListAnnotationStores/ListAnnotationStoreVersions, variant_stores.go's ListVariantStores, and shares.go's ListShares each call storeMatchesFilter/shareMatchesFilter before including a row, and TestListAnnotationStores_Filters/TestListVariantStores_Filters/TestListAnnotationStoreVersions_FiltersByStatus/TestListShares_Filters all pass, asserting non-matching rows are excluded. This gap note simply never got marked CLOSED when the fix landed -- direction-1 stale claim (work described as open that is already done), not a re-discovery."
  - "RunBatchFilter.RunGroupID (ListBatch) is accepted from the query string for wire compatibility but not applied -- this backend has no run-group-of-a-batch's-runs association. RunsInBatchFilter.SubmissionStatus (ListRunsInBatch) is likewise accepted but not applied -- this backend has no async submission-status state machine (batches complete submission synchronously). RunSettingID IS now applied (fixed this pass, see RunBatch family note)."
deferred:
  - "Field-by-field diff of ReferenceMetadata/ReadSetMetadata optional sub-object fields (Files/ReferenceFiles, CreationJobId, CreationType, Etag, SequenceInformation) against the SDK model -- MD5/fileType (top-level scalars) are now confirmed correct; the sub-objects remain unpopulated but are optional/pointer-safe on the wire"
leaks: {status: clean, note: "pure synchronous in-memory backend -- no goroutines, tickers, or janitors; nothing to leak (reconfirmed this pass)"}
---

## Notes

**2026-09-04 (gopherstack-42g, missing-delete-precondition sweep):** read
every `Delete*` op's full doc comment in `omics@v1.49.5` looking for a
documented precondition the handler didn't enforce (the "highest-yield
pattern" per this campaign's own tracking). Four of the eleven `Delete*`/
`BatchDeleteReadSet` ops document one; the other seven (`DeleteRunGroup`,
`DeleteWorkflow`, `DeleteWorkflowVersion`, `DeleteVariantStore`,
`DeleteAnnotationStore`, `DeleteAnnotationStoreVersions`, `DeleteRunCache`,
`DeleteShare`, `BatchDeleteReadSet`) state no precondition and were correctly
left alone. All four documented preconditions were entirely unenforced:

- `DeleteReferenceStore` -- "You can only delete a reference store when it
  does not contain any reference genomes." (api_op_DeleteReferenceStore.go)
  -- deleted the store (and cascade-deleted its references) unconditionally.
- `DeleteReference` -- "The read set associated with the reference genome
  must first be deleted before deleting the reference genome."
  (api_op_DeleteReference.go) -- deleted the reference even while a read
  set's `ReferenceARN` (populated by `StartReadSetImportJob`/
  `CompleteMultipartReadSetUpload`, `read_sets.go`) still pointed at it.
- `DeleteSequenceStore` -- "You can only delete a sequence store when it
  does not contain any read sets." (api_op_DeleteSequenceStore.go) --
  deleted the store (and cascade-deleted its read sets) unconditionally.
- `DeleteRun` -- "You can only delete a run that has reached a COMPLETED,
  FAILED, or CANCELLED stage." (api_op_DeleteRun.go) -- deleted a run in any
  status, including PENDING/RUNNING. Notably this file's own `DeleteRunBatch`
  (real `DeleteBatch` semantics, `runs.go`) already enforces the identical
  precondition class correctly via `ErrInvalidState`/`isRunBatchTerminal` --
  `DeleteRun` itself was the miss, not the pattern.

All four confirmed against the per-op error switch in `deserializers.go`
(`awsRestjson1_deserializeOpError<Op>`): each op's error set includes
`ConflictException`, matching the existing `ErrInvalidState` sentinel
(`store.go`, wraps `awserr.ErrConflict`, already used by `DeleteRunBatch`).
Fixed by adding a precondition check to each backend method before the
delete proceeds: `DeleteReferenceStore`/`DeleteSequenceStore` check their
respective by-store index is empty; `DeleteReference` scans `b.readSets.All()`
for a matching `ReferenceARN`; `DeleteRun` checks `run.Status` against a new
`runDeletableStatuses` set (COMPLETED/FAILED/CANCELLED), mirroring
`DeleteRunBatch`'s existing `runBatchDeletableStatuses`/`isRunBatchTerminal`
pattern rather than inventing a new one.

Four new tests in `delete_precondition_test.go`
(`TestDeleteRun_RequiresTerminalState`,
`TestDeleteReferenceStore_RequiresEmpty`,
`TestDeleteSequenceStore_RequiresEmpty`,
`TestDeleteReference_RequiresNoAssociatedReadSet`), each asserting the
precondition is rejected (409/ConflictException), the resource survives the
rejected delete, and the delete succeeds once the precondition is satisfied.
Each fix was hand-neutered (guard clause removed, package rebuilt, test
re-run to confirm the expected failure, source restored and `md5sum`-verified
byte-identical) independently -- including cross-checking that neutering only
`DeleteReference`'s guard failed only
`TestDeleteReference_RequiresNoAssociatedReadSet` while
`TestDeleteReferenceStore_RequiresEmpty` (a similarly-shaped adjacent guard)
stayed green, guarding against the near-duplicate-guard false-negative trap.

Gates: `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean),
`go test -race -count=1` (pass), `golangci-lint run` (0 issues) --
`./services/omics/...`; `go test -race -count=1 ./services/cloudformation/...`
(dependent service, pass, unaffected).

**2026-08-21 (gopherstack-r80d batch 7):** required-response-member sweep
(inverted from required-INPUT-member sweeps elsewhere in this file: is the
field WRITTEN, not READ). Read all 40 ops with >=1 required output field
end to end (182 required fields total per `cmd/requiredoutputfields`)
against the pinned `omics@v1.49.5` SDK, not just grepped. This service had
already been through unusually dense incidental required-output
field-diffing across five prior passes (gopherstack-lx5h/kb66, -7s8r, -dv4s
x2, -80bz) — most of the surface was already correct. Four real bugs
remained, all previously flagged-but-explicitly-deferred by those prior
passes as "the opposite class" from their own narrower scope, when they are
in fact exactly this campaign's target class:

- `CreateAnnotationStore` dropped the required `versionName` entirely (no
  struct field at all) and, once narrowed to its own dedicated response
  type to add it, turned out to also have been over-sharing the full
  `AnnotationStore` shape (`storeArn`/`reference`/etc, none real
  `CreateAnnotationStoreOutput` members) -- both fixed together, see the
  `AnnotationStore` family note above.
- `AnnotationStoreVersion` (backing Create/Get/Update, plus the List
  summary) was missing a required `id` member entirely and mistagged the
  required `name` member as the invented key `storeName` -- see the
  `AnnotationStoreVersion` family note above.
- `VariantStore` (backing Get/Create, plus the List summary) was missing a
  required `sseConfig` member entirely -- see the `VariantStore` family
  note above.
- `MultipartReadSetUpload`'s `referenceArn` was tagged `omitempty` despite
  being a required `CreateMultipartReadSetUploadOutput` member -- see the
  `MultipartReadSetUpload` family note above.

**Method note for future batches: `omitempty` only matters for pointer-like
required fields.** Cross-checking every `omitempty`-tagged field on this
service against its real SDK counterpart's Go type turned up several
`map[string]any`/`*time.Time`/`bool` fields that looked suspicious at a
glance (`AnnotationStore.Reference`, `AnnotationImportJob.FormatOptions`,
`AnnotationImportJob.RunLeftNormalization`, etc.) but are NOT bugs, verified
by reading the real deserializer, not just the struct tag:

- For map/interface/struct-pointer-typed required fields (unions like
  `types.FormatOptions`, `*types.SseConfig`, `types.ReferenceItem`), every
  generated deserializer in this module starts with
  `if value == nil { return nil }` *before* checking any inner key -- so a
  JSON key present with value `null` and a JSON key absent entirely decode
  to the exact same zero value. `omitempty` dropping the key changes
  nothing a real client can observe.
- For `bool`/non-pointer-enum-string-typed required fields (e.g.
  `RunLeftNormalization bool`, `Status types.JobStatus`), the real SDK
  struct field itself isn't a pointer -- its Go zero value already equals
  what a real client sees whether or not the key was present in the JSON.
- The distinguishing case that IS a real bug: a required field that's a
  **pointer to a scalar** in the real SDK (`*string`, `*int32`, `*int64`)
  paired with a **non-pointer** field on this backend's struct tagged
  `omitempty`. There, a legitimately-zero value (`""`, `0`) is empty enough
  for Go's `omitempty` to drop the key, but the deserializer's
  `if value != nil` check treats a present zero value (`""`, not `null`)
  as real data and sets a non-nil pointer -- so omitting the key (leaving
  the real client's pointer `nil`) is genuinely different from including it
  (`referenceArn`/the `versionSizeBytes`-class fixes already in this file
  are this shape; `runLeftNormalization`/the passthrough-map fields are
  not). Check the real Go field's pointer-ness before spending time on an
  `omitempty` fix.

Also confirmed (not fixed, out of this cut's scope): `CreateVariantStore`
still doesn't read the real, optional `CreateVariantStoreInput.Description`
-- unlike `SseConfig`, this doesn't break the required-output contract
(`Description` is already present-but-empty on `VariantStore`, satisfying
`GetVariantStoreOutput`'s requirement), so it's an input-fidelity bug for a
future input-sweep pass, not this cut's target class.

`sagemaker` (459 required fields, largest remaining candidate) was
deliberately not started this batch: `_REQUIRED_OUTPUT_CANDIDATES.md` flags
it as overlapping the ongoing `gopherstack-oc9v` anonymous-inline-request-
struct conversion, and its 403-op surface is roughly 2.5x this batch's
combined omics+bedrockagent scope -- left for a batch that can commit to it
alone. `bedrockagent` picked up next in the same batch, see its own
PARITY.md.

**2026-08-15 (gopherstack-keee):** investigated the reported host-prefix
reachability gap ("the Omics SDK client unconditionally rewrites the request
host to workflows-<host>"). The real scope is larger than the issue's own
framing: **all 107 real Omics operations** carry a host-prefix rewrite, not
just the run/workflow/configuration family, split across **five** distinct
literal prefixes (grepped every `api_op_*.go` in the pinned
`omics@v1.49.5` module for `req.URL.Host = "..." + req.URL.Host`):
`workflows-` (38 ops), `control-storage-` (34), `analytics-` (28), `storage-`
(4: GetReadSet/GetReference/CompleteMultipartReadSetUpload/
UploadReadSetPart), `tags-` (3: Tag/UntagResource/ListTagsForResource).
Mechanism: a **per-operation Smithy Finalize-stage middleware**
(`endpointPrefix_op<Op>Middleware`, e.g. `api_op_CancelRun.go:127`, inserted
via `stack.Finalize.Insert(..., "ResolveEndpointV2", middleware.After)`),
**not** an endpoint resolver and not a static trait read once — this is the
generated code for Smithy's `@endpoint(hostPrefix:)` trait, checked at
`smithy-go@v1.27.6/transport/http/middleware_metadata.go`.

**Not unique to Omics.** Grepping every pinned SDK service module in
`go.mod` for the same `req.URL.Host = "..." + req.URL.Host` shape found five
more affected, ALL of which gopherstack implements: `mwaa` (12 ops — nearly
its entire surface, three prefixes `api.`/`env.`/`ops.` using `.` not `-`),
`lakeformation` (5: GetQueryState/GetWorkUnitResults/GetQueryStatistics/
GetWorkUnits/StartQueryPlanning, `query-`/`data-`), `cloudwatchlogs` (2:
GetLogObject/StartLiveTail, `stream-`), `servicediscovery` (2:
DiscoverInstances/DiscoverInstancesRevision, `data-`), `sfn`/stepfunctions
(2: TestState/StartSyncExecution, `sync-`). Filed as gopherstack-3gbe (P2) —
same mechanism, same conclusion below almost certainly applies to each, but
none were individually re-verified against their own RouteMatcher this pass.

**No gopherstack routing/auth code needed to change, for Omics or (by the
same reasoning) likely the other five.** `Handler.RouteMatcher`
(`handler.go:223`) matches on `URL.Path` alone; cross-checking all 107 real
`(method, path)` pairs (extracted from `serializers.go`'s
`httpbinding.SplitURI` calls) against their host-prefix family found **zero
collisions** — no two ops share a path that only Host could disambiguate,
unlike s3's bucket-vs-path or glacier's vacuity-trap class. SigV4
verification (`pkgs/httputils/sigv4.go:241`) derives its canonical-request
"host" from whatever the request actually arrived with (`r.Host`), not a
configured/expected value, so it verifies correctly regardless of which
prefix a real client sent. **The actual unreachability is a pure
client-side DNS/dial failure**: the Finalize middleware runs before the
transport dials, so `req.URL.Host` becomes `workflows-127.0.0.1:NNNN` (etc.)
before any TCP SYN is sent — confirmed live, quoting the real error:
`dial tcp: lookup workflows-127.0.0.1 on 127.0.0.53:53: no such host`. No
gopherstack server code executes at all in the failure case; there is
nothing in `pkgs/service/router.go` or any `RouteMatcher` to fix.

Added `host_prefix_reachability_test.go`: drives the real,
**unmodified** `aws-sdk-go-v2/service/omics` client (not a hand-crafted
request) through one representative op per prefix family
(workflows/analytics/control-storage/tags — the four "storage-" ops are
scoped out, they need an existing sequence/reference store with real
uploaded byte content before they're callable, out of scope for this pass).
`TestSDKRoundTrip_HostPrefix_Unreachable_BeforeFix` proves the unmodified
client fails as described (quoted above). `TestSDKRoundTrip_HostPrefix_Reachable_AfterFix`
redials straight to the httptest listener regardless of the rewritten host
(same technique as `services/s3control/handler_create_tags_test.go`'s
per-account-ID-host workaround) — critically, this does **not** disable the
SDK's host-prefix rewrite (unlike `wire_field_additions_test.go`'s existing
`disableAnalyticsHostPrefix`, which every other round-trip test in this
package already uses to sidestep this exact problem): the request that
reaches gopherstack still carries `Host: workflows-127.0.0.1:NNNN`, and the
op succeeds and decodes correct values anyway, proving gopherstack survives
the real rewrite rather than avoiding it. Confirmed s3 virtual-hosted-style
addressing (`TestHandler_VirtualHostedStyle*`) and `pkgs/...`,
`pkgs/service/...` remain green, unmodified by this pass.

Real-deployment implication (documented, not fixed in code, since there is
no code to fix): a production gopherstack endpoint that real Omics SDK
clients must reach needs DNS coverage for the five literal prefixes above
prepended to its hostname (e.g. a wildcard record), the same class of
requirement s3's virtual-hosted-style addressing and CloudFront
KeyValueStore's per-account-ID host already impose on deployers.

**2026-08-13 (gopherstack-lx5h/gopherstack-kb66):** fixed the omics items
these two bd issues deferred from the required-response-member sweep (the
other 7 services across both issues were fixed elsewhere; omics was held by
another agent at the time). All four premises verified against the pinned
`omics@v1.49.5` `deserializers.go`/`serializers.go` (path resolved from
`go.mod`, read only that module-cache copy, not a stale sibling version):

- `GetAnnotationStore`/`GetVariantStore` ARN tagged `json:"arn"` → real key
  `storeArn` (deserializers.go:6266/11673).
- `GetAnnotationStoreVersion` ARN tagged `json:"arn"` → real key
  `versionArn` (deserializers.go:6564).
- `StartAnnotationImportJob` job id tagged `json:"id"` → real key `jobId`
  (deserializers.go:17434) — but `GetAnnotationImportJob`/
  `ListAnnotationImportJobs` genuinely use `id` (deserializers.go:5954), so
  this was NOT a blanket rename: `AnnotationImportJob.ID` keeps its `id` tag
  (correct for Get/List) and `handleStartAnnotationImportJob` now builds a
  dedicated `{"jobId": ...}` response instead of marshaling the domain
  struct. Reading the whole Start/Get operation pair (not just the one op
  gopherstack-lx5h named) found the *identical* bug on
  `StartVariantImportJob` (real key `jobId`, deserializers.go:18893, vs
  `GetVariantImportJob`'s `id`, deserializers.go:11383) — fixed the same way,
  reported here since neither originating issue named it.
- `NumVersions`/`StoreSizeBytes` (AnnotationStore, VariantStore) and
  `VersionSizeBytes` (AnnotationStoreVersion) had no model field at all.
  `NumVersions` is derived live from `annotationVersionsByStore` (the backend
  already tracks per-store version rows via that index) rather than stored,
  to avoid drift. The two size fields are honestly modeled but always `0`:
  nothing in this in-memory backend measures real stored bytes, and a
  required wire field can't be omitted the way an optional one can, so `0`
  (not a fabricated plausible-looking number) is what a client receives.
- `FormatOptions`/`RunLeftNormalization` were missing from
  `GetAnnotationImportJob`/`GetVariantImportJob` on both the request
  (`StartAnnotationImportJob`/`StartVariantImportJob` silently dropped
  caller-supplied values — the handler never even read them into its request
  struct) and response sides — a schema gap, not a dropped key, per the
  issue's framing. Field-diffing the full `GetAnnotationImportJobOutput`/
  `GetVariantImportJobOutput` shapes (not just the two named fields) while
  already inside these structs turned up the same class of gap on
  `VersionName` (annotation only — confirmed variant import jobs have no
  such field anywhere in the real API), `StatusMessage`, `UpdateTime`, and
  `AnnotationFields`; all four are also real required (or accepted-but-
  dropped optional) members of the same ops, so they were closed in the same
  pass rather than left half-fixed next to the two the issue named.

Two further gaps were found but NOT fixed this pass, to keep scope to what
the two structs actually needed for their named ops (both worth a follow-up
bd issue): `StatusMessage` (real required `GetAnnotationStoreOutput`/
`GetVariantStoreOutput`/`GetAnnotationStoreVersionOutput` field,
deserializers.go:6257 etc.) is absent from `AnnotationStore`/`VariantStore`/
`AnnotationStoreVersion` entirely — a wider version of the same StatusMessage
gap closed on the import-job structs above, but touching three more
Create/Get/Update/List families was out of scope here; and per-item
`JobStatus` (real required `AnnotationImportItemDetail`/
`VariantImportItemDetail` field, types.go:75-88/2060-2076) is still missing
from `AnnotationImportJob.Items`/`VariantImportJob.Items`, which only carry
`Source` — `StartAnnotationImportJob`/`StartVariantImportJob` take
`ItemSource` (Source only) but `Get`/`List` return `ItemDetail` (Source +
JobStatus), two genuinely different real shapes this service currently
conflates into one.

Proof: `wire_field_additions_test.go` drives the real `aws-sdk-go-v2/service/
omics` client against an `httptest` server (same pattern as
`services/acm/wire_field_additions_test.go`) for all of the above — a raw-
JSON assertion against the wrong key would have passed against the bug, so
only round-tripping through the genuine SDK deserializer proves the fix.
Every case was hand-verified to fail against the pre-fix code (tag/field/
handler-arg reverted, test re-run, re-applied) before being counted as
proof, not just written and trusted: 8 tests, 7 of 8 fix categories directly
reverted and re-confirmed failing (the eighth, `VersionSizeBytes`, shares its
model-diffing proof with `StoreSizeBytes`/`NumVersions`).

The annotation/variant-store family of operations routes to a real
`analytics-<region>...` endpoint-host-prefix (see e.g.
`endpointPrefix_opGetAnnotationStoreMiddleware` in the generated SDK); the
test helper disables it via `smithyhttp.DisableEndpointHostPrefix` on an
Initialize-step middleware so the SDK talks to the local `httptest` server
instead of trying to resolve a nonexistent `analytics-127.0.0.1` host.

**2026-08-13 (gopherstack-7s8r):** closed the two gaps the prior pass
deferred, both verified against the pinned `omics@v1.49.5`
`deserializers.go`/`types/types.go`:

- `StatusMessage` is a real required member of `GetAnnotationStoreOutput`,
  `GetVariantStoreOutput` and `GetAnnotationStoreVersionOutput` and was
  absent from `AnnotationStore`/`VariantStore`/`AnnotationStoreVersion`
  entirely (the equivalent gap on the import-job structs was already closed
  in c41d36cb6). Added to all three, always empty: none of these ops track
  an error state to describe.
- The `Items` conflation: the issue's stated premise was that Start returns
  `ItemSource` (Source only) while Get *and List* return `ItemDetail`
  (Source + required `JobStatus`). Verified half true, half false against
  the SDK: Get does return `ItemDetail` — confirmed for both
  `AnnotationImportItemDetail` (types.go:75-89) and
  `VariantImportItemDetail` (types.go:2060-2071, which also carries an
  optional `StatusMessage` the annotation variant lacks) — but List does
  not return `Items` at all. The real List element types
  (`AnnotationImportJobItem`, types.go:102-146;
  `VariantImportJobItem`, types.go:2090-2132) have no `items`,
  `formatOptions` or `statusMessage` member whatsoever — a narrower shape
  than Get, the same class of gap `c41d36cb6` already found and split for
  Start's `jobId`-only response. `AnnotationImportJob`/`VariantImportJob`
  (used for Get) now carry `Items []AnnotationImportItemDetail`/
  `[]VariantImportItemDetail`, populated once at Start time by stamping
  every source item with the job's own `Status` — honest because this
  backend always completes import jobs synchronously in one step, so that
  status is each item's genuine final state, not a guess partway through an
  async pipeline that doesn't exist here. `AnnotationImportItem`/
  `VariantImportItem` (the pre-existing Source-only structs) are unchanged
  and now documented as exactly `ItemSource`, still correct for
  `StartAnnotationImportJobInput.Items`/`StartVariantImportJobInput.Items`.
  New `AnnotationImportJobSummary`/`VariantImportJobSummary` types back the
  List responses instead of the Get-shaped domain structs.

Five new tests in `wire_field_additions_test.go`, all driving the real
`aws-sdk-go-v2/service/omics` client (or, for the two List-narrowing tests,
a raw HTTP body inspection — the SDK's `ListAnnotationImportJobsOutput`/
`ListVariantImportJobsOutput` deserializers silently discard unrecognized
keys, so an SDK round trip cannot detect an over-wide response the way it
can detect a missing field; only inspecting the raw wire body proves the
extra keys are gone). All five were hand-verified to fail against the
pre-fix code (files reverted to `HEAD`, tests re-run, fix re-applied) before
being counted as proof: `Test_SDKRoundTrip_StatusMessage`,
`Test_SDKRoundTrip_AnnotationImportJob_ItemDetail`,
`Test_SDKRoundTrip_VariantImportJob_ItemDetail`,
`TestListAnnotationImportJobs_OmitsGetOnlyFields`,
`TestListVariantImportJobs_OmitsGetOnlyFields`.

Found while reading the whole operations but NOT fixed this pass (separate,
narrower scope than the two named findings; worth its own bd issue):
`ListAnnotationStores`/`ListVariantStores`/`ListAnnotationStoreVersions`
have the identical List-narrower-than-Get defect just fixed for import
jobs — the real List element types (`AnnotationStoreItem`, types.go:152-211;
similarly for variant stores and store versions) lack `NumVersions`/`Tags`/
`StoreOptions` that `Get*StoreOutput` requires, but this backend still
marshals the full Get-shaped store struct for List, leaking those fields.

**2026-08-13 (gopherstack-jqh2 pass 2):** re-extracted all 107 ops' real
method+path directly from `omics@v1.49.5` serializers.go and drove them
through `ExtractOperation` via `handler_sdk_route_table_test.go`
(`TestExtractOperation_SDKRouteTable`, one subtest per op). All 107 resolved
correctly — the route/dispatch fixes from the 2026-08-07 pass below held, and
no new drift was found. This test is now the permanent regression guard for
route-table drift, replacing ad hoc re-verification on future audits.

Protocol: restjson1. Every op path/method was cross-checked op-by-op against
`aws-sdk-go-v2/service/omics@v1.45.0`'s generated `serializers.go` (both the
`awsRestjson1_serializeOpHttpBindings*Input` — method/URI/query — and
`awsRestjson1_serializeOpDocument*Input` — JSON body — functions for every
op), not against this handler's own output. That direct cross-check is what
surfaced the three route/wire-shape bugs below; `go test ./services/omics/...`
was green throughout because the pre-existing unit tests drive the handler via
`h.Handler()(c)` directly (bypassing `RouteMatcher`) and, worse, used the
*wrong* HTTP method for `ListRunsInBatch` to begin with — the bug and its test
coverage were both wrong in the same direction. This is the same trap class
that hit services/backup, eks, and s3control.

**Waiter-hang state-machine bug (5 resources).** HealthOmics's SDK ships
generated waiters (`WorkflowActiveWaiter`, `WorkflowVersionActiveWaiter`,
`AnnotationStoreCreatedWaiter`, `VariantStoreCreatedWaiter`,
`RunRunningWaiter`/`RunCompletedWaiter`, `TaskRunningWaiter`/
`TaskCompletedWaiter`) that poll `GetWorkflow`/`GetAnnotationStore`/
`GetVariantStore`/`GetWorkflowVersion`/`GetRun`/`GetRunTask` until `Status`
leaves its initial transient value (`CREATING` or `PENDING`). Before this
pass, `Workflow`, `WorkflowVersion`, `AnnotationStore`, `VariantStore` were
created with `Status: CREATING` and nothing ever mutated it, and `Run`/
`RunTask` were created `PENDING` and only ever moved to a terminal state via
explicit `CancelRun`. Any script/test using the real SDK's own waiters (the
idiomatic way to wait for `CreateWorkflow`/`StartRun` to be usable) would
therefore poll forever until the waiter's own `maxWaitDur` elapsed. Fixed by
adding an unexported `pollCount int` field to each of these five structs
(intentionally not JSON-tagged, so it isn't part of the wire shape and isn't
persisted across a snapshot/restore — the exact same pattern
`services/kafka`'s `Cluster.pollCount` / `DescribeCluster` already uses) and
advancing `Status` by one step each time the corresponding `Get*` op is
called: `CREATING`→`ACTIVE` on the first poll for the four store/workflow
resources, `PENDING`→`RUNNING`→`COMPLETED` across the first two polls for
`Run`/`RunTask`. `List*` ops deliberately do **not** advance status (matching
kafka's `ListClusters` precedent) — only the real waiters' polling target
(`Get*`) does.

**Pagination wire-shape bug (16 List ops, all with a `filter` body field).**
For `ListReferenceStores`, `ListReferences`, `ListReferenceImportJobs`,
`ListSequenceStores`, `ListReadSets`, `ListReadSetActivationJobs`,
`ListReadSetExportJobs`, `ListReadSetImportJobs`,
`ListMultipartReadSetUploads`, `ListReadSetUploadParts`,
`ListAnnotationImportJobs`, `ListVariantImportJobs`,
`ListAnnotationStoreVersions`, `ListShares`, `ListAnnotationStores`,
`ListVariantStores`, the real SDK always sends `maxResults`/`nextToken` as
**query-string** parameters — only the optional `filter` (plus, for a few
ops, `ids`/`resourceOwner`) travels in the JSON body. This handler previously
read `maxResults`/`nextToken` from the JSON body for all of these, which real
SDK clients never populate there, so pagination was silently broken: a
client's `nextToken` was ignored (every page restarted from the top) and
`maxResults` defaulted to the 100-item cap regardless of what the client
asked for. Fixed via a new `listQueryParams(c)` helper (reads `maxResults` +
`nextToken` from the query string) used by all 16 handlers; the `filter` body
struct is kept as-is where the backend already supports it.

The RunGroup/Run/RunTask/Workflow/WorkflowVersion/RunCache/Configuration
family instead uses `maxResults` + **`startingToken`** (not `nextToken`) —
those already read correctly via the pre-existing `paginationQueryParams`
helper and were not touched. The RunBatch family (`ListBatch`/
`ListRunsInBatch`) uses **`maxItems`** (not `maxResults`) + `startingToken`;
a new `batchQueryParams(c)` helper was added for those two.

**RunBatch family: three compounding bugs, all in the same neighborhood.**
1. `ListRunsInBatch` (real wire shape: `GET /runBatch/{batchId}/run`) was
   classified under `classifyPOST` instead of `classifyGET`, so it was
   entirely unreachable by a real SDK client (which always sends GET); a
   POST to that path fell through to `opUnknown` → 501. The two pre-existing
   `parity_test.go` tests for this op used `http.MethodPost`, which is why
   green tests didn't catch it.
2. Real `DeleteBatch` (`DELETE /runBatch/{batchId}`) deletes the batch
   *resource*; real `DeleteRunBatch` (`POST /runBatch/delete`, body
   `{"batchId": "<single id>"}`) deletes the *runs* belonging to a batch and
   leaves the batch resource intact — confirmed against `DeleteBatchInput`/
   `DeleteRunBatchInput` and their serializers, which both take a single
   `BatchId *string`, not an array, and `DeleteRunBatchOutput` is empty (no
   error list). This codebase had the two operations' wire paths bound to the
   opposite handler bodies: `DELETE /runBatch/{id}` ran a single-ID
   batch-record delete under the *`DeleteRunBatch`* op name, and
   `POST /runBatch/delete` expected a `{"batchIds": [...]}` array and
   bulk-deleted batch *records* (not runs) under the *`DeleteBatch`* op name.
   Fixed by swapping which op constant `classifyDELETE`/`classifyPOST` return
   for each path, swapping the two handler function bodies to match, and
   replacing `InMemoryBackend.DeleteRunBatches([]string)` with
   `DeleteRunsInBatch(batchID string)` (cascades to the run's `RunTask`s, not
   the `RunBatch` record).
3. `ListBatch`/`ListRunsInBatch` pagination used the query key `maxResults`;
   real AWS uses `maxItems` for these two ops specifically (see the
   pagination note above).

**Trap for the next auditor:** `RunBatch`/`ReadSetActivationJob`/
`ReadSetExportJob`/`ReadSetImportJob`/`AnnotationImportJob`/
`VariantImportJob`/`ReferenceImportJob`/`AnnotationStoreVersion` are all
created with a terminal `Status` (`COMPLETED`/`ACTIVE`) synchronously — this
looks superficially like the same "never transitions" bug class as
Workflow/AnnotationStore/VariantStore/Run/RunTask, but it is **not** a bug:
these resources have no real-AWS waiter that ever expects to observe a
transient state first (`RunBatch` in particular starts `COMPLETED` because
this emulator has no actual async batch-run orchestration to model), so
returning the terminal state immediately is correct and matches how a fast
real backend would eventually settle. Confirm by checking whether the SDK
ships a `*Waiter` for the corresponding `Get*`/`Describe*` op before flagging
this pattern again.

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped key renames

`omicsSnapshotVersion` bumped 1 -> 2. `c41d36cb6` retagged `AnnotationStore.Arn` and
`AnnotationStoreVersion.Arn` (both registered table value types) to `storeArn`/`versionArn`,
and `95edfe255` retagged `AnnotationStoreVersion.StoreName` from `storeName` to `name`,
neither bump applied at the time. A pre-fix snapshot's `"arn"`/`"storeName"` data is
unrecognized by the new tags: `StoreArn`/`VersionArn`/`StoreName` silently decode as empty
strings, and since `AnnotationStoreVersion`'s secondary `byStore` index is keyed on
`StoreName`, the zeroed field also misfiles the restored version out of its store's version
list entirely (silent, not loud).

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `TestInMemoryBackend_RestoreV1AnnotationStoreVersionDiscarded` (persistence_test.go)
builds a v1-shaped snapshot with an `AnnotationStore`/`AnnotationStoreVersion` tagged
`arn`/`storeName` and asserts `GetAnnotationStoreVersion` returns `ErrNotFound` after restore
(clean wholesale discard), not silently decoded with the identity fields zeroed.
Hand-reverted to version 1: `ListAnnotationStoreVersions("store-1", ...)` on the same
snapshot returns 0 versions instead of the 1 restored (the version record decodes but is
misfiled under the zeroed `StoreName` key, so it silently vanishes from every store-scoped
lookup) -- confirming the original symptom; restored and `md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

**Bug found and fixed:** `paginateStrings` (`store.go`), the tail of `paginatedCopies` —
the shared pagination path for all 20 `List*` operations built on it (annotation stores,
annotation import jobs, annotation store versions, configurations, reference stores, shares,
run groups, runs, run caches, run batches, workflows, sequence stores, variant stores,
variant import jobs, read sets, read set activation/export/import jobs, multipart uploads) —
looked up the cursor by exact id match (`id == nextToken`) and left `start` at its zero-value
default when no match was found. Net effect: if the id named by the cursor had been deleted
between the two `List*` calls, pagination silently restarted from the beginning, redelivering
every already-seen item instead of resuming past the gap. Fixed by searching for the first
`id >= nextToken` instead of `==`, defaulting `start = len(ids)` on no match (was 0) — matches
the same fix applied to the equivalent bug independently found in `services/dax`
(`paginateList`/`paginateClusters`/`ListTags`) this same pass.

Proof: `TestPaginateStrings_StaleCursorAfterDeletion`/`TestPaginateStrings_CursorPastEnd`
(pagination_arithmetic_test.go, unit, calls `paginateStrings` directly via a new
`PaginateStringsForTest` export) and
`TestListReferenceStores_SDKRoundTrip_PaginationSurvivesDeleteBetweenPages`
(pagination_sdk_roundtrip_test.go, real `aws-sdk-go-v2/service/omics` client, deletes the
store the cursor names before fetching page 2) both fail pre-fix and pass post-fix.

**Recorded, not fixed:** `ListReadSetUploadParts` (`read_sets.go`) has the identical
exact-match-cursor shape (`strconv.Itoa(p.PartNumber) == nextToken`, no fallback), and its
`parts` slice is never sorted before pagination (insertion order, not `PartNumber` order) —
but it was found dormant, not reachable: there is no operation that removes an individual
upload part (only whole-upload delete/abort via `AbortMultipartReadSetUpload`/
`CompleteMultipartReadSetUpload`, and `UploadReadSetPart` re-uploading the same
part+source *updates* the existing entry in place rather than moving or removing it). A
correct fix would also need either a numeric (not string-lexicographic) comparison or an
explicit sort by `PartNumber`, since `"10" < "9"` as strings — deferred as a design decision
rather than patched blind, per this pass's instruction to record undefined/unreachable
behaviour rather than invent a rule for it.

`paginatedCopies` itself (the sort + call to `paginateStrings` + per-id copy) was re-read and
found otherwise correct — every caller sorts implicitly via `sort.Strings(ids)` inside
`paginatedCopies`, so no caller-side ordering bug.

Gates: `go build`, `go vet` (default/e2e/integration), `go test -race -count=1`,
`golangci-lint run` (0 issues) — all `./services/omics/...`.

## 2026-08-31 (gopherstack-uox6, value-semantics sweep)

Swept every List/Describe filter matcher in this service against its own SDK doc
comment (annotation/variant stores + versions, shares, runs/run groups/run
batches/run tasks, workflows/workflow versions, read sets, reference stores,
references, sequence stores) for the class this issue targets — a request
parameter read and applied, but WRONG, as opposed to never read at all (already
covered by the request-field-never-read axis). ONE BUG:

- **`StartRun`'s `NetworkingMode` ignored its own documented default.**
  `StartRunInput.NetworkingMode`'s doc comment (`api_op_StartRun.go:136-138`):
  "Optional configuration for run networking behavior. If not specified, this
  will default to RESTRICTED." `runs.go`'s `startRunLocked` stored whatever
  string it was given, including `""`, and `Run.NetworkingMode` is tagged
  `json:"networkingMode,omitempty"` — so an omitted value was dropped from the
  wire entirely rather than resolving to `RESTRICTED`, and a real client's
  `*string` on both `StartRunOutput.NetworkingMode` and `GetRunOutput.NetworkingMode`
  decoded to nil/`""` instead of the documented default. Fixed by defaulting to
  `RESTRICTED` inside `startRunLocked` when the caller passes an empty string —
  this also correctly applies the same default to `StartRunBatch`'s constituent
  runs, which always pass `""` here since `DefaultRunSetting.NetworkingMode` is
  not modeled for batches (disclosed below), and real AWS applies the identical
  per-run default there too. Proven via
  `Test_SDKRoundTrip_StartRun_NetworkingModeDefault` (`wire_field_additions_test.go`),
  a real `aws-sdk-go-v2` client test asserting both the omitted-default case
  (`RESTRICTED` on `StartRunOutput` and `GetRunOutput`) and that an explicit
  `VPC` value is not overridden; hand-reverted to confirm it fails against the
  pre-fix code (`""` instead of `"RESTRICTED"`), restored byte-identical.

**Everything else checked came back clean, member by member:**

- Query-protocol concerns don't apply here — omics is REST-JSON and every
  filter struct is decoded via `encoding/json` with no explicit struct tags, so
  Go's case-insensitive field matching handles every wire key checked
  (`resourceArns`/`status`/`type` on `types.Filter` for `ListShares`,
  confirmed against `awsRestjson1_serializeDocumentFilter`); no casing bug
  found or possible via this path.
- `shareResourceType`'s ARN-pattern switch and `ListShares`'s `resourceOwner`
  switch (`SELF`/`OTHER`) both compare against the exact real enum members
  (`types.ShareResourceType`, `types.ResourceOwner`) — verified against
  `enums.go`, no invented value, no partial-spelling collision.
- `shareMatchesFilter`'s `ResourceArns`/`Status`/`Type` are each real "any of"
  lists (`slices.Contains`, every element checked, not just the first) —
  matches `types.Filter`'s own doc comments ("You can specify up to 10
  values").
- Every other filter checked (`ReadSetFilter`, `RunFilter`, `RunGroupFilter`,
  `RunBatchFilter`, `RunTaskFilter`, `WorkflowFilter`, `WorkflowVersionFilter`,
  `StoreStatusFilter`, `ImportJobFilter`, `SequenceStoreFilter`,
  `ReferenceStoreFilter`, `ReferenceFilter`) is a documented single-value
  equality (`Name`/`Status`/`Type`/`StoreName`/`RunGroupId`/`BatchId`) with no
  documented case-insensitivity, wildcard, or negation modifier anywhere in
  the pinned SDK — plain `==` is correct for all of them, verified against
  each field's own doc comment rather than assumed.
- MaxResults/MaxItems: no `List*Input` in this service documents a numeric
  default or maximum except `ListBatch`'s `MaxItems` ("If not specified,
  defaults to 100") — `batchQueryParams` + the shared `maxPageSize = 100`
  cap/default already match it exactly. Every other List op's MaxResults doc
  comment states no number, so the uniform 100 cap contradicts nothing (same
  clean verdict as the campaign's quicksight List/Describe pass).
- `ListRunBatches`'s disclosed `RunGroupID`-accepted-but-not-applied gap
  (real AWS filters by the *contained runs'* run-group, which this simplified
  RunBatch model doesn't track) was re-verified against the SDK rather than
  trusted from the existing comment — still correct, still structural.

**Recorded as the request-field-never-read axis, not this one (declared
nowhere in this backend's filter/request structs, so not applicable to
"wrong algorithm on a read field"):** `ReadSetFilter` is missing
`CreatedAfter`/`CreatedBefore`/`CreationType`/`GeneratedFrom`/`ReferenceArn`/
`SampleId`/`SubjectId`; `ReferenceFilter` is missing `CreatedAfter`/
`CreatedBefore`/`Md5`; `ReferenceStoreFilter`/`SequenceStoreFilter` are
missing `CreatedAfter`/`CreatedBefore` (and `SequenceStoreFilter` also
`UpdatedAfter`/`UpdatedBefore`); `StartRunInput`'s own `RetentionMode`
("default value is RETAIN"), `ScratchStorageMode` ("default to SHARED"), and
`StorageCapacity` ("Defaults to 1200 GiB") are never read by `StartRun` at
all (distinct from `RunBatch.DefaultRunSetting`'s identical, already-disclosed
gap for the same three fields) — a bug requires the field to be read first;
these aren't.

**Left open, correctly, as unfabricatable rather than fixed or fabricated:**
`CreateWorkflowInput.Engine`'s doc comment ("By default, Amazon Web Services
HealthOmics detects the engine automatically from your workflow definition")
describes content-based auto-detection from a real zip archive, which this
backend cannot honestly simulate without parsing workflow definition files —
left empty on omission rather than guessing a value, the same restraint this
issue's brief asks for on `PatchOrchestratorFilter`-shaped traps.

No web pages fetched — everything resolved from the pinned SDK module cache
(`aws-sdk-go-v2/service/omics@v1.49.5`).

Gates: `go build ./services/omics/... ./services/docdb/...`, `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/omics/...`,
`golangci-lint run ./services/omics/...` (0 issues).

## 2026-08-31 (gopherstack-4glf, never-declared-field sweep, `cmd/reqfielddiff`)

`go run ./cmd/reqfielddiff -dir omics` reported 25 tier-1 findings
("documented default"). 19 of 25 fixed; 6 recorded as unmodellable/false
positive with reasoning below. This pass's own earlier 2026-08-31 entry
(value-semantics sweep) had already found and recorded `StartRun`'s
`RetentionMode`/`ScratchStorageMode`/`StorageCapacity` as never read at all,
explicitly deferring them to "the request-field-never-read axis, not this
one" -- this pass is that deferred axis, and closes all three.

**Fixed (19):**

- **`StartRun.RetentionMode`/`.ScratchStorageMode`/`.StorageType`/
  `.WorkflowType`** -- each has an unambiguous documented default (RETAIN,
  SHARED, STATIC, PRIVATE respectively). None was declared anywhere in this
  backend. Added all four to `Run`/`StartRunInput`, defaulted in the new
  `startRunDefaults` helper, echoed via `GetRun`/`ListRuns` (verified against
  `GetRunOutput`'s own field list -- these four are GetRun-only, not on
  `StartRunOutput`). REMOVE's automatic old-run eviction and READY2RUN's
  public workflow catalog are NOT implemented; both are disclosed as
  structural gaps in the field's own doc comment on `Run`, not silently
  claimed. RETAIN is honest regardless, since it already describes this
  backend's actual behavior (never auto-removing runs).
- **`StartRun.StorageCapacity`** -- own doc comment: "The default run storage
  capacity is 1200 GiB." Defaults to 1200 only when `StorageType` resolves to
  STATIC -- real AWS's own doc states DYNAMIC "ignores any value that you
  enter," so fabricating a capacity number for DYNAMIC storage would claim
  something untrue; none is set in that case (`Run.StorageCapacity` is
  `*int`, nil when inapplicable).
- **`StartRun.WorkflowVersionName`** -- entirely undeclared; explicit values
  are now stored/echoed. No "default version" is fabricated on omission:
  real AWS lets a workflow designate one version as default and falls back
  to it, but this backend has no such designation mechanism at all (no
  `CreateWorkflowVersion`/`UpdateWorkflowVersion` field sets a version as
  default) -- inventing a "first version created" or similar substitute
  would be exactly the kind of guessed behavior this campaign's guidance
  warns against, so omission leaves the field empty, same as today.
- **`StartRun.CacheBehavior`** (plus the previously-tier5, not independently
  targeted, but necessarily-paired `CacheId`) -- own doc comment: "You
  specify this value if you want to override the default behavior for the
  cache. You had set the default value when you created the cache." Added
  `Run.CacheID`/`.CacheBehavior`; when `CacheID` is given and `CacheBehavior`
  is not, the referenced cache's own `CacheBehavior` is looked up and used
  (`startRunDefaults`). `CacheId` was necessary scaffolding, not scope creep:
  `CacheBehavior` is meaningless without it, and it was undeclared too.
  Actual task-output caching (skipping re-execution of a previously
  cache-hit task) is NOT simulated -- this backend has no task-execution
  graph to apply it to (a single stub task per run) -- so, like ECS's
  `Monitoring` fix, this is a stored/echoed preference, matching real AWS's
  own API contract (real AWS never exposes cache-hit/-miss as a distinct API
  response either; it is purely CloudWatch/execution-internal).
- **`CreateRunCache.CacheBehavior`** -- own doc comment: "If you don't
  specify a value, the default behavior is CACHE_ON_FAILURE." Added
  `RunCache.CacheBehavior`, defaulted on create.
- **`UpdateRunCache.CacheBehavior`** -- own doc comment: "Update the default
  run cache behavior" (no omission-default stated -- PATCH semantics: applied
  only when non-empty, like `Name`/`Description` already were).
- **`CreateWorkflow`/`CreateWorkflowVersion.ParameterTemplate`** -- own doc
  comment: blank means auto-parse from the workflow definition file, which
  this backend cannot honestly simulate without parsing a real archive (same
  restraint as `Engine`'s auto-detection, recorded 2026-08-31 above) --
  NOT implemented, left empty on omission. But an EXPLICITLY supplied
  template is a different question: it's a plain structured value handed
  directly on the wire, no parsing required, and it was being silently
  dropped regardless of whether the caller provided one. Added
  `Workflow`/`WorkflowVersion.ParameterTemplate` (new `WorkflowParameter`
  type mirroring `types.WorkflowParameter`), stored/echoed only when
  non-blank.
- **`CreateWorkflow`/`CreateWorkflowVersion`/`UpdateWorkflow`/
  `UpdateWorkflowVersion.StorageCapacity`/`.StorageType`** -- reqfielddiff's
  "documented default" tier flagged these on the strength of the word
  "default" appearing in their doc comments, but re-reading each doc comment
  closely: none of them states what happens when the field ITSELF is
  omitted at create/update time -- they describe what the value means for
  runs that inherit it ("The default static storage capacity ... for runs
  that use this workflow"), which is a different claim. This is the same
  heuristic false-positive shape as ECS's `ServiceConnectDefaults` (see
  2026-08-31 ecs sweep). Unlike `StartRun`'s own `StorageCapacity`/
  `StorageType` (which DO state a fixed default for their own omission --
  "By default, the run uses STATIC storage type"), no numeric/enum default
  is fabricated here: the fields are declared and stored/echoed exactly as
  given, nil/empty when omitted. Added `StorageCapacity *int`/`StorageType
  string` to `Workflow`/`WorkflowVersion`, threaded through new
  `CreateWorkflowInput`/`CreateWorkflowVersionInput` structs (the backend
  signatures were already gaining 3 new fields each; a positional-parameter
  refactor was overdue) and two new `UpdateWorkflow`/`UpdateWorkflowVersion`
  parameters (PATCH semantics: applied only when non-zero/non-nil).

**Recorded as unmodellable or false positive, not fixed (6):**

- **`CreateWorkflow`/`CreateWorkflowVersion.ParameterTemplatePath`** -- own
  doc comment: "The path to the workflow parameter template JSON file
  *within the repository*." This field only means something when the
  workflow is created from a source-code repository
  (`DefinitionRepository`), which reqfielddiff's own tier-5 list already
  flags as unmodeled in this backend (no strong signal, structural gap) --
  this backend never clones or reads a repository. Storing a bare path
  string with nothing to resolve it against would be inert config with no
  observable meaning; recorded rather than fabricated.
- **`CreateWorkflow`/`CreateWorkflowVersion.ReadmePath`** -- same reasoning
  as `ParameterTemplatePath`: "The path to the workflow README markdown file
  *within the repository*." `ReadmePath` IS present on `GetWorkflowOutput`
  (verified -- unlike `ParameterTemplatePath`, which appears nowhere on any
  Get* output), but its only documented meaning is still repository-relative
  path resolution this backend cannot perform; echoing a string with no
  connection to any actual README content would misrepresent what the field
  does.
- **`CreateWorkflow.WorkflowBucketOwnerId`** -- own doc comment: "the
  expected owner of the S3 bucket that contains the workflow definition. If
  not specified, the service skips the validation." This is a pure
  validation-gating field with no wire-visible echo anywhere (absent from
  `GetWorkflowOutput`, confirmed) -- honoring it would require real S3
  bucket-ownership verification, which this backend cannot perform (it
  already discards `DefinitionZip`/`DefinitionURI` entirely, a pre-existing
  disclosed simplification). Since no such validation exists regardless of
  this field's value, storing it would create a field with no effect and no
  visible echo -- pure inert config, not worth a wire slot.
- **`ListBatch.MaxItems`** -- re-verified against source instead of trusted:
  this field IS already correctly modeled. `batchQueryParams` (handler.go)
  reads the real "maxItems" query key (NOT "maxResults" -- verified against
  `serializers.go`'s `encoder.SetQuery("maxItems")`), and `paginateStrings`
  (store.go) already applies the documented default of 100
  (`maxPageSize = 100`) whenever `maxResults <= 0`. This service's own prior
  PARITY entry (2026-08-31, value-semantics sweep) already stated this
  explicitly: "batchQueryParams + the shared maxPageSize = 100 cap/default
  already match it exactly." reqfielddiff's declared-field enumeration
  doesn't recognize a raw `q.Get("maxItems")` read inside a shared helper as
  a "declared field" for this specific operation, which is why it still
  flagged tier-1 despite the behavior being correct and already verified --
  a genuine detector blind spot, not a bug. No code change.

**Ratio and what it says about detector precision:** 19/25 (76%) of this
service's tier-1 findings were genuinely fixable, higher than ecs's true
positive rate would suggest in isolation but consistent with the campaign's
overall experience that "documented default" is the tier most worth mining
-- most of omics's misses here were the SAME two shapes already seen
elsewhere (repository-relative paths tied to an unmodeled feature; a
heuristic matching the word "default" in unrelated prose) rather than novel
failure modes, which suggests the detector's false-positive surface is
narrow and identifiable rather than diffuse.

**Where the fix belongs:** every fixed field here landed at the BACKEND
layer (`Run`/`RunCache`/`Workflow`/`WorkflowVersion` models, populated inside
`startRunLocked`/`CreateRunCache`/`CreateWorkflow`/`CreateWorkflowVersion`),
beneath both the input-decode and response-encode wire structs, matching the
lesson from the `StartRun.NetworkingMode` fix earlier this campaign: several
of these fields (e.g. `WorkflowType`, `RetentionMode`) are read on `GetRun`
by a DIFFERENT handler (`handleGetRun`) than the one that creates the run
(`handleStartRun`) -- defaulting inside the wire layer would have required
duplicating the same default in two unrelated handler files, and getting one
of them right while missing the other is exactly the two-response-shapes
trap the earlier entry describes.

**Backend signature changes (interface + all in-repo callers updated,
verified via `go build ./...`/`go vet ./...` repo-wide -- no other service
calls into any of these):**
`StartRun(workflowID, roleARN, ..., tags) (*Run, error)` ->
`StartRun(StartRunInput) (*Run, error)`;
`CreateRunCache(name, cacheS3Location, tags)` ->
`CreateRunCache(name, cacheS3Location, cacheBehavior, tags)`;
`UpdateRunCache(id, name, description)` ->
`UpdateRunCache(id, name, description, cacheBehavior)`;
`CreateWorkflow(name, description, definitionZip, definitionURI, engine, tags)`
-> `CreateWorkflow(CreateWorkflowInput)`;
`UpdateWorkflow(id, name, description)` ->
`UpdateWorkflow(id, name, description, storageType, storageCapacity)`;
`CreateWorkflowVersion(workflowID, versionName, description, tags)` ->
`CreateWorkflowVersion(CreateWorkflowVersionInput)`;
`UpdateWorkflowVersion(workflowID, versionName, description)` ->
`UpdateWorkflowVersion(workflowID, versionName, description, storageType, storageCapacity)`.
Two internal test call sites (`persistence_test.go`) updated to match; no
assertions dropped, only call syntax.

New tests: `wire_field_additions_omicssweep_test.go`, all driving the real
`aws-sdk-go-v2/service/omics` client. Every default-value test (`StartRun`
five-defaults test, `CreateRunCache` default) omits the field entirely. The
`CacheBehavior`-inherits-from-cache test seeds two caches with different
`CacheBehavior` values (one on each side of the distinction) so it can tell
"inherited the referenced cache's default" apart from "picked some fixed
default." Confirmed failing pre-fix by temporarily reverting
`startRunDefaults` to only its pre-existing `NetworkingMode` line and
`CreateRunCache`'s default-fill (not by removing the new struct fields,
since most fields did not exist before this pass and removing them fails
the whole package to compile rather than demonstrate a behavioural gap):
all three targeted tests (`StartRun` defaults, `StartRun` cache-inherits,
`CreateRunCache` default) reproduced their expected pre-fix failures, then
were restored byte-identical (`md5sum`-verified) and re-confirmed green.
Assertion count: 0 existing assertions changed or dropped; all new.

Gates: `go build ./services/omics/...`, `go vet ./services/omics/...` (both
clean), `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/omics/...` (pass), `golangci-lint run ./services/omics/...` (0
issues, `golangci-lint run --fix` used once for fieldalignment on the new
structs, re-verified with plain `run` afterward). Work left uncommitted per
this pass's instructions.
