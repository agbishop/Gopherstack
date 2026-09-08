---
service: codecommit
sdk_module: aws-sdk-go-v2/service/codecommit@v1.36.4
last_audit_commit: 1835ab406
last_audit_date: 2026-08-13
overall: A            # this pass (gopherstack-gvkf): the entire Comment family (8 ops — the 7
                      # named in the bug plus DeleteCommentContent, found the same day) was
                      # undecodable by a real typed client: Comment.CreationDate/LastModifiedDate
                      # were stored and emitted as RFC3339 strings, but codecommit's own
                      # deserializeDocumentComment requires a JSON number (epoch seconds). A raw
                      # status-code/body test could not see this; only a typed SDK decode could.
                      # Fixed by matching Repository/PullRequest/ApprovalRuleTemplate's existing
                      # pattern (time.Time on the domain struct, .Unix() at the wire boundary).
                      # SECOND bug found and fixed in the same family:
                      # GetCommentsForComparedCommit/GetCommentsForPullRequest emitted a flat
                      # []Comment where the real shape is []CommentsForComparedCommit /
                      # []CommentsForPullRequest, each wrapping a nested "comments" list plus
                      # repositoryName/afterCommitId/beforeCommitId — unknown top-level JSON keys
                      # are silently dropped by the JSON-RPC protocol, so this failed silently
                      # (empty Comments slice) rather than erroring. Both are now correct; see ops
                      # table + Notes below. Prior pass: MergeBranchesBySquash/ByThreeWay now real
                      # distinct backend methods (real parent-count semantics,
                      # TargetBranch/CommitMessage/AuthorName/Email honored, specifier
                      # resolution+validation); GetMergeConflicts validates required fields and
                      # resolves specifiers instead of echoing them; found and fixed an
                      # inverted-boolean bug (GetMergeConflicts always reported mergeable:false);
                      # SameFileContentException now returned by PutFile/CreateCommit.
                      # Content-level merge/conflict diffing remains a gap.
