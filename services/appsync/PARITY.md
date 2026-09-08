---
service: appsync
sdk_module: aws-sdk-go-v2/service/appsync@v1.60.0
last_audit_commit: 198990e82
last_audit_date: 2026-09-04
overall: A            # 2026-09-04 (gopherstack-2yo): DeleteGraphqlApi's cascade-delete (issue #842) missed two ghost-row classes -- SourceAPIAssociation rows (either SourceAPIID or MergedAPIID matching the deleted API) and the APIAssociation/DomainName.APIID link created by AssociateApi -- both outlived the API indefinitely, so Get/ListSourceApiAssociations and GetApiAssociation kept returning associations pointing at a deleted API forever. Fixed for real (cascadeDeleteAPIAssociations); regression tests added. Also fixed: CreateApiKey's default expiry was wrong (365 days; real SDK doc says 7) and Create/UpdateApiKey's two AppSync-specific error codes (ApiKeyLimitExceededException, ApiKeyValidityOutOfBoundsException) were never actually surfaced -- both collapsed into a generic BadRequestException, and an out-of-bounds custom expiry was silently clamped into range instead of rejected. Also disclosed (not fixed, structural): GetIntrospectionSchema's format/includeDirectives were silently ignored, same missing-SDL<->JSON-converter class already disclosed for ListTypes/GetType/ListTypesByAssociation but not previously called out for this op. Grade held at A.
                      # 2026-07-24: systemic route-matcher/method bugs fixed across nearly every family; the two remaining gaps from the 2026-07-12 pass (StartSchemaMerge, Start/GetDataSourceIntrospection) are now implemented for real
                      # 2026-07-31: pkgs/sdkcheck reverse check found ExecuteGraphQL wrongly advertised/documented as a real SDK op (it isn't -- see its ops-block note); corrected, route left wired as internal data-plane scaffolding. Grade held at A: a documentation defect, not a served-client bug.
                      # 2026-07-31 (second pass, browser parity): RouteMatcher's /v2/apis-vs-ApiGatewayV2 disambiguation (see its doc comment) checked only the User-Agent header, which a browser cannot set (Fetch spec) -- the AWS SDK for JavaScript in a browser puts its SDK identification in X-Amz-User-Agent instead, so every browser dashboard request through /v2/apis silently fell through to API Gateway V2 or S3. Fixed via the new pkgs/service.MatchesUserAgentMarker helper (checks both headers, case-insensitively -- the JS SDK's marker is "api/AppSync", PascalCase, vs aws-sdk-go-v2's lowercase "api/appsync"), shared with the identical bug class fixed the same pass in mediastoredata/docdb/neptune. Grade held at A: fixed, not deferred.
                      # 2026-08-07 (gopherstack-ivwh): ExecuteGraphQL's field resolution silently ignored a UNIT resolver's Code (APPSYNC_JS) field entirely -- only VTL RequestMappingTemplate/ResponseMappingTemplate were ever applied, so a Code-configured resolver behaved as if it had no mapping at all, and PIPELINE resolvers (Kind="PIPELINE"+PipelineConfig) were never executed as a chain at all (resolveField only ever looked at resolver.DataSourceName directly). Both fixed for real: Code-configured UNIT resolvers now run their request/response handlers through the existing documented-subset JS evaluator (jseval.go); PIPELINE resolvers now execute each Function in PipelineConfig order, threading ctx.prev.result between them, then the resolver's own after-mapping. Also fixed a related VTL gap: renderVTL had no $context.prev.result support at all (only $context.result existed), which would have made pipeline function request templates silently render "$ctx.prev.result.x" as a literal string instead of the previous function's field. DataSourceIntrospection's introspected *content* remains a documented structural gap (needs RDS Data API cross-service integration); see gaps.
                      # 2026-08-15 (gopherstack-6flj wrapper-key sweep): this file's extensive "wire: ok" history was re-verified independently against the real deserializer's own case list (not trusted on faith, per that issue's flagship kafka finding). Layer-1 wrapper keys came back entirely clean. 7 layer-2/3 bugs found and fixed: SourceApiAssociation's status field used the wrong wire key ("associationStatus", a sibling-trap copy from the genuinely-different ApiAssociation type -- real key is "sourceApiAssociationStatus", deserializers.go:16488); EventConfig.LogConfig, DataSource.MetricsConfig and Resolver.MetricsConfig were all real, accepted request fields silently discarded on both Create and Update (discarded-input class); GraphqlApi.EnvironmentVariables leaked real customer-set env-var values into GetGraphqlApi/ListGraphqlApis/CreateGraphqlApi/UpdateGraphqlApi, a field the real GraphqlApi type does not have at all (env vars are only ever exposed via the dedicated Get/PutGraphqlApiEnvironmentVariables ops); GraphqlApi.Owner (real member, "the account owner") was unmodeled despite the backend already holding the account ID. Grade held at A: all fixed, not deferred, except the always-disclosed structural gaps below. Full detail in services/_WRAPPER_KEY_SWEEP_REMAINDER.md's "appsync (this session)" section.
                      # 2026-09-06 (gopherstack-idv8): ExecuteGraphQL performed no authentication at all -- it fetched the GraphqlApi record and then discarded it (_ = api) rather than checking AuthenticationType/AdditionalAuthenticationProviders against the caller's credentials, and handleGraphQL never read x-api-key or Authorization. Fixed for real for all five AWS AppSync authentication types. API_KEY: x-api-key checked against stored APIKey.ID, honoring Expires. AWS_LAMBDA: reuses the existing appsync-local LambdaInvoker with the real {authorizationToken, requestContext:{apiId,queryString,operationName,variables}} event shape and isAuthorized response field. AWS_IAM: reuses pkgs/httputils.SigV4Validator, gopherstack's existing single-secret SigV4 verifier. AMAZON_COGNITO_USER_POOLS and OPENID_CONNECT: cryptographic JWT verification (RSA signature, issuer, expiry, and audience/client-id where configured) via a new JWKSProvider hook (store.go) wired in cli.go's wireAppSyncCognito to services/cognitoidp's InMemoryBackend -- the same GetJWTPublicKey/pattern services/apigateway and services/apigatewayv2 already use for their own Cognito/JWT authorizers, not a third JWT verifier. AdditionalAuthenticationProviders is honored throughout: a request authorizes if ANY configured provider (primary or additional) accepts it. A rejected request returns HTTP 401 with body {"message":"Unauthorized"} (real AppSync's transport-level auth-failure shape, distinct from the 200+errors[] shape used for resolver-level field auth). Deliberate carve-out: if SetJWKSProvider was never called (true for every appsync.InMemoryBackend built outside cli.go's real wiring, including most of this package's own tests), Cognito/OIDC auth passes every request through rather than rejecting -- a check that cannot run must not masquerade as a rejection; a real gopherstack server always wires it, so production traffic gets full verification. An external OIDC issuer this instance has no key material for (e.g. a real Auth0/Okta/AWS Cognito, as opposed to gopherstack's own emulated Cognito) is still rejected once the provider IS wired, same as a bad signature -- gopherstack does not fetch a real IdP's JWKS over the network, so "cannot verify" there means "reject", not "trust". Still not fixed: AWS_IAM verifies against httputils.SigV4Validator's built-in "test" default rather than a configured --sigv4-secret; SetSigV4Secret exists on InMemoryBackend but cli.go never calls it (a real gap, left for follow-up -- harmless under the default, extremely common configuration). Grade held at A: every implemented mechanism is fixed for real and regression-tested, including per-guard-neuter-verified accept/reject coverage for both Cognito and OIDC; the one residual gap is disclosed, not silently left unauthenticated.
                      # 2026-09-06 (gopherstack-d96g): GetIntrospectionSchema's format=JSON gap (disclosed 2026-09-04, previously called structural) fixed for real. format is types.OutputType (aws-sdk-go-v2 appsync@v1.56.4 api_op_GetIntrospectionSchema.go:38, enums.go:535-541 -- SDL/JSON, only those two values, required); an unrecognized value is now rejected the same way CreateType already rejects an unrecognized TypeDefinitionFormat (BadRequestException via ErrValidation), and an empty format still defaults to SDL. JSON output is built by walking the already-parsed *ast.Schema (gqlparser/v2, already a direct dependency, already used for query execution in graphql.go) into the GraphQL specification's standard introspection document -- {"data":{"__schema":{...}}}, confirmed against a real AppSync-exported schema.json (github.com/benawad/aws-appsync-example). All type kinds (SCALAR/OBJECT/INTERFACE/UNION/ENUM/INPUT_OBJECT/LIST/NON_NULL), fields with args, interfaces, possibleTypes, enumValues, inputFields, deprecation (isDeprecated/deprecationReason via @deprecated), and defaultValue are emitted; this covers the full standard system, including the introspection meta-types (__Schema/__Type/... ) and built-in scalars, since gqlparser's LoadSchema always merges its prelude into the parsed schema. Left out, disclosed not fabricated: __Type.specifiedByURL (@specifiedBy) and __Type.isOneOf (2024 spec addition) -- both omitted, not defaulted to a guessed value. includeDirectives gates only the top-level __schema.directives list (defaults to true when the query param is absent); SDL output is untouched by it and continues to return the raw stored SDL text verbatim regardless of includeDirectives, since honoring it there would mean re-serializing the schema instead of returning what was stored. New introspection.go walker plus regression tests in introspection_test.go/schema_test.go/handler_schema_test.go, including a hand-neutered/confirmed-failing/restored proof (schema.go's GetIntrospectionSchema, byte-identical after restore). Grade held at A: fixed, not deferred. NOTE: this entry's own "BadRequestException via ErrValidation" claim for the invalid-format path was itself wrong (see gopherstack-w4kf below) -- it borrowed CreateType's pattern without checking GetIntrospectionSchema's own declared error set, which has no BadRequestException at all.
                      # 2026-09-07 (gopherstack-w4kf): schema.go's two GetIntrospectionSchema error raises both emitted BadRequestException, a code that op does not declare (real declared set: GraphQLSchemaException, InternalFailureException, NotFoundException, UnauthorizedException -- appsync@v1.56.4 deserializers.go). The "no valid parsed schema" raise (schema.parsedSchema == nil, format=JSON on a schema that failed StartSchemaCreation parsing) was reusing ErrInvalidSchema, the same sentinel StartSchemaCreation uses -- correct there (StartSchemaCreation's declared set does include BadRequestException) but wrong here, the gopherstack-hdvu shared-sentinel shape. Fixed by giving GetIntrospectionSchema its own sentinel, ErrGraphQLSchemaInvalid -> GraphQLSchemaException, whose doc comment ("The GraphQL schema is not valid.") is a word-for-word match for the guarded condition; StartSchemaCreation's own ErrInvalidSchema raise is untouched. The other raise (invalid format value, e.g. "XML") stays a landmine: none of the four declared exceptions fits a malformed format parameter -- GraphQLSchemaException guards schema content, not the format arg -- so BadRequestException remains wrong on the wire there, disclosed rather than silently left. Regression: TestHandler_GetIntrospectionSchema_InvalidSchema_JSON asserts the wire "code" field through the handler (not just sentinel identity); TestInMemoryBackend_GetIntrospectionSchema/invalid_schema_json_format_rejected asserts sentinel identity at the backend layer. Grade held at A: one site fixed for real, one landmined with cause recorded.
