---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: iotwireless
sdk_module: aws-sdk-go-v2/service/iotwireless@v1.59.4   # version audited against; bumped from v1.54.7 by gopherstack-jvqt (LoRaWAN/Sidewalk typing) -- no op surface changes between the two, only this manifest's citations needed updating
last_audit_commit: d1235ad5                              # HEAD when this full-audit pass was written; families updated piecemeal since (c2733f39a, gopherstack-jvqt) without a full re-audit
last_audit_date: 2026-08-23
overall: A                # all 4 prior gaps + 9 deferred families field-diffed and fixed this pass;
                           # 2026-08-23: corrected one stale gap claim, see gaps below
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  AssociateAwsAccountWithPartnerAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable — routed PUT /partner-accounts/{id}, real op is POST /partner-accounts with Sidewalk.AmazonId in body; fixed"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable — routed /tags/{arn} path segment, real op is bare POST /tags with resourceArn query param + []Tag body; fixed"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "same /tags routing fix as TagResource"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "same /tags routing fix; response now []Tag{Key,Value} not a bare map"}
  GetWirelessDevice: {wire: ok, errors: ok, state: ok, persist: ok, note: "ThingArn/ThingName were tracked by the backend but never surfaced — fixed; now also surfaces LoRaWAN/Sidewalk/Positioning"}
  GetWirelessGateway: {wire: ok, errors: ok, state: ok, persist: ok, note: "same ThingArn/ThingName fix as GetWirelessDevice; now also surfaces LoRaWAN"}
  StartBulkAssociateWirelessDeviceWithMulticastGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "was a no-op 204; now associates every device in the account/region with the group (emulates 'all qualifying devices' since there's no QueryString expression evaluator)"}
  StartBulkDisassociateWirelessDeviceFromMulticastGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "was a no-op 204; now clears the group's full device-association set"}
  DisassociateWirelessDeviceFromMulticastGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "routing silently discarded the {WirelessDeviceId} path segment, so calling it for any one device cleared ALL associations for the group; fixed via lastPathSegment() + per-device set removal"}
  DisassociateWirelessDeviceFromFuotaTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "same discarded-path-segment bug as DisassociateWirelessDeviceFromMulticastGroup; fixed"}
  DisassociateMulticastGroupFromFuotaTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "same discarded-path-segment bug; fixed"}
  StartFuotaTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "was corrupting FirmwareUpdateRole by overwriting it with a fabricated status string to fake state; FuotaTask now has a real Status field, transitioned Pending -> FuotaSession_Waiting. Now also parses StartFuotaTaskInput.LoRaWAN (types.LoRaWANStartFuotaTask, types.go:1202), previously unparsed entirely, into FuotaTask.StartTime; GetFuotaTask surfaces it via LoRaWANFuotaTaskGetInfo.StartTime, which was permanently nil before (gopherstack-pgvj)"}
  UpdateFuotaTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "was silently dropping Descriptor/FirmwareUpdateImage/FirmwareUpdateRole/FragmentIntervalMS/FragmentSizeBytes/RedundancyPercent -- only Name/Description/LoRaWAN were wired even though UpdateFuotaTaskInput (api_op_UpdateFuotaTask.go:28) carries all nine; a client updating any of the six got a 200 and no change. All six now applied (gopherstack-pgvj), verified byte-for-byte against a real aws-sdk-go-v2 client's serialized PATCH body"}
  AssociateWirelessDeviceWithFuotaTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-jqh2: STALE GAP CLOSED -- this row said unreachable (singular-vs-plural routing mismatch, gopherstack-pgvj), but routing.go now has a dedicated pathSubWirelessDevice=\"wireless-device\" (singular) constant matched by PUT in parseFuotaTaskSubPath, landed in d39bf33e4 without this PARITY.md row being updated. Re-verified reachable via TestExtractOperation_SDKRouteTable against the real PUT /fuota-tasks/{Id}/wireless-device path."}
  AssociateMulticastGroupWithFuotaTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-jqh2: STALE GAP CLOSED -- same as AssociateWirelessDeviceWithFuotaTask; routing.go's pathSubMulticastGroup=\"multicast-group\" (singular) constant already handles PUT /fuota-tasks/{Id}/multicast-group correctly, this row was simply never updated after the fix landed in d39bf33e4. Re-verified via TestExtractOperation_SDKRouteTable."}
