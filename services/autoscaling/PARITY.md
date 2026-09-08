---
service: autoscaling
sdk_module: aws-sdk-go-v2/service/autoscaling@v1.70.4
last_audit_commit: 1c4ee34e
last_audit_date: 2026-07-23
# ERROR path verified 2026-08-29 (wrapper-key-sweep pass): extracted every
# op's deserializeOpError<Op> switch (autoscaling@v1.70.4 deserializers.go,
# 66/67 ops N-of-N). Handler.autoscalingErrorCode is one global sentinel
# table applied to all ops. Confirmed the AlreadyExists/ResourceInUse/
# ScalingActivityInProgress/ActiveInstanceRefreshNotFound sentinels are each
# used only by ops that model that exact code -- no wrong-code bugs found
# there. "ValidationError" (ErrInvalidParameter and 5 other not-found
# sentinels' shared code) does not exist anywhere in this SDK's exception set
# -- confirmed: the whole autoscaling API models only 11 typed exceptions
# (AlreadyExistsFault/LimitExceededFault/ResourceContentionFault/
# ResourceInUseFault/ScalingActivityInProgressFault/
# ActiveInstanceRefreshNotFoundFault/InstanceRefreshInProgressFault/
# IrreversibleInstanceRefreshFault/InvalidNextToken/
# IdempotentParameterMismatchError/ServiceLinkedRoleFailure), none matching
# generic not-found/invalid-parameter -- left as-is per campaign restraint
# (no op models anything this class of failure could be corrected to).
# ErrUnknownAction ("InvalidAction") fires only for an unrecognized Action=
# value at the routing layer, before any operation is identified -- a real
# typed SDK client can never construct such a request, so this path is
# unreachable by real traffic and not a bug of this class.
# Missing-error bug found and fixed: StartInstanceRefresh accepted a second
# concurrent call unconditionally instead of rejecting it -- the op's own
# deserializer models InstanceRefreshInProgress for exactly this case. Added
# ErrInstanceRefreshInProgress and an in-progress check (instance_refreshes.go).
# See error_sentinel_fixes_test.go (real-SDK errors.As assertion, confirmed
# failing pre-fix).
overall: A            # parity-3 sweep. No aws-sdk-go-v2/service/autoscaling version bump
                       # (still v1.64.2 in go.mod/go.sum). This pass independently
                       # field-diffed the prior pass's "gaps" list against actual code
                       # (per the campaign's "if PARITY.md counts don't match reality,
                       # independently field-diff" instruction) and found it was stale in
                       # two places: the "ASG->EC2" and "ASG/ECS->ELBv2" gaps were already
                       # fixed by a prior, undocumented pass (bd gopherstack-8sk/18k,
                       # confirmed closed; EC2Launcher/ELBv2TargetRegistrar wiring verified
                       # present in ec2_launch.go/elbv2_targets.go) and the
                       # scale-in-lifecycle-hook-gating gap the prior ledger claimed to
                       # have fixed (bd gopherstack-9wo) was in fact genuinely fixed in
                       # code (applyScaleIn/terminationCapacityPreset in
                       # auto_scaling_groups.go) - the bd issue itself was just left open
                       # by mistake, now closed. Real work this pass: (1) implemented the
                       # scheduled-action background scheduler that was the one
                       # deliberately-deferred gap with a live bd id (gopherstack-6ys) -
                       # see families below; (2) wired the 7 CreateAutoScalingGroupInput/
                       # UpdateAutoScalingGroupInput fields the prior ledger listed as
                       # "not attempted" (AvailabilityZoneDistribution,
                       # AvailabilityZoneImpairmentPolicy,
                       # CapacityReservationSpecification, DeletionProtection,
                       # InstanceLifecyclePolicy, InstanceMaintenancePolicy,
                       # SkipZonalShiftValidation), including making DeletionProtection a
                       # real DeleteAutoScalingGroup gate, not just a stored/echoed value;
                       # (3) removed all 7 banned complexity nolints (cyclop/gocognit/
                       # funlen) via decomposition, zero remaining, zero golangci-lint
                       # issues. No leak: `go test -race` clean; the new scheduler goroutine
                       # is ctx-parented and Shutdown-drained via pkgs/worker.SingleRun
                       # (see families below).
