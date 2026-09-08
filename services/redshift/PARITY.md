---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: redshift
sdk_module: aws-sdk-go-v2/service/redshift@v1.65.4
sibling_sdk_modules: [aws-sdk-go-v2/service/redshiftserverless@v1.38.5]  # pinned in go.mod 2026-08-13, bd gopherstack-0w2p; see "Redshift Serverless" family row
last_audit_commit: 0fe7aaf4d
last_audit_date: 2026-08-08
overall: A            # RESTORED FROM A- (2026-07-25 follow-up pass, bd gopherstack-0eyk): the
                       # Create/ModifyRedshiftIdcApplicationResult missing-inner-<RedshiftIdcApplication>
                       # -wrapper bug that caused the prior A- downgrade is now fixed (see
                       # families.IdcApplication and gaps history below). Verified against
                       # awsAwsquery_deserializeOpDocumentCreate/ModifyRedshiftIdcApplicationOutput in
                       # aws-sdk-go-v2/service/redshift@v1.65.0/deserializers.go before fixing. Tests
                       # strengthened to assert the literal nested envelope
                       # (<CreateRedshiftIdcApplicationResult><RedshiftIdcApplication>, same for Modify,
                       # plus Describe's <member> wrapping) instead of loose substring Contains checks,
                       # so this class of bug can't silently regress again. Nothing else found holding
                       # the grade down this pass.
                       # (2026-08-13, gopherstack-3jqz, required-member sweep pass 3):
                       # RegisterNamespace/DeregisterNamespace fixed for real (see families.
                       # NamespaceRegistration) after this manifest's own "Descriptive/static ops" row
                       # falsely claimed them spot-checked. That same re-check turned up two more
                       # real, unfixed no-stub violations of the identical shape in the same family
                       # (ModifyAquaConfiguration, ModifyLakehouseConfiguration), flagged here rather
                       # than fixed in that pass.
                       # FIXED (2026-08-13, gopherstack-6xxt): both ModifyAquaConfiguration and
                       # ModifyLakehouseConfiguration now read and validate ClusterIdentifier for
                       # real (ClusterNotFoundFault on a miss) -- see families.AquaConfiguration/
                       # families.LakehouseConfiguration below. Grade holds at A.
                       # FIXED (2026-08-13, gopherstack-afi1, required-member sweep): CreateHsmConfiguration
                       # dropped both required HSM secrets (HsmPartitionPassword, HsmServerPublicCertificate)
                       # entirely -- neither reached the backend, whose signature had no parameters for
                       # them. See families.HsmClientCertificate/HsmConfiguration below. Grade holds at A.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  RestoreFromClusterSnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: Cluster.Tags nil-panic + stuck-in-restoring lifecycle bug"}
  ModifyCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: Encrypted/EnhancedVpcRouting tri-state (*bool). PendingModifiedValues never serialized -- confirmed inert, see Notes, not re-flagged as a gap"}
  GetClusterCredentials: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed prior pass: Expiration now serialized"}
  GetClusterCredentialsWithIAM: {wire: ok, errors: ok, state: ok, persist: n/a}
  ResizeCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS: now populates activeResizes (SUCCEEDED, AllowCancelResize=false) so DescribeResize/CancelResize observe a resize triggered via the real API op, not just AddActiveResizeInternal test seeding -- see gaps history"}
  RegisterNamespace: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-3jqz, required-member sweep pass 3): took `_ url.Values`, ignoring required ConsumerIdentifiers/NamespaceIdentifier (api_op_RegisterNamespace.go:33,41) entirely and returning static XML with no state change -- see families.NamespaceRegistration below."}
  DeregisterNamespace: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-3jqz), same bug and fix as RegisterNamespace -- see families.NamespaceRegistration below."}
  ModifyAquaConfiguration: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-6xxt): took `_ url.Values`, ignoring the required ClusterIdentifier (api_op_ModifyAquaConfiguration.go) and performing no existence check -- see families.AquaConfiguration below."}
  ModifyLakehouseConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-6xxt): took `_ url.Values`, ignoring ClusterIdentifier plus CatalogName/LakehouseIdcApplicationArn/LakehouseIdcRegistration/LakehouseRegistration, and returned a bare empty response -- see families.LakehouseConfiguration below."}