families:
  WirelessDevice: {status: ok, note: "CRUD + Associate/Disassociate*, Deregister, statistics, data queue, test — all routes verified against real serializers.go SplitURI+Method; Tags wire shape fixed; LoRaWAN/Sidewalk/Positioning now stored and round-tripped (was gap); SendDataToWirelessDevice now captures TransmitMode (was silently dropped, queued messages always reported 0); DeleteWirelessDevice now cascade-cleans thing association, queued messages, and multicast/FUOTA group membership"}
  WirelessGateway: {status: ok, note: "CRUD + certificate/thing association, task, firmware/statistics — routes verified; Tags wire shape fixed; LoRaWAN now stored (GatewayEui/RfRegion/JoinEuiFilters/NetIdFilters/MaxEirp/SubBands/Beaconing) — Create nests it under LoRaWAN, Update's JoinEuiFilters/MaxEirp/NetIdFilters are top-level fields that merge into the same map; DeleteWirelessGateway now cascade-cleans thing/cert association and any pending gateway task"}
  DeviceProfile: {status: ok, note: "Tags wire shape fixed; LoRaWAN/Sidewalk now stored and returned on Get (types.LoRaWANDeviceProfile/SidewalkGetDeviceProfile were previously dropped entirely); List entries correctly narrowed to Arn/Id/Name only (types.DeviceProfile), matching real ListDeviceProfilesOutput. LoRaWAN/Sidewalk since typed (were map[string]any): LoRaWAN shared verbatim by Create/Get (types.LoRaWANDeviceProfile, types.go:780); Sidewalk splits into the empty types.SidewalkCreateDeviceProfile request (types.go:1715, no client-configurable fields) and the wider types.SidewalkGetDeviceProfile response (types.go:1796) whose AWS-assigned fields (ApplicationServerPublicKey/DakCertificateMetadata/QualificationStatus) this backend never fabricates"}
  ServiceProfile: {status: ok, note: "Tags wire shape fixed; LoRaWAN now stored and returned on Get (types.LoRaWANGetServiceProfileInfo); List entries correctly narrowed to Arn/Id/Name. LoRaWAN since typed (was map[string]any): create shape types.LoRaWANServiceProfile (types.go:1161, 9 fields) is genuinely narrower than the get shape types.LoRaWANGetServiceProfileInfo (types.go:933, 23 fields, e.g. DrMax is *int32 on create vs plain int32 on get) -- handler converts via loRaWANGetServiceProfileInfoFrom, leaving AWS-computed get-only fields unset rather than fabricated"}
  Destination: {status: ok, note: "Tags wire shape fixed; CRUD + Update routes verified; no CreatedAt field on the real wire shape (confirmed against GetDestinationOutput) so none added"}
  FuotaTask: {status: ok, note: "Tags wire shape fixed; Start/Update/Disassociate* verified. Field-diffed against GetFuotaTaskOutput and found genuinely missing: CreatedAt (epoch-seconds), Descriptor, FragmentIntervalMS, FragmentSizeBytes, RedundancyPercent, LoRaWAN, and a real Status field (StartFuotaTask was previously faking status by overwriting FirmwareUpdateRole) — all added. List entries correctly narrowed to Arn/Id/Name (types.FuotaTask); Get/List device+multicast-group associations upgraded from single-slot maps to real per-task sets (a task can have multiple of each) with cascade cleanup on delete. LoRaWAN since typed (was map[string]any): types.LoRaWANFuotaTask (types.go:844, RfRegion only) is shared by Create and Update, converted to the wider types.LoRaWANFuotaTaskGetInfo (types.go:853, adds StartTime) on Get; UpdateFuotaTask now also accepts LoRaWAN (previously silently dropped -- its request struct didn't even declare the field). gopherstack-pgvj closed the two remaining gaps this same family had flagged: UpdateFuotaTask now applies Descriptor/FirmwareUpdateImage/FirmwareUpdateRole/FragmentIntervalMS/FragmentSizeBytes/RedundancyPercent (previously silently dropped), and StartTime is now captured from StartFuotaTaskInput.LoRaWAN.StartTime (types.LoRaWANStartFuotaTask, types.go:1202) into a new FuotaTask.StartTime field instead of being permanently nil. Sibling-op audit while in there found AssociateWirelessDeviceWithFuotaTask/AssociateMulticastGroupWithFuotaTask unreachable due to a singular-vs-plural routing mismatch -- fixed in a later pass (routing.go's pathSubWirelessDevice/pathSubMulticastGroup) though this PARITY.md wasn't updated at the time; gopherstack-jqh2 corrected the stale rows and re-verified both reachable via TestExtractOperation_SDKRouteTable. Create/Get/List/Disassociate* all verified correct against a real aws-sdk-go-v2 client"}
  MulticastGroup: {status: ok, note: "Tags wire shape fixed. Field-diffed against GetMulticastGroupOutput and found genuinely missing: CreatedAt (epoch-seconds), Description, LoRaWAN — all added. Bulk associate/disassociate now mutate real per-group device-association sets (was gap); per-device disassociate now uses the real path-segment device ID instead of clearing everything; DeleteMulticastGroup cascade-cleans its device-association set and its FUOTA-task associations. LoRaWAN since typed (was map[string]any): types.LoRaWANMulticast (types.go:1043) is shared by Create and Update, converted to types.LoRaWANMulticastGet (types.go:1064, adds NumberOfDevicesInGroup/NumberOfDevicesRequested) on Get; NumberOfDevicesInGroup is a real count from the device-association set, NumberOfDevicesRequested stays unset (no separate 'requested' count exists in this backend). UpdateMulticastGroup now also accepts LoRaWAN (previously silently dropped)"}
  NetworkAnalyzerConfiguration: {status: ok, note: "Tags wire shape fixed. Field-diffed against GetNetworkAnalyzerConfigurationOutput/CreateNetworkAnalyzerConfigurationInput and found genuinely missing: TraceContent (LogLevel/MulticastFrameInfo/WirelessDeviceFrameInfo) and MulticastGroups — both were accepted by nothing and always empty; now stored and round-tripped through Create/Get/Update"}
  PartnerAccount: {status: ok, note: "AssociateAwsAccountWithPartnerAccount route+wire rewritten (see ops); Get/Update/Disassociate/List were already correct (PartnerAccountId as path parameter); ListPartnerAccounts previously iterated a Go map with no sort (non-deterministic order across identical calls) — now sorted by AmazonId and paginated"}
  Tags (TagResource/UntagResource/ListTagsForResource): {status: ok, note: "route + wire shape rewritten; see ops"}
  GatewayTask / GatewayTaskDefinition: {status: ok, note: "was deferred; field-diffed against GetWirelessGatewayTaskOutput/CreateWirelessGatewayTaskDefinitionOutput/GetWirelessGatewayTaskDefinitionOutput/ListWirelessGatewayTaskDefinitionsOutput. Found and fixed: TaskCreatedAt was always empty (GatewayTask never recorded a creation time) — now set on CreateWirelessGatewayTask and formatted as an ISODateTimeString (this field is a *string on the wire, not a smithy timestamp, confirmed via the deserializer using plain string decode); CreateWirelessGatewayTaskDefinition's Update object (LoRaWAN current/update firmware version, UpdateDataRole, UpdateDataSource) was silently accepted and dropped — now stored and returned on Get; ListWirelessGatewayTaskDefinitions entries wrongly included Name/AutoCreateTasks — real types.UpdateWirelessGatewayTaskEntry carries only Arn/Id/LoRaWAN, fixed; CreateWirelessGatewayTaskOutput doesn't model WirelessGatewayId, trimmed the response to match"}
  WirelessDeviceImportTask / SingleWirelessDeviceImportTask: {status: ok, note: "was deferred; field-diffed against GetWirelessDeviceImportTaskOutput/StartSingleWirelessDeviceImportTaskOutput. Found and fixed: CreationTime was completely missing from Get/List responses — added, formatted as an ISODateTimeString (smithytime.ParseDateTime — a string, NOT epoch-seconds, unlike FuotaTask/MulticastGroup's CreatedAt which IS epoch-seconds; confirmed by reading both deserializer branches directly rather than assuming a single convention service-wide). SingleWirelessDeviceImportTask has no dedicated Get operation in the real API (confirmed: no api_op_GetSingleWirelessDeviceImportTask.go exists) so the current create-only implementation is already complete, not partial"}
  Position / PositionConfiguration / PositionEstimate / ResourcePosition: {status: ok, note: "was deferred; field-diffed against GetPositionOutput and found a real bug: Accuracy was modeled as a bare *float64, but the real shape is types.Accuracy{HorizontalAccuracy, VerticalAccuracy} — a client would have failed to deserialize this field entirely. Fixed to the correct object shape. GetPositionConfiguration/PutPositionConfiguration/ListPositionConfigurations/GetPositionEstimate field names confirmed correct via opaque-map echo (Solvers/Destination). GetResourcePosition/UpdateResourcePosition's GeoJsonPayload is an httpPayload member, not a JSON-wrapped field — the SDK streams it as the ENTIRE raw request body (serializers.go:9182, SetStream) and reads the whole response body back into it (deserializers.go:7941, buf.Bytes()); fixed in 94b4f51ba to a raw application/octet-stream body instead of a {\"GeoJsonPayload\":...} JSON envelope. ListPositionConfigurations now paginated"}
  EventConfiguration: {status: ok, note: "was deferred; field-diffed against GetEventConfigurationByResourceTypesOutput/GetResourceEventConfigurationOutput/ListEventConfigurationsOutput — ConnectionStatus/DeviceRegistrationState/Join/MessageDeliveryStatus/Proximity field names confirmed correct; opaque-map echo of each nested *XxxEventConfiguration sub-object is faithful since these are simple enable/disable objects. ListEventConfigurations now paginated"}
  LogLevels: {status: ok, note: "was deferred; field-diffed against GetLogLevelsByResourceTypesOutput/UpdateLogLevelsByResourceTypesInput and found a real bug: FuotaTaskLogOptions/WirelessDeviceLogOptions/WirelessGatewayLogOptions were accepted by UpdateLogLevelsByResourceTypes and silently dropped — GetLogLevelsByResourceTypes always echoed empty arrays regardless of what was set. Fixed: backend now has a real LogLevelsConfig carrying all four fields. Also trimmed GetResourceLogLevelOutput to just LogLevel — the prior response fabricated ResourceType/ResourceId fields that aren't in the real output shape"}
  MetricConfiguration / GetMetrics: {status: ok, note: "was deferred; field-diffed against GetMetricConfigurationOutput/GetMetricsOutput — SummaryMetric.Status and the per-query QueryId/QueryStatus/MetricName echo are field-name-correct. This backend doesn't ingest telemetry to aggregate real Values/Dimensions/Timestamps, so GetMetrics intentionally returns an empty Values array per query rather than fabricating aggregation results — this is a documented, deliberate partial-fidelity emulation, not a stub (no fabricated data is ever returned)"}
  ServiceEndpoint: {status: ok, note: "field-diffed against GetServiceEndpointOutput — ServiceType/ServiceEndpoint/ServerTrust match exactly; no changes needed"}
  SendDataToWirelessDevice / SendDataToMulticastGroup / QueuedMessages: {status: ok, note: "was deferred; field-diffed against SendDataToWirelessDeviceInput/DownlinkQueueMessage. Found and fixed: TransmitMode was accepted by SendDataToWirelessDeviceInput but never captured into the queued QueuedMessage, so every ListQueuedMessages entry reported TransmitMode 0 regardless of what was sent — now captured. DownlinkQueueMessage's MessageId/ReceivedAt/TransmitMode field names confirmed correct (LoRaWAN sub-object omitted, matching the no-fabrication principle: this backend has no router metadata to report). SendDataToMulticastGroup confirmed already minimal-and-correct: real AWS also returns only {MessageId}, and there is no reachable read-back API for multicast group sent data"}
  errors (global): {status: ok, note: "writeError now sets X-Amzn-Errortype header + __type body field derived from HTTP status (404->ResourceNotFoundException, 400->ValidationException, 403->AccessDeniedException, 409->ConflictException, 429->ThrottlingException, else->InternalServerException). Every error path in the service routes through writeError, so this is a single-point fix covering all ops."}
  pagination (List operations): {status: ok, note: "was gap; every List* op (ListWirelessDevices, ListWirelessGateways, ListServiceProfiles, ListDeviceProfiles, ListDestinations, ListFuotaTasks, ListMulticastGroups, ListMulticastGroupsByFuotaTask, ListNetworkAnalyzerConfigurations, ListPositionConfigurations, ListEventConfigurations, ListPartnerAccounts, ListWirelessGatewayTaskDefinitions, ListWirelessDeviceImportTasks, ListQueuedMessages) now honors maxResults/nextToken via a shared paginateQuery helper (pkgs/page), against a deterministically sorted slice"}
  locking (InMemoryBackend): {status: ok, note: "was gap; InMemoryBackend.mu is now *lockmetrics.RWMutex (was a raw sync.RWMutex), matching the project's coarse-instrumented-lock convention. All ~110 Lock()/RLock() call sites across every <family>.go file were labeled with their enclosing method name as the metrics operation label"}