ops:
  CreateAutoScalingGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: MixedInstancesPolicy, LifecycleHookSpecificationList, TrafficSources were parsed as no-ops (silently dropped) - now parsed, validated, and registered atomically with the group; initial instances are gated by any launch hook just registered. Prior pass: wired 7 previously-unparsed fields (AvailabilityZoneDistribution, AvailabilityZoneImpairmentPolicy, CapacityReservationSpecification, DeletionProtection, InstanceLifecyclePolicy, InstanceMaintenancePolicy, SkipZonalShiftValidation) - parsed, validated (DeletionProtection enum), stored, and (all but SkipZonalShiftValidation, which real AWS itself never echoes back - verified against types.AutoScalingGroup) projected on Describe. bd gopherstack-2uti: MixedInstancesPolicy.LaunchTemplate.Overrides.member.N.InstanceRequirements (attribute-based instance-type selection, 24 of 25 sub-fields) is now parsed; also fixed a real loop-termination bug in parseLaunchTemplateOverrides - an override carrying only InstanceRequirements (no InstanceType/WeightedCapacity/LaunchTemplateSpecification, the common real-world shape) was indistinguishable from 'no more members', silently truncating every override after it too. bd gopherstack-02ue (this pass): the 25th and last InstanceRequirements field, BaselinePerformanceFactors, is now modelled too - see Notes for its wire-shape outlier (singular 'Reference' key, 'item'-wrapped list)"}
  DescribeAutoScalingGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "added MixedInstancesPolicy to the XML projection (was entirely absent from xmlAutoScalingGroup even though the backend model carried it). bd gopherstack-2uti: projects InstanceRequirements on each override (see CreateAutoScalingGroup). bd gopherstack-02ue (this pass): projects BaselinePerformanceFactors too"}
  UpdateAutoScalingGroup: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed: MixedInstancesPolicy was not parsed from the request. Prior passes: scale-in path (via applyDesiredCapacityChange) now also gates on a terminating lifecycle hook (bd gopherstack-9wo; re-verified present in code this pass, the bd issue itself was just stale-open); wired the same 7 fields as CreateAutoScalingGroup (see above); each pointer-struct field replaces the group's existing value wholesale when present in the request (matches AWS's opaque-nested-object semantics - there is no partial-field patch for e.g. InstanceMaintenancePolicy). bd gopherstack-2uti / bd gopherstack-02ue: inherits the InstanceRequirements (incl. BaselinePerformanceFactors) parsing fix via the shared parseMixedInstancesPolicy/parseLaunchTemplateOverrides helpers. write-only-state sweep (this pass): PlacementGroup was a plain string guarded by != \"\" (not *string like the real UpdateAutoScalingGroupInput.PlacementGroup, api_op_UpdateAutoScalingGroup.go), whose doc says \"To remove the placement group setting, pass an empty string for placement-group\" -- a client's explicit clear was silently dropped. Now *string with a nil check. Round-trip test: wire_field_fixes_test.go."}
  DeleteAutoScalingGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: DeletionProtection is now a real gate, not just a stored/echoed value - prevent-all-deletion rejects every delete, prevent-force-deletion rejects only ForceDelete=true, matching real AWS's ResourceInUse (ErrorCode) fault. Previously the field didn't exist on the model at all"}
  CreateLaunchConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLaunchConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLaunchConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeScalingActivities: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-21 (gopherstack-r80d batch 29): types.Activity.Cause (aws-sdk-go-v2/service/autoscaling@v1.70.4/types/types.go:298, *string, required) had no backing field on ScalingActivity at all -- every DescribeScalingActivities/DescribeAutoScalingInstances/DetachInstances/EnterStandby/ExitStandby-triggered activity decoded a nil Cause on a real client, in every reachable state, since the field never existed to populate. Added ScalingActivity.Cause and threaded a real, derived narrative through all 8 construction sites (auto_scaling_groups.go, instances.go x6, scheduled_action_scheduler.go) via a shared scalingActivityCause helper -- not fabricated data, built from the same real group/instance/hook identifiers Description already carries. Proven via a real aws-sdk-go-v2/service/autoscaling client round trip (wire_output_required_r80d_test.go), hand-reverted (7 files)/confirmed-failing/restored, md5sum-verified byte-identical."}
  AttachInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  AttachLoadBalancerTargetGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  AttachLoadBalancers: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed bd gopherstack-hch9: was bookkeeping-only (appended to LoadBalancerNames with no ELB-side effect, unlike its ELBv2 sibling AttachLoadBalancerTargetGroups). Now registers the group's existing instances with the newly-attached load balancers via the new ELBInstanceRegistrar hook (elb_targets.go), mirroring elbv2-target-registration's shape and behavior exactly - see that families entry"}
  AttachTrafficSources: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchDeleteScheduledAction: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchPutScheduledUpdateGroupAction: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: StartTime/EndTime were parsed nowhere and silently dropped; now parsed and stored"}
  CancelInstanceRefresh: {wire: ok, errors: ok, state: ok, persist: ok}
  CompleteLifecycleAction: {wire: ok, errors: ok, state: ok, persist: ok, note: "CRITICAL fix: previously only stopped a timer that was never created anywhere (dead code) and had zero effect on instance state. Now resolves a real pending lifecycle wait (Pending:Wait/Terminating:Wait -> actual transition), looked up by token OR by (group,hook,instance). This pass (bd gopherstack-2uti): ABANDON on a launching hook now terminates AND relaunches a replacement to restore DesiredCapacity (see Notes) - previously it terminated with no replacement, silently leaving the group under capacity"}
  CreateOrUpdateTags: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLifecycleHook: {wire: ok, errors: ok, state: ok, persist: ok}
  SetDesiredCapacity: {wire: ok, errors: ok, state: ok, persist: ok, note: "scale-out path gates new instances through an active launch hook. This pass: scale-in path now also gates removed instances through an active terminating hook (was previously immediate regardless of hooks; closes bd gopherstack-9wo) via the new applyScaleIn/terminationCapacityPreset machinery - see Notes"}
  TerminateInstanceInAutoScalingGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "CRITICAL fix: now defers actual removal to Terminating:Wait + CompleteLifecycleAction/timeout when a terminating hook is registered, instead of always terminating instantly; also fixed the replacement-instance path never adding the new instance to instanceIndex"}
  PutLifecycleHook: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: NotificationMetadata was never parsed from the request"}
  DescribeLifecycleHooks: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeScheduledActions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-31 (value-semantics pass): ScheduledActionNames only filtered when AutoScalingGroupName was ALSO given (len(actionNames)>0 && groupName!=\"\"); api_op_DescribeScheduledActions.go documents ScheduledActionNames unconditionally (\"If you omit this property, all scheduled actions are described\") with AutoScalingGroupName as a separate optional field, not a precondition. Supplying names without a group name fell through to the time-range path, which does not consult actionNames at all -- every group's actions in the (usually unbounded) time window were returned instead, silently dropping the name filter and admitting unwanted actions from other groups. scheduledActionsByNamesLocked now searches every group when groupName is empty (ScheduledActionName is unique only within a group, so a name can legitimately match entries in more than one). Regression test TestAutoscalingHandler_DescribeScheduledActions/scheduled_action_names_filters_without_group_name, proved failing pre-fix."}
  DeleteTags: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-31 (value-semantics pass): tagMatchesFilters recognised auto-scaling-group/key/value but not the fourth documented Filter Name, propagate-at-launch (types.Filter, types/types.go:844-847, \"Accepts a Boolean value ... results only include tags associated with the specified Boolean value\") -- an unrecognised Name silently matched every tag, so this filter was a no-op. Also found while fixing it: DescribeTags never copied PropagateAtLaunch from the stored Tag into the response ResourceTag at all, so the response's own PropagateAtLaunch field always reported false regardless of the real stored value -- a real client could not read the field's correct value at all, let alone filter on it. Both fixed together (tags.go); regression test TestInMemoryBackend_DescribeTags_WithFilters/filter_by_propagate_at_launch, proved failing pre-fix. NOT fixed, recorded separately: the standalone CreateOrUpdateTags API (distinct from tags set at CreateAutoScalingGroup time, which correctly thread PropagateAtLaunch via parseTags) drops PropagateAtLaunch on both create and update, always storing/leaving false -- a write-path bug, not a Describe-filter-semantics bug, kept out of this pass's scope."}
  DescribeAutoScalingInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteNotificationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteScheduledAction: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteWarmPool: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAccountLimits: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeAdjustmentTypes: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeAutoScalingNotificationTypes: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeInstanceRefreshes: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLifecycleHookTypes: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeLoadBalancerTargetGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLoadBalancers: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeMetricCollectionTypes: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeNotificationConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribePolicies: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: MinAdjustmentStep and MetricAggregationType were never returned. bd gopherstack-2uti: now echoes back PredictiveScalingConfiguration (see PutScalingPolicy). bd gopherstack-02ue (this pass): now echoes back TargetTrackingConfiguration.CustomizedMetricSpecification and the three predictive-scaling Customized*MetricSpecification variants"}
  DescribeScalingProcessTypes: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeTerminationPolicyTypes: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeTrafficSources: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeWarmPool: {wire: ok, errors: ok, state: ok, persist: ok}
  DetachInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  DetachLoadBalancerTargetGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DetachLoadBalancers: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed bd gopherstack-hch9: was bookkeeping-only, no ELB-side deregistration on detach (unlike DetachLoadBalancerTargetGroups). Now deregisters via ELBInstanceRegistrar - see elb-instance-registration families entry"}
  DetachTrafficSources: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableMetricsCollection: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableMetricsCollection: {wire: ok, errors: ok, state: ok, persist: ok}
  EnterStandby: {wire: ok, errors: ok, state: ok, persist: ok}
  ExecutePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: StepScaling policies ignored StepAdjustments/MetricValue/BreachThreshold entirely and always used the flat ScalingAdjustment/AdjustmentType path; now selects the matching StepAdjustment interval and validates the required fields. Also routed through applyDesiredCapacityChange so ExecutePolicy scale-out/in now respects SuspendedProcesses, scale-in protection, instanceIndex bookkeeping, and launch-hook gating like SetDesiredCapacity does (previously it duplicated and diverged from that logic). This pass: inherits terminating-hook gating on scale-in for free via the same applyDesiredCapacityChange routing"}
  ExitStandby: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPredictiveScalingForecast: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "fixed: response was missing the required UpdateTime field and returned a wrong-shaped, entirely empty LoadForecast; now returns UpdateTime and a real (though intentionally naive - see Notes) Timestamps/Values series. fixed 2026-08-21 (gopherstack-r80d batch 29): types.LoadForecast.MetricSpecification (types.go:2670, *PredictiveScalingMetricSpecification, required) was structurally absent from every LoadForecast entry -- always nil on a real client, even though the referenced policy's own PredictiveScalingConfiguration.MetricSpecifications (parsed and stored by PutScalingPolicy, bd gopherstack-2uti) already carries the real data. handleGetPredictiveScalingForecast now reads PolicyName, looks the policy up via the already-exported DescribePolicies, and emits one LoadForecast entry per configured metric specification with MetricSpecification populated from real stored data (toXMLPredictiveScalingMetricSpecification, extracted from the existing PutScalingPolicy/DescribePolicies converter so both paths share one conversion). Falls back to a single unlabeled series (MetricSpecification nil) only when the policy or its PredictiveScalingConfiguration can't be found -- a pre-existing, out-of-scope gap (this op has no PolicyName/PolicyType validation at all) left unchanged. Proven via a real aws-sdk-go-v2/service/autoscaling client round trip (wire_output_required_r80d_test.go), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
  LaunchInstances: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 3 bugs: (1) handler read the wrong query param (DesiredCapacity instead of the real RequestedCapacity, so every call silently launched only 1 instance regardless of the requested count); (2) response used the DescribeAutoScalingGroups per-instance shape instead of the real LaunchInstancesOutput InstanceCollection (grouped by AZ/InstanceType with InstanceIds) shape; (3) the backend never added launched instances to instanceIndex, so they could never be found by TerminateInstanceInAutoScalingGroup"}
  PutNotificationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  PutScalingPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: MetricAggregationType was accepted nowhere on input or output. bd gopherstack-2uti: PredictiveScalingConfiguration rode along entirely unparsed - accepted with 200 OK, silently discarded. Now parses the top-level scalar fields (MaxCapacityBreachBehavior/MaxCapacityBuffer/Mode/SchedulingBufferTime) and MetricSpecifications' three predefined-metric variants. bd gopherstack-02ue (this pass): the four Customized*MetricSpecification variants (TargetTrackingConfiguration.CustomizedMetricSpecification, and PredictiveScalingMetricSpecification's Customized{Load,Scaling,Capacity}MetricSpecification) were the same silent-discard gap - TargetTrackingConfiguration.CustomizedMetricSpecification was worse: a dead ScalingPolicy.CustomizedMetricSpecification string field existed but was never even parsed from the wire. All four now modelled in full, including the shared CloudWatch MetricDataQuery/MetricStat/Metric/Dimensions nesting - see Notes"}
  PutScheduledUpdateGroupAction: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: StartTime/EndTime were parsed nowhere and silently dropped despite the backend model and DescribeScheduledActions XML projection already supporting them"}
  PutWarmPool: {wire: ok, errors: ok, state: ok, persist: ok}
  RecordLifecycleActionHeartbeat: {wire: ok, errors: ok, state: ok, persist: ok, note: "was re-arming a timer that called a no-op (expireHookAction just deleted the map entry); now re-arms to re-resolve with the hook's DefaultResult, and supports lookup by instance ID (not just token)"}
  ResumeProcesses: {wire: ok, errors: ok, state: ok, persist: ok}
  RollbackInstanceRefresh: {wire: ok, errors: ok, state: ok, persist: ok}
  SetInstanceHealth: {wire: ok, errors: ok, state: ok, persist: ok}
  SetInstanceProtection: {wire: ok, errors: ok, state: ok, persist: ok}
  StartInstanceRefresh: {wire: ok, errors: ok, state: ok, persist: ok}
  SuspendProcesses: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  static-describe-types (AdjustmentTypes/NotificationTypes/LifecycleHookTypes/MetricCollectionTypes/ScalingProcessTypes/TerminationPolicyTypes): {status: ok, note: "unchanged this pass; verified op-by-op against the SDK enum lists, all correct"}
  instance-refresh (Start/Cancel/Describe/Rollback): {status: ok, note: "unchanged this pass; wire shapes and status machine (InProgress/Cancelling/RollbackInProgress) verified against SDK"}
  warm-pool (Put/Delete/Describe): {status: ok, note: "unchanged this pass; verified"}
  metrics-collection / suspend-resume-processes / standby: {status: ok, note: "unchanged this pass; verified"}
  ec2-provisioning (ASG to EC2 real instance launch/terminate via EC2Launcher): {status: ok, note: "was gap bd gopherstack-8sk, marked NOT-fixed by the prior ledger; independently field-diffed this pass and found ALREADY fixed by an undocumented earlier pass - services/autoscaling/ec2_launch.go defines EC2Launcher (LaunchInstances/TerminateInstances), auto_scaling_groups.go/instances.go route scale-out/in through it when wired (SetEC2Launcher), bd gopherstack-8sk is closed. Ledger corrected to reflect reality"}
  elbv2-target-registration (ASG to ELBv2 real target register/deregister via ELBv2TargetRegistrar): {status: ok, note: "was gap bd gopherstack-18k, marked NOT-fixed by the prior ledger; independently field-diffed this pass and found ALREADY fixed by the same undocumented earlier pass - services/autoscaling/elbv2_targets.go defines ELBv2TargetRegistrar (RegisterTargets/DeregisterTargets), wired into attach/detach/scale-in paths, bd gopherstack-18k is closed. Ledger corrected to reflect reality"}
  scheduled-action-scheduler (background execution of Put/BatchPutScheduledUpdateGroupAction): {status: ok, note: "NEW this pass, closing bd gopherstack-6ys. Prior passes correctly parsed/persisted StartTime/EndTime/Recurrence but nothing ever evaluated them against wall-clock time - DescribeScheduledActions reflected what was requested, but no action ever fired. Added scheduled_action_cron.go (5-field Unix-cron parser matching AWS's documented Recurrence format: minute hour day-of-month month day-of-week - distinct from EventBridge's 6-field cron() with a year field) and scheduled_action_scheduler.go (ScheduledActionScheduler, a service.BackgroundWorker: 1-minute ticker, wired via pkgs/worker.SingleRun in handler.go's StartWorker/Shutdown so it is ctx-parented and Shutdown-drained like every other service's background worker in this codebase). Each tick applies any due action's MinSize/MaxSize/DesiredCapacity through the same validated capacity path (applyUpdateCapacityLocked) UpdateAutoScalingGroup uses, so it inherits identical validation/error behavior. Covers one-time actions (Recurrence empty, fires once at/after StartTime) and recurring actions (bounded by StartTime/EndTime when set); a new ScheduledAction.LastExecutedTime field (internal bookkeeping, not on the wire - AWS's real ScheduledUpdateGroupAction response type has no equivalent field) prevents re-firing the same occurrence and prevents an invalid action from busy-looping every tick"}
  lifecycle-hook-chaining (multiple hooks on one transition): {status: ok, note: "FIXED this pass (bd gopherstack-9tqg, deferred from bd gopherstack-2uti/b7d3a8485). Registering a second+ hook on the same transition previously armed nothing - see dated Notes section below for the ordering rule, the chain data model, and how it composes with ABANDON's terminate-and-replace"}
  elb-instance-registration (ASG to classic ELB real instance register/deregister via ELBInstanceRegistrar): {status: ok, note: "FIXED this pass, closing bd gopherstack-hch9. Classic ELB had no registrar equivalent to ELBv2TargetRegistrar: AttachLoadBalancers/DetachLoadBalancers only ever mutated LoadBalancerNames, and none of the 7 other places that register/deregister ELBv2 targets (scale-out fabricated and real-EC2-launch paths, AttachInstances, TerminateInstanceInAutoScalingGroup, the terminating-lifecycle-hook-wait resolution path, DetachInstances, scale-in) had a classic-ELB counterpart either - confirmed by reading elbv2_targets.go and finding zero uses of RegisterInstancesWithLoadBalancer/DeregisterInstancesFromLoadBalancer (services/elb/instances.go) anywhere in this package. Added elb_targets.go: ELBInstanceRegistrar (RegisterInstances/DeregisterInstances, instance IDs only - real AWS's RegisterInstancesWithLoadBalancer/DeregisterInstancesFromLoadBalancer, elasticloadbalancing@v1.36.4 api_op_*.go, take LoadBalancerName+[]Instance{InstanceId} with no port, unlike ELBv2's target+port shape) plus SetELBRegistrar/registerELBInstances/deregisterELBInstances, mirroring elbv2_targets.go's structure exactly (nil-guarded no-op when unwired, best-effort logged-not-propagated errors). Wired a parallel registerELBInstances/deregisterELBInstances call alongside every existing registerELBTargets/deregisterELBTargets call site (9 total, one more than ELBv2's 8 - applyScaleIn's no-hook branch in auto_scaling_groups.go was an ELBv2 call site the prior pass's own count missed), plus AttachLoadBalancers/DetachLoadBalancers themselves gained the register-existing-members-on-attach / deregister-on-detach behavior AttachLoadBalancerTargetGroups/DetachLoadBalancerTargetGroups already had. cli.go: wireAutoScalingELB + autoscalingELBRegistrarAdapter, wired alongside wireAutoScalingELBv2. All 9 call sites covered by elb_targets_test.go (including two the existing ELBv2 suite has no equivalent test for: the real-EC2Launcher registration path and the terminating-lifecycle-hook-wait resolution path), each confirmed to fail only its own test when neutered by line number"}
