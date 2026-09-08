---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: pinpoint
sdk_module: aws-sdk-go-v2/service/pinpoint@v1.42.4
last_audit_commit: 31283c0f
last_audit_date: 2026-08-13
overall: A            # genuine field-diff bugs found and fixed this pass across the template family
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateVoiceTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed to full parity: added TemplateType, LastModifiedDate, DefaultSubstitutions, LanguageCode, TemplateDescription, Version, VoiceId vs VoiceTemplateResponse/VoiceTemplateRequest — was missing all of these"}
  GetVoiceTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same field set fix as CreateVoiceTemplate"}
  UpdateVoiceTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "now applies DefaultSubstitutions/LanguageCode/TemplateDescription/VoiceId and advances LastModifiedDate/Version, matching every other Update*Template"}
  DeleteVoiceTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "was leaking its templateVersionHistory entry (only Delete{Email,InApp,Push,Sms}Template cleaned it up) — fixed; locked by TestDeleteVoiceTemplate_ReleasesVersionHistory"}
  CreateEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "DefaultSubstitutions was wire-typed as a nested JSON object (map[string]any); the real EmailTemplateRequest/Response serializers/deserializers treat it as a JSON-*encoded string* — fixed. Added missing required TemplateType field"}
  GetEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same DefaultSubstitutions + TemplateType fixes; simplified to return the model directly instead of a hand-built map (cloneEmailTemplateToResponse deleted, now redundant)"}
  UpdateEmailTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same DefaultSubstitutions fix"}
  CreateInAppTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "added missing TemplateType (required) and CustomConfig (map[string]string) fields vs InAppTemplateResponse/InAppTemplateRequest"}
  GetInAppTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same TemplateType/CustomConfig fix"}
  UpdateInAppTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "now applies CustomConfig updates"}
  CreatePushTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETED invented top-level Body/Title fields — the real PushNotificationTemplateRequest/Response has no such fields, per-platform body/title live inside ADM/APNS/Baidu/Default/GCM only. Added missing ADM, Baidu, DefaultSubstitutions (string, same wire-type fix as email), RecommenderId, TemplateType"}
  GetPushTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same field set fix as CreatePushTemplate"}
  UpdatePushTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same field set fix; decomposed into applyPushTemplateUpdate to keep the op function's complexity down given the larger field set"}
  CreateSmsTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETED invented SenderId field — the real SMSTemplateRequest/Response has no SenderId (that's an SMS *channel* field, SMSChannelRequest, not a template field). Added missing DefaultSubstitutions (string), RecommenderId, TemplateType"}
  GetSmsTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same field set fix as CreateSmsTemplate"}
  UpdateSmsTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same field set fix"}
  UpdateSmsChannel: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETED PromotionalMessagesPerSecond/TransactionalMessagesPerSecond from the request type — real SMSChannelRequest has no such fields (they're SMSChannelResponse-only, AWS-computed account throughput); gopherstack was accepting and echoing back caller-supplied values for fields no real SDK client can send"}
  UpdateEmailChannel: {wire: ok, errors: ok, state: ok, persist: ok, note: "added missing OrchestrationSendingRoleArn field vs EmailChannelRequest/EmailChannelResponse"}
  GetCampaignVersion: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was silently falling back to the CURRENT campaign when the requested version number wasn't in history, instead of 404 NotFoundException; AWS's own resource docs for /v1/apps/{appId}/campaigns/{campaignId}/versions/{version} document 404 NotFoundException as the response when \"the specified resource was not found\" — fixed to always 404 on an unknown version. Locked by TestGetCampaignVersion_UnknownVersionNotFound"}
  GetSegmentVersion: {wire: ok, errors: ok, state: ok, persist: n/a, note: "same fallback bug and fix as GetCampaignVersion. Locked by TestGetSegmentVersion_UnknownVersionNotFound"}
  CreateSegment: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-wksweep-pp-1 (2026-08-28, acceptguard): WriteSegmentRequest has no ImportDefinition member (pinpoint@v1.42.4 types/types.go:7240) -- it's derived only from CreateImportJob, which already materializes an IMPORT-type segment correctly (export_import_jobs.go). A prior version accepted ImportDefinition directly on CreateSegment/UpdateSegment and let a client set an IMPORT-typed segment a real client never could. Fixed by removing it from both request structs; the CreateImportJob derivation path is unchanged. Real client can't send the field, so proof is raw-body (TestCreateSegment_RawImportDefinitionFieldIgnored, wire_field_fixes_test.go) plus rewritten TestSegment_ImportType/TestSegment_UpdatePreservesType (segments_test.go) driving CreateImportJob instead."}
  UpdateSegment: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same ImportDefinition fix as CreateSegment -- see that note."}
  DeleteUserEndpoints: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-r80d batch 5: DeleteUserEndpointsOutput.EndpointsResponse is required (pinpoint@v1.42.4 api_op_DeleteUserEndpoints.go:44-51) and the wire is the entire body deserialized directly into it (deserializers.go:5482), not a wrapper key. The handler wrote a bare 204 No Content; the real client's decoder treats the empty body as EOF (tolerated, deserializers.go:5472) so the call succeeded with EndpointsResponse left nil — same empty-body class as batch one's lambda DeleteCapacityProvider. Fixed to return the deleted endpoints as EndpointsResponse.Item with a 200 body, matching the sibling DeleteEndpoint (singular)'s existing pattern. Locked by TestDeleteUserEndpoints_EndpointsResponse_RealClient"}
  # ops carried forward unchanged from the 2026-07-12 pass (files not touched this pass, still trusted):
  CreateJourney: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-wksweep-pp-2 (2026-08-28, acceptguard): neither WriteJourneyRequest nor JourneyResponse has a Tags member at all (pinpoint@v1.42.4 types/types.go:7118, 4227) -- journeys are taggable only through the generic TagResource/ListTagsForResource ARN-based API (tags.go), same as every other Pinpoint resource. A prior version accepted a tags field on CreateJourney/UpdateJourney and echoed it back in journeyResponse -- fabricated on BOTH the request and response sides, matching this sweep's appstream Email precedent. Fixed by removing tags/Tags from createJourneyRequest, updateJourneyRequest, and journeyResponse; the real TagResource path (already correct, storage-only via the tagHolder interface) is untouched. Real client can't send/read the field, so proof is raw-body (TestCreateJourney_RawTagsFieldIgnored, wire_field_fixes_test.go), which also exercises the real TagResource/ListTagsForResource round trip on the same journey to prove tagging still works the real way."}
  GetJourneyExecutionMetrics: {wire: ok, errors: ok, state: ok, persist: ok, note: "route fix from prior pass; now covered by full-state persistence too"}
  GetJourneyExecutionActivityMetrics: {wire: ok, errors: ok, state: ok, persist: ok}
  GetJourneyRunExecutionMetrics: {wire: ok, errors: ok, state: ok, persist: ok}
  GetJourneyRunExecutionActivityMetrics: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveAttributes: {wire: ok, errors: ok, state: ok, persist: n/a}
  SendMessages: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-lffs: request and response were both wrapped under a top-level MessageRequest/MessageResponse key that a real client never sends/reads (pinpoint@v1.42.4's awsRestjson1_serializeOpSendMessages/deserializeOpSendMessages both operate on the member's own fields flat, no wrapper) -- fixed both directions. Locked by TestSendMessages_RealClient"}
  SendUsersMessages: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-lffs: same flat-not-wrapped bug as SendMessages, both directions (SendUsersMessageRequest/SendUsersMessageResponse) -- fixed. Locked by TestSendUsersMessages_RealClient"}
  SendOTPMessage: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-lffs: response was wrapped under a MessageResponse key that a real client never reads -- fixed (request body was already unused/ignored by this op, no request-side bug). Locked by TestSendOTPMessage_RealClient"}
  VerifyOTPMessage: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-lffs: request was wrapped under a VerifyOTPMessageRequestParameters key a real client never sends, so a real client's Otp value never reached the backend and verification always fell back to the no-code has-pending-OTP check regardless of the code sent -- fixed (response was already flat/correct). Locked by TestVerifyOTPMessage_WrongCode_RealClient"}
  PhoneNumberValidate: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-lffs: request and response were both wrapped under a top-level NumberValidateRequest/NumberValidateResponse key that a real client never sends/reads -- fixed both directions. Locked by TestPhoneNumberValidate_RealClient"}
  PutEvents: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-lffs: request was wrapped under an EventsRequest key a real client never sends, so BatchItem was always read as empty and every event silently vanished for a real client (response was already flat/correct) -- fixed. Locked by TestPutEvents_RealClient"}
  UpdateApnsChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tp8x (2026-08-21): APNSChannelResponse has no BundleId/Certificate/TeamId/TokenKey/TokenKeyId member at all (only HasCredential/HasTokenKey/DefaultAuthenticationMethod) -- all five were being echoed raw on the wire via toChannelResponse's blind maps.Copy(resp, ch.ExtraData). Same bug and fix for ApnsSandbox/ApnsVoip/ApnsVoipSandbox (shared parseAPNSChannelExtra/filterChannelExtraForEcho code path). Fixed by filtering ExtraData per channel type before echo. Raw-body-asserted (TestGetApnsChannel_NoRawSecretInBody) since a typed real-client decode can't observe an extraneous unknown key that has no struct field to land in."}
  GetApnsChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "see UpdateApnsChannel's gopherstack-tp8x note -- same toChannelResponse fix. Locked by TestGetApnsChannel_NoCredentialLeak_RealClient (DefaultAuthenticationMethod/HasCredential round-trip) and TestGetApnsChannel_NoRawSecretInBody (raw secret absence)"}
  GetGcmChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tp8x (2026-08-21): GCMChannelResponse's real credential member is \"Credential\", not the request's \"ApiKey\" -- was echoed under the wrong key so a real client's Credential field was always nil. GCM's ServiceJson also has no response member at all (only the boolean HasFcmServiceCredentials, now tracked on Channel and derived in channelCredentialFlags). Fixed. Locked by TestGetGcmChannel_CredentialField_RealClient, TestGetGcmChannel_ServiceJSON_HasFcmServiceCredentials_RealClient"}
  UpdateGcmChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same fix as GetGcmChannel -- shared toChannelResponse"}
  GetBaiduChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tp8x (2026-08-21): BaiduChannelResponse's real credential member is \"Credential\" (required), not the request's \"ApiKey\"; SecretKey has no response member at all and was being echoed raw alongside it. Fixed. Locked by TestGetBaiduChannel_CredentialField_RealClient"}
  UpdateBaiduChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same fix as GetBaiduChannel"}
  GetAdmChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tp8x (2026-08-21): ADMChannelResponse has no ClientId/ClientSecret member at all (only HasCredential) -- both were being echoed raw on the wire. Fixed; raw-body-asserted (TestGetAdmChannel_NoRawSecretInBody) since a typed real-client decode can't observe an extraneous unknown key that has no struct field to land in."}
  UpdateAdmChannel: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same fix as GetAdmChannel"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCampaignVersions: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetSegmentVersions: {wire: ok, errors: ok, state: ok, persist: n/a}
