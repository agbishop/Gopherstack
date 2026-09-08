---
service: sns
sdk_module: aws-sdk-go-v2/service/sns@v1.46.0
last_audit_commit: 3afc23468
last_audit_date: 2026-08-10
overall: A
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateTopic: {wire: ok, errors: ok, state: ok, persist: ok, note: "idempotent on existing name; FIFO/CBD/Kms validation correct"}
  DeleteTopic: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: now also drops topicMessageArchive (was leaking + could resurrect stale archive on ARN reuse)"}
  ListTopics: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTopicAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "computed attrs (Owner/TopicArn/EffectiveDeliveryPolicy/SubscriptionsConfirmed|Pending|Deleted) correct"}
  SetTopicAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  Subscribe: {wire: ok, errors: ok, state: ok, persist: ok, note: "all 9 protocols; pending-confirmation literal 'pending confirmation'; firehose requires SubscriptionRoleArn; dedup on existing confirmed sub; fixed this pass: FilterPolicy 5-key cap, FilterPolicyLimitExceeded (200/topic, 10,000/account), SubscriptionLimitExceeded (12,500,000/topic, test-overridable) were all previously unenforced; fixed this pass: per-protocol Endpoint validation (sqs/lambda/firehose require a matching-service ARN with the expected resource prefix, application requires an sns endpoint/ ARN, http/https require a URL whose scheme matches the protocol) — previously any string was accepted for every non-SMS/email protocol"}
  ConfirmSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  Unsubscribe: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSubscriptions: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSubscriptionsByTopic: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSubscriptionAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  SetSubscriptionAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "FilterPolicy/FilterPolicyScope/RedrivePolicy(+DLQ existence check)/DeliveryPolicy/ReplayPolicy/RawMessageDelivery/SubscriptionRoleArn; fixed this pass: ReplayPolicy is now rejected (InvalidParameter) unless the subscription's topic is FIFO and its protocol is sqs/lambda/firehose (the real AWS application-to-application scope), was previously accepted unconditionally on any topic/protocol"}
  Publish: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: Lambda/Firehose/SQS-emitter now share one signed envelope (buildPublishedEvent) instead of Lambda fabricating a random-UUID signature"}
  PublishBatch: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed this pass: per-entry MessageAttributes field prefix was missing '.MessageAttributes' segment (verified against serializers.go) — every batch entry's attributes were silently dropped, breaking FilterPolicy matching for PublishBatch"}
  PublishToTargetArn (TargetArn publish): {wire: ok, errors: ok, state: ok, persist: n/a, note: "EndpointDisabled enforced"}
  PublishSMS (PhoneNumber publish): {wire: ok, errors: ok, state: ok, persist: n/a, note: "opt-out + sandbox-unverified enforced"}
  CreatePlatformApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (gopherstack-r3pr): duplicate-name/platform now returns InvalidParameter, not the invented PlatformApplicationAlreadyExists — CreatePlatformApplication's own deserializeOpError (sns@v1.42.4 deserializers.go:437-477) models only AuthorizationError/InternalError/InvalidParameter, no already-exists shape exists in the pinned module"}
  GetPlatformApplicationAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  SetPlatformApplicationAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPlatformApplications: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePlatformApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePlatformEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  GetEndpointAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  SetEndpointAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  ListEndpointsByPlatformApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  AddPermission: {wire: ok, errors: ok, state: ok, persist: ok, note: "stored on Topic.Permissions, travels with topic snapshot; fixed this pass: AuthorizationError now returns HTTP 403 (was 400 — handleBackendError had no 403 bucket at all)"}
  RemovePermission: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: AuthorizationError (label not found) now returns HTTP 403, see AddPermission"}
  GetSMSSandboxAccountStatus/CreateSMSSandboxPhoneNumber/DeleteSMSSandboxPhoneNumber/ListSMSSandboxPhoneNumbers/VerifySMSSandboxPhoneNumber: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (gopherstack-r3pr): CreateSMSSandboxPhoneNumber duplicate-phone now returns UserError, not the invented AlreadyExists — its own deserializeOpError (deserializers.go:676-726) models AuthorizationError/InternalError/InvalidParameter/OptedOut/Throttled/UserError, no already-exists shape. UserError ('a request parameter does not comply with the associated constraints') is the nearest modelled fit; UNCONFIRMED against AWS prose docs"}
  CheckIfPhoneNumberIsOptedOut/ListPhoneNumbersOptedOut/OptInPhoneNumber: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass: ErrOptedOut sentinel text was the unrelated copy-pasted string 'KMSOptInRequired'"}
  GetSMSAttributes/SetSMSAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDataProtectionPolicy/PutDataProtectionPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass (bd gopherstack-4wtz): PutDataProtectionPolicy now enforces the documented 30,720-char max length (aws-sdk-go-v2/service/sns@v1.42.4 api_op_PutDataProtectionPolicy.go DataProtectionPolicy field doc) and the required top-level JSON keys Name/Version/Statement (docs.aws.amazon.com/sns/latest/dg/sns-message-data-protection-policies.html), both previously unenforced (any valid-JSON string was accepted); also fixed: DataProtectionPolicy no longer settable via SetTopicAttributes nor returned by GetTopicAttributes (confirmed absent from both operations' documented Attributes list — real AWS exposes it only through the dedicated Get/PutDataProtectionPolicy ops), previously it silently shared the generic topic-attributes bag; the deep data-identifier/statement grammar remains unimplemented, see deferred"}
  ListOriginationNumbers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "AWS has no public create API; empty by default, SeedOriginationNumber for tests. FIXED 2026-08-14 (gopherstack-3tpf structural diff): XMLOriginationPhone (the domain model itself, not just a DTO) was entirely missing CreatedAt and Status, two real members of types.PhoneNumberInformation (types/types.go:82-103) confirmed present in the actual awsAwsquery_deserializeDocumentPhoneNumberInformation wire decoder (deserializers.go:7950) -- a real client always decoded a nil CreatedAt and empty Status regardless of what SeedOriginationNumber supplied. Added both fields (CreatedAt *time.Time xml:CreatedAt,omitempty; Status string xml:Status,omitempty, matching the cloudformation *time.Time-for-omitempty convention). Verified via TestListOriginationNumbers_CreatedAtAndStatusWireRoundTrip driving the real aws-sdk-go-v2 SNS client; hand-reverted the struct fields (test unchanged) and confirmed the revert does not just fail the assertion but fails to COMPILE (\"unknown field Status/CreatedAt in struct literal\"), the strongest possible confirmation the fields were structurally absent, not merely unwired."}
  TagResource/UntagResource/ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "pkgs/tags-backed. VERIFIED CLEAN (wrapper-key sweep, 2026-08-29): checked for the stepfunctions-class bug (a Tags field typed as a Go map when the SDK sends an array, or vice versa). sns@v1.42.4 serializers.go:3862-3867/3893-3898 confirm TagResource.Tags serializes as Tags.member.N.Key/Value (awsAwsquery_serializeDocumentTagList, array element name 'member') and UntagResource.TagKeys as TagKeys.member.N (awsAwsquery_serializeDocumentTagKeyList) — handler_tags.go's parseSNSTagsFromForm/parseSNSTagKeysFromForm already parse exactly these wrapper names. Confirmed via TestTagResourceFamily_SDKRoundTrip (tag_resource_sdk_test.go) driving the real SDK client."}
