---
service: iot
sdk_module: aws-sdk-go-v2/service/iot@v1.77.4
sibling_sdk_modules: [aws-sdk-go-v2/service/iotdataplane@v1.35.0]  # device-shadow ops (Get/Update/DeleteThingShadow, ListNamedShadowsForThing); see device_shadows family
last_audit_commit: 2a94081753c196de1bbad6b25b8f9b9a90dce321  # pass #4; pass #5 below is uncommitted at write time
last_audit_date: 2026-08-29
overall: A            # 2026-08-29 (wrapper-key-sweep, constraint-not-honoured class): pagination/
                       # filter/sort constraints across the certificate, policy, authorizer,
                       # role-alias, stream, and audit-suppression families were never read or
                       # never plumbed through at all. ListCertificates/ListCACertificates/
                       # ListCertificatesByCA/ListCertificateProviders never applied AscendingOrder
                       # or pageSize/marker pagination (ListCACertificates also never applied its
                       # TemplateName filter); ListPolicies read the WRONG pagination query keys
                       # (maxResults/nextToken instead of the real pageSize/marker) and never sorted
                       # by creation date; ListAuthorizers/ListRoleAliases/ListStreams never applied
                       # AscendingOrder or pagination at all (ListAuthorizers also never applied its
                       # Status filter); ListAuditSuppressions read NO request fields whatsoever
                       # (CheckName/ResourceIdentifier/MaxResults/NextToken/AscendingOrder all
                       # silently ignored). Fixing ListPrincipalPolicies' AscendingOrder surfaced a
                       # separate, more severe wire bug found along the way: it read the Principal
                       # from the WRONG header (X-Amzn-Principal instead of the real
                       # X-Amzn-Iot-Principal), so every real client's request principal was
                       # silently dropped and the op always returned empty -- a pre-existing test
                       # sent the same wrong header and could never have caught it. ListPolicyPrincipals'
                       # AscendingOrder was left unimplemented and documented as a genuine structural
                       # gap: it returns bare principal strings with no per-attachment creation
                       # timestamp anywhere in this backend to sort by. ListOutgoingCertificates was
                       # verified already correct. See the ops: entries below for full detail.
                       #
                       # --- 2026-08-21 (gopherstack-c8ge) --- fixed two singleton-configs-with-no-Create-op
                       # merge bugs -- UpdateAccountAuditConfiguration and UpdatePackageConfiguration both
                       # wholesale-replaced a stored map with whatever the request carried instead of
                       # merging per key, so naming one check/field in a call silently reset every
                       # other one a prior call had set. See the two op rows.
                       # 2026-07-25 pass #4 (this pass): closed the ONE remaining partial
                       # family, security_profiles, the sole reason pass #3 stayed at A-.
                       # CreateSecurityProfile silently dropped Behaviors/AlertTargets/
                       # AdditionalMetricsToRetain/AdditionalMetricsToRetainV2/
                       # MetricsExportConfig entirely (types.CreateSecurityProfileInput,
                       # v1.76.0) -- SecurityProfile never persisted any of them. All five
                       # are now modeled (extending, not duplicating,
                       # ValidateSecurityProfileBehaviors' existing SecurityProfileBehavior/
                       # SecurityProfileBehaviorCriteria shapes per this pass's brief) and
                       # wired end-to-end: request parsing, backend storage, response wire
                       # shape (field-diffed against DescribeSecurityProfileOutput/
                       # UpdateSecurityProfileOutput), and persistence (SecurityProfile
                       # round-trips through the existing store.Table[SecurityProfile]
                       # registry unchanged -- no persistence.go wiring gap, since that
                       # layer already marshals the full struct). UpdateSecurityProfile was
                       # rebuilt from a single description-only field into the real
                       # UpdateSecurityProfileInput shape, including ExpectedVersion's
                       # optimistic-lock semantics and every DeleteX-flag-vs-field mutual-
                       # exclusion rule (previously entirely unmodeled). Closing this also
                       # unblocked ListActiveViolations/ListViolationEvents'
                       # behaviorCriteriaType filter (device_defender family), now
                       # implemented by resolving each violation's owning security
                       # profile's stored Behaviors live. security_profiles is now `ok`; see
                       # its families: entry and the new Scope-of-this-pass note below for
                       # detail. job_and_jobtemplate and device_defender were already closed
                       # by pass #3, below (kept verbatim for history).
                       #
                       # This pass also did the explicitly-required routing sweep ("check
                       # routing while you're there") for every security-profile op, driven
                       # through a real generated AWS SDK v2 client against the actual
                       # service.Router path rather than h.Handler() directly -- three prior
                       # passes each found real routing bugs this way for other op families,
                       # and this family had never been checked this way before. It found two
                       # MORE previously-undiscovered bugs specific to security_profiles: (1) a
                       # RouteMatcher-whitelist gap identical in kind to ListJobs'/the
                       # job-template/mitigationaction families' own prior-pass gaps --
                       # ListSecurityProfiles (plain "/security-profiles", no trailing slash)
                       # and ListSecurityProfilesForTarget ("/security-profiles-for-target")
                       # were both entirely unreachable by a real client despite op dispatch
                       # itself being correct; (2) three wire-shape key-name bugs on
                       # ListSecurityProfiles/ListTargetsForSecurityProfile/
                       # ListSecurityProfilesForTarget's list-entry shapes (invented/full keys
                       # in place of the real, shortened SecurityProfileIdentifier/
                       # SecurityProfileTarget/SecurityProfileTargetMapping keys). Also found
                       # and fixed DetachSecurityProfile's missing existence validation
                       # (AttachSecurityProfile's sibling gopherstack-ep0r fix was never
                       # mirrored onto Detach) and a DeleteSecurityProfile ghost-row leak
                       # (target attachments were never cascade-cleaned). See the
                       # security_profiles families: entry's "ROUTING VERIFIED" paragraph and
                       # the new per-op ops: entries above for full detail.
                       #
                       # --- pass #3 (2026-07-25, superseded by pass #4 above for overall:)
                       # closed both of the two remaining
                       # partial families (job_and_jobtemplate, device_defender), each now
                       # `ok`. Found and fixed a severe, previously-undiscovered bug class:
                       # CreateJob/CreateJobTemplate were routed on POST when real AWS uses
                       # PUT, and GetJobDocument was routed at /jobs/{jobId}/document instead
                       # of the real /jobs/{jobId}/job-document -- all three ops were
                       # completely unreachable by any real SDK client. Separately, found the
                       # RouteMatcher (the layer that decides whether a request even reaches
                       # the IoT handler at all in a real deployment, distinct from the
                       # op-dispatch layer) never matched "/jobs" (no trailing slash, so
                       # ListJobs), the entire "/job-templates" path family, or the entire
                       # "/mitigationactions/" path family (CreateMitigationAction and
                       # siblings) -- all silently 404'd before ever reaching op dispatch.
                       # None of this was visible to any prior pass because every existing
                       # test called h.Handler() directly, bypassing RouteMatcher entirely;
                       # this pass added a real generated AWS SDK v2 client driven through
                       # the actual service.Router path specifically to catch this class of
                       # bug (see TestJob_FanOutAndAdvancedFields_SDKRoundTrip et al.).
                       # Implemented the foundational per-target JobExecution fan-out at
                       # CreateJob/AssociateTargetsWithJob time (previously nonexistent --
                       # CancelJobExecution's create-on-miss fallback was the only thing
                       # papering over it), Job's and JobTemplate's advanced fields
                       # (jobExecutionsRetryConfig, presignedUrlConfig, schedulingConfig,
                       # maintenanceWindows, destinationPackageVersions, computed
                       # jobProcessDetails), StartAuditMitigationActionsTask's target
                       # resolution (was silently ignoring auditCheckToReasonCodeFilter
                       # whenever auditTaskId was also set, and matched reason codes by check
                       # name alone), DetectMitigationActionsTaskSummary's wire shape
                       # (invented "actions" field instead of real "actionsDefinition";
                       # ListDetectMitigationActionsTasks returned a hand-picked 4-field
                       # summary instead of the real, richer shared summary type Describe
                       # uses), DetectMitigationActionExecution's wrong field names
                       # (executionStartTime/executionEndTime instead of real
                       # executionStartDate/executionEndDate), ActiveViolation/ViolationEvent's
                       # missing lastViolationTime/violationEventAdditionalInfo fields, and
                       # ListAuditFindings.resourceIdentifier filtering (previously left
                       # unimplemented as "can't be honestly matched without guessing" --
                       # resolved by modeling a real, fully-typed ResourceIdentifier struct
                       # instead of a freeform map, at which point the filter's per-field
                       # discriminator semantics become the same simple equality-match every
                       # other filter in this service already uses). Also implemented
                       # ListActiveViolations/ListViolationEvents' listSuppressedAlerts filter.
                       # Both target families are now genuinely `ok` -- see families: below.
                       # Overall STAYS at A- rather than A because closing device_defender
                       # surfaced a real, substantial, PREVIOUSLY UNTRACKED gap in a third,
                       # different family: CreateSecurityProfile silently drops every one of
                       # Behaviors/AlertTargets/AdditionalMetricsToRetain(V2)/
                       # MetricsExportConfig -- SecurityProfile never persisted behavior
                       # definitions at all. This is what blocks ListActiveViolations/
                       # ListViolationEvents' remaining behaviorCriteriaType filter (there is
                       # no behavior-criteria-type data anywhere in this backend to filter on)
                       # and is a real, previously-unaudited `security_profiles` family gap
                       # in its own right -- NOT part of job_and_jobtemplate or
                       # device_defender, and explicitly out of scope for this pass, but too
                       # substantial to paper over by silently declaring the service A. See
                       # the new `security_profiles` families: entry and gaps: below.
                       #
                       # --- pass #5 (2026-08-13, gopherstack-oc9v, stays A) ---
                       # Scoped via gopherstack-oc9v's wire-sweep-blind-spot campaign
                       # (anonymous inline request structs are invisible to the repo's
                       # name-regex wire-diff tooling; iot has 79 of them, third-largest
                       # concentration repo-wide). Read this file first per that issue's
                       # instructions: overall was already `A` with every family `ok`
                       # except `fleet_metric`, explicitly `partial`, so this pass scoped
                       # to `fleet_metric` alone rather than re-auditing already-`ok`
                       # families. Converted its 3 remaining inline structs
                       # (UpdateFleetMetric/UpdateCustomMetric/UpdateDimension,
                       # handler_metrics.go) to named types and closed the family -- see
                       # `fleet_metric` below for the two real bugs found (the
                       # UpdateFleetMetric gap noted by a prior pass, plus a
                       # sibling CreateFleetMetric gap the conversion surfaced that no
                       # prior pass had tracked). 76 of iot's 79 inline request structs
                       # remain unconverted; see the campaign note under Notes below for
                       # the full accounting.