families:
  Cluster: {status: ok, note: "CreateCluster/DeleteCluster/DescribeClusters/RebootCluster/PauseCluster/ResumeCluster/RotateEncryptionKey/ModifyClusterIamRoles/ModifyClusterMaintenance verified. FIXED THIS PASS: xmlCluster never embedded Tags inline (real Cluster.Tags []Tag) -- every cluster response silently omitted tags a real client would expect on the object itself, not just via DescribeTags. Also added SnapshotScheduleIdentifier/SnapshotScheduleState (see SnapshotSchedule below)."}
  Tags: {status: ok, note: "CreateTags/DeleteTags/DescribeTags verified. See Cluster row for the inline-Tags wire gap fixed this pass. FIXED 2026-08-30 (wire-key-read sweep): DescribeTags read TagKey/TagValue as bare scalars, but real DescribeTagsInput.TagKeys/TagValues are []string wire-encoded as the indexed lists TagKeys.TagKey.N/TagValues.TagValue.N (confirmed against awsAwsquery_serializeDocumentTagKeyList/TagValueList) -- wrong key name AND wrong cardinality, so a real client's TagKeys/TagValues filter was always a silent no-op returning every tag. Also confirmed DeleteTags already used the correct TagKeys.TagKey.N form, which is what exposed the inconsistency. Fixed to parse the real indexed keys via the existing parseRedshiftTagKeysAt helper, with OR semantics across TagKeys/TagValues (matches DescribeClusters' clusterMatchesTagKeysOrValues convention and the real docs' \"any combination of the specified keys and values\" wording) via new shared tagMatchesFilter/anyTagMatchesFilter helpers (handler_tags.go). A pre-existing test (filter_by_key_and_value) asserted AND semantics, which was itself wrong; corrected to assert the real OR behavior."}
  ClusterParameterGroup: {status: ok, note: "no changes needed. CHECKED 2026-08-30 (wire-key-read sweep): DescribeClusterParameterGroupsInput.TagKeys/TagValues are declared and unread, but ClusterParameterGroup (param_groups.go) has no Tags field at all -- this backend never models tags on parameter groups (unlike UsageLimit/HsmClientCertificate/HsmConfiguration, fixed this pass). Left unread deliberately: implementing the filter would have nothing real to filter against, and DescribeTags itself already documents (see its own handler comment) that only cluster resources are tag-tracked here. Not re-flagged as a gap since it's the same documented single-resource-type-tagging limitation, just newly confirmed against this specific op."}
  ClusterSubnetGroup: {status: ok, note: "FIXED 2026-08-08 (bd gopherstack-emho): CreateClusterSubnetGroup previously accepted a fabricated 'VpcId' request param not present in the real CreateClusterSubnetGroupInput (confirmed against awsAwsquery_serializeOpDocumentCreateClusterSubnetGroupInput in aws-sdk-go-v2/service/redshift@v1.65.4/serializers.go -- real fields are only ClusterSubnetGroupName/Description/SubnetIds/Tags). Handler no longer reads it. The response's VpcId field IS real on ClusterSubnetGroup (types.ClusterSubnetGroup.VpcId), normally derived by AWS from the subnets' own VPC, but this backend has no EC2 cross-reference to derive it from (Provider.Init does not wire an EC2 backend into Redshift, and Subnet only tracks SubnetIdentifier/SubnetStatus, no VPC linkage) -- left honestly empty rather than fabricated, matching the EndpointAccess precedent below. AddSubnetGroupInternal (test-seeding only, not wire-reachable) can still set it directly. CHECKED 2026-08-30 (wire-key-read sweep): DescribeClusterSubnetGroupsInput.TagKeys/TagValues are also declared and unread, same missing-Tags-field situation as ClusterParameterGroup above -- left unread for the same reason."}
  ClusterSecurityGroup: {status: ok, note: "FIXED 2026-08-23 (third pass, closing the prior continued pass's follow-up): RevokeClusterSecurityGroupIngress now returns AuthorizationNotFound when nothing matched the given CIDRIP/EC2SecurityGroupName, closing the follow-up left open by the prior continued pass -- see dated entry above. SECOND FIND (same pass, sibling check per this campaign's own rule): AuthorizeClusterSecurityGroupIngress had the inverse gap -- re-authorizing a CIDR/EC2 group already on the security group silently appended a duplicate entry instead of returning AuthorizationAlreadyExists (declared in this op's own error switch, and already enforced by the sibling AuthorizeEndpointAccess family's own duplicate-rejection test). Fixed both; see dated entry above. CHECKED 2026-08-30 (wire-key-read sweep): DescribeClusterSecurityGroupsInput.TagKeys/TagValues are also declared and unread, same missing-Tags-field situation as ClusterParameterGroup/ClusterSubnetGroup above -- left unread for the same reason."}
  Snapshot/ClusterSnapshot: {status: ok, note: "FIXED 2026-08-23 (third pass): AuthorizeSnapshotAccess had the same missing-duplicate-check gap as AuthorizeClusterSecurityGroupIngress (re-authorizing an already-authorized account silently added a second AccountsWithRestoreAccess entry instead of returning AuthorizationAlreadyExists, declared in this op's own error switch) -- see dated entry above. A pre-existing test asserted the buggy behavior outright (\"AWS allows multiple accounts\"); corrected to assert the real error instead. FIXED 2026-08-23 (continued pass): ModifyClusterSnapshot/BatchModifyClusterSnapshots omitted-vs-explicit-(-1) retention clobber, RevokeSnapshotAccess wrong error code (InvalidParameterValue -> AuthorizationNotFound) -- see dated entry above. FIXED 2026-08-14 (gopherstack-7185, mutating-response sweep, broken in both directions): BatchDeleteClusterSnapshots' Identifiers is a list of DeleteClusterSnapshotMessage structs, not a flat string list -- the real serialized wire key is Identifiers.DeleteClusterSnapshotMessage.N.SnapshotIdentifier (confirmed against aws-sdk-go-v2/service/redshift@v1.65.4/serializers.go: awsAwsquery_serializeDocumentDeleteClusterSnapshotMessageList wraps the array in DeleteClusterSnapshotMessage, and the nested object serializer emits SnapshotIdentifier as a child field, not a value at the array index itself). The handler instead read 'Identifiers.DeleteClusterSnapshotMessage.N' directly and, failing that, fell back to 'Identifiers.SnapshotIdentifier.N' -- neither is a key any real SDK client ever sends, so a real BatchDeleteClusterSnapshots call always deleted nothing while still returning 200 OK with an empty Resources list. Three pre-existing tests all posted the second (also wrong) fallback shape, so tests and handler agreed on the fabricated request format -- same entrenching pattern as ssm's AddedLabels and ec2's ModifyVpcEndpointServicePermissions. Fixed to read the real nested key; BatchModifyClusterSnapshots' SnapshotIdentifierList (a genuine flat string list, serializeDocumentSnapshotIdentifierList wraps it in 'String') was re-verified and is correct as-is, so this is NOT a copy-paste bug across both batch ops, just the one whose real Input shape is structs. FIXED 2026-08-30 (wire-key-read sweep): DescribeClusterSnapshots read only SnapshotIdentifier/ClusterIdentifier/SnapshotType/Marker/MaxRecords -- StartTime/EndTime (real DescribeClusterSnapshotsInput fields, api_op_DescribeClusterSnapshots.go) were declared and never read at all, so a real client's time-window filter silently returned every snapshot regardless. Fixed via new filterSnapshotsByTimeRange, applied before the existing marker-pagination cut (filter-before-paginate). OwnerAccount/TagKeys/TagValues/SnapshotArn remain unread -- Snapshot (models.go) has no Tags or OwnerAccount field at all (this backend's snapshots are single-account and untagged), so those would be fabricated filter semantics; left honestly absent rather than invented, not re-flagged as a gap. FIXED 2026-09-07 (gopherstack-do4v, correction to gopherstack-igsa's grouping): ClusterExists does NOT need the cross-account ownership model OwnerAccount does -- confirmed against DescribeClusterSnapshotsInput.ClusterExists doc, api_op_DescribeClusterSnapshots.go: true selects snapshots of a cluster that still exists (and requires ClusterIdentifier), false selects snapshots whose cluster no longer exists (every orphaned snapshot when ClusterIdentifier is omitted, or that one deleted cluster's snapshots when given) -- a single-account existence check b.clusters.Get(id) already answers, nothing to do with who owns the snapshot. Implemented in DescribeClusterSnapshots/handleDescribeClusterSnapshots; true with no ClusterIdentifier returns InvalidParameterValue, matching the doc's stated requirement. OwnerAccount stays unimplemented."}
  ClusterCredentials: {status: ok}
  Resize: {status: ok, note: "FIXED THIS PASS, see ResizeCluster op row"}
  DataShare: {status: ok, note: "Associate/Authorize/Deauthorize/Reject/Disassociate/DescribeDataShares* field-diffed against types.DataShare. FIXED: DataShareType was completely absent from the model/wire (real Cluster... err DataShare.DataShareType, defaults to INTERNAL, the only enum value); now serialized. All mutation ops confirmed to mutate the store.Table-returned pointer in place (not stubs)."}
  EventSubscription/Events: {status: ok, note: "field-diffed against types.EventSubscription/Event. FIXED: EventSubscription.SubscriptionCreationTime was computed (SubscriptionCreated) but never serialized into any response; now emitted as RFC3339. DescribeEventCategories/DescribeEvents verified against SDK shapes, no other gaps found. CHECKED 2026-08-30 (wire-key-read sweep): DescribeEventsInput.StartTime/EndTime/Duration are declared and unread by handleDescribeEvents (only SourceIdentifier/SourceType are read) -- but this is inert, not a bug: nothing in this package ever writes to the b.events store (grepped every call site; no AddEvent/internal seed method exists, not even test-only), so DescribeEvents unconditionally returns an empty list regardless of any filter. Left as-is rather than adding dead filtering code for a store nothing populates."}
  Logging: {status: ok, note: "NEW FAMILY ROW 2026-08-23 (continued pass) -- EnableLogging/DisableLogging/DescribeLoggingStatus had no families: row at all before this pass, despite real per-cluster state (events.go's loggingStatuses map). EnableLogging/DisableLogging held up clean. FIXED: DescribeLoggingStatus was a static stub (hardcoded LoggingEnabled=false, ignored ClusterIdentifier, never consulted loggingStatuses) -- see dated entry above. gopherstack-ok46 (2026-09-06): EnableLogging accepted any S3KeyPrefix unchecked; api_op_EnableLogging.go documents an explicit valid-character set for it (letters/whitespace/numerics plus _ . : / = + \\ - @), now enforced and rejected with InvalidS3KeyPrefixFault (the fault EnableLogging itself declares, types/errors.go). BucketName has no documented format/length rule beyond \"must be an existing bucket in the same region with read/put permissions\" -- none of which is a request-shape rule -- so no new BucketName format check was added; its pre-existing presence check is untouched. InsufficientS3BucketPolicyFault (real policy evaluation against services/s3) is out of scope, left open."}
  ScheduledAction: {status: ok, note: "FIXED THIS PASS (major): TargetAction was parsed as a single flat top-level string param and never serialized in ANY response -- real CreateScheduledActionInput.TargetAction is a nested ScheduledActionType{PauseCluster|ResumeCluster|ResizeCluster} struct sent as TargetAction.ResizeCluster.ClusterIdentifier=... etc (query-protocol nested member convention), and the object is meaningless without it. Rebuilt as a real tagged-union type (ScheduledActionTarget) with correct nested request parsing (parseTargetAction) and response serialization (targetActionToXML), verified symmetric against both serializers.go and deserializers.go. Also fixed: Enable request param was completely ignored (State was hardcoded ACTIVE forever); now a real tri-state *bool driving ACTIVE/DISABLED. FIXED 2026-08-08 (bd gopherstack-emho): NextInvocations was previously unmodeled; this backend's Schedule field already carries a real at()/cron() expression, so a real evaluator (schedule.go) now computes it instead of leaving it fabricated or perpetually empty -- unparseable/unsupported expressions (e.g. rate(), which real Redshift does not accept here) still yield an honest empty list. StartTime/EndTime remain unmodeled -- see items_still_open. FIXED 2026-08-30 (wire-key-read sweep): DescribeScheduledActions read only ScheduledActionName -- Active (real DescribeScheduledActionsInput field) was declared and never read, so a real client's Active=true/false filter silently returned both enabled and disabled actions. Fixed by comparing against ScheduledAction.State (real backend data, already set correctly by scheduledActionState). New named constant scheduledActionStateActiveValue introduced deliberately instead of reusing the pre-existing dataShareStatusActive constant, which happens to share the same \"ACTIVE\" string by coincidence -- this campaign has already found bugs from exactly that kind of borrowed-constant coupling (see the ReservedNodeExchangeStatus fix in wire_field_fixes_test.go). TargetActionType/Filters/StartTime/EndTime remain unread: TargetActionType and the iam-role/cluster-identifier Filters names are real, cheap, and backed by existing data (IamRole, TargetAction's populated union member) but were left for a follow-up pass to keep this fix's blast radius small; StartTime/EndTime filter on computed next-invocation times, not stored data, and are a materially larger addition (see NextInvocations note above)."}
  UsageLimit: {status: ok, note: "Create/Delete/Describe/Modify field-diffed, real state mutation confirmed. FIXED 2026-08-08 (bd gopherstack-emho): Tags were accepted and stored on create but never echoed on the wire -- xmlUsageLimit now includes Tags>Tag via the existing tagMapToKVList/parseRedshiftTags shared helpers (same convention as Integration/Qev2IdcApplication), verified against awsAwsquery_deserializeDocumentUsageLimit's Tags case in deserializers.go. FIXED 2026-08-30 (wire-key-read sweep): DescribeUsageLimits read only ClusterIdentifier/FeatureType -- TagKeys/TagValues (real DescribeUsageLimitsInput fields) were declared and never read at all, even though UsageLimit.Tags is real, populated backend data (the fix immediately above this one). Fixed using the same anyTagMatchesFilter helper introduced for the Tags family fix."}
  SnapshotCopyGrant: {status: ok, note: "Create/Delete/Describe field-diffed, real state mutation confirmed. FIXED 2026-08-08 (bd gopherstack-emho): Tags now echoed on the wire (Tags>Tag), same fix pattern and SDK verification as UsageLimit above (awsAwsquery_deserializeDocumentSnapshotCopyGrant)."}
  SnapshotSchedule: {status: partial, note: "FIXED THIS PASS (real no-op found): ModifyClusterSnapshotSchedule validated ClusterIdentifier/ScheduleIdentifier existence but never recorded the association anywhere -- a textbook no-stub violation (looked like it worked, did nothing). Now sets/clears Cluster.SnapshotScheduleIdentifier/SnapshotScheduleState (real Cluster wire fields, confirmed against types.Cluster), and SnapshotSchedule.AssociatedClusters/AssociatedClusterCount are derived live by scanning clusters for a match and serialized correctly (AssociatedClusters>member>ClusterIdentifier/ScheduleAssociationState). Round-trip verified with a dedicated test. FIXED 2026-08-14 (gopherstack-7185, mutating-response sweep): Create/ModifySnapshotScheduleOutput both carry Tags (confirmed against deserializers.go:43027's generic TagList, wrapped in <Tag>), and this backend already tracks SnapshotSchedule.Tags (accepted on Create, stored, never dropped), but xmlSnapshotSchedule (shared by Create/Modify/Describe) had no field for it at all -- every schedule's tags were silently absent from every response. Added, reusing the existing tagMapToKVList helper. NOT fixed, left partial: Create/ModifySnapshotScheduleOutput also carry NextInvocations ([]time.Time). This service already computes NextInvocations for ScheduledAction via schedule.go's nextInvocations(), but that evaluator explicitly does not (and real ScheduledAction.Schedule does not) support rate(...) expressions or the 3-field cron(Minutes Hours Day-of-month) form CreateSnapshotScheduleInput.ScheduleDefinitions documents (e.g. \"cron(30 12 *)\", \"rate(12 hours)\") -- a different grammar from ScheduledAction's 6-field cron, not a drop-in reuse. Computing it correctly needs a second parser, disproportionate to this pass; ScheduleDefinitions/AssociatedClusters/Tags (the fields with real backing state and no format ambiguity) were fixed, NextInvocations was not -- see items_still_open."}
  SnapshotCopy: {status: ok, note: "Enable/Disable/ModifySnapshotCopyRetentionPeriod field-diffed, real state mutation confirmed, no changes needed"}
  AuthenticationProfile: {status: ok, note: "field-diffed against types.AuthenticationProfile (no Tags field on this type in the real SDK, confirmed), no changes needed"}
  ResourcePolicy: {status: ok, note: "FIXED THIS PASS: error code ErrResourcePolicyNotFound was a fabricated 'ResourcePolicyNotFound' string -- real GetResourcePolicy/PutResourcePolicy/DeleteResourcePolicy return ResourceNotFoundFault for a missing policy (confirmed against the op error-dispatch table in deserializers.go), now fixed."}
  HsmClientCertificate/HsmConfiguration: {status: ok, note: "Create/Delete/Describe field-diffed, real state mutation confirmed. FIXED 2026-08-08 (bd gopherstack-emho): Create handlers previously passed nil for tags unconditionally (never parsing Tags.Tag.N.* from the request) and the wire never echoed them; both now parse via parseRedshiftTags and serialize via tagMapToKVList, verified against awsAwsquery_deserializeDocumentHsmClientCertificate/HsmConfiguration's Tags case. Also found and fixed while verifying: CreateHsmConfiguration read the IP address request param as 'HsmIPAddress' but the real wire param is case-different 'HsmIpAddress' (confirmed against awsAwsquery_serializeOpDocumentCreateHsmConfigurationInput) -- url.Values lookups are case-sensitive, so a real SDK client's HsmIpAddress was silently dropped on every call; fixed. FIXED 2026-08-13 (gopherstack-afi1, required-member sweep): CreateHsmConfiguration also dropped both required HSM secrets -- HsmPartitionPassword and HsmServerPublicCertificate (api_op_CreateHsmConfiguration.go:64,70) -- entirely; the backend signature had no parameters for them at all. HsmConfiguration's real response shape (types/types.go:1118-1137) has no fields for either, so neither is echoed by real AWS either. Following this service's own existing precedent for CreateCluster's MasterUserPassword (handler.go:543-549,551: validated for shape/policy, never threaded into CreateCluster or persisted), both are now validated for presence in handleCreateHsmConfiguration and then discarded rather than passed to the backend or stored -- HsmPartitionPassword is a credential and is never logged, stored, or echoed in any response. Missing-required-member requests return InvalidParameterValue: this op's own deserializeOpErrorCreateHsmConfiguration switch declares only HsmConfigurationAlreadyExistsFault/HsmConfigurationQuotaExceededFault/InvalidTagFault/TagLimitExceededFault, no validation-style exception, so this follows the same ErrInvalidParameter convention already used for this handler's pre-existing HsmConfigurationIdentifier-required check. FIXED 2026-08-30 (wire-key-read sweep): DescribeHsmClientCertificates/DescribeHsmConfigurations each read only their own Identifier param -- TagKeys/TagValues (real, declared Input fields on both ops) were never read at all, even though both HsmClientCertificate.Tags/HsmConfiguration.Tags are real, populated backend data. Fixed using the same anyTagMatchesFilter helper introduced for the Tags family fix."}
  CustomDomainAssociation: {status: ok, note: "field-diffed, no changes needed to Create/Delete/Describe/Modify wire shapes. FIXED: ErrCustomDomainAlreadyExists was a fabricated 'CustomDomainAssociationAlreadyExistsFault' code -- no such fault exists in the real SDK; the real conflict fault for CreateCustomDomainAssociation is CustomCnameAssociationFault (confirmed against the op's error-dispatch table), now fixed. FIXED 2026-08-14 (gopherstack-7185, mutating-response sweep): the 'no changes needed to Create/Modify wire shapes' claim above was wrong -- both CreateCustomDomainAssociationOutput and ModifyCustomDomainAssociationOutput carry CustomDomainCertExpiryTime (confirmed against aws-sdk-go-v2/service/redshift@v1.65.4/api_op_Create/ModifyCustomDomainAssociation.go), which this backend's response structs never had a field for at all. Added CustomDomainCertExpiryTime to the CustomDomainAssociation model and both Create/Modify responses, generated the same fabricated-but-consistent-365-day way Redshift Serverless's own equivalent field already is (see families.Redshift Serverless' slCertExpiryDays). DescribeCustomDomainAssociations intentionally NOT touched -- its real Association shape is structurally different (grouped by certificate via CertificateAssociations, not a flat per-domain list), a pre-existing, larger, separately-scoped gap the code comment above it already documents; adding the field there would not fix that shape mismatch."}
  EndpointAccess: {status: ok, note: "FIXED THIS PASS (major param-shape bug): CreateEndpointAccess/ModifyEndpointAccess read/wrote a fabricated 'VpcId' parameter that does not exist anywhere in CreateEndpointAccessInput/ModifyEndpointAccessInput -- real requests carry SubnetGroupName/ResourceOwner/VpcSecurityGroupIds (Create) and VpcSecurityGroupIds only (Modify); VpcId on the response is *derived* from the subnet group, not settable directly. Rebuilt CreateEndpointAccess/ModifyEndpointAccess signatures and wire parsing/serialization around the real fields (SubnetGroupName, ResourceOwner, VpcSecurityGroupIds -> VpcSecurityGroups>VpcSecurityGroup list on the response), with VpcID derived via a ClusterSubnetGroup lookup when SubnetGroupName is known. VpcEndpoint (network interfaces) intentionally left unmodeled -- reconfirmed 2026-08-08: real types.VpcEndpoint.NetworkInterfaces needs AvailabilityZone/PrivateIpAddress/NetworkInterfaceId/SubnetId per ENI, none of which this backend's Subnet type carries (no CIDR/AZ data at all), and VpcEndpointId would have to be a fabricated ID with no real ENI allocation behind it -- left absent rather than invented, see items_still_open. FIXED 2026-08-30 (wire-key-read sweep): DescribeEndpointAccess read only ClusterIdentifier/EndpointName -- ResourceOwner and VpcId (real DescribeEndpointAccessInput fields) were declared and never read, even though EndpointAccess.ResourceOwner/VpcID are both real backend fields (ResourceOwner set directly from CreateEndpointAccessInput.ResourceOwner; VpcID derived from the subnet group per the note above, which is often empty since this backend's ClusterSubnetGroup.VpcID is itself never populated by the wire-reachable Create path -- a separate, pre-existing, NOT-fixed gap noted here for visibility). Both filters now applied post-fetch in the handler."}
  EndpointAuthorization: {status: ok, note: "AuthorizeEndpointAccess/RevokeEndpointAccess/DescribeEndpointAuthorization field-diffed against types.EndpointAuthorization, no changes needed. FIXED 2026-08-31 (value-semantics pass): DescribeEndpointAuthorization's Account filter compared the wrong side of the grantor/grantee pair. api_op_DescribeEndpointAuthorization.go documents Account precisely: 'the account ID of either the cluster owner (grantor) or grantee. If Grantee parameter is true, then the Account value is of the grantor' -- the handler had this backwards in both branches (Grantee=true compared against ea.Grantee instead of ea.Grantor; Grantee=false/default compared against ea.Grantor instead of ea.Grantee). Since this backend's every AuthorizeEndpointAccess-created record has Grantor pinned to b.accountID, the bug meant an Account filter almost never matched anything in the default (grantor) view unless the caller happened to pass their own account id, and the grantee view was equally backward. Regression test (TestHandler_DescribeEndpointAuthorization_GranteeAccountSide) proved both directions failed against the unfixed code before the swap."}
  Integration: {status: ok, note: "FIXED THIS PASS: (1) CreateIntegration read 'KmsKeyId' but the real wire param is case-different 'KMSKeyId' (confirmed against the query-protocol serializer) -- url.Values lookups are case-sensitive, so this silently dropped the KMS key for every real client call; (2) tags use 'TagList' not 'Tags' on this op specifically (unlike every other Create* op in this service) and were not parsed at all -- added parseTagListPrefixed and wired it in, response now includes Tags; (3) CreateTime was never serialized -- added; (4) ModifyIntegration was missing IntegrationName (real ModifyIntegrationInput supports renaming), added with existing-name-conflict handling. FIXED 2026-08-31 (value-semantics pass): integrationMatchesFilters switched on DescribeIntegrationsFilterName's four real values (integration-arn/source-arn/source-types/status, types/enums.go:194-202) but only handled two -- 'status' fell through the switch with no default and silently matched every integration regardless of the filter, even though Integration.Status is real, tracked data. 'source-types' remains deliberately unenforced (this backend has no SourceArn-to-AWS-resource-type classifier); 'status' is now handled. Regression test TestHandler_DescribeIntegrations_StatusFilter."}
  IdcApplication: {status: ok, note: "FIXED THIS PASS (bd gopherstack-0eyk): CreateRedshiftIdcApplicationResult/ModifyRedshiftIdcApplicationResult were serializing redshiftIdcAppXML's fields directly under the Result element; the real deserializer (awsAwsquery_deserializeOpDocumentCreateRedshiftIdcApplicationOutput/...Modify... in aws-sdk-go-v2/service/redshift@v1.65.0/deserializers.go, confirmed by reading it directly) requires them nested one level deeper under an inner <RedshiftIdcApplication> element -- a real SDK client parsing either response previously got every field as zero-value. Both response structs' xml tags fixed to `...Result>RedshiftIdcApplication`, matching the sibling Qev2IdcApplication family's pattern. DescribeRedshiftIdcApplications's <member> list wrapping and DeleteRedshiftIdcApplication (no response body) were re-checked against the same deserializers.go and confirmed already correct -- no changes needed there. Tests strengthened: Create/Modify success cases now assert the literal nested envelope string, not just substring presence of field values, so this class of bug is caught going forward; Describe's list_all case likewise now asserts the <member> wrapping explicitly. FIXED 2026-08-08 (bd gopherstack-emho): ApplicationType ('None'/'Lakehouse' enum) was unmodeled -- CreateIdcApplication now accepts and stores it (confirmed real request field via awsAwsquery_serializeOpDocumentCreateRedshiftIdcApplicationInput), echoed on Create/Describe/Modify responses; it is create-only, matching real ModifyRedshiftIdcApplicationInput which has no field for it (confirmed against awsAwsquery_serializeOpDocumentModifyRedshiftIdcApplicationInput). ServiceIntegrations deliberately left unmodeled -- it is a 3-level-deep tagged union (ServiceIntegrationsUnion -> {LakeFormation,Redshift,S3AccessGrants} -> per-family scope unions), disproportionate to this pass's scope; see items_still_open. AuthorizedTokenIssuerList/SsoTagKeys/IdcManagedApplicationArn/IdcOnboardStatus/IdentityNamespace remain unmodeled too."}
  Qev2IdcApplication: {status: ok, note: "NEW FAMILY THIS PASS (2026-07-25, SDK v1.62.3 -> v1.65.0 added CreateQev2IdcApplication/DeleteQev2IdcApplication/DescribeQev2IdcApplications/ModifyQev2IdcApplication). Confirmed via aws-sdk-go-v2/service/redshift@v1.65.0/types.Qev2IdcApplication and the Create/Delete/Describe/Modify Input/Output shapes that this is a DISTINCT resource from RedshiftIdcApplication, not a sub-resource -- no shared ID space, no cross-reference field either direction, and Qev2IdcApplication has no IamRoleArn (RedshiftIdcApplication's federated-auth role) at all. Implemented as its own store.Table/model/handler file pair. Wire-diffed field-by-field against serializers.go/deserializers.go: Create/Modify responses correctly nest the inner <Qev2IdcApplication> element (the bug found in the sibling family above, avoided here); Describe response uses real Marker/MaxRecords pagination (this op IS paginated in the real API, unlike DescribeRedshiftIdcApplications which this backend never paginates) implemented via the exact same sorted-snapshot/marker-cutoff convention as DescribeClusters; list items use <member> wrapping (confirmed against awsAwsquery_deserializeDocumentQev2IdcApplicationList); Tags round-trip via Tags.Tag.N.Key/Value on create and Tags>Tag on responses, matching this package's tagMapToKVList/parseRedshiftTags helpers exactly (real field name is 'Tags', not 'TagList' as CreateIntegration idiosyncratically uses). Cardinality: name-keyed uniqueness -> Qev2IdcApplicationAlreadyExists (real fault code, confirmed against types/errors.go; no separate quota fault exists for this family, unlike RedshiftIdcApplicationQuotaExceededFault). Modify only accepts IdcDisplayName (real ModifyQev2IdcApplicationInput has no other mutable field) -- IdcInstanceArn/Qev2IdcApplicationName verified immutable post-creation and covered by a regression test."}
  ReservedNode: {status: ok, note: "AcceptReservedNodeExchange/PurchaseReservedNodeOffering/Describe*/GetReservedNodeExchange* field-diffed, real state mutation confirmed. FIXED 2026-08-08 (bd gopherstack-emho): RecurringCharges is now derived from the node's own UsagePrice (this backend's real per-offering pricing model, see defaultReservedNodeOfferings) -- a No Upfront offering's nonzero UsagePrice produces one RecurringCharges>RecurringCharge{Hourly} entry, an All Upfront offering's zero UsagePrice produces none, verified against awsAwsquery_deserializeDocumentRecurringChargeList's RecurringCharges>RecurringCharge wrapper. ReservedNodeOfferingType remains unmodeled -- see items_still_open."}
  TableRestoreStatus/RestoreTableFromClusterSnapshot: {status: fixed, note: "FIXED THIS PASS: SnapshotIdentifier was parsed from the request and then explicitly discarded (bound to `_`), never stored -- now stored and serialized. RequestTime was computed but never serialized on ANY response (RestoreTableFromClusterSnapshotResult only echoed TableRestoreRequestId+Status) -- now serialized as RFC3339 on both RestoreTableFromClusterSnapshot and DescribeTableRestoreStatus. Also fixed the response's TargetTableName wire tag to the real 'NewTableName' (TableRestoreStatus has no TargetTableName field in the real SDK). SourceSchemaName/TargetSchemaName/ProgressInMegaBytes/TotalDataInMegaBytes/EnableCaseSensitiveIdentifier intentionally left unmodeled -- see items_still_open. gopherstack-muzq (2026-08-21): CreateTableRestoreStatus stamped Status IN_PROGRESS (the correct AWS-documented initial response, unchanged) but nothing in this backend ever advanced it -- no ticker, no later call -- while its sibling ServerlessTableRestoreStatus (serverless_table_restore.go) lands directly on SUCCEEDED at creation, since neither does real async data-copy work. Fixed by extending the existing cluster reconciler (reconciler.go's advanceClusterStates/StartReconciler machinery) with a parallel advanceTableRestoreStates, keyed by a new tableRestoreReadyAt map (100ms tableRestoreCompletionDelay), called both lazily on DescribeTableRestoreStatus and periodically by the same reconciler tick -- no new infrastructure class, matching the existing cluster-lifecycle pattern in this exact file. New test case reaches_succeeded in TestBackend_TableRestoreStatus asserts the terminal SUCCEEDED state; the pre-existing create_returns_in_progress case is still correct and kept as-is. FIXED 2026-08-31 (value-semantics pass): that same fix made the following bug observable for the first time (Status previously never left IN_PROGRESS, so no request could ever be excluded). DescribeTableRestoreStatusInput.TableRestoreRequestId's own doc: 'If you don't specify a TableRestoreRequestId value, then DescribeTableRestoreStatus returns the status of all in-progress table restore requests' -- a documented NARROWING default (only in-progress, not every request regardless of status), but the handler returned every stored request unconditionally when the id was omitted, only filtering by id when one was given. A request that had already reached SUCCEEDED stayed in the default (unfiltered) listing forever. Fixed: omitting TableRestoreRequestId now also excludes anything not still IN_PROGRESS; an explicit TableRestoreRequestId lookup is unaffected and still returns a succeeded request. Regression test TestHandler_DescribeTableRestoreStatus_DefaultOmitsSucceeded, proved failing pre-fix (used the real 100ms tableRestoreCompletionDelay via require.Eventually, not a sleep)."}
  Partner: {status: ok, note: "FIXED THIS PASS (severe, systemic): AddPartner/DeletePartner/DescribePartners/UpdatePartnerStatus all read/wrote a fabricated 'PartnerIntegrationId' parameter/wire-field name -- no such name exists anywhere in the real SDK (AddPartnerInput/Output, DeletePartnerInput/Output, UpdatePartnerStatusInput/Output, and PartnerIntegrationInfo all use 'PartnerName', confirmed against every relevant api_op_*.go and the DescribePartners deserializer). Every real client's PartnerName value was silently dropped on every request, and every response field a real client tried to read came back empty. Fixed across all 4 ops plus the internal error message text. Regression test locks in the exact wire element name. FIXED 2026-08-14 (gopherstack-7185, mutating-response sweep): AddPartner/DeletePartner/UpdatePartnerStatus responses ALSO carried an invented ClusterIdentifier field with no counterpart in AddPartnerOutput/DeletePartnerOutput/UpdatePartnerStatusOutput (confirmed against aws-sdk-go-v2/service/redshift@v1.65.4's api_op_*.go -- each carries only DatabaseName/PartnerName) -- removed from all three response structs. A pre-existing test (TestAddPartner_ResponseIncludesClusterIdentifier) only checked the cluster id string appeared somewhere in the body, so it entrenched the fabricated field rather than catching it; renamed and rewritten to assert the field's absence."}
  Descriptive/static ops: {status: ok, note: "RE-AUDITED gopherstack-3jqz (required-member sweep pass 3): the prior claim here -- 'RegisterNamespace/DeregisterNamespace spot-checked: real state mutation/derivation confirmed' -- was FALSE; both took `_ url.Values`, read neither ConsumerIdentifiers nor NamespaceIdentifier, and returned static XML with no state change at all. Moved out of this family (now families.NamespaceRegistration, fixed for real). Re-checking every other op this line vouched for: ListRecommendations and GetIdentityCenterAuthToken hold up -- both genuinely read and validate their input (ListRecommendations derives recommendations from DescribeClusters(id) and surfaces a real ClusterNotFoundFault for an unknown id; GetIdentityCenterAuthToken requires and checks IdentityCenterApplicationArn). DescribeAccountAttributes/DescribeClusterVersions/DescribeClusterTracks/DescribeOrderableClusterOptions/DescribeStorage/DescribeClusterDbRevisions are legitimately static/filter-less (already disclosed by 'NOT exhaustively field-diffed' below, not a new finding). CORRECTION 2026-08-31 (value-semantics pass): DescribeNodeConfigurationOptions is NOT filter-less -- it has a real NumberOfNodes filter (nodeConfigFilterValue/nodeConfigFilterInt) this line mischaracterized. That filter's NodeConfigurationOptionsFilter.Operator (eq/lt/le/gt/ge/between/in, types/types.go:1379-1388) was parsed nowhere -- every filter was compared with == against only the first supplied value, so a real client's gt/lt/le/ge/between/in filter silently behaved like an eq on the wrong value. The earlier wire-key-read sweep (see the citation at line ~1394 below) fixed the WIRE KEY (Filter.NodeConfigurationOptionsFilter.N.Value.item.M) but its own regression test only exercised Operator=Eq, so the operator-ignored bug was invisible to it -- a clean example of the wire-key and value-semantics axes needing separate verification. Fixed: NumberOfNodes now honours its Operator via numericFilterMatches; NodeType's Operator (documented as 'in'-only) is still not honoured -- only the first value seeds the synthesized target node type -- recorded as a gap rather than fixed, since choosing among several candidate node types when more than one is given isn't specified precisely enough to implement without guessing. Regression tests in handler_sdk_roundtrip_test.go's TestDescribeNodeConfigurationOptions_FilterWireKey (gt, between subtests) proved both failed against the unfixed code. Two more real bugs of the exact same shape as RegisterNamespace were found here by the same 'does the handler even read `vals`' check (ModifyAquaConfiguration, ModifyLakehouseConfiguration) and moved out to their own families below, same as NamespaceRegistration -- FIXED gopherstack-6xxt, see families.AquaConfiguration/families.LakehouseConfiguration. Restored to ok now that both are real."}
  NamespaceRegistration: {status: ok, note: "FIXED (gopherstack-3jqz, required-member sweep pass 3): RegisterNamespace/DeregisterNamespace previously ignored `_ url.Values` -- the entire request -- and returned static XML with no state change; see the ops: entries above. Both are the awsAwsquery_* (Query) protocol (redshift@v1.65.4 serializers.go), confirmed NOT the stale awsQuery_* prefix the repo's SDK-shape tooling defaults to detecting. NamespaceIdentifier is a union (NamespaceIdentifierUnion: ProvisionedIdentifier{ClusterIdentifier} or ServerlessIdentifier{NamespaceIdentifier,WorkgroupIdentifier}, confirmed against awsAwsquery_serializeDocumentNamespaceIdentifierUnion) arriving as dotted query keys (NamespaceIdentifier.ProvisionedIdentifier.ClusterIdentifier / NamespaceIdentifier.ServerlessIdentifier.{NamespaceIdentifier,WorkgroupIdentifier}), ConsumerIdentifiers as ConsumerIdentifiers.member.N via the existing parseStringList helper. Both variants now validate against REAL backend state before accepting: ProvisionedIdentifier checks b.clusters (ClusterNotFound if missing, InvalidClusterState if not 'available' -- both error codes taken from the op's own declared awsAwsquery_deserializeOpErrorRegisterNamespace/DeregisterNamespace switch, the same three-fault set for both ops: ClusterNotFound/InvalidClusterState/InvalidNamespaceFault), ServerlessIdentifier checks b.slNamespaces/b.slWorkgroups (InvalidNamespaceFault if either is missing) -- this package already models Redshift Serverless namespaces/workgroups internally (serverless.go), so this is real cross-reference validation, not a fabricated check. A new NamespaceRegistration record (namespace_registration.go, persisted via the standard store.Registry/store.Table mechanism) tracks ConsumerIdentifiers/Status per namespace identity; DeregisterNamespace removes exactly the given consumers from the existing set (real AWS scopes deregistration per-consumer, not per-namespace) rather than deleting the whole record. Status is always 'Registering'/'Deregistering' -- confirmed these are the ONLY two enum values NamespaceRegistrationStatus declares (types/enums.go); there is no describe/list operation anywhere in this SDK version for a client to observe a terminal state, so returning the in-flight status on every call is the real, complete contract, not a partial implementation. Proven via TestSDKRoundTrip_RegisterNamespace (real aws-sdk-go-v2 client, six subtests covering both union variants' accept/reject paths, hand-verified to fail against the unfixed handler) and TestNamespaceRegistration_ConsumerIdentifiersStateMutation (drives the backend directly, since there is no wire-level Describe to round-trip the consumer-list mutation through)."}
  AquaConfiguration: {status: ok, note: "FIXED (gopherstack-6xxt): handleModifyAquaConfiguration previously took `_ url.Values`, ignoring the required ClusterIdentifier (api_op_ModifyAquaConfiguration.go) entirely, performing no existence check, and always returning a canned AquaConfigurationStatus=auto/AquaStatus=disabled that didn't even match this backend's own DescribeClusters convention (toXMLClusterWithTags already emits disabled/disabled for every cluster's inline AquaConfiguration). The real op is documented retired (\"Calling this operation does not change AQUA configuration. Amazon Redshift automatically determines whether to use AQUA\") but still requires and existence-checks ClusterIdentifier -- ClusterNotFound is declared in its own error switch (awsAwsquery_deserializeOpErrorModifyAquaConfiguration: ClusterNotFound/InvalidClusterState/UnsupportedOperation). New backend method ModifyAquaConfiguration(id) (cluster_mgmt.go) does the real existence check; the response now shares a single defaultAquaConfig() helper (handler.go) with toXMLClusterWithTags so the two can never diverge again. InvalidClusterState/UnsupportedOperation left undeclared/unused -- no real precondition for either is documented for this retired op, matching this service's existing convention of not inventing trigger conditions for declared-but-unreachable exceptions (see glue's OperationTimeoutException reasoning for the same judgment call in a sibling service)."}
  LakehouseConfiguration: {status: ok, note: "FIXED (gopherstack-6xxt): handleModifyLakehouseConfiguration previously took `_ url.Values`, ignoring ClusterIdentifier plus CatalogName/LakehouseIdcApplicationArn/LakehouseIdcRegistration/LakehouseRegistration (api_op_ModifyLakehouseConfiguration.go) and returning a bare empty response. Classic Cluster (models.go) had no CatalogArn/LakehouseRegistrationStatus fields at all despite both being real, confirmed types.Cluster members (aws-sdk-go-v2/service/redshift@v1.65.4/types/types.go:153,343) -- this backend already modeled the equivalent state for Redshift Serverless (Namespace.CatalogArn/LakehouseRegistrationStatus, families.Redshift Serverless above), so the classic version was simply left behind; now added to Cluster and echoed on every Cluster-returning response (xmlCluster/toXMLClusterWithTags), not just this op's own. New backend method ModifyLakehouseConfiguration (lakehouse.go) follows UpdateLakehouseConfigurationSL's (serverless_lakehouse.go) existing carry-forward-when-omitted pattern: CatalogArn is derived via arn.Build(\"glue\",...,\"catalog/\"+CatalogName) same as the serverless sibling, and a new cluster-keyed store.Table (ClusterLakehouseConfig) holds LakehouseIdcApplicationArn, which has no Cluster member on the real wire either -- observable only through this op's own response, same convention as ServerlessLakehouseConfig. SECOND-LAYER FIND beyond the bd issue's stated scope: LakehouseIdcApplicationArn, when the caller is setting a new one, is now validated against this backend's own RedshiftIdcApplication store (idc_applications.go) via a lock-safe inline scan (idcApplicationExistsLocked) -- real cross-reference validation this backend can perform because it already models that resource, returning RedshiftIdcApplicationNotExists (declared in this op's own error switch, reusing the existing ErrIdcApplicationNotFound sentinel) on a miss; the Serverless sibling has no equivalent IDC-application backend to check against, so it does not do this. SECOND-LAYER FIND: DryRun does NOT map to a DryRunException here the way the Serverless sibling's UpdateLakehouseConfiguration does -- confirmed absent from awsAwsquery_deserializeOpErrorModifyLakehouseConfiguration's declared switch (ClusterNotFound/DependentServiceAccessDenied/DependentServiceUnavailableFault/InvalidClusterState/RedshiftIdcApplicationNotExists/UnauthorizedOperation/UnsupportedOperation, no DryRun-shaped fault) -- ModifyLakehouseConfigurationInput.DryRun's own doc text ('validates the request without actually modifying the lakehouse configuration') is honored literally instead: a successful DryRun runs every validation and returns the would-be result as a normal 200, without persisting it. DependentServiceAccessDenied/DependentServiceUnavailableFault/UnauthorizedOperation/InvalidClusterState remain undeclared/unused -- no real precondition for any is discoverable from this backend's state, left honest rather than inventing triggers."}
  Redshift Serverless: {status: ok, note: "AUDITED AND PARTLY FIXED 2026-08-08 (bd gopherstack-hsfm). aws-sdk-go-v2/service/redshiftserverless was still not a go.mod dependency; fetched via `go get ...@v1.38.5` to populate GOMODCACHE for field-diffing serializers.go/deserializers.go/types directly (not from memory/docs), then `go mod tidy` dropped it again afterward since the fix (like the rest of this repo) hand-rolls JSON wire structs rather than importing SDK types at runtime -- no persistent new dependency. SEVERE FINDING: this whole 25-op surface used REST-style path/verb routing (/redshift-serverless/namespaces, GET/POST/PATCH/DELETE) that NO real client ever sends -- confirmed every awsAwsjson11_serializeOp* in serializers.go POSTs to \"/\" with an X-Amz-Target header and puts all fields (including resource identifiers) in the JSON body. RouteMatcher required the REST path prefix, so a real SDK client's request never matched at all: all 25 ops were unroutable, the same unreachable-service bug class found in opsworks (gopherstack-vjj2) but total instead of partial. FIXED: RouteMatcher/ExtractOperation rewritten to X-Amz-Target dispatch (PriorityHeaderExact, matching redshiftdata.Handler's existing pattern in this package); every handler decodes resource identifiers from the body instead of the URL. Also fixed while rewriting (all confirmed against deserializers.go before fixing): ServerlessScheduledAction's status field used wire key \"status\" but the real ScheduledActionResponse field is \"state\" (types.State, ACTIVE/DISABLED) -- ScheduledActionResponse has no \"status\" field at all; StartTime/EndTime and GetCredentialsOutput's Expiration/NextRefreshTime were RFC3339 strings but the real wire format is epoch-seconds JSON numbers (awstime.Epoch, same bug class as the QuickSight/IoT precedent in parity-principles.md); Schedule/TargetAction were flat strings but the real shapes are tagged-union JSON objects ({\"cron\":...}/{\"at\":...} and {\"createSnapshot\":{...}}) -- now passed through as json.RawMessage (accurate shape, no fabricated execution semantics); CreateScheduledActionInput.RoleArn (a REQUIRED real field) was completely absent from the request struct, so every real client's roleArn was silently dropped and unrecoverable -- now required and stored; Enabled/ScheduledActionDescription were also dropped, now threaded through; ScheduledActionUUID and the fabricated scheduledActionArn field (not a real ScheduledActionResponse member) were fixed to match the real shape. Also fixed accepted-then-dropped (a) fields: Namespace.DefaultIamRoleArn, ManageAdminPassword/AdminPasswordSecretKmsKeyId (with a fabricated-but-consistent secretsmanager ARN, same convention as this backend's other resource ARNs); DeleteNamespace's FinalSnapshotName/FinalSnapshotRetentionPeriod now actually create a final snapshot; CreateSnapshot's retentionPeriod; Workgroup's ConfigParameters/MaxCapacity/Port/IpAddressType/TrackName/PricePerformanceTarget/EnhancedVpcRouting/ExtraComputeForAutomaticOptimization/PubliclyAccessible; GetCredentials' DurationSeconds and the previously entirely-absent NextRefreshTime response field; List*'s MaxResults, which was hardcoded to 0 and silently ignored on every List call regardless of protocol. Error envelope switched from ad hoc 404/409 status codes to the real awsJson1.1 convention (HTTP 400 for every client-fault exception, confirmed by the absence of any per-exception status override in types/errors.go). Deliberately left unfixed, each independently verified absent from all reachable output: Tags on Create* (defers to the excluded Tagging family below), AdminUserPassword (real API never echoes it either), Namespace.RedshiftIdcApplicationArn (accepted by the real API but not a field on types.Namespace -- no observable output surface exists for it among these 25 ops), ScheduledActionResponse.NextInvocations (this service's cron format is unwrapped, unlike classic Redshift's cron(...)/at(...) strings that schedule.go already evaluates -- adapting that evaluator is a reasonable follow-up, not done this pass), Snapshot's backup-progress/size/cross-account-restore-access fields (this backend creates snapshots instantaneously, so progress fields have no real driving state; restore-access fields are populated via the excluded ResourcePolicy family), GetCredentials' CustomDomainName lookup (depends on the excluded CustomDomainAssociation family). Full field-by-field audit table with file:line citations recorded in bd gopherstack-hsfm's close reason. Whole missing resource families (EndpointAccess, CustomDomainAssociation, ResourcePolicy, RecoveryPoint, SnapshotCopyConfiguration, TableRestoreStatus, Tagging, ListManagedWorkgroups, restore-from-snapshot) still have zero code -- see items_still_open. TAGGING AND CUSTOMDOMAINASSOCIATION BUILT 2026-08-09 (bd gopherstack-w8g2): TagResource/UntagResource/ListTagsForResource and Create/Get/List/Update/DeleteCustomDomainAssociation implemented against the pinned botocore redshift-serverless/2021-04-21/service-2.json model (json protocol, confirmed via metadata.protocol), not the aws-sdk-go-v2 module (kept out of go.mod per this issue's constraint -- verified via TagList's Tag{key,value} shape, not a JSON map). Confirmed only Namespace/Workgroup/Snapshot accept a create-time \"tags\" list (CreateUsageLimitRequest/CreateScheduledActionRequest have none) and that none of Namespace/Workgroup/Snapshot echo a \"tags\" field on their own GET/response shape -- tags are stored in a new resourceArn-keyed store.Table (slResourceTags) reachable only via ListTagsForResource, proven with a handler-level round trip (TestServerless_TagResource_RoundTrip) plus a persistence Snapshot/Restore round trip. CustomDomainAssociation modeled per Association{customDomainCertificateArn,customDomainCertificateExpiryTime,customDomainName,workgroupName} (Create/Get/Update responses are flat, NOT wrapped in an envelope key, unlike every other serverless resource -- confirmed against the Response shapes directly; Delete has zero response members); customDomainCertificateExpiryTime uses SyntheticTimestamp_date_time (ISO8601 string), NOT the epoch-seconds Timestamp shape GetCredentials' Expiration/NextRefreshTime use -- confirmed as a genuine per-field wire-format difference, not an inconsistency to \"fix\". Real Workgroup also carries customDomainName/customDomainCertificateArn/customDomainCertificateExpiryTime directly (added to the Workgroup struct, mirrored on associate/update/delete). GetCredentials now resolves workgroupName via customDomainName per GetCredentialsRequest's documented either-or requirement. EndpointAccess/ResourcePolicy/RecoveryPoint/SnapshotCopyConfiguration/TableRestoreStatus/ListManagedWorkgroups/restore ops deliberately NOT attempted this pass -- see items_still_open. RESOURCEPOLICY AND SNAPSHOTCOPYCONFIGURATION BUILT 2026-08-10 (bd gopherstack-w8g2): Get/Put/DeleteResourcePolicy implemented as a new resourceArn-keyed store.Table[ServerlessResourcePolicy] (slResourcePolicies), distinct from classic Redshift's own resourcePolicies table/methods (same op names, different protocol and sentinel error, disambiguated with an SL suffix on the backend methods). Envelope convention (`{\"resourcePolicy\": {...}}`) and DeleteResourcePolicyResponse's zero members both confirmed against service-2.json -- the flat-response oddity found in CustomDomainAssociation does NOT generalize here. Create/Update/Delete/ListSnapshotCopyConfiguration implemented as a new store.Table[ServerlessSnapshotCopyConfiguration] (slSnapshotCopyConfig) plus a sortedStringIndex for List's deterministic pagination; CreateSnapshotCopyConfiguration validates namespaceName against the existing namespace store (ResourceNotFoundException on a miss). This backend does not simulate real cross-region replication, consistent with how Namespace/Workgroup/Snapshot are already handled -- only the configuration object itself is tracked. One business rule was deliberately NOT invented: service-2.json documents no one-configuration-per-namespace constraint, so none is enforced (unlike classic Redshift's EnableSnapshotCopy, which this backend does gate one-per-cluster, but that is a different family entirely). EndpointAccess/RecoveryPoint/TableRestoreStatus/ListManagedWorkgroups/restore ops remain unbuilt -- see items_still_open. RECOVERYPOINT AND TABLERESTORESTATUS BUILT 2026-08-10 (bd gopherstack-w8g2, entangled group): Get/ListRecoveryPoints, RestoreFromRecoveryPoint, RestoreTableFromSnapshot, RestoreTableFromRecoveryPoint, Get/ListTableRestoreStatus implemented. RecoveryPoint has NO create operation anywhere in service-2.json (\"Recovery points are created every 30 minutes and kept for 24 hours\", confirmed on the RecoveryPoint shape's own documentation) -- this backend generates exactly one recovery point per workgroup at CreateWorkgroup time instead of running a real 30-minute scheduler (generateRecoveryPointLocked, serverless_recovery.go), matching this service's existing instant-apply convention (e.g. snapshots created instantaneously); an AddRecoveryPointInternal test-seed method exists for tests that need more than one, not wired to any wire-reachable op, same convention as AddSnapshotInternal etc. RestoreFromSnapshot (namespace-level restore from a Snapshot, no recovery point involved) was deliberately NOT built this pass -- it does not depend on RecoveryPoint and was excluded from this entangled group by design; still open, see items_still_open. Timestamp formats verified to genuinely differ within this one family: RecoveryPoint.recoveryPointCreateTime is SyntheticTimestamp_date_time (ISO8601 string, confirmed against both service-2.json and awsAwsjson11_deserializeDocumentRecoveryPoint's smithytime.ParseDateTime call), while TableRestoreStatus.requestTime is the bare Timestamp shape (epoch-seconds JSON number, confirmed against awsAwsjson11_deserializeDocumentTableRestoreStatus's smithytime.ParseEpochSeconds call) -- two timestamp fields in the same entangled group, two different wire formats, both re-verified rather than assumed from the nearer-looking sibling. RestoreFromRecoveryPointSL additionally validates that the given workgroupName belongs to the given namespaceName (the same Namespace-Workgroup FK relationship CreateWorkgroup already enforces) -- not a fabricated recovery-point-specific rule, just this backend's existing invariant applied here too. ServerlessTableRestoreStatus.Status is set to SUCCEEDED immediately (this backend applies every restore synchronously, consistent with the rest of this service) rather than left IN_PROGRESS forever the way classic Redshift's own TableRestoreStatus is (a pre-existing, out-of-scope quirk in table_restore.go, not touched); ProgressInMegaBytes/TotalDataInMegaBytes are honestly left at zero/omitted rather than fabricated, since this backend has no real data to move. EndpointAccess and ListManagedWorkgroups remain unbuilt -- see items_still_open. ENDPOINTACCESS, LISTMANAGEDWORKGROUPS, RESTOREFROMSNAPSHOT AND CONVERTRECOVERYPOINTTOSNAPSHOT BUILT 2026-08-10 (bd gopherstack-w8g2, final pass -- closes the issue): Create/Get/List/Update/DeleteEndpointAccess implemented as a new endpointName-keyed store.Table[ServerlessEndpointAccess] (slEndpointAccesses), distinct from classic Redshift's own cluster-keyed EndpointAccess (endpoint_access.go) -- real CreateEndpointAccessRequest requires workgroupName/subnetIds (individual subnet IDs), not clusterIdentifier/subnetGroupName, confirmed against CreateEndpointAccessRequest/UpdateEndpointAccessRequest/EndpointAccess in service-2.json and cross-checked against types.EndpointAccess in aws-sdk-go-v2/service/redshiftserverless@v1.38.5/types/types.go. Per this issue's explicit instruction to check how classic Redshift's own EndpointAccess handled the same judgment call: confirmed families.EndpointAccess above left the entire nested VpcEndpoint object (network interfaces) absent rather than invented, and the identical problem exists here in a slightly different shape -- real types.VpcEndpoint carries vpcEndpointId/vpcId/networkInterfaces (each NetworkInterface needing availabilityZone/privateIpAddress/networkInterfaceId/subnetId, confirmed against types.NetworkInterface), none of which this backend tracks anywhere (no EC2 cross-reference wired into Redshift at all, same finding as families.ClusterSubnetGroup). Followed the same precedent exactly: vpcEndpoint is left absent from every response rather than partially fabricated (e.g. a real-looking vpcEndpointId with no ENI behind it). VpcSecurityGroups IS modeled (unlike VpcEndpoint) since it only echoes client-supplied IDs, the same shape as classic's own VpcSecurityGroupMembership, reusing its \"active\" status convention (endpointStatusActive) since both are the identical real shape. ListEndpointAccessRequest's vpcId filter is deliberately not accepted for the same reason -- nothing honest to filter against. DeleteEndpointAccessResponse echoes the deleted object (confirmed against service-2.json: it carries a real \"endpoint\" member, unlike DeleteResourcePolicy/DeleteCustomDomainAssociation's zero-member responses). LISTMANAGEDWORKGROUPS: per this issue's instruction to check whether the \"thin, no real backing state\" judgment holds -- it does. ListManagedWorkgroupsRequest.sourceArn is documented and pattern-constrained as a Glue Data Catalog database/catalog ARN (`^arn:aws[a-z-]*:glue:...`, confirmed in the SourceArn shape), meaning ManagedWorkgroupListItem represents a workgroup Glue/Lake Formation auto-provisions when federated queries run against shared data -- confirmed by grep that this package has zero Glue Data Catalog or Lake Formation integration anywhere (AssociateDataShareConsumer is classic Redshift's unrelated data-sharing feature, not this). Implemented as an honest, correctly-shaped, always-empty response (ListManagedWorkgroupsSL) rather than inventing entries -- no store.Table needed since there is no create path, real or otherwise, that could ever populate one. RESTOREFROMSNAPSHOT: RestoreFromSnapshotRequest requires namespaceName/workgroupName (confirmed against service-2.json) with the identical \"name of the namespace to restore ... to/into\" wording convention and required-field shape RestoreFromRecoveryPointRequest already uses -- by that symmetry, both are treated as pre-existing resources here too (same design RestoreFromRecoveryPointSL established in the prior pass), validated via the same Namespace-Workgroup FK check. Resolves snapshotName or snapshotArn (either, mutually exclusive per the real request) via the same ARN-suffix-stripping convention GetServerlessSnapshot already uses. manageAdminPassword/adminPasswordSecretKmsKeyId are threaded through onto the namespace (a real, easy-to-honor field, not left as an inert accepted-then-dropped parameter) but only in the true direction -- false does not clear existing Secrets-Manager fields, since real AWS's documented false-branch behavior (\"uses the admin credentials the namespace or cluster had at the time the snapshot was taken\") is data this backend cannot reconstruct, so it is left untouched rather than fabricated. Real AWS restores a namespace's storage layer in place; this backend does not simulate real data content, so once the lookup/FK checks pass, the existing Namespace is returned unchanged, same as RestoreFromRecoveryPointSL. CONVERTRECOVERYPOINTTOSNAPSHOT: recoveryPointId/snapshotName both required (confirmed against service-2.json); implemented by writing a new ServerlessSnapshot from the recovery point's namespace linkage (NamespaceName/NamespaceArn) plus the target namespace's AdminUsername when resolvable, reusing the exact same snapshotName-conflict check and arn.Build/store/index-insert/putServerlessTagsLocked sequence CreateServerlessSnapshot already uses. All four verified to genuinely fail beforehand: temporarily removed their slDispatchTable entries (file copy, not git stash) and reran the new tests -- every one flipped from real behavior to \"unknown operation\" ValidationException/400, confirmed, then the entries were restored. go.mod/go.sum confirmed unmodified (git status clean before and after fetching aws-sdk-go-v2/service/redshiftserverless@v1.38.5 and aws-sdk-go-v2/service/redshift@v1.65.4 into GOMODCACHE via `go get` then reverting) and `go mod tidy` produced no diff. This closes bd gopherstack-w8g2: all nine originally-missing serverless families now have real code. GO.MOD PIN + NINE FIELD GAPS + PHANTOM FIELD FIXED 2026-08-13 (bd gopherstack-0w2p/8v8v/mbcq): aws-sdk-go-v2/service/redshiftserverless was STILL not a go.mod dependency despite the note above (the 2026-08-08 `go get`/`go mod tidy` round-trip left no persistent pin, exactly as documented) -- every audit of this surface, including the one that produced this entry's own predecessors, was reading whatever version happened to be in a dev machine's module cache. Fixed properly this time: added `github.com/aws/aws-sdk-go-v2/service/redshiftserverless v1.38.5` as an explicit go.mod requirement (v1.38.5 chosen deliberately -- confirmed via `go list -m -json` that it shares the exact same release timestamp, 2026-08-05T18:20:26Z, as the already-pinned redshift@v1.65.4 and redshiftdata@v1.43.4, i.e. the same upstream release batch, rather than the newer v1.38.6 sitting alone in the module graph), and added TestSDKCompleteness_Serverless (sdk_completeness_test.go) so `go mod tidy` has a real import to keep -- this package hand-rolls JSON wire structs and imports no SDK types at runtime, so without that test the requirement would be silently stripped again on the next tidy. That completeness test immediately surfaced 10 SDK operations with zero code that no prior audit had caught (CreateReservation, GetIdentityCenterAuthToken, GetReservation, GetReservationOffering, GetTrack, ListReservationOfferings, ListReservations, ListTracks, UpdateLakehouseConfiguration, UpdateSnapshot -- separate feature surfaces: capacity reservations, tracks, lakehouse config, IDC token vending, plus a plain UpdateSnapshot gap); filed as gopherstack-irh7, deliberately NOT built this pass (out of scope), listed in the test's notImplemented slice with a comment. Re-verified gopherstack-8v8v and gopherstack-mbcq's findings against the now-pinned v1.38.5 source directly (api_op_*.go/types/types.go in GOMODCACHE) rather than trusting the prior audit's citations: all held exactly as reported, no findings changed -- the module cache copy the prior audit read from was already v1.38.5, same as what's now pinned. FIXED gopherstack-8v8v: UpdateNamespace accepted a `dbName` request field and mutated Namespace.DBName from it (serverless_namespaces.go); UpdateNamespaceInput has no dbName member at all (confirmed against api_op_UpdateNamespace.go -- a namespace's database name cannot be changed after creation), while CreateNamespaceInput does have one (real, kept). Field and mutation removed; UpdateNamespaceParams no longer carries DBName. FIXED gopherstack-mbcq's nine gaps, each re-verified against api_op_*.go before fixing: (1) AdminUserPassword added to CreateNamespace/UpdateNamespace -- the only way to set an explicit admin password outside the ManageAdminPassword/Secrets-Manager path; as a credential it is read from the wire, threaded through *Params structs, but explicitly discarded (`_ = p.AdminUserPassword`, documented) before ever reaching the Namespace struct -- same accept-but-never-store convention this package's own CreateCluster already uses for classic Redshift's MasterUserPassword (handler.go/cluster_mgmt.go), and consistent with real AWS itself: types.Namespace has no adminUserPassword member either, so no client can ever observe whether this backend stores it. Proven never echoed by TestServerless_Namespace_AdminUserPassword_NeverEchoed (asserts the literal secret string is absent from the raw response body, not just the decoded struct). (2) RedshiftIdcApplicationArn added to CreateNamespace, same accept-then-discard treatment -- real types.Namespace has no such member either (confirmed against types/types.go), so this is write-only on the real API too, not merely on this backend. (3) MaintainIntegration added to RestoreFromSnapshot (RestoreFromSnapshotParams) -- accepted but inert, documented: this backend does not model data-sharing/zero-ETL/S3-event integration state on namespaces at all, so there is nothing to maintain or drop. (4) ActivateCaseSensitiveIdentifier added to the shared slTableRestoreReq/RestoreTableFromSnapshotParams used by both RestoreTableFromSnapshot and RestoreTableFromRecoveryPoint -- accepted but inert, documented: this backend never executes queries against a restored table, so there is no case-sensitive identifier matching to gate. Five real filter gaps fixed (all previously accepted-and-silently-ignored, each proven to narrow a multi-item result set by a new test, not just parse): ListSnapshots gained EndTime/StartTime (bound SnapshotCreateTime, epoch-seconds on the wire per serializers.go, reusing the existing slEpochFromPtr helper), NamespaceArn (compares against the already-stored ServerlessSnapshot.NamespaceArn), and OwnerAccount; ListRecoveryPoints gained EndTime/StartTime bounding RecoveryPointCreateTime; ListWorkgroups gained OwnerAccount; GetSnapshot gained OwnerAccount; ListUsageLimits gained UsageType (compares against the already-stored ServerlessUsageLimit.UsageType). OwnerAccount on all three (ListSnapshots/ListWorkgroups/GetSnapshot) is honestly single-account: this backend never simulates cross-account snapshot/workgroup sharing for the serverless surface (AuthorizeSnapshotAccess is not part of this API; ServerlessSnapshot.AccountsWithRestoreAccess is declared for wire shape but never populated), so every resource's real owner is b.accountID -- a non-empty OwnerAccount that doesn't match b.accountID is implemented as matching nothing, same as real AWS would return for an inaccessible cross-account resource, not left as a silently-ignored no-op. Re-confirmed DO-NOT-TOUCH: ListEndpointAccess's VpcId omission (serverless_endpoint_access.go) is still correct and was left untouched -- this backend never derives a real vpcId for any endpoint, so there remains nothing honest to filter against. FOUR OF THE TEN GAPS FROM gopherstack-irh7 FIXED, ONE FAMILY DELIBERATELY DEFERRED 2026-08-13 (bd gopherstack-v4wu): UpdateSnapshot (retentionPeriod is optional and nilable, confirmed against api_op_UpdateSnapshot.go -- omitting it leaves the stored value unchanged, proven by TestServerless_UpdateSnapshot_OmittedRetentionPeriodUnchanged) now completes the Snapshot CRUD family. GetTrack/ListTracks return a static two-entry catalog (current/trailing, both at this backend's single modelVersion10 release) -- the same precedent classic Redshift's own DescribeClusterTracks already set for the identical real-world enumeration (see families.Descriptive/static ops); UpdateTargets is honestly left empty since there is no second release to invent an upgrade path to. UpdateLakehouseConfiguration writes real Namespace.CatalogArn/LakehouseRegistrationStatus (both confirmed present on types.Namespace but previously entirely absent from this backend's Namespace struct -- a genuine pre-existing wire gap, not new fabrication) plus a new namespaceName-keyed store.Table (slLakehouseConfig, serverless_lakehouse.go) for LakehouseIdcApplicationArn, which has no Namespace member at all and is therefore kept out of every other namespace response, observable only via this op's own response, matching the AdminUserPassword accept-then-scope-limited convention already used elsewhere in this family; DryRun=true returns the real DryRunException (confirmed in service-2.json: \"request was successful, but dry run was enabled\") without mutating state, verified by TestServerless_UpdateLakehouseConfiguration_DryRun. LakehouseRegistrationStatus's exact string values (\"Registered\"/\"Deregistered\") are a direct derivation from the client's own LakehouseRegistration request value, not an invented vocabulary -- real AWS documents no enum for this field (plain *string in types.Namespace). GetIdentityCenterAuthToken mints a synthetic opaque token after validating every named workgroup actually exists (a real FK check classic Redshift's own same-named operation, handler_idc_applications.go, does not even perform) -- following the identical honest-limitation precedent classic Redshift's sibling op of the same name already established (no real IAM Identity Center backend exists here to mint a real token). DELIBERATELY NOT BUILT: the reservation-capacity family (CreateReservation/GetReservation/GetReservationOffering/ListReservationOfferings/ListReservations) -- judged as fabrication rather than honest emulation and left in sdk_completeness_test.go's notImplemented slice; see items_still_open for the full reasoning, which turns on ReservationOffering's AWS-set commercial pricing having no fixed SDK-enumerable catalog to derive from (unlike classic Redshift's own ReservedNode, whose curated offering catalog -- see families.ReservedNode -- keys off a small, real, AWS-documented hardware node-type list, not free-floating commercial rates) and this family having zero pre-existing backend state. New store.Table (slLakehouseConfig) registered/reset/persisted via the standard store.Registry mechanism, no snapshot version bump (additive Tables map); wiring proven load-bearing by temporarily removing both the store_setup.go registration and the slDispatchTable entries and confirming the new tests fail (nil-pointer panic and ValidationException \"unknown operation\" respectively) before restoring."}
