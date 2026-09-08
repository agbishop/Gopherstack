---
service: scheduler
sdk_module: aws-sdk-go-v2/service/scheduler@v1.20.4   # version audited against
last_audit_commit: 615cda74e                           # HEAD when this audit pass started
last_audit_date: 2026-08-20
overall: A            # genuine wire-breaking and next-invocation-computation bugs found and fixed (see Notes)
ops:
  CreateSchedule:      {wire: fixed, errors: ok, state: fixed, persist: ok, note: "Target.EcsParameters wire bugs fixed (see 2026-08-20 Notes); ClientToken now idempotent (see Notes); ScheduleExpressionTimezone now validated as a real IANA name; ScheduleExpression now semantically validated (rate/cron/at), not just structurally; cron field values (ranges/names/wildcards) now validated per-field, see 2026-08-11 gopherstack-cz9e Notes"}
  GetSchedule:         {wire: fixed, errors: ok, state: ok, persist: ok, note: "Target.EcsParameters wire bugs fixed (see 2026-08-20 Notes); invented non-canonical Tags field deleted; invented non-canonical Tags field deleted; 2026-08-21 gopherstack-r80d batch 32: EcsParameters.NetworkConfiguration.AwsvpcConfiguration and CapacityProviderStrategyItem's members were wrong-cased wire keys, invisible to any real client; AwsVpcConfiguration.Subnets (required) was also tagged omitempty despite being reachably empty -- see Notes"}
  UpdateSchedule:      {wire: fixed, errors: ok, state: fixed, persist: ok, note: "Target.EcsParameters wire bugs fixed (see 2026-08-20 Notes); ScheduleExpressionTimezone now validated as a real IANA name (prior pass's State-omission fix re-verified still correct, see Notes); ScheduleExpression now semantically validated; cron field values now validated per-field, see 2026-08-11 gopherstack-cz9e Notes"}
  DeleteSchedule:      {wire: ok, errors: ok, state: ok, persist: ok}
  ListSchedules:       {wire: fixed, errors: ok, state: ok, persist: ok, note: "invented Target.RoleArn field deleted (real TargetSummary has only Arn)"}
  CreateScheduleGroup: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "ClientToken now idempotent (see Notes); FIXED 2026-09-06 (bd gopherstack-ui6k): accepted a fabricated Description request field not present in the real CreateScheduleGroupInput -- see Notes"}
  GetScheduleGroup:    {wire: fixed, errors: ok, state: ok, persist: ok, note: "invented non-canonical Tags field deleted; FIXED 2026-09-06 (bd gopherstack-ui6k): also echoed the same fabricated Description back -- see Notes"}
  DeleteScheduleGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-delete-all-schedules-in-group re-verified correct against the real API doc comment; async DELETING intermediate state intentionally not modeled, see Notes"}
  ListScheduleGroups:  {wire: fixed, errors: ok, state: ok, persist: ok, note: "invented non-canonical Tags field deleted"}
  TagResource:         {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource:       {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  RouteMatcher: {status: ok, note: "re-verified every op's REST method+path prefix against aws-sdk-go-v2 serializers.go this pass -- no drift; see prior pass's per-op mapping in Notes."}
  next-invocation computation: {status: fixed, note: "at() one-time expressions were validated at Create/Update time but the runner's isDue only matched rate()/cron() prefixes -- an at() schedule could NEVER fire. ScheduleExpressionTimezone was stored/round-tripped on the wire but never applied when evaluating cron/at wall-clock matches (runner always used the poll goroutine's raw time.Time, i.e. implicitly UTC/server-local). StartDate/EndDate were stored/round-tripped but the runner never gated cron/rate firing on them. All three fixed this pass -- see Notes."}
  cross-service target delivery: {status: ok, note: "cli.go's wireSchedulerRunner wires ALL 8 Runner invoker interfaces (Lambda, SQS, SNS, StepFunctions, EventBridge, Kinesis, SageMaker, ECS); unchanged this pass, re-confirmed not a gap."}
gaps:
  - {area: "cron L/W/# matching", note: "validateCronFields (2026-08-11, gopherstack-cz9e) accepts AWS-documented L/W/# cron tokens (last day, nearest-weekday, nth-weekday-of-month), plus the undocumented-but-plausible LW and L-<n> composite forms (see Notes), as syntactically legal, but matchesCronPart (schedule_expression.go) does not implement any of their matching semantics -- a schedule using e.g. cron(15 10 ? * 6L 2022-2023) or cron(30 23 L-2 * ? *) is accepted at Create/Update and then never fires. Deliberately left accepting rather than rejecting per this pass's under-enforcement directive (AWS genuinely accepts at least the documented subset of this syntax, and neither AWS source rules out the rest); implementing the matcher is separate follow-up work."}
deferred: []
leaks: {status: clean, note: "leak_main_test.go (testleak.VerifyTestMain) passes under -race. The runner's poll goroutine remains the only background goroutine (ctx-parented via Handler.StartWorker/Shutdown, unchanged this pass). New state added this pass (Runner.locCache, Handler.idempotency) is plain in-memory data with no goroutines/tickers of its own; both are swept/bounded (locCache via the existing per-poll sweep alongside cronCache; idempotency via TTL-based lazy eviction) and cleared on Handler.Reset."}
---

## Notes (2026-08-21 pass, gopherstack-r80d batch 32)

Part of the mgn/redshiftdata/scheduler batch testing r80d's op-count-vs-
field-count hypothesis (see `services/_REQUIRED_OUTPUT_CANDIDATES.md`).
scheduler tied at 5 required output fields (12 ops); flat scan alone is
clean (CreateSchedule/CreateScheduleGroup/UpdateSchedule's `ScheduleArn`/
`ScheduleGroupArn`, ListScheduleGroups/ListSchedules's `ScheduleGroups`/
`Schedules` are all always emitted). The bugs are both below the flat scan,
in `GetScheduleOutput.Target` (itself not required, so invisible to the
per-op ranking) wrapping `types.Target.EcsParameters.NetworkConfiguration.
AwsvpcConfiguration` and `.CapacityProviderStrategy`.

1. **Wrong wire key entirely, not a dropped value.**
   scheduler@v1.20.4's `deserializers.go` (`awsRestjson1_deserializeDocumentNetworkConfiguration`)
   switches on `"awsvpcConfiguration"` (lowercase first rune);
   `awsRestjson1_deserializeDocumentCapacityProviderStrategyItem` switches on
   `"capacityProvider"`/`"base"`/`"weight"` (same). gopherstack's
   `scheduleTargetEcsNetworkConfiguration.AwsvpcConfiguration` and
   `scheduleTargetEcsCapacityProviderStrategyItem`'s three members were tagged
   with the capitalized Go-field-name spelling instead
   (`"AwsvpcConfiguration"`/`"CapacityProvider"`/`"Base"`/`"Weight"`). A real
   SDK client's response deserializer does an exact-case `switch`, so every
   one of these wrong-cased keys fell into the `default: _, _ = key, value`
   no-op branch on every decode -- the entire `AwsvpcConfiguration` object,
   and `CapacityProviderStrategyItem`'s required `CapacityProvider` member
   (`*string`, provable) along with it, were invisible to any real client on
   `GetSchedule`, independent of value. (`PlacementConstraint.Expression`/
   `.Type` and `PlacementStrategy.Field`/`.Type` carry the same lowercase-first
   wire keys and had the same capitalized-tag mistake; fixed alongside since
   it's the same mechanical error in the same ECS-target struct family, but
   neither member is Smithy-required so this half is disclosed cleanup, not a
   counted bug.) Fixed by re-tagging all four structs to the real
   lowercase-first keys.
2. **`AwsVpcConfiguration.Subnets` (required, `[]string`) tagged `omitempty`
   despite being reachably empty.** The real client-side validator
   (`validators.go`'s `validateAwsVpcConfiguration`) only null-checks it
   (`if v.Subnets == nil`), never length-checks, and the request serializer
   (`awsRestjson1_serializeDocumentAwsVpcConfiguration`, `if v.Subnets !=
   nil`) will send a non-nil empty slice on the wire -- so a real client may
   legally construct `Subnets: []string{}`. gopherstack validates
   `EcsParameters.TaskDefinitionArn` and every other reachable-empty-string
   candidate on this service's `Target` (all correctly disqualified: gopherstack's
   own `validateTarget` rejects an empty `TaskDefinitionArn`/`PartitionKey`/
   `DetailType`/`Source` before storage, stricter than AWS's null-only check),
   but never validated `Subnets`'s length. Fixed by dropping `omitempty` and
   normalizing a nil backend value to `[]string{}` in `ecsNetworkConfigToOutput`
   (matching this campaign's "required-but-inapplicable means present-and-empty,
   not absent" convention) rather than counting on the (also-fixed) wire-key
   bug to mask it.

Both proven via real `aws-sdk-go-v2/service/scheduler` client round trips
(`services/scheduler/wire_output_required_r80d_test.go`, 2 test funcs),
hand-reverted (`handler_schedules.go` restored via `git show
HEAD:services/scheduler/handler_schedules.go`, confirmed both tests fail
against the pre-fix file)/confirmed-failing/restored, md5sum-verified
byte-identical. All of `scheduler`'s pre-existing tests (including 3 raw-JSON
round-trip tests that already exercised these same nested types with
non-empty values, e.g. `TestCreateSchedule_EcsParametersNetworkConfigurationRoundtrip`)
continued to pass unchanged -- they drive the handler directly with
gopherstack's own (previously wrong) capitalized keys rather than a real SDK
client, which is why this class survived them.

Companion services this batch (see `services/_REQUIRED_OUTPUT_CANDIDATES.md`
for full detail): `mgn` (95 ops, the batch's primary hypothesis test) and
`redshiftdata` (12 ops) both came back clean -- 0 bugs each, unlike
scheduler's 2.
## Notes (2026-08-20 pass, wrapper-key/nested-shape sweep)

Full wire sweep of all 12 ops against the pinned SDK
(`aws-sdk-go-v2/service/scheduler@v1.20.4`, restjson1, confirmed via
`serializers.go`'s `awsRestjson1_serializeOp*` prefix on every op -- no
`X-Amz-Target`-only or CBOR protocol involved). No cnhp trap: every op's Output
struct has no `httpPayload`-tagged single member, so `deserializeOpDocument*Output`
is live/body-flat for every op (confirmed: `DeleteSchedule`/`DeleteScheduleGroup`/
`TagResource`/`UntagResource` have 0 occurrences of the helper because their
Outputs carry no body fields at all, not because the helper is dead).

**Bugs found and fixed, all inside `Target.EcsParameters` (bug class (a)/(c)/(d)
per this campaign's taxonomy) -- the deepest-nested, least-tested corner of the
six mutually-exclusive Target parameter blocks:**

- **(d) Case-sensitive key near-miss: `NetworkConfiguration.AwsvpcConfiguration`
  wrapper key was `"AwsvpcConfiguration"` (PascalCase); real SDK wraps it under
  a lower-camel `"awsvpcConfiguration"` key
  (`serializers.go:awsRestjson1_serializeDocumentNetworkConfiguration`,
  `object.Key("awsvpcConfiguration")`; confirmed same-case on the deserialize
  side, `deserializers.go:awsRestjson1_deserializeDocumentNetworkConfiguration`,
  `case "awsvpcConfiguration":`). Real SDK's generated deserializer switches on
  the exact-case key with a silent `default:` no-op, so this doesn't error --
  it drops the whole `AwsvpcConfiguration` (Subnets/SecurityGroups/AssignPublicIp)
  block to nil on every real client parsing a gopherstack response. Fixed:
  `scheduleTargetEcsNetworkConfiguration.AwsvpcConfiguration` json tag
  (handler_schedules.go).
- **(d) Case-sensitive key near-misses:
  `CapacityProviderStrategyItem`'s three fields were `"CapacityProvider"`/
  `"Base"`/`"Weight"` (PascalCase); real SDK uses `"capacityProvider"`/`"base"`/
  `"weight"` (`serializers.go:awsRestjson1_serializeDocumentCapacityProviderStrategyItem`;
  `deserializers.go:awsRestjson1_deserializeDocumentCapacityProviderStrategyItem`,
  `case "base"`/`"capacityProvider"`/`"weight":`). Fixed:
  `scheduleTargetEcsCapacityProviderStrategyItem` json tags (handler_schedules.go).
- **(d) Case-sensitive key near-misses: `PlacementConstraint`'s `Expression`/
  `Type` and `PlacementStrategy`'s `Field`/`Type` were PascalCase; real SDK uses
  lower-camel `"expression"`/`"type"`/`"field"`/`"type"` for both shapes
  (`serializers.go:awsRestjson1_serializeDocumentPlacementConstraint`,
  `awsRestjson1_serializeDocumentPlacementStrategy`;
  `deserializers.go:awsRestjson1_deserializeDocumentPlacementConstraint`,
  `awsRestjson1_deserializeDocumentPlacementStrategy`). Fixed:
  `scheduleTargetEcsPlacementConstraint`/`scheduleTargetEcsPlacementStrategy`
  json tags (handler_schedules.go). Note: `AwsVpcConfiguration`'s own three
  fields (`AssignPublicIp`/`SecurityGroups`/`Subnets`) and every other
  `EcsParameters` top-level key (`NetworkConfiguration`, `PlacementConstraints`,
  `PlacementStrategy`, `Tags`, `CapacityProviderStrategy`, `TaskDefinitionArn`,
  `LaunchType`, `PlatformVersion`, `Group`, `ReferenceId`, `TaskCount`,
  `EnableECSManagedTags`, `EnableExecuteCommand`) ARE PascalCase and were
  already correct -- the lower-camel casing is a narrow exception confined to
  these three nested shapes plus the `awsvpcConfiguration` wrapper key, not a
  service-wide pattern.
- **(c) Wrong JSON type: `EcsParameters.Tags` was `[]{Key,Value}` (a list of
  keyed objects, e.g. `[{"Key":"env","Value":"prod"}]`); real SDK's field is
  `[]map[string]string` -- a list of free-form single-entry maps, e.g.
  `[{"env":"prod"}]`, serialized by iterating the Go map's own keys as JSON
  object keys with no `Key`/`Value` wrapper at all
  (`types/types.go:138`, `EcsParameters.Tags []map[string]string`;
  `serializers.go:awsRestjson1_serializeDocumentTags` +
  `awsRestjson1_serializeDocumentTagMap`, `deserializers.go`'s mirror pair).
  This is the shape a **prior audit pass (2026-07-24 Notes, "Looks wrong but is
  correct" section) explicitly misdiagnosed**: it correctly wrote down the real
  type as `[]map[string]string` but concluded the existing `{Key,Value}` list
  "remains correct" -- a self-contradiction that locked the bug in as
  documented-intentional. Fixed: `EcsParameters.Tags` is now `[]map[string]string`
  end to end (models.go's backend `Target.EcsParameters.Tags`,
  handler_schedules.go's `scheduleTargetEcsParameters.Tags`); the now-unneeded
  `EcsTag`/`scheduleTargetEcsTag` conversion types and
  `ecsTagsFromInput`/`ecsTagsToOutput` helpers deleted.
- **(a) Fabricated member: `Target.InputTransformer`.** Not a real Scheduler
  `Target` field at all -- `aws-sdk-go-v2/service/scheduler/types/types.go`'s
  `Target` struct (line 377) has exactly `Arn`, `RoleArn`, `DeadLetterConfig`,
  `EcsParameters`, `EventBridgeParameters`, `Input`, `KinesisParameters`,
  `RetryPolicy`, `SageMakerPipelineParameters`, `SqsParameters` -- confirmed
  against both `awsRestjson1_serializeDocumentTarget` and
  `awsRestjson1_deserializeDocumentTarget`'s exhaustive key switches, neither of
  which has an `InputTransformer` case. `InputTransformer` is an EventBridge
  *Rules* target concept, not a Scheduler one -- classic bug class (a),
  generalized in from a different (wider) sibling service. Deleted:
  `InputTransformer` type and `Target.InputTransformer` field (models.go),
  `scheduleTargetInputTransformer` type and both `Target.InputTransformer`
  wire-mirror fields plus `inputTransformerFromInput`/`inputTransformerToOutput`
  (handler_schedules.go).

**Proof**: `services/scheduler/wire_sdk_ecs_target_test.go` (new) drives
CreateSchedule/GetSchedule through the real `aws-sdk-go-v2/service/scheduler`
client (not gopherstack's own JSON tags) and asserts every fixed field survives
the round trip. Hand-reverted `handler_schedules.go`/`models.go` to their
pre-fix content (`cp` from a scratch backup, per this campaign's revert
protocol) and reran: `TestCreateSchedule_EcsParametersSDKRoundTrip` failed with
`awsvpcConfiguration must survive the round trip` (nil), confirming the real
SDK client silently drops the field pre-fix; restored the fix (`cp` back,
`md5sum`-verified identical) and it passes again.

**Existing tests corrected** (wrong-case/wrong-shape literals that happened to
still pass under Go's case-insensitive `encoding/json`, which can't itself
distinguish these bugs -- the SDK round-trip test above is what actually proves
it): `TestCreateSchedule_EcsParametersNetworkConfigurationRoundtrip`,
`TestCreateSchedule_EcsParametersCapacityProviderStrategyRoundtrip`,
`TestCreateSchedule_EcsParametersPlacementConstraintsRoundtrip`,
`TestCreateSchedule_EcsParametersTaskTagsRoundtrip` (all schedules_target_test.go)
updated to the real lower-camel keys / map-shaped Tags.
`TestCreateSchedule_InputTransformerRoundTrip` renamed to
`TestCreateSchedule_InputTransformerNotEchoed` and inverted to assert the field
is dropped, not round-tripped.

**Families re-verified clean, no changes**: `Schedule` vs `ScheduleSummary`
(`GetScheduleOutput` full field list -- `ActionAfterCompletion`, `Arn`,
`CreationDate`, `Description`, `EndDate`, `FlexibleTimeWindow`, `GroupName`,
`KmsKeyArn`, `LastModificationDate`, `Name`, `ScheduleExpression`,
`ScheduleExpressionTimezone`, `StartDate`, `State`, `Target` -- vs
`ScheduleSummary`'s narrower `Arn`, `CreationDate`, `GroupName`,
`LastModificationDate`, `Name`, `State`, `Target *TargetSummary`, matching
`getScheduleOutput`/`scheduleSummary` exactly, including `TargetSummary`'s
`Arn`-only field list matching `scheduleSummaryTarget`); `ScheduleGroup` vs
`ScheduleGroupSummary` (both narrower than `GetScheduleGroupOutput`'s own field
list, matching `getScheduleGroupOutput`/`scheduleGroupSummary` exactly, no
`Tags` field on either side per the 2026-07-24 fix, re-confirmed);
`FlexibleTimeWindow` (`Mode`/`MaximumWindowInMinutes`); all REST HTTP
method+path bindings (`/schedules/{Name}`, `/schedule-groups/{Name}`,
`/tags/{ResourceArn}`, list endpoints) against every op's
`serializeOpHttpBindings*Input`, including the `ScheduleGroup` (not
`GroupName`) query param name on `ListSchedules` and the lowercase
`groupName`/`clientToken` query params on `GetSchedule`/`DeleteSchedule`/
`DeleteScheduleGroup`, all already correct in handler.go; `RetryPolicy`,
`DeadLetterConfig`, `EventBridgeParameters`, `KinesisParameters`,
`SqsParameters`, `SageMakerPipelineParameters`/`SageMakerPipelineParameter`
(all PascalCase, all correct); top-level resource `Tag`/`resourceTag`
(`{Key,Value}`, correctly distinct from the `EcsParameters.Tags` map-list
shape); `ListSchedules`/`ListScheduleGroups`/`ListTagsForResource` wrapper keys
(`Schedules`/`ScheduleGroups`/`Tags`); enums both directions -- every SDK value
(`ActionAfterCompletion`: NONE/DELETE; `AssignPublicIp`: ENABLED/DISABLED;
`FlexibleTimeWindowMode`: OFF/FLEXIBLE; `LaunchType`: EC2/FARGATE/EXTERNAL;
`PlacementConstraintType`: distinctInstance/memberOf;
`PlacementStrategyType`: random/spread/binpack; `PropagateTags`:
TASK_DEFINITION; `ScheduleGroupState`: ACTIVE/DELETING; `ScheduleState`:
ENABLED/DISABLED) representable, and no constant gopherstack emits falls
outside these sets.

**Provenance verdict**: the pre-existing stamp (`last_audit_commit: 174b1f53`,
`last_audit_date: 2026-08-11`) was stale -- `git show -s --format=%ad 174b1f53`
returns `2026-07-12`, a ~30-day gap with the commit predating the claimed audit
date. Refreshed to current HEAD (`615cda74e`) and today's date (2026-08-20).

## Notes (2026-08-11 pass, gopherstack-cz9e)

- **The half gopherstack-8cg7 couldn't reach: cron field *values* were never
  checked.** `parseCronExpression` only checked field count; `matchesCronField`/
  `matchesCronPart` swallowed any token they couldn't parse as "no match" rather
  than erroring, so a structurally valid six-field cron with a garbage field --
  `cron(0 12 * * ? GARBAGE)` -- passed `validateScheduleExpression` and was
  accepted at Create/Update, then never fired: the year field never matched, so
  `matchesCron` was always false. Same invisible-failure shape as 8cg7, but fixing
  it needed new per-field validation logic, not just wiring up existing parsers.
  Fixed: `validateCronFields` (new, `cron_field_validation.go`) walks each of the
  six fields against a `cronFieldSpec` (range, name-alias resolver, legal
  wildcards) and returns `ErrUnknownCronValue` (wrapped as `ValidationException`)
  on anything it can't validate, called from `validateScheduleExpression`'s cron
  branch. Covered by 16 new cases added to
  `TestCreateSchedule_ScheduleExpression_Validation`.
- **Field semantics sourced from AWS's own docs, not memory or Unix/Quartz cron
  conventions** (fetched 2026-08-11,
  https://docs.aws.amazon.com/scheduler/latest/UserGuide/schedule-types.html#cron-based,
  the "Cron-based schedules" field/wildcard table -- the botocore model's
  `CreateScheduleInput.ScheduleExpression` doc, `data/scheduler/2021-06-30/service-2.json.gz`,
  confirms only the six-field split, not per-field detail):

  | Field | Range | Names | Wildcards |
  |---|---|---|---|
  | Minutes | 0-59 | -- | `, - * /` |
  | Hours | 0-23 | -- | `, - * /` |
  | Day-of-month | 1-31 | -- | `, - * ? / L W` |
  | Month | 1-12 | JAN-DEC | `, - * /` |
  | Day-of-week | 1-7 | SUN-SAT | `, - * ? L #` |
  | Year | 1970-2199 | -- | `, - * /` |

  Enforced per field, all cited to that table: numeric range; name aliases (month,
  day-of-week) case-insensitive via the existing `cronMonthValue`/`cronDOWValue`;
  `?` legal only where the table lists it (day-of-month, day-of-week) -- so
  `cron(? 12 * * ? *)` (minutes) and `cron(0 12 ? ? ? *)` (month) are now rejected;
  `/` step legal everywhere **except day-of-week**, per the table's Day-of-week row
  omitting it -- `cron(0 12 ? * 1-5/2 *)` is now rejected; the day-of-month/
  day-of-week cross rule quoted verbatim from the doc's Wildcards bullets: "You
  can't use \* in both the Day-of-month and Day-of-week fields. If you use it in
  one, you must use ? in the other" -- so `cron(0 12 * * * *)` (both `*`) is now
  rejected, `#`'s doc note "If you use a '#' character, you can define only one
  expression in the day-of-week field" -- so `cron(0 12 ? * 3#1,6#3 *)` is now
  rejected.
- **L, W, # accepted structurally, not rejected, per this pass's
  under-enforcement directive.** AWS documents `L` (day-of-month or day-of-week,
  "last day of month/week"), `<n>W` (day-of-month, "nearest weekday to day n"),
  `<n>L` (day-of-week, e.g. `6L` = "last Friday of month", from the doc's own
  example `cron(15 10 ? * 6L 2022-2023)`), and `<n>#<m>` (day-of-week, "mth
  occurrence of weekday n") as legal syntax. `validateCronFields` recognizes and
  accepts well-formed instances of all four in the fields AWS places them. It does
  **not** implement their runtime matching semantics -- `matchesCronPart`
  (schedule_expression.go, pre-existing) has no L/W/# handling, so a schedule
  using one is accepted but still never fires, exactly the pre-existing gap 8cg7's
  notes flagged (not new, not widened). Filed as `gaps` above rather than silently
  left out of PARITY, since it is a real, if narrow, next-invocation gap.
- **Two rules could not be pinned from the fetched AWS text and were left
  accepted rather than guessed**, per "prefer under-enforcing to guessing":
  (1) the `#` instance number's upper bound -- Quartz-style cron typically caps it
  at 5 (a month has at most 5 of any given weekday), but the fetched AWS doc text
  states only that it is "a certain instance," giving no explicit maximum, so no
  upper bound is enforced (`validateCronHashToken` only requires a positive
  integer); (2) `"LW"` (last-weekday-of-month, a common Quartz idiom formed by
  combining `L` and `W`) -- not shown in the fetched AWS examples in either
  form, so `validateCronWToken` accepts it rather than reject on a guess.
- **Follow-up: `cron(30 23 L-2 * ? *)` was initially rejected** (`ValidationException:
  unknown cron value: day-of-month field: "L"`) -- the Quartz `L-<n>` offset idiom
  ("n days before the last day of the month") fell through `validateCronPart`'s
  generic dash-range parser, which tried to parse `"L"` as a range endpoint and
  failed. Re-checked both the EventBridge Scheduler doc (schedule-types.html) and
  the legacy EventBridge cron doc (eb-scheduled-rule-pattern.html, fetched
  2026-08-11) end to end for any mention of an offset-from-`L` form: neither
  confirms nor rules it out -- both document only bare `L`, `<n>W`, `<n>L` (via
  the `6L` example), and `<n>#<m>`. Per "if you cannot establish it either way,
  accept it," added `validateCronLOffsetToken` to recognize `L-<n>` (digits after
  the dash) as a structurally valid token wherever bare `L` is legal --
  day-of-month (the reported case) **and** day-of-week, since neither AWS source
  distinguishes between the two fields for this form and there was no basis to
  accept it in one but not the other. `LW` was already accepted (verified, not
  changed). Lists/ranges containing `L` or `W` as one comma-separated element
  (e.g. `L,15`) already worked correctly before this follow-up, since
  `validateCronField` splits on comma and validates each part independently --
  the bug was specific to the dash-offset shape, not list/comma handling. Not
  extended further: a bare range with `L`/`W` as an endpoint outside the named
  idioms above (e.g. `"3-L"`, `"L-W"`) has no precedent in Quartz, the legacy
  EventBridge cron dialect, or the EventBridge Scheduler docs, so it remains
  rejected -- accepting every string containing `L` or `W` would defeat the
  validator's purpose of catching real typos/garbage. Covered by 5 new cases in
  `TestCreateSchedule_ScheduleExpression_Validation`: `cron_valid_last_day_offset_accepted`
  (the reported case), `cron_valid_last_weekday_of_month_accepted` (`LW`),
  `cron_valid_day_of_week_last_offset_accepted` (`L-1` in day-of-week),
  `cron_valid_list_containing_last_day_accepted` (`L,15`), and
  `cron_last_day_offset_non_numeric_rejected` (`L-abc`, confirms the offset
  digits are still checked, not blanket-accepted).
- **Existing tests: none needed changing.** Grepped every `cron(` literal across
  `services/scheduler/*_test.go` and `test/integration/scheduler_test.go` (the
  every-file inventory, not per-file) before implementing and traced each field of
  each literal through the new rules by hand -- all use standard 6-field AWS forms
  (numeric ranges, `*`, `?`, month/day-of-week names, one `n-m` range, one `*/n`
  step); none hit a newly-enforced rejection. No test needed changing in either
  direction.
- **Snapshot-restore test still passes.** `TestScheduler_Runner_SwallowsPreExistingInvalidExpression`
  (added by 8cg7, unmodified this pass) snapshots a valid schedule, mutates the
  stored expression to `rate(5)` in the raw snapshot bytes, restores, and asserts
  `Restore` still succeeds with the mutated value intact -- `Restore` does not call
  `validateScheduleExpression`/`validateCronFields` at all, so tightening cron
  field validation cannot affect it. Re-ran explicitly: still passes.
- **Runner swallow behaviour: unchanged.** `isDueCron` still returns "not due" on
  a `cachedParseCron` error rather than propagating it; `validateCronFields` is
  reached only from `validateScheduleExpression` (Create/Update), never from the
  runner's poll path, so one schedule with a since-invalidated stored expression
  still cannot stop every other schedule's evaluation.
- **No snapshot version bump.** Persisted `Schedule.ScheduleExpression` is an
  opaque string; this pass only tightens what `CreateSchedule`/`UpdateSchedule`
  accept for *new* writes, and `Restore` (unchanged, see above) still loads
  anything a prior version wrote.

## Notes (2026-08-11 pass, gopherstack-8cg7)

- **`validateScheduleExpression` checked shape only, not semantics -- a schedule
  could be created that would never fire, with zero signal to the caller.** It
  accepted anything matching `rate(...)`/`cron(...)`/`at(...)` with balanced
  parens and (for cron) exactly 6 fields, e.g. `rate(5)` (no unit) or
  `at(2024-01-01)` (no time component). `CreateSchedule`/`UpdateSchedule`
  returned success; the runner's `isDueRate`/`isDueCron`/`isDueAt` then failed to
  parse the same string and swallowed the error as "not due," so the schedule
  silently sat forever. Fixed: `validateScheduleExpression` now calls the same
  `parseRateExpression`/`parseCronExpression`/`parseAtExpression` the runner uses,
  wrapped as `ErrValidation` (`ValidationException`), so a semantically invalid
  expression is rejected at write time on both Create and Update. Covered by new
  cases in `TestCreateSchedule_ScheduleExpression_Validation` and the new
  `TestUpdateSchedule_ScheduleExpression_SemanticValidation`.
- **Boundaries enforced, cited against the pinned SDK
  (`aws-sdk-go-v2/service/scheduler@v1.20.4`, `api_op_CreateSchedule.go:63-64`,
  identical text in botocore `data/scheduler/2021-06-30/service-2.json.gz`'s
  `CreateScheduleInput.ScheduleExpression` doc)**: "A rate expression consists of
  a value as a positive integer, and a unit with the following options: minute |
  minutes | hour | hours | day | days" -- so a zero or negative value is rejected
  (`parseRateExpression` already required `n > 0`) and an unrecognized unit is
  rejected. Cron: "six fields separated by white spaces: (minutes hours
  day_of_month month day_of_week year)" -- confirms the existing 6-field count is
  correct (not the 5-field Unix form). `at`: `at(yyyy-mm-ddThh:mm:ss)`, matching
  the existing `atExpressionLayout`.
- **Known, pre-existing, intentionally-untouched deviation**: `parseRateExpression`
  also accepts a non-standard `second`/`seconds` unit (its own doc comment says
  "for local testing"), which the real API does not. Wiring this same parser into
  `validateScheduleExpression` means `rate(1 second)` is still accepted at
  Create/Update -- this pass did not tighten it further, since dozens of existing
  runner tests rely on `rate(1 second)` schedules to fire within a short test
  window, and narrowing it was not part of this issue's scope. Flagging rather
  than silently leaving it, per this pass's "prefer under-enforcing to guessing"
  directive -- this is a case where the existing looseness is already understood
  and deliberate, not a new gap.
- **Not enforced (deferred, filed as a follow-up)**: cron field *values* are still
  not deeply validated. `parseCronExpression` only checks field count; a
  structurally-valid 6-field cron with a garbage token in one field (e.g.
  `cron(0 12 * * ? GARBAGE)`) passes `validateScheduleExpression` because
  `matchesCronField`/`matchesCronPart` swallow unparseable tokens as "no match"
  rather than erroring, so at runtime that field simply never matches and the
  schedule never fires -- the same invisible-failure bug class as this issue,
  but for cron field content rather than expression shape. Left alone here
  because closing it requires new validation logic (walking each field with the
  same list/range/step/alias grammar `matchesCronField` uses, but propagating
  `ErrUnknownCronValue` instead of swallowing it) that goes beyond "wire in the
  existing parsers," which is what gopherstack-8cg7 scoped. File as a follow-up
  issue.
- **Runner swallow behaviour: kept, not changed to hard-fail.** `isDueRate`/
  `isDueCron`/`isDueAt` still return "not due" on a parse error rather than
  crashing the poll loop -- a background loop iterating every stored schedule
  every second must not let one bad expression take down every other schedule's
  evaluation. This should be unreachable in practice now that Create/UpdateSchedule
  reject invalid expressions, but `Restore` does not re-validate (see below), so a
  schedule persisted before this fix can still carry one. Added `Runner.
  warnInvalidExpression` (runner.go): the first poll that fails to parse a given
  schedule's expression logs one `WARN`, deduped per schedule key via
  `invalidExprWarned` (swept alongside `lastFiredAt` in `checkAndFireSchedules`,
  same lifecycle), instead of either staying silent forever or logging every
  `runnerTickInterval` (1s) indefinitely. Covered by
  `TestScheduler_Runner_SwallowsPreExistingInvalidExpression`.
- **Snapshot/restore does not re-validate `ScheduleExpression`.** `InMemoryBackend.
  Restore` (persistence.go) decodes DTOs straight into the live tables; it never
  calls `validateScheduleExpression`. A snapshot taken before this fix that holds
  an expression like `rate(5)` restores successfully with that value intact --
  restore does not reject data it previously accepted. Such a schedule behaves
  exactly as it did before this pass: it loads, lists, and round-trips normally;
  the runner still evaluates it every poll and still never fires it, now with the
  one-time warning above instead of total silence. Verified directly by
  `TestScheduler_Runner_SwallowsPreExistingInvalidExpression`, which snapshots a
  valid schedule, mutates the expression in the raw snapshot bytes to an invalid
  one (simulating a pre-fix snapshot), restores it, and asserts `Restore` returns
  no error and `GetSchedule` still returns the mutated (invalid) expression
  unchanged.
- **Existing tests already encoding invalid expressions: none found in the sense
  the issue warned about.** Grepped every `rate(`/`cron(`/`at(` literal across
  the test suite; all cron literals use the correct 6-field AWS form and all
  `at()` literals use the correct `yyyy-mm-ddThh:mm:ss` layout. The one
  systematic looseness (`rate(1 second)`, used by ~20 runner tests for fast
  firing) is the pre-existing, documented non-standard-unit allowance discussed
  above, not a value the real API would reject as malformed shape -- it is a real
  AWS-shaped rate expression, just with a unit AWS does not offer. No test
  changes were needed to keep the suite passing after this fix.
- **Brief sweep for the day's other three bug classes (parameter parsed then
  ignored; ID accepted for a nonexistent resource; state mutated before
  validation) found nothing new**: `handler_schedules.go`'s `scheduleInput`/
  `scheduleTarget` fields are all threaded through to the backend (traced every
  field, including nested `EcsParameters`/`SageMakerPipelineParameters`);
  `TagResource`/`UntagResource`/`ListTagsForResource` (tags.go) all reject an
  unknown ARN with `ErrNotFound` before touching state;
  `CreateSchedule`/`UpdateSchedule`/`CreateScheduleGroup`/`DeleteScheduleGroup`
  all validate before acquiring the write lock or mutating a map. Consistent with
  this service's 2026-07-24 A-grade audit; no new findings to file beyond the
  cron-field-value gap above.

## Notes (2026-07-24 pass)

- **`at()` one-time schedules could never fire (the big one).** `validateScheduleExpression`
  has always accepted `at(yyyy-mm-ddThh:mm:ss)` at Create/UpdateSchedule time, but
  `Runner.isDue` only recognized the `rate(` and `cron(` prefixes -- any `at()`
  schedule silently sat forever with zero invocations, a genuine
  next-invocation-computation bug (the parity bar this service is held to). Fixed
  by adding `parseAtExpression` (schedule_expression.go) and `Runner.isDueAt`
  (runner.go): an `at()` schedule fires exactly once, the first poll on/after its
  target instant, and never again (tracked via the existing `lastFiredAt` map, the
  same mechanism used for cron's within-minute dedup). Covered by
  `TestScheduler_Runner_AtExpressionFiresOnceThenNeverAgain`,
  `TestScheduler_Runner_AtExpressionNotYetDue`, `TestScheduler_ParseAtExpression`.
- **`ScheduleExpressionTimezone` was stored and echoed back on the wire but had zero
  effect on runtime firing.** Real AWS evaluates cron and at() expressions'
  wall-clock fields against the schedule's `ScheduleExpressionTimezone` (default
  UTC when unset). gopherstack's runner always matched cron fields against the
  poll goroutine's raw `time.Time` with no timezone conversion, and (per the bug
  above) never evaluated `at()` at all. Fixed: `Runner.cachedLocation` resolves and
  caches the `*time.Location` for a schedule's timezone (mirroring the existing
  `cronCache` pattern, swept the same way in `checkAndFireSchedules`), and both
  `isDueCron` and `isDueAt` convert `now` into that location before matching/
  comparing. Also added `validateTimezone` (schedules.go), called from
  Create/UpdateSchedule, rejecting a `ScheduleExpressionTimezone` that isn't a
  resolvable IANA name with `ValidationException` -- an unresolvable name could
  never be evaluated by the runner anyway (previously it silently fell through to
  UTC with no error). Covered by `TestScheduler_Runner_CronRespectsTimezone`,
  `TestScheduler_Runner_AtExpressionRespectsTimezone`,
  `TestScheduler_Runner_LocCacheEviction`,
  `TestCreateSchedule_ScheduleExpressionTimezone_Validation`,
  `TestUpdateSchedule_ScheduleExpressionTimezone_Validation`.
- **`StartDate`/`EndDate` were stored and echoed back on the wire but had zero effect
  on runtime firing.** Real AWS: "invocations might occur on, or after, the
  StartDate"; "invocations might stop on, or before, the EndDate" for recurring
  (cron/rate) schedules, while one-time (`at()`) schedules explicitly ignore both.
  gopherstack's runner never referenced `s.StartDate`/`s.EndDate` at all -- a
  schedule with an `EndDate` in the past kept firing forever, and one with a future
  `StartDate` fired immediately. Fixed via `withinScheduleWindow` (runner.go),
  called from `isDue` for the `rate(`/`cron(` branches only (matching AWS's
  documented at()-ignores-the-window behavior). Covered by
  `TestScheduler_Runner_StartDateGatesRecurringSchedule`,
  `TestScheduler_Runner_EndDateGatesRecurringSchedule`,
  `TestScheduler_Runner_AtExpressionIgnoresStartAndEndDate`.
- **ClientToken idempotency implemented for CreateSchedule/CreateScheduleGroup.**
  The prior pass documented this as an accepted gap ("no idempotency-token pkg
  exists in pkgs/"); this pass implements a bounded, handler-level cache
  (idempotency.go) instead of a new pkgs/ package or a StorageBackend interface
  change: `Handler.idempotency` (a `safemap.Map[string, idempotentResult]`) caches
  a successful Create's ARN by a composite key of (op kind, group, name,
  ClientToken) for `clientTokenTTL` (5 minutes). A retried Create with the same
  ClientToken replays the cached ARN instead of hitting the backend's
  name-uniqueness check and failing with ConflictException. A *different*
  ClientToken (or none) on a colliding name still conflicts, preserving existing
  semantics. `createScheduleGroupInput` was also missing the `ClientToken` field
  entirely (dead on the wire, silently discarded) -- added. `Handler.Reset` now
  also clears the cache. This is intentionally handler-level, not
  backend/StorageBackend-level: it doesn't change `CreateSchedule`/
  `CreateScheduleGroup`'s widely-referenced (25+ call sites across every test file)
  StorageBackend signature, keeping the fix's blast radius contained to the two
  Create handlers. Covered by `idempotency_test.go`
  (`TestCreateSchedule_ClientToken_ReplaysOnRetry`,
  `TestCreateSchedule_NoClientToken_DuplicateNameStillConflicts`,
  `TestCreateSchedule_DifferentClientToken_DuplicateNameStillConflicts`,
  `TestCreateScheduleGroup_ClientToken_ReplaysOnRetry`,
  `TestSchedulerHandler_Reset_ClearsIdempotencyCache`). Deep AWS
  idempotency-mismatch semantics (rejecting a token reused with genuinely
  different parameters) are intentionally NOT modeled -- out of scope for the
  narrow lost-response-retry case this closes.
- **Invented (non-canonical) fields deleted, per this pass's directive to remove
  anything not in the real SDK rather than leave it as a documented gap.** The
  prior pass identified but chose to keep three non-canonical fields as "harmless
  extras"; this pass deletes them and fixes the three tests that had locked them
  in as expected behavior (`TestListSchedules_IncludesTargetSummary` in
  schedules_list_test.go asserted `Target.RoleArn`; `TestGetScheduleGroup_IncludesTags` /
  `TestListScheduleGroups_IncludesTags` in schedule_groups_test.go asserted a
  `Tags` field on Get/ListScheduleGroups -- all three now assert the field's
  *absence* and, where relevant, that the real `ListTagsForResource` path still
  returns the correct tags; renamed to `TestGetScheduleGroup_OmitsTags` /
  `TestListScheduleGroups_OmitsTags`):
  - `GetScheduleOutput`/`GetScheduleGroupOutput`/`ScheduleGroupSummary` do not have
    a `Tags` field in the real API (`aws-sdk-go-v2/service/scheduler/types` --
    tags are only ever fetched via `ListTagsForResource`). Deleted from
    `getScheduleOutput` (handler_schedules.go), `getScheduleGroupOutput` and
    `scheduleGroupSummary` (handler_schedule_groups.go).
  - `ScheduleSummary.Target` (`TargetSummary`) has only `Arn` in the real API, not
    `RoleArn`. Deleted `RoleArn` from `scheduleSummaryTarget`
    (handler_schedules.go).
- **DeleteScheduleGroup cascade-delete re-verified correct, not a gap.** Checked the
  real SDK's doc comment on `Client.DeleteScheduleGroup`
  (`api_op_DeleteScheduleGroup.go`): "Deleting a schedule group results in
  EventBridge Scheduler deleting all schedules associated with the group" --
  confirms gopherstack's synchronous cascade-delete (schedule_groups.go) is the
  correct outcome, not a rejection of non-empty groups. AWS's actual behavior is
  *asynchronous* (the group enters a `DELETING` state --
  `types.ScheduleGroupStateDeleting` exists in the real enum -- and schedules drain
  over time, described as "eventually consistent" in the doc comment);
  gopherstack's synchronous, immediate cascade is a deliberate, reasonable
  emulation simplification (consistent with how this in-memory backend has always
  modeled multi-step AWS async operations) and is left as-is -- modeling the
  `DELETING` intermediate state would require a background sweep goroutine for
  marginal emulation value and was judged out of scope for this pass's parity bar
  (schedule CRUD + correct next-invocation computation + state).
- **Protocol / route-matcher / error-shape / timestamp findings from the prior pass
  re-verified, unchanged**: restjson1, no `X-Amz-Target` on real traffic (kept for
  internal test convenience), REST path-to-op mapping matches
  `aws-sdk-go-v2/service/scheduler`'s `serializers.go` exactly for all 12 ops,
  `ConflictException`/`ResourceNotFoundException`/`ValidationException` via
  `service.JSONErrorResponse` match `restjson.GetErrorInfo`'s field lookup,
  `CreationDate`/`LastModificationDate`/`StartDate`/`EndDate` are epoch-seconds
  JSON numbers on both sides (matches `smithytime.FormatEpochSeconds`/
  `ParseEpochSeconds`), resource tags are `[]{Key,Value}` (`TagList`) not a JSON
  map on `CreateScheduleGroup.Tags`/`TagResource.Tags`/`ListTagsForResource.Tags`,
  `UntagResource`'s `TagKeys` REST query param is repeated
  (`?TagKeys=a&TagKeys=b`) not comma-joined, `UpdateSchedule`'s omitted `State`
  does not blank out the schedule's enabled/disabled status (re-verified this pass
  against `awsRestjson1_serializeOpDocumentUpdateScheduleInput`'s
  `if len(v.State) > 0` guard -- the omission is real client behavior, not a
  gopherstack invention, so preserving the existing value on omission remains the
  correct emulation), `ActionAfterCompletion` enum-validated (`NONE`/`DELETE`
  only).
- **"Looks wrong but is correct" traps for the next auditor**:
  - ~~`EcsParameters.Tags []scheduleTargetEcsTag` (`{Key,Value}` list) is unrelated
    to the resource-tag/invented-field findings above and remains correct -- it's
    a genuine, real `Target.EcsParameters.Tags []map[string]string` field
    (per-ECS-task tags at launch time)~~ -- **CORRECTED 2026-08-20**: this note
    contradicted itself (wrote down the real type as `[]map[string]string` then
    called the existing `{Key,Value}` list "correct" anyway) and was wrong. The
    real wire shape genuinely is `[]map[string]string`, not `{Key,Value}` objects;
    see the 2026-08-20 Notes above for the fix and SDK citations.
  - The `X-Amz-Target`/`AWSScheduler.<Op>` JSON-1.1 dispatch path in handler.go is
    dead code for real AWS SDK traffic (restjson1 has no such header) but is kept
    intentionally for internal test convenience (`doSchedulerRequest` in
    handler_test.go uses it) -- don't remove it as "dead code."
  - `Runner.locCache`/`Handler.idempotency` are new pieces of in-memory state added
    this pass; neither owns a goroutine or ticker of its own (both are read/written
    synchronously from the existing poll loop or HTTP handler goroutines), so
    neither needed wiring into `Handler.StartWorker`/`Shutdown`'s ctx-parenting.

## gopherstack-o7gx (2026-08-22): handleREST's ReadBody-failure path wrote untyped errors

`handleREST`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
scheduler is restjson1 (confirmed from `scheduler@v1.20.4` deserializers.go's
`awsRestjson1_deserializeOpError*` prefix); plain text doesn't decode
through `aws/protocol/restjson.GetErrorInfo`, so a real client got
`*json.SyntaxError`, not even `UnknownError`. (This is a different, REST-shaped
site from the one gopherstack-he80/58567cc03 already typed -- that earlier
fix covered `handleError`'s own catch-all branches; this one is the
framework-level `ReadBody` call in `handleREST` itself, upstream of
`handleError` ever being reached.)

Fixed by routing the ReadBody error through this handler's own
`handleError(ctx, c, action, err)`: none of its typed `case`s
(`ErrNotFound`, `ErrAlreadyExists`, `ErrValidation`, `errInvalidRequest`,
`errUnknownAction`, syntax/type errors) match a `*http.MaxBytesError`/read
error, so it falls through to the pre-existing default -- `{Type:
"InternalServerException", Message: ...}` via `service.JSONErrorResponse`
(shared with `pkgs/service`), modeled at `scheduler@v1.20.4`
`types/errors.go:45`.

Proven with a real `aws-sdk-go-v2/service/scheduler` client's
`CreateSchedule`, whose `Description` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandleREST_OversizedBodySurfacesInternalServerException`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalServerException"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after).

## gopherstack-wlo1 (2026-08-22): handleREST's restOpUnknown branch was untyped

`handleREST`'s own `if action == restOpUnknown { return c.String(http.StatusNotFound,
"not found") }` guard (handler.go) wrote a bare text/plain 404 -- a
different, earlier site than the `httputils.ReadBody` failure the
`gopherstack-o7gx` entry above already fixed. scheduler is restjson1
(`scheduler@v1.20.4` `awsRestjson1_` prefix; error decode via
`restjson.GetErrorInfo`), so a real client saw
`smithy.GenericAPIError{Code:"UnknownError"}`.

Reachability: `RouteMatcher` (handler.go) accepts any request whose path has
the coarse `/schedules` or `/schedule-groups` prefix (or a scheduler-owned
`/tags/{arn}`), while `parseSchedulerRESTPath`/`parseScheduleRESTPath`
classify by exact method+segment-count -- a prefix-matched path using a
method none of those cases recognise (e.g. PATCH) falls through to
`restOpUnknown`, the same prefix-vs-classifier gap securityhub's analogous
fix (a98561767) established as provable, not the xray case above where the
two checks are identical.

Fixed: routes through the existing `handleError(ctx, c, action, ErrNotFound)`
-- `ErrNotFound` (errors.go) already maps to `ResourceNotFoundException` at
404 in `handleError`'s own switch, so no new exception vocabulary was
introduced.

Proof: `TestHandleREST_WrongMethodSurfacesResourceNotFoundException`
(`handler_restopunknown_dispatch_malformed_test.go`) drives a real
Scheduler client's `ListSchedules` through a Finalize-stage middleware that
rewrites the request's HTTP method to PATCH post-signing (GET, the method
`ListSchedules` really uses, is itself a valid case in
`parseScheduleRESTPath`, so path corruption alone doesn't reach this
branch). Hand-reverted `handler.go` to `git show HEAD`, confirmed the test
fails with `*json.SyntaxError: "invalid character 'o' in literal null
(expecting 'u')"`, restored the fix, `md5sum`-confirmed byte-identical.

## gopherstack-ui6k (2026-09-06): CreateScheduleGroup/GetScheduleGroup accepted and echoed a fabricated Description field

A prior audit pass recorded this divergence and deliberately left it alone
as "additive, no behavioural parity gain, would touch existing tests for no
parity gain." Re-examined against the `services/redshift/PARITY.md`
gopherstack-emho precedent (`CreateClusterSubnetGroup` accepting a
fabricated `VpcId` request param -- fixed, because an emulator accepting
input real AWS rejects is itself a wire-shape divergence, independent of
whether the field is later read).

**Confirmed against `aws-sdk-go-v2/service/scheduler@v1.20.4`:**
`CreateScheduleGroupInput` (`api_op_CreateScheduleGroup.go:31-42`) has only
`Name` (required), `ClientToken`, `Tags` -- no `Description`.
`awsRestjson1_serializeOpDocumentCreateScheduleGroupInput`
(`serializers.go:254-271`) serializes only `ClientToken`/`Tags` onto the
wire, confirming this isn't merely absent from the generated struct but
genuinely never sent by a real client. `GetScheduleGroupOutput`
(`api_op_GetScheduleGroup.go:39-60`) has `Arn`, `CreationDate`,
`LastModificationDate`, `Name`, `State` -- no `Description`. There is no
`types.ScheduleGroup` type at all (only `types.ScheduleGroupSummary`, used
by `ListScheduleGroups`, which also has no `Description`). The likely
origin: the neighbouring `CreateScheduleGroupInput` sibling
`CreateScheduleInput` (for *schedules*, not schedule *groups*) does carry a
real `Description *string` (`api_op_CreateSchedule.go:89`) -- probably
copied across when the schedule-group handlers were written.

This case is a strictly worse divergence than emho's: emho's backend only
accepted-and-ignored the fabricated field (`VpcId` was never read).
gopherstack's scheduler backend accepted `Description` on
`CreateScheduleGroup`, stored it on the `ScheduleGroup` model, persisted it
in `persistedScheduleGroup`, and returned it from `GetScheduleGroup` --
accept-store-and-return, not accept-and-ignore. Client code reading
`Description` back from `GetScheduleGroup` would work against gopherstack
and fail against real AWS (the field simply wouldn't be there).

Fixed end-to-end: `Description` removed from `createScheduleGroupInput`
(handler_schedule_groups.go, request wire struct) and
`getScheduleGroupOutput` (response wire struct); the `ScheduleGroup` model
field (models.go), `CreateScheduleGroup`'s backend signature
(interfaces.go, schedule_groups.go), and the `persistedScheduleGroup`
snapshot DTO (persistence.go) all dropped it too, rather than leaving a
now-unreachable fabricated field wired through the backend for no reason.

`TestCreateScheduleGroup_WithDescription`
(schedule_groups_test.go) previously asserted the fabricated field
round-tripped through the wire -- exactly the "tests entrenching the
fabricated shape" pattern this repo has hit before (redshift
BatchDeleteClusterSnapshots, ssm AddedLabels). Renamed to
`TestCreateScheduleGroup_DescriptionNotAccepted` and corrected to assert
the real shape: `GetScheduleGroup`'s response must not carry a
`Description` key at all, not merely that it round-trips. Two other
pre-existing tests asserted the `ScheduleGroup.Description` Go field
survived a Snapshot/Restore round trip
(`TestPersistence_RoundTripWithGroupName`,
`TestScheduler_SnapshotRestore_FullState`); corrected to assert a real
field instead (`g.Name`/`groupTags.State`) since there is no real
replacement for a fabricated field's round-trip.
`TestCreateScheduleGroup_NameRequired` incidentally used
`{"Description": "no-name"}` as its missing-`Name` request body; simplified
to `{}` since `Description` is no longer a plausible field name to reach
for.

Proof: `TestCreateScheduleGroup_DescriptionNotAccepted` hand-reverted
against `git show HEAD` for all 14 touched files (both production and the
mechanical `CreateScheduleGroup` call-site signature updates across test
files), confirmed it fails --
`schedule_groups_test.go:124: Error: Should be false ... Messages:
GetScheduleGroupOutput has no Description member on real AWS` -- then
every file restored from a pre-revert backup.

Snapshot-version guard: no version bump. `persistedScheduleGroup`'s decode
path (`pkgs/store/table.go`) uses plain `json.Unmarshal` with no
`DisallowUnknownFields`, so an older snapshot's now-unrecognised
`"description"` key is silently dropped on restore -- safe, no data loss
for any *remaining* field, matching the guard's own stated bar for when a
bump is unwarranted. `go test ./pkgs/persistence/ -run
TestSnapshotVersionGuard` (read-only, no `-update`, `pkgs/persistence/
testdata/` is out of scope for this change) reports scheduler's golden
entry as stale, as expected; refreshing it is for whoever owns
`pkgs/persistence/testdata/`.
