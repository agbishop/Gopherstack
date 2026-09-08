---
service: apigatewayv2
sdk_module: aws-sdk-go-v2/service/apigatewayv2@v1.37.4
last_audit_commit: ca3a1e21f
last_audit_date: 2026-09-08
overall: A            # 2026-09-08 (gopherstack-wsvb, P1): enforceRouteThrottle/enforceRouteAuth
                       # (http_proxy.go) and enforceIAMAuth/enforceRequestAuthorizer/
                       # finishAuthDecision (authorizers.go) rejected a request by writing its
                       # 429/401/403 via writeErr and returning that call's result -- c.JSON
                       # returns nil after a successful write, so applyRouteControls' and
                       # handleHTTPAPIProxy's "if ctrlErr != nil" checks never fired and a
                       # throttled or unauthorized (JWT/CUSTOM/AWS_IAM) request was forwarded to
                       # the real integration anyway, even though the client already received a
                       # 429/401/403. Same class as elasticache (gopherstack-8haq) and pinpoint
                       # (gopherstack-246v). Fixed with the pinpoint raw-unwritten-error pattern:
                       # every enforce* helper in the chain now returns one of a small set of
                       # unwritten sentinel errors (errRoute{Unauthorized,Forbidden,ExplicitDeny,
                       # MissingAuthToken,AuthConfigInvalid}, errors.go), and a single new
                       # writeRouteControlRejection maps and writes the response exactly once, at
                       # handleHTTPAPIProxy -- no sentinel-plus-inline-write shape (elasticache's
                       # errResponseWritten) was introduced; the fan-out didn't warrant it.
                       # enforceRouteThrottle's fail-open default branch (an unexpected backend
                       # error still returns nil, allowing the request) is preserved unchanged --
                       # a separate, deliberate design decision, not part of this bug. New and
                       # strengthened tests prove the integration was NOT invoked, not just that
                       # the right status code came back -- a status-only assertion passes against
                       # this bug, since c.JSON's first WriteHeader call wins the response and the
                       # integration's later write only corrupts the body underneath it. Covers
                       # every write site in the chain: route throttled
                       # (TestHTTPAPIProxy_RouteThrottle_TooManyRequests, strengthened), JWT
                       # authorizer missing (new
                       # TestHTTPAPIProxy_JWTAuthorizerMissing_DoesNotInvokeIntegration), JWT
                       # validation failure (TestHTTPAPIProxy_JWTAuthorizer, strengthened),
                       # AWS_IAM unsigned request (TestRouteAuth_AWSIAM, strengthened), and the
                       # CUSTOM/REQUEST authorizer paths found to share the same shape: missing
                       # authorizerId, missing identity source, and explicit/implicit deny in both
                       # the simple-response and IAM-policy-response shapes (all in
                       # handler_authorizers_test.go, strengthened). See Notes.
                       # ---- prior pass's note follows ----
                       # route-throttle enforcement pass (this pass, 2026-09-06, gopherstack-dv44).
                       # RouteSettings/DefaultRouteSettings.Throttling{Rate,Burst}Limit were
                       # stored (CreateStage/UpdateStage) and echoed back, but handleHTTPAPIProxy
                       # dispatched straight to the integration with no limiter ever consulted --
                       # a configured limit had zero effect. Fixed: a new per-(api,stage,routeKey)
                       # token-bucket limiter (throttle.go), enforced in applyRouteControls before
                       # the route authorizer (mirrors apigateway v1's gopherstack-91f2 stage-
                       # throttle-before-authorizer precedence), returning AWS's real 429
                       # TooManyRequestsException shape. RouteSettings[routeKey] fully replaces
                       # DefaultRouteSettings when present (not merged), matching v1's
                       # MethodSettings "*/*" override convention; a zero/unset
                       # ThrottlingRateLimit means unlimited, also matching v1. Bucket state is
                       # ephemeral (not persisted, like v1's usageTracker) and is evicted on
                       # DeleteStage, DeleteAPI (cascade), DeleteRoute, and UpdateRoute route-key
                       # rename, closing the same ghost-row class v1 closed for DeleteStage.
                       # ---- prior pass's note follows ----
                       # data-plane sweep pass (2026-09-04, gopherstack-vli). Focused
                       # on the data plane (handleProxy/handleHTTPAPIProxy/invokeWSRoute), not
                       # re-covered here: control-plane wire-shape/pagination/leak ground already
                       # swept by prior passes below. Found and fixed three real bugs: (1)
                       # handleProxy never checked the URL's stage-name segment against the
                       # backend at all -- any string, including a stage that was never created
                       # via CreateStage, routed straight through to the live route/integration
                       # (proxy.go); now gated with a GetStage check returning 404 before protocol
                       # dispatch, closing this for both HTTP and WebSocket APIs. (2)
                       # CreateIntegration/UpdateIntegration accepted AWS/HTTP/MOCK integration
                       # types on HTTP-protocol APIs with no protocol check at all, even though
                       # api_op_CreateIntegration.go's IntegrationType doc comment states each of
                       # those three is "Supported only for WebSocket APIs" -- an HTTP API could
                       # accept e.g. MOCK at CreateIntegration time and only fail opaquely (500)
                       # at invoke time; now rejected with BadRequestException at Create/Update.
                       # (3) invokeWSRoute (WebSocket data plane) recognized only AWS_PROXY and
                       # rejected every other integration type with ErrUnsupportedType --
                       # including MOCK, a genuinely valid WebSocket integration type whose SDK
                       # doc comment says it is "a 'loopback' endpoint without invoking any
                       # backend"; a $connect route on a MOCK integration therefore always failed
                       # with 403 Forbidden. Now MOCK short-circuits to success without invoking
                       # anything, matching the doc verbatim. See Notes #16-18 and the per-
                       # integration-type verdict table in the audit report. Deliberately NOT
                       # fixed (see gaps): a stage only gates on *existing*, not on ever having
                       # been deployed (DeploymentId != "") or on serving a point-in-time
                       # snapshot rather than the API's live current config -- see gaps for why.
                       # ---- prior pass's note follows ----
                       # write-only-state sweep pass (2026-08-28). Existing
                       # wire_field_fixes_test.go (ListRoutingRules wrapper key, Portal
                       # PublishStatus) was a PARTIAL prior pass, not a finished one -- per this
                       # campaign's protocol, treated as a signal to dig deeper rather than skip.
                       # Ran the write-only-state method (what does each backend persist, what real
                       # op reads it back) across the Api/Stage/Route/Integration/Authorizer/
                       # Deployment/DomainName/VpcLink/RoutingRule families. Found one real bug:
                       # UpdateAuthorizer's AuthorizerResultTtlInSeconds/EnableSimpleResponses were
                       # plain int32/bool with a truthy/nonzero guard (not *int32/*bool like the
                       # real SDK), so a client's documented way to explicitly disable caching
                       # (TTL=0) or simple responses (false) was silently dropped -- fixed, see
                       # UpdateAuthorizer row and Notes. enumcheck: 0 findings in this service.
                       # apigatewayv2 is REST-shaped (path-bound members via echo routes in
                       # handler.go, e.g. /v2/apis/{apiId}/authorizers/{authorizerId}), confirmed
                       # against the vendored SDK's httpBindingEncoder-based serializers.go/
                       # api_op_*.go for the ops this pass touched. Did not re-verify every op in
                       # this large service (24k lines) -- see gaps for scope not reached.
                       # ---- query/header-to-non-string-field sweep (this pass, 2026-08-29) ----
                       # Hunted for query/header values fed into a non-string Go field without
                       # conversion (the apigateway-v1 Limit-into-JSON-body class). No merging
                       # pattern here (nothing merges query values into the JSON body) and no
                       # hard-fail found. Inventoried every non-string query/header/path member
                       # across all 103 ops: MaxResults is *string on every Get*/List sibling
                       # except ListRoutingRules (*int32, serializers.go:6988) -- all correctly
                       # parsed via apigwPaginationParams/strconv. Found and fixed two inert
                       # (SILENT) params: ExportApi's IncludeExtensions (*bool) and
                       # ListRoutingRules' MaxResults/NextToken were declared but never read. See
                       # ExportApi/ListRoutingRules rows.
                       # ---- prior pass's note follows ----
                       # gopherstack-0xs7 follow-up pass. Verified against live code (not
                       # PARITY.md prose) that gopherstack-e81/2tx/jni0 were all still genuinely
                       # open, then closed the real parts of each: RoutingRule Actions/Conditions
                       # are now typed unions (gopherstack-e81, see Notes #12); UpdateRoute now
                       # blocks route-key changes and UpdateStage blocks all changes on
                       # quick-create-managed resources (gopherstack-2tx, partial -- see gaps);
                       # ImportApi/ReimportApi now read+validate basepath/failOnWarnings and
                       # implement basepath=prepend (gopherstack-jni0, partial -- see gaps). Also
                       # swept for and fixed three "state mutated before validation" bugs
                       # (UpdateRoute, UpdateAPI, UpdateDomainName -- see Notes #13) and two
                       # under-validated RoutingRule inputs (priority range, action/condition
                       # required sub-fields and API/stage FK existence). Portal/PortalProduct/
                       # ProductPage/ProductRestEndpointPage family re-counted against botocore:
                       # 26 operations (31 including RoutingRule's 5), all 26 already implemented
                       # with real backend state (not stubs) -- the family is NOT the large
                       # unmodelled surface a prior pass's note speculated it might be.
                       # ---- prior pass's note follows ----
                       # re-audit pass (parity-3 campaign). The previously recorded
                       # last_audit_commit (d6fae6df) was a ledger bug, not a valid baseline: that
                       # commit's own message is "parity(apigateway): ..." and its diffstat touches
                       # only services/apigateway (the v1 REST API service), never
                       # services/apigatewayv2 -- it was almost certainly pasted from the wrong
                       # session. The real predecessor commit (the one that last wrote this file)
                       # is efc42cbc4 ("Parity 4"), confirmed via `git log -- services/apigatewayv2/
                       # PARITY.md`; corrected here. Diffing efc42cbc4..HEAD showed zero local drift
                       # to apigatewayv2/*.go (the two intervening commits touching this repo were a
                       # docs/gendocs rewrite and a pure-reorg refactor of *other* services), same
                       # pinned SDK version. Independent field-diff of the in-scope surface (Apis,
                       # Stages, Routes, Integrations (+responses), RouteResponses, Authorizers,
                       # Deployments, DomainNames (+ApiMappings), VpcLinks, Models, ExportApi, Tags)
                       # against aws-sdk-go-v2/service/apigatewayv2@v1.33.7/types/types.go and the
                       # per-op api_op_*.go input/output structs turned up five more genuinely
                       # missing wire fields the prior pass's field-diff missed (Integration.
                       # CredentialsArn, Api.IpAddressType, CreateApi/UpdateApi's quick-create-only
                       # CredentialsArn, Api.ImportInfo/Warnings, DomainName.RoutingMode) plus a real
                       # fix for the previously-deferred authorizerCache leak (bd gopherstack-wmh,
                       # now closed) and a newly-found ImportApi/ReimportApi query-param gap (bd
                       # gopherstack-jni0, deferred -- see gaps). All fixed for real except the
                       # newly-filed gap. RoutingRule wire:partial rows, the quick-create
                       # immutability gap (gopherstack-2tx), and the Portal/PortalProduct family
                       # (out of this pass's declared scope, per the task's op list) were
                       # re-confirmed as still accurate/deliberately out of scope, not re-touched.
                       # ---- sort-totality sweep, Class F/G (this pass, 2026-08-30) ----
                       # Reviewed every sort.Slice call site across every paginated listing in this
                       # service (apis/api_mappings/api_models/authorizers/deployments/domain_names
                       # incl. RoutingRules/integrations/integration_responses/routes/
                       # route_responses/portals/portal_products/stages/vpc_links). Every one sorts
                       # on that resource's own real unique ID (APIID/ModelID/APIMappingID/
                       # AuthorizerID/DeploymentID/RoutingRuleID/DomainNameValue/IntegrationID/
                       # IntegrationResponseID/RouteID/RouteResponseID/PortalID/PortalProductID/
                       # StageName/VpcLinkID) -- confirmed each is that resource's primary/unique
                       # identifier, not assumed from the field name. No non-unique sort key found;
                       # no Class F bug. Confirmed no listing in this service returns two-or-more
                       # collections the API defines as one ordered sequence truncated
                       # independently -- no Class G candidate found. No code changes.