gaps: []          # bd gopherstack-0eyk (IdcApplication missing inner <RedshiftIdcApplication>
                   # wrapper) FIXED this pass -- see families.IdcApplication above for detail.
deferred: []      # all 17 prior deferred families field-diffed in the 2026-07-22 pass, see families above
leaks: {status: clean, note: "reviewed reconciler.go: StartReconciler/StopReconciler use a WaitGroup + stop channel, idempotent, no per-cluster goroutines. New Qev2IdcApplication store.Table this pass introduces no goroutines/tickers -- registered through the existing store.Registry the same way every other table is (store_setup.go), snapshotted/restored generically via registry.SnapshotAll/RestoreAll, no bespoke persistence code added."}
---

## Notes

### 2026-08-23 pass (third): closed the RevokeClusterSecurityGroupIngress follow-up, re-derived coverage, found the Authorize-side sibling bug (no bd id assigned this session)

Started with the one named follow-up the prior continued pass left open:
**`RevokeClusterSecurityGroupIngress`** (`security_groups.go`) filtered
`IPRanges`/`EC2SecurityGroups` without tracking whether anything actually
matched, so revoking a rule that was never authorized returned 200 OK with
the group unchanged instead of `AuthorizationNotFound` (confirmed in this
op's own declared error switch,
`awsAwsquery_deserializeOpErrorRevokeClusterSecurityGroupIngress`,
`deserializers.go:16495` -- `AuthorizationNotFound`/
`ClusterSecurityGroupNotFound`/`InvalidClusterSecurityGroupState`, no
`InvalidParameterValue`-shaped fault). Fixed the same way as the
`RevokeSnapshotAccess` fix it was modeled on: added a new sentinel
(`ErrSecurityGroupIngressNotFound`, `AuthorizationNotFound`, distinct from
`ErrSnapshotAccessNotFound` per this file's existing same-text-different-
sentinel convention) and return it when neither the CIDR nor EC2-group filter
actually removed anything. Extracted the filtering into two small helpers
(`revokeCIDRIngress`/`revokeEC2GroupIngress`) to keep the backend method under
`gocognit`'s complexity ceiling (this project bans `//nolint:gocognit`).
Test: `testRevokeClusterSecurityGroupIngressAuthorizationNotFoundErrorCode`
(SDK round trip) plus a new `cidr_never_authorized` handler-level case.