ops:
  CreateRepository: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-23 — decode struct (createRepositoryInput) dropped CreateRepositoryInput.KmsKeyId entirely, so a repository created with a customer KMS key always reported an empty kmsKeyId; Repository.KmsKeyID is a real tracked field, correctly populated by UpdateRepositoryEncryptionKey but never by CreateRepository itself. Now threaded through CreateRepository's backend signature and set at creation time."}
  GetRepository: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRepository: {wire: ok, errors: ok, state: fixed, persist: ok, note: "cascades branches/commits/files/fileHistory/triggers/PRs/comments/commentReactions; comments and fileHistory were leaking past repo deletion before this pass (see Notes)"}
  ListRepositories: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetRepositories: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRepositoryDescription: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRepositoryName: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRepositoryEncryptionKey: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDefaultBranch: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateBranch: {wire: ok, errors: fixed, state: ok, persist: ok, note: "validateBranchName's BranchNameRequiredException/InvalidBranchNameException sentinels were missing from errCodeLookup (see Notes) and so were unreachable — both fell through to generic ValidationException"}
  GetBranch: {wire: ok, errors: ok, state: ok, persist: ok}
  ListBranches: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteBranch: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCommit: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "filesAdded[].blobId was hardcoded empty (fixed prior pass); this pass: filesDeleted[].blobId was omitted entirely (now the real removed blob id, matching filesAdded), and ParentCommitIdOutdatedException/ParentCommitIdRequiredException were unreachable (missing from errCodeLookup — see Notes). ALSO this pass: putFiles entries with content identical to what's already at that path now return SameFileContentException instead of silently creating a no-op commit (the sentinel existed but no backend path ever returned it — see gaps' prior note, now partially closed). FIXED 2026-09-07 (gopherstack-8pe4): that SameFileContentException was itself the wrong code — CreateCommit's own declared error set (codecommit@v1.36.4) has no SameFileContentException at all (that's PutFile-only); the identical-content check now returns NoChangeException, CreateCommit's real declared equivalent. See Notes."}
  GetCommit: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-09-07 (gopherstack-8pe4): not-found returned CommitDoesNotExistException, a real but different exception (used correctly elsewhere by CreateBranch/the merge family for a commit-specifier-resolution failure); GetCommit's own declared error set has CommitIdDoesNotExistException specifically for an unresolvable commitId (verified against the real API docs' Errors section). See Notes."}
  BatchGetCommits: {wire: ok, errors: fixed, state: ok, persist: ok, note: "CHECKED 2026-09-07 (gopherstack-8pe4): errtargetaudit flagged the per-entry BatchCommitError.ErrorCode literal (\"CommitDoesNotExistException\") as a class A finding. FALSE POSITIVE (unchanged) — this is document data in a 200 response (BatchGetCommitsOutput.errors[].errorCode), not a thrown/declared HTTP exception, so it isn't in BatchGetCommits' deserializeOpError set at all. FIXED 2026-09-07 (gopherstack-pfyr): the VALUE was wrong. codecommit's api-2.json types both GetCommitInput.commitId and BatchGetCommitsError.commitId as the ObjectId shape (a raw full-SHA lookup) — the same shape, with GetCommit's own declared not-found error being CommitIdDoesNotExistException. Every CommitDoesNotExistException-throwing op (CreateBranch and 17 others) instead uses the CommitId shape, reserved for specifier-resolution fields (branch tips, before/after commit, merge base). BatchGetCommits' own live API doc even describes the two errors[] failure modes — \"shortened SHA ID\" / \"not found\" — matching GetCommit's InvalidCommitIdException/CommitIdDoesNotExistException pair exactly, not CreateBranch's specifier-resolution CommitDoesNotExistException. Now CommitIdDoesNotExistException. See Notes."}
  PutFile: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "blobId was hardcoded empty (fixed prior pass); this pass: File.CommitSpecifier stored branchName instead of the real commit id, so GetFile's commitId field after a PutFile returned the branch name — now the real commit id. Also never recorded fileHistory, so files written via PutFile (not CreateCommit) were invisible to ListFileCommitHistory — now recorded. ALSO this pass: writing content identical to what's already at that path now returns SameFileContentException instead of silently creating a no-op commit"}
  GetFile: {wire: ok, errors: fixed, state: ok, persist: ok, note: "not-found now FileDoesNotExistException, was RepositoryDoesNotExistException"}
  GetFolder: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteFile: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "deleting a non-existent path silently fabricated a commit before; now FileDoesNotExistException. blobId in response was hardcoded empty; now the removed file's real blob id. This pass: parentCommitId was accepted and silently ignored (documented gap) — now required and validated against the branch tip (ParentCommitIdRequiredException/ParentCommitIdOutdatedException), matching real AWS's DeleteFileInput.ParentCommitId (a required field per the SDK's validators.go). Also never recorded fileHistory — now recorded"}
  GetBlob: {wire: ok, errors: fixed, state: ok, persist: ok, note: "not-found now BlobIdDoesNotExistException, was RepositoryDoesNotExistException"}
  ListFileCommitHistory: {wire: fixed, errors: ok, state: fixed, persist: fixed, note: "revisionDag entries were raw Commit objects; real AWS's shape is FileVersion (blobId/path/commit/revisionChildren) — a real SDK client's FileVersion deserializer could not have read this response at all. Also added nextToken/maxResults pagination (was a documented deferred item) and fixed the underlying state gap: PutFile/DeleteFile never populated fileHistory (see those ops' notes), so single-file writes/deletes were invisible to this op even though it's the primary op AWS clients use to see a file's commit history"}
  CreateApprovalRuleTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  GetApprovalRuleTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApprovalRuleTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  ListApprovalRuleTemplates: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApprovalRuleTemplateContent: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApprovalRuleTemplateDescription: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApprovalRuleTemplateName: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateApprovalRuleTemplateWithRepository: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateApprovalRuleTemplateFromRepository: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchAssociateApprovalRuleTemplateWithRepositories: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchDisassociateApprovalRuleTemplateFromRepositories: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAssociatedApprovalRuleTemplatesForRepository: {wire: ok, errors: ok, state: ok, persist: ok}
  ListRepositoriesForApprovalRuleTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePullRequest: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPullRequest: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPullRequests: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePullRequestTitle: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePullRequestDescription: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePullRequestStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribePullRequestEvents: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- PullRequestEvent.EventDate was a string built via time.Now().UTC().Format(time.RFC3339); the real EventDate deserializes via ParseEpochSeconds(json.Number) (deserializers.go, case \"eventDate\"), so every real SDK client's DescribePullRequestEvents call failed outright once any pull request event existed (always true after OverridePullRequestApprovalRules). Fixed by changing the domain field to time.Time and projecting to epoch seconds at the handler's wire-build step. Proven via a real aws-sdk-go-v2/service/codecommit client round trip (wire_pull_request_event_test.go), hand-reverted/confirmed-failing (expected EventDate to be a JSON Number, got string instead)/restored, md5sum-verified byte-identical. FIXED 2026-09-08 (gopherstack-a7tx): ActorArn was dropped from the decode struct entirely (silently ignored) and unvalidated; also OverridePullRequestApprovalRules -- the only op that ever records a PullRequestEvent -- hardcoded its event's actor to \"\" instead of the resolved caller identity already available via awsmeta.CallerArn(ctx) (set onto the request context repo-wide by cli.go's principalMiddleware before dispatch runs; codecommit's own dispatch was discarding ctx). Now: PullRequestEvent carries ActorARN, OverridePullRequestApprovalRules records the real caller, actorArn is parsed+validated as an ARN (InvalidActorArnException on a malformed value, matching the declared error set) and used to filter DescribePullRequestEvents. NOT fixed (structural, out of scope): ActorDoesNotExistException (would require cross-service coupling to IAM's user/role store to check the ARN actually names an account principal); also, 8 of the 9 real PullRequestEventType values (PULL_REQUEST_CREATED, _STATUS_CHANGED, _SOURCE_REFERENCE_UPDATED, _MERGE_STATE_CHANGED, _APPROVAL_RULE_CREATED/_UPDATED/_DELETED, _APPROVAL_STATE_CHANGED) are never recorded by any backend op -- only PULL_REQUEST_APPROVAL_RULE_OVERRIDDEN is, so actorArn filtering is only observable against that one event type today; a pre-existing gap, not introduced or widened by this pass."}
  CreatePullRequestApprovalRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePullRequestApprovalRule: {wire: ok, errors: fixed, state: ok, persist: ok, note: "rule-not-found now ApprovalRuleDoesNotExistException, was RepositoryDoesNotExistException"}
  UpdatePullRequestApprovalRuleContent: {wire: ok, errors: fixed, state: ok, persist: ok, note: "rule-not-found now ApprovalRuleDoesNotExistException, was RepositoryDoesNotExistException"}
  UpdatePullRequestApprovalState: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-30 (gopherstack-4a8v): revisionId is a required UpdatePullRequestApprovalStateInput member (codecommit@v1.36.4 api_op_UpdatePullRequestApprovalState.go) that was decoded and never validated. Added a required-field check. NOT fixed (gap): no staleness/mismatch check against the PR's real, tracked RevisionID (models.go/pull_requests.go) -- real AWS can also return InvalidRevisionIdException/RevisionNotCurrentException for a wrong or stale value; only the RevisionIdRequiredException case is covered, to avoid inventing which of those two codes an unmodeled mismatch should map to."}
  GetPullRequestApprovalStates: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-30 (gopherstack-4a8v): same revisionId-required fix and same NOT-fixed staleness-check gap as UpdatePullRequestApprovalState above (GetPullRequestApprovalStatesInput.RevisionId is also required)."}
  EvaluatePullRequestApprovalRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-lx5h) — response emitted evaluationResults, an array of {approvalRuleName,satisfied} objects; the real required key (deserializers.go EvaluatePullRequestApprovalRulesOutput) is a single evaluation object (types.Evaluation: approved/overridden/approvalRulesSatisfied/approvalRulesNotSatisfied). Prior wire: ok was false. Handler now splits the backend's per-rule []RuleEvaluation into satisfied/not-satisfied name lists and folds in the existing prOverrides/prOverriders override state (approved := overridden || no unsatisfied rules). Backend still marks every rule Satisfied: true unconditionally (never checks a rule's real approval-pool/numberOfApprovalsNeeded content against actual approvals) — that evaluation-logic gap is pre-existing and out of this pass's scope (a wrong-key bug, not a wrong-logic one), tracked separately. FIXED 2026-08-30 (gopherstack-4a8v): same revisionId-required fix and same NOT-fixed staleness-check gap as UpdatePullRequestApprovalState above (see its note)."}
  OverridePullRequestApprovalRules: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-30 (gopherstack-4a8v): same revisionId-required fix and same NOT-fixed staleness-check gap as UpdatePullRequestApprovalState (see its note). FIXED 2026-09-08 (gopherstack-a7tx): the recorded PullRequestEvent's actor was hardcoded to the empty string instead of the resolved caller (see DescribePullRequestEvents' note above for the full fix and its scope)."}
  GetPullRequestOverrideState: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-30 (gopherstack-4a8v): same revisionId-required fix and same NOT-fixed staleness-check gap as UpdatePullRequestApprovalState (see its note)."}
  MergePullRequestByFastForward: {wire: ok, errors: ok, state: ok, persist: ok}
  MergePullRequestBySquash: {wire: ok, errors: ok, state: ok, persist: ok, note: "status transition is real; content-level squash semantics are not modeled (see gaps)"}
  MergePullRequestByThreeWay: {wire: ok, errors: ok, state: ok, persist: ok, note: "status transition is real; content-level 3-way merge semantics are not modeled (see gaps)"}
  MergeBranchesByFastForward: {wire: ok, errors: ok, state: ok, persist: ok, note: "OUT-OF-SCOPE FINDING (not fixed this pass, flagging per audit brief): same TargetBranch/source-dest-existence-validation gaps found and fixed in Squash/ThreeWay this pass also apply here — TargetBranch is accepted by the real MergeBranchesByFastForwardInput but never read (always updates destinationCommitSpecifier's literal string as if it were the target branch name), and neither source nor destination specifier is validated to exist before creating a commit and moving a branch. Also creates a brand-new zero-parent commit unconditionally, where real AWS fast-forward semantics would typically just move the branch pointer to the existing source commit without fabricating a new one. This op was graded ok by two prior audits and is outside this pass's assigned scope (codecommit-3bsb was Squash/ThreeWay/GetMergeConflicts specifically); left as-is, not re-graded, but noted for a future pass."}
  MergeBranchesBySquash: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED this pass — was calling the FastForward backend method verbatim; now a real distinct method: resolves+validates both specifiers exist (CommitDoesNotExistException if not, previously unvalidated), creates a commit with exactly ONE parent (the destination tip, matching real squash-merge shape vs. 3-way's two), and honors TargetBranch/CommitMessage/AuthorName/Email request fields that were previously silently dropped. Content-level squash (combining file changes) still not modeled — see gaps. CHECKED 2026-08-30 (gopherstack-4a8v): mergeBranchesRequest.{TargetBranch,CommitMessage,AuthorName,Email} were flagged unread by cmd/reqfieldscan's anonymous-struct-decode scan -- FALSE POSITIVE, confirmed by reading handler_merges.go: they ARE read, via mergeBranchesRequest's own options() method (r.TargetBranch etc., handler_merges.go:388-391), which the tool's collectLocalBindings doesn't bind because it only tracks a function's own parameters/locals, never a method receiver. No code change."}
  MergeBranchesByThreeWay: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED this pass — same as MergeBranchesBySquash, but the created commit has TWO parents ([destination, source]), a real merge-commit shape FastForward's zero-parent commit and Squash's one-parent commit both lack. Content-level 3-way merge still not modeled — see gaps. Same false-positive check as MergeBranchesBySquash above (shares mergeBranchesRequest)."}
  CreateUnreferencedMergeCommit: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-08-23 — decode struct dropped CreateUnreferencedMergeCommitInput's authorName/commitMessage/email entirely (the exact bug class PutFile/DeleteFile were fixed for, gopherstack-n3zi's flagged lead): the resulting commit always carried the hardcoded 'Unreferenced merge commit' message and an anonymous author, even though Commit.AuthorName/AuthorEmail/Message are real tracked fields populated correctly by CreateCommit and MergeBranchesBySquash/ByThreeWay. Now threaded through the backend signature and set on the commit, defaulting to the prior hardcoded message only when the client omits commitMessage (matching MergeBranchesBySquash/ByThreeWay's own default-message pattern). FIXED 2026-08-30 (gopherstack-4a8v): mergeOption is a required CreateUnreferencedMergeCommitInput member (api_op_CreateUnreferencedMergeCommit.go) that was parsed and never validated OR forwarded to the backend at all -- the backend method has no mergeOption parameter to receive it. Added the same required+valid-enum check BatchDescribeMergeConflicts/GetMergeConflicts already had. Not threaded into the backend beyond validation: like GetMergeConflicts's own blank-discarded mergeOption (merges.go), this backend has no per-branch content model to actually compute a differing squash/3-way/fast-forward result, so there's nothing for the value to drive once it's valid."}
  GetMergeCommit: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-30 (gopherstack-4a8v): the decode struct declared a mergeOption field that is not a real GetMergeCommitInput member at all (confirmed against api_op_GetMergeCommit.go and awsAwsjson11_serializeOpDocumentGetMergeCommitInput in serializers.go -- a real client never sends it). Deleted rather than wired up, per this campaign's fabricated-field convention. No observable runtime behavior changed (the field was already never read), so no new regression test was written for the deletion itself -- existing tests (TestHandler_GetMergeCommit et al.) still pass sending the now-ignored key, since an unrecognized JSON key is silently dropped by encoding/json either way."}
  GetMergeConflicts: {wire: fixed, errors: fixed, state: fixed, persist: n/a, note: "FIXED this pass — three bugs: (1) required-field/mergeOption-enum validation was entirely missing (repositoryName/sourceCommitSpecifier/destinationCommitSpecifier/mergeOption all 'This member is required' per the real SDK's validateOpGetMergeConflictsInput); (2) sourceCommitId/destinationCommitId echoed the raw request specifier instead of the resolved commit ID (now resolved via resolveCommitSpecifier, CommitDoesNotExistException if unresolvable); (3) SEVERE — mergeable was hardcoded to `false` (inverted: this emulator never computes real conflicts, so every merge was actually mergeable, but every real client polling this op before merging would have seen mergeable:false and refused to proceed). Now true. conflicts/mergeHunks remain always empty — no content-diff engine (see gaps); this is AWS-correct for FAST_FORWARD_MERGE specifically (doc-guaranteed empty) but a documented gap for SQUASH_MERGE/THREE_WAY_MERGE. FIXED (gopherstack-lx5h) — response key was also wrong: emitted \"conflicts\", real required key (deserializers.go) is conflictMetadataList. Confirmed the always-empty list itself is the deliberate, documented stub described above (no content-diff engine) and left that behavior untouched; only the key name changed, which is a zero-behavior-change fix since the value is always []"}
  GetMergeOptions: {wire: ok, errors: ok, state: n/a, persist: n/a}
  DescribeMergeConflicts: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "was a disguised no-op that echoed the request and never checked the repository existed; now delegates to the same backend logic as BatchDescribeMergeConflicts with full validation"}
  BatchDescribeMergeConflicts: {wire: ok, errors: ok, state: partial, persist: n/a, note: "validates repo/params correctly; conflicts are always empty since files aren't diffed (see gaps, same root cause as GetMergeConflicts). NOT touched this pass — still echoes the raw specifier strings rather than resolving them (unlike GetMergeConflicts, fixed this pass); flagged as a smaller, lower-priority instance of the same pattern for a future pass, out of this pass's scope (issue was GetMergeConflicts specifically)."}
  PostCommentForComparedCommit: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — Comment.CreationDate/LastModifiedDate were RFC3339 strings; codecommit@v1.36.4 deserializers.go:20415,20430 requires a JSON number (smithytime.ParseEpochSeconds), so every response was undecodable by a real client (status 200, unreadable body). Now time.Time on the domain struct + .Unix() at the wire boundary, matching Repository/PullRequest/ApprovalRuleTemplate. Also now echoes repositoryName/afterCommitId/beforeCommitId at the top level (previously omitted; beforeCommitId was parsed from the request and silently discarded)"}
  PostCommentForPullRequest: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — same CreationDate/LastModifiedDate string-vs-JSON-number bug as PostCommentForComparedCommit. Also now echoes pullRequestId/repositoryName/afterCommitId/beforeCommitId at the top level (previously omitted; the backend still doesn't store afterCommitId/beforeCommitId per-comment, so these are echoed from the request, not read back from storage)"}
  PostCommentReply: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — same timestamp bug. errors: parent-not-found now CommentDoesNotExistException, was RepositoryDoesNotExistException"}
  GetComment: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — same timestamp bug. errors: not-found now CommentDoesNotExistException, was RepositoryDoesNotExistException"}
  GetCommentReactions: {wire: ok, errors: fixed, state: ok, persist: ok, note: "operates on Reaction, not Comment — unaffected by gopherstack-gvkf"}
  GetCommentsForComparedCommit: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — TWO bugs. (1) same Comment timestamp bug as the rest of the family. (2) SEPARATE, more severe bug: the real response is []CommentsForComparedCommit (deserializers.go:20763), each wrapping a nested \"comments\" array plus repositoryName/afterCommitId/beforeCommitId/afterBlobId/beforeBlobId/location — this emulator emitted a flat []Comment instead. Unknown top-level JSON keys are silently dropped by the JSON-RPC protocol (no decode error), so every real client got back a group with an empty Comments slice — total silent data loss, worse than a hard failure. Now wraps all matching comments into one group (repositoryName/afterCommitId always set, beforeCommitId when provided; afterBlobId/beforeBlobId/location omitted — not tracked by this backend, and are optional pointer fields in the real shape)"}
  GetCommentsForPullRequest: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — same two bugs as GetCommentsForComparedCommit: Comment timestamps, and flat []Comment instead of []CommentsForPullRequest (deserializers.go:20883) wrapping a nested \"comments\" list. Now wraps into one group with pullRequestId always set and repositoryName populated from the stored comments' RepoName when available; afterCommitId/beforeCommitId omitted (PostCommentForPullRequest doesn't persist them per-comment)"}
  PutCommentReaction: {wire: ok, errors: fixed, state: ok, persist: ok, note: "operates on Reaction, not Comment — unaffected by gopherstack-gvkf"}
  UpdateComment: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — same timestamp bug. errors: unchanged from prior pass"}
  DeleteCommentContent: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-gvkf) — same timestamp bug via the shared commentToMap converter; not one of the 7 ops named in the original bug report, but calls the identical converter and was found broken the same way while auditing the rest of the family. errors: unchanged from prior pass"}
  GetDifferences: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "was a documented deferred item (nextToken/maxResults accepted but not enforced); now paginated via pkgs/page. Also fixed a wire-shape bug: this op is the one CodeCommit exception to lowercase pagination field names — both request and response use MaxResults/NextToken (capital), verified against the SDK's generated (de)serializers; the handler previously used lowercase and so real pagination requests/responses were silently no-ops"}
  GetRepositoryTriggers: {wire: ok, errors: ok, state: ok, persist: ok}
  PutRepositoryTriggers: {wire: ok, errors: ok, state: ok, persist: ok}
  TestRepositoryTriggers: {wire: ok, errors: ok, state: fixed, persist: n/a, note: "always-succeed simulation; matches AWS's own TestRepositoryTriggers semantics (it doesn't invoke real destinations either). FIXED 2026-08-30 (gopherstack-4a8v): the request's own, required \"triggers\" list (TestRepositoryTriggersInput.Triggers, api_op_TestRepositoryTriggers.go) was decoded and then discarded -- the backend tested whatever was currently saved via PutRepositoryTriggers instead, even though real AWS's own doc comment says testing \"does not change or create a repository trigger\", i.e. the two must be independent inputs. A request testing zero triggers against a repo with saved triggers wrongly reported the saved ones as successful; a request testing triggers no one had ever PUT reported nothing. Backend signature now takes the request's trigger list directly. An existing test (TestHandler_TestRepositoryTriggers) asserted exactly the old wrong behavior (sent triggers:[] in the request, asserted 1 success from a prior PutRepositoryTriggers call) -- corrected to assert on the request's own triggers instead of dropping it; a second existing test in wire_field_fixes_y1zn_test.go had the same shape and was corrected the same way, both hand-confirmed failing against unmodified code first."}
