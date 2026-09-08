service: applicationautoscaling
sdk_module: aws-sdk-go-v2/service/applicationautoscaling@v1.45.4
last_audit_commit: d0f3046ef
last_audit_date: 2026-09-04
overall: A            # real, wire-breaking bugs found and fixed
ops:
  RegisterScalableTarget: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "upsert confirmed; RoleARN/Tags/SuspendedState correctly left unchanged when omitted on update. FIXED: over-tag-limit now reports LimitExceededException (RegisterScalableTarget's modeled error set has no TooManyTagsException, confirmed against the vendored SDK's deserializeOpErrorRegisterScalableTarget), not ValidationException. FIXED (gopherstack-8xo): MinCapacity/MaxCapacity were plain int32 on the wire and backend signature, so omitting either field on an update call (e.g. a client that only wants to change RoleARN) decoded as 0 and silently reset the scalable target's capacity to 0 -- real AWS models both as *int32, 'required when registering a new scalable target' only (api_op_RegisterScalableTarget.go field docs), and the op doc states 'Any parameters that you don't specify are not changed by this update request.' Changed MinCapacity/MaxCapacity to *int32 end to end (wire, RegisterScalableTarget, updateExistingTarget); omitted on update now correctly preserves the stored value, still required when registering a brand-new target."}
  DeregisterScalableTarget: {wire: ok, errors: fixed, state: ok, persist: ok, note: "cascades delete to scaling policies + scheduled actions for the same (ns,resourceId,dimension), matching real AWS. FIXED: ObjectNotFoundException HTTP status was 404, now 400 (see notes)."}
  DescribeScalableTargets: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED: NextToken is now an opaque base64 cursor (was the raw sort key) and a malformed token now returns InvalidNextTokenException/400 (DescribeScalableTargets' modeled error set includes it). Added PredictedCapacity field (always omitted -- see notes)."}
  PutScalingPolicy: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "FIXED (prior pass): default PolicyType was TargetTrackingScaling, real default is StepScaling; PolicyARN colon-vs-slash. FIXED prior pass: (1) now requires the scalable target to already be registered, raising ObjectNotFoundException otherwise -- PutScalingPolicy's modeled error set includes it and its doc text names 'any operation that depends on the existence of a scalable target'; a client could previously PutScalingPolicy against a namespace/resourceId/dimension that was never registered, which real AWS rejects. (2) PredictiveScalingPolicyConfiguration was accepted by the real API but silently dropped -- now captured, persisted, and echoed by DescribeScalingPolicies. (3) enforces the real, documented AWS quotas: 50 scaling policies/scalable target and 20 step adjustments/step-scaling-policy, raising LimitExceededException. DOWNGRADED this pass: Alarms (CloudWatch alarm references) is a real field on both PutScalingPolicy's and DescribeScalingPolicies' response shapes; the prior pass synthesized stable-looking Alarm name+ARN entries for it, but those ARNs pointed at CloudWatch alarms that do not actually exist anywhere (gopherstack's applicationautoscaling backend has no cross-service reference to the cloudwatch backend) -- a caller querying cloudwatch:DescribeAlarms with that ARN would get nothing back. That is exactly the invented-resource fabrication this project removes elsewhere, so Alarms is now honestly left empty (nil/omitted) for every policy type instead. See gaps/deferred."}
  DeleteScalingPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeScalingPolicies: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (prior pass): deleted the invented PolicyARNs filter field/behavior -- confirmed against DescribeScalingPoliciesInput/its serializer in the vendored SDK, real AWS has no such filter (only PolicyNames/ResourceId/ScalableDimension/ServiceNamespace). FIXED (prior pass): NextToken is now opaque base64 with InvalidNextTokenException on malformed input. FIXED (prior pass): PredictiveScalingPolicyConfiguration now populated. DOWNGRADED this pass: Alarms now honestly empty (see PutScalingPolicy)."}
  DescribeScalingActivities: {wire: fixed, errors: fixed, state: ok, persist: n/a, note: "scalingActivities intentionally ephemeral; most-recent-first via slices.Backward; NextToken now opaque base64 with InvalidNextTokenException on malformed input. Added Details/NotScaledReasons wire fields (always empty/omitted -- see gaps, IncludeNotScaledActivities is accepted but vacuous)."}
  PutScheduledAction: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "FIXED (prior pass): StartTime/EndTime epoch-seconds; ARN colon-vs-slash. FIXED (prior pass): now requires the scalable target to already be registered (ObjectNotFoundException), same rationale as PutScalingPolicy. Enforces the real, documented, non-adjustable AWS quota of 200 scheduled actions/scalable target, raising LimitExceededException. FIXED (gopherstack-8xo): on update, StartTime/EndTime were only overwritten when the caller resent them, otherwise silently keeping the old values -- the exact opposite of the documented behavior: 'To update a scheduled action, specify the parameters that you want to change. If you don't specify start and end times, the old values are deleted.' Now always overwritten with whatever the caller sent (nil included), matching the doc. FIXED (gopherstack-8xo): Schedule was required on every PutScheduledAction call including updates, but PutScheduledActionInput does not mark it 'This member is required' (only ResourceId/ScalableDimension/ScheduledActionName/ServiceNamespace are), consistent with the same 'specify the parameters you want to change' update semantics -- Schedule is now only required when registering a brand-new action, and is left unchanged when omitted on an update."}
  DeleteScheduledAction: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeScheduledActions: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "StartTime/EndTime/CreationTime/LastModifiedTime output already correctly used epoch seconds. FIXED: NextToken now opaque base64 with InvalidNextTokenException on malformed input."}
  ListTagsForResource: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED: not-found now reports ResourceNotFoundException (with ResourceName), not ObjectNotFoundException -- ListTagsForResource/TagResource/UntagResource are modeled with ResourceNotFoundException only, confirmed against each op's deserializeOpError* switch in the vendored SDK. FIXED 2026-08-23 (gopherstack-wlo1): an empty ResourceARN reported ValidationException, a code ListTagsForResource's own deserializeOpError switch cannot type (it has no ValidationException case, unlike TagResource/UntagResource) -- removed the dedicated empty-ARN guard so it falls through to the same not-found path, which types correctly."}
  TagResource: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED: not-found -> ResourceNotFoundException (see ListTagsForResource). FIXED: over-tag-limit now reports TooManyTagsException (with ResourceName), not ValidationException -- TagResource is the one op actually modeled with TooManyTagsException."}
  UntagResource: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED: not-found -> ResourceNotFoundException (see ListTagsForResource)."}
  GetPredictiveScalingForecast: {wire: fixed, errors: fixed, state: fixed, persist: n/a, note: "FIXED (prior pass): epoch-seconds wire format end to end; unknown-policy error switched from ObjectNotFoundException to ValidationException -- GetPredictiveScalingForecast's modeled error set is {InternalServiceException, ValidationException} only (confirmed against awsAwsjson11_deserializeOpErrorGetPredictiveScalingForecast in the vendored SDK); a real aws-sdk-go-v2 client's typed-error matching on ObjectNotFoundException would never have fired here. DOWNGRADED (prior pass): forecast data was previously a flat synthetic 10.0-per-hour simulation -- a fabricated curve a caller could mistake for a real ML-produced forecast. gopherstack has no real metric history to forecast from, so CapacityForecast/LoadForecast now honestly return zero data points for every request instead (which also matches real AWS's own behavior for a predictive scaling policy that hasn't yet accumulated enough history). See gaps. FIXED this pass (2026-08-20, wrapper-key/nested-shape sweep): LoadForecast[].MetricSpecification is types.PredictiveScalingMetricSpecification -- an OBJECT (types/types.go, 'This member is required') -- but gopherstack was emitting a synthesized STRING (`fmt.Sprintf(\"%s/%s/%s\", ...)`) for it on every call. A real aws-sdk-go-v2 client's JSON unmarshal of that field into the typed struct fails outright ('unexpected JSON type ...'), breaking the ENTIRE GetPredictiveScalingForecast call for every caller, not a cosmetic mismatch -- classic shape (c), wrong JSON type. Proven with a new real-SDK round-trip test (TestGetPredictiveScalingForecast_SDKRoundTrip in wire_sdk_roundtrip_test.go) that failed with exactly that deserialization error before the fix and passes after. Fixed in forecast.go/models.go/handler_forecast.go: LoadForecastData.MetricSpecification (and the wire loadForecastOutput.MetricSpecification) changed from string to map[string]any, and GetPredictiveScalingForecast now echoes back the caller's own PredictiveScalingPolicyConfiguration.MetricSpecifications[0] (real, caller-supplied data from PutScalingPolicy's raw passthrough map) instead of fabricating a placeholder string -- LoadForecast is an empty slice (no entries) when the policy has no configured metric spec, rather than one entry with an invalid-type field. Corrected TestHandler_GetPredictiveScalingForecast_HonestlyEmpty, which had asserted the fabricated string as if it were the correct wire value."}
