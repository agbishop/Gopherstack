---
service: sesv2
sdk_module: aws-sdk-go-v2/service/sesv2@v1.66.4   # version audited against (bumped from v1.60.1; 2 new ops appeared: PutAccountPricingAttributes, PutTenantSuppressionAttributes)
last_audit_commit: 8ddfcca9b7157a079a75e8cda1d26d70118f4ae9
last_audit_date: 2026-08-13
overall: A            # route-matcher rewrite + wire-shape DTOs; this pass implemented the 2 new v1.66.0 ops and fixed a previously-mis-graded GetAccount wire-shape bug found while wiring PutAccountPricingAttributes in (see "This pass (2026-07-25)")
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateEmailIdentity: {wire: ok, errors: ok, state: ok, persist: ok}
  GetEmailIdentity: {wire: ok, errors: ok, state: ok, persist: ok}
  ListEmailIdentities: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEmailIdentity: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-09-04 (parity sweep, gopherstack-22qd) -- deleted the identity but left b.resourceTags[identityARN] and b.emailIdentityPolicies[identity] behind; a re-verified identity of the same name inherited both. Same class as DeleteContactList/DeleteDedicatedIpPool/DeleteEmailTemplate/DeleteTenant."}
  CreateConfigurationSet: {wire: ok, errors: ok, state: ok, persist: ok}
  GetConfigurationSet: {wire: fixed, errors: ok, state: ok, persist: ok, note: "TrackingOptions.CustomRedirectDomain (required on types.TrackingOptions) was tagged omitempty and dropped whenever a caller set HttpsPolicy alone via PutConfigurationSetTrackingOptions -- see 2026-08-21 entry (gopherstack-r80d batch 21)"}
  ListConfigurationSets: {wire: fixed, errors: ok, state: ok, persist: ok, note: "ConfigurationSets was []{Name} objects; real shape is []string. handler.go:listConfigurationSetsOutput"}
  DeleteConfigurationSet: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-09-04 (parity sweep) -- deleted the configuration set (cascading its event destinations) but left b.resourceTags[configurationSetARN] behind. Same class as DeleteContactList/DeleteDedicatedIpPool/DeleteEmailTemplate/DeleteEmailIdentity/DeleteTenant."}
  CreateConfigurationSetEventDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  GetConfigurationSetEventDestinations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "items marshalled internal EventDestination struct (lowerCamelCase tags, extra ConfigurationSetName/CreatedAt fields); added eventDestinationOutput"}
  DeleteConfigurationSetEventDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateConfigurationSetEventDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  PutConfigurationSetSendingOptions: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "route required sub-path 'sending-options'; real path is '.../sending'. Unroutable before fix."}
  PutConfigurationSetArchivingOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  PutConfigurationSetDeliveryOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  PutConfigurationSetReputationOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  PutConfigurationSetSuppressionOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  PutConfigurationSetTrackingOptions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "CustomRedirectDomain is optional on this op's own input (api_op_PutConfigurationSetTrackingOptions.go), so a caller can set HttpsPolicy alone -- see GetConfigurationSet's 2026-08-21 entry"}
  PutConfigurationSetVdmOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  SendEmail: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED 2026-08-29 (error-path sweep) -- an unverified From identity/domain raised BadRequestException (the generic sentinel), but SendEmail's own deserializeOpError models the dedicated MailFromDomainNotVerifiedException for exactly this case (types/errors.go:220, 'The message can't be sent because the sending domain isn't verified.'); a real client's errors.As against that type never matched. Now raises the dedicated sentinel. Wrong-sentinel bug, not missing -- gopherstack already checked the condition, just labeled it with the wrong wire code. FIXED 2026-09-04 (parity sweep) -- Content.Template (api_op_SendEmail.go:24-26, 'Templated -- a message that contains personalization tags') was decoded into no field at all: emailContent only had Simple/Raw, so a templated SendEmail silently sent an empty subject/body. Now shares SendBulkEmail's bulkEmailTemplate resolution (inline TemplateContent or a TemplateName lookup, {{var}} substitution)."}
  SendBulkEmail: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "request body was parsed into map[string]any with ad-hoc type assertions; now typed (bulkEmailEntry/bulkEmailDestination/messageHeader/messageTag/replacementEmailContent/replacementTemplate in send_email.go, field-diffed against types.BulkEmailEntry et al), and the response uses bulkEmailEntryResultOutput (types.BulkEmailEntryResult) instead of a raw map. gopherstack-afi1: DefaultContent (required, api_op_SendBulkEmail.go:43) was decoded into sendBulkEmailInput but never read -- SendEmail was called with hardcoded empty subject/HTML/text, so every bulk email was recorded with no content regardless of what the caller sent. Now resolves DefaultContent.Template (inline TemplateContent, or a TemplateName lookup against b.emailTemplates -- NotFoundException if missing) and applies {{var}} substitution (parseTemplateVars/renderTemplateVars, shared with TestRenderEmailTemplate) using TemplateData merged with each entry's ReplacementEmailContent.ReplacementTemplate.ReplacementTemplateData as a per-recipient override. DefaultContent.Template.Attachments/Headers and per-entry ReplacementHeaders/ReplacementTags remain unstored/inert -- consistent with SendEmail's existing scope, which doesn't model attachments/headers/tags on Email either. FIXED 2026-08-29 (error-path sweep) -- per-entry SendEmail call was `msgID, _ := b.SendEmail(...)`, silently discarding the from-identity-not-verified error and always reporting Status SUCCESS with a synthesized message ID regardless. Real AWS reports this per-entry via Status: MAIL_FROM_DOMAIN_NOT_VERIFIED (types.go:305, a real BulkEmailStatus enum value -- confirmed no top-level exception applies here, since the from-identity check is per-recipient-eligible, not per-request). Missing-error bug (success where AWS reports failure), not a wrong sentinel. Now checks the From identity once up front and returns that status for every entry, recording no emails, when unverified."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateContactList: {wire: ok, errors: ok, state: ok, persist: ok}
  GetContactList: {wire: fixed, errors: ok, state: ok, persist: ok, note: "marshalled internal ContactList struct directly (lowerCamelCase tags, wrong field name 'name' vs 'ContactListName'); added contactListOutput"}
  ListContactLists: {wire: fixed, errors: ok, state: ok, persist: ok, note: "item shape now matches types.ContactList (ContactListName+LastUpdatedTimestamp only)"}
  DeleteContactList: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-09-04 (parity sweep) -- deleted the contact list but left its b.resourceTags[contactListARN] entry behind; ListTagsForResource on the deleted (or a same-named recreated) list's ARN kept returning the old tags."}
  UpdateContactList: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateContact: {wire: ok, errors: ok, state: ok, persist: ok}
  GetContact: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added contactOutput (PascalCase, epoch timestamps, TopicPreferences item casing)"}
  ListContacts: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, filter: partial, note: "real route is POST .../contacts/list with NextToken/Filter in the JSON body, not GET .../contacts with a query string; gopherstack had fabricated the GET route and it was completely unroutable by a real SDK client. This pass (2026-08-29): PageSize was parsed but never honored (hardcoded 0) -- fixed. Filter (FilteredStatus/TopicFilter) still unread: ContactList doesn't model per-topic default subscription status needed for TopicFilter.UseDefaultIfPreferenceUnavailable, and the AWS doc doesn't settle what standalone FilteredStatus filters against -- left."}
  DeleteContact: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateContact: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  GetEmailTemplate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "TemplateContent.HTML tag was 'html'/'text'/'subject' lowercase and top-level CreatedAt leaked into the response; real field is 'Html' and GetEmailTemplateOutput has no timestamp"}
  ListEmailTemplates: {wire: fixed, errors: ok, state: ok, persist: ok, note: "metadata items now use TemplateName+CreatedTimestamp (types.EmailTemplateMetadata), no content"}
  DeleteEmailTemplate: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-09-04 (parity sweep) -- same ghost b.resourceTags[emailTemplateARN] entry left behind as DeleteContactList/DeleteDedicatedIpPool/DeleteTenant."}
  UpdateEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  TestRenderEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDedicatedIpPool: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDedicatedIpPool: {wire: fixed, errors: ok, state: ok, persist: ok, note: "response was the bare internal struct (lowerCamelCase, no 'DedicatedIpPool' wrapper); real shape is {DedicatedIpPool: {PoolName, ScalingMode}}"}
  ListDedicatedIpPools: {wire: ok, errors: ok, state: ok, persist: ok, note: "This pass (2026-08-29): PageSize was parsed but hardcoded to 0 -- fixed."}
  DeleteDedicatedIpPool: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED 2026-09-04 (parity sweep) -- same ghost b.resourceTags[dedicatedIPPoolARN] entry left behind as DeleteContactList/DeleteEmailTemplate/DeleteTenant; only visible via ListTagsForResource since DedicatedIpPool has no Tags field of its own."}
  PutDedicatedIpPoolScalingAttributes: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "route required sub-path 'scaling-attributes'; real path is '.../scaling'. Unroutable before fix."}
  GetDedicatedIp: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDedicatedIps: {wire: fixed, errors: ok, state: ok, persist: ok, note: "This pass (2026-08-29): handleGetDedicatedIps took no arguments at all -- PoolName filter, NextToken, and PageSize (all real query params) were completely ignored, always returning every tracked IP on one page. Fixed: backend now filters by pool and paginates."}
  PutDedicatedIpInPool: {wire: ok, errors: ok, state: ok, persist: ok}
  PutDedicatedIpWarmupAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  PutSuppressedDestination: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "top-level path was fabricated as '/v2/email/suppressed-destination'; real path family is '/v2/email/suppression/addresses[/{EmailAddress}]'. All 4 ops in this family were completely unroutable before fix."}
  GetSuppressedDestination: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, note: "also needed a {SuppressedDestination: {...}} wrapper and PascalCase fields"}
  DeleteSuppressedDestination: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed}
  ListSuppressedDestinations: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, filter: partial, note: "This pass (2026-08-29): Reasons/StartDate/EndDate/PageSize (all real query params) were parsed only for NextToken; the rest were dropped -- fixed Reasons/StartDate/EndDate/PageSize. TenantName left: SuppressedDestination has no per-tenant tracking or separate per-tenant store."}
  CreateCustomVerificationEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCustomVerificationEmailTemplate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "added customVerificationEmailTemplateOutput (PascalCase)"}
  ListCustomVerificationEmailTemplates: {wire: fixed, errors: ok, state: ok, persist: ok, note: "metadata items (no TemplateContent) now use customVerificationEmailTemplateMetadataOutput"}
  DeleteCustomVerificationEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCustomVerificationEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  SendCustomVerificationEmail: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "POST /v2/email/outbound-custom-verification-emails was not matched by any path pattern at all; added parseOutboundCustomVerificationEmailsPath"}
  GetAccount: {wire: fixed, errors: ok, state: ok, persist: ok, note: "previously graded 'wire: ok' in error -- the handler marshalled the internal *AccountDetails struct directly (lowerCamelCase snapshot-format tags, flat instead of the real nested Details/SuppressionAttributes/PricingAttributes sub-objects, VdmAttributes keyed 'vdmAttributes' not 'VdmAttributes'), the same bug class already fixed for every other family in this package (see 'Root-cause bug class' below) but missed for Account specifically. Found and fixed while wiring PutAccountPricingAttributes's GetAccount-visible effect this pass. Added accountOutput/accountDetailsOutput/accountSuppressionAttributesOutput/accountPricingAttributesOutput (wire_output.go), field-diffed against GetAccountOutput/types.AccountDetails/types.SuppressionAttributes/types.PricingAttributes. EnforcementStatus/ProductionAccessEnabled/SendQuota/ReviewDetails/ValidationAttributes are honestly omitted (all pointer/optional in the real shape; gopherstack has no account-review, sandbox-status, or send-quota tracking to source them from) rather than fabricated."}
  GetBlacklistReports: {wire: ok, errors: ok, state: ok, persist: n/a}
  PutAccountDetails: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-101r): the request decoded \"UseCaseName\", which is not a real member; the real, deprecated member is UseCaseDescription (api_op_PutAccountDetails.go:60-63). The output side (wire_output.go's toAccountOutput) already emitted the correct \"UseCaseDescription\" key, so a real client's field was silently dropped on the way in even though the readback shape looked right. Checked handler_deliverability.go's two documented deliberate-alias cases (UpdateReputationEntityCustomerManagedStatus/Policy, which read both names with a fallback) before concluding this one has no such alias logic and no in-repo dependency on the old \"UseCaseName\" wire key -- a clean rename, not an alias. Round-trip test: wire_field_fixes_test.go (TestPutAccountDetails_UseCaseDescription)."}
  PutAccountPricingAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "new in aws-sdk-go-v2/service/sesv2 v1.66.0. Real path/verb confirmed against serializers.go: PUT /v2/email/account/pricing-attributes (awsRestjson1_serializeOpPutAccountPricingAttributes's httpbinding.SplitURI). Plan is validated against the real PricingPlan enum (NONE/ESSENTIALS/PRO/ENTERPRISE); an unrecognized value is a BadRequestException. Writes b.accountDetails.PricingPlan (existing account state, no parallel store) and is reflected by GetAccount's PricingAttributes.CurrentPlan. gopherstack has no billing-cycle concept, so the write takes effect immediately as CurrentPlan; PricingAttributes.NextPlan (real SES's 'scheduled for next billing cycle' field) is always empty -- there's nothing to schedule, and reporting a fabricated NextPlan would be worse than omitting it."}
  PutAccountDedicatedIpWarmupAttributes: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "sub-path was 'dedicated-ip-warmup-attributes' (2 segs); real path is 3 segs, 'account/dedicated-ips/warmup'. Unroutable before fix."}
  PutAccountSendingAttributes: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "sub-path was 'sending-attributes'; real is 'sending'. Unroutable before fix."}
  PutAccountSuppressionAttributes: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "sub-path was 'suppression-attributes'; real is 'suppression'. Unroutable before fix."}
  PutAccountVdmAttributes: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "sub-path was 'vdm-attributes'; real is 'vdm'. A dead top-level '/v2/email/vdm-attributes' route (not a real SES path at all) was also removed."}
  BatchGetMetricData: {wire: ok, errors: ok, state: fixed, persist: n/a, note: "now derives real per-day SEND counts from b.emails (gopherstack's actual send history) for Metric=SEND with no dimension or the EMAIL_IDENTITY dimension (matched against each Email's From address/domain, same resolution SendEmail uses) -- genuine aggregated data, not a placeholder. Every other Metric (COMPLAINT/PERMANENT_BOUNCE/TRANSIENT_BOUNCE/OPEN/CLICK/DELIVERY*) and the CONFIGURATION_SET/ISP dimensions have no backing data source (no bounce/complaint/engagement pipeline, no per-email config-set/ISP association) and honestly fall back to a single zero-valued datapoint rather than a fabricated count. Values is now []int64 (was []float64), matching types.MetricDataResult. Request StartDate/EndDate/Dimensions were previously silently dropped by the handler; now decoded (JSON-body epoch-seconds, per serializers.go)."}
  CreateExportJob: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "previously graded 'wire: ok' in error (gopherstack-rcmn): the handler expected a flat DataSource string; real required members are ExportDataSource *types.ExportDataSource (nested MetricsDataSource|MessageInsightsDataSource, exactly one) and ExportDestination *types.ExportDestination (DataFormat required), both absent entirely. A body sending the invented flat field parsed identically to one that sent nothing, so the bug was silent. Now: both required members validated present (400 BadRequestException, matching CreateExportJob's declared error switch -- no ValidationException modeled for this op); ExportDataSource's two branches accepted opaquely via json.RawMessage (gopherstack has no metrics-aggregation or message-log engine to act on Dimensions/Metrics/Namespace/StartDate/EndDate/Exclude/Include/MaxResults) but which branch was set is used to derive and persist ExportSourceType, now echoed back via GetExportJob/ListExportJobs. ExportDestination.S3Url is accepted but not echoed back -- gopherstack never writes an export file, so there is no pre-signed URL to report."}
  GetExportJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "CreateExportJob/GetExportJob leaked lowerCamelCase jobId/jobStatus/createdAt; added exportJobOutput. Now also reports ExportSourceType (see CreateExportJob fix)."}
  CancelExportJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListExportJobs: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, filter: ok, note: "real op is POST /v2/email/list-export-jobs (filter/pagination in body) -- a distinct top-level path from /v2/email/export-jobs, not a GET on that same path. Previous GET-based route was gopherstack-invented and unroutable by a real client; removed and replaced. This pass (2026-08-29): ExportSourceType/JobStatus were both stored on ExportJob already, but a stale handler comment claimed they 'aren't modelled by the backend yet' and neither was applied -- fixed."}
  CreateImportJob: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "previously graded 'wire: ok' in error (gopherstack-rcmn): same bug class as CreateExportJob -- flat invented DataSource string vs real required ImportDataSource *types.ImportDataSource (DataFormat + S3Url, both required, flat and modeled directly) and ImportDestination *types.ImportDestination (nested ContactListDestination|SuppressionListDestination, exactly one, absent entirely). Now: ImportDataSource.DataFormat/S3Url and ImportDestination presence validated (400 BadRequestException); ImportDestination's selected branch (and its own required members -- ContactListImportAction+ContactListName, or SuppressionListImportAction) is stored as the backend ImportDestination and echoed back via GetImportJob/ListImportJobs. gopherstack has no S3 fetcher, so the job never actually applies any records to a contact list or the suppression list -- only which destination the (unfetchable) import targeted is recorded."}
  GetImportJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same lowerCamelCase leak as ExportJob; added importJobOutput. Now also reports ImportDestination (see CreateImportJob fix)."}
  ListImportJobs: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, filter: ok, note: "real op is POST /v2/email/import-jobs/list (filter/pagination in body), not GET /v2/email/import-jobs. Previous GET-based route removed and replaced. This pass (2026-08-29): ImportDestinationType was derivable from ImportDestination's already-stored oneof branch, but a stale handler comment claimed it 'isn't modelled by the backend yet' and it was never applied -- fixed."}
  CreateEmailIdentityPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetEmailIdentityPolicies: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEmailIdentityPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateEmailIdentityPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEmailIdentityConfigurationSetAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEmailIdentityDkimAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEmailIdentityDkimSigningAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEmailIdentityFeedbackAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEmailIdentityMailFromAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDeliverabilityTestReport: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDeliverabilityTestReport: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "real sub-path is 'test-reports[/{ReportId}]', not 'reports[/{id}]'. Fixed alongside ListDeliverabilityTestReports."}
  ListDeliverabilityTestReports: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "same 'test-reports' vs 'reports' route fix as GetDeliverabilityTestReport."}
  GetDeliverabilityDashboardOptions: {wire: ok, errors: ok, state: ok, persist: ok, note: "now reflects PutDeliverabilityDashboardOption state (DashboardEnabled/AccountStatus/ActiveSubscribedDomains); previously hardcoded {DashboardEnabled:false}"}
  PutDeliverabilityDashboardOption: {wire: ok, errors: ok, state: ok, persist: ok, note: "was a true no-op; now persists enablement + subscribed-domain list (b.deliverabilityDashboardEnabled/b.deliverabilityDashboardDomains, wired into Reset/Snapshot/Restore) so GetDeliverabilityDashboardOptions reflects it"}
  GetDomainDeliverabilityCampaign: {wire: ok, errors: ok, state: fixed, persist: n/a, route: fixed, note: "real path is deliverability-dashboard/campaigns/{CampaignId} (campaign ID only, no domain -- confirmed against GetDomainDeliverabilityCampaignInput). Backend signature changed to drop the domain param. CampaignId/FromAddress/Subject/FirstSeenDateTime/LastSeenDateTime are now derived for real by grouping b.emails (gopherstack's actual send history) by (FromAddress, Subject) -- the same key real SES auto-detects campaigns by -- via campaignIDFor/domainCampaignsLocked (deliverability.go); a campaignID with no matching send history falls back to the prior echoed-ID placeholder rather than NotFoundException, since gopherstack has no way to distinguish a caller-guessed ID from one it legitimately handed out. InboxCount/SpamCount/ReadRate/DeleteRate/ReadDeleteRate/ProjectedVolume/Esps/SendingIps remain honest zero-valued/empty placeholders -- real inbox/spam placement tracking requires opted-in production sending history AWS tracks server-side, which gopherstack genuinely can't derive."}
  GetDomainStatisticsReport: {wire: fixed, errors: ok, state: partial, persist: n/a, route: fixed, note: "real sub-path is 'statistics-report/{Domain}', not 'statistics/{domain}'. DailyVolumes now enumerates one entry per calendar day in the requested [StartDate, EndDate] window (RFC3339 query params, parsed via parseSESv2Timestamp; capped at maxDailyVolumeDays=366) matching what GetDomainStatisticsReportOutput documents (\"data for each day\") -- previously always an empty list regardless of range, which was itself a wire-shape gap, not just a data placeholder. Every actual statistic (VolumeStatistics/DomainIspPlacements/ReadRatePercent, both per-day and in OverallVolume) is an honest zero/empty placeholder: every field in this shape measures inbox-vs-spam delivery placement, which requires real mail-delivery-outcome tracking gopherstack doesn't have -- there's no plausible partial derivation the way there is for the campaign family."}
  ListDomainDeliverabilityCampaigns: {wire: ok, errors: ok, state: fixed, persist: n/a, route: fixed, note: "real path is deliverability-dashboard/domains/{SubscribedDomain}/campaigns; gopherstack's old 'campaigns/{domain}/{id}' pattern actively misrouted a real GET to campaigns/{CampaignId} as this op with the campaign ID misread as a domain. Now derives real campaigns from b.emails via domainCampaignsLocked (see GetDomainDeliverabilityCampaign), filtered to messages with a recipient in the subscribed domain and restricted to campaigns overlapping [StartDate, EndDate] when both parse. Same InboxCount/etc. placeholder tradeoff as GetDomainDeliverabilityCampaign."}
  GetEmailAddressInsights: {wire: ok, errors: ok, state: partial, persist: n/a, route: fixed, note: "real op is POST /v2/email/email-address-insights with EmailAddress in the body; gopherstack had a fabricated GET /v2/email/email-insights/{email}. HasValidSyntax and IsRoleAddress are now real checks (regex + role-address local-part lookup); HasValidDnsRecords/IsDisposable/IsRandomInput/MailboxExists are honest MEDIUM-confidence placeholders since gopherstack has no DNS/disposable-domain/mailbox-probing data source."}
  GetMessageInsights: {wire: ok, errors: ok, state: ok, persist: n/a, route: fixed, note: "real path is /v2/email/insights/{MessageId}; gopherstack had a fabricated /v2/email/messages/{id}. Was a stub returning {}; now looks up the message in the backend's SendEmail history and returns NotFoundException for an unknown MessageId, matching real semantics -- this is the one insights op gopherstack has genuine data for."}
  ListRecommendations: {wire: ok, errors: ok, state: fixed, persist: n/a, route: fixed, note: "real op is POST /v2/email/vdm/recommendations (Filter/NextToken/PageSize in body); gopherstack had a fabricated GET /v2/email/recommendations. Filter was previously decoded by the handler and silently dropped; now threaded through and applied (TYPE/STATUS/IMPACT/RESOURCE_ARN, ANDed). Now derives real OPEN/HIGH-impact recommendations from gopherstack's actual configuration state: DKIM for identities with DkimSigningEnabled=false, SPF for identities with a MAIL FROM domain that hasn't reached SUCCESS status (gopherstack never simulates async verification, so it's honestly stuck at PENDING), COMPLAINT for reputation entities with CustomerManagedStatus=DISABLED. DMARC/BIMI and reputation-finding-driven types (BOUNCE/FEEDBACK_3P/IP_LISTING) are never returned -- gopherstack has no DNS-record model or bounce/complaint-rate pipeline to derive those from, and fabricating them would be worse than omitting them (see ListRecommendations' doc comment, deliverability.go)."}
  ListReputationEntities: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, filter: partial, note: "real op is POST /v2/email/reputation/entities (filter/pagination in body); gopherstack only accepted GET. A gopherstack-invented duplicate top-level path, /v2/email/reputation-entities/..., was also found and deleted (not in the real SDK at all; the real 'reputation/entities/...' family already covered every op in this family correctly). Now returns []reputationEntityOutput (typed) instead of []map[string]any. This pass (2026-08-29): the backend signature discarded NextToken/PageSize into blank identifiers (`_, _`), always returning every entity on one page; Filter was parsed by the handler and never even passed to the backend. Fixed pagination and SENDING_STATUS/ENTITY_REFERENCE_PREFIX. ENTITY_TYPE/REPUTATION_IMPACT left: EntityType is never assigned anywhere in this backend (always empty) and there is no reputation-impact field on the model."}
  GetReputationEntity: {wire: fixed, errors: ok, state: ok, persist: ok, note: "field-diffed against types.ReputationEntity: ReputationEntityReference/ReputationEntityType/CustomerManagedStatus (nested {Status: ...}, matching *StatusRecord)/ReputationManagementPolicy were already correct; SendingStatusAggregate (derived from CustomerManagedStatus, gopherstack has no separate AWS-SES-managed status to combine it with) unchanged. Now a typed reputationEntityOutput/statusRecordOutput DTO (wire_output.go) instead of an ad-hoc map[string]any -- same field-verified-correct shape, now compile-time checked."}
  UpdateReputationEntityCustomerManagedStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateReputationEntityPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateMultiRegionEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "field-diffed against CreateMultiRegionEndpointOutput (EndpointId/Status); generates a real EndpointId and reads Details.RoutesDetails[].Region/Tags from the body. Returns AlreadyExistsException for a duplicate name. Now returns a typed *createMultiRegionEndpointOutput (wire_output.go) instead of map[string]any."}
  GetMultiRegionEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "field-diffed against GetMultiRegionEndpointOutput/types.Route: CreatedTimestamp/LastUpdatedTimestamp (epoch)/Routes ([]{Region}, projected from the endpoint's region list) unchanged. Now returns a typed *multiRegionEndpointOutput (wire_output.go) instead of map[string]any."}
  DeleteMultiRegionEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against DeleteMultiRegionEndpointOutput: returns {Status: \"DELETING\"} (the status documented as returned 'right after the delete request'); NotFoundException for an unknown name."}
  ListMultiRegionEndpoints: {wire: fixed, errors: ok, state: ok, persist: ok, note: "item shape matches types.MultiRegionEndpoint (EndpointId/EndpointName/Status/Regions/CreatedTimestamp/LastUpdatedTimestamp, no Routes -- that's Get-only). Now returns []multiRegionEndpointSummaryOutput (typed, wire_output.go) instead of []map[string]any."}
  CreateTenant: {wire: fixed, errors: ok, state: ok, persist: ok, note: "field-diffed against CreateTenantOutput/types.Tenant: TenantId/TenantArn (via pkgs/arn)/SendingStatus/CreatedTimestamp/Tags unchanged. AlreadyExistsException for a duplicate name. Now returns a typed *tenantOutput (wire_output.go) instead of map[string]any."}
  GetTenant: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, note: "real op is POST /v2/email/tenants/get with TenantName in the body (RPC-style, no REST path param); gopherstack had GET /v2/email/tenants/{name}. Response wrapped in {Tenant: {...}} matching GetTenantOutput, now a typed *tenantOutput instead of map[string]any."}
  DeleteTenant: {wire: ok, errors: ok, state: fixed, persist: ok, route: fixed, note: "real op is POST /v2/email/tenants/delete with TenantName in the body; gopherstack had DELETE /v2/email/tenants/{name}. Cascades: removes the tenant's resource associations from both the tenant->resources and resource->tenants indexes. FIXED 2026-09-04 (parity sweep) -- despite that cascade's own 'no ghost rows remain' claim, b.resourceTags[tenantARN] was left behind after delete; same class of bug as DeleteContactList/DeleteDedicatedIpPool/DeleteEmailTemplate."}
  ListTenants: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, note: "real op is POST /v2/email/tenants/list with NextToken/PageSize in the body; gopherstack had GET /v2/email/tenants. Item shape matches types.TenantInfo (TenantName/TenantId/TenantArn/CreatedTimestamp, no SendingStatus/Tags -- those are Get-only), now []tenantInfoOutput (typed, wire_output.go) instead of []map[string]any."}
  CreateTenantResourceAssociation: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "real op is POST /v2/email/tenants/resources with TenantName+ResourceArn in the body; gopherstack had POST /v2/email/tenants/{name}/resources. NotFoundException if the tenant doesn't exist."}
  DeleteTenantResourceAssociation: {wire: ok, errors: ok, state: ok, persist: ok, route: fixed, note: "real op is POST /v2/email/tenants/resources/delete with TenantName+ResourceArn in the body; gopherstack had DELETE /v2/email/tenants/{name}/resources/{arn}."}
  ListResourceTenants: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, note: "real op is POST /v2/email/resources/tenants/list -- a distinct top-level path from the rest of the tenant family (/v2/email/tenants/...) -- with ResourceArn/NextToken/PageSize in the body; gopherstack had the fabricated GET /v2/email/resource-tenants. Item shape matches types.ResourceTenantMetadata (TenantName/TenantId/ResourceArn/AssociatedTimestamp), now []resourceTenantOutput (typed, wire_output.go) instead of []map[string]any."}
  ListTenantResources: {wire: fixed, errors: ok, state: ok, persist: ok, route: fixed, note: "real op is POST /v2/email/tenants/resources/list with TenantName/Filter/NextToken/PageSize in the body; gopherstack had GET /v2/email/tenants/{name}/resources and silently dropped NextToken entirely. Item shape matches types.TenantResource (ResourceArn/ResourceType, inferred from the ARN's resource-segment prefix); the RESOURCE_TYPE filter key is honored. Now []tenantResourceOutput (typed, wire_output.go) instead of []map[string]any."}
  PutTenantSuppressionAttributes: {wire: ok, errors: ok, state: ok, persist: ok, route: ok, note: "new in aws-sdk-go-v2/service/sesv2 v1.66.0. Real path/verb confirmed against serializers.go: POST /v2/email/tenant/suppression -- note the *singular* 'tenant' top-level segment, a genuinely distinct path from the rest of this family's plural '/v2/email/tenants/...' (awsRestjson1_serializeOpPutTenantSuppressionAttributes's httpbinding.SplitURI; this service has a history of invented paths -- verified by grepping serializers.go directly rather than assuming it lives under 'tenants'). SuppressedReasons entries validated against the real SuppressionListReason enum (BOUNCE/COMPLAINT); SuppressionScope against SuppressionListScope (ACCOUNT/TENANT); NotFoundException for an unknown TenantName. Writes onto the existing per-tenant map entry in b.tenants (SuppressedReasons/SuppressionScope keys) -- no parallel store -- so it's cascade-deleted for free when DeleteTenant removes the tenant's map entry. Surfaced via CreateTenant/GetTenant's SuppressionAttributes field (types.TenantSuppressionAttributes; added tenantSuppressionAttributesOutput + toTenantSuppressionAttributesOutput, wire_output.go), which was previously entirely missing from tenantOutput."}
