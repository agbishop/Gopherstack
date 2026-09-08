---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: kinesisanalyticsv2
sdk_module: aws-sdk-go-v2/service/kinesisanalyticsv2@v1.41.4
last_audit_commit: 47436caf9
last_audit_date: 2026-09-04
overall: A            # one real non-total-sort bug found and fixed this pass
                       # (ListApplicationSnapshots tie-break); every other prior
                       # finding re-verified, none regressed
ops:
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "inline ApplicationConfiguration/CloudWatchLoggingOptions were previously silently discarded (fixed pre-existing pass); ApplicationCodeConfiguration/FlinkApplicationConfiguration/EnvironmentProperties/ApplicationSnapshotConfiguration/ApplicationSystemRollbackConfiguration/ApplicationEncryptionConfiguration/ZeppelinApplicationConfiguration were accepted-but-not-modeled (this and a prior pass's gap) -- now seeded via SeedApplicationConfiguration's extended SeedConfig, still without bumping past version 1. ZeppelinApplicationConfiguration (Studio notebook: MonitoringConfiguration/CatalogConfiguration+GlueDataCatalogConfiguration/DeployAsApplicationConfiguration+S3ContentBaseLocation/CustomArtifactsConfiguration+S3orMaven) is now fully typed and echoed via ZeppelinApplicationConfigurationDescription -- sized first (4-level-deep tree, one ArtifactType-discriminated union, ~9 leaf fields across 3 wire variants, no recursion), all shallow and typeable, no part left opaque. Referenced ARNs (GlueDataCatalogConfiguration.DatabaseARN, S3ContentLocation/S3ContentBaseLocation.BucketARN) are stored as plain strings with no cross-service existence check, matching this service's pre-existing convention for every other ARN field (ServiceExecutionRole, S3CodeLocationDesc.BucketARN, KinesisStreamsInputDesc.ResourceARN, etc.). CORRECTED (gopherstack-osg7): the claim that this codebase has no cross-service backend-to-backend validation anywhere was false -- grafana's validateWorkspaceRoleArn/validateVpcConfiguration (services/grafana/cross_service.go) and ec2's validateOutpostArn are exactly that, using the SetAppConfig/siblingServices pattern documented on pkgs/service/service.go's AppContext. This service doesn't use that pattern for its ARN fields; adding it here would reuse an existing mechanism, not invent a new architecture. Whether to add it is a separate decision, not made this pass. This pass also dropped an invented top-level Tags field from applicationDetailOutput (real ApplicationDetail, types/types.go:179, has no such member -- tags are only retrievable via the separate ListTagsForResource op); harmless to a typed client (unknown JSON keys are ignored) but a genuine shape deviation."}
  DescribeApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "applicationDetailOutput previously omitted LastUpdateTimestamp/ConditionalToken/ApplicationVersionCreateTimestamp/ApplicationVersionRolledBackFrom/To/ApplicationVersionUpdatedFrom/ApplicationMaintenanceConfigurationDescription (all now populated); its VpcConfigurationDescriptions was WRONGLY placed at the top level of ApplicationDetail (real AWS has no such field -- it only exists nested inside ApplicationConfigurationDescription) -- this gopherstack-invented field placement is fixed (moved into appConfigDesc, matching real ApplicationConfigurationDescription.VpcConfigurationDescriptions)."}
  UpdateApplication: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "2026-08-29: request accepted a gopherstack-invented ApplicationDescription member and actually applied it to backend state -- real UpdateApplicationInput (api_op_UpdateApplication.go:33-78) has exactly 8 members (ApplicationName, ApplicationConfigurationUpdate, CloudWatchLoggingOptionUpdates, ConditionalToken, CurrentApplicationVersionId, RunConfigurationUpdate, RuntimeEnvironmentUpdate, ServiceExecutionRoleUpdate) and no way to change an application's description after CreateApplication. Found by cmd/acceptguard (a decoded-but-never-real request field being read and applied). DELETED from updateApplicationInput/UpdateApplicationParams/applyBasicFields; proven by TestKAV2_UpdateApplication_ApplicationDescription_NotARealField (wire_field_fixes_test.go), which failed against the unfixed code. ApplicationConfigurationUpdate (code/Flink/env-properties/snapshot/rollback/encryption/SQL-input-output-refdata/VPC sub-updates), CloudWatchLoggingOptionUpdates, RunConfigurationUpdate, RuntimeEnvironmentUpdate, and ConditionalToken were all accepted-but-ignored; all now implemented (applications.go/application_update_apply.go/handler_application_update.go). ConditionalToken is a deterministic sha256-derived function of (ApplicationARN, ApplicationVersionId) -- see conditionalToken/checkAndBumpVersionOrToken in store.go -- so it needs no extra persisted field and automatically rotates on every version bump. Sub-resource IDs referenced by CloudWatchLoggingOptionUpdates/SqlApplicationConfigurationUpdate/VpcConfigurationUpdates are validated to exist BEFORE the version is bumped (validateUpdateReferences), matching the Add*/Delete* config ops' existing 'find before bumping' convention -- a request naming an unknown ID leaves ApplicationVersionId untouched. ZeppelinApplicationConfigurationUpdate (this pass's gap) was also accepted-but-ignored; now implemented (applyZeppelinConfigUpdate), merging onto any existing ZeppelinConfig the same way applyFlinkConfigUpdate does. CustomArtifactsConfigurationUpdate reuses the create-time item shape wholesale (verified: real AWS's botocore model has no separate per-item update shape). THIS PASS'S BUG: InputUpdate.InputSchemaUpdate/InputParallelismUpdate and ReferenceDataSourceUpdate.ReferenceSchemaUpdate (same root cause as AddApplicationInput/AddApplicationReferenceDataSource's gap) were accepted-but-ignored -- a code comment even said so explicitly ('InputSchemaUpdate/InputParallelismUpdate are not modeled anywhere in this backend...and are ignored if present on the wire') but this was never surfaced as a PARITY.md gap despite InputSchema being a REQUIRED member one level up. Fixed: InputSchemaUpdateDesc (types/types.go:1336 'InputSchemaUpdate' -- its own Update-suffixed shape, field names RecordFormatUpdate/RecordEncodingUpdate/RecordColumnUpdates, NOT SourceSchema reused) and InputParallelismUpdateDesc now apply in applyInputUpdate, regenerating InAppStreamNames when NamePrefixUpdate or InputParallelismUpdate lands. ReferenceDataSourceUpdate.ReferenceSchemaUpdate is the asymmetric case: real AWS types it plain *SourceSchema (types/types.go:2106), NOT a dedicated Update shape like InputSchemaUpdate -- verified and modeled as such (ReferenceDataSourceUpdate.ReferenceSchemaUpdate *SourceSchemaDesc, reusing the same type as the create/describe sides). Proven via TestUpdateApplication_InputSchemaUpdate_SDKRoundTrip and TestUpdateApplication_ReferenceSchemaUpdate_SDKRoundTrip."}
  DeleteApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreateTimestamp request field is now validated against the application's actual CreateTimestamp (epoch-seconds float64 comparison with 1e-3/1ms tolerance, matching smithy-go's millisecond-precision unixTimestamp wire truncation); a mismatch returns InvalidArgumentException instead of silently deleting. DeleteApplication remains synchronous (see gaps, unchanged from prior audit)."}
  ListApplications: {wire: ok, errors: ok, state: ok, persist: ok}
  StartApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "RunConfiguration request field (ApplicationRestoreConfiguration/FlinkRunConfiguration) was never parsed at all -- now applied and echoed back via DescribeApplication's ApplicationConfigurationDescription.RunConfigurationDescription. SqlRunConfigurations was accepted-but-ignored, and its InputId was never validated: this pass found it DOES have somewhere to land -- real AWS's InputDescription (not RunConfigurationDescription, which has no such field) carries a per-input InputStartingPositionConfiguration -- so it is now validated (unknown InputId -> ResourceNotFoundException, checked BEFORE ApplicationStatus is mutated to RUNNING) and stored/echoed on the matching InputDescription."}
  StopApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "Force request field was not even parsed (worse than accepted-but-ignored) -- now parsed and enforces real AWS's documented Flink-only restriction ('You can only force stop a Managed Service for Apache Flink application' -- api_op_StopApplication.go doc comment, aws-sdk-go-v2/service/kinesisanalyticsv2@v1.41.4): Force=true on a SQL-1_0 application now returns InvalidArgumentException. Force's state-broadening effect (permitting stop from STARTING/UPDATING/STOPPING/AUTOSCALING) has no observable effect here since this backend's ApplicationStatus is only ever READY/RUNNING (synchronous lifecycle, same structural gap as DeleteApplication's unused ApplicationStatusDeleting -- confirmed no other status is ever assigned). The pre-stop auto-snapshot itself remains unimplemented: confirmed via AWS's own 'Deep dive into the Amazon Managed Service for Apache Flink application lifecycle' blog post that the auto-snapshot's naming/visibility is still not documented publicly, so fabricating one continues to be avoided as a gopherstack-invented-behavior risk -- see gaps."}
  RollbackApplication: {wire: ok, errors: ok, state: ok, persist: n/a, note: "now also sets ApplicationVersionRolledBackFrom/To (the version rolled back from/to) and ApplicationVersionUpdatedFrom on the resulting live Application, echoed via ApplicationDetail; these three lineage fields are cleared by every subsequent non-rollback version-bumping op (see bumpVersion in store.go) so they never linger as stale rollback markers."}
  DescribeApplicationOperation: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListApplicationOperations: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeApplicationVersion: {wire: ok, errors: ok, state: ok, persist: n/a, note: "shares toDetailOutput with DescribeApplication/CreateApplication/UpdateApplication/RollbackApplication, so it picked up every wire-shape fix in this pass automatically."}
  ListApplicationVersions: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreateApplicationSnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "unchanged this pass; not re-diffed (files untouched since 782e2a93)."}
  DescribeApplicationSnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "unchanged this pass; not re-diffed."}
  ListApplicationSnapshots: {wire: ok, errors: ok, state: fixed, persist: ok, note: "2026-09-04: sort.Slice ordered results by SnapshotCreation alone, with no tiebreak -- sort.Slice is not guaranteed stable, so two snapshots sharing a creation timestamp could come back in either relative order depending on the pre-sort order of the byApp index group, which an unrelated Delete on a THIRD snapshot in the same group silently changes (Index.remove's swap-with-last-element removal, pkgs/store/index.go) -- the same tie-prone-sort class fixed across bedrock/cloudwatchlogs/lightsail/quicksight/ssm/etc. in c78177958, which did not touch this service. Fixed: falls through to SnapshotName (unique per application, enforced by CreateApplicationSnapshot's pre-create existence check) when SnapshotCreation ties. Proven via TestBackend_ListApplicationSnapshots_TieBreak (whitebox_test.go), confirmed to fail against the pre-fix code."}
  DeleteApplicationSnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "unchanged this pass; not re-diffed."}
  AddApplicationCloudWatchLoggingOption: {wire: ok, errors: ok, state: ok, persist: ok, note: "real AWS's AddApplicationCloudWatchLoggingOptionOutput carries an OperationId field (unlike most other Add*/Delete* config ops -- verified field-by-field against aws-sdk-go-v2's api_op_AddApplicationCloudWatchLoggingOption.go); gopherstack's response never had one. Fixed: now records an ApplicationOperation and returns OperationId."}
  AddApplicationInput: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified AddApplicationInputOutput has no OperationId field in the real SDK -- correctly has none here. THIS PASS'S BUG: real AWS's Input shape (types/types.go:1125) has InputSchema as a REQUIRED member and InputParallelism as optional -- gopherstack's inputConfig/InputDescription modeled neither, so a real client's InputSchema (the column/format mapping the operation exists to configure) was silently dropped and never echoed back by DescribeApplication, and InAppStreamNames (documented on Input.NamePrefix: '...creates one or more...in-application streams with the names MyInApplicationStream_001, MyInApplicationStream_002...') was never populated at all. Fixed: added SourceSchemaDesc/RecordFormatDesc/MappingParametersDesc/InputParallelismDesc to models.go, wired into inputConfig (request) and InputDescription (response), and added inAppStreamNames() to synthesize the documented '<NamePrefix>_NNN' names. Proven via TestAddApplicationInput_InputSchema_SDKRoundTrip (wire_sdk_roundtrip_test.go) and hand-revert (removing the two assignment lines reproduces 'InputSchema silently dropped by the real client's deserializer', confirmed then restored byte-identical)."}
  AddApplicationInputProcessingConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  AddApplicationOutput: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified AddApplicationOutputOutput has no OperationId field in the real SDK -- correctly has none here. Output/OutputDescription/OutputUpdate/DestinationSchema re-verified field-by-field against types/types.go:1782-1810,1839 this pass -- all fields present, DestinationSchema correctly flat (RecordFormatType only, no MappingParameters -- real AWS's DestinationSchema has none)."}
  AddApplicationReferenceDataSource: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified AddApplicationReferenceDataSourceOutput has no OperationId field in the real SDK -- correctly has none here. THIS PASS'S BUG: real AWS's ReferenceDataSource shape (types/types.go:2048) has ReferenceSchema as a REQUIRED member -- gopherstack's refDataSourceConfig/ReferenceDataSourceDescription never modeled it, silently dropping it. Fixed: added ReferenceSchema *SourceSchemaDesc to both. Proven via TestAddApplicationReferenceDataSource_ReferenceSchema_SDKRoundTrip and hand-revert (same symptom class as AddApplicationInput's InputSchema)."}
  AddApplicationVpcConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "same OperationId gap/fix as AddApplicationCloudWatchLoggingOption -- verified against api_op_AddApplicationVpcConfiguration.go."}
  DeleteApplicationCloudWatchLoggingOption: {wire: ok, errors: ok, state: ok, persist: ok, note: "same OperationId gap/fix as AddApplicationCloudWatchLoggingOption -- verified against api_op_DeleteApplicationCloudWatchLoggingOption.go."}
  DeleteApplicationInputProcessingConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified no OperationId field in the real SDK -- correctly has none here."}
  DeleteApplicationOutput: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified no OperationId field in the real SDK -- correctly has none here."}
  DeleteApplicationReferenceDataSource: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified no OperationId field in the real SDK -- correctly has none here."}
  DeleteApplicationVpcConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "same OperationId gap/fix as AddApplicationCloudWatchLoggingOption -- verified against api_op_DeleteApplicationVpcConfiguration.go."}
  CreateApplicationPresignedUrl: {wire: ok, errors: ok, state: ok, persist: n/a, note: "unchanged this pass."}
  UpdateApplicationMaintenanceConfiguration: {wire: partial, errors: ok, state: ok, persist: ok, note: "ApplicationMaintenanceWindowEndTime never computed/returned (unchanged gap, low value -- see gaps)."}
  DiscoverInputSchema: {wire: deferred, errors: ok, state: n/a, persist: n/a, note: "the synthetic JSON/UTF-8 placeholder schema itself is unchanged (real AWS samples live stream data, which this emulator cannot do -- confirmed this was NEVER made to error, contrary to this pass's starting premise: the synthetic response has existed since the op's introduction, commit 0d4fdada4). Fixed real wire bugs found while re-checking it: the request's ServiceExecutionRole (required by botocore's DiscoverInputSchemaRequest) was wired to the wrong key 'RoleARN' and never validated -- a real client's ServiceExecutionRole was silently dropped and an empty/absent one never rejected; InputStartingPositionConfiguration was a flat string instead of the real nested object; the response's InputSchema.SourceSchema was missing RecordColumns entirely (a required member -- the previous response couldn't even satisfy its own required fields, and RecordColumns is the field a real client actually needs to configure its application's input schema, the operation's whole purpose)."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "unchanged this pass."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "unchanged this pass."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "unchanged this pass."}