families:
  tagging: {status: ok, note: "TagResource/ListTagsForResource/UntagResource operate on scalable-target ARNs only, matching real AWS (Application Auto Scaling only supports tagging scalable targets)"}
  error_types: {status: fixed, note: "Every modeled AWS exception (ConcurrentUpdateException/FailedResourceAccessException/InternalServiceException/InvalidNextTokenException/LimitExceededException/ObjectNotFoundException/ResourceNotFoundException/TooManyTagsException/ValidationException) now has a distinct sentinel in errors.go and a correct HTTP status in handler.go's handleError, matching each type's ErrorFault() classification in the vendored SDK's types/errors.go: FaultServer (ConcurrentUpdateException, InternalServiceException) -> HTTP 500; FaultClient (everything else) -> HTTP 400. Previously ObjectNotFoundException incorrectly returned 404, ValidationException(ErrAlreadyExists) incorrectly returned 409, and TooManyTagsException/LimitExceededException/InvalidNextTokenException/ResourceNotFoundException/ConcurrentUpdateException/FailedResourceAccessException did not exist as distinct types at all (their scenarios either fell through to a generic ValidationException/404 or were simply unreachable). ConcurrentUpdateException/FailedResourceAccessException specifically remain without a backend-state trigger but are reachable via chaos fault injection -- see deferred."}
  quotas: {status: fixed, note: "FIXED this pass (gopherstack-cdxe): RegisterScalableTarget now enforces the real, documented per-account/per-region 'scalable targets per resource type' AWS quota (5,000 for dynamodb, 3,000 for ecs, 1,500 for cassandra/Keyspaces, 500 for every other ServiceNamespace -- see maxScalableTargetsForNamespace in scalable_targets.go), raising LimitExceededException once exhausted. Upserting an already-registered target does not consume additional quota. Combined with the prior pass's 50 scaling policies/target, 200 scheduled actions/target, and 20 step adjustments/policy quotas, every documented Application Auto Scaling quota is now enforced."}