ops:
  CreateThing: {wire: ok, errors: ok, state: ok, persist: ok, note: "now accepts+wires billingGroupName (was silently dropped)"}
  DescribeThing: {wire: ok, errors: ok, state: ok, persist: ok, note: "now returns billingGroupName (was omitted entirely)"}
  UpdateThing: {wire: ok, errors: ok, state: ok, persist: ok, note: "AttributePayload.merge default was inverted (defaulted to merge; AWS defaults to replace) and empty-value attribute removal was missing -- both fixed via applyAttributePayload"}
  DeleteThing: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED (gopherstack-4c0r, b8484292f) -- left b.resourceTags[thingARN] and b.thingBillingGroups[thingName] behind, inherited by a recreated thing of the same name (ListTagsForResource / DescribeThing.BillingGroupName); now cleared alongside its existing jobExecutions cascade. Regression: TestDeleteThing_ClearsGhostStateOnRecreate. Map enumeration (that pass) found the same resourceTags gap unfixed on the rest of this service's Delete* paths (filed gopherstack-1ycq) and a related but distinct ghost-reference bug where DeleteThingGroup/DeleteBillingGroup don't clean the thingThingGroups/thingBillingGroups reverse index for surviving members (filed gopherstack-6pt8). Both now resolved (2026-09-06 pass) -- see the Notes section entry for the full enumeration, observability verdicts, and regression tests; neither was DeleteThing's own bug, so this entry is unchanged beyond the status update."}
  ListThings: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateThingGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeThingGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "thingGroupMetadata.creationDate was a raw time.Time (RFC3339 string on the wire) instead of epoch-seconds; fixed via awstime.Epoch"}
  UpdateThingGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "same AttributePayload.merge bug as UpdateThing (handler didn't even parse merge from the request); fixed with the same applyAttributePayload helper"}
  DeleteThingGroup: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(gopherstack-1ycq/6pt8, 2026-09-06) left b.resourceTags[thingGroupARN] behind on delete (fixed, same shape as DeleteThing/DeletePolicy) and never removed the deleted group from thingThingGroups for surviving former members, so SearchIndex kept reporting membership in a group that no longer existed (fixed via the same removeThingFromGroupIndexes helper UpdateThingGroupsForThing already used correctly). DeleteDynamicThingGroup had both identical gaps (shares thingGroups/thingGroupMembers with the static path) and got the same fix. Regressions: TestDeleteResource_ClearsResourceTagsOnRecreate/{thing_group,dynamic_thing_group}, TestDeleteThingGroup_ClearsReverseIndexForSurvivingMembers, TestDeleteDynamicThingGroup_ClearsReverseIndexForSurvivingMembers."}
  RemoveThingFromThingGroup: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(gopherstack-6pt8, 2026-09-06) only updated thingGroupMembers[groupName], never thingThingGroups[thingName] -- inconsistent with its own sibling UpdateThingGroupsForThing's ThingGroupsToRemove path, which already called the shared removeThingFromGroupIndexes helper correctly. Now calls the same helper. Regression: TestRemoveThingFromThingGroup_UpdatesReverseIndex (negative: TestRemoveThingFromThingGroup_LeavesOtherGroupsIntact)."}
  AttachPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "was appending without dedup, so double-attach produced duplicate ListAttachedPolicies entries; fixed with appendUnique (AWS attach ops are idempotent/set semantics)"}
  AttachPrincipalPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "same duplicate-entry bug as AttachPolicy; fixed"}
  AttachThingPrincipal: {wire: ok, errors: ok, state: ok, persist: ok, note: "same duplicate-entry bug; fixed"}
  AttachSecurityProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "same duplicate-entry bug; fixed. Also now returns ResourceNotFoundException for an unknown security profile name instead of silently succeeding (gopherstack-ep0r)"}
  DetachSecurityProfile: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "(pass #4) silently no-op'd for an unknown security profile name instead of returning ResourceNotFoundException -- the same gap AttachSecurityProfile had before gopherstack-ep0r, just never mirrored onto Detach; fixed. Also confirmed reachable through RouteMatcher (already whitelisted via the /security-profiles/ prefix) and now, upon its sibling DeleteSecurityProfile firing, has no ghost-row risk -- see security_profiles family note for the DeleteSecurityProfile cascade-cleanup fix."}
  ListSecurityProfiles: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #4) two real bugs: (1) RouteMatcher whitelist never matched plain \"/security-profiles\" (no trailing slash) -- op dispatch was already correct, but no real client request ever reached it; fixed. (2) securityProfileIdentifiers entries used the full \"securityProfileName\"/\"securityProfileArn\" keys instead of the real, shortened \"name\"/\"arn\" (types.SecurityProfileIdentifier, confirmed against awsRestjson1_deserializeDocumentSecurityProfileIdentifier) -- fixed. Also now paginates via maxResults/nextToken (previously unpaginated)."}
  ListSecurityProfilesForTarget: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #4) same RouteMatcher-whitelist gap as ListSecurityProfiles for \"/security-profiles-for-target\"; fixed. Also, securityProfileTargetMappings entries were missing both the identifier's \"arn\" and the entire sibling \"target\" object real types.SecurityProfileTargetMapping has (confirmed against awsRestjson1_deserializeDocumentSecurityProfileTargetMapping); fixed. Also now paginates."}
  ListTargetsForSecurityProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #4) securityProfileTargets entries used an invented \"securityProfileTargetArn\" key instead of the real \"arn\" (types.SecurityProfileTarget, confirmed against awsRestjson1_deserializeDocumentSecurityProfileTarget); fixed. Already reachable (RouteMatcher whitelists /security-profiles/ as a prefix). Also now paginates."}
  DeleteSecurityProfile: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(pass #4) never cleaned up the deleted profile's securityProfileTargets attachment-map entry, leaving a ghost row a same-named profile re-created later would incorrectly inherit; fixed via cascade-delete. (gopherstack-1ycq, 2026-09-06) also left b.resourceTags[securityProfileARN] behind; fixed the same way. Regression: TestDeleteResource_ClearsResourceTagsOnRecreate/security_profile."}
  ValidateSecurityProfileBehaviors: {wire: ok, errors: ok, state: ok, persist: n/a, note: "(pass #4) re-verified reachable (POST /security-profile-behaviors/validate, already RouteMatcher-whitelisted via pathValidateSecurityProfileBehaviors) and its standalone validation-only semantics unchanged; its SecurityProfileBehavior/SecurityProfileBehaviorCriteria shapes were extended (not duplicated) to also serve as the real persisted Behaviors shape -- see security_profiles family note."}
  DetachPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAttachedPolicies: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "creationDate/lastModifiedDate were raw time.Time (RFC3339 string) instead of epoch-seconds; fixed via awstime.Epoch"}
  DeletePolicy: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(gopherstack-6kyn sweep) left two ghost rows: resourceTags[policyARN] (already tracked by gopherstack-6kyn, fixed prior pass) and policyVersions[policyName] -- the latter meant GetPolicyVersion on a deleted policy still returned the stale default version instead of ErrPolicyVersionNotFound (ListPolicyVersions was unaffected: it already checked policies.Has first). CreatePolicy overwrites policyVersions[name] wholesale, so a same-named recreate wasn't vulnerable, but the direct-get leak was real and observable; fixed via delete(b.policyVersions, policyName)."}
  ListPolicies: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): two bugs. (1) binding trap -- handler read maxResults/nextToken (parseIoTPagination) but the real wire binding is pageSize/marker (serializers.go awsRestjson1_serializeOpHttpBindingsListPoliciesInput), so a real client's pageSize/marker were silently ignored; switched to parseIoTMarkerPagination. (2) AscendingOrder (\"results are returned in ascending creation order\") was never read at all -- results were always name-sorted (ListPolicies() default), not by CreatedAt; now sorted by CreatedAt per the flag."}
  CreatePolicyVersion: {wire: fixed, errors: ok, state: ok, persist: ok, note: "response was missing policyArn (real CreatePolicyVersionOutput has it); fixed"}
  GetPolicyVersion: {wire: fixed, errors: ok, state: ok, persist: ok, note: "used wrong date field name \"createDate\" (real GetPolicyVersionOutput uses \"creationDate\", verified against v1.76.0's awsRestjson1_deserializeOpDocumentGetPolicyVersionOutput -- \"createDate\" is only correct for the ListPolicyVersions summary shape) and was missing generationId/lastModifiedDate + epoch encoding; fixed, added GenerationID to the PolicyVersion domain type"}
  ListPolicyVersions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "createDate was a raw time.Time; fixed via awstime.Epoch"}
  CreateTopicRule: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTopicRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "rule.createdAt was a raw time.Time (RFC3339 string) instead of epoch-seconds; fixed via awstime.Epoch"}
  DeleteTopicRule: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(gopherstack-1ycq, 2026-09-06) left b.resourceTags[ruleARN] behind on delete, inherited by a same-named recreate; fixed. Regression: TestDeleteResource_ClearsResourceTagsOnRecreate/topic_rule."}
  ReplaceTopicRule: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableTopicRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableTopicRule: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTopicRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same createdAt epoch-encoding bug as GetTopicRule; fixed"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "Tag shape uses capitalized Key/Value JSON keys -- verified against real deserializer, this IS correct for IoT (not a bug, see Notes)"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeCertificate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was missing ownedBy/previousOwnedBy/generationId/certificateMode/customerVersion/validity/transferData (bd: gopherstack-jy57, now closed) and creationDate/lastModifiedDate were raw time.Time instead of epoch-seconds; fully field-diffed against v1.76.0 CertificateDescription and implemented"}
  ListCertificates: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was returning the wrong summary shape (included lastModifiedDate, which real ListCertificates does NOT have; was missing certificateMode) plus the same epoch-encoding bug; fixed to match the real Certificate summary shape exactly (certificateArn/certificateId/certificateMode/creationDate/status). A pre-existing test (TestListCertificates_IncludesLastModifiedDate) asserted the WRONG shape -- rewritten as TestListCertificates_WireShape. 2026-08-29 (wrapper-key-sweep): AscendingOrder and pageSize/marker pagination were never read at all -- the handler returned every certificate in one response, unsorted by creation date regardless of the flag. Both now applied."}
  ListCACertificates: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): AscendingOrder, pageSize/marker pagination, and TemplateName (\"only CA certificates linked to the provided provisioning template are returned\") were all never read -- handler returned every CA cert unfiltered/unpaginated/unsorted. All three now applied; TemplateName matched against the already-stored RegistrationConfig.TemplateName (populated by RegisterCACertificate)."}
  ListCertificatesByCA: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): same AscendingOrder + pageSize/marker gap as ListCertificates -- fixed the same way."}
  ListCertificateProviders: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): AscendingOrder (\"ascending alphabetical order\") was never read -- always returned the name-ascending default regardless of the flag; now reverses to descending when false. Real op has no MaxResults field at all (only NextToken with an undocumented implicit page size), so pagination is left as a single implicit page -- unobservable without a documented page size to honor, consistent with this service's other N/A-pagination ops (ListVersions-class)."}
  ListOutgoingCertificates: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): verified already correct -- AscendingOrder and pageSize/marker were already applied (handler_certificates.go handleListOutgoingCertificates), unlike its four siblings above. Confirmed by reading the handler; no change made."}
  ListPrincipalPolicies: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): two bugs. (1) wire key: read the Principal from header X-Amzn-Principal (headerIoTPrincipal), but this op's real header is X-Amzn-Iot-Principal (serializers.go awsRestjson1_serializeOpHttpBindingsListPrincipalPoliciesInput; matches its AttachPrincipalPolicy/DetachPrincipalPolicy siblings, which already hardcoded the correct header) -- every real client's request principal was silently dropped, always returning empty. A pre-existing test (TestPolicyPrincipalListing_Pagination/list_principal_policies) sent the same wrong header and could never have caught this; fixed alongside the handler. (2) AscendingOrder (\"ascending creation order\") was never read -- backend always sorted by policy name; now sorted by each returned policy's own CreatedAt per the flag."}
  ListPolicyPrincipals: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): AscendingOrder is documented (\"ascending creation order\") but this op returns bare principal identifier strings (ListPolicyPrincipals() -> []string from policyTargets), which carries no per-attachment creation timestamp at all -- AttachPolicy/AttachPrincipalPolicy never record an attach time. Honoring the flag would require fabricating a timestamp, banned by the no-stub rule; left as the existing deterministic alphabetical order (sort.Strings) regardless of the flag. Documented gap, not silently mishandled -- no bd issue filed, structural (unlike ListPrincipalPolicies' sibling, which returns full Policy objects that DO carry the policy's own CreatedAt)."}
  ListAuthorizers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): AscendingOrder, Status filter, and pageSize/marker pagination were all never read -- handler returned every authorizer unfiltered/unpaginated, always name-ascending. All three now applied (name-ascending is ListAuthorizers()'s existing default via store.Table.Snapshot's key order, so only the false/descending case needed a reversal)."}
  ListRoleAliases: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): same AscendingOrder + pageSize/marker gap as ListAuthorizers -- fixed the same way (no Status-equivalent filter on this op)."}
  ListStreams: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): AscendingOrder and maxResults/nextToken pagination (this op's binding differs from its pageSize/marker-based siblings -- confirmed against its own serializer) were both never read -- handler returned every stream in one response. Both now applied; ascending basis taken as StreamID (the store's key order), the only stable sort key available since real AWS's doc comment doesn't state a basis the way the alphabetical-order ops do."}
  ListAuditSuppressions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 (wrapper-key-sweep): the handler read no request fields at all -- CheckName, ResourceIdentifier, MaxResults/NextToken, and AscendingOrder were all silently ignored, always returning every suppression. All now applied (JSON-body-bound: serializers.go awsRestjson1_serializeOpDocumentListAuditSuppressionsInput). AscendingOrder needed special handling: real AWS documents 'If parameter isn't provided, ascendingOrder=true' but the Go SDK's field is a plain bool (encoded only when true), so the request struct here uses *bool to distinguish omitted (default true/ascending) from explicit false (descending) -- a bare bool would have made the documented default unreachable to detect. ResourceIdentifier is matched via a dynamic per-key-set-in-filter equality helper since AuditSuppression stores it as an opaque map (unlike AuditFinding's typed ResourceIdentifier)."}
  DescribeCertificateProvider: {wire: fixed, errors: ok, state: ok, persist: ok, note: "creationDate/lastModifiedDate were raw time.Time instead of epoch-seconds; fixed. Full field set (name/arn/lambdaFunctionArn/accountDefaultForOperations/creationDate/lastModifiedDate) verified against v1.76.0 -- no other gaps"}
  TransferCertificate: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "now accepts+stores transferMessage (was silently dropped) and records TransferDate for transferData"}
  AcceptCertificateTransfer: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "was a near-total stub: wrote a bogus value into the wrong side of the certificateTransfers map for ANY certificate ID (including nonexistent ones), never validated PENDING_TRANSFER state, and never actually moved ownership or changed cert status. Fully reimplemented: validates the cert exists and is pending transfer (ResourceNotFoundException/InvalidRequestException), moves ownedBy -> previousOwnedBy chain, activates/deactivates per SetAsActive, and consumes the pending transfer"}
  RejectCertificateTransfer: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "now requires PENDING_TRANSFER state (InvalidRequestException otherwise), accepts+stores rejectReason, and records TransferRejectDate"}
  CancelCertificateTransfer: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "was an unconditional no-op success (didn't check the cert existed or was pending transfer, didn't revert cert status); now validates and reverts status to INACTIVE"}
  CreateJob: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "(pass #3) was routed on POST /jobs/{jobId}; real AWS IoT's CreateJob is PUT /jobs/{jobId} (confirmed against awsRestjson1_serializeOpCreateJob's request.Method) -- completely unreachable by any real SDK client. Fixed. Also now fans a real QUEUED JobExecution out to every resolved target thing (direct thing ARN, or thing-group ARN expanded to direct members) instead of only ever materializing an execution lazily via CancelJobExecution's create-on-miss fallback -- the foundational gap this family was previously flagged for. Also now accepts+stores jobExecutionsRetryConfig/presignedUrlConfig/schedulingConfig (incl. maintenanceWindows)/destinationPackageVersions, all previously entirely unmodeled."}
  CreateJobTemplate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) same POST-vs-PUT routing bug as CreateJob (real AWS: PUT /job-templates/{jobTemplateId}); fixed. Also now accepts+stores jobExecutionsRetryConfig/presignedUrlConfig/destinationPackageVersions/maintenanceWindows -- note maintenanceWindows is a TOP-LEVEL field on JobTemplate, unlike Job's nested schedulingConfig.maintenanceWindows (real AWS has no schedulingConfig on JobTemplate at all; confirmed against both CreateJobTemplateInput and DescribeJobTemplateOutput)."}
  GetJobDocument: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) was routed at /jobs/{jobId}/document; real AWS IoT's GetJobDocument path is /jobs/{jobId}/job-document (confirmed against awsRestjson1_serializeOpGetJobDocument's SplitURI call) -- completely unreachable by any real SDK client. Fixed."}
  ListJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "(pass #3) op dispatch itself was already correct, but the RouteMatcher whitelist (the layer deciding whether a request reaches the IoT handler at all in a real deployment) never matched \"/jobs\" with no trailing slash -- a real ListJobs request never reached op dispatch. Fixed in matchCoreIoTPathSecondary/matchJobAndTemplatePath (handler_routing.go)."}
  ListJobTemplates: {wire: ok, errors: ok, state: ok, persist: ok, note: "(pass #3) same RouteMatcher-whitelist gap as ListJobs -- the entire /job-templates path family (this op plus CreateJobTemplate/DescribeJobTemplate/DeleteJobTemplate) was absent from the whitelist, so no request in that family ever reached the IoT handler in a real deployment. Fixed."}
  DeleteJobTemplate: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(pass #3) same RouteMatcher-whitelist gap as ListJobs/ListJobTemplates; fixed. (gopherstack-1ycq, 2026-09-06) also left b.resourceTags[jobTemplateARN] behind; fixed. Regression: TestDeleteResource_ClearsResourceTagsOnRecreate/job_template."}
  AssociateTargetsWithJob: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "mutated jobTargets for any job ID without checking the job existed; now returns ResourceNotFoundException for an unknown job (gopherstack-ep0r). (pass #3) response was also missing \"description\" (real AssociateTargetsWithJobOutput has it); newly associated targets are now merged into the job's own Targets list (previously only written to an otherwise-unread jobTargets map, so DescribeJob never reflected them) and immediately fanned out into QUEUED JobExecution rows, matching CreateJob's own fan-out for a CONTINUOUS job's initial targets."}
  DescribeJobExecution: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(2026-07-25 #2) was routed under /jobs/{jobId}/things/{thingName}, a path no real client sends (real AWS: /things/{thingName}/jobs/{jobId}, confirmed against serializers.go http bindings) -- completely unreachable by a real SDK client. Also leaked an invented \"thingName\" field instead of the real \"thingArn\", and was missing statusDetails/versionNumber/forceCanceled/approximateSecondsBeforeTimedOut entirely. Both fixed."}
  CancelJobExecution: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "(2026-07-25 #2) same routing bug as DescribeJobExecution, fixed. Also silently ignored force/expectedVersion/statusDetails entirely; now rejects an IN_PROGRESS cancel without force=true (InvalidStateTransitionException) and a mismatched expectedVersion (VersionConflictException), matching real CancelJobExecutionInput semantics"}
  DeleteJobExecution: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "(2026-07-25 #2) same routing bug (real path also carries an executionNumber URI segment), fixed. Also silently ignored force; now rejects deleting a non-terminal (QUEUED/IN_PROGRESS) execution without force=true"}
  ListJobExecutionsForJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(2026-07-25 #2) response was flat {jobId,thingName,status} per entry; real ListJobExecutionsForJobOutput.executionSummaries is []JobExecutionSummaryForJob{thingArn, jobExecutionSummary:{...}} (confirmed against awsRestjson1_deserializeDocumentJobExecutionSummaryForJob) -- a real client's deserializer would have found none of the keys it looks for and returned entirely empty summaries. Fixed."}
  ListJobExecutionsForThing: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same bug and fix as ListJobExecutionsForJob, for the sibling JobExecutionSummaryForThing{jobId, jobExecutionSummary:{...}} shape"}
  UpdateAccountAuditConfiguration: {wire: ok, errors: ok, state: fixed, persist: ok, note: "2026-08-21 (gopherstack-c8ge): singleton config with no Create op. AuditCheckConfigurations is map[checkName]*AuditCheckConfig (types.UpdateAccountAuditConfigurationInput); a real client only ever names the checks it's changing in one call, but the handler wholesale-replaced the stored map with whatever the request carried, so a later call enabling check B silently disabled every check enabled by an earlier call that never mentioned it. Fixed to merge per key. See TestUpdateAccountAuditConfiguration_ChecksSurviveIndependentUpdates."}
  UpdatePackageConfiguration: {wire: ok, errors: ok, state: fixed, persist: ok, note: "2026-08-21 (gopherstack-c8ge): singleton config with no Create op. VersionUpdateByJobsConfig (types.VersionUpdateByJobsConfig) has two independently-optional pointer scalars, Enabled and RoleArn; the handler wholesale-replaced the stored map[string]any with whatever the request carried, so an Update naming only roleArn wiped a previously-set enabled. Fixed to merge per key. See TestUpdatePackageConfiguration_FieldsSurviveIndependentUpdates."}
  CancelAuditTask: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "unconditionally set status to CANCELED for any task ID; now returns ResourceNotFoundException for an unknown task and InvalidRequestException if it isn't IN_PROGRESS (gopherstack-ep0r)"}
  CancelAuditMitigationActionsTask: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "same class of bug as CancelAuditTask; fixed identically (gopherstack-ep0r)"}
  ListAuditFindings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(2026-07-25 #2) was routed on GET; real AWS's ListAuditFindings is POST /audit/findings with filters in a JSON body (confirmed against serializers.go http bindings) -- completely unreachable by a real SDK client. Also ignored every filter field entirely. Both fixed: now POST-routed and implements checkName/taskId/listSuppressedFindings/startTime/endTime filtering (resourceIdentifier filtering remains unimplemented -- see families: device_defender)"}
  DescribeAuditFinding: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(2026-07-25 #2) AuditFinding was missing isSuppressed/reasonForNonComplianceCode/reasonForNonCompliance/taskStartTime entirely (confirmed against awsRestjson1_deserializeDocumentAuditFinding); all four now modeled. taskStartTime is auto-derived from the referenced AuditTask when the finding has a taskId but no explicit taskStartTime"}
  ListAuditFindings_resourceIdentifier: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) resourceIdentifier filtering, previously left unimplemented (AuditFinding.NonCompliantResource was a freeform map[string]any that couldn't honestly discriminate against real AWS's ~10 per-check-type ResourceIdentifier fields). Fixed by modeling a real, fully-typed ResourceIdentifier struct (account/caCertificateId/clientId/cognitoIdentityPoolId/deviceCertificateArn/deviceCertificateId/iamRoleArn/issuerCertificateIdentifier/policyVersionIdentifier/roleAliasArn, confirmed against types.ResourceIdentifier) in place of the map, at which point the filter becomes the same per-field-equality-when-set semantics every other filter in this service already uses -- no per-check-type guessing required."}
  StartAuditMitigationActionsTask: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(pass #3) target resolution had two real bugs: (1) when a target set both auditTaskId and auditCheckToReasonCodeFilter, only auditTaskId was ever honored (a switch's first matching case won) -- auditCheckToReasonCodeFilter was silently ignored even though real AWS's AuditMitigationActionsTaskTarget lets both apply together (\"this audit's findings for check X with reason code Y\"). (2) auditCheckToReasonCodeFilter matched by check name alone, ignoring the actual reason-code list value (real AWS filters on the listed codes when non-empty; an empty list for a check means \"any reason code\"). Both fixed in auditMitigationFindingIDs (device_defender.go)."}
  CreateMitigationAction_and_4_siblings: {wire: ok, errors: ok, state: ok, persist: ok, note: "(pass #3) CreateMitigationAction/DescribeMitigationAction/UpdateMitigationAction/DeleteMitigationAction/ListMitigationActions' op-dispatch routing (resolveMitigationActionOps) was already correct, but the RouteMatcher whitelist (the layer deciding whether a request reaches the IoT handler at ALL in a real deployment, checked before op dispatch) never matched the \"/mitigationactions/\" path prefix -- every request to any of these 5 ops 404'd before ever reaching op dispatch in a real deployment. Only caught because this pass added real generated-SDK-client tests driven through the actual service.Router path (existing tests all called h.Handler() directly, bypassing RouteMatcher entirely). Fixed in matchCoreIoTPathSecondary (handler_routing.go)."}
  StartDetectMitigationActionsTask: {wire: ok, errors: ok, state: fixed, persist: ok, note: "(pass #3) now accepts+stores violationEventOccurrenceRange (previously entirely unmodeled)"}
  DescribeDetectMitigationActionsTask: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) DetectMitigationActionsTaskSummary's real wire field is \"actionsDefinition\" (a list of full MitigationAction objects with id/name/roleArn/actionParams, confirmed against types.DetectMitigationActionsTaskSummary/types.MitigationAction) -- this emulator instead emitted \"actions\" (a list of bare action-name strings), a field that does not exist on the real type at all. A real client's deserializer would never have found \"actionsDefinition\" and left every task's actions permanently empty. Fixed via a new MitigationActionRefs backend lookup that resolves stored action names to their id/name/roleArn/actionParams at response time. violationEventOccurrenceRange also now surfaced (was entirely absent)."}
  ListDetectMitigationActionsTasks: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) previously built a hand-picked 4-field summary ({taskId,taskStatus,taskStartTime,taskEndTime}); real AWS's ListDetectMitigationActionsTasksOutput.Tasks is []types.DetectMitigationActionsTaskSummary -- the EXACT SAME rich type DescribeDetectMitigationActionsTask returns (confirmed against v1.76.0), not a narrower list-only summary (unlike the audit-mitigation side, where ListAuditMitigationActionsTasks genuinely does use a narrower AuditMitigationActionsTaskMetadata type). A real client's deserializer silently dropped target/actionsDefinition/taskStatistics/onlyActiveViolationsIncluded/suppressedAlertsIncluded/violationEventOccurrenceRange from every list entry. Fixed by sharing the same wire-shape builder (detectMitigationTaskSummaryWire) with Describe."}
  ListDetectMitigationActionsExecutions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) DetectMitigationActionExecution's execution-time fields were wire-keyed \"executionStartTime\"/\"executionEndTime\"; real AWS's are \"executionStartDate\"/\"executionEndDate\" (confirmed against awsRestjson1_deserializeDocumentDetectMitigationActionExecution) -- a real client's deserializer would never have found either key and left both permanently unset. Fixed (fields renamed ExecutionStartDate/ExecutionEndDate to match)."}
  ListAuditMitigationActionsTasks: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) emitted an invented \"endTime\" key; real types.AuditMitigationActionsTaskMetadata is {taskId, taskStatus, startTime} only (confirmed against v1.76.0). Harmless to a real client (unknown fields are ignored by deserializers), but removed for wire-shape accuracy."}
  ListActiveViolations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "(pass #3) ActiveViolation was missing lastViolationTime and violationEventAdditionalInfo entirely (confirmed against types.ActiveViolation -- real AWS distinguishes \"when the violation started\" from \"when the most recent violation occurred\", the latter updating on every subsequent detection of the same ongoing violation); both now modeled. Also implemented the listSuppressedAlerts filter (previously unimplemented) by adding an internal-only (json:\"-\", real ActiveViolation has no wire field for this) Suppressed flag, directly seedable, mirroring AuditFinding.IsSuppressed's identical simplification elsewhere in this service. (pass #4) behaviorCriteriaType filter now also implemented, resolved live against the owning security profile's now-persisted Behaviors -- see security_profiles below."}
  ListViolationEvents: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same violationEventAdditionalInfo + listSuppressedAlerts fixes as ListActiveViolations, for the sibling ViolationEvent type"}
  DescribeJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "documentSource was nested inside \"job\" instead of being a top-level DescribeJobOutput field (verified against v1.76.0); the nested Job object also leaked invented document/documentSource/tags fields that don't exist on real types.Job -- fixed (documentSource promoted to top level, invented fields tagged json:\"-\"). (pass #3) now also returns jobExecutionsRetryConfig/presignedUrlConfig/schedulingConfig/destinationPackageVersions, and a computed jobProcessDetails rollup (numberOf{Queued,InProgress,Succeeded,Failed,Rejected,Canceled,Removed}Things + processingTargets) derived live from the backend's real per-target JobExecution rows rather than a separately-maintained (and driftable) counter."}
  DescribeJobTemplate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "JobTemplate leaked an invented \"tags\" field not present in real DescribeJobTemplateOutput; tagged json:\"-\". (pass #3) now also returns jobExecutionsRetryConfig/presignedUrlConfig/destinationPackageVersions/maintenanceWindows, field-diffed separately from Job's own advanced fields (see CreateJobTemplate note on the maintenanceWindows nesting difference)."}
  AttachPrincipalPolicy_and_11_other_handlers: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "12 handlers (Attach*, AcceptCertificateTransfer, AddThingToBillingGroup/ThingGroup, AssociateSbomWithPackageVersion, AssociateTargetsWithJob, CancelAuditMitigationActionsTask, CancelAuditTask, DescribeEndpoint) returned a raw {\"error\":...} 500 body instead of h.handleError's {__type,message} shape on any backend error. Fixed in the prior pass; this pass found a DEEPER version of the same bug class affecting ~130 call sites (see error_handling family note below)"}
