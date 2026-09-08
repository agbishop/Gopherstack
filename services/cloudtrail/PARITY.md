---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: cloudtrail
sdk_module: aws-sdk-go-v2/service/cloudtrail@v1.58.4   # version audited against
last_audit_commit:                                # unknown: pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-09-06   # gopherstack-g9b4: CreateTrail/UpdateTrail S3 bucket validation + log file delivery
overall: A            # A = ~1k genuine fixes found; B = already-accurate, proven op-by-op
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateTrail: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "fixed prior pass: response KmsKeyId key case (was KMSKeyId). gopherstack-g9b4 (2026-09-06): now validates S3BucketName exists via the wired S3 backend (SetS3Backend), returning S3BucketDoesNotExistException when missing -- see Notes below. Unwired (no SetS3Backend call): unchanged, permissive. gopherstack-f94x (2026-09-06): S3KeyPrefix's documented 200-character bound (types/types.go:912-914) was never enforced; now rejected with InvalidS3PrefixException, per the exception CreateTrail itself declares (types/errors.go:1565)."}
  GetTrail: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateTrail: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "gopherstack-g9b4 (2026-09-06): same S3BucketName existence check as CreateTrail when a new bucket is supplied; a rejected update leaves the trail's existing bucket unchanged. gopherstack-f94x (2026-09-06): same S3KeyPrefix 200-character bound as CreateTrail now enforced."}
  DeleteTrail: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTrails: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTrails: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: NextToken pagination via pkgs/page (was always one page)"}
  StartLogging: {wire: ok, errors: ok, state: ok, persist: ok}
  StopLogging: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTrailStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "IsLogging/StartLoggingTime/StopLoggingTime/LatestDeliveryTime as epoch numbers, TimeLoggingStarted/Stopped as RFC3339 strings — matches SDK deserializer exactly"}
  PutEventSelectors: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-31 (gopherstack-uox6): basic EventSelector.IncludeManagementEvents/ReadWriteType now default correctly (true/All) when omitted -- see Notes below."}
  GetEventSelectors: {wire: ok, errors: ok, state: ok, persist: ok, note: "echoes PutEventSelectors' resolved (not raw) selector values -- see Notes below."}
  PutInsightSelectors: {wire: ok, errors: ok, state: ok, persist: ok}
  GetInsightSelectors: {wire: ok, errors: ok, state: partial, persist: ok, note: "gopherstack-6flj: real GetInsightSelectorsOutput additionally has InsightsDestination (S3 destination ARN for a specific advanced Insights setup this backend does not model). Structural gap, disclosed not fabricated -- see gaps."}
  LookupEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: EventCategory input field now filters (omitted/'Management' -> management events; 'insight' -> none, this backend never synthesizes Insight events); Event gained EventCategory + a matching UnmarshalJSON (see leaks note)"}
  AddTags: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveTags: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTags: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  GetChannel: {wire: ok, errors: ok, state: partial, persist: ok, note: "gopherstack-6flj: real GetChannelOutput additionally has IngestionStatus/SourceConfig (confirmed against cloudtrail@v1.58.4's deserializer); this backend's Channel struct does not model either (no per-channel ingestion tracking or AWS-service-linked source config). Structural gap, disclosed not fabricated -- see gaps."}
  UpdateChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  ListChannels: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: NextToken/MaxResults pagination via pkgs/page"}
  CreateDashboard: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed prior pass: Widgets/RefreshSchedule/TerminationProtectionEnabled were accepted on the wire but never modeled/stored/echoed; now real Dashboard fields. gopherstack-4gzs: CORRECTED -- the shared dashToMap helper leaked Status/CreatedTimestamp/UpdatedTimestamp/LastRefreshId/LastRefreshFailureReason (none exist on the real CreateDashboardOutput) and, the inverse bug, never emitted TagsList (the real output's only tag field -- confirmed via cloudtrail@v1.58.4 deserializers.go's CreateDashboardOutput case switch: DashboardArn/Name/RefreshSchedule/TagsList/TerminationProtectionEnabled/Type/Widgets only). Split into a dedicated dashCreateToMap; TagsList now populated from the dashboard's own tags."}
  GetDashboard: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed prior pass: now returns Widgets/RefreshSchedule/TerminationProtectionEnabled/LastRefreshId/LastRefreshFailureReason/CreatedTimestamp/UpdatedTimestamp. gopherstack-4gzs: CORRECTED -- the shared dashToMap helper leaked Name (GetDashboardOutput has no Name field; confirmed via deserializers.go's case switch: CreatedTimestamp/DashboardArn/LastRefreshFailureReason/LastRefreshId/RefreshSchedule/Status/TerminationProtectionEnabled/Type/UpdatedTimestamp/Widgets only). Split into a dedicated dashGetToMap."}
  UpdateDashboard: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed prior pass: removed a gopherstack-invented Name (rename) parameter -- real UpdateDashboardInput has no Name field, dashboards cannot be renamed. Now takes the real fields: Widgets, RefreshSchedule, TerminationProtectionEnabled. gopherstack-4gzs: CORRECTED -- the shared dashToMap helper leaked Status/LastRefreshId/LastRefreshFailureReason (none exist on the real UpdateDashboardOutput; confirmed via deserializers.go's case switch: CreatedTimestamp/DashboardArn/Name/RefreshSchedule/TerminationProtectionEnabled/Type/UpdatedTimestamp/Widgets only). Split into a dedicated dashUpdateToMap."}
  DeleteDashboard: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDashboards: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: NextToken/MaxResults pagination; narrowed the per-item shape to the real DashboardDetail{DashboardArn,Type} (previously returned the full dashToMap shape, harmless-extra but now exact); added Type/NamePrefix filters"}
  StartDashboardRefresh: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: real StartDashboardRefreshOutput has exactly one field, RefreshId. Previously returned a fabricated {DashboardArn, Status} shape with Status set to \"REFRESHING\", which is not even a valid DashboardStatus enum value (real values: CREATING/CREATED/UPDATING/UPDATED/DELETING)"}
  CreateEventDataStore: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj: this pass's prior 'wire: ok' claim was WRONG. The shared edsToMap helper leaked FederationRoleArn/FederationStatus/InsightSelectors (none exist on the real CreateEventDataStoreOutput; confirmed against cloudtrail@v1.58.4's deserializers.go case switch: AdvancedEventSelectors/BillingMode/CreatedTimestamp/EventDataStoreArn/KmsKeyId/MultiRegionEnabled/Name/OrganizationEnabled/RetentionPeriod/Status/TagsList/TerminationProtectionEnabled/UpdatedTimestamp only) and never emitted the real TagsList (a value the backend already held -- tags are captured and stored on the EventDataStore at creation, just never echoed back). Split into a dedicated edsCreateToMap; TagsList now populated, fabricated fields removed."}
  GetEventDataStore: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: CreatedTimestamp/UpdatedTimestamp were raw time.Time values marshaled by encoding/json as RFC3339 strings; the real awsjson1.1 deserializer requires epoch-seconds JSON numbers (ParseEpochSeconds), so a real SDK client would fail to decode these fields entirely. Now emitted as float64(t.Unix()). gopherstack-6flj: CORRECTED -- the shared edsToMap helper also leaked an InsightSelectors field (does not exist on the real GetEventDataStoreOutput; that field belongs only to Get/PutInsightSelectorsOutput). Split into a dedicated edsGetOrUpdateToMap; InsightSelectors removed."}
  UpdateEventDataStore: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same CreatedTimestamp/UpdatedTimestamp epoch fix as GetEventDataStore. gopherstack-6flj: CORRECTED -- same fabricated InsightSelectors leak as GetEventDataStore, same edsGetOrUpdateToMap fix (real UpdateEventDataStoreOutput's fields are identical to GetEventDataStoreOutput's minus PartitionKeys, which this backend already never emitted -- see gaps)."}
  DeleteEventDataStore: {wire: ok, errors: ok, state: ok, persist: ok, note: "termination-protection conflict correctly returns EventDataStoreTerminationProtectedException"}
  ListEventDataStores: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: NextToken/MaxResults pagination; same CreatedTimestamp/UpdatedTimestamp epoch fix. gopherstack-6flj: per-item shape now uses edsGetOrUpdateToMap (fabricated InsightSelectors removed). Note: the real types.EventDataStore item type marks every field except EventDataStoreArn/Name as 'Deprecated: no longer returned by ListEventDataStores' in the SDK's own doc comments -- AWS's real server has stopped populating them for this op even though the shared struct still supports decoding them. gopherstack's list items are therefore richer than what real AWS currently sends; harmless/informational (a typed client just gets extra populated fields), not the silent-empty class this issue targets, so left as-is rather than artificially trimmed."}
  RestoreEventDataStore: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same CreatedTimestamp/UpdatedTimestamp epoch fix. gopherstack-6flj: CORRECTED -- shared edsToMap also leaked FederationRoleArn/FederationStatus/InsightSelectors (none exist on the real RestoreEventDataStoreOutput; same field list as CreateEventDataStoreOutput minus TagsList). Split into a dedicated edsRestoreToMap."}
  StartEventDataStoreIngestion: {wire: ok, errors: ok, state: ok, persist: ok}
  StopEventDataStoreIngestion: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableFederation: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableFederation: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResourcePolicy: {wire: ok, errors: ok, state: partial, persist: ok, note: "gopherstack-6flj: real GetResourcePolicyOutput additionally has DelegatedAdminResourcePolicy (only populated when queried from an org-member account for a delegated-admin-set policy); consistent with this service's existing, already-documented lack of org-admin state modeling (see RegisterOrganizationDelegatedAdmin/DeregisterOrganizationDelegatedAdmin). Structural gap, disclosed not fabricated -- see gaps."}
  PutResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  StartQuery: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: removed a gopherstack-invented \"EventDataStore\" JSON input field -- the real StartQueryInput has no such field; the target event data store is embedded in QueryStatement's FROM clause (real CloudTrail Lake SQL syntax). The handler now derives it via a FROM-clause regex. Added the real QueryAlias/QueryParameters/DeliveryS3Uri/EventDataStoreOwnerAccountId fields (output now returns QueryId + EventDataStoreOwnerAccountId, was QueryId only). gopherstack-2wvq (2026-08-21): QueryAlias was decoded off the wire but then discarded -- StartQuery's backend signature had no alias parameter at all, so Query.QueryAlias (a field that already existed on the struct) was always empty. Now threaded through and stored, enabling DescribeQuery's QueryAlias lookup (see DescribeQuery). QueryParameters and EventDataStoreOwnerAccountId remain accept-and-drop the same way -- see gaps."}
  CancelQuery: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed error codes: not-found now returns QueryIdNotFoundException (was incorrectly InactiveQueryException, which per the real SDK means \"query already in a terminal state\" -- a completely different condition); cancelling an already-terminal query now correctly returns InactiveQueryException (was InvalidParameterException)"}
  DescribeQuery: {wire: ok, errors: ok, state: fixed, persist: ok, note: "fixed: real DescribeQueryOutput has no top-level CreationTime field at all -- it was returning a fabricated one, and was entirely missing the real (required) nested QueryStatistics object (QueryStatisticsForDescribeQuery: BytesScanned/CreationTime/EventsMatched/EventsScanned/ExecutionTimeInMillis). Also fixed the QueryIdNotFoundException error code (see CancelQuery). gopherstack-2wvq (2026-08-21): DescribeQueryInput marks neither QueryId nor QueryAlias required (cloudtrail@v1.58.4 api_op_DescribeQuery.go:12-16) -- 'You must specify either QueryId or QueryAlias. Specifying the QueryAlias parameter returns information about the last query run for the alias.' The handler previously required QueryId unconditionally, a false negative masking that the backend had no alias lookup at all. Added a queriesByAlias store.Index (grouped by StartQuery call order) and DescribeQueryByAlias, which resolves to the last-inserted match -- the documented 'last query run for the alias' semantic; verified with a multi-query-same-alias test. Response shape unchanged and correct on this newly-reachable path: DescribeQueryOutput has no QueryAlias member at all, so the alias-resolved and QueryId-resolved paths share one response-building path with nothing to diverge. RefreshId (view a dashboard query's results as of a specific refresh) is accepted on the real input but not modeled here -- see gaps."}
  GetQueryResults: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (was the #1 deferred item last pass): QueryResultRows was unconditionally empty. Implemented a bounded, honest CloudTrail Lake SQL subset (SELECT <*|cols> FROM <eds> [WHERE col[!]=val [AND ...]] [LIMIT n]) executed lazily against the backend's shared recorded-events log on first read (see query_exec.go); QueryStatistics.BytesScanned/ResultsCount/TotalResultsCount are real, derived counts, not fabricated. Statements outside the supported grammar still reach FINISHED (never rejected) but yield zero rows -- a narrower, more honest version of the previous blanket limitation. Added NextToken/MaxQueryResults pagination over the computed rows"}
  ListQueries: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: NextToken/MaxResults pagination; EventDataStore/QueryStatus filters now applied (EventDataStore is required on the real input but left permissive here -- see gaps); CreationTime epoch-seconds fix (was raw time.Time)"}
  GenerateQuery: {wire: ok, errors: ok, state: ok, persist: n/a}
  StartImport: {wire: ok, errors: ok, state: partial, persist: ok, note: "fixed (was a gap last pass): ImportSource.S3 now models all three real (all-required) S3ImportSource fields -- S3LocationUri, S3BucketRegion, S3BucketAccessRoleArn -- not just S3LocationUri; all three are stored and echoed back on Start/Get/Stop via a new ImportSource/S3ImportSource backend type. Import execution itself (actual file replay) remains not real -- unchanged, documented limitation. gopherstack-6flj: real StartImportInput also has optional StartEventTime/EndEventTime (a time-range filter on which events to import); the handler discards both (no field to receive them at all). Consistent with the pre-existing 'import execution not real' limitation -- disclosed, not fixed, since honoring a time filter over data that is never actually replayed would be misleading. See gaps."}
  GetImport: {wire: ok, errors: ok, state: partial, persist: ok, note: "same ImportSource fix as StartImport. gopherstack-6flj: real GetImportOutput additionally has StartEventTime/EndEventTime/ImportStatistics, none of which this backend's Import struct models -- same 'import execution not real' root cause as the discarded StartEventTime/EndEventTime inputs. Structural gap, disclosed not fabricated -- see gaps."}
  ListImports: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: NextToken/MaxResults pagination; Destination/ImportStatus filters added"}
  StopImport: {wire: ok, errors: ok, state: ok, persist: ok, note: "same ImportSource fix as StartImport"}
  ListImportFailures: {wire: ok, errors: ok, state: partial, persist: n/a, note: "always empty — consistent since imports never actually execute/fail in this backend"}
  GetEventConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEventConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  RegisterOrganizationDelegatedAdmin: {wire: ok, errors: ok, state: partial, persist: n/a, note: "re-verified this pass: MemberAccountId field name matches the real (only) input field exactly; accepts/validates only, no org-admin state modeled — genuinely acceptable, since the real CloudTrail API itself has no read-back op for delegated admins either (no GetOrganizationDelegatedAdmins-equivalent exists upstream)"}
  DeregisterOrganizationDelegatedAdmin: {wire: ok, errors: ok, state: partial, persist: n/a, note: "re-verified this pass: DelegatedAdminAccountId field name matches the real (only) input field exactly; same as RegisterOrganizationDelegatedAdmin"}
  SearchSampleQueries: {wire: ok, errors: ok, state: partial, persist: n/a, note: "always empty list; SDK output shape has no other required fields"}
  ListPublicKeys: {wire: ok, errors: ok, state: partial, persist: n/a, note: "always empty; legacy CloudTrail log-file-validation feature, no public keys are ever generated by this backend"}
  ListInsightsData: {wire: fixed, errors: ok, state: partial, persist: n/a, note: "gopherstack-6flj: this pass's prior 'wire: ok' claim was WRONG -- the response was wrapped under a fabricated 'Insights' key; the real ListInsightsDataOutput wraps its list under 'Events' (confirmed against cloudtrail@v1.58.4's awsAwsjson11_deserializeOpDocumentListInsightsDataOutput). Silently dropped by any real JSON-RPC 1.1 client (case-sensitive protocol). Fixed; also added required-field validation for DataType/InsightSource (previously the whole request body was ignored). List itself is still always empty -- no Insights event generation exists."}
  ListInsightsMetricData: {wire: fixed, errors: ok, state: partial, persist: n/a, note: "gopherstack-6flj: this pass's prior 'wire: ok' claim was WRONG -- the real ListInsightsMetricDataOutput is a flat time series (ErrorCode/EventName/EventSource/InsightType/NextToken/Timestamps/TrailARN/Values), not a '{Values: [...]}' wrapped list of records (confirmed against cloudtrail@v1.58.4's awsAwsjson11_deserializeOpDocumentListInsightsMetricDataOutput). Fixed: now echoes EventName/EventSource/InsightType (all required, validated) plus optional ErrorCode/TrailARN (TrailName resolved to TrailARN via the existing trail lookup), and returns Timestamps/Values as the real flat arrays. Data itself is still always empty -- no Insights metric computation exists."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "ListQueries' EventDataStore filter is real AWS's required field but left optional/permissive here (an empty filter returns every query) for backward wire compatibility with an existing smoke test that calls ListQueries with no arguments; a real client omitting it would get a client-side validation error before the request is even sent, so this is low-risk."
  - "GetQueryResults' SQL execution only understands a bounded grammar (SELECT <*|cols> FROM <eds> [WHERE col[!]=val [AND ...]] [LIMIT n]); joins, aggregates (COUNT/GROUP BY), OR, LIKE, and subqueries are accepted (the query still reaches FINISHED, never rejected) but always yield zero rows. See query_exec.go's file doc comment."
  - "RegisterOrganizationDelegatedAdmin / DeregisterOrganizationDelegatedAdmin validate input but track no org-admin state (no GetOrganizationDelegatedAdmins-equivalent op exists in gopherstack's CloudTrail service to read it back anyway, and none exists in the real upstream API either)."
  - "PARITY-FOLLOWUP (pkgs/service, out of scope for this service): pkgs/service/cloudtrail_capture.go's wrapCloudTrailCapture records a management event unconditionally after next(c) returns, regardless of the wrapped handler's response status — a failed (4xx/5xx) mutating API call is captured identically to a successful one, and the synthesized CloudTrailEvent detail JSON always sets errorCode/errorMessage-equivalent fields absent (no error info at all). Real CloudTrail records failed calls too, but with populated errorCode/errorMessage. Not broken (chokepoint IS wired correctly end-to-end: RecordManagementEvent -> InMemoryBackend.RecordManagementEvent -> LookupEvents returns real captured events), just an accuracy gap in a shared file outside services/cloudtrail/'s edit scope."
  - "gopherstack-6flj: GetChannel's real output has IngestionStatus/SourceConfig, which this backend's Channel struct does not model (no per-channel ingestion tracking or AWS-service-linked source config)."
  - "gopherstack-6flj: GetEventDataStore's real output has PartitionKeys, which this backend does not model at all (no field on EventDataStore, no CreateEventDataStore input to source it from)."
  - "gopherstack-6flj: GetInsightSelectors' real output has InsightsDestination (an S3 ARN for a specific advanced Insights setup), which this backend does not model."
  - "gopherstack-6flj: GetResourcePolicy's real output has DelegatedAdminResourcePolicy, unreachable without org-admin state modeling (same root cause as RegisterOrganizationDelegatedAdmin's gap above)."
  - "gopherstack-2wvq (2026-08-21): StartQuery's QueryAlias is now threaded through and stored (see StartQuery/DescribeQuery), but two sibling input fields are still decoded off the wire and then dropped the same way: QueryParameters (Query already has a matching struct field, never populated) and EventDataStoreOwnerAccountId (only echoed back in StartQuery's own response, never stored on the created Query, so a later DescribeQuery/GetQueryResults on that query reports no owner even though one was supplied at creation time). Both found while checking StartQuery's other members for the same accept-and-drop class; left unfixed as separate from this issue's QueryAlias scope."
  - "gopherstack-2wvq (2026-08-21): DescribeQuery's real input also accepts RefreshId (disambiguates a QueryAlias lookup to one dashboard refresh's query run) -- not modeled, since gopherstack's StartDashboardRefresh does not create linked Query records to disambiguate by (dashboards.go). DescribeQueryByAlias always resolves to the most recently started query for the alias regardless of RefreshId, matching the documented default ('last query run for the alias') but not the RefreshId-scoped refinement."
  - "gopherstack-6flj: StartImport's real input has optional StartEventTime/EndEventTime (a time-range filter on which events to import) and GetImport's real output has matching StartEventTime/EndEventTime/ImportStatistics — none of it modeled, consistent with the pre-existing 'import execution not real' limitation (import file replay was already a documented gap before this pass)."
  - "gopherstack-h6a (2026-09-04): FIXED by gopherstack-g9b4 (2026-09-06) -- see ops rows above and the numbered entry below. Both halves of the originally-recorded gap (bucket validation, log file delivery) are now real when S3 is wired via SetS3Backend; unwired stays permissive as before."
  - "gopherstack-g9b4 (2026-09-06): log file delivery writes one gzipped file per recorded management event, rather than AWS's real ~5-minute batched delivery. A disclosed simplification, not a fabricated batching model -- see the numbered entry below."
deferred: []              # both prior deferred items (Lake SQL execution, Dashboard Widgets) were implemented this pass — see ops/gaps above
leaks: {status: fixed, note: "no goroutines/janitors in this service; Reset() closes every tags.Tags (trails/channels/dashboards/eventDataStores) before clearing tables. Fixed this pass: Event had a hand-written MarshalJSON (epoch-seconds EventTime) but no matching UnmarshalJSON, so any Snapshot containing a recorded event failed Restore entirely (100% data loss of the events log on every restart with in-flight events) -- this was a previously-documented-but-unfixed bug (TestInMemoryBackend_SnapshotRestore_EventsPreexistingBug), now fixed with a real UnmarshalJSON and the test repurposed to assert the round trip succeeds (TestInMemoryBackend_SnapshotRestore_EventsRoundTrip). GetQueryResults/DescribeQuery now mutate on read (materializeQueryLocked lazily executes a QUEUED query) -- both switched from RLock to Lock accordingly, no lock-upgrade race since the mutation happens entirely under the single write lock, not via RLock->Lock promotion. gopherstack-h6a (2026-09-04): DeleteTrail never purged b.eventConfigs/b.resourcePolicies (both keyed by the deleted resource's ARN, separate from the store.Table-managed trails/channels/eventDataStores tables and their auto-maintained indexes). A trail's ARN is deterministic from its user-chosen name (arn.Build('trail/'+name)), so DeleteTrail followed by CreateTrail with the same name silently resurrected the previous trail's GetEventConfiguration/GetResourcePolicy state -- the exact reused-identity ghost-row class this campaign flags as severe. Regression tests (both proven to fail against unmodified code first): TestCloudTrailEventConfiguration/recreated_trail_does_not_inherit_deleted_trails_config (handler_event_selectors_test.go) and TestCloudTrailResourcePolicy/recreated_trail_does_not_inherit_deleted_trails_resource_policy (handler_resource_policies_test.go). Fixed by purging both maps in DeleteTrail; DeleteEventDataStore and DeleteChannel got the same two-line cleanup for the matching (lower-severity, since eds-/channel- IDs are counter-generated and never reused) unbounded-growth leak."}
---

## Notes

Protocol: awsjson1.1 (single POST endpoint, `X-Amz-Target: CloudTrail_20131101.<Op>` —
verified against the real SDK's `httpBindingEncoder.SetHeader("X-Amz-Target").String(...)`
call sites in serializers.go; the service's `cloudtrailTargetPrefix` constant matches
exactly).

### Real bugs found and fixed this pass (field-diffed against aws-sdk-go-v2/service/cloudtrail@v1.55.7)

1. **`GetQueryResults` always returned empty `QueryResultRows`** (the #1 deferred item from
   the prior pass: "CloudTrail Lake SQL query execution... QueryResultRows is always empty
   by design"). Implemented a bounded, honest SQL subset executor (`query_exec.go`):
   `SELECT <* | col[, col...]> FROM <event-data-store> [WHERE <col>[!]=<'value'|value>
   [AND ...]] [LIMIT n]`, executed lazily (on first `GetQueryResults`/`DescribeQuery` read,
   not eagerly at `StartQuery`, so a query cancelled before being read stays cancellable —
   matching AWS's async `QUEUED`->`RUNNING`->`FINISHED` lifecycle) against the backend's
   shared recorded-events log. `QueryStatistics` (`BytesScanned`/`ResultsCount`/
   `TotalResultsCount`, and `DescribeQuery`'s `EventsMatched`/`EventsScanned`/
   `ExecutionTimeInMillis`) are genuine derived counts, not fabricated. Statements outside
   the supported grammar are never rejected (still reach `FINISHED`) but yield zero rows —
   see gaps.

2. **`StartQuery` accepted a gopherstack-invented `"EventDataStore"` JSON field.** The real
   `StartQueryInput` has exactly five fields (`DeliveryS3Uri`, `EventDataStoreOwnerAccountId`,
   `QueryAlias`, `QueryParameters`, `QueryStatement`) — no `EventDataStore` field exists; the
   target event data store is embedded directly in `QueryStatement`'s `FROM` clause, per real
   CloudTrail Lake SQL syntax. Removed the invented field; the handler now derives the target
   via `extractQueryFromTarget` (a `FROM`-clause regex) instead. Also added the real
   `QueryAlias`/`QueryParameters`/`EventDataStoreOwnerAccountId` fields (output was missing
   `EventDataStoreOwnerAccountId` entirely).

3. **`CancelQuery`/`DescribeQuery`/`GetQueryResults` returned the wrong not-found error
   code.** `ErrQueryNotFound` was mapped to `InactiveQueryException`, but that real SDK
   exception means "the specified query cannot be canceled because it is in the FINISHED,
   FAILED, TIMED_OUT, or CANCELLED state" — an entirely different condition from "no such
   query ID". The real not-found code is `QueryIdNotFoundException`. Split into
   `ErrQueryIDNotFound` (404, all three ops) and `ErrQueryInactive` (400, `CancelQuery`'s
   already-terminal-state case, which previously used the equally-wrong
   `InvalidParameterException`).

4. **`DescribeQuery` had a fabricated top-level `CreationTime` field and was missing the
   real, required, nested `QueryStatistics` object entirely.** Real `DescribeQueryOutput`
   has no top-level `CreationTime` — it's nested inside `QueryStatistics`
   (`QueryStatisticsForDescribeQuery`: `BytesScanned`/`CreationTime`/`EventsMatched`/
   `EventsScanned`/`ExecutionTimeInMillis`), none of which were previously populated. Now
   returns the real nested shape with genuine values from query execution.

5. **`UpdateDashboard` accepted a gopherstack-invented `Name` (rename) parameter.** Real
   `UpdateDashboardInput` has exactly `DashboardId`/`RefreshSchedule`/
   `TerminationProtectionEnabled`/`Widgets` — dashboards cannot be renamed via this or any
   other CloudTrail API. Removed the rename capability; added the three real fields, none of
   which were previously modeled at all (also missing from `CreateDashboard`/`GetDashboard`,
   the #2 deferred item from the prior pass: "Dashboard Widgets modeling"). `Dashboard`
   gained `Widgets`, `RefreshSchedule`, `TerminationProtectionEnabled`, `CreatedTimestamp`/
   `UpdatedTimestamp`, and `LastRefreshId`/`LastRefreshFailureReason`.

6. **`StartDashboardRefresh` returned a fabricated response shape.** Real
   `StartDashboardRefreshOutput` has exactly one field, `RefreshId`. The handler instead
   returned `{DashboardArn, Status}`, with `Status` hardcoded to `"REFRESHING"` — not even a
   valid `DashboardStatus` enum value (real values: `CREATING`/`CREATED`/`UPDATING`/
   `UPDATED`/`DELETING`). Fixed to return only `RefreshId`, generated fresh per call and
   stored on the dashboard as `LastRefreshId`.

7. **`EventDataStore` timestamp epoch-seconds bug** (the flagged bug class: raw `time.Time`
   marshaled where the awsjson1.1 deserializer requires an epoch-seconds JSON number).
   `edsToMap`'s `CreatedTimestamp`/`UpdatedTimestamp` were placed directly as `time.Time`
   values, which `encoding/json` renders as RFC3339 strings; the real deserializer calls
   `smithytime.ParseEpochSeconds(f64)` on these fields (confirmed in `deserializers.go`), so
   a real SDK client would fail to decode them (or, since these aren't pointer fields on the
   Go SDK's `*time.Time`, would just always see `nil`). Affects
   `CreateEventDataStore`/`GetEventDataStore`/`UpdateEventDataStore`/
   `RestoreEventDataStore`/`ListEventDataStores` (all share `edsToMap`) and `ListQueries`'
   `CreationTime` (a separate, similar bug in `handleListQueries`). Fixed by emitting
   `float64(t.Unix())` at the map-building call sites, the same pattern already used
   correctly by `trailToMap`/`GetTrailStatus`/the import handlers/`DescribeQuery`.

8. **`Event.MarshalJSON` had no matching `UnmarshalJSON`** — a pre-existing, previously
   *documented-but-deliberately-unfixed* bug (`TestInMemoryBackend_SnapshotRestore_
   EventsPreexistingBug`, added during an earlier store.Table migration pass with a comment
   explicitly deferring the fix). `Event.EventTime` is hand-encoded as an epoch-seconds JSON
   number for the LookupEvents wire response, but with no matching decoder, any `Snapshot`
   containing so much as one recorded event failed `Restore` outright (`b.events` — the one
   raw, non-`store.Table` field — round-trips through the exact same `Event.MarshalJSON`).
   Added the inverse `UnmarshalJSON` and repurposed the test
   (`TestInMemoryBackend_SnapshotRestore_EventsRoundTrip`) to assert the round trip now
   succeeds losslessly, including the new `EventCategory` field.

9. **`StartImport`'s `ImportSource.S3` only modeled `S3LocationUri`** (a gap from the prior
   pass). Real `S3ImportSource` has three fields, *all* marked `required` in the SDK docs:
   `S3LocationUri`, `S3BucketRegion`, `S3BucketAccessRoleArn`. The latter two were accepted
   on the wire and silently discarded. Restructured `Import.ImportSource` from a bare
   `string` into a real `*ImportSource{S3: *S3ImportSource{...}}` type, storing and echoing
   all three fields on `StartImport`/`GetImport`/`StopImport`.

10. **`LookupEvents` ignored the `EventCategory` input field** (a gap from the prior pass).
    Real semantics: omitting `EventCategory` (or passing anything but `"insight"`, its only
    enum value) returns Management events only; passing `"insight"` returns Insight events
    only. Added an `EventCategory` field to `Event` (defaulted to `"Management"` in
    `RecordEvent`, since this backend only ever records management-plane API calls) and wired
    the filter into `eventMatchesFilters`.

### Deliberately NOT flagged (false-positive checks per parity-principles.md rule 4)

- `ListImportFailures`/`SearchSampleQueries`/`ListPublicKeys`/`ListInsightsData`/
  `ListInsightsMetricData` all return empty lists. Each was re-checked against its real SDK
  output shape: the *shape* is correct, and the emptiness reflects a genuinely unimplemented
  downstream capability (import file replay, Insights event synthesis) rather than a
  populated-but-never-returned map — documented simplifications, not disguised no-ops.
- gopherstack-4gzs: CORRECTED — this entry previously argued the single shared `dashToMap`
  helper's extra `Status`/`Name` keys (present on some but not all of Create/Get/Update
  Dashboard's real output structs) were inert, since AWS JSON-protocol deserializers ignore
  unknown response keys. The premise is true but the conclusion was wrong: `CreateDashboardOutput`
  and `UpdateDashboardOutput` have no `Status` field, `GetDashboardOutput` has no `Name` field
  (cloudtrail@v1.58.4, each op's own `case "...":` switch in `deserializers.go`) — a raw-body or
  non-SDK caller sees the leak regardless of SDK-client tolerance. It is a shared-helper bug, not
  a same-type copy: `dashToMap` is now three dedicated converters, `dashCreateToMap`/
  `dashGetToMap`/`dashUpdateToMap` (see ops rows above), each emitting exactly its op's real
  field set — including the inverse gap this surfaced: `CreateDashboardOutput` genuinely has a
  `TagsList` field that the old helper never populated at all. `DeleteDashboard` (empty
  `map[string]any{}`, matching the real empty `DeleteDashboardOutput`) and `ListDashboards`
  (`dashDetailToMap`, already its own dedicated converter) never used `dashToMap` and needed no
  change. `edsToMap`/`importToMap` are separate shared helpers (event data stores / imports) —
  not re-verified this pass, left as previously assessed.

**2026-08-28 (gopherstack-6flj/21my re-audit)**: this service was tasked as "unswept" for
the wrapper-key/per-item bug class, but git history and this manifest's own prior entries
show it was already thoroughly swept (last_audit_date 2026-08-15, gopherstack-6flj, commit
d4e234022). Independently re-verified a representative sample at both layers against
cloudtrail@v1.58.4's own deserializers rather than trusting the manifest: `Trail`
(trailToMap, all 13 fields incl. SnsTopicARN capitalization), `Channel`/`Destination`
(GetChannel/ListChannels/CreateChannel/UpdateChannel), `Widget`
(QueryAlias/QueryParameters/QueryStatement/ViewProperties), and `AdvancedEventSelector`/
`AdvancedFieldSelector` (all 7 nested field names). All matched the real deserializer's case
labels exactly; no new bugs found. Op-routing-table-vs-manifest diff: 60/60 ops match, no
unaudited op. No changes made to this file's `ops:`/`gaps:` this pass.

**2026-08-31 (gopherstack-uox6, value-semantics sweep):** swept the pinned SDK
(`aws-sdk-go-v2/service/cloudtrail@v1.58.4`) for omission-default language, line-wrap
tolerant. One real bug, fixed with a regression test proven to fail against the
unmodified code first (plus a companion test proving the fix doesn't overwrite an
explicit `false`, which already passed unmodified and still passes now):

- **Basic `EventSelector`'s two documented defaults were lost at decode.**
  `types.EventSelector.IncludeManagementEvents` doc: "By default, the value is
  true." — and the real SDK field is `*bool`, i.e. the wire genuinely distinguishes
  omitted from explicit `false`. `types.EventSelector.ReadWriteType` doc: "By
  default, the value is All." gopherstack's `EventSelector` (`models.go`) used a
  plain `bool`/`string` as the *decode* target too, so an omitted
  `IncludeManagementEvents` silently became the Go zero value `false` (inverting the
  documented default) and an omitted `ReadWriteType` became `""` instead of `"All"`
  — then that wrong value was stored and echoed back verbatim by
  `GetEventSelectors`. Fixed by introducing a wire-only decode type
  `eventSelectorWire` (`handler_event_selectors.go`) with `IncludeManagementEvents
  *bool`, matching the real SDK's type, converted to the internal `EventSelector`
  via `toEventSelector()`, which applies both defaults only when the wire value is
  absent (nil pointer / empty string — `""` is not itself a valid `ReadWriteType`,
  so treating it as "omitted" is safe). `PutEventSelectors`'s response and
  `GetEventSelectors` both now echo the resolved values, not the raw request.
  Regression tests in `omission_defaults_test.go`:
  `TestPutEventSelectors_Defaults` (omits both fields, asserts `true`/`"All"` on
  both the `PutEventSelectors` response and a follow-up `GetEventSelectors` —
  failed against unmodified code with `false`/`""`) and
  `TestPutEventSelectors_ExplicitFalseSurvives` (explicit `false`/`"ReadOnly"`
  survive unchanged — already passed against unmodified code, confirming the
  fix must not simply force `true`).

**Checked and confirmed correct, not fixed:** `LookupEvents.MaxResults` (default
50, cap 50, matching "The default number of results returned is 50, with a maximum
of 50 possible" — `events.go`'s `LookupEvents`) and `LookupEvents.EventCategory`
(omitted category correctly excludes Insight events per "if you do not specify an
event category, events of [the Insight] category are not returned" — already
correctly implemented and commented at `events.go:86-94`, predating this pass).

**Gap recorded, not fixed (documentation silent):** `CreateTrail`/`UpdateTrail`'s
`IncludeGlobalServiceEvents` (`*bool` on the real SDK type, same shape as the bug
above) has **no** "by default" wording anywhere in the pinned SDK's doc comments for
`api_op_CreateTrail.go`, `api_op_UpdateTrail.go`, or `types/types.go` — unlike
`IsMultiRegionTrail` on the same structs, which explicitly states "The default is
false." AWS's public web documentation is known to state a default for this field
elsewhere, but per this campaign's discipline that source is not the pinned SDK and
was not used to fabricate a fix; gopherstack's current `IncludeGlobalServiceEvents
bool` (plain, zero-value `false`) is left as-is. Follow-up should re-check this
field's Go doc comment on a future SDK bump before implementing a default.

**Recorded as the other axis, not fixed here:** `DescribeTrails.IncludeShadowTrails`
is decoded (`handler_trails.go`'s `describeTrailsBody`) but never passed to
`Backend.DescribeTrails`, which takes only a name list. Not fixed in this pass
because this backend has no "shadow trail" (cross-Region/organization-member
replica) concept at all in its data model — `IsOrganizationTrail` is stored but
nothing ever creates a replica row keyed off it, so no trail currently in this
store could ever be excluded or included differently by this flag. A real fix
requires modeling shadow-trail replication, which is a structural feature, not a
value-semantics bug in this parameter's handling.

**2026-09-06 (gopherstack-g9b4): CreateTrail/UpdateTrail bucket validation +
log file delivery, both previously recorded as a structural gap
(gopherstack-h6a) and now fixed.**

- **Bucket validation (SDK-verified).** Real `CreateTrail`'s error switch
  declares `S3BucketDoesNotExistException`
  (`awsAwsjson11_deserializeOpErrorCreateTrail`, cloudtrail@v1.58.4
  `deserializers.go`) and so does `UpdateTrail`'s
  (`awsAwsjson11_deserializeOpErrorUpdateTrail`, same file). The type's own
  doc comment (`types/errors.go:2247`): "This exception is thrown when the
  specified S3 bucket does not exist." CreateTrail/UpdateTrail now call a new
  `S3Backend.HeadBucket` hook (wired via `SetS3Backend`, `cli.go`'s
  `wireCloudTrailS3`) and return `ErrS3BucketNotFound` when it errors. HTTP
  status (400) is an inference by analogy to the sibling `InvalidS3BucketNameException`
  on the same op (also 400) — the pinned SDK's client-side deserializer code
  does not itself carry a status code (that's server behavior, not part of
  the client model), so this is a reasoned choice, not a wire-verified fact.
  Unwired (no `SetS3Backend` call): both ops stay exactly as permissive as
  before -- see `TestCreateTrail_UnwiredS3StaysPermissive`/
  `TestUpdateTrail_UnwiredS3StaysPermissive`.

- **Log file delivery (documentation-sourced, not SDK-verified).** CloudTrail
  log files are an AWS Developer Guide artifact with no representation in the
  pinned SDK at all (no client ever parses one). A logging trail
  (`StartLogging`) with S3 wired now receives one gzipped JSON file per
  recorded management event (`RecordEvent` -> `deliverLogFileLocked`,
  `delivery.go`), body `{"Records": [<event detail>]}` (the same
  per-event detail JSON already built for `CloudTrailEvent` /
  `management_event.go`), key
  `AWSLogs/{account}/CloudTrail/{region}/{yyyy}/{mm}/{dd}/{account}_CloudTrail_{region}_{yyyyMMddTHHmmZ}_{unique}.json.gz`
  (`S3KeyPrefix`-prefixed when set). Two disclosed simplifications versus
  real AWS: (1) one file per event, not AWS's real ~5-minute batched
  delivery; (2) only events carrying a `CloudTrailEvent` detail (i.e. routed
  through `RecordManagementEvent`, the real activity path -- see
  `pkgs/service/cloudtrail_capture.go`) are delivered; a directly seeded
  `RecordEvent` call with no detail delivers nothing, since there is no
  genuine record content to write. Unwired (no `SetS3Backend` call), or a
  trail that never called `StartLogging`: no delivery, matching this
  backend's pre-existing behavior exactly -- see
  `TestRecordManagementEvent_UnwiredS3StaysPermissive`/
  `TestRecordManagementEvent_TrailNotLogging_NoDelivery`.

- **Assessment of the two original claims, evaluated separately per this
  campaign's discipline:** (a) "no log file is ever delivered" had a real
  prerequisite question -- was there anything to deliver? Yes:
  `RecordManagementEvent` (wired globally via `pkgs/service/cloudtrail_capture.go`,
  landed in an earlier pass) means `b.events` is populated by real
  cross-service mutating API activity, not just test seeding. So this was a
  genuine, fixable gap, not structural. (b) "the bucket is never validated"
  was independently fixable and cheaper, per above.

Gates: `go build ./services/cloudtrail/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/cloudtrail/...` (all pass), `golangci-lint run
./services/cloudtrail/...` (0 issues, after a `fieldalignment -fix` pass on the new
`eventSelectorWire` struct, re-verified with a plain `golangci-lint run`
afterward). No other service's files touched.

**2026-09-06 (gopherstack-g9b4) gates:** `go build ./...` (repo-wide, clean),
`go test -race -count=1 ./services/cloudtrail/...` (all pass, including the
new `s3_delivery_test.go`), `go test -race -count=1 .` (root package, clean),
`golangci-lint run ./ services/cloudtrail/...` (0 issues, after a `golines`
pass on the new test file and a `govet` shadow fix in `delivery.go`). Files
touched: `services/cloudtrail/{interfaces.go (new), delivery.go (new),
s3_delivery_test.go (new), store.go, trails.go, events.go, errors.go,
handler.go, PARITY.md}` and `cli.go` (root). No other service's files
touched.