families:
  error_mapping: {status: ok, note: "unchanged this pass; ConcurrentModificationException mapping (fixed prior pass) also now covers ConditionalToken mismatches (checkAndBumpVersionOrToken returns the same ErrConcurrentModification sentinel as version mismatches)."}
gaps:
  - FlinkApplicationConfigurationDescription.JobPlanDescription (DescribeApplicationRequest.IncludeAdditionalDetails) remains accepted-but-ignored: it is real AWS's Apache Flink job graph/scheduling plan (see the Apache Flink "Jobs and Scheduling" docs JobPlanDescription's own doc comment links to), which requires an actual Flink job compiler to produce -- structural, same class as DiscoverInputSchema's synthetic-schema limitation. Confirmed still genuinely unmodelable this pass; IncludeAdditionalDetails isn't even parsed by describeApplicationInput. Leniency only.
  - StopApplication's Force field now enforces the Flink-only restriction and is stored, but the pre-stop auto-snapshot itself is still not modeled: real AWS's auto-snapshot naming/visibility convention isn't documented publicly enough to fabricate (re-confirmed this pass via AWS's own "Deep dive into the Amazon Managed Service for Apache Flink application lifecycle" blog, which describes that a snapshot is taken but not how it's named or surfaced) -- deliberately left unimplemented rather than invented.
  - UpdateApplicationMaintenanceConfiguration's ApplicationMaintenanceWindowEndTime is never computed/returned (pre-existing gap, unchanged, low value -- no client observably depends on the exact window end time).
  - ZeppelinApplicationConfiguration's referenced ARNs (GlueDataCatalogConfiguration.DatabaseARN, S3ContentLocation/S3ContentBaseLocation.BucketARN) are not validated to exist in a Glue/S3 backend -- matches every other ARN field in this service (ServiceExecutionRole, KinesisStreamsInputDesc.ResourceARN, etc.), none of which are cross-service-validated. CORRECTED (gopherstack-osg7): this codebase does have a cross-service backend-to-backend validation mechanism (SetAppConfig/siblingServices, used by grafana/ec2/others to reject a request referencing a resource that doesn't exist elsewhere) -- this service simply doesn't use it for these ARN fields. Not a Zeppelin-specific gap, and not a "no mechanism exists" gap either; a follow-up could adopt the existing pattern here if desired.
  - DeleteApplication is synchronous (app removed immediately); real AWS transitions through a DELETING status first. ApplicationStatusDeleting const is defined but unused. Matches the synchronous-delete convention used elsewhere in this codebase; not fixed (pre-existing, unchanged).
  - Real AWS's default-assigned maintenance window (every application gets one automatically at creation, before any UpdateApplicationMaintenanceConfiguration call) is not modeled -- ApplicationMaintenanceConfigurationDescription is only populated in DescribeApplication once UpdateApplicationMaintenanceConfiguration has been called at least once. Pre-existing, unchanged; low value.