families:
  App: {status: ok, note: "unchanged this pass; last verified 2026-07-12"}
  Campaign: {status: ok, note: "unchanged this pass except GetCampaignVersion fallback-to-current bug (see ops)"}
  Segment: {status: ok, note: "GetSegmentVersion fallback-to-current bug (prior pass) plus this pass's ImportDefinition phantom-field fix on CreateSegment/UpdateSegment (see ops, gopherstack-wksweep-pp-1)"}
  Endpoint: {status: ok, note: "gopherstack-r80d batch 5 fixed DeleteUserEndpoints (bare 204 dropped the required EndpointsResponse — see ops); prior 'unchanged, still trusted' note was stale for this one op. Rest of the family unchanged, now participates in full persistence (see Persistence section)"}
  EventStream: {status: ok, note: "unchanged this pass; now participates in full persistence"}
  Channels: {status: ok, note: "SMS channel PromotionalMessagesPerSecond/TransactionalMessagesPerSecond request-side hygiene fix + Email channel OrchestrationSendingRoleArn field addition (prior pass); gopherstack-tp8x (2026-08-21) found and fixed a credential-echo bug this 'no other gaps found' note had missed: toChannelResponse blindly echoed the raw request-side ExtraData map for every channel type, so GCM/Baidu's ApiKey was echoed under the wrong key (should be 'Credential') and ADM/APNS's ClientId/ClientSecret/BundleId/Certificate/TeamId/TokenKey/TokenKeyId/GCM's ServiceJson were echoed raw despite having no response member at all -- see ops for the per-op notes. Now participates in full persistence"}
  Tags: {status: ok, note: "unchanged this pass"}
  Template (email): {status: ok, note: "field-diffed this pass: DefaultSubstitutions wire-type bug + missing TemplateType fixed (see ops). Was previously marked ok on an incomplete field-diff — this pass caught what the prior pass missed"}
  Template (inapp): {status: ok, note: "field-diffed this pass: added missing TemplateType + CustomConfig (see ops)"}
  Template (push): {status: ok, note: "field-diffed this pass: DELETED invented top-level Body/Title, added ADM/Baidu/DefaultSubstitutions/RecommenderId/TemplateType (see ops). This family had the largest gap between gopherstack's shape and the real SDK's shape found this pass"}
  Template (sms): {status: ok, note: "field-diffed this pass: DELETED invented SenderId (real field lives on the SMS channel, not the template), added DefaultSubstitutions/RecommenderId/TemplateType (see ops)"}
  Template (voice): {status: ok, note: "was partial — now field-diffed to full parity against VoiceTemplateRequest/VoiceTemplateResponse: added TemplateType/LastModifiedDate/DefaultSubstitutions/LanguageCode/TemplateDescription/Version/VoiceId, plus fixed a templateVersionHistory leak on delete (see ops). Locked by TestVoiceTemplate_FullFieldSet"}
  Journey: {status: ok, note: "this pass fixed CreateJourney/UpdateJourney/journeyResponse's phantom Tags field, fabricated on both request and response sides (see ops, gopherstack-wksweep-pp-2). Otherwise unchanged; last verified 2026-07-12"}
  Job (export/import): {status: ok, note: "unchanged this pass"}
  Recommender: {status: ok, note: "unchanged this pass"}
  Messaging (SendMessages/SendUsersMessages/OTP/PutEvents): {status: ok, note: "gopherstack-lffs (2026-08-20): the '6flj sweep's own note that this family was 'unchanged this pass' meant it was never re-diffed against the flat/payload shape -- it wasn't. Found and fixed a request- and/or response-side top-level wrapper key on SendMessages, SendUsersMessages, SendOTPMessage, VerifyOTPMessage, and PutEvents (see ops). No further gaps found."}
  Phone: {status: ok, note: "gopherstack-lffs (2026-08-20): same wrapper-key bug as Messaging, both directions on PhoneNumberValidate (see ops)."}
  Route matcher: {status: ok, note: "gopherstack-jqh2: added TestExtractOperation_SDKRouteTable (handler_paths_sdk_diff_test.go), a permanent per-op method+path diff of all 122 real ops extracted from pinpoint@v1.42.4 serializers.go against ExtractOperation, including the generic {TemplateName}/{TemplateType}/versions and /active-version paths (discriminated from the per-type Create/Get/Update/Delete paths, which use a literal type segment, not a placeholder). 122/122 pass; no route-matcher bugs found, no duplicate op-resolution table, no query-flag-discriminated ops, no wrong-date-prefix paths."}
  Persistence: {status: ok, note: "was the biggest structural gap: persistRegistry() excluded voiceTemplates/endpoints/eventStreams/channels (all store.Table-backed — mechanical fix, just needed registering) and appSettings/campaignVersions/segmentVersions/templateVersionHistory/campaignActivities/journeyRuns/appEvents/sentMessages/otpCodes (map-shaped state, added as direct JSON fields on backendSnapshot since every value type is already plain-JSON-friendly). Snapshot version bumped 1->2 so an old on-disk snapshot is cleanly discarded (not partially misdecoded) rather than silently accepted with a shape mismatch. Locked by the rewritten TestSnapshotRestore_FullStateRoundTrip, which now asserts these resource kinds SURVIVE a restart instead of asserting they don't"}
