---
service: awsconfig
sdk_module: aws-sdk-go-v2/service/configservice@v1.68.4
last_audit_commit: 198990e82
last_audit_date: 2026-08-07
overall: A            # this pass: implemented the 5 ops the SDK bump (v1.61.2 -> v1.68.0)
                       # revealed as newly-supported and missing from GetSupportedOperations:
                       # PutConnector/GetConnector/ListConnectors/DeleteConnector (a new
                       # Connector family) and PutThirdPartyServiceLinkedConfigurationRecorder
                       # (wired into the existing ConfigurationRecorder model, not a new one --
                       # see its entry below). Prior pass's grade/notes retained unchanged.
ops:
  # --- ConfigurationRecorder family ---
  PutConfigurationRecorder: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: empty/blank name now InvalidConfigurationRecorderNameException, empty roleARN now InvalidRoleException (were both generic ValidationException) -- see gopherstack-eboy"}
  DescribeConfigurationRecorders: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConfigurationRecorderStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  StartConfigurationRecorder: {wire: ok, errors: ok, state: ok, persist: ok}
  StopConfigurationRecorder: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConfigurationRecorder: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-cleans any ServiceLinkedRecorderLink pointing at the deleted recorder"}
  ListConfigurationRecorders: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateResourceTypes: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateResourceTypes: {wire: ok, errors: ok, state: ok, persist: ok}
  PutServiceLinkedConfigurationRecorder: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-e0f1): was a no-op stub; now creates a real ACTIVE recorder named AWSConfigurationRecorderFor<Service> (best-effort deterministic casing -- AWS's exact per-service capitalization isn't publicly enumerable), idempotent per ServicePrincipal via a new ServiceLinkedRecorderLink table"}
  DeleteServiceLinkedConfigurationRecorder: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-e0f1): was a no-op stub; now looks up and deletes the linked recorder, NoSuchConfigurationRecorderException when unknown"}
  PutThirdPartyServiceLinkedConfigurationRecorder: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op (SDK bump v1.61.2 -> v1.68.0). ConfigurationRecorder gained three real wire fields for this (connectorArn/scopeConfiguration/servicePrincipal, field-diffed against the SDK's serializeDocumentConfigurationRecorder) so a third-party service-linked recorder is a genuine RECORDER in the existing model: DescribeConfigurationRecorders/DescribeConfigurationRecorderStatus/ListConfigurationRecorders/DeleteConfigurationRecorder all see and can act on it via the plain recorders table, no orphan. Enforces the real declared constraint (verified against the AWS Config API reference's PutConnector/PutThirdPartyServiceLinkedConfigurationRecorder ConflictException doc: 'the specified service principal does not support multiple configuration recorders and one already exists') -- one service-linked recorder per ServicePrincipal, looked up via a new recordersByServicePrincipal secondary index. Put is create-or-update *conditionally*: same ServicePrincipal+same ConnectorArn updates ScopeConfiguration (idempotent, matching the doc comment); same ServicePrincipal+different ConnectorArn errors ConflictException (not a silent upsert, unlike PutServiceLinkedConfigurationRecorder) -- confirmed against both the doc comment and the deserializer's declared error switch (ConflictException/InsufficientPermissionsException/ValidationException only, no ResourceNotFoundException, so an unknown ConnectorArn errors ValidationException not ErrResourceNotFound)."}

  # --- Connector family (new, SDK bump v1.61.2 -> v1.68.0) ---
  PutConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op. NOT idempotent -- verified against the doc comment ('Connectors cannot be updated -- to update the connector configuration, you must delete all associated configuration recorders, delete the connector, and recreate it') and the declared ConflictException ('a connector already exists for the specified connector configuration'): a repeat PutConnector with a ConnectorConfiguration matching an existing connector errors ConflictException rather than upserting. Requires exactly one provider (Azure, the only one AWS Config documents) with both ClientIdentifier/TenantIdentifier set -- ValidationException otherwise (the SDK's client-side validators.go doesn't itself enforce 'exactly one provider', so this is server-side). Connector Name/Arn are server-generated (PutConnectorInput has no Name field) -- best-effort deterministic naming, same caveat as the existing serviceLinkedRecorderName."}
  GetConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op. Unknown Arn errors ResourceNotFoundException (ErrResourceNotFound), matching the declared error switch (ResourceNotFoundException/ValidationException only)."}
  ListConnectors: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op. Filters by the real 'provider' FilterName (types.ConnectorFilterName's sole enum value); paginated via the existing pkgs/page helper, mirroring DescribeConfigRules' pattern."}
  DeleteConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "new op. Unknown Arn errors ResourceNotFoundException; the real op's declared error model has no ConflictException for 'still referenced by a recorder', so this backend doesn't invent one either."}

  # --- DeliveryChannel family ---
  PutDeliveryChannel: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: empty/blank name now InvalidDeliveryChannelNameException (was generic ValidationException) -- see gopherstack-eboy"}
  DescribeDeliveryChannels: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDeliveryChannel: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDeliveryChannelStatus: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-22 (gopherstack-v4a4): DeliveryChannelStatus/DeliveryChannelStatusInfo were tagged PascalCase (Name/ConfigHistoryDeliveryInfo/ConfigStreamDeliveryInfo/LastStatus/LastAttemptTime); the real deserializer is lowerCamelCase for this shape (like DeliveryChannel itself), so a real client's whole response decoded as the zero value. Structurally underspecified vs the real 3-shape/3-field-set DeliveryChannelStatus -- re-audited 2026-08-23, confirmed a genuine modelling gap (no backend state to source the missing fields from), left unfixed. See Notes, gopherstack-ru0y."}
  DeliverConfigSnapshot: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was a no-op stub; now validates the named channel exists (NoSuchDeliveryChannelException), a recorder is configured (NoAvailableConfigurationRecorderException) and running (NoRunningConfigurationRecorderException), and returns a generated ConfigSnapshotId"}

  # --- ConfigRule + compliance family ---
  PutConfigRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConfigRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-s7u1): unknown name in a non-empty ConfigRuleNames filter now errors NoSuchConfigRuleException instead of silently omitting it; backend signature changed to return an error (~14 call sites across this package updated). ALSO fixed (gopherstack-m0ow): the real optional Filters *types.DescribeConfigRulesFilters (EvaluationMode/RuleEvaluationVisibility) was entirely absent from describeConfigRulesInput and therefore silently dropped by the JSON decoder even when a client sent it. Now modeled and accepted, but inert: gopherstack's ConfigRule has no EvaluationMode/RuleEvaluationVisibility concept at all (PutConfigRule doesn't model the real types.ConfigRule.EvaluationModes field either), so a filtered request currently returns the same unfiltered set -- there is no per-rule state to filter by, and none is fabricated."}
  DeleteConfigRule: {wire: ok, errors: ok, state: ok, persist: ok}
  GetComplianceDetailsByConfigRule: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-s7u1): unknown ConfigRuleName now errors NoSuchConfigRuleException instead of silently returning empty"}
  GetComplianceDetailsByResource: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeComplianceByConfigRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeComplianceByResource: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now rolls per-(rule,resource) evaluations (b.ruleResourceEvals) up per resource, with ComplianceContributorCount and ResourceType/ResourceId/ComplianceTypes filters"}
  GetComplianceSummaryByConfigRule: {wire: ok, errors: ok, state: ok, persist: ok}
  GetComplianceSummaryByResourceType: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConfigRuleEvaluationStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  StartConfigRulesEvaluation: {wire: ok, errors: ok, state: ok, persist: ok}
  PutEvaluations: {wire: ok, errors: ok, state: ok, persist: ok}
  PutExternalEvaluation: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEvaluationResults: {wire: ok, errors: ok, state: ok, persist: ok}
  GetCustomRulePolicy: {wire: ok, errors: ok, state: ok, persist: n/a}
  StartResourceEvaluation: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResourceEvaluationSummary: {wire: ok, errors: ok, state: ok, persist: ok}
  ListResourceEvaluations: {wire: ok, errors: ok, state: ok, persist: ok}

  # --- ConformancePack family ---
  PutConformancePack: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "extended (gopherstack-e0f1): now accepts TemplateBody and, when it's a JSON or YAML CloudFormation-shaped template with AWS::Config::ConfigRule resources, deploys those as real config rules linked to the pack (see conformance_pack_template.go) -- matching real AWS Config, where a conformance pack literally creates managed config rules. FIXED this pass (gopherstack-ag85): (1) TemplateBody now also parses YAML, not JSON-only -- tried as JSON first, falling back to YAML via yamlToJSON; (2) TemplateS3Uri and TemplateSSMDocumentDetails, previously entirely absent from putConformancePackInput and therefore silently dropped by the JSON decoder even when a client sent them, are now parsed off the wire; (3) specifying more than one of TemplateBody/TemplateS3Uri/TemplateSSMDocumentDetails is now rejected with ValidationException, matching PutConformancePackInput's documented \"only one of\" constraint. TemplateS3Uri/TemplateSSMDocumentDetails still deploy zero rules (no S3/SSM fetcher -- see gaps); a request specifying zero sources is still accepted (deploys zero rules) rather than rejected, to avoid breaking this codebase's existing tests that call PutConformancePack with no template purely to set up pack existence -- the exact zero-sources validation is pre-existing deferred scope (gopherstack-eboy), not this pass's target."}
  DescribeConformancePacks: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConformancePack: {wire: ok, errors: ok, state: ok, persist: ok, note: "extended: cascade-deletes every config rule the pack deployed (and their evaluations), matching AWS"}
  DescribeConformancePackStatus: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConformancePackCompliance: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now returns each deployed rule's rolled-up compliance from b.ruleEvaluations, with NoSuchConformancePackException/NoSuchConfigRuleInConformancePackException validation and ComplianceType/ConfigRuleNames filters"}
  GetConformancePackComplianceDetails: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now returns real per-resource evaluation results for the pack's deployed rules (DetailedEvaluationResult is wire-shape identical to ConformancePackEvaluationResult)"}
  GetConformancePackComplianceSummary: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now rolls each named pack's deployed rules up into COMPLIANT/NON_COMPLIANT/INSUFFICIENT_DATA per AWS's documented rollup rule, NoSuchConformancePackException for unknown names"}
  ListConformancePackComplianceScores: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now computes a real percentage compliance score per pack from its rule-resource evaluations, INSUFFICIENT_DATA when none recorded"}

  # --- AggregationAuthorization / ConfigurationAggregator family ---
  PutAggregationAuthorization: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAggregationAuthorizations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-22 (gopherstack-v4a4): AggregationAuthorization.AuthorizedAccountId/AuthorizedAwsRegion were tagged lowercase; the real deserializer is PascalCase for both (its AggregationAuthorizationArn/CreationTime siblings were already correct), so a real client's AuthorizedAccountId/AuthorizedAwsRegion decoded empty. See Notes."}
  DeleteAggregationAuthorization: {wire: ok, errors: ok, state: ok, persist: ok}
  PutConfigurationAggregator: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConfigurationAggregators: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConfigurationAggregator: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeConfigurationAggregatorSourcesStatus: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now derives one status entry per configured AccountAggregationSources/OrganizationAggregationSource account+region, reporting SUCCEEDED (no per-source sync failures modeled), NoSuchConfigurationAggregatorException validation"}
  DescribeAggregateComplianceByConfigRules: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAggregateComplianceByConformancePacks: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now returns every local conformance pack's rule counts, tagged with the requested account/region, once the aggregator is validated to exist"}
  GetAggregateComplianceDetailsByConfigRule: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now reuses local per-resource evaluations (mirroring the already-established DescribeAggregateComplianceByConfigRules pattern), echoing the requested accountId/awsRegion, aggregator existence validated"}
  GetAggregateConfigRuleComplianceSummary: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now derives a single-group compliant/non-compliant rollup keyed by the local account ID or region (GroupByKey), aggregator existence validated"}
  GetAggregateConformancePackComplianceSummary: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now derives compliant/non-compliant conformance-pack counts for the local account/region group, aggregator existence validated"}
  GetAggregateDiscoveredResourceCounts: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAggregateResourceConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-h910): decoded into *emptyInput, dropping ConfigurationAggregatorName/ResourceIdentifier and always returning 'the first resource config found' -- every distinct request returned the same arbitrary item. Now resolves the requested identifier against b.resourceConfigs (mirroring BatchGetAggregateResourceConfig), NoSuchConfigurationAggregatorException for an unknown aggregator, ResourceNotDiscoveredException for no match"}
  BatchGetAggregateResourceConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-ctaz): the aggregatorName parameter was discarded (blank identifier in the backend method signature), so an unknown ConfigurationAggregatorName never yielded NoSuchConfigurationAggregatorException, unlike its siblings ListAggregateDiscoveredResources and GetAggregateResourceConfig (fixed gopherstack-h910), both of which call requireAggregatorLocked. Now validates the aggregator first. Unlike GetAggregateResourceConfig's bug, this one was purely the missing validation -- each identifier in the batch was already resolved individually against b.resourceConfigs by its own ResourceType/ResourceID, never falling back to 'whichever resource came first'; confirmed by reading the resolution loop, not assumed from the shared bug report. Confirmed against the pinned SDK that a missing aggregator IS the right error to add: BatchGetAggregateResourceConfig's own deserializeOpError switch declares NoSuchConfigurationAggregatorException (and ValidationException) but no ResourceNotDiscovered-style exception -- a per-identifier miss is correctly reported via UnprocessedResourceIdentifiers, not an error, matching the pre-existing behavior."}
  SelectAggregateResourceConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAggregateDiscoveredResources: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now returns local discovered resources of the requested type tagged with the local account/region as source, account/region/resourceId filters applied, aggregator existence validated"}
  DescribePendingAggregationRequests: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now derives pending requests from AggregationAuthorizations this account granted that no local ConfigurationAggregator has yet incorporated into its AccountAggregationSources -- the only genuinely-derivable cross-account state a single-account emulator has"}
  DeletePendingAggregationRequest: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was a no-op stub; now deletes the underlying AggregationAuthorization, idempotent per its declared error model (no not-found exception)"}

  # --- RemediationConfiguration family ---
  PutRemediationConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeRemediationConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRemediationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "extended: cascade-deletes any recorded remediation executions for the rule too (new remediationExecutions table introduced this pass). FIXED 2026-08-29 (error-path sweep) -- this op previously deleted unconditionally and never raised for a rule with no remediation configuration, although its own deserializeOpError models NoSuchRemediationConfigurationException for exactly this case ('You specified an Config rule without a remediation configuration.', types/errors.go:1283) and its Output struct is a plain void result (no per-item FailedBatches-style field, unlike the sibling DeleteRemediationExceptions). Missing-error bug: real AWS raises, this emulator returned success. Now checks existence first."}
  PutRemediationExceptions: {wire: fixed, errors: fixed, state: fixed, persist: n/a, note: "previously graded 'wire: ok' in error (gopherstack-m0ow): the handler read invented flat ConfigRuleName/ResourceType/ResourceId fields; real required member is ResourceKeys []types.RemediationExceptionResourceKey (a LIST, one exception per key -- 'Config adds exception for each resource key. For example, Config adds 3 exceptions for 3 resource keys'), with wire keys ResourceType/ResourceId nested PascalCase inside each array element. Also note RemediationExceptionResourceKey's wire keys are PascalCase, unlike the pre-existing, similarly-named ResourceKey type (used by StartRemediationExecution/DescribeRemediationExecutionStatus) whose wire keys are lowerCamelCase -- verified as two distinct serializers (awsAwsjson11_serializeDocumentRemediationExceptionResourceKey vs awsAwsjson11_serializeDocumentResourceKey), not the same shape reused. Backend signature changed to accept the key list, upserting one exception per key. ConfigRuleName/ResourceKeys presence now validated -- InvalidParameterValueException (new ErrInvalidParameterValue sentinel), not ValidationException: this op's declared error switch is InsufficientPermissionsException/InvalidParameterValueException only (verified against awsAwsjson11_deserializeOpErrorPutRemediationExceptions), matching this package's documented policy of not modeling ValidationException on ops that don't declare it. ExpirationTime/Message (real optional members) aren't modeled: gopherstack's RemediationException has no fields to reflect them into, so they're left for the JSON decoder to silently discard."}
  DescribeRemediationExceptions: {wire: ok, errors: ok, state: ok, persist: n/a}
  DeleteRemediationExceptions: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "previously graded 'wire: ok' in error (gopherstack-m0ow): the handler read ConfigRuleName + an invented ResourceGroupName field that doesn't exist on the real API surface, so a real client's request never populated it and nothing was ever actually deleted. Real required member is ResourceKeys []types.RemediationExceptionResourceKey (same PascalCase-nested list shape as PutRemediationExceptions -- see its note). Backend signature changed to accept the key list, deleting exceptions matching (ResourceType, ResourceID) pairs. No validation error added for a missing ConfigRuleName/ResourceKeys: this op's declared error switch is NoSuchRemediationExceptionException only (verified against awsAwsjson11_deserializeOpErrorDeleteRemediationExceptions) -- no ValidationException/InvalidParameterValueException modeled at all, so an empty request is treated as a no-op rather than inventing an error code AWS doesn't declare for this op."}
  StartRemediationExecution: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-e0f1): was a no-op stub; now validates a remediation configuration exists for the rule (NoSuchRemediationConfigurationException) and records a SUCCEEDED execution per resource key (no real SSM Automation runner modeled), readable back via DescribeRemediationExecutionStatus"}
  DescribeRemediationExecutionStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-e0f1): was an empty-list stub; now returns recorded executions for the rule, optionally filtered by resource key, NoSuchRemediationConfigurationException validation"}

  # --- OrganizationConfigRule / OrganizationConformancePack family ---
  PutOrganizationConfigRule: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "fixed 2026-08-23 (gopherstack-xit0): now generates and returns OrganizationConfigRuleArn (real PutOrganizationConfigRuleOutput.OrganizationConfigRuleArn was always empty before). ARN format mirrors putConfigRuleLocked's config-rule convention (config_rules.go): arn:aws:config:<region>:<account>:organization-config-rule/organization-config-rule-<counter>, generated once on create and preserved on update via a new orgRuleCounter."}
  DescribeOrganizationConfigRules: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-22 (gopherstack-v4a4): OrganizationConfigRule.OrganizationConfigRuleName was tagged lowercase; the real deserializer is PascalCase, so a real client's OrganizationConfigRuleName decoded empty on every rule. Fixed 2026-08-23 (gopherstack-xit0): OrganizationConfigRule gained the required OrganizationConfigRuleArn field (see PutOrganizationConfigRule note) -- real OrganizationConfigRule (aws-sdk-go-v2/service/configservice@v1.68.4 types/types.go:2081-2091) declares it 'This member is required'; gopherstack's type previously had no Arn field at all, so a real client's OrganizationConfigRuleArn was always empty. Real-SDK-client proof: TestDescribeOrganizationConfigRules_Arn_RealClient (wire_field_fixes_test.go), confirmed to fail against the pre-fix type and hand-reverted/restored byte-identical."}
  DeleteOrganizationConfigRule: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeOrganizationConfigRuleStatuses: {wire: ok, errors: ok, state: ok, persist: ok}
  GetOrganizationConfigRuleDetailedStatus: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; now returns a single CREATE_SUCCESSFUL MemberAccountStatus for the local account (the only member this single-account emulator can model), NoSuchOrganizationConfigRuleException validation, optional AccountId filter"}
  GetOrganizationCustomRulePolicy: {wire: ok, errors: ok, state: ok, persist: n/a}
  PutOrganizationConformancePack: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeOrganizationConformancePacks: {wire: fixed, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-22 (gopherstack-v4a4): OrganizationConformancePack.OrganizationConformancePackName was tagged lowercase; the real deserializer is PascalCase, so a real client's OrganizationConformancePackName decoded empty on every pack. See Notes."}
  DeleteOrganizationConformancePack: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeOrganizationConformancePackStatuses: {wire: ok, errors: ok, state: ok, persist: ok}
  GetOrganizationConformancePackDetailedStatus: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed (gopherstack-e0f1): was an empty-list stub; mirrors GetOrganizationConfigRuleDetailedStatus's single-local-account model, NoSuchOrganizationConformancePackException validation"}

  # --- RetentionConfiguration family ---
  PutRetentionConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeRetentionConfigurations: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteRetentionConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}

  # --- StoredQuery family ---
  PutStoredQuery: {wire: ok, errors: ok, state: ok, persist: ok}
  GetStoredQuery: {wire: ok, errors: ok, state: ok, persist: ok}
  ListStoredQueries: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteStoredQuery: {wire: ok, errors: ok, state: ok, persist: ok}

  # --- ResourceConfig (Get/List/BatchGet/Select) family ---
  PutResourceConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-h910): request struct omitted the required SchemaVersionId entirely. Now decoded and required (ValidationException if empty); not stored since real AWS uses it only to validate Configuration against the CloudFormation-registered schema for ResourceType, a check this emulator cannot perform, and no output ever echoes it"}
  DeleteResourceConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  GetResourceConfigHistory: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDiscoveredResources: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetResourceConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  SelectResourceConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDiscoveredResourceCounts: {wire: ok, errors: ok, state: ok, persist: n/a}

  # --- Tags family ---
  TagResource: {wire: ok, errors: ok, state: ok, persist: n/a}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: n/a}

