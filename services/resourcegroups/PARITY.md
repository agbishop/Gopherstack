---
service: resourcegroups
sdk_module: aws-sdk-go-v2/service/resourcegroups@v1.36.4
last_audit_commit: f01b0c551   # HEAD when this audit started (errtargetaudit sweep, 2026-09-07)
last_audit_date: 2026-09-07
overall: A            # clean pass this sweep -- no wire bugs found; see notes
                      # 2026-08-29 (request-direction sweep): checked every List/Describe/Get op's REQUEST side (filter/sort/time-range/pagination/precondition members from the real Input struct), not just response shape -- a prior "wire: ok" here had only ever been verified response-side. FOUND AND FIXED one real dropped-filter bug: ListGroupingStatuses' Filters member (real ListGroupingStatusesFilterName values "status"/"resource-arn") had no field at all on gopherstack's listGroupingStatusesInput wire struct, so json.Unmarshal silently discarded it and every real client's Filters was a no-op. Fixed via a new ListGroupingStatusesFilter type threaded through StorageBackend.ListGroupingStatuses (interfaces.go/resources.go/handler_resources.go) and proven by Test_ListGroupingStatuses_FiltersRoundTrip (list_grouping_statuses_filters_test.go), which drives the real typed aws-sdk-go-v2/service/resourcegroups client and includes a non-matching (FAILED-status / other-ARN) record the filter must EXCLUDE. Every other List/Describe/Get op's filter/pagination members (ListGroups.Filters, ListGroupResources.Filters, ListTagSyncTasks.Filters, SearchResources's ResourceQuery.Query ResourceTypeFilters) were re-checked and confirmed already correctly read and applied -- see gaps: for the one already-disclosed, structurally-blocked exception (SearchResources' TagFilters, which needs a cross-service tag registry this backend does not have; left as previously documented, not fabricated).
ops:
  CreateGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: Tags/ResourceQuery no longer nested inside Group; Owner tag renamed; now accepts Owner/DisplayName/Criticality at creation time via CreateGroupOption; Criticality range corrected to 1-10"}
  GetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: Owner wire tag"}
  UpdateGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: Owner wire tag, now includes ApplicationTag; now accepts Owner input field; Criticality range corrected to 1-10 (was 1-5)"}
  DeleteGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now echoes deleted Group (was empty envelope)"}
  ListGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: GroupIdentifiers now include DisplayName/Criticality/Owner; Filters now support the real owner/display-name/criticality GroupFilterName values; invented name-prefix filter removed"}
  GetGroupQuery: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateGroupQuery: {wire: ok, errors: ok, state: ok, persist: ok}
  GetGroupConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: removed fabricated GroupName field, added required Status field"}
  PutGroupConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GroupResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now rejects a group with a ResourceQuery (BadRequestException) instead of silently accepting membership writes on a query-based group -- see 'Real bugs fixed this sweep'"}
  UngroupResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: same ResourceQuery-group rejection as GroupResources"}
  ListGroupResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: deprecated ResourceIdentifiers field now populated identically to Resources; QueryErrors field now present on the wire (always empty -- see gaps, CFN-stack queries not modeled)"}
  ListGroupingStatuses: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: UpdatedAt now epoch-seconds, was RFC3339 string. FIXED 2026-08-29 (request direction): Filters (Name: status/resource-arn) had no field on the wire input struct at all -- silently dropped by json.Unmarshal, every real client's Filters was a no-op. Now a real ListGroupingStatusesFilter, threaded through StorageBackend.ListGroupingStatuses and applied by groupingStatusMatchesFilters (resources.go) before pagination. FIXED 2026-09-07 (gopherstack-m4k0 errtargetaudit class A finding) -- 'errors: ok' was also overstated: a nonexistent Group returned NotFoundException, but deserializeOpErrorListGroupingStatuses declares no such exception for this op (only BadRequestException/ForbiddenException/InternalServerErrorException/MethodNotAllowedException/TooManyRequestsException/UnknownError), unlike sibling ListGroupResources, which keys on the same required Group member and does declare NotFoundException. See Notes."}
  SearchResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: QueryErrors field now present on the wire (always empty -- see gaps, CFN-stack queries not modeled)"}
  GetTags: {wire: ok, errors: ok, state: ok, persist: ok}
  Tag: {wire: ok, errors: ok, state: ok, persist: ok}
  Untag: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-7opw (2026-09-08): extractUntagKeys' two error branches (ReadBody failure, JSON-unmarshal failure) wrote their own error body via h.handleError and returned that (always-nil) result; handleUntagRequest tested that nil and fell through to call Backend.RemoveTagsByARN with keys == nil, writing a second body on top of the committed one (gopherstack-8haq shape). Confirmed Tags.DeleteKeys(nil) is a genuine no-op (ranges over a nil slice), so no tag was ever actually removed -- only the response was corrupted, not state. Fixed to return the raw error; handleUntagRequest now maps and writes it exactly once via handleError."}
  GetAccountSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAccountSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  StartTagSyncTask: {wire: ok, errors: ok, state: ok, persist: ok}
  CancelTagSyncTask: {wire: ok, errors: partial, state: ok, persist: ok, note: "fixed: was setting fabricated CANCELLED status (not a valid TagSyncTaskStatus wire value); now deletes the task per AWS docs. 2026-09-07 (gopherstack-m4k0 errtargetaudit): flagged NotFoundException on a not-found guard -- confirmed not in this op's modeled error set (BadRequestException/ForbiddenException/InternalServerErrorException/MethodNotAllowedException/TooManyRequestsException/UnauthorizedException only). Left unfixed: an initial pass removed the guard on the theory the doc's 'HTTP 200 response with an empty HTTP body' sentence meant idempotent delete, but that sentence is generic Response-shape boilerplate present verbatim on this service's own GetGroup/DeleteGroup, which DO error on not-found -- not evidence either way (same trap that got codepipeline's DeletePipeline/DeleteCustomActionType reverted under gopherstack-3djp). BadRequestException (which this op does declare) is a plausible alternative but unconfirmed by any doc or sibling behavior. Reverted. See the landmine comment in tagsync.go."}
  GetTagSyncTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: CreatedAt now epoch-seconds, was ISO8601 string"}
  ListTagSyncTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: CreatedAt now epoch-seconds, was RFC3339 string (default time.Time marshal)"}