gaps:
  - DescribeScalingActivities accepts IncludeNotScaledActivities (now threaded into the backend filter, and the response shape now has NotScaledReasons/Details fields) but it remains observably vacuous: gopherstack's mock backend never generates "not scaled" activities (no real metric evaluation loop exists to decide not-to-scale), so there is nothing to surface regardless of the flag's value. Verified vacuous, not a fabricated stub -- generating fake not-scaled events would be worse than reporting none. Re-confirmed this pass (gopherstack-cdxe): implementing this honestly would require a real metric-evaluation loop against real CloudWatch data, out of scope.
  - GetPredictiveScalingForecast returns zero data points for CapacityForecast/LoadForecast rather than any real forecasting simulation (DOWNGRADED this pass from a fabricated flat 10.0-per-hour curve -- see the op table entry). Producing a genuine forecast would require an actual ML/statistical model over real historical CloudWatch metric data gopherstack does not have; honest-empty is the correct terminal state here, not a stopgap.
  - PolicyType/ScalableDimension/ServiceNamespace enum values are accepted permissively (no allowlist validation) rather than validated against the real AWS enum lists. Consistent with this codebase's general emulator philosophy of not over-validating; not treated as a bug. Re-confirmed this pass (gopherstack-cdxe) against that stated philosophy -- no change made.
  - DISCLOSED, NOT FIXED (2026-08-20 sweep): DescribeScalableTargets' scalableTargetSummary wire struct (handler_scalable_targets.go) emits `Tags` and `LastModifiedTime` fields that do not exist on the real SDK's `types.ScalableTarget` (confirmed by reading the full struct in the pinned v1.45.4 types.go -- it has exactly CreationTime/MaxCapacity/MinCapacity/ResourceId/RoleARN/ScalableDimension/ServiceNamespace/PredictedCapacity/ScalableTargetARN/SuspendedState, no Tags, no LastModifiedTime). Same pattern on DescribeScheduledActions' scheduledActionSummary: it emits `LastModifiedTime`, which `types.ScheduledAction` also does not have. Both are real backend state (not fabricated values), and a real aws-sdk-go-v2 client's JSON unmarshal into the typed SDK struct silently ignores unrecognized keys -- so unlike the GetPredictiveScalingForecast bug this pass fixed, these do not break a real client and are not one of the five wire-breaking bug shapes (missing member, wrong nesting, wrong type, case mismatch, wrong value/invented enum). Left as-is rather than manufacturing a fix for a non-breaking, additive deviation; flagged here for visibility if a future pass wants strict shape purism.
