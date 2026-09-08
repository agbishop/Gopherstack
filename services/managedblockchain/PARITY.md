---
service: managedblockchain
sdk_module: aws-sdk-go-v2/service/managedblockchain@v1.34.4
last_audit_commit: a073b2b1
last_audit_date: 2026-09-04
overall: A
ops:
  CreateNetwork: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "FrameworkConfiguration.Fabric.Edition, VpcEndpointServiceName, Framework restricted to HYPERLEDGER_FABRIC; see Notes"}
  GetNetwork: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now returns FrameworkAttributes.Fabric + VpcEndpointServiceName"}
  ListNetworks: {wire: fixed, errors: ok, state: ok, persist: ok, note: "server-side pagination now implemented via pkgs/page; see Notes"}
  CreateMember: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "InvitationId now required and validated against a real PENDING invitation for this network, consumed (ACCEPTED) on success; MemberConfiguration.FrameworkConfiguration.Fabric.AdminUsername/AdminPassword required and validated, KmsKeyArn accepted; see Notes"}
  GetMember: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now returns FrameworkAttributes.Fabric.AdminUsername/CaEndpoint + KmsKeyArn; LogPublishingConfiguration.Fabric.CaLogs wire key fixed CloudWatch->Cloudwatch, see 2026-08-20 Notes"}
  ListMembers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "server-side pagination now implemented"}
  DeleteMember: {wire: ok, errors: ok, state: fixed, persist: ok, note: "cascades to member's nodes, matching real AWS; now also cascade-deletes the network when the removed member was its last, matching both the direct-call and approved-removal-proposal paths; see 2026-09-04 Notes"}
  UpdateMember: {wire: fixed, errors: ok, state: ok, persist: ok, note: "LogPublishingConfiguration.Fabric.CaLogs.Cloudwatch request/response wire key fixed, see 2026-08-20 Notes"}
  CreateNode: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "NodeConfiguration.StateDB accepted (defaults CouchDB), KmsKeyArn inherited from owning member; see Notes"}
  GetNode: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now returns FrameworkAttributes.Fabric.PeerEndpoint/PeerEventEndpoint + StateDB + KmsKeyArn; LogPublishingConfiguration.Fabric.{ChaincodeLogs,PeerLogs} wire key fixed CloudWatch->Cloudwatch, see 2026-08-20 Notes"}
  ListNodes: {wire: fixed, errors: ok, state: ok, persist: ok, note: "server-side pagination now implemented"}
  DeleteNode: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateNode: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "MemberId moved from a required query parameter to the required JSON body field a real client actually sends; LogPublishingConfiguration.Fabric.{ChaincodeLogs,PeerLogs}.Cloudwatch wire key fixed; see 2026-08-20 Notes"}
  CreateProposal: {wire: ok, errors: ok, state: ok, persist: ok}
  GetProposal: {wire: ok, errors: ok, state: fixed, persist: ok, note: "now resolves a lapsed IN_PROGRESS proposal to EXPIRED on read; see 2026-09-04 Notes"}
  ListProposals: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "server-side pagination now implemented; fabricated ProposalSummary.NetworkId member removed, see 2026-08-20 Notes; now resolves EXPIRED the same way GetProposal does, see 2026-09-04 Notes"}
  VoteOnProposal: {wire: ok, errors: ok, state: fixed, persist: ok, note: "tallies votes and resolves APPROVED/REJECTED against VotingPolicy; not a disguised no-op; now also resolves EXPIRED on a lapsed proposal and rejects votes on it, see 2026-09-04 Notes; executeProposalActionsLocked now fails a RemoveAction whose target member already left independently, setting ACTION_FAILED instead of silently succeeding as APPROVED, see 2026-09-08 Notes"}
  ListProposalVotes: {wire: fixed, errors: ok, state: ok, persist: ok, note: "server-side pagination now implemented"}
  ListInvitations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "server-side pagination now implemented; fabricated Invitation.NetworkId/NetworkName top-level members removed, see 2026-08-20 Notes"}
  RejectInvitation: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAccessor: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccessor: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAccessor: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAccessors: {wire: fixed, errors: ok, state: ok, persist: ok, note: "server-side pagination now implemented"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  network: {status: fixed, note: "CreateNetwork/GetNetwork/ListNetworks field-diffed against types.go/api_op_*.go/validators.go; FrameworkAttributes+VpcEndpointServiceName+Framework restriction added, see Notes"}
  member: {status: fixed, note: "MemberConfiguration.FrameworkConfiguration was entirely unmodeled (a real, required field per validateMemberFabricConfiguration) -- now implemented with real server-side validation + FrameworkAttributes/KmsKeyArn on responses, see Notes; distinct wire structs confirmed for Member vs MemberSummary, each matching its own live deserializer case list, see 2026-08-20 Notes"}
  node: {status: fixed, note: "StateDB/KmsKeyArn/FrameworkAttributes were entirely unmodeled -- now implemented; the prior audit's node-routing-URI fix remains correct and unchanged; UpdateNode's MemberId location bug and the CloudWatch/Cloudwatch key bug fixed this pass, see 2026-08-20 Notes"}
  proposal: {status: fixed, note: "CreateProposal/GetProposal/ListProposals/ListProposalVotes/VoteOnProposal verified; vote tallying and threshold-based APPROVED/REJECTED transition confirmed real (not a stub); ListProposals/ListProposalVotes now paginate; fabricated ProposalSummary.NetworkId removed this pass, see 2026-08-20 Notes; EXPIRED status (real AWS's 5-value ProposalStatus enum, types/enums.go v1.34.4) was entirely unmodeled -- now implemented, see 2026-09-04 Notes; ACTION_FAILED (the enum's 5th value) was also unmodeled and wrongly assessed as structurally unreachable -- it is reachable via a client-callable path (a RemoveAction target that self-departs before the proposal executes) and is now implemented, see 2026-09-08 Notes"}
  invitation: {status: fixed, note: "ListInvitations/RejectInvitation only -- correctly no CreateInvitation op (real AWS has none either; invitations are created only as a side effect of an approved proposal's Invitations actions, which executeProposalActionsLocked implements); ListInvitations now paginates; fabricated top-level NetworkId/NetworkName removed this pass, see 2026-08-20 Notes"}
  accessor: {status: ok, note: "CreateAccessor/GetAccessor/DeleteAccessor/ListAccessors verified; ListAccessors now paginates; Accessor vs AccessorSummary wire structs confirmed distinct and each matches its own live deserializer, see 2026-08-20 Notes"}
  tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource verified against /tags/{ResourceArn} shape and ARN-keyed lookup"}
gaps:
  - "Member.IsOwned is always true, even for a member created via CreateMember (i.e. joining via invitation, which in real AWS is not owned by the joining account's original network-owner relationship). gopherstack has no multi-account model to distinguish an owned member from an invited one, so this is a reasonable simplification, not flagged as a bug to fix (gopherstack-u84u re-reviewed this alongside InvitationId; InvitationId itself is now real, see Notes #8)."
  - "No artificial service quotas (max members per network, max nodes per member, max networks per account) are enforced, so ResourceLimitExceededException is never returned. Consistent with this emulator's general no-limits style elsewhere; not treated as a bug."
  - "Network.FrameworkAttributes.Ethereum and Node.FrameworkAttributes.Ethereum are not modeled. gopherstack-u84u answered the design question this was deferred under: real AWS's CreateNode documents exactly one well-known public Ethereum NetworkId, \"n-ethereum-mainnet\" (aws-sdk-go-v2 managedblockchain api_op_CreateNode.go:44-47 and api_op_DeleteNode.go:36, v1.34.4 -- confirmed NOT invented; older SDKs additionally listed now-sunset n-ethereum-goerli/n-ethereum-rinkeby testnets, absent from this pin), with FrameworkAttributes.Ethereum.ChainId documented as \"1\" for mainnet (types/types.go:538-547's NetworkEthereumAttributes). ListNetworks/GetNetwork both self-document \"Applies to Hyperledger Fabric and Ethereum\", so real AWS does surface this network through both once an account has a node on it. Seeding the network itself would therefore be honest (a real, stable constant, not invented). Still deferred: CreateNode's real MemberId is documented \"Applies only to Hyperledger Fabric\" (api_op_CreateNode.go:56-58) -- Ethereum nodes have no owning member -- but gopherstack's Node storage is keyed by (networkID, memberID, nodeID) (nodeKey in store_setup.go) and CreateNode already requires MemberId unconditionally (ErrMissingNodeMemberID) for its one supported framework. Making CreateNode against Ethereum reachable needs a memberless Node storage path, not just a seeded network row -- a real structural change, not an adjacent fix."
  - "ResourceNotReadyException (\"The requested resource exists but isn't in a status that can complete the operation\", aws-sdk-go-v2 managedblockchain types/errors.go:198-199, v1.34.4) is modeled by 8 ops' deserializers -- CreateMember/CreateNode/CreateProposal/DeleteMember/DeleteNode plus ListTagsForResource/TagResource/UntagResource (re-confirmed against botocore data/managedblockchain/2018-09-24/service-2.json.gz, same 8-op set) -- but gopherstack has no sentinel or return path for it anywhere. Real AWS's plausible trigger is an op against a network/member/node still in a transient CREATING/DELETING/UPDATING status, but gopherstack has no async lifecycle at all: every write site for NetworkStatus/MemberStatus/NodeStatus (networks.go:104,250; members.go:129,241; nodes.go:113,226) sets AVAILABLE synchronously and no code path ever produces CREATING/DELETING/UPDATING/FAILED; DeleteMember/DeleteNode remove the resource from the store in the same call (members.go:217; nodes.go:190,206) rather than transiting through a DELETING status first. Re-confirmed genuinely unreachable (gopherstack-rcp6, 2026-09-08 audit): making this reachable needs a real async lifecycle simulation, not a one-line guard; not fabricated, not flagged as a bug to fix."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; InMemoryBackend.mu is the single coarse lockmetrics.RWMutex guarding every map/store.Table, consistent with pkgs-catalog.md's locking rule. The new paginate() helper (pagination.go) and buildNetworkFrameworkAttributes/buildMemberFrameworkAttributes/CreateNode's FrameworkAttributes synthesis are all pure functions operating on already-locked state or post-lock snapshots -- no new lock paths introduced. 2026-09-04: expireProposalLocked/deleteNetworkIfEmptyLocked (both pure functions taking an already-locked *Proposal/*Network) and promoting GetProposal/ListProposals from RLock to Lock (they can now mutate a proposal's Status in place) introduce no new lock paths either."}
---

## 2026-09-04 (lifecycle-state precondition sweep)

**Real bug #1, fixed: proposal EXPIRED status was entirely unmodeled.** Real
AWS's `ProposalStatus` enum has 5 values (`aws-sdk-go-v2/service/
managedblockchain/types/enums.go:222-226`, v1.34.4): `IN_PROGRESS`,
`APPROVED`, `REJECTED`, `EXPIRED`, `ACTION_FAILED`. gopherstack only ever set
3 of them. The AWS Managed Blockchain Hyperledger Fabric dev guide ("View
Proposals") documents `EXPIRED`: "Members did not cast the number of votes
required to determine the proposal outcome before the proposal expired. The
specified `ProposalActions` are not carried out." gopherstack stored and
round-tripped `Proposal.ExpirationDate` but never enforced it: a proposal
that ran out its 24-hour voting window with an inconclusive vote stayed
`IN_PROGRESS` forever, and `VoteOnProposal` would still accept votes on it
indefinitely. Fixed: `expireProposalLocked` (`proposals.go`) transitions an
`IN_PROGRESS` proposal to `EXPIRED` once `time.Now()` passes
`ExpirationDate`; called from `GetProposal`, `ListProposals` (both promoted
from `RLock` to `Lock` since they can now mutate proposal state), and
`VoteOnProposal` (before its in-progress check, so a vote on a lapsed
proposal is correctly rejected as `InvalidRequestException` rather than
silently accepted). New test: `TestHandler_ProposalExpiresAfterExpirationDate`
(`proposals_voting_test.go`), 3 subtests covering `GetProposal`,
`ListProposals`, and `VoteOnProposal`; confirmed all 3 fail against the
unmodified code (GetProposal/ListProposals report `IN_PROGRESS` instead of
`EXPIRED`, VoteOnProposal returns 204 instead of 400). New export_test.go
helper `SetProposalExpiration` (test-only, matches the file's existing
`*Count`/`ARNIndexSize` pattern) backdates a proposal's `ExpirationDate`
without a real sleep.

**Real bug #2, fixed: DeleteMember never cascaded to the network on its last
member.** `api_op_DeleteMember.go`'s doc comment (v1.34.4): "If MemberId is
the last member in a network specified by the last Amazon Web Services
account, the network is deleted also." Real AWS has no `DeleteNetwork`
operation at all (confirmed: `ls aws-sdk-go-v2/service/managedblockchain@*/
api_op_*.go` lists no such file) -- this DeleteMember side effect is the
*only* way a network is ever deleted, so without it gopherstack's networks
are immortal. Two independent code paths remove a member -- `DeleteMember`
(`members.go`) and `executeProposalActionsLocked`'s removal-action cascade
(`proposals.go`, run when an approved proposal's `Actions.Removals` fires) --
and neither checked whether the network was now empty. Fixed: new shared
helper `deleteNetworkIfEmptyLocked` (`networks.go`) called from both. New
tests: `TestInMemoryBackend_DeleteMemberNetworkCascade` (`members_test.go`,
2 subtests: last member deletes the network, non-last member does not) and
`TestHandler_ApprovedRemovalProposalCascadeDeletesEmptyNetwork`
(`proposals_voting_test.go`, exercises the proposal-approval path
specifically). Confirmed failing against the unmodified code: neutering
`deleteNetworkIfEmptyLocked` to a no-op fails the "last member" subtest
(`GetNetwork` returns no error instead of `ErrNetworkNotFound`) while the
"non-last member" subtest still correctly passes, and fails the HTTP-level
removal-proposal test (`GET /networks/{id}` returns 200 instead of 404).
One pre-existing test, `TestInMemoryBackend_DeleteMemberCascadeARNIndex`,
asserted the old (buggy) behavior directly -- a 1-member network's ARN index
had 1 entry ("only network remains") after deleting that member's only
member -- and was corrected to assert 0 (network is gone too), with a
comment explaining why.

**Considered, not fixed (see `gaps`):** `ACTION_FAILED` (no failure path
exists in `executeProposalActionsLocked` for approved actions to hit) and
`ResourceNotReadyException` (no async CREATING/DELETING/UPDATING lifecycle
exists anywhere in this emulator for any resource; every Create op sets
`AVAILABLE` synchronously). Both are structural gaps consistent with this
emulator's existing no-injected-failure, no-async-lifecycle style elsewhere,
not fabricated and not one-line fixes.

## 2026-08-30 (request-field axis sweep, gopherstack-4shm's class)

`cmd/reqfieldscan` flagged `ClientRequestToken` on all 5 create ops
(`CreateNetwork`/`CreateMember`/`CreateNode`/`CreateProposal`/`CreateAccessor`)
as declared-but-never-read. This service does not use `service.JSONOpFunc`/
`service.WrapOp` at all (it's REST-routed through `dispatch`/
`dispatchNetworkOps`/etc., all literal `json.Unmarshal` decodes), so the
scan's coverage guard is silent here by construction (see the tool's own
`packageMentionsJSONOpFunc` gate) -- not a blind spot, confirmed by reading
that condition rather than inferring from the guard's silence.

**Real bug, fixed:** all 5 ops' Go SDK struct doc comments mark
`ClientRequestToken` "This member is required", and `validators.go` (v1.34.4)
enforces it client-side for every one (`validateOpCreateNetworkInput`,
`...CreateMemberInput`, `...CreateNodeInput`, `...CreateProposalInput`,
`...CreateAccessorInput`, all calling `smithy.NewErrParamRequired`). A real
`aws-sdk-go-v2` client never omits it -- the SDK's idempotency-token
middleware (`idempotencyToken_initializeOp<Op>`) auto-fills it when unset --
but gopherstack accepted a raw HTTP request missing it outright, certifying a
call the real service rejects. Fixed: each of the 5 handlers now returns
`InvalidRequestException` (`ErrMissingClientRequestToken`, `errors.go`) when
the field is empty, checked immediately after JSON decode. `~50` pre-existing
tests across `accessors_test.go`, `framework_attributes_test.go`,
`members_test.go`, `networks_test.go`, `nodes_test.go`, `pagination_test.go`,
`proposals_test.go`, `proposals_voting_test.go`, `store_test.go`,
`tags_test.go` built request bodies with no `ClientRequestToken` at all (the
field being silently ignored meant nothing ever caught it) and were updated
to include one; none had an assertion weakened -- one,
`TestHandler_CreateAccessor`'s "empty body still creates accessor" case, was
corrected from asserting 200/`AccessorId` (matching the bug) to asserting 400
(matching real AWS), since an empty body genuinely has no
`ClientRequestToken`. New test: `client_request_token_test.go`
(`TestHandler_CreateOps_MissingClientRequestToken`), confirmed failing
(200/200/200/200/404 instead of 400) against unmodified code before the fix
landed.

**Not implemented (layer-boundary, reported not fixed):** real AWS's
documented purpose for this token is retry-safety -- "allows failed
Create<X> requests to be retried without the risk of running the operation
twice" -- implying idempotency-token *deduplication* (a retried call with the
same token should return the original result, not create a second resource).
gopherstack does not implement that: only presence is now validated, not the
value. This repo has an established pattern for exactly this
(`services/acm`'s `idempotencyMap`/`certIdempotencyEntry`,
`services/acmpca`'s `lookupIdempotentCert`/`idempotentResourceARN`), but
replicating it here means a new per-resource-type dedup store (network/
member/node/proposal/accessor, 5 call sites) plus persistence wiring -- a
real, boundable feature, but its own pass, not a one-line field-read fix.
Left undone; not fabricated, not silently dropped -- recorded here per
gopherstack-4shm's restraint principle.

## Notes

**Framework/protocol**: restjson1. Base path family is `/networks`, plus `/tags/{ResourceArn}`,
`/accessors[/{AccessorId}]`, `/invitations[/{InvitationId}]`.

**This pass's real fixes** (field-diffed against `aws-sdk-go-v2/service/managedblockchain@v1.31.19`'s
`types/types.go`, `api_op_*.go`, and `validators.go`):

1. **`MemberConfiguration.FrameworkConfiguration` was entirely unmodeled.** The real API's
   `validateMemberConfiguration` client-side validator requires it on *both* `CreateNetwork`'s
   nested `MemberConfiguration` and `CreateMember`'s top-level one, and `validateMemberFabricConfiguration`
   requires `Fabric.AdminUsername`/`Fabric.AdminPassword` whenever `FrameworkConfiguration.Fabric` is
   supplied. gopherstack previously accepted `CreateMember`/`CreateNetwork` requests missing this
   field entirely -- a raw HTTP client bypassing SDK-side validation sailed straight through with a
   member that had no Fabric identity at all. `validateMemberConfigurationRequest` in
   `handler_networks.go` now mirrors these validators server-side (stricter than the real API in one
   respect: gopherstack requires `Fabric` specifically, not just a non-nil `FrameworkConfiguration`,
   since gopherstack only emulates Hyperledger Fabric -- same rationale as `ErrMissingNodeMemberID`).
   `AdminPassword`'s real documented 8-32 character length constraint is also enforced
   (`ErrInvalidMemberAdminPassword`). New errors: `ErrMissingMemberFrameworkConfig`,
   `ErrMissingMemberFabricConfig`, `ErrMissingMemberAdminUsername`, `ErrMissingMemberAdminPassword`,
   `ErrInvalidMemberAdminPassword`.

2. **`Member.FrameworkAttributes` / `Node.FrameworkAttributes` / `Network.FrameworkAttributes` were
   entirely unmodeled** -- the prior audit pass explicitly deferred this as "a bigger design question."
   They are now implemented for real: `Member.FrameworkAttributes.Fabric.AdminUsername` (echoed from
   the request) and `.CaEndpoint` (synthesized, since gopherstack has no real Fabric CA --
   `memberCaEndpoint` in `members.go`); `Node.FrameworkAttributes.Fabric.PeerEndpoint`/
   `.PeerEventEndpoint` (synthesized -- `nodePeerEndpoint`/`nodePeerEventEndpoint` in `nodes.go`);
   `Network.FrameworkAttributes.Fabric.Edition` (echoed from `FrameworkConfiguration.Fabric.Edition`
   when the caller supplies it -- gopherstack does *not* invent an edition the caller never asked
   for) and `.OrderingServiceEndpoint` (synthesized -- `fabricOrderingServiceEndpoint` in
   `networks.go`). Only Fabric is modeled on all three, matching gopherstack's Fabric-only scope --
   see the Ethereum gap above.

3. **`Member.KmsKeyArn` / `Node.KmsKeyArn` were entirely unmodeled.** Real AWS documents the sentinel
   string `"AWS Owned KMS Key"` as the default when the caller supplies no customer managed key, and
   documents that a node "inherits this parameter from the member that it belongs to." Both are now
   implemented: `resolveMemberKmsKeyArn` in `members.go` applies the default/passthrough at
   `CreateMember`/`CreateNetwork` time, and `CreateNode` in `nodes.go` copies its owning member's
   current `KmsKeyArn` (looked up under the same lock that already validates the member exists).

4. **`NodeConfiguration.StateDB` / `Node.StateDB` were entirely unmodeled.** Real AWS defaults to
   `CouchDB` for Hyperledger Fabric 1.4+ (gopherstack's only emulated version --
   `defaultFrameworkVersion`); `resolveStateDB` in `nodes.go` now applies that default or the
   caller's explicit `LevelDB`/`CouchDB` choice.

5. **`Network.VpcEndpointServiceName` was entirely unmodeled.** Real AWS assigns every `AVAILABLE`
   network a VPC PrivateLink endpoint service name regardless of framework configuration; now
   synthesized unconditionally at network-creation time (`networkVPCEndpointServiceName`).

6. **`CreateNetwork` accepted any `Framework` value, including `ETHEREUM`.** The real API's
   `CreateNetwork` doc comment states "Applies only to Hyperledger Fabric" -- new networks can no
   longer be created on any other framework. gopherstack now rejects a non-empty, non-
   `HYPERLEDGER_FABRIC` `Framework` at `CreateNetwork` with `InvalidRequestException`
   (`ErrUnsupportedNetworkFramework`), while leaving `Framework=ETHEREUM` valid everywhere else it's
   used (e.g. `Accessor.NetworkType`'s `ETHEREUM_MAINNET`/`ETHEREUM_GOERLI`, unrelated to this enum).

7. **No server-side pagination.** Every `List*` op (`ListNetworks`/`ListMembers`/`ListNodes`/
   `ListProposals`/`ListProposalVotes`/`ListAccessors`/`ListInvitations`) previously accepted
   `maxResults`/`nextToken` but always returned every matching item in one page. Now implemented via
   the shared `paginate()` helper in `pagination.go`, which wraps `pkgs/page.New` (the same
   convention `services/acmpca` already established) -- confirmed the real query parameter names
   (`maxResults`/`nextToken`, both lowercase) directly against `serializers.go`'s
   `SetQuery("maxResults")`/`SetQuery("nextToken")` bindings, identical across all seven ops.
   `defaultListPageSize` (100) matches `services/acmpca`'s `defaultMaxItems` convention since real
   AWS does not document a specific default for this service.

8. **`CreateMember` parsed `InvitationId` off the request body and never read it again**
   (gopherstack-u84u). Real AWS's client-side validator marks it required
   (`validateOpCreateMemberInput`, `validators.go:805-806`, v1.34.4) and never sends a
   request without it; `Invitation.Status`'s doc comment (`types/enums.go:106-122`)
   documents `PENDING`→`ACCEPTED` as the one-time transition a successful `CreateMember`
   drives. gopherstack now requires it (`InvalidRequestException` if empty,
   `ErrMissingInvitationID`), looks it up (`ResourceNotFoundException` if unknown,
   reusing `ErrInvitationNotFound`), rejects one issued for a different network or not
   `PENDING` (`InvalidRequestException`, new `ErrInvitationNetworkMismatch`/
   `ErrInvitationNotPending`), and marks it `ACCEPTED` on success so it cannot be
   replayed. `Member.IsOwned` staying unconditionally `true` is unchanged and still a
   reasonable simplification (see gaps) -- gopherstack has no multi-account model to
   distinguish an owned member from an invited one, but the invitation itself is now a
   real, consumed resource rather than an ignored field.

**The prior pass's node-routing-URI fix** (nodes live at `/networks/{id}/nodes[/{id}]` with
`MemberId` carried via JSON body / `memberId` query parameter, never nested under `/members/`)
remains correct and was re-verified against `serializers.go`'s `opPath` constants during this pass;
no changes were needed there.

**Timestamps**: `*time.Time` fields marshal via Go's default `encoding/json` (RFC3339Nano), which
`smithytime.ParseDateTime` (used by every `CreationDate`/`ExpirationDate` field in the real
deserializer) parses correctly. Confirmed NOT an epoch-vs-ISO8601 bug class hit here -- this
service's JSON protocol (restjson1) uses ISO8601 date-time timestamps by default, unlike services
whose JSON members are individually marked epoch-seconds. Re-confirmed this pass for the new
surfaces added: `FrameworkAttributes`/`KmsKeyArn`/`StateDB`/`VpcEndpointServiceName` are all plain
strings, so no new timestamp fields were introduced.

**Error codes**: gopherstack's `errorResponse{Message, Code}` round-trips correctly through the
real SDK's `restjson.GetErrorInfo`, which matches `Code`/`code` and `Message`/`message` names
case-insensitively via plain `encoding/json` struct tags (confirmed by reading
`aws/protocol/restjson/decoder_util.go`). All error codes gopherstack emits
(`ResourceNotFoundException`, `ResourceAlreadyExistsException`, `InvalidRequestException`,
`InternalServiceErrorException`) match real exception types in `types/errors.go`.

**Vote tallying is real, not a disguised no-op**: confirmed by reading
`applyVoteThresholdLocked` in `backend.go` -- it computes yes-percentage against
`ApprovalThresholdPolicy.ThresholdPercentage`/`ThresholdComparator`, transitions
`IN_PROGRESS → APPROVED` when the threshold is met, and separately computes whether rejection is
mathematically guaranteed (remaining possible yes votes can't reach the requirement) to transition
`IN_PROGRESS → REJECTED`. `executeProposalActionsLocked` genuinely creates invitations and removes
members on approval. This is the "grep-based stub hunting has false positives" trap from
parity-principles.md #4 -- it would be easy to mistake this for a stub without reading the
threshold math.

## 2026-08-20 wrapper-key / nested-shape sweep

Protocol re-confirmed restjson1 (56 `awsRestjson1_*` functions in `serializers.go`; every
`HandleDeserialize` calls its `awsRestjson1_deserializeOpDocument<Op>Output` directly on the
decoded body -- none of managedblockchain's 27 ops hit the singular-output dead-code trap from a
prior appmesh audit). All 27 ops in the pinned SDK (`ls api_op_*.go`, v1.34.4) match
`GetSupportedOperations()` exactly.

The five summary/full pairs (Network/NetworkSummary, Member/MemberSummary, Node/NodeSummary,
Proposal/ProposalSummary, Accessor/AccessorSummary) all use distinct gopherstack wire structs
(`networkObject`/`networkSummaryObject`, etc.), and each was diffed field-by-field against its own
live `awsRestjson1_deserializeDocument<Type>` case list -- confirming gopherstack does NOT reuse
one wire struct across both sides of any pair (the dominant bug class this campaign, per kinesis
Consumer/ConsumerDescription, codeconnections Host/Connection). Four real bugs were found and
fixed, none of them a full/summary struct-reuse case:

1. **`LogConfigurations.Cloudwatch` wire key was `"CloudWatch"` (capital W), not real AWS's
   `"Cloudwatch"`** (`awsRestjson1_deserializeDocumentLogConfigurations`, deserializers.go:4999 --
   confirmed case-sensitive, no other case). Silently dropped by a real client on GetMember's
   `LogPublishingConfiguration.Fabric.CaLogs` and GetNode's `.Fabric.{ChaincodeLogs,PeerLogs}`.
   Fixed in `models.go` (`logConfigRespObj`/`logConfigReq`), `handler_members.go`,
   `handler_nodes.go`. Hand-revert reproduced a nil `ChaincodeLogs.Cloudwatch` through a real
   `managedblockchainsdk.Client` in the new `Test_SDKRoundTrip_NodeCloudwatchLogConfig`
   (`wire_shape_test.go`). Two existing tests (`TestHandler_UpdateMemberLogPublishingConfig`,
   `TestHandler_UpdateNodeLogPublishingConfig`) asserted the wrong response key and were corrected.

2. **`UpdateNode` required `memberId` as a query parameter; real AWS sends `MemberId` only in the
   JSON body.** Confirmed via `awsRestjson1_serializeOpHttpBindingsUpdateNodeInput`
   (serializers.go:2259, binds only NetworkId/NodeId to the URI, no `SetQuery("memberId")`) versus
   `awsRestjson1_serializeOpDocumentUpdateNodeInput` (serializers.go:2285, serializes `MemberId`
   into the body) -- unlike GetNode/ListNodes/DeleteNode, which really do bind `memberId` as a
   query parameter and were left unchanged. This meant every real SDK client's `UpdateNode` call
   was rejected outright with `InvalidRequestException`, not just a dropped field -- a request-side
   HTTP-binding bug found incidentally while diffing the node family, outside this campaign's
   strict response-wrapper-key scope but severe enough to fix. Fixed in `handler_nodes.go`
   (`handleUpdateNode` now reads `MemberId` from the decoded body) and `models.go`
   (`updateNodeRequest.MemberID` added). Hand-revert reproduced the exact real-client
   `InvalidRequestException: MemberId is required...` in
   `Test_SDKRoundTrip_NodeCloudwatchLogConfig`. Three existing tests
   (`TestHandler_NodeLifecycle_RealWireShape`, `TestHandler_UpdateNode`,
   `TestHandler_UpdateNodeLogPublishingConfig`) sent `memberId` as a query parameter for UpdateNode
   and were corrected to send it in the body.

3. **`ProposalSummary` (ListProposals) fabricated a `NetworkId` member real AWS never sends.**
   `awsRestjson1_deserializeDocumentProposalSummary` (deserializers.go:6573) has no `NetworkId`
   case at all -- only the full `Proposal` type (GetProposal) has one. Removed from
   `proposalSummaryObject` (`models.go`) and `toProposalSummaryObject` (`handler_proposals.go`).
   `TestHandler_ProposalSummaryHasNetworkID`, a test literally named for asserting the fabricated
   field was present, was rewritten as `TestHandler_ProposalSummaryOmitsNetworkID` asserting its
   absence. Hand-revert reproduced the field reappearing in the raw response body.

4. **`Invitation` (ListInvitations) fabricated top-level `NetworkId`/`NetworkName` members.**
   `awsRestjson1_deserializeDocumentInvitation` (deserializers.go:4762) has cases only for
   `Arn`/`CreationDate`/`ExpirationDate`/`InvitationId`/`NetworkSummary`/`Status` -- real AWS
   carries network identity only inside the nested `NetworkSummary`, confirmed against
   `types.Invitation`'s field list (types/types.go:116-155). Removed from `invitationObject`
   (`models.go`) and `toInvitationObject` (`handler_invitations.go`); the internal `Invitation`
   struct's `NetworkID`/`NetworkName` fields were left alone since `members.go:109`'s
   invitation/network-mismatch check on `CreateMember` genuinely needs them -- only the wire object
   was fabricating them onto the response. No existing test asserted these fields, so a new one
   (`TestHandler_InvitationOmitsTopLevelNetworkFields`) was added; hand-revert reproduced both
   fields reappearing.

All four fixes were proven by hand-revert (flip the fix back, run the relevant test, confirm the
exact predicted symptom, restore, `diff` byte-identical against a pre-revert copy) before being
finalized. Bug 1 and 2 were additionally proven end-to-end through a real
`aws-sdk-go-v2/service/managedblockchain` client round-tripped through `pkgs/service`'s router
(`wire_shape_test.go`, matching `services/mediaconvert/wire_shape_test.go`'s pattern) since
`types.Node.LogPublishingConfiguration` gives a typed client field to observe. Bugs 3 and 4 used a
raw-body absence assertion instead, since the corresponding real types
(`types.ProposalSummary`, `types.Invitation`) have no field for a typed client to observe a
fabricated key on.

No other gap was found in the five summary/full pairs, `NetworkFrameworkAttributes`/
`NetworkFabricAttributes`, `MemberFrameworkAttributes`/`MemberFabricAttributes`,
`NodeFrameworkAttributes`/`NodeFabricAttributes`, `VotingPolicy`/`ApprovalThresholdPolicy`,
`ProposalActions`/`InviteAction`/`RemoveAction`, or `MemberConfiguration`/
`MemberFabricConfiguration`/`NodeConfiguration`. One gap was noted but NOT fixed (Layer 3, out of
this campaign's scope): real AWS's `NodeConfiguration` (CreateNode's input) accepts an optional
create-time `LogPublishingConfiguration` member (`api_op_CreateNode.go`'s
`awsRestjson1_serializeDocumentNodeConfiguration`, serializers.go:2624) that gopherstack's
`nodeConfiguration` request struct does not model at all -- a real client can only set node log
config after creation via `UpdateNode` in gopherstack today.

**`last_audit_commit` provenance**: this pass's audit date is 2026-08-20; `last_audit_commit` was
updated to `a073b2b1` (this repo's HEAD at the time this file was written, per the schema). The
prior entry (`d08692ef`, dated `2026-08-10` in this file) checks out: `git show -s --format=%ad
d08692ef` returns `2026-08-10`, matching the manifest's `last_audit_date` -- unlike appmesh and
codeconnections, both caught this campaign citing a 2026-07-13 sha against a 2026-08-10 audit
date. Verdict: clean provenance, no fabricated audit trail.

## 2026-09-08: writeError nil-on-write fall-through sweep (gopherstack-246v) -- clean

Part of the 12-service sweep for the elasticache class bug (gopherstack-8haq): a helper
that rejects a request via the local response writer and *returns* that writer's result
hands a caller doing `if err != nil { return err }` a `nil`, since the writer returns nil
after a successful write -- the rejection is silently skipped and the operation continues.

**Base writer**: `writeError` (`handler.go:627`) returns `c.JSON(status, errorResponse{...})`
directly -- nil on a successful write. `writeBackendError` (`handler.go:613`, a method) wraps
it, `return writeError(...)` at every branch of its error-classification switch.

**Method (mechanical).** A `go/parser`/`go/ast` script over every non-test `.go` file found
every function with a `return`-statement whose result is a bare call to `writeError`, then
fixed-point-expanded to any function bare-returning a call to an already-found member --
35 functions discovered: `writeBackendError`, all ~28 `handleXxx` op handlers, and the 6
`dispatch`/`dispatchXxxOps` routing functions.

**Dispatch verified, not assumed.** `dispatch` and its four sub-dispatchers
(`dispatchNetworkOps`, `dispatchAccessorOps`, `dispatchProposalOps`, `dispatchInvitationOps`,
plus `dispatchMemberNodeOps`) use a single-error-value sentinel chain: each leaf switches on
`op` and `return`s a `handleXxx(...)` call directly per case, falling through to a private
`errUnknownOp` sentinel if no case matched. Callers check `if err := h.dispatchX(...);
!errors.Is(err, errUnknownOp) { return err }` -- the branch is on the sentinel identity, not
on `err != nil`, so a matched op that rejected via `writeError` (returning nil) still takes
the `!errors.Is(...)` == true path and propagates by bare `return err`. Read all 5 such
sites (`handler.go:490-536`) confirming zero exceptions to the pattern.

Every call site of `writeError` and `writeBackendError` across the package (109 total) was
enumerated: 104 are direct `return writeError(...)` / `return writeBackendError(...)` /
`return h.handleXxx(...)` sites; the other 5 are the `errUnknownOp`-sentinel dispatch
assigns above, verified safe. Zero `_ =` discards, zero stored-and-`!= nil`-checked sites.
Independently confirmed by grepping every non-test-file occurrence of
`writeError(`/`writeBackendError(` outside their own definitions: every one is immediately
preceded by `return` on the same line.

**No instance of the broken shape exists in managedblockchain.** No code changed. Gates
re-run for the record: `GOTOOLCHAIN=go1.27.0 golangci-lint run
./services/managedblockchain/...` 0 issues; `GOTOOLCHAIN=go1.27.0 go test -race
./services/managedblockchain/...` ok.

## 2026-09-08: gopherstack-rcp6 re-audit -- ResourceNotReadyException unreachable (confirmed), ACTION_FAILED reachable (real bug, fixed)

Re-derived gopherstack-rcp6 ("ResourceNotReadyException and ACTION_FAILED are structurally
unreachable") rather than trusting its title. This module ships a `deserializers.go` (not one
of the 11 schema-codegen modules), so declared error sets were read directly from it.

**ResourceNotReadyException: confirmed genuinely unreachable, verdict (a).**
`awsRestjson1_deserializeOpError*` case statements for `ResourceNotReadyException`
(`deserializers.go`) appear in exactly 8 functions: `CreateMember` (:336), `CreateNode`
(:699), `CreateProposal` (:873), `DeleteMember` (:1119), `DeleteNode` (:1222),
`ListTagsForResource` (:3369), `TagResource` (:3605), `UntagResource` (:3705) -- 3 more ops
than the issue's original list (which named only the 5 Create/Delete ops), independently
matched against botocore's `data/managedblockchain/2018-09-24/service-2.json.gz` per-op
`errors` arrays (same 8-op set exactly). Doc comment (`types/errors.go:198-199`, verbatim):
"The requested resource exists but isn't in a status that can complete the operation." Its
only plausible trigger is a resource caught in a transient `CREATING`/`DELETING`/`UPDATING`
status. Enumerated every write site for `NetworkStatus`/`MemberStatus`/`NodeStatus` across
the package (`networks.go:104,250`, `members.go:129,241`, `nodes.go:113,226`): all 6 write
only the `AVAILABLE` constant: `networkStatusAvailable`/`memberStatusAvailable`/
`nodeStatusAvailable`. No write site anywhere sets `CREATING`, `UPDATING`, or `DELETING`.
`DeleteMember` (`members.go:203-221`) and `DeleteNode` (`nodes.go:177-206`) both remove the
resource from the store synchronously in the same call, never transiting through a
`DELETING` status a concurrent request could observe. Verdict stands: this needs a real
async lifecycle simulation across the service, not a one-line guard.

**ACTION_FAILED: the issue's claim was wrong -- reachable via a client-callable path, verdict
(b), fixed.** `ProposalStatus` enum (`types/enums.go:226`, `types.go:938-940` doc, verbatim):
"ACTION_FAILED - One or more of the specified ProposalActions in a proposal that was approved
couldn't be completed because of an error. The ACTION_FAILED status occurs even if only one
ProposalAction fails and other actions are successful." Carried by `Proposal.Status` and
`ProposalSummary.Status` (both `ProposalStatus`). `RemoveAction.MemberId`
(`types/types.go`/botocore `RemoveAction` shape) is never validated against a live member
either at `CreateProposal` time (`proposals.go:78-108`, only the *proposing* member is
checked to exist) or at execution time before this fix (`executeProposalActionsLocked`,
`proposals.go:412-421`, pre-fix: `if m, exists := ...; exists { ...delete... }` -- silently
skipped a missing target with no failure signal). Real AWS allows a member to leave a network
on its own initiative at any time (`DeleteMember`, independent of any pending proposal), and
a network's approved-but-not-yet-executed proposals can reference a member that has since
departed -- this is the natural, single-client-flow trigger, no async simulation or races
between two proposals required. Fixed: `executeProposalActionsLocked` now tracks whether any
`RemoveAction` target is missing and sets `proposal.Status = ACTION_FAILED` (new const
`proposalStatusActionFailed`, `proposals.go`) instead of leaving the proposal `APPROVED`;
other actions in the same proposal still execute, matching the doc's "even if only one
ProposalAction fails" partial-failure semantics. New test:
`TestHandler_ApprovedProposalActionFailedWhenTargetMemberAlreadyGone`
(`proposals_voting_test.go`) -- creates a network + a second member, proposes removing that
member, has the member call `DeleteMember` on itself independent of the proposal, then votes
the stale proposal to approval and asserts `GetProposal` reports `ACTION_FAILED`. Confirmed
failing against the unmodified code: `Not equal: expected: "ACTION_FAILED" actual:
"APPROVED"`.

**Adjacent finding, not fixed (out of gopherstack-rcp6's scope, reported for a follow-up
issue):** `TagResource`'s own doc (botocore `service-2.json.gz` operations.TagResource,
verbatim excerpt): "A resource can have up to 50 tags. If you try to create more than 50 tags
for a resource, your request fails and returns an error." `TagResource`'s declared error set
includes `TooManyTagsException` (`deserializers.go:3605-3654`), distinct from
`ResourceNotReadyException`. `TagResource`/`handleTagResource` (`tags.go`/`handler_tags.go`)
never counts existing tags before merging and has no `TooManyTagsException` sentinel in
`errors.go` at all -- the 50-tag cap is entirely unenforced. Left unfixed here since it is a
different error than the ones gopherstack-rcp6 concerns and needs its own regression test.

Gates re-run for the record: `GOTOOLCHAIN=go1.27.0 golangci-lint run
./services/managedblockchain/...` 0 issues; `GOTOOLCHAIN=go1.27.0 go test -race
./services/managedblockchain/...` ok.

## 2026-09-08: gopherstack-9u4s -- TooManyTagsException wired for TagResource and every Create* op that declares it (fixed)

Fixed the adjacent finding the gopherstack-rcp6 audit above flagged and deferred:
`TagResource` never enforced the documented 50-tag cap and had no
`TooManyTagsException` sentinel at all.

**Declared-error audit, per op** (aws-sdk-go-v2 managedblockchain@v1.34.4
`deserializers.go`, each op's own `awsRestjson1_deserializeOpError<Op>`
switch, read directly rather than grepped):

| Op | Declares `TooManyTagsException`? | Source |
|---|---|---|
| `TagResource` | yes | `deserializers.go:3555` switch |
| `UntagResource` | no | `deserializers.go:3655` switch (removing tags can't overflow the cap) |
| `CreateNetwork` | yes | `deserializers.go:457` switch |
| `CreateMember` | yes | `deserializers.go:277` switch |
| `CreateNode` | yes | `deserializers.go:640` switch |
| `CreateProposal` | yes | `deserializers.go:820` switch |
| `CreateAccessor` | yes | `deserializers.go:85` switch |

Every op that accepts an initial `Tags` map also declares
`TooManyTagsException` -- unlike the fsx precedent (gopherstack-u7rl) where
`TagResource` was the one op that did *not* declare `ServiceLimitExceeded`
while its eleven `Create*` ops did. Here the reverse holds and all seven
tag-touching ops (minus `UntagResource`) declare the same error, so one
check, wired everywhere a declaring op accepts tags, is correct -- no op
was found accepting tags with no suitable declared error.

**Wire-shape constraints, verbatim** (botocore
`managedblockchain/2018-09-24/service-2.json.gz`):

- `InputTagMap` (used by `TagResourceRequest.Tags`, `CreateNetworkInput.Tags`,
  `CreateNodeInput.Tags`, `CreateProposalInput.Tags`,
  `CreateAccessorInput.Tags`, and `MemberConfiguration.Tags` -- the shape
  `CreateMemberInput` nests its tags under, since `CreateMemberInput` itself
  has no top-level `Tags` member): `{"type": "map", "key": {"shape":
  "TagKey"}, "value": {"shape": "TagValue"}, "max": 50, "min": 0}`.
  `min: 0` -- unlike fsx's `Tags` list (`min: 1`), there is no
  omitted-vs-empty trap here to avoid enforcing.
- `TagResourceRequest.Tags` documentation (verbatim): "The tags to assign to
  the specified resource. Tag values can be empty, for example, `"MyTagKey"
  : ""`. You can specify multiple key-value pairs in a single request, with
  an overall maximum of 50 tags added to each resource."
- `TooManyTagsException` shape (verbatim): `{"type": "structure", "members":
  {"Message": {"shape": "ExceptionMessage"}, "ResourceName": {"shape":
  "ArnString", "documentation": "<p/>"}}, "documentation": "<p/>", "error":
  {"httpStatusCode": 400}, "exception": true}` -- the exception carries no
  documentation of its own; the cap's wording lives on each caller's `Tags`
  field instead.

**Per-request vs. per-resource**: per-resource, on the resulting total.
`TagResourceRequest.Tags`'s doc phrase "an overall maximum of 50 tags added
**to each resource**" (and the identical phrasing on
`MemberConfiguration.Tags`, `CreateNetworkInput.Tags`, `CreateNodeInput.Tags`,
`CreateProposalInput.Tags`, `CreateAccessorInput.Tags`) is explicit: the cap
counts the tags a resource ends up with, not the size of one request. A
resource that already carries 45 tags must reject a `TagResource` call
adding 10 more distinct keys (55 > 50) even though 10 alone is under the
per-request `InputTagMap` `max: 50`.

**Fix**: `checkTagLimit(existing, additions map[string]string) error`
(`tags.go`) counts only the keys in `additions` not already present in
`existing`, rejects with `ErrTooManyTags` (new sentinel, `errors.go`) if the
resulting total exceeds `maxTagsPerResource = 50`. Wired into:
`TagResource` (`tags.go`, existing tags read via the pre-existing
`resourceTags` helper), `CreateNetwork` (`networks.go`, checked
independently for both the network's own `tags` and the initial member's
`memberTags` -- two distinct resources, two independent 50-tag budgets),
`CreateMember` (`members.go`, `tags` sourced from
`MemberConfiguration.Tags`), `CreateNode` (`nodes.go`), `CreateProposal`
(`proposals.go`), `CreateAccessor` (`accessors.go`). All five `Create*`
checks pass `nil` for `existing` since a freshly created resource has no
prior tags, making the check equivalent to `len(tags) > 50`.
`handler.go`'s `writeBackendError` gained a case mapping
`errors.Is(err, ErrTooManyTags)` to wire code `TooManyTagsException` (HTTP
400), ahead of the generic `ErrInvalidParameter` fallthrough it would
otherwise match (`ErrTooManyTags` wraps `awserr.ErrInvalidParameter`).

**Regression tests** (`tag_limit_test.go`, new file): `TestTagResource_TooManyTags`
(SDK round-trip: 51 tags on an untagged network, asserts
`errors.As` against `*types.TooManyTagsException` and that
`ListTagsForResource` still returns zero tags afterward);
`TestTagResource_TooManyTags_Cumulative` (45 existing + 10 new distinct
keys = 55 > 50 rejected, and the existing 45 are unchanged after the
rejection); `TestCreateNetwork_TooManyTags` (SDK round-trip, 51 tags on
`CreateNetworkInput.Tags`); `TestCreateOps_TooManyTags` (table-driven,
one subtest per remaining `Create*` op); `TestTagResource_ExactlyFiftyTags`
(boundary: exactly 50 on a previously untagged resource succeeds).

Confirmed failing against the unmodified code before the fix (verbatim,
`TestTagResource_TooManyTags`/`_Cumulative`/`TestCreateNetwork_TooManyTags`
run together): `An error is expected but got nil.` — repeated for all
three, since none of the seven call sites checked the count at all.
Confirmed each new guard is load-bearing by removing the `CreateAccessor`
check (`accessors.go`) alone and re-running
`TestCreateOps_TooManyTags/create_accessor`: fails the same way
(`An error is expected but got nil.`); file still compiles with the guard
removed. Guard restored before commit.

Gates re-run for the record: `GOTOOLCHAIN=go1.27.0 golangci-lint run
./services/managedblockchain/...` 0 issues; `GOTOOLCHAIN=go1.27.0 go test -race
-count=1 ./services/managedblockchain/...` ok.