families:
  filter_policy_matching: {status: ok, note: "prefix/suffix/equals-ignore-case/anything-but(+nested)/exists/numeric(6 ops)/wildcard/cidr/$or, MessageBody vs MessageAttributes scope, String.Array expansion, 150-condition cap, 256KiB size cap, 5-key-per-policy cap (fixed this pass, was unenforced), FilterPolicyLimitExceeded 200/topic+10,000/account quota (fixed this pass, was unenforced and the error sentinel/code did not exist at all) — field-diffed against docs.aws.amazon.com/sns/latest/dg/subscription-filter-policy-constraints.html and API_Subscribe.html Errors table"}
  fifo_topics: {status: ok, note: "MessageGroupId required, ContentBasedDeduplication (SHA-256 body digest) vs explicit MessageDeduplicationId mutually exclusive, 5-min dedup window with bounded+swept map, 20-digit zero-padded monotonic SequenceNumber per topic, PublishBatch per-entry dedup"}
  delivery_lambda_firehose_sms_application: {status: ok, note: "fixed this pass: (1) Lambda envelope now carries the real per-publish Timestamp/Signature/SigningCertURL/UnsubscribeURL instead of a fabricated random-UUID signature and empty cert/unsub URLs; (2) Firehose now respects RawMessageDelivery (envelopes as JSON when false, matching AWS default, previously always sent the bare message); DLQ redrive on failure now forwards the same body that was attempted"}
  replay_policy_archive: {status: ok, note: "fans out through the same per-protocol delivery functions Publish uses (SQS via the emitter, Lambda/Firehose via their delivery functions). fixed this pass (bd: gopherstack-bz6), re-verified against docs.aws.amazon.com/sns/latest/dg/fifo-message-archiving-replay.html and message-archiving-and-replay-topic-owner.html ('Amazon SNS message archiving and replay is only available for application-to-application (A2A) FIFO topics'): ArchivePolicy is now rejected (InvalidParameter) on non-FIFO topics at both CreateTopic and SetTopicAttributes; ReplayPolicy is now rejected (InvalidParameter) unless the subscription's topic is FIFO and its protocol is sqs/lambda/firehose. Previously ArchivePolicy/ReplayPolicy were accepted on any topic and fanned out to any protocol (HTTP/email/sms/application), which is not real AWS behavior — standard topics have no archive/replay mechanism at all, and SMS/Application/HTTP/HTTPS are A2P protocols never eligible even on a FIFO topic"}
  http_https_delivery: {status: ok, note: "RSA-2048 self-signed cert; SignatureVersion-aware signing (SHA1withRSA for the AWS default SignatureVersion=1, SHA256withRSA when a topic explicitly sets SignatureVersion=2), retry via DeliveryPolicy/EffectiveDeliveryPolicy, DLQ redrive, concurrency-capped worker semaphore, ctx-cancel on shutdown; fixed this pass: delivery previously always signed with SHA-256 regardless of the topic's SignatureVersion attribute (and always declared SignatureVersion=2 in every envelope: HTTP/HTTPS, Lambda, Firehose, and the SQS delivery envelope built by services/sqs) — now resolveSignatureVersion/signWithVersion select SHA1 vs SHA256 per-topic and every envelope declares the version that actually produced its Signature"}
  error_codes: {status: ok, note: "NotFound/InvalidParameter/EndpointDisabled/OptedOut/AuthorizationError(permission label)/SubscriptionLimitExceeded/FilterPolicyLimitExceeded all map to correct AWS code strings; fixed this pass: handleBackendError previously only split 400-vs-500 (per the prior audit's own 'verified' note) with NO 403 bucket at all, so AuthorizationError/SubscriptionLimitExceeded/FilterPolicyLimitExceeded (all documented HTTP 403 in the SNS API errors tables) were silently returning 400; EndpointDisabled correctly stays 400 (confirmed against API_Publish.html, not 403 despite being permission-adjacent). CORRECTED this pass (gopherstack-r3pr, errcodeaudit no-near-miss sweep): the previous claim that TopicAlreadyExists/PlatformApplicationAlreadyExists mapped to correct AWS code strings was wrong. ErrTopicAlreadyExists is a DEAD sentinel — declared and matched in two switch statements but never raised at any call site (CreateTopic is real-AWS idempotent on name collision and raises nothing); left as-is, no wire path exercises it. ErrPlatformApplicationAlreadyExists and ErrSandboxPhoneAlreadyExists WERE live and emitted the invented codes PlatformApplicationAlreadyExists/AlreadyExists — fixed to InvalidParameter/UserError respectively (see CreatePlatformApplication and the SMS-sandbox row above)."}
gaps:
  - "gopherstack-wksw (2026-08-29, constraint-not-honoured sweep): ListPhoneNumbersOptedOut's backend method (InMemoryBackend.ListPhoneNumbersOptedOut) accepts a maxResults int parameter, but the real ListPhoneNumbersOptedOutInput (api_op_ListPhoneNumbersOptedOut.go) has no MaxResults member at all -- only NextToken (itself serialized under the unusual lowercase 'nextToken' key for this one op, confirmed against awsAwsjson1_serializeOpDocumentListPhoneNumbersOptedOutInput -- verified NOT a bug, gopherstack's handler_sms.go already reads the matching lowercase form key). The extra backend parameter is inert (the handler always passes a form value that a real client never sends), not a wire defect -- noted here only because it looked suspicious at first read."
  - "2026-08-14 (gopherstack-3tpf): ConfirmSubscriptionInput.AuthenticateOnUnsubscribe (aws-sdk-go-v2/service/sns@v1.42.4 api_op_ConfirmSubscription.go:14 doc comment: 'This call requires an AWS signature only when the AuthenticateOnUnsubscribe flag is set to \"true\"') is accepted by the real SDK request shape but has no field on gopherstack's ConfirmSubscriptionInput and is silently dropped. Structurally undeliverable without the caller-identity/SigV4-principal infrastructure gopherstack does not have (see gopherstack-cu4g, open): Unsubscribe (subscriptions.go:217) takes no caller identity at all today, so there is nothing to condition an 'unauthenticated unsubscribe' rejection on. Same class as sts's disclosed JWTPayloadSizeExceededException gap and secretsmanager's disclosed PutSecretValueInput.RotationToken gap. DISCLOSED, not fixed."
deferred:
  - "PutDataProtectionPolicy: the policy statement grammar (DataIdentifier ARNs, Operation/Audit/De-identify/Deny shapes, Principal formats) is not validated — only the top-level document shape (JSON object, <=30,720 chars, Name/Version/Statement present). Amazon SNS message data protection is also no longer available to new customers as of 2026-04-30 per docs.aws.amazon.com/sns/latest/dg/sns-message-data-protection-availability-change.html (existing customers may continue using it); implementing the full grammar is disproportionate feature work for a frozen/legacy feature and was explicitly out of scope this pass (bd gopherstack-4wtz)."
  - "Cross-service integration (test/integration/*_parity_test.go) was not run this pass — see parity-principles.md note that unit tests are not parity proof; recommend running the SDK-driven integration suite in a follow-up"
leaks: {status: clean, note: "fixed this pass: (1) topicMessageArchive was never persisted (Snapshot/Restore) and was never cleaned up on DeleteTopic (both leak + ARN-reuse resurrection bug); (2) smsDeliveries/emailDeliveries/applicationDeliveries observability buffers had no cap and grew unboundedly under sustained publish traffic without a Drain* call — added appendBounded with maxRecordedDeliveries=100k; (3) notificationSigner.certURL was read/written without synchronization (SetSigningCertBaseURL vs concurrent delivery reads) — added a dedicated RWMutex. HTTP delivery goroutines already had proper ctx-cancel + semaphore + deliveryWg cleanup (unchanged, verified correct). 2026-09-03 (gopherstack-0k0): found and fixed a genuine unsynchronized data race, same class as (3) above but in three call sites (3) missed — see Notes."}
---

## Notes

## 2026-09-03 audit (gopherstack-0k0)

Full five-dimension audit (AWS behavior compliance, LocalStack parity,
cross-service integration, performance, resource leaks) requested against
`3afc23468`/current HEAD `69092a108`. Given how exhaustively this service has
already been swept (see the passes below, 2026-07-11 through 2026-08-31 —
wire shapes, error codes, filter-policy semantics, pagination tokens, exact
XML element casing, error-envelope error-code mapping), most of AWS behavior
compliance re-checked clean rather than finding new gaps. One genuine bug
found and fixed, in the resource-leak/concurrency-correctness dimension:

