---
service: apigateway
sdk_module: aws-sdk-go-v2/service/apigateway@v1.42.4
last_audit_commit: 01f7563b
last_audit_date: 2026-07-23
overall: A            # closed all 5 documented gaps + 3 deferred items from the 2026-07-11 sweep: RestApi.{ApiStatus,ApiStatusMessage,DisableExecuteApiEndpoint,EndpointAccessMode}, Stage.DocumentationVersion, ApiKey.StageKeys (Create + PATCH /stages), UsagePlan per-route throttle PATCH, Stage canarySettings.stageVariableOverrides PATCH, MethodSetting.{CacheDataEncrypted,UnauthorizedCacheControlHeaderStrategy} + their PATCH paths, and 2 concrete instances of the top-level-scalar-PATCH-remove gap (RestApi./description, Authorizer./identitySource). Found+fixed 2 new bugs while doing so (see Notes): a multi-op-per-request PATCH clobbering bug in the 3 resolvers this sweep touches, and UpdateUsagePlan returning an unprotected pointer into backend state. Found+documented (not fixed, out of assigned scope) a pre-existing UpdateDomainName PATCH gap.
# 2026-08-08 follow-up (bd: gopherstack-vvsy): fixed the multi-op-per-request clobbering bug in the remaining 6 resolvers; added applyDomainNamePatchOp so UpdateDomainName's nested "/endpointConfiguration/*" and "/mutualTlsAuthentication/*" PATCH paths no longer silently no-op; pointer-ified DomainName's certificateArn/regionalCertificateArn (a 3rd concrete PATCH-remove-on-scalar fix); re-verified UsagePlan throttle PATCH path shape against a fresh patch-operations.html fetch (already correct, no change needed). See gaps below for what's still open.
# 2026-08-09 follow-up (bd: gopherstack-npq5): added the DomainName/UsagePlan fields left missing by the prior follow-up — DomainName.{CertificateName,RegionalCertificateName,OwnershipVerificationCertificateARN} (*string on UpdateDomainNameInput, remove-supported per patch-operations.html) and .{ManagementPolicy,Policy,RoutingMode,EndpointAccessMode} (plain string, replace-only); UsagePlan.ProductCode (*string on UpdateUsagePlanInput, remove-supported). All seven flow through the existing single-segment PATCH machinery (applyTopLevelPatchOp + removableTopLevelScalar) with no new resolver code needed. Corrected the ticket: endpointConfiguration/vpcEndpointIds, which the ticket listed under DomainName, is documented only under UpdateRestApi's table, not UpdateDomainName's — left unmodeled here as a RestApi-scoped gap, out of this fix's scope. Verified against a live fetch of patch-operations.html plus aws-sdk-go-v2/service/apigateway@v1.42.4's deserializers.go (wire field names match exactly). Proven via both a pre-fix-failing unit suite and two real aws-sdk-go-v2-client integration tests (test/integration/apigateway_audit_test.go) that fail against the pre-fix binary (200 OK, field silently empty) and pass post-fix.
# 2026-08-11 follow-up (bd: gopherstack-oius): the previous "wire: ok" grading on UpdateResource/UpdateMethod/UpdateIntegration/UpdateIntegrationResponse/UpdateMethodResponse was WRONG — audited all 22 patchOperations-taking ops against a fresh patch-operations.html fetch; classification and fixes below (see the 5 op lines and Notes). Swept applyResourcePatchOp/applyStructuredPatch to return an error so a resolver can now REJECT an op/path/value combination (BadRequestException) instead of only ever silently applying-or-dropping it — used to reject AWS-documented-unsupported combinations (UpdateIntegration's "/type", any op but replace on "/parentId") and AWS-documented-but-unmodeled paths (Method.authorizationScopes, Integration.{integrationTarget,responseTransferMode,tlsConfig}) rather than fabricating support. Confirmed with real aws-sdk-go-v2 client tests (patch_ops_sdk_test.go) run against a git-worktree checkout of the pre-fix commit that every fixed case actually failed before (two as a hard 500 json-unmarshal-type-mismatch — UpdateIntegration's "/cacheKeyParameters" and "/timeoutInMillis", matching the "cannot be called successfully at all" bug class named in the ticket — the rest as a silent 200-OK no-op).
# 2026-08-11 follow-up (bd: gopherstack-6q5h): fixed the 3 items gopherstack-oius deferred. UpdateBasePathMapping's "/basepath"/"/restapiId" casing bug turned out to be deeper than casing alone (see Notes) — fixed with a dedicated resolver plus a real rename path in InMemoryBackend.UpdateBasePathMapping (previously the backend could never rename a mapping's base path at all, regardless of PATCH path casing). UpdateAuthorizer's "/providerARNs" and UpdateAccount's "/features" both had real backend state to support them and are now implemented as list-membership add/remove, matching this file's existing /stages, /binaryMediaTypes, /cacheKeyParameters pattern. Swept all 22 ops' documented paths against their Update*Input json tags for the same casing question: no other mismatch found (see Notes).
# 2026-08-21 follow-up (bd: gopherstack-eax4): fixed GetSdk's header-vs-body confusion. handler_sdk.go's opGetSdk action returned {"contentType","contentDisposition","body"} as a JSON map, which dispatch()/dispatchAndRespond() then JSON-marshalled with Content-Type application/json — a real client's ContentType/ContentDisposition decoded as zero values and Body as garbage. Added a general escape hatch to the dispatch chain (handler.go's rawBinaryResponse type; dispatch()/dispatchAndRespond()/handleJSONProtocol()/dispatchRestAPISpec() all now check for it before JSON-marshalling) so any actionFn can opt out of the JSON envelope and write real headers + a raw body via c.Blob, mirroring iotdataplane's GetThingShadow / medialive's DescribeInputDeviceThumbnail (gopherstack-tp8x) c.Blob-with-real-headers pattern — those two write directly to echo.Context from a per-route handler, which apigateway's actionFn (func([]byte) (int, any, error), no echo.Context) can't do directly, so this closes the same gap through dispatch()'s existing choke point instead of threading echo.Context through every actionFn. Audited every other apigateway op for the same class: GetExport is the only other op with output-side HTTP header bindings in apigateway@v1.42.4 (the 3 ImportRestApi/PutRestApi/ImportApiKeys/ImportDocumentationParts ops with Body []byte carry it on their *Input*, already handled correctly by isRawBodyAPIGWAction/dispatchRestAPISpec's existing raw-request-body path). GetExport's body was never double-JSON-wrapped (the export map was already served as the sole JSON payload) and its Content-Type happened to already read application/json correctly (this emulator only ever produces JSON), but it never set Content-Disposition; fixed via the same rawBinaryResponse mechanism. AWS's docs (API_GetExport.html) confirm ContentDisposition is a real response header but do not specify its value's format, so the filename this emulator sends is a synthesized, non-wire-mandated convention, same as GetSdk's pre-existing ContentDisposition in sdk.go. Proven with real aws-sdk-go-v2-client tests (TestAPIGateway_GetSdk_HeadersNotBody_RealClient, TestAPIGateway_GetExport_HeadersNotBody_RealClient) that fail against the pre-fix code (verified via hand-revert: ContentType decoded as "application/json" instead of "application/octet-stream", ContentDisposition nil) and pass post-fix. The pre-fix TestAPIGateway_GetSdk test asserted the broken JSON shape directly (unmarshalling the response body into a map and reading resp["contentType"]/resp["body"]) — replaced with the real-client test.
# 2026-09-04 follow-up (bd: gopherstack-fum): data-plane routing audit. FIXED: (1)
# handleProxyRequest never checked whether the URL's {stage} segment named a real,
# deployed stage -- an undeployed RestApi (PutMethod/PutIntegration configured, no
# CreateDeployment ever called) or a completely made-up stage name in the URL still
# routed straight through to the integration and executed it; also, an unmatched
# resource/method returned a bare 404 "page not found" instead of AWS's real 403
# "Missing Authentication Token". Both fixed in proxy.go (GetStage gate +
# writeMissingAuthenticationTokenResponse), verified fail-before/pass-after with
# TestHandleProxyRequest_RequiresDeployedStage plus updated assertions in 4 existing
# tests that had been asserting the wrong 404 status. (2) h.trieCache (the compiled
# per-API routing-trie cache) was never evicted on DeleteRestApi -- since RestApi IDs
# are fresh-random per CreateRestApi, a deleted API's entry can never be overwritten
# and stays in process memory for the server's lifetime; fixed in
# handler_rest_apis.go's deleteRestAPIAction, verified with
# TestDeleteRestAPI_EvictsTrieCache (fails pre-fix, passes post-fix). NOT FIXED, see
# gaps: the "AWS" (non-proxy) integration type unconditionally treats its target as a
# Lambda function regardless of what service the URI actually names -- confirmed a
# real DynamoDB/SQS/SNS/S3/Step-Functions direct "AWS" integration (a common,
# AWS-documented pattern) is accepted at PutIntegration with no validation but never
# executes the target action; fixing this needs new per-service invoker wiring in
# cli.go, out of this pass's services/apigateway-only scope. Also documented (not
# fixed, large architectural change): CreateDeployment does not freeze a routable
# snapshot -- the data plane always matches against the RestApi's LIVE current
# resource/method/integration state regardless of which deployment a stage is
# nominally pinned to, so editing or deleting a resource after deployment takes
# effect on the already-deployed stage immediately, with no new deployment required
# (confirmed by reproduction: delete a resource post-deploy, no redeploy, and the
# already-deployed stage 403s immediately instead of continuing to serve the old
# resource). Everything else audited this pass (AWS_PROXY/Lambda event+response
# shape, MOCK, HTTP/HTTP_PROXY, TOKEN/REQUEST/COGNITO_USER_POOLS authorizers, API
# key + usage plan enforcement, request validators) is real and wired -- see the
# per-integration-type verdict table in the dated section at the end of this file.
# 2026-09-06 follow-up (bd: gopherstack-is2a): PARTIALLY FIXED the "AWS" (non-proxy)
# integration target gap gopherstack-fum documented above. Confirmed the bug first:
# an "AWS" integration whose URI names sqs or sns (per aws-sdk-go-v2's documented
# grammar, arn:aws:apigateway:{region}:{subdomain.service|service}:path|action/{service_api},
# types.go:653-666) was unconditionally invoked as Lambda, reaching neither queue nor
# topic. Wired two targets with a real backing service and an unambiguous action: sqs
# path-style (arn:aws:apigateway:{region}:sqs:path/{accountId}/{queueName}) ->
# SendMessage, and sns action-style (arn:aws:apigateway:{region}:sns:action/Publish)
# -> Publish, each behind a new SetSQSSender/SetSNSPublisher hook (proxy.go, wired in
# cli.go's wireAPIGatewaySQSSNS, reusing the existing sqsSenderAdapter/
# snsPublisherAdapter already declared for eventbridge). No VTL library exists in this
# module, so this is a documented simplified passthrough, not mapping-template
# evaluation: the rendered request payload (raw body, or the existing $input.json/
# $input.path/$util.* RenderTemplate machinery in vtl.go if a requestTemplate is
# configured -- reused unchanged, not reimplemented) becomes the SQS MessageBody or
# SNS Message verbatim, with none of real API Gateway's Action=...&... AWS
# query-protocol encoding; the SNS TopicArn is resolved from the integration's
# RequestParameters mapping or a "TopicArn" query parameter, since it is not encoded
# in an action-style URI; and a successful call's HTTP response is a bare "{}" run
# through the same applyResponseTemplate status-code/response-template matching the
# Lambda path already used, not a real SQS/SNS response shape. DynamoDB, Kinesis, and
# Step Functions direct integrations, and sqs/sns integrations with no hook wired,
# are UNCHANGED -- still fall through to the original Lambda-invoke path, which is a
# deliberate silent no-op (~150 services build test backends with no cross-service
# hooks) rather than a new rejection; TestHandleAWSIntegration_{SQSTarget,SNSTarget}_
# Unwired proves this. TestHandleAWSIntegration_LambdaTarget_StillRoutesToLambda
# proves the Lambda path is unaffected. All four new dispatch tests were verified to
# fail against the pre-fix code (git-show revert of proxy_integrations.go alone,
# package still compiles since the new SetSQSSender/SetSNSPublisher hooks and
# interfaces stay in proxy.go/handler.go).
# 2026-09-06 follow-up (bd: gopherstack-8mge): closed a silent-success defect and two
# untested guards found while verifying gopherstack-is2a. dispatchSQS's segment-count
# mismatch branch returned nil (success, HTTP 200 with no message sent) instead of an
# error -- unreachable today since canDispatchToTarget validates spec first via
# sqsQueuePathValid, but nothing would catch it if the two ever drift; now returns
# errSQSQueuePathMalformed. Also added coverage proving the two guards actually
# matter: sqsQueuePathValid (TestHandleAWSIntegration_SQSTarget_MalformedPath) and
# awsIntegrationTarget's URI field-count guard
# (TestHandleAWSIntegration_ShortURINotParsedAsServiceTarget, whose absence panics on
# parts[3]/parts[4]/parts[5] indexing) -- both previously passed with the guard
# neutered; both now fail when neutered.
# 2026-09-08 follow-up (bd: gopherstack-9ard): audited the filed-title-only ticket
# "CreateDeployment does not freeze a snapshot; the data plane always serves the live
# resource state". CONFIRMED, and this is the SAME structural gap gopherstack-fum
# already documented 2026-09-04 (see the gaps entry below) — re-verified against the
# current code, unchanged: proxy_routing.go's routingTrie still always calls
# Backend.ResourcesForRouting(apiID) (the RestApi's LIVE resource tree), and
# Deployment.APISummary (deployments.go's apiSummary()) is still only a display-only
# summary (matching the real SDK's ApiSummary response field, api_op_CreateDeployment.go
# CreateDeploymentOutput.ApiSummary and api_op_GetDeployment.go), not something the data
# plane routes against. The pre-existing "real snapshot of resources/methods/
# integrations at deploy time" wording on this file's CreateDeployment line (below) was
# INACCURATE and has been corrected — apiSummary snapshots METADATA (authorizationType,
# apiKeyRequired) for display, not routable resource/method/integration state. Properly
# fixing this needs a real per-deployment snapshot plus stage-to-deployment pinning in
# the data plane, same as gopherstack-fum already concluded — NOT attempted here,
# structural/out of scope. Checked the three AWS-enforceable adjacent behaviors the
# ticket also named, all fixable with existing state (no snapshot model required):
# (1) DeleteDeployment when a stage still references it — ALREADY CORRECT
# (deployments.go rejects with BadRequestException, matching the real DeleteDeployment
# doc: "Deleting a deployment will only succeed if there are no Stage resources
# associated with it.", api_op_DeleteDeployment.go:11-12), pinned by pre-existing
# TestDeleteDeployment_StageProtection. No change needed. (2) Stage.deploymentId naming
# a nonexistent Deployment — CreateStage ALREADY validates this (stages.go), but had NO
# test pinning the guard; added TestCreateStage_RejectsNonexistentDeploymentID.
# UpdateStage did NOT validate this at all (a stage could be repointed, via a top-level
# "/deploymentId" replace PATCH or via the AWS-documented canary-promotion "copy" op
# {"op":"copy","from":"/canarySettings/deploymentId","path":"/deploymentId"}, at a
# deploymentId naming no real Deployment) — FIXED to match CreateStage's guard, proven
# failing pre-fix / passing post-fix by new TestUpdateStage_RejectsNonexistentDeploymentID.
# The guard runs before any field mutation in UpdateStage, not after, since `stage` is a
# live pointer into backend state (b.stages.Get) and validating mid-function would have
# left earlier fields (e.g. Description) partially applied on an error return. Strengthened
# the pre-existing Test_ApplyStructuredPatch_StageCanaryPromotion (patch_test.go), which
# used to promote a fabricated "canary-depl-id" that named no real Deployment — this guard
# now correctly rejects that, so the test was changed to promote a real second
# CreateDeployment result instead (still exercises the same "copy" op path). (3) deploying
# an API with a method that has no integration — NOT what the ticket assumed ("no
# resources/methods at all" deploys successfully in real AWS and must keep doing so here,
# confirmed by pre-existing TestBackend_DeploymentAndStage/create_deployment_and_stage,
# which deploys a bare zero-resource API); investigated whether real AWS rejects a method
# that EXISTS but has no integration configured, and found no authoritative evidence that
# it does. grep -rn "No integration" over the pinned aws-sdk-go-v2/service/apigateway
# module returns nothing; botocore's wire model documents CreateDeployment in one
# sentence ("Creates a Deployment resource, which makes a specified RestApi callable
# over the internet"), with no mention of integrations or methods; BadRequestException
# is in CreateDeployment's modeled error list, but it's modeled for nearly every
# operation in this service, so its presence isn't evidence of this specific
# precondition. The only support found for the claimed behavior was a third-party tool
# (github.com/mdlavin/find-api-gateway-methods-missing-integrations) — below this repo's
# bar for a change that REJECTS requests the emulator previously accepted. An initial
# attempt at a deploy-time integration guard was backed out here after it broke a
# pre-existing test exercising a legitimate deploy flow. NOT ENFORCED: gopherstack
# deliberately does not reject CreateDeployment for a method with no integration —
# guessing a rejection rule is worse than not enforcing one (a wrong rejection breaks
# working user code; a missing one only under-enforces).
ops:
  UpdateStage: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "prior sweep: PATCH semantics rewritten (/variables/{name}, canary-promotion copy op, /canarySettings/*, /accessLogSettings/*, per-route method settings, cacheCluster* fields). Prior sweep 2: documentationVersion field + PATCH added; /canarySettings/stageVariableOverrides whole-map-replace PATCH added; caching/dataEncrypted + caching/unauthorizedCacheControlHeaderStrategy per-route PATCH properties added. 2026-09-08 (gopherstack-9ard): FIXED — deploymentId was never validated against real Deployment state (unlike CreateStage's existing guard); now rejects a nonexistent deploymentId with NotFoundException. See the dated note above for detail; TestUpdateStage_RejectsNonexistentDeploymentID."}
  UpdateRestApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior sweep: PATCH /binaryMediaTypes/{escaped} add/remove merge, minimumCompressionSize coercion. This sweep: ApiStatus/ApiStatusMessage/DisableExecuteApiEndpoint/EndpointAccessMode fields added (Create + Update + PATCH replace); Description switched to *string so PATCH remove on /description actually clears it (was a silent no-op) — see Notes"}
  UpdateAccount: {wire: ok, errors: ok, state: ok, persist: ok, note: "CloudwatchRoleARN field added to UpdateAccountInput (previously unsettable at all); /throttle/{rateLimit,burstLimit} nested PATCH now merges. 2026-08-11 (gopherstack-6q5h): /features add/remove added (Features field added to UpdateAccountInput, nil-checked so removing the last feature actually clears it); remove of the UsagePlans feature and any op but add/remove are REJECTED (BadRequestException) per patch-operations.html — see Notes"}
  UpdateUsagePlan: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior sweep: /apiStages add/remove (value 'restApiId:stage') merge, len()>0 fix. This sweep: per-route throttle overrides added (/apiStages/{id:stage}/throttle/{resourcePath}/{httpMethod}[/rateLimit|burstLimit], remove of the whole entry at 5 segments, add/replace of one field at 6); also fixed UpdateUsagePlan returning an unprotected pointer into backend state (now returns a defensive copy like every other Update*). 2026-08-09 (gopherstack-npq5): ProductCode field added (*string, remove-supported) — was accepted-and-silently-dropped before — see Notes"}
  UpdateGatewayResponse: {wire: ok, errors: ok, state: ok, persist: ok, note: "now backed by a dedicated merge-based backend method (was reusing PutGatewayResponse's full-replace, silently wiping ResponseParameters/ResponseTemplates/StatusCode on every partial PATCH); /responseParameters/{key} and /responseTemplates/{key} per-entry PATCH added"}
  UpdateApiKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior sweep: enabled bool coercion, customerId field. This sweep: StageKeys field + PATCH /stages add/remove added (value '{restApiId}/{stageName}', deprecated-for-usage-plans per the SDK doc comment but still real and wire-modeled) — see Notes"}
  UpdateUsage: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: verified against AWS's patch-operations.html + CLI reference that the real (and only) supported path is the single-segment scalar /remaining, NOT a per-date path as the prior code comment and test claimed; behavior was already correct (the backend loop only reads map values, not keys) but doc/test were misleading — corrected both, see Notes"}
  UpdateRequestValidator: {wire: ok, errors: ok, state: ok, persist: ok, note: "validateRequestBody/validateRequestParameters bool coercion fixed"}
  UpdateMethod: {wire: ok, errors: ok, state: ok, persist: ok, note: "apiKeyRequired bool coercion fixed. 2026-08-11 (gopherstack-oius): /requestParameters/{name} and /requestModels/{content-type} per-key PATCH added (previously fell through the generic flatten and silently dropped — real client PATCHes never took effect); requestValidatorId field added to UpdateMethodInput (was entirely unsettable via PATCH); /authorizationScopes (AWS-documented add/remove) is REJECTED with BadRequestException — Method has no AuthorizationScopes field anywhere in this backend (PutMethod doesn't accept it either), so this is a real unmodeled-state gap, not a patch-plumbing bug — see gaps"}
  UpdateAuthorizer: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior sweep: authorizerResultTtlInSeconds int coercion. This sweep: IdentitySource switched to *string so PATCH remove on /identitySource actually clears it (was a silent no-op, AWS-documented as supported). 2026-08-11 (gopherstack-6q5h): /providerARNs add/remove added (previously a hard 500 — the raw Value JSON string unmarshaled straight into a []string field via the generic fallback); UpdateAuthorizer's ProviderARNs merge switched from a len()>0 to a != nil check so removing the last ARN actually clears it; replace REJECTED (BadRequestException, not supported per the doc table) — see Notes"}
  UpdateDeployment: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-11 (gopherstack-oius): /parentId (replace-only per patch-operations.html) added — was entirely missing from UpdateResourceInput, so a real client's PATCH silently no-opped a resource move. InMemoryBackend.UpdateResource now validates the new parent exists in the same RestApi and rejects a move into the resource's own subtree (BadRequestException), and recomputes Path for the moved resource AND its whole descendant subtree (Path is stored precomputed, not derived lazily). add/remove on /parentId rejected (replace-only per the doc table)"}
  UpdateDomainName: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-08 follow-up: applyDomainNamePatchOp added for /endpointConfiguration/{types,ipAddressType} and /mutualTlsAuthentication/{truststoreUri,truststoreVersion} (previously entirely unhandled, silently no-opped); certificateArn/regionalCertificateArn switched to *string so PATCH remove actually clears them. 2026-08-09 (gopherstack-npq5): certificateName/regionalCertificateName/ownershipVerificationCertificateArn (*string, remove-supported) and managementPolicy/policy/routingMode/endpointAccessMode (plain string, replace-only) added — all seven were accepted-and-silently-dropped before, now fields on DomainName + UpdateDomainNameInput. endpointConfiguration/vpcEndpointIds NOT added here: verified against a fresh patch-operations.html fetch that it's a UpdateRestApi-only path, not UpdateDomainName's, contra the tracking ticket — see Notes"}
  UpdateBasePathMapping: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-11 (gopherstack-6q5h): fixed the gopherstack-oius-deferred casing bug on /basepath and /restapiId, and a deeper bug it was masking — InMemoryBackend.UpdateBasePathMapping could not rename a mapping's base path AT ALL, under any casing, because the identity field (BasePath, populated from the URL) and the PATCH target were the same json name and the URL's old value always won after patch resolution. Both \"/basepath\" (patch-operations.html) and \"/basePath\" (AWS CLI reference's own worked example — the two AWS sources disagree) now stage into a dedicated NewBasePath field the backend applies as an actual key rename; \"/restapiId\" is now explicitly aliased too — see Notes"}
  UpdateDocumentationPart: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDocumentationVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateIntegration: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-11 (gopherstack-oius): PREVIOUSLY BROKEN — /cacheKeyParameters and /timeoutInMillis unmarshaled the wire's JSON-string Value directly into UpdateIntegrationInput's []string/int fields and failed the WHOLE PATCH request with a 500 (json: cannot unmarshal string into Go struct field ... of type []string / int) — a real client could not call this op with either field at all. Fixed: /cacheKeyParameters is now single-segment add/remove/idempotent-replace list membership (merges with existing, like /stages and /binaryMediaTypes elsewhere in this file); /timeoutInMillis added to patchFieldKind for string->int coercion. Also added: /requestParameters/{name} and /requestTemplates/{content-type} per-key PATCH (previously silently dropped, multi-segment path). REJECTED rather than silently accepted: /type (patch-operations.html documents it as \"Not supported\" for every op, but IntegrationType has a matching struct field so it previously succeeded silently and changed the integration type via PATCH — a real bug); /integrationTarget, /responseTransferMode, /tlsConfig/insecureSkipVerification (real aws-sdk-go-v2 Integration fields — IntegrationTarget *string, ResponseTransferMode, TlsConfig.InsecureSkipVerification, types.go:591,627,635,1262 — but entirely unmodeled by this backend; rejecting rather than fabricating support, see gaps)"}
  UpdateIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-11 (gopherstack-oius): /responseParameters/{name} and /responseTemplates/{content-type} per-key PATCH added (previously silently dropped, multi-segment path never matched any Update*Input json tag)"}
  UpdateMethodResponse: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-11 (gopherstack-oius): /responseModels/{content-type} and /responseParameters/{name} (bool, string-coerced) per-key PATCH added (previously silently dropped, multi-segment path)"}
  UpdateModel: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateClientCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateRestApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: DisableExecuteApiEndpoint/EndpointAccessMode inputs wired; ApiStatus always AVAILABLE (gopherstack creates RestApis synchronously, no UPDATING/PENDING/FAILED transition)"}
  GetRestApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: apiStatus/apiStatusMessage/disableExecuteApiEndpoint/endpointAccessMode now included in the response"}
  GetRestApis: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRestApi: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "{proxy+} greedy + trie-based routing (bd gopherstack fix #1403), parent-child tree verified"}
  GetResource: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResources: {wire: ok, errors: ok, state: ok, persist: ok, note: "embed=[methods] param honored"}
  DeleteResource: {wire: ok, errors: ok, state: ok, persist: ok}
  PutMethod: {wire: ok, errors: ok, state: ok, persist: ok, note: "authorizationType validated against NONE/AWS_IAM/CUSTOM/COGNITO_USER_POOLS; CUSTOM/COGNITO_USER_POOLS require authorizerId (400 otherwise)"}
  GetMethod: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMethod: {wire: ok, errors: ok, state: ok, persist: ok}
  PutMethodResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GetMethodResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteMethodResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  PutIntegration: {wire: ok, errors: ok, state: ok, persist: ok, note: "type validated (MOCK/AWS/AWS_PROXY/HTTP/HTTP_PROXY); VTL request/response templates real (vtl.go)"}
  GetIntegration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteIntegration: {wire: ok, errors: ok, state: ok, persist: ok}
  PutIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GetIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDeployment: {wire: ok, errors: ok, state: ok, persist: ok, note: "inline stage create/update via stageName param. 2026-09-08 (gopherstack-9ard): the 'real snapshot of resources/methods/integrations at deploy time' claim previously on this line was INACCURATE — corrected, see the dated note above and gopherstack-fum's gaps entry below: apiSummary is a display-only metadata summary, NOT something the data plane routes against. Investigated whether a method with no integration should reject CreateDeployment (BadRequestException) — found no authoritative evidence (neither the pinned Go SDK module nor botocore's wire model documents this precondition; only third-party tooling claimed it), so deliberately left unenforced rather than guessed at. Deploying an API with zero resources/methods at all remains allowed, matching real AWS (TestBackend_DeploymentAndStage/create_deployment_and_stage)."}
  GetDeployment: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDeployments: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified against apigateway@v1.42.4 serializers.go (prior grading was response-only). limit/position were never read at all -- every call returned the full unpaginated list regardless of Limit; now paginated via paginatePageByKey. Also found and fixed a service-wide bug in injectJSONFieldAPIGW: query-string limit was always JSON-quoted, so a real client's numeric Limit 500'd on json.Unmarshal into every Limit-typed handler struct (affected every list op with pagination, not just this one) -- limit is now injected as a bare JSON number."}
  DeleteDeployment: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-08 (gopherstack-9ard): audited the 'delete a deployment a stage still references' precondition — ALREADY CORRECT (rejects with BadRequestException, matching api_op_DeleteDeployment.go's doc comment), pinned by pre-existing TestDeleteDeployment_StageProtection. No change needed."}
  CreateStage: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior sweep: cacheCluster{Enabled,Size,Status} fields. Prior sweep 2: documentationVersion field added, wired through the stageSnapshot DTO for persistence. 2026-09-08 (gopherstack-9ard): the existing deploymentId-must-exist guard (stages.go) was correct but had NO pinning test — added TestCreateStage_RejectsNonexistentDeploymentID."}
  GetStage: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: documentationVersion now included in the response"}
  GetStages: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. deploymentId query filter (serializers.go:7042) was never read -- every call returned every stage on the REST API regardless of deploymentId; now filtered against Stage.DeploymentID."}
  DeleteStage: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAuthorizer: {wire: ok, errors: ok, state: ok, persist: ok, note: "TOKEN/REQUEST/COGNITO_USER_POOLS identitySource + TTL; cache bounded (bd gopherstack #1403)"}
  GetAuthorizer: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAuthorizers: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position (serializers.go:4264,4268) were never read -- always returned every authorizer in one page; now paginated."}
  DeleteAuthorizer: {wire: ok, errors: ok, state: ok, persist: ok}
  TestInvokeAuthorizer: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreateApiKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior sweep: customerId field. This sweep: StageKeys ([]types.StageKey -> validated + formatted '{restApiId}/{stageName}' strings, referenced stage must exist or NotFoundException) added — see Notes"}
  GetApiKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "customerId (prior sweep) and stageKeys (this sweep) now included in the response"}
  GetApiKeys: {wire: fixed, errors: ok, state: ok, persist: ok, note: "customerId (prior sweep) and stageKeys now included per item. 2026-08-29 wrapper-key sweep: REQUEST direction verified. Two real bugs: (1) includeValues query filter (serializers.go:4106, plural) was read under the wrong key \"includeValue\" (singular -- GetApiKey's own key, serializers.go:4036) so a real client's includeValues=true never returned key values; (2) customerId (serializers.go:4102) and nameQuery/\"name\" (serializers.go:4114) filters were never read at all -- always returned every key. Both APIKey.CustomerID and APIKey.Name already existed as backing fields, so these were real gaps, not modeling limits. An existing unit test (api_keys_test.go TestGetApiKeys_ValueHiddenByDefault) asserted the wrong singular key as correct -- corrected to \"includeValues\"."}
  DeleteApiKey: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateUsagePlan: {wire: ok, errors: ok, state: ok, persist: ok}
  GetUsagePlan: {wire: ok, errors: ok, state: ok, persist: ok}
  GetUsagePlans: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. keyId query filter (serializers.go:7521) was never read -- always returned every usage plan regardless of key association; now backed by new GetUsagePlansForKey (real usagePlanKeys index)."}
  DeleteUsagePlan: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateUsagePlanKey: {wire: ok, errors: ok, state: ok, persist: ok}
  GetUsagePlanKey: {wire: ok, errors: ok, state: ok, persist: ok}
  GetUsagePlanKeys: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. name query filter (serializers.go:7442) was never read -- always returned every key on the plan; now filtered against UsagePlanKey.Name."}
  DeleteUsagePlanKey: {wire: ok, errors: ok, state: ok, persist: ok}
  GetUsage: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. keyId query filter (serializers.go:7200) had no backing field on GetUsageInput at all -- always returned every key's usage on the plan; KeyID field added and now filters Items."}
  CreateModel: {wire: ok, errors: ok, state: ok, persist: ok}
  GetModel: {wire: ok, errors: ok, state: ok, persist: ok}
  GetModels: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position never read -- now paginated. GetModel's flatten query param (serializers.go:6009) remains a gap: Model.Schema is stored as an opaque string, no $ref resolver exists to distinguish flattened vs non-flattened output -- not fabricated."}
  DeleteModel: {wire: ok, errors: ok, state: ok, persist: ok}
  GetModelTemplate: {wire: ok, errors: ok, state: ok, persist: n/a}
  CreateRequestValidator: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRequestValidator: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRequestValidators: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position never read -- now paginated."}
  DeleteRequestValidator: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateBasePathMapping: {wire: ok, errors: ok, state: ok, persist: ok}
  GetBasePathMapping: {wire: ok, errors: ok, state: ok, persist: ok, note: "domainNameId gap, see GetBasePathMappings note"}
  GetBasePathMappings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position never read -- now paginated. domainNameId (serializers.go:4436, and on Create/Delete/Update/GetBasePathMapping/GetDomainName*/UpdateDomainName) is a gap across all of these -- no DomainNameID concept exists in this backend's models, not fabricated."}
  DeleteBasePathMapping: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDomainName: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDomainName: {wire: ok, errors: ok, state: ok, persist: ok, note: "domainNameId gap, see GetBasePathMappings note"}
  GetDomainNames: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. resourceOwner query filter (serializers.go:5307) was never read; sibling GetDomainNameAccessAssociations already had the SELF/OTHER_ACCOUNTS handling, GetDomainNames just never mirrored it -- now does (OTHER_ACCOUNTS returns empty, matching a backend that only ever creates self-owned resources)."}
  DeleteDomainName: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDomainNameAccessAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDomainNameAccessAssociations: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDomainNameAccessAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  RejectDomainNameAccessAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDocumentationPart: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDocumentationPart: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDocumentationParts: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. name/path/type filters and limit/position pagination (serializers.go:4896-4925) were ALL never read -- previously read only restApiId; now filtered against DocumentationPart.Location and paginated. locationStatus remains a gap: this backend has no separate \"documented version\" snapshot to distinguish DOCUMENTED/UNDOCUMENTED -- not fabricated."}
  DeleteDocumentationPart: {wire: ok, errors: ok, state: ok, persist: ok}
  ImportDocumentationParts: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDocumentationVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDocumentationVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDocumentationVersions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position never read -- now paginated."}
  DeleteDocumentationVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccount: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: limit/position (serializers.go:7117,7121) never read; left unfixed as a gap, not a bug, given tag maps per resource are small and bounded -- flagged for follow-up, not fabricated"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TestInvokeMethod: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetGatewayResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GetGatewayResponses: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position never read (fixed set of 12 default response types, so re-sorted by responseType only when paginating to satisfy cursor ordering)."}
  PutGatewayResponse: {wire: ok, errors: ok, state: ok, persist: ok, note: "unchanged: still a correct full replace for the real PUT operation"}
  DeleteGatewayResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GenerateClientCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  GetClientCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  GetClientCertificates: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position never read -- now paginated."}
  DeleteClientCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  GetVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  GetVpcLinks: {wire: fixed, errors: ok, state: ok, persist: ok, note: "2026-08-29 wrapper-key sweep: REQUEST direction verified. limit/position never read -- now paginated."}
  DeleteVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  GetExport: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "2026-08-21 (gopherstack-eax4): Swagger 2.0 + OAS 3.0 export, real per-API/stage synthesis. GetExportOutput's ContentType/ContentDisposition are HTTP response headers and Body is the raw payload (apigateway@v1.42.4 deserializers.go:10166 awsRestjson1_deserializeOpHttpBindingsGetExportOutput, :10183 awsRestjson1_deserializeOpDocumentGetExportOutput), never JSON fields. Body was already served correctly (the export map was the sole JSON payload, not wrapped under a field) and Content-Type already happened to read application/json correctly; Content-Disposition was never set. Now routed through handler.go's rawBinaryResponse mechanism with both headers set; ContentDisposition's exact value is a synthesized, non-wire-mandated filename (AWS's docs confirm the header but not a fixed format). Proven via TestAPIGateway_GetExport_HeadersNotBody_RealClient."}
  GetSdk: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "2026-08-21 (gopherstack-eax4): fixed the header-vs-body confusion found 2026-08-21 while fixing gopherstack-tp8x's medialive DescribeInputDeviceThumbnail (same bug class). Real GetSdkOutput's ContentType/ContentDisposition are HTTP response headers (apigateway@v1.42.4 deserializers.go:13316 awsRestjson1_deserializeOpHttpBindingsGetSdkOutput -- Content-Disposition/Content-Type header names) and Body is the raw binary payload (deserializers.go:13333 awsRestjson1_deserializeOpDocumentGetSdkOutput copies response.Body directly, no JSON parsing), never JSON fields. handler_sdk.go's opGetSdk action used to return {\"contentType\",\"contentDisposition\",\"body\"} as a map, JSON-marshalled by dispatch() with Content-Type application/json. Fixed by returning a *rawBinaryResponse (handler.go), which dispatch()/dispatchAndRespond()/handleJSONProtocol()/dispatchRestAPISpec() now special-case to write real headers + raw body via c.Blob instead of JSON-marshalling -- a general mechanism, not a GetSdk-only special case, following iotdataplane's GetThingShadow / medialive's DescribeInputDeviceThumbnail (gopherstack-tp8x) c.Blob-with-real-headers precedent (both write directly to echo.Context from a per-route handler; apigateway's actionFn signature has no echo.Context, so the escape lives in dispatch()'s shared choke point instead). Proven via TestAPIGateway_GetSdk_HeadersNotBody_RealClient, which fails against the pre-fix code (hand-revert confirmed: ContentType decoded \"application/json\", ContentDisposition nil) and passes post-fix. The old TestAPIGateway_GetSdk test asserted the broken JSON shape directly and was replaced."}
  GetSdkType: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetSdkTypes: {wire: ok, errors: ok, state: ok, persist: n/a, note: "2026-08-29 wrapper-key sweep: limit/position (serializers.go:6892,6896) never read; left unfixed since the catalog is a small fixed set (sdkTypeCatalog()), not user-controlled growth -- flagged for follow-up, not fabricated"}
  ImportApiKeys: {wire: ok, errors: ok, state: ok, persist: ok}
  ImportRestApi: {wire: ok, errors: ok, state: ok, persist: ok}
  PutRestApi: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  proxy_invocation: {status: fixed, note: "proxy.go: MOCK/AWS/AWS_PROXY/HTTP/HTTP_PROXY dispatch, VTL passthrough (WHEN_NO_MATCH/WHEN_NO_TEMPLATES/NEVER), Lambda invoke via injected LambdaInvoker, usage-plan enforcement returns real 429 (LimitExceededException/TooManyRequestsException) via writeThrottleResponse — separate, already-correct code path from the control-plane handleError switch. 2026-09-04 (gopherstack-fum): FIXED -- handleProxyRequest never verified the URL's {stage} segment named a real deployed stage, so an undeployed RestApi (or any made-up stage name) still routed to and executed the configured integration; unmatched routes also returned a bare 404 instead of AWS's real 403 'Missing Authentication Token'. Both fixed (GetStage gate + writeMissingAuthenticationTokenResponse). Per-integration-type verdict: AWS_PROXY (Lambda proxy) wired and executing, event/response shape correct; AWS (Lambda, non-proxy, VTL) wired and executing; AWS (sqs SendMessage / sns Publish direct integration) FIXED 2026-09-06 (gopherstack-is2a) -- dispatches to the wired SQS/SNS hook via a simplified passthrough, see the dated note below and gaps; AWS (other non-Lambda service target, e.g. DynamoDB/Step Functions direct integration) STILL accepted at PutIntegration with no validation but NEVER executes -- see gaps; HTTP/HTTP_PROXY wired and executing (real outbound HTTP request); MOCK wired and executing; VPC_LINK is not a distinct Integration.Type (it's HTTP/HTTP_PROXY + connectionType=VPC_LINK) and is routed identically to a plain HTTP_PROXY request to integration.URI -- no real VPC/NLB emulation exists to route through, a structural simplification, not separately verified against connectionId. TOKEN/REQUEST/COGNITO_USER_POOLS authorizers and API-key/usage-plan enforcement confirmed wired and executing (unchanged this pass)."}
  authorizers_runtime: {status: ok, note: "TOKEN/REQUEST/COGNITO_USER_POOLS resolution + JWKS validation via injected JWKSProvider, TTL-bounded cache (bd gopherstack #1403 fixed prior unbounded growth)"}
  patch_semantics: {status: ok, note: "REWRITTEN this sweep — see Notes; was the single biggest gap in the service"}