deferred: []                # none — every family from the prior pass was field-diffed this pass; see families above
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "STALE, CORRECTED 2026-08-23 (found already fixed in code, not reflected in this file -- same pattern as gopherstack-jqh2 below): this entry claimed ListWirelessDevices doesn't implement the DestinationName/DeviceProfileId/ServiceProfileId/FuotaTaskId/MulticastGroupId/WirelessDeviceType query-parameter filters. All six are fully implemented: handler_wireless_devices.go's listWirelessDevices reads all six query params into a ListWirelessDevicesFilter (wireless_devices.go), whose matches() method checks every one of them (DeviceProfileId across both LoRaWAN and Sidewalk via hasDeviceProfileID; FuotaTaskId/MulticastGroupId via the b.fuotaTaskDevices/b.multicastGroupDevices membership maps this note itself predicted could back them). Covered end-to-end, including AND-combination semantics, by TestHandler_ListWirelessDevices_Filters (handler_wireless_devices_filter_test.go, 12 subtests, all passing) -- verified again this pass via a real HTTP round-trip. git blame dates the filter code to commit d39bf33e4 (2026-08-11), two days before this file's last_audit_date at the time (2026-08-13); the audit that produced this claim either predates that commit's landing in its working tree or simply never re-checked after. No code change needed -- this file was wrong, not the backend."
  # gopherstack-jqh2: the AssociateWirelessDeviceWithFuotaTask/AssociateMulticastGroupWithFuotaTask
  # singular-vs-plural routing gap formerly listed here was found already fixed in code
  # (routing.go's pathSubWirelessDevice/pathSubMulticastGroup, landed d39bf33e4) but never
  # reflected in this file. Corrected in ops/families above; TestExtractOperation_SDKRouteTable
  # (handler_paths_sdk_diff_test.go) now guards all 112 real ops against regressing.
