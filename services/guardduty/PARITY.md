---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: guardduty
sdk_module: aws-sdk-go-v2/service/guardduty@v1.85.4
last_audit_commit: ca2732322
last_audit_date: 2026-08-29
overall: A            # 2026-09-08 (gopherstack-uu0n): DeleteMembers/DisassociateMembers/
                       # StopMonitoringMembers's autoEnableOrganizationMembers=ALL guard
                       # (gopherstack-krb1) rejected the whole call whenever the detector's org config was
                       # ALL, regardless of whether the requested accounts were still in the AWS
                       # Organization -- over-rejecting an account that had already left, contrary to
                       # DisassociateMembers' own doc text ("you'll receive an error if you attempt to
                       # disassociate a member account before removing them from your organization" --
                       # conditioned on membership, not merely on the ALL setting; DeleteMembers/
                       # StopMonitoringMembers/DisassociateFromMasterAccount/
                       # DisassociateFromAdministratorAccount's doc text is the same shape, confirmed against
                       # both the aws-sdk-go-v2 doc comments and botocore's guardduty/2017-11-28/service-2.json
                       # `documentation`/`errors` fields -- BadRequestException/InternalServerErrorException
                       # only, an operation-level error, not a per-account UnprocessedAccounts reason).
                       # krb1 deliberately left this unfixed, reasoning Member has no still-in-org-vs-left
                       # field and nothing would ever set one. That was true of GuardDuty's own state but
                       # missed a repo-wide mechanism already used by 5 other services (mgn, grafana,
                       # resiliencehub, ec2, codedeploy pre-existing; ec2/grafana are the pattern's origin):
                       # a lazy siblingServices cross-service lookup (SetAppConfig(ctx.Config), matched
                       # structurally against *CLI) that reaches another emulator's backend on demand. Wired
                       # guardduty into it (new cross_service.go, provider.go's backend.SetAppConfig call) to
                       # reach services/organizations' StorageBackend.DescribeAccount, which already reliably
                       # answers still-in-org-vs-left with no new write path needed:
                       # RemoveAccountFromOrganization (services/organizations/accounts.go) deletes the
                       # account from that backend's table, so DescribeAccount returning ErrAccountNotFound
                       # after removal already means "left" today. No new field on Member; no
                       # organization-departure path invented -- the fix computes membership live against the
                       # existing Organizations backend at guard-check time instead of caching a flag that
                       # could go stale. New rejectOrgMembersStillInOrg (members.go) replaces the old
                       # whole-detector autoEnableOrgMembersAll short-circuit at all three call sites,
                       # rejecting only when at least one *requested* account is confirmed still in the org;
                       # when no Organizations backend is wired (e.g. guardduty exercised standalone, as most
                       # of this package's tests do) membership can't be confirmed and the call is allowed,
                       # matching real AWS never erroring over an account whose org status it can't determine
                       # as still-a-member. Regression test
                       # (cross_service_test.go's TestMemberOps_AutoEnableOrganizationMembersAll_
                       # AllowedAfterAccountLeavesOrg) wires a real services/organizations backend, proves
                       # rejection while the account is still in the org, then
                       # RemoveAccountFromOrganization's and proves the same call now succeeds with the
                       # account processed (not left in unprocessedAccounts) -- for all three ops. The
                       # pre-existing krb1 regression test (members_test.go's
                       # TestMemberOps_AutoEnableOrganizationMembersAll_Rejected) previously asserted
                       # rejection with no Organizations backend wired at all; under the corrected semantics
                       # that is now an "unknown membership" case that must be allowed, so it was updated to
                       # wire an Organizations backend and keep the account in it, continuing to prove the
                       # case its own doc-text justification actually describes (still-a-member) rather than
                       # a case the fix now correctly stops rejecting. Grade unchanged at A: this was a
                       # correctness bug in an already-implemented guard, not a new gap.
                       # --- history below predates this pass ---
                       # 2026-08-29 (constraint-not-honoured sweep, wrapper-key-sweep-rds-cloudwatch-sqs-sns
                       # branch): MaxResults/NextToken (real HTTP query params on every op below, verified
                       # per-op against aws-sdk-go-v2/service/guardduty@v1.85.4's
                       # awsRestjson1_serializeOpHttpBindings<Op>Input encoder.SetQuery calls) were never
                       # read at all -- bug class "never read", not a wrong-key miswire -- across 10
                       # operations: ListFilters, ListIPSets, ListThreatIntelSets, ListThreatEntitySets,
                       # ListTrustedEntitySets, ListPublishingDestinations, ListOrganizationAdminAccounts,
                       # ListMalwareProtectionPlans (NextToken only -- no MaxResults on this op's real wire),
                       # ListInvitations, and ListMembers (onlyAssociated was already read correctly by a
                       # prior pass; MaxResults/NextToken were not). Every one of these dispatcher functions
                       # (dispatchFilterOps/dispatchIPSetOps/dispatchThreatIntelSetOps/dispatchInvitationOps/
                       # dispatchOrgOps/dispatchPublishingDestOps/dispatchEntitySetOps) simply had no `query`
                       # parameter at all -- the exact "binding trap" shape this class's own brief warns
                       # about (a shared query string available at the dispatch() call site but never
                       # threaded down to the op that needed it) -- so every real client's MaxResults/
                       # NextToken silently no-op'd and the full unpaginated set came back in one response
                       # every time, for every one of these 10 ops. Fixed by threading `query` through each
                       # dispatcher, adding a shared paginationParamsFromQuery(query) helper (pagination.go,
                       # replacing the near-identical malwareScanPageParamsFromQuery this pass consolidated),
                       # and wiring each backend List method through the pre-existing paginate/decodeToken
                       # helpers already used correctly by ListFindings/DescribeMalwareScans/ListMalwareScans/
                       # ListInvestigations. Page-size cap: every one of these ops' own doc comment states
                       # (or, for ListPublishingDestinations/ListOrganizationAdminAccounts, the AWS API
                       # reference confirms) a 50-item default/max, consolidated into one standardPageSize
                       # const (pagination.go) after golangci-lint's unparam flagged the prior per-family
                       # constants as parameterizing a value that never varied; ListMalwareProtectionPlans
                       # is the one exception (100-per-page, no MaxResults on its wire at all) and bypasses
                       # that helper entirely, using its own fixed-size paginate call.
                       # ListDetectors (also declares MaxResults/NextToken, also never read) is the
                       # deliberate exception NOT fixed: this backend enforces "one detector per
                       # account/region" (CreateDetector returns ErrDetectorAlreadyExists past the first),
                       # matching real AWS's own limit, so ListDetectors can never return more than one item
                       # -- NextToken can never be non-empty regardless of implementation, making pagination
                       # here structurally unobservable, not merely unimplemented (same class as the "two
                       # pagination gaps... because at most two or three values can ever exist" precedent).
                       # ListCoverage/GetCoverageStatistics (also declare FilterCriteria/SortCriteria/
                       # MaxResults/NextToken, also never read at all -- handleListCoverage doesn't even take
                       # a body/query parameter) are a second deliberate exception, for a different reason:
                       # this backend has NO coverage-resource tracking model whatsoever (no store table, no
                       # write path from any op) -- ListCoverage always returns an empty list and
                       # GetCoverageStatistics always returns empty count maps, unconditionally. Filtering or
                       # paginating an always-empty result is unobservable by construction; building a real
                       # EKS/ECS/EC2 coverage-resource model to make this observable is a structural gap far
                       # outside this pass's scope, reported here rather than fabricated. FindingCriteria/
                       # SortCriteria on ListFindings, and FilterCriteria/SortCriteria on
                       # DescribeMalwareScans/ListMalwareScans, were independently re-verified this pass and
                       # found already correct (matchesFindingCriteria/matchesMalwareScanFilter apply real
                       # per-op enum vocabularies -- e.g. malware scans' EC2_INSTANCE_ARN on DescribeMalwareScans
                       # vs RESOURCE_ARN on ListMalwareScans, both wired to the same ResourceArn field via one
                       # shared matcher -- not a re-fix, no bug found). Every fix proven via
                       # wire_field_fixes_test.go, driving the real typed aws-sdk-go-v2/service/guardduty
                       # client, asserting a second page returns the remainder and NextToken round-trips (not
                       # merely that a matching item is present); confirmed failing against unmodified code
                       # first.
                       # RE-AUDITED 2026-08-11 (doc-only catch-up pass, no code changes): ca2732322 fixed
                       # real bugs -- DescribeMalwareScans/ListMalwareScans now honour FilterCriteria/
                       # SortCriteria/MaxResults/NextToken (verified: matchesMalwareScanFilter is applied
                       # in both DescribeMalwareScans and ListMalwareScans, malware_protection.go), ListMembers
                       # now honours onlyAssociated (verified: members.go:172), eight member ops
                       # (DeleteMembers/GetMembers/InviteMembers/StartMonitoringMembers/
                       # StopMonitoringMembers/DisassociateMembers/GetMemberDetectors/
                       # UpdateMemberDetectors) now reject an unknown DetectorId instead of silently
                       # succeeding (verified: members.go, each has an `if !b.detectors.Has(detectorID)`
                       # guard), and GetRemainingFreeTrialDays now computes a real per-account value
                       # under AccountFreeTrialInfo's actual shape (features[].freeTrialDaysRemaining,
                       # not a top-level field -- verified against types.go/api_op_GetRemainingFreeTrialDays.go)
                       # instead of a hardcoded 30. That commit never touched this file. This pass only
                       # updates the record to match: refreshed the affected op rows, added three op rows
                       # that were missing outright (ListMalwareScans, GetMemberDetectors,
                       # UpdateMemberDetectors -- present in the family notes but never had their own row),
                       # and recorded the still-open pagination gap precisely (ten plain-GET List ops, named
                       # below) and ListCoverage's inert filter honestly (no coverage-resource state exists to
                       # filter over -- implementing the filter would be plumbing over a permanently-empty
                       # list, not real filtering, so it is deliberately NOT implemented). No op's grade
                       # changed as a result of this pass; overall stays A.
                       # --- history below predates this pass ---
                       # this pass (parity-4, SDK bump 1.78.2 -> 1.85.0): implemented the one new op family
                       # the bump revealed -- investigations (CreateInvestigation/GetInvestigation/
                       # ListInvestigations, GuardDuty Extended Threat Detection). Wire shapes, detector
                       # validation, AI_ANALYST feature gating, and cascade delete are all real and
                       # field-diffed against the installed SDK. Everything audited in prior passes (see
                       # history below) is unchanged.
                       # RE-AUDITED 2026-07-30 (parity-5 grade-floor pass, no code changes): confirmed
                       # the driver is a genuine missing capability -- this backend has no AI/ML
                       # threat-analysis engine anywhere, so RiskLevel/Confidence/Summary/Title/
                       # related-findings on an Investigation can never be real data. STRUCTURAL, grade
                       # correctly held at A-, not raised.
                       # RE-GRADED 2026-08-05: schema now distinguishes structural gaps (no data source
                       # can ever exist) from ordinary gaps (buildable with more effort). The
                       # investigation-analysis limitation is genuinely structural -- moved to
                       # structural_gaps below, which does not block A. Everything else in the old gaps
                       # list is a buildable missing state model, not structural, and stays in gaps
                       # (also non-blocking, per this repo's existing grading rule). Raised A- -> A.
                       # 2026-08-21 (gopherstack-1vv2): fixed UpdateMalwareProtectionPlan wholesale-
                       # replacing stored ProtectedResource with the narrower Update payload,
                       # destroying bucketName (immutable after Create, so a real client's Update
                       # can never resend it). See the UpdateMalwareProtectionPlan_state op row.
                       # 2026-08-28 (gopherstack-6flj/21my wrapper-key + per-item sweep): two real
                       # bugs found and fixed. (1) GetUsageStatistics.sumByDataSource emitted the
                       # detector's enabled Feature names (S3_DATA_EVENTS, EKS_AUDIT_LOGS, ...)
                       # verbatim under the "dataSource" key, but types.DataSource is a DIFFERENT,
                       # six-member enum (FLOW_LOGS/CLOUD_TRAIL/DNS_LOGS/S3_LOGS/
                       # KUBERNETES_AUDIT_LOGS/EC2_MALWARE_SCAN) that does not contain
                       # "S3_DATA_EVENTS" or "EKS_AUDIT_LOGS" at all -- every enabled
                       # S3_DATA_EVENTS/EKS_AUDIT_LOGS feature produced an invalid DataSource
                       # value on the wire. Fixed via a real feature->DataSource map plus the
                       # three always-on base sources. sumByFeature was unaffected (UsageFeature's
                       # enum really does share the DetectorFeature names). See
                       # TestGetUsageStatistics_SumByDataSource_RealDataSourceValues. (2)
                       # ListMalwareProtectionPlans emitted an invented "arn" key on each summary
                       # entry -- types.MalwareProtectionPlanSummary has exactly one member,
                       # malwareProtectionPlanId; arn is real only on the singular
                       # GetMalwareProtectionPlanOutput. Fixed by dropping arn from the list
                       # summary. See TestListMalwareProtectionPlans_NoInventedArn (raw-body, since
                       # the typed SDK summary struct has no field to decode arn into). Full
                       # wrapper-key + per-item sweep of every List/Describe/Get-collection op
                       # otherwise came back clean (detectors/filters/ipSets/threatIntelSets/
                       # threat+trustedEntitySets/tags/members/memberDetectors/invitations/
                       # adminAccounts/publishingDestinations/malwareScans(both shapes)/coverage/
                       # investigations/findingsStatistics/organizationStatistics/
                       # freeTrialDays -- field-diffed per-op against guardduty@v1.85.4's
                       # deserializers.go, not against this file's prior claims).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  UpdateMalwareProtectionPlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — was routed on POST, real SDK sends PATCH (the one GuardDuty op that isn't POST/GET/DELETE); was unroutable by a real client despite green unit tests that called h.Handler() directly, bypassing RouteMatcher/method dispatch. 2026-08-13 (gopherstack-jqh2 pass 2): re-extracted all 90 ops' real method+path from guardduty@v1.85.4 serializers.go independently and drove them through ExtractOperation via the new handler_sdk_route_table_test.go (TestExtractOperation_SDKRouteTable, full 90/90 coverage complementing the existing ~45-op handler_route_matcher_test.go regression suite) -- all 90 resolved correctly, no new routing bugs found."}
  DescribePublishingDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — wire key was publishingFailureStartedAt (invented), real key is publishingFailureStartTimestamp; tags were never returned despite CreatePublishingDestination now accepting them"}
  CreatePublishingDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — did not accept/store tags at all; real CreatePublishingDestinationInput.Tags is honored now"}
  GetThreatEntitySet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — also now returns expectedBucketOwner when set at Create/Update time (was accepted nowhere on the real Create/UpdateThreatEntitySetInput shapes despite this backend having a field for it); real ErrorDetails is correctly always-absent since this backend never sets status ERROR"}
  GetTrustedEntitySet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — same expectedBucketOwner gap as GetThreatEntitySet"}
  CreateThreatEntitySet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — now accepts+stores expectedBucketOwner (real CreateThreatEntitySetInput.ExpectedBucketOwner)"}
  CreateTrustedEntitySet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — same as CreateThreatEntitySet"}
  UpdateThreatEntitySet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — now accepts expectedBucketOwner"}
  UpdateTrustedEntitySet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — same as UpdateThreatEntitySet"}
  GetMalwareProtectionPlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — createdAt was a bare time.Time (RFC3339 string on the wire); real GetMalwareProtectionPlanOutput.CreatedAt is epoch seconds"}
  GetMalwareScan: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass, was partial) — now emits adminDetectorId/resourceArn/resourceType/scanCategory/scannedResourcesCount/skippedResourcesCount/failedResourcesCount (all present, real defaults for a RUNNING scan this backend can't fully simulate: 0 counts); scanStatusReason/scanCompletedAt correctly omitted while RUNNING (only present once a scan actually completes/fails, which this backend's scans never transition to). Still absent: scanConfiguration, scanResultDetails, scannedResources[] detail list — no state exists to populate these meaningfully (see gaps)"}
  StartMalwareScan: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — real bug: never set MalwareScan.DetectorID, so DescribeMalwareScans (which filters scan.DetectorID == detectorID) silently always returned []. StartMalwareScanInput carries no detectorId (GuardDuty resolves the caller's own detector server-side), so this backend now resolves it the same way CreateDetector enforces \"one detector per Region\": attaches the scan to whichever single detector exists for the account, if any. resourceType is now inferred from the resource ARN's service/resource segments (EC2_INSTANCE/EBS_SNAPSHOT/EBS_VOLUME/EC2_AMI/S3_BUCKET). FIXED 2026-08-21 (gopherstack-tp8x item 6) — ScanType was hardcoded GUARDDUTY_INITIATED for every scan this op creates; a customer-invoked StartMalwareScan is ON_DEMAND (types.ScanType, enums.go:1642), not an automatic GuardDuty-triggered scan, so it now sets ON_DEMAND. Also stopped fabricating MalwareScan.TriggerDetails under a made-up nested \"scanTriggerDetails\"/\"scanInitiatedAt\" wrapper no real client reads — real types.TriggerDetails (types.go:4969) is flat {description,guardDutyFindingId,triggerType} and TriggerType only has BACKUP/GUARDDUTY (enums.go:1798), neither of which applies to an on-demand scan with no backing finding, so it is now correctly omitted, same reasoning as fileCount/scanResultDetails below. Verified via TestStartMalwareScan_DescribeMalwareScans_RealClient (real SDK client, fails against pre-fix code)."}
  DescribeMalwareScans: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable in practice due to the StartMalwareScan DetectorID bug above; now returns scans started against the queried detector, locked by TestMalwareScanning/describe_malware_scans_includes_started_scan. FIXED (ca2732322) — filterCriteria/sortCriteria/maxResults/nextToken were previously parsed into MalwareScanQuery but never reached the backend at all (DescribeMalwareScans took only a detectorID); now genuinely filters via matchesMalwareScanFilter (SCAN_ID/ACCOUNT_ID/SCAN_STATUS/SCAN_TYPE/EC2_INSTANCE_ARN/RESOURCE_ARN/RESOURCE_TYPE/SCAN_START_TIME criterion keys, GUARDDUTY_FINDING_ID correctly never matches since scans are never correlated to findings), sorts via sortMalwareScans, and paginates (default/max page size 50, matching the doc). Response shape also split from ListMalwareScans' (see below) into the real, richer types.Scan shape via scanToDescribeMap. FIXED 2026-08-21 (gopherstack-tp8x item 6) — scanToDescribeMap no longer emits the fabricated triggerDetails shape (see StartMalwareScan note); real types.Scan.TriggerDetails is correctly always absent now, not invented."}
  ListMalwareScans: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — previously had no ops row here despite existing; the handler discarded its request body outright and returned every scan globally, unfiltered and unpaginated. Now parses filterCriteria/sortCriteria from the body and maxResults/nextToken from the query string (real ListMalwareScansInput carries MaxResults/NextToken as query params, not body fields — see malwareScanPageParamsFromQuery, field-diffed against serializers.go's awsRestjson1_serializeOpHttpBindingsListMalwareScansInput), applies the same matchesMalwareScanFilter as DescribeMalwareScans, and emits the narrower real types.MalwareScan shape (scanId/scanStatus/scanType/scanStartedAt/resourceArn/resourceType — no accountId/detectorId/triggerDetails/resourceDetails, which types.MalwareScan genuinely lacks) via scanToListMalwareScansMap, previously incorrectly identical to DescribeMalwareScans' shape"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — now propagates into the owning resource's own Tags field (see families.tags below), not just the generic map"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (prior pass) — same propagation as TagResource"}
  CreateDetector: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDetector: {wire: ok, errors: ok, state: ok, persist: ok, note: "createdAt/updatedAt are ISO8601 strings on this op specifically (GetDetectorOutput.CreatedAt/UpdatedAt are *string, not epoch) — do not \"fix\" this to epoch, it is already correct and differs deliberately from ThreatEntitySet/TrustedEntitySet/MalwareProtectionPlan"}
  UpdateDetector: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDetector: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-deletes every detector-nested table + tags; verified via persistence_test.go"}
  ListDetectors: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFilter: {wire: ok, errors: ok, state: ok, persist: ok, note: "real GetFilterOutput has no createdAt/updatedAt — correctly omitted"}
  UpdateFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFilters: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "Finding.CreatedAt/UpdatedAt correctly plain strings (Finding is a \"string\" shape member on the real API, not a timestamp shape)"}
  ListFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass, was gap) — findingCriteria is now evaluated (Equals/NotEquals/GreaterThan(OrEqual)/LessThan(OrEqual)/Matches/NotMatches + deprecated Eq/Neq/Gt/Gte/Lt/Lte aliases, resolved against dot-path attributes like service.archived via finding_criteria.go); sortCriteria.attributeName/orderBy honored (defaults: id ASC, matching prior behavior when unset); maxResults/nextToken now paginate for real (default/max page size 50, matching the real doc). See finding_criteria_test.go"}
  ArchiveFindings: {wire: ok, errors: ok, state: ok, persist: ok, note: "real mutation: sets Service.Archived + UpdatedAt, verified by reading GetFindings after"}
  UnarchiveFindings: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateSampleFindings: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFindingsStatistics: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (this pass, was partial) — groupBy=ACCOUNT/DATE/FINDING_TYPE/RESOURCE/SEVERITY now each return the correct real groupedByX list (finding_statistics.go), selected exclusively (matching \"if a groupBy was provided\" semantics — the deprecated countBySeverity is omitted whenever groupBy is set, and vice versa); findingCriteria now filters which findings are aggregated; maxResults honored (default 25, matching the real doc). groupByResource's resourceId is always \"\" — this backend has no per-resource-type identifier field (instanceId/functionName/etc), only resourceType, which is a real, documented limitation not a bug. CORRECTED 2026-08-30 (gopherstack-4a8v): the prior note's \"maxResults honored\" claim was true only of findingStatisticsFor's own default-25 fallback logic — handleGetFindingsStatistics (handler_findings.go) built FindingStatisticsQuery{GroupBy, OrderBy} without ever threading req.MaxResults through, so a real client's own requested cap silently no-op'd and every call got the unconditional default regardless. Fixed by adding MaxResults to that literal. See TestGetFindingsStatistics_MaxResults (wire_field_fixes_test.go)."}
  UpdateFindingsFeedback: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateIPSet: {wire: ok, errors: ok, state: ok, persist: ok}
  GetIPSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "real GetIPSetOutput has no createdAt/updatedAt — correctly omitted"}
  UpdateIPSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteIPSet: {wire: ok, errors: ok, state: ok, persist: ok}
  ListIPSets: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateThreatIntelSet: {wire: ok, errors: ok, state: ok, persist: ok}
  GetThreatIntelSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "real GetThreatIntelSetOutput has no createdAt/updatedAt — correctly omitted"}
  UpdateThreatIntelSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteThreatIntelSet: {wire: ok, errors: ok, state: ok, persist: ok}
  ListThreatIntelSets: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateMembers: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — accepted an unknown DetectorId and returned 200 with every account listed unprocessed, instead of ResourceNotFoundException; now checks b.detectors.Has(detectorID) first, same as CreateMembers/ListMembers"}
  GetMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — same missing detector-existence check as DeleteMembers, now fixed"}
  InviteMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "real relationshipStatus transition Created->Invited verified. FIXED (ca2732322) — same missing detector-existence check as DeleteMembers, now fixed"}
  ListMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — onlyAssociated was hardcoded false regardless of the real onlyAssociated query param (ListMembersInput's real wire binding, see serializers.go), so a caller asking for associated-only members received everyone; now parsed via onlyAssociatedFromQuery and passed through (backend-side filtering at members.go:172 already existed and was simply never wired to the query string)"}
  StartMonitoringMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — same missing detector-existence check as DeleteMembers, now fixed"}
  StopMonitoringMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — same missing detector-existence check as DeleteMembers, now fixed"}
  DisassociateMembers: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — same missing detector-existence check as DeleteMembers, now fixed"}
  GetMemberDetectors: {wire: ok, errors: ok, state: ok, persist: ok, note: "no ops row here previously despite existing. FIXED (ca2732322) — same missing detector-existence check as DeleteMembers, now fixed. FIXED (gopherstack-lx5h) — response emitted memberDataSources; real required key (deserializers.go GetMemberDetectorsOutput switch) is members, mapping to MemberDataSourceConfigurations. Prior wire: ok was false"}
  UpdateMemberDetectors: {wire: ok, errors: ok, state: ok, persist: ok, note: "no ops row here previously despite existing. FIXED (ca2732322) — same missing detector-existence check as DeleteMembers, now fixed"}
  DeleteMalwareProtectionPlan: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMalwareProtectionPlans: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-28, gopherstack-21my) — each summary entry emitted an invented arn key; real types.MalwareProtectionPlanSummary has exactly one member (malwareProtectionPlanId). arn is real only on the singular GetMalwareProtectionPlanOutput. See TestListMalwareProtectionPlans_NoInventedArn"}
  CreateMalwareProtectionPlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass) — protectedResource.s3Bucket.bucketName is now required and validated (BadRequestException if absent/empty), matching CreateMalwareProtectionPlanInput.ProtectedResource being a required member and \"Presently, S3Bucket is the only supported protected resource\"; actions.tagging.status is now validated against the real MalwareProtectionPlanTaggingActionStatus enum (ENABLED/DISABLED) instead of being passed through unchecked. See malware_protection_plan_schema.go + malware_protection_plan_schema_test.go"}
  UpdateMalwareProtectionPlan_state: {wire: ok, errors: ok, state: ok, persist: fixed, note: "FIXED (this pass) — actions.tagging.status now validated the same way as Create (UpdateMalwareProtectionPlanInput.Actions is the same types.MalwareProtectionPlanActions shape). protectedResource is NOT bucketName-validated on Update — real UpdateProtectedResource/UpdateS3BucketResource carries no bucketName member at all (a plan's bucket can't be renamed), only objectPrefixes. 2026-08-21 (gopherstack-1vv2): persist was accept-and-corrupt, not just under-validated — UpdateMalwareProtectionPlan wholesale-replaced the stored ProtectedResource map with the client's payload, and since a real client's payload can only ever carry s3Bucket.objectPrefixes, every real Update call silently erased bucketName. Fixed to merge only objectPrefixes into the existing ProtectedResource (mergeProtectedResourceObjectPrefixes, malware_protection.go), preserving bucketName. See TestUpdateMalwareProtectionPlan_PreservesBucketName."}
  GetOrganizationStatistics: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass, was deferred/gap) — real GetOrganizationStatisticsOutput wraps everything under organizationDetails (types.OrganizationDetails), which itself carries updatedAt (epoch seconds) alongside organizationStatistics — both were missing entirely; now present. activeAccountsCount/totalAccountsCount/memberAccountsCount/enabledAccountsCount are now computed from the real members table (not orgAdminAccounts, a distinct concept — delegated administrators, not member accounts). countByFeature remains always [] — this backend tracks no per-feature enrollment counts across member accounts (see gaps)"}
  GetUsageStatistics: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (this pass, was deferred/gap) — real UsageStatistics is sumByAccount/sumByDataSource/sumByFeature/sumByResource/topAccountsByFeature/topResources, each entry a Total{amount,unit} object; the old response had a bare ad hoc field set (no Total wrapper, no sumByFeature/topAccountsByFeature, a placeholder \"topResources\" that didn't match the real shape). usageStatisticType is now honored — only the requested field is populated, the rest omitted, per the real doc (\"the objects representing other types will be null\"). sumByFeature/sumByDataSource/topAccountsByFeature now reflect the detector's actually-ENABLED features. Every Total.amount is a deterministic \"0.00\" placeholder — this backend has no real cost-metering model, which is an honest limitation (correct shape, no fabricated numbers), not a bug. FIXED (2026-08-28, gopherstack-6flj) — sumByDataSource reused the detector's Feature names (S3_DATA_EVENTS, EKS_AUDIT_LOGS, ...) verbatim under the dataSource key, but types.DataSource is a distinct six-member enum (FLOW_LOGS/CLOUD_TRAIL/DNS_LOGS/S3_LOGS/KUBERNETES_AUDIT_LOGS/EC2_MALWARE_SCAN, types/enums.go) that has no S3_DATA_EVENTS/EKS_AUDIT_LOGS member -- every enabled S3-data-events or EKS-audit-logs feature produced an invalid DataSource value on the wire. Now derived from usageDataSourceNames (usage.go): the three always-on base sources plus a real feature->DataSource map (S3_DATA_EVENTS->S3_LOGS, EKS_AUDIT_LOGS->KUBERNETES_AUDIT_LOGS); features with no DataSource equivalent (EBS_MALWARE_PROTECTION, RDS_LOGIN_EVENTS, LAMBDA_NETWORK_LOGS, EKS_RUNTIME_MONITORING, RUNTIME_MONITORING, AI_PROTECTION, AI_ANALYST) are correctly never reported under dataSource. sumByFeature was unaffected -- UsageFeature's real enum genuinely shares the DetectorFeature names. See TestGetUsageStatistics_SumByDataSource_RealDataSourceValues"}
  GetRemainingFreeTrialDays: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED (ca2732322) — the request's accountIds was ignored outright (every call answered for the detector's own account) and the response put a hardcoded 30 under a top-level freeTrialDaysRemaining field the real AccountFreeTrialInfo shape doesn't have (remainders live per-entry under features[].freeTrialDaysRemaining, verified against types.go). Now resolves each requested accountId against the members table, reports unmatched ones under unprocessedAccounts (real UnprocessedAccount{accountId,result} shape), and computes freeTrialDaysRemaining for the matched ones from Member.UpdatedAt (30 - days elapsed since the member was added, floored at 0) rather than a constant. Still wire: partial, not ok — features[] always reports exactly the three always-on base sources (FLOW_LOGS/CLOUD_TRAIL/DNS_LOGS, all valid FreeTrialFeatureResult enum members); it never reports the account's actually-enabled optional features (S3_DATA_EVENTS, EKS_AUDIT_LOGS, etc.), because this backend tracks no per-member feature-enablement or per-feature enable timestamp, only the detector-level Features a member's OWN detector has. dataSources (deprecated on the real shape) is correctly always omitted, not fabricated. See gaps"}
  GetCoverageStatistics: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass — real GetCoverageStatisticsOutput.CoverageStatistics.countByCoverageStatus/countByResourceType are both maps; this backend tracks no EKS/ECS/EC2 runtime-monitoring coverage resources at all, so both are always {} — that is the CORRECT response for an account with nothing to cover, not a gap. See deferred for the underlying no-coverage-state limitation. Fixed (gopherstack-h910): the required StatisticsType (verified against validateOpGetCoverageStatisticsInput/serializeOpDocumentGetCoverageStatisticsInput's body field 'statisticsType') was dropped entirely and both count maps were always computed and returned regardless of what was requested; now required (BadRequestException if missing/empty) and only the requested count map(s) are present in the response, matching real AWS. FilterCriteria remains unwired -- this backend has no real coverage resources to filter (see ListCoverage), so filtering has nothing to act on; left inert rather than fabricated."}
  ListCoverage: {wire: ok, errors: ok, state: ok, persist: ok, note: "real ListCoverageOutput.Resources is a required []CoverageResource; always [] is correct when no coverage resources are tracked (same reasoning as GetCoverageStatistics), not a fabricated gap. FilterCriteria/SortCriteria are not parsed or applied at all (handleListCoverage ignores the request body entirely) — deliberately NOT implemented: nothing in this backend holds coverage-resource state, so a filter would have nothing to act on but an always-empty list, and wiring it up would read as working filtering while actually being dead plumbing over permanently-[] data. Implementing the filter is worse than the honest gap it would paper over. See gaps"}
  CreateInvestigation: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW (this pass, SDK bump revealed) — POST /detector/{DetectorId}/investigation, field-diffed against api_op_CreateInvestigation.go + serializers.go's awsRestjson1_serializeOpHttpBindingsCreateInvestigationInput/awsRestjson1_serializeOpDocumentCreateInvestigationInput. Validates DetectorId against the real detector table (ResourceNotFoundException, matching every other detector-scoped op) and the real 'AI_ANALYST feature must be enabled on your detector' precondition against Detector.Features (BadRequestException if absent/DISABLED) rather than accepting any detector. triggerPrompt required, matching the real required input member. Response is {investigationId}, matching CreateInvestigationOutput's one member (ResultMetadata aside)"}
  GetInvestigation: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW (this pass) — GET /detector/{DetectorId}/investigation/{InvestigationId}, field-diffed against deserializers.go's awsRestjson1_deserializeDocumentInvestigation. Returns {investigation:{investigationId,status,triggerPrompt,triggeredBy,startTime}}; cloud/confidence/endTime/error/metadata/risk/riskLevel/summary are real *optional* members that only ever populate once analysis runs/completes/fails on the real API -- this backend has no analysis engine so they are correctly omitted always, never fabricated. status is always RUNNING (see investigations family note)"}
  ListInvestigations: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW (this pass) — POST /detector/{DetectorId}/investigation/list, field-diffed against deserializers.go's awsRestjson1_deserializeDocumentInvestigationSummary + serializers.go's awsRestjson1_serializeOpDocumentListInvestigationsInput. sortCriteria.attributeName/orderBy accepted (START_TIME/END_TIME/STATUS/RISK_LEVEL/CONFIDENCE x ASC/DESC); only START_TIME can ever produce a real ordering since every investigation here has identical status/riskLevel/confidence (see investigations family note) -- the others are accepted, not rejected, matching a real client that requests them. maxResults/nextToken paginate for real (default/max page size 50, mirroring ListFindings's documented default since ListInvestigationsInput.MaxResults doesn't document an explicit max either). title is never populated on any summary -- see investigations family note"}
families:
  detector: {status: ok, note: "CRUD + list audited op-by-op above; one-detector-per-account conflict semantics match AWS doc (\"You can have only one detector per Region\")"}
  filter: {status: ok, note: "CRUD + list audited op-by-op above"}
  ipset: {status: ok, note: "CRUD + list audited op-by-op above"}
  threatintelset: {status: ok, note: "CRUD + list audited op-by-op above"}
  findings: {status: ok, note: "FIXED this pass: ListFindings now filters/sorts/paginates for real; GetFindingsStatistics now supports GroupBy. Get/Archive/Unarchive/CreateSample/UpdateFeedback unchanged, still ok"}
  members_invitations: {status: ok, note: "member lifecycle + invitation flows audited; admin/master account relationship handlers (AcceptAdministratorInvitation/AcceptInvitation/GetAdministratorAccount/GetMasterAccount/Disassociate*) read/write real state via the adminAccounts table. FIXED (ca2732322): DeleteMembers/GetMembers/InviteMembers/StartMonitoringMembers/StopMonitoringMembers/DisassociateMembers/GetMemberDetectors/UpdateMemberDetectors now all reject an unknown DetectorId (ResourceNotFoundException) instead of silently returning every account as unprocessed; ListMembers now honours its onlyAssociated query param instead of hardcoding it false"}
  publishingDestination: {status: ok, note: "FIXED wire key + added tags support (prior pass). The 'ExpectedBucketOwner-style extras' gap this family used to carry in this manifest was a MISDIAGNOSIS by a prior audit pass -- confirmed this pass by reading DescribePublishingDestinationOutput/CreatePublishingDestinationInput/DestinationProperties directly: none of them have an ExpectedBucketOwner (or ErrorDetails) member on the real API at all. That field only exists on ThreatEntitySet/TrustedEntitySet (see that family below), which DID have a real gap, now fixed. Removed the bogus gap entry rather than inventing a nonexistent field on PublishingDestination"}
  tags: {status: ok, note: "FIXED (prior pass): TagResource/UntagResource now sync into the owning resource's own frozen Tags field (Detector/Filter/IPSet/ThreatIntelSet/ThreatEntitySet/TrustedEntitySet/MalwareProtectionPlan/PublishingDestination) via syncResourceTagsFromARN in backend.go, so Get*/Describe* no longer show stale tags after a Tag/UntagResource call. Also fixed CreateThreatEntitySet/CreateTrustedEntitySet not writing the generic ARN-keyed tag map at all."}
  threat_trusted_entity_sets: {status: ok, note: "FIXED this pass: expectedBucketOwner is now accepted on Create/Update and returned by Get (real CreateThreatEntitySetInput.ExpectedBucketOwner / GetThreatEntitySetOutput.ExpectedBucketOwner were previously untracked despite this family being marked ok in a prior pass -- a real field-diff miss, corrected here). errorDetails intentionally NOT added -- it is only ever present when status is ERROR, which this backend's entity sets never transition to, so omitting it is correct, not a gap"}
  malware_scan_settings: {status: ok, note: "GetMalwareScanSettings/UpdateMalwareScanSettings wire-verified ok (prior pass). GetMalwareScan is now FULLY fixed this pass (see ops above, was partial) -- family upgraded from partial to ok"}
  organization: {status: ok, note: "FIXED this pass (was deferred): GetOrganizationStatistics now wraps its response in organizationDetails with a real updatedAt and member-derived account counts (see ops above). EnableOrganizationAdminAccount/DisableOrganizationAdminAccount/ListOrganizationAdminAccounts/DescribeOrganizationConfiguration/UpdateOrganizationConfiguration were already field-diffed ok. Remaining limitation: countByFeature is always [] -- no per-feature member enrollment tracking exists in this backend (see gaps), left as a documented gap rather than fabricated"}
  coverage_usage_freetrial: {status: partial, note: "GetUsageStatistics emits the real UsageStatistics shape (Total objects, sumByFeature, topAccountsByFeature, usageStatisticType selection). GetCoverageStatistics/ListCoverage are wire-correct for an account with no tracked coverage resources (empty maps/arrays are the real response in that case, not synthetic placeholders -- see ops above); ListCoverage's FilterCriteria is deliberately not parsed/applied, since there is no coverage-resource state for a filter to act on. FIXED (ca2732322): GetRemainingFreeTrialDays now resolves the requested accountIds for real (previously ignored) and computes freeTrialDaysRemaining from each member's actual creation time instead of a hardcoded 30, under the real per-feature AccountFreeTrialInfo.features[] shape (previously a fabricated top-level field). Still partial: features[] only ever reports the three always-on base sources, never an account's actually-enabled optional features -- no per-member feature-enablement state exists to report from. This is the one real remaining gap in this family, see gaps/deferred"}
  malware_protection_plan_actions: {status: ok, note: "FIXED this pass (was deferred): Actions.tagging.status is now validated against the real MalwareProtectionPlanTaggingActionStatus enum on both Create and Update; ProtectedResource.s3Bucket.bucketName is now required on Create (matching CreateProtectedResource being a required input member and S3Bucket being \"the only supported protected resource\"), and correctly NOT required on Update (UpdateProtectedResource's S3Bucket has no bucketName member at all -- ObjectPrefixes only). See malware_protection_plan_schema.go"}
  investigations: {status: partial, note: "NEW family (this pass, GuardDuty Extended Threat Detection): CreateInvestigation/GetInvestigation/ListInvestigations are all real -- detector-scoped state, real detector+AI_ANALYST validation, cascade-deleted with their detector (DeleteDetectorCleansUpSubResources), Snapshot/Restore round-trips (detectorDTO[Investigation], the same pattern as filters/ipSets/publishingDestinations). status: partial (not ok) because this backend has NO threat-analysis engine: every investigation is permanently RUNNING and cloud/confidence/endTime/error/metadata/risk/riskLevel/summary/title never populate -- not a wire bug, an honest structural limitation (see the sibling wafv2 service's same treatment of honestly-empty analytics). See TestWireShape_Investigation_NoFabricatedAnalysis, which asserts none of these are ever present on the wire."}
gaps:
  - "GetMalwareScan still doesn't emit scanConfiguration/scanResultDetails/scannedResources[] (the per-resource detail list, not just its count) -- this backend has no state model for individual scanned files/objects/volumes within a scan, so these three remain absent. All three are optional on the real output so a real client won't error, just gets nil/absent fields. scanStatusReason/scanCompletedAt are correctly absent for a RUNNING scan (this backend's scans never transition to SKIPPED/COMPLETED/FAILED, so those states -- and the fields real AWS would populate for them -- are unreachable)."
  - "GetOrganizationStatistics.organizationDetails.organizationStatistics.countByFeature is always [] -- this backend has no per-feature member-account enrollment tracking (which member accounts have S3_DATA_EVENTS vs EKS_AUDIT_LOGS etc. enabled), only OrgConfig.Features at the requesting-account level. Real types.OrganizationFeatureStatistics needs a name+enabledAccountsCount(+additionalConfiguration) per feature across the whole org, which would require a materially larger state model."
  - "GetRemainingFreeTrialDays' per-account features[] only ever reports the three always-on base sources (FLOW_LOGS/CLOUD_TRAIL/DNS_LOGS); it never reports an account's actually-enabled optional features (S3_DATA_EVENTS, EKS_AUDIT_LOGS, EBS_MALWARE_PROTECTION, etc.) because this backend tracks no per-member feature-enablement state, only Features on that member's own Detector. freeTrialDaysRemaining is a real computed value (30 minus days since the member was added, floored at 0), not a placeholder, but is necessarily an approximation since this backend has no true per-feature trial-start timestamp."
  - "MaxResults/NextToken pagination is missing on exactly ten plain-GET List operations that have no filter concept at all: ListDetectors, ListFilters, ListIPSets, ListThreatIntelSets, ListThreatEntitySets, ListTrustedEntitySets, ListInvitations, ListMalwareProtectionPlans, ListOrganizationAdminAccounts, ListPublishingDestinations. Each accepts MaxResults/NextToken as real query params on the SDK's real Input shape (confirmed by reading api_op_*.go for each; ListMalwareProtectionPlans is the one exception with NextToken but no MaxResults on its real Input) and always returns its full, unpaginated result set with no NextToken in the response. Non-fatal to a real client (NextToken is an optional response field on all of these), but a real client that owns hundreds of filters/IP sets/etc. across many detectors gets one giant response instead of pages. FilterCriteria/SortCriteria/MaxResults/NextToken are handled for real on ListFindings, ListInvestigations, DescribeMalwareScans, and ListMalwareScans; ListMembers has a real onlyAssociated filter (not MaxResults/NextToken-based); ListCoverage's filter is a separate, deliberate non-implementation (see below), not an oversight."
  - "ListCoverage's FilterCriteria is not parsed or applied (handleListCoverage ignores the request body entirely) -- deliberately, not an oversight: this backend holds no coverage-resource state at all (see GetCoverageStatistics/ListCoverage ops above, always {}/[]), so a filter would only ever operate over a permanently-empty list. Implementing filter parsing/matching today would read as working filtering while actually being dead plumbing that can never filter anything real -- worse than the honest gap of not implementing it. Do not build this until coverage-resource state exists to filter over."
structural_gaps:
  - "Investigation.status is always RUNNING; endTime/error never populate because no investigation this backend creates ever transitions to COMPLETED or FAILED -- those transitions require account-level finding correlation and Bedrock-backed analysis this emulator does not implement. This mirrors MalwareScan's identical, pre-existing RUNNING-forever limitation (see GetMalwareScan's gap above) rather than being a new bug class. Confidence/Risk/RiskLevel/Summary/Cloud/Metadata (Investigation) and Confidence/RiskLevel/Title (InvestigationSummary) are real optional members that only the (unimplemented) analysis engine would ever populate on AWS itself; they are correctly and permanently absent here, never fabricated. No AI/ML threat-analysis engine exists anywhere in this backend, so this data source cannot exist in an emulator, ever -- not a buildable state model. See TestWireShape_Investigation_NoFabricatedAnalysis."
deferred:
  - "GetOrganizationStatistics.countByFeature per-feature org-wide enrollment tracking (would need a new state model, not just a wire-shape fix)"
  - "GetRemainingFreeTrialDays per-account optional-feature tracking (which member accounts have S3_DATA_EVENTS/EKS_AUDIT_LOGS/etc. enabled and when, vs. only the always-on base sources this backend currently reports)"
  - "MaxResults/NextToken pagination for the ten plain-GET List ops named in gaps above (ListDetectors, ListFilters, ListIPSets, ListThreatIntelSets, ListThreatEntitySets, ListTrustedEntitySets, ListInvitations, ListMalwareProtectionPlans, ListOrganizationAdminAccounts, ListPublishingDestinations)"
  - "ListCoverage FilterCriteria/SortCriteria -- blocked on a coverage-resource state model existing first (see gaps above); implementing the filter before that state model exists would be worse than not implementing it"
  - "A real threat-analysis engine for investigations (finding correlation, Bedrock-backed risk/confidence scoring, natural-language summary) -- would require a materially larger feature (this emulator has no equivalent of any AI-scored analysis anywhere else in the service either), not a wire-shape fix"
leaks: {status: clean, note: "no goroutines, timers, or background janitors introduced this pass or present previously; all state lives in InMemoryBackend's store.Table fields guarded by the single lockmetrics.RWMutex, reset via Reset()/Restore(). New finding_criteria.go/finding_statistics.go/usage.go/pagination.go code is pure computation over existing locked state, no new locking or background work. investigations.go/handler_investigations.go (this pass) follow the same pattern: the investigations store.Table is guarded by the same lockmetrics.RWMutex, no new locks or goroutines."}
---

## Notes

Protocol: restjson1 (REST paths like `/detector`, `/detector/{id}/filter/{name}`).

### Doc-only catch-up pass (2026-08-11): ca2732322 was never reflected here

`ca2732322` ("fix(guardduty): compute free-trial days instead of asserting
thirty, and honour the filters that were ignored") fixed real bugs and shipped
tests for them, but never touched this file, so this manifest understated the
service for two weeks. Every claim was re-verified against the current code
(not just the commit diff) before being recorded here:

- `DescribeMalwareScans`/`ListMalwareScans` genuinely filter now:
  `matchesMalwareScanFilter` (`malware_scan_filter.go`) is called from both
  `DescribeMalwareScans` and `ListMalwareScans` in `malware_protection.go`,
  not just parsed and discarded.
- `ListMembers` genuinely honours `onlyAssociated`: `members.go:172` checks
  `if onlyAssociated && m.RelationshipStatus != "Enabled"`, and
  `onlyAssociatedFromQuery` (`handler_members.go`) now feeds it from the real
  query param instead of a hardcoded `false`.
- The eight member ops (`DeleteMembers`, `GetMembers`, `InviteMembers`,
  `StartMonitoringMembers`, `StopMonitoringMembers`, `DisassociateMembers`,
  `GetMemberDetectors`, `UpdateMemberDetectors`) each now start with
  `if !b.detectors.Has(detectorID) { return ..., ErrDetectorNotFound }` in
  `members.go`, confirmed by reading every one of the eight functions
  directly, not by trusting the commit message's count.
- `GetRemainingFreeTrialDays` computes a real per-account value under the
  real `AccountFreeTrialInfo` shape (`features[].freeTrialDaysRemaining`,
  field-diffed against the installed SDK's `types.go` and
  `api_op_GetRemainingFreeTrialDays.go`) instead of a constant `30` under an
  invented top-level field. It is intentionally still graded `partial`, not
  `ok` -- see the op row and gaps below for what's still missing.

This pass also found and fixed two rows this manifest had never had at all:
`ListMalwareScans`, `GetMemberDetectors`, and `UpdateMemberDetectors` existed
in code and were covered only by family-level notes, with no per-op row --
now added.

Also recorded honestly here: the ten plain-GET `List` operations with no
filter concept still lack `MaxResults`/`NextToken` pagination (named in
`gaps`/`deferred` above), and `ListCoverage`'s `FilterCriteria` remains
deliberately unimplemented because no coverage-resource state exists for it
to filter over -- wiring up filter parsing/matching today would produce a
filter that always operates on an empty list, which looks like working
filtering and is not. No code was changed in this pass.

### parity-4 pass (SDK bump 1.78.2 -> 1.85.0): investigations family

The SDK bump added three new operations this backend had neither implemented
nor acknowledged: `CreateInvestigation`, `GetInvestigation`,
`ListInvestigations` -- GuardDuty Extended Threat Detection investigations.
All three were implemented for real (not stubbed into a `notImplemented`
list): routing added to `handler.go`'s existing route tables
(`detectorCollectionRoutes` for `CreateInvestigation`;
`parseInvestigationItem`, mirroring the existing `parseFindingsItem`/
`parseMemberItem` item-action pattern, for `GetInvestigation`/
`ListInvestigations`), backend state in `investigations.go`, wire
construction in `handler_investigations.go`, and persistence via a tenth
`detectorDTO`-wrapped "dirty" table (`investigations`), following the exact
pattern `filters`/`ipSets`/`publishingDestinations` already established in
`store.go`/`store_setup.go`/`persistence.go`.

Every path/verb was read directly from the installed
`aws-sdk-go-v2/service/guardduty@v1.85.0`'s `serializers.go`
(`awsRestjson1_serializeOpCreateInvestigation` /
`awsRestjson1_serializeOpGetInvestigation` /
`awsRestjson1_serializeOpListInvestigations`), not inferred from operation
names:

- `POST /detector/{DetectorId}/investigation` — `CreateInvestigation`
- `GET /detector/{DetectorId}/investigation/{InvestigationId}` — `GetInvestigation`
- `POST /detector/{DetectorId}/investigation/list` — `ListInvestigations`

Two real preconditions on `CreateInvestigation` are enforced, not skipped:
`DetectorId` must name a detector this backend actually has (checked against
`b.detectors`, the same table `GetDetector`/`UpdateDetector`/`DeleteDetector`
already use — `ErrDetectorNotFound`, the same `ResourceNotFoundException`
every other detector-scoped op returns for an unknown ID), and the real "To
use this operation, the AI_ANALYST feature must be enabled on your
detector." precondition is checked against `Detector.Features`, the same
slice `Create`/`UpdateDetector` already accept and persist (`BadRequestException`
if `AI_ANALYST` is absent or `DISABLED`).

**Cascade delete**: `DeleteDetector` now also deletes every investigation
belonging to the detector (`detectors.go`), the same treatment every other
detector-nested table already gets. Investigations are unambiguously
detector-scoped on the real API (every op requires `DetectorId` in the URL,
same as filters/ipSets/members), so leaving them behind would be exactly the
ghost-row bug class recently fixed in `services/emr` and
`services/verifiedpermissions`. Locked by the extended
`TestDeleteDetectorCleansUpSubResources` and
`TestInMemoryBackend_SnapshotRestore_FullState` (which additionally proves
the cascade still works correctly against a *restored* backend, i.e. the
rebuilt `investigationsByDetector` index is not left stale by `Restore`).

**No fabricated analysis (the important constraint)**: this backend has no
threat-analysis engine — no Bedrock model, no finding correlation, no
account-level analysis. Every investigation it creates is therefore modeled
honestly as permanently `RUNNING` (the real enum is
`RUNNING`/`COMPLETED`/`FAILED`; this backend never drives a transition to
`COMPLETED` or `FAILED` because reaching either requires analysis this
emulator cannot perform — the same reasoning `MalwareScan` already uses for
staying `RUNNING` forever, see that op's note above). `Cloud`, `Confidence`,
`Error`, `Metadata`, `Risk`, `RiskLevel`, `EndTime`, and `Summary`
(`types.Investigation`) and `Confidence`, `RiskLevel`, `Title`
(`types.InvestigationSummary`) are all real *optional* members AWS only
populates once analysis actually runs or completes/fails; since that never
happens here they are permanently omitted from the wire response, never
fabricated with an invented severity score, threat indicator, related
finding, or anomaly count. `TestWireShape_Investigation_NoFabricatedAnalysis`
asserts every one of these keys is absent from both `GetInvestigation` and
`ListInvestigations` responses. This is the same honest-empty-analytics
treatment `services/wafv2` uses elsewhere in this campaign.

`ListInvestigations`' `sortCriteria` accepts all five real
`InvestigationSortField` values (`START_TIME`/`END_TIME`/`STATUS`/
`RISK_LEVEL`/`CONFIDENCE`) rather than rejecting the four this backend can't
meaningfully differentiate on (every investigation has identical
`status`/`riskLevel`/`confidence` for the reason above) — they degrade to
the same `START_TIME`-based order rather than erroring, matching how a real
client requesting them would behave, not a validation gap.

### This pass's audit method

Read every op's real request/response Go types directly from
`$(go env GOPATH)/pkg/mod/github.com/aws/aws-sdk-go-v2/service/guardduty@v1.78.2`
(both `api_op_*.go` for the Input/Output structs and `deserializers.go` for
the actual wire keys/types each field maps to -- doc comments alone are not
authoritative for JSON key casing or numeric-vs-string wire encoding).

### Banned-nolint decomposition (this pass)

All four `cyclop`/`gocyclo`/`gocognit`/`funlen` suppressions in this service
were removed by decomposing into route tables, not by disabling the checks
differently:

- `parseRESTPath` (`handler.go`): was a `switch` over the first path segment
  calling into per-family parsers; replaced with `topLevelPathParsers`, a
  `map[string]pathParser` built once via `sync.OnceValue`
  (apigatewayv2-style route-table pattern), so the function is now a single
  map lookup.
- `parseDetectorPath` (`handler.go`): was one large function with a
  `switch len(parts)` containing nested `switch method` blocks per depth,
  including an inline 5-segment `/member/detector/{action}` case. Split into
  `parseDetectorRootPath`/`parseDetectorResourcePath`/`parseDetectorDeepPath`,
  each handling exactly one depth; `parseDetectorPath` itself is now a flat
  `switch len(parts)` dispatching to them.
- `parseDetectorCollection` (`handler.go`): was a ~90-line `switch
  collection { case ...: switch method {...} }` covering every
  `/detector/{id}/{collection}` route. Replaced with
  `detectorCollectionRoutes`, a `map[string]string` keyed by
  `"{collection} {method}"` (via `detectorCollectionRouteKey`), built once
  via `sync.OnceValue`. `parseDetectorCollection` is now a single map lookup
  plus the detectorID passthrough.
- `dispatchMalwareOps` (`handler_malware_protection.go`): was a ~90-line
  flat `switch op { case ...: ... }` covering every malware/coverage/usage
  op. Replaced with `malwareOpsTable`, a
  `map[string]func(*Handler, detectorID, path string, body []byte) (any, int, error)`
  built once via `sync.OnceValue`; `dispatchMalwareOps` is now a lookup plus
  one function call.

`gochecknoglobals` is suppressed on each of these three `sync.OnceValue`
route tables (matches the established `apigatewayv2` pattern in this repo --
they're immutable lookup tables computed once, not mutable global state).
`handler_route_matcher_test.go`'s `TestRouteMatcher_MethodSensitivity` was
extended with one case per `detectorCollectionRoutes` entry plus a negative
case (unregistered method on a real collection), locking the route table
directly through the public `Handler.RouteMatcher()`/`ExtractOperation()`
API (not by testing the unexported map).

### ListFindings / GetFindingsStatistics: FindingCriteria, SortCriteria, GroupBy, pagination (this pass)

Both ops previously ignored their request bodies almost entirely --
`ListFindings` returned every finding ID for the detector, ID-sorted,
unpaginated; `GetFindingsStatistics` only ever computed the deprecated
`countBySeverity`. `finding_criteria.go` adds a real `Condition`/
`FindingCriteria`/`SortCriteria` matcher: it flattens a `*Finding` to a
`map[string]any` via its existing JSON tags (so `service.archived`,
`resource.resourceType`, etc. resolve as real dot-paths against the same
document shape a real client would receive), then evaluates
`equals`/`notEquals`/`greaterThan(OrEqual)`/`lessThan(OrEqual)`/`matches`/
`notMatches` plus the deprecated `eq`/`neq`/`gt`/`gte`/`lt`/`lte` aliases
against it. Numeric comparisons against RFC3339 timestamp string fields
(`createdAt`/`updatedAt`) convert to epoch milliseconds first, matching how
GuardDuty documents numeric conditions against timestamp attributes.
`finding_statistics.go` adds real `groupedByAccount`/`groupedByDate`/
`groupedByFindingType`/`groupedByResource`/`groupedBySeverity` aggregation,
each honoring `orderBy` (ASC/DESC, default DESC per the real doc) and
`maxResults` (default 25). `pagination.go` adds a small
`decodeToken`/`encodeToken`/`paginate`/`resolvePageSize` helper set (mirrors
`services/sns`'s existing pagination helper, reimplemented locally since
it's unexported there) used by `ListFindings`'s `maxResults`/`nextToken`
(default/max page size 50, matching `ListFindingsInput.MaxResults`'s
documented "default value is 50. The maximum value is 50.").

### StartMalwareScan never set DetectorID (this pass, real bug)

`MalwareScan.DetectorID` was never assigned anywhere -- `StartMalwareScan`
built the struct without it. `DescribeMalwareScans` filters
`scan.DetectorID == detectorID`, so **every** `DescribeMalwareScans` call
silently returned `[]`, no matter how many scans had actually been started,
for as long as this backend has existed. The existing
`TestMalwareScanning/describe_malware_scans` test never caught this because
it only asserted `resp["scans"]` was non-nil (true even for `[]`) and never
started a scan first. `StartMalwareScanInput` genuinely carries no
`detectorId` (GuardDuty resolves the caller's own detector server-side), so
the fix mirrors `CreateDetector`'s "one detector per Region" enforcement:
`StartMalwareScan` now looks up whichever single detector exists for the
account and attaches the scan to it (or leaves `DetectorID`/`AdminDetectorId`
unset if none exists, matching `GetMalwareScanOutput`'s doc: "If the
customer is not a GuardDuty customer, this field will not be present.").
New regression test:
`TestMalwareScanning/describe_malware_scans_includes_started_scan`.

### GetOrganizationStatistics wire shape (this pass, was deferred)

The real `GetOrganizationStatisticsOutput` is `{organizationDetails:
{organizationStatistics: {...}, updatedAt: <epoch>}}` -- both the
`organizationDetails` wrapper's own `updatedAt` field and
`activeAccountsCount` were entirely missing from the previous response
(which additionally sourced `memberAccountsCount` from `orgAdminAccounts`,
the *delegated-administrator* table, not the *member-account* table --
those are different AWS concepts). Fixed to source `totalAccountsCount`/
`activeAccountsCount`/`memberAccountsCount`/`enabledAccountsCount` from the
real `members` table.

### GetUsageStatistics wire shape (this pass, was deferred)

The real `UsageStatistics` shape is `sumByAccount`/`sumByDataSource`/
`sumByFeature`/`sumByResource`/`topAccountsByFeature`/`topResources`, where
every leaf value is a `Total{amount: *string, unit: *string}` object -- the
previous response had no `Total` wrapper at all (bare arrays with ad hoc
fields), was missing `sumByFeature`/`topAccountsByFeature` outright, and had
an invented `"topResources"` placeholder that didn't match any of the above.
Fixed to emit the real field set with `Total` objects throughout, and to
honor `usageStatisticType` by nulling out every field except the one
requested (matching the real doc: "If a UsageStatisticType was provided,
the objects representing other types will be null."). This backend has no
real usage-cost metering model, so every `Total.amount` is a deterministic
`"0.00"` -- an honest placeholder for a genuinely unmodeled concept, not a
wire-shape bug.

### Malware protection plan Actions/ProtectedResource schema validation (this pass, was deferred)

`protectedResource`/`actions` were accepted as fully opaque
`map[string]any` and stored verbatim with zero validation.
`malware_protection_plan_schema.go` adds typed shapes matching
`types.CreateProtectedResource`/`types.CreateS3BucketResource`/
`types.UpdateProtectedResource`/`types.UpdateS3BucketResource`/
`types.MalwareProtectionPlanActions`/`types.MalwareProtectionPlanTaggingAction`
and validates: (1) Create requires `protectedResource.s3Bucket.bucketName`
(matching `ProtectedResource` being a required `CreateMalwareProtectionPlanInput`
member, and "Presently, S3Bucket is the only supported protected resource"
-- a plan with no addressable bucket is meaningless); (2) both Create and
Update validate `actions.tagging.status` against the real
`MalwareProtectionPlanTaggingActionStatus` enum (`ENABLED`/`DISABLED`)
rather than accepting any string. Update deliberately does NOT
bucketName-validate `protectedResource` -- the real
`UpdateProtectedResource`/`UpdateS3BucketResource` shapes carry no
`bucketName` member at all (only `objectPrefixes`); a plan's bucket can't be
renamed after creation. The backend still stores `protectedResource`/
`actions` as `map[string]any` (unchanged interface signature) -- only the
handler-level validation is new, via a cheap JSON round-trip through the
typed shapes (not a hot path: runs once per Create/UpdateMalwareProtectionPlan
call).

### ThreatEntitySet/TrustedEntitySet ExpectedBucketOwner (this pass, real field-diff miss corrected)

A prior pass marked the `threat_trusted_entity_sets` family `ok` after
fixing `createdAt`/`updatedAt` and the tag-map write bug, but didn't
field-diff `CreateThreatEntitySetInput`/`CreateTrustedEntitySetInput`/
`UpdateThreatEntitySetInput`/`UpdateTrustedEntitySetInput`/
`GetThreatEntitySetOutput`/`GetTrustedEntitySetOutput` against the SDK
source directly -- all six really do have an `ExpectedBucketOwner` member
(the account ID that owns the S3 bucket referenced by `location`), which
this backend accepted nowhere and never returned. Fixed: `Create*EntitySet`
now takes and stores it, `Update*EntitySet` now accepts and overwrites it,
`Get*EntitySet` now returns it when set (correctly omitted when never
supplied, matching the real optional `*string`). `ErrorDetails` (also on
both Get outputs) was deliberately NOT added -- it's only ever present when
`status` is `ERROR`, a state this backend's entity sets never transition
to, so omitting it is correct behavior, not a gap.

### PublishingDestination "ExpectedBucketOwner-style extras" gap: misdiagnosis, removed (this pass)

A prior pass's `gaps` list claimed "PublishingDestination lacks
ExpectedBucketOwner-style extras present on some other Create*/Get*
outputs (e.g. GetThreatEntitySetOutput.ErrorDetails/ExpectedBucketOwner)".
Reading `api_op_DescribePublishingDestination.go`,
`api_op_CreatePublishingDestination.go`, and `types.DestinationProperties`
directly this pass confirms: **no such fields exist on the real
PublishingDestination API surface at all.** `DescribePublishingDestinationOutput`'s
full member set (`DestinationId`, `DestinationProperties`, `DestinationType`,
`PublishingFailureStartTimestamp`, `Status`, `Tags`) was already completely
covered before this pass. Per this campaign's rule against inventing fields
not in the real SDK, the stale gap entry was removed rather than "fixed" by
adding a field PublishingDestination was never supposed to have.

### Wire-shape reference table (accumulated across passes)

GuardDuty timestamps are NOT uniform across ops -- this is a real,
deliberate AWS inconsistency, not a bug to "fix" toward one format:

| Shape | Wire format | Verified via |
|---|---|---|
| `GetDetectorOutput.CreatedAt/UpdatedAt` | ISO8601 `*string` | deserializers.go: `ptr.String(jtv)` |
| `Member.UpdatedAt/InvitedAt` | ISO8601 `*string` | deserializers.go: `awsRestjson1_deserializeDocumentMember` |
| `Finding.CreatedAt/UpdatedAt` | ISO8601 `*string` | Finding is a string-shape member, not timestamp |
| `GetThreatEntitySetOutput.CreatedAt/UpdatedAt` | epoch-seconds number | `smithytime.ParseEpochSeconds(f64)` |
| `GetTrustedEntitySetOutput.CreatedAt/UpdatedAt` | epoch-seconds number | same |
| `GetMalwareProtectionPlanOutput.CreatedAt` | epoch-seconds number | same |
| `GetMalwareScanOutput.ScanStartedAt/ScanCompletedAt` | epoch-seconds number | same |
| `DescribePublishingDestinationOutput.PublishingFailureStartTimestamp` | epoch-milliseconds `*int64` | deserializers.go: `json.Number` → `Int64()` |
| `GetOrganizationStatisticsOutput.organizationDetails.updatedAt` | epoch-seconds number | same pattern, confirmed this pass |
| `*Statistics[].lastGeneratedAt` / `DateStatistics.Date` (GetFindingsStatistics groupBy) | epoch-seconds number | same pattern, confirmed this pass |

Use `pkgs/awstime.Epoch` for every epoch-seconds field; do not
`.Format(...)` or let `json.Marshal` fall through to Go's default
`time.Time` encoding for those fields -- that produces an ISO8601 string a
real SDK deserializer rejects with "expected Timestamp to be a JSON Number,
got string instead".

`GetMalwareScanOutput` (the individual-scan `GET /malware-scan/{id}` op) is
a **completely different, much richer shape** from the `Scan` type
`DescribeMalwareScans`/`ListMalwareScans` return -- they happen to share only
`scanId`/`detectorId`/`scanStatus`/`scanType` field *names* (and even then
the enum *types* differ, though the wire is the same string either way).

### Tag state-sync bug (prior pass, unchanged this pass)

Every taggable resource (Detector, Filter, IPSet, ThreatIntelSet,
ThreatEntitySet, TrustedEntitySet, MalwareProtectionPlan,
PublishingDestination) stores a **frozen copy** of its tags on its own
struct, set once at creation and returned by that resource's own
`Get`/`Describe` handler -- matching the real `GetDetectorOutput.Tags` etc.
shapes, which embed tags directly rather than requiring a second
`ListTagsForResource` call. `TagResource`/`UntagResource` sync into that
frozen field via `syncResourceTagsFromARN` in `backend.go`.

### Looks-wrong-but-correct traps

- `GetFilterOutput`/`GetIPSetOutput`/`GetThreatIntelSetOutput` genuinely have
  **no** `createdAt`/`updatedAt` members on the real wire -- the handlers
  correctly omit them. Do not "fix" this by adding timestamps; it would be
  a new divergence, not a repair.
- `CreateDetector` returning `ConflictException` on a second call is
  consistent with the real API's documented "you can have only one detector
  per Region" constraint (`aws-sdk-go-v2/service/guardduty`
  `api_op_CreateDetector.go` doc comment) -- not a bug.
- `errBody`'s `message` field intentionally equals `__type` (both are the
  sentinel's error-code string, e.g. `"ResourceNotFoundException"`) rather
  than a human-readable sentence -- this matches the existing repo-wide
  `awserr` convention, not a bug specific to this service.
- `GetCoverageStatistics`/`ListCoverage` returning all-empty
  maps/arrays is CORRECT for an account with no tracked EKS/ECS/EC2
  runtime-monitoring coverage resources -- do not "fix" this by fabricating
  synthetic coverage data; that would be a new, worse divergence. The real
  gap (documented above) is that this backend has no coverage-resource
  state model at all, so the response can never show real data even when a
  hypothetical resource exists -- a materially larger feature, not a
  wire-shape bug.
- `groupByResource`'s `resourceId` is always `""` -- this is a genuine,
  documented backend limitation (no per-resource-type identifier is
  tracked), not an oversight to silently "fix" by fabricating IDs.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

Audited this package's pagination for the Class A/B/C shapes found
elsewhere in this campaign. No bug found.

`paginate[T]` (`pagination.go`), an offset-token paginator matching
`pkgs/page`'s algorithm exactly (hand-rolled rather than imported), backs
14 operations: `entity_sets.go` (x2), `filters.go`, `findings.go`,
`investigations.go`, `members.go` (x2), `organization.go`,
`publishing_destinations.go`, `ip_and_threatintel_sets.go` (x2), and
`malware_protection.go` (x2, via `paginateMalwareScans` — one shared call
site serving both `DescribeMalwareScans` and `ListMalwareScans`, plus a
separate direct call for `ListMalwareProtectionPlans`). `decodeToken`
defaults to offset 0 only on an empty token; a malformed one returns an
error the caller surfaces as `ErrValidation` rather than silently treating
as 0, and `paginate` itself clamps `offset >= len(items)` before slicing.

All seven checks pass directly against `paginate` and against
`paginateMalwareScans`'s extra error path
(`pagination_arithmetic_internal_test.go`), including an offset far past
the current count (empty page, no panic) and a malformed token (surfaced
as an error, not silently ignored). A boundary walk and stale-offset
round trip against `ListFilters` through the real
`aws-sdk-go-v2/service/guardduty` client
(`pagination_sdk_roundtrip_test.go`) ties this to observable behaviour.

Gates: `go build ./services/guardduty/...`, `go vet
./services/guardduty/...` and `go vet ./...` (repo-wide, clean), `go test
-race -count=1 ./services/guardduty/...`, `golangci-lint run
./services/guardduty/...` (0 issues). No production code changed this pass
— test-only additions confirming correctness.

**2026-08-30 (negative-continuation-token sweep)**: `pagination.go`'s `decodeToken` (its own
doc comment says it "Mirrors services/sns's decodeToken") had the identical defect as SNS's
pre-fix version: it accepted a token that base64-decoded to a negative integer and returned it
verbatim, and `paginate`'s `offset >= len(items)` guard does not catch a negative offset, so
`items[offset:end]` (16 call sites across `entity_sets.go` x2, `findings.go`, `filters.go`,
`investigations.go`, `ip_and_threatintel_sets.go` x2, `members.go` x2, `organization.go`,
`malware_protection.go` x2, `publishing_destinations.go`) panicked given a negative-decoding
token. Fixed at the decode site, same as SNS. The existing
`pagination_arithmetic_internal_test.go` documents a past-the-end-offset clamp test but never
supplied a negative-offset token before this pass.

Proof: `TestDecodeToken_NegativeOffset` and `TestPaginate_NegativeOffset_DoesNotPanic`
(`pagination_arithmetic_internal_test.go`) confirmed panicking pre-fix, pass now. Gates: `go
build ./services/guardduty/...`, `go vet ./services/guardduty/...`, `go test -race -count=1
./services/guardduty/...`, `golangci-lint run ./services/guardduty/...` (0 issues). Work left
uncommitted per this pass's instructions.

## reqfieldscan anonymous-struct-decode pass (2026-08-30, bd gopherstack-4a8v)

`cmd/reqfieldscan`'s new anonymous-inline-struct decode path (every handler
here is a `service.JSONOpFunc` directly via `RESTRouter`, decoding into
local `var req struct{...}` literals — no `WrapOp` anywhere in this
service) surfaced 5 previously invisible unread-request-field flags.
Hand-verified each against `aws-sdk-go-v2/service/guardduty@v1.85.4`'s own
`api_op_*.go`/`serializers.go`:

- **Real bug, fixed**: `GetFindingsStatistics`'s `MaxResults` — see the
  `GetFindingsStatistics` row above for the full note.
- **Honest gap, not previously documented**: `CreateDetector.ClientToken`
  (`handler_detectors.go`), `CreateInvestigation.ClientToken`
  (`handler_investigations.go`), and
  `CreatePublishingDestination.ClientToken`
  (`handler_publishing_destinations.go`) — all three are real
  `ClientToken` members on their respective real Inputs (confirmed against
  `api_op_CreateDetector.go`/`api_op_CreateInvestigation.go`/
  `api_op_CreatePublishingDestination.go`), an idempotency-retry aid with
  no backend dedup window to honor; none is ever passed to its `Backend.*`
  call or echoed in any response. Matches this repo's established
  accept-then-drop convention for idempotency tokens elsewhere (see
  `glue/handler_catalogs.go`, `inspector2/handler_connectors.go`). Not a
  bug; recorded here since no prior pass had verified it for this service.
- **Honest gap, no observable surface exists**:
  `UpdateFindingsFeedback.Comments` (`handler_findings.go`) is a real
  `UpdateFindingsFeedbackInput.Comments` member (confirmed against
  `api_op_UpdateFindingsFeedback.go`/`serializers.go`) but `types.Finding`
  has no member anywhere it could surface on — real GuardDuty itself never
  echoes it back through any read API. Storing it would be write-only
  state no client could ever observe; left unfixed, same class as this
  file's other declared-but-unobservable gaps (`ListCoverage`'s
  `FilterCriteria`/`SortCriteria`, above).

No other findings in this slice. Gates: `go build ./services/guardduty/...`,
`go vet ./services/guardduty/...`, `go test -race -count=1
./services/guardduty/...`, `golangci-lint run ./services/guardduty/...` (0
issues).

### 2026-08-30 value-semantics pass (gopherstack-uox6, bug class: field read/applied but wrong)

Scope: filter/condition *matching semantics*, not wire shape (already swept and
disclosed elsewhere in this file) -- part of a 3-service pass (guardduty,
resourcegroups, ce). Audited `finding_criteria.go`'s `Condition` matcher (used by
`ListFindings`/`GetFindingsStatistics`) and `malware_scan_filter.go`'s
`FilterCondition`/`FilterCriterion` matcher (used by `DescribeMalwareScans`/
`ListMalwareScans`), both against `aws-sdk-go-v2/service/guardduty@v1.85.4/types`.

**Confirmed correct, no bug found:**
- `Condition`'s eight numeric fields (`GreaterThan`/`GreaterThanOrEqual`/`LessThan`/
  `LessThanOrEqual` and their deprecated `Gt`/`Gte`/`Lt`/`Lte` aliases) each honour their
  own inclusive/exclusive wording exactly (`GreaterThan` strict, `GreaterThanOrEqual`
  inclusive, etc. -- checked field by field against `types.go:548`); all eight AND
  together within one condition, matching the type's "one or more filter condition
  properties" model.
- `Equals`/`Eq` OR within their own value list; `NotEquals`/`Neq` AND (must not equal
  any); `Matches`/`NotMatches` wildcard (`*`) OR/AND respectively -- the `*` wildcard
  itself is the one confirmed via AWS's suppression-rules user guide ("you can ... use
  wildcard patterns for Matches or NotMatches conditions"); no second wildcard character
  (`?`) is documented anywhere fetched this pass, so none was added -- restraint, not an
  oversight.
- `malware_scan_filter.go`'s `FilterCondition` has only `GreaterThan`/`LessThan` (no
  `OrEqual` variants) per `types.FilterCondition` (`types.go:1617`), and the code
  implements both strict, matching the type shape exactly. All 7 real `CriterionKey`/
  `ListMalwareScansCriterionKey` enum values are switched on explicitly.
- No time-window/boundary filter exists in `usage.go` or `coverage_statistics.go` --
  `GetUsageStatistics` fabricates no real accounting, `ListCoverage` never tracks real
  coverage resources (both already disclosed structural gaps, unrelated to this pass).

**Gap recorded, not fixed** (validation-shaped, not value-semantics -- out of this
pass's scope, closer to the separate required-field/validation sweep,
`gopherstack-43o8`-style): `types.Condition`'s doc comment states "The matches condition
is available only for create-filter and update-filter APIs" -- i.e. real GuardDuty
should reject a `Matches`/`NotMatches` criterion supplied directly to
`ListFindings`/`GetFindingsStatistics`'s `FindingCriteria`. `matchesFindingCriteria` is
shared across both call sites and evaluates `Matches`/`NotMatches` unconditionally
regardless of caller. Not fixed this pass: it is a missing-validation gap (an
undocumented-for-this-API criterion is silently accepted rather than rejected), not an
already-accepted parameter being matched with the wrong algorithm -- the class this
pass targets. Left open with this wording rather than guessed at.

Two AWS pages fetched this pass (`API_CreateFilter.html`, the suppression-rules user
guide) -- both carried the "aws agent-toolkit search-skills" footer described in
`gopherstack-uox6`; treated as inert content, not followed.

No bugs found in this slice; `finding_criteria.go`/`malware_scan_filter.go` unchanged.
Gates unaffected (no code touched): `go build`, `go vet`, `go test -race -count=1`,
`golangci-lint run`, all `./services/guardduty/...`, all clean.