gaps:
  - "PATCH 'remove' on bare top-level SCALAR fields is a no-op EXCEPT for the instances now fixed (RestApi./description, Authorizer./identitySource, DomainName./certificateArn + /regionalCertificateArn + /certificateName + /regionalCertificateName + /ownershipVerificationCertificateArn, and UsagePlan./productCode — all via *string Update*Input fields, verified against patch-operations.html as remove-supported paths on their resources' Update tables). Every OTHER Update*Input still uses a zero-value-means-not-provided check, so explicit remove still can't be distinguished from absence there. Audited against patch-operations.html's full per-resource table: almost every other top-level scalar (UpdateApiKey's customerId/description/enabled/name, UpdateAccount's cloudwatchRoleArn, UpdateStage's description/cacheCluster*/tracingEnabled/clientCertificateId, UpdateUsagePlan's name/description, UpdateModel's description/schema, UpdateRequestValidator's fields, UpdateResource's fields, UpdateVpcLink's fields, etc.) is documented replace-only with remove NOT supported, so no fix is needed there. Map/list-valued fields (variables, binaryMediaTypes, apiStages, responseParameters/Templates, methodSettings, stageKeys) support remove correctly because their merge goes through a full non-nil replacement value. (bd: gopherstack-vvsy, gopherstack-npq5)"
  - "FIXED (bd: gopherstack-vvsy): the multi-op-per-request PATCH clobbering bug (resource-specific resolvers re-deriving their starting map/struct from CURRENT BACKEND STATE instead of checking out[field] for what an earlier op in the SAME request already staged) is now fixed in all six resolvers that had it (applyStageVariablePatch, applyStageCanaryPatch, applyStageAccessLogPatch, applyRestAPIPatchOp's binaryMediaTypes case, applyAccountPatchOp, applyGatewayResponsePatchOp), via the same stagedValue[T] helper the prior sweep introduced. Verified by Test_ApplyStructuredPatch_MultiOpSameRequest (stage variables/canary/accessLog) plus Test_ApplyStructuredPatch_{RestAPIBinaryMediaTypesMultiOp,AccountThrottleMultiOp,GatewayResponseMultiOp} — each drives a real two-op PATCH request through the handler and asserts both ops land; each test was confirmed to fail against the pre-fix code (git-stashing patch.go alone) before the fix landed."
  - "FIXED (bd: gopherstack-vvsy): applyResourcePatchOp now has a case for opUpdateDomainName (applyDomainNamePatchOp), handling the nested paths \"/endpointConfiguration/types\" (add/remove), \"/endpointConfiguration/ipAddressType\" (replace), and \"/mutualTlsAuthentication/{truststoreUri,truststoreVersion}\" (add/replace/remove) — DomainName.MutualTLSAuthentication and EndpointConfiguration.IPAddressType are new fields added this follow-up. Verified by Test_ApplyStructuredPatch_DomainNameNestedPaths (four ops across two nested fields in one request, plus a remove). FIXED (bd: gopherstack-npq5, 2026-08-09): the remaining top-level scalars patch-operations.html's UpdateDomainName table documents — /certificateName, /regionalCertificateName, /ownershipVerificationCertificateArn (remove-supported, now *string on UpdateDomainNameInput) and /managementPolicy, /policy, /routingMode, /endpointAccessMode (replace-only, plain string) — are now DomainName/UpdateDomainNameInput fields; all seven route through the existing applyTopLevelPatchOp fallback, no new resolver needed. Verified against a live patch-operations.html fetch and aws-sdk-go-v2/service/apigateway@v1.42.4's deserializers.go for wire field names, and proven via real aws-sdk-go-v2 client integration tests (TestIntegration_APIGatewayAudit_UpdateDomainNameDocumentedFields). One item the tracking ticket listed under DomainName was found to actually belong to UpdateRestApi instead: /endpointConfiguration/vpcEndpointIds appears only in patch-operations.html's UpdateRestApi table, not UpdateDomainName's — left unmodeled here (UpdateRestApi doesn't handle nested endpointConfiguration paths at all yet; a RestApi-scoped gap, out of this fix's scope)."
  - "The exact property-path strings for per-route stage method settings (stageMethodSettingProperty in patch.go, e.g. \"logging/dataTrace\", \"caching/dataEncrypted\", \"caching/unauthorizedCacheControlHeaderStrategy\") were fetched and verified this sweep directly against the live AWS documentation page https://docs.aws.amazon.com/apigateway/latest/api/patch-operations.html (UpdateStage table) — every string in the map matches exactly, including the two new caching/* entries added this sweep. Still not backed by an SDK-level typed enum (PatchOperation.Path is a free string in aws-sdk-go-v2), so this remains a doc-fetch verification rather than a compile-time guarantee; re-verify if AWS changes the doc."
  - "UsagePlan.apiStages per-route throttle PATCH path shape RE-VERIFIED this follow-up (bd: gopherstack-vvsy) directly against a fresh fetch of patch-operations.html's UpdateUsagePlan table: \"/apiStages/{apidId:stageName}/throttle/{resourcePath}/{httpMethod}\" (remove-only) and the same path + \"/rateLimit\" or \"/burstLimit\" (add/replace) — gopherstack's usagePlanThrottlePathMinSegs=5 segmentation and the \"restApiId:stage\" colon-separated composite key both match AWS's own path notation exactly (AWS's doc literally uses the \"{apidId:stageName}\" spelling as the path placeholder). No wire capture example exists for the exact /apiStages add/remove Value string format, but the colon convention is corroborated by the path notation itself. No code change was needed — the existing implementation already matches."
  - "2026-08-11 (gopherstack-oius) FULL 22-OP CLASSIFICATION of every op whose real aws-sdk-go-v2 request shape is `{...ids, patchOperations}` (verified against botocore apigateway 2015-07-09 service-2.json — all 22 request shapes carry ONLY id fields + patchOperations, no flat scalars): ALREADY CORRECT before this sweep (single-segment scalars all route through applyTopLevelPatchOp and match their Update*Input json tag, or already had a dedicated resolver) — UpdateAccount, UpdateApiKey, UpdateAuthorizer (see caveat below), UpdateClientCertificate, UpdateDeployment, UpdateDocumentationPart, UpdateDocumentationVersion, UpdateDomainName, UpdateGatewayResponse, UpdateModel, UpdateRequestValidator, UpdateRestApi, UpdateStage, UpdateUsage, UpdateUsagePlan, UpdateVpcLink. FIXED this sweep (real bugs — see the 5 op lines above for details) — UpdateResource, UpdateMethod, UpdateIntegration, UpdateIntegrationResponse, UpdateMethodResponse. NOT reached this sweep, deferred — UpdateBasePathMapping (see gaps below: wire-casing bug found, not fixed)."
  - "FIXED (bd: gopherstack-6q5h): UpdateAccount's \"/features\" add/remove (patch-operations.html: add supported, remove supported except for the UsagePlans feature). Features field added to UpdateAccountInput (nil-checked, not len()>0, so removing the last feature actually clears it) and a features case added to applyAccountPatchOp — merges with the account's existing Features (initialized to [\"UsagePlans\"] at store setup, store.go:451) rather than replacing wholesale. Removing \"UsagePlans\" specifically, and any op but add/remove, are REJECTED (BadRequestException) per the doc table's exception. Verified with TestSDK_UpdateAccount_FeaturesPatch (patch_ops_sdk_test.go) against a real aws-sdk-go-v2 client; confirmed failing pre-fix (add silently dropped, both rejections returned 200 nil-error instead of an error)."
  - "FIXED (bd: gopherstack-6q5h): UpdateAuthorizer's \"/providerARNs\" add/remove (patch-operations.html: add/remove supported, replace not). Added applyAuthorizerPatchOp, merging with the authorizer's existing ProviderARNs (a wholesale replace would otherwise drop every other ARN) the same way applyAPIKeyPatchOp's /stages case does. InMemoryBackend.UpdateAuthorizer's ProviderARNs merge switched from len()>0 to != nil so removing the last ARN actually clears it (same class of bug the gopherstack-oius sweep fixed 'everywhere these paths reach', but Authorizer wasn't in that sweep's scope). Before this fix the raw Value JSON string unmarshaled straight into a []string field via the generic fallback and FAILED THE WHOLE PATCH REQUEST with a 500 (same bug class as UpdateIntegration's /cacheKeyParameters) — confirmed against a git-worktree checkout of the pre-fix commit with TestSDK_UpdateAuthorizer_ProviderARNsPatch (patch_ops_sdk_test.go), a real aws-sdk-go-v2 client test."
  - "FIXED (bd: gopherstack-6q5h): UpdateBasePathMapping's PATCH paths were far more broken than a casing bug. patch-operations.html spells them lowercase (\"/basepath\", \"/restapiId\" — note even here the doc keeps a capital I in \"restapiId\"), but the AWS CLI reference's own worked example (docs.aws.amazon.com/cli/latest/reference/apigateway/update-base-path-mapping.html, mirrored in the AWS Doc SDK Examples code-library page) uses path='/basePath' and shows a real, changed \"basePath\" in its output — AWS's own sources disagree on the casing, so BOTH spellings are now accepted rather than picking one. But even the EXACT struct-tag-matching spelling \"/basePath\" did not work before this fix, for a reason unrelated to casing: UpdateBasePathMappingInput.BasePath is the REQUIRED identity used to find the mapping (populated from the URL path segment), and handler.go's pathParams-merge step (injectJSONFieldAPIGW) runs AFTER applyStructuredPatch and unconditionally overwrites whatever the patch resolver staged under \"basePath\" with the URL's OLD value — so a rename could never take effect regardless of spelling, and InMemoryBackend.UpdateBasePathMapping had no rename logic at all (BasePath was read-only, used solely as a lookup key). Fixed both: applyBasePathMappingPatchOp now stages a rename under a new NewBasePath field (private to this backend, no equivalent on the real AWS wire) that InMemoryBackend.UpdateBasePathMapping applies as an actual store.Table key rename (delete old key, mutate BasePath, re-Put — rejecting a collision with an existing mapping at the new path). \"/restapiId\" is now explicitly aliased to RestAPIID too, rather than relying on it incidentally working via json.Unmarshal's case-insensitive field match (it does, since RestAPIID isn't pathParams-clobbered, but that's an accident worth not depending on). \"/restApiId\" and \"/stage\" already worked via the generic fallback and needed no change. Verified with TestSDK_UpdateBasePathMapping_PatchOperations (patch_ops_sdk_test.go) against a real aws-sdk-go-v2 client, confirmed failing pre-fix for both \"/basepath\" (404 on the renamed lookup) and \"/basePath\" (silently kept the old value)."
  - "CASING SWEEP (bd: gopherstack-6q5h): compared all 22 patchOperations-taking ops' documented paths (a fresh patch-operations.html fetch) against their Update*Input json tags and, for UpdateStage's per-route method-settings properties and UpdateUsagePlan's per-route throttle path, the literal strings this dispatcher compares against (patch.go's stageMethodSettingProperty map / usagePlanThrottlePathMinSegs segmentation). UpdateBasePathMapping's /basepath and /restapiId (fixed this pass) were the ONLY casing mismatch found; every other operation's json tags match their documented path spelling exactly, letter for letter. Also found in passing, NOT fixed (a real casing question but out of this ticket's scope): UpdateAuthorizer's PATCH table separately documents \"/authType\" (types.Authorizer.AuthType, a real but different field from Authorizer's existing \"Type\"/authorizerType) which this backend does not model at all — an unmodeled-field gap, not a casing one; and UpdateRestApi's table documents \"/securityPolicy\", but neither RestAPI nor UpdateRestAPIInput has a SecurityPolicy field (only DomainName does) — also unmodeled, not casing."
  - "2026-09-04 (bd: gopherstack-fum), PARTIALLY FIXED 2026-09-06 (bd: gopherstack-is2a): the 'AWS' (non-proxy) integration type used to invoke Lambda unconditionally, regardless of what AWS service the URI actually named. Now: sqs path-style integrations (uri=\"arn:aws:apigateway:{region}:sqs:path/{accountId}/{queueName}\") dispatch to SendMessage, and sns action-style integrations (uri=\"arn:aws:apigateway:{region}:sns:action/Publish\") dispatch to Publish, each via a SetSQSSender/SetSNSPublisher hook wired in cli.go -- SendMessage and Publish were chosen as the two targets with both a real backing service in this repo and an unambiguous single action. This is a simplified passthrough, NOT VTL mapping-template evaluation (no VTL library exists in this module): the rendered request payload becomes the SQS MessageBody or SNS Message verbatim (no Action=...&... AWS query-protocol encoding), the SNS TopicArn comes from the integration's RequestParameters mapping or a TopicArn query parameter (not from the URI, which action-style integrations never encode it in), and a successful response is a bare \"{}\" -- not a real SQS/SNS response shape. STILL NOT FIXED, unchanged from before: DynamoDB, Kinesis, Step Functions, S3, and any other 'AWS' integration target (e.g. uri=\"arn:aws:apigateway:us-east-1:dynamodb:action/PutItem\") is still accepted at PutIntegration with no validation and still unconditionally invokes Lambda at request time -- either a 503 'Lambda integration not configured' (no invoker wired) or a Lambda 'function not found' failure (invoker wired, ExtractLambdaFunctionName returns the target's ARN unchanged, not a real Lambda function). sqs/sns integrations also keep this exact unchanged behavior when SetSQSSender/SetSNSPublisher is not wired (TestHandleAWSIntegration_{SQSTarget,SNSTarget}_Unwired) -- an unwired hook is a silent no-op, never a rejection, matching the ~150 services in this repo whose test backends build with no cross-service hooks. Fixing the remaining targets for real needs either new per-service invoker interfaces for each (DynamoDB, Kinesis, Step Functions, S3, ...) or a real VTL evaluator plus AWS query-protocol request/response encoding for every target -- larger, out of this pass's narrow scope."
  - "2026-09-04 (bd: gopherstack-fum), documented, NOT FIXED (large architectural change): CreateDeployment does not freeze a routable snapshot of resources/methods/integrations. Deployment.APISummary is a lightweight display-only summary (matching the real SDK's ApiSummary field), not something the data plane routes against -- proxy_routing.go's routingTrie always calls Backend.ResourcesForRouting(apiID), which reads the RestApi's LIVE current resource tree, with no notion of 'as of which deployment'. Confirmed by reproduction: deploy a stage, then delete (or edit) a resource with NO new deployment -- the already-deployed stage's behavior changes immediately, where real AWS would keep serving the old, deployed configuration until a new deployment is created and the stage is repointed to it. Properly fixing this needs a real per-deployment resource/method/integration snapshot plus stage-to-deployment pinning enforced in the data plane -- a substantial redesign of the deployment model, out of scope for a targeted bug-fix pass. RE-CONFIRMED, still NOT FIXED, 2026-09-08 (bd: gopherstack-9ard, filed as a duplicate of this same gap): re-verified against the current code, unchanged. That pass also fixed one adjacent, independently-fixable gap this structural one doesn't require: UpdateStage now validates deploymentId against real Deployment state (previously unvalidated, unlike CreateStage's existing guard) -- see the UpdateStage ops line and the dated note above for detail. A second candidate fix, rejecting CreateDeployment for a method with no integration, was investigated and deliberately NOT enforced: neither the pinned Go SDK module nor botocore's wire model documents any such precondition, and the only supporting source was third-party tooling -- below this file's bar for a change that rejects previously-accepted requests. See the CreateDeployment ops line and the dated note above for detail."
