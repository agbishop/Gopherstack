---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: codeconnections
sdk_module: aws-sdk-go-v2/service/codeconnections@v1.13.4   # unchanged this pass; still the go.mod pin
last_audit_commit: bb9dd1e99                      # HEAD at the start of this pass (gopherstack-2y2); this pass's changes are uncommitted working-tree changes on top
last_audit_date: 2026-09-04
overall: A            # true-parity pass: closed every gaps/deferred item from the prior audit, plus new
                       # wire/error-shape bugs found while field-diffing. 2026-08-19 pass found and fixed
                       # 3 additional wrapper-key/nested-shape wire bugs (GetHost x4 fields, ListHosts Tags, GetConnection+ListConnections Tags) that the prior A grade had missed entirely -- see ops notes below and the 2026-08-19 section in Notes. Grade restored to A only because all three are now fixed and proven by hand-revert; the prior A was not honest at the time it was written.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "Tags field on CreateConnectionOutput correct. validProviderTypes has all 6 real ProviderType values. RESOLVED this pass (was a gap): duplicate ConnectionName is no longer rejected. CreateConnection's real error list -- confirmed by reading the operation's own deserializer, aws-sdk-go-v2/service/codeconnections@v1.13.4 deserializers.go's awsAwsjson10_deserializeOpErrorCreateConnection switch -- is exactly [LimitExceededException, ResourceNotFoundException, ResourceUnavailableException]; no ResourceAlreadyExistsException case exists, so a real client sending that error code would fall through to the generic/unmodelled branch. Sibling ops CreateRepositoryLink/CreateSyncConfiguration in the same service DO have a ResourceAlreadyExistsException case in their deserializers, showing the omission here is deliberate, not an SDK oversight. The direction of the prior bug: gopherstack was MORE RESTRICTIVE than real AWS, rejecting creates the real service accepts. The connectionsByName index (its only reader) was removed as dead weight."}
  GetConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-19: the nested Connection object carried an invented Tags field. Real types.Connection (aws-sdk-go-v2/service/codeconnections@v1.13.4) has no Tags member at all (confirmed against awsAwsjson10_deserializeDocumentConnection's case switch: ConnectionArn/ConnectionName/ConnectionStatus/HostArn/OwnerAccountId/ProviderType only). Tags is only ever returned by CreateConnectionOutput. Removed from connectionItem/connToItem (handler_connections.go); TestGetConnectionExcludesTags (was TestGetConnectionIncludesTags, asserting the bug) rewritten to assert absence + verify tags via ListTagsForResource."}
  ListConnections: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-19: same fabricated Tags field as GetConnection above (both share the connectionItem view type). New test TestListConnectionsExcludesTags."}
  DeleteConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateHost: {wire: ok, errors: ok, state: ok, persist: ok, note: "Tags field on CreateHostOutput correct. RESOLVED 2026-07-13 pass: duplicate Name is no longer rejected. CreateHost's real error list, confirmed via its own deserializer (deserializers.go's awsAwsjson10_deserializeOpErrorCreateHost switch), is exactly [LimitExceededException] -- no ResourceAlreadyExistsException. Same direction of bug and same fix as CreateConnection above; the hostsByName index was removed as dead weight. ADDED 2026-08-19: CreateHostInput.VpcConfiguration (real, optional member) is now accepted and stored -- needed to make GetHost/ListHosts' VpcConfiguration fix (see below) observably testable end-to-end."}
  GetHost: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-19 (confirmed bug, sibling gopherstack-2mwl-class): the response fabricated THREE members not on the real GetHostOutput -- HostArn, StatusMessage, Tags -- while omitting the one real optional member, VpcConfiguration, entirely. aws-sdk-go-v2/service/codeconnections@v1.13.4's GetHostOutput struct (api_op_GetHost.go) and its live deserializer awsAwsjson10_deserializeOpDocumentGetHostOutput (confirmed live: grep -c on deserializers.go returns 2, defined+called) both have exactly five members: Name/ProviderEndpoint/ProviderType/Status/VpcConfiguration. HostArn/StatusMessage/Tags were generalized in from the full Host type (used by ListHosts, which DOES legitimately have HostArn/StatusMessage) and from CreateHostOutput (which DOES legitimately have Tags) -- the same 'summarized/generalized from a wider sibling type' mistake class as codestarconnections' prior fix. VpcConfiguration is now backed by a real domain type+backend field (models.go/hosts.go), settable via CreateHost/UpdateHost and returned by GetHost/ListHosts. Fixed in getHostOutput/handleGetHost (handler_hosts.go). New tests: TestGetHost_RawShapeExcludesFabricatedFields (raw-body absence check for all 3 fabricated keys, since the typed SDK GetHostOutput has no fields to bind them to), TestGetHost_VpcConfigurationRoundTrip (real aws-sdk-go-v2 client create-with-VpcConfiguration -> GetHost/ListHosts/UpdateHost, typed field observation). Existing tests TestGetHostIncludesHostArn/TestCreateHostWithTags/TestHostGet asserted the bug as correct and were corrected (renamed TestGetHostExcludesHostArn where the premise flipped)."}
  ListHosts: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-19: hostItem fabricated a Tags field. The real Host type (types.Host) has HostArn/Name/ProviderEndpoint/ProviderType/Status/StatusMessage/VpcConfiguration but NO Tags member at all (confirmed against awsAwsjson10_deserializeDocumentHost's case switch) -- tags for a host are only ever returned via ListTagsForResource, exactly like Connection/GetConnection above. VpcConfiguration (real, previously omitted) added incidentally while fixing GetHost's VpcConfiguration support, since both views share the same backend Host.VpcConfiguration field. Existing test TestListHostsIncludesTags asserted the Tags bug as correct; rewritten as TestListHostsExcludesTags."}
  DeleteHost: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: the in-use check (added in the 2026-07-13 pass) used ConflictException, but DeleteHost's real, complete error list (botocore codeconnections/2023-12-01/service-2.json) is exactly [ResourceNotFoundException, ResourceUnavailableException] -- ConflictException is not a possible error for this operation at all. Changed ErrResourceInUse's wire type to ResourceUnavailableException (its doc note also covers the sibling 'host cannot be deleted while VPC_CONFIG_INITIALIZING/VPC_CONFIG_DELETING' case, the same 'host not currently deletable' family)."}
  UpdateHost: {wire: ok, errors: ok, state: ok, persist: ok, note: "ADDED 2026-08-19: UpdateHostInput.VpcConfiguration (real, optional member) is now accepted and applied -- same reason as CreateHost above. UpdateHostOutput remains correctly empty (only ResultMetadata on the real type)."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok, note: "duplicate-link check present (ResourceAlreadyExistsException IS in CreateRepositoryLink's real error list, confirmed via botocore -- unlike CreateConnection/CreateHost above)."}
  GetRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: response item no longer carries an invented Tags field (see gaps history below -- removed, not merely noted)."}
  DeleteRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok, note: "in-use check present; SyncConfigurationStillExistsException IS in DeleteRepositoryLink's real error list, confirmed via botocore."}
  ListRepositoryLinks: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: response items no longer carry an invented Tags field."}
  CreateSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "duplicate check present (ResourceAlreadyExistsException IS in CreateSyncConfiguration's real error list, confirmed via its deserializer). PullRequestComment (present in this service's pinned SDK types.SyncConfiguration/CreateSyncConfigurationInput) is accepted, stored, and round-tripped through Get/List/persistence. FIXED this pass: PublishDeploymentStatus/TriggerResourceUpdateOn/PullRequestComment were accepted with NO enum validation at all (types/enums.go: PublishDeploymentStatus/PullRequestComment={ENABLED,DISABLED}, TriggerResourceUpdateOn={ANY_CHANGE,FILE_CHANGE}) -- any string, including garbage, was silently stored and echoed back. Now validated against their real enum sets (empty string still allowed -- none of the three are required input members)."}
  GetSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "deletion also removes any syncBlockers rows for that resource+syncType (cascade fix from a prior pass) -- DeleteSyncConfiguration's real error list has no 'blockers still exist'-style exception, so deletion stays unconditional; only the orphaned children are cleaned up."}
  UpdateSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "PullRequestComment updatable (empty string preserves the existing value, matching the PublishDeploymentStatus/TriggerResourceUpdateOn convention). FIXED this pass: same missing enum validation as CreateSyncConfiguration above, same fix (empty string still means 'leave unchanged')."}
  ListSyncConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRepositorySyncStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "epoch-seconds StartedAt/Events[].Time unchanged from 2026-07-13 pass (already correct); RepositorySyncAttempt's real wire shape is only Events/StartedAt/Status (no revision fields), which this response already matched -- unlike ResourceSyncAttempt below."}
  GetResourceSyncStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "MAJOR wire-shape fix this pass: the real ResourceSyncAttempt type (used for both LatestSync and LatestSuccessfulSync) requires Events/InitialRevision/StartedAt/Status/Target/TargetRevision -- InitialRevision/Target/TargetRevision were entirely missing from the response struct (confirmed via aws-sdk-go-v2/service/codeconnections@v1.13.4's own deserializers.go awsAwsjson10_deserializeDocumentResourceSyncAttempt switch, which has explicit cases for all six). Also, the previously-deferred optional DesiredState/LatestSuccessfulSync top-level members are now populated: this backend does not simulate real git-repo content, so InitialRevision/TargetRevision/DesiredState are synthesized identically from the resource's SyncConfiguration (Branch/ConfigFile-as-Directory/OwnerId/ProviderType/RepositoryName), with a deterministic (not random) Sha derived via syntheticRevisionSha (sha1 of stable identity fields, hex-encoded -- SHA is unconstrained beyond min:1/max:255 on the wire, real git shas are simulated shape only). LatestSuccessfulSync is populated identically to LatestSync since every synthesized attempt is immediately SUCCEEDED (no partial/failed-attempt history is modeled)."}
  GetSyncBlockerSummary: {wire: ok, errors: ok, state: ok, persist: ok, note: "checked 2026-08-19: SyncBlocker's other required members (Id/Type/Status/CreatedReason/CreatedAt, epoch-seconds) and optional ResolvedAt/ResolvedReason all match the real type/deserializer. GAP DISCLOSED, not fixed (Layer 3 / never-emitted, out of this pass's scope): real types.SyncBlocker also has an optional Contexts []SyncBlockerContext member (types/types.go:352) that this response never emits at all -- SyncBlockerContext is never modeled in this service."}
  UpdateSyncBlocker: {wire: ok, errors: ok, state: ok, persist: ok, note: "checked 2026-08-19: same Contexts gap as GetSyncBlockerSummary above (shared SyncBlocker type), disclosed not fixed."}
  ListRepositorySyncDefinitions: {wire: ok, errors: ok, state: ok, persist: ok, note: "RESOLVED this pass (was 'deferred: pagination'): confirmed via aws-sdk-go-v2/service/codeconnections@v1.13.4's ListRepositorySyncDefinitionsInput struct AND botocore's paginators-1.json (empty pagination config for this op) that the real INPUT has NO NextToken/MaxResults member at all, even though the real OUTPUT has an optional NextToken -- a real client has no way to ever request a further page for this specific operation. Added a NextToken field to the output wire shape for completeness (real member); it always stays nil/omitted since every definition is returned in one response, which is the only behavior a real client could ever observe anyway."}
  UpdateRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok, note: "response no longer carries an invented Tags field (prior pass). FIXED 2026-09-04 (gopherstack-2y2.1): ConnectionArn was written through with zero validation. UpdateRepositoryLinkInput.ConnectionArn's own doc comment states the updated ARN must share the original connection's providerType; the backend now looks up the new connection (region-scoped) and returns ResourceNotFoundException (ErrNotFound) if it does not exist, InvalidInputException (ErrValidation) on a ProviderType mismatch -- both confirmed present in UpdateRepositoryLink's own error switch. Same bug found (not fixed, out of scope) in services/codestarconnections' twin -- gopherstack-5k45."}