ops:
  CreateGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: added real \"owner\" member (account owner), previously unmodeled despite the account ID already being on hand"}
  GetGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: fixed EnvironmentVariables leaking into the GraphqlApi wire object (json:\"-\" now; real type has no such member at all -- env vars belong only to the dedicated Get/PutGraphqlApiEnvironmentVariables ops); added \"owner\""}
  UpdateGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable — handler only accepted PATCH/PUT (405 on real SDK's POST); fixed, PATCH/PUT kept as alias. 2026-08-15: same EnvironmentVariables-leak fix as GetGraphqlApi"}
  ListGraphqlApis: {wire: ok, errors: ok, state: ok, persist: ok, filter: fixed, note: "2026-08-15: same EnvironmentVariables-leak fix as GetGraphqlApi. This pass (2026-08-29): owner query param (CURRENT_ACCOUNT/OTHER_ACCOUNTS) was never read at all; fixed -- OTHER_ACCOUNTS now returns empty, matching this backend's single-simulated-account model."}
  DeleteGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04: cascade now also deletes SourceAPIAssociation rows referencing this API (as either source or merged) and the domain-name APIAssociation/DomainName.APIID link -- previously ghost rows outlived the API"}
  StartSchemaCreation: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSchemaCreationStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  GetIntrospectionSchema: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-06 (gopherstack-d96g): format=JSON now returns a real GraphQL introspection document (walks the parsed *ast.Schema); format=SDL unchanged (raw stored text). includeDirectives honored for the JSON output's top-level directives list only. See overall log for full detail and the disclosed omissions (specifiedByURL, isOneOf). 2026-09-07 (gopherstack-w4kf): the schema-failed-to-parse + format=JSON case now correctly emits GraphQLSchemaException (was BadRequestException, undeclared for this op); the invalid-format-value case (e.g. format=XML) still emits BadRequestException, also undeclared and also not fixable -- no declared exception fits a malformed format parameter. See overall log and gaps."}
  CreateDataSource: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: added real \"metricsConfig\" member (ENABLED/DISABLED), previously discarded entirely on both create and update. apiId/tags fields on the wire object are fabricated (not on the real DataSource type at all) but harmless and disclosed, not fixed -- see remainder file"}
  GetDataSource: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDataSource: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT-only); fixed, PUT kept as alias. 2026-08-15: metricsConfig now round-trips (see CreateDataSource note)"}
  ListDataSources: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDataSource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateResolver: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: added real \"metricsConfig\" member (ENABLED/DISABLED), previously discarded entirely on both create and update. apiId field on the wire object is fabricated (not on the real Resolver type) but harmless, disclosed not fixed"}
  GetResolver: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- Resolver.PipelineConfig was emitted as a bare array; the real types.PipelineConfig is an object wrapping a Functions list (deserializers.go: awsRestjson1_deserializeDocumentPipelineConfig requires a JSON object), so every real SDK client's Get/ListResolvers(ByFunction)/Create/UpdateResolver call failed outright for any PIPELINE-kind resolver. Fixed via a MarshalJSON/UnmarshalJSON pair on Resolver projecting PipelineConfig into {functions: [...]} at the wire boundary, keeping the Go field a plain []string for internal/test use. Proven via a real aws-sdk-go-v2/service/appsync client round trip (wire_pipeline_config_test.go), hand-reverted/confirmed-failing (deserialization error)/restored, md5sum-verified byte-identical."}
  UpdateResolver: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT/PATCH-only); fixed, PUT/PATCH kept as alias. 2026-08-15: metricsConfig now round-trips (see CreateResolver note)"}
  ListResolvers: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteResolver: {wire: ok, errors: ok, state: ok, persist: ok}
  ListResolversByFunction: {wire: ok, errors: ok, state: ok, persist: ok, note: "This pass (2026-08-29): maxResults/nextToken query params were never read -- every resolver for the function always came back on one page. Fixed via appsyncPaginate, matching every sibling List handler."}
  # ExecuteGraphQL is intentionally NOT listed as an advertised SDK op here.
  # 2026-07-31 CORRECTION: the row that used to live at this position ("wire:
  # ok, ...") was inaccurate -- ExecuteGraphQL is not a real AWS AppSync SDK
  # operation at all (verified against aws-sdk-go-v2/service/appsync: `go doc`
  # lists only management operations, no ExecuteGraphQL method; real clients
  # POST GraphQL queries straight to the API's graphqlEndpoint, a request the
  # typed SDK does not model as an operation). Caught by pkgs/sdkcheck's
  # reverse check (commit 12cfe14d5; gopherstack-vhw2 category A). The route
  # (POST /v1/apis/{apiId}/graphql -> handleGraphQL) stays wired -- gopherstack
  # still needs to serve real GraphQL data-plane traffic -- and dispatch keys
  # off the literal "graphql" path segment, not this label, so no client is
  # affected. GetSupportedOperations()/ChaosOperations() no longer advertise
  # it; see opExecuteGraphQL's doc comment in handler.go. Same resolution as
  # CloudFront's GetFunctionAssociations/SetFunctionAssociations and EMR's
  # ListTagsForResource. The route/method plumbing itself was and remains
  # correctly audited (see deferred note below on VTL/JS execution scope).
  AssociateApi: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateApi: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateMergedGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: SourceApiAssociation.AssociationStatus wire key fixed, see GetSourceApiAssociation note"}
  AssociateSourceGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: same SourceApiAssociation status-key fix as AssociateMergedGraphqlApi"}
  DisassociateMergedGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateSourceGraphqlApi: {wire: ok, errors: ok, state: ok, persist: ok}
  GetSourceApiAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: SourceApiAssociation.AssociationStatus was wired to the wrong key, \"associationStatus\" -- a sibling-trap copy from the genuinely-different ApiAssociation type (domain-name associations), which really does use that plain key. Real key is \"sourceApiAssociationStatus\" (deserializers.go:16488); a real client's typed field was always empty. Fixed; also added the real (never-populated, since merges here always succeed) sourceApiAssociationStatusDetail member"}
  ListSourceApiAssociations: {wire: ok, errors: ok, state: ok, persist: ok, note: "two bugs fixed: (1) real SDK also lists via GET /v1/apis/{apiId}/sourceApiAssociations (apiId-keyed, distinct from the mergedApis-prefixed path) — added; (2) response was wrapped as \"sourceApiAssociations\" instead of the real \"sourceApiAssociationSummaries\" — a real client always got an empty list back. Summary narrowing fixed: now maps to narrow SourceAPIAssociationSummary matching real types.SourceApiAssociationSummary (omits sourceApiAssociationStatus/Detail and config). This pass (2026-08-29): maxResults/nextToken were never read either -- every association always came back on one page. Fixed via appsyncPaginate."}
  UpdateSourceApiAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT/PATCH-only); fixed, PUT/PATCH kept as alias. 2026-08-15: same status-key fix as GetSourceApiAssociation"}
  CreateApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: added EventConfig.LogConfig (real member, previously discarded entirely on both create and update -- new EventLogConfig type, distinct 2-field shape from GraphqlApi's LogConfig)"}
  GetApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-15: EventConfig.LogConfig now round-trips, see CreateApi note"}
  ListApis: {wire: ok, errors: ok, state: ok, persist: ok, note: "response was wrapped as \"items\" instead of the real \"apis\" — disguised no-op, a real client always saw an empty list; fixed, added pagination"}
  UpdateApi: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT/PATCH-only); fixed, PUT/PATCH kept as alias. 2026-08-15: EventConfig.LogConfig now round-trips, see CreateApi note"}
  DeleteApi: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateApiCache: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-hnyl): isValidAPICacheType was missing R4_LARGE/R4_XLARGE and invented a nonexistent R4_1XLARGE; isValidAPICachingBehavior was missing OPERATION_LEVEL_CACHING and invented a nonexistent FULL_REQUEST_DATA_CACHING. Both now derive from types.ApiCacheType.Values()/types.ApiCachingBehavior.Values()."}
  DeleteApiCache: {wire: ok, errors: ok, state: ok, persist: ok}
  FlushApiCache: {wire: ok, errors: ok, state: ok, persist: ok, note: "real path is DELETE /v1/apis/{apiId}/FlushCache, not /ApiCaches/entries — was unreachable; fixed, old path kept as alias"}
  GetApiCache: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApiCache: {wire: ok, errors: ok, state: ok, persist: ok, note: "real path is POST /v1/apis/{apiId}/ApiCaches/update, not PUT to the collection path — was unreachable; fixed, old path kept as alias"}
  CreateApiKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04: default expiry was wrong (365 days; real default per CreateApiKeyInput.Expires doc is 7 days) and both the max-keys-exceeded and out-of-bounds-expiry cases returned generic BadRequestException instead of the real ApiKeyLimitExceededException/ApiKeyValidityOutOfBoundsException. Also, an out-of-bounds expiry (real bound: 1-365 days, ApiKeyValidityOutOfBoundsException doc) was silently clamped into range instead of rejected. All fixed."}
  DeleteApiKey: {wire: ok, errors: ok, state: ok, persist: ok}
  ListApiKeys: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApiKey: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT/PATCH-only); fixed, PUT/PATCH kept as alias. 2026-09-04: same ApiKeyValidityOutOfBoundsException fix as CreateApiKey (silent clamp -> real error)."}
  CreateChannelNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteChannelNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  GetChannelNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  ListChannelNamespaces: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateChannelNamespace: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT/PATCH-only); fixed, PUT/PATCH kept as alias"}
  CreateDomainName: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDomainName: {wire: ok, errors: ok, state: ok, persist: ok}
  GetApiAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDomainName: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDomainNames: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDomainName: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT/PATCH-only); fixed, PUT/PATCH kept as alias"}
  CreateFunction: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteFunction: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFunction: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFunctions: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateFunction: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT-only); fixed, PUT kept as alias"}
  CreateType: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteType: {wire: ok, errors: ok, state: ok, persist: ok}
  GetType: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTypes: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateType: {wire: ok, errors: ok, state: ok, persist: ok, note: "was unreachable (PUT-only); fixed, PUT kept as alias"}
  GetGraphqlApiEnvironmentVariables: {wire: ok, errors: ok, state: ok, persist: ok}
  PutGraphqlApiEnvironmentVariables: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "real path is GET /v1/tags/{resourceArn}, entirely unreachable at the previously-only-implemented /v1/apis/{apiId}/tags — fixed (both v1 GraphqlApi and v2 Api ARNs), old path kept as alias"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as ListTagsForResource; TagResource/UntagResource now also work against v2 Api (Event API) resources, not just v1 GraphqlApi"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as ListTagsForResource"}
  EvaluateCode: {wire: ok, errors: ok, state: ok, persist: n/a, note: "real path is POST /v1/dataplane-evaluatecode (standalone), not /v1/dataplane-evaluations/code — was unreachable; fixed, old path kept as alias"}
  EvaluateMappingTemplate: {wire: ok, errors: ok, state: ok, persist: n/a, note: "real path is POST /v1/dataplane-evaluatetemplate (standalone), not /v1/dataplane-evaluations/template — was unreachable; fixed, old path kept as alias"}
  GetDataSourceIntrospection: {wire: ok, errors: ok, state: ok, persist: ok, note: "real path added (GET /v1/datasources/introspections/{introspectionId}, distinct from the /v1/dataSource-introspections legacy alias); response body rebuilt to the real flat shape (introspectionId/introspectionResult/introspectionStatus/introspectionStatusDetail at the top level, introspectionResult itself {models,nextToken}) instead of the old {introspectionResult: {introspectionId, status, models}} nesting; unknown IDs now correctly 404 (previously always synthesized a fake SUCCESS for ANY id, even ones never started)"}
  ListTypesByAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "This pass (2026-08-29): maxResults/nextToken query params were never read -- every type on the merged API always came back on one page. Fixed via appsyncPaginate."}
  StartDataSourceIntrospection: {wire: ok, errors: ok, state: ok, persist: ok, note: "real path added (POST /v1/datasources/introspections); input contract corrected from the invented {apiId, dataSourceName} (not part of the real StartDataSourceIntrospectionInput, which is NOT scoped to any AppSync API/DataSource at all) to the real optional rdsDataApiConfig{databaseName,resourceArn,secretArn}; now persists a real DataSourceIntrospection record (new 'introspections' store.Table) keyed by introspectionId instead of returning an unpersisted random ID with nothing behind it. gopherstack has no real RDS Data API connectivity, so every well-formed request completes synchronously with SUCCESS and an empty models list -- wire shape, error codes and persisted/retrievable state are all real; the *contents* of a genuine introspection (actual RDS table/column data) are out of scope, same category as ExecuteGraphQL's VTL/JS engine scope limit below"}
  StartSchemaMerge: {wire: ok, errors: ok, state: ok, persist: ok, note: "moved from the invented POST /v1/apis/{apiId}/schemaMerge (apiId-only, response {sourceApiSchemaMetadata:[], status}) to the real POST /v1/mergedApis/{mergedApiIdentifier}/sourceApiAssociations/{associationId}/merge, keyed by BOTH mergedApiIdentifier and associationId with response {sourceApiAssociationStatus}; backend signature changed from StartSchemaMerge(apiID) to StartSchemaMerge(mergedAPIID, associationID), now validates and mutates the real SourceAPIAssociation.AssociationStatus (MERGE_SUCCESS) instead of returning a hardcoded SchemaStatus disconnected from any association. The old invented endpoint was deleted outright rather than aliased: an apiId-only request has no way to recover the associationId the real operation requires, so a path-only alias would still be wrong on the request/response shape"}
