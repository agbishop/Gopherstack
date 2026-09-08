---
service: lambda
sdk_module: aws-sdk-go-v2/service/lambda@v1.107.0
last_audit_commit: a007ec3e
last_audit_date: 2026-09-03
overall: A   # durable_execution wire-shape rewrite closed the last open gap; all gates green
protocol: REST-JSON
families:
  resource_policy: {status: ok, note: "PROVEN — RemovePermission StatementId from URI path, Qualifier scoping, EventSourceToken/PrincipalOrgID. This sweep closed the AddPermission deferred item: FunctionUrlAuthType/InvokedViaFunctionUrl are now accepted and rendered as IAM Condition entries (StringEquals lambda:FunctionUrlAuthType, Bool lambda:InvokedViaFunctionUrl — verified against real AWS docs/terraform-provider-aws issue #44829), and RevisionId optimistic concurrency is enforced on AddPermission/RemovePermission/GetPolicy (was hardcoded RevisionId:\"1\" — now a real content-hash of the statement-ID set, changing on every mutation, stable otherwise). Same RevisionId + duplicate-StatementId (ResourceConflictException) treatment extended to AddLayerVersionPermission/RemoveLayerVersionPermission/GetLayerVersionPolicy (layers.go), which had the identical hardcoded-\"1\" bug and silently overwrote a duplicate StatementId instead of rejecting it."}
  event_source_mappings: {status: ok, note: unchanged since c3b5d46a; ARN parsing, pollers PROVEN — backoff, FilterCriteria, BisectBatchOnFunctionError, ReportBatchItemFailures, MaxRecordAge. Storage backing (b.eventSourceMappings) converted map->store.Table (ce30166a); re-verified — CreateEventSourceMapping/Get/List/Delete/Update and janitor.sweepESMs all correctly ported}
  datalayer_refactor: {status: ok, note: "ce30166a converted functions/functionURLConfigs/eventSourceMappings/aliases/permissions/codeSigningConfigs/capacityProviders/provisionedConcurrencies from raw maps to pkgs/store Table/Index (store_setup.go, new file). Re-verified every call site in backend.go, janitor.go, async_destinations.go, export_test.go: key derivation (functionURLConfigsKeyFn/aliasKeyFn/permissionKeyFn/provisionedConcurrencyKeyFn all pure + stable), index-returned-slice aliasing (ListAliases/GetPolicy copy into a fresh slice before returning, never leak the Index-owned backing slice), delete cascades (deleteAliasesForFunctionLocked/deletePermissionsForFunctionLocked/deleteProvisionedConcurrenciesForFunctionLocked). No behavior change found — mechanical, correct conversion. codeSigningConfigs/capacityProviders/provisionedConcurrencies correctly kept on b.ephemeralRegistry (not b.registry) preserving their pre-refactor not-persisted status; permissions correctly kept off both registries with a DTO round-trip (permissionSnapshot) since FunctionName/Qualifier are json:\"-\" on the live struct"}
  persistence:      {status: ok, note: "ce30166a added lambdaSnapshotVersion=1 gate (mirrors sqs/ec2 pilot) — an incompatible/absent Version discards to empty rather than partially decoding. Same known systemic trait as sqs/ec2: on a version-mismatch Restore, only b.registry + b.permissions are reset; raw non-Table fields (versions/layers/eventInvokeConfigs/layerPolicies/functionConcurrencies/accountID/region) are left as-is. Not a lambda-specific regression — identical to services/sqs and services/ec2's Restore; Restore only ever runs once against a freshly-constructed backend in practice. Not flagging as a new bug; tracked here for awareness only. Note: PublishVersion's new RevisionId precondition check deliberately reuses fn.RevisionID (already persisted as part of FunctionConfiguration) rather than adding new persisted state, so this is unaffected."}
  runtime_lifecycle: {status: ok, note: unchanged since c3b5d46a; PROVEN — LRU eviction, async cleanup semaphore, container stop/remove, port release, dir cleanup. Real Docker exec}
  function_crud_versions_aliases_layers_concurrency_urls_tags: {status: ok, note: "Field-diffed this sweep (was 'skimmed, not exhaustively re-verified'). Real bug found + fixed: FunctionEventInvokeConfig.LastModified was a time.Time (ISO8601-string wire shape) but the real deserializer (PutFunctionEventInvokeConfig/GetFunctionEventInvokeConfig 'LastModified' case in deserializers.go) parses a json.Number — unlike FunctionConfiguration.LastModified, which IS an ISO8601 string. Fixed to float64 via pkgs/awstime.Epoch, matching the exact bug class documented in parity-principles.md. Also found + fixed a latent double-write bug in handleUpdateFunctionCode/handleUpdateFunctionConfiguration: applyFunctionCodeUpdate returned h.writeError(...)'s own return value as its error signal, but c.JSON (and so writeError) returns nil on ANY successful write — including a written error response — so the `!= nil` check could never detect a validation failure and would silently fall through to a second, conflicting 200 write. Converted to the bool-return convention (see checkRevisionID's doc comment in handler.go). RevisionId optimistic concurrency (previously only on AddPermission) extended to UpdateFunctionConfiguration/UpdateFunctionCode (checked against fn.RevisionID before mutating), UpdateAlias (against alias.RevisionID), and PublishVersion (new PublishVersionWithRevision atomic backend method — kept the existing 2-arg PublishVersion signature untouched since it has ~20 call sites across tests + a CFN caller; the revision check and the publish happen under one lock acquisition via a shared internal publishVersion(name, description, revisionID) to avoid a check-then-act race). Other families (function URL configs, tags, reserved/provisioned concurrency, code signing) spot-checked against the SDK's Output shapes/timestamp wire formats — no further gaps found; CreateFunctionUrlConfig/GetFunctionUrlConfig's CreationTime/LastModifiedTime and ProvisionedConcurrencyConfig.LastModified are correctly ISO8601 strings (verified against deserializers.go), not epoch numbers. Re-checked this pass (wrapper-key sweep) against the sfn TagResource map/array bug class: lambda's own TagResourceInput/UntagResourceInput/ListTagsOutput all genuinely take Tags as map[string]string (api_op_TagResource.go:44, serializers.go:6822-6834) -- unlike sfn, a map here is correct and needed no change; confirmed via a real-client round-trip test (tag_resource_sdk_test.go)."}
  durable_execution: {status: ok, note: "CLOSED (was gap) — dedicated rewrite of durable_execution.go/handler_durable_execution.go, field-diffed against api_op_GetDurableExecution.go, api_op_GetDurableExecutionHistory.go, api_op_GetDurableExecutionState.go, api_op_ListDurableExecutionsByFunction.go, api_op_StopDurableExecution.go, api_op_CheckpointDurableExecution.go, api_op_SendDurableExecutionCallback{Success,Failure,Heartbeat}.go and their types.go/serializers.go/deserializers.go on the installed aws-sdk-go-v2/service/lambda@v1.101.2 module (unchanged for these ops/types between v1.97.0 and v1.101.2). All 9 ops confirmed present in the SDK (not a gopherstack-invented family). Fixed: (1) GetDurableExecutionOutput splits DurableExecutionArn/DurableExecutionName (was one merged ExecutionArn), uses Unix-epoch StartTimestamp/EndTimestamp (was ISO8601 StartTime/StopTime), and adds the previously-entirely-absent DurableConfig echo, Error, ExecutionDataIncluded (honors ?IncludeExecutionData=, default true), InputPayload, Result, TraceHeader, Version; (2) DurableExecutionStatus gained TIMED_OUT; (3) GetDurableExecutionHistory's Events use real types.Event field names/types (EventId/epoch EventTimestamp/EventType/Id/Name/ParentId/SubType + the 5 Execution*Details subtypes this emulator's checkpoint-driven state machine can produce), honors IncludeExecutionData (redacts payload/result/error sub-fields via fresh copies, never mutating the stored event) and ReverseOrder, paginates via Marker/MaxItems (pkgs/page) — previously emitted one invented 'Checkpoint' EventType (not a real enum value) with no pagination; (4) GetDurableExecutionState returns real types.Operation-shaped Operations (Id/Type/Status/StartTimestamp/EndTimestamp/Name/ParentId/SubType) tracked through a new CheckpointDurableExecution Updates state machine (Action START/SUCCEED/FAIL/CANCEL/RETRY on STEP/WAIT/CALLBACK/CONTEXT/CHAINED_INVOKE operations, each mapped to its real EventType via a verified (Type,Action)->EventType table) — CheckpointDurableExecutionInput/Output were previously dead types (handler read an untyped map and discarded it; GetDurableExecutionState always echoed only raw StateData with no Operations). Also found (via the required field-diff) and fixed two real ROUTING bugs beyond the named field-shape gap: StopDurableExecution was wired as DELETE on the bare execution path returning the full execution object — real wire is POST .../stop returning {StopTimestamp} (epoch), and an unknown-ARN Stop silently 200'd 'idempotent' — now 404 ResourceNotFoundException matching Get/GetState; ListDurableExecutionsByFunction was wired at GET /2025-12-01/durable-executions?FunctionArn= — the real op is GET /2025-12-01/functions/{FunctionName}/durable-executions, a completely different path family, now correctly routed with DurableExecutionName/Statuses/StartedAfter/StartedBefore/ReverseOrder/Marker/MaxItems all wired. Also fixed: SendDurableExecutionCallback{Success,Failure,Heartbeat} were routed under the durable-executions ARN prefix with suffixes /callback/success|failure|heartbeat — the real wire is a wholly separate resource, POST /2025-12-01/durable-execution-callbacks/{CallbackId}/{succeed|fail|heartbeat} (note succeed/fail, NOT success/failure) keyed by CallbackId alone; now correctly routed, resolved via a callbackOwner index populated when a checkpoint Update starts a CALLBACK operation, and 404s on an unknown CallbackId (previously silently 200'd regardless). Locking hardened as part of the rewrite: durableExecutionStore's raw sync.RWMutex replaced with lockmetrics.RWMutex (pkgs-catalog.md's 'one coarse instrumented mutex per invariant' rule — this file was the one remaining raw-mutex holdout in the package), and every read method now builds its complete wire response — deep-copying any *DurableOperation it returns — while still holding the lock, rather than handing the handler a live internal pointer to read unsynchronized (previously a genuine, if not test-triggered, data race between a concurrent Get and Checkpoint/Stop on the same execution). Deliberately unchanged, pre-existing, out-of-gap-scope limitation: gopherstack has no StartDurableExecution entry point (correctly — neither does the real API; AWS starts an execution implicitly on Invoke) and this emulator's Invoke path does not model durable-execution semantics, so it still auto-creates the execution record on its first CheckpointDurableExecution call. FunctionArn/DurableConfig/InputPayload/Version are therefore wire-correct (right name, right type, will round-trip through the real SDK client) but always empty/nil today, since no caller threads them through that never-built entry point — this is an entry-point/architecture gap, not a wire-shape gap, and rewiring Invoke was out of this task's scope. Also intentionally not populated: the ~19 CONTEXT/STEP/WAIT/CALLBACK/CHAINED_INVOKE *Details sub-objects the real types.Event/types.Operation declare (no step-function-style replay engine exists to produce their contents) — the generic Id/Name/ParentId/SubType/EventType/Status fields ARE populated for those operation types via the Updates state machine, only the type-specific Details payloads are omitted."}
  capacity_providers: {status: ok, note: "gopherstack-m53b (required-member sweep pass 4). CreateCapacityProvider read a top-level \"Name\" field that does not exist on the wire -- the real required field is CapacityProviderName (api_op_CreateCapacityProvider.go:28-45 vs the old models.go CreateCapacityProviderInput) -- so every real client request 400'd with \"Name is required\" before ever reaching the backend; PermissionsConfig and VpcConfig, both also required, were dropped entirely. Full-shape read (per this sweep's standing instruction) found the drop was worse than the three named fields: CapacityProvider/CreateCapacityProviderInput/UpdateCapacityProviderInput had a wholesale-fabricated shape -- a TargetOnDemandConcurrency field that appears nowhere in the real API (removed), Status/LastModifiedTime field names that are actually State/LastModified on the wire (renamed), an ACTIVE status value where the real CapacityProviderState enum is title-cased Active/Pending/Failed/Deleting (fixed), and CapacityProviderScalingConfig/InstanceRequirements/KmsKeyArn/PropagateTags/TelemetryConfig(partially)/VpcConfig were entirely un-modeled despite being real CapacityProvider members. Rebuilt CreateCapacityProviderInput/UpdateCapacityProviderInput/CapacityProvider field-for-field against types.CapacityProvider (types/types.go:206-249) and its nested types (CapacityProviderPermissionsConfig/VpcConfig/ScalingConfig/TelemetryConfig, InstanceRequirements, PropagateTags, TargetTrackingScalingPolicy); UpdateCapacityProvider (not itself one of the five named bugs, but sharing the same CapacityProvider model and left broken by a narrower fix) was corrected alongside it -- CapacityProviderName is a URI label there, not a body field (serializers.go:7098-7113), matching the existing name-from-path handler wiring. Get/List now correctly echo the real state instead of a fabricated shape. Existing tests (capacity_providers_test.go) encoded the broken \"Name\"/TargetOnDemandConcurrency shape end to end (3 create/update/list tests + 1 telemetry test); corrected to the real field names, and a Test_SDKRoundTrip_CreateCapacityProvider/Test_SDKRoundTrip_UpdateCapacityProvider pair added, driving the real aws-sdk-go-v2 lambda client end to end -- both fail against the unfixed decode (hand-reverted and confirmed). TestHandlerReset_ClearsState (dispatch_test.go) also encoded the old \"Name\" shape and was corrected. gopherstack-r80d (required-OUTPUT-member sweep): DeleteCapacityProvider returned bare 204 No Content, but DeleteCapacityProviderOutput.CapacityProvider is required on the wire (api_op_DeleteCapacityProvider.go:44-46) -- real AWS returns 200 with the deleted provider's state. The real SDK deserializer treats an empty 204 body as JSON-decode-EOF (not an error), so the old code produced a client-side success with CapacityProvider left nil -- exactly the zero-value-on-success-path bug class. Fixed: DeleteCapacityProvider now returns the pre-deletion snapshot, handler responds 200 with {CapacityProvider}. Test_SDKRoundTrip_DeleteCapacityProvider added, driving the real client; fails against the unfixed handler with 'Expected value not to be nil' on CapacityProvider (hand-reverted and confirmed). Full sweep of the other 20 required-output-member ops in this service's SDK surface (CheckpointDurableExecution, Create/Get/List/UpdateCapacityProvider, Create/Get/UpdateCodeSigningConfig, GetDurableExecution/-History/-State, GetFunctionCodeSigningConfig, Create/Get/List/UpdateFunctionUrlConfig, ListFunctionVersionsByCapacityProvider, PutFunctionCodeSigningConfig, PutRuntimeManagementConfig, StopDurableExecution) found all correctly populated on their success paths -- this was the only miss."}
  route_reachability: {status: ok, note: "gopherstack-l5ir (2026-08-13). All 85 real lambda ops extracted from serializers.go (request.Method + httpbinding.SplitURI in each op's awsRestjson1_serializeOp<Op>.HandleSerialize) and diffed against the route table. Found and fixed 12 ops that were unreachable or misrouted at their true path/method, beyond the two routing bugs durable_execution's rewrite already caught (see that family's note): GetLayerVersionByArn was wired to a fictional literal path /2018-10-31/layers-by-arn -- the real op shares ListLayers' bare /2018-10-31/layers path, disambiguated only by a ?find=LayerVersion query flag (the query-parameter-discriminator class this sweep was told to watch for specifically); ListFunctionEventInvokeConfigs checked a fictional plural suffix /event-invoke-configs instead of the real /event-invoke-config/list; GetFunctionRecursionConfig/PutFunctionRecursionConfig used date 2024-08-28 instead of the real 2024-08-31; GetFunctionScalingConfig/PutFunctionScalingConfig used date 2023-10-26 AND path segment scaling-config instead of the real 2025-11-30 and function-scaling-config (both wrong, independently); ListTags/TagResource/UntagResource used date 2015-03-31 instead of the real 2017-03-31 -- all three tagging operations were unreachable; InvokeAsync's suffix predicate required a trailing slash (/invoke-async/) the real client never sends (real path has none); ListLayerVersions/PublishLayerVersion resolved via a separate parallel implementation (extractLayerOperation, used by ExtractOperation and IAMAction, NOT by the real HTTP dispatch table which was already correct) that left its discriminating segment empty for exactly this path shape, so both ops always fell through to empty/Unknown -- a real IAM-action and CloudTrail-naming gap even though the request itself was correctly handled. Also corrected, not a bug: ExtractOperation previously returned the lambdaOpRoutes table's first-matching entry for POST .../invocations, which was the literal string \"InvokeFunction\" -- that is the correct IAM *action* name for this op (a documented AWS naming quirk where the IAM action differs from the API operation name) but the wrong *operation* name; ExtractOperation now special-cases this path to return the real op name \"Invoke\" while IAMAction is untouched and still correctly returns lambda:InvokeFunction. ExtractOperation, previously covering only ~30 of 85 ops (CRUD, layers, durable exec), was extended to mirror dispatchSpecialRoutes/lambdaOpRoutes/layerOpTable op-for-op so TestExtractOperation_SDKRouteTable (handler_paths_sdk_diff_test.go, one subtest per op) exercises the real dispatch tree directly -- 85/85 pass. Existing tests that encoded the old wrong paths/dates/expected-op-names (tags_test.go, handler_tags_iam_test.go, function_settings_test.go, event_invoke_config_test.go, layers_http_test.go, invocation_test.go, handler_routing_test.go) were corrected to the real shapes rather than preserved."}