leaks: {status: clean, note: "no goroutines/janitors in this service; all state is plain in-memory maps/store.Table under the single mu *lockmetrics.RWMutex, released on Reset(). DeleteWirelessDevice/DeleteWirelessGateway/DeleteMulticastGroup/DeleteFuotaTask now cascade-clean every dependent association map (thing associations, queued messages, multicast/FUOTA membership sets, gateway tasks) so no ghost row survives a parent resource's deletion — this was NOT the case before this pass. FIXED (gopherstack-8907, 2026-09-06): DeleteWirelessDevice/DeleteWirelessGateway also missed the positions map (GetPosition/UpdatePosition). GetPosition has no existence check, so it still returned the stale position for a deleted device/gateway's own ID, and positions is persisted verbatim in Snapshot() regardless (device/gateway IDs are uuid.NewString(), so this is unbounded growth rather than a wrong-answer-on-recreate case). Now cleared in both delete paths. See TestDelete_ClearsPosition."}
---

## Notes

Freeform: AWS-behavior specifics worth remembering, and any "looks-wrong-but-correct"
traps so the next auditor doesn't re-flag them.

- **Protocol**: restjson1 (REST paths, not X-Amz-Target header dispatch). Field names on
  the wire are **PascalCase** ("Arn", "Id", "Name", ...) — this is unusual among AWS
  REST-JSON services (most use camelCase) but is IoT Wireless's real wire shape, confirmed
  against `aws-sdk-go-v2/service/iotwireless@v1.54.7`'s generated (de)serializers. Do not
  "fix" this to camelCase.

