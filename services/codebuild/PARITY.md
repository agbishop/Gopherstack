---
service: codebuild
sdk_module: aws-sdk-go-v2/service/codebuild@v1.72.4   # version audited against
last_audit_commit: 0627d5d3                             # HEAD when the PRIOR manifest was written;
                                                          # this pass ran under the "no git" constraint
                                                          # and could not read/update this hash
last_audit_date: 2026-09-04
overall: A                # 2026-09-04 pass (parity sweep): 2 genuine bugs found and fixed.
                           # (1) DeleteProject cascade-deleted a project's builds; the real doc
                           # comment (api_op_DeleteProject.go) says plainly builds are NOT deleted.
                           # A prior pass had (incorrectly) recorded the cascade as intentional in
                           # this very file, and janitor_test.go asserted it -- that test was
                           # rewritten, not just the code; see DeleteProject/Notes below.
                           # (2) closed the 2026-08-31 (gopherstack-uox6) gaps: entry --
                           # ListBuildsForProject now rejects sortOrder when the project has more
                           # than 100 builds, per that op's own SortOrder doc comment. Both fixed
                           # with regression tests; golangci-lint 0 issues before and after.
                           # 2026-08-28 pass (gopherstack-6flj write-only-state sweep): 7 genuine
                           # bugs found and fixed via the write-only-state method (backend-persisted
                           # fields with no read path, or request fields accepted then silently
                           # dropped before reaching the backend at all) -- see Notes. All fixed
                           # with real-client round-trip tests in wire_field_fixes_test.go.
                           # 2026-07-23 pass: deleted 3 invented ops, implemented pagination,
                           # sourceVersion, extended Webhook fields (see below). 2026-07-25 pass #1:
                           # field-diffed Fleet against real types.Fleet -- found+fixed a real gap
                           # (id/overflowBehavior/imageId/fleetServiceRole silently unsupported on
                           # Create/UpdateFleet); ComputeConfiguration/ProxyConfiguration/VpcConfig/
                           # ScalingConfiguration were left genuinely unmodeled, holding this at A-.
                           # 2026-07-25 pass #2: implemented all four nested Fleet configuration
                           # objects end to end (request parsing, backend state, response wire
                           # shape, persistence via the existing store.Table JSON round trip) --
                           # gaps: is now empty. DescribeCodeCoverages/DescribeTestCases/
                           # GetReportGroupTrend's empty report-content data remains a genuinely
                           # out-of-scope items_still_open entry (not a gaps: blocker -- see below),
                           # so this reaches A.
                           # 2026-08-11 pass (gopherstack-3y6x follow-up): field-diffed
                           # DescribeCodeCoverages/DescribeTestCases/GetReportGroupTrend's
                           # error sets against botocore -- confirmed the empty-content verdict
                           # is still correct (no report-execution pipeline exists to source real
                           # numbers from) but found the *validation* half was two more bugs:
                           # DescribeTestCases/GetReportGroupTrend accepted a nonexistent
                           # reportArn/reportGroupArn and returned success (real AWS declares
                           # ResourceNotFoundException for both; DescribeCodeCoverages correctly
                           # does not); GetReportGroupTrend's trendField was parsed and never
                           # validated against its 9-value enum. Also field-diffed
                           # CodeCoverage/TestCase against real types -- both had invented field
                           # names. Swept Delete* ops repo-wide for the same existence-check
                           # pattern and found the inverse bug in 5 places: DeleteProject/
                           # DeleteBuildBatch/DeleteReport/DeleteReportGroup/DeleteFleet all
                           # rejected a nonexistent resource with ResourceNotFoundException, but
                           # real AWS declares no such exception for any of the five (idempotent
                           # delete) -- fixed all five. ListReportsForReportGroup was missing the
                           # same reportGroupArn existence check as GetReportGroupTrend/
                           # DescribeTestCases -- fixed.
                           # 2026-08-30 pass (gopherstack-6flj wrapper-key sweep,
                           # workspaces/codebuild/elasticbeanstalk batch): type-aware field-usage
                           # scan of all 59 request structs (90 named XxxInput types minus dupes)
                           # flagged 2 declared-but-never-referenced fields. Hand-verified against
                           # the pinned SDK per this sweep's own rule: DeleteReportGroup.DeleteReports
                           # was a genuine bug (fixed, see ops below); ImportSourceCredentials.Username
                           # was NOT a bug on inspection -- already correctly disclosed as a
                           # deliberate non-fix in the 2026-08-23 gopherstack-secp note below (real
                           # SourceCredentialsInfo has no Username member to round-trip through any
                           # response, same as the sibling Token field already discarded by design).
                           # No other unread fields found across workspaces (0/90) or codebuild
                           # (2/59, one real, one already-disclosed non-bug).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateProject:   {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28: badgeEnabled, Source.buildStatusConfig/gitSubmodulesConfig, Environment.computeConfiguration/dockerServer/fleet/hostKernel were all silently dropped; see Notes. Prior fix: now threads top-level sourceVersion, see gaps fixed below"}
  UpdateProject:   {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28, same fields as CreateProject (badgeEnabled/Source/Environment gaps). Prior fix: same as CreateProject"}
  DeleteProject:   {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-04: no longer cascade-deletes the project's builds -- api_op_DeleteProject.go states plainly 'When you delete a project, its builds are not deleted', but the backend deleted them via the buildsByProject index anyway; a prior pass's PARITY note had misdescribed this as an intentional cleanup. The stale janitor_test.go assertion of the cascade (TestDeleteProject_CleanupBuilds) was rewritten to TestDeleteProject_DoesNotCleanupBuilds asserting the correct survive-deletion behavior. Also idempotent on a nonexistent name -- real AWS declares no ResourceNotFoundException for this op, gopherstack previously invented one"}
  BatchGetProjects: {wire: ok, errors: ok, state: ok, persist: ok, note: "includes webhook and sourceVersion fields"}
  ListProjects:    {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: nextToken/sortBy(NAME|CREATED_TIME|LAST_MODIFIED_TIME)/sortOrder all implemented via ListProjectsSortedBy + paginateIDs, 100-item default page matching real AWS"}
  StartBuild:      {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28: sourceVersion override was corrupting Source.Location (Build had no SourceVersion field at all); artifactsOverride was parsed off the wire then silently dropped, never reaching the backend; ~20 more real override fields (cacheOverride/environmentTypeOverride/fleetOverride/etc.) were entirely unmodeled. AutoRetryConfig (real Build field) added. env var override uses correct AWS replace-by-name-else-append merge semantics"}
  StopBuild:       {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetBuilds:  {wire: ok, errors: ok, state: ok, persist: ok, note: "accepts both build ID and ARN via buildsByARN index"}
  ListBuilds:      {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: nextToken/sortOrder via paginateIDs (ListBuilds has no sortBy/maxResults in the real request shape)"}
  ListBuildsForProject: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: nextToken/sortOrder via paginateIDs. FIXED 2026-09-04: sortOrder is now rejected with InvalidInputException when the project has more than 100 builds, per api_op_ListBuildsForProject.go's SortOrder doc comment ('If the project has more than 100 builds, setting the sort order will result in an error') -- previously accepted and silently sorted anyway; see former gaps: entry (gopherstack-uox6), now closed"}
  RetryBuild:      {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28: now maintains the real AutoRetryConfig chain (AutoRetryNumber/PreviousAutoRetry/NextAutoRetry), a real Build field with no prior model support. inherits env/source/artifacts/role/timeouts from original build, matching AWS"}
  BatchDeleteBuilds: {wire: ok, errors: ok, state: ok, persist: ok}
  StartBuildBatch: {wire: ok, errors: ok, state: ok, persist: ok}
  StopBuildBatch:  {wire: ok, errors: ok, state: ok, persist: ok}
  RetryBuildBatch: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetBuildBatches: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteBuildBatch: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: now idempotent on a nonexistent id, same real-AWS error-contract fix as DeleteProject"}
  ListBuildBatches: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: filter.status/nextToken/sortOrder/maxResults implemented, and the op is now documented here (it was already routed/tested pre-pass, just missing from this manifest)"}
  ListBuildBatchesForProject: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass, same as ListBuildBatches; also newly documented here"}
  CreateReportGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateReportGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteReportGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "idempotent on a nonexistent arn, same real-AWS error-contract fix as DeleteProject. FIXED 2026-08-30 (gopherstack-6flj wrapper-key sweep): DeleteReportGroupInput.DeleteReports (real, api_op_DeleteReportGroup.go) was parsed off the wire and never passed to the backend -- deleting a group with existing reports always silently succeeded (real AWS: 'If you call DeleteReportGroup for a report group that contains one or more reports, an exception is thrown' when DeleteReports is false) and DeleteReports=true never cascade-deleted the group's reports, leaving them orphaned. Now: DeleteReports=false + existing reports -> InvalidInputException; DeleteReports=true -> reports deleted along with the group."}
  BatchGetReportGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "accepts ARN or bare name"}
  ListReportGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: nextToken/sortBy(NAME|CREATED_TIME|LAST_MODIFIED_TIME)/sortOrder/maxResults via ListReportGroupsSortedBy + paginateIDs"}
  BatchGetReports: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteReport:    {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: now idempotent on a nonexistent arn, same real-AWS error-contract fix as DeleteProject"}
  ListReports:     {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: filter.status/nextToken/sortOrder/maxResults implemented"}
  ListReportsForReportGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: pagination (as before), plus a real gap found this audit -- a nonexistent reportGroupArn returned an empty list instead of ResourceNotFoundException (real AWS declares that exception for this op, unlike ListReports/ListReportGroups)"}
  GetReportGroupTrend: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: reportGroupArn existence and trendField enum (9 real values) are now validated (both were previously accepted-then-ignored -- ResourceNotFoundException/InvalidInputException respectively); rawData (a real response field, previously missing entirely) is now present as an empty list. Content itself (stats map) remains empty -- no report-execution data is modeled, and none can be fabricated; see items_still_open"}
  DescribeCodeCoverages: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: reportArn is now required (was unchecked); CodeCoverage's field names were fixed to match the real type (id/reportARN/lineCoveragePercentage/branchCoveragePercentage/linesCovered/linesMissed/branchesCovered/branchesMissed/expired -- previously invented filePath/branchCoverage/lineCoverage). Confirmed (not changed) that a nonexistent reportArn correctly still returns an empty list rather than ResourceNotFoundException -- real AWS declares no such exception for this op, unlike DescribeTestCases/GetReportGroupTrend. Content itself remains empty; see items_still_open"}
  DescribeTestCases: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: reportArn is now required and validated to exist (real AWS declares ResourceNotFoundException for this op; previously accepted any ARN and returned an empty list); TestCase's field names were fixed to match the real type (reportArn/testRawDataPath/prefix/name/status/durationInNanoSeconds/message/testSuiteName/expired -- previously invented duration). Content itself remains empty; see items_still_open"}
  CreateFleet:     {wire: ok, errors: ok, state: ok, persist: ok, note: "id (Fleet had no separate id field at all -- now uuid-generated), overflowBehavior, imageId, fleetServiceRole fixed in the earlier 2026-07-25 pass. Second 2026-07-25 pass: computeConfiguration/proxyConfiguration/vpcConfig/scalingConfiguration now also accepted, stored, and echoed back (scalingConfiguration's desiredCapacity is populated from baseCapacity, matching AWS's no-scaling-event-yet behavior -- see fleets.go's outputScalingConfiguration doc comment)"}
  UpdateFleet:     {wire: ok, errors: ok, state: ok, persist: ok, note: "computeType/environmentType/overflowBehavior/imageId/fleetServiceRole fixed in the earlier 2026-07-25 pass. Second 2026-07-25 pass: computeConfiguration/proxyConfiguration/vpcConfig/scalingConfiguration now also updatable (nil pointer leaves the existing value unchanged, non-nil overwrites -- matches real UpdateFleetInput's partial-update semantics)"}
  DeleteFleet:     {wire: ok, errors: ok, state: ok, persist: ok, note: "accepts ARN or bare name. FIXED this pass: now idempotent on a nonexistent name/arn, same real-AWS error-contract fix as DeleteProject"}
  BatchGetFleets:  {wire: ok, errors: ok, state: ok, persist: ok}
  ListFleets:      {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: nextToken/sortBy(NAME|CREATED_TIME|LAST_MODIFIED_TIME)/sortOrder/maxResults via ListFleetsSortedBy + paginateIDs; also fixed default ordering to be NAME-ascending (was ARN-string-ascending, an internal artifact with no real-AWS basis)"}
  CreateWebhook:   {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: adds manualCreation/scopeConfiguration/pullRequestBuildPolicy request fields and status/secret/lastModifiedSecret/statusMessage response fields, see gaps fixed below"}
  UpdateWebhook:   {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: adds pullRequestBuildPolicy/rotateSecret request fields (rotateSecret regenerates secret+lastModifiedSecret)"}
  DeleteWebhook:   {wire: ok, errors: ok, state: ok, persist: ok, note: "clears Project.Webhook"}
  ImportSourceCredentials: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-audited 2026-08-23 (gopherstack-secp): Username is parsed off the wire and discarded, same as Token -- not a fix, see Notes"}
  DeleteSourceCredentials: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSourceCredentials: {wire: ok, errors: ok, state: ok, persist: ok}
  PutResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "idempotent, matches AWS"}
  UpdateProjectVisibility: {wire: ok, errors: ok, state: ok, persist: ok, note: "generates/clears publicProjectAlias correctly on PUBLIC_READ toggle"}
  InvalidateProjectCache: {wire: ok, errors: ok, state: ok, persist: n/a, note: "correctly a real no-op (cache not modeled) once project existence is validated"}
  StartSandbox:    {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28: Sandbox never inherited environment/source/vpcConfig/serviceRole/encryptionKey/sourceVersion/secondarySources/fileSystemLocations/timeouts from the project (types.Sandbox carries the same project-derived field set as types.Build); Sandbox.Environment/Source/etc. were always nil regardless of project config"}
  StopSandbox:     {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetSandboxes: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSandboxes:   {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: nextToken/sortOrder/maxResults via paginateIDs"}
  ListSandboxesForProject: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass, same as ListSandboxes"}
  StartSandboxConnection: {wire: partial, errors: ok, state: ok, persist: n/a, note: "returns a synthesized wss:// endpoint; real interactive terminal not modeled, acceptable for an emulator"}
  StartCommandExecution: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28: ExitCode was modeled as int32; real wire type is string (deserializer: expected NonEmptyString to be of type string) -- latent hard-decode-error risk once ever populated (it never was, pre-fix). standardErrContent wire key was misspelled standardErrorContent, so real AWS's field was always nil"}
  BatchGetCommandExecutions: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCommandExecutionsForSandbox: {wire: ok, errors: ok, state: ok, persist: ok, note: "correctly returns full CommandExecution objects, not just IDs. FIXED 2026-08-29 (wrapper-key sweep): maxResults/nextToken/sortOrder were real ListCommandExecutionsForSandboxInput fields (aws-sdk-go-v2 api_op_ListCommandExecutionsForSandbox.go) that listCommandExecutionsForSandboxInput didn't even declare -- json.Unmarshal silently dropped them, so every call returned every execution, unpaginated, always ascending-ID order. Now uses a new paginateCommandExecutions helper (pagination.go), the same nextToken/sortOrder semantics as every other List op's shared paginateIDs, generalized to page full objects since this op (unlike its siblings) returns CommandExecution records directly rather than bare IDs for a separate BatchGet* step. See TestCodeBuild_CommandExecutionsForSandbox/pagination_and_sort_order."}
  ListCuratedEnvironmentImages: {wire: ok, errors: n/a, state: ok, persist: n/a, note: "hardcoded minimal image catalog, acceptable (AWS's own catalog is also effectively static reference data)"}
  ListSharedProjects: {wire: ok, errors: n/a, state: ok, persist: n/a, note: "correctly empty — no cross-account project sharing modeled"}
  ListSharedReportGroups: {wire: ok, errors: n/a, state: ok, persist: n/a, note: "correctly empty, same reasoning"}
families:
  errors: {status: ok, note: "handleError maps ErrNotFound/ErrAlreadyExists/ErrValidation to ResourceNotFoundException/ResourceAlreadyExistsException/InvalidInputException at 400, matching real AWS; all backend ErrNotFound paths reach errCodeLookup correctly; invalid nextToken now also maps to InvalidInputException via ErrValidation"}
  persistence: {status: ok, note: "Handler.Snapshot/Restore delegate to InMemoryBackend.Snapshot/Restore, versioned (codebuildSnapshotVersion), backed by store.Registry across all store.Table-based resource maps plus a plain resourcePolicies map"}
  janitor: {status: ok, note: "janitor.tick runs sweepCompletedBuilds (TTL eviction) then advanceInProgressBuilds (status advancement) every tick"}
  tags: {status: ok, note: "REMOVED this pass: TagResource/UntagResource/ListTagsForResource were gopherstack-invented operations with no counterpart on the real aws-sdk-go-v2/service/codebuild Client (verified: the SDK module has no api_op_TagResource.go/api_op_UntagResource.go/api_op_ListTagsForResource.go, and Client's exported method set — grepped directly from api_op_*.go — has no such methods). Real AWS CodeBuild only supports tagging inline via the `tags` field on CreateProject/CreateReportGroup/CreateFleet/UpdateProject (already implemented and unaffected). Deleted services/codebuild/tags.go, handler_tags.go, tags_test.go; removed the 3 ops from GetSupportedOperations()/dispatchTable(); TestHandler_GetSupportedOperations now asserts their absence."}
items_still_open:            # genuinely unfinished — do not mark ok
  - "DescribeCodeCoverages/DescribeTestCases/GetReportGroupTrend always return empty content (codeCoverages/testCases/stats) because no report actually populates coverage/test-case/trend data anywhere in the backend (reports are seed-only via the AddReportInternal test helper — there is no real CodeBuild API to push test-case/coverage content; on real AWS it's ingested by the managed build agent parsing buildspec `reports` sections and artifact files, which this emulator's build execution does not model). This remains genuinely correct to leave empty rather than fabricate numbers a client cannot distinguish from real data. Implementing this for real would require modeling report-content ingestion from build artifacts, which is out of scope for this pass. NOTE: as of the 2026-08-11 pass, this is now *only* a content gap -- the request validation these three ops perform (required fields, ARN existence where real AWS declares it, trendField enum) is complete and correct; see ops: above."
gaps:                      # known divergences NOT fixed — link bd issue ids. Fleet's
                           # ComputeConfiguration/ProxyConfiguration/VpcConfig/ScalingConfiguration
                           # (found genuinely unmodeled in the first 2026-07-25 pass) were
                           # implemented end to end in the second 2026-07-25 pass -- see Notes.
  - "FIXED 2026-09-04 (see ListBuildsForProject above): the 2026-08-31 (gopherstack-uox6) ListBuildsForProjectInput.SortOrder>100-builds gap is closed."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "Report-content ingestion (DescribeCodeCoverages/DescribeTestCases/GetReportGroupTrend real data) — see items_still_open above for why this is a substantially larger feature (build artifact parsing), not a quick fix."
leaks: {status: clean, note: "janitor.Run selects on ctx.Done() and calls worker.Group.Stop(); TestCodeBuildJanitor_RunContext passes under -race. paginateIDs/ListProjectsSortedBy/ListFleetsSortedBy/ListReportGroupsSortedBy are pure functions under the existing RLock scope — no new goroutines, no new lock paths, all backend locks remain defer-released."}
---

## Notes

### 2026-08-23 audit of `gopherstack-secp` (ImportSourceCredentials Username)

Confirmed real: `ImportSourceCredentialsInput.Username` exists (pinned SDK
`codebuild@v1.72.4/api_op_ImportSourceCredentials.go:57-59`, `*string`, not
required -- doc comment: "The Bitbucket username when the authType is
BASIC_AUTH. This parameter is not valid for other types of source providers
or connections"). `handler_source_credentials.go`'s
`importSourceCredentialsInput` parses it off the wire but
`handleImportSourceCredentials` never passes it to
`InMemoryBackend.ImportSourceCredentials` (`source_credentials.go`), and the
backend's `SourceCredentials` struct (`models.go`) has no field for it.

Not fixed, reclassified as a modelling gap rather than accept-and-drop data
loss, for two reasons:

1. **No API surface can ever observe it either way.** The real
   `SourceCredentialsInfo` (the only type any codebuild op returns for stored
   credentials -- `types/types.go:2785-2802`) has exactly four members
   (Arn/AuthType/Resource/ServerType) and no `Username`. AWS itself never
   returns Bitbucket usernames back through any op; a black-box parity test
   cannot distinguish "gopherstack stores Username but never serializes it"
   from "gopherstack drops Username on the floor" -- there is no
   `ListSourceCredentials` (or any other) response shape a real SDK client
   could assert against, so the round-trip proof the issue proposed is not
   constructible against the real API surface.
2. **The existing design already discards the sibling required field the
   same way.** `ImportSourceCredentials`'s `Token` -- the actual auth
   secret, always required -- is accepted and explicitly discarded
   (`source_credentials.go`: `_ = token`) for the identical reason: nothing
   in this emulator ever authenticates to a real Git host, and no op
   surfaces stored credential material back. Adding a `Username` field that
   is stored but that no real op will ever read would be exactly the
   "fabricated/never-read field" anti-pattern this pass was warned against,
   not a fix for it.

No code changed for this issue.

Protocol: awsjson1.1 (single POST endpoint, `X-Amz-Target: CodeBuild_20161006.<Op>`).
Route matcher (`RouteMatcher`) is a simple `X-Amz-Target` prefix check — verified every
op in `GetSupportedOperations()` round-trips through `doRequest`'s header-based dispatch
in the existing handler_test.go/codebuild_ops_test.go/aws_accuracy_test.go suites, not just
via `Handler()` direct calls that could bypass the matcher.

Timestamps (`created`, `lastModified`, `startTime`, `endTime`, etc.) are plain `float64`
Unix-seconds fields that marshal as bare JSON numbers — this is wire-compatible with the
real deserializer's `case "created": ... jtv.(json.Number) -> smithytime.ParseEpochSeconds`
path (confirmed by reading `codebuild@v1.68.11/deserializers.go`), even though the field
isn't typed via `pkgs/awstime.Epoch`. Not a bug; noted so a future auditor doesn't "fix" it.

Pagination: `services/codebuild/pagination.go`'s `paginateIDs` applies nextToken/maxResults/
sortOrder uniformly across every List* op, using `pkgs/page.New` for the opaque-offset token
(matching the pattern already used by e.g. `services/acm`). `defaultListPageSize = 100` matches
real AWS's documented page cap. `ListProjects`/`ListFleets`/`ListReportGroups` additionally
support `sortBy` (NAME|CREATED_TIME|LAST_MODIFIED_TIME) via dedicated
`List*SortedBy(sortBy string)` backend methods, since real AWS's request shape for exactly
these three ops includes a `sortBy` field (confirmed by field-diffing
`api_op_ListProjects.go`/`api_op_ListFleets.go`/`api_op_ListReportGroups.go`
against `api_op_ListBuilds.go` et al, which have no `sortBy`). `ListBuildBatches(ForProject)`/
`ListReports(ForReportGroup)` additionally support `filter.status`, matching real AWS's
`BuildBatchFilter`/`ReportFilter` request shapes (each a single optional `status` field).

### Invented-ops deletion this pass

**TagResource / UntagResource / ListTagsForResource were gopherstack inventions, not real AWS
CodeBuild operations.** Field-diffed against `aws-sdk-go-v2/service/codebuild@v1.68.11`: the
module directory has no `api_op_TagResource.go`, `api_op_UntagResource.go`, or
`api_op_ListTagsForResource.go`, and grepping every `func (c *Client) ...` across all
`api_op_*.go` files in the module yields exactly 59 operations — none of which are these three.
Real CodeBuild only exposes tagging inline via the `tags` field on `CreateProject`/
`UpdateProject`/`CreateReportGroup`/`CreateFleet` (cross-service ARN-based tag discovery is the
job of the separate `resourcegroupstaggingapi` service on real AWS, which CodeBuild does not
duplicate). Deleted `tags.go` (backend `TagResource`/`UntagResource`/`ListTagsForResource`
methods), `handler_tags.go` (the three op handlers), and `tags_test.go` (12 tests exercising the
invented API). Removed the three names from `GetSupportedOperations()`/`dispatchTable()` in
`handler.go`. `TestHandler_GetSupportedOperations` now asserts these three are absent, so a
future accidental reintroduction gets caught immediately. `janitor_test.go`'s
`TestJanitor_SweepCleansARNIndex` (which used `TagResource` on an evicted build's ARN purely as
a convenient "does an ARN-based op see this as gone" probe) was rewritten to use `BatchGetBuilds`
instead — same assertion (no ghost row after eviction), real op.

### Bugs fixed this pass

1. **Missing pagination wire shape on every List\* operation** (`services/codebuild/pagination.go`,
   `handler_projects.go`, `handler_builds.go`, `handler_build_batches.go`, `handler_reports.go`,
   `handler_fleets.go`, `handler_sandboxes.go`). Every List* op previously accepted but silently
   ignored `nextToken`/`sortBy`/`sortOrder`/`maxResults`, always returning the full unpaginated
   result set with `nextToken` always omitted. A real client relying on the SDK paginator's
   `HasMorePages()` (which checks the returned `nextToken`, not result-set size) would still
   terminate correctly, but a client testing "at least 100 items on this page then a token"
   load-testing/pagination-contract scenario, or a client using `sortOrder=DESCENDING` to see
   newest-first results, would silently get wrong data. Fixed by adding a shared
   `paginateIDs(all []string, nextToken, sortOrder string, maxResults int32) (page.Page[string], error)`
   helper (using `pkgs/page`, matching the pattern in `services/acm`) and wiring it into every
   List* handler. `ListProjects`/`ListFleets`/`ListReportGroups` (the three ops whose real
   request shape has `sortBy`) got new `List*SortedBy(sortBy string)` backend methods;
   `ListFleets`'s default order was also corrected from ARN-string-ascending (an internal
   implementation artifact) to NAME-ascending, consistent with `ListProjects`/`ListReportGroups`
   and with `sortBy=NAME`.

2. **`Project` missing the top-level `sourceVersion` field** (`services/codebuild/models.go`,
   `projects.go`, `handler_projects.go`). Real AWS's `Project` shape has a `sourceVersion` field
   distinct from `secondarySourceVersions` (confirmed via `codebuild@v1.68.11/types/types.go`);
   `CreateProjectInput`/`UpdateProjectInput` also carry it. Nothing threaded it through before —
   a client setting `sourceVersion` on `CreateProject` would silently have it dropped. Fixed by
   adding `Project.SourceVersion`, `ProjectConfig.SourceVersion`, and wiring it through
   `CreateProject`/`UpdateProject`/`applyProjectOptionalFields` (update semantics: non-empty
   value overwrites, matching every other optional string field on this resource) and the
   `createProjectInput`/`updateProjectInput` wire structs.

3. **`Webhook` missing `status`/`statusMessage`/`manualCreation`/`lastModifiedSecret`/`secret`/
   `pullRequestBuildPolicy`/`scopeConfiguration`** (`services/codebuild/models.go`,
   `webhooks.go`, `handler_webhooks.go`). Field-diffed against
   `codebuild@v1.68.11/types/types.go`'s `Webhook` struct. Since this emulator never performs a
   real GitHub/GitLab/Bitbucket round-trip, `CreateWebhook` now synthesizes a terminal
   `status: "ACTIVE"` immediately (the state a client would eventually observe on real AWS after
   webhook provisioning completes) plus a generated `secret` and `lastModifiedSecret` timestamp;
   `manualCreation`/`scopeConfiguration` are accepted on `CreateWebhook` and echoed back;
   `pullRequestBuildPolicy` is accepted on both `CreateWebhook` and `UpdateWebhook`; `UpdateWebhook`
   also gained `rotateSecret` (regenerates `secret` + bumps `lastModifiedSecret` when true, leaves
   both untouched otherwise, matching real AWS's `UpdateWebhookInput.rotateSecret` semantics).

Covered by new table-driven tests: `TestHandler_ListProjects_SortOrderDescending`,
`TestHandler_ListProjects_InvalidNextToken`, `TestHandler_ListProjects_SortByCreatedTime`,
`TestHandler_ListFleets_MaxResultsPagination`, `TestHandler_ListBuildBatches_FilterByStatus`,
`TestHandler_ListReports_FilterByStatus` (`pagination_test.go`); `TestHandler_Project_SourceVersion`
(`projects_test.go`); `TestHandler_CreateWebhook_ExtendedFields`,
`TestHandler_UpdateWebhook_RotateSecret` (`webhooks_test.go`).

Prior-pass fixes (builds/build batches stuck IN_PROGRESS forever; `Project.Webhook` not mirrored
after `CreateWebhook`) remain in place and are covered by `TestJanitor_AdvanceInProgressBuilds` /
`TestJanitor_AdvanceInProgressBuilds_LeavesTerminalBuildsAlone` (`janitor_test.go`) and
`TestHandler_Webhook_MirroredOnProject` (`webhooks_test.go`).

### 2026-07-25 pass: Fleet field-diff

Field-diffed `Fleet`/`CreateFleetInput`/`UpdateFleetInput` against
`aws-sdk-go-v2/service/codebuild@v1.68.11/types/types.go` and
`awsAwsjson11_deserializeDocumentFleet` directly (not against gopherstack's own
output, per parity-principles.md rule 2). Found and fixed a real, previously-unflagged
gap: `Fleet` had no `id` field at all (a real, separate field from `name`/`arn` on
`types.Fleet`), and `CreateFleetInput`/`UpdateFleetInput`'s wire structs had no
`overflowBehavior`/`imageId`/`fleetServiceRole` members, so a real client setting any
of these had them silently dropped on create and had **no way at all** to change them
(or `computeType`/`environmentType`) after creation via `UpdateFleet`, which previously
only ever touched `baseCapacity`. Fixed by adding `Fleet.ID`/`Fleet.ImageID`, generating
a UUID `id` at `CreateFleet` time (mirroring how other resources in this service
generate IDs), and refactoring `CreateFleet`/`UpdateFleet`'s backend signatures to take
`CreateFleetOptions`/`UpdateFleetOptions` structs (the growing flat-positional-parameter
lists were becoming unwieldy) wired through from new `createFleetInput`/
`updateFleetInput` JSON fields. `UpdateFleet`'s "empty string leaves field unchanged"
semantics mirror the existing `applyProjectOptionalFields` convention for optional
string-field updates on this service.

**STALE CLAIM, FIXED in the very next section (2026-07-25 pass #2 below) -- flagged by
`cmd/staleclaims`, gopherstack-anjf: this paragraph was dispatch-bait for a reader who
stopped at the first match.** Original text follows, kept for history: "Also found, NOT
fixed in this pass": `Fleet.ComputeConfiguration`/`ProxyConfiguration`/
`VpcConfig`/`ScalingConfiguration` (all real fields on `types.Fleet`) remained entirely
unmodeled -- these are nested objects (attribute-based-compute vCPU/memory/disk specs,
subnet/security-group VPC config, scaling-type semantics) that would require real design
work, not a wire-shape passthrough fix. Documented as a new `gaps:` entry rather than
silently left unflagged like the id/overflowBehavior/imageId/fleetServiceRole gap was
before this pass. **Closed in the second 2026-07-25 pass immediately below.**

Covered by new tests: `TestHandler_CreateFleet_ExtendedFields`,
`TestHandler_UpdateFleet_ExtendedFields` (`fleets_test.go`).

### 2026-07-25 pass #2: Fleet nested configuration objects (closes the gap above)

Implemented `ComputeConfiguration`/`ProxyConfiguration`/`VpcConfig`/`ScalingConfiguration`
end to end, field-diffed against `aws-sdk-go-v2/service/codebuild@v1.68.11`'s
`types.ComputeConfiguration`/`types.ProxyConfiguration`/`types.VpcConfig`/
`types.FleetProxyRule`/`types.TargetTrackingScalingConfiguration`/
`types.ScalingConfigurationInput`/`types.ScalingConfigurationOutput` directly (`go doc`
plus reading `serializers.go`'s `awsAwsjson11_serializeOpDocumentCreateFleetInput`/
`UpdateFleetInput` for exact request field names, and `deserializers.go`'s
`awsAwsjson11_deserializeDocumentFleet` for the response). Added new model types
(`models.go`): `ComputeConfiguration`, `ProxyConfiguration`, `FleetProxyRule`,
`TargetTrackingScalingConfig`; extended the existing (previously dead-field) `ScalingConfiguration`
type with `TargetTrackingScalingConfigs`. `Fleet.VpcConfig` deliberately reuses the
existing `VpcConfig` type already defined for `Project` -- the real
`aws-sdk-go-v2/service/codebuild/types.VpcConfig` used by both `Fleet` and `Project` has
the identical shape (`securityGroupIds`/`subnets`/`vpcId`), so a second, duplicate type
would have been pure duplication.

Wired through `CreateFleetOptions`/`UpdateFleetOptions` (fleets.go) -- extending the
existing options structs from the prior pass rather than inventing a parallel path, per
this pass's instructions -- and the `createFleetInput`/`updateFleetInput` JSON wire
structs (handler_fleets.go). `UpdateFleet`'s nested-object semantics: a non-nil pointer
overwrites, `nil` (absent from the request) leaves the existing value unchanged, matching
real `UpdateFleetInput`'s partial-update contract (distinct from the string fields' "empty
string leaves unchanged" convention, since a nested object has no equivalent "empty"
sentinel).

`ScalingConfiguration.DesiredCapacity` is response-only on real AWS (`types.
ScalingConfigurationOutput` has it; `types.ScalingConfigurationInput`, the request shape,
does not) -- confirmed by diffing the two types separately in `types/types.go`. Since this
emulator does not model live auto-scaling telemetry, `outputScalingConfiguration`
(`fleets.go`) populates it with the fleet's `baseCapacity` on every `Create`/`UpdateFleet`
response, matching real AWS's own value immediately after a create/update, before any
scaling event has occurred -- not a fabricated number, the literal correct value for that
moment.

**Disguised-stub pattern found and fixed while doing this**: `Fleet.ScalingConfiguration`
already existed as a model field (with a real JSON tag) before this pass, but nothing
anywhere ever set it -- a stray leftover from an earlier partial attempt, matching the
"field exists, no write-sites" bug class documented in `parity-principles.md`. It is now
genuinely wired end to end.

Covered by a new table test, `TestHandler_Fleet_NestedConfiguration` (`fleets_test.go`,
cases: `create_computeConfiguration`, `create_proxyConfiguration`, `create_vpcConfig`,
`create_scalingConfiguration_desiredCapacityMatchesBase`,
`update_overwrites_nested_configuration`,
`update_without_nested_configuration_leaves_it_unchanged`).

### 2026-08-11 pass: report-content follow-up (gopherstack-3y6x)

Re-examined the `items_still_open` premise that `DescribeCodeCoverages`/`DescribeTestCases`/
`GetReportGroupTrend` "always return empty, blamed on no report-content ingestion pipeline."
Confirmed the content verdict is still correct (no build-artifact/report-content pipeline
exists to source real numbers from, and fabricating them would be worse than an empty
response per `parity-principles.md`) -- but the *validation* half of each op was checked
against `aws-sdk-go-v2/service/codebuild@v1.72.4`'s botocore source
(`codebuild/2016-10-06/service-2.json`'s per-operation `errors` list, which is the
authoritative declared-exception contract each op's real deserializer is generated from)
and two of the three had a real bug:

- **`DescribeTestCases`/`GetReportGroupTrend` accepted a nonexistent `reportArn`/
  `reportGroupArn` and returned 200 with empty content.** Real AWS declares
  `ResourceNotFoundException` for both ops. Fixed: both now look up the resource first and
  return `ErrNotFound` if absent (`reports.go`'s `DescribeTestCases`/`GetReportGroupTrend`).
  **`DescribeCodeCoverages` does *not* declare `ResourceNotFoundException`** (only
  `InvalidInputException`) -- confirmed by reading the same errors list -- so its identical
  "accept anything, return empty" behavior was already correct and was left unchanged; a new
  test (`describe_code_coverages_nonexistent_report_still_returns_empty`) documents this
  asymmetry so a future pass doesn't "fix" it into a regression.
- **`GetReportGroupTrend`'s `trendField` was parsed and never validated.** Real AWS's
  `ReportGroupTrendFieldType` is a 9-value enum (`types/enums.go:895-926`); any string,
  including garbage, was silently accepted. Fixed: `handler_reports.go` now checks
  `trendField` against the real enum via `slices.Contains`, rejecting with
  `InvalidInputException` otherwise.
- **`reportArn`/`reportGroupArn` were never checked for presence** on any of the three ops
  (nor was `GetReportGroupTrend`'s `reportGroupArn`) -- all are `required` members on the real
  input shapes. Fixed: added the standard `in.X == ""` → `errInvalidRequest` check already
  used throughout this file (see `handleDeleteReport` etc.) to all three.
- **`GetReportGroupTrend`'s response was missing `rawData`**, a real member of
  `GetReportGroupTrendOutput` (`api_op_GetReportGroupTrend.go`) — the handler only ever
  returned `stats`. Fixed: `rawData` is now present as an empty list (structurally correct,
  not fabricated — there is no report data to populate it with).
- **`CodeCoverage`/`TestCase` had invented field names**, not matching
  `aws-sdk-go-v2/service/codebuild@v1.72.4/types.CodeCoverage`/`types.TestCase` (verified via
  `deserializers.go`'s `awsAwsjson11_deserializeDocumentCodeCoverage`). `CodeCoverage` had
  only `filePath`/`branchCoverage`/`lineCoverage`; real AWS has 10 fields including
  `id`/`reportARN`/`lineCoveragePercentage`/`branchCoveragePercentage`/`linesCovered`/
  `linesMissed`/`branchesCovered`/`branchesMissed`/`expired`. `TestCase` had only
  `name`/`status`/`duration`; real AWS has 9 fields including `reportArn`/`testRawDataPath`/
  `prefix`/`durationInNanoSeconds`/`message`/`testSuiteName`/`expired` (no `duration` field
  exists on the real type at all). Fixed both models to match; `handler_reports.go` now
  marshals the real slices directly instead of hand-building `map[string]any` with the wrong
  keys. Since both lists remain always-empty (per the content verdict above), this has no
  observable effect today, but is now correct for whenever report-content ingestion is
  eventually implemented.

**Deliberately not implemented**: `DescribeCodeCoverages`'s `sortBy`/`sortOrder`/`maxResults`/
`nextToken`/`minLineCoveragePercentage`/`maxLineCoveragePercentage` and `DescribeTestCases`'s
`filter`/`maxResults`/`nextToken` request fields. Unlike the pagination/filter parameters
fixed on `ListReports`/`ListBuildBatches`/etc in the 2026-07-23 pass (which silently returned
wrong results against *real, non-empty* data), these parameters would have zero observable
effect: the result set they'd sort/filter/paginate is provably always empty (see above), so
accepting-and-ignoring them is behaviorally identical to not accepting them at all. Revisit
this decision if/when report-content ingestion is ever implemented.

**Swept the rest of the service for the same two bug classes** (nonexistent-resource
accepted; hand-written checks vs the real error contract) and found:

- **The inverse bug in five `Delete*` ops.** `DeleteProject`/`DeleteBuildBatch`/`DeleteReport`/
  `DeleteReportGroup`/`DeleteFleet` all rejected a nonexistent resource with
  `ResourceNotFoundException` (400) — but real AWS declares no such exception for *any* of the
  five (`DeleteProject.errors`/`DeleteBuildBatch.errors`/`DeleteReport.errors`/
  `DeleteReportGroup.errors`/`DeleteFleet.errors`: all just `["InvalidInputException"]`),
  meaning all five are idempotent deletes on real AWS. This matches the precedent already
  established (and already correctly implemented) by `DeleteResourcePolicy`
  ("idempotent, matches AWS" in `ops:` above). Fixed all five to no-op on a missing resource
  instead of erroring. Cross-checked the two `Delete*` ops that legitimately keep erroring —
  `DeleteWebhook`/`DeleteSourceCredentials` — both *do* declare `ResourceNotFoundException`
  for real, so they were correctly left unchanged.
- **`ListReportsForReportGroup` had the same missing-existence-check bug** as
  `DescribeTestCases`/`GetReportGroupTrend` above: a nonexistent `reportGroupArn` returned an
  empty list instead of `ResourceNotFoundException` (real AWS declares it for this op,
  confirmed via the same botocore errors list — unlike `ListReports`/`ListReportGroups`, which
  correctly don't and were correctly left alone). Fixed the same way: existence check before
  listing, `ListReportsForReportGroup`'s backend signature gained an `error` return (no
  external call sites outside `services/codebuild`, confirmed by repo-wide grep; `go vet .`
  clean at the repository root).

New/updated tests: `reports_test.go` (`TestCodeBuild_ReportExtras`,
`TestCodeBuild_Reports`, `TestCodeBuild_ReportGroups`), `fleets_test.go`
(`delete_missing_fleet_is_idempotent`), `build_batches_test.go`
(`delete_missing_is_idempotent`), `projects_test.go` (`not_found_is_idempotent`),
`handler_test.go` (`TestHandler_ErrorTypeMapping`'s `delete_project_missing_is_idempotent`/
`delete_fleet_missing_is_idempotent`), `pagination_test.go`/`persistence_test.go` (updated to
create real report groups instead of hand-constructing ARNs that were never registered, which
the new `ListReportsForReportGroup` existence check would otherwise correctly reject).

**2026-08-22 (gopherstack-r80d, batch 34 -- required-OUTPUT-member sweep, wrapped-type-shape
candidate, 0 bugs):** every codebuild op's `<Op>Output` declares zero required members at its
own top level (confirmed via `cmd/requiredoutputfields`), invisible to every ranking this
campaign used through batch 33. Selected as one of the two candidates named by batch 33's
"ops with zero required fields wrapping richly-required domain types" mechanism test (`services/
_REQUIRED_OUTPUT_CANDIDATES.md`'s batch-33 section) and given the full hand audit that batch
left undone.

Walked every non-slice field of every `<Op>Output` one hop into its own type
(`aws-sdk-go-v2/service/codebuild@v1.72.4/types/types.go`); the only wrapped types declaring
>=2 required members of their own are `ProjectEnvironment` (`ComputeType`, `Image`, `Type` --
`Project`/`Build`/`BuildBatch`/`Sandbox` each nest it via a field named `Environment`) and
`ScopeConfiguration` (`Name`, `Scope` -- nested one hop inside `Webhook`). Of those, only
`Image` (`*string`) and `Name` (`*string`) are provable per this campaign's pointer-vs-enum
rule; `ComputeType`/`Type`/`Scope` are non-pointer enums and not provable regardless of
gopherstack's behavior.

All confirmed correctly emitted, no bugs:
- `Environment` on `Build`/`BuildBatch`/`Sandbox` (`*ProjectEnvironment`, `omitempty`) is always
  populated in practice: `StartBuild` (`builds.go`) copies it from the project via
  `applyBuildOverrides`/`env`, never leaves it nil. `ProjectEnvironment.Image` has no
  `omitempty` tag in gopherstack's own wire struct (`models.go`), so even a hypothetically
  empty value would still serialize the key.
- `ScopeConfiguration` on `Webhook` is not itself Smithy-required (only present for GitHub/
  GitHub Enterprise org-scoped webhooks, matching real AWS's documented restriction) and is
  threaded straight through from `CreateWebhookInput.ScopeConfiguration` to the response with
  no fabrication or dropping (`handler_webhooks.go`/`webhooks.go`); `Name`/`Scope` in
  gopherstack's `ScopeConfiguration` struct carry no `omitempty`, so whatever a real client's
  own required-field validator already guaranteed on the way in survives to the response
  unmodified.
- One hop further: `ProjectSourceVersion` (`SourceIdentifier`, `SourceVersion`, both
  `*string` and provable, nested in `Project`/`Build`'s `SecondarySourceVersions` slice) is
  passed through verbatim from the request with no `omitempty` on either field in gopherstack's
  struct -- clean, same reasoning as `ScopeConfiguration`.
- Also checked one hop further into `Fleet`/`ReportGroup`/`CommandExecution`/`Sandbox`'s other
  nested types (`ComputeConfiguration`, `ProxyConfiguration`, `ScalingConfigurationOutput`,
  `FleetStatus`, `LogsLocation`, `SandboxSession`, `LogsConfig`, `VpcConfig`, `ProjectSource`):
  all declare zero required members of their own except `ProjectSource.Type` (`SourceType`, a
  non-pointer enum) -- not provable, not pursued further.

**Wrapped-type-shape hypothesis verdict for codebuild: did not hold.** Every candidate this
mechanism surfaced was either already correctly wired or not provable under this campaign's own
rules. See `services/_REQUIRED_OUTPUT_CANDIDATES.md`'s batch-34 section for the cross-service
verdict (the paired candidate, fsx, did find a bug this way).

### 2026-08-28 pass (gopherstack-6flj): write-only-state sweep, 7 bugs found

An existing `wire_field_fixes_test.go` (1 test, StartBuild inheriting
Cache/VpcConfig/FileSystemLocations) marked this service PARTIAL, not
finished, per this campaign's own established rule. Ran the write-only-state
method first: for every write op (CreateProject/UpdateProject/CreateFleet/
UpdateFleet/CreateWebhook/UpdateWebhook/PutResourcePolicy/StartBuild/
StartSandbox/StartCommandExecution/RetryBuild), field-diffed the real
`aws-sdk-go-v2/service/codebuild@v1.72.4` request/response types directly
against gopherstack's wire structs and backend models (never against
gopherstack's own prior output). All 7 bugs found this way; none via a
key-diff pass alone (every key gopherstack already emitted was correctly
named).

1. **`Project.BadgeEnabled`/`Badge` — accepted from the request, never
   stored at all.** `CreateProjectInput`/`UpdateProjectInput.BadgeEnabled
   *bool` is real (`api_op_CreateProject.go:65`, `api_op_UpdateProject.go:48`;
   `types.ProjectBadge{BadgeEnabled, BadgeRequestUrl}` on the response,
   `deserializers.go:11042`'s `"badge"` case), but gopherstack's
   `projectConfigFields` wire struct had no `badgeEnabled` field at all —
   the value never reached the backend. Fixed: `projectConfigFields`,
   `ProjectConfig.BadgeEnabled *bool`, `InMemoryBackend.applyBadge`
   (`projects.go`) generates a stable synthesized `badgeRequestUrl` the
   first time badging is enabled, matching real AWS not rotating it on
   every subsequent `UpdateProject`.

2. **`ProjectSource` missing `buildStatusConfig`/`gitSubmodulesConfig`
   entirely.** Both are real fields on `types.ProjectSource`
   (`serializers.go:4299,4311`; `deserializers.go:12083,12101`) affecting
   `Source`/`SecondarySources` on `CreateProject`/`UpdateProject` and (by
   inheritance) `Build.Source`. Silently dropped since gopherstack's
   `ProjectSource` model had neither field. Fixed: added `BuildStatusConfig`/
   `GitSubmodulesConfig` types and fields to `ProjectSource` (`models.go`);
   flows through automatically via the existing `Source *ProjectSource`
   request/response wiring, no handler changes needed.

3. **`ProjectEnvironment` missing `computeConfiguration`/`dockerServer`/
   `fleet`/`hostKernel` entirely.** All four are real fields on
   `types.ProjectEnvironment` (`deserializers.go:10075,11705,11719,11729,
   11734`). Most notably `fleet` (`types.ProjectFleet{FleetArn}`) — the
   field that assigns a project to a reserved-capacity compute fleet, the
   exact feature this service's own Fleet API already models end to end —
   was silently discarded on every `Create`/`UpdateProject`. Fixed: added
   `ComputeConfiguration`/`DockerServer`/`DockerServerStatus`/`ProjectFleet`
   types and the four fields to `ProjectEnvironment` (`models.go`).

4. **`StartBuild`'s `sourceVersion` override corrupted `Source.Location`.**
   Real `types.Build` has a `SourceVersion *string` field distinct from both
   `Source.Location` and `ResolvedSourceVersion`
   (`types/types.go`'s `Build` struct) — the requested commit/branch/tag
   to build, not the source URL. Gopherstack's `Build` model had no
   `SourceVersion` field at all, and `applyBuildOverrides` wrote the
   version string directly into `src.Location`, corrupting the project's
   real source URL on every build that set a sourceVersion. Fixed: added
   `Build.SourceVersion`; `StartBuild` now sets it (and a best-effort
   `ResolvedSourceVersion`, mirroring the requested version since this
   emulator does no real git resolution) without touching `Source.Location`.

5. **`StartBuildInput.ArtifactsOverride` — parsed off the wire, then
   silently dropped before reaching the backend.** `handler_builds.go`'s
   `startBuildInput` already declared `ArtifactsOverride *ProjectArtifacts`
   with a correct JSON tag, but `handleStartBuild` never forwarded it into
   `StartBuildConfig` — a textbook accept-then-drop bug. Swept the rest of
   `StartBuildInput` (`api_op_StartBuild.go`) against gopherstack's handler
   and found ~20 more real override fields entirely unmodeled
   (`cacheOverride`, `registryCredentialOverride`, `fleetOverride`,
   `sourceAuthOverride`, `buildStatusConfigOverride`,
   `gitSubmodulesConfigOverride`, `insecureSslOverride`,
   `reportBuildStatusOverride`, `privilegedModeOverride`,
   `gitCloneDepthOverride`, `sourceTypeOverride`, `sourceLocationOverride`,
   `environmentTypeOverride`, `certificateOverride`,
   `imagePullCredentialsTypeOverride`, `hostKernelOverride`,
   `encryptionKeyOverride`, `secondaryArtifactsOverride`,
   `secondarySourcesOverride`, `secondarySourcesVersionOverride`,
   `queuedTimeoutInMinutesOverride`, `autoRetryLimitOverride`). Fixed: all
   now accepted and applied (`builds.go`'s `applySourceOverrides`/
   `applyEnvironmentOverrides`/`applyEnvironmentScalarOverrides`/
   `applyBuildOverrides`, `handler_builds.go`). `idempotencyToken` and
   `logsConfigOverride` are deliberately still not modeled: neither has any
   observable effect through a real read op (this emulator doesn't
   deduplicate submissions, and `Build` has no `logsConfig` field of its
   own — `Build.Logs`, the actual log-delivery-location field, isn't
   populated by this emulator regardless, since no real log delivery is
   simulated).

6. **`Build`/no model support for `AutoRetryConfig` at all.** Real
   `types.Build.AutoRetryConfig *types.AutoRetryConfig{AutoRetryLimit,
   AutoRetryNumber, NextAutoRetry, PreviousAutoRetry}` lets a client detect
   its own retry chain — a documented real use of `RetryBuild`. Gopherstack
   modeled neither the field nor the chain. Fixed: added `AutoRetryConfig`
   to `Build`; `StartBuild` sets `AutoRetryLimit` from the project (or
   `autoRetryLimitOverride`) with `AutoRetryNumber: 0`; `RetryBuild`
   increments `AutoRetryNumber`, sets `PreviousAutoRetry` to the original
   build's ARN, and (mutating the still-live in-store original) sets the
   original's `NextAutoRetry` to the new build's ARN.

7. **`StartSandbox` never inherited any project configuration.** Real
   `types.Sandbox` carries the identical project-derived field set as
   `types.Build` (`environment`/`source`/`vpcConfig`/`serviceRole`/
   `encryptionKey`/`sourceVersion`/`secondarySources`/
   `secondarySourceVersions`/`fileSystemLocations`/`timeoutInMinutes`/
   `queuedTimeoutInMinutes` — confirmed via
   `awsAwsjson11_deserializeDocumentSandbox`), but gopherstack's `Sandbox`
   model only ever had `id`/`arn`/`projectName`/`status`/`startTime`/
   `endTime` — `StartSandbox` created a sandbox with none of a real
   project's configuration attached. Fixed: added the matching fields to
   `Sandbox` and wired `StartSandbox` to copy them from the project, the
   same way `StartBuild` already does for `Build`. `currentSession` and
   `logConfig` deliberately not modeled: `StartSandboxConnection` already
   documents (see its `ops:` row above) that a real interactive terminal
   isn't simulated, and `logConfig` has the identical no-observable-effect
   reasoning as `StartBuildConfig`'s `LogsConfigOverride` above.

Also fixed while sweeping sandbox command execution wire shapes:

8. **`CommandExecution.ExitCode` was `int32`; real wire type is `string`.**
   `deserializers.go:9084`'s `"exitCode"` case requires a JSON string
   (`"expected NonEmptyString to be of type string"`) — a real client's
   decoder would reject a numeric value outright. Never actually triggered
   pre-fix because the field was never populated (zero value + `omitempty`
   omits the key), but a latent hard-decode-error landmine per this
   campaign's failure-signature #2. Fixed: retyped to `string`,
   `StartCommandExecution` now sets it to `"0"` (the emulator always
   completes commands synchronously and successfully).
9. **`CommandExecution`'s stderr field used the wrong wire key.**
   Gopherstack emitted `standardErrorContent`; real AWS's key is
   `standardErrContent` (`deserializers.go:9125`) — a silent drop, a real
   client's `StandardErrContent` was always nil. Fixed: renamed the field
   and its JSON tag (`StandardErrContent`).

**Not reached this pass** (documented, not fabricated as covered):
`ImportSourceCredentials`, `DeleteSourceCredentials`, `ListSourceCredentials`,
`InvalidateProjectCache`, `UpdateProjectVisibility`,
`ListCuratedEnvironmentImages`, `ListSharedProjects`/`ListSharedReportGroups`,
`DescribeCodeCoverages`/`DescribeTestCases`/`GetReportGroupTrend` (re-verified
still correctly empty-content per the 2026-08-11 pass, not re-audited further),
all List* pagination paths (unchanged this pass), `StartSandboxConnection`
(already documented `partial`, unchanged). `enumcheck` (`go run
./cmd/enumcheck`) reports 0 findings for codebuild.

Round-trip tests (`wire_field_fixes_test.go`, all driving the real
`aws-sdk-go-v2/service/codebuild` client against this handler): each of the 9
bugs above has a dedicated `*_RealClient` test; each was hand-verified to
fail against the pre-fix code (via `git stash` of only the fix files, never
the test file) and pass after.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/codebuild/...`).

## 2026-08-29 (pagination-arithmetic sweep, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

Census: `paginateIDs` and `paginateCommandExecutions` (`pagination.go`) both delegate
straight to `pkgs/page.New` after an optional descending-order reversal — an offset
token `pkgs/page` itself clamps to the collection length. No equality-scan cursor
anywhere in this service. Re-checked every `List*` handler for a bypass of the shared
paginator (per this campaign's specific warning that `ListCommandExecutionsForSandbox`
had one, fixed 2026-08-29 in `4cc1b6238`/an adjacent commit on this same branch, already
reflected above as `paginateCommandExecutions`): every remaining `List*` op either calls
`paginateIDs`/`paginateCommandExecutions`, or has no real-AWS pagination fields at all
(`ListSharedProjects`, `ListSharedReportGroups`, `ListSourceCredentials`,
`ListCuratedEnvironmentImages`, all `BatchGet*`). No further bypasses found. Verdict:
correct, no bug found.

Added `pagination_arithmetic_test.go`: a real `aws-sdk-go-v2` typed-client boundary walk
over `ListProjects` (N=7, page implicit default, `assert.ElementsMatch` against the full
set).

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/codebuild/...`).

## 2026-08-31 (value-semantics sweep, gopherstack-uox6): re-derived clean, zero code changed

Dispatched by targeting ("no `filter_default_semantics` covledger row"); `covledger -service codebuild`
in fact credits only `request_field_never_read` (`0c9b33a27`) and `wrong_wire_key` (`e50f52dce`) — a
different labeling of what four prior passes on this branch (`e50f52dce`, `4cc1b6238`, `0c9b33a27`, plus
the original `wire_field_fixes_test.go` sweep) already substantially covered as wire-key, request-field,
pagination-cursor, and first-element-only-list bugs. Read all four commits' full diffs and PARITY notes
before doing new work, per this campaign's "twice already" precedent for a service the ledger calls
unaudited that was in fact already audited.

**The brief's own lead (documented `sortBy`/`sortOrder` defaults) does not hold for codebuild.** Checked
every `List*` operation's own doc comment in the pinned SDK (`api_op_ListBuilds.go`,
`ListBuildsForProject`, `ListBuildBatches(ForProject)`, `ListProjects`, `ListReportGroups`, `ListReports
(ForReportGroup)`, `ListSandboxes(ForProject)`, `ListCommandExecutionsForSandbox`, `ListFleets`,
`ListSharedProjects`, `ListSharedReportGroups`) plus the live AWS API Reference pages for `ListBuilds`,
`ListProjects`, and `ListReportGroups` (3 pages fetched, all three carried the injected "aws
agent-toolkit search-skills" footer, treated as data and ignored) — none document a default sort order or
a default `sortBy` criterion. Correctly recorded as documentation being SILENT, not a bug: gopherstack's
`paginateIDs`/`paginateCommandExecutions` (`pagination.go`) treat omitted `sortOrder` as ascending and
omitted `sortBy` as name-ascending (`ListFleetsSortedBy`/`ListProjectsSortedBy`/`ListReportGroupsSortedBy`
switch defaults), which is a reasonable convention but not something the doc contradicts either way.

**Swept every genuine filter-typed field in the service** (not just sortBy/sortOrder scalars), all
correctly implemented, re-verified from source:
- `ListReportsInput.Filter`/`ListReportsForReportGroupInput.Filter` (`types.ReportFilter{Status}`) —
  `handler_reports.go`'s `reportFilter{Status string}` correctly decodes the nested `{"filter":
  {"status": ...}}` wire shape (not a flat field), compared by exact equality against `Report.Status` in
  `reports.go`'s `ListReports`/`ListReportsForReportGroup`, matching the doc's "You can filter using one
  status only."
- `ListBuildBatchesInput.Filter`/`ListBuildBatchesForProjectInput.Filter` (`types.BuildBatchFilter{Status}`)
  — same nested-object decode, compared against `BuildBatch.BuildBatchStatus`, matching "Only batch builds
  that have this status will be retrieved."
- `ListFleetsInput.SortBy` (`CREATED_TIME|LAST_MODIFIED_TIME|NAME`) — `ListFleetsSortedBy` implements all
  three (NAME is the natural construction order, the other two sort explicitly).
- `DescribeTestCasesInput.Filter` (`types.TestCaseFilter{Keyword,Status}`) and
  `DescribeCodeCoveragesInput.{MinLineCoveragePercentage,MaxLineCoveragePercentage,SortBy,SortOrder}` are
  **provably inert, not merely unimplemented**: grepped the whole package for `TestCase{`/`CodeCoverage{`
  construction sites — the only ones are the two `return []TestCase{}, nil` / `return []CodeCoverage{},
  nil` unconditional-empty returns in `reports.go`. No write path anywhere populates either type (matches
  `items_still_open` above, already correctly recorded as a content gap, re-confirmed rather than
  re-derived from the note alone). No legal filter value can change an always-empty result — same
  reasoning as this campaign's other "provably inert" retirements — so `DescribeTestCasesInput.Filter`
  being undeclared in `describeTestCasesInput` is the never-declared axis, not a fixable value-semantics
  bug, and is correctly left alone.

**One new gap found and recorded (not fixed)**: `ListBuildsForProjectInput.SortOrder`'s own doc states
setting it on a project with more than 100 builds must error; `handleListBuildsForProject` never checks
build count before sorting. Missing rejection — validation axis, kept separate from this pass's
value-semantics remit per the campaign's own discipline; see `gaps:` above.

**Strengthened coverage rather than fixed a bug**: the confirmed first-element-only-list shape doesn't
apply anywhere in codebuild's current filter surface — every real `Filter` type here (`ReportFilter`,
`BuildBatchFilter`, `TestCaseFilter`) carries a single-value `Status`/`Keyword` scalar, not a `Values
[]string` list, so no test addition was needed for that specific blind spot (unlike fsx, same pass,
`services/fsx/PARITY.md`). No code or test changes made to this service this pass.

Gates: `go build ./services/codebuild/...`, `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/codebuild/...`, `golangci-lint run ./services/codebuild/...` (0 issues).

## 2026-09-04 parity sweep: 2 bugs found and fixed (DeleteProject cascade, ListBuildsForProject sort-order limit)

Ran the standard mechanical checks (sentinel round-trip, parsed-then-dropped fields,
lifecycle preconditions, ghost rows after delete, fabricated/unreachable error codes)
across the whole package before pursuing leads. Baseline before any edit: `go build`
clean, `golangci-lint run ./services/codebuild/...` reported 0 issues.

**Bug 1 — DeleteProject cascade-deleted a project's builds; real AWS does not.**
`api_op_DeleteProject.go` (pinned `codebuild@v1.72.4`): "Deletes a build project. When
you delete a project, its builds are not deleted." `projects.go`'s `DeleteProject`
walked the `buildsByProject` index and called `b.builds.Delete(id)` for every build of
the deleted project — the opposite of the documented behavior. This file's own prior
`DeleteProject` PARITY note had (incorrectly) described the cascade as a deliberate
cleanup rather than flagging it, and `janitor_test.go`'s
`TestDeleteProject_CleanupBuilds` asserted the cascade outright — a case of a test
encoding the bug, called out per this campaign's own instructions rather than quietly
rewritten: the test is now `TestDeleteProject_DoesNotCleanupBuilds` and asserts builds
survive project deletion (`BuildCount`/`BuildARNIndexSize`/`BuildsByProjectSize` all stay
at 2, not 0). Fix: `DeleteProject` now only calls `b.projects.Delete(name)`; idempotency
on a nonexistent name is preserved because `store.Table.Delete` is a safe no-op on a
missing key. `DeleteProject` models only `InvalidInputException`
(`botocore codebuild/2016-10-06/service-2.json operations.DeleteProject.errors`) so no
error-path change was needed.
Fail-before: neutered by re-inserting the cascade-delete loop under `if true {}` in
`DeleteProject` (verified by printing the function body post-edit); rerunning
`TestDeleteProject_DoesNotCleanupBuilds` failed with:
`Not equal: expected: 2 actual: 0` on all three assertions (BuildCount,
BuildARNIndexSize, BuildsByProjectSize). Restored from a scratchpad copy and reverified
`0 issues` / all package tests green.

**Bug 2 — ListBuildsForProject didn't enforce its own documented sortOrder limit.**
`api_op_ListBuildsForProject.go`'s `SortOrder` field doc: "If the project has more than
100 builds, setting the sort order will result in an error." (Already correctly
identified as an open gap by the 2026-08-31 gopherstack-uox6 pass but left unfixed.)
`handleListBuildsForProject` (`handler_builds.go`) called `paginateIDs` with whatever
`sortOrder` the client sent regardless of how many builds the project had. Fix: added a
check right after fetching the project's build IDs — `sortOrder != "" && len(ids) >
defaultListPageSize` now returns `%w: sortOrder cannot be set...` wrapping
`ErrValidation` (→ `InvalidInputException`, matching the only error
`ListBuildsForProject` models).
New test `TestHandler_ListBuildsForProject_SortOrderBuildCountLimit`
(`pagination_test.go`) covers three cases: 101 builds + sortOrder → 400; 100 builds +
sortOrder → 200 (boundary); 101 builds + no sortOrder → 200 (unaffected). Fail-before:
neutered the guard's condition to `if false && ...` (verified by printing the line
post-edit and confirming the build still compiled); only the
`over_100_builds_rejects_sort_order` subtest failed, with `Not equal: expected: 400
actual: 200` — the other two subtests correctly still passed, proving the guard's single
condition is exactly what the test exercises. Restored from a scratchpad copy and
reverified `0 issues` / all package tests green.

**Other mechanical checks, no further bugs found**: sentinel round-trip (all three
package sentinels — `ResourceNotFoundException`/`ResourceAlreadyExistsException`/
`InvalidInputException` — are both defined and reachable via `handleError`; no handler
returns any of the other three real CodeBuild error types
(`AccountLimitExceededException`/`OAuthProviderException`/`AccountSuspendedException`),
which is correct since none of those conditions (account-level resource limits, OAuth
provider handshake failures, account suspension) are modeled anywhere in this emulator
and fabricating them would be worse than omitting them. `StatusType`'s `FAILED`/`FAULT`/
`TIMED_OUT` values are declared but structurally unreachable — the janitor's
`advanceInProgressBuilds` only ever advances `IN_PROGRESS` builds to `SUCCEEDED`,
because no build-execution/failure semantics exist in this emulator to source a real
failure from; this is the same category of structural gap as the already-documented
`items_still_open` report-content entries, not a fixable per-op bug, and is left
unfixed here for the same reason. `BuildBatch`'s deeper lifecycle gaps are the
previously-identified unmodelled-capability structural gap (full batch group/environment
lifecycle design), not touched this pass.

Gates: `go build ./services/codebuild/...`, `go test -race -count=1
./services/codebuild/...`, `golangci-lint run ./services/codebuild/...` (0 issues before
and after). Cross-service check: `services/cloudformation/resources_codebuild.go` calls
`InMemoryBackend.DeleteProject` with an unchanged signature — `go test -race -count=1 .
./services/cloudformation/...` passes.