families:
  approval_rule_template_crud: {status: ok, note: "Create/Get/Delete/List/Update* all verified against real SDK shapes"}
  pull_request_lifecycle: {status: ok, note: "create/list/get/update/status/events verified"}
  pull_request_approval: {status: ok, note: "rules, states, overrides, evaluation all mutate real backend state; 2 error-code fixes this pass"}
gaps:
  - "MergeBranchesBySquash/MergeBranchesByThreeWay (FIXED this pass to be real, distinct backend methods — see ops table) still do not model content-level squash/3-way merge semantics: the produced commit has the right parent-count shape (one parent for squash, two for three-way) and the right branch-tip update, but there is no second version of any file to actually combine. Root cause, re-confirmed this pass: File is stored flatly, keyed only by repoName|filePath (fileKey in store_setup.go) — there is no per-branch or per-commit file tree at all, so there is no 'source branch version' vs 'destination branch version' of a file to even diff, let alone merge. Implementing real content-level merge semantics is not a bug fix but a full data-model rework (branch- or commit-scoped file trees) touching PutFile/DeleteFile/CreateCommit/GetFile/GetFolder/GetDifferences and every other file-reading op; out of scope for this pass. (bd: gopherstack-3bsb follow-up)"
  - "GetMergeConflicts (FIXED this pass — see ops table for the mergeable-inversion bug and validation gaps closed)/BatchDescribeMergeConflicts/DescribeMergeConflicts never report a real conflict: conflicts/mergeHunks are always empty. Same root cause as the merge-strategy gap above (no per-branch file state to diff) — there is nothing to diff even in principle without a data-model change. Note: for FAST_FORWARD_MERGE specifically this is not a gap at all — AWS's own GetMergeConflictsOutput.ConflictMetadataList doc comment guarantees an empty list for that strategy, so the behavior is correct by definition there; the gap is genuinely only SQUASH_MERGE/THREE_WAY_MERGE. (bd: gopherstack-3bsb follow-up)"
  - "FilePathConflictsWithSubmodulePathException (ErrFilePathConflicts in errors.go) is declared and wired into errCodeLookup, but no backend path ever returns it — submodules aren't modeled at all in this backend, so there is no concept to build a conflict check on. SameFileContentException (ErrSameFileContent) was the other half of this gap and is FIXED this pass — PutFile and CreateCommit's putFiles entries now compare new content against the existing blob at that path and reject identical writes (see PutFile/CreateCommit ops rows). Note this is a best-effort approximation, not full parity: because File has no per-branch identity (same root cause as the merge gaps above), the comparison is against the single flat current value at that path repo-wide, not specifically against the destination branch's parent-commit content the way real AWS computes it — for a repo with no branch divergence at a path (the common case) these are identical, but they could theoretically diverge. (bd: gopherstack-3bsb follow-up, partially closed)"
  - "2026-08-23: MergePullRequestBySquash/MergePullRequestByThreeWay drop authorName/commitMessage/email from their decode structs, the same shape as the CreateUnreferencedMergeCommit bug fixed this pass — but this is a modelling gap, not a bug: neither backend method creates a Commit at all (they only flip PullRequestStatus and LastActivityDate; unlike MergeBranchesBySquash/ByThreeWay, no branch tip moves and no commit object exists to carry an author/message onto). The real MergePullRequestBySquashOutput doesn't even return author/message — that data would surface via PullRequestTarget.MergeMetadata (MergeCommitId/MergedBy/IsMerged, types.go:936 in codecommit@v1.36.4), a struct gopherstack's PullRequestTarget doesn't model at all. Adding just the three decode fields with nothing to do with them would be a no-op stub, which parity-principles.md rule 1 forbids. Root cause is the same PR-merge-doesn't-create-a-commit gap already noted by the 2026-08-07 pass (see 'Traps for the next auditor' below) — not synthesized here. (bd: gopherstack-3bsb follow-up)"
  - "2026-08-23: UpdateApprovalRuleTemplateContent/UpdatePullRequestApprovalRuleContent drop ExistingRuleContentSha256 from their decode structs. This IS a genuine modelling gap, not a false positive: ApprovalRuleTemplate.RuleContentSha256 is a real tracked field (computed and returned correctly elsewhere), so the precondition value exists to compare against — but there is no comparison logic anywhere in this backend, and no InvalidRuleContentSha256Exception equivalent in errors.go (the real SDK has one: deserializers.go:15493, codecommit@v1.36.4), confirming the optimistic-concurrency check itself was never implemented, not merely that the parameter was dropped. A real client relying on this precondition to avoid clobbering a concurrent edit gets no protection. Not synthesized (accepting the field with no check would be worse than dropping it — a false sense of safety). (bd: gopherstack-3bsb follow-up)"
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; Reset/Snapshot/Restore cover all state including the 3 dirty tables (comments, files, prApprovalRules). Fixed this pass: DeleteRepository never cleaned up fileHistory[repoName], and never cascade-deleted comments (compared-commit comments by RepoName, PR comments by PRid) or their commentReactions — both are ghost-row leaks now closed (see Notes); locked by TestHandler_DeleteRepository_Cascade_FileHistory and TestHandler_DeleteRepository_Cascade_Comments."}
---

## Notes

### Bugs fixed this pass (2026-08-13, HEAD 1835ab406) — gopherstack-gvkf

1. **The entire `Comment` family (8 ops) returned an undecodable body to any
   typed client.** `models.go` typed `Comment.CreationDate`/`LastModifiedDate`
   as `string`, filled with `time.Now().UTC().Format(time.RFC3339)`
   (`comments.go`) and emitted verbatim by the shared `commentToMap`
   converter (`handler_comments.go`). The real deserializer
   (`codecommit@v1.36.4 deserializers.go:20415,20430`, inside
   `awsAwsjson11_deserializeDocumentComment`) requires a JSON *number* —
   `smithytime.ParseEpochSeconds` — and falls through to `expected
   CreationDate/LastModifiedDate to be a JSON Number, got string instead` on
   anything else. Status 200, body no SDK client could read. Affects
   `PostCommentForComparedCommit`, `PostCommentForPullRequest`,
   `PostCommentReply`, `GetComment`, `GetCommentsForComparedCommit`,
   `GetCommentsForPullRequest`, `UpdateComment` (the 7 ops the bug report
   named), plus `DeleteCommentContent` (an 8th — same `commentToMap` call
   path, found while auditing the rest of the family; its
   `DeleteCommentContentOutput.Comment` shape is identical). This service
   already got the pattern right elsewhere: `Repository`, `PullRequest`, and
   `ApprovalRuleTemplate` all store `time.Time` and convert with `.Unix()` at
   the wire boundary (`handler_repositories.go`, `handler_pull_requests.go`,
   `handler_approval_rules.go`); only `Comment` was missed. Fixed by matching
   that exact pattern rather than inventing a new one: `Comment.CreationDate`/
   `LastModifiedDate` are now `time.Time`, `comments.go` sets them with
   `time.Now().UTC()` (no `.Format`), and `commentToMap` emits `.Unix()`.
   `Comment` is persisted through a DTO (`commentSnapshot` in
   `persistence.go`, not the live struct — see "Traps for the next auditor"
   below), so this is not a wire-format-only change: `commentSnapshot`'s own
   `CreationDate`/`LastModifiedDate` were changed to `time.Time` too, to keep
   `toCommentSnapshot`/`fromCommentSnapshot` a straight field copy.
   `encoding/json` already renders `time.Time` as a quoted RFC3339-ish
   string — the exact shape these fields already held on disk — so an
   existing on-disk snapshot still decodes; `codecommitSnapshotVersion` did
   **not** need to bump. Verified live: reverted the fix, rebuilt the
   container image, and drove all 8 ops through the real `aws-sdk-go-v2`
   client — every one failed with `deserialization failed ... expected
   CreationDate to be a JSON Number, got string instead` (or
   `LastModifiedDate`, depending which field decoded first); reapplied the
   fix and the same 8 calls passed with real decoded timestamps
   (`test/integration/codecommit_test.go`,
   `TestIntegration_CodeCommit_CommentFamily`).