ops:
  CreateApi: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "routeKey+target quick-create shortcut was entirely unimplemented -- CreateAPIInput had no such fields at all, so real quick-create requests silently created a bare API with no route/integration/stage (fixed by a prior pass, see Notes #6). This pass: ipAddressType and quick-create's credentialsArn were ALSO entirely absent from CreateAPIInput -- fixed, see Notes #8-9."}
  GetApi: {wire: fixed, errors: ok, state: ok, persist: ok, note: "Api.ipAddressType/importInfo/warnings were entirely absent -- fixed, see Notes #8"}
  GetApis: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same Api shape fix as GetApi"}
  UpdateApi: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "routeKey/target (\"part of quick create\" per SDK doc comments) were also entirely absent from UpdateAPIInput (fixed by a prior pass, see Notes #6). This pass: ipAddressType and quick-create's credentialsArn were ALSO entirely absent from UpdateAPIInput -- fixed, see Notes #8-9. Also: was mutating Name/Description/etc. before validating ipAddressType and the quick-create routeKey/target/credentialsArn fields, so a rejected update could leave those partially applied -- fixed, see Notes #13."}
  DeleteApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "now also purges authorizerCache entries for the API's authorizers on cascade delete -- see Notes #11"}
  ImportApi: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "basepath and failOnWarnings query params (SetQuery in serializers.go, not body fields) are now read and validated instead of silently ignored; basepath=prepend now prefixes route paths with the spec's declared base path. basepath=split and failOnWarnings-triggered rollback remain unimplemented -- bd gopherstack-jni0, narrowed, see gaps. Api.importInfo/warnings shape itself is correct (Notes #8) but always empty since the emulator never generates import warnings, so failOnWarnings has no observable effect yet."}
  ReimportApi: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "same basepath/failOnWarnings fix as ImportApi -- bd gopherstack-jni0, narrowed"}
  ExportApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-h910): OutputType (required query param 'outputType', verified against validateOpExportApiInput/serializeOpHttpBindingsExportApiInput) was ignored and JSON was always returned. Now required (400 if missing/invalid) and YAML actually serializes via gopkg.in/yaml.v3 when requested. Also fixed (query/header wrapper-key sweep, this pass): IncludeExtensions (real *bool query param, api_op_ExportApi.go:52, serializers.go:3975) was never read, so AWS extension keys (x-amazon-apigateway-authtype and friends) were always emitted; now defaults true (AWS's documented default) and false strips them recursively. StageName/ExportVersion remain unwired -- StageName would need per-stage route filtering this backend's route model doesn't support (routes are API-level, not stage-scoped); ExportVersion is a cosmetic knob on the exported doc's own metadata, not state this backend tracks. Left absent rather than fabricated."}
  CreateRoute: {wire: ok, errors: ok, state: ok, persist: ok, note: "HTTP routeKey format + WS $connect/$disconnect/$default/custom validated; auth type NONE/AWS_IAM/JWT/CUSTOM enforced"}
  GetRoute: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRoutes: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateRoute: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "was mutating RouteKey before validating AuthorizationType, so a rejected update (bad auth type) could still leave a changed route key -- fixed by validating the whole input before mutating anything, see Notes #13. Also now rejects a route-key change on a quick-create $default route (gopherstack-2tx, see Notes #14)."}
  DeleteRoute: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateIntegration: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "was missing tlsConfig, hardcoded 29000ms ceiling/default for HTTP APIs (should be 30000ms), no connectionType default/validation (fixed by a prior pass). credentialsArn was ALSO entirely absent -- fixed, see Notes #7. This pass (gopherstack-vli): AWS/HTTP/MOCK integrationType were accepted on HTTP-protocol APIs with no protocol check at all, though each is 'Supported only for WebSocket APIs' per the SDK doc comment -- fixed, see Notes #16."}
  GetIntegration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "credentialsArn fix, see Notes #7"}
  GetIntegrations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same Integration shape fix as GetIntegration"}
  UpdateIntegration: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "same protocol-aware timeout + connectionType validation applied (prior pass); credentialsArn fixed this pass, see Notes #7. This pass (gopherstack-vli): same AWS/HTTP/MOCK-on-HTTP-API rejection as CreateIntegration, see Notes #16."}
  DeleteIntegration: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GetIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GetIntegrationResponses: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): GetIntegrationResponsesOutput.NextToken (declared on both input and output, apigatewayv2@v1.37.4) was never populated -- the shared nestedResponseOps.wrapList closure took only the item slice, dropping the cursor entirely. handleGetChildList now applies pkgs/page.New (via apigwPaginationParams) like every other list op in this package."}
  UpdateIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteIntegrationResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateRouteResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRouteResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  GetRouteResponses: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): same nestedResponseOps.wrapList gap as GetIntegrationResponses -- NextToken never populated. Fixed alongside it."}
  UpdateRouteResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRouteResponse: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateStage: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was missing clientCertificateId (WS-only) and Tags -- fixed"}
  GetStage: {wire: fixed, errors: ok, state: ok, persist: ok}
  GetStages: {wire: fixed, errors: ok, state: ok, persist: ok}
  UpdateStage: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "now rejects any modification of a quick-create $default stage (gopherstack-2tx, see Notes #14)"}
  DeleteStage: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAccessLogSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRouteSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRouteRequestParameter: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteCorsConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDeployment: {wire: ok, errors: ok, state: fixed, persist: fixed, note: "autoDeploy interaction verified. FIXED 2026-09-06 (gopherstack-cfr1): now snapshots the API's current routes and integrations onto the created Deployment (internal-only fields, not on the wire) -- see gaps and Notes #19."}
  GetDeployment: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDeployments: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDeployment: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDeployment: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateAuthorizer: {wire: ok, errors: ok, state: ok, persist: ok, note: "JWT issuer/audience + REQUEST identitySource/payloadFormatVersion/enableSimpleResponses/TTL all modeled and enforced on the data plane (http_proxy.go, authorizer.go)"}
  GetAuthorizer: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAuthorizers: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAuthorizer: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "write-only-state bug (gopherstack-wire-sweep, this pass): AuthorizerResultTtlInSeconds/EnableSimpleResponses were plain int32/bool (not *int32/*bool like the real SDK's UpdateAuthorizerInput, api_op_UpdateAuthorizer.go) with a truthy/nonzero guard, so a real client's documented way to disable caching (TTL=0) or simple responses (false) via Update was silently dropped, leaving the previous value forever. The Authorizer response shape also carried omitempty on both fields, which would have hidden a real 0/false value as an absent key on GetAuthorizer/ListAuthorizers -- also fixed. Round-trip test in wire_field_fixes_test.go. Follow-up sweep (this pass, wrapper-key sweep): the same != \"\" guard bug also affected the other four string fields of UpdateAuthorizerInput. Fixed three (AuthorizerURI, AuthorizerCredentialsArn, AuthorizerPayloadFormatVersion): none is required at CreateAuthorizer time (unlike Name), so a client explicitly clearing one -- e.g. dropping AuthorizerCredentialsArn to switch to resource-based Lambda permissions, per its own doc ('don't specify this parameter') -- is a legitimate state, not an error; converted to *string with a nil check. Response side (Authorizer.AuthorizerURI/AuthorizerCredentialsArn/AuthorizerPayloadFormatVersion, models.go) intentionally kept omitempty, unlike TTL/EnableSimpleResponses above -- these three are commonly N/A altogether (e.g. a JWT authorizer never sets AuthorizerURI at all), and stripping omitempty would put spurious empty keys on the common case rather than only the rare explicit-clear case. Left Name unfixed as a silent-ignore: unlike the other three, Name IS required at CreateAuthorizer ('This member is required'), so no authorizer has a valid empty-Name state -- converted to *string too, but an explicit empty value is now rejected with a BadRequestException (fixed handleUpdate's generic error mapping in handler.go, which had never routed ErrBadRequest to 400 for any Update op, to make this correct) instead of either silently ignored or silently applied. Round-trip tests: wire_field_fixes_test.go (TestUpdateAuthorizer_URICredentialsAndPayloadVersionCanBeCleared, TestUpdateAuthorizer_EmptyNameRejected)."}
  DeleteAuthorizer: {wire: ok, errors: ok, state: ok, persist: ok, note: "now purges authorizerCache entries for this authorizer -- see Notes #11 (bd gopherstack-wmh, closed)"}
  ResetAuthorizersCache: {wire: ok, errors: ok, state: ok, persist: n/a, note: "cache is in-memory only by design"}
  CreateModel: {wire: ok, errors: ok, state: ok, persist: ok}
  GetModel: {wire: ok, errors: ok, state: ok, persist: ok}
  GetModels: {wire: ok, errors: ok, state: ok, persist: ok}
  GetModelTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateModel: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteModel: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDomainName: {wire: fixed, errors: ok, state: ok, persist: ok, note: "was missing mutualTlsAuthentication and domainNameArn (fixed by a prior pass). This pass: routingMode was ALSO entirely absent -- fixed, see Notes #10."}
  GetDomainName: {wire: fixed, errors: ok, state: ok, persist: ok, note: "routingMode fix, see Notes #10"}
  GetDomainNames: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same DomainName shape fix as GetDomainName. FIXED 2026-08-29 (cursor-pagination sweep): GetDomainNamesOutput.NextToken was never populated -- handler called h.Backend.GetDomainNames() and returned the full slice with no pagination at all. Now routed through apigwPaginationParams + pkgs/page.New like GetAPIs/GetDeployments/etc."}
  UpdateDomainName: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "routingMode fix, see Notes #10. This pass: was also mutating Tags/DomainNameConfigurations/MutualTLSAuthentication before validating RoutingMode, so a rejected update could leave those partially applied -- fixed, see Notes #13."}
  DeleteDomainName: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateApiMapping: {wire: ok, errors: ok, state: ok, persist: ok}
  GetApiMapping: {wire: ok, errors: ok, state: ok, persist: ok}
  GetApiMappings: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): GetApiMappingsOutput.NextToken was never populated -- no pagination applied at all. Now routed through apigwPaginationParams + pkgs/page.New."}
  UpdateApiMapping: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApiMapping: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  GetVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  GetVpcLinks: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (cursor-pagination sweep): GetVpcLinksOutput.NextToken was never populated -- no pagination applied at all. Now routed through apigwPaginationParams + pkgs/page.New."}
  UpdateVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteVpcLink: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateRoutingRule: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "Actions/Conditions are now typed AWS union shapes (RoutingRuleAction/RoutingRuleActionInvokeAPI, RoutingRuleCondition/RoutingRuleMatchBasePaths/RoutingRuleMatchHeaders/RoutingRuleMatchHeaderValue) instead of []map[string]any passthrough, with required-subfield and FK (target api/stage must exist) validation, plus RoutingRulePriority's modeled [1,1000000] range -- gopherstack-e81, closed, see Notes #12."}
  GetRoutingRule: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same typed-shape fix as CreateRoutingRule"}
  ListRoutingRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "same typed-shape fix as CreateRoutingRule. Also fixed (query/header wrapper-key sweep, this pass): MaxResults/NextToken (real *int32/*string query params, api_op_ListRoutingRules.go:40-45, serializers.go:6988 -- the one List op in this service where MaxResults is int32, unlike every Get*/List sibling's *string MaxResults) were never read at all, so every rule always came back in one page regardless of the limit a client asked for. Now paginates via the shared apigwPaginationParams/page.New path like every other List/Get collection op."}
  PutRoutingRule: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "same typed-shape + validation fix as CreateRoutingRule"}
  DeleteRoutingRule: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "now supports stage ARNs (arn:.../apis/{id}/stages/{name}) in addition to apis/vpclinks/domainnames; 404s were surfacing as 500 for stage ARNs before the errStageNotFound check was added to the handler"}
  UntagResource: {wire: fixed, errors: fixed, state: ok, persist: ok}
  GetTags: {wire: fixed, errors: fixed, state: ok, persist: ok}