families:
  route_matcher: {status: ok, note: "verified every REST path/method (POST for all ops except GET/PUT/PATCH /resources/{Arn}/tags) against serializers.go opPath/request.Method -- exact match, no gaps"}
gaps:
  - "SearchResources/ListGroupResources' QueryErrors field is present on the wire but always empty. CLOUDFORMATION_STACK_INACTIVE/NOT_EXISTING/UNASSUMABLE_ROLE only ever arise for CLOUDFORMATION_STACK_1_0-based groups (AWS docs describe them as occurring when 'the CloudFormation stack on which the query is based either does not exist, or has a status that renders the stack inactive'); RESOURCE_TYPE_NOT_SUPPORTED is likewise CFN-only by mechanism -- it is absent from SearchResourcesOutput.QueryErrors' own possible-value doc (api_op_SearchResources.go:79-86, aws-sdk-go-v2/service/resourcegroups@v1.36.4, lists only the 3 CFN codes) but present on ListGroupResourcesOutput's (api_op_ListGroupResources.go:105-108, lists all 4), and it is never documented independently of the 3 explicit CFN codes anywhere in the public API reference. Ordinary TAG_FILTERS_1_0 queries can't hit it: tag:GetResources (which backs tag-based membership) only ever returns already-taggable/supported resources, so there's no 'unsupported type' to error on outside a CFN-stack-to-Resource-Groups-type translation. Re-checked this sweep: 'no CFN stack backend' is stale framing -- gopherstack has a real CloudFormation stack backend (services/cloudformation) and an established cross-service wiring pattern for a Provider to reach another service's backend (services/sagemakerruntime/provider.go's sagemakerHandlerProvider; services/cloudformation/provider.go's BackendsProvider), so this isn't structurally blocked. What's actually missing: resourcegroups' Provider.Init (provider.go) never receives a CloudFormation backend reference, and SearchResources/ListGroupResources' CLOUDFORMATION_STACK_1_0 path (query.go's SearchResources, resources.go's ListGroupResources) doesn't evaluate the query against real stack resources at all -- it silently falls through to the same manually-grouped-ARN set used for every other group, CFN-scoping and all. Finishing this needs a handler-provider interface threaded into Provider.Init to reach the cloudformation backend, real CLOUDFORMATION_STACK_1_0 query evaluation against that stack's resources, and the stack-state checks that drive the three CFN error codes -- left undone this sweep as cross-service wiring, out of the single-service scope of this pass. (bd: gopherstack-rg-cfn-queryerrors)"
  - "ListGroupResourcesItem.Status (real types.ResourceStatus) is never populated. Confirmed via AWS docs (API_ListGroupResourcesItem.html): 'This field is present in the response only if the group is of type AWS::EC2::HostManagement.' That's a group *configuration* type (License Manager host resource groups over EC2 Dedicated Hosts), which this emulator does allow-list (groups.go's validConfigTypes already accepts AWS::EC2::HostManagement), and the underlying resource type (EC2 Dedicated Hosts) is itself modeled elsewhere in gopherstack (services/ec2's AllocateHosts/etc) -- so the prior framing ('a resource type this backend has no notion of') overstated the gap. The real reason Status can never be right here: GroupResources/UngroupResources (resources.go) are fully synchronous in this backend for every group, HostManagement-configured or not -- membership changes complete before the call returns, so there is never a window in which a resource is genuinely 'not completed yet' (the only state ResourceStatus.Name can hold is PENDING). Populating Status would mean fabricating an async delay this backend doesn't otherwise model anywhere, which the no-stub rule rules out. Legitimate, permanent absence -- not fixable without inventing asynchronicity, not a resource-type gap. (bd: gopherstack-rg-hostmgmt-status)"
  - "TAG_FILTERS_1_0/CLOUDFORMATION_STACK_1_0 groups have no dynamic membership evaluation: ListGroupResources/SearchResources both source 'membership' from the manually-tracked groupResources store (populated only by GroupResources), not from evaluating the query against real tagged resources or a CFN stack. As of this sweep, GroupResources/UngroupResources correctly reject being called on a group that has a ResourceQuery (see 'Real bugs fixed this sweep'), which means a TAG_FILTERS_1_0/CLOUDFORMATION_STACK_1_0 group's ListGroupResources is now honestly always-empty rather than showing whatever was manually (and invalidly) added -- surfacing, rather than introducing, this gap. A TAG_FILTERS_1_0 query's TagFilters (key/value pairs) are also parsed (tagFilterQuery.TagFilters) but never applied -- only ResourceTypeFilters is honored (query.go's SearchResources/parseResourceTypeFilters). Needs a real tagged-resource evaluation path (or cross-service tag registry access) to close; not attempted this sweep."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors; CancelTagSyncTask fix removes the only TTL-dependent eviction path's live producer (tagSyncTaskTTL eviction in ListTagSyncTasks is now effectively dead code since nothing sets a non-ACTIVE status, but is left as harmless defensive generic logic for a future ERROR-status producer)"}