deferred:
  - Full CloudWatch cross-service integration for scaling-policy alarms: real AWS creates genuine backing CloudWatch alarms (visible via cloudwatch:DescribeAlarms) and can fail PutScalingPolicy with FailedResourceAccessException if the scalable target's RoleARN lacks CloudWatch permissions. gopherstack's cloudwatch service does have a real backend (services/cloudwatch, with a working PutMetricAlarm), and other services (e.g. cloudformation) do wire a cross-service reference to it. CORRECTED (gopherstack-osg7): that wiring is NOT set up at CLI backend-provider init time in cli.go -- cloudformation's own provider.Init (services/cloudformation/provider.go) type-asserts ctx.Config to its own BackendsProvider interface and calls bp.GetCloudWatchHandler() itself; cli.go only builds the AppContext and hands *CLI in as Config, it never calls GetCloudWatchHandler. The accessor this service would need, GetCloudWatchHandler, already exists on *CLI (cli.go:1133), and the general mechanism (a backend stashing ctx.Config in its own provider.Init via SetAppConfig, then type-asserting it to a narrow sibling interface) is the pattern documented on pkgs/service/service.go's AppContext and already used by seven services (codedeploy/ec2/grafana/mgn/resiliencehub/guardduty/appconfig). A prior pass instead synthesized stable-looking Alarm name+ARN entries on the Application Auto Scaling side pointing at a CloudWatch alarm that doesn't exist; that fabrication was removed this pass (gopherstack-cdxe) in favor of an honestly-empty Alarms field (see PutScalingPolicy), which remains the right call either way. Real cross-service alarm creation is available to a future pass via this service's own provider.Init -- not blocked on cli.go changes -- but whether to add it is a separate decision, not made here.
  - ConcurrentUpdateException/FailedResourceAccessException: sentinels (ErrConcurrentUpdate/ErrFailedResourceAccess) and correct HTTP statuses exist in errors.go/handler.go, but no backend method returns either -- gopherstack's backend serializes every operation behind one coarse lockmetrics.RWMutex (no update-race window) and has no cross-service CloudWatch permission check (see the deferred alarm-integration item above), so neither has a non-fabricated backend-state trigger. ALREADY COVERED BY CHAOS (verified this pass, gopherstack-cdxe): `pkgs/chaos.Middleware` (wired globally via `registry.Use(chaos.Middleware(faultStore))` in cli.go) sits in front of every service's handler and matches purely on the request's SigV4 service name ("application-autoscaling") + X-Amz-Target operation + region -- it never inspects backend state, so a fault rule such as `{"service":"application-autoscaling","error":{"code":"ConcurrentUpdateException","statusCode":500}}` deterministically returns that exact error to a real aws-sdk-go-v2 client on any operation, with zero code changes needed in this service. This is the same generic mechanism proven end-to-end against a real containerized client in test/integration/chaos_test.go. Wiring a fabricated in-backend trigger for either exception would be redundant with, and strictly worse than, this existing mechanism.