- **Tags wire shape**: every op that accepts/returns `Tags` on IoT Wireless — every
  `Create*`, `AssociateAwsAccountWithPartnerAccount`, `TagResource`,
  `ListTagsForResource` — uses `[]Tag{Key,Value}` (an array of key/value objects), **never**
  a bare `{"k":"v"}` JSON object map. Before this audit every Tags field in this handler was
  typed `map[string]string`, which fails to unmarshal a real client's `[{"Key":...}]` array
  (the whole request 400s, not just the Tags field — encoding/json aborts the struct decode
  on any field type mismatch it can't skip past cleanly here since the check is
  `if err != nil { return 400 }` immediately after Unmarshal). Fixed via
  `tags_wire.go`'s `tagKVsToMap`/`tagMapToKVs`, backed by `pkgs/tags.KV`. If you add a new Tags
  field to any future op, use `[]tags.KV`, not `map[string]string`.

- **`/tags` is a bare, fixed path** — `POST|GET|DELETE /tags`, never `/tags/{arn}`. The
  resource ARN travels as the `resourceArn` query parameter (confirmed via
  `awsRestjson1_serializeOpHttpBindingsTagResourceInput` etc., which call
  `encoder.SetQuery("resourceArn")`, never a URI path segment). `UntagResource`'s `tagKeys`
  is also a query parameter (repeated `tagKeys=a&tagKeys=b`), which was already handled
  correctly.

- **`AssociateAwsAccountWithPartnerAccount` is `POST /partner-accounts` with no path
  parameter** — the partner account ID is `Sidewalk.AmazonId` in the JSON body. This is the
  one partner-accounts op that does NOT bind `PartnerAccountId` as a `{PartnerAccountId}`
  URI segment; `GetPartnerAccount`/`UpdatePartnerAccount`/
  `DisassociateAwsAccountFromPartnerAccount` all do bind it as a path segment and were
  already correct before this audit.

- **`GetWirelessDevice`/`GetWirelessGateway` include `ThingArn`/`ThingName`** — derived from
  the IoT Thing ARN's last `/`-separated segment (`thingNameFromArn` in handler.go), since
  `AssociateWirelessDeviceWithThing`/`AssociateWirelessGatewayWithThing` requests only ever
  carry `ThingArn`, never `ThingName` — AWS derives the name itself. Neither the request nor
  the backend need to store ThingName separately.

- **Error responses need `X-Amzn-Errortype`** — aws-sdk-go-v2's REST-JSON error
  deserializer (`awsRestjson1_deserializeOpError*`) picks the modeled exception type
  (`ResourceNotFoundException`, `ValidationException`, ...) from the `X-Amzn-Errortype`
  response header first, falling back to a `code`/`__type` field in the JSON body
  (`restjson.GetErrorInfo`). Before this audit neither was ever set, so every error from
  this service — including plain 404s — deserialized into an untyped
  `smithy.GenericAPIError{Code: "UnknownError"}` client-side, breaking any
  `errors.As(err, &types.ResourceNotFoundException{})` handling (waiters, retries, most
  application code). Fixed centrally in `writeError`/`awsErrorType` in handler.go — every
  error path in this service already funneled through `writeError`, so this was a
  single-function fix with service-wide effect. Any new error path MUST use `writeError`,
  not a bare `c.JSON`/`WriteHeader`, or it will regress this fix.