---

## Notes

Protocol: **rest-json1**. Every op except `Tag` (PUT), `Untag` (PATCH), and `GetTags`
(GET) uses POST, including "list"/"search" verbs (`ListGroups` is `POST /groups-list`,
`SearchResources` is `POST /resources/search`, `UpdateGroupQuery` is
`POST /update-group-query` -- note: NOT PUT, despite the name). All of this was
verified directly against `serializers.go`'s `opPath`/`request.Method` in
aws-sdk-go-v2/service/resourcegroups@v1.36.4 (re-verified 2026-08-20; the prior
note here cited @v1.33.22, stale relative to this file's own `sdk_module`
header even at the time -- see the 2026-08-20 sweep section below) and matches
gopherstack's `rgRESTPathOps` exactly; no route-matcher bugs found this sweep.

### Real bugs fixed this sweep

1. **`Owner` wire tag was `OwnerId`.** The real `types.Group`/`types.GroupIdentifier`
   JSON field is `Owner` (confirmed via `deserializers.go`'s `awsRestjson1_deserializeDocumentGroup`
   `case "Owner":`), not `OwnerId`. gopherstack additionally fabricated its value as the
   AWS account ID on every `CreateGroup` call; real `Owner` is an optional, free-form,
   caller-supplied string (person/email/team) with no relation to account ID, and is
   `nil`/absent unless the caller supplies it. Both the tag and the fabrication were
   fixed; `Owner` is now unset by default (see gaps: no input path exists to set it).

2. **`CreateGroupOutput` nested `Tags`/`ResourceQuery` inside the `Group` object.**
   The real `types.Group` shape has no `Tags` or `ResourceQuery` members -- both travel
   as separate top-level sibling fields of `CreateGroupOutput`
   (`{"Group":{...},"ResourceQuery":{...},"Tags":{...}}`). gopherstack was marshaling
   the internal `*Group` backend struct directly as the `"Group"` field, which (a) leaked
   `Tags`/`ResourceQuery` into the wrong place and (b) meant a real SDK client's
   `CreateGroupOutput.Tags` was always empty even though the caller passed tags on
   create -- real data loss for any real-SDK caller. Fixed by building the wire shape
   from a dedicated `getGroupBody` (now shared by CreateGroup/GetGroup/UpdateGroup) and
   adding a top-level `Tags` field.

3. **`GetGroupConfiguration` fabricated a `GroupName` field and omitted `Status`.**
   Real `types.GroupConfiguration` is `{Configuration, FailureReason,
   ProposedConfiguration, Status}` -- no `GroupName`. `Status` (`"UPDATE_COMPLETE"` once
   a configuration is set) was entirely missing from the response. Fixed by reusing the
   `groupConfigurationBody` type (already correct, used by `CreateGroup`'s inline
   `GroupConfiguration`) instead of a bespoke shape.

4. **`CancelTagSyncTask` transitioned to a fabricated `"CANCELLED"` status.**
   `TagSyncTaskStatus`'s only documented wire values are `ACTIVE` and `ERROR`
   (confirmed via `types/enums.go` and the AWS API reference); there is no `CANCELLED`
   value. AWS's own docs describe `CancelTagSyncTask` as taking "the TaskArn of the
   tag-sync task **you want to delete**", and the operation requires
   `resource-groups:DeleteGroup` permission. gopherstack instead kept the task alive for
   24h with an invalid enum value, which a prior audit pass (`handler_audit1_test.go`
   "issue #22") had deliberately locked in as expected behavior -- re-verified against
   AWS docs this sweep and reverted: `CancelTagSyncTask` now deletes the task outright,
   and `GetTagSyncTask`/`ListTagSyncTasks` no longer find it afterward.

5. **`TagSyncTask.CreatedAt` and `GroupingStatusItem.UpdatedAt` were RFC3339/ISO8601
   strings, not epoch-seconds numbers.** rest-json1 uses the `unixTimestamp` format
   (JSON number of seconds since epoch) for these fields (confirmed via
   `deserializers.go`'s `smithytime.ParseEpochSeconds` calls) -- the exact bug class
   flagged in `.claude/memories/parity-principles.md`. `GetTagSyncTask` was manually
   formatting an ISO8601 string; `ListTagSyncTasks`/`ListGroupingStatuses` were relying
   on `time.Time`'s default `encoding/json` marshaling (RFC3339). Fixed with
   `pkgs/awstime.Epoch` via new wire DTOs (`tagSyncTaskItem`, `groupingStatusItemWire`)
   that keep the backend's internal `time.Time` representation (used for persistence and
   pagination-token derivation) separate from the wire encoding.

