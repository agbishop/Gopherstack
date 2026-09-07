---
service: cleanrooms
sdk_module: aws-sdk-go-v2/service/cleanrooms@v1.49.4   # bumped from v1.48.0 this pass (go.mod already pinned v1.49.4; PARITY.md was stale)
last_audit_commit:                                # unknown: pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-09-04
overall: A            # systemic invented-field cleanup + several real state-machine/wire-shape bugs fixed (prior pass); IntermediateTable family implemented for real at the same quality bar (2026-07-25 pass)
                      # 2026-08-07 pass (bd gopherstack-kiqa): CollaborationChangeRequest.Changes is now a
                      # real typed union (Change/ChangeSpecification/MemberChangeSpecification/
                      # CollaborationChangeSpecification) instead of a generic []map[string]any
                      # pass-through, with real required-field/enum validation, and COMMIT now applies
                      # real semantic effects for ADD_MEMBER / GRANT_-/REVOKE_RECEIVE_RESULTS_ABILITY /
                      # EDIT_AUTO_APPROVED_CHANGE_TYPES (see families.CollaborationChangeRequest).
                      # Differential-privacy budget modeling implemented for ListPrivacyBudgets/
                      # ListCollaborationPrivacyBudgets/PreviewPrivacyImpact (see families.PrivacyBudget)
                      # -- found and fixed a real wire-shape bug in the process (PrivacyBudget struct
                      # mislabeled its "type" key as "privacyBudgetType" and carried three invented
                      # duplicate *Identifier fields, the same systemic bug class fixed everywhere else
                      # in this service in a prior pass but missed on this one struct). Members-own-table
                      # (moving Collaboration.Members off the wire into its own store.Table) not
                      # attempted this pass -- still deferred, see gaps.
                      # 2026-09-04 pass: DeleteMembership silently orphaned membership-scoped child
                      # resources (ConfiguredTableAssociation/AnalysisTemplate/PrivacyBudgetTemplate/
                      # IDMappingTable/IDNamespaceAssociation/ConfiguredAudienceModelAssociation/
                      # IntermediateTable) since it never enforced api_op_DeleteMembership.go's "All
                      # resources under a membership must be deleted" -- now returns ConflictException
                      # (modeled on this op) while any remain. See memberships.go's
                      # membershipHasResources and delete_membership_test.go.
                      # 2026-08-13 (bd gopherstack-bv5d): the entire collaboration-scoped API
                      # surface was silently broken for real clients -- twelve response-key bugs
                      # this "wire: ok"/"FIXED" grading never disclosed, since the SDK decodes
                      # nothing from an unrecognised key. Eight were plain wrong keys (e.g.
                      # BatchGetCollaborationAnalysisTemplate wrote analysisTemplates instead of
                      # collaborationAnalysisTemplates; PopulateIdMappingTable emitted a fabricated
                      # mappedJobIdentifier instead of the real idMappingJobId). Four
                      # (GetCollaborationAnalysisTemplate/-ConfiguredAudienceModelAssociation/
                      # -IdNamespaceAssociation/-PrivacyBudgetTemplate) shared their unprefixed
                      # sibling's keyXxx response-key constant, which was correct for the sibling
                      # and wrong for the collaboration-scoped op -- each now has its own
                      # keyCollaborationXxx constant. Also found and fixed while verifying the
                      # request side of these same ops: CreateConfiguredAudienceModelAssociation
                      # read the Name field from "name" instead of the real
                      # configuredAudienceModelAssociationName wire key, and
                      # CreateCollaborationChangeRequest required a client-supplied "types" field
                      # that types.ChangeInput (the real request shape) doesn't have at all --
                      # types.Change.Types is a server-computed response-only field, now derived
                      # server-side via deriveChangeTypes instead of rejected as missing. All
                      # twelve verified against pinned cleanrooms@v1.49.4 deserializers.go and
                      # proven with real aws-sdk-go-v2 client round-trip tests
                      # (sdk_response_keys_test.go), not raw-JSON assertions. A repo-wide grep for
                      # other scoped/unscoped shared response-key constants in this service found
                      # no further instances -- these four were the only ones.
                      # 2026-08-31 (bd gopherstack-6flj/gopherstack-21my, PARITY-gap targeting):
                      # audited the 8 List ops whose names never appeared anywhere in this file --
                      # ListAnalysisTemplates, ListConfiguredTableAssociations, ListConfiguredTables,
                      # ListIdMappingTables, ListIdNamespaceAssociations, ListPrivacyBudgetTemplates,
                      # ListProtectedJobs, ListProtectedQueries -- against their own deserializers in
                      # cleanrooms@v1.49.4 (confirmed restjson1, no case folding). All 8 wrapper keys
                      # were already correct. FOUR sibling-shape bugs found and fixed: AnalysisTemplateSummary,
                      # IdMappingTableSummary, and IdNamespaceAssociationSummary all omit the real,
                      # optional "description" key (types.go) even though each backend already tracks
                      # Description (set at Create*, correctly surfaced by the singular Get) -- a real
                      # client's list arrived with every description silently blank. Fourth:
                      # ConfiguredTableAssociationSummary was missing the real, optional
                      # "analysisRuleTypes" key entirely -- the field isn't just unfilled, it isn't a
                      # struct member at all -- even though AnalysisRuleTypes is real tracked per-association
                      # state (appended by CreateConfiguredTableAssociationAnalysisRule, correctly
                      # surfaced by GetConfiguredTableAssociation). All four proven with real
                      # aws-sdk-go-v2/typed-decoder tests (list_summary_description_test.go,
                      # list_configured_table_associations_rule_types_test.go), confirmed failing
                      # against unmodified code first. ALSO FOUND, NOT FIXED: ProtectedJob and
                      # ProtectedJobSummary both emit a "type" key that is not a member of either real
                      # type at all (types.ProtectedJob/ProtectedJobSummary have no such field --
                      # "type" is request-only, on StartProtectedJobInput). Attempted json:"-" on the
                      # model field, which broke persistence: this backend persists these exact model
                      # structs' JSON encoding directly (store.Table.Snapshot -> json.Marshal), so the
                      # same tag drives both the wire response and the on-disk snapshot -- confirmed by
                      # TestInMemoryBackend_SnapshotRestore_FullState failing (seed.job.Type "PYTHON" ->
                      # restored ""). Reverted. A real client's typed decoder silently drops unrecognized
                      # keys, so this is unobservable to it; the correct fix needs a wire-vs-persistence
                      # split this service doesn't have, out of proportion for this pass -- recorded as a
                      # gap, not fixed. Everything else in the 8 (all 8 wrapper keys; ConfiguredTableSummary;
                      # PrivacyBudgetTemplateSummary; ProtectedQuerySummary except the already-disclosed
                      # ReceiverConfigurations gap) re-verified field-by-field against types.go and is
                      # genuinely clean.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