**Data race: `b.lambdaBackend`/`b.firehoseBackend`/`b.emitter` read without
the backend lock in the Publish delivery fan-out.** `SetLambdaBackend`/
`SetFirehoseBackend`/`SetPublishEmitter` (store.go) all mutate these fields
under `b.mu.Lock`, matching the pkgs-catalog coarse-lock convention — but
`deliverToLambdaSubscriptions`/`deliverToFirehoseSubscriptions`
(lambda_firehose_delivery.go) read `b.lambdaBackend`/`b.firehoseBackend`/
`b.sqsSender`, and `emitPublishedEvent` (publish.go) read `b.emitter`, with
no lock at all — an unsynchronized concurrent read/write on interface-typed
fields, confirmed by `go test -race` (`WARNING: DATA RACE`,
`lambda_firehose_delivery.go:128`/`179` vs `store.go:281`/`289`, and
`publish.go:289` vs `store.go:273`). The codebase was already aware of this
exact race class and had defended against it elsewhere — `Publish` captures
`b.httpClient` under its RLock specifically "to avoid data races with
concurrent SetHTTPDeliveryClient calls", and `replayMessagesToSubscription`
(archive.go) captures `b.emitter` under RLock too — but the three call sites
above were missed. Not client-observable in production (the `Set*` wiring
calls in `cli.go` all run once at startup, strictly before the HTTP server
accepts traffic, so there is a real happens-before edge there), but a real,
reproducible race under any concurrent test or dynamic-rewiring caller, and
a genuine Go memory-model violation on interface-typed fields (a torn
type/data-pointer read is a crash risk, not just a stale-value risk).
Fixed by capturing all four fields under `b.mu.RLock` before use, the same
pattern already used for `httpClient`/`emitter`(archive.go). Regression test:
`TestConcurrentWiringVsPublish` (concurrent_wiring_test.go), one subtest per
affected field (`lambda_backend`/`firehose_backend`/`publish_emitter`),
confirmed failing (`WARNING: DATA RACE`) against the unfixed code via a
temporary revert, passing after. `go test -race -count=1 ./services/sns/...`
passes (unchanged otherwise); `go vet`/`gofmt` clean;
`golangci-lint run ./services/sns/...` could not be run this pass — it
panics repo-wide (`buildir`/`nilness`/`fact_purity`/`typedness`/`SA5012` on
package `poll`, "unexpected expr: `*ast.KeyValueExpr`" / "interface
conversion: interface {} is nil, not `*buildir.IR`"), reproduced identically
scoped to just the three changed files and on an unrelated package, so this
is an environment/tool issue (honnef.co/go/tools vs. the installed Go
toolchain), not something introduced by this change — flagging for the next
auditor rather than silently skipping.

Cross-service integration (`test/integration/sns_*_test.go`) was not
re-run this pass — running `make build-linux`/Docker-based integration
tests was judged too heavy/risky while another agent was concurrently
editing `services/sqs` in the same worktree; SNS→SQS/Lambda ARN
resolution and event-shape code was instead reviewed statically
(`collectPublishTargets`, `buildPublishedEvent`, `pkgs/events.
SNSPublishedEvent`, `services/sqs/sns_delivery.go` read-only) and found
consistent. Recommend running the integration suite in a follow-up, per
the standing note below from 2026-08-10.


### 2026-08-29 constraint-not-honoured sweep (gopherstack-wksw)

New bug class for this campaign: a parameter that constrains a result (filter/sort/page
limit) present in the real Input but not correctly honoured -- distinct from the wire-key
bugs prior passes swept for. Read every collection-returning op's real `<Op>Input` in
`sns@v1.42.4` (`ListEndpointsByPlatformApplication`, `ListOriginationNumbers`,
`ListPhoneNumbersOptedOut`, `ListPlatformApplications`, `ListSMSSandboxPhoneNumbers`,
`ListSubscriptions`, `ListSubscriptionsByTopic`, `ListTagsForResource`, `ListTopics`) --
9 ops total. SNS's List surface turned out to be almost entirely pagination (`NextToken`,
sometimes `MaxResults`) plus a required target-scoping ARN on 2 ops
(`ListSubscriptionsByTopic.TopicArn`, `ListEndpointsByPlatformApplication.
PlatformApplicationArn`) -- no filter/sort parameters exist on any SNS List op beyond
that, which is a much smaller real surface than the campaign brief's rough estimate of
~22 (confirmed overestimate, consistent with 8 other services this campaign).

**0 bugs found.** Every op checked out: `NextToken` correctly read and threaded through
(`pagination.go`'s shared `paginate`/`decodeToken`/`encodeToken`); the 3 ops with a real
`MaxResults` member (`ListOriginationNumbers`, `ListSMSSandboxPhoneNumbers`, and --
inertly, see gaps -- `ListPhoneNumbersOptedOut`) correctly resolve via `resolvePageSize`
against per-op default/max constants matching each op's own doc comment;
`ListSubscriptionsByTopic`/`ListEndpointsByPlatformApplication` correctly 404
(`ErrTopicNotFound`/`ErrPlatformApplicationNotFound`) before filtering rather than
silently returning empty for a nonexistent target, and correctly scope results to only
that target's subscriptions/endpoints (`b.subscriptionsByTopic`/filtering by
`PlatformApplicationArn`, verified not leaking cross-target results). One SNS-specific
wire quirk re-confirmed while checking this class: `ListPhoneNumbersOptedOut`'s `NextToken`
is genuinely serialized under a lowercase `nextToken` key by the real SDK (confirmed in
`serializers.go`, not a case-insensitivity artifact) and `handler_sms.go` already matches
it exactly -- correct, not a bug, but easy to mistake for one on a quick read.

Test style: no new tests needed (0 bugs to regress-guard); existing pagination/filter
tests (`pagination_test.go`, `platform_endpoints_test.go`, `subscriptions_test.go`) already
assert on decoded response content for the cases that exist. Real SDK client not driven
fresh for this pass -- the existing wrapper-key sweep entry above (`TagResource/
UntagResource/ListTagsForResource`, `tag_resource_sdk_test.go`) and `ListOriginationNumbers`
entry already have real-client round-trip coverage on this same code path.

Freeform notes for the next auditor — AWS-behavior specifics worth remembering, and
"looks-wrong-but-correct" traps.

### 2026-08-10 re-audit (bd gopherstack-4wtz)
Worked the four items the bd issue listed. Three were already resolved by the
2026-07-25 pass below — the bd issue (filed 2026-07-23, two days earlier) was
simply stale, not a live gap:
1. **ArchivePolicy/ReplayPolicy FIFO-only enforcement** — already fully
   implemented (`validateCreateTopicAttrs`/`validateTopicAttributeValue` in
   topics.go/topic_attributes.go, `validateReplayPolicyEligibleLocked` in
   subscriptions.go) and covered by `archive_test.go`'s
   `TestArchivePolicyRejectedOnStandardTopic` /
   `TestReplayPolicyRejectedForIneligibleProtocolOrTopic`. No HTTP-replay
   tests on standard topics remain — the "needs test rework" excuse recorded
   in the bd issue no longer describes the code; it was already reworked.
2. **Subscribe sqs endpoint ARN validation** — already fully implemented
   (`validateSubscribeEndpoint` in subscriptions.go rejects any non-ARN or
   wrong-service Endpoint for the sqs/lambda/firehose/application
   protocols).
3. **SignatureVersion SHA-1 signing** — already fully implemented as real
   signing (not accept-and-drop): `resolveSignatureVersion`/`signWithVersion`
   in signing.go select SHA1withRSA vs SHA256withRSA per-topic and every
   delivery envelope declares the version that actually produced its
   Signature.