**Re-derivation of remaining coverage** (per this campaign's own protocol,
diffing dispatched op-name strings against the manifest rather than trusting
prose): ran `go run ./cmd/opcensus -json` for redshift's full 193-op
`allOps` list and grepped each literal name against this file. The naive
diff flagged ~80 "missing" names (`AuthorizeDataShare`,
`CreateClusterParameterGroup`, `DescribeHsmClientCertificates`,
`GetWorkgroup`, etc.) -- **every one hand-checked and confirmed a false
positive**, the exact trap this campaign's protocol warns about: this file
documents coverage at the family level with wildcard prose (`Describe*`,
`Create/Get/List/Update/Delete<Resource>`, `GetReservedNodeExchange*`) rather
than spelling out every literal op name, so a bare grep can't see it. Spot
checks confirmed the pattern holds: `ModifySnapshotSchedule` and
`ModifyClusterSnapshotSchedule` are two distinct real ops (confirmed both
declared in `interfaces.go`/`handler.go`, both with their own backend
methods) that a naive name diff could have mistaken for one op wrongly named
-- they aren't. **Conclusion: 0 ops remain genuinely unaudited.** All 193
dispatched op names trace to an existing `ops:`/`families:` entry, either
literally or via one of the wildcard prose patterns above.

**Second find, from this campaign's own "check every op sharing the family"
rule applied to the op just fixed**: `RevokeClusterSecurityGroupIngress`'s
sibling, **`AuthorizeClusterSecurityGroupIngress`**, had the mirror-image
bug -- re-authorizing a CIDR or EC2 security group already present on the
group silently appended a second, duplicate entry instead of returning
`AuthorizationAlreadyExists` (declared in this op's own error switch,
`awsAwsquery_deserializeOpErrorAuthorizeClusterSecurityGroupIngress`,
confirmed real by its own type doc comment in `types/errors.go`: "The
specified CIDR block or EC2 security group is already authorized for the
specified cluster security group"). Checking the analogous grant-list op,
**`AuthorizeSnapshotAccess`** (`snapshots.go`), found the identical gap --
also declares `AuthorizationAlreadyExists`, also silently duplicated the
`AccountsWithRestoreAccess` entry on a second call. Confirmed this class of
bug is real and not an invented rule by checking the one already-correct
sibling in this service: `AuthorizeEndpointAccess`
(`handler_endpoint_authorization.go`) already rejects a duplicate grant with
`EndpointAuthorizationAlreadyExists`, proven by its own pre-existing test
(`TestAuthorizeEndpointAccess_DuplicateReturnsError`) -- the exact same
grant-list-dedup pattern, already established elsewhere in this codebase, was
simply missing from these two ops. Fixed both (`security_groups.go`,
`snapshots.go`) with a new shared sentinel `ErrAuthorizationAlreadyExists`.
One pre-existing test, `TestAuthorizeSnapshotAccess_DuplicateAccount`,
asserted the *buggy* behavior outright (comment: "AWS allows multiple
accounts") -- that claim was never checked against the SDK error switch or
the sibling `AuthorizeEndpointAccess` convention; corrected to assert the
real `AuthorizationAlreadyExists` error. New tests:
`testAuthorizeSnapshotAccessAlreadyExistsErrorCode`,
`testAuthorizeClusterSecurityGroupIngressAlreadyExistsErrorCode` (both SDK
round trip), plus a `duplicate_cidr` handler-level case for
`AuthorizeClusterSecurityGroupIngress`.

All three fixes this pass proven with a real-SDK-client round trip
(`handler_sdk_roundtrip_test.go`), each hand-reverted (including after the
`gocognit` decomposition, so the proof matches the shipped shape) and
confirmed to fail against the unfixed code before restoring
(`md5sum`-identical restore each time). No persisted struct's JSON tag or
field type/name changed -- no new fields on `ClusterSecurityGroup`/`Snapshot`,
only new validation logic and error sentinels -- so **no
`redshiftSnapshotVersion` bump**. `go test ./pkgs/persistence/...` passes
clean (no other in-progress session's files were dirty this time, confirmed
via `git status` up front -- `services/ec2/` was checked and untouched, as
instructed).

**Not reached this pass**: nothing -- see the re-derivation above, which
concludes 0 ops remain unaudited. `golangci-lint run
./services/redshift/...` clean (0 issues) after the `gocognit` decomposition;
`go test -race ./services/redshift/...` clean.

### 2026-08-23 pass (continued): re-derived the "not reached" queue and audited it (no bd id assigned this session)

The prior entry below named "roughly 100 of the ~133" `*Handler` ops as not
reached and listed them only by family/prefix, not literal op name. Per this
campaign's own re-derivation protocol, diffed every one of the 133 declared
`*Handler` op names against every family/op entry actually present in this
file (not just this file's own prose claim) before trusting that count.
Result: only **5 of 133** ops have zero real audit trail anywhere in this
file -- `DescribeLoggingStatus`, `EnableLogging`, `DisableLogging`,
`ModifyClusterSnapshot`, `RevokeSnapshotAccess`. Every other op the prior
entry's prose called "not reached" is in fact covered by an existing
`families:` entry above (e.g. `ClusterParameterGroup`, `EventSubscription/
Events`, `Hsm*`, `Partner`, `UsageLimit`, `AuthenticationProfile`,
`EndpointAccess`/`EndpointAuthorization`, `CustomDomainAssociation`,
`Integration`, `Qev2IdcApplication`, `DataShare`) with real field-diff
citations against `deserializers.go`/`serializers.go` from earlier passes
(2026-07-22 through 2026-08-14) -- those audits predate the opcensus fix but
were done by reading source directly, not by the broken coverage tool, so
they hold. **The prior entry's "~100" figure was wrong** -- both the two
ledgers (this file's own `families:` table vs. this file's own dated-entry
prose) disagreed with each other, and the family table was the accurate one.
Two of the five true gaps (`EnableLogging`, `DisableLogging`) held up clean
on inspection -- no bug found. Three did not:

- **`DescribeLoggingStatus`** (`handler.go`, moved to `handler_events.go`):
  hardcoded `LoggingEnabled: false` unconditionally, ignoring
  `ClusterIdentifier` entirely and never consulting `b.loggingStatuses`, the
  very map `EnableLogging`/`DisableLogging` already populate (`events.go`).
  A real client could never observe logging state it had itself just
  enabled. Wire shape (`DescribeLoggingStatusResult`, flat, no wrapper --
  confirmed against `awsAwsquery_deserializeOpDescribeLoggingStatus` in
  `deserializers.go`) was already correct; this was a pure
  state-never-surfaced bug. Added `StorageBackend.GetLoggingStatus` and a
  real `handleDescribeLoggingStatus`. Test:
  `testDescribeLoggingStatusReflectsRealState`.
- **`ModifyClusterSnapshot` / `BatchModifyClusterSnapshots`** (siblings
  sharing the identical bug shape -- checking the family paid off again):
  `ManualSnapshotRetentionPeriod` is optional on both
  `ModifyClusterSnapshotInput` and `BatchModifyClusterSnapshotsInput` (`*int32`,
  no "required" doc comment, confirmed against `api_op_ModifyClusterSnapshot.go`
  / `api_op_BatchModifyClusterSnapshots.go`). Both handlers used a bare `int`
  sentinel of `-1` for "field omitted", which collides with the real,
  explicit wire value `-1` ("retain indefinitely") -- so a `Force`-only call
  with no `ManualSnapshotRetentionPeriod` silently reset every named
  snapshot's real retention period to indefinite. Fixed by changing both
  `StorageBackend` methods to take `*int` and only assign when non-nil,
  distinguishing "omitted" from "explicit -1". Tests:
  `testModifyClusterSnapshotOmittedRetentionPreserved`,
  `testBatchModifyClusterSnapshotsOmittedRetentionPreserved`.
- **`RevokeSnapshotAccess`** (`snapshots.go`): revoking access for an
  account that was never granted it returned `InvalidParameterValue`, a
  fault code not in this op's own declared error switch at all
  (`awsAwsquery_deserializeOpErrorRevokeSnapshotAccess`:
  `AccessToSnapshotDenied`/`AuthorizationNotFound`/`ClusterSnapshotNotFound`/
  `UnsupportedOperation`). The real fault is `AuthorizationNotFound`
  (`types.AuthorizationNotFoundFault`), the same fault family
  `RevokeClusterSecurityGroupIngress` declares for the identical
  nothing-to-revoke condition. Added `ErrSnapshotAccessNotFound` and wired
  it in; also added the missing `AccountWithRestoreAccess`-required check
  (`AuthorizeSnapshotAccess`, its sibling, already had it). Test:
  `testRevokeSnapshotAccessAuthorizationNotFoundErrorCode` (asserts
  `smithy.APIError.ErrorCode() == "AuthorizationNotFound"` via a real
  client).

All three fixes proven with a real-SDK-client round trip
(`handler_sdk_roundtrip_test.go`), each hand-reverted and confirmed to fail
against the unfixed code before restoring the fix (`md5sum`-identical
restore each time). No persisted struct's JSON tag or field type/name
changed -- `LoggingStatus`/`Snapshot` themselves are untouched; only the
`StorageBackend` interface's `ModifyClusterSnapshot`/
`BatchModifyClusterSnapshots`/(new)`GetLoggingStatus` method signatures
changed, which is Go-level, not wire/disk-level -- **no
`redshiftSnapshotVersion` bump**. `go test ./pkgs/persistence/...` run
regardless per protocol; passes for every service except `glue`, which
another in-progress session was actively editing at the time (confirmed via
`git status`, out of scope here, not touched).

**Spot-checked beyond the 5 confirmed gaps** (the family table's two
weakest-evidenced "no changes needed"-with-no-citation entries,
`ClusterParameterGroup` and `ClusterSecurityGroup`): `ClusterParameterGroup`
held up. `ClusterSecurityGroup` did not, fully -- **documented, not fixed,
out of scope for this pass**: `RevokeClusterSecurityGroupIngress`
(`security_groups.go`) filters `IPRanges`/`EC2SecurityGroups` without
tracking whether anything actually matched, so revoking a rule that was
never authorized silently succeeds (200 OK, unchanged security group)
instead of returning `AuthorizationNotFound` -- confirmed real via the same
`AuthorizationNotFound` case in
`awsAwsquery_deserializeOpErrorRevokeClusterSecurityGroupIngress`'s error
switch. Same bug shape as the `RevokeSnapshotAccess` fix above; left for a
follow-up since it wasn't part of the re-derived 5-op queue this pass
committed to.

**Not reached this pass**: nothing within the re-derived 5-op queue; the
`ClusterSecurityGroup` finding above is additional, outside that queue, and
intentionally left open.

### 2026-08-23 pass: cluster-management ops audit after opcensus union-bug fix (no bd id assigned this session)

`cmd/opcensus` used to index `GetSupportedOperations` by name only, so when a
service declared the method in two files (here, `*Handler` and
`*ServerlessHandler`), the second silently overwrote the first in every
downstream tool. For redshift that dropped `*Handler`'s ~133 ops (the whole
classic cluster-management surface: `CreateCluster`, `DescribeClusters`, etc.)
from every audit/coverage run to date -- they were never counted, so this is
the first pass to actually look at them as a group. Confirmed via
`go run ./cmd/opcensus -json` (`declaredBy: [*Handler, *ServerlessHandler]`,
`total: 193`) and `cmd/clientcoverage` (15/193 real-SDK-client-tested, 7.8%,
the worst ratio of any large service) before auditing.

Audited (wire shape only, against `redshift@v1.65.4/deserializers.go`,
confirmed AWS Query/XML case-insensitive decoding first so casing near-misses
were not filed): the shared `xmlCluster`/`toXMLClusterWithTags` struct (backs
`CreateCluster`, `DeleteCluster`, `DescribeClusters`, `ModifyCluster`,
`RebootCluster`, `PauseCluster`, `ResumeCluster`, `ResizeCluster`,
`RotateEncryptionKey`, `ModifyClusterIamRoles`, `ModifyClusterMaintenance`,
`RestoreFromClusterSnapshot`, `FailoverPrimaryCompute`), `ModifyClusterDbRevision`,
`DescribeClusterDbRevisions`, `CancelResize`/`DescribeResize`, all six
reserved-node ops, `DescribeAccountAttributes`, `DescribeClusterTracks`,
`DescribeClusterVersions`, `DescribeOrderableClusterOptions`, `DescribeStorage`,
`DescribeNodeConfigurationOptions`, `ListRecommendations`, and the Snapshot
family (`CreateClusterSnapshot`/`DeleteClusterSnapshot`/
`DescribeClusterSnapshots`/`CopyClusterSnapshot`/`AuthorizeSnapshotAccess`).

Two real bugs found and fixed, both proven with a real-SDK-client round trip
(`handler_sdk_roundtrip_test.go`, confirmed failing against hand-reverted
code before the fix, restored copy `md5sum`-identical to the fix):

- **`ModifyClusterDbRevision`** (`handler_cluster_mgmt.go`): every other
  Cluster-returning op wraps its XML as `<XxxResult><Cluster>...</Cluster></XxxResult>`,
  matching `awsAwsquery_deserializeOpDocumentModifyClusterDbRevisionOutput`
  (`deserializers.go:52728`), which looks for a nested `<Cluster>` element.
  This op alone flattened `xmlCluster`'s fields directly under
  `<ModifyClusterDbRevisionResult>` with no `<Cluster>` wrapper (`Result
  xmlCluster \`xml:"ModifyClusterDbRevisionResult"\``), so a real client's
  `ModifyClusterDbRevisionOutput.Cluster` always decoded nil regardless of
  backend state. Fixed by renaming the field to `Cluster` and adding
  `>Cluster` to the tag, matching every sibling op. Test:
  `testModifyClusterDBRevisionClusterWrapper`.
- **`ListRecommendations`** (`handler_advisor.go`): `recommendationXML.Type`
  was tagged `xml:"Type"`, but the real `Recommendation` document deserializer
  (`deserializers.go`, case list around `awsAwsquery_deserializeDocumentRecommendation`)
  has no `Type` case at all -- the field is named `RecommendationType`
  (`types.Recommendation.RecommendationType`, a `*string`). The handler
  always populates a real value (e.g. `"Security"`), so this wasn't a
  modelling gap -- a real client's `RecommendationType` always decoded to
  `nil`. Fixed the tag to `xml:"RecommendationType"`. Test:
  `testListRecommendationsRecommendationType`.

One finding dropped as unprovable: `CancelResize`/`DescribeResize`'s shared
`xmlResizeProgress` emits `<AllowCancelResize>`, but neither
`CancelResizeOutput` nor `DescribeResizeOutput` has any such case in their
deserializers (confirmed: 16 cases each, `AllowCancelResize` absent from
both). It turns out to be a real field, but on a different type entirely --
`types.ResizeInfo.AllowCancelResize`, nested under `types.Cluster.ResizeInfo`
(itself not modeled in `xmlCluster` -- a separate, unfixed modelling gap).
Every deserializer here defaults unmatched elements to
`decoder.Decoder.Skip()`, so the fabricated element is silently discarded by
a real client and there is no observable behavioral difference to assert
before/after removing it -- it fails this audit's own proof bar (a real-SDK
round trip that fails pre-fix), so left in place and only documented here.

Everything else checked (list wrapper names, member field names, nested
struct paths) matched the real deserializer exactly; the remaining
differences from the full `types.Cluster`/`types.Snapshot`/etc. field sets
are gopherstack never emitting an optional member it doesn't track --
modelling gaps, not wire bugs, per this campaign's own rule.

**Not reached this pass** (named per the "an unnamed op is an unchecked op"
rule) -- roughly 100 of the ~133 previously-invisible `*Handler` ops,
concentrated in: `ClusterSecurityGroup`/`ClusterSubnetGroup`/
`ClusterParameterGroup` CRUD, `EventSubscription`/`DescribeEvents`,
`Hsm*`, `Partner*`, `SnapshotCopyGrant`/`SnapshotSchedule`/`EnableSnapshotCopy`/
`DisableSnapshotCopy`, `UsageLimit` CRUD, `AuthenticationProfile` CRUD,
`EndpointAccess`/`EndpointAuthorization` CRUD, `CustomDomainAssociation` CRUD,
`Integration`/`InboundIntegration`, `Qev2IdcApplication`/`RedshiftIdcApplication`
CRUD, `DataShare` CRUD, `BatchModifyClusterSnapshots`, `ModifyClusterSnapshot`,
`ModifySnapshotCopyRetentionPeriod`, `RevokeSnapshotAccess`,
`RestoreTableFromClusterSnapshot`, `AddPartner`/`DeletePartner`/
`UpdatePartnerStatus`, `GetResourcePolicy`/`PutResourcePolicy`/
`DeleteResourcePolicy`, `RegisterNamespace`/`DeregisterNamespace` (already
covered by the 2026-08-13 gopherstack-3jqz pass above). Several of these
already have real-SDK-client tests from prior passes (see the 2026-08-13/
2026-08-08 entries below); this pass did not re-verify those, per the
schema's own re-audit protocol (unchanged files since their `last_audit_commit`
are trusted).


### 2026-08-13 pass: CreateHsmConfiguration dropped both HSM secrets (bd gopherstack-afi1)

From the "five ops drop the fields that define what they do" required-member
sweep. `handler_hsm.go:handleCreateHsmConfiguration`/
`hsm.go:CreateHsmConfiguration` read/passed 4 of 6 required members --
`HsmPartitionPassword` and `HsmServerPublicCertificate`
(`api_op_CreateHsmConfiguration.go:64,70`) never reached the backend, whose
signature had no parameters for them at all. See the
`families.HsmClientCertificate/HsmConfiguration` entry above for the full
fix/credential-handling detail. `TestHandler_CreateHsmConfiguration`'s
`"success"`/`"duplicate"` cases and the setup calls in
`TestHandler_DeleteHsmConfiguration`/`TestHandler_DescribeHsmConfigurations`
previously omitted both fields entirely -- they would have passed identically
whether or not the handler ever read them, encoding the same gap as the bug.
All updated to supply both required fields (via a shared
`hsmRequiredSecrets` body-suffix constant); added
`"missing_partition_password"`/`"missing_server_public_certificate"` cases
and an explicit assertion that the submitted secret value never appears
anywhere in the response body, on every case in the table (not just the
success path).

### 2026-08-13 pass: ModifyAquaConfiguration, ModifyLakehouseConfiguration (classic Redshift) (bd gopherstack-6xxt)

Follow-up to gopherstack-3jqz below: fixes the two no-op stubs that audit
flagged but left out of scope. Both `handleModifyAquaConfiguration` and
`handleModifyLakehouseConfiguration` (`handler_cluster_mgmt.go`) took
`_ url.Values`, ignoring their required `ClusterIdentifier` entirely with no
existence check. See `families.AquaConfiguration`/`families.LakehouseConfiguration`
above for the full account; short version:

