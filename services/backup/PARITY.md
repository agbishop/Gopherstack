---
service: backup
sdk_module: aws-sdk-go-v2/service/backup@v1.64.0
last_audit_commit: 621eeacb
last_audit_date: 2026-08-29
overall: A            # all 4 prior gaps closed with real fixes + tests; all 4 prior deferred items field-diffed and closed; a service-wide error-code/HTTP-status bug found and fixed (see notes)
                      # 2026-08-21 (gopherstack-r80d batch 11, required-OUTPUT-member cut): 41 required output fields across 13 ops (the restore-testing-plan/selection and scan-job families -- the entirety of this service's required-output surface) read end to end against backup@v1.59.4's api_op_*.go/types.go, including every nested domain struct (RestoreTestingPlanForGet/-ForList, RestoreTestingSelectionForGet/-ForList, ScanJob, ScanJobCreator) the flat op-level scan can't see. 2 bugs: (1) RestoreTestingPlanForGet.RecoveryPointSelection (required) had no backing field at all -- CreateRestoreTestingPlanInput's own client-side validator (validateRestoreTestingPlanForCreate) rejects a nil RecoveryPointSelection, so every real client's plan has one, but it was silently discarded on Create and GetRestoreTestingPlan could never return it; fixed, threaded through Create/Update/Get. (2) DescribeScanJob/ListScanJobs returned only ScanJobId/Status, dropping 12 of DescribeScanJobOutput's 15 required members (AccountId/BackupVaultArn/BackupVaultName/CreatedBy/CreationDate/IamRoleArn/MalwareScanner/RecoveryPointArn/ResourceArn/ResourceName/ResourceType/ScanMode/ScannerRoleArn/State) even though the backend already tracked most of them on ScanJob -- this op's own PARITY line read 'wire: ok' (a stale verdict from checking an unrelated fabricated-200-status bug, not this required-output surface). Fixed: AccountId/BackupVaultName/ResourceArn/ResourceType/ResourceName (derived from the recovery point input.RecoveryPointArn identifies, never fabricated) added to ScanJob and emitted; CreatedBy (ScanJobCreator: BackupPlanArn/Id/Version + RuleId) stays a disclosed gap -- no backup-plan/rule lineage is tracked for a scan job or its recovery point in this backend, and StartScanJobInput itself carries no such reference, so there is no honest source to derive it from. Both bugs proven via real aws-sdk-go-v2/service/backup client round trips (wire_output_required_r80d_test.go), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical. Everything else read clean -- see families.RestoreTestingPlan/families.ScanJob below.
                      # 2026-08-29 (gopherstack-i25e, wrong-query-key sweep, REQUEST direction): every q.Get(...) across services/backup/ enumerated and compared one-by-one against the pinned SDK's per-op serializer SetQuery calls (not assumed from the "by"-prefix pattern). Two distinct defect classes found, both with the same user-visible symptom (a real client's filter silently no-ops, unfiltered results returned with no error): (1) WRONG KEY -- ListBackupJobs/ListCopyJobs/ListRecoveryPointsByBackupVault/ListBackupVaults read "by"-prefixed keys (byState/byResourceArn/etc.) the real wire doesn't have; fixed by dropping the prefix to match serializers.go exactly (backupVaultName was already correct, not touched). ListCopyJobs additionally had a "bySourceBackupVaultArn" filter with NO real wire equivalent at all -- the real filter is BySourceRecoveryPointArn -> "sourceRecoveryPointArn" (filters by the individual recovery point copied, not its containing vault); added CopyJob.SourceRecoveryPointArn and rewired the filter onto it. (2) FILTER NEVER ATTEMPTED -- ListRestoreJobs and ListScanJobs read no query filters at all (not even under a wrong key); added ListRestoreJobsFilter/ListScanJobsFilter, their Filtered backend methods, and query-parsing helpers. ListScanJobs is the single exception to the by-prefix-stripping pattern: verified directly against serializers.go that it keeps the full PascalCase "By..." key names on the wire, unlike every sibling op -- this file's own prior gaps note (now removed, see gaps below) had assumed otherwise and was wrong. Every fix proven via wire_field_fixes_test.go, which drives the real typed aws-sdk-go-v2 client with a matching AND a non-matching record per filter and asserts the non-matching one is excluded (a matching-only assertion would pass against the unfixed code, since the unfiltered response also contains it) -- all cases confirmed failing against unmodified code first. One pre-existing backend-level test (copy_jobs_test.go's "filter by source vault" case) asserted behavior for the fabricated SourceBackupVaultArn filter concept; corrected to test the real SourceRecoveryPointArn semantics instead. Several real filters remain unimplemented as honest follow-ups rather than fabricated: ByMessageCategory/ByCompleteAfter/ByCompleteBefore on ListBackupJobs, ByShared on ListBackupVaults, backupPlanId/backupVaultAccountId on ListRecoveryPointsByBackupVault, ByParentJobId/ByRestoreTestingPlanArn on ListRestoreJobs (RestoreJob has no field to hold either), ByScanResultStatus on ListScanJobs (ScanJob has no field to hold it), and IncludeDeleted on ListBackupPlans (would need a soft-delete model -- DeleteBackupPlan hard-removes the record today, a bigger structural change out of scope for this pass).
                      # 2026-08-29 (constraint-not-honoured sweep, same-day follow-on to gopherstack-i25e above,
                      # wrapper-key-sweep-rds-cloudwatch-sqs-sns branch): the i25e pass above fixed WRONG-KEY
                      # and never-attempted query filters; this pass specifically re-checked pagination
                      # (MaxResults/NextToken) and the *JobSummaries state-grouping, a different sub-shape of
                      # the same "constraint not honoured" bug class i25e didn't target. Found and fixed: (1)
                      # ListRestoreJobs and ListScanJobs never read MaxResults/NextToken at all (i25e's own
                      # note on ListRestoreJobs already flagged this as "remains unimplemented, unchanged by
                      # this pass" -- now closed); both wired through the existing paginateByID helper already
                      # used by ListBackupJobs/ListCopyJobs/ListBackupPlans/etc. in this same package. (2)
                      # ListRestoreJobSummaries/ListScanJobSummaries never grouped by State at all (unlike their
                      # ListBackupJobSummaries/ListCopyJobSummaries siblings, which already did) -- always
                      # returned one fabricated {Count[, Region]} entry regardless of job count or state,
                      # silently dropping State/AccountId (required RestoreJobSummary/ScanJobSummary members).
                      # Both now group by Status the same way the Backup/Copy siblings do. AccountId/
                      # AggregationPeriod/MessageCategory filtering on all four *JobSummaries ops, and
                      # ByParentJobId/ByRestoreTestingPlanArn/ByScanResultStatus filters already disclosed by
                      # i25e, remain open -- see residual_gaps. Every fix proven via wire_field_fixes_test.go
                      # driving the real typed aws-sdk-go-v2 client, confirmed failing against unmodified code
                      # first. ListBackupJobs/ListCopyJobs/ListBackupVaults/ListBackupPlans/ListFrameworks/
                      # ListReportPlans/ListRestoreTestingPlans/ListRestoreTestingSelections/ListLegalHolds/
                      # ListBackupSelections were independently re-checked this pass and found already correct
                      # (pagination applied via the same paginateByID/query-binding pattern, no filter fields
                      # silently dropped) -- not a re-fix, no bug found.
                      # CORRECTION (2026-08-30, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch, tie-prone-sort
                      # audit): the line above was WRONG about ListProtectedResources/ListProtectedResourcesByBackupVault
                      # -- neither one read MaxResults/NextToken at all (both real query params, backup@v1.59.4
                      # api_op_ListProtectedResources.go/api_op_ListProtectedResourcesByBackupVault.go,
                      # serializers.go:5645-5735); dispatchProtectedResourceOps called the bare backend accessors
                      # and returned every record in a single unpaginated response every time. Fixed: both backend
                      # methods now take (maxResults int, nextToken string) and page via the existing paginateByID
                      # helper over their pre-existing sort-by-ResourceArn order (ResourceArn is the protectedResources
                      # table's own key, so the sort was already total -- no tie-prone-sort bug here, just missing
                      # pagination). Handler now parses maxResults/nextToken from the query string and echoes
                      # NextToken when non-empty. Proven via wire_field_fixes_test.go's
                      # TestListProtectedResources_Pagination/TestListProtectedResourcesByBackupVault_Pagination
                      # (real client, confirmed failing against unmodified code first: MaxResults=1 returned both
                      # seeded records with no NextToken).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  StartBackupJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "job now actually completes -- see families.BackupJob. gopherstack-0o0q (2026-09-06): ResourceArn previously accepted any non-empty string with no cross-service existence check. A generic cross-service ARN-existence registry doesn't exist in this repo, so rather than build one, added a per-service switch (the pattern used five times already for cloudtrail/textract/rekognition->S3, athena->glue, ses->sns): an S3 ResourceArn (arn:{partition}:s3:::{bucket}, no object key -- the exact form Backup's S3 resource type uses) is checked via SetS3Backend's HeadBucket, returning ResourceNotFoundException for a bucket that doesn't exist. Every other resource type real AWS Backup supports (EC2/EBS/RDS/DynamoDB/EFS/FSx/...) stays permissive -- this emulator has no resolvable identity wired for them, and rejecting an ARN of a type this backend doesn't model would be a fabricated rejection. Unwired S3 (no SetS3Backend call) also stays permissive, matching this repo's unwired-hook-stays-permissive convention."}
  ListBackupJobs: {wire: ok, errors: ok, state: fixed, persist: n/a, note: "gopherstack-i25e (2026-08-29): REQUEST direction fixed -- query filters read under wrong \"by\"-prefixed keys (byState/byResourceArn/byResourceType/byAccountId/byParentJobId/byCreatedAfter/byCreatedBefore vs real state/resourceArn/resourceType/accountId/parentJobId/createdAfter/createdBefore, serializers.go:4629-4677); backupVaultName was already correct. The underlying jobMatchesFilter logic was already correct -- this was purely a wrong-wire-key defect, so every real client's filter on this op silently no-op'd and returned the unfiltered list with no error. messageCategory/completeAfter/completeBefore (real filters on ListBackupJobsInput) remain unimplemented -- left as a follow-up, not fabricated. See wire_field_fixes_test.go, which drives the real typed client and asserts a non-matching record is excluded per filter (not just that a matching one is present)."}
  StopBackupJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unroutable; real path is POST /backup-jobs/{id}, not /backup-jobs/{id}/stop-backup-job"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unroutable; real path is POST /untag/{arn}, not DELETE /tags/{arn}"}
  DisassociateBackupVaultMpaApprovalTeam: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was unroutable; real path is POST (with ?delete) on the same /mpaApprovalTeam path as Associate; responseCode 204 (was 200, fixed this pass)"}
  AssociateBackupVaultMpaApprovalTeam: {wire: ok, errors: ok, state: ok, persist: n/a, note: "responseCode 204 confirmed via botocore model's explicit http.responseCode -- was 200, fixed this pass"}
  GetRecoveryPointIndexDetails: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was unroutable (no route emitted this op at all); fixed path + vaultName wiring (was hardcoded \"\")"}
  UpdateRecoveryPointIndexSettings: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was unroutable; fixed path + vaultName wiring"}
  ListRecoveryPointsByBackupVault: {wire: ok, errors: ok, state: fixed, persist: n/a, note: "gopherstack-i25e (2026-08-29): REQUEST direction fixed -- query filters read under wrong \"by\"-prefixed keys (byResourceArn/byResourceType/byParentRecoveryPointArn/byCreatedAfter/byCreatedBefore vs real resourceArn/resourceType/parentRecoveryPointArn/createdAfter/createdBefore, serializers.go ListRecoveryPointsByBackupVault query bindings); rpMatchesFilter logic itself was already correct. backupPlanId/backupVaultAccountId (real filters) remain unimplemented, follow-up not fabricated. See wire_field_fixes_test.go."}
  UpdateRecoveryPointLifecycle: {wire: ok, errors: fixed, state: ok, persist: partial, note: "was unroutable AND a disguised no-op (wrote to a side map nobody read); now mutates RecoveryPoint.Lifecycle/CalculatedLifecycle directly. RecoveryPoint table is VOLATILE (not persisted) -- see families.RecoveryPoint. FIXED 2026-08-23 (batch9): errVaultNotFoundB1/errRecoveryPointNotFound (errors.go) did not wrap the shared ErrNotFound sentinel, and handleUpdateRecoveryPointLifecycle (handler_recovery_points.go) DOES route through h.handleError -- unlike GetTieringConfiguration/DescribeRestoreJob (already fixed a prior pass) but exactly the same bug class this file's own notes already flagged as live. handleError's switch falls through to its default case for any unwrapped error, so calling this op against an unknown vault or unknown recovery point ARN returned 500 InternalFailure instead of 400 ResourceNotFoundException. Fixed by wrapping ErrNotFound directly and deleting both now-orphaned local sentinels, same remediation as the prior TieringConfig/RestoreJob fixes. Proven via Test_UpdateRecoveryPointLifecycle_UnknownVaultIsResourceNotFound/_UnknownRecoveryPointIsResourceNotFound (wire_error_code_recovery_point_lifecycle_test.go), real aws-sdk-go-v2/service/backup client round trips asserting errors.As into *types.ResourceNotFoundException; hand-reverted to git show HEAD, confirmed both fail with a smithy.GenericAPIError{Code:\"InternalFailure\"} in the chain (not ResourceNotFoundException), restored, md5sum byte-identical."}
  CreateRestoreAccessBackupVault: {wire: ok, errors: ok, state: fixed, persist: ok, note: "method was POST, real AWS is PUT; SourceBackupVaultArn is now resolved against real vaults (ResourceNotFoundException if unresolvable) -- was previously stored verbatim with no validation. gopherstack-muzq (2026-08-21): VaultState was stamped CREATING and nothing ever advanced it -- no ticker, no later call -- so ListRestoreAccessBackupVaults showed CREATING forever. Fixed via a new Janitor.advanceRestoreAccessVaults, reusing the existing backup Janitor (advanceCreatedJobs' CREATED->COMPLETED is the same shape) rather than new infrastructure."}
  ListRestoreAccessBackupVaults: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GAP CLOSED this pass -- was unroutable (op existed only as dead handler code on the flat /restore-access-backup-vaults collection, which is NOT the real path). Real path is GET /logically-air-gapped-backup-vaults/{BackupVaultName}/restore-access-backup-vaults, always scoped to one source vault; there is no list-all. Backend now tracks SourceBackupVaultName per restore-access vault and filters by it."}
  RevokeRestoreAccessBackupVault: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GAP CLOSED this pass -- same unroutable/wrong-path issue as List; real path is DELETE .../logically-air-gapped-backup-vaults/{name}/restore-access-backup-vaults/{arn}. Revoking a restore-access vault whose source vault name doesn't match the path is now correctly rejected (ResourceNotFoundException) instead of silently succeeding cross-vault."}
  GetLegalHold: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unroutable (no GET case in parseLegalHoldRoute at all); error body now uses errResp so the SDK deserializes ResourceNotFoundException instead of UnknownError"}
  ListLegalHolds: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unroutable"}
  CreateLegalHold: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED this pass -- CreateLegalHoldInput.RecoveryPointSelection (DateRange/ResourceIdentifiers/VaultNames) was entirely absent from the model/wire parsing; now accepted and stored on the hold"}
  ListRecoveryPointsByLegalHold: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GAP CLOSED this pass -- backend previously returned [] unconditionally (association never tracked). CreateLegalHold now accepts a RecoveryPointSelection (VaultNames/ResourceIdentifiers/DateRange, matching real types.RecoveryPointSelection) and List now actually filters tracked recovery points against it. Wire response also fixed from a bare RecoveryPointArn to the real RecoveryPointMember shape (BackupVaultName/RecoveryPointArn/ResourceArn/ResourceType)."}
  DescribeBackupVault: {wire: ok, errors: ok, state: fixed, persist: ok, note: "GAP CLOSED this pass -- now returns EncryptionKeyType (derived: CUSTOMER_MANAGED_KMS_KEY iff EncryptionKeyArn set, else AWS_OWNED_KMS_KEY) and MpaApprovalTeamArn (from b.mpaApprovals, already tracked but never surfaced). MpaSessionArn/LatestMpaApprovalTeamUpdate remain absent -- this backend has no MPA-session-approval-workflow state to source them from (see gaps). gopherstack-muzq (2026-08-21): VaultState was hardcoded AVAILABLE unconditionally, even for an air-gapped vault the instant after creation, while ListBackupVaults (below) hardcoded CREATING unconditionally for every air-gapped vault forever -- the two read paths for the same resource could never agree. Fixed via a shared vaultStateFor(v) helper (vaults.go) computing CREATING for airGappedVaultCreatingWindow (100ms) after CreationTime when MinRetentionDays > 0, else AVAILABLE, used by both handlers."}
  ListBackupVaults: {wire: ok, errors: ok, state: fixed, persist: ok, note: "gopherstack-muzq (2026-08-21): see DescribeBackupVault's note -- same vaultStateFor(v) fix, same bug. gopherstack-i25e (2026-08-29): byVaultType -> vaultType (serializers.go ListBackupVaults query bindings) -- every real client's ByVaultType filter silently no-op'd. ByShared (real filter, boolean) remains unimplemented, follow-up not fabricated."}
  CreateTieringConfiguration: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GAP CLOSED this pass -- see families.TieringConfiguration for the full redesign"}
  DeleteTieringConfiguration: {wire: ok, errors: ok, state: ok, persist: n/a, note: "see families.TieringConfiguration"}
  GetTieringConfiguration: {wire: ok, errors: ok, state: ok, persist: n/a, note: "see families.TieringConfiguration"}
  ListTieringConfigurations: {wire: ok, errors: ok, state: ok, persist: n/a, note: "see families.TieringConfiguration"}
  UpdateTieringConfiguration: {wire: ok, errors: ok, state: ok, persist: n/a, note: "see families.TieringConfiguration"}
  StartCopyJob: {wire: ok, errors: ok, state: ok, persist: n/a, note: "DEFERRED ITEM CLOSED this pass -- SourceBackupVaultName (a NAME on the real wire) was passed straight into SourceBackupVaultArn with zero resolution/validation (silent data corruption for any real client); now resolved against real vaults (ResourceNotFoundException if either source name or destination ARN don't resolve), and the job now actually materializes a RecoveryPoint in the destination vault (previously a disguised no-op -- CopyJobId was returned but nothing was ever copied). DestinationRecoveryPointArn is now tracked and surfaced via DescribeCopyJob."}
  DescribeCopyJob: {wire: ok, errors: ok, state: ok, persist: n/a, note: "wire response was missing AccountId/ResourceType/IamRoleArn (tracked in the model but silently dropped) and DestinationRecoveryPointArn (not tracked at all); both fixed"}
  ListCopyJobs: {wire: ok, errors: ok, state: ok, persist: n/a, note: "same missing-field fix as DescribeCopyJob, via the same copyJobToJSON helper. gopherstack-i25e (2026-08-29): REQUEST direction fixed -- query filters read under wrong \"by\"-prefixed keys (byState/byResourceArn/byResourceType/byAccountId/byCreatedAfter/byCreatedBefore vs real state/resourceArn/resourceType/accountId/createdAfter/createdBefore, serializers.go:5624-5647) so every filter silently no-op'd; also \"bySourceBackupVaultArn\" was never a real parameter at all (no such field on ListCopyJobsInput) -- the real filter is BySourceRecoveryPointArn -> \"sourceRecoveryPointArn\", filtering by the individual recovery point copied, not its containing vault. Added CopyJob.SourceRecoveryPointArn (populated in StartCopyJob from the recoveryPointArn argument) and rewired the filter onto it. byDestinationVaultArn -> destinationVaultArn also fixed. See wire_field_fixes_test.go."}
  StartRestoreJob: {wire: ok, errors: ok, state: ok, persist: n/a, note: "DEFERRED ITEM CLOSED this pass -- RecoveryPointArn/IamRoleArn/Metadata are all required on the real wire and were previously unvalidated (a request missing all three silently 'succeeded'). Now validated (MissingParameterValueException). Also now enriches ResourceArn/BackupVaultName/BackupVaultArn/BackupSizeInBytes from the tracked source recovery point when known, and synthesizes CreatedResourceArn (real AWS provisions an actual new resource; this emulator cannot, so it fabricates a plausible ARN) -- both were entirely absent before."}
  DescribeRestoreJob: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was a disguised no-op: unknown job IDs returned a fabricated 200 COMPLETED body instead of 404 ResourceNotFoundException (fixed prior pass). This pass: response wire shape extended with AccountId/BackupVaultArn/CreatedResourceArn/ValidationStatus/ValidationStatusMessage -- previously silently dropped or (for ValidationStatus) never wired at all, see PutRestoreValidationResult. FIXED (gopherstack-k26u): restoreJobToJSON emitted \"ResourceArn\"; neither RestoreJobsListMember nor DescribeRestoreJobOutput (backup@v1.59.4 types/types.go:2109-2196, api_op_DescribeRestoreJob.go:39-124) declares that name -- both use SourceResourceArn. A real client's DescribeRestoreJob/ListRestoreJobs silently dropped the key and always saw a nil SourceResourceArn. Fixed at the shared helper (handler_restore_jobs.go); see TestSDKRoundTrip_RestoreJobSourceResourceArn, which drives the real aws-sdk-go-v2 client (a raw-body assertion would only show the value under the wrong key, not prove a real client loses it)."}
  ListRestoreJobs: {wire: ok, errors: ok, state: fixed, persist: n/a, note: "gopherstack-i25e (2026-08-29): was a WORSE variant of the by-prefix bug -- dispatchRestoreJobOps's opListRestoreJobs case called h.Backend.ListRestoreJobs() with no query parameters read at all (not even under a wrong key), so every real client's filter set on this op was silently ignored. Added ListRestoreJobsFilter/ListRestoreJobsFiltered/restoreJobMatchesFilter (restore_jobs.go) and wired accountId/resourceType/status/createdAfter/createdBefore/completeAfter/completeBefore (real ListRestoreJobsInput query keys, serializers.go). parentJobId/restoreTestingPlanArn (also real filters) are NOT implemented: RestoreJob has no field to hold either (StartRestoreJob never receives or fabricates one) -- left as a follow-up rather than fabricating a value. FIXED 2026-08-29 (constraint-not-honoured sweep, same day, follow-on pass): the i25e note above was itself correct that pagination remained unimplemented -- confirmed and fixed. MaxResults/NextToken (real query params, same serializers.go binding) added to ListRestoreJobsFilter and wired through the existing paginateByID helper (already used by ListBackupJobsFiltered/ListCopyJobsFiltered/ListBackupPlansPaged/etc. in this package); ListRestoreJobsFiltered's signature changed to return (jobs, nextToken). Proven via wire_field_fixes_test.go's TestListRestoreJobs_Pagination (real client, asserts a second page returns the remainder and NextToken round-trips, hand-reverted, confirmed failing pre-fix, restored)."}
  PutRestoreValidationResult: {wire: ok, errors: ok, state: ok, persist: n/a, note: "DISGUISED NO-OP FIXED this pass -- wrote ValidationStatus into a side map (b.restoreValidations) that NOTHING ever read; DescribeRestoreJob never reflected a validation result no matter how many times this was called. Side map deleted entirely; result now mutates the RestoreJob record directly (ValidationStatus + ValidationStatusMessage), and an unknown RestoreJobId now correctly returns ResourceNotFoundException instead of silently no-op'ing. responseCode 204 confirmed correct (unchanged)."}
  GetRestoreJobMetadata: {wire: ok, errors: ok, state: ok, persist: n/a, note: "unknown job ID silently returned an empty metadata map with 200 instead of ResourceNotFoundException; fixed"}
  DescribeReportJob: {wire: ok, errors: ok, state: ok, persist: n/a, note: "same fabricated-200 bug as DescribeRestoreJob, fixed"}
  DescribeScanJob: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "same fabricated-200 bug, fixed. 2026-08-21 (gopherstack-r80d batch 11): CORRECTED -- the prior 'wire: ok' verdict only checked the fabricated-200 issue, not this op's 15 required output members; the handler (dispatchReportJobOps's opDescribeScanJob case) returned only ScanJobId/Status. See PARITY's dated overall note and families.ScanJob below."}
  StartScanJob: {wire: ok, errors: ok, state: ok, persist: n/a, note: "body field was BackupVaultArn (doesn't exist on the wire); real input is BackupVaultName. Now resolved to an ARN via DescribeBackupVault before storing. Prior pass: responseCode fixed from 200 to 201 (confirmed via botocore model). gopherstack-m53b (required-member sweep pass 4): five of the six required StartScanJobInput members (IamRoleArn, MalwareScanner, RecoveryPointArn, ScanMode, ScannerRoleArn -- BackupVaultName was already read) were dropped entirely -- api_op_StartScanJob.go:29-75 field-diff confirms all six are 'This member is required', but the handler's ad-hoc decode struct declared only BackupVaultName. Only two of the five (IamRoleArn, RecoveryPointArn) surfaced in the original required-member candidate list; the other three field names (MalwareScanner, ScanMode, ScannerRoleArn) occur elsewhere in the same file for other operations (GetPITRMalwareScanResults, etc.), defeating literal grep matching -- found only by reading the whole op. Fixed: all six required fields plus the three optional ones (ContinuousScanEndTime, IdempotencyToken, ScanBaseRecoveryPointArn) now decoded, validated (MissingParameterValueException per missing field, InvalidParameterValueException for MalwareScanner!=GUARDDUTY or an unrecognized ScanMode -- matching this op's own declared error set: InvalidParameterValueException/InvalidRequestException/LimitExceededException/MissingParameterValueException/ResourceNotFoundException/ServiceUnavailableException per deserializeOpErrorStartScanJob), and threaded into a real StartScanJobInput passed to the backend, which now stores them on ScanJob instead of discarding. Extracted into handleStartScanJob (was an inline switch case) to keep dispatchReportJobOps's cognitive complexity under the gocognit gate. Proven via Test_SDKRoundTrip_StartScanJob (handler_start_scan_job_test.go), which asserts on backend state (not just a 2xx) and fails against the unfixed decode; TestScanJob and the scan_jobs persistence-registered-tables subtest (which called the old two-arg backend signature with none of the five fields) were corrected rather than preserved."}
  DescribeProtectedResource: {wire: ok, errors: ok, state: ok, persist: n/a, note: "same fabricated-200 bug, fixed; see tests for the never-backed-up-resource case"}
  GetRestoreTestingInferredMetadata: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was unroutable (/restore-testing/inferred-metadata doesn't share the /restore-testing/plans prefix)"}
  CreateRestoreTestingPlan: {wire: fixed, errors: ok, state: ok, persist: ok, note: "responseCode fixed from 200 to 201 (confirmed via botocore model). 2026-08-21 (gopherstack-r80d batch 11): CORRECTED -- RecoveryPointSelection (required on RestoreTestingPlanForCreate/-ForGet, and enforced non-nil by the real SDK client's own validateRestoreTestingPlanForCreate) had no struct field at all and was silently discarded. Now parsed/stored (RestoreTestingRecoveryPointSelection, models.go) and returned by GetRestoreTestingPlan -- see families.RestoreTestingPlan."}
  DeleteRestoreTestingPlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "responseCode fixed from 200 to 204 (confirmed via botocore model)"}
  GetRestoreTestingPlan: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "2026-08-21 (gopherstack-r80d batch 11): required member RecoveryPointSelection was entirely absent from the response (no backing field on RestoreTestingPlan) -- fixed, see ops.CreateRestoreTestingPlan/families.RestoreTestingPlan. CreationTime/RestoreTestingPlanArn/RestoreTestingPlanName/ScheduleExpression were already correctly present."}
  UpdateRestoreTestingPlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-21 (gopherstack-r80d batch 11): RecoveryPointSelection is optional on the real RestoreTestingPlanForUpdate (no 'This member is required.' marker) -- now accepted and applied when present, left unchanged when omitted (partial-update semantics, matching this op's real behavior)."}
  ListScanJobs: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "2026-08-21 (gopherstack-r80d batch 11): same fix as DescribeScanJob -- ListScanJobsOutput.ScanJobs is []types.ScanJob, sharing the same 13 required members that were previously dropped to ScanJobId/Status. See ops.DescribeScanJob/families.ScanJob. gopherstack-i25e (2026-08-29): REQUEST direction fixed -- dispatchReportJobOps's opListScanJobs case called h.Backend.ListScanJobs() with no query parameters read at all (same missing-filter defect as ListRestoreJobs, not a wrong-key defect). CORRECTION to this file's own prior gaps note (2026-08-29 timestamp-pattern-hunt entry, now removed): that note claimed ListScanJobs strips the \"by\" prefix like its siblings -- it does NOT. ListScanJobs is the one op in this service where serializers.go keeps the full PascalCase Go field name on the wire (ByAccountId, ByBackupVaultName, ByCompleteAfter, ByCompleteBefore, ByMalwareScanner, ByRecoveryPointArn, ByResourceArn, ByResourceType, ByState, MaxResults, NextToken -- none lowercased or stripped). Verified directly against serializers.go rather than assumed from the sibling pattern. Added ListScanJobsFilter/ListScanJobsFiltered/scanJobMatchesFilter (restore_testing.go) wired under the correct PascalCase keys (ScanJobsFilterFromQuery, handler_report_plans.go). ByScanResultStatus (real filter) is NOT implemented: ScanJob has no field to hold a scan result status -- follow-up, not fabricated. FIXED 2026-08-29 (constraint-not-honoured sweep, same-day follow-on pass): MaxResults/NextToken -- also query-bound PascalCase per the same serializers.go binding this file already confirmed -- were never read either, same missing-filter defect as ListRestoreJobs's pagination. Added to ListScanJobsFilter, wired through paginateByID; ListScanJobsFiltered now returns (jobs, nextToken). Proven via wire_field_fixes_test.go's TestListScanJobs_Pagination (real client, hand-reverted, confirmed failing pre-fix, restored)."}
  CreateRestoreTestingSelection: {wire: ok, errors: ok, state: ok, persist: ok, note: "DEFERRED ITEM CLOSED this pass -- IamRoleArn (required on the real wire) was entirely absent from the model and unvalidated; ProtectedResourceType map[string]any-free-form ControlInputParameters-style bugs did NOT apply here (this family never had that bug), but ProtectedResourceArns/ProtectedResourceConditions (StringEquals/StringNotEquals []KeyValue)/RestoreMetadataOverrides/ValidationWindowHours were all missing from the model and wire parsing. All added, field-diffed against types.RestoreTestingSelectionForCreate. responseCode fixed from 200 to 201."}
  UpdateRestoreTestingSelection: {wire: ok, errors: ok, state: ok, persist: ok, note: "same field additions as Create; ProtectedResourceType is correctly left untouched on Update now (immutable per types.RestoreTestingSelectionForUpdate -- the prior implementation let it be silently changed, which real AWS does not allow)"}
  DeleteRestoreTestingSelection: {wire: ok, errors: ok, state: ok, persist: ok, note: "responseCode fixed from 200 to 204 (confirmed via botocore model)"}
  DisassociateRecoveryPointFromParent: {wire: ok, errors: ok, state: ok, persist: n/a, note: "responseCode fixed from 200 to 204 (confirmed via botocore model)"}
  CancelLegalHold: {wire: ok, errors: ok, state: ok, persist: ok, note: "responseCode fixed from 200 to 201 -- unusual for a DELETE but confirmed directly against the botocore service model's explicit http.responseCode"}
  CreateFramework: {wire: ok, errors: ok, state: ok, persist: ok, note: "DEFERRED ITEM CLOSED this pass -- ControlInputParameters was modeled/parsed as map[string]string; real wire shape is []ControlInputParameter ({ParameterName,ParameterValue} pairs). ControlScope was modeled as a free-form map[string]any; real wire shape is a struct {ComplianceResourceIds,ComplianceResourceTypes,Tags}. Both were WRONG SHAPES that would fail against any real aws-sdk-go-v2 client sending the real request shape, or silently mis-decode responses. Fixed, field-diffed against types.FrameworkControl/types.ControlScope."}
  UpdateFramework: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED this pass -- FrameworkControls was not even accepted by UpdateFramework (real UpdateFrameworkInput.FrameworkControls lets you replace a framework's controls); now supported, omitted-field-means-unchanged"}
  CreateReportPlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "ReportDeliveryChannel was missing S3KeyPrefix; ReportSetting was missing Accounts/OrganizationUnits/Regions/NumberOfFrameworks. All added, field-diffed against types.ReportDeliveryChannel/types.ReportSetting."}
  UpdateReportPlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP CLOSED this pass -- ReportDeliveryChannel/ReportSetting were not accepted by UpdateReportPlan at all (only description); real UpdateReportPlanInput accepts both. Now supported, omitted-field-means-unchanged."}
  GetPITRMalwareScanResults: {wire: partial, errors: ok, state: ok, persist: n/a, note: "NEW this pass (GET /scan/pitr-malware-scan-results, confirmed from serializers.go's awsRestjson1_serializeOpGetPITRMalwareScanResults path literal; all 4 input members -- BackupVaultName/MalwareScanner/RecoveryPointArn/ScanEndTime -- are query-string params per awsRestjson1_serializeOpHttpBindingsGetPITRMalwareScanResultsInput, not path segments or a JSON body, field-diffed against GetPITRMalwareScanResultsInput/Output and types.ScanResultInfo/ScanResultStatus). Real state validated: BackupVaultName resolved via DescribeBackupVault, RecoveryPointArn validated against that vault via DescribeRecoveryPoint -- both genuinely fail (400 ResourceNotFoundException, matching this service's uniform 400-for-not-found convention -- see errors.go) for an unknown vault or recovery point, not accepted verbatim. No malware scanning engine exists in this backend (GuardDuty malware-protection integration is out of scope/unmodeled), so ScanResult.ScanResultStatus is always the SDK's own 'UNKNOWN' enum value -- never a fabricated NO_THREATS_FOUND/THREATS_FOUND verdict, infected-file count, or threat name. ScanId/ScanMode/LastScanJobTime (all optional output members) are omitted entirely rather than populated with an invented ID/mode/timestamp. wire: partial reflects that these three optional members are never populated (by design, not oversight) rather than a genuine wire-shape defect -- ScanEndTime (required) and ScanResult (required) are both correctly present and accurate."}
  ListBackupJobSummaries: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-jqh2: GAP CLOSED -- GET /audit/backup-job-summaries had real handler+backend code (handler_backup_jobs.go) but was NEVER routed; parseBackupPath/parseBackupJobFamilyPath had no case for any /audit/*-job-summaries path, so every real client request 404'd. Route added. Already groups by State (backup_jobs.go); AccountId/AggregationPeriod/MessageCategory filters remain unimplemented -- disclosed gap, not fixed this pass (see gaps)."}
  ListCopyJobSummaries: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-jqh2: same unroutable bug as ListBackupJobSummaries (GET /audit/copy-job-summaries); fixed alongside it. Already groups by State (copy_jobs.go); same disclosed AccountId/AggregationPeriod/MessageCategory gap as ListBackupJobSummaries."}
  ListRestoreJobSummaries: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "gopherstack-jqh2: same unroutable bug as ListBackupJobSummaries (GET /audit/restore-job-summaries); fixed alongside it. FIXED 2026-08-29 (constraint-not-honoured sweep): unlike its ListBackupJobSummaries/ListCopyJobSummaries siblings, this op never grouped by State at all -- the handler called the plain ListRestoreJobs() accessor and always returned exactly one fabricated {Count, Region} entry for the WHOLE job set, silently dropping State/AccountId (both required members on real RestoreJobSummary, api_op_ListRestoreJobSummaries.go) regardless of how many distinct states existed. Added ListRestoreJobSummaries() (restore_jobs.go), grouping by Status the same way the Backup/Copy siblings already do. AccountId/AggregationPeriod filters remain unimplemented, same disclosed gap as the siblings. Proven via wire_field_fixes_test.go's TestListRestoreJobSummaries_State (real client, hand-reverted, confirmed failing pre-fix, restored)."}
  ListScanJobSummaries: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "gopherstack-jqh2: same unroutable bug as ListBackupJobSummaries (GET /audit/scan-job-summaries); fixed alongside it. FIXED 2026-08-29 (constraint-not-honoured sweep): the most degenerate of the four -- returned a single {Count} entry with no Region/AccountId/State at all (real ScanJobSummary, api_op_ListScanJobSummaries.go, requires AccountId/Count/Region/State at minimum). Added ListScanJobSummaries() (restore_testing.go), grouped by Status matching the Backup/Copy/Restore siblings. MalwareScanner/ScanResultStatus grouping and AggregationPeriod filtering remain unimplemented (ScanJob doesn't track a scan-result outcome at all -- see the ScanJob type doc). Proven via wire_field_fixes_test.go's TestListScanJobSummaries_State."}
  ListProtectedResources: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-30 (tie-prone-sort audit): GET /resources ignored MaxResults/NextToken entirely, always returning every protected resource in one response -- a real client's MaxResults was silently dropped. Now paginated via paginateByID over the existing sort-by-ResourceArn order. See wrapper-key-sweep header note above for detail."}
  ListProtectedResourcesByBackupVault: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "gopherstack-jqh2: GAP CLOSED -- GET /backup-vaults/{BackupVaultName}/resources had real handler+backend code (handler_protected_resources.go) but vaultSubRoute's suffix list never included \"/resources\", so the op was unreachable from any real path. Route added. FIXED 2026-08-30 (tie-prone-sort audit): once reachable, still ignored MaxResults/NextToken like its ListProtectedResources sibling; same fix applied. See wrapper-key-sweep header note above."}
families:
  BackupVault: {status: ok, note: "CRUD, AccessPolicy, Notifications, Lock all verified against real paths/methods and already correct. mpaApprovalTeam Associate/Disassociate both fixed to responseCode 204 this pass (see ops). DescribeBackupVault field-diffed and extended (EncryptionKeyType, MpaApprovalTeamArn) this pass. FIXED (gopherstack-hnyl): PutBackupVaultNotifications's validVaultEvents was a hand-copied 17-entry allowlist that misspelled COPY_JOB_FAILED as \"COPY_JOB_FAILURE\" (an existing test, TestVaultNotificationsEventValidation/all_valid_event_types, encoded the same typo as a valid input -- fixed alongside the source) and was missing 7 newer types.BackupVaultEvent members (CONTINUOUS_BACKUP_INTERRUPTED, the three RECOVERY_POINT_INDEX* events, and the three EKS_* events). Now derives from types.BackupVaultEvent.Values()."}
  BackupPlan: {status: ok, note: "CRUD + versions + selections verified against real paths; already correct."}
  BackupSelection: {status: ok, note: "verified against real paths; already correct."}
  BackupJob: {status: ok, note: "StartBackupJob created jobs that stayed in CREATED forever -- CompleteBackupJob (state transition + recovery-point creation) existed as dead code, never called from anywhere. Janitor now advances CREATED jobs to COMPLETED on each sweep tick (mirrors services/batch's advanceJobs pattern), so StartBackupJob itself stays synchronous/CREATED (matching AWS) while background completion actually happens. StopBackupJob was unroutable (fixed)."}
  RecoveryPoint: {status: ok, note: "list/describe/delete/disassociate/restore-metadata routes were already correct. Index details/settings and lifecycle-update ops were entirely unroutable (no route emitted these op names) and, once reached directly, passed an empty vaultName to the backend. UpdateRecoveryPointLifecycle's write additionally went to a write-only side map (recoveryPointLifecycle) that nothing ever read -- DescribeRecoveryPoint never reflected the update. Fixed: real routes added, vaultName wiring fixed, backend now mutates RecoveryPoint.Lifecycle/CalculatedLifecycle directly, CalculatedLifecycle is now computed (MoveToColdStorageAfterDays/DeleteAfterDays -> timestamps from CreationDate) and serialized as epoch-seconds. This pass: AddRecoveryPoint (public helper used by CopyJob/RestoreJob enrichment and tests) was found to never populate BackupVaultArn from the vault it's added to -- fixed."}
  Tags: {status: ok, note: "TagResource/ListTags correct. UntagResource was routed as DELETE /tags/{arn}; real AWS uses POST /untag/{arn} -- completely different path, so every real SDK client's UntagResource call 404'd. Fixed."}
  Framework: {status: ok, note: "CRUD verified against real paths. DEFERRED ITEM CLOSED this pass: FrameworkControl.ControlInputParameters/ControlScope were wrong wire shapes (map instead of array/struct); UpdateFramework didn't accept FrameworkControls at all. Both fixed and field-diffed against types.FrameworkControl/types.ControlScope -- see ops.CreateFramework/UpdateFramework."}
  ReportPlan: {status: ok, note: "CRUD verified against real paths. DEFERRED ITEM CLOSED this pass: ReportDeliveryChannel/ReportSetting were missing fields (S3KeyPrefix; Accounts/OrganizationUnits/Regions/NumberOfFrameworks) and UpdateReportPlan didn't accept either at all. Both fixed -- see ops.CreateReportPlan/UpdateReportPlan."}
  RestoreTestingPlan: {status: ok, note: "CRUD + selections verified against real paths. GetRestoreTestingInferredMetadata was unroutable (fixed prior pass). This pass: responseCodes fixed (Create 201, Delete 204) across plan+selection ops; RestoreTestingSelection DEFERRED ITEM CLOSED -- see ops.CreateRestoreTestingSelection. 2026-08-21 (gopherstack-r80d batch 11): RecoveryPointSelection (required on RestoreTestingPlanForGet, types.go:2307-2371) had no field at all -- CreateRestoreTestingPlanInput's own client-side validator requires it non-nil, so every real client's plan has one, but gopherstack silently dropped it. Fixed: RestoreTestingRecoveryPointSelection added to models.go, threaded through Create (required)/Update (optional, partial-update semantics)/Get (now emitted). Not required on RestoreTestingPlanForList (types.go:2376-2419, no such member exists there), so ListRestoreTestingPlans correctly omits it. RestoreTestingSelectionForGet/-ForList's required members (CreationTime/IamRoleArn/ProtectedResourceType/RestoreTestingPlanName/RestoreTestingSelectionName) were already all correctly present -- IamRoleArn is emitted conditionally via setOptionalStr but is never reachably empty, since CreateRestoreTestingSelection's own validation already rejects an empty IamRoleArn (ErrValidation) before a selection is ever stored."}
  LegalHold: {status: ok, note: "CreateLegalHold/CancelLegalHold were routed; GetLegalHold/ListLegalHolds/ListRecoveryPointsByLegalHold were never routed at all despite full handler code existing. Fixed prior pass. This pass: CreateLegalHold now accepts RecoveryPointSelection and ListRecoveryPointsByLegalHold actually filters by it (GAP CLOSED, was unconditional empty list); CancelLegalHold responseCode fixed 200->201."}
  ScanJob: {status: partial, note: "NEW 2026-08-21 (gopherstack-r80d batch 11). DescribeScanJob/ListScanJobs (dispatchReportJobOps's opDescribeScanJob/opListScanJobs cases, handler_report_plans.go) returned only ScanJobId/Status -- a PARITY 'wire: ok' verdict on ops.DescribeScanJob had only checked an unrelated fabricated-200-status bug, missing that 12 of DescribeScanJobOutput's 15 required members (api_op_DescribeScanJob.go:39-146) were dropped even though the backend already tracked most of them on ScanJob (models.go). Fixed via a shared scanJobToJSON helper emitting every tracked required member; AccountId/BackupVaultName/ResourceArn/ResourceType/ResourceName had no backing field at all and are now derived without fabrication -- AccountId from the backend's own account, BackupVaultName from the StartScanJob request, ResourceArn/ResourceType from the recovery point input.RecoveryPointArn identifies (via the existing findRecoveryPointByArn helper), ResourceName from that ARN's trailing segment. status: partial because CreatedBy (types.ScanJobCreator, also required) cannot be honestly derived -- see the residual_gaps entry below -- so this op still cannot return a byte-for-byte-complete required response; every member this backend can source without fabricating one is now correct."}
  RouteMatcher: {status: ok, note: "matchesBackupPath (the RouteMatcher gate -- see pkgs/service/registry.go, this is the ONLY thing that decides whether a request ever reaches this service's Handler) was missing /resources, /restore-jobs, /report-jobs(audit), /scan/jobs+/scan/job, /global-settings, /account-settings, /supported-resource-types, /tiering-configurations(prefix), /indexes/recovery-point, and /untag entirely. Fixed prior pass. This pass: /tiering-configurations path text itself was ALSO wrong (was \"/backup-vault-tiering\", a gopherstack-invented path -- fixed as part of the TieringConfiguration redesign) and /logically-air-gapped-backup-vaults now additionally routes the nested restore-access-vault sub-paths (already covered by the existing prefix, no RouteMatcher change needed there). gopherstack-jqh2: added TestExtractOperation_SDKRouteTable (handler_paths_sdk_diff_test.go), a permanent per-op method+path diff against all 109 real backup ops extracted from backup@v1.59.4 serializers.go. Found and fixed 5 previously-invisible unroutable ops -- see ops.ListBackupJobSummaries/ListCopyJobSummaries/ListRestoreJobSummaries/ListScanJobSummaries/ListProtectedResourcesByBackupVault -- all had real handler+backend code that a request could simply never reach; the prior route-matcher sweep that produced this file's history checked family-level path prefixes and the RouteMatcher gate, not every individual op's literal path, so these were missed. DisassociateBackupVaultMpaApprovalTeam's real path carries a bare \"?delete\" query flag (distinguishing it from Associate at the same base path); confirmed the existing method-only (POST vs PUT) discrimination is sufficient and correct, no query-flag check needed. No wrong-API-date-prefix or duplicate op-resolution-table issues found; this service has neither IAM-action nor CloudTrail-naming secondary tables to drift."}
  TieringConfiguration: {status: ok, note: "GAP CLOSED this pass -- full backend redesign. Real API keys tiering configs by TieringConfigurationName (CreateTieringConfigurationInput.TieringConfiguration nests BackupVaultName+ResourceSelection inside), gopherstack previously keyed by vault name with no TieringConfigurationName/ResourceSelection concept at all -- a completely different (invented) data model. Routing path was also wrong (\"/backup-vault-tiering\" instead of the real \"/tiering-configurations\", \"/tiering-configurations/{Name}\"). Redesigned: TieringConfiguration now keyed by TieringConfigurationName, ResourceSelection ([]{ResourceType,Resources,TieringDownSettingsInDays}) validated (60-36500 day range, matching AWS docs), routing fixed (Create is PUT on the bare collection -- name lives in the body, not the URL -- Get/Update/Delete address by name in the path), CreatorRequestId idempotency added. Field-diffed against types.TieringConfiguration/TieringConfigurationInputForCreate/-ForUpdate/TieringConfigurationsListMember."}
  RestoreAccessVault: {status: fixed, note: "GAP CLOSED this pass -- List/Revoke were routed against the WRONG (flat, invented) /restore-access-backup-vaults collection; real paths nest both under the source air-gapped vault (/logically-air-gapped-backup-vaults/{BackupVaultName}/restore-access-backup-vaults[/{arn}]), scoped per-source-vault (there is no list-all/revoke-any-vault op in the real API). Backend now tracks SourceBackupVaultName (resolved from the ARN at Create time) and both List and Revoke correctly scope/reject by it. Create's SourceBackupVaultArn is now validated against real vaults instead of stored verbatim. gopherstack-muzq (2026-08-21): VaultState (real aws-sdk-go-v2/service/backup/types.VaultState: CREATING|AVAILABLE|FAILED) was stamped CREATING at construction and nothing else in this backend ever wrote to it -- confirmed via ListRestoreAccessBackupVaults, which echoes the stored VaultState verbatim. Fixed by extending the existing Janitor (janitor.go) with advanceRestoreAccessVaults, run every SweepOnce alongside advanceCreatedJobs, moving CREATING -> AVAILABLE. New test TestRestoreAccessVaultCreate_ReachesAvailable asserts the terminal AVAILABLE state after a sweep, not just the correct initial CREATING which no prior test checked at all."}
  CopyJob: {status: ok, note: "DEFERRED ITEM CLOSED this pass -- StartCopyJob's SourceBackupVaultName (wire: a NAME) was stored directly into the ARN field with zero resolution, and the 'copy' never actually created anything in the destination vault (CopyJobId was returned but DescribeRecoveryPoint against the destination vault would never see it -- a disguised no-op per parity-principles.md #2). Now: source name and destination ARN are both resolved/validated against real vaults, and a real RecoveryPoint is materialized in the destination vault with a tracked DestinationRecoveryPointArn. DescribeCopyJob/ListCopyJobs wire responses extended to surface AccountId/ResourceType/IamRoleArn/DestinationRecoveryPointArn (previously tracked-but-dropped or not tracked at all)."}
  RestoreJob: {status: ok, note: "DEFERRED ITEM CLOSED this pass -- StartRestoreJob accepted requests missing all of RecoveryPointArn/IamRoleArn/Metadata (all required on the real wire) with no validation. PutRestoreValidationResult was a disguised no-op (wrote to a side map, b.restoreValidations, that DescribeRestoreJob never read -- deleted the side map, wired the result directly onto the RestoreJob record). StartRestoreJob now also enriches from the tracked source recovery point and synthesizes CreatedResourceArn. DescribeRestoreJob/ListRestoreJobs wire responses extended with AccountId/BackupVaultArn/CreatedResourceArn/ValidationStatus/ValidationStatusMessage. FIXED (gopherstack-k26u): the shared restoreJobToJSON helper emitted \"ResourceArn\" where both RestoreJobsListMember and DescribeRestoreJobOutput declare SourceResourceArn -- DescribeRestoreJob and ListRestoreJobs (and ListRestoreJobsByProtectedResource, same helper) were wrong identically. Renamed to SourceResourceArn; see TestSDKRoundTrip_RestoreJobSourceResourceArn."}
  timestamps: {status: ok, note: "Pattern-hunt pass (timestamp encoding class, 2026-08-29): protocol confirmed REST-JSON (awsRestjson1_* serializer prefix, backup@v1.59.4). Body response fields: every *time.Time deserializer call in deserializers.go is smithytime.ParseEpochSeconds (73 occurrences across types.go + api_op_*.go); gopherstack's epochSeconds() helper (handler_dispatch.go) wraps every body-response timestamp as a float64 before it reaches c.JSON -- confirmed no map[string]any/response struct anywhere in handler_*.go assigns a raw time.Time to a Date/Time-suffixed key. Query-string request filters (ByCreatedAfter/ByCompleteAfter/etc. on the List* ops, plus GetPITRMalwareScanResults.ScanEndTime) are the one place this protocol uses ISO8601 instead of epoch -- serializers.go encodes them via smithytime.FormatDateTime, not FormatEpochSeconds -- and gopherstack's ParseTimeFilter/the GetPITRMalwareScanResults handler both correctly parse with time.RFC3339, which accepts FormatDateTime's fixed-Z / optional-fractional-second output (verified with a throwaway time.Parse repro). 0 wrong-format bugs found in either direction. See gaps for an adjacent, unfixed non-format bug (wrong query key names) found in the same code path."}
gaps: []
  # FIXED 2026-08-29 (gopherstack-i25e): the wrong-query-key gap previously
  # recorded here (ListBackupJobs/ListCopyJobs/ListRecoveryPointsByBackupVault/
  # ListBackupVaults reading "by"-prefixed keys the real wire doesn't have) is
  # closed -- see ops.ListBackupJobs/ListCopyJobs/ListRecoveryPointsByBackupVault/
  # ListBackupVaults. That note also misidentified the affected op as
  # "ListRestoreJobsByProtectedResource" and claimed ListScanJobs strips the
  # "by" prefix -- both wrong on re-verification: ListRestoreJobs (not
  # ListRestoreJobsByProtectedResource) and ListScanJobs (not ListRestoreJobs)
  # read NO query filters at all, and ListScanJobs keeps its full PascalCase
  # "By..." keys on the wire rather than stripping them -- see ops.ListRestoreJobs
  # /ListScanJobs for the corrected, individually-verified fixes.
  # All 4 gaps from the 2026-07-12 audit are now closed with real fixes + tests:
  #   TieringConfiguration data model -> families.TieringConfiguration
  #   RestoreAccessVault List/Revoke paths -> families.RestoreAccessVault
  #   ListRecoveryPointsByLegalHold empty-list -> ops.ListRecoveryPointsByLegalHold
  #   DescribeBackupVault missing MPA/EncryptionKeyType fields -> ops.DescribeBackupVault
  # New residual gap found and left open this pass (see below).
residual_gaps:
  - "2026-08-29 (constraint-not-honoured sweep): ListBackupJobSummaries/ListCopyJobSummaries/ListRestoreJobSummaries/ListScanJobSummaries all ignore AccountId, AggregationPeriod, and MessageCategory (ListBackupJobSummaries/ListCopyJobSummaries only) -- real filters/grouping keys on all four ops (backup@v1.59.4 api_op_List*JobSummaries.go). AccountId/MessageCategory filtering was left unimplemented consistent with the existing precedent immediately below (ListBackupJobs' own messageCategory gap) rather than adding filtering logic this backend can't yet exercise meaningfully (MessageCategory is hardcoded to 'SUCCESS' on every job, see ListBackupJobs' gap note). AggregationPeriod (ONE_DAY/SEVEN_DAYS/FOURTEEN_DAYS historical day-bucketed counts) is the larger gap: this backend produces one point-in-time snapshot per call, not a time series, so honoring it would mean building a new historical-bucketing model across all four job types -- reported as too large for this pass rather than rushed or fabricated. What WAS fixed this pass: ListRestoreJobSummaries/ListScanJobSummaries previously didn't even group by State (always one fabricated {Count} entry for the whole job set, dropping State/AccountId/Region entirely) -- now match the State-grouping ListBackupJobSummaries/ListCopyJobSummaries already had. ListRestoreJobs/ListScanJobs pagination (MaxResults/NextToken, distinct from the Summaries ops above) was also found never read at all and fixed in the same pass -- see ops.ListRestoreJobs/ListScanJobs."
  - "DescribeBackupVault still omits MpaSessionArn and LatestMpaApprovalTeamUpdate. This backend's AssociateBackupVaultMpaApprovalTeam only ever stores an MpaApprovalTeamArn string (b.mpaApprovals map[string]string) -- there is no modeled MPA-session-approval workflow (session creation, approval status, expiry) anywhere in this service to source MpaSessionArn/LatestMpaApprovalTeamUpdate from. Populating them would mean fabricating session/approval state that isn't backed by any real API call in this emulator (CreateRestoreAccessBackupVault is MPA-adjacent but doesn't create an approval-team *session*) -- left genuinely open rather than invented. Real fix needs a broader MPA-session model, out of scope for a single-pass field-diff."
  - "STALE, RE-VERIFIED 2026-08-23 (batch9 audit): this note claimed ListBackupPlanVersions/ExportBackupPlanTemplate 'silently swallow backend not-found errors and return an empty-but-200 response instead of propagating ResourceNotFoundException'. Reading handler_backup_plans.go's dispatchPlanTemplateCatalogOps today shows both opListBackupPlanVersions and opExportBackupPlanTemplate cases already check `if err != nil` and return `http.StatusBadRequest` with `errResp(\"ResourceNotFoundException\", ...)` explicitly (not via handleError, but correctly inline) -- there is no empty-200 path. TestListBackupPlanVersions_NotFound (handler_backup_plans_test.go) and TestExportBackupPlanTemplate_UnknownPlanNotFound (handler_templates_test.go) both already assert this. The bug this note described either predates the current handler_backup_plans.go content or was fixed by a later, uncited pass without this note being updated -- classic 'already fixed lower/later in the file' staleness. No code change needed; correcting the record only."
  - "GetPITRMalwareScanResults has no malware scanning engine backing it (this emulator does not integrate with GuardDuty malware protection). ScanResultStatus is always 'UNKNOWN' and ScanId/ScanMode/LastScanJobTime are always absent -- an honest, documented limitation (see ops.GetPITRMalwareScanResults), not a hidden gap. Also: recovery points are not checked for continuous-backup/PITR eligibility (this backend has no EnableContinuousBackup-style flag on RecoveryPoint) -- a recovery point that would not actually support PITR in real AWS is still accepted here as long as it exists."
  - "DescribeScanJob/ListScanJobs's required CreatedBy member (types.ScanJobCreator: BackupPlanArn/BackupPlanId/BackupPlanVersion/RuleId) is never populated -- gopherstack-r80d batch 11. This backend has no association between a scan job (or the recovery point it targets) and an originating backup plan/rule: RecoveryPoint doesn't track which plan/rule created it, and StartScanJobInput itself carries no plan/rule reference for a real client to supply one. Fabricating plan/rule IDs would violate the no-fabrication rule, so this required member stays honestly absent rather than invented -- everything else DescribeScanJob/ListScanJobs are required to return (AccountId/BackupVaultArn/BackupVaultName/CreationDate/IamRoleArn/MalwareScanner/RecoveryPointArn/ResourceArn/ResourceName/ResourceType/ScanMode/ScannerRoleArn/State) is now populated (see ops.DescribeScanJob, families.ScanJob)."
  - "gopherstack-i25e (2026-08-29): ListBackupPlans ignores IncludeDeleted (real ListBackupPlansInput query filter, serializers.go: `includeDeleted` -- key itself is not the by-prefix bug, this op was never affected by that). DeleteBackupPlan hard-removes the record from the store (no DeletionDate retained anywhere) so there is no honest way to serve IncludeDeleted=true without a soft-delete model change -- left open rather than fabricating deleted-plan records. Filed as a follow-up, not fixed this pass (out of the by-prefix bug's scope)."
  - "gopherstack-i25e (2026-08-29): ListRestoreJobs and ListScanJobs still ignore MaxResults/NextToken (both real query params on both ops) -- neither op paginates, both return every matching record in one response. This predates this pass (ListBackupJobs/ListCopyJobs/ListRecoveryPointsByBackupVault/ListBackupVaults already paginate via the existing paginateByID helper) and is a distinct defect from the query-filter-key bug this pass fixed; left open as a follow-up."
  - "gopherstack-i25e (2026-08-29): CopyJob.SourceRecoveryPointArn (added this pass to fix the ListCopyJobs BySourceRecoveryPointArn filter -- REQUEST direction) is not yet surfaced in copyJobToJSON's RESPONSE body, even though it's a real member of types.CopyJob. Left as a response-direction follow-up; this pass's scope was verified REQUEST-direction only per the parity_principles wire-shape rule (a bare 'wire: ok' having previously been found to mean response-only)."
deferred: []
  # All 4 deferred items from the 2026-07-12 audit are now closed with real
  # fixes + tests (see the matching families/ops entries above):
  #   CopyJob response-shape/error-code depth -> families.CopyJob
  #   RestoreJob family beyond DescribeRestoreJob -> families.RestoreJob
  #   Framework/ReportPlan nested shapes -> families.Framework / families.ReportPlan
  #   RestoreTestingSelection deep shape -> ops.CreateRestoreTestingSelection
leaks: {status: clean, note: "Janitor's advanceCreatedJobs takes the backend RLock, copies job IDs, releases, then calls CompleteBackupJob per ID (which takes its own Lock) -- no lock held across the loop, no goroutine leak. This pass added no new goroutines/tickers; all new Lock()/RLock() call sites (TieringConfiguration CRUD, RestoreAccessVault List/Revoke, CopyJob/RestoreJob enrichment via findRecoveryPointByArn, PutRestoreValidationResult) follow the existing single-Lock-per-call, defer-Unlock, no-nested-backend-lock-calls pattern -- verified by reading each new method body. -race clean (go test -race -count=1)."}
---

## Notes

- **Protocol**: restjson1. Timestamps are epoch-seconds JSON numbers via the
  handler's local `epochSeconds()` helper (equivalent to `pkgs/awstime.Epoch`,
  just not routed through that package -- functionally correct, a style-only
  divergence from the catalog's preferred helper).
- **AWS Backup does NOT use 404/409 for not-found/conflict -- it's 400 for
  everything client-fault.** This was the single biggest finding this pass,
  verified against botocore's `backup/2018-11-15/service-2.json` (the
  authoritative AWS service model): none of Backup's client-fault exceptions
  (`ResourceNotFoundException`, `AlreadyExistsException`,
  `InvalidParameterValueException`, `MissingParameterValueException`,
  `InvalidRequestException`, `LimitExceededException`, `ConflictException`)
  carry an explicit `httpStatusCode` override in the model, so the restJson1
  protocol default of **400** applies uniformly to all of them. Compare e.g.
  Lambda's `ResourceNotFoundException`, which the same botocore model
  encodes with an explicit `"httpStatusCode": 404` -- Backup's has no such
  override. This service's `handleError()` previously returned 404 for
  not-found and 409 for already-exists, matching the *common* REST-JSON
  convention but not Backup's actual behavior. Fixed centrally in
  `handleError()`, plus ~90 direct (non-`handleError`-routed) call sites that
  hardcoded the same wrong assumption.
- **`ValidationException` is not a real AWS Backup error code** -- it does
  not appear anywhere in the service model. It was a gopherstack invention
  used at ~90 call sites. Deleted; replaced with the real generic codes
  `MissingParameterValueException` (message names a required field) and
  `InvalidParameterValueException` (everything else), both confirmed present
  in the real per-operation error lists. `errors.go`'s `ErrValidation`
  sentinel's label was updated to `InvalidParameterValueException`
  accordingly; a new `ErrInvalidRequest` sentinel (`InvalidRequestException`)
  was added for genuine state-conflict validation failures (e.g. deleting a
  non-empty or locked vault), distinct from malformed-parameter failures.
- **Several success responseCodes were wrong** (found via the same botocore
  model, which encodes explicit `http.responseCode` for the operations that
  deviate from the restJson1 default of 200): `AssociateBackupVaultMpaApprovalTeam`
  /`DisassociateBackupVaultMpaApprovalTeam`/`DisassociateRecoveryPointFromParent`
  /`DeleteRestoreTestingPlan`/`DeleteRestoreTestingSelection` are 204, not 200;
  `CreateRestoreTestingPlan`/`CreateRestoreTestingSelection`/`StartScanJob` are
  201, not 200; `CancelLegalHold` is (unusually, for a DELETE) 201, not
  200/204. All fixed. **Trap for the next auditor**: don't assume 200 for
  every 2xx response in this service -- check the botocore model's
  `operations.<Op>.http.responseCode` field explicitly per op.
- **Local "not found" error sentinels that don't wrap the shared `ErrNotFound`
  are a live bug class in this service, not just a style inconsistency.**
  Several ops (`GetTieringConfiguration`/`DeleteTieringConfiguration` before
  this pass, `DescribeRestoreJob`/`PutRestoreValidationResult` before this
  pass) used a locally-defined `errors.New("xxx not found")` sentinel instead
  of wrapping the shared `ErrNotFound`. Where the handler calls `h.handleError`
  (the normal path), an unwrapped local sentinel falls through to the
  `default` case and returns `500 InternalFailure` instead of the correct
  `400 ResourceNotFoundException` -- this was a real, live bug for
  `PutRestoreValidationResult`'s not-found path before this pass's fix (it
  hadn't been wired to call `handleError` at all yet, so it was latent, but
  would have surfaced the instant that wiring was added without also fixing
  the sentinel). Fixed the two op families above by wrapping `ErrNotFound`
  directly and removing the now-orphaned local sentinels
  (`errTieringConfigNotFound`, `errRestoreJobNotFound`). **Two more local
  sentinels of this shape remain** (`errRecoveryPointNotFound` appears
  unused/dead, `errBackupPlanNotFoundB1` is used by `ListBackupPlanVersions`/
  `ExportBackupPlanTemplate`, which don't call `handleError` at all --  they
  silently swallow the error into an empty-200 response instead, a *different*
  bug, see residual_gaps) -- not touched this pass, out of the 4+4 scope, but
  worth a dedicated look.

  **CORRECTED 2026-08-23 (batch9): both halves of the above paragraph's
  "two more remain" claim were wrong.** `errRecoveryPointNotFound` was NOT
  unused/dead -- it, and its sibling `errVaultNotFoundB1`, are both used
  exclusively inside `UpdateRecoveryPointLifecycle` (`recovery_points.go`),
  which DOES call `h.handleError` (`handler_recovery_points.go`'s
  `handleUpdateRecoveryPointLifecycle`, line ~351) -- this was a live,
  reachable 500-instead-of-400 bug, not a dead sentinel. Fixed; see
  `ops.UpdateRecoveryPointLifecycle` above. Separately, `errBackupPlanNotFoundB1`'s
  claimed "silently swallow the error into an empty-200 response" bug does
  NOT match the current `dispatchPlanTemplateCatalogOps` (`handler_backup_plans.go`)
  -- both its `opListBackupPlanVersions` and `opExportBackupPlanTemplate` cases
  already check `err != nil` and return `400 ResourceNotFoundException`
  inline (not via `handleError`, but correctly), and
  `TestListBackupPlanVersions_NotFound`/`TestExportBackupPlanTemplate_UnknownPlanNotFound`
  already assert this. See the corrected `residual_gaps` entry below --
  that bug was already fixed by the time this pass re-read the code, an
  "already fixed lower/later in the file" staleness, not a genuine
  reachable bug today.
- **RouteMatcher is the real gate, not parseBackupPath.** `matchesBackupPath`
  (`Handler.RouteMatcher()`) decides whether `pkgs/service/registry.go` ever
  calls this service's `Handler()` at all. `parseBackupPath` can have perfectly
  correct logic for a path family and it will STILL never run if that family's
  prefix/exact isn't also listed in `matchesBackupPath`. This was the single
  biggest source of bugs in the 2026-07-12 audit and is invisible to this
  package's own unit tests because `doREST()`/`doBatch1Request()` call
  `h.Handler()(c)` directly, skipping the matcher entirely. **Any future
  audit that adds a new top-level path constant MUST also add it to
  `matchesBackupPath`'s `prefixes`/`exacts` slices, or it will silently never
  receive real traffic.**
- **AWS Backup's UntagResource path is `/untag/{ResourceArn}` (POST), not
  `/tags/{ResourceArn}` (DELETE).** Easy to get wrong by analogy with other
  services that DO use DELETE /tags/{arn} for untag -- Backup doesn't.
- **StopBackupJob shares its path with DescribeBackupJob**: `GET
  /backup-jobs/{BackupJobId}` describes, `POST /backup-jobs/{BackupJobId}`
  stops. There is no `.../stop-backup-job` suffix on the wire.
- **StartReportJob's path parameter is `ReportPlanName`, not a generic ID**:
  `POST /audit/report-jobs/{ReportPlanName}`. The same path shape
  (`/audit/report-jobs/{name}`) is reused for `DescribeReportJob` (GET, name =
  ReportJobId) -- distinguish purely by HTTP method.
- **StartScanJob's real path is the singular `/scan/job`** (PUT), separate
  from the plural `/scan/jobs` (GET list) and `/scan/jobs/{ScanJobId}` (GET
  describe). Do not conflate the two -- they really are different URIs in the
  smithy model.
- **DescribeRegionSettings/UpdateRegionSettings bind to `/account-settings`**,
  not `/region-settings`, despite the operation names. Confirmed directly from
  `serializers.go`'s `SplitURI` call for both ops.
- **CalculatedLifecycle must be epoch-seconds, not Go's default RFC3339.**
  Any backend field of type `*time.Time` that isn't currently populated is a
  landmine -- check its JSON serialization path even if it "looks unused."
  (Historical: this bit `CalculatedLifecycle` in the 2026-07-12 audit.)
- **StartCopyJob's SourceBackupVaultName vs DestinationBackupVaultArn
  asymmetry is real, not a typo** -- confirmed directly against
  `StartCopyJobInput` in the SDK: the source is addressed by NAME, the
  destination by ARN. Getting this backwards (treating both as ARNs, or both
  as names) is an easy mistake; this backend had exactly that bug
  (SourceBackupVaultName was stored straight into an ARN-typed field with no
  resolution) before this pass.
- **A "job started successfully" response with a real-looking ID is not
  proof the operation actually did anything** (parity-principles.md #2's
  "disguised stub" warning, generalized): `StartCopyJob` returned a
  populated `CopyJob` with COMPLETED state, but no recovery point was ever
  created in the destination vault -- any client polling
  `ListRecoveryPointsByBackupVault` on the destination after a "successful"
  copy would see nothing. `PutRestoreValidationResult` returned 204 (success)
  every time, but the result was written to a map nothing ever read. Both
  looked correct in isolation (right status code, right response shape) and
  both required tracing the write through to confirm nothing downstream
  actually consumed it.
- **Unit tests are not parity proof (again)**: every routing bug in this
  service in the 2026-07-12 audit was invisible to `go test ./services/backup/...`
  because the test helpers call `h.Handler()(c)` directly and skip
  `RouteMatcher`/`matchesBackupPath` entirely. A green test suite here says
  nothing about whether a real `aws-sdk-go-v2` client could reach the
  operation at all. This pass's error-code/HTTP-status fixes were verified
  against the authoritative botocore service model (not just re-reading this
  package's own code), which is the only way the wrong-404/wrong-409/
  wrong-responseCode/fake-`ValidationException` findings were caught --
  they were all internally self-consistent (this service's own tests
  asserted the wrong values right back at it) until compared against
  ground truth.

- **GetPITRMalwareScanResults is the one Backup GET operation whose entire
  input lives in the query string, not the path.** Every other GET in this
  service addresses its target with a URI path segment
  (`/backup-vaults/{name}`, `/legal-holds/{id}`, ...); this one has a fixed
  literal path (`/scan/pitr-malware-scan-results`, no `{...}` segments at
  all) and binds `BackupVaultName`/`MalwareScanner`/`RecoveryPointArn`/
  `ScanEndTime` as query parameters instead (confirmed against
  `awsRestjson1_serializeOpHttpBindingsGetPITRMalwareScanResultsInput`).
  `handleGetPITRMalwareScanResults` reads `c.Request().URL.Query()`
  directly rather than using `route.resource`. There is no real malware
  scanner behind this handler (see gaps) -- `ScanResultStatus` is always
  `"UNKNOWN"`, one of the three real enum values, meaning exactly
  "no determination available," not a disguised clean/infected claim.

## gopherstack-muzq (2026-08-21): resources stuck in a transitional status forever

Continues gopherstack-oc9v/gopherstack-muzq's cross-service sweep for resources
stamped with a transitional status at construction that nothing in the backend
ever advances.

**Two confirmed instances, both fixed:**

- `CreateRestoreAccessBackupVault` stamped `VaultState` `CREATING` and nothing
  else in this backend ever wrote to that field -- confirmed via
  `ListRestoreAccessBackupVaults`, which echoes the stored value verbatim.
  Fixed by extending the existing `Janitor` (`janitor.go`) with
  `advanceRestoreAccessVaults`, run every `SweepOnce` alongside the pre-existing
  `advanceCreatedJobs` (`CREATED`->`COMPLETED`) -- same shape, no new
  infrastructure. New test `TestRestoreAccessVaultCreate_ReachesAvailable`.
- Found while investigating the above, a related but distinct instance:
  `ListBackupVaults` unconditionally reported `VaultState: CREATING` for
  *every* logically air-gapped vault (`MinRetentionDays > 0`) regardless of
  age -- there was no time check at all, so it could never report anything
  else. `DescribeBackupVault` on the very same vault unconditionally reported
  `AVAILABLE`, even the instant after creation -- the opposite bug. The two
  read paths for one resource could never agree with each other, let alone
  with real AWS's documented behavior ("the specified vault enters the
  creating state until it has been created successfully"). Fixed via a shared
  `vaultStateFor(v *Vault) string` helper (`vaults.go`) computing `CREATING`
  for `airGappedVaultCreatingWindow` (100ms) after `CreationTime` when
  `MinRetentionDays > 0`, else `AVAILABLE`, used by both handlers so they
  cannot diverge again. New test
  `TestLogicallyAirGappedBackupVault_ReachesAvailable` asserts both that
  `ListBackupVaults` reaches `AVAILABLE` and that `DescribeBackupVault` agrees
  with it on the same vault. `TestCreateLogicallyAirGappedBackupVault` (the
  pre-existing test) asserted `CREATING` immediately after create -- true, but
  it never checked `ListBackupVaults` at all, so it could not have caught
  either half of this bug.

Verified by hand-revert for both fixes: each fix's files were reverted to
their pre-fix `git show HEAD:<path>` content, the corresponding tests failed
with the predicted symptom (`Condition never satisfied`), then were restored
and confirmed `md5sum`-identical to the fixed versions.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 1 confirmed bug

`UpdateBackupPlan`: {wire: fixed} -- emitted "UpdateDate"; real member
(UpdateBackupPlanOutput, deserializers.go) is "CreationDate"
(UpdateBackupPlan creates a new plan version, so the timestamp represents
that version's creation). Proven via `TestUpdateBackupPlanUpdateDate`
(handler_backup_plans_test.go, strengthened in place), hand-reverted/
confirmed-failing/restored/`md5sum`-verified byte-identical.

## 2026-08-30 (gopherstack-uox6, value-semantics sweep): first audit of this class on backup, 2 bugs

This service had not previously been audited for "field is read and applied, but
with the wrong semantics" bugs (as opposed to wrong-key/never-attempted/missing-
pagination, all covered by the two 2026-08-29 entries above). Derived matcher/filter
count directly rather than trusting an estimate: ~14 real value-comparison sites
(ListBackupJobsFiltered/ListCopyJobsFiltered/ListRestoreJobsFiltered/
ListScanJobsFiltered/ListRecoveryPointsFiltered/ListBackupVaultsFiltered field
matchers, the shared inTimeRange time-range helper, and recoveryPointMatchesSelection
for legal holds), none of it HTTP-path routing (this service's routing lives in
handler_routes.go's regex table, disjoint from these filter helpers).

2 bugs found, both under-matching via an unhandled enum/wildcard value falling
through to "match everything" -- the same "switch/condition doesn't cover a real
value" shape flagged twice already in this campaign (ssm ListDocuments, this pass's
own count now three):

- `vaults.go` `ListBackupVaultsFiltered`: `types.VaultType` (backup@v1.59.4
  enums.go) has three enum members -- BACKUP_VAULT, LOGICALLY_AIR_GAPPED_BACKUP_VAULT,
  RESTORE_ACCESS_BACKUP_VAULT -- but the filter only had `if`s for the first two
  (via a MinRetentionDays>0 heuristic), so `ByVaultType=RESTORE_ACCESS_BACKUP_VAULT`
  fell through both conditions and returned every vault in `b.vaults` unfiltered,
  rather than the empty set a real client would get for that value (restore access
  vaults are correctly modeled in a wholly separate table, `b.restoreAccessVaults`,
  never in `b.vaults`). Fixed by comparing directly against the Vault struct's own
  already-populated `VaultType` field (`vaults.go` sets it to VaultTypeBackupVault/
  VaultTypeAirGapped at creation) instead of re-deriving type from
  MinRetentionDays -- this also future-proofs against any further enum growth. Test
  `TestListBackupVaultsFiltered` (vaults_test.go) gained 3 cases (BACKUP_VAULT,
  LOGICALLY_AIR_GAPPED_BACKUP_VAULT, RESTORE_ACCESS_BACKUP_VAULT); its pre-existing
  air-gapped-vault setup was itself wrong (used `PutBackupVaultLockConfiguration`,
  which only writes a separate VaultLockConfig record and never touches
  Vault.VaultType/MinRetentionDays at all) and was fixed to use the real
  `CreateLogicallyAirGappedBackupVault` constructor. All 3 new cases confirmed
  failing against unmodified code+corrected setup before the fix (RESTORE_ACCESS
  wanted 0, got 3; the other two incidentally passed under the old MinRetentionDays
  heuristic once the vault setup itself was corrected, confirming the fix doesn't
  regress the two cases the old code did handle).
- `ByAccountId` on ListBackupJobs and ListScanJobs: both ops' own doc comments
  (api_op_ListBackupJobs.go, api_op_ListScanJobs.go) state "If used from an
  [Amazon Web Services] Organizations management account, passing * returns all
  jobs across the organization" -- `*` is a documented wildcard, not a literal
  account ID. `jobMatchesFilter` (backup_jobs.go) and `scanJobMatchesFieldFilters`
  (restore_testing.go) both compared it as a literal equality, so `ByAccountId=*`
  excluded every job (no seeded job's AccountID is ever literally "*") instead of
  matching all of them -- the opposite of the documented behavior. Fixed both call
  sites against a new shared `wildcardAccountID = "*"` const (filters.go);
  extracted `scanJobAccountMatches` out of `scanJobMatchesFieldFilters` to keep it
  under the cyclop budget. New tests: `TestListBackupJobsFiltered` gained an
  "accountID wildcard matches all" case (backup_jobs_test.go); new
  `TestListScanJobsFiltered_AccountIDWildcard` (restore_testing_test.go, no prior
  test existed for ListScanJobsFiltered's AccountID facet at all). Both confirmed
  failing against unmodified code first.

Gap recorded, not fixed: `ListCopyJobsInput.ByAccountId`/`ListRestoreJobsInput.
ByAccountId`'s own doc comments say only "Returns only copy/restore jobs associated
with the specified account ID" -- no "*" wildcard note, unlike the two ops above.
`copyJobMatchesFilter`/`restoreJobMatchesFilter` (copy_jobs.go/restore_jobs.go)
still compare AccountID as a literal for these two ops; left unchanged rather than
assuming the same wildcard applies where AWS's own docs don't say so for that
specific operation (the sagemaker "read the documentation per caller, not once"
lesson from this same campaign).

Also checked and confirmed correct, not touched: the shared `inTimeRange` helper
(filters.go) implements BOTH bounds as strictly exclusive across all 5 of its
callers (backup jobs, copy jobs, restore jobs x2, scan jobs) -- consistent with
every one of those ByCreatedBefore/ByCreatedAfter/ByCompleteBefore/ByCompleteAfter
doc comments, which uniformly say only "before"/"after" with no "or equal to"
language (no cross-caller disagreement here, unlike the sagemaker shared-helper
case this campaign found elsewhere). By contrast `recoveryPointMatchesSelection`'s
legal-hold DateRange bound is genuinely inclusive on both ends, matching
`types.DateRange`'s explicit doc comment ("This value is the beginning/end date,
inclusive") -- two different documented fields with two different, each correctly
implemented, semantics, not one shared matcher misapplied to disagreeing callers.
ProtectedResourceConditions (StringEquals/StringNotEquals tag conditions on restore
testing selections) and BackupSelection's ListOfTags/Conditions are stored and
echoed back verbatim but never evaluated against any resource -- this backend has no
scheduled restore-test or plan-execution engine to run them against, a structural
gap already disclosed elsewhere in this file, not a wrong-algorithm bug.

Gates: `go build ./services/backup/...`, `go vet ./services/backup/...` and
repo-wide `go vet ./...` (clean), `go test -race -count=1 ./services/backup/...`
(pass), `golangci-lint run ./services/backup/...` (0 issues after decomposing
scanJobMatchesFieldFilters to stay under cyclop's limit).

## 2026-08-31 error-envelope-shape / fabricated-error-code sweep

**Scope**: error envelope shape (does an error deserialize into the typed
exception a real SDK client branches on) and fabricated error codes (a code the
emulator returns that the pinned SDK does not define for that specific
operation), per-operation -- not the filter-semantics class other recent passes
chased.

**Envelope mechanism confirmed correct**: `errResp(code, msg) ->
{"code": ..., "message": ...}` (handler_dispatch.go) is read correctly by every
operation's real `awsRestjson1_deserializeOpError<Op>` (`backup@v1.59.4/deserializers.go`)
via `restjson.GetErrorInfo`, which checks `Code` (case-insensitively matches this
service's lowercase `"code"` key) before falling back to `__type` -- this service
never sets `__type` or a header, but the `Code` fallback always resolves. This is
the same `restjson.GetErrorInfo` mechanism networkmanager and iot both use;
confirmed directly in the pinned SDK source (`aws-sdk-go-v2@v1.43.4/aws/protocol/restjson/decoder_util.go`),
not assumed.

**Per-operation ground truth extracted programmatically**: every one of this
service's 95 `deserializeOpError<Op>` functions' declared exception cases were
extracted directly from source (not sampled), then cross-referenced against every
`ErrNotFound`/`ErrAlreadyExists`/`ErrInvalidRequest` call site in the backend
(~60 sites across 14 files) by mapping each site to its enclosing
`InMemoryBackend` method and treating the method name as the operation name
(verified 1:1 for every site reached, including internal-helper exceptions like
`CompleteBackupJob`/`GetBackupVaultLockConfig`, which are not real API operations
and were confirmed to have their errors discarded/swallowed before reaching any
client, not just skipped from the cross-check).

**2 real bugs found and fixed**:

1. `DeleteRestoreTestingPlan`'s unknown-plan-name path returned
   `ResourceNotFoundException`, but this operation's own deserializer switch
   (`deserializers.go`) declares only `InvalidRequestException`/
   `ServiceUnavailableException` -- no not-found case at all, unlike almost every
   other Delete op in this service. A real client's deserializer never matches
   `ResourceNotFoundException` for this op and falls to
   `*smithy.GenericAPIError` (silent failure: the typed-exception branch never
   fires). Fixed: `restore_testing.go` now wraps `ErrInvalidRequest` instead of
   `ErrNotFound`. Proven fail-before/pass-after with a real `aws-sdk-go-v2`
   client (`Test_DeleteRestoreTestingPlan_UnknownPlanIsInvalidRequest`,
   `wire_error_code_restore_testing_plan_test.go`).

2. `CreateBackupSelection`'s unresolved-`BackupPlanId` path returned
   `ResourceNotFoundException`, but this operation's own deserializer declares
   `{AlreadyExistsException, InvalidParameterValueException,
   LimitExceededException, MissingParameterValueException,
   ServiceUnavailableException}` -- no `ResourceNotFoundException`. Fixed:
   `selections.go` now wraps `ErrValidation` (renders as
   `InvalidParameterValueException`, the real type for "a parameter value does
   not refer to a real resource" per this service's own established
   convention). Proven fail-before/pass-after
   (`Test_CreateBackupSelection_UnknownPlanIsInvalidParameterValue`,
   `wire_error_code_backup_selection_test.go`). An existing test
   (`TestCreateBackupSelection/plan_not_found`, `handler_selections_test.go`)
   asserts only `http.StatusBadRequest` -- unchanged either way (both codes are
   400 in this service) and therefore could never have caught this class; left
   as-is, not weakened.

**Everything else checked held**: the remaining ~58 `ErrNotFound`/`ErrAlreadyExists`
call sites all map to operations whose real deserializer switch does declare the
corresponding type. Two internal-only sentinels (`CompleteBackupJob`,
`GetBackupVaultLockConfig`) are not real API operations and their errors never
reach a client. `ErrInvalidRequest` usages (DeleteBackupVault,
DeleteBackupVaultChecked, PutBackupVaultLockConfiguration) all target operations
that do declare `InvalidRequestException`.

**Gap recorded, not fixed, with reasoning**: `ConflictException` is declared for
`DeleteFramework`/`DeleteReportPlan`/`CreateRestoreTestingPlan`/`UpdateFramework`/
`UpdateReportPlan`/`UpdateRestoreTestingPlan`/`UpdateRestoreTestingSelection`/
`CreateTieringConfiguration`/`UpdateTieringConfiguration`, but this backend has no
sentinel or state model for "resource is in a conflicting state" for
framework/report-plan deletion (e.g. "framework still referenced by a report
plan") -- `DeleteFramework` deletes unconditionally once existence is confirmed,
with no dependent-tracking to check. Not fixed: the backend cannot reach this
state (no legal input triggers it), which is a completeness/validation gap
distinct from a wire-shape bug -- this pass's mandate was envelope shape and
fabricated codes, not general completeness, so it is recorded rather than
fabricated a check for.

**Fabricated error codes**: `cmd/errcodeaudit` returned zero findings (confident
or needs-review) for `services/backup/`. No further fabrications found by the
per-operation cross-reference above.

Gates: `go build ./services/backup/...` (clean), `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/backup/...` (pass), `golangci-lint
run ./services/backup/...` (0 issues).

## gopherstack-21my per-item typed round-trip pass (2026-08-31)

This service was one of the eighteen marked "clean at wrapper level, never
swept per-item" in gopherstack-21my. Per that issue's own finding (a manual
per-item read of rds's DescribeDBInstances came back clean the session
before `e2a4d084a` found its `DBParameterGroups` list decoding empty for
every client), a source read is not trusted here -- this pass writes a real
`aws-sdk-go-v2` client round-trip instead of reading `deserializers.go` by
eye.

**Covered**: `CreateBackupPlan`/`GetBackupPlan`/`ListBackupPlans` --
specifically `Plan.Rules[].CopyActions[]`, the deepest nested list in this
service's wire shape (two rules, one with two `CopyAction`s each carrying
its own `Lifecycle`). New test
`TestSDKRoundTrip_BackupPlanRulesAndCopyActions`
(`sdk_roundtrip_nested_test.go`), 15 `require` calls, all against the real
SDK client's decoded response. **Result: clean.** Also manually verified
`Rule`/`CopyAction`/`Lifecycle`'s field names against
`awsRestjson1_deserializeDocumentBackupRule`/`...CopyAction`/`...Lifecycle`
(`backup@v1.59.4` deserializers.go) before writing the test -- all match
(`TargetBackupVaultName`, `DestinationBackupVaultArn`,
`MoveToColdStorageAfterDays`, `DeleteAfterDays`, etc.) -- no bug found here.

**Not covered this pass** (next pass should start here):
`ListBackupJobs`/`ListCopyJobs`/`ListRestoreJobs`/`ListRecoveryPointsByBackupVault`/
`ListProtectedResources`/`ListLegalHolds`/`ListFrameworks`/`ListReportPlans`/
`ListRestoreTestingPlans`/`ListRestoreTestingSelections`/`ListBackupSelections`/
`ListBackupVaults` -- none received a real-client round-trip test in this
pass. `RecoveryPoint.Lifecycle`/`CalculatedLifecycle` (single-level nested
objects, not lists) were spot-checked against
`awsRestjson1_deserializeDocumentCalculatedLifecycle` by source read only
(matches) -- per this issue's own thesis that a source-read clean is not
proof, this is recorded as unverified-by-test, not as a clean finding.

**Test-file exposure**: of 45 `*_test.go` files in this service, only 8 (9
counting the new one) drive a real typed `aws-sdk-go-v2` client
(`NewFromConfig`/`newTestBackupClient`) -- the remaining ~82% assert on raw
HTTP bodies or internal structs via `doREST`/`parseResp`, which cannot see a
wrong-element-name or dropped-nested-list bug of this class at all.

Gates: `go build ./services/backup/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/backup/...` (pass), `golangci-lint run
./services/backup/...` (0 issues, `golines -w -m 120` applied then
re-verified with plain `golangci-lint run`).

## 2026-08-31 -- gopherstack-6flj/21my: ops never named in this file

Computed the queue directly rather than trusting a prior count: every
`List*`/`Describe*` op in `backup@v1.59.4`'s `api_op_*.go` files whose
literal name never appears anywhere in this PARITY.md. Seven such ops:
`DescribeFramework`, `DescribeGlobalSettings`, `DescribeReportPlan`,
`ListBackupPlanTemplates`, `ListIndexedRecoveryPoints`,
`ListRecoveryPointsByResource`, `ListReportJobs`. Protocol confirmed from
this service's own deserializer: `awsRestjson1_deserializeOpError*`
throughout `deserializers.go` -- rest-json, case-sensitive, no XML-style
key folding.

All seven checked at both the wrapper-key and per-item-field layer against
their own `awsRestjson1_deserializeOpDocument*Output`/
`awsRestjson1_deserializeDocument*` functions.

**Clean (wrapper key and every emitted per-item field correct):**
`DescribeFramework` (wraps `FrameworkArn`/`FrameworkName`/
`FrameworkDescription`/`FrameworkStatus`/`DeploymentStatus`/`CreationTime`/
`FrameworkControls`, `IdempotencyToken` legitimately never populated -- no
value to echo since this backend generates its own token only at create
time and doesn't persist the caller's), `DescribeGlobalSettings` (wraps
`GlobalSettings`+`LastUpdateTime`, both present), `ListBackupPlanTemplates`
(wraps `BackupPlanTemplatesList`, item fields `BackupPlanTemplateId`/
`BackupPlanTemplateName` both correct).

**`DescribeReportPlan`**: wrapper key `ReportPlan` correct; emitted fields
(`ReportPlanArn`/`ReportPlanName`/`ReportPlanDescription`/`CreationTime`/
`ReportDeliveryChannel`/`ReportSetting`) all correctly named. Three real
`ReportPlan` members are never emitted at all: `DeploymentStatus` (no
tracked signal -- this backend has no deployment lifecycle to report),
`LastAttemptedExecutionTime`/`LastSuccessfulExecutionTime` (derivable in
principle from this service's own `ReportJob.ReportPlanArn`+`CompletionTime`
records, but computing that cross-reference is a distinct feature, not a
wire-shape fix -- disclosed as a gap, not fabricated).

**BUG FIXED: `ListRecoveryPointsByResource`** (`handler_recovery_points.go`,
`dispatchRecoveryPointQueryOps`). Wrapper key `RecoveryPoints` was already
correct, but the per-item shape emitted only `RecoveryPointArn`/`Status`
even though the real `RecoveryPointByResource` type's `BackupVaultName` and
`CreationDate` (both real, non-required members,
`deserializers.go:24314-24461`) were already tracked on this backend's
`RecoveryPoint` model and simply never surfaced. Also added
`BackupSizeBytes`/`EncryptionKeyArn` (also tracked, also real members).
`ResourceName`/`StatusMessage`/`IndexStatus`/`IndexStatusMessage`/
`IsParent`/`VaultType`/`AggregatedScanResult`/`EncryptionKeyType` remain
disclosed gaps -- not tracked on the backend model, not fabricated.

**BUG FIXED (per-item field, wrong-field-for-the-type class): `ListIndexedRecoveryPoints`**
(same file/function). The real `IndexedRecoveryPoint` type
(`deserializers.go:22769-22855`) has **no `Status` member at all** -- the
handler emitted `rp.Status` (a backup-job status like `COMPLETED`) under a
key the real type's deserializer never reads, a sibling-trap bug: this
service's `RecoveryPointByResource` sibling type genuinely does have a
`Status` field, and the shared item-shaping code was copied from there
without checking the target type. Real `IndexedRecoveryPoint.IndexStatus`
was already tracked by this backend (`GetRecoveryPointIndexDetails`/
`UpdateRecoveryPointIndexSettings`, `recoveryPointIndexStatus` map) but
never read here -- a real client's index status was always nil regardless
of what `UpdateRecoveryPointIndexSettings` had set. Fixed to emit
`RecoveryPointArn`/`BackupVaultArn`/`IamRoleArn`/`IndexStatus`/
`ResourceType`/`SourceResourceArn`/`BackupCreationDate`, all backed by
already-tracked fields. `IndexCreationDate`/`IndexStatusMessage` remain
disclosed gaps (not tracked).

**BUG FIXED (sibling-shares-the-gap): `DescribeReportJob`/`ListReportJobs`**
(`handler_report_plans.go`, `dispatchReportJobOps`). Both ops shared one
inline map literal emitting only `ReportJobId`/`Status`, even though
`ReportPlanArn`/`CreationTime`/`CompletionTime` are all real members
(`deserializers.go:24870-24943`) already set on `ReportJob` at
`StartReportJob` time. This op's own prior PARITY line ("same fabricated-200
bug as DescribeRestoreJob, fixed") only ever verified the 404-vs-fabricated-200
behavior, not this required-and-tracked-but-dropped field set -- the exact
stale-verdict trap this file's `DescribeScanJob` entry already documents.
Extracted a shared `reportJobToJSON` helper so both ops stay in sync.
`ReportTemplate`/`ReportDestination`/`StatusMessage` remain disclosed gaps
-- this backend never generates an actual report artifact, so there is no
honest value to source them from.

Tests: `wire_field_fixes_indexed_rp_test.go`, 3 new tests
(`TestListRecoveryPointsByResource_WireFields`,
`TestListIndexedRecoveryPoints_WireFields`, `TestReportJob_WireFields`), all
driving the real `aws-sdk-go-v2/service/backup` typed client and asserting
on the decoded response (not raw body). All three confirmed failing against
unmodified code first (`git stash` of just the fixed handler file, run,
`git stash pop`), then passing after the fix.

No transposition, no case-only mismatch (rest-json is case-sensitive here,
not applicable anyway), no hard decode error/panic, no wrong Go type under
a correct key, no field existing both nested and top-level, found this
pass. No web pages fetched this pass (`gopherstack-sdk-shape`-style lookups
went through the pinned SDK module cache only).

Gates: `go build ./services/backup/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/backup/...` (pass, 3 new tests),
`golangci-lint run ./services/backup/...` (0 issues, `golines -w -m 120`
applied to the new test file for one long line then re-verified). No
`models.go`/persisted-struct change this pass (response-shaping code only)
-- snapshot version guard not run, not needed.