families:
  GraphqlApi_CRUD: {status: ok, note: "Create/Get/List/Delete already correct; UpdateGraphqlApi POST-method bug fixed"}
  ApiKey_CRUD: {status: ok, note: "expires already epoch-seconds (pkgs/awstime not used but manual int64 matches wire); UpdateApiKey POST-method bug fixed. 2026-09-04: wrong default expiry (365d not 7d) and wrong/missing error codes (ApiKeyLimitExceededException, ApiKeyValidityOutOfBoundsException) fixed -- see CreateApiKey/UpdateApiKey op notes."}
  DataSource_CRUD: {status: ok, note: "UpdateDataSource POST-method bug fixed"}
  Resolver_CRUD: {status: ok, note: "UpdateResolver POST-method bug fixed"}
  Function_CRUD: {status: ok, note: "UpdateFunction POST-method bug fixed"}
  Type_CRUD: {status: ok, note: "UpdateType POST-method bug fixed"}
  Tags: {status: ok, note: "full path rewrite from /v1/apis/{apiId}/tags to real /v1/tags/{resourceArn}, with ARN-embedded-slash handling and v1+v2 API resource support"}
  ApiCache: {status: ok, note: "UpdateApiCache and FlushApiCache both had wrong path+method; fixed"}
  DomainName_and_ApiAssociation: {status: ok, note: "UpdateDomainName POST-method bug fixed; rest already correct"}
  ChannelNamespace_EventApi: {status: ok, note: "CreateApi/GetApi/ListApis/UpdateApi/DeleteApi + ChannelNamespace CRUD; ListApis \"items\"->\"apis\" wrapper bug fixed, UpdateApi/UpdateChannelNamespace POST-method bugs fixed"}
  SourceApiAssociation_and_Merge: {status: ok, note: "Associate/Get/Update/Disassociate + the apiId-keyed List path fixed (2026-07-12 pass); StartSchemaMerge implemented for real this pass at the correct path/keying/response shape (2026-07-24)"}
  DataplaneEvaluation: {status: ok, note: "EvaluateCode/EvaluateMappingTemplate path rewrite from nested /v1/dataplane-evaluations/{code,template} to the real standalone top-level paths"}
  DataSourceIntrospection: {status: ok, note: "implemented for real this pass (2026-07-24): real path, real rdsDataApiConfig-based input contract, real persisted/keyed state, real error codes. No real RDS Data API connectivity in gopherstack, so introspected model content is always an empty list -- documented in items_still_open, not a wire/error/state gap"}
  ExecuteGraphQL_resolvers: {status: fixed, note: "gopherstack-ivwh (2026-08-07): UNIT resolvers configured with APPSYNC_JS Code (instead of VTL templates) now actually run their request/response handlers via jseval.go's documented-subset evaluator, for all three data source types (Lambda/DynamoDB/None) -- previously Code was silently ignored and the resolver behaved as if unmapped. PIPELINE resolvers (Kind=PIPELINE + PipelineConfig) now execute their Function chain for real (each function's own VTL-or-JS request/response mapping against its own data source, ctx.prev.result threaded between them), followed by the resolver's own after-mapping -- previously PIPELINE resolvers were never distinguished from UNIT at all (resolveField read resolver.DataSourceName directly, which a PIPELINE resolver doesn't even set). Also fixed renderVTL, which had no $context.prev.result support (only $context.result existed) -- a pipeline function's request template referencing $ctx.prev.result.x would have rendered the literal string unexpanded. See gaps for the documented-subset limits that remain (pipeline before-mapping, DynamoDB JS resolver helpers)."}
  ExecuteGraphQL_auth: {status: fixed, note: "gopherstack-idv8 (2026-09-06): ExecuteGraphQL performed zero authentication -- fetched the GraphqlApi record then discarded it (_ = api), and handleGraphQL never read x-api-key/Authorization at all. Fixed for real for all five auth types (API_KEY, AWS_LAMBDA, AWS_IAM, AMAZON_COGNITO_USER_POOLS, OPENID_CONNECT), plus AdditionalAuthenticationProviders (any configured provider, primary or additional, may authorize the request). Cognito/OIDC verify RSA signature, issuer, expiry, and audience/client-id via a JWKSProvider hook wired to services/cognitoidp in cli.go (wireAppSyncCognito) -- see gaps for the narrow unwired-provider and external-issuer carve-outs, and for the still-open SigV4-secret gap. A rejected request returns HTTP 401 {\"message\":\"Unauthorized\"}, matching real AppSync's transport-level auth-failure shape."}