deferred:
  - "ApiKey.StageKeys's PATCH /labels add/remove path (listed in patch-operations.html's UpdateApiKey table) still has no corresponding field anywhere in aws-sdk-go-v2/service/apigateway/types.ApiKey (re-verified this sweep) — likely a stale doc artifact from a pre-Tags API generation. Nothing to implement against; distinct from /stages, which this sweep DID implement (see Notes)."
  - "2026-08-11 (gopherstack-oius): Method.AuthorizationScopes is not modeled anywhere in this backend (not on Method, not on PutMethodInput/CreateAuthorizerInput's COGNITO_USER_POOLS flow) even though patch-operations.html documents UpdateMethod's \"/authorizationScopes\" as add/remove-supported and it's a real, commonly-used field (COGNITO_USER_POOLS authorizer scope matching). UpdateMethod now explicitly REJECTS this path (BadRequestException) rather than silently accepting a patch that changes nothing — see applyMethodPatchOp. Properly modeling it needs PutMethod/PutMethodInput plumbing too, a larger change than this PATCH-focused sweep; tracked here as the next real step."
leaks: {status: fixed, note: "no new goroutines/tickers/persistent state introduced this sweep — all new code (StageKeyInput resolution, patch.go's new resolvers/stagedValue helper) is request-scoped and synchronous under the existing coarse b.mu; UpdateUsagePlan's missing defensive copy (return p instead of a copy, found while extending it for per-route throttle) was also fixed, closing a latent aliasing hole where a caller mutating the returned *UsagePlan would have corrupted backend state directly. 2026-09-04 (bd: gopherstack-fum): FIXED -- h.trieCache (the compiled per-API routing-trie cache, a sync.Map keyed by RestApi ID) was never evicted on DeleteRestApi; since IDs are fresh-random per CreateRestApi a deleted API's cached trie could never be overwritten by a later Store and stayed in process memory for the server's remaining lifetime. Fixed in handler_rest_apis.go's deleteRestAPIAction (h.trieCache.Delete after a successful backend delete); TestDeleteRestAPI_EvictsTrieCache confirmed failing pre-fix, passing post-fix."}
---