- **`ModifyAquaConfiguration`** now existence-checks `ClusterIdentifier`
  (`ClusterNotFound`, declared in the op's own error switch) via a new
  `ModifyAquaConfiguration(id)` backend method. The canned response is
  unavoidable by design -- the real op is documented retired ("Calling this
  operation does not change AQUA configuration") -- but it now shares a
  single `defaultAquaConfig()` helper with `toXMLClusterWithTags` instead of
  hardcoding a second, different canned value (the stub previously returned
  `AquaConfigurationStatus=auto`, while every `DescribeClusters` response
  already returned `disabled` for the same field -- a real client got a
  different answer depending which op it called).
- **`ModifyLakehouseConfiguration`** required a bigger fix: classic
  `Cluster` had no `CatalogArn`/`LakehouseRegistrationStatus` fields at all,
  even though both are confirmed real `types.Cluster` members
  (`aws-sdk-go-v2/service/redshift@v1.65.4/types/types.go:153,343`) -- this
  backend already modeled the equivalent state for Redshift Serverless
  (`Namespace.CatalogArn`/`LakehouseRegistrationStatus`, wired the same day),
  so the classic version was simply left behind. Added to `Cluster` and
  wired into `xmlCluster`/`toXMLClusterWithTags` so every cluster-returning
  op echoes them, not just this op's own response. The backend method
  (`lakehouse.go`) follows `UpdateLakehouseConfigurationSL`'s existing
  carry-forward-when-omitted pattern almost exactly (same `arn.Build`
  derivation, same "existing value survives a call that only touches one
  field" behavior), with a new cluster-keyed `ClusterLakehouseConfig` store
  table for `LakehouseIdcApplicationArn` (no `Cluster` member on the real
  wire, same as its Serverless counterpart).
- **Two second-layer findings**, from reading the op's full real
  input/output rather than only `ClusterIdentifier`: (1)
  `LakehouseIdcApplicationArn`, when the caller sets a new one, is now
  validated against this backend's own `RedshiftIdcApplication` store
  (`idc_applications.go`) -- real cross-reference validation this backend
  can perform because it already models that resource; a miss returns
  `RedshiftIdcApplicationNotExists`, declared in this op's own error switch.
  The Serverless sibling has no such backend to check against, so it
  doesn't do this -- not a discrepancy, a capability this family happens to
  have. (2) `DryRun` does **not** map to a `DryRunException` here the way
  the Serverless sibling's `UpdateLakehouseConfiguration` does -- confirmed
  absent from `awsAwsquery_deserializeOpErrorModifyLakehouseConfiguration`'s
  declared switch. `DryRun`'s own doc text ("validates the request without
  actually modifying the lakehouse configuration") is honored literally
  instead: a successful dry run runs every validation and returns the
  would-be result as an ordinary 200, without persisting it. Assuming the
  Serverless sibling's DryRunException behavior here would have been wrong.
- Error codes used (`ClusterNotFound`, `RedshiftIdcApplicationNotExists`)
  both come from each op's own declared switch.
  `InvalidClusterState`/`UnsupportedOperation` (both ops) and
  `DependentServiceAccessDenied`/`DependentServiceUnavailableFault`/
  `UnauthorizedOperation` (Lakehouse only) remain declared but unused -- no
  real precondition for any is discoverable from this backend's state, left
  honest rather than inventing triggers, matching this repo's existing
  convention for declared-but-unreachable exceptions (see glue's
  `OperationTimeoutException` reasoning in the sibling service's own
  PARITY.md for the same judgment call).

Tests: table-driven handler tests for both ops (existence check, missing
ClusterIdentifier, IDC-application cross-reference miss) plus dedicated
tests for DryRun (no mutation), persistence/carry-forward across separate
calls, and a real `aws-sdk-go-v2` client round trip
(`TestSDKRoundTrip_ModifyLakehouseConfiguration`) proving
`CatalogArn`/`ClusterIdentifier`/`LakehouseIdcApplicationArn`/
`LakehouseRegistrationStatus` decode correctly and that `DescribeClusters`
decodes the same `Cluster.CatalogArn`/`LakehouseRegistrationStatus` fields
this pass added. All new/changed tests hand-verified to fail against the
pre-fix handlers (temporarily reverted both handler functions to their old
stub bodies, confirmed the expected failures, restored).

Gates run this pass, all green: `go build`, `go vet`, `go test -race`,
`go fix -diff` (no diff), `golangci-lint run` (0 issues).

### 2026-08-13 pass: UpdateSnapshot, GetTrack/ListTracks, UpdateLakehouseConfiguration, GetIdentityCenterAuthToken; reservation family deliberately deferred (bd gopherstack-v4wu)

Follow-up to gopherstack-0w2p/8v8v/mbcq below: `TestSDKCompleteness_Serverless`
found ten operations with zero code (filed as gopherstack-irh7, duplicate of
this issue). This pass implements four of the ten and documents why the
fifth -- the reservation-capacity family -- is a deliberate gap rather than
an oversight. See the `Redshift Serverless` family row's final addendum for
the full account; short version:

- **`UpdateSnapshot`** (the most conspicuous gap: Create/Get/List/Delete
  snapshot all existed, so CRUD symmetry was broken) now completes the
  family. `RetentionPeriod` is optional and nilable on the real
  `UpdateSnapshotInput` (confirmed against `api_op_UpdateSnapshot.go`) --
  omitting it leaves the stored value unchanged, proven by
  `TestServerless_UpdateSnapshot_OmittedRetentionPeriodUnchanged`, and the
  retention-period change itself is proven observable through a second
  `GetSnapshot` call, not just the `UpdateSnapshot` response
  (`TestServerless_SnapshotCRUD`).
- **`GetTrack`/`ListTracks`** return a static two-entry catalog (`current`,
  `trailing`, both at this backend's single `modelVersion10` release) --
  the exact same precedent classic Redshift's own `DescribeClusterTracks`
  already set for the identical real-world enumeration
  (`handler_cluster_info.go`). `UpdateTargets` (the list of newer versions a
  track could update to) is honestly left empty: this backend has one static
  release, so there is no second version to invent an upgrade path to.
- **`UpdateLakehouseConfiguration`** writes `Namespace.CatalogArn`/
  `LakehouseRegistrationStatus` -- both confirmed present on real
  `types.Namespace` (`types/types.go`) but previously entirely absent from
  this backend's `Namespace` struct, a genuine pre-existing wire gap this
  pass also closes, not new fabrication. `LakehouseIdcApplicationArn` has NO
  `Namespace` member at all in the real SDK, so it is kept out of every other
  namespace response and lives in a new namespace-keyed store table
  (`slLakehouseConfig`, `serverless_lakehouse.go`), observable only through
  this operation's own response -- proven to survive a later call that
  changes only the registration status
  (`TestServerless_UpdateLakehouseConfiguration_RegisterAndAssociate`) and
  through a full persistence round trip
  (`TestInMemoryBackend_FullStateRoundTrip`). `DryRun: true` returns the real
  `DryRunException` (confirmed in the pinned `service-2.json`: "the request
  was successful, but dry run was enabled so no action was taken") without
  mutating state -- initially modeled this as a 200 with preview data before
  reading the deserializer's error-set comment, which is exactly the kind of
  mistake this repo's SDK-shape discipline exists to catch.
  `LakehouseRegistrationStatus`'s exact string values (`"Registered"`/
  `"Deregistered"`) are a direct derivation from the client's own
  `LakehouseRegistration` request value, not an invented vocabulary -- real
  AWS documents no enum for this field (a plain `*string`, confirmed in
  `types/types.go`).
- **`GetIdentityCenterAuthToken`** mints a synthetic opaque token after
  validating every named workgroup actually exists in this backend -- an FK
  check classic Redshift's own operation of the identical name
  (`handleGetIdentityCenterAuthToken`, `handler_idc_applications.go`) does
  not even perform. The token-minting approach itself follows that sibling
  operation's own precedent, already judged acceptable by a prior audit
  (`families.Descriptive/static ops` above lists it as spot-checked ok): no
  real IAM Identity Center backend exists here to mint a real token, so a
  synthetic opaque value is the honest ceiling, not a shortcut invented for
  this pass.
- **Reservation-capacity family deliberately NOT built**
  (`CreateReservation`/`GetReservation`/`GetReservationOffering`/
  `ListReservationOfferings`/`ListReservations`), still listed in
  `sdk_completeness_test.go`'s `notImplemented` slice. Judgment call, argued
  both ways: this package already has a directly analogous precedent
  (classic Redshift's `ReservedNode`/`defaultReservedNodeOfferings`,
  `reserved_nodes.go`) -- a curated, fabricated-but-consistent catalog of
  offering IDs/prices, graded `ok` by a prior audit. Built the same way here,
  a `CreateReservation` implementation would be structurally
  straightforward (`PurchaseReservedNodeOffering` is the exact same shape:
  no cluster/namespace reference, just an offering ID and a billing
  commitment). What tips this the other way: `ReservedNode`'s offering
  catalog keys off `NodeType` (`dc2.large`, `ra3.xlplus`, ...), a small, real,
  AWS-published hardware SKU list that constrains what a "curated" catalog
  can honestly contain. `ReservationOffering` has no such anchor -- it is
  `{Capacity RPUs, HourlyCharge, UpfrontCharge, CurrencyCode, OfferingType}`,
  free-floating commercial pricing AWS derives from its own live rate cards
  (confirmed: `ListReservationOfferings`'s own doc comment is "Returns the
  current reservation offerings in your account" -- "current" implying rates
  that move, not a fixed catalog). A gopherstack client cannot tell a
  plausible-looking price from a real one, and unlike a wrong ARN or status
  code (which fails loudly), a wrong dollar figure silently corrupts any
  cost-projection logic built on top of it. This is the invented-capability
  case parity-principles.md warns about, not the same kind of judgment call
  the `ReservedNode` precedent already settled -- and this family additionally
  has zero pre-existing backend state (no `CreateReservation` call has ever
  run here) to hang a reservation's identity on, unlike every other op built
  this pass, each of which extended CRUD symmetry or wire completeness on an
  already-real resource. Recorded here rather than silently reclassified so
  the next audit can revisit the call with both arguments in view.

Gates run this pass, all green: `go build`, `go vet`, `go test -race`,
`go fix -diff` (no diff), `golangci-lint run` (0 issues). New/changed tests
verified to have teeth, not just pass vacuously: temporarily removed the five
new `slDispatchTable` entries (file edit, reverted after) and confirmed every
new handler test flips to `ValidationException`/"unknown operation"; and
separately removed the `slLakehouseConfig` `store_setup.go` registration and
confirmed `TestInMemoryBackend_FullStateRoundTrip` panics with a nil-pointer
dereference (proving the table wiring, not just the test assertions, is
load-bearing) before restoring both files.

### 2026-08-13 pass: Redshift Serverless go.mod pin, phantom UpdateNamespace.DBName, nine request-member gaps (bd gopherstack-0w2p/8v8v/mbcq)

See the `Redshift Serverless` family row above for the full account. Short
version: `aws-sdk-go-v2/service/redshiftserverless` was pinned into `go.mod`
for real this time (v1.38.5, matched to the same upstream release batch as
the already-pinned `redshift`/`redshiftdata`), kept alive against `go mod
tidy` by a new `TestSDKCompleteness_Serverless` test rather than a bare
reference import -- that test doubles as real verification tooling and
immediately found 10 entirely-unimplemented operations (filed as
gopherstack-irh7, not built this pass). Re-verifying the prior audit's
field-level findings against the newly-pinned source changed nothing: the
module cache copy it was read from was already v1.38.5. Fixed:
`UpdateNamespace`'s phantom `dbName` field/mutation (gopherstack-8v8v,
removed); `AdminUserPassword` on Create/UpdateNamespace (credential,
accepted then explicitly discarded, never persisted or echoed);
`RedshiftIdcApplicationArn` on CreateNamespace; `MaintainIntegration` on
RestoreFromSnapshot; `ActivateCaseSensitiveIdentifier` on both table-restore
ops; and five real filter gaps (ListSnapshots EndTime/StartTime/
NamespaceArn/OwnerAccount, ListRecoveryPoints EndTime/StartTime,
ListWorkgroups OwnerAccount, GetSnapshot OwnerAccount, ListUsageLimits
UsageType) -- OwnerAccount implemented as an honest single-account
comparison against `b.accountID`, not a no-op. `ListEndpointAccess`'s VpcId
omission re-confirmed correct, left untouched.

### 2026-08-10 pass: Redshift Serverless EndpointAccess, ListManagedWorkgroups, RestoreFromSnapshot, ConvertRecoveryPointToSnapshot (bd gopherstack-w8g2, final pass)

Eighth/ninth of the nine originally-missing serverless families, plus the two
restore/convert ops left aside earlier -- this closes bd gopherstack-w8g2.
Confirmed against the pinned `botocore` `redshift-serverless/2021-04-21/
service-2.json.gz` (protocol `json` 1.1, botocore 1.43.56) and cross-checked
against `aws-sdk-go-v2/service/redshiftserverless@v1.38.5`'s serializers.go/
deserializers.go/types (pulled into GOMODCACHE via `go get` for this pass,
confirmed `git status --porcelain go.mod go.sum` clean before and after, and
`go mod tidy` a no-op).

**EndpointAccess** was explicitly flagged as needing "the same
no-per-ENI-AZ/IP-data judgement call classic Redshift's EndpointAccess
already made." Re-read `endpoint_access.go`/`handler_endpoint_access.go`
first: classic leaves the entire nested `VpcEndpoint`/network-interface
object absent from every response rather than fabricating a `vpcEndpointId`
with no real ENI allocation behind it (see `families.EndpointAccess`'s
addendum). The serverless `EndpointAccess` shape has the identical problem
in a different nesting: real `types.VpcEndpoint.NetworkInterfaces` needs
`AvailabilityZone`/`PrivateIpAddress`/`NetworkInterfaceId`/`SubnetId` per
ENI (confirmed against `types.NetworkInterface`), and this backend tracks
none of it anywhere (no EC2 cross-reference wired into Redshift at all,
same finding `families.ClusterSubnetGroup` already recorded). Followed the
classic precedent exactly: `vpcEndpoint` is omitted from every response,
not partially invented. What IS modeled: `address`/`endpointArn`/
`endpointCreateTime`/`endpointName`/`endpointStatus`/`port`/`subnetIds`/
`workgroupName`/`vpcSecurityGroups` -- all real, all backed by state this
backend actually holds. `vpcSecurityGroups` reuses classic's own
`VpcSecurityGroupMembership`-equivalent shape and its `"active"` status
convention (`endpointStatusActive`), since it is the identical real type.
`ListEndpointAccessRequest`'s `vpcId` filter is deliberately not accepted,
for the same no-vpcId-data reason. `DeleteEndpointAccessResponse` echoes
the deleted object (confirmed against service-2.json: unlike
`DeleteResourcePolicy`/`DeleteCustomDomainAssociation`'s zero-member
responses, this one carries a real `endpoint` member).

**ListManagedWorkgroups** was called "thin, with no real backing state
without data-sharing consumer modeling." Checked rather than assumed: it
holds. `ListManagedWorkgroupsRequest.sourceArn` is pattern-constrained to a
Glue Data Catalog database/catalog ARN in service-2.json, meaning
`ManagedWorkgroupListItem` represents a workgroup Glue/Lake Formation
auto-provisions for federated queries against shared data -- confirmed by
grepping this package for any Glue Data Catalog or Lake Formation
integration (none; `AssociateDataShareConsumer` is classic Redshift's
unrelated data-sharing feature). Implemented as an honest,
correctly-shaped, always-empty response (`ListManagedWorkgroupsSL`) -- no
`store.Table` added, since nothing in this backend could ever populate one.

**RestoreFromSnapshot** requires `namespaceName`/`workgroupName`, with the
identical "name of the namespace to restore ... to/into" wording and
required-field shape `RestoreFromRecoveryPointRequest` already uses; by
that symmetry it reuses the same already-reviewed design
`RestoreFromRecoveryPointSL` established in the prior pass (both resources
must pre-exist, FK-checked, no real data movement simulated -- the existing
`Namespace` is returned unchanged). `snapshotName`/`snapshotArn` resolve via
the same ARN-suffix-stripping convention `GetServerlessSnapshot` already
uses. `manageAdminPassword`/`adminPasswordSecretKmsKeyId` are threaded onto
the namespace in the true direction only -- the documented false-branch
behavior ("uses the admin credentials the namespace or cluster had at the
time the snapshot was taken") is data this backend cannot reconstruct, so
it is left untouched rather than fabricated.

**ConvertRecoveryPointToSnapshot** requires `recoveryPointId`/
`snapshotName`; writes a new `ServerlessSnapshot` from the recovery point's
own namespace linkage, reusing `CreateServerlessSnapshot`'s exact
conflict-check/arn.Build/store/index/tag sequence.

All four verified to genuinely fail beforehand: their `slDispatchTable`
entries were removed in a file copy (not `git stash`), the new tests
rerun, and every one flipped from real behavior to `"unknown operation"`
`ValidationException`/400 -- confirmed, then the entries were restored and
the diff checked identical to before.

One new `store.Table` registered this pass (`slEndpointAccesses`) plus its
`sortedStringIndex` -- no snapshot version bump, same additive convention
every prior family in this issue used. `ListManagedWorkgroupsSL` needed no
new table at all, since it has no backing state to hold.



Fifth/sixth/seventh of the nine originally-missing serverless families, taken
together deliberately as the "entangled group" the prior two passes flagged
and left for last: `RecoveryPoint`, `TableRestoreStatus`, and the two
restore operations that consume a recovery point
(`RestoreFromRecoveryPoint`, `RestoreTableFromRecoveryPoint`), plus
`RestoreTableFromSnapshot` (grouped with `TableRestoreStatus` since both
table-level restores write into the same result store). Confirmed against
the pinned `botocore` `redshift-serverless/2021-04-21/service-2.json.gz`
(protocol `json` 1.1, botocore 1.43.56) and cross-checked against
`aws-sdk-go-v2/service/redshiftserverless@v1.38.5`'s serializers.go/
deserializers.go already sitting in GOMODCACHE from prior passes (not
re-added to go.mod, confirmed via `git status --porcelain go.mod go.sum`
before and after, and `go mod tidy` producing no diff):

- **Where recovery points come from**: real AWS creates them automatically,
  "every 30 minutes ... kept for 24 hours" (the `RecoveryPoint` shape's own
  documentation in service-2.json) -- there is no `CreateRecoveryPoint`
  operation anywhere in this API's operation list, confirmed by enumerating
  every operation name. No wire-reachable create op was added (that would
  have been exactly the fabricated-API mistake this issue's instructions
  warned against). Instead, `generateRecoveryPointLocked`
  (`serverless_recovery.go`) creates exactly one recovery point per
  namespace+workgroup pair at `CreateWorkgroup` time, inline under the same
  lock `CreateWorkgroup` already holds (same pattern `DeleteNamespace`
  already uses for its final-snapshot write) -- a workgroup is required to
  exist before recovery points make sense at all (`RecoveryPoint.workgroupName`
  is a real field), and this backend has no real 30-minute scheduler
  infrastructure to run, consistent with its existing instant-apply
  simplifications elsewhere (snapshots created instantaneously, resizes
  applied synchronously). `AddRecoveryPointInternal` is a test-only seed
  helper (not wired to any op), matching the existing
  `AddSnapshotInternal`/`AddReservedNodeInternal` convention in this package.
- `GetRecoveryPointRequest` keys on `recoveryPointId`; `ListRecoveryPointsRequest`
  optionally filters by `namespaceArn`/`namespaceName` (both accepted; this
  backend does not filter by the request's `startTime`/`endTime` window --
  see items_still_open) and paginates via the existing shared
  `maxResults`/`nextToken` convention. Both response envelopes
  (`{"recoveryPoint": ...}` / `{"recoveryPoints": [...]}`) confirmed against
  `awsAwsjson11_deserializeOpDocumentGetRecoveryPointOutput`/
  `...ListRecoveryPointsOutput`.
- **Timestamp formats genuinely differ within this one entangled group**:
  `RecoveryPoint.recoveryPointCreateTime` is shape `SyntheticTimestamp_date_time`
  (ISO8601 string; confirmed both in service-2.json and via
  `awsAwsjson11_deserializeDocumentRecoveryPoint`'s `smithytime.ParseDateTime`
  call in deserializers.go), while `TableRestoreStatus.requestTime` is the
  bare `Timestamp` shape (epoch-seconds JSON number; confirmed via
  `awsAwsjson11_deserializeDocumentTableRestoreStatus`'s
  `smithytime.ParseEpochSeconds` call) -- re-verified per field rather than
  assumed from the nearer-looking sibling, same discipline the
  Tagging/CustomDomainAssociation pass applied to
  `customDomainCertificateExpiryTime` vs `GetCredentials`' `expiration`.
- `RestoreFromRecoveryPointRequest` requires `namespaceName`/`recoveryPointId`/
  `workgroupName` (all three required, confirmed in service-2.json);
  response is `{"namespace": {...}, "recoveryPointId": "..."}` (confirmed via
  `awsAwsjson11_deserializeOpDocumentRestoreFromRecoveryPointOutput`). This
  backend does not simulate real data movement (consistent with
  `CreateSnapshotCopyConfiguration`'s cross-region copy also not simulating
  real replication), so once FK checks pass -- namespace exists, workgroup
  exists, workgroup belongs to that namespace (this backend's existing
  Namespace-Workgroup invariant, not a fabricated recovery-point-specific
  rule), recovery point exists -- the existing `Namespace` is returned
  unchanged. `RestoreFromSnapshot` (namespace-level restore from a
  `Snapshot`, no recovery point involved at all) was deliberately **not**
  built this pass: it does not depend on `RecoveryPoint` and was excluded
  from this entangled group by design, matching the original issue's
  own framing ("split per family ... they are independent") -- still open,
  see items_still_open.
- `RestoreTableFromSnapshotRequest`/`RestoreTableFromRecoveryPointRequest`
  both require `namespaceName`/`newTableName`/`sourceDatabaseName`/
  `sourceTableName`/`workgroupName` plus their respective
  `snapshotName`/`recoveryPointId` (confirmed in service-2.json); both
  responses envelope under `tableRestoreStatus`
  (`awsAwsjson11_deserializeOpDocumentRestoreTableFrom{Snapshot,RecoveryPoint}Output`).
  Modeled as `ServerlessTableRestoreStatus` (`serverless.go`), distinct from
  classic Redshift's cluster-keyed `TableRestoreStatus` in `models.go` (same
  concept, different protocol, different resource keys -- disambiguated the
  same way `ServerlessResourcePolicy` was kept distinct from classic
  Redshift's `ResourcePolicy`). `Status` is set to `SUCCEEDED` immediately
  (this backend restores synchronously, consistent with the rest of this
  service) rather than left `IN_PROGRESS` forever the way classic Redshift's
  own `TableRestoreStatus` currently is (a pre-existing quirk in
  `table_restore.go`, out of this pass's scope, not touched).
  `ProgressInMegaBytes`/`TotalDataInMegaBytes` are honestly left at
  zero/omitted (`omitempty`) rather than fabricated -- this backend has no
  real data to move, same reasoning classic Redshift's own
  `items_still_open` entry already gives for the identical fields.
  `GetTableRestoreStatusRequest` keys on `tableRestoreRequestId`;
  `ListTableRestoreStatusRequest` optionally filters by
  `namespaceName`/`workgroupName`.

Proven with a persistence round trip extending
`TestInMemoryBackend_FullStateRoundTrip`: the `rt-workgroup` seed already in
that test auto-generates a recovery point (no new seed call needed for
that part), and a `RestoreTableFromSnapshotSL` call was added to seed a
`ServerlessTableRestoreStatus`. Verified the round trip has teeth two ways,
both reverted immediately after confirming the intended failure: (1)
temporarily renamed the two new tables' `store.Register` name strings so
the two failure modes could not be conflated, which -- as expected, since
Snapshot and Restore ran within the same process against the same renamed
keys -- did NOT reproduce data loss (a useful negative result, not just a
placeholder check); (2) temporarily commented out the two new
`rebuildFromKeys` calls in `rebuildServerlessIndexes`, which DID make
`TestInMemoryBackend_FullStateRoundTrip` fail with `"[]" should have 1
item(s), but has 0` on the new `ListRecoveryPointsSL`/`ListTableRestoreStatusSL`
assertions -- proving those two lines are load-bearing, not decorative.
Also verified the two new handler test files
(`handler_serverless_recovery_test.go`,
`handler_serverless_table_restore_test.go`) fail against the pre-pass
codebase: copied them into a `git worktree add --detach HEAD` checkout (not
`git stash`) with none of this pass's backend/handler files present, and
every new test failed there -- `unknown operation` `ValidationException`
(400) where the current tree returns 200/`ResourceNotFoundException`, since
none of these operations existed in `slDispatchTable` before this pass.

Two new `store.Table`s (`slRecoveryPoints`, `slTableRestoreStatuses`) plus
two new `sortedStringIndex`es (`slRecoveryPointIdx`,
`slTableRestoreStatusIdx`) registered/wired the same way every prior
serverless table is -- no snapshot version bump, `Registry.Tables` stays an
additive `map[string]json.RawMessage`.

Deliberately not modeled: `ListRecoveryPointsRequest`'s `startTime`/`endTime`
filter window (accepted nowhere -- not parsed at all, rather than parsed and
silently ignored, since this backend's single auto-generated recovery point
per workgroup has no meaningful creation-time range to filter against
without inventing one); `ConvertRecoveryPointToSnapshot` (a real op that
converts a recovery point into a `Snapshot` -- not part of this issue's
named restore-op list and not required for `RestoreFromRecoveryPoint`/
`RestoreTableFrom{Snapshot,RecoveryPoint}` to work, left for a future pass);
`RestoreFromSnapshot` (namespace-level restore from a `Snapshot`, no
recovery point dependency -- deliberately excluded from this entangled
group, see above); `TooManyTagsException`/`ServiceQuotaExceededException`
preconditions on the restore ops, consistent with this service's existing
unsimulated-quota precedent. EndpointAccess and ListManagedWorkgroups
remain unbuilt -- see items_still_open.

### 2026-08-10 pass: Redshift Serverless ResourcePolicy + SnapshotCopyConfiguration (bd gopherstack-w8g2)

Third and fourth of the nine originally-missing serverless families (two of
nine already done in the 2026-08-09 pass below); picked as the most
self-contained of the remaining seven -- neither depends on any other unbuilt
family (RecoveryPoint, TableRestoreStatus and the restore ops are entangled
with each other; EndpointAccess additionally needs the same
no-per-ENI-AZ/IP-data judgment call `families.EndpointAccess` already made for
classic Redshift; ListManagedWorkgroups has no real backing state in this
backend without data-sharing consumer modeling). Confirmed against the pinned
`botocore` `redshift-serverless/2021-04-21/service-2.json.gz` (protocol `json`
1.1) via `python3 -c "import gzip, json; ..."` against the installed botocore
1.43.56 package, cross-checked against
`aws-sdk-go-v2/service/redshiftserverless@v1.38.5`'s serializers.go/
deserializers.go already sitting in GOMODCACHE from the prior passes (not
re-added to go.mod):

- `GetResourcePolicyRequest`/`PutResourcePolicyRequest`/
  `DeleteResourcePolicyRequest` all key on `resourceArn`; `PutResourcePolicyRequest`
  additionally requires `policy` (a JSON string, opaque to this backend).
  `ResourcePolicy` (the response shape) is just `{policy, resourceArn}` --
  confirmed via `awsAwsjson11_deserializeDocumentResourcePolicy` in
  deserializers.go and the `ResourcePolicy` shape in service-2.json.
  `GetResourcePolicy`/`PutResourcePolicy` both envelope the object under
  `resourcePolicy` (NOT flat -- the CustomDomainAssociation flat-response
  oddity does not generalize to every serverless family, confirmed by reading
  `awsAwsjson11_deserializeOpDocumentGetResourcePolicyOutput` directly rather
  than assuming either convention). `DeleteResourcePolicyResponse` has zero
  members, same convention as `DeleteCustomDomainAssociationResponse`.
  Real errors are `GetResourcePolicy`/`DeleteResourcePolicy`:
  `ResourceNotFoundException` on a missing policy; `PutResourcePolicy` also
  lists `ConflictException`/`ServiceQuotaExceededException` in its error set,
  but the precondition that trips them is undocumented and not simulated
  (`PutResourcePolicy` is a create-or-replace upsert here, same as the real
  API's stated behavior).
- `SnapshotCopyConfiguration` mirrors `CreateSnapshotCopyConfigurationInput`'s
  required `namespaceName`/`destinationRegion` plus optional
  `destinationKmsKeyId`/`snapshotRetentionPeriod`, and an
  id/arn pair generated the same way every other serverless resource's is
  (`randomHex`/`arn.Build`). `UpdateSnapshotCopyConfigurationInput` only
  mutates `snapshotRetentionPeriod` (confirmed against
  `awsAwsjson11_serializeOpDocumentUpdateSnapshotCopyConfigurationInput`).
  `DeleteSnapshotCopyConfigurationResponse` DOES echo the deleted object under
  `snapshotCopyConfiguration` (`"required": ["snapshotCopyConfiguration"]` in
  service-2.json) -- the opposite convention from
  `DeleteResourcePolicy`/`DeleteCustomDomainAssociation`, worth flagging since
  it would be easy to assume Delete responses are uniformly empty across this
  service. `ListSnapshotCopyConfigurationsResponse` envelopes under the
  plural `snapshotCopyConfigurations`, with `namespaceName` as an optional
  server-side filter and `maxResults`/`nextToken` on the existing shared
  pagination convention.

Both proven with a persistence round trip in
`TestInMemoryBackend_FullStateRoundTrip`: verified the round trip actually
catches data loss (not just green-by-construction) by temporarily
de-registering both new `store.Table`s from `store_setup.go` in a scratch copy
and confirming the test fails with `no resource policy for ...`, then
restoring the real registration. Two new `store.Table`s (`slResourcePolicies`,
`slSnapshotCopyConfig`) plus one new `sortedStringIndex`
(`slSnapshotCopyConfigIdx`) registered/wired the same way every prior
serverless table is -- no snapshot version bump.

Deliberately not modeled: a one-configuration-per-namespace constraint for
`CreateSnapshotCopyConfiguration` -- service-2.json documents no such rule for
this family (unlike classic Redshift's `EnableSnapshotCopy`, which this same
backend does gate one-per-cluster), so none was invented.
`TooManyTagsException`/`ServiceQuotaExceededException`/`ConflictException`
preconditions remain unsimulated, consistent with this service's existing
`TooManyTagsException` precedent. EndpointAccess, RecoveryPoint,
TableRestoreStatus, ListManagedWorkgroups, and the restore-from-snapshot/
recovery-point ops remain unbuilt -- see items_still_open.

### 2026-08-09 pass: Redshift Serverless Tagging + CustomDomainAssociation (bd gopherstack-w8g2)

Split out of bd gopherstack-hsfm's "nine resource families do not exist" list
per that issue's own instruction to pick two at a time; these two were named
because Tagging is what creation-time `Tags` on the other Create ops defer to,
and CustomDomainAssociation is what `GetCredentials.CustomDomainName` depends
on. Built both, full account in the `Redshift Serverless` family row's
addendum above. Confirmed via the pinned `botocore`
`redshift-serverless/2021-04-21/service-2.json.gz` model (not
`aws-sdk-go-v2/service/redshiftserverless`, which stayed out of `go.mod` per
this issue's constraint -- fetched into `/tmp` only, diffed, discarded):

- `TagResourceRequest`/`ListTagsForResourceRequest`'s `tags` member is a
  `TagList` of `{key, value}` structs, not a JSON map -- a different shape
  than this package's classic-Redshift XML `Tags>Tag` convention.
- Only `CreateNamespaceRequest`/`CreateWorkgroupRequest`/`CreateSnapshotRequest`
  have a `tags` member; `CreateUsageLimitRequest`/`CreateScheduledActionRequest`
  do not, and neither `UpdateNamespaceRequest` nor `UpdateWorkgroupRequest` do.
- `Namespace`/`Workgroup`/`Snapshot` (the response shapes) have NO `tags`
  field at all -- confirmed by listing every member of each shape. Tags are
  therefore write-only through Create*/TagResource and read-only through
  ListTagsForResource; there is no "echo tags on the resource itself" wire
  gap to fix here, unlike the classic-Redshift `Cluster.Tags` bug this same
  file's 2026-07-22 pass found. Implemented as a new resourceArn-keyed
  `store.Table[slResourceTagSet]` (`serverless_tags.go`), not attached to any
  existing resource struct.
- `Create/Get/UpdateCustomDomainAssociationResponse` serialize the
  association's fields directly at the top level -- NOT wrapped in an
  envelope key the way every other serverless resource response is (e.g.
  `{"namespace": {...}}`). `DeleteCustomDomainAssociationResponse` has zero
  members. Both confirmed by reading the shapes directly, not assumed from
  the sibling ops' pattern.
- `Association.customDomainCertificateExpiryTime` uses shape
  `SyntheticTimestamp_date_time` (`timestampFormat: iso8601`), while
  `GetCredentialsResponse.expiration`/`nextRefreshTime` use the bare
  `Timestamp` shape (epoch-seconds JSON number, no explicit format) --
  genuinely different wire formats for two timestamp fields in the same
  service, both re-confirmed by reading the shape definitions rather than
  copying the nearer-looking convention.
- Real `Workgroup` carries `customDomainName`/`customDomainCertificateArn`/
  `customDomainCertificateExpiryTime` directly (added to the `Workgroup`
  struct), so `CreateCustomDomainAssociationSL`/`UpdateCustomDomainAssociationSL`/
  `DeleteCustomDomainAssociationSL` mirror the association onto its workgroup,
  not just into the separate association store.
- `GetCredentialsRequest.customDomainName` is a real, documented alternative
  to `workgroupName` ("The custom domain name or the workgroup name must be
  included in the request") -- `handleGetCredentials` now resolves it via the
  new association store before falling through to the existing
  workgroup-keyed credential logic.

Proven with a round trip at both layers: `TestServerless_TagResource_RoundTrip`
(HTTP-level Create-with-tags -> ListTagsForResource) and
`TestInMemoryBackend_FullStateRoundTrip` (Snapshot/Restore preserves both the
new tag store and a custom domain association). Two new `store.Table`s
(`slResourceTags`, `slCustomDomainsSL`) registered the same way every other
table in this package is -- no snapshot version bump, since `Registry.Tables`
is an additive `map[string]json.RawMessage` and `RestoreAll` already resets
any table absent from an older snapshot to empty.

Deliberately not modeled: `TooManyTagsException` (no tag-count cap enforced),
`AccessDeniedException` on the custom-domain ops (no IAM simulation in this
backend, consistent with the rest of this service). EndpointAccess,
ResourcePolicy, RecoveryPoint, SnapshotCopyConfiguration, TableRestoreStatus,
ListManagedWorkgroups, and the restore-from-snapshot/recovery-point ops were
not attempted this pass -- each still needs its own follow-up issue per
gopherstack-hsfm's original sizing note.

### 2026-08-08 pass: Redshift Serverless audit + wire-shape fixes (bd gopherstack-hsfm)

See the `Redshift Serverless` family row above for the full account. Short
version: the entire 25-op surface used invented REST routing that no real
`aws-sdk-go-v2` client could ever reach (real protocol is awsJson1.1: POST "/"
with an `X-Amz-Target` header, all fields in the body) -- fixed by switching to
the same `X-Amz-Target` dispatch pattern `redshiftdata.Handler` already uses.
Also fixed while rewriting: `ScheduledActionResponse`'s wire key is `state`,
not `status` (which doesn't exist on that type); `StartTime`/`EndTime` and
`GetCredentialsOutput.Expiration`/`NextRefreshTime` are epoch-seconds numbers,
not RFC3339 strings; `Schedule`/`TargetAction` are tagged-union JSON objects,
not flat strings; `CreateScheduledActionInput.RoleArn` (a required real field)
was entirely unmodeled. Several accepted-then-dropped fields threaded through
(`Namespace.DefaultIamRoleArn`, `DeleteNamespace`'s final-snapshot params,
`Workgroup`'s advanced-config fields, `List*`'s `MaxResults` which was
hardcoded to 0 on every call). SDK obtained via `go get
github.com/aws/aws-sdk-go-v2/service/redshiftserverless@v1.38.5` to field-diff
serializers.go/deserializers.go directly; `go mod tidy` dropped it again
afterward since nothing in the fix imports the SDK package at runtime (this
repo hand-rolls wire structs everywhere), so no persistent new go.mod
dependency was added. Whole resource families this backend has zero code for
(EndpointAccess, CustomDomainAssociation, ResourcePolicy, RecoveryPoint,
SnapshotCopyConfiguration, TableRestoreStatus, Tagging, ListManagedWorkgroups,
restore-from-snapshot) remain unbuilt -- each needs its own follow-up issue,
not a shared one, given the size this family already proved to be.

### 2026-08-08 pass: remaining-fields sweep (bd gopherstack-emho)

Modeled the 8 fields tracked by bd gopherstack-emho, each verified against
`aws-sdk-go-v2/service/redshift@v1.65.4`'s serializers.go/deserializers.go before
writing: `UsageLimit`/`SnapshotCopyGrant`/`HsmClientCertificate`/`HsmConfiguration`
`Tags` (all use the existing shared `Tags>Tag` wire convention via
`tagMapToKVList`/`parseRedshiftTags` -- no new tag mechanism added),
`IdcApplication.ApplicationType`, `ReservedNode.RecurringCharges` (derived from
the node's own real `UsagePrice`, not fabricated), `ScheduledAction.NextInvocations`
(a real `at()`/`cron()` evaluator, `schedule.go`, since `Schedule` already carries a
real expression), and `ClusterSubnetGroup.VpcId` (removed the fabricated
`CreateClusterSubnetGroupInput.VpcId` request param real clients never send; the
response field is left honestly empty absent EC2 cross-reference data). Also found
and fixed while verifying HSM: `CreateHsmConfiguration` read the request's IP
address as `HsmIPAddress`, but the real wire param is `HsmIpAddress` (case-different)
-- silently dropped for every real SDK client.

Deliberately left unmodeled, with reasoning recorded per-field: `IdcApplication.
ServiceIntegrations` (3-level-deep tagged union, disproportionate), `EndpointAccess.
VpcEndpoint` (no per-ENI AZ/IP/subnet data in this backend to derive it from).

Assessed the Redshift Serverless surface (`handler_serverless.go` + `serverless*.go`,
25 ops, JSON protocol) per this issue's instruction to size before starting: LARGE,
not started this pass. Picked up and audited in the 2026-08-08 gopherstack-hsfm
pass above -- see that section and the `Redshift Serverless` family row for what
was found and fixed.

Protocol: query/XML (`Version=2012-12-01`), same envelope convention as EC2 -- see
`redshiftXMLNS`/`marshalXML` in handler.go. Timestamps are wire-formatted as RFC3339
strings (`time.Now().UTC().Format(time.RFC3339)`), matching `smithytime.ParseDateTime`
used by the SDK's query-XML deserializer. Do not switch to epoch numbers for this
service -- that's a JSON-protocol convention used elsewhere (`pkgs/awstime.Epoch`),
not query-XML.

Real AWS error `ErrorCode()` strings are NOT consistent about a trailing "Fault"
suffix -- some fault types' `ErrorCode()` strip it (`ClusterNotFoundFault` ->
`"ClusterNotFound"`), others keep it (`HsmConfigurationNotFoundFault` ->
`"HsmConfigurationNotFoundFault"`), and some resource families use an entirely
different fault than their name would suggest (data share lookup failures use
`InvalidDataShareFault`, not a `DataShareNotFound`-shaped fault; a resource-policy
lookup failure uses the generic `ResourceNotFoundFault`). Every sentinel in
errors.go was individually checked against `aws-sdk-go-v2/service/redshift@v1.62.3/
types/errors.go`'s `ErrorCode()` bodies this pass -- do not "clean up" perceived
inconsistency in that file without re-checking the SDK source per-sentinel.
`resolveErrCode` (handler.go) now derives the wire `<Code>` directly from each
sentinel's own `.Error()` text via `errCodeSentinels` instead of a second duplicated
string table, specifically to prevent the two from silently drifting apart again
(that drift is exactly how the IdcApplication error-code bug happened).

### 2026-07-25 pass: Qev2IdcApplication (new SDK ops) + IdcApplication envelope gap found

The Go SDK modules were bumped (v1.62.3 -> v1.65.0), adding 4 new operations:
`CreateQev2IdcApplication`, `DescribeQev2IdcApplications`, `ModifyQev2IdcApplication`,
`DeleteQev2IdcApplication` -- the Query Editor V2 IAM Identity Center application family.
Implemented for real (routing, backend state in a new `qev2IdcApplications` `store.Table`,
request parsing, response wire shapes field-diffed against
`aws-sdk-go-v2/service/redshift@v1.65.0`'s `types.Qev2IdcApplication` and the
Create/Delete/Describe/Modify Input/Output shapes' own `serializers.go`/`deserializers.go`,
correct fault codes, Snapshot/Restore via the existing generic `store.Registry` machinery).
See `models.go`, `qev2_idc_applications.go`, `handler_qev2_idc_applications.go`, and the new
table cases in `handler_idc_applications_test.go`.

Confirmed `Qev2IdcApplication` is a resource **distinct from** `RedshiftIdcApplication`
(the family added in the 2026-07-22 pass), not a sub-resource of it: no shared ARN/ID space,
no cross-reference field in either direction, and `Qev2IdcApplication` has no `IamRoleArn` at
all (that field only exists on `RedshiftIdcApplication`, which uses it to invoke the IDC
Identity Center API for cluster-level federated auth; Query Editor V2's IdC application has no
equivalent need). Stored and routed entirely separately from the existing family.

While field-diffing the sibling `RedshiftIdcApplication` family closely enough to be sure the
two didn't need to share wiring, found that its Create/Modify response envelopes are missing a
nesting level the real deserializer requires (see `gaps` above and
`families.IdcApplication`) -- left unfixed as out of this pass's declared scope, tracked
instead of silently absorbed into the "ok" rating.

### 2026-07-25 follow-up: IdcApplication envelope gap fixed (bd gopherstack-0eyk)

Fixed the gap tracked above. Confirmed directly against
`aws-sdk-go-v2/service/redshift@v1.65.0/deserializers.go`:
`awsAwsquery_deserializeOpDocumentCreateRedshiftIdcApplicationOutput` and
`...ModifyRedshiftIdcApplicationOutput` both look for a `RedshiftIdcApplication` child element
inside the `...Result` element (`case strings.EqualFold("RedshiftIdcApplication", t.Name.Local)`).
`createIdcApplicationResponse.Result` and `modifyIdcApplicationResponse.Result` in
`handler_idc_applications.go` were tagged `xml:"CreateRedshiftIdcApplicationResult"` /
`xml:"ModifyRedshiftIdcApplicationResult"` with no inner element, so a real SDK client would
decode an empty struct for every Create/Modify call. Fixed both tags to
`...Result>RedshiftIdcApplication`, matching `createQev2IdcApplicationResponse`'s existing
correct `CreateQev2IdcApplicationResult>Qev2IdcApplication` pattern in the sibling file.

Also re-verified `DescribeRedshiftIdcApplicationsResult>RedshiftIdcApplications>member` against
`awsAwsquery_deserializeDocumentRedshiftIdcApplicationList` (list items unwrapped via `member`,
confirmed correct, no change) and `DeleteRedshiftIdcApplication` (real
`DeleteRedshiftIdcApplicationOutput` deserializer parses no body at all -- the handler's
response struct correctly carries no `Result` field, no change needed).

The prior audit missed this because the existing tests asserted only substring presence
(`wantContains: []string{"CreateRedshiftIdcApplicationResponse", "my-app"}`), which passes
whether or not the wrapper element exists. Strengthened `TestHandler_CreateIdcApplication`,
`TestHandler_ModifyIdcApplication`, and `TestHandler_DescribeIdcApplications`'s `success`/
`list_all` cases to assert the literal nested envelope string (e.g.
`<CreateRedshiftIdcApplicationResult><RedshiftIdcApplication>`), the same way the Qev2 sibling
tests already did -- a regression to the old flat shape now fails the table test directly.

### Bugs fixed this pass (2026-07-22)

This pass audited every family PARITY.md previously listed as `deferred:` (17) plus
the 2 `gaps:` items, field-diffing wire shapes against
`aws-sdk-go-v2/service/redshift@v1.62.3`'s serializers.go/deserializers.go/api_op_*.go
rather than trusting the absence of stub patterns. Full detail is in the `families`
table above; the highlights, roughly in order of severity:

1. **`IdcApplication` family was entirely unreachable by real clients.** The
   dispatch table registered handlers under `CreateIdcApplication` etc. instead of
   the real action names `CreateRedshiftIdcApplication` etc. — every real SDK call
   got `InvalidAction`. Also had swapped `IdcInstanceArn`/`IamRoleArn` XML tags
   (values transposed on the wire), wrong request param names, wrong response
   envelope names, and fabricated error codes. All fixed; see handler.go's
   `buildOpsGroup3` and handler_idc_applications.go.

2. **`Partner` family used a fabricated `PartnerIntegrationId` name everywhere**
   instead of the real `PartnerName` — every request/response field for
   AddPartner/DeletePartner/DescribePartners/UpdatePartnerStatus was affected. See
   handler_partners.go and partners.go.

3. **`ScheduledAction.TargetAction`** — the single field that determines what a
   scheduled action actually does — was parsed as a flat string and never
   serialized in any response at all. Rebuilt as the real nested
   `PauseCluster|ResumeCluster|ResizeCluster` tagged union with correct
   `TargetAction.ResizeCluster.ClusterIdentifier=...`-style nested request parsing
   and response serialization. See models.go, scheduled_actions.go,
   handler_scheduled_actions.go.

4. **`ModifyClusterSnapshotSchedule` was a real no-op past input validation** — it
   checked the cluster and schedule both existed and then did nothing, so the
   association was never recorded anywhere and could never be observed. Fixed to
   set/clear `Cluster.SnapshotScheduleIdentifier` (a real Cluster wire field this
   backend wasn't tracking at all) and derive `SnapshotSchedule.AssociatedClusters`
   live from it.

5. **`ResizeCluster` gap closed**: now populates `activeResizes` so
   `DescribeResize`/`CancelResize` can observe a resize triggered through the real
   API op (previously only the `AddActiveResizeInternal` test-seed helper could).

6. **`Cluster.Tags` was never embedded inline** on any Cluster-returning response
   (CreateCluster, DescribeClusters, ModifyCluster, ...) — real `Cluster.Tags
   []Tag` is a first-class field on the object itself, not just reachable via the
   separate `DescribeTags` API. Required a `toXMLCluster` -> `Handler` method
   conversion (to reach `DescribeTags`) plus a `toXMLClusterWithTags` split to
   avoid an O(n²) `DescribeTags` re-scan inside `handleDescribeClusters`'s loop.

7. **`EndpointAccess`/`Integration` used fabricated or mis-cased parameter names**
   (`VpcId` doesn't exist on `CreateEndpointAccessInput`/`ModifyEndpointAccessInput`
   — real fields are `SubnetGroupName`/`VpcSecurityGroupIds`; `CreateIntegration`'s
   KMS key param is `KMSKeyId`, not `KmsKeyId`, and its tags param is `TagList`, not
   `Tags`). Both rebuilt around the real wire shapes.

8. Smaller wire-completeness fixes: `DataShare.DataShareType`,
   `EventSubscription.SubscriptionCreationTime`, `TableRestoreStatus.
   SnapshotIdentifier` (previously discarded, not just unserialized) and
   `RequestTime`, and `ResourcePolicy`/`CustomDomainAssociation`'s fabricated error
   codes (`ResourcePolicyNotFound` -> `ResourceNotFoundFault`;
   `CustomDomainAssociationAlreadyExistsFault` -> `CustomCnameAssociationFault`).

Every fix above has a dedicated regression test (see handler_*_test.go files
touched this pass) asserting the corrected wire shape/behavior, not just that the
handler doesn't error.

### Bugs fixed in prior passes (kept for history)

1. `RestoreFromClusterSnapshot` nil `Tags` panic (snapshots.go) — every cluster
   value must have `Tags` initialized; `RestoreFromClusterSnapshot` omitted it,
   crashing `DescribeTags` the instant a snapshot-restored cluster existed.
2. `RestoreFromClusterSnapshot` cluster stuck in `"restoring"` forever — no
   lifecycle transition was scheduled to advance it to `"available"`.
3. `ModifyCluster` `Encrypted`/`EnhancedVpcRouting` could never be turned off —
   both are `*bool` on the real SDK; a plain `bool` couldn't distinguish
   "unspecified" from "explicitly false".
4. `GetClusterCredentials` dropped `Expiration` — computed but never serialized.

### Traps for the next auditor

- `resolveErrCode`'s `errCodeSentinels` table derives the wire `<Code>` from each
  sentinel's own `.Error()` text (see Notes above on the Fault-suffix
  inconsistency). If you add a new sentinel, verify its exact `ErrorCode()` string
  against `aws-sdk-go-v2/service/redshift@v1.62.3/types/errors.go` individually —
  do not assume the pattern from a neighboring sentinel.
- `ScheduledAction.TargetAction`'s `NextInvocations`/`StartTime`/`EndTime` are
  intentionally NOT modeled (empty list / never set) — this backend is
  synchronous/instant-apply and has no cron/at-expression evaluator to compute
  real next-invocation times. An empty `NextInvocations` list is valid per the AWS
  docs (not "must always have up to 5 entries"), so this is a deliberate scope
  bound, not a bug.
- `EndpointAccess.VpcEndpoint` (the nested network-interface/address list) is
  intentionally NOT modeled — would require simulating ENI allocation per subnet,
  out of proportion to this backend's fidelity level elsewhere.
- `ClusterSubnetGroup`'s `CreateClusterSubnetGroupInput` accepting a `VpcId`
  parameter is a PRE-EXISTING fabrication (not touched this pass, not part of the
  audited family list) — the real SDK has no such field (VPC is derived from the
  subnets). Left alone to avoid uncontrolled scope creep into a family this pass
  didn't own; flag for the next audit if `ClusterSubnetGroup` is revisited.
- `ResizeCluster`'s `AllowCancelResize` is always `false` immediately after a
  resize (since this backend applies resizes instantly/synchronously) — a
  `CancelResize` call right after `ResizeCluster` will correctly get
  `InvalidClusterState`, not `ResizeNotFound`. This is intentional, matching real
  AWS's behavior once a resize has actually completed, not a bug.
- The `ApplyImmediately` parameter on `ModifyCluster` is NOT part of the real
  `ModifyClusterInput` wire shape — confirmed again this pass, still intentional
  and covered by its own test (`TestParity_ModifyCluster_ApplyImmediately`). Do
  not remove it.
- `RebootCluster` flips status to `"rebooting"` then immediately back to
  `"available"` within the same call — consistent instant-apply simplification,
  not a bug.
- `DeleteClusterParameterGroup`/similar delete ops still do not special-case
  AWS's `default.*` parameter group protection. Not touched this pass (out of the
  audited family list) — candidate for the next audit if `ClusterParameterGroup`
  is revisited.

### items_still_open (genuinely deferred, NOT reclassified as ok on a no-stub basis)

These are real, identified wire-completeness gaps within families that are
otherwise correctly wired (routing/params/errors/state all verified real) — kept
open rather than silently fixed because each would require non-trivial new
modeling (nested nested nested types, nested list-of-object shapes, nested
nested response subtrees) disproportionate to the traffic these fields see:

- `IdcApplication`: `AuthorizedTokenIssuerList`, `ServiceIntegrations` (3-level
  nested tagged union -- see families.IdcApplication), `SsoTagKeys`,
  `IdcManagedApplicationArn`, `IdcOnboardStatus`, `IdentityNamespace` not
  modeled. (`ApplicationType` FIXED 2026-08-08, see families.IdcApplication.)
- `ReservedNode`: `ReservedNodeOfferingType` not modeled.
  (`RecurringCharges` FIXED 2026-08-08, see families.ReservedNode.)
- `TableRestoreStatus`: `SourceSchemaName`, `TargetSchemaName`,
  `ProgressInMegaBytes`, `TotalDataInMegaBytes`, `EnableCaseSensitiveIdentifier`
  not modeled (this backend's restores complete instantly, so Progress/Total are
  always 0 in practice even if added).
- `EndpointAccess`: `VpcEndpoint` (nested network-interface list) not modeled --
  no per-ENI AZ/IP/subnet data exists in this backend to derive it honestly from
  (see families.EndpointAccess).
- `ScheduledAction`: `StartTime`/`EndTime` not modeled (`NextInvocations` FIXED
  2026-08-08, see families.ScheduledAction).
- `ClusterSubnetGroup`/`EndpointAccess` `VpcId`: honestly empty by default --
  this backend has no EC2 cross-reference to derive a real VPC from subnet IDs
  (see families.ClusterSubnetGroup, FIXED 2026-08-08: the fabricated
  CreateClusterSubnetGroupInput.VpcId request param that used to seed this is
  now removed).
- Descriptive/static ops family: RE-AUDITED 2026-08-13 (gopherstack-3jqz) --
  the "spot-checked, no-stub, real derivation confirmed" claim previously
  here was false for two ops (see families.Descriptive/static ops for the
  full account): `ModifyAquaConfiguration` and `ModifyLakehouseConfiguration`
  both ignore their required `ClusterIdentifier` entirely, with no
  existence/state validation and (for ModifyLakehouseConfiguration) no
  modeled response state at all. Not fixed this pass (out of scope) --
  genuinely open, not reclassified. `ListRecommendations`/
  `GetIdentityCenterAuthToken` re-confirmed real. The remaining
  Describe*/static-catalog ops are still not exhaustively field-diffed
  element-by-element (filters/pagination params like Marker/MaxRecords/
  ClusterVersion/NodeType are accepted-and-ignored on several of them, not
  yet audited for severity).
- Redshift Serverless (`handler_serverless.go`): separate JSON-protocol API
  surface (`redshift-serverless` service ID). AUDITED 2026-08-08 (bd
  gopherstack-hsfm, see the family row and Notes section above) -- routing and
  several field-level wire bugs fixed. Tagging and CustomDomainAssociation
  FIXED 2026-08-09, ResourcePolicy and SnapshotCopyConfiguration FIXED
  2026-08-10 (bd gopherstack-w8g2, see the family row's addenda) --
  `GetCredentials.CustomDomainName` lookup is no longer open. RecoveryPoint,
  TableRestoreStatus, `RestoreFromRecoveryPoint`, `RestoreTableFromSnapshot`
  and `RestoreTableFromRecoveryPoint` FIXED 2026-08-10 (bd gopherstack-w8g2,
  the entangled group -- see the family row's addendum). Still open within
  the audited ops: `Namespace.AdminUserPassword`/`RedshiftIdcApplicationArn`,
  `Snapshot.backup-progress-and-size` fields/cross-account restore-access
  lists, `ScheduledActionResponse.NextInvocations` (needs schedule.go's cron
  evaluator adapted to serverless's unwrapped cron string format),
  `ListRecoveryPointsRequest`'s `startTime`/`endTime` filter window (not
  parsed at all) -- each independently verified as either legitimately
  un-derivable by this backend or deferred to an excluded resource family
  (see the family row for per-field reasoning). EndpointAccess,
  ListManagedWorkgroups, `RestoreFromSnapshot` and
  `ConvertRecoveryPointToSnapshot` FIXED 2026-08-10 (bd gopherstack-w8g2,
  final pass -- see the family row's addendum). This closes gopherstack-w8g2:
  all nine originally-missing serverless families now have real code.
  `EndpointAccess.VpcEndpoint` (nested `vpcEndpointId`/`vpcId`/
  `networkInterfaces`) remains unmodeled -- the same no-per-ENI-AZ/IP-data
  judgment call `families.EndpointAccess` already made for classic Redshift,
  confirmed to apply identically here (see the family row's addendum).
  `UpdateSnapshot`, `GetTrack`/`ListTracks`, `UpdateLakehouseConfiguration`
  and `GetIdentityCenterAuthToken` FIXED 2026-08-13 (bd gopherstack-v4wu, see
  the family row's addendum below) -- the reservation-capacity family
  (`CreateReservation`/`GetReservation`/`GetReservationOffering`/
  `ListReservationOfferings`/`ListReservations`) is DELIBERATELY DEFERRED,
  not fixed: `ReservationOffering` carries AWS-set commercial pricing
  (`HourlyCharge`/`UpfrontCharge`/`CurrencyCode`) with no fixed, SDK-enumerable
  catalog to model honestly against (unlike classic Redshift's own
  `ReservedNode`, whose offerings key off a small, real, AWS-documented
  node-type catalog -- see `families.ReservedNode` above), and this family has
  zero pre-existing backend state (no `CreateReservation` call has ever run
  against this backend) to hang a reservation's identity on. Still tracked in
  `sdk_completeness_test.go`'s `notImplemented` slice.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed InternalFailure**: same shape as autoscaling's entry
(see that entry or gopherstack-3a8t for the full survey/rationale). `RouteMatcher` now
falls back to `service.MatchesUserAgentMarker(r.Header, "api/redshift")` (verified against
the pinned `redshift@v1.65.4/api_client.go:637` `AddSDKAgentKeyValue` call) only on the
`ReadBody` failure branch. Migrated `ExtractOperation`/`ExtractResource`/`Handler()` off
`r.ParseForm()` onto `httputils.ReadBody`+`url.ParseQuery`, per the docdb/neptune precedent
(gopherstack-bahs). Proof: `TestHandler_OversizedBodySurfacesInternalFailure` in
`handler_oversized_body_test.go` drives a real Redshift SDK client through
`service.NewRegistry`/`service.NewServiceRouter`, confirmed failing pre-fix with
`UnknownError`; passes now with `InternalFailure`. `TestHandler_NormalSizedBodyStillRoutes`
is the regression guard. Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race
./services/redshift/...` (pass), `golangci-lint run ./services/redshift/...` (0 issues).

**2026-08-23 -- empty-body sweep and `gaps:` re-check, no fixes found (gopherstack-jf8z)**:
audited redshift for the same two bug classes checked in cloudfront the same pass. Class 1
(empty body where the real Output requires a document): `grep -n "return nil, nil$"` and the
broader `"return nil,"`/`"nil, nil"` scans across every `handler_*.go` found nothing -- every
`redshiftActionFn` (`func(vals url.Values) (any, error)`) returns a concrete typed wrapper
struct on the success path, even for void ops (e.g. `handleDeleteTags` still returns
`&response{Xmlns: redshiftXMLNS}` with a real `DeleteTagsResponse` root, never a bare nil
through `writeXMLResponse`'s `xml.Marshal`). No `c.NoContent`/direct-write bypass of the
dispatch table exists in this service (Query protocol, single `dispatch` -> `writeXMLResponse`
path, confirmed via the `awsAwsquery_` deserializer prefix in `redshift@v1.65.4`). Class 2:
this file's `gaps:` front-matter is `[]` and has been for the prior several passes -- zero live
entries to re-check against the "does the state already exist in the backend" test. No code
changes this pass. Gates: `go build ./services/redshift/...` clean; `go test
./services/redshift/... -count=1` -- `ok github.com/blackbirdworks/gopherstack/services/redshift
0.328s`.

## 2026-08-29 enum-VALUE sweep (wrapper-key-sweep campaign, wire-shape enforcement all services)

Targeted pattern hunt for the comprehend class of bug: a status/state value assigned to a
domain struct field that is not a member of the real AWS enum for the corresponding response
member, reaching the wire through the field rather than a same-site literal `cmd/enumcheck` can
resolve. Redshift's older query/XML API leaves most status-like fields (`Cluster.ClusterStatus`,
`ClusterAvailabilityStatus`, `AvailabilityZoneRelocationStatus`, `IPRange.Status`,
`EC2SecurityGroup.Status`, `ReservedNode.State`, `DomainConfigurationStatus`-adjacent fields,
`Cluster.LakehouseRegistrationStatus`) as untyped `*string` on the real SDK with no documented
enum at all — those are out of scope by definition (no enum to violate) and were confirmed
untyped, not assumed. Every field that IS a real typed enum (`redshift@v1.65.4 types/enums.go`:
`AquaConfigurationStatus`, `AquaStatus`, `AuthorizationStatus`, `DataShareStatus`,
`DataShareStatusForConsumer`/`ForProducer`, `NamespaceRegistrationStatus`,
`PartnerIntegrationStatus`, `ScheduledActionState`, `ScheduleState`, `ZeroETLIntegrationStatus`,
`TableRestoreStatusType`, `ReservedNodeExchangeStatusType`, `LakehouseRegistration`,
`LakehouseIdcRegistration`) was traced through every assignment. `cmd/enumcheck` was run both
before and after and flagged **none** of the finding below.

**Found and fixed**: `reserved_nodes.go` `DescribeReservedNodeExchangeStatus` returned
`partnerStatusActive` ("Active") — a constant borrowed from the unrelated
`PartnerIntegrationStatus` enum — for `ReservedNodeExchangeStatus.Status`, whose real member is
`types.ReservedNodeExchangeStatusType` (REQUESTED/PENDING/IN_PROGRESS/RETRYING/SUCCEEDED/FAILED,
`types/enums.go:468`), which has no `"Active"` member at all. Fixed to a new
`reservedNodeExchangeStatusSucceeded = "SUCCEEDED"` constant, scoped to this field rather than
reusing another family's constant for its string value — this backend has no real exchange-
request pipeline to simulate, so the immediate-terminal value is the honest choice, matching this
service's own `slTableRestoreStatusSucceeded`/reconciler precedent for the same "no async
pipeline" pattern. A pre-existing test
(`TestRedshiftHandler_DescribeReservedNodeExchangeStatus`'s `"success"` case) asserted the raw
XML body contained `"Active"` — updated to assert `"SUCCEEDED"` instead, per this campaign's "do
not trust existing tests" rule.

**Checked clean** (N-of-N legal-value coverage against the real enum, no fix needed):
`AquaConfigurationStatus`/`AquaStatus` (both `"disabled"`, documented permanently-retired field),
`AuthorizationStatus` (2/2: Authorized/Revoking), `DataShareStatus` (3/6: ACTIVE/AUTHORIZED/
DEAUTHORIZED/REJECTED used across `DataShare`/`DataShareAssociation`), `NamespaceRegistrationStatus`
(2/2), `PartnerIntegrationStatus` (1/4: Active), `ScheduledActionState` (2/2), `ScheduleState`
(1/3: ACTIVE, reused via the misleadingly-named `dataShareStatusActive` constant for both
`ScheduleAssociationState` and `SnapshotScheduleState` — same string value is coincidentally
legal for both `DataShareStatus` and `ScheduleState`, so not a value bug, but noted as a naming
smell worth a follow-up rename), `ZeroETLIntegrationStatus` (1/7: active), `TableRestoreStatusType`
(2/5: IN_PROGRESS/SUCCEEDED for the classic-cluster path; `ServerlessTableRestoreStatus` jumps
straight to SUCCEEDED, same no-async-pipeline convention). `DataShareStatusForConsumer`/
`ForProducer` are real typed enums but only ever appear as client-supplied *input* filter
parameters on `DescribeDataSharesForConsumer`/`ForProducer` (passthrough, not backend-assigned)
— out of scope for this pass, not a fabrication risk since a real typed SDK client can only send
a legal member.

Also confirmed, not a bug: `lakehouseStatusRegistered`/`lakehouseStatusDeregistered`
("Registered"/"Deregistered", `lakehouse.go`) back `Cluster.LakehouseRegistrationStatus`, which
is untyped `*string` on the real SDK (no enum exists) — already documented in this file's own
header comment as a deliberate, honest derivation from the client's real
`types.LakehouseRegistration` request value, re-confirmed correct this pass, not re-touched.

Gates: `go build ./services/redshift/...` (clean), `go vet ./...` (repo-wide, clean — no
signature changes this pass), `go test -race -count=1 ./services/redshift/...` (pass, including
new `wire_field_fixes_test.go` and the one corrected pre-existing test, each new/changed
assertion hand-verified to fail against the pre-fix literal then restored),
`golangci-lint run --fix ./services/redshift/...` (0 issues). Work left uncommitted per this
pass's instructions.

## 2026-08-29 error-path sweep (wrong-code bug hunt, no fix needed)

Cross-referenced the `errCodeSentinels`/`resolveErrCode` table (`handler.go`) and a sample of
call sites (crawlers-equivalent multi-code ops: `DeleteCluster`, `CreateClusterSnapshot`,
`RevokeEndpointAccess`, `DescribeReservedNodeExchangeStatus`) against each op's own
`awsAwsquery_deserializeOpError<Op>` switch (redshift@v1.65.4 deserializers.go, all 145 ops
extracted). Found no wrong-sentinel bugs: every checked sentinel's wire code appears in the
modeled set of every op that raises it. This service's `errors.go` already carries extensive
per-op SDK-verified citations from a prior pass (e.g. `ErrSnapshotAccessNotFound`,
`ErrSecurityGroupIngressNotFound`, `ErrNamespaceRegistrationInvalidClusterState` all cite their
specific `deserializeOpError<Op>` switch by name), and that prior work held up under
re-verification. `DescribeReservedNodeExchangeStatus` models both `ReservedNodeNotFound` and a
second, AWS-side-misspelled `ReservedNodeExchangeNotFond` code the SDK also recognizes; this
backend only implements the first condition (reserved node doesn't exist) — a coverage gap
(no exchange-status-not-found case exists in this backend at all), not a wrong-code bug, so left
unfixed per this pass's scope.

Only change: `errors.go`'s header comment cited SDK version v1.62.3 (stale — go.mod pins
v1.65.4); re-verified every code string against v1.65.4's `types/errors.go` (unchanged) and
updated the comment to the correct version. No behavior change.

Gates: `go build ./services/redshift/...`, `go vet ./...` (repo-wide), `go test -race -count=1
./services/redshift/...`, `golangci-lint run --fix ./services/redshift/...` — all clean, no
regressions (expected, since no runtime code changed).

## 2026-08-29 indexed-list wire-key sweep (rds `Values.Value`/neptune `EventCategory` bug family)

Enumerated every hand-parsed indexed-list query key in this service -- every `vals.Get(fmt.Sprintf(...))`
call site plus every `parseStringList`/`parseTagListPrefixed`/`parseParameterList` caller (19 sites) --
and resolved each against its own operation's `awsAwsquery_serializeOpDocument<Op>Input` in the pinned
redshift@v1.65.4 SDK, following up the wrapper-list serializer it calls to the actual `value.Array("...")`
element name. 19-of-19 resolved (17 by direct serializer read, 2 by hand-tracing a two-level nested
serializer). Two real bugs found, both real client's-eye-view zeros regardless of what the backend stored:

1. **`nodeConfigFilterValue` (`handler_advisor.go`), wrong key entirely.** Looked for
   `<prefix>.Values.<M>` after matching a filter's `.Name` key. The real wire shape (serializers.go:
   `awsAwsquery_serializeOpDocumentDescribeNodeConfigurationOptionsInput` wraps `Filters` under object key
   `"Filter"`, `awsAwsquery_serializeDocumentNodeConfigurationOptionsFilterList` names each element
   `NodeConfigurationOptionsFilter`, and `awsAwsquery_serializeDocumentNodeConfigurationOptionsFilter`
   wraps `Values` under singular object key `"Value"`, itself an `array("item")` per
   `awsAwsquery_serializeDocumentValueStringList`) puts every value at
   `Filter.NodeConfigurationOptionsFilter.N.Value.item.M` -- plural "Values" never appears on the wire at
   all. A real client's `NodeType`/`NumberOfNodes`/`Mode` filters on `DescribeNodeConfigurationOptions`
   were silently ignored entirely (fell through to the full unfiltered option set), same bug class as
   rds's `Values.Value`/neptune's `EventCategory`, just a different wrapper depth. Fixed the prefix match
   to `.Value.item.`.
2. **`parseStringList(vals, "ScheduleDefinitions.ScheduleDefinition")` (`handler_snapshot_schedules.go`,
   both `CreateSnapshotSchedule` and `ModifySnapshotSchedule`), missing separator, not a wrong element
   name.** `awsAwsquery_serializeDocumentScheduleDefinitionList` confirms the element name itself
   (`ScheduleDefinition`) was already right, but the prefix argument was missing its trailing `.`, so the
   handler looked for `ScheduleDefinitions.ScheduleDefinition1` while a real client always sends
   `ScheduleDefinitions.ScheduleDefinition.1` -- schedule definitions were silently dropped on every
   Create/Modify regardless of what a client sent. Fixed both call sites to `"ScheduleDefinitions.ScheduleDefinition."`.

**Confirmed NOT present in this service**: the rds `Values.Value`/neptune `EventCategory` bugs
themselves don't recur verbatim -- `parseDescribeFilters`-equivalent generic `Filters.Filter.N.Values.*`
parsing doesn't exist here (redshift's only generic filter surface is the advisor one above, fixed);
`EventCategories.EventCategory.N`/`SourceIds.SourceId.N` (`handler_events.go`, `CreateEventSubscription`/
`ModifyEventSubscription`) already read the correct element names, cross-checked against
`awsAwsquery_serializeDocumentEventCategoriesList`/`awsAwsquery_serializeDocumentSourceIdsList`. No list
truncated to its first element (checked every loop terminates on first empty index, not a fixed `.1`/`[0]`
read). No Create/Modify divergence found among the 19 resolved sites (each list-accepting param used
consistently across its Create/Modify pair, where both exist). One structural gap noted but not fixed
(out of this class's scope, filed for awareness only): `handleCreateCluster` (`handler.go`) only ever
reads 5 of `CreateClusterInput`'s fields (`ClusterIdentifier`/`NodeType`/`DBName`/`MasterUsername`/
`MasterUserPassword`) -- `IamRoles`/`VpcSecurityGroupIds`/`ClusterSubnetGroupName`/etc. are silently
ignored at creation time even though `ModifyClusterIamRoles` and friends manage the equivalent state
post-creation. This is a missing-feature gap, not a wrong-key bug -- no wire key is misread, the keys are
simply never looked at.

Two new SDK-driven tests added to `handler_sdk_roundtrip_test.go`
(`TestDescribeNodeConfigurationOptions_FilterWireKey`, `TestCreateSnapshotSchedule_ScheduleDefinitionsWireKey`),
both confirmed failing against the pre-fix code (asserted defaults/empty results) before the fix, passing after.

Gates: `go build ./services/redshift/...`, `go vet ./services/redshift/...` and `go vet ./...` (repo-wide,
clean -- no signature changes), `go test -race -count=1 ./services/redshift/...` (pass), `golangci-lint run
./services/redshift/...` (0 issues, ran plain after an initial `paralleltest`/`tparallel` finding on the
new test's subtests, fixed by adding the missing `t.Parallel()` calls, re-ran clean).

## 2026-08-29 ordering-bug sweep (paginate-before-filter, iam class)

Audited every filtered-and-paginated operation for order of operations (filter-then-paginate is
correct; paginate-then-filter silently shorts the page and can be missed entirely past the cursor).
Found and fixed one real instance in classic `DescribeClusters`; the entire serverless List family (11
ops) plus `DescribeClusterSnapshots`/`DescribeQev2IdcApplications` were already correct. All other
classic `Describe*` ops implement no pagination at all in either handler or backend (confirmed by
grepping every backend `Describe*` signature for a marker/token parameter), so there is no cursor to
get the order wrong.

1. **`DescribeClusters` (`handler.go`/`store.go`), paginate-then-filter, plus wrong param names and
   wrong match semantics.** The handler read singular `TagKey`/`TagValue` query params, applied them as
   an AND filter to the *page* `Backend.DescribeClusters` had already cut by `Marker`/`MaxRecords`, and
   discarded the singular strings supplied by no real client. Real `DescribeClustersInput` (redshift@
   v1.65.4 `api_op_DescribeClusters.go`) has `TagKeys`/`TagValues []string`, wire-encoded as
   `TagKeys.TagKey.N`/`TagValues.TagValue.N` (`serializers.go:12572`,
   `awsAwsquery_serializeDocumentTagKeyList`), matched as "any tag whose key is in TagKeys OR whose
   value is in TagValues" per the operation doc comment -- not an AND of one key/value pair. Moved the
   tag filter into `InMemoryBackend.DescribeClusters` (new `tagKeys, tagValues []string` params),
   applied to the full snapshot before the `Marker` cut/`MaxRecords` slice, and added
   `clusterMatchesTagKeysOrValues` implementing the real any-key-or-value semantics via `Tags.Range`.
   Marker/nextMarker were already computed correctly independent of the tag filter (unlike the iam bug,
   a client that kept following `Marker` to empty would eventually see every match, just via
   short/uneven pages) -- so this was the "page comes back short" half of the class, not the
   "truncation lies" half. `handler_cluster_mgmt.go`/`handler_advisor.go`'s id-only lookups pass
   `nil, nil` (unaffected, since a non-empty `ClusterIdentifier` bypasses tag filtering and pagination
   entirely). The prior `TestDescribeClusters_TagFilter` (`handler_cluster_test.go`) hand-built form
   posts with the wrong singular param names and asserted the bug's own output as correct; replaced
   with SDK-driven `TestDescribeClusters_TagKeysFilter` and
   `TestDescribeClusters_TagKeysFilter_PaginationOrdering` (`handler_cluster_tagkeys_test.go`), the
   latter creating more tag-matching clusters than fit in one page and asserting the full match set is
   reachable by following `Marker`. Both confirmed failing against the pre-fix handler (wrong clusters
   returned / filter engaging as a no-op) before the fix.

**Clean, verified**: all 11 `services/redshift/serverless_*.go` `List*` backends (`ListRecoveryPointsSL`,
`ListServerlessUsageLimits`, `ListNamespaces`, `ListServerlessSnapshots`, `ListEndpointAccessSL`,
`ListWorkgroups`, `ListServerlessTracks`, `ListSnapshotCopyConfigurationsSL`,
`ListCustomDomainAssociationsSL`, `ListTableRestoreStatusSL`, `ListServerlessScheduledActions`) filter
the full index before slicing by `nextToken`/`maxResults`, and their handlers pass request fields
straight through with no post-backend re-filtering. `DescribeClusterSnapshots` (`handler_snapshots.go`)
filters fully in the backend, then paginates the filtered slice in the handler via a base64 marker --
correct. `DescribeQev2IdcApplications` has only a single-ID lookup, no combinable filter.

**Gaps noted, not fixed** (no pagination implemented at all, so no ordering bug is possible; each is a
structural/never-plumbed gap, judged unobservable for a typically small collection per operation, left
for a future "never-plumbed pagination" pass rather than folded into this one): `DescribeEvents`,
`DescribeReservedNodeOfferings`, `DescribeDataShares*`, `DescribeEndpointAccess`,
`DescribeEndpointAuthorization`, `DescribeClusterParameterGroups`/`Parameters`, and others enumerated
above under classic `Describe*` -- none read `Marker`/`MaxRecords` from the request at all.

New test file: `handler_cluster_tagkeys_test.go` (SDK-driven, real `redshiftsdk.Client`, per this
service's `newTestRedshiftClient` harness -- required here since the bug included wrong wire-key
binding, not just handler logic over an already-correct value).

Gates: `go build ./services/redshift/...`, `go vet ./...` (repo-wide, `DescribeClusters` signature
changed), `go test -race -count=1 ./services/redshift/...` (pass), `golangci-lint run
./services/redshift/...` (0 issues after fixing a `govet` shadow and an `nlreturn` finding).

**2026-08-30 (negative-continuation-token sweep)**: Redshift Serverless's 11 `List*` ops
(`serverless_namespaces.go`, `serverless_table_restore.go`, `serverless_workgroups.go`,
`serverless_snapshots.go`, `serverless_recovery.go`, `serverless_snapshot_copy_config.go`,
`serverless_endpoint_access.go`, `serverless_usage_limits.go`, `serverless_custom_domains.go`,
`serverless_tracks.go`, `serverless_scheduled_actions.go`) each copy-pasted an identical
inline `nextToken` decode with a bare `strconv.Atoi` and no negative check; each caller's
`startIdx >= len(list)` guard does not catch a negative `startIdx`, so `list[startIdx:end]`
panicked given `nextToken="-5"`. No shared decode function existed to fix in one place — this
was 11 duplicated inline blocks, not one helper — so this pass extracted a new shared
`decodeServerlessPageToken` (`serverless.go`, next to the existing `serverlessDefaultPageSize`
helper) and replaced all 11 inline blocks with a single call, consolidating what should always
have been one decode site.

Proof: `TestServerlessNamespaceIndex_NegativeToken` (`serverless_index_test.go`) confirmed
panicking pre-fix, passes now (the fix in `serverless.go` covers all 11 ops via one function,
so this single reproduction stands for the class). Gates: `go build ./services/redshift/...`,
`go vet ./services/redshift/...`, `go test -race -count=1 ./services/redshift/...`,
`golangci-lint run ./services/redshift/...` (0 issues). Work left uncommitted per this pass's
instructions.

**2026-08-30 (unstable-pagination-order sweep, wrapper-key-sweep branch)**: `DescribeClusterSnapshots`
(`snapshots.go`) built its unfiltered result from `b.snapshots.All()` -- an unspecified-order map
walk (`pkgs/store`'s `Table.All` doc) -- with no sort at all before `handleDescribeClusterSnapshots`
(`handler_snapshots.go`) applied its `Marker`-based pagination. Two calls could observe different
underlying orders, so a client paging with `MaxRecords` smaller than the snapshot count could drop
or duplicate a snapshot at a page boundary even though `SnapshotIdentifier` (the marker value, and
the table's own key) is itself unique -- the same shape the campaign brief documents for 3 elbv2
listings resumed by a unique listener ARN and 3 ssoadmin listings resumed by a unique request id.
Fixed by reading via `b.snapshots.Snapshot()` instead of `.All()` -- `Snapshot()` sorts by the
table's own key (`SnapshotIdentifier`) ascending, deterministically, matching the existing
`DescribeClusters` pattern this same file already uses for the same reason.

Every other `Describe*`/`List*` site in this service was audited: the 40+ non-serverless `Describe*`
ops (`custom_domains.go`, `endpoint_access.go`, `events.go`, `auth_profiles.go`, `data_shares.go`,
`hsm.go`, `param_groups.go`, and the rest) accept no `Marker`/`MaxRecords` at all -- they always
return the full set in one response, so there is no page boundary for this bug class to hit (a
separate, pre-existing gap: these ops ignore `Marker`/`MaxRecords` entirely, not newly introduced or
touched this pass). `DescribeClusters` and `DescribeQev2IdcApplications` already page via
`.Snapshot()`/sort-by-table-key and were confirmed safe, unchanged. All 11 Redshift Serverless
`List*` ops page via the pre-sorted `sortedStringIndex` (`serverless_index.go`) keyed by each
resource's own unique name -- confirmed safe, unchanged.

Proof: `TestDescribeClusterSnapshots_PaginationOrderIsReproducible`
(`handler_snapshots_test.go`) creates 130 same-cluster snapshots, walks them with `MaxRecords=25`
across `Marker`-resumed pages, and asserts the concatenation reproduces the set exactly with no
drops/duplicates, looped 30 times; failed on the first iteration against the unfixed code (some
snapshots missing entirely, others double-counted), passes after the `.Snapshot()` fix. Existing
`TestDescribeClusterSnapshots_Pagination` subtests never exercised a real multi-page walk (every
snapshot count used fits in one `MaxRecords=20` page), so they could not have caught this.

Gates: `go build ./services/redshift/...`, `go vet ./services/redshift/...`,
`go test -race -count=1 ./services/redshift/...` (pass), `golangci-lint run ./services/redshift/...`
(0 issues). Work left uncommitted per this pass's instructions.

## 2026-08-30 wire-key-read sweep, continued (remaining Describe/List operations)

Completed the wire-key-read sweep across all 43 Describe/List operations (derived from
`handler.go`'s dispatch-table registrations, not this file's prose). The prior pass on this
branch covered 13 (7 fixed bugs: Tags, ClusterSnapshots, ScheduledActions, UsageLimits,
HsmClientCertificates, HsmConfigurations, EndpointAccess; 6 confirmed-correct: ClusterParameterGroups,
ClusterSubnetGroups, ClusterSecurityGroups, Events, DescribeClusters, ReservedNodeExchangeStatus
enum). This pass audited the remaining 30 and found 8 more real bugs, all the same "declared field
never read" shape:

- `DescribeClusterParameters`: `Source` (real values `engine-default`/`user`, `param_groups.go`'s
  `ClusterParameter.Source`) was declared and never read -- every request returned every parameter
  regardless of `Source`. Fixed (`handler_param_groups.go`).
- `DescribeEventCategories`: `SourceType` (5 legal values, 4 modeled in this backend's static
  catalog) was declared and never read -- `_ url.Values` ignored the whole request. Fixed
  (`handler_events.go`).
- `DescribeCustomDomainAssociations`: `CustomDomainCertificateArn` was declared and never read,
  even though it's real backend data already echoed in every response. Fixed
  (`handler_custom_domains.go`). NOTE: the response shape itself remains the pre-existing,
  separately-scoped gap already documented under `families.CustomDomainAssociation` above (real
  `Association` groups by certificate via `CertificateAssociations`, this backend emits a flat
  per-domain list) -- not touched, out of scope for a filter fix.
- `DescribeInboundIntegrations`: full no-stub violation, not just a dropped filter -- `_
  url.Values` ignored the request AND the handler never consulted the integrations store at all,
  always returning empty regardless of real `Integration` data (every integration this backend can
  create already has a real `TargetArn`, i.e. it always targets something in Redshift). Fixed by
  filtering the same store `DescribeIntegrations` reads, keyed on `IntegrationArn`/`TargetArn`
  (`handler_integrations.go`). Response reshaped into a dedicated `inboundIntegrationXML` (CreateTime/
  IntegrationArn/SourceArn/Status/TargetArn only, confirmed against `types.InboundIntegration`,
  types/types.go:1160) instead of reusing `integrationXML`, which carries fields
  (IntegrationName/Description/KMSKeyId/Tags) not on `InboundIntegration`'s real wire shape at all.
- `DescribeIntegrations`: `Filters` (real enum `integration-arn`/`source-arn`/`source-types`,
  `DescribeIntegrationsFilterName`, types/enums.go:194) was declared and never read. Fixed
  `integration-arn` and `source-arn` (both exact-match against real, already-stored `Integration`
  fields); `source-types` deliberately left unenforced -- it classifies `SourceArn` by AWS resource
  type (e.g. "rds", "aurora-mysql"), data this backend does not derive from the stored ARN string,
  so implementing it would fabricate a classification rather than read real data.
- `DescribeSnapshotCopyGrants`: `TagKeys`/`TagValues` were declared and never read, even though
  `SnapshotCopyGrant.Tags` is real, populated data already echoed on every response (same shape as
  the previous pass's UsageLimit/Hsm* fixes). Fixed via the existing `anyTagMatchesFilter` helper
  (`handler_snapshot_copy.go`).
- `DescribeSnapshotSchedules`: both `ClusterIdentifier` and `TagKeys`/`TagValues` were declared and
  never read. `SnapshotSchedule.AssociatedClusters` (derived at read time from
  `Cluster.SnapshotScheduleIdentifier`) and `SnapshotSchedule.Tags` are both real, populated data.
  Fixed both (`handler_snapshot_schedules.go`).
- `DescribeTableRestoreStatus`: `TableRestoreRequestId` -- the real per-request identifier,
  `TableRestoreStatus.TableRestoreRequestID` -- was declared and never read; only `ClusterIdentifier`
  was. A client polling one specific restore request got back every restore status for the account
  instead. Fixed (`handler_table_restore.go`).

Confirmed correct / left alone, with reasoning:

- `DescribeAccountAttributes`: `AttributeNames` declared, never read, but the whole response is a
  static empty envelope regardless (no account-quota data modeled anywhere in this backend) --
  filtering an unconditionally empty set is provably inert. Not fixed, matches this file's existing
  "legitimately static/filter-less" note.
- `DescribeClusterVersions`: `ClusterVersion`/`ClusterParameterGroupFamily` declared, never read,
  but the static catalog has exactly one entry (`modelVersion10`) -- provably inert, same standard as
  a single-legal-value enum.
- `DescribeClusterTracks` / `DescribeOrderableClusterOptions`: `MaintenanceTrackName` /
  `ClusterVersion`+`NodeType` declared, never read. Both catalogs are small, hardcoded reference
  tables (2 and 4 entries) rather than real per-account resource state -- consistent with this file's
  prior explicit judgment call on the same ops ("legitimately static/filter-less", `families.
  Descriptive/static ops` above). Re-examined this pass and left as-is rather than reversing that
  call: unlike the fixed bugs above, there is no real per-account backend record being silently
  hidden here, only a fixed reference list whose contents do not vary by account or request.
- `DescribeEventSubscriptions`: `TagKeys`/`TagValues` declared, never read -- `EventSubscription`
  (`events.go`) has no `Tags` field at all, missing backend data, not a misread key.
- `DescribeNodeConfigurationOptions`: `NodeType`/`NumberOfNodes` (carried inside `Filters`, not
  top-level params) are already read via `nodeConfigFilterValue`'s indexed-list fallback, verified
  against the real wire key (`Filter.NodeConfigurationOptionsFilter.N.Name`/`.Value.item.M`,
  `awsAwsquery_serializeDocumentValueStringList` uses `item` not `member`). `Operator` (eq/lt/le/gt/
  ge/between/in) is not honoured -- every filter is treated as equality -- a real gap, but a missing
  feature on an already-correctly-read field, not the silent-full-list class this sweep targets; not
  fixed, left for a follow-up pass.
- `DescribeReservedNodeExchangeStatus`: `ReservedNodeExchangeRequestId` declared, never read, but
  this backend does not model exchange requests as distinct entities at all (`Describe
  ReservedNodeExchangeStatus` returns a hardcoded "Succeeded" keyed only on whether the reserved
  node exists) -- missing backend data, not a misread key.
- `DescribeStorage`: real Input struct has zero fields (`noSmithyDocumentSerde` only) -- nothing to
  misread.
- `DescribeAuthenticationProfiles`, `DescribeClusterDbRevisions`, `DescribeDataShares`,
  `DescribeDataSharesForConsumer`, `DescribeDataSharesForProducer`, `DescribeDefaultClusterParameters`,
  `DescribeEndpointAuthorization`, `DescribeLoggingStatus`, `DescribePartners`,
  `DescribeQev2IdcApplications`, `DescribeRedshiftIdcApplications`, `DescribeReservedNodeOfferings`,
  `DescribeReservedNodes`, `DescribeResize`: every real request field is already read; re-diffed
  field-by-field against each op's Input struct, no gaps found.

New tests (`wire_field_fixes_test.go`, real `aws-sdk-go-v2` client, decoded-response assertions,
each hand-confirmed to fail against the pre-fix handler):
`TestDescribeClusterParameters_FiltersBySource`, `TestDescribeEventCategories_FiltersBySourceType`,
`TestDescribeCustomDomainAssociations_FiltersByCertificateArn`,
`TestDescribeInboundIntegrations_ReturnsRealData`, `TestDescribeIntegrations_FiltersBySourceArn`,
`TestDescribeSnapshotCopyGrants_FiltersByTagKeys`,
`TestDescribeSnapshotSchedules_FiltersByClusterIdentifier`,
`TestDescribeTableRestoreStatus_FiltersByRequestId`.

Gates: `go build ./services/redshift/...`, `go vet ./...` (repo-wide, clean), `go test -race
-count=1 ./services/redshift/...` (pass), `golangci-lint run ./services/redshift/...` (0 issues).
Work left uncommitted per this pass's instructions.

## enumcheck confident-tier fix (2026-08-30)

`cmd/enumcheck`'s CONFIDENT tier flagged `PurchaseReservedNodeOffering`'s
`State: "payment-pending"`. Not actually an enum-class bug: real
`types.ReservedNode.State` is a plain `*string`
(redshift@v1.65.4 types/types.go), not a typed enum, so `cmd/enumcheck`'s
match against an unrelated `State` enum sharing the wire key name was a
false positive for that tool's class. But the doc comment on that field
enumerates AWS's own legal values, and gopherstack's word order was
backwards: real AWS's pending-payment value is `"pending-payment"` (state,
then reason), not `"payment-pending"` (reason, then state). Fixed the
literal. Covered by `TestPurchaseReservedNodeOffering_State`
(`handler_sdk_roundtrip_test.go`), driven through the real SDK client.

## 2026-08-30 anonymous-struct-decode sweep (gopherstack-4a8v): re-verified clean, no code change

`cmd/reqfieldscan`'s fifth dispatch shape (anonymous inline `var req
struct{...}` literal-decode, no `WrapOp`) made this service newly visible
to that scanner: dispatch coverage 47/60 resolved (78%, both coverage lines
identical, no guard warning — the 13 unresolved ops are the
Get/Put/DeleteResourcePolicy, Create/Update/Delete/ListSnapshotCopyConfiguration,
Create/Get/List/Update/DeleteEndpointAccess, and GetIdentityCenterAuthToken
families. Root cause confirmed by reading the tool's own resolution path
(main.go's package doc) plus this package's handlers: the literal-decode
path resolves an op name only via a "handle"+opName fallback match
(case-insensitive), and every one of these 13 ops' real, correctly-shaped
`var req struct{...}` decoder lives on `*ServerlessHandler` under an
`SL`-suffixed function name (`handleGetResourcePolicySL`,
`handleCreateSnapshotCopyConfigurationSL`, `handleListEndpointAccessSL`,
`handleGetIdentityCenterAuthTokenSL`, ...) that "handle"+opName never
matches. Three of the 13 (`GetResourcePolicy`/`PutResourcePolicy`/
`DeleteResourcePolicy`) additionally have a same-named classic-Redshift
handler (`handleGetResourcePolicy(vals url.Values)`, query-param based, no
JSON body) that the fallback matches instead, finding no decode there
either; the other ten have no classic-Redshift op of the same name at all
and simply match nothing. `RestoreFromRecoveryPoint` is the one op in this
family whose Serverless handler kept the unsuffixed name
(`handleRestoreFromRecoveryPoint`), so it alone resolved. A disclosed
measurement gap, not a plausibility problem — worth a follow-up to
cmd/reqfieldscan, not chased here (out of this pass's scope:
cmd/reqfieldscan is held by another agent this pass). One field flagged:
`RestoreFromRecoveryPoint`'s `MaintainIntegration`
(`handler_serverless_recovery.go:65`).

Hand-verified against `redshiftserverless@v1.38.5`'s
`api_op_RestoreFromRecoveryPoint.go`: `MaintainIntegration` is a real
`*bool` member ("If true, maintain existing data sharing, zero-ETL and S3
event integrations when restoring"), parsed off the wire and never passed
to `RestoreFromRecoveryPointSL`. This is the identical field, on the
identical sibling operation's honest-gap precedent already fixed and
documented in the 2026-08-13 pass above: `RestoreFromSnapshotSL`
(`serverless_restore.go`) accepts the same field on
`RestoreFromSnapshotParams` and explicitly discards it
(`_ = p.MaintainIntegration`) with the reasoning "this backend does not
model data-sharing/zero-ETL/S3-event integration state on namespaces at
all, so there is nothing to maintain or drop." `RestoreFromRecoveryPointSL`
has exactly the same limitation — no integration state exists anywhere on
`Namespace` for either restore path to gate. Confirmed structurally, not
just by analogy: grepped this package's `Namespace`/`ServerlessNamespace`
struct and found no data-sharing/zero-ETL/S3-event field on either.

**No code or test changes made to this service this pass.** The flagged
field is an honest, structurally-identical restatement of an
already-fixed-and-documented sibling gap, not a new bug — restraint per
this campaign's own instructions, not a fabricated clean bill. This
service's earlier verdict (see `overall: A` header and the Serverless
family notes above) holds.

Gates: not re-run (no change); `go build ./...` and `go vet ./...`
(repo-wide) confirmed clean as part of this session's checks.

## 2026-08-31 per-item exact-case sweep (gopherstack-21my continuation), classic Redshift

Byte-for-byte item-level check against redshift@v1.65.4 deserializers.go
(awsAwsquery_, confirmed by the `strings.EqualFold` match sites, not assumed
from age or neighbours). Classic Redshift (as opposed to Redshift Serverless,
which is out of scope here -- see the earlier `gopherstack-4a8v` entry above)
had not been touched by this issue's two-layer batches before now (`rds` in
6flj's notes is Amazon RDS, a different service).

Covered, both wrapper key and every populated per-item field name, including
nested wrapping shape:

- **DescribeClusters** (`Cluster`, the richest item shape in this service):
  AquaConfiguration{AquaConfigurationStatus,AquaStatus}, MasterUsername,
  PreferredMaintenanceWindow, Endpoint{Address,Port}, ClusterStatus, NodeType,
  ClusterAvailabilityStatus, MultiAZ, ClusterIdentifier,
  SnapshotScheduleIdentifier, DBName, KmsKeyId, AvailabilityZoneRelocationStatus,
  SnapshotScheduleState, CatalogArn, LakehouseRegistrationStatus,
  ClusterParameterGroups>ClusterParameterGroup{ParameterGroupName,
  ParameterApplyStatus}, ClusterNodes>member{NodeRole,PrivateIPAddress,
  PublicIPAddress}, IamRoles>ClusterIamRole{IamRoleArn,ApplyStatus},
  Tags>Tag{Key,Value}, NumberOfNodes, EnhancedVpcRouting, Encrypted. Wrapper
  `Clusters>Cluster` confirmed against `awsAwsquery_deserializeDocumentClusterList`
  (item-type-name leaf, not `member` -- one of the query-protocol exceptions to
  the usual `member` convention, matching this service's own struct tags exactly).
  All clean.
- **DescribeClusterSnapshots** (`Snapshot`): SnapshotIdentifier,
  ClusterIdentifier, SnapshotType, SnapshotCreateTime, Status,
  AccountsWithRestoreAccess>AccountWithRestoreAccess{AccountId,AccountAlias},
  ManualSnapshotRetentionPeriod. Wrapper `Snapshots>Snapshot`. All clean.
- **DescribeClusterSecurityGroups** (`ClusterSecurityGroup`):
  ClusterSecurityGroupName, Description, EC2SecurityGroups>EC2SecurityGroup
  {EC2SecurityGroupName,EC2SecurityGroupOwnerId,Status}, IPRanges>IPRange
  {CIDRIP,Status}. Wrapper `ClusterSecurityGroups>ClusterSecurityGroup`. All clean.
- **DescribeEventSubscriptions** (`EventSubscription`): SubscriptionCreationTime,
  CustSubscriptionId, CustomerAwsId, SnsTopicArn, Status, SourceType, Severity,
  SourceIdsList>SourceId, EventCategoriesList>EventCategory, Enabled. Wrapper
  `EventSubscriptionsList>EventSubscription` (note: the wrapper key itself is
  `EventSubscriptionsList`, not the more guessable `EventSubscriptions` --
  confirmed exact). All clean.

No hard mismatches, no case-only mismatches, no wrong list-wrapping shape found
in any of the above. No unwrapped-list-deserializer call site exists for
`ClusterList`, `ClusterSecurityGroups`, `EventSubscriptionsList`, or
`SnapshotList` in the pinned SDK (grepped `*ListUnwrapped`/`*Unwrapped` by name
-- zero call sites outside their own func definitions). No code changes this
pass -- everything checked was already correct.

NOT REACHED at item level: the ~90+ remaining Describe/List ops in this
service (DescribeClusterParameters, DescribeClusterParameterGroups,
DescribeClusterSubnetGroups, DescribeClusterVersions, DescribeClusterDbRevisions,
DescribeOrderableClusterOptions, DescribeReservedNodes,
DescribeReservedNodeOfferings, DescribeHsmClientCertificates,
DescribeHsmConfigurations, DescribeScheduledActions, DescribeUsageLimits,
DescribeDataShares and family, DescribeCustomDomainAssociations,
DescribeInboundIntegrations, DescribeIntegrations, DescribeSnapshotCopyGrants,
DescribeSnapshotSchedules, DescribeTableRestoreStatus, DescribeTags,
DescribeAuthenticationProfiles, DescribePartners, DescribeNodeConfigurationOptions,
and the remaining IDC-application/lakehouse/advisor families).

Gates: `go build ./services/redshift/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/redshift/...` (pass, no test changes needed
since no code changed), `golangci-lint run ./services/redshift/...` (0 issues).

## 2026-09-08 delete/modify precondition sweep (gopherstack-d1xc, partial)

Scope per gopherstack-d1xc: delete/modify preconditions beyond
DeleteClusterSnapshot (subnet/security/parameter-group in-use checks on
delete, cluster-state preconditions on Modify ops), ghost rows after delete,
performance, resource leaks. This pass did NOT finish the list -- see NOT
COVERED below.

**nil-on-write fall-through (elasticache-class bug) checked first, NOT
PRESENT here.** `writeError`/`handleOpError` (`handler.go:882-903`) return a
value the caller returns directly (`return h.handleOpError(c, action, opErr)`
at `handler.go:547`, `dispatch`'s only caller), and are used in only 8 call
sites total across the whole package, all direct-return. Individual op
handlers (`handleModifyCluster` etc.) never touch `echo.Context` at all --
they return `(any, error)` to `dispatch`, which is the sole place an error is
turned into a wire response. There is no intermediate site where a rejection
write's return value is stored and re-checked, so the bug class this campaign
keeps finding elsewhere cannot occur in this package's current shape.

**In-use preconditions on delete (priority 1):**
- `DeleteClusterSecurityGroup` (`security_groups.go:44`) and
  `DeleteClusterParameterGroup` (`param_groups.go:138`) ALREADY had correct
  in-use checks (loop over `b.clusters.All()`, reject with
  `ErrSecurityGroupInvalidState`/`ErrParameterGroupInvalidState` if any
  cluster references the group) with existing tests
  (`handler_security_groups_test.go`/`handler_param_groups_test.go`,
  `associated_with_cluster_rejected` cases) asserting the 400 + wire code. Not
  modified this pass; not a defect.
- `DeleteClusterSubnetGroup` (`subnet_groups.go:60`) has NO in-use check at
  all, and real `DeleteClusterSubnetGroup` declares
  `InvalidClusterSubnetGroupStateFault` ("The cluster subnet group cannot be
  deleted because it is in use", confirmed against botocore
  redshift/2012-12-01/service-2.json). **This is structurally blocked, not a
  quick fix**: `Cluster` (`models.go:346`) has no `ClusterSubnetGroupName`
  field at all -- `CreateCluster` (`handler.go:553`,`store.go:211`) never
  reads or stores a subnet group reference, so there is nothing to check
  in-use against. This gap was already flagged (not fixed) in the
  2026-08-29 PARITY.md entry above ("missing-feature gap, not a wrong-key
  bug"). Fixing it for real means adding the field to `Cluster` (persisted
  type change), parsing `ClusterSubnetGroupName` in `handleCreateCluster`,
  validating it against `ClusterSubnetGroupNotFoundFault` (also declared on
  `CreateClusterInput`), and only then adding the in-use check -- and
  `Backend.CreateCluster`'s signature is called at 84 sites across this
  package's test files alone, so a signature change ripples everywhere. Left
  unfixed this pass as out of budget for a P3 sweep; flagging it explicitly
  rather than leaving it silently unaudited.

**Cluster-state preconditions on Modify ops (priority 3) -- two fixed, rest
surveyed but not fixed:**

1. **`ModifyCluster` accepted modification of a non-`available` cluster.**
   Real `ModifyCluster` declares `InvalidClusterStateFault` ("The specified
   cluster is not in the available state", confirmed against
   `InvalidClusterStateFault`'s botocore doc and its presence in
   `awsAwsquery_deserializeOpErrorModifyCluster`,
   aws-sdk-go-v2/service/redshift@v1.65.4/deserializers.go). `PauseCluster`
   (`cluster_mgmt.go`) sets `Status="paused"` with no reconciler transition
   back, so a paused cluster is a real, reachable, indefinite non-available
   state (no activation-delay config needed). Before the fix, `ModifyCluster`
   on a paused cluster silently applied the change and left the cluster
   `paused` with a mutated `NodeType` -- a real client would see 200 OK and a
   changed cluster where AWS would 400. Regression test
   `TestModifyCluster_RejectsWhenClusterNotAvailable`
   (`handler_cluster_mgmt_test.go`) confirmed failing pre-fix (asserted 400,
   got 200 with `NodeType` changed to `ra3.xlplus`); fix adds the same
   available-state guard `namespace_registration.go:132` already uses for
   `RegisterNamespace`, via a new sentinel `ErrClusterInvalidState`
   (`errors.go`, wired into `errCodeSentinels`). Test asserts the emitted
   wire code AND that a follow-up `DescribeClusters` shows the original
   `NodeType`/status unchanged (not just that some error occurred).
2. **`ModifyClusterIamRoles` had the same gap** -- its declared error set is
   `[InvalidClusterStateFault, ClusterNotFoundFault]` only (botocore), i.e.
   the state check is essentially the entire precondition surface this op
   has beyond existence. Regression test
   `TestModifyClusterIamRoles_RejectsWhenClusterNotAvailable` confirmed
   failing pre-fix (200, role silently added to a paused cluster); fixed
   with the same `ErrClusterInvalidState` guard in
   `ModifyClusterIamRoles` (`cluster_mgmt.go:302`).

Both new tests run under `-race -count=10` (10/10 pass); neither touches
global/shared state (each spins up its own `InMemoryBackend`/`Handler`), so
no `synctest` or parallelism-flake concern applies.

**NOT COVERED this pass** (confirmed missing the same class of guard by
reading each function body, not fixed -- flagging honestly rather than
silently skipping): `ModifyClusterMaintenance` (`cluster_mgmt.go:343`),
`ModifyClusterDBRevision`, `ModifyAquaConfiguration`,
`ModifyLakehouseConfiguration`, `RebootCluster`, `PauseCluster` itself (can a
non-`available` cluster be paused?), `ResumeCluster` (must the cluster be
`paused`, not just non-available, to resume?), `ResizeCluster` -- all declare
`InvalidClusterStateFault` per botocore (`ModifyClusterDbRevision` additionally
`ClusterOnLatestRevisionFault`) but none were checked against the backend
code in this pass. Also not covered: `DeleteCluster`'s own state precondition
(can you delete a cluster mid-resize/reboot?), and the ~130 remaining ops in
this package's Delete/Modify surface not named above.

**Ghost rows after delete:** spot-checked `DescribeClusters` (lazily calls
`advanceClusterStates` before reading, `store.go:337-343`, so a
reconciler-pending delete is never stale-visible) and `DeleteEndpointAccess`
(`endpoint_access.go:70`, deletes from the map before returning, ephemeral
"deleting" status only on the returned copy, not stored) -- both clean, no
defect. The other ~27 `Delete*`/`Deregister*` backend methods in this package
(listed by `grep -n "^func (b \*InMemoryBackend) Delete"`) were NOT
individually re-verified for ghost-row correctness this pass.

**Resource leaks:** the package has exactly one ticker/background goroutine
(`reconciler.go`'s `reconcileLoop`, via `time.NewTicker`). Read in full:
`StartReconciler`/`StopReconciler` are wired to `service.BackgroundWorker`/
`service.Shutdowner` (`handler.go:74-86`) with the framework's real ctx (not
`context.Background()`), the loop selects on both `ctx.Done()` and an
explicit stop channel, `ticker.Stop()` is deferred, and `StopReconciler`
blocks on a `WaitGroup` until the goroutine actually exits. No leak found --
confirmed by reading the full lifecycle, not by grep absence.

**Performance:** not audited this pass beyond what the above reading surfaced
(no O(n^2) or obviously pathological pattern noticed in the functions read,
but no dedicated pass was made).

No persisted-type field was added (only a new error sentinel and status
checks), so `pkgs/persistence` golden data needed no `-update`; ran
`go test ./pkgs/persistence/...` anyway to confirm (pass).

Gates this pass: `golangci-lint run ./services/redshift/...` (0 issues),
`go test -race ./services/redshift/...` (pass), plus the two new tests
individually under `-race -count=10` (10/10 pass each).