# Families audited as a group (when per-op is impractical):
families:
  route-matcher: {status: fixed, note: "Built a full (method,path)->op regression matrix from aws-sdk-go-v2/service/sesv2 v1.60.1 serializers.go (services/sesv2/route_matrix_test.go, 110+ real routes, every real SDK route now covered -- see route_matrix_test.go). Original pass fixed 12/30 unroutable-or-misrouted routes; this pass closed the remaining 18: RPC-style tenant/resource-tenant paths (8 routes), deliverability-dashboard sub-resources (5 routes: test-reports x2, statistics-report, campaigns, domains/.../campaigns), insights/recommendations (3: email-address-insights, insights/{MessageId}, vdm/recommendations), reputation-entity listing (1, plus deletion of a gopherstack-invented duplicate 'reputation-entities' top-level path), and the POST-based list-export-jobs/import-jobs/list variants (2). gopherstack-jqh2: independently re-extracted all 112 real ops' method+path from the pinned sesv2@v1.66.4 serializers.go (no manual reliance on this file's prior citation) and diffed against ExtractOperation directly -- 112/112 match, confirming route_matrix_test.go is current and this family's 'fixed' status holds against the pinned SDK version; no new test added since route_matrix_test.go already covers this exact ground (including the 2 ops -- PutAccountPricingAttributes, PutTenantSuppressionAttributes -- that appeared between v1.60.1 and v1.66.4) and duplicating it would just be two tables to keep in sync. No query-flag-discriminated ops, no duplicate op-resolution table, no wrong-date-prefix paths found in this pass either."}
