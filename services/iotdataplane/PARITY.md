---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: iotdataplane
sdk_module: aws-sdk-go-v2/service/iotdataplane@v1.35.4   # bumped from v1.32.20; +3 new ops (device connection/messaging introspection)
last_audit_commit: 18d7bc9b8   # HEAD at time of this pass (pre-commit of this pass); one bug fixed (ListSubscriptions pagination)
last_audit_date: 2026-09-04
overall: A            # restored from A-: ListSubscriptions now reports real per-client subscriptions and SendDirectMessage now truly addresses one client, both read/written through a new MQTTPublisher.ClientSubscriptions/SendToClient boundary implemented in services/iot/broker.go off mochi-mqtt's real cl.State.Subscriptions/cl.WritePacket -- see gaps for the one remaining honest divergence (fallback broadcast when the broker has no live session for a gopherstack-admin-tracked clientId)
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  GetThingShadow: {wire: ok, errors: ok, state: ok, persist: ok, note: "404 on deleted (tombstoned) shadow now correctly excluded"}
  UpdateThingShadow: {wire: ok, errors: ok, state: ok, persist: ok, note: "ConflictException (was VersionConflictException); RequestEntityTooLargeException (was InvalidRequestException/plain-413) for >8KB doc; version continues across delete+recreate; state.desired/state.reported now enforce AWS's documented 8-level JSON nesting depth cap; invented maxShadowsPerThing=100 cap REMOVED (no such AWS quota exists -- see notes)"}
  DeleteThingShadow: {wire: ok, errors: ok, state: ok, persist: ok, note: "response now omits state (empty response state document, AWS-doc-confirmed); soft-delete tombstone preserves version continuity"}
  ListNamedShadowsForThing: {wire: ok, errors: ok, state: ok, persist: ok, note: "excludes tombstoned (deleted) named shadows"}
  Publish: {wire: ok, errors: ok, state: ok, persist: n/a, note: "parses+validates the full PublishInput wire surface (contentType/messageExpiry/responseTopic as query params; correlationData/payloadFormatIndicator/userProperties as X-Amz-Mqtt5-* headers, per serializers.go); userProperties persists onto the retained message (see GetRetainedMessage); RESOLVED this pass (gopherstack-76fj): every field now also reaches the broker as a real MQTT5 packet property via the new MQTTPublisher.PublishWithProperties(topic,payload,retain,qos,MQTT5Properties), implemented in services/iot/broker.go off mochi-mqtt's Server.InjectPacket -- a v5-connected subscriber genuinely observes contentType/correlationData/messageExpiry/payloadFormatIndicator/responseTopic/userProperties on the wire (mochi-mqtt encodes packet Properties only when the *receiving* client negotiated protocol version 5, packets.Packet.PublishEncode gate, so a v3.1.1 subscriber sees the same message Publish always delivered). Also fixed in the same pass: correlationData was accepted as opaque unvalidated text (never base64-decoded despite AWS documenting it as base64-encoded binary) and userProperties' JSON-array-of-single-key-objects shape was never validated -- both now produce InvalidRequestException on malformed input. ErrNoBroker path still logs+drops when no broker is wired."}
  DeleteConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "REAL AWS op restored to its real wire path DELETE /connections/{clientId} (was regressed to /_admin/-only in a prior 'AWS-accuracy' pass); admin alias kept for test convenience; now returns ResourceNotFoundException (was an unconditional no-op) when clientId has no tracked connection -- real AWS models this error for DeleteConnection"}
  GetRetainedMessage: {wire: ok, errors: ok, state: ok, persist: ok, note: "response now includes userProperties (base64, null when unset) -- was missing entirely; confirmed against GetRetainedMessageOutput"}
  ListRetainedMessages: {wire: ok, errors: ok, state: ok, persist: ok, note: "summary now includes qos -- a prior audit incorrectly asserted RetainedMessageSummary excludes qos; the real deserializer (awsRestjson1_deserializeDocumentRetainedMessageSummary) proves it's present"}
  GetConnection: {wire: ok, errors: ok, state: partial, persist: ok, note: "NEW op (GET /connections/{clientId}, real path field-diffed against serializers.go/deserializers.go). Reuses the same connections table DeleteConnection already tracks (gopherstack-only RegisterConnection admin extension) -- an untracked clientId is ResourceNotFoundException (matches the real op's modeled error), a tracked one returns connected:true/clientId/connectedSince genuinely. cleanSession/disconnectReason/disconnectedSince/keepAliveDuration/sessionExpiry/sourcePort/targetIp/targetPort/thingName/vpcEndpointId have no real backing data in this emulator and are omitted from the response (not fabricated as zero values) -- see gaps"}
  ListSubscriptions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /connections/{clientId}/subscriptions. Errors/not-found semantics reuse the connections table (consistent with GetConnection/DeleteConnection). subscriptions now reflects the client's REAL live MQTT subscriptions: InMemoryBackend.ListSubscriptions calls the new MQTTPublisher.ClientSubscriptions(clientID), implemented in services/iot/broker.go off mochi-mqtt's cl.State.Subscriptions.GetAll() (topicFilter+qos, field-diffed against types.SubscriptionSummary). A tracked clientId whose broker session the broker doesn't currently know about (no broker wired, or gopherstack's admin-only RegisterConnection registered it without a real MQTT socket connection -- a distinct, weaker notion of 'connected' than a live broker session) still honestly returns an empty list rather than fabricating entries -- see gaps. FIXED (parity sweep 2026-09-04): maxResults/nextToken -- real ListSubscriptionsInput query params (awsRestjson1_serializeOpHttpBindingsListSubscriptionsInput in serializers.go) -- were parsed nowhere; handleListSubscriptions always returned every subscription in one page and never emitted nextToken. Now paginated the same way as the other list ops (findCursorIndex/parsePageSize), honoring the documented MaxResults default of 20 (ListSubscriptionsInput.MaxResults doc comment) rather than the generic defaultPageSize=25 used elsewhere in this service."}
  SendDirectMessage: {wire: ok, errors: ok, state: ok, persist: n/a, note: "POST /connections/{clientId}/messages, field-diffed against serializers.go's awsRestjson1_serializeOpHttpBindingsSendDirectMessageInput. Validates clientId/topic exactly like GetConnection/Publish and returns ResourceNotFoundException for an untracked clientId (413 RequestEntityTooLargeException on oversized payload -- unlike Publish, this IS modeled for SendDirectMessage, confirmed via its error case list). Delivers via MQTTPublisher.SendToClientWithProperties(clientId,...) when the broker has a live session for that client -- a genuine per-client-addressed write (services/iot/broker.go's cl.WritePacket, bypassing subscription matching entirely, matching AWS's documented 'the receiving client does not need to subscribe to the topic'). Falls back to PublishWithProperties (topic broadcast) only when the broker has no live session for a tracked clientId. RESOLVED this pass (gopherstack-76fj): SendDirectMessageInput shares its contentType/correlationData/payloadFormatIndicator/responseTopic/userProperties wire locations with PublishInput (confirmed identical query/header names in the SDK's serializer) but contentType and correlationData were never even parsed for this op before -- now reuses Publish's parseMQTT5PublishParams and forwards every field to the broker on both the direct-send and broadcast-fallback paths, same as Publish (SendDirectMessageInput has no messageExpiry field, unlike PublishInput)."}