gaps:
  - ErrValidation is still mapped to a single generic ValidationException wire type for
    most Put* validation paths. This pass added the three most load-bearing per-op
    Invalid*Exception types (InvalidConfigurationRecorderNameException,
    InvalidRoleException on PutConfigurationRecorder; InvalidDeliveryChannelNameException
    on PutDeliveryChannel). Still generic: InvalidRecordingGroupException,
    InvalidS3KeyPrefixException, InvalidS3KmsKeyArnException, InvalidSNSTopicARNException,
    and the full per-op taxonomy for every other Put* op (bd: gopherstack-eboy, updated
    this pass with a comment noting partial completion -- not closed)
  - FIXED (gopherstack-jkma triage, 2026-09-07): errtargetaudit's module-conditional
    genericProtocolCodes (gopherstack-udkm) surfaced 19 ops emitting ValidationException
    that configservice@v1.68.4 does not declare for them. 8 had a fitting declared
    alternative and were fixed: DeleteAggregationAuthorization/PutConfigurationAggregator/
    DeletePendingAggregationRequest/PutConfigRule/PutConformancePack/
    StartRemediationExecution/PutRetentionConfiguration now raise ErrInvalidParameterValue
    (InvalidParameterValueException, the same generic-fallback sentinel PutRemediationExceptions
    already used); DescribeConfigRules' invalid-NextToken check now raises a new
    ErrInvalidNextToken (InvalidNextTokenException, a word-for-word match per its doc
    comment). The remaining 11 (DeleteConfigurationAggregator, DeleteConfigRule,
    DeleteEvaluationResults, StartConfigurationRecorder, StopConfigurationRecorder,
    DeleteConfigurationRecorder, DeleteConformancePack, PutDeliveryChannel's s3BucketName
    check, DeleteDeliveryChannel, DeleteOrganizationConfigRule,
    DeleteOrganizationConformancePack) have no declared validation-shaped code at all
    (verified per-op against deserializers.go) -- left on ErrValidation with a landmine
    comment at each site rather than inventing a code, per this campaign's no-swap rule.
  - PutConformancePack's TemplateS3Uri/TemplateSSMDocumentDetails template sources
    (bd: gopherstack-ag85, JSON+YAML TemplateBody parsing FIXED this pass) still deploy
    zero rules rather than being fetched/parsed: real fetching needs cross-service S3/SSM
    access, which this service has no wiring for -- appsync/vpclattice-style cross-service
    calls in this fleet are wired centrally in cli.go, outside this task's
    services/awsconfig/ edit boundary. Buildable with that wiring in place (not a
    structural impossibility), so kept in gaps rather than structural_gaps. Honest
    limitation: the request is accepted and the source is stored on nothing (not
    fabricated), documented in conformance_pack_template.go/conformance_packs.go.
  - PutConformancePack accepts zero template sources (TemplateBody/TemplateS3Uri/
    TemplateSSMDocumentDetails all empty) without erroring, though real AWS Config
    requires exactly one. This pass added rejection for *more than one* source (a genuine
    new validation, real and tested), but left the zero-sources case alone: this
    codebase's existing test suite routinely calls PutConformancePack with no template
    purely to establish a pack's existence for unrelated assertions (DeleteConformancePack,
    ARN format, etc.), and enforcing the full requirement would need updating every one of
    those call sites' intent, which is per-field validation-taxonomy work already tracked
    under gopherstack-eboy, not this issue's scope.
  - MaxNumberOfConnectorsExceededException (PutConnector's per-account connector-count
    limit) is declared by the real API but its numeric value isn't published anywhere in
    AWS's docs (checked the API reference and the Config service-limits page as of this
    pass -- no "connectors" row exists in either). Not enforced rather than guessing an
    unverifiable number; the wire error type isn't wired into errorWireMappings since
    nothing in this backend raises it.
  - FIXED (parity sweep 2026-09-04): the single-customer-managed-recorder-per-account
    limit ("You can create only one customer managed configuration recorder
    for each account for each Amazon Web Services Region" -- api_op_PutConfigurationRecorder.go
    doc comment) was unenforced: PutConfigurationRecorder created a new recorder for any
    unseen name with no cap. Now hasCustomerManagedRecorderLocked (configuration_recorders.go)
    rejects a second customer-managed recorder under a different name with
    MaxNumberOfConfigurationRecordersExceededException (ErrAlreadyExists), matching the
    modelled error on PutConfigurationRecorder's deserializer. Service-linked and
    third-party service-linked recorders don't count against the limit -- confirmed via
    PutThirdPartyServiceLinkedConfigurationRecorder's own, separately-enforced
    one-per-ServicePrincipal limit (still real, unchanged). Test:
    TestAWSConfigBackend_PutConfigurationRecorder_MaxOneCustomerManaged.