families:
  Collaboration: {status: ok, note: "FIXED this pass -- see bugs 1-4: CollaborationIdentifier/memberAbilities were invented output fields (deleted from the wire), auto-membership creation added, DeleteMember/DeleteCollaboration state machines fixed. gopherstack ignored-parameter sweep (2026-08-29): ListCollaborationsInput.MemberStatus ('the caller's status in a collaboration') was parsed from the query string but then discarded (passed as `_`) before reaching InMemoryBackend.ListCollaborations -- every collaboration was always returned. Backend signature now takes memberStatus and filters against the (still hardcoded-ACTIVE, see gaps) CollaborationSummary.MemberStatus field. SCREENED 2026-09-07 (bd gopherstack-b76t, errtargetaudit class-A leads): GetCollaboration/UpdateCollaboration (collaborations.go:96,143) emit ResourceNotFoundException for an unknown CollaborationIdentifier, but cleanrooms@v1.49.4's awsRestjson1_deserializeOpErrorGetCollaboration/-UpdateCollaboration switches don't type that exception (only AccessDeniedException/InternalServerException/ThrottlingException/ValidationException) -- the same trio-wide SDK omission already found and accepted for DeleteCollaboration (see handler_delete_collaboration_idempotent_test.go). Verdict: false positive, not fixed. Sibling ops with the identical Get/Update-by-ID shape (GetMembership, UpdateMembership, UpdateConfiguredTable) DO declare ResourceNotFoundException, so this is a genuine, consistent AWS quirk specific to the Collaboration resource's own CRUD trio, not a random omission. restjson1 doesn't restrict a server to an operation's modeled error set -- the generated Go client falls back to smithy.GenericAPIError for an unmodeled code but still reads the literal code off the wire, so .ErrorCode() still reports \"ResourceNotFoundException\" correctly (proven by TestHandler_GetUpdateCollaboration_NonexistentIsResourceNotFound). handler.go's handleError already documents why this matters: the Terraform provider's delete-waiter polls GetCollaboration after DeleteCollaboration and must recognize ResourceNotFoundException by that string. Unlike Delete, Get/Update can't be made idempotent (no real resource to fabricate for a missing ID), so ResourceNotFoundException remains the only non-misleading choice among the declared codes. Regression test added; no production code changed."}
  Membership: {status: ok, note: "FIXED this pass -- MembershipIdentifier/collaborationIdentifier were invented output fields (deleted from the wire); paymentConfiguration (real, required) now always populated with a correct default. FIXED 2026-08-21 (bd gopherstack-r80d): required memberAbilities (types.Membership, types.go:4165) was tagged omitempty -- encoding/json omits a zero-length slice regardless of nilness, so a membership created with an empty creatorMemberAbilities list (a valid, reachable Smithy-required-list state) silently dropped the key. Fixed by removing omitempty and normalizing nil to []string{} in createMembershipLocked; same fix applied to MembershipSummary.MemberAbilities (types.go same struct)."}
  ConfiguredTable: {status: ok, note: "FIXED this pass -- ConfiguredTableIdentifier was an invented output field (deleted from the wire); cascade delete of analysis rules on DeleteConfiguredTable re-verified real. FIXED 2026-08-21 (bd gopherstack-r80d): required allowedColumns (types.go:2059) and analysisRuleTypes (types.go:2077) were both tagged omitempty -- allowedColumns can be legitimately empty (required-on-input list, Smithy required only means present not non-empty); analysisRuleTypes is empty on every table between CreateConfiguredTable and the first CreateConfiguredTableAnalysisRule call, a common, easily reached window. Fixed by removing omitempty on both (ConfiguredTable and ConfiguredTableSummary.analysisRuleTypes), initializing both to []string{} at CreateConfiguredTable, and fixing removeFrom (store.go) to return []string{} instead of nil so DeleteConfiguredTableAnalysisRule on the last rule doesn't reintroduce the same bug."}
  ConfiguredTableAssociation: {status: ok, note: "FIXED this pass -- *Identifier output fields deleted from the wire; cascade delete of ctaAnalysisRules on association delete re-verified real. FIXED 2026-08-21 (bd gopherstack-r80d): required analysisRuleTypes (types.go:2270) was tagged omitempty, same reachable-empty-before-first-rule bug as ConfiguredTable above. Fixed the same way (initialize []string{} at create, removeFrom no longer returns nil). FIXED 2026-08-31 (bd gopherstack-6flj/21my): ConfiguredTableAssociationSummary (the List shape) had no analysisRuleTypes field at all, despite it being real on types.ConfiguredTableAssociationSummary and already tracked per-association -- ListConfiguredTableAssociations always returned it absent. Added, wired from the same state the full resource already surfaces."}
  AnalysisTemplate: {status: ok, note: "FIXED this pass -- *Identifier output fields deleted from the wire. FIXED 2026-08-13 (bd gopherstack-bv5d): GetCollaborationAnalysisTemplate/BatchGetCollaborationAnalysisTemplate/ListCollaborationAnalysisTemplates all emitted the wrong response key -- see overall note. CORRECTED and FIXED 2026-08-14 (bd gopherstack-dv4s): the prior response-key fix never checked field-level shape -- ListCollaborationAnalysisTemplates reused AnalysisTemplateSummary (the membership-scoped shape) verbatim, leaking membershipArn/membershipId and omitting creatorAccountId. types.CollaborationAnalysisTemplateSummary (types.go) declares creatorAccountId, not membershipArn/membershipId -- a genuine distinct shape from types.AnalysisTemplateSummary, not a superset. Now emits a dedicated CollaborationAnalysisTemplateSummary via toCollaborationAnalysisTemplateSummary, populating creatorAccountId from the looked-up collaboration. FIXED 2026-08-31 (bd gopherstack-6flj/21my): ListAnalysisTemplates' AnalysisTemplateSummary omitted the real, optional description key even though it's tracked and correctly surfaced by the singular GetAnalysisTemplate. Added. isSyntheticData remains deferred, unchanged (see gaps)."}
  Schema/SchemaAnalysisRule: {status: ok, note: "FIXED this pass -- collaborationIdentifier was an invented output field (deleted from the wire, collaborationId added); still no Create path anywhere in this backend (matches real API -- schemas are derived from ConfiguredTable+association state), pre-existing and correctly scoped as always-empty until that projection is implemented; SchemaAnalysisRule's real wire shape is actually a deeper types.AnalysisRule union this backend does not model precisely -- deferred, see gaps (unreachable in practice since schemas are never populated)"}
  ProtectedQuery: {status: ok, note: "FIXED prior pass (stuck-status bug); FIXED this pass -- membershipIdentifier was an invented output field (deleted from the wire)"}
  ProtectedJob: {status: ok, note: "FIXED prior pass (stuck-status bug); FIXED this pass -- membershipIdentifier was an invented output field (deleted from the wire). FOUND, NOT FIXED 2026-08-31 (bd gopherstack-6flj/21my): both ProtectedJob and ProtectedJobSummary emit a \"type\" key that is not a member of either real type at all (types.go; \"type\" is request-only, on StartProtectedJobInput) -- unobservable to a real client (typed decoders drop unrecognized keys) but still wrong. Not fixed: this backend persists these exact model structs' JSON encoding directly (store.Table.Snapshot), so the response tag and the on-disk tag are the same tag -- json:\"-\" on Type broke TestInMemoryBackend_SnapshotRestore_FullState (confirmed, then reverted). Needs a wire-vs-persistence split this service doesn't have; out of proportion for this pass."}
  PrivacyBudgetTemplate: {status: ok, note: "FIXED this pass -- *Identifier output fields deleted from the wire. FIXED 2026-08-13 (bd gopherstack-bv5d): GetCollaborationPrivacyBudgetTemplate/ListCollaborationPrivacyBudgetTemplates/ListCollaborationPrivacyBudgets all emitted the wrong response key -- see overall note. CORRECTED and FIXED 2026-08-14 (bd gopherstack-dv4s): the prior response-key fix never checked field-level shape -- ListCollaborationPrivacyBudgetTemplates reused PrivacyBudgetTemplateSummary (the membership-scoped shape) verbatim, leaking membershipArn/membershipId and omitting creatorAccountId. types.CollaborationPrivacyBudgetTemplateSummary declares creatorAccountId, not membershipArn/membershipId. Now emits a dedicated CollaborationPrivacyBudgetTemplateSummary via toCollaborationPrivacyBudgetTemplateSummary. FIXED 2026-08-21 (bd gopherstack-r80d): required autoRefresh (types.go:4874) is optional on CreatePrivacyBudgetTemplateInput (no 'This member is required' on that field) but was passed through unmodified when omitted, then dropped by the omitempty tag -- a real client creating a template without autoRefresh got back a required-but-absent field. Fixed by removing omitempty and defaulting an unspecified value to NONE (the only other valid enum value besides CALENDAR_MONTH, and the natural off-state for an opt-in refresh schedule) in CreatePrivacyBudgetTemplate; UpdatePrivacyBudgetTemplate already guarded against clearing to empty, unchanged."}
  PrivacyBudget: {status: ok, note: "IMPLEMENTED 2026-08-07 (bd gopherstack-kiqa). FIXED wire-shape bug: PrivacyBudget's PrivacyBudgetType field was tagged json:\"privacyBudgetType\" (real wire key, verified against awsRestjson1_deserializeDocumentPrivacyBudgetSummary, is \"type\") and the struct additionally emitted invented privacyBudgetTemplateIdentifier/collaborationIdentifier/membershipIdentifier keys alongside the correctly-named .../Id fields (same systemic bug class fixed elsewhere in this service, missed on this struct); createTime/updateTime (both real, required) were entirely absent. All fixed. ListPrivacyBudgets/ListCollaborationPrivacyBudgets now build a real PrivacyBudgetSummary per DIFFERENTIAL_PRIVACY-type PrivacyBudgetTemplate, deriving a deterministic (documented-approximation, not real-AWS-numeric-parity -- AWS's formula is proprietary/undocumented) aggregation-count budget from the template's stored epsilon/usersNoisePerQuery. PreviewPrivacyImpact computes the same way from request parameters instead of returning a fixed empty shape. Query-time budget CONSUMPTION is not tracked (StartProtectedQuery's differentialPrivacy parameter is not modeled -- remainingCount always equals maxCount, a fresh/unconsumed budget rather than a fabricated partial one); see gaps. ACCESS_BUDGET (the other real PrivacyBudgetType) is not modeled at all -- toPrivacyBudget returns nil for it rather than fabricating a budget. CORRECTED and FIXED 2026-08-14 (bd gopherstack-dv4s): ListCollaborationPrivacyBudgets reused PrivacyBudget (the membership-scoped shape used for ListPrivacyBudgets, despite its name) verbatim, leaking membershipArn/membershipId and omitting the required creatorAccountId that types.CollaborationPrivacyBudgetSummary declares in its place. Now emits a dedicated CollaborationPrivacyBudgetSummary via toCollaborationPrivacyBudget. ListPrivacyBudgets itself (membership-scoped) was re-verified field-by-field against types.PrivacyBudgetSummary and is genuinely correct -- not a leak, despite the misleadingly generic local type name. gopherstack ignored-parameter sweep (2026-08-29): ListPrivacyBudgetsInput/ListCollaborationPrivacyBudgetsInput both declare AccessBudgetResourceArn (an ACCESS_BUDGET-type filter) that neither handler reads nor passes to the backend -- left unfixed rather than fabricated, since ACCESS_BUDGET is not modeled at all (see above) and neither PrivacyBudget nor CollaborationPrivacyBudgetSummary carries any resource-ARN field to filter against. PrivacyBudgetType itself IS honored on both ops (qp(c, \"privacyBudgetType\") reaches the backend and filters correctly) -- confirmed not a gap."}
  IDMappingTable: {status: ok, note: "FIXED this pass -- *Identifier output fields deleted from the wire; Summary was missing the real inputReferenceConfig field, added. FIXED 2026-08-13 (bd gopherstack-bv5d): PopulateIdMappingTable emitted a fabricated mappedJobIdentifier key instead of the real idMappingJobId. FIXED 2026-08-31 (bd gopherstack-6flj/21my): ListIdMappingTables' IdMappingTableSummary omitted the real, optional description key even though it's tracked and correctly surfaced by the singular GetIdMappingTable. Added."}
  IDNamespaceAssociation: {status: ok, note: "FIXED this pass -- *Identifier output fields deleted from the wire; Summary was missing the real inputReferenceProperties field, added. FIXED 2026-08-13 (bd gopherstack-bv5d): GetCollaborationIdNamespaceAssociation/ListCollaborationIdNamespaceAssociations both emitted the wrong response key -- see overall note. CORRECTED and FIXED 2026-08-14 (bd gopherstack-dv4s): the prior response-key fix never checked field-level shape -- ListCollaborationIdNamespaceAssociations reused IDNamespaceAssociationSummary (the membership-scoped shape) verbatim, leaking membershipArn/membershipId and omitting creatorAccountId. types.CollaborationIdNamespaceAssociationSummary declares creatorAccountId, not membershipArn/membershipId. Now emits a dedicated CollaborationIDNamespaceAssociationSummary via toCollaborationIDNamespaceAssociationSummary. FIXED 2026-08-31 (bd gopherstack-6flj/21my): ListIdNamespaceAssociations' IdNamespaceAssociationSummary omitted the real, optional description key even though it's tracked and correctly surfaced by the singular GetIdNamespaceAssociation. Added."}
  ConfiguredAudienceModelAssociation: {status: ok, note: "FIXED this pass -- *Identifier output fields deleted from the wire. FIXED 2026-08-13 (bd gopherstack-bv5d): GetCollaborationConfiguredAudienceModelAssociation/ListCollaborationConfiguredAudienceModelAssociations both emitted the wrong response key, and CreateConfiguredAudienceModelAssociation read Name from the wrong request key (\"name\" instead of configuredAudienceModelAssociationName) -- see overall note. CORRECTED and FIXED 2026-08-14 (bd gopherstack-dv4s): the prior response-key fix never checked field-level shape -- ListCollaborationConfiguredAudienceModelAssociations reused ConfiguredAudienceModelAssociationSummary (the membership-scoped shape) verbatim, leaking membershipArn/membershipId and omitting creatorAccountId. types.CollaborationConfiguredAudienceModelAssociationSummary declares creatorAccountId, not membershipArn/membershipId. Now emits a dedicated CollaborationConfiguredAudienceModelAssociationSummary via toCollaborationConfiguredAudienceModelAssociationSummary. FIXED 2026-08-21 (bd gopherstack-r80d): required configuredAudienceModelArn (types.ConfiguredAudienceModelAssociationSummary, types.go:2011) was never carried by this struct at all -- the exact gap 2026-08-14's dv4s pass found and explicitly deferred ('A real missing-field gap, recorded rather than folded into this pass's leak fix to keep the two bug classes separate', see gaps below). Field added, populated in toConfiguredAudienceModelAssociationSummary from the already-stored full resource -- no new data needed."}
  CollaborationChangeRequest: {status: ok, note: "FIXED prior pass -- see bug 5: the entire shape was wrong (type+details input/output that don't exist in the real API; real API uses changes[]/action). Rebuilt against CreateCollaborationChangeRequestInput/UpdateCollaborationChangeRequestInput/types.CollaborationChangeRequest with a real PENDING->APPROVED/DENIED/CANCELLED->COMMITTED/CANCELLED state machine. IMPLEMENTED 2026-08-07 (bd gopherstack-kiqa): Changes is now a real typed union (Change{Specification,SpecificationType,Types}/ChangeSpecification{Member,Collaboration}/MemberChangeSpecification/CollaborationChangeSpecification, field-diffed against awsRestjson1_deserializeDocumentChange/-ChangeSpecification/-MemberChangeSpecification/-CollaborationChangeSpecification) replacing the prior []map[string]any pass-through, with real validation (specificationType/types enum membership, required specification.member.accountId or specification.collaboration). COMMIT now applies real semantic effects: ADD_MEMBER appends an invited MemberSummary (matching CreateCollaboration's non-creator member shape); GRANT_/REVOKE_RECEIVE_RESULTS_ABILITY toggle CAN_RECEIVE_RESULTS on the matching member (and its Membership.MemberAbilities when it has one); EDIT_AUTO_APPROVED_CHANGE_TYPES writes Collaboration.AutoApprovedChangeTypes (a real, previously-unmodeled Collaboration field, added this pass). Payer-candidate and ML-output-ability change types are validated (real enum values) but their semantic effect is not applied -- those touch PaymentConfiguration payer-candidate lists and MLMemberAbilities, neither modeled in this backend; see gaps. FIXED 2026-08-13 (bd gopherstack-bv5d): ListCollaborationChangeRequests emitted the wrong response key (collaborationChangeRequests instead of collaborationChangeRequestSummaries); CreateCollaborationChangeRequest also required a client-supplied \"types\" field that the real ChangeInput request shape doesn't have (types.Change.Types is server-computed, response-only) -- now derived server-side via deriveChangeTypes, matching the real API's request/response asymmetry. CHECKED 2026-08-14 (bd gopherstack-dv4s): ListCollaborationChangeRequests' CollaborationChangeRequest fields were diffed against types.CollaborationChangeRequestSummary field-by-field -- genuinely clean, no membership-arn-style leak (unlike its five sibling Collaboration-scoped List ops in this service, see AnalysisTemplate/PrivacyBudgetTemplate/PrivacyBudget/IDNamespaceAssociation/ConfiguredAudienceModelAssociation). gopherstack ignored-parameter sweep (2026-08-29): ListCollaborationChangeRequestsInput.Status ('a filter to only return change requests with the specified status') was declared but the handler never read it at all -- every change request in the collaboration was always returned. Backend gained a status param, handler now reads qp(c, \"status\")."}
  IntermediateTable/IntermediateTableAnalysisRule: {status: ok, note: "NEW this pass (parity-4 campaign, SDK bumped v1.45.6->v1.48.0, 12 new ops). Field-diffed against v1.48.0's awsRestjson1_deserializeDocumentIntermediateTable(Summary/ActiveVersion)/IntermediateTableAnalysisRule/IntermediateTableVersionSummary. Membership-owned (routed under /memberships/{id}/intermediateTables, matching AnalysisTemplate/ConfiguredTableAssociation/ProtectedQuery -- CollaborationArn/CollaborationID are derived from the membership at create time, same pattern as those families). IntermediateTableAnalysisRule uses a distinct SDK union (types.IntermediateTableAnalysisRulePolicy, isIntermediateTableAnalysisRulePolicy) from ConfiguredTableAnalysisRule's types.AnalysisRulePolicy (isAnalysisRulePolicy) -- confirmed via the UnknownUnionMember interface-method list in types.go -- so nothing was reused at the Go-type level; both are modeled with this service's established generic map[string]any policy pass-through, so the *strategy* is reused, not code. IntermediateTableAnalysisRule's real output key genuinely is intermediateTableIdentifier (not intermediateTableId), confirmed directly against the deserializer -- a real, documented exception, not a re-introduction of the *Identifier invented-field bug class fixed last pass (locked in by TestIntermediateTables_WireShape). DeleteIntermediateTable cascades to its analysis rule and versions (real ctAnalysisRules-style cascade, locked in by TestHTTP_DeleteIntermediateTable_CascadesAnalysisRule and assertMembershipNestedRestored). PopulateIntermediateTable starts a real ProtectedQuery via a new startProtectedQueryLocked helper shared with StartProtectedQuery (mirroring the createMembershipLocked split) and records a POPULATE_STARTED version; advanceIntermediateTablesLocked resolves both the version and the table to POPULATE_SUCCESS/POPULATE_FAILED once that ProtectedQuery reaches a terminal status, reusing the exact 'advance on next read' pattern StartProtectedQuery already established -- no row count or Schema is ever fabricated (this backend has no SQL engine), locked in by TestHTTP_PopulateIntermediateTable_AdvancesToSuccess. DisallowIntermediateTable does a real name-based lookup (ResourceNotFoundException for an unknown name) and moves the matched table(s) to DISALLOWED_BY_DATA_PROVIDER, which PopulateIntermediateTable then honestly rejects with ConflictException (TestHTTP_PopulateIntermediateTable_AfterDisallow) -- IncludeDescendants cascading is accepted but is a documented no-op (see gaps)."}
  Tags: {status: ok, note: "CRUD + ARN validation (fixed prior pass) re-verified; no change this pass"}
  RouteMatcher/classifyPath: {status: ok, note: "no change this pass; prior pass's GetCollaborationAnalysisTemplate routing fix re-verified via handler_route_matcher_test.go. 2026-08-13 (gopherstack-jqh2 pass 2): re-extracted all 100 ops' real method+path from cleanrooms@v1.49.4 serializers.go independently and confirmed handler_route_matcher_test.go's TestRouteMatcher_MethodSensitivity already covers every op exactly once with the correct method/path (including the two ARN-embeds-slashes special cases, GetCollaborationAnalysisTemplate and the /tags/{arn} family) -- this IS the SDK-route-fidelity table this audit's method calls for; no duplicate added, per the sesv2 precedent."}