families:
  admin-only-extensions: {status: ok, note: "RegisterConnection/ListConnections/ListThingsWithShadows have NO real AWS iotdataplane equivalent (confirmed against the SDK's op file listing); correctly confined to gopherstack-only paths (/_admin/connections, /api/things/shadow/ListThingsWithShadows) so they cannot shadow real AWS traffic"}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "RESOLVED this pass (gopherstack-76fj): Publish with no MQTT broker wired still logs a warning and silently drops the message (ErrNoBroker path in backend.go Publish()) -- that part is intentional degradation, not a disguised no-op. But the rest of this gap is closed: MQTTPublisher (services/iotdataplane/interfaces.go) now carries PublishWithProperties/SendToClientWithProperties(...,MQTT5Properties) alongside the original topic/payload/retain/qos-only Publish/SendToClient, implemented in services/iot/broker.go via mochi-mqtt's Server.InjectPacket/Client.WritePacket with real packets.Properties attached (ContentType/ResponseTopic/CorrelationData/MessageExpiryInterval/PayloadFormat/User). contentType/correlationData/messageExpiry/payloadFormatIndicator/responseTopic/userProperties now all reach a v5-connected live subscriber as real MQTT5 packet properties, for both Publish and SendDirectMessage. Proven two ways: (1) Test_Publish_MQTT5Fields_ForwardedToBroker / Test_SendDirectMessage_MQTT5Fields_ForwardedToBroker (services/iotdataplane) assert the exact MQTT5Properties value reaching a mock MQTTPublisher; (2) Test_Publish_DeliversThroughRealBroker / Test_SendDirectMessage_DeliversThroughRealBroker connect a real paho MQTT 3.1.1 client to a real mochi-mqtt broker (services/iot) and confirm delivery is not regressed -- this pass could not add a live MQTT5-capable client (none of this repo's pinned dependencies speak MQTT5; paho.mqtt.golang v1.5.1 is 3.1.1-only), so the properties' on-wire presence for a v5 client rests on reading mochi-mqtt's own encode path (packets.Packet.PublishEncode gates property encoding on the *receiving* client's negotiated ProtocolVersion==5, github.com/mochi-mqtt/server/v2@v2.7.9/packets/packets.go:623, set from cl.Properties.ProtocolVersion in clients.go's WritePacket:543) rather than an end-to-end MQTT5 wire capture. No AWS-modeled response surface within iotdataplane echoes these fields back either way (GetRetainedMessageOutput only carries userProperties, which was already wired through)."
  - "UnsupportedDocumentEncodingException (real AWS error, modeled for GetThingShadow/DeleteThingShadow/UpdateThingShadow, HTTP 415) is never returned -- no validation exists that could trigger it. Left unimplemented: re-verified again this pass (gopherstack-76fj) after two other 'no documented trigger' claims elsewhere in this campaign turned out to be wrong. Checked six independent AWS sources this time: botocore's iot-data service-2.json model (doc string is exactly \"The document encoding is not supported.\", no further detail), aws-sdk-go-v2's types/errors.go doc comment (identical), the IoT API reference's Errors sections for GetThingShadow/UpdateThingShadow/DeleteThingShadow (same one-line description, HTTP 415, no header/parameter named), the Device Shadow REST API developer guide page (no Content-Encoding/Content-Type/charset mention at all for any of the three ops), the device communication protocols page (no compression/encoding support documented for the HTTPS publish/shadow surface), and the shadow troubleshooting page 'Diagnosing problems with shadows' (does not mention this exception among its documented failure modes). All six agree: AWS has never published what triggers this exception. Speculative validation (e.g. rejecting a guessed Content-Encoding header) risks a wrong-shape fix for behavior nobody can verify. Candidate for a future audit pass only if a live AWS account probe becomes available."
  - "RESOLVED this pass (parity-5, gopherstack-polh): ListSubscriptions previously always returned an empty subscriptions array. MQTTPublisher (interfaces.go) now carries ClientSubscriptions(clientID) (subs map[string]byte, connected bool), implemented in services/iot/broker.go off s.Clients.Get(clientID) + cl.State.Subscriptions.GetAll(). InMemoryBackend.ListSubscriptions calls through it and reports real topicFilter/qos pairs for a client the broker has a live session for. Proven against a REAL mochi-mqtt session (not a mock): TestBroker_ClientSubscriptionsAndSendToClient (services/iot/broker_test.go) connects a real paho MQTT client over real TCP, subscribes, and asserts the broker reports the exact filter/qos back. Residual honest gap: gopherstack's connections table (populated only via the admin-only RegisterConnection extension) is a distinct, weaker notion of 'connected' than a real broker session -- a clientId tracked there but with no live broker session still returns an honestly empty list (never fabricated), which is the expected/correct behavior for e.g. purely admin-registered test clients that never established a real MQTT connection."
  - "RESOLVED this pass (parity-5, gopherstack-polh): SendDirectMessage previously always broadcast on the target topic through the same path as Publish, never truly addressing one client. MQTTPublisher now also carries SendToClient(clientId, topic, payload, qos) (ok bool, err error), implemented in services/iot/broker.go via s.Clients.Get(clientID) + cl.WritePacket(packets.Packet{...}) -- a genuine per-client write that bypasses topic subscription matching entirely, matching real AWS's documented 'the receiving client does not need to subscribe to the topic' semantics. Proven against a real broker+paho client: the receiving client, NOT subscribed to the direct-send topic, still receives the message (TestBroker_ClientSubscriptionsAndSendToClient). Residual honest gap: when gopherstack's connections table has a tracked clientId but the broker has no live session for it (see above), SendDirectMessage falls back to the pre-existing topic-broadcast Publish path -- a deliberate, documented best-effort approximation, not a disguised no-op. confirmation/timeout (real AWS: wait for a QoS-1 PUBACK, HTTP 504 on timeout) still only select QoS 0-vs-1 on the outgoing message but never actually block or time out, since neither MQTTPublisher.Publish nor SendToClient wait for an ack."
  - "GetConnection omits cleanSession/disconnectReason/disconnectedSince/keepAliveDuration/sessionExpiry/sourcePort/targetIp/targetPort/thingName/vpcEndpointId from its response for every client, tracked or not -- gopherstack's connections table (populated only by the gopherstack-only RegisterConnection admin extension) never had this data to begin with (no real MQTT CONNECT packet is parsed anywhere in this service). Omitted (not zero-valued) so a real SDK client decodes these exactly as if the server had never observed them, which is wire-compatible even though it under-reports what a live AWS endpoint would return."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "Chaos fault-injection paths (ChaosServiceName/ChaosOperations) -- not part of AWS wire surface, no parity concern."