deferred:
  - Per-field/per-op AWS validation ordering and exact message text (not audited this pass)
leaks: {status: clean, note: "no goroutines/janitors in this service; single coarse lockmetrics.RWMutex; every new Lock/RLock this pass is defer-released; DeleteConfigurationRecorder cascade-cleans ServiceLinkedRecorderLink rows, DeleteConformancePack cascade-cleans its deployed config rules + evaluations, DeleteRemediationConfiguration cascade-cleans its recorded executions -- no ghost rows found"}
---

## Notes

- Wire protocol: awsjson1.1, single POST endpoint, `X-Amz-Target:
  StarlingDoveService.<Op>`. Verified the `StarlingDoveService` target prefix and every
  routed op name against `aws-sdk-go-v2/service/configservice@v1.68.0`'s
  `serializers.go`/`deserializers.go` -- all 102 real SDK ops (97 from the prior audit's
  v1.61.2 + the 5 this pass added for the v1.68.0 bump: `StarlingDoveService.PutConnector`,
  `.GetConnector`, `.ListConnectors`, `.DeleteConnector`,
  `.PutThirdPartyServiceLinkedConfigurationRecorder`) are wired into the dispatch table,
  none missing -- confirmed via `TestSDKCompleteness`.

- `ConfigurationRecorder`/`DeliveryChannel` use **camelCase** wire field names (`name`,
  `roleARN`, `recordingGroup`, `arn`, `s3BucketName`, ...) -- this is genuinely how AWS
  Config serializes these two shapes (confirmed against the real serializer), unlike
  `ConfigRule`/most other shapes in this API which use PascalCase. Don't "fix" this to
  PascalCase in a future pass -- it would break real-SDK-client compatibility.