gaps: []
deferred: []
leaks: {status: ok, note: "gopherstack-9zx (2026-09-03): 2 real leak-class bugs found + fixed, see dated section below -- cleanupTimedOutRuntime silently dropped container/port/tempdir cleanup when b.cleanupSem was saturated (its two sibling call sites already fell back to inline cleanup; this one just returned), and a genuine async-invocation timeout skipped both retry and DLQ/on-failure destination delivery entirely (AWS treats a runtime timeout as a function error for async purposes). Everything else re-verified clean this pass: event-source pollers + janitor + container lifecycle otherwise leak-conscious; go test -race passes (3/3 clean runs). New PublishVersionWithRevision path adds no new goroutines/locks (reuses the existing PublishVersion lock); layerPolicyRevisionID/policyRevisionID are pure functions with no new backend state (derived from already-persisted b.permissions / b.layerPolicies, so no new persistence surface either). durable_execution rewrite: durableExecutionStore starts no goroutines and holds no live resources (pure in-memory map + mutex), so Shutdown has nothing to drain; every Lock/RLock is immediately followed by a deferred Unlock/RUnlock with no intervening early return; b.durableExecs.reset() (lifecycle.go) clears both the executions map and the callbackOwner index together, so no ghost callbackOwner entries survive a Reset."}
---