6. **`DeleteGroup` returned an empty envelope.** Real `DeleteGroupOutput.Group` echoes
   back a full description of the just-deleted group. gopherstack discarded it. Fixed:
   `InMemoryBackend.DeleteGroup` now returns `(*Group, error)` (was `error`), and the
   handler echoes it via `getGroupBody`.

7. **`ListGroups`' `GroupIdentifiers` omitted `DisplayName`/`Criticality`/`Owner`.**
   Real `types.GroupIdentifier` carries all of `GroupName`, `GroupArn`, `Description`,
   `DisplayName`, `Criticality`, `Owner`. Only `GroupName`/`GroupArn`/`Description` were
   populated. Fixed (Owner remains unset per gap #2 above, same as everywhere else).

### Real bugs fixed this sweep (parity-3, 2026-07-24)

8. **`ListGroups` `Filters` supported an invented `"name-prefix"` filter name and was
   missing three real ones.** The real `types.GroupFilterName` enum (confirmed via
   `types/enums.go`'s `GroupFilterName.Values()`) is exactly
   `resource-type|configuration-type|owner|display-name|criticality` -- there is no
   `name-prefix` value. gopherstack had fabricated `name-prefix` and silently ignored
   `owner`/`display-name`/`criticality` filters (matching everything instead of
   filtering, since the `switch` had no `case` for them). Fixed: `name-prefix` and its
   handler/tests were deleted; `owner`/`display-name` (exact string match) and
   `criticality` (exact match against `strconv.Itoa(g.Criticality)`) are now
   implemented in `groupMatchesFilters`.

9. **`CreateGroup` had no input path for `Owner`/`DisplayName`/`Criticality`.** The real
   `CreateGroupInput` accepts all three directly (confirmed via `api_op_CreateGroup.go`);
   gopherstack only allowed setting them via a follow-up `UpdateGroup` call, so a
   real-SDK caller's `CreateGroupInput.Owner`/`DisplayName`/`Criticality` were silently
   dropped on create. Fixed: `InMemoryBackend.CreateGroup` gained a
   `...CreateGroupOption` variadic parameter (`WithOwner`/`WithDisplayName`/
   `WithCriticality`) -- chosen over widening the positional parameter list to avoid
   breaking the ~65 existing call sites that already pass exactly six positional
   arguments -- and the `CreateGroup` handler now threads `Owner`/`DisplayName`/
   `Criticality` from `CreateGroupInput` through to it.

10. **`UpdateGroup` had no input path for `Owner`.** The real `UpdateGroupInput` accepts
    `Owner` alongside `Description`/`DisplayName`/`Criticality` (confirmed via
    `api_op_UpdateGroup.go`); gopherstack's `UpdateGroup` never accepted or updated it.
    Fixed: `InMemoryBackend.UpdateGroup` gained an `owner string` parameter (leaves
    `Owner` unchanged when empty, matching the existing `displayName`/`criticality`
    convention), and the handler now reads `Owner` from `UpdateGroupInput`.

11. **`Criticality` validation used the wrong range (1-5 instead of 1-10).** Both
    `api_op_CreateGroup.go` and `api_op_UpdateGroup.go` document Criticality as "a scale
    of 1 to 10, with a rank of 1 being the most critical, and a rank of 10 being least
    critical" -- gopherstack's `UpdateGroup` rejected valid values 6-10 with a
    `BadRequestException`. Fixed via a shared `validateCriticality` helper (now also
    used by the new `CreateGroup` Criticality input) enforcing the correct 1-10 range.

12. **`ListGroupResourcesOutput` omitted the deprecated `ResourceIdentifiers` field.**
    The real API keeps `ResourceIdentifiers` populated alongside `Resources` for
    backward-compatible clients that still read the deprecated field (confirmed via
    `api_op_ListGroupResources.go`); gopherstack only populated `Resources`. Fixed:
    `ResourceIdentifiers` is now populated identically to `Resources`.

13. **`SearchResourcesOutput`/`ListGroupResourcesOutput` were missing the `QueryErrors`
    field entirely.** Both real output shapes carry a `QueryErrors []types.QueryError`
    member (confirmed via `api_op_SearchResources.go`/`api_op_ListGroupResources.go`);
    it was absent from gopherstack's wire structs. Added a `queryErrorWire` DTO and the
    field to both outputs (`omitempty`, so it serializes identically to real AWS for the
    common case of no errors). It remains always-empty here since
    `CLOUDFORMATION_STACK_1_0`-based groups (the only source of `QueryError`s) are not
    modeled -- see gaps.

### Real bugs fixed this sweep (parity-5, 2026-08-10)

14. **`GroupResources`/`UngroupResources` accepted any group, including one with a
    `ResourceQuery`.** Real `GroupResources` docs carry an explicit "Important" callout:
    "You can only use this operation with the following groups:
    `AWS::EC2::HostManagement`, `AWS::EC2::CapacityReservationPool`,
    `AWS::ResourceGroups::ApplicationGroup`. Other resource group types ... are not
    currently supported by this operation." `UngroupResources` docs are equally explicit:
    "This operation works only with static groups that you populated using the
    `GroupResources` operation. It doesn't work with any resource groups that are
    automatically populated by tag-based or CloudFormation stack-based queries."
    gopherstack's `InMemoryBackend.GroupResources`/`UngroupResources` (resources.go)
    checked only that the named group existed, so a caller could silently write explicit
    membership onto a `TAG_FILTERS_1_0`/`CLOUDFORMATION_STACK_1_0` group -- membership
    the real API computes dynamically from the query and would reject a manual write for.
    Fixed: both now return `BadRequestException` (`ErrValidation`) when the target
    group's `ResourceQuery != nil`, proven by `TestGroupResources_RejectsQueryBasedGroup`
    (failed before the fix: `An error is expected but got nil`). Narrower than the full
    3-configuration-type allow-list the docs describe (that stricter check isn't
    implemented, since none of the 3 types have any real behavior modeled here either --
    would need its own audit before enforcing); see gaps for the resulting exposure of
    query-based groups' membership evaluation gap.

### Real bugs fixed this sweep (errtargetaudit, 2026-09-07, gopherstack-m4k0)

`cmd/errtargetaudit` flagged 2 class A findings for resourcegroups (both
`NotFoundException`/sentinel-reference, `CancelTagSyncTask` and
`ListGroupingStatuses`). Both were real: gopherstack emitted
`NotFoundException` for a nonexistent key parameter on an op whose real
declared error set (`deserializers.go`) has no such exception, even though a
structurally identical sibling op does declare it. Not the filter-as-key
false-positive shape flagged in the audit's own leads doc -- `Group`
(`ListGroupingStatuses`) and `TaskArn` (`CancelTagSyncTask`) are both
`// This member is required` per the real Input structs
(`api_op_ListGroupingStatuses.go`, `api_op_CancelTagSyncTask.go`), not
optional filters. Only one of the two had a confirmed remedy; see below.

16. **`ListGroupingStatuses` 404'd on an unknown `Group`. FIXED.** Raw
    extraction from `deserializeOpErrorListGroupingStatuses`: `UnknownError`,
    `BadRequestException`, `ForbiddenException`, `InternalServerErrorException`,
    `MethodNotAllowedException`, `TooManyRequestsException` -- no
    `NotFoundException`. Sibling `ListGroupResources` keys on the identical
    required `Group` member and DOES declare `NotFoundException`
    (`deserializeOpErrorListGroupResources`) -- this op appears to have been
    cloned from that sibling without checking its own declared set, the same
    class ecr's `DescribeRepositoryCreationTemplates` finding was. This is a
    List op: the declared-set mismatch forces the remedy, because there is
    nothing else to return but an empty list (same shape as rds
    `DescribeDBClusterEndpoints`, ecr `ListImageReferrers`, firehose
    `ListDeliveryStreams`). Fixed: `InMemoryBackend.ListGroupingStatuses`
    (resources.go) no longer checks group existence; an unknown group now
    falls through to the same map-lookup-on-absent-key path already used for
    a group with zero grouping history, yielding
    `{"Group": "...", "GroupingStatuses": []}` at 200.

    Proven by neutering (the guard was manually restored, confirmed to still
    build, and confirmed to fail `TestResourceGroupsHandler_ListGroupingStatuses`'s
    `nonexistent_group_returns_empty` case at its
    `wantContains: []string{"\"GroupingStatuses\":[]"}` check -- body was the
    404 error instead) and by a corrected pre-existing test: that case
    previously asserted `wantCode: http.StatusNotFound` under the name
    `"not_found"` with no note disclosing it as a known shortcut; renamed and
    reasserted at `http.StatusOK`. `TestErrorShapes` (handler_test.go) had its
    `list_grouping_statuses_404` case deleted (no longer belongs in a
    404-only table) with a one-line comment on the test explaining the
    omission.

15. **`CancelTagSyncTask` 404s on an unknown `TaskArn`. LEFT UNFIXED --
    evidenced but unresolved, see the landmine comment in tagsync.go.** Raw
    extraction from `deserializeOpErrorCancelTagSyncTask`: `UnknownError`,
    `BadRequestException`, `ForbiddenException`, `InternalServerErrorException`,
    `MethodNotAllowedException`, `TooManyRequestsException`,
    `UnauthorizedException` -- no `NotFoundException`, confirmed against both
    the live docs.aws.amazon.com/ARG/latest/APIReference/API_CancelTagSyncTask.html
    Errors section and botocore's service-2.json documentation string (neither
    carries idempotency language). Sibling
    `GetTagSyncTask`/`StartTagSyncTask`/`DeleteGroup` all declare
    `NotFoundException` for their own required-key lookups, so the absence is
    a real per-op asymmetry, not a scan artifact -- but unlike
    `ListGroupingStatuses`, this is a mutate op with no forced remedy. An
    earlier pass in this sweep removed the guard and made the op
    idempotent-success, citing the doc's "HTTP 200 response with an empty
    HTTP body" sentence as evidence -- reverted on review: that sentence is
    generic Response-shape boilerplate that also appears verbatim on this
    same service's `GetGroup`/`DeleteGroup`, which DO error on not-found, so
    it is not evidence of idempotent-delete semantics (the identical trap
    that got codepipeline's `DeletePipeline`/`DeleteCustomActionType`
    reverted under gopherstack-3djp). The declared `BadRequestException` is a
    plausible alternative remedy -- an unmatched `TaskArn` could map to it --
    but no doc or sibling behavior confirms AWS actually does this rather
    than something else; it remains inference, not a confirmed replacement.
    Reverted to the original guard with a landmine comment naming both
    candidates. All test changes made against this op earlier in the sweep
    (`TestResourceGroupsHandler_CancelTagSyncTask`'s table case,
    `TestCancelTagSyncTask_NotFound`, `TestErrorShapes`'s
    `cancel_task_not_found_404` case, and
    `TestResourceGroupsRegionIsolation`'s cross-region assertion in
    isolation_test.go) were reverted to their original form alongside it.

No wire, persist, or state changes beyond the one error-path removal
(`ListGroupingStatuses`); no persisted struct field was added, so the
`pkgs/persistence` guard was not run.

### Wire-shape traps confirmed correct (do not re-flag)

- `ListGroupResourcesItem`'s wire shape (`Identifier`, optional `Status`) matches;
  `Status` is legitimately omitted -- see gaps for why (it's a synchronicity gap, not a
  missing-resource-type gap).
- `GroupResourcesOutput`/`UngroupResourcesOutput`'s `Pending` field reusing
  `GroupingFailedItem`'s shape (`ResourceArn`/`ErrorCode`/`ErrorMessage`) instead of the
  real `PendingResource` shape (`ResourceArn` only) is currently harmless: `Pending` is
  always an empty array (all grouping/ungrouping here is synchronous), so the extra
  fields never actually serialize. Would need fixing if async pending-state support is
  ever added.
- `ListGroupsFilter`'s "configuration-type"/"resource-type" filter *values* match real
  behavior; only the filter *names* diverge (see gaps).

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors (5 sites)

resourcegroups is restjson1 (confirmed from `resourcegroups@v1.36.4`
deserializers.go's `awsRestjson1_deserializeOpError*` prefix). Its
JSON-RPC-shaped path already routes through `pkgs/service.HandleTarget`,
fixed by c6554e9f8. But this service ALSO has its own REST-path dispatch
that never touches that helper: `handleREST` (static `/groups`,
`/get-group`, etc. paths) and three sites in `handler_tags.go`
(`handleTagRequest`'s PUT, `extractUntagKeys`'s DELETE, and the PATCH
compat-alias branch of `handleResourceTags`) -- all 5 wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")` on a
`ReadBody` failure. Plain text doesn't decode through
`aws/protocol/restjson.GetErrorInfo`, so a real client got
`*json.SyntaxError`, not even `UnknownError`.

Fixed all 5 identically: route the `ReadBody` error through this package's
own `handleError(ctx, c, action, err)` instead of the bare `c.String`. None
of the 5 errors match any of `handleError`'s typed `case`s (`errInvalidRequest`,
`ErrUnknownOperation`, `json.SyntaxError`/`UnmarshalTypeError`,
`ErrAlreadyExists`, `ErrValidation`, `ErrNotFound`, `ErrTagSyncTaskNotFound`),
so all 5 fall through to the pre-existing default (`InternalServerErrorException`,
500) -- modeled at `resourcegroups@v1.36.4` `types/errors.go:71`, and already
sets the `X-Amzn-Errortype` header via `amznErrorTypeHeader`.

Proven with a real `aws-sdk-go-v2/service/resourcegroups` client's
`CreateGroup` (the static-REST-path `handleREST` site), whose `Description`
field alone exceeds `httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandleREST_OversizedBodySurfacesInternalServerErrorException`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalServerErrorException"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after). The
other 4 sites share the identical one-line fix and were not independently
client-driven given the campaign's budget across 14 services.
### 2026-08-20 sweep (wrapper-key / nested-shape pass) -- clean, zero new wire bugs

Full 23-op re-read against resourcegroups@v1.36.4's own per-op deserializer, focused on
the wrapper-key/nesting/type/enum-value bug class. Confirmed protocol restjson1 from
both `serializers.go`'s `awsRestjson1_*` prefix (184 serializer, 398 deserializer hits)
and `api_client.go`; cross-checked `services/_PROTOCOLS.md`'s resourcegroups row, which
is correct. Checked all 23 ops' `awsRestjson1_deserializeOpDocument<Op>Output` call
count: 21 are called (wrapped, live), and `CancelTagSyncTask`/`PutGroupConfiguration`
have no such function at all (both real Outputs carry zero members beyond
`ResultMetadata` -- legitimately void, not the flat-decode false-positive trap; no op in
this service hits that trap).

Re-verified, field-by-field against `types/types.go` and the per-op `api_op_*.go`
Output structs: `Group`/`GroupIdentifier` (`ListGroups`' dual `Groups`/`GroupIdentifiers`
fields, both still correctly populated and correctly shaped -- `Group` uses `Name`,
`GroupIdentifier` uses `GroupName`, exactly matching the real types, which gopherstack's
`listGroupsGroupOutput`/`listGroupIdentifierOutput` already get right), `ResourceQuery`/
`GroupQuery` (wrapper under `"GroupQuery"`, matches `types.GroupQuery{GroupName,
ResourceQuery}`), `GroupConfiguration`/`GroupConfigurationItem`/
`GroupConfigurationParameter` (`Type`/`Parameters`/`Name`/`Values` all match),
`ListGroupResourcesItem`/`QueryError` (`Message` not `ErrorMessage` on `QueryError` --
confirmed via `deserializers.go`'s `awsRestjson1_deserializeDocumentQueryError`, and
gopherstack's `queryErrorWire` already gets this right), `FailedResource`/
`GroupingFailedItem`, `AccountSettings`, `TagSyncTaskItem` (flat `GetTagSyncTaskOutput`/
`StartTagSyncTaskOutput`, both correctly un-wrapped in gopherstack), and every enum in
`types/enums.go` (`QueryType`, `QueryErrorCode`, `GroupFilterName`, `ResourceFilterName`,
`GroupConfigurationStatus`, `GroupingStatus`, `GroupingType`, `TagSyncTaskStatus`,
`GroupLifecycleEventsDesiredStatus`/`Status`) against every literal string gopherstack
emits for that field. No wrong key, no wrong nesting level, no wrong JSON type, no wrong
or invented enum value found anywhere in this pass -- this service's prior sweeps
(parity-3/4/5, 14 fixes total) already closed the bug class this pass was looking for.
Also cross-checked all 23 ops' modeled error sets against botocore's
`resource-groups/2017-11-27/service-2.json` (installed locally): identical to the Go
SDK's per-op `awsRestjson1_deserializeOpError<Op>` switches, no discrepancy.

One incidental, disclosed-not-fixed observation while comparing `ListGroups`' deprecated
`Groups []types.Group` field against `types.Group`'s own member list
(`aws-sdk-go-v2/service/resourcegroups@v1.36.4/types/types.go`, `type Group struct`):
real `Group` carries an `ApplicationTag map[string]string` member that gopherstack's
`listGroupsGroupOutput` (handler_groups.go) does not emit, even though the sibling
`getGroupBody` (used by `CreateGroup`/`GetGroup`/`UpdateGroup`/`DeleteGroup`) already
does. Left unfixed: `ApplicationTag` is never set by any operation in this backend
(`grep -rn ApplicationTag services/resourcegroups/*.go` shows it only read/persisted,
never written) -- it is real "application group" state this emulator's group lifecycle
doesn't model at all, so there is no way to reach a non-nil value through any real
handler call, which means a fix here couldn't be proven by an SDK round-trip test (the
field would still always serialize as absent, before and after). Structurally identical
to the already-documented `ListGroupResourcesItem.Status`/`Pending` async gaps -- noting
it rather than forcing an unprovable "fix." (bd: gopherstack-rg-applicationtag-listgroups)

Also noted, not a wire bug: `GroupConfiguration.ProposedConfiguration`/`FailureReason`
(real members alongside `Configuration`/`Status`) are never populated by
`GetGroupConfiguration`/`CreateGroup`'s inline `GroupConfiguration`, for the same
already-documented reason as the `ListGroupResourcesItem.Status` gap above --
`PutGroupConfiguration` is fully synchronous in this backend, so there is never an
in-flight update to report as `ProposedConfiguration` with a `FailureReason`. Not
independently fixable without inventing an async config-update path this backend
doesn't otherwise have.

**Provenance check on this file's prior `last_audit_commit`.** The value this file
carried before this sweep, `343e1204`, is `git show -s --format=%ad 343e1204` ->
`Mon Jul 13 02:23:26 2026 -0500`, eleven days before the `last_audit_date: 2026-07-24`
this file claimed. Per this session's standing finding (four PARITY.md files caught with
this same tell, three of them citing this exact `343e1204` sha), that gap means the
audit-commit pointer was stale/copy-pasted rather than genuinely the HEAD the 2026-07-24
sweep ran against -- it does not mean the 2026-07-24 sweep's *content* is wrong (its
14 documented fixes were independently re-verified against the pinned SDK in this
2026-08-20 pass and all still hold), only that its provenance metadata shouldn't be
trusted at face value. Separately, this file's own prose (the route-matcher paragraph
above) cited SDK version v1.33.22 while the YAML header already said v1.36.4 -- a real,
if harmless, drift between the two; corrected in place this sweep. `last_audit_commit`
above is now set to this sweep's actual starting HEAD (`a8a59e42`).