families:
  RouteMatcher: {status: ok, note: "unchanged from 2026-07-13 pass; X-Amz-Target prefix and Content-Type verified byte-for-byte, no bug."}
  ConnectionStatus: {status: ok, note: "unchanged; CreateConnection sets AVAILABLE immediately, defensible emulation choice, do not 'fix' to PENDING."}
  HostStatus: {status: ok, note: "unchanged; CreateHost sets AVAILABLE immediately, defensible emulation choice."}
  SyncBlockerPersistence: {status: ok, note: "unchanged store.Table structure from 2026-07-13 pass; this pass's DeleteSyncConfiguration cascade-cleanup (see ops above) reuses the existing syncBlockersByResource index, no schema change, no snapshot version bump needed."}
  ListPaginationTotalOrder: {status: ok, note: "FIXED 2026-09-04 (gopherstack-2y2.2): ListHosts/ListConnections sorted purely on the non-unique Name/ConnectionName field (both explicitly allow duplicates -- see CreateHost/CreateConnection notes above). Reasoned defensive fix, not an independently observed nondeterminism (this service's List* backends read from an insertion-ordered store.Index, not a re-ranged native map, so two back-to-back calls with no intervening Put/Delete already returned identical order) -- matches the precedent PR #2442 applied to its own one insertion-ordered case. Added HostArn/ConnectionArn as secondary sort keys."}
  ErrValidationWireType: {status: ok, note: "FIXED this pass: ErrValidation's wire type was 'ValidationException', a gopherstack-INVENTED error code -- aws-sdk-go-v2/service/codeconnections@v1.13.4's types/errors.go has NO ValidationException type at all in its full modeled exception set (17 types: AccessDeniedException/ConcurrentModificationException/ConditionalCheckFailedException/ConflictException/InternalServerException/InvalidInputException/LimitExceededException/ResourceAlreadyExistsException/ResourceNotFoundException/ResourceUnavailableException/RetryLatestCommitFailedException/SyncBlockerDoesNotExistException/SyncConfigurationStillExistsException/ThrottlingException/UnsupportedOperationException/UnsupportedProviderTypeException/UpdateOutOfSyncException). Renamed to InvalidInputException, confirmed as the real type for malformed/missing-required-field input by cross-referencing every mutating op's real error list in botocore's codeconnections/2023-12-01/service-2.json (all list InvalidInputException for input validation; none list ValidationException). This affected every required-field check across every handler in this service (Handler.resolveErrorType's single switch case), not just one op."}