gaps:
  - GetPredictiveScalingForecast returns a real, well-shaped, non-empty forecast, but it is a flat naive projection (current DesiredCapacity repeated hourly), not a statistical model - genuinely out of scope for an emulator; documented simplification, see Notes
  - "PutScalingPolicy's parsePredictiveScalingMetricSpecifications (gopherstack-r80d batch 29, reviewed not fixed): a MetricSpecifications element carrying only a Customized*/Predefined* sub-field with no TargetValue is accepted with TargetValue defaulted to 0.0 instead of rejected, even though AWS's own doc comment on this exact field says \"TargetValue is required ... on every element\" and its client-side validator (validators.go:1660 validatePredictiveScalingMetricSpecification) unconditionally rejects a nil TargetValue. Out of scope for this cut (an input-validation permissiveness gap, not a dropped required OUTPUT field -- the wire-side TargetValue member has no omitempty and is always echoed correctly) and, per this campaign's proof standard, not reachable via any real aws-sdk-go-v2 client anyway (the SDK's own validator blocks the request before it is ever sent) -- same \"unreachable via any real Go SDK client\" class apprunner's batch 10 SourceCodeVersion hit. Left unfixed."
deferred: []
leaks: {status: clean, note: "go test -race passes (verified this pass). The pendingHookTokens timer machinery (the CRITICAL item flagged in a prior sweep) remains real (armed on every gated launch/terminate), Close() stops all of them, DeleteAutoScalingGroup/DeleteLifecycleHook/Purge call cleanupHookTimers, and Restore() re-arms timers for any instance left in a *:Wait state. NEW this pass: the ScheduledActionScheduler's 1-minute ticker goroutine is started via pkgs/worker.SingleRun.Start in Handler.StartWorker and stopped (cancelled + waited-on) via pkgs/worker.SingleRun.Stop in Handler.Shutdown - the exact same ctx-parented/Shutdown-drained shape every other backgroundWorker service in this codebase uses (e.g. secretsmanager's rotation scheduler). TestScheduledActionScheduler_RunFiresAndStopsCleanly explicitly starts the real ticker, waits for it to fire, cancels its context, and asserts Run() returns within 2s. testleak.VerifyTestMain (leak_main_test.go) additionally guards the whole package: any test that started a worker without stopping it would fail the suite."}
---

## Notes

Protocol: EC2 Auto Scaling uses the `query` (form-urlencoded request, XML response)
protocol, `Version=2011-01-01`. Verified against the awsquery serializers/deserializers
in `aws-sdk-go-v2/service/autoscaling@v1.64.2`.

### bd gopherstack-2uti (2026-08-08): PredictiveScalingConfiguration, InstanceRequirements, ABANDON auto-relaunch

Three specific, previously-documented gaps (see prior `gaps`/`deferred` entries this
section replaces), addressed in priority order (worst failure mode first, per the
issue's own priority note - a config accepted and silently discarded is worse than a
missing feature or a documented simplification):

1. **`PutScalingPolicy`/`DescribePolicies`: `PredictiveScalingConfiguration`.**
   Verified the exact query-protocol flattening against
   `aws-sdk-go-v2/service/autoscaling@v1.70.4`'s `serializers.go:5967`
   (`awsAwsquery_serializeDocumentPredictiveScalingConfiguration`) and
   `deserializers.go:16133` before writing any parsing code, rather than inferring it
   from field names: `PredictiveScalingConfiguration.{MaxCapacityBreachBehavior,
   MaxCapacityBuffer,Mode,SchedulingBufferTime}` are flat scalars;
   `MetricSpecifications` is a standard `.member.N.`-flattened list (confirmed against
   `aws-sdk-go-v2@v1.43.4`'s `aws/protocol/query/array.go` - non-flat arrays are
   `<prefix>.<memberName>.<n>`, memberName is always `"member"` for this protocol,
   matching every other list in this handler); each element has an independent
   `TargetValue` plus one of three `{PredefinedMetricType,ResourceLabel}`-shaped
   predefined-metric objects (`PredefinedMetricPairSpecification`/
   `PredefinedLoadMetricSpecification`/`PredefinedScalingMetricSpecification`,
   `types.go:2743/2778/2821`). Added `PredictiveScalingConfiguration`/
   `PredictiveScalingMetricSpecification`/`PredefinedMetricRef` to `models.go` and
   wired parse/echo into `handler_scaling_policies.go`. The `Customized*` metric
   variants (CloudWatch `MetricDataQuery` math expressions) are deliberately not
   modelled - see `deferred`.

2. **`MixedInstancesPolicy.LaunchTemplate.Overrides[].InstanceRequirements`**
   (attribute-based instance-type selection, `types.go:1267`, 25 fields). Modelled 24
   of 25 as `IntRangeRequest`/`FloatRangeRequest` `{Min,Max}` pairs (6 int, 3 float),
   8 `.member.N`-flattened string lists, 3 plain string enums, 3 `*int32` scalars, and
   1 `*bool`; `BaselinePerformanceFactors` (nests a CPU-instance-family reference list
   with no analogue elsewhere in this handler) is the one field deferred - see
   `deferred`. Verified the flattening the same way as (1), directly against
   `serializers.go:5230` (`awsAwsquery_serializeDocumentInstanceRequirements`) and its
   sub-message serializers, and `deserializers.go:12592` for the response side.
   Fixing this also surfaced and fixed a real, independent bug: `parseLaunchTemplateOverrides`'s
   loop-continuation check only looked at
   `InstanceType`/`WeightedCapacity`/`LaunchTemplateSpecification` presence - an
   override carrying only `InstanceRequirements` (no `InstanceType`, the whole point
   of attribute-based selection, and the shape Terraform emits for
   `instance_requirements` blocks) was indistinguishable from "end of list", silently
   truncating every override after it in the same request too. Covered by
   `TestAutoscalingHandler_MixedInstancesPolicyInstanceRequirementsRoundTrip`, which
   asserts a *second* override with a plain `InstanceType` survives past a first,
   `InstanceRequirements`-only override - verified this fails pre-fix (empty
   `<Overrides>`, both members dropped).

3. **ABANDON on a launching lifecycle hook now relaunches a replacement.** AWS's
   lifecycle-hooks docs (`docs.aws.amazon.com/autoscaling/ec2/userguide/lifecycle-hooks.html`,
   "Considerations and limitations", fetched this pass) state: "If an instance is
   launching, continue indicates that your actions were successful, and that Amazon
   EC2 Auto Scaling can put the instance into service. Otherwise, abandon indicates
   that your custom actions were unsuccessful, and that **we can terminate and replace
   the instance**." (emphasis added). The prior implementation terminated the failed
   instance but never replaced it, silently leaving the group permanently under
   `DesiredCapacity` until some unrelated event (a scaling policy, a manual
   `SetDesiredCapacity` call) happened to top it back up. Fixed in
   `applyLifecycleResult`'s launching/ABANDON branch by reusing the exact same
   top-up-to-`DesiredCapacity` pattern `finishTermination`'s `terminationReplace`
   disposition already used for the analogous terminating-hook case
   (`adjustInstances` + `instanceIndex` registration + `gateNewLaunchInstances`, so
   the replacement is itself gated by the same launch hook, matching real AWS - a
   replacement for an abandoned instance is a normal launch, not a bypass). Required
   updating one existing test
   (`TestAutoscalingHandler_LifecycleHookGatesLaunch`/"abandon...") whose assertion
   (`assert.Empty(t, gotInstances, ...)`) encoded the old, AWS-incorrect behavior;
   verified the updated test fails against the pre-fix code (group stuck at 0
   instances despite `DesiredCapacity=1`) and passes after.

   The same docs page also answers the *multiple-hooks-on-one-transition* ordering
   question this issue asked about, for the *terminating* case specifically: "If an
   instance is terminating, both abandon and continue allow the instance to
   terminate. However, **abandon stops any remaining actions, such as other lifecycle
   hooks, and continue allows any other lifecycle hooks to complete**." This confirms
   AWS's model is an ordered chain with a short-circuit-on-abandon semantic, not
   documented true concurrency, and there is no `order`/`priority` field anywhere in
   `PutLifecycleHookInput`/`LifecycleHookSpecification` to determine the chain order -
   the SDK does not determine it, and neither does this doc page. Implementing the
   chain (arm one hook at a time per instance+transition, advance to the next on
   CONTINUE, short-circuit on ABANDON, across all 4 `armLifecycleWait` call sites plus
   the Restore-time `rearmPendingWaits` path) is a materially larger, riskier change
   than (1) and (2) above and was deliberately left as a documented gap rather than
   rushed - see `gaps`.

### bd gopherstack-9tqg (2026-08-08): lifecycle-hook chaining

Implemented the chain deferred by bd `gopherstack-2uti`/b7d3a8485 above: registering a
second (or third, ...) hook on the same transition now actually arms it, instead of
silently doing nothing.