families:
  thing_core: {status: ok, note: "CreateThing/DescribeThing/UpdateThing/DeleteThing/ListThings audited field-by-field against v1.76.0 serializers/deserializers; 2 real bugs found+fixed (billingGroupName, AttributePayload merge default)"}
  thing_group: {status: ok, note: "Create/Describe/Update/Delete/List audited; UpdateThingGroup AttributePayload bug fixed (mirrors UpdateThing); DescribeThingGroup epoch-timestamp bug fixed this pass"}
  thing_type: {status: ok, note: "field-diffed this pass: CreateThingType/DescribeThingType/ListThingTypes/DeprecateThingType/UpdateThingType output shapes all verified against v1.76.0 (CreateThingTypeOutput, DescribeThingTypeOutput, ThingTypeMetadata, ThingTypeProperties). Epoch-timestamp bug (creationDate/deprecationDate) fixed. Only gap: optional mqtt5Configuration in thingTypeProperties not implemented (low-value MQTT5 user-property enrichment feature, not filed as a blocking gap)"}
  policy_attach: {status: ok, note: "Attach/Detach/List for both Policy and PrincipalPolicy audited; duplicate-entry bug on double-attach fixed across all 4 attach ops (Policy/PrincipalPolicy/ThingPrincipal/SecurityProfile); AttachSecurityProfile existence-validation gap fixed this pass"}
  policy_version: {status: ok, note: "CreatePolicyVersion/GetPolicyVersion/ListPolicyVersions field-diffed against v1.76.0 this pass: CreatePolicyVersion was missing policyArn (fixed), GetPolicyVersion used the wrong date field name \"createDate\" instead of \"creationDate\" and was missing generationId/lastModifiedDate (fixed), both had the epoch-timestamp bug (fixed)"}
  tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource verified real state mutation + correct Key/Value wire casing"}
  topic_rule: {status: ok, note: "Create/Get/Delete/Replace/Enable/Disable/List spot-checked against restjson1 deserializer field names (rule/ruleArn wrapper); epoch-timestamp bug on createdAt fixed this pass"}
  error_handling: {status: fixed, note: "Prior pass fixed 12 handlers that bypassed h.handleError entirely (raw {\"error\":...} 500). This pass found a deeper, broader version of the same bug class: respondErr (the error helper used by ~130 call sites across 21 \"batch2/batch3\" handler files) only recognized ErrResourceNotFound and ErrAlreadyExists -- every other sentinel (ErrThingNotFound, ErrCertificateNotFound, ErrThingGroupNotFound, ErrPolicyVersionNotFound, ErrVersionConflict, ErrDeleteConflict, ErrVersionsLimitExceeded, etc.) fell through to the wrong HTTP status/error code (400 InvalidRequestException or the 500 default instead of the correct 404/409). Fixed by extracting the canonical mapping into a single writeIoTError function shared by both respondErr and Handler.handleError, so every handler now maps every sentinel identically regardless of which helper it calls."}
  timestamps: {status: fixed, note: "NEW bug class found this pass, not previously flagged: Policy, GetPolicyOutput, PolicyVersion, ThingType, ThingGroup, Certificate, CertificateProvider, and TopicRule all stored creationDate/lastModifiedDate/deprecationDate as raw time.Time struct fields that were being embedded directly into map[string]any JSON responses -- json.Marshal renders a bare time.Time as an RFC3339 STRING, but restjson1's DateType wire format requires a JSON NUMBER of epoch seconds (confirmed against v1.76.0's awsRestjson1_deserialize* functions, which reject non-json.Number timestamp values with 'expected ... to be a JSON Number, got string instead'). This is the same bug class documented as previously fixed in sagemaker/glue/ssm. Fixed at every response call site via pkgs/awstime.Epoch(); the internal struct fields remain time.Time (only used for internal storage/persistence, not wire output) so no persistence-format changes were needed. Contrary to the PARITY.md note this superseded, NOT every part of this service already used the float64-epoch convention -- Job/JobTemplate/CACertificate/OutgoingCertificate/provisioning/packages/metrics did, but the 8 struct families above did not."}
  invented_fields: {status: fixed, note: "Job leaked \"tags\"/\"document\"/\"documentSource\" and JobTemplate leaked \"tags\" -- none of these exist on real types.Job/DescribeJobTemplateOutput (verified against v1.76.0's awsRestjson1_deserializeDocumentJob, which has no tags/document/documentSource cases; documentSource is real but only as a top-level DescribeJobOutput field, and document is only retrievable via the separate GetJobDocument operation). Fixed via json:\"-\" tags on the domain struct fields (kept for internal storage) plus promoting documentSource to the DescribeJobOutput top level."}
  certificate: {status: ok, note: "Full CRUD (Create/Register/RegisterWithoutCA/Describe/List/Update/Delete) plus the transfer lifecycle (Transfer/Accept/Reject/Cancel) field-diffed and fixed this pass -- see DescribeCertificate/ListCertificates/AcceptCertificateTransfer/etc. ops above and gopherstack-jy57 (now closed)"}
  certificate_provider: {status: ok, note: "Create/Describe/List/Update/Delete field-diffed against v1.76.0; only bug was the epoch-timestamp encoding on Describe (fixed). Full field set otherwise already correct"}
  job_and_jobtemplate: {status: ok, note: "(pass #3) CLOSED. Field-diffed exhaustively against v1.76.0. Foundational fan-out gap implemented: CreateJob/AssociateTargetsWithJob now fan a real QUEUED JobExecution out to every resolved target thing (thing ARN direct, or thing-group ARN expanded to direct members -- matching ListThingsInThingGroup's own non-recursive semantics), cascade-cleaned on DeleteThing/DeleteJob. Job's and JobTemplate's advanced fields (jobExecutionsRetryConfig, presignedUrlConfig, schedulingConfig incl. maintenanceWindows for Job / top-level maintenanceWindows for JobTemplate, destinationPackageVersions, computed jobProcessDetails) implemented end to end: request parsing, backend state, response wire shape, persistence. Found and fixed a severe, previously-undiscovered routing bug class: CreateJob and CreateJobTemplate were both routed on POST when real AWS uses PUT (awsRestjson1_serializeOpCreateJob/CreateJobTemplate), and GetJobDocument was routed at /jobs/{jobId}/document instead of the real /jobs/{jobId}/job-document -- all three completely unreachable by any real SDK client. Also found the RouteMatcher whitelist (checked before op dispatch in a real deployment) never matched plain \"/jobs\" (ListJobs) or the entire \"/job-templates\" path family -- both silently 404'd. AssociateTargetsWithJob was missing the real \"description\" output field and never merged newly-associated targets into the job's own Targets list. All fixed. See ops: above for each op's specifics."}
  device_defender: {status: ok, note: "(pass #3) CLOSED for everything within this family's own scope. StartAuditMitigationActionsTask's target resolution fixed (combined auditTaskId+auditCheckToReasonCodeFilter AND semantics, real reason-code-list matching instead of check-name-only). ML-Detect surface (StartDetectMitigationActionsTask and siblings) field-diffed: DetectMitigationActionsTaskSummary's actionsDefinition wire shape fixed (was an invented \"actions\" field), ListDetectMitigationActionsTasks now returns the same rich summary type Describe does (was a hand-picked 4-field subset), DetectMitigationActionExecution's executionStartDate/executionEndDate field names fixed (were wire-keyed wrong), violationEventOccurrenceRange added. Violations surface (ListActiveViolations/ListViolationEvents) field-diffed: lastViolationTime/violationEventAdditionalInfo added, listSuppressedAlerts filter implemented. ListAuditFindings.resourceIdentifier filtering implemented (previously the family's most-cited unimplementable gap) by modeling a real, fully-typed ResourceIdentifier struct instead of a freeform map. Also found the entire \"/mitigationactions/\" path family (CreateMitigationAction and siblings) was absent from the RouteMatcher whitelist -- completely unreachable in a real deployment despite correct op-dispatch routing. (pass #4) ListActiveViolations/ListViolationEvents' behaviorCriteriaType filter is now also implemented, once security_profiles (see below) closed the Behaviors-persistence gap that previously blocked it."}
  security_profiles: {status: ok, note: "(pass #4) CLOSED. CreateSecurityProfile's real input (types.CreateSecurityProfileInput) has Behaviors/AlertTargets/AdditionalMetricsToRetain/AdditionalMetricsToRetainV2/MetricsExportConfig -- this backend's SecurityProfile struct stored NONE of them; the request fields were silently accepted and dropped (the same severe 'dropped request field' bug class flagged elsewhere in this campaign, e.g. elasticache). All five are now modeled on SecurityProfile and wired end-to-end. Extended (rather than duplicated) ValidateSecurityProfileBehaviors' existing SecurityProfileBehavior/SecurityProfileBehaviorCriteria shapes to also be the real persisted Behaviors shape: SecurityProfileBehavior gained MetricDimension/ExportMetric/SuppressAlerts, SecurityProfileBehaviorCriteria gained Value/StatisticalThreshold/MlDetectionConfig (field-diffed against types.Behavior/types.BehaviorCriteria). New types SecurityProfileAlertTarget/SecurityProfileMetricToRetain/SecurityProfileMetricsExportConfig/SecurityProfileMetricDimension/SecurityProfileMetricValue/SecurityProfileStatisticalThreshold/SecurityProfileMLDetectionConfig mirror types.AlertTarget/MetricToRetain/MetricsExportConfig/MetricDimension/MetricValue/StatisticalThreshold/MachineLearningDetectionConfig. DescribeSecurityProfile/UpdateSecurityProfile field-diffed against DescribeSecurityProfileOutput/UpdateSecurityProfileOutput and confirmed to have had the identical gap (UpdateSecurityProfile previously accepted only securityProfileDescription); both rebuilt to return the full real field set, epoch-encoded creationDate/lastModifiedDate. UpdateSecurityProfile now also implements ExpectedVersion's optimistic-lock semantics (-> ErrVersionConflict/VersionConflictException on mismatch, confirmed against awsRestjson1_serializeOpHttpBindingsUpdateSecurityProfileInput -- expectedVersion is a QUERY parameter, not a body field) and every DeleteX-flag-vs-field mutual exclusion rule (deleteBehaviors/deleteAlertTargets/deleteAdditionalMetricsToRetain/deleteMetricsExportConfig, each rejecting InvalidRequestException-mapped ErrValidation when the corresponding field is also supplied in the same call, matching real AWS's documented semantics). Also found and fixed a real 'invented field' leak while field-diffing: SecurityProfile's pre-existing Tags field was surfaced on Describe/Update responses, but real DescribeSecurityProfileOutput/UpdateSecurityProfileOutput have NO \"tags\" field at all (tags are only ever retrievable via the separate ListTagsForResource op) -- fixed via json:\"-\" (same pattern as Job/JobTemplate's previously-fixed leaked \"tags\"). Persistence required no persistence.go changes: SecurityProfile already round-trips via the generic store.Table[SecurityProfile] registry (store.go/store_setup.go), which marshals the full struct -- confirmed by a new persistence regression case seeding a profile with all five previously-dropped fields. Closing this also unblocked device_defender's ListActiveViolations/ListViolationEvents behaviorCriteriaType filter (STATIC/STATISTICAL/MACHINE_LEARNING, types.BehaviorCriteriaType), now implemented via securityProfileBehaviorCriteriaTypeLocked, which resolves a violation's owning security profile's now-real stored Behaviors live -- see device_defender's own families: entry, updated below. ROUTING VERIFIED: with the Behaviors gap closed, every security-profile op (CreateSecurityProfile, UpdateSecurityProfile, DescribeSecurityProfile, ListSecurityProfiles, ListSecurityProfilesForTarget, AttachSecurityProfile, DetachSecurityProfile, ListTargetsForSecurityProfile, ValidateSecurityProfileBehaviors) was driven through a real generated AWS SDK v2 IoT client against the actual service.Router path (newIoTSDKClient/TestSecurityProfile_RoutingWireShapesAndBehaviorCriteriaType_SDKRoundTrip, handler_security_profiles_test.go; also TestHandler_RouteMatcher's list_security_profiles/list_security_profiles_for_target cases), not just h.Handler() directly -- the same class of gate three prior passes each found real bugs in for other op families. This turned up two more, previously-undiscovered bugs in this family specifically: (1) ListSecurityProfiles (GET /security-profiles, no trailing slash) and ListSecurityProfilesForTarget (GET /security-profiles-for-target) were BOTH entirely absent from the RouteMatcher whitelist (matchCoreIoTPathSecondary, handler_routing.go) -- op dispatch itself (resolveSecurityProfileOps) already handled both paths correctly, but a real client's request never reached op dispatch at all in a real deployment; fixed. (2) three wire-shape key-name bugs, confirmed against v1.76.0's awsRestjson1_deserializeDocumentSecurityProfileIdentifier/SecurityProfileTarget/SecurityProfileTargetMapping: ListSecurityProfiles' securityProfileIdentifiers used the full \"securityProfileName\"/\"securityProfileArn\" keys instead of the real, SHORTENED \"name\"/\"arn\" SecurityProfileIdentifier keys; ListTargetsForSecurityProfile's securityProfileTargets used an invented \"securityProfileTargetArn\" key instead of the real \"arn\"; ListSecurityProfilesForTarget's securityProfileTargetMappings nested only {securityProfileIdentifier:{name}} with no arn and no sibling \"target\" object at all, instead of the real {securityProfileIdentifier:{name,arn}, target:{arn}} -- a real client's deserializer would have left the affected fields permanently nil/empty under all three. All three fixed; all three List ops also gained maxResults/nextToken pagination (previously always returned every item in one page, unlike sibling List ops elsewhere in this service). Two smaller bugs found in the same pass: DetachSecurityProfile silently no-op'd for an unknown security profile name instead of returning ResourceNotFoundException (AttachSecurityProfile already had this validation, from gopherstack-ep0r, but it was never mirrored onto Detach); and DeleteSecurityProfile never cleaned up the deleted profile's entry in the securityProfileTargets attachment map, leaving a ghost row that a same-named profile re-created later would incorrectly inherit -- both fixed (TestSecurityProfile_DetachNotFoundAndDeleteCascade)."}
  fleet_indexing: {status: ok, note: "Field-diffed against v1.76.0 this pass (previously entirely untouched). Two real, previously-unflagged wire-shape bugs found and fixed: (1) SearchIndex's ThingGroupDocument sent a single \"parentGroupName\" string (direct parent only) instead of the real \"parentGroupNames\" LIST field (the full ancestor chain) -- confirmed against awsRestjson1_deserializeDocumentThingGroupDocument, a real client's deserializer would never find the key it looks for under the old shape and silently leave the field empty; also added the missing \"thingGroupDescription\" field. (2) DescribeThingGroup's thingGroupMetadata was completely missing \"rootToParentThingGroups\" (root-first ancestor name+ARN list) -- confirmed against awsRestjson1_deserializeDocumentThingGroupMetadata; not implemented at all previously. Both fixed via a new thingGroupAncestors backend helper (indexing.go) that reconstructs the full chain by walking gopherstack's per-group direct-ParentGroupName links, since the domain model only stores one level per group. (3) GetStatistics' Statistics response was missing \"sumOfSquares\" entirely (types.Statistics has it; confirmed against awsRestjson1_deserializeDocumentStatistics) -- fixed by computing it in computeStatistics alongside the existing sum/variance accumulation. GetCardinality/GetPercentiles/GetBucketsAggregation/DescribeIndex/ListIndices output shapes also field-diffed against their real GetCardinalityOutput/GetPercentilesOutput/GetBucketsAggregationOutput/types.PercentPair/types.Bucket counterparts -- no further gaps found on this pass's sample."}
  billing_group: {status: fixed, note: "AddThingToBillingGroup/RemoveThingFromBillingGroup/ListThingsInBillingGroup verified real state mutation via thingBillingGroups map; DescribeThing now surfaces it (see CreateThing/DescribeThing above). (gopherstack-1ycq/6pt8, 2026-09-06) DeleteBillingGroup left two ghost rows: b.resourceTags[billingGroupARN] (same shape as the rest of gopherstack-1ycq) and thingBillingGroups[thingName] for every thing still pointing at the deleted group, so DescribeThing.BillingGroupName kept naming a group that no longer existed (RemoveThingFromBillingGroup already clears this correctly for a single thing; DeleteBillingGroup just never swept the rest). Both fixed. Regressions: TestDeleteResource_ClearsResourceTagsOnRecreate/billing_group, TestDeleteBillingGroup_ClearsThingBillingGroupsForSurvivingMembers."}
  persistence: {status: ok, note: "backendSnapshot/Restore in persistence.go covers all backend maps observed during this audit (policyTargets, thingPrincipals, thingBillingGroups, thingThingGroups, securityProfileTargets, resourceTags, certificateTransfers, etc.); Handler.Snapshot/Restore already delegate correctly -- no gaps found. Certificate struct's new transfer-lifecycle fields (OwnedBy/PreviousOwnedBy/GenerationID/CertificateMode/CustomerVersion/Validity*/Transfer*) round-trip correctly since persistence marshals the full struct, not the handler-layer wire shape."}
  fleet_metric: {status: ok, note: "(pass #5, 2026-08-13, gopherstack-oc9v) CLOSED. Prior pass fixed UpdateFleetMetric's dropped expectedVersion but left indexName/aggregationType/aggregationField/queryVersion/unit unfixed. This pass converted handler_metrics.go's 3 remaining anonymous inline request structs (UpdateFleetMetric, UpdateCustomMetric, UpdateDimension -- part of the wire-sweep-blind-spot campaign, gopherstack-oc9v) to named types (UpdateFleetMetricInput/UpdateCustomMetricInput/UpdateDimensionInput, metrics.go), and while doing so field-diffed the whole family against v1.77.4's UpdateFleetMetricInput/CreateFleetMetricInput/DescribeFleetMetricOutput directly. Fixed all 5 of those documented UpdateFleetMetric gaps (indexName, aggregationType, aggregationField, queryVersion, unit all now applied). Also found a SIXTH, previously-untracked gap the same diff surfaced: CreateFleetMetricInput was ALSO missing aggregationField/aggregationType entirely (both `This member is required` on the real type) -- CreateFleetMetric silently dropped them with no error, and FleetMetric never modeled them at all, so DescribeFleetMetric/ListFleetMetrics could never have surfaced them either even if a caller worked around the drop. New `AggregationType{Name,Values}` type (metrics.go) mirrors types.AggregationType; both Create and Update now thread aggregationField/aggregationType through end to end (request parsing, backend storage on FleetMetric, response wire shape -- confirmed against awsRestjson1_deserializeOpDocumentDescribeFleetMetricOutput's \"aggregationField\"/\"aggregationType\" keys, aggregationType nested as {name,values}). UpdateCustomMetric/UpdateDimension's inline structs were already field-complete (DisplayName-only / StringValues-only, matching real UpdateCustomMetricInput/UpdateDimensionInput exactly) -- converted for tooling visibility only, no bug. Regression: TestFleetMetric_AggregationAndUpdateFields (handler_metrics_test.go), verified to fail against the pre-fix code by temporarily reverting the field-wiring."}
  device_shadows: {status: ok, note: "NEW entry (2026-07-31, reverse sdkcheck sweep, gopherstack-vhw2): DeleteThingShadow/GetThingShadow/ListNamedShadowsForThing/UpdateThingShadow are real IoT Data Plane operations, on a separate SDK client (aws-sdk-go-v2/service/iotdataplane) from this service's control-plane client (aws-sdk-go-v2/service/iot) -- confirmed by name against iotdataplane.Client. pkgs/sdkcheck's reverse check was flagging all 4 as 'phantom' only because it compared them against iotsdk.Client instead of iotdataplanesdk.Client; sdk_completeness_test.go now checks this family separately against the correct client (notImplemented: DeleteConnection/GetConnection/GetRetainedMessage/ListRetainedMessages/ListSubscriptions/Publish/SendDirectMessage, the rest of that client's surface, covered instead by the separate services/iotdataplane package -- this Handler's shadow REST routes (handler_shadows.go) and services/iotdataplane's own shadow implementation are a pre-existing duplication across the two packages, not introduced by this fix and not resolved here). No wire-shape field-diff done, naming/completeness only."}
  filter_semantics: {status: ok, note: "gopherstack-uox6 (value-semantics sweep, 2026-08-30): audited every hand-rolled filter/matcher/comparison helper against its SDK doc comment (aws-sdk-go-v2/service/iot@v1.77.4) for the class field-diff tools can't see (right field, wrong algorithm) -- MatchesTopic/matchParts (MQTT # /+ wildcard rules, correct), ListAuditFindings' matchResourceIdentifier/matchPolicyVersionIdentifier/matchIssuerCertificateIdentifier/matchesFilter (per-field discriminator AND-match against types.ResourceIdentifier, correct; ListSuppressedFindings tri-state nil/true/false correctly mirrors 'if not provided, lists both'), ListAuditSuppressions' matchAuditSuppressionResourceIdentifier (same shape, dynamic map form, correct) plus its AscendingOrder default-true-when-nil (matches api_op_ListAuditSuppressions.go), ListCommandExecutionsByFilter (commandARN/targetARN/status AND-equality, matches api_op_ListCommandExecutions.go -- no documented modifier on any of the three), and device_defender's violationFilter.matchesCommon/matchesBehaviorCriteriaType/matchesWindow (ListActiveViolations/ListViolationEvents, types.ListActiveViolationsInput/ListViolationEventsInput carry no modifier docs -- AND-equality plus a nil/true/false tri-state for listSuppressedAlerts, correct). No bugs found -- clean verdict. Adjacent, NOT this bug class (field never read at all, not read-and-misapplied -- a field-diff-catchable gap, not fixed here): handleListThings (handler.go) ignores ListThingsInput's documented attributeName/attributeValue/thingTypeName query parameters entirely, unlike ListCommandExecutionsByFilter's real filtering; and indexing.go's matchesThingQuery/matchThingTerm/matchesThingGroupQuery/matchThingGroupTerm reimplement AWS's fleet-indexing query syntax (SearchIndex's queryString, doc-linked to https://docs.aws.amazon.com/iot/latest/developerguide/query-syntax.html) as a simplified whitespace/colon/substring DSL rather than the real query grammar -- the real grammar isn't specified precisely enough in the SDK source to verify field-by-field without guessing, same class of gap as secretsmanager's word-splitting rule (gopherstack-uox6's own originating note), recorded rather than reshaped."}
gaps: []
  # The UpdateFleetMetric gap (dropped indexName/aggregationType/
  # aggregationField/queryVersion/unit) closed by pass #5 (2026-08-13, gopherstack-oc9v)
  # -- see fleet_metric: above, which also documents a 6th, previously-untracked gap
  # (CreateFleetMetric dropping aggregationField/aggregationType too) that surfaced
  # only once the family's inline request structs were converted to named types.
  #
  # All families closed as of pass #4 (2026-07-25). security_profiles -- the sole reason
  # pass #3 stayed at A- -- is now `ok` (see its families: entry above): CreateSecurityProfile/
  # UpdateSecurityProfile persist the full real field set, ListActiveViolations/
  # ListViolationEvents' behaviorCriteriaType filter is implemented, and every
  # security-profile op (CreateSecurityProfile, UpdateSecurityProfile, DescribeSecurityProfile,
  # ListSecurityProfiles, ListSecurityProfilesForTarget, AttachSecurityProfile,
  # DetachSecurityProfile, ListTargetsForSecurityProfile, ValidateSecurityProfileBehaviors) was
  # re-verified reachable end to end through the real RouteMatcher, not just callable on the
  # handler -- see the security_profiles families: entry's "routing verified" paragraph for the
  # two additional, previously-undiscovered bugs that check turned up (a RouteMatcher-whitelist
  # gap for ListSecurityProfiles/ListSecurityProfilesForTarget, and three wire-shape key-name
  # bugs on the same two ops plus ListTargetsForSecurityProfile).
deferred: []
  # gopherstack-srzb (job_and_jobtemplate + device_defender consolidated tracking issue) and
  # the security_profiles item that superseded it as pass #3's sole open item are both closed
  # as of this pass. No known deferred work remains for this service.
leaks: {status: found_and_fixed, note: "FOUND: Handler.StartWorker launched the embedded MQTT broker in a bare `go func(){ broker.Start(ctx) }()` with no way to wait for it to exit -- Handler didn't implement service.Shutdowner at all, so the broker goroutine had no deterministic drain path on service shutdown (relied entirely on the caller's ctx being cancelled elsewhere, with no join/wait). This is the same 'ctx-parented but not Shutdown-drained' bug class fixed elsewhere via pkgs/worker.SingleRun (see services/autoscaling, services/scheduler for the established pattern). FIXED: added a worker.SingleRun-backed brokerRun field, Broker.Run(ctx) adapter method, and a Handler.Shutdown(ctx) that calls brokerRun.Stop(ctx) and blocks until the broker goroutine actually exits (or ctx is done). Handler now implements both service.BackgroundWorker and service.Shutdowner. Regression test: TestHandlerShutdownDrainsBrokerGoroutine (broker_test.go) starts a real broker and asserts Shutdown returns within 2s of the goroutine actually stopping, not just cancelling and returning immediately."}
---

## Notes

- **IoT is restjson1, not XML** — the "wrong XML list wrappers" bug class from other
  services' parity sweeps doesn't apply here. List/object field names were verified
  directly against `deserializers.go`/`serializers.go` in
  `aws-sdk-go-v2/service/iot@v1.76.0`.
- **Timestamps — CORRECTED from the prior version of this note**: the prior audit pass
  claimed "the service stores/serializes epoch-seconds as `float64` struct fields... [so]
  it was not flagged as a bug." That claim was **only true for part of the service**
  (Job/JobTemplate/CACertificate/OutgoingCertificate/provisioning/packages/metrics). This
  pass found 8 struct families (Policy, GetPolicyOutput, PolicyVersion, ThingType,
  ThingGroup, Certificate, CertificateProvider, TopicRule) that stored `time.Time`
  directly and were marshaled straight into JSON responses — which `encoding/json`
  renders as an RFC3339 **string**, not the epoch-seconds **number** the restjson1
  `DateType` wire format requires. Real AWS SDK deserializers reject a string here
  outright (`"expected ... to be a JSON Number, got string instead"`). All 8 are now
  fixed via `pkgs/awstime.Epoch()` at the handler response-building call site (not by
  changing the struct field type, so no persistence-format changes were needed — the
  internal struct fields are only used for storage/computation, not direct wire
  marshaling). **If re-auditing timestamps in this service, grep for `time.Time` in
  `types.go` and verify every call site that embeds one of those fields into a
  `map[string]any` response wraps it in `awstime.Epoch(...)`.**