**Per-item-failure sweep (this pass):** re-checked `GroupResources`/`UngroupResources`
(`Failed []types.FailedResource`) and `ListGroupResources`/`SearchResources`
(`QueryErrors []types.QueryError`). `GroupResources`/`UngroupResources.Failed` are
correctly populated (handler-level `INVALID_ARN` for a malformed ARN, backend-level
`RESOURCE_NOT_FOUND` for an ARN not currently a group member) while the rest of the
batch still succeeds. `QueryErrors` on both list/search ops remains genuinely
unreachable in this backend and is already tracked as such (see `gaps:` above,
`bd: gopherstack-rg-cfn-queryerrors`) -- confirmed the reasoning still holds and left
alone, consistent with this sweep's scope boundary against touching
`services/cloudformation`.

### 2026-08-30 value-semantics pass (gopherstack-uox6, bug class: field read/applied but wrong)

Scope: filter *matching* semantics (not shape) for `ListGroups.Filters`,
`ListGroupResources.Filters`, and `ListGroupingStatuses.Filters` -- part of a
3-service pass (guardduty, resourcegroups, ce). Checked against
`aws-sdk-go-v2/service/resourcegroups@v1.36.4/types` directly (`GroupFilter`/
`GroupFilterName`, `ResourceFilter`/`ResourceFilterName`,
`ListGroupingStatusesFilter`/`ListGroupingStatusesFilterName`).