## Notes
- InvocationType is a type alias (type InvocationType = string) so lambda backend satisfies sns.LambdaInvoker directly.
- ARN-parsing anti-pattern "take last colon segment" recurs — watch for it elsewhere.
- Trap: RemovePermission wire = DELETE /2015-03-31/functions/{name}/policy/{StatementId} (path, not query).
- ce30166a (Parity sweep 3, unrelated commit that swept in a large dependency+datalayer PR) converted most lambda backend maps to pkgs/store Table/Index. eventInvokeConfigs, versions, layers, versionCounters, functionConcurrencies, layerVersionCounters, layerPolicies, activeConcurrencies, fnCodeSigningConfigs, fisFaults, runtimeManagementConfigs, functionRecursionConfigs, functionScalingConfigs, versionIndex, esmByFunctionARN, runtimes, functionURLServers were deliberately left as plain maps (documented per-field in store_setup.go's package doc) — each has a concrete reason (no pure identity in the value, one-to-many shape, or live non-serializable state). Read that doc comment before "fixing" any of them into a Table.
- pkgs/store.Table/Index perform NO internal locking (by design — see pkgs/store package doc); every lambda call site still takes b.mu itself. Index.Get() returns a slice OWNED BY THE INDEX — never return it directly from a public method without copying first (ListAliases/GetPolicy both copy correctly; verified).
- Policy RevisionId (function-policy and layer-version-policy) is deliberately a pure content-hash of the sorted StatementId set (policyRevisionID in permissions.go, layerPolicyRevisionID in layers.go), NOT a stored uuid.New()-per-mutation field like Function/Version/Alias RevisionID. This works because statement content is immutable once added (no UpdatePermission op exists — a StatementId can only be added once, then removed), so the ID set alone detects every real mutation, and it stays correct across Snapshot/Restore without adding new persisted state.
- writeError's return value is NOT a reliable "did this write an error response" signal — c.JSON (which it wraps) returns nil on any successful write, including a written error, so `if xErr := h.writeError(...); xErr != nil` can never trigger. Handler helpers that write an error and need the caller to stop must return bool (true=continue), matching validateMemoryAndTimeout/checkRevisionID/applyFunctionCodeUpdate. A stale `!= nil` check on such a helper is a latent double-write bug (found + fixed in applyFunctionCodeUpdate this sweep) — grep for this pattern before trusting any "returns error, checked with != nil" helper that calls writeError internally.
- Durable-execution family spans THREE independent path prefixes, not one — do not assume everything nests under `/2025-12-01/durable-executions/{DurableExecutionArn}/...`: GetDurableExecution/History/State + CheckpointDurableExecution + StopDurableExecution do; ListDurableExecutionsByFunction is `/2025-12-01/functions/{FunctionName}/durable-executions` (a `/functions` path, verified against api_op_ListDurableExecutionsByFunction.go); SendDurableExecutionCallback{Success,Failure,Heartbeat} is `/2025-12-01/durable-execution-callbacks/{CallbackId}/{succeed|fail|heartbeat}` keyed by CallbackId, not DurableExecutionArn (note succeed/fail, not success/failure — trap for anyone guessing the suffix). See handler_paths.go's prefix constants and handler_durable_execution.go's `isDurableExecPath`/`dispatchDurableExecRoutes`.
- Lambda's REST API is spread across a dozen+ date-versioned path prefixes (2015-03-31, 2017-03-31, 2017-10-31, 2018-10-31, 2019-09-25, 2019-09-30, 2020-04-22, 2020-06-30, 2021-07-20, 2021-10-31, 2021-11-15, 2024-08-31, 2025-11-30, 2025-12-01 all appear). gopherstack-l5ir found 4 of these constants carrying a wrong date (tags: 2015-03-31 vs real 2017-03-31; recursion-config: 2024-08-28 vs real 2024-08-31; scaling-config: 2023-10-26 vs real 2025-11-30) that made every op under that prefix unreachable. When adding or auditing any lambda op, verify its date prefix against `httpbinding.SplitURI(...)` in serializers.go directly -- do not assume a "close enough" date is correct, and do not trust an existing constant's date without checking it against the SDK source at least once.
- durable_execution is intentionally NOT wired into Snapshot/Restore (durableExecutionStore isn't touched by persistence.go) — this predates the wire-shape rewrite and is unrelated to it; durable executions were never persisted, only cleared on Reset (lifecycle.go's `b.durableExecs.reset()`). Not flagged as a bug: no entry point exists to repopulate FunctionArn/DurableConfig/InputPayload after a restore anyway (see durable_execution family note above), so persisting the store today would only round-trip empty shells.
- `ListLayers` and `ListLayerVersions` summary narrowing: `LayerVersion.Content` was previously populated on `ListLayers` and `ListLayerVersions` responses. In `aws-sdk-go-v2/service/lambda@v1.101.2`, `types.LayerVersionsListItem` does not contain `Content` (only `GetLayerVersion` / `PublishLayerVersion` returns `Content`). Fixed: `ListLayers` and `ListLayerVersions` omit `Content`.


## 2026-08-23: pagination bug sweep (ListLayerVersions, ListProvisionedConcurrencyConfigs, ListCodeSigningConfigs, ListFunctionsByCodeSigningConfig)

Discovered while auditing the pagination bug class found in medialive.
`handleListLayerVersions`, `handleListProvisionedConcurrencyConfigs`,
`handleListCodeSigningConfigs`, and `handleListFunctionsByCodeSigningConfig`
all ignored the real `Marker`/`MaxItems` request members (lambda@v1.101.2:
`ListLayerVersionsInput`, `ListProvisionedConcurrencyConfigsInput`,
`ListCodeSigningConfigsInput`, `ListFunctionsByCodeSigningConfigInput`) and
always returned every item in one unbounded page with no `NextMarker`,
despite `NextMarker` already existing (unused) on all four output structs.
Fixed using the existing `parsePaginationParams` + `pkgs/page.New` +
`lambdaDefaultMaxItems` pattern already used by `ListFunctions`/`ListLayers`
in this package. `ListLayerVersions`, `ListProvisionedConcurrencyConfigs`,
and `ListFunctionsByCodeSigningConfig` are unexported `*InMemoryBackend`
methods (not part of a public interface) but changed return type from a
bare slice to `page.Page[T]`; `go build ./...` confirmed clean, and two
pre-existing test call sites (persistence_test.go, layers_test.go) updated
for the new `ListLayerVersions` signature. Proven with four
`Test*_SDKRoundTrip_Pagination` tests (`list_pagination_ignored_test.go`),
each driving the real SDK client across two 10-item pages of 25 seeded
items and asserting the pages are disjoint; all four fail against the
unfixed handlers (`should have 10 item(s), but has 25`), hand-reverted
and confirmed.

Audited but NOT fixed: `handleListFunctionURLConfigs` also ignores
Marker/MaxItems, but the route is always called with a non-empty
`{name}` path segment, and the per-function code path
(`GetFunctionURLConfig(name)`) can only ever return 0 or 1 items — this
service's data model has no per-qualifier function URL configs, so the
unbounded branch is dead code with zero real blast radius. Not fixed.

## 2026-08-23: DeleteFunction's Qualifier discarded — every delete removed the whole function

Read `serializeOpHttpBindings<Op>Input` directly for `DeleteFunctionInput`
(lambda@v1.101.2 serializers.go:1690,
`awsRestjson1_serializeOpHttpBindingsDeleteFunctionInput`): `FunctionName`
is URI-bound, `Qualifier` is query-bound
(`encoder.SetQuery("Qualifier")`). `handleDeleteFunction`
(`handler_functions.go`) never read the query string at all — it called
`h.Backend.DeleteFunction(name)` unconditionally, so a client asking to
delete one published version (`DeleteFunctionInput{FunctionName,
Qualifier: "2"}`) instead had the entire function deleted: every version,
every alias, every event source mapping. `api_op_DeleteFunction.go`'s doc
comment is explicit: "To delete a specific function version, use the
Qualifier parameter. Otherwise, all versions and aliases are deleted", and
"You can't delete a version that an alias references." The backend already
tracked exactly the state this needed (`b.versionIndex`/`b.versions` for
published versions, `b.aliasesByFunction` for the alias-reference check) —
only `DeleteFunction`'s dispatch ignored the qualifier.

Fixed via the existing `QualifierInvoker`/`QualifierResolver`
optional-extension pattern (`store.go`) rather than changing
`StorageBackend.DeleteFunction`'s existing signature (would have required
touching `services/cloudformation/resources.go:2150`, the one out-of-package
caller, and running `make build-check`): added `QualifierDeleter` with
`DeleteFunctionVersion(name, qualifier string) error`, implemented on
`InMemoryBackend` (`functions.go`). `handleDeleteFunction` now reads
`Qualifier` off the query string; when present it type-asserts
`QualifierDeleter` and calls `DeleteFunctionVersion`, which deletes only the
targeted `b.versionIndex[name][qualifier]` entry (and its `b.versions[name]`
slice element) after checking `b.aliasesByFunction` for a referencing alias
(`ErrVersionReferencedByAlias`, new sentinel → 409 ResourceConflictException)
and rejecting `Qualifier=$LATEST` (`ErrInvalidParameterValue` → 400 — $LATEST
has no separate version resource; omit Qualifier to delete the whole
function). An empty Qualifier still calls the original unqualified
`DeleteFunction` path unchanged. Function tags are only released when the
whole function is deleted (`qualifier == ""`).