gaps: ["SyncBlocker.Contexts (types.SyncBlockerContext) is never emitted by GetSyncBlockerSummary/UpdateSyncBlocker -- disclosed 2026-08-19, out of scope for that pass (Layer-3/never-emitted-member hunt), not fixed.", "CreateRepositoryLink's ConnectionArn and CreateSyncConfiguration's RepositoryLinkId are never checked for existence -- disclosed 2026-09-04, deliberately NOT fixed: unlike UpdateRepositoryLink (gopherstack-2y2.1), neither CreateRepositoryLink's nor CreateSyncConfiguration's own error deserializer switch (awsAwsjson10_deserializeOpErrorCreateRepositoryLink / ...CreateSyncConfiguration) contains ResourceNotFoundException at all, so there is no real error type this service's error set offers for 'referenced resource does not exist' on these two ops -- adding one would itself be the exact wire-shape bug this campaign exists to catch. Cannot be determined from the SDK whether real AWS validates these fields at all (and if so, via which mechanism); declining to guess."]
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; all state lives in store.Table/Index behind lockmetrics.RWMutex, snapshotted via persistence.go. FIXED this pass: DeleteSyncConfiguration previously left orphaned syncBlockers rows behind forever (see GetResourceSyncStatus/DeleteSyncConfiguration ops notes) -- a real ghost-row leak, now cleaned up via a cascade delete keyed off the existing syncBlockersByResource index. No new goroutines or tables were introduced; the fix reuses existing indexes."}
---