- **CreateWirelessDevice/CreateWirelessGateway et al. return only `{Arn, Id}`** (or
  `{Arn, Name}` for name-keyed resources) — this is correct; real AWS's Create*Output shapes
  genuinely omit every other field (confirmed against `api_op_CreateWirelessDevice.go` etc.).
  Do not "fix" this to return the full resource — that would itself be a wire-shape bug in
  the other direction.

- **DeleteMulticastGroup et al. issuing 204 without touching state you'd expect** (e.g.
  `StartBulkAssociateWirelessDeviceWithMulticastGroup`) are intentionally left as documented
  gaps above, not silently "fixed" to fabricate bulk-task tracking that doesn't otherwise
  exist in this backend — see gaps.

- **Two different timestamp wire formats coexist in this service — read the deserializer,
  don't assume one convention.** `FuotaTask.CreatedAt` / `MulticastGroup.CreatedAt` are
  epoch-seconds JSON numbers (`smithytime.ParseEpochSeconds`, via `pkgs/awstime.Epoch`).
  `WirelessDeviceImportTask.CreationTime` / `GetWirelessGatewayTaskOutput.TaskCreatedAt` /
  `GetMulticastGroupSessionOutput.LoRaWAN.SessionStartTime` are ISO8601 **strings**
  (`smithytime.ParseDateTime`, plain `*string` on the Go SDK type, formatted here with
  `time.RFC3339`). `LoRaWANFuotaTaskGetInfo.StartTime` (types.go:853) is the same
  ISO8601-string convention despite sitting right next to `FuotaTask.CreatedAt`'s
  epoch-seconds field in the same response — the split is genuinely per-field, not per-op
  or per-family — confirmed by reading each `awsRestjson1_deserializeOpDocument*Output`
  switch case directly. Do not "fix" one to match the other.

- **LoRaWAN/Sidewalk/Update/TraceContent nested config objects are individually typed**
  (lorawan_types.go), not opaque `map[string]any` — the last four holdouts
  (ServiceProfile/DeviceProfile/FuotaTask/MulticastGroup) were typed by gopherstack-jvqt,
  following WirelessDevice/WirelessGateway/GatewayTaskDefinition/NetworkAnalyzerConfiguration
  from c2733f39a. Each family's create shape and get shape are modeled as separate Go
  types where the SDK genuinely defines separate types (e.g. `LoRaWANServiceProfile` vs
  `LoRaWANGetServiceProfileInfo`), never merged into one wider struct — a `copyLoRaWANXxx`
  helper isolates the backend's stored pointer on read (mirroring `newTagsCopy`'s isolation
  for Tags), and a `loRaWANXxxFrom`/`sidewalkXxxFrom` converter builds the wider Get shape
  from the narrower stored Create/Update shape without fabricating AWS-computed fields.
  `UpdateFuotaTask`/`UpdateMulticastGroup`'s `LoRaWAN` uses the *same* type as Create (per
  the SDK) and replaces the stored value wholesale rather than merging, unlike
  `LoRaWANUpdateDevice`-style Update shapes elsewhere in this service which are narrower
  than Create and do merge field-by-field.

- **List entries are narrower than Get responses for several families** — real AWS list
  operations often return a stripped-down per-item type (e.g. `types.FuotaTask` /
  `types.DeviceProfile` / `types.ServiceProfile` carry only `Arn`/`Id`/`Name`, while
  `types.UpdateWirelessGatewayTaskEntry` carries only `Arn`/`Id`/`LoRaWAN`) even though the
  singular Get operation for the same resource returns many more fields. Each handler now
  uses a dedicated `*ListEntry`/`taskDefEntry` DTO for List responses, separate from the Get
  DTO — do not consolidate them, that would reintroduce over-inclusive list wire shapes.

- **`{WirelessDeviceId}`/`{MulticastGroupId}` trailing path segments are not carried by
  `parseIoTWirelessPath`'s `(op, resource) string` return** — that function only ever
  returns the top-level `{Id}` path parameter. Per-item disassociate handlers
  (`DisassociateWirelessDeviceFromMulticastGroup`, `DisassociateWirelessDeviceFromFuotaTask`,
  `DisassociateMulticastGroupFromFuotaTask`) recover the trailing sub-resource ID directly
  from the request URL via `lastPathSegment(c)` in routing dispatch (handler.go). Before
  this fix, calling disassociate for any one device/group silently cleared the *entire*
  association set for the parent resource, since the specific child ID was never read.