2. **`GetCommentsForComparedCommit`/`GetCommentsForPullRequest` emitted the
   wrong response shape entirely — a flat `[]Comment` instead of the real
   nested wrapper.** The real shape (verified against
   `codecommit@v1.36.4 deserializers.go:20763`/`20883`,
   `awsAwsjson11_deserializeDocumentCommentsForComparedCommit`/
   `...CommentsForPullRequest`) is `[]types.CommentsForComparedCommit` /
   `[]types.CommentsForPullRequest`: each element wraps a nested `comments`
   array plus `repositoryName`/`afterCommitId`/`beforeCommitId` (and
   optional `afterBlobId`/`beforeBlobId`/`location`, not tracked by this
   backend). This emulator's handlers instead put the flat, unwrapped
   `Comment` objects directly under `commentsForComparedCommitData`/
   `commentsForPullRequestData`. Unlike bug 1, this does **not** produce a
   decode error — the JSON-RPC protocol silently drops unrecognized
   top-level keys, so a real client's array element decodes successfully as
   a `CommentsForComparedCommit`/`CommentsForPullRequest` with every field at
   its zero value, including `Comments: nil`. Every comment posted through
   this backend was therefore **silently unreachable** through either list
   op — worse than a hard failure, since nothing in a raw-body or
   status-code check (or even a naive `err == nil` typed-client check) would
   catch it. Fixed: both handlers now group all matching comments into a
   single wrapper object (this backend has no per-location grouping, and
   each query is already scoped to one repository/commit or one pull
   request, so one group is the correct AWS-shaped answer here) with
   `repositoryName`/`afterCommitId` (compared-commit) or `pullRequestId`
   (pull-request, backed by the stored comment's own `RepoName` for
   `repositoryName` when available) always set, `beforeCommitId` set when
   the caller provided it, and the real comments nested under `comments`.
   Verified live the same way as bug 1: with the timestamp fix in place but
   this fix reverted, `TestIntegration_CodeCommit_CommentFamily/get_comments_for_compared_commit`
   and `.../get_comments_for_pull_request` failed on content, not on `err`:
   `group.RepositoryName`/`group.AfterCommitId` decoded as `""` and
   `group.Comments` as `[]` (`"[]" should have 1 item(s), but has 0`) even
   though the call itself returned no error — exactly the silent-data-loss
   signature described above.

3. **Two smaller, related findings, left as documented gaps rather than
   fixed this pass** (out of scope for a decode-correctness bug fix — both
   are shape completeness, not shape correctness): `PostCommentForComparedCommit`
   parses `beforeCommitId` from the request but the backend method signature
   discards it (`comments.go`'s `PostCommentForComparedCommit(repoName, _,
   afterCommitID, content string)`); `PostCommentForPullRequest` similarly
   never stores `afterCommitId`/`beforeCommitId` per-comment. Both Post*
   handlers now *echo* the caller-supplied values back in their top-level
   `afterCommitId`/`beforeCommitId` response fields (real, optional members
   of `PostCommentForComparedCommitOutput`/`PostCommentForPullRequestOutput`
   that were previously omitted entirely), which is honest for the
   just-posted response but means `GetCommentsForPullRequest`'s wrapper (bug
   2's fix) cannot populate `afterCommitId`/`beforeCommitId` on later reads —
   the backend has nowhere to read them back from. Also unaddressed:
   `Comment`'s real wire shape additionally carries `clientRequestToken`,
   `callerReactions`, and `reactionCounts` (`deserializers.go:20362` case
   list), none of which this backend threads through — `reactionCounts`
   would need `commentToMap` to gain backend access (reactions are tracked
   separately in `InMemoryBackend.commentReactions`); `callerReactions`
   presupposes a caller identity concept this backend doesn't model. All are
   optional pointer/map fields in the real shape, so their absence doesn't
   break decode the way bugs 1–2 did.

Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: CodeCommit_20150413.<Op>`.

### Bugs fixed this pass (2026-08-07, HEAD 1d7169f66) — gopherstack-3bsb

1. **`MergeBranchesBySquash`/`MergeBranchesByThreeWay` literally called the
   `MergeBranchesByFastForward` backend method** (`handler_merges.go`) — the
   response looked plausible (a fresh commit ID + branch update) but there
   was zero distinction between the three merge strategies at the object
   level, let alone the content level. Fixed by giving each its own real
   backend method (`merges.go`): both now resolve and validate their
   `sourceCommitSpecifier`/`destinationCommitSpecifier` (new
   `resolveCommitSpecifier` helper — tries a branch name first, then a full
   commit ID, `CommitDoesNotExistException` if neither resolves; previously
   *no* validation existed, so a nonexistent branch/commit silently
   "succeeded"), and produce the correct parent-count shape verified against
   `aws-sdk-go-v2/service/codecommit@v1.33.10`'s generated
   `MergeBranchesBySquashInput`/`MergeBranchesByThreeWayInput`: squash
   creates a commit with exactly one parent (the destination tip — squash
   discards source history), three-way creates a commit with two parents
   (`[destination, source]`, standard merge-commit shape). Both now also
   honor `TargetBranch`/`CommitMessage`/`AuthorName`/`Email` request fields
   (real, wire-verified members of both inputs) that were previously parsed
   by neither handler and silently dropped — a field-drop bug of the same
   class flagged in the audit brief. Content-level squash/3-way merging
   (actually combining file changes) remains unimplemented — see gaps; the
   root cause (flat, non-branch-scoped `File` storage) is unchanged.