deferred:
  - DiscoverInputSchema (inherently synthetic without live stream sampling)
leaks: {status: clean, note: "New Application fields (CodeConfig/FlinkConfig/EnvironmentPropertyGroups/SnapshotsEnabled/RollbackEnabled/EncryptionConfig/RunConfig/version-lineage pointers) all live inside the Application struct itself, not a separate map -- DeleteApplication's existing applications.Delete(...) cleans them up with no new leak surface. The four Add*/Delete* config ops that now call recordOperation (AddApplicationCloudWatchLoggingOption/AddApplicationVpcConfiguration/DeleteApplicationCloudWatchLoggingOption/DeleteApplicationVpcConfiguration) write into the same b.operations[region][name] map DeleteApplication already clears -- verified via TestBackend_AddDeleteVpcAndCWLOption_ReturnOperationID plus the existing DeleteApplication cleanup tests, no new cleanup path needed. go test -race clean at -count=3."}
---

## Notes

- 2026-08-22, gopherstack-r80d batch 31 (required-output-member audit):
  kinesisanalyticsv2 (6 required output fields / 33 ops, 6 ops-with-required
  per a fresh `cmd/requiredoutputfields` run, cross-checked against an
  independent standalone `go/ast` walk of `kinesisanalyticsv2@v1.41.4`'s
  `api_op_*.go` files -- both agreed exactly at 6). Module resolved directly:
  directory `kinesisanalyticsv2` == SDK module
  `aws-sdk-go-v2/service/kinesisanalyticsv2@v1.41.4` per `go.mod`, with no
  `dirModuleOverride` entry and no import of the sibling v1 module -- verified
  by grepping this service's own source for its SDK import, distinct from
  `services/kinesisanalytics`, which imports `aws-sdk-go-v2/service/kinesisanalytics`
  and was not touched by this batch.

  All 6 flagged ops -- `CreateApplication`, `DescribeApplication`,
  `DescribeApplicationSnapshot`, `ListApplications`, `RollbackApplication`,
  `UpdateApplication` -- are the "one wrapper key" shape this campaign has
  named repeatedly (pinpoint/bedrockagent precedent): 5 of 6 wrap
  `*types.ApplicationDetail` (types.go:179-252, 5 required members --
  ApplicationARN/ApplicationName/ApplicationStatus/ApplicationVersionId/
  RuntimeEnvironment), `ListApplications` wraps `[]types.ApplicationSummary`
  (types.go:439-471, the same 5 required members), `DescribeApplicationSnapshot`
  wraps `*types.SnapshotDetails` (types.go:2349-2376, 3 required --
  ApplicationVersionId/SnapshotName/SnapshotStatus). Unlike the tagged
  structs this campaign usually finds bugs in, gopherstack's own wire types
  (`applicationDetailOutput`, handler_applications.go:154-174;
  `applicationSummary`, models.go:413-420; `snapshotDetail`, models.go:435-441)
  tag every one of these members with no `omitempty`, so shape 1 of this
  campaign's bug class cannot occur syntactically here. Checked shape 2
  instead (never populated on some write path): `toDetailOutput`
  (handler_applications.go:715-747), `toSummary` (models.go:423-431), and
  `toSnapshotDetail` (models.go:444-451) all read straight from the backend's
  `Application`/`Snapshot` structs, whose ApplicationARN/ApplicationName/
  ApplicationStatus/RuntimeEnvironment/ApplicationVersionID are all set
  unconditionally in `CreateApplication` (applications.go:14-51) and never
  cleared afterward (`ApplicationStatus` only ever transitions
  Ready<->Running, applications.go:341,405, both real non-empty values);
  `RollbackApplication`/`UpdateApplication` share the same `toDetailOutput`
  call, so they inherit the same guarantee. Result: 0 bugs. No code changes.