- **Association state (multicast-group↔device, FUOTA-task↔device, FUOTA-task↔multicast-group)
  is a set, not a single slot** — `map[string]map[string]bool` (`multicastGroupDevices`,
  `fuotaTaskDevices`, `fuotaTaskMulticast` in store.go), backing `ListMulticastGroupDeviceIDs`
  / `ListFuotaTaskDeviceIDs` / `ListMulticastGroupsByFuotaTask`. A prior
  `map[string]string` implementation silently dropped every association but the most
  recently added one for a given parent ID. `backendSnapshot`'s JSON shape for these three
  fields changed accordingly (object-of-arrays, not object-of-strings) —
  `iotwirelessSnapshotVersion` was bumped 1→2 so an old snapshot is cleanly discarded
  instead of partially misdecoded.

- **2026-08-31, gopherstack-uox6 (value-semantics sweep, first pass on this service for
  this class)**: this file's existing `ops`/`families` grades above are wire-shape audits
  (field exists, is read, round-trips) — a separate axis from whether a filter's
  documented VALUE semantics are honored once read. `cmd/covledger -service iotwireless`
  reported no rows (never swept for any bug class) going into this pass; no contradicting
  evidence found in git log. Checked all 17 List/Describe ops' filter parameters against
  their own SDK doc comments (`aws-sdk-go-v2/service/iotwireless@v1.59.4`). Two real bugs
  found and fixed:
  - `ListDeviceProfiles`'s documented `deviceProfileType` filter
    (`api_op_ListDeviceProfiles.go`, "A filter to list only device profiles that use this
    type, which can be LoRaWAN or Sidewalk") was never read by the handler at all — every
    call returned every profile regardless of the filter. Fixed: `profiles.go`'s
    `ListDeviceProfiles` now takes a `deviceProfileType` param and matches on which of
    the profile's `LoRaWAN`/`Sidewalk` sub-objects is set (a profile has exactly one,
    never both, since `CreateDeviceProfile` accepts only one).
  - `ListEventConfigurations`'s `resourceType` filter (enum:
    `SidewalkAccount|WirelessDevice|WirelessGateway`) matched against the entry's stored
    `IdentifierType` (a DIFFERENT enum: `PartnerAccountId|DevEui|GatewayEui|
    WirelessDeviceId|WirelessGatewayId`) using a same-string-prefix check. This
    accidentally worked for `WirelessDeviceId`/`WirelessGatewayId` but silently excluded
    `DevEui`/`GatewayEui` (LoRaWAN-EUI-identified devices/gateways) from their resource
    type entirely, and NEVER matched `SidewalkAccount` (`PartnerAccountId` shares no
    prefix with `SidewalkAccount`) — filtering by SidewalkAccount always returned empty.
    Fixed via `eventResourceTypeIdentifierTypes` (event_configurations.go), an explicit
    mapping grounded in the SDK's own `IdentifierType`/`EventNotificationPartnerType` enum
    definitions (`PartnerType` has exactly one legal value, "Sidewalk", so
    `PartnerAccountId` unambiguously means SidewalkAccount).

  Two gaps recorded, not fixed:
  - `ListWirelessGatewayTaskDefinitions`'s `taskDefinitionType` filter is never read.
    Left alone: `types.WirelessGatewayTaskDefinitionType` has exactly ONE legal value
    ("UPDATE"), and this backend's `GatewayTaskDefinition` has no type-selecting field at
    all — `CreateWirelessGatewayTaskDefinitionInput` only ever creates an Update-type
    definition. No legal filter value could ever change the result.
  - `ListDevicesForWirelessDeviceImportTask`'s `status` filter is never read. Left alone:
    the handler (`handler_certificates.go`) returns an unconditionally empty
    `ImportedWirelessDeviceList ([]struct{})` regardless of input — this backend tracks no
    per-device import records to filter over, so no legal value could change the result.
    (The underlying "no per-device import records modeled" gap is a separate, structural
    axis, not this one — recorded here only for the filter's own consequence.)

  One item recorded as a different axis (validation, not semantics): `ListQueuedMessages`'s
  `wirelessDeviceType` parameter is never read. The device is already uniquely identified
  by the required `Id` path parameter (with its own fixed, already-known type), so this
  parameter cannot narrow a multi-device result — its only plausible real-AWS role is
  validating that the caller's stated type matches the device's actual type, which is a
  missing-rejection/validation concern, not a filter-semantics one.

  `ListPositionConfigurations`'s `resourceType` filter (positioning.go) was checked and is
  correct (exact match against the same two-value enum on both write and read paths).
  `pagination.go`'s documented-default-page-size reasoning (no single default page size is
  documented across every List* op in this SDK) was reconfirmed against every op's doc
  comment; no numeric default is stated anywhere, so the existing choice stands unchanged.

  2026-08-31 error-envelope-shape sweep (gopherstack-6flj/gopherstack-uox6
  axis), CONFIRMED CLEAN, no code changes. `covledger` had no
  `error_envelope_shape`/`fabricated_error_code` row for this service, but
  the `ops:` block above's `errors: ok` entries and this file's own "errors
  (global)" row already document a prior fix: `writeError` derives a single
  `X-Amzn-Errortype` from the HTTP status (404/400/403/409/429/else), and
  every error path in the service routes through it — a genuine single-point
  fix, not merely a claim.

  Re-derived rather than trusted: extracted every op's declared error codes
  from the pinned `iotwireless@v1.59.4/deserializers.go` (112 restjson1
  ops, confirmed per-op via `awsRestjson1_deserializeOpError<Op>`, not
  assumed uniform) and diffed against `awsErrorType`'s fixed six-code
  vocabulary. 17 ops declare no ResourceNotFoundException and 1
  (`GetEventConfigurationByResourceTypes`) declares no ValidationException;
  traced every one back to source and confirmed none of the 18 ever
  triggers `isNotFound`/`ErrValidation` in its own handler (they're
  Create/List/singleton-config ops with no not-found or validation-error
  path at all) — so the mismatch the declared-set diff raises is never
  actually reachable. ConflictException/AccessDeniedException/
  ThrottlingException are declared in `awsErrorType` but never triggered by
  any handler (grepped for `StatusConflict`/`StatusForbidden`/
  `StatusTooManyRequests` outside `handler.go`'s own switch — zero hits),
  so those branches are dead but not wrong. No handler bypasses `writeError`
  (grepped for `X-Amzn-Errortype`/`__type` outside `handler.go` — zero
  hits), so no fabricated-code path exists either. `errcodeaudit`
  (gopherstack-r3pr/r08q) independently reports zero findings for this
  service, confident or needs-review.

  Both verdicts hold. Effort for this pass went to `services/bedrock`
  instead, which had no equivalent prior fix and four real bugs on this
  axis (see its own PARITY.md, same date).

## 2026-09-08: writeError/writeJSON/handleError nil-on-write fall-through sweep (gopherstack-246v) -- clean

Part of the 12-service sweep for the elasticache class bug (gopherstack-8haq): a helper
that rejects a request via the local response writer and *returns* that writer's result
hands a caller doing `if err != nil { return err }` a `nil`, since the writer returns nil
after a successful write -- the rejection is silently skipped and the operation continues.

**Base writers**: `writeError` (`handler.go:955`) and `writeJSON` (`handler.go:968`) both
write directly via `c.Response()` and unconditionally `return nil`; `handleError`
(`handler.go:1004`) dispatches to `writeError` on every branch.

**Method (mechanical).** A `go/parser`/`go/ast` script over every non-test `.go` file (20
files) found every function with a `return`-statement whose result is a bare call to one
of the three base writers, then fixed-point-expanded to any function bare-returning a call
to an already-found member. This discovered 128 functions: `handleError` itself, all ~70
per-operation `handleXxx`/verb-named handlers (`createDestination`, `getWirelessDevice`,
...), and every `dispatchXxx` routing function.

**Dispatch verified, not assumed.** This service's dispatch is a multi-level `(bool,
error)` chain: `Handler()` -> `dispatch` -> `dispatchCoreOps` -> per-family
`dispatchWirelessDevice`/`dispatchWirelessGateway`/etc., down to leaf switches over `op`
that `return true, h.someOpHandler(...)`. Every intermediate level uses the uniform
`if handled, result := h.dispatchX(...); handled { return true, result }` shape -- it
branches on the `handled` bool, never on `result != nil`, so a sub-dispatcher that
"handled" a rejected op by writing an error and returning `nil` still propagates
correctly (handled=true short-circuits with a bare `return`); the value of `result` is
never inspected on the miss path either, only discarded to try the next family. Read all
16 such branch sites (`handler.go:472-865`) confirming the pattern holds with zero
exceptions -- none stores an err/result and branches on `!= nil`.

Every call site of the 3 base writers plus all 128 discovered wrapper functions across the
package was enumerated: 294 total. 278 are direct `return writeError(...)` / `return
writeJSON(...)` / `return handleError(...)` / `return h.handleXxx(...)` sites; the other 16
are the `(handled, result)` dispatch-chain assigns above, verified safe. Zero `_ =`
discards, zero instances of a stored single-value error checked with `if err != nil`.
Independently confirmed by grepping every non-test-file occurrence of
`writeError(`/`writeJSON(`/`handleError(` outside their own definitions: every one is
immediately preceded by `return` on the same line.

**No instance of the broken shape exists in iotwireless.** No code changed. Gates re-run
for the record: `GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/iotwireless/...` 0
issues; `GOTOOLCHAIN=go1.27.0 go test -race ./services/iotwireless/...` ok.