leaks: {status: clean, note: "no goroutines/janitors spawned; email retention capped at maxRetainedEmails (10000, FIFO-compacted) so SendEmail/SendCustomVerificationEmail can't leak memory on a long-running instance. DeleteTenant now cascades its resource-association index cleanup (both tenantResources and resourceTenants maps) so deleting a tenant with associated resources doesn't leave ghost rows."}
---

## This pass (2026-08-29): pagination-arithmetic sweep

Distinct from the filter/pagination *parameter* sweep directly below (that
pass measured whether a param was read/honoured at all; this one measured
the arithmetic of the cursor itself, once honoured). Census: every List op
in this service is `pkgs/page.New` (safe by construction -- the one
`start >= len(all)` guard `pkgs/page` has that a hand-rolled equivalent
would need to remember) except five (`ListDomainDeliverabilityCampaigns`,
`ListMultiRegionEndpoints`, `ListTenants`, `ListResourceTenants`,
`ListTenantResources`) that instead call one shared local helper,
`paginateMaps` (store.go), which does its own equality-matched cursor scan
over `[]map[string]any`.

**Bug found (Class B — infinite loop):** `paginateMaps` searched linearly
for the item whose `keyName` field equalled `nextToken` and left `start` at
its zero value on a miss. A client whose cursor named a since-deleted
tenant/endpoint/campaign got page one back forever, with the same NextToken
echoed back, and never terminated. All 5 call sites already sort their
input before calling `paginateMaps` (`sortMapsByStringKey`, or -- for
`ListDomainDeliverabilityCampaigns` -- campaigns in `b.emails`'s stable
append order), so this was the only bug in the shape; no "unsorted
collection" issue here as quicksight had.