**Ordering rule.** Neither `PutLifecycleHookInput` nor `LifecycleHookSpecification`
(`aws-sdk-go-v2/service/autoscaling@v1.70.4` `api_op_PutLifecycleHook.go:70-140`,
`types.go:1973-2020`) carries an order/priority field, and `DescribeLifecycleHooksOutput`
(`api_op_DescribeLifecycleHooks.go:42-51`) is a plain `[]types.LifecycleHook` with no
ordering metadata either - the SDK does not determine chain order, and the pending-hook
wait itself is not observable to a client beyond the instance's coarse
`LifecycleState` (`Pending:Wait`/`Terminating:Wait`; there is no per-hook field on
`AutoScalingInstanceDetails`). Chose **registration order** as the defensible default
and documented it here and in `lifecycleHookChain`'s doc comment
(`lifecycle_hooks.go`): each `LifecycleHook` gets an internal-only `Sequence` field,
assigned once from a backend counter (`nextHookSeq`) the first time a hook of that name
is registered (`putLifecycleHookLocked`) and preserved across updates to the same hook.
`Sequence` is never sent or accepted on the wire - `handleDescribeLifecycleHooks` builds
`xmlLifecycleHook` field-by-field rather than by converting `LifecycleHook` directly, so
adding it couldn't leak onto the response the way a straight type conversion would have.

**Mechanics.** `armLifecycleWait` now also records the armed hook's name on the
instance (`Instance.LifecycleHookName`). `applyLifecycleResult`, on CONTINUE, looks up
the next hook in `lifecycleHookChain` after the one that just resolved and re-arms the
same instance on it instead of applying the transition's terminal effect; only once the
chain is exhausted does it fall through to `InService`/`finishTermination`. ABANDON
never consults the chain - it goes straight to the terminal effect at whatever position
it occurred, i.e. short-circuits.

**Composing with b7d3a8485's terminate-and-replace.** ABANDON on a launching hook,
at any chain position, still reuses the exact same terminate-and-replace branch
(`removeInstanceByID` + `adjustInstances` + `gateNewLaunchInstances`) that existed
before this pass - chaining only changes *which* hook's resolution can reach that
branch, not the branch itself. The replacement instance is a brand-new `Instance`, so
`gateNewLaunchInstances` arms it via `firstHookInChain`, restarting the launch chain
from hook 1 rather than continuing wherever the abandoned instance's chain position
was. Verified with a three-hook launching test
(`launchChainAbandonShortCircuits`): hook-1 CONTINUE advances to hook-2 (proving the
chain-advance code path actually ran, not merely that a single-hook flow still works),
hook-2 ABANDON terminates-and-replaces without hook-3 ever being armed, and the
replacement is confirmed back at hook-1 (completing hook-3 on it is a no-op; completing
hook-1 is not).

**Restore mid-chain.** The hardest part: `pendingHookTokens` (in-flight timers/action
state, including which hook is currently gating an instance) is deliberately never
persisted, but which hook an instance is paused on cannot be recovered from
`lifecycleHookChain` alone once the group's earlier hooks have already resolved. Added
`Instance.LifecycleHookName` (internal-only, rides along transparently in the existing
`AutoScalingGroup`/`store.Table[AutoScalingGroup]` JSON snapshot - additive field, no
`autoscalingSnapshotVersion` bump needed, same precedent as b7d3a8485's
`PredictiveScalingConfiguration` addition) so `rearmPendingWaits` can look the specific
hook back up by name after `Restore()`, falling back to `firstHookInChain` only when
that hook is empty (pre-chain-tracking snapshot) or gone (deleted while persisted).
`nextHookSeq` itself is not persisted (it is not observable through any AWS API, so
there is nothing to keep byte-identical across a restore); `Restore()` recomputes it as
the max `Sequence` across restored hooks, so hooks registered post-restore chain after
all of them rather than colliding. Verified with a dedicated round-trip test
(`TestAutoscalingHandler_LifecycleHookChainResumesAfterRestore`): a two-hook launch
chain is advanced past hook-1 (now waiting on hook-2), snapshotted, restored into a
fresh backend, and completing hook-1 again post-restore is asserted to be a no-op
(proving the chain did not restart) while completing hook-2 is what actually resolves
it (proving it correctly resumed there) - confirmed failing pre-fix (hook-1 alone
finishes the transition, since pre-fix `rearmPendingWaits` always re-armed the chain's
first hook).

**Data-model change, stated explicitly**: this required two new fields -
`LifecycleHook.Sequence` (chain ordering) and `Instance.LifecycleHookName` (chain
position, for restore) - neither of which exists on AWS's wire types; both are
internal bookkeeping only.

### bd gopherstack-02ue (2026-08-09): Customized\*MetricSpecification, InstanceRequirements.BaselinePerformanceFactors