gaps:
  - "IMPLEMENTED 2026-08-07 (bd gopherstack-kiqa): ListPrivacyBudgets/ListCollaborationPrivacyBudgets/PreviewPrivacyImpact -- see families.PrivacyBudget. Remaining: query-time budget consumption is not tracked (no differentialPrivacy parameter on StartProtectedQuery), so remainingCount always equals maxCount; ACCESS_BUDGET privacy-budget type is not modeled at all."
  - "Collaboration.Members is kept on the wire (json:\"members\") even though it is not a real field on the real Collaboration/CreateCollaborationOutput/GetCollaborationOutput/UpdateCollaborationOutput shape (confirmed against awsRestjson1_deserializeDocumentCollaboration -- members only come from ListMembers). This is a deliberate exception, not an oversight: Members is the only backing store for ListMembers/DeleteMember and has no separate persisted representation the way tagsByArn has for Tags, so a json:\"-\" tag would silently lose every collaboration's member list across a service restart (store.Table's Snapshot/Restore round-trips through this same struct tag). Real AWS SDK/Terraform clients tolerate the extra key (every deserializer in this service ends its field switch with a default case that discards unrecognized keys), so this trades a harmless wire non-canonicality for correct state persistence. Properly removing it requires moving Members to its own store.Table (like tagsByArn), which is deferred -- not attempted this pass (bd gopherstack-kiqa's third named item); no bd id filed for the follow-up."
  - "IMPLEMENTED 2026-08-07 (bd gopherstack-kiqa): CollaborationChangeRequest's `changes` field is now the typed Change/ChangeSpecification union with real COMMIT semantic effects for ADD_MEMBER/GRANT_-/REVOKE_RECEIVE_RESULTS_ABILITY/EDIT_AUTO_APPROVED_CHANGE_TYPES -- see families.CollaborationChangeRequest. Remaining: ADD_PAYER_CANDIDATE/REMOVE_PAYER_CANDIDATE and the GRANT_/REVOKE_CAN_RECEIVE_MODEL_OUTPUT/GRANT_/REVOKE_CAN_RECEIVE_INFERENCE_OUTPUT change types are validated (real enum values, requests with them are accepted) but their COMMIT effect is not applied -- they touch PaymentConfiguration payer-candidate lists and MLMemberAbilities, neither modeled in this backend."
  - "Collaboration's optional analyticsEngine/dataEncryptionMetadata/allowedResultRegions/isMetricsEnabled/jobLogStatus fields (autoApprovedChangeTypes is now modeled, see above), Membership's isMetricsEnabled/jobLogStatus/defaultJobResultConfiguration/mlMemberAbilities, ProtectedQuery/Job's differentialPrivacy/receiverConfigurations/queryComputePayerAccountId/jobComputePayerAccountId, AnalysisTemplate's errorMessageConfiguration/sourceMetadata/syntheticDataParameters/validations/isSyntheticData, and ConfiguredTable(Summary)'s selectedAnalysisMethods are real optional SDK fields not modeled by this backend (never populated). None are invented -- they are simply omitted (correct per the JSON protocol: an absent optional field is valid), not stubbed with fake values. Deferred as lower-value completeness work."
  - "FOUND 2026-08-14 (bd gopherstack-dv4s, not fixed that pass -- opposite bug direction from the over-wide leaks that pass targeted): types.ConfiguredAudienceModelAssociationSummary declares configuredAudienceModelArn, but this backend's ConfiguredAudienceModelAssociationSummary (used by ListConfiguredAudienceModelAssociations, the membership-scoped op) never carried that field at all, even though the full ConfiguredAudienceModelAssociation resource stores it. FIXED 2026-08-21 (bd gopherstack-r80d) -- see families.ConfiguredAudienceModelAssociation."
  - "IntermediateTable's schema/childResources/tableDependencies (all real, optional fields) are never populated, matching the same 'omit, don't fabricate' convention as the gap above: schema requires actually executing the stored populationAnalysisConfiguration query to learn real column types (this backend has no SQL engine); childResources/tableDependencies require a full base-table-dependency graph across other members' configured tables, which this backend does not build. UpdateIntermediateTable's real 'columns' input (retype existing schema columns) is not modeled for the same reason -- there is no real column data to retype. DisallowIntermediateTable's includeDescendants=true cascade is accepted on the wire but is a documented no-op for the same underlying reason (no dependency graph to cascade through) -- the direct-name-match status transition it performs is real, only the cascade is deferred."
  - "FOUND 2026-08-31 (bd gopherstack-6flj/21my, not fixed): ProtectedJob and ProtectedJobSummary both emit a \"type\" key that is not a member of either real type at all (types.ProtectedJob/ProtectedJobSummary, cleanrooms@v1.49.4 types.go -- \"type\" is request-only, on StartProtectedJobInput, never echoed in any response). Unobservable to a real client since typed decoders drop unrecognized keys. Not fixed because this backend persists these exact model structs' JSON encoding directly (store.Table.Snapshot -> json.Marshal using the same struct tags as the wire response) -- a json:\"-\" tag on Type was tried and confirmed to break TestInMemoryBackend_SnapshotRestore_FullState (job type silently lost across a snapshot round-trip), then reverted. A real fix needs Type to be excluded from the wire response specifically while still round-tripping through persistence, which requires either a dedicated persistence DTO or building the wire response as a redacted map instead of marshaling the struct directly -- both larger changes than this pass's scope."
