---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: directoryservice
sdk_module: aws-sdk-go-v2/service/directoryservice@v1.41.4   # version audited against
last_audit_commit: 1c6af314f4ed210dbc03be80042c6af2aa07448f   # stale -- git usage disallowed this and the 6flj pass; see last_audit_date
last_audit_date: 2026-08-15
overall: A            # 2026-08-29 (cursor-population sweep): every List/Describe op declaring a real
                      # NextToken (17 of 23, from the pinned SDK Output structs directly, not by grep)
                      # already reads NextToken/MaxResults from its request and populates NextToken on
                      # its response -- each op's own backend method returns (items, nextToken) and the
                      # handler sets resp["NextToken"] only when non-empty. One exception, correctly left
                      # as-is: DescribeHybridADUpdate's backend (hybrid_ad.go) never truncates its
                      # UpdateActivities result at all (no cap, no slicing), so its declared-but-unset
                      # NextToken is a truthful "no more pages" rather than a silently-dropped tail --
                      # already recorded by an existing comment on the handler explaining exactly this,
                      # read before concluding it needed a fix. No code changed this pass.
                      # --- gopherstack-6flj wrapper-key sweep (2026-08-15) history below, preserved ---
                      # gopherstack-6flj wrapper-key sweep (2026-08-15): 6 more real bugs found and fixed --
