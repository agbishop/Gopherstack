---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: fsx
sdk_module: aws-sdk-go-v2/service/fsx@v1.68.4   # version audited against
last_audit_commit: 8d4556e7938635cdf7c945d46cea23d9dbe03cb9
last_audit_date: 2026-08-29
overall: A            # genuine wire-format + error-code bugs found and fixed
                      # 2026-08-29 (constraint-not-honoured sweep, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch):
                      # every Describe* op whose real Input struct declares a Filters member had NO field for it
                      # at all in gopherstack's request struct -- bug class 1 ("never read"), not a wrong-key
                      # miswire. 7 ops affected: DescribeBackups (file-system-id/backup-type/file-system-type;
                      # volume-id left as a disclosed gap, see below), DescribeDataRepositoryAssociations
                      # (file-system-id only -- the shared types.Filter/FilterName enum's other 6 values don't
                      # apply to a DRA), DescribeDataRepositoryTasks (file-system-id/task-lifecycle;
                      # data-repository-association-id/file-cache-id left as disclosed gaps),
                      # DescribeSnapshots (file-system-id/volume-id; IncludeShared not modeled, see below),
                      # DescribeVolumes (file-system-id/storage-virtual-machine-id, both supported),
                      # DescribeStorageVirtualMachines (file-system-id, its only real filter name), and
                      # DescribeS3AccessPointAttachments (file-system-id/volume-id/type, all supported).
                      # DescribeFileCaches/DescribeFileSystems confirmed clean -- neither op's real Input
                      # declares a Filters member at all (field-diffed against fsx@v1.68.4 api_op_*.go), so
                      # there was nothing to miss. Every filter's semantics taken from its own SDK enum
                      # (types.FilterName/SnapshotFilterName/VolumeFilterName/StorageVirtualMachineFilterName/
                      # DataRepositoryTaskFilterName/S3AccessPointAttachmentsFilterName in types/enums.go), not
                      # invented. Shared {Name,Values} decode + AND-across-filters/OR-within-filter matching
                      # logic in filters.go (matchesFilters); an unrecognized filter Name for a given op is
                      # treated as unsupported-and-ignored (matches everything), same as an unset filter --
                      # never rejected, since AWS doesn't reject an unsupported filter name either. All 7
                      # BackupIds/AssociationIds/TaskIds/SnapshotIds/VolumeIds/StorageVirtualMachineIds/Names
                      # ID-list params continue to override Filters entirely when both are set, per each op's
                      # own doc comment (pre-existing branch structure, unchanged). Every fix proven via
                      # wire_field_fixes_test.go driving the real typed aws-sdk-go-v2/service/fsx client,
                      # asserting a non-matching resource is excluded (not just that a matching one is
                      # present) -- confirmed failing against unmodified code first. Disclosed, not fixed
                      # (no honest tracked data to filter on -- see gaps): DescribeBackups' volume-id
                      # (CreateBackup never accepts a VolumeId to back an ONTAP-volume backup, though real
                      # CreateBackupInput has one -- an adjacent create-side gap, out of this filter-class
                      # pass's scope, reported not fixed); DescribeDataRepositoryTasks'
                      # data-repository-association-id/file-cache-id (CreateDataRepositoryTask has no field
                      # for either); DescribeSnapshots' IncludeShared (this backend is single-account/
                      # single-tenant, so every snapshot is definitionally "owned" -- no cross-account
                      # snapshot exists to differ on, structurally unobservable, not merely unimplemented).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