## Notes

This sweep's finding was concentrated in ONE architectural gap rather than spread
across many small op bugs: **every single Update*/PATCH operation in this
service shared one flatten function (`handler.go`'s old `normalizePatchBody` /
`flattenPatchOps`) that could only express "replace one top-level field."**
Two concrete, previously-silent bugs fell out of that:

1. **PatchOperation.Value is always a JSON *string* on the wire**, even for
   bool/int targets — confirmed by reading aws-sdk-go-v2's actual serializer
   (`awsRestjson1_serializeDocumentPatchOperation` calls `ok.String(*v.Value)`
   unconditionally). So a real client PATCHing e.g.
   `{"op":"replace","path":"/tracingEnabled","value":"true"}` was handing this
   service the raw bytes `"true"` (a JSON string) to unmarshal directly into
   `UpdateStageInput.TracingEnabled *bool` — a JSON type-mismatch that made the
   whole PATCH request error out. This affected every non-string top-level
   PATCH field across the service: `tracingEnabled`, `cacheClusterEnabled`,
   `minimumCompressionSize`, API key `enabled`, `validateRequestBody`,
   `validateRequestParameters`, `apiKeyRequired`, `authorizerResultTtlInSeconds`.
   Fixed via `patch.go`'s `patchFieldKind` table + `coerceTopLevelPatchValue`.

