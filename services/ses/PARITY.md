---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: ses
sdk_module: aws-sdk-go-v2/service/ses@v1.37.4   # version audited against (query-XML, 2010-12-01); verified == go.mod this pass
last_audit_commit: 44a1f8a1c                      # HEAD at audit time; a40e7cc1 (prior value) is not an
                       # ancestor of this branch -- confirmed via `git merge-base --is-ancestor`, it predates
                       # a squash-history rewrite. Re-audit protocol used dates instead: diffed every commit
                       # touching services/ses/ after last_audit_date (b8484292f, c78177958) by inspection.
last_audit_date: 2026-09-05                       # gopherstack wrapper-key/constraint sweep: 4 fixes below
                       # (ListTemplates default page size, DescribeConfigurationSet attribute gating,
                       # ListCustomVerificationEmailTemplates + ListReceiptRuleSets pagination never
                       # plumbed through the call chain at all) -- see the four rows' notes.
overall: A            # gopherstack-mhnk pass fixed: (1) GetSendStatistics Bounces/Complaints were hardcoded
                       # zero even though the AWS mailbox simulator addresses are a real, documented,
                       # deterministic trigger AWS publishes -- now genuinely reachable; (2) NotificationType
                       # enum unvalidated on Set{IdentityHeadersInNotificationsEnabled,IdentityNotificationTopic}
                       # (silent no-op + 200 for garbage input); (3) EventDestination.MatchingEventTypes
                       # (required member, 8-value enum) completely unvalidated on Create/Update; (4)
                       # ListIdentities IdentityType filter parsed by nothing, always returned every identity.
                       # LimitExceeded/MailFromDomainNotVerified/MaxSendRate/FromArn/TemplateArn re-investigated
                       # per campaign task and confirmed correctly gapped, not silently broken -- see gaps below
                       # for the specific evidence per item (this pass added citations, changed no behavior there).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  PutIdentityPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed prior pass; this pass added Policy JSON-validity check -> InvalidPolicy (ErrInvalidPolicy), matching real AWS InvalidPolicyException code — was previously accepted unvalidated (no wire error existed for malformed Policy at all)"}
  DeleteIdentityPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed this pass"}
  SetIdentityDkimEnabled: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed this pass"}
  SetIdentityFeedbackForwardingEnabled: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed this pass"}
  SetIdentityHeadersInNotificationsEnabled: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed prior pass; gopherstack-mhnk pass added NotificationType enum validation (Bounce/Complaint/Delivery only, matching aws-sdk-go-v2/service/ses/types/enums.go NotificationType) -> ValidationError (ErrValidation, previously declared but dead code, now wired into sesErrorCode) — an unrecognised NotificationType previously silently no-op'd and returned 200"}
  SetIdentityMailFromDomain: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed + BehaviorOnMXFailure now accepted/validated/persisted/returned (was silently dropped)"}
  SetIdentityNotificationTopic: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed prior pass; gopherstack-mhnk pass added the same NotificationType enum validation as SetIdentityHeadersInNotificationsEnabled (see its note) -- same silent-no-op-then-200 bug"}
  UpdateReceiptRule: {wire: ok, errors: partial, state: ok, persist: ok, note: "wire fixed prior pass; gopherstack-6xj6 (2026-09-08) added the same required-member validation as CreateReceiptRule (see its note -- both ops share validateReceiptAction/hasPrefixedKey), same errors: partial reasoning (InvalidSnsTopicException/InvalidS3ConfigurationException/InvalidLambdaFunctionException still gapped, cross-service existence checks out of scope)"}
  ReorderReceiptRuleSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed this pass"}
  SetReceiptRulePosition: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed this pass"}
  PutConfigurationSetDeliveryOptions: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed this pass"}
  UpdateConfigurationSetEventDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed prior pass; gopherstack-mhnk pass added MatchingEventTypes validation (see CreateConfigurationSetEventDestination note — same fix applied to both ops, they share validateMatchingEventTypes)"}
  UpdateConfigurationSetTrackingOptions: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire fixed this pass"}
  VerifyEmailAddress: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed zero-member output shape — no Result wrapper, verified against SDK deserializer (body discarded)"}
  DeleteVerifiedEmailAddress: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAccountSendingEnabled: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCustomVerificationEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateConfigurationSetReputationMetricsEnabled: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateConfigurationSetSendingEnabled: {wire: ok, errors: ok, state: ok, persist: ok}
  SendEmail: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior pass added AccountSendingPausedException + 24h-quota + ConfigurationSetDoesNotExist enforcement; this pass added the ReturnPathArn input member (SendEmailInput.ReturnPathArn), which was silently dropped -- now captured on the stored Email record like the sibling SourceArn/ReturnPath members"}
  SendRawEmail: {wire: ok, errors: ok, state: ok, persist: ok, note: "delegates to SendEmail, inherits the same fixes; this pass added ReturnPathArn parsing (SendRawEmailInput.ReturnPathArn, confirmed real member via api_op_SendRawEmail.go). gopherstack-x0sl (2026-08-13): fixed -- Destinations (wire key \"Destinations.member.N\", serializers.go:6682-6684 / AddressList array encoding at 4982-4990) was parsed nowhere; recipients came only from the raw message's To: header, silently dropping Bcc-only recipients (Destinations is the documented mechanism for delivering to addresses deliberately absent from the headers, which is exactly how Bcc works). Now: Destinations, when present, is the actual envelope SES delivers to and takes precedence over the headers; each address is classified To/Cc/Bcc by whether it's visible in the raw message's To/Cc header, with anything not visible landing in Bcc. Cc is now also parsed from the header (previously only To was) as a direct consequence. SendRawEmailInput.FromArn remains unhandled -- see gaps"}
  SendTemplatedEmail: {wire: ok, errors: ok, state: ok, persist: ok, note: "same enforcement added as SendEmail; this pass added ReturnPathArn (see SendEmail note)"}
  SendBulkTemplatedEmail: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior pass fixed ConfigurationSetName/ReplyToAddresses/ReturnPath/SourceArn. This pass: (1) added ReturnPathArn; (2) added DefaultTags and per-destination BulkEmailDestination.ReplacementTags, both real SendBulkTemplatedEmailInput members that were entirely unparsed by the handler (Tags on bulk-send silently vanished) -- ReplacementTags overrides (not merges with) DefaultTags per destination; (3) refactored the backend method's 8-positional-argument signature into SendBulkTemplatedEmailInput (struct, mirrors SendEmailInput/SendTemplatedEmailInput) so future members don't grow the param list further. TemplateArn remains unhandled -- see gaps."}
  SendBounce: {wire: ok, errors: ok, state: ok, persist: ok, note: "was a disguised stub (no field validation, no sender-verification check, deterministic fabricated MessageId); now validates BounceSender + BouncedRecipientInfoList as required (matching SendBounceInput), enforces sender verification, real unique MessageId"}
  SendCustomVerificationEmail: {wire: ok, errors: ok, state: ok, persist: ok, note: "was a disguised stub (never checked template existed, never registered/verified the identity, fabricated deterministic MessageId); now validates template + optional ConfigurationSetName, registers+verifies the identity, real unique MessageId"}
  VerifyEmailIdentity: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteIdentity: {wire: ok, errors: ok, state: ok, persist: ok}
  ListIdentities: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-mhnk pass: IdentityType (EmailAddress/Domain) filter param, confirmed a real optional ListIdentitiesInput member (aws-sdk-go-v2/service/ses/api_op_ListIdentities.go), was parsed by nothing -- silently ignored, always returning every identity. Fixed: filter applied before pagination (matching AWS's NextToken-continuation contract, which requires re-supplying the same IdentityType); an unrecognised IdentityType value now returns ValidationError. Backend signature changed (ListIdentities gained an identityType string param) -- StorageBackend interface and ~7 test call sites updated; no root-package callers exist."}
  GetIdentityVerificationAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "entry/key/value map shape verified against SDK deserializer"}
  GetIdentityDkimAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  GetIdentityMailFromDomainAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "BehaviorOnMXFailure now actually reflects the persisted value. gopherstack-r80d batch 24 (2026-08-21): FIXED -- required *string member MailFromDomain (types.IdentityMailFromDomainAttributes, ses@v1.37.4 types/types.go:557-577) was tagged xml omitempty; an identity with no custom MailFrom domain configured (the default state, before SetIdentityMailFromDomain is ever called) decoded it as a nil pointer on a real client instead of a pointer to \"\". BehaviorOnMXFailure's own omitempty was also removed as harmless cleanup (non-pointer enum on the real type, so omitted vs present-empty decode identically -- not a distinguishable bug, unlike MailFromDomain)."}
  GetIdentityNotificationAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-r80d batch 24 (2026-08-21): FIXED -- required *string members BounceTopic/ComplaintTopic/DeliveryTopic (types.IdentityNotificationAttributes, ses@v1.37.4 types/types.go) were tagged xml omitempty; an identity with no SNS topic configured for a given notification type (the default state before SetIdentityNotificationTopic is called) decoded each as a nil pointer instead of a pointer to \"\". Also fixed, not part of this cut (HeadersIn* are optional, not required): the xmlNotificationAttributes.HeadersInBounce/HeadersInComplaint/HeadersInDelivery XML tags didn't match the real deserializer's key names (HeadersInBounceNotificationsEnabled/HeadersInComplaintNotificationsEnabled/HeadersInDeliveryNotificationsEnabled, deserializers.go's IdentityNotificationAttributes case-switch) -- these three were always silently dropped by a real client regardless of value, now correctly keyed."}
  GetIdentityPolicies: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-05 pass: PolicyNames is a required member (api_op_GetIdentityPolicies.go: \"This member is required\"; client-side validateOpGetIdentityPoliciesInput also enforces v.PolicyNames != nil) -- gopherstack instead treated an absent/empty PolicyNames as \"return every policy for this identity\", a fabricated mode the real op doesn't have (its own doc even directs callers who don't know the names to call ListIdentityPolicies first). Fixed: empty/nil PolicyNames now returns InvalidParameterValue (ErrInvalidParameter, same convention as this op's own \"Identity is required\" check). Confirmed safe against errtargetaudit: GetIdentityPolicies's own deserializeOpError switch declares only the default case (no typed exceptions at all), so any wire code is passed through as a generic smithy.GenericAPIError verbatim -- unlike the DeleteReceiptRule-class bug, there's no risk of colliding with a real typed exception this op does declare."}
  ListIdentityPolicies: {wire: ok, errors: ok, state: ok, persist: ok}
  VerifyDomainIdentity: {wire: ok, errors: ok, state: ok, persist: ok}
  VerifyDomainDkim: {wire: ok, errors: ok, state: ok, persist: ok, note: "deterministic tokens per identity, stable across calls"}
  ListVerifiedEmailAddresses: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccountSendingEnabled: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSendQuota: {wire: ok, errors: ok, state: ok, persist: ok, note: "SentLast24Hours logic factored into sentLast24HoursLocked(), now also used to enforce the quota on send ops"}
  GetSendStatistics: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-mhnk pass: DeliveryAttempts was already accurate (real sends), but Bounces/Complaints were HARDCODED zero even for sends that would deterministically bounce/complain per real AWS's documented mailbox simulator (bounce@/complaint@/suppressionlist@/success@simulator.amazonses.com — https://docs.aws.amazon.com/ses/latest/dg/send-an-email-from-console.html#send-email-simulator). Fixed: SendEmail/SendTemplatedEmail (and SendRawEmail/SendBulkTemplatedEmail, which delegate to them) now classify recipients against these addresses and set Email.Bounced/Email.Complained; GetSendStatistics buckets these into real Bounces/Complaints counts. Rejects remains 0 — no content/virus-scanning concept exists anywhere in this backend, no documented deterministic trigger exists for it (unlike Bounces/Complaints), so it is left honestly at 0 rather than fabricated."}
  CreateTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTemplates: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack wrapper-key sweep, 2026-08-29): MaxItems defaulted to sesDefaultMaxItems (100) when absent -- real ListTemplatesInput.MaxItems documents 10 as its default (own doc comment, api_op_ListTemplates.go) and a distinct 100 cap for oversized requests, both now enforced via listTemplatesDefaultMaxItems/listTemplatesMaxItemsCap (templates.go)."}
  DeleteTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  TestRenderTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "{{key}} substitution against real stored template parts"}
  CreateConfigurationSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConfigurationSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades event destinations + tracking options"}
  ListConfigurationSets: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConfigurationSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack wrapper-key sweep, 2026-08-29): ConfigurationSetAttributeNames (api_op_DescribeConfigurationSet.go: 'A list of configuration set attributes to return') was read by nothing -- EventDestinations/TrackingOptions/DeliveryOptions/ReputationOptions were unconditionally included regardless of what was requested, matching real AWS SES behavior of only returning the attribute groups named in the request. Now gated via parseSESMemberList (handler_configuration_sets.go)."}
  CreateConfigurationSetEventDestination: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-mhnk pass: EventDestination.MatchingEventTypes is a required member restricted to the 8-value EventType enum (send/reject/bounce/complaint/delivery/open/click/renderingFailure — confirmed required via botocore ses/2010-12-01 service-2.json EventDestination.required, and the enum via aws-sdk-go-v2/service/ses/types/enums.go EventType), but was completely unvalidated: absent, empty, wrong-case (\"Send\"), or nonsense values all succeeded. Added validateMatchingEventTypes (shared with UpdateConfigurationSetEventDestination) -> InvalidParameterValue. Several existing tests/fixtures across this service used capitalized event-type strings (\"Send\"/\"Bounce\") that no real AWS client would ever send (the wire enum is lowercase-only) or omitted MatchingEventTypes entirely; all were corrected to real lowercase values rather than the validation being loosened to accommodate them."}
  DeleteConfigurationSetEventDestination: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateConfigurationSetTrackingOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConfigurationSetTrackingOptions: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCustomVerificationEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteCustomVerificationEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCustomVerificationEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCustomVerificationEmailTemplates: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack wrapper-key sweep, 2026-08-29): MaxResults/NextToken were never plumbed through the call chain at all -- handleListCustomVerificationEmailTemplates took no query params and the backend method took none either, so ListCustomVerificationEmailTemplatesOutput.NextToken was always empty and every template was returned in one page regardless of MaxResults. Now paginated (own documented 1-50 range, default+cap 50, api_op_ListCustomVerificationEmailTemplates.go) via page.New (custom_verification.go)."}
  CreateReceiptRuleSet: {wire: ok, errors: ok, state: ok, persist: ok}
  CloneReceiptRuleSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteReceiptRuleSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "BEHAVIOR BUG FIXED this pass: previously allowed deleting the active rule set and silently cleared the active pointer. Real AWS SES explicitly forbids this (\"The currently active rule set cannot be deleted.\", api_op_DeleteReceiptRuleSet.go doc comment) via CannotDeleteException (wire code \"CannotDelete\", confirmed in deserializers.go). Added ErrReceiptRuleSetActive -> CannotDelete; the active pointer is no longer touched by delete and callers must SetActiveReceiptRuleSet to something else (or \"\") first."}
  ListReceiptRuleSets: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack wrapper-key sweep, 2026-08-29): NextToken was never plumbed through the call chain -- handleListReceiptRuleSets took no query params and the backend method took none either, so ListReceiptRuleSetsOutput.NextToken was always empty and every rule set was returned in one page. Now paginated at the real 100-per-page default (own doc comment, api_op_ListReceiptRuleSets.go) via page.New (receipt_rule_sets.go)."}
  DescribeReceiptRuleSet: {wire: ok, errors: ok, state: ok, persist: ok}
  SetActiveReceiptRuleSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeActiveReceiptRuleSet: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateReceiptRule: {wire: ok, errors: partial, state: ok, persist: ok, note: "gopherstack-6xj6 (2026-09-08): added required-member validation for ReceiptAction subtypes (S3Action.BucketName, SNSAction.TopicArn, LambdaAction.FunctionArn, BounceAction.{SmtpReplyCode,Message,Sender}, AddHeaderAction.{HeaderName,HeaderValue} -- all required per ses@v1.37.4 types/types.go / botocore ses/2010-12-01 service-2.json 'required' lists) -> InvalidParameterValue (ErrInvalidParameter, validateReceiptAction in receipt_rules.go). Also fixed a compounding parse-layer bug (handler_receipt_rules.go): parseReceiptActions detected each action type by testing presence of ONE specific subfield -- usually the very field now known to be required, e.g. S3Action detected via BucketName != \"\" -- so an S3Action submitted WITHOUT BucketName was invisible to the parser (not merely unvalidated), and its Type == \"\" sentinel silently terminated the entire Rule.Actions.member.N enumeration loop, dropping every later action in the same rule too. Detection now uses hasPrefixedKey (any key under the action's own namespace), so a malformed action reaches validateReceiptAction instead of vanishing along with its siblings. errors: partial, not ok -- CreateReceiptRule's declared error set (botocore CreateReceiptRule.errors) additionally includes InvalidSnsTopicException/InvalidS3ConfigurationException/InvalidLambdaFunctionException, which real AWS emits only after verifying the referenced SNS topic/S3 bucket/Lambda function actually exists and grants SES permission -- that requires a cross-service existence check (services/sns, services/s3, services/lambda), explicitly out of scope for this pass; left gapped, see the ReceiptAction gaps entry below."}
  DescribeReceiptRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteReceiptRule: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateReceiptFilter: {wire: ok, errors: ok, state: ok, persist: ok}
  ListReceiptFilters: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteReceiptFilter: {wire: ok, errors: fixed, state: ok, persist: ok, note: "gopherstack-yatn orphan-code triage FIX: not-found rejection emitted the fabricated FilterDoesNotExist, which exists nowhere in ses@v1.37.4 (deserializers.go's empty error switch for this op) or in botocore's ses/2010-12-01 service-2.json (no \"errors\" key on this op at all -- fewer than sibling DeleteReceiptRule's RuleSetDoesNotExist). Made idempotent (missing filter is now a no-op success) to match this op's own declared error set and the sibling DeleteReceiptRule/DeleteReceiptRuleSet precedent already established in undeclared_delete_errors_test.go. Deleted the now-dead ErrReceiptFilterNotFound sentinel (single raiser). Verified via TestDeleteOps_MissingTarget_Idempotent/receipt_filter (real SDK client) and TestDeleteReceiptFilter_NotFound_IsIdempotent; both pre-existing tests asserting the old 400/FilterDoesNotExist behavior corrected."}