leaks: {status: clean, note: "no goroutines/timers introduced; tombstone rows are bounded by the same lifecycle as live shadow rows (same store.Table, same Reset/Snapshot/Restore path); removing the maxShadowsPerThing cap does not introduce unbounded growth risk beyond what already existed (shadows were never capped process-wide, only per-thing, and the per-thing cap had no eviction/GC of its own -- it only returned an error)"}
---

## Notes

Freeform: AWS-behavior specifics worth remembering (exact algorithms, wire quirks,
error-message text, protocol = query-XML / REST-XML / REST-JSON / json-1.0), and any
"looks-wrong-but-correct" traps so the next auditor doesn't re-flag them.

- **2026-09-04 pass: ListSubscriptions pagination was never wired.**
  `ListSubscriptionsInput.MaxResults`/`NextToken` are real, documented query
  params (`maxResults`/`nextToken`, confirmed via
  `awsRestjson1_serializeOpHttpBindingsListSubscriptionsInput` in
  `aws-sdk-go-v2/service/iotdataplane@v1.35.4`'s `serializers.go`) with a
  documented default: "The maximum number of subscriptions to return in a
  single request. By default, this is set to 20."
  (`ListSubscriptionsInput.MaxResults` doc comment, `api_op_ListSubscriptions.go`).
  `handleListSubscriptions` (handler_connections.go) never read either
  param -- every call returned the client's complete subscription list in one
  page and `nextToken` never appeared in the response, regardless of how many
  subscriptions existed or what the caller asked for. Every other list op in
  this service (`ListThingsWithShadows`, `ListNamedShadowsForThing`,
  `ListRetainedMessages`) already paginated via `parsePageSize`/
  `findCursorIndex`; `ListSubscriptions` was the one exception. Fixed by
  applying the same pattern, but honoring ListSubscriptions' own 20-item
  default (added `defaultSubscriptionsPageSize`) rather than the generic
  `defaultPageSize=25` those other (partly gopherstack-invented) admin/list
  endpoints use, since 20 is an AWS-documented value specific to this op.
  `parsePageSize` was generalized to take an explicit default instead of
  hard-coding `defaultPageSize`, and its three pre-existing call sites updated
  to pass `defaultPageSize` explicitly (behavior-preserving for them). See
  `TestHandler_ListSubscriptions_Pagination`.

- **2026-08-20 wrapper-key/nested-shape sweep, zero new bugs.** Re-verified all
  11 real ops against `aws-sdk-go-v2/service/iotdataplane@v1.35.4` (unchanged
  version -- no SDK bump since the prior pass): per-op HTTP method/path via
  `SplitURI` call sites in `serializers.go`, every Input/Output struct field
  (including optional members) via each op's own `api_op_<Op>.go`, and every
  op's own `awsRestjson1_deserializeOpError<Op>` case list. Confirmed the cnhp
  condition directly: `GetThingShadow`/`UpdateThingShadow`/`DeleteThingShadow`
  are the only ops whose `HandleDeserialize` calls
  `deserializeOpDocument<Op>Output(output, response.Body, response.ContentLength)`
  -- i.e. it reads the raw body straight into `Payload []byte`, never
  JSON-decodes a `shape`; `Publish`/`DeleteConnection` discard the body
  entirely (empty Output structs); the remaining 6 ops (`GetConnection`,
  `GetRetainedMessage`, `ListNamedShadowsForThing`, `ListRetainedMessages`,
  `ListSubscriptions`, `SendDirectMessage`) JSON-decode a `shape` and call
  `OpDocument<Op>Output(&output, shape)` for real -- this service has both
  shapes present simultaneously, one per op, exactly as the payload-passthrough
  risk note predicted. Found zero wire-shape bugs: every field name, JSON
  type, nesting level, query/header binding, and enum value (`PayloadFormatIndicator`'s
  `UNSPECIFIED_BYTES`/`UTF8_DATA`, checked both ways) already matched the SDK.
  `SendDirectMessageOutput`'s `message`/`traceId` fields (easy to miss --
  most other ops here return either an empty body or no members at all) were
  already wired correctly. `ListNamedShadowsForThing`'s unusual URI
  (`/api/things/shadow/ListNamedShadowsForThing/{thingName}`, not the
  `{thingName}/shadow?name=` pattern the other shadow ops use) was already
  matched exactly (`listNamedShadowsPrefix` in `handler.go`).
  Added `wire_sdk_roundtrip_test.go` (this service had no
  `sdk_roundtrip_helper_test.go`-style file before this pass): drives the
  real `aws-sdk-go-v2/service/iotdataplane` client through `pkgs/service`'s
  router for the shadow lifecycle trio, `ListNamedShadowsForThing`, `Publish`
  + retained-message read-back, and the full connections family
  (`GetConnection`/`ListSubscriptions`/`SendDirectMessage`/`DeleteConnection`),
  asserting real `smithy.APIError` codes on the not-found paths rather than
  string-matching. Fixed two stale/misleading comments found in passing (not
  wire bugs, just wrong prose): `handleListRetainedMessages`'s doc comment
  claimed "AWS RetainedMessageSummary does NOT include qos" directly
  contradicting the function's own (correct) behavior and a second, correct
  comment 26 lines below it; `ErrRequestTooLarge`'s doc comment claimed
  `RequestEntityTooLargeException` is "modeled only for UpdateThingShadow",
  but `SendDirectMessage`'s own `deserializeOpErrorSendDirectMessage` case
  list also models it (already correctly handled in code and in this file's
  own `SendDirectMessage` ops-note above -- only the sentinel's doc comment
  was out of date).
  **Provenance check**: `git show -s --format=%ad 058bf0373` = 2026-07-25
  15:47, matching `last_audit_date: 2026-07-25` exactly (zero-day gap) --
  clean by the sha-date-vs-audit-date check. Separately (informational, not
  part of that check): `d39bf33e4` (2026-08-11, "Chore/parity upgrade
  #2414") substantively touched this file's `ops`/`gaps` content and bumped
  `sdk_module` to v1.35.4 without bumping `last_audit_commit`/`last_audit_date`
  -- the manifest's prose was current as of Aug 11 even though its stamp
  still read Jul 25. No drift-causing commits landed between Aug 11 and this
  pass (`7abc9be9a`/`69bbb940a` only reordered an import and added
  `handler_sdk_route_table_test.go` within `services/iotdataplane/`), so this
  pass re-verified from the SDK directly rather than trusting the stale stamp.

- **Protocol**: restjson1. Verified directly against the compiled
  `aws-sdk-go-v2/service/iotdataplane@v1.32.20` serializers/deserializers (most
  authoritative source available -- prefer this over doc prose when they conflict).

- **Real error codes per op** (confirmed via `deserializers.go`'s per-op
  `awsRestjson1_deserializeOpError*` case lists): `ConflictException` and
  `RequestEntityTooLargeException` are modeled **only** for `UpdateThingShadow` --
  no other op (including `Publish`) has them in its error set. Don't add
  `RequestEntityTooLargeException` handling to `Publish`'s oversized-body path
  without new evidence; its current generic 413 (no specific AWS error code) is
  the closest defensible behavior given it isn't a modeled exception there.

- **There is no `VersionConflictException`** in the real API -- it was a
  gopherstack-invented name. The real wire error code for a shadow version
  mismatch is `ConflictException` (`ErrVersionConflict`'s Go identifier is kept
  unchanged for API stability; only its wire string changed).

- **DeleteThingShadow response shape**: AWS docs (device-shadow-rest-api.html)
  say the body is an "Empty response state document" -- confirmed this means
  only `version` + `timestamp`, NOT `state`/`metadata`/`clientToken`. Do not
  "fix" this back to including state; a previous implementation had a *dead*
  fallback branch with the correct minimal shape that was never reached because
  the primary path always succeeded and returned the (wrong, too-rich) full
  shadow response.

- **Version does not reset on delete**: verbatim from AWS docs: "Note that
  deleting a shadow does not reset its version number to 0." Implemented via a
  tombstone (`shadowEntry.deleted`) that keeps the row (with state cleared) in
  the `shadows` table instead of physically removing it, so
  `nextShadowVersion` naturally continues from the pre-delete version when the
  shadow is recreated. Tombstones are excluded from `GetThingShadow`,
  `ListNamedShadowsForThing`, and `ListThingsWithShadows`, and don't count
  against `maxShadowsPerThing` (see `liveShadowCount`). Persisted via
  `shadowEntrySnap.Deleted` (additive `omitempty` field -- old snapshots decode
  fine with `deleted=false`, no `iotdataplaneSnapshotVersion` bump needed).

- **DeleteConnection is a real, published AWS op** (`DELETE
  /connections/{clientId}`, confirmed via `api_op_DeleteConnection.go` +
  serializer) -- unlike `RegisterConnection`/`ListConnections`, which do NOT
  exist in the real SDK at all (grep the SDK module's op file listing to
  reconfirm if in doubt). A prior "AWS-accuracy audit batch" commit
  (3f01eaf0) moved all three off `/connections` to `/_admin/connections` in
  one sweep to properly hide the two fake ops, but collaterally broke real
  wire compatibility for the one genuine op. Fixed by restoring `DELETE
  /connections/{clientId}` as an additional real route (kept the `/_admin/`
  alias too, since existing tests and tooling depend on it). If you see
  `/connections` show up again in a "cleanup", check whether DeleteConnection
  is being swept along with the fake ops before touching it.

- **`/connections/{id}` collides with Outposts' real GetConnection wire path**
  (gopherstack-vpoh): both services expose a real, published op at this exact
  path. `RouteMatcher` used to claim it by path+method alone, silently
  swallowing correctly-signed Outposts `GetConnection` requests since this
  handler's `MatchPriority` (88) outranks Outposts' (85). Fixed by gating the
  real-wire-path branch on the SigV4 signing scope via the new
  `pkgs/httputils.ScopedPrefixMatch` (unsigned requests still match; a
  request signed for a different, known service does not). See
  `test/integration/tag_routing_test.go`'s
  `TestIntegration_ConnectionsRouting_CrossServiceIsolation` for the
  cross-service regression coverage.

- **Named/classic shadow key**: `shadowKey(thingName, shadowName)` = `"<thingName>#<shadowName>"`,
  classic shadow uses `shadowName == ""`. `#` cannot appear in either
  component given their validation regexes, so no collision risk.

- **Path-style named shadow route is NOT real AWS wire**: `handler.go` also
  accepts `/things/{thingName}/shadow/name/{shadowName}` in addition to the
  real `/things/{thingName}/shadow?name=...` query-param form. Confirmed via
  `httpbinding.SplitURI` call sites in `serializers.go` that the real SDK only
  ever generates the `?name=` form for Get/Update/DeleteThingShadow -- the
  path-style route has no equivalent in `aws-sdk-go-v2/service/iotdataplane`.
  This is pure test-convenience leniency (a superset of accepted request
  shapes, same op names, same responses): it never causes a real SDK client's
  traffic to be misrouted or misinterpreted, since a real client only ever
  sends the `?name=` form. Left in place (heavily used by existing tests), but
  noted here so a future audit doesn't mistake it for a modeled AWS op.

- **Shadow doc size limit**: `maxShadowDocumentBytes` = 8KB, matches
  `maxShadowBodyBytes` at the HTTP layer (`handler.go`'s `MaxBytesReader`), so
  in practice the HTTP-layer cutoff fires first and the backend's own check is
  a defensive backstop reachable only when the backend is invoked directly
  (bypassing the HTTP body limit) -- see
  `TestRefinement2_ShadowDocumentValidation_BackendSizeCheck`. Both paths now
  return `RequestEntityTooLargeException`/413 consistently.

- **Publish max size**: 128KB (`maxPublishBodyBytes`), matches real AWS IoT
  Core's documented MQTT/HTTP publish payload limit. No error-code fix applied
  here (see gaps) since `RequestEntityTooLargeException` isn't modeled for
  `Publish`.

- **`errMethodNotAllowed` constant** changed from the placeholder string
  `"method not allowed"` to the real AWS wire error code
  `"MethodNotAllowedException"` (used across every 405 response in this
  handler). No test depended on the old literal text (only status codes were
  asserted), so this was a safe, systemic wire-accuracy fix.

- **`maxShadowsPerThing=100` cap REMOVED**: the prior pass flagged this as
  low-confidence and left it in place. This pass re-verified against the
  authoritative AWS General Reference "AWS IoT Core endpoints and quotas"
  page: it documents shadow document size (8KB), shadow name length (64
  bytes), in-flight-unacknowledged-messages-per-thing (10), and
  requests-per-second-per-shadow (20) -- but **no limit at all on the number
  of named shadows per thing**. Community reports (AWS re:Post) describe
  200-10,000+ named shadows on a single thing without hitting any API-level
  cap. gopherstack's self-imposed 100-shadow cap was therefore a
  gopherstack-invented behavior that would reject `UpdateThingShadow` calls a
  real AWS account would accept -- the wrong direction for parity (stricter
  than AWS, not more lenient). Removed the check, the now-dead
  `liveShadowCount` helper, and the `MaxShadowsPerThing` test export; replaced
  the cap-boundary tests in shadows_test.go with
  `Test_ManyNamedShadowsPerThing_*` tests proving no artificial limit exists.

- **Shadow state JSON nesting depth cap ADDED**: the same AWS quotas page
  documents "Maximum depth of JSON device state documents: 8 levels (in both
  desired and reported sections)" -- this was previously unenforced entirely.
  Added `maxShadowStateDepth = 8` and `validateShadowStateDepth` (shadows.go),
  applied to `state.desired`/`state.reported` in `applyShadowStateSection`,
  returning `InvalidRequestException` when exceeded. The section's top-level
  object itself counts as depth 1 (i.e. `{"a":1}` is depth 1, `{"a":{"b":1}}`
  is depth 2). See `Test_ShadowStateDepth_*` in shadows_validation_test.go.

- **`ListRetainedMessages` summary was missing `qos`**: a prior audit pass
  asserted (incorrectly) that AWS's `RetainedMessageSummary` excludes `qos`
  and added tests locking that in. Directly reading
  `awsRestjson1_deserializeDocumentRetainedMessageSummary` in the real SDK's
  `deserializers.go` shows `qos` IS a recognized field on that shape. Fixed
  `handleListRetainedMessages` to include it and rewrote the tests that had
  asserted its absence (`Test_ListRetainedMessages_SummaryIncludesQos`).

- **`GetRetainedMessage` was missing `userProperties`**: `GetRetainedMessageOutput`
  in the real SDK carries a `UserProperties []byte` field (base64-encoded MQTT5
  user properties JSON array, or absent/null when unset) that gopherstack's
  response never included. Added `RetainedMessage.UserProperties` (types.go),
  threaded it through `StoreRetainedMessage`'s new `userProperties []byte`
  parameter, and included it in the `GetRetainedMessage` JSON response.

- **`Publish` was missing its entire MQTT5-property wire surface**: real
  `PublishInput` (per `awsRestjson1_serializeOpHttpBindingsPublishInput` in
  `serializers.go`) carries `contentType`/`messageExpiry`/`responseTopic` as
  query params and `correlationData`/`payloadFormatIndicator`/`userProperties`
  as `X-Amz-Mqtt5-*` headers (the last base64-encoded). None of these were
  even read from the request before this pass -- a real SDK client setting
  any of them got silently ignored with no validation. Added
  `parseMQTT5PublishParams` (handler_publish.go) plus validators in publish.go
  (`validatePayloadFormatIndicator`, `validateResponseTopic`,
  `parseMessageExpiry`, `decodeUserProperties`) so malformed values now
  produce the correct `InvalidRequestException`. At the time of this note,
  only `userProperties` had an AWS-visible effect reachable within this
  service (persisted onto the retained message, see above) -- the rest were
  accepted/validated but not forwarded anywhere further. RESOLVED in a later
  pass (gopherstack-76fj): all six fields now also reach the broker as real
  MQTT5 packet properties -- see the resolved `gaps` entry above and
  `toMQTT5Properties`/`PublishWithProperties` in the current source. That pass
  also found and fixed two bugs in the validation described here:
  `correlationData` was never actually base64-decoded (stored as opaque
  header text despite AWS documenting it as base64-encoded binary), and
  `userProperties`' JSON-array-of-single-key-objects shape was never
  validated (only its base64 envelope was) -- see `decodeCorrelationData`/
  `parseUserProperties` in publish.go.

- **`DeleteConnection` never returned `ResourceNotFoundException`**: real AWS
  models this error for `DeleteConnection` (confirmed via
  `awsRestjson1_deserializeOpErrorDeleteConnection`'s case list in
  `deserializers.go`), but gopherstack's implementation unconditionally
  succeeded even for a `clientId` with no tracked connection ("idempotent
  delete"). Since gopherstack's only concept of "connected" is the
  `connections` table (populated via the gopherstack-only `RegisterConnection`
  admin extension -- see admin-only-extensions family), a real AWS SDK client
  that never calls the admin endpoint will always get 404 from
  `DeleteConnection`, which is the intended test-convenience contract: use
  `/_admin/connections/{clientId}` (POST) to simulate a connected client
  first. Added `ErrConnectionNotFound` (errors.go) wired to
  `ResourceNotFoundException` in `handleError`. `cleanSession`/
  `preventWillMessage` (the two other real `DeleteConnectionInput` query
  params -- confirmed via `serializers.go:77-91`,
  `SetQuery("cleanSession")`/`SetQuery("preventWillMessage")`) are still not
  parsed. Re-investigated this pass (gopherstack-76fj): checked whether
  `services/iot`'s mochi-mqtt broker already tracks enough live per-client
  session state to honor them if `DeleteConnection` were wired through to it.
  It doesn't, for two independent reasons, both confirmed by reading
  mochi-mqtt's own source (`github.com/mochi-mqtt/server/v2@v2.7.9`):
  (1) gopherstack's `connections` table (the thing `DeleteConnection` actually
  mutates) is populated only by the admin-only `RegisterConnection` extension
  and has never been correlated with the broker's live `Clients` sessions at
  all -- unlike `GetConnection`/`ListSubscriptions`/`SendDirectMessage`, which
  already bridge that gap via `MQTTPublisher`. (2) Even if it were bridged,
  mochi-mqtt's own public API to force-disconnect a client
  (`Server.DisconnectClient`, `server.go:1414`) provides no lever for either
  parameter: it always calls `sendLWT(cl)` on the way out when the read loop
  returns a non-nil error (`server.go:476-481`, and identically at
  `server.go:735-736` for a second-CONNECT kick) -- there is no
  parameter to suppress the Will message on a *server-initiated* disconnect
  (Will is only cleared client-side, on a graceful client-sent DISCONNECT
  packet, `cl.Properties.Will = Will{}` at `server.go:481`, which
  `DeleteConnection` cannot trigger). Likewise `cleanSession`'s target,
  session-expiry-vs-clean behavior, is fixed at CONNECT time
  (`cl.Properties.Clean`/`SessionExpiryInterval`) with no override accepted by
  `DisconnectClient`. Honoring either parameter faithfully would require
  either patching mochi-mqtt itself (a third-party dependency, out of scope)
  or reaching into its unexported `Client.Properties.Will` via reflection,
  which this repo's parity principles treat as a disguised stub, not a real
  fix. Left unimplemented, now tracked here explicitly rather than folded into
  the Publish/broker gap paragraph (that gap was resolved this pass; this one
  is structurally blocked, not a scope choice).

- **New family this pass: device connection/messaging introspection**
  (`GetConnection`, `ListSubscriptions`, `SendDirectMessage` -- SDK bumped to
  `v1.35.0`, `+3` ops). All three are real, published AWS iotdataplane
  operations rooted at `/connections/{clientId}` (confirmed via
  `api_op_{GetConnection,ListSubscriptions,SendDirectMessage}.go` and their
  serializers). Routing required generalizing the old
  `isDeleteConnectionPath`/`extractConnectionClientID` DELETE-only helpers
  (`handler.go`) into `splitConnectionsWirePath`/`connectionsWireOperation`,
  which classify any `/connections/{clientId}[/subscriptions|/messages]`
  method+path combination into the right op (or `""` for the one
  non-AWS combination that must keep 404ing: bare `POST
  /connections/{clientId}`, which is `RegisterConnection`'s territory and
  only exists at `/_admin/connections/{clientId}`).

- **Real-state survey, updated (grade restored to A this pass,
  parity-5/gopherstack-polh):** this service already had one piece of
  genuine, if gopherstack-only, connection state -- the `connections` table,
  populated exclusively via the `RegisterConnection` admin extension (see
  `admin-only-extensions`) and already used by `DeleteConnection` for its 404
  semantics. `GetConnection` reuses this table honestly: a registered
  client's `connected`/`clientId`/`connectedSince` are real, tracked values,
  not fabricated, and an unregistered client correctly 404s. This repo's
  *other* candidate source of real state -- `services/iot`'s `mochi-mqtt`-
  backed broker, which genuinely tracks live subscriptions per client
  (`cl.State.Subscriptions` in `github.com/mochi-mqtt/server/v2`) -- was
  previously unreachable from `iotdataplane` because `MQTTPublisher`
  (interfaces.go) only exposed topic-broadcast `Publish`, and its only
  implementation (`services/iot/broker.go`) was out of scope for the pass
  that shipped `ListSubscriptions`/`SendDirectMessage`. That interface
  boundary is now closed: `MQTTPublisher` also carries
  `ClientSubscriptions(clientID) (map[string]byte, bool)` and
  `SendToClient(clientID, topic, payload, qos) (bool, error)`, both
  implemented in `services/iot/broker.go` off the broker's real
  `s.Clients`/`cl.State.Subscriptions`/`cl.WritePacket`, and proven against a
  REAL mochi-mqtt session (a live paho MQTT client over real TCP, not a mock
  -- see `TestBroker_ClientSubscriptionsAndSendToClient` in
  `services/iot/broker_test.go`). `ListSubscriptions` now reports real
  topicFilter/qos pairs and `SendDirectMessage` now genuinely addresses one
  client's connection, bypassing subscription matching, matching real AWS.
  The one remaining honest divergence -- gopherstack's admin-only
  `connections` table can track a clientId the broker has no live session
  for, in which case both ops degrade to their prior honest behavior (empty
  list / topic broadcast) rather than fabricating a per-client result -- is
  fully documented in `gaps`.

- **`GetConnection`/`ListSubscriptions`/`SendDirectMessage` share
  `ErrConnectionNotFound`** (`errors.go`) with `DeleteConnection` rather than
  inventing per-op not-found errors: all four ops are modeled with
  `ResourceNotFoundException` in their real error case lists
  (`deserializers.go`), and gopherstack's one and only concept of "is this
  clientId connected" is the same `connections` table for all of them --
  reusing the sentinel keeps that consistent instead of accidentally
  diverging.

- **`SendDirectMessage`'s `RequestEntityTooLargeException` is real, unlike
  `Publish`'s**: confirmed via `awsRestjson1_deserializeOpErrorSendDirectMessage`'s
  case list (`GatewayTimeoutException`, `RequestEntityTooLargeException`,
  `UnauthorizedException` are modeled here but *not* on `Publish`'s case
  list -- see the existing "Real error codes per op" note above). Reused
  the existing `ErrRequestTooLarge` sentinel (413) for an oversized message
  body, capped at the same `maxPublishBodyBytes` (128KB) as `Publish`, since
  AWS IoT Core doesn't document a different MQTT/HTTP payload limit specific
  to `SendDirectMessage`. `GatewayTimeoutException`/`UnauthorizedException`
  are left unimplemented: the former needs real PUBACK-wait semantics this
  emulator's fire-and-forget broker interface can't provide (see gaps), and
  the latter has no IAM/permission model in this service to ever trigger it
  -- both are genuine impossibilities given current scope, not oversights.