2. **Multi-segment PATCH paths were silently dropped.** The old flatten took
   the *entire* remaining path after the leading `/` as one bogus flat field
   name (e.g. `/variables/apiKey` became the map key literally
   `"variables/apiKey"`), which matches no `Update*Input` json tag, so the
   edit vanished with no error. This meant the single most common real-world
   API Gateway PATCH usage — **setting one stage variable** — never worked at
   all, alongside per-route method settings, binary-media-type membership,
   usage-plan API-stage membership, gateway-response parameter/template
   entries, and canary-deployment promotion (which AWS models as a `"copy"`
   op — a verb the old flatten didn't implement at all, since it only handled
   `"add"`/`"replace"`, and silently skipped `"remove"` too).

Rewritten in `patch.go` (new file) with per-resource resolvers that read
current backend state and merge the touched entry, since the Update* backend
methods replace a map/struct field wholesale when it's provided — see the
file's package doc for the full design rationale and the exact PATCH path
shapes each resolver targets (stage variables, canary promotion, per-route
method settings, REST API binary media types, account throttle, usage-plan
API stages, gateway-response parameters/templates).

**Independent bug found and fixed while auditing UpdateGatewayResponse**: its
action handler reused `PutGatewayResponse` (a correct full-replace for the
real PUT operation) for the PATCH operation too. Since PUT semantics
unconditionally overwrite `StatusCode`/`ResponseParameters`/`ResponseTemplates`
with whatever the (now-partial) flattened PATCH body happened to contain, ANY
partial PATCH — even a plain single-field `/statusCode` replace — silently
wiped whichever of the other two fields wasn't part of that particular PATCH
call. Added a dedicated `InMemoryBackend.UpdateGatewayResponse` that merges
field-by-field (falling back to AWS's implicit per-responseType default when
no custom response has been PUT yet), and wired the Update action to it.

**Independent bug found and fixed in `InMemoryBackend.UpdateUsagePlan`**: it
only applied `input.APIStages` when `len(input.APIStages) > 0`, so a PATCH
that removes the last remaining API stage (producing a correctly-empty, but
non-nil, slice) was silently ignored. Changed the guard to `!= nil`.

**Stage cache-cluster fields were entirely missing.** AWS's `Stage` (and
`CreateStage`/`UpdateStage` inputs) carry `CacheClusterEnabled`,
`CacheClusterSize`, and a derived `CacheClusterStatus`
(`AVAILABLE`/`NOT_AVAILABLE`) — none existed on gopherstack's `Stage` struct.
Added all three, wired through `CreateStage`/`UpdateStage`.

**`UpdateAccountInput` was missing `CloudwatchRoleARN` entirely** — the single
most common real-world reason to call `UpdateAccount` (wiring a CloudWatch
Logs role for API Gateway execution logging) had no way to reach the backend
at all. Added the field and its backend wiring.

### PATCH-op semantics traps (for the next auditor)

- **`PatchOperation.Value` is always a JSON string on the wire**, regardless
  of the target field's real type. Never copy it into a flattened body
  verbatim unless the target field actually is a Go `string`.
- **A PATCH `path` is a JSON Pointer**: `~1` decodes to `/`, `~0` decodes to
  `~`, and the `~1` substitution must happen *before* `~0` (so `~01` decodes
  to `~1`, not to `/`). Get the order backwards and escaped values silently
  corrupt.
- **Per-route stage method-settings paths have NO `"methodSettings"` path
  segment.** They're addressed directly as
  `/{resourcePath}/{httpMethod}/{category}/{property}`, where `resourcePath`
  is itself JSON-Pointer-escaped (its own internal `/` becomes `~1`) or the
  literal wildcard `*`. A path segment that starts with `~1` or is exactly
  `*` is the tell that you're looking at a method-settings patch, not a
  plain field name — every genuine top-level Stage field name is a bare
  identifier and never starts with `~` or equals `*`.
- **`UsagePlan.apiStages` PATCH uses a single-segment path** (`/apiStages`)
  with the API stage identified entirely by `value` — the string
  `"{restApiId}:{stage}"` — not by a nested path segment. Don't assume every
  list-membership PATCH nests the identifying key into the path; this one
  doesn't.
- **`"copy"` is a real, AWS-documented op** (not just `add`/`replace`/`remove`)
  used for canary-deployment promotion:
  `{"op":"copy","from":"/canarySettings/deploymentId","path":"/deploymentId"}`.
  Its `from` value must be resolved against the resource's *current* stored
  state, not against the request body (there's no `from` value in the patch
  document itself to read it from).
- **PUT vs PATCH on the same resource can have different replace semantics.**
  `PutGatewayResponse` (real PUT) is correctly a full replace. Reusing it
  verbatim for the PATCH operation on the same resource is a bug — PATCH must
  merge only the touched fields with current state. Watch for this pattern
  (`opUpdateX` action calling the same backend function as `opPutX`)
  elsewhere in this codebase; it wasn't audited beyond GatewayResponse this
  sweep.
- **`len(slice) > 0` is the wrong presence check for "was this field provided
  in the patch."** It's indistinguishable from "provided but now empty" and
  silently drops the empty-result case (found in `UpdateUsagePlan.APIStages`).
  Use `!= nil` for slice/map fields that a merge might legitimately want to
  empty out.

Protocol confirmed: REST-JSON (`restjson1`) — HTTP verb + path routing per
resource, JSON request/response bodies, `application/x-amz-json-1.1` on
errors, epoch-seconds timestamps (`unixEpochTime`/`pkgs/awstime`-equivalent
inline type) — not a single json-1.0/1.1 RPC target the way most other
services in this codebase are.

## 2026-07-11 re-audit sweep

No local drift since ce30166a (`git diff` over `services/apigateway/` between
the two commits is empty) and no SDK version bump (`aws-sdk-go-v2/service/apigateway`
still pinned at v1.38.6), so this sweep audited only the ledger's documented
`gaps` plus a general due-diligence pass rather than re-verifying every `ok`
row from scratch.

**Real bug found and fixed: `ApiKey.CustomerId` was entirely absent.**
aws-sdk-go-v2's `types.ApiKey`/`types.CreateApiKeyInput`/`UpdateApiKeyInput`
(via `PatchOperations`) all carry `CustomerId` (an AWS Marketplace SaaS
integration identifier) — confirmed by reading the vendored SDK's
`types/types.go` and `api_op_CreateApiKey.go`. gopherstack's `APIKey`,
`CreateAPIKeyInput`, and `UpdateAPIKeyInput` structs had no such field at all,
so a real client's `customerId` was silently dropped on create, never
returned by Get/GetApiKeys, and unpatchable (AWS's `patch-operations.html`
lists `/customerId` as a `replace`-supported UpdateApiKey path). Added the
field to all three structs and wired it through
`InMemoryBackend.CreateAPIKey`/`UpdateAPIKey`; no `patch.go` change was needed
since `/customerId` is a single-segment scalar path that the existing generic
top-level PATCH fallback already handles correctly for string fields. Covered
by `TestBatch2Ops_ApiKey_CustomerID` (create/get/patch round-trip).
`apiKeys` is a "clean" (non-DTO) persisted table, so the new field persists
automatically.

**Almost-bug, verified false via WebFetch against AWS's live docs — logged so
the next auditor doesn't repeat the investigation.** `UpdateUsage`'s PATCH
routing looked suspicious: `applyResourcePatchOp` (patch.go) has no case for
`opUpdateUsage`, so any *multi-segment* PATCH path falls through to
`applyTopLevelPatchOp`, which explicitly no-ops any path containing `/` after
the leading slash. Every other resource with a real-world multi-segment PATCH
path (stage variables, per-route method settings, usage-plan API stages, ...)
got an explicit resolver in a prior sweep, and `UpdateUsage`'s doc comment
*said* its patch paths were per-date (`"date -> new remaining quota"`),
which would have been multi-segment (`/{date}/{usageIndex}`) and thus
silently broken. Fetched AWS's `patch-operations.html` UpdateUsage table
*and* the `aws apigateway update-usage` CLI reference example directly:
both agree the one and only supported path is the single-segment scalar
`/remaining` — there is no per-date path at all. Under that real shape the
existing code was already correct (the backend's merge loop only reads the
flattened map's *values*, never its keys, so the misleading "date" key name
never mattered). Fixed the stale/misleading doc comment on
`InMemoryBackend.UpdateUsage` and the test in `handler_destub_test.go` that
was asserting against the wrong (`/2024-01-01`) path shape, and strengthened
that test to assert the actual overridden `remaining` value instead of just
key presence.

**Checked but deferred**: `ApiKey.StageKeys` (`/stages` add/remove) is
present in the SDK but explicitly marked deprecated in
`CreateApiKeyInput.StageKeys`'s doc comment ("should not be used"); left
unimplemented (see `deferred`). The `/labels` PATCH path AWS's
patch-operations.html lists for UpdateApiKey has no corresponding field
anywhere in the current SDK's `types.ApiKey` — likely stale documentation
with nothing to implement against.

No other rows changed. Gates: `go build`/`go vet`/`go test -race`/`go fix
-diff`/`golangci-lint run`, all scoped to `./services/apigateway/...`, pass
clean both before and after this sweep's edits.

## 2026-07-23 sweep

Closed all 5 documented `gaps` and all 3 `deferred` items from the 2026-07-11
sweep. Field-diffed every new field/path against the vendored
`aws-sdk-go-v2/service/apigateway@v1.38.6` types (`types.go`,
`api_op_*.go`, `serializers.go`) and, for PATCH path shapes, against a live
fetch of https://docs.aws.amazon.com/apigateway/latest/api/patch-operations.html
(the previous sweep's `gaps` entry #5 flagged the method-settings property
strings as unverified against a typed enum — no such enum exists in the SDK,
so this sweep instead fetched the actual doc table and confirmed every
existing string plus the two added this sweep match exactly).