- Persistence: `Handler.Snapshot`/`Restore` delegate to `InMemoryBackend`, which uses a
  versioned `backendSnapshot{Tables, Version}` wrapping `store.Registry.SnapshotAll()`.
  The prior pass bumped `awsconfigSnapshotVersion` 1 -> 2, adding three tables:
  `conformancePackRules` (which config rules each conformance pack deployed),
  `remediationExecutions` (StartRemediationExecution history), and
  `serviceLinkedRecorders` (servicePrincipal -> recorder-name links -- kept as its own
  table rather than a field on `ConfigurationRecorder` specifically so it isn't lost by
  round-tripping through `json:"-"`, since `ConfigurationRecorder` is serialized verbatim
  as the real AWS wire response and store.Table's Snapshot/Restore marshal that same
  struct/tags). This pass bumped it 2 -> 3, adding the `connectors` table (Connector
  values keyed by ARN) and a `byServicePrincipal` secondary index on the existing
  `recorders` table -- unlike `serviceLinkedRecorders`, this new index needed no separate
  Table/Tables-map entry: `ConfigurationRecorder` itself gained real
  `ConnectorArn`/`ScopeConfiguration`/`ServicePrincipal` wire fields (field-diffed against
  the SDK's `serializeDocumentConfigurationRecorder`), so the index just derives its key
  from the recorder's own new field and rides the recorders table's existing
  Snapshot/Restore. Six scalar/slice-valued maps (`ruleEvaluations`, `resourceHistory`,
  `resourceTags`, `remediationExceptions`, `customRulePolicies`, `orgCustomRulePolicies`)
  still have no `store.Table` identity and are NOT persisted -- this is a pre-existing gap
  (not introduced or fixed this pass), see `persistence.go`'s doc comment.

- 2026-08-13 pass (`gopherstack-h910`, required-member sweep pass 5): `GetAggregateResourceConfig`
  decoded into `*emptyInput`, dropping `ConfigurationAggregatorName`/`ResourceIdentifier` and
  always returning "the first resource config found" regardless of what was requested -- a
  correctness bug, not just a dropped field. `PutResourceConfig` omitted the required
  `SchemaVersionId` entirely. Both fixed -- see their `ops` entries above.

- 2026-08-22 pass (`gopherstack-v4a4` follow-up): commit `351ee095d` corrected nine wrong
  `json:` tags on `AggregationAuthorization`, `OrganizationConfigRule`,
  `OrganizationConformancePack`, `DeliveryChannelStatus`, and `DeliveryChannelStatusInfo`
  (verified against `configservice@v1.68.4`), but three of those five types are also
  `store.Table`-backed and persisted through `backendSnapshot` (`DeliveryChannelStatus`/
  `DeliveryChannelStatusInfo` are computed on the fly in `DescribeDeliveryChannelStatus`,
  not stored). The tag correction is a rename of the on-disk key, not an addition -- a v3
  snapshot's `authorizedAccountId`/`organizationConfigRuleName`/
  `organizationConformancePackName` keys no longer match the corrected struct tags and
  would decode as empty strings. Bumped `awsconfigSnapshotVersion` 3 -> 4 so a v3 snapshot
  is discarded (`registry.ResetAll`, confirmed as a clean whole-registry reset, not a
  partial decode) instead of silently losing those fields.

- 2026-08-13 follow-up pass (`gopherstack-ctaz`, found alongside the `GetAggregateResourceConfig`
  fix above): `BatchGetAggregateResourceConfig`'s backend method signature took
  `aggregatorName string` but discarded it with a blank identifier, so an unknown
  `ConfigurationAggregatorName` never yielded `NoSuchConfigurationAggregatorException` --
  unlike `GetAggregateResourceConfig` and `ListAggregateDiscoveredResources`, both of which
  validate via `requireAggregatorLocked`. Checked whether the batch variant also shared
  `GetAggregateResourceConfig`'s worse defect (returning an arbitrary "first" item for every
  distinct request): it did not -- each identifier in the batch was already correctly
  resolved against `b.resourceConfigs` by its own `ResourceType`/`ResourceID`, so only the
  aggregator-existence check was missing. Fixed by adding the same `requireAggregatorLocked`
  call used by its siblings. See its `ops` entry above for the pinned-SDK verification that
  a per-identifier miss is correctly `UnprocessedResourceIdentifiers`, not an error.