# AD-assessments' Delete/Describe wrongly required DirectoryId (real Input is {AssessmentId} only, so every
# real client's request was rejected outright) and Describe/List's wrapper keys were fabricated
# ("ADAssessment(s)" vs real "Assessment(s)", silent-empty); RegisterCertificate discarded the real
# ClientCertAuthSettings.OCSPUrl entirely; DescribeUpdateDirectory's wrapper key was fabricated
# ("UpdateDirectoryInfo" vs real "UpdateActivities") AND every entry's NewValue/PreviousValue were emitted as
# flat "" strings where the real type is a nested struct -- a real client's decode hard-failed, not just
# silent-empty; DescribeSettings' SettingEntry emitted the request-side filter field's name "Status" instead
# of the real response member "RequestStatus"; AcceptSharedDirectory returned only {SharedDirectoryId} where
# the real output is a full SharedDirectory object. None of these were caught by the prior passes' field-diffs
# against types.go, because a field-diff checks member SETS, not the top-level wrapper key or which request
# members are actually required -- see gopherstack-6flj 2026-08-15 Notes entry below. Grade held at A: every
# fix closes a real client-breaking bug rather than revealing a new unfixable gap. Previous pass
# (gopherstack-10hx 2nd follow-up, 2026-07-30) had already closed the AD-assessment AssessmentConfiguration
# gap that was the sole remaining reason this service was held at B; what remains unfixed (StatusCode/
# StatusReason/Version on Assessment; OsVersion/StageReason/etc. on Directory; the Settings DataType/Type
# lookup table; RadiusServersIpv6; ShareTarget.Type) is, in every case, AWS-internal or request-input data
# this in-memory backend has no way to derive without fabricating it. See gaps/deferred and the dated Notes
# sections for the evidence.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateDirectory: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "Requested->Creating->Active lifecycle goroutine. This pass added the real optional NetworkType request member (types.NetworkType: IPv4/IPv6/Dual-stack), validated and defaulted to IPv4 to match AWS's documented default."}
  CreateMicrosoftAD: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "NetworkType added, same as CreateDirectory."}
  DeleteDirectory: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades all dependent resources"}
  DescribeDirectories: {wire: FIXED, errors: ok, state: FIXED, persist: ok, note: "Prior pass fixed StageLastUpdatedDateTime and DnsIpAddrs. This pass field-diffed DirectoryDescription against types.DirectoryDescription (v1.41.0) and closed 7 of the 13 remaining gaps with real (non-fabricated) data: NetworkType (real request field, see CreateDirectory/CreateMicrosoftAD/ConnectDirectory), DnsIpv6Addrs (synthesized like DnsIpAddrs, only for Dual-stack/IPv6 directories), DesiredNumberOfDomainControllers (tracked on the directory itself for MicrosoftAD, defaulted to 2 and kept in sync by UpdateNumberOfDomainControllers), RegionsInfo (PrimaryRegion = the directory's home region, AdditionalRegions from the real dsRegions replication state, MicrosoftAD only), RadiusSettings+RadiusStatus (mirrored from the real per-directory RADIUS state tracked by radius.go, RadiusStatus=Completed once enabled since this backend has no async RADIUS provisioning), and ConnectSettings (new required ConnectSettings input on ConnectDirectory -- CustomerUserName/VpcId/SubnetIds/CustomerDnsIps/CustomerDnsIpsV6 -- now captured and echoed; ConnectIps synthesized like DnsIpAddrs since AWS-managed connector IPs have no real backing state. Also fixed AD Connector's top-level DnsIpAddrs, which was wrongly synthesizing a Directory Service address instead of echoing the customer's real self-managed DNS IPs -- confirmed against the SDK doc: 'For an AD Connector directory, these are the IP addresses of self-managed directory to which the AD Connector is connected.'). HybridSettings/OsVersion/OwnerDirectoryDescription/ShareMethod/ShareNotes/ShareStatus/StageReason remain unpopulated -- see gaps for why each one specifically cannot be derived honestly."}
  CreateAlias: {wire: ok, errors: ok, state: ok, persist: ok}
  EnableSso: {wire: ok, errors: ok, state: ok, persist: ok}
  DisableSso: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDirectoryLimits: {wire: ok, errors: ok, state: ok, persist: n/a, note: "computed, not stored"}
  CreateSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeSnapshots: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSnapshotLimits: {wire: ok, errors: ok, state: ok, persist: n/a}
  RestoreFromSnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "Restoring->Active lifecycle goroutine"}
  AddTagsToResource: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveTagsFromResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  AddIpRoutes: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "AddedDateTime was ISO8601 string, now awstime.Epoch"}
  RemoveIpRoutes: {wire: ok, errors: ok, state: ok, persist: ok}
  ListIpRoutes: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "AddedDateTime epoch fix"}
  AddRegion: {wire: FIXED, errors: FIXED, state: FIXED, persist: ok, note: "VPCSettings is a required AddRegionInput member (DirectoryVpcSettings{VpcId,SubnetIds}) that was silently dropped -- handler used the generic 2-field helper and never parsed it. Now required+parsed+stored+echoed. RegionType=Additional/Status=Active confirmed valid against types.RegionType/DirectoryStage enums (closes the deferred RegionType/RegionStatus item). gopherstack-wlo1 (2026-08-23): the already-in-Region check returned EntityAlreadyExistsException, a code AddRegion's own error switch does not type (only DirectoryAlreadyInRegionException is). Fixed."}
  RemoveRegion: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeRegions: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "LaunchTime epoch fix (prior pass); this pass added the RegionDescription fields that were completely absent: VpcSettings, DesiredNumberOfDomainControllers (defaulted to 2, AddRegion has no request field for it), StatusLastUpdatedDateTime. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  StartSchemaExtension: {wire: FIXED, errors: ok, state: FIXED, persist: ok, note: "2026-08-30 (reqfieldscan fifth-dispatch-shape sweep): CreateSnapshotBeforeSchemaExtension -- a required StartSchemaExtensionInput member -- was decoded and then silently dropped, never applied. Now takes a real Auto-type snapshot of the directory first when true (SnapshotTypeAuto added; visible via DescribeSnapshots, doesn't count against the manual-snapshot limit, matching the real op's documented behavior)."}
  CancelSchemaExtension: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSchemaExtensions: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "StartDateTime/EndDateTime epoch fix"}
  CreateConditionalForwarder: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "Re-diffed against types.ConditionalForwarder; found and closed a real gap -- DnsIpv6Addrs (a genuine optional CreateConditionalForwarderInput/ConditionalForwarder member) was entirely absent. Now accepted on input and round-tripped through Describe."}
  UpdateConditionalForwarder: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "same DnsIpv6Addrs fix as CreateConditionalForwarder"}
  DeleteConditionalForwarder: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConditionalForwarders: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "DnsIpv6Addrs now included in the response list"}
  CreateLogSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLogSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  ListLogSubscriptions: {wire: ok, errors: ok, state: ok, persist: ok, note: "SubscriptionCreatedDateTime epoch fix (prior pass). Re-diffed LogSubscription against types.LogSubscription this pass: DirectoryId/LogGroupName/SubscriptionCreatedDateTime is the full real member set -- genuinely clean, no gap."}
  RegisterEventTopic: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): a duplicate registration returned EntityAlreadyExistsException, a code RegisterEventTopic's own error switch does not type at all. Re-registration now succeeds (upsert) instead -- inferred idempotent behavior, not documented, since no already-exists exception exists to report otherwise."}
  DeregisterEventTopic: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEventTopics: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreatedDateTime epoch fix (prior pass). Re-diffed EventTopic against types.EventTopic this pass: CreatedDateTime/DirectoryId/Status/TopicArn/TopicName is the full real member set -- genuinely clean, no gap."}
  DescribeDomainControllers: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "LaunchTime epoch fix (prior pass). This pass field-diffed DomainController against types.DomainController (v1.41.0) and closed 5 of the 6 gaps flagged in the prior pass's deferred note: DnsIpAddr and DnsIpv6Addr (synthesized deterministically like Directory.DnsIpAddrs; DnsIpv6Addr only for Dual-stack/IPv6 directories), SubnetId/VpcId (echo the real parent directory's VpcSettings, cycled across controllers -- not fabricated), StatusLastUpdatedDateTime (stamped equal to LaunchTime at creation, since this backend's domain controllers never transition status after Active so there is no later real update to reflect). StatusReason remains unpopulated -- see gaps (no real failure state exists to produce a genuine reason string, and inventing status-message text would itself be a fabrication)."}
  UpdateNumberOfDomainControllers: {wire: ok, errors: ok, state: FIXED, persist: ok, note: "Now also updates the parent directory's DesiredNumberOfDomainControllers (see DescribeDirectories note)."}
  CreateTrust: {wire: FIXED, errors: FIXED, state: FIXED, persist: ok, note: "TrustDirection (required per SDK) and TrustPassword had zero presence validation; TrustDirection/TrustType/SelectiveAuth accepted any free-form string (closes the deferred TrustDirection/TrustType item) -- now validated against types.TrustDirection/TrustType/SelectiveAuth enums with InvalidParameterException on mismatch. SelectiveAuth was silently ignored and hardcoded to Disabled on every create despite being a real optional CreateTrustInput member -- now wired through."}
  DeleteTrust: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTrusts: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "CreatedDateTime/LastUpdatedDateTime epoch fix (prior pass); this pass found StateLastUpdatedDateTime and TrustStateReason were tracked in TrustInfo/storedTrust but never serialized into the response map at all -- both real Trust struct members, now included (TrustStateReason omitted when empty, matching AWS's null-omission)."}
  UpdateTrust: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "SelectiveAuth free-form (closes deferred item, now enum-validated); RequestId was fabricated by reusing TrustId instead of being a real per-request identifier (UpdateTrustOutput.RequestId is documented as the AWS request ID, not derived from TrustId) -- now uuid.NewString(), matching the CreateHybridAD/UpdateHybridAD RequestId pattern already used elsewhere in this service."}
  VerifyTrust: {wire: ok, errors: ok, state: ok, persist: ok}
  ShareDirectory: {wire: partial, errors: ok, state: FIXED, persist: ok, note: "HANDSHAKE now starts PendingAcceptance (was Shared, skipping the handshake); ORGANIZATIONS starts Shared (prior pass). Re-diffed the request shape this pass: real ShareDirectoryInput.ShareTarget is {Id, Type} where Type is TargetType (ACCOUNT/ORGANIZATION); this backend's ShareDirectory(ctx, directoryID, shareMethod, shareNotes, targetID) only accepts the target ID string and drops Type entirely. Not fixed this pass (request-input gap, not a response wire-shape defect -- SharedDirInfo/SharedDirectory has no Type member either in the real API, so no response is corrupted by this) -- see gaps."}
  UnshareDirectory: {wire: ok, errors: ok, state: ok, persist: ok}
  AcceptSharedDirectory: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj (2026-08-15): AcceptSharedDirectoryOutput.SharedDirectory is the full types.SharedDirectory object (api_op_AcceptSharedDirectory.go) -- the same shape DescribeSharedDirectories already emitted correctly. This handler returned only {SharedDirectoryId}; every other field (OwnerDirectoryId, OwnerAccountId, SharedAccountId, ShareMethod, ShareStatus, ShareNotes, CreatedDateTime, LastUpdatedDateTime) silently decoded to nil/zero on a real client. Fixed by sharing the same field-mapping helper (toSharedDirInfo) DescribeSharedDirectories already used."}
  RejectSharedDirectory: {wire: ok, errors: ok, state: FIXED, persist: ok, note: "was setting ShareStatus=RejectFailed (the AWS enum value for a FAILED reject) on every SUCCESSFUL reject; now Rejected"}
  DescribeSharedDirectories: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreatedDateTime/LastUpdatedDateTime epoch fix (prior pass). Re-diffed SharedDirInfo against types.SharedDirectory this pass: CreatedDateTime/LastUpdatedDateTime/OwnerAccountId/OwnerDirectoryId/ShareMethod/ShareNotes/ShareStatus/SharedAccountId/SharedDirectoryId is the full real member set -- genuinely clean, no response-shape gap. See gaps for a real (but request-side, not response-shape) ShareDirectory finding."}
  RegisterCertificate: {wire: FIXED, errors: FIXED, state: FIXED, persist: ok, note: "CLOSED the CommonName=example.com gap: CertificateData is documented as a real PEM string, so it is now decoded (encoding/pem) and parsed (crypto/x509); CommonName comes from cert.Subject.CommonName and ExpiryDateTime from cert.NotAfter (both previously fabricated/hardcoded). Unparseable CertificateData now returns the real InvalidCertificateException (was silently accepted). Type is now validated against CertificateType (ClientLDAPS/ClientCertAuth). gopherstack-6flj (2026-08-15): the real, optional ClientCertAuthSettings.OCSPUrl request member (types.ClientCertAuthSettings) was discarded entirely -- not read from the request, no field to hold it anywhere in this backend. Now captured and persisted; see DescribeCertificate for the echo side. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DeregisterCertificate: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): a missing certificate returned EntityDoesNotExistException, a code this op's own error switch does not type (only CertificateDoesNotExistException is). Fixed."}
  ListCertificates: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "ExpiryDateTime epoch fix. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException. FIXED (2026-08-29 list-filter-params pass) -- the wire key read for pagination was 'PageSize', a field that does not exist on ListCertificatesInput (the real field is 'Limit'); every real client's Limit was silently ignored. Fixed to read 'Limit'."}
  DescribeCertificate: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "RegisteredDateTime/ExpiryDateTime epoch fix. gopherstack-6flj (2026-08-15): Certificate.ClientCertAuthSettings (real, optional member) now echoes the OCSPUrl captured at RegisterCertificate time -- previously always absent since nothing captured it (see RegisterCertificate). gopherstack-wlo1 (2026-08-23): a missing certificate returned EntityDoesNotExistException, a code this op's own error switch does not type (only CertificateDoesNotExistException is). Fixed."}
  EnableLDAPS: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "Type accepted any free-form string; now validated against the LDAPSType enum (only Client is a valid value) -- closes deferred item. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DisableLDAPS: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "same LDAPSType validation as EnableLDAPS. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeLDAPSSettings: {wire: partial, errors: FIXED, state: ok, persist: ok, note: "LastUpdatedDateTime/CertificateExpiryDateTime epoch fix. gopherstack-6flj (2026-08-15, disclosed not fixed): real types.LDAPSSettingInfo is exactly {LDAPSStatus, LDAPSStatusReason, LastUpdatedDateTime} -- LDAPSType/CertificateId/CertificateExpiryDateTime are NOT real members of this shape at all (fabricated). Left in place rather than removed: no sensitive data, a real client simply ignores unknown JSON fields, and removing buys nothing testable. LDAPSStatusReason (real, optional) is genuinely omitted -- this backend tracks no LDAPS state-change reason anywhere. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  EnableClientAuthentication: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "Type is a required AWS input member but had no presence or enum check at all; now required + validated against ClientAuthenticationType (SmartCard/SmartCardOrPassword) -- closes deferred item. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DisableClientAuthentication: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "same Type validation as EnableClientAuthentication. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeClientAuthenticationSettings: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "LastUpdatedDateTime epoch fix. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException. FIXED (2026-08-29 list-filter-params pass) -- same 'PageSize' vs real 'Limit' wire-key bug as ListCertificates/ListADAssessments; additionally the backend accepted limit/nextToken params (marked nolint:revive 'existing issue') but never applied them at all -- no truncation, no pagination, ever. Both fixed: handler reads 'Limit', backend now truncates and returns a real cursor."}
  EnableRadius: {wire: partial, errors: ok, state: ok, persist: ok, note: "Re-diffed the EnableRadiusInput.RadiusSettings shape against types.RadiusSettings (the input variant): AuthenticationProtocol/DisplayLabel/SharedSecret/RadiusServers/RadiusPort/RadiusRetries/RadiusTimeout/UseSameUsername all captured correctly; RadiusServersIpv6 (a real optional input member) is not accepted -- see gaps. Bigger finding: this data was previously enable-only-write-never-read -- DirectoryDescription.RadiusSettings/RadiusStatus never mirrored it. Now fixed, see DescribeDirectories."}
  DisableRadius: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRadius: {wire: partial, errors: ok, state: ok, persist: ok, note: "same RadiusServersIpv6 gap as EnableRadius"}
  EnableDirectoryDataAccess: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DisableDirectoryDataAccess: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeDirectoryDataAccess: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  EnableCAEnrollmentPolicy: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "fixed (gopherstack-h910): request struct dropped the required PcaConnectorArn entirely, and CAEnrollmentPolicy had no field to hold it, so it was unrecoverable. Now decoded, required (InvalidParameterException if empty), and persisted. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DisableCAEnrollmentPolicy: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeCAEnrollmentPolicy: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "fixed (gopherstack-h910): the pre-fix response was a wholly fabricated shape -- a nested {\\"CAEnrollmentPolicy\\":{\\"EnrollmentStatus\\":\\"Enabled\\"/\\"Disabled\\"}} that does not exist on the real API at all. Real DescribeCAEnrollmentPolicyOutput is flat: CaEnrollmentPolicyStatus (enum InProgress/Success/Failed/Disabling/Disabled/Impaired, verified against types/enums.go), CaEnrollmentPolicyStatusReason, DirectoryId, LastUpdatedDateTime, PcaConnectorArn. All now wired; snapshot version bumped 1->2 since CAEnrollment's persisted value type changed from bool to *CAEnrollmentPolicy. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  StartADAssessment: {wire: FIXED, errors: FIXED, state: FIXED, persist: ok, note: "synchronous SUCCESS; AWS is async but no client-visible divergence for polling clients (prior-pass note, still true). gopherstack-10hx 2nd follow-up (2026-07-30): CLOSED the SEVERE finding from the prior pass -- StartADAssessmentInput.AssessmentConfiguration (types.AssessmentConfiguration: CustomerDnsIps, DnsName, InstanceIds, VpcSettings{VpcId,SubnetIds} required when supplied at all; SecurityGroupIds optional -- confirmed against the installed SDK's validateAssessmentConfiguration) is now accepted, required-field-validated (InvalidParameterException per missing member, matching the real validator's shape), and genuinely stored on storedADAssessment. UpdateHybridAD's internally-triggered assessment (no AssessmentConfiguration in the real API either) passes nil and is unaffected. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DeleteADAssessment: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj (2026-08-15): real DeleteADAssessmentInput is {AssessmentId} only (api_op_DeleteADAssessment.go) -- assessment IDs are globally addressable, not directory-scoped. This handler required DirectoryId too (via the generic handleTwoFieldOp helper, wrong for this one op), so every real typed client's Delete call was rejected outright with InvalidParameterException before reaching the backend. Now takes only AssessmentId."}
  DescribeADAssessment: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "StartTime epoch fix (prior pass); removed the fabricated 'Region' wire field and fixed the AssessmentType->ReportType/Operational->CUSTOMER fabrication (prior pass); AssessmentConfiguration round-trip (gopherstack-10hx 2nd follow-up). gopherstack-6flj (2026-08-15): two wire-breaking bugs found and fixed. (1) Same DirectoryId-required bug as DeleteADAssessment -- real DescribeADAssessmentInput is {AssessmentId} only, but this handler required DirectoryId too, so every real client's request was rejected outright. (2) The wrapper key was the fabricated 'ADAssessment', not the real 'Assessment' (DescribeADAssessmentOutput.Assessment) -- even a request that got past bug (1) would have decoded resp.Assessment as nil on every call. Both fixed; a real-SDK-client test (wire_field_fixes_test.go) round-trips Start->List->Describe->Delete. StatusCode/StatusReason/Version and AssessmentReports remain genuinely unpopulated -- see gaps (AWS-internal assessment-engine output with no request input and no documented deterministic default; same class of honest gap as Directory.OsVersion)."}
  ListADAssessments: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "StartTime epoch fix (prior pass); same Region/ReportType fabrication fix as DescribeADAssessment (prior pass); AssessmentConfiguration round-trip (gopherstack-10hx 2nd follow-up). gopherstack-6flj (2026-08-15): the wrapper key was the fabricated 'ADAssessments', not the real 'Assessments' (ListADAssessmentsOutput.Assessments) -- every real client's resp.Assessments field silently decoded to nil/empty on every call regardless of how many assessments existed. Fixed; confirmed against types.AssessmentSummary that SecurityGroupIds/SelfManagedInstanceIds/SubnetIds/VpcId/StatusCode/StatusReason/Version are Assessment-only (Describe) members and correctly do NOT appear here -- a dedicated test (TestStartADAssessment_ConfigurationRoundTrip) asserts their absence on List and presence on Describe. FIXED (2026-08-29 list-filter-params pass) -- same 'PageSize' vs real 'Limit' wire-key bug as ListCertificates/DescribeClientAuthenticationSettings; every real client's Limit was silently ignored."}
  CreateHybridAD: {wire: FIXED, errors: ok, state: ok, persist: ok, note: "gopherstack-10hx: real input {AssessmentId, SecretArn, Tags} (both required, matching validateOpCreateHybridADInput exactly); real output is {DirectoryId} only -- the fabricated RequestId is gone. AssessmentId must reference an existing, real assessment (adAssessmentGet) with Status==SUCCESS (ErrAssessmentNotFound / ErrInvalidParameter otherwise). Name/ShortName/Description/Edition are NOT real input members (confirmed against types.CreateHybridADInput) -- AWS derives them from the assessment's own AssessmentConfiguration.DnsName, which this backend cannot capture (StartADAssessment doesn't accept AssessmentConfiguration -- see StartADAssessment gap, out of scope for gopherstack-10hx). Rather than fabricate a domain name, this backend snapshots the assessed directory's real Name/ShortName/Description/Edition onto the storedADAssessment record at StartADAssessment time and derives the new hybrid directory from that -- genuinely real, non-invented data, at the cost of CreateHybridAD requiring its AssessmentId to trace back to an existing directory (this backend's only supported assessment mode) rather than AWS's normal directory-less pre-creation assessment. Documented as a deliberate, bounded compromise -- see Notes."}
  UpdateHybridAD: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "gopherstack-10hx: real input {DirectoryId (required) + optional HybridAdministratorAccountUpdate{SecretArn} and/or SelfManagedInstancesSettings{CustomerDnsIps,InstanceIds}, at least one required, matching validateOpUpdateHybridADInput}; real output {AssessmentId, DirectoryId} -- the fabricated RequestId is gone. AssessmentId is now REAL: UpdateHybridAD triggers an actual assessment via the same startADAssessmentLocked path StartADAssessment uses (real, since UpdateHybridAD always targets an existing directory). SelfManagedInstancesSettings now genuinely mutates state: storedDirectory.HybridDNSIPs/HybridInstanceIDs, which DescribeDirectories' HybridSettings now reads (closing that companion gap -- see families). HybridAdministratorAccountUpdate.SecretArn is validated present and discarded, matching the real 'used once and not stored' contract. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeHybridADUpdate: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "gopherstack-10hx: real output UpdateActivities{HybridAdministratorAccount: []HybridUpdateInfoEntry, SelfManagedInstances: []HybridUpdateInfoEntry} (types.HybridUpdateActivities), each entry AssessmentId/InitiatedBy/LastUpdatedDateTime/NewValue/PreviousValue/StartTime/Status/StatusReason (NewValue/PreviousValue are HybridUpdateValue{DnsIps,InstanceIds}, omitted when empty matching the real serializer) -- the fabricated flat {RequestId,DirectoryId,Status} list is gone. UpdateType request filter validated against the real enum. NextToken is accepted but this backend returns every matching entry in one page (no cursor pagination modeled) -- SDK-valid (NextToken omitted means no more pages, truthfully) but a real simplification, noted here not hidden. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  CreateComputer: {wire: ok, errors: ok, state: ok, persist: n/a, note: "AWS has no Describe/List for computer accounts either; not persisting matches the real API's surface"}
  UpdateSettings: {wire: ok, errors: FIXED, state: ok, persist: ok, note: "gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeSettings: {wire: partial, errors: FIXED, state: ok, persist: ok, note: "LastUpdatedDateTime epoch fix (prior pass). Re-diffed SettingEntry against types.SettingEntry this pass: found a real gap -- Name/AllowedValues/AppliedValue/RequestedValue/LastUpdatedDateTime/Status(->RequestStatus) are covered, but DataType, LastRequestedDateTime, RequestDetailedStatus (a per-region map[string]DirectoryConfigurationStatus), RequestStatusMessage, and Type are real members with no equivalent in storedDirectorySetting at all. Not fixed this pass: DataType/Type are per-known-setting-name metadata (e.g. TLS_1_0 -> DataType=Enum, Type=Protocol) that would require a static lookup table of every real Directory Service setting name, and getting that table wrong would itself be a fabrication risk -- see gaps. gopherstack-6flj (2026-08-15): the prior note's own parenthetical '(->RequestStatus)' correctly named the real field but the code never made that rename -- SettingEntry has NO 'Status' member at all; the response emitted the request-side filter field's name (DescribeSettingsInput.Status) instead of the real response member RequestStatus, so a real client's RequestStatus always decoded to its zero value. Fixed (real key from the wrong side). gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  UpdateDirectorySetup: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "UpdateType is a required AWS input member but had no presence or enum check; now required + validated against UpdateType (OS/NETWORK/SIZE) -- closes deferred item. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  DescribeUpdateDirectory: {wire: FIXED, errors: FIXED, state: ok, persist: ok, note: "StartTime/LastUpdatedDateTime epoch fix. gopherstack-6flj (2026-08-15): two more bugs found and fixed. (1) Wrapper key was the fabricated 'UpdateDirectoryInfo', not the real 'UpdateActivities' (DescribeUpdateDirectoryOutput.UpdateActivities) -- silent-empty on every call. (2) Every entry's NewValue/PreviousValue were emitted as flat \\"\\" strings; the real types.UpdateInfoEntry member type is *types.UpdateValue{OSUpdateSettings}, a nested struct -- a real client's decode hard-failed with a type-mismatch error on every call that returned at least one entry (which is every call after any UpdateDirectorySetup), not just silent-empty. This backend never populates real NewValue/PreviousValue content (always the Go zero value for any UpdateType, not just OS), so both are now omitted rather than fabricated into the nested shape. gopherstack-wlo1 (2026-08-23): directory-not-found returned EntityDoesNotExistException, a code this op's own deserializeOpError switch does not type (only DirectoryDoesNotExistException is) -- errors.As into the real client's typed exception failed. Fixed to DirectoryDoesNotExistException."}
  ResetUserPassword: {wire: ok, errors: ok, state: ok, persist: n/a}
  ConnectDirectory: {wire: FIXED, errors: FIXED, state: FIXED, persist: ok, note: "Real ConnectDirectoryInput requires ConnectSettings{CustomerUserName, VpcId, SubnetIds required; CustomerDnsIps/CustomerDnsIpsV6 optional} -- this backend previously accepted no connect-settings input at all. Now required and validated (InvalidParameterException if CustomerUserName/VpcId/SubnetIds absent), stored, and surfaced via DirectoryDescription.ConnectSettings; see DescribeDirectories note."}
# Families audited as a group (when per-op is impractical):
families:
  persistence-registration: {status: FIXED, note: "Handler lacked Snapshot(ctx)/Restore(ctx,[]byte) delegation methods, so cli.go's setupPersistence type-assertion against the local `persistable` interface silently failed and directoryservice was NEVER registered with the persistence manager -- a fully-correct BackendSnapshot/Restore on InMemoryBackend was completely unreachable. Fixed by adding delegation methods to Handler (handler.go)."}
  timestamps: {status: FIXED, note: "22 call sites across handler_appendixa.go formatted timestamps as ISO8601 strings (time.Format(\"2006-01-02T15:04:05.000Z\")); confirmed against aws-sdk-go-v2 directoryservice deserializers.go that every timestamp field uses smithytime.ParseEpochSeconds (JSON number), so real SDK clients would fail to deserialize every affected List/Describe response. All converted to awstime.Epoch."}
  sdk_completeness: {status: ok, note: "sdk_completeness_test.go verifies every dssdk.Client op is in GetSupportedOperations(); notImplemented is empty -- full op coverage."}
  error-taxonomy: {status: FIXED, note: "Systemic error-code bug across ~90 validation call sites in ~20 handler_*.go files: every request-validation failure (missing required field, invalid enum value) returned __type=\"ClientException\" instead of AWS's real InvalidParameterException (confirmed as a distinct documented exception in types/errors.go, present in nearly every op's real Errors list). Also fixed the dead-but-wrong mapError case for the backend awserr.ErrInvalidParameter sentinel (was also \"ClientException\"). Left \"invalid body\"/\"invalid JSON\" transport-parse failures and the backend awserr.ErrConflict case as ClientException (defensible: not a documented-parameter-value problem). Every corresponding test assertion updated; see handler_directories_test.go/handler_directories_extra_test.go/handler_test.go for the renamed expectations."}
  directory-domaincontroller-field-diff: {status: FIXED, note: "This pass's primary target: full field-diff of DirectoryDescription and DomainController against types.go (v1.41.0). Closed the DirectoryDescription gaps ConnectSettings/DesiredNumberOfDomainControllers/DnsIpv6Addrs/NetworkType/RadiusSettings/RadiusStatus/RegionsInfo (6 of 13) and the DomainController gaps DnsIpAddr/DnsIpv6Addr/StatusLastUpdatedDateTime/SubnetId/VpcId (5 of 6) with real, derivable data -- none synthesized where a real source existed (VpcSettings/RADIUS state/replication state/request input), only IP addresses use the pre-existing synthesize* deterministic-placeholder convention where AWS's real value is genuinely unknowable to an in-memory backend. HybridSettings/OsVersion/OwnerDirectoryDescription/ShareMethod/ShareNotes/ShareStatus/StageReason (DirectoryDescription) and StatusReason (DomainController) remain unpopulated -- see gaps for why each specifically cannot be derived honestly right now."}
  re-diff-ok-families: {status: FIXED, note: "Re-diffed every family the prior pass's deferred note flagged as untrusted (conditional forwarders, log subscriptions, event topics, schema extensions, radius, shared directories, hybrid AD, AD assessments, settings) against v1.41.0 types.go. Result, matching the prior pass's warning that 'ok' marks were weak evidence: log-subscriptions and event-topics are genuinely clean (verified 1:1 field match, no changes). conditional-forwarders had a real gap (DnsIpv6Addrs missing, FIXED). schema-extensions has a real gap (SchemaExtensionStatusReason missing, NOT fixed -- no real value to derive it from). radius had a real gap (DirectoryDescription never mirrored the RADIUS state at all, FIXED; RadiusServersIpv6 missing from EnableRadius/UpdateRadius input, NOT fixed). shared-directories response shape is genuinely clean; the request shape has a real gap (ShareTarget.Type dropped, NOT fixed). settings has a real gap (DataType/LastRequestedDateTime/RequestDetailedStatus/RequestStatusMessage/Type missing from SettingEntry, NOT fixed -- no safe way to derive DataType/Type without a lookup table this pass couldn't verify). hybrid-AD and AD-assessments both had SEVERE, previously-undetected gaps: AD-assessments had two outright fabricated wire fields (invented 'Region' field, invented 'AssessmentType' field name with hardcoded invalid value 'Operational') -- FIXED (fabrication deleted, real ReportType/CUSTOMER substituted) -- but the operation still can't accept real AssessmentConfiguration input, so most of Assessment's real fields remain unreachable (NOT fixed, large gap). hybrid-AD's CreateHybridAD/UpdateHybridAD/DescribeHybridADUpdate wire shapes are substantially wrong (not just missing fields -- wrong required input members, wrong output members, invented RequestId) -- NOT fixed this pass, see the ops table and gaps. UPDATE (gopherstack-10hx, 2026-07-30 follow-up pass): hybrid-AD's wire-shape gap is now FIXED -- see the ops table and the dated Notes section below. UPDATE (gopherstack-10hx, 2026-07-30 2nd follow-up pass): AD-assessments' AssessmentConfiguration input-capture gap is now also FIXED -- see the ops table and the dated Notes section below. StatusCode/StatusReason/Version remain honestly unpopulated (AWS-internal, no request input, no documented default) -- not a fabrication gap, see gaps."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "DirectoryDescription still does not populate: OsVersion (AWS assigns this internally with no request input and no documented deterministic default -- genuinely unknowable to an in-memory backend); OwnerDirectoryDescription/ShareMethod/ShareNotes/ShareStatus (these describe the directory-CONSUMER's copy of a shared directory -- AcceptSharedDirectory in this backend updates the existing storedSharedDirectory record but never materializes a second Directory entry in the consumer's own DescribeDirectories view, so there is no directory record these fields could attach to; DescribeSharedDirectories already exposes the real ShareMethod/ShareNotes/ShareStatus for the owner-tracked share record, so this data is not lost, just not duplicated onto a nonexistent consumer-side Directory); StageReason (only ever populated by AWS on a failed stage transition, and this backend's Requested->Creating->Active/Restoring->Active lifecycles never fail, so there is genuinely never a reason to report -- always nil is the honest value, not a fabricated placeholder). HybridSettings is now populated (gopherstack-10hx, 2026-07-30) -- see families/hybrid-AD."
  - "DomainController.StatusReason is never populated: AWS only sets this when a domain controller enters a Failed/Impaired state, and this backend's UpdateNumberOfDomainControllers only ever creates controllers directly into Active -- there is no real failure state to describe, and inventing status-message text would be a fabrication."
  - "hybrid-AD (CreateHybridAD/UpdateHybridAD/DescribeHybridADUpdate) wire-shape divergence: FIXED, gopherstack-10hx (2026-07-30) -- see families and the ops table. Residual, deliberately-scoped compromise: CreateHybridAD's AssessmentId must reference an assessment of an EXISTING directory (this backend's only supported StartADAssessment mode), not AWS's normal directory-less pre-creation assessment (AssessmentConfiguration input capture -- see the StartADAssessment gap below -- is what a fully-real fix would need); this backend derives the new hybrid directory's Name/ShortName/Description/Edition from that assessed directory's own real, already-existing values rather than fabricating them. Documented in CreateHybridAD's ops-table note and PARITY.md Notes; not hidden."
  - "AD-assessments (StartADAssessment/DescribeADAssessment/ListADAssessments): FIXED, gopherstack-10hx 2nd follow-up (2026-07-30). StartADAssessment now accepts, required-field-validates (InvalidParameterException, matching the real SDK's validateAssessmentConfiguration shape), and genuinely stores the real StartADAssessmentInput.AssessmentConfiguration member (CustomerDnsIps, DnsName, InstanceIds, VpcSettings{VpcId,SubnetIds}, SecurityGroupIds). DescribeADAssessment's Assessment now reports the real, non-fabricated CustomerDnsIps/DnsName/LastUpdateDateTime/SecurityGroupIds/SelfManagedInstanceIds/SubnetIds/VpcId; ListADAssessments' AssessmentSummary reports the correct real SUBSET (CustomerDnsIps/DnsName/LastUpdateDateTime only -- confirmed against types.AssessmentSummary that the other four are Assessment-only). Remaining, honestly-unpopulated: StatusCode, StatusReason, Version -- AWS documents these as assessment-engine-internal output (a detailed status code, a human-readable status/error message, an assessment-framework version) with no request input and no documented deterministic default; same class of gap as Directory.OsVersion (see above) and DomainController.StatusReason, not a fabrication risk. This was the sole remaining reason directoryservice's overall grade was held at B; with input capture closed and only AWS-internal, genuinely-unknowable metadata left, the grade is raised to A this pass -- see overall note. (Prior-pass fix retained: Assessment.Status's non-enum 'Completed' -> 'SUCCESS'.)"
  - "SettingEntry (DescribeSettings) is missing DataType, LastRequestedDateTime, RequestDetailedStatus (a per-region map[string]DirectoryConfigurationStatus), RequestStatusMessage, and Type -- confirmed against types.SettingEntry. DataType/Type are AWS-documented per-setting-name metadata (e.g. TLS_1_0 -> DataType=Enum, Type=Protocol) that would require a static lookup table of every real Directory Service setting name to populate correctly; this pass could not verify such a table's completeness/accuracy against AWS's docs with confidence, and getting it wrong would itself be a fabrication, so it was left out rather than guessed."
  - "EnableRadius/UpdateRadius (and the resulting DirectoryDescription.RadiusSettings) do not accept/expose RadiusServersIpv6, a real optional member of both the input and output RadiusSettings shapes -- this backend's storedRadiusSettings/RadiusSettingsInput/RadiusSettingsDescription have no IPv6 RADIUS server support modeled at all."
  - "ShareDirectory's real ShareTarget input is {Id, Type} where Type is TargetType (ACCOUNT/ORGANIZATION); this backend's ShareDirectory(ctx, directoryID, shareMethod, shareNotes, targetID) only accepts the target ID and silently drops Type. This is a request-input gap, not a response-shape defect (SharedDirInfo/SharedDirectory has no Type member in the real API either, confirmed genuinely clean this pass), so no wire response is corrupted by it, but a client that relies on Type-based validation (e.g. rejecting an ORGANIZATION-typed target when the caller isn't in an Organization) would see no such validation here."
  - StartADAssessment/CreateTrust/ShareDirectory etc. complete synchronously instead of AWS's async in-progress states (e.g. no "Creating"/"Sharing"/"Verifying" transient states observable by a fast poller); acceptable for emulation, but a client that asserts on an intermediate state would diverge (gopherstack-g2eo, 2026-09-07: re-examined at enum granularity -- TrustState declares 11 values, this backend reaches 2 (Created via CreateTrust, Verified via VerifyTrust); SnapshotStatus declares 3, this backend reaches 1 (Completed via CreateSnapshot). The other 9/2 are all async-only in real AWS too: VerifyTrust's own doc says it "initiates" verification -- Verifying/VerifyFailed depend on real connectivity to an external domain this backend cannot simulate; Creating/Updating/Deleting are transient windows this backend collapses by completing instantly; CreateSnapshotOutput has no status field at all, confirming Creating/Failed are only ever observed via async polling. Same class as the ram/emrserverless/stepfunctions precedents (gopherstack-9ojs/3vyq/kx95): reaching them needs a background ticker, out of scope. Verdict: modelling gap, not a defect; no code changed.)
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "Settings DataType/Type static lookup table (see gaps): would need to be built and verified against AWS's own Directory Service setting-name documentation, not guessed."
  - "hybrid-AD's directory-less pre-creation assessment mode (see the hybrid-AD gap entry above): now that StartADAssessment genuinely captures AssessmentConfiguration.DnsName, CreateHybridAD could in principle derive a new hybrid directory's descriptive fields from an assessment with no backing DirectoryId, matching AWS's normal flow more closely than the current existing-directory-only compromise. NOT attempted this pass -- out of scope for the AD-assessment-configuration gap this pass targeted, and CreateHybridAD's existing-directory requirement is not itself blocking directoryservice's grade (see hybrid-AD gap note)."
leaks: {status: clean, note: "transitionDirectoryToActive and RestoreFromSnapshot's goroutine both self-terminate after two bounded time.Sleep stages (50ms/100ms); no unbounded loops, no leaked tickers/timers; isolation_test.go and the -race gate confirm no cross-region/cross-goroutine data races. This pass added no new goroutines/tickers/locks -- setStage() is a plain synchronous helper called under the existing b.mu lock, verified by -race."}
---

## Notes

### 2026-08-29 (list-filter-params sweep: parameters declared and never honoured)

Measured all ~16 collection-returning operations (14 `List*`/`Describe*` ops returning
arrays, verified by SDK output shape) and every constraining parameter each declares in
its own `api_op_<Op>.go` Input struct. Found no never-read filter parameters in this
service -- every real filter (`RemoteDomainNames`, `DomainControllerIds`, `TopicNames`,
`RegionName`, `SharedDirectoryIds`, `SnapshotIds`, `TrustIds`, `Status`, `Type`,
`UpdateType`, `DirectoryIds`) was already correctly read and applied by its handler and
backend. Found 3 wrong-wire-key bugs instead, adjacent to but distinct from the
declared-and-never-read class this pass targeted: `ListADAssessments`, `ListCertificates`,
and `DescribeClientAuthenticationSettings` all read a JSON key `"PageSize"` that does not
exist on their real Input structs (the real field is `"Limit"` on all three) -- a real
client's `Limit` was silently ignored, same observable symptom as the declared-and-unread
class but a different root cause (this repo's prior wire-key sweep didn't catch these
three). Fixed all three to read `"Limit"`; fixing `DescribeClientAuthenticationSettings`
also exposed that its backend accepted `limit`/`nextToken` params but never applied them
at all (marked `//nolint:revive // existing issue` -- a landmine comment describing a real
bug, not a false positive) -- added real truncation/cursor logic matching the pattern
already used by `DescribeDomainControllers` et al. in this same file family. A pre-existing
test (`TestListCertificates_Pagination`) asserted pagination worked using the same wrong
`"PageSize"` key the bug read -- a textbook case of a test asserting wrong behavior as
correct; fixed to use `"Limit"`. No parameter-parsed-then-discarded-to-`_` cases, no
handler skipping its request body, and no never-truncating pagination were found in this
service this pass.

Protocol: AWS JSON 1.1 (`X-Amz-Target: DirectoryService_20150416.<Op>`, error shape
`{"__type": "<Code>Exception", "message": "..."}`). Confirmed against
aws-sdk-go-v2/service/directoryservice@v1.38.20's deserializers.go: **every** timestamp
field in this API (LaunchTime, StartTime, CreatedDateTime, LastUpdatedDateTime,
ExpiryDateTime, AddedDateTime, etc.) is deserialized via `smithytime.ParseEpochSeconds`,
i.e. wire format is a JSON number of seconds since epoch, NEVER an ISO8601 string. This
is the single biggest wire-shape trap in this service — the base handler.go (audited in
an earlier pass) already used `awstime.Epoch` correctly for Directory.LaunchTime and
Snapshot.StartTime, but every Appendix-A handler (added in a later pass, before this
service had a PARITY.md) used `time.Format("2006-01-02T15:04:05.000Z")` instead. Any
future op added to this service MUST use `awstime.Epoch(...)`, never
`.Format(time.RFC3339...)` or similar — the sdk_completeness test won't catch this class
of bug since it only checks operation-name coverage, not wire shape.

`ShareStatus` enum (aws-sdk-go-v2/service/directoryservice/types/enums.go): Shared,
PendingAcceptance, Rejected, Rejecting, RejectFailed, Sharing, ShareFailed, Deleted,
Deleting. `RejectFailed` means the *reject operation itself* failed asynchronously — it
is NOT the terminal state of a successful reject (that's `Rejected`). Easy to invert by
pattern-matching the string "Reject" without checking AWS's actual semantics; the bug
fixed this pass had exactly this shape.

`ShareMethod` enum: ORGANIZATIONS, HANDSHAKE. Only HANDSHAKE requires the consumer
account to call AcceptSharedDirectory (initial ShareStatus = PendingAcceptance);
ORGANIZATIONS shares are active immediately (ShareStatus = Shared). The handler defaults
ShareMethod to "HANDSHAKE" when the request omits it (matches AWS's own default).

2026-08-13 pass (`gopherstack-h910`, required-member sweep pass 5): a required-member miss
on `EnableCAEnrollmentPolicy` (`PcaConnectorArn` dropped) led to finding
`DescribeCAEnrollmentPolicy`'s response was a wholly fabricated shape -- a required-member
miss reliably smells of a wholly wrong shape, exactly as this class of bug has looked
before. The pre-fix response nested a boolean-derived `EnrollmentStatus` string under a
`CAEnrollmentPolicy` key that doesn't exist on the real API; the real
`DescribeCAEnrollmentPolicyOutput` is flat with a six-value status enum
(`CaEnrollmentPolicyStatus`: InProgress/Success/Failed/Disabling/Disabled/Impaired). Fixed
both the request and response shape; `CAEnrollmentPolicy`'s persisted representation
changed from a bare `bool` to a struct carrying `PcaConnectorArn`/`Status`/
`LastUpdatedDateTime`, so `directoryserviceSnapshotVersion` was bumped 1->2 (an existing
field's type changed, not a pure addition -- confirmed via
`pkgs/persistence/snapshotversion_guard_test.go`'s `TestSnapshotVersionGuard`, which
otherwise refuses exactly this kind of silent-data-loss bump).

Directory lifecycle (Stage enum): Requested → Creating → Active, each transition on its
own goroutine with a fixed delay (`directoryLifecycleDelay` = 50ms) — this is real state
mutation, not a fabricated instant-Active response. RestoreFromSnapshot similarly drives
Active → Restoring → Active via `restoreLifecycleDelay` (100ms). Both were verified to
actually flip backend state (not just return success) by reading backend.go directly,
per the parity-principles.md "grep for stubs has false positives" caveat — these looked
suspicious as "fire-and-forget goroutine" patterns at first glance but are correct.

Sixteen resource collections use the region-qualified `store.Table[V]` +
`store.Index[V]` pattern (`store_setup.go`); six raw maps (aliases, ipRoutes,
dirDataAccess, caEnrollment, dirSettings, updateInfoEntries) were deliberately left as
plain `map[string]map[string]V` because their values have no independent identity to key
a `store.Table` by — this is documented in-line and is correct, not a shortcut.
`InMemoryBackend` uses the single coarse `lockmetrics.RWMutex` correctly: every method
takes the full backend lock for its whole read/write, matching the "lock granularity
follows invariant granularity" rule (a `DeleteDirectory` cascade touches 12+ tables
atomically).

CreateComputer intentionally does not persist a computer record: AWS Directory Service's
own API surface has no Describe/List operation for computer accounts (they're plain AD
objects, not DS-managed resources), so there's nothing a client could read back — this
looked like a disguised no-op at first glance (RLock used for a "create" op, nothing
stored) but is correct emulation of an op whose only observable effect in real AWS is the
synchronous response itself.

`InvalidParameterException` vs `ClientException` (2026-07-23 pass): both are real,
distinct AWS Directory Service exception types (types/errors.go). Prior code used
`ClientException` uniformly for every request-validation failure. AWS's own per-op Errors
lists document `InvalidParameterException` as the code for "one or more parameters are not
valid" (missing required members, invalid enum values) — that is what this service's
handler-level and backend-level validation checks actually detect, so they now return
`InvalidParameterException`. `ClientException` is now reserved for cases that are not a
specific documented parameter problem: malformed/unparseable request bodies ("invalid
body"/"invalid JSON") and the generic backend `awserr.ErrConflict` sentinel. Any new
validation check added to this service should return `InvalidParameterException`, not
`ClientException`, unless it's genuinely one of those two exempted cases.

`setStage(d *storedDirectory, stage DirectoryStage)` (directories.go) is the single place
that mutates `storedDirectory.Stage`; it also stamps `StageLastUpdatedDateTime = now()`.
Both the create lifecycle (`transitionDirectoryToActive`) and the restore lifecycle
(`RestoreFromSnapshot`'s goroutine) now go through it. Any future code that flips
`Directory.Stage` directly instead of calling `setStage` will silently reintroduce the
StageLastUpdatedDateTime bug class named in this campaign's brief.

`synthesizeDNSIPAddrs(directoryID string) []string` (directories.go) derives two
deterministic `10.0.x.y`-shaped addresses from a SHA-256 of the directory ID for the
`DnsIpAddrs` field. This is a synthesized-but-consistent value (same directory ID always
yields the same IPs, matching real AWS's stable-per-directory DNS IPs), not a random
placeholder — documented here so a future auditor doesn't mistake it for a fabricated/stub
value the "no stub" rule would forbid; the alternative (omitting the field, as before) was
the actual parity bug.

### 2026-07-30 pass: DirectoryDescription/DomainController field-diff + re-diff of "ok" families

`synthesizeDNSIPv6Addrs`/`synthesizeDomainControllerDNSIPv6Addr` (directories.go /
domain_controllers.go) extend the same synthesized-but-consistent convention to
`fd00:ec2::`-shaped ULA IPv6 addresses, used only when a directory's `NetworkType` is
`Dual-stack` or `IPv6` (i.e. only when the directory is IPv6-capable in the first place —
an IPv4-only directory correctly reports no `DnsIpv6Addrs`).

AD Connector's `DnsIpAddrs` is **not** synthesized like every other directory type's: AWS's
own doc for `DirectoryDescription.DnsIpAddrs` says "For an AD Connector directory, these are
the IP addresses of self-managed directory to which the AD Connector is connected" — i.e. the
*customer's own* DNS servers, which this backend now genuinely has (as real
`ConnectSettings.CustomerDnsIps` request input) rather than a fabricated Directory
Service-managed address. `DirectoryConnectSettingsDescription.ConnectIps` (the AD
Connector's *own* AWS-managed IPs, a different field) is still synthesized, since that
network state genuinely has no backing in an in-memory backend.

`Directory.DesiredNumberOfDomainControllers` is tracked per-directory (not just per-Region
as before) for MicrosoftAD directories only, defaulting to `defaultRegionDomainControllers`
(2) at creation and kept in sync by `UpdateNumberOfDomainControllers`. AWS docs confirm this
field is `nil` for non-MicrosoftAD directory types ("if the directory is Microsoft AD"),
matching this backend's `DirectoryType(d.DirType) == DirectoryTypeMicrosoftAD` gate.

`Directory.RegionsInfo` is assembled at read time (`describeDirectory` in directories.go, not
`storedDirectory.toDirectory`) because it needs cross-table state (`dsRegions`) that a plain
struct-to-struct mapper can't see. `PrimaryRegion` is always the directory's own home region
(`d.region`); `AdditionalRegions` comes from real `AddRegion` replication state, never
fabricated. Same rationale for `Directory.RadiusSettings`/`RadiusStatus`, assembled from the
real per-directory RADIUS state `radius.go` already tracks — this data existed and was
correctly stored, it just was never mirrored onto the summary object real AWS clients read
most often (`DescribeDirectories`), which is the single highest-value fix in this pass.

The AD-assessment fabrication found this pass (`AssessType: "Operational"` under the wire key
`"AssessmentType"`) is a good example of the "hidden gap behind an `ok` mark" pattern the
prior pass's deferred note warned about: neither the field name nor the value existed in the
real SDK, yet every test in the package passed, because no test asserted on that specific
key/value pair. The fix (`"ReportType": "CUSTOMER"`) is deliberately narrow — it corrects what
was actively wrong without expanding scope into the AssessmentConfiguration input-capture
work that would be needed to populate the rest of `Assessment`'s real fields (see gaps).

hybrid-AD was the other severe finding: unlike every other family in this service, its
request/response shapes were not incrementally wrong (a missing optional field here, a wrong
timestamp format there) but *structurally* wrong — required input members that don't exist in
the real API, a fabricated output field (`RequestId`) that doesn't exist in either real output
type, and a response shape (`UpdateType`-keyed activity log) that has no relationship at all to
what this backend returns. This was deliberately left unfixed rather than partially patched: a
half-fixed hybrid-AD (e.g. renaming `RequestId` to `AssessmentId` without actually linking to a
real assessment record) would look more correct than it is, which is worse than an honestly
documented gap.

### 2026-07-30 follow-up pass: gopherstack-10hx (hybrid-AD real-shape fix)

Closed the SEVERE hybrid-AD finding from the same-day audit pass above by field-diffing
`types.CreateHybridADInput`/`Output`, `types.UpdateHybridADInput`/`Output`,
`types.DescribeHybridADUpdateInput`/`Output`, `types.HybridUpdateActivities`,
`types.HybridUpdateInfoEntry`, `types.HybridUpdateValue`,
`types.HybridAdministratorAccountUpdate`, and `types.HybridCustomerInstancesSettings` directly
against `aws-sdk-go-v2/service/directoryservice@v1.41.0`'s `types/types.go`, `serializers.go`,
`deserializers.go`, and `validators.go`.

**The AssessmentId-linkage problem.** Real `CreateHybridADInput` is `{AssessmentId, SecretArn,
Tags}` — AWS derives the new hybrid directory's `Name` etc. entirely from the referenced
assessment's own `AssessmentConfiguration.DnsName`. This backend cannot capture
`AssessmentConfiguration` at all (see the `StartADAssessment` gap, explicitly out of scope for
this ticket), so there was no way to honestly source a domain name for a brand-new directory —
inventing one (even a deterministic, `synthesizeDNSIPAddrs`-style placeholder) would fabricate
customer-meaningful data, unlike that helper's genuinely-opaque-to-the-client IP addresses.
Resolution: `StartADAssessment` (`ad_assessments.go`) now snapshots the assessed directory's
real `Name`/`ShortName`/`Description`/`Edition` onto the `storedADAssessment` record
(`SourceDirectory*` fields — internal bookkeeping only, never surfaced by
`DescribeADAssessment`/`ListADAssessments`, confirmed not real `Assessment`/`AssessmentSummary`
members). `CreateHybridAD` derives the new hybrid directory from that snapshot, so
`hybridDirID != assessedDirID` (a genuinely new `Directory` record is created, matching real
AWS's "always mints a new DirectoryId" behavior) while every descriptive field is real,
previously-existing data, not invented. The trade-off, stated plainly: this backend's
`CreateHybridAD` can only be driven off an assessment of an *existing* directory (this
backend's only supported `StartADAssessment` mode), not AWS's normal directory-less
pre-creation assessment flow. Fixing that fully requires the separately-tracked
`AssessmentConfiguration` input-capture gap.

**The RequestId fabrication.** All three ops' `RequestId` field — present on none of the real
output types (`CreateHybridADOutput` is `{DirectoryId}`; `UpdateHybridADOutput` is
`{AssessmentId, DirectoryId}`; `DescribeHybridADUpdateOutput` is
`{NextToken, UpdateActivities}`) — is gone. `UpdateHybridADOutput.AssessmentId` is now a REAL
assessment ID: `UpdateHybridAD` triggers an actual assessment via the same
`startADAssessmentLocked` helper `StartADAssessment` itself uses (refactored out of
`StartADAssessment` to allow reentrant use under an already-held write lock — `UpdateHybridAD`
holds `b.mu.Lock` for its own state changes and cannot re-acquire it). This is genuinely real,
not fabricated, because `UpdateHybridAD` always targets an *existing* directory — exactly the
one assessment mode this backend supports.

**HybridSettings, closed as a side effect.** `DirectoryDescription.HybridSettings` was
previously always `nil` because nothing ever captured self-managed instance/DNS data.
`UpdateHybridAD`'s `SelfManagedInstancesSettings` (`CustomerDnsIps`/`InstanceIds`) now writes
real `storedDirectory.HybridDNSIPs`/`HybridInstanceIDs`, and `toDirectory()` now populates
`HybridSettings` from them whenever `IsHybridAD` is set. Verified end-to-end in
`handler_hybrid_ad_test.go` (`TestHybridAD_CreateUpdateDescribeCycle`): create → assess → update
with `SelfManagedInstancesSettings` → `DescribeDirectories` shows the real
`SelfManagedDnsIpAddrs`/`SelfManagedInstanceIds`.

**A genuine wire-VALUE bug found in `AD-assessments` as a hybrid-AD dependency.**
`storedADAssessment.Status` was hardcoded to `"Completed"`, which is not a valid
`types.Assessment.Status` enum value at all (real values: `SUCCESS`, `FAILED`, `PENDING`,
`IN_PROGRESS`). Found because `CreateHybridAD` needed a real success status to gate on — fixed
to `"SUCCESS"` (`assessmentStatusSuccess` in `ad_assessments.go`). This is a different class of
bug than the already-documented `AssessmentConfiguration` gap (a wrong VALUE on an existing
field, not a missing field), and was in scope only because hybrid-AD's correctness depended on
it.

**Grade held at B, not raised.** The `AssessmentConfiguration` input-capture gap (see `gaps`)
was cited in the same downgrade note as hybrid-AD and remains fully open — `StartADAssessment`
still cannot accept it, so `Assessment`/`AssessmentSummary` are still missing ~8 real fields.
Closing hybrid-AD alone does not restore parity with the prior A grade while that gap stands.

### 2026-07-30 2nd follow-up pass: gopherstack-10hx (AD-assessment configuration gap)

Closed the remaining reason for B: `StartADAssessment` now accepts the real, optional
`StartADAssessmentInput.AssessmentConfiguration` member. Field-diffed directly against the
installed `aws-sdk-go-v2/service/directoryservice@v1.41.0`'s `types/types.go` (`AssessmentConfiguration`,
`Assessment`, `AssessmentSummary`), `serializers.go` (`awsAwsjson11_serializeDocumentAssessmentConfiguration`,
confirming the wire keys `CustomerDnsIps`/`DnsName`/`InstanceIds`/`SecurityGroupIds`/`VpcSettings{VpcId,SubnetIds}`),
`deserializers.go` (`awsAwsjson11_deserializeDocumentAssessment`, confirming `Assessment`'s full member set
and that `AssessmentSummary` is genuinely a strict subset — no `SecurityGroupIds`/`SelfManagedInstanceIds`/
`StatusCode`/`StatusReason`/`SubnetIds`/`Version`/`VpcId`), and `validators.go`
(`validateAssessmentConfiguration`, confirming `CustomerDnsIps`/`DnsName`/`InstanceIds`/`VpcSettings` are
required the moment `AssessmentConfiguration` is supplied at all, `SecurityGroupIds` is optional).

**What changed.** `ADAssessmentConfiguration` (`models.go`) is the new backend-facing input type;
`handler_ad_assessments.go`'s `parseAssessmentConfiguration` validates it with the same required-member
shape as the real SDK validator (`InvalidParameterException` per missing member, not a single generic
error). `startADAssessmentLocked`/`StartADAssessment`/the `StartADAssessment` interface method all gained
a `cfg *ADAssessmentConfiguration` parameter; `UpdateHybridAD`'s internally-triggered assessment (`hybrid_ad.go`)
passes `nil`, which is correct — `UpdateHybridADInput` has no `AssessmentConfiguration` member in the real API
either. `storedADAssessment` gained `DNSName`/`VPCID`/`CustomerDNSIPs`/`InstanceIDs`/`SecurityGroupIDs`/
`SubnetIDs`/`LastUpdateDateTime` (persisted via the existing `store.Table[storedADAssessment]` registration,
no persistence-layer changes needed). `DescribeADAssessment`'s `ADAssessment` response and `ListADAssessments`'
`ADAssessments` entries now emit the real, correctly-scoped field sets via the new shared `assessmentSummaryWire`
helper (the `AssessmentSummary`-shaped common fields) plus `handleDescribeADAssessment`'s own `Assessment`-only
additions — verified by `TestStartADAssessment_ConfigurationRoundTrip`, which asserts both presence on Describe
and absence on List for the four Assessment-only fields.

**What's still honestly absent.** `StatusCode`, `StatusReason`, `Version` remain unpopulated on `Assessment`.
Unlike the fields above, these have no real request-input source at all — AWS's own doc comments describe
them as assessment-engine-internal output (a detailed status code, a human-readable status/error message, a
framework version number) with no documented deterministic default. Inventing plausible-looking values here
(e.g. a fake version string) would be exactly the kind of fabrication this campaign has repeatedly downgraded
services for. Left empty and documented, the same treatment already given to `Directory.OsVersion` and
`DomainController.StatusReason` elsewhere in this same service — a treatment that has never itself blocked
an A grade in this file, since it reflects a genuine backend limitation rather than an unaudited or
half-fixed gap.

**Why this raises the grade.** Both downgrades that produced B (`b8552fe92`'s original "AD-assessments and
hybrid-AD have severe, previously-hidden gaps" finding, and the `10hx` follow-up's explicit "this is the
reason directoryservice's overall grade remains B" statement) are now closed with verified, non-fabricated
data. No new fabrication was introduced closing this gap — every field added round-trips real caller-supplied
data, and the fields that can't be real were left out rather than invented. See `TestStartADAssessment_ConfigurationValidation`
and `TestStartADAssessment_ConfigurationRoundTrip` in `handler_ad_assessments_test.go` for the wire-level proof.

## 2026-08-15 pass (`gopherstack-6flj`, wrapper-key / nested-shape sweep)

Full L+D+G sweep: all 25 List/Describe/Get ops (of 80 total, confirmed via `GetSupportedOperations`,
matching the `_WRAPPER_KEY_SWEEP_REMAINDER.md` ranked table exactly). Protocol confirmed `awsAwsjson11`
(JSON-RPC 1.1) exclusively, single client (`directoryservice@v1.41.4`, matches `go.mod`) — case-SENSITIVE
decode. All 453 `strings.EqualFold` hits in `deserializers.go` are `errorCode` matching only (0 in a
body-field-key switch) — confirmed via `grep -vc 'errorCode)'`. The restjson1 dead-deserializer trap does
not apply to this protocol family. `sdk_completeness_test.go` plus a direct diff of `GetSupportedOperations`
against every `api_op_*.go` in the pinned module confirm 0 phantom ops (all 80 real) and 0 missing ops (full
coverage). Router: `RouteMatcher` dispatches by `X-Amz-Target` header prefix into a flat `map[string]HandlerFunc`
(`h.dispatch`) keyed by exact op name — structurally immune to the elasticsearch-style "op unreachable at the
top-level router" class, since there is no path-segment matching to get wrong; not re-verified per-op beyond
confirming both `opDeleteADAssessment`/`opDescribeADAssessment` are present in the map.

6 real bugs found and fixed, all confirmed against the pinned SDK source with file+line and hand-reverted
individually to confirm the exact predicted failure before restoring byte-identical (see `wire_field_fixes_test.go`):

1. **`DeleteADAssessment`/`DescribeADAssessment` wrongly required `DirectoryId`** — real
   `DeleteADAssessmentInput`/`DescribeADAssessmentInput` are `{AssessmentId}` only (assessment IDs are
   globally addressable, not directory-scoped). Every real typed client's Delete/Describe call was rejected
   outright by this handler's own validation before ever reaching the backend — a total op failure, not
   silent-empty. `DeleteADAssessment` used the generic `handleTwoFieldOp` helper (which always requires
   `DirectoryId`), wrong for this one op; `DescribeADAssessment` had its own bespoke but equally wrong check.
2. **`DescribeADAssessment`/`ListADAssessments` wrapper keys were fabricated** — `"ADAssessment"`/
   `"ADAssessments"` instead of the real `DescribeADAssessmentOutput.Assessment`/`ListADAssessmentsOutput.Assessments`.
   Even a request that got past bug 1 would have decoded to nil/empty on every call.
3. **`RegisterCertificate` discarded `ClientCertAuthSettings.OCSPUrl` entirely** — a real, optional
   `RegisterCertificateInput` member (`types.ClientCertAuthSettings`) with no equivalent field anywhere in
   this backend (not just unemitted — genuinely untracked). Now captured, persisted (`storedCertificate.OCSPUrl`,
   `CertDetail.OCSPUrl`), and echoed on `DescribeCertificate`'s `Certificate.ClientCertAuthSettings`.
4. **`DescribeUpdateDirectory` wrapper key was fabricated, and its per-item shape was wire-breaking** —
   `"UpdateDirectoryInfo"` instead of the real `DescribeUpdateDirectoryOutput.UpdateActivities` (silent-empty).
   Separately, every entry emitted `NewValue`/`PreviousValue` as flat `""` strings; the real
   `types.UpdateInfoEntry` member type is `*types.UpdateValue{OSUpdateSettings}`, a nested struct — a real
   client's decode hard-FAILED with a JSON type-mismatch error on every call that returned at least one entry
   (i.e. every call after any `UpdateDirectorySetup`), not just silent-empty. This backend never populates real
   `NewValue`/`PreviousValue` content for any `UpdateType` (always the Go zero value), so both are now omitted
   entirely rather than fabricated into the nested shape.
5. **`DescribeSettings`' `SettingEntry` emitted the request-side filter field's name** — `"Status"` (matching
   `DescribeSettingsInput.Status`, a real but different field) instead of the real response member
   `SettingEntry.RequestStatus` (real `SettingEntry` has no `Status` member at all). A real client's
   `RequestStatus` field silently decoded to its zero value on every call. Sixth instance of this campaign's
   "real key from the wrong side" pattern.
6. **`AcceptSharedDirectory` returned only `{SharedDirectoryId}`** — the real `AcceptSharedDirectoryOutput.SharedDirectory`
   is a full `types.SharedDirectory` object, the exact same shape its sibling `DescribeSharedDirectories`
   already emitted correctly (every field, confirmed clean). Every other field (`OwnerDirectoryId`,
   `OwnerAccountId`, `SharedAccountId`, `ShareMethod`, `ShareStatus`, `ShareNotes`, `CreatedDateTime`,
   `LastUpdatedDateTime`) silently decoded to nil/zero on a real client. Fixed by sharing the same
   field-mapping helper (`toSharedDirInfo`) `DescribeSharedDirectories` already used — the correct sibling sat
   right beside the broken one, matching this campaign's recurring pattern.

**Disclosed, not fixed** (1): `DescribeLDAPSSettings`'s `LDAPSType`/`CertificateId`/`CertificateExpiryDateTime`
are NOT real `types.LDAPSSettingInfo` members at all (the real shape is exactly `{LDAPSStatus,
LDAPSStatusReason, LastUpdatedDateTime}`) — left in place rather than removed, since no sensitive data is
involved and a real client simply ignores unknown JSON fields; removing them buys nothing testable. Per this
campaign's own precedent (elasticsearch, rekognition passes), extra harmless fields are disclosed, not stripped.
`LDAPSStatusReason` (real, optional) is genuinely absent — this backend tracks no LDAPS state-change reason.

**Real-data-leak sweep** (this service holds AD credentials and trust passwords, called out explicitly for this
pass): no leak found. `Password`/`TrustPassword`/`NewPassword` request fields are read only for backend
invocation, never placed into any `map[string]any` response body (grepped every call site). `TrustPassword` is
accepted on `CreateTrust` and never echoed by `DescribeTrusts` (matches AWS's own real behavior — the real
`types.Trust` has no password member either). `SecretArn` (`CreateHybridAD`/`UpdateHybridAD`, a real Secrets
Manager ARN) is genuinely "used once and not stored" per its own doc comment and never appears in any Describe
response (matches the real API — `types.HybridUpdateInfoEntry` has no `SecretArn` member). `PcaConnectorArn`
(`DescribeCAEnrollmentPolicy`) is a real, intentional response member, not a leak. No environment-variable- or
KMS-ARN-shaped fields exist anywhere in this service's surface.

**Siblings checked and confirmed already correct** (full per-op wrapper-key diff against each op's own real
`Output` struct, not assumed from a passing family): `DescribeDirectories`/`GetDirectoryLimits`/`DescribeSnapshots`/
`GetSnapshotLimits`/`ListTagsForResource`/`DescribeCAEnrollmentPolicy`/`DescribeClientAuthenticationSettings`/
`DescribeConditionalForwarders`/`DescribeDirectoryDataAccess`/`DescribeDomainControllers`/`DescribeEventTopics`/
`DescribeHybridADUpdate`/`DescribeRegions`/`DescribeSharedDirectories`/`DescribeTrusts`/`ListCertificates`/
`ListIpRoutes`/`ListLogSubscriptions`/`ListSchemaExtensions` — all 19 hold their real wrapper key and real
per-item member set (each individually diffed against its own `types.go` struct, e.g. confirmed `EventTopic`'s
`{CreatedDateTime,DirectoryId,Status,TopicArn,TopicName}` is the full real member set with nothing missing or
extra). `InboundConnection`-style "sibling already correct beside a broken op" pattern repeated exactly:
`DescribeSharedDirectories` was the correct sibling sitting right beside the broken `AcceptSharedDirectory`.

No discarded inputs found beyond `RegisterCertificate`'s `ClientCertAuthSettings.OCSPUrl` (bug 3). No struct
retagged this pass doubles as a persistence DTO in a way that risked breaking snapshot/restore — `storedCertificate`
(the `OCSPUrl` addition) *is* the persistence DTO, and the addition is a pure field addition (not a retag/removal),
confirmed safe by `TestInMemoryBackend_SnapshotRestore_FullState` round-tripping the new field. No phantom ops
(0 among all 80; confirmed against every `api_op_*.go` in the pinned module, not just the L+D+G subset). Every
prior audit this file records (`h910`, `10hx` and its two follow-ups, the 2026-07-23/2026-08-13 passes) covered
the ops its own notes claim — none of the 6 bugs above fall in an op any prior pass's notes claimed to have
checked; all 6 are in the ops those passes' field-diffs treated as `ok`/`wire: ok` on the strength of a member-set
diff that never checked the wrapper key or which request members were actually required.

Real-client test ratio before this pass: 1 file (`handler_ca_enrollment_sdk_test.go`, 2 tests) out of 80 ops.
Added `wire_field_fixes_test.go`: 5 new real-SDK-client tests (`TestADAssessment_RealClientRoundTrip`,
`TestRegisterCertificate_OCSPUrl_RoundTrip`, `TestDescribeUpdateDirectory_RealClientRoundTrip`,
`TestDescribeSettings_RequestStatus_RealClientRoundTrip`, `TestAcceptSharedDirectory_RealClientRoundTrip`), each
driven through `newTestDirectoryServiceClient`'s full `service.NewServiceRouter`/`RouteHandler` stack (not just
`h.ServeHTTP` directly), and each hand-reverted individually against the fix it covers to confirm the exact
predicted failure before restoring byte-identical. One existing raw-body test strengthened after finding it
could not fail against the unfixed code: `TestSharedDirectories`'s Accept step previously asserted only HTTP 200,
never the response body — now asserts every `SharedDirectory` field.

Gates: full `go build ./...` (mandatory — `DeleteADAssessment`/`DescribeADAssessment`/`RegisterCertificate`/
`AcceptSharedDirectory` interface+backend signatures all changed; clean, no other package references this
service's backend directly), `go vet` (scoped + full `./...`), `go test -race` (scoped + `./pkgs/...`),
`go fix -diff` (no diff), `gofmt -l` (clean), `golangci-lint run ./services/directoryservice/...` (0 issues —
2 `goconst`/`nolintlint` findings from introducing a new `keyCreatedDateTime` constant and 1 `fieldalignment`
finding on `RegisterCertificate`'s new request struct, all fixed by hand, not `-fix`, per this campaign's
documented `fieldalignment -fix` nolint-stripping hazard). 0 `cyclop`/`gocyclo`/`gocognit`/`funlen` nolints
added (grep-confirmed). No subagents used (Read/Grep/Bash/Edit only, per this session's hard constraint). No
git-mutating commands run — orchestrator must commit/push. `git status` re-checked before every edit batch; a
sibling session was live on `services/opsworks/` throughout (confirmed via its own file changes appearing and
later disappearing in `git status`) — only `services/directoryservice/*` files were ever touched by this pass.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 1 fixed, 2 already-documented false positives

Part of the gopherstack-us9u/g479 map-literal scanner's 526-key unknown-key
bucket triage.

- `DescribeDirectoryDataAccess`: {wire: fixed} -- emitted
  "DirectoryDataAccessStatus"; real member
  (deserializers.go's
  awsAwsjson11_deserializeOpDocumentDescribeDirectoryDataAccessOutput) is
  "DataAccessStatus". Proven via
  `TestDescribeDirectoryDataAccess_DataAccessStatusKey_RealClient`
  (wire_field_fixes_test.go), hand-reverted/confirmed-failing/restored/
  `md5sum`-verified byte-identical.
- `DescribeLDAPSSettings` (LDAPSType/CertificateId/CertificateExpiryDateTime)
  and `DescribeUpdateDirectory` (UpdateType): rejected, not bugs. Both are
  already documented inline (handler_ldaps.go, handler_settings.go) as
  deliberate, previously-reviewed disclosed extras -- confirmed harmless
  since a real client ignores unknown keys, and in LDAPSType's case
  explicitly verified against the real types.LDAPSSettingInfo shape already.

gopherstack-wlo1 (2026-08-23), per-op error deserializer diff. Read every op's
own `deserializeOpError<Op>` switch (deserializers.go, awsjson11) and compared
against every error this backend's handler/mapError can actually return for
that op. Found a systemic pattern: this service's `mapError` collapses every
`awserr.ErrNotFound`-category sentinel to a single hardcoded wire code
(`EntityDoesNotExistException`), but a large family of newer ops --
CAEnrollmentPolicy, ClientAuthentication, DirectoryDataAccess, LDAPS,
Regions (Remove/Describe, not Add), Settings, UpdateDirectory, HybridAD,
StartADAssessment, Certificate list/register -- type the more specific
`DirectoryDoesNotExistException` instead, which their own switch does not
recognize `EntityDoesNotExistException` for at all; a real client's
`errors.As` into the intended `types.*Exception` failed on every one of them.
Fixed by introducing `ErrDirectoryNotFoundDDNE` (23 call sites across
certificates.go, ldaps.go, hybrid_ad.go, ad_assessments.go, settings.go,
client_auth.go, regions.go) alongside the existing `ErrDirectoryNotFound`,
with `mapError` special-casing it before the generic category fallback.

Three more independent bugs in the same family: `DeregisterCertificate`/
`DescribeCertificate` typed `EntityDoesNotExistException` for a missing
certificate where the SDK's own switch only recognizes
`CertificateDoesNotExistException` (new `ErrCertificateDoesNotExist`
sentinel, `ErrCertNotFound` removed as fully superseded); `AddRegion`'s
already-in-Region check returned `EntityAlreadyExistsException`, a code its
switch never types, where the real code is `DirectoryAlreadyInRegionException`
(new `ErrDirectoryAlreadyInRegion` sentinel); `RegisterEventTopic`'s
duplicate-registration check returned `EntityAlreadyExistsException`, which
its switch does not type _at all_ (no already-exists exception exists for
this op) -- fixed to succeed as an idempotent upsert instead (inferred, not
documented in the SDK doc comment, unlike codecommit's delete-idempotency
comment).

All four fix classes proven via a real `directoryservicesdk.Client` +
`require.ErrorAs` into the concrete `types.*Exception`
(handler_error_deserializer_diff_test.go); hand-reverted to confirm each
assertion fails against the pre-fix code, then restored byte-identical
(md5sum-verified). One pre-existing test
(`TestDescribeEventTopics_Filtering/duplicate_topic_registration_...`)
asserted the old wrong `EntityAlreadyExistsException` behavior and was
corrected in place to assert the real idempotent-upsert behavior, not
deleted. 8/8 dlm ops (separate service, same pass) were diffed and found
already clean -- see gopherstack-wlo1 for the full cross-service report.

## 2026-08-30 reqfieldscan fifth-dispatch-shape sweep

Ran `cmd/reqfieldscan` (after its own method-receiver-binding fix) against this package's
anonymous-inline-struct request decodes (opsworks-style handlers implementing
`service.JSONOpFunc` directly). 2 fields flagged, both hand-verified against the pinned
`directoryservice` SDK's real input shapes:

- `handleStartSchemaExtension`'s `CreateSnapshotBeforeSchemaExtension bool`: REAL bug, a
  required `StartSchemaExtensionInput` member silently dropped -- fixed, see
  StartSchemaExtension's own note above.
- `handleDescribeHybridADUpdate`'s `NextToken string`: confirmed real
  (`DescribeHybridADUpdateInput.NextToken`) but an already-documented honest gap -- see this
  op's existing ops-table note and the `overall` history above (DescribeHybridADUpdate never
  truncates its result set, so an inbound NextToken has nothing to resume from; its absence
  from the response is a truthful "no more pages"). No change needed.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` -- all clean
(`./services/directoryservice/...`).