families:
  Portal/PortalProduct/ProductPage/ProductRestEndpointPage (preview APIGW "portals" feature): {status: ok, note: "gopherstack-0xs7 pass counted the family against botocore apigatewayv2/2018-11-29: 26 operations (CreatePortal/GetPortal/ListPortals/UpdatePortal/DeletePortal/PreviewPortal/PublishPortal/DisablePortal, the same 5 for PortalProduct, Create/List/Get/Update/Delete for ProductPage and ProductRestEndpointPage, Get/Put/DeletePortalProductSharingPolicy). All 26 are implemented with real backend state in portals.go/handler_portals.go (confirmed via GetSupportedOperations() and backend method presence) -- NOT a large unmodelled surface as a prior pass's note speculated. PreviewPortal returns the live Portal (a reasonable preview simulation, not a stub). 2026-08-23 (manifest harvest): did the field-level wire audit this note deferred, against aws-sdk-go-v2/service/apigatewayv2@v1.37.4's api_op_{Create,Update,Get}Portal.go/types.PortalSummary. Found and fixed 3 real accept-and-drop bugs on the Portal type: CreatePortalInput/UpdatePortalInput.IncludedPortalProductArns (a *required* PortalSummary member) and .RumAppMonitorName were decoded off the wire into nothing (no backing field existed) and silently dropped on both Create and Update; PublishPortalInput.Description ('When the portal is published, this description becomes the last published description' -- api_op_PublishPortal.go) was decoded but never used, and GetPortalOutput.LastPublished/LastPublishedDescription had no backing field at all. Added Portal.IncludedPortalProductArns/RumAppMonitorName/LastPublished/LastPublishedDescription (models.go), wired through CreatePortal/UpdatePortal/handlePublishPortal (portals.go/handler_portals.go). GetPortalOutput.Preview/StatusException remain correctly unmodeled -- see gaps. UpdatePortalInput is ALSO missing Authorization/EndpointConfiguration/PortalContent entirely (all three real, optional UpdatePortalInput members -- api_op_UpdatePortal.go); NOT fixed this pass, newly disclosed as a gap (see below) rather than rushed alongside the three accept-and-drop fixes. FIXED (constraint sweep, this pass): ListPortals/ListPortalProducts/ListProductPages/ListProductRestEndpointPages all declare real maxResults/nextToken query params (query-bound, confirmed via each op's own httpBindings serializer) but the handlers called the backend with no pagination args at all -- every item always came back on one page. Wired through apigwPaginationParams/page.New, the same pattern GetApis etc. already use. ListPortalProducts/ListProductPages/ListProductRestEndpointPages' ResourceOwner/ResourceOwnerAccountId query params remain unfiltered: PortalProduct/ProductPage/ProductRestEndpointPage carry no ownership-account field to filter on, so honoring them would mean inventing a model field -- left as a disclosed gap, not fixed."}
  WebSocket @connections data plane (apigatewaymanagementapi): {status: ok, note: "delegated to services/apigatewaymanagementapi via SetManagementAPIBackend; out of scope for this apigatewayv2-only sweep"}
gaps:
  - "Quick-create route/stage immutability partially enforced (gopherstack-2tx, narrowed): UpdateRoute
    now rejects a route-key change on an apiGatewayManaged route (\"You can't modify the $default
    route key\") and UpdateStage now rejects any modification of an apiGatewayManaged stage (\"You
    can't modify the $default stage\") -- both backed by BadRequestException, which IS in
    UpdateRoute/UpdateStage's modeled error set (service-2.json). Still NOT enforced:
    DeleteRoute/DeleteStage/DeleteIntegration on a managed resource. Deliberately not extended
    there: those three operations' error sets in service-2.json list only NotFoundException/
    TooManyRequestsException, no BadRequestException or ConflictException, so there is no
    wire-verifiable error code to reject with -- guessing one would violate the wire-verification
    principle the same way UpdateRoute/UpdateStage's prior deferral (re-confirmed open, then
    narrowed this pass) originally cited."
  - "ImportApi/ReimportApi's basepath query param now supports \"prepend\" (prefixes route paths
    with the spec's declared base path -- Swagger 2 basePath or OpenAPI 3 servers[0].url's path).
    \"split\" is not implemented (falls back to ignore-like behavior): API Gateway's split
    semantics (part of the base path becomes an ApiMapping key, part stays in routes) aren't
    described by the SDK wire model, only by prose docs, so implementing it would mean guessing
    at unverified behavior. failOnWarnings is now read and validated (boolean) but has no
    observable effect: the emulator's OpenAPI import (parseOpenAPISpec/applyOpenAPIToAPI) never
    generates import warnings for any spec it accepts (see Notes #8), so there is never a warning
    for failOnWarnings to escalate into an error. Not fabricating warning-generation heuristics to
    manufacture an effect -- see the existing trap note on API.ImportInfo/Warnings below. bd:
    gopherstack-jni0, narrowed to these two residual items."
  - "Stage deployment gates only on the stage EXISTING (gopherstack-vli), not on having ever
    been deployed to. Real AWS: 'Deployments are an immutable snapshot of the API, and to make
    your API callable, you must create a stage and deploy an API snapshot into it' (AWS docs,
    apigateway/latest/developerguide/http-api-stages.html -- weaker evidence than the SDK, since
    data-plane invoke behavior isn't part of the modeled control-plane wire shapes). Two distinct
    gaps were disclosed here: (1) a stage that exists but has stage.DeploymentID == \"\" (created,
    never auto- or manually deployed) still serves live traffic -- only a stage that was NEVER
    CREATED is rejected (see Notes #16-18); genuinely gating on DeploymentID=='' risks
    over-rejecting, since it's unclear whether real AWS performs an implicit initial deployment
    when CreateStage's autoDeploy=true and the API already has routes (gopherstack's CreateStage
    does not call autoDeployLocked itself -- only route/integration/API mutations do) -- left
    open, unchanged. (2) FIXED 2026-09-06 (gopherstack-cfr1) for the HTTP API data plane only:
    handleHTTPAPIProxy previously always read the API's LIVE current routes/integrations via
    h.Backend.GetRoutes/GetIntegration regardless of which deployment a stage was nominally
    pinned to, so an autoDeploy=false stage saw route/integration edits with no new deployment
    required. CreateDeployment and autoDeployLocked (deployments.go) now copy the API's current
    routes and integrations onto the created Deployment (Deployment.Routes/Integrations, internal
    only -- json:\"-\", not part of the real GetDeploymentOutput wire shape); handleHTTPAPIProxy
    resolves routes/integrations from the stage's pinned deployment snapshot when
    stage.DeploymentID != \"\", falling back to live state (unchanged behavior) when the stage has
    no deployment yet or its pinned deployment was since deleted -- avoiding the DeploymentID=='' 
    gating question above rather than resolving it. See Notes #19. Residual, disclosed rather than
    guessed at: WebSocket routing (invokeWSRoute) is unaffected -- it doesn't even thread
    stageName through, a separate, larger gap; a route's AuthorizerID is captured by the route
    snapshot, but the referenced Authorizer's own definition (e.g. JWT issuer/audience) is still
    resolved live via h.Backend.GetAuthorizer, since autoDeployLocked has never triggered on
    authorizer or CORS mutations either (a pre-existing, separate incompleteness, not newly
    introduced); CORS headers (Api.CorsConfiguration) are similarly still resolved live, since
    they don't affect route/integration matching. apigateway (v1, bd gopherstack-fum) has the
    identical bug and was deliberately left unfixed this pass -- v1's data plane matches against a
    resource TREE via a cached routingTrie (proxy_routing.go), not v2's flat route/integration
    lists, and also lacks v2's autoDeploy/AutoDeployed model entirely (v1 has no auto-deployment
    concept, only explicit CreateDeployment), so the same fix shape does not carry over; scoped as
    its own, larger effort."