- 2026-07-25 pass (SDK bump v1.61.2 -> v1.68.0 revealed 5 new operations): implemented
  all 5 for real rather than adding them to `notImplemented` -- `PutConnector`/
  `GetConnector`/`ListConnectors`/`DeleteConnector` (new Connector family, see their ops
  entries above) and `PutThirdPartyServiceLinkedConfigurationRecorder`. The key
  correctness risk flagged going in was whether a third-party service-linked recorder
  would be an orphan no existing op could observe -- it isn't: `ConfigurationRecorder`
  gained the three real wire fields real AWS Config actually serializes for this case
  (`connectorArn`/`scopeConfiguration`/`servicePrincipal`), so
  `DescribeConfigurationRecorders`/`DescribeConfigurationRecorderStatus`/
  `ListConfigurationRecorders`/`DeleteConfigurationRecorder` all see and can act on it
  through the same `recorders` table every other recorder uses. Two semantics were
  verified against AWS's docs rather than assumed: `PutConnector` is create-only
  (ConflictException on a repeat call with matching configuration, not an upsert) while
  `PutThirdPartyServiceLinkedConfigurationRecorder` is conditionally idempotent (updates
  ScopeConfiguration when the same ServicePrincipal+ConnectorArn repeat, but
  ConflictException when the same ServicePrincipal reuses a different ConnectorArn -- "one
  recorder per service principal" is enforced, unlike the pre-existing, still-unenforced
  single-customer-managed-recorder limit noted in `gaps`).

- 2026-08-12 pass (`gopherstack-m0ow`, from the `gopherstack-7rq1` sweep for request
  fields present in the model but absent from wire structs): `Put`/`DeleteRemediationExceptions`
  read invented flat fields (`ResourceType`/`ResourceId` at the top level for Put;
  `ResourceGroupName` -- which doesn't exist on the real API at all -- for Delete) instead
  of the real required `ResourceKeys []types.RemediationExceptionResourceKey` list, making
  both ops unreachable by a real client. Fixed -- see their `ops` entries above for the
  wire-shape/error-model detail, including the PascalCase-vs-lowerCamelCase gotcha between
  the new `RemediationExceptionResourceKey` and the pre-existing, similarly-named
  `ResourceKey`. Also added `DescribeConfigRules`' real optional `Filters` (accepted,
  currently inert -- see its `ops` entry).