Closed the two items `gopherstack-2uti` deferred, plus a third, related gap found
while auditing the first: all four `Customized*MetricSpecification` variants this
service accepts were either unparsed or (in `TargetTrackingConfiguration.
CustomizedMetricSpecification`'s case) not even wired to a form key at all - a dead
`ScalingPolicy.CustomizedMetricSpecification string` field existed but nothing ever
set it from `vals`. All verified against `aws-sdk-go-v2/service/autoscaling@v1.70.4`'s
`serializers.go`/`deserializers.go` before writing any parsing code, per the
`parsePredictiveScalingConfiguration` precedent from `gopherstack-2uti`.

**The four variants and their shared math-expression sub-language.**
`TargetTrackingConfiguration.CustomizedMetricSpecification`
(`types.CustomizedMetricSpecification`, `types.go:587`) and the three predictive-scaling
variants - `PredictiveScalingMetricSpecification.Customized{Load,Scaling,Capacity}
MetricSpecification` (`types.PredictiveScalingCustomized{Load,Scaling,Capacity}Metric`,
`types.go:2625/2638/2651`, each just `{MetricDataQueries []MetricDataQuery}`) - all
bottom out in the same CloudWatch metric-data-query shape, duplicated by smithy-codegen
under two Go type names for what is structurally one shape:
`types.MetricDataQuery`/`types.MetricStat` (`types.go:2268`/`2345`, no `Period`) for the
predictive-scaling variants, and `types.TargetTrackingMetricDataQuery`/
`types.TargetTrackingMetricStat` (`types.go:3400`/`3459`, has `Period`) for the
target-tracking variant. Both nest `types.Metric` (`types.go:2182`:
`MetricName`/`Namespace`/`Dimensions []MetricDimension`) and `types.MetricDimension`
(`types.go:2314`: `Name`/`Value`). This bottoms out cleanly - `Expression` is a plain
string referencing other queries' `Id`s, not another nested structure - so it was
modelled to full depth rather than partially, per the "worse than absent" rule: models
`MetricDataQuery`, `MetricDataStat`, `MetricRef`, `MetricDimension`,
`CustomizedMetricSpecification` (the legacy pre-metric-math shape - `MetricName`/
`Namespace`/`Dimensions`/`Statistic`/`Unit`/`Period` - and the newer `Metrics`
metric-data-query list, both accepted since AWS's mutual-exclusivity rule is its own
validation, not the parser's), and `CustomMetricQueries` (the `{MetricDataQueries}`
wrapper shared structurally by all three predictive-scaling variants).

**Exact query-protocol flattening**, confirmed against `serializers.go`, not guessed:

- `TargetTrackingConfiguration.CustomizedMetricSpecification.{MetricName,Namespace,
  Statistic,Unit,Period}` are flat scalars (`serializers.go:4985`
  `awsAwsquery_serializeDocumentCustomizedMetricSpecification`).
- `...CustomizedMetricSpecification.Dimensions.member.N.{Name,Value}` - standard
  `.member.`-flattened list (`serializers.go:5767`).
- `...CustomizedMetricSpecification.Metrics.member.N.{Id,Expression,Label,Period,
  ReturnData}` - standard `.member.`-flattened list (`serializers.go:6508`); this is
  the exact key path the issue's prompt named as the worked example
  (`...Metrics.member.1.Id`), confirmed correct.
- `...Metrics.member.N.MetricStat.{Stat,Period,Unit}` plus
  `...MetricStat.Metric.{MetricName,Namespace}` and
  `...MetricStat.Metric.Dimensions.member.M.{Name,Value}` (`serializers.go:6559`,
  `5680`, `5750`).
- `PredictiveScalingConfiguration.MetricSpecifications.member.N.Customized
  {Load,Scaling,Capacity}MetricSpecification.MetricDataQueries.member.M.*` follows the
  identical shape one level down (no `Period` on this variant's `MetricDataQuery`/
  `MetricStat` - `serializers.go:5704/5716/5789`, confirmed by their absence in those
  serializer bodies, not by omission from mine).

**`InstanceRequirements.BaselinePerformanceFactors`** (`types.
BaselinePerformanceFactorsRequest`, referenced `types.go:1267`/`2582`) nests
`CpuPerformanceFactorRequest.References []PerformanceFactorReferenceRequest`
(`types.go:550`/`2468`) - small and fully bounded, modelled in full. Its wire shape is
a genuine outlier worth flagging for the next person: `References` serializes under the
**singular** key `Reference`, not `References`
(`serializers.go:4971` `awsAwsquery_serializeDocumentCpuPerformanceFactorRequest`), and
that list is wrapped in `item`, not `member`
(`serializers.go:5918` `awsAwsquery_serializeDocumentPerformanceFactorReferenceSetRequest`)
- every other list in this handler uses `member`. Confirmed the same on the response
(deserializer) side (`deserializers.go:10587`/`16003`). Full flattened key:
`MixedInstancesPolicy.LaunchTemplate.Overrides.member.N.InstanceRequirements.
BaselinePerformanceFactors.Cpu.Reference.item.M.InstanceFamily`.

**Operations checked, one by one** (grepped every `api_op_*.go` for
`TargetTrackingConfiguration`/`PredictiveScalingConfiguration`/`MixedInstancesPolicy`/
`InstanceRequirements` rather than assuming): `PutScalingPolicy` and `DescribePolicies`
are the only operations carrying `TargetTrackingConfiguration`/
`PredictiveScalingConfiguration` (fixed, both directions).
`CreateAutoScalingGroup`/`UpdateAutoScalingGroup` and `DescribeAutoScalingGroups` are
the only operations reaching `InstanceRequirements` (via `MixedInstancesPolicy.
LaunchTemplate.Overrides`; fixed, both directions). `PutWarmPool` does **not** carry
`InstanceRequirements` - warm pools have no launch-template-override mechanism of their
own, confirmed by its absence from `api_op_PutWarmPool.go`.

**Verification.** `handler_customized_metrics_test.go` drives all four Customized
variants plus `BaselinePerformanceFactors` through a real `aws-sdk-go-v2` client against
an `httptest` server (not hand-built form bodies - the campaign's own "27 such tests
found" warning is precisely about a test that can't see this bug class because it
shares the handler's wrong parse). Each of the four new round-trip tests was run first
against a pristine pre-fix worktree and confirmed to fail (nested body silently
dropped, top-level 200 OK); all four pass against the fix. `persistence_test.go` gained
two more `TestInMemoryBackend_Persistence` cases (`CustomizedMetricSpecification` and
`BaselinePerformanceFactors` survive `Snapshot`/`Restore`) - both structs are stored
directly by `store.Table`'s JSON-marshal-the-real-struct path (no hand-maintained DTO
in `persistence.go`), so the new `omitempty` fields round-trip automatically; the tests
exist to prove that, not to work around a gap. No `autoscalingSnapshotVersion` bump -
purely additive fields.

**Not touched.** `ScalingPolicy.CustomMetricSpec` (`models.go`, a second, entirely
unrelated dead `string` field sitting next to the one this pass fixed) is still
unreferenced anywhere in the handler. Flagged here rather than removed or wired up -
outside this issue's scope and its intended shape is unclear (unlike
`CustomizedMetricSpecification`, there's no AWS field it obviously maps to).

### Parity-3 sweep (2026-07-23): scheduler, 7 CreateASG/UpdateASG fields, ledger correction

This pass found the prior ledger's `gaps` list had drifted from reality in two places:
the "ASG->EC2 real instance provisioning" gap (bd `gopherstack-8sk`) and the
"ASG/ECS->ELBv2 target registration" gap (bd `gopherstack-18k`) were both listed as
"NOT fixed this pass per scope", but both bd issues were actually already `CLOSED`
(2026-07-12) and both `EC2Launcher` (`ec2_launch.go`) and `ELBv2TargetRegistrar`
(`elbv2_targets.go`) are present and wired into the scale-out/scale-in/attach/detach
paths. Grepped for the adapter types and their call sites to confirm before correcting
the ledger - per the campaign's "if PARITY.md counts don't match reality, independently
field-diff and record what you verify" instruction, not just trusting the bd close
reasons. Moved both from `gaps` to `families` as `ok`.

Similarly, bd `gopherstack-9wo` (terminate-lifecycle-hook gating on the
desired-capacity-driven scale-in path) was still `OPEN` in the tracker, but reading
`auto_scaling_groups.go` shows `applyScaleIn`/`terminationCapacityPreset` fully
implementing exactly what the 2026-07-12 re-audit pass's notes (below) describe. The
code was correct; only the bd issue's status was stale. Closed it this pass rather than
re-doing already-complete work.

**Real new work this pass:**

1. **Scheduled-action background scheduler** (bd `gopherstack-6ys`, the one gap in the
   prior ledger with a live, still-open bd id describing a genuine missing feature, not
   ledger drift). Added `scheduled_action_cron.go` (a 5-field Unix-cron parser -
   `minute hour day-of-month month day-of-week`, matching AWS's documented
   `ScheduledUpdateGroupAction.Recurrence` format) and
   `scheduled_action_scheduler.go` (`ScheduledActionScheduler`, wired as a
   `service.BackgroundWorker` via `pkgs/worker.SingleRun` in `handler.go`'s
   `StartWorker`/`Shutdown`, matching the exact lifecycle shape secretsmanager's
   rotation scheduler and every janitor-style worker in this codebase already use).
   Deliberately reuses `applyUpdateCapacityLocked` (the same helper
   `UpdateAutoScalingGroup` calls) to apply a due action's MinSize/MaxSize/
   DesiredCapacity, so scheduled-action capacity changes get identical validation
   behavior to a manual `UpdateAutoScalingGroup` call for free, rather than
   duplicating (and risking diverging from) that logic - the same lesson the prior
   pass's `ExecutePolicy`/`applyDesiredCapacityChange` fix already established for this
   service. A new `ScheduledAction.LastExecutedTime` field (internal bookkeeping only,
   not projected onto the `DescribeScheduledActions` XML response, since AWS's real
   wire type has no equivalent) prevents a one-time action from refiring and prevents a
   since-invalid recurring action from busy-looping every tick forever (it still logs a
   warning and stamps `LastExecutedTime` so it doesn't retry the same occurrence
   indefinitely - see `fireScheduledActionLocked`).

2. **7 previously-unwired `CreateAutoScalingGroupInput`/`UpdateAutoScalingGroupInput`
   fields**: `AvailabilityZoneDistribution`, `AvailabilityZoneImpairmentPolicy`,
   `CapacityReservationSpecification`, `DeletionProtection`, `InstanceLifecyclePolicy`,
   `InstanceMaintenancePolicy`, `SkipZonalShiftValidation`. Field-diffed each nested
   type and its awsquery-serialized param names against
   `aws-sdk-go-v2/service/autoscaling/types` and `serializers.go`/`deserializers.go`
   directly (via `go doc` + reading the generated serializer functions) rather than
   guessing param names from the field names alone - this caught that the real wire
   field is `CapacityReservationIds` (matching `CapacityReservationTarget`'s Go SDK
   field name exactly, no "ID" initialism), which an early pass of this change
   accidentally renamed to `CapacityReservationIDs` while fixing a `revive` lint
   warning on the *Go identifier* (a legitimate rename) before catching that the *wire
   string* used in `parseMembers`/the XML tag must NOT follow that rename. Fixed by
   keeping the Go field named `CapacityReservationIDs` (satisfies `revive`) while the
   `parseMembers` prefix and the `xml:"..."` tag stay `CapacityReservationIds` (matches
   the real wire byte-for-byte). Flagging this here because it is exactly the kind of
   near-miss the next auditor should watch for: an identifier-hygiene lint fix silently
   corrupting a wire-format string literal that happens to share the identifier's name.
   `DeletionProtection` is the one field with real behavioral impact beyond store-and-
   echo: `DeleteAutoScalingGroup` now gates on it (`prevent-all-deletion` blocks every
   delete, `prevent-force-deletion` blocks only `ForceDelete=true`), matching real AWS's
   `ResourceInUseFault` (`ErrorCode()` is `"ResourceInUse"`, not `"ResourceInUseFault"` -
   read the SDK's generated `errors.go` to confirm, don't assume the Go type name is the
   wire code). `SkipZonalShiftValidation` is accepted and stored but never projected on
   `DescribeAutoScalingGroups`, because real AWS's `types.AutoScalingGroup` response
   type has no such field either (confirmed via `go doc`) - it is a one-time
   launch/update validation-bypass flag, not a persistent group attribute.

3. **Removed all 7 banned complexity nolints** (`cyclop`/`gocognit`/`funlen` across
   `handler_scaling_policies.go`, `handler_auto_scaling_groups.go` x2,
   `instances.go`, `auto_scaling_groups.go` x2, `handler_launch_configurations.go`) by
   extracting focused helper functions (e.g. `applyUpdateCapacityLocked`/
   `applyUpdateLaunchSourceFields`/`applyUpdateScalarFields`/`applyUpdatePolicyFields`
   replacing one large `UpdateAutoScalingGroup`; `parseCreateASGSizeFields`/
   `applyUpdateASGSizeFields`/`applyUpdateASGBoolFields` replacing repetitive
   parse-and-check blocks with small param-table-driven loops). Zero
   `golangci-lint` issues after (was already near-zero; the field-table-driven helpers
   this decomposition introduced initially tripped `govet fieldalignment` on their
   anonymous struct literals - fixed by reordering fields, not suppressing).

### Re-audit pass (2026-07-12): scale-in lifecycle-hook gating fix

This pass found no local drift under `services/autoscaling/` since ce30166a (the
commit that actually authored this ledger - the previously-recorded
`last_audit_commit: d0ebe979` was not an ancestor of HEAD, so ce30166a was used as
baseline per the re-audit protocol) and no `aws-sdk-go-v2/service/autoscaling`
dependency bump (still pinned at v1.64.2 in `go.mod`/`go.sum`), even though a sibling
commit in this repo's history bumped other Go/UI dependencies. All `ok` rows above
were therefore trusted unchanged and not re-verified wire-shape-by-wire-shape.

One item explicitly called out in the prior pass's `gaps` list (and filed as bd
`gopherstack-9wo`) was fixed this pass: a registered `EC2_INSTANCE_TERMINATING`
lifecycle hook gated instance removal in `TerminateInstanceInAutoScalingGroup`, but
NOT in the desired-capacity-driven scale-in path shared by `SetDesiredCapacity`,
`UpdateAutoScalingGroup`, and `ExecutePolicy` (all three route through
`applyDesiredCapacityChange`). That path (`services/autoscaling/backend.go`, the old
`removeUnprotectedInstances` helper) always removed instances from
`g.Instances`/`b.instanceIndex` immediately and unconditionally, regardless of any
configured terminating hook - the exact "disguised stub" class this service's ledger
has previously flagged (state mutated, but the one config knob that should have
changed behavior was silently ignored).