gaps:
  - "gopherstack-coib: PutEvents' documented per-individual-event size quota (1,000 KB) is not enforced -- only its request-level 4 MB quota is. See the gopherstack-coib Notes section."
  - "gopherstack-coib: PayloadTooLargeException size checks are wired for the 39 ops that both model the exception (digit-safe-extracted from deserializers.go: 113 of 122 ops) and have an observable non-trivial request body in this handler. The other 74 modeled ops (GET/DELETE with an empty body) and TagResource/UntagResource/ListTagsForResource/Create{Email,InApp,Push,Sms,Voice}Template (the 9 ops that don't model the exception at all) are left unenforced -- see Notes."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "GetApplicationDateRangeKpi/GetCampaignDateRangeKpi/GetJourneyDateRangeKpi always return an empty KpiResult.Rows — acceptable stub-shaped-but-real-state pattern (queries real backend, returns AWS-accurate empty analytics), not re-flagged"
  - "SendMessages has thin per-channel-type payload assertions (SMS/EMAIL/push body shape) — response envelope itself is fully covered, but content-shape assertions per channel type could be deepened in a future pass"
  - "PushTemplate/APNSPushNotificationTemplate/AndroidPushNotificationTemplate/DefaultPushNotificationTemplate sub-objects (ADM/APNS/Baidu/Default/GCM) are stored as generic map[string]any rather than field-validated structs, consistent with the project's existing convention for nested platform-override objects elsewhere in this file (Campaign.MessageConfiguration, Journey.Activities, etc.) — round-tripped but not field-validated. Not re-flagged as a gap since gopherstack does not field-validate equivalent nested objects anywhere else in this service either"
leaks: {status: clean, note: "no goroutines/timers spawned by this service; purgeAppStateLocked correctly frees all per-app maps on DeleteApp (verified by reading the function and its purgeTableByAppID/deletePrefixed helpers, leak_test.go covers it). This pass additionally fixed DeleteVoiceTemplate leaking its templateVersionHistory entry (only Delete{Email,InApp,Push,Sms}Template cleaned theirs up — VoiceTemplate was the odd one out); locked by new TestDeleteVoiceTemplate_ReleasesVersionHistory"}
---

## Notes

Protocol: **restjson1**, `/v1/...` paths, service alias `mobiletargeting` (checked via
`httputils.ExtractServiceFromRequest`). Tags on every taggable resource use a **lowercase**
`"tags"` JSON key (confirmed against `deserializers.go`/`serializers.go` `object.Key("tags")`
call sites) while every other field is PascalCase — this looks like a bug if you're skimming
but is AWS-accurate; don't re-flag it.