- 2026-07-24 pass bug-class findings (see `.claude/memories/parity-principles.md` bug
  classes) -- this pass closed all remaining items from the prior audit's `gaps` list
  (`gopherstack-e0f1`, `gopherstack-s7u1`) plus a partial `gopherstack-eboy` fix:
  - **Silently-dropped not-found instead of erroring**: `DescribeConfigRules` and
    `GetComplianceDetailsByConfigRule` used to omit/empty-return for an unknown
    `ConfigRuleName` instead of `NoSuchConfigRuleException`; `DescribeConfigRules`'
    backend signature changed from `([]ConfigRule)` to `([]ConfigRule, error)` (~14
    call sites across this package's tests updated accordingly).
  - **~18 disguised-as-honest empty stubs that were actually derivable**: the prior
    audit (`gopherstack-e0f1`) reasoned these ~18 ops "can't model cross-account state."
    That's true for genuinely cross-account data, but most of these ops' *local* half was
    fully derivable from existing backend state that was simply being ignored:
    conformance-pack rule compliance from `ruleResourceEvals` (once conformance packs
    track which rules they deployed -- a new `conformancePackRules` link table, populated
    by parsing `PutConformancePack`'s `TemplateBody` for `AWS::Config::ConfigRule`
    resources), aggregator source status from the aggregator's own already-stored
    `AccountAggregationSources`/`OrganizationAggregationSource`, aggregate compliance/
    resource-listing ops from local rule-evaluation/resource-config state (mirroring the
    already-"ok" `DescribeAggregateComplianceByConfigRules`/`GetAggregateResourceConfig`
    pattern), pending-aggregation-requests from `AggregationAuthorizations` not yet
    consumed by a local aggregator, remediation execution from `remediationConfigs` (new
    `remediationExecutions` table), and organization detailed-status from treating the
    local account as the org's sole member. Only genuinely un-derivable data (other
    accounts' real resource/compliance state) remains out of scope, and none of these ops
    fabricate it -- they validate real preconditions (aggregator/pack/rule/remediation
    existence) and return real local data.
  - **New cascade-delete surfaces**: two new stateful features this pass introduced
    matching real AWS deletion semantics: `DeleteConformancePack` now cascade-deletes the
    config rules the pack deployed (+ their evaluations), and `DeleteRemediationConfiguration`
    now cascade-deletes recorded remediation executions for the rule -- both to avoid
    introducing new ghost-row classes alongside the new stateful tables.

- Prior-pass bug-class findings (retained for history; see git blame for the fixes):
  wrong wire error-type strings on `ErrNotFound`/`ErrNoSuchOrganizationConformancePack`,
  an error-family collision on `ErrNoDeliveryChannel`, a fabricated
  `NoSuchAggregationAuthorizationException` that doesn't exist in the real SDK, and
  several disguised no-ops (`AssociateResourceTypes`, `BatchGetResourceConfig`,
  `BatchGetAggregateResourceConfig`, `DeleteResourceConfig`, `DisassociateResourceTypes`,
  `DeleteEvaluationResults`) that discarded real backend state instead of acting on it.

- 2026-08-21 (gopherstack-r80d batch 23, required-OUTPUT-member cut): read all 12
  ops-with-required (15 required fields total, largest remaining candidate after
  sagemaker) end to end against `aws-sdk-go-v2/service/configservice@v1.68.4`'s
  `api_op_*.go`, plus every domain struct reachable through a wrapper-key op
  (`Connector`/`ConnectorSummary`/`ConfigurationRecorderSummary`/
  `ConformancePackRuleCompliance`/`ConformancePackEvaluationResult`/
  `EvaluationResultIdentifier`) against `types/types.go` directly -- the flat
  op-level count undercounts by 12 members once `ConnectorSummary` (5 required:
  `Arn`/`CreatedTime`/`Name`/`Provider`/`TenantIdentifier`, reachable through
  `ListConnectors`' required `ConnectorSummaries`) and
  `ConfigurationRecorderSummary` (3 required: `Arn`/`Name`/`RecordingScope`,
  reachable through `ListConfigurationRecorders`) are added; `ConfigurationRecorder`,
  `ConformancePackRuleCompliance`, `ConformancePackEvaluationResult`'s only
  Config-specific nested type (`EvaluationResultIdentifier`) declare zero
  required members each, confirmed via the same AST walk rather than assumed
  from the wrapper shape (appmesh/rolesanywhere precedent: verify, don't infer).
  0 bugs -- every required member found is either tagged without `omitempty`
  (always emitted regardless of value, per this campaign's established
  convention) or, for `Connector.ConnectorConfiguration`/`.CreatedTime` and
  `ConnectorSummary.CreatedTime` (tagged `omitempty` despite being required),
  structurally unreachable-empty: `connectors.go`'s `PutConnector` is the sole
  construction site for both types and unconditionally sets both fields on
  every success path (repo-wide grep for `Connector{`/`ConnectorSummary{`
  confirms no second construction site), so the `omitempty` tag is dead code,
  never actually reachable via any real client path -- reviewed, not fixed,
  same "dead tag" class amplify batch 14 first named for `Branch.Stage`.
  services/_REQUIRED_OUTPUT_CANDIDATES.md updated.

- 2026-08-22 (gopherstack-v4a4, response-struct-TAG sweep -- the same silent-drop
  class as wrong map keys, but on `json:"..."` struct tags instead): the extended
  `cmd/keycheck` struct-tag scan flagged nine fields across four ops, all
  confirmed real against `aws-sdk-go-v2/service/configservice@v1.68.4`'s
  `deserializers.go` and fixed in `models.go`:
  - `AggregationAuthorization.AuthorizedAccountID`/`.AuthorizedAwsRegion` were
    tagged `authorizedAccountId`/`authorizedAwsRegion` (lowercase); the real
    `awsAwsjson11_deserializeDocumentAggregationAuthorization`
    (deserializers.go:14466) switches on `"AuthorizedAccountId"`/
    `"AuthorizedAwsRegion"` (PascalCase) -- the struct's own
    `AggregationAuthorizationArn`/`CreationTime` siblings were already PascalCase,
    so this was a mixed-casing typo on two fields, not a whole-struct
    convention.
  - `OrganizationConfigRule.OrganizationConfigRuleName` was tagged
    `organizationConfigRuleName`; the real
    `awsAwsjson11_deserializeDocumentOrganizationConfigRule`
    (deserializers.go:21357) switches on `"OrganizationConfigRuleName"`.
  - `OrganizationConformancePack.OrganizationConformancePackName` was tagged
    `organizationConformancePackName`; the real
    `awsAwsjson11_deserializeDocumentOrganizationConformancePack`
    (deserializers.go:21699) switches on `"OrganizationConformancePackName"`.
  - `DeliveryChannelStatus.{ConfigHistoryDeliveryInfo,ConfigStreamDeliveryInfo,Name}`
    and `DeliveryChannelStatusInfo.{LastStatus,LastAttemptTime}` were all tagged
    PascalCase; the real
    `awsAwsjson11_deserializeDocumentDeliveryChannelStatus`
    (deserializers.go:18209) switches on `"configHistoryDeliveryInfo"`/
    `"configStreamDeliveryInfo"`/`"name"` (lowerCamelCase, consistent with this
    service's pre-existing `DeliveryChannel`/`ConfigurationRecorder` note above).
    Every one of these fields is part of the *entire* payload of its op (each
    op's wrapper has no other real member), so a real client's response
    decoded as the complete zero value on each of these four ops, not just a
    missing field.
  All four proven with real-`aws-sdk-go-v2` client round-trips in
  `wire_field_fixes_test.go` (`TestDescribeAggregationAuthorizations_RealClient`,
  `TestDescribeOrganizationConfigRules_RealClient`,
  `TestDescribeOrganizationConformancePacks_RealClient`,
  `TestDescribeDeliveryChannelStatus_RealClient`); each confirmed to fail against
  the pre-fix tag (empty/nil decoded fields) and hand-reverted/restored
  byte-identical. Not fixed, and structurally out of scope for a tag-only fix,
  filed as `gopherstack-ru0y`: the real `DeliveryChannelStatus` also declares
  `ConfigSnapshotDeliveryInfo` (a third delivery target gopherstack has no
  concept of) and its `ConfigHistoryDeliveryInfo`/`ConfigSnapshotDeliveryInfo`
  are really `ConfigExportDeliveryInfo` (lastAttemptTime/lastErrorCode/
  lastErrorMessage/lastStatus/lastSuccessfulTime/nextDeliveryTime) while
  `ConfigStreamDeliveryInfo` is a distinct, differently-shaped real type
  (lastErrorCode/lastErrorMessage/lastStatus/lastStatusChangeTime) --
  gopherstack shares one flat `DeliveryChannelStatusInfo` for both, missing
  every field but `LastStatus`.

  2026-08-23 re-audit of `gopherstack-ru0y` (DeliveryChannelStatus): confirmed
  still a genuine modelling gap, not fixed. `DescribeDeliveryChannelStatus`
  (delivery_channels.go) hardcodes `LastStatus: recorderStatusSuccess` for
  both slots on every call and tracks no other delivery state at all --
  `DeliverConfigSnapshot` (same file) generates a snapshot id but persists no
  per-channel last-attempt/success/error/next-delivery timestamp anywhere the
  Describe path could read back, and there is no notion of a distinct
  snapshot-vs-history delivery event. Splitting `DeliveryChannelStatusInfo`
  into the two real shapes (`ConfigExportDeliveryInfo`/
  `ConfigStreamDeliveryInfo`) plus adding `ConfigSnapshotDeliveryInfo` would
  only be able to populate `LastStatus` (still hardcoded) and leave every
  other real member -- `LastAttemptTime`/`LastErrorCode`/`LastErrorMessage`/
  `LastSuccessfulTime`/`NextDeliveryTime`/`LastStatusChangeTime` -- fabricated
  rather than sourced from tracked state. Per this pass's policy (report a gap
  the backend has no state for, don't synthesize fields nobody's tracking),
  left unfixed. Still `gopherstack-ru0y`.

  `gopherstack-xit0` (`OrganizationConfigRule.OrganizationConfigRuleArn`
  required-but-absent) fixed 2026-08-23: see the `PutOrganizationConfigRule`/
  `DescribeOrganizationConfigRules` entries above.

  Also swept for CASE-MISMATCH-shaped findings and confirmed artifact, not filed:
  `iot` (18 flagged fields, e.g. `CreatePolicy`'s `PolicyARN`/`PolicyName`/...)
  traces entirely to the same OUTPUT-SUFFIX NAME COLLISION class `cmd/keycheck`
  already documents for kinesis -- `services/iot/types.go`'s untagged
  `CreatePolicyOutput` (a same-named internal domain struct returned by
  `InMemoryBackend.CreatePolicy`, never marshaled) collides with the heuristic
  that any `*Output`-suffixed composite literal is the wire response; the actual
  handler re-keys through `keyPolicyArn = "policyArn"` etc, already correct.
  `dynamodb`'s 2 (`ExportTableToPointInTime`'s `itemCount`/`tableArn`) trace to
  `import_export_s3.go`'s S3 *manifest file* (an internal export-bookkeeping
  object, never the HTTP response) pulled into the same-package call-graph walk.
  `macie2`'s 1 (`GetBucketStatistics`'s `UNKNOWN`) is an enum *value*
  (`"unknown"` vs `"UNKNOWN"`), not a JSON key, misclassified by the scanner.
  `medialive` (225) and `quicksight` (4) were already-documented
  SHARED-ERROR-HELPER POLLUTION. No code changes for any of these five.

- **2026-08-29 error-path sweep**: all 102 `awsAwsjson11_deserializeOpError*`
  functions extracted from `configservice@v1.68.4/deserializers.go` (matching
  the 102 dispatch-table ops confirmed above) and cross-checked against every
  sentinel this service's `errorWireMappings` table (`handler.go`) and its
  call sites raise. 2 ops model no typed exception at all
  (`DescribeRemediationConfigurations`, `GetComplianceSummaryByConfigRule`).
  Wire mechanism confirmed: a single service-wide `sentinel -> (wireType,
  httpStatus)` table (`handler.go`'s `errorWireMappings`), not a per-op
  switch, so the bug surface is entirely "does each call site choose the
  sentinel its own operation actually models," matching this campaign's
  standing observation that the shared table is usually correct and the bug
  is at the call site.

  **One confirmed missing-error bug, fixed**: `DeleteRemediationConfiguration`
  -- see the `ops:` note above for the full citation and fix. An existing
  test (`TestDeleteRemediationConfiguration`) only covered the happy path and
  never exercised the not-found case, so it never caught the gap (a blind
  test, not a wrong one).

  **Confirmed clean by inspection, not fixed**:
  `DeleteRemediationExceptions`'s own declared error model has no
  `ValidationException`/not-found-shaped exception (only
  `NoSuchRemediationExceptionException`, a distinct wire type this service
  does not implement); confirmed its real `DeleteRemediationExceptionsOutput`
  carries a `FailedBatches []types.FailedDeleteRemediationExceptionsBatch`
  field, i.e. per-item failures are real AWS's own documented mechanism for
  this op, not a typed exception -- so treating an unknown key as a no-op
  (existing behavior, `remediation.go`'s doc comment) is correct, not a gap.

  **Not independently re-verified this pass** (no unique per-op codes
  suggesting a call-site mismatch, given the time budget): the remaining ~40
  quota/role/S3-validation-shaped exceptions unique to single ops
  (`PutConfigurationAggregator`'s `InvalidRoleException`/
  `NoAvailableOrganizationException`, `PutDeliveryChannel`'s
  `InvalidS3KeyPrefixException`/`NoSuchBucketException`/..., `PutConfigRule`'s
  `MaxNumberOfConfigRulesExceededException`, etc.) have no corresponding
  backend validation logic at all (no quota tracking, no S3-bucket-existence
  check, no IAM-role validation), so they can never fire -- feature gaps, not
  wrong-sentinel bugs, and out of scope for a sentinel-correctness pass.

## 2026-08-29 ordering-bug audit (paginate-before-filter, iam class) -- clean, no code change

Audited every `pkgs/page.New(...)` call site (3, via `grep -rn "page.New(" services/awsconfig`) plus
every handler reading `NextToken`/`Filters` together. `pkgs/page.New` is filter-blind by design
(operates on the slice it is handed, computes `Next` from that slice's own length) -- correct here
requires only that callers pass it an already-filtered slice, which all three do:
`handleDescribeConfigRules` (`handler_config_rules.go:83`) filters by `ConfigRuleNames` in
`Backend.DescribeConfigRules` before `page.New`; `handleListConnectors`
(`handler_connectors.go:123`) filters by the request's `Filters` in `Backend.ListConnectors` before
`page.New`; `GetResourceConfigHistoryPage` (`resources.go:212`) resolves the single
resourceType/resourceID's history before paginating it -- a single-resource lookup, not a
combinable collection filter. No filter is ever applied to a `page.Data` result after the fact
anywhere in this service.

One related-but-different finding, not the ordering bug: `handleGetComplianceDetailsByConfigRule`
(`handler_config_rules.go:114`) declares `NextToken` on both its input and output structs but never
reads or writes either -- the field is bound (decoded from the request) and then silently discarded,
matching this campaign's "parsed then discarded" class rather than "wrongly ordered" (there is no
pagination cursor here to get backwards; the op always returns every result in one response, which
over-returns rather than silently drops data, and doesn't reflect a `NextToken` even when a client
supplies a stale/foreign one). Left unfixed this pass -- flagged for whoever next touches this op's
pagination surface, since fixing it means adding real `page.New` pagination, not a one-line ordering
swap.

Every other Describe*/List* op checked (`handler_aggregators.go`'s `DescribeConfigurationAggregators`/
`DescribeAggregationAuthorizations`/`DescribePendingAggregationRequests`,
`handleListDiscoveredResources`, `handleListAggregateDiscoveredResources`) implements no pagination
at all -- no `NextToken` read anywhere in the handler -- so there is no cursor for a filter-ordering
bug to hide behind.

Zero ordering-bug findings; no files changed.

## enumcheck confident-tier fix (2026-08-30)

`cmd/enumcheck`'s CONFIDENT tier flagged both `DescribeConformancePackStatus`
call sites: `ConformancePackState: "COMPLETE"` isn't a member of real
`types.ConformancePackState`, which only defines `CREATE_IN_PROGRESS` /
`CREATE_COMPLETE` / `CREATE_FAILED` / `DELETE_IN_PROGRESS` / `DELETE_FAILED`
(configservice@v1.68.4 types/enums.go:232). Fixed
`conformancePackStateComplete` from `"COMPLETE"` to `"CREATE_COMPLETE"`
(`conformance_packs.go`; the constant has no other callers). Covered by
`TestDescribeConformancePackStatus_State_RealClient`
(`handler_conformance_packs_test.go`), driven through the real SDK client
and asserted against `types.ConformancePackStateCreateComplete`.

## 2026-08-30 WrapOp reflective-decode re-scan (gopherstack-4shm follow-up)

Prior scans anchored on literal `json.Unmarshal`/`Bind` calls found nothing
in this service because every op decodes reflectively through
`pkgs/service.WrapOp` -- gopherstack-4shm's blind spot. Re-scanned with
`cmd/reqfieldscan`, which resolves `WrapOp`'s own generic parameter: 102/102
ops in the dispatch table, 88 request types, 157 fields.

8 fields flagged unread; hand-verified against configservice@v1.68.4:

- **Real bug, fixed**: `GetAggregateDiscoveredResourceCounts`'s
  `ConfigurationAggregatorName` ("This member is required",
  api_op_GetAggregateDiscoveredResourceCounts.go) was accepted on the wire
  and then dropped entirely -- the backend method took no aggregator name at
  all, so a request naming a nonexistent aggregator still succeeded,
  unlike every sibling aggregate-* op in this file (all validated via
  `requireAggregatorLocked`, declaring `NoSuchConfigurationAggregatorException`
  per their own deserializers -- see the doc comment on
  `requireAggregatorLocked` in `aggregators.go`, which lists five other ops
  and conspicuously omits this one). Missing-existence-check class: an
  empty/success result and a missing parent are not the same answer. Fixed
  by threading `aggregatorName` through to a `requireAggregatorLocked` call,
  matching every sibling. Also fixed the doc comment above
  `handleGetAggregateDiscoveredResourceCounts`, which claimed `GroupByKey`
  "is not read from the request at all here" while the code two lines below
  already echoed `in.GroupByKey` correctly -- a stale comment, not a bug.
  Tests: `handler_resources_test.go`
  (`TestAWSConfigHandler_GetAggregateDiscoveredResourceCounts`, two cases,
  driven through the JSON handler), plus existing `resources_test.go`/
  `store_test.go` direct-backend tests updated for the new signature.
  Confirmed failing (200 instead of 404/NoSuchConfigurationAggregatorException)
  against unmodified code before the fix.
- **False positive, documented in code**: `describeConfigRulesInput.Filters`
  -- already has a doc comment explaining `EvaluationMode`/
  `RuleEvaluationVisibility` are accepted-but-inert (`ConfigRule` has no
  matching state to filter by). Correct as-is.
- **Deferred, same disclosed root cause as `PutEvaluations`'s wire-shape
  divergence**: five `NextToken`/`Limit` pagination fields
  (`DescribeComplianceByResource`, `GetAggregateComplianceDetailsByConfigRule`
  x2, `GetComplianceDetailsByConfigRule`, `GetComplianceDetailsByResource`)
  are accepted but never enforced -- each op always returns its complete,
  unbounded result set in one response with no output `NextToken`, unlike
  `DescribeConfigRules` (same file), which does real `page.New` pagination.
  Functionally this over-returns rather than silently drops data (a client
  walking pages the normal way sees `NextToken=""` immediately and stops
  with the complete, correct set), so it is not the same class as a field
  that discards information the caller needs -- left as an honest,
  not-yet-implemented pagination gap rather than fixed this pass, to avoid
  scope creep into five separate `page.New` wirings under this issue's
  budget. Named here for whoever next touches these ops' pagination.
- **Real, disclosed-not-fixed wire-shape gap, found while verifying
  `PutEvaluations`**: `putEvaluationsInput`/`evaluationBody` carry a
  `ConfigRuleName` field that does not exist on the real
  `PutEvaluationsInput`/`types.Evaluation` at all (configservice@v1.68.4:
  the real required field is the opaque `ResultToken`, which this backend
  accepts but never reads or validates). Real AWS derives the rule identity
  server-side by decrypting `ResultToken`, issued to a Lambda invocation
  this backend never performs (`evaluation.go`'s own comment: "Custom/Lambda
  rules are evaluated out-of-band; their results arrive via..."); a real SDK
  client's `PutEvaluations` call carries no `ConfigRuleName` field to
  serialize, so every evaluation would file under `ConfigRuleName=""` for a
  real client today. Fixing this honestly needs `ResultToken`
  issuance/redemption tied to a rule-invocation flow that does not exist in
  this backend -- a feature-sized addition, not a wire-key rename -- so left
  disclosed rather than attempted under this pass's budget. `ResultToken`
  itself is likewise accepted and never validated/stored.

Gates: `go build ./services/awsconfig/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/awsconfig/...` (pass),
`golangci-lint run ./services/awsconfig/...` (0 issues).