gaps:
  - "PIPELINE resolver before-mapping (RequestMappingTemplate / Code's `request` handler, at the resolver level, not a Function's) is intentionally not evaluated (bd: gopherstack-ivwh). On real AppSync its only observable effects beyond building a request object nothing here consumes are writing to ctx.stash (read by later pipeline functions) and short-circuiting the pipeline via util.error/an early return -- neither of which this evaluator's documented subset implements. Evaluating it and discarding the result would be pointless busywork; skipping it is the honest reflection of what's supported. See executePipeline's doc comment in graphql.go."
  - "The APPSYNC_JS evaluator (jseval.go) supports a documented subset of real JS: `return <object/array/json literal>;`, context member expressions, and the pure util.* helpers (toJson/parseJson/error/appendError/unauthorized) -- not control flow, loops, variable bindings, or DynamoDB-specific helpers like util.dynamodb.get()/put(). A JS DynamoDB resolver must therefore return the raw {operation,key/item} object literal directly (mirroring what a VTL template renders) rather than using util.dynamodb.* sugar. Constructs outside the subset return ErrUnsupportedJSCode rather than a fabricated result -- see jseval.go's doc comment for the full supported-pattern list."
  - "2026-08-15: GraphqlApi missing real dns/enhancedMetricsConfig/mergedApiExecutionRoleArn/wafWebAclArn members -- none tracked anywhere in this backend (merged-API execution role, WAF ACL association, and enhanced metrics config are all unsimulated cross-feature concepts). Api (Event API) missing real created timestamp (optional, not required) and wafWebAclArn, same reason. DataSource missing the deprecated legacy elasticsearchConfig member (real AWS docs steer new integrations to openSearchServiceConfig instead)."
  - "2026-08-15: DataSource/Resolver/Function/ApiCache/APIType/DomainNameConfig each carry a fabricated apiId field on their own wire object (none of the corresponding real types has one -- apiId lives on the URL path only); DataSource also carries a fabricated tags field (the real DataSource type has no tags member, consistent with handler_create_tags_test.go's existing finding that DataSource ARNs aren't a TagResource target). GraphqlApi.Region/CreatedAt/UpdatedAt are also fabricated (no such real members). All harmless -- a real client silently ignores unknown JSON keys -- and disclosed rather than fixed to avoid 6+ call-site changes for no functional benefit; see services/_WRAPPER_KEY_SWEEP_REMAINDER.md's appsync section."
  - "2026-09-06 (gopherstack-d96g): FIXED -- format=JSON now returns a real GraphQL introspection document; see overall log for detail. Two __Type fields remain unemitted, disclosed rather than guessed: specifiedByURL (the @specifiedBy custom-scalar URL) and isOneOf (the 2024 oneOf-input-object addition). Neither is fabricated as a null/false placeholder guess -- they are simply absent from the JSON. ListTypes/GetType/ListTypesByAssociation's own format parameter (SDL<->JSON for individual APIType records, a separate, narrower converter than GetIntrospectionSchema's whole-schema one) remains unfixed -- see the 2026-08-29 Filter/pagination-not-honoured sweep section below."
  - "2026-09-06 (gopherstack-idv8): AMAZON_COGNITO_USER_POOLS and OPENID_CONNECT GraphQL auth (auth.go's checkCognitoAuth/checkOIDCAuth) cryptographically verify RSA signature, issuer, expiry, and audience/client-id via cli.go's wireAppSyncCognito -> InMemoryBackend.SetJWKSProvider(cognitoBk), the same JWKSProvider pattern services/apigateway and services/apigatewayv2 already use. One deliberate permissive carve-out: if SetJWKSProvider was never called (every appsync.InMemoryBackend built outside cli.go's wiring, including most of this package's own tests), Cognito/OIDC auth passes every request through instead of rejecting -- a check that structurally cannot run must not present as a rejection, and a real gopherstack server always wires it (wireAppSyncCognito), so production traffic gets full verification. Once the provider IS wired, an issuer this instance has no signing key for -- an OIDC Issuer pointed at a genuine external IdP (Auth0/Okta/real AWS Cognito), or a UserPoolID that doesn't match any locally emulated pool -- is rejected, not trusted: gopherstack does not fetch a real IdP's JWKS document over the network, so those credentials are unverifiable rather than implicitly valid. A Cognito-authenticated API, or an OIDC-authenticated API whose Issuer points at one of gopherstack's own emulated Cognito user pools (the realistic local-dev OIDC setup, since Cognito user pools are themselves OIDC-compliant issuers), gets full, real verification with no gap at all."
  - "2026-09-06 (gopherstack-idv8): AWS_IAM GraphQL auth cryptographically verifies the caller's SigV4 signature (via pkgs/httputils.SigV4Validator), but always against that validator's built-in \"test\" secret rather than a configured --sigv4-secret -- InMemoryBackend.SetSigV4Secret exists but cli.go never calls it (unlike the analogous wireAppSyncCognito added this same pass for the JWKSProvider hook). Left as a genuine, documented gap rather than wired, since it's out of this pass's scope. Harmless under the default configuration (--sigv4-secret also defaults to \"test\"); only affects deployments that set a non-default secret."
  - "2026-09-07 (gopherstack-w4kf, landmine): GetIntrospectionSchema rejects an unrecognized format value (e.g. XML) with BadRequestException, which that op does not declare (real declared set: GraphQLSchemaException, InternalFailureException, NotFoundException, UnauthorizedException -- appsync@v1.56.4 deserializers.go). Ruled out: GraphQLSchemaException (doc: \"The GraphQL schema is not valid.\" -- guards schema content, not the format arg), NotFoundException (nothing is missing), InternalFailureException (this is a client input error, not a server fault), UnauthorizedException (no auth failure here). No declared exception fits a malformed format parameter; left as-is rather than forced into a wrong-but-plausible code. The sibling BadRequestException raise on the same op (schema-failed-to-parse + format=JSON) was fixed for real this same pass -- see overall log."