### gopherstack-lffs (2026-08-20): re-audit of the 6flj wrapper-key sweep, and 5 real wrapper-key bugs found outside its scope

gopherstack-6flj's pinpoint pass (documented in `services/_WRAPPER_KEY_SWEEP_REMAINDER.md`)
correctly identified that pinpoint is flat/payload service-wide (every op's generated
`deserializeOpDocument<Op>Output` wrapper is dead code, confirmed by `cmd/bodyclass`: 120
flat/payload, 2 void, 0 wrapped) and correctly avoided making any top-level-wrapper-key "fix" —
its 5 reported bugs are all genuinely below the top level (nested `Definition`, missing
required members). None of that pass's verdicts were void or need withdrawal.

But that pass's own scope note ("Messaging ... unchanged this pass", "Phone ... unchanged this
pass") meant the message/OTP/phone-validate family was never re-diffed against the flat shape at
all — and it turned out to be the one place in the service where gopherstack really had fallen
into the trap the sweep was designed to catch, on **both** the request and response sides:
`SendMessages`, `SendUsersMessages`, `SendOTPMessage`'s responses, and `PhoneNumberValidate`'s
request+response were wrapped under a top-level key (`MessageResponse`,
`SendUsersMessageResponse`, `NumberValidateRequest`/`NumberValidateResponse`) that pinpoint's
real `awsRestjson1_serializeOp*`/`deserializeOp*` functions never write or read (confirmed
file+line in `pinpoint@v1.42.4/deserializers.go` and `serializers.go` for each). `VerifyOTPMessage`
had the same bug on its request only (response was already flat) — a real client's `Otp` value
never reached the backend, so verification silently fell back to the no-code
"was an OTP ever sent?" path regardless of the code actually sent, and `PutEvents`'s request was
wrapped under an `EventsRequest` key, so a real client's `BatchItem` was always read as empty and
every event silently vanished. All 6 ops fixed in both directions where applicable; see the `ops:`
entries above. Two pre-existing tests (`TestSendMessages_ResponseEnvelope`,
`TestSendUsersMessages_*`/`TestPhoneNumberValidate_*` raw-body assertions) had actively encoded
the wrapped shape as correct — these were "ratifying tests" for the bug, not for a fix; rewritten
to assert the flat shape. 6 new real-SDK-client tests
(`services/pinpoint/messages_wrapper_fix_test.go`) lock the fix; each was hand-reverted
individually (file swap, not git) and confirmed to fail with the exact predicted symptom before
being restored.

No other void verdicts found: cross-checked every `member` name `cmd/bodyclass` reports against
every JSON struct tag in `wire.go` service-wide (`grep` for `json:"<Member>"` per member) — the
only wrapper-shaped response structs in the entire file were the 4 fixed above (now removed).
Response handlers for the other 116 flat ops all write their `to*Response`/`*Response` struct
directly (no wrapping map), confirmed by reading every `httputils.WriteJSON` call site.

### Highest-severity finding this pass: the template family had systematic wire-shape drift