Fixed by defaulting `start` to `len(all)` on a miss instead of 0 -- one
change in `store.go` fixes all 5 operations at once, since they share the
helper. New test `pagination_arithmetic_test.go`
(`TestListTenantsPaginationStaleCursor`) drives `ListTenants` with a cursor
naming a tenant that was never created (equivalent to "since deleted") and
fails against the pre-fix code (returned tenant-00 again instead of an
empty page); a boundary-walk and final-page/empty test are included too.
Confirmed through the real typed client (`aws sesv2 create-tenant` x3,
`list-tenants --page-size 2`, `delete-tenant` + re-list with the deleted
tenant's stale token -> empty page with no NextToken, not page one again).

## This pass (2026-08-29): filter/pagination parameter sweep

Measured every collection-returning op (verified from each op's Output shape
in the pinned SDK, not from its name -- `Get*` ops that return a single
resource were excluded) against its own declared constraining parameters
(filters, status/type selectors, page size, cursor). 18 List/Get ops declare
26 constraining parameters beyond NextToken across the family; 15 of those
26 were unhonoured before this pass.

Fixed (all confirmed against a real `aws-sdk-go-v2/service/sesv2` client
driving the handler, test file `list_filter_params_test.go`):
- `ListContacts`: `PageSize` (parsed struct never included it -- request
  fields covers NextToken only).
- `GetDedicatedIps`: `PoolName`, `NextToken`, `PageSize` -- the handler took
  no arguments at all and always returned every tracked IP on one page.
- `ListDedicatedIpPools`: `PageSize` (hardcoded to 0).
- `ListSuppressedDestinations`: `Reasons`, `StartDate`, `EndDate`,
  `PageSize` (only NextToken was read; the rest of this op's real
  query-string parameters were never parsed).
- `ListExportJobs`: `ExportSourceType`, `JobStatus` -- both already stored
  on `ExportJob`, but a stale handler comment claimed neither was "modelled
  by the backend yet" and neither was applied.
- `ListImportJobs`: `ImportDestinationType` -- derivable from the already-
  stored `ImportDestination` oneof branch; same stale-comment pattern as
  `ListExportJobs`.
- `ListReputationEntities`: pagination (the backend signature discarded
  `nextToken`/`pageSize` into blank identifiers `_, _`, so it always
  returned every entity on one page) and the `SENDING_STATUS`/
  `ENTITY_REFERENCE_PREFIX` filter keys (`Filter` was decoded by the
  handler and never even passed to the backend call).

Left unfixed, with reason (RESTRAINT -- no filter name/semantics invented):
- `ListContacts.Filter` (`FilteredStatus`/`TopicFilter`): `TopicFilter.
  UseDefaultIfPreferenceUnavailable` needs each topic's default
  subscription status, which `ContactList` doesn't model at all (no
  `Topics` field anywhere in this backend -- structural gap). The AWS doc
  for standalone `FilteredStatus` (no `TopicFilter`) doesn't say what it
  filters against, so nothing was invented for that case either.