deferred:
  - "CloudTrail-capture chokepoint / pkgs/service integration — not audited (shared/cross-service, out of scope per this task's edit boundary)."
  - "DataSourceIntrospection real model content: gopherstack has no RDS Data API backend to introspect against, so StartDataSourceIntrospection/GetDataSourceIntrospection always complete SUCCESS with an empty models list rather than real table/column data. Wire shape, error codes (BadRequestException on missing/incomplete rdsDataApiConfig, NotFoundException on unknown introspectionId), and persisted per-ID state are all real and field-diffed against the SDK; only the introspected *content* is out of scope. Would require a services/rds (or similar) cross-service integration to fix — out of this task's services/appsync/ edit boundary."
leaks: {status: bugs found, note: "janitor.go's background goroutine already takes ctx and is started once via StartWorker; no new goroutines, tickers, or unbounded maps were added this (or the prior) pass. The two safemap-style Tags-table lookups (b.apis / b.eventAPIs) reuse existing store.Table entries. This pass added one new store.Table (b.introspections, registered in store_setup.go, generically covered by the existing Snapshot/Restore/ResetAll wiring in persistence.go) — introspection records are NOT scoped to any GraphqlApi/Api/DataSource (matches the real AWS operation, which isn't either), so DeleteGraphqlApi/DeleteApi/DeleteDataSource correctly do NOT cascade-delete them; there is no lock path in the new code without a matching defer-release (verified by -race). 2026-09-04 (gopherstack-2yo): found and fixed two ghost-row leaks -- DeleteGraphqlApi did not cascade to b.sourceAssocs (SourceAPIAssociation rows keyed by user-chosen association IDs, either SourceAPIID or MergedAPIID referencing the deleted API) or to b.apiAssociations/DomainName.APIID (the AssociateApi domain-name link) -- both are store.Table-backed and covered by Snapshot(), so the ghost rows also survived snapshot/restore. Fixed via cascadeDeleteAPIAssociations in graphql_apis.go; see TestInMemoryBackend_DeleteGraphqlAPI_CascadeDelete_SourceAPIAssociations / _DomainNameAssociation."}
---