deferred:
  - "Schema creation/projection from ConfiguredTable+ConfiguredTableAssociation state (pre-existing gap noted in persistence_test.go; not touched this pass, out of scope)"
  - "SchemaAnalysisRule's real wire shape (types.AnalysisRule, a deeper union) is not modeled precisely; unreachable in practice since schemas are never created (see Schema/SchemaAnalysisRule family note)"
leaks: {status: clean, note: "Handler.StartWorker is a no-op (no goroutines, no timers, no channels); Reset()/Snapshot()/Restore() only touch store.Registry-managed tables and the plain tagsByArn map. No lifecycle to leak. Re-verified this pass; the createMembershipLocked refactor (shared by CreateMembership and CreateCollaboration) still runs entirely under the caller's already-held b.mu write lock with no additional goroutines."}
---

## Notes

Protocol: restjson1. This pass field-diffed every resource struct in `models.go` directly
against the real deserializer functions in
`aws-sdk-go-v2/service/cleanrooms@v1.45.6/deserializers.go`
(`awsRestjson1_deserializeDocument<Type>`), extracting the literal `case "<key>":` list for
every type -- not against this package's own handlers or the prior audit's claims (see bug 1
below: the prior audit's claim that `id`/`collaborationIdentifier` are "both real, AWS emits
both" was itself wrong and is corrected here with direct deserializer evidence).

### Bugs fixed this pass

1. **Systemic invented `*Identifier` output fields across almost every resource type.**
   `Collaboration`, `CollaborationSummary`, `Membership`, `MembershipSummary`,
   `ConfiguredTable(Summary)`, `ConfiguredTableAssociation(Summary)`,
   `ConfiguredTableAnalysisRule`, `AnalysisTemplate(Summary)`, `PrivacyBudgetTemplate(Summary)`,
   `IDMappingTable(Summary)`, `IDNamespaceAssociation(Summary)`,
   `ConfiguredAudienceModelAssociation(Summary)`, `ProtectedQuery(Summary)`,
   `ProtectedJob(Summary)`, and `Schema(Summary/AnalysisRule)` all additionally emitted a
   `collaborationIdentifier`/`membershipIdentifier`/`configuredTableIdentifier`/
   `configuredTableAssociationIdentifier`/`analysisTemplateIdentifier`/
   `privacyBudgetTemplateIdentifier`/`idMappingTableIdentifier`/
   `idNamespaceAssociationIdentifier`/`configuredAudienceModelAssociationIdentifier` key
   duplicating the real `id`/`collaborationId`/`membershipId`/etc key with the identical
   value. None of these `*Identifier` forms exist as *output* fields anywhere in the real
   API -- confirmed by grepping every `awsRestjson1_deserializeDocument*` function's field
   switch in `deserializers.go`: every one of them uses only the short form (`id`,
   `collaborationId`, `membershipId`, `configuredTableId`, ...). `*Identifier` is exclusively
   a *request* parameter name (`GetCollaborationInput.CollaborationIdentifier`, etc, which
   `handler_*.go`'s request DTOs correctly still use -- those were not touched). One
   exception found and preserved: `ConfiguredTableAssociationAnalysisRule`'s real wire key
   really is `membershipIdentifier` (not `membershipId`), confirmed directly against
   `awsRestjson1_deserializeDocumentConfiguredTableAssociationAnalysisRule` -- an intentional
   asymmetry in the real API, not a bug. Also on that same type, `membershipArn` was a fully
   invented field with zero real counterpart (removed). `Collaboration` additionally invented
   a `memberAbilities` field (real AWS has no such field on Collaboration -- abilities are
   per-member, visible only via each member's own entry) and a `tags` field (real AWS returns
   tags only from ListTagsForResource, never embedded in the resource body).
   Fixed by tagging every invented Go field `json:"-"` (kept as internal-only bookkeeping,
   since the identical value is still available on the correctly-tagged sibling field) except
   `Collaboration.Members`, which is a deliberate, documented exception (see gaps) because it
   is the sole backing store for ListMembers/DeleteMember with no alternate persisted
   representation. Locked in by `TestCollaborationWireShape`,
   `TestConfiguredTables_Create`, `TestMemberships_Create`,
   `TestAnalysisTemplateHasIDKeys`, and `TestProtectedQueries_Start` asserting the invented
   keys are absent.

2. **Persistence-safety follow-on from fix 1.** `store.Table`'s Snapshot/Restore
   (`pkgs/store`) round-trips resource state through `encoding/json` using each type's
   *exact same* struct tags used for the HTTP response -- there is no separate
   persistence-only representation in this backend (`store_setup.go`'s own doc comment notes
   this was a deliberate Phase-3.3 design choice). This means every internal read of a
   now-`json:"-"`-tagged `*Identifier` field (composite-key derivation in `store_setup.go`,
   parent lookups like `b.collaborations.Get(mem.CollaborationIdentifier)` in
   `analysis_templates.go`/`privacy_budgets.go`/`id_mapping_tables.go`/
   `id_namespace_associations.go`/`configured_audience_model_associations.go`, cascade-delete
   key construction in `configured_tables.go`/`collaborations.go`, and every
   `toXSummary`/List sort predicate) would silently read an empty string for any object that
   had been through a Restore cycle (a real gopherstack server restart), corrupting parent
   lookups and list ordering with no error raised. Fixed by redirecting every such internal
   read to the correctly-tagged sibling field (`.ID`, `.CollaborationID`, `.MembershipID`,
   `.ConfiguredTableID`) instead of the now-dead `*Identifier` field, across
   `store_setup.go`, `collaborations.go`, `memberships.go`, `configured_tables.go`,
   `configured_table_associations.go`, `analysis_templates.go`, `privacy_budgets.go`,
   `id_mapping_tables.go`, `id_namespace_associations.go`,
   `configured_audience_model_associations.go`, `protected_queries.go`, `protected_jobs.go`,
   and `schemas.go`. `Schema`/`SchemaSummary`/`SchemaAnalysisRule` gained a new
   `CollaborationID string \`json:"collaborationId"\`` field for the same reason (they
   previously had no short-form sibling at all).

3. **`DeleteMember` spliced the member out of `Collaboration.Members` instead of marking
   them `REMOVED`.** Real AWS's `DeleteMember` doc comment: "The removed member is placed in
   the Removed status and can't interact with the collaboration" -- the member entry must
   stay in `ListMembers`/`GetCollaboration.members` afterward with `status: "REMOVED"`, not
   disappear. Fixed; locked in by `TestDeleteMember_MarksRemoved`.

4. **`DeleteCollaboration` never transitioned dependent memberships.**
   `MembershipStatus` documents a real `COLLABORATION_DELETED` enum value specifically for
   this case (in addition to `ACTIVE`/`REMOVED`), but `DeleteCollaboration` previously just
   deleted the collaboration row and left every membership under it silently `ACTIVE`
   forever, with no way for a client to ever observe the collaboration is gone via
   `GetMembership`. Fixed: `DeleteCollaboration` now transitions every `ACTIVE` membership
   under the deleted collaboration to `COLLABORATION_DELETED` (matching real AWS, which
   retains memberships after collaboration deletion for audit/history rather than deleting
   them). Relatedly, `CreateCollaboration` did not auto-create a membership for the creator;
   real AWS does (confirmed by `Collaboration.membershipArn`/`membershipId`'s doc comment:
   "The unique ARN for your membership within the collaboration"), and without it there was
   no membership for `DeleteCollaboration` to ever transition in the common case of a
   collaboration with no explicitly-created memberships. Fixed via a new
   `createMembershipLocked` helper shared by `CreateMembership` and `CreateCollaboration`.
   Locked in by `TestDeleteCollaboration_CascadesMembershipToCollaborationDeleted` and the
   membership-count assertions in `TestCollaborationWireShape`/`TestMemberships_List`.

5. **`CollaborationChangeRequest` was an entirely invented shape.** The previous
   implementation's `CreateCollaborationChangeRequest(collaborationID, type, details)` /
   `UpdateCollaborationChangeRequest(collaborationID, changeRequestID, status)` and the
   `CollaborationChangeRequest{Type, Details, CollaborationArn, ChangeRequestIdentifier}`
   struct do not match the real API at all:
   - Real `CreateCollaborationChangeRequestInput` takes `changes []types.ChangeInput`
     (`{specification, specificationType}` objects), never a free-form `type`+`details` pair.
   - Real `UpdateCollaborationChangeRequestInput` takes `action`
     (`APPROVE`/`DENY`/`CANCEL`/`COMMIT` -- `ChangeRequestAction`), never a raw `status`
     write. The old code let a client set `status` to literally any string with no
     validation or transition logic.
   - Real `types.CollaborationChangeRequest` output keys (confirmed against
     `awsRestjson1_deserializeDocumentCollaborationChangeRequest`) are `approvals`, `changes`,
     `collaborationId`, `createTime`, `id`, `isAutoApproved`, `status`, `updateTime` -- there
     is no `changeRequestIdentifier`, `collaborationIdentifier`, `collaborationArn`, `type`,
     or `details` key.
   Fixed: `Changes []map[string]any` (generic pass-through, matching this service's existing
   convention for other complex unions like `Policy`/`TableReference`) replaces
   `type`+`details`; `IsAutoApproved`/`Approvals`/`CollaborationID`/`ID` added with real
   tags; the invented fields kept as `json:"-"` internal bookkeeping. `Update` now takes
   `action` and applies a real state machine
   (`changeRequestNextStatus`): `PENDING` -> `APPROVE`/`DENY`/`CANCEL` ->
   `APPROVED`/`DENIED`/`CANCELLED`; `APPROVED` -> `COMMIT`/`CANCEL` ->
   `COMMITTED`/`CANCELLED`; any other `(action, currentStatus)` pair now returns
   `ConflictException` (previously any status could be force-set at any time, including
   "reopening" a terminal request). An unrecognized `action` value returns
   `ValidationException`. Locked in by `TestCollaborationChangeRequest_Lifecycle`.

### Wire-shape spot checks (no bug found, recorded so the next audit doesn't re-check)

- `createTime`/`updateTime` are epoch-seconds JSON numbers (`float64`), matching
  `smithytime.ParseEpochSeconds` in `deserializers.go` -- correct as-is, no `awstime.Epoch`
  needed since these are already plain numeric fields serialized by `encoding/json`.
- `UntagResource`'s `tagKeys` arrives as a repeated query parameter (`?tagKeys=a&tagKeys=b`),
  confirmed against `serializers.go`'s `encoder.AddQuery("tagKeys")` binding.
  `handleUntagResource`'s query-param fallback is correct.
- `MemberSummary` (the `ListMembers`/`Collaboration.members` item shape) genuinely requires
  `paymentConfiguration` (confirmed required in `types.MemberSummary`); it was previously
  entirely absent from this backend's `MemberSummary` struct. Added, with a default computed
  per `QueryComputePaymentConfig.IsResponsible`'s documented default ("If the collaboration
  creator hasn't specified anyone as the member paying for query compute costs, then the
  member who can query is the default payer"): `{"queryCompute": {"isResponsible":
  <has CAN_QUERY ability>}}` unless the caller supplied an explicit
  `paymentConfiguration` in the member spec. Same default now applied to
  `Membership.PaymentConfiguration` (also real, required, previously could be emitted
  entirely empty).

## 2026-07-25 pass (parity-4 campaign): IntermediateTable family

The Go SDK module was bumped `v1.45.6` -> `v1.48.0`, which shipped 12 new operations this
service's `TestSDKCompleteness` had no coverage for at all: `CreateIntermediateTable`,
`GetIntermediateTable`, `UpdateIntermediateTable`, `DeleteIntermediateTable`,
`ListIntermediateTables`, `ListIntermediateTableVersions`, `PopulateIntermediateTable`,
`DisallowIntermediateTable`, `CreateIntermediateTableAnalysisRule`,
`GetIntermediateTableAnalysisRule`, `UpdateIntermediateTableAnalysisRule`,
`DeleteIntermediateTableAnalysisRule`. All 12 are implemented for real this pass (new files
`intermediate_tables.go`/`handler_intermediate_tables.go`, new `models.go` types, new routing
in `handler_routing.go`, new `store.go`/`store_setup.go` tables) -- none were added to the
`notImplemented` list.

Every wire shape (`IntermediateTable`, `IntermediateTableSummary`,
`IntermediateTableAnalysisRule`, `IntermediateTableVersionSummary`,
`PopulateIntermediateTableOutput`) was field-diffed directly against
`aws-sdk-go-v2/service/cleanrooms@v1.48.0/deserializers.go`'s
`awsRestjson1_deserializeDocumentIntermediateTable*` field switches (the same method the prior
pass used, not against this backend's own handlers), following the "verify every output field
against the real SDK type" rule from `.claude/memories/parity-principles.md` to avoid
re-introducing the systemic invented-`*Identifier`-field bug class the prior pass fixed. No
invented output field was added; see the `IntermediateTable/IntermediateTableAnalysisRule`
family note above for the one real, confirmed exception
(`IntermediateTableAnalysisRule.intermediateTableIdentifier`, not an invented field).

Two design questions the task called out explicitly, resolved by reading the generated SDK
directly rather than assuming:

1. **Does `IntermediateTableAnalysisRule` share `ConfiguredTableAnalysisRule`'s rule-policy
   union?** No. `types.IntermediateTableAnalysisRulePolicy` (`isIntermediateTableAnalysisRulePolicy`)
   and `types.AnalysisRulePolicy` (`isAnalysisRulePolicy`) are separate Go interfaces --
   confirmed by grepping `types.go`'s `UnknownUnionMember` method list, which implements both
   interfaces separately. Their *content* shape is structurally similar
   (`{"v1": {"custom": {...}}}`), but no Go type is shared. Both are modeled with this
   service's existing generic `map[string]any` policy pass-through convention (matching
   `ConfiguredTableAnalysisRule.Policy`, `ConfiguredTableAssociationAnalysisRule.Policy`,
   etc) -- the pass-through *strategy* is reused, not a shared type or shared code path.
2. **Membership or collaboration owned?** Membership. `CreateIntermediateTable`'s real path is
   `/memberships/{membershipIdentifier}/intermediateTables` (confirmed against
   `serializers.go`'s `httpbinding.SplitURI` call for every one of the 12 ops -- all keyed
   under `memberships/{membershipIdentifier}`, never `collaborations/{id}`), matching the
   existing `AnalysisTemplate`/`ConfiguredTableAssociation`/`ProtectedQuery` pattern: created
   under a specific membership, with `CollaborationArn`/`CollaborationID` derived from that
   membership at create time (not independently settable).

`PopulateIntermediateTable`'s doc comment says the returned `analysisId` should be usable "with
GetProtectedQuery to track the population progress" -- taken literally: it now starts a real
`ProtectedQuery` via a new `startProtectedQueryLocked` helper (factored out of
`StartProtectedQuery`, mirroring the existing `createMembershipLocked` split between
`CreateMembership`/`CreateCollaboration`), so `analysisId` is a genuine, `GetProtectedQuery`-able
resource, not a fabricated UUID. A new `IntermediateTableVersionSummary` row is recorded
`POPULATE_STARTED`, and `advanceIntermediateTablesLocked` (called from every
`IntermediateTable`/version read path) resolves both the version and the table to
`POPULATE_SUCCESS`/`POPULATE_FAILED` once that `ProtectedQuery` reaches a terminal status --
reusing the exact "advance on next read" pattern `advanceProtectedQueriesLocked` already
established, since this backend has no background worker (`Handler.StartWorker` is a no-op).
Per the task's explicit instruction, no row count or `Schema` is ever fabricated: this emulator
has no SQL engine to actually execute the stored query, so `IntermediateTable.Schema` is never
populated (real, optional field, simply omitted -- see gaps) even after a version reaches
`POPULATE_SUCCESS`. `DisallowIntermediateTable` does a real name-based lookup against the
membership's tables (`ResourceNotFoundException` for an unknown name) and moves matched tables
to `DISALLOWED_BY_DATA_PROVIDER`, which `PopulateIntermediateTable` then honestly rejects with
`ConflictException` -- real state movement, not a no-op, though the `includeDescendants` cascade
itself is a documented no-op (see gaps: no base-table-dependency graph exists to cascade
through). `DeleteIntermediateTable` cascades to its analysis rule and versions (same
`ctAnalysisRulesByTable`-style index-then-delete pattern `DeleteConfiguredTable` already uses).

Locked in by `TestHTTP_IntermediateTables_Lifecycle`, `TestHTTP_UpdateIntermediateTable`,
`TestHTTP_UpdateIntermediateTableAnalysisRule`,
`TestHTTP_PopulateIntermediateTable_AdvancesToSuccess`, `TestHTTP_DisallowIntermediateTable`,
`TestHTTP_PopulateIntermediateTable_AfterDisallow`, `TestIntermediateTables_WireShape`,
`TestHTTP_DeleteIntermediateTable_CascadesAnalysisRule` (`id_mapping_tables_test.go`), new
routing cases in `TestRouteMatcher_MethodSensitivity` (`handler_route_matcher_test.go`), and
extended `seedFullState`/`assertMembershipNestedRestored`/`assertTagsRestored` coverage in
`persistence_test.go`.

## 2026-08-21 pass (bd gopherstack-r80d, batch 8): required OUTPUT members never populated

This pass is a different cut from every prior one on this file: not "is a field
wrong-keyed or invented" (the systemic bug class the 2026-08-13/-14 passes above
fixed) but "is a field AWS marks `This member is required.` on a response shape
ever *written at all*, including the empty-but-present case a real client needs
to distinguish from an absent key." `cmd/requiredoutputfields` puts cleanrooms at
88 required output fields across 83 ops-with-required (100 ops total) -- second
only to sagemaker (off limits, a concurrent agent's in-flight conversion) among
the campaign's remaining candidates.

**Wire shape**: cleanrooms's `<Op>Output` structs are almost entirely "one
wrapper key = the whole nested domain object" (`{"collaboration": {...}}`,
`{"membership": {...}}`, etc, matching pinpoint/bedrockagent's pattern from
earlier batches), so the flat 83-op count only reflects each op's own
top-level required member. Read all 83 ops' top-level shape AND every domain
struct in `models.go` (~36 types) field-by-field against the real SDK's
`This member is required.` annotations directly
(`$(go env GOMODCACHE)/github.com/aws/aws-sdk-go-v2/service/cleanrooms@v1.49.4/types/types.go`),
not just the op-level tool output -- all 5 bugs found this pass were only
visible this way.

**Trusted vs re-derived**: this service's PARITY.md already had four prior
passes (2026-07-24 field-diff cleanup, 2026-08-07 kiqa, 2026-08-13 bv5d,
2026-08-14 dv4s) that did real, cited SDK-deserializer field-diffing --
unusually rigorous prior work for this campaign. Trusted their invented-field
and wrong-response-key findings (re-spot-checked a sample against
`deserializers.go` rather than re-deriving from scratch: `Collaboration`,
`Membership`, `PrivacyBudget` all still match their documented citations). Did
NOT trust the "clean" implication of `overall: A` for this specific bug
class -- none of those four passes were looking for "required but
legitimately-reachable-empty, tagged omitempty" bugs, which is exactly what
this pass found. One deferred `gaps` entry from 2026-08-14 (dv4s) was already
this exact bug, explicitly named and left open ("opposite bug direction...
recorded rather than folded into this pass's leak fix") -- fixed here, see
below.

**Method for the reachable-empty class**: for every required field tagged
`omitempty`, checked whether the corresponding real AWS input field is (a)
required and enum/ARN-typed (client validation guarantees non-empty, not a
bug: `Collaboration.QueryLogStatus`, `Membership.QueryLogStatus`,
`ConfiguredTable.AnalysisMethod`, `ConfiguredTableAssociation.RoleArn`,
`ConfiguredTableAnalysisRule.Policy`, `IntermediateTable.PopulationAnalysisConfiguration`
-- all confirmed required-on-input via the same `types.go` read, all left
as-is), or (b) genuinely reachable-empty because the input field is optional,
a server-computed accumulator, or the field only exists post-create
(the 5 bugs below). Also confirmed a Go-specific mechanic that matters for
provability: `encoding/json`'s `omitempty` omits a zero-length slice/map
**regardless of nilness** (`[]string{}` is omitted exactly like `nil`), but a
required field simply *not tagged* `omitempty` still needs a non-nil value at
marshal time -- a nil Go slice marshals as JSON `null`, and the real SDK
deserializer's field switch treats a `null` value identically to the key
being entirely absent (`if value == nil { return nil }` in every
`awsRestjson1_deserializeDocument*` list helper, confirmed by reading
`deserializers.go:27377`'s `awsRestjson1_deserializeDocumentMemberAbilities`).
So each fix below both removes `omitempty` and normalizes the Go value to a
non-nil empty slice at construction time -- either fix alone is insufficient.

### 5 bugs found and fixed, all proven via real `aws-sdk-go-v2/service/cleanrooms`
### client round-trip tests (`wire_output_required_r80d_test.go`)

1. **`Membership.MemberAbilities`/`MembershipSummary.MemberAbilities`**
   (`types.go:4165`, required). A collaboration created with an empty
   `creatorMemberAbilities` list (valid: Smithy "required" on a list means
   "must be present", not "non-empty") produced a membership whose
   `memberAbilities` key vanished from `GetMembership`/`ListMemberships`.
   Fixed: removed `omitempty`, normalized `nil` to `[]string{}` in
   `createMembershipLocked` (`memberships.go`).
2. **`ConfiguredTable.AllowedColumns`/`AnalysisRuleTypes`,
   `ConfiguredTableSummary.AnalysisRuleTypes`** (`types.go:2059`/`2077`,
   both required). `AllowedColumns` can be legitimately empty (also
   required-on-input, same Smithy-list argument as above);
   `AnalysisRuleTypes` starts empty on every table between
   `CreateConfiguredTable` and the first `CreateConfiguredTableAnalysisRule`
   call -- a common, trivially reached window, not an edge case. Fixed:
   removed `omitempty` on both fields, initialized `AnalysisRuleTypes:
   []string{}` at `CreateConfiguredTable` (`AllowedColumns` already arrives
   non-nil from any client that satisfies the real client-side required-list
   validation).
3. **`ConfiguredTableAssociation.AnalysisRuleTypes`** (`types.go:2270`,
   required). Same reachable-empty-before-first-rule bug as #2. Fixed the
   same way in `CreateConfiguredTableAssociation`.
4. **`removeFrom` (`store.go`) nil-on-empty-result bug**, found while fixing
   #2/#3: `DeleteConfiguredTableAnalysisRule`/
   `DeleteConfiguredTableAssociationAnalysisRule` on a table/association's
   *last* remaining analysis rule type called `removeFrom`, which built its
   result with `var out []string` -- nil when the loop appends nothing,
   reintroducing the exact same omitted-required-field bug on delete that
   #2/#3 fixed on create. Fixed: `removeFrom` now starts from `out :=
   []string{}`. (Not counted as a 6th bug -- same field/same test as #2/#3,
   this is the fix that makes those two actually complete across the full
   create-then-delete-last-rule lifecycle, not a separate finding.)
5. **`ConfiguredAudienceModelAssociationSummary.ConfiguredAudienceModelArn`**
   (`types.go:2011`, required). Missing struct field entirely -- the exact
   gap 2026-08-14's dv4s pass found and explicitly deferred ("A real
   missing-field gap, recorded rather than folded into this pass's leak fix
   to keep the two bug classes separate", see the now-updated `gaps` entry
   above). Fixed: field added, populated in
   `toConfiguredAudienceModelAssociationSummary` from the already-stored full
   resource -- no new data needed, matching the no-stub rule.
6. **`PrivacyBudgetTemplate.AutoRefresh`** (`types.go:4874`, required).
   Optional on `CreatePrivacyBudgetTemplateInput` (no `This member is
   required.` on that field) but passed through unmodified when the client
   omits it, then dropped by the `omitempty` tag. Fixed: removed
   `omitempty`, defaulted an unspecified value to `NONE` in
   `CreatePrivacyBudgetTemplate` -- the AWS API doc
   (https://docs.aws.amazon.com/clean-rooms/latest/apireference/API_CreatePrivacyBudgetTemplate.html)
   does not state an explicit default, but `CALENDAR_MONTH`/`NONE` are the
   only two valid enum values and `NONE` is the natural off-state for an
   opt-in refresh schedule -- inferred from the enum's own closed value set,
   not fabricated data. `UpdatePrivacyBudgetTemplate` already guarded against
   clearing to empty on update (`if autoRefresh != "" { ... }`), unchanged.

All 6 items (5 counted bugs, #4 folded into #2/#3 as noted) proven via
`wire_output_required_r80d_test.go`'s 5 real-SDK-client round-trip tests,
each hand-reverted to the pre-fix `git show HEAD:<path>` content, confirmed
failing, and restored -- `md5sum` byte-identical to the fixed version after
restore.

### Reviewed, not a bug: fields legitimately safe despite `omitempty`

Every other required field tagged `omitempty` was checked against the real
`CreateXInput`/`UpdateXInput` shape and found required-on-input too (with an
enum, ARN, or union type that has no valid "empty" client-supplied value), so
it is never actually reachable-empty from a real client:
`Collaboration.QueryLogStatus`/`CreateTime`/`UpdateTime`,
`Membership.QueryLogStatus`, `ConfiguredTable.AnalysisMethod`/`TableReference`,
`ConfiguredTableAnalysisRule.Policy`,
`ConfiguredTableAssociation.RoleArn`,
`ConfiguredTableAssociationAnalysisRule.Policy`,
`AnalysisTemplate.Format`/`Schema`/`Source`,
`PrivacyBudgetTemplate.Parameters`,
`IDMappingTable.InputReferenceConfig`/`InputReferenceProperties`,
`IDNamespaceAssociation.InputReferenceConfig`/`InputReferenceProperties`,
`ConfiguredAudienceModelAssociation.ConfiguredAudienceModelArn`,
`CollaborationChangeRequest.Changes`,
`IntermediateTable.PopulationAnalysisConfiguration`,
`IntermediateTableAnalysisRule.AnalysisRulePolicy`. Every `*CreateTime`/
`*UpdateTime` field across every struct is also tagged `omitempty` but always
set at creation time by this backend itself (never client-supplied, never
legitimately zero), so left as-is.

### Disclosed, not a bug: `Schema.Description`/`SchemaStatusDetails`

`types.Schema` (`types.go:5963`) requires `Description` (`*string`) and
`SchemaStatusDetails` (`[]SchemaStatusDetail`), both fields this backend's
`Schema` struct has never carried at all. Confirmed via a repo-wide grep that
`b.schemas.Put` is called from **nowhere** in this package -- there is no
Create path for schemas anywhere in this backend (matches the pre-existing
`Schema/SchemaAnalysisRule` family note: "derived from ConfiguredTable+
association state ... always-empty until that projection is implemented"), so
`GetSchema`/`ListSchemas`/`BatchGetSchema` can only ever return
`ErrNotFound`/empty results -- a populated `Schema` instance is never
constructed by any code path, so this gap is unreachable in practice and no
real-client test can observe it (same honest-emptiness reasoning as the
pre-existing `IntermediateTable.Schema`/`childResources`/`tableDependencies`
gaps). Not fixed, not counted; noted here since the pre-existing gap note
only named `resourceArn`/`schemaTypeProperties`/`selectedAnalysisMethods`/
`schemaStatusDetails` were named already -- `description` was not.

### Disclosed, not a bug (already documented pre-existing gaps, re-confirmed unreachable)

- `ProtectedQuerySummary.ReceiverConfigurations`/`ProtectedJobSummary.ReceiverConfigurations`
  (both required, `types.go:5865` for the Query case) have no struct field at
  all. Already named in the pre-existing `gaps` list ("receiverConfigurations
  (summary) are not modeled"). Deriving a real value would require modeling
  who receives protected-query/job results, which this backend does not
  track independently of `ResultConfiguration` -- left as a documented gap,
  not fabricated, consistent with the no-stub rule.
- `PrivacyBudget.Budget`/`CollaborationPrivacyBudgetSummary.Budget` (required,
  `types.go`) are tagged `omitempty` but re-confirmed NOT a bug:
  `toPrivacyBudget` returns `nil` for `ACCESS_BUDGET`-type templates (not
  modeled, pre-existing gap), and every caller (`ListPrivacyBudgets`'s `if pb
  := toPrivacyBudget(t); pb != nil` guard) skips the whole summary rather
  than emitting one with an empty `Budget` -- so the field is never actually
  present-but-empty on the wire, only absent-as-a-whole-record, which is
  correct.

### Gates

`go build ./services/cleanrooms/...`, `go vet ./services/cleanrooms/...`,
`gofmt -l services/cleanrooms/` (empty), `go test -race
./services/cleanrooms/...`, `golangci-lint run ./services/cleanrooms/...`
(0 issues) all clean. No exported signature changed, so the repo-wide
`go build ./...`/`go vet -tags e2e/integration ./...` gates were run anyway
as a courtesy (see the bd gopherstack-r80d batch-8 comment for verbatim
output) and are also clean.

## gopherstack-wlo1 (2026-08-22): Handler()'s dispatch-miss branch was untyped

`Handler()`'s own `if op == opUnknown { return c.String(http.StatusNotFound,
"not found") }` guard (handler.go) wrote a bare text/plain 404. cleanrooms
is restjson1 (`cleanrooms@v1.49.4` `awsRestjson1_` prefix; error decode via
`restjson.GetErrorInfo`), so a real client saw
`smithy.GenericAPIError{Code:"UnknownError"}`.

Reachability: `RouteMatcher` (handler.go) accepts any request whose path
has the coarse `/collaborations`, `/configuredTables`, or `/memberships`
prefix (or a cleanrooms-owned tagged-resource ARN), while `classifyPath`
classifies by exact method+path -- a prefix-matched sub-path
`classifyPath` doesn't recognise falls through to `opUnknown`, the same
prefix-vs-classifier gap securityhub's analogous fix (a98561767)
established as provable.

Fixed: routes through the existing `handleError(c, ErrNotFound)` --
`ErrNotFound` (errors.go) already maps to `ResourceNotFoundException` at
404 in `handleError`'s own switch, so no new exception vocabulary was
introduced.

Proof: `TestHandler_UnrecognisedPathSurfacesResourceNotFoundException`
(`handler_dispatch_malformed_test.go`) drives a real Clean Rooms client's
`ListCollaborations` through a Finalize-stage middleware that rewrites the
signed request's path from `/collaborations` to
`/collaborations/does-not-exist/nowhere` post-signing -- still inside
`RouteMatcher`'s `/collaborations` prefix but matching none of
`classifyPath`'s cases. Hand-reverted `handler.go` to `git show HEAD`,
confirmed the test fails with `*json.SyntaxError: "invalid character 'o' in
literal null (expecting 'u')"`, restored the fix, `md5sum`-confirmed
byte-identical.