**Confirmed correct, no bug found:**
- `groupMatchesFilters` (`groups.go`) exhaustively switches on all 5 real
  `GroupFilterName` values (`resource-type`, `configuration-type`, `owner`,
  `display-name`, `criticality`) -- no gap, no default fallthrough needed since every
  enum member is handled. AND across filter entries, OR within one entry's `Values`,
  matching the `Name`/`Values` filter contract every one of these three filter types
  documents identically ("One or more filter values ... Filter names are case-sensitive
  ... filter values ... are case-sensitive"). String comparisons throughout are plain
  `==`/`slices.Contains` (case-sensitive), matching that documented case-sensitivity --
  no `EqualFold` leniency introduced anywhere in these three matchers.
- `ListGroupResourcesFilter`'s only real `ResourceFilterName` value is `resource-type`
  (confirmed: `types.ResourceFilterName.Values()` returns exactly one member) --
  `resources.go`'s `ListGroupResources` only recognizes that one name, correctly
  matching the enum's full extent.
- `groupingStatusMatchesFilters` (`resources.go`, added in the prior 2026-08-29
  sweep noted above) exhaustively switches on both real `ListGroupingStatusesFilterName`
  values (`status`, `resource-arn`); AND-across-entries/OR-within-values re-verified
  against the same `GroupFilter`-family doc text.
- No range/bound/time-window filter exists anywhere in this service's filter surface --
  every filter here is a plain string-equality allow-list, so the boundary-inclusivity
  check this pass prioritizes does not apply to `resourcegroups`.

No web pages fetched this pass -- everything resolved from the pinned SDK's Go doc
comments and `types/enums.go`.

No bugs found; no files changed. The one pre-existing, already-disclosed gap in this
area (`TAG_FILTERS_1_0`'s `TagFilters` parsed but never applied to narrow membership --
see `gaps:` above) is a field-never-read gap, not a wrong-algorithm one, so it is
outside this pass's class and was re-confirmed rather than touched.

## 2026-09-08 pass (bd gopherstack-t3uf): CancelTagSyncTask / NotFoundException re-audited, left unfixed

Third evidence pass on the finding tracked at gaps item 15 above (originally
gopherstack-m4k0). Re-extracted independently: `ErrTagSyncTaskNotFound`
(`errors.go:13-16`, `awserr.New("NotFoundException: tag-sync task not
found", awserr.ErrNotFound)`) is emitted from `tagsync.go:102-104`:
`if !b.tagSyncTasks.Has(tableKey) { return fmt.Errorf("%w: task %s not
found", ErrTagSyncTaskNotFound, taskARN) }`.
`awsRestjson1_deserializeOpErrorCancelTagSyncTask`
(`resourcegroups@v1.36.4` deserializers.go:63-130) declares exactly
`BadRequestException, ForbiddenException, InternalServerErrorException,
MethodNotAllowedException, TooManyRequestsException,
UnauthorizedException` -- no `NotFoundException`. botocore's
`resource-groups/2017-11-27/service-2.json.gz`
`operations.CancelTagSyncTask.errors` lists the identical six. The live
docs.aws.amazon.com/ARG/latest/APIReference/API_CancelTagSyncTask.html
Errors section (fetched fresh this pass) agrees verbatim and adds no
not-found-specific language; Response Elements says only "HTTP 200
response with an empty HTTP body" -- generic boilerplate, not idempotency
language, per the control-experiment already on record (identical sentence
on this service's own `GetGroup`/`DeleteGroup`, both of which DO error on
not-found and both of which declare `NotFoundException`).

Searched for new oracle evidence not already on record: live AWS API
reference (re-fetched, matches botocore/SDK exactly, no new info), and web
search for real-world reports of `CancelTagSyncTask` against an unknown
`TaskArn` (a relevant AWS re:Post thread exists --
"Deleted an application, but the tag sync remains, blocking me from
recreating the app" -- but returned HTTP 403 to an unauthenticated fetch
and could not be read). No new evidence found either for idempotent-success
or for a `BadRequestException` mapping.

**Left unfixed**, matching the two prior passes' conclusion. Neither
candidate remedy clears the evidence bar: idempotent-success is
contradicted by the GetGroup/DeleteGroup control experiment, and
`BadRequestException` ("The request includes one or more parameters that
violate validation rules" -- botocore shape doc) does not fit an
unmatched-but-well-formed `TaskArn`, which violates no validation rule, it
just doesn't resolve to an existing resource. No test was added or
changed. `TestCancelTagSyncTask_NotFound`
(`handler_tagsync_test.go:346`) and the `cancel_task_not_found_404` case
of `TestErrorShapes` (`handler_test.go:521-526`) both still assert
`http.StatusNotFound` for this path -- flagged here per the audit brief's
instruction on pre-existing tests asserting the current wrong code:
these are deliberately characterizing (not aspirational) tests, disclosed
as such by the doc comment directly above `TestErrorShapes`
(`handler_test.go:470-475`) and by gaps item 15 above; left unedited since
no evidenced replacement behavior exists to assert instead.