families:
  FileSystem: {wire: ok, errors: ok, state: ok, persist: ok, note: "Fixed this pass: CreateFileSystem/UpdateFileSystem now accept and CreateFileSystem/DescribeFileSystems/UpdateFileSystem/CreateFileSystemFromBackup now return real WindowsConfiguration/OntapConfiguration/OpenZFSConfiguration blocks (previously only LustreConfiguration was ever modeled). Windows requires WindowsConfiguration.ThroughputCapacity; ONTAP requires OntapConfiguration.DeploymentType + (ThroughputCapacity or ThroughputCapacityPerHAPair); OpenZFS requires OpenZFSConfiguration.DeploymentType + ThroughputCapacity -- an absent config block on these three types returns MissingFileSystemConfiguration, a present-but-incomplete block returns BadRequest, matching real AWS's required-member validation (field-diffed against CreateFileSystemWindowsConfiguration/CreateFileSystemOntapConfiguration/CreateFileSystemOpenZFSConfiguration in types/types.go). OpenZFS file systems now get a real, describable RootVolumeId (a genuine storedVolume row is created, not a disguised placeholder string) matching AWS auto-creating a root volume per OpenZFS file system. CreateFileSystemFromBackup now carries the source file system's type-specific config fields (ThroughputCapacity, DeploymentType, etc.) onto the restored file system instead of returning an all-zero-valued config block, and now sets DNSName (previously left empty). DeleteFileSystem now cascades to child StorageVirtualMachines/Volumes/Snapshots/DataRepositoryAssociations (see leaks note) -- previously only removed the file system + its own tags. UpdateFileSystem applies WindowsConfiguration/OntapConfiguration/OpenZFSConfiguration update sub-blocks (ThroughputCapacity, backup schedule fields, HAPairs) with real AWS's 'only overwrites non-null values' semantics. CreationTime already used epochTime pre-audit (correct). FIXED THIS PASS (gopherstack-wjjl): CreateFileSystem now implements real ClientRequestToken idempotency -- createFileSystemInput gained the field (previously entirely absent, so a retried create silently made a second resource); a repeat call with the same token and identical parameters now returns the ORIGINAL FileSystem (verified via FileSystemId/ResourceARN/CreationTime equality + unchanged resource count, not merely 'no error'); a repeat call with the same token but different parameters returns IncompatibleParameterError (new sentinel, field-diffed against types/errors.go), matching real AWS's documented CreateFileSystem contract verbatim. Dedup state (createFileSystemTokens) is a plain map guarded by the same coarse b.mu as the fileSystems table (the token check-then-set must be atomic with the resource write) and is now part of backendSnapshot (fsxSnapshotVersion bumped 1->2), so it survives Snapshot/Restore -- proven in TestInMemoryBackend_SnapshotRestore_FullState. ALSO FIXED THIS PASS: SubnetIds/SecurityGroupIds, when supplied to CreateFileSystem, are now format-validated against the real ID patterns from the API reference (subnet-[0-9a-f]{8,} / sg-[0-9a-f]{8,}) and rejected with the real InvalidNetworkSettings exception if malformed. This is real-format validation only, not existence/topology validation -- see gaps below for what's still not covered (SubnetIds required-ness, AZ-count-per-deployment-type rules). FIXED (gopherstack-cgq3) — CreateFileSystemFromBackup was missing the real optional FileSystemTypeVersion field (*string, the Lustre engine-version override; per api_op_CreateFileSystemFromBackup.go, real AWS lets a restore specify a newer Lustre version than the backup's own setting, defaulting to the backup's if omitted). Now modeled on FileSystem/storedFileSystem and threaded through: an explicit request value wins, otherwise it falls back to the source file system's own FileSystemTypeVersion. Note CreateFileSystem (the non-backup create path) still has no way to set FileSystemTypeVersion at all, so that fallback is currently always empty in practice — a related, distinct gap (real CreateFileSystemInput also has this field) left unfixed since it's out of this op's scope; see gaps: below."}
  Backup: {wire: ok, errors: ok, state: ok, persist: ok, note: "Create/Describe/Delete/Copy + CreateFileSystemFromBackup verified against real BackupId/FileSystemId shapes. CreationTime already epochTime pre-audit. Confirmed this pass: DeleteFileSystem does NOT cascade-delete backups, matching real AWS (backups persist independently of their source file system). 2026-09-08 (gopherstack-u7rl): BackupInProgress/BackupRestoring/BackupBeingCopied (DeleteBackup's three lifecycle-precondition errors) reconfirmed structurally unobservable, not a bug -- see Notes. Found and fixed instead, same pass: CreateBackup/CopyBackup accepted more than 50 Tags (the shared real \"Tags\" shape's own max:50, never checked -- see Notes)."}
  FileSystemAliases: {wire: ok, errors: ok, state: ok, persist: ok, note: "Associate/Disassociate/Describe verified; insertion-order preserved via plain map+slice (documented in store_setup.go), matches DescribeFileSystemAliases pagination expectations. DeleteFileSystem now clears aliases[fileSystemID] on delete (fixed this pass; see leaks note)."}
  DataRepositoryAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreationTime wire bug fixed in a prior pass. Tag storage + arnExists coverage fixed in a prior sweep. Fixed this pass: DeleteFileSystem now cascade-deletes DRAs belonging to the deleted file system (previously left as ghost rows; see leaks note)."}
  DataRepositoryTask: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "CreationTime wire bug fixed in a prior pass. Cancel/Create/Describe verified; Lifecycle EXECUTING/CANCELING matches real enum values. Intentionally NOT cascade-deleted on DeleteFileSystem: DataRepositoryTasks are historical execution records in real AWS, not live child resources. FIXED this pass (gopherstack-4ggy): Report (a required CreateDataRepositoryTaskInput member, api_op_CreateDataRepositoryTask.go:49-129, whose own Enabled member is required per validateCompletionReport) was dropped entirely -- the request read only FileSystemId/Type/Paths/Tags. Now required, validated, stored, and echoed back on DataRepositoryTask.Report (the real DescribeDataRepositoryTasks/CreateDataRepositoryTask response member); Format/Path/Scope accepted but not enforced, matching the SDK's own client-side validator (only Enabled is checked there, despite the doc comment saying the other three are 'required if Enabled is true')."}
  FileCache: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "CreationTime wire bug fixed in a prior pass. Create/Delete/Describe/Update verified against FileCacheId/FileCacheType shapes. errValidation's wire code fixed this pass (see Misc/global note below) -- FileCache's own ErrValidation-based rejections (missing FileCacheType) now correctly return BadRequest instead of the non-existent 'ValidationError'. FIXED 2026-08-11 -- CreateFileCache's request/response StorageCapacity field was wire-tagged StorageCapacityGiB; the real CreateFileCacheRequest/FileCache field is StorageCapacity, so every real client's capacity value was silently discarded (created caches always got 0 GiB). UpdateFileCache's StorageCapacityGiB acceptance is untouched -- the real UpdateFileCacheRequest has no storage-capacity field at all (out of scope, pre-existing invented field, not a rename target). FIXED this pass (gopherstack-4ggy): FileCacheTypeVersion (named in the issue) AND SubnetIds (also a required CreateFileCacheInput member, api_op_CreateFileCache.go:48-124, equally absent -- floor confirmed) were both dropped entirely; StorageCapacity was wired but never required-checked (also fixed, same required set). All three now validated and echoed back on FileCache.FileCacheTypeVersion/SubnetIds (types.FileCacheCreating, types.go:2349). FIXED 2026-08-20 (wrapper-key sweep): a single FileCache Go type, WITH a Tags field, was reused for CreateFileCache/DescribeFileCaches/UpdateFileCache responses alike. Real AWS splits these into two distinct wire types -- types.FileCacheCreating (types/types.go:2349, HAS Tags; deserializers.go:9984 case \"Tags\") for CreateFileCacheOutput.FileCache only, vs types.FileCache (types/types.go:2264, NO Tags at all; deserializers.go:9818 has no case \"Tags\") for DescribeFileCachesOutput.FileCaches/UpdateFileCacheOutput.FileCache -- so gopherstack emitting a Tags key on Describe/Update responses was a fabricated member with no case in the live deserializer, silently dropped by a real client (harmlessly, since the real Go type has no field to hold it, but still wire-inaccurate). Split into FileCacheCreating (interfaces.go, Tags) and FileCache (interfaces.go, no Tags); CreateFileCache's backend method now returns *FileCacheCreating via toPublicCreating(), Describe/UpdateFileCache keep *FileCache via toPublic(). Proven by services/fsx/file_cache_wire_test.go (TestFileCache_TagsWireShape), hand-revert confirmed the exact predicted symptom (Tags key present on Describe/Update)."}
  Snapshot: {wire: ok, errors: ok, state: fixed, persist: ok, note: "CreationTime wire bug fixed in a prior pass. Create/Delete/Describe/Update verified; RestoreVolumeFromSnapshot correctly validates volume+snapshot existence before returning (real read+validate, not a disguised no-op). Fixed this pass: DeleteVolume and DeleteStorageVirtualMachine (transitively) now cascade-delete a volume's snapshots (previously left as ghost rows pointing at a deleted VolumeId; see leaks note). errValidation's wire code fixed this pass (see Misc/global note). FIXED 2026-08-20 (wrapper-key sweep, critical): CopySnapshotAndUpdateVolume's response was wrapped under a fabricated \"Volume\" key ({Volume: *Volume}). Real AWS's CopySnapshotAndUpdateVolumeOutput (api_op_CopySnapshotAndUpdateVolume.go:87) has NO Volume member at all -- it wraps under root-level \"Lifecycle\"/\"VolumeId\" plus \"AdministrativeActions\" (a list of the new AdministrativeAction type, TargetVolumeValues nested), confirmed via deserializers.go:15903's live per-op switch (no case \"Volume\"). A real client got a completely empty CopySnapshotAndUpdateVolumeOutput back (VolumeId/Lifecycle empty, AdministrativeActions nil) -- total data loss, not a dropped field. See Volume family for the paired RestoreVolumeFromSnapshot bug (identical pattern, shared fix). Proven by services/fsx/administrative_action_wire_test.go; two PRE-EXISTING tests that had encoded the wrong key as correct (handler_snapshots_test.go asserting out[\"Volume\"], handler_volumes_test.go same) were corrected to assert the real shape. FIXED 2026-08-29 (write-only-state sweep): the 08-20 note above claimed CopySnapshotAndUpdateVolume 'correctly validates volume+snapshot existence' -- this pass's write-only-state method (primary method: what's accepted from a request and never read?) found that claim was WRONG for the snapshot half. SourceSnapshotARN (a required real CopySnapshotAndUpdateVolumeInput member, api_op_CopySnapshotAndUpdateVolume.go) was decoded off the wire into copySnapshotAndUpdateVolumeInput.SourceSnapshotID but never referenced anywhere else in the package (grep-confirmed zero other reads) -- any ARN, including one naming a nonexistent or malformed snapshot, silently 'succeeded'. Fixed: extracts the snapshot ID from the ARN's trailing snapshot/<id> segment (matching the format snapshotARN itself builds) and existence-checks it, returning SnapshotNotFound like the sibling RestoreVolumeFromSnapshot op already correctly did for its own (non-ARN) SnapshotId parameter. Proven by wire_field_fixes_test.go's TestCopySnapshotAndUpdateVolume_SourceSnapshotARNValidated (real client, hand-reverted, confirmed failing pre-fix, restored md5sum-identical)."}
  StorageVirtualMachine: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreationTime wire bug fixed in a prior pass. Create requires FileSystemId (matches real required-parameter behavior); Subtype/RootVolumeSecurityStyle round-trip. Fixed this pass: DeleteStorageVirtualMachine now cascade-deletes the volumes hosted on that SVM (and, transitively, those volumes' snapshots); DeleteFileSystem now cascade-deletes SVMs belonging to the deleted file system. errValidation's wire code fixed this pass (see Misc/global note). ActiveDirectoryConfiguration/Endpoints (SvmEndpoints/SvmEndpoint) are genuine real SDK members never emitted at all (types/types.go, deserializers.go:14651 case list confirmed) -- Layer 3 gap, out of scope this pass (not hunted, not fixed)."}
  Volume: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "CreationTime wire bug fixed in a prior pass. Create/CreateFromBackup/Delete/Describe/Update verified. Fixed this pass: DeleteVolume now cascade-deletes that volume's snapshots; DeleteFileSystem/DeleteStorageVirtualMachine now cascade-delete volumes belonging to the deleted file system/SVM. errValidation's wire code fixed this pass (see Misc/global note). FIXED 2026-08-20 (wrapper-key sweep, critical): RestoreVolumeFromSnapshot's response was wrapped under a fabricated \"Volume\" key, exactly mirroring CopySnapshotAndUpdateVolume's bug (see Snapshot family for full citation) -- real RestoreVolumeFromSnapshotOutput (api_op_RestoreVolumeFromSnapshot.go) also has no Volume member, only root-level Lifecycle/VolumeId + AdministrativeActions (deserializers.go:17381 live switch, no case \"Volume\"). Added AdministrativeAction (interfaces.go) reusing the existing Volume type for TargetVolumeValues (matches real types.AdministrativeAction.TargetVolumeValues *Volume, types/types.go:185) so no backend logic changed, only the handler's response wrapping. AdministrativeActionType values used (VOLUME_RESTORE for Restore, VOLUME_UPDATE_WITH_SNAPSHOT for Copy) are exact matches against types/enums.go, Status COMPLETED likewise. FIXED 2026-08-23 (gopherstack batch8, request-side): CreateVolume's INPUT was reading gopherstack-invented top-level FileSystemId/StorageVirtualMachineId fields real CreateVolumeInput has never had at all (api_op_CreateVolume.go) -- a real client's SVM/parent-volume reference was silently ignored, producing a volume with an empty FileSystemId and no real StorageVirtualMachine association, no error either way. Now reads the real nested OntapConfiguration.StorageVirtualMachineId (ONTAP, existence-checked, FileSystemId derived from the resolved SVM) / OpenZFSConfiguration.ParentVolumeId (OPENZFS, existence-checked, FileSystemId derived from the resolved parent volume), and rejects a VolumeType=ONTAP/OPENZFS request with no matching config block as MissingVolumeConfiguration (types.MissingVolumeConfiguration, real wire code, fsx@v1.68.4 types/errors.go) -- mirrors CreateFileSystem's already-established per-type-config-block-required pattern (see FileSystem family). See Notes for proof/hand-revert. FIXED 2026-08-29 (write-only-state sweep, response-side): the 2026-08-20/08-23 passes both explicitly disclosed 'Volume has no OntapVolumeConfiguration at all' as a Layer-3 gap and left it there -- but re-reading the live deserializer (deserializers.go:15307's Volume case switch) this pass found gopherstack was NOT simply omitting the SVM: it was emitting StorageVirtualMachineId as a FABRICATED TOP-LEVEL key with no counterpart on real types.Volume at all (the real member is OntapConfiguration.StorageVirtualMachineId, deserializers.go:12447). A real typed client silently drops the top-level key and gets nil OntapConfiguration -- so even after 08-23 fixed CreateVolume's *request*-side SVM resolution, the resolved SVM remained completely unreadable through every op returning a Volume (CreateVolume, CreateVolumeFromBackup, DescribeVolumes, UpdateVolume, and the AdministrativeAction.TargetVolumeValues nested Volume on RestoreVolumeFromSnapshot/CopySnapshotAndUpdateVolume). Added OntapVolumeConfiguration{StorageVirtualMachineId} (interfaces.go, only this one real member modeled, matching this fix's scope); storedVolume.toPublic() now nests it under Volume.OntapConfiguration for ONTAP volumes (OpenZFS volumes correctly get no OntapConfiguration -- OpenZFS has no SVM concept). Also fixed CreateVolumeFromBackup (same sweep): its request struct had a flat top-level StorageVirtualMachineId, exactly the same accept-and-drop bug the 08-23 pass fixed on CreateVolume itself -- real CreateVolumeFromBackupInput (api_op_CreateVolumeFromBackup.go) has no top-level VolumeType or StorageVirtualMachineId at all, only nested OntapConfiguration.StorageVirtualMachineId (types.CreateOntapVolumeConfiguration); no real client's SVM assignment could ever have reached this backend. Now resolves the same createOntapVolumeConfigInput type CreateVolume already uses, existence-checks the SVM, derives FileSystemId from it, and rejects a request with no OntapConfiguration as MissingVolumeConfiguration. Proven by wire_field_fixes_test.go's TestVolume_StorageVirtualMachineIdWireShape and TestCreateVolumeFromBackup_StorageVirtualMachineIdRoundTrip (real aws-sdk-go-v2 client round trips); both hand-reverted (git checkout -- the touched files, confirmed all four new round-trip tests fail with the predicted symptom, restored, md5sum byte-identical)."}
  S3AccessPoint: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "CreationTime wire bug fixed in a prior pass. errValidation's wire code fixed this pass (see Misc/global note). FIXED 2026-08-20 (wrapper-key sweep, most severe bug this pass): the ENTIRE S3AccessPoint feature modeled the wrong AWS type on both request and response. gopherstack's old flat S3AccessPoint{Name,FileSystemID,VolumeID,Lifecycle,ResourceARN,Tags,CreationTime} does not correspond to any real FSx wire shape -- CreateAndAttachS3AccessPointOutput/DescribeS3AccessPointAttachmentsOutput actually wrap under \"S3AccessPointAttachment\"/\"S3AccessPointAttachments\" (types.S3AccessPointAttachment, types/types.go:3898; deserializers.go:15957/16995 live switches confirm no case \"S3AccessPoint\"/\"S3AccessPoints\" exists), whose real case list is CreationTime/Lifecycle/LifecycleTransitionReason/Name/OntapConfiguration/OpenZFSConfiguration/S3AccessPoint/Type -- NO top-level FileSystemId, VolumeId, ResourceARN, or Tags at all. The attached VolumeId lives nested under whichever of OntapConfiguration/OpenZFSConfiguration (types/types.go:3956/3970) matches Type, and ResourceARN/Alias live under a DIFFERENT nested type, types.S3AccessPoint (deserializers.go:13775, case list Alias/ResourceARN/VpcConfiguration only). The real request side is equally different: CreateAndAttachS3AccessPointInput has no FileSystemId member at all (api_op_CreateAndAttachS3AccessPoint.go:52) -- Name+Type+OntapConfiguration.VolumeId|OpenZFSConfiguration.VolumeId is the real contract -- and DetachAndDeleteS3AccessPointInput has no FileSystemId either (api_op_DetachAndDeleteS3AccessPoint.go:36, Name alone). Before this fix a real typed SDK client's CreateAndAttachS3AccessPoint call sent the real (Name/Type/OntapConfiguration) shape and gopherstack's old handler, which required FileSystemId, rejected it outright with 400 BadRequest -- the op was non-functional against a real client. Rebuilt: S3AccessPointAttachment/S3AccessPointOntapConfiguration/S3AccessPointOpenZFSConfiguration/S3AccessPoint (interfaces.go), createAndAttachS3AccessPointInput now parses Type+nested VolumeId (s3_access_points.go), DetachAndDeleteS3AccessPoint(name string) dropped the fileSystemID parameter, Tags support removed from Create input (real AWS has none there -- also independently confirmed by the pre-existing exclusion note in handler_create_tags_test.go). A synthetic Alias is generated (generateS3AccessPointAlias) since AWS's real alias-hashing algorithm is undocumented -- a plausible stand-in, not a byte-exact reproduction. Proven by services/fsx/s3_access_point_wire_test.go via a real typed SDK client round-trip; hand-revert reproduced a nil S3AccessPointAttachment. Three PRE-EXISTING tests that had encoded the wrong contract as correct (handler_s3_access_points_test.go x2, handler_test.go's Test_CreationTime_IsEpochSecondsNumber/S3AccessPoint case, persistence_test.go's createS3AP helper) were corrected."}
  SharedVpcConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "Describe/Update verified; single scalar field, not a collection, so untouched by the store.Table refactor per store_setup.go."}
  Misc: {wire: ok, errors: ok, state: ok, persist: n/a, note: "ReleaseFileSystemNfsV3Locks and StartMisconfiguredStateRecovery both validate FileSystemId existence against real state (not disguised no-ops) and echo the file system back; neither op has persisted side effects in real AWS beyond a transient Lifecycle flicker, which this synchronous emulator does not model (consistent with the immediate-AVAILABLE pattern used for every other resource in this service). GLOBAL FIX this pass: errValidation's wire code was 'ValidationError', which is not a real FSx exception (field-diffed against types/errors.go -- FSx's generic client-error type is BadRequest; there is no ValidationError type at all). Every op across every family that returns ErrValidation (CreateFileSystem, CreateSnapshot, CreateStorageVirtualMachine, CreateVolume, CreateAndAttachS3AccessPoint, CreateFileCache) now correctly returns BadRequest. Added ErrMissingFileSystemConfiguration (wire code MissingFileSystemConfiguration) for CreateFileSystem's new required-config-block validation."}
  Tags: {wire: ok, errors: ok, state: ok, persist: ok, note: "TagResource/UntagResource/ListTagsForResource error code fixed in a prior pass: unrecognized ARNs return the generic ResourceNotFound exception. ListTagsForResource already returned [] not null for empty tag sets."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "DescribeBackups' documented volume-id filter (real DescribeBackupsInput.Filters, backup-type ONTAP/OpenZFS volume backups) has no honest value to filter on: CreateBackup never accepts a VolumeId at all, even though real CreateBackupInput has one (api_op_CreateBackup.go) -- an adjacent create-side accept-and-drop gap, out of the 2026-08-29 constraint-not-honoured pass's filter-only scope. A request setting this filter matches every backup rather than excluding any, same as AWS treating an unset filter."
  - "DescribeDataRepositoryTasks' documented data-repository-association-id/file-cache-id filters have no honest value to filter on: CreateDataRepositoryTaskInput accepts neither an association nor a file-cache reference to track (only FileSystemId), even though the real DataRepositoryTaskFilterName enum documents both. Both filters match everything rather than excluding, same as AWS treating an unset filter."
  - "DescribeSnapshots' IncludeShared (real DescribeSnapshotsInput member) is not modeled: this backend is single-account/single-tenant, so every snapshot is definitionally \"owned\" by the caller regardless of that flag -- there is no cross-account snapshot for it to differ on, a structural gap rather than an unimplemented one."
  - "FIXED 2026-08-29 (write-only-state sweep): CreateFileSystemFromBackup had no SubnetIds field at all -- SubnetIds is a required real CreateFileSystemFromBackupInput member (api_op_CreateFileSystemFromBackup.go) that every real client's SDK-side validator forces it to send, and it round-trips onto FileSystem.SubnetIds on every other file-system create path (CreateFileSystem already accepts/echoes it). It was being silently discarded: the restored file system always came back with empty SubnetIds/NetworkInterfaceIds regardless of what was requested. Fixed: accepted, format-validated (same subnet-[0-9a-f]{8,} pattern as CreateFileSystem), stored, and echoed, plus SecurityGroupIds accepted-and-validated for consistency (matches real AWS: 'This value isn't returned in later DescribeFileSystem requests', so, like CreateFileSystem, intentionally not stored/echoed). Not made required-and-rejecting-when-absent, matching the existing precedent immediately below (CreateFileSystem's own SubnetIds gap) and to avoid breaking the existing test fixtures that predate SubnetIds support on this op. Proven by wire_field_fixes_test.go's TestCreateFileSystemFromBackup_SubnetIdsRoundTrip (real client, hand-reverted, confirmed failing pre-fix, restored md5sum-identical)."
  - "Delete*Output shapes (DeleteFileSystem, DeleteVolume) do not include the optional WindowsResponse/LustreResponse/OpenZFSConfiguration finalizer sub-objects (e.g. FinalBackupTags) that real AWS returns when a final backup is requested at delete time. Low traffic; not fixed this pass (gopherstack-wjjl was scoped to idempotency + network validation, not this)."
  - "CreateFileSystem still does not REQUIRE SubnetIds (real AWS: Required: Yes, and exactly two for Windows/ONTAP MULTI_AZ_1 deployments). Re-confirmed this pass (gopherstack-wjjl) against the live API reference (docs.aws.amazon.com/fsx/latest/APIReference/API_CreateFileSystem.html): SubnetIds is genuinely required. Still not enforced: grep confirms zero test fixtures across the entire fsx package (5 test files, 28+ CreateFileSystem call sites) ever populate SubnetIds, so flipping it to required would be a wholesale fixture migration, not a small fix, and this emulator still does not model Availability Zone topology needed for the exactly-one-vs-exactly-two-subnets MULTI_AZ_1 rule. What WAS fixed this pass: SubnetIds/SecurityGroupIds, when supplied, are now format-validated against the real ID patterns (subnet-[0-9a-f]{8,} / sg-[0-9a-f]{8,}) and rejected with InvalidNetworkSettings if malformed -- see families note below."
  - "ActiveDirectoryError (AD-join failures for WINDOWS/ONTAP file systems joining a directory) is not modeled: ActiveDirectoryId is accepted and echoed back but never validated against a real Directory Service resource (gopherstack's ds package). Not fixed this pass -- cross-service validation, out of scope for a single-service parity pass."
  - "CreateFileSystem (the non-backup create path) does not accept FileSystemTypeVersion, unlike CreateFileSystemFromBackup which gained it this pass (gopherstack-cgq3). Real CreateFileSystemInput has this field too (api_op_CreateFileSystem.go:118), so a Lustre file system created directly (not restored from a backup) can never have a non-empty FileSystemTypeVersion in this emulator, and CreateFileSystemFromBackup's own \"inherit from source file system\" fallback is therefore currently always empty in practice unless the caller supplies an explicit override. Not fixed this pass -- out of the single-op scope that found it."
  - "FIXED 2026-08-23: CreateVolume's input-shape gap (see the Volume family note and Notes section) -- real CreateVolumeInput has no top-level FileSystemId/StorageVirtualMachineId; the anchor is OntapConfiguration.StorageVirtualMachineId (ONTAP) / OpenZFSConfiguration.ParentVolumeId (OPENZFS). Response-side OntapVolumeConfiguration/OpenZFSVolumeConfiguration on Volume remain unmodeled (Layer 3, unchanged, see the Volume family note above)."
  - "2026-08-31 (value-semantics sweep, gopherstack-uox6): CreateDataRepositoryAssociationInput.BatchImportMetaDataOnCreate (bool, real field, api_op_CreateDataRepositoryAssociation.go, 'Default is false') and DeleteDataRepositoryAssociationInput.DeleteDataInFileSystem (bool, api_op_DeleteDataRepositoryAssociation.go) are not declared anywhere in gopherstack's request/backend structs at all -- the never-declared axis, not this pass's value-semantics axis, so recorded rather than fixed. Not at risk of the flattened-pointer-default shape found elsewhere this campaign: both real fields default to false, which is also Go's bool zero value, so there is no omitted-vs-explicit-false distinction to lose. Honouring BatchImportMetaDataOnCreate would mean auto-creating a real DataRepositoryTask as a side effect of CreateDataRepositoryAssociation, a feature addition rather than a value-semantics fix."
deferred: []              # consciously not audited this pass (scope) — next pass targets
leaks: {status: clean, note: "Single InMemoryBackend with no goroutines, timers, or janitors; Reset()/Snapshot()/Restore() all go through the coarse lockmetrics.RWMutex and store.Registry -- no ephemeral state outside the registered tables/maps. FIXED THIS PASS (previously leaky): DeleteFileSystem only removed the file system + its own tags, leaving ghost StorageVirtualMachine/Volume/Snapshot/DataRepositoryAssociation rows (and a stale aliases[fileSystemID] map entry) referencing a FileSystemId that no longer existed. DeleteVolume and DeleteStorageVirtualMachine had the same gap one level down (a deleted volume's snapshots, and a deleted SVM's volumes, were never cleaned up). All four Delete ops now cascade correctly (deleteVolumeLocked / deleteStorageVirtualMachineLocked / cascadeDeleteFileSystemChildrenLocked in file_systems.go, volumes.go, storage_virtual_machines.go), while intentionally leaving Backups and DataRepositoryTasks alone (real AWS retains both independently of the file system they reference). Regression tests added in cascade_delete_test.go."}
---

## Notes

Protocol: awsjson1.1 (single POST endpoint, `X-Amz-Target: AWSSimbaAPIService_v20180301.<Op>`).
Route matcher (`RouteMatcher`) does a prefix match on the target header, which is correct
and matches the real client's request shape; `GetSupportedOperations()` / `buildOps()` stay
in sync (verified no orphan registrations, no missing dispatch entries).

**Fixed this pass — the FileSystem config-block gap (the headline item from the prior
pass's `gaps` list):**
Field-diffed against `aws-sdk-go-v2/service/fsx@v1.66.2`'s `types/types.go`
(`CreateFileSystemWindowsConfiguration`, `CreateFileSystemOntapConfiguration`,
`CreateFileSystemOpenZFSConfiguration`, `WindowsFileSystemConfiguration`,
`OntapFileSystemConfiguration`, `OpenZFSFileSystemConfiguration`,
`UpdateFileSystemWindowsConfiguration`, `UpdateFileSystemOntapConfiguration`,
`UpdateFileSystemOpenZFSConfiguration`). Added `WindowsConfiguration`, `OntapConfiguration`,
`OpenZFSConfiguration`, `FileSystemEndpoints`, `FileSystemEndpoint` to interfaces.go, and
matching `createWindowsConfiguration`/`createOntapConfiguration`/`createOpenZFSConfiguration`
request types and `updateWindowsConfiguration`/`updateOntapConfiguration`/
`updateOpenZFSConfiguration` request types to file_systems.go. Required-member validation
(`applyWindowsConfig`/`applyOntapConfig`/`applyOpenZFSConfig` in file_systems.go) matches real
AWS: an absent config block on WINDOWS/ONTAP/OPENZFS returns `MissingFileSystemConfiguration`;
a present block missing `ThroughputCapacity` (Windows/OpenZFS) or `DeploymentType`+
(`ThroughputCapacity` or `ThroughputCapacityPerHAPair`) (ONTAP) returns `BadRequest`. Lustre is
untouched (LustreConfiguration remains genuinely optional with a SCRATCH_1 default, matching
real AWS). OpenZFS file systems now get a real backing root `Volume` row (Name `"fsx"`,
VolumeType `OPENZFS`) so `RootVolumeId` in the response is a genuine, describable ID rather than
a disguised placeholder string -- verified via `DescribeVolumes` in
`TestFSx_FileSystem_OpenZFSConfiguration`.

This required updating three shared test fixtures that previously created WINDOWS/ONTAP/OPENZFS
file systems with no config block at all (which is now correctly rejected): the shared
`createFS` helper in handler_test.go (now routes through `fileSystemCreateBody()`, which builds
a minimal valid config per type) and the two `CreateFileSystem` request bodies in
`TestCreateFileSystem_FileSystemTypeValidation`/`TestCreateFileSystem_StorageCapacityMinimum` in
handler_file_systems_test.go.

**Fixed this pass — errValidation's wire code (`"ValidationError"` is not a real FSx
exception):**
Field-diffed against `types/errors.go` in the SDK: FSx's generic client-error exception is
`BadRequest`; there is no `ValidationError` exception type anywhere in the FSx API. This was a
pre-existing bug (not introduced this pass) affecting every op that returns `ErrValidation`
across every family -- `handleError` in handler.go had a hardcoded `"ValidationError"` string
for the generic `awserr.ErrInvalidParameter` case instead of deriving the code from the error's
own message, which happened to mask the same bug in the `errValidation` constant. Fixed both:
`errValidation` is now `"BadRequest"`, and `handleError`'s generic case now emits `"BadRequest"`
directly. Added a `ErrMissingFileSystemConfiguration` case just above it for the new
`MissingFileSystemConfiguration` code introduced this pass.

**Fixed this pass — DeleteFileSystem/DeleteVolume/DeleteStorageVirtualMachine cascade
deletes (see `leaks` above):** previously these three ops only removed the target resource
itself, leaving ghost rows in every child table. Now: `DeleteStorageVirtualMachine` cascades to
its volumes (which cascades to those volumes' snapshots); `DeleteVolume` cascades to its
snapshots; `DeleteFileSystem` cascades to its SVMs (transitively volumes/snapshots), its
directly-attached volumes (e.g. an OpenZFS root volume), its DataRepositoryAssociations, and its
DNS aliases. Backups and DataRepositoryTasks are intentionally left alone (real AWS retains both
independently of the file system they reference).

**Traps for the next auditor:**
- The `toFileSystem()` special-case for `fileSystemTypeLustre` (always populating
  `LustreConfiguration` even when the create request didn't send one) is *intentional*,
  not a bug — see its doc comment re: terraform-provider-aws treating a nil config block as
  an empty read. Don't "simplify" it away. The same pattern now applies to
  `toWindowsConfiguration()`/`toOntapConfiguration()`/`toOpenZFSConfiguration()`.
- `CreateVolumeFromBackup`'s local `VolumeType` input field with an `"ONTAP"` default looks
  unusual but is harmless: the real `CreateVolumeFromBackupInput` wire shape has no
  `VolumeType` member at all (the operation is ONTAP-only), so no real client will ever send
  a conflicting value.
- Every resource here transitions straight to `AVAILABLE`/deletes straight away rather than
  sitting in `CREATING`/`DELETING` for a poll cycle. This is a deliberate, service-wide
  synchronous-emulation choice (matches every other resource type in this file), not a
  disguised no-op — don't flag individual ops for this without also flagging the whole
  service's design.
- `storedFileSystem`'s per-type fields (`ThroughputCapacity`, `DeploymentType`,
  `PreferredSubnetID`, etc.) are shared across Lustre/Windows/ONTAP/OpenZFS rather than each
  type getting its own nested struct -- this is intentional (see the doc comment on
  `storedFileSystem`): the real wire shape happens to reuse the same concept name
  (`DeploymentType`) across all four `*Configuration` blocks, and `toFileSystem()`'s switch on
  `FileSystemType` picks which public `*Configuration` block to populate from those shared
  fields. Don't "fix" this into four separate nested stored structs without a concrete reason.
- `WindowsConfiguration.Aliases` is deliberately never populated by `toWindowsConfiguration()`;
  the source of truth for DNS aliases is `DescribeFileSystemAliases`
  (`AssociateFileSystemAliases`/`DisassociateFileSystemAliases` in file_systems.go). See the doc
  comment on `WindowsConfiguration` in interfaces.go before "fixing" this.

**2026-08-22 (gopherstack-r80d, batch 34 -- required-OUTPUT-member sweep, wrapped-type-shape
candidate):** every fsx op's `<Op>Output` declares zero required members at its own top level
(confirmed via `cmd/requiredoutputfields`), so this service was invisible to every ranking this
campaign used through batch 33. Selected as one of the two candidates named by batch 33's
"ops with zero required fields wrapping richly-required domain types" mechanism test (`services/
_REQUIRED_OUTPUT_CANDIDATES.md`'s batch-33 section) and given the full hand audit that batch left
undone.

Every non-slice field of every `<Op>Output` was walked one hop into its own type
(`aws-sdk-go-v2/service/fsx@v1.68.4/types/types.go`); only two wrapped types declare >=2
required members of their own: `Backup` (`BackupId`, `CreationTime`, `FileSystem`, `Lifecycle`,
`Type` -- types.go, `type Backup struct`) and `DataRepositoryTask` (`CreationTime`, `Lifecycle`,
`TaskId`, `Type` -- types.go, `type DataRepositoryTask struct`), reached via
`CreateBackup`/`DescribeBackups`/`CopyBackup` and `CreateDataRepositoryTask`/
`DescribeDataRepositoryTasks` respectively.

**1 bug found and fixed:** `Backup.FileSystem` (`*types.FileSystem`, required, a pointer and
therefore provable) was dropped whenever the backup's source file system had since been
deleted. `Backup.FileSystem`'s own doc comment states this metadata "is persisted even if the
file system is deleted" -- and `DeleteFileSystem` intentionally does not cascade-delete backups
(already covered by `TestFSx_DeleteFileSystem_DoesNotCascadeToBackups`), so this is a genuinely
reachable, unexceptional state, not an edge case. `backups.go`'s `toBackup` derived `FileSystem`
from a live lookup in the `fileSystems` table at read time (`CreateBackup`/`DescribeBackups`/
`CopyBackup` all did this); once the file system was deleted, the lookup missed, and the
`omitempty`-tagged `FileSystem` wire key vanished entirely, decoding to `nil` on any real client
even though real FSx keeps serving it.

Fixed by snapshotting the source file system's metadata onto `storedBackup` at
`CreateBackup`/`CopyBackup` time (a new `FileSystem *storedFileSystem` field, deep-copied via a
new `cloneStoredFileSystem` helper so the snapshot never aliases the live, mutable
`fileSystems` table entry -- `store.Table.Get` returns the live pointer and `UpdateFileSystem`
mutates it in place) instead of re-deriving it from live state on every read; `toBackup` now
prefers the stored snapshot and falls back to a live lookup only for pre-existing
snapshot-restored backups that predate this fix. `CopyBackup` propagates the *source backup's*
own snapshot (not a fresh live lookup), matching that a copied backup's metadata should reflect
the source backup's own recorded state.

This is a purely additive field on `storedBackup` (part of `backendSnapshot`'s `backups` table);
`fsxSnapshotVersion` was correctly **not** bumped -- `pkgs/persistence`'s
`TestSnapshotVersionGuard` enforces this, and the checked-in golden
(`pkgs/persistence/testdata/snapshot_inventory.json`) was regenerated with `-update` to add the
new field.

Proven via a real `aws-sdk-go-v2/service/fsx` client round trip
(`wire_output_required_r80d_test.go`): create a file system, create a backup, delete the file
system, then `DescribeBackups`/`CopyBackup` and assert `Backup.FileSystem` is still non-nil with
the original `FileSystemId`. Hand-reverted (`backups.go` restored to `git show HEAD:services/
fsx/backups.go`), confirmed the test fails with "Backup.FileSystem is required and must survive
deletion..." on the pre-fix code, restored, `md5sum` byte-identical.

**Reviewed and ruled clean, not a bug:** `DataRepositoryTask`'s four required members
(`CreationTime`, `Lifecycle`, `TaskId`, `Type`) are all set unconditionally in `toPublic()` for
every task, with no live-lookup dependency of the `Backup.FileSystem` kind. `TaskId`/
`CreationTime` are the only provable (pointer) members among the four and both are always
populated; `Lifecycle`/`Type` are non-pointer enums in the real SDK type, so per this campaign's
standing rule they are not provable regardless and were not further pursued.

**Wrapped-type-shape hypothesis verdict for fsx: held.** The bug above sat two levels below
`DescribeBackupsOutput` (Output -> `Backup` -> `FileSystem`), invisible to every ranking this
campaign has used (required-field count, op count) since `Backup` and `FileSystem` are not
op-level fields at all. See `services/_REQUIRED_OUTPUT_CANDIDATES.md`'s batch-34 section for the
cross-service verdict (fsx found a bug this way; the paired candidate, codebuild, did not).
## 2026-08-20 — wrapper-key / nested-shape wire-parity sweep

Protocol re-confirmed independently this pass (don't trust `services/_PROTOCOLS.md` per the
sweep brief): `awsAwsjson11_*` prefix in both serializers.go/deserializers.go, `X-Amz-Target:
AWSSimbaAPIService_v20180301.<Op>` header — JSON-RPC 1.1. All 48 `awsAwsjson11_deserializeOpDocument<Op>Output`
helpers are both defined AND called from their op's own `HandleDeserialize` (grep-verified), so
the restjson "flat body, dead OpDocument helper" false-positive trap from the sweep brief does
not apply to this service.

Enumerated all 48 SDK ops (`ls api_op_*.go`) against `GetSupportedOperations()` /
`TestSDKCompleteness`'s empty `notImplemented` list — full 1:1 coverage, no orphans either
direction (`opDescribeDataRepositoryAssocs`'s Go identifier maps to the real op name
`DescribeDataRepositoryAssociations`, not a gap).

Three real bugs found and fixed, in order of severity: the S3AccessPoint feature modeling the
wrong AWS type entirely (wrong on both request and response, non-functional against a real
typed client), RestoreVolumeFromSnapshot/CopySnapshotAndUpdateVolume wrapping their response
under a fabricated "Volume" key that doesn't exist on either op's real Output struct (both
opened with an empty response for a real client), and FileCache's Describe/Update paths leaking
a Tags key that only the sibling Create response type actually has. Full citations and fix
descriptions are inline in the `families:` block above, per bug. All three were proven by a
real aws-sdk-go-v2 client round-trip (`services/fsx/s3_access_point_wire_test.go`,
`services/fsx/administrative_action_wire_test.go`, `services/fsx/file_cache_wire_test.go`), and
each fix was hand-reverted to confirm the exact predicted symptom before being restored.

Near-identical type families checked this pass, one line each:
- `FileCache` vs `FileCacheCreating` — was a single shared Go type with a fabricated `Tags`
  field bleeding across both real wire shapes; now split, matching the two real SDK types 1:1.
- The four per-flavour `*FileSystemConfiguration` blocks (Lustre/Windows/Ontap/OpenZFS) — each
  has its own distinct Go type with a case-by-case key check against its own live deserializer
  function; all clean, no cross-contamination.
- The `Target*Values` trio implied by `AdministrativeAction` — only `TargetVolumeValues` is
  populated (reusing the existing `Volume` type, matching the real SDK's own reuse);
  `TargetFileSystemValues`/`TargetSnapshotValues` are never emitted (Layer 3, out of scope).
- `S3AccessPointAttachment` vs the nested `S3AccessPoint` — two genuinely distinct real SDK
  types with the same-ish name; gopherstack previously conflated them into one flat type that
  matched neither. Now split into `S3AccessPointAttachment` (top-level) + `S3AccessPoint`
  (nested details, `Alias`/`ResourceARN` only) + per-flavour `S3AccessPointOntapConfiguration`/
  `S3AccessPointOpenZFSConfiguration`.

Families hand-verified clean (own deserializer's case list read and diffed against gopherstack's
emitted keys, no fabricated members found): `FileSystem` (including all four per-flavour
configs + `FileSystemEndpoints`/`FileSystemEndpoint`), `Backup` (single `types.Backup` shape
shared correctly across Create/Describe/Copy — confirmed via each op's own Output struct field
type, not assumed from the name), `DataRepositoryAssociation`, `DataRepositoryTask` (+
`CompletionReport`), `Snapshot`, `StorageVirtualMachine`, `Volume` (top-level fields only —
see gaps for the nested `OntapVolumeConfiguration`/`OpenZFSVolumeConfiguration` members never
emitted), `SharedVpcConfiguration`, `Tag`. Enum spellings spot-checked against `types/enums.go`
for every value gopherstack emits (`FileSystemType`, `StorageType`, `VolumeType`,
`BackupType`/`BackupLifecycle`, every resource's `*Lifecycle`, `DataRepositoryTaskType`,
`AdministrativeActionType`, `S3AccessPointAttachmentType`) — all exact matches, zero
case-mismatches or fabricated enum values found this pass.

Genuinely NOT reached this pass, disclosed rather than hunted: `ActiveDirectoryConfiguration`/
`SvmEndpoints`/`SvmEndpoint` on `StorageVirtualMachine`, and `OntapVolumeConfiguration`/
`OpenZFSVolumeConfiguration`/`TieringPolicy`/`SnaplockConfiguration`/`AutocommitPeriod`/
`RetentionPeriod` on `Volume` — all real SDK members never emitted at all by this backend
(Layer 3, "never emitted" is out of scope for a hunt per this sweep's brief; noted in the
`Volume`/`StorageVirtualMachine` family rows above rather than silently skipped).

`last_audit_commit` provenance: the prior manifest cited `3f66c846b...`, dated 2026-07-24 in
its own commit metadata (`git show -s --format=%ad`) — which matches `last_audit_date:
2026-07-24` exactly, even though that sha is not an ancestor of current `HEAD` (repo-wide
history-rewrite artifact per `gopherstack-z31a`, not a sign this manifest's prior audit was
fabricated or stale). Verdict: the prior audit's provenance checks out; updated to this
session's `HEAD` (`8d4556e79386...`, 2026-08-20) above.

## 2026-08-23 (gopherstack batch8) — CreateVolume request-side accept-and-drop fixed

**Bug**: `CreateVolume`'s decode struct (`createVolumeInput`, `volumes.go`) had
top-level `FileSystemId`/`StorageVirtualMachineId` fields. Real
`CreateVolumeInput` (`fsx@v1.68.4 api_op_CreateVolume.go`) has neither — the
only real anchor for an ONTAP volume's parent is
`OntapConfiguration.StorageVirtualMachineId` (required within that block per
`validators.go:1122`'s `validateCreateOntapVolumeConfiguration`), and for
OpenZFS it's `OpenZFSConfiguration.ParentVolumeId` (required per
`validators.go:1160`). Since gopherstack's struct had no `OntapConfiguration`/
`OpenZFSConfiguration` fields at all, a real client's nested SVM/parent-volume
reference was silently dropped by `encoding/json` (unknown key, no error) —
`CreateVolume` always succeeded with `FileSystemId`/`StorageVirtualMachineId`
both empty, no existence check performed, no error surfaced either way. This
is the "request-side accept-and-drop" bug class (see the S3AccessPoint/
RestoreVolumeFromSnapshot fixes above for the paired response-side version)
that this campaign's batches have repeatedly found to be real.

**Fix** (`volumes.go`): added `createOntapVolumeConfigInput{StorageVirtualMachineID}`
and `createOpenZFSVolumeConfigInput{ParentVolumeID}` (the sole required member
of each real config block; the rest of each block's fields are out of this
fix's scope since `Volume`'s response has no matching
`OntapVolumeConfiguration`/`OpenZFSVolumeConfiguration` to round-trip them
into at all — a separate, disclosed, pre-existing Layer-3 gap, unchanged). New
`resolveVolumeParentLocked` mirrors `CreateFileSystem`'s existing
`applyOntapConfig`/`applyOpenZFSConfig` precedent: `VolumeType=ONTAP` without
`OntapConfiguration.StorageVirtualMachineId` (or `OPENZFS` without
`OpenZFSConfiguration.ParentVolumeId`) now returns `MissingVolumeConfiguration`
(new sentinel `ErrMissingVolumeConfiguration`, `errors.go`; real wire code
confirmed via `types.MissingVolumeConfiguration`, `types/errors.go:769`, "A
volume configuration is required for this operation."); a named-but-unknown
`StorageVirtualMachineId`/`ParentVolumeId` returns
`StorageVirtualMachineNotFound`/`VolumeNotFound` (both pre-existing
sentinels); a resolved SVM/parent volume's `FileSystemId` is copied onto the
new volume for real, closing the FileSystemId-always-empty symptom.

**Proof**: `TestCreateVolume_RealRequestShape` (`handler_volumes_test.go`, 4
subtests) drives the real `aws-sdk-go-v2/service/fsx` client end to end
(`CreateFileSystem` -> `CreateStorageVirtualMachine` -> `CreateVolume` for
ONTAP; `CreateFileSystem` -> `CreateVolume` against the auto-created OpenZFS
root volume) and asserts `Volume.FileSystemId` matches the real parent, plus
the two negative cases (missing config block, unknown SVM). Hand-reverted
`volumes.go` to `git show HEAD:services/fsx/volumes.go` (the pre-fix flat
top-level-fields version) and confirmed all 4 subtests fail exactly as
predicted: the two positive cases assert `Volume.FileSystemId` equals the
real file system's ID and get back `""` instead (no error — the exact silent
accept-and-drop symptom), and the two negative cases assert an error and get
`nil` (no error — the exact fabricated-success symptom). Restored;
`md5sum` byte-identical to the pre-revert file.

Five pre-existing test call sites relied on the old (invalid) flat wire shape
and were updated to build a real SVM/OpenZFS-root-volume anchor first,
matching what a real client must now do:
`administrative_action_wire_test.go` (2 sites, via a new shared
`createTestOntapVolume` helper in `handler_create_tags_test.go`),
`s3_access_point_wire_test.go` (1 site, same helper),
`handler_create_tags_test.go`'s own "snapshot"/"volume" tag-reachability
cases (2 sites), `cascade_delete_test.go` (3 sites, ONTAP + OPENZFS),
`handler_volumes_test.go`'s `TestFSx_Volume` table test and the shared
`createVolume(t, h, fsID, volType, name)` helper in `handler_test.go` (now
resolves/creates a real SVM or OpenZFS root volume internally instead of
sending the old flat shape), and `Test_CreationTime_IsEpochSecondsNumber`'s
"Snapshot"/"Volume" cases in `handler_test.go`. None of these were testing
anything about the flat shape itself — they were incidentally relying on the
bug being present to get away with an unauthenticated SVM reference.

**Not fixed, still disclosed** (unchanged from the prior gap entry, narrowed):
`Volume`'s response has no `OntapVolumeConfiguration`/`OpenZFSVolumeConfiguration`
at all (Layer 3, pre-existing, see the Volume family note above) — this fix is
request-side only; the resolved `StorageVirtualMachineId` is stored on
`storedVolume` (used internally, e.g. by `deleteStorageVirtualMachineLocked`'s
cascade-delete) but has nowhere real on the wire to be echoed back nested,
same as before this pass. `OntapConfiguration`'s other real fields
(`JunctionPath`, `SizeInBytes`, `SecurityStyle`, `OntapVolumeType`,
`SnaplockConfiguration`, ...) and `OpenZFSConfiguration`'s (`DataCompressionType`,
`NfsExports`, `OriginSnapshot`, `UserAndGroupQuotas`, ...) remain unmodeled —
only each block's sole required member was added, matching this fix's scope
(the bug named in the prior gap entry was specifically about the missing
anchor, not full per-type field coverage).

Gates: `go build ./...`, `go vet ./services/fsx/...`, `gofmt -l`/`golines -l`
(clean), `go test -race -count=1 ./services/fsx/...` (pass),
`golangci-lint run ./services/fsx/...` (0 issues). No persisted struct's wire
shape changed (`storedVolume`'s `StorageVirtualMachineID` field already
existed; only how it's populated changed) — `fsxSnapshotVersion` correctly
left unbumped.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

**Bug found and fixed, Class B:** `paginate` (`store.go`), the single generic
offset paginator behind all 7 `Describe*` list operations
(`DescribeDataRepositoryAssociations`, `DescribeFileCaches`,
`DescribeDataRepositoryTasks`, `DescribeStorageVirtualMachines`,
`DescribeSnapshots`, `DescribeS3AccessPointAttachments`, `DescribeVolumes`),
searched for the `nextToken`'s named item by exact equality and left `start`
at its zero-value default on a miss. A token naming an item deleted between
calls, or any hand-built/tampered token, reset pagination to the first page
instead of resuming past it or terminating — a client following the cursor
would see page one, forever.

Every caller sorts its slice ascending by the same key `paginate`'s `keyFn`
returns (confirmed per call site, not assumed), so the fix searches for the
first surviving key `>= nextToken` instead of `==`, defaulting `start = n`
(not `0`) when nothing matches. This can no longer resolve a miss to zero by
construction, so a future edit that forgets to handle "not found" still gets
the safe answer without a signature change.

Proof: `TestPaginate_StaleCursor_DeletedItem` and
`TestPaginate_TamperedCursor_NoMatch`
(`pagination_arithmetic_internal_test.go`, unit, call `paginate` directly)
both reproduce the bug pre-fix (returning the stale duplicate/full list
instead of the correct remainder); `TestDescribeVolumes_SDKRoundTrip_StaleCursorResumesPastDeletedItem`
(`pagination_sdk_roundtrip_test.go`) ties it to the real
`aws-sdk-go-v2/service/fsx` client — deletes the volume the cursor names
between calls, then asserts the resumed page holds neither the already-seen
first item nor the deleted one.

All seven checks (boundary walk, final page, single page, empty collection,
exact division, cursor round trip, stale cursor) pass post-fix; no Class A
(panic) or Class C shape found — `maxResults <= 0` is normalized to a
positive default at every call site before reaching `paginate`, so a
negative limit can't drive `end < start` either.

Gates: `go build ./services/fsx/...`, `go vet ./services/fsx/...` and
`go vet ./...` (repo-wide, clean — no signature changed),
`go test -race -count=1 ./services/fsx/...`, `golangci-lint run
./services/fsx/...` (0 issues).

### 2026-08-31 pass (gopherstack-uox6, value-semantics class): re-derived clean, zero bugs, coverage strengthened

Dispatched by targeting ("no `filter_default_semantics` covledger row"), but `covledger -service fsx`
credits only `pagination_ordering` and `request_field_never_read` -- both from the 2026-08-29 filter/cursor
passes (`e3a19f13e`, `39d671395`). The ledger's known blind spot (attribution rides the commit subject/body,
so a value-semantics audit filed under a different bug-class label is invisible to it) applies here:
`e3a19f13e`'s own PARITY note IS a value-semantics audit of every fsx filter -- enum membership per
operation, the OR-within-values/AND-across-filters combining rule, and the unrecognized-name policy -- it
was simply never tagged that way. Per this campaign's "twice already" precedent, re-derived rather than
re-audited from scratch:

1. **`matchesFilters` (filters.go) combining rule** -- read directly: `slices.Contains(f.Values, got)` ORs
   every element of one filter's Values (not `Values[0]`), the loop ANDs across distinct filter Names.
   HOLDS.
2. **Per-operation filter-name coverage matches each operation's own SDK doc comment exactly** --
   independently re-fetched `types/enums.go` for all six FilterName-family enums and every operation's own
   `Filters` doc comment (not a sibling's): `DescribeBackupsInput` documents exactly file-system-id/
   backup-type/file-system-type/volume-id (4), gopherstack implements 3 and discloses volume-id as a gap;
   `DataRepositoryTaskFilterName` has 4 members, gopherstack implements file-system-id/task-lifecycle and
   discloses the other 2; Snapshot(2/2), Volume(2/2), StorageVirtualMachine(1/1), S3AccessPointAttachments(3/3)
   all fully implemented. HOLDS.
3. **The three disclosed "no honest data" gaps are structurally real, not assumed** -- grepped
   `createBackupInput`/`CreateDataRepositoryTaskInput` in gopherstack source: neither has ever had a
   VolumeId/association/file-cache field to store, confirming the filter truly has nothing to compare
   against (rather than an unread-but-present field, which would be a different, fixable bug). HOLDS.

**No `SortBy`/`SortOrder` and no `default`/`if you omit`/`if not specified` language anywhere in fsx's
pinned SDK doc comments** (swept every `api_op_*.go`) -- fsx genuinely lacks the sortOrder-default and
narrowing-default-omitted sub-shapes this class has found repeatedly elsewhere; this is a structural absence
of surface, confirmed rather than assumed, same as cloudfront/apigateway/cloudformation/elbv2 in this
campaign. `maxResultsDefault = math.MaxInt32` (store.go) is consistent with this -- no operation documents a
numeric MaxResults default to contradict.

**Two never-declared-axis findings, recorded not fixed** (added to `gaps:` above):
`CreateDataRepositoryAssociationInput.BatchImportMetaDataOnCreate` and
`DeleteDataRepositoryAssociationInput.DeleteDataInFileSystem` are real fields nowhere in gopherstack's
structs. Different axis from this pass's remit (a field never declared, not a value read wrongly); not
at risk of the flattened-pointer-default shape since both real defaults are `false`, matching Go's zero
value.

**Coverage gap closed, not a bug**: every existing fsx filter test (`wire_field_fixes_test.go`) passes
exactly one value in each filter's `Values` list, so none of them can distinguish "matched anywhere in
Values" from "matched only `Values[0]`" -- the confirmed first-element-only shape (four sightings
elsewhere this campaign). Added `TestDescribeVolumes_Filters_MultipleValuesInOneFilter`, a two-value
filter that must match volumes on either value and exclude a third. Confirmed it can fail: temporarily
changed `matchesFilters` to compare only `f.Values[0]`, watched the new test fail (extra/missing element
diff on the expected two-volume result), restored `filters.go` byte-identical (md5sum verified). No
existing test's assertions were touched or weakened; 3 new assertions added, 0 dropped.

Gates: `go build ./services/fsx/... ./services/codebuild/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/fsx/...`, `golangci-lint run ./services/fsx/... ./services/codebuild/...`
(0 issues).

## 2026-08-31 Error-envelope sweep (gopherstack-6flj/uox6, errtargetaudit)

`errtargetaudit -dir fsx` reported 6 class-A findings, all
`mechanism=sentinel reference`. fsx's error matching is
`awsAwsjson11_deserializeOpError<Op>` (a per-op switch in deserializers.go).
Verified every finding against its op's own switch (fsx@v1.68.4
deserializers.go). All 6 were real. False-positive rate: 0% (6/6).

**CopySnapshotAndUpdateVolume (2 findings, `snapshots.go`)**: emitted
`ErrVolumeNotFound`/`ErrSnapshotNotFound` on a missing volume/snapshot. Its
own switch is exactly `[BadRequest, IncompatibleParameterError,
InternalServerError, ServiceLimitExceeded]` — neither NotFound type is
declared for this op, even though both are legitimately declared elsewhere
(VolumeNotFound: CreateAndAttachS3AccessPoint, CreateBackup, CreateSnapshot,
DeleteVolume, DescribeBackups, and others not touched by this pass;
SnapshotNotFound: DeleteSnapshot, DescribeSnapshots, UpdateSnapshot).
**Fixed**: both checks now use `ErrValidation` (`BadRequest`, "a generic
error indicating a failure with a client request" per
`aws-sdk-go-v2/service/fsx/types.BadRequest`'s doc comment) — this op's own
declared generic-client-error type, already how the rest of this service
answers validation-shaped conditions with no more specific declared type
(see `errValidation`'s existing doc comment in errors.go).

**RestoreVolumeFromSnapshot (1 finding, `volumes.go`)**: its own switch is
`[BadRequest, InternalServerError, VolumeNotFound]` — the VolumeId check's
`ErrVolumeNotFound` is correctly declared and untouched; only the SnapshotId
check's `ErrSnapshotNotFound` was wrong (not declared here). **Fixed**: same
`ErrValidation` substitution as above.

**TagResource (1 finding, `tags.go`)**: the 50-tag-limit check emitted
`ErrTagLimitExceeded` (`ServiceLimitExceeded`). TagResource's own switch is
`[BadRequest, InternalServerError, NotServiceResourceError,
ResourceDoesNotSupportTagging, ResourceNotFound]` — no
ServiceLimitExceeded, even though it's legitimately declared by CopyBackup,
CopySnapshotAndUpdateVolume, CreateBackup, CreateDataRepositoryAssociation,
CreateDataRepositoryTask. Neither `NotServiceResourceError`
("resource...not owned by Amazon FSx") nor `ResourceDoesNotSupportTagging`
fit "too many tags" by their own doc comments, so `ErrValidation`
(BadRequest) is the correct substitution, not an invented code. **Existing
test corrected**: `handler_tags_test.go`'s `TestFSx_TagLimit` subtest
"51st tag returns ServiceLimitExceeded" asserted the wrong wire type;
renamed to "51st tag returns BadRequest" and its one assertion changed to
match (assertion count unchanged: 1).

**DescribeS3AccessPointAttachments + DetachAndDeleteS3AccessPoint (2
findings, `s3_access_points.go`)**: both emitted `ErrS3AccessPointNotFound`
(`InvalidRequest`) on an unknown attachment name. Their own switches both
declare `S3AccessPointAttachmentNotFound`, not InvalidRequest — InvalidRequest
only fits CreateAndAttachS3AccessPoint's declared set (`[..., InvalidAccessPoint,
InvalidRequest, ...]`), and CreateAndAttachS3AccessPoint doesn't actually
emit `ErrS3AccessPointNotFound` anywhere in the current code (it uses
`ErrVolumeNotFound` for its own not-found case) — so this sentinel had zero
correct callers before this fix. **Fixed**: added a new sentinel
`ErrS3AccessPointAttachmentNotFound` (`S3AccessPointAttachmentNotFound`) and
switched both call sites to it; `ErrS3AccessPointNotFound` (`InvalidRequest`)
is left declared but currently unused by any code path, kept in case a
future CreateAndAttachS3AccessPoint validation needs it — not deleted,
since deleting a still-declared, still-potentially-correct sentinel would
be a different kind of loss than deleting an always-wrong check.

New real-SDK-client tests (`error_envelope_fixes_test.go`, 6 assertions
across 4 test funcs / 4 subtests) drive `types.BadRequest` and
`types.S3AccessPointAttachmentNotFound` via `errors.As`; all 6 confirmed
failing against unmodified code (got `*smithy.GenericAPIError` for
VolumeNotFound/SnapshotNotFound/ServiceLimitExceeded/InvalidRequest
respectively).

Re-run after fix: `errtargetaudit -dir fsx` now reports 0 class-A findings.

Gates: `go build`, `go vet` (repo-wide, clean), `go test -race -count=1`,
`golangci-lint run` — all clean (`./services/fsx/...`).

## 2026-09-08 (gopherstack-u7rl) — Backup lifecycle errors reconfirmed structurally unobservable; tag-limit gap found and fixed

**The issue's claim, re-derived independently.** fsx ships a real `deserializers.go`
(not one of the eleven schema-codegen modules with no deserializer file), so declared
error sets were read from it directly, cross-checked against botocore
`fsx/2018-03-01/service-2.json.gz` (`operations.<Op>.errors`) — identical on every op
checked, both oracles agree.

Three errors, verbatim doc comments (`aws-sdk-go-v2/service/fsx@v1.68.4/types/errors.go`):
- `BackupBeingCopied` (`errors.go:68`): "You can't delete a backup while it's being copied."
- `BackupInProgress` (`errors.go:96-97`): "Another backup is already under way. Wait for
  completion before initiating additional backups of this file system."
- `BackupRestoring` (`errors.go:149`): "You can't delete a backup while it's being used
  to restore a file system."

Declaring ops, read from each op's own `awsAwsjson11_deserializeOpError<Op>` switch
(`deserializers.go`), botocore's `operations.<Op>.errors` agrees exactly:
- `CreateBackup`: `BackupInProgress, BadRequest, FileSystemNotFound,
  IncompatibleParameterError, InternalServerError, ServiceLimitExceeded,
  UnsupportedOperation, VolumeNotFound`
- `DeleteBackup`: `BackupBeingCopied, BackupInProgress, BackupNotFound, BackupRestoring,
  BadRequest, IncompatibleParameterError, InternalServerError` — the only op that declares
  all three named errors together; `DeleteBackup`'s own doc comment plus each shape's own
  doc comment establish all three as `DeleteBackup`-time preconditions on the backup being
  deleted (in progress being created / being restored from / being copied), not on any other
  op's target.
- `CopyBackup` and `CreateFileSystemFromBackup` (the two other ops with a plausible claim on
  these errors, since they're the ones that could put a backup "in use") declare neither
  `BackupRestoring` nor `BackupBeingCopied` at all — confirmed via both oracles.

**Backup.Lifecycle write-site enumeration** (`grep -n "Lifecycle:" services/fsx/backups.go`,
plus `handleDeleteBackup` in `handler_backups.go`): exactly two places ever set
`storedBackup.Lifecycle`, both to `lifecycleAvailable` ("AVAILABLE") unconditionally —
`CreateBackup` (`backups.go:101`) and `CopyBackup` (`backups.go:289`). `DeleteBackup`
(`backups.go:229-242`) never sets `storedBackup.Lifecycle` at all; it deletes the row
outright, and the handler (`handler_backups.go:59`) returns a hardcoded `lifecycleDeleted`
string in the response, not a value read from stored state. No code path anywhere in the
package ever produces a `storedBackup` with `Lifecycle` other than `AVAILABLE` — `CREATING`,
`PENDING`, `TRANSFERRING`, `COPYING`, `PARTIALLY_COPIED`, `RESTORING`, `DELETED` are all
absent from the persisted set (`DELETED` only ever appears transiently in a `DeleteBackup`
response, never stored).

**Janitor check**: `services/fsx/` has no `janitor.go` (confirmed:
`ls services/fsx/janitor.go` — no such file; repo-wide, 38 of 161 services have one, fsx is
not among them) and no goroutine/timer of any kind (`leaks:` above already documents this:
"Single InMemoryBackend with no goroutines, timers, or janitors"). Unlike stepfunctions
(closed this campaign by wiring `DELETING` through a pre-existing janitor sweep),
there is no existing background-transition mechanism here to hang a fix on.

**Verdict: (a), genuinely unobservable — the issue's title holds.** `CreateBackup` and
`CopyBackup` both complete synchronously within a single handler call, holding the
package's one coarse `lockmetrics.RWMutex` (`b.mu`) for the call's full duration; a
concurrent `DeleteBackup` on the same or a different goroutine blocks on that same mutex
and only proceeds after the create/copy has already fully committed (`AVAILABLE`) or failed
(no row written). There is no window, even under genuine goroutine concurrency from two real
HTTP requests, in which a backup is observably "in progress," "restoring," or "being
copied" for `DeleteBackup` to reject — the coarse lock serializes exactly the interleaving
that would need to exist. This is the same shape as the campaign's other closed
structurally-unobservable calls (amplify, networkmonitor, support, detective): reaching any
of the three requires an async backup lifecycle this synchronous, single-mutex emulator does
not have and was not asked to add (adding one would be inventing a mechanism to justify the
issue, not fixing a defect — same trap the amplify/networkmonitor/support/detective issues
correctly avoided).

**Smaller defect found while checking the "is there an easier win nearby" candidates named
in this audit's brief**: `CreateBackup`'s and `CopyBackup`'s `Tags` input. Real
`CreateBackupInput.Tags`/`CopyBackupInput.Tags` (and every other `Create*Input.Tags` in this
service) reuse a single shared `Tags` list shape, documented in botocore
(`fsx/2018-03-01/service-2.json.gz`, shape `Tags`) as "A list of Tag values, with a maximum
of 50 elements" (`max: 50`). The real SDK's own client-side validator
(`validateOpCreateBackupInput` -> `validateTags`, `aws-sdk-go-v2/service/fsx@v1.68.4/
validators.go:1928,1679`) only validates each tag's own key/value constraints — it never
checks the list's length — so a real typed client CAN put 51+ tags on the wire, and
gopherstack's own `validateTags` (`tags.go`, pre-fix) had the identical gap: per-tag checks
only, no length check, on every one of its 12 call sites. Confirmed reachable via the real
SDK client, not merely constructible: `TestCreateBackup_TagLimitExceeded` /
`TestCopyBackup_TagLimitExceeded` (`tag_limit_test.go`) drive
`aws-sdk-go-v2/service/fsx`'s real `CreateBackup`/`CopyBackup` with 51 distinct-keyed tags;
both confirmed FAILING pre-fix (`require.Error` got `nil` — the backup was silently created
with all 51 tags, no rejection).

**Fix**: `validateCreateTags` (`tags.go`), which runs the existing per-tag `validateTags`
then rejects `len(tags) > maxTagsPerResource` (50, pre-existing constant, already used by
`TagResource`'s own separate cumulative check) with `ErrTagLimitExceeded`
(`ServiceLimitExceeded`, pre-existing sentinel). Wired at all 11 `Create*` call sites that
previously called `validateTags(input.Tags)` directly (`backups.go` x2 — `CreateBackup`/
`CopyBackup`; `data_repository_associations.go`; `data_repository_tasks.go`;
`file_caches.go`; `file_systems.go` x2 — `CreateFileSystem`/`CreateFileSystemFromBackup`;
`snapshots.go`; `storage_virtual_machines.go`; `volumes.go` x2 — `CreateVolume`/
`CreateVolumeFromBackup`). `ServiceLimitExceeded` independently confirmed legitimately
declared by every one of those 11 ops' own `deserializeOpError<Op>` switch /
botocore `errors` list before wiring (not assumed from the `Tags` shape's constraint alone).

**`TagResource`'s own call (`tags.go:25`) deliberately left on plain `validateTags`, not
`validateCreateTags`**: `TagResource` does not declare `ServiceLimitExceeded` at all (its
own switch: `BadRequest, InternalServerError, NotServiceResourceError,
ResourceDoesNotSupportTagging, ResourceNotFound` — already established by the 2026-08-31
error-envelope sweep above, which for the same reason corrected `TagResource`'s *separate*
cumulative-tag-count check from `ServiceLimitExceeded` to `BadRequest`). `TagResource`'s
`Tags` input reuses the same shape and so has the identical un-enforced max-50-per-call gap,
but fixing it is out of this pass's scope (a distinct call site needing its own `BadRequest`
variant, not `validateCreateTags` as written) — disclosed, not fixed:
`TagResource(resourceARN, tags)` can also be called with 51+ tags in a single request
without the per-call list-length being rejected (only the pre-existing cumulative
existing-plus-new check at `tags.go:48` applies, which is a different constraint).

No persisted type's fields changed (`storedBackup` untouched) — `fsxSnapshotVersion`
correctly not touched, `go test ./pkgs/persistence/...` not required.

Gates: `go build ./services/fsx/...`, `go test -race -count=1 ./services/fsx/...` (pass,
including the two new regression tests), `golangci-lint run ./services/fsx/...` (0 issues).