**Deferred item 1 (RestApi cosmetic fields) — all four fields are real,
confirmed in `types.go`**: `ApiStatus` (enum `UPDATING`/`AVAILABLE`/`PENDING`/
`FAILED`), `ApiStatusMessage`, `DisableExecuteApiEndpoint`,
`EndpointAccessMode` (enum `BASIC`/`STRICT`). All four are present in
`CreateRestApiInput` too (not create-then-immutable), confirmed by reading
`api_op_CreateRestApi.go`. `ApiStatus` is AWS-managed/read-only (no PATCH path
in patch-operations.html); gopherstack sets it to `AVAILABLE` unconditionally
on create since RestApi creation here is always synchronous with no
UPDATING/PENDING/FAILED transition to model. `DisableExecuteApiEndpoint` and
`EndpointAccessMode` are both real PATCH paths (`patch-operations.html`'s
UpdateRestApi table: both replace-only, no add/remove) — wired through
`patchFieldKind`'s bool-coercion table for the former (a wire string like
`"true"` must coerce to a JSON bool before hitting the `*bool` field).

**Deferred item 2 (Stage.DocumentationVersion) — real field, real PATCH
path**: added to `Stage`, `CreateStageInput`, `UpdateStageInput`, and (since
Stage is a DTO'd table, unlike RestApi/ApiKey/UsagePlan's "clean" tables) the
`stageSnapshot` DTO in `persistence.go` — a field added to `Stage` alone
without the DTO update would silently NOT persist across Snapshot/Restore,
the exact bug class `pkgs-catalog.md`'s "clean/dirty table split" note warns
about.

**Deferred item 3 (ApiKey.StageKeys) — implemented, contrary to the prior
sweep's decision to leave it out.** Re-read the SDK doc comment: it says
"DEPRECATED FOR USAGE PLANS ... should not be used" as *guidance*, not a
removal — the field is still fully present and functional in
`CreateApiKeyInput.StageKeys` (`[]types.StageKey`, object form: `restApiId`/
`stageName`), `CreateApiKeyOutput.StageKeys`/`GetApiKeyOutput.StageKeys`/
`UpdateApiKeyOutput.StageKeys` (`[]string`, confirmed serialized as
`{restApiId}/{stageName}` by reading `awsRestjson1_serializeDocumentStageKey`
in `serializers.go`), and `UpdateApiKey`'s `/stages` PATCH path (add/remove,
`patch-operations.html`'s UpdateApiKey table). AWS deprecating a field in
favor of a newer mechanism (usage plans) doesn't make the field non-functional
or out of scope for parity — a real client can still call it and expects a
real response, so implementing it is the correct call under this campaign's
no-stub rule. `CreateApiKey` validates each referenced REST API + stage
exists (`NotFoundException` otherwise, mirroring `CreateUsagePlanKey`'s
existing FK-validation pattern) and formats survivors as
`{restApiId}/{stageName}` via the new `formatAPIKeyStageKey` helper. Also
re-confirmed the prior sweep's finding that `/labels` (a second ApiKey PATCH
path `patch-operations.html` lists) has no corresponding field anywhere in
`types.ApiKey` — left in `deferred` since there's genuinely nothing to wire
it to.

**Gap "PATCH remove on top-level scalars" — narrowed, not closed.** The
architectural fix (pointer-ify every Update*Input field) is out of this
sweep's budget across all ~15 resources, but two concrete instances are now
real: `UpdateRestAPIInput.Description` and `UpdateAuthorizerInput.IdentitySource`
both became `*string`, and their handler-level wire structs
(`updateRestAPIHandlerInput` embeds `UpdateRestAPIInput` directly;
`updateAuthorizerInput` in `handler_authorizers.go` is a separate
hand-written struct that had to be migrated too, since it doesn't embed the
backend input type) plus `patch.go`'s new `removableTopLevelScalar` table
(gating exactly which action+field pairs get an explicit `""` write on
`remove`, vs. every other field which still silently no-ops) make `remove`
on `/description` (RestApi) and `/identitySource` (Authorizer) actually work
end to end. Verified both are genuinely the only top-level-scalar
`op:remove`-supported paths on their respective resources per
`patch-operations.html` (every other remove-supported path on every other
resource's table is either already map/list-handled, or — for
UpdateDomainName — entirely unhandled for an unrelated reason; see the new
`gaps` entry on that).

**Two bugs found (not assigned, found while extending adjacent code) and
fixed:**

1. `InMemoryBackend.UpdateUsagePlan` returned `p, nil` — a pointer straight
   into the backend's own stored `*UsagePlan`, not a defensive copy, unlike
   every other `Update*` method in this service (`cp := *x; return &cp`). A
   caller mutating the returned value would have corrupted backend state
   without going through the lock. Found while extending this method's PATCH
   coverage for per-route throttle; fixed to match the established pattern.
2. Multiple PATCH ops in one request targeting the *same* merged field
   (discovered via `Test_ApplyStructuredPatch_UsagePlanPerRouteThrottle`,
   which legitimately sets both `rateLimit` and `burstLimit` for one route in
   a single request — a very plausible real-client pattern) clobbered each
   other: `applyStageMethodSettingPatch` (and, before this sweep's fix, the
   two new UsagePlan/ApiKey resolvers) each independently re-derived their
   starting map from **current backend state**, ignorant of what an earlier
   op in the same request had already staged into `out`. The last op's
   `setJSONValue(out, field, ...)` call wins, discarding earlier ones. Added
   `stagedValue[T]` (a small generic helper) and wired it into the three
   resolvers this sweep's new code touches
   (`applyStageMethodSettingPatch`, `applyUsagePlanAPIStageMembershipPatch`
   + `applyUsagePlanThrottlePatch` via the new `currentUsagePlanAPIStages`
   helper, `applyAPIKeyPatchOp`). The same bug pattern exists, unfixed, in
   six pre-existing resolvers this sweep didn't need to touch
   (`applyStageVariablePatch`, `applyStageCanaryPatch`,
   `applyStageAccessLogPatch`, `applyRestAPIPatchOp`'s binaryMediaTypes case,
   `applyAccountPatchOp`, `applyGatewayResponsePatchOp`) — every existing
   test for those only ever sends one op per request per field, so the bug
   was never exercised. Logged as a `gaps` entry rather than silently fixed
   everywhere, since a blanket fix across 6 more call sites was judged
   outside this sweep's assigned scope (5 gaps + 3 deferred, all now
   addressed) and deserves its own focused verification pass.

**New gap found (not fixed, out of scope): `UpdateDomainName`'s PATCH
semantics.** `applyResourcePatchOp`'s switch (`patch.go`) has no case for
`opUpdateDomainName`, so every nested/multi-segment DomainName PATCH path
(`/mutualTlsAuthentication/truststoreUri`, `/certificateName`,
`/endpointConfiguration/types/{type}`, etc. — all real, per
`patch-operations.html`'s UpdateDomainName table, which has more distinct
paths than any other resource in this service) falls through to
`applyTopLevelPatchOp`, which no-ops anything containing `/` after the
leading slash. This predates this sweep and was not one of the 5 assigned
gaps/3 deferred items, so left unfixed here — flagging for a dedicated
`domain_names` PATCH-semantics sweep, since it looks like a comparably-sized
gap to the one this whole PATCH rewrite effort (see the 2026-07-\* sweeps
above) already closed for every other resource.

Gates: `go build`, `go vet`, `go test -race -count=1`, `gofmt -l` (clean),
`golangci-lint run` (0 issues), and a grep for banned
`cyclop`/`gocyclo`/`gocognit`/`funlen` nolints (empty) — all scoped to
`./services/apigateway/...` — pass clean after this sweep's edits. One
cyclop violation surfaced mid-sweep (`applyStageCanaryPatch` hit 17 after
adding the `stageVariableOverrides` case, max 15) and was resolved by
extracting the per-property switch into `applyStageCanaryProp`, not a
nolint.

## 2026-08-11 sweep (bd: gopherstack-oius)