- **Looks-wrong-but-correct**: `ListTagsForResource`'s tag objects use capitalized
  `"Key"`/`"Value"` JSON keys (not lowercase `"key"`/`"value"`). Verified directly
  against `awsRestjson1_deserializeDocumentTag` in v1.76.0 — this is genuinely how IoT's
  `Tag` shape serializes, unlike the lowercase convention used by many other AWS
  RESTJSON services. Don't re-flag this.
- **AttributePayload merge semantics**: AWS IoT's `AttributePayload.merge` defaults to
  **false = replace**, not merge — confirmed against `moto`'s reference
  `update_thing`/`update_thing_group`. In both modes, an attribute given an **empty
  string value in the payload is deleted** from the result — this is AWS's documented
  "how to remove an attribute via UpdateThing" mechanism and applies to both UpdateThing
  and UpdateThingGroup. Both are implemented via the shared `applyAttributePayload`
  helper in `backend.go`. Don't revert the "replace is the default" direction without
  re-verifying against real AWS — it's non-obvious and easy to get backwards.
- **Attach op idempotency**: AWS IoT's various Attach* control-plane ops
  (AttachPolicy/AttachPrincipalPolicy/AttachThingPrincipal/AttachSecurityProfile) are
  **set semantics**, not list-append — attaching an already-attached target/principal is
  a no-op success, not a duplicate entry. Fixed via a shared `appendUnique` helper in
  `backend.go`.
- **Certificate transfer lifecycle** (new this pass): AWS IoT's cross-account cert
  transfer is a real state machine — `TransferCertificate` sets `PENDING_TRANSFER` and
  records `(targetAccount, transferMessage)`; `AcceptCertificateTransfer` must validate
  the cert exists AND is `PENDING_TRANSFER`, then moves `ownedBy` → `previousOwnedBy`
  and activates/deactivates per `SetAsActive`; `RejectCertificateTransfer` and
  `CancelCertificateTransfer` both require `PENDING_TRANSFER` and revert to `INACTIVE`
  (with a reject reason recorded for Reject). The certificate's `transferData` wire
  object (`transferDate`/`transferMessage`/`acceptDate`/`rejectDate`/`rejectReason`) is
  only present once a transfer has actually been initiated — real
  `CertificateDescription.transferData` is unset for certs that were never transferred.
  If re-auditing: `AcceptCertificateTransfer` previously validated NOTHING and wrote into
  the *pending-transfers map itself* keyed by any certificate ID (even nonexistent ones)
  — that bug is what `TestCertTransferCount`/`TestPersistenceWithAssociations` exercised
  before this pass; both tests were rewritten to use real certs + real transfers.
- **Persistence is comprehensive**: `persistence.go`'s `backendSnapshot` already covers
  every backend map touched during this audit (including `certificateTransfers`,
  `thingBillingGroups`, `policyTargets`, `thingPrincipals`, `securityProfileTargets`).
  No Handler-level Snapshot/Restore gap was found — `Handler.Snapshot`/`Restore` already
  delegate correctly to the backend when it implements `Snapshottable`.
- **Scope of the 2026-07-23 pass** (commit `135882ff`): closed both previously-filed
  gaps (gopherstack-jy57, gopherstack-ep0r), fixed a real goroutine-leak bug in the
  embedded MQTT broker's Shutdown path, fixed a systemic error-code-mapping bug across
  ~130 call sites, fixed a systemic epoch-timestamp encoding bug across 8 struct
  families, fully closed out `certificate`/`certificate_provider`/`thing_type`/
  `policy_version` families, and made partial progress on `job_and_jobtemplate` and
  `device_defender`. `fleet_indexing` was left entirely untouched, which is why that
  pass's grade stopped at B+.