## Notes

- **Protocol**: `application/x-amz-json-1.0` (awsjson1.0), single POST endpoint,
  `X-Amz-Target: CodeConnections_20231201.<Op>` -- unchanged from the 2026-07-13
  pass, re-verified against serializers.go, no bug.

- **Epoch-seconds timestamps**: unchanged from the 2026-07-13 pass (already
  fixed then via `pkgs/awstime.Epoch`); re-verified this pass while field-diffing
  `GetResourceSyncStatus`'s full response shape.

- **`ErrValidation` wire type was `ValidationException`, a gopherstack-INVENTED
  error code** -- the real SDK's `types/errors.go` has no such type in its full
  17-member modeled exception set. Renamed to `InvalidInputException`, the real
  type for input validation, confirmed against every mutating op's real error
  list in botocore's `codeconnections/2023-12-01/service-2.json`. This is a
  single fix in `Handler.resolveErrorType`'s switch (`handler.go`) plus the
  `ErrValidation` sentinel's message string (`errors.go`), but it affects the
  wire error type returned by every required-field check across every handler
  in this service.

- **`DeleteHost`'s in-use rejection used `ConflictException`**, a type not in
  DeleteHost's real, complete error list (`[ResourceNotFoundException,
  ResourceUnavailableException]` per botocore). Renamed to
  `ResourceUnavailableException` -- the real type, and the same one covering
  the doc-noted sibling case ("host cannot be deleted while
  VPC_CONFIG_INITIALIZING/VPC_CONFIG_DELETING").

- **`GetResourceSyncStatus`'s `ResourceSyncAttempt` response items were
  missing `InitialRevision`/`Target`/`TargetRevision`** entirely -- all three
  are required members of the real `ResourceSyncAttempt` type (confirmed via
  this service's own `deserializers.go`). Additionally, the previously-deferred
  optional `DesiredState`/`LatestSuccessfulSync` top-level output members were
  never populated. Both are now real: synthesized from the resource's
  `SyncConfiguration` (this backend does not simulate actual git-repo commit
  history), with a deterministic `Sha` derived via a stable hash of the
  configuration's identity fields (`syntheticRevisionSha`,
  `repository_sync.go`) rather than a fabricated/random one.

- **`RepositoryLinkInfo` response items carried an invented `Tags` field** --
  the real `RepositoryLinkInfo` wire type has no `Tags` member at all (tags
  for a repository link are retrievable only via `ListTagsForResource`). A
  previous audit pass found this but left it in place "to avoid risk breaking
  existing tests"; per this pass's mandate to delete gopherstack-invented
  fields not in the real SDK, it has been removed from
  `CreateRepositoryLink`/`GetRepositoryLink`/`UpdateRepositoryLink`/
  `ListRepositoryLinks` response items, and the one test that asserted on it
  (`TestRepositoryLinkTagsInListItem`) was rewritten
  (`TestRepositoryLinkNoTagsFieldInListItem`) to assert the field's absence
  and that the tags remain real state via `ListTagsForResource`.

- **`PullRequestComment`** (present in this service's pinned SDK
  `types.SyncConfiguration`/`CreateSyncConfigurationInput`/
  `UpdateSyncConfigurationInput` but previously entirely unimplemented) is now
  a real field: accepted on create/update, stored on `SyncConfiguration`,
  returned by Get/List, and round-tripped through the `syncConfigurations`
  DTO persistence table (no snapshot version bump needed -- old snapshots
  without the field simply decode it as `""`, matching the existing
  `PublishDeploymentStatus`/`TriggerResourceUpdateOn` precedent on the same
  struct).

- **`DeleteSyncConfiguration` leaked `syncBlockers` rows** (LEAKS finding):
  deletion only ever removed the `syncConfigurations` entry, never the
  `syncBlockers` rows indexed under the same resource+syncType key. Because
  `GetSyncBlockerSummary` requires the sync configuration to still exist, the
  orphaned blockers were invisible through the API immediately -- but they
  were not actually gone: recreating a sync configuration for the exact same
  `ResourceName`+`SyncType` would make the old, already-resolved blockers
  silently reappear via `GetSyncBlockerSummary`. Fixed with a cascade delete
  in `DeleteSyncConfiguration` (`sync_configurations.go`) reusing the existing
  `syncBlockersByResource` index; locked by
  `TestDeleteSyncConfiguration_CleansUpSyncBlockers`.

- **`CreateConnection` accepted a `HostArn` referencing a nonexistent host
  with zero validation.** Fixed: `CreateConnection`'s real, complete error
  list (`[LimitExceededException, ResourceNotFoundException,
  ResourceUnavailableException]` per botocore) confirms
  `ResourceNotFoundException` is the correct type for a missing host, the
  same type `GetHost`/`DeleteHost` already use. A previous audit pass left
  this gap open citing inability to confirm without live AWS access; the
  botocore error list resolves that uncertainty.

- **Ambiguity RESOLVED this pass**: `CreateConnection`/`CreateHost` used to
  reject a duplicate `ConnectionName`/`Name` with `ResourceAlreadyExistsException`,
  even though neither operation's real error list includes that exception. The
  operation's own deserializer (not just botocore's service-2.json) is the
  authoritative enumeration: `aws-sdk-go-v2/service/codeconnections@v1.13.4`
  `deserializers.go`'s `awsAwsjson10_deserializeOpErrorCreateConnection` switch
  recognizes only `LimitExceededException`/`ResourceNotFoundException`/
  `ResourceUnavailableException`, and `awsAwsjson10_deserializeOpErrorCreateHost`'s
  switch recognizes only `LimitExceededException` -- a real client sending
  `ResourceAlreadyExistsException` for either op falls through to the generic/
  unmodelled-error branch, meaning the real service cannot use that exception
  for either op. This is not an oversight: sibling ops in the very same
  service, `CreateRepositoryLink` and `CreateSyncConfiguration`, DO have a
  `ResourceAlreadyExistsException` case in their own deserializers, proving the
  modelers know how to add it when the constraint is enforced and chose not to
  here. Verdict: real AWS does not reject a duplicate name for these two ops
  (the "must be unique" doc text is not backed by an enforced API contract);
  gopherstack's prior behavior was MORE RESTRICTIVE than the real service,
  rejecting creates a real client would receive as 200s with distinct ARNs.
  Fixed by removing the duplicate-name check from `CreateConnection`/
  `CreateHost` (`connections.go`/`hosts.go`); the now-dead `connectionsByName`/
  `hostsByName` secondary indexes (their only reader) were removed with it
  (`store.go`/`store_setup.go`). `ErrAlreadyExists` remains in use for
  `CreateRepositoryLink`/`CreateSyncConfiguration`, where it is real.

- **`PublishDeploymentStatus`/`TriggerResourceUpdateOn`/`PullRequestComment`
  had zero enum validation** on `CreateSyncConfiguration`/
  `UpdateSyncConfiguration` -- any string, valid or not, was silently accepted
  and echoed back verbatim. The real enums (`types/enums.go`) are
  `PublishDeploymentStatus`/`PullRequestComment` = `{ENABLED, DISABLED}` and
  `TriggerResourceUpdateOn` = `{ANY_CHANGE, FILE_CHANGE}`. None of the three
  are required input members, so an omitted/empty value is still accepted (and
  for `UpdateSyncConfiguration`, empty string remains the existing
  "leave unchanged" sentinel). Fixed with `validEnabledDisabled`/
  `validTriggerResourceUpdateOn` in `models.go`, matching the
  `validProviderTypes`/`validSyncTypes` pattern already used elsewhere in this
  service.

- 2026-08-21 (gopherstack-r80d batch 23, required-OUTPUT-member cut): read all
  14 ops-with-required (15 required fields total, tied with
  codestarconnections/awsconfig as the largest remaining candidates after
  sagemaker) end to end against `aws-sdk-go-v2/service/codeconnections@
  v1.13.4`'s `api_op_*.go`, plus every nested domain struct one level deeper
  (`RepositoryLinkInfo`, `SyncConfiguration`, `SyncBlockerSummary`/
  `SyncBlocker`, `RepositorySyncDefinition`, `ResourceSyncAttempt`/
  `RepositorySyncAttempt`, `Revision`, `ResourceSyncEvent`/
  `RepositorySyncEvent`) against `types/types.go` directly -- this service's
  `GetResourceSyncStatus`/`GetRepositorySyncStatus` are the "one wrapper key"
  shape (`LatestSync` wraps a whole `ResourceSyncAttempt`/
  `RepositorySyncAttempt`), so the flat op-level count undercounts
  substantially; `handler_repository_sync.go`'s own doc comments record that a
  prior pass already closed this exact gap
  (`InitialRevision`/`Target`/`TargetRevision` "were previously missing
  entirely from this response shape"), confirmed still correctly wired this
  pass. 0 new bugs. One tagged-`omitempty`-on-a-required-member reviewed and
  ruled out: `repositorySyncDefinitionItem.Parent` (wire member of
  `RepositorySyncDefinition`, required) is tagged `omitempty`, but its only
  value source is `SyncConfiguration.ResourceName`, which
  `handleCreateSyncConfiguration`/`handleUpdateSyncConfiguration` both reject
  as empty via `ErrValidation` before any `SyncConfiguration` (and therefore
  any `RepositorySyncDefinition`) is ever stored -- the real SDK's own
  client-side validator only rejects a nil `ResourceName` pointer, not an
  empty string (`validateOpCreateSyncConfigurationInput`,
  `aws-sdk-go-v2/service/codeconnections@v1.13.4/validators.go:722-748`), so
  this backend is stricter than real AWS and the empty state is genuinely
  unreachable through it -- same "stricter than real AWS, unreachable" class
  `batch` (service) named for `QuotaShareCapacityLimit.CapacityUnit`.
  services/_REQUIRED_OUTPUT_CANDIDATES.md updated.
## 2026-08-19 wrapper-key/nested-shape sweep

The prior pass's `overall: A` was awarded while three separate "generalized
from a wider sibling type" wire bugs were live and untested-for. All three
are the same bug class as `codestarconnections`' prior GetHost fix
(`gopherstack-2mwl`-style): a narrow response type's field set was inferred
from a wider sibling type instead of read from its own deserializer.

- **`GetHost`'s response fabricated `HostArn`/`StatusMessage`/`Tags`, and
  omitted the real `VpcConfiguration` member entirely.** The real
  `GetHostOutput` (`api_op_GetHost.go`) and its live deserializer
  (`awsAwsjson10_deserializeOpDocumentGetHostOutput` -- confirmed live via
  `grep -c` returning 2, not dead code) both have exactly
  `Name`/`ProviderEndpoint`/`ProviderType`/`Status`/`VpcConfiguration`.
  `HostArn`/`StatusMessage` belong to the wider `Host` type (used by
  `ListHosts`); `Tags` belongs to `CreateHostOutput`. Fixed in
  `handler_hosts.go`'s `getHostOutput`/`handleGetHost`. `VpcConfiguration`
  is now backed by a real `models.go` type and a `hosts.go`
  `CreateHost`/`UpdateHost` parameter (previously the backend had no VPC
  concept at all), so the fix is observable end-to-end through the real SDK
  client, not just a permanently-nil field.

- **`ListHosts`' `hostItem` fabricated a `Tags` field.** The real `Host`
  type (`types.Host`) has `HostArn`/`Name`/`ProviderEndpoint`/`ProviderType`/
  `Status`/`StatusMessage`/`VpcConfiguration` but **no `Tags` member at
  all** (confirmed against `awsAwsjson10_deserializeDocumentHost`'s case
  switch) -- tags for a host are only ever retrievable via
  `ListTagsForResource`. Fixed alongside `GetHost`'s `VpcConfiguration` fix
  (both views share `Host.VpcConfiguration`).

- **`GetConnection`/`ListConnections`' shared `connectionItem` fabricated a
  `Tags` field.** The real `Connection` type (`types.Connection`) has
  `ConnectionArn`/`ConnectionName`/`ConnectionStatus`/`HostArn`/
  `OwnerAccountId`/`ProviderType` but **no `Tags` member at all** (confirmed
  against `awsAwsjson10_deserializeDocumentConnection`'s case switch) --
  `Tags` is only ever returned by `CreateConnectionOutput`. Fixed in
  `handler_connections.go`.

Every fix above was proven by hand-revert: the fabricated/omitted field was
reintroduced, the corresponding new/corrected test was run and failed with
exactly the predicted symptom (fabricated key present, or `VpcConfiguration`
nil), then the file was restored and diffed byte-identical (`md5sum`) before
re-running green.

**Existing tests that asserted the fabricated fields as correct** (all
rewritten this pass, not just noted): `TestGetHostIncludesHostArn` (renamed
`TestGetHostExcludesHostArn`, premise inverted), `TestHostGet`'s "returns
host fields" subtest (dropped the `HostArn` assertion),
`TestCreateHostWithTags` (now verifies tags via `ListTagsForResource`
instead of `GetHost`), `TestListHostsIncludesTags` (renamed
`TestListHostsExcludesTags`), `TestGetConnectionIncludesTags` (renamed
`TestGetConnectionExcludesTags`). New: `TestListConnectionsExcludesTags`,
`TestGetHost_VpcConfigurationRoundTrip` (real `aws-sdk-go-v2` client,
`hosts_vpc_roundtrip_test.go`), `TestGetHost_RawShapeExcludesFabricatedFields`.

**`last_audit_commit` provenance check**: the manifest's previous value,
`749ff939`, was checked via `git show --stat 749ff939` and found to touch
only `services/serverlessrepo/` -- never `services/codeconnections/`. That
citation was fabricated by a prior pass (the fourth such fabricated
manifest citation found across this campaign). Corrected to the actual HEAD
at the start of this pass, `b451ad0d6`.

**Disclosed, not fixed** (out of this pass's scope per the Layer-3
never-emitted-member exclusion): real `types.SyncBlocker` has an optional
`Contexts []SyncBlockerContext` member (`types/types.go:352`) that
`GetSyncBlockerSummary`/`UpdateSyncBlocker` never emit; `SyncBlockerContext`
is not modeled anywhere in this service.

**Divergence from `services/codestarconnections/`** (its already-fixed
twin, read-only reference this pass): none found beyond the bugs above,
which `codestarconnections` had already fixed in a prior pass and
`codeconnections` had not. Both services' `GetHost`/`ListHosts`/
`GetConnection`/`ListConnections` shapes now match after this pass's fixes.
`codestarconnections` additionally validates `Name`/`ConnectionName`
non-empty inside `handleCreateHost`/`handleCreateConnection` themselves
(belt-and-suspenders on top of the backend's own validation);
`codeconnections` relies on the backend validation alone for those two
fields. This is a defensive-depth style difference, not a wire-parity bug
(the backend validation already returns the correct error), so it was not
changed.

### 2026-08-31 (gopherstack-uox6, value-semantics-of-a-correctly-read-field pass)

`covledger -service codeconnections` reported no rows for every class. Same
axis and same fields as `services/codestarconnections/PARITY.md`'s
same-dated entry -- read here first, then re-derived independently from
this service's OWN pinned SDK
(`aws-sdk-go-v2/service/codeconnections@v1.13.4`) rather than assumed from
its twin, per this campaign's standing rule that a correct neighbour is
still not evidence:

- `ListConnections.ProviderTypeFilter`/`.HostArnFilter`: plain equality
  (`connections.go:114,118`), matches doc, IDENTICAL to
  `codestarconnections`. `TestListConnectionsProviderTypeFilter` already
  covers this with 3 seeded connections across 2 provider types and a
  zero-match case (`connections_validation_test.go:592-628`) -- adequate,
  not a single-record test that could hide a wrong algorithm.
- `ListRepositorySyncDefinitions.SyncType`/`ListSyncConfigurations.SyncType`:
  equality-compared (`repository_sync.go:121`, `sync_configurations.go:163`),
  IDENTICAL logic to `codestarconnections`.
- `ListHosts`/`ListRepositoryLinks`: no filter fields, pagination-only;
  `handleListRepositoryLinks`'s inline pointer-unwrap-then-`page.New` here
  (`handler_repository_links.go:141-151`) is a decode-style difference from
  `codestarconnections`' direct-value-type call, not a behavior difference
  -- both hit `page.New`'s `limit <= 0 -> defaultLimit` fallback identically
  when `MaxResults` is absent.

Same negative conclusion as the twin: no `MaxResults` doc states a specific
default number anywhere in this service, no operator grammar/wildcard/
negation/case-sensitivity language exists. Zero bugs found. No files
changed.

### 2026-08-31 Error-envelope sweep 2 (gopherstack-uox6, errtargetaudit, post-reachability-fix)

`errtargetaudit -dir codeconnections` reported 4 class-A findings, all
`InvalidInputException` reaching an operation whose own
`awsAwsjson10_deserializeOpError<Op>` switch does not declare it
(`aws-sdk-go-v2/service/codeconnections@v1.13.4` deserializers.go, plain
JSON-body-driven switch, not the older `EqualFold` cascade shape --
confirmed by reading all four ops' functions directly).

**2 real, fixed by deletion:**

- `GetHost` (`handler_hosts.go:101`, was): declares `ResourceNotFoundException`,
  `ResourceUnavailableException` -- no `InvalidInputException`. The
  `HostArn is required` pre-check fired on an empty-but-present ARN (the
  client-side validator only rejects a nil pointer, per
  `validateOpGetHostInput`); the backend's own lookup (`b.hosts.Get("")`,
  not found) already answers with the correct `ResourceNotFoundException`
  once the pre-check is removed. Same shape as `codestarconnections`' 8
  deletions (9cf2d2292).
- `UpdateHost` (`handler_hosts.go:202`, was): same reasoning, same fix.
  Declares `ConflictException`, `ResourceNotFoundException`,
  `ResourceUnavailableException`, `UnsupportedOperationException`.

**2 refusals** (operation's own model declares no type for this condition;
`errors.go`'s own comment already confirms no `ValidationException`
equivalent exists anywhere in this SDK module):

- `CreateConnection` (`connections.go:21,25`): declares
  `LimitExceededException`, `ResourceNotFoundException`,
  `ResourceUnavailableException` -- no validation-shaped exception for a
  missing `ConnectionName` or invalid `ProviderType`. Not fixed.
- `CreateHost` (`hosts.go:23,27,31`): declares only
  `LimitExceededException`. Same reasoning. Not fixed.

`ErrValidation` (the shared `InvalidInputException` sentinel) is otherwise
correct for its ~35 other call sites across
`handler_repository_sync.go`/`handler_sync_configurations.go`/
`handler_repository_links.go`/`sync_configurations.go` -- not touched.

New SDK-driven test (`error_envelope_fixes_test.go`,
`TestEmptyHostArn_NotFoundNotInvalidInput_RealClient`, 2 subtests, `errors.As`
against `*types.ResourceNotFoundException`) confirmed failing pre-fix (both
subtests got `*smithy.GenericAPIError` wrapping `InvalidInputException`).
One existing test corrected: `connections_test.go`'s `TestErrorPaths/"GetHost
missing HostArn"` asserted `wantErrType: "InvalidInputException"` (a
wire-code-string assertion, not a typed-error one) -- now
`"ResourceNotFoundException"`, assertion count unchanged.