- `ListSuppressedDestinations.TenantName`: `SuppressedDestination` has no
  per-tenant tracking and there is no separate per-tenant suppression-list
  store (only a per-tenant *reasons/scope config* exists, in
  `tenants.go`) -- structural gap.
- `ListReputationEntities.Filter["ENTITY_TYPE"]`: nothing in this backend
  ever assigns `ReputationEntity.EntityType` (grepped for `.EntityType =`
  -- zero hits), so it is always empty; filtering on it would be
  filtering against data that doesn't exist.
- `ListReputationEntities.Filter["REPUTATION_IMPACT"]`: no reputation-
  impact field on the model at all -- structural gap.
- `ListTenantResources`, `ListRecommendations`: already correctly wired
  (Filter/RESOURCE_TYPE and Filter/TYPE|STATUS|IMPACT|RESOURCE_ARN
  respectively, both applied and pagination honored) -- audited, no
  change needed.

Adjacent finding, not in this class: `ListExportJobs`/`ListImportJobs`'s own
handler comments ("ExportSourceType/JobStatus filters aren't modelled by the
backend yet", "ImportDestinationType filter isn't modelled by the backend
yet") were simply wrong -- the data existed the whole time. Comments in this
repo have caused bugs before (gopherstack-101r and others); these two are
new instances of the same failure mode.

Existing tests never set these parameters: the pre-fix `contacts_test.go`,
`export_jobs_test.go`, `import_jobs_test.go`, `suppression_test.go`, and
`deliverability_test.go` coverage for these ops asserted only that the call
succeeded and returned *some* data, never that a filter/PageSize/cursor
actually constrained the result -- none of them could have caught any of the
above.

## 2026-08-21: TrackingOptions.CustomRedirectDomain dropped when only HttpsPolicy is set (gopherstack-r80d batch 21)

`GetConfigurationSet`'s `TrackingOptions` wrapper mirrors
`aws-sdk-go-v2/service/sesv2/types/types.go`'s `TrackingOptions`, whose
`CustomRedirectDomain` is `This member is required.` `handler_configuration_
sets.go`'s `trackingOptionsOutput.CustomRedirectDomain` was tagged
`,omitempty`. `PutConfigurationSetTrackingOptionsInput`
(`api_op_PutConfigurationSetTrackingOptions.go`) only requires
`ConfigurationSetName` -- `CustomRedirectDomain` and `HttpsPolicy` are both
optional, independently settable -- so a real client can call
`PutConfigurationSetTrackingOptions({ConfigurationSetName, HttpsPolicy:
"REQUIRE"})` with no redirect domain at all.
`handleGetConfigurationSet`'s guard (`cs.TrackingCustomRedirectDomain != ""
|| cs.TrackingHTTPSPolicy != ""`) then built the wrapper because
`HttpsPolicy` was set, but the `omitempty` tag dropped the required
`CustomRedirectDomain` key entirely from the response body instead of
emitting it as an empty string -- the "required-but-inapplicable means
present-and-empty, not absent" shape this campaign has repeatedly named.
Fixed by dropping the `omitempty` tag (the wrapper's `HttpsPolicy` stays
`,omitempty`, correctly optional on the real type). Proven via a real
`aws-sdk-go-v2/service/sesv2` client round trip
(`wire_output_required_r80d_test.go`), hand-reverted/confirmed-failing/
restored, md5sum-verified byte-identical.

Selected as the largest remaining `gopherstack-r80d` candidate tied with
`cloudfrontkeyvaluestore` (both 18 required output fields per
`cmd/requiredoutputfields`); this service's real surface is much larger once
domain structs are walked (232 required fields across 143 structs in
`types/types.go` -- almost entirely `*Input` structs for this service's
large identity/configuration-set/contact CRUD surface, not response types).
Every other required output-relevant domain struct checked this pass came
back clean: `SuppressedDestination`/`SuppressedDestinationSummary.
LastUpdateTime` carries the same dead `omitempty` tag but is structurally
unreachable (the sole construction site, `PutSuppressedDestination`,
unconditionally stamps `time.Now()`); `DedicatedIp`/`DedicatedIpPool`/
`EventDestination`/`MailFromAttributes`/`BulkEmailEntryResult` are all built
either as non-nil map literals or with their required members always
populated before construction; `DeliverabilityTestReport`/`IspPlacement`/
`OverallVolume`/`DailyVolume`/`ContactList`/`VdmOptions`/`DashboardOptions`/
`GuardianOptions`/`ArchivingOptions`/`BlacklistEntry` all genuinely declare
zero required members on the pinned SDK, confirmed by reading `types.go`
directly rather than assuming from the struct's presence in this file's
domain model.

## This pass (2026-08-13): SendBulkEmail dropped DefaultContent (gopherstack-afi1)

From the "five ops drop the fields that define what they do" required-member
sweep. `send_email.go`'s `SendBulkEmail` called `SendEmail(from, to, "", "",
"")` for every entry -- `DefaultContent`, required
(`api_op_SendBulkEmail.go:43`), was never read, so every bulk email was
stored with an empty subject and body no matter what the caller sent. See
the `SendBulkEmail` `ops:` entry above for the full fix detail (template
content/name resolution, `{{var}}` substitution, per-entry override).
`TestSendBulkEmail` and `TestSendBulkEmailSDKRoundTrip` previously asserted
only `http.StatusOK`/non-empty `MessageId` -- neither checked the recorded
email's actual content, so both passed against the unfixed code exactly as
easily as against the fix; both now assert on `ListEmails()` content.
Added `TestSendBulkEmailRequiresDefaultContent` (missing `DefaultContent` ->
`BadRequestException`, matching `SendBulkEmail`'s declared error switch,
which has no dedicated validation exception).

## This pass (2026-08-12): CreateExportJob/CreateImportJob wire-shape fix (gopherstack-rcmn)

From the `gopherstack-7rq1` sweep for request fields present in the model but
absent from wire structs. Both ops read an invented flat `DataSource` string;
real required members are nested structs (`ExportDataSource`/
`ExportDestination`, `ImportDataSource`/`ImportDestination`) entirely absent
from the handlers. A body sending the invented flat field parsed identically
to one sending nothing, so gopherstack silently accepted structurally-wrong
requests -- see the `CreateExportJob`/`CreateImportJob` `ops:` entries above
for the full field-diff/validation/error-model detail. Fixed for real:
required members are now validated present (400 BadRequestException, each
op's own declared error switch -- confirmed neither declares
ValidationException), and the parts of each nested shape gopherstack can act
on (`ExportSourceType` derivation, `ImportDestination`'s selected branch) are
stored and echoed back via Get/List rather than merely validated and
discarded. Leaves genuinely inert where there's no backend engine behind them
(metrics/message-insights export contents, the unfetchable import file
itself) -- documented per-field in the `ops:` notes, not silently dropped.

## Notes

**Root-cause bug class (fixed in the original pass, ~15 ops):** most of the
"extended" GET/List handlers (contact lists, contacts, suppressed
destinations, dedicated IP pools, event destinations, email templates,
custom verification templates, export/import jobs) marshalled their
**internal storage struct** directly as the HTTP response. Those structs
intentionally keep `lowerCamelCase` JSON tags because they double as the
on-disk snapshot format (persistence.go) — but AWS JSON-protocol responses
need `PascalCase` field names, and several also need a `{Foo: {...}}`
wrapper the internal struct doesn't have (e.g. `GetDedicatedIpPool` →
`{"DedicatedIpPool": {...}}`, `GetSuppressedDestination` →
`{"SuppressedDestination": {...}}`). Fixed by adding a parallel set of
`*Output` wire DTOs + `to*Output()` converters in `wire_output.go`,
**without** touching the internal structs' tags (that would have required
bumping `sesv2SnapshotVersion` and losing old snapshots for no wire-format
benefit). `EmailIdentity`/`ConfigurationSet` already had proper wire DTOs
before that pass (`getEmailIdentityOutput`, `getConfigurationSetOutput`,
etc.) — only `ListConfigurationSets` had a residual bug there (see below).

**`ListConfigurationSets` wire bug:** `ConfigurationSetsOutput.ConfigurationSets`
is `[]string` in the real SDK (plain names), not a list of
`{"Name": "..."}` objects. Confirmed against
`awsRestjson1_deserializeDocumentConfigurationSetNameList` in the SDK's
deserializers.go, which type-asserts each array element directly to `string`
and would fail to decode gopherstack's previous `[{"Name":"foo"}]` shape.

**Route-matcher bug class (fixed across both passes; 30/30 real-SDK
route-matcher gaps now closed):** `parseSESv2Path` (handler.go/handler_routes.go)
had several sub-path segments that were plausible-looking guesses rather
than the real SDK's REST path templates, and a handful of ops (the tenant
family, `ListExportJobs`/`ListImportJobs`) are genuinely RPC-style — a fixed
POST verb path with identifying fields in the JSON body — where gopherstack
had guessed a REST-with-path-param shape instead. Confirmed against
`aws-sdk-go-v2/service/sesv2`'s `serializers.go`
(`httpbinding.SplitURI("...")` + `request.Method = "..."` in each
`awsRestjson1_serializeOp*` type). Original-pass fixes (Account attributes,
SuppressionList, `ListContacts`, `PutConfigurationSetSendingOptions`,
`PutDedicatedIpPoolScalingAttributes`, `SendCustomVerificationEmail`) are
unchanged from the prior audit. This pass's fixes:

- **Tenants / resource-tenants** (`CreateTenant`, `GetTenant`, `DeleteTenant`,
  `ListTenants`, `CreateTenantResourceAssociation`,
  `DeleteTenantResourceAssociation`, `ListResourceTenants`,
  `ListTenantResources`): rewrote `parseTenantPath` to the real RPC-style
  verb paths (`/tenants`, `/tenants/get`, `/tenants/delete`,
  `/tenants/list`, `/tenants/resources`, `/tenants/resources/delete`,
  `/tenants/resources/list`) and added the distinct top-level
  `/resources/tenants/list` route for `ListResourceTenants`. Rewrote every
  handler in `handler_tenants.go` to decode `TenantName`/`ResourceArn` from
  the JSON body instead of a path-derived `resource` string.
- **Deliverability dashboard sub-resources**: `test-reports[/{ReportId}]`
  (was `reports[/{id}]`) and `statistics-report/{Domain}` (was
  `statistics/{domain}`).
- **`GetDomainDeliverabilityCampaign` / `ListDomainDeliverabilityCampaigns`**:
  were actively *misrouted*, not just unreachable — gopherstack's
  `campaigns/{domain}/{id}` pattern meant a real GET to
  `campaigns/{CampaignId}` (2 segments) was misinterpreted as
  `ListDomainDeliverabilityCampaigns` with the campaign ID read as a domain.
  Real paths are `campaigns/{CampaignId}` (Get, no domain param at all —
  `GetDomainDeliverabilityCampaign`'s backend signature dropped the domain
  parameter to match) and `domains/{SubscribedDomain}/campaigns` (List).
- **`GetEmailAddressInsights`**: real op is
  `POST /v2/email/email-address-insights` with `EmailAddress` in the body;
  gopherstack had `GET /v2/email/email-insights/{email}`.
- **`GetMessageInsights`**: real path is `GET /v2/email/insights/{MessageId}`;
  gopherstack had `/v2/email/messages/{id}`.
- **`ListRecommendations`**: real op is `POST /v2/email/vdm/recommendations`;
  gopherstack had `GET /v2/email/recommendations`.
- **`ListReputationEntities`**: real op is `POST /v2/email/reputation/entities`
  (filter/pagination in body); gopherstack only accepted `GET` on that path.
  A gopherstack-invented duplicate top-level path,
  `/v2/email/reputation-entities/...` (with its own copy of Get/List/Update*
  routes), was found alongside the real `/v2/email/reputation/entities/...`
  family and **deleted** — the real family already covered every op
  correctly, so the duplicate was pure invented surface, not a fallback.
- **`ListExportJobs`** / **`ListImportJobs`**: real ops are
  `POST /v2/email/list-export-jobs` and `POST /v2/email/import-jobs/list`
  (filter/pagination in body); gopherstack had fabricated `GET
  /v2/email/export-jobs` and `GET /v2/email/import-jobs` routes that don't
  exist in the real API (the real `/v2/email/export-jobs` path only has
  POST for `CreateExportJob` and GET-with-id for `GetExportJob`).

`services/sesv2/route_matrix_test.go` (`Test_RouteMatrix_AgainstRealSDK`)
now has a case for every real SDK route covered above (110+ cases) — no
routes are omitted from the matrix anymore.

**Wire/state fixes this pass:**

- `PutDeliverabilityDashboardOption` was a true no-op; now persists
  `DashboardEnabled` and the subscribed-domain list
  (`b.deliverabilityDashboardEnabled`/`b.deliverabilityDashboardDomains`,
  wired into `Reset`/`Snapshot`/`Restore`) so `GetDeliverabilityDashboardOptions`
  reflects it, including deriving `AccountStatus` (ACTIVE/DISABLED).
- `MultiRegionEndpoint` family field-diffed against
  `CreateMultiRegionEndpointOutput`/`GetMultiRegionEndpointOutput`/
  `types.MultiRegionEndpoint`/`types.Route`: added `EndpointId` generation,
  `CreatedTimestamp`/`LastUpdatedTimestamp` (epoch), `Routes`
  (`[]{Region}`, one per region the endpoint spans), and
  `AlreadyExistsException`/`NotFoundException` handling that was previously
  entirely absent (Create silently overwrote, Delete silently no-opped on an
  unknown name).
- `Tenant` family field-diffed against `CreateTenantOutput`/`GetTenantOutput`/
  `types.Tenant`/`types.TenantInfo`/`types.ResourceTenantMetadata`/
  `types.TenantResource`: added `TenantId`, `TenantArn` (via `pkgs/arn`,
  not hand-formatted), `SendingStatus`, `CreatedTimestamp`, `Tags`; List
  item shapes correctly trimmed per-type (`TenantInfo` has no
  `SendingStatus`/`Tags`; `TenantResource` has no timestamp).
  `CreateTenantResourceAssociation` now checks the tenant exists
  (`NotFoundException` otherwise) and `DeleteTenant` cascades its
  resource-association index cleanup.
- `GetReputationEntity`/`ListReputationEntities` field-diffed against
  `types.ReputationEntity`: `ReputationEntityReference`/`ReputationEntityType`/
  `CustomerManagedStatus` (nested `{Status: ...}`, matching `*StatusRecord`)/
  `ReputationManagementPolicy` were already correct from the original pass;
  added `SendingStatusAggregate`.

## This pass (2026-07-25): real derivation for the analytics-placeholder ops + typed DTOs

Closed `gopherstack-03th`. Per-op judgment calls (derive-from-real-state vs
genuinely-impossible) are in each op's `note:` above; summary:

- **`BatchGetMetricData`**: SEND metric, no dimension or EMAIL_IDENTITY
  dimension only, is now real (aggregated from `b.emails`). Every other
  metric/dimension combination remains an honest zero-valued datapoint --
  gopherstack has no bounce/complaint/open/click/delivery pipeline and no
  per-email config-set/ISP association to derive them from, and a
  plausible-looking fabricated non-zero count would be strictly worse than
  the honest zero.
- **`GetDomainDeliverabilityCampaign`/`ListDomainDeliverabilityCampaigns`**:
  CampaignId/FromAddress/Subject/FirstSeenDateTime/LastSeenDateTime are now
  derived for real by grouping `b.emails` by `(FromAddress, Subject)` --
  gopherstack's own send history is exactly the data real SES's campaign
  auto-detection is built from, just without AWS's server-side production
  tracking. InboxCount/SpamCount/ReadRate/DeleteRate/ReadDeleteRate/
  ProjectedVolume/Esps/SendingIps remain honest zero/empty: inbox-vs-spam
  placement genuinely requires infrastructure gopherstack doesn't have and
  never will (real mailbox delivery-outcome tracking), so these stay
  documented placeholders rather than invented numbers.
- **`GetDomainStatisticsReport`**: fixed a real wire-shape gap (`DailyVolumes`
  was always `[]` regardless of the requested date range; real SES documents
  one entry per day in range) without fabricating the per-day statistics
  themselves, which have the same inbox/spam-placement-only-AWS-can-know
  problem as the campaign family -- there's no partial derivation available
  here the way there is for campaigns (this shape has no "raw send count"
  field, only inbox/spam splits).
- **`ListRecommendations`**: derives real DKIM/SPF/COMPLAINT recommendations
  from stored identity/reputation-entity configuration state (see the op's
  entry above). DMARC/BIMI and reputation-finding-driven types
  (BOUNCE/FEEDBACK_3P/IP_LISTING) are never returned, not even as
  placeholders -- gopherstack has no DNS-record model or
  bounce/complaint-rate pipeline, and there's no honest zero/empty value to
  report for "does this domain have a DMARC record" the way there is for a
  count.
- **`SendBulkEmail`**: request DTO (`bulkEmailEntry` et al, `send_email.go`)
  and per-entry result DTO (`bulkEmailEntryResultOutput`, `wire_output.go`)
  are now typed, field-diffed against `types.BulkEmailEntry`/
  `types.BulkEmailEntryResult`/`types.Destination`/`types.MessageHeader`/
  `types.MessageTag`/`types.ReplacementEmailContent`/
  `types.ReplacementTemplate`. Functional behavior unchanged (still records
  sent emails with real recipients via the existing `SendEmail` path).
- **`Tenant`/`MultiRegionEndpoint`/`GetReputationEntity`/
  `ListReputationEntities`** now return typed wire DTOs
  (`tenantOutput`/`tenantInfoOutput`/`resourceTenantOutput`/
  `tenantResourceOutput`/`multiRegionEndpointOutput`/
  `multiRegionEndpointSummaryOutput`/`createMultiRegionEndpointOutput`/
  `reputationEntityOutput`/`statusRecordOutput`, all in `wire_output.go`)
  instead of ad-hoc `map[string]any`. The underlying backend storage
  (`b.tenants`/`b.multiRegionEndpoints`: `map[string]map[string]any`) is
  **unchanged** -- those maps are still both the persisted snapshot format
  and an internal staging shape; the typed DTOs are a conversion step added
  at the response boundary (`toTenantOutput`/`toMultiRegionEndpointOutput`/
  etc.), so no `sesv2SnapshotVersion` bump was needed. All fields were
  already field-verified correct from the prior pass; this is a
  compile-time-safety upgrade, not a wire-correctness fix (except where
  individually noted `wire: fixed` above for the type change itself).

**Verification**: every op above is covered by a real
`aws-sdk-go-v2/service/sesv2` client round-trip test (not just a decoded
`map[string]any` or backend-struct assertion) -- see
`newSESv2SDKClient` (`store_test.go`) and the `*SDKRoundTrip` tests in
`tenants_test.go`, `multi_region_endpoints_test.go`, `deliverability_test.go`,
`message_insights_test.go`, `send_email_test.go`.

## This pass (parity-4, SDK bump to v1.66.0): 2 new ops + a missed GetAccount wire bug

The Go SDK modules were bumped (`aws-sdk-go-v2/service/sesv2` v1.60.1 ->
v1.66.0), which shipped 2 new operations `TestSDKCompleteness` caught:
`PutAccountPricingAttributes` and `PutTenantSuppressionAttributes`. Both are
now implemented for real (not added to a `notImplemented` skip list) -- see
their `ops:` entries above for the full field-diff/route/state detail.
Summary:

- **`PutAccountPricingAttributes`**: `PUT /v2/email/account/pricing-attributes`,
  `{Plan}` body, validated against the real `PricingPlan` enum. Writes the
  existing `b.accountDetails` account state (no parallel store); no
  billing-cycle concept, so `PricingAttributes.NextPlan` is always empty
  rather than a fabricated "scheduled" value.
- **`PutTenantSuppressionAttributes`**: `POST /v2/email/tenant/suppression`
  -- confirmed directly against `serializers.go` rather than assumed, since
  this family (like the rest of sesv2's tenant paths, per the original
  route-matcher pass) turned out to use a **singular** `tenant` top-level
  segment, distinct from every other tenant op's plural `tenants`. Writes
  onto the tenant's existing `b.tenants[name]` map entry (no parallel store),
  so it's cascade-deleted for free by the existing `DeleteTenant` cleanup.
  Surfaced through `CreateTenant`/`GetTenant`'s `SuppressionAttributes` field,
  which `tenantOutput` didn't expose before this pass.
- **`GetAccount` wire-shape bug found while wiring `PutAccountPricingAttributes`
  in**: the handler marshalled the internal `*AccountDetails` struct directly
  -- the exact `lowerCamelCase`-tags-leaking-into-the-response bug class the
  original audit pass fixed for every *other* family in this file (contact
  lists, dedicated IP pools, templates, etc.), but missed for `Account`
  itself, and the op was incorrectly graded `wire: ok` as a result. Fixed the
  same way as every other family: added `accountOutput` +
  `accountDetailsOutput`/`accountSuppressionAttributesOutput`/
  `accountPricingAttributesOutput` DTOs in `wire_output.go`, field-diffed
  against `GetAccountOutput`/`types.AccountDetails`/
  `types.SuppressionAttributes`/`types.PricingAttributes`. Fields gopherstack
  has no data source for (`EnforcementStatus`, `ProductionAccessEnabled`,
  `SendQuota`, `ReviewDetails`, suppression `ValidationAttributes`) are
  omitted -- all pointer/optional in the real shape -- rather than
  fabricated. `AccountDetails`'s internal `lowerCamelCase` snapshot-format
  tags are unchanged (same "don't touch persisted tags" rule as every other
  family; see "Traps for the next auditor").

**Verification**: `TestAccountSDKRoundTrip` (`account_test.go`) and
`TestPutTenantSuppressionAttributesSDKRoundTrip` (`tenants_test.go`) drive
both new ops (plus the fixed `GetAccount`/`GetTenant` responses) through the
real `aws-sdk-go-v2/service/sesv2` client, not just decoded JSON maps.
`route_matrix_test.go` gained both new routes.

## Remaining known limitation (not a gap — reachable, correctly routed, AWS-accurate shape)

- `BatchGetMetricData` returns real SEND counts for the SEND/no-dimension and
  SEND/EMAIL_IDENTITY-dimension cases; every other metric/dimension
  combination returns one zero-valued datapoint per query rather than real
  aggregated metrics — gopherstack has no metrics aggregation engine to
  source those specific values from. Envelope shape (`Results: [{Id,
  Timestamps, Values}]`) is correct.
- `GetDomainDeliverabilityCampaign`/`ListDomainDeliverabilityCampaigns`
  derive real campaign identity/timing from send history (see "This pass"
  above); `GetDomainStatisticsReport`'s per-day/overall statistics are
  zero-valued placeholders. All three require either opted-in-and-AWS-tracked
  production inbox/spam-placement data or a reputation findings engine
  gopherstack doesn't have for the fields that remain placeholder.
- `GetEmailAddressInsights`: `HasValidSyntax` (regex) and `IsRoleAddress`
  (local-part lookup against common role names) are real checks;
  `HasValidDnsRecords`/`IsDisposable`/`IsRandomInput`/`MailboxExists` are
  honest `MEDIUM`-confidence placeholders (no DNS/disposable-domain-list/
  mailbox-probing data source in an emulator).
- `ListRecommendations` derives real DKIM/SPF/COMPLAINT recommendations from
  stored configuration state (see "This pass" above); DMARC/BIMI and
  reputation-finding-driven types are never returned (no DNS-record model,
  no bounce/complaint-rate pipeline).
- SDK-driven integration test coverage (`test/integration/*_parity_test.go`)
  has not been run for this service — this and prior passes added a
  route/path regression test (`route_matrix_test.go`) and real-SDK-client
  round-trip unit tests (see "This pass" above); no `make build-linux` +
  Docker-based integration run was performed.

## Traps for the next auditor

- `EmailIdentity`/`ConfigurationSet`/`Tags` families already had correct
  PascalCase wire DTOs before the original pass (`getEmailIdentityOutput`,
  `dkimAttributesOutput`, `getConfigurationSetOutput`, `tagEntry`, etc.) —
  don't re-flag those as bugs; only `ListConfigurationSets`'
  `ConfigurationSets` field type was wrong.
- Don't "fix" the internal model structs' `lowerCamelCase` JSON tags
  (`EmailIdentity`, `ConfigurationSet`, `ContactList`, `Contact`,
  `SuppressedDestination`, `EmailTemplate`, `DedicatedIPPool`,
  `EventDestination`, etc. in backend.go/backend_ops.go) to fix wire output —
  those tags are the **persisted snapshot format** (persistence.go). Add a
  wire DTO in `wire_output.go` instead; changing the internal tags would
  require bumping `sesv2SnapshotVersion` and silently discarding every
  existing snapshot on upgrade for no wire benefit.
- The `tenants`/`multiRegionEndpoints` backend fields are still intentionally
  `map[string]map[string]any` with PascalCase keys (e.g. `keyTenantName =
  "TenantName"`) rather than typed structs — unlike `EmailIdentity`/
  `ConfigurationSet`/etc., these maps store the **wire-shaped** data
  directly, so adding a field there is adding it to both the snapshot and
  the eventual response in one place (build the map in `tenants.go`/
  `multi_region_endpoints.go` as before). **As of this pass, though, every
  backend method that used to *return* one of these maps directly now
  converts it through a typed `wire_output.go` DTO first**
  (`toTenantOutput`/`toMultiRegionEndpointOutput`/etc.) — don't reintroduce a
  bare `map[string]any` return type on a tenant/multi-region-endpoint/
  reputation-entity op; add fields to the existing DTO struct + its
  `to*Output` converter instead. `GetReputationEntity`/
  `ListReputationEntities` are simpler: their internal storage
  (`b.reputationEntities`, a `*store.Table[ReputationEntity]`) was already a
  typed struct — only the response conversion (`reputationEntityToMap` →
  `toReputationEntityOutput`) changed.
- `campaignIDFor`/`domainCampaignsLocked` (deliverability.go) derive
  deliverability-dashboard "campaigns" as a **pure function** of `b.emails`
  (grouped by `(FromAddress, Subject)`, hashed to a stable ID) — there is no
  separate campaign index/table and none should be added; a real campaign
  index would need its own persisted ID-generation state to stay stable
  across restarts, which the hash-of-(From,Subject) approach gets for free
  without touching `persistence.go`. Don't "fix" `GetDomainDeliverabilityCampaign`
  to return `NotFoundException` for an unrecognized `CampaignId` —
  `TestGetDomainDeliverabilityCampaignFields` (deliverability_test.go)
  documents that gopherstack has no way to distinguish a caller-guessed ID
  from a legitimately-issued one, so it echoes the placeholder shape instead
  (see that op's PARITY note above).
- `parseSESv2Timestamp` (deliverability.go) accepts both RFC3339 (the real
  wire format for the `GetDomainStatisticsReport`/
  `ListDomainDeliverabilityCampaigns` query-string `StartDate`/`EndDate`
  params, confirmed against serializers.go's `smithytime.FormatDateTime`)
  and a bare `YYYY-MM-DD` date (for backend-direct callers/tests). The
  `BatchGetMetricData` JSON-body `StartDate`/`EndDate` fields use a
  *different* wire format (epoch-seconds numbers, `smithytime.FormatEpochSeconds`)
  and are decoded separately via `epochSecondsToTime`
  (handler_message_insights.go) — don't conflate the two parsers.
- `route_matrix_test.go`'s case table now covers every real SDK route this
  service routes to (110+ cases, including the RPC-style tenant/
  resource-tenant paths and deliverability-dashboard sub-resources added
  this pass) — if you add a new op, add its route(s) here too.
- `newSESv2SDKClient` (`store_test.go`) stands up the real
  `aws-sdk-go-v2/service/sesv2` client against an in-process `httptest`
  server running the handler through the same `pkgs/service` router used in
  production — prefer it over hand-decoded `map[string]any` response
  assertions for any new DTO-conversion test; it's what actually proves wire
  compatibility (see the `*SDKRoundTrip` tests added this pass for the
  pattern).
- `PutTenantSuppressionAttributes` lives under the **singular**
  `/v2/email/tenant/suppression` (`parseTenantSuppressionPath`,
  `handler_routes.go`) — not `/v2/email/tenants/...` like the rest of the
  tenant family. Confirmed directly against
  `awsRestjson1_serializeOpPutTenantSuppressionAttributes`'s
  `httpbinding.SplitURI` call in `serializers.go`; don't "fix" it to the
  plural without re-checking the serializer, and don't assume any *other*
  newly-added op's path follows the family it looks like it belongs to
  without the same check — this service has a specific history of invented
  paths (see `68b00b120`'s route-matcher rewrite and the original audit
  pass's "Route-matcher bug class" above).
- `GetAccount` (`handler_account.go`/`wire_output.go`) was the one family the
  original wire-DTO pass missed — every *other* family already got the
  internal-struct-vs-typed-DTO treatment documented in "Root-cause bug
  class" above, but `AccountDetails` slipped through and was incorrectly
  graded `wire: ok`. If you find another family still returning an internal
  struct/map directly (grep handler_*.go for a `return acct, nil`-shaped
  line with no `to*Output(...)` wrapper), it's the same bug, not a new one —
  add a DTO in `wire_output.go` the same way, don't assume `overall: A`
  means every individual op was actually wire-checked.

- **2026-08-29 error-path sweep**: protocol re-confirmed REST-JSON
  (`awsRestjson1_*` serializer prefix) before relying on it, per this
  campaign's standing rule that briefs get protocol wrong often enough to be
  worth re-checking. All 112 `awsRestjson1_deserializeOpError*` functions
  extracted from `sesv2@v1.66.4/deserializers.go` (matching the 112
  dispatch-table ops), none modeling zero typed exceptions -- every op models
  at least `BadRequestException`/`TooManyRequestsException`. Wire mechanism:
  a single service-wide `sentinel -> (wireType, httpStatus)` switch
  (`handler.go`'s `handleOpError`), same shape as the shared-table pattern
  this campaign has found elsewhere -- correct in aggregate, with the bug
  living at specific call sites rather than the table itself.

  **Two confirmed bugs found and fixed, both on the same code path**
  (`checkFromIdentityLocked` in `send_email.go`) -- see the `SendEmail`/
  `SendBulkEmail` `ops:` notes above for full citations:
  1. `SendEmail`: wrong-sentinel bug -- raised the generic
     `BadRequestException` for an unverified From identity where the op's
     own deserializer models the dedicated
     `MailFromDomainNotVerifiedException`.
  2. `SendBulkEmail`: missing-error bug -- silently discarded the identical
     per-entry error (`msgID, _ := b.SendEmail(...)`) and always reported
     `Status: SUCCESS`; real AWS reports `MAIL_FROM_DOMAIN_NOT_VERIFIED` per
     entry (a `BulkEmailStatus` enum value, not a top-level exception, since
     verification is evaluated once for the shared From address but surfaced
     per recipient result).

  No prior test exercised the unverified-identity path for either op (a gap,
  not a wrong test) -- both new tests (`TestSendEmail_UnverifiedIdentity`,
  `TestSendBulkEmail_UnverifiedIdentity`) drive the real SDK client and
  failed against the pre-fix code before the fix landed.

  **Left unimplemented, not fixed (feature gaps)**: this service has no
  sentinel at all for `ConcurrentModificationException` (modeled on ~15 ops:
  every `Delete*`/`Update*` on configuration sets, contacts, contact lists,
  dedicated IP pools, email identities, multi-region endpoints, tenants,
  `TagResource`/`UntagResource`), `LimitExceededException` (quota-shaped, ~15
  ops), `ConflictException` (`PutAccountDetails`/`PutAccountPricingAttributes`/
  `UpdateReputationEntity*` -- distinct from the `AlreadyExistsException`
  sentinel this service already has), `AccountSuspendedException`, and
  `SendingPausedException`. None have corresponding backend logic (no
  optimistic-concurrency versioning, no quota tracking, no account-suspension
  or sending-pause simulation) to ever raise them, so implementing any would
  mean adding new business-logic simulation from scratch, not fixing a wrong
  sentinel -- out of scope for a sentinel-correctness pass.

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `Ip`/`IP` acronym casing
(dedicated-IP pool operations) gives it 9 op/handler pairs needing the
ambiguous fold, all genuine collisions between an exported backend method
and the real unexported handler: `CreateDedicatedIpPool`,
`DeleteDedicatedIpPool`, `GetDedicatedIp`, `GetDedicatedIpPool`,
`ListDedicatedIpPools`, `PutAccountDedicatedIpWarmupAttributes`,
`PutDedicatedIpInPool`, `PutDedicatedIpPoolScalingAttributes`,
`PutDedicatedIpWarmupAttributes`.

Verified directly: ran the unpatched tool from `ef0eef041~1` five times and
diffed against the fixed tool at HEAD. `cmd/reqfieldscan` was byte-identical
across all 5 runs and HEAD -- zero damage. `cmd/reqfielddiff`'s finding
COUNT matched exactly (209 both), but 3 fields flickered inconsistently
run to run rather than settling: `PutDedicatedIpInPool.Ip`,
`PutDedicatedIpWarmupAttributes.{Ip, WarmupPercentage}`.

Built an instrumented copy of the unpatched tool (scratch space, discarded,
`cmd/` itself untouched) to see the actual winning candidate per run: when
old resolution nondeterministically picked the exported
`PutDedicatedIPInPool` backend method instead of the real handler, an
unrelated coincidental signal produced an `ip`-named field that happened to
suppress the finding on some runs -- not a signal this tool's decode
detection is actually designed to recognise. Read the real source
(handler_dedicated_ips.go:37-71, handler_dispatch.go:181-187): `Ip` is a URL
path segment threaded into the handler as a plain function parameter (never
read via a body-decode call this tool's signals cover), and
`WarmupPercentage` IS decoded, via `json.NewDecoder(...).Decode(&in)` -- a
call this tool's `decodeCallVerbs` list doesn't match (only
`Unmarshal`/`Bind`/`ReadJSON`). Confirmed with the debug instrumentation
that BOTH the correct handler and its exported namesake resolve to
`HasSignal=false` here regardless of which wins the fold -- this finding is
a standing, pre-existing `reqfielddiff` blind spot, unrelated to and
unaffected by the collision defect itself.

Verdict: zero real bugs. Both `Ip` and `WarmupPercentage` are genuinely
handled; the flicker is a coincidental interaction between the collision
defect and this tool's separate `Decode()`/path-param-as-argument blind
spot, not a service bug.