- **Scope of this pass (2026-07-25)**: closed out `fleet_indexing`, the family the prior
  pass explicitly left untouched. Field-diffed `SearchIndex` (both the `AWS_Things` and
  `AWS_ThingGroups` index result shapes), `DescribeThingGroup`'s `thingGroupMetadata`,
  and `GetCardinality`/`GetStatistics`/`GetPercentiles`/`GetBucketsAggregation` against
  `aws-sdk-go-v2/service/iot@v1.76.0`'s deserializers directly (not against
  gopherstack's own output, per parity-principles.md rule 2). Found and fixed 3 real,
  previously-unflagged wire-shape bugs (see `fleet_indexing`'s `families:` note above):
  a wrong-shaped/wrong-key `parentGroupNames` field and a missing
  `thingGroupDescription` field on `SearchIndex`'s `ThingGroupDocument` results, a
  completely absent `rootToParentThingGroups` field on `DescribeThingGroup`, and a
  missing `sumOfSquares` field on `GetStatistics`. Also spot-checked (not exhaustively
  diffed) `AuditFinding`'s wire shape in `device_defender`: its epoch-timestamp
  encoding (`findingTime`) is correct (already `float64` seconds, consistent with this
  service's Job/JobTemplate/CACertificate timestamp convention), but `isSuppressed`,
  `reasonForNonComplianceCode`, and `taskStartTime` are missing entirely from
  gopherstack's `AuditFinding` type -- left unfixed (large sub-surface, `device_defender`
  remains explicitly `partial`, tracked under gopherstack-srzb along with
  `job_and_jobtemplate`'s remaining advanced-field gaps). This is what justifies A-
  rather than a full A: `fleet_indexing` is now `ok`, but two families remain
  genuinely partial rather than exhaustively verified.
- **Scope of this pass (2026-07-25 #2)**: closed the specifically-flagged `AuditFinding`
  gap (`isSuppressed`/`reasonForNonComplianceCode`/`taskStartTime`) left by the pass
  above, field-diffing against `aws-sdk-go-v2/service/iot@v1.76.0`'s
  `awsRestjson1_deserializeDocumentAuditFinding` directly. Found a fourth real field in
  the same diff, `reasonForNonCompliance` (also entirely missing), and added it too.
  `taskStartTime` is auto-derived from the referenced `AuditTask`'s own `TaskStartTime`
  in `SeedAuditFinding` when a finding has a `taskId` but no explicit `taskStartTime`,
  rather than left unset or requiring every caller to redundantly pass it.

  While closing this, field-diffed `ListAuditFindings` (`ListAuditFindingsInput`) and
  found it was **routed on GET** — real AWS's `ListAuditFindings` is `POST
  /audit/findings` (its filter fields travel in a JSON body, confirmed against
  `serializers.go`'s `awsAwsjson11_serializeOpListAuditFindings`/http bindings), meaning
  the op was **completely unreachable by any real SDK client** before this pass (a real
  client's POST request would never match the GET-only route). This bug was invisible to
  every prior audit pass because the existing test (`TestAuditFinding`) issued a
  hand-constructed GET request that happened to match gopherstack's own (wrong) route,
  rather than going through a real generated SDK client — the exact "tests assert
  against gopherstack's own shape, not real AWS's" trap `parity-principles.md` rule 3
  warns about, just manifesting as a routing bug instead of a field-shape bug this time.
  Fixed the route (GET → POST) and implemented `checkName`/`taskId`/
  `listSuppressedFindings`/`startTime`/`endTime` filtering, previously entirely
  unimplemented (the handler ignored the request body altogether).
  `resourceIdentifier` filtering was deliberately left unimplemented: its real shape has
  roughly 15 optional per-audit-check-type discriminator fields (deviceCertificateId,
  caCertificateId, cognitoIdentityPoolId, iamRoleArn, ...), and this emulator's
  synthetic, freely-shaped `NonCompliantResource map[string]any` cannot honestly
  discriminate against them without guessing per-check-type semantics — see
  `ListAuditFindingsFilter`'s doc comment in `audit.go`.

  Separately, field-diffing `job_and_jobtemplate`'s `JobExecution` shape (explicitly
  flagged as not-yet-diffed by the prior pass) against
  `awsRestjson1_deserializeDocumentJobExecution` found an even more severe version of
  the same routing-bug class: `DescribeJobExecution`/`CancelJobExecution`/
  `DeleteJobExecution` were all routed under `/jobs/{jobId}/things/{thingName}[...]`, a
  path shape no real AWS SDK client has ever sent — real AWS paths these three ops under
  `/things/{thingName}/jobs/{jobId}[...]` (confirmed against `serializers.go`'s http
  bindings for all three operations; `DeleteJobExecution`'s real path additionally
  carries an `/executionNumber/{executionNumber}` URI segment). All three ops were
  therefore **completely unreachable by a real client** — any real request would fall
  through gopherstack's routing entirely and be swallowed by the generic per-Thing CRUD
  dispatcher (`resolveThingsPathOperation`'s `default` branch), which is checked *before*
  the family's own (wrongly-shaped) resolver even runs. Fixed by adding
  `resolveThingJobExecutionOps` (`handler_routing.go`) ahead of that generic fallback,
  removing the old, always-dead `/jobs/{jobId}/things/{thingName}` matching from
  `resolveJobExecutionSubPathOps`, and rewriting `parseJobThingPath` as
  `parseThingJobPath` (`handler_jobs.go`) to parse the real path shape.

  While rewriting these three handlers, also found and fixed: `JobExecution` leaked an
  invented `"thingName"` field on the wire in place of the real `"thingArn"` (real
  `types.JobExecution` has no `thingName` at all); `statusDetails`/`versionNumber`/
  `forceCanceled`/`approximateSecondsBeforeTimedOut` were entirely unmodeled despite all
  being real `JobExecution` fields; and `CancelJobExecution`/`DeleteJobExecution`
  silently ignored `force`/`expectedVersion`/`statusDetails` entirely (a real client
  could never cancel/delete an `IN_PROGRESS`/non-terminal execution, nor would a stale
  `expectedVersion` ever be rejected). Implemented real
  `InvalidStateTransitionException`/`VersionConflictException` semantics (new
  `ErrInvalidStateTransition` sentinel, wired through `writeIoTError`, the single
  error-mapping source of truth from the 2026-07-23 pass).

  Finally, `ListJobExecutionsForJob`/`ListJobExecutionsForThing`'s response shape was
  also wrong: it returned a flat `{"jobId","thingName","status"}` per entry, but real
  AWS's `executionSummaries` is `[]JobExecutionSummaryForJob{thingArn,
  jobExecutionSummary:{executionNumber,queuedAt,startedAt,lastUpdatedAt,status}}` (and
  the `JobExecutionSummaryForThing` sibling with `jobId` instead of `thingArn`) —
  confirmed against `awsRestjson1_deserializeDocumentJobExecutionSummaryForJob`/`ForThing`.
  A real client's deserializer would have found none of the keys it looks for and
  returned entirely empty summaries for every execution. Fixed.

  All of the above (`AuditFinding`'s fields, `ListAuditFindings`' routing+filters,
  `DescribeJobExecution`/`CancelJobExecution`/`DeleteJobExecution`'s routing+wire+state
  semantics, `ListJobExecutionsForJob`/`ForThing`'s response nesting) are covered by new
  table tests: `TestDeviceDefender_AuditFinding_WireFieldsAndFilters`
  (`handler_devicedefender_test.go`) and `TestJobExecution_RoutingAndStateGuards`
  (`handler_jobs_test.go`), plus updates to the pre-existing `TestJobExecutions`/
  `TestJobExecution`/`TestAuditFinding` tests (which previously asserted against the
  wrong, unreachable-by-real-clients path shapes).

  **What remains genuinely partial** (why `job_and_jobtemplate`/`device_defender` stay
  `partial` despite these fixes, holding this service at A- rather than A): Job's more
  advanced optional fields (`jobExecutionsRetryConfig` read-path, `presignedUrlConfig`,
  `jobProcessDetails` rollup counts, `schedulingConfig`, `maintenanceWindows`,
  `destinationPackageVersions`) remain unimplemented, as does the more foundational fact
  that this emulator never fans a `QUEUED` `JobExecution` out per target at `CreateJob`
  time (`CancelJobExecution`'s create-on-miss fallback papers over this for the common
  test case, but it is not a substitute for real per-target execution tracking).
  `device_defender`'s `StartAuditMitigationActionsTask` target resolution, ML-based
  detect models, and violations families remain not exhaustively field-diffed, and
  `ListAuditFindings.resourceIdentifier` filtering remains unimplemented for the reasons
  given above. These are real, substantial, unimplemented sub-surfaces, not proven
  impossibilities — left honestly documented under `deferred:` rather than claimed as
  closed.

  **Superseded by pass #3 below** — every item in the paragraph immediately above is now
  fixed. It's left in place (rather than deleted) because it accurately documents what
  pass #2 genuinely didn't reach, which is useful context for how the gap closed.

- **Scope of this pass (2026-07-25 pass #3)**: closed both `job_and_jobtemplate` and
  `device_defender` — see their `families:` entries above for the full list of fixes.
  Three points worth calling out beyond the `families:`/`ops:` summaries:

  1. **A new bug class this campaign hadn't yet named for this service: wrong HTTP
     method, not just wrong path.** Every prior routing bug found in this service (and
     most of this campaign) was "wrong path shape." This pass found `CreateJob` and
     `CreateJobTemplate` were both routed on `POST` when real AWS IoT uses `PUT`
     (confirmed directly against `awsRestjson1_serializeOpCreateJob`/
     `CreateJobTemplate`'s `request.Method` assignment in `serializers.go`) — same path,
     wrong verb, same result: completely unreachable by any real SDK client. `GetJobDocument`
     had the more familiar wrong-path variant (`/jobs/{jobId}/document` vs. the real
     `/jobs/{jobId}/job-document`). All three were invisible to every prior pass because
     every existing test (`iotOK`/`iotRequest`) called `h.Handler()` directly with a
     hand-picked method string that happened to match gopherstack's own (wrong) routing —
     exactly the `parity-principles.md` rule 3 trap, just manifesting as a wrong verb
     instead of a wrong field.

  2. **A second, deeper bug class: the RouteMatcher whitelist itself, not op dispatch.**
     This service's `Handler.RouteMatcher()` (`matchIoTPath` and its helpers in
     `handler_routing.go`) is a separate, EARLIER gate than `resolveOperation`/op
     dispatch — in a real deployment via `service.NewServiceRouter`, a request must match
     `RouteMatcher()` before the IoT handler is even invoked, let alone before
     `resolveOperation` picks an op. This pass found THREE path families entirely absent
     from that whitelist despite having perfectly correct op-dispatch logic once
     reached: plain `/jobs` (no trailing slash — `ListJobs`), the entire
     `/job-templates` family, and the entire `/mitigationactions/` family
     (`CreateMitigationAction` and its four siblings, foundational to
     `StartAuditMitigationActionsTask`'s whole workflow). Every one of these requests
     404'd before `resolveOperation` ever ran. This was invisible to every prior pass for
     the same underlying reason as point 1: `iotRequest`/`iotOK` call `h.Handler()`
     directly, which bypasses `RouteMatcher()` entirely (that gate only exists in the
     `service.NewServiceRouter` request path). This pass added `newIoTSDKClient`
     (`handler_jobs_test.go`) — a real generated AWS SDK v2 IoT client driven through an
     actual `httptest.Server` + `service.NewServiceRouter`, matching the pattern already
     established in `services/elasticache` — specifically because catching this bug class
     requires exercising the real routing path, not just the handler function. **If
     re-auditing this service (or any service) for routing bugs: a `RouteMatcher()` gap
     is a distinct bug from a `resolveOperation` gap, and only the SDK-client-through-a-
     real-router pattern can find the former.**

  3. **The foundational per-target `JobExecution` fan-out.** `CreateJob` now resolves
     `input.Targets` (thing ARNs directly, or thing-group ARNs expanded to that group's
     direct members — deliberately non-recursive, matching `ListThingsInThingGroup`'s own
     non-recursive semantics rather than inventing recursive-group-membership tracking
     this backend doesn't otherwise have) and creates a real `QUEUED` `JobExecution` row
     for each resolved thing (`fanOutJobExecutionsLocked`, `jobs.go`).
     `AssociateTargetsWithJob` does the same for newly-added targets, and now also merges
     them into the job's own `Targets` list (previously written only to an
     otherwise-never-read `jobTargets` map, so `DescribeJob` never reflected newly
     associated targets at all — a second, smaller bug found alongside the fan-out work).
     `DeleteThing` cascade-deletes any `JobExecution` rows for that thing (mirroring
     `DeleteJob`'s existing cascade over the same `jobId`/`thingName` key from the other
     side), so a deleted thing never leaves a ghost `JobExecution` behind.
     `CancelJobExecution`'s old create-on-miss fallback is kept as a narrow defensive
     backstop (documented in its own doc comment) for the one case fan-out still can't
     cover — a thing-group target with zero members at `CreateJob` time, later joined by
     a thing without this backend simulating AWS's lazy continuous-job rollout — rather
     than removed outright.

  4. **`ListAuditFindings.resourceIdentifier` filtering, previously the family's most-cited
     "can't be done" item, closed.** The prior pass's reasoning — that
     `AuditFinding.NonCompliantResource`'s freeform `map[string]any` couldn't honestly
     discriminate against real AWS's ~10 per-check-type `ResourceIdentifier` fields — was
     correct as far as it went, but the fix was to stop using a freeform map at all: this
     pass modeled `ResourceIdentifier` (and its `NonCompliantResource` parent) as real,
     fully-typed structs matching `types.ResourceIdentifier`/`types.NonCompliantResource`
     exactly (`account`/`caCertificateId`/`clientId`/`cognitoIdentityPoolId`/
     `deviceCertificateArn`/`deviceCertificateId`/`iamRoleArn`/
     `issuerCertificateIdentifier`/`policyVersionIdentifier`/`roleAliasArn`). Once the
     shape itself is real, the filter's semantics collapse to the same "every field SET
     on the filter must be present and equal on the target" pattern every other filter in
     this service already implements — no per-check-type guessing required, because
     callers (`SeedAuditFinding`) simply populate whichever field is appropriate to the
     check they're simulating, exactly as real AWS does per finding.

  5. **A genuinely new, previously-untracked gap surfaced along the way, in a different
     family.** Investigating `ListActiveViolations`/`ListViolationEvents`'
     `listSuppressedAlerts` and `behaviorCriteriaType` filters led to discovering that
     `CreateSecurityProfile` doesn't persist `Behaviors`/`AlertTargets`/
     `AdditionalMetricsToRetain(V2)`/`MetricsExportConfig` AT ALL — `SecurityProfile`
     never modeled them, despite `ValidateSecurityProfileBehaviors` (a separate,
     standalone validation-only endpoint) already having a reasonably rich
     `SecurityProfileBehavior{Name,Metric,Criteria}` shape that's never connected to an
     actual stored profile. `listSuppressedAlerts` was still honestly implementable (see
     `ListActiveViolations`' `ops:` entry — suppression modeled as a directly-seedable
     flag, mirroring `AuditFinding.IsSuppressed`'s identical precedent elsewhere in this
     exact service), but `behaviorCriteriaType` genuinely is not: there is no
     behavior-criteria-type data anywhere in this backend to filter on, and building it
     would mean implementing the missing `SecurityProfile.Behaviors` persistence first —
     a distinct, substantial project belonging to a `security_profiles` family this
     service has never tracked before, not a `device_defender` sub-item. Filed as a new
     `security_profiles` `families:` entry and `gaps:`/`deferred:` item rather than
     silently worked around or ignored — see those sections above. This is the sole
     reason `overall:` stays `A-` rather than `A` despite both of this pass's two
     assigned families (`job_and_jobtemplate`, `device_defender`) now being genuinely
     `ok`.

- **Scope of this pass (2026-07-25 pass #4)**: closed `security_profiles`, the sole
  family pass #3 left partial, bringing `overall:` to `A`. Two parts:

  1. **Behaviors/AlertTargets/AdditionalMetricsToRetain(V2)/MetricsExportConfig
     persistence**, field-diffed against `types.CreateSecurityProfileInput`/
     `UpdateSecurityProfileInput`/`DescribeSecurityProfileOutput`/
     `UpdateSecurityProfileOutput` (v1.76.0). All five request fields, previously
     silently dropped, are now modeled on `SecurityProfile` and wired end to end:
     request parsing, backend storage, response wire shape, and persistence (no
     `persistence.go` changes needed — `SecurityProfile` already round-trips via the
     generic `store.Table[SecurityProfile]` registry, which marshals the full struct).
     `UpdateSecurityProfile` was rebuilt from a single description-only field into the
     real `UpdateSecurityProfileInput` shape, including `ExpectedVersion`'s
     optimistic-lock semantics (`expectedVersion` is a query parameter, not a body
     field — confirmed against `awsRestjson1_serializeOpHttpBindingsUpdateSecurityProfileInput`)
     and every `DeleteX`-flag-vs-field mutual-exclusion rule. Also fixed an invented-field
     leak found in the same diff: `SecurityProfile.Tags` was surfaced on
     Describe/Update responses, but real `DescribeSecurityProfileOutput`/
     `UpdateSecurityProfileOutput` have no `"tags"` field at all (tags are only
     retrievable via the separate `ListTagsForResource` op) — fixed via `json:"-"`.
     Closing this unblocked `ListActiveViolations`/`ListViolationEvents`'
     `behaviorCriteriaType` filter (`device_defender` family), now resolved live
     against each violation's owning security profile's real stored `Behaviors`.

  2. **The routing sweep the task brief explicitly required** ("check routing while
     you are there — three prior passes each found ops unreachable by a real
     client"). Every security-profile op was driven through a real generated AWS SDK
     v2 IoT client against the actual `service.Router` path (`newIoTSDKClient`,
     already established by pass #3 for exactly this purpose), not just
     `h.Handler()` directly. This family had never been checked this way before, and
     it found two more real, previously-undiscovered bugs of the exact same classes
     prior passes found elsewhere in this service: a `RouteMatcher`-whitelist gap
     (`ListSecurityProfiles`' plain `"/security-profiles"`, no trailing slash — same
     shape as `ListJobs`' own prior-pass gap — and `ListSecurityProfilesForTarget`'s
     `"/security-profiles-for-target"` were both entirely absent from
     `matchCoreIoTPathSecondary`, `handler_routing.go`, despite `resolveSecurityProfileOps`
     already dispatching both correctly), and three wire-shape key-name bugs
     (`ListSecurityProfiles`/`ListTargetsForSecurityProfile`/
     `ListSecurityProfilesForTarget`'s list-entry shapes used invented or
     full-length keys in place of the real, shortened `SecurityProfileIdentifier`/
     `SecurityProfileTarget`/`SecurityProfileTargetMapping` keys — confirmed against
     `awsRestjson1_deserializeDocumentSecurityProfileIdentifier`/`SecurityProfileTarget`/
     `SecurityProfileTargetMapping`). All fixed, along with two smaller bugs found in
     the same sweep: `DetachSecurityProfile` never mirrored `AttachSecurityProfile`'s
     existing `gopherstack-ep0r` existence-validation fix, and `DeleteSecurityProfile`
     never cascade-cleaned its target-attachment map entry (a ghost row a same-named
     profile re-created later would incorrectly inherit). See the `security_profiles`
     `families:` entry's "ROUTING VERIFIED" paragraph and the new per-op `ops:`
     entries above for full detail, and `TestSecurityProfile_RoutingWireShapesAndBehaviorCriteriaType_SDKRoundTrip`/
     `TestSecurityProfile_DetachNotFoundAndDeleteCascade`
     (`handler_security_profiles_test.go`) plus two new `TestHandler_RouteMatcher`
     cases (`handler_routing_test.go`) for the regression coverage.
- **Broker capability addition (parity-5, gopherstack-polh, no grade change --
  `Broker` is internal plumbing, not an AWS `iot` wire op):** `Broker` (broker.go)
  now implements two new methods consumed by `services/iotdataplane`'s
  `MQTTPublisher` interface: `ClientSubscriptions(clientID) (map[string]byte, bool)`,
  reading a connected client's real live subscriptions off mochi-mqtt's
  `cl.State.Subscriptions.GetAll()`, and `SendToClient(clientID, topic, payload, qos) (bool, error)`,
  writing a PUBLISH packet straight to one client's connection via
  `cl.WritePacket`, bypassing topic subscription matching entirely (mirrors
  real AWS `SendDirectMessage`'s documented "receiving client does not need
  to subscribe" semantics). Both are proven against a REAL mochi-mqtt session
  — a live `paho.mqtt.golang` client connected over real TCP loopback, not a
  mock — by `TestBroker_ClientSubscriptionsAndSendToClient` (`broker_test.go`).
  This closes the `services/iotdataplane` `ListSubscriptions`/`SendDirectMessage`
  gaps previously blocked on this exact interface boundary (see
  `services/iotdataplane/PARITY.md`'s gaps list for the resolution writeup).
  `export_test.go` was NOT touched for this — the new test drives the real
  broker entirely through already-exported API (`NewBroker`/`Start`/
  `ClientSubscriptions`/`SendToClient`) plus a real TCP client, no whitebox
  hooks needed.
- **Scope of pass #5 (2026-08-13, gopherstack-oc9v)**: this campaign targets a
  coverage blind spot in the sweep *tooling*, not this file's wire-shape
  content — handlers whose request is an anonymous inline `struct{...}`
  literal generate no candidate for the repo's name-regex wire-diff sweep, so
  a wrong-name or dropped field on one of them is invisible to that tooling
  regardless of how correct the field values themselves are. iot has 79 such
  structs (`grep -c 'var req struct\|var body struct'` across non-test
  `services/iot/*.go`), the third-largest concentration repo-wide behind
  sagemaker (362) and cleanrooms (97).

  Per the campaign's stated method (proven by sagemaker's earlier passes):
  read `PARITY.md` first and scope to what it shows as genuinely uncovered,
  rather than re-deriving already-verified work. This file showed `overall:
  A` with every family `ok` except `fleet_metric`, explicitly `partial` (the
  one item under `gaps:`) — so this pass scoped there. Converted
  `fleet_metric`'s 3 inline structs (`UpdateFleetMetric`,
  `UpdateCustomMetric`, `UpdateDimension`, all in `handler_metrics.go`) to
  named types (`UpdateFleetMetricInput`/`UpdateCustomMetricInput`/
  `UpdateDimensionInput`, `metrics.go`) and field-diffed the whole family
  (`Create`/`Describe`/`List`/`Update`/`Delete` FleetMetric) against
  `aws-sdk-go-v2/service/iot@v1.77.4` directly. This closed the
  gap a prior pass noted (`UpdateFleetMetric` dropping
  `indexName`/`aggregationType`/`aggregationField`/`queryVersion`/`unit`) and
  surfaced a sibling, previously-untracked one on `CreateFleetMetric`
  (`aggregationField`/`aggregationType`, both `This member is required` on
  the real `CreateFleetMetricInput`) that no prior wire-diff pass had found —
  exactly the failure mode gopherstack-oc9v predicts: the bug was invisible
  to the name-regex sweep because `CreateFleetMetricInput` in this codebase
  was *already* a named type (so it wasn't part of the 79-count at all), but
  nobody had actually diffed its field set against the real SDK type before
  this pass, because the campaign that would have prompted that diff had
  never been run. See the `fleet_metric` `families:` entry above for the
  full fix (new `AggregationType{Name,Values}` type, threaded through
  `Create`/`Update`/response wire shape) and
  `TestFleetMetric_AggregationAndUpdateFields` (`handler_metrics_test.go`)
  for the regression, confirmed to fail against the pre-fix code by manually
  reverting the field-wiring and re-running before restoring it.

  `UpdateCustomMetric`/`UpdateDimension`'s inline structs were already
  field-complete against `UpdateCustomMetricInput`/`UpdateDimensionInput` —
  converted to named types for tooling visibility only, no bug found.

  **Not done by this pass, still exposed to the blind spot**: 76 of iot's 79
  anonymous inline request structs remain unconverted (only the 3 in
  `fleet_metric` were addressed) — every op family other than
  `fleet_metric` was left exactly as pass #4 verified it, on the read-first
  finding that those families were already `ok`. Converting the rest and
  wire-diffing each is real, substantial, unstarted work; the next pass on
  this service for this campaign should pick a family (or run a full
  `grep -n 'var req struct\|var body struct' services/iot/*.go` sweep) rather
  than assume `overall: A` means the inline-struct blind spot is closed here
  — it means the *content* that pass #4's tooling could see was verified, not
  that every request shape has been read as a named type against the pinned
  SDK.

- **Scope of this pass (2026-08-23)**: this file's `overall: A` and near-universal
  `families: ok` mask a real gap — **155 of iot's 276 ops (56%) were never named
  anywhere in this file's prose**, derived by diffing every op name in `op_names.go`
  against a grep of every op name mentioned anywhere in this file. Of those 155, 103
  have a real response body (confirmed per-op against
  `awsRestjson1_deserializeOpDocument<Op>Output` in
  `aws-sdk-go-v2/service/iot@v1.77.4/deserializers.go`); this pass audited 53 of those
  103. The other 52 have empty/void outputs and were only spot-checked. Found and fixed
  18 real bugs spanning 22 ops, none previously flagged, all field-diffed against the
  pinned SDK's deserializer case list (not against gopherstack's own output):
  - `SetDefaultAuthorizer`/`DescribeDefaultAuthorizer` (`handler_authorizers.go`):
    `SetDefaultAuthorizer` dropped the required `authorizerArn` entirely;
    `DescribeDefaultAuthorizer` returned only `{authorizerDescription:{authorizerName}}`
    instead of the full `AuthorizerDescription` shape — a real client got almost
    nothing back. Both now look up the full `Authorizer` record.
  - `CreateProvisioningClaim` (`certificates.go`): backend never created a real
    certificate record at all — response was missing `certificateId`/`expiration`
    entirely (`CreateProvisioningClaimOutput`, both present in the real deserializer).
    Now creates a `PENDING_ACTIVATION` certificate and returns a real 5-minute
    expiration, matching AWS's fleet-provisioning claim-cert TTL.
  - `RegisterCACertificate` (`certificates.go`/`handler_certificates.go`): the request
    struct bound the real `registrationConfig` object (a nested struct on the wire) to
    a field typed `string` — any real client that supplies `registrationConfig` (the
    normal way to enable JITP) got a hard `400`/decode failure, confirmed live
    (`json: cannot unmarshal object into Go struct field .registrationConfig of type
    string`). `DescribeCACertificateOutput`'s sibling top-level `registrationConfig` key
    was also entirely absent from the response. Both fixed: real `RegistrationConfig`
    type added, threaded through Register→Describe.
  - `ListCACertificates`/`ListCertificatesByCA` (`handler_certificates.go`): list items
    were missing `creationDate` (both) and `certificateMode` (`ListCertificatesByCA`
    only) — fields the backend already tracked on every certificate but the handler
    never surfaced, unlike the sibling `ListCertificates` op which already got this right.
  - `CreateProvisioningTemplate`/`CreateProvisioningTemplateVersion`
    (`handler_provisioning.go`): `CreateProvisioningTemplate` dropped `defaultVersionId`
    (backend already sets it to `1`). `CreateProvisioningTemplateVersion` dropped
    `templateArn`/`isDefaultVersion` from the response *and* silently ignored the real
    `setAsDefault` input field entirely — sending it had no effect on which version was
    default. Now honors `setAsDefault` (flips the prior default version off, updates the
    parent template's `defaultVersionId`) and returns the full real shape.
  - `CreateOTAUpdate`/`GetOTAUpdate`/`ListOTAUpdates` (`ota_updates.go`/
    `handler_ota_updates.go`): `CreateOTAUpdateOutput`'s `awsIotJobArn`/`awsIotJobId`
    were entirely unmodeled (now synthesized as `AFR_OTA-<otaUpdateId>`, matching AWS's
    documented job-ID convention, with a real job ARN). Separately, the OTA update's
    file list was serialized under the wrong key `files` instead of the real
    `otaUpdateFiles` — confirmed against `awsRestjson1_deserializeDocumentOTAUpdateInfo`
    — so a real client's `OtaUpdateFiles` field silently decoded empty even when files
    were set. `ListOTAUpdates` entries were also missing `creationDate`.
  - `ListStreams`/`UpdateStream` (`handler_streams.go`): list entries were missing
    `description`/`streamVersion` (`StreamSummary`); `UpdateStream`'s response was
    missing `description` (`UpdateStreamOutput`) — all three fields the backend already
    tracked on every stream.
  - `CreateTopicRuleDestination`/`GetTopicRuleDestination`/`ListTopicRuleDestinations`
    (`handler_topic_rules.go`): all three only ever returned `{arn, status}`, dropping
    `httpUrlProperties`/`httpUrlSummary` entirely even though the backend already builds
    and stores it (`TopicRuleDestination.HTTPURLProperties`) — the confirmation URL for
    an HTTP destination was never visible to a real client calling any of these three
    ops. Note the wrapper key differs by op: `httpUrlProperties`
    (`types.TopicRuleDestination`, Create/Get) vs. `httpUrlSummary`
    (`types.TopicRuleDestinationSummary`, List) — same shape, different key, confirmed
    against both deserializer functions.
  - `GetPackageVersion`/`CreatePackageVersion` (`packages.go`/`handler_packages.go`):
    `GetPackageVersion` never surfaced `sbom`/`sbomValidationStatus` even though
    `AssociateSbomWithPackageVersion` already stores both in a side map keyed by
    package/version — now merged in at read time. `CreatePackageVersion` also silently
    dropped three real `CreatePackageVersionInput` fields it never even declared in its
    request struct — `attributes`, `recipe`, `artifact` — now parsed, persisted, and
    echoed back on both Create and Get.
  - `UpdateCertificateProvider` (`handler_certificates.go`): returned a bare `204`-style
    empty `200` body — the real `UpdateCertificateProviderOutput` has
    `certificateProviderArn`/`certificateProviderName`, both non-optional in practice.
    A real client got nothing back at all, not just a partial shape. Now looks up the
    updated record and returns the full real body.
  - `DescribeProvisioningTemplate`/`ListProvisioningTemplates`
    (`provisioning.go`/`handler_provisioning.go`): `preProvisioningHook`
    (`types.ProvisioningHook`) was entirely unmodeled — not in the backend struct, not
    parseable from `CreateProvisioningTemplateInput`, so `DescribeProvisioningTemplate`
    could never return it no matter what a real client sent on Create. Added the type,
    threaded it through Create→Describe. Separately, `ListProvisioningTemplates`
    entries dropped `type` (`ProvisioningTemplateSummary`) even though the backend
    already tracks `TemplateType` on every template.

  **Modelling gaps found, not fixed** (real SDK members with no backend concept at
  all, so nothing to field-diff against — per `parity-principles.md`'s no-synthesis
  rule these are reported, not guessed at):
  - `DescribeDomainConfiguration` (`provisioning.go`): the backend's
    `DomainConfiguration` struct models only `domainName`/`serviceType`/
    `domainConfigurationStatus`/`domainType`/dates. The real
    `types.DomainConfiguration` additionally has `applicationProtocol`,
    `authenticationType`, `authorizerConfig`, `clientCertificateConfig`,
    `lastStatusChangeDate`, `serverCertificateConfig`, `serverCertificates`, and
    `tlsConfig` — an entire unmodeled subsystem (custom-domain TLS/mTLS/authorizer
    configuration), not a wire-shape omission fixable at the handler layer.
  - `CreatePackageVersion`'s `errorReason` (`PackageVersionSummary`/`PackageVersion`):
    legitimately absent — gopherstack's package versions never transition to a
    `FAILED` state asynchronously the way real AWS package-version processing can, so
    there is no backing state to surface. Not a bug; would need a fabricated failure
    mode to populate.

  **Ops audited and found already correct** (field-diffed, no gap): `CreateAuthorizer`,
  `DescribeAuthorizer`, `ListAuthorizers`, `UpdateAuthorizer`, `CreateCertificateFromCsr`,
  `CreateCertificateProvider`, `CreateKeysAndCertificate`, `DescribeCACertificate`
  (aside from the `registrationConfig` fix above), `ListCertificateProviders`,
  `RegisterCertificate`, `RegisterCertificateWithoutCA`, `CreateCustomMetric`,
  `CreateDimension`, `DescribeCustomMetric`, `DescribeDimension`, `ListCustomMetrics`,
  `ListDimensions`, `CreateRoleAlias`, `DescribeRoleAlias`, `ListRoleAliases`,
  `UpdateRoleAlias`, `CreateDomainConfiguration`, `UpdateDomainConfiguration`,
  `DescribeProvisioningTemplateVersion`, `ListProvisioningTemplateVersions`,
  `DescribeBillingGroup`, `ListBillingGroups`, `CreatePackage`, `GetPackage`,
  `DescribeStream`, `CreateStream`.

  **Ops named in the never-named-155 list but not reached this pass** — 52 void-output
  ops (spot-checked only, not field-diffed): `AddThingToThingGroup`,
  `CancelDetectMitigationActionsTask`, `ClearDefaultAuthorizer`,
  `ConfirmTopicRuleDestination`, `CreateAuditSuppression`, `DeleteAccountAuditConfiguration`,
  `DeleteAuditSuppression`, `DeleteAuthorizer`, `DeleteBillingGroup`, `DeleteCACertificate`,
  `DeleteCertificate`, `DeleteCertificateProvider`, `DeleteCustomMetric`, `DeleteDimension`,
  `DeleteDomainConfiguration`, `DeleteDynamicThingGroup`, `DeleteFleetMetric`,
  `DeleteOTAUpdate`, `DeletePackage`, `DeletePackageVersion`, `DeletePolicyVersion`,
  `DeleteProvisioningTemplate`, `DeleteProvisioningTemplateVersion`, `DeleteRegistrationCode`,
  `DeleteRoleAlias`, `DeleteScheduledAudit`, `DeleteStream`, `DeleteThingType`,
  `DeleteTopicRuleDestination`, `DeleteV2LoggingLevel`, `DetachPrincipalPolicy`,
  `DetachThingPrincipal`, `DisassociateSbomFromPackageVersion`,
  `PutVerificationStateOnViolation`, `RemoveThingFromThingGroup`, `SetDefaultPolicyVersion`,
  `SetLoggingOptions`, `SetV2LoggingLevel`, `SetV2LoggingOptions`, `StopThingRegistrationTask`,
  `UpdateAuditSuppression`, `UpdateCACertificate`, `UpdateCertificate`,
  `UpdateEncryptionConfiguration`, `UpdateEventConfigurations`, `UpdateIndexingConfiguration`,
  `UpdateJob`, `UpdatePackage`, `UpdatePackageVersion`, `UpdateProvisioningTemplate`,
  `UpdateThingGroupsForThing`, `UpdateTopicRuleDestination`.

  50 has-body ops from the never-named-155 list remain genuinely unaudited by any pass
  (this one included): `CancelJob`, `CreateBillingGroup`, `CreateDynamicThingGroup`,
  `UpdateBillingGroup`, `UpdateDynamicThingGroup`, the `audit`/`device_defender` families
  (`DescribeAuditMitigationActionsTask`, `ListAuditMitigationActionsExecutions`,
  `ListAuditSuppressions`, `ListAuditTasks`, `ListRelatedResourcesForAuditFinding`,
  `DescribeAccountAuditConfiguration`, `DescribeAuditSuppression`, `DescribeAuditTask`,
  `CreateScheduledAudit`, `DescribeScheduledAudit`, `ListScheduledAudits`,
  `StartOnDemandAuditTask`, `UpdateScheduledAudit`), most of `handler_routing.go`'s
  grab-bag (`DescribeEncryptionConfiguration`, `DescribeEventConfigurations`,
  `DescribeManagedJobTemplate`, `DescribeThingRegistrationTask`,
  `GetBehaviorModelTrainingSummaries`, `GetThingConnectivityData`, `ListDomainConfigurations`,
  `ListManagedJobTemplates`, `ListMetricValues`, `ListOutgoingCertificates`,
  `ListThingGroupsForThing`, `ListThingPrincipals`, `ListThingPrincipalsV2`,
  `ListThingRegistrationTaskReports`, `ListThingRegistrationTasks`, `RegisterThing`,
  `StartThingRegistrationTask`, `TestAuthorization`, `TestInvokeAuthorizer`),
  `handler_policies.go`'s (`GetEffectivePolicies`, `ListPolicyPrincipals`,
  `ListPrincipalPolicies`, `ListPrincipalThings`, `ListPrincipalThingsV2`,
  `ListTargetsForPolicy`), `handler_logging.go` (`GetLoggingOptions`,
  `GetV2LoggingOptions`, `ListV2LoggingLevels`), `handler_indexing.go`
  (`GetIndexingConfiguration`), `GetRegistrationCode`, `GetPackageConfiguration`, and
  `ListSbomValidationResults`. These are the next pass's fresh ground — none have ever
  been named in this file.

## over-wide/wrong-name list responses (gopherstack-g3jk, gopherstack-k26u)

Three `List*` ops (`ListCommands`, `ListPackages`, `ListPackageVersions`,
`handler_commands.go`/`handler_packages.go`) shared one root cause: the
handler `c.JSON`'d the raw `[]*IoTCommand`/`[]*IoTPackage`/`[]*IoTPackageVersion`
straight from the backend, marshaled by that domain struct's own JSON tags
with no per-op summary DTO -- so every internal field (`tags`, `payload`,
`description`, `namespace`, `packageArn`, `packageVersionArn`) leaked onto
the wire regardless of what the real `CommandSummary`/`PackageSummary`/
`PackageVersionSummary` types (`types.go:1504-1527`/`3386-3401`/`3413-3433`,
`iot@v1.77.4`) declare. Fixed by copying the pattern this service's own
`ListCertificates`/`ListThingTypes`/`ListThingGroups` already used correctly:
a small per-op `*SummaryFields` function building a scoped `map[string]any`.
An SDK-driven client cannot prove an over-wide response is fixed -- its
deserializer silently drops unrecognized members, so both the buggy and
fixed shapes decode identically -- so these three are proven with a raw-body
assertion instead (`TestListCommands_SummaryScoping`,
`TestListPackages_SummaryScoping`, `TestListPackageVersions_SummaryScoping`).

Reading each op's real Output struct while fixing the above surfaced two
further, previously-undetected bugs beyond what the over-wide sweep was
looking for:

- `ListCommands`' raw struct tag was `"creationDate"`; the real
  `CommandSummary.CreatedAt` wire key is `"createdAt"`
  (`deserializers.go`, `awsRestjson1_deserializeDocumentCommandSummary`) --
  a silent wrong-name bug riding along with the over-wide one, now fixed as
  part of the same DTO.
- `ListPackages`/`ListPackageVersions` wrapped their list under the
  fabricated `"packageList"`/`"packageVersionList"` keys; the real
  `ListPackagesOutput`/`ListPackageVersionsOutput` wrap under
  `"packageSummaries"`/`"packageVersionSummaries"`
  (`awsRestjson1_deserializeOpDocumentListPackagesOutput`/
  `...ListPackageVersionsOutput`). A real client's list was **always
  empty**, regardless of backend state -- worse than the over-wide leak
  itself. Fixed alongside the summary scoping.

`ListCommandExecutions` (`handler_commands.go`, `IoTCommandExecution`,
`commands.go`) had the wrong-name bug gopherstack-k26u flagged --
`"thingArn"` where the real `CommandExecutionSummary.TargetArn`
(`types.go:1327-1352`) wire key is `"targetArn"` -- plus never emitted
`CompletedAt`/`StartedAt`. Those two are deliberately left absent rather
than fabricated: this backend has no `StartCommandExecution`/
`UpdateCommandExecution` control-plane op, so there is no honest source for
a start or completion time distinct from `CreatedAt` (see the doc comment on
`commandExecutionSummaryFields`). Reading the operation's full real shape
(not just the flagged field) surfaced a third, more severe bug: the real
`ListCommandExecutions` is `POST /command-executions` with filters
(`commandArn`/`targetArn`/`status`) in the JSON body
(`serializers.go:13785`, `awsRestjson1_serializeOpListCommandExecutions`) --
this service's `RouteMatcher` (`matchIoTPath`) never matched the bare
`/command-executions` path at all (only `/command-executions/{id}`), so a
real client's `ListCommandExecutions` call 404'd outright, never reaching
resolveOperation. Fixed: `matchFinalOpsPath` now also matches the bare path,
`resolveCommandOps` resolves `POST /command-executions` to
`opListCommandExecutions`, and the handler parses filters from the body via
a new `Backend.ListCommandExecutionsByFilter`. The pre-existing fictional
`/commands/{commandId}/executions` route (untested, unreachable by any real
client, but not proven unused) is left wired for backward compatibility.
`route_matcher_whitebox_test.go`'s `TestRouteMatcher_ExhaustiveCoverage`
previously carried `/command-executions` in `knownUnmatchedIoTPathsRaw` as a
deliberately-out-of-scope gap from the earlier tags sweep (gopherstack-2mwl)
-- removed now that it matches. Because this bug is a wrong key **plus** an
unreachable route, only driving the real `aws-sdk-go-v2` client proves the
fix (a raw-body assertion would pass against the old fictional route without
ever exercising the real one); see `TestSDKRoundTrip_ListCommandExecutions`.

**Fixed (gopherstack-8ez0)**: `GetCommandExecution`'s real route (`GET
/command-executions/{executionId}?targetArn=...`, no `commandId`) already
matched `matchIoTPath` (via the same prefix rule the `ListCommandExecutions`
fix above extended), but `resolveFinalOpsGroupB` only resolved that path
prefix for `DELETE` (`DeleteCommandExecution`) -- `GET` fell through to
`unknownOperation`, so a real client's `GetCommandExecution` 400'd with
"unknown operation" before ever reaching the handler, the same failure mode
as the `ListCommandExecutions` bug above. Fixed: `resolveFinalOpsGroupB` now
also resolves `GET` for that prefix. The existing `GetCommandExecutionInput`
requires `TargetArn`, but executions were only ever stored/addressed by
`commandID+executionID` internally, so a new `Backend.GetCommandExecutionByID
(executionID, targetARN string)` was added that scans by `ExecutionID` alone
(optionally scoped by `targetARN`), mirroring `DeleteCommandExecution`'s
existing lookup-by-executionID pattern -- `handleGetCommandExecution` now
branches on the real top-level path vs. the pre-existing fictional nested
`/commands/{commandId}/executions/{executionId}` route the same way
`handleListCommandExecutions` already does. Reading the full real
`GetCommandExecutionOutput` shape while fixing this also surfaced that both
routes had been serializing the raw `IoTCommandExecution` struct directly,
whose own JSON tag is `"thingArn"` -- not a member `GetCommandExecutionOutput`
declares at all; the real key is `"targetArn"`. Both routes now render
through `commandExecutionSummaryFields` (the same scoped map
`ListCommandExecutions` already used), which happens to cover every field
`GetCommandExecutionOutput` and this backend can honestly source in common:
`CommandArn`, `CreatedAt`, `ExecutionId`, `Status`, `TargetArn`.
`ExecutionTimeoutSeconds`, `LastUpdatedAt`, `Parameters`, `Result`,
`StatusReason`, `CompletedAt` and `StartedAt` all stay absent for the same
reason those last two already did: no `StartCommandExecution`/
`UpdateCommandExecution` control-plane op exists to source them from.
Because this is a wrong key **plus** a previously-unreachable route, only
driving the real `aws-sdk-go-v2` client proves the fix
(`TestSDKRoundTrip_GetCommandExecution`); reverting the new `resolveFinalOpsGroupB`
case by hand reproduces the original 400 ("unknown operation") against the
`found_by_execution_id_and_target_arn` case.

Swept the rest of the command/command-execution family against the pinned
SDK's own `serializers.go` HTTP bindings while here: `CreateCommand`
(`PUT /commands/{commandId}`), `GetCommand`/`UpdateCommand`/`DeleteCommand`
(`GET`/`PATCH`/`DELETE /commands/{commandId}`), `ListCommands`
(`GET /commands`) and `DeleteCommandExecution`
(`DELETE /command-executions/{executionId}`) all already matched their real
routes correctly -- `GetCommandExecution` was the only unreachable op left in
the family.

## 2026-08-23 (pass #6, continued): audit/device-defender family + singleton ops

**Re-derived the never-audited count instead of trusting the prior pass's prose.**
Diffed every op name in `op_names.go` (276 total) against every op name appearing
anywhere in this file -- including the structured `ops:`/`families:` blocks, not
just prose -- distinguishing "named in an actually-audited context" from "merely
listed in the prior pass's own not-reached enumeration" (a name-only grep can't
tell those apart, since the prior pass's own two 52/50-item lists name every op
it did NOT audit). Excluding lines that only enumerate not-yet-reached ops, 174
of 276 ops resolve to genuinely-audited context (the `ops:` block, `families:`
prose, the Commands over-wide-response section, and the 2026-08-23 pass's own
found-and-fixed/audited-correct lists) and **102 do not -- the 102 figure holds
exactly**, matching the prior pass's own two lists (52 void-output + 50
has-body) name-for-name.

**Audited 19 of the 102** (13 from the audit/device-defender family, plus 6
singletons): `DescribeAuditMitigationActionsTask`, `ListAuditMitigationActionsExecutions`,
`ListAuditSuppressions`, `ListAuditTasks`, `ListRelatedResourcesForAuditFinding`,
`DescribeAccountAuditConfiguration`, `DescribeAuditSuppression`, `DescribeAuditTask`,
`CreateScheduledAudit`, `DescribeScheduledAudit`, `ListScheduledAudits`,
`StartOnDemandAuditTask`, `UpdateScheduledAudit`, `CreateBillingGroup`,
`UpdateBillingGroup`, `CancelJob`, `GetRegistrationCode`, `GetPackageConfiguration`,
`ListSbomValidationResults`. Found and fixed 5 real bugs across 4 ops (a ~26% hit
rate), field-diffed against the pinned SDK's deserializer/type definitions, not
against gopherstack's own output:

- `UpdateAccountAuditConfiguration`/`DescribeAccountAuditConfiguration`
  (`audit.go`): real `types.AuditCheckConfiguration` (v1.77.4) has both
  `enabled` and `configuration` (`map[string]string`); this backend's
  `AuditCheckConfig` only ever modeled `enabled`, so a real client's
  `UpdateAccountAuditConfiguration` call setting per-check configuration
  values had them silently dropped, and `DescribeAccountAuditConfiguration`
  could never surface them. Fixed: added `Configuration` to `AuditCheckConfig`
  (purely additive; both request-parsing and response marshaling already bind
  the same struct, so no handler change was needed beyond the type). See
  `TestUpdateAccountAuditConfiguration_ConfigurationFieldSurvives`.
- `DescribeScheduledAudit` (`audit.go`): `ScheduledAudit.Tags` (internal
  write-only scratch state -- the canonical tag store is the separately
  persisted `resourceTags` map, confirmed by grep: `sa.Tags` is written once
  at Create and never read back) was tagged `json:"tags,omitempty"`, leaking
  onto the wire even though real `DescribeScheduledAuditOutput` has no
  `tags` member -- the same leaked-field class already fixed for
  Job/JobTemplate/SecurityProfile. Fixed via `json:"-"`.
- `ListScheduledAudits` (`handler_audit.go`): `ScheduledAuditMetadata`
  (v1.77.4) has `dayOfMonth`/`dayOfWeek`; the backend already tracks both per
  scheduled audit (set on `CreateScheduledAudit`, mutable via
  `UpdateScheduledAudit`) but the list summary only ever emitted
  `scheduledAuditName`/`scheduledAuditArn`/`frequency`. Fixed to include both.
  See `TestScheduledAudit_ListFieldsAndDescribeWireShape` (covers both fixes
  above, table-driven over MONTHLY/WEEKLY).
- `DescribeAuditMitigationActionsTask` (`handler_devicedefender.go`): real
  `DescribeAuditMitigationActionsTaskOutput.actionsDefinition`
  (`[]types.MitigationAction`, v1.77.4) was never surfaced at all -- a real
  client's deserializer never found the key and it stayed permanently empty.
  This is the same field its sibling `DescribeDetectMitigationActionsTask`
  already resolves via the existing `MitigationActionRefs` helper (fixed in
  an earlier pass), just never given the same treatment on the audit side.
  Fixed via a new `auditMitigationTaskActionsDefinition` helper that
  collects the unique action names across `AuditCheckToActionsMapping`'s
  per-check lists and resolves them the same way. See
  `TestDeviceDefender_AuditMitigationTaskLifecycle`'s new `actionsDefinition`
  assertions.
- `CancelJob` (`jobs.go`/`handler_jobs.go`): two bugs. (1) real
  `CancelJobOutput` has a `description` member this handler never returned
  (the backend already tracks `Job.Description`). (2) `CancelJob`
  unconditionally set `Status` to `CANCELED` regardless of the job's current
  state, silently "re-canceling" an already-`COMPLETED`/`CANCELED`/`FAILED`/
  `DELETION_IN_PROGRESS` job instead of returning
  `InvalidStateTransitionException` -- the same terminal-state guard
  `CancelJobExecution`/`CancelAuditTask` already enforce elsewhere in this
  service. Both fixed. See `TestCancelJob_DescriptionAndTerminalStateGuard`.

**Sibling check**: yes -- the `DescribeAuditMitigationActionsTask` bug is a
direct case of "a correct sibling beside a broken op" (shape #7): its
detect-mitigation sibling already had the `actionsDefinition` fix from an
earlier pass, but the audit-mitigation side was never given the same
treatment despite sharing the identical `MitigationActionRefs` resolution
mechanism. The other three fixes (`AuditCheckConfig.Configuration`,
`ScheduledAudit.Tags`, `ListScheduledAudits` fields) have no direct sibling
within their own small families -- checked and none found.

**Audited and found already correct** (field-diffed, no gap):
`ListAuditMitigationActionsExecutions`, `ListAuditSuppressions`, `ListAuditTasks`,
`DescribeAuditSuppression`, `CreateScheduledAudit`, `StartOnDemandAuditTask`,
`CreateBillingGroup`, `UpdateBillingGroup`, `GetRegistrationCode`,
`GetPackageConfiguration`, `ListSbomValidationResults`.

**Modelling gaps found, not fixed** (per `parity-principles.md`'s no-synthesis
rule -- reported, not guessed at):
- Audit *task execution* is not simulated at all: `StartOnDemandAuditTask`
  ignores its `TargetCheckNames` input entirely (bound to `_`), audit tasks
  never transition past `IN_PROGRESS` (only `CancelAuditTask` moves them, to
  `CANCELED`), and `DescribeAuditTaskOutput`'s real `auditDetails`
  (`map[string]types.AuditCheckDetails`, per-check pass/fail detail) and
  `taskStatistics` (`*types.TaskStatistics`, aggregate check counts) have no
  backing state to compute from -- there is no simulated check-execution
  result to roll up. This is the same class of gap as the already-documented
  `DescribeDomainConfiguration` TLS subsystem: an entire unmodeled subsystem,
  not a wire-shape omission fixable at the handler layer.
- `AuditFinding.RelatedResources` (`[]map[string]any`) has no production
  write path anywhere in this service -- grepped every `.go` file outside
  `_test.go`; the only assignment is the field's own clone helper. It is
  reachable only via test-only seeding, so `ListRelatedResourcesForAuditFinding`
  and `DescribeAuditFinding`'s `relatedResources` field can never be
  populated in any real-client-driven scenario. Converting the freeform
  `map[string]any` to a typed `RelatedResource{additionalInfo,
  resourceIdentifier, resourceType}` (mirroring the fix already applied to
  `AuditFinding.NonCompliantResource` in an earlier pass) would not close
  this gap, since nothing populates the field either way -- reported, not
  fixed.

**False positives ruled out**: `EventConfigEntry.Enabled` uses the wire key
`"Enabled"` (capitalized) -- looked like a casing bug at first glance, but
confirmed correct against `awsRestjson1_deserializeDocumentConfiguration`
(v1.77.4): real `types.Configuration.Enabled` genuinely serializes under
`"Enabled"`, not `"enabled"`. `GetPackageConfiguration` initially looked like
it was missing the real `versionUpdateByJobsConfig` wrapper key (the handler
returns the whole `*PackageConfiguration` struct with no visible wrapper in
the handler code) -- but `PackageConfiguration`'s own field is tagged
`json:"versionUpdateByJobsConfig,omitempty"`, so the wrapper is already
present; not a bug.

**Snapshot version**: bumped `iotSnapshotVersion` 2 -> 3. Reason: the
`ScheduledAudit.Tags` retag from `json:"tags,omitempty"` to `json:"-"` is a
persisted-struct json-tag change (`ScheduledAudit` round-trips via the
generic `store.Table[ScheduledAudit]` registry, which marshals the struct
directly per its own json tags -- `pkgs/store/table.go`'s `Snapshot`/`Restore`
use `json.Marshal`/`Unmarshal` on the same type). `pkgs/persistence`'s
`TestSnapshotVersionGuard` confirmed this needed a bump (failed without one:
"at least one existing field's name, type, or json tag changed or was
removed -- this is NOT the additive case"); bumped, then regenerated
`pkgs/persistence/testdata/snapshot_inventory.json` via `-update` and
re-ran the guard clean. Functionally the removed field was dead weight (its
real state lives in the separately-persisted `resourceTags` map and is never
read back from `ScheduledAudit.Tags`), but the guard is right that the
on-disk shape changed and can't tell the difference from a dangerous change
without the bump.

**Ops not reached this pass** (of the 102; the remaining 83): the rest of the
audit/device-defender family already covered by prior passes' `ops:`/
`families:` entries is done, but `CreateDynamicThingGroup`/
`UpdateDynamicThingGroup`, all of `handler_routing.go`'s grab-bag
(`DescribeEncryptionConfiguration`, `DescribeEventConfigurations`,
`DescribeManagedJobTemplate`, `DescribeThingRegistrationTask`,
`GetBehaviorModelTrainingSummaries`, `GetThingConnectivityData`,
`ListDomainConfigurations`, `ListManagedJobTemplates`, `ListMetricValues`,
`ListOutgoingCertificates`, `ListThingGroupsForThing`, `ListThingPrincipals`,
`ListThingPrincipalsV2`, `ListThingRegistrationTaskReports`,
`ListThingRegistrationTasks`, `RegisterThing`, `StartThingRegistrationTask`,
`TestAuthorization`, `TestInvokeAuthorizer`), `handler_policies.go`'s
(`GetEffectivePolicies`, `ListPolicyPrincipals`, `ListPrincipalPolicies`,
`ListPrincipalThings`, `ListPrincipalThingsV2`, `ListTargetsForPolicy`),
`handler_logging.go` (`GetLoggingOptions`, `GetV2LoggingOptions`,
`ListV2LoggingLevels`), `handler_indexing.go` (`GetIndexingConfiguration`),
and the 52 void-output ops listed in the 2026-08-23 (pass #6) entry above
remain unaudited by any pass. These are the next pass's ground.

Gates run: `go build ./...`, `go vet ./services/iot/...`, `gofmt -l`,
`go test -race ./services/iot/... ./pkgs/persistence/...`,
`golangci-lint run ./services/iot/...` -- all clean.

## 2026-08-23 (pass #7): all 31 has-body ops from the never-audited 83 + 8 void-op spot checks

**Re-derived the 83 figure independently** (diffed `op_names.go`'s 276 ops
against every "genuinely audited" mention in this file's `ops:`/`families:`
blocks, distinguishing that from the prior pass's own not-reached
enumeration) -- **confirmed exactly 83**, split 31 has-body / 52 void-output,
matching pass #6's own count name-for-name.

**Audited all 31 has-body ops.** Found and fixed 13 real bugs across 12 ops
(a ~39% hit rate on the has-body queue), field-diffed against the pinned
SDK's serializer/deserializer/type definitions:

- `DescribeEventConfigurations`/`UpdateEventConfigurations` (`audit.go`):
  `EventConfigurations` never modeled `creationDate`/`lastModifiedDate`
  (both real `DescribeEventConfigurationsOutput` members) -- a real client
  got neither back no matter how many times `UpdateEventConfigurations` ran.
  Fixed: `CreationDate` set once on first write, `LastModifiedDate` on every
  write. See `TestEventConfigurations_DatesSurface`.
- `ListDomainConfigurations` (`handler_provisioning.go`): the real
  `serviceType`/`marker`/`pageSize` query params (input) and `nextMarker`
  (output) were entirely ignored -- every domain configuration was always
  returned in one unfiltered page. Fixed. New shared
  `parseIoTMarkerPagination` helper (`handler.go`) factors out the
  marker/pageSize pattern, also applied to `ListOutgoingCertificates` below.
  See `TestListDomainConfigurations_ServiceTypeFilterAndPagination`.
- `ListThingGroupsForThing` (`handler_thing_groups.go`): real
  `ListThingGroupsForThingOutput.thingGroups` is
  `[]types.GroupNameAndArn{groupName,groupArn}` -- this op returned bare
  group-name strings, which a real client's deserializer (expects a JSON
  object) would reject outright. **Correct sibling**: `ListThingGroups`
  (plain, non-thing-scoped) already got this right from an earlier pass,
  with a comment explaining the exact shape -- `ListThingGroupsForThing` was
  never given the same fix despite sharing the identical `GroupNameAndArn`
  wire type. Also added the real `maxResults`/`nextToken` pagination,
  previously ignored. See
  `TestListThingGroupsForThing_WireShapeAndPagination`.
- `ListThingPrincipals` (`handler.go`): real `maxResults`/`nextToken`
  pagination entirely ignored. Fixed. See `TestListThingPrincipals_Pagination`.
- `ListThingPrincipalsV2`/`ListPrincipalThingsV2` (`handler.go`/
  `handler_policies.go`): the real `thingPrincipalType` query filter
  (`EXCLUSIVE_THING`/`NON_EXCLUSIVE_THING`) was ignored on both ops; separately
  `ListPrincipalThingsV2` didn't even call its own V2 backend method (it
  delegated to V1 `ListPrincipalThings` and only ever emitted a bare
  `thingName`, dropping `thingPrincipalType` from every entry, the whole
  reason a V2 variant of this op exists per real AWS). Both fixed. See
  `TestListThingPrincipalsV2_ThingPrincipalTypeFilter`,
  `TestListPrincipalThingsV2_WireShapeAndFilter`. **Follow-up gap, not
  fixed** (see below): `AttachThingPrincipal` drops its own real
  `thingPrincipalType` query param entirely, so every attachment is always
  stored as the default `NON_EXCLUSIVE_THING` regardless of what a real
  client requests -- these two filters are correct but can only ever
  observe the default value until that's fixed.
- `ListPrincipalPolicies`/`ListPolicyPrincipals`/`ListTargetsForPolicy`
  (`handler_policies.go`): the real `marker`/`pageSize` pagination
  (`nextMarker` on output) was ignored on all three. Fixed. See
  `TestPolicyPrincipalListing_Pagination`.
- `GetEffectivePolicies` (`handler_policies.go`): real
  `GetEffectivePoliciesInput.thingName` is a query parameter, not a body
  field (`principal`/`cognitoIdentityPoolId` are body fields) -- a real
  client's thing-scoped effective-policy resolution always saw an empty
  `thingName`. Fixed. See `TestGetEffectivePolicies_ThingNameIsQueryParam`.
- `ListV2LoggingLevels`/`SetV2LoggingOptions`/`GetV2LoggingOptions`
  (`handler_logging.go`/`logging.go`): two bugs. (1) `ListV2LoggingLevels`
  ignored the real `targetType` filter and `maxResults`/`nextToken`
  pagination entirely. (2) `SetV2LoggingOptionsInput`'s real
  `eventConfigurations` member
  (`[]types.LogEventConfiguration{eventType,logDestination,logLevel}`) was
  silently dropped, and `GetV2LoggingOptionsOutput` never modeled it at all.
  Both fixed (new `LogEventConfigurationV2` type); `SetV2LoggingOptions`'s
  exported `Backend` interface signature changed to add the parameter --
  `make build-check` run clean afterward. See
  `TestListV2LoggingLevels_TargetTypeFilterAndPagination`,
  `TestV2LoggingOptions_EventConfigurationsSurvive`.
- `GetIndexingConfiguration`/`UpdateIndexingConfiguration`
  (`handler_indexing.go`/`types.go`): `ThingIndexingConfiguration` never
  modeled the real `deviceDefenderIndexingMode` member -- silently dropped
  on Update, never surfaced on Get. Fixed (purely additive field; no
  snapshot bump). See `TestIndexing_DeviceDefenderIndexingModeSurvives`.
  **Modelling gap, not fixed**: the real type's `managedFields` (both
  Thing/ThingGroup indexing configs) and `filter.geoLocations` have no
  backing concept in this backend at all (no simulated "already known by
  Fleet Indexing" managed-field catalog) -- reported, not synthesized.
- `CreateDynamicThingGroup`/`UpdateDynamicThingGroup`
  (`handler_thing_groups.go`/`thing_groups.go`/`types.go`): the real request
  body nests description under `thingGroupProperties.thingGroupDescription`
  (mirroring the sibling static `CreateThingGroup`/`UpdateThingGroup`,
  already correct) -- both dynamic ops instead read a top-level
  `description` field that doesn't exist on the wire, so a real client's
  description was always silently dropped. Both also dropped `indexName`/
  `queryVersion` entirely (now modeled on `ThingGroup`, threaded through
  Create/Update/response). **Correct sibling**: static `UpdateThingGroup`
  already enforces `expectedVersion`'s optimistic-lock semantics via the
  shared `UpdateThingGroupInput` struct; `UpdateDynamicThingGroup` received
  the same struct but never checked the field -- fixed to match. See
  `TestDynamicThingGroup_RealWireShape`.
- `UpdateJob` (`handler_jobs.go`/`jobs.go`): real `UpdateJobInput` has six
  members beyond `description` -- `abortConfig`, `jobExecutionsRolloutConfig`,
  `timeoutConfig`, `jobExecutionsRetryConfig`, `presignedUrlConfig`, and
  `namespaceId` -- this op only ever applied `description`, silently
  dropping the other five already-modeled-on-`Job` config blocks (the sixth,
  `namespaceId`, has no backing field on `Job` at all -- reported as a
  modelling gap, not fixed, since `CreateJob` also never persists it). Fixed
  the five. Driven through a real generated SDK client
  (`TestUpdateJob_AdvancedFieldsSurvive`) since the bug is on the request
  side and a body-shape-only test can't prove it.
- `UpdateCACertificate` (`handler_certificates.go`/`certificates.go`):
  real `newStatus`/`newAutoRegistrationStatus` are query parameters, not
  body fields -- this op read `newStatus` from the body only, so a real
  client's status change was **always silently dropped**, the single
  highest-impact bug this pass found. `registrationConfig`/
  `removeAutoRegistration` (real body fields) were also entirely
  unimplemented. All fixed. See
  `TestUpdateCACertificate_QueryParamsAndBodyFields`; the pre-existing
  `TestCACertificate` also asserted an update that never took effect and
  is now a real round-trip assertion.
- `TestAuthorization`/`TestInvokeAuthorizer`,
  `ListThingRegistrationTasks`/`ListThingRegistrationTaskReports`,
  `RegisterThing`/`StartThingRegistrationTask`, `ListManagedJobTemplates`/
  `DescribeManagedJobTemplate`, `GetBehaviorModelTrainingSummaries`,
  `ListMetricValues`, `DescribeEncryptionConfiguration`,
  `DescribeThingRegistrationTask` -- field-diffed, found already correct.

**Spot-checked 8 void-output ops' request-decode structs** against their
real `<Op>Input` (the never-swept request-side class this campaign has
flagged repeatedly): `UpdateJob` (above, fixed), `UpdateCACertificate`
(above, fixed), `UpdateTopicRuleDestination` and `UpdateAuditSuppression`
(field-diffed, already correct), plus **found but not fixed** for time
(reported here for the next pass, each a confirmed accept-and-drop):
`UpdatePackage` drops the real `unsetDefaultVersion` bool entirely;
`UpdatePackageVersion` drops `action`/`artifact`/`attributes`/`recipe` (four
real members, only `description`/`status` are applied); `UpdateProvisioningTemplate`
drops `defaultVersionId`/`preProvisioningHook`/`removePreProvisioningHook`
(the `ProvisioningHook` type already exists from an earlier pass's
`CreateProvisioningTemplate` fix, so wiring these three is mechanical, not
a modelling gap).

**Modelling gaps found, not fixed** (no backing state to synthesize from,
per `parity-principles.md`'s no-synthesis rule):
- `GetThingConnectivityData`: real `GetThingConnectivityDataOutput` has nine
  members beyond `connected`/`timestamp`/`disconnectReason`
  (`cleanSession`, `clientId`, `keepAliveDuration`, `sessionExpiry`,
  `sourceIp`, `sourcePort`, `targetIp`, `targetPort`, `vpcEndpointId`) --
  this backend's connectivity model is already documented (`store.go`'s
  `SetThingConnectivityInternal`) as derived from live MQTT session events
  it doesn't yet simulate; none of the nine socket-level fields have
  anywhere to come from.
- `TestInvokeAuthorizer`: real `TestInvokeAuthorizerInput.httpContext`/
  `tlsContext` (beyond the already-modeled `mqttContext`/`token`/
  `tokenSignature`) have no backing use in `TestInvokeAuthorizer`'s
  evaluation logic, which never actually invokes a Lambda authorizer --
  it derives a deterministic result from stored authorizer config alone.
- `ListMetricValues`: real `ListMetricValuesInput.dimensionName`/
  `dimensionValueOperator` filter has no backing state -- `MetricDatapoint`
  doesn't track a dimension per datapoint at all (only
  `AddMetricValueInternal`-seeded test data exists).
- `TestAuthorization`: real `TestAuthorizationInput.clientId` is a query
  parameter (confirmed via `serializers.go`), currently read from the body
  instead -- **investigated and NOT fixed**: `ClientID` has zero
  consumers anywhere in `TestAuthorization`'s evaluation logic (grepped;
  no `${iot:ClientId}`-style policy-variable substitution exists in this
  backend at all), so the wire-location fix would have no observable effect
  and no way to satisfy this campaign's "real-SDK-client round-trip,
  confirmed to fail against unfixed code" proof requirement. Reported as
  unprovable rather than counted as a fix.

**False positive ruled out**: `ListDomainConfigurations`' response entries
include a `domainConfigurationStatus` key that real
`types.DomainConfigurationSummary` doesn't have (only
`domainConfigurationArn`/`domainConfigurationName`/`serviceType`) --
confirmed via the deserializer that extra keys are silently ignored by a
real client, not a wire-shape defect; left as-is (pre-existing, out of
scope for this fix).

**Sibling checks**: every fix above notes its family in-line;
`ListThingGroupsForThing`/`ListThingGroups` and static/dynamic
`UpdateThingGroup` are the two genuine "correct sibling exists" cases
(shape #8) found this pass.

**Snapshot version**: NOT bumped (stays 3). Every persisted-struct change
this pass (`EventConfigurations.CreationDate`/`LastModifiedDate`,
`MetricValueData.Numbers`/`Strings`, `ThingIndexingConfiguration.DeviceDefenderIndexingMode`,
`V2LoggingOptions.EventConfigurations`, `ThingGroup.IndexName`/`QueryVersion`)
is purely additive -- confirmed by `pkgs/persistence`'s
`TestSnapshotVersionGuard`, which reported each as "additive only, needs no
bump" rather than a bump case. `pkgs/persistence/testdata/snapshot_inventory.json`
regenerated via `-update` and merged by hand: a concurrent, uncommitted,
unrelated `services/ec2/` change (owned by another in-flight session, not
touched by this pass) was also live in the working tree when `-update` ran
and got pulled into the same regeneration pass; that single `ec2` line was
manually excluded from the diff before committing so this pass's golden-file
change is iot-only. `go test ./pkgs/persistence/` is clean for `iot`; it
still fails for the pre-existing, unrelated `ec2` reason, confirmed present
before this pass started (`git status` showed `services/ec2/*` already
modified) and out of this pass's scope per its own instructions.

**Hand-revert verified for every fix above**: each change was reverted via
`cp` from a scratch backup, its guarding test confirmed to fail against the
reverted code (quoted failure captured during the session), then restored
and `md5sum`-confirmed identical to the fixed version before moving on.

**Ops not reached this pass**: all 31 has-body ops from the 83 were
reached; the 52 void-output ops were spot-checked (8 of 52 request-decode
structs diffed, see above) but not exhaustively field-diffed. The full
52-op void list (unchanged from pass #6's enumeration, reproduced there)
remains the next pass's most direct ground, prioritizing the three
found-but-not-fixed `Update*` ops above and a full request-side sweep of
the rest.

Gates run: `go build ./...`, `go vet ./services/iot/...`, `gofmt -l
services/iot/`, `go test -race -count=1 ./services/iot/...`,
`go test ./pkgs/persistence/...` (iot clean, ec2 pre-existing/out-of-scope
failure), `golangci-lint run ./services/iot/...` (0 issues), `make
build-check` (exit 0, whole-repo, covers the `SetV2LoggingOptions`
exported-signature change) -- all clean. Work left uncommitted per this
pass's instructions.

## 2026-08-23 (pass #8): the four `Update*`/`AttachThingPrincipal` bugs pass #7 found but ran out of time to fix

Fixed all four, re-verified against `iot@v1.77.4` before touching code (all
four confirmed real; none were false positives or query/URI-bound
mislabeled as body):

- **`AttachThingPrincipal`** (`handler.go`): real `thingPrincipalType` is a
  **query parameter**
  (`awsRestjson1_serializeOpHttpBindingsAttachThingPrincipalInput`,
  `serializers.go`), not a body field -- confirmed via the serializer, not
  guessed from the Input struct. The handler never read it at all, so
  every attachment was silently forced to the default `NON_EXCLUSIVE_THING`
  regardless of what a real client requested. This was also a genuine
  storage gap, not just a handler oversight: `thingPrincipals` was
  `map[string][]string` with nowhere to record a per-principal type at all,
  so `ListThingPrincipalsV2` hardcoded `defaultThingPrincipalType` for
  every entry and `ListPrincipalThingsV2` didn't even have its own backend
  method (it delegated to V1 `ListPrincipalThings` and hardcoded the same
  default). Fixed: added `thingPrincipalTypes map[string]map[string]string`
  (thingName -> principal -> type) to `InMemoryBackend`, wired through
  `AttachThingPrincipal`/`DetachThingPrincipal`/`ListThingPrincipalsV2`, and
  gave `ListPrincipalThingsV2` its own real backend method (new
  `PrincipalThingObject` type, new `Backend.ListPrincipalThingsV2` method)
  instead of borrowing V1's. Persisted in `backendSnapshot` as a new
  `thingPrincipalTypes` field (purely additive, no version bump -- see
  below). See `TestAttachThingPrincipal_ThingPrincipalTypeSurvives_SDKRoundTrip`
  (new, real generated SDK v2 client round-trip through both
  `ListThingPrincipalsV2` and `ListPrincipalThingsV2`).
  **Unblocked**: the two V2 list filter tests pass #7 landed
  (`TestListThingPrincipalsV2_ThingPrincipalTypeFilter`,
  `TestListPrincipalThingsV2_WireShapeAndFilter`) could previously only ever
  observe the default type, so their EXCLUSIVE_THING-filter assertions were
  unfalsifiable by construction. Both rewritten to attach a real
  `EXCLUSIVE_THING` principal alongside a default one and assert the filter
  separates them by actual recorded type, not a hardcoded constant.
- **`UpdatePackage`** (`handler_packages.go`/`packages.go`): real
  `unsetDefaultVersion` bool
  (`awsRestjson1_serializeOpDocumentUpdatePackageInput`, `serializers.go`)
  is a body field, confirmed dropped -- entirely unread by the handler, so
  a default version could be set but never cleared. Fixed:
  `UpdateIoTPackage` takes a new `unsetDefaultVersion bool` parameter;
  `unsetDefaultVersion=true` clears `DefaultVersionName`, otherwise a
  non-empty `defaultVersionName` still sets it (mutually exclusive per AWS
  docs). See `TestUpdatePackage_UnsetDefaultVersionSurvives` (SDK
  round-trip: set, confirm, unset, confirm cleared).
- **`UpdatePackageVersion`** (`handler_packages.go`/`packages.go`): real
  `action`/`artifact`/`attributes`/`recipe` (four body fields,
  `awsRestjson1_serializeOpDocumentUpdatePackageVersionInput`) confirmed
  dropped -- only `description`/`status` were ever applied. `action`
  (`PUBLISH`/`DEPRECATE`) is a lifecycle-transition shorthand for `status`,
  not a field of its own (`GetPackageVersionOutput` has no `action`
  member) -- mapped `PUBLISH`->`PUBLISHED`, `DEPRECATE`->`DEPRECATED`,
  applied only when `status` isn't explicitly given. New
  `UpdateIoTPackageVersionOptions` struct (mirrors the existing
  `CreateIoTPackageVersionOptions` pattern) bundles the four. See
  `TestUpdatePackageVersion_AdvancedFieldsSurvive` (SDK round-trip
  asserting all four survive through `GetPackageVersion`).
- **`UpdateProvisioningTemplate`** (`handler_provisioning.go`/
  `provisioning.go`): real `defaultVersionId`/`preProvisioningHook`/
  `removePreProvisioningHook` (three body fields,
  `awsRestjson1_serializeOpDocumentUpdateProvisioningTemplateInput`)
  confirmed dropped. Mechanical, as pass #7 flagged: `ProvisioningTemplate`
  already modeled `DefaultVersionID`/`PreProvisioningHook` (from
  `CreateProvisioningTemplate`'s existing fields), just never wired on
  Update. Fixed: `UpdateProvisioningTemplate` takes three new parameters;
  `removePreProvisioningHook=true` clears the hook, else a non-nil
  `preProvisioningHook` replaces it. See
  `TestUpdateProvisioningTemplate_AdvancedFieldsSurvive` (SDK round-trip:
  set `defaultVersionId`/hook, confirm via `DescribeProvisioningTemplate`,
  then remove the hook and confirm it clears).

**Hand-revert verified for every fix above**: each change was reverted via
`cp` from a scratch backup to the exact pre-fix line, its guarding test run
and confirmed to fail against the reverted code (failure output captured
during the session, see report), then restored and `md5sum`-confirmed
byte-identical to the fixed version before moving on.

**Snapshot version**: NOT bumped (stays 3). The only persisted-struct
change this pass, `backendSnapshot.ThingPrincipalTypes
map[string]map[string]string`, is a brand-new field with no prior on-disk
key to collide with -- purely additive. `pkgs/persistence`'s
`TestSnapshotVersionGuard` confirmed this directly ("bookkeeping, not a
version-bump case... diff is additive only and needs no bump") before the
golden file was regenerated with `-update`.
`pkgs/persistence/testdata/snapshot_inventory.json` diff is a single added
line (`backendSnapshot.ThingPrincipalTypes ...`); `git status` was checked
before and after -- `services/ec2/` was dirty (another in-flight session,
per this pass's own instructions) both times and the regenerated golden
diff contains no `ec2` entries, so nothing needed manual exclusion this
time (contrast pass #7, which did need to hand-exclude a concurrent `ec2`
line).

**Not touched, still open**: the 52 void-output ops' request-decode
structs beyond the 8 pass #7 spot-checked, and the three modelling gaps
pass #7 investigated and left (`GetThingConnectivityData`,
`TestInvokeAuthorizer`'s `httpContext`/`tlsContext`, `ListMetricValues`'
`dimensionName`) -- re-confirmed genuine gaps (no backing state anywhere
in this backend to synthesize from), not touched, per this pass's own
scope.

Gates run: `gofmt -l services/iot/` (clean), `go vet ./services/iot/...`
(clean), `go build ./...` (clean), `go test -race -count=1
./services/iot/...` (clean), `go test ./pkgs/persistence/...` (clean, iot
and ec2 both -- no pre-existing ec2 failure was hit this pass),
`golangci-lint run ./services/iot/...` (0 issues after one `--fix` pass for
two `golines` line-length and one `fieldalignment` finding introduced by
this pass's own edits, plus one `testifylint` JSONEq finding in a new
test), `go build ./...` + `go vet -tags e2e ./...` + `go vet -tags
integration ./...` (`make build-check`'s three steps, run individually;
exit 0 -- covers this pass's exported-signature changes to
`UpdateIoTPackage`, `UpdateIoTPackageVersion`, `UpdateProvisioningTemplate`,
and the new `Backend.ListPrincipalThingsV2`). Work left uncommitted per
this pass's instructions.

## 2026-08-29 enum-VALUE sweep (wrapper-key-sweep campaign, wire-shape enforcement all services) -- no fix found

Targeted pattern hunt for the comprehend class of bug: a status/state value assigned to a
domain struct field that is not a member of the real AWS enum for the corresponding response
member, reaching the wire through the field rather than a same-site literal `cmd/enumcheck` can
resolve. Checked every domain struct field holding a status/state/type/mode concept against its
real SDK enum (`iot@v1.77.4 types/enums.go`): `CertificateStatus`, `TopicRuleDestinationStatus`,
`ConfigurationStatus`, `DomainConfigurationStatus`, `AuthorizerStatus`, `PackageVersionStatus`,
`IndexStatus`, `OTAUpdateStatus`, `AuditTaskStatus`, `AuditMitigationActionsTaskStatus`,
`DetectMitigationActionsTaskStatus`, `SbomValidationResult`, `SbomValidationStatus`,
`VerificationState`. `cmd/enumcheck` was run and, consistent with the rest of this campaign,
would not have caught anything even if a bug existed (it can't see struct-field assignment) —
moot here since none was found.

Specifically checked for the comprehend shape (one shared vocabulary reused across several
enums that don't actually share values): `jobs.go`'s local `JobStatus`/`JobExecutionStatus`
mirror types (`IN_PROGRESS`/`CANCELED`) are reused verbatim for `AuditTaskStatus`/
`AuditMitigationActionsTaskStatus`/`DetectMitigationActionsTaskStatus` fields across
`audit.go`/`device_defender.go` — genuinely risky-looking, but every string gopherstack actually
assigns from that shared vocabulary (`"IN_PROGRESS"`, `"CANCELED"`) happens to be a legal member
of all three real target enums, so no wrong value currently escapes; this is a near-miss worth
flagging for future vigilance, not a live bug. Likewise `packages.go` assigns
`SbomValidationResult`'s `"SUCCEEDED"`/`"FAILED"` values onto a field typed for the sibling
`SbomValidationStatus` enum — both target values are members of both enums, so also not live.

One DORMANT finding, not fixed (unreachable, so not fabricating a path to it per this campaign's
rule): `jobs.go`'s local `JobStatus` mirror type declares `JobStatusFailed = "FAILED"`, which is
NOT a member of the real `types.JobStatus` (IN_PROGRESS/CANCELED/COMPLETED/DELETION_IN_PROGRESS/
SCHEDULED -- no FAILED at the aggregate-Job level in the real API, only per-execution). The
constant is never assigned anywhere in the backend (`Job.Status` only ever reaches
`JobStatusInProgress`/`JobStatusCanceled` via `jobs.go:352`/`578`) -- confirmed by grep across the
whole service. Real Jobs in this backend also never reach `COMPLETED`/`DELETION_IN_PROGRESS`/
`SCHEDULED` at all, a completeness gap (missing lifecycle transitions), not a wrong-value bug --
named here, not fixed, out of this pass's scope.

Everything else checked used values that were both legal for their real enum and client-input
passthrough where the field is a request parameter rather than a backend-computed value
(`CertificateStatus`/`DomainConfigurationStatus`/`PackageVersionStatus`/`VerificationState`
transitions all originate from the caller's own typed SDK field, which cannot carry an illegal
member in the first place).

No code changes this pass. Gates: `go build ./services/iot/...` (clean), `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/iot/...` (pass, no new tests --
nothing to prove).

## 2026-08-31 error-envelope-shape / fabricated-error-code sweep

**Scope**: this campaign's two remaining classes -- error envelope shape (does an
error deserialize into the typed exception a real SDK client branches on) and
fabricated error codes (a code the emulator returns that the pinned SDK does not
define for that specific operation). Not the filter/value-semantics class other
recent passes chased.

**Protocol/envelope mechanism confirmed correct at the generic level**: this
service's `awsErrBody{Type string \`json:"__type"\`, Message string \`json:"message"\`}`
(handler_helpers.go) is read correctly by every operation's real
`awsRestjson1_deserializeOpError<Op>` function (`iot@v1.77.4/deserializers.go`) via
the shared `restjson.GetErrorInfo` helper (`aws-sdk-go-v2@v1.43.4/aws/protocol/restjson/decoder_util.go`),
which checks header `X-Amzn-ErrorType` first, then body `code`, then body `__type`
-- this service sets no header but does set `__type`, so the body fallback always
resolves. Also confirmed for the `iotdataplane` SDK (Get/Update/DeleteThingShadow,
ListNamedShadowsForThing) via the same generic pattern.

**IMPORTANT DISCOVERY: `handler_shadows.go`/`shadows.go` (Device Shadow ops) are
unreachable by any correctly-signed real client.** `Handler.RouteMatcher()`
(handler.go) explicitly gates shadow paths (`isThingShadowPath`) by SigV4 signing
service, matching only `svc == "" || svc == iotServiceName` -- a genuinely
iotdataplane-signed request (`svc == "iotdata"`) is deliberately NOT claimed here,
per the existing comment citing gopherstack-61i8, so a real `iotdataplane` client's
shadow calls route to the separate `services/iotdataplane` package instead (out of
this pass's scope; confirmed to exist via `cmd/errcodeaudit`'s
`services/iotdataplane/handler.go:411 ResourceAlreadyExistsException` finding, not
investigated further). Verified empirically: a real `aws-sdk-go-v2/service/iotdataplane`
client's `UpdateThingShadow` against this package's handler 404's at the Echo
routing layer (RouteMatcher rejects it, falls through to default 404) before ever
reaching `shadows.go`. A found wire-shape bug there (UpdateThingShadow's
unknown-thing path wrongly returns ResourceNotFoundException; real op declares no
such case) was NOT fixed because it cannot affect any real client -- fixing dead
code would not move the needle this campaign cares about. This should be recorded
as a standing caveat for any future pass over this file.

**8 real bugs found and fixed, one shape, one family**: the entire TopicRule/
TopicRuleDestination op family (GetTopicRule, DeleteTopicRule, EnableTopicRule,
DisableTopicRule, ReplaceTopicRule, GetTopicRuleDestination,
UpdateTopicRuleDestination, DeleteTopicRuleDestination) uses a genuinely different,
smaller exception vocabulary than the rest of this service -- confirmed by direct
per-op read of each operation's own `deserializeOpError<Op>` switch:
`{InternalException, InvalidRequestException, ServiceUnavailableException,
UnauthorizedException}` plus `ConflictingResourceUpdateException`/
`SqlParseException` where applicable. None of the 8 declare
`ResourceNotFoundException` at all, unlike almost every other Get/Delete op in this
service. `writeIoTError`'s shared not-found case previously rendered
`ErrRuleNotFound`/`ErrTopicRuleDestinationNotFound` as `ResourceNotFoundException`
-- a code none of these 8 operations' real deserializer switches match, so a real
client got a `*smithy.GenericAPIError` instead of any typed exception (silent
failure mode). Fixed by moving both sentinels into `writeIoTError`'s
`InvalidRequestException` case (the only client-fault type this family declares).
Two existing tests asserted the old, wrong behavior as correct and were corrected,
not weakened: `TestRuleNotFound_Returns404` (renamed `_Returns400`,
`handler_test.go`) and `TestErrorFormat_UsesAWSFormat`'s `RuleNotFound` case
(`errors_test.go`) both asserted 404/ResourceNotFoundException; now assert
400/InvalidRequestException. `TestDeleteTopicRule_Handler`'s `delete_missing_rule`
case (`handler_topic_rules_test.go`) had the same fix. Zero assertions dropped in
any of the three -- only expected values changed.

**14 more real bugs, same shape, spread across families that share the generic
`ErrResourceNotFound`/`ErrThingGroupNotFound`/`ErrDeleteConflict`/
`ErrInvalidStateTransition` sentinels with other operations that DO need the
richer type**: DeleteAuditSuppression, DeleteMitigationAction, DeleteBillingGroup,
PutVerificationStateOnViolation, DeleteV2LoggingLevel, DeleteFleetMetric,
DeleteCustomMetric, DeleteDimension, DeleteSecurityProfile, DeleteThingGroup,
DeleteDynamicThingGroup, ListThingRegistrationTaskReports (all: not-found ->
InvalidRequestException, not ResourceNotFoundException, per their own real
deserializer switches), plus CancelJob (InvalidStateTransitionException not
declared; InvalidRequestException is) and DeleteThing (DeleteConflictException not
declared for the "has attached principals" case; InvalidRequestException is --
DeleteThing's genuine not-found case via `ErrThingNotFound` IS correctly declared
and was left alone). Because these sentinels are shared with other operations that
correctly need `ResourceNotFoundException`/etc, the fix is a new per-call-site
override (`respondAsInvalidRequest(c, err, sentinel)`, handler_helpers.go) rather
than a change to the sentinels' own semantics or `writeIoTError`'s global mapping
-- preserves every other caller and every existing backend-level test asserting the
sentinel itself. One existing test asserted the old wrong behavior:
`TestCancelJob_DescriptionAndTerminalStateGuard` (`handler_jobs_test.go`) expected
409/InvalidStateTransitionException; now asserts 400/InvalidRequestException. Zero
assertions dropped.

Every fix above was proven fail-before/pass-after with a real `aws-sdk-go-v2`
client (`errors.As` on the specific typed exception, not a status code): the
TopicRule family in `wire_error_code_topic_rule_test.go` (8 subtests), the 12
shared-sentinel operations plus CancelJob/DeleteThing in
`wire_error_code_delete_not_found_test.go` (14 subtests total). For the 12-op batch
the fail-before proof was done as a batch via `git apply -R` on the handler diff
(all 14 new subtests confirmed failing against the reverted code, then confirmed
passing after `git apply` re-applied it) rather than one revert per operation --
recorded here since it is a coarser proof than the per-operation reverts used
elsewhere in this pass, though it exercises the same code paths.

**9 more confirmed real bugs, found but NOT fixed this pass -- different families,
need new wire infrastructure**: CreateCommand/DeleteCommand/DeleteCommandExecution
(Commands API) and CreateIoTPackage/CreateIoTPackageVersion/DeleteIoTPackage/
DeleteIoTPackageVersion (Software Package Catalog, real ops CreatePackage/
CreatePackageVersion/DeletePackage/DeletePackageVersion) both use AWS's newer
common vocabulary (`ConflictException`/`ValidationException`/
`InternalServerException`) instead of this service's classic
`InvalidRequestException`/`ResourceAlreadyExistsException`/`InternalFailureException`
-- confirmed by direct per-op read, e.g. `CreatePackage`'s real set is
`{ConflictException, InternalServerException, ServiceQuotaExceededException,
ThrottlingException, ValidationException}`, no `ResourceAlreadyExistsException` at
all. `ErrAlreadyExists`/`ErrResourceNotFound` render as the wrong family's codes
for these 7 ops. Also: CreateJobTemplate (`ErrAlreadyExists` -> needs
`ConflictException`, not declared as `ResourceAlreadyExistsException`) and the
AlreadyExists half of StartAuditMitigationActionsTask/
StartDetectMitigationActionsTask (need `TaskAlreadyExistsException`, a type this
service's `writeIoTError` has never rendered at all; their not-found halves were
already correctly declared and untouched). Deferred because fixing any of these
requires adding genuinely new wire-error-code paths (`ConflictException`,
`ValidationException` as distinct from `InvalidRequestException`,
`TaskAlreadyExistsException`) to `writeIoTError`, not just redirecting an existing
sentinel to an existing code -- more invasive than this pass's remaining time
allowed to do with the same fail-before/pass-after rigor as the fixes above.
Recorded here with full reasoning rather than silently dropped.

**Fabricated error codes**: `cmd/errcodeaudit` returned zero findings (confident or
needs-review) for `services/iot/` directly (only `services/iotdataplane/handler.go:411`,
out of scope). No further literal-code fabrications found by manual per-op
cross-reference beyond the shape above (which is a *wrong-code-for-this-operation*
class, not an *undefined-anywhere-in-the-SDK* class).

**PARITY.md correction (typo, not substantive)**: the 2026-07-25 note above citing
`serializers.go`'s `awsAwsjson11_serializeOpListAuditFindings` names the wrong
protocol prefix -- the real symbol is `awsRestjson1_serializeOpListAuditFindings`
(confirmed directly; this service has no awsjson1.1 operations at all). The
route/field fix that note documents is unaffected; only the protocol-prefix string
in the note was wrong.

Gates: `go build ./services/iot/...` (clean), `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/iot/...` (pass), `golangci-lint run
./services/iot/...` (0 issues).

## 2026-08-31 errtargetaudit re-sweep: 6 of the 9 recorded-deferred fabricated codes fixed

`errtargetaudit -dir iot` (post-reachability-fix, post-sentinel-collision-fix)
reports 11 class-A findings, all falling inside the prior pass's "9 more
confirmed real bugs, found but NOT fixed" list above (Commands API +
Software Package Catalog + CreateJobTemplate + Start*MitigationActionsTask).
Re-verified every one directly against `iot@v1.77.4/deserializers.go`'s
per-op `awsRestjson1_deserializeOpError<Op>` switch rather than trusting the
prior pass's grouping.

**6 fixed, real ConflictException/TaskAlreadyExistsException gaps**:
`CreateCommand` (`commands.go:52`), `CreatePackage` (`packages.go:63`),
`CreatePackageVersion` (`packages.go:200`), `CreateJobTemplate`
(`jobs.go:856`) all declare `ConflictException` for the AlreadyExists case
(confirmed: `CreatePackage`/`CreatePackageVersion`'s full declared set is
`{ConflictException, InternalServerException, ServiceQuotaExceededException,
ThrottlingException, ValidationException}`, `CreateJobTemplate`'s is
`{ConflictException, InternalFailureException, InvalidRequestException,
LimitExceededException, ResourceNotFoundException, ThrottlingException}`) --
not `ResourceAlreadyExistsException`, the code `writeIoTError`'s shared
`ErrAlreadyExists` case renders by default (correct for ~150 other Create
ops in this service). `StartAuditMitigationActionsTask`
(`device_defender.go:158`) and `StartDetectMitigationActionsTask`
(`device_defender.go:477`) both declare `TaskAlreadyExistsException`
instead (their not-found halves already correctly declare/render
`ResourceNotFoundException` and are untouched). New per-call-site override
`respondAsConflictCode(c, err, sentinel, code)` (`handler_helpers.go`, same
pattern as the existing `respondAsInvalidRequest`) added rather than
changing `writeIoTError`'s shared `ErrAlreadyExists` mapping, since that
mapping is correct for every other caller. Proven fail-before/pass-after
with a real `aws-sdk-go-v2` client (`errors.As` on the specific typed
exception): `wire_error_code_already_exists_test.go`, 6 subtests, all
confirmed failing (asserting `*types.GenericAPIError`
`ResourceAlreadyExistsException` in the chain) against the pre-fix call
sites, passing after.

**CORRECTION to the prior pass's grouping**: the remaining 4 findings
(`DeleteCommand` `commands.go:111`, `DeleteCommandExecution`
`commands.go:254`, `DeletePackage` `packages.go:121`,
`DeletePackageVersion` `packages.go:316`/`319`) were filed alongside the 6
above as "needs new wire infrastructure" -- re-verified and that framing is
wrong for these four specifically. `DeleteCommand`/`DeleteCommandExecution`'s
full declared set is `{ConflictException, InternalServerException,
ThrottlingException, ValidationException}`; `DeletePackage`/
`DeletePackageVersion`'s is `{InternalServerException, ThrottlingException,
ValidationException}`. None of the four declare *any* not-found-capable
type, and no new infrastructure would help -- `ConflictException`/
`ValidationException` don't fit "resource does not exist" semantically, and
neither operation's own doc comment describes idempotent-delete behavior
for an unknown ID (unlike some AWS delete ops). Reclassified as the same
"operation's own model declares no type for this condition" refusal as
the TopicRule family above, not an infrastructure gap. Left unchanged
(still renders `ResourceAlreadyExistsException`-family's sibling
`ResourceNotFoundException`, itself equally undeclared -- no available code
is more correct).

**Fixed by deletion**: `DeleteCommandExecution`'s `executionID == ""`
pre-check (`commands.go:233-235`, now removed) returned `ErrValidation` ->
`InvalidRequestException`, a code this operation also does not declare
(`InvalidRequestException` is absent from its 4-member set above). The real
SDK client's `validateOpDeleteCommandExecutionInput` only rejects a nil
`*string`, not an empty string, so an empty-but-present `executionId`
reached this check -- same empty-but-present-identifier shape as the prior
pass's 8 deletions. Unlike those 8, the natural not-found fallback this
check short-circuited is *also* undeclared (see correction above), so this
deletion does not fully close the class-A gap -- it consolidates two
distinct wrong emissions (`InvalidRequestException` for the empty case,
`ResourceNotFoundException` for the not-found case) into one, removing the
invented validation check rather than leaving it beside an equally-wrong
neighbor. Regression: `TestDeleteCommandExecution_EmptyExecutionID`
(`handler_commands_test.go`), confirms `ErrResourceNotFound` fires and
`ErrValidation` does not.

**Confirmed unchanged from the prior pass**: the "25 operations fixed" /
"9 recorded refusals" history above still holds; this sweep only refined
the refusal reasoning for 4 of those 9-10 items, it did not reopen any of
the 25 fixed ones or the topic-rule/gopherstack-oc9v family's separate
refusals.

Gates: `go build ./services/iot/...` (clean), `go vet ./services/iot/...
./services/workmail/...` (clean; a concurrent, out-of-scope edit to
`services/codeconnections/handler_hosts.go` by another agent broke
repo-wide `go vet ./...` at the time of this pass -- confirmed via `git
status`/`git diff --stat` to be someone else's in-progress change, not
caused by or related to this pass), `go test -race -count=1
./services/iot/...` (pass), `golangci-lint run ./services/iot/...` (0
issues; one `unparam` finding on the first-draft `respondAsCode(..., status
int)` was fixed by dropping the always-409 `status` parameter and renaming
to `respondAsConflictCode`, not suppressed).

- **2026-09-06 pass (gopherstack-1ycq, gopherstack-6pt8)**: resolved both
  ghost-row bugs the 4c0r/6kyn sweep found but left filed for a future pass
  (see the `DeleteThing` entry above).

  **gopherstack-1ycq (resourceTags leak on delete)**: enumerated every
  `putResourceTagsLocked` call site (the only way a resource's tags enter
  `b.resourceTags`) against every `Delete*` path. 26 taggable resource
  types exist; before this pass only `DeleteThing` and `DeletePolicy`
  cleared their entry. The other 24 leaked -- the prior pass's "22" was an
  estimate, not a count; the real number, enumerated here, is 24:
  `DeleteScheduledAudit`, `DeleteMitigationAction`,
  `DeleteCertificateProvider`, `DeleteCACertificate`, `DeleteAuthorizer`,
  `DeleteCommand`, `DeleteIoTPackage`, `DeleteIoTPackageVersion`,
  `DeleteJob`, `DeleteJobTemplate`, `DeleteBillingGroup`,
  `DeleteTopicRule`, `DeleteThingType`, `DeleteFleetMetric`,
  `DeleteCustomMetric`, `DeleteDimension`, `DeleteOTAUpdate`,
  `DeleteRoleAlias`, `DeleteDomainConfiguration`,
  `DeleteProvisioningTemplate`, `DeleteStream`, `DeleteSecurityProfile`,
  `DeleteThingGroup`, `DeleteDynamicThingGroup`.

  For each, checked whether the leak is *observable*: is the resource's ARN
  deterministic from its user-chosen name/id (so delete + recreate under
  the same name lands on the same ARN and `ListTagsForResource` returns the
  stale tags), or does the ARN embed a value regenerated fresh on every
  create regardless of user input (making the leaked map entry permanently
  unreachable -- the same "unobservable, declined" shape an earlier ec2
  pass in this campaign correctly left alone)? 23 of the 24 build their ARN
  from `fmt.Sprintf(".../%s", <user-chosen name or id>)` -- deterministic,
  observable, fixed. The 24th, `DeleteCACertificate`, is the exception:
  `RegisterCACertificate` mints `id := uuid.NewString()[:12]` fresh on
  every call regardless of PEM content, so no re-register can ever land on
  a deleted cert's old ARN again. Left unfixed with a landmine comment at
  the call site explaining why -- there is no reachable path to prove a
  fix with a real observable-leak test, and no such test was written.

  Regression tests: `tags_delete_cleanup_test.go` adds
  `TestDeleteResource_ClearsResourceTagsOnRecreate` and
  `TestDeleteResource_LeavesOtherResourceTagsIntact`, both table-driven
  over the 23 fixed resource types via a shared `tagCleanupCase` (a
  `create`/`delete` closure pair per type) -- each type gets its own
  subtest, so a regression in one type's cleanup fails only that subtest,
  not a shared assertion. The first proves delete -> recreate under the
  same key yields no stale tags (and that the ARN really is stable,
  guarding against a future accidental ARN change silently making the test
  meaningless); the second proves deleting one resource doesn't touch a
  surviving resource of the same type. All 23 fixes were verified to fail
  their own subtest, and only their own subtest, when neutered individually
  by exact line content (confirmed compiling after the neuter, confirmed
  restored byte-for-byte after) -- see commit for the full neuter log.

  **gopherstack-6pt8 (thingThingGroups/thingBillingGroups reverse-index
  staleness)**: `thingGroupMembers` (group -> members) and `thingThingGroups`
  (thing -> groups) are meant to be kept symmetric; `UpdateThingGroupsForThing`
  already did this correctly via a shared `removeThingFromGroupIndexes`
  helper. `DeleteThingGroup`/`DeleteDynamicThingGroup` and
  `RemoveThingFromThingGroup` did not use it and only updated
  `thingGroupMembers`, leaving `thingThingGroups` stale. This is observable
  through `SearchIndex`'s `AWS_Things` index (`ThingGroupNames` field and
  `thingGroupNames:` query key), which reads `thingThingGroups` directly
  (`searchThingsIndex`/`matchedThings`, indexing.go) -- NOT through
  `ListThingGroupsForThing`, which was already correct (it derives from
  `thingGroupMembers`, not `thingThingGroups`).

  Fix semantics: `RemoveThingFromThingGroup` removes one thing from one
  group -- the group survives, so only that thing's `thingThingGroups`
  entry loses that one group name; now implemented as a direct call to
  `removeThingFromGroupIndexes`. `DeleteThingGroup`/`DeleteDynamicThingGroup`
  remove the whole group -- every former member's `thingThingGroups` entry
  loses that group name (the group's own `thingGroupMembers` entry is
  deleted outright either way, so calling the helper once per former member
  before the group-level delete is sufficient and correct).
  `DeleteBillingGroup` has the same shape one level over: it left
  `thingBillingGroups[thingName]` pointing at the deleted group for every
  member, so `DescribeThing.BillingGroupName` kept naming a group that no
  longer existed; fixed by clearing every `thingBillingGroups` entry equal
  to the deleted group's name (matching the shape `RemoveThingFromBillingGroup`
  already used for a single thing).

  Regression tests: `thing_group_reverse_index_test.go` --
  `TestRemoveThingFromThingGroup_UpdatesReverseIndex` (+ negative
  `_LeavesOtherGroupsIntact`), `TestDeleteThingGroup_ClearsReverseIndexForSurvivingMembers`
  (one surviving member keeps its other group, one loses its only group),
  `TestDeleteDynamicThingGroup_ClearsReverseIndexForSurvivingMembers`, and
  `TestDeleteBillingGroup_ClearsThingBillingGroupsForSurvivingMembers` (+
  negative, a second thing's own billing group is untouched). All assert
  through `SearchIndex`/`DescribeThing`, not internal maps. Each of the 4
  reverse-index cleanup sites was neutered individually (by exact line
  content, or -- for `RemoveThingFromThingGroup`, whose single-line fix
  isn't safely reducible to a no-op without an unused-variable compile
  error -- by reverting to the exact pre-fix forward-index-only body) and
  confirmed to fail only its own test(s), then restored byte-for-byte.

  Not reached: no other iot ghost-row classes were searched for beyond
  what 1ycq/6pt8 already named; this pass did not re-run a fresh
  map-vs-delete-path walk of the rest of the service.

  Gates: baseline (pre-edit) `golangci-lint run ./services/iot/...` was
  already `0 issues`; baseline `go test -race -count=1 ./services/iot/...`
  was already passing (1.172s). Post-fix: `go build ./services/iot/...`
  clean, `go test -race -count=1 ./services/iot/...` pass, `golangci-lint
  run ./services/iot/...` `0 issues`. No persisted snapshot DTO changed
  (no new struct fields; only added `delete(...)` calls and one reverse-index
  loop), so `TestSnapshotVersionGuard` was not affected and not run.

## 2026-09-07 errtargetaudit re-sweep (gopherstack-yr88): same 10 findings, 0 fixed

`errtargetaudit` reports the identical 10 class-A findings the 2026-08-31
sweep already resolved down to (immediately above): 6 `ResourceAlreadyExistsException`
false positives (`CreateCommand`/`CreateJobTemplate`/`CreatePackage`/
`CreatePackageVersion`/`StartAuditMitigationActionsTask`/
`StartDetectMitigationActionsTask`) and the same 4 `DeleteCommand`/
`DeleteCommandExecution`/`DeletePackage`/`DeletePackageVersion`
`ResourceNotFoundException` findings the prior pass explicitly left open.
Re-verified everything from scratch rather than trusting the prior write-up.

**Confirmed false positive, all 6, not touched**: the tool's trace stops at
the sentinel-creation call site inside the *backend* method (e.g.
`commands.go:52`'s `return nil, fmt.Errorf(..., ErrAlreadyExists)`), one hop
short of the *handler*, which already renders the correct declared code via
a per-call-site override added by the 2026-08-31 pass:
`respondAsConflictCode(c, err, ErrAlreadyExists, "ConflictException")`
(`handler_commands.go:83`, `handler_jobs.go:430`, `handler_packages.go:153`,
`handler_packages.go:252`) and `respondAsConflictCode(c, err, ErrAlreadyExists,
"TaskAlreadyExistsException")` (`handler_devicedefender.go:159`,
`handler_devicedefender.go:281`). Re-extracted each op's declared set
directly from `iot@v1.77.4/deserializers.go` and confirmed every override
matches: `CreateCommand`/`CreateJobTemplate`/`CreatePackage`/
`CreatePackageVersion` all declare `ConflictException`;
`StartAuditMitigationActionsTask`/`StartDetectMitigationActionsTask` both
declare `TaskAlreadyExistsException`. This is a variant of the tool's known
"one hop of callees" blind spot not yet enumerated in its false-positive
taxonomy: the miscoded value isn't inferred from a guard the tool can't
see (classes 2-5) or a doc/model disagreement (class 6) -- it's inferred
correctly at the sentinel, then silently overridden a second hop away, at
the handler call site, which the tool's single-hop trace never visits.

**Confirmed real, all 4, deliberately left unfixed -- same conclusion as
2026-08-31, re-derived independently**: re-extracted the full declared set
per op directly from `iot@v1.77.4/deserializers.go` (`DeleteCommand`/
`DeleteCommandExecution`: `{ConflictException, InternalServerException,
ThrottlingException, ValidationException, UnknownError}`; `DeletePackage`/
`DeletePackageVersion`: `{InternalServerException, ThrottlingException,
ValidationException, UnknownError}`) and independently cross-checked
against the live public API reference
(`docs.aws.amazon.com/iot/latest/apireference/API_DeleteCommand.html`,
`.../API_DeletePackage.html`) -- both list the same set with no
`ResourceNotFoundException` and no sentence describing idempotent-delete
behavior for an unknown id. This adds no evidence beyond what the
2026-08-31 pass already had from `deserializers.go` alone (the API
reference is generated from the same model). Weighed fixing this as the
"operation's model declares no not-found code at all -> no error" bug
shape (this service's own `TopicRule` family and workmail's
`DeleteMobileDeviceAccessOverride` both confirmed real instances of this
shape elsewhere): none of the 4 operations' declared codes fit "resource
missing" semantically (`ConflictException`/`ValidationException` are both
wrong shape), so an idempotent no-op read as the only internally-consistent
value among the declared set. But per this service's own prior verdict on
these exact 4 operations, that inference is not proof of AWS's actual
behavior absent either an explicit doc sentence (which workmail had and
this doesn't) or an idempotency-token argument that would need to apply
uniformly (`DeletePackage`/`DeletePackageVersion` carry a `clientToken`
idempotency parameter consistent with idempotent-retry semantics;
`DeleteCommand`/`DeleteCommandExecution` carry no such parameter, so the
same argument doesn't explain their identical gap). No new evidence
changes the prior pass's tiebreak. Left unchanged, matching gopherstack-yr88's
own instruction to fix only unambiguous findings and describe an ambiguous
one for the filer rather than guess: the two live options are (a) make all
4 idempotent (return success, no error, on an already-gone resource) or
(b) leave the current `ResourceNotFoundException` emission as-is since no
declared alternative is demonstrably more correct. Both were fully
implemented, regression-tested (table-driven idempotency test plus a
negative case proving sibling Get/Update ops still 404), lint-clean, and
neutered-verified to actually be reachable by the assertions during this
pass, then reverted before commit once the ambiguity above was recognized
-- reported here rather than left silently reverted.

No code changes this pass. Gates: `go build ./services/iot/...` clean,
`go test -race -count=1 ./services/iot/...` pass (unchanged from baseline),
`golangci-lint run ./services/iot/...` `0 issues`. `errtargetaudit` iot
count after this pass: unchanged at 10 (6 confirmed-correct-but-flagged,
4 confirmed-real-but-ambiguous) -- filed as gopherstack-yr88 stays open for
a human tiebreak on the 4, not because verification was skipped.