Verified all three against the current code (not just the PARITY.md prose)
and the full `services/sns` test suite, since a prior PARITY.md claim in this
repo (apigatewayv2/gopherstack-jni0) turned out to be false — this one
checked out as true.

4. **DataProtectionPolicy grammar** — confirmed out of scope for the full
   data-identifier/statement grammar (see deferred; also confirmed via
   docs.aws.amazon.com/sns/latest/dg/sns-message-data-protection-availability-change.html
   that the feature is frozen for new customers as of 2026-04-30). Found and
   fixed two real, narrowly-scoped wire-level gaps instead of the full
   grammar:
   - `PutDataProtectionPolicy` accepted any valid-JSON string with no length
     cap and no required-key check. aws-sdk-go-v2/service/sns@v1.42.4
     api_op_PutDataProtectionPolicy.go documents "Length Constraints: Maximum
     length of 30,720" on the `DataProtectionPolicy` field (also present in
     the pinned botocore sns/2010-03-31/service-2.json.gz model) — not
     enforced. docs.aws.amazon.com/sns/latest/dg/sns-message-data-protection-policies.html
     states "A data protection policy requires the following basic policy
     information for identification: Name ... Version ... Statement ..."
     (Description explicitly marked Optional) — none of the three required
     keys were checked. Added `validateDataProtectionPolicy`
     (topic_attributes.go) enforcing both; `testDataProtectionPolicy` and the
     JSON-validation test table were rewritten to include `Name` (the prior
     fixture `{"Version":...,"Statement":[]}` would now be rejected as
     AWS-inaccurate, since a real DataProtectionPolicy without `Name` is
     invalid).
   - **Bonus find, same investigation**: `DataProtectionPolicy` was listed as
     a settable `SetTopicAttributes` attribute name and leaked into
     `GetTopicAttributes`'s response map. Real AWS's documented AttributeName
     list for both `SetTopicAttributesInput` and `GetTopicAttributesResponse`
     (same SDK/botocore model) never mentions `DataProtectionPolicy` — it is
     managed exclusively through the dedicated Get/PutDataProtectionPolicy
     operations. This is the "more permissive than the real service" bug
     class: `SetTopicAttributes(arn, "DataProtectionPolicy", ...)` silently
     succeeded when real AWS would reject an unrecognized attribute name.
     Removed `DataProtectionPolicy` from `isKnownTopicAttribute` and filtered
     it out of `GetTopicAttributes`'s returned map; one existing test
     (`TestSNS_GetDataProtectionPolicy`/with_policy) that used
     `SetTopicAttributes` to seed the policy was reworked to call
     `PutDataProtectionPolicy` instead, matching how a real client would
     have to set it.
   Also swept platform_applications.go, platform_endpoints.go, permissions.go,
   sms.go, origination_numbers.go, tags.go, and topics.go's DeleteTopic for
   the "state mutated before validation" class (the session's most recurrent
   bug elsewhere today) — every operation here validates before mutating;
   none found.

Gates: `go build ./services/sns/...` and `go test -race ./services/sns/...`
pass clean; `golangci-lint run ./services/sns/...` reports 0 issues. The root
package (`go build ./...` / `go test .`) fails, but only inside
`services/workspaces` (a concurrent, unrelated in-progress change) — confirmed
`services/sns` alone builds and passes in isolation.

### 2026-07-25 re-audit (parity-3 phase 2, bd: gopherstack-bz6)
Closed all three items the prior pass had deliberately left in `gaps:` rather than
deferring/guessing at them. Each was re-verified against live AWS documentation
before the behavior change (not just re-asserted from the prior ledger):

1. **ArchivePolicy/ReplayPolicy FIFO-only restriction.** Re-confirmed against
   `docs.aws.amazon.com/sns/latest/dg/fifo-message-archiving-replay.html` ("Amazon
   SNS message archiving and replay is only available for application-to-application
   (A2A) FIFO topics") and `message-archiving-and-replay-topic-owner.html` (same
   console-note language, repeated verbatim). "A2A" means the subscription protocol
   must be sqs, lambda, or firehose — HTTP/HTTPS/email/email-json/sms/application are
   "A2P" (application-to-person) and are never eligible, even on a FIFO topic.
   Implemented: `ArchivePolicy` is now rejected (`InvalidParameter`) on a non-FIFO
   topic at both `CreateTopic` and `SetTopicAttributes` (`validateCreateTopicAttrs`,
   `validateTopicAttributeValue`); `ReplayPolicy` is now rejected unless the
   subscription's topic is FIFO AND its protocol is sqs/lambda/firehose
   (`validateReplayPolicyEligibleLocked`, called from `setSubscriptionAttributesLocked`
   before the attribute is applied — covers both direct `SetSubscriptionAttributes`
   calls and the attrs-at-Subscribe-time path in `handleSubscribe`, since both route
   through the same function). `replayMessagesToSubscription` (archive.go) was
   simplified to only fan out to Lambda/Firehose (SQS already goes through the
   unconditional emitter path) — the HTTP/SMS/Application branches were deleted
   since they are now provably unreachable rather than left as dead defensive code.
   Rewrote `archive_test.go`'s HTTP-replay coverage (`TestReplayPolicyTriggersHTTPReplay`
   → `TestReplayPolicyTriggersLambdaReplay`, using the existing `mockLambdaInvoker`
   test double plus a new `All()` accessor for ordering assertions, since HTTP is no
   longer a valid replay target) and `TestReplayPolicyDeliversToAllSubscriptionProtocols`
   (split into `TestReplayPolicyDeliversToA2AProtocols` — lambda/firehose only — and a
   new `TestReplayPolicyRejectedForIneligibleProtocolOrTopic` table asserting rejection
   for sms/http/https on a FIFO topic and sqs/lambda on a standard topic). Every other
   `archive_test.go`/`subscription_attributes_test.go`/`persistence_test.go` test that
   exercised ArchivePolicy/ReplayPolicy on a bare (non-`.fifo`) topic name was updated
   to a `.fifo` name — `CreateTopic` auto-sets `FifoTopic=true` from the name suffix, so
   this is a mechanical rename, not a behavior change to what each test verifies.
2. **Subscribe per-protocol Endpoint validation.** Added `validateSubscribeEndpoint`
   cases (previously only sms/email/email-json were checked) for sqs, lambda,
   firehose, application, and http/https: sqs/lambda/firehose/application must be a
   well-formed ARN (`arn:{partition}:{service}:{region}:{account}:{resource}`) for the
   matching service, with the expected resource prefix (`function:` for lambda,
   `deliverystream/` for firehose, `endpoint/` for the sns-service application
   protocol; sqs has no further resource-prefix constraint beyond the service match);
   http/https must be a syntactically valid URL whose scheme matches the protocol
   (AWS rejects an `http` Subscribe whose Endpoint is `https://...` and vice versa).
   This surfaced and fixed a handful of pre-existing test fixtures that used
   filler/placeholder endpoints ("x", "y", "q", a scheme-mismatched URL) for `sqs`/
   `http` subscriptions in scenarios unrelated to endpoint validity (pagination,
   listing, unsubscribe) — all replaced with well-formed fake ARNs/URLs. Also
   surfaced one real bug in `services/cloudformation/resources_storage_test.go`'s
   `TestResourceCreator_SNSSubscription`: it passed an SQS queue **URL**
   (`https://sqs.us-east-1.amazonaws.com/.../my-queue`) as the `Endpoint` for a
   `Protocol: sqs` `AWS::SNS::Subscription`, which is not what real AWS CloudFormation
   accepts either (the SNS Subscribe API requires the SQS **ARN** for the sqs
   protocol) — fixed the fixture to use a proper ARN.