Gates: `go build ./services/codeconnections/...`, `go vet ./...`
(repo-wide, clean -- no exported signature changed), `go test -race
-count=1 ./services/codeconnections/...` (pass), `golangci-lint run
./services/codeconnections/...` (0 issues, no `nolint` in any edited file).

### 2026-09-04 (gopherstack-2y2, 5-dimension audit: behavior/LocalStack/cross-service/perf/leaks)

**1. AWS behavior compliance.** Re-derived independently against
`aws-sdk-go-v2/service/codeconnections@v1.13.4` rather than trusting this
manifest's prior "A" grade. Two real bugs found and fixed, both above
(`UpdateRepositoryLink` ConnectionArn validation, gopherstack-2y2.1; List*
pagination total order, gopherstack-2y2.2). One referential-integrity
question examined and deliberately left unfixed (see `gaps` above:
`CreateRepositoryLink`/`CreateSyncConfiguration` FK existence checks --
neither op's error switch has `ResourceNotFoundException`, so there is no
real error type available to signal it). `ConnectionStatus`/`HostStatus`
hardcoded to `AVAILABLE` re-examined and reconfirmed as the prior passes'
deliberate, documented choice (real `PENDING` requires simulating a console
OAuth hand-off this backend does not model) -- not re-litigated.