leaks: {status: clean, note: "no goroutines/janitors in this service; all state is synchronous map/slice access under lockmetrics.RWMutex"}
---

## Notes

- Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: AnyScaleFrontendService.<Op>`.
  Verified the `AnyScaleFrontendService.` prefix against the real SDK's `serializers.go`
  (every op serializer sets exactly this header value) -- matcher in handler.go is correct.

- **HTTP status is NOT resource semantics, it's ErrorFault() (this pass's main bug class)**:
  awsjson1.1/json-protocol services do not use HTTP 404 for "not found" the way REST
  protocols do. Every modeled exception's HTTP status is determined entirely by whether the
  AWS Smithy model classifies it as a client fault (400) or a server fault (500) --
  `ErrorFault()` on each type in `aws-sdk-go-v2/service/applicationautoscaling/types/errors.go`.
  `ObjectNotFoundException`/`ResourceNotFoundException` are BOTH client faults (400) despite
  signaling "not found"; only `ConcurrentUpdateException`/`InternalServiceException` are
  server faults (500). gopherstack previously returned HTTP 404 for ObjectNotFoundException
  and HTTP 409 for the (unreachable) ValidationException/ErrAlreadyExists case -- both wrong.
  A real aws-sdk-go-v2 client doesn't care about the HTTP status for *type resolution* (it
  reads `__type`/`X-Amzn-ErrorType` from the body/header), but does care for retry
  classification (5xx is retryable, 4xx generally is not) -- so the old 404/409 codes could
  cause a real SDK to retry a request that would never succeed, or vice versa.

- **Per-operation modeled error sets are NOT interchangeable (second major bug class)**:
  field-diffing each op's `awsAwsjson11_deserializeOpError<Op>` switch in the vendored SDK's
  `deserializers.go` revealed gopherstack had reused one exception type across ops that model
  different ones for the same "not found" or "too many X" concept:
  - `ListTagsForResource`/`TagResource`/`UntagResource` model **ResourceNotFoundException**,
    never ObjectNotFoundException.
  - `TagResource` models **TooManyTagsException** for its own over-limit case; but
    `RegisterScalableTarget` (which also accepts a `Tags` parameter) has NO
    TooManyTagsException in its modeled set -- its over-limit case must be
    **LimitExceededException** instead.
  - `GetPredictiveScalingForecast`'s modeled set is `{InternalServiceException,
    ValidationException}` only -- no ObjectNotFoundException, unlike every other
    policy/target-keyed op.
  - `PutScalingPolicy`/`PutScheduledAction` (but not `DescribeScalingPolicies` etc.) DO model
    ObjectNotFoundException, because real AWS requires the scalable target to already exist
    for these two ops specifically (`ObjectNotFoundException`'s own doc text: "For any
    operation that depends on the existence of a scalable target, this exception is thrown
    if the scalable target ... does not exist").
  `TooManyTagsException`/`ResourceNotFoundException` both carry a `ResourceName` field on the
  wire (confirmed via their `deserializeDocument*` functions) -- handler.go's
  `marshalResourceError` now emits it; the plain `marshalError` helper is used for exception
  types with no such field.

- **PutScalingPolicy/PutScheduledAction now enforce pre-registration**: previously either op
  could be called against a `(serviceNamespace, resourceId, scalableDimension)` that was
  never passed to `RegisterScalableTarget`, silently creating an orphaned policy/action --
  real AWS rejects this with ObjectNotFoundException, a well-known Terraform gotcha
  (`aws_appautoscaling_policy`/`aws_appautoscaling_scheduled_action` need
  `depends_on = [aws_appautoscaling_target...]` when not otherwise implied). Fixed in
  `scaling_policies.go`/`scheduled_actions.go` via `scalableTargetExists` (added to
  `scalable_targets.go`). This required adding a `RegisterScalableTarget` precondition to
  every existing test that exercises PutScalingPolicy/PutScheduledAction directly (dozens of
  call sites across `handler_scaling_policies_test.go`, `handler_scheduled_actions_test.go`,
  `handler_forecast_test.go`, `pagination_test.go`, `persistence_test.go`) -- see
  `seedTargetNS` in `handler_test.go`.

- **Real, documented AWS quotas now enforced** (source: "Quotas for Application Auto Scaling"
  AWS documentation page): 50 scaling policies per scalable target (not adjustable), 200
  scheduled actions per scalable target (not adjustable), 20 step adjustments per step
  scaling policy (adjustable, but 20 is the real default and gopherstack has no
  per-account-quota-override concept), and (added gopherstack-cdxe) the per-resource-type
  "scalable targets per account/region" quota on `RegisterScalableTarget` -- 5,000 for
  dynamodb, 3,000 for ecs, 1,500 for cassandra (Keyspaces), 500 for every other
  ServiceNamespace (`maxScalableTargetsForNamespace` in `scalable_targets.go`). All four
  raise `LimitExceededException`.

- **NextToken is now opaque** (`encodePageToken`/`decodePageToken` in `store.go`, base64 of
  the same sort-key cursor `paginate()` already used): previously the "opaque" NextToken was
  literally the raw sort key (a resource ID, ARN, or composite string) -- syntactically any
  string was a "valid" cursor, so there was no way to detect a malformed NextToken and
  gopherstack could never return InvalidNextTokenException, which all four Describe* ops
  model. `paginate()` now returns `(page, nextToken, error)`; a token that fails to
  base64-decode returns ErrInvalidNextToken. This does not change any client-visible
  behavior for well-behaved clients (they only ever pass back a token gopherstack itself
  issued), only for a client that fabricates or corrupts a token.

- **PredictiveScalingPolicyConfiguration was a real field never wired** (prior pass):
  it wasn't in gopherstack's input/output structs AT ALL, despite being a real field on both
  `PutScalingPolicyInput` and `types.ScalingPolicy` in the vendored SDK -- a client creating
  a PredictiveScaling policy with a real config had that config silently discarded. Fixed:
  see the `ScalingPolicy.PredictiveScalingConfig` field in `models.go`.

- **Alarms is honestly empty, not fabricated (gopherstack-cdxe, this pass)**:
  `scalingPolicySummary.Alarms` is a real field on both PutScalingPolicy's and
  DescribeScalingPolicies' response shapes. A prior pass populated it with a `synthesizeAlarms`
  helper that invented stable-looking Alarm name+ARN entries (2 for TargetTrackingScaling
  unless DisableScaleIn, 1 for StepScaling, 0 for PredictiveScaling) built with
  `arn.Build("cloudwatch", ...)`. Those ARNs never corresponded to any real CloudWatch alarm
  -- gopherstack's applicationautoscaling backend has no cross-service reference to the
  cloudwatch service's backend (that wiring pattern exists for other services, e.g.
  cloudformation's `bp.GetCloudWatchHandler()` -- CORRECTED gopherstack-osg7: wired from
  cloudformation's OWN provider.Init, not from cli.go; this service's own provider.Init
  could do the same via the SetAppConfig/siblingServices pattern documented on
  pkgs/service/service.go's AppContext, and *CLI already exposes GetCloudWatchHandler --
  not wired this pass, a separate decision from whether it's possible), so a caller that
  took the ARN at face value and called cloudwatch:DescribeAlarms would find nothing. This is the same fabrication
  class as the GetPredictiveScalingForecast flat-curve bug below, so `synthesizeAlarms` was
  deleted and `ScalingPolicy.Alarms` is now always nil (the wire field is `omitempty`, so it
  is simply absent from the response) for every policy type. DOWNGRADE, taken deliberately:
  an empty field is honest; a plausible-looking pointer to nothing is not.

- **`ScalableTarget.PredictedCapacity`/`ScalingActivity.Details`/`ScalingActivity.NotScaledReasons`**:
  three more real wire fields gopherstack's structs didn't declare at all. Added for field
  presence parity; all three are honestly left nil/empty (never fabricated) since nothing in
  gopherstack computes them -- see each field's doc comment in `models.go`.

- **Deleted invented field**: `DescribeScalingPoliciesFilter.PolicyARNs` /
  `describeScalingPoliciesInput.PolicyARNs` did not exist on the real
  `DescribeScalingPoliciesInput` (confirmed against both the Go SDK struct and its
  serializer in `serializers.go` -- the only filters are `PolicyNames`/`ResourceId`/
  `ScalableDimension`/`ServiceNamespace`). Removed from `scaling_policies.go`,
  `handler_scaling_policies.go`, and the now-deleted
  `TestHandler_DescribeScalingPolicies_PolicyARNsFilter` test.

- **ARN colon-vs-slash quirk** (prior pass, unchanged): real Application Auto Scaling ARNs
  for scaling policies and scheduled actions place the trailing `policyName/{name}` or
  `scheduledActionName/{name}` segment after a **colon**, not a slash, e.g.:
  `arn:aws:autoscaling:us-east-2:123456789012:scalingPolicy:{uuid}:resource/ecs/service/my-cluster/my-service:policyName/MyPolicy`

- **PolicyType default quirk** (prior pass, unchanged): real AWS/Terraform
  (`aws_appautoscaling_policy`) defaults an omitted `PolicyType` to `StepScaling`, not
  `TargetTrackingScaling`.

- Upsert semantics verified for `RegisterScalableTarget`, `PutScalingPolicy`, and
  `PutScheduledAction` (all check their secondary index and mutate in place on a second call
  with the same key). `DeregisterScalableTarget`'s cascade-delete of policies and scheduled
  actions for the same resource matches real AWS ("If a scalable target is deregistered ...
  any scaling policies that were specified for the scalable target are deleted").

- `ErrAlreadyExists` remains declared and wired into handleError (HTTP 400, fixed from 409)
  but no backend method returns it -- every Put*/Register* op is upsert-only by design,
  matching real AWS semantics (there is no create-only path that could conflict).

## 2026-08-20 wrapper-key/nested-shape sweep

- **`ResourceLabel` cross-service twin check (`gopherstack-41di`)**: the sibling `autoscaling`
  service's `PutScalingPolicy` was reported to drop `ResourceLabel` from
  `PredefinedMetricSpecification` in its target-tracking config. That bug does NOT exist here.
  Confirmed by reading the pinned SDK's `types.PredefinedMetricSpecification`
  (`types/types.go`: `PredefinedMetricType MetricType` required, `ResourceLabel *string`
  optional) and gopherstack's own wire path: `applicationautoscaling` stores
  `TargetTrackingScalingPolicyConfiguration`/`StepScalingPolicyConfiguration`/
  `PredictiveScalingPolicyConfiguration` as opaque `map[string]any` passthroughs end to end
  (`putScalingPolicyInput` in handler_scaling_policies.go decodes the raw JSON body directly
  via `encoding/json.Unmarshal` into a `map[string]any` field -- `pkgs/service/jsondisp.go`'s
  `HandleJSON`/`WrapOp` do no field allowlisting), and `DescribeScalingPolicies` echoes the
  same map back verbatim (`scalingPolicySummary.TargetTrackingScalingPolicyConfiguration =
  p.TargetTrackingConfig`). There is no named Go field for `ResourceLabel` (or any other
  nested member) to omit it from -- every key the caller sends, at any nesting depth, survives
  the round trip untouched. This is architecturally different from `autoscaling`'s bug (a
  typed struct that named some fields and not others); the twin does not apply.

- **The two policy-config types are fully independent, verified against their own
  deserializers**: `TargetTrackingScalingPolicyConfiguration` and
  `StepScalingPolicyConfiguration` are separate top-level keys in `PutScalingPolicyInput`
  (`api_op_PutScalingPolicy.go`) and separate `map[string]any` fields in gopherstack's
  `ScalingPolicy`/`putScalingPolicyInput`/`scalingPolicySummary` (never merged or aliased), so
  they cannot cross-contaminate. `PredictiveScalingPolicyConfiguration` is a third, equally
  independent key/field. Confirmed against `PutScalingPolicyInput`'s three separate optional
  members and `ScalingPolicy`'s (types.go) three separate optional members.

- **Real bug found and fixed**: `GetPredictiveScalingForecast`'s `LoadForecast[].
  MetricSpecification` was a fabricated STRING (`fmt.Sprintf("%s/%s/%s", serviceNamespace,
  resourceID, scalableDimension)`) where the real wire shape
  (`types.LoadForecast.MetricSpecification`, `types/types.go`) is a required OBJECT
  (`*types.PredictiveScalingMetricSpecification`). A real aws-sdk-go-v2 client's JSON
  unmarshal into that struct field fails outright on a string value
  (`unexpected JSON type ecs/service/...`), breaking the ENTIRE
  `GetPredictiveScalingForecast` call -- proven with a new SDK round-trip test
  (`wire_sdk_roundtrip_test.go`) that reproduced this exact deserialization error against the
  pre-fix code and passes after. See the op table entry for the full fix description. This is
  shape (c) from the sweep's bug taxonomy (wrong JSON type, fails the entire call).

- **Four Describe ops' four item types, verified separately against the pinned SDK**:
  `ScalableTarget` (10 real fields), `ScalingActivity` (12 real fields), `ScheduledAction`
  (11 real fields), and `ScalingPolicy` (config-map passthrough, not field-enumerated) were
  each diffed member-by-member against their own struct in `types/types.go` -- no missing
  members found on any of the four. Two extra (non-fabricated, non-breaking) fields were
  found and disclosed rather than fixed; see gaps.

- **Enums checked both directions**: `PolicyType` (gopherstack's `isValidPolicyType` allows
  exactly the SDK's 3 values: StepScaling/TargetTrackingScaling/PredictiveScaling -- no
  invented 4th value). `ScalingActivityStatusCode` (gopherstack only ever emits
  `"Successful"`, one of the SDK's 6 real values -- no invented value). `ServiceNamespace`
  (the per-namespace scalable-target quota switch in `scalable_targets.go` matches exactly 3
  of the SDK's 15 real values -- dynamodb/ecs/cassandra -- with every other real value falling
  through to the documented default case; no invented namespace string). `AdjustmentType`,
  `MetricAggregationType`, `MetricStatistic`, `MetricType`, `ScalableDimension` are accepted
  permissively (passthrough, no gopherstack-side enum encoding to check) -- pre-existing,
  documented gap, not new.

- **Protocol reconfirmed**: awsjson1.1 (`AnyScaleFrontendService.<Op>` X-Amz-Target), all 14
  ops have `awsAwsjson11_deserializeOpDocument<Op>Output` both defined and called (count=2 in
  deserializers.go) -- the cnhp restjson dead-code trap does not apply to this JSON-RPC
  service.

- **Provenance verdict**: the prior stamp (`last_audit_commit: 2d47b51d4`,
  `last_audit_date: 2026-07-29`) checks out -- `git show -s --format=%ad 2d47b51d4` returns
  `Wed Jul 29 22:13:36 2026 -0500`, the same day as the recorded audit date. No gap, no false
  provenance. Refreshed this pass to the current HEAD (`bf7f0944b`) and today (2026-08-20).
