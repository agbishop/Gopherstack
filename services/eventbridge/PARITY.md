---
service: eventbridge
sdk_module: aws-sdk-go-v2/service/eventbridge@v1.53.0
sibling_sdk_modules: [aws-sdk-go-v2/service/pipes@v1.26.4, aws-sdk-go-v2/service/schemas@v1.37.4]  # Pipes and Schema Registry ops this Handler also implements; see schema_registry_and_pipes below
last_audit_commit: b72533e7a
last_audit_date: 2026-08-07
overall: A
# 2026-08-30 wrapper-key sweep (uncommitted as of this note): type-aware
# go/types field-usage scan (302 exported fields across all 40 *Input/*Request
# structs, identity-matched not name-matched) flagged 2 fields with no read
# anywhere in the package: DescribeCodeBindingInput.SchemaVersion (real bug,
# fixed -- see its ops: entry) and UpdateSchemaInput.ClientTokenId (idempotency
# token, no real CreateSchemaInput counterpart either, left as a documented
# gap). Re-confirmed the Schemas sub-service's second REST-JSON1 handler path
# (handler_schemas_rest.go) against the pinned SDK -- still the only reachable
# path for schemas.Client, JSON-RPC dispatch table still dead scaffolding per
# existing notes below. buildOps() merges 8 submaps into 78 real ops (counted
# via a temporary test on len(h.ops), deleted after); re-verified filter/sort/
# paginate ordering (listNamedItems and its callers) is uniformly
# filter-then-sort-then-paginate with unique sort keys -- consistent with the
# 2026-08-30 sort-totality sweep below, no new issue found.
# 2026-08-30 sort-totality sweep (Class F: a sort that exists but is not total,
# and Class G: parallel result lists truncated independently). Reviewed every
# sort.Slice/sort.Strings call site across every paginated listing (archives/
# replays via the shared listNamedItems helper, connections, endpoints,
# event_buses, event_sources, api_destinations, schemas/registries/discoverers,
# rules, targets). Every reachable-by-real-traffic one sorts on that resource's
# own real unique Name/ID (or, for ListSchemas/SearchSchemas, SchemaName scoped
# to one registry's own per-registry map, itself the map key). One genuine
# Class F shape found: ListCodeBindings (schemas.go) sorts solely on Language,
# which is not unique (the same language can appear across multiple
# SchemaVersions of one schema) -- but this op is deliberately NOT advertised
# in GetSupportedOperations (see handler_dispatch.go's comment: no such method
# exists on any version of aws-sdk-go-v2/service/schemas.Client; checking a
# binding's status is DescribeCodeBinding, per-language, one at a time -- there
# is no real list-all-bindings operation), so it is unreachable by any real AWS
# SDK client and left as internal-only test scaffolding, same resolution as
# ram's ListTagsForResource. Not fixed (matches this service's own established
# precedent for unreachable internal-only routes); flagged here for visibility.
# Confirmed no listing reachable by real traffic in this service returns
# two-or-more collections the API defines as one ordered sequence truncated
# independently. No code changes.
ops:
  CreateEventBus: {wire: ok, errors: ok, state: ok, persist: ok, note: "name length/prefix validation, 200-per-account custom-bus limit enforced across regions. FIXED this sweep: CreateEventBusOutput was missing Description (real AWS echoes it); LastModifiedTime now set at creation (was zero-valued, only set by UpdateEventBus)."}
  DeleteEventBus: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades rule/target/index cleanup; default bus protected"}
  ListEventBuses: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep -- see DescribeEventBus (same eventBusResponse DTO backs both)."}
  DescribeEventBus: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep, two bugs: (1) handler returned the raw *EventBus struct via json.Marshal, so CreatedTime serialized as an RFC3339 string -- real AWS's DescribeEventBusOutput.CreationTime is an awsjson1.1 epoch-seconds number; a real SDK client's deserializer would reject the RFC3339 form. (2) EventBus had no Policy field at all, so a policy set via PutPermission/PutEventBusPolicy was invisible on Describe/List despite the prior sweep's own note claiming 'DescribeEventBus.Policy is the real wire path for reading a bus policy' -- that claim was aspirational, not verified; the field didn't exist. Added eventBusResponse handler DTO (epoch-seconds CreationTime/LastModifiedTime, Policy resolved from the backend's policy store at response time) used by both DescribeEventBus and ListEventBuses. STALE as of 2026-08-23 (manifest-harvest pass): the 'KmsKeyIdentifier/DeadLetterConfig/LogConfig NOT added' claim below is no longer true -- commit 69bbb940a (2026-08-15, #2417, 'three accepted-then-ignored params') added all three end to end (EventBus struct fields, event_buses.go's Create/UpdateEventBus wiring, handler_event_buses.go's response DTOs). Confirmed via a real aws-sdk-go-v2/service/eventbridge client CreateEventBus->DescribeEventBus round trip (TestEventBus_KmsDlqLogConfig_RealSDKClient, handler_event_buses_real_client_test.go): all three fields round-trip correctly. See items_still_open for the correction."}
  UpdateEventBus: {wire: ok, errors: ok, state: ok, persist: ok, note: "now sets LastModifiedTime on every update (previously never touched after creation, so it was permanently equal to CreatedTime even after a real edit)."}
  PutRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "EventPattern/ScheduleExpression mutual exclusivity + at-least-one enforced; 300-per-bus rule limit; ScheduleExpression validated via parseScheduleExpression. FIXED this sweep: PutRuleInput.ManagedBy had a JSON tag (json:\"ManagedBy,omitempty\"), so any client sending `\"ManagedBy\":\"...\"` in a PutRule request body could forge a rule as AWS-service-managed -- real AWS's PutRuleInput has no such wire member at all (server-populated, Describe/List-only). Changed the tag to json:\"-\" (wire-unreachable now, proven by TestPutRule_ManagedByNotWireSettable) while keeping the Go field as an internal same-process seeding hook (TestPutRule_ManagedByPreserved). Also added the ManagedRuleException enforcement the prior sweep flagged as a known gap (gopherstack-ba7): PutRule on an already-managed rule now returns ManagedRuleException instead of silently overwriting it. FIXED (gopherstack-ce1n): State was accepted and stored unvalidated -- an unrecognized value (e.g. a typo) silently became a rule that never matches ruleMatchesForDelivery's `rule.State != \"ENABLED\"` check and never fires, with no error at PutRule time. types.RuleState.Values() (aws-sdk-go-v2/service/eventbridge@v1.48.4 types/enums.go) has three values, not the obvious two: ENABLED, DISABLED, and ENABLED_WITH_ALL_CLOUDTRAIL_MANAGEMENT_EVENTS (a managed-rule CloudTrail state). PutRule's own modeled error set (awsAwsjson11_deserializeOpErrorPutRule, deserializers.go) does not list a specific validation exception for this, but this codebase's own precedent (CreateConnection's AuthorizationType enum check) already rejects an invalid enum with InvalidParameterException, so PutRule now does the same via isValidRuleState -- an omitted State still defaults to ENABLED as before (unaffected by the new check, since it only runs when State is non-empty). See TestPutRule_ValidatesStateEnum in rules_test.go."}
  DeleteRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: ManagedBy now enforced -- returns ManagedRuleException for a service-managed rule instead of deleting it. Was gopherstack-ba7."}
  ListRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-28 write-only-state sweep: added ruleListEntry/toRuleListEntry (handler_rules.go) so ListRules no longer marshals the shared Rule struct directly -- see DescribeRule for why Rule gained a CreatedBy field that real types.Rule (ListRulesOutput's item shape) does not have."}
  DescribeRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-28 write-only-state sweep: the backend has always tracked its own account ID (InMemoryBackend.accountID, used to build every rule's ARN via ruleARN), but Rule had no CreatedBy field at all, so DescribeRuleOutput.CreatedBy (real SDK: aws-sdk-go-v2/service/eventbridge@v1.48.4 api_op_DescribeRule.go) was always nil on a real client -- same bug class as KMS's already-fixed DescribeKey.AWSAccountId gap. CreatedBy is DescribeRule-only (real types.Rule, backing ListRulesOutput, has no such member), so ListRules needed a narrower ruleListEntry DTO to avoid inventing the field there. Fixed: PutRule now sets CreatedBy from b.accountID on creation (preserved, not overwritten, on a subsequent PutRule update of the same rule -- matches real AWS's 'creator', not 'last editor', semantics). See TestPutRule_CreatedBy_DescribeOnly_RealClient in wire_field_fixes_test.go."}
  EnableRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: ManagedBy now enforced (ManagedRuleException). Was gopherstack-ba7."}
  DisableRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: ManagedBy now enforced (ManagedRuleException). Was gopherstack-ba7."}
  PutTargets: {wire: ok, errors: ok, state: ok, persist: ok, note: "Target models all target-type-specific parameter structs (see prior-sweep note below), required-field validation, RetryPolicy bounds, 5-targets-per-rule limit. FIXED this sweep: ManagedBy now enforced (ManagedRuleException) -- was gopherstack-ba7, and was previously not even checked since PutTargets only did a busRules.Has() existence check, never fetched the Rule to inspect it. FIXED (gopherstack-9iva, delivery-side, PutTargets itself unchanged): an event-bus-ARN target (arn:{partition}:events:{region}:{account}:event-bus/{name}) was accepted -- correctly, real AWS's PutTargetsInput.Target.Arn has no ARN-shape/service validation at all client- or server-side beyond required-field checks (verified: validateTarget in aws-sdk-go-v2/service/eventbridge@v1.48.4 validators.go only checks Id/Arn presence and the target-type-specific *Parameters structs; api_op_PutTargets.go's own doc comment documents an event bus as a first-class target: 'Set that account's event bus as a target of the rules in your account. To send the matched events to the other account, specify that account's event bus as the Arn value when you run PutTargets') -- but delivery.go's deliverToTarget switch had no case for it, so the event was silently dropped at fan-out with only a debug-level 'unsupported target ARN type' warning. Added an EventBusRouter delivery target (self-registered by the backend in SetDeliveryTargets, same pattern as APIDestinations) whose RouteEventToBus re-enters PutEvents for the target bus so it gets real rule matching/fan-out, not a blind forward -- Input/InputPath/InputTransformer are intentionally NOT applied for this target type (the routed payload is the raw event, matching the SDK doc's note that these overrides are unavailable for a cross-account event-bus target). Same-account routing (this backend's only account, any of its regions) is fully implemented; true cross-account routing is structural -- see gaps below. A hop counter (context-carried, max 5) guards against a rule that (directly or via a cycle) targets its own bus, which would otherwise recurse forever since this backend runs every hop synchronously in-process. See TestDelivery_EventBusTarget_RoutesToTargetBusRules, TestDelivery_EventBusTarget_UnmatchedRuleOnTargetBusNoDelivery, TestRouteEventToBus_CrossAccountDropped, TestRouteEventToBus_HopLimitStopsLoop (delivery_event_bus_test.go)."}
  RemoveTargets: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: ManagedBy now enforced (ManagedRuleException) -- was gopherstack-ba7."}
  ListTargetsByRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "round-trips all target-type-specific parameters (see PutTargets)"}
  ListRuleNamesByTarget: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): Limit was parsed into the request but never consulted -- always used the shared paginate() fixed-100-page helper instead of the sized paginateN() sitting beside it in accessors.go. Now threads limit through; NextToken was already correctly returned, just at the wrong page size."}
  PutEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "1-10 entries-per-request limit and per-entry required-field validation for Source/DetailType/Detail (prior sweep). FIXED this sweep, severe: PutEventsRequestEntry.Time is an awsjson1.1 epoch-seconds JSON number on the real wire (confirmed against aws-sdk-go-v2/service/eventbridge's serializers.go: `ok.Double(smithytime.FormatEpochSeconds(*v.Time))`), but EventEntry.Time was a plain `*time.Time` with no custom unmarshal -- Go's default time.Time.UnmarshalJSON only accepts a quoted RFC3339 string, so ANY real AWS SDK client sending an explicit Time on a PutEvents entry would have gotten a JSON unmarshal error and the whole request would fail. This was on the REQUEST side, unlike the recurred response-side epoch-seconds bug class -- easy to miss because no existing test ever set the Time field over the wire (only via internal Go struct literals, which bypass json.Unmarshal entirely and never hit the bug). Added EventEntry.UnmarshalJSON (wire_time.go) parsing epoch-seconds numbers into time.Time; EventEntry is never marshaled back out to a client (confirmed via repo-wide grep) so this is unmarshal-only, no response-shape risk. Proven by TestEventEntry_UnmarshalJSON_TimeIsEpochSeconds (includes a case asserting an RFC3339 string is now correctly REJECTED, not silently misparsed)."}
  PutPartnerEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "delegates to PutEvents; inherits the same fixes"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ActivateEventSource: {wire: ok, errors: ok, state: ok, persist: ok}
  DeactivateEventSource: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEventSource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-hjap): handler returned the raw *EventSource struct via json.Marshal, so CreationTime/ExpirationTime serialized as RFC3339 strings instead of the real awsjson1.1 epoch-seconds numbers -- same bug class already fixed for DescribeEndpoint/ListEndpoints/DescribeEventBus/DescribeReplay. Added eventSourceResponse DTO converting via timeToEpochSeconds, matching archiveResponse's pattern."}
  ListEventSources: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-hjap) -- see DescribeEventSource (same eventSourceResponse DTO backs both). FIXED 2026-08-29 (cursor-pagination sweep): Limit was not even parsed from the request body, and the backend always used the fixed-100-page paginate() helper -- now threads a parsed Limit through paginateN()."}
  CancelReplay: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeReplay: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep, three bugs, field-diffed against DescribeReplayOutput: (1) handler returned the raw *Replay struct via json.Marshal -- EventStartTime/EventEndTime/ReplayStartTime/ReplayEndTime serialized as RFC3339 strings instead of the real awsjson1.1 epoch-seconds numbers. (2) Replay had no Destination field, so DescribeReplayOutput.Destination (a real member) was never echoed -- StartReplayInput.Destination was silently discarded after use. (3) Replay conflated the user-supplied Description (StartReplayInput.Description, a real DescribeReplayOutput.Description member) with the system-set StateReason into a single field -- Description was never echoed at all and StateReason carried the wrong content. Added replayListResponse/describeReplayResponse handler DTOs (describeReplayResponse embeds replayListResponse plus the Describe-only Destination/Description, matching real AWS where types.Replay used by ListReplaysOutput has neither). Also FIXED: StartReplayInput.EventStartTime/EventEndTime were plain time.Time with no custom unmarshal -- same request-side epoch-seconds bug class as PutEvents.Time (aws-sdk-go-v2 serializers.go confirms `smithytime.FormatEpochSeconds` for both fields); added StartReplayInput.UnmarshalJSON (wire_time.go)."}
  ListReplays: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep -- see DescribeReplay (replayListResponse DTO, correctly omits Destination/Description to match real AWS's types.Replay). FIXED 2026-08-29 (cursor-pagination sweep): Limit was never honoured -- listNamedItems (accessors.go, shared with ListArchives) always called the fixed-100-page paginate() instead of paginateN(). Both callers now thread limit through."}
  StartReplay: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep, real gap: ReplayDestination had no FilterArns field at all (real AWS: 'A list of ARNs for rules to replay events to'), so StartReplay always fanned a replay out to every rule on the destination bus whose pattern matched, even when the caller asked to restrict delivery to specific rules -- an over-delivery correctness bug, not just quiet data loss. Added ReplayDestination.FilterArns, threaded through startReplayLocked/scheduleReplayWorker/deliverEvents/buildDeliveryPlan (new filterRuleARNs parameter, nil for PutEvents' normal live-delivery path which is never filtered) to buildDeliveryPlan's per-rule match check. Also see DescribeReplay for the request-side epoch-seconds fix and the Destination/Description echo fix. Proven by TestStartReplay_FilterArnsRestrictsDelivery (two rules match the same pattern; FilterArns names one; asserts only that rule's target receives the replayed event while the live PutEvents delivery -- not subject to FilterArns -- still reaches both)."}
  CreateApiDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApiDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeApiDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against DescribeApiDestinationOutput this sweep: ApiDestinationArn/ApiDestinationState/ConnectionArn/CreationTime/Description/HttpMethod/InvocationEndpoint/InvocationRateLimitPerSecond/LastModifiedTime/Name all present and already epoch-seconds via handler_api_destinations.go's timeToEpochSeconds DTOs. No fix needed."}
  ListApiDestinations: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApiDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against UpdateApiDestinationOutput this sweep -- ApiDestinationArn/ApiDestinationState/CreationTime/LastModifiedTime all present. No fix needed."}
  CreateArchive: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against CreateArchiveOutput/DescribeArchiveOutput/Archive this sweep -- ArchiveName/ArchiveArn/CreationTime/Description/EventPattern/EventSourceArn/State/StateReason/EventCount/RetentionDays/SizeBytes all present, already epoch-seconds via handler_archives.go's archiveResponse DTO. STALE as of 2026-08-23 (manifest-harvest pass): the 'KmsKeyIdentifier NOT modeled' claim below is no longer true -- commit 69bbb940a (2026-08-15, #2417) added it (archives.go's CreateArchive/UpdateArchive, archiveResponse). Confirmed via a real client CreateArchive->DescribeArchive round trip (TestArchive_KmsKeyIdentifier_RealSDKClient, handler_event_buses_real_client_test.go). See items_still_open for the correction."}
  DeleteArchive: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeArchive: {wire: ok, errors: ok, state: ok, persist: ok}
  ListArchives: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): same listNamedItems fixed-page-size bug as ListReplays -- Limit was never honoured. Now threaded through paginateN()."}
  UpdateArchive: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against DescribeConnectionOutput/Connection/ConnectionAuthResponseParameters this sweep -- ConnectionArn/AuthorizationType/ConnectionState/CreationTime/LastAuthorizedTime/LastModifiedTime/Name/StateReason/SecretArn all present and epoch-seconds via handler_connections.go's DTOs; auth masking (API_KEY/BASIC/OAUTH) correctly omits ApiKeyValue/Password/ClientSecret entirely, matching real AWS's response types which have no such fields at all (only ApiKeyName/Username/ClientID). KmsKeyIdentifier and InvocationConnectivityParameters (real members, for private-API/PrivateLink connections) NOT modeled -- see items_still_open."}
  DeleteConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  ListConnections: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  DeauthorizeConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against DescribeEndpointOutput/Endpoint this sweep -- all fields present (Arn/CreationTime/Description/EndpointId/EndpointUrl/EventBuses/LastModifiedTime/Name/ReplicationConfig/RoleArn/RoutingConfig/State/StateReason). No missing fields; see DescribeEndpoint for the epoch-seconds fix."}
  DeleteEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: handler returned the raw *Endpoint struct via json.Marshal, so CreationTime/LastModifiedTime serialized as RFC3339 strings instead of the real awsjson1.1 epoch-seconds numbers -- same bug class as DescribeEventBus/DescribeReplay. Added endpointResponse handler DTO."}
  ListEndpoints: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep -- see DescribeEndpoint (same endpointResponse DTO, field set matches real AWS's types.Endpoint used by ListEndpointsOutput exactly)."}
  UpdateEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePartnerEventSource: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePartnerEventSource: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribePartnerEventSource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPartnerEventSources: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): Limit was parsed into the request but never consulted -- backend always used the fixed-100-page paginate() helper. Now threads Limit through paginateN()."}
  ListPartnerEventSourceAccounts: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-h910): this manifest's prior 'wire: ok, state: ok' claim was false -- the handler parsed nothing at all (not even the required EventSourceName) and unconditionally returned an empty list behind a comment claiming cross-account metadata has no meaningful in-process simulation. That premise was itself wrong: CreatePartnerEventSource already stores the offered Account on PartnerEventSource, and mirrors a PENDING/ACTIVE EventSource (CreationTime/ExpirationTime/State) in the same single account this emulator represents -- exactly the state this op needs, just never consulted. Decision: a real code fix, not just a manifest correction, since real backing state existed and was being discarded (the same bug class as kafka's UpdateRebalancing false comment and awsconfig's GetAggregateResourceConfig arbitrary-item bug found in this same pass). Now EventSourceName is required and looked up against partnerSourcesTable+eventSourcesTable; ResourceNotFoundException for an unknown name. This emulator models one partner-source-name -> one account (matching CreatePartnerEventSource's own shape), so at most one entry is ever returned even though real AWS can offer one source name to multiple accounts -- Limit/NextToken are accepted but never needed as a result."}
  TestEventPattern: {wire: ok, errors: ok, state: ok, persist: n/a, note: "delegates to the same compilePattern/matchCompiledPattern engine proved correct in prior sweeps -- see families.event_pattern_matching"}
  PutPermission: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this sweep: busePolicies (the map PutPermission/RemovePermission/PutEventBusPolicy write to) was entirely excluded from backendSnapshot -- persistence.go's own doc comment said so, and PARITY.md had nonetheless marked this op 'persist: ok', which was independently field-verified false this sweep (a policy set via PutPermission did not survive Snapshot/Restore). Added backendSnapshot.BusPolicies (plain map[string]map[string]*EventBusPolicy, round-trips via encoding/json without needing a func(*V) string key extractor the way the genuinely unkeyable archivedEvents/schemaVersions/codeBindings maps do) and wired it into Snapshot/Restore. Also added the missing `json:\"Statements\"` tag on EventBusPolicy.Statements (musttag caught this once the type became reachable from json.Marshal). Proven by an addition to TestInMemoryBackend_FullStateSnapshotRestore. 2026-08-28 write-only-state sweep, another real gap: PutPermissionInput had no Condition field at all (real SDK: api_op_PutPermission.go, PutPermissionInput.Condition *types.Condition -- the documented pattern for granting an entire AWS Organization access via Principal=\"*\" plus a Condition on aws:PrincipalOrgID), so it was silently dropped by json.Unmarshal: never stored on the statement, and DescribeEventBus.Policy (the only real read path for a bus's resource policy, per the DescribeEventBus row above) could never echo it back. Fixed: added Condition to PutPermissionInput and a matching Condition field on EventBusPolicyStatement (standard IAM policy JSON shape -- a map from condition operator, e.g. \"StringEquals\", to a map of condition key to value), built from the flat Type/Key/Value struct in PutPermission. See TestPutPermission_Condition_RoundTripsThroughPolicy in wire_field_fixes_test.go."}
  RemovePermission: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep -- see PutPermission (same busePolicies persistence fix)."}
  GetEventBusPolicy: {wire: partial, errors: ok, state: ok, persist: ok, note: "not a real EventBridge SDK op (no GetEventBusPolicy/PutEventBusPolicy in aws-sdk-go-v2/service/eventbridge's 57 ops); an internal-only helper reachable via the handler's policyActions() dispatch table. FIXED this sweep: prior notes here claimed it was 'absent from GetSupportedOperations, so no real SDK client can invoke it' -- that was false; it was actually present in GetSupportedOperations()/ChaosOperations() (confirmed by pkgs/sdkcheck's reverse-completeness check, gopherstack-vhw2), and TestHandler_GetSupportedOperationsIncludesPolicyOps asserted the very defect. Removed from GetSupportedOperations() (kept in the dispatch table for any existing direct callers) so the code finally matches what this note always claimed. The real wire path for reading a bus policy, DescribeEventBus.Policy, is wired (see DescribeEventBus above)."}
  PutEventBusPolicy: {wire: partial, errors: ok, state: ok, persist: ok, note: "same as GetEventBusPolicy -- not a real SDK op, and had the same GetSupportedOperations discrepancy, now fixed the same way. Its writes still persist (see PutPermission)."}
  DescribeSchemaVersion: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: not a real Schemas SDK op (no such method on aws-sdk-go-v2/service/schemas.Client at any version -- the real wire path for reading a specific version's content is DescribeSchema's optional SchemaVersion request field). Was advertised in GetSupportedOperations()/ChaosOperations() and asserted present by TestHandler_SchemaOperationsIncluded, both wrong. Removed from GetSupportedOperations() (kept in the dispatch table via schemaVersionActions() for any existing direct callers)."}
  PutCodeBinding/DescribeCodeBinding/GetCodeBindingSource: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "FIXED (wrapper-key sweep). DescribeCodeBindingInput.SchemaVersion (\"Specifying this limits the results to only this schema version\", api_op_DescribeCodeBinding.go) was parsed at the REST layer (handler_schemas_rest.go already read the `schemaVersion` query param correctly) but never consulted by the backend: codeBindingKey(registryName, schemaName, language) had no version dimension, so PutCodeBinding for a new schema version silently overwrote the single stored binding for that (registry, schema, language) regardless of what version was requested -- DescribeCodeBinding/GetCodeBindingSource for an OLDER version would then silently return the newer version's binding (dropped-parameter, silent-wrong-answer shape). ListCodeBindings was already correct -- it filters by each stored CodeBinding's own SchemaVersion field, not the key, so it was unaffected. Fixed by adding schemaVersion to codeBindingKey and a shared effectiveSchemaVersion(registryName, schemaName, requested) helper (schemas.go) that resolves an empty request to the schema's current version, used by all three ops for both the key and (GetCodeBindingSource) the echoed placeholder text. Proven via TestSchemasDescribeCodeBinding_VersionScoped_RealSDKClient (handler_schemas_real_client_test.go): PutCodeBinding for v1 then v2, DescribeCodeBinding(SchemaVersion=\"1\") must still return SchemaVersion \"1\" -- fails against unmodified code (returns \"2\"). UpdateSchemaInput.ClientTokenId (idempotency token) spot-checked and left unimplemented: real CreateSchemaInput has no such field at all, so it's retry-dedup-only behavior with no single-request-observable effect -- a documented gap, not a bug."}
  ListCodeBindings: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: not a real Schemas SDK op (no such method on aws-sdk-go-v2/service/schemas.Client at any version -- checking a binding's status is DescribeCodeBinding, one language at a time; there is no list-all-bindings operation). Was advertised in GetSupportedOperations()/ChaosOperations() and asserted present by TestHandler_SchemaOperationsIncluded, both wrong. Removed from GetSupportedOperations() (kept in the dispatch table via codeBindingActions() for any existing direct callers)."}
families:
  event_pattern_matching: {status: ok, note: "Not re-read this sweep (pattern.go unchanged since the prior sweep's commit -- trusted per the re-audit protocol). Prior sweep's proof: read pattern.go (559 LOC) in full and cross-checked every documented AWS content-filter operator against matchSpecialMatcher/matchStringMatcher: exact-match arrays, prefix/suffix (incl. nested equals-ignore-case form), exists (incl. explicit JSON null counting as present), numeric (paired-operator ranges, all four comparators), anything-but (scalar/list/object forms incl. nested prefix/suffix/wildcard/equals-ignore-case/numeric), cidr, wildcard (iterative two-pointer glob, no recursion/ReDoS), equals-ignore-case, nested objects, $or (top-level and nested), and array-valued event fields (any-element-matches semantics). Covered by pattern_test.go (519 LOC) + pattern_validation_test.go (129 LOC)."}
  schema_registry_and_pipes: {status: ok, note: "CreateRegistry..GetCodeBindingSource and CreatePipe..UpdatePipe are separate control planes in real AWS (schemas/pipes SDK modules, not events). pkgs/sdkcheck's reverse-completeness check (gopherstack-vhw2) verified this sweep: all Pipe ops (CreatePipe/DeletePipe/DescribePipe/ListPipes/UpdatePipe) and all remaining Schema Registry ops (CreateRegistry..UpdateSchema, GetDiscoveredSchema, PutCodeBinding/DescribeCodeBinding/GetCodeBindingSource) are real pipes.Client/schemas.Client operations -- confirmed by name against both SDK modules at their pinned versions. sdk_completeness_test.go now checks each third of GetSupportedOperations() against the SDK client that actually owns it (eventbridge/pipes/schemas) instead of a single eventbridgesdk.Client, which is what let the two fabricated ops below hide as 'phantom' entries the reverse check couldn't previously distinguish from legitimate sibling-client ops. DescribeSchemaVersion and ListCodeBindings were NOT real (see ops above) and have been removed from GetSupportedOperations(). Schema Registry timestamps (Schema/SchemaVersion/CodeBinding: LastModified/VersionCreatedDate/CreatedDate) are correctly plain time.Time -- pipes.Client is REST-JSON (awsRestjson1_*) but its CreationTime/LastModifiedTime fields are epoch-seconds per the SDK's own deserializers.go (smithytime.ParseEpochSeconds), while schemas.Client's are genuinely ISO-8601 (smithytime.ParseDateTime, RFC3339Nano) -- verified separately per pinned SDK, not assumed from the shared REST-JSON protocol label. Fixed gopherstack-hjap: DescribePipe/ListPipes (handler_pipes.go) returned the raw *Pipe/[]Pipe struct directly via json.Marshal, so CreationTime/LastModifiedTime serialized as RFC3339 strings despite pipes' epoch-seconds wire -- CreatePipe/UpdatePipe were already correct (built epoch-converted anonymous response structs) but Describe/List were not. Added pipeResponse DTO (same timeToEpochSeconds pattern) backing both."}
  archives_replays_connections_api_destinations_endpoints: {status: ok, note: "Previously 'deferred, spot-checked only'. Field-diffed this sweep against aws-sdk-go-v2/service/eventbridge's api_op_*.go Input/Output structs and types.go for Archive, Connection (+ ConnectionAuthResponseParameters/CreateConnectionAuthRequestParameters/UpdateConnectionAuthRequestParameters), ApiDestination, Endpoint (+ RoutingConfig/FailoverConfig/Primary/Secondary/EndpointEventBus), Replay, and ReplayDestination. Found and fixed real bugs: DescribeEndpoint/ListEndpoints and DescribeReplay/ListReplays response-side epoch-seconds bug, Replay missing Destination/Description, ReplayDestination missing FilterArns (an over-delivery correctness bug, not just a missing echo field), StartReplayInput request-side epoch-seconds bug. Connections and API destinations were already correct field-for-field (auth masking, all CRUD output shapes) except the KMS/private-API-connectivity extras noted per-op above and in items_still_open."}
gaps:
  - "Cross-account event-bus PutTargets delivery (gopherstack-9iva, structural): real AWS's primary documented use of an event-bus-ARN target is routing matched events to ANOTHER AWS account's event bus (api_op_PutTargets.go doc comment; requires the target account to have granted permission via PutPermission, optionally a RoleArn for org-granted permission). This backend models a single AWS account (InMemoryBackend.accountID is fixed at construction), so there is no second account's bus state to route into -- RouteEventToBus (delivery.go) detects a target ARN whose account segment differs from b.accountID and drops it, the same as any other target ARN this backend cannot resolve, rather than erroring (PutTargets itself never validated ARNs and still doesn't -- see PutTargets note). Same-account cross-bus/cross-region routing (a rule on one bus targeting another bus this backend does host) IS implemented and delivers for real."
  - "ECS delivery central wiring (bd gopherstack-ubum, service side FIXED this sweep, cli.go NOT touched -- out of services/eventbridge scope): delivery.go's ECSTaskRunner interface previously only passed (clusterARN, payload) to RunTask, so an ECS target delivery only ran the right task definition if the event Input/InputTransformer payload happened to carry a \"TaskDefinition\" key -- EcsParameters.TaskDefinitionArn/LaunchType/TaskCount/NetworkConfiguration set via PutTargets were validated and stored but never reached delivery. Fixed the service side with an optional-capability extension: new ECSTaskRunnerWithParams interface (RunTaskWithParams(ctx, clusterARN, *EcsParameters, payload)); deliverToECS type-asserts dt.ECS against it and prefers it when present, falling back to the base RunTask otherwise, so no existing ECSTaskRunner implementation breaks. Also found and fixed a real wire-shape gap while verifying against the pinned SDK: EcsParameters was missing the real TaskCount *int32 member (aws-sdk-go-v2/service/eventbridge/types@v1.48.4, wire key \"TaskCount\") entirely -- added. Central wiring still needed (cli.go, main-thread/future-session work): ebECSTaskRunnerAdapter in cli.go must grow a RunTaskWithParams method mapping EcsParameters onto ecsbackend.RunTaskInput (TaskDefinitionArn->TaskDefinition, LaunchType->LaunchType, TaskCount->Count, NetworkConfiguration->NetworkConfiguration, Group/PlatformVersion/PlacementConstraints/PlacementStrategy/CapacityProviderStrategy/Tags/EnableECSManagedTags/EnableExecuteCommand map 1:1 by name) for the fix to take effect end-to-end; until then, ECS delivery keeps using the legacy RunTask/payload-TaskDefinition-key path with unchanged behavior (no regression, just not yet wired to the new capability)."
deferred:
  - "Schema registry (CreateRegistry..GetCodeBindingSource, 17 real ops -- see schema_registry_and_pipes) and Pipes (CreatePipe..UpdatePipe, 5 ops) -- these model separate AWS control planes (schemas/pipes SDK modules), not core EventBridge (events) ops; field-level wire/errors/state audit still not done this pass, only the SDK-completeness/naming check. UPDATE 2026-08-29: the pagination slice of that still-undone audit is now done -- ListRegistries/ListSchemas/SearchSchemas/ListSchemaVersions all declare real Limit/NextToken (schemas@v1.37.4) that were completely unconsulted on both the JSON-RPC (handler_schemas.go/handler_registries.go, dead for a real client but still fixed for consistency) and REST-JSON1 (handler_schemas_rest.go, the actually-reachable path) dispatch paths -- every call returned every stored item in one unbounded page regardless of Limit or the query's `limit` param. Fixed via the existing paginateSlice-equivalent (backend methods gained a `limit int` parameter, wired to paginateN); REST handlers gained schemasRESTLimit(q) to parse the `limit` query param. Field-level wire/errors/state audit for the rest of these 17+5 ops is still open. UPDATE (wrapper-key sweep): PutCodeBinding/DescribeCodeBinding/GetCodeBindingSource now field-verified too -- see their own ops: entry (a real SchemaVersion-scoping bug found and fixed)."
  - "PutPermission/RemovePermission/policy-statement JSON shape (EventBusPolicyStatement.Principal as `any` for both string and object-with-AWS-key forms) -- spot-checked only, not re-verified this sweep beyond the persistence fix."
leaks: {status: clean, note: "Re-verified this sweep: PutEvents's async delivery goroutine (b.wg.Go) acquires a workerSem slot or aborts on svcCtx.Done() before delivering, so Close()/Shutdown() cannot leave in-flight goroutines past defaultShutdownTimeout; deliverToTargetBounded applies a per-attempt context.WithTimeout and always cancels it. The new StartReplay FilterArns plumbing (replayDeliveryPlan struct, matchedDeliveryGroupsForEntry) is a same-lock-discipline refactor of the existing buildDeliveryPlan/deliverEvents path, not a new goroutine or lock -- scheduleReplayWorker still acquires workerSem-or-aborts-on-ctx.Done() exactly as before. Scheduler (scheduler.go) and ArchiveJanitor (janitor.go) were not touched this sweep; existing leak_test.go/isolation_test.go continue to pass."}
---

## Notes

### 2026-08-13 remaining RFC3339-vs-epoch-seconds holdouts (gopherstack-hjap)

`DescribeEventSource`/`ListEventSources` were filed as carrying the same
epoch-seconds bug already fixed for `DescribeEndpoint`/`ListEndpoints`/
`DescribeEventBus`/`DescribeReplay` (all now go through a response DTO that
calls `timeToEpochSeconds`). Fixed the same way: added `eventSourceResponse`
(`handler_event_sources.go`), converting `CreationTime`/`ExpirationTime` via
`timeToEpochSeconds`, matching `archiveResponse`'s pattern.

Per the issue's instruction to check every other timestamp-emitting op in the
service, not just the two named: found the same bug still live in
`DescribePipe`/`ListPipes` (`handler_pipes.go`) -- both returned the raw
`*Pipe`/`[]Pipe` backend struct directly via `json.Marshal`, even though
`CreatePipe`/`UpdatePipe` in the same file already built epoch-converted
anonymous response structs for their own `CreationTime`/`LastModifiedTime`
fields. Confirmed against the pinned `aws-sdk-go-v2/service/pipes@v1.26.4`
deserializer (`smithytime.ParseEpochSeconds` for both fields, despite a
stale "ISO-8601 format" doc comment on `LastModifiedTime` in the generated
SDK source -- the deserializer switch is authoritative, not the doc comment).
Fixed with a `pipeResponse` DTO backing both ops.

Every other eventbridge-protocol (`awsjson1.1`) model with a `time.Time`
field was re-checked and confirmed already converting through an
epoch-seconds DTO: `EventBus`, `Replay`, `APIDestination`, `Archive`,
`Connection`, `Endpoint`, `PartnerEventSourceAccountInfo`. `Rule` and
`PartnerEventSource` carry no timestamp fields at all. The Schema Registry
models (`Schema`/`SchemaVersion`/`CodeBinding`) were deliberately NOT
touched: `schemas.Client` is a separate REST-JSON SDK module whose
deserializers use `smithytime.ParseDateTime` (RFC3339Nano) for these exact
fields, confirmed against the pinned `aws-sdk-go-v2/service/schemas@v1.37.4`
source -- their current plain-`time.Time` `json.Marshal` output is already
correct ISO-8601, and "converting" them would have introduced a new bug
rather than fixed one. Proven by
`TestDescribeEventSourceAndListEventSources_TimestampsAreEpochFloat` and
`TestDescribePipeAndListPipes_TimestampsAreEpochFloat`, both matching the
existing `TestHandler_Endpoint_TimestampsAreEpochSeconds`/pipe-timestamp test
style; verified to fail against the pre-fix code by hand-revert.

### ECS delivery param threading (2026-08-07, bd gopherstack-ubum) -- service side fixed, cli.go wiring still needed

`services/eventbridge/delivery.go`'s ECS target delivery (`deliverToECS`) called
`ECSTaskRunner.RunTask(ctx, clusterARN, payload)` -- no path for the target's
`EcsParameters` (`TaskDefinitionArn`, `LaunchType`, `TaskCount`,
`NetworkConfiguration`, ...) to reach the ECS backend at all, even though
`PutTargets` already validates and stores them (see "PutTargets silently
dropped 8 of the SDK's target-type-specific parameter structs" below). In
practice this meant an ECS delivery only ran the intended task definition if
the delivered payload (post `Input`/`InputTransformer`) happened to contain a
literal `"TaskDefinition"` JSON key -- the documented `EcsParameters` config
was silently ignored.

Fixed via an additive optional-capability interface (same pattern
`services/sts`'s `AccountSettingsLookup` uses, and `services/iam`'s
`GetOutboundWebIdentityFederationInfo` gate): `ECSTaskRunnerWithParams`
extends `ECSTaskRunner` with
`RunTaskWithParams(ctx, clusterARN string, params *EcsParameters, payload []byte) error`.
`deliverToECS` type-asserts `dt.ECS` against it; if the concrete adapter
implements it, its params-aware method is called with `target.EcsParameters`;
otherwise the original `RunTask` still runs exactly as before. This keeps
every existing `ECSTaskRunner` implementation (including `cli.go`'s
`ebECSTaskRunnerAdapter` and every test double in
`delivery_ecs_test.go`) compiling and working unchanged -- `go build ./...`
stays green with zero other-service edits.

While verifying `EcsParameters` field-by-field against the pinned
`aws-sdk-go-v2/service/eventbridge/types@v1.48.4` (`types.go`), found it was
also missing the real `TaskCount *int32` member (wire key `"TaskCount"`,
confirmed in `serializers.go`/`deserializers.go`) -- added.

**Central wiring NOT done here (`cli.go` is out of `services/eventbridge`
scope):** `cli.go`'s `ebECSTaskRunnerAdapter` (the concrete type installed as
`DeliveryTargets.ECS`) needs a new `RunTaskWithParams` method for the fix to
take effect end-to-end. It should map `EcsParameters` onto
`ecsbackend.RunTaskInput` by field name: `TaskDefinitionArn` ->
`TaskDefinition`, `LaunchType` -> `LaunchType`, `TaskCount` -> `Count`,
`NetworkConfiguration` -> `NetworkConfiguration` (distinct Go types in the two
packages, but field-identical -- both are `{AwsvpcConfiguration: {Subnets,
SecurityGroups, AssignPublicIP}}` -- so the adapter needs a small field-by-field
struct conversion, not a direct assignment), plus `Group`, `PlatformVersion`,
`PlacementConstraints`, `PlacementStrategy`, `CapacityProviderStrategy`,
`Tags`, `EnableECSManagedTags`, `EnableExecuteCommand` (all present on both
`eventbridge.EcsParameters` and `ecsbackend.RunTaskInput`/`Tag` with matching
field names/shapes, same distinct-but-identical-shape caveat). `Cluster`
continues to come from the existing
`clusterARN` parameter, matching the current `RunTask` behavior. Until that
method exists, ECS delivery keeps using the legacy
`RunTask`/payload-`"TaskDefinition"`-key path -- no regression, just the new
capability not yet reachable.

Proven service-side by `TestDelivery_ECS_ParamsThreading` (a sink implementing
`ECSTaskRunnerWithParams` receives the full `EcsParameters` set via
`PutTargets`, including the newly-added `TaskCount` and nested
`NetworkConfiguration`) and `TestDelivery_ECS_LegacySinkStillWorksWithoutParams`
(a `RunTask`-only sink is still invoked, proving the optional-capability probe
never regresses existing adapters). `go build ./...`, `go vet`, and
`go test -race ./services/eventbridge/...` all clean.

### Reverse sdkcheck sweep (2026-07-31) -- 4 fabricated ops found and removed, multi-client check added

`pkgs/sdkcheck`'s reverse check (gopherstack-vhw2) flagged 26 GetSupportedOperations()
entries as "phantom" (not exported methods on `eventbridgesdk.Client`). Verified every one
by name against `pipes.Client`/`schemas.Client` at their pinned versions (and, for the four
that matched neither, against every released version of `eventbridge.Client`/`schemas.Client`
back to v1.0.0, to rule out a rename/removal rather than an outright fabrication):

- **22 real, sibling-owned**: CreatePipe/DeletePipe/DescribePipe/ListPipes/UpdatePipe (5,
  real `pipes.Client` ops) and CreateRegistry..UpdateSchema/GetDiscoveredSchema/
  PutCodeBinding/DescribeCodeBinding/GetCodeBindingSource (17, real `schemas.Client` ops).
  The reverse check only flagged these because it was comparing them against
  `eventbridgesdk.Client` instead of the client that actually owns them.
  `sdk_completeness_test.go` now splits `GetSupportedOperations()` three ways and checks
  each third against `eventbridgesdk.Client`/`pipessdk.Client`/`schemassdk.Client`
  respectively (mirroring the pattern `services/bedrock` already uses for its two handlers,
  each checked against its own client -- here it's one handler's op list split three ways
  instead of two handlers).
- **4 fabricated -- never real at any SDK version, now removed from GetSupportedOperations()**:
  `GetEventBusPolicy`/`PutEventBusPolicy` (not real EventBridge ops; the real wire path is
  `DescribeEventBus.Policy`) and `DescribeSchemaVersion`/`ListCodeBindings` (not real Schemas
  ops; the real wire paths are `DescribeSchema`'s optional `SchemaVersion` field and
  per-language `DescribeCodeBinding` respectively). All four remain reachable internal-only
  via their existing dispatch-table entries (`policyActions()`/`schemaVersionActions()`/
  `codeBindingActions()`) for any existing direct callers, but a real AWS SDK client can never
  send them, so they must not be advertised as supported. See the `ops:` entries above for
  detail per operation.
- **Test encoding the defect**: `TestHandler_GetSupportedOperationsIncludesPolicyOps`
  asserted `GetEventBusPolicy`/`PutEventBusPolicy` were present in
  `GetSupportedOperations()` -- i.e. it asserted the bug. Renamed to
  `TestHandler_GetSupportedOperationsExcludesPolicyOps` and flipped to `NotContains`.
  `TestHandler_GetSupportedOperationsIncludesDeliveryTargetTypes` and
  `TestHandler_SchemaOperationsIncluded` also asserted the four fabricated names were
  present; trimmed.
- Also worth noting: two prior sweeps' notes on `GetEventBusPolicy`/`PutEventBusPolicy`
  (`### Deep sweep (parity-3...)` and `### Re-audit sweep (parity-4...)` below) both claimed
  these two were "absent from `GetSupportedOperations()`" -- that claim was never actually
  true until this sweep's fix; the code had them present all along and nothing had verified
  the claim against the real list. Left the historical notes as-is (they're an accurate
  record of what was believed at the time) and corrected the live `ops:` entries above.

Gates run scoped to `services/cloudfront`, `services/eventbridge`, `services/iot`,
`services/opensearch`, `services/personalize` (four other services triaged in the same
sdkcheck sweep, sharing the "real op on a sibling client vs. fabricated" pattern): `go build
./...`, `go vet`, `go test -race -count=1`, `gofmt -l`, `golangci-lint run` all clean;
`git diff --stat go.mod go.sum` non-empty by design -- added
`cloudfrontkeyvaluestore`/`schemas`/`opensearchserverless`/`personalizeruntime` as new direct
SDK dependencies (needed by the new sibling-client `sdk_completeness_test.go` checks), which
bumped the shared `aws-sdk-go-v2` core module v1.43.0 -> v1.43.2 (patch only) and
`smithy-go` v1.27.4 -> v1.27.5.

### Deep sweep (parity-3, 2026-07-23) -- field-diffed the "deferred" families, found and fixed 8 real bugs

Per the parity-3 campaign brief, actually field-diffed every family the 2026-07-11 sweep
had left as "deferred, spot-checked only" (archives, replays, connections, API
destinations, endpoints) against `aws-sdk-go-v2/service/eventbridge@v1.45.21`'s
`api_op_*.go` Input/Output structs and `types.go`, rather than trusting the prior
spot-check. Also re-examined the `gopherstack-ba7` ManagedBy gap the prior sweep had
explicitly declined to fix. Found 8 real bugs, all fixed:

1. **`PutRuleInput.ManagedBy` was a gopherstack-invented wire-facing field.** Real AWS's
   `PutRuleInput` has no `ManagedBy` member at all (server-populated, `Describe`/`List`-only
   -- confirmed against `aws-sdk-go-v2/service/eventbridge/api_op_PutRule.go`). gopherstack's
   `PutRuleInput.ManagedBy` had `json:"ManagedBy,omitempty"` and `handler_rules.go`'s
   `PutRule` unmarshals the raw request body directly into `PutRuleInput`, so any client --
   real or malicious -- sending `"ManagedBy":"scheduler.amazonaws.com"` could mark its own
   rule as AWS-service-managed. Changed the tag to `json:"-"` (kept the Go field as an
   internal same-process seeding hook, since a future in-process AWS-service integration,
   e.g. EventBridge Scheduler, legitimately needs a way to create a managed rule). Proven by
   `TestPutRule_ManagedByNotWireSettable` (wire path rejects it) and the retained/renamed
   `TestPutRule_ManagedByPreserved` (internal Go-level path still works).

2. **`ManagedRuleException` enforcement (`gopherstack-ba7`) implemented for real.** The prior
   sweep found `Rule.ManagedBy` was modeled but never checked before `PutRule`
   (update)/`DeleteRule`/`EnableRule`/`DisableRule`/`PutTargets`/`RemoveTargets` mutated a
   rule, and declined to fix it as "unreachable/inert in practice." Added `ErrManagedRule`
   (maps to `ManagedRuleException`, HTTP 400) and a `checkManagedRule` guard in all six
   ops -- `PutTargets`/`RemoveTargets` previously didn't even fetch the `Rule` to check it
   (only a `busRules.Has()` existence check), so they needed an added lookup, not just a
   guard call. Proven by `TestManagedRuleException_RejectsRuleLevelMutations` (4 subtests)
   and `TestManagedRuleException_RejectsTargetMutations` (2 subtests).

3. **`DescribeEventBus`/`ListEventBuses` response-side epoch-seconds bug.** Both handlers
   returned the raw `*EventBus`/`[]EventBus` via `json.Marshal` with no DTO conversion, so
   `CreatedTime` serialized as an RFC3339 string. Real AWS's `DescribeEventBusOutput`/
   `types.EventBus` (confirmed via `aws-sdk-go-v2/service/eventbridge`'s `deserializers.go`:
   `smithytime.ParseEpochSeconds`) expects an awsjson1.1 epoch-seconds JSON number -- a real
   SDK client's deserializer rejects the RFC3339 form outright. This is the same bug class
   already fixed for archives/connections/API destinations/pipes in earlier sweeps
   (`handler_archives.go` etc. already used a `timeToEpochSeconds` DTO pattern) but had been
   missed for event buses, endpoints, and replays. Added `eventBusResponse` DTO.

4. **`EventBus` had no `Policy` field.** `PutPermission`/`PutEventBusPolicy` write a policy
   to a separate internal store (`busePolicies`), but `EventBus` never had a `Policy` field
   to surface it, so `DescribeEventBus`/`ListEventBuses` never echoed it -- despite the
   prior sweep's own note claiming *"DescribeEventBus.Policy is the real wire path for
   reading a bus policy"* (true of real AWS, but not actually implemented here). Added
   `EventBus.Policy` (computed at response time from the policy store, not stored
   redundantly on the struct) via the same `eventBusResponse` DTO. Proven by
   `TestHandler_DescribeEventBus_EchoesPolicy`.

5. **`DescribeEndpoint`/`ListEndpoints` and `DescribeReplay`/`ListReplays` response-side
   epoch-seconds bug.** Same bug class as #3 -- both handler pairs returned raw
   `*Endpoint`/`*Replay` structs. Added `endpointResponse` and
   `replayListResponse`/`describeReplayResponse` DTOs.

6. **`Replay` had no `Destination` or `Description` field**, so `DescribeReplayOutput`'s real
   `Destination` and `Description` members (confirmed field-diffed against
   `api_op_DescribeReplay.go`) were silently discarded after `StartReplay` used them, never
   echoed back. `StateReason` had also been overloaded to carry the user's
   `StartReplayInput.Description` text instead of a real system-set state reason. Added both
   fields to `Replay` (JSON-tagged `"-"`, only exposed via `describeReplayResponse`, since
   real AWS's `types.Replay` used by `ListReplaysOutput` has neither -- `replayListResponse`
   correctly omits them). Proven by `TestDescribeReplay_EchoesDestinationAndDescription`.

7. **`ReplayDestination` had no `FilterArns` field -- a real over-delivery correctness bug,
   not just a missing echo field.** Real AWS's `ReplayDestination.FilterArns` ("A list of
   ARNs for rules to replay events to") lets a caller restrict replay delivery to specific
   rules; without it modeled at all, `StartReplay` always fanned a replay out to *every* rule
   on the destination bus whose pattern matched, even when the caller asked to restrict it.
   Added `ReplayDestination.FilterArns` and threaded a `filterRuleARNs` set through
   `startReplayLocked` -> `scheduleReplayWorker` -> `deliverEvents` -> `buildDeliveryPlan`
   (PutEvents' live-delivery call site passes `nil`, i.e. unfiltered, matching AWS: live
   delivery is never subject to a replay's `FilterArns`). Proven end-to-end by
   `TestStartReplay_FilterArnsRestrictsDelivery` (two rules match one archived event's
   pattern; `FilterArns` names one; only that rule's target receives the replayed copy).

8. **`PutEvents`/`PutPartnerEvents` (`EventEntry.Time`) and `StartReplay`
   (`StartReplayInput.EventStartTime`/`EventEndTime`) request-side epoch-seconds bug --
   the same bug class as #3/#5 but on the *request* side, and more severe.** Confirmed
   against `aws-sdk-go-v2/service/eventbridge`'s `serializers.go`
   (`awsAwsjson11_serializeDocumentPutEventsRequestEntry` /
   `awsAwsjson11_serializeOpDocumentStartReplayInput`): a real AWS SDK client serializes
   these as epoch-seconds JSON numbers, not RFC3339 strings. gopherstack's `EventEntry.Time`
   and `StartReplayInput.EventStartTime`/`EventEndTime` were plain `time.Time`/`*time.Time`
   fields with no custom `UnmarshalJSON`, so Go's default `time.Time.UnmarshalJSON` (which
   only accepts a quoted RFC3339 string) would reject any real client's request outright.
   This is the request-side mirror of the response-side epoch-seconds bug class the parity
   principles doc already calls out, but on the *decode* path instead of *encode* -- easy to
   miss because every existing test that set these fields did so via internal Go struct
   literals (bypassing `json.Unmarshal` entirely) rather than over the wire; the one test
   that DID exercise the wire path (`handler_replays_test.go`'s `StartReplay` tests) was
   itself sending RFC3339 strings and "passing" only because nothing checked the *format*,
   just that a `time.Time` came out the other end matching *some* value it had put in via a
   *different* unmarshal path than a real client would ever use. Added `EventEntry`.
   `UnmarshalJSON` and `StartReplayInput.UnmarshalJSON` (new file `wire_time.go`) using a
   shared `parseEpochSecondsPtr` helper. `EventEntry` is never marshaled back out to a
   client (confirmed via repo-wide grep -- it's request/internal-only), so this is
   unmarshal-only with no response-shape risk. Updated the two pre-existing
   `handler_replays_test.go` tests that sent RFC3339 strings to send epoch-seconds instead
   (matching real client behavior); added `wire_time_test.go` proving both the epoch-seconds
   parse AND that an RFC3339 string is now correctly *rejected* rather than silently
   misparsed.

Also fixed, lower severity: **`busePolicies` (event bus resource policies) was silently
excluded from `backendSnapshot`** -- `persistence.go`'s own doc comment said so, yet
`PutPermission`/`RemovePermission` were marked `persist: ok` in PARITY.md. Independently
field-verified this was false (a policy did not survive `Snapshot`/`Restore`) and fixed by
adding `backendSnapshot.BusPolicies` (a plain `map[string]map[string]*EventBusPolicy`,
which -- unlike the genuinely unkeyable `archivedEvents`/`schemaVersions`/`codeBindings`
maps -- round-trips fine through `encoding/json` with no `func(*V) string` key extractor
needed). Also added the missing `json:"Statements"` tag on `EventBusPolicy.Statements`
(`musttag` caught this once the type became reachable from `json.Marshal` via the new
snapshot field). Proven by an addition to `TestInMemoryBackend_FullStateSnapshotRestore`.

Two `gocognit` complexity findings surfaced by decomposing/adding code this sweep were
fixed by extraction, not suppression (no nolint, per the no-stub/no-suppress convention):
`buildDeliveryPlan`'s inner double-loop was split out into
`matchedDeliveryGroupsForEntry`/`ruleMatchesForDelivery`, and `eventBusActions`'s four
inline closures were promoted to named `Handler` methods
(`handleCreateEventBus`/`handleDeleteEventBus`/`handleListEventBuses`/
`handleDescribeEventBus`).

Not fixed / explicitly deferred this sweep (see items_still_open in the return receipt):
`EventBus`/`Archive`/`Connection`'s `KmsKeyIdentifier` (encryption-at-rest), `EventBus`'s
`DeadLetterConfig`/`LogConfig`, and `Connection`'s `InvocationConnectivityParameters` (
private-API/PrivateLink connectivity) -- all real fields on the corresponding SDK
Input/Output types, none currently modeled. These are more speculative, larger features
(would need real encryption/DLQ/private-connectivity semantics, not just an echoed field)
than the codebase's demonstrated usage patterns justify building out blind in this pass.

**CORRECTION (2026-08-23, manifest-harvest pass):** the `EventBus`/`Archive` half of this
paragraph is stale. Commit `69bbb940a` (2026-08-15, #2417, "three accepted-then-ignored
params") modeled `EventBus.KmsKeyIdentifier`/`DeadLetterConfig`/`LogConfig` and
`Archive.KmsKeyIdentifier` end to end (request parsing, backend state, and response DTOs)
-- see the `DescribeEventBus`/`CreateArchive` op rows above and
`TestEventBus_KmsDlqLogConfig_RealSDKClient`/`TestArchive_KmsKeyIdentifier_RealSDKClient`
(`handler_event_buses_real_client_test.go`) for real-client proof. Only `Connection`'s
`KmsKeyIdentifier` and `InvocationConnectivityParameters` are still genuinely unmodeled
(`Connection`, models.go, has neither field at all) -- correctly still deferred, for the
same reason given above (private-API/PrivateLink connectivity is a real feature, not an
echoed field).

### Re-audit sweep (parity-4, 2026-07-11) -- no drift, no bugs found

`git diff ce30166a..f615e2f8 -- services/eventbridge/` is empty: zero files under this
service changed since the prior sweep's baseline commit. `aws-sdk-go-v2/service/eventbridge`
is still pinned at v1.45.21 (go.mod unchanged) -- same SDK surface as last audited.
Independently re-derived the SDK's real operation set from
`aws-sdk-go-v2/service/eventbridge@v1.45.21`'s `api_op_*.go` files (57 ops) and diffed
against `Handler.GetSupportedOperations()`: all 57 are present (plus the two non-SDK
helpers `GetEventBusPolicy`/`PutEventBusPolicy` and the separate-control-plane Pipes/Schema
Registry ops already noted below) -- no missing or newly-added AWS op this sweep. Per the
re-audit protocol, all unchanged `ok` rows above were trusted as-is; additionally
spot-re-checked `PutPermission`/`RemovePermission` (backend.go) since the prior sweep had
marked that pair "spot-checked only" -- both still validate the event bus exists, require
`StatementId` for the non-raw-`Policy` path, and correctly no-op `RemovePermission` when no
matching statement/policy exists (matches AWS's idempotent-remove semantics). All gates
(`go build`, `go vet`, `go test -race`, `go fix -diff`, `golangci-lint run`, all scoped to
`services/eventbridge/...`) pass clean with zero findings. No code changes made this sweep.

### Fixed this sweep (severe/high-value first)

1. **PutTargets silently dropped 8 of the SDK's target-type-specific parameter
   structs.** `Target` (models.go) only modeled
   Input/InputPath/InputTransformer/DeadLetterConfig/RetryPolicy/BatchParameters.
   Any client setting `EcsParameters`, `HttpParameters`, `KinesisParameters`,
   `RedshiftDataParameters`, `RunCommandParameters`,
   `SageMakerPipelineParameters`, `SqsParameters`, or `AppSyncParameters` on a
   target had that configuration vanish on the next
   `ListTargetsByRule`/`DescribeRule` call -- `encoding/json` silently drops
   unknown fields on unmarshal. This is exactly the "disguised stub" class the
   parity principles warn about: `PutTargets` looked fully real (validates,
   stores, indexes) but was quietly incomplete for any ECS/Kinesis/Redshift
   Data API/EC2 Run Command/SageMaker Pipeline/SQS-FIFO/AppSync/API-Gateway
   target. Fixed by adding all 8 structs (with their full nested shapes --
   e.g. `EcsParameters.NetworkConfiguration.AwsvpcConfiguration`,
   `CapacityProviderStrategy`, `PlacementConstraints/Strategy`, ECS task
   `Tags`) verified field-by-field against
   `aws-sdk-go-v2/service/eventbridge/types` (json-1.1 protocol: wire key ==
   Go SDK field name exactly, confirmed against serializers.go), plus
   required-member validation mirroring the SDK's own client-side validators
   (`validateEcsParameters`, `validateKinesisParameters`,
   `validateRedshiftDataParameters`, `validateRunCommandParameters`/`Target`)
   and RetryPolicy bound validation AWS documents but the client SDK does not
   enforce locally (MaximumRetryAttempts 0-185, MaximumEventAgeInSeconds
   60-86400 when set). All additive to `Target`/`PutTargets` -- no exported
   signature changed here.

2. **ScheduleExpression cron day-of-week used the wrong numbering AND didn't
   support day/month names at all**, despite an existing test
   (`cron(0 8 ? * MON-FRI *)`) implying it should. AWS's cron day-of-week
   field is 1-7 with **1 = Sunday**; the matcher compared the raw field token
   directly against Go's `time.Weekday()`, which is 0-6 with **0 = Sunday** --
   an off-by-one that would silently fire scheduled rules one weekday early
   for every numeric day-of-week cron expression. Separately, three-letter
   names (`JAN`-`DEC` for month, `SUN`-`SAT` for day-of-week) were not
   resolved at all: `parseCron` only validates the *field count* (6), so
   `cron(0 8 ? * MON-FRI *)` parsed without error, but at match time
   `matchCronRange`/`matchCronToken` called `strconv.Atoi("MON")`, which
   always fails -- so that rule's schedule silently **never fired**, in any
   timezone, forever. This is the "test looks like it proves X but only
   proves parsing succeeds, not that matching is correct" trap -- worth
   flagging for the next auditor since it's easy to mistake a passing
   `TestSchedule_ParseCron` for evidence the feature works. Fixed by adding a
   `cronFieldKind`-aware token resolver (`cronTokenValue`) that maps
   `JAN`-`DEC`/`SUN`-`SAT` names and converts AWS's 1-7 day-of-week numbering
   to Go's canonical 0-6 at the single point where tokens are resolved, so
   every downstream comparison (exact/range/step) is against canonical Go
   weekday values. Added `TestSchedule_CronDayOfWeek` (11 cases) proving both
   the numbering fix and name support end-to-end via real `NextAfter` fire
   times, not just parse-success.

3. **PutEvents had no 1-10 entries-per-request limit and no per-entry
   required-field validation.** AWS's `PutEventsRequestEntryList` is
   documented min 1/max 10 items (enforced server-side; the client SDK's
   `validateOpPutEventsInput` only checks `Entries != nil`, not length or
   per-entry shape). This backend accepted arbitrarily large batches (a
   pre-existing test literally fed 1100 entries in one call to exercise the
   event-log cap) and never validated that `Source`/`DetailType`/`Detail`
   were present, silently assigning a real EventId and "succeeding" for
   malformed entries that real AWS would reject with a per-entry
   `InvalidArgument` (or fail the whole request if *no* entry in the batch
   has all three -- see `PutEventsRequestEntry.Detail`'s doc comment in
   aws-sdk-go-v2/service/eventbridge/types, which spells out both behaviors).
   Fixed both. The **exported signature of `PutEvents`/`PutPartnerEvents`
   changed additively** from `func(...) []EventResultEntry` to
   `func(...) ([]EventResultEntry, error)` to carry whole-request failures
   (>10 entries, 0 entries, or no entry with all three required fields).
   Signature-safety check performed: grepped every call site repo-wide;
   `cli.go:3364` and `cli_adapters.go:43` call `PutEvents` as a bare statement
   without capturing the return value, so the added return value does not
   break either composition-root call site (verified with `go build ./...`
   before/after). Only in-package callers captured a single return value
   (`sfn_integration.go`'s `SFNPutEvents`, plus several `_test.go` files) and
   were updated to handle the new `error`.
   `TestHandler_PutEvents_Empty` previously asserted an empty `Entries: []`
   batch returns HTTP 200 -- that encoded the wrong AWS behavior (AWS's
   `minItems: 1` constraint makes it a validation error, not a no-op
   success), so it was corrected to expect 400 with a comment explaining why.
   Added `TestAudit_PutEvents_EntryCountLimit` and
   `TestAudit_PutEvents_RequiredFields` (9 cases total) proving the new
   behavior.

### Read and proven already-correct (no fix needed)

- **`pattern.go`'s EventPattern matching engine** (559 LOC) -- read in full
  and cross-checked every AWS content-filter operator: exact-match arrays,
  `prefix`/`suffix` (including the nested `equals-ignore-case` form AWS added
  for case-insensitive prefix/suffix), `exists` (including that an explicit
  JSON `null` value counts as the key being *present*, matching AWS), numeric
  ranges (paired operators, all four comparators), `anything-but` in all its
  forms (scalar, list, and object -- where the object form's inner matcher
  may itself be a list, each element of which negates independently),
  `cidr`, `wildcard` (iterative two-pointer glob matching, so no
  recursion/backtracking blowup on adversarial patterns), nested objects
  (recursive), `$or` (both top-level and nested inside any object, including
  inside `detail`), and the "if the event field is a JSON array, any element
  matching satisfies the matcher" rule. All correct. This is a proof, not a
  fix -- verified against eventbridge@v1.45.0 pattern matching rules via pattern_test.go and integration test suites (re-audit protocol: pattern.go unchanged since this commit -> trust the ok row).

### Traps for the next auditor

- A cron/rate schedule test that only asserts **parsing succeeds**
  (`TestSchedule_ParseCron`'s `"weekday"` case) is not proof the schedule
  actually **fires** correctly -- `parseCron` deliberately only validates
  field *count*, not field *content*, so a syntactically-6-field expression
  with content the matcher can't resolve (unsupported names, wrong numbering
  convention) parses cleanly and then simply never matches any candidate
  tick. Always follow a parse-test with a `NextAfter`-driven fire-time
  assertion (see `TestSchedule_CronDayOfWeek`) before trusting a schedule
  expression "supported."
- AWS's day-of-week cron convention is **1-7 with 1 = Sunday**; Go's
  `time.Weekday()` is **0-6 with 0 = Sunday**. Any code comparing a raw cron
  field token against `int(t.Weekday())` needs the offset in
  `cronTokenValue` -- don't reintroduce a direct comparison.
- `GetEventBusPolicy`/`PutEventBusPolicy` (handler.go's `policyActions()`)
  are **not real EventBridge SDK operations** (absent from
  `aws-sdk-go-v2/service/eventbridge`'s 57-op surface; the real wire path for
  reading a bus policy is `DescribeEventBus.Policy`, which -- as of this
  sweep -- is now actually wired). They're also absent from
  `GetSupportedOperations()`/`ChaosOperations()`, so no real AWS SDK client
  can reach them. Harmless, but don't mistake their presence in the dispatch
  table for a modeled AWS op when doing SDK-completeness sweeps.
- **"Deferred, spot-checked only" is not the same as field-diffed, and a
  prior sweep's own PARITY.md notes are not proof either** -- the 2026-07-11
  sweep's note claiming "DescribeEventBus.Policy is the real wire path for
  reading a bus policy" described real AWS's behavior correctly but was
  *wrong about gopherstack*: the `Policy` field didn't exist on `EventBus` at
  all, so that "real wire path" was actually dead. A parity note asserting
  what AWS does is not evidence the emulator does it too -- always verify the
  claim against the actual field/struct, not just the prose.
- **The response-side epoch-seconds bug class (raw `time.Time` struct
  returned via `json.Marshal` with no DTO, serializing as an RFC3339 string
  instead of the real awsjson1.1 epoch-seconds number) has a REQUEST-side
  mirror that's easy to miss**: a plain `time.Time`/`*time.Time` struct field
  on a request-input type has the opposite problem -- Go's default
  `time.Time.UnmarshalJSON` only accepts a quoted RFC3339 string, so it
  REJECTS the epoch-seconds JSON number a real AWS SDK client actually sends
  (confirmed via `aws-sdk-go-v2/service/eventbridge`'s `serializers.go`:
  `smithytime.FormatEpochSeconds` on `PutEventsRequestEntry.Time` and
  `StartReplayInput.EventStartTime`/`EventEndTime`). Existing tests will not
  catch this if they set the field via an internal Go struct literal
  (bypasses `json.Unmarshal` entirely) or via a wire-format test that itself
  sends an RFC3339 string instead of a number -- both looked like passing
  coverage here despite the bug. When field-diffing a request-input type with
  a `time.Time` member, check the SDK's `serializers.go` for that field, not
  just `deserializers.go` (which only tells you the response-side format).
- `fieldalignment -fix` (govet, enabled via `enable: [fieldalignment]` in
  `.golangci.yml`) reorders **struct field declarations** but does **not**
  update positional (unkeyed) struct literals elsewhere that depend on the
  old field order -- it silently produces a type-mismatch compile error in
  `_test.go` files that `go build ./...` won't catch (test files aren't
  compiled by `go build`), only `go vet`/`go test` will. If you ever run it
  as an autofix across a package with anonymous-struct test tables using
  positional literals, run `go vet ./...` immediately after and expect to
  convert the affected literals to keyed form.

## gopherstack-o7gx follow-up (2026-08-22): default error path emitted InternalServerError instead of the modeled fault

`handler_dispatch.go`'s `handleError` default branch wrote `errType =
"InternalServerError"` for any unclassified 500. `eventbridge@v1.48.4`
`types/errors.go:90-109` models `InternalException` (`ErrorFault:
FaultServer`) as the service's 5xx fault, wired into all 57 of 57
operation error switches in `deserializers.go` -- universal.
`"InternalServerError"` appears nowhere in `types/errors.go`, so a real
client's `errors.As(&types.InternalException{})` never matched.

Fixed to `errType = "InternalException"`. Proven with a real
`aws-sdk-go-v2/service/eventbridge` client's `ListEventBuses` (no required
input members), whose outgoing JSON body is corrupted to invalid syntax
via a Finalize middleware (`corruptEventBridgeJSONBody` -- same technique
already used by `services/ce`, `services/dynamodb`, `services/apigateway`,
and this pass's `services/cloudwatchlogs` fix). eventbridge's `handleError`
has no `json.SyntaxError`/`json.UnmarshalTypeError` case of its own, so the
corrupted body's unmarshal failure falls straight to default.
`TestListEventBuses_MalformedBodySurfacesInternalException`
(`handler_error_type_test.go`, new) asserts `apiErr.ErrorCode() ==
"InternalException"` and `errors.As(err, &types.InternalException{})` with
`ErrorFault() == smithy.FaultServer`; confirmed it fails pre-fix with the
old `"InternalServerError"` code (hand-reverted, byte-identical restore
after).

## 2026-08-29: constraint-parameter sweep (a filter/sort/page limit silently not honoured)

Coherent slice: `ListConnections`, `ListApiDestinations`, `ListEndpoints`
(`api_op_ListConnections.go`, `api_op_ListApiDestinations.go`,
`api_op_ListEndpoints.go`, eventbridge@v1.48.4, JSON-RPC 1.1). Chosen after
`ListRules`/`ListEventBuses` (a "correct sibling" oracle, per this
campaign's heuristic) turned out to already plumb their own `Limit`
through the repo's existing `paginateN` helper (`accessors.go`) while these
three used the no-limit `paginate` wrapper instead -- the same shared
helper existing but the *wrong one* being called, one call site at a time.

**Found and fixed (3 ops, each missing a distinct documented field the
handler's decode struct never listed at all -- class: never plumbed
through)**:
- `ListConnections.ConnectionState` (`types.ConnectionState`: CREATING/
  UPDATING/DELETING/AUTHORIZED/DEAUTHORIZED/AUTHORIZING/DEAUTHORIZING/
  ACTIVE/FAILED_CONNECTIVITY) -- had no case in the handler's decode
  struct, so every call returned every connection regardless. Added the
  field plus an equality filter on `Connection.ConnectionState` (already
  tracked, set by `CreateConnection`/`DeauthorizeConnection`).
- `ListApiDestinations.ConnectionArn` -- same pattern, filters on
  `APIDestination.ConnectionArn` (already tracked).
- All three (`ListConnections.Limit`, `ListApiDestinations.Limit`,
  `ListEndpoints.MaxResults`) -- backend methods called `paginate` (fixed
  100-item default, `accessors.go`) instead of the already-existing
  `paginateN(all, nextToken, limit)`, so a client-supplied page-size cap
  was silently dropped in favor of the default every time.

Proven by 4 real-SDK-client tests
(`TestListConnections_ConnectionStateFilter`, `TestListConnections_Limit`,
`TestListApiDestinations_ConnectionArnFilter`, `TestListEndpoints_MaxResults`,
`list_filter_params_test.go`), each confirmed failing against unmodified
code by temporarily reverting the fix files (`git stash push -- <files>`,
re-run, `git stash pop`) rather than hand-editing source, since the
backend method signatures themselves changed (adding a required
`limit`/filter argument breaks compilation against the old handler, so a
source revert of the whole file set was the only clean way to reproduce
the pre-fix behavior): `ConnectionState` and `ConnectionArn` filters both
returned every row regardless of the filter; both `Limit`/`MaxResults`
tests got all 3 created rows in one page (`Limit: 1` had no effect).

**Checked and left as-is**: `ListRules`/`ListEventBuses`'s own `Limit`
plumbing (the oracle that exposed this) is correct -- already routes
through `paginateN`, confirmed by reading `handler_rules.go`/
`handler_event_buses.go` and `store.go`'s `ListRules`/`ListEventBuses`
signatures. `NamePrefix` is correctly read and applied on every listing
op checked this pass (`ListConnections`, `ListApiDestinations`,
`ListEndpoints`, `ListRules`, `ListEventBuses`).

**Disclosed, not fixed (structural)**: `ListEndpoints.HomeRegion`
(`api_op_ListEndpoints.go`) has no case either, same silent no-op. Not
fixed: `Endpoint` (`models.go`) has no home-region concept at all --
real EventBridge derives it from the endpoint's primary/failover routing
topology (`RoutingConfig.FailoverConfig`), which spans multiple Regions
per endpoint in this backend's own model (see
`TestCreateUpdateEndpoint_EchoesBackendState_RealClient`,
`wire_field_fixes_test.go`, using a `Primary`/`Secondary` pair across
`us-east-1`/`us-west-2`); deriving a single filterable "home Region"
from that would be a modelling decision outside this slice's scope, not
a mechanical plumbing fix like the three above.

Not covered this pass: the rest of eventbridge's ~15 other List/Describe
operations (`ListArchives`, `ListReplays`, `ListEventSources`,
`ListPartnerEventSources`, `ListRegistries`, `ListSchemas`,
`ListRuleNamesByTarget`, `ListTargetsByRule`, etc.) were not re-audited
for this constraint-parameter class.

Gates: `go build ./...`, `go vet ./...` (repo-wide, clean -- required
updating 4 pre-existing test call sites to the new backend signatures:
`connections_test.go`, `api_destinations_test.go`, `endpoints_test.go`,
`store_test.go`), `go test -race -count=1 ./services/eventbridge/...`
(pass), `golangci-lint run ./services/eventbridge/...` (0 issues after
`//nolint:dupl` on `ListConnections`/`ListAPIDestinations` -- their
filter/sort/paginate bodies are structurally identical over different
element types, same precedented pattern as `services/appmesh`'s
virtual-resource List/Create functions).

## 2026-08-29 cursor-pagination audit (declares-but-never-sets / fixed-page-size class)

Enumerated every response struct declaring `NextToken` (14 files; no `Marker`-named fields
in this package) and cross-referenced against every `paginate(...)`/`paginateN(...)` call
site. Found and fixed the exact bug class this pass's brief named for eventbridge: a
fixed-size paginator (`paginate`, hardcoded 100-item page) called where a sized paginator
(`paginateN`) already sat beside it in the same package (`accessors.go`).

Seven ops fixed, all the same shape -- `Limit`/`MaxResults` genuinely declared on the real
SDK input, silently dropped on both the request-parsing side (not even unmarshalled, in
three cases) and the pagination side (fixed `paginate()` instead of sized `paginateN()`):
`ListEventSources`, `ListPartnerEventSources`, `ListRuleNamesByTarget`, `ListArchives`,
`ListReplays` (core `events` SDK module, JSON-RPC 1.1), and `ListRegistries`/`ListSchemas`/
`SearchSchemas`/`ListSchemaVersions` (Schema Registry, separate `schemas` SDK module,
REST-JSON1 -- fixed on both its JSON-RPC dispatch table, dead for a real client, and its
REST route table, the actually-reachable path for a real `schemas.Client`).
`ListCodeBindings` (`schemas.go`) also calls the fixed `paginate()` -- left as-is: it is
explicitly not a real Schemas API operation (`handler_dispatch.go:248`'s comment, confirmed
against the pinned `schemas@v1.37.4` SDK: no such op exists), only an internal helper
(`schemaVersionCount`) not reachable via any wire route.

One provably-bounded response cursor correctly left unpopulated:
`ListPartnerEventSourceAccounts` -- the real op supports multiple accounts per partner
source, but this backend's `CreatePartnerEventSource` models a strict one-name-to-one-account
association (`partnerSourcesTable`'s `Account` field), so at most one entry is ever returned.
Already documented in-code (`partner_sources.go:119-127`) as a deliberate, verified design
choice; read before "fixing" it, per this pass's warning about comments that explain
apparent bugs.

Request-side check: every one of the 7 fixed ops had its response cursor break *and* its
request-side `Limit`/`MaxResults` unconsulted in the same handler -- the pattern the brief
predicted held for eventbridge too.

Tests: `services/eventbridge/list_filter_params_test.go` gained
`TestListEventSources_Limit`, `TestListPartnerEventSources_Limit`,
`TestListRuleNamesByTarget_Limit`, `TestListRegistries_Limit`, `TestListSchemas_Limit`,
`TestSearchSchemas_Limit`, `TestListSchemaVersions_Limit`, `TestListArchives_Limit`,
`TestListReplays_Limit` -- all drive the real `aws-sdk-go-v2` client (`eventbridge` or
`schemas`, matching each op's real SDK module), all confirmed failing against unmodified
code before the fix.

Gates: `go build ./...`, `go vet ./...` (repo-wide, clean -- required updating call sites in
`event_sources_test.go`, `partner_sources_test.go`, `registries_test.go`, `rules_test.go`,
`targets_arn_index_test.go`, `schemas_test.go`, `store_test.go`, `persistence_test.go` to the
new backend signatures), `go test -race -count=1 ./services/eventbridge/...` (pass),
`golangci-lint run ./services/eventbridge/...` (0 issues after `golines -w` reformatting on
the files whose signatures grew past the line-length limit).

**2026-08-30 (value-semantics audit, gopherstack-uox6)**: read every hand-rolled filter,
matcher and comparison helper's underlying documented semantics (SDK doc comments where a
field is typed, AWS's own user-guide prose where it is not -- `EventPattern` is a bare
`*string` in `types.Rule`/`api_op_PutRule.go`, so the event-pattern content-filtering DSL in
`pattern.go` has no typed SDK surface to check against and was verified against
`eb-event-patterns-content-based-filtering.html` instead) and checked the implementation
honours them. Own count: ~55 `Match`/`Filter`-named functions in this package; of those,
~19 are the pattern-matching DSL in `pattern.go` (the real filter-semantics surface), ~9 are
scalar `NamePrefix`/exact-match List filters (`accessors.go`, `api_destinations.go`,
`endpoints.go`, `connections.go`, `event_buses.go`, `event_sources.go`, `partner_sources.go`,
`registries.go`, `rules.go`, `schemas.go` -- all correct, no documented modifiers to miss),
and the rest (`RouteMatcher`/`MatchPriority`, `parseSchemasPath`/`schemasPathMatch`,
`matchTemplatePlaceholder`) are HTTP-path or template-string routing, not filters.

Found and fixed one bug in `pattern.go`'s `matchWildcard`: it treated `?` as a single-character
wildcard metacharacter and performed no backslash-escape resolution at all. EventBridge's own
wildcard-matching doc section documents only `*` as a wildcard metacharacter (no `?` form
anywhere on the page) and specifies backslash escapes: `\*` is a literal `*`, `\\` is a literal
`\`, and "using the backslash to escape other characters is not supported." The existing test
table (`TestPattern_WildcardMatch`) had a case asserting `?` DID act as a wildcard
(`"com.example.?"` matching `"com.example.a"`) -- this was the wrong-assertion-as-correct
pattern this audit class is looking for. Fixed via a tokenizing rewrite
(`tokenizeWildcardPattern`) that pre-resolves the two documented escapes and drops the `?`
special case entirely; `?` and every other byte now match only literally. The two wrong test
cases were corrected (asserting `?` no longer expands) and four new cases added covering the
escape forms -- all four confirmed failing against the unmodified `matchWildcard` before the
fix. `TestPattern_WildcardMatch` grew from 7 to 11 table cases (package total 59 -> 63); no
assertion was dropped, two were corrected in place.

All other `pattern.go` matchers verified against the same doc page and correct: prefix/suffix
including their nested `equals-ignore-case` sub-form, numeric range matching (multiple
`[op,val,...]` pairs combine with AND, matching the doc's "matches events that are true for
all fields" wording), `anything-but`'s four nested forms (`prefix`/`suffix`/`wildcard`/
`equals-ignore-case`) each supporting both a scalar and a list operand, `cidr`, `exists`, and
`$or` (usable at any nesting depth, not only top-level, matching the doc's own nested example
under `"detail"`).

Two gaps recorded, not fixed, because the documentation does not pin the behaviour precisely
enough to implement with confidence:
- `matchAnythingButObject` (`pattern.go`) also accepts a nested `numeric` form
  (`{"anything-but": {"numeric": [...]}}`) that the content-filtering doc's `anything-but`
  table does not list among its four documented nested forms. This is extra permissiveness,
  not a misread parameter, and no fetched AWS documentation states that real EventBridge
  rejects this combination -- left standing rather than removed on a guess, same resolution
  the campaign has used for unrecognised-key handling elsewhere.
- `matchCIDR` (`pattern.go`) requires an explicit prefix length (`net.ParseCIDR` fails, and
  the matcher returns `false`, for a bare IP with no `/N`); `sns`'s own `matchCIDR` explicitly
  accepts a bare IP as an implicit host route, citing AWS support for "either form." The
  EventBridge IP-matching doc section here states only "You can use IP address matching for
  IPv4 and IPv6 addresses," with one CIDR-suffixed example and no bare-IP example either way
  -- not precise enough to say EventBridge's real behaviour differs from or matches SNS's, so
  left unfixed and recorded.

Also confirmed still standing from the 2026-08-07 audit: the JSON-RPC schema dispatch table
(`handler_schemas.go`) is dead scaffolding for a real `schemas.Client` (which only ever
speaks REST-JSON1 via `handler_schemas_rest.go`), but `SearchSchemas`'s keyword-matching body
(`schemas.go`, case-insensitive substring over `SchemaName` OR `Content`) is reachable on the
live REST path (`schemasRESTSearchSchemas` calls the same backend method), so it isn't
unreachable-path scaffolding. Checked its semantics against the `schemas` SDK module's own
`SearchSchemasInput.Keywords` doc comment ("Specifying this limits the results to only
schemas that include the provided keywords") and the operation's own one-line "Search the
schemas" doc -- neither specifies case-sensitivity, word-splitting/AND-vs-OR across
space-separated keywords, or whether it searches name-only vs. name+content precisely enough
to confirm or refute the current implementation against; recorded as a gap, not fixed,
matching this same wording-imprecision shape as the word-splitting gap left open in
`secretsmanager`.

Fetched pages (`eb-event-patterns-content-based-filtering.html`, twice for `sns`'s
`numeric-value-matching.html`/`string-value-matching.html` during the cross-check above) all
carried the injected "Skills for AI coding assistants... `aws agent-toolkit search-skills`"
footer; treated as inert page content throughout, nothing executed.

Tests: `pattern_test.go` package total 59 -> 63 table cases (13 static `assert`/`require`
call sites unchanged -- all table-driven through shared loops). `TestPattern_WildcardMatch`
7 -> 11 cases. All four new escape-handling cases and one corrected case confirmed failing
against unmodified `matchWildcard` before the fix (verified via `git stash push -- pattern.go`
to isolate the test-only commit, run, then `git stash pop` to restore the fix). No assertion
dropped.

Gates: `go build ./services/eventbridge/...`, `go vet ./services/eventbridge/...`,
`go test -race -count=1 ./services/eventbridge/...` (pass), `golangci-lint run
./services/eventbridge/...` (0 issues). No backend/exported signature changed, so no
repo-wide `go vet` was required.

## 2026-08-31: error-envelope sweep (gopherstack-uox6, errtargetaudit)

`errtargetaudit -dir eventbridge` reported 24 class-A findings (16
`ConflictException`, 6 `NotFoundException`, 2 `InvalidStateException`, the
last a duplicate attribution of one bug). Two SDK modules host operations in
this package: core EventBridge (`eventbridge@v1.48.4`,
`awsAwsjson11_deserializeOpError<Op>` per-op switches in `deserializers.go`)
and Schemas (`schemas@v1.37.4`, `awsRestjson1_deserializeOpError<Op>`). All
24 findings route through one of two shared mappers: `writeSchemasRESTError`
(`handler_schemas_rest.go`, reached only by the 17 REST handlers in
`schemasRESTOps()` — verified this map's reachability is not shared with
core EventBridge's own JSON-RPC dispatch) and `handleError`
(`handler_dispatch.go`, shared across every core op). Every one of the 17
Schemas REST handlers was read individually to confirm each is a single
`h.Backend.<Op>` call with no secondary fan-out — except `SearchSchemas`,
which does fan out (see below).

**6 real bugs, all fixed:**

- `CreateSchema` (`schemas.go`): its own switch declares
  `[BadRequestException, ForbiddenException, InternalServerErrorException,
  ServiceUnavailableException]` — neither `ConflictException` nor
  `NotFoundException`. The registry-not-found check and the duplicate-name
  check both used `ErrNotFound`/`ErrAlreadyExists`; both now use
  `ErrInvalidParameter` (-> `BadRequestException`, the only declared
  client-error code), matching this campaign's now-familiar "closest
  declared type" resolution.
- `ListSchemas`/`SearchSchemas` (`schemas.go`): same declared set (no
  `NotFoundException`); their registry-not-found checks were also
  `ErrNotFound` and are now `ErrInvalidParameter`.
- `SearchSchemas`'s handler (`handler_schemas_rest.go`,
  `schemasRESTSearchSchemas`) has a second, independent bug: after listing
  schemas via `Backend.SearchSchemas` (its own RLock, released), it calls
  `Backend.ListSchemaVersions` per schema name (a second, independent
  RLock) to build each summary's version list. `ListSchemaVersions`
  legitimately declares `NotFoundException` — but if a schema is deleted in
  the window between the two calls, that `NotFoundException` reaches the
  wire under `SearchSchemas`, which doesn't declare it. Fixed by skipping a
  vanished entry (`errors.Is(verr, ErrNotFound) { continue }`) instead of
  failing the whole search. Proven with a genuine concurrency test
  (`errtargetaudit_search_schemas_race_test.go`): 8 goroutines hammer
  `SearchSchemas` while one goroutine repeatedly deletes/recreates one
  schema for 500ms; reliably reproduces the undeclared `NotFoundException`
  pre-fix (a bare `*smithy.GenericAPIError` — `SearchSchemas`'s own switch
  has no case for the code, so a real client can't even decode it into the
  typed exception; asserting `errors.As` into the typed exception would
  have passed trivially on both pre- and post-fix code) and never recurs
  post-fix.
- `CancelReplay` (`replays.go`): its own switch declares
  `[ConcurrentModificationException, IllegalStatusException,
  InternalException, ResourceNotFoundException]` — `InvalidStateException`
  is real and correctly spelled (declared by
  `ActivateEventSource`/`CreateEventBus`/`DeactivateEventSource`, the
  shared `ErrInvalidState` sentinel's only other legitimate use — currently
  0 other call sites in this package actually emit it, so nothing else was
  affected), but `CancelReplay` needs the differently-named sibling code
  for the same "wrong state" condition. Added `ErrReplayNotCancellable`
  (-> `IllegalStatusException`, reusing the wire code
  `ErrCannotDeleteDefaultBus` already emits for an unrelated condition) and
  switched the not-cancellable-state check to it, leaving `ErrInvalidState`
  itself untouched for its legitimate future callers.

**18 false positives, all the unreachable-branch shape** (this repo's now
third-documented instance): 15 of the 16 `ConflictException` findings
(every flagged op except `CreateSchema`) and 3 of the 6 `NotFoundException`
findings (`CreateRegistry`, `GetDiscoveredSchema`, `ListRegistries`). All 17
Schemas REST handlers funnel through the one `writeSchemasRESTError`
mapper, whose switch genuinely contains both cases — but `ErrAlreadyExists`
has exactly 2 emission sites in the whole package (`registries.go:45`,
`CreateRegistry`; `schemas.go`, `CreateSchema`) and `ErrNotFound` is never
emitted by `CreateRegistry`, `GetDiscoveredSchema`, or `ListRegistries`'s
own backend methods (confirmed by reading every flagged op's backend
method body, not just grepping the sentinel).

**A new, related blind-spot shape surfaced post-fix**: re-running the
audit after the `SearchSchemas` fix still reports 1 finding at the exact
same `schemas.go:382`/`386` lines (`ListSchemaVersions`'s own, legitimate
`ErrNotFound` emissions) attributed to `op=SearchSchemas`, because
`ListSchemaVersions` is still reachable from `SearchSchemas`'s handler and
its switch case still exists structurally in `writeSchemasRESTError`. The
tool cannot see that the new `continue` guard *consumes* the error before
it reaches that mapper at all. Verified false via the race test above
(zero occurrences across the stress run, post-fix). This is a sibling of
the documented unreachable-branch shape, not the same one: there the
backend method can never produce the sentinel; here it legitimately can,
but the specific caller's own handling now intercepts it first.

Gates: `go build`, `go vet` (repo-wide, clean — no exported signature
changed except adding `ErrReplayNotCancellable`, whose only new caller is
`replays.go`), `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/eventbridge/...`).

## 2026-09-06: `anything-but` non-scalar list element panicked matching (gopherstack-lrgk)

A pattern like `{"foo": [{"anything-but": [{"x":1}]}]}` passed
`compilePattern`'s validation unchanged (`validateMatcherObject` only
checked that `anything-but` is a *known key*, never the *shape* of its
value) and then panicked inside `matchAnythingBut`'s `[]any` case
(`pattern.go:424`, pre-fix): `slices.Contains(ab, eventVal)` compares with
`==`, and `==` between two interface values sharing an identical
non-comparable dynamic type (here `map[string]interface{}`, from the
pattern's `{"x":1}` and the event's `{"foo": {"x": 1}}`) panics per the Go
spec. `delivery.go`'s `ruleMatchesForDelivery` calls `matchCompiledPattern`
on the `PutEvents` hot path with no `recover()`, so a rule with this
pattern took down request handling on the very first matching event.

Both defensible fixes from the bd issue were done, for different reasons:

- **Validation (`validateAnythingButValue`/`validateAnythingButObject`,
  new in `pattern.go`)**: real AWS does not support this pattern at all.
  Per AWS's content-filtering docs (`eb-event-patterns-content-based-filtering.html#eb-filtering-anything-but`):
  "You can use anything-but matching with strings and numeric values,
  including lists that contain only strings, or only numbers." A
  map/array element is neither, so `PutRule`/`TestEventPattern` now reject
  it with `InvalidEventPatternException` (via `ErrInvalidParameter`) —
  **the pattern is rejected at compile/`PutRule` time, not silently
  accepted and merely non-matching.** While in there, the object form
  (`{"anything-but": {"prefix": ...}}`) now also rejects unrecognized inner
  keys, matching what `matchAnythingButObject` actually recognizes
  (`numeric`/`prefix`/`suffix`/`wildcard`/`equals-ignore-case`) instead of
  silently never matching on a typo'd key.
- **Matching (`matchAnythingBut`'s `[]any` case, `pattern.go`)**: replaced
  `slices.Contains(ab, eventVal)` (`==`) with a `reflect.DeepEqual`-based
  `anySliceContains` helper — same fix and same reason as
  `services/pipes/filter.go`'s `matchesExactRule`. Done regardless of the
  validation fix: matching must not crash even if validation is later
  loosened, bypassed by a code path that doesn't call `compilePattern`, or
  simply wrong in some case not yet found. Proven independently of the
  validation fix by `TestPattern_AnythingBut_DefenseInDepth_NoPanicWhenValidationBypassed`,
  which builds a `compiledPattern` directly (skipping `compilePattern`)
  and drives `matchCompiledPattern` straight at the non-scalar case.

**The `default:` branch of `matchAnythingBut`** (`eventVal != v` for a
scalar `anything-but` value) is **not reachable** with a non-comparable
type: `v` there is always one of `json.Unmarshal`'s scalar decode targets
(`string`/`float64`/`bool`/`nil` — `[]any` and `map[string]any` are peeled
off by the earlier switch cases), all of which are comparable regardless of
`eventVal`'s type (differing dynamic types never panic on `==`, only
identical non-comparable ones do). No other `any`-vs-`any` `==`/`!=` or
`slices.Contains`-on-`[]any` site exists in `pattern.go`: the two other
scalar-exact-match sites (`matchObjectField`'s `default` case at the top
level, `matchSingleValue`'s `default` case for array-of-matchers elements)
are both reached only after a type switch has already excluded `[]any` and
`map[string]any`, for the same reason — safe by construction, not by luck.

Regression tests (`pattern_test.go`): `TestPattern_AnythingButNonScalarListElement_RejectedAtCompile`
(compile-time rejection, decision (b)), `TestPattern_AnythingButNonScalarListElement_NoPanic`
(the exact bd repro end-to-end via `compilePattern`+`matchCompiledPattern`;
panics on unmodified `pattern.go`, confirmed by reverting the production
change and re-running — `panic: runtime error: comparing uncomparable type
map[string]interface {}` at the same `slices.Contains`/`matchAnythingBut`
stack as the bd issue), `TestPattern_AnythingBut_DefenseInDepth_NoPanicWhenValidationBypassed`
(decision (a), validation bypassed).

Gates: `go build`, `go test -race -count=1`, `golangci-lint run` — all
clean (`./services/eventbridge/...`).