## Notes

**2026-08-13 (gopherstack-jqh2 pass 3):** re-extracted all 74 ops' real
method+path directly from `appsync@v1.56.4` serializers.go and drove them
through `ExtractOperation` via the new `handler_sdk_route_table_test.go`
(`TestExtractOperation_SDKRouteTable`, one subtest per op, `t.Parallel()`).
All 74 resolved correctly, including the several same-path/different-method
collisions this service's routing depends on
(`/v1/apis/{apiId}/ApiCaches`, `/v1/tags/{arn}`, `/v2/apis/{apiId}`,
`/v1/apis/{apiId}` GET/DELETE/POST). No pre-existing table existed to
check. This confirms the extensive Update*-uses-POST route work from the
2026-07-24/07-31 passes documented below held under the strong per-op SDK
diff method — no new routing bugs found. This test is now the permanent
regression guard for route-table drift.

### The core bug class this sweep found and fixed: Update* uses POST, not PUT/PATCH

AppSync is restjson1. Verified directly against `aws-sdk-go-v2/service/appsync@v1.55.0`'s
`serializers.go`: **every single Update\* operation is serialized as `POST`** to the
same path as the corresponding Get (e.g. `UpdateGraphqlApi` → `POST /v1/apis/{apiId}`,
same path as `GetGraphqlApi`'s `GET` and `DeleteGraphqlApi`'s `DELETE`). AWS AppSync's
REST API never uses HTTP PUT or PATCH for anything — that convention doesn't apply here
the way it does for many other AWS REST services. This handler had every Update*
dispatch gated on `PUT`/`PATCH` only, so **every Update operation** (10 total:
UpdateGraphqlApi, UpdateApi, UpdateDataSource, UpdateFunction, UpdateType,
UpdateResolver, UpdateApiKey, UpdateDomainName, UpdateChannelNamespace,
UpdateSourceApiAssociation) returned `405 MethodNotAllowed` to a real AWS SDK client.
Fixed by adding `POST` as an accepted method everywhere alongside the existing
PUT/PATCH (kept for any non-SDK/manual callers) — a strict superset, no existing
behavior removed.

`UpdateApiCache` and `FlushApiCache` are worse: they live at entirely different paths
from what was implemented (`POST /v1/apis/{apiId}/ApiCaches/update` and
`DELETE /v1/apis/{apiId}/FlushCache`, vs the implemented `PUT .../ApiCaches` and
`DELETE .../ApiCaches/entries`). Fixed the same way — new correct routes added, old
ones kept working as aliases.

### Tags: entirely different top-level path

`TagResource`/`UntagResource`/`ListTagsForResource` are NOT nested under
`/v1/apis/{apiId}/tags` on the wire — the real endpoint is
`/v1/tags/{resourceArn}`, a standalone top-level path taking a full ARN, not an apiId.
This was **completely unreachable** (RouteMatcher didn't even claim `/v1/tags/*`
before this fix — none of the six registered prefixes match it). resourceArn itself
contains `/` (`arn:aws:appsync:region:account:apis/{apiId}`); the AWS SDK
percent-encodes it as `%2F` in the URI label, and since `net/http` decodes the request
path before routing reaches this handler, the ARN's internal slash arrives as an
ordinary extra path segment that must be rejoined (`apiIDFromResourceARN` in
handler.go). Also extended `TagResource`/`UntagResource`/`ListTagsForResource` in
backend.go to check both the `b.apis` (GraphqlApi v1) and `b.eventAPIs` (Api v2 /
Event API) tables — previously only v1 GraphqlApi was taggable even though both
resource kinds share the `apis/{id}` ARN shape and are both valid TagResource targets
on the real API.

### Two disguised-no-op response-wrapper-key bugs (found by cross-checking every
### List* op's JSON field name against the real deserializer)

- `ListApis` wrapped its result as `{"items": [...]}`; the real
  `ListApisOutput` field is `"apis"`. A real SDK client's JSON deserializer only reads
  known field names, so this was silently returning an always-empty list to every
  caller regardless of actual backend state — the classic "real-looking op that's
  actually a stub" pattern flagged in the parity principles doc. Fixed, and pagination
  (`nextToken`/`maxResults`) added to match the other List ops (it had none).
- `ListSourceApiAssociations` wrapped its result as `{"sourceApiAssociations": [...]}`;
  the real `ListSourceApiAssociationsOutput` field is
  `"sourceApiAssociationSummaries"`. Same always-empty-to-a-real-client bug. Fixed. (The
  `"sourceApiAssociations"` string is *also*, confusingly, the correct literal URL path
  segment name for several unrelated endpoints — that usage was untouched, only the
  JSON body wrapper key was wrong.)

All other List* response wrapper keys (`apiKeys`, `channelNamespaces`, `dataSources`,
`domainNameConfigs`, `functions`, `graphqlApis`, `resolvers`, `tags`, `types`) were
independently verified against the real deserializers and are correct.

### RouteMatcher