**2026-08-30 (negative-continuation-token sweep)**: `store.go`'s shared `paginate` helper
(backing every `List*` op via `listItems`/`listNestedItems`, 8 call sites across
`configured_tables.go`, `configured_table_associations.go`, `intermediate_tables.go`,
`collaborations.go`, `protected_jobs.go`, `memberships.go`, `protected_queries.go`) used a bare
`fmt.Sscanf(nextToken, "%d", &start)` with no bounds check; `start >= len(items)` does not
catch a negative `start`, so `items[start:end]` panicked given `"-5"` as a NextToken. Fixed at
the decode site: the scanned value is now validated `>= 0` before being assigned to `start`
(so a negative-decoding token falls back to `start=0`, matching every other malformed-token
case), so all 8 callers inherit the fix.

Proof: `TestPaginate_NegativeOffsetToken` (new file
`pagination_negative_token_internal_test.go`) confirmed panicking pre-fix, passes now. Gates:
`go build ./services/cleanrooms/...`, `go vet ./services/cleanrooms/...`, `go test -race
-count=1 ./services/cleanrooms/...`, `golangci-lint run ./services/cleanrooms/...` (0 issues).
Work left uncommitted per this pass's instructions.

## Handler-collision determinism sweep verification (2026-08-31, gopherstack-fr30)

`cmd/reqfielddiff`/`cmd/reqfieldscan` used to resolve a handler by breaking
case-insensitive name ties on Go's randomized map iteration order
(ef0eef041 fixed it). cleanrooms is named in that fix's census of 26
affected services, so it was a candidate for having been measured wrong.

Checked directly: ran the unpatched `reqfielddiff` from `ef0eef041~1` five
times against this service and diffed each run against the current
(fixed) tool's output. `with declared fields` (97/100) and the full
79-entry undeclared-fields list, tier-for-tier, were **identical in every
run, pre-fix and post-fix** -- zero op.field findings changed. The one
number that did move was the summary line's raw `emulator-declared
fields` total (780 in 3 of 5 pre-fix runs, 790 in the other 2; 780
post-fix) -- a package-wide field-declaration count unrelated to any
specific operation's resolution, and it never altered which fields were
reported undeclared for which op. Not investigated further since it
carries no finding-level consequence, but noted here rather than silently
ignored.

No bug found or fixed in this service from this sweep. `reqfieldscan` was
independently re-verified byte-identical for this service too. The honest
result: the pre-fix nondeterminism did not change any reported finding for
cleanrooms.