families:
  mailbox_simulator_bounces_complaints: {status: ok, note: "gopherstack-mhnk: GetSendStatistics reported real DeliveryAttempts alongside permanently-hardcoded-zero Bounces/Complaints/Rejects -- a statistics call stating a fact (0% bounce rate) it had not established. Investigated whether this backend has any bounce/complaint concept at all: it did not (no SendBounce-to-stats wiring, no simulator addresses recognised anywhere in services/ses or services/sesv2 -- grepped for bounce@/complaint@/success@/suppressionlist@simulator.amazonses.com, zero hits before this pass). AWS's mailbox simulator addresses are the documented, deterministic, real mechanism for this (AWS SES Developer Guide, mailbox simulator page) -- not invented. Fixed: SendEmail/SendTemplatedEmail (recipient union of To/Cc/Bcc) now recognise bounce@/suppressionlist@simulator.amazonses.com -> Email.Bounced and complaint@simulator.amazonses.com -> Email.Complained (both new omitempty Email fields, round-trip through Snapshot/Restore for free per the existing ReturnPathArn precedent, no sesSnapshotVersion bump); GetSendStatistics buckets these into real per-hour Bounces/Complaints counts. success@/ooto@ are recognised as no-ops (correctly non-bouncing/non-complaining) but not otherwise modeled (no delivery-notification or auto-responder concept exists to hang them on). Rejects is NOT fixed -- no publicly documented client-triggerable path to a post-acceptance SES-side rejection exists, and no content/virus-scanning concept exists in this backend to hang one on; left at 0 rather than fabricated (see gaps)."}
  notification_type_enum_validation: {status: ok, note: "gopherstack-mhnk: SetIdentityHeadersInNotificationsEnabled/SetIdentityNotificationTopic both switched on notificationType with no default case -- an unrecognised value (anything other than Bounce/Complaint/Delivery) silently matched nothing and returned 200 success with no state change, more permissive than real AWS SES's NotificationType enum constraint (aws-sdk-go-v2/service/ses/types/enums.go). Added isValidNotificationType, wired to the previously dead-code ErrValidation sentinel (declared in errors.go, never returned or mapped by any wire code before this pass) -> ValidationError, now mapped in sesErrorCode."}
  event_destination_matching_event_types_validation: {status: ok, note: "gopherstack-mhnk: EventDestination.MatchingEventTypes is a required member (botocore ses/2010-12-01 service-2.json: EventDestination.required = [Name, MatchingEventTypes]) restricted to an 8-value enum (send/reject/bounce/complaint/delivery/open/click/renderingFailure, aws-sdk-go-v2/service/ses/types/enums.go EventType) on both CreateConfigurationSetEventDestination and UpdateConfigurationSetEventDestination, but was entirely unvalidated -- absent, empty, and garbage/wrong-case values (several EXISTING tests in this service used \"Send\"/\"Bounce\", which no real lowercase-only AWS wire value ever matches) all silently succeeded. Added validateMatchingEventTypes (shared by both ops) -> InvalidParameterValue for empty or unrecognised entries. Corrected the wrong-case/missing fixtures across event_destinations_test.go, configuration_sets_test.go, persistence_test.go, store_test.go to real lowercase enum values rather than loosening validation to match their prior (incorrect) expectations, per the campaign's do-not-trust-existing-tests directive. Note: this backend still only models an SNS event destination (EventDestination.SNSTopicARN) -- CloudWatchDestination/KinesisFirehoseDestination, and the real \"exactly one destination type\" constraint, are structurally unmodeled; left out of scope for this pass (bd: none filed, tracked here for a future pass)."}
  sns_notification_delivery: {status: ok, note: "gopherstack-y6rv FIXED. Configuration-set EventDestinations and identity notification topics (SetIdentityNotificationTopic) were validated/stored/returned but never published to over SNS. Added an SNSPublisher interface (services/ses/interfaces.go, same PublishToTopic(topicARN, message string) error shape as cloudwatch's) + SetSNSPublisher (store.go), wired in cli.go's wireSESSNS (SES x SNS, alongside the other 5 SNSPublisher call sites: cloudwatch/eventbridge/pipes/s3/scheduler). SendEmail/SendTemplatedEmail (and their SendRawEmail/SendBulkTemplatedEmail delegates) now, after appending the Email record, publish exactly one notification per send: Bounce/Complaint per classifySimulatedRecipients (see mailbox_simulator_bounces_complaints), otherwise Delivery. SDK-VERIFIED pieces: EventDestination.SNSTopicARN/MatchingEventTypes (models.go, already validated -- see event_destination_matching_event_types_validation) and IdentityRecord.BounceTopic/ComplaintTopic/DeliveryTopic (GetIdentityNotificationAttributes-backed, see that row) gate which topics receive a publish; no new SDK-facing wire shape was touched. DOCUMENTATION-SOURCED, NOT SDK-verified (disclosed both here and in notifications.go's doc comments): the SNS notification JSON payload itself (notificationType/eventType, mail.{timestamp,messageId,source,destination}, bounce.{bounceType,bounceSubType,bouncedRecipients[].emailAddress}, complaint.complainedRecipients[].emailAddress, delivery.{timestamp,recipients}) is sourced verbatim from the AWS SES Developer Guide (https://docs.aws.amazon.com/ses/latest/dg/notification-contents.html) -- SNS notification bodies are consumed by SNS subscribers, never decoded by the ses SDK client, so no pinned aws-sdk-go-v2 type exists to verify field names against; this is the documentation-sourced exception this campaign has already accepted elsewhere (CloudFormation EmptyOnDelete, CloudTrail log-file key layout, Athena query-result object). Kept deliberately minimal per that precedent: only fields every documented example includes are modeled (headers, DSN diagnostics, feedback-report fields, sourceArn/sourceIp/sendingAccountId, processingTimeMillis/smtpResponse/reportingMTA/remoteMtaIp are omitted, not fabricated); bounceType/bounceSubType are always the doc's \"Permanent\"/\"General\" hard-bounce values since classifySimulatedRecipients does not distinguish subtypes; the doc's explicit eventType-vs-notificationType top-level field distinction (\"If you set up event publishing, this field is named eventType\") is honoured -- identity topics use notificationType, configuration-set event destinations use eventType. The identity notification-topic lookup is an exact match on the From address (unlike isVerifiedLocked, no fallback to a verified domain identity) -- a disclosed simplification, not SDK-relevant. An unwired SNSPublisher (SetSNSPublisher never called, matching ~150 services' unwired-hook norm) is a silent no-op, proven by TestSendEmail_NoSNSPublisherWired_StaysPermissive (notifications_test.go)."}
  list_identities_identity_type_filter: {status: ok, note: "gopherstack-mhnk: ListIdentitiesInput.IdentityType (EmailAddress|Domain, aws-sdk-go-v2/service/ses/api_op_ListIdentities.go) was parsed by nothing in handleListIdentities -- always returned every identity regardless of the filter, more permissive than real AWS. Fixed: ListIdentities backend signature gained an identityType parameter (filter applied pre-pagination, matching AWS's own doc note that NextToken continuation requires re-supplying the same IdentityType used in the original call); an unrecognised IdentityType value now returns ValidationError. StorageBackend interface and all in-repo test call sites updated (~7 sites, all within services/ses -- confirmed no root-package caller via grep, go vet . at repo root passes)."}
  void_result_ops: {status: ok, note: "SEVERE FIX — 13 void-result ops (PutIdentityPolicy, DeleteIdentityPolicy, SetIdentityDkimEnabled, SetIdentityFeedbackForwardingEnabled, SetIdentityHeadersInNotificationsEnabled, SetIdentityMailFromDomain, SetIdentityNotificationTopic, UpdateReceiptRule, ReorderReceiptRuleSet, SetReceiptRulePosition, PutConfigurationSetDeliveryOptions, UpdateConfigurationSetEventDestination, UpdateConfigurationSetTrackingOptions) emitted a literal '<*Result></*Result>' XML element (a hardcoded xml:\"*Result\" tag on a struct{} field is not a Go/encoding-xml templating mechanism — it marshals as a literal element named '*Result'). Real aws-sdk-go-v2 client deserializers call decoder.GetElement(\"<Action>Result\") before parsing the body; the mismatched wrapper caused every real AWS SDK client to fail with a client-side DeserializationError even though the emulator's backend mutation succeeded — i.e. these 19 ops were unusable from a real SDK despite passing this repo's own unit tests. Fixed by replacing the generic emptyResponse.Result struct{} field with a nested emptyResult{XMLName xml.Name} whose name is set per-op at construction (newEmptyResponseWithResult for the 13 ops verified via the SDK deserializer to require a <Action>Result wrapper; newEmptyResponse — no wrapper — for the other 6 ops (VerifyEmailAddress, DeleteVerifiedEmailAddress, UpdateAccountSendingEnabled, UpdateCustomVerificationEmailTemplate, UpdateConfigurationSetReputationMetricsEnabled, UpdateConfigurationSetSendingEnabled) whose real output shape has zero members, so the SDK deserializer never looks for a Result element at all — confirmed by reading both deserializers.go code paths line-by-line, not guessing)."}
  account_sending_paused: {status: ok, note: "UpdateAccountSendingEnabled(false)/GetAccountSendingEnabled were stored/toggled/persisted but never enforced anywhere — a paused account could still send unboundedly. Added ErrAccountSendingPaused -> AccountSendingPausedException (exact code, verified against aws-sdk-go-v2/service/ses/types/errors.go), enforced in checkSendingAllowedLocked (shared by SendEmail/SendTemplatedEmail/SendRawEmail/SendBulkTemplatedEmail)."}
  send_quota_enforcement: {status: ok, note: "GetSendQuota advertised Max24HourSend=200 (the real AWS SES sandbox default) but nothing enforced it — accounts could send arbitrarily many emails despite reporting a 200/24h cap. Added enforcement in checkSendingAllowedLocked using the same sentLast24HoursLocked() counting logic GetSendQuota already used (factored out, not duplicated). MaxSendRate (per-second) remains unenforced — deferred, bd: gopherstack-a6y."}
  config_set_existence_validation: {status: ok, note: "ConfigurationSetName was accepted and stored on the Email record by SendEmail/SendTemplatedEmail/SendBulkTemplatedEmail but never validated to exist — real AWS SES returns ConfigurationSetDoesNotExist for an unknown name. Fixed via the same checkSendingAllowedLocked helper; SendBulkTemplatedEmail additionally gained ReplyToAddresses/ReturnPath/SourceArn plumbing it was silently dropping."}
  identity_dkim_notification_ops: {status: ok, note: "reviewed GetIdentity{Dkim,MailFromDomain,Notification}Attributes + Set* counterparts; all read/write real per-identity state under the coarse lock; no stubs found beyond the BehaviorOnMXFailure gap (fixed)."}
  receipt_rules_filters: {status: ok, note: "reviewed CRUD + ordering (CreateReceiptRule after=, ReorderReceiptRuleSet, SetReceiptRulePosition) — real slice manipulation, deep-copies on read to prevent aliasing, verified index math by tracing each function; no bugs found."}
  templates_rendering: {status: ok, note: "{{key}} substitution verified against templated_render_test.go table cases; TemplateData JSON parse errors correctly surfaced as InvalidParameterValue; SendBulkTemplatedEmail default/replacement merge semantics verified (replacement overrides default per-destination)."}
  persistence_janitor: {status: ok, note: "Snapshot/Restore cover every map (identities, templates, configSets, receiptRuleSets, receiptFilters, eventDestinations, trackingOptions, customVerifTemplates, policies) with correct deep-copies; Restore re-applies the TTL/maxRetainedEmails bound immediately; janitor uses pkgs/worker ticker + lockmetrics coarse lock; no goroutine or map leaks found. This pass: confirmed the new Email.ReturnPathArn field round-trips through Snapshot/Restore for free (Email is JSON-encoded directly in backendSnapshot.Emails, no DTO to update, no sesSnapshotVersion bump needed for an additive omitempty field)."}
  invented_error_removed: {status: ok, note: "ErrIdentityNotFound (\"IdentityNotFound\") was DEAD CODE -- defined in errors.go and mapped to wire code \"NoSuchEntity\" in handler.go's sesErrorCode, but never returned by any backend method (grepped every source file; zero call sites). Worse, \"NoSuchEntity\" is not a real SES v1 error code at all -- confirmed by enumerating every case in aws-sdk-go-v2/service/ses/deserializers.go's error-code switch (72-op full list), which has no such entry; IAM has NoSuchEntity, SES does not. Deleted both the sentinel and the wire mapping per the no-invented-errors rule."}
  tracking_options_error_code_wire_bug: {status: ok, note: "SEVERE FIX -- gopherstack sent wire error codes \"TrackingOptionsDoesNotExist\" / \"TrackingOptionsAlreadyExists\" for CreateConfigurationSetTrackingOptions/DeleteConfigurationSetTrackingOptions/UpdateConfigurationSetTrackingOptions failures. Real AWS SES's TrackingOptions{DoesNotExist,AlreadyExists}Exception.ErrorCode() (types/errors.go) returns the Exception-suffixed forms (\"TrackingOptionsDoesNotExistException\" / \"TrackingOptionsAlreadyExistsException\") -- confirmed against deserializers.go's `strings.EqualFold(\"TrackingOptionsDoesNotExistException\", errorCode)` case, the only match real SDK clients recognize. Every sibling *DoesNotExist/*AlreadyExists error in this service omits the suffix, which is why this one was missed by symmetry-based review; it is a genuine SDK-model asymmetry, not a copy-paste target. A real AWS SDK client hitting the old unsuffixed code would fail typed-exception matching and fall back to a generic error. Fixed both the sentinel error strings (errors.go) and the wire mapping (handler.go)."}
  putidentitypolicy_policy_validation: {status: ok, note: "PutIdentityPolicy accepted any string (including empty) as Policy with no validation. Real SES requires Policy as a required, well-formed-JSON member and returns InvalidPolicyException (wire code \"InvalidPolicy\", confirmed in deserializers.go) for malformed input. Added json.Valid() check -> ErrInvalidPolicy; this backend still does not evaluate policy semantics (no sending-authorization enforcement exists anywhere in this emulator, consistent with SourceArn/ReturnPathArn being stored-but-unenforced elsewhere), only well-formedness."}
  required_output_members_r80d: {status: ok, note: "gopherstack-r80d batch 24 (2026-08-21): swept all 13 required-output-member ops (13 required fields per cmd/requiredoutputfields, module `ses` not `sesv2` -- sesv2 already settled batch 21, separate module/version). Cross-referenced every domain struct reachable through those ops against ses@v1.37.4/types/types.go's own required-member annotations (31 structs carry at least one, mostly request-side action types): the 4 Get*Attributes ops (Dkim/MailFromDomain/Notification/Verification) each wrap a map to one of these domain structs one level deeper than the flat op-level scan sees. Found 2 findings / 4 member-level fixes, both the dominant 'required member tagged omitempty in a reachable zero state' class -- see GetIdentityMailFromDomainAttributes/GetIdentityNotificationAttributes rows above. All other required members (ConfigurationSet.Name, ReceiptRule.Name, EventDestination.Name/MatchingEventTypes, MessageId across all Send* ops, DkimTokens, VerificationToken, PolicyNames/Identities lists) confirmed always emitted unconditionally, no omitempty on a required member left. DkimEnabled/DkimVerificationStatus/VerificationStatus/ForwardingEnabled are non-pointer required members on the real SDK type -- omitted vs present-empty decode identically for those, so no bug is even possible there regardless of tagging; none carried omitempty anyway. Proven via real aws-sdk-go-v2/service/ses client round trips (wire_output_required_r80d_test.go), hand-reverted/confirmed-failing/restored, md5sum-verified byte-identical."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "GetSendStatistics Rejects always reports 0 -- unlike Bounces/Complaints (fixed gopherstack-mhnk via the mailbox simulator addresses), Rejects models AWS rejecting a message post-acceptance (e.g. virus-scan rejection) and has no documented deterministic client-side trigger; no content/virus-scanning concept exists anywhere in this backend to hang a real Rejects count off of. Left honestly at 0 (bd: gopherstack-uve, scope narrowed this pass -- see families for what WAS fixed)."
  - "LimitExceededException never returned — no per-resource count caps modeled (max receipt rules/templates/filters etc.); AWS's actual limits are account-specific/adjustable so any hardcoded threshold would be fabricated, not honest (bd: gopherstack-ssk)"
  - "MailFromDomainNotVerifiedException never triggers — confirmed still modeled in the real operation's error list for SendEmail/SendRawEmail/SendTemplatedEmail/SendBulkTemplatedEmail (aws-sdk-go-v2/service/ses@v1.37.4/deserializers.go:5199,5450,5574,5698, strings.EqualFold(\"MailFromDomainNotVerifiedException\", errorCode)), but SetIdentityMailFromDomain instantly marks Success with no Pending/Failed/TemporaryFailure window (the real trigger condition per api_op_SetIdentityMailFromDomain.go: \"taken when the custom MAIL FROM domain setup is in the Pending, Failed, and TemporaryFailure states\") and this backend does no DNS/MX lookups (no pkgs/dns usage in this service) to ever produce one. Consistent with this service's instant-verify convention everywhere else (VerifyEmailIdentity/VerifyDomainIdentity/VerifyDomainDkim all skip the real Pending window too); deliberately not changed in isolation to avoid an inconsistent one-off Pending state (bd: gopherstack-nbp; re-confirmed gopherstack-mhnk, not tractable without either a real DNS-check primitive or a fabricated Pending window)"
  - "MaxSendRate (per-second) advertised via GetSendQuota but not enforced, only the 24h quota is now enforced -- a fabricated per-second throttle would need sub-second timing state with no test-visible way to exercise it without time.Sleep (banned); the advertised value (1/sec) is already the correct AWS sandbox default, just not yet gated (bd: gopherstack-a6y)"
  - "SendRawEmailInput.FromArn (cross-account sending-authorization ARN for the raw message's From: header, distinct from SourceArn/ReturnPathArn) is not captured -- confirmed via handler_email_sending.go: handleSendRawEmail never calls vals.Get(\"FromArn\") at all, so the field is present in the parsed form body but never read into SendEmailInput (accepted-then-silently-dropped, not genuinely absent from the wire shape). botocore's ses/2010-12-01 service-2.json models FromArn as a plain string with no format pattern, so real AWS does not appear to client-side-validate its shape either; rejecting a malformed FromArn cannot be cited to a documented behavior. No cross-account identity/policy enforcement exists anywhere in this backend even for SourceArn (PutIdentityPolicy stores policies but nothing evaluates them), so capturing-but-ignoring FromArn would be indistinguishable from today's behavior. Left unimplemented (bd: none filed, tracked here; re-confirmed gopherstack-mhnk)."
  - "SendTemplatedEmailInput/SendBulkTemplatedEmailInput.TemplateArn (cross-account template reference) is not captured -- same accepted-then-silently-dropped shape as FromArn (handler never reads TemplateArn out of vals), same botocore evidence of no format pattern to validate against, same absence of any cross-account resource model in this backend to act on it. Template remains a required member on both real inputs regardless of TemplateArn. Left unimplemented (bd: none filed, tracked here; re-confirmed gopherstack-mhnk)."
  - "2026-09-05: ReceiptAction fields (S3BucketName, SNSTopicARN, LambdaFunctionARN, SQSQueueARN, BounceTopicARN, etc. on every action type CreateReceiptRule/UpdateReceiptRule accepts) are stored as inert configuration and returned correctly on every describe/list, but this backend has no inbound-mail entry point at all -- no SMTP listener, no API to inject a simulated received message -- so no action ever fires. Unlike the EventDestination gap above, this is judged structural/unfixable within this emulator's architecture (an HTTP API emulator has no MTA to receive real internet SMTP traffic with), not a missing wiring step: there is no reachable trigger to hang a fix off of, mirroring the reasoning already accepted for MailFromDomainNotVerified (gopherstack-nbp). Recorded for completeness, not filed as a bd issue."
  - "gopherstack-6xj6 (2026-09-08), re-auditing the entry above: re-confirmed no inbound-mail path exists, via `grep -rni 'inbound|SMTP|ReceiveEmail|InjectMessage|SimulateReceipt' services/ses` (zero hits outside XML/doc-comment noise) and by diffing handler.go's full 71-op GetSupportedOperations action list against the real SES v1 API -- neither gopherstack nor real AWS SES itself exposes an operation to inject an inbound message (SMTP from the public internet is the only real ingestion path for actual AWS), so the non-firing behavior remains correctly judged structural and still warrants no bd issue. Two independent, genuinely self-contained defects WERE found beside it and fixed this pass -- see CreateReceiptRule/UpdateReceiptRule rows: (1) none of the 5 modeled action subtypes with required members were validated at Create/UpdateReceiptRule time even though CreateReceiptRule's own declared error set implies such a check exists (InvalidSnsTopicException/InvalidS3ConfigurationException/InvalidLambdaFunctionException); (2) the wire parser used a required subfield's own presence as its detection signal, so a malformed action was invisible to the parser rather than merely unvalidated, and silently truncated every later action in the same rule. Newly identified, NOT fixed this pass (no bd filed, left for a future pass): gopherstack's ReceiptActionTypeSQS/xmlSQSAction (models.go, handler_receipt_rules.go -- wire keys Rule.Actions.member.N.SqsAction.{QueueArn,TopicArn}) has no counterpart anywhere in the real ReceiptAction union (ses@v1.37.4 types/types.go:848-882 -- AddHeaderAction/BounceAction/ConnectAction/LambdaAction/S3Action/SNSAction/StopAction/WorkmailAction only; confirmed zero 'SqsAction'/'SQSAction' hits anywhere in the pinned SDK module) -- a gopherstack-only fabrication no real SDK client could ever send, harmless (accepted, stored, and round-tripped, never validated against anything real) but inaccurate, and the issue title itself listed it as though it were a real action type. WorkmailAction and ConnectAction are real action types gopherstack does not model at all. Three real, optional members are accepted-then-silently-dropped on round-trip, independent of the firing question: LambdaAction.InvocationType (types.go:719, Event|RequestResponse, default Event), SNSAction.Encoding (types.go:1264, UTF-8|Base64, default UTF-8), and S3Action.IamRoleArn/KmsKeyArn (types.go:1148,1164)."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "services/sesv2/ — separate REST-JSON service, out of scope this pass per task constraints (bd: gopherstack-029)"
leaks: {status: clean, note: "janitor sweep uses pkgs/worker.Group ticker with proper ctx cancellation via WithJanitor/StartWorker/Shutdown; sweepExpiredEmails is O(k) amortized (slice prefix trim, not full rescan); emailsByID map kept in sync on every eviction path (appendEmailLocked cap-eviction, sweepExpiredEmails, Restore pruning); maxRetainedEmails (10000) bounds the emails slice; no unbounded identity/template/config-set/receipt-rule maps found (all are keyed by caller-supplied names with no synthetic churn); no goroutines leaked outside the single janitor ticker."}
---

## Notes

**Protocol**: SES v1, AWS query-XML protocol (`Version=2010-12-01`, `Action=<Op>` form-encoded
POST, XML response). A separate `services/sesv2/` exists implementing the modern SESv2
REST-JSON API — that is a **different wire protocol and separate audit surface**, deliberately
out of scope this pass (see `deferred`, bd: gopherstack-029).

**The `emptyResponse` XMLName trick is legitimate, not a bug pattern to flag.** Both the outer
`emptyResponse.XMLName xml.Name` (no tag) and the new `emptyResult.XMLName xml.Name` (no tag)
rely on a real `encoding/xml` behavior: when a struct's `XMLName xml.Name` field has **no**
tag (or an empty tag string), `xml.Marshal` uses the runtime value stored in that field as the
element name, rather than requiring a hardcoded tag. This is different from the bug class
called out in `.claude/memories/parity-principles.md` ("a hardcoded `xml:"StubResponse"`
XMLName tag silently overriding every runtime root") — that bug is a **non-empty** literal tag
clobbering the runtime value; an **empty** tag correctly defers to the runtime value. Don't
re-flag `newEmptyResponseWithResult`/`newEmptyResponse` as suspicious; they were specifically
verified against `aws-sdk-go-v2/service/ses/deserializers.go` (see `families.void_result_ops`).

**Which void-result ops need a `<Action>Result` wrapper vs. none at all is NOT a style choice
— it's determined by the real AWS SES service model.** Ops whose output shape has at least a
theoretical member (even if none are ever populated in this emulator) always get a
`<Action>Result>` wrapper in the real wire format, and the generated SDK client's
deserializer explicitly calls `decoder.GetElement("<Action>Result")`. Ops whose output shape
has **zero members at all** skip body parsing entirely on the client (`io.Copy(io.Discard,
response.Body)`), and real AWS omits the wrapper. The 6 no-wrapper ops found this pass
(`VerifyEmailAddress`, `DeleteVerifiedEmailAddress`, `UpdateAccountSendingEnabled`,
`UpdateCustomVerificationEmailTemplate`, `UpdateConfigurationSetReputationMetricsEnabled`,
`UpdateConfigurationSetSendingEnabled`) were confirmed by reading the corresponding
`awsAwsquery_deserializeOp<Action>` function bodies in the SDK, not guessed from symmetry.
If a new void-result op is added, check the SDK deserializer for a `GetElement(...)` call
before assuming a wrapper is needed.

**Instant-verification is a deliberate, consistent design convention across this entire
service**, not a bug: `VerifyEmailIdentity`, `VerifyDomainIdentity`, `VerifyDomainDkim`,
`SetIdentityMailFromDomain`, and (as of this pass) `SendCustomVerificationEmail` all mark
their target `Success`/`Verified` immediately rather than modeling AWS's real
Pending-until-DNS/click-through window. Don't "fix" any one of these in isolation to add a
Pending state — that would make it *inconsistent* with the rest of the service. If a future
pass wants real Pending-window simulation, it should be applied uniformly across all of
these, as a deliberate feature, not a bug fix.

**AWS SES sandbox defaults are already correct constants in this codebase**:
`maxSendQuota24Hours = 200` and `maxSendRate = 1` match the real default SES sandbox
quota/rate for a fresh account. This pass didn't invent numbers to enforce — it wired the
*already-correct* advertised quota value into actual enforcement.

**2026-07-23 pass**: this was an independent field-diff against
`aws-sdk-go-v2/service/ses@v1.34.20` source directly (types/errors.go, deserializers.go,
api_op_*.go structs), not a trust of the prior pass's `ok` markings, per campaign
instructions. Found and fixed: (1) a dead, invented error code (`ErrIdentityNotFound` /
`"NoSuchEntity"` -- never returned by any backend method, and `"NoSuchEntity"` is not a
real SES error at all); (2) a wire error-code bug affecting real SDK clients
(`TrackingOptions{DoesNotExist,AlreadyExists}` sent without the required `Exception`
suffix real AWS uses for just these two error types, unlike every sibling error in this
service); (3) a missing AWS restriction (`DeleteReceiptRuleSet` allowed deleting the
active rule set instead of rejecting with `CannotDeleteException`); (4) silently-dropped
request members across `SendEmail`/`SendTemplatedEmail`/`SendRawEmail`/
`SendBulkTemplatedEmail` (`ReturnPathArn`, and for bulk-send specifically `DefaultTags`
+ per-destination `ReplacementTags`, which the handler never parsed at all); (5) no
JSON-validity check on `PutIdentityPolicy`'s `Policy` member (`InvalidPolicyException`
now enforced). `git` commands are off-limits for this campaign task; `last_audit_commit`
above was intentionally left unchanged rather than updated with an unverifiable value
(one read-only `git rev-parse` was run in error mid-pass — flagged, not repeated).

**Test-only `AppendEmailForTest` helper** (`export_test.go`) was added because the new 24h
send-quota enforcement is incompatible with the pre-existing retention/eviction-cap tests
that send `maxRetainedEmails+N` (10000+) emails through `SendEmail` in a loop to exercise
`appendEmailLocked`'s eviction path — a real account would never accumulate 10000 sends
within a 200/day quota. `AppendEmailForTest` calls the same internal `appendEmailLocked` path
`SendEmail` uses (so eviction/O(1)-map-sync is still exercised for real) while bypassing the
business-rule preconditions (verification, quota, account-paused) that are orthogonal to what
those specific tests assert. Don't be alarmed seeing it used for high-volume tests instead of
`SendEmail` — that's intentional now, not a regression.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed InternalFailure**: same shape as autoscaling's entry
(see that entry or gopherstack-3a8t for the full survey/rationale). `RouteMatcher` now
falls back to `service.MatchesUserAgentMarker(r.Header, "api/ses")` (verified against the
pinned `ses@v1.37.4/api_client.go:638` `AddSDKAgentKeyValue` call) only on the `ReadBody`
failure branch, leaving the existing `Version`+`Action` matching untouched. Migrated
`ExtractOperation`/`ExtractResource`/`Handler()` off `r.ParseForm()` onto
`httputils.ReadBody`+`url.ParseQuery`, per the docdb/neptune precedent (gopherstack-bahs).
Proof: `TestHandler_OversizedBodySurfacesInternalFailure` in `handler_oversized_body_test.go`
drives a real SES SDK client through `service.NewRegistry`/`service.NewServiceRouter`,
confirmed failing pre-fix with `UnknownError`; passes now with `InternalFailure`.
`TestHandler_NormalSizedBodyStillRoutes` is the regression guard. Gates: `go build`,
`go vet`, `gofmt -l` (clean), `go test -race ./services/ses/...` (pass),
`golangci-lint run ./services/ses/...` (0 issues).

### 2026-08-31 (response-element-naming re-verification, gopherstack-uox6 trigger)

Triggered by the rds `DBParameterGroups` bug (`e2a4d084a`): a response list whose
per-item XML wrapper was named for the wrong type, decoding empty for every real
client. ses already found and fixed the same *class* of bug on the 2026-08-29 sweep
(the `<*Result></*Result>` literal-tag void-response wrapper, see `void_result_ops`
above) -- that was a scalar envelope-naming bug, not a list one, so it doesn't cover
the list-wrapper axis specifically. Re-checked that axis here.

Checked `SendBulkTemplatedEmail`'s response, the one op in this service whose output
carries a list of per-item structs (`BulkEmailDestinationStatus`, one entry per
destination) rather than a flat collection of scalars or a map -- the shape closest to
rds's bug. `aws-sdk-go-v2/service/ses@v1.37.4` (matches `go.mod`)
`awsAwsquery_deserializeDocumentBulkEmailDestinationStatusList`
(deserializers.go:9728) matches `strings.EqualFold("member", t.Name.Local)`, i.e. the
generic wrapper, not a custom name; `handler_email_sending.go`'s `xmlBulkStatusList`
emits `xml:"member"` per item -- correct. Per-item fields
(`awsAwsquery_deserializeDocumentBulkEmailDestinationStatus`, deserializers.go:9653:
`MessageId`, `Status`, `Error`) match `xmlBulkEmailDestStatus{MessageID, Status}`
exactly; `Error` is genuinely never populated by this backend (SES send failures are
modeled as an early-return, not a per-destination error field) -- a real-but-unobservable
gap, recorded not fixed. No `*StatusList` shape (rds's specific pattern) exists
elsewhere in this service's deserializers -- confirmed by grep, only the one match
above. **Zero new bugs found; nothing changed in this service.** `go build`, `go vet`
(repo-wide, clean), `go test -race ./services/ses/...` all pass on the unmodified
tree. No AWS documentation was fetched this pass.

### 2026-08-31 (error-target sweep, gopherstack-uox6 class-A campaign)

`errtargetaudit -dir ses` flagged 3 findings, all real. Verified each op's own
`awsAwsquery_deserializeOpError<Op>` switch in `ses@v1.37.4` deserializers.go:

- `DeleteCustomVerificationEmailTemplate`: switch has `default:` only (zero declared
  exceptions) -- was emitting `CustomVerificationEmailTemplateDoesNotExist` (declared
  correctly by 3 siblings: `GetCustomVerificationEmailTemplate`,
  `SendCustomVerificationEmail`, `UpdateCustomVerificationEmailTemplate`). Fixed by
  deletion, not remapping: an op with no declared exception at all can't signal
  not-found via any typed code, so a missing template is now treated as
  already-deleted (idempotent), consistent with common AWS delete semantics.
- `DeleteReceiptRule`: declares only `RuleSetDoesNotExist` -- was also emitting
  `RuleDoesNotExist` (declared correctly by `CreateReceiptRule`'s after-rule lookup,
  `DescribeReceiptRule`, `ReorderReceiptRuleSet`, `SetReceiptRulePosition`,
  `UpdateReceiptRule`) for a rule missing within an existing set. The rule-set
  not-found check stays (it's genuinely declared); only the rule-not-found branch was
  deleted, making a missing rule name idempotent.
- `DeleteReceiptRuleSet`: declares only `CannotDelete` -- was also emitting
  `RuleSetDoesNotExist` (declared correctly by `CloneReceiptRuleSet`,
  `CreateReceiptRule`, `DeleteReceiptRule`, `DescribeReceiptRule`,
  `DescribeReceiptRuleSet`, and others) for a missing rule set name. Deleted; the
  active-rule-set `CannotDelete` check (genuinely declared) stays. A missing rule set
  is now idempotent.

None of the three shared sentinels (`ErrCustomVerifTemplateNotFound`,
`ErrReceiptRuleNotFound`, `ErrReceiptRuleSetNotFound`) were changed -- each stays
correct for its other legitimate callers listed above; only the three offending call
sites were edited.

Test-first: `undeclared_delete_errors_test.go` (real SDK client, `errors.As`-free —
asserts `NoError` since none of these three ops can produce a typed not-found error)
confirmed failing against the unmodified tree (all three returned the undeclared
typed error), then passing after the fix. Two pre-existing table-driven tests
(`custom_verification_test.go`'s `template_not_found`, `receipt_rule_sets_test.go`'s
`not_found`, `receipt_rules_test.go`'s `rule_not_found`) asserted the old wrong
400/exception-name pairing as correct; corrected to assert 200/success.

Gates: `go build ./services/ses/...`, `go vet ./services/ses/...`,
`go test -race -count=1 ./services/ses/...` (pass), `golangci-lint run
./services/ses/...` (0 issues).

### 2026-09-05 (parity re-audit, chore/parity-sweep-2026-09-03)

`last_audit_commit: a40e7cc1` turned out not to be an ancestor of this branch
(`git merge-base --is-ancestor a40e7cc1 HEAD` fails) -- it predates a squash-history
rewrite, consistent with this repo's documented squash-merge behavior
(`CLAUDE.md`: "this repo squash-merges ... only 8 of 155 Closes trailers survive
into main"). Re-audit protocol fell back to `last_audit_date` (2026-08-29): every
commit touching `services/ses/` after that date was inspected directly rather than
diffed against the stale hash.

Two such commits existed: `c78177958` (2026-09-02, part of a 222-bug cross-service
campaign PR) and `b8484292f` (2026-09-04, a ghost-row-after-delete sweep). Both
were read in full (`git show <sha> -- services/ses/`) and confirmed to be exactly
what this file's existing 2026-08-31 sections already describe (idempotent-delete
fixes, `ConfigurationSetAttributeNames` gating, `ListReceiptRuleSets`/
`ListCustomVerificationEmailTemplates` pagination) plus one new fix not yet
recorded here: `DeleteIdentity` left a `policies[identity]` ghost row for a
recreated identity to inherit (`b8484292f`, regression test
`TestDeleteIdentity_ClearsPoliciesOnRecreate` in `identities_test.go`) -- already
fixed on this branch, re-verified correct (only `policies` is a side map keyed by
identity outside the `identities` table itself; DKIM/mail-from/notification-topic
state lives on `IdentityRecord` and is wiped by `b.identities.Delete` already).

Bug classes named across the recent repo-wide campaign but not previously checked
by name for `ses` were re-derived from source, not trusted:

- **Tie-prone sorts** (a `sort.Slice` comparator with no tiebreak, over a map-walk
  source): every paginated `List*` op in this service (`ListIdentities`,
  `ListConfigurationSets`, `ListReceiptRuleSets`, `ListTemplates`,
  `ListCustomVerificationEmailTemplates`) sorts on the field that is also the
  underlying `store.Table`'s own unique key, so no tie can exist structurally --
  same "clean by structure" class as iam/cleanrooms/medialive in the campaign's own
  commit messages. `ListReceiptFilters`/`ListVerifiedEmailAddresses`/
  `ListIdentityPolicies` sort similarly but aren't paginated at all in the real API,
  so no page-boundary loss is possible either way.
- **Negative-continuation-token panic**: every paginated op here goes through the
  shared `pkgs/page.New`, which already guards `idx < 0` (`pkgs/page/page.go:68`) --
  confirmed by reading the helper directly, not assumed from the service using it.
- **Equality-cursor restart-to-zero**: doesn't apply -- `pkgs/page` cursors are
  plain offsets, not identity-matched, so there's no "cursor no longer found, so
  resume at 0" branch to have this bug in the first place.

New finding this pass, unrelated to the campaign's named classes:
`GetIdentityPolicies` treated an absent/empty `PolicyNames` as "return every policy
for this identity" -- `PolicyNames` is a required member
(`api_op_GetIdentityPolicies.go`) with no such mode in the real op; the doc itself
says to call `ListIdentityPolicies` first if you don't know the names. Fixed to
return `InvalidParameterValue`; see the `GetIdentityPolicies` row for the full
citation and the errtargetaudit cross-check. Regression test:
`TestGetIdentityPolicies_EmptyNamesList_IsRejected` (backend-level) and
`TestHandler_GetIdentityPolicies/missing_policy_names_param` (wire-level, since a
real SDK client's own client-side validator would refuse to build this request at
all -- only a raw/non-Go-SDK client can reach it). Both neutered (guard commented
out, restored from a saved copy) and confirmed failing pre-fix: the backend test
got `nil` instead of an error, the wire test got `200`/`GetIdentityPoliciesResponse`
instead of `400`/`InvalidParameterValue`.

One new **gap** recorded (not fixed, see `gaps` above for full citations and
reasoning): receipt-rule S3/SNS/Lambda/SQS/Bounce actions are inert
configuration with no inbound-mail entry point in this backend to ever
trigger them -- judged structural/unfixable, same class as the already-accepted
`MailFromDomainNotVerified` gap.

2026-09-06 (gopherstack-y6rv): the configuration-set `EventDestinations` /
identity notification-topic SNS gap recorded above on 2026-09-05 is FIXED --
see `sns_notification_delivery` in `families` for the full account (SDK-verified
vs. documentation-sourced pieces, exact payload provenance, and the
unwired-stays-permissive proof).

Also checked, confirmed already correct, no changes: `MessageRejected` reachability
for unverified-sender sends (yes, all four `Send*` paths); `GetSendStatistics`/
`sentLast24HoursLocked` hot-path shape (bounded by `maxRetainedEmails`, no O(n²));
`DeleteConfigurationSet`'s cascade (event destinations + tracking options; delivery/
reputation options are plain struct fields wiped by `configSets.Delete`, not a
separate map, so no further ghost-row risk there); LocalStack parity -- **NOT
CHECKED**, no LocalStack instance or repo harness available in this sandbox to
compare against.

Baseline (pre-fix, this pass): `golangci-lint run ./services/ses/...` → `0 issues`;
`go test -race -count=1 ./services/ses/...` → `ok`; `go build ./services/ses/...`
→ clean. Same three gates green after the fix. `go run ./cmd/errtargetaudit -dir
ses` → `0 class A findings` both before and after (this fix doesn't touch error
targeting). `go run ./cmd/requiredoutputfields` was not re-run repo-wide this pass
(no `-dir` flag exists on the current binary, and a repo-wide run would have read
`services/sagemakerruntime/`, off-limits this pass per a concurrent editor there);
the existing `wire_output_required_r80d_test.go` sweep for this service was
trusted as unchanged and still green.