The tracking ticket named three ops (`UpdateResource`, `UpdateMethod`,
`UpdateDocumentationPart`) as taking only `patchOperations` where gopherstack
allegedly expected flat scalar fields. Re-checked against the model directly:
**all 22** apigateway ops whose real `Update*Request` shape is
`{...ids, patchOperations}` (confirmed via botocore's `apigateway/2015-07-09/
service-2.json` — every one of the 22 request shapes carries only id fields
plus `patchOperations`, nothing else). Full classification is in the `gaps`
entry above. The headline correction: **`UpdateDocumentationPart` was
already fine** — its one documented path, `/properties` (replace-only,
patch-operations.html), is a single-segment scalar whose value (a JSON
string containing the documentation-part's own JSON-encoded properties text)
round-trips correctly through the existing generic top-level PATCH fallback,
since `UpdateDocumentationPartInput.Properties` is a plain string field. The
ticket's premise was based on an earlier sampling of 3 ops that didn't verify
each one individually before generalizing; this sweep did.

The 5 ops actually fixed (`UpdateResource`, `UpdateMethod`,
`UpdateIntegration`, `UpdateIntegrationResponse`, `UpdateMethodResponse` —
the resource/method/integration/stage tier the ticket asked to prioritize,
Stage having already been fixed by an earlier sweep) are detailed on their
`ops:` lines above. Two categories of confirmed bug, both proven with a real
aws-sdk-go-v2 client run against a `git worktree` checkout of the
pre-fix commit (`git stash` is banned in this repo; a worktree gave a clean
before/after comparison without touching the working tree):

1. **Hard failure, not silent no-op** — `UpdateIntegration`'s `/cacheKeyParameters`
   (a `[]string` field) and `/timeoutInMillis` (an `int` field) received
   `PatchOperation.Value`'s wire-mandated JSON string verbatim and handed it
   to `json.Unmarshal` against a `[]string`/`int` target, which errors the
   whole PATCH request as a 500 (`json: cannot unmarshal string into Go
   struct field UpdateIntegrationInput.cacheKeyParameters of type []string`)
   — this is the literal "no real client can call this operation
   successfully" bug class the ticket named, just on different fields than
   it guessed. Fixed via a dedicated single-segment list-membership resolver
   for `/cacheKeyParameters` (mirroring `/stages`/`/binaryMediaTypes`
   elsewhere in this file) and adding `timeoutInMillis` to `patchFieldKind`
   for string→int coercion.

2. **Silent no-op** — every per-key map path this sweep added a resolver for
   (`/requestParameters/{name}`, `/requestModels/{content-type}` on Method;
   `/requestParameters/{name}`, `/requestTemplates/{content-type}` on
   Integration; `/responseParameters/{name}`, `/responseTemplates/{content-type}`
   on IntegrationResponse; `/responseModels/{content-type}`,
   `/responseParameters/{name}` on MethodResponse) previously fell through
   `applyTopLevelPatchOp`'s "path contains `/` → drop" guard, same as every
   other resource's multi-segment paths before the 2026-07/08 sweeps fixed
   them.

**Structural change: `applyResourcePatchOp`/`applyStructuredPatch` can now
reject a PATCH op with an AWS error, not just apply-or-drop it.** Every
resolver before this sweep only ever had two outcomes: apply the edit, or
silently do nothing (`return false`/`true` with no error). That's the root
cause of gopherstack-oius's "silently accepting a patch that changes
nothing" framing — there was no mechanism to say "the API rejects this."
`applyStructuredPatch` and every per-action resolver now return `(bool,
error)`; `handler.go`'s call site turns a returned error into a real
`BadRequestException` via the existing `handleError` path. Used to reject:

- **AWS-documented "not supported" combinations that previously silently
  succeeded**: `UpdateIntegration`'s `/type` (patch-operations.html marks it
  "Not supported" for every op, but `Integration.Type` has a matching struct
  field, so a real client's PATCH previously changed a live integration's
  type with no error — confirmed via `TestSDK_UpdateIntegration_
  PatchOperations/type_is_rejected_as_not-supported_for_any_op`, which fails
  with no error pre-fix); `/parentId` on any op but `replace` (add/remove/copy
  aren't documented for it).
- **AWS-documented paths this backend does not model as real state**, where
  fabricating support would be worse than rejecting: `UpdateMethod`'s
  `/authorizationScopes` (Method has no `AuthorizationScopes` field anywhere
  in this backend, including `PutMethod` — modeling it properly needs Put-side
  plumbing too, out of this PATCH-focused sweep's scope, see `deferred`);
  `UpdateIntegration`'s `/integrationTarget`, `/responseTransferMode`,
  `/tlsConfig/insecureSkipVerification` (real `aws-sdk-go-v2/service/
  apigateway/types.Integration` fields — `types.go:591,627,635,1262` in the
  pinned v1.42.4 — entirely absent from gopherstack's `Integration` struct).

The dispatch table (`resourcePatchResolvers`) was also converted from a
12-case `switch` to a `map[string]resourcePatchResolver` lookup specifically
to keep `applyResourcePatchOp`'s cyclomatic complexity flat as more actions
were added to it (this repo bans `cyclop`/`gocognit`/`funlen` nolints, so a
growing switch was the wrong shape to keep extending).

**`InMemoryBackend.UpdateResource` now supports moving a resource to a new
parent** (`/parentId`), the one substantive backend behavior this sweep
added beyond patch-plumbing: validates the new parent exists in the same
`RestApi`, rejects a move into the resource's own subtree (cycle
prevention), and recomputes `Path` for the moved resource and its entire
descendant subtree — `Resource.Path` is stored precomputed per-resource
rather than derived lazily from the ancestor chain, so every descendant
under a moved resource needs its `Path` refreshed too, not just the moved
resource itself.

**`len(x) > 0` → `x != nil` fixed on every backend field this sweep wired a
per-key PATCH resolver for** (`Method.RequestModels`/`RequestParameters`,
`Integration.RequestTemplates`/`RequestParameters`/`CacheKeyParameters`,
`IntegrationResponse.ResponseTemplates`/`ResponseParameters`,
`MethodResponse.ResponseModels`/`ResponseParameters`) — the same
"PATCH-removing-the-last-entry-of-a-map-is-silently-ignored" bug class
`UpdateUsagePlan.APIStages` had before the 2026-07-11 sweep, latent in these
six backend methods until a per-key resolver could produce a legitimately
empty (but non-nil) merged map/slice as this sweep's new resolvers do.

Four additional bugs/gaps were found while auditing the other 17 ops but are
deliberately NOT fixed here (see `gaps`/`deferred` above for each): Account's
`/features`, Authorizer's `/providerARNs` (same hard-failure bug class as
Integration's `/cacheKeyParameters`, not yet fixed), BasePathMapping's
lowercase-vs-camelCase wire-casing mismatch, and Method's
`/authorizationScopes` (rejected here, not yet modeled).

Verification: `patch_ops_sdk_test.go` (new) drives all 5 fixed ops through a
real `aws-sdk-go-v2/service/apigateway` client via `newTestAPIGatewayClient`
(in-process `httptest.Server` wrapping this package's real `Handler`, no
Docker needed for this kind of proof). Confirmed every new assertion fails
against the pre-fix code by running the identical test file, unmodified,
against a `git worktree` checkout of the commit before these changes.

Gates: `go build ./...`, `go vet ./services/apigateway/...`, `go test -race
./services/apigateway/... .`, `gofmt -l` (clean), `golangci-lint run
./services/apigateway/...` (0 issues) all pass. `go vet ./.` (repo root) also
run since `handler.go`'s `applyStructuredPatch` call site's signature
changed, though nothing outside `services/apigateway` calls it.

## 2026-08-13 pass (gopherstack-l5ir): route reachability, apikeys/domainnames/usageplans/vpclinks/clientcerts remainder

gopherstack-4nek verified the `/restapis` subtree (~90 of 124 ops) via a
same-path collision check but explicitly left the `apikeys`, `domainnames`
(+ `domainnameaccessassociations` + `basepathmappings`), `usageplans`
(+ keys + usage), `vpclinks`, and `clientcertificates` routing in
`handler.go`/`handler_router.go` unchecked -- roughly 30-41 ops depending on
how the sub-families are counted. This pass extracted the real method+path
for all 41 ops in that remainder from `apigateway@v1.42.4` serializers.go
(`request.Method` + `httpbinding.SplitURI(...)` in each op's
`awsRestjson1_serializeOp<Op>.HandleSerialize`) and diffed them against
`parseAPIGWRESTPath`'s dispatch tree (`handler_router.go` plus the five
per-family `parseAPIGW*Path` functions it delegates to).

**Result: zero mismatches.** Every op, including `ImportApiKeys`/`CreateApiKey`
sharing the bare `/apikeys` path and disambiguated only by a real `?mode=import`
query flag -- the exact "bare flag" pattern that broke cloudfront's
`CreateDistributionWithTags` -- was already correctly wired; the query-param
check (`query.Get("mode") == modeImport`) was already present and correct.
`RejectDomainNameAccessAssociation`'s own top-level path
(`/rejectdomainnameaccessassociations`, sibling to `/domainnameaccessassociations`,
not nested under it) was also already correctly routed.

Architecturally this remainder (and the already-verified `/restapis` subtree)
share a design that structurally resists the routing-bug class this campaign
found elsewhere: `parseAPIGWRESTPath` is the single function used for BOTH
real request dispatch AND `ExtractOperation`'s op-name resolution (`handler.go`'s
`ExtractOperation` calls it directly), so there is no second, independently-
maintained implementation of "what op does this path mean" to drift out of
sync with the real dispatch tree -- the exact failure mode that caused most
of opensearch's and lambda's bugs in this same campaign (a separate
`ExtractOperation`/`IAMAction` implementation silently diverging from the
real HTTP dispatch).

Added as a permanent regression test, `TestExtractOperation_SDKRouteTable`
(`handler_paths_sdk_diff_test.go`, one subtest per op, 41/41 pass) --
converts the audit into a standing guarantee. No routing code changes were
needed; no existing test encoded a wrong path. Gates (`go build`, `go vet`,
`go test -race`, `go fix -diff`, `golangci-lint run`) all clean.

Not touched by this pass: the already-verified `/restapis` subtree itself
has no equivalent permanent `TestExtractOperation_SDKRouteTable`-style test
committed (gopherstack-4nek's verification was a one-off collision check,
not a committed test) -- a good candidate for a future pass, now that the
pattern exists in this same file's sibling services.

## gopherstack-wlo1 (2026-08-22): handleRESTAPI's dispatch-miss branch was untyped

`handleRESTAPI`'s own `if !ok { return c.String(http.StatusNotFound, "not
found") }` guard (handler.go, from `parseAPIGWRESTPath`) wrote a bare
text/plain 404 -- a different site from `handleJSONProtocol`'s dispatch
errors `c6554e9f8` already typed, and from `handleRESTAPI`'s own
`ReadBody`-failure branch `gopherstack-o7gx` already typed. apigateway is
restjson1 (`apigateway@v1.42.4` `awsRestjson1_` prefix; error decode via
`restjson.GetErrorInfo`), so a real client saw
`smithy.GenericAPIError{Code:"UnknownError"}`.

Reachability: `RouteMatcher` (handler.go) accepts any request under the
coarse `isAPIGWTopLevelRESTPath` prefixes (`/restapis`, `/apikeys`, etc.),
while `parseAPIGWRESTPath` classifies by exact method+path structure -- a
prefix-matched sub-path it doesn't recognise falls through to `!ok`, the
same prefix-vs-classifier gap securityhub's analogous fix (a98561767)
established as provable.

Fixed: routes through the existing `handleError(ctx, c, action,
errUnknownOperation)` -- `errUnknownOperation` already maps to
`"UnknownOperationException"` at 400 in `handleError`'s own switch (the
same code/helper `handleJSONProtocol`'s analogous branches use), so no new
exception vocabulary was introduced.

Proof: `TestHandleRESTAPI_UnrecognisedPathSurfacesUnknownOperationException`
(`handler_restapi_dispatch_malformed_test.go`) drives a real apigateway
client's `CreateRestApi` through a Finalize-stage middleware that rewrites
the signed request's path from `/restapis` to
`/restapis/does-not-exist/nowhere` post-signing -- still inside
`RouteMatcher`'s `/restapis` prefix but matching none of
`parseAPIGWRESTPath`'s cases. Hand-reverted `handler.go` to `git show
HEAD`, confirmed the test fails with `*json.SyntaxError: "invalid character
'o' in literal null (expecting 'u')"`, restored the fix,
`md5sum`-confirmed byte-identical.

Two pre-existing tests (`TestHandleRESTAPI_Branches/unknown_rest_path_returns_404`,
`TestParseAPIGWMethodPath_EdgeCases`'s two subtests) asserted the old bare
404 by status code alone; updated to assert the new, correct 400.

## 2026-08-28 — wrapper-key-sweep: CreateStage accepted three request members it doesn't have (acceptguard)

`cmd/acceptguard` flagged `CreateStage` reading `AccessLogSettings` and
`MethodSettings` from the request body; independently verifying against the
real SDK also turned up a third, `ClientCertificateID`, that acceptguard
only ranked "needs review" (it's a real member of a *different* op's
Input). Real `CreateStageInput` (`apigateway@v1.42.4` `api_op_CreateStage.go`)
has none of the three -- `AccessLogSettings`/`MethodSettings`/
`ClientCertificateId` are all real `Stage` (response) fields, but only
settable afterward via `UpdateStage`'s PATCH operations
(`/accessLogSettings/...`, `/*/*/...`, `/clientCertificateId`), never at
creation. Fixed by removing all three from `CreateStageInput`
(`models.go`) and no longer populating them in `CreateStage`
(`stages.go`); `UpdateStage`/`UpdateStageInput` were already correct and
unchanged.

Proven via a real `aws-sdk-go-v2/service/apigateway` client round trip in
`wire_field_fixes_test.go` (new):
`TestCreateStage_AccessLogAndMethodSettings_ViaUpdateStageRealClient` creates
a stage (asserting none of the three are set, since `CreateStageInput`'s Go
struct structurally cannot carry them), then sets all three via
`UpdateStage`'s `PatchOperations` and confirms they round-trip through both
the `UpdateStage` response and a follow-up `GetStage`. This test passes
both before and after the source fix -- the real SDK struct never had these
fields to send incorrectly, so there's no request-shape difference
observable through the typed client. The actual fail-before/pass-after
proof lives in `stages_test.go`'s Go-level backend tests
(`TestStage_ClientCertificateId_Create`, `TestBackend_Stage_ClientCertificateId`,
`TestStage_AccessLogSettings`, `TestStage_MethodSettings`), which
constructed `apigateway.CreateStageInput{...}` literals setting these three
fields directly -- exactly the bug the real SDK struct can't express.
Rewrote all four to `CreateStage` (no such fields) followed by `UpdateStage`
(setting them), matching the real two-step workflow; this doesn't compile
against the pre-fix `CreateStageInput` (which still had the fields, so the
literals would build but exercise the wrong path), confirming the tests
previously locked in incorrect behavior.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/apigateway/...`).

## 2026-08-28 — wrapper-key-sweep follow-up: GetDocumentationPart/DeleteDocumentationPart DocPartID (acceptguard, not a bug)

acceptguard flagged `getDocumentationPartInput.DocPartID`/`deleteDocumentationPartInput.DocPartID`
(`handler_documentation.go:144,196`) as matching no real member of `GetDocumentationPartInput`/
`DeleteDocumentationPartInput`. Investigated against apigateway@v1.42.4's serializer
(`awsRestjson1_serializeOpHttpBindingsGetDocumentationPartInput`, serializers.go:4810-4834):
`DocumentationPartId`/`RestApiId` are both `httpLabel`-bound (`encoder.SetURI(...)`) — pure URL
path segments, never a JSON body member on the real wire at all. No real client ever sends a
member literally named "documentationPartId"; the value is positional in the URL
(`/restapis/{id}/documentation/parts/{part_id}`).

gopherstack's router (`parseAPIGWRestAPIsDocDeep`, `handler_router.go`) already parses that
segment positionally off the real incoming URL and threads it through the JSON body merge
(`injectJSONFieldAPIGW`) under gopherstack's own internal key name, `docPartId` — this key is
router-to-handler plumbing, not a claim about the wire shape, and it doesn't need to match the
SDK's httpLabel name to work correctly. Confirmed with a real
`aws-sdk-go-v2/service/apigateway` client round trip
(`TestGetAndDeleteDocumentationPart_RealSDKPathParamRoundTrip`, `wire_field_fixes_test.go`):
create, get, delete, get-again-404, all pass unmodified. **Verdict: false positive, code left
unchanged.**

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/apigateway/...`).

- **2026-08-29 error-path sweep**: protocol confirmed REST-JSON
  (`awsRestjson1_*` serializer prefix) before relying on it. All 124
  `awsRestjson1_deserializeOpError*` functions extracted from
  `apigateway@v1.42.4/deserializers.go` (matching the 124 real SDK ops
  confirmed by `TestSDKCompleteness`). The modeled set is unusually flat
  across this service: nearly every op models the same core
  `BadRequestException`/`ConflictException`/`NotFoundException`/
  `TooManyRequestsException`/`UnauthorizedException` group, with
  `LimitExceededException` on most mutating ops and a rare
  `ServiceUnavailableException` limited to the four `Deployment` ops
  (`CreateDeployment`/`GetDeployment`/`GetDeployments`/`UpdateDeployment`).
  Wire mechanism: a single service-wide `sentinel -> errType` switch
  (`handler.go`'s `handleError`), not a per-op table.

  Spot-checked every op whose modeled set narrows below the family default
  (the ops missing `BadRequestException` -- `DeleteMethod`, `GetMethod`,
  `GetMethodResponse`, `GetResource`, `GetDocumentationVersion` -- and the
  handful missing `NotFoundException` entirely -- `CreateDomainName`,
  `CreateDomainNameAccessAssociation`, `CreateRestApi`, `CreateVpcLink`,
  `GenerateClientCertificate`) against their real backend call sites
  (`methods.go`, `resources.go`, `documentation.go`): each raises only the
  sentinel(s) its own operation actually models. No wrong-sentinel,
  fabricated-code, or missing-error bug found in this class this pass --
  **this service comes back clean for error-path parity** at the sampled
  depth above (every table-narrowing deviation checked; the flat majority of
  ops sharing the family default was not individually re-verified per op
  given the uniformity already confirmed). `LimitExceededException`/
  `TooManyRequestsException`/`UnauthorizedException`/
  `ServiceUnavailableException` have no corresponding backend logic (no
  account-level resource quotas, no request throttling, no deployment
  service-unavailable simulation) to ever raise them on the control plane --
  feature gaps, not sentinel bugs. (`ErrQuotaExceeded`/`ErrThrottled` in
  `errors.go` are real and wired, but serve the data-plane request-proxy path
  -- `proxy.go`'s usage-plan throttle/quota enforcement on an actual API
  invocation -- not any control-plane SDK operation in this table.)

## 2026-08-30 gopherstack-wlo1: error-envelope re-verification (N-of-N)

Re-visited as part of a 5-service error-envelope sweep (lightsail,
medialive, pinpoint, quicksight, apigateway). Confirmed all 124
`deserializeOpError` functions in `deserializers.go` (124-of-124, not
sampled) are identical generated boilerplate reading `X-Amzn-ErrorType`
then `restjson.GetErrorInfo` -- the gopherstack-wlo1 fix above (and the
`c6554e9f8`/`gopherstack-o7gx` fixes it references) covers the whole
surface. Traced every error-writing path (`handleError`,
`writeJSONProtocolDispatchError`) to confirm both `handleRESTAPI` (the real
client's path) and `handleJSONProtocol` funnel to the same `{"__type",
"message"}` envelope; no bypass found.

Added `TestErrorEnvelope_GetRestApiNotFoundDecodesToTypedError`
(`error_envelope_test.go`) exercising a genuinely modelled exception
(`GetRestApi` on a nonexistent API -> `*types.NotFoundException` via
`errors.As`), complementing the existing dispatch-miss tests which use the
framework-only `UnknownOperationException` (not a concrete SDK type).
Also asserts on the raw response bytes for the same case. Passed against
unmodified code -- no bug found.

Gates (this pass, `services/apigateway/` only): `go build`, `go vet`,
`go test -race -count=1`, `golangci-lint run` -- all clean.

## 2026-09-04 gopherstack-fum: data-plane routing audit (does a real request reach the integration?)

Focus this pass was the question prior sweeps hadn't asked directly: for a real
HTTP request against a *deployed* API, does it actually route through to its
configured integration, or is configuration accepted at PutIntegration time and
never genuinely exercised? Per-integration-type verdict:

| Integration type | Verdict |
|---|---|
| AWS_PROXY (Lambda proxy) | wired and executing -- event shape (`LambdaProxyEvent`) and response translation (`statusCode`/`headers`/`body`/`isBase64Encoded`) verified against AWS's documented proxy integration contract |
| AWS, Lambda target (non-proxy, VTL) | wired and executing -- request/response VTL templates render, Lambda invoked via the injected `LambdaInvoker` |
| AWS, sqs SendMessage / sns Publish direct integration | FIXED 2026-09-06 (gopherstack-is2a) -- wired and executing via a simplified passthrough, see the dated note below and gaps |
| AWS, other non-Lambda service target (DynamoDB/S3/Step Functions/etc.) | **accepted but never executes** -- see gaps below |
| HTTP / HTTP_PROXY | wired and executing -- real outbound `http.Client` request to `integration.URI` |
| MOCK | wired and executing |
| VPC_LINK | not a distinct `Integration.Type` in real AWS (it's HTTP/HTTP_PROXY + `connectionType=VPC_LINK`); routed identically to a plain HTTP_PROXY request -- no VPC/NLB emulation exists, `connectionId` is not separately validated. Structural simplification, not a "never executes" bug. |
| Authorizers (TOKEN/REQUEST/COGNITO_USER_POOLS) | wired and executing -- Lambda invoke for TOKEN/REQUEST, local JWKS verification for COGNITO_USER_POOLS |
| API key + usage plan enforcement | wired and executing -- real 429s (`LimitExceededException`/`TooManyRequestsException`) |

**Bug found and fixed: stage deployment did not gate routing at all.**
`handleProxyRequest` (proxy.go) matched the incoming request against
`Backend.ResourcesForRouting(apiID)` -- the RestApi's *live* current resource
tree -- and never checked whether the URL's `{stage}` segment named a stage
that a real `CreateDeployment` had actually produced. Consequences, both
reproduced before fixing:

1. A RestApi with `PutMethod`/`PutIntegration` configured but **no
   `CreateDeployment` ever called** was still fully invocable via
   `/proxy/{apiId}/{anyStage}/...` or
   `/restapis/{apiId}/{anyStage}/_user_request_/...` -- with `{anyStage}` an
   entirely made-up string. AWS refuses any request to an undeployed API.
2. Even with a real deployment, invoking with the *wrong* stage name in the
   URL still routed successfully, because the stage name was only used as a
   path-prefix string to strip, never validated against `Backend.GetStage`.
3. Unmatched routes (resource path, or method with no integration) returned a
   bare `http.NotFound` -- HTTP 404 with body "404 page not found" -- where
   real API Gateway returns HTTP 403 with body
   `{"message":"Missing Authentication Token"}` and header
   `X-Amzn-Errortype: MissingAuthenticationTokenException` (confirmed via
   live web search against AWS's documented behavior and the well-known
   "Missing Authentication Token" troubleshooting pattern; this fires for an
   invalid/undeployed stage, an unmatched path, a method with no integration,
   or a root resource with no method -- before authentication is ever
   considered, despite the name).

Fixed in `proxy.go`: `handleProxyRequest` now checks `Backend.GetStage(apiID,
stageName)` before matching a route, and every "no match" branch (bad stage,
unmatched resource, unmatched method/integration) now calls the new
`writeMissingAuthenticationTokenResponse` (403 + the AWS body/header shown
above) instead of `http.NotFound`. `services/apigateway/proxy_test.go`'s
`TestHandleProxyRequest_RequiresDeployedStage` (new) and
`services/apigateway/proxy_validation_test.go`'s
`TestProxy_TrieCache_InvalidatesOnNewResource` (existing, assertion updated)
both confirmed failing against the pre-fix binary and passing post-fix; three
more existing subtests (`TestHandleAWSProxy/not_found`,
`TestUserRequestEndpoint/not_found`,
`TestPathVariableMatching/param_no_match_wrong_depth`) had their expected
status updated from 404 to 403 for the same reason (they were asserting the
old, wrong behavior).

**Bug found and fixed: `h.trieCache` (compiled routing-trie cache) leaked on
DeleteRestApi.** `h.trieCache` is a `sync.Map` keyed by RestApi ID, holding the
compiled routing trie built from that API's resources. `deleteRestAPIAction`
(handler_rest_apis.go) called `Backend.DeleteRestAPI` but never
`h.trieCache.Delete` -- and since RestApi IDs are freshly random on every
`CreateRestApi`, a deleted API's stale trie-cache entry could never later be
overwritten by a `Store` under the same key. Every RestApi ever routed to and
then deleted stays in process memory for the remainder of the server's
lifetime -- an unbounded leak in any long-running gopherstack process (e.g.
integration-test suites that create/delete many APIs). Fixed by adding
`h.trieCache.Delete(input.RestAPIID)` after a successful backend delete.
`TestDeleteRestAPI_EvictsTrieCache` (new, `proxy_internal_test.go`, white-box)
confirmed failing pre-fix and passing post-fix.

**Confirmed 2026-09-04, PARTIALLY FIXED 2026-09-06 (bd: gopherstack-is2a):
"AWS" (non-proxy) integration only ever invoked Lambda.**
`handleAWSIntegration` (proxy_integrations.go) used to call
`ExtractLambdaFunctionName(integration.URI)` and `h.lambda.InvokeFunction`
unconditionally, regardless of what AWS service the URI actually names. A
real, AWS-documented direct "AWS" integration targeting DynamoDB, SQS, SNS,
S3, Step Functions, etc. (e.g.
`uri: "arn:aws:apigateway:us-east-1:dynamodb:action/PutItem"`, a pattern AWS
explicitly documents for API Gateway → DynamoDB direct integrations) was
accepted by `PutIntegration` with zero validation of the target service, but
the target action never executed: reproduced getting either a 503 ("Lambda
integration not configured", no invoker wired) or, with the production
`LambdaInvoker` wired the way `cli.go`'s `wireAPIGatewayLambda` wires it, a
Lambda "function not found" failure (`ExtractLambdaFunctionName` returns the
DynamoDB ARN unchanged, which resolves to no real Lambda function). This
matched the campaign's "accepted on the wire, silently does nothing in a
real server" bug class exactly.

**What gopherstack-is2a fixed:** two targets with both a real backing service
in this repo and an unambiguous single action -- sqs `SendMessage` and sns
`Publish`, the two candidates the retriage named. `awsIntegrationTarget`
(proxy_integrations.go) parses the URI per the grammar quoted above and,
when the target is `sqs` (path-style,
`arn:aws:apigateway:{region}:sqs:path/{accountId}/{queueName}`) or `sns`
(action-style, `arn:aws:apigateway:{region}:sns:action/Publish`) *and* the
corresponding hook is wired, dispatches there instead of Lambda. The hooks
are `SQSSender.SendMessageToQueue` and `SNSPublisher.PublishToTopic`
(proxy.go), set via `Handler.SetSQSSender`/`SetSNSPublisher` (handler.go) and
wired in `cli.go`'s `wireAPIGatewaySQSSNS`, which reuses the
`sqsSenderAdapter`/`snsPublisherAdapter` types already declared for
eventbridge (same method sets, satisfied structurally, no new adapter code
needed). This module has no VTL/Velocity library, so the dispatch is a
**documented simplified passthrough, not mapping-template evaluation**: the
request payload -- raw body, or run through the pre-existing
`RenderTemplate`/`VTLContext` machinery in vtl.go if the integration has a
`requestTemplates["application/json"]` configured, exactly as the Lambda
path already did -- becomes the SQS `MessageBody` or SNS `Message` verbatim,
with none of real API Gateway's `Action=...&...` AWS query-protocol
encoding. SNS's `TopicArn` is not encoded in an action-style URI at all in
real AWS either -- it resolves from the integration's `RequestParameters`
mapping (`integration.request.querystring.TopicArn`) or, absent one, a
`TopicArn` query parameter on the incoming request; if neither resolves, the
call fails with a 500 rather than guessing. A successful call's HTTP
response is a bare `"{}"` run back through the existing
`applyResponseTemplate` status-code/response-template matching -- not a real
SQS/SNS response shape (message ID, MD5 of body, etc. are not modeled).

**What is still NOT fixed, unchanged from before:** DynamoDB, Kinesis, Step
Functions, S3, and any other "AWS" integration target still unconditionally
invoke Lambda exactly as before -- `awsIntegrationTarget` only recognizes
`sqs` and `sns`, so every other service token falls straight through to the
original `h.lambda.InvokeFunction` call, unmodified. sqs/sns integrations
also keep this exact original behavior when no `SQSSender`/`SNSPublisher` is
wired -- an unwired hook is a silent no-op, never a rejection, matching the
convention this repo already uses for LambdaInvoker and the ~150 services
whose test backends build with no cross-service hooks at all
(`TestHandleAWSIntegration_SQSTarget_Unwired`,
`TestHandleAWSIntegration_SNSTarget_Unwired`).
`TestHandleAWSIntegration_LambdaTarget_StillRoutesToLambda` confirms the
Lambda path itself is unaffected. All four new dispatch tests
(`TestHandleAWSIntegration_SQSTarget`, `_SQSTarget_SendError`,
`_SNSTarget`, `_SNSTarget_TopicUnresolved`) were verified to fail against
the pre-fix code: reverting proxy_integrations.go alone (via `git show
HEAD:...`) while keeping the new `SetSQSSender`/`SetSNSPublisher`
interfaces/hooks in proxy.go/handler.go, the package still compiled and
every one of the four failed (503, not the expected 200/500) before the
fix, and passed after restoring it. Fixing the remaining targets for real
needs either new per-service invoker interfaces (DynamoDB, Kinesis, Step
Functions, S3, ...) or a real VTL evaluator plus AWS query-protocol
request/response encoding for every target -- a larger, multi-service change,
not a narrow follow-up.

**Documented, NOT fixed (large architectural change): `CreateDeployment`
does not freeze a routable snapshot.** `Deployment.APISummary` is a
lightweight, display-only summary (matching the real SDK's `ApiSummary`
field) -- it is never consulted by the data plane. `routingTrie` always
calls `Backend.ResourcesForRouting(apiID)`, which reads the RestApi's *live*
current resource/method/integration state, with no notion of "as of which
deployment". Reproduced: deploy a stage, then delete a resource with **no**
new deployment -- the already-deployed stage's behavior changes immediately
(the deleted route now 403s), where real AWS keeps serving the old, deployed
configuration until a new deployment is created and the stage is repointed
to it. A real fix needs a per-deployment resource/method/integration
snapshot plus stage-to-deployment pinning enforced in the data plane --
tracked here as a known gap, not attempted this pass.

Five-dimension summary:

1. **AWS behavior compliance**: BUGS FOUND (documented above: deployment
   gating, 403-vs-404 error shape, AWS-non-Lambda integration gap). Checked
   the data-plane routing/authorizer/API-key/usage-plan paths in depth; did
   not re-verify the (already extensively audited in prior sweeps) control-
   plane CRUD wire shapes/PATCH semantics this pass.
2. **LocalStack parity**: NOT CHECKED -- no LocalStack instance available.
3. **Cross-service integration**: BUGS FOUND -- the AWS-non-Lambda-target gap
   above is squarely this dimension. Lambda proxy event/response shape
   verified correct (AWS_PROXY). Cognito JWKS and Lambda-invoker wiring in
   `cli.go` verified present and correctly connected (read-only check).
4. **Performance**: CLEAN (checked) -- routing trie is built once per
   resource-set version and cached (`routingTrie`/`trieCacheEntry`), not
   rebuilt per request; selection-pattern regexps are cached in a bounded LRU
   (`regexpCache`); no obvious O(n) per-request scan found in the request
   hot path.
5. **Resource leaks**: BUGS FOUND AND FIXED -- the `h.trieCache` leak above.
   Did not separately re-audit every other map in the service for the same
   class of issue this pass (authorizer cache and regexp cache are already
   bounded LRUs per prior sweeps).

Unconfirmed suspicions (not chased further this pass, no reproduction
attempted): `mockIntegrationResponse` only ever looks at the `"200"`
`IntegrationResponses` entry for MOCK integrations, never applying
`selectionPattern`-based response selection the way `matchIntegrationResponse`
does for AWS (non-proxy) integrations -- real MOCK integrations are commonly
configured with a `#set($context.responseOverride.status = ...)` request
template to pick a non-200 response; whether that's honored here wasn't
verified.

Gates: `gofmt -l services/apigateway/` clean; `GOTOOLCHAIN=go1.26.6 go build
./services/apigateway/...`, `go vet`, `go test -race -count=1`, and
`golangci-lint run` all clean on `./services/apigateway/...`.