2. **`GetMergeConflicts` had an inverted boolean — `mergeable` was hardcoded
   `false`.** This is the sharpest finding of the pass: this backend has no
   content-diff engine and never computes a real conflict, so every merge
   this emulator could ever be asked about is, in fact, mergeable — but the
   handler unconditionally returned `mergeable: false`. A real client that
   polls `GetMergeConflicts` before attempting a merge (a documented,
   common pattern) would have concluded every merge was blocked and never
   proceeded, against a backend that would have happily merged in every
   case. Fixed: now returns `true`. Also fixed: `sourceCommitId`/
   `destinationCommitId` in the response echoed the raw request specifier
   string instead of a resolved commit ID (wrong per
   `GetMergeConflictsOutput`'s doc: "the commit ID ... used in the merge
   evaluation"), and the op performed **zero** input validation —
   `repositoryName`/`sourceCommitSpecifier`/`destinationCommitSpecifier`/
   `mergeOption` are all `"This member is required"` per the real SDK's
   `validateOpGetMergeConflictsInput`, none were checked. Both fixed using
   the same `resolveCommitSpecifier` helper and validation pattern as
   `BatchDescribeMergeConflicts`'s existing (correct) handler.

3. **`SameFileContentException`/`ErrSameFileContent` was declared and wired
   into `errCodeLookup` but no backend path ever returned it** (flagged as a
   gap by the 2026-07-23 pass). Fixed for the buildable half: `PutFile` and
   `CreateCommit`'s `putFiles` entries now compare new content against the
   existing blob at that path (`bytes.Equal`) and reject an identical write
   before mutating any state, matching AWS's documented behavior ("the
   content of the file you're trying to add is exactly the same as the
   content of that file ... you specified as the parent commit"). This
   surfaced and required fixing two existing unit tests
   (`TestHandler_ListFileCommitHistory_Pagination`/`_TableDriven`) that had
   been writing byte-identical content across multiple commits to build
   history — itself a real client-unrepresentative test pattern (a real
   AWS client doing this would already get `SameFileContentException`),
   now writes distinct content per commit. `FilePathConflictsWithSubmodulePathException`
   remains unreturned — no submodule concept exists in this backend to
   build a conflict check on top of (see gaps).

Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: CodeCommit_20150413.<Op>`.
`RouteMatcher` checks the header prefix only (no body sniffing needed since it's a single
endpoint) — verified every op in `GetSupportedOperations()` has a `buildOps()` entry and
vice versa via `sdk_completeness_test.go`.

### Bugs fixed this pass (2026-07-12, HEAD 2ca17ef1)

1. **`DescribeMergeConflicts` was a disguised no-op** (`handler_ops.go`,
   `handleDescribeMergeConflicts`). It unmarshaled the request and echoed
   `destinationCommitSpecifier`/`sourceCommitSpecifier`/`filePath` straight back into the
   response with hardcoded `mergeHunks: []` and `numberOfConflicts: 0` — it never called
   into the backend, never validated required fields, and never checked the repository
   existed (a request against a nonexistent repository returned 200 OK). Its sibling
   `BatchDescribeMergeConflicts` did all of this correctly. Fixed by validating the same
   required fields (mirroring the batch handler) and delegating to
   `Backend.BatchDescribeMergeConflicts` with a single-element `filePaths`, translating the
   first (only) conflict entry into the single-file response shape.

2. **`PutFile`/`DeleteFile`/`CreateCommit` responses hardcoded `blobId: ""`** even though
   the backend generates and stores a real blob ID for every file write
   (`backend.go`'s `applyFileChanges`, `backend_ops.go`'s `PutFile`). AWS's
   `PutFileOutput.BlobId`, `DeleteFileOutput.BlobId`, and
   `CreateCommitOutput.filesAdded[].blobId` are all **required** response fields (verified
   against `aws-sdk-go-v2/service/codecommit@v1.33.10`'s generated types) — a client that
   calls `PutFile` then immediately `GetBlob(blobId)` (a common pattern to verify what was
   written) would get `BlobIdDoesNotExistException` against this emulator before this fix.
   `PutFile`/`DeleteFile`/`CreateCommit`/`applyFileChanges` now all return the real blob
   ID(s) they generate, and the handlers thread them into the response.

3. **`DeleteFile` never checked the file existed.** Deleting a path that was never added
   silently fabricated a "delete" commit and updated the branch tip, instead of returning
   AWS's `FileDoesNotExistException`. Fixed: `DeleteFile` now looks up the file first and
   returns `ErrFileNotFound` if absent (also lets it recover the real blob ID for the
   response — see #2).

4. **Six "not found" error paths returned the wrong AWS exception type.** `GetFile`,
   `GetBlob`, all 6 comment ops (`GetComment`, `GetCommentReactions`, `PostCommentReply`,
   `PutCommentReaction`, `UpdateComment`, `DeleteCommentContent`), and the 2 PR-approval-rule
   ops (`DeletePullRequestApprovalRule`, `UpdatePullRequestApprovalRuleContent`) all reused
   the generic `ErrNotFound` sentinel, which `handleError` maps to
   `RepositoryDoesNotExistException` — so e.g. `GetFile` on a missing path inside an
   *existing* repository returned "repository not found", which is both the wrong
   exception type (breaks SDK-side `errors.As`/type-switch handling) and a misleading
   message. Added dedicated sentinels (`ErrFileNotFound` →
   `FileDoesNotExistException`, `ErrBlobNotFound` → `BlobIdDoesNotExistException`,
   `ErrCommentNotFound` → `CommentDoesNotExistException`, `ErrApprovalRuleNotFound` →
   `ApprovalRuleDoesNotExistException`), all verified against real exception type names in
   `aws-sdk-go-v2/service/codecommit/types/errors.go`. `handleError`'s dispatch was
   refactored from an 18-branch `switch` (tripped `cyclop`) into a table
   (`errCodeLookup`) so future sentinel additions don't grow its cyclomatic complexity.

### Bugs fixed this pass (2026-07-23, HEAD aabde46b5)

No commits touched `services/codecommit/` between the prior audit (`2ca17ef1`) and this
one (`git log 2ca17ef1..HEAD -- services/codecommit/` is empty), so the prior pass's "ok"
entries were re-verified rather than re-derived from scratch; the bugs below are new
findings from field-diffing the file/commit/merge-conflict/pagination surface against
`aws-sdk-go-v2/service/codecommit@v1.33.10`'s generated (de)serializers.

1. **`ListFileCommitHistory`'s `revisionDag` was the wrong wire shape entirely.** Each
   entry was a flattened `Commit` map (`commitId`/`treeId`/`message`/...) instead of AWS's
   `FileVersion` shape (`blobId`/`path`/`commit`/`revisionChildren`, verified against
   `awsAwsjson11_deserializeDocumentFileVersion` in the SDK's `deserializers.go`) — a real
   SDK client's `FileVersion` deserializer would find no nested `commit` object and no
   `blobId`/`path` at all. Fixed by having the backend return `[]FileVersionEntry`
   (`models.go`) built from `fileHistory`, with `revisionChildren` computed as a linear
   chain (this backend has no branch-aware file versioning — see the `gaps` entry on merge
   conflicts for why) over the *unpaginated* history so a page boundary never truncates a
   still-valid child reference.

2. **`PutFile`/`DeleteFile` never recorded `fileHistory` at all** — only `CreateCommit`'s
   `applyFileChanges` did. Since `PutFile` is the primary single-file write path, any file
   added that way was invisible to `ListFileCommitHistory` when queried by `filePath`
   (silently fell through to the always-empty-history branch). Fixed: both ops now call the
   same `recordFileHistory` helper `CreateCommit` uses; `fileHistory`'s value type changed
   from `[]string` (bare commit IDs) to `[]FileHistoryEntry` (commit ID + blob ID pairs, the
   blob ID needed for bug #1's `FileVersion.BlobId`) — `codecommitSnapshotVersion` bumped
   `1 -> 2` since this changes a persisted table's value shape.

3. **`PutFile` stored the branch name, not the commit ID, in `File.CommitSpecifier`.**
   `GetFile`'s `commitId` response field is documented as "the full commit ID of the commit
   that contains the content" — after a `PutFile("repo", "main", ...)`, `GetFile` returned
   `"main"` in that field instead of a real commit ID. `CreateCommit`'s `applyFileChanges`
   already did this correctly (`CommitSpecifier: commitID`); `PutFile` now generates its
   commit ID before constructing the `File` row and uses it the same way.

4. **`GetDifferences` used lowercase pagination field names; AWS uses capitalized ones for
   this one op.** Verified against the SDK's generated
   `awsAwsjson11_serializeOpDocumentGetDifferencesInput` /
   `awsAwsjson11_deserializeOpDocumentGetDifferencesOutput`: `MaxResults`/`NextToken` (both
   request and response), unlike every other paginated op in this service which uses
   lowercase `maxResults`/`nextToken`. Since the handler used lowercase, a real SDK client's
   pagination requests were silent no-ops (always the first page). Fixed the field names and
   implemented real pagination via `pkgs/page` (closing the `GetDifferences pagination`
   deferred item); `ListFileCommitHistory` pagination (also previously deferred) was
   implemented the same way with the correct lowercase names for that op.

5. **`DeleteFile` accepted and silently ignored `parentCommitId`** (a documented gap).
   Real AWS's `DeleteFileInput.ParentCommitId` is a **required** field (verified via the
   SDK's `validateOpDeleteFileInput` in `validators.go`, which client-side rejects a nil
   value before ever sending a request) and must be the branch's current HEAD. Fixed:
   `DeleteFile` now returns `ParentCommitIdRequiredException` when empty and
   `ParentCommitIdOutdatedException` when non-empty but stale, mirroring the check
   `CreateCommit` already performs for its own `parentCommitId`.

6. **`CreateCommit`'s `filesDeleted` entries never carried a `blobId`**, unlike
   `filesAdded` (fixed in the prior pass). `FileMetadata.BlobId` (the type both arrays use)
   is optional but informative — mirroring the fix already applied to the standalone
   `DeleteFile` op (which does report the removed blob), `applyFileChanges` now also
   returns the blob ID each `deleteFiles` entry removed, and the handler threads it through.

7. **Six error sentinels were declared but missing from `errCodeLookup`, making them
   unreachable.** `ErrBranchNameRequired`, `ErrInvalidBranchName` (both actively returned by
   `validateBranchName`, used by `CreateBranch`), `ErrParentCommitIDRequired`,
   `ErrParentCommitIDOutdated` (returned by `CreateCommit`, and now `DeleteFile` — see #5),
   and the still-unused `ErrSameFileContent`/`ErrFilePathConflicts` (see `gaps`) were all
   absent from the table `handleError` looks up in `handler.go`. Every error using one of
   the four *active* sentinels fell through to a generic 400 `ValidationException` instead
   of its real, SDK-matching exception name — meaning `CreateCommit` with a stale
   `parentCommitId` has been returning the wrong exception type since before this pass, an
   `errCodeLookup` gap the prior pass's own refactor (which introduced the table) didn't
   catch because nothing exercised that path. All six now map to their real AWS exception
   name (still 400, matching this table's existing all-client-errors-are-400 convention).

8. **`DeleteRepository` leaked `fileHistory[repoName]` and every comment (+ reactions)
   belonging to the repository.** `fileHistory` was never touched by the cascade at all.
   Comments have no secondary index (`comments` is a "dirty" table — see the trap below) and
   `GetComment(commentId)` does a pure by-ID lookup with no repository check, so a comment
   survived its repository's deletion as a permanently-reachable ghost row; the same was
   true of pull-request comments when their PR was cascade-deleted. Fixed:
   `deleteCommentsForRepo`/`deleteCommentsForPR` helpers (`repositories.go`) sweep
   `comments`/`commentReactions` by `RepoName`/`PRid`, and `fileHistory[repoName]` is now
   `delete()`-d in the same cascade. Locked by
   `TestHandler_DeleteRepository_Cascade_{Comments,FileHistory}`.

### 2026-08-23 — merges.go finally opened, plus CreateRepository

The 2026-08-23 request-side sweep (see `services/codecommit/handler_files_sdk_roundtrip_test.go`'s
`PutFile`/`DeleteFile` fixes) left a strong but explicitly unconfirmed lead: five merge ops
looked like they dropped `authorName`/`commitMessage`/`email` the same way PutFile/DeleteFile
did, but `merges.go` itself was never opened. This pass opened it and checked each of the five
against the pinned SDK (codecommit@v1.36.4) individually rather than trusting the precedent:

1. **`MergeBranchesBySquash`/`MergeBranchesByThreeWay` — REFUTED.** Both already decode
   `authorName`/`commitMessage`/`email` via the shared `mergeBranchesRequest` struct and pass
   them through `MergeBranchesOptions` into the backend, which sets `Commit.AuthorName`/
   `AuthorEmail`/`Message` correctly. This was fixed by the 2026-08-07 pass (see below); the
   lead's own pattern-match caught these as false positives had it read the file.
2. **`CreateUnreferencedMergeCommit` — CONFIRMED, fixed this pass.** Decode struct had none of
   the three fields; backend hardcoded `Message: "Unreferenced merge commit"` and never set
   `AuthorName`/`AuthorEmail`, despite those being real tracked `Commit` fields populated
   correctly elsewhere (`CreateCommit`, the two ops above). Real-client proof:
   `Test_SDKRoundTrip_CreateUnreferencedMergeCommit_CommitMetadata` — creates a repo, calls
   `CreateUnreferencedMergeCommit` with `CommitMessage`/`AuthorName`/`Email` set, reads the
   result back via `GetCommit`. Confirmed failing against unfixed code (quoted below), fixed,
   hand-reverted via `cp` and reconfirmed failing identically, restored byte-identical (`md5sum`
   matched). Fix: `merges.go`'s `CreateUnreferencedMergeCommit` gained three trailing string
   params (`authorName, authorEmail, message`); `handler_merges.go`'s decode struct gained
   `authorName`/`email`/`commitMessage` JSON tags.
   ```
   Error: Not equal:
     expected: "unref merge result"
     actual  : "Unreferenced merge commit"
   ```
3. **`MergePullRequestBySquash`/`MergePullRequestByThreeWay` — MODELLING GAP, not a bug.** Both
   drop the same three fields, but unlike every op above, neither backend method creates a
   `Commit` at all — they only flip `PullRequestStatus`/`LastActivityDate`. There is no commit
   object for an author/message to land on. Real AWS's `MergePullRequestBySquashOutput` doesn't
   even return author/message directly; that data lives in `PullRequestTarget.MergeMetadata`
   (`MergeCommitId`/`MergedBy`/`IsMerged`, `types.go:936`), a struct gopherstack's
   `PullRequestTarget` doesn't model. See `gaps` above; not synthesized (a decode-only fix with
   no backend consumer would be exactly the no-op stub `parity-principles.md` rule 1 forbids).

**`CreateRepository` — found independently while widening the scan past the five merge ops
(28 of codecommit's 30 raw candidates had never been read), CONFIRMED, fixed this pass.**
`createRepositoryInput` dropped `CreateRepositoryInput.KmsKeyId`; `Repository.KmsKeyID` is a
real tracked field (correctly populated by `UpdateRepositoryEncryptionKey`, exercised by
`wire_encryption_key_test.go`) but `CreateRepository` itself never set it. A repository created
with a customer KMS key always reported an empty `kmsKeyId`. Real-client proof:
`Test_SDKRoundTrip_CreateRepository_KmsKeyId` — creates a repo with `KmsKeyId` set, reads it
back via `GetRepository`. Confirmed failing against unfixed code, hand-reverted and
reconfirmed, restored byte-identical. Fix: `CreateRepository`'s backend signature gained a
`kmsKeyID string` param (all in-package callers, including five test call sites in
`handler_test.go`/`persistence_test.go`, updated); `createRepositoryInput` gained a
`KmsKeyID json:"kmsKeyId"` field.

Also checked and ruled real gaps, not bugs (see `gaps` above): `UpdateApprovalRuleTemplateContent`/
`UpdatePullRequestApprovalRuleContent` drop `ExistingRuleContentSha256` — `RuleContentSha256` is
tracked, but no comparison logic or `InvalidRuleContentSha256Exception` equivalent exists
anywhere in this backend, so the precondition concept itself was never built, not merely
wired to the wrong field.

Not reached this pass (named, not implied covered): the remaining unread portion of
codecommit's 30 raw candidates beyond the 5 merge ops + `CreateRepository` — specifically
`PostCommentForComparedCommit`/`PostCommentForPullRequest`/`PostCommentReply`'s
`ClientRequestToken`/`Location` (checked; `Comment` has no such fields at all — likely the
same class of modelling gap as `ExternalIds`/`Extensions` elsewhere in this campaign, but not
verified against `InvalidRuleContentSha256Exception`-style error-set evidence the way the sha256
gap above was), and every List/Get op's dropped `MaxResults`/`NextToken`/filter-only params
(`GetCommentsForPullRequest`, `GetCommentReactions`, `DescribePullRequestEvents`'s `ActorArn`/
`PullRequestEventType`, `ListFileCommitHistory`'s `CommitSpecifier`, `GetDifferences`'s
`AfterPath`/`BeforePath`) — these narrow what a real client could filter/paginate by rather than
returning wrong data for the un-filtered case, a materially different (and lower-severity) risk
profile than the four confirmed-or-refuted findings above, and were triaged by comparison against
the pinned SDK's Input structs but not each individually proven against a real client.

### Traps for the next auditor

- `MergeBranchesBySquash`/`MergeBranchesByThreeWay` (fixed 2026-08-07 to be real, distinct
  backend methods — see Notes) now have correct object-graph shape (parent count, branch
  update, honored `TargetBranch`/`CommitMessage`/`AuthorName`/`Email`) but still don't
  implement content-level squash/3-way merging — this remains a known, documented gap (see
  `gaps` above), not something newly introduced. The analogous
  `MergePullRequestBySquash`/`MergePullRequestByThreeWay` were NOT touched (out of this
  pass's scope; they only flip PR status, no commit/branch mutation at all, so there was no
  parent-count distinction to fix there). The root cause of the remaining content-level gap
  is precisely identified (not just "no diff engine"): `File` has no per-branch or
  per-commit identity at all (flat `repoName|filePath` key, see `fileKey`), so there is no
  second version of a file to diff against in the first place. Don't re-flag without also
  proposing the branch/commit-scoped file-tree rework needed to fix it for real (large
  feature, not a bug fix).
- `BatchDescribeMergeConflicts`'s own doc comment says "stub implementation" — that refers
  to the fact it never finds real conflicts (no content diffing), not that it's unwired;
  it does validate real backend state (repo existence) and is exercised correctly by both
  `BatchDescribeMergeConflicts` and `DescribeMergeConflicts`.
- `TestRepositoryTriggers` always reporting every trigger as a `successfulExecution` is
  *correct* emulator behavior, matching real AWS (which also doesn't actually invoke SNS
  and always reports success in the common case) — do not "fix" this into a stub-detector
  false positive.
- Comment/File/PullRequestApprovalRule tables are the 3 "dirty" tables (not on
  `b.registry`) persisted via DTOs in `persistence.go` — if you add a field to `Comment`,
  `File`, or `PullRequestApprovalRule`, you must also update the matching DTO
  (`commentSnapshot`/`fileSnapshot`/`prApprovalRuleSnapshot`) or it silently won't persist.
- `fileHistory`'s value type is `[]FileHistoryEntry` (commit ID + blob ID), not the bare
  `[]string` of commit IDs it was before this pass — if you touch it again, remember
  `codecommitSnapshotVersion` must bump whenever its shape changes again (see the comment on
  the constant in `persistence.go`).
- `errCodeLookup` (`handler.go`) is checked by test coverage now (#7 above added tests for
  the four previously-unreachable-but-active entries), but there's no automated check that
  every sentinel declared in `errors.go` has a table entry — a new `awserr.New(...)`
  sentinel with no matching `errCodeLookup` row will silently fall through to generic
  `ValidationException` again. Diff `errors.go`'s `Err*` declarations against
  `errCodeLookup`'s `sentinel:` entries by hand if you add or suspect one.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 2 confirmed bugs

- `CreateApprovalRuleTemplate`/`GetApprovalRuleTemplate`/
  `UpdateApprovalRuleTemplateContent`: {wire: fixed} -- approvalRuleTemplateToMap
  emitted "approvalRuleTemplateArn"; types.ApprovalRuleTemplate has no ARN
  member -- templates are identified by approvalRuleTemplateId only.
- `TestRepositoryTriggers`: {wire: fixed} -- wrapped each successful trigger
  name in a `{"triggerName": ...}` object; real member
  (TestRepositoryTriggersOutput.SuccessfulExecutions) is `[]string`.

Both proven via real `aws-sdk-go-v2/service/codecommit` client round trips
(wire_field_fixes_y1zn_test.go), hand-reverted/confirmed-failing/restored/
`md5sum`-verified byte-identical.

## 2026-08-23 request-side accept-and-drop (gopherstack-n3zi)

PutFile and DeleteFile dropped commitMessage, name and email; PutFile also
dropped fileMode and parentCommitId. All body-bound, all real members of
PutFileInput (api_op_PutFile.go:56-77). Commit.Message was hardcoded to
"Add "+path / "Delete "+path, and Commit.AuthorName/AuthorEmail -- fields that
exist and are populated correctly by CreateCommit (commits.go:122-127) -- were
never set. Proven by real-SDK-client round trip failing pre-fix with
expected "initial import", actual "Add hello.txt".

NOT AUDITED, LIKELY THE SAME BUG: CreateUnreferencedMergeCommit,
MergeBranchesBySquash, MergeBranchesByThreeWay, MergePullRequestBySquash and
MergePullRequestByThreeWay all show the same missing authorName, commitMessage
and email in the same scan. merges.go was never opened. Treat as unconfirmed.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

`paginateStrings` (`handler.go` — backs `ListBranches`, `ListPullRequests` via the shared
helper) verified clean: boundary walk, exact division, single-page, empty, cursor-past-end,
and negative/malformed-token handling all correct (`pagination_arithmetic_internal_test.go`).
No bug found.

**Adjacent finding, not a bug:** `handleListRepositories` (`handler_repositories.go`)
hand-rolls an identical copy of `paginateStrings`'s arithmetic instead of calling it — because
`paginateStrings` is typed `[]string`, not generic, and `ListRepositories` paginates a `[]Repository`
slice. The duplicated arithmetic was read and is itself correct (same clamp,
`if start > len(repos) { start = len(repos) }`, present). Separately: the real AWS
`ListRepositoriesInput`/`ListBranchesInput` wire shapes have **no `MaxResults` field at all**
(fixed internal batch size per AWS's own docs) — gopherstack's `maxResults` JSON field on
both is emulator-internal and unreachable from a real SDK client, which can only ever get one
unbounded page from either op. `ListPullRequests`, which *does* carry `MaxResults` on the real
wire, was used for the SDK-level pagination proof instead
(`pagination_sdk_roundtrip_test.go`).

Gates: `go build`, `go vet` (repo-wide), `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/codecommit/...`).

## 2026-08-30 anonymous-struct-decode sweep (gopherstack-4a8v)

`cmd/reqfieldscan` gained a fifth dispatch shape (handlers implementing
`service.JSONOpFunc` directly, decoding into anonymous inline structs, no
`WrapOp` anywhere) that made real findings newly visible in this service.
Dispatch coverage: 78/79 (99%), literal-decode-only and WrapOp-resolved
lines identical; no coverage-guard warning. The one unresolved op
(`ListApprovalRuleTemplates`) is a pre-existing dispatch-resolution gap
unrelated to this campaign, not investigated here (out of scope: no
request-body fields to flag on an op the scanner can't even reach).

12 fields flagged, hand-verified against `codecommit@v1.36.4`'s own `Input`
structs and serializers:

- **6 real bugs, fixed** (see `ops:` notes above for each): `TestRepositoryTriggers`
  (tested saved triggers instead of the request's own, required list — the
  dominant "parsed parameter never passed on" shape this campaign keeps
  finding); `CreateUnreferencedMergeCommit.mergeOption` (required, never
  validated or forwarded); `GetMergeCommit.mergeOption` (fabricated — not a
  real `GetMergeCommitInput` member at all, deleted); `revisionId` on
  `GetPullRequestApprovalStates`, `GetPullRequestOverrideState`,
  `OverridePullRequestApprovalRules`, `UpdatePullRequestApprovalState`,
  `EvaluatePullRequestApprovalRules` (all five require it per the SDK, none
  validated it — one fix covering 5 ops).
- **1 false positive** (4 flagged fields): `mergeBranchesRequest.{TargetBranch,
  CommitMessage,AuthorName,Email}` — a SIXTH tool blind spot, distinct from
  the five already known: the scanner's `collectLocalBindings` only binds a
  function's own parameters and `:=`/`=` locals, never a method *receiver*
  (`func (r mergeBranchesRequest) options()`). All four fields are read via
  `r.FieldName` inside that receiver method. Reported per the campaign's
  "report, don't patch the tool" instruction rather than fixed here.

**Explicitly checked and not found this pass:** no handler discarding its
entire request body; no listing found to skip its own store outside the
one already-fixed case (TestRepositoryTriggers, which *was* skipping its
own request in favor of stored state — arguably this shape rather than
"parameter never passed on", noted here since it doesn't cleanly fit either
bucket); the "missing existence check" shape (empty result vs. real
not-found) — RevisionID's own required-field gap is closer to a
validation gap than an existence check, and a genuine staleness/mismatch
check (InvalidRevisionIdException/RevisionNotCurrentException) was
deliberately NOT added, see the `ops:` notes, to avoid inventing which
error code an unmodeled mismatch should map to; no "required field
dropped, no wire member to put it in" case beyond what's covered above; no
list consumed only at its first element; no value-semantics/timestamp-format
mismatch found in this scan's flagged set.

**redshiftdata was NOT touched this pass** — see its own PARITY.md's
2026-08-30 note; the campaign's own spot-check verdict for it (genuine bug)
did not survive re-verification against redshiftdata's existing, dated
PARITY.md entries.

Tests: `TestHandler_CreateUnreferencedMergeCommit_MergeOptionRequired`,
`TestHandler_CreateUnreferencedMergeCommit_InvalidMergeOption` (new,
`handler_merges_test.go`); `TestHandler_RevisionIDRequired` (new, 5
subtests, `handler_pull_request_approvals_test.go`);
`TestHandler_TestRepositoryTriggers` (corrected, was asserting the old
wrong behavior — now asserts the request's own triggers are tested, plus a
new empty-request-with-saved-triggers case) and
`TestTestRepositoryTriggers_SuccessfulExecutionsIsStringArray_RealClient`
(corrected the same way, in `wire_field_fixes_y1zn_test.go`) — both
hand-confirmed failing against unmodified code before the fix. No other
existing test assertions were weakened; 0 dropped.

Gates: `go build`, `go vet` (repo-wide), `go test -race -count=1`,
`golangci-lint run` — all clean (`./services/codecommit/...`).

## 2026-08-30 value-semantics filter sweep (gopherstack-uox6's class)

Audited codecommit's filter-bearing List/Describe/Merge operations for the
class this bd issue tracks: a documented filter/status semantic that is
read and applied but wrong, invisible to every field-shape or enum-legality
scan. `ListPullRequests`'s `pullRequestStatus`/`authorArn` filters, its
sort/pagination, and every declared-but-dropped filter param already
recorded in this file's "Not reached this pass" section were re-checked and
found clean or already correctly recorded.

### 1 bug found and fixed: fabricated "MERGED" pull request status

`aws-sdk-go-v2/service/codecommit@v1.36.4`'s `types.PullRequestStatusEnum`
has exactly two members, `OPEN` and `CLOSED` — confirmed directly in
`types/enums.go`. `UpdatePullRequestStatusInput.PullRequestStatus`'s own doc
comment is explicit: "The only valid operations are to update the status
from OPEN to OPEN, OPEN to CLOSED or from CLOSED to CLOSED." A merge is a
terminal CLOSED, distinguished from an explicit close only via
`PullRequestTarget.MergeMetadata` (`MergeCommitId`/`MergedBy`/`IsMerged`,
`types.go:936` — a struct this backend doesn't model at all, per the
existing 2026-08-23 note above).

`MergePullRequestByFastForward`/`BySquash`/`ByThreeWay` (`merges.go`) set
`pr.PullRequestStatus = "MERGED"` instead of `"CLOSED"` — a value no real
AWS CodeCommit response ever emits and no real SDK client's
`PullRequestStatusEnum` can express. This corrupted `ListPullRequests`'
`pullRequestStatus` filter specifically: a real client filtering for
`CLOSED` (the only way to ask for terminal PRs, since `MERGED` isn't a
legal filter value either) would see merged PRs in real AWS but got none
back from this backend, because their stored status never matched `CLOSED`.
`handleListPullRequests` additionally accepted `"MERGED"` as a valid filter
value to request them by — a value a real SDK client's typed enum could
never send.

Fixed: all three merge operations now set `PullRequestStatus = "CLOSED"`;
`ListPullRequests`' validation now accepts only `OPEN`/`CLOSED` (error
message updated); the now-redundant `== prStatusMerged` guard clauses in
`pull_requests.go` (blocking mutation of an already-terminal PR) collapsed
to the `prStatusClosed` check alone; the now-unused `prStatusMerged`
constant removed from `handler.go`. `UpdatePullRequestStatus`'s existing,
correct rejection of an explicit `"MERGED"` input status (it's not a legal
transition target either) is untouched.

**Known, not fixed — out-of-scope, cosmetic-only:** `ui/src/routes/codecommit/+page.svelte:222`
has a status-badge color mapping for the literal string `'MERGED'`, which
can now never match. Outside this pass's `services/codecommit/` scope and
not a Go caller broken by a signature change (no signature changed), so not
touched; the badge will fall through to whatever color the mapping uses for
an unrecognized status. Flagged for whoever next touches that page.

Test: `TestHandler_ListPullRequests_ClosedFilterIncludesMerged` (new,
`handler_pull_requests_test.go`), hand-confirmed failing against unmodified
code before the fix (merged status `"MERGED"` instead of `"CLOSED"`, empty
`CLOSED`-filtered list instead of one match).
`TestHandler_MergePullRequest_StatusBecomesmerged` (renamed
`...StatusBecomesClosed`) and the three merge-response assertions in
`handler_merges_test.go` were asserting the bug (`"MERGED"`) and are now
corrected to assert `"CLOSED"` — 4 assertions changed, 0 dropped. The three
`UpdatePullRequestStatus` validation tests asserting `"MERGED"` is REJECTED
as an explicit input status are unrelated (that rejection was already
correct) and untouched.

### Other filters checked, no bug

- `ListPullRequests`' `pullRequestStatus`/`authorArn`: neither field
  documents behaviour on absence beyond "if used, this refines the
  results" — no omission-default language exists on either field
  (`api_op_ListPullRequests.go`), so empty-means-no-filter is correct as
  implemented.
- `ListRepositories`' `sortBy`/`order`: real `ListRepositoriesInput`
  documents no default for either enum; the emulator's
  repositoryName-ascending default is a reasonable, undocumented choice,
  not a contradiction of documented behaviour.
- `GetDifferences`' dropped `beforeCommitSpecifier` (structurally unfixable
  — no per-commit file tree exists to diff against, see the existing
  `gopherstack-3bsb` note above) and `GetDifferences`/`DescribePullRequestEvents`/
  `GetCommentsForPullRequest`/`GetCommentReactions`/`ListFileCommitHistory`'s
  other dropped filter params are the OTHER axis (never read/applied at
  all) — already correctly recorded in this file's "Not reached this pass"
  section; re-confirmed present, not re-litigated as this class's bug.

Gates: `go build`, `go vet` (repo-wide, clean), `go test -race -count=1`,
`golangci-lint run` (0 issues) — all clean (`./services/codecommit/...`).
Work left uncommitted per this pass's instructions.

## 2026-09-07 errtargetaudit sweep (gopherstack-8pe4)

No prior error-envelope/errtargetaudit entry existed for this service (screened before
starting). `cmd/errtargetaudit` (declared-vs-emitted error-code cross-check against
codecommit@v1.36.4's `deserializeOpError<Op>` tables) reported: operations with SDK ground
truth 79, resolved 79, emission found for 76, no coverage warning; 3 class A findings.

### 1. CreateCommit / SameFileContentException — CONFIRMED, fixed

`awk "/deserializeOpErrorCreateCommit\(/,/^}/" deserializers.go | grep -oE '"[A-Za-z0-9]+"'`
lists CreateCommit's entire declared error set (35 codes) — `SameFileContentException` is
not among them; `NoChangeException` is ("The commit cannot be created because no changes
will be made to the repository as a result of this commit", per the real API docs' Errors
section). `SameFileContentException`'s own doc ("The file was not added or updated because
the content of the file is exactly the same...") is PutFile's exception specifically, and
PutFile's declared set does contain it (confirmed the same way) — PutFile's own identical-
content check (`files.go:48`) was untouched, it already used the right code. CreateCommit's
`putFiles`-identical-content check (`commits.go`, added by the 2026-08-07 gopherstack-3bsb
pass) reused PutFile's `ErrSameFileContent` sentinel for a different op with a different
declared error set — a global-sentinel-map-shaped bug (gopherstack-hdvu's class), fixed per
call site: new `ErrNoChange` sentinel (`errors.go`) wired to `NoChangeException` in
`errCodeLookup`, used only by CreateCommit's check; `ErrSameFileContent`/PutFile untouched.

### 2. GetCommit / CommitDoesNotExistException — CONFIRMED, fixed

Same extraction for GetCommit's declared set: `CommitIdDoesNotExistException`,
`CommitIdRequiredException`, plus repo/encryption/invalid-id codes — no
`CommitDoesNotExistException`. The real API docs' GetCommit Errors section lists
`CommitIdDoesNotExistException` — "The specified commit ID does not exist" — a distinct
type from `CommitDoesNotExistException` ("The specified commit does not exist or no commit
was specified, and the specified repository has no default branch", confirmed via
`types/errors.go`'s doc comments on both). The shared `ErrCommitNotFound` sentinel
(`errors.go`) is correctly `CommitDoesNotExistException` for every other caller
(`branches.go`'s `CreateBranch`, `merges.go`'s `resolveCommitSpecifier` users —
`BatchDescribeMergeConflicts`/`CreateUnreferencedMergeCommit`/`DescribeMergeConflicts`/
`GetCommentsForComparedCommit`/the merge family — all confirmed against their own declared
sets, all correct, untouched); only GetCommit's own by-ID lookup (`commits.go`) needed the
other exception. Fixed per call site, not per sentinel: new `ErrCommitIDNotFound` sentinel
wired to `CommitIdDoesNotExistException`, used only by GetCommit's not-found return.

### 3. BatchGetCommits / CommitDoesNotExistException — FALSE POSITIVE (class 1: batch data)

The flagged line (`commits.go`) is a composite-literal `BatchCommitError.ErrorCode` field
inside the per-`commitId` `errors` list of a 200 response — document data, not a thrown
exception, so it has no entry in `deserializeOpErrorBatchGetCommits` at all (confirmed: that
op's declared set has only `CommitIdsLimitExceededException`/`CommitIdsListRequiredException`
plus the usual repo/encryption codes — no commit-not-found code of either name). Not changed:
the real docs for `BatchGetCommitsError.errorCode` don't enumerate valid string values (unlike
the typed exceptions above), so there's no positive evidence the current string is wrong —
only that GetCommit's sibling by-ID lookup uses the `...Id...` variant, which is suggestive
but not proof for a free-string field with no declared enum. Left as a documented open
question (see `ops:` above) rather than a synthesized remedy.

### Verification

Per-line neuter: reverted `commits.go`'s `ErrNoChange` (CreateCommit) back to
`ErrSameFileContent` — compiled, `TestHandler_CreateCommit_NoChange` failed on the asserted
`__type` (`"NoChangeException"` vs `"SameFileContentException"`), restored. Reverted
`ErrCommitIDNotFound` (GetCommit) back to `ErrCommitNotFound` — compiled,
`TestHandler_GetCommit_CommitIDNotFound` failed the same way (`"CommitIdDoesNotExistException"`
vs `"CommitDoesNotExistException"`), restored.

Pre-existing test correction: `TestHandler_CreateCommit_SameFileContent` (`handler_commits_
test.go`) asserted `SameFileContentException` for CreateCommit's identical-content rejection
— the exact bug fixed above, with no note that it pinned wrong behavior. Renamed
`TestHandler_CreateCommit_NoChange`, assertion corrected to `NoChangeException`, comment added
citing this issue. 1 test corrected, 0 dropped. New regression test:
`TestHandler_GetCommit_CommitIDNotFound` (no pre-existing test asserted GetCommit's not-found
`__type` at all).

Re-ran `cmd/errtargetaudit` after the fix: codecommit's class A findings dropped from 3 to 1
(the BatchGetCommits false positive above; unchanged, not a regression).

Gates: `go build`, `go test -race -count=1 ./services/codecommit/...` (ok),
`golangci-lint run services/codecommit/...` (0 issues).

## 2026-09-07 BatchGetCommits errorCode value resolved (gopherstack-pfyr)

Follow-up to section 3 above, which correctly left the *value* question open
(the finding itself was, and remains, a confirmed class-1 false positive —
not re-litigated here).

Evidence: `codecommit@v1.36.4`'s `api-2.json` types `GetCommitInput.commitId`
and `BatchGetCommitsError.commitId` both as the `ObjectId` shape (a raw,
required, full-SHA lookup with no specifier resolution). `CreateBranchInput.
commitId` and every other operation whose declared error set includes
`CommitDoesNotExistException` (`CreateBranch`, `BatchDescribeMergeConflicts`,
`CreateUnreferencedMergeCommit`, `DescribeMergeConflicts`,
`GetCommentsForComparedCommit`, `GetCommentsForPullRequest`, `GetDifferences`,
`GetFile`, `GetFolder`, `GetMergeCommit`, `GetMergeConflicts`,
`GetMergeOptions`, `ListFileCommitHistory`, the three `MergeBranchesBy*` ops,
`PostCommentForComparedCommit`, `PostCommentForPullRequest`) instead use the
distinct `CommitId` shape, reserved for specifier-resolution fields (branch
tips, before/after commit, merge base) per `docs-2.json`'s `refs`. `GetCommit`
is the only operation in the service whose declared error set has
`CommitIdDoesNotExistException` — confirmed against `deserializeOpErrorGetCommit`
and `api-2.json`'s per-op `errors` list.

The live API reference for `BatchGetCommits` corroborates independently: its
`errors[]` doc text — "if one of the commit IDs was a shortened SHA ID or that
commit was not found in the specified repository" — names exactly the two
failure modes GetCommit's own declared set distinguishes as
`InvalidCommitIdException` and `CommitIdDoesNotExistException`, not the
specifier-resolution condition `CommitDoesNotExistException` describes ("no
commit was specified, and the specified repository has no default branch").

`BatchGetCommitsError.errorCode` itself still has no enumerated values in
either `docs-2.json` or the live API reference — confirmed again this pass,
no new information there. The type-shape identity to `GetCommit` is what
settles it, not the errorCode field's own doc.

Fixed: `commits.go`'s `BatchGetCommits` not-found entry now uses
`CommitIdDoesNotExistException`. New regression test
`TestHandler_BatchGetCommits_ErrorCodeIsCommitIdDoesNotExist`
(`handler_commits_test.go`) — failed pre-fix with `expected:
"CommitIdDoesNotExistException" / actual: "CommitDoesNotExistException"`, now
passes. No pre-existing test asserted this field's value, so nothing else
changed.

Gates: `golangci-lint run ./services/codecommit/...` (0 issues), `go test -race
./services/codecommit/...` (ok).

## 2026-09-08 actorArn caller-identity audit (gopherstack-a7tx)

Filed as "codecommit: no caller-identity plumbing, so actorArn filters cannot
work" (P3, title-only, empty description). The premise was imprecise:
`pkgs/awsmeta` + `services/iam`/`services/sts` do resolve caller identity, and
that resolution is wired repo-wide, not per-service -- cli.go's
`awsMetaMiddleware` (`e.Pre`, applies to every request) and `principalMiddleware`
(`registry.Use`, applies to every registered service including codecommit)
populate `awsmeta.CallerArn(ctx)` before any service's dispatch ever runs. The
real defect was narrower and local to this package: codecommit's own
`dispatch` discarded that ctx (`func (h *Handler) dispatch(_ context.Context,
...)`), and `OverridePullRequestApprovalRules` -- the one op that records a
`PullRequestEvent` -- hardcoded its actor to `""` even though its backend
signature already had an `overriderARN` parameter to receive it
(`handler_pull_requests.go`, pre-fix: `h.Backend.OverridePullRequestApprovalRules(req.PullRequestID,
req.OverrideStatus, "")`). Confirmed via `OverridePullRequestApprovalRulesInput`
(codecommit@v1.36.4 api_op_OverridePullRequestApprovalRules.go / botocore
service-2.json): it has no actor/overrider ARN field at all -- real AWS derives
"who overrode" purely from caller identity, exactly what `awsmeta.CallerArn`
already resolves.

`DescribePullRequestEventsInput.ActorArn` ("The Amazon Resource Name (ARN) of
the user whose actions resulted in the event. Examples include updating the
pull request with more commits or changing the status of a pull request.",
api_op_DescribePullRequestEvents.go:36-39, confirmed against botocore's
longer `service-2.json` doc string) was silently dropped: the decode struct
in `handleDescribePullRequestEvents` had no `ActorArn` field, so a
client-supplied filter was ignored outright (not merely unvalidated -- never
read). `actorArn`'s wire shape is `Arn` (an unconstrained string in the
model), but the operation's declared error set includes
`InvalidActorArnException` ("Make sure that you have provided the full ARN
...") and `ActorDoesNotExistException` ("does not exist in the Amazon Web
Services account").

Fixed (in scope -- uses existing plumbing, no new identity mechanism, no
cross-service wiring):
- `PullRequestEvent` gained an `ActorARN` field (`models.go`).
- `OverridePullRequestApprovalRules` now stores its (already-passed)
  `overriderARN` onto the event it creates (`pull_requests.go`).
- `dispatch` now special-cases `OverridePullRequestApprovalRules` to call it
  with `ctx` (rather than growing the `ops` table's function type to
  `func(context.Context, []byte)` across all 79 other ops, out of proportion
  to a one-op fix) and `handleOverridePullRequestApprovalRules` extracts
  `awsmeta.CallerArn(ctx)` in place of the old `""` literal.
- `handleDescribePullRequestEvents` parses `actorArn`, validates it as an ARN
  shape (new `ErrInvalidActorArn` / `InvalidActorArnException`, `errors.go`'s
  `actorArnRe`), and both `DescribePullRequestEvents` (backend) and the wire
  response now filter/echo by it.

NOT fixed, and explicitly out of scope for this pass:
- `ActorDoesNotExistException` -- checking that an ARN names a real account
  principal would require coupling codecommit to IAM's user/role store
  cross-service, which the audit brief excluded.
- 8 of the 9 real `PullRequestEventType` values are never recorded by any
  backend operation -- only `PULL_REQUEST_APPROVAL_RULE_OVERRIDDEN` is (the
  sole `b.prEvents[prID] = append(...)` call site in the package, before this
  pass). actorArn filtering is consequently only observable against that one
  event type today; this is a pre-existing structural gap in event recording,
  independent of caller identity, and was not widened or narrowed by this fix.

Verdict: (i) -- the plumbing exists and codecommit simply wasn't using it, so
this was a fixable defect, not a structural blocker. Not a pure case of
"just call `awsmeta.CallerArn(ctx)` at filter time", though: `actorArn`
attributes each *past* event to whoever performed *that* action, which meant
capturing identity at event-creation time (already ctx-available at
`dispatch`), not at query time.

New regression test `TestOverridePullRequestApprovalRules_RecordsCallerAsActorArn`
(`actor_arn_test.go`), 4 subtests, all failing pre-fix:
- `matches_caller`: expected the recorded event's `actorArn` to equal the
  caller's ARN, got `<nil>` (field absent).
- `no_filter_returns_all`: same `<nil>` failure (event existed but carried no
  actor).
- `different_actor_returns_none`: expected 0 events filtering by a different
  actor, got 1 (filter param didn't exist, so nothing was ever excluded).
- `malformed_arn_rejected`: expected HTTP 400 / `InvalidActorArnException`,
  got HTTP 200 with the (unfiltered) event list.
All 4 pass post-fix. One pre-existing call site updated for the new backend
signature (`persistence_test.go`'s `fresh.DescribePullRequestEvents(pr.PullRequestID,
"", "")`, third arg added) -- no assertion changed, purely mechanical.

Stability: new test run 10x under `-race -count=1` (all pass), full package
run 5x under `-race -count=1` (all pass). No global/shared state involved
(fresh backend/handler per test, no package-level mutable state touched).

Gates: `go build ./services/codecommit/...` (clean), `go vet
./services/codecommit/...` (clean), `golangci-lint run
./services/codecommit/...` (0 issues), `go test -race
./services/codecommit/...` (ok).
