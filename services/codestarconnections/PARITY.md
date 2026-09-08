---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: codestarconnections
sdk_module: aws-sdk-go-v2/service/codestarconnections@v1.38.4   # unchanged this pass; still the go.mod pin
last_audit_commit: e0dfee4c0                       # HEAD at the start of this pass (gopherstack-42j/5k45); this pass's changes are uncommitted working-tree changes on top
last_audit_date: 2026-09-04
# 2026-08-19 re-audit (gopherstack-6flj wrapper-key/nested-shape sweep): all 27 ops
# re-verified field-by-field against the LIVE awsjson1.0 deserializers in
# codestarconnections@v1.38.4. Zero bugs, no code changed. last_audit_commit above is
# NOT stale-by-fabrication -- 1d7169f66 is dated the same day as last_audit_date and the
# schema defines this field as "HEAD when this manifest was written", not "a commit
# touching this service". It IS unreachable from main (it lives only on the unmerged
# chore/parity-upgrade branch), which breaks this schema's own re-audit protocol -- see
# gopherstack-z31a. Also note the gaps: prose still cites SDK v1.35.15 while the header
# is correctly v1.38.4; PullRequestComment re-confirmed still absent from v1.38.4.
overall: A            # zero-gap field-diff pass against botocore's authoritative per-op error lists
                      # (2026-07-24). This pass (gopherstack-7mmd): re-examined whether
                      # GetResourceSyncStatus's Git-SHA-bearing fields and SyncBlocker.Contexts
                      # are buildable with a scoped simulation, or genuinely structural. Concluded
                      # structural (no code change) -- see structural_gaps and Notes below for the
                      # reasoning. Spot-audit of the rest of the surface (stub/no-op patterns,
                      # live-client construction) turned up nothing new.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "Tags field in response verified correct (CreateConnectionOutput.Tags is real). NEW this pass: HostArn is now validated against a real, previously-created host -- unknown HostArn returns ResourceNotFoundException (real error list is [LimitExceededException, ResourceNotFoundException, ResourceUnavailableException], confirmed via botocore's codestar-connections/2019-12-01/service-2.json operations[].errors; ResourceNotFoundException chosen to match the identical fix already made in the codeconnections sibling service, both citing GetHost/DeleteHost's reuse of ResourceNotFoundException for missing hosts). Tag-count-exceeded now maps to LimitExceededException (was InvalidInputException, which is not in this op's real error list at all)."}
  GetConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  ListConnections: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateHost: {wire: ok, errors: ok, state: ok, persist: ok, note: "Tags field in response verified correct. Host name length validation fixed: was reusing ConnectionName's 32-char max (validateConnectionName) for host names too; real HostName shape (botocore) has its own 64-char max, and a separate character-class restriction that never existed in the real API (pattern is `.*`, effectively unrestricted) was also removed -- see validateHostName in errors.go. Tag-count-exceeded now maps to LimitExceededException (CreateHost's real, complete error list is [LimitExceededException] only -- it never had InvalidInputException as a possible error)."}
  GetHost: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: response no longer includes an invented StatusMessage field. Confirmed via aws-sdk-go-v2's generated GetHostOutput struct and its deserializer (awsAwsjson10_deserializeOpDocumentGetHostOutput) that the real GetHost response is exactly Name/ProviderEndpoint/ProviderType/Status/VpcConfiguration -- StatusMessage is a genuine real-API asymmetry (present on types.Host, used by ListHosts, but NOT on GetHostOutput specifically), not a gopherstack omission. listHostView (ListHosts' per-item shape) still includes it correctly."}
  ListHosts: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteHost: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: in-use error type was ConflictException (itself a real type, but the wrong one for this op); DeleteHost's real, complete error list (botocore) is exactly [ResourceNotFoundException, ResourceUnavailableException] -- ConflictException belongs to UpdateHost's error list instead. Now returns ResourceUnavailableException, matching the identical fix already made in the codeconnections sibling service."}
  UpdateHost: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass: now also resolves repository link ARNs (see CreateRepositoryLink note), not just connection/host ARNs."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "Tag-count-exceeded now maps to LimitExceededException (was InvalidInputException; TagResource's real, complete error list is [ResourceNotFoundException, LimitExceededException] -- it never had InvalidInputException as a possible error). Also now resolves repository link ARNs."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "Also now resolves repository link ARNs."}
  CreateRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok, note: "duplicate-link error type (ResourceAlreadyExistsException) verified correct. NEW this pass: real CreateRepositoryLinkInput has a `Tags []types.Tag` member (confirmed against aws-sdk-go-v2's generated api_op_CreateRepositoryLink.go) that gopherstack previously dropped entirely -- repository links are now taggable via TagResource/UntagResource/ListTagsForResource by RepositoryLinkArn, same as connections/hosts. RepositoryLinkInfo (the Get/List/Create/Update response shape) correctly still has no Tags member of its own -- tags are only visible via ListTagsForResource, never echoed back in-line."}
  GetRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok, note: "in-use error type (SyncConfigurationStillExistsException) verified correct."}
  ListRepositoryLinks: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "duplicate-config error type (ResourceAlreadyExistsException) verified correct."}
  GetSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSyncConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSyncConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRepositorySyncStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "epoch-seconds StartedAt/Events[].Time verified correct."}
  GetResourceSyncStatus: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED this pass: LatestSync.Target (a required real member of types.ResourceSyncAttempt, confirmed against aws-sdk-go-v2's awsAwsjson10_deserializeDocumentResourceSyncAttempt) is now populated with the request's ResourceName -- it was previously omitted entirely. DesiredState/LatestSuccessfulSync (top-level optional output members) and InitialRevision/TargetRevision (types.Revision fields nested in LatestSync/LatestSuccessfulSync) remain unpopulated -- see gaps."}
  GetSyncBlockerSummary: {wire: ok, errors: ok, state: ok, persist: ok, note: "epoch-seconds CreatedAt/ResolvedAt verified correct."}
  UpdateSyncBlocker: {wire: ok, errors: ok, state: ok, persist: ok, note: "'SyncBlocker' (singular) response key verified correct. NEW this pass: ResourceName/SyncType/ResolvedReason are now enforced as required input (all four members -- Id, ResolvedReason, ResourceName, SyncType -- are 'This member is required' per aws-sdk-go-v2's generated api_op_UpdateSyncBlocker.go); previously only Id was validated."}
  ListRepositorySyncDefinitions: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRepositoryLink: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-04 (gopherstack-5k45): ConnectionArn was written through with zero validation. UpdateRepositoryLinkInput.ConnectionArn's own doc comment states the updated ARN must share the original connection's providerType; the backend now looks up the new connection (region-scoped via regionFromARN) and returns ResourceNotFoundException (ErrNotFound) if it does not exist, InvalidInputException (ErrValidation) on a ProviderType mismatch -- both confirmed present in this SDK module's own UpdateRepositoryLink error switch (awsAwsjson10_deserializeOpErrorUpdateRepositoryLink). Same bug and fix already applied to the codeconnections twin (commit 619f7c06a)."}
families:
  RouteMatcher: {status: ok, note: "X-Amz-Target prefix 'CodeStar_connections_20191201.' and Content-Type 'application/x-amz-json-1.0' both verified byte-for-byte against aws-sdk-go-v2's serializers.go SetHeader calls for every op."}
  ConnectionStatus: {status: ok, note: "CreateConnection sets AVAILABLE immediately (not AWS's real PENDING-until-console-handshake); this is the correct emulated behavior for a service with no state-transition API -- do not 'fix' this to PENDING."}
  HostStatus: {status: ok, note: "CreateHost sets PENDING and stays there (no auto-transition); consistent with AWS's real HOST behavior (host setup genuinely requires console/agent installation)."}
  ErrorTaxonomy: {status: ok, note: "Every error sentinel field-diffed this pass against botocore's codestar-connections/2019-12-01/service-2.json operations[].errors (the authoritative per-op error list, cross-checked against aws-sdk-go-v2/service/codestarconnections/types/errors.go's exhaustive 17-type catalog). Two invented-type bugs fixed: ErrValidation was 'ValidationException' (does not exist in this service's real error catalog at all) -> now InvalidInputException; DeleteHost's dependency-check error was 'ConflictException' (a real type, but not in DeleteHost's real error list -- it belongs to UpdateHost's) -> now ResourceUnavailableException. Both fixes mirror decisions already made and evidence-documented in the codeconnections sibling service's own error-taxonomy audit. New ErrTagLimitExceeded sentinel (LimitExceededException) replaces the previous ErrValidation/InvalidInputException mapping for 'too many tags' on CreateConnection/CreateHost/TagResource, none of which document InvalidInputException as a possible error for that case."}
  Tagging: {status: ok, note: "Connections, hosts, AND repository links are all real taggable resources (CreateRepositoryLinkInput has a genuine Tags member -- see CreateRepositoryLink note above). Sync configurations are NOT taggable (CreateSyncConfigurationInput has no Tags member at all in the real SDK) -- verified, not touched."}
  ListPaginationTotalOrder: {status: ok, note: "FIXED 2026-09-04 (gopherstack-42j): ListHosts/ListConnections sorted purely on the non-unique Name/ConnectionName field (both explicitly allow duplicates -- CreateHost/CreateConnection model no ResourceAlreadyExistsException for a name collision). sort.Slice is not stable, so two rows sharing a name had no defined total order -- enough for a cursor-paginated caller to skip or repeat one. Added HostArn/ConnectionArn (always unique) as secondary sort keys, matching the identical fix already made in the codeconnections sibling service (commit 619f7c06a)."}
gaps:
  - PullRequestComment field (CreateSyncConfiguration/UpdateSyncConfiguration/SyncConfiguration) — present in current AWS API docs but NOT in the pinned aws-sdk-go-v2@v1.35.15 SDK's types/serializers/deserializers; correctly omitted to match the SDK version actually vendored by this repo (not a gap in the usual sense — flagged here only so a future SDK bump re-checks it)
  - "CreateRepositoryLink's ConnectionArn and CreateSyncConfiguration's RepositoryLinkId are never checked for existence -- disclosed 2026-09-04 (gopherstack-42j), deliberately NOT fixed: unlike UpdateRepositoryLink (gopherstack-5k45), neither CreateRepositoryLink's nor CreateSyncConfiguration's own error deserializer switch (awsAwsjson10_deserializeOpErrorCreateRepositoryLink / ...CreateSyncConfiguration) contains ResourceNotFoundException; both DO contain InvalidInputException, but this service's own ErrValidation doc comment (errors.go) establishes InvalidInputException as the real type for malformed/missing-required-field input specifically, not FK-style reference validation -- using it here would be exactly the kind of invented-purpose wire-shape bug this campaign exists to catch, not a fix. Cannot be determined from the SDK whether real AWS validates these fields at all (and if so, via which mechanism); declining to guess. Same conclusion independently reached for the codeconnections twin."
structural_gaps:
  - "GetResourceSyncStatus does not populate optional DesiredState/LatestSuccessfulSync (types.Revision-bearing) fields, nor InitialRevision/TargetRevision within LatestSync/LatestSuccessfulSync. types.Revision.Sha is a required member (a 40-hex-char git commit SHA) with no real backing state in this emulator: RECLASSIFIED this pass (gopherstack-7mmd) from gaps to structural_gaps after re-examining whether a scoped simulation is buildable. It is not, for two independent reasons specific to this service (not merely 'large'): (1) codestarconnections has zero concept of repository content anywhere in its data model -- RepositoryLink stores only an ARN to an external provider connection (GitHub/Bitbucket/etc.), never any file tree, commit graph, or branch HEAD; there is no internal state a SHA could be honestly derived FROM. (2) There is no customer-facing operation in this service that would ever cause AWS to observe/generate a new commit SHA in the first place -- sync statuses are populated exclusively via the SetRepositorySyncStatus/SetResourceSyncStatus test/internal helpers (sync_statuses.go), not any routed op, so there is no real 'moment' analogous to a real sync attempt to hang a SHA off of. Fabricating a placeholder SHA would present as if the emulator had observed a specific real commit from an external git host it has never connected to -- exactly the kind of fabricated data parity-principles.md's no-stub rule prohibits. The current behavior (omit the whole optional Revision-bearing wrapper) is wire-correct, not a stub. (bd: gopherstack-7mmd)"
  - "SyncBlocker.Contexts ([]types.SyncBlockerContext) is never populated. Real AWS creates sync blockers, with contexts describing what specifically went wrong, as a side effect of internal Git-sync/CloudFormation validation actually running against real repository content and a real CloudFormation stack. This emulator has neither: no git content (see the Revision.Sha structural gap above) and no CloudFormation integration. gopherstack's only blocker-creation path is the CreateSyncBlocker test/internal helper (not a routed customer-facing op), which has no realistic source of context data to attach -- there is no internal event this backend could honestly summarize into a Contexts entry. Contexts is an optional member, so omitting it remains wire-correct. RECLASSIFIED this pass from gaps to structural_gaps for the same reason as the Revision.Sha entry above: the missing data's source (real CFN validation against real git content) cannot exist in this emulator, not merely a feature this pass ran out of room for. (bd: gopherstack-7mmd)"
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; all state lives in store.Table/Index behind lockmetrics.RWMutex, snapshotted via persistence.go. NEW this pass: repositoryLinksByArn secondary index added (store.Table auto-maintains it through Put/Delete/Restore, same as every other index in this service) to let tags.go resolve repository link ARNs without a ghost/duplicate row risk -- DeleteRepositoryLink still removes the whole RepositoryLink (including its embedded Tags map) in one Delete call, so no separate tag store exists to leak."}
---

## Notes

### 2026-08-07 pass (gopherstack-7mmd): is simulated git state in scope?

The assigned follow-up asked whether "sync-status revisions need simulated git SHA" is
buildable this pass, or whether a documented subset is the honest answer. Re-read the full
data model (`store.go`, `sync_configurations.go`, `sync_statuses.go`) end to end before
concluding: **no code change**, reclassified both affected gap entries to `structural_gaps`
(see that section for the full reasoning). Summary of why this differs from a "merely large"
gap that would stay in `gaps`:

- This service's entire data model has no representation of repository *content* at any
  layer -- `RepositoryLink` is an ARN pointing at an external provider connection, never a
  file tree/commit graph/branch pointer. A commit SHA has nothing internal to be derived
  from, unlike (for comparison) a case this same audit pass found genuinely buildable in
  `codecommit` -- gopherstack owns and generates that service's commit graph itself, so
  fixing gaps there was a data-model *rework*, not a data-*source* problem.
  `codestarconnections` has no such internal source to rework in the first place.
- There is no routed, customer-facing operation that ever causes a sync attempt to occur in
  this backend -- `SetRepositorySyncStatus`/`SetResourceSyncStatus` are test/internal
  helpers only. Real AWS's SHA comes from actually contacting GitHub/Bitbucket during a real
  sync; simulating that would mean either integrating with a real external git host (out of
  an emulator's reach entirely) or inventing one, which is exactly the fabricated-data
  pattern this repo's conventions prohibit.

Considered and rejected: a deterministic-but-fake SHA (e.g. hashed from the resource name +
a revision counter) would at least be internally consistent and non-random, but it would
still assert to a real AWS SDK client that a specific git commit exists and was observed --
untrue in every case, since no such commit was ever seen by anything. That crosses from
"simplification" into "fabrication," so it was not implemented.

- Protocol is `application/x-amz-json-1.0` (awsjson1.0), single POST endpoint,
  `X-Amz-Target: CodeStar_connections_20191201.<Op>` — verified against every
  op's `SetHeader("X-Amz-Target")` call in aws-sdk-go-v2's generated
  serializers.go. No long `com.amazonaws.codestar.connections.` prefix exists
  in the real wire protocol for this service; do not "fix" the route matcher
  to add one.

- **Epoch-seconds timestamps** (bug class from parity-principles.md #2): every
  timestamp in this service's sync/blocker surface was already fixed to
  epoch-seconds JSON numbers (via `pkgs/awstime.Epoch`) in a prior audit and
  was re-verified, not re-fixed, this pass.

- **Error-taxonomy field-diff (this pass's main finding)**: this audit had
  direct access to botocore's `codestar-connections/2019-12-01/service-2.json`
  (the authoritative source for real per-op documented error lists), not just
  the Go SDK's exhaustive-but-undifferentiated type catalog. Two invented-type
  bugs were found and fixed by cross-referencing every mutating op's
  `operations[].errors` array:
  - `ErrValidation` was mapped to a fabricated `"ValidationException"` type
    that does not exist anywhere in this service's real 17-type error catalog
    (`AccessDeniedException`/`ConcurrentModificationException`/
    `ConditionalCheckFailedException`/`ConflictException`/
    `InternalServerException`/`InvalidInputException`/`LimitExceededException`/
    `ResourceAlreadyExistsException`/`ResourceNotFoundException`/
    `ResourceUnavailableException`/`RetryLatestCommitFailedException`/
    `SyncBlockerDoesNotExistException`/`SyncConfigurationStillExistsException`/
    `ThrottlingException`/`UnsupportedOperationException`/
    `UnsupportedProviderTypeException`/`UpdateOutOfSyncException`). Every
    mutating op's real error list uses `InvalidInputException` for malformed
    input instead; `ErrValidation` now maps there.
  - `DeleteHost`'s "host has active connections" error was `ConflictException`
    — itself a real type in the catalog, but not a possible error for
    `DeleteHost` specifically: botocore's `DeleteHost.errors` is exactly
    `[ResourceNotFoundException, ResourceUnavailableException]`.
    `ConflictException` belongs to `UpdateHost`'s error list instead. Now
    `ResourceUnavailableException`.
  - Both fixes exactly mirror decisions already made (with the same
    botocore-backed evidence) in the `codeconnections` sibling service's own
    error-taxonomy audit, confirming this is a shared bug class introduced by
    the same historical sweep across both CodeStar/CodeConnections services.
  - `LimitExceededException` was also found to be systematically
    under-used: `CreateConnection`/`CreateHost`/`TagResource` all document it
    as their (sole, for `CreateHost`) real error for "too many tags", but
    gopherstack mapped that case to `ErrValidation`/`InvalidInputException`
    instead. A new `ErrTagLimitExceeded` sentinel now carries the correct
    type across all three call sites (`validateTags`'s count check and
    `TagResource`'s post-merge count check).

- **CreateConnection HostArn existence validation** (previously an open gap):
  `CreateConnection`'s real error list is `[LimitExceededException,
  ResourceNotFoundException, ResourceUnavailableException]` — a `HostArn`
  referencing a host that does not exist now returns
  `ResourceNotFoundException`, the same real type `GetHost`/`DeleteHost` use
  for a missing host. This exactly mirrors the fix already made in the
  `codeconnections` sibling service (see its `connections.go` comment), which
  resolved the same ambiguity this service's previous audit had left open
  citing lack of live-AWS confirmation.

- **HostName length/pattern bugs**: `CreateHost` was reusing
  `ConnectionName`'s 32-character max (via a shared `validateConnectionName`
  helper) for host names too. botocore's `HostName` shape has its own,
  different 64-character max — a valid 33-64 character host name was
  previously rejected. Additionally, both `ConnectionName` (pattern
  `[\s\S]*`) and `HostName` (pattern `.*`) are, per botocore, unrestricted in
  character class (any string up to the length limit); a previous
  implementation invented a `[a-zA-Z0-9_.\-]+` regex that rejected valid
  names containing spaces or other punctuation. Both bugs fixed via a new
  `validateHostName` (64-char max) alongside `validateConnectionName`
  (32-char max), neither doing character-class filtering anymore.

- **GetHost's missing `StatusMessage`**: aws-sdk-go-v2's generated
  `GetHostOutput` struct and its deserializer
  (`awsAwsjson10_deserializeOpDocumentGetHostOutput`) confirm the real
  `GetHost` response is exactly `Name`/`ProviderEndpoint`/`ProviderType`/
  `Status`/`VpcConfiguration` — no `StatusMessage`, even though the sibling
  `types.Host` (used by `ListHosts`) does have that field. This is a genuine
  real-API asymmetry, not a gopherstack gap; a previous implementation had
  added `StatusMessage` to `GetHost`'s response by incorrectly generalizing
  from `types.Host`. Removed from `getHostView`; `listHostView` (`ListHosts`'
  per-item shape) is unaffected and still correctly includes it.

- **`CreateRepositoryLink` `Tags` field**: aws-sdk-go-v2's generated
  `CreateRepositoryLinkInput` struct has a real `Tags []types.Tag` member
  that gopherstack previously dropped entirely — repository links could not
  be tagged at creation, and were not resolvable at all by
  `TagResource`/`UntagResource`/`ListTagsForResource` (only connections and
  hosts were). Fixed: `RepositoryLink` gained a `Tags map[string]string`
  field (persisted automatically, since it's a plain exported field on a
  type whose `Snapshot`/`Restore` DTO already carries the whole struct — see
  `persistence.go`'s `regionalDTO[RepositoryLink]`), a new
  `repositoryLinksByArn` secondary index lets `tags.go` resolve a repository
  link by its `RepositoryLinkArn` (mirroring how connections/hosts are
  resolved by their own ARN), and `CreateRepositoryLink`'s handler now
  accepts and forwards a `Tags` array. `RepositoryLinkInfo` (the
  Get/List/Create/Update response shape) correctly still has no `Tags`
  member of its own — verified against the real SDK — so created tags are
  only visible via a follow-up `ListTagsForResource` call, never echoed
  in-line the way `CreateConnectionOutput`/`CreateHostOutput` do.

- **`GetResourceSyncStatus` missing `Target`**: `types.ResourceSyncAttempt`'s
  `Target` member is required (always populated by real AWS) per its
  deserializer (`awsAwsjson10_deserializeDocumentResourceSyncAttempt`) and
  is simply the resource name being synchronized — always known here since
  it equals the request's `ResourceName`. Previously omitted entirely; now
  populated. `DesiredState`/`LatestSuccessfulSync`/`InitialRevision`/
  `TargetRevision` remain unpopulated (see gaps) since they require a
  simulated Git commit SHA this emulator has no real backing state for.

- **`UpdateSyncBlocker` required-field validation**: all four
  `UpdateSyncBlockerInput` members (`Id`, `ResolvedReason`, `ResourceName`,
  `SyncType`) are "This member is required" per the real SDK, but only `Id`
  was previously validated. `ResourceName`/`SyncType`/`ResolvedReason` are
  now also enforced.

- **`ListRepositorySyncDefinitions`** (fixed in a prior audit, unchanged
  this pass): derives real definitions from the repository link's
  `SyncConfiguration`s rather than being a disguised no-op.

- **`UpdateSyncBlocker` wire shape** (fixed in a prior audit, unchanged this
  pass): response key is the real singular `SyncBlocker`, not a fabricated
  `SyncBlockerSummary` list.

- 2026-08-21 (gopherstack-r80d batch 23, required-OUTPUT-member cut): read all
  14 ops-with-required (15 required fields total, tied with
  codeconnections/awsconfig as the largest remaining candidates after
  sagemaker) end to end against `aws-sdk-go-v2/service/codestarconnections@
  v1.38.4`'s `api_op_*.go`, plus every nested domain struct one level deeper
  (`RepositoryLinkInfo`, `SyncConfiguration`, `SyncBlockerSummary`/
  `SyncBlocker`, `RepositorySyncDefinition`) against `types/types.go`
  directly. 0 new bugs. The one real gap in this territory --
  `GetResourceSyncStatus`'s `LatestSync.InitialRevision`/`.TargetRevision`
  (both required members of `types.ResourceSyncAttempt`) never populated --
  is already fully disclosed, reclassified from `gaps` to `structural_gaps`
  by a very recent prior pass (`gopherstack-7mmd`, see `structural_gaps`
  above) with a well-reasoned no-fabrication justification (this service has
  no git-content data model to honestly derive a commit SHA from); re-read
  `handler_sync_statuses.go`'s own doc comment and confirmed it still matches
  current behavior, not re-flagged as new. Same tagged-`omitempty`-on-a-
  required-member reviewed and ruled out as codeconnections found:
  `repositorySyncDefinitionItem.Parent` is unreachable-empty because
  `handleCreateSyncConfiguration` rejects an empty `ResourceName` (its only
  value source) via `errInvalidRequest` before storage, stricter than the
  real SDK's nil-only client-side check. services/_REQUIRED_OUTPUT_CANDIDATES.md
  updated.

### 2026-08-31 (gopherstack-uox6, value-semantics-of-a-correctly-read-field pass)

`covledger -service codestarconnections` reported no rows for every class.
This axis checks whether a correctly-read filter is applied with the RIGHT
algorithm, distinct from the wire-shape entries above. Read every
List/Describe filter field against `aws-sdk-go-v2/service/
codestarconnections@v1.38.4`'s own doc comments and, since this service and
`codeconnections` are the same API renamed, against its twin's
implementation of the identical field:

- `ListConnections.ProviderTypeFilter`/`.HostArnFilter`: plain equality,
  matches "Filters the list of connections to those associated with...".
  `connections.go:111,115`. IDENTICAL to `codeconnections`' implementation
  (`connections.go:114,118` there) -- confirmed consistent twin, not just
  assumed from the shared history.
- `ListRepositorySyncDefinitions.SyncType`/`ListSyncConfigurations.SyncType`:
  both required fields, equality-compared defensively
  (`sync_configurations.go:192,229`); IDENTICAL logic in `codeconnections`.
  `RepositoryLinkId` is equality-matched first in both.
- `ListHosts`/`ListRepositoryLinks`: no filter fields on the real input at
  all (SDK confirmed), pagination-only.

No `MaxResults` doc comment on any List op in this service states a
specific default number, so the narrowing/widening-default sub-shape has no
surface. No operator grammar, wildcard, negation, or case-sensitivity
language exists anywhere in this service's pinned SDK. Zero bugs found; see
`services/codeconnections/PARITY.md`'s same-dated entry for the twin
verdict. No files changed here (test strengthening landed in
`services/mediapackage/` instead, unrelated to this service).

## 2026-08-31 Error-envelope sweep (gopherstack-6flj/uox6, errtargetaudit)

`errtargetaudit -dir codestarconnections` reported 10 class-A findings, all
`code=InvalidInputException mechanism=sentinel reference`. This service's
error matching is `awsAwsjson10_deserializeOpError<Op>` (a per-op switch in
deserializers.go, same shape family as `awsRestjson1`/`awsAwsjson11`, just
protocol variant 1.0). Verified every finding against its op's own switch
(codestarconnections@v1.38.4 deserializers.go). 8 of 10 were real and fixed;
2 were real but had no correct declared type and are recorded, not fixed;
a manual trace beyond the tool's own findings found one more of the second
kind. False-positive rate: 0% (10/10 were real reachability bugs; the
question was fix-vs-refuse, not real-vs-imagined).

**Root cause (8 fixes, all one shape)**: `errInvalidRequest`
("X is required") pre-checks in the Handler layer for `DeleteConnection`,
`DeleteHost`, `GetConnection`, `GetHost`, `ListTagsForResource`,
`TagResource`, `UntagResource`, `UpdateHost` fired only on an
empty-but-present ARN (`aws.String("")`) — a real client's own
`validateOp<Op>Input` (codestarconnections@v1.38.4 validators.go) only
rejects a **nil** pointer client-side, so an explicit empty string reaches
the server unchanged. None of these 8 ops declare `InvalidInputException`
in their own switch; all declare `ResourceNotFoundException` (some also
`ResourceUnavailableException`), which the ordinary ARN-lookup-miss path
already answers correctly. **Fix: deleted each pre-check** (deletion beats
remapping — nothing needed inventing) — `handler_connections.go`,
`handler_hosts.go`, `handler_tags.go`. New real-SDK-client test
`TestEmptyRequiredARN_NotFoundNotInvalidInput_RealClient`
(`error_envelope_fixes_test.go`, 8 subtests) asserts `errors.As` against
`*types.ResourceNotFoundException`; confirmed all 8 fail against unmodified
code (got `*smithy.GenericAPIError` for `InvalidInputException` instead).

**3 refusals, recorded not fixed** (comments added at each site; no
behavior change):

- `CreateConnection` (`connections.go`): name/ProviderType/tag validation.
  Own switch: `[LimitExceededException, ResourceNotFoundException,
  ResourceUnavailableException]` — no InvalidInputException.
- `CreateHost` (`hosts.go`): name/ProviderEndpoint/ProviderType/tag
  validation. Own switch: `[LimitExceededException]` only.
- `UpdateHost`'s ProviderEndpoint-too-long check (`hosts.go`): own switch
  `[ConflictException, ResourceNotFoundException,
  ResourceUnavailableException, UnsupportedOperationException]`.
  `TestUpdateHost_ProviderEndpointTooLong_NoDeclaredType` pins the
  pre-existing status quo so a future change is deliberate.
- **Extra, found by manual trace, not by the tool**: `TagResource`'s
  `validateTags` per-key/value checks (`tags.go`) — empty key, oversized
  key/value — also use `ErrValidation`. TagResource's own switch is
  `[LimitExceededException, ResourceNotFoundException]`. The count-limit
  check right above it in the same function correctly uses
  `ErrTagLimitExceeded` (`LimitExceededException`, declared); only the
  per-key/value branch is unresolved. `errtargetaudit`'s "sentinel
  reference" mechanism only resolved the shallow `handler_tags.go` site for
  this op, not this deeper backend-internal one — a gap worth noting for
  future passes.

All 3 (4, counting the manual-trace one) refusals share the same reason:
**the operation's own model declares no type for this condition**, and no
`ValidationException` equivalent exists anywhere in this SDK module (per
`ErrValidation`'s existing doc comment in errors.go) to substitute either.

No shared sentinel was changed — `errInvalidRequest`/`ErrValidation` still
carry their original wire codes and are still legitimately used elsewhere
in this service (sync configurations, sync blockers, repository links,
CreateConnection/CreateHost/UpdateHost's own remaining validation) — only
the specific call sites that reached those sentinels for a wrong op were
touched.

Existing test found and fixed: none newly broken by this pass (existing
`TestCreateConnection_DuplicateNameSucceedsSDKRoundTrip`/
`TestCreateHost_DuplicateNameSucceedsSDKRoundTrip`/
`TestDeleteSyncConfiguration_UnknownResourceIsIdempotentSDKRoundTrip`,
`errors_deserializer_test.go`, already document this exact bug class from
an earlier pass — gopherstack-wlo1 — and continued to pass unmodified).

Re-run after fix: `errtargetaudit -dir codestarconnections` now reports 3
class-A findings (`CreateConnection`, `CreateHost`, `UpdateHost`'s
ProviderEndpoint check) — exactly the 3 documented refusals it can see (it
cannot see the 4th, `TagResource`'s validateTags path, since its shallow
mechanism never flagged it).

Gates: `go build`, `go vet` (repo-wide, clean), `go test -race -count=1`,
`golangci-lint run` — all clean (`./services/codestarconnections/...`).

### 2026-09-04 (gopherstack-42j/5k45 parity sweep, template: codeconnections commit 619f7c06a)

Two real bugs found and fixed, both above (`UpdateRepositoryLink`
ConnectionArn validation, gopherstack-5k45; List* pagination total order,
gopherstack-42j). `TestUpdateRepositoryLink`'s "success" case had been
asserting the bug: it passed a fabricated `ConnectionArn` that was never
created and asserted a 200 with that ARN written straight through
unchanged. Rewritten to a table matching the codeconnections precedent, with
`new_connection_does_not_exist` and `new_connection_provider_type_mismatch`
cases added. `TestListHosts_OrdersTiedNamesByArn`/
`TestListConnections_OrdersTiedNamesByArn` added, seeding three rows with a
shared name via `AddHostInternal`/`AddConnectionInternal` in descending-ARN
order and asserting ascending-ARN output. All four new/changed subtests
confirmed to fail without their respective fix (hand-reverted one file at a
time, re-tested, restored).

One referential-integrity question examined and deliberately left unfixed
(see `gaps` above): `CreateRepositoryLink`/`CreateSyncConfiguration` FK
existence checks -- neither op's error switch has `ResourceNotFoundException`,
and using the `InvalidInputException` that IS present would repurpose it
beyond this service's own documented scope for that code (malformed input,
not a missing reference). `ConnectionStatus`/`HostStatus` hardcoded-`AVAILABLE`
state machine re-examined and reconfirmed as the prior passes' deliberate,
documented choice -- not re-litigated. `DeleteConnection`/`DeleteHost`/
`DeleteRepositoryLink`/`DeleteSyncConfiguration` preconditions re-verified
against this SDK module's own error switches: all four match already
(`DeleteConnection` has no dependency check because none is modeled;
`DeleteHost`/`DeleteRepositoryLink` correctly block on in-use dependents;
`DeleteSyncConfiguration` is correctly idempotent). Noted, not filed: hosts
without any HostArn-referencing connection are never route-reachable via
`SetRepositorySyncStatus`/repositorySyncStatuses cleanup on
`DeleteRepositoryLink` -- inert in practice, since that table is populated
only by a test/internal helper never reachable through any routed op (same
structural reasoning as the existing `structural_gaps` entries above).

Gates: `GOTOOLCHAIN=go1.26.6 go test -race -count=1
./services/codestarconnections/...` (pass), `GOTOOLCHAIN=go1.26.6
golangci-lint run ./services/codestarconnections/...` (0 issues), dependents
`./services/cloudformation/...` and `./services/codeconnections/...` (both
pass, unchanged).