Field-diffing every `Create/Get/Update*Template` op against
`aws-sdk-go-v2/service/pinpoint/types` (not just re-trusting the prior pass's "ok" status, per
the audit brief's explicit instruction not to mark a family ok on a no-stub basis alone) found
that **every one of the five template types had real bugs**, not just the previously-flagged
voice template:

- **`DefaultSubstitutions` was wire-typed wrong on every template that has it (email/push/sms/voice).**
  The real `EmailTemplateRequest`/`EmailTemplateResponse`/`PushNotificationTemplateRequest`/
  `PushNotificationTemplateResponse`/`SMSTemplateRequest`/`SMSTemplateResponse`/
  `VoiceTemplateRequest`/`VoiceTemplateResponse` types all declare `DefaultSubstitutions *string`
  — confirmed against the deserializer (`jtv, ok := value.(string)`) and serializer
  (`ok.String(*v.DefaultSubstitutions)`) for each. gopherstack stored/serialized it as a nested
  JSON object (`map[string]any`) instead of the JSON-encoded string a real SDK client actually
  sends/receives. Fixed on `EmailTemplate`/`PushTemplate`/`SmsTemplate`/`VoiceTemplate` to a plain
  `string` field; a real client is expected to pass an already-`json.Marshal`ed string, same as it
  would to the real API.
- **Every template response was missing the required `TemplateType` field.** All five
  `*TemplateResponse` types mark `TemplateType` "This member is required" (`EMAIL`/`SMS`/`VOICE`/
  `PUSH`/`INAPP`). None of gopherstack's five template model structs had it at all — a real SDK
  client reading `output.EmailTemplateResponse.TemplateType` (etc.) always got the zero value.
  Fixed by adding the field to every template struct and populating it at create time.
- **`PushTemplate` had two INVENTED fields (`Body`, `Title`) that don't exist on the real wire.**
  `PushNotificationTemplateRequest`/`PushNotificationTemplateResponse` have no top-level
  `Body`/`Title` — per-platform body/title live inside `ADM`/`APNS`/`Baidu`/`Default`/`GCM` only
  (confirmed against `awsRestjson1_serializeDocumentPushNotificationTemplateRequest`, which has no
  `Body`/`Title` cases). Deleted both fields per the audit brief's "delete gopherstack-invented
  fields" rule. Also added the two real fields gopherstack was missing entirely: `ADM` and `Baidu`
  (the same generic-map treatment already used for `APNS`/`Default`/`GCM`), plus
  `RecommenderId`.
- **`SmsTemplate` had an INVENTED field (`SenderId`) that doesn't exist on the real wire.**
  `SMSTemplateRequest`/`SMSTemplateResponse` have no `SenderId` at all (confirmed against
  `awsRestjson1_serializeDocumentSMSTemplateRequest`) — `SenderId` is a *channel* setting
  (`SMSChannelRequest`), not a template field; the channel-side `SenderId` (in `channels.go`/
  `handler_channels.go`) is unaffected and correct. Deleted the invented field from
  `SmsTemplate`/`createSmsTemplateRequest`; added the real missing `RecommenderId` field.
- **`VoiceTemplate` (previously flagged `partial`) was missing six real fields**:
  `TemplateType`, `LastModifiedDate`, `DefaultSubstitutions`, `LanguageCode`,
  `TemplateDescription`, `Version`, `VoiceId`. All added; `UpdateVoiceTemplate` now advances
  `Version`/`LastModifiedDate` the same way every other `Update*Template` does (it previously did
  neither).
- **`InAppTemplate` was missing `TemplateType` and `CustomConfig`** (`map[string]string`, a real
  field on `InAppTemplateRequest`/`InAppTemplateResponse`). Added both.

All test files under `templates_*_test.go` that exercised the invented fields (`TestSMSTemplate_SenderID`,
`TestSMSTemplate_UpdateSenderID`, the top-level `Body`/`Title` assertions in
`TestPushTemplate_PerPlatformOverrides`/`TestPushTemplate_UpdatePerPlatform`) were rewritten to
exercise the real fields instead (renamed to `TestSMSTemplate_RecommenderID`/
`TestSMSTemplate_UpdateRecommenderID`; push tests now nest `Body`/`Title` inside `Default`, and
also cover `ADM`/`Baidu`). New tests lock every added field:
`TestVoiceTemplate_FullFieldSet`, `TestEmailTemplate_TemplateType`,
`TestInAppTemplate_TemplateTypeAndCustomConfig`, and the rewritten
`TestEmailTemplate_DefaultSubstitutions` (now asserts the string wire shape instead of a nested
object).

### Second-highest-severity finding: persistRegistry() excluded most of the backend's state

The prior pass documented this as a known gap rather than fixing it (see the 2026-07-12 gaps
list, now empty). `voiceTemplates`/`endpoints`/`eventStreams`/`channels` are `store.Table`-backed
the same as every persisted resource kind — they simply weren't registered in
`persistRegistry()`. `appSettings`/`campaignVersions`/`segmentVersions`/
`templateVersionHistory`/`campaignActivities`/`journeyRuns`/`appEvents`/`sentMessages`/
`otpCodes` are map-shaped (`map[string][]T` / `map[string]T`, not `map[string]*T`) so they can't
go through `store.Table` (which requires a pure key function on one concrete pointer type), but
every value type is already a plain JSON-friendly struct, so they're persisted as direct fields
on `backendSnapshot` instead of a separate DTO. `pinpointSnapshotVersion` bumped 1→2 so an
old-shape snapshot is cleanly discarded (the existing version-mismatch path already did this —
`resetMapStateLocked`/`nonNil*Map` helpers added so the discard path and a snapshot from before
these fields existed both leave every map non-nil, never triggering a nil-map write panic).

### Third finding: GetCampaignVersion/GetSegmentVersion silently fell back to the current resource

Flagged as an open question in the prior pass ("low confidence on whether this is intentional
leniency vs a bug"). Resolved this pass by checking AWS's own API reference docs for
`/v1/apps/{appId}/campaigns/{campaignId}/versions/{version}`: the documented response table
lists `404 NotFoundException` as "The request failed because the specified resource was not
found" — a requested version number absent from history is exactly that case. Fixed both ops to
404 instead of substituting the current campaign/segment under the wrong `Version` number in the
response (which would be actively misleading to a caller who explicitly asked for e.g. version 3
and silently got version 7's content labeled `"Version": 7`).

### Fourth finding: SMS channel and Email channel wire hygiene

- `updateSMSChannelRequest` accepted `PromotionalMessagesPerSecond`/`TransactionalMessagesPerSecond`
  from the request body and echoed back whatever the caller sent. The real `SMSChannelRequest` has
  no such fields — they exist only on `SMSChannelResponse` as AWS-computed account throughput. No
  real SDK client can send them (there's no field on the Go request struct to set), so this was
  harmless in practice, but per the audit brief's field-diff instruction it's wire-shape noise
  that shouldn't exist. Deleted from the request type.
- `updateEmailChannelRequest` was missing `OrchestrationSendingRoleArn`, a real field on both
  `EmailChannelRequest` and `EmailChannelResponse`. Added.

### DeleteVoiceTemplate template-version-history leak

`DeleteVoiceTemplate` was the only one of the five `Delete*Template` ops that didn't clean up its
`templateVersionHistory[name+"/VOICE"]` entry — `Delete{Email,InApp,Push,Sms}Template` all
`delete()` their corresponding key, `DeleteVoiceTemplate` didn't. Fixed; locked by
`TestDeleteVoiceTemplate_ReleasesVersionHistory` in `leak_test.go`.

### funlen nolint removed

`Handler.GetSupportedOperations` carried `//nolint:funlen` over a ~140-line literal list of
operation-name strings. Decomposed into one small per-resource-family helper function each
(`supportedOpsAppFamily`, `supportedOpsCampaignFamily`, ...), concatenated by the now-short
`GetSupportedOperations`. No package-level state introduced — each helper returns a fresh local
slice, so there was no need for a `sync.OnceValue`/`gochecknoglobals` route-table pattern here
(that pattern is for lookup tables consulted per-request; this list is built once per call and is
cheap either way).

### Traps for the next auditor (looks-wrong-but-correct)

- `GetApplicationDateRangeKpi`/`GetCampaignDateRangeKpi`/`GetJourneyDateRangeKpi` always return
  an empty `KpiResult.Rows` slice. This is intentional — gopherstack has no analytics engine to
  compute real KPI rows, and an empty-but-correctly-shaped result is the honest emulation choice
  (not a disguised no-op, since the ops do validate the parent resource exists and return the
  real response envelope). Do not re-flag without a concrete plan for synthesizing KPI data.
- `UpdateTemplateActiveVersion` is a genuine no-op on the version-history data structure (the
  last entry in `templateVersionHistory` already *is* the active version by construction — every
  `Update*Template` call appends), but it still validates the template exists and returns
  `NotFoundException` correctly, and returns the real `202 Accepted` envelope. Confirmed correct,
  not a stub.
- `tags` uses a lowercase JSON key while everything else is PascalCase (see protocol note above)
  — this is real AWS behavior, not a bug.
- `ADM`/`APNS`/`Baidu`/`Default`/`GCM` on `PushTemplate`, and `Attributes`/`Dimensions`/etc.
  elsewhere in this service, are intentionally generic `map[string]any` rather than fully typed
  structs — this matches the project's existing convention for nested platform-override objects
  (`Campaign.MessageConfiguration`, `Journey.Activities`, ...) and is round-tripped, not
  field-validated, by design. Do not re-flag as a gap without a concrete plan to type every nested
  object in the service consistently.

### Persistence

`persistence.go`'s `Snapshot`/`Restore` now round-trips the ENTIRE backend: every
`store.Table`-backed resource (`apps`, `campaigns`, `channels`, `emailTemplates`, `endpoints`,
`eventStreams`, `exportJobs`, `importJobs`, `inAppTemplates`, `journeys`, `pushTemplates`,
`recommenders`, `segments`, `smsTemplates`, `voiceTemplates`) through `store.Registry`, plus the
map-shaped state (`appSettings`, `campaignVersions`, `segmentVersions`,
`templateVersionHistory`, `campaignActivities`, `journeyRuns`, `appEvents`, `sentMessages`,
`otpCodes`) as direct JSON fields on `backendSnapshot`. `pinpointSnapshotVersion` is `2`; an
older-version (or otherwise shape-mismatched) snapshot is discarded and the backend starts
empty rather than attempting a partial decode, same policy as before, now also resetting the
map-shaped state to non-nil empty maps on that path.

## 2026-08-30: paginated-listing reproducibility sweep (unstable page-boundary drop)

Targeted class: an offset-based cursor (`pkgs/page.New` for `GetApps`, the hand-rolled
`applyPageParams`/base64-offset scheme for `GetCampaigns`/`GetJourneys`/`GetSegments`)
over a listing re-sorted from a `*store.Table` map walk on every call. Read all 6
`sort.Slice` sites in the service.

**Found and fixed, 4 sites**: `GetApps` (`apps.go`), `GetCampaigns` (`campaigns.go`),
`GetJourneys` (`journeys.go`), `GetSegments` (`segments.go`) all sorted solely by `Name`.
None of `CreateApp`/`CreateCampaign`/`CreateJourney`/`CreateSegment` checks for an
existing `Name` -- real Pinpoint doesn't require these names to be unique either. Because
these four use *offset*-based pagination (not a value cursor), the bug isn't a
deterministic single-call drop like a Marker cursor -- it's that the full list gets
re-sorted from a fresh, differently-ordered map walk on every page request, so a tie
group's relative order can shuffle between the call serving page 1 and the call serving
page 2, silently dropping or duplicating members at the offset boundary. Proven with
`TestHandler_GetApps_DuplicateNames_NoDropOrDupAcrossPages` (`apps_test.go`),
`TestHandler_GetCampaigns_DuplicateNames_NoDropOrDupAcrossPages` (`campaigns_test.go`),
`TestHandler_GetJourneys_DuplicateNames_NoDropOrDupAcrossPages` (`journeys_test.go`), and
`TestHandler_GetSegments_DuplicateNames_NoDropOrDupAcrossPages` (`segments_test.go`),
each looped 30x (map-iteration-dependent, so it doesn't reproduce every run) -- all four
confirmed failing against unmodified code, passing after. Fixed by sorting on `(Name,
ID)` in all four functions, `ID` being each table's own unique key
(`appKeyFn`/`campaignKeyFn`/`journeyKeyFn`/`segmentKeyFn`).

**Confirmed safe**: `GetRecommenderConfigurations` sorts by `ID` (`recommenderKeyFn`,
unique) -- unaffected. The combined multi-type template listing (`templates.go`) sorts by
`(TemplateName, TemplateType)`; `TemplateName` is each per-type table's own key
(`emailTemplateKeyFn`/etc.), so within one type it's already unique, and the `TemplateType`
tiebreak disambiguates across types -- confirmed already correct, no fix needed.

**Confirmed ignoring pagination entirely** (a different, disclosed completeness gap, not
this pass's target -- can't drop a record at a page boundary that never truncates):
`GetExportJobs`, `GetImportJobs`, `ListTemplates`, `ListTemplateVersions`, `GetChannels`,
`GetUserEndpoints`, `GetRecommenderConfigurations` all return every item unbounded, with
no `maxResults`/`pageSize`/`NextToken` support at all (`grep -rln NextToken
services/pinpoint/*.go` finds only `handler_apps.go`, `handler_campaigns.go`,
`handler_journeys.go`, `handler_segments.go`). Real Pinpoint paginates several of these;
left as-is, out of this pass's scope.

**Test-suite gap this pass filled**: the pre-existing `TestHandler_GetAppsPagination` and
`TestHandler_GetAppsContinuation` only ever used distinct app names
(`app-a`/`app-b`/`app-c`) -- no existing test in the service constructed a tie or compared
item identity across a paginated walk before this pass.

Gate output (this pass, `services/pinpoint/` only): `go build ./services/pinpoint/...`
clean; `go vet ./services/pinpoint/...` clean; `go test ./services/pinpoint/... -race
-count=1` -- `ok`; `golangci-lint run ./services/pinpoint/...` -- `0 issues.`

## 2026-08-30 gopherstack-wlo1: error-envelope sweep, confirmed clean

Pinpoint is restjson1 (`aws-sdk-go-v2/service/pinpoint@v1.42.4`:
`awsRestjson1_` prefix). Read all 122 `deserializeOpError` functions in
`deserializers.go` (122-of-122, not sampled): all identically call
`restjson.GetErrorInfo(decoder)` (`aws-sdk-go-v2@v1.43.4`
`aws/protocol/restjson/decoder_util.go`) after checking the
`X-Amzn-ErrorType` response header, and `GetErrorInfo` itself checks body
key `Code` before `__type` (tag `json:"__type"`), with `Message`/`message`
for the message. `handler.go`'s `writeErrorResponse` writes
`{"message": ..., "__type": ...}` with no header -- satisfies the body
`__type` fallback (header absent -> `jsonCode` from body is used). Grepped
every `writeErrorResponse` call site (215) and every direct
`http.Status{Bad,NotFound,...}` use in the package: all route through
`writeErrorResponse`, no bypass found.

No bug found. Added `TestErrorEnvelope_GetAppNotFoundDecodesToTypedError`
(`error_envelope_test.go`), driving a real `pinpointsdk.Client` through
`GetApp` for a nonexistent app: asserts `errors.As` unwraps to the concrete
`*types.NotFoundException`, and separately asserts on the raw response
bytes for the same case (raw HTTP request needs an `Authorization` header
naming the SigV4 credential scope `mobiletargeting` -- Pinpoint's actual
signing name, not `pinpoint` -- since `RouteMatcher` reads it via
`httputils.ExtractServiceFromRequest`). Passed against unmodified code,
confirming this service's error envelope was already wire-correct.

Gates (this pass, `services/pinpoint/` only): `go build`, `go vet`,
`go test -race -count=1`, `golangci-lint run` -- all clean.

## 2026-08-31: error-envelope sweep (gopherstack-uox6, errtargetaudit)

`errtargetaudit -dir pinpoint` reported 5 class-A findings, all
`code=ConflictException mechanism=sentinel reference`, ops
`[CreateEmailTemplate CreateInAppTemplate CreatePushTemplate
CreateSmsTemplate CreateVoiceTemplate]`. This service's error matching is
`awsRestjson1_deserializeOpError<Op>` (per-op switch in
`deserializers.go`). Verified each op's own switch
(pinpoint@v1.42.4): all five declare exactly `[BadRequestException,
ForbiddenException, InternalServerErrorException,
MethodNotAllowedException, TooManyRequestsException]` — no
`ConflictException` at all. `UpdateJourney` is the package's only op that
legitimately declares `ConflictException`. All 5 real, 0 false positives.

**Root cause (1 shared call site, all 5 ops)**: `handleCreateTemplate`
(`handler_templates.go`) has a single `errors.Is(creationErr,
ErrAlreadyExists)` case shared by all five `CreateXTemplate` REST paths,
writing `409 ConflictException`. `ErrAlreadyExists` has exactly 5 emission
sites (`templates.go`, one per Create op) and this is its only consumer
in the package, so this is not really a "sentinel correct for most
callers" case — 0 legitimate callers exist for the code this site was
emitting. Fixed by changing the one shared case to `400
BadRequestException` (the closest declared client-error type, same
resolution as this campaign's other "no NotFound/Conflict type declared"
findings). `ErrAlreadyExists` itself is untouched.

5 existing tests asserted only the HTTP status
(`TestDuplicateEmailTemplate`, `TestDuplicateInAppTemplate`,
`TestVoiceTemplate_DuplicateRejected`, `TestDuplicatePushTemplate`,
`TestDuplicateSmsTemplate` — `templates_email_test.go`,
`templates_test.go`, `templates_push_test.go`), each a single
`assert.Equal(t, http.StatusConflict, rec2.Code)`; corrected in place to
`http.StatusBadRequest`, assertion count unchanged (1 each). New
`TestCreateTemplate_DuplicateName_RealClient`
(`errtargetaudit_duplicate_template_test.go`, 5 subtests) drives the real
`aws-sdk-go-v2/service/pinpoint` client and asserts `errors.As` unwraps to
`*types.BadRequestException`, not `*types.ConflictException`; confirmed
all 5 fail against unmodified code (`api error ConflictException`).

Gates: `go build`, `go vet` (repo-wide, clean), `go test -race -count=1`,
`golangci-lint run` — all clean (`./services/pinpoint/...`).

### gopherstack-coib (2026-09-06): PayloadTooLargeException wired for the ops with an observable request body

`PayloadTooLargeException` (`pinpoint@v1.42.4/types/errors.go:179`) is modeled by 113 of the
122 ops with a `deserializeOpError<Op>` function (digit-safe-extracted:
`awk "/^func awsRestjson1_deserializeOpError<Op>\(/,/^}/" deserializers.go | grep -oE
'"[A-Za-z0-9]+"'`) but the handler never emitted it. `types/errors.go`'s doc comment on the
type ("Provides information about an API request or response.") is generic boilerplate shared
verbatim by all 8 exception types in that file, not resource-specific documentation, so it
carries no numeric information.

The pinned SDK has no numeric size field to verify a threshold against, so the number is
**sourced from AWS documentation, not the SDK**: https://docs.aws.amazon.com/pinpoint/latest/developerguide/quotas.html,
API request quotas section, verbatim: "The maximum size of an invocation (request and response)
payload is 7 MB, unless otherwise specified for a particular type of resource." That qualifier
matters: the same page specifies smaller limits for two resource types --

- Event ingestion quotas: "Maximum size of a request | 4 MB" (supersedes the general 7 MB for
  `PutEvents`).
- Endpoint quotas: "Endpoint size | Maximum size 15 KB" (supersedes the general 7 MB for a
  single `UpdateEndpoint` body). The same table's `EndpointBatchItem`/`EndpointBatchRequest`
  row reaffirms the general 7 MB for `UpdateEndpointsBatch` rather than overriding it, so that
  op keeps the general limit.

A single blanket 7 MB check across every op would have contradicted the page it cites, so
implementation is scoped: the general 7 MB ceiling (`maxInvocationPayloadBytes`,
`payload_size.go`) applies to every op below that models the exception and has no more specific
documented limit; `PutEvents` and `UpdateEndpoint` get their own, smaller constants.

**39 ops enforced** (every write handler where `httputils.ReadBody`'s raw byte length is
observable before JSON-decoding): `CreateApp`, `CreateCampaign`, `CreateExportJob`,
`CreateImportJob`, `CreateJourney`, `CreateRecommenderConfiguration`, `CreateSegment`,
`PhoneNumberValidate`, `PutEventStream`, `PutEvents`, `RemoveAttributes`, `SendMessages`,
`SendOTPMessage`, `SendUsersMessages`, `Update{Adm,Apns,ApnsSandbox,ApnsVoip,ApnsVoipSandbox,
Baidu,Email,Gcm,Sms,Voice}Channel`, `UpdateApplicationSettings`, `UpdateCampaign`,
`UpdateEndpoint`, `UpdateEndpointsBatch`, `Update{Email,InApp,Push,Sms,Voice}Template`,
`UpdateJourney`, `UpdateJourneyState`, `UpdateRecommenderConfiguration`, `UpdateSegment`,
`UpdateTemplateActiveVersion`, `VerifyOTPMessage`.

**Deliberately left unenforced**, both disclosed as `gaps` above:

- The other 74 ops that model `PayloadTooLargeException` are GET/DELETE ops with no
  meaningful request body in this handler (an empty body can never exceed any positive
  threshold), plus the 9 ops the SDK does *not* model the exception on at all
  (`TagResource`/`UntagResource`/`ListTagsForResource`,
  `Create{Email,InApp,Push,Sms,Voice}Template`) are untouched. AWS's own exception applies to
  the response side too ("invocation (request **and response**) payload"), which would in
  principle apply to these read ops' output, but this backend has no chokepoint that measures
  an assembled response body's size before writing it, and retrofitting one is a materially
  larger change than wiring the modeled-but-unemitted exception this bug is about.
- `PutEvents`' per-individual-event quota ("Maximum size of an individual event | 1,000 KB",
  same Event ingestion quotas table) is not enforced. Once the raw body is JSON-decoded into
  `putEventsRequest` (`wire.go`), an individual event's raw byte length is no longer observable
  without a raw-message decode path (e.g. `map[string]json.RawMessage`) this handler doesn't
  have; adding one is a distinct, larger change than the request-level check, which reuses the
  same raw-`body`-length pattern already used everywhere else in this fix.

7 MB / 4 MB are implemented as `7 * 1024 * 1024` / `4 * 1024 * 1024` bytes (binary, not decimal)
to match this repo's existing convention for AWS's own "MB" quota language
(`pkgs/httputils.MaxRequestBodyBytes` documents Lambda's "6 MB" synchronous-invoke limit as
6 MiB); 15 KB is `15 * 1024` bytes for the same reason.

Regression coverage: `payload_size_test.go` boundary-tests all three thresholds (at-limit
succeeds, one byte over is rejected with `PayloadTooLargeException`/413) via `CreateApp`
(general), `PutEvents` (event-specific, plus a case between 4 MB and 7 MB proving the specific
limit — not the general one — governs), and `UpdateEndpoint` (endpoint-specific, plus a case
between 15 KB and 7 MB proving the same). All three failed against pre-fix code (verified by
reverting the 15 touched files to `HEAD`, confirming the package still built, running the new
tests to see them fail with the exact predicted status-code mismatch, then restoring the fix
byte-for-byte).

## 2026-09-08: writeErrorResponse nil-on-write fall-through audit (gopherstack-246v) -- found and fixed

Part of the sweep following the elasticache fix (gopherstack-8haq): `writeErrorResponse`
(`handler.go:506`) writes the JSON error body and unconditionally `return nil`s. Any helper
that rejects via `return writeErrorResponse(...)` and is called by code storing and checking
the result would get a silent nil and fall through past the rejection.

**Method (mechanical).** A `go/parser`/`go/ast` script over every non-test file in this
package (26k lines, the largest in this batch after quicksight, flat, no subdirectories)
computed the fixed-point closure seeded with `writeErrorResponse`: find every function with a
bare `return <sink>(...)`, add it, repeat to convergence. `ServeHTTP` (`handler.go:383`,
returned as-is by `Handler()` and registered directly with echo) is pinpoint's dispatch
entry; its own unrecognized-path fallback and every `dispatchXxx` sub-router it calls end
their default cases in a direct `return writeErrorResponse(...)`, pulling them into the
closure automatically along with the ~85 `handleXxx` op handlers those routers call — no
separate dispatch/non-dispatch partition was needed, since the same call-site sweep covers
both.

The closure converged at 107 functions. Every call site of every one of those 107 was
re-walked and classified: 340 total call sites. 336 are `return <fn>(...)` (direct-return,
safe) -- this includes 3 sites where `writeErrorResponse`'s result is stored but explicitly
discarded (`_ = writeErrorResponse(...)`) inside a `bool`-returning checked-helper
(`unmarshalBody`, `checkPayloadSize`, mirroring mwaa's `decodeJSONBody` pattern), which is
safe: callers check the returned `bool`, never the discarded `error`.

**One broken instance found**, at `handler_templates.go:355` (now fixed) in
`handleUpdateTemplate`:

```go
if updateErr := h.applyTemplateUpdate(c, body, templateName, templateType); updateErr != nil {
    return updateErr
}
httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusAccepted,
    messageBodyResponse{Message: acceptedMessage})
return nil
```

`applyTemplateUpdate` and the five `update{Email,InApp,Push,SMS,Voice}TemplateFromBody`
functions it dispatched to each had exactly one caller and each rejected (unknown template
type, invalid JSON body, or a backend error such as the template not existing) via a direct
`return writeErrorResponse(...)` / `return writeNotFoundOrInternal(...)` — correct in
isolation, but that made `applyTemplateUpdate` return `nil` on *every* path, success or
rejected. `handleUpdateTemplate`'s `updateErr != nil` guard therefore never fired, and a
rejected `PUT /v1/templates/{name}/{type}` always fell through to writing a second, spurious
`202 Accepted` response on top of the already-committed rejection.

Unlike elasticache's instance, no double *backend* mutation results (none of the five
`update*FromBody` functions call the backend again after a rejection), so the observable
damage is a corrupted HTTP response body, not a phantom-created resource. Status code alone
does not distinguish fixed from broken: echo's own `Response.WriteHeader` guards on
`Committed` and silently drops the second status, so `rec.Code` is 404 either way. But the
`httptest.ResponseRecorder` used by this package's tests has no `Content-Length`
enforcement (unlike a real `net/http` server, which would return `http.ErrContentLength`
from the second `Write` and truncate it) — so the second write's bytes land in `rec.Body`
verbatim. `TestUpdateTemplate_RejectedUpdate_DoesNotDoubleWrite`
(`handler_templates_rejection_test.go`) asserts on that: against unmodified code it fails
with body
`{"__type":"NotFoundException","message":"NotFoundException: app not found"}{"Message":"Accepted"}`
(concatenated, invalid JSON; `json.Unmarshal` on it fails with `invalid character '{' after
top-level value`), and an `echo: response already written to client` ERROR log fires from
echo's own guard. Confirmed failing against the pre-fix tree before applying the fix, then
passing after.

**Fix** (one caller at each level, so per the elasticache pattern: raw unwritten error mapped
at the call site, no sentinel needed): `applyTemplateUpdate` and the five
`update*TemplateFromBody` functions no longer take `*echo.Context` or write anything — they
return the raw backend error, or the existing `errInvalidRequestBody` sentinel (already used
elsewhere in this file for the same "bad JSON" case) for a decode failure, or a new
`errUnknownTemplateType` sentinel for an unrecognized type. `handleUpdateTemplate` maps and
writes the result exactly once, directly returning in every branch.

Gates: `GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/pinpoint/...` 0 issues;
`GOTOOLCHAIN=go1.27.0 go test -race ./services/pinpoint/...` ok (includes the new
regression test). No handler dispatch table changed, so no repo-wide blast-radius run was
needed beyond the package itself.