**2. LocalStack parity** (behavior differences a client would observe):
NOT CHECKED -- no LocalStack instance available to this pass; the
pagination-order fix above is the one behavior-observable difference found
by static reasoning alone.

**3. Cross-service integration**: CLEAN, confirmed by grep, not assumed.
No other service imports `services/codeconnections` or calls into its
backend. `services/cloudformation` only references
`GetCodeConnectionsHandler()`/`GetCodeStarConnectionsHandler()` on its
provider interface for registration wiring -- no CloudFormation resource
type actually manages a CodeConnections connection/host/repository-link/sync
configuration, so there is no CFN teardown path touching this service's
state. `services/iam/actions.go` only maps the `CodeConnections_20231201`
target prefix to the service name for IAM action-name resolution. `go test
-race -count=1 ./services/cloudformation/...` passes unchanged.
`services/codestarconnections` (this service's independent twin, confirmed
via grep to share no import) has the identical `UpdateRepositoryLink` bug --
filed separately as gopherstack-5k45, not fixed here (out of scope for this
service's issue).

**4. Performance**: CLEAN. All List* operations read from a `store.Index`
scoped by region (O(records in region), not O(all records)); the two
referential-check helpers (`connectionHasReferenceToHostLocked`,
`syncConfigHasReferenceToLinkLocked`) are the same pattern. No hot path
bypasses an index that exists for it -- `ListRepositorySyncDefinitions`
scans all of a region's sync configurations rather than an
index-by-RepositoryLinkID, but no such index exists anywhere in this
service (`store_setup.go` has only `byRegion`/`byResource`), so this is
consistent with every other List op here, not a deviant bypass.

**5. Resource leaks**: CLEAN, confirmed by grep (`go func`, `time.Tick`,
`time.NewTicker`, `time.AfterFunc`, `context.WithCancel` all zero matches
outside tests). No goroutines, tickers, or unbounded caches; all state lives
in `store.Table`/`Index` behind the one `lockmetrics.RWMutex`, snapshotted
via `persistence.go`. Matches the prior pass's leaks note (already fixed
then: `DeleteSyncConfiguration`'s `syncBlockers` cascade).

Gates: `GOTOOLCHAIN=go1.26.6 go build ./services/codeconnections/...`,
`GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/codeconnections/...`
(pass), `GOTOOLCHAIN=go1.26.6 golangci-lint run
./services/codeconnections/...` (0 issues), `GOTOOLCHAIN=go1.26.6 go test
-race -count=1 ./services/cloudformation/...` and
`./services/codestarconnections/...` (both pass, unmodified).