Fixed by replacing `removeUnprotectedInstances` with `(*InMemoryBackend).applyScaleIn`,
which keeps the original protected-instance selection algorithm (remove from the end,
skip `ProtectedFromScaleIn`, stop short of target if everything eligible is
protected - now also skipping instances already in `Terminating:Wait` so a second
scale-in call while one is still pending doesn't double-select it) but branches on
whether the group has an active terminating hook:

- **No hook**: unchanged behavior - instances are removed from `g.Instances` and
  `b.instanceIndex` immediately.
- **Hook present**: selected instances are NOT removed. Each is transitioned to
  `Terminating:Wait` in place (staying in `g.Instances`, consistent with the
  `TerminateInstanceInAutoScalingGroup` gating path and the "Traps for the next
  auditor" note below) and a heartbeat timer is armed via the existing
  `armLifecycleWait`/`resolveLifecycleWait`/`finishTermination` machinery, exactly
  like the single-instance path.

The one real complication (the reason the prior pass deferred this): unlike
`TerminateInstanceInAutoScalingGroup`, where `DesiredCapacity`/`MinSize` are only
decremented once the wait resolves (`ShouldDecrementDesiredCapacity`), the
desired-capacity-driven path sets `g.DesiredCapacity = newDesired` **immediately, up
front**, before any instance is actually removed or gated (`applyDesiredCapacityChange`,
the assignment precedes the `switch`). Reusing `finishTermination`'s existing
decrement-or-replace disposition would have double-decremented (or wrongly launched a
replacement to backfill the already-lowered target) once the wait resolved. Fixed by
generalizing the previously-boolean `pendingHookAction.ShouldDecrement` into a
three-way `terminationDisposition` enum (`terminationReplace` / `terminationDecrement`
/ `terminationCapacityPreset`), and giving scale-in-originated waits
`terminationCapacityPreset`: `finishTermination` removes the instance and does
nothing further to capacity bookkeeping for that disposition, since
`applyDesiredCapacityChange` already applied the target capacity before the wait was
even armed. `terminationReplace` is the enum's zero value, preserving the exact
existing fallback behavior for `rearmPendingWaits` (Restore-time re-arming, which
never persisted the original disposition and defaults to the replace behavior, as
before this change).

Net effect: `DescribeAutoScalingGroups` immediately reflects the new
`DesiredCapacity` after a scale-in call (matching real AWS - the target is accepted
immediately), while the actual instance count/`Terminating:Wait` state lags until the
hook resolves, exactly mirroring the single-instance-termination gating path that was
already correct. Covered by new tests: `Test_LifecycleHookGatesDesiredCapacityScaleIn`
(`parity_b_test.go`, full HTTP round-trip through `SetDesiredCapacity` +
`CompleteLifecycleAction`) and two new subtests of
`TestInMemoryBackend_SetDesiredCapacity` (`backend_test.go`) covering the no-hook
immediate-removal path and scale-in-protection interaction at the unit level.

### The lifecycle-hook fix in detail (highest-value finding this pass)

Prior-sweep notes flagged "lifecycle-hook timeout goroutines" as CRITICAL. The
investigation this pass found the real bug was worse than a leak: `pendingHookTokens`,
its `*time.Timer` field, `cleanupHookTimers`, and `expireHookAction` all existed and
looked plausible, but **nothing anywhere ever inserted an entry into
`pendingHookTokens`**. `PutLifecycleHook` stored the hook; `CompleteLifecycleAction`
and `RecordLifecycleActionHeartbeat` only ever *looked up* an entry that could never
exist. The net effect: creating a lifecycle hook had zero effect on any instance
transition. New instances always went straight to `InService`; terminated instances
were always removed immediately. This is the "disguised stub" pattern called out in
`parity-principles.md` #4 - the code looked real (validated params, stored state,
returned 200) but never touched the one thing lifecycle hooks exist for.

Fixed by wiring real gating into every instance-creating and instance-terminating
code path:
- launch gating: `CreateAutoScalingGroup` (initial instances, only relevant once
  `LifecycleHookSpecificationList` was also wired up - see below),
  `applyDesiredCapacityChange` (scale-out, shared by `SetDesiredCapacity`,
  `UpdateAutoScalingGroup`, and now `ExecutePolicy`), `LaunchInstances`, and the
  replacement-instance branch of `TerminateInstanceInAutoScalingGroup`.
- terminate gating: `TerminateInstanceInAutoScalingGroup` only (see gaps: the
  desired-capacity-driven scale-in path in `applyDesiredCapacityChange` was
  deliberately left as immediate removal - see below).
- `CompleteLifecycleAction` and `RecordLifecycleActionHeartbeat` now resolve/re-arm a
  real pending action, looked up by token OR by `(group, hook, instanceId)` since AWS
  allows either.
- Heartbeat timeout expiry calls the same resolution path with the hook's
  `DefaultResult`, so an abandoned wait behaves identically whether it was resolved
  explicitly or by timeout.

**Scope boundary, stated explicitly**: terminate-hook gating was *not* wired into the
`applyDesiredCapacityChange` scale-in path (used by `SetDesiredCapacity` decreasing,
`UpdateAutoScalingGroup`, `ExecutePolicy` scale-in). That path currently still removes
instances immediately regardless of a registered terminating hook. Reason: unlike
`TerminateInstanceInAutoScalingGroup` (a single, self-contained instance removal),
scale-in there interacts with `removeUnprotectedInstances`'s batch compaction and the
desired-capacity bookkeeping for potentially many instances at once; deferring N
removals concurrently, each independently completable/timeoutable, while keeping the
group's effective capacity accounting consistent for concurrent
`DescribeAutoScalingGroups` callers, is a meaningfully bigger state machine than the
single-instance case and was judged too risky to rush. Filed as a known, explicit gap
above rather than silently left broken.

**ABANDON semantics** (launching case fixed bd `gopherstack-2uti`/b7d3a8485; chaining
fixed bd `gopherstack-9tqg` - see dated section below): for a *launching* hook, ABANDON
terminates the pending instance and relaunches a replacement to restore
`DesiredCapacity`, gated by the same launch chain from its first hook. ABANDON at any
position in either chain now also short-circuits every hook still to come, matching
AWS's documented "abandon stops any remaining actions, such as other lifecycle hooks".
For a *terminating* hook, both CONTINUE and ABANDON eventually let the instance
terminate (you cannot veto a termination via a terminating lifecycle hook) - CONTINUE
differs from ABANDON only in whether the next chained hook, if any, gets to run first.

**Multiple-hooks-per-transition chaining** (fixed bd `gopherstack-9tqg`, deferred from
bd `gopherstack-2uti`/b7d3a8485 - see dated section below for the full writeup):
hooks on the same transition now form AWS's documented ordered chain instead of only
the first ever being armed.

**Restore/persistence**: `pendingHookTokens` (in-flight timers) are intentionally not
part of `backendSnapshot` - a `*time.Timer` can't be serialized. `Restore()` sweeps
every instance left in `Pending:Wait`/`Terminating:Wait` by a restored snapshot and
re-arms a timer for it. Without this, an instance restored mid-wait would be stuck in
that state forever, since nothing would ever call `CompleteLifecycleAction`/hit a
timeout for it. Since bd `gopherstack-9tqg` (see dated section below), the re-armed
hook is the one the instance was *actually* waiting on
(`Instance.LifecycleHookName`, itself part of the persisted group so it survives the
round-trip) rather than always the chain's first hook, so a group restored mid-chain
resumes at the right position instead of restarting or getting stuck.

### Other wire-shape bugs fixed this pass

- **`LaunchInstances` param typo**: the handler read `vals.Get("DesiredCapacity")`.
  The real field name (verified against `LaunchInstancesInput` and its awsquery
  serializer) is `RequestedCapacity`. Any real SDK client always sends
  `RequestedCapacity`, so gopherstack's handler saw an empty string every time and
  fell back to launching exactly 1 instance regardless of what was requested.
- **`LaunchInstances` output shape**: `LaunchInstancesOutput.Instances` is
  `[]InstanceCollection` (grouped by `AvailabilityZone`/`InstanceType`, each carrying
  `InstanceIds []string`), NOT a flat list of per-instance
  `LifecycleState`/`HealthStatus` records (that shape belongs to
  `DescribeAutoScalingGroups`/`DescribeAutoScalingInstances`). The handler was reusing
  the wrong XML type. Fixed with a dedicated `xmlInstanceCollection` type and a
  grouping helper.
- **`LaunchInstances` never indexed its instances**: the backend method appended to
  `g.Instances` but never touched `b.instanceIndex`, so an instance launched this way
  could never be found by `TerminateInstanceInAutoScalingGroup` (which looks up
  purely via `instanceIndex`). Same bug existed in the replacement-instance branch of
  `TerminateInstanceInAutoScalingGroup` itself; both fixed.
- **`MixedInstancesPolicy` silently dropped end-to-end**: the backend input struct and
  the `AutoScalingGroup` model both already had a `MixedInstancesPolicy` field, but
  neither `CreateAutoScalingGroup` nor `UpdateAutoScalingGroup`'s handlers ever parsed
  it from the request, and `xmlAutoScalingGroup` didn't even have a field for it in
  the response projection. A request specifying a mixed-instances policy (spot+
  on-demand mixes, very common via Terraform) was accepted with 200 OK and then
  quietly discarded. Fixed: full parse (launch template + overrides + instances
  distribution) on both Create/Update, full XML projection on Describe.
- **`LifecycleHookSpecificationList` never parsed**: `CreateAutoScalingGroupInput`
  (the real AWS one) lets you register lifecycle hooks atomically with group
  creation - this is exactly the wire shape Terraform's `aws_autoscaling_group`
  `initial_lifecycle_hook` block uses. Completely unhandled before this pass; a group
  created with initial hooks would come up with **no hooks at all**. Fixed, and wired
  into the new launch-hook gating so the group's own initial instances are correctly
  gated by a hook registered at creation time.
- **`TrafficSources` never parsed on `CreateAutoScalingGroup`**: only
  Attach/DetachTrafficSources touched this field; Create silently dropped an inline
  `TrafficSources` list. Fixed (reuses the existing `parseTrafficSources` helper).
- **`PutScheduledUpdateGroupAction`/`BatchPutScheduledUpdateGroupAction` dropped
  `StartTime`/`EndTime`**: the backend model and the `DescribeScheduledActions` XML
  projection both already had `StartTime`/`EndTime` fields, but neither handler parsed
  them from the request - the entire point of a "scheduled" action (when it fires)
  was silently discarded on every call. Fixed (parses AWS query-protocol DateTime,
  i.e. RFC3339/ISO8601).
- **`ExecutePolicy` ignored `StepScaling` entirely**: regardless of `PolicyType`, the
  handler always used the flat `ScalingAdjustment`/`AdjustmentType` fields. Real
  `ExecutePolicy` requires `MetricValue`/`BreachThreshold` for a `StepScaling` policy
  and uses `(MetricValue-BreachThreshold)` to select which `StepAdjustment` interval's
  `ScalingAdjustment` applies. Fixed: parses both fields, validates they're required
  for `StepScaling`, and selects the matching step interval
  (`MetricIntervalLowerBound`/`UpperBound`, nil meaning unbounded, exactly as AWS
  documents it).
- **`ExecutePolicy` scale duplicated (and diverged from) `SetDesiredCapacity`'s
  logic**: it called `adjustInstances` directly, bypassing `SuspendedProcesses`
  checks, scale-in protection (`removeUnprotectedInstances`), and `instanceIndex`
  maintenance that `applyDesiredCapacityChange` already does correctly. Fixed by
  routing through the shared helper.
- **`PutLifecycleHook` dropped `NotificationMetadata`**: parsed nowhere despite being
  a plain top-level request field and already present on both the backend model and
  the XML response type. Fixed.
- **`PutScalingPolicy`/`DescribePolicies` dropped `MetricAggregationType`** (request
  and response) and `DescribePolicies` never returned `MinAdjustmentStep`. Fixed.
- **`GetPredictiveScalingForecast` returned an all-empty response** missing the
  required `UpdateTime` field entirely and shaping `LoadForecast` as `[]string`
  (nowhere close to the real `[]LoadForecast` struct list). A full
  `PredictiveScalingMetricSpecification` projection is out of scope (see gaps), but
  the response now includes a real `UpdateTime` and a real, non-empty, correctly-
  shaped `Timestamps`/`Values` series (naive flat projection at current
  `DesiredCapacity` - explicitly documented as a simplification, not a hidden stub).

### Traps for the next auditor

- `Instance.LifecycleState` values `"Pending:Wait"` and `"Terminating:Wait"` are real
  AWS enum values (`types.LifecycleState`), not placeholders - don't "simplify" them
  back to `InService`/removed without re-reading this Notes section.
- An instance appearing in `g.Instances` with `LifecycleState="Terminating:Wait"`
  still counts toward `len(g.Instances)` but the group's `DesiredCapacity`/`MinSize`
  bookkeeping intentionally has NOT yet been decremented for it - that's deferred to
  `finishTermination`. Don't "fix" an apparent capacity/instance-count mismatch during
  a wait without checking for a `*:Wait` instance first.
- `ExecutePolicy` calling `b.applyDesiredCapacityChange` instead of its own
  `adjustInstances` call is intentional (bug fix, not a regression) - see above.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed InternalFailure**: `RouteMatcher` calls
`httputils.ReadBody` to inspect the form body and decide ownership; on a read failure
(oversized body) it returned `false`, so the router found no owner and answered a
generic 404 instead of ever reaching `Handler()`. Autoscaling is one of 13 services (of
17 total, per gopherstack-3a8t's survey; elasticache/docdb/neptune already fixed) sharing
this shape, all form-urlencoded query-protocol services distinguished from each other only
by the body's `Action`/`Version` -- so claiming unconditionally on a read failure would
misroute a sibling service's oversized body. **The fix**: `RouteMatcher` now falls back to
`service.MatchesUserAgentMarker(r.Header, "api/autoscaling")` (verified against the pinned
`autoscaling@v1.70.4/api_client.go:641` `AddSDKAgentKeyValue` call) only on the `ReadBody`
failure branch, leaving the readable-body `Version`/`Action` matching untouched. Also
migrated `ExtractOperation`/`ExtractResource`/`Handler()` off `r.ParseForm()` onto
`httputils.ReadBody`+`url.ParseQuery`: `net/http`'s `ParseForm` caches an empty-but-non-nil
`r.PostForm` after its first failed call, so a second call (the telemetry wrapper's
`ExtractOperation` runs before `Handler()`) would have silently "succeeded" empty instead
of surfacing the read error, per the docdb/neptune precedent (gopherstack-bahs). Proof:
`TestHandler_OversizedBodySurfacesInternalFailure` in `handler_oversized_body_test.go`
drives a real autoscaling SDK client through `service.NewRegistry`/`service.NewServiceRouter`,
confirmed failing pre-fix with `UnknownError`; passes now with `InternalFailure`.
`TestHandler_NormalSizedBodyStillRoutes` is the regression guard. Gates: `go build`,
`go vet`, `gofmt -l` (clean), `go test -race ./services/autoscaling/...` (pass),
`golangci-lint run ./services/autoscaling/...` (0 issues).

## 2026-08-29 -- exhaustive indexed-list/filter-key request-parameter sweep

Every generic indexed-list parse site enumerated against its own operation's
serializer in `autoscaling@v1.70.4` (request-side parameter reads, not the
response-wrapper-key class the 2026-08-29 error/wrapper passes above cover).

**~42 call sites checked by hand this pass, 0 bugs found:** 31 `parseMembers`
call sites (`InstanceIds`/`SecurityGroups`/`ClassicLinkVPCSecurityGroups`/
`LaunchConfigurationNames`/`NotificationTypes`/`AutoScalingGroupNames`/
`AvailabilityZones`/`LoadBalancerNames`/`TargetGroupARNs`/
`TerminationPolicies`/`ScalingProcesses`/`ScheduledActionNames`/
`LifecycleHookNames`/`InstanceRefreshIds`/`Metrics`/`PolicyNames`, each
checked against its own operation's serializer independently -- several keys
like `InstanceIds`/`TargetGroupARNs`/`AvailabilityZones` are read identically
across multiple sibling operations, and each one's own serializer was read
rather than inferred from the first), `parseTags`/`parseResourceTags` (3
sites) plus `parseTagFilters` (already correctly iterating every
`Values.member.M`, not just the first), `parseBlockDeviceMappings`/
`parseEbsBlockDevice`, `parseLifecycleHookSpecifications`,
`parseCapacityReservationTarget`/`parseCapacityReservationSpecification`,
`parseTrafficSources` (Attach+Detach), and `parseBatchScheduledActions`. All
confirmed to use the generic query-protocol `.member.N` wrapper this
handler already assumes, with the field's own `serializeDocument*` function
read in each case rather than pattern-matched by name.

**Not re-derived from scratch this pass** (previously exhaustively verified
against the identical bug class with serializers.go line citations -- see
"bd gopherstack-2uti" and the immediately-following predictive-scaling
section above): the `TargetTrackingConfiguration`/
`PredictiveScalingConfiguration`/`MixedInstancesPolicy.LaunchTemplate.
Overrides[].InstanceRequirements` nested-list machinery in
`handler_scaling_policies.go`/`handler_auto_scaling_groups.go`, including the
`BaselinePerformanceFactors.Cpu.Reference.item.M` singular/`item`-wrapped
outlier that prior pass already caught. Spot-checked
`parseLaunchTemplateOverrides`'s outer `Overrides.member.N` wrapper and the
`CapacityReservationSpecification`/`CapacityReservationTarget` sub-lists this
pass; did not re-walk every leaf field of the ~25-field `InstanceRequirements`
struct a second time.

**Missing feature, left alone (not this bug class):** `DescribeAutoScalingGroups`
never parses its real `Filters` member (confirmed on
`DescribeAutoScalingGroupsInput`); `DescribePolicies` never parses `PolicyTypes`.
Both are parameters never read, not wrong keys.

**Coverage: N-of-N for every generic-helper call site found this pass (73
of 73: 42 freshly checked + the ~31 already covered by the 2026-08-08/02ue
scaling-policy pass, cross-referenced rather than re-verified).** What
remains unchecked by any pass: the handful of pure-scalar object parsers
(`parseInstanceLifecyclePolicy`, `parseInstanceMaintenancePolicy`,
`parseAvailabilityZoneDistribution`, `parseAvailabilityZoneImpairmentPolicy`,
`parseInstancesDistribution`) carry no `.N` indexing at all, so they are
outside this bug class by construction and were not separately audited here.

No code changes in this service this pass -- the enumeration found nothing
to fix.

## 2026-08-29 constraint-parameter sweep (filters/pagination never applied) -- 5 operations fixed

Measured from each op's own Input struct in the pinned SDK (`autoscaling@v1.70.4`): 13 Describe ops
carry `Filters`/a named filter field/`MaxRecords`/`NextToken`. This pass closes the two gaps the prior
pass explicitly flagged and left alone as "not this bug class" (quoted above), plus three more found
by reading every one of the 13 Input structs directly:

- **`DescribePolicies`** (`scaling_policies.go`/`handler_scaling_policies.go`/`interfaces.go`):
  `PolicyTypes` (`api_op_DescribePolicies.go`: "The valid values are SimpleScaling, StepScaling,
  TargetTrackingScaling, and PredictiveScaling") was parsed nowhere -- confirmed exactly the prior
  pass's note. Fixed: `PolicyTypes.member` now filters alongside `PolicyNames`.
- **`DescribeAutoScalingGroups`** (`auto_scaling_groups.go`/`handler_auto_scaling_groups.go`/
  `interfaces.go`): `Filters` wasn't even part of the backend method signature -- confirmed exactly
  the prior pass's note. The Go SDK's `Filter` type carries no closed `Name` enum; the API reference's
  own worked examples are the only place the valid forms are spelled out (`API_DescribeAutoScalingGroups.html`
  Examples 2-3): `tag-key`, `tag-value`, `tag:<key>`, ANDed across filters, each satisfied by any one
  tag on the group. All three forms implemented in `autoScalingGroupMatchesFilters`/
  `groupHasTagMatchingFilter`.
- **`DescribeScalingActivities`** (`activities.go`/`handler_activities.go`): the `Filters` member
  (`Status`, documented "This filter can only be used in combination with the AutoScalingGroupName
  parameter") was never read, and `MaxRecords` truncated the slice with **no `NextToken` returned** --
  results past the cutoff were silently dropped, not paginated. Fixed: `Status` filter applied;
  real `pkgs/page`-backed pagination replaces the truncate, defaulting/capping at the documented 100
  (`api_op_DescribeScalingActivities.go`: "The default value is 100 and the maximum value is 100").
  **Gap left**: `StartTimeLowerBound`/`StartTimeUpperBound` (the other two documented `Filter.Name`
  values) are not applied -- noted in code, not fabricated. **Restriction left unenforced**: the doc's
  "Status can only be used with AutoScalingGroupName" is not rejected when violated (applied
  regardless) -- a permissiveness gap, not a correctness one, left as-is given the added risk of a new
  validation error path outweighing the benefit for a documented-but-unenforced restriction.
- **`DescribeScheduledActions`** (`scheduled_actions.go`/`handler_scheduled_actions.go`): `StartTime`/
  `EndTime` (`api_op_DescribeScheduledActions.go`: "the latest/earliest scheduled start time to
  return... If scheduled action names are provided, this property is ignored") were never read. Fixed:
  both now bound the returned set's `StartTime`, applied only when `actionNames` is empty per the
  documented precedence (matching the existing name-lookup branch this backend already had).
- **`DescribeTrafficSources`** (`traffic_sources.go`/`handler_traffic_sources.go`): `TrafficSourceType`
  (`api_op_DescribeTrafficSources.go`: `elb`/`elbv2`/`vpc-lattice`) was never read. Fixed.

**Confirmed already correct, not touched**: `DescribeTags`'s `Filters` (`handler_tags.go`'s
`parseTagFilters`/`tagMatchesFilters`) was already correctly applied per-tag; `DescribeScheduledActions`'s
`ScheduledActionNames` was already correct.

**CORRECTED 2026-08-30 (gopherstack-zslr)**: the claim two lines above that
`DescribeLaunchConfigurations`'s pagination "were already correct" and that
`DescribeLaunchConfigurations`/`DescribeNotificationConfigurations`/`DescribeAutoScalingInstances`/
`DescribeLoadBalancers`/`DescribeLoadBalancerTargetGroups`/`DescribeWarmPool`/`DescribeInstanceRefreshes`
"already implements [pagination] correctly for each" was wrong -- re-reading each handler directly
(not spot-checked this time) found all ten ignored `MaxRecords`/`NextToken` entirely: every one read
the backend's full result and returned it in one unbounded response, several with a `NextToken` XML
field already declared on the result struct and never populated. `DescribeScalingActivities` above this
note is the one op in the file that legitimately already had correct pagination (it's cited, correctly,
as the pattern to copy). See "MaxRecords/NextToken pagination sweep" below for the fix.

Gates: `go build ./services/autoscaling/...`, `go vet ./...` (repo-wide -- also required a call-site fix
in `/cli_asg_ec2_wiring_test.go`, outside this service, since `DescribeAutoScalingGroups`'s signature
changed), `go test ./services/autoscaling/... -race -count=1` (pass), `golangci-lint run
./services/autoscaling/...` (0 issues after decomposing `DescribeScheduledActions` to clear gocognit --
this repo bans the nolint for that linter). New tests in `list_filter_params_test.go` drive the real
typed SDK client (`assdk.Client`) for every read path under test; fixture setup for the scaling-activity
`InProgress` status and the traffic-source-type cases goes through the backend directly (lifecycle-hook
wait state and raw `TrafficSource` structs are awkward to reach through the SDK's own input validation),
consistent with the narrow exception for setup that doesn't touch the code path being tested.

## 2026-08-30: MaxRecords/NextToken pagination sweep, 10 operations (gopherstack-zslr)

Corrects the false "already implements [pagination] correctly" claim two sections above (see the
CORRECTED note there) for the ten Describe ops that carry `MaxRecords`/`NextToken` on their real Input
(`go doc github.com/aws/aws-sdk-go-v2/service/autoscaling.Describe*Input`, one op at a time) but whose
handlers never read either field: `DescribeLaunchConfigurations`, `DescribeAutoScalingInstances`,
`DescribeScheduledActions`, `DescribeTags`, `DescribeLoadBalancers`, `DescribeLoadBalancerTargetGroups`,
`DescribeNotificationConfigurations`, `DescribeTrafficSources`, `DescribeWarmPool`,
`DescribeInstanceRefreshes`, `DescribePolicies` (11 operations; `DescribeWarmPool` turned out to be a
partial exception, see below). `handler_launch_configurations.go`'s `describeLaunchConfigurationsResult`
already declared a `NextToken` XML field that was never populated -- the tell this campaign has seen
several times now (a shape that promises a cursor the handler never fills in).

All now paginate via `pkgs/page.New` (the repo's generic opaque-index-token pager -- see
`pkgs-catalog.md`: "use instead of hand-rolled NextToken/cursor logic"), matching the existing
`DescribeScalingActivities` reference (not `DescribeAutoScalingGroups`'s older hand-rolled
base64-last-name marker, predating `pkgs/page`). Each op's own documented default/max page size was
read individually (`go doc`, not assumed uniform): `DescribeLoadBalancers`/
`DescribeLoadBalancerTargetGroups` are 100/100; `DescribeAutoScalingInstances`/`DescribeTrafficSources`/
`DescribeWarmPool` are 50/50 (no distinct default documented for the latter two); the other seven are
50/100.

**The two listings that ranged a map with zero sort calls** (flagged going in, confirmed by reading both
before touching either):
- **`DescribeNotificationConfigurations`** (`notifications.go`): account-wide (`groupNames` empty)
  ranged `b.notificationConfigs` (a `map[string][]*NotificationConfiguration]`) directly into the result
  slice. Fixed: sorted by `(AutoScalingGroupName, TopicARN, NotificationType)` -- `NotificationConfiguration`
  has no single-field unique key, but that triple is: `PutNotificationConfiguration` replaces any existing
  config for exactly that combination. Verified end-to-end via the real SDK client (real
  `DescribeNotificationConfigurationsInput.AutoScalingGroupNames` is optional, so the account-wide branch
  is reachable through the typed client, unlike the case below).
- **`DescribeInstanceRefreshes`** (`instance_refreshes.go`): same pattern over
  `b.instanceRefreshes` when `groupName` is empty. Fixed: sorted by `InstanceRefreshID`, a
  `uuid.NewString()` value (`StartInstanceRefreshWithInput`) -- globally unique, no tiebreak needed,
  matching the existing `DescribeScalingActivities` UUID-sort precedent. **Not reachable through the real
  SDK client**: `go doc` confirms `DescribeInstanceRefreshesInput.AutoScalingGroupName` is `*string` with
  "This member is required", so a real client refuses to build the account-wide request that exercises
  this branch at all -- the bug is real (a raw HTTP caller bypassing SDK-side validation can still hit
  it) but untestable through `assdk.Client`. Covered instead by
  `TestDescribeInstanceRefreshes_AccountWide_SortIsDeterministic`, which calls
  `backend.DescribeInstanceRefreshes("", nil)` directly 21 times against the same seeded state and
  asserts identical order every time; the SDK-reachable single-group path (deterministic already, since
  `b.instanceRefreshes[groupName]` is a plain slice, not a map) is covered separately by
  `TestDescribeInstanceRefreshes_SDKRoundTrip_Pagination`, seeded via the existing test-only
  `AddInstanceRefresh` helper to get 25 refreshes onto one group without tripping
  `StartInstanceRefresh`'s one-active-refresh-per-group rule.

**Two more sort-uniqueness gaps found while wiring pagination, not in the original two flagged sites**,
same failure shape (a sort key that's only unique within a group, exposed once an account-wide query
scans every group):
- **`DescribeScheduledActions`** (`scheduled_actions.go`): sorted by `ScheduledActionName` alone when
  `groupName` is empty (`scheduledActionsInTimeRangeLocked` then scans `b.scheduledActions.All()` across
  every group), but `ScheduledActionName` is unique only within a group (`scheduledActions` is keyed by
  `scopedKey(groupName, name)`) -- two different groups can share an action name. Tiebroken with
  `AutoScalingGroupName`.
- **`DescribePolicies`** (`scaling_policies.go`): same shape, sorted by `PolicyName` alone
  (`scalingPolicies` keyed by `scopedKey(groupName, PolicyName)`). Tiebroken with
  `AutoScalingGroupName`. `TestDescribePolicies_SDKRoundTrip_Pagination` seeds all 25 policies on
  distinct groups with the SAME `PolicyName` specifically to force this tie and prove the tiebreak
  makes the pagination cursor deterministic.

Both are timestamp/name-shaped keys admitting ties exactly as the task brief predicted ("A name...
admits ties and needs the id appended"), found by reading each backend method's `sort.Slice` while
wiring its handler's pagination rather than trusting the handler-level fix alone.

**`DescribeWarmPool` is a structural partial exception**, not a full fix like the other nine: real
`DescribeWarmPoolOutput` carries `Instances []types.Instance` (the pool's actual member instances) plus
`NextToken`, but this backend's `WarmPool` model has no instance list at all -- `PutWarmPool` only
stores pool-level config (`MinSize`/`MaxGroupPreparedCapacity`/`PoolState`/`Status`), and nothing
anywhere provisions simulated warm-pool instances into it (confirmed: no `Warmed:`-prefixed
`LifecycleState` anywhere in the package, which is how real AWS represents warm-pool instances within
the ASG's own instance list). `Instances` is therefore always empty, so pagination over it is correctly
a no-op today -- not a bug I could reproduce, and not something to fabricate fixture data for. Fixed the
part that's real: `MaxRecords`/`NextToken` are read and threaded through `pkgs/page.New` (an empty slice)
so a client supplying either doesn't error, and the previously entirely-absent `Instances`/`NextToken`
XML fields were added to the response for wire completeness. Unlike the other nine,
`TestDescribeWarmPool_MaxRecordsNextToken_Wired` does **not** fail against the pre-fix handler (both
versions produce an equivalently-empty/absent `Instances`/`NextToken` on the wire, since there was
nothing to truncate either way) -- it only proves the new plumbing doesn't error, not that it fixes an
observable bug. Genuine warm-pool instance modeling (so this pagination has something real to page over)
is out of scope here; noted as a separate, larger gap.

**Restraint**: `DescribeLoadBalancers`, `DescribeLoadBalancerTargetGroups`, and `DescribeTrafficSources`
are all scoped to a single `AutoScalingGroupName` (not account-wide) and already read from a plain
`[]string`/`[]TrafficSource` slice field on the group (`LoadBalancerNames`/`TargetGroupARNs`/
`TrafficSources`), not a map -- insertion-ordered and already deterministic across calls with no sort
needed. No filter had to move ahead of pagination in this service (unlike the iam sweep referenced in
the task brief): every filter already in these handlers (`DescribeTags`'s `Filters`,
`DescribeTrafficSources`'s `TrafficSourceType`, `DescribePolicies`'s `PolicyNames`/`PolicyTypes`,
`DescribeScheduledActions`'s name/time-range filtering) already runs inside the backend method, before
the handler's new `page.New` call -- there was no pre-existing "paginate then filter" ordering bug to
fix.

Every fix except `DescribeInstanceRefreshes`'s account-wide sort (see above) is proven with a
`TestDescribe*_SDKRoundTrip_Pagination` test in `list_pagination_ignored_test.go`: 25 records seeded,
`MaxRecords`=10, asserts page 1 is full and carries a `NextToken`, the remainder comes back exactly once
across however many follow-up pages with no duplicates, confirmed failing against the pre-fix handler
via a scoped `git stash` of only the ten source files (test file untouched, so it compiles against both
versions) -- 11 of the 12 new tests failed pre-fix as expected;
`TestDescribeWarmPool_MaxRecordsNextToken_Wired` passed both before and after, per the structural
exception above.

No AWS documentation was fetched for this pass (all wire-shape facts came from `go doc` against the
pinned `aws-sdk-go-v2` module and from reading this service's own source), so the security note about an
injected `aws agent-toolkit search-skills` footer in fetched docs does not apply here.

Gates: `go build ./services/autoscaling/...` clean; `go vet ./services/autoscaling/...` clean (repo-wide
`go vet ./...` also clean -- no call-site fix needed in any root `cli_*_test.go`, unlike the constraint-
parameter sweep above); `go test ./services/autoscaling/... -race -count=1 -shuffle=on` -- `ok`;
`golangci-lint run ./services/autoscaling/...` -- `0 issues` (after adding `//nolint:dupl` to
`DescribeLoadBalancers`/`DescribeLoadBalancerTargetGroups`, newly flagged once both shared the same
`page.New` pagination shape -- confirmed pre-existing "different resource types, same list-XML
structure" duplication, not new debt, before suppressing).

### 2026-08-31 (response-element-naming re-verification, gopherstack-uox6 trigger)

Triggered by the rds `DBParameterGroups` bug (`e2a4d084a`): a list field whose per-item
XML wrapper was named for the *status type* (`DBParameterGroupStatus`) where the pinned
deserializer's list decoder matches on the *group* name (`DBParameterGroup`), so the list
decoded as empty for every SDK client despite the emitted XML looking correct on skim.
Asked whether this repo's wrapper-key/nested-shape campaign (gopherstack-6flj/21my) covers
response element naming, or whether it only escaped for rds.

**It covers it, and autoscaling was already fully swept at both layers.** gopherstack-21my's
own notes record: "autoscaling -- both layers verified across all 21 Describe/Get ops and
essentially every nested item type reachable from them (AutoScalingGroup incl.
MixedInstancesPolicy/... , Instance, AutoScalingInstanceDetails, TagDescription/ResourceTag,
ScalingPolicy incl. .../CustomizedMetricSpecification/PredefinedMetricSpecification,
ScheduledUpdateGroupAction, LifecycleHook, LaunchConfiguration incl. .../InstanceMonitoring,
Activity, NotificationConfiguration, InstanceRefresh incl. RefreshPreferences,
WarmPoolConfiguration, LoadBalancerState, LoadBalancerTargetGroupState, TrafficSourceState,
CapacityForecast/LoadForecast). All clean at both layers -- no wrong-key or wrong-nesting
bugs found." That predates the rds bug and used the identical method (read each op's own
`awsAwsquery_deserializeDocument*`/`*List` function, compare element names).

This pass independently re-spot-checked the exact shape class that bit rds -- a list field
nested inside a larger struct, checking the *wrapping* element name each list decoder
matches on, not just top-level keys -- against `aws-sdk-go-v2/service/autoscaling@v1.70.4`
(matches `go.mod`): `TargetGroupARNs`, `LoadBalancerNames`, `SuspendedProcesses`,
`EnabledMetrics`, `TrafficSources` (deserializers.go:18654/14208/18414/11001/19294) all
match on `strings.EqualFold("member", t.Name.Local)`, and `auto_scaling_groups.go`'s
`xmlStringValueList`/`xmlSuspendedProcessList`/`xmlTrafficSourceList`/`xmlEnabledMetricList`
all emit `xml:"member"` per item -- correct. No status-shaped list (the rds bug's specific
shape, a list of `*Status` structs wrapped under a non-`member` name) exists anywhere in
this service's deserializers -- confirmed by `grep -n
"func awsAwsquery_deserializeDocument.*StatusList\b"` against `deserializers.go`, zero
matches. **Zero new bugs found; nothing changed in this service.** `go build`, `go vet`
(repo-wide, clean), `go test -race ./services/autoscaling/...` all pass on the unmodified
tree. No AWS documentation was fetched this pass (all facts came from the pinned module
cache and existing repo source).