`TestDeleteFunction_Qualifier` (`delete_function_version_test.go`) drives
the real `aws-sdk-go-v2` lambda client, table-driven across three cases:
qualified delete removes only the targeted version ($LATEST and the other
version survive, `GetFunctionConfiguration(Qualifier: v1)` now 404s);
qualified delete is rejected with `ResourceConflictException` when an alias
still references that version (and the version survives the rejected
delete); unqualified delete still removes the whole function. Hand-reverted
`handleDeleteFunction` back to its pre-fix unconditional
`h.Backend.DeleteFunction(name)` call: both the "removes only that version"
and "blocked by alias reference" subtests failed exactly as predicted (the
whole function vanished instead of just the targeted version, so
`GetFunctionConfiguration` against the survivor 404'd and the
expected-error assertion against the alias-referenced delete saw no error
at all); restored and confirmed byte-identical via `md5sum`.

**Modelling gaps found in the same header sweep, not implemented**:
`InvokeInput`'s `TenantId` (lambda@v1.101.2 serializers.go:3859,
`awsRestjson1_serializeOpHttpBindingsInvokeInput`) is a real
`X-Amz-Tenant-Id` header for Lambda's multi-tenant-function feature —
gopherstack has no tenant concept anywhere in this service, so this is a
genuine unmodeled feature, not a discarded-but-tracked field; reported, not
attempted. `InvokeInput.DurableExecutionName` (request header) and
`InvokeOutput.DurableExecutionArn` (response header, deserializers.go:8744,
`awsRestjson1_deserializeOpHttpBindingsInvokeOutput`) are likewise never
wired on the `Invoke` path — consistent with, not a new instance of, the
already-documented durable_execution family gap above ("gopherstack has no
StartDurableExecution entry point... this emulator's Invoke path does not
model durable-execution semantics").

Gates: `go build ./...`, `go vet ./services/lambda/...`, `go test -race
-count=1 ./services/lambda/...`, `go fix -diff ./services/lambda/...` (no
diff), `gofmt -l services/lambda/` (no output), `golangci-lint run
./services/lambda/...` (1 finding — `godot` on the new
`DeleteFunctionVersion` doc comment's closing quoted sentence, fixed by
rewording so the comment's last line ends outside the quote; 0 issues after,
no `//nolint` added), `go test ./pkgs/persistence/...` (no persisted struct
changed) all clean. No exported method signature was changed —
`StorageBackend.DeleteFunction` is untouched — so `make build-check` was not
required; `go build ./...` (whole repo) confirmed clean regardless.


## 2026-08-28: UpdateAlias didn't validate FunctionVersion, unlike CreateAlias

gopherstack-huyl (Create-vs-Update precondition sweep). `UpdateAlias`
(versions_aliases.go) set `alias.FunctionVersion = input.FunctionVersion`
unconditionally, so an alias could be repointed at a version number that was
never published — `CreateAlias` validates the target version against
`b.versions[name]` (or accepts `$LATEST`), but `UpdateAlias` had no
equivalent check. lambda@v1.101.2 deserializers.go's
`deserializeOpErrorUpdateAlias` models `ResourceNotFoundException` (the same
code `ErrVersionNotFound` already maps to on the `CreateAlias` path), so the
fix mirrors `CreateAlias`'s `versionInList` check and reuses the existing
sentinel error. `handleUpdateAlias` (handler_versions_aliases.go) previously
had no `ErrVersionNotFound` case at all — added one, matching `handleCreateAlias`'s.
New real-SDK-client proof: `TestUpdateAlias_UnknownVersionSurfacesResourceNotFoundException`
(`$LATEST` still exempted, proven by `TestUpdateAlias_LatestVersionSucceeds`)
in `wire_field_fixes_test.go`; hand-reverted `versions_aliases.go` +
`handler_versions_aliases.go`, confirmed both tests fail
(`ResourceNotFoundException` never surfaced), restored.

## 2026-08-28: PutFunctionScalingConfig invented a flat MaximumConcurrency field (acceptguard)

acceptguard flagged `PutFunctionScalingConfigInput.MaximumConcurrency` (`models.go:130`, read
in `PutFunctionScalingConfig`) as matching no member of any real Input in the module. Confirmed
against lambda@v1.101.2's real shape (`api_op_PutFunctionScalingConfig.go`,
`api_op_GetFunctionScalingConfig.go`, `types/types.go:1614`): the real request nests a
`FunctionScalingConfig *types.FunctionScalingConfig` under the request body key
`"FunctionScalingConfig"`, and that nested type carries `MinExecutionEnvironments`/
`MaxExecutionEnvironments` (both `*int32`) — an unrelated concept (execution-environment
pool sizing for Lambda Managed Instances functions) to the flat concurrency-limit field a
prior version invented. `GetFunctionScalingConfigOutput` is also a different shape than what
gopherstack emulated: `AppliedFunctionScalingConfig`/`RequestedFunctionScalingConfig`/
`FunctionArn` as three top-level members, not a single flat struct.