3. **SignatureVersion-aware signing.** Confirmed against
   `docs.aws.amazon.com/sns/latest/api/API_SetTopicAttributes.html` ("By default,
   SignatureVersion is set to 1") and `sns-verify-signature-of-message.html`
   ("SignatureVersion1 – Uses an SHA1 hash of the message. SignatureVersion2 – Uses an
   SHA256 hash of the message"). Added `notificationSigner.signSHA1` /
   `resolveSignatureVersion` / `signWithVersion` (signing.go); `Publish` now resolves
   the topic's `SignatureVersion` attribute once under the read lock and threads the
   resolved value ("1" unless the attribute is explicitly "2") through
   `buildPublishedEvent` and every `httpDelivery` so HTTP/HTTPS, Lambda, and Firehose
   envelopes all sign with — and declare — the same version. `SetTopicAttributes` now
   rejects a `SignatureVersion` value other than "1"/"2". **Cross-package fix (flagged
   per HARD CONSTRAINTS):** `pkgs/events.SNSPublishedEvent` gained a new
   `SignatureVersion` field (additive, no existing field changed) so the SQS delivery
   path (`services/sqs/sns_delivery.go`, which re-embeds `ev.Signature` in its own SQS
   notification envelope) can declare the version that actually produced that
   signature — it previously hardcoded `SignatureVersion: "1"` unconditionally, which
   would have been silently wrong for any topic explicitly set to SignatureVersion=2.
   `services/sqs` was linted and its tests (`go test ./services/sqs/...`) re-run clean.
   Made observable per the task's requirement (previously "unobservable" was the
   stated reason for leaving this unfixed): `signing_test.go` now has
   `TestSignatureVersion1UsesSHA1Signing` (new — default/unset topic, verifies the
   emitted `Signature` against the signing cert using SHA1) and
   `TestSignatureVersion2UsesSHA256Signing` (rewritten from
   `TestSignatureIsValidRSASHA256`, now explicitly sets `SignatureVersion=2` before
   publishing so it genuinely exercises the SHA256 path rather than relying on it
   being the unconditional default). `TestNotificationSignatureNotMock`,
   `TestSubjectIncludedInSignature`, and `TestNotificationEnvelopeFields` were updated
   from asserting `SignatureVersion == "2"` (the old unconditional hardcode) to `"1"`
   (the real AWS default, since none of those three set the attribute).

Full-tree gates (`go build`/`go vet`/`-race` test for services/sns, services/sqs,
services/cloudformation, pkgs/events/gofmt/golangci-lint) pass clean with zero
issues; no banned nolints introduced.

### 2026-07-23 re-audit (parity-5)
Between `3d4de4f9` and this pass, `services/sns/` had only a mechanical file/test
reorg (`backend.go`/`accuracy*_test.go`/etc. split into per-op-family files, e.g.
`refactor: idiomatic file/test reorg` #2385) — diffed and confirmed no behavior
change, `buildActions()` still routes all 42 real SDK ops 1:1. SDK module pinned
at `v1.41.0` (unchanged). This pass field-diffed against live AWS documentation
(`docs.aws.amazon.com/sns/latest/api/API_Subscribe.html`,
`.../dg/subscription-filter-policy-constraints.html`, `.../api/API_Publish.html`,
`.../api/CommonErrors.html`) rather than relying on the prior ledger's "ok"
classifications, and found four real, previously-unenforced gaps, all fixed:
1. **`AuthorizationError` (and by extension `SubscriptionLimitExceeded`/
   `FilterPolicyLimitExceeded`) were returning HTTP 400, not 403.**
   `handleBackendError` had exactly two status buckets — the default 400 and an
   explicit 500 for unmapped errors — with no 403 path at all, despite the prior
   ledger's own note claiming "400 vs 500 split verified". Every AWS SNS error
   table (Subscribe, Publish, ...) documents `AuthorizationError` as HTTP 403.
   Fixed by adding a `http.StatusForbidden` case in `handleBackendError`.
   `EndpointDisabled` was re-confirmed to correctly stay 400 (verified against
   `API_Publish.html` — it is NOT 403 despite being permission-adjacent).
2. **`FilterPolicyLimitExceeded` did not exist at all** — no sentinel error, no
   quota enforcement. Real AWS SNS caps filter-policy-bearing subscriptions at
   200/topic and 10,000/account (both adjustable) and returns this exact error
   code (HTTP 403) when exceeded (documented on `Subscribe`'s Errors table).
   Added `ErrFilterPolicyLimitExceeded`, `maxFilterPoliciesPerTopic`,
   `maxFilterPoliciesPerAccount`, and `checkFilterPolicyQuotaLocked` (called from
   both `Subscribe` and `SetSubscriptionAttributes`, with self-exclusion so
   updating a subscription's own existing filter policy in place doesn't
   double-count).
3. **`SubscriptionLimitExceeded` did not exist at all.** Real AWS caps
   subscriptions at 12,500,000/topic (fixed quota) and returns this error (HTTP
   403, "the customer already owns the maximum allowed number of
   subscriptions") when exceeded. Added `ErrSubscriptionLimitExceeded` and a
   `subscriptionLimitPerTopic` backend field (defaults to the real 12.5M quota,
   overridable via `SetSubscriptionLimitPerTopicForTest` so tests don't need to
   create millions of subscriptions to exercise the path).
4. **FilterPolicy's "maximum of five keys" constraint was completely
   unenforced** — `parseFilterPolicy` checked size (256 KiB) and total
   combination/condition count (150) but never the AWS-documented 5-key-per-policy
   cap. Added a `len(rawPolicy) > maxFilterPolicyKeys` check. Note: this backend
   does not parse genuinely nested MessageBody filter policies (pre-existing,
   documented gap — "Nesting depth ... not yet enforced"), so the 5-key check
   uses top-level key count for both scopes; AWS's real leaf-key counting for
   nested payload-based policies is not replicated. The check is also applied
   recursively to each `$or` sub-policy object (each is parsed via the same
   `parseFilterPolicy` path) — this is a reasonable but NOT doc-confirmed
   interpretation of how the 5-key cap composes with `$or`; flagging for the
   next auditor rather than asserting certainty.
All four fixes are covered by new tests in `subscription_limits_test.go` and
`permissions_test.go`. Gates (build/vet/`-race` test/gofmt/golangci-lint) pass
clean with zero issues; no banned nolints introduced.

### 2026-07-11 re-audit (parity-4)
No code changes made — no genuine bugs found. `services/sns/` had zero commits between
the prior ledger's `last_audit_commit` and current HEAD (that prior audit's own commit,
`ce30166a`, is what's actually recorded in the ledger, so there was no local drift to
re-check). SDK bumped `v1.40.3` -> `v1.41.0`: changelog is serialization-snapshot-test
and dependency-only, zero new/changed SNS operations. Re-verified: `buildActions()`
still routes all 42 real SDK operations 1:1 (no stubs, no missing ops); the four fixes
called out in the prior pass are intact (`topicMessageArchive` cleanup on DeleteTopic +
persistence round-trip, PublishBatch per-entry `MessageAttributes.` prefix, `ErrOptedOut`
sentinel text, `appendBounded`-capped delivery buffers). All gates
(build/vet/`-race` test/`go fix -diff`/golangci-lint) pass clean with zero issues.

### Protocol
SNS is the **AWS query (XML) protocol** (`Version=2010-03-31`, form-encoded request,
XML response with a `ResponseMetadata`/`RequestId` wrapper). PublishBatch entries and
per-entry MessageAttributes use the query-protocol list/map indexing convention:
`Parent.member.N.Field`, and nested maps use `Parent.member.N.MapField.entry.M.Key/Value`.
**Trap**: it is easy to drop a nesting segment when building one of these prefixes by
hand (see the PublishBatch fix this pass) — always cross-check against
`serializers.go` in the vendored SDK (`go list -m -f '{{.Dir}}' .../service/sns` to find
the extracted module, or `go get` into a scratch module) rather than trusting a
hand-derived guess.

### FilterPolicy matching semantics
- Default `FilterPolicyScope` is `MessageAttributes`; `MessageBody` requires the
  published message to parse as a JSON object, otherwise nothing matches.
- A `String.Array` message attribute (JSON array in `StringValue`) expands to one
  candidate per element; the condition matches if ANY element matches (OR).
- `$or` is only treated as the OR operator when its array has ≥2 elements, every
  element is a JSON object, and no element uses a reserved operator keyword
  (`prefix`/`suffix`/`equals-ignore-case`/`anything-but`/`exists`/`numeric`/`wildcard`/
  `cidr`) as a top-level field — otherwise `$or` is an ordinary attribute name.
- `numeric` conditions are `[op, num, op, num, ...]` pairs, ANDed together.
- Non-existent attributes still get a *single* match attempt against `exists:false`
  and `anything-but` conditions — do not special-case "attribute missing" as "no match"
  without checking those two operators first.
- 150 total attribute conditions across a policy (including `$or` sub-policies), and a
  256 KiB serialized-policy size cap, both enforced at Subscribe/SetSubscriptionAttributes
  time (fail fast) not at match time.

### Per-protocol message envelope shapes
- **HTTP/HTTPS and Firehose** (when `RawMessageDelivery=false`, the AWS default): full
  `Notification` JSON envelope — `Type, MessageId, TopicArn, Subject?, Message, Timestamp,
  SignatureVersion, Signature, SigningCertURL, UnsubscribeURL`. `RawMessageDelivery=true`
  delivers the bare message body only.
- **Lambda**: ALWAYS the full envelope wrapped in `{"Records":[{"EventVersion":"1.0",
  "EventSource":"aws:sns","EventSubscriptionArn":...,"Sns":{...}}]}` — Lambda does NOT
  support `RawMessageDelivery` (unconditional envelope, unlike HTTP/SQS/Firehose).
- **SQS**: delivered via the `pkgs/events` emitter (`SetPublishEmitter`/`SubscribeToSNS`
  in cli.go, out of scope for this service) — SNS's job ends at emitting
  `events.SNSPublishedEvent` with the same Timestamp/Signature/SigningCertURL used
  everywhere else for this publish.
- **SMS/Application(mobile push)**: no real external sink in this mock; delivery is
  recorded (`SMSDelivery`/`ApplicationDelivery`) and drained via `DrainSMSDeliveries`/
  `DrainApplicationDeliveries` for test/dashboard observability. These buffers are now
  bounded (`maxRecordedDeliveries`) — do not remove the cap thinking it is dead code.
- **Trap**: as of this pass, every channel for a single `Publish` call shares ONE
  `Timestamp`/`Signature`/`SigningCertURL`, built once in `buildPublishedEvent` and
  threaded through to the SQS emitter, Lambda, and Firehose. Do not reintroduce a
  second, independent `time.Now()`/sign call per channel — that was the bug (Lambda
  used to fabricate `uuid.NewString()` as a fake "signature").

### Signature / MD5
- SNS signs the **Notification envelope** (RSA, canonical string of
  Message/MessageId/Subject?/Timestamp/TopicArn/Type sorted by field name,
  newline-separated) — this is NOT the same as SQS's `MD5OfMessageBody`. **SNS's
  `Publish`/`PublishBatch` responses do NOT include any MD5 field** — that's an
  SQS-only concept; don't add one here, it would be a fabricated field AWS never
  returns.
- **Fixed 2026-07-25**: this backend now signs with SHA1withRSA by default
  ("SignatureVersion 1", matching real AWS's documented default) and only switches to
  SHA256withRSA ("SignatureVersion 2") when the topic's `SignatureVersion` attribute
  is explicitly set to `"2"`. `resolveSignatureVersion`/`signWithVersion`
  (signing.go) select the hash; `Publish` resolves the topic's attribute once under
  the read lock and threads it through `buildPublishedEvent`/`httpDelivery` so every
  channel (HTTP/HTTPS, Lambda, Firehose, and — via the new
  `events.SNSPublishedEvent.SignatureVersion` field — the SQS delivery envelope built
  by `services/sqs`) signs with, and declares, the same version. Both PKCS1v15/SHA1
  and PKCS1v15/SHA256 use Go's standard `rsa.SignPKCS1v15`, so there is no bespoke
  padding logic to get subtly wrong.

### Subscription ARN format
- Confirmed subscription ARN: `arn:{partition}:sns:{region}:{account}:{topicName}:{uuid}`
  (built via `arn.Build("sns", region, accountID, topicName+":"+uuid.New().String())`).
- **Pending** (unconfirmed) HTTP/HTTPS/email/email-json subscriptions return the
  **literal string** `"pending confirmation"` (lowercase, with a space) as the
  `SubscriptionArn` in `Subscribe`'s response AND in `ListSubscriptions`/
  `ListSubscriptionsByTopic` — this is NOT a placeholder ARN, it is the exact string
  AWS returns. `Subscribe`'s `ReturnSubscriptionArn=true` parameter overrides this and
  always returns the real ARN even while pending.
- SQS, Lambda, Firehose, SMS, and Application subscriptions are auto-confirmed
  (`PendingConfirmation=false` immediately); only HTTP, HTTPS, email, and email-json
  require confirmation.

### FIFO topics
- `MessageGroupId` is required on every Publish/PublishBatch entry to a `.fifo` topic.
- `ContentBasedDeduplication=true` forbids an explicit `MessageDeduplicationId` (uses
  SHA-256 hex of the message body instead); `false` (default) requires one.
- Dedup window is 5 minutes, keyed by `topicArn + "/" + dedupID`; a duplicate within
  the window returns a **new** synthesized MessageId/SequenceNumber without
  re-publishing or re-delivering — this mirrors real AWS (dedup is silent, not an error).
- `SequenceNumber` is a 20-digit zero-padded, per-topic monotonic counter — not derived
  from message content or timestamp.

### Locking
`InMemoryBackend` uses one coarse `lockmetrics.RWMutex` per the pkgs-catalog rule.
`notificationSigner` additionally has its own small `sync.RWMutex` guarding just its
mutable `certURLValue` field (set once via `SetSigningCertBaseURL` when the mock
server's address becomes known, read on every signed delivery) — this is a
single-field auxiliary lock, not a second backend-resource lock, so it does not
violate the "one coarse lock per backend" rule.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404**:
same shape as autoscaling's entry (see that entry or gopherstack-3a8t for the full
survey/rationale). `RouteMatcher` now falls back to
`service.MatchesUserAgentMarker(c.Request().Header, "api/sns")` (verified against the
pinned `sns@v1.42.4/api_client.go:638` `AddSDKAgentKeyValue` call) only on the `ReadBody`
failure branch, leaving the existing `Version`-substring matching untouched.

**Left alone deliberately**: `Handler()`'s own `c.Request().ParseForm()` call was NOT
migrated. `ExtractOperation`/`ExtractResource` already use `httputils.ReadBody`, not
`ParseForm`, so `Handler()`'s is the *only* `ParseForm()` call for a given request -- the
docdb/neptune double-call landmine (a second call silently seeing a cached-empty,
non-nil `r.PostForm`) does not apply here, verified via the oversized-body test below.
However, every action handler in this package (`handleCreateTopic` and ~50 others) reads
parameters via `c.Request().FormValue(...)`, which depends on `r.Form` already being
populated by that one `ParseForm()` call; migrating it to `httputils.ReadBody` would mean
threading parsed `url.Values` through every action handler, well beyond this bug's blast
radius. The read failure IS surfaced (not silently emptied) -- just mapped to
`InvalidParameter`/400 rather than a 500-class code, a pre-existing wrong-code gap, not a
masked-empty-body bug. Not fixed; flagged here rather than silently left for the next
pass to rediscover.

Proof: `TestHandler_OversizedBodySurfacesTypedError` in `handler_oversized_body_test.go`
drives a real SNS SDK client through `service.NewRegistry`/`service.NewServiceRouter`,
confirmed failing pre-fix with `UnknownError`; passes now with a typed
`InvalidParameter`/400 (not `InternalFailure`, see above). `TestHandler_NormalSizedBodyStillRoutes`
is the regression guard. Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race
./services/sns/...` (pass), `golangci-lint run ./services/sns/...` (0 issues).

**2026-08-22 (gopherstack-ioww) -- the `InvalidParameter`/400 wrong-code gap flagged above,
fixed without migrating off `ParseForm`**: the `ParseForm` migration this entry's own note
flagged as the risk was not needed. `Handler()` now calls `httputils.ReadBody(c.Request())`
itself, immediately before `c.Request().ParseForm()`, and answers a `ReadBody` failure with
`"InternalError"`/500 instead of letting it fall into `ParseForm`'s own error, which was
always mapped to `"InvalidParameter"`/400 regardless of cause.

This works because of how `pkgs/httputils.ReadBody` and `net/http`'s own `ParseForm` compose
on the same `r.Body`: a first `ReadBody` failure (oversized body, over
`httputils.MaxRequestBodyBytes`) replaces `r.Body` with a `bodyReadErrCloser` that returns
the identical cached error on every subsequent read (see the type's doc comment in
httputils.go) -- so `parsePostForm`'s own read (`net/http`'s `request.go`, `parsePostForm`,
which applies its own independent 10 MiB cap via `io.LimitReader` since `r.Body` is not a
`*maxBytesReader`) sees that same error immediately and `ParseForm` returns it verbatim. A
successful first `ReadBody` instead replaces `r.Body` with a `bodyReadCloser` wrapping a
seekable `*bytes.Reader`, which the added call rewinds (`Seek(0, io.SeekStart)`) on this
second invocation, so `ParseForm`'s subsequent read still sees the full body and succeeds
exactly as before. Net effect: the pre-check adds no second real read, changes nothing about
the successful path, and turns only the read-failure case into a distinguishable branch --
no `url.Values` needs threading through the ~50 `c.Request().FormValue(...)` call sites this
entry flagged as out of scope for a full migration, and none were touched.

`"InternalError"` (not `InternalFailure`) is confirmed as SNS's own modelled code for this:
`sns@v1.42.4/types/errors.go:198-220`, `InternalErrorException.ErrorCode()` returns
`"InternalError"` with `ErrorFault() == smithy.FaultServer`; wired into the error-code switch
of effectively every operation's deserializer in `deserializers.go` (39
`case strings.EqualFold("InternalError", errorCode)` sites across 34 op-level error
deserializers) -- not a code invented for this fix, and already this package's existing
fallback in `errorCode()`/`handleBackendError()` (handler_errors.go) for any unclassified
error, so this now uses the same convention rather than a new one.

Proof: `TestHandler_OversizedBodySurfacesTypedError` updated -- confirmed failing pre-fix
(got `"InvalidParameter"`, `ErrorFault() == FaultClient`); passes now with `"InternalError"`
and `FaultServer`. `TestHandler_NormalSizedBodyStillRoutes` (unchanged) is the regression
guard, plus the full `-race` suite confirms none of the `FormValue` call sites regressed.
Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race ./services/sns/...` (pass,
~21s), `golangci-lint run ./services/sns/...` (0 issues, 0 new nolints). No exported
signature changed.

**2026-08-30 (negative-continuation-token sweep)**: `pagination.go`'s `decodeToken` accepted
a token that base64-decoded to a negative integer and returned it verbatim; `paginate`'s
`offset >= len(items)` guard does not catch a negative offset, so `items[offset:end]`
(the 8 call sites: `origination_numbers.go`, `platform_endpoints.go`,
`platform_applications.go`, `sms.go` x2, `subscriptions.go` x2, `topics.go`) panicked with
`slice bounds out of range [-5:]` given `LTU=` (base64 for `-5`) as `NextToken`. Fixed at the
decode site: `decodeToken` now rejects a negative offset the same way it already rejects
malformed base64 or a non-integer payload, so all 8 callers inherit the fix without change.
`pagination_test.go`'s existing suite asserted page contents/token presence only — no test
supplied a hostile token before this pass.

Proof: `TestSNSPagination_NegativeToken` (`pagination_test.go`) confirmed panicking pre-fix,
passes now. Gates: `go build ./services/sns/...`, `go vet ./services/sns/...`, `go test -race
-count=1 ./services/sns/...`, `golangci-lint run ./services/sns/...` (0 issues). Work left
uncommitted per this pass's instructions.

**2026-08-30 (wrapper-key-sweep cross-call pagination-reproducibility audit)**: audited every
`sns` listing (`ListTopics`/`ListTopicsInRegion`, `ListPlatformApplications`,
`ListEndpointsByPlatformApplication`, `ListSubscriptions`/`ListSubscriptionsByTopic`,
`ListOriginationNumbers`, `ListSMSSandboxPhoneNumbers`, `ListPhoneNumbersOptedOut`) for
whether the full sorted order is reproducible between two calls with nothing changed in
between — the class described in `.claude/memories/parity-principles.md`'s wrapper-key
sweep: a `store.Table.All()`/map walk feeding a sort whose key can tie drops or duplicates a
record at a page boundary. Every one of these sorts by its own `store.Table` key (TopicArn,
PlatformApplicationArn, EndpointArn, SubscriptionArn, PhoneNumber), or, for
`ListPhoneNumbersOptedOut`, by the phone-number string that is itself the source map's own
key — so no tie is possible regardless of the underlying walk order. `ListOriginationNumbers`
sorts by `PhoneNumber` (not obviously unique) but its source is a direct per-region slice
(`b.originationNumbers[b.region]`), not a map walk, so it is stable across calls independent
of any tie. No pagination-reproducibility bug found; nothing changed. This confirms/extends
(does not contradict) the negative-token pass above.

**2026-08-30 (value-semantics audit, gopherstack-uox6)**: read `filter_match.go`/
`filter_policy.go` (subscription `FilterPolicy` matching for both `MessageAttributes` and
`MessageBody` scope) against their documented semantics. `FilterPolicy` is a freeform JSON
string (`SubscribeInput.FilterPolicy` is `*string` in the pinned SDK, no typed matcher
surface), so this was verified against SNS's own user-guide pages
(`sns-subscription-filter-policies.html`, `numeric-value-matching.html`,
`string-value-matching.html`) rather than SDK doc comments -- one exception:
`MessageAttributeValue.DataType`'s own doc comment ("Amazon SNS supports the following
logical data types: String, String.Array, Number, and Binary") is the authoritative type
list, overriding a stray `"Number.Array"` example on the numeric-matching doc page that
doesn't correspond to any type the SDK itself declares.

Own count: 19 `match`/`Match`-prefixed functions in `filter_match.go`, all genuine filter
predicates (no HTTP-routing false positives in this file -- `RouteMatcher`/`MatchPriority`
live in `handler.go` and were excluded).

Found and fixed one bug: `filter_policy.go`'s `validateNumericOperands` whitelisted `"<>"` as
a sixth numeric operator, and `filter_match.go`'s `numericOpMatches` implemented a
not-equal comparison for it. SNS's numeric-value-matching page documents exactly five
operators -- `=`, `<`, `<=`, `>`, `>=` -- with no `"<>"` form anywhere on the page or its
range-matching/anything-but sections; a `"<>"` operand should be rejected at Subscribe/
SetSubscriptionAttributes time the same way `"??"` already is, not silently accepted and
evaluated. `filter_policy_test.go`'s `TestNumericValidOperatorsAccepted` asserted `"<>"` as
one of the valid, accepted operators -- the wrong-assertion-as-correct this audit class looks
for. Fixed by removing `"<>"` from both the validation whitelist and the comparator switch;
removed the `"<>"` entry from that test's table (6 -> 5 loop iterations, one assertion
dropped -- it was asserting the bug) and added
`TestNumericOperatorNotEqualRejectedAtSubscribeTime` asserting rejection in its place (1 new
assertion), confirmed failing against unmodified code (subscribe succeeded with no error)
before the fix. Net assertion count for the file is unchanged; the dropped assertion tested
the wrong behavior and is replaced by a new one testing the correct behavior for the same
input.

Every other matcher in `filter_match.go` was checked against the same doc pages and is
correct: `prefix`/`suffix`/`equals-ignore-case` (SNS does not document a nested
`{"prefix": {"equals-ignore-case": ...}}` form the way EventBridge does, and this file
correctly does not implement one), `wildcard` (only `*` is a metacharacter, matching the
doc's single-character-wildcard-not-supported note already in this file's own comment --
verified rather than trusted), `cidr` including the bare-host-IP case (doc's own wording
"IP address or subnet" supports treating a bare IP as an implicit host route), `anything-but`
in all its documented forms (scalar, list, and the three nested `prefix`/`suffix`/
`equals-ignore-case`/`wildcard` forms -- correctly does NOT implement a nested `numeric`
form, which SNS's `anything-but` docs also do not list), numeric range matching (multiple
pairs AND, matching the range-matching example), and `String.Array`/`MessageBody`-array
expansion (OR across elements, matching the doc's own "matches ... because it contains a
value that isn't ..." examples for both `String.Array` attributes and JSON-array body
values). `anything-but` combined with `"exists": false"` (documented as a supported
combination) is not special-cased anywhere and needs none: it already falls out of
`matchesConditions`' existing OR-across-conditions loop.

Unrecognised filter keys: `parseFilterPolicy` already rejects any object-condition operator
name outside `knownFilterPolicyOperators` at Subscribe/SetSubscriptionAttributes time
(`validateConditionShapes`), so there is no silent-match-everything or silent-match-nothing
path for this service -- confirmed by `TestSNS_FilterPolicyValidation/rejects_unknown_operator_name`
already in the suite and unchanged by this pass.

Tests: `filter_policy_test.go` gained one function (19 -> 20 `func Test...`);
`TestNumericValidOperatorsAccepted` 6 -> 5 loop-driven assertions (1 dropped, see above, not
a weakening); `TestNumericOperatorNotEqualRejectedAtSubscribeTime` added, 1 new assertion,
confirmed failing pre-fix.

Gates: `go build ./services/sns/...`, `go vet ./services/sns/...`, `go test -race -count=1
./services/sns/...` (pass), `golangci-lint run ./services/sns/...` (0 issues, no new
nolints). No backend/exported signature changed, so no repo-wide `go vet` was required.

## 2026-08-31 exact-case element check (gopherstack-21my)

Re-verified byte-for-byte, not by folding comparison: every list, map and
nested-item response shape checked against the exact string literals the pinned
deserializer matches on. No hard mismatch, and no case-only mismatch either.

The case-only class is the one worth naming here. This service is query
protocol with XML responses, and smithy-go's XML decoder matches element names
with EqualFold, so a name differing only in case decodes correctly and no
round-trip test can see it. It is still wrong - it is not what AWS emits, and
any consumer matching exactly would break. None was found.

Every list in this service is member-wrapped rather than flattened, confirmed
by there being no call site of an unwrapped-list deserializer variant.

## 2026-08-31 Error-envelope sweep (gopherstack-uox6, errtargetaudit, post-reachability-fix)

`errtargetaudit -dir sns` reported 4 class-A findings across 2 causes,
verified against the pinned SDK's own per-op `awsAwsquery_deserializeOpError<Op>`
switch (`aws-sdk-go-v2/service/sns@v1.42.4` deserializers.go -- the older
`EqualFold` cascade shape, not the newer `rpc2` plain switch).

**3 real, 2 distinct fixes:**

- `DeleteSMSSandboxPhoneNumber` (`sms.go:96`) and `VerifySMSSandboxPhoneNumber`
  (`sms.go:114`): both declare `ResourceNotFound` (not `NotFound`). Both
  return `ErrPhoneNumberNotFound`, whose own sentinel text already reads
  `"ResourceNotFound"` (`errors.go:16`) -- but `handler_errors.go`'s
  `errorCode()` grouped it into the same `case` as
  `ErrTopicNotFound`/`ErrSubscriptionNotFound`/
  `ErrPlatformApplicationNotFound`/`ErrEndpointNotFound`, all of which
  correctly return `"NotFound"`. Fixed by giving `ErrPhoneNumberNotFound`
  its own case returning `"ResourceNotFound"`, matching its own message
  text. `ErrPhoneNumberNotFound` has exactly these 2 call sites (both
  fixed by the one switch change) -- not a call-site override of a shared
  sentinel, a correction of the mapper's own grouping.
- `DeletePlatformApplication` (`platform_applications.go`, was line 161):
  declares only `AuthorizationError`/`InternalError`/`InvalidParameter` --
  no not-found type at all, the same shape as this SDK's `DeleteEndpoint`
  (documented `"This action is idempotent"`). `ErrPlatformApplicationNotFound`
  has 4 other call sites (`GetPlatformApplicationAttributes`,
  `SetPlatformApplicationAttributes`, `CreatePlatformEndpoint`,
  `ListEndpointsByPlatformApplication`), all of which correctly declare
  `"NotFound"` -- so the sentinel itself is untouched; only
  `DeletePlatformApplication`'s own not-found check is removed, making
  delete-of-a-missing-ARN a no-op (its own doc comment does not use the
  word "idempotent" the way `DeleteEndpoint`/`DeleteTopic` do, so this is
  an inference from the declared-error-set shape, not a direct doc
  statement -- recorded as such, not overclaimed).

**1 refusal:**

- `Publish` (`publish.go`, was line 430, `OptedOut` for an opted-out SMS
  destination): `Publish`'s own switch does not declare `OptedOut`
  (`OptedOutException` exists in this SDK and is correctly declared by
  `CreateSMSSandboxPhoneNumber`, but not by `Publish`). The two
  closest-sounding declared types (`InvalidParameter`,
  `ParameterValueInvalid`) share the identical generic doc comment
  ("Indicates that a request parameter does not comply with the associated
  constraints") -- neither describes an opted-out recipient, so neither was
  substituted, per this campaign's standing near-miss caution. Not fixed;
  `Publish` still sends an undecodable `OptedOut` code to a real typed
  client for this one case.

New SDK-driven test (`error_envelope_fixes_test.go`):
`TestSMSSandboxPhoneNotFound_ResourceNotFound_RealClient` (2 subtests,
`errors.As` against `*types.ResourceNotFoundException`) and
`TestDeletePlatformApplication_MissingArn_Idempotent_RealClient`
(asserts `require.NoError`) -- both confirmed failing pre-fix (the sandbox
subtests got `*smithy.GenericAPIError` wrapping `NotFound`; the delete test
got a 400 `NotFound` error instead of success).

One existing test corrected: `platform_applications_test.go`'s
`TestSNSHandler_DeletePlatformApplication`'s `"not_found"` case asserted
`wantStatus: http.StatusBadRequest` (a status-only assertion, not a typed
one) for deleting a nonexistent ARN -- renamed `"not_found_is_idempotent"`,
now `wantStatus: http.StatusOK`, assertion count unchanged. No other
existing test needed correction: the backend-level tests in
`platform_applications_test.go`/`sms_test.go` assert against the Go
sentinels (`sns.ErrPlatformApplicationNotFound`,
`sns.ErrPhoneNumberNotFound`) directly, which are unchanged by this pass.

Gates: `go build ./services/sns/...`, `go vet ./...` (repo-wide, clean --
no exported signature changed), `go test -race -count=1 ./services/sns/...`
(pass), `golangci-lint run ./services/sns/...` (0 issues, no `nolint` in
any edited file).