deferred:
  - "2026-08-23 (manifest harvest): UpdatePortal's real UpdatePortalInput (aws-sdk-go-v2/service/apigatewayv2@v1.37.4's api_op_UpdatePortal.go) has optional Authorization/EndpointConfiguration/PortalContent members letting a caller replace a portal's auth config, domain/cert config, or displayed content post-creation -- gopherstack's UpdatePortalInput (models.go) has no fields for any of the three, so a real client sending them gets no error but no effect either. All three are already-modeled types (used by CreatePortal) and Create's existing validateCreatePortal{Authorization,EndpointConfiguration,Content} helpers look reusable for a nil-check-and-replace Update path; not implemented this pass to keep the fix scoped to the three accept-and-drop bugs found and closed alongside this note (IncludedPortalProductArns/RumAppMonitorName/LastPublished(Description), see the family's ops-table note) -- newly disclosed, not previously known."
  - PortalProduct / ProductPage / ProductRestEndpointPage field-level wire audit still not re-verified field-by-field against botocore (only Portal itself got a field-level audit this pass -- see the family's ops-table note)
  - ImportApi/ReimportApi basepath=split; failOnWarnings real effect (see gaps, bd gopherstack-jni0)
  - Quick-create DeleteRoute/DeleteStage/DeleteIntegration rejection (see gaps, bd gopherstack-2tx)
  - DeploymentID=="" gating for a never-deployed stage (see gaps, bd gopherstack-vli) -- per-deployment route/integration snapshotting itself was fixed 2026-09-06 (gopherstack-cfr1, see gaps and Notes #19)
  - apigateway (v1)'s identical live-routing-vs-deployment-snapshot bug (bd gopherstack-fum) -- deliberately not fixed alongside v2's; v1's resource-tree/routingTrie data plane and lack of an autoDeploy model make it a distinctly larger effort, not a copy of this fix
leaks: {status: clean, note: "portalProductSharingPolicies cleanup on DeletePortalProduct already covered by leak_internal_test.go from a prior sweep; authorizerCache entries are now purged on DeleteAuthorizer/DeleteApi (bd gopherstack-wmh, fixed and closed this pass -- see Notes #11), not merely TTL-bounded; no goroutines/janitors in this package"}
---

## Notes

Protocol = REST-JSON (`awsRestjson1` in the SDK's serializers/deserializers, confirmed by
reading `aws-sdk-go-v2/service/apigatewayv2@v1.33.7/serializers.go`). Timestamps use
`__timestampIso8601` (RFC3339 string), not epoch seconds — see `models.go` `isoTime` — this is
correct for apigatewayv2 and should NOT be "fixed" to `awstime.Epoch()`; that would be a
regression (epoch-seconds is a `json-1.0`/query-XML pattern from unrelated services, not REST-JSON
timestampIso8601).

Genuine bugs found and fixed this pass (all confirmed against `aws-sdk-go-v2/service/apigatewayv2@v1.33.7/types/types.go`):

1. **Integration.TlsConfig was entirely absent.** Real `Integration`/`CreateIntegrationInput`/
   `UpdateIntegrationInput` carry a `tlsConfig{serverNameToVerify}` for private (VPC_LINK)
   integrations. Added `IntegrationTLSConfig` and wired it through Create/Update with a
   deep-copy helper (`cloneIntegrationTLSConfig`) so backend state can't alias caller memory.

2. **Integration timeout ceiling/default was NOT protocol-aware.** The backend hardcoded
   29,000ms (`integrationTimeoutMax`) as both the validation ceiling and the zero-value default
   for every API, regardless of protocol. Real AWS: HTTP APIs allow up to 30,000ms (default
   30,000ms), WebSocket APIs allow up to 29,000ms (default 29,000ms) — see the SDK's doc comment
   on `Integration.TimeoutInMillis`. Before this fix, HTTP API integrations with an unset timeout
   were under-defaulted to 29000 instead of 30000, and a valid HTTP API timeout of e.g. 29500ms
   was wrongly rejected as `BadRequestException`. Fixed via `integrationTimeoutMaxFor(protocolType)`
   threaded through `CreateIntegration`/`UpdateIntegration`/`validateTimeoutInMillis`.

3. **Integration.ConnectionType had no default and no validation.** Real AWS defaults
   `connectionType` to `INTERNET` when unset and validates the enum (`INTERNET`|`VPC_LINK`),
   requiring `connectionId` when `VPC_LINK` is specified. Before this fix, `GetIntegration`
   on an integration created without an explicit connectionType returned `""` instead of
   `"INTERNET"`, and a `VPC_LINK` integration with no `connectionId` silently succeeded.

4. **Stage.ClientCertificateID (WebSocket-only) and Stage.Tags were entirely absent.** Real
   `Stage` carries `clientCertificateId` and its own `tags` map (a Stage is independently
   taggable via `arn:aws:apigateway:{region}::/apis/{apiId}/stages/{stageName}`, confirmed by
   the `Tags` field on the SDK's `Stage` type). Added both fields, wired `clientCertificateId`
   through Create/UpdateStage, and extended `TagResource`/`UntagResource`/`GetTags` to resolve
   the nested stage ARN shape (`parseStageARN` + `lookupStageLocked`) since stages — unlike
   APIs/VPC links/domain names — need two path segments (apiId + stageName), not one, to
   resolve. The tag handlers in `handler.go` were also missing an `ErrStageNotFound` → 404
   mapping, so a not-found stage tag lookup would have surfaced as a 500 instead of 404 (the
   exact bug class called out in `parity-principles.md` #2).

5. **DomainName.MutualTlsAuthentication and DomainName.DomainNameArn were entirely absent.**
   Real `DomainName` carries `mutualTlsAuthentication{truststoreUri,truststoreVersion,
   truststoreWarnings}` and `domainNameArn`. Added both, wired through Create/UpdateDomainName
   (`domainNameArn` is computed once at creation and stable across updates, matching AWS
   behavior since it is not settable). `truststoreWarnings` is always empty because the
   emulator has no S3 truststore object to validate — this matches the "no warnings" case for
   a well-formed request; it does not represent unvalidated input.

6. **CreateApi's `routeKey`+`target` "quick create" shortcut was entirely unimplemented** (this
   pass's re-audit, commit range ce30166a..d6fae6df — the ledger's prior gap description
   ("Integration ApiGatewayManaged / Stage ApiGatewayManaged not tracked", bd gopherstack-2tx)
   understated the actual severity: the SDK's `CreateApiInput` carries `RouteKey *string` and
   `Target *string` ("The route target must always be prefixed with `integrations/`..."; "For
   Lambda integrations, specify a function ARN. The type of the integration will be HTTP_PROXY
   or AWS_PROXY, respectively"), but gopherstack's `CreateAPIInput` had no such fields at all —
   `encoding/json` silently dropped them on decode, so a real quick-create request (e.g. `aws
   apigatewayv2 create-api --target ...`, one of the most common ways to stand up an HTTP API)
   succeeded but produced an API with *no* route, integration, or stage whatsoever. Fixed:
   - Added `RouteKey`/`Target` to `CreateAPIInput` (wire keys `routeKey`/`target`, confirmed
     against `serializers.go`).
   - Added `ApiGatewayManaged` → **`APIGatewayManaged`** (Go naming, `revive` var-naming) bool
     field, wire key `apiGatewayManaged`, to `Route`, `Stage`, and `Integration` (confirmed
     present on all four AWS types — `API`, `Integration`, `Route`, `Stage` — in `types.go`;
     `API.ApiGatewayManaged` was deliberately NOT added since nothing in this emulator ever
     marks an *API* itself managed — that flag covers a different mechanism, CloudFormation/SAM
     tooling-created APIs, not quick create).
   - `CreateAPI` now validates routeKey/target (HTTP-only, both-or-neither, valid HTTP route key
     format) and, when both are set, calls new `quickCreateLocked` to auto-provision: an
     integration (`HTTP_PROXY` for a URL target, `AWS_PROXY` for a Lambda ARN target, detected
     by `isLambdaFunctionARN`), a `$default` route targeting `integrations/{id}`, and an
     auto-deployed `$default` stage — all three marked `apiGatewayManaged: true`. Extracted
     `CreateIntegration`'s validation/defaulting body into `buildIntegration` so quick-create
     reuses the exact same AWS-realistic defaults (payload format version, passthrough
     behavior, connection type, protocol-aware timeout ceiling) instead of a second,
     drift-prone copy.
   - `UpdateApi` also carries `RouteKey`/`Target` in the real SDK ("This property is part of
     quick create... you can update a quick-created target, but you can't remove it from an
     API"), also entirely absent from `UpdateAPIInput`. Fixed the same way: each field
     independently updates the API's existing managed route/integration (found via
     `APIGatewayManaged`, since AWS doesn't expose an explicit back-reference and there is at
     most one of each per quick-created API), and returns `ErrBadRequest` if the API has no
     managed route/integration to update (rather than silently no-opping).
   - Added `APIGatewayManaged` to the `stageSnapshot`/`routeSnapshot`/`integrationSnapshot`
     persistence DTOs (`persistence.go`) — additive field, snapshot version not bumped (see the
     version-bump criterion in that file's doc comment: only breaking shape changes need a
     bump).
   - Deliberately NOT implemented this pass (see `gaps`): enforcing that a managed
     route/stage/integration actually rejects mutation/deletion the way real AWS does. The SDK
     doc comments describe the restriction in prose but the exact error code/HTTP status isn't
     derivable from `serializers.go`/`deserializers.go` (it's server-side validation, not part
     of the wire format), and guessing at it would be the same "fabricated behavior" failure
     mode `parity-principles.md` warns against for stubs.

7. **Integration.CredentialsArn was entirely absent.** Real `Integration`/`CreateIntegrationInput`/
   `UpdateIntegrationInput` all carry `credentialsArn` (confirmed at `types.go:614` and in
   `api_op_CreateIntegration.go`/`api_op_UpdateIntegration.go`/`api_op_GetIntegration.go`) --
   the IAM role ARN (or `arn:aws:iam::*:user/*` passthrough sentinel) API Gateway assumes to
   invoke an `AWS`/`AWS_PROXY` integration's backend. `encoding/json` silently dropped it on
   decode; `GetIntegration` always returned `""` regardless of what a caller sent. Added the
   field to `Integration`/`CreateIntegrationInput`/`UpdateIntegrationInput`, wired it through
   `buildIntegration` (so `CreateIntegration` and `CreateApi`'s quick-create path share it) and
   `applyIntegrationUpdate`, and added it to the `integrationSnapshot` persistence DTO.

8. **Api.IpAddressType, Api.ImportInfo, and Api.Warnings were entirely absent.** Real `Api`
   carries `ipAddressType` (`ipv4`|`dualstack`, confirmed in `types.go:104` and
   `CreateApiInput`/`UpdateApiInput`/`ImportApiOutput`/`ReimportApiOutput`), `importInfo`
   (validation feedback from `ImportApi`/`ReimportApi` about ignored OpenAPI properties), and
   `warnings` (warning messages when `failOnWarnings` is set). `ipAddressType` was silently
   dropped on `CreateApi`/`UpdateApi` decode and `GetApi` always returned `""` instead of AWS's
   default (`"ipv4"`). Added all three fields to `API`, `ipAddressType` to
   `CreateAPIInput`/`UpdateAPIInput` with default-to-`ipv4` + enum validation
   (`validateIPAddressType`). `ImportInfo`/`Warnings` are always empty: `API` is a "clean"
   (non-DTO) persisted table so no `persistence.go` change was needed, and the emulator's
   `parseOpenAPISpec`/`applyOpenAPIToAPI` never generates import warnings, which correctly
   represents the "well-formed input" response case (same precedent as `TruststoreWarnings` in
   Notes #5) rather than a stub -- see gaps for the related `basepath`/`failOnWarnings`
   query-param gap this uncovered (gopherstack-jni0).

9. **CreateApiInput's and UpdateApiInput's quick-create-only CredentialsArn were entirely
   absent.** Real `CreateApiInput`/`UpdateApiInput` both carry `credentialsArn` ("part of quick
   create... specifies the credentials required for the integration"), independent of the
   `routeKey`/`target` fields a prior pass already fixed (Notes #6). Added `CredentialsArn` to
   both inputs; `CreateAPI`'s quick-create path (`quickCreateLocked`) now threads it into the
   auto-provisioned integration's `CredentialsArn`, and `UpdateAPI` independently replaces the
   managed integration's credentials via the new `applyQuickCreateCredentialsUpdateLocked`
   (mirroring `applyQuickCreateUpdateLocked`'s existing routeKey/target handling, including the
   same `ErrBadRequest` when the API has no quick-create integration to update).

10. **DomainName.RoutingMode was entirely absent.** Real `DomainName`/`CreateDomainNameInput`/
    `UpdateDomainNameInput` all carry `routingMode` (`API_MAPPING_ONLY`|`ROUTING_RULE_ONLY`|
    `ROUTING_RULE_THEN_API_MAPPING`, confirmed in `types.go:297-304` and the two input structs).
    Added the field with default-to-`API_MAPPING_ONLY` + enum validation
    (`validateRoutingMode`). The `ROUTING_RULE_*` modes only take semantic effect together with
    RoutingRule resources on the domain name, which are explicitly out of this pass's scope
    (RoutingRule's typed-union gap is tracked separately, gopherstack-e81) -- this fix is wire
    completeness (store/return the field correctly) only, not RoutingRule enforcement.

11. **authorizerCache entries were never purged on delete (bd gopherstack-wmh, now fixed and
    closed).** `authorizerCache` (`authorizers.go`) caches REQUEST-authorizer allow/deny
    decisions keyed by `authorizerId + "\n" + identity-source-values`, but neither
    `DeleteAuthorizer` nor `DeleteApi`'s cascade delete purged entries for the deleted
    authorizer(s) -- they only self-healed via TTL expiry or lazy eviction on `get`. Added
    `authorizerCache.purge(authorizerID)` (prefix-matches and deletes every cached entry for
    that authorizer, across all identity-source values) and wired it into
    `handleDeleteAuthorizer` (purge the one authorizer) and `handleDeleteAPI` (snapshot the
    API's authorizer IDs via `GetAuthorizers` before the cascade delete removes them, then purge
    each afterward). This was a leak-adjacent correctness gap, not a wire bug -- a stale cached
    `allow` decision could keep authorizing requests against a route for up to
    `authorizerResultTtlInSeconds` (max 3600s) after the authorizer or its API was deleted.

Genuine bugs found and fixed in the `gopherstack-0xs7` follow-up pass (confirmed against
`aws-sdk-go-v2/service/apigatewayv2@v1.37.4/types/types.go` and
`botocore/data/apigatewayv2/2018-11-29/service-2.json.gz`):

12. **`RoutingRule.Actions`/`Conditions` were untyped `[]map[string]any` instead of AWS's
    modeled union shapes (bd gopherstack-e81, closed).** Sized first per this session's appmesh
    precedent: the real shapes are only 6 small structs at max depth 3 (`RoutingRuleAction` ->
    `RoutingRuleActionInvokeApi{ApiId,Stage,StripBasePath}`; `RoutingRuleCondition` ->
    `RoutingRuleMatchBasePaths{AnyOf}` and/or `RoutingRuleMatchHeaders{AnyOf
    []RoutingRuleMatchHeaderValue{Header,ValueGlob}}`) -- shallow enough to model properly rather
    than leave opaque. Added the 6 types plus `validateRoutingRuleActions`/
    `validateRoutingRuleConditions` (required-subfield checks per `types.go:1280-1353`'s
    `// This member is required` doc comments) and `validateRoutingRulePriority` (the modeled
    `RoutingRulePriority` range, min 1 max 1,000,000, `service-2.json` shape `RoutingRulePriority`
    -- previously unvalidated entirely). Also added `validateRoutingRuleActionTargetsLocked`: each
    action's `InvokeApi.ApiId`/`Stage` must reference an API/stage that actually exists (previously
    any string succeeded, an "operation accepting an ID for a resource that does not exist and
    reporting success" bug). `CreateRoutingRule`/`PutRoutingRule` validate before mutating/writing
    (`PutRoutingRule` previously mutated the existing rule's Priority/Actions/Conditions with zero
    validation). `routingRuleSnapshot`'s persistence DTO field types were updated to match; no
    snapshot version bump (JSON field names unchanged, only the Go type of two existing fields).

13. **Three `Update*` backends mutated fields before validating the whole input, so a rejected
    request could still leave earlier fields in the same call changed.** The session's most
    recurrent bug class. `UpdateRoute` set `r.RouteKey` before validating `AuthorizationType`, so
    e.g. `{routeKey: "POST /x", authorizationType: "BOGUS"}` returned `BadRequestException` but
    left the route key changed. `UpdateAPI` mutated `Name`/`Description`/etc. before validating
    `IPAddressType` and the quick-create `routeKey`/`target`/`credentialsArn` fields (which
    themselves validate against the API's existing managed route/integration). `UpdateDomainName`
    mutated `Tags`/`DomainNameConfigurations`/`MutualTLSAuthentication` before validating
    `RoutingMode`. Fixed by splitting each into a pure-validation pass (no mutation) that runs
    first, then a mutation pass that runs only once every field validates --
    `validateRouteKeyUpdate`/`validateRouteAuthUpdate`/`applyRouteUpdate` (routes.go),
    `validateQuickCreateUpdateLocked`/`applyQuickCreateUpdateMutateLocked` (apis.go, replacing the
    old `applyQuickCreateUpdateLocked`/`applyQuickCreateCredentialsUpdateLocked` which validated
    and mutated in the same pass), and reordering `UpdateDomainName`'s `RoutingMode` check ahead of
    its other field mutations.

14. **Quick-create managed-resource immutability, narrowed (bd gopherstack-2tx).** Real AWS:
    "You can't modify the $default route key" (`Route.ApiGatewayManaged` doc) and "You can't
    modify the $default stage" (`Stage.ApiGatewayManaged` doc). `UpdateRoute` now rejects a
    route-key change when `r.APIGatewayManaged` (other fields on a managed route remain
    updatable, matching the doc's route-*key*-specific wording); `UpdateStage` now rejects any
    modification of a managed stage (matching the doc's unqualified "can't modify"). Both return
    `BadRequestException`, which is in `UpdateRoute`/`UpdateStage`'s modeled error set
    (`service-2.json`). `DeleteRoute`/`DeleteStage`/`DeleteIntegration` remain unenforced: their
    modeled error sets contain only `NotFoundException`/`TooManyRequestsException`, no error code
    that fits "rejected because managed" -- guessing one would be the same fabrication risk this
    gap's original deferral (2026-07-05) correctly flagged.

15. **Route reachability (bd gopherstack-l5ir).** Every one of the 103 real apigatewayv2 ops was
    extracted from `apigatewayv2@v1.37.4` serializers.go (`request.Method` +
    `httpbinding.SplitURI(...)` in each op's `awsRestjson1_serializeOp<Op>.HandleSerialize`) and
    diffed against this service's route table. Zero mismatches -- all 103 method+path pairs
    resolve to the correct op via `ExtractOperation`, including the shared-path/method-only
    disambiguation used by `GetTags`/`TagResource`/`UntagResource` (all `/v2/tags/{ResourceArn}`)
    and `PublishPortal`/`DisablePortal` (both `/v2/portals/{id}/publish`, POST vs DELETE) -- unlike
    cloudfront's `TagResource`/`UntagResource` bug (both `POST /tagging` distinguished only by an
    `Operation=` query param the router ignored), apigatewayv2's tag ops are genuinely
    method-disambiguated in the real SDK, so switching on method here is correct, not a latent bug.
    No op in this service is distinguished by a query parameter or bare flag. Added as a permanent
    test, `TestExtractOperation_SDKRouteTable` in `handler_paths_sdk_diff_test.go` (one subtest per
    op), rather than left as a one-off audit.

Genuine bugs found and fixed in the data-plane sweep (`gopherstack-vli`, this pass, 2026-09-04;
confirmed against `aws-sdk-go-v2/service/apigatewayv2@v1.37.4/api_op_CreateIntegration.go`):

16. **`buildIntegration`/`UpdateIntegration` accepted `AWS`/`HTTP`/`MOCK` integrationType on
    HTTP-protocol APIs with no protocol check at all.** `api_op_CreateIntegration.go`'s
    `IntegrationType` doc comment states, verbatim, for each of the three: `AWS: ... Supported
    only for WebSocket APIs.`; `HTTP: ... Supported only for WebSocket APIs.`; `MOCK: ...
    Supported only for WebSocket APIs.` -- only `AWS_PROXY`/`HTTP_PROXY` are valid on HTTP APIs.
    `buildIntegration`'s `validTypes` map accepted all five regardless of the API's
    `protocolType`, and `applyIntegrationIdentityUpdate` set `i.IntegrationType` from
    `UpdateIntegrationInput` with the same no-check pattern. Concretely: `CreateIntegration`
    with `integrationType: MOCK` on an HTTP API previously succeeded (201), and only failed
    opaquely (500 `Unsupported integration type`) the first time a client actually invoked the
    route -- an "accepted but never done" bug (a request that should have been rejected at
    Create/Update time with `BadRequestException` instead silently created broken state).
    Fixed via `validateIntegrationTypeForProtocol` (`integrations.go`), called from both
    `buildIntegration` (covers `CreateIntegration` and quick-create, which never generates these
    three types itself so is unaffected) and `UpdateIntegration`. Test:
    `TestHandler_CreateIntegration_InvalidType` (`handler_integrations_test.go`) now covers both
    directions (HTTP API rejects AWS/HTTP/MOCK; WebSocket API accepts HTTP/MOCK).

17. **`invokeWSRoute` (the WebSocket data plane) rejected `MOCK` integrations with
    `ErrUnsupportedType`, even though MOCK is a real, valid WebSocket integration type.**
    `api_op_CreateIntegration.go`: `MOCK: for integrating the route or method request with API
    Gateway as a "loopback" endpoint without invoking any backend. Supported only for WebSocket
    APIs.` A `$connect` route (or any other) backed by a MOCK integration therefore always failed
    -- for `$connect` specifically, `handleWebSocketProxy` maps any `invokeWSRoute` error to a
    403 Forbidden, so a client's WebSocket handshake was rejected outright for a configuration
    real AWS explicitly supports and documents as a no-op success. Fixed: `invokeWSRoute` now
    short-circuits `MOCK` to `nil` (success, no backend invocation) before the AWS_PROXY check,
    implementing exactly what the doc comment describes -- not a fabricated response shape, since
    "no backend invoked" has no payload to construct. `AWS`/`HTTP` (the two remaining WebSocket-
    only custom-integration types, which require Velocity-Template-style request/response mapping
    the emulator has no engine for) remain `ErrUnsupportedType` -- left as a disclosed, structural
    gap rather than a guessed template-execution implementation. Test:
    `TestInvokeWSRoute_MockIntegrationIsLoopback` (`proxy_internal_test.go`, white-box: calls
    `invokeWSRoute` directly since WebSocket upgrade isn't needed to exercise this code path).

18. **`handleProxy` never checked the URL's stage-name segment against the backend at all.**
    Both data-plane entry points (`/v2proxy/{apiId}/{stageName}/...` and
    `/restapis/{apiId}/{stageName}/_user_request_/...`) parse a `stageName` out of the URL and
    pass it to `handleProxy`, which used it only to best-effort look up stage variables
    (`handleHTTPAPIProxy`'s `if stage, err := h.Backend.GetStage(...); err == nil` -- errors
    silently ignored) and, for WebSocket APIs, didn't even pass `stageName` to
    `handleWebSocketProxy` at all. This meant ANY string in the URL's stage-name slot --
    including a stage name that was never created via `CreateStage` -- still routed a request
    through to the API's live route/integration and executed it. Confirmed AWS requires a
    deployed stage to invoke an API ("Deployments are an immutable snapshot of the API, and to
    make your API callable, you must create a stage and deploy an API snapshot into it" -- AWS
    public documentation, weaker evidence than the pinned SDK since data-plane invoke behavior
    isn't part of apigatewayv2's modeled control-plane wire shapes). Fixed: `handleProxy` now
    calls `h.Backend.GetStage(apiID, stageName)` before the protocol switch and returns 404
    (`{"message":"Not Found"}` via the existing `writeErr` helper, matching the JSON shape real
    HTTP APIs return for an unmatched route) if the stage doesn't exist, for both HTTP and
    WebSocket protocols in one chokepoint. This closes the "nonexistent stage still serves
    traffic" case but deliberately does NOT gate on the stage having ever been deployed
    (`DeploymentID != ""`) or on serving a deployment-time snapshot rather than live state -- see
    gaps for why those two are disclosed but left open. Test:
    `TestHTTPAPIProxy_NonexistentStage_NotFound` (`http_proxy_test.go`). This fix also required
    updating the shared `doProxyRequest` test helper to provision a `$default` stage before
    issuing a data-plane request (`ensureDefaultStage`) -- every existing proxy test had been
    unknowingly relying on the absence of this check, since none of them called `CreateStage`.

19. **`handleHTTPAPIProxy` never froze a per-deployment routing snapshot -- an `autoDeploy=false`
    stage saw route/integration edits with no new deployment (gopherstack-cfr1).** `CreateDeployment`
    and `autoDeployLocked` (`deployments.go`) created a `Deployment` record with no capture of the
    API's routes/integrations at all, and `handleHTTPAPIProxy` (`http_proxy.go:124,146` pre-fix)
    called `h.Backend.GetRoutes(apiID)`/`h.Backend.GetIntegration(apiID, integrationID)` on every
    request regardless of which deployment a stage was nominally pinned to -- an `autoDeploy=true`
    and an `autoDeploy=false` stage were indistinguishable at the data plane; both always served
    the API's live current state. Fixed: `Deployment` gained internal-only `Routes []Route` /
    `Integrations []Integration` fields (`json:"-"` -- not part of the real `GetDeploymentOutput`
    wire shape, verified against `aws-sdk-go-v2/service/apigatewayv2`'s `api_op_GetDeployment.go`).
    `CreateDeployment` and `autoDeployLocked` now both call a shared `snapshotRoutingLocked`
    helper to copy the API's current routes/integrations onto the deployment they create.
    `handleHTTPAPIProxy` resolves the routes to match against, and the integration a matched
    route's target names, via new `resolveHTTPAPIRoutes`/`resolveHTTPAPIIntegration` helpers: when
    `stage.DeploymentID != ""` and that deployment still exists, both resolve from its frozen
    snapshot; otherwise (no deployment yet, or the pinned deployment was since deleted) both fall
    back to the API's live state, unchanged from pre-fix behavior -- deliberately sidestepping the
    `DeploymentID==""` gating question left open in gaps (#2) rather than resolving it. Persisted
    via `deploymentSnapshot`'s new `Routes []routeSnapshot`/`Integrations []integrationSnapshot`
    fields (`persistence.go`), reusing the existing `routeSnapshot`/`integrationSnapshot` DTOs so
    the nested entries round-trip identically to the top-level route/integration tables; additive
    fields, no `apigatewayv2SnapshotVersion` bump. Scope, disclosed not guessed at: a route's
    `AuthorizerID` is captured by the route snapshot, but the *referenced* `Authorizer`'s own
    definition is still resolved live via `h.Backend.GetAuthorizer` -- `autoDeployLocked` has
    never triggered on authorizer or CORS mutations either, a pre-existing incompleteness this fix
    didn't introduce or widen. WebSocket routing (`invokeWSRoute`) is untouched -- it doesn't even
    take a `stageName` parameter, a separate, larger gap. apigateway v1's identical bug
    (`bd gopherstack-fum`) was left unfixed: v1 has no `autoDeploy` model, and its data plane
    matches a resource TREE via a cached `routingTrie` (`proxy_routing.go`), not v2's flat
    route/integration lists -- the same fix shape does not transfer. Tests:
    `TestHTTPAPIProxy_DeploymentSnapshot` (table over `autoDeploy` true/false, asserting through
    the proxy path -- not the stored record -- that an `autoDeploy=false` stage keeps serving a
    stale integration after an edit until a fresh `CreateDeployment`, while `autoDeploy=true`
    reflects the edit immediately) and `TestHTTPAPIProxy_NoDeploymentYet_ServesLiveState` (a stage
    that exists but was never deployed still serves live state, not a 500). The pre-existing
    `TestAutoDeploy_RouteAndIntegrationChangesDeploy` (`handler_deployments_test.go`) was left
    unchanged rather than extended: it already covered the deployment *record* correctly
    (`autoDeployLocked` firing per stage), a genuinely separate concern from the proxy *routing*
    bug this note fixes.

Traps for the next auditor (don't re-flag):

- Every data-plane proxy test now depends on `doProxyRequest` (`http_proxy_test.go`) calling
  `ensureDefaultStage` first, which POSTs `/v2/apis/{apiId}/stages` and tolerates a 409 (idempotent
  across repeat calls with the same apiID in one test). Don't "clean this up" by removing it or
  by making `createAPI`/`buildHTTPAPIWithLambda` auto-create a stage instead -- `createAPI` is
  shared by ~50+ control-plane tests across the package that don't want a stage as a side effect,
  and quick-create APIs (`CreateApi` with `routeKey`+`target`) already get their own `$default`
  stage from `quickCreateLocked`, so `ensureDefaultStage`'s 409-tolerance is load-bearing there.
- Per-integration-type data-plane verdict (this pass, gopherstack-vli): HTTP API AWS_PROXY
  (`invokeHTTPAPILambda`) and HTTP_PROXY/HTTP (`forwardHTTPAPIHTTPIntegration`) both genuinely
  execute. HTTP API MOCK/AWS/HTTP are now correctly NOT ACCEPTED at CreateIntegration (Notes #16;
  previously accepted-but-never-done). WebSocket AWS_PROXY genuinely executes (`invokeWSRoute`).
  WebSocket MOCK is now a genuine no-op success/loopback (Notes #17). WebSocket AWS/HTTP (custom,
  template-mapped integrations) remain ACCEPTED BUT NEVER EXECUTE -- `invokeWSRoute` still returns
  `ErrUnsupportedType` for them; implementing real VTL-style template execution is a structural
  gap, not fixed this pass (no test asserts otherwise; don't assume these work).
- `arnResourceType` (single `type/id` suffix) intentionally does NOT handle Stage ARNs — Stage
  tagging goes through the separate `parseStageARN` (4-segment `apis/{id}/stages/{name}`) checked
  *before* falling through to `arnResourceType` in `TagResource`/`UntagResource`/`GetTags`. This
  is correct, not a missed generalization — Stages are the only nested (parent + child) taggable
  resource in this API.
- The hand-formatted `"arn:aws:apigateway:" + region + "::/..."` ARN construction (not
  `pkgs/arn`) is a pre-existing convention in this file (see `RoutingRuleARN`, now also
  `DomainNameArn`); left as-is for consistency rather than partially migrating one call site.
- `Portal`/`PortalProduct` preview-feature code was spot-checked in the parity-3 pass and
  confirmed fully operation-complete (26/26 ops, see `families`) in the `gopherstack-0xs7`
  follow-up; field-level wire-shape depth still not audited — don't assume "26/26 present" means
  "every field on those 26 is correct."
- `RoutingRule` `Actions`/`Conditions` are typed as of `gopherstack-0xs7` (Notes #12) — do not
  revert to `[]map[string]any` "for round-trip fidelity." That reasoning was evaluated and
  superseded: `RoutingRuleAction`/`RoutingRuleActionInvokeApi`/`RoutingRuleCondition`/
  `RoutingRuleMatchBasePaths`/`RoutingRuleMatchHeaders`/`RoutingRuleMatchHeaderValue` are only
  6 small structs at a max depth of 3 (confirmed by reading `types.go:1259-1353`,
  `aws-sdk-go-v2/service/apigatewayv2@v1.37.4`) — well inside "shallow enough to model properly,"
  not the "genuinely deep nested union" case where opaque passthrough is the right call.
- `quickCreateLocked`'s auto-created `$default` stage name reuses the existing `routeKeyDefault`
  constant (`proxy.go`) for its literal value rather than a new stage-specific constant — same
  string (`"$default"`), and introducing a second constant with the identical value would have
  tripped `goconst`. The name is a slight misnomer when read at the stage call site; this is
  intentional, not an oversight.
- `API.ImportInfo`/`API.Warnings` (Notes #8) always marshal as omitted/empty. This is NOT an
  unfinished stub to "complete" by inventing warning-generation heuristics — real AWS only
  populates them when its (unspecified, business-logic) OpenAPI validation actually finds
  something to flag, and the emulator's `parseOpenAPISpec` tolerates any input it's given, so
  "nothing to flag" is the correct, non-fabricated response for every import this emulator can
  currently perform. Do not add speculative warning text.
- If a future auditor's ledger baseline (`last_audit_commit`) is not an ancestor of the current
  branch, don't assume it was merely rebased/squashed — check whether the commit even touches
  `services/apigatewayv2/` at all (`git show --stat <hash>`). This pass's recorded baseline
  (`d6fae6df`) belonged entirely to the sibling `services/apigateway` (v1 REST API) service; the
  real baseline was recovered via `git log -- services/apigatewayv2/PARITY.md`.

## 2026-08-29 cursor-pagination audit (declares-but-never-sets class)

Enumerated every response struct declaring `NextToken` (17 total, in `models.go`) against
this package's two shared pagination mechanisms: `handleGetList` (generic helper,
`handler.go`) and direct `page.New(...)` calls (`pkgs/page`, the repo's shared opaque-cursor
paginator). 12 of 17 were already correctly wired through one of the two. 5 were not:
`GetDomainNames`, `GetApiMappings`, `GetIntegrationResponses`, `GetRouteResponses`,
`GetVpcLinks` -- all real, genuinely-paginated ops (`apigatewayv2@v1.37.4`: each declares
`MaxResults *string`/`NextToken *string` on input and `NextToken *string` on output) whose
handlers called the backend and returned the full, unbounded result with no pagination logic
at all -- not even a broken attempt, just absent. `GetIntegrationResponses`/
`GetRouteResponses` share a generic `nestedResponseOps[T,U]` helper (two-levels-nested
"response" resources under an integration/route); its `wrapList` closures took only the item
slice, with no way to carry a cursor, so `handleGetChildList` (its backing implementation)
never had a token to set. Widened `wrapList`'s signature to `func([]T, string) any` and moved
the `apigwPaginationParams`/`page.New` call into `handleGetChildList` itself, matching
`handleGetList`'s existing shape -- one fix covers both ops.

Every one of these 5 also had the request-side `MaxResults`/`NextToken` completely unread
(no query-string parsing at all before this fix), the same broken-both-sides pattern the
brief predicted.

No provably-bounded gaps found in this service -- every declared cursor corresponds to a
genuinely user-growable collection (domain names, API mappings, integration/route responses,
VPC links all accumulate via Create* calls with no compile-time cap).

Tests: new `services/apigatewayv2/pagination_cursor_test.go`
(`TestGetDomainNames_Limit`, `TestGetApiMappings_Limit`, `TestGetIntegrationResponses_Limit`,
`TestGetRouteResponses_Limit`, `TestGetVpcLinks_Limit`), all driving the real
`aws-sdk-go-v2/service/apigatewayv2` client via the existing `newTestAPIGatewayV2Client`
helper, all confirmed failing against unmodified code before the fix.

Gates: `go build ./...`, `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/apigatewayv2/...` (pass), `golangci-lint run ./services/apigatewayv2/...`
(0 issues after `gofmt -w` on `handler.go`).

## Notes (2026-08-30 pass — pagination map-order audit)

Audited every `pkgs/page.New`/`NewHMAC` call site in this service (11 literal call
sites, covering 17 list operations via `handleGetList`/`handleGetChildList`/
`nestedResponseOps`) for the class of bug confirmed in `services/opsworks`: a
paginator consuming `Table.All()`/`Table.Range()` (an unspecified-order Go map
walk, per `pkgs/store.Table.All`'s doc comment) with no total sort, so a
cursor-token round-trip drops/duplicates records.

Verdict: 0 bugs. Every call site is safe by construction, by one of two
mechanisms:
- filtered to a single parent via a `pkgs/store.Index.Get` lookup (stable,
  insertion-derived order, not a map walk) -- `ListRoutingRules`,
  `GetApiMappings`, `GetModels`, `GetDeployments`, `GetIntegrations`,
  `GetRoutes`, `GetStages`, `GetAuthorizers`, `GetIntegrationResponses`,
  `GetRouteResponses`, `ListProductPages`, `ListProductRestEndpointPages`; and
- `Table.All()` re-sorted by the table's own primary key (`sort.Slice` on the
  same field the table's `keyFn` returns), which is definitionally unique --
  `GetDomainNames` (sorted by `DomainNameValue`, the `domainNames` table key),
  `GetVpcLinks` (`VpcLinkID`), `GetAPIs` (`APIID`), `ListPortals` (`PortalID`),
  `ListPortalProducts` (`PortalProductID`).

Empirically proved the riskiest case (`GetDomainNames`, `Table.All()` + sort)
with a new full-walk test rather than trusting the reasoning alone: added
`pagination_full_walk_test.go`'s `TestGetDomainNames_FullWalk_NoDropsOrDuplicates`,
which seeds 25 domain names via the real `aws-sdk-go-v2` client, walks
`GetDomainNames` to completion at `MaxResults=5`, and asserts the union of
every page is exactly the seed set with no drop or duplicate. Passed 10/10
runs under `-race -count=10`. Existing `pagination_cursor_test.go` tests
(`TestGetDomainNames_Limit` etc.) only ever fetch one page and assert
`len==1`/`NextToken != ""` -- structurally unable to see a map-order
drop/duplicate, since that only manifests across a second `GetDomainNames`
call re-walking the same (re-randomized) map iteration.

No sort found non-total on a call site sourced from a map walk (the actual bug
condition); no filter-after-pagination; no MaxResults/NextToken-accepting op
found that silently returns everything untruncated. `PARITY.md` claims not
re-verified beyond what this pass touched. Gates on `./services/apigatewayv2/...`:
`go build`, `go vet`, `go test -race -count=1` (all pass, existing suite
unmodified/ungrown-except-the-1-new-file), `golangci-lint run` (0 issues).

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

`cmd/reqfielddiff`/`cmd/reqfieldscan` used to break ties among
case-insensitive handler-name matches by whichever Go's randomized map
iteration visited first (ef0eef041 fixed it repo-wide; appsync, e2643a6dd,
was the first measured victim). This package's REST-path dispatch (no
`service.JSONOpFunc` table at all) means `reqfielddiff` relies entirely on
name-convention resolution here, and its `Api`/`API` acronym casing gives it
39 op/handler pairs that need the ambiguous fold, 10 of them genuine
collisions between an exported `*InMemoryBackend` method and the real
unexported handler: `CreateApi`, `DeleteApi`, `DeleteApiMapping`,
`ExportApi`, `GetApi`, `GetApiMapping`, `GetApiMappings`, `GetApis`,
`UpdateApi`, `UpdateApiMapping`.

Verified the damage directly: ran the unpatched tool from `ef0eef041~1` five
times and diffed against the fixed tool at HEAD. `cmd/reqfieldscan` was
byte-identical across all 5 runs and HEAD -- zero damage (this service's
dispatch table has no `WrapOp` entries for `reqfieldscan` to resolve
ambiguously at all). `cmd/reqfielddiff` was not: findings ranged 245-253
across the 5 old runs (5 distinct counts) vs 238 at HEAD, with 31 op.field
keys flickering.

29 of the 31 were the safe direction -- present in some old (misresolved)
run, never at HEAD: `CreateApi.{ApiKeySelectionExpression, CorsConfiguration,
Description, DisableExecuteApiEndpoint, DisableSchemaValidation,
IpAddressType, Name, ProtocolType, RouteSelectionExpression, Tags,
Version}`, `ExportApi.{IncludeExtensions, OutputType}`, `GetApi.ApiId`,
`GetApiMapping.ApiMappingId`, most of `UpdateApi`'s flickering fields, and
`UpdateApiMapping.{ApiId, ApiMappingId, ApiMappingKey, Stage}`. Read the
source for every one of these (apis.go:13-90's `CreateAPI`/`UpdateAPI`,
handler_apis.go's `ExportApi`/`GetApi` query- and path-param handling,
handler_api_mappings.go's `GetAPIMapping`/`UpdateAPIMapping`): all genuinely
declared and threaded to the backend. The tool's own "declared" signal for
`CreateApi`/`UpdateApi` is itself an artifact of `matchReturnsStructCall`
picking up the `*API` domain struct `h.Backend.CreateAPI`/`.UpdateAPI`
returns (which happens to mirror most Input field names) rather than genuine
recognition of the `json.NewDecoder(...).Decode(&input)` call this tool's
`decodeCallVerbs` list doesn't match at all -- but the underlying claim
(field genuinely handled) checks out by direct source read regardless.

2 of the 31 went the other way -- present at HEAD, absent from some old
(misresolved) runs, the direction that would hide a real bug:
`UpdateApi.RouteKey`, `UpdateApi.Target`. Investigated specifically for that
reason. Confirmed genuine and fully applied: `UpdateAPIInput.RouteKey`/
`.Target` (models.go:250-256, wire keys `routeKey`/`target`, matching
apigatewayv2@v1.37.4 api_op_UpdateApi.go:80,93's "part of quick create"
fields) are validated and applied by
`validateQuickCreateUpdateLocked`/`applyQuickCreateUpdateMutateLocked`
(apis.go:363-431). Not a bug -- the same tool artifact in reverse (the
exported `UpdateAPI`'s return-type match doesn't happen to carry these two
quick-create-only field names, since they're not mirrored onto the `API`
struct itself).

Verdict: zero real bugs. Every moved finding traces to either the
determinism fix (safe direction, now resolved) or a separate, pre-existing
`reqfielddiff` blind spot (`Decode` not in `decodeCallVerbs`) that reading
the actual source -- rather than trusting either tool -- neutralizes.

## 2026-09-08: route-throttle/authorization nil-on-write fall-through fix (gopherstack-wsvb, P1) -- found and fixed

Part of the sweep following the elasticache fix (gopherstack-8haq) and the pinpoint audit
(gopherstack-246v). `writeErr`/`writeErrType` (errors.go) write the JSON error body via
`c.JSON` and return its result, which is nil after a successful write. `enforceRouteThrottle`
and `enforceRouteAuth` (http_proxy.go) rejected a request via `return writeErr(...)`, so
`applyRouteControls`' and `handleHTTPAPIProxy`'s `if ctrlErr != nil` checks never fired and a
throttled or unauthorized request was forwarded to the real integration anyway, even though
the client had already received the 429/401/403. The same shape was independently present in
`enforceIAMAuth`, `enforceRequestAuthorizer`, and `finishAuthDecision` (authorizers.go), all
reached from `enforceRouteAuth`'s AWS_IAM/CUSTOM branches -- same call chain, same bug.

**Tests first.** Following gopherstack-246v's lesson that a status-only assertion passes
against this bug (echo's `Response.WriteHeader` is a no-op after the first call, so the
committed rejection status stays on the wire even when the integration is invoked
underneath it -- only the body gets corrupted, or in this package's case, since the mock
integration Lambda usually still returns a clean `{"statusCode":200,...}` envelope that
`writeHTTPAPILambdaResponse` writes into the already-committed response, the body). Every
test below therefore asserts a mock Lambda invocation counter, not just the response status.
Confirmed each FAILS against unmodified code before fixing (verbatim, `go test -race
./services/apigatewayv2/...`):

```
=== NAME  TestHTTPAPIProxy_RouteThrottle_TooManyRequests
    http_proxy_throttle_test.go:64:
        Error Trace:    /home/agbishop/gopherstack/services/apigatewayv2/http_proxy_throttle_test.go:64
        Error:          Not equal:
                        expected: 1
                        actual  : 2
        Test:           TestHTTPAPIProxy_RouteThrottle_TooManyRequests
        Messages:       a throttled request must not reach the integration (gopherstack-wsvb)
--- FAIL: TestHTTPAPIProxy_RouteThrottle_TooManyRequests (0.00s)

=== NAME  TestHTTPAPIProxy_JWTAuthorizerMissing_DoesNotInvokeIntegration
    http_proxy_test.go:574:
        Error Trace:    /home/agbishop/gopherstack/services/apigatewayv2/http_proxy_test.go:574
        Error:          Not equal:
                        expected: 0
                        actual  : 1
        Test:           TestHTTPAPIProxy_JWTAuthorizerMissing_DoesNotInvokeIntegration
        Messages:       a JWT route with an unresolvable authorizerId must not reach the integration (gopherstack-wsvb)
--- FAIL: TestHTTPAPIProxy_JWTAuthorizerMissing_DoesNotInvokeIntegration (0.00s)

=== NAME  TestHTTPAPIProxy_JWTAuthorizer
    http_proxy_test.go:513: Error Trace ... expected: 0, actual: 1, Messages: a missing JWT must not reach the integration
    http_proxy_test.go:519: Error Trace ... expected: 0, actual: 2, Messages: an invalid JWT must not reach the integration
--- FAIL: TestHTTPAPIProxy_JWTAuthorizer (0.00s)

=== NAME  TestRouteAuth_AWSIAM/unsigned_request_rejected
    handler_authorizers_test.go:653: Error Trace ... expected: 0, actual: 1
--- FAIL: TestRouteAuth_AWSIAM (0.00s)
    --- FAIL: TestRouteAuth_AWSIAM/unsigned_request_rejected (0.00s)
    --- PASS: TestRouteAuth_AWSIAM/sigv4_signed_allowed (0.00s)
    --- PASS: TestRouteAuth_AWSIAM/presigned_query_allowed (0.00s)
```

The CUSTOM/REQUEST-authorizer paths (`enforceRequestAuthorizer`/`finishAuthDecision`,
authorizers.go), reached from the same `enforceRouteAuth` call chain, share the identical
shape and were also caught pre-fix, each an existing test strengthened from a status-only
assertion to also assert non-invocation (see "modified pre-existing tests" below):

```
--- FAIL: TestRequestAuthorizer_SimpleResponse (0.00s)
    --- PASS: TestRequestAuthorizer_SimpleResponse/simple_allow (0.00s)
    --- FAIL: TestRequestAuthorizer_SimpleResponse/simple_deny (0.00s)
        Error: Not equal: expected: 0, actual: 1

--- FAIL: TestRequestAuthorizer_IAMPolicyResponse (0.00s)
    --- PASS: TestRequestAuthorizer_IAMPolicyResponse/policy_allow_wildcard (0.00s)
    --- FAIL: TestRequestAuthorizer_IAMPolicyResponse/policy_explicit_deny (0.00s)
        Error: Not equal: expected: 0, actual: 1
    --- FAIL: TestRequestAuthorizer_IAMPolicyResponse/policy_no_matching_resource_implicit_deny (0.00s)
        Error: Not equal: expected: 0, actual: 1

--- FAIL: TestRequestAuthorizer_MissingIdentitySource (0.00s)
        Error: Not equal: expected: 0, actual: 1

--- FAIL: TestRequestAuthorizer_MissingAuthorizerRejected (0.00s)
        Error: Not equal: expected: 0, actual: 1
```

Echo's own instrumentation independently corroborates the double write on several of these
runs, logged alongside the failures above: `{"level":"ERROR","msg":"echo: response already
written to client"}`.

**Fix.** Following the pinpoint pattern (handler_templates.go's `applyTemplateUpdate`), not
elasticache's `errResponseWritten` sentinel-plus-inline-write shape -- the fan-out here (two
`http_proxy.go` enforce helpers plus three `authorizers.go` helpers, all funneling into one
`applyRouteControls` and one `handleHTTPAPIProxy`) doesn't warrant a sentinel. Every helper in
the chain (`enforceRouteThrottle`, `enforceRouteAuth`, `enforceIAMAuth`,
`enforceRequestAuthorizer`, `finishAuthDecision`) now returns a raw, unwritten error: either
the existing `ErrThrottled`, or one of five new package-private sentinels in errors.go
(`errRouteUnauthorized`, `errRouteForbidden`, `errRouteExplicitDeny`,
`errRouteMissingAuthToken`, `errRouteAuthConfigInvalid`). A single new
`writeRouteControlRejection` (errors.go) maps each to its AWS-accurate status/body and writes
it exactly once, called only at `handleHTTPAPIProxy`'s `applyRouteControls` check.
`enforceRouteThrottle`'s `default:` branch (an unexpected backend enforcement error) still
returns nil and allows the request through -- that fail-open behavior is preserved unchanged;
it's a separate, deliberate design decision from this bug, not part of it.

**Modified pre-existing tests** (all strengthened from a status-only assertion to also assert
the integration was not invoked -- exactly the class of assertion that let this bug hide):
`TestHTTPAPIProxy_RouteThrottle_TooManyRequests`, `TestHTTPAPIProxy_JWTAuthorizer`,
`TestRouteAuth_AWSIAM`, `TestRequestAuthorizer_SimpleResponse`,
`TestRequestAuthorizer_IAMPolicyResponse`, `TestRequestAuthorizer_MissingIdentitySource`,
`TestRequestAuthorizer_MissingAuthorizerRejected`. `setupRequestAuthAPI`
(handler_authorizers_test.go) gained a second atomic counter, `integrationCalls`, alongside
its existing `authCalls`, distinguishing the two Lambdas the same way the existing mock
already did by ARN substring (`auth-fn` vs the route's own integration function) -- it just
wasn't being counted before. `TestRequestAuthorizer_CachingTTL` (already passing pre-fix, not
a bug site) also gained an `integrationCalls` assertion for completeness since the counter
was threaded through its call site anyway.

**New test**: `TestHTTPAPIProxy_JWTAuthorizerMissing_DoesNotInvokeIntegration`
(http_proxy_test.go) -- a JWT route whose `authorizerId` doesn't resolve to a stored
authorizer, the JWT-authorizer counterpart of `TestRequestAuthorizer_MissingAuthorizerRejected`
(CUSTOM).

**Scope check**: grepped every `writeErr`/`writeErrType`/`c.JSON`/`c.String` call site in the
package (~180) against every checked `if xErr := h.foo(...); xErr != nil` call site (~10).
Outside the enforcement chain above, every other writer call is either the final `return` of
its own echo-registered handler (never re-checked by another function in this package) or an
already-safe direct passthrough (`return writeErr(...)`/`return c.JSON(...)`, not stored and
rechecked). No other instance of this shape exists in `services/apigatewayv2/`.

**Verification**: `golangci-lint run ./services/apigatewayv2/...` → 0 issues.
`go test -race ./services/apigatewayv2/...` → ok. Full `go test ./services/...` → ok, 169
packages, zero failures (`services/stepfunctions`, owned by another concurrent change, also
passed unaffected).