Fixed by reshaping `FunctionScalingConfig` to the real nested type
(`MaxExecutionEnvironments`/`MinExecutionEnvironments`), `PutFunctionScalingConfigInput` to
nest it under `FunctionScalingConfig`, and adding real `PutFunctionScalingConfigOutput`
(`FunctionState`) and `GetFunctionScalingConfigOutput` (`AppliedFunctionScalingConfig`/
`RequestedFunctionScalingConfig`/`FunctionArn`) types (`models.go`). Backend methods
(`function_settings.go`) now return/accept the real Output/Input shapes directly. The
concurrency-throttling logic in `invocation.go` (`acquireConcurrencySlot`) that previously read
`sc.MaximumConcurrency` now reads `sc.MaxExecutionEnvironments` as its enforcement knob — a
reasonable emulation choice given execution-environment count is the real API's actual
concurrency-shaping lever for this operation, and no other field in the real shape serves an
analogous role.

Proven via a real `aws-sdk-go-v2/service/lambda` client round trip
(`TestPutFunctionScalingConfig_MinMaxExecutionEnvironments`, `wire_field_fixes_test.go`):
`PutFunctionScalingConfig` with `MinExecutionEnvironments`/`MaxExecutionEnvironments`, then
`GetFunctionScalingConfig` asserts both values round-trip through
`AppliedFunctionScalingConfig`/`RequestedFunctionScalingConfig`/`FunctionArn`. Hand-reverted
`function_settings.go`/`invocation.go`/`models.go`/`store_setup.go`, confirmed the test fails
(the real client's `FunctionScalingConfig` was never read; response fields empty), restored.

**Test judgement**: `function_settings_test.go`'s `TestFunctionScalingConfig_PutGet` sent a raw
body of `{"MaximumConcurrency":10}` and asserted it round-tripped — testing the invented field as
correct. Rewrote to send the real wire shape (`{"FunctionScalingConfig":{"MaxExecutionEnvironments":10}}`)
and assert against `AppliedFunctionScalingConfig.MaxExecutionEnvironments`.
`TestScalingConfig_MaximumConcurrency_Enforced`/`TestScalingConfig_ZeroConcurrency_Blocked`
constructed `PutFunctionScalingConfigInput{MaximumConcurrency: &n}` literals directly — updated
to the nested `FunctionScalingConfig{MaxExecutionEnvironments: &n}` shape; the concurrency
enforcement behavior itself (a limit of N blocks the N+1th concurrent invocation) was already
correct and is unchanged, only the field it reads moved.

Known gap noted, not fixed (out of scope for this finding): the real
`PutFunctionScalingConfigInput`/`GetFunctionScalingConfigInput` mark `Qualifier` as a required
member (a version/alias-scoped scaling config), but gopherstack's route
(`/2025-11-30/functions/{name}/function-scaling-config`) has no qualifier segment and the
backend stores one scaling config per function name regardless of qualifier. A real client must
still supply `Qualifier` (client-side SDK validation requires it), and gopherstack silently
ignores it rather than erroring or scoping by it. Worth a follow-up bd issue.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/lambda/...`).

### 2026-08-29 -- ERROR PATH sweep (wrong-error-code class)

Extracted ground truth from all 85 `awsRestjson1_deserializeOpError<Op>` switches
in `lambda@v1.101.2/deserializers.go` (REST-JSON, matched via `strings.EqualFold`
against `X-Amzn-ErrorType`/body `__type`) and diffed every literal exception-code
string used across `services/lambda/*.go` (both `errors.go` sentinels and
handler-inline `h.writeError(...)` literals) against both that per-op ground
truth and the 56 real shapes in `lambda@v1.101.2/types/errors.go`.

**Unlike ecs this same sweep, lambda's codes were already disciplined**: of 9
distinct literal exception-name strings hardcoded in handler files (outside the
`errors.go` sentinel table), only `MethodNotAllowedException` isn't a real
Lambda type -- and that one is a router-level HTTP-405 guard on unsupported
path/method combinations, not tied to any operation's error model (a real SDK
client can never trigger it), so it's out of this bug class and untouched.

**2 bugs found and fixed**, both the "two distinct exceptions are both modeled
by this exact op, and gopherstack always emits the wrong one" shape (same as
cloudformation's `DescribeStackInstance` this same sweep):

1. `PutFunctionCodeSigningConfig` (`code_signing.go`): when the function exists
   but the given `CodeSigningConfigArn` doesn't, the backend returned
   `ErrFunctionNotFound` ("ResourceNotFoundException") -- the *function*-not-found
   sentinel -- for the CSC-not-found case too (the handler's own error message
   literally said "Function or code signing config not found", indicating the
   two conditions were known but conflated). This op's own deserializer models
   `CodeSigningConfigNotFoundException` as a distinct shape from
   `ResourceNotFoundException`; fixed the backend to return
   `ErrCodeSigningConfigNotFound` for this branch and the handler to map it to
   the correct wire code.
2. `GetProvisionedConcurrencyConfig` (`concurrency.go`/`handler_concurrency.go`):
   the "config not found for this qualifier" branch already used a
   distinctly-named sentinel (`ErrProvisionedConcurrencyConfigNotFound`) but the
   handler mapped it to the generic `ResourceNotFoundException` wire code
   instead of `ProvisionedConcurrencyConfigNotFoundException`, which this op's
   own deserializer models as a separate shape. `DeleteProvisionedConcurrencyConfig`
   uses the same sentinel correctly -- its own deserializer does **not** model
   the specific exception, only `ResourceNotFoundException`, so that call site
   was left unchanged (verified from its own switch, not assumed from the
   sibling).

Pre-existing test asserting the wrong behavior as correct (found and fixed,
same shape as the iam `InvalidAction` test): `provisioned_concurrency_test.go`'s
`TestGetProvisionedConcurrencyConfig/config_not_found` asserted `wantErrType:
"ResourceNotFoundException"`; updated to `"ProvisionedConcurrencyConfigNotFoundException"`.

New tests: `error_code_fixes_lambdasweep_test.go`, both driving the real
`aws-sdk-go-v2/service/lambda` client and asserting via `errors.As` against the
SDK's own typed exception; both confirmed failing against the pre-fix code.

Gates: `go build ./services/lambda/...`, `go vet ./services/lambda/...` and
repo-wide `go vet ./...` (clean except a pre-existing, unrelated
`services/appconfig` failure from a concurrently-edited service), `go test
-race -count=1 ./services/lambda/...` (pass), `golangci-lint run --fix
./services/lambda/...` (0 issues).

## 2026-08-30 enumcheck typed-response-struct extension: 5 findings, all false positives

`cmd/enumcheck` was extended to see an enum value carried on a named
response struct's own composite literal, not only a `map[string]any` entry.
Run against `services/lambda`, it surfaced 5 needs-review findings, all
under an SDK-wide ambiguous wire key ("Status" or "Type" shared by
`OperationStatus`/`ExecutionStatus`/`ProvisionedConcurrencyStatusEnum` or
`KafkaSchemaRegistryAuthType`/`OperationType`/`SourceAccessType` in
`lambda@v1.101.2/types/enums.go`). Hand-checked against each site's true
field: `ProvisionedConcurrencyConfig.Status = "READY"` (legal
`ProvisionedConcurrencyStatusEnumReady`), `DurableExecution.Status =
"RUNNING"` (legal `ExecutionStatusRunning`), `DurableOperation.Type =
"EXECUTION"` (legal `OperationTypeExecution`), `DurableOperation.Status =
"STARTED"` (legal `OperationStatusStarted`), `TracingConfig.Mode =
"PassThrough"` (legal `TracingModePassThrough`). Every value is a real
member of its true single candidate; each only fails the ambiguous-key
tier's "legal in every candidate" check because the other enum(s) sharing
the wire key don't declare that member. No bug found; nothing changed in
this service.

## 2026-08-30 (gopherstack-uox6, value-semantics sweep): event_filter.go, 2 bugs

Audited eventFilterMatches/patternMatchesObject/fieldMatchesRule/operatorMatches
(event_filter.go) -- the FilterCriteria/Filter.Pattern event-pattern matcher shared
by SQS/Kinesis/DynamoDB event source mappings -- against the real AWS Lambda "Filter
rule syntax" comparison-operator table (docs.aws.amazon.com/lambda/latest/dg/
invocation-eventfiltering.html; the pinned SDK's types.go carries no prose for this
family, FilterCriteria.Filters[].Pattern is a bare *string). 2 bugs, both under-
matching:

- `$or` ("Or (multiple fields)" in AWS's own table, example `"$or": [
  {"Location":["New York"]}, {"Day":["Monday"]} ]`) was not special-cased at all --
  patternMatchesObject treated "$or" as a literal record field name, so
  `value["$or"]` was always absent and the clause could never match, silently
  discarding an entire documented operator. Fixed: patternMatchesObject now
  recognizes "$or", evaluating its array of sibling pattern fragments against the
  same value and ORing the results; a non-"$or" sibling key in the same object still
  ANDs against it normally.
- `exists`: AWS's own doc states plainly "the Exists operator only works on leaf
  nodes in your event source JSON. It doesn't match intermediate nodes," with a
  worked example (`{"person":{"address":[{"exists":true}]}}` does NOT match even
  though `address` is present, because its value is an object, not a leaf).
  existsMatches previously took only (arg, present bool) and had no way to see the
  field's value, so it matched purely on key-presence -- exists:true incorrectly
  matched an intermediate/nested-object field. Fixed: existsMatches now also takes
  fieldVal and returns false whenever the field is present but its value is a
  map[string]any (an intermediate node), matching the documented example exactly.

Gaps recorded, not fixed (documentation doesn't state these precisely enough to
implement without guessing): the page's own text says "Lambda supports the Amazon
EventBridge rules and uses the same syntax as EventBridge," but the page's
comparison-operator table lists only Null/Empty/Equals/Equals-ignore-case/And/Or/
$or/Not (anything-but)/Numeric/Exists/prefix/suffix -- no `wildcard`, no `cidr`, and
no nested anything-but forms (`{"anything-but":{"prefix":...}}` etc.) appear in that
table, even though EventBridge itself documents them. Whether Lambda's event
filtering actually honors those beyond the table is not stated on this page, so
`wildcard`/`cidr` remain unimplemented (singleOperatorMatches's default case returns
false, i.e. they always fail to match rather than being silently accepted) and a
nested-object arg to `anything-but` still falls through to
`!scalarMatches(...)` (always true, i.e. an unconditional match) rather than being
given real prefix/suffix/equals-ignore-case semantics -- left as-is rather than
fabricated.

New/changed tests (event_filter_test.go, table-driven, same TestLambda_EventFilterMatches
func): +4 cases (2 for $or matching/non-matching/AND-with-sibling, 1 for the
intermediate-node exists fix), all confirmed failing against unmodified code first
(2 actually fail pre-fix: "$or matches when second branch matches" and "exists true
does not match an intermediate object node"; the other 2 new $or cases pass either
way since their expected result is `false` under both the buggy and fixed logic, but
are kept as regression coverage for the AND-with-sibling-key and no-branch-matches
shapes). Assertion count: 26 -> 30 subtests, 0 dropped, all pre-existing cases
unchanged.

Gates: `go build ./services/lambda/...`, `go vet ./services/lambda/...` and repo-wide
`go vet ./...` (clean), `go test -race -count=1 ./services/lambda/...` (pass),
`golangci-lint run ./services/lambda/...` (0 issues).

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD.
`cmd/reqfielddiff`: findings ranged 234-240 across the 5 old runs (234 at
HEAD), 6 op.field keys moving: `CreateFunctionUrlConfig`/`UpdateFunctionUrlConfig`
`.{AuthType,Cors}` and `CreateFunctionUrlConfig.InvokeMode`, all present in
some old (misresolved) run and absent at HEAD. The collision is
`FunctionUrlConfig`/`FunctionURLConfig`: `handleCreateFunctionURLConfig`/
`handleUpdateFunctionURLConfig` (the real handlers) each fold onto a
same-named exported `*InMemoryBackend` method. Read both handler bodies
(`handler_function_urls.go:14,160`): `AuthType` and `Cors` are genuinely
read on both Create and Update; `InvokeMode` is genuinely read on Create.
Over-reporting, safe direction.

**Real bug found and fixed while reading `handleUpdateFunctionURLConfig`
to settle the above** (not itself one of the 6 moved keys -- reqfielddiff
never flagged it, because `UpdateFunctionURLConfigInput` simply had no
`InvokeMode` field to be undeclared against): `UpdateFunctionUrlConfigInput.InvokeMode`
(lambda@v1.101.2 `api_op_UpdateFunctionUrlConfig.go:68`) was never declared
on this package's `UpdateFunctionURLConfigInput` (`models.go`, only had
`Cors`/`AuthType`), so a function URL created `BUFFERED` could never be
switched to `RESPONSE_STREAM` (or back) after creation -- `CreateFunctionUrlConfig`
already supported the field correctly, which is why a spot-check of Create
alone would have said parity was fine. Fixed: added `InvokeMode` to
`UpdateFunctionURLConfigInput`, threaded it through
`handleUpdateFunctionURLConfig` into `(*InMemoryBackend).UpdateFunctionURLConfig`
(new 4th parameter, applied non-destructively like `AuthType`/`Cors`: only
overwrites when non-empty), same pattern as the pre-existing `AuthType`/`Cors`
fields. Single in-package caller of the backend method; no other callers to
fix.

New test `TestUpdateFunctionUrlConfig_InvokeMode`
(`wire_field_fixes_test.go`) drives the real `aws-sdk-go-v2/service/lambda`
client: Create with `InvokeMode: BUFFERED`, Update with
`InvokeMode: RESPONSE_STREAM`, asserts the SDK-decoded `UpdateFunctionUrlConfigOutput`
and a follow-up `GetFunctionUrlConfig` both show `RESPONSE_STREAM`.
Confirmed failing (asserted `BUFFERED`, i.e. the update was dropped)
against the pre-fix code before applying the fix.

Gates: `go build ./services/lambda/...`, `go vet ./services/lambda/...`
(clean), `go test -race -count=1 ./services/lambda/...` (pass, existing
suite unweakened, one new test/3 new assertions added),
`golangci-lint run ./services/lambda/...` (0 issues, no `--fix` used).

## 2026-08-31: parity-targeting method correction re-derivation (gopherstack-6flj/21my)

Queue derivation: real `List*` ops in lambda@v1.101.2 (14 total, lambda has zero
`Describe*` ops) whose full name never appears (case-insensitive, glob-expanded) verbatim
anywhere in this file. Mechanical grep gave 3: `ListCapacityProviders`,
`ListEventSourceMappings`, `ListVersionsByFunction`.

`ListCapacityProviders` field-diffed clean: `types.CapacityProvider` (lambda@v1.101.2
`types/types.go`) has no `Name` member at all (identity is `CapacityProviderArn`-only, real
AWS design, matching this file's existing `UpdateCapacityProvider` URI-label note) --
gopherstack's `CapacityProvider` model carries all 10 real members and none of the
`json:"-"`-internal ones leak onto the wire. Recorded, not fixed (different axis): real
`ListCapacityProvidersInput` declares `Marker`/`MaxItems`/`State` (pagination + a state
filter); `Backend.ListCapacityProviders()` takes no parameters and always returns every
provider on one page, unfiltered -- same "pagination/filter ignored" class already
catalogued elsewhere in this campaign, not a naming bug.

`ListEventSourceMappings` and `ListVersionsByFunction` were NOT clean -- both are the
Get-right/List-wrong sibling shape, and both share the item builder with their respective
singular/publish operations (so the bug reached every caller of that builder, not just the
List op):

1. **`FunctionVersion` (shared by `ListVersionsByFunction`/`PublishVersion`/
   `GetFunction`-by-version) silently dropped 8 real, backend-tracked
   `types.FunctionConfiguration` members that the sibling `FunctionConfiguration`
   type (used by `GetFunctionConfiguration`) already carries correctly:
   `Architectures`/`EphemeralStorage`/`LoggingConfig`/`MasterArn`/`StateReason`/
   `StateReasonCode`/`LastUpdateStatus`/`LastUpdateStatusReason`. Both `fnToVersion` and
   `publishVersion` (`versions_aliases.go`) build `FunctionVersion` directly from a
   `*FunctionConfiguration` that already has every one of these fields populated -- the
   source struct had the data, the conversion never copied it. Fixed: added all 8 fields to
   `FunctionVersion` (`models.go`) with the same json tags `FunctionConfiguration` uses, and
   populated them in both builders. (Two real `types.FunctionConfiguration` members --
   `LastUpdateStatusReasonCode` and several capacity/signing/tenancy fields -- are absent
   from gopherstack's `FunctionConfiguration` too, i.e. a shared gap with no disagreement to
   detect; left as a recorded gap, not fixed this pass. `ReservedConcurrentExecutions`,
   present on gopherstack's `FunctionConfiguration` but not on the real
   `types.FunctionConfiguration` at all, is a separate, pre-existing possible issue on the
   Get side, out of this pass's List-sibling scope -- recorded, not touched.)

   `FunctionVersion` is part of `backendSnapshot` (`persistence.go`'s `Versions
   map[string][]*FunctionVersion`) -- the same struct serves both the wire and the
   persisted shape. The 8 new fields are purely additive (`omitempty`), so
   `TestSnapshotVersionGuard` correctly demanded a golden bookkeeping update rather than a
   version bump; ran with `-update`, confirmed the diff is additive-only, re-ran clean.

   Test: `TestListVersionsByFunction_SiblingFields_RealClient` (`wire_field_fixes_test.go`),
   creates a function with all 8 fields set to distinguishable values via
   `bk.CreateFunction`, publishes two versions, asserts all 8 round-trip through
   `ListVersionsByFunction`'s real SDK client for `$LATEST` and both published versions (3+
   items). Verified failing pre-fix (`Architectures`/`EphemeralStorage` decoded nil/empty).

2. **`ListEventSourceMappings`/`CreateEventSourceMapping`/`GetEventSourceMapping` (all
   sharing `toJSONESMResponse`) never emitted `LastModified`**, despite
   `EventSourceMapping.LastModified` (`event_source_mapping.go`) being real, tracked state
   set at creation. Real `types.EventSourceMappingConfiguration.LastModified` decodes via
   `smithytime.ParseEpochSeconds` on a JSON Number (confirmed against
   lambda@v1.101.2 `deserializers.go`'s
   `awsRestjson1_deserializeDocumentEventSourceMappingConfiguration` case `"LastModified"`)
   -- epoch-seconds, not RFC3339, the same timestamp-format bug class documented elsewhere
   in this campaign. Fixed: added `LastModified float64` to `jsonESMResponse`
   (`event_source_mapping.go`), populated via `awstime.Epoch(m.LastModified)` in
   `toJSONESMResponse`. `golangci-lint run --fix` additionally reordered the struct for `fieldalignment` and, as a
   side effect of that reorder, dropped all three pre-existing `//nolint:lll` directives on
   this struct. That drop was WRONG -- all three lines still exceed 120 characters after
   realignment (128/126/123 chars; the AWS field names themselves are the width, not the
   column position), and a subsequent full `golangci-lint run` (without `--fix`, across all
   three services together) caught the regression: 3 `lll` findings on exactly those lines.
   Restored all three `//nolint:lll // AWS field name` directives by hand; `golangci-lint
   run` back to 0 issues. Recorded here because it is a small but concrete instance of this
   session's own "never trust an artefact's prior verification" mandate applying to a tool's
   own `--fix` output, not just to hand-written notes.

   Recorded, not fixed (different axis, genuine unmodeled gaps):
   `EventSourceMappingArn`/`FilterCriteriaError`/`KMSKeyArn`/`LoggingConfig`/
   `MetricsConfig`/`ProvisionedPollerConfig`/`ScalingConfig`/`StartingPositionTimestamp`/
   `StateTransitionReason` are real `types.EventSourceMappingConfiguration` members with no
   backing state in this backend's `EventSourceMapping` model at all -- each would need new
   accept/store/read wiring, not a field-copy fix.

   Test: `TestListEventSourceMappings_LastModified_RealClient` (`wire_field_fixes_test.go`),
   creates a mapping via the real SDK client, asserts `LastModified` round-trips (non-nil,
   after a pre-call timestamp) on both `CreateEventSourceMapping`'s response and
   `ListEventSourceMappings`. Verified failing pre-fix (`LastModified` decoded nil on
   create).

Protocol: lambda is REST-JSON (`awsRestjson1`, confirmed from `deserializers.go`'s function
prefix) -- no case folding, so any naming mismatch here is a hard failure class.

No wrapper-key mismatches, no hard decode errors/panics, no transpositions, no invented
elements found this pass. Pages fetched: 0 (module cache used throughout).

Gates: `go build ./...` clean; `go vet ./...` clean;
`go test -race -count=1 ./services/lambda/...` clean; `go test -race -count=1
-run TestSnapshotVersionGuard ./pkgs/persistence/` clean (after `-update` refreshed the
additive-only golden, confirmed with `git diff --stat` showing an 8-line addition only);
`golangci-lint run ./services/transfer/... ./services/opensearch/... ./services/lambda/...`
0 issues (one `--fix` pass for `fieldalignment` on `event_source_mapping.go`, scoped to that
file, plus a hand restoration of 3 `nolint:lll` directives `--fix` incorrectly dropped -- see
above). `nolint` directives in files touched this pass: `event_source_mapping.go` has 3
(`//nolint:lll` x3, all pre-existing and now confirmed still necessary). No `nolint`
directives in `models.go`, `versions_aliases.go`, or `wire_field_fixes_test.go`.

## 2026-09-03: resource-leak-focused sweep (gopherstack-9zx), 2 bugs

Five-dimension audit (wire compliance, LocalStack parity, cross-service integration,
performance, resource leaks) with explicit focus on dimension 5 given this package's
long-lived event-source pollers and per-function container lifecycle. Read
`store.go`/`containers.go`/`runtime_api.go`/`invocation.go`/`event_source_poller.go`/
`janitor.go` end to end tracing every goroutine's start/stop and every semaphore's
acquire/release. Both bugs found are in the resource-leak dimension; wire/protocol
compliance was largely spot-checked against this file's existing extensive coverage
rather than re-derived from scratch (see caveat in the final report).

1. **`cleanupTimedOutRuntime` (`containers.go`) dropped cleanup entirely when
   `b.cleanupSem` (cap 64) was saturated.** The two sibling call sites with the
   identical "evict a runtime under `b.mu`, clean it up outside the lock via a bounded
   semaphore" shape -- `lookupOrRegisterRuntime`'s LRU eviction and `UpdateFunction`'s
   runtime eviction -- both fall back to running `cleanupRuntime` inline when the
   semaphore is full, so the container/port/temp-dirs are always released.
   `cleanupTimedOutRuntime` instead had a bare `default: return` on the saturated
   branch: since the timed-out `rt` had *already* been deleted from `b.runtimes` in the
   same function, nothing else in the codebase would ever revisit it, so the container
   was never stopped, the port never released, and `zipDir`/`layerDirs` never removed
   -- a genuine leak of all three, exactly the "concurrency-limit semaphore never
   released on error paths" class this audit was scoped to look for. Fixed to match
   the other two call sites' inline fallback.

2. **A genuine async-invocation timeout never reached the function's DLQ/on-failure
   destination, and was never retried.** `runAsyncInvocationRetryLoop` (`invocation.go`)
   treated `waitForAsyncResult`'s `ok=false` return (container never responded within
   `timeout+containerResponseGracePeriod`) as an unconditional `return` -- skipping
   both the retry loop (ignoring `MaximumRetryAttempts` entirely, even on attempt 0)
   and the terminal `dispatchAsyncOutcome` call that delivers to
   `DeadLetterConfig`/`DestinationConfig.OnFailure`. Verified against AWS's own docs
   (docs.aws.amazon.com/lambda/latest/dg/invocation-async-error-handling.html):
   "Function errors include errors returned by the function's code and errors returned
   by the function's runtime, such as timeouts" -- and "To capture records of failed
   invocations (such as timeouts or runtime errors), create an on-failure destination."
   A real client publishing an async function whose container hangs would see the
   event silently vanish with no DLQ/destination record and no retry, instead of the
   documented retry-then-deliver behavior. Fixed by folding the timeout case into the
   same `isError` path already used for a runtime-reported `/error` result, so a
   timeout is now retried (subject to `MaximumRetryAttempts`) and, once retries are
   exhausted, reaches the destination/DLQ with `functionError: "Unhandled"`.

   Known related limitation, not fixed (deeper architecture question, out of this
   fix's scope): a retry scheduled after a timeout re-enqueues onto the *same*
   `runtimeServer` whose runtime `cleanupTimedOutRuntime` just evicted, rather than
   re-resolving a fresh container via `getOrCreateRuntime` -- so each retry attempt is
   guaranteed to also time out (burning a full `timeout+grace` per attempt) rather than
   potentially succeeding on a fresh execution environment the way real AWS's retry
   would. The end state (eventual destination/DLQ delivery) is now correct; only the
   retry attempts' chance of succeeding is not. This pre-existing "retry reuses the
   same runtimeServer" design applies equally to plain function-error retries and was
   not introduced or changed by this fix.

Both regression tests drive the real bug via existing repo helpers
(`trackingDockerAPI`/`fakeAsyncDelivery`) rather than new mocks:
`TestCleanupTimedOutRuntime_SemSaturated_StillCleansUp` (`container_cleanup_test.go`)
saturates `b.cleanupSem` via a new `FillCleanupSem` export helper and a new
`CleanupTimedOutRuntimeForTest` export wrapper, then asserts the container is still
stopped; `TestEnqueueAsync_TimeoutDeliversToFailureDestination` (`async_invoke_test.go`)
starts a runtime server that never answers `/next` and asserts the configured
`OnFailure` destination is eventually invoked. Both confirmed failing against the
pre-fix code (hand-reverted via `git show HEAD:<path>`, not `git stash`, to avoid
disturbing a concurrent agent's uncommitted work elsewhere in the worktree) and passing
after; restored.

No other bugs found this pass. Also reviewed, no bug found: `enqueueAsyncInvocation`'s
fast/slow-path split and `asyncEnqueueWaiters` bound; `dispatchInvocationLog`'s
`logSem`; `EventSourcePoller`'s single-goroutine-for-the-whole-service design
(`sweepStaleIterators` correctly bounds `shardIterators`/`sqsBatchBuffers` growth on
delete); `Close()`'s poller-cancel + URL-server + runtime + `asyncWG.Wait()` shutdown
sequencing; `handleInvoke`'s status codes (202 Event, 204 DryRun,
X-Amz-Function-Error/X-Amz-Executed-Version/X-Amz-Log-Result headers) against
`awsRestjson1_deserializeOpHttpBindingsInvokeOutput`/`...DocumentInvokeOutput`
(lambda@v1.101.2 deserializers.go:8744-8793) -- `StatusCode` is read verbatim from the
HTTP response and all four headers are read exactly as this package sets them, no
mismatch.

Observed, not fixed (pre-existing, confirmed unrelated to this pass via hand-revert):
`go test -race -count=1 ./services/lambda/...` intermittently reports a `goleak`
failure (leftover `net/http.(*persistConn)` read/write-loop goroutines from
`http.DefaultClient`, used directly by several `_test.go` files e.g.
`handler_runtime_test.go`/`iam_enforcement_test.go`/`store_test.go`) -- reproduced on
unmodified `HEAD` (2 of 3 runs failed, 1 passed) as well as with this pass's fix
applied (1 of 3 runs failed), confirming it is a pre-existing test-infrastructure
timing flake (idle keep-alive connections not yet closed when `TestMain`'s
`goleak.VerifyTestMain` samples goroutines), not a production-code leak and not caused
by this pass. Not investigated further or fixed -- root-causing which specific test(s)
leave an idle connection open, and whether to add a `testleak` ignore or have those
tests call `CloseIdleConnections`, is a separate, broader change than this pass's two
targeted fixes.

Gates: `go build ./...`, `go vet ./services/lambda/...`, `gofmt -l services/lambda/`
(no output), `go test -race -count=1 ./services/lambda/...` (3/3 clean runs after the
fix). `golangci-lint run ./services/lambda/...` panics repo-wide on this toolchain
(`honnef.co/go/tools@v0.7.0` / goanalysis `buildir`/`nilness`/`typedness`/`fact_purity`
interface-conversion panics) -- confirmed pre-existing and environment-wide, not
scoped to this package or this pass's changes.

## 2026-09-06: Code.ImageUri never resolved against ECR (gopherstack-vrpy)

Confirmed real AWS validates an Image package-type function's `Code.ImageUri` at
`CreateFunction`/`UpdateFunctionCode` time, not only at pull time: a nonexistent
repository or tag is rejected immediately with `InvalidParameterValueException`
("Source image \<uri\> does not exist. Provide a valid source image."), reproduced
in multiple public bug reports (e.g. aws/aws-cdk#24648, aws/aws-sdk-go#3736). The
issue's own "possibly structural" caveat does not hold.

Added an optional cross-service seam, following `services/networkmanager`'s
`EC2Resolver` pattern exactly: `ECRResolver` (`crossservice.go`, one method,
`ResolveImage(imageURI string) bool`), a `SetECRResolver` setter and `ecrResolver`
field on `InMemoryBackend` (`store.go`), and an `ImageURIResolver` optional
extension of `StorageBackend` (alongside the existing `QualifierResolver` etc.)
that `Handler.validateImageURIResolves` type-asserts `h.Backend` against before
accepting a Image-type `CreateFunction`/`UpdateFunctionCode` call
(`handler_functions.go`). A nil resolver -- the default, and every existing test's
backend -- accepts every `ImageUri` unvalidated, matching this repo's convention
for unwired cross-service checks and preserving every pre-existing test that
creates an Image function with an arbitrary `ImageUri` like `"x"`.

**Not wired up**: cli.go is out of scope for this pass (owned by a concurrent
agent). Wiring needs a `wireLambdaECR(lambdaReg, ecrReg service.Registerable)`
function mirroring `wireLambdaS3`/`wireLambdaCWLogs`, plus an adapter type (e.g.
`lambdaECRResolverAdapter`) whose `ResolveImage(imageURI string) bool` parses the
account/region/repository/tag-or-digest out of the ECR-style URI and calls
`services/ecr`'s already-exported `InMemoryBackend.DescribeImages(ctx, repositoryName,
[]ImageIdentifier{...})`, returning `true` only on a nil error. No changes needed in
services/ecr itself -- `DescribeImages` already returns `ErrRepositoryNotFound`/
`ErrImageNotFound` for exactly this check. Call site: alongside the other
`wireLambda*` calls in cli.go's service-wiring sequence.

Regression tests (`ecr_resolver_test.go`): a fake `ECRResolver` proves both
directions (unknown image rejected with `InvalidParameterValueException` and not
persisted; known image accepted) for both `CreateFunction` and
`UpdateFunctionCode`, plus a no-resolver-wired test documenting the accept-all
default is unchanged. Each guard was neutered individually and confirmed to make
exactly its own regression test fail (and no others).