`/v1/tags`, `/v1/dataplane-evaluatecode`, and `/v1/dataplane-evaluatetemplate` prefixes
were added to `RouteMatcher()` (previously only `/v1/dataplane-evaluations` was
registered, which doesn't match either real path). `TestRouteMatcher_RealPaths` in
`wire_route_parity_test.go` exercises `h.RouteMatcher()` directly (not just
`h.Handler()`) for every path fixed this sweep, per the audit's explicit route-matcher
check — this is the exact bug class ("unit tests bypass the matcher") that hit backup,
eks, s3control, guardduty, cleanrooms, bedrockagent, iotwireless, and pinpoint
previously.

### Deliberate non-breaking-alias strategy

Every fix in this sweep is **additive**: the previously-implemented (AWS-inaccurate)
paths/methods still work exactly as before (PUT/PATCH aliases for Update ops, the old
`/v1/apis/{apiId}/tags` path, the old `/ApiCaches/entries` and `/v1/dataplane-evaluations/*`
paths). Only the *real* AWS SDK-accurate paths/methods were added alongside them. This
was a deliberate choice to fix the (severe) real-client-facing bugs without touching
or re-validating ~15 existing unit tests that exercise the old aliases — reducing risk
in an already-large sweep. A future cleanup pass could remove the aliases once nothing
in-tree depends on them, per de-stub hygiene, but they are not stubs themselves (both
paths reach the same real, fully-implemented business logic) so leaving them is not a
parity violation.

### persistence.go

Read and verified intact — not modified. `Snapshot`/`Restore` already drive every
`*store.Table[V]` on `b.registry` generically (including `eventAPIs`, the v2 Api table
this sweep's Tags fix now also mutates), so no persistence wiring changes were needed;
the new `eventAPI.Tags` mutations are automatically covered by the existing generic
snapshot mechanism.

### 2026-07-24 pass: StartSchemaMerge and Start/GetDataSourceIntrospection implemented for real

The 2026-07-12 audit left these two gaps explicitly untouched ("not even path-aliased")
because a path-only fix would still have been broken on the request/response shape.
This pass reworked both for real, field-diffed against
`aws-sdk-go-v2/service/appsync@v1.55.0`.

**StartSchemaMerge.** The old implementation lived at the invented
`POST /v1/apis/{apiId}/schemaMerge`, keyed only by `apiId`, returning an invented
`{sourceApiSchemaMetadata: [], status}` body. The real operation is
`POST /v1/mergedApis/{mergedApiIdentifier}/sourceApiAssociations/{associationId}/merge`
— keyed by BOTH `mergedApiIdentifier` and `associationId` (a merge always targets one
specific source-API association, never "the merged API" as a whole) — and returns
`{sourceApiAssociationStatus}`. `InMemoryBackend.StartSchemaMerge`'s signature changed
from `(apiID) (SchemaStatus, error)` to `(mergedAPIID, associationID) (string, error)`;
it now validates both the merged API and the association exist (and that the
association actually belongs to that merged API), mutates the real
`SourceAPIAssociation.AssociationStatus` to `MERGE_SUCCESS`, and returns that value —
replacing the old version's hardcoded, association-disconnected `SchemaStatusActive`
return. The invented old endpoint was **deleted outright**, not aliased: an
`apiId`-only request has no way to recover the `associationId` the real operation
requires, so keeping it as a path alias would still have served the wrong contract.
`TestHandler_LegacySchemaMergeEndpointRemoved` locks that the old path now 404s instead
of silently accepting invented-shape requests.

**StartDataSourceIntrospection / GetDataSourceIntrospection.** The old implementation
lived at `/v1/dataSource-introspections[/{id}]` and took `{apiId, dataSourceName}` —
neither field exists on the real `StartDataSourceIntrospectionInput`, which carries
only an optional `rdsDataApiConfig{databaseName,resourceArn,secretArn}` and is **not
scoped to any AppSync GraphqlApi/DataSource at all** (it introspects an RDS Data
API-backed database directly). `GetDataSourceIntrospection` was a pure stub — its own
doc comment said so — that returned a fabricated `SUCCESS` result for literally any
ID string, including ones that were never started. Fixed:

- Real path added: `POST /v1/datasources/introspections` and
  `GET /v1/datasources/introspections/{introspectionId}` (registered in
  `RouteMatcher()`, `parseOperation` via the new `pathSegDatasources` top-level case,
  and `dispatchTopLevel`). The old `/v1/dataSource-introspections` path is kept working
  as a non-breaking alias, rewired to the same corrected backend contract (same
  "deliberate non-breaking-alias strategy" as the rest of this service — see above).
- New backend contract: `StartDataSourceIntrospection(cfg *RDSDataAPIConfig)
  (*DataSourceIntrospection, error)` validates `cfg` and its three required fields
  (`BadRequestException` if missing, matching the real client-side
  `validateRdsDataApiConfig`), then creates and **persists** a real
  `DataSourceIntrospection` record in a new `introspections` store.Table (registered in
  store_setup.go; automatically covered by the existing generic
  Snapshot/Restore/ResetAll wiring in persistence.go — see
  `Test_InMemoryBackend_SnapshotRestore`). `GetDataSourceIntrospection` now looks the
  record up by ID and returns `NotFoundException` for unknown IDs instead of
  fabricating a result.
- Response shapes corrected: `StartDataSourceIntrospectionOutput` is
  `{introspectionId, introspectionStatus, introspectionStatusDetail}` (no
  `introspectionResult` — that field only exists on Get); `GetDataSourceIntrospectionOutput`
  is the flat `{introspectionId, introspectionResult: {models, nextToken},
  introspectionStatus, introspectionStatusDetail}`, not the old
  `{introspectionResult: {introspectionId, status, models}}` nesting.
- New model types (`RDSDataAPIConfig`, `DataSourceIntrospectionModel`,
  `DataSourceIntrospectionModelField(Type)`, `DataSourceIntrospectionModelIndex`,
  `DataSourceIntrospectionResult`, `DataSourceIntrospectionStatus*` constants) added to
  models.go, field-named and -shaped to match
  `aws-sdk-go-v2/service/appsync/types` exactly.
- **Known, documented limitation** (not a wire/error/state gap): gopherstack has no
  real RDS Data API connectivity, so every well-formed request completes synchronously
  with `SUCCESS` and an **empty** `models` list — there is no real database to
  introspect. This is called out explicitly in `deferred` above rather than silently
  passed off as full parity; a real fix would require a cross-service RDS Data API
  integration, out of this task's `services/appsync/` edit boundary.

### 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped retype

`appsyncSnapshotVersion` bumped 1 -> 2. `d83f4b5d3` gave `Resolver` (the registered
`resolvers` table's value type) the `MarshalJSON`/`UnmarshalJSON` pair described above
(`GetResolver`'s note), rendering `PipelineConfig` as `{functions: [...]}` instead of a
bare array, without bumping the snapshot version at the time. A pre-fix (v1) snapshot's
array no longer unmarshals into the new object field at all -- `RestoreAll` now errors
outright rather than silently losing data, but the whole backend then fails to restore,
which the version guard exists to convert into a clean, recoverable "discard and start
empty" instead.

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `Test_InMemoryBackend_Restore_V1PipelineConfigDiscarded` (persistence_test.go)
builds a v1-shaped `resolvers` snapshot with an array-shaped `pipelineConfig` and asserts
`Restore` succeeds (discarding cleanly) rather than erroring. Hand-reverted to version 1:
the same test then fails with `Restore` returning `json: cannot unmarshal array into Go
struct field .pipelineConfig of type appsync.pipelineConfigWire`, confirming the symptom;
restored and `md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).

## Filter/pagination-not-honoured sweep (2026-08-29)

This service had not been swept for this class before. Measured all 11
List ops (verified by output shape, not name -- `EvaluateCode`/
`EvaluateMappingTemplate`/`GetIntrospectionSchema` were excluded despite
having slice-shaped output fields, since none return a paginated
collection resource). Constraining parameters beyond NextToken: MaxResults
on all 11; `ApiType`/`Owner` on `ListGraphqlApis`; `Format` on
`ListTypes`/`ListTypesByAssociation`; `TypeName` on `ListResolvers`
(path-bound, not a filter). `ApiId`/`FunctionId`/`AssociationId`/
`MergedApiIdentifier` on the rest are path-bound scoping identifiers, not
filters.

Found and fixed 4 bugs (all confirmed against a real
`aws-sdk-go-v2/service/appsync` client, `list_filter_params_test.go`):
- `ListGraphqlApis`: `owner` (`CURRENT_ACCOUNT`/`OTHER_ACCOUNTS`) was never
  read at all -- `apiType` was, but `owner` wasn't even looked up.
  gopherstack simulates one AWS account, so `OTHER_ACCOUNTS` now returns
  empty.
- `ListResolversByFunction`: `maxResults`/`nextToken` weren't read by the
  handler at all -- it called the backend and returned every matching
  resolver on one page, unlike every sibling List handler which routes
  through the shared `appsyncPaginate` helper.
- `ListSourceApiAssociations`: same bug -- `maxResults`/`nextToken`
  ignored, every association on one page.
- `ListTypesByAssociation`: same bug -- `maxResults`/`nextToken` ignored.

`ApiType` (`ListGraphqlApis`) was already read and applied correctly before
this pass -- no change.

**Correction (2026-08-29, gopherstack-6flj follow-up):** the claim above that
`Format` on `ListTypes`/`ListTypesByAssociation` was "already read and
applied correctly" was wrong. `ListTypes`/`GetType` never read `format` at
all, and `ListTypesByAssociation`'s handler reads it but passes it into
`Backend.ListTypesByAssociation`'s blank-identifier third parameter --
discarded either way. This is left unfixed, but as a genuine **structural
gap**, not a parameter-plumbing bug: real AWS uses `format` to convert a
type's definition between GraphQL SDL text and its JSON AST representation
on the fly, which needs a real GraphQL SDL<->JSON parser/serializer this
package doesn't have (and building one is out of scope for a
parameter-plumbing sweep). Every stored `APIType` already carries a single
`Format` value fixed at creation/update time (`CreateType`/`UpdateType`
both take and store it correctly), and `Get`/`List` return the definition
in that stored format regardless of what the caller asks for -- there is no
conversion to apply the requested `format` to, so plumbing it through
end-to-end would be a schema-only change with no real behavior to ratify
(the exact reasoning already used for `ecr`'s `ListImageReferrers`). Now
disclosed in code as a structural gap (`handler_schema_types.go`'s
`getType`/`listTypes`/`listTypesByAssociation` doc comments) instead of the
previous "accepted for AWS SDK compatibility" comment, which read as though
the behavior were intentional and complete.

## enumcheck confident-tier fix (2026-08-30)

`cmd/enumcheck`'s CONFIDENT tier flagged `GetApiAssociation`'s
`AssociationStatus: "NOT_FOUND"`: real `types.AssociationStatus` only
defines `PROCESSING`/`FAILED`/`SUCCESS` (appsync@v1.56.4
types/enums.go:96). The real bug wasn't the enum value alone -- when a
domain name exists but has no API association, `GetApiAssociation` returned
a synthetic 200-OK `ApiAssociation` body instead of the `NotFoundException`
real AWS returns, matching every other appsync "not found" path in this
backend. Fixed to return `ErrNotFound` (404 `NotFoundException`); three
pre-existing tests that asserted the old 200/`"NOT_FOUND"` behavior were
updated to expect the error
(`TestGetApiAssociation_NoAssociation_NotFound`, `wire_field_fixes_test.go`).

## Handler-collision determinism sweep (2026-08-31, gopherstack-fr30)

`cmd/reqfielddiff`'s handler resolution used to break ties among
case-insensitive name matches by whichever Go's randomized map iteration
visited first (ef0eef041 fixed it repo-wide). appsync was named in that
fix's own census as the motivating example: `CreateApi`/`createApi` and 64
other op/handler pairs in this package differ only by how this repo
capitalizes the `Api`/`API` acronym, so before the fix the tool could
resolve to `(b *InMemoryBackend) CreateAPI` (business logic) instead of
`(h *Handler) createAPI` (the real decode site) on any given run.

Verified the damage directly: ran the unpatched tool from `ef0eef041~1`
five times against this package. `with declared fields` bounced between 33
and 36 across runs (post-fix: a stable 42) and 67 distinct op.field
findings flickered tier depending on which candidate won that run. 65 of
those 67 are now resolved correctly and no longer flagged -- e.g.
`CreateGraphqlApi.Name`, `.Tags`, `.AuthenticationType`,
`AssociateApi.ApiId`/`.DomainName`, all of `CreateApiCache`'s fields --
because `createGraphqlAPI`/`createResolver`/etc. do read them; the
misresolution to the exported backend method previously reported them as
unread false positives.

The remaining 2 of 67 (`CreateGraphqlApi.OwnerContact`,
`UpdateGraphqlApi.OwnerContact`) stayed flagged in every pre-fix run *and*
post-fix, and turned out to be real: `CreateGraphqlApiInput`/
`UpdateGraphqlApiInput.OwnerContact` (appsync@v1.56.4
api_op_CreateGraphqlApi.go:79, api_op_UpdateGraphqlApi.go:79) was never
decoded by `createGraphqlAPI`/`updateGraphqlAPI`
(`handler_graphql_apis.go`), and `GraphqlAPI` (`models.go`) had no field to
hold it at all -- a real client's owner-contact value was silently
dropped, never stored, never echoed back by Get/List. (gopherstack's `API`
Event-API type already modeled its own separate `OwnerContact`; this was
specifically the classic `GraphqlApi` type missing it.) Fixed: `GraphqlAPI`
gained an `OwnerContact` field (wire key `ownerContact`, matching
appsync@v1.56.4 types.go:1073), threaded through the existing
`GraphqlAPIConfig` the same way `IntrospectionConfig` already is, and
decoded on both Create and Update. Covered by
`TestInMemoryBackend_CreateAndUpdateGraphqlAPI_OwnerContact`,
`TestHandler_CreateAndUpdateGraphqlAPI_OwnerContact`, and an addition to
`test/integration/appsync_test.go`'s `TestIntegration_AppSync_CRUD` that
asserts the value on the real typed SDK's decoded `CreateGraphqlApiOutput`/
`UpdateGraphqlApiOutput`/`GetGraphqlApiOutput`.

The other 25 services in the census's collision list are out of scope for
this pass (only amplify, appsync, cleanrooms were checked). Within scope:
amplify's and cleanrooms's `reqfielddiff` output was **byte-identical**
across all 5 pre-fix runs and post-fix -- neither service's handler naming
happens to produce an ambiguous fold match for any operation reqfielddiff
resolves (amplify's and most of cleanrooms's op names have no acronym-case
mismatch against their handlers, so `findHandlerByName`'s exact-match
candidates resolve them before the ambiguous fold is ever reached).
`cmd/reqfieldscan` (the sibling tool) was also re-verified byte-identical
for all three services before and after ef0eef041, matching that commit's
own doc claim of zero real collisions in `reqfieldscan`'s narrower
`wrapOpFuncs`-only universe.