Protocol: awsjson1.1 (X-Amz-Target: `KinesisAnalytics_20180523.<Op>`, single POST
endpoint). RouteMatcher/ExtractOperation unchanged this pass.

### Real bugs found and fixed this pass

1. **`UpdateApplication` silently dropped `ApplicationConfigurationUpdate`,
   `CloudWatchLoggingOptionUpdates`, `RunConfigurationUpdate`,
   `RuntimeEnvironmentUpdate`, and `ConditionalToken`** -- the single largest
   gap flagged by the prior audit. Every field is now threaded through:
   `applications.go`'s `UpdateApplication` takes a new `UpdateApplicationParams`
   struct; `application_update_apply.go` applies each sub-field to the live
   `*Application`; `handler_application_update.go` converts the awsjson1.1
   wire shapes. `ConditionalToken` implements the alternative optimistic-
   concurrency check real AWS documents ("use ConditionalToken instead of
   CurrentApplicationVersionId") via a deterministic
   `sha256(ApplicationARN + "#" + ApplicationVersionId)`-derived token (see
   `conditionalToken`/`checkAndBumpVersionOrToken` in `store.go`) that
   automatically rotates on every version bump without a separate persisted
   field.

2. **`applicationDetailOutput.VpcConfigurationDescriptions` was placed at the
   wrong nesting level -- a gopherstack-invented field that doesn't exist in
   real AWS's `ApplicationDetail`.** Real AWS's `ApplicationDetail` struct
   (verified field-by-field against aws-sdk-go-v2's `types.go`) has no
   top-level `VpcConfigurationDescriptions` at all; it only exists nested
   inside `ApplicationConfigurationDescription`. A real SDK client's
   deserializer would simply never populate this field from gopherstack's
   previous (wrong) top-level placement, silently losing VPC config on every
   `DescribeApplication`/`CreateApplication`/`UpdateApplication` response.
   Fixed by moving it into `appConfigDesc` (`handler_applications.go`),
   matching the real nesting exactly.

3. **`CreateApplication`/`UpdateApplication` never modeled
   `ApplicationCodeConfiguration`, `FlinkApplicationConfiguration`,
   `EnvironmentProperties`, `ApplicationSnapshotConfiguration`,
   `ApplicationSystemRollbackConfiguration`, or
   `ApplicationEncryptionConfiguration`** -- accepted on the wire but produced
   no backend state and were never echoed back, so any client (Terraform,
   CloudFormation) reading its own `CreateApplication`/`DescribeApplication`
   response back would see silent drift on every one of these fields. All six
   are now modeled (`models.go`'s `ApplicationCodeConfigDesc`/
   `FlinkApplicationConfigDesc`/etc.), seeded at create time
   (`SeedConfig`/`seedExtendedConfig` in `applications.go`) and updatable via
   `UpdateApplication`'s `ApplicationConfigurationUpdate`. `CheckpointConfiguration`'s
   documented `DEFAULT` behavior ("the application will use
   CheckpointingEnabled: true / CheckpointInterval: 60000 /
   MinPauseBetweenCheckpoints: 5000, even if set to other values") is
   enforced by `applyCheckpointDefaults` -- verbatim from the real API's
   `CheckpointConfiguration.ConfigurationType` documentation.
   `MonitoringConfiguration`/`ParallelismConfiguration` also accept a
   `DEFAULT` `ConfigurationType`, but AWS's public documentation does not
   specify literal forced values for those two the way it does for
   checkpointing, so gopherstack deliberately leaves them as provided rather
   than fabricating undocumented defaults.

4. **`StartApplication`'s `RunConfiguration` request field was never parsed
   at all.** Real clients commonly start a Flink application with
   `ApplicationRestoreConfiguration` set to restore from a snapshot; this was
   silently accepted and discarded. Fixed: `StartApplication` now takes a
   `*RunConfigInput` parameter, stored as `Application.RunConfig` and echoed
   via `ApplicationConfigurationDescription.RunConfigurationDescription`
   (shared with `UpdateApplication`'s `RunConfigurationUpdate`, since real
   AWS uses the identical `ApplicationRestoreConfiguration`/
   `FlinkRunConfiguration` shape for both).

5. **Four `Add*`/`Delete*` config ops were missing `OperationId` in their
   response** -- `AddApplicationCloudWatchLoggingOption`,
   `AddApplicationVpcConfiguration`,
   `DeleteApplicationCloudWatchLoggingOption`,
   `DeleteApplicationVpcConfiguration`. Verified field-by-field against
   aws-sdk-go-v2's `api_op_*.go`: these four (and only these four, among the
   `Add*`/`Delete*` config family) carry an `OperationId` field in real AWS's
   output shape -- an asymmetry in the real API, not a gopherstack oversight
   to "fix" toward consistency. All four backend methods now call
   `recordOperation` and return the ID.

6. **`DeleteApplication`'s `CreateTimestamp` safety check was parsed but
   never validated** -- real AWS uses it as a check that the caller has a
   fresh `DescribeApplication` view before deleting. Fixed: compares the
   request's epoch-seconds value against `awstime.Epoch(app.CreatedAt)`
   and returns `InvalidArgumentException` on mismatch instead of deleting.
   Tolerance is 1e-3 (1ms), not 1e-6: smithy-go's wire encoding for
   `unixTimestamp` (`time.FormatEpochSeconds`/`ParseEpochSeconds`) truncates
   to millisecond precision, while `app.CreatedAt` -- and the `CreateTimestamp`
   a real client reads back from a prior `CreateApplication`/`DescribeApplication`
   response -- carries full nanosecond precision. A real SDK client can only
   ever round-trip `CreateTimestamp` truncated to the millisecond, so a 1e-6
   tolerance rejected every legitimate delete (caught by
   `TestIntegration_KinesisAnalyticsV2_*`); regression test added at
   `TestBackend_DeleteApplication_CreateTimestamp/millisecond-truncated_timestamp_deletes`.

7. **`persistedApplication.MaintenanceWindowStartTime` was declared but never
   assigned or restored** -- a pre-existing field that predates this pass;
   `UpdateApplicationMaintenanceConfiguration` state silently didn't survive
   `Snapshot`/`Restore`. Fixed alongside the `kinesisanalyticsv2SnapshotVersion`
   bump to 2 (which also added persistence for every new `Application` field
   from items 1/3/4 above).

### Follow-up pass (gopherstack-uci4, 2026-08-11)

Re-examined the four gaps this pass's predecessor deferred. Two premises did
not hold up:

- `StopApplication`'s `Force` field wasn't merely accepted-and-ignored, it
  wasn't parsed at all -- `startStopApplicationInput` had no `Force` field.
  Now parsed and enforces real AWS's Flink-only force-stop restriction
  (SQL-1_0 + Force=true -> `InvalidArgumentException`); the auto-snapshot
  itself is still deliberately unfabricated (naming/visibility genuinely
  undocumented, re-verified via AWS's own lifecycle blog post).
- `DiscoverInputSchema` was never made to error -- it has returned the same
  synthetic placeholder since its introduction (`0d4fdada4`). What it *did*
  have were real wire bugs: `ServiceExecutionRole` (required) was wired to
  a wrong key (`RoleARN`, never validated), `InputStartingPositionConfiguration`
  was a flat string instead of a nested object, and the response's
  `RecordColumns` (required) was omitted entirely. Fixed the wire; left the
  synthetic placeholder itself alone.

`StartApplication`'s `SqlRunConfigurations` turned out to have somewhere
real to land: `InputDescription.InputStartingPositionConfiguration` (not
`RunConfigurationDescription`, which real AWS has no such field on). Now
validated (unknown `InputId` -> `ResourceNotFoundException`, checked before
`ApplicationStatus` flips to `RUNNING`) and echoed. `JobPlanDescription`
was confirmed genuinely structural (a real Flink job graph) and left alone.

`ZeppelinApplicationConfiguration` was sized before typing: 4 levels deep
(config -> sub-config -> nested struct -> scalar leaf), one
`ArtifactType`-discriminated union (`CustomArtifactConfiguration`'s
S3-vs-Maven choice), ~9 leaf fields total across the create/describe/update
variants, no recursion. Fully typeable -- nothing left opaque. Referenced
ARNs (`DatabaseARN`, `BucketARN`) are plain strings with no cross-service
existence check, matching every other ARN field in this service. CORRECTED
(gopherstack-osg7): this codebase does have a cross-service backend
validation mechanism (`SetAppConfig`/`siblingServices`, e.g.
`services/grafana/cross_service.go`'s `validateWorkspaceRoleArn`); this
service just doesn't use it here. Adding it for Zeppelin would reuse an
existing pattern, not invent new architecture -- whether to do so is a
separate decision.

`Application.ZeppelinConfig` and `InputDescription.InputStartingPositionConfiguration`
are additive (`omitempty`); `kinesisanalyticsv2SnapshotVersion` was **not**
bumped -- both are wired into `persistedApplication`/`toPersistedApp`/
`fromPersistedApp` and round-trip-tested
(`TestPersistence_ZeppelinConfigSurvivesRoundTrip`).

### Traps for the next auditor

- `ConditionalToken` is **computed, not stored** -- `conditionalToken(app)` in
  `store.go` derives it from `(ApplicationARN, ApplicationVersionId)`. Don't
  add a stored field for it; that would require keeping it in sync on every
  version bump for no benefit (the derivation already changes automatically).
- `validateUpdateReferences` (`application_update_apply.go`) MUST run and
  return before `checkAndBumpVersionOrToken` in `UpdateApplication` -- it
  checks every `CloudWatchLoggingOptionUpdates`/`SqlApplicationConfigurationUpdate`/
  `VpcConfigurationUpdates` referenced ID actually exists. If a future change
  moves a mutation before this check, a request naming an unknown ID will
  bump `ApplicationVersionId` and leave a phantom version-history entry
  before failing -- the same bug class the pre-existing Add*/Delete* config
  ops' "find before bump" comments already warn about.
- `bumpVersion` (`store.go`) is now the single place that sets
  `LastUpdateTimestamp`/`ApplicationVersionCreateTimestamp`/
  `ApplicationVersionUpdatedFrom` and clears
  `ApplicationVersionRolledBackFrom/To` on every version-bumping op except
  `RollbackApplication` (which sets the Rolled-Back fields itself since it
  doesn't go through `bumpVersion`). Don't reintroduce a second place that
  increments `ApplicationVersionID` directly (e.g. `app.ApplicationVersionID++`)
  without also calling/mirroring `bumpVersion` -- that would silently freeze
  these lineage fields.
- `CheckpointConfigDesc`'s `DEFAULT`-forcing (`applyCheckpointDefaults`) is
  intentionally NOT mirrored for `MonitoringConfigDesc`/`ParallelismConfigDesc`
  -- this is a verified asymmetry in real AWS's own public API documentation,
  not an inconsistency to "fix".
- `ApplicationOperation.OperationID`/`StartTimestamp`/`EndTimestamp` are wired
  from eight call sites now (Start/Stop/Update/RollbackApplication plus the
  four `OperationId`-bearing config ops from item 5 above) -- don't re-flag
  `b.operations` as unused.
- `operations` and `versions` remain intentionally NOT persisted
  (`persistence.go` only snapshots `applications`/`snapshots` tables) --
  predates this audit, matches pre-Phase-3.3 behavior; don't treat it as a
  newly-introduced gap.
- `kinesisanalyticsv2SnapshotVersion` is now 2 (was 1) -- a v1 on-disk
  snapshot is discarded (not partially decoded) on `Restore`, per the
  existing version-mismatch-discard convention. If you add more
  `persistedApplication` fields, bump to 3 and document why in the constant's
  doc comment, matching the existing pattern.

### Follow-up pass (2026-08-20)

Wrapper-key/nested-shape sweep of every documented `*Configuration`/
`*ConfigurationDescription`/`*ConfigurationUpdate` triple in this service.
Found and fixed one real, previously-undocumented bug family: real AWS's
`Input` shape (`types/types.go:1125`) has `InputSchema` as a **required**
member (the column/format mapping the whole operation exists to configure)
and `InputParallelism` as optional, and `ReferenceDataSource`
(`types/types.go:2048`) has `ReferenceSchema` as required -- none of the
three were modeled anywhere in this backend. `application_config_update.go`
even had a code comment acknowledging `InputSchemaUpdate`/
`InputParallelismUpdate` were "not modeled...and are ignored if present on
the wire", but this was never surfaced in PARITY.md's `gaps` list despite
being a REQUIRED member, so the service's prior "A grade" didn't account for
it. `InAppStreamNames` (`InputDescription`, documented on `Input.NamePrefix`'s
own doc comment: "...creates one or more...in-application streams with the
names MyInApplicationStream_001, MyInApplicationStream_002...") was likewise
never populated.

Fixed across all three directions:
- `Input`/`InputDescription`: added `InputSchema`/`InputParallelism` to
  `inputConfig` (request) and `InputDescription` (response), plus
  `inAppStreamNames()` to synthesize the documented `<NamePrefix>_NNN` names.
- `InputUpdate`: added `InputSchemaUpdate`/`InputParallelismUpdate`, applied
  in `applyInputUpdate` (`application_update_apply.go`), regenerating
  `InAppStreamNames` when either lands.
- `ReferenceDataSource`/`ReferenceDataSourceDescription`/
  `ReferenceDataSourceUpdate`: added `ReferenceSchema`/`ReferenceSchemaUpdate`.
  Confirmed a genuine wire asymmetry while modeling this: real AWS's
  `InputUpdate.InputSchemaUpdate` is its own dedicated shape
  (`InputSchemaUpdate`, `RecordFormatUpdate`/`RecordEncodingUpdate`/
  `RecordColumnUpdates` -- Update-suffixed field names), but
  `ReferenceDataSourceUpdate.ReferenceSchemaUpdate` is typed plain
  `*SourceSchema` (`types/types.go:2106`), reusing the create/describe shape
  verbatim with NO renaming. Modeled distinctly (`InputSchemaUpdateDesc` vs.
  reusing `SourceSchemaDesc` for the reference-data side) rather than
  assuming symmetry.

All five new types (`SourceSchemaDesc`, `RecordFormatDesc`,
`MappingParametersDesc`, `CSVMappingParametersDesc`/`JSONMappingParametersDesc`,
`InputParallelismDesc`, `InputSchemaUpdateDesc`, `InputParallelismUpdateDesc`)
are additive (`omitempty`) fields on `InputDescription`/
`ReferenceDataSourceDescription`, which `persistedApplication` embeds
directly -- no new persistence wiring needed, `kinesisanalyticsv2SnapshotVersion`
correctly left at 2 (same additive convention as the prior Zeppelin/
`InputStartingPositionConfiguration` pass). Proven with four real-SDK
round-trip tests (`wire_sdk_roundtrip_test.go`, new file, driven through
`pkgs/service`'s router exactly like `services/emrserverless/wire_sdk_roundtrip_test.go`)
and one hand-revert (removing `buildInputDescription`'s two new assignment
lines reproduces "InputSchema silently dropped by the real client's
deserializer" exactly, then restored byte-identical).

Also dropped one harmless-but-invented member found incidentally while
diffing `ApplicationDetail` field-by-field: `applicationDetailOutput` carried
a top-level `Tags` field that real `ApplicationDetail` (`types/types.go:179`)
does not have (tags are retrieved only via the separate
`ListTagsForResource` op). A typed client ignores unknown JSON keys so this
was never observably wrong, but it was a real shape deviation; removed for
fidelity.

Every other documented triple (`ApplicationConfiguration`/`SqlApplicationConfiguration`/
`FlinkApplicationConfiguration`/`ApplicationCodeConfiguration`/`CodeContent`/
`S3ContentLocation`/`CheckpointConfiguration`/`MonitoringConfiguration`/
`ParallelismConfiguration`/`EnvironmentProperties`/`ApplicationSnapshotConfiguration`/
`ApplicationSystemRollbackConfiguration`/`ApplicationEncryptionConfiguration`/
`VpcConfiguration`/`ZeppelinApplicationConfiguration`+its four sub-configs/
`Output`/`DestinationSchema`) was independently re-verified field-by-field
against this pass's own reading of `types/types.go` (not re-trusted from the
prior audit's notes) and found to match, including the previously-flagged
landmine (`EnvironmentProperties.PropertyGroups` vs.
`EnvironmentPropertyDescriptions.PropertyGroupDescriptions` vs.
`EnvironmentPropertyUpdates.PropertyGroups` -- the third one reuses the
create-side name, unrenamed, and gopherstack has this right). Enum values
emitted by non-test code (`DEFAULT`, `DELETING`, `JSON`, `READY`, `RUNNING`,
`SUCCESSFUL`) were grepped and cross-checked against `types/enums.go` --
all real, no fabricated constants.

### Follow-up pass (2026-08-22)

CI's terraform-tests job failed destroying a real
`aws_kinesisanalyticsv2_application`: `DeleteApplication` returned
`InvalidArgumentException` for a `CreateTimestamp` the real
terraform-provider-aws itself had just echoed back. `DeleteApplication`'s
optional safety check already tolerated the wire format's own millisecond
truncation (`epsilon = 1e-3`), but that wasn't the precision loss that
mattered: `terraform-provider-aws`'s `resourceApplicationRead` persists
`create_timestamp` to Terraform state via `time.RFC3339`
(`internal/service/kinesisanalyticsv2/application.go`), a format with *no*
fractional-second component at all, and `resourceApplicationDelete` parses
that string back with `time.Parse(time.RFC3339, ...)` before calling
`DeleteApplication` -- so a real provider-driven delete can only ever send
the whole-second-floored value, off by up to just under one full second,
not one millisecond. Reproduced against the real HashiCorp AWS provider via
`go test ./test/terraform/ -run TestTerraform_KinesisAnalyticsV2` (`tofu
destroy` failing with the exact CI error) and against the real
`aws-sdk-go-v2` client (`TestBackend_DeleteApplication_CreateTimestamp/second-truncated_timestamp_deletes`,
`application_update_test.go`) before the fix, both now passing. Fixed by
widening the tolerance to a full second (`epsilon = 1.0`), verified against
the provider's own source rather than assumed; the mismatch-rejection case
(`wrong := 12345.0`) still fails as expected, so the check still catches a
genuinely wrong value.

`DeleteApplication`'s row in `ops` above still correctly notes the
1ms-vs-wire-truncation rationale for existing; that note is now stale on the
precision figure (says 1e-3, code now uses 1.0) but the underlying
`ops`/ `gaps` grades are unaffected -- not re-touching the YAML front matter
for a single constant's value.

### Follow-up pass (2026-08-29, gopherstack-6flj/21my wrapper-key/silent-drop sweep, V1-vs-V2 lens)

Paired with `services/kinesisanalytics` under the explicit instruction to
verify V1 (`kinesisanalytics`) and V2 (`kinesisanalyticsv2`) do not share
Go types or assume shape parity. **Confirmed 0 shared types**: neither
package imports the other (`grep -rn "kinesisanalytics\"" services/kinesisanalyticsv2/*.go`
and the reverse both come back empty except an unrelated ARN-namespace
string literal in `store.go:109`), each has its own separate `models.go`,
and each is registered under its own SDK module (`kinesisanalytics@v1.33.4`
vs. `kinesisanalyticsv2@v1.41.4`, confirmed via `go.mod`) -- there is no
op-level V1/V2 naming collision within either package for this concern to
even apply to (unlike kafka's V1/V2 cluster ops sharing one package).

Independently re-derived member lists from the pinned SDK's own
`awsAwsjson11_deserializeDocument*`/`serializeOpDocument*` case switches
(not by reading `types.go`) and diffed against gopherstack's structs,
rather than trusting this file's prior audits:
- `ApplicationDetail` (v2): **18 of 18** deserializer cases, all 18 present
  on `applicationDetailOutput` (`handler_applications.go:157-176`),
  including `ApplicationMode`, traced end-to-end request-to-response
  (`handler_applications.go:324` -> `applications.go:37` ->
  `persistence.go:132/180` -> `handler_applications.go:725`) -- not
  write-only.
- `ApplicationSummary` (v2): **6 of 6**, matching `applicationSummary`
  (`models.go:501-508`) exactly -- re-confirms the gopherstack-r80d note
  above independently.
- `ApplicationDetail` (v1): **12 of 12** deserializer cases
  (`deserializers.go:2870`), matching the `applicationDetailOutput`-
  equivalent struct at `models.go:219-230` exactly.
- `InputDescription` (v1): **9 of 9** (`deserializers.go:3400`), matching
  `models.go:81-91` exactly; traced `InputID`/`InputStartingPositionConfiguration`
  to their actual write sites (`application_inputs.go:32`,
  `applications.go:486,641-642`) -- both genuinely wired, not
  present-but-unpopulated.
- `OutputDescription` (v1): **6 of 6** (`deserializers.go:4133`), matching
  `models.go:117-124` exactly.
- `CreateApplicationInput` (v1, request side): **7 of 7** serializer fields
  (`serializers.go:2350`) all read and acted on in
  `handleCreateApplication` (`handler_applications.go:9-77`).

**Real bug found and fixed** (write-only-state, forward direction --
accepted-and-acted-on capability that shouldn't exist): `UpdateApplication`
(v2) accepted an `ApplicationDescription` request field and applied it to
`app.ApplicationDescription` (`application_update_apply.go`'s
`applyBasicFields`, introduced in `3c8a7ff5f`, survived three subsequent
detailed `UpdateApplication` audits of this same op because it visibly
"worked" -- the accepted-but-ignored-field detector this campaign otherwise
relies on doesn't catch a field that IS wired, just wired to something
that doesn't exist). Real AWS's `UpdateApplicationInput` has no such member
(verified by reading `api_op_UpdateApplication.go:33-78` directly -- 8
members total, none of them a description field); there is no real-AWS way
to change an application's description after `CreateApplication`. Caught
by `cmd/acceptguard`'s repo-wide run flagging
`handler_application_update.go:248`. Four existing tests
(`handler_applications_test.go`'s `TestKAV2_UpdateApplication/update_description`,
`handler_application_versions_test.go`'s `TestKAV2_RollbackApplication`,
`applications_test.go`'s `TestBackend_UpdateApplication`,
`whitebox_test.go`'s `TestBackend_UpdateApplication_ConditionalToken`) were
asserting this wrong behavior as correct -- exactly the "asserting wrong
behaviour as correct" pattern this campaign has flagged repeatedly
elsewhere; all four rewritten to use `ServiceExecutionRoleUpdate` (a real
member) as their version-distinguishing marker field instead, and the
`update_description` case now asserts `ApplicationDescription` does NOT
change. New regression test:
`TestKAV2_UpdateApplication_ApplicationDescription_NotARealField`
(`wire_field_fixes_test.go`), confirmed to fail against the pre-fix code.

Write-only-state check, both directions, beyond the bug above: no other
persisted-but-unread or computable-but-unemitted fields found this pass --
`RollbackApplication`'s `ApplicationVersionRolledBackFrom/To` and
`UpdateApplication`'s `ConditionalToken` rotation (both already fixed in
the 2026-08-11 pass) were re-checked and remain correctly wired.

Gates this pass: `go build ./services/kinesisanalytics/... ./services/kinesisanalyticsv2/...`,
`go vet ./...` (repo-wide, since `UpdateApplicationParams`'s field set
changed), `go test ./services/kinesisanalytics/... ./services/kinesisanalyticsv2/... -race -count=1`,
`golangci-lint run --fix ./services/kinesisanalytics/... ./services/kinesisanalyticsv2/...`
-- all clean (0 lint issues, tests pass). `cmd/enumcheck`/`cmd/zeroguard`/
`cmd/xmlitemwrap` repo-wide runs: no findings for either service.
`cmd/acceptguard` repo-wide: one finding (the bug above), re-ran clean
after the fix.

Ops NOT independently re-derived from the deserializer this pass (trusted
from the prior three audits' documented derivations, files unchanged since
`3cec37291`/`782e2a93`): the `Add*`/`Delete*` config family,
`CreateApplicationSnapshot`/`DescribeApplicationSnapshot`/
`ListApplicationSnapshots`/`DeleteApplicationSnapshot`,
`CreateApplicationPresignedUrl`, `DiscoverInputSchema`, and every
`*ConfigurationDescription` sub-shape covered by the 2026-08-20 pass's
field-by-field re-verification.

### Follow-up pass (2026-09-04)

Full parity sweep (`sdk_module` unchanged at `kinesisanalyticsv2@v1.41.4` --
no new ops to re-derive). Diffed `55397dd52..47436caf9`: the only change in
scope was `c78177958`'s `ApplicationDescription` removal, already documented
in this file and re-confirmed present (deleted from
`UpdateApplicationParams`/`updateApplicationInput`/`applyBasicFields`, guard
comment now on `updateApplicationInput`).

**Real bug found and fixed:** `ListApplicationSnapshots` (`application_snapshots.go`)
sorted its result with `sort.Slice(out, func(i, j int) bool { return
out[i].SnapshotCreation.Before(out[j].SnapshotCreation) })` -- a comparator
that returns `false` for equal timestamps, which is not a total order.
`sort.Slice` gives no stability guarantee across ties, so two snapshots of
the same application created close enough together to land on the same
timestamp could come back in either relative order on different calls. The
concrete trigger: the source is `b.snapshotsByApp.Get(...)`, a
`pkgs/store.Index` group whose `remove()` (backing `DeleteApplicationSnapshot`)
swaps the last element into a removed slot (`pkgs/store/index.go:110-133`,
by design -- documented as O(1) removal, not an insertion-order guarantee).
Deleting an unrelated third snapshot in the same application can therefore
silently swap two *other*, untouched, tied snapshots' relative order in the
pre-sort slice, and `sort.Slice` propagates that swap straight into the
result. A client paginating this listing across that page boundary would see
the pair swap sides with nothing about either snapshot itself having
changed -- the identical bug class `c78177958` fixed across
bedrock/cloudwatchlogs/lightsail/quicksight/ssm/macie2/pinpoint/cloudfront/
wafv2 ("sorted on a field that admits ties with no secondary comparison"),
which did not touch this service. Fixed by falling through to `SnapshotName`
(unique per application -- enforced by `CreateApplicationSnapshot`'s
pre-create `b.snapshots.Has` check) when `SnapshotCreation` ties.

Proven with `TestBackend_ListApplicationSnapshots_TieBreak` (`whitebox_test.go`):
puts three snapshots directly into `b.snapshots` (bypassing the real-clock
`CreateApplicationSnapshot` path so two of them share an exact, controlled
timestamp), deletes the unrelated third, and asserts the tied pair's order
is unchanged. Confirmed failing against the pre-fix comparator (asserted
`snap-a` before `snap-b`, got `snap-b` before `snap-a`) before the fix, and
passing after. No other listing in this service shares the shape: `ListApplications`
sorts by `ApplicationName` (the table's own primary key component, so no
ties are possible), `ListTagsForResource` sorts by tag `Key` (AWS enforces
unique tag keys per resource), and `ListApplicationOperations`/
`ListApplicationVersions` do not sort at all -- they return `b.operations`/
`b.versions` in raw insertion order, which `store.go`'s `InMemoryBackend` doc
comment already documents as the reason those two stay plain
`map[string][]*T` rather than `store.Table`+`Index` (order-sensitive append
histories, never rebuilt from a map). `parseNextToken` (`store.go:263`)
already guards `idx < 0`, so this service was never exposed to the separate
negative-continuation-token panic class `c78177958` fixed in eleven other
services.

Five dimensions:
1. **AWS behavior compliance** -- re-verified `ListApplicationSnapshots`'
   ordering contract against `api_op_ListApplicationSnapshots.go`'s doc
   comment (no ordering documented, so gopherstack's creation-time-ascending
   convention is a reasonable choice, now made a genuine total order rather
   than a client-visible flaky one). Every other op's wire/error/state
   grades trusted from the prior five audits, whose files are unchanged
   (confirmed via `git diff 55397dd52..HEAD -- services/kinesisanalyticsv2/`
   touching only the six files `c78177958` changed, all already accounted
   for in this file).
2. **LocalStack parity** -- NOT CHECKED. No LocalStack instance was run
   side-by-side this pass; this file has never recorded a LocalStack
   comparison for this service in any prior pass either.
3. **Cross-service integration** -- checked by reading: every ARN this
   service accepts (`ServiceExecutionRole`, `KinesisStreamsInputDesc.ResourceARN`,
   S3/Glue ARNs inside `ZeppelinApplicationConfiguration`) is stored as a
   plain string with no existence check against the owning service's
   backend, consistent with every other ARN field here. CORRECTED
   (gopherstack-osg7): this file's prior claim that the codebase has no
   cross-service backend-to-backend validation mechanism was wrong --
   `SetAppConfig`/`siblingServices` (grafana/ec2/others) is exactly that
   mechanism; this service just doesn't use it for its ARN fields, which is
   a choice, not a structural absence.
4. **Performance** -- `ListApplications`/`ListApplicationSnapshots`/
   `ListApplicationOperations`/`ListApplicationVersions` all clone and
   (where sorted) re-sort their full per-application/per-region collection
   on every call under one coarse `b.mu.RLock`, before applying the
   `kav2DefaultPageSize=50` page window -- O(n log n) per call, standard for
   this codebase's `pkgs/store` convention and not a new or
   service-specific hotspot. No unbounded scan under a write lock found.
5. **Resource leaks** -- re-confirmed `DeleteApplication` (`applications.go:267-306`)
   still clears `b.snapshots` (via `snapshotsByApp`), `b.versionsStore(region)`,
   and `b.operations[region]`, matching the prior audit's "clean" grade;
   no new leak surface introduced by this pass's fix (`SnapshotName` already
   persisted on `Snapshot`, no new field, no `kinesisanalyticsv2SnapshotVersion`
   bump needed).

Gates this pass: `GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/kinesisanalyticsv2/...`
(0 issues, both before this pass's fix and after) and
`GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/kinesisanalyticsv2/...`
(pass, `ok ... 1.0s`, before/after as described above).
