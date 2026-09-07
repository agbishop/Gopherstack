service: sagemaker
sdk_module: aws-sdk-go-v2/service/sagemaker@v1.263.2   # version audited against (parity-5)
last_audit_commit: 5f91d37c7                            # HEAD when this manifest was written
last_audit_date: 2026-08-08
                       # parity-6: fixed the actual gopherstack-e39w gap. parity-5's own note
                       # ("AutoMLJobInputDataConfig ... does not exist anywhere in
                       # aws-sdk-go-v2/service/sagemaker") was wrong — it IS real, it's the
                       # required field on CreateAutoMLJobV2Input (types.go / api_op_CreateAutoMLJobV2.go:91,
                       # []types.AutoMLJobChannel), not CreateAutoMLJobInput (V1). CreateAutoMLJobV2/
                       # DescribeAutoMLJobV2 were routed to the V1 handlers (handler_catalog.go),
                       # so V2's required AutoMLJobInputDataConfig/AutoMLProblemTypeConfig were
                       # silently dropped on every V2 request. Both ops now have dedicated
                       # handlers/wire shapes (handler_automl_v2.go/automl_v2.go); V1's own
                       # handleDescribeAutoMLJob was also changed to build an explicit response
                       # map instead of json.Marshal-ing the shared AutoMLJob struct directly,
                       # since the struct now carries V2-only fields too and would otherwise leak
                       # them into V1 responses for a job created via V2. See Notes: parity-6.
overall: A            # parity-4: implemented the 22 ops the v1.236.0 -> v1.261.0 SDK bump added
                       # (AIBenchmarkJob, AIRecommendationJob, AIWorkloadConfig, generic Job/
                       # JobSchemaVersion, StartClusterHealthCheck families — see Notes). No
                       # previously-audited op touched or regressed. Grade held at A: every new op
                       # is real (routed, stateful, persisted, correct required/optional wire
                       # fields, accurate ResourceNotFound/ResourceInUse error typing verified
                       # against deserializers.go) with clearly-scoped, disclosed depth limits
                       # (see the aiBenchmarkJob/aiRecommendationJob/aiWorkloadConfig/job families
                       # below and gaps:) rather than any invented field or silent stub.
                       # parity-5: wire-audited the 8 families parity-4 left fully deferred +
                       # AutoMLJobInputDataConfig (renamed to the real field, InputDataConfig).
                       # 8 class-a accept-and-drop bugs fixed across
                       # pipeline/experiment/trial/trial-component/feature-group/labeling-job/
                       # automl/inference-recommendations-job (see Notes: parity-5). hub_hub_content
                       # and lineage_action_artifact_context_association audited clean, no bug found.
                       # Grade held at A: every fix is real (routed, stateful, persisted, tested
                       # against a real JSON request body); every remaining gap (feature_store's
                       # online/offline store config, cluster's Orchestrator/AutoScaling/etc.,
                       # PipelineDefinitionS3Location) is disclosed below, not silently absent.

# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateModel: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeModel: {wire: ok, errors: ok, state: ok, persist: ok}
  ListModels: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteModel: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateEndpointConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEndpointConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  ListEndpointConfigs: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEndpointConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-19: added missing DeploymentConfig (json.RawMessage passthrough, echoed as LastDeploymentConfig on Describe)"}
  DescribeEndpoint: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-19: added AsyncInferenceConfig/DataCaptureConfig/ShadowProductionVariants/LastDeploymentConfig, all previously absent"}
  ListEndpoints: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-19: was NextToken-only; added CreationTimeAfter/Before, LastModifiedTimeAfter/Before, MaxResults, NameContains, SortBy, SortOrder, StatusEquals"}
  UpdateEndpoint: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "parity-19: RetainAllVariantProperties/ExcludeRetainedVariantProperties/RetainDeploymentConfig were entirely absent — Desired* was ALWAYS taken from the new EndpointConfig regardless of client intent; now real"}
  UpdateEndpointWeightsAndCapacities: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEndpoint: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateTrainingJob: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTrainingJob: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTrainingJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-20: was sorted ascending-by-name unconditionally, contradicting the doc's real default (CreationTime/Ascending); added LastModifiedTimeAfter/Before, SortBy(Name/Status/CreationTime), SortOrder, TrainingPlanArnEquals/WarmPoolStatusEquals (both decoded and disclosed as always-no-match — this backend never associates a job with a training plan or warm-pool status, so no job can ever match a non-empty value of either)"}
  StopTrainingJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "routes to FSM (InProgress->Stopping->Stopped)"}
  DeleteTrainingJob: {wire: ok, errors: ok, state: ok, persist: ok}
  AddTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "covers models/endpoints/endpoint-configs/training jobs/notebooks/HPO jobs/processing/transform/clusters/domains/feature-groups/pipelines/experiments/trials/trial-components/actions/algorithms/model-packages/associations"}
  ListTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "paginated via offset NextToken, sagemakerDefaultPageSize=100"}
  DeleteTags: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateHyperParameterTuningJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass — see Notes. parity-20: Autotune/WarmStartConfig/TrainingJobDefinition/TrainingJobDefinitions were entirely absent from decode; HyperParameterTuningJobObjective/ParameterRanges/RandomSeed/StrategyConfig/TrainingJobEarlyStoppingType/TuningJobCompletionCriteria (all real optional HyperParameterTuningJobConfig sub-fields) were silently dropped by the flat Strategy/ResourceLimits-only decode. Now the full config is captured as json.RawMessage passthrough (Strategy/ResourceLimits also kept typed for internal filter/sort use) and Autotune/WarmStartConfig/TrainingJobDefinition(s) are accepted."}
  DescribeHyperParameterTuningJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass — HyperParameterTuningJobConfig now nested correctly, ObjectiveStatusCounters/TrainingJobStatusCounters (both required) now always emitted. parity-20: HyperParameterTuningJobConfig response is now the full raw config verbatim (previously only Strategy/ResourceLimits were reconstructed, silently dropping every other sub-field the client sent); Autotune/WarmStartConfig/TrainingJobDefinition/TrainingJobDefinitions now echoed. BestTrainingJob/OverallBestTrainingJob/ConsumedResources/TuningJobCompletionDetails/HyperParameterTuningEndTime disclosed not modeled — this backend never launches or executes child training jobs, so there is no real search result to report."}
  ListHyperParameterTuningJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED this pass — ResourceLimits/ObjectiveStatusCounters/TrainingJobStatusCounters (all required) now always emitted. parity-20: was NextToken-only; added CreationTimeAfter/Before, LastModifiedTimeAfter/Before, MaxResults, NameContains, SortBy(Name/Status/CreationTime, default Name per the op's own doc — unlike most sibling List ops, which default to CreationTime), SortOrder, StatusEquals"}
  StopHyperParameterTuningJob: {wire: ok, errors: ok, state: fixed, persist: ok, note: "parity-20: set status to Stopping and never advanced it — no ticker, no later call. Every stopped job stayed Stopping forever. Third instance of this bug class after parity-15's ClusterSchedulerConfig and parity-19's InferenceComponent. Fixed via a Stopping->Stopped FSM (hpTuningJobStoppingToStopped, 150ms) matching the sibling FSMs already established in lifecycle.go."}
  DeleteHyperParameterTuningJob: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDeviceFleet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — OutputConfig (required) now validated at Create"}
  DescribeDeviceFleet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — see Notes"}
  UpdateDeviceFleet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — OutputConfig is now accepted/persisted (was silently dropped)"}
  CreateModelPackage: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — ModelPackageStatusDetails (required) now always emitted"}
  DescribeModelPackage: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — see Notes"}

  # --- parity-4: new ops added by the v1.236.0 -> v1.261.0 SDK bump ---
  CreateAIBenchmarkJob: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAIBenchmarkJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "required fields (Arn/Name/Status/AIWorkloadConfigIdentifier/BenchmarkTarget/CreationTime/OutputConfig/RoleArn) always emitted; BenchmarkTarget/OutputConfig/NetworkConfig are json.RawMessage passthrough of the Create payload — see aiBenchmarkJob family note"}
  DeleteAIBenchmarkJob: {wire: ok, errors: ok, state: ok, persist: ok}
  StopAIBenchmarkJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "InProgress->Stopping->Stopped FSM via stopSimpleJobFSM (lifecycle.go)"}
  ListAIBenchmarkJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "StatusEquals/NameContains/CreationTimeAfter/Before/SortBy/SortOrder/MaxResults all real filters; AIWorkloadConfigName derived from the stored identifier. parity-21: an unset SortBy/SortOrder fell through to Name/Ascending, the reverse of the op's own doc default (CreationTime/Descending) -- fixed."}
  CreateAIRecommendationJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: AdapterSource was entirely absent from decode -- added as json.RawMessage passthrough, threaded through Create and echoed on Describe."}
  DescribeAIRecommendationJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "required fields always emitted; ModelSource/OutputConfig/PerformanceTarget/ComputeSpec/InferenceSpecification/AdapterSource are json.RawMessage passthrough — see aiRecommendationJob family note; Recommendations intentionally always empty, see gaps:"}
  DeleteAIRecommendationJob: {wire: ok, errors: ok, state: ok, persist: ok}
  StopAIRecommendationJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "InProgress->Stopping->Stopped FSM via stopSimpleJobFSM (lifecycle.go)"}
  ListAIRecommendationJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: same SortBy/SortOrder default bug as ListAIBenchmarkJobs (real default CreationTime/Descending, was falling through to Name/Ascending) -- fixed."}
  CreateAIWorkloadConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAIWorkloadConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "AIWorkloadConfigs/DatasetConfig are json.RawMessage passthrough of the Create payload"}
  DeleteAIWorkloadConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAIWorkloadConfigs: {wire: ok, errors: ok, state: ok, persist: ok}

  # --- parity-21: CompilationJob/AppImageConfig/CodeRepository field audit
  # (previously ungraded -- neither family had ops: entries at all) ---
  CreateCompilationJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: RoleArn/OutputConfig/StoppingCondition (all required) were entirely unvalidated -- a request omitting all three still succeeded; now enforced. ModelPackageVersionArn/VpcConfig were absent from decode entirely -- added as passthrough."}
  DescribeCompilationJob: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: ModelArtifacts (required) and FailureReason were entirely absent from the struct and always nil/missing -- added; ModelArtifacts now populated once a job reaches COMPLETED."}
  DeleteCompilationJob: {wire: ok, errors: ok, state: ok, persist: ok}
  StopCompilationJob: {wire: ok, errors: ok, state: fixed, persist: ok, note: "parity-21: set STOPPED directly with no STOPPING step at all, contradicting the op's own doc (\"changes ... to Stopping. After ... stops the job, it sets ... to Stopped\"). Fixed via a real Stopping->Stopped FSM (compilationJobStoppingToStopped, 150ms)."}
  ListCompilationJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: was NextToken-only -- added CreationTimeAfter/Before, LastModifiedTimeAfter/Before, MaxResults, NameContains, SortBy(Name/CreationTime/LastModifiedTime, default Name), SortOrder (default Ascending, confirmed per-op)."}
  CreateAppImageConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: KernelGatewayImageConfig/JupyterLabAppImageConfig/CodeEditorAppImageConfig (the entire real payload of this resource) were entirely absent from decode -- a client's chosen kernel/container config was silently dropped on every Create. Added as json.RawMessage passthrough."}
  DescribeAppImageConfig: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: the three config fields now echoed (previously never captured, so never present to echo)."}
  UpdateAppImageConfig: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "parity-21: decoded and applied nothing beyond the name -- a client changing an image's kernel/container config had every field silently dropped, the same class as UpdateTrainingJob's prior no-op. Now applies all three config fields."}
  DeleteAppImageConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAppImageConfigs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: was NextToken-only -- added CreationTimeAfter/Before, ModifiedTimeAfter/Before (note the op's own field name -- not LastModifiedTime* like most sibling ops), MaxResults, NameContains, SortBy(Name/CreationTime/LastModifiedTime, default CreationTime), SortOrder (default Descending, confirmed per-op)."}
  CreateCodeRepository: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: GitConfig (required) and its RepositoryUrl (required within it) were entirely unvalidated -- a request omitting GitConfig outright still succeeded."}
  DescribeCodeRepository: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCodeRepository: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "parity-21, most severe finding this pass: Update replaced the entire stored GitConfig map wholesale with whatever the client sent, silently deleting RepositoryUrl/Branch on any real call -- UpdateCodeRepositoryInput.GitConfig is actually types.GitConfigForUpdate, which has only SecretArn (RepositoryUrl/Branch are Create-only/immutable). Fixed to merge only SecretArn, leaving RepositoryUrl/Branch untouched."}
  DeleteCodeRepository: {wire: ok, errors: ok, state: ok, persist: ok}
  ListCodeRepositories: {wire: fixed, errors: ok, state: ok, persist: ok, note: "parity-21: was NextToken-only -- added CreationTimeAfter/Before, LastModifiedTimeAfter/Before, MaxResults, NameContains, SortBy(Name/CreationTime/LastModifiedTime, default Name), SortOrder (default Ascending, confirmed per-op)."}
  CreateJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "JobConfigSchemaVersion validated against jobConfigSchemaVersionsForCategory before create (real ResourceNotFound if unknown)"}
  DescribeJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "required fields (incl. SecondaryStatus/SecondaryStatusTransitions) always emitted; scoped by (JobCategory,JobName) — a category mismatch 404s, see jobs.go doc comment"}
  DeleteJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects a still-InProgress job with ResourceInUse (StopJob required first), matching DeleteJob's doc comment + error deserializer"}
  StopJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "InProgress->Stopping->Stopped FSM with SecondaryStatusTransitions history, distinct JobSecondaryStatusTransition type from TrainingJob's SecondaryStatusTransition"}
  ListJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "scoped to the required JobCategory param plus NameContains/StatusEquals/CreationTime*/LastModifiedTime*/SortBy/SortOrder"}
  DescribeJobSchemaVersion: {wire: ok, errors: ok, state: ok, persist: n/a, note: "real function over a static per-instance schema registry (jobConfigSchemaVersionsForCategory); schema document text itself is a generic synthetic placeholder, not AWS's real unpublished schema — see gaps:"}
  ListJobSchemaVersions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "same registry as DescribeJobSchemaVersion"}
  StartClusterHealthCheck: {wire: ok, errors: ok, state: ok, persist: n/a, note: "validates cluster exists (name or ARN) + DeepHealthCheckConfigurations non-empty, returns ClusterArn; does not synthesize per-node health-check pass/fail results (no fabricated telemetry) — consistent with this service's existing ClusterNode model, which has no health-check fields at all"}

# Families audited as a group (when per-op is impractical):
families:
  ai_benchmark_job: {status: partial, note: "parity-4, new family (CreateJob-era SDK bump). Field-diffed against api_op_{Create,Describe,Delete,Stop,List}AIBenchmarkJob.go + types.AIBenchmarkJobStatus/AIBenchmarkJobSummary/ListAIBenchmarkJobsSortBy — all required/optional Describe fields correct, Stopping/Stopped FSM genuine (real time.Time delay, no fabricated completion metrics). PARTIAL because BenchmarkTarget/OutputConfig/NetworkConfig are stored+echoed as opaque json.RawMessage rather than fully-typed AIBenchmarkTarget/AIBenchmarkOutputConfig/AIBenchmarkNetworkConfig structs (same convention as algorithms.go's TrainingSpecification/InferenceSpecification/ValidationSpecification, already accepted at grade A in this file) — wire-correct for every field a client sent, but AIBenchmarkOutputResult's server-only CloudWatchLogs sub-field is never synthesized."}
  ai_recommendation_job: {status: partial, note: "parity-4, new family. Field-diffed against api_op_{Create,Describe,Delete,Stop,List}AIRecommendationJob.go + types.AIRecommendationJobStatus/AIRecommendationJobSummary. Distinct from the older InferenceRecommendationsJob family (inference_recommendations_jobs.go) — different store, different wire shape, no shared state. PARTIAL for the same json.RawMessage-passthrough reason as ai_benchmark_job (ModelSource/OutputConfig/PerformanceTarget/ComputeSpec/InferenceSpecification), plus Recommendations ([]types.AIRecommendation) is intentionally always empty rather than fabricated — see gaps:."}
  ai_workload_config: {status: partial, note: "parity-4, new family. No status/lifecycle in the real API (DescribeAIWorkloadConfigOutput has no status field) — CRUD only. WorkloadSpec (wire field name AIWorkloadConfigs, confusingly same as the resource-family name)/DatasetConfig stored as json.RawMessage passthrough for the same reason as the two job families above. FIXED parity-24 (gopherstack-oc9v) — DescribeAIWorkloadConfigOutput.Tags ([]types.Tag, a JSON array of {Key,Value} objects) was being serialized from this backend's internal map[string]string Tags field directly, which encodes as a JSON object; a real aws-sdk-go-v2 client's []types.Tag deserializer rejects that outright. AIWorkloadConfig.MarshalJSON now shadows Tags with toTagObjects(c.Tags), proven via a real-SDK-client test. The 4 anonymous request structs in handler_ai_workload_configs.go were also converted to named types this pass."}
  job_and_job_schema_version: {status: partial, note: "parity-4, new generic 'model customization job' family (CreateJob/DescribeJob/DeleteJob/StopJob/ListJobs/DescribeJobSchemaVersion/ListJobSchemaVersions). NOT the same as TrainingJob/ProcessingJob/TransformJob/AutoMLJob/CompilationJob/etc — keyed by JobName alone (matches CreateJob's doc: unique per account+region), Describe/Delete/Stop additionally scoped by JobCategory (mismatch => ResourceNotFound), own JobSecondaryStatusTransition type (does not alias training_jobs.go's SecondaryStatusTransition despite the identical shape), own store (b.jobs). DeleteJob correctly rejects a still-InProgress job with ResourceInUse per its doc comment. PARTIAL: DescribeJobSchemaVersion/ListJobSchemaVersions serve a single synthetic '1.0' schema version with a generic (not per-category, not AWS's real unpublished) JSON-schema document — AWS does not ship real per-JobCategory schema content in the SDK, so this is the most honest deterministic approximation available, not a wire-shape bug, but is disclosed as a depth limit."}
  model_endpoint_config_crud: {status: partial, note: "CreateModel/DescribeModel/ListModels/DeleteModel and CreateEndpointConfig/DescribeEndpointConfig/ListEndpointConfigs/DeleteEndpointConfig verified op-by-op against handler.go + backend.go: correct ARN building via pkgs/arn, epoch timestamps via epochSeconds (float64 unix seconds, matches awsjson1.1 numeric timestamp), errCodeLookup-equivalent sentinel wiring (awserr.New wraps ErrNotFound/ErrConflict, handler.go handleError maps to ValidationException/ResourceInUse), persistence.go backendSnapshot wiring confirmed for both models and endpointConfigs keyed by region. CORRECTION parity-25 (gopherstack-oc9v): the 'ok' verdict above covered ARN/timestamp/error/persistence plumbing only, not full request-field completeness, and did not hold on the request side. FIXED parity-25 — CreateModelInput.ExecutionRoleArn was rejected as required (test TestCreateModel_RequiresExecutionRoleArn asserted this explicitly), but CreateModelInput (api_op_CreateModel.go:50-90) marks only ModelName 'This member is required' — ExecutionRoleArn is optional; a real client's valid model-without-role request was wrongly rejected. Validation removed, test rewritten (TestCreateModel_ExecutionRoleArnOptional) to assert the correct behavior. FIXED parity-25 — ListModelsInput's CreationTimeAfter/CreationTimeBefore/NameContains/SortBy/SortOrder (api_op_ListModels.go) and ListEndpointConfigsInput's identical five fields (api_op_ListEndpointConfigs.go) were NextToken-only, dropping the entire filter/sort surface on both ops; both now real (SortBy default CreationTime/SortOrder default Descending on each, per each op's own doc), proven by TestHandler_ListModels_FilterSort/TestHandler_ListEndpointConfigs_FilterSort. FIXED parity-25 — CreateEndpointConfigInput's ExplainerConfig/MetricsConfig (both real, optional fields) had no field for them anywhere in the request struct at all; now stored+echoed as opaque json.RawMessage passthrough (same convention as algorithms.go's TrainingSpecification), proven via a real-SDK-client round-trip test (TestHandler_CreateEndpointConfig_ExplainerAndMetricsConfig_RealClient). All 6 anonymous request structs across handler_models.go/handler_endpoint_configs.go converted to named types this pass (a single shared nameTimeListRequest/nameTimeFilter serves all three of ListModels/ListEndpointConfigs/ListAlgorithms, since their filter shape is identical). VERIFIED NEGATIVE (gopherstack-tauw, 2026-09-06) — CreateEndpointConfig's ProductionVariants[].ModelName is stored without checking the named Model exists. Confirmed not a bug: awsAwsjson11_deserializeOpErrorCreateEndpointConfig only recognizes UnknownError/ResourceLimitExceeded (no ResourceNotFound), and api_op_CreateEndpointConfig.go's own doc describes model-existence-adjacent validation as happening later, at CreateEndpoint (an eventual-consistency DynamoDB check), not at CreateEndpointConfig. Left unchecked. CreateTransformJob's equivalent ModelName reference IS validated — see processing_transform_job above."}
  endpoint_lifecycle: {status: ok, note: "CreateEndpoint/UpdateEndpoint/DescribeEndpoint/DeleteEndpoint/ListEndpoints + UpdateEndpointWeightsAndCapacities audited and FIXED — see Notes. FSM-driven Creating/Updating -> InService transitions (backend_accuracy.go scheduleEndpointTransition) verified correct after fix."}
  training_job: {status: ok, note: "CreateTrainingJob(Full)/DescribeTrainingJob(Full)/ListTrainingJobs(Filtered)/StopTrainingJob(FSM)/DeleteTrainingJob verified: InProgress->Completed FSM populates ModelArtifacts, BillableTimeInSeconds, SecondaryStatusTransitions with epoch timestamps; StopTrainingJobFSM drives InProgress->Stopping->Stopped. CORRECTION parity-20: the prior 'UpdateTrainingJob verified' claim above was never true — the handler decoded no fields at all and simply re-Described the job, so every UpdateTrainingJob request silently did nothing; this was found and fixed this pass (see Notes) rather than being a pre-existing verified op. FIXED this pass — UpdateTrainingJob now applies ResourceConfig.KeepAlivePeriodInSeconds via a real backend UpdateTrainingJob method (previously did not exist at all); ProfilerConfig/ProfilerRuleConfigurations/RemoteDebugConfig remain disclosed not modeled (no such concept anywhere in this backend's TrainingJob, Create included)."}
  tags: {status: ok, note: "AddTags/ListTags/DeleteTags verified against findTagMapLocked, which indexes ~20 resource kinds by ARN. Not-found path returns ValidationException (400), matching real AWS TagKeys validation error class. CORRECTION parity-25 (gopherstack-oc9v): the 'ok' verdict above covered not-found error mapping only. FIXED parity-25 — AddTagsOutput.Tags ([]types.Tag, 'A list of tags associated with the SageMaker resource') was never emitted; the handler returned a bare `{}` on every AddTags call. Now returns the resource's full current tag set (via a ListTags call after the write), proven by a new assertion in TestHandler_Tags — a pre-existing gap in that same test's coverage (it round-tripped AddTags without ever reading its response body, so nothing caught the missing field). FIXED parity-25 — AddTagsInput.Tags and DeleteTagsInput.TagKeys (both 'This member is required') were accepted with no presence check at all; both now enforced (TestHandler_AddTags_RequiresTags/TestHandler_DeleteTags_RequiresTagKeys). FIXED parity-25 — ListTagsInput.MaxResults (api_op_ListTags.go, default 100) was decoded nowhere; ListTags paginated at a fixed sagemakerDefaultPageSize regardless of what a client requested. Now honored via paginateSlice (TestHandler_ListTags_MaxResults). All 3 anonymous request structs in handler_tags.go converted to named types this pass."}
  algorithm: {status: partial, note: "parity-25 (gopherstack-oc9v), first wire audit of this family — CreateAlgorithm/DescribeAlgorithm/DeleteAlgorithm/ListAlgorithm, field-diffed against api_op_{Create,Describe,Delete,List}Algorithm.go. FIXED — CreateAlgorithmInput.TrainingSpecification is 'This member is required' (alongside AlgorithmName), but only AlgorithmName was ever validated present; a request missing it silently succeeded with an empty spec, and DescribeAlgorithmOutput.TrainingSpecification (itself required on that output) would then be emitted as an empty/absent value. Now enforced, and describeAlgorithmResponse's TrainingSpecification json tag had its incorrect omitempty removed to match. FIXED — ListAlgorithmsInput was NextToken-only, dropping CreationTimeAfter/CreationTimeBefore/NameContains/SortBy/SortOrder entirely (SortBy default CreationTime, SortOrder default Ascending per api_op_ListAlgorithms.go — the one op in this pass's List trio whose real SortOrder default is Ascending, not Descending); all now real, proven by TestHandler_ListAlgorithms_FilterSort. AlgorithmStatusDetails/CreationTime/AlgorithmStatus (all required DescribeAlgorithmOutput fields) were already correctly emitted with no omitempty. TrainingSpecification/InferenceSpecification/ValidationSpecification remain opaque json.RawMessage passthrough (same convention as ai_benchmark_job etc., see gaps:) rather than fully-typed TrainingSpecification/InferenceSpecification/AlgorithmValidationSpecification structs — each is a deep, low-traffic nested type (ChannelSpecification/MetricDefinition/HyperParameterSpecification/...). All 3 anonymous request structs in handler_algorithms.go converted to named types this pass."}
  monitoring_alert: {status: partial, note: "parity-25 (gopherstack-oc9v), first wire audit of this family — UpdateMonitoringAlert/ListMonitoringAlerts/ListMonitoringAlertHistory, field-diffed against api_op_{Update,List}MonitoringAlert*.go. FIXED — UpdateMonitoringAlertInput.DatapointsToAlert/EvaluationPeriod are both 'This member is required', but neither was validated present (a zero/absent value looks identical for a non-pointer int32, so this follows the same == 0 convention as this campaign's other required-int-field fixes, e.g. TransformResources.InstanceCount). FIXED — ListMonitoringAlertsInput.MaxResults (api_op_ListMonitoringAlerts.go, default 100) was decoded nowhere; the backend's sagemakerListKeyPagedMap helper had no maxResults parameter at all, always paging at the fixed sagemakerDefaultPageSize. Both the handler and the helper now thread it through, proven by TestHandler_ListMonitoringAlerts_MaxResults. ListMonitoringAlertHistoryInput's CreationTimeAfter/CreationTimeBefore/MonitoringScheduleName/MonitoringAlertName/StatusEquals/SortOrder/NextToken/MaxResults were already all real — no gap found there. All 3 anonymous request structs in handler_monitoring.go converted to named types this pass (a fourth, ListMonitoringExecutions, was already a named type from an earlier pass)."}
  presigned_session: {status: ok, note: "parity-25 (gopherstack-oc9v), first wire audit of this family — CreatePresignedDomainUrl/RenderUiTemplate/StartSession, field-diffed against api_op_{CreatePresignedDomainUrl,RenderUiTemplate,StartSession}.go. FIXED — RenderUiTemplateInput.Task ('This member is required') and its own required Input field (types.RenderableTask, types/types.go:19548) were accepted with no presence check; an absent Task.Input silently rendered the template unchanged (via the existing empty-string early-return in renderUITemplateContent) rather than being rejected. Now enforced, proven by TestHandler_RenderUiTemplate_MissingTaskInput. StartSessionInput/Output already matched exactly (ResourceIdentifier in; SessionId/StreamUrl/TokenValue out). CreatePresignedDomainUrlInput's ExpiresInSeconds/LandingUri/SessionExpirationDurationInSeconds (real, optional fields) are now decoded (for tooling visibility) but are disclosed no-ops — CreatePresignedDomainUrlOutput is a bare {AuthorizedUrl}, and this backend's synthetic URL (a token appended to the domain's stored URL) carries no verified real query-parameter format to encode an expiry or landing path into, the same disclosed-no-op stance as PartnerApps' identical fields. All 3 anonymous request structs in handler_presigned_session.go converted to named types this pass. CORRECTION parity-29 — the 'now decoded... disclosed no-ops' claim above described this as commented at the code level like PartnerApps' siblings, but createPresignedDomainURLRequest's doc comment carried no such disclosure; added, no behavior change (the fields' inertness was already real, just undocumented in-code — a PARITY.md-vs-code drift, not a functional bug). Also noted for the first time: CreatePresignedDomainUrlInput additionally carries a real SpaceName field (an alternative identity to UserProfileName) not decoded at all; left unmodeled rather than guessed at, since this backend has no Studio Space + shared-space presigned-URL precedent to model it faithfully against (no bd issue filed yet)."}
  processing_transform_job: {status: partial, note: "Wire-audited this pass: DescribeProcessingJob/DescribeTransformJob field-by-field against SDK output structs — field names, optional-field gating, and epoch-seconds timestamps all correct. No bugs found. CORRECTION parity-24 (gopherstack-oc9v): the 'No bugs found' claim above covered only the Describe response shape, not the full request surface, and did not hold there. FIXED parity-24 — CreateProcessingJobInput's VPC settings nest under NetworkConfig.VpcConfig (api_op_CreateProcessingJob.go); this handler instead decoded a top-level \"VpcConfig\" key that does not exist anywhere on the real request, so every real client's VPC-isolated processing job silently lost its network settings (and the accepted top-level key was dead code no real client would ever populate). Now ProcessingNetworkConfig nests VpcConfig/EnableInterContainerTrafficEncryption/EnableNetworkIsolation under NetworkConfig, proven via a real-SDK-client test. CreateProcessingJobInput's RoleArn/AppSpecification/ProcessingResources (all 'This member is required') were also never validated present — fixed. ExperimentConfig (ExperimentName/RunName/TrialComponentDisplayName/TrialName) and StoppingCondition (MaxRuntimeInSeconds) were both accept-and-drop, now fully modeled and round-tripped (both small flat types, no passthrough needed). ListProcessingJobs accepted only NextToken/StatusEquals/MaxResults, dropping CreationTimeAfter/CreationTimeBefore/LastModifiedTimeAfter/LastModifiedTimeBefore/NameContains/SortBy/SortOrder entirely (SortBy default CreationTime, SortOrder default Ascending per api_op_ListProcessingJobs.go) — all now real. FIXED parity-24 — CreateTransformJobInput has no RoleArn field at all (api_op_CreateTransformJob.go:55-166); this handler accepted, stored, and echoed one anyway on every Create/Describe, a fabricated field no real client ever sends. Removed entirely (TransformJob/TransformJobOptions/decode/emit), proven by a test asserting RoleArn is absent from Describe's response even when supplied on Create. ListTransformJobs gained the same CreationTimeAfter/CreationTimeBefore/LastModifiedTimeAfter/LastModifiedTimeBefore/SortBy/SortOrder surface (SortOrder default Descending per that op's doc) it was missing. CreateTransformJobInput's other four required members (ModelName already checked; TransformInput.DataSource.S3DataSource.S3Uri/TransformOutput.S3OutputPath/TransformResources.InstanceType+InstanceCount) were also never validated present — fixed. ProcessingJob's ProcessingInput.DatasetDefinition sub-fields beyond DataDistributionType/InputMode, ProcessingOutput.FeatureStoreOutput, and TransformJob's DataCaptureConfig/DataProcessing/ExperimentConfig/ModelClientConfig/LabelingJobArn/AutoMLJobArn remain accept-and-drop or unmodeled — see gaps:. All 8 anonymous request structs across handler_processing_jobs.go/handler_transform_jobs.go converted to named types this pass. FIXED (gopherstack-tauw, 2026-09-06) — CreateTransformJob's ModelName was stored without checking it names an existing Model. api_op_CreateTransformJob.go: 'ModelName must be the name of an existing Amazon SageMaker model'; ResourceNotFound is modeled for this op (deserializers.go: awsAwsjson11_deserializeOpErrorCreateTransformJob). Now checked against modelsStore before the job is created, returning ResourceNotFound (TestHandler_CreateTransformJob_ModelNotFound). Same issue was also raised against CreateEndpointConfig's ProductionVariants[].ModelName — NOT a bug there: CreateEndpointConfig's deserializer models only UnknownError/ResourceLimitExceeded (no ResourceNotFound), and its own doc describes only an eventual-consistency check at the later CreateEndpoint call, not model existence at CreateEndpointConfig time — so real AWS accepts a ModelName that does not exist yet on this op. Left unchecked; see model_endpoint_config_crud below."}
  notebook_instance: {status: ok, note: "Wire-audited this pass: DescribeNotebookInstanceFull field-by-field against SDK — all optional fields correctly gated, epoch-seconds timestamps correct. No bugs found."}
  hyperparameter_tuning_job: {status: partial, note: "FIXED this pass — see Notes (wire-shape bug: flat Strategy instead of nested HyperParameterTuningJobConfig, missing required ObjectiveStatusCounters/TrainingJobStatusCounters/ResourceLimits). FIXED parity-20 (gopherstack-oc9v) — all 5 inline structs converted to named types; StopHyperParameterTuningJob's Stopping-forever status bug fixed via a real FSM; CreateHyperParameterTuningJob/DescribeHyperParameterTuningJob now capture and echo the full HyperParameterTuningJobConfig (ParameterRanges/HyperParameterTuningJobObjective/RandomSeed/StrategyConfig/TrainingJobEarlyStoppingType/TuningJobCompletionCriteria) plus Autotune/WarmStartConfig/TrainingJobDefinition/TrainingJobDefinitions, all previously entirely absent; ListHyperParameterTuningJobs/ListTrainingJobsForHyperParameterTuningJob gained real filter/sort/pagination (previously NextToken-only / unpaginated). PARTIAL because BestTrainingJob/OverallBestTrainingJob/ConsumedResources/TuningJobCompletionDetails/HyperParameterTuningEndTime and the full semantic content of TrainingJobDefinition(s)/ParameterRanges/StrategyConfig remain json.RawMessage passthrough rather than modeled (this backend never launches or searches child training jobs) — every field a client sends round-trips exactly, but no real hyperparameter search ever runs."}
  domain_app_userprofile_space: {status: partial, note: "Space's Describe/List timestamp encoding FIXED parity-4 (see systemic timestamp bug in Notes). FIXED this pass (parity-7, gopherstack-oc9v) — this family was the largest concentration of anonymous inline request structs in the service (part of the 362 counted repo-wide) and had never been wire-audited; converted all 19 Create/Describe/List/Delete/Update handlers across Domain/App/Space/UserProfile to named types and found real gaps, not just a tooling blind spot. See Notes: parity-7 for the full list; highlights: CreateDomain was missing DefaultUserSettings entirely — a 'This member is required' CreateDomainInput field — so it was silently accepted-and-dropped rather than rejected; CreateApp had no way to create a Space-owned app at all (CreateAppInput.SpaceName, the real alternative to UserProfileName, didn't exist on the wire struct), so any client without a UserProfile could never launch an app even though this backend has supported Spaces since spaces.go; ListDomains/ListApps/ListSpaces/ListUserProfiles all silently ignored MaxResults and had none of ListApps'/ListSpaces'/ListUserProfiles' real SortBy/SortOrder/*Equals/*Contains filter-and-sort fields — the exact 'parsed field, silently ignored' defect class this campaign targets. All now real: MaxResults caps the page via paginateSlice, SortBy/SortOrder reorder by CreationTime/LastModifiedTime, UserProfileNameEquals/SpaceNameEquals/SpaceNameContains/UserProfileNameContains narrow the result set. DefaultSpaceSettings/DomainSettings/DomainSettingsForUpdate/UserSettings/OwnershipSettings/SpaceSettings/SpaceSharingSettings/ResourceSpec are carried as opaque json.RawMessage passthrough (established convention, see ai_workload_configs.go) rather than fully typed — these are all deeply-nested union/config shapes (UserSettings alone has ~20 app-specific sub-configs) out of this pass's budget; every field a client actually sends round-trips exactly. UpdateDomain went from a pure no-op (only bumped LastModifiedTime) to a real partial update of AppNetworkAccessType/AppSecurityGroupManagement/HomeEfsFileSystemCreation/TagPropagation/VpcId/SubnetIds/DefaultUserSettings/DefaultSpaceSettings/DomainSettingsForUpdate. See gaps: for what's still not modeled (DescribeApp/DescribeDomain's remaining server-derived/identity fields, UserSettings' internal structure)."}
  pipeline_pipeline_execution: {status: partial, note: "parity-5, wire-audited op-by-op against api_op_{Create,Update,Delete,Describe,List}Pipeline*.go. FIXED this pass — DescribePipelineExecution silently dropped ParallelismConfiguration even though it was already stored on the backend struct (class-a bug); StartPipelineExecution/DescribePipelineExecution now also accept+echo PipelineVersionId and SelectiveExecutionConfig (previously accepted-and-dropped, both real optional CreateInput/DescribeOutput fields). FIXED this pass (parity-6) — DescribePipeline now accepts the optional PipelineVersionId input (previously ignored, always describing the current version regardless; an unknown version now correctly errors instead of silently returning the current one) and returns LastRunTime (derived as the max StartTime across the pipeline's PipelineExecutions, or omitted if it has never run — a real, not fabricated, value). FIXED this pass (gopherstack-i359, session 2) — CreatePipeline/UpdatePipeline's PipelineDefinitionS3Location (api_op_CreatePipeline.go:59, api_op_UpdatePipeline.go:43) was previously accepted-and-dropped; honoring it for real needed a cross-service S3 GetObject call (out of scope that session — cli.go's S3 wiring was owned elsewhere), so it was rejected explicitly with a ValidationException instead of silently ignored. FIXED for real this pass (gopherstack-i359, session 3) — CreatePipeline/UpdatePipeline now fetch the real object through the backend's wired S3Accessor (services/sagemaker/s3pipeline.go, cli.go's wireSageMakerS3, same registry pattern as wireMGNS3/wireDynamoDBS3) and use its body as PipelineDefinition. The ValidationException path is retained only for the genuinely-unreadable case (no S3 backend wired, or GetObject/read failure against a real bucket/key) — an honest error, not a fabricated definition. Remaining gaps (not fixed, see gaps:): DescribePipeline still omits PipelineVersionDescription/PipelineVersionDisplayName/CreatedBy/LastModifiedBy; ListPipelines summary is missing PipelineDescription/PipelineDisplayName/RoleArn/LastExecutionTime. FIXED parity-24 (gopherstack-oc9v) — ListPipelineVersionsInput's CreatedAfter/CreatedBefore/SortOrder (api_op_ListPipelineVersions.go) were silently dropped, leaving only PipelineName/NextToken/MaxResults; now all real, and the op's 4 handlers (ListPipelineVersions/UpdatePipelineVersion/DescribePipelineDefinitionForExecution/UpdatePipelineExecution, handler_pipeline_versions.go) were converted from anonymous inline structs to named types."}
  experiment_trial_trial_component: {status: partial, note: "parity-5, wire-audited against api_op_{Create,Describe,List}{Experiment,Trial,TrialComponent}.go. FIXED this pass — CreateExperiment/CreateTrial silently dropped DisplayName (and Experiment's Description), both real optional Create fields, so a client-supplied display name never round-tripped through Describe/List until a later Update call; ListExperiments/ListTrials summaries also gained DisplayName/LastModifiedTime (real ExperimentSummary/TrialSummary fields). CreateTrialComponent was the worst finding in this family: it silently dropped StartTime/EndTime/Status/Parameters/InputArtifacts/OutputArtifacts/DisplayName entirely — every field a client actually uses a TrialComponent for — now accepted and stored. Also fixed a genuine wire-shape bug (not accept-and-drop, but same severity class): TrialComponent.Status was serialized as a bare JSON string, but the real DescribeTrialComponentOutput.Status/TrialComponentSummary.Status is a {PrimaryStatus,Message} object (types.TrialComponentStatus) — a real AWS SDK client's JSON deserializer would fail outright on the old shape. The pre-existing TestHandler_UpdateTrialComponent test literally asserted the buggy bare-string shape; updated it to the correct object shape as part of this fix. Not fixed (see gaps:): CreatedBy/LastModifiedBy/Source (UserContext — no identity model to derive from, class d)."}
  feature_store: {status: partial, note: "parity-5, wire-audited CreateFeatureGroup/DescribeFeatureGroup/UpdateFeatureGroup against api_op_{Create,Describe,Update}FeatureGroup.go. FIXED this pass — RoleArn and Description are both real CreateFeatureGroupInput fields (RoleArn is what OfflineStoreConfig replication would use) that were accepted-and-dropped entirely; now stored and returned. FIXED this pass (parity-6) — OnlineStoreConfig/OfflineStoreConfig/ThroughputConfig (CreateFeatureGroupInput/DescribeFeatureGroupOutput) are now fully modeled and round-trip: OnlineStoreConfig (EnableOnlineStore/StorageType/SecurityConfig.KmsKeyId/TtlDuration), OfflineStoreConfig (S3StorageConfig/DataCatalogConfig/TableFormat/DisableGlueTableCreation), ThroughputConfig (ThroughputMode/ProvisionedRead+WriteCapacityUnits — one Go type serves both CreateFeatureGroupInput.ThroughputConfig and DescribeFeatureGroupOutput.ThroughputConfigDescription since their fields are identical). NOT fixed (see gaps:): UpdateFeatureGroup's OnlineStoreConfigUpdate/ThroughputConfigUpdate (a distinct, separate update path from Create's fields, out of this pass's scope); LastUpdateStatus/OfflineStoreStatus/FailureReason/OnlineStoreTotalSizeBytes (DescribeFeatureGroupOutput fields describing async store-creation progress, not modeled); FeatureRecord PutRecord/GetRecord/DeleteRecord/BatchGetRecord (feature_store.go) belong to the separate sagemaker-featurestore-runtime SDK, not the sagemaker control-plane SDK audited here, and were out of scope."}
  feature_metadata: {status: partial, note: "parity-26 (gopherstack-oc9v), first wire audit of this family — DescribeFeatureMetadata/UpdateFeatureMetadata field-diffed against api_op_{Describe,Update}FeatureMetadata.go. FIXED — UpdateFeatureMetadataInput.ParameterRemovals ([]string, real, optional) was absent from decode entirely; a real client removing a parameter key had the removal silently dropped (accept-and-drop). Now threaded through UpdateFeatureMetadata (feature_store.go) and deleted from the stored map, proven by TestHandler_UpdateFeatureMetadata_ParameterRemovals. FIXED — DescribeFeatureMetadataOutput.LastModifiedTime ('This member is required') was hardcoded to the owning feature group's CreationTime on every call, never advancing — handleDescribeFeatureMetadata emitted epochSeconds(fg.CreationTime) unconditionally rather than the metadata's own last-modified time. FeatureMetadata gained a LastModifiedTime field, set by UpdateFeatureMetadata on every successful call; Describe now falls back to the group's CreationTime only for a feature never updated (matches real AWS: a feature's metadata timestamp starts at group creation). Proven by TestBackend_FeatureMetadata_LastModifiedTimeAdvances (asserted on the backend's time.Time field directly, not through the wire's epochSeconds truncation, since two calls in one test can land in the same whole second) and TestHandler_DescribeFeatureMetadata_LastModifiedTimeDefaultsToGroupCreation. Both anonymous request structs in handler_feature_metadata.go converted to named types this pass."}
  model_package_model_package_group: {status: partial, note: "FIXED this pass — ModelPackage was missing the required ModelPackageStatusDetails field entirely (see Notes); ModelPackage/ModelPackageGroup Describe+List timestamp encoding also fixed. Other model-package fields (InferenceSpecification, SourceAlgorithmSpecification validation, etc.) not otherwise wire-audited this pass."}
  automl_job: {status: partial, note: "FIXED this pass (parity-4) — AutoMLJob was missing the required LastModifiedTime/AutoMLJobSecondaryStatus fields entirely, plus the timestamp encoding bug (see Notes). FIXED this pass (parity-5) — the required DescribeAutoMLJobOutput/CreateAutoMLJobInput field InputDataConfig ([]types.AutoMLChannel) is now modeled (AutoMLChannel/AutoMLDataSource/AutoMLS3DataSource types added), accepted at Create, and always emitted (as [] when absent, matching the required-field contract). CORRECTED+FIXED this pass (parity-6) — parity-5's note that 'AutoMLJobInputDataConfig does not exist in the SDK' was itself wrong: it is the required field on CreateAutoMLJobV2Input ([]types.AutoMLJobChannel, CreateAutoMLJobV2Input:91), a real, distinct-from-V1 field. CreateAutoMLJobV2/DescribeAutoMLJobV2 were routed to the V1 handlers and so silently dropped it (plus the required AutoMLProblemTypeConfig union) on every V2 request — the actual bug gopherstack-e39w asked for. Both ops now have their own handlers (handler_automl_v2.go) with the correct V2 wire shape: AutoMLJobInputDataConfig ([]AutoMLJobChannel, a narrower type than V1's AutoMLChannel — no TargetAttributeName/SampleWeightAttributeName), AutoMLProblemTypeConfig (5-member tagged union, carried opaque per gaps: below), AutoMLProblemTypeConfigName (derived from which union member is present), AutoMLComputeConfig/DataSplitConfig/SecurityConfig/ModelDeployConfig (all small flat types, fully modeled). handleDescribeAutoMLJob (V1) was also changed from json.Marshal(struct) to an explicit response map, since the shared AutoMLJob struct now carries V2-only fields that would otherwise leak into a V1 Describe of a V2-created job. FIXED parity-24 (gopherstack-oc9v) — CreateAutoMLJobInput's RoleArn/InputDataConfig/OutputDataConfig are each 'This member is required' (api_op_CreateAutoMLJob.go), but only AutoMLJobName was ever validated present; a request missing any of the other three silently succeeded with an empty role and no data config. Now all four are enforced (InputDataConfig checked non-empty, not just non-nil). ModelDeployConfig (a real, optional CreateAutoMLJobInput field, and a type this backend already modeled for CreateAutoMLJobV2) was decoded nowhere on V1 Create at all — SetAutoMLJobExtras now accepts and DescribeAutoMLJob now returns it. ListAutoMLJobsInput was NextToken-only, dropping CreationTimeAfter/CreationTimeBefore/LastModifiedTimeAfter/LastModifiedTimeBefore/NameContains/StatusEquals/SortBy/SortOrder entirely (SortBy default Name, SortOrder default Descending per api_op_ListAutoMLJobs.go) — all now real. AutoMLJobConfig (CandidateGenerationConfig/CompletionCriteria/Mode) remains accept-and-drop on V1 Create; DataSplitConfig/SecurityConfig are already modeled for V2 but not wired to the V1 Create path, since V1's own AutoMLJobConfig is a distinct, still-unmodeled field. All 4 anonymous request structs in handler_automl.go converted to named types this pass. FIXED parity-26 (gopherstack-oc9v) — CreateAutoMLJobV2Input's RoleArn/AutoMLJobInputDataConfig/AutoMLProblemTypeConfig/OutputDataConfig (all 'This member is required' alongside AutoMLJobName, api_op_CreateAutoMLJobV2.go:72-166) were decoded but never validated present; a request missing any of the three non-name/non-role fields silently succeeded, and a pre-existing test (TestHandler_CreateAutoMLJobV2_RoundTrip's 'minimal' case) exercised exactly that gap, asserting a 200 for a request missing all three. Now all four enforced, the test rewritten to supply a structurally-valid fixture, and a new TestHandler_CreateAutoMLJobV2_RequiresAllRequiredMembers added. The remaining 2 anonymous request structs in handler_automl_v2.go converted to named types this pass."}
  lineage_action_artifact_context_association: {status: ok, note: "parity-5, wire-audited CreateAction/CreateArtifact/CreateContext + Describe/Update/Delete/List against api_op_{Create,Describe,Update}{Action,Artifact,Context}.go. No accept-and-drop bugs found — Source/Properties/Description/Status/Tags all round-trip correctly. QueryLineage/DescribeLineageGroup/ListLineageGroups/GetLineageGroupPolicy also verified (the single auto-provisioned lineage group with no policy is an honest, correctly-typed 404, not a stub). FIXED (gopherstack-cgq3) — ListAssociations was missing CreatedAfter/CreatedBefore/DestinationType/MaxResults/SortBy/SortOrder (six of eleven real ListAssociationsInput members; the audit that found this counted six, but SourceType was also absent and is fixed alongside them) — the request had been an anonymous inline struct with only SourceArn/DestinationArn/AssociationType/NextToken, invisible to field-audit tooling (gopherstack-oc9v); now a named listAssociationsInput. All six (seven) fields are real filters/sorts, not accept-and-drop: SourceType/DestinationType resolve the entity's type via the existing lineageEntityLookup; CreatedAfter/CreatedBefore filter on Association.CreationTime; SortBy/SortOrder reorder by SourceArn/DestinationArn/SourceType/DestinationType/CreationTime (default); MaxResults truncates via the existing paginateSlice helper. Proven with TestHandler_ListAssociations_Filters/_Sort/_MaxResults, which assert on the actual narrowed/reordered/paginated result set, not just on the parsed request. FIXED this pass (parity-8, gopherstack-oc9v) — the remaining 19 inline `struct{...}` request declarations in this family (CreateArtifact/DescribeArtifact/UpdateArtifact/DeleteArtifact/ListArtifacts, CreateContext/DescribeContext/UpdateContext/DeleteContext/ListContexts, DescribeAction/UpdateAction/DeleteAction/ListActions, DeleteAssociation, DescribeLineageGroup/ListLineageGroups/GetLineageGroupPolicy, QueryLineage) converted to named types and wire-audited; MetadataProperties (the gap this note flagged since parity-5) is now real on both CreateArtifact and CreateAction; DeleteArtifact's Source alternative identity, five real filter/sort/pagination fields each on ListArtifacts/ListContexts/ListActions, ListLineageGroups' CreatedAfter/CreatedBefore/SortBy/SortOrder/MaxResults, and QueryLineage's Filters/MaxResults/NextToken are all now real. See Notes: parity-8 for the full list and for what remains disclosed rather than modeled (QueryFilters.Types). FIXED parity-29 — AddAssociationInput has no Tags member at all (api_op_AddAssociation.go); addAssociationRequest decoded one anyway and applied it to the new association, a fabricated field no real client can ever populate (AddAssociationOutput echoes only Source/DestinationArn, matching the real op). Removed; an association can still be tagged afterward via AddTags against its resulting ARN. Proven by TestHandler_AddAssociation_TagsNotOnWire (no bd issue filed yet)."}
  edge_deployment_device_fleet: {status: partial, note: "FIXED this pass — DeviceFleet/Device family: OutputConfig (required in Create+Update) was silently optional and UpdateDeviceFleet silently dropped it; DeviceFleet/Device Describe+List timestamp encoding also fixed (see Notes). EdgeDeploymentPlan/EdgePackagingJob not otherwise wire-audited this pass. gopherstack-muzq (2026-08-21): EdgePackagingJobStatus was stamped STARTING at Create and STOPPING at Stop, and nothing else in this backend ever advanced either -- no ticker, no later call. Fixed via scheduleEdgePackagingJobCompletion (STARTING -> COMPLETED) and a runDelayed continuation in StopEdgePackagingJob (STOPPING -> STOPPED), mirroring the existing lifecycle.go runDelayed pattern already used by TrainingJob/Endpoint/InferenceComponent/the generic Job family. FailureReason/other field-level EdgePackagingJob wire audit remains open, unchanged from this note's prior scope. FIXED parity-24 (gopherstack-oc9v) — CreateEdgePackagingJobInput.OutputConfig ('This member is required', api_op_CreateEdgePackagingJob.go:13-52, types.EdgeOutputConfig{S3OutputLocation required, KmsKeyId/PresetDeploymentConfig/PresetDeploymentType optional}) was entirely absent from decode, storage, and Describe — the most severe finding of this pass, the same required-member-never-read class as this campaign's other headline bugs. Now required, stored, and echoed. ModelName/ModelVersion/RoleArn/CompilationJobName are also each 'This member is required' but only EdgePackagingJobName was ever validated present — all four now enforced. ListEdgePackagingJobsInput accepted only StatusEquals/NameContains/NextToken, dropping CreationTimeAfter/CreationTimeBefore/LastModifiedTimeAfter/LastModifiedTimeBefore/ModelNameContains/SortBy/SortOrder/MaxResults entirely; neither SortBy nor SortOrder documents a default on this op, so an unset value keeps this backend's pre-existing ascending-by-name order rather than inventing one (same conservative stance as parity-23's ListFlowDefinitions/ListHumanTaskUis). ResourceKey (a real, optional CreateEdgePackagingJobInput field) is now also stored and returned. DescribeEdgePackagingJobOutput's ModelArtifact/ModelSignature/PresetDeploymentOutput/EdgePackagingJobStatusMessage remain unmodeled — server-derived fields with no synchronous backend process to honestly derive them from, left absent rather than fabricated. All 4 anonymous request structs in handler_edge_packaging_jobs.go converted to named types this pass."}
  labeling_job: {status: partial, note: "parity-5, wire-audited CreateLabelingJob/DescribeLabelingJob against api_op_CreateLabelingJob.go/api_op_DescribeLabelingJob.go — this family was already the most fully-typed in the service (real InputConfig/OutputConfig/HumanTaskConfig/StoppingConditions/LabelingJobAlgorithmsConfig structs, real Initializing->InProgress->Completed FSM). FIXED this pass — Tags (a real, optional DescribeLabelingJobOutput field) were accepted and stored on Create but never serialized back out by DescribeLabelingJob; also fixed the LabelingJob.Tags struct field's json:\"-\" tag (was silently dropping Tags across a persistence snapshot/restore round-trip too, a second manifestation of the same bug). No other gaps found."}
  hub_hub_content: {status: ok, note: "parity-5, wire-audited CreateHub/DescribeHub/ImportHubContent/DescribeHubContent against api_op_{Create,Describe}Hub.go/api_op_{Import,Describe}HubContent.go. No accept-and-drop bugs found — this was already a thorough implementation: S3StorageConfig is correctly nested (not flattened) on both request and response, HubContentDependencies/presigned URLs/ModelReference content-references (CreateHubContentReference/UpdateHubContentReference) all real. No changes made."}
  cluster: {status: partial, note: "parity-5, wire-audited CreateCluster/DescribeCluster/UpdateCluster against api_op_{Create,Describe,Update}Cluster.go. FIXED parity-5 — ClusterRole and VpcConfig (both real optional CreateClusterInput/DescribeClusterOutput fields; VpcConfig reuses the existing shared VpcConfig type from training_jobs.go) were accepted-and-dropped entirely — CreateCluster's signature didn't have parameters for them at all. FIXED this pass (gopherstack-i359) — AutoScaling (types.ClusterAutoScalingConfig, Mode/AutoScalerType; DescribeCluster reports the required Status as InService, mirroring instanceGroupStatusInService's existing no-async-provisioning convention), NodeProvisioningMode (plain string), and TieredStorageConfig (types.ClusterTieredStorageConfig, Mode/InstanceMemoryAllocationPercentage) are now accepted on Create+Update and returned by Describe. Orchestrator (types.ClusterOrchestrator) is also now modeled — confirmed via botocore sagemaker/2017-07-24@1.43.56 service-2.json (`shapes.ClusterOrchestrator.type == \"structure\"`, not `\"union\"`) and serializers.go:27593-27612 that despite AWS's docs saying 'exactly one of Eks or Slurm', this is a plain struct with two independent optional members, not a discriminated wire union — so both fields decode independently and the exactly-one rule is enforced as a runtime ValidationException (api_op_CreateCluster.go:76-78) instead of a union tag. ALSO FIXED this pass (gopherstack-i359) — a persistence bug found while wiring the above: ClusterRole and VpcConfig (parity-5's fix) were never added to persistedCluster (persistence.go's hand-maintained Cluster DTO), so both were silently dropped across Snapshot/Restore even though CreateCluster/DescribeCluster round-tripped them correctly in memory; fixed alongside the four new fields. NOT fixed (see gaps:): RestrictedInstanceGroups/RestrictedInstanceGroupsConfig — judged too large to model faithfully within this pass's budget (ClusterRestrictedInstanceGroupSpecification alone nests EnvironmentConfig->FSxLustreConfig, a real 3-member InstanceStorageConfig union, and ScheduledUpdateConfig->DeploymentConfiguration->RollingDeploymentPolicy/AlarmDetails — six more nested types beyond the top-level spec); left entirely untouched rather than partially modeled. Re-examined a third time (gopherstack-i359, session 3): same conclusion, with the scope confirmed even larger than previously written up — see gaps: for the session-3 detail, including a wholly separate RestrictedInstanceGroupsConfig field this campaign hadn't previously named. StartClusterHealthCheck (parity-4) unaffected."}
  inference_recommendations_edge_packaging: {status: partial, note: "parity-5, wire-audited CreateInferenceRecommendationsJob/DescribeInferenceRecommendationsJob against api_op_{Create,Describe}InferenceRecommendationsJob.go. This is a DIFFERENT family from AIRecommendationJob (ai_recommendation_jobs.go, parity-4) — distinct SDK ops, distinct store, no shared state. FIXED this pass — InputConfig ([]types.RecommendationJobInputConfig-shaped) is 'This member is required' on both CreateInferenceRecommendationsJobInput and DescribeInferenceRecommendationsJobOutput but was not modeled, accepted, or returned at all (the struct had no field for it whatsoever) — now stored+echoed as opaque json.RawMessage passthrough (same established convention as ai_benchmark_job/ai_recommendation_job/ai_workload_config's own deeply-nested union fields, see gaps: below). Real client-populated content round-trips exactly. EdgePackagingJob portion not otherwise wire-audited this pass. gopherstack-muzq (2026-08-21): InferenceRecommendationsJob.Status was stamped IN_PROGRESS at Create and STOPPING at Stop, and nothing else in this backend ever advanced either -- confirmed via DescribeInferenceRecommendationsJob, which echoed the stored value verbatim forever. Fixed via scheduleInferenceRecommendationsJobCompletion (IN_PROGRESS -> COMPLETED) and a runDelayed continuation in StopInferenceRecommendationsJob (STOPPING -> STOPPED), same lifecycle.go runDelayed pattern as EdgePackagingJob's fix above."}
  training_plan: {status: partial, note: "FIXED this pass — TrainingPlan/ReservedCapacity/ReservedCapacitySummary timestamp encoding (see Notes). Not otherwise wire-audited this pass. FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- TrainingPlanExtension.ExtendedAt/StartDate/EndDate and TrainingPlanExtensionOffering.StartDate/EndDate were plain time.Time fields marshaled directly by ExtendTrainingPlan and SearchTrainingPlanOfferings (handler_training_plan.go's json.Marshal(map[string]any{...})), unlike the sibling TrainingPlan/ReservedCapacity types this same file already fixed with a MarshalJSON override -- these two types were missed by that pass. Real ExtendTrainingPlanOutput/SearchTrainingPlanOfferingsOutput deserialize these members via ParseEpochSeconds(json.Number), so every real SDK client's call failed outright once a training plan had any extension offering (SearchTrainingPlanOfferings always generates one when TrainingPlanArn is set) or purchased extension. Fixed via the same alias-embedding MarshalJSON/UnmarshalJSON pattern as TrainingPlan/ReservedCapacity. Proven via a real aws-sdk-go-v2/service/sagemaker client round trip through both ops (wire_training_plan_extension_test.go), hand-reverted/confirmed-failing (expected Timestamp to be a JSON Number, got string instead)/restored, md5sum-verified byte-identical. FIXED parity-26 (gopherstack-oc9v), first field audit of CreateTrainingPlan/DescribeTrainingPlan themselves — CreateTrainingPlanInput.TrainingPlanOfferingId is 'This member is required' alongside TrainingPlanName (api_op_CreateTrainingPlan.go), but only TrainingPlanName was validated; a request naming no offering silently created a minimal Active plan with no backing reserved capacity instead of being rejected. A pre-existing test, TestHandler_CreateTrainingPlan_WithoutOffering_StaysMinimal, asserted this directly (200 for a request with no TrainingPlanOfferingId) — rewritten as TestHandler_CreateTrainingPlan_RequiresTrainingPlanOfferingId, asserting the corrected 400. Separately, TrainingPlan.TargetResources/TotalInstanceCount/UpfrontFee (all real, optional DescribeTrainingPlanOutput members) were tagged json:\"-\" on the backend struct, so handleDescribeTrainingPlan's direct json.Marshal(result) silently omitted all three from every Describe response even though ListTrainingPlans' summary builder (trainingPlanSummaryJSON, handler_training_plan.go) had already been projecting the same three fields into List responses the whole time — a Describe/List same-key/same-field asymmetry. Fixed by correcting the three tags to their real wire names with omitempty; proven by TestHandler_DescribeTrainingPlan's new assertions. The 2 anonymous request structs in handler_training_plans.go converted to named types this pass."}
  monitoring_schedule_workteam_compilation_job: {status: partial, note: "FIXED this pass — MonitoringSchedule and CompilationJob Describe+List timestamp encoding (see Notes). Workteam field audit done separately (parity-20). CompilationJob's own deep field audit done parity-21 (gopherstack-oc9v): required-field validation, ModelArtifacts/FailureReason, Stopping FSM, List filter/sort — see ops: entries above and Notes: parity-21. MonitoringSchedule field audit still not done. FIXED 2026-08-29 (constrain-not-honoured sweep, gopherstack-oc9v continuation, uncommitted at write time): ListMonitoringAlertHistoryInput.SortBy (types.MonitoringAlertHistorySortKey -- real values CreationTime (default) and Status, api_op_ListMonitoringAlertHistory.go) was never decoded by listMonitoringAlertHistoryRequest at all -- a client's SortBy=Status was silently dropped, and MonitoringAlertHistoryFilter's own doc comment asserted 'sort key is always CreationTime', an incorrect absence-comment of exactly the kind this campaign warns about. Fixed: SortBy now decoded and threaded through; ListMonitoringAlertHistory sorts by AlertStatus when SortBy=Status (case-insensitive per this service's established SortBy-matching convention), CreationTime otherwise. ListMonitoringExecutions/ListMonitoringAlerts/ListWorkteams/ListEdgeDeploymentPlans/ListModelPackages/ListTrainingPlans/SearchTrainingPlanOfferings/ListClusters*/ListApps/ListUserProfiles/ListSpaces/ListDevices/ListTrialComponents/ListInferenceRecommendationsJobSteps were all independently re-checked field-by-field against their pinned SDK input structs this pass (decoded-but-dropped and never-plumbed-at-all patterns specifically) and found already correct or already honestly disclosed as no-ops with a cited reason -- no other bug found in this slice. Proven via TestHandler_ListMonitoringAlertHistory_SortByStatus (handler_modelmonitor_test.go), confirmed failing pre-fix (returned CreationTime-descending order regardless of SortBy)."}
  studio_lifecycle_config: {status: ok, note: "FIXED this pass (gopherstack-5wj0) — CreateStudioLifecycleConfig accepted a request body with no field for StudioLifecycleConfigContent at all, even though it is 'This member is required' on CreateStudioLifecycleConfigRequest (botocore sagemaker service-2.json) and is also part of DescribeStudioLifecycleConfigResponse. Every real client's script content was silently discarded and Create succeeded without it, where real AWS would reject the request. Now required, stored, and returned by Describe. FIXED 2026-08-21 (parity-23, gopherstack-oc9v) — StudioLifecycleConfigAppType (also 'This member is required' on CreateStudioLifecycleConfigInput) was accepted-and-dropped the same way Content once was, and ListStudioLifecycleConfigsInput's AppTypeEquals/CreationTimeAfter/CreationTimeBefore/ModifiedTimeAfter/ModifiedTimeBefore/NameContains/SortBy/SortOrder/MaxResults were all silently ignored (NextToken-only). Both fixed — see Notes: parity-23."}
  modelcard_export: {status: ok, note: "parity-26 (gopherstack-oc9v), first wire audit of this family — CreateModelCardExportJob/DescribeModelCardExportJob field-diffed against api_op_{Create,Describe}ModelCardExportJob.go. No gaps found: ModelCardExportJobName/ModelCardName/OutputConfig.S3OutputPath (all 'This member is required' on CreateModelCardExportJobInput) are validated in the backend (CreateModelCardExportJob, modelcard_export.go), and every DescribeModelCardExportJobOutput required/optional member (CreatedAt/LastModifiedAt/ModelCardExportJobArn/ModelCardExportJobName/ModelCardName/ModelCardVersion/OutputConfig/Status/ExportArtifacts/FailureReason) was already correctly emitted. Both anonymous request structs in handler_modelcard_export.go converted to named types this pass; no behavioral change."}
  monitoring_job_definitions: {status: partial, note: "parity-26 (gopherstack-oc9v), first wire audit of the four Model Monitor job definition types' shared Create path (parseJobDefRequest, handler_monitoring_job_definitions.go) against api_op_Create{DataQuality,ModelBias,ModelQuality,ModelExplainability}JobDefinition.go. FIXED — RoleArn ('This member is required' on all four Create*JobDefinitionInput types) was decoded but never validated present. FIXED — JobResources and the type's own AppSpecification/JobOutputConfig (all 'This member is required') were accept-and-drop with no presence check at all, kept only inside the opaque Config passthrough map; a request omitting any of them silently succeeded with an incomplete job definition. All five now validated by a new validateJobDefRequest helper, keyed off the type's name prefix derived from jobInputKey (e.g. 'DataQualityJobInput' -> 'DataQuality'). Multiple pre-existing tests across handler_monitoring_job_definitions_test.go and handler_modelmonitor_test.go supplied only JobDefinitionName for ModelBias/ModelQuality/ModelExplainability Create calls and asserted 200 — the missing-assertion/fixture-gap test-trap shape this campaign keeps finding, at its widest scope yet (three of four sibling types plus several List-family setup helpers) — all rewritten via a new shared minimalJobDefinitionFixture helper, and a new TestHandler_CreateDataQualityJobDefinition_RequiresAllRequiredMembers added. PARTIAL: JobResources/AppSpecification/JobOutputConfig/BaselineConfig/NetworkConfig/StoppingCondition remain opaque json.RawMessage passthrough rather than fully-typed structs (same established convention as algorithm's TrainingSpecification) — every field a client sends round-trips exactly. Both anonymous request structs (parseJobDefinitionName/parseJobDefinitionListRequest, shared by Describe/Delete/List across all four types) converted to named types this pass."}
  mlflow: {status: partial, note: "parity-29 (fix/wrapper-key-sweep-rds-cloudwatch-sqs-sns exhaustive request-field sweep): first full field-diff of MlflowTrackingServer/MlflowApp against api_op_{Create,Describe,Update,Delete,Start,Stop,List}Mlflow{TrackingServer,App}.go and the presigned-URL ops. FIXED — UpdateMlflowTrackingServerInput has no MlflowVersion member at all (only Create/Describe do); updateMlflowTrackingServerInput decoded one anyway and UpdateMlflowTrackingServerOptions applied it to the stored server on every Update, a fabricated field no real client can ever send. Removed from both the wire struct and UpdateMlflowTrackingServerOptions. Proven by TestHandler_UpdateMlflowTrackingServer_MlflowVersionNotOnWire (fails against the pre-fix code: DescribeMlflowTrackingServer's MlflowVersion changed from a Create-time value to an Update-time one that no real client could have sent). Everything else in this family (presigned-URL no-ops, MlflowApp's Name no-op, List filter/sort/page surfaces) was already correctly disclosed prior to this pass — see handler_mlflow.go/handler_mlflow_test.go doc comments. (no bd issue filed yet)"}
  pagination_sweep: {status: ok, note: "2026-08-28/29 (wrapper-key-sweep-rds-cloudwatch-sqs-sns pagination pass): audited all ~90 List ops plus Search/QueryLineage/DescribeEdgeDeploymentPlan/DescribeTrainingPlanExtensionHistory/CreateHubContentPresignedURLs (every op with a MaxResults+NextToken pair) against the pinned SDK. All correctly truncate at MaxResults (falling back to sagemakerDefaultPageSize=100, store_setup.go:43, when unspecified), consume NextToken as a resume offset, and emit NextToken only when more results remain — confirms and extends the existing gaps: entry describing this service's integer-offset-token convention as 'functionally correct'. Nearly every op routes through the shared list_helpers.go family (paginateSlice/sagemakerListPaged/sagemakerListKeyPagedN/filterSortPaginateByName*), which already handles the offset-parse/cap/emit logic once, correctly, for every caller. The few ops with no shared-helper call were individually verified: ListClusterEvents/ListResourceCatalogs are disclosed structural void-results (no event/catalog data this backend ever populates, so nothing to paginate); ListTags (sagemaker's own resource tags, not S3) applies paginateSlice directly in its handler over a deterministically name-sorted slice; ListDataQualityJobDefinitions/ListModelBiasJobDefinitions/ListModelQualityJobDefinitions/ListModelExplainabilityJobDefinitions all delegate to the shared listJobDefinitions helper, which itself calls paginateSlice. One pre-existing, unrelated (not this pass's bug class) divergence spot-checked but not fixed: ListEndpoints documents a real default MaxResults of 10 (api_op_ListEndpoints.go:49) but this service's uniform sagemakerDefaultPageSize=100 applies there too, same as every other List op — truncation/resumption/token-emission are all still correct at 100 rather than 10, so no client-visible pagination-loop failure, just a larger-than-AWS default page. Spot-verified with a real aws-sdk-go-v2/service/sagemaker client (existing coverage; no new pagination bugs found, so no new fix/test needed for this service)."}

gaps:                     # known divergences NOT fixed — link bd issue ids
  - "FIXED 2026-09-07 (gopherstack-z5hj): StartPipelineExecution and StartPipelineExecutionFull (pipelines.go) both set PipelineExecutionStatus directly to pipelineStatusSucceeded and returned -- neither went through pipelineStatusExecuting, and neither called runDelayed. StartPipelineExecutionFull is the wire-facing path (handleStartPipelineExecutionFull -> StartPipelineExecutionFull); StartPipelineExecution is the plain backend entrypoint used directly by tests. Both had the identical bug (the originally-filed triage note, gopherstack-z1sd, named only StartPipelineExecution -- StartPipelineExecutionFull was found and fixed in the same pass since it shares the same code shape and is the one real clients actually hit). Both now set pipelineStatusExecuting, then runDelayed(b.lifecycleCtx, startTransitionDelay, ...) transitions to pipelineStatusSucceeded, re-reading the execution from the store before mutating (same pattern as RetryPipelineExecution/StopPipelineExecution). startTransitionDelay is a new constant (200ms, pipeline_executions.go) -- no existing delay constant fit a start's semantics (retryTransitionDelay is for retry, stopTransitionDelay runs the opposite direction); every other async-transition family in this service (processingJobCompletionDelay, labelingJobCompletionDelay, transformJobCompletionDelay, ...) likewise defines its own named constant rather than reusing another family's. Verified against sagemaker@v1.263.2 types/enums.go:6945-6954 that Executing is the correct initial PipelineExecutionStatus (the only non-terminal value in the enum: Executing/Stopping/Stopped/Failed/Succeeded). Searched every sagemaker _test.go for a PipelineExecutionStatus/Status assertion tied to Start -- none exist, so no pre-existing test asserted the bug and none needed correction. New regression test (pipeline_execution_start_test.go, testing/synctest, no time.Sleep outside the bubble) covers both entrypoints and was proven to fail against the unfixed code before the fix was applied."
  - "Pagination across the service is a hand-rolled integer-offset NextToken (parseNextToken/strconv.Atoi) rather than pkgs/page's opaque-token helper. Functionally correct (AWS clients treat NextToken as opaque) and internally consistent, but is a pkgs-catalog convention deviation across ~15 call sites. Not fixed this pass — refactor is cross-cutting and out of budget for a single-family sweep. (no bd issue filed yet)"
  - "ProductionVariantSummary.VariantStatus is populated with a single synthetic {Status: \"Creating\"|\"InService\"} entry, not a full AWS VariantStatus enum/message model (StatusMessage is always empty, no DeployedImages/CapacityReservationConfig/ManagedInstanceScaling/RoutingConfig fields). Sufficient for status-polling clients; deeper fidelity deferred. (no bd issue filed yet)"
  - "parity-4: AIBenchmarkJob's BenchmarkTarget/OutputConfig/NetworkConfig, AIRecommendationJob's ModelSource/OutputConfig/PerformanceTarget/ComputeSpec/InferenceSpecification, and AIWorkloadConfig's AIWorkloadConfigs/DatasetConfig are all stored+echoed as opaque json.RawMessage rather than modeled as fully-typed structs (same convention as algorithms.go's TrainingSpecification/InferenceSpecification/ValidationSpecification). Every field the client sends round-trips exactly; the only thing not reproduced is AWS server-synthesized sub-fields that don't exist in the Create input at all (e.g. AIBenchmarkOutputResult.CloudWatchLogs). Not a wire-shape bug for any field a client actually populates, but real if a client depends on those server-only sub-fields appearing. (no bd issue filed yet)"
  - "parity-4: AIRecommendationJob.Recommendations ([]types.AIRecommendation) is a real, always-empty slice — this backend never fabricates optimization recommendations, deployment configs, or performance-metric numbers a client could mistake for a measured result. A client polling DescribeAIRecommendationJob for actual recommendations will never see any, even after the job reaches Completed. Deliberate per this campaign's 'no fabricated metrics' rule, but is a real functional gap for any test asserting recommendation content. (no bd issue filed yet)"
  - "parity-4: DescribeJobSchemaVersion/ListJobSchemaVersions serve a single synthetic JobConfigSchemaVersion (\"1.0\") with a generic, not-per-category JSON-schema document for every JobCategory — AWS does not publish real per-category schema content anywhere in the SDK module, so there is no ground truth to model against. CreateJob does validate JobConfigSchemaVersion against this same registry (real ResourceNotFound if unknown), so the three ops are at least internally consistent, just not a reproduction of AWS's actual (unpublished) schema catalog. (no bd issue filed yet)"
  - "parity-5: DescribePipeline never returns PipelineVersionDescription/PipelineVersionDisplayName/CreatedBy/LastModifiedBy (PipelineVersionId input + LastRunTime output FIXED parity-6, see Notes). ListPipelines' PipelineSummary is also missing PipelineDescription/PipelineDisplayName/RoleArn/LastExecutionTime (real optional PipelineSummary fields) and has a PipelineStatus field that does not exist on the real type at all (harmless for JSON-protocol clients, which ignore unknown fields, but not a reproduction of AWS's shape). (no bd issue filed yet)"
  - "parity-5: TrialComponent/Experiment/Trial's CreatedBy/LastModifiedBy/Source/ExperimentSource/TrialSource (types.UserContext / *Source ARN+type pairs) are not modeled at all — there is no IAM-identity or resource-provenance model in this backend to honestly derive them from (class d, not fabricated). (no bd issue filed yet)"
  - "parity-6: feature_store's UpdateFeatureGroup does not accept OnlineStoreConfigUpdate/ThroughputConfigUpdate (CreateFeatureGroupInput's OnlineStoreConfig/OfflineStoreConfig/ThroughputConfig FIXED parity-6, see Notes — this is the separate Update-path pair of fields, out of that fix's scope) — nor LastUpdateStatus/OfflineStoreStatus/FailureReason/OnlineStoreTotalSizeBytes (DescribeFeatureGroupOutput fields describing async store-creation progress this backend has no notion of, since store creation is synchronous here). (no bd issue filed yet)"
  - "gopherstack-i359 (session 3, re-confirmed): cluster's RestrictedInstanceGroups/RestrictedInstanceGroupsConfig (CreateClusterInput/UpdateClusterInput/DescribeClusterOutput) remain accept-and-drop — Orchestrator/AutoScaling/NodeProvisioningMode/TieredStorageConfig were fixed in session 2 (see cluster: note above); PipelineDefinitionS3Location was fixed for real in session 3 (see pipeline_pipeline_execution: note above). RestrictedInstanceGroups was re-examined a third time this session rather than deferred by default, and the scope is confirmed larger than session 2's write-up: ClusterRestrictedInstanceGroupSpecification (types/types.go:5622) nests EnvironmentConfig->FSxLustreConfig (2 required fields), a real 3-member ClusterInstanceStorageConfig union (types/types.go:5107, EbsVolumeConfig/FsxLustreConfig/FsxOpenZfsConfig — confirmed a genuine Go interface union, not a struct-with-business-rule like ClusterOrchestrator turned out to be), and ScheduledUpdateConfig->DeploymentConfiguration->RollingDeploymentPolicy->CapacitySizeConfig (x2)/AutoRollbackConfiguration []AlarmDetails — 8 new leaf/union types, not 6, once the union's 3 members and RollingDeploymentPolicy's nested CapacitySizeConfig are counted individually. On top of that, CreateClusterInput/UpdateClusterInput/DescribeClusterOutput carry a SEPARATE field this campaign had not previously named — RestrictedInstanceGroupsConfig (types/types.go:5598) -> ClusterSharedEnvironmentConfig (types/types.go:5727, a required FSxLustreConfig + a required FSxLustreDeletionPolicy enum) — meaning the honest scope of 'RestrictedInstanceGroups' is two independent top-level fields, not one. Modeling all of this without shaving any field (this campaign's explicit rule, restated for this issue) is comparable in size to the entire session-2 pass that modeled Orchestrator/AutoScaling/NodeProvisioningMode/TieredStorageConfig combined. Left entirely untouched a third time, now with this deeper accounting on record so a future pass can scope it accurately instead of re-deriving the type tree from scratch. (no bd issue filed yet)"
  - "parity-5: InferenceRecommendationsJob.InputConfig (fixed this pass to stop being silently dropped) is stored as opaque json.RawMessage passthrough rather than the fully-typed RecommendationJobInputConfig union (ContainerConfig/Endpoints/ModelPackageVersionArn/ModelName/...) — same convention as the parity-4 AI-job families' passthrough fields. Every field a client sends round-trips exactly; no server-synthesized sub-field is fabricated. (no bd issue filed yet)"
  - "parity-5: lineage's CreateAction/CreateArtifact accept no MetadataProperties field (a real, optional CreateActionInput/CreateArtifactInput field) — low-severity accept-and-drop left for a follow-up pass since the rest of this family was clean. (no bd issue filed yet)"
  - "parity-6: CreateAutoMLJobV2/DescribeAutoMLJobV2's AutoMLProblemTypeConfig is a 5-member tagged union (ImageClassificationJobConfig/TabularJobConfig/TextClassificationJobConfig/TextGenerationJobConfig/TimeSeriesForecastingJobConfig), each itself a materially large nested struct (e.g. TabularJobConfig alone has CandidateGenerationConfig/FeatureSpecificationS3Uri/Mode/ProblemType/TargetAttributeName/...). Carried as opaque json.RawMessage passthrough, same established convention as this file's other deeply-nested unions (ai_benchmark_job/ai_recommendation_job/inference_recommendations_job) — every field a client sends round-trips exactly; only AutoMLProblemTypeConfigName (which member is present) is derived, not the member's internal fields. (no bd issue filed yet)"
  - "parity-6: DescribeAutoMLJobV2Output's BestCandidate/PartialFailureReasons/ResolvedAttributes/AutoMLJobArtifacts/EndTime/FailureReason/ModelDeployResult are not modeled — these are server-synthesized/derived fields that mirror V1 DescribeAutoMLJobOutput's pre-existing, disclosed depth limit (V1 has never modeled BestCandidate/ResolvedAttributes/etc. either); not a V2-specific regression, just not newly fixed by this pass. (no bd issue filed yet)"
  - "parity-7 (gopherstack-oc9v): Domain's DefaultUserSettings/DefaultSpaceSettings/DomainSettings, UserProfile's UserSettings, Space's OwnershipSettings/SpaceSettings/SpaceSharingSettings, and App's ResourceSpec are all carried as opaque json.RawMessage passthrough rather than fully-typed structs — UserSettings alone has ~20 app-specific sub-configs (JupyterServerAppSettings, KernelGatewayAppSettings, CanvasAppSettings, CodeEditorAppSettings, SpaceStorageSettings, ...), each individually as large as a small family already in this file. Every field a client sends round-trips exactly; no server-synthesized sub-field is fabricated. (no bd issue filed yet)"
  - "parity-7 (gopherstack-oc9v): DescribeApp/DescribeDomain still omit several real optional output-only fields this pass didn't add backend state for: App's EffectiveTrustedIdentityPropagationStatus/BuiltInLifecycleConfigArn/FailureReason/LastHealthCheckTimestamp/LastUserActivityTimestamp; Domain's FailureReason/HomeEfsFileSystemId/SecurityGroupIdForDomainBoundary/SingleSignOnApplicationArn/SingleSignOnManagedApplicationInstanceId/HomeEfsFileSystemKmsKeyId (deprecated, superseded by KmsKeyId which IS modeled). These are server-derived/lifecycle fields with no synchronous backend process to derive them from truthfully; left absent rather than fabricated. (no bd issue filed yet)"
  - "parity-24 (gopherstack-oc9v): ProcessingInput.DatasetDefinition (AthenaDatasetDefinition/RedshiftDatasetDefinition sub-fields beyond the pre-existing DataDistributionType/InputMode) and ProcessingOutput.FeatureStoreOutput remain unmodeled/accept-and-drop — both are deep, low-traffic optional unions; NetworkConfig/ExperimentConfig/StoppingCondition (the higher-severity findings this pass) were fixed instead. (no bd issue filed yet)"
  - "parity-24 (gopherstack-oc9v): TransformJob's DataCaptureConfig/DataProcessing/ExperimentConfig/ModelClientConfig (all real, optional CreateTransformJobInput fields) and LabelingJobArn/AutoMLJobArn (real, optional DescribeTransformJobOutput-only fields) remain accept-and-drop or unmodeled — this pass fixed the fabricated RoleArn field and the List filter/sort surface instead, both higher severity. (no bd issue filed yet)"
  - "parity-24 (gopherstack-oc9v): CreateAutoMLJobInput's AutoMLJobConfig (CandidateGenerationConfig/CompletionCriteria/Mode) remains accept-and-drop on the V1 path — DataSplitConfig/SecurityConfig are already modeled (reused from CreateAutoMLJobV2) but not wired to V1 Create, since V1's own AutoMLJobConfig field is itself still unmodeled. StopAutoMLJob also transitions directly InProgress->Stopped with no intermediate Stopping state, unlike the real AutoMLJobStatus enum (which declares Stopping) and unlike this backend's own TransformJob/ProcessingJob/EdgePackagingJob FSMs — pre-existing, not introduced this pass, left as a disclosed stuck-status-class finding for a future pass. (no bd issue filed yet)"
  - "parity-24 (gopherstack-oc9v): DescribeEdgePackagingJobOutput's ModelArtifact/ModelSignature/PresetDeploymentOutput/EdgePackagingJobStatusMessage remain unmodeled — server-derived fields from an async packaging pipeline this backend does not simulate; left absent rather than fabricated. (no bd issue filed yet)"
  - "parity-25 (gopherstack-oc9v): algorithm's TrainingSpecification/InferenceSpecification/ValidationSpecification (all now required-checked/present, see families: algorithm) remain opaque json.RawMessage passthrough rather than fully-typed structs — TrainingSpecification alone nests ChannelSpecification/MetricDefinition/HyperParameterSpecification, deep and low-traffic; every field a client sends round-trips exactly. (no bd issue filed yet)"
  - "parity-25 (gopherstack-oc9v): model_endpoint_config_crud's CreateEndpointConfigInput.ExplainerConfig (types.ExplainerConfig -> ClarifyExplainerConfig -> ClarifyShapConfig/...) is stored+echoed as opaque json.RawMessage rather than fully modeled, same passthrough convention as algorithm's specs above; every field a client sends round-trips exactly, proven via a real-SDK-client test. (no bd issue filed yet)"
  - "parity-25 (gopherstack-oc9v): presigned_session's CreatePresignedDomainUrlInput.ExpiresInSeconds/LandingUri/SessionExpirationDurationInSeconds are decoded but are disclosed no-ops — CreatePresignedDomainUrlOutput has no field to reflect them into, and this backend's synthetic authorized-URL token carries no verified real query-parameter format to encode them, the same stance already established for PartnerApps' identical fields. (no bd issue filed yet)"

deferred:                 # consciously not (fully) audited this pass (scope) — next pass targets
  - model_package_model_package_group (beyond ModelPackageStatusDetails fix; InferenceSpecification etc. not audited)
  - edge_deployment_device_fleet (EdgeDeploymentPlan portion; DeviceFleet/Device fixed parity-earlier, EdgePackagingJob's Create/List wire surface fixed parity-24 — see families: edge_deployment_device_fleet)
  - training_plan (beyond timestamp fix)
  - monitoring_schedule_workteam_compilation_job (Workteam portion; MonitoringSchedule/CompilationJob timestamps fixed)
  - inference_recommendations_edge_packaging (EdgePackagingJob portion only; InferenceRecommendationsJob itself audited+fixed parity-5)

leaks: {status: clean, note: "Re-verified this pass: grepped every 'go func()'/runDelayed call site service-wide (8 files). Only one raw 'go func()' exists (lifecycle.go Shutdown, which waits on b.wg and is itself bounded by ctx.Done()); every timer-based state transition goes through runDelayed(b.lifecycleCtx, ...), which Shutdown cancels and drains via b.wg. No goroutine leaks found."}

---

## Notes

**Bug fixed this pass (systemic epoch-seconds timestamp wire bug across 27 resource types):**

The AWS `awsjson1.1` protocol this service emulates encodes timestamps as JSON *numbers*
(Unix epoch seconds), never as RFC3339 strings — `pkgs/awstime` and this service's own
`epochSeconds()` helper exist specifically for this. However, ~25 resource types' `Describe*`
handlers called `json.Marshal()` directly on the backend struct (e.g.
`return json.Marshal(result)` in `handleDescribeCodeRepository`) instead of building an
explicit response map with `epochSeconds(...)` conversions. Since Go's default `encoding/json`
marshals a bare `time.Time` field via its own `MarshalJSON` (RFC3339 string), every one of
these Describe responses put out `"CreationTime": "2026-07-22T10:00:00Z"` instead of
`"CreationTime": 1753178400` — a wire-shape bug that would make a real AWS SDK client
(Go, Python boto3, anything using a spec-compliant JSON-protocol deserializer) fail to parse
the response outright, since a numeric-typed field receiving a JSON string is a hard
deserialization error, not a silent zero-value.

A parallel form of the same bug existed in ~15 `List*` handlers, which built
`map[string]any{keyCreationTime: x.CreationTime, ...}` — putting the raw `time.Time` into an
`any`-typed map value instead of calling `epochSeconds(x.CreationTime)`.

Affected types (Describe path, fixed via a `MarshalJSON`/`UnmarshalJSON` pair added next to
each type — see below for why both are needed): `CodeRepository`, `HumanTaskUI`, `SMImage`,
`ImageVersion`, `ModelCard`, `InferenceExperiment`, `MlflowTrackingServer`, `ModelPackage`,
`ModelPackageGroup`, `StudioLifecycleConfig`, `TrainingPlan`, `ReservedCapacitySummary`,
`ReservedCapacity`, `AppImageConfig`, `AutoMLJob`, `CompilationJob`, `DeviceFleet`, `Device`,
`InferenceComponent`, `FlowDefinition`, `ClusterSchedulerConfig`, `ComputeQuota`,
`OptimizationJob`, `Project`, `Space`, `MonitoringSchedule`.

Affected List handlers (fixed by wrapping the map value in `epochSeconds(...)`):
`ListAutoMLJobs`, `ListCompilationJobs`, `ListDeviceFleets`, `ListDevices`,
`ListClusterSchedulerConfigs`, `ListComputeQuotas`, `ListInferenceComponents`,
`ListModelPackageGroups`, `ListCodeRepositories`, `ListImages`, `ListImageVersions`,
`ListMonitoringSchedules`, `ListProjects`, `ListSpaces`.

Fix approach: rather than rewrite each Describe handler into an explicit field-by-field
response map (the larger, higher-risk refactor), each affected type got a `MarshalJSON` that
wraps a type-aliased copy of itself with the timestamp field(s) overridden to
`epochSeconds(...)` float64s (Go's JSON marshaling: a shallower same-tagged field wins over an
embedded one). This is a much smaller diff per type and fixes both `Describe*` AND any other
code path that marshals the struct directly. **This has one consequence the next auditor must
know**: `persistence.go`'s snapshot/restore path also marshals these same structs directly
(table snapshots are just `json.Marshal`/`Unmarshal` of the store), so it now round-trips
through the same epoch-seconds encoding — every type also got a paired `UnmarshalJSON` (using
new `timeFromEpochSeconds`/`timeFromEpochSecondsPtr` helpers in `handler.go`) so
`persistence_test.go`'s round-trip tests still pass. Sub-second precision is lost across a
persistence round-trip as a result (epoch-seconds is whole-second granularity) — this is
inherent to the fix (AWS's own wire format is whole-second) and does not affect any tested
behavior, since no test asserts sub-second `CreationTime` precision.

New shared helpers in `handler.go`: `epochSecondsPtr` (nil-safe `*time.Time` → `*float64`,
preserving `omitempty` semantics for optional timestamps like `TrainingPlan.StartTime`),
`timeFromEpochSeconds`, `timeFromEpochSecondsPtr` (the inverses, used by the new
`UnmarshalJSON` methods).

**Bug fixed this pass (HyperParameterTuningJob wire shape — nested config + missing required
counters):**

`DescribeHyperParameterTuningJob`/`ListHyperParameterTuningJobs` emitted a flat top-level
`"Strategy"` field; real AWS nests `Strategy`/`ResourceLimits` inside a
`HyperParameterTuningJobConfig` object — a client reading
`output.HyperParameterTuningJobConfig.Strategy` (the only place the real SDK exposes it on
`DescribeHyperParameterTuningJobOutput`) got nothing. Both responses also omitted
`ObjectiveStatusCounters`/`TrainingJobStatusCounters`, which are `This member is required` in
the real output types — a real AWS SDK client dereferences these unconditionally, so omitting
them entirely (not even an empty object) would nil-pointer-panic real client code. `Strategy`
alone was also stored on `CreateHyperParameterTuningJob`; `ResourceLimits` was accepted in the
test fixtures' request bodies but silently discarded by the handler. Fix: `HyperParameterTuningJob`
gained `ResourceLimits`/`ObjectiveStatusCounters`/`TrainingJobStatusCounters` fields (the latter
two always zero-valued-but-present, since this emulator doesn't launch child training jobs);
`CreateHyperParameterTuningJob`'s signature gained a `limits HPResourceLimits` parameter;
Describe/List handlers now emit the correctly-nested/complete shape. Files: `hp_tuning_jobs.go`,
`handler_hp_tuning_jobs.go`, `interfaces.go`. Tests:
`TestHandler_DescribeHyperParameterTuningJob_WireShape`,
`TestHandler_ListHyperParameterTuningJobs_WireShape` in `handler_hp_tuning_jobs_test.go`.

**Bug fixed this pass (DeviceFleet — required OutputConfig silently optional, dropped on
Update):**

`OutputConfig` (specifically `S3OutputLocation`) is `This member is required` on both
`CreateDeviceFleetInput` and `UpdateDeviceFleetInput` in the real API — real AWS rejects a
`CreateDeviceFleet` call missing it with `ValidationException`. This emulator's
`handleCreateDeviceFleet` treated it as fully optional, so a client (or the pre-existing test
suite, which never sent it) could create a `DeviceFleet` with no `OutputConfig` at all; since
`OutputConfig` is *also* required on `DescribeDeviceFleetOutput`, the resulting
`DescribeDeviceFleet` response would then omit a required field. Separately,
`handleUpdateDeviceFleet`/`UpdateDeviceFleet` didn't accept `OutputConfig` in the request body
at all — a client updating a fleet's output location would have the call silently succeed
while `OutputConfig` stayed unchanged. Fix: `CreateDeviceFleet` now validates
`OutputConfig.S3OutputLocation` is present (`ValidationException` otherwise, matching real AWS);
`UpdateDeviceFleet`'s signature gained an `outputConfig *DeviceFleetOutputConfig` parameter,
threaded through from the handler. Every pre-existing `CreateDeviceFleet` test call site across
`handler_device_fleets_test.go` and `handler_edge_deployment_test.go` was updated to send a
valid `OutputConfig` (12 call sites). Files: `device_fleets.go`, `handler_device_fleets.go`.

**Bug fixed this pass (ModelPackage — missing required ModelPackageStatusDetails):**

`ModelPackageStatusDetails` (with a required `ValidationStatuses` list inside it) is
`This member is required` on `DescribeModelPackageOutput`; the `ModelPackage` struct didn't
have this field at all, so `DescribeModelPackage`/`CreateModelPackage` responses omitted it
entirely — the same "required field missing from the struct, not just unpopulated" bug class as
the HPO fix above. Fix: added `ModelPackageStatusDetails`/`ModelPackageStatusItem` types
matching `types.ModelPackageStatusDetails`/`types.ModelPackageStatusItem`; `CreateModelPackage`
now populates an empty-but-present `ValidationStatuses: []ModelPackageStatusItem{}`. Files:
`models.go`, `model_packages.go`.

**Looks-wrong-but-correct traps for the next auditor:**

**Bug fixed this pass (CreateEndpoint/DescribeEndpoint/UpdateEndpoint wire + state gap):**

Before this fix, `InMemoryBackend.CreateEndpoint` (backend_new_ops.go) did two things wrong,
both in the highest-traffic Endpoint family:

1. It never validated that `EndpointConfigName` referenced an existing EndpointConfig — AWS
   returns `ValidationException: Could not find endpoint configuration "..."` for
   `CreateEndpoint`/`UpdateEndpoint` against an unknown config; gopherstack silently created
   the endpoint anyway.
2. `Endpoint.ProductionVariants` was typed as `[]ProductionVariant` (the *EndpointConfig-time*
   config shape: `InitialVariantWeight`/`InitialInstanceCount`) and was **never populated** —
   `CreateEndpoint` left it nil. Since `DescribeEndpoint`/`ListEndpoints` serialize this field
   directly, every `DescribeEndpoint` response silently omitted `ProductionVariants` entirely
   (a disguised no-op: the field existed in the struct and even had a JSON tag, but the write
   path that should populate it was missing). Real AWS `DescribeEndpoint` always returns
   `ProductionVariants` as `[]ProductionVariantSummary`, a *different* shape from
   `ProductionVariant`: `CurrentWeight`/`DesiredWeight` (not `InitialVariantWeight`),
   `CurrentInstanceCount`/`DesiredInstanceCount` (not `InitialInstanceCount`), plus
   `VariantStatus`. `UpdateEndpointWeightsAndCapacitiesFull` was also silently mutating the
   wrong field names (`InitialVariantWeight`/`InitialInstanceCount`) which happened to compile
   only because both structs used to share the same type — this was itself latent evidence the
   op never actually worked end-to-end against a real AWS-shaped response.

Fix: added `ProductionVariantSummary`/`ProductionVariantStatus` types matching the real SDK
(`aws-sdk-go-v2/service/sagemaker/types.ProductionVariantSummary`); `CreateEndpoint` now 404s
via `ErrEndpointConfigNotFound` when the config doesn't exist, and populates
`Desired*`/`VariantStatus:[{Status:"Creating"}]` from the config's variants; `UpdateEndpoint`
does the same 404 check and rebuilds Desired* from the new config while carrying forward
`Current*` from the previously-deployed variant (matches AWS: old capacity keeps serving
traffic until the new config finishes rolling out); `scheduleEndpointTransition` (the
Creating/Updating -> InService FSM timer) now sets `Current* = Desired*` and
`VariantStatus:[{Status:"InService"}]` the moment the endpoint reaches `InService`, matching
real AWS's converged-state behavior. Files: `backend_new_ops.go`, `backend_accuracy.go`. Tests
added/extended in `handler_accuracy2_test.go`
(`TestHandler_DescribeEndpoint_FullResponse`, `TestHandler_DescribeEndpoint_EventuallyInService`,
new `TestHandler_CreateEndpoint_UnknownEndpointConfig`).

**Looks-wrong-but-correct traps for the next auditor:**
- `awserr.New("ValidationException", awserr.ErrNotFound)` — the string literal passed as `msg`
  is NOT the `__type` value sent on the wire; `handler.go`'s `handleError` hardcodes
  `__type: "ValidationException"` for any error matching `errors.Is(err, awserr.ErrNotFound)`
  regardless of the sentinel's own message text. The message string only ends up embedded in
  the human-readable `message` field via `fmt.Errorf("%w: ...", sentinel, ...)`. This is
  correct/intentional, not a bug — don't "fix" the redundant-looking string literal.
- Tag lookups (`findTagMapLocked`) intentionally search ~20 different ARN-index maps in
  priority order; this is not a stub or inefficiency to "simplify" — every resource kind that
  supports `AddTags`/`ListTags`/`DeleteTags` needs its own branch since there's no unified
  resource registry (see pkgs-catalog.md's noted `gopherstack-drp` planned fix for exactly this
  kind of per-map boilerplate, which is a cross-service concern, not sagemaker-specific).
- Pagination throughout sagemaker is a hand-rolled stringified integer offset
  (`parseNextToken`/`strconv.Itoa`), not `pkgs/page`. This deviates from the pkgs-catalog
  convention but is wire-compatible (AWS `NextToken` is client-opaque) — flagged as a gap
  above, not fixed, since it's a cross-cutting refactor far outside a single-family bug-fix
  budget.
- Protocol is JSON (`awsjson1.1`, `X-Amz-Target: SageMaker.<Op>`), not REST-XML/REST-JSON.
  Timestamps are epoch-seconds `float64` via `epochSeconds()`, not ISO8601 strings — this is
  correct for this protocol; do not "fix" to ISO8601 strings.
- `ErrResourceNotFound` (`errors.go`) looks like the same "message string doesn't affect the
  wire `__type`" trap noted above for the generic `awserr.ErrNotFound` sentinel — it is NOT.
  `handleError` (`handler.go`) has a genuine, deliberate extra `case errors.Is(err,
  ErrResourceNotFound):` checked *before* the generic `ErrNotFound` case, which emits
  `__type: "ResourceNotFound"` instead of the blanket `"ValidationException"` the rest of the
  service emits for not-found. This exists because AIBenchmarkJob/AIRecommendationJob/
  AIWorkloadConfig/Job's real error deserializers (verified directly against
  `aws-sdk-go-v2/service/sagemaker/deserializers.go`) only recognize a `"ResourceNotFound"`
  wire exception — `"ValidationException"` would be wrong for these four families specifically.
  Do not collapse this into the generic `ErrNotFound` branch; do not add more sentinels
  wrapping `ErrResourceNotFound` for other (older, already-`ValidationException`-correct)
  families without re-verifying their real deserializer first.

## parity-4 (2026-07-25): 22 ops added by the aws-sdk-go-v2/service/sagemaker
v1.236.0 -> v1.261.0 bump

Implemented, not stubbed, all 22: **AIBenchmarkJob** (Create/Describe/Delete/Stop/List),
**AIRecommendationJob** (Create/Describe/Delete/Stop/List), **AIWorkloadConfig**
(Create/Describe/Delete/List), the generic **Job**/**JobSchemaVersion** family
(Create/Describe/Delete/Stop/List/DescribeJobSchemaVersion/ListJobSchemaVersions), and
**StartClusterHealthCheck**. New files: `ai_benchmark_jobs.go` + `handler_ai_benchmark_jobs.go`,
`ai_recommendation_jobs.go` + `handler_ai_recommendation_jobs.go`, `ai_workload_configs.go` +
`handler_ai_workload_configs.go`, `jobs.go` + `handler_jobs.go`; `cluster.go`/`handler_cluster.go`
gained `StartClusterHealthCheck`.

The generic `Job` type is a genuinely new resource kind (SageMaker's "model customization job"
API), not another name for `TrainingJob`/`ProcessingJob`/etc: it is keyed by `JobName` alone
(per `CreateJob`'s own doc — unique per account+region), `Describe`/`Delete`/`Stop` additionally
require a matching `JobCategory` (a category mismatch 404s — see `resolveJobLocked`), and it
carries an opaque `JobConfigDocument` validated only against `JobConfigSchemaVersion`, never the
`AlgorithmSpecification`/`ResourceConfig`/etc. shape every other job type has. It has its own
`JobSecondaryStatusTransition` type (deliberately not sharing `training_jobs.go`'s
`SecondaryStatusTransition`, despite the identical field shape) and its own store (`b.jobs`).

All three job-lifecycle families (`AIBenchmarkJob`, `AIRecommendationJob`, `Job`) use a real
`InProgress -> Completed` timer (`aiJobInProgressToCompleted`, 300ms, `lifecycle.go`) and a real
`InProgress -> Stopping -> Stopped` timer (`aiJobStoppingToStopped`, 150ms) — the same
`runDelayed`-based pattern `TrainingJob`/`ProcessingJob` already use, not a status flipped
synchronously in the same call. `StopAIBenchmarkJob`/`StopAIRecommendationJob` share one generic
implementation, `stopSimpleJobFSM` (`lifecycle.go`), to avoid duplicating the FSM by hand per
family; `StopJob` has its own (richer, `SecondaryStatusTransitions`-tracking) version since the
generic `Job` family's wire shape needs that history.

`CreateAIBenchmarkJob`/`CreateAIRecommendationJob` validate `AIWorkloadConfigIdentifier`
resolves to a real, already-created `AIWorkloadConfig` (`resolveAIWorkloadConfigLocked`, by name
or ARN via a real `aiWorkloadConfigARNIndex`, rebuilt on Restore in `rebuildARNIndexes`) — a
genuine cross-resource FK check, not assumed. See `gaps:` above for the three disclosed depth
limits (opaque `json.RawMessage` passthrough for several deeply-nested union/config fields,
`AIRecommendationJob.Recommendations` always empty, and the synthetic single-version
`JobConfigSchemaVersion` registry).

## parity-5 (2026-08-08): wire audit of 8 deferred families + AutoMLJobInputDataConfig

Audited the 8 families this file's parity-4 pass left fully deferred (pipeline/pipeline
execution, experiment/trial/trial component, feature store, lineage, labeling job, hub/hub
content, cluster, inference recommendations job) plus the previously-misnamed
"AutoMLJobInputDataConfig" gap (the real SDK field is `InputDataConfig`, not
`AutoMLJobInputDataConfig` — that name does not exist anywhere in
`aws-sdk-go-v2/service/sagemaker`). Every finding was verified against the pinned SDK module
(`v1.263.2`) source directly, not against this repo's own handler output, per this campaign's
rule; several bd-issue-title-style names (including the audit's own starting point) turned out
not to match the SDK and were corrected before implementing anything.

**Fixed (class-a: accepted-and-silently-dropped, or equivalent-severity wire-shape bugs):**

- `pipelines.go`/`handler_pipelines.go`: `DescribePipelineExecution` never returned
  `ParallelismConfiguration` even though `StartPipelineExecutionFull` already stored it on the
  backend struct — a pure silent-drop-on-read bug, the worst subtype named in this campaign's
  memory. `StartPipelineExecution`/`DescribePipelineExecution` also gained `PipelineVersionId`
  and `SelectiveExecutionConfig` (new `SelectiveExecutionConfig`/`SelectedStep` types), both real
  optional fields that were previously accepted by JSON (unknown-field silent success) and then
  thrown away.
- `experiments.go`/`trials.go`: `CreateExperiment`/`CreateTrial` didn't accept `DisplayName`
  (`CreateExperiment` also didn't accept `Description`) at all — real, commonly-used optional
  `Create*Input` fields, silently dropped until a separate `Update*` call. `ListExperiments`/
  `ListTrials` summaries also gained `DisplayName`/`LastModifiedTime` (real `ExperimentSummary`/
  `TrialSummary` fields, previously omitted).
- `trial_components.go`/`handler_trial_components.go`: the single highest-value fix this pass.
  `CreateTrialComponent` accepted only `TrialComponentName`/`Tags` — every other real
  `CreateTrialComponentInput` field (`StartTime`, `EndTime`, `Status`, `Parameters`,
  `InputArtifacts`, `OutputArtifacts`, `DisplayName`) was silently dropped, meaning this backend
  could not actually record what a trial component exists to record. Also fixed a genuine
  wire-shape bug of the same severity: `TrialComponent.Status` was a bare Go `string`, serialized
  as a JSON string; the real `DescribeTrialComponentOutput.Status`/`TrialComponentSummary.Status`
  is `types.TrialComponentStatus` (`{PrimaryStatus, Message}`), an object — a real AWS SDK JSON
  deserializer would fail outright on the old shape, not silently misparse it. The pre-existing
  `TestHandler_UpdateTrialComponent` test literally asserted the buggy bare-string shape
  (`"Status": "InProgress"` in, `descResp["Status"] == "InProgress"` out); it was updated to the
  correct `{PrimaryStatus: "InProgress"}` object shape as part of this fix, not left as
  bug-compatible.
- `feature_groups.go`/`handler_feature_groups.go`: `CreateFeatureGroup` didn't accept `RoleArn`
  or `Description` at all (`RoleArn` is what a real offline-store replication would use).
- `labeling.go`/`handler_labeling.go`: `CreateLabelingJob` stored `Tags` but
  `DescribeLabelingJob` never serialized them back out (a real, optional
  `DescribeLabelingJobOutput` field) — plus a second manifestation of the same class of bug in
  `LabelingJob.Tags`'s own `json:"-"` struct tag, which meant Tags were also silently dropped
  across a persistence snapshot/restore round-trip, not just the API response.
- `automl.go`/`handler_automl.go`: `InputDataConfig` (`[]types.AutoMLChannel`, `This member is
  required` on both `CreateAutoMLJobInput` and `DescribeAutoMLJobOutput`) was not modeled at all
  — new `AutoMLChannel`/`AutoMLDataSource`/`AutoMLS3DataSource` types added, accepted via the
  existing `SetAutoMLJobExtras` post-create-fields pattern, and always emitted (as `[]` when a
  client sends none, never `null`, matching the required-field contract — this needed an explicit
  non-nil-preserving `cloneAutoMLJob` fix since a naive `append(nil, emptySlice...)` collapses an
  intentionally-non-nil-but-empty slice back to `nil`).
- `inference_recommendations_jobs.go`/`handler_inference_recommendations_jobs.go`: `InputConfig`
  (`This member is required` on both `CreateInferenceRecommendationsJobInput` and
  `DescribeInferenceRecommendationsJobOutput`) had no struct field at all — added as opaque
  `json.RawMessage` passthrough (same convention as the parity-4 AI-job families' own deeply
  nested union fields — `RecommendationJobInputConfig` is a comparably deep union type). This is
  a distinct family from `AIRecommendationJob`/`ai_recommendation_jobs.go` (parity-4): different
  SDK ops, different store, no shared state — do not conflate the two in a future audit.
- `models.go`/`cluster.go`/`handler_cluster.go`: `CreateCluster` didn't accept `ClusterRole` or
  `VpcConfig` at all (`VpcConfig` reuses the existing shared type from `training_jobs.go`, not a
  new duplicate).

**Audited, no bug found (grade held/confirmed):**

- `hub.go`/`handler_hub.go` (`hub_hub_content`): already a thorough implementation —
  `S3StorageConfig` correctly nested (not flattened) on both request and response,
  `HubContentDependencies`, presigned URLs, and `CreateHubContentReference`/
  `UpdateHubContentReference` (ModelReference content) all real and wire-correct. No changes.
- `lineage.go`/`handler_lineage.go` (`lineage_action_artifact_context_association`): `Source`/
  `Properties`/`Description`/`Status`/`Tags` all round-trip correctly across
  Action/Artifact/Context CRUD; `QueryLineage` graph traversal and the single
  auto-provisioned `LineageGroup` (with an honest, correctly-typed not-found for
  `GetLineageGroupPolicy`, not a stub) verified. Only gap: `MetadataProperties` not accepted at
  Create (see `gaps:`).

**Deliberately not fixed this pass (class a/b, disclosed in `gaps:` rather than fixed):**
`CreatePipeline`/`UpdatePipeline`'s `PipelineDefinitionS3Location` (would need a real
cross-service S3 `GetObject` to resolve honestly — fabricating a definition would violate the
no-fabrication rule); `DescribePipeline`'s missing `PipelineVersionId` input param and several
missing optional response fields; `feature_store`'s `OnlineStoreConfig`/`OfflineStoreConfig`/
`ThroughputConfig` (the actual substance of Feature Store — a materially larger typed-struct
effort than this pass's bounded-fix budget); `cluster`'s `Orchestrator`/`AutoScaling`/
`NodeProvisioningMode`/`TieredStorageConfig`/`RestrictedInstanceGroups(Config)` (each a
nontrivial nested union type); `TrialComponent`/`Experiment`/`Trial`'s `CreatedBy`/
`LastModifiedBy`/`Source` (`types.UserContext` — no IAM-identity model to derive from, class d,
not fabricated); `lineage`'s `MetadataProperties` on Create.

## parity-6 (2026-08-08): CreateAutoMLJobV2/DescribeAutoMLJobV2 dedicated handlers

Fixes the actual gap bd issue `gopherstack-e39w` asked for. parity-5's own note claiming
`AutoMLJobInputDataConfig` "does not exist anywhere in aws-sdk-go-v2/service/sagemaker" was
wrong — verified directly against the pinned SDK (`v1.263.2`): it is
`CreateAutoMLJobV2Input.AutoMLJobInputDataConfig` (`api_op_CreateAutoMLJobV2.go:91`, `This
member is required`, `[]types.AutoMLJobChannel`), and also the corresponding
`DescribeAutoMLJobV2Output` field. It is a real, V2-only field distinct from V1's
`InputDataConfig` ([]types.AutoMLChannel) — parity-5 fixed the V1 field under a name that made
this look already-handled, when the V2 gap (what the issue title actually named) was untouched.

`handler_catalog.go` routed `CreateAutoMLJobV2`/`DescribeAutoMLJobV2` to the identical V1
handlers (`handleCreateAutoMLJob`/`handleDescribeAutoMLJob`). A V2 request's
`AutoMLJobInputDataConfig` and `AutoMLProblemTypeConfig` — both `This member is required` on
`CreateAutoMLJobV2Input` — were unknown JSON fields to the V1 request struct and silently
dropped; `DescribeAutoMLJobV2` never had a way to emit them back regardless. Full field-by-field
divergence (verified against `api_op_{Create,Describe}AutoMLJob{,V2}.go`):

- **Input, required, name differs**: `InputDataConfig` (V1) vs `AutoMLJobInputDataConfig` (V2) —
  different field name AND different element type (`types.AutoMLChannel` has
  `TargetAttributeName`/`SampleWeightAttributeName`; `types.AutoMLJobChannel` does not).
- **Input, required, V2 only**: `AutoMLProblemTypeConfig` (a 5-member tagged union —
  `ImageClassificationJobConfig`/`TabularJobConfig`/`TextClassificationJobConfig`/
  `TextGenerationJobConfig`/`TimeSeriesForecastingJobConfig`). V1 has no equivalent required
  field — the closest V1 fields, `ProblemType` (optional enum) and `AutoMLJobConfig`, are both
  optional and structurally unrelated.
- **Input, optional, V2 only**: `AutoMLComputeConfig`, `DataSplitConfig`, `SecurityConfig`. V1
  has none of these.
- **Input, optional, V1 only**: `AutoMLJobConfig`, `GenerateCandidateDefinitionsOnly`,
  `ProblemType`.
- **Input, optional, both (same type)**: `AutoMLJobObjective`, `ModelDeployConfig`, `Tags`.
- **Output**: mirrors the input divergence — `DescribeAutoMLJobV2Output` additionally returns
  `AutoMLProblemTypeConfigName` (derived, not a Create input) and `AutoMLComputeConfig`; V1's
  `DescribeAutoMLJobOutput` additionally returns `AutoMLJobConfig`/`ProblemType`/
  `GenerateCandidateDefinitionsOnly`. `ResolvedAttributes` exists on both but as different types
  (`types.ResolvedAttributes` vs `types.AutoMLResolvedAttributes`) — neither is modeled by this
  backend (pre-existing V1 depth limit, not new).
- **Create*Output for both versions is identical**: just `AutoMLJobArn`.

**Decision: separate handlers, not a shared one.** The required-field divergence alone rules out
a shared handler — a JSON struct that satisfies both `InputDataConfig`/`AutoMLJobInputDataConfig`
without misnaming one of them cannot exist, and `AutoMLProblemTypeConfig` has no V1 analogue to
silently reuse. `handleCreateAutoMLJobV2`/`handleDescribeAutoMLJobV2` (new `handler_automl_v2.go`)
were added; both versions still share the same `b.autoMLJobs` store and the same `AutoMLJob`
struct (AWS job names are unique across V1/V2 in the same account+region), but each op now
parses/emits its own accurate field subset via an explicit `map[string]any` response rather than
relying on the struct's own JSON tags.

This required also changing `handleDescribeAutoMLJob` (V1) from `json.Marshal(result)` (the
struct's default tags) to the same explicit-map style: since `AutoMLJob` now carries V2-only
fields (`AutoMLJobInputDataConfig`, `AutoMLProblemTypeConfig`), a V1 `Describe` of a job created
via `CreateAutoMLJobV2` would otherwise leak them into the V1 response shape — caught by a test
(`TestHandler_DescribeAutoMLJobV1_OmitsV2Fields`) asserting isolation both directions.

`AutoMLProblemTypeConfigName` is derived at Describe time from which single top-level key is
present in the opaque `AutoMLProblemTypeConfig` payload (`automlProblemTypeConfigName` in
`automl_v2.go`), matching the member->JSON-key mapping in `serializers.go`'s
`awsAwsjson11_serializeDocumentAutoMLProblemTypeConfig` — this is a real, verifiable derivation,
not a guess. `AutoMLProblemTypeConfig`'s member configs themselves (`TabularJobConfig` etc.) are
carried opaque (see `gaps:`), consistent with this service's established convention for other
deeply-nested unions.

Pre-fix verification: wrote `TestHandler_CreateAutoMLJobV2_RoundTrip` (table-driven,
full/minimal cases) against the pre-fix code first — both subtests failed with `DescribeAutoMLJobV2
must always emit AutoMLJobInputDataConfig`, confirming the field was silently absent from the V2
Describe response entirely (the exact bug class this issue names). All AutoML tests pass after
the fix; `go build ./...`, `go test -race ./services/sagemaker/...`, and
`golangci-lint run ./services/sagemaker/...` are clean.

**Bounded remainder (feature_store, DescribePipeline), same pass:**

`feature_groups.go`'s `CreateFeatureGroupOptions`/`FeatureGroup` gained `OnlineStoreConfig`
(`EnableOnlineStore`/`StorageType`/`SecurityConfig.KmsKeyId`/`TtlDuration`), `OfflineStoreConfig`
(`S3StorageConfig`/`DataCatalogConfig`/`TableFormat`/`DisableGlueTableCreation`), and
`ThroughputConfig` (`ThroughputMode`/`ProvisionedReadCapacityUnits`/`ProvisionedWriteCapacityUnits`)
— all verified field-by-field against `types.{OnlineStoreConfig,OfflineStoreConfig,
ThroughputConfig,ThroughputConfigDescription,S3StorageConfig,DataCatalogConfig,
OnlineStoreSecurityConfig,TtlDuration}` in `types/types.go`. None of these are unions; all are
small flat structs, so all are fully typed rather than carried opaque. `ThroughputConfig` (Create
input) and `ThroughputConfigDescription` (Describe output) are distinct SDK type names with
identical fields — this backend uses one Go type, `ThroughputConfig`, for both, since the wire
shape is the same either direction. Pre-fix verification:
`TestHandler_CreateFeatureGroup_StoreConfigsRoundTrip` failed with `OnlineStoreConfig must
round-trip` (the field was entirely absent from the Describe response) before the fix.

`pipelines.go`'s `DescribePipeline` gained a `versionID int64` parameter
(`DescribePipelineInput.PipelineVersionId`) — when non-zero, it looks up that version in the
existing `pipelineVersionsStore` (already populated by every `CreatePipeline`/`UpdatePipeline`
call, per parity-5's pipeline-version-history work) and substitutes that version's
`PipelineDefinition`; an unknown version ID now returns `ErrPipelineNotFound` instead of silently
falling back to the current version. It also now returns `lastRunTime`
(`DescribePipelineOutput.LastRunTime`), computed as the max `StartTime` across the pipeline's
`PipelineExecution`s (matched by `PipelineArn`) — a real derived value, omitted (not zero-faked)
when the pipeline has never run. Pre-fix verification:
`TestHandler_DescribePipeline_PipelineVersionId/version_1_returns_original_definition` and
`/unknown_version_is_not_found` both failed (version 1 returned the current v2 definition; the
unknown version 99 returned 200 with the current pipeline instead of erroring), and
`TestHandler_DescribePipeline_LastRunTime` failed with `a pipeline that has run must emit
LastRunTime` — confirming the field was never emitted regardless of execution history.

Not attempted, per this issue's explicit scope: `PipelineDefinitionS3Location` (needs a real
cross-service S3 fetch) and the cluster family's six nested union/struct types
(`Orchestrator`/`AutoScaling`/`NodeProvisioningMode`/`TieredStorageConfig`/
`RestrictedInstanceGroups(Config)`).

Gates for the full pass: `go build ./...`, `go test -race ./services/sagemaker/...`, and
`golangci-lint run ./services/sagemaker/...` all clean, zero `nolint:{cyclop,gocyclo,gocognit,
funlen}` added.

## gopherstack-i359 (2026-08-09): cluster's five nested types + PipelineDefinitionS3Location rejection

Closes most of the "not attempted" list parity-5 left at the end of its section: five of
`cluster`'s six remaining nested types (`Orchestrator`/`AutoScaling`/`NodeProvisioningMode`/
`TieredStorageConfig`, plus a pre-existing persistence bug in `ClusterRole`/`VpcConfig`), and
`PipelineDefinitionS3Location`'s accept-and-drop. `RestrictedInstanceGroups(Config)` remains
untouched — see below.

**`ClusterOrchestrator` is not a wire union.** AWS's docs for `Orchestrator`
(`api_op_CreateCluster.go:72-78`, `sagemaker@v1.263.2`) read like a discriminated union
("you must provide exactly one orchestrator configuration: either Eks or Slurm"), and this issue
flagged it as "likely a union" needing care. Checked against botocore
(`sagemaker/2017-07-24@1.43.56 service-2.json.gz`, `metadata.protocol == "json"`,
`metadata.jsonVersion == "1.1"`): `shapes.ClusterOrchestrator.type == "structure"`, not
`"union"`. `serializers.go:27593-27612`'s `awsAwsjson11_serializeDocumentClusterOrchestrator`
confirms: it emits `Eks` and `Slurm` as two independent optional object keys, not a tagged
member. So `ClusterOrchestrator` is modeled here as a plain struct with two `*optional` pointer
fields (`ClusterOrchestratorEksConfig{ClusterArn}`, `ClusterOrchestratorSlurmConfig{
SlurmConfigStrategy}`), and the "exactly one" business rule is enforced as a runtime
`ValidationException` (`validateClusterOrchestratorLocked` in `cluster.go`), not a Go interface
union like `pkgs`/other services use for real smithy `@union` shapes.

**Fully modeled, all small flat structs verified field-by-field against `types/types.go`**
(`sagemaker@v1.263.2`): `ClusterAutoScalingConfig` (`Mode`/`AutoScalerType`, :4492),
`ClusterOrchestrator`/`ClusterOrchestratorEksConfig`/`ClusterOrchestratorSlurmConfig` (:5456,
:5470, :5483), `ClusterTieredStorageConfig` (`Mode`/`InstanceMemoryAllocationPercentage`, :5847),
and `NodeProvisioningMode` (a plain string enum, `ClusterNodeProvisioningMode`, :2674 — currently
only one real value, `Continuous`, but stored/echoed as an opaque string like `NodeRecovery`
already is, not validated against the enum). `DescribeClusterOutput.AutoScaling` uses a distinct
`ClusterAutoScalingConfigOutput` type that adds a required `Status` field (:4507) — this backend
reports it as `InService` once `AutoScaling` is set, the same no-async-provisioning convention
`instanceGroupStatusInService` already uses for instance groups; no other field is fabricated.
`Orchestrator`/`TieredStorageConfig` use the *same* Go SDK type on both `CreateClusterInput` and
`DescribeClusterOutput` (confirmed by reading both `api_op_CreateCluster.go` and
`api_op_DescribeCluster.go`), so no separate output shape was needed for those two.

**Left entirely untouched: `RestrictedInstanceGroups`/`RestrictedInstanceGroupsConfig`.**
`ClusterRestrictedInstanceGroupSpecification` (`types/types.go:5622`) has ~10 fields of its own
and nests `EnvironmentConfig` (`->FSxLustreConfig`), a real 3-member `ClusterInstanceStorageConfig`
union (`EbsVolumeConfig`/`FsxLustreConfig`/`FsxOpenZfsConfig`, a genuine Go `interface` union this
time — confirmed via `types/types.go:5107`'s `isClusterInstanceStorageConfig()` marker methods),
and `ScheduledUpdateConfig` (`->DeploymentConfiguration->RollingDeploymentPolicy`/
`AlarmDetails`) — six more nested types beyond the top-level spec. Per this campaign's rule
(medialive precedent, restated in this issue): a nested config whose fields are only partly
parsed is worse than an absent one, since callers can't tell what survived. Rather than shave
fields off this one to fit it into the same pass as the other four, it was left completely alone
— no parsing, no partial struct, no explicit-rejection error either (the issue's explicit
guidance was to "leave that one untouched," which explicit rejection would not be). Full support
would need all six additional types modeled, including the real union.

**Found and fixed in passing: a pre-existing `ClusterRole`/`VpcConfig` persistence bug.**
parity-5 added `ClusterRole`/`VpcConfig` to `Cluster`, `CreateClusterOptions`, and the
Create/Describe handlers, but never added them to `persistedCluster`
(`persistence.go`'s hand-maintained DTO for `Cluster`, needed because `Cluster.Nodes` carries
`json:"-"`) — so both fields round-tripped correctly in memory but were silently dropped across
every `Snapshot`/`Restore` cycle. Found while adding the four new fields to the same DTO; fixed
alongside them. Pre-fix repro (in a throwaway `git worktree` at the pre-`gopherstack-i359` HEAD,
using only fields that already existed then): create a cluster with `ClusterRole`+`VpcConfig`,
`Snapshot`, `Restore` into a fresh backend, `DescribeCluster` — `ClusterRole` came back `""` and
`VpcConfig` came back absent. `TestPersistenceRoundtrip_ClusterFullFields` now guards this
(and the four new fields) permanently.

**`PipelineDefinitionS3Location` rejected explicitly, not silently dropped.**
`CreatePipelineInput`/`UpdatePipelineInput` (`api_op_CreatePipeline.go:59`,
`api_op_UpdatePipeline.go:43`) both accept `PipelineDefinitionS3Location` (`Bucket`/`ObjectKey`/
optional `VersionId`, `types/types.go:17313`) as an S3-backed alternative to inline
`PipelineDefinition`. Honoring it for real needs a cross-service S3 `GetObject` call — the
registry-wiring pattern `cli.go` uses for `wireStepFunctionsServiceIntegrations`/
`wireAppConfigDeployments` — which touches `cli.go`, owned by another agent this session, so it
is out of scope here. Rather than continue accepting-and-silently-dropping the field (a client
relying on it today gets a pipeline created with an empty `PipelineDefinition` and no error, the
worst failure mode), `handleCreatePipelineFull`/`handleUpdatePipelineFull` now reject any request
that sets it with a `ValidationException`, following this service's own established
explicit-rejection precedent (`images.go`'s `UpdateImage` rejecting unsupported
`DeleteProperties` values via `ErrValidation`). Full support would need: a wired
`services/s3.InMemoryBackend` reference (or interface, matching `organizations_directory.go`'s
pattern in `cloudformation`), a `GetObject(bucket, key, versionID)` call resolving the definition
body, and using it as `PipelineDefinition` — real work, not attempted here.

Pre-fix verification: `TestPrefixCheck_ClusterRoleAndVpcConfig_SurviveSnapshotRestore` (temporary,
run against a `git worktree` at pre-`gopherstack-i359` HEAD, then discarded) failed as described
above. The `AutoScaling`/`Orchestrator`/`NodeProvisioningMode`/`TieredStorageConfig`/
`PipelineDefinitionS3Location` behaviors didn't exist in any form before this pass — there is no
meaningful "before" state beyond "the field is not in the request struct at all," already
established by reading the pre-edit `cluster.go`/`handler_cluster.go`/`handler_pipelines.go`.

All new/changed behavior verified through the real `aws-sdk-go-v2/service/sagemaker@v1.263.2`
client (`newTestSageMakerClient`), not hand-built JSON bodies:
`TestHandler_CreateCluster_NestedTypes_RealClient`,
`TestHandler_UpdateCluster_NestedTypes_RealClient`,
`TestHandler_CreatePipeline_S3Location_Rejected_RealClient`. Snapshot version not bumped — every
new field is additive with `omitempty`. Gates: `go build ./...`,
`go test -race ./services/sagemaker/... .`, and `golangci-lint run ./services/sagemaker/...` all
clean; zero `nolint:{cyclop,gocyclo,gocognit,funlen}` added.

## gopherstack-i359 (session 3, 2026-08-10): real S3 pipeline definitions; RestrictedInstanceGroups re-confirmed deferred

Closes the two items session 2 left open: wires real S3 fetching for
`PipelineDefinitionS3Location` (`cli.go` was owned by another agent in session 2), and makes a
fresh, deeper-researched call on `RestrictedInstanceGroups` rather than repeating the prior
deferral without re-checking it.

**`PipelineDefinitionS3Location` now genuinely fetches from S3.** New
`services/sagemaker/s3pipeline.go`: an `S3Accessor` interface
(`GetObject(ctx, *s3.GetObjectInput) (*s3.GetObjectOutput, error)`, identical shape to
`services/mgn/s3import.go`'s own `S3Accessor` — both are satisfied directly by
`services/s3.InMemoryBackend`, no adapter needed), `InMemoryBackend.SetS3Backend`/`s3Backend`
(same lock-guarded-field pattern as `services/mgn`), and `readPipelineDefinitionFromS3` (fetches,
caps the read at 64MiB — matching `services/mgn`'s identical import-source safety cap — and
errors via the sentinel `errPipelineDefinitionUnreadable` on a missing backend, missing
bucket/key, or an empty object). `cli.go` gained `wireSageMakerS3` (new function, same shape as
the pre-existing `wireMGNS3`/`wireDynamoDBS3`) called from `wireStorageAndSecretsIntegrations`.
`handleCreatePipelineFull`/`handleUpdatePipelineFull` now resolve `PipelineDefinitionS3Location`
through `readPipelineDefinitionFromS3` and use the fetched body as `PipelineDefinition`, instead
of unconditionally rejecting it. The `ValidationException` rejection path is retained, but now
only fires for the genuinely-unreadable case (no S3 backend wired, object missing, or a real
`GetObject`/read failure) — an honest error, never a fabricated definition, consistent with this
campaign's no-fabrication rule.

Per this repo's non-negotiable wiring-test requirement: `cli_sagemaker_s3_pipeline_wiring_test.go`
drives `initializeServices(appCtx)` (the function `Run()` actually calls, not `wireSageMakerS3`
called directly) through a real `aws-sdk-go-v2/service/sagemaker` client, creates a bucket and
object through the real S3 backend, calls `CreatePipeline` with `PipelineDefinitionS3Location`,
and asserts the fetched body round-trips through `DescribePipeline`. Verified with teeth: deleted
the `wireSageMakerS3(...)` call site from `wireStorageAndSecretsIntegrations` (not the helper
function) and re-ran the test — it failed with `ValidationException: ... no S3 backend
configured`, confirming the test is sensitive to the actual composition-root call site, not just
the helper's own correctness. Restored the call site and confirmed green again.

Package-level tests: `handler_pipelines_test.go` renamed the two prior
`*_S3Location_Rejected(_RealClient)` tests to `*_S3Location_UnreadableRejected(_RealClient)` (same
assertions — `newTestHandler` never wires an S3 backend, so the unreadable path still fires and
still returns `ValidationException`) and added `TestHandler_CreatePipeline_S3Location_Fetched`/
`TestHandler_UpdatePipeline_S3Location_Fetched` against a lightweight in-package
`mockPipelineS3` (mirrors `services/mgn`'s own test-only `mockS3` helper). Pre-fix verification:
copied the new/changed test files into a throwaway `git worktree` at the pre-session-3 HEAD
(pre-fix `s3pipeline.go` doesn't exist there) — `go vet` failed with `h.Backend.SetS3Backend
undefined`, confirming the new tests exercise code that did not exist before this pass; worktree
discarded after.

**Persistence: no DTO change needed, but guarded anyway.** `Pipeline` has no hand-maintained
persisted DTO (unlike `Cluster`'s `persistedCluster`, needed only because `Cluster.Nodes` carries
`json:"-"`) — it round-trips generically through `registry.SnapshotAll`/`RestoreAll` using its own
JSON tags, and `PipelineDefinition` already existed as a field before this pass. So there was
nothing new to add to `persistence.go`. Added
`TestPersistenceRoundtrip_PipelineDefinitionFromS3` anyway, both as a regression guard on the
new code path (a pipeline created from a fetched S3 definition round-trips its
`PipelineDefinition` through `Snapshot`/`Restore` like any other pipeline) and as a tripwire
against a future hand-maintained Pipeline DTO silently forgetting the field, the exact bug class
session 2 found for `Cluster`. Snapshot version not bumped — no new persisted field exists.

**`RestrictedInstanceGroups`: re-examined, still deferred — the scope is larger than previously
written up, not smaller.** Read directly against `types/types.go` (`sagemaker@v1.263.2`) rather
than trusting session 2's summary. Confirmed real: `ClusterInstanceStorageConfig`
(`types/types.go:5107`) is declared `interface { isClusterInstanceStorageConfig() }` with three
member wrapper types (`ClusterInstanceStorageConfigMemberEbsVolumeConfig`/
`MemberFsxLustreConfig`/`MemberFsxOpenZfsConfig`) — a genuine discriminated union, unlike
`ClusterOrchestrator` (session 2 found that one is a plain struct despite reading like a union in
prose). `serializers.go`'s `case *types.ClusterInstanceStorageConfigMemberEbsVolumeConfig:` etc.
confirm each member serializes under its own field name, the expected union wire shape.

Beyond the union, the full type tree under `ClusterRestrictedInstanceGroupSpecification`
(`types/types.go:5622`, the `CreateClusterInput`/`UpdateClusterInput` shape) is:
`EnvironmentConfig` (`types/types.go:8395`) `->` `FSxLustreConfig` (`types/types.go:9152`, 2
required fields); `InstanceStorageConfigs []ClusterInstanceStorageConfig` (the union above, whose
3 members reference `ClusterEbsVolumeConfig`/`ClusterFsxLustreConfig`/`ClusterFsxOpenZfsConfig`,
`types/types.go:4548`/`4683`/`4704`); and `ScheduledUpdateConfig` (`types/types.go:20564`) `->`
`DeploymentConfiguration` (`types/types.go:7106`) `->` `RollingDeploymentPolicy`
(`types/types.go:20006`, itself nesting `CapacitySizeConfig` twice, `types/types.go:3824`) plus
`AutoRollbackConfiguration []AlarmDetails` (`types/types.go:841`). That is 8 new leaf/union types
once the union's members and `RollingDeploymentPolicy`'s nested `CapacitySizeConfig` are each
counted, not the "six" session 2 wrote down.

On top of that: `CreateClusterInput`/`UpdateClusterInput`/`DescribeClusterOutput` all also carry
`RestrictedInstanceGroupsConfig` (`types/types.go:5598`) — a field session 2's write-up never
named — which requires its own `SharedEnvironmentConfig` (`types/types.go:5727`,
`ClusterSharedEnvironmentConfig`: a required `FSxLustreConfig` plus a required
`FSxLustreDeletionPolicy` enum). So "`RestrictedInstanceGroups`" is honestly two independent
top-level fields, not one, and the combined faithful-modeling effort is comparable in size to all
four of session 2's cluster fixes (`Orchestrator`/`AutoScaling`/`NodeProvisioningMode`/
`TieredStorageConfig`) combined — while this session's other mandatory deliverable
(`PipelineDefinitionS3Location`, including its non-negotiable `cli.go` wiring-test proof) already
consumed a full pass's budget on its own.

Per this campaign's standing rule (medialive precedent, restated for this issue in session 2 and
again here): a nested config whose fields are only partly parsed is worse than an absent one,
because callers cannot tell what survived. Splitting the type tree to fit the remaining budget
would violate that rule as surely as skipping validation would. So `RestrictedInstanceGroups`/
`RestrictedInstanceGroupsConfig` are left untouched a third time — this time with the full,
verified type tree on record (`gaps:` entry above) so a future pass can scope and budget for it
accurately in one sitting, rather than re-deriving it from scratch a fourth time.

Gates for this session: `go build ./...`, `go test -race ./services/sagemaker/... .`, and
`golangci-lint run ./services/sagemaker/...` all clean; zero
`nolint:{cyclop,gocyclo,gocognit,funlen}` added.

## parity-7 (2026-08-13, gopherstack-oc9v): Domain/App/Space/UserProfile inline-struct sweep

gopherstack-oc9v sized a repo-wide blind spot: handlers that declare their request as an
anonymous inline `struct{...}` are invisible to both wire-sweep tools, which match on named
types. sagemaker held 362 of the repo's 1487 candidates — the largest concentration, and the
only service proven (via `ListAssociations`, fixed gopherstack-cgq3) to hide real bugs.

Per `PARITY.md`'s own frontmatter/families at the start of this session: sagemaker was already
graded A with an extensive per-op/family audit history (parity-4/5/6). The `domain_app_
userprofile_space` family was explicitly marked `deferred`/`partial` — "Domain/App/UserProfile
not otherwise wire-audited this pass" — making it the correct, honestly-scoped starting point:
real uncovered surface, not a re-derivation of already-verified work.

**Enumerated vs. converted vs. audited:** all 19 inline `struct{...}` request declarations
across `handler_domains.go` (5), `handler_apps.go` (4), `handler_spaces.go` (5), and
`handler_user_profiles.go` (5) were converted to named types (`createDomainInput`,
`describeDomainInput`, `listDomainsInput`, `deleteDomainInput`, `updateDomainInput`, and the
equivalent for App/Space/UserProfile) and wire-audited field-by-field against the pinned SDK
(`v1.263.2`: `api_op_{Create,Describe,List,Delete,Update}{Domain,App,Space,UserProfile}.go`).
This is a small slice of the repo-wide 362/1487, scoped deliberately (see gopherstack-oc9v's own
"work in deterministic order, state exactly where you stopped" instruction) rather than a shallow
pass over all of them — see that issue for what remains repo-wide.

**Findings, classified (a=absent entirely, b=wrong name, c=deliberately unmodelled):**

- (a) `CreateDomainInput.DefaultUserSettings` — `This member is required` — did not exist on the
  wire struct at all; a real client's mandatory field was silently accepted-and-dropped instead
  of rejected. Now required (`ValidationException` if absent) and stored as opaque
  `json.RawMessage` passthrough (`domains.go`).
- (a) `CreateAppInput.SpaceName` — the real, documented alternative to `UserProfileName`
  ("The name of the space. If this value is not set, then UserProfileName must be set.") — did
  not exist on the wire struct. A client with only a Space (no UserProfile) could never launch an
  app through `CreateApp`, even though this backend has modeled Spaces since `spaces.go`. Fixed:
  `CreateApp`/`DescribeApp`/`DeleteApp` now accept `SpaceName` as an alternative identity to
  `UserProfileName`, validated as mutually exclusive-and-required (one, not both, not neither).
- (a) `ListDomainsInput.MaxResults`, `ListAppsInput.{MaxResults,SortBy,SortOrder,
  SpaceNameEquals,UserProfileNameEquals}`, `ListSpacesInput.{MaxResults,SortBy,SortOrder,
  SpaceNameContains}`, `ListUserProfilesInput.{MaxResults,SortBy,SortOrder,
  UserProfileNameContains}` — none of these nine real filter/sort/pagination fields were modeled
  anywhere in the family; every `List*` silently used a fixed page size and insertion-order-ish
  sort regardless of what the client asked for. This is the exact "parsed field, silently
  ignored" defect class gopherstack-oc9v exists to find. All nine are now real: `MaxResults`
  caps the page via the existing `paginateSlice` helper; `SortBy`/`SortOrder` reorder by
  `CreationTime`/`LastModifiedTime` (the real `AppSortKey`/`SpaceSortKey`/`UserProfileSortKey`
  enum values, confirmed against `types/enums.go`); the four `*Equals`/`*Contains` filters narrow
  the result set.
- (c) `CreateAppInput.ResourceSpec`, `CreateSpaceInput.{OwnershipSettings,SpaceSettings,
  SpaceSharingSettings}`, `CreateUserProfileInput.UserSettings`,
  `CreateDomainInput.{DefaultSpaceSettings,DomainSettings}`, `UpdateDomainInput.
  DomainSettingsForUpdate` — deeply-nested config/union shapes (`UserSettings` alone has ~20
  app-specific sub-configs). Modeled as opaque `json.RawMessage` passthrough, the established
  convention in this file (`ai_workload_configs.go`, `algorithms.go`) for shapes materially
  larger than a single pass's budget — every field a client sends round-trips exactly through
  Create→Describe and a persistence Snapshot/Restore cycle; nothing is fabricated.
- (a) `CreateUserProfileInput.{SingleSignOnUserIdentifier,SingleSignOnUserValue}` — simple flat
  strings, previously absent; now modeled and round-tripped.
- `UpdateDomain` was a pure no-op beyond bumping `LastModifiedTime` — none of
  `UpdateDomainInput`'s nine real optional fields were accepted. Now a real partial update
  (`UpdateDomainOptions`/`applyUpdateDomainOptions`): each field overwrites only when the client
  supplies it, leaving the rest of the domain untouched, matching AWS's partial-update semantics.

**A second bug the conversion itself surfaced** (the exact pattern gopherstack-oc9v warned about
— "conversion itself surfaces gaps"): adding `SpaceName` to `appKey` fixed the request-shape gap,
but `store_domain.go`'s `appsStore`/`appsStoreRO` had their own separately-hand-written `keyFn`
closures that built the `App` table's primary key — and those were not updated alongside
`appKey`. The result: `CreateApp` computed its duplicate-check key with the new 5-field
`appKeyString` (including `SpaceName`), but `store.Table.Put` computed a *different* key via the
stale 4-field closure, so a Space-owned app was stored under one key and looked up under another
— `DescribeApp` returned `ResourceNotFound` for an app that had just been created successfully.
Caught immediately by `TestHandler_CreateApp_SpaceOwned` (added this pass) before this reached
any shared branch; fixed by updating both closures in `store_domain.go` to match `appKey`'s new
shape.

**Tests:** every fix above has a table-driven or targeted test that asserts on the actual
narrowed/reordered/capped/rejected result — not just that the request parsed. Verified against
unfixed code by temporarily reverting three representative fixes (the `DefaultUserSettings`
requiredness check, `ListDomains`' `MaxResults` wiring, and `ListApps`' `UserProfileNameEquals`
filter) one at a time and confirming the corresponding test fails, then restoring — the same
protocol used for the rest. `TestPersistenceRoundtrip_Domain` confirms the new fields survive a
Snapshot/Restore cycle, not just an in-process Describe.

**Not touched this pass:** `DescribeApp`/`DescribeDomain`'s remaining server-only derived fields
(see `gaps:` above); the internal structure of `UserSettings`/`SpaceSettings`/etc.; any of the
other 343 (362 minus the 19 converted here) inline structs elsewhere in this service —
gopherstack-oc9v remains open for those.

Gates for this session: `go build ./...`, `go vet ./services/sagemaker/...`,
`go test -race ./services/sagemaker/...`, `go fix -diff ./services/sagemaker/...` (no diff), and
`golangci-lint run ./services/sagemaker/...` all clean; zero
`nolint:{cyclop,gocyclo,gocognit,funlen}` added.

## parity-8 (2026-08-21, gopherstack-oc9v): ML Lineage family (Action/Artifact/Context/
Association/LineageGroup/QueryLineage) inline-struct sweep

Second pass of the gopherstack-oc9v campaign, sized at 362 candidate anonymous inline request
structs in sagemaker, 343 remaining after parity-7's Domain/App/Space/UserProfile family (19
converted). Per PARITY.md's own boundary note from parity-7 ("any of the other 343 ... elsewhere
in this service — gopherstack-oc9v remains open for those"), this pass took the next coherent,
self-contained family: all 19 remaining anonymous `struct{...}` request declarations in
`handler_lineage.go` (`grep -c 'var req struct {' handler_lineage.go` = 19, exactly matching
343's per-file count) — CreateArtifact/DescribeArtifact/UpdateArtifact/DeleteArtifact/
ListArtifacts, CreateContext/DescribeContext/UpdateContext/DeleteContext/ListContexts,
DescribeAction/UpdateAction/DeleteAction/ListActions, DeleteAssociation,
DescribeLineageGroup/ListLineageGroups/GetLineageGroupPolicy, QueryLineage. **324 of sagemaker's
362 inline structs now remain** (362 − 19 parity-7 − 19 parity-8). This pass did not touch any
other family; PARITY.md's remaining 40+ `partial`/`deferred` entries and every other service file
are unaudited by this pass.

This family had already been wire-audited for *content* correctness in parity-5 (`lineage_action_
artifact_context_association: {status: ok, ...}`) and was clean except two disclosed gaps:
`ListAssociations`' six-then-seven absent members (fixed gopherstack-cgq3, the proof case for the
whole campaign) and `MetadataProperties` on `CreateAction`/`CreateArtifact`. Converting the
remaining 19 to named types re-confirmed the parity-5 finding was accurate, then went further:
diffing every converted struct field-by-field against `aws-sdk-go-v2/service/sagemaker@v1.263.2`
turned up five more absent-member groups parity-5's narrower "is Source/Properties/Tags correct"
pass had not been scoped to catch.

**Absent members added, per op, with SDK file:line:**

- `CreateArtifactInput.MetadataProperties` (`types/types.go:13617..13631`, `*types.
  MetadataProperties{CommitId,GeneratedBy,ProjectId,Repository}`) — the parity-5-disclosed gap,
  now fixed: a flat 4-string struct, no reason to defer it to opaque passthrough like the
  Domain/App family's genuinely-huge configs. Threaded through `Artifact.MetadataProperties` and
  returned by `DescribeArtifact`. **`CreateActionInput.MetadataProperties`
  (`api_op_CreateAction.go`) had the identical gap** on `createActionRequest` — a named type, so
  technically outside this pass's inline-struct scope, but fixed alongside CreateArtifact's since
  it is the same root cause on the same shared type; threaded through `Action.MetadataProperties`
  and returned by `DescribeAction`.
- `DeleteArtifactInput.Source` (`api_op_DeleteArtifact.go:28-37`) — entirely absent. Real docs
  (docs.aws.amazon.com/sagemaker/latest/APIReference/API_DeleteArtifact.html): "Deletes an
  artifact. Either ArtifactArn or Source must be specified" — neither field is marked required on
  the Go struct because it's an either/or. Before this fix, `DeleteArtifact` unconditionally
  required `ArtifactArn`, so a client that (correctly, per the real API) supplied only `Source`
  got a `ValidationException` for a well-formed request. Now `Source.SourceUri` is a real
  alternative identity (`artifactArnBySourceURI`, deterministic lowest-ARN tie-break when
  multiple artifacts share a `SourceUri` — undocumented by AWS, disclosed here rather than
  guessed at silently).
- `ListArtifactsInput.{CreatedAfter,CreatedBefore,MaxResults,SortBy,SortOrder}`,
  `ListContextsInput.{CreatedAfter,CreatedBefore,MaxResults,SortBy,SortOrder}`,
  `ListActionsInput.{CreatedAfter,CreatedBefore,MaxResults,SortBy,SortOrder}` (`api_op_
  List{Artifacts,Contexts,Actions}.go`) — fifteen fields across three ops, all silently ignored
  before this pass (fixed page size, arbitrary ARN/name order). The exact "parsed field, silently
  dropped" defect class this campaign exists to find, same shape as parity-7's `ListDomains`/
  `ListApps`. All fifteen are now real: `MaxResults` caps the page via `paginateSlice`;
  `SortBy`/`SortOrder` reorder by `CreationTime` (default, all three) or `Name`
  (`ListContexts`/`ListActions` only — `SortArtifactsBy` has a single enum value, `CreationTime`,
  `types/enums.go:9056-9061`, so `ListArtifacts` correctly has no `Name` sort key);
  `CreatedAfter`/`CreatedBefore` filter on `CreationTime`. `ListContexts`/`ListActions` share one
  new generic helper (`filterSortPaginateByNameOrTime`, `list_helpers.go`) since their filter/sort
  shape is identical apart from field accessors — `ListArtifacts` does not share it since it lacks
  the `Name` sort key.
- `ListLineageGroupsInput.{CreatedAfter,CreatedBefore,MaxResults,SortBy,SortOrder}` (`api_op_
  ListLineageGroups.go`) — absent, and easy to dismiss as pointless since this backend only ever
  has one auto-provisioned lineage group. That's exactly why it was still a real bug: before this
  fix, `ListLineageGroups` returned the singleton unconditionally regardless of what
  `CreatedAfter`/`CreatedBefore` asked for — a `CreatedAfter` window that should exclude the one
  group still silently returned it. Fixed for real (`TestHandler_ListLineageGroups_CreatedWindow`
  asserts a future `CreatedAfter` returns an *empty* list). `SortBy`/`SortOrder` are accepted but
  are a genuine, disclosed no-op: no ordering of a 0-or-1-element list is observable, documented
  as such on `ListLineageGroupsParams` rather than silently doing nothing without saying so.
- `QueryLineageInput.{Filters,MaxResults,NextToken}` (`api_op_QueryLineage.go`) — absent
  entirely; `QueryLineage` returned every reachable vertex/edge with no filtering or pagination.
  `MaxResults`/`NextToken` now real (vertices paginated via `paginateSlice`; real docs describe
  both as bounding "the number of vertices", not edges — `api_op_QueryLineage.go:34,38` — so
  `Edges` is the full, unpaginated edge set between surviving vertices, not further paginated).
  `Filters` (`types.QueryFilters`, `types/types.go:19078-19108`) is mostly real:
  `LineageTypes`/`Properties`/`CreatedAfter`/`CreatedBefore`/`ModifiedAfter`/`ModifiedBefore` all
  narrow the result set against the vertex's resolved Action/Artifact/Context detail (a vertex
  that isn't a tracked Action/Artifact/Context — e.g. a `TrainingJob`/`Model`/`Endpoint` ARN — is
  excluded whenever any of these five filters is set, since this backend has no truthful
  timestamp/properties to check it against). **Disclosed, not modeled:** `Filters.Types` (matches
  entities by their AWS resource type, e.g. `DataSet`/`Model`/`Endpoint`) is parsed but not
  enforced — this backend has no per-service entity-type resolver for arbitrary ARNs outside
  Action/Artifact/Context, and building one is out of this pass's scope (it would mean threading
  type resolution through every other service this backend's lineage graph can reference).

**Bugs found beyond the wire diff:** none of the storage-key-inconsistency shape (`SpaceName`/
`appKey`) this campaign has twice found before — this family adds no new identity field to any
primary key (Artifact keyed by ARN, Context/Action keyed by name, both unchanged by this pass;
`DeleteArtifact`'s `Source` is an alternate *lookup* path onto the existing ARN key, not a new key
component). The two bugs of a different, still-real shape are the `ListLineageGroups` window-filter
gap and the pre-existing `ListArtifactsInput` requiring `ArtifactArn` even when the real API
accepts `Source` alone (`DeleteArtifact`) — both are "the field was accepted or partially modeled,
but the business rule around it was wrong or absent," the class of bug this campaign was
calibrated to expect but that a wire-field diff alone reliably surfaces once the fields exist to
diff.

**Tests:** every fix has a real-`aws-sdk-go-v2`-client round-trip test (`newTestSageMakerClient`,
not a raw-JSON-body `doSageMakerRequest` call) asserting on the actual behavior — narrowed/
reordered/paginated result sets, a `DescribeArtifact`/`DescribeAction` response actually carrying
`MetadataProperties`, a `DeleteArtifact` that actually deletes when only `Source` is given.
Verified against unfixed code by hand-reverting eight representative fixes one at a time
(`CreateArtifact`/`CreateAction` MetadataProperties, `DeleteArtifact` Source fallback, `ListArtifacts`
CreatedAfter/CreatedBefore/MaxResults/SortOrder, `QueryLineage` Filters, `QueryLineage` MaxResults/
NextToken, `ListLineageGroups` CreatedAfter/CreatedBefore, `ListContexts`/`ListActions`
SortBy=Name, `ListContexts`/`ListActions` CreatedAfter/CreatedBefore) and confirming the
corresponding test fails with the predicted symptom, then restoring — files verified byte-
identical (`md5sum`) to their pre-revert state afterward.

**Not touched this pass:** the other 324 (362 − 19 parity-7 − 19 parity-8) inline structs
elsewhere in this service — `handler_hub.go` (15), `handler_pipelines.go` (14), `handler_mlflow.go`
(14), `handler_model_packages.go` (12), `handler_notebook_instances.go` (11), `handler_images.go`
(11), `handler_edge_deployment.go` (11), and the rest — gopherstack-oc9v remains open for those.

Gates for this session: `go build ./...`, `go vet -tags e2e ./...`, `go vet -tags integration
./...`, `gofmt -l ./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix
-diff ./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` all
clean; zero `nolint:{cyclop,gocyclo,gocognit,funlen}` added (two `nolint:dupl` added on
`ListContexts`/`ListActions`, matching this repo's 98 existing precedents for that specific
linter, disclosed since it isn't in the banned group).

## parity-9 (2026-08-21, gopherstack-oc9v): Hub / HubContent family inline-struct sweep

Third pass of the gopherstack-oc9v campaign. Per parity-8's own boundary note ("324 of
sagemaker's 362 inline structs now remain ... `handler_hub.go` (15), `handler_pipelines.go` (14),
`handler_mlflow.go` (14), `handler_model_packages.go` (12) ... "), this pass took the largest
remaining single file, verified by `grep -c 'var req struct {' handler_hub.go` = 15 before
starting. All 15 were converted to named types (`createHubInput`, `describeHubInput`,
`listHubsInput`, `updateHubInput`, `deleteHubInput`, `importHubContentInput`,
`describeHubContentInput`, `listHubContentsInput`, `listHubContentVersionsInput`,
`deleteHubContentInput`, `createHubContentReferenceInput`, `deleteHubContentReferenceInput`,
`createHubContentPresignedURLsInput`, `updateHubContentInput`,
`updateHubContentReferenceInput`) and wire-audited field-by-field against the pinned SDK
(confirmed `v1.263.2` from `go.mod`, matching parity-7/8's assumption). **309 of sagemaker's 362
inline structs now remain** (362 − 19 − 19 − 15); `handler_hub.go` itself now has zero. This pass
did not touch `handler_pipelines.go`/`handler_mlflow.go`/`handler_model_packages.go` or any other
family — all still open for gopherstack-oc9v.

**Enumerated vs. converted vs. audited:** of the 15 ops, 11 already matched the real SDK struct
exactly once named (`CreateHub`, `DescribeHub`, `UpdateHub`, `DeleteHub`, `ImportHubContent`,
`DescribeHubContent`, `DeleteHubContent`, `CreateHubContentReference`,
`DeleteHubContentReference`, `UpdateHubContent`, `UpdateHubContentReference`). The other 4 had
absent members:

- `ListHubsInput` (`api_op_ListHubs.go:29-58`) — was missing **seven** of its nine fields:
  `CreationTimeAfter`, `CreationTimeBefore`, `LastModifiedTimeAfter`, `LastModifiedTimeBefore`,
  `MaxResults`, `SortBy`, `SortOrder` (only `NameContains`/`NextToken` existed). The exact "parsed
  field, silently dropped" class this campaign exists to find, at the largest count yet found in
  one op. All seven now real: the four timestamp windows filter on `CreationTime`/
  `LastModifiedTime`; `MaxResults` caps the page via `paginateSlice`; `SortBy` orders by
  `HubName`/`CreationTime`/`HubStatus` (real `HubSortBy` enum, `types/enums.go:3929-3944` — a
  fourth value, `AccountIdOwner`, has no distinguishing order in this single-account-per-region
  backend and is disclosed as a no-op tiebreak, the same shape as parity-8's
  `ListLineageGroups.SortBy`); `SortOrder` reorders ascending/descending. No default is documented
  by AWS for either field (checked docs.aws.amazon.com/sagemaker/latest/APIReference/API_ListHubs.html
  directly — neither the SDK struct comments nor the HTML docs state one), so the pre-existing
  unconditional HubName-ascending behavior was kept as the disclosed fallback rather than guessed.
- `ListHubContentsInput` (`api_op_ListHubContents.go:29-58`) — missing `CreationTimeAfter`,
  `CreationTimeBefore`, `MaxResults`, `MaxSchemaVersion`, `SortBy`, `SortOrder` (six of ten real
  fields). All six now real, same shape as above; `MaxSchemaVersion` is a new filter class this
  campaign hadn't hit yet — a `"\d{1,4}.\d{1,4}.\d{1,4}"` dotted-version upper bound compared via a
  new `compareDottedVersions` helper (`hub.go`), not a timestamp or a plain string.
- `ListHubContentVersionsInput` (`api_op_ListHubContentVersions.go:29-62`) — missing
  `CreationTimeAfter`, `CreationTimeBefore`, `MaxResults`, `MaxSchemaVersion`, `MinVersion`,
  `SortBy`, `SortOrder` (seven of eleven real fields) — `MinVersion` was a real lower-bound version
  filter previously entirely unimplementable since the field didn't exist on the wire at all.
- `CreateHubContentPresignedUrlsInput` (`api_op_CreateHubContentPresignedUrls.go:29-58`) —
  missing `AccessConfig`, `MaxResults`, `NextToken` (three of seven real fields). `MaxResults`
  (real documented default 100, confirmed via
  docs.aws.amazon.com/sagemaker/latest/APIReference/API_CreateHubContentPresignedUrls.html) and
  `NextToken` now paginate the real URL list via `paginateSlice`. `AccessConfig`
  (`types.PresignedUrlAccessConfig{AcceptEula,ExpectedS3Url}`, `types/types.go:17716-17729`) is
  modeled and round-tripped as `PresignedURLAccessConfig` but **disclosed, not enforced**: this
  backend has no concept of "gated" hub content requiring EULA acceptance to reject against, and no
  independently-resolved S3 URL to validate `ExpectedS3Url`'s consistency claim against — the same
  disclosed-no-op shape as parity-8's `ListLineageGroups.SortBy`/`SortOrder`, not a fabricated
  business rule.

**Bugs found beyond the wire diff:** none of the storage-key-inconsistency shape this campaign has
twice found before (`SpaceName`/`appKey` in parity-7). Every op in this family is keyed by
`HubName`/`HubContentType`/`HubContentName`/`HubContentVersion`, none of which changed shape this
pass — checked explicitly, no new field participates in a primary key here. One accept-and-drop
bug of the class this campaign is calibrated to expect, beyond the raw absent-field count: before
this pass, `CreateHubContentPresignedUrls` always returned every generated URL unconditionally
regardless of `MaxResults`/`NextToken` (both silently absent from the wire struct), so a client
capping the page size would have unknowingly received the full unpaginated set. In practice this
is observable only when `HubContentDependencies` is non-empty (2+ URLs) — and this backend (like
the real `ImportHubContent`/`CreateHubContentReference` request shapes it mirrors) has **no request
field that ever populates `HubContentDependencies`**, so every reachable call produces at most one
URL and the truncation path, while now implemented for real, is not exercisable through any public
request shape. Disclosed in `TestHandler_CreateHubContentPresignedUrls_AccessConfigAndPaging`'s doc
comment rather than fabricating a dependency-populating input this pass didn't add.

**Tests:** every fix has a real-`aws-sdk-go-v2`-client round-trip test (`newTestSageMakerClient`)
asserting on actual behavior — narrowed/reordered/paginated result sets, not just that the request
parsed. Verified against unfixed code by hand-reverting three representative fixes one at a time
(`ListHubs`' full `CreationTimeAfter/Before/LastModifiedTimeAfter/Before/SortBy/SortOrder/
MaxResults` wiring, `ListHubContents`' `MaxSchemaVersion` wiring, `ListHubContentVersions`'
`MinVersion` wiring) and confirming the corresponding tests failed with the predicted symptom
(wrong count, wrong order, or wrong membership), then restoring — `handler_hub.go`/`hub.go`
verified byte-identical (`md5sum`) to their pre-revert state afterward.

**Not touched this pass:** the other 309 (362 − 19 − 19 − 15) inline structs elsewhere in this
service — `handler_pipelines.go` (14), `handler_mlflow.go` (14), `handler_model_packages.go` (12),
`handler_notebook_instances.go` (11), `handler_images.go` (11), `handler_edge_deployment.go` (11),
and the rest — gopherstack-oc9v remains open for those. `HubContent`'s response-side
`OriginalCreationTime` (present on the real `HubContentInfo` summary per
docs.aws.amazon.com/sagemaker/latest/APIReference/API_HubContentInfo.html, absent from this
backend's `hubContentInfoSummary`) is a response-field gap, not a request-struct one, so it is
outside this pass's inline-struct scope — disclosed here rather than silently left unmentioned.

Gates for this session: `go build ./...`, `go vet ./services/sagemaker/...`, `go vet -tags e2e
./...`, `go vet -tags integration ./...`, `gofmt -l ./services/sagemaker` (empty), `go test -race
./services/sagemaker/...`, `go fix -diff ./services/sagemaker/...` (no diff), and `golangci-lint
run ./services/sagemaker/...` all clean; zero `nolint` of any kind added (fixed the `gocognit`/
`goconst`/`golines`/`fieldalignment`/shadow findings golangci-lint raised mid-pass by decomposing
`ListHubContents` into a `hubContentMatchesListParams` helper, reusing the existing
`keyCreationTime` constant and a new `keyHubContentStatus` constant, reordering two structs'
fields, and renaming three shadowed test `err`s — rather than suppressing any of them).

## parity-10 (2026-08-21, gopherstack-oc9v): Pipeline / PipelineExecution family inline-struct sweep

Fourth pass of the gopherstack-oc9v campaign. Per parity-9's boundary note ("309 of sagemaker's 362
inline structs now remain ... `handler_pipelines.go` (14), `handler_mlflow.go` (14), `handler_
model_packages.go` (12) ..."), this pass took `handler_pipelines.go`, verified by `grep -c 'var req
struct {' handler_pipelines.go` = 14 before starting. All 14 were converted to named types
(`retryPipelineExecutionInput`, `stopPipelineExecutionInput`, `sendPipelineExecutionStepSuccess
Input`, `sendPipelineExecutionStepFailureInput`, `listPipelineExecutionStepsInput`, `createPipeline
Input`, `updatePipelineInput`, `startPipelineExecutionInput`, `listPipelineParametersForExecution
Input`, `describePipelineInput`, `listPipelinesInput`, `deletePipelineInput`, `describePipeline
ExecutionInput`, `listPipelineExecutionsInput`) and wire-audited field-by-field against the pinned
SDK (`v1.263.2`, confirmed from `go.mod`, matching prior passes' assumption). **295 of sagemaker's
362 inline structs now remain** (362 − 19 − 19 − 15 − 14); `handler_pipelines.go` itself now has
zero. This pass did not touch `handler_mlflow.go`/`handler_model_packages.go` or any other family —
all still open for gopherstack-oc9v.

**Enumerated vs. converted vs. audited:** of the 14 ops, 4 already matched the real SDK struct
exactly once named (`StopPipelineExecution`, `UpdatePipeline`, `DescribePipeline`, `DescribePipeline
Execution`, `DeletePipeline` — modulo `ClientRequestToken`, see below). The rest had absent members
or, in two ops, fields that do not exist on the real wire at all:

- `ListPipelinesInput` (`api_op_ListPipelines.go:29-58`) — was missing **six** of its seven
  optional fields: `CreatedAfter`, `CreatedBefore`, `MaxResults`, `PipelineNamePrefix`, `SortBy`,
  `SortOrder` (only `NextToken` existed). The exact "parsed field, silently dropped" class this
  campaign exists to find. All six now real: `CreatedAfter`/`CreatedBefore` filter on
  `CreationTime`; `PipelineNamePrefix` filters by prefix; `MaxResults` caps the page via
  `paginateSlice`; `SortBy` orders by `CreationTime` (documented default,
  docs.aws.amazon.com/sagemaker/latest/APIReference/API_ListPipelines.html: "The default is
  CreatedTime") or `Name`; `SortOrder` has no documented default (confirmed by fetching the same
  page — only `SortBy`'s default is stated), so ascending is kept as the disclosed fallback,
  matching `ListHubs`'/`ListLineageGroups`' precedent.
- `ListPipelineExecutionsInput` (`api_op_ListPipelineExecutions.go:29-62`) — missing `CreatedAfter`,
  `CreatedBefore`, `MaxResults`, `SortBy`, `SortOrder` (five of seven real fields, only
  `PipelineName`/`NextToken` existed). Same shape and same fix pattern as `ListPipelines`; `SortBy`
  orders by `CreationTime` (documented default) or `PipelineExecutionArn`.
- `ListPipelineExecutionStepsInput` (`api_op_ListPipelineExecutionSteps.go:29-43`) — missing
  `MaxResults` and `SortOrder` (the op has no `SortBy`, sorting always by `CreationTime`/`StartTime`
  per its documented default); previously hardcoded ascending-by-`StepName`.
- `ListPipelineParametersForExecutionInput` (`api_op_ListPipelineParametersForExecution.go:29-42`)
  — missing `MaxResults`; previously returned every parameter unconditionally.
- `RetryPipelineExecutionInput` (`api_op_RetryPipelineExecution.go:29-45`) — missing
  `ParallelismConfiguration` entirely. Real docs: "if specified, overrides the parallelism
  configuration of the parent pipeline" — implying the *default*, unspecified case still applies
  the parent pipeline's configuration. Before this fix, a retried execution carried no parallelism
  configuration at all, not even the one its own pipeline was created with; now
  `RetryPipelineExecution` defaults to the parent `Pipeline.ParallelismConfiguration` via the
  existing `findPipelineByARNLocked` helper (`pipeline_versions.go`) when the caller doesn't
  override it.
- `StartPipelineExecutionInput` (`api_op_StartPipelineExecution.go:29-63`) — missing
  `MlflowExperimentName`. Threaded through to `PipelineExecution.MlflowExperimentName` and returned
  as `DescribePipelineExecutionOutput.MLflowConfig.MlflowExperimentName` (`types.MLflowConfiguration`,
  `types/types.go:13862`). **Disclosed, not modeled:** `MLflowConfig.MlflowResourceArn` (the
  tracking-server ARN) is left absent — this backend has no notion of which MLflow tracking server
  (`handler_mlflow.go`, a separate op family untouched by this pass) an execution is attached to, so
  fabricating an ARN would be a guess, not a fact.
- `SendPipelineExecutionStepSuccessInput`/`SendPipelineExecutionStepFailureInput` (`api_op_
  SendPipelineExecutionStepSuccess.go:29-43`, `api_op_SendPipelineExecutionStepFailure.go:29-42) —
  the real wire shape is `CallbackToken` (+ `OutputParameters` for Success, `FailureReason` for
  Failure) and nothing else. **The previous handler read two fields, `PipelineExecutionArn` and
  `StepName`, that do not exist on either real input type at all** — no real `aws-sdk-go-v2` client
  can ever populate them, since AWS resolves the target step from the opaque `CallbackToken` alone.
  `OutputParameters` — entirely absent before this pass — is the real gap: before this fix, a
  callback step's output parameters were silently discarded. Fixed for real: `PipelineExecutionStep`
  now carries `OutputParameters` and a `CallbackToken` field, both returned via `ListPipelineExecution
  Steps`' new `Metadata.Callback` (`types.CallbackStepMetadata`, `types/types.go:3641` — `SqsQueueUrl`
  is not modeled, since this backend never notifies a real SQS queue). This backend has no
  pipeline-definition step graph to generate distinct per-step callback tokens the way real AWS
  does, so — disclosed rather than silently narrowed — it treats the caller-supplied `CallbackToken`
  as the target execution's ARN (matching the existing test suite's own usage before this pass) and
  can record at most one trackable callback step per execution, under a fixed step name.
- `ClientRequestToken`, required on six of these fourteen ops (`RetryPipelineExecution`,
  `StopPipelineExecution`, `CreatePipeline`, `DeletePipeline`, `StartPipelineExecution`, and — via
  `SendPipelineExecutionStepSuccess`/`Failure` — two more), is a pure client-side idempotency token
  with no server-observable effect and, per a repo-wide grep, is not modeled by any op in this
  service — omitted here too rather than introducing the service's first (inert) instance of it.

**Bugs found beyond the wire diff:** three, all beyond a raw field-presence count:

1. `ListPipelines`/`ListPipelineExecutions` silently dropping every filter/sort control except
   `NextToken` (and, for the latter, `PipelineName`) — the "parsed field, silently dropped" class,
   at the largest per-op count this campaign has found outside `ListHubs`.
2. `SendPipelineExecutionStepSuccess`/`Failure` reading two fields no real client can ever send
   (`PipelineExecutionArn`, `StepName`) while silently dropping the one real field
   (`OutputParameters`) that exists beyond the identifier — the inverse of every prior finding in
   this campaign, which were all "real field present in the model, absent from the wire." Converting
   surfaced a wire-shape *fabrication*, not just an omission.
3. `RetryPipelineExecution` producing a retried execution with no parallelism configuration at all,
   even when the parent pipeline had one — silently narrower than both the explicit-override and the
   implicit-inherit paths the real API documents.

**Storage-key check:** none of this family's ops changed a primary key's shape. `Pipeline` is keyed
by `PipelineName`, `PipelineExecution` by `PipelineExecutionArn`, `PipelineExecutionStep` by
`ExecutionArn|StepName` (`pipelineExecutionStepsKey`) — the fixed callback step name introduced by
this pass replaces a caller-controllable value with a constant, which *narrows* addressability
(disclosed above) but does not change the key's shape or introduce inconsistency with any other
computation of it (there is exactly one `keyFn`, in `store_domain.go`'s `pipelineExecStepsStore`,
consistent before and after).

**Response-side completeness fixed alongside:** `PipelineSummary` (`ListPipelines`) was returning
only 5 of 8 real response fields (missing `PipelineDescription`, `PipelineDisplayName`, `RoleArn`,
`LastExecutionTime` — all data the backend already stored or could derive) and `PipelineExecution
Summary` (`ListPipelineExecutions`) only 3 of 6 (missing `PipelineExecutionDisplayName`,
`PipelineExecutionDescription`, `PipelineExecutionFailureReason`). Both fixed via a new exported
`(*InMemoryBackend).PipelineLastExecutionTime` helper (reusing the `latestExecutionStartTime` logic
`DescribePipeline`'s `LastRunTime` already relied on) and straightforward field copies. These are
response-side, not request-struct, gaps, so strictly outside this pass's inline-struct scope —
fixed anyway since they were adjacent, free, and directly observable by any real client.

**Tests:** every fix has a real-`aws-sdk-go-v2`-client round-trip test (`newTestSageMakerClient`)
asserting on actual behavior — narrowed/reordered/paginated result sets, `MLflowConfig` actually
present on `DescribePipelineExecution`, a retried execution's `ParallelismConfiguration` actually
inheriting or overriding, `ListPipelineExecutionSteps`' `Metadata.Callback` actually round-tripping
`OutputParameters`. Verified against unfixed code by hand-reverting six representative fixes one at
a time (`ListPipelines`' `PipelineNamePrefix`, `ListPipelineExecutions`' `CreatedAfter`/
`CreatedBefore`, `RetryPipelineExecution`'s `ParallelismConfiguration` inheritance/override,
`SendPipelineExecutionStepSuccess`'s `OutputParameters`, `StartPipelineExecution`'s
`MlflowExperimentName`, `ListPipelineParametersForExecution`'s `MaxResults`) and confirming each
corresponding test failed with the predicted symptom, then restoring — `pipelines.go`/`pipeline_
executions.go`/`handler_pipelines.go` verified byte-identical (`md5sum`) to their pre-revert state
afterward.

**Disclosed, not tested:** `ListPipelineExecutionSteps`' `MaxResults` truncation branch is real,
working code that cannot be exercised through any public request shape on this backend — Success/
Failure both write under the same fixed callback-step key, so a single execution can have at most
one step record ever, and `ListPipelineExecutionSteps` filters to one execution at a time. Same
shape as parity-9's `CreateHubContentPresignedUrls` disclosure: the test proves `MaxResults=1`
returns the one step with no `NextToken`, and says why a second page can't exist, rather than
fabricating a multi-step scenario this backend cannot produce.

Gates for this session: `go build ./...`, `go vet ./services/sagemaker/...`, `go vet -tags e2e
./...`, `go vet -tags integration ./...`, `gofmt -l ./services/sagemaker` (empty), `go test -race
./services/sagemaker/...`, `go fix -diff ./services/sagemaker/...` (no diff), and `golangci-lint run
./services/sagemaker/...` all clean; zero `nolint` of any kind added (fixed `golines`,
two `govet shadow`s, `prealloc`, `staticcheck S1016`, `testifylint`, and two `fieldalignment`
findings golangci-lint raised mid-pass — the latter two via `fieldalignment -fix` reordering
`PipelineExecutionStep`/`PipelineExecution`'s fields — rather than suppressing any of them).

Next by size, per parity-9's own list: `handler_mlflow.go` (14), `handler_model_packages.go` (12).

## parity-11 (2026-08-21, gopherstack-oc9v): MlflowTrackingServer / MlflowApp family inline-struct sweep

Fifth pass of the gopherstack-oc9v campaign. Per parity-10's boundary note ("295 of sagemaker's
362 inline structs now remain ... `handler_mlflow.go` (14), `handler_model_packages.go` (12) ..."),
this pass took `handler_mlflow.go`, verified by `grep -c 'var req struct {' handler_mlflow.go` = 14
before starting. All 14 were converted to named types (`createMlflowTrackingServerInput`,
`describeMlflowTrackingServerInput`, `deleteMlflowTrackingServerInput`,
`startMlflowTrackingServerInput`, `stopMlflowTrackingServerInput`,
`createPresignedMlflowTrackingServerURLInput`, `createMlflowAppInput`, `describeMlflowAppInput`,
`deleteMlflowAppInput`, `updateMlflowAppInput`, `listMlflowAppsInput`,
`createPresignedMlflowAppURLInput`, `listMlflowTrackingServersInput`,
`updateMlflowTrackingServerInput`) and wire-audited field-by-field against the pinned SDK
(`v1.263.2`, confirmed from `go.mod`, matching prior passes). **281 of sagemaker's 362 inline
structs now remain** (362 − 19 − 19 − 15 − 14 − 14, confirmed by `grep -rc 'var req struct {'
services/sagemaker/*.go` summed, not arithmetic); `handler_mlflow.go` itself now has zero. This
pass did not touch `handler_model_packages.go` or any other family — all still open for
gopherstack-oc9v.

**Enumerated vs. converted vs. audited:** of the 14 ops, only `DescribeMlflowApp` already matched
its real SDK input struct exactly (its *output* has a real gap, disclosed below). Every other op
had absent members — this family has the highest per-op gap density this campaign has found:

- `CreateMlflowTrackingServerInput` (`api_op_CreateMlflowTrackingServer.go:31-96`) — was missing
  **six** of its nine fields, including a *required* one: `ArtifactStoreUri` (required — the
  backend had never stored an artifact store URI for a tracking server at all, unlike
  `MlflowApp` which already had one), `AutomaticModelRegistration` (default `false`, documented),
  `S3BucketOwnerAccountId`, `S3BucketOwnerVerification` (default `true`, documented),
  `TrackingServerSize` (default `"Small"`, documented), `WeeklyMaintenanceWindowStart`. Only
  `TrackingServerName`/`RoleArn`/`MlflowVersion`/`Tags` existed. All six now real and threaded onto
  a new `MlflowTrackingServer.ArtifactStoreURI`/`AutomaticModelRegistration`/
  `S3BucketOwnerAccountID`/`S3BucketOwnerVerification`/`TrackingServerSize`/
  `WeeklyMaintenanceWindowStart`.
- `DescribeMlflowTrackingServerOutput` (`api_op_DescribeMlflowTrackingServer.go:39-106`) — was
  returning only 8 of 17 real response fields (the handler previously marshaled the persisted
  struct directly via its own `MarshalJSON`). Now built from a dedicated
  `describeMlflowTrackingServerResponse` (matching the `describeMlflowAppResponse` precedent from
  parity-8/9): adds every `CreateMlflowTrackingServerInput` field above (all describable) plus two
  derived fields with no client-supplied value to thread: `IsActive` (`types.IsTrackingServerActive`,
  `types/enums.go:4962-4975` — derived `Active` iff `TrackingServerStatus == "Running"`, i.e. the
  same state Start/Stop already drive, not a guess) and `TrackingServerUrl` (built the same
  real-shaped-but-unsigned way as the existing `CreatePresignedMlflowTrackingServerURL`, factored
  into a shared `mlflowTrackingServerURL` helper). `CreatedBy`/`LastModifiedBy`
  (`types.UserContext`) and `TrackingServerMaintenanceStatus` are disclosed absent: grepped
  repo-wide, no op in this service models a caller-identity concept, and no maintenance subsystem
  exists to report a real status from.
- `DeleteMlflowTrackingServerOutput`/`StartMlflowTrackingServerOutput`/
  `StopMlflowTrackingServerOutput` (`api_op_{Delete,Start,Stop}MlflowTrackingServer.go`) — all
  three previously returned **no body at all** (`dispatchMlflowTrackingServerOps` called them for
  their side effect only, discarding the real `TrackingServerArn` every one of these ops returns
  on success). All three now return `{"TrackingServerArn": "..."}`, requiring the three backend
  methods to return `(string, error)` instead of bare `error`.
- `DeleteMlflowAppOutput` (`api_op_DeleteMlflowApp.go:37-43`) — same class: previously no body,
  real output is `Arn`. Fixed without a backend signature change, since the request already
  carries the app's `Arn` as its sole identifier — the handler echoes `req.Arn` back after a
  successful delete.
- `CreateMlflowAppInput`/`UpdateMlflowAppInput`/`DescribeMlflowAppOutput`
  (`api_op_{Create,Update,Describe}MlflowApp.go`) — all three were missing
  `WeeklyMaintenanceWindowStart`, now threaded onto a new `MlflowApp.WeeklyMaintenanceWindowStart`.
  `UpdateMlflowAppInput.Name` is decoded but deliberately not threaded — see the bug-class writeup
  below.
- `ListMlflowAppsInput` (`api_op_ListMlflowApps.go:30-73`) — was missing **nine of ten** fields:
  `AccountDefaultStatus`, `CreatedAfter`, `CreatedBefore`, `DefaultForDomainId`, `MaxResults`,
  `MlflowVersion`, `SortBy`, `SortOrder`, `Status` (only `NextToken` existed) — the largest
  absent-field count this campaign has found in a single op, surpassing `ListHubs`' seven. All nine
  now real via a new `ListMlflowAppsParams`/`mlflowAppMatchesListParams`/`mlflowAppSortLess`, with
  the default sort resolved from the op's own doc comment rather than guessed: "By default, MLflow
  Apps are listed in Descending order by creation time" (`api_op_ListMlflowApps.go:22-24`) — unlike
  `ListHubs`/`ListPipelines`, where no default was documented and ascending was kept as a disclosed
  fallback, this op states both the default sort key *and* the default order explicitly, so both are
  implemented as documented. `MlflowVersion` (both the input filter and the newly-modeled
  `Summaries[].MlflowVersion`) is real-shaped but disclosed unreachable: neither
  `CreateMlflowAppInput` nor `UpdateMlflowAppInput` carries an `MlflowVersion` field at all (checked
  both `api_op_` files directly), so no real client can ever populate a non-empty value for this
  backend to filter or sort on — the same "implemented for real, unreachable through any public
  request shape" disclosure as parity-9's `CreateHubContentPresignedUrls` truncation path and
  parity-10's `ListPipelineExecutionSteps` pagination.
- `ListMlflowTrackingServersInput` (`api_op_ListMlflowTrackingServers.go:30-72`) — missing seven of
  eight fields: `CreatedAfter`, `CreatedBefore`, `MaxResults`, `MlflowVersion`, `SortBy`, `SortOrder`,
  `TrackingServerStatus` (only `NextToken` existed). Same fix pattern and same documented
  Descending-by-CreationTime default as `ListMlflowApps` above, via a new
  `ListMlflowTrackingServersParams`/`mlflowTrackingServerMatchesListParams`/
  `mlflowTrackingServerSortLess`. `Summaries[].IsActive` (`types.TrackingServerSummary`,
  `types/types.go:22152-22176`) — absent before this pass — now populated from the same
  `mlflowTrackingServerIsActive` derivation as `DescribeMlflowTrackingServer`.
- `UpdateMlflowTrackingServerInput` (`api_op_UpdateMlflowTrackingServer.go:28-63`) — was missing all
  six of its optional fields beyond the required `TrackingServerName`: `ArtifactStoreUri`,
  `AutomaticModelRegistration`, `S3BucketOwnerAccountId`, `S3BucketOwnerVerification`,
  `TrackingServerSize`, `WeeklyMaintenanceWindowStart` (the handler previously threaded only
  `MlflowVersion`). All six now real via a new `UpdateMlflowTrackingServerOptions`, with the two
  booleans as `*bool` (precedented elsewhere in this service — `ai_recommendation_jobs.go`,
  `automl_v2.go`, `feature_groups.go`, `images.go` — for exactly this reason) so the handler can
  tell "omitted" from "explicitly false" apart. See the doc-vs-behavior judgment call below.
- `CreatePresignedMlflowAppUrlInput`/`CreatePresignedMlflowTrackingServerUrlInput`
  (`api_op_CreatePresignedMlflow{App,TrackingServer}Url.go`) — both missing `ExpiresInSeconds` and
  `SessionExpirationDurationInSeconds`. Modeled on the named request types for wire visibility but
  disclosed no-op: this backend generates presigned URLs with no TTL/session-expiry enforcement
  mechanism anywhere in the service (grepped repo-wide) — the same structural gap as `hub.go`'s
  `PresignedUrlAccessConfig`.

**A doc-vs-behavior judgment call, disclosed rather than silently resolved either way:**
`UpdateMlflowTrackingServerInput.AutomaticModelRegistration`'s AWS doc page
(`API_UpdateMlflowTrackingServer.html`, fetched directly) restates
`CreateMlflowTrackingServerInput`'s "if not specified, defaults to False" language verbatim for
this *update* op. Read literally, every update call omitting the field would silently reset it to
`false` — destructive PATCH-as-PUT semantics inconsistent with every one of this same op's other
five optional fields (all leave-unchanged-if-omitted) and with every other Update op in this
service (none resets a value to a constant on omission). Treated as leave-unchanged, matching the
surrounding fields and the rest of the service, with the doc's literal text recorded in a comment
on `UpdateMlflowTrackingServerOptions` rather than silently picked either way. A real-client test
(`TestHandler_UpdateMlflowTrackingServer_FullFields_RealClient`) proves an update that sets other
fields but omits `AutomaticModelRegistration`/`S3BucketOwnerVerification` leaves their prior
explicit values in place.

**A second judgment call:** `UpdateMlflowAppInput.Name` — "The name of the MLflow App to update" —
gets no further description on AWS's own doc page either (fetched directly; identical text to the
SDK comment). This backend's `MlflowApp` is stored by an `Arn` built from `Name` at creation
(`mlflow-app/<name>`), so treating `Name` as a rename would require rekeying the store, and neither
AWS's nor this repo's docs establish that a rename is what this field does. Decoded but not
threaded, disclosed on `UpdateMlflowAppOptions`' doc comment; proved with
`TestHandler_UpdateMlflowApp_NameIsNoOp_RealClient`, which sends a different `Name` on update and
confirms `DescribeMlflowApp` still reports the original.

**Bugs found beyond the wire diff:**

1. `CreateMlflowTrackingServer` never stored an artifact store URI at all — `ArtifactStoreUri` is a
   *required* real input field with no counterpart anywhere in the old `MlflowTrackingServer`
   struct, unlike `MlflowApp`, which already had one. Every tracking server created by this backend
   was answering `DescribeMlflowTrackingServer` with no artifact store information whatsoever.
2. `DeleteMlflowTrackingServer`/`StartMlflowTrackingServer`/`StopMlflowTrackingServer`/
   `DeleteMlflowApp` all discarded their real response body entirely (`ok, err := ...; return nil,
   true, err`-style dispatch) — a real field silently dropped at the widest possible scope, the
   entire response.
3. `ListMlflowApps`/`ListMlflowTrackingServers` accepting only `NextToken` while silently dropping
   every filter and sort control — the "parsed field, silently dropped" class this campaign exists
   to find, at the largest single-op count yet (`ListMlflowApps`, nine of ten fields).

No storage-key inconsistency of the `SpaceName`/`appKey` kind: `MlflowTrackingServer` is keyed by
`TrackingServerName`, `MlflowApp` by `Arn` (already built from `Name` at creation, unchanged by this
pass) — checked explicitly, and the one field that could plausibly touch a key (`UpdateMlflowApp`'s
`Name`) is exactly the field disclosed as not threaded, above.

**Tests:** every fix has a real-`aws-sdk-go-v2`-client round-trip test (`newTestSageMakerClient`) —
`TestHandler_CreateMlflowTrackingServer_FullFields_RealClient`,
`TestHandler_CreateMlflowTrackingServer_Defaults_RealClient`,
`TestHandler_UpdateMlflowTrackingServer_FullFields_RealClient`,
`TestHandler_ListMlflowTrackingServers_FilterSortPage_RealClient`,
`TestHandler_StartStopMlflowTrackingServer_IsActive_RealClient`,
`TestHandler_MlflowApp_WeeklyMaintenanceWindowStart_RealClient`,
`TestHandler_UpdateMlflowApp_NameIsNoOp_RealClient`,
`TestHandler_ListMlflowApps_FilterSortPage_RealClient` — plus updated assertions on the
pre-existing `TestHandler_StartStopMlflowTrackingServer`/`TestHandler_DeleteMlflowTrackingServer`/
`TestHandler_MlflowApp_Lifecycle` proving the newly-real response bodies. Verified against unfixed
code by hand-reverting three representative fixes one at a time — `CreateMlflowTrackingServer`'s
new-field threading (reverted to the old four-argument call), `ListMlflowApps`' full filter/sort
wiring (reverted to `NextToken`-only), and `mlflowTrackingServerIsActive` (reverted to
unconditional `"Inactive"`) — confirming the corresponding tests failed with the predicted symptom
(fields empty, wrong sort/filter/page membership, `IsActive` stuck at `Inactive`), then restoring —
`mlflow.go`/`handler_mlflow.go`/`handler_mlops.go` verified byte-identical (`md5sum`) to their
pre-revert state afterward.

Gates for this session: `go build ./...`, `go vet ./services/sagemaker/...`, `go vet -tags e2e
./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`, `gofmt -l
./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` all clean;
zero `nolint` of any kind added (fixed the `goconst` finding on a third `"Active"` literal — which
also caught one pre-existing occurrence in `pipelines.go` sharing the same constant — and two
`fieldalignment`/one `staticcheck S1016` finding golangci-lint raised mid-pass via
`golangci-lint run --fix`, rather than suppressing any of them). Repo-wide `go vet -tags e2e ./...`
fails in `services/apprunner_test` on an unrelated, untracked, in-progress file
(`wire_output_required_r80d_test.go`) from a concurrent agent sweeping a different service — outside
this pass's `services/sagemaker/` scope; the sagemaker-scoped and repo-wide `integration`-tagged vet
runs are both clean, as is `go build ./...`.

**281 of sagemaker's 362 inline structs now remain.** Next by size, per this pass's own count:
`handler_model_packages.go` (12), `handler_notebook_instances.go` (11), `handler_images.go` (11),
`handler_edge_deployment.go` (11).

## parity-12 (2026-08-21, gopherstack-oc9v): ModelPackage / ModelPackageGroup family inline-struct sweep

Sixth pass of the gopherstack-oc9v campaign. Also fixed in passing: the `model_endpoint_config_crud`
manifest entry (line 104) was missing its closing `}` — a brace-counting parser regressed on it
(the old gendocs parser never checked brace balance). Fixed in place, kept inline per repo
convention (`cmd/gendocs` now accepts block style too, but inline remains the convention here) —
no other change to that entry.

Per parity-11's boundary note ("281 of sagemaker's 362 inline structs now remain ...
`handler_model_packages.go` (12), `handler_notebook_instances.go` (11), `handler_images.go` (11),
`handler_edge_deployment.go` (11)"), this pass took `handler_model_packages.go`, verified by
`grep -c 'var req struct {' handler_model_packages.go` = 12 before starting. All 12 were converted
to named types (`createModelPackageInput`, `describeModelPackageInput`, `deleteModelPackageInput`,
`listModelPackagesInput`, `createModelPackageGroupInput`, `describeModelPackageGroupInput`,
`deleteModelPackageGroupInput`, `listModelPackageGroupsInput`, `getModelPackageGroupPolicyInput`,
`putModelPackageGroupPolicyInput`, `deleteModelPackageGroupPolicyInput`, `updateModelPackageInput`)
and wire-audited field-by-field against the pinned SDK (`v1.263.2`, confirmed from `go.mod`,
matching prior passes). **269 of sagemaker's 362 inline structs now remain** (362 − 19 − 19 − 15 −
14 − 14 − 12), confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go` summed, not
arithmetic; `handler_model_packages.go` itself now has zero. `BatchDescribeModelPackage` was already
a named type (`batchDescribeModelPackageRequest`) before this pass and so was not one of the 12 —
untouched. This pass did not touch `handler_notebook_instances.go`/`handler_images.go`/
`handler_edge_deployment.go` or any other family — all still open for gopherstack-oc9v.

**Enumerated vs. converted vs. audited:** every one of the 12 had absent members, several by a
wide margin — this family's `CreateModelPackageInput` is the largest single-op gap this campaign
has found, surpassing `ListMlflowApps`' nine of ten:

- `CreateModelPackageInput` (`api_op_CreateModelPackage.go:47-171`) — was missing **19 of 20**
  optional fields, keeping only `ModelPackageName`/`ModelPackageGroupName`/
  `ModelPackageDescription`/`Tags`. Simple scalars/enums now real and threaded onto new
  `ModelPackage` fields: `ModelApprovalStatus` (previously settable only via a later
  `UpdateModelPackage` call — no real client could set initial approval status at creation time),
  `Domain`, `Task`, `SamplePayloadUrl`, `SourceUri`, `ManagedStorageType`,
  `ModelPackageRegistrationType`, `SkipModelValidation`, `CertifyForMarketplace`,
  `CustomerMetadataProperties`. Deeply-nested union/config shapes —
  `InferenceSpecification`/`SourceAlgorithmSpecification`/`ValidationSpecification`/
  `DriftCheckBaselines`/`ModelMetrics`/`ModelCard`/`ModelLifeCycle`/`MetadataProperties`/
  `SecurityConfig`/`AdditionalInferenceSpecifications` — are carried as opaque `json.RawMessage`
  passthrough, the same established convention as `ai_workload_configs.go`'s
  `WorkloadSpec`/`DatasetConfig`: every field a client actually sends round-trips exactly (proven
  with real `types.InferenceSpecification` values in
  `TestHandler_CreateModelPackage_FullFields_RealClient`), out of this pass's budget to fully type.
  `ClientToken` (idempotency token, no server-observable effect, same as every other Create op in
  this service) is deliberately omitted.
- `DescribeModelPackageInput` (`api_op_DescribeModelPackage.go:16-42`) — missing `IncludedData`
  (`AllData`/`MetadataOnly`, controls whether `ModelCard.ModelCardContent` is KMS-redacted). Now
  accepted for wire-shape fidelity but **disclosed as a no-op**: this backend has no KMS-gated
  redaction mechanism to apply either way, and `ModelCard` is already opaque passthrough with no
  field-level access control, so `MetadataOnly` cannot honestly sanitize it further than `AllData`
  already does.
- `ListModelPackagesInput` (`api_op_ListModelPackages.go:29-77`) — missing **7 of 9** optional
  fields (only `ModelPackageGroupName`/`NextToken` existed): `CreationTimeAfter`,
  `CreationTimeBefore`, `MaxResults`, `ModelApprovalStatus`, `ModelPackageType`, `NameContains`,
  `SortBy`, `SortOrder`. All seven now real via a new `ListModelPackagesParams`/
  `modelPackageMatchesListParams`/`modelPackageMatchesGroupAndType`, with the documented default
  sort (`CreationTime`, `Ascending` — `api_op_ListModelPackages.go:71,74`) implemented as stated.
  **`ModelPackageType`'s three enum values are `"Versioned"`/`"Unversioned"`/`"Both"`
  (`types/enums.go:6115-6122`), not the all-caps `UNVERSIONED`/`VERSIONED`/`BOTH` the op's own doc
  comment uses** — caught by checking the enum source directly rather than trusting the prose (the
  campaign's own recurring lesson), and confirmed load-bearing by hand-revert (below): the
  all-caps version silently breaks the `Versioned`/`Both` branches while the default branch
  happens to still match. This backend interprets "versioned" as "has a `ModelPackageGroupName`"
  — the only sense in which it distinguishes versioned from unversioned models, since it does not
  implement AWS's group+version ARN addressing scheme (see `ModelPackageVersion` disclosure below).
- `CreateModelPackageGroupInput` (`api_op_CreateModelPackageGroup.go:16-40`) — missing
  `ManagedConfiguration` (a new `*ManagedConfiguration` type, `types/types.go:13591` — a
  single-field wrapper around `ManagedStorageType`); now threaded through Create and returned by
  Describe/List.
- `DescribeModelPackageGroupOutput` — `CreatedBy` (`types.UserContext`) is `"This member is
  required"` but **disclosed absent, not fabricated** — no IAM-identity model exists in this
  service to honestly derive it from, the same class-d gap as every other `CreatedBy`/
  `LastModifiedBy` field already disclosed for Pipeline/MlflowApp/TrialComponent.
- `ListModelPackageGroupsInput` (`api_op_ListModelPackageGroups.go:29-72`) — missing **6 of 7**
  optional fields (only `NextToken` existed): `CreationTimeAfter`, `CreationTimeBefore`,
  `CrossAccountFilterOption`, `MaxResults`, `NameContains`, `SortBy`, `SortOrder`. All six now real
  via a new `ListModelPackageGroupsParams`/pagination path mirroring `ListPipelines`'
  `paginateSlice` convention (previously this op used a distinct key-based-token style; switched to
  match the rest of this campaign's List ops, harmless since `NextToken` is opaque to any real
  client). `CrossAccountFilterOption` is honored but **disclosed as a real, not fabricated,
  no-op**: this backend has no cross-account resource-sharing model at all (grepped repo-wide), so
  a `CrossAccount` request correctly returns empty — the true answer for an account with zero
  shared groups, not a guess.
- `UpdateModelPackageInput` (`api_op_UpdateModelPackage.go:26-95`) — **the standout finding of this
  pass.** The real op's sole identifier is `ModelPackageArn` ("This member is required") —
  `UpdateModelPackageInput` has **no `ModelPackageName` field at all**. The previous handler decoded
  `"ModelPackageName"` from the request body instead, a field no genuine `aws-sdk-go-v2` client
  could ever populate for this op, since the real client always serializes `ModelPackageArn`. Every
  real `UpdateModelPackage` call against this backend therefore failed outright with
  `"ModelPackageName is required"` — confirmed by the pre-existing `TestHandler_UpdateModelPackage`
  test, which had enshrined the bug by sending `ModelPackageName` itself (fixed alongside; a new
  `TestHandler_UpdateModelPackage_RealClient` proves the real-client path now works, and hand-revert
  reproduces the exact original failure, below). The same wire-shape-fabrication class parity-10
  found on `SendPipelineExecutionStepSuccess`/`Failure`'s `PipelineExecutionArn`/`StepName`. Beyond
  the identifier, **6 of 7** optional fields were also missing (only `ModelApprovalStatus` existed):
  `ApprovalDescription`, `ModelPackageRegistrationType`, `SourceUri`, `CustomerMetadataProperties`
  (merged, not replaced, matching every other Update op in this service),
  `CustomerMetadataPropertiesToRemove` (deletes the named keys), `InferenceSpecification`,
  `ModelCard`, `ModelLifeCycle` (all three replace the corresponding opaque field wholesale, since
  the real op's doc doesn't describe a merge for these), and
  `AdditionalInferenceSpecificationsToAdd` — implemented as a real JSON-array append onto the
  existing `AdditionalInferenceSpecifications`, per the op's own doc: "to be added to the existing
  array" (not a replace), proven with
  `TestHandler_UpdateModelPackage_AdditionalInferenceSpecificationsToAdd_RealClient` asserting both
  the pre-existing and newly-added entries are present, in order, after the call.

**Bugs found beyond the wire diff:** four, all beyond a raw field-presence count:

1. `UpdateModelPackage` reading `ModelPackageName`, a field that does not exist on the real
   `UpdateModelPackageInput` at all — every real client's `UpdateModelPackage` call failed
   outright. The pre-existing test suite had encoded this bug as expected behavior.
2. `ListModelPackages`/`ListModelPackageGroups` accepting only `ModelPackageGroupName`/`NextToken`
   and `NextToken` respectively while silently dropping every other filter/sort control — the
   "parsed field, silently dropped" class this campaign exists to find, with
   `CreateModelPackageInput`'s 19-of-20 gap the largest single-op count yet.
3. `CreateModelPackage` had no way to set `ModelApprovalStatus` at creation time at all — a real,
   optional `CreateModelPackageInput` field — forcing every real client to make an immediate
   follow-up `UpdateModelPackage` call just to set the approval status a single `Create` call could
   have set.
4. The `ModelPackageType` enum-casing bug described above — caught during this pass (not shipped),
   but load-bearing enough to hand-revert and confirm below, since it is exactly the kind of "doc
   prose vs. SDK source" mismatch this campaign's method (grep the enum source, not the comment) is
   meant to catch before it ships, not after.

**Storage-key check:** neither `ModelPackage` (keyed by `ModelPackageArn`, `store.go:417`) nor
`ModelPackageGroup` (keyed by `ModelPackageGroupName`, `store.go:440`) changed key shape this pass.
`UpdateModelPackage`'s identifier changed from a decoded `ModelPackageName` to a decoded
`ModelPackageArn`, but the backend method itself already accepted an ARN directly via
`modelPackagesStore(region).Get(arnStr)` — only the handler's request-decoding field was wrong, not
the backend's lookup path, so no key-shape change was needed to fix it.

**Disclosed, not modeled** (structural, not accept-and-drop):

- `ModelPackageVersion` (`ModelPackageSummary`/`DescribeModelPackageOutput`) — this backend does not
  implement AWS's group+version ARN addressing scheme (a versioned model package is identified by
  `model-package/<group-name>/<version>`, not by its own name); implementing that would mean
  rekeying `ModelPackage`'s entire identity model, well beyond a wire-field-audit pass. Disclosed
  rather than fabricating a version counter that wouldn't match a real client's ARN expectations.
- `CreatedBy`/`LastModifiedBy` (`types.UserContext`, both `ModelPackage` and `ModelPackageGroup`) —
  no IAM-identity model exists in this service (class-d, same as every other family).
- `DescribeModelPackageInput.IncludedData` — accepted but a real no-op, see above.

**Tests:** every fix has a real-`aws-sdk-go-v2`-client round-trip test (`newTestSageMakerClient`) —
`TestHandler_UpdateModelPackage_RealClient`, `TestHandler_CreateModelPackage_FullFields_RealClient`,
`TestHandler_ListModelPackages_FilterSortPage_RealClient`,
`TestHandler_ListModelPackages_ApprovalStatusFilter_RealClient`,
`TestHandler_ListModelPackages_ModelPackageType_RealClient`,
`TestHandler_ListModelPackageGroups_FilterSortPage_RealClient`,
`TestHandler_CreateModelPackageGroup_ManagedConfiguration_RealClient`,
`TestHandler_UpdateModelPackage_FullFields_RealClient`,
`TestHandler_UpdateModelPackage_CustomerMetadataPropertiesToRemove_RealClient`,
`TestHandler_UpdateModelPackage_AdditionalInferenceSpecificationsToAdd_RealClient` — plus the
pre-existing `TestHandler_UpdateModelPackage`/`TestHandler_UpdateModelPackage_NotFound` updated to
send `ModelPackageArn` (the real wire field) instead of the bug they had previously enshrined, and a
new `TestHandler_UpdateModelPackage_MissingArn` covering the validation path. Verified against
unfixed code by hand-reverting four representative fixes one at a time —
`UpdateModelPackage`'s `ModelPackageArn`/`ModelPackageName` identifier (reverted to decoding
`ModelPackageName` again), `CreateModelPackageGroup`'s `ManagedConfiguration` threading (disabled),
`ListModelPackages`' `ModelPackageType` enum casing (reverted to all-caps), and
`UpdateModelPackage`'s `CustomerMetadataPropertiesToRemove` loop (disabled) — confirming each
corresponding test failed with the predicted symptom (`"ModelPackageName is required"` from a real
client sending `ModelPackageArn`; `ManagedConfiguration` nil; `Versioned`/`Both` filters silently
returning the wrong set while the default branch coincidentally still passed; a removed key still
present) — then restoring; `handler_model_packages.go`/`model_packages.go` verified byte-identical
(`md5sum`) to their pre-revert state afterward.

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` all clean;
zero `nolint` of any kind added (fixed a `cyclop` finding by extracting
`modelPackageMatchesGroupAndType` out of `modelPackageMatchesListParams`, a `goconst` finding by
adding `keyModelPackageGroupName`, a `gocritic appendAssign` and a `prealloc` finding in the new
`appendInferenceSpecifications` helper, a `golines` formatting issue, two `fieldalignment` findings
via `fieldalignment -fix`, two `revive var-naming` findings (`SamplePayloadUrl`/`SourceUri` →
`SamplePayloadURL`/`SourceURI`), a `revive` doc-comment mismatch, and three `govet shadow` findings
in the new tests — rather than suppressing any of them). `go build ./...` (repo-wide) was clean at
the time this pass ran; the `services/backup`/`services/databrew` breakage a concurrent agent's
in-progress edit was expected to cause was not observed, so nothing outside `services/sagemaker/`
needed to be worked around this session.

**269 of sagemaker's 362 inline structs now remain.** Next by size:
`handler_notebook_instances.go` (11), `handler_images.go` (11), `handler_edge_deployment.go` (11).

## parity-13 (2026-08-21, gopherstack-oc9v): NotebookInstance / NotebookInstanceLifecycleConfig family inline-struct sweep

Seventh pass of the gopherstack-oc9v campaign. Per parity-12's boundary note ("269 of sagemaker's
362 inline structs now remain ... `handler_notebook_instances.go` (11), `handler_images.go` (11),
`handler_edge_deployment.go` (11)"), this pass took `handler_notebook_instances.go`, verified by
`grep -c 'var req struct {' handler_notebook_instances.go` = 11 before starting. All 11 were
converted to named types (`describeNotebookInstanceLifecycleConfigInput`,
`updateNotebookInstanceLifecycleConfigInput`, `deleteNotebookInstanceLifecycleConfigInput`,
`listNotebookInstanceLifecycleConfigsInput`, `describeNotebookInstanceInput`,
`updateNotebookInstanceInput`, `listNotebookInstancesInput`, `deleteNotebookInstanceInput`,
`startNotebookInstanceInput`, `stopNotebookInstanceInput`,
`createPresignedNotebookInstanceURLInput`) and wire-audited field-by-field against the pinned SDK
(`v1.263.2`, confirmed from `go.mod`, matching prior passes). **258 of sagemaker's 362 inline
structs now remain** (362 − 19 − 19 − 15 − 14 − 14 − 12 − 11), confirmed by
`grep -rc 'var req struct {' services/sagemaker/*.go` summed, not arithmetic; `handler_notebook_
instances.go` itself now has zero. `createNotebookInstanceFullRequest` (Create) was already a named
type before this pass and so was not one of the 11, but was still wire-audited and fixed alongside
its Update/Describe/List siblings since the same `NotebookInstance` model and the same two
fabricated-wire-shape bugs (below) run through all of them. This pass did not touch
`handler_images.go`/`handler_edge_deployment.go` or any other family — all still open for
gopherstack-oc9v.

**Enumerated vs. converted vs. audited:**

- `DescribeNotebookInstanceLifecycleConfigInput`/`DeleteNotebookInstanceLifecycleConfigInput`
  (`api_op_{Describe,Delete}NotebookInstanceLifecycleConfig.go:23-31`) — already matched exactly
  (`NotebookInstanceLifecycleConfigName` is each op's sole field).
- `UpdateNotebookInstanceLifecycleConfigInput` (`api_op_UpdateNotebookInstanceLifecycleConfig.go:
  24-41`) — already matched exactly (`NotebookInstanceLifecycleConfigName`/`OnCreate`/`OnStart`).
- `ListNotebookInstanceLifecycleConfigsInput` (`api_op_ListNotebookInstanceLifecycleConfigs.go:
  30-72`) — was missing **8 of 9** fields, only `NextToken` existed: `CreationTimeAfter`,
  `CreationTimeBefore`, `LastModifiedTimeAfter`, `LastModifiedTimeBefore`, `MaxResults`,
  `NameContains`, `SortBy`, `SortOrder`. All eight now real via a new
  `ListNotebookInstanceLifecycleConfigsParams`/`matchesLifecycleConfigParams`/
  `lifecycleConfigSortLess`, with the documented default sort key (`CreationTime` —
  `api_op_ListNotebookInstanceLifecycleConfigs.go:62`) implemented as stated and `SortOrder`'s
  undocumented default kept as the disclosed Ascending fallback (this campaign's recurring
  ListHubs/ListPipelines precedent).
- `DescribeNotebookInstanceInput` (`api_op_DescribeNotebookInstance.go:34-42`) — already matched
  exactly, but `DescribeNotebookInstanceOutput` (`api_op_DescribeNotebookInstance.go:44-158`) was
  missing `InstanceMetadataServiceConfiguration`/`IpAddressType` (both real, now threaded) and
  `FailureReason`/`NetworkInterfaceId` (disclosed absent below).
- `UpdateNotebookInstanceInput` (`api_op_UpdateNotebookInstance.go:38-143`) — was missing
  `InstanceMetadataServiceConfiguration`, `IpAddressType`, `PlatformIdentifier`, `RootAccess` — all
  four now real via new `NotebookUpdateOptions` fields. `AcceleratorTypes`/
  `DisassociateAcceleratorTypes` are not modeled: both are marked "no longer supported. Elastic
  Inference (EI) is no longer available" directly in AWS's own doc comment
  (`api_op_UpdateNotebookInstance.go:45-49,72-76`) — a real client sending them is a documented
  no-op on real AWS too, not a gap this backend introduces.
- `ListNotebookInstancesInput` (`api_op_ListNotebookInstances.go:31-90`) — was missing **9 of 12**
  fields, only `NextToken`/`StatusEquals`/`NameContains` existed:
  `AdditionalCodeRepositoryEquals`, `CreationTimeAfter`, `CreationTimeBefore`,
  `DefaultCodeRepositoryContains`, `LastModifiedTimeAfter`, `LastModifiedTimeBefore`, `MaxResults`,
  `NotebookInstanceLifecycleConfigNameContains`, `SortBy`/`SortOrder`. All now real via a new
  `ListNotebookInstancesParams`/`matchesNotebookParams`/`notebookInstanceSortLess`, with the
  documented default sort key (`Name` — `api_op_ListNotebookInstances.go:80`) implemented as
  stated and `SortOrder`'s undocumented default kept as the disclosed Ascending fallback.
  `NotebookInstanceSummary` (`types/types.go:16173-16225`) was also missing
  `AdditionalCodeRepositories`/`DefaultCodeRepository`/`NotebookInstanceLifecycleConfigName`/`Url` —
  all four now populated.
- `DeleteNotebookInstanceInput`/`StartNotebookInstanceInput`/`StopNotebookInstanceInput`
  (`api_op_DeleteNotebookInstance.go:33-41, api_op_StartNotebookInstance.go:31-39, api_op_StopNotebookInstance.go:35-43`) — already matched exactly
  (`NotebookInstanceName` is each op's sole field).
- `CreatePresignedNotebookInstanceUrlInput` (`api_op_CreatePresignedNotebookInstanceUrl.go:49-60`)
  — missing `SessionExpirationDurationInSeconds`. Modeled for wire visibility but disclosed no-op:
  this backend's presigned URL (below) is a static string with no TTL/session-expiry enforcement
  mechanism, the same structural gap already disclosed for parity-11's
  `CreatePresignedMlflowAppUrl`/`CreatePresignedMlflowTrackingServerUrl` and `hub.go`'s
  `PresignedUrlAccessConfig`.

**Two fabricated-wire-shape bugs found beyond the missing-field diff — both from the campaign's
"what does the handler read that AWS never sends" question, and both pre-dating this pass:**

1. **`CreateNotebookInstanceInput`/`UpdateNotebookInstanceInput`'s `LifecycleConfigName` wire key is
   literally `"LifecycleConfigName"`, not `"NotebookInstanceLifecycleConfigName"`** — confirmed
   directly from the generated protocol code, not the Go struct field name alone: sagemaker uses
   the `awsjson11` (AWS JSON 1.1 RPC) protocol, and `serializers.go:41852` /`serializers.go:51314`
   both emit the object key `LifecycleConfigName` for these two ops, while
   `deserializers.go:117035` and `deserializers.go:84325`'s sibling
   `NotebookInstanceLifecycleConfigName` case confirm the *response* side really does use the
   longer name for the same concept — AWS's own request and response field names disagree here.
   `createNotebookInstanceFullRequest`/the old anonymous Update struct both decoded
   `NotebookInstanceLifecycleConfigName` from the request body, a key no real SDK client sending
   `LifecycleConfigName` ever populates: every genuine `CreateNotebookInstance`/
   `UpdateNotebookInstance` call that set a lifecycle config was silently ignored by this backend.
   Caught by a real-client round-trip test (`TestHandler_ListNotebookInstances_FilterSortPage_
   RealClient`) failing with the lifecycle-config-name field coming back empty despite being set on
   `Create`, then confirmed against `serializers.go` directly rather than assumed from the Go field
   name. Fixed by correcting both structs' `json` tags to `LifecycleConfigName`.
2. **`NotebookInstance.URL` was declared on the model and read by both `DescribeNotebookInstance`
   and `NotebookInstanceSummary`'s response builders, but never populated anywhere except the
   already-existing `CreatePresignedNotebookInstanceURL` call** — so `Describe`/`ListNotebookInstances`'
   `Url` field was unconditionally empty for every notebook instance that had never had a presigned
   URL requested for it, i.e. almost always in practice. Fixed by extracting the existing URL
   construction (`"https://" + arn + ".notebook.sagemaker.aws/lab"`) into a shared
   `notebookInstanceURL` helper and calling it at creation time (`CreateNotebookInstanceFull`, and
   the otherwise-unused legacy `CreateNotebookInstance`), so `Url` is populated from the moment a
   notebook instance exists, matching `NotebookInstanceSummary.Url`'s doc comment ("The URL that you
   use to connect...") rather than only after a separate presigned-URL call.

Both bugs discarded a real field for the entire lifetime of the resource (case 1) or its entire
existence up to an unrelated later call (case 2) — the same "field decoded/declared but silently
never populated by the real path" class as parity-11/parity-12's discarded response bodies, just on
the request-decoding and model-population sides respectively rather than the response-assembly
side.

**A doc-vs-behavior judgment call:** `ListNotebookInstances`/`ListNotebookInstanceLifecycleConfigs`
both document a default sort key explicitly (`Name`/`CreationTime` respectively) but neither
documents a default `SortOrder`. Kept Ascending as the disclosed fallback, consistent with every
prior pass that hit this same gap (`ListHubs`, `ListPipelines`, parity-11's `ListMlflowApps`/
`ListMlflowTrackingServers`).

**Disclosures:**

- `DescribeNotebookInstanceOutput.FailureReason` — this backend's notebook FSM (`lifecycle.go`)
  only ever transitions through Pending/InService/Stopping/Stopped; it never reaches `Failed`, so
  there is no real failure to report a reason for.
- `DescribeNotebookInstanceOutput.NetworkInterfaceId` — no VPC ENI-provisioning subsystem exists in
  this service to generate a real network interface ID from.
- `UpdateNotebookInstanceInput.AcceleratorTypes`/`DisassociateAcceleratorTypes` — both are AWS's own
  documented no-ops (Elastic Inference is retired); not modeling them changes nothing a real client
  can observe.
- `CreatePresignedNotebookInstanceUrlInput.SessionExpirationDurationInSeconds` — accepted for wire
  shape but has no session-expiry mechanism to enforce against, the same class as parity-11's two
  Mlflow presigned-URL ops.

**Tests:** three real-`aws-sdk-go-v2`-client round-trip tests added —
`TestHandler_ListNotebookInstances_FilterSortPage_RealClient` (covers
`AdditionalCodeRepositoryEquals`/`CreationTimeAfter`/`CreationTimeBefore`/
`DefaultCodeRepositoryContains`/`NotebookInstanceLifecycleConfigNameContains`/`SortBy`/`SortOrder`/
`MaxResults`, plus the four new `NotebookInstanceSummary` fields, and is the test that caught bug
1), `TestHandler_ListNotebookInstanceLifecycleConfigs_FilterSortPage_RealClient` (covers
`CreationTimeAfter`/`CreationTimeBefore`/`MaxResults`/`NameContains`/`SortBy`/`SortOrder`), and
`TestHandler_CreateUpdateNotebookInstance_FullFields_RealClient` (covers `IpAddressType`/
`InstanceMetadataServiceConfiguration` on both Create and Update, and `PlatformIdentifier`/
`RootAccess` on Update). All pre-existing notebook tests (`TestHandler_NotebookInstanceLifecycle`,
`TestListNotebookInstances_Filters`, `TestCreateNotebookInstance_Validation`,
`TestUpdateNotebookInstance_RequiresStoppedState`) still pass unmodified. Verified against unfixed
code by hand-reverting both bug fixes one at a time — the `LifecycleConfigName` wire-key correction
(reverted to `NotebookInstanceLifecycleConfigName` in both `createNotebookInstanceFullRequest` and
`updateNotebookInstanceInput`) and the `nb.URL` population at creation (removed from
`CreateNotebookInstanceFull`) — confirming each corresponding assertion in
`TestHandler_ListNotebookInstances_FilterSortPage_RealClient` failed with the predicted symptom
(lifecycle config name empty / filter returning zero matches; `Url` empty), then restoring;
`handler_notebook_instances.go`/`notebook_instances.go` verified byte-identical (`md5sum`) to their
pre-revert state afterward.

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` all clean;
`go build ./...` (repo-wide) also clean. Zero `nolint` of any kind added — fixed two `cyclop`
findings by extracting `matchesNotebookStringFilters`/`matchesNotebookTimeWindows` out of
`matchesNotebookParams` and `applyNotebookUpdateOptions` out of `UpdateNotebookInstanceFull`, one
`fieldalignment` finding by reordering `createPresignedNotebookInstanceURLInput`'s fields, one
`govet shadow` finding in a new test, and several `golines`/`lll` line-length findings (mostly from
a long shared `imdsConfigRequest` type name pushing struct-tag alignment past 120 columns) by
shortening the type name and rewrapping the affected lines — rather than suppressing any of them.

**258 of sagemaker's 362 inline structs now remain.** Next by size:
`handler_images.go` (11), `handler_edge_deployment.go` (11), `handler_hyperpod_scheduling.go` (10).

## parity-14 (2026-08-21, gopherstack-oc9v): Image/ImageVersion and EdgeDeploymentPlan/EdgeDeploymentStage family inline-struct sweep

Eighth pass of the gopherstack-oc9v campaign. Per parity-13's boundary note ("258 of sagemaker's
362 inline structs now remain ... `handler_images.go` (11), `handler_edge_deployment.go` (11),
`handler_hyperpod_scheduling.go` (10)"), this pass took both `handler_images.go` and
`handler_edge_deployment.go`, each verified by `grep -c 'var req struct {' <file>.go` = 11 before
starting. All 22 were converted to named types (`createImageInput`, `describeImageInput`,
`deleteImageInput`, `listImagesInput`, `updateImageInput`, `createImageVersionInput`,
`describeImageVersionInput`, `deleteImageVersionInput`, `listImageVersionsInput`,
`updateImageVersionInput`, `listAliasesInput`; `createEdgeDeploymentPlanInput`,
`describeEdgeDeploymentPlanInput`, `deleteEdgeDeploymentPlanInput`,
`listEdgeDeploymentPlansInput`, `createEdgeDeploymentStageInput`, `deleteEdgeDeploymentStageInput`,
`startEdgeDeploymentStageInput`, `stopEdgeDeploymentStageInput`, `getDeviceFleetReportInput`,
`listStageDevicesInput`, `updateDevicesInput`) and wire-audited field-by-field against the pinned
SDK (`v1.263.2`, confirmed from `go.mod`, matching prior passes). **236 of sagemaker's 362 inline
structs now remain** (362 − 19 − 19 − 15 − 14 − 14 − 12 − 11 − 11 − 11), confirmed by
`grep -rc 'var req struct {' services/sagemaker/*.go` summed, not arithmetic; both files now have
zero. This pass did not touch `handler_hyperpod_scheduling.go` or any other family — all still open
for gopherstack-oc9v.

**Enumerated vs. converted vs. audited — Image family:**

- `CreateImageInput` (`api_op_CreateImage.go:32-55`) — was missing `DisplayName`, the only optional
  field besides `Description`/`Tags` already present. Now threaded onto `SMImage.DisplayName` at
  creation instead of only being settable via a later `UpdateImage` call.
- `DescribeImageInput`/`DeleteImageInput` — already matched exactly (`ImageName` is each op's sole
  field).
- `ListImagesInput` (`api_op_ListImages.go:32-64`) — was missing **7 of 8** optional fields, only
  `NextToken` existed: `CreationTimeAfter`, `CreationTimeBefore`, `LastModifiedTimeAfter`,
  `LastModifiedTimeBefore`, `MaxResults`, `NameContains`, `SortBy`, `SortOrder`. All eight now real
  via a new `ListImagesParams`/`matchesImageListParams`/`imageSortLess`. **The real default
  `SortOrder` is Descending** (`api_op_ListImages.go:60`), not the Ascending default nearly every
  other List op in this service uses — implemented as documented rather than reusing the more
  common convention by reflex. `ImageSortBy`'s real values are the all-caps, underscore-separated
  `CREATION_TIME`/`LAST_MODIFIED_TIME`/`IMAGE_NAME` (`types/enums.go:4180-4182`), a different casing
  convention from the mixed-case `"Name"`/`"CreationTime"` sort keys most other List ops in this
  service use — read from the enum constants, not assumed by analogy. The `Images` summary
  (`types.Image`, `types/types.go:11531-11568`) was also missing `Description`/`DisplayName`; both
  now populated when set.
- `UpdateImageInput` — already matched exactly.
- `CreateImageVersionInput` (`api_op_CreateImageVersion.go:30-100`) — **the standout finding of
  this pass, matching parity-12's `UpdateModelPackage` in severity.** `BaseImage` ("This member is
  required") was not modeled at all — the previous handler only decoded `ImageName`, so a version's
  underlying container image could never be set at creation. Now required (`ErrValidation` if
  empty) and threaded onto two new `ImageVersion` fields, `BaseImage` and `ContainerImage` (see
  disclosure below). The other **7 of 7** optional fields (`Aliases`, `Horovod`, `JobType`,
  `MLFramework`, `Processor`, `ProgrammingLang`, `ReleaseNotes`, `VendorGuidance`) were also entirely
  absent, forcing every real client to make an immediate follow-up `UpdateImageVersion` call just to
  set them — the same "no way to set at creation" class parity-12 found on
  `CreateModelPackage.ModelApprovalStatus`. All now real. `ClientToken` (idempotency token, no
  server-observable effect) is deliberately omitted, matching every other Create op in this service.
- `DescribeImageVersionInput` (`api_op_DescribeImageVersion.go:34-48`) — missing `Alias`
  entirely, and **the handler did not implement the documented default at all**: "Version: ... If
  not specified, the latest version is described" (`:44`), but the previous code passed a
  zero-value `int` straight through to a `versions[version]` lookup, so an unspecified Version
  always 404'd instead of returning the latest version — a real functional bug independent of the
  wire-field diff, caught by `TestHandler_DescribeImageVersion_DefaultsToLatest_RealClient` and
  hand-reverted below. Fixed via a new `resolveImageVersionNumber`/`latestImageVersion` pair shared
  with `UpdateImageVersion`/`DeleteImageVersion`/`ListAliases`.
- `DeleteImageVersionInput`/`UpdateImageVersionInput` — each missing `Alias`. Both now resolve
  `Alias` to a version number via the same shared helper as Describe. **Delete's doc states no
  "if unspecified" default** (unlike Describe's explicit latest-version default), so
  `DeleteImageVersion` now requires one of `Version`/`Alias` and returns `ErrValidation` if neither
  is given, rather than guessing — the create/update/describe/delete-are-different-contracts
  principle extended to a fourth op.
- `ListImageVersionsInput` (`api_op_ListImageVersions.go:31-65`) — was missing **6 of 8**
  optional fields, only `ImageName`/`NextToken` existed: `CreationTimeAfter`, `CreationTimeBefore`,
  `LastModifiedTimeAfter`, `LastModifiedTimeBefore`, `MaxResults`, `SortBy`, `SortOrder`. All now
  real via a new `ListImageVersionsParams`/`matchesImageVersionListParams`/`imageVersionSortLess`,
  again with the real Descending default (`:61`) and the all-caps `ImageVersionSortBy` values
  (`CREATION_TIME`/`LAST_MODIFIED_TIME`/`VERSION`, `types/enums.go:4249-4251`). **The
  `ImageVersions` summary type (`types.ImageVersion`, `types/types.go:11606-11642`) requires
  `ImageArn` — the previous handler never emitted it at all**, so every real client's
  `ListImageVersions` call saw an empty `ImageArn` on every summary; fixed by adding it to the
  response map. Confirmed load-bearing by hand-revert below.
- `ListAliasesInput` (`api_op_ListAliases.go:28-50`) — missing `Alias`/`MaxResults`. `Alias`
  resolves to a version the same way Describe/Update/Delete do (a version-or-alias identifier, not
  a separate concept), narrowing the aggregation to that one version's aliases;
  `MaxResults` now threaded into the existing `paginateSlice` call instead of a hardcoded `0`.

**Two fabricated-wire-shape/behavior bugs found beyond the missing-field diff, both pre-dating this
pass:**

1. `DescribeImageVersion` called with no `Version`/`Alias` 404'd instead of returning the latest
   version, contradicting the real op's own documented default. Fixed via `latestImageVersion`
   (highest existing version key, not the create-time counter, so a deleted top version doesn't
   leave the fallback pointing at nothing).
2. `ListImageVersions`' summary never emitted the required `ImageArn` field.

**Disclosed, not modeled:**

- `SMImage`/`ImageVersion.FailureReason` — `ImageStatus`/`ImageVersionStatus` never reach
  `CREATE_FAILED`/`UPDATE_FAILED`/`DELETE_FAILED` in this backend (no failure FSM), so there is no
  real failure to report a reason for.
- `ImageVersion.ContainerImage` is set equal to `BaseImage` at creation: this backend has no ECR
  subsystem to resolve `BaseImage` to a distinct digest-pinned registry path the way real AWS can
  after validation, so the two coincide for this backend's entire lifetime rather than only at
  creation.

**Enumerated vs. converted vs. audited — EdgeDeploymentPlan/EdgeDeploymentStage family:**

- `CreateEdgeDeploymentPlanInput`/`CreateEdgeDeploymentStageInput`/
  `Delete{EdgeDeploymentPlan,EdgeDeploymentStage}Input`/`Start/StopEdgeDeploymentStageInput`/
  `GetDeviceFleetReportInput`/`UpdateDevicesInput` — already matched exactly; converted to named
  types for tooling visibility with no field changes.
- `DescribeEdgeDeploymentPlanInput` (`api_op_DescribeEdgeDeploymentPlan.go:43-58`) — missing
  `MaxResults`/`NextToken` entirely, and consequently `DescribeEdgeDeploymentPlanOutput.NextToken`
  was never emitted either: **`Stages` was always returned in full**, contradicting the op's own doc
  ("If the edge deployment plan has enough stages to require tokening, then this is the response
  from the last list of stages returned", `:50-56`). Fixed by paginating `Stages` with the existing
  `paginateSlice` helper and returning `NextToken` when truncated. Confirmed load-bearing by
  hand-revert below (a real client requesting 3 stages 2-at-a-time got all 3 back on the first
  page instead of 2).
- `ListEdgeDeploymentPlansInput` (`api_op_ListEdgeDeploymentPlans.go:30-65`) — was missing **8 of
  9** optional fields, only `NextToken` existed: `CreationTimeAfter`, `CreationTimeBefore`,
  `DeviceFleetNameContains`, `LastModifiedTimeAfter`, `LastModifiedTimeBefore`, `MaxResults`,
  `NameContains`, `SortBy`, `SortOrder`. All eight now real via a new
  `ListEdgeDeploymentPlansParams`/`matchesEdgeDeploymentPlanListParams`/
  `edgeDeploymentPlanSortLess`. **The op's own doc comment lists `SortBy`'s values without
  underscores — "NAME, DEVICEFLEETNAME, CREATIONTIME, LASTMODIFIEDTIME" — but the real
  `ListEdgeDeploymentPlansSortBy` enum constants are `NAME`/`DEVICE_FLEET_NAME`/`CREATION_TIME`/
  `LAST_MODIFIED_TIME`** (`types/enums.go:5312-5315`), underscored: another instance of this
  campaign's doc-prose-vs-enum-source mismatch, caught by reading the constants directly rather than
  the comment. No default `SortBy`/`SortOrder` is documented for this op (unlike `ListImages`'
  explicit Descending default above); `CreationTime`/Ascending kept as the disclosed fallback,
  this campaign's recurring `ListHubs`/`ListPipelines`/`ListNotebookInstanceLifecycleConfigs`
  precedent for an undocumented default. `SortOrder` itself is the generic mixed-case
  `types.SortOrder` (`"Ascending"`/`"Descending"`, `types/enums.go:9240-9246`), the same enum
  `ListModelPackages` uses — unlike `ImageSortOrder` above, this op does *not* have its own
  all-caps sort-order enum, only its own sort-*by* enum; the two List families sit on opposite
  sides of that split, confirmed by reading each op's field type rather than assuming consistency
  across the service.
- `ListStageDevicesInput` (`api_op_ListStageDevices.go:30-53`) — missing
  `ExcludeDevicesDeployedInOtherStage`/`MaxResults`. `MaxResults` now threads through
  `devicesInFleetPaged` (a helper shared with the out-of-scope `ListDevices` op, updated to accept
  it with a `0` default preserving that op's existing behavior). `ExcludeDevicesDeployedInOtherStage`
  is accepted for wire-shape fidelity but is a **real, disclosed no-op**: this backend tracks one
  `DeploymentStatus` per stage, not a per-device-per-stage assignment record, so there is no
  "deployed in another stage" fact for a real device to be excluded on.

**Bugs found beyond the wire diff, this family:**

1. `DescribeEdgeDeploymentPlan` silently returning every stage regardless of `MaxResults`/
   `NextToken`, contradicting the op's own doc.
2. `ListEdgeDeploymentPlans` accepting only `NextToken` while silently dropping every filter/sort
   control — the "parsed field, silently dropped" class this campaign exists to find.

**Disclosed, not modeled:**

- `EdgeDeploymentPlan`/`EdgeDeploymentPlanSummary.EdgeDeploymentSuccess`/`Pending`/`Failed` — always
  zero, consistent with the pre-existing `stageStatusSummary` per-stage device-count disclosure:
  this backend does not simulate per-device deployment progress at all.
- `ListStageDevicesInput.ExcludeDevicesDeployedInOtherStage` — accepted but a real no-op, see above.

**Storage-key check:** neither `SMImage` (keyed by `ImageName`), `ImageVersion` (keyed by
`imageName`+`version` in a nested map), nor `EdgeDeploymentPlan` (keyed by
`EdgeDeploymentPlanName`) changed key shape this pass.

**Enums touched, all read from the constants:** `ImageStatus`, `ImageSortBy`, `ImageSortOrder`,
`ImageVersionStatus`, `ImageVersionSortBy`, `ImageVersionSortOrder`, `JobType`, `Processor`,
`VendorGuidance` (Image family — all match their doc comments' casing, unlike some prior passes'
finds); `ListEdgeDeploymentPlansSortBy` (doc comment wrong, underscores omitted — see above) and
the generic `SortOrder` (Edge family).

**Tests:** real-`aws-sdk-go-v2`-client round-trip tests added for every new field —
`TestHandler_CreateImage_DisplayName_RealClient`, `TestHandler_ListImages_FilterSortPage_RealClient`,
`TestHandler_CreateImageVersion_FullFields_RealClient`,
`TestHandler_DescribeImageVersion_DefaultsToLatest_RealClient`,
`TestHandler_ImageVersionAlias_RealClient`,
`TestHandler_DeleteImageVersion_RequiresIdentifier_RealClient`,
`TestHandler_ListImageVersions_FilterSortPage_RealClient`,
`TestHandler_ListAliases_AliasAndMaxResults_RealClient`,
`TestHandler_DescribeEdgeDeploymentPlan_StagesPaginated_RealClient`,
`TestHandler_ListEdgeDeploymentPlans_FilterSortPage_RealClient`,
`TestHandler_ListStageDevices_MaxResults_RealClient`. All pre-existing Image/EdgeDeployment tests
updated only where they exercised a field the real client is required to send
(`CreateImageVersion`'s pre-existing tests all send `BaseImage` now) or otherwise pass unmodified.
Verified against unfixed code by hand-reverting four representative fixes one at a time —
`ListImageVersions`' `ImageArn` summary field (removed), `DescribeImageVersion`'s latest-version
fallback (disabled, forcing the stale zero-value lookup), `DescribeEdgeDeploymentPlan`'s `Stages`
pagination (disabled), and `ListEdgeDeploymentPlans`' `NameContains` filter (short-circuited) —
confirming each corresponding test failed with the predicted symptom (empty `ImageArn`; "version 0
not found"; 3 stages returned instead of 2; 3 plans returned instead of 2 for a substring filter)
— then restoring; `images.go`/`handler_images.go`/`edge_deployment.go`/
`handler_edge_deployment.go`/`device_fleets.go` verified byte-identical (`md5sum`) to their
pre-revert state afterward.

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` all clean;
`go build ./...` (repo-wide) also clean. Zero `nolint` of any kind added — fixed a `goconst` finding
by adding a shared `sortByLastModifiedTime` constant (the `"LAST_MODIFIED_TIME"` literal recurs
across `Image`/`ImageVersion`/`EdgeDeploymentPlan`'s three distinct sort-by enums), a `revive
unused-parameter` finding by naming `ListStageDevices`' disclosed-no-op parameter `_` (the same
convention already used by `DescribeModelPackage`'s `IncludedData` parameter), and several
`golines` line-length findings by rewrapping the affected call sites — rather than suppressing any
of them.

**`last_audit_commit` left at its existing value (`5f91d37c7`)** — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**236 of sagemaker's 362 inline structs now remain.** Next by size:
`handler_hyperpod_scheduling.go` (10), `handler_device_fleets.go` (9), `handler_cluster.go` (9).

## parity-15 (2026-08-21, gopherstack-oc9v): ClusterSchedulerConfig/ComputeQuota (HyperPod scheduling) inline-struct sweep

Ninth pass of the gopherstack-oc9v campaign. Per parity-14's boundary note ("236 of sagemaker's
362 inline structs now remain ... `handler_hyperpod_scheduling.go` (10), `handler_device_fleets.go`
(9), `handler_cluster.go` (9)"), this pass took `handler_hyperpod_scheduling.go`, verified by
`grep -c 'var req struct {' handler_hyperpod_scheduling.go` = 10 before starting. All 10 were
converted to named types (`createClusterSchedulerConfigInput`,
`describeClusterSchedulerConfigInput`, `listClusterSchedulerConfigsInput`,
`updateClusterSchedulerConfigInput`, `deleteClusterSchedulerConfigInput`,
`createComputeQuotaInput`, `describeComputeQuotaInput`, `listComputeQuotasInput`,
`updateComputeQuotaInput`, `deleteComputeQuotaInput`) and wire-audited field-by-field against the
pinned SDK (`v1.263.2`, confirmed from `go.mod`, matching prior passes). **226 of sagemaker's 362
inline structs now remain** (362 − 19 − 19 − 15 − 14 − 14 − 12 − 11 − 11 − 10), confirmed by
`grep -rc 'var req struct {' services/sagemaker/*.go` summed, not arithmetic; the file itself now
has zero. This pass did not touch `handler_device_fleets.go`/`handler_cluster.go` or any other
family — both still open for gopherstack-oc9v.

This family's backend (`hyperpod_scheduling.go`) was already unusually well-audited going in — its
existing comments already cite `api_op_*.go` line numbers and correctly disclose `ClusterArn` as
Update-non-settable — so this pass's diff is narrower than most, but it still found a real Create
bug and a real absent-field bug beyond the wire diff.

**Enumerated vs. converted vs. audited — ClusterSchedulerConfig:**

- `CreateClusterSchedulerConfigInput` (`api_op_CreateClusterSchedulerConfig.go:30-54`) — already
  matched exactly (`ClusterArn`/`Name`/`SchedulerConfig`/`Description`/`Tags`).
- `DescribeClusterSchedulerConfigInput` (`api_op_DescribeClusterSchedulerConfig.go:31-42`) — missing
  the optional `ClusterSchedulerConfigVersion` entirely, previously undecoded. This backend keeps
  only the live version counter, not a per-version historical snapshot, so a request for the
  current version now succeeds and a request for any other version returns `ResourceNotFound`
  rather than fabricating a snapshot that was never stored — disclosed in
  `DescribeClusterSchedulerConfig`'s new doc comment, not silently ignored as before.
- `ListClusterSchedulerConfigsInput` (`api_op_ListClusterSchedulerConfigs.go:30-68`) — was missing
  **8 of 9** fields, only `NextToken` existed: `ClusterArn`, `CreatedAfter`, `CreatedBefore`,
  `MaxResults`, `NameContains`, `SortBy`, `SortOrder`, `Status`. All eight now real via a new
  `ListClusterSchedulerConfigsParams`/`matchesClusterSchedulerConfigListParams`/
  `clusterSchedulerConfigSortLess`. Unlike the Image family (parity-14), this op's time filters are
  named `CreatedAfter`/`CreatedBefore`, not `CreationTimeAfter`/`CreationTimeBefore` — read from
  this op's own field list rather than assumed by analogy — and it has no `LastModifiedTime*`
  filters at all. **The real default `SortOrder` is Descending** (`api_op_
  ListClusterSchedulerConfigs.go:62`), the same explicit-Descending-default pattern as `ListImages`;
  `SortBy`'s own default is undocumented, kept as the disclosed `CreationTime` fallback per this
  campaign's recurring precedent. `SortClusterSchedulerConfigBy`'s real values
  (`Name`/`CreationTime`/`Status`, `types/enums.go:9123-9125`) are mixed-case, matching most other
  List ops in this service — unlike the Image family's all-caps enums, read from the constants
  directly rather than assumed either way.
- `UpdateClusterSchedulerConfigInput`/`DeleteClusterSchedulerConfigInput` — already matched exactly.

**A real Create bug found beyond the wire diff, from this campaign's "does it honour its own
documented defaults / does it ever reach a terminal state" question:** `CreateClusterSchedulerConfig`
set the new resource's `Status` to `statusCreating` ("Creating") and nothing anywhere in this
backend ever transitioned it forward — no ticker, no lifecycle goroutine, nothing else writes to a
`ClusterSchedulerConfig`'s `Status` field at all. Every `DescribeClusterSchedulerConfig`/
`ListClusterSchedulerConfigs` call showed `Status: "Creating"` for the entire lifetime of every
cluster scheduler config ever created, in a backend with no failure FSM to ever advance it further.
Its sibling `ComputeQuota` (same file) already set `Status: statusCreated` at creation, landing
directly on the terminal state — the correct pattern. Fixed by matching that sibling. **The
pre-existing `TestClusterSchedulerConfigLifecycle_RealClient` asserted
`smtypes.SchedulerResourceStatusCreating` on a freshly created resource** — enshrining the bug, the
same class of finding as parity-12's `UpdateModelPackage` test — and was updated to assert
`SchedulerResourceStatusCreated` instead, matching `TestComputeQuotaLifecycle_RealClient`'s existing
assertion two tests below it.

**Enumerated vs. converted vs. audited — ComputeQuota:**

- `CreateComputeQuotaInput` (`api_op_CreateComputeQuota.go:30-65`) — already matched exactly.
- `DescribeComputeQuotaInput` (`api_op_DescribeComputeQuota.go:29-40`) — missing the optional
  `ComputeQuotaVersion`, same fix and same rationale as `ClusterSchedulerConfig`'s above.
- `ListComputeQuotasInput` (`api_op_ListComputeQuotas.go:30-68`) — was missing **8 of 9** fields,
  only `NextToken` existed: same eight as `ListClusterSchedulerConfigs` above (`ClusterArn`,
  `CreatedAfter`, `CreatedBefore`, `MaxResults`, `NameContains`, `SortBy`, `SortOrder`, `Status`).
  All eight now real via a new `ListComputeQuotasParams`/`matchesComputeQuotaListParams`/
  `computeQuotaSortLess`, again with the documented Descending `SortOrder` default (`:62`) and the
  undocumented `SortBy` default kept as `CreationTime`. **`SortQuotaBy` has a fourth value beyond its
  `SortClusterSchedulerConfigBy` sibling** — `Name`/`CreationTime`/`Status`/`ClusterArn`
  (`types/enums.go:9301-9304`) — read from this op's own enum rather than assumed identical to the
  three-value sibling right beside it.
- `UpdateComputeQuotaInput`/`DeleteComputeQuotaInput` — already matched exactly.

**An absent-field bug found beyond the wire diff:** `ClusterSchedulerConfigSummary`
(`types/types.go:5687-5724`) has an optional but real `ClusterArn` field — the previous
`ListClusterSchedulerConfigs` summary map never included it at all, so every real client's list call
saw no cluster association for any entry, even though `Describe` on the same resource always
returned it correctly. `ComputeQuotaSummary`'s sibling map already emitted its own `ClusterArn`
correctly; only the `ClusterSchedulerConfig` side had the gap. Fixed by adding it to the summary map.

**Disclosed, not modeled:**

- `ClusterSchedulerConfig`/`ComputeQuota`'s `FailureReason` (both Describe outputs) and
  `ClusterSchedulerConfig`'s `StatusDetails` — `Status` never reaches a Failed state in this backend
  (no failure FSM), so there is no real failure to report a reason or per-status detail for.
- `CreatedBy`/`LastModifiedBy` (`types.UserContext`) on both Describe outputs — this service models
  no caller-identity concept anywhere, the same gap already disclosed for
  `describeMlflowTrackingServerResponse`/`describeMlflowAppResponse`/`ModelPackageGroup`.
- `DescribeClusterSchedulerConfigInput.ClusterSchedulerConfigVersion`/
  `DescribeComputeQuotaInput.ComputeQuotaVersion` — modeled and enforced (see above) but only ever
  honor the current version, since this backend keeps no historical per-version snapshot.

**Storage-key check:** `ClusterSchedulerConfig` and `ComputeQuota` both stayed keyed by Name in
their `store.Table`s (Describe/Update/Delete resolve `...Id` to the matching row via
`clusterSchedulerConfigByID`/`computeQuotaByID`, both pre-existing and unchanged this pass) — no key
shape change.

**Enums touched, all read from the constants:** `SchedulerResourceStatus`,
`SortClusterSchedulerConfigBy`, `SortQuotaBy`, `ActivationState` — the first two confirmed
mixed-case (unlike the Image family) and confirmed to *not* share a value set (`SortQuotaBy`'s extra
`ClusterArn`), read per-op rather than assumed.

**Tests:** four new real-`aws-sdk-go-v2`-client round-trip tests —
`TestListClusterSchedulerConfigs_FilterSortPage_RealClient` (covers `ClusterArn` filter and summary
emission, `NameContains`, `Status`, `SortBy`/`SortOrder`, `MaxResults`),
`TestListComputeQuotas_FilterSortPage_RealClient` (covers `NameContains`, `SortBy`/`SortOrder`,
`MaxResults`), and `TestDescribeVersion_RealClient` (both resources: matching version succeeds,
mismatched version returns `ResourceNotFound`). One pre-existing test
(`TestClusterSchedulerConfigLifecycle_RealClient`) updated per the enshrined-bug finding above; all
other pre-existing hyperpod-scheduling tests pass unmodified. Verified against unfixed code by
hand-reverting three representative fixes one at a time — the `Status: statusCreated` correction
(reverted to `statusCreating`), the `DescribeClusterSchedulerConfig`/`DescribeComputeQuota` version
mismatch check (short-circuited to never fire), and `ListClusterSchedulerConfigs`' summary
`ClusterArn` field (removed) — confirming each corresponding test failed with the predicted symptom
(`Status` "Creating" instead of "Created"; mismatched-version Describe wrongly succeeding instead of
erroring; empty `ClusterArn` instead of the real value) — then restoring; `hyperpod_scheduling.go`/
`handler_hyperpod_scheduling.go` verified byte-identical (`md5sum`) to their pre-revert state
afterward.

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker` (empty after one `gofmt -w` pass to realign a struct literal after
an inserted comment), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` all clean;
`go build ./...` (repo-wide) also clean. Zero `nolint` of any kind added — fixed two `golines`
line-length findings (one from an inlined argument list, one in the new test file) by rewrapping via
`golines -m 120 -w`, and two `fieldalignment` findings in `describeClusterSchedulerConfigInput`/
`describeComputeQuotaInput` by reordering the pointer field ahead of the string field.

**`last_audit_commit` left at its existing value (`5f91d37c7`)** — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**226 of sagemaker's 362 inline structs now remain.** Next by size (tied):
`handler_device_fleets.go` (9), `handler_cluster.go` (9), then `handler_monitoring_schedules.go` (7),
`handler_model_cards.go` (7), `handler_jobs.go` (7), `handler_inference_experiments.go` (7).

## parity-16 (2026-08-21, gopherstack-oc9v): DeviceFleet/Device and HyperPod Cluster inline-struct sweep, plus a repo-spanning epoch-time-decode bug found across three prior passes

Tenth pass of the gopherstack-oc9v campaign. Per parity-15's boundary note ("226 of sagemaker's 362
inline structs now remain ... `handler_device_fleets.go` (9), `handler_cluster.go` (9)"), this pass
took both files, each verified by `grep -c 'var req struct {' <file>.go` = 9 before starting. All 18
were converted to named types (`createDeviceFleetInput`, `describeDeviceFleetInput`,
`listDeviceFleetsInput`, `updateDeviceFleetInput`, `deleteDeviceFleetInput`, `registerDevicesInput`,
`deregisterDevicesInput`, `describeDeviceInput`, `listDevicesInput`; `startClusterHealthCheckInput`,
`describeClusterInput`, `listClustersInput`, `deleteClusterInput`, `updateClusterSoftwareInput`,
`describeClusterNodeInput`, `listClusterNodesInput`, `describeClusterEventInput`,
`listClusterEventsInput`) and wire-audited field-by-field against the pinned SDK (`v1.263.2`,
confirmed from `go.mod`, matching prior passes). **208 of sagemaker's 362 inline structs now remain**
(362 − 19 − 19 − 15 − 14 − 14 − 12 − 11 − 11 − 10 − 9 − 9), confirmed by
`grep -rc 'var req struct {' services/sagemaker/*.go` summed, not arithmetic; both files now have
zero.

**A repo-spanning bug found first, before any new code was written:** deciding how to decode this
pass's own time filters required checking how the *previous three passes* (parity-14 x2,
parity-15 x2) had done it, since their List ops establish the pattern this pass would otherwise copy.
`ListImages`/`ListImageVersions` (`handler_images.go`), `ListEdgeDeploymentPlans`
(`handler_edge_deployment.go`), and `ListClusterSchedulerConfigs`/`ListComputeQuotas`
(`handler_hyperpod_scheduling.go`) all decoded their `CreationTimeAfter`/`CreationTimeBefore`/
`CreatedAfter`/`CreatedBefore` filters straight into a `*time.Time`-typed request field. The real
client serializes these as awsjson1.1 epoch-second **numbers** (confirmed at
`serializers.go:45256-45258` for `ListImages`), never RFC3339 strings — and Go's `encoding/json`
cannot unmarshal a bare number into `time.Time` (`Time.UnmarshalJSON: input is not a JSON string`),
confirmed by a standalone repro. **Any real client call setting any of those five ops' time filters
failed outright** with a 400, the entire request rejected by the top-level `json.Unmarshal` error,
not just the filter silently ignored. No existing test caught it because each pass's own
`_FilterSortPage_RealClient` test exercised `NameContains`/`SortBy`/`SortOrder`/`MaxResults` but never
the time filters themselves. The correct pattern, already used correctly by
`handler_notebook_instances.go`/`handler_hub.go`/`handler_pipelines.go`/`handler_lineage.go`/
`handler_mlflow.go`/`handler_model_packages.go`/`handler_trial_components.go`, decodes to `*float64`
and converts via `timeFromEpochSecondsPtr`. **Fixed in all five prior ops** (both `handler_images.go`
List ops, `handler_edge_deployment.go`'s, and both `handler_hyperpod_scheduling.go` ops) as a
prerequisite to writing this pass's own five time-filtered List ops correctly, with a regression
subtest (`creation time filter(s) do not error` / `created after filter does not error`) added to
each op's existing `_FilterSortPage_RealClient` test. Hand-reverted `handler_images.go`'s fix alone,
confirmed `TestHandler_ListImages_FilterSortPage_RealClient/creation_time_filters_do_not_error` failed
with exactly the predicted `Time.UnmarshalJSON` error, then restored (`md5sum`-verified). This is the
same bug class the `time.Time`/`*float64` distinction always is in this campaign, just never
previously triggered because no test had populated those particular fields.

**Enumerated vs. converted vs. audited — DeviceFleet:**

- `CreateDeviceFleetInput` (`api_op_CreateDeviceFleet.go:28-59`) — was missing `EnableIotRoleAlias`
  entirely, and `OutputConfig` (`types.EdgeOutputConfig`, `types/types.go:7856-7903`) was missing two
  of its four fields, `PresetDeploymentConfig`/`PresetDeploymentType`. `EnableIotRoleAlias` now
  synthesizes `DescribeDeviceFleetOutput.IotRoleAlias` as `"SageMakerEdge-{DeviceFleetName}"`, exactly
  the pattern the input doc itself specifies — previously **`IotRoleAlias` had no source at all**,
  not even a stub field on `DeviceFleet`. `UpdateDeviceFleetInput` had the identical gap and is fixed
  the same way, including toggling the alias back off when explicitly disabled.
- `DescribeDeviceFleetInput`/`DeleteDeviceFleetInput` — already matched exactly.
- `ListDeviceFleetsInput` (`api_op_ListDeviceFleets.go:30-61`) — was missing **8 of 9** optional
  fields, only `NextToken` existed: `CreationTimeAfter`, `CreationTimeBefore`,
  `LastModifiedTimeAfter`, `LastModifiedTimeBefore`, `MaxResults`, `NameContains`, `SortBy`,
  `SortOrder`. All eight now real via a new `ListDeviceFleetsParams`/`matchesDeviceFleetListParams`/
  `deviceFleetSortLess`. No default `SortBy`/`SortOrder` is documented for this op; `CreationTime`/
  Ascending kept as the disclosed undocumented-default fallback, this campaign's recurring precedent.
  `ListDeviceFleetsSortBy`'s real values are `NAME`/`CREATION_TIME`/`LAST_MODIFIED_TIME`
  (`types/enums.go:5291-5293`), all-caps like the Image family, not mixed-case like most other List
  ops — read from the enum constants, not assumed.
- `RegisterDevicesInput` (`api_op_RegisterDevices.go:28-40`) — **the standout finding of this
  family.** The previous handler read `Tags` from a **per-device key that does not exist on the real
  wire at all**: `types.Device` (`types/types.go:7222-7236`), the wire type for each
  `RegisterDevicesInput.Devices` entry, has only `DeviceName`/`Description`/`IotThingName` — no
  `Tags` field. The real, top-level `RegisterDevicesInput.Tags` (applying to every device in the
  batch) was never read at all. Net effect: **every real client's `RegisterDevices` call silently
  lost its tags**, regardless of whether it sent them per-device (ignored, since that shape isn't
  real) or correctly at the top level (never read). Fixed by removing the fabricated per-device
  `Tags` field and threading the real top-level `Tags` through a new `tags` parameter on
  `InMemoryBackend.RegisterDevices`, applied to every device created in that call. Device tags were
  also previously unreachable through `ListTagsForResource`/`AddTags`/`DeleteTags` at all — `tags.go`
  had no `scanTagLookup` entry for `devicesStoreRO`; added one, mirroring the pre-existing
  `DeviceFleet` entry beside it.
- `DeregisterDevicesInput` — already matched exactly.
- `DescribeDeviceInput` (`api_op_DescribeDevice.go:28-43`) — missing `NextToken` (paginates the
  `Models` field within one device's description). Decoded for wire-shape fidelity but a disclosed
  no-op: this backend's `Device` never carries `Models` at all (see below), so there is never a
  second page to token into.
- `ListDevicesInput` (`api_op_ListDevices.go:30-49`) — missing `MaxResults` (real; now threaded
  through the pre-existing `devicesInFleetPaged` helper, which already accepted a `maxResults`
  parameter the handler had simply never passed), `LatestHeartbeatAfter`, and `ModelName`. The latter
  two are decoded for wire-shape fidelity but are disclosed no-ops: `AgentVersion`, `LatestHeartbeat`,
  `Models`, and `MaxModels` on `DescribeDeviceOutput`/`DeviceSummary` all come from the SageMaker Edge
  Manager device-agent runtime protocol (`SendHeartbeat`/`GetDeviceRegistration`, on the *separate*
  `sagemakeredge` service, not `sagemaker`) — this backend has no device-agent simulation of any
  kind, so there is no heartbeat timestamp or model registration ever to have.

**Disclosed, not modeled (DeviceFleet/Device):**

- `ListDevicesInput.LatestHeartbeatAfter`/`ModelName`, `DescribeDeviceInput.NextToken` — see above.
- `DescribeDeviceOutput`/`DeviceSummary`'s `AgentVersion`/`LatestHeartbeat`/`Models`/`MaxModels` — no
  device-agent runtime exists in this backend at all (separate service, not implemented here).
- `DeviceFleetOutputConfig.PresetDeploymentConfig`/`PresetDeploymentType` are stored and echoed back
  verbatim but otherwise inert: this backend has no edge-packaging-job subsystem to actually act on a
  preset deployment configuration.

**Enums touched, all read from the constants:** `ListDeviceFleetsSortBy` (all-caps, confirmed
distinct from the mixed-case convention most other List ops use), `EdgePresetDeploymentType` (single
value, `GreengrassV2Component`).

**Storage-key check:** `DeviceFleet` (keyed by `DeviceFleetName`) and `Device` (keyed by
`fleetName|deviceName` composite) both stayed keyed the same. `Device`'s tag reachability changed
(see `RegisterDevicesInput` above), not its storage key.

**Enumerated vs. converted vs. audited — HyperPod Cluster:**

- `StartClusterHealthCheckInput`/`DescribeClusterInput`/`DeleteClusterInput`/
  `DescribeClusterEventInput` — already matched exactly; converted to named types for tooling
  visibility with no field changes.
- `ListClustersInput` (`api_op_ListClusters.go:30-71`) — was missing **6 of 8** optional fields, only
  `NameContains`/`NextToken` existed: `CreationTimeAfter`, `CreationTimeBefore`, `MaxResults`,
  `SortBy`, `SortOrder`, `TrainingPlanArn`. The first five are now real via a new
  `ListClustersParams`/`matchesClusterListParams`/`clusterSortLess`, replacing the previous
  map-then-`sagemakerListPagedMap` implementation (which had no `MaxResults` support at all — that
  helper is now dead code and removed). **The op's own doc states both real defaults explicitly**:
  `SortBy` defaults to `CREATION_TIME`, `SortOrder` to `Ascending` — the latter is this campaign's
  first *documented* Ascending default found (every prior documented default was Descending);
  implemented as documented, not assumed. `TrainingPlanArn` is decoded for wire-shape fidelity but is
  a disclosed no-op: `CreateClusterInput` has no `TrainingPlanArn` field at all (confirmed by reading
  the real op's full field list), so no cluster in this backend has ever had a training-plan
  association to filter by.
- `UpdateClusterSoftwareInput` (`api_op_UpdateClusterSoftware.go:28-63`) — was missing
  `DeploymentConfig`/`ImageId`/`InstanceGroups` entirely. All three now decoded (the array via a new
  `updateClusterSoftwareInstanceGroupRequest`) but are disclosed no-ops, consistent with this op's
  pre-existing doc comment in `cluster.go`: "this emulator applies the request immediately with no
  observable software-version state to update" — there is no per-instance-group AMI/version field
  anywhere in this backend's `ClusterInstanceGroup` for a requested image ID or rollout policy to act
  on.
- `DescribeClusterNodeInput` (`api_op_DescribeClusterNode.go:28-42`) — missing `NodeLogicalId`
  (optional; "You can specify either `NodeLogicalId` or [NodeId], but not both... `NodeLogicalId` can
  be used to describe nodes that are still being provisioned and don't yet have an `InstanceId`
  assigned"). Decoded for wire-shape fidelity but a disclosed no-op: every node in this backend gets
  its `NodeId` assigned synchronously at creation (`newClusterNode`), so there is never a
  still-provisioning, logical-id-only node for it to resolve — this handler's pre-existing
  requires-`NodeId` validation is therefore still correct, not a bug, just previously undocumented as
  a deliberate choice rather than an oversight.
- `ListClusterNodesInput` (`api_op_ListClusterNodes.go:30-70`) — was missing **7 of 9** optional
  fields, only `NextToken` existed: `CreationTimeAfter`, `CreationTimeBefore`,
  `IncludeNodeLogicalIds`, `InstanceGroupNameContains`, `MaxResults`, `SortBy`, `SortOrder`. The five
  real ones are now live via a new `ListClusterNodesParams`/`matchesClusterNodeListParams`/
  `clusterNodeSortLess`, which required adding a `CreationTime` field to `ClusterNode` itself (set at
  node creation in `newClusterNode`) since nothing tracked when a node was created before this pass.
  Both documented defaults (`CREATION_TIME`/`Ascending`) match `ListClusters`' above.
  `IncludeNodeLogicalIds` is decoded but a disclosed no-op for the same reason as
  `DescribeClusterNode`'s `NodeLogicalId` above — no still-provisioning node state exists to include
  or exclude.
- `ListClusterEventsInput` (`api_op_ListClusterEvents.go:29-72`) — was missing **8 of 9** optional
  fields, only `ClusterName` was read. All eight (`EventTimeAfter`, `EventTimeBefore`,
  `InstanceGroupName`, `MaxResults`, `NextToken`, `NodeId`, `ResourceType`, `SortBy`, `SortOrder`) are
  now decoded for wire-shape fidelity, but every one is a disclosed no-op consistent with this op's
  pre-existing doc comment: this backend never generates a single cluster event (`DescribeClusterEvent`
  always errors "not found" for the same reason), so every filter is trivially satisfied by the
  always-empty result set — there is nothing behaviorally different a real filter could produce here.

**A real absent-required-field bug found beyond the wire diff, matching this campaign's severest
class of finding (parity-14's `ListImageVersions.ImageArn`, parity-15's
`ListClusterSchedulerConfigs.ClusterArn`):** `ClusterNodeSummary` (`types/types.go:5398-5423`)
declares `LaunchTime` as a **required** member — **the previous `ListClusterNodes` handler never
emitted it at all**, so every real client's `ListClusterNodes` call saw every node summary missing a
required field, even though `DescribeClusterNode`'s sibling `ClusterNodeDetails.LaunchTime` (optional
there) was equally never populated for the single-node case. Both are now sourced from the new
`ClusterNode.CreationTime` field (the node's real creation timestamp, not a fabricated value — nodes
are created synchronously and immediately begin running in this backend, so creation time and launch
time coincide exactly). Confirmed load-bearing by hand-revert below.

**Disclosed, not modeled (HyperPod Cluster):**

- `ListClustersInput.TrainingPlanArn` — no training-plan-to-cluster association exists anywhere in
  this backend's `CreateClusterInput`/`Cluster` modeling.
- `UpdateClusterSoftwareInput.DeploymentConfig`/`ImageId`/`InstanceGroups` — no per-instance-group
  AMI/version state exists to update, consistent with `UpdateClusterSoftware`'s pre-existing
  disclosure.
- `DescribeClusterNodeInput.NodeLogicalId`/`ListClusterNodesInput.IncludeNodeLogicalIds` — no
  still-provisioning, logical-id-only node lifecycle stage exists; every node has its `NodeId`
  assigned synchronously at creation.
- `ListClusterEventsInput`'s eight filter fields — this backend never generates a cluster event,
  consistent with `DescribeClusterEvent`'s pre-existing disclosure.
- `ClusterNodeDetails`' many optional infrastructure-simulation fields (`CapacityType`, the
  Current/DesiredImage* AMI-patch fields, `KubernetesConfig`, `NetworkInterface`, `Placement`,
  `PrivateDnsHostname`/`PrivatePrimaryIp`, `ThreadsPerCore`, `UltraServerInfo`) — this backend
  simulates none of the underlying EC2/network/Kubernetes state these would require, and are out of
  this pass's scope (only `clusterNodeDetails`'/`clusterNodeSummary`'s pre-existing anonymous-struct
  request/response shapes were in scope, not `ClusterInstanceGroupSpecification`'s own many optional
  fields, which are behind the already-named, not-anonymous `clusterInstanceGroupRequest` type and
  were not touched this pass).

**Storage-key check:** `Cluster` (keyed by `ClusterName`, ARN-indexed) and `ClusterNode` (keyed by
`NodeId` within its parent `Cluster.Nodes` map) both stayed keyed the same; `ClusterNode` gained a
`CreationTime` field but no key-shape change.

**Enums touched, all read from the constants:** `ClusterSortBy` (`CREATION_TIME`/`NAME`,
types/enums.go:2775-2776 — reused unchanged by `ListClusterNodesInput`, even though a node has no
`Name` of its own; interpreted as `InstanceGroupName`, the closest analogous field, and disclosed as
such rather than silently assumed), `EventSortBy` (single value, `EventTime`),
`ClusterEventResourceType` (`Cluster`/`InstanceGroup`/`Instance`).

**Tests:** real-`aws-sdk-go-v2`-client round-trip tests added —
`TestHandler_CreateDeviceFleet_IotRoleAlias_RealClient`,
`TestHandler_CreateDeviceFleet_PresetDeploymentConfig_RealClient`,
`TestHandler_ListDeviceFleets_FilterSortPage_RealClient`,
`TestHandler_RegisterDevices_Tags_RealClient`, `TestHandler_ListDevices_MaxResults_RealClient`,
`TestHandler_ListClusters_FilterSortPage_RealClient`,
`TestHandler_ListClusterNodes_FilterSortPage_RealClient`; plus a `creation time filter(s) do not
error` subtest added to each of the five previously-broken time-filtered List ops named above.
Pre-existing `TestHandler_ClusterLifecycle` extended with `LaunchTime` presence assertions at both
the List and Describe call sites. No pre-existing test asserted a now-fixed defect's wrong shape
directly (unlike parity-12/15's enshrined-bug finds) — the `LaunchTime`/`RegisterDevices` Tags/
`EnableIotRoleAlias` gaps were pure absences no test had touched. Verified against unfixed code by
hand-reverting three representative fixes one at a time — `ListClusterNodes`' summary `LaunchTime`
field (removed, confirming the first assertion attempt with a bare `assert.NotZero` was too weak to
catch a zero-valued-but-still-epoch-1970 `LaunchTime` and had to be strengthened to
`assert.WithinDuration` before the revert would actually fail), `RegisterDevices`' tags parameter
(short-circuited to `nil`), and `CreateDeviceFleet`'s `EnableIotRoleAlias` handling (removed) —
confirming each corresponding test failed with the predicted symptom, then restoring;
`handler_cluster.go`/`device_fleets.go` verified byte-identical (`md5sum`) to their pre-revert state
afterward. The `handler_images.go` time-filter fix was also hand-reverted and restored the same way
(see above).

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` (0 issues) all
clean; `go build ./...` (repo-wide) also clean. Zero new `nolint` added. One pre-existing
`//nolint:dupl` moved (not added): adding `devicesStoreRO` to `tags.go`'s `statefulTagLookupsPart2`
shifted which pair of the three structurally-identical `statefulTagLookupsPart{1,2,3}` registration
functions the `dupl` linter matched (previously `Part1`↔`Part2`, now `Part2`↔`Part3`); the existing
justified suppression moved from `Part1` to `Part3` rather than being duplicated or dropped. Also
fixed real findings by editing, not suppressing: `goconst` (a new shared `sortByName` constant,
alongside the pre-existing `sortByLastModifiedTime`/`sortOrderDescending`, for the `"NAME"` literal
now repeated across `Cluster`/`DeviceFleet`/`EdgeDeploymentPlan`'s three distinct sort-by enums),
`golines` (one call-site rewrap), `fieldalignment -fix` (two structs), a `revive var-naming` rename
(`IncludeNodeLogicalIds` field → `IncludeNodeLogicalIDs`, JSON tag unchanged), a `staticcheck S1016`
struct-conversion simplification, and a `testifylint` `assert.JSONEq` swap.

**`last_audit_commit` left at its existing value (`5f91d37c7`)** — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**208 of sagemaker's 362 inline structs now remain.** Next by size (tied):
`handler_monitoring_schedules.go` (7), `handler_model_cards.go` (7), `handler_jobs.go` (7),
`handler_inference_experiments.go` (7).

## parity-17 (2026-08-21, gopherstack-oc9v): Job/ModelCard/MonitoringSchedule/InferenceExperiment inline-struct sweep

Eleventh pass of the gopherstack-oc9v campaign. Per parity-16's boundary note ("208 of sagemaker's
362 inline structs now remain ... `handler_monitoring_schedules.go` (7), `handler_model_cards.go`
(7), `handler_jobs.go` (7), `handler_inference_experiments.go` (7)"), this pass took all four
tied-at-7 files, each verified by `grep -c 'var req struct {' <file>.go` = 7 before starting. All 28
were converted to named types and wire-audited field-by-field against the pinned SDK (`v1.263.2`,
confirmed from `go.mod`, matching prior passes). **180 of sagemaker's 362 inline structs now
remain** (362 − 19 − 19 − 15 − 14 − 14 − 12 − 11 − 11 − 10 − 9 − 9 − 7 − 7 − 7 − 7), confirmed by
`grep -rc 'var req struct {' services/sagemaker/*.go` summed, not arithmetic; all four files now
have zero.

**`handler_jobs.go` (generic Job/JobSchemaVersion family) — already correct going in.** Every field
of `CreateJobInput`/`DescribeJobInput`/`DeleteJobInput`/`StopJobInput`/`ListJobsInput`/
`DescribeJobSchemaVersionInput`/`ListJobSchemaVersionsInput` was already decoded, including
`ListJobs`' four time filters, which were already `*float64`/`epochPtr` — this family was added in
an earlier SDK-bump pass (see the file's own `aiAndGenericJobOpsSupported` doc comment) that had
already applied this campaign's time-filter lesson before parity-16 even discovered the bug
elsewhere. `jobs.go`'s pre-existing comments already cite `deserializers.go`/doc text directly. This
pass only converted the 7 anonymous structs to named types (`createJobInput`, `describeJobInput`,
`deleteJobInput`, `stopJobInput`, `listJobsInput`, `describeJobSchemaVersionInput`,
`listJobSchemaVersionsInput`) for tooling visibility — zero field changes, zero new bugs found. This
is the campaign's first pass to convert a fully-correct family without finding anything: recorded
here as a control data point, not an oversight.

**`handler_model_cards.go` (ModelCard family) — the largest gap found this pass.** Full detail:

- `CreateModelCardInput` (`api_op_CreateModelCard.go:32-66`) — was missing **required**
  `ModelCardStatus` entirely (the handler hardcoded every card to `"Draft"`, silently discarding
  whatever status a real client sent) and optional `SecurityConfig` (`types.ModelCardSecurityConfig`,
  one member, `KmsKeyId`). Fixed: `ModelCardStatus` is now read, validated against
  `types.ModelCardStatus`'s real values (`Draft`/`PendingReview`/`Approved`/`Archived`,
  `types/enums.go:5910-5913`), and stored; `SecurityConfig` is stored and echoed back on Describe.
- `DescribeModelCardInput` (`api_op_DescribeModelCard.go:36-59`) — was missing optional
  `ModelCardVersion` and `IncludedData` (`types.IncludedData`: `AllData`/`MetadataOnly`,
  `types/enums.go:4314-4315`). `ModelCardVersion` now honors the current (only tracked) version and
  returns `ResourceNotFound` for any other, matching parity-15's `ClusterSchedulerConfigVersion`
  precedent. `IncludedData=MetadataOnly` now genuinely sanitizes `Content` down to the five JSON
  paths the op's own doc comment lists (`model_overview.model_id`/`model_overview.model_name`/
  `intended_uses.risk_rating`/`model_package_details.model_package_group_name`/
  `model_package_details.model_package_arn`) rather than accepting the parameter and doing nothing —
  a real client requesting `MetadataOnly` previously received the full, un-redacted `Content` back,
  including whatever sensitive fields it contained.
- `UpdateModelCardInput` (`api_op_UpdateModelCard.go:31-59`) — was missing `ModelCardStatus`
  entirely, so no real client could ever approve/archive a model card through this emulator. Its doc
  comment states "You cannot update both model card content and model card status in a single
  call" — enforced now (mutual-exclusion `ValidationException`). A status-only update does not bump
  `ModelCardVersion` (real versioning tracks content revisions, not approval-workflow transitions);
  a content update still increments it, matching pre-existing behavior.
- `ListModelCardsInput` (`api_op_ListModelCards.go:30-59`) — was missing all 8 optional fields, only
  `NextToken` existed: `CreationTimeAfter`, `CreationTimeBefore`, `MaxResults`, `ModelCardStatus`,
  `NameContains`, `SortBy`, `SortOrder`. All seven now real via `ListModelCardsParams`/
  `matchesModelCardListParams`/`sortModelCardsByParams`. Neither `SortBy` nor `SortOrder` has a
  documented default; kept as the disclosed `CreationTime`/Ascending fallback, this campaign's
  recurring precedent for undocumented cases. **A fabricated response field found and removed:**
  the pre-existing summary map included a `"ModelCardVersion"` key — `ModelCardSummary`
  (`types/types.go`) has no such member at all; only its sibling `ModelCardVersionSummary` does.
  Every real client's `ListModelCards` call previously received an extra field with no wire
  counterpart.
- `ListModelCardVersionsInput` (`api_op_ListModelCardVersions.go:30-59`) — was missing all 7 optional
  filters beyond `ModelCardName`. This backend keeps no historical per-version snapshot (only the
  card's current state), so the op still returns at most one synthetic entry, but
  `ModelCardStatus`/`CreationTimeAfter`/`CreationTimeBefore` now genuinely filter that single entry
  in or out rather than being silently ignored; `SortBy`/`SortOrder`/`MaxResults` are disclosed
  no-ops for the same one-entry-max reason.
- `ListModelCardExportJobsInput` (`api_op_ListModelCardExportJobs.go:29-61`) — was missing
  `CreationTimeAfter`, `CreationTimeBefore`, `ModelCardVersion`, `SortBy`, `SortOrder` (only
  `ModelCardName`/`ModelCardExportJobNameContains`/`StatusEquals`/`NextToken`/`MaxResults` existed).
  All five now real via a new `ListModelCardExportJobsParams`/`matchesModelCardExportJobListParams`/
  `modelCardExportJobSortLess` in `modelcard_export.go` (the sibling file backing this op, not itself
  one of this pass's four target files, but touched as a necessary consequence). `SortBy` defaults to
  `CreationTime` (documented); `SortOrder` has no documented default, kept as Ascending.

**Existing test found asserting a wrong shape:** `TestHandler_ListModelCards` asserted
`assert.EqualValues(t, 1, s["ModelCardVersion"])` against the fabricated field above — the same class
of finding as parity-12/15's enshrined-bug tests. Updated to assert the field's absence instead.

**`handler_monitoring_schedules.go` (MonitoringSchedule family) — the most severe gap found this
pass.** `CreateMonitoringScheduleInput.MonitoringScheduleConfig` (`api_op_CreateMonitoringSchedule.go:29-50`)
is **required** and was never read at all — every real client's monitoring job definition, cron
schedule expression, and endpoint association was silently discarded, and the resulting
`MonitoringSchedule` carried none of it. `UpdateMonitoringScheduleInput` had the identical gap
(`api_op_UpdateMonitoringSchedule.go:28-45`). Both now require the field (`ValidationException` if
absent — the real client's own `validateOpCreateMonitoringScheduleInput`/
`validateOpUpdateMonitoringScheduleInput` enforce non-nil-ness the same way, confirmed by reading
`validators.go` directly; neither validates `MonitoringJobDefinition`/`MonitoringJobDefinitionName`
as a required oneof beyond that, so this backend doesn't either — an intentionally-empty
`&smtypes.MonitoringScheduleConfig{}`, as the pre-existing `TestCreateOpsWithTags_RoundTrip` sends,
still succeeds). `MonitoringScheduleConfig.MonitoringJobDefinition` (`types.MonitoringJobDefinition`)
is deeply nested (`MonitoringAppSpecification`/`MonitoringInputs`/`MonitoringOutputConfig`/
`MonitoringResources`/`NetworkConfig`/...) and is carried as opaque `json.RawMessage` passthrough per
this campaign's established convention for such types — stored and echoed back verbatim on
Describe/List, never simulated. `ScheduleConfig` (3 flat strings) is modeled directly. `EndpointName`
(optional on both `DescribeMonitoringScheduleOutput` and `ListMonitoringSchedulesInput`) is
best-effort derived from `MonitoringInputs[0].EndpointInput.EndpointName` inside the stored raw
document — the only place this backend ever parses that document's contents.

`ListMonitoringSchedulesInput` (`api_op_ListMonitoringSchedules.go:29-72`) was missing all 11
optional fields beyond `NextToken`: `CreationTimeAfter`, `CreationTimeBefore`, `EndpointName`,
`LastModifiedTimeAfter`, `LastModifiedTimeBefore`, `MaxResults`, `MonitoringJobDefinitionName`,
`MonitoringTypeEquals`, `NameContains`, `SortBy`, `SortOrder`, `StatusEquals`. All now real via
`ListMonitoringSchedulesParams`/`matchesMonitoringScheduleNameParams`/
`matchesMonitoringScheduleTimeParams`/`sortMonitoringSchedulesByParams`. **Both `SortBy`
("`CreationTime` by default") and `SortOrder` ("`Descending` by default") are documented in this
op's own doc comment** — implemented as documented, this campaign's first op with both defaults
explicitly stated. **A doc/enum mismatch found:** the doc prose says results can be sorted "by the
`Status`, `CreationTime`, or `ScheduledTime` field", but the real `MonitoringScheduleSortKey` enum
(`types/enums.go:6363-6365`) only declares `Name`/`CreationTime`/`Status` — no `ScheduledTime` value
exists at all. Implemented per the enum, not the prose, per this campaign's standing rule that "a doc
comment is prose about the API; the enum block is the API."

**`handler_inference_experiments.go` (InferenceExperiment family) — the largest field gap and two
discarded-response-body bugs found this pass.**

- `CreateInferenceExperimentInput` (`api_op_CreateInferenceExperiment.go:29-90`) was missing all
  three of its non-`Name`/`Type` **required** fields: `EndpointName`, `ModelVariants`
  (`[]types.ModelVariantConfig`), `ShadowModeConfig`. No real client could previously create an
  inference experiment that referenced an actual endpoint, defined actual model variants, or
  configured an actual shadow split — the emulator accepted the call and silently produced an
  experiment with none of that. Also missing: optional `DataStorageConfig`, `KmsKey`, `Schedule`.
  All six now modeled with real Go types (`ModelVariantConfig`/`ModelInfrastructureConfig`/
  `RealTimeInferenceConfig`/`ShadowModeConfig`/`ShadowModelVariantConfig`/
  `InferenceExperimentDataStorageConfig`/`CaptureContentTypeHeader`/`InferenceExperimentSchedule`) —
  all flat/shallow enough not to need `json.RawMessage` passthrough.
- **`DescribeInferenceExperimentOutput.EndpointMetadata` and `.ModelVariants` are both required**
  (`api_op_DescribeInferenceExperiment.go:32-46`) and were both entirely absent. `EndpointMetadata`
  is now freshly computed from the live `Endpoint` store at Describe time (never persisted, so it
  can never go stale). **`ModelVariants` uses the same wire key on request and response with two
  different shapes**: the request-side `types.ModelVariantConfig` has no `Status`; the response-side
  `types.ModelVariantConfigSummary` requires one. This backend stores the request shape
  (`InferenceExperiment.ModelVariants`, tagged with the persistence-only key `ModelVariantConfigs` to
  avoid colliding with the real wire name) and projects it into the response shape at marshal time,
  synthesizing `Status: "InService"` (`types.ModelVariantStatusInService`) since there is no
  per-variant deployment FSM.
- **`StopInferenceExperimentOutput.InferenceExperimentArn` and
  `DeleteInferenceExperimentOutput.InferenceExperimentArn` are both required**
  (`api_op_StopInferenceExperiment.go:59-65`, `api_op_DeleteInferenceExperiment.go:36-42`) and were
  both **entirely discarded** — `dispatchMlopsOps` routed both ops through `return nil, true, err`,
  so every real client's `StopInferenceExperiment`/`DeleteInferenceExperiment` call received an empty
  response body where a required ARN belonged. Fixed: both handlers now return the ARN; `Delete`'s
  backend method changed from `error` to `(string, error)`.
- `StopInferenceExperimentInput.ModelVariantActions` (`map[string]types.ModelVariantAction`) is
  **required** and was entirely absent — no real client's Promote/Remove/Retain instruction was ever
  applied. Fixed: `Promote` keeps only that variant, `Remove` drops it, `Retain` is a no-op, applied
  to the stored variant list. `DesiredModelVariants` (optional) replaces the list outright when
  supplied. `DesiredState`/`Reason` (both optional) now set the resulting `Status`/`StatusReason`
  instead of always hardcoding `"Cancelled"`.
- `UpdateInferenceExperimentInput` was missing `DataStorageConfig`/`ModelVariants`/`Schedule`/
  `ShadowModeConfig` entirely (only `Description` was ever applied). All four now real.
- `ListInferenceExperimentsInput` (`api_op_ListInferenceExperiments.go:29-59`) was missing all 10
  optional fields, only `NextToken` existed. All ten now real via `ListInferenceExperimentsParams`/
  `matchesInferenceExperimentListParams`/`matchesInferenceExperimentTimeParams`/
  `sortInferenceExperimentsByParams`. Neither `SortBy` nor `SortOrder` is documented; kept as the
  disclosed `CreationTime`/Ascending fallback. Also fixed: the summary map's `Type` field (required
  by `InferenceExperimentSummary`) was previously emitted conditionally (`if e.Type != ""`) instead
  of unconditionally — harmless in practice since `Type` is required at Create and therefore never
  actually empty, but corrected to always emit per the real required-ness.

**Disclosed, not modeled:**

- `DescribeModelCardOutput.CreatedBy`/`LastModifiedBy` (`types.UserContext`) — this service models no
  caller-identity concept anywhere, the same gap already disclosed repeatedly in prior passes.
- `DescribeModelCardOutput.ModelCardProcessingStatus` — only ever populated during an in-progress
  deletion; `DeleteModelCard` is synchronous in this backend, so there is never an observable
  in-progress deletion state.
- `ListDevicesInput`-class no-ops carried over unchanged from prior passes are not re-listed here.
- `MonitoringScheduleConfig.MonitoringJobDefinitionName`/`MonitoringType` are modeled (flat fields,
  used for `ListMonitoringSchedules` filtering) but this backend never actually runs a monitoring
  job — no periodic execution, no `MonitoringExecutionSummary`, no `FailureReason` — consistent with
  `DescribeMonitoringScheduleOutput.LastMonitoringExecutionSummary`/`FailureReason` never being
  populated.
- `CreateInferenceExperimentInput.DataStorageConfig`/`KmsKey`/`Schedule` are stored and echoed back
  verbatim but inert: no data-capture or scheduling simulation exists in this backend.
- `InferenceExperiment.CompletionTime` — `Status` never reaches `"Completed"` in this backend (no
  experiment-duration FSM), so there is never a real completion timestamp to report; the field is
  never populated, consistent with disclosing rather than fabricating one.

**Storage-key check:** `ModelCard` (keyed by `ModelCardName`), `MonitoringSchedule` (keyed by
`MonitoringScheduleName`), and `InferenceExperiment` (keyed by `Name`) all stayed keyed the same;
none of this pass's new fields changed any table's key shape.

**Enums touched, all read from the constants:** `ModelCardStatus`, `ModelCardSortBy`,
`ModelCardVersionSortBy`, `ModelCardExportJobSortBy`, `IncludedData` (ModelCard family);
`ScheduleStatus`, `MonitoringScheduleSortKey`, `MonitoringType` (MonitoringSchedule family, the
`ScheduledTime` doc/enum mismatch noted above); `InferenceExperimentStatus`,
`InferenceExperimentStopDesiredState`, `ModelVariantAction`, `ModelVariantStatus`,
`SortInferenceExperimentsBy` (InferenceExperiment family) — all confirmed mixed-case, none assumed
by analogy to a sibling.

**Tests:** real-`aws-sdk-go-v2`-client round-trip tests added —
`TestHandler_CreateModelCard_Status_SecurityConfig_RealClient`,
`TestHandler_UpdateModelCard_StatusOnly_RealClient`,
`TestHandler_DescribeModelCard_MetadataOnly_RealClient`,
`TestHandler_DescribeModelCard_Version_RealClient`,
`TestHandler_ListModelCards_FilterSortPage_RealClient`,
`TestHandler_ListModelCardExportJobs_Version_RealClient`,
`TestHandler_MonitoringScheduleConfig_RealClient`,
`TestHandler_UpdateMonitoringSchedule_ConfigRequired_RealClient`,
`TestHandler_ListMonitoringSchedules_FilterSortPage_RealClient`,
`TestHandler_CreateInferenceExperiment_FullFields_RealClient`,
`TestHandler_StopInferenceExperiment_Arn_RealClient`,
`TestHandler_DeleteInferenceExperiment_Arn_RealClient`,
`TestHandler_ListInferenceExperiments_FilterSortPage_RealClient`. Pre-existing raw-HTTP tests for all
three now-required-config ops (`CreateModelCard`/`CreateMonitoringSchedule`/
`CreateInferenceExperiment`) updated to supply the newly-required fields; new
`_RequiredFieldsEnforced`/`_ConfigRequired` tests added asserting the previously-silent gaps now
reject. Verified against unfixed code by hand-reverting one representative fix per file (eight
total) one at a time: `ModelCard`'s `CreateModelCard` ignoring `opts.Status` (confirmed
`TestHandler_CreateModelCard_Status_SecurityConfig_RealClient` failed with `"Draft"` instead of
`"Approved"`); the fabricated `ModelCardVersion` summary field re-added (confirmed
`TestHandler_ListModelCards` failed asserting its absence); `MetadataOnly` sanitization
short-circuited (confirmed the full, un-redacted `Content` including the secret `training_details`
field was returned); `CreateMonitoringSchedule`'s config-required check disabled (confirmed a nil
`MonitoringScheduleConfig` panicked with a nil-pointer dereference rather than a clean 400 — a worse
symptom than predicted, and confirming the check's necessity all the more); `EndpointName`
derivation short-circuited to always `""` (confirmed); `ListMonitoringSchedules`' documented
Descending default flipped to Ascending (confirmed the wrong sort order); `CreateInferenceExperiment`'s
`ShadowModeConfig`-required check disabled (confirmed only that one subtest of
`TestHandler_CreateInferenceExperiment_RequiredFieldsEnforced` started passing incorrectly); and
`applyModelVariantActions`' `Promote` handling disabled (confirmed both variants survived instead of
only the promoted one) — each restored and `md5sum`-verified byte-identical to its pre-revert state
afterward.

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` (0 issues) all
clean; `go build ./...` (repo-wide) also clean. Two `//nolint:gochecknoglobals` added
(`modelCardStatusValues`/`modelCardMetadataOnlyPaths` lookup tables), matching this repo's
pre-existing precedent (`automl_v2.go`/`automl_search.go`/`persistence.go`/`training_plan.go` all
carry the identical justification) — no cyclop/gocyclo/gocognit/funlen suppressions, which remain
banned. Also fixed real findings by editing, not suppressing: `fieldalignment -fix` (six structs),
`golines -m 120 -w` (two files), a `cyclop` finding in `matchesMonitoringScheduleListParams` fixed by
decomposing it into `matchesMonitoringScheduleNameParams`/`matchesMonitoringScheduleTimeParams`
rather than suppressed, a `revive var-naming` rename (`JsonContentTypes` field → `JSONContentTypes`,
JSON tag unchanged), and two `govet shadow` findings in test files (renamed shadowing `err`
variables).

**`last_audit_commit` left at its existing value (`5f91d37c7`)** — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**180 of sagemaker's 362 inline structs now remain.** Next by size (tied at 6):
`handler_trial_components.go`, `handler_training_plan.go`, `handler_partner_apps.go`,
`handler_labeling.go`, `handler_inference_components.go`, `handler_endpoints.go`.

## parity-18 (2026-08-21, gopherstack-oc9v): TrialComponent/TrainingPlan/PartnerApp inline-struct
sweep (partial — 3 of the 6 tied-at-6 files)

Twelfth pass of the gopherstack-oc9v campaign. Per parity-17's boundary note, this pass took the
first three of the six files tied at 6 (`handler_trial_components.go`, `handler_training_plan.go`,
`handler_partner_apps.go`), each verified by `grep -c 'var req struct {' <file>.go` = 6 before
starting. **`handler_labeling.go`, `handler_inference_components.go`, and `handler_endpoints.go` were
not started this pass** — full files were taken with full rigor rather than spreading thin across
all six. All 18 of this pass's structs were converted to named types and wire-audited field-by-field
against the pinned SDK (`v1.263.2`, confirmed from `go.mod`, matching prior passes). **162 of
sagemaker's 362 inline structs now remain** (362 − 19 − 19 − 15 − 14 − 14 − 12 − 11 − 11 − 10 − 9 − 9
− 7 − 7 − 7 − 7 − 6 − 6 − 6), confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go`
summed, not arithmetic; all three files now have zero.

**`handler_trial_components.go` (TrialComponent family):**

- `CreateTrialComponentInput` (`api_op_CreateTrialComponent.go:44-91`) was missing optional
  `MetadataProperties` (`types.MetadataProperties`) entirely. Now decoded directly into this
  service's existing shared `MetadataProperties` type (`lineage.go:56-61`, whose JSON tags already
  match the wire shape byte-for-byte — reused rather than re-declaring a second wire-shape struct).
- `DescribeTrialComponentOutput` (`api_op_DescribeTrialComponent.go:36-83`) was missing
  `LineageGroupArn` entirely. SageMaker has no `CreateLineageGroup` op (confirmed absent from the
  pinned SDK) — every account has exactly one auto-provisioned default lineage group
  (`lineage.go`'s `defaultLineageGroupName`), and every trial component belongs to it. Fixed via a
  new `trialComponentLineageGroupArn` backend method mirroring the existing
  `DescribeLineageGroup`/`ListLineageGroups` ARN construction. `MetadataProperties` is now echoed
  back when set.
- `ListTrialComponentsInput` (`api_op_ListTrialComponents.go:30-71`) was missing **6 of 9** optional
  fields, only `ExperimentName`/`TrialName`/`NextToken` existed: `CreatedAfter`, `CreatedBefore`,
  `MaxResults`, `SortBy`, `SortOrder`, `SourceArn`. The first five are now real via a new
  `ListTrialComponentsParams`/sort-by-params rewrite of the backend method (previously a bare
  3-argument function with no filter/sort/page-size support at all). **This op's own doc states both
  real defaults explicitly**: `SortBy` defaults to `CreationTime`, `SortOrder` to `Descending` —
  implemented as documented. `SourceArn` is decoded for wire-shape fidelity but is a disclosed
  no-op: `CreateTrialComponentInput` has no `Source` field at all (a trial component's
  `TrialComponentSource` is only ever populated when SageMaker auto-tracks a processing/training job,
  which this backend never does), so no trial component ever has a source ARN to filter by.
  `SortTrialComponentsBy`'s real values are `Name`/`CreationTime` (`types/enums.go:9345-9346`,
  mixed-case) — read from the enum constants, not assumed.
- `UpdateTrialComponentInput` (`api_op_UpdateTrialComponent.go:28-70`) was missing
  `InputArtifactsToRemove`/`OutputArtifactsToRemove`/`ParametersToRemove` entirely — a real client
  could add or replace parameters/artifacts but never remove one. Fixed: applied additive-then-remove,
  matching this file's sibling lineage handlers' (`UpdateAction`/`UpdateArtifact`/`UpdateContext`)
  existing `PropertiesToRemove` pattern in `lineage.go`.
- `DeleteTrialComponentInput`/`DisassociateTrialComponentInput` — already matched exactly; converted
  to named types for tooling visibility with no field changes.

**Disclosed, not modeled (TrialComponent):** `DescribeTrialComponentOutput`'s `CreatedBy`/
`LastModifiedBy` (`types.UserContext`, no caller-identity concept anywhere in this service, the same
gap disclosed repeatedly in prior passes); `Metrics` (`[]types.TrialComponentMetricSummary`,
populated only by the separate `sagemaker-metrics` service's `BatchPutMetrics`, not implemented
here); `Source`/`Sources` (`types.TrialComponentSource`, see `SourceArn` above).

**`handler_training_plan.go` (TrainingPlan/ReservedCapacity extras) — a repo-spanning response-side
epoch-time bug, the same class this campaign has repeatedly found on the request-decode side, found
here for the first time on the response-encode side:**

`trainingPlanSummaryJSON` (the `ListTrainingPlans` summary builder) assigned `t.StartTime`/`t.EndTime`
— both `*time.Time` — directly into the response `map[string]any`, bypassing `TrainingPlan`'s own
correct `MarshalJSON` override (`training_plans.go:112-126`, which the sibling `DescribeTrainingPlan`
handler uses safely via `json.Marshal(result)`). A bare map has no such override, so `encoding/json`
fell back to Go's default `time.Time` marshaling — an RFC3339 **string** — for a field the real
`ListTrainingPlansOutput` (`api_op_ListTrainingPlans.go`) declares as an awsjson1.1 **number**.
**Any real client's `ListTrainingPlans` call against a plan with a `StartTime`/`EndTime` set (i.e. any
purchased plan) failed outright** with `deserialization failed ... expected Timestamp to be a JSON
Number, got string instead` — the same failure signature parity-16 found across five List-filter ops,
here on a List *response* field instead. Fixed by converting through `epochSeconds`/`epochSeconds(*...)`
before assignment; confirmed via hand-revert below.

- `ListTrainingPlansInput` (`api_op_ListTrainingPlans.go:30-71`) was also missing `StartTimeAfter`/
  `StartTimeBefore` entirely (now `*float64`/`timeFromEpochSecondsPtr`, threaded into a new
  `ListTrainingPlansParams.StartTimeAfter`/`StartTimeBefore` and applied against
  `trainingPlanStartTime(t)`, the existing StartTime-or-CreationTime fallback the sort logic already
  used).
- `TrainingPlanSummary` (`types/types.go:22825-22903`) was missing **5 of 15** members in the List
  summary even though `CreateTrainingPlan` already populates the underlying data:
  `StatusMessage`, `TargetResources`, `TotalInstanceCount`, `UpfrontFee` (all pre-existing `TrainingPlan`
  fields tagged `json:"-"` since `DescribeTrainingPlan` doesn't need a hand-built map — but
  `trainingPlanSummaryJSON` builds one manually and can read them as ordinary Go fields), and
  `TotalUltraServerCount` (not tracked directly; computed by counting `ReservedCapacitySummaries` with
  `ReservedCapacityType == "UltraServer"`, disclosed as relying on this catalog's one-UltraServer-per-
  offering design in `trainingPlanTotalUltraServerCount`'s doc comment).
  `DescribeTrainingPlanOutput`'s identical `UpfrontFee`/`TargetResources`/`TotalInstanceCount` gap
  (`handler_training_plans.go`, a sibling file not in this pass's scope) was found but not fixed —
  flagged for follow-up, see Notes.
- `SearchTrainingPlanOfferingsInput` (`api_op_SearchTrainingPlanOfferings.go:26-71`) had two absent-
  effect bugs, not just absent-decode ones: **`InstanceCount` was already decoded and threaded through
  to `SearchTrainingPlanOfferingsParams` but the matching loop never checked it at all** — a real
  client's instance-count requirement was silently ignored regardless of value. **`UltraServerCount`
  was worse: present on `SearchTrainingPlanOfferingsParams` (a struct field nobody ever wrote to) but
  never even decoded by the handler.** Both are now real filters
  (`offeringMatchesInstanceCount`/`offeringMatchesUltraServerCount` in `training_plan.go`); the latter
  is disclosed as only ever satisfiable at 0 or 1 given this catalog's one-UltraServer-per-offering
  design. `StartTimeAfter`/`EndTimeBefore` are decoded for wire-shape fidelity but disclosed no-ops:
  the static catalog's entries have no absolute start/end time of their own (only a relative
  duration) until purchased into a `TrainingPlan`/`ReservedCapacity`.
- `DescribeReservedCapacityOutput` (`api_op_DescribeReservedCapacity.go`) was missing
  `UltraServerSummary` entirely — `ReservedCapacity.UltraServers` was tracked internally
  (`json:"-"`, used only by `ListUltraServersByReservedCapacity`) but never projected into the
  Describe response. Fixed via a new `ultraServerSummary()` method on `*ReservedCapacity`, wired into
  its existing `MarshalJSON` override; `UnhealthyInstanceCount` is always 0, disclosed as this
  backend never simulating an unhealthy UltraServer.
- `ExtendTrainingPlanInput`/`DescribeTrainingPlanExtensionHistoryInput`/`DescribeReservedCapacityInput`/
  `ListUltraServersByReservedCapacityInput` — already matched exactly; converted to named types for
  tooling visibility with no field changes.

Also added `sagemakerListKeyPagedN` (`list_helpers.go`), a maxResults-aware sibling of the
pre-existing `sagemakerListKeyPaged` (14 other call sites left untouched — this pass's `ListPartnerApps`
is its only caller), since `ListPartnerAppsInput.MaxResults` needed threading through and changing the
shared helper's signature would have touched 14 unrelated ops out of scope.

**Disclosed, not modeled (TrainingPlan):** `ListClustersInput.TrainingPlanArn`-class gaps are
pre-existing and unrelated; `ListTrainingPlansInput`/`SearchTrainingPlanOfferingsInput`'s no-op
filters above.

**`handler_partner_apps.go` (PartnerApp family) — the largest field gap found this pass, across
every op:**

- `CreatePartnerAppInput` (`api_op_CreatePartnerApp.go`) declares `AuthType`/`ExecutionRoleArn`/
  `Name`/`Tier`/`Type` all **required**, but only `Name` was ever validated — a real client's
  hand-crafted (SDK-bypassing) request missing any of the other four previously succeeded anyway.
  Now all five are enforced. Also missing entirely: `EnableAutoMinorVersionUpgrade`,
  `EnableIamSessionBasedIdentity`, `KmsKeyId`, `MaintenanceConfig` (`types.PartnerAppMaintenanceConfig`,
  a single-field struct modeled directly rather than as `json.RawMessage`). `ClientToken` is
  deliberately omitted, matching this service's repo-wide convention (see `CreateModelPackageOptions`)
  that a pure client-side idempotency token has no server-observable effect.
- `DescribePartnerAppOutput` (`api_op_DescribePartnerApp.go:36-125`) was missing **9 of 19** members:
  `AvailableUpgrade`, `BaseUrl`, `CurrentVersionEolDate`, `EnableAutoMinorVersionUpgrade`,
  `EnableIamSessionBasedIdentity`, `Error`, `KmsKeyId`, `MaintenanceConfig`, `Version`. The four
  simple stored fields are now echoed back. `BaseUrl` is synthesized (mirroring
  `CreatePartnerAppPresignedURL`'s own synthesized host — this backend has no real partner-app-hosting
  infrastructure to derive one from). `AvailableUpgrade`/`CurrentVersionEolDate`/`Version` are
  disclosed, not modeled: this backend tracks no minor-version-upgrade catalog at all. `Error`
  (`types.ErrorInfo`) is disclosed: `Status` never reaches `Failed`/`UpdateFailed` in this backend, so
  there is never a failure to describe. `DescribePartnerAppInput.IncludeAvailableUpgrade` is decoded
  for wire-shape fidelity but is a disclosed no-op for the same reason.
- `UpdatePartnerAppInput` (`api_op_UpdatePartnerApp.go:24-71`) was missing **`Tags` — a real,
  observable bug**: `PartnerApp` is already tag-lookup-registered (`tags.go`'s
  `statefulTagLookupsPart2`), so `ListTagsForResource`/`AddTags` already worked against it, but a
  client updating tags through `UpdatePartnerApp` itself (the field exists on the real input) had
  them silently discarded. Fixed: threaded through and merged via the existing `mergeTags` helper.
  Also missing: `EnableAutoMinorVersionUpgrade`, `EnableIamSessionBasedIdentity`, `MaintenanceConfig`
  (all now real). `AppVersion` is decoded for wire-shape fidelity but is a disclosed no-op — no
  version state exists to advance, consistent with `DescribePartnerAppOutput.Version`'s disclosure
  above.
- `ListPartnerAppsInput` (`api_op_ListPartnerApps.go:24-38`) was missing `MaxResults` entirely — the
  backend's `ListPartnerApps` had no page-size parameter at all before this pass (see
  `sagemakerListKeyPagedN` above).
- `CreatePartnerAppPresignedUrlInput` (`api_op_CreatePartnerAppPresignedUrl.go:24-38`) was missing
  `ExpiresInSeconds`/`SessionExpirationDurationInSeconds`. Both are decoded for wire-shape fidelity
  but disclosed no-ops: the response is a bare `{Url}`, and this backend's already-synthetic
  presigned URL carries no verified real query-parameter format to encode an expiry into.
- `DeletePartnerAppInput` — already matched exactly (`ClientToken` omitted per the same convention as
  Create/Update); converted to a named type for tooling visibility.

**An existing test used an invalid enum value, found but not treated as a wrong-shape assertion**:
the pre-existing `TestHandler_CreatePartnerApp` sent `"Type": "custom"` — `PartnerAppType`
(`types/enums.go:6905-6912`) has exactly four real values (`lakera-guard`/`comet`/
`deepchecks-llm-evaluation`/`fiddler`), none of which is `"custom"`. This wasn't ratifying a defect
(the handler never validated `Type` against the enum, before or after this pass — matching this
service's convention of not whitelisting every open string field), but the now-added required-field
validation would have made the test's other omissions fail regardless; fixed to send a real value
and all five required fields, alongside three sibling tests with the same gap
(`TestHandler_DescribePartnerApp`/`DeletePartnerApp`/`DeletePartnerApp_ReturnsArn`).

**Enums touched, all read from the constants:** `SortTrialComponentsBy` (TrialComponent family);
`TrainingPlanSortBy`/`TrainingPlanFilterName` (confirmed `TrainingPlanFilterName` has exactly one
real value, `Status` — the pre-existing status-only filter was already complete, not a gap),
`ReservedCapacityType`/`ReservedCapacityStatus` (confirmed the pre-existing `"UltraServer"`/
`"Instance"`/`"Active"` literals already matched real enum values) (TrainingPlan family);
`PartnerAppAuthType` (confirmed exactly one real value, `IAM`), `PartnerAppType` (PartnerApp family,
see the invalid-test-value finding above).

**Storage-key check:** `TrialComponent` (keyed by `TrialComponentName`), `TrainingPlan` (keyed by
`TrainingPlanName`), `ReservedCapacity`/`PartnerApp` (both keyed by ARN) all stayed keyed the same;
no new field changed any table's key shape.

**Tests:** real-`aws-sdk-go-v2`-client round-trip tests added —
`TestHandler_CreateTrialComponent_MetadataProperties_RealClient`,
`TestHandler_ListTrialComponents_FilterSortPage_RealClient`,
`TestHandler_UpdateTrialComponent_RemoveLists_RealClient`,
`TestHandler_DescribeReservedCapacity_UltraServerSummary_RealClient`,
`TestHandler_SearchTrainingPlanOfferings_InstanceUltraServerCount_RealClient`,
`TestHandler_ListTrainingPlans_StartTimeFilter_RealClient`,
`TestHandler_ListTrainingPlans_SummaryFields_RealClient`,
`TestHandler_CreatePartnerApp_FullFields_RealClient`, `TestHandler_UpdatePartnerApp_Tags_RealClient`,
`TestHandler_ListPartnerApps_MaxResults_RealClient`; plus a raw-HTTP
`TestHandler_CreatePartnerApp_RequiredFieldsEnforced` (table-driven over the four newly-required
fields). Four pre-existing raw-HTTP tests updated to supply `CreatePartnerApp`'s newly-required
fields (see above). Verified against unfixed code by hand-reverting one representative fix per file
(five total) one at a time: `ListTrialComponents`' `ParametersToRemove` handling (confirmed
`TestHandler_UpdateTrialComponent_RemoveLists_RealClient` failed asserting the removed key's
absence); `CreateTrialComponent`'s `MetadataProperties` storage (confirmed
`TestHandler_CreateTrialComponent_MetadataProperties_RealClient` failed on a nil field);
`trainingPlanSummaryJSON`'s `epochSeconds` conversion (confirmed
`TestHandler_ListTrainingPlans_SummaryFields_RealClient` failed with the exact predicted
`expected Timestamp to be a JSON Number, got string instead` deserialization error);
`ReservedCapacity.ultraServerSummary`'s early return (confirmed
`TestHandler_DescribeReservedCapacity_UltraServerSummary_RealClient` failed on a nil field);
`UpdatePartnerApp`'s `Tags` merge (confirmed `TestHandler_UpdatePartnerApp_Tags_RealClient` failed
with zero tags); and `CreatePartnerApp`'s four required-field checks (confirmed all four
`TestHandler_CreatePartnerApp_RequiredFieldsEnforced` subtests failed) — each restored and
`md5sum`-verified byte-identical to its pre-revert state afterward.

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker` (empty), `go test -race ./services/sagemaker/...`, `go fix -diff
./services/sagemaker/...` (no diff), and `golangci-lint run ./services/sagemaker/...` (0 issues) all
clean; `go build ./...` (repo-wide) also clean. Zero new `nolint` added. Fixed real findings by
editing, not suppressing: `fieldalignment -fix` (multiple structs across
`handler_partner_apps.go`/`partner_apps.go`/`handler_training_plan.go`/`training_plan.go`/
`handler_trial_components.go`/`trial_components.go`), and two `govet shadow` findings in
`handler_training_plan_test.go` (renamed shadowing `err` variables to `searchErr`/`createErr`).

**`last_audit_commit` left at its existing value (`5f91d37c7`)** — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**162 of sagemaker's 362 inline structs now remain.** `handler_labeling.go`,
`handler_inference_components.go`, and `handler_endpoints.go` — all three still tied at 6, carried
over unstarted from parity-17's boundary note — are next.

Filed follow-up: `handler_training_plans.go`'s `DescribeTrainingPlanOutput` shares
`trainingPlanSummaryJSON`'s absent `UpfrontFee`/`TargetResources`/`TotalInstanceCount` gap (see
above) but is a sibling file outside this pass's scope; not fixed here.

## parity-19 (2026-08-21, gopherstack-oc9v): LabelingJob/InferenceComponent/Endpoint
inline-struct sweep (all three remaining tied-at-6 files)

Thirteenth pass of the gopherstack-oc9v campaign. Per parity-18's boundary note, this pass took the
final three files tied at 6 (`handler_labeling.go`, `handler_inference_components.go`,
`handler_endpoints.go`), each verified by `grep -c 'var req struct {' <file>.go` = 6 before starting.
All 18 of this pass's structs were converted to named types and wire-audited field-by-field against
the pinned SDK (`v1.263.2`, confirmed from `go.mod`, matching prior passes). **144 of sagemaker's 362
inline structs now remain** (362 − 19 − 19 − 15 − 14 − 14 − 12 − 11 − 11 − 10 − 9 − 9 − 7 − 7 − 7 − 7
− 6 − 6 − 6 − 6 − 6 − 6), confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go` summed,
not arithmetic; all three files now have zero. **New boundary: 8 files tied at 5**
(`handler_workteams.go`, `handler_workforces.go`, `handler_trials.go`, `handler_training_jobs.go`,
`handler_projects.go`, `handler_optimization_jobs.go`, `handler_inference_recommendations_jobs.go`,
`handler_hp_tuning_jobs.go`).

**`handler_labeling.go` (LabelingJob/SubscribedWorkteam family):**

- `CreateLabelingJobInput` (`api_op_CreateLabelingJob.go:68-116`) had every field decoded already
  except that **`LabelAttributeName` is documented `This member is required` but was never
  validated** — a real client's request missing it previously succeeded silently. Fixed: now
  enforced like the other four required members.
- `ListLabelingJobsInput` (`api_op_ListLabelingJobs.go:30-71`) was missing **6 of 10** fields: only
  `NextToken`/`NameContains`/`StatusEquals`/`MaxResults` existed. `CreationTimeAfter`/
  `CreationTimeBefore`/`LastModifiedTimeAfter`/`LastModifiedTimeBefore` are now `*float64`/
  `timeFromEpochSecondsPtr`; `SortBy`/`SortOrder` are real (this op's own doc states both defaults
  explicitly: `CreationTime`/Ascending — implemented as documented, **not** generalized from
  sibling ops, several of which default to Descending instead). `LabelingJobSummary`
  (`types/types.go:13460-13520`) was also missing **2 of 12** response members —
  `InputConfig`/`LabelingJobOutput` — despite the backend already storing both; now echoed.
- `ListLabelingJobsForWorkteamInput` (`api_op_ListLabelingJobsForWorkteam.go:30-64`) was missing
  **4 of 7** optional fields: `CreationTimeAfter`/`CreationTimeBefore`/`JobReferenceCodeContains`/
  `SortBy`. `SortBy`'s real enum (`ListLabelingJobsForWorkteamSortByOptions`,
  `types/enums.go:5379-5383`) has exactly **one** value, `CreationTime` — decoded for wire-shape
  fidelity but disclosed as a no-op, since no alternate order exists to apply.
  `LabelingJobForWorkteamSummary` (`types/types.go:13262-13290`) already matched exactly — no
  response gap here.
- `ListSubscribedWorkteamsInput` (`api_op_ListSubscribedWorkteams.go:31-46`) was missing
  `MaxResults` entirely (only `NextToken`/`NameContains` existed) — disclosed as a no-op along with
  the pre-existing `NameContains`, since this backend models no Marketplace-vendor subscriptions at
  all and the list is always empty.
- `DescribeLabelingJobOutput` (`api_op_DescribeLabelingJob.go:39-97`) was already complete —
  control data point, not an oversight (matches parity-17's `handler_jobs.go` precedent for a
  fully-correct family found on inspection).
- `StopLabelingJob`/`DescribeSubscribedWorkteam`/`CreateLabelingJob` request shapes (beyond the
  `LabelAttributeName` gap above) already matched exactly; converted to named types for tooling
  visibility with no further field changes.

**`handler_inference_components.go` (InferenceComponent family) — the largest gap found this pass,
and this file's own `statusCreating` was the campaign's second "stuck-forever status" bug:**

- **Status never advanced.** `CreateInferenceComponent` set `InferenceComponentStatus: statusCreating`
  and nothing in the file ever transitioned it — identical to parity-15's ClusterSchedulerConfig
  finding, here for InferenceComponent. Real values are `InService`/`Creating`/`Updating`/`Failed`/
  `Deleting` (`types/enums.go:4390-4398`). Fixed via `scheduleInferenceComponentTransition`
  (mirroring `endpoints.go`'s `scheduleEndpointTransition` FSM pattern exactly): Creating ->
  InService on Create, Updating -> InService on both Update ops, `CurrentCopyCount` catching up to
  `CopyCount` only once InService — confirmed via hand-revert below.
- **A fabricated request field.** `UpdateInferenceComponentInput`
  (`api_op_UpdateInferenceComponent.go:24-53`) has **no `VariantName` member at all** — a variant is
  fixed at Create time and cannot be moved via Update — yet the handler decoded and (harmlessly,
  since no real client ever sends it) applied one. Removed; replaced with the op's four real optional
  members: `DeploymentConfig`, `RuntimeConfig`, `Specification`, `Specifications`, none of which were
  previously decoded at all.
- **`CreateInferenceComponentInput`** (`api_op_CreateInferenceComponent.go:68-116`) was missing
  `Specification`/`Specifications` entirely — the request shape carried only `RuntimeConfig.CopyCount`
  plus identity fields; the whole model/container/compute-resource deployment spec was silently
  dropped. Modeled via new `InferenceComponentSpecification`/`InferenceComponentContainerSpec` types.
  `ComputeResourceRequirements`/`DataCacheConfig`/`SchedulingConfig`/`StartupParameters` are carried
  as opaque `json.RawMessage`: each of those four sub-types (`types/types.go:11705-11727`,
  `11788-11815`, `11922-11941`, `12037-12051`) is used **unchanged** by both this request type and its
  response-side Summary counterpart (`InferenceComponentSpecificationSummary`,
  `types/types.go:12002-12034`), so a byte-for-byte passthrough between Create/Update and
  Describe/List is wire-correct without semantic modeling. Only `Container` needed real translation
  (see below).
- **`DescribeInferenceComponentOutput`** (`api_op_DescribeInferenceComponent.go:39-97`) was missing
  **`EndpointArn` — a required member, absent entirely** (synthesized via
  `arn.Build("sagemaker", region, accountID, "endpoint/"+name)`, matching `endpoints.go`'s own
  scheme byte-for-byte) plus `LastDeploymentConfig`, `RuntimeConfig` (Summary), `Specification`/
  `Specifications` (Summary). **A fabricated response field found and removed**: the previous
  handler called `json.Marshal(c)` directly on the internal storage struct, which carries a `Tags`
  field with no wire counterpart at all — `DescribeInferenceComponentOutput` has no `Tags` member;
  every real client's Describe call was receiving an extra field no SDK deserializer expects. Fixed
  by building a dedicated `inferenceComponentResponseMap` (mirroring `handler_labeling.go`'s
  `labelingJobResponseMap` pattern) instead of marshaling the storage type directly.
  `RuntimeConfig.CurrentCopyCount`/`DesiredCopyCount` are now real (`types/types.go:11906-11920`);
  `PlacementStatus` is disclosed not modeled (this backend simulates no per-instance-type placement).
  `Container.Image` (request) is echoed back as `Container.DeployedImage.SpecifiedImage` (response) —
  the one field where request/response shapes genuinely diverge
  (`InferenceComponentContainerSpecification.Image` vs
  `InferenceComponentContainerSpecificationSummary.DeployedImage`, `types/types.go:11731-11785`);
  `DeployedImage.ResolvedImage`/`ResolutionTime` are disclosed no-ops since this backend never
  resolves a registry digest. `LastDeploymentConfig` is `DeploymentConfig` echoed verbatim — both
  request and response use the **identical** `types.InferenceComponentDeploymentConfig`
  (`types/types.go:11820-11832`), so raw passthrough is wire-correct.
- **`ListInferenceComponentsInput`** (`api_op_ListInferenceComponents.go:30-71`) was missing **9 of
  12** optional fields — only `NextToken`/`EndpointNameEquals` existed (and under a fabricated
  `EndpointNameEquals` JSON tag on a Go field misleadingly named `EndpointName` in the old code).
  `CreationTimeAfter`/`CreationTimeBefore`/`LastModifiedTimeAfter`/`LastModifiedTimeBefore`/
  `MaxResults`/`NameContains`/`SortBy`/`SortOrder`/`StatusEquals`/`VariantNameEquals` are all now
  real. `InferenceComponentSummary` (`types/types.go:12054-12096`) was also missing **2 of 8**
  members, both **required**: `EndpointArn` and `VariantName` were absent entirely from the List
  response even though both were already stored per-component.
- **Disclosed, not modeled (InferenceComponent):** `ExplainerConfig`/`MetricsConfig`-class detail
  inside `Specification`'s sub-configs is carried opaquely (see above); no per-instance-type
  `PlacementStatus` simulation.

**`handler_endpoints.go` (Endpoint family) — a real behavioral bug found in `UpdateEndpoint`, not
just absent fields:**

- **`UpdateEndpointInput`** (`api_op_UpdateEndpoint.go:1-56`) was missing **`RetainAllVariantProperties`,
  `ExcludeRetainedVariantProperties`, `RetainDeploymentConfig`, `DeploymentConfig`** — all four
  optional members entirely absent. Worse than a missing field: **the existing code unconditionally
  behaved as if `RetainAllVariantProperties` were always false** (`Desired*` was always taken from
  the new EndpointConfig), silently discarding a real client's explicit request to retain the old
  deployment's variant properties (the doc's own default is `false`, so the *shape* looked
  plausible while the true-case behavior was simply unreachable). Fixed via
  `carryOverVariantProperties`: `Current*` still always carries over from the same-named old variant
  (traffic keeps flowing on old counts regardless of the flag — that part was already correct);
  `Desired*` now retains from the old variant when `RetainAllVariantProperties` is true, unless the
  specific `VariantPropertyType` (`types/enums.go:10837-10844`: `DesiredInstanceCount`/
  `DesiredWeight`/`DataCaptureConfig`) is listed in `ExcludeRetainedVariantProperties` — the
  `DataCaptureConfig` value has no effect here since this backend tracks data capture per-endpoint,
  not per-variant, disclosed in the type's doc comment. `RetainDeploymentConfig`/`DeploymentConfig`
  are real: retaining keeps the endpoint's prior `DeploymentConfig`, otherwise the new value (which
  may be absent, clearing it) is applied. Confirmed via hand-revert below.
- **`CreateEndpointInput`** (`api_op_CreateEndpoint.go:1-49`) was missing `DeploymentConfig` entirely.
- **`DescribeEndpointOutput`** (`api_op_DescribeEndpoint.go:39-97`) was missing **7 of 15** members:
  `AsyncInferenceConfig`, `DataCaptureConfig` (Summary), `ShadowProductionVariants`,
  `LastDeploymentConfig`, plus the always-disclosed `ExplainerConfig`/`MetricsConfig`/
  `PendingDeploymentSummary`. The first three were a genuine parsed-then-ignored bug: this backend's
  `EndpointConfig` (`models.go:148-161`) already stores `AsyncInferenceConfig`/`DataCaptureConfig`/
  `ShadowProductionVariants` from `CreateEndpointConfig`, but `DescribeEndpoint` never copied them
  onto the `Endpoint` at Create/Update time — every real client asking for an endpoint's data-capture
  or async-inference configuration got nothing back despite having configured one. Fixed: both
  Create and Update now copy all three from the active `EndpointConfig`; a new
  `dataCaptureConfigSummary` builds the real `DataCaptureConfigSummary` shape
  (`types/types.go:6685-6715`) from the stored `DataCaptureConfig`, with `CaptureStatus` mirroring
  `EnableCapture` (`Started`/`Stopped`, `types/enums.go:1809-1814`). `LastDeploymentConfig` is
  `DeploymentConfig` echoed verbatim (same identical-type passthrough rationale as
  InferenceComponent's, above — `types.DeploymentConfig` is shared by `CreateEndpointInput`,
  `UpdateEndpointInput`, and `DescribeEndpointOutput.LastDeploymentConfig`). `ExplainerConfig`/
  `MetricsConfig` are disclosed not modeled: this service's `EndpointConfig` type has no counterpart
  field for either, and adding one is out of this pass's scope (a `CreateEndpointConfig` change, a
  sibling op outside `handler_endpoints.go`). `PendingDeploymentSummary` is disclosed not simulated —
  this backend's FSM does not track fine-grained in-progress-deployment state.
- **`ListEndpointsInput`** (`api_op_ListEndpoints.go:30-64`) was missing **9 of 10** optional fields —
  only `NextToken` existed, and the backend had no filtering at all (`ListEndpoints(ctx, nextToken)`).
  All nine now real: `CreationTimeAfter`/`CreationTimeBefore`/`LastModifiedTimeAfter`/
  `LastModifiedTimeBefore`/`MaxResults`/`NameContains`/`SortBy`/`SortOrder`/`StatusEquals`.
  `EndpointSummary` (`types/types.go:8329-8386`) already matched exactly — no response gap.
- **`DeleteEndpointInput`**/`UpdateEndpointWeightsAndCapacitiesInput` already matched exactly;
  converted to named types for tooling visibility with no field changes.

**The six questions, answered explicitly:**

1. **What does the handler read that AWS never sends?** `UpdateInferenceComponentInput`'s
   `VariantName` (see above) — the real op has no such member.
2. **Do request and response use the same key?** Checked separately throughout: `Container.Image`
   (request) vs `Container.DeployedImage.SpecifiedImage` (response) diverge — the one place a naive
   passthrough would have been wrong-shaped; `DeploymentConfig`/`LastDeploymentConfig` and every
   `ComputeResourceRequirements`/`DataCacheConfig`/`SchedulingConfig`/`StartupParameters` sub-config
   confirmed **identical** on both sides before choosing raw passthrough for those.
3. **Is any required request member never read at all?** `CreateLabelingJobInput.LabelAttributeName`
   (required per doc, never validated) — see above.
4. **Is any field parsed and then ignored?** `UpdateEndpointInput.RetainAllVariantProperties` was the
   most severe instance this pass — not merely absent-decode but active wrong-default behavior (see
   above). This backend's `EndpointConfig.AsyncInferenceConfig`/`DataCaptureConfig`/
   `ShadowProductionVariants` were also stored-then-never-read by `DescribeEndpoint`.
5. **Does it emit every declared member, and does any handler return a nil body where the op
   declares required members?** `InferenceComponentSummary.EndpointArn`/`VariantName` (both
   required) were completely absent from `ListInferenceComponents`' response; `DescribeInferenceComponentOutput.EndpointArn`
   (required) likewise absent from Describe.
6. **Does any status or lifecycle field ever advance?** `InferenceComponentStatus` stayed
   `Creating` forever — see the FSM fix above, the campaign's second instance of this bug class
   after parity-15's ClusterSchedulerConfig.

**Timestamps touched, each with its own serializer/deserializer citation and a test that sets the
value:** `ListLabelingJobsInput.CreationTimeAfter/CreationTimeBefore/LastModifiedTimeAfter/LastModifiedTimeBefore`
and `ListLabelingJobsForWorkteamInput.CreationTimeAfter/CreationTimeBefore` (all `*time.Time` per
`api_op_ListLabelingJobs.go:30-71`/`api_op_ListLabelingJobsForWorkteam.go:30-64`, awsjson1.1 numbers,
decoded as `*float64`/`timeFromEpochSecondsPtr`); `ListInferenceComponentsInput`'s four time filters
(`api_op_ListInferenceComponents.go:30-71`, same treatment); `ListEndpointsInput`'s four time filters
(`api_op_ListEndpoints.go:30-64`, same treatment). Every one has a table-driven test
(`TestHandler_ListLabelingJobs_TimeFilters`, `TestHandler_ListLabelingJobsForWorkteam_Filters`,
`TestHandler_ListInferenceComponents_Filters`, `TestHandler_ListEndpoints_Filters`) asserting a
future value excludes and a past value includes — none left nil.

**An existing test found ratifying the fabricated-status bug:** `TestHandler_InferenceComponentLifecycle`
asserted `assert.Equal(t, "Creating", descResp["InferenceComponentStatus"])` immediately after
Create — correct in isolation (Creating is the right status right after Create) but the test never
checked the status ever left Creating, so it never caught that it never did. Fixed: the immediate
assertion is kept (still valid), plus a new `require.Eventually` block confirming the FSM reaches
`InService` and `CurrentCopyCount` catches up to `CopyCount`. The same test's `UpdateInferenceComponent`
call sent the fabricated `"VariantName": "variant-2"` — replaced with a real `DeploymentConfig`
payload and an assertion that it round-trips through `LastDeploymentConfig`.

**Enums touched, all read from the constants, none generalized across ops:** `SortBy`/`SortOrder`
(shared `types.SortBy`/`types.SortOrder`, `ListLabelingJobsInput` — default `CreationTime`/Ascending,
confirmed from this op's own doc, **not** assumed from sibling ops); `LabelingJobStatus`
(`types/enums.go:5114-5123`); `ListLabelingJobsForWorkteamSortByOptions` (confirmed exactly one real
value, `CreationTime` — the field is a disclosed no-op, not a gap, since no second value exists to
sort by); `InferenceComponentStatus`/`InferenceComponentSortKey`/`OrderKey`
(`types/enums.go:4369-4398`, `6798-6803` — `ListInferenceComponentsInput`'s own doc gives
`CreationTime`/Descending as defaults, **differing** from `ListLabelingJobsInput`'s
`CreationTime`/Ascending in the same pass, confirming per-op defaults rather than a service-wide
convention); `EndpointStatus`/`EndpointSortKey`/`OrderKey`/`VariantPropertyType`
(`types/enums.go:3386-3398`, `3365-3371`, `10837-10844` — `ListEndpointsInput`'s doc also gives
`CreationTime`/Descending, matching InferenceComponent's but confirmed independently from this op's
own doc text, not copied over).

**Disclosures (all three files):** `TrialComponentSource`-class ARN filters n/a here;
`LabelingJobsForWorkteam.SortBy` (one real value, no-op); `ListSubscribedWorkteams`
filters (always-empty backend); InferenceComponent's `PlacementStatus`/`ExplainerConfig`/
`MetricsConfig`/`PendingDeploymentSummary` (all not modeled, stated above with the reason each
lacks a backing field).

**Hand-revert results (one representative fix per file, each restored and `md5sum`-verified
byte-identical to its pre-revert state afterward):**
- `handler_labeling.go`: stripped `ListLabelingJobs`' four `timeFromEpochSecondsPtr` assignments back
  to omitted — `TestHandler_ListLabelingJobs_TimeFilters` failed exactly as predicted (all four
  "excludes" subtests returned the filtered-out job instead of an empty list).
- `handler_inference_components.go`: removed `CreateInferenceComponent`'s
  `scheduleInferenceComponentTransition` call — `TestHandler_InferenceComponentLifecycle`'s new
  `require.Eventually` block failed with "Condition never satisfied" (2s timeout), reproducing the
  stuck-`Creating`-forever bug precisely.
- `handler_endpoints.go`: removed the `RetainAllVariantProperties`-gated block from
  `carryOverVariantProperties` (reverting to the original always-take-new-config behavior) —
  `TestHandler_UpdateEndpoint_RetainAllVariantProperties`'s "keeps the old Desired values" and
  "ExcludeRetainedVariantProperties" subtests both failed with the exact predicted symptom
  (`Desired*` values from the new EndpointConfig instead of the retained old ones).

**Tests added:** `TestHandler_CreateLabelingJob_MissingRequiredFields` (table, extended with the
`LabelAttributeName` case), `TestHandler_ListLabelingJobs_TimeFilters`,
`TestHandler_ListLabelingJobs_SortBy`, `TestHandler_ListLabelingJobs_ResponseIncludesInputConfigAndOutput`,
`TestHandler_ListLabelingJobsForWorkteam_Filters`; `TestHandler_CreateInferenceComponent_MissingRequiredFields`,
`TestHandler_CreateInferenceComponent_SpecificationContainerImage`,
`TestHandler_ListInferenceComponents_Filters`, `TestHandler_ListInferenceComponents_SortByName`;
`TestHandler_DescribeEndpoint_DataCaptureAndAsyncInferenceConfig`,
`TestHandler_CreateEndpoint_DeploymentConfigEchoedOnDescribe`,
`TestHandler_UpdateEndpoint_RetainAllVariantProperties` (table, three subtests),
`TestHandler_ListEndpoints_Filters`, `TestHandler_ListEndpoints_SortByName`. All `t.Parallel()`
outer+subtest, short lowercase subtest names, `require`/`assert` split. One pre-existing unbubbled
`time.Sleep(400ms)` in `TestHandler_DescribeEndpoint_EventuallyInService` converted to
`require.Eventually` as a drive-by fix while already restructuring this test file (flagged by this
repo's own no-`time.Sleep`-in-tests convention, not part of this pass's assigned scope but trivial
to fix in passing).

**Refactoring for `cyclop`/`gocognit` (no `nolint` used, per the ban on suppressing these):**
`ListEndpoints`/`ListInferenceComponents`/`ListLabelingJobs` each decomposed into a
`*MatchesFilter`/`less*` helper pair (`endpointMatchesFilter`/`lessEndpoint`,
`inferenceComponentMatchesFilter`/`lessInferenceComponent`, `labelingJobMatchesFilter`/
`lessLabelingJob`); `inferenceComponentMatchesFilter` further split into
`inferenceComponentMatchesIdentityFilters`/`inferenceComponentMatchesTimeFilters` to clear cyclop's
15-branch limit; `UpdateEndpoint`'s per-variant retention loop extracted into
`carryOverVariantProperties`. `goconst` findings (my new code pushed five wire-key strings —
`"EndpointArn"`/`"EndpointName"`/`"Name"`/`"Status"`/`"Updating"` — over the 3-occurrence threshold,
which also flagged four **pre-existing, untouched** files sharing those strings) fixed by routing
through the existing `keyEndpointArn`/`keyEndpointNameField`/`keyGenericName`/`keyStatus` constants
and a new shared `statusUpdating` constant (`handler_keys.go`), rather than touching the unrelated
flagged files. `revive var-naming` (`ArtifactUrl` -> `ArtifactURL`, JSON tag unchanged) and a
`modernize mapsloop` (`maps.Copy` in a test) also fixed. `fieldalignment -fix` run repo-wide-scoped
to `./services/sagemaker/...` (no other packages touched).

Gates for this session: `go build ./services/sagemaker/...`, `go build ./...` (repo-wide), `go vet
./services/sagemaker/...`, `go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration
./services/sagemaker/...`, `go vet -vettool=$(go tool -n mulint-vet) ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker/*.go` (empty), `go test -race ./services/sagemaker/...` (pass, run
twice plus `-count=1` to rule out FSM-timing flakiness after two `StatusEquals` list-filter tests
were found racing the async Creating->InService transition under full-suite load — fixed by waiting
for InService via `require.Eventually` before asserting on `StatusEquals` rather than depending on
transient `Creating`), and `golangci-lint run ./services/sagemaker/...` (0 issues) all clean. Zero
`nolint` added.

**`last_audit_commit` left at its existing value (`5f91d37c7`)** — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**144 of sagemaker's 362 inline structs now remain.** New boundary: 8 files tied at 5
(`handler_workteams.go`, `handler_workforces.go`, `handler_trials.go`, `handler_training_jobs.go`,
`handler_projects.go`, `handler_optimization_jobs.go`, `handler_inference_recommendations_jobs.go`,
`handler_hp_tuning_jobs.go`) — next in line per the campaign's largest-pile-first ordering, now that
all files previously tied at 6 or higher are cleared.

## parity-20 (2026-08-21, gopherstack-oc9v): Workteam/Workforce/Trial/TrainingJob/
Project/OptimizationJob/InferenceRecommendationsJob/HyperParameterTuningJob
inline-struct sweep (all eight files tied at 5)

Fourteenth pass of the gopherstack-oc9v campaign. Per parity-19's boundary note, this pass took all
eight files tied at 5 (`handler_workteams.go`, `handler_workforces.go`, `handler_trials.go`,
`handler_training_jobs.go`, `handler_projects.go`, `handler_optimization_jobs.go`,
`handler_inference_recommendations_jobs.go`, `handler_hp_tuning_jobs.go`), each verified by
`grep -c 'var req struct {' <file>.go` = 5 before starting. All 40 of this pass's structs were
converted to named types and wire-audited field-by-field against the pinned SDK (`v1.263.2`,
confirmed from `go.mod`, matching prior passes). **104 of sagemaker's 362 inline structs now
remain** (362 − 19 − 19 − 15 − 14 − 14 − 12 − 11 − 11 − 10 − 9 − 9 − 7 − 7 − 7 − 7 − 6 − 6 − 6 − 6 −
6 − 6 − 5 − 5 − 5 − 5 − 5 − 5 − 5 − 5), confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go`
summed, not arithmetic; all eight files now have zero. **New boundary: 8 files tied at 5**
(`handler_ai_benchmark_jobs.go`, `handler_ai_recommendation_jobs.go`, `handler_app_image_configs.go`,
`handler_automl_search.go`, `handler_code_repositories.go`, `handler_compilation_jobs.go`,
`handler_experiments.go`, `handler_feature_groups.go`).

**`handler_workteams.go` (Workteam family):**

- `CreateWorkteamInput`/`UpdateWorkteamInput` (`api_op_CreateWorkteam.go:35-83`,
  `api_op_UpdateWorkteam.go:28-70`) were both missing `NotificationConfiguration` and
  `WorkerAccessConfiguration` entirely — new `NotificationConfiguration`/`WorkerAccessConfiguration`/
  `S3Presign`/`IamPolicyConstraints` types added (`workteams.go`), threaded through Create/Update and
  echoed on Describe/List.
- `ListWorkteamsInput` (`api_op_ListWorkteams.go:31-51`) was `NextToken`-only — `NameContains`,
  `SortBy`/`SortOrder`, `MaxResults` all now real. **Doc-versus-enum mismatch**: the op's own doc
  says "The default is `CreationTime`", but `ListWorkteamsSortByOptions`
  (`types/enums.go:5441-5442`) has only `Name`/`CreateDate` — no `CreationTime` value exists.
  `CreateDate` (the timestamp field the doc was clearly describing) is used as the real default,
  not the nonexistent documented string.

**`handler_workforces.go` (Workforce family):**

- `CreateWorkforceInput`/`UpdateWorkforceInput` (`api_op_CreateWorkforce.go:48-86`,
  `api_op_UpdateWorkforce.go:65-93`) were both missing `IpAddressType` — added, stored, echoed.
- `ListWorkforcesInput` (`api_op_ListWorkforces.go:31-48`) was `NextToken`-only — `NameContains`,
  `SortBy`/`SortOrder`, `MaxResults` all now real (functionally bounded to ≤1 result, since AWS
  allows only one private workforce per account per region, but the filter/sort/pagination fields
  are wire-decoded and honored for shape fidelity regardless).

**`handler_trials.go` (Trial family) — `ExperimentName` was a documented filter that had never
actually been wired:**

- `CreateTrialInput` (`api_op_CreateTrial.go:44-71`) was missing `MetadataProperties` — added,
  reusing the existing shared `MetadataProperties` type (`lineage.go:56-61`, established by
  parity-18 for the identical field on `CreateTrialComponent`).
- `ListTrialsInput` (`api_op_ListTrials.go:34-63`) was `NextToken`-only. **A pre-existing test,
  `TestHandler_TrialLifecycle`, sent `"ExperimentName": "trial-experiment"` to `ListTrials` and
  asserted exactly 1 result — passing only because the single trial created happened to belong to
  that experiment anyway, never because the filter did anything.** `ExperimentName`,
  `TrialComponentName` (real, resolved via the existing `trialComponentAssociationsStoreRO`
  association store), `CreatedAfter`/`CreatedBefore`, `MaxResults`, `SortBy`/`SortOrder` are now all
  real. The op's own doc states both real defaults explicitly: `CreationTime`/`Ascending`.
  `SortTrialsBy`'s values are `Name`/`CreationTime` (`types/enums.go:9364-9365`) — no
  doc/enum mismatch here, unlike Workteam's `SortBy` above.

**`handler_training_jobs.go` (TrainingJob family) — the most severe finding of this pass:**

- **`UpdateTrainingJob` was a complete no-op.** The handler decoded zero fields beyond
  `TrainingJobName`, called `DescribeTrainingJob`, and returned its ARN — every real
  `UpdateTrainingJobInput` field (`ResourceConfig.KeepAlivePeriodInSeconds`, `ProfilerConfig`,
  `ProfilerRuleConfigurations`, `RemoteDebugConfig`, all `api_op_UpdateTrainingJob.go:29-56`) was
  silently discarded, and there was no backend `UpdateTrainingJob` method at all. **This service's
  own `families:` manifest entry (line 106) falsely claimed `UpdateTrainingJob verified` as part of
  an `ok` grade** — corrected inline (see above) rather than silently overwritten, per this
  campaign's standing instruction to treat existing claims as suspect and disclose corrections
  explicitly. Fixed: a real `UpdateTrainingJob` backend method now applies
  `ResourceConfig.KeepAlivePeriodInSeconds` (the one field this backend's `ResourceConfig` already
  modeled); `ProfilerConfig`/`ProfilerRuleConfigurations`/`RemoteDebugConfig` are decoded for
  wire-shape fidelity only and disclosed not modeled — this backend's `TrainingJob` has no
  profiler/remote-debug concept at all, `CreateTrainingJob` included, so there is nothing on the
  resource for an Update to mutate.
- `ListTrainingJobsInput` (`api_op_ListTrainingJobs.go:33-84`) was unconditionally sorted
  ascending-by-name regardless of what was requested, **contradicting the op's own documented
  default (`CreationTime`/`Ascending`)** — a real, previously-undetected sort-order bug, not merely
  an absent field. `LastModifiedTimeAfter`/`Before`, `SortBy`(`Name`/`Status`/`CreationTime`),
  `SortOrder` all now real. `TrainingPlanArnEquals`/`WarmPoolStatusEquals` are decoded and correctly
  return zero matches when set — this backend never associates a job with either concept, so
  "zero jobs match a training plan/warm-pool status" is the true answer, not a silently-ignored
  filter (disclosed, not modeled).

**`handler_projects.go` (Project family) — a fabricated-response-field bug, the same class as
parity-19's `DescribeInferenceComponent`:**

- **`DescribeProject` marshaled the internal storage struct directly** (`json.Marshal(result)`),
  and `Project`'s storage type carries `Tags`. Real `DescribeProjectOutput` has **no `Tags` member
  at all** (a real client fetches tags via `ListTags`) — every real client's Describe call was
  receiving a field the SDK deserializer never expects. Fixed via a dedicated
  `projectResponseMap` (matching parity-19's `inferenceComponentResponseMap`/
  `labelingJobResponseMap` precedent) instead of marshaling the storage type directly.
- `CreateProjectInput` (`api_op_CreateProject.go:30-59`) was missing
  `ServiceCatalogProvisioningDetails`/`TemplateProviders` entirely — added as `json.RawMessage`
  passthrough (this backend never simulates actual Service Catalog provisioning) and echoed on
  Describe. `LastModifiedTime` (required per `DescribeProjectOutput`) was entirely absent from the
  `Project` struct — added, threaded through Create/Update.
- `UpdateProjectInput`'s `ServiceCatalogProvisioningUpdateDetails`/`TemplateProvidersToUpdate` are
  disclosed not modeled (not decoded at all, so nothing is accepted-and-dropped) — applying either
  for real requires simulating an actual provisioned-product update, out of this pass's scope.
- `ListProjectsInput` (`api_op_ListProjects.go:30-58`) was `NextToken`-only — `CreationTimeAfter`/
  `Before`, `NameContains`, `SortBy`/`SortOrder`, `MaxResults` all now real.

**`handler_optimization_jobs.go` (OptimizationJob family) — the second-most severe finding, on par
with TrainingJob's:**

- **`CreateOptimizationJobInput`'s five other required members were never decoded at all**:
  `ModelSource`, `OptimizationConfigs`, `OutputConfig`, `DeploymentInstanceType`,
  `StoppingCondition` (all `api_op_CreateOptimizationJob.go:48-114`, all `This member is required`)
  — a request missing every one of them previously succeeded. Fixed: all five now decoded (the
  three deeply-nested ones as `json.RawMessage` passthrough, matching this file's
  `ai_workload_config`/`ai_benchmark_job` precedent) and validated present, matching the real API's
  required-field contract.
- `OptimizationJobSummary` (`types/types.go:16620-16660`) was missing **two required members**:
  `DeploymentInstanceType` and `OptimizationTypes`. `OptimizationTypes` has no formal enum in the
  pinned SDK (it is a bare `[]string`), so its real values were derived from the
  `OptimizationConfig` union's four member wire keys (`ModelCompilationConfig` →
  `"Compilation"`, `ModelQuantizationConfig` → `"Quantization"` — both confirmed by the op's own
  doc text for `OptimizationContains`; `ModelShardingConfig` → `"Sharding"`,
  `ModelSpeculativeDecodingConfig` → `"SpeculativeDecoding"` inferred from the same
  `Model*Config`-stripped naming convention, not independently confirmed against a real wire
  example — disclosed).
- `ListOptimizationJobsInput` (`api_op_ListOptimizationJobs.go:30-72`) was `NextToken`-only —
  `CreationTimeAfter`/`Before`, `LastModifiedTimeAfter`/`Before`, `NameContains`,
  `OptimizationContains` (real, derived from the same technique-name mapping above),
  `StatusEquals`, `SortBy`/`SortOrder`, `MaxResults` all now real.

**`handler_inference_recommendations_jobs.go` (InferenceRecommendationsJob family):**

- `CreateInferenceRecommendationsJobInput`'s `RoleArn` and `InputConfig`
  (`api_op_CreateInferenceRecommendationsJob.go:1-58`, both `This member is required`) were decoded
  but **never validated as present** — now enforced. `OutputConfig`/`StoppingConditions` were
  entirely absent — added as `json.RawMessage` passthrough; `OutputConfig` is deliberately never
  echoed by Describe (real `DescribeInferenceRecommendationsJobOutput` has no `OutputConfig` member
  at all — an asymmetry by design, not a gap) while `StoppingConditions` is echoed (it is a real
  optional Describe field). **`JobType` doc-versus-required mismatch**: flagged `This member is
  required` in the generated struct comment, but the op's own prose says "If left unspecified,
  Amazon SageMaker Inference Recommender will run ... (DEFAULT)" — implemented as documented
  (default `"Default"` when absent) rather than hard-rejecting a request that omits it, matching
  this campaign's standing rule to read the doc text over the required-flag alone. (The pinned
  Go SDK's own client-side validation middleware does still reject an omitted `JobType`, so no real
  Go SDK caller can currently exercise the default path — but a non-Go client that skips
  client-side validation could, and the documented default is honored either way.)
- `ListInferenceRecommendationsJobsInput` (`api_op_ListInferenceRecommendationsJobs.go:30-63`) was
  `NextToken`-only — `CreationTimeAfter`/`Before`, `LastModifiedTimeAfter`/`Before`, `NameContains`,
  `ModelNameEquals`/`ModelPackageVersionArnEquals` (real, decoded from the opaque stored
  `InputConfig`'s `ModelName`/`ModelPackageVersionArn` sub-fields via a small
  `inputConfigModelIdentity` shim), `StatusEquals`, `SortBy`/`SortOrder`, `MaxResults` all now real.
- `ListInferenceRecommendationsJobStepsInput`'s `MaxResults`/`Status`/`StepType` are decoded for
  wire-shape fidelity but disclosed no-ops — this backend never populates any job's `Steps` at all
  (no recommender-subtask simulation exists), so there is nothing to filter or page over.

**`handler_hp_tuning_jobs.go` (HyperParameterTuningJob family) — the third instance of the
stuck-status bug class:**

- **`HyperParameterTuningJobStatus` never left `Stopping`.** `StopHyperParameterTuningJob` set the
  status and nothing in the file ever transitioned it — the third instance of this bug class after
  parity-15's ClusterSchedulerConfig and parity-19's InferenceComponent, this time with **no
  existing test even asserting the immediate post-Stop value**, let alone checking it advanced.
  Fixed via a `hpTuningJobStoppingToStopped` (150ms) FSM matching the sibling FSMs already
  established in `lifecycle.go`.
- **`CreateHyperParameterTuningJobInput`'s `Autotune`/`WarmStartConfig`/`TrainingJobDefinition`/
  `TrainingJobDefinitions`** (`api_op_CreateHyperParameterTuningJob.go:44-77`) **were entirely
  absent from decode** — a real client could never actually configure what training job the tuning
  job launches. Fixed as `json.RawMessage` passthrough (the same convention as the campaign's other
  deeply-nested-union families), threaded through Create and echoed verbatim on Describe (real
  `DescribeHyperParameterTuningJobOutput` never mutates any of these after Create, so raw passthrough
  is wire-correct, not merely convenient).
- **`HyperParameterTuningJobConfig`'s own sub-fields beyond `Strategy`/`ResourceLimits`** —
  `HyperParameterTuningJobObjective`, `ParameterRanges`, `RandomSeed`, `StrategyConfig`,
  `TrainingJobEarlyStoppingType`, `TuningJobCompletionCriteria`
  (`types/types.go:11052-11113`) — **were silently dropped by the existing flat
  Strategy/ResourceLimits-only decode**, and `DescribeHyperParameterTuningJob`'s hand-rebuilt
  `map[string]any{"Strategy":..., "ResourceLimits":...}` response threw them away a second time on
  the way out. Fixed: the full config is now captured as raw JSON at Create (Strategy/ResourceLimits
  are *also* decoded a second time from that same blob into their own typed fields, since this
  file's internal filter/sort/summary logic needs them independent of the raw blob — a single JSON
  key cannot bind to two struct fields in one `Unmarshal` pass, so this is a genuine second decode,
  not a redundant one) and echoed verbatim on Describe.
- **`TrainingJobEarlyStoppingType` doc-versus-enum casing mismatch**: the field's doc prose says
  "the default value is `OFF`" (and separately narrates `AUTO`), but
  `TrainingJobEarlyStoppingType`'s real enum values (`types/enums.go:10293-10295`) are
  `"Off"`/`"Auto"` — Titlecase, not the all-caps the prose implies. Confirmed via a real-SDK-client
  round-trip test (`TestHandler_CreateHyperParameterTuningJob_ExtrasRoundTrip_RealClient`) rather
  than assumed.
- `ListHyperParameterTuningJobsInput` (`api_op_ListHyperParameterTuningJobs.go:30-64`) was
  `NextToken`-only — `CreationTimeAfter`/`Before`, `LastModifiedTimeAfter`/`Before`, `NameContains`,
  `StatusEquals`, `SortBy`(`Name`/`Status`/`CreationTime`), `SortOrder`, `MaxResults` all now real.
  The op's own doc states the real default explicitly: `SortBy` is `Name` — **not** `CreationTime`
  like most sibling List ops in this service, confirmed per-op rather than generalized.
- `ListTrainingJobsForHyperParameterTuningJobInput` (`api_op_ListTrainingJobsForHyperParameterTuningJob.go:27-56`)
  previously discarded its `nextToken` parameter entirely (named `_`) and returned every matching
  training job unpaginated. Added real `StatusEquals`, `SortBy`(`Name`/`Status`/`CreationTime`,
  default `Name` per the op's own doc), `SortOrder`, `MaxResults`/pagination via the shared
  `paginateSlice` helper. `SortBy == "FinalObjectiveMetricValue"` is a disclosed, genuinely-correct
  no-op: the doc's own text says a training job with no objective metric is excluded entirely when
  sorting by it, and this backend never computes an objective metric for any child training job, so
  always-empty is the correct answer, not a convenient shortcut.

**The six questions, answered explicitly:**

1. **What does the handler read that AWS never sends?** Nothing found this pass — every field this
   pass's eight handlers previously decoded does exist on the real request type. (Contrast with
   prior passes' fabricated-field finds; this pass's severe bugs were all absences, not fabrications,
   on the request-decode side.)
2. **Do request and response use the same key?** Checked separately throughout; no divergence found
   this pass (contrast parity-19's `Container.Image`/`Container.DeployedImage.SpecifiedImage`).
   Confirmed several identical-shape passthroughs are wire-correct: `ServiceCatalogProvisioningDetails`
   (same `types.ServiceCatalogProvisioningDetails` on both Create request and Describe response),
   `HyperParameterTuningJobConfig` (never mutated between Create and Describe).
3. **Is any required request member never read at all?** `CreateOptimizationJobInput.ModelSource`/
   `OptimizationConfigs`/`OutputConfig`/`DeploymentInstanceType`/`StoppingCondition` (all five,
   simultaneously — the most severe instance of this bug class found in this pass) —
   `CreateInferenceRecommendationsJobInput.RoleArn`/`InputConfig` (decoded but unvalidated) — see
   above for both.
4. **Is any field parsed and then ignored?** None found parsed-then-ignored this pass in the sense
   of a decoded field silently discarded at apply time (contrast parity-19's
   `UpdateEndpoint.RetainAllVariantProperties`); this pass's analogous bugs were fields never
   decoded in the first place, which is the adjacent but distinct failure mode this campaign exists
   to find.
5. **Does it emit every declared member, and does any handler return a nil body where required
   members are declared?** `OptimizationJobSummary.DeploymentInstanceType`/`OptimizationTypes`
   (both required) were completely absent from `ListOptimizationJobs`' response — see above.
   `Project`'s `LastModifiedTime` (required per `DescribeProjectOutput`) didn't exist on the struct
   at all before this pass.
6. **Does any status or lifecycle field ever advance?** `HyperParameterTuningJobStatus` stayed
   `Stopping` forever after Stop — the third instance of this bug class, fixed via FSM (see above).
   `InferenceRecommendationsJob`'s own IN_PROGRESS→COMPLETED and STOPPING→STOPPED FSMs (added by an
   earlier pass, per this file's own doc comments) were re-verified correct and left untouched — a
   control data point showing this file's own prior author already knew and fixed this exact bug
   class for a sibling status field.

**Timestamps touched, each with its own serializer citation and a test that sets the value:**
`ListTrialsInput.CreatedAfter/CreatedBefore` (`*time.Time` per `api_op_ListTrials.go:37-40`, decoded
as `*float64`/`timeFromEpochSecondsPtr`, asserted in
`TestHandler_ListTrials_FilterSortPage_RealClient`'s `created after future excludes`/`past includes`
subtests); `ListTrainingJobsInput.LastModifiedTimeAfter/Before`
(`api_op_ListTrainingJobs.go:33-84`, asserted in `TestHandler_ListTrainingJobs_FilterSortPage_RealClient`);
`ListOptimizationJobsInput`'s four time filters (`api_op_ListOptimizationJobs.go:30-72`, asserted in
`TestHandler_ListOptimizationJobs_FilterSortPage_RealClient`); `ListInferenceRecommendationsJobsInput`'s
four time filters (asserted in `TestHandler_ListInferenceRecommendationsJobs_Filters_RealClient`);
`ListProjectsInput.CreationTimeAfter/Before` (asserted in `TestHandler_ListProjects_Filters`);
`ListHyperParameterTuningJobsInput`'s four time filters (asserted in
`TestHandler_ListHyperParameterTuningJobs_FilterSortPage_RealClient`). `Project.LastModifiedTime`
and `OptimizationJob`'s existing `MarshalJSON`/`UnmarshalJSON` epoch-seconds override (already
correct from an earlier pass) were extended, not rewritten, to also cover the new field. None left
nil in any test.

**Four existing tests found ratifying defects, three not previously disclosed:**

1. `TestHandler_TrialLifecycle` sent `ExperimentName` to `ListTrials` and asserted a matching count
   — passing only because the sole trial happened to belong to that experiment regardless of
   whether the filter did anything (it didn't, until this pass).
2. `TestHandler_CreateOptimizationJob`/`TestHandler_DescribeOptimizationJob`/
   `TestHandler_StopOptimizationJob`/`TestHandler_DeleteOptimizationJob`/`TestHandler_ListOptimizationJobs`
   all sent a request with only `OptimizationJobName` (`ListOptimizationJobs`' variant also sent
   `RoleArn`) and asserted success — every one of `ModelSource`/`OptimizationConfigs`/
   `OutputConfig`/`DeploymentInstanceType`/`StoppingCondition` absent, none rejected. Rewritten
   around a shared `validOptimizationJobBody` helper carrying every required field, plus a new
   table-driven `TestHandler_CreateOptimizationJob_RequiredFieldsEnforced` asserting each one is
   independently enforced.
3. `TestHandler_InferenceRecommendationsJobLifecycle`/`TestHandler_InferenceRecommendationsJob_ReachesCompleted`/
   `TestHandler_CreateInferenceRecommendationsJob_InputConfigRoundTrip` sent `RoleArn` but no
   `InputConfig` (the first two), silently accepted because `InputConfig` was decoded but never
   validated present. Fixed by adding `InputConfig` to all three and asserting the omission is now
   rejected (`TestHandler_CreateInferenceRecommendationsJob_RequiredFieldsEnforced`).
4. `TestHandler_GetScalingConfigurationRecommendation` (`handler_automl_search_test.go`, outside
   this pass's file scope but a cross-file consumer of `CreateInferenceRecommendationsJob`) also
   sent no `InputConfig` and broke once the new validation landed — fixed as a knock-on correction
   in the same file, not a new defect introduced by this pass's validation (the request it was
   sending was never valid against the real API to begin with).

**Enums read per op, not generalized:** `ListWorkteamsSortByOptions` (`Name`/`CreateDate` — doc says
`CreationTime`, a mismatch); `HyperParameterTuningJobSortByOptions` (`Name`/`Status`/`CreationTime`,
default `Name` — unlike most sibling ops); `TrainingJobSortByOptions` (adds
`FinalObjectiveMetricValue`, a fourth value none of this service's other `SortBy` enums have);
`OptimizationJobStatus` (`INPROGRESS`/`COMPLETED`/`FAILED`/`STARTING`/`STOPPING`/`STOPPED` — all
caps, unlike most sibling status enums in this service, confirmed matching the pre-existing
`"COMPLETED"` hardcode rather than assumed); `TrainingJobEarlyStoppingType` (`Off`/`Auto` — doc
prose says `OFF`/`AUTO`, a casing mismatch); `RecommendationJobType` (`Default`/`Advanced` — doc
says a request may omit it and get `DEFAULT` behavior despite the struct being flagged required).

**Disclosures (not fixed, out of this pass's scope):** `HyperParameterTuningJob`'s
`TrainingJobDefinition(s)`/`ParameterRanges`/`StrategyConfig`/`HyperParameterTuningJobObjective`/
`TuningJobCompletionCriteria` remain opaque passthrough, not semantically modeled — actually running
a hyperparameter search or launching child training jobs is out of scope for an in-memory emulator
of this depth. `OptimizationJob`'s `ModelSource`/`OutputConfig`/`VpcConfig` likewise. `Project`'s
`ServiceCatalogProvisioningUpdateDetails`/`TemplateProvidersToUpdate` (Update-only) not accepted at
all, disclosed rather than silently dropped. `InferenceRecommendationsJob`'s `Steps` always empty
(no recommender-subtask simulation). None of these are wire-shape bugs — every field a real client
sends round-trips exactly; the gap is in simulated *behavior*, disclosed the same way this file's
`ai_workload_config`/`ai_benchmark_job`/`ai_recommendation_job` families already are.

**Hand-revert proof (four representative fixes, the most severe found this pass):**

1. `UpdateTrainingJob` no-op: reverted `handler_training_jobs.go`/`training_jobs.go` to HEAD,
   rebuilt clean, ran `TestHandler_UpdateTrainingJob_KeepAlivePeriod_RealClient` — failed exactly as
   predicted (`expected: 600, actual: 0`). Restored, `md5sum` byte-identical, test passes again.
2. `StopHyperParameterTuningJob` stuck-forever status: reverted `hp_tuning_jobs.go`/`lifecycle.go`/
   `handler_hp_tuning_jobs.go`/`interfaces.go` to HEAD, rebuilt clean, ran
   `TestHandler_StopHyperParameterTuningJob_ReachesStopped` — failed exactly as predicted
   (`Condition never satisfied`, stuck at `Stopping` for the full 2s timeout). Restored, `md5sum`
   byte-identical for all four files, test passes again.
3. `CreateOptimizationJob` missing required-field validation: reverted `optimization_jobs.go`/
   `handler_optimization_jobs.go` to HEAD, rebuilt clean, ran
   `TestHandler_CreateOptimizationJob_RequiredFieldsEnforced` — all five subtests failed exactly as
   predicted (`expected: 400, actual: 200`). Restored, `md5sum` byte-identical, test passes again.
4. `DescribeProject` Tags leak: reverted `handler_projects.go`/`projects.go` to HEAD, rebuilt clean,
   ran `TestHandler_DescribeProject_NoTagsLeak` — failed exactly as predicted (`Tags":{"env":"prod"}`
   present in the response map, `LastModifiedTime` absent/`nil`). Restored, `md5sum` byte-identical,
   test passes again.

**Refactoring for `cyclop`/`gocognit` (no `nolint` used, per the ban on suppressing these):**
`inferenceRecommendationsJobMatchesFilter`/`optimizationJobMatchesFilter` each decomposed into a
`*MatchesModelIdentity`/`*MatchesOptimizationContains` helper plus a new shared `timeWindowOK`
(`list_helpers.go`) collapsing the repeated `CreationTimeAfter`/`CreationTimeBefore`/
`LastModifiedTimeAfter`/`LastModifiedTimeBefore` if-pairs this pass introduced across six files into
one two-line call; `(*InMemoryBackend).ListTrainingJobsFiltered`/`ListTrials` each decomposed into a
`*MatchesListFilter`/`less*` helper pair (`trainingJobMatchesListFilter`/`lessTrainingJobBySortBy`,
`trialMatchesFilter`/`lessTrial`, the latter also extracting `trialNamesForComponent` for the
`TrialComponentName` association lookup). `gochecknoglobals` (`optimizationConfigTypeNames`
map) converted to a `switch`-based `optimizationConfigTypeName` function with the wire-key list
moved into a function-local array, eliminating the package-level var entirely rather than
suppressing the finding. `goconst` findings (this pass's new code pushed `ProjectArn`/`ProjectId`/
`ProjectName`/`ProjectStatus`/`LastModifiedTime`/`CreationTime` over the 3-occurrence threshold)
fixed by adding `keyProjectID`/`keyProjectName`/`keyProjectStatus` to the existing `handler_keys.go`
constant block and routing through them plus the pre-existing `keyProjectArn`/`keyCreationTime`/
`keyLastModifiedTime`, rather than leaving literals in place; two pre-existing, untouched
`automl_search.go`/`notebook_instances.go` `goconst` findings left alone (not this pass's files).
`revive var-naming` (`workteams.go`'s `IamPolicyConstraints.SourceIp`/`VpcSourceIp` →
`SourceIP`/`VpcSourceIP`, JSON tags unchanged) and six `govet shadow` findings in
`handler_trials_test.go` (renamed shadowing `err` to `setupErr`/`createErr`) also fixed.
`fieldalignment -fix` run scoped to `./services/sagemaker/...`; confirmed via a before/after
`git status --short` file-listing diff that only the same set of files this pass already touched
were modified (all nine flagged structs lived in files this pass authored: `handler_hp_tuning_jobs.go`,
`handler_inference_recommendations_jobs.go`, `handler_training_jobs.go`, `hp_tuning_jobs.go`,
`optimization_jobs.go`, `projects.go`) — no unintended files touched.

Gates for this session: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`,
`gofmt -l ./services/sagemaker/*.go` (empty), `go test -race -count=1 ./services/sagemaker/...`
(pass), and `golangci-lint run ./services/sagemaker/...` (0 issues) all clean. Zero `nolint` added.
(`go build ./...` repo-wide shows pre-existing, unrelated failures in `services/opensearch` from a
concurrent agent's in-progress map-literal sweep — confirmed via `git status --short` showing those
files modified outside this session's changes, not touched by this pass.)

**`last_audit_commit` left at its existing value (`5f91d37c7`)** — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**104 of sagemaker's 362 inline structs now remain.** New boundary: 8 files tied at 5
(`handler_ai_benchmark_jobs.go`, `handler_ai_recommendation_jobs.go`, `handler_app_image_configs.go`,
`handler_automl_search.go`, `handler_code_repositories.go`, `handler_compilation_jobs.go`,
`handler_experiments.go`, `handler_feature_groups.go`) — next in line per the campaign's
largest-pile-first ordering, now that all files previously tied at 6 or higher, and all eight
previously tied at 5, are cleared.

## parity-21 (2026-08-21, gopherstack-oc9v): AIBenchmarkJob/AIRecommendationJob
List sort-order default + CompilationJob/AppImageConfig/CodeRepository field audit
(5 of 8 files tied at 5)

Fifteenth pass of the gopherstack-oc9v campaign. Per parity-20's boundary note, this pass took
five of the eight files tied at 5 (`handler_ai_benchmark_jobs.go`, `handler_ai_recommendation_jobs.go`,
`handler_app_image_configs.go`, `handler_code_repositories.go`, `handler_compilation_jobs.go`), each
verified by `grep -c 'var req struct {' <file>.go` = 5 before starting; the remaining three
(`handler_automl_search.go`, `handler_experiments.go`, `handler_feature_groups.go`) were not started
this pass, per this pass's own effort budget rather than any difficulty found in them. All 25 of
this pass's structs were converted to named types and wire-audited field-by-field against the pinned
SDK (`v1.263.2`, confirmed from `go.mod`, matching prior passes). **79 of sagemaker's 362 inline
structs now remain** (362 minus the running total through parity-20's 104, minus this pass's 25),
confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go` summed, not arithmetic; all five
files now have zero. **New boundary: 3 files tied at 5** (`handler_automl_search.go`,
`handler_experiments.go`, `handler_feature_groups.go`), then an 8-file tier at 4.

**`handler_code_repositories.go` (CodeRepository family) — the most severe finding of this pass,
a destructive-write bug rather than an absence:**

- **`UpdateCodeRepository` replaced the entire stored `GitConfig` map wholesale with whatever the
  client sent**, silently deleting `RepositoryUrl`/`Branch` on any real Update call. The real
  `UpdateCodeRepositoryInput.GitConfig` (`api_op_UpdateCodeRepository.go:35-38`) is
  `*types.GitConfigForUpdate`, which has **only `SecretArn`** (`types.go:9239-9248`) — unlike
  `CreateCodeRepositoryInput.GitConfig` (`*types.GitConfig`, `RepositoryUrl`/`Branch`/`SecretArn`,
  `types.go:9216-9231`). A real client's Update call can never even construct a payload containing
  `RepositoryUrl`, so this backend's own generic `map[string]string` storage silently corrupting
  itself on the first Update was previously invisible to any test that didn't check the field
  survived. Fixed: `UpdateCodeRepository` now takes a bare `secretArn string` and merges only that
  key into the existing map.
- **`CreateCodeRepositoryInput.GitConfig`** (`api_op_CreateCodeRepository.go:44`, required) **and
  its `RepositoryUrl`** (required within it, `types.go:9216-9221`) **were never validated** — a
  request omitting `GitConfig` entirely still succeeded, and this exact gap was itself present in a
  pre-existing test (`TestHandler_UpdateCodeRepository`, which created its fixture repository with
  no `GitConfig` at all). Fixed: both now enforced.
- `ListCodeRepositoriesInput` (`api_op_ListCodeRepositories.go:29-64`) was `NextToken`-only — added
  `CreationTimeAfter`/`Before`, `LastModifiedTimeAfter`/`Before`, `MaxResults`, `NameContains`,
  `SortBy`(`Name`/`CreationTime`/`LastModifiedTime`, default `Name` per the op's own doc),
  `SortOrder` (default `Ascending`, confirmed per-op — the opposite default from this pass's other
  four List ops).

**`handler_compilation_jobs.go` (CompilationJob family) — the second-most severe finding, and the
first family in this campaign to reach this depth of audit (a prior pass fixed only its timestamp
encoding, per PARITY.md's own `monitoring_schedule_workteam_compilation_job` entry):**

- **`StopCompilationJob` set `STOPPED` directly with no `STOPPING` step at all**, contradicting the
  op's own doc text verbatim (`api_op_StopCompilationJob.go:16-19`: "Amazon SageMaker AI changes the
  CompilationJobStatus of the job to Stopping. After Amazon SageMaker stops the job, it sets the
  CompilationJobStatus to Stopped."). No existing test asserted the immediate post-Stop value, so
  nothing caught a client polling immediately after Stop seeing a state one call further along than
  real AWS would ever show. Fixed via a real `Stopping->Stopped` FSM
  (`compilationJobStoppingToStopped`, 150ms), matching this campaign's established convention.
- **`CompilationJobStatus` never advanced past `INPROGRESS` on its own at all** — every sibling
  "job" family in this service (`AIBenchmarkJob`, `AIRecommendationJob`, `TrainingJob`, ...) auto-
  completes after a fixed delay; `CompilationJob` had no such transition, so a job stayed
  `INPROGRESS` forever unless a client explicitly called `StopCompilationJob`. Fixed with a
  `compilationJobInProgressToCompleted` (300ms) FSM matching the established pattern, populating
  `ModelArtifacts` (see below) on completion.
- **`RoleArn`/`OutputConfig`/`StoppingCondition`** (all `This member is required` per
  `api_op_CreateCompilationJob.go:60,98,102`) **were entirely unvalidated** — a request supplying
  only `CompilationJobName` still succeeded, and this exact gap was ratified by five pre-existing
  tests (see below). Fixed: all three now enforced.
- **`ModelArtifacts`** (required, `api_op_DescribeCompilationJob.go:88`) **and `FailureReason`
  were entirely absent from the `CompilationJob` struct** — a required response member permanently
  missing, the same bug class as parity-20's `OptimizationJobSummary` fields. Fixed: `ModelArtifacts`
  (reusing the existing `ModelArtifacts` type already defined in `training_jobs.go`, avoiding a
  duplicate) is populated with a derived `S3ModelArtifacts` once the job reaches `COMPLETED`;
  `FailureReason` added for wire-shape completeness (this backend never simulates a failed
  compilation, so it is always empty — disclosed, not modeled).
- `ModelPackageVersionArn`/`VpcConfig` (both optional) were absent from decode entirely — added as
  passthrough, threaded through Create and echoed on Describe.
- `ListCompilationJobsInput` (`api_op_ListCompilationJobs.go:33-73`) was `NextToken`-only — added
  the same six filter/sort/pagination fields as `ListCodeRepositories` above, with the *same*
  `Name`/`Ascending` defaults (confirmed independently for this op, not assumed from the sibling).

**`handler_app_image_configs.go` (AppImageConfig family) — the third-most severe finding, the
entire real payload of the resource silently dropped:**

- **`KernelGatewayImageConfig`/`JupyterLabAppImageConfig`/`CodeEditorAppImageConfig`** — the three
  mutually-exclusive union members that are the actual configured content of an AppImageConfig
  (`api_op_CreateAppImageConfig.go:28-52`) — **were entirely absent from both Create and Update
  decode.** `CreateAppImageConfig` accepted only a bare name and tags; `UpdateAppImageConfig`
  decoded and applied nothing beyond the name at all, an `UpdateTrainingJob`-class complete no-op.
  Fixed: all three added as `json.RawMessage` passthrough (this backend never runs a real Studio
  app image), threaded through Create/Update, and echoed on Describe/List.
- `ListAppImageConfigsInput` (`api_op_ListAppImageConfigs.go:31-64`) was `NextToken`-only — added
  `CreationTimeAfter`/`Before`, **`ModifiedTimeAfter`/`Before`** (this op's own field name — not
  `LastModifiedTime*` like every sibling List op in this pass, confirmed per-op), `MaxResults`,
  `NameContains`, `SortBy`(`Name`/`CreationTime`/`LastModifiedTime`, default `CreationTime`),
  `SortOrder` (default `Descending`) — and the three config fields are now also emitted per-item in
  `AppImageConfigDetails`, which the real op's response type (`types.go:1961-1985`) declares them on
  too, previously silently omitted from List even though Describe would have shown them (once the
  Describe-side gap above was also fixed).

**`handler_ai_benchmark_jobs.go`/`handler_ai_recommendation_jobs.go` (AIBenchmarkJob/
AIRecommendationJob families) — the same sort-order-default bug found independently in both:**

- **`sortAIBenchmarkJobs`/`sortAIRecommendationJobs` treated an unset `SortBy`/`SortOrder` as
  `Name`/`Ascending`**, but both ops' own doc text states the real default is `CreationTime`/
  `Descending` (`api_op_ListAIBenchmarkJobs.go:44,49`, `api_op_ListAIRecommendationJobs.go:51,55`) —
  the exact reverse. A client calling either List op with no sort parameters got results in the
  wrong order and wrong field. Both fixed to default explicitly rather than falling through a
  switch's zero-value case, verified with a real-SDK-client round-trip test per file.
- `AIRecommendationJob`'s `AdapterSource` (`api_op_CreateAIRecommendationJob.go`'s optional LoRA-
  adapter union) was entirely absent from decode — added as `json.RawMessage` passthrough, threaded
  through Create and echoed on Describe.

**The six questions, answered explicitly:**

1. **What does the handler read that AWS never sends?** Nothing found this pass — every field
   this pass's five handlers decoded does exist on the real request type.
2. **Do request and response use the same key?** Checked separately throughout; one real
   divergence found and preserved correctly by the fix, not created by it: `UpdateCodeRepository`'s
   real request type (`GitConfigForUpdate`) has a strictly narrower key set than `Create`'s
   (`GitConfig`) for the same conceptual field — the bug was treating them as interchangeable, not
   a response/request key mismatch.
3. **Is any required request member never read at all?** `CreateCompilationJob`'s
   `RoleArn`/`OutputConfig`/`StoppingCondition` (all three, decoded but never validated present) —
   `CreateCodeRepository`'s `GitConfig`/`GitConfig.RepositoryUrl` (same pattern) — see above for
   both.
4. **Is any field parsed and then ignored?** `UpdateCodeRepository`'s entire `GitConfig` map was
   parsed and then used to *destructively overwrite* stored state rather than being ignored or
   correctly merged — a new variant of this bug class (accept-and-corrupt rather than accept-and-
   drop), the most severe found this pass. `UpdateAppImageConfig` was the more familiar
   accept-and-drop variant: it decoded nothing beyond the name in the first place, so there was
   nothing to parse-then-ignore — the fields were simply never read.
5. **Does it emit every declared member, and does any handler return a nil body where required
   members are declared?** `DescribeCompilationJob`'s `ModelArtifacts` (required) was completely
   absent from the struct — see above. `ListAppImageConfigs`' three config fields were absent from
   every summary despite the real `AppImageConfigDetails` response type declaring them.
6. **Does any status or lifecycle field ever advance?** `CompilationJobStatus` never left
   `INPROGRESS` on its own at all (the fourth instance of this campaign's stuck-status bug class,
   and the first where the gap was a missing completion path entirely rather than a stalled
   intermediate state) — fixed via FSM. `StopCompilationJob` additionally skipped the `STOPPING`
   intermediate status the op's own doc requires — fixed via a second FSM. `AIBenchmarkJob`'s own
   `StopAIBenchmarkJob`/`scheduleAIBenchmarkJobCompletion` FSMs (established by an earlier pass)
   were re-verified correct and left untouched.

**Timestamps touched, each with its own serializer citation and a test that sets the value:**
`CompilationJob.CompilationStartTime`/`CompilationEndTime` (`api_op_DescribeCompilationJob.go:109-121`,
both optional `*time.Time`, decoded/encoded via the established `epochSecondsPtr`/
`timeFromEpochSecondsPtr` pair, asserted in `TestHandler_CompilationJob_ReachesCompleted_RealClient`);
`ListCompilationJobsInput`/`ListCodeRepositoriesInput`/`ListAppImageConfigsInput`'s four time filters
each (asserted in `TestHandler_ListCompilationJobs`-family and
`TestHandler_ListCodeRepositories_FilterSort_RealClient` — the last of which also exercises
`NameContains`). None left nil in any test.

**Five existing tests found ratifying defects, all from `CreateCompilationJob`'s missing
required-field validation, plus one from `UpdateCodeRepository`'s destructive overwrite:**

1. `TestHandler_CreateCompilationJob`/`TestHandler_DescribeCompilationJob`/
   `TestHandler_StopCompilationJob`/`TestHandler_ListCompilationJobs`/
   `TestCompilationJob_InitialStatus_InProgress` all created a compilation job with only
   `CompilationJobName`(+`RoleArn`), never `OutputConfig`/`StoppingCondition` — every one passed
   only because neither was enforced. Rewritten to supply both; a new
   `TestHandler_CreateCompilationJob_RequiredFieldsEnforced` table test asserts each of
   `RoleArn`/`OutputConfig`/`StoppingCondition` is independently rejected when absent.
2. `TestHandler_UpdateCodeRepository` created its fixture repository with no `GitConfig` at all,
   then updated with `{"Branch": "main"}` — a real UpdateCodeRepositoryInput.GitConfig can never
   even contain `Branch`, and the test never checked the result's `GitConfig` content, so it could
   not have caught the wholesale-overwrite bug even before this pass's Create fix made the fixture
   itself invalid. Rewritten with a real `RepositoryUrl` at Create, a real `SecretArn`-only Update,
   and an explicit assertion that `RepositoryUrl` survives the Update untouched.

**Enums read per op, not generalized:** `ListAIBenchmarkJobsSortBy`/`ListAIRecommendationJobsSortBy`
share the `Name`/`CreationTime`/`Status` set (verified independently, not assumed identical because
of the shared behavior fix — they are, but for two mechanically separate reasons: same doc text,
different files); `AppImageConfigSortKey` (`Name`/`CreationTime`/`LastModifiedTime` — no `Status`,
unlike the two above, since an AppImageConfig has no status field at all);
`CodeRepositorySortBy`/`ListCompilationJobsSortBy` share `Name`/`CreationTime`/`LastModifiedTime`.
No doc-versus-enum mismatch found this pass (contrast parity-20's `ListWorkteamsSortByOptions`).

**Disclosures (not fixed, out of this pass's scope):** `CompilationJob`'s `DerivedInformation`/
`ModelDigests`/`InferenceImage` remain unmodeled — this backend never runs real Neo compilation, so
there is no derived-platform or digest information to report; `ModelArtifacts.S3ModelArtifacts` is
a synthesized path (`<S3OutputLocation>/model.tar.gz`) rather than anything a real compiler wrote,
disclosed the same way `TrainingJob`'s equivalent field already is. `AppImageConfig`'s three kernel
configs are opaque passthrough, not semantically validated (e.g. a client could send a
`KernelGatewayImageConfig` with no `KernelSpecs`, which real AWS validation would reject) — wire-shape
fidelity only, matching this campaign's established convention for deeply-nested union configs.

**Hand-revert proof (four representative fixes, the most severe found this pass):**

1. `UpdateCodeRepository` destructive overwrite: reverted `code_repositories.go`/
   `handler_code_repositories.go` to HEAD, rebuilt clean, ran `TestHandler_UpdateCodeRepository` —
   failed exactly as predicted (`RepositoryUrl` came back `nil` after Update). Restored, `md5sum`
   byte-identical for both files, test passes again.
2. `StopCompilationJob` skipped-`STOPPING` bug: reverted `compilation_jobs.go`/
   `handler_compilation_jobs.go`/`handler_compilation_jobs_test.go` to HEAD (the test file needed a
   matching temporary edit to compile against the old 5-arg `SetCompilationJobExtras`), rebuilt
   clean, ran `TestHandler_StopCompilationJob` — failed exactly as predicted (`STOPPED` immediately,
   never observed `STOPPING`). Restored, `md5sum` byte-identical for all three files, test passes
   again.
3. `CreateCompilationJob` missing required-field validation: removed just the three new `if`
   blocks from `handler_compilation_jobs.go`, rebuilt clean, ran
   `TestHandler_CreateCompilationJob_RequiredFieldsEnforced` — all three subtests failed exactly as
   predicted (`expected: 400, actual: 200`). Restored, `md5sum` byte-identical, test passes again.
4. `ListAIBenchmarkJobs` sort-order default: reverted `sortAIBenchmarkJobs` in `ai_benchmark_jobs.go`
   to its pre-fix body, rebuilt clean, ran
   `TestHandler_ListAIBenchmarkJobs_DefaultSortOrder_RealClient` — failed exactly as predicted
   (results returned `Name`/`Ascending` instead of `CreationTime`/`Descending`). Restored, `md5sum`
   byte-identical, test passes again.

Gates for this session: `go build ./...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./...`, `go vet -tags integration ./...`, `gofmt -l ./services/sagemaker/*.go`
(empty), `go test -race -count=1 ./services/sagemaker/...` (pass), and
`golangci-lint run ./services/sagemaker/...` (0 issues, after fixing 4 goconst/1 golines/2
govet/3 unconvert findings introduced by this pass's own new code — no `nolint` added). No
`fieldalignment -fix` run this pass (no struct-heavy files warranted it).

`last_audit_commit` left at its existing value (`5f91d37c7`) — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**79 of sagemaker's 362 inline structs now remain.** New boundary: 3 files tied at 5
(`handler_automl_search.go`, `handler_experiments.go`, `handler_feature_groups.go`), then a 9-file
tier at 4 (`handler_transform_jobs.go`, `handler_studio_lifecycle_configs.go`,
`handler_processing_jobs.go`, `handler_pipeline_versions.go`, `handler_human_task_ui.go`,
`handler_flow_definitions.go`, `handler_edge_packaging_jobs.go`, `handler_automl.go`,
`handler_ai_workload_configs.go`) — not started this pass; no difficulty found in them, purely an
effort-budget stopping point.

## parity-22 (2026-08-21, gopherstack-oc9v): AutoMLSearch/Experiment/FeatureGroup
field audit (3 files tied at 5, now zero)

Sixteenth pass of the gopherstack-oc9v campaign. Per parity-21's boundary note, this pass took the
three files tied at 5 that parity-21 left unstarted purely for effort-budget reasons:
`handler_automl_search.go`, `handler_experiments.go`, `handler_feature_groups.go`, each verified by
`grep -c 'var req struct {' <file>.go` = 5 before starting. All 15 of this pass's structs were
converted to named types and wire-audited field-by-field against the pinned SDK (`v1.263.2`,
confirmed from `go.mod`). **64 of sagemaker's 362 inline structs now remain** (79 minus this
pass's 15), confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go` summed, not
arithmetic; all three files now have zero. **New boundary: a 9-file tier at 4**
(`handler_transform_jobs.go`, `handler_studio_lifecycle_configs.go`, `handler_processing_jobs.go`,
`handler_pipeline_versions.go`, `handler_human_task_ui.go`, `handler_flow_definitions.go`,
`handler_edge_packaging_jobs.go`, `handler_automl.go`, `handler_ai_workload_configs.go`) — not
started this pass, per this pass's own effort budget rather than any difficulty found in them.

**`handler_feature_groups.go` (FeatureGroup family) — the most severe finding of this pass, two of
three real Update mechanisms silently dropped:**

- **`UpdateFeatureGroupInput.OnlineStoreConfig` and `.ThroughputConfig`** (both optional,
  `api_op_UpdateFeatureGroup.go:38-63`) **were entirely absent from decode** — only `FeatureAdditions`
  was ever read. The op's own doc says a client "can update the online store configuration by using
  the `OnlineStoreConfig` request parameter" and "can switch between on-demand and provisioned modes"
  via `ThroughputConfig`; a real call to either got a 200 and no effect whatsoever. Fixed: added
  `OnlineStoreConfigUpdate`/`ThroughputConfigUpdate` types (mirroring the real
  `types.OnlineStoreConfigUpdate`/`types.ThroughputConfigUpdate`, which are narrower than their
  Create-side counterparts — the same create/update contract-split bug class parity-21's
  `UpdateCodeRepository` fix addressed), merged into the stored config, and added `LastUpdateStatus`
  (`types.LastUpdateStatus`, always `Successful` since every update here is synchronous — disclosed,
  not modeled as a real async FSM).
- **`CreateFeatureGroupInput.EventTimeFeatureName`/`FeatureDefinitions`/`RecordIdentifierFeatureName`**
  (all three `This member is required`, `api_op_CreateFeatureGroup.go:29-119`) **were never
  validated** — a request supplying only `FeatureGroupName` still succeeded, and this exact gap was
  ratified by four pre-existing tests, one of them via a typo (see below). Fixed: all three now
  enforced.
- **`DescribeFeatureGroupOutput.NextToken`** (`This member is required`,
  `api_op_DescribeFeatureGroup.go:60-63`) **was completely absent from the response** — the same bug
  class as parity-20's `OptimizationJobSummary` fields and parity-21's `CompilationJob.ModelArtifacts`.
  Fixed: always emitted (empty string — this backend never paginates `FeatureDefinitions`, the field
  it controls on the real op, disclosed rather than silently accepted-and-dropped).
- `FeatureGroup` had no `LastModifiedTime` field at all (only `CreationTime`) despite
  `DescribeFeatureGroupOutput` declaring one — added, set on Create and bumped on every Update.
  `OfflineStoreStatus` (`Active`/`Blocked`/`Disabled`) was likewise absent — added as a static
  `Active` once an `OfflineStoreConfig` is configured (this backend never simulates offline
  replication failure, disclosed).
- `ListFeatureGroupsInput` (`api_op_ListFeatureGroups.go:29-64`) was `NextToken`-only — added
  `CreationTimeAfter`/`Before`, `FeatureGroupStatusEquals`, `MaxResults`, `NameContains`,
  `OfflineStoreStatusEquals`, `SortBy`/`SortOrder`. Unlike every List op parity-21 touched, this op's
  own doc states no explicit SortBy/SortOrder default, so the pre-existing `Name`/`Ascending`
  behavior was kept rather than invented.

**`handler_automl_search.go` (AutoML candidates / Search / model metadata family) — the second
finding, and the one that turned up a pre-existing wire-format bug outside this pass's own new
code:**

- **`Search`'s `SortBy`/`SortOrder` were decoded by the handler and then never forwarded to the
  backend at all** — a real request specifying either had no effect, the exact "parsed and ignored"
  bug class. Fixed: both now reach `InMemoryBackend.Search`, which sorts by the named property
  (default `LastModifiedTime`/`Descending` per `api_op_Search.go:56,60`) after the existing key-based
  stable sort for determinism among ties.
- **Fixing that surfaced a second, independent, pre-existing bug**: `Search`'s `TrainingJob`/`Pipeline`
  results were emitted via a direct `json.Marshal` of the raw stored struct (`toJSONFlatMap`, then
  stored as-is on the response). Neither `TrainingJob` nor `Pipeline` has a custom `MarshalJSON`
  (unlike `CompilationJob`), so `CreationTime`/`LastModifiedTime`/etc. serialized as Go's default
  RFC3339 strings instead of the epoch-second JSON numbers the real awsjson1.1 protocol requires —
  a real SDK client's `Search` call against either of the only two fully-supported resource types
  failed outright with "expected Timestamp to be a JSON Number, got string instead". This was caught
  by this pass's own new `TestHandler_Search_SortByAndCrossAccount_RealClient`, not found by
  inspection. Fixed: added `trainingJobSearchView`/`pipelineSearchView`, reusing
  `addTrainingJobOptionalFields` (already used by `DescribeTrainingJob`) so there is one source of
  truth for the epoch-safe shape rather than a second field list to drift.
- `SearchInput.CrossAccountFilterOption` was absent from decode — added; `CrossAccount` now
  correctly returns zero results, since this single-tenant backend models no other account's
  resources to discover. `VisibilityConditions` remains undecoded and disclosed unmodeled — this
  service has no per-resource caller-visibility ACL concept anywhere, the same reasoning already
  applied to `CreatedBy`/`LastModifiedBy` (`types.UserContext`) elsewhere in this service.
  `SearchOutput.TotalHits` was absent — added (`Relation: EqualTo`, since this backend's counts are
  always exact).
- **`GetScalingConfigurationRecommendationInput.ScalingPolicyObjective`** (optional,
  `api_op_GetScalingConfigurationRecommendation.go:27-53`) **was entirely absent from decode** — the
  real response echoes it back verbatim ("An object representing the anticipated traffic pattern...
  that you specified in the request"), so a real client got a response with the field simply missing.
  Fixed: decoded and echoed; `GetScalingConfigurationRecommendationOutput.Metric` (also previously
  absent) added with disclosed synthesized values (this backend never benchmarks a real endpoint).
- `ListCandidatesForAutoMLJobInput.SortBy`/`SortOrder` were absent from decode — added
  (`CandidateSortBy`: `CreationTime`/`Status`/`FinalObjectiveMetricValue`). This op's own doc text on
  the `SortBy` field reads "The default is Descending" — not a valid `CandidateSortBy` value at all,
  so that half of the doc is corrupted copy-paste (see Enums below) and not trusted; the `SortOrder`
  field's doc ("The default is Ascending") is internally consistent and taken at face value.

**`handler_experiments.go` (Experiment family) — the third finding, an unreachable "remove"
operation the op's own doc promises:**

- **`UpdateExperimentInput.DisplayName`/`Description`** (`api_op_UpdateExperiment.go:28-43`, both
  `*string` on the real type) **were decoded as plain non-pointer strings**, so an omitted key and an
  explicit `""` were indistinguishable — the op's own doc says it "adds, updates, or **removes** the
  description", but removal (sending an explicit empty string) was silently treated as "no change".
  Fixed: both now `*string`, `nil` meaning unchanged and a present `""` meaning clear, matching the
  established convention already used by `UpdateImage`/`UpdateHub` elsewhere in this service.
- `ListExperimentsInput` (`api_op_ListExperiments.go:32-55`) was `NextToken`-only — added
  `CreatedAfter`/`CreatedBefore`, `SortBy`(`Name`/`CreationTime`, default `CreationTime` per the
  op's own doc), `SortOrder` (default `Descending`), `MaxResults`. Previously every List call
  returned `Name`/`Ascending` order regardless of request or default.
- `DescribeExperimentOutput.CreatedBy`/`LastModifiedBy` (`types.UserContext`) and `.Source`
  (`types.ExperimentSource`) remain disclosed absent — this service models no caller-identity
  concept, and experiments here are always created directly rather than derived from another
  resource (e.g. a Pipeline execution).

**The six questions, answered explicitly:**

1. **What does the handler read that AWS never sends?** Nothing found this pass — every field
   decoded by this pass's converted handlers exists on the real request type.
2. **Do request and response use the same key?** Checked separately throughout; no divergence found
   this pass (contrast parity-21's `UpdateCodeRepository`/`CreateCodeRepository` `GitConfig` split).
3. **Is any required request member never read at all?** `CreateFeatureGroup`'s
   `EventTimeFeatureName`/`FeatureDefinitions`/`RecordIdentifierFeatureName` (all three, decoded but
   never validated present) — see above.
4. **Is any field parsed and then ignored, or worse, applied destructively?** `Search`'s
   `SortBy`/`SortOrder` were parsed and then dropped before reaching the backend (a real, if
   comparatively mild, instance of this bug class — no data was corrupted, only ordering silently
   ignored). `UpdateFeatureGroup`'s `OnlineStoreConfig`/`ThroughputConfig` were the more severe
   accept-and-drop variant: not even parsed, so two of the op's three real update mechanisms were
   invisible to the handler entirely.
5. **Does it emit every declared member, and does any handler return a nil body where required
   members are declared?** `DescribeFeatureGroup`'s `NextToken` (required) was completely absent
   from the response — see above. No nil-body case found this pass.
6. **Does any status or lifecycle field ever advance?** Checked: `FeatureGroupStatus` is set once at
   Create (`Created`) and never transitions through a `Creating` intermediate the real async
   provisioning doc implies, but this pass did not add a new FSM for it — declined this pass, see
   Disclosures. `LastUpdateStatus` (new this pass) is always `Successful` synchronously rather than
   passing through `InProgress`, disclosed rather than modeled as a real async FSM. No stuck-forever
   status found (contrast parity-21's fourth/fifth instances on `CompilationJob`).

**Timestamps touched, each with its own serializer citation and a test that sets the value:**
`ListExperimentsInput.CreatedAfter`/`CreatedBefore` (`api_op_ListExperiments.go:34-38`, decoded via
`epochPtr`, asserted in `TestHandler_ListExperiments_CreatedAfterFilter_RealClient`);
`ListFeatureGroupsInput.CreationTimeAfter`/`CreationTimeBefore`
(`api_op_ListFeatureGroups.go:33-37`, same pattern, asserted in
`TestHandler_ListFeatureGroups_FilterSort_RealClient`, which also exercises `NameContains`/`SortBy`).
The `TrainingJob`/`Pipeline` Search-view fix above is also a timestamp fix in substance (RFC3339
string to epoch-seconds number), asserted by
`TestHandler_Search_SortByAndCrossAccount_RealClient` — a real-SDK-client test that would have
failed to deserialize the response at all before the fix (confirmed by hand-revert, see below).

**Two existing tests found ratifying the missing-required-field-validation defect, plus one earlier
still hiding it via a typo:** `TestHandler_FeatureGroup_Duplicate` and
`TestHandler_Tags_FeatureGroup`/`TestHandler_CreateFeatureGroup_RoleArnAndDescription` created
feature groups with `EventTimeFeatureName`/`RecordIdentifierFeatureName` present but no
`FeatureDefinitions` at all — passed only because the field wasn't enforced.
`TestHandler_FeatureGroupLifecycle` (and, independently, `TestHandler_UpdateAndDescribeFeatureMetadata`
in `handler_feature_metadata_test.go`, a file outside this pass's own scope but broken by this pass's
new validation) sent `"RecordIdentifierFeatureDefinition"` — a typo'd key that is not
`RecordIdentifierFeatureName` at all — so the real required field was silently never sent, and the
test still passed because nothing validated its presence. All four rewritten to supply valid
required fields; a new `TestHandler_CreateFeatureGroup_RequiredFieldsEnforced` table test asserts
each of the three required fields is independently rejected when absent.

**Enums read per op, not generalized:** `CandidateSortBy`
(`CreationTime`/`Status`/`FinalObjectiveMetricValue` — see the doc-vs-source mismatch on its own
`SortBy` field's default-value prose, noted above); `SortExperimentsBy` (`Name`/`CreationTime`, no
`Status` option, unlike `CandidateSortBy`); `FeatureGroupSortBy`
(`Name`/`FeatureGroupStatus`/`OfflineStoreStatus`/`CreationTime` — confirmed independently, not
assumed from any sibling); `FeatureGroupStatus`/`OfflineStoreStatusValue`/`LastUpdateStatusValue`
read for the new fields added this pass. `ThroughputMode`'s real values are `OnDemand`/`Provisioned`
(camelCase) — this backend's `ThroughputConfig.ThroughputMode` remains opaque string passthrough
with no enum validation (matching parity-21's `AppImageConfig` kernel-config precedent), and the
pre-existing round-trip test at `handler_feature_groups_stores_test.go:35` sends `"PROVISIONED"`
(uppercase), which is not a real SDK value — disclosed, not fixed, since fixing it would mean adding
enum validation this campaign has not applied to any other opaque passthrough config.

**Disclosures (not fixed, out of this pass's scope):** `FeatureGroupStatus` never transitions
through a `Creating` intermediate state before landing on `Created` — real AWS provisioning is
documented as taking "approximately 10-15 minutes" for an `InMemory` online store, implying an
async `Creating` window this backend skips entirely by setting `Created` synchronously at Create
time. Not fixed this pass (an effort-budget decision, not a difficulty found) — this is the same
bug class as parity-21's `CompilationJob` stuck-status fixes and is flagged as the natural next step
for `FeatureGroup` specifically. `DescribeFeatureGroupOutput.FailureReason` remains unmodeled (this
backend never fails an offline-store replication). `Search`'s `VisibilityConditions` and
`GetScalingConfigurationRecommendation`'s synthesized `Metric` values are disclosed above.

**Hand-revert proof (three representative fixes, the most severe found this pass):**

1. `UpdateFeatureGroup` accept-and-drop on `OnlineStoreConfig`/`ThroughputConfig`: reverted
   `feature_groups.go`/`handler_feature_groups.go` to HEAD, rebuilt clean, ran
   `TestHandler_UpdateFeatureGroup_StoreConfigs_RealClient` — failed exactly as predicted
   (`OnlineStoreConfig` came back `nil` after Update). Restored, `md5sum` byte-identical for both
   files, test passes again.
2. `Search`'s raw-struct timestamp bug: with `automl_search.go` otherwise at its post-fix state (the
   fix spans the same file as several others this pass, so a whole-file revert does not build
   standalone), reverted just the one line storing `raw: view` back to `raw: tj` for the
   `TrainingJob` case, rebuilt clean, ran `TestHandler_Search_SortByAndCrossAccount_RealClient` —
   failed exactly as predicted (`deserialization failed... expected Timestamp to be a JSON Number,
   got string instead`). Restored, `md5sum` byte-identical, test passes again.
3. `UpdateExperiment` unreachable description-clear: reverted `experiments.go`/
   `handler_experiments.go` to HEAD (both revert cleanly together since this fix's parts live only
   in these two files), rebuilt clean, ran
   `TestHandler_UpdateExperiment_ClearsDescription_RealClient` — failed exactly as predicted (an
   explicit empty-string `Description` update left the original value in place). Restored, `md5sum`
   byte-identical for both files, test passes again.

Gates for this session: `go build ./...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./...`, `go vet -tags integration ./...`, `gofmt -l ./services/sagemaker/*.go`
(empty), `go test -race -count=1 ./services/sagemaker/...` (pass), and
`golangci-lint run ./services/sagemaker/...` (0 issues, after fixing 11 goconst findings — several
in pre-existing files whose own already-repeated literals crossed goconst's threshold only once this
pass's new code added one more occurrence — 1 golines, 4 govet/fieldalignment, and 1 nonamedreturns
finding, all introduced by this pass's own new code; no `nolint` added). No `fieldalignment -fix` or
`golangci-lint --fix` run (both would run package-wide); each fieldalignment struct was reordered by
hand after reading the analyzer's plain-mode (non-`-fix`) output.

`last_audit_commit` left at its existing value (`5f91d37c7`) — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**64 of sagemaker's 362 inline structs now remain.** New boundary: a 9-file tier at 4
(`handler_transform_jobs.go`, `handler_studio_lifecycle_configs.go`, `handler_processing_jobs.go`,
`handler_pipeline_versions.go`, `handler_human_task_ui.go`, `handler_flow_definitions.go`,
`handler_edge_packaging_jobs.go`, `handler_automl.go`, `handler_ai_workload_configs.go`) — not
started this pass, purely an effort-budget stopping point.

## parity-23 (2026-08-21, gopherstack-oc9v): StudioLifecycleConfig/HumanTaskUI/
FlowDefinition field audit (3 files tied at 4, now zero)

Seventeenth pass of the gopherstack-oc9v campaign. Per parity-22's boundary note, this pass took
three of the nine files tied at 4 that parity-22 left unstarted purely for effort-budget reasons:
`handler_studio_lifecycle_configs.go`, `handler_human_task_ui.go`, `handler_flow_definitions.go`,
each verified by `grep -c 'var req struct {' <file>.go` = 4 before starting. All 12 of this pass's
structs were converted to named types and wire-audited field-by-field against the pinned SDK
(`v1.263.2`, confirmed from `go.mod`). **52 of sagemaker's 362 inline structs now remain** (64 minus
this pass's 12), confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go` summed, not
arithmetic; all three files now have zero. **New boundary: a 6-file tier at 4**
(`handler_transform_jobs.go`, `handler_processing_jobs.go`, `handler_pipeline_versions.go`,
`handler_edge_packaging_jobs.go`, `handler_automl.go`, `handler_ai_workload_configs.go`) — not
started this pass, per this pass's own effort budget rather than any difficulty found in them.

**`handler_flow_definitions.go` — the most severe finding of this pass, a required member and
three entire optional config families never read at all:**

- **`CreateFlowDefinitionInput.OutputConfig`** (`This member is required`,
  `api_op_CreateFlowDefinition.go:28-66`, `types.FlowDefinitionOutputConfig`) **was entirely absent
  from decode** — only `FlowDefinitionName` and `RoleArn` were ever read. A real client's flow
  definition create request always carries `OutputConfig.S3OutputPath` (also required); it was
  silently discarded and Create succeeded anyway, where real AWS would reject the request outright.
  Fixed: required, stored, and echoed by Describe (nested under `OutputConfig`, matching the real
  response shape — `KmsKeyId` optional and round-tripped too).
- **`HumanLoopActivationConfig`/`HumanLoopConfig`/`HumanLoopRequestSource`** (all optional but, once
  present, each carrying their own required sub-members — `types.HumanLoopActivationConfig`
  `types.types.go:9445-9454`, `types.HumanLoopConfig:9457-9560`,
  `types.HumanLoopRequestSource:9722-9732`) **were entirely absent from decode** — every real
  flow-definition-specific setting (what triggers a human loop, who reviews and what they see, which
  managed AWS integration source populates it) was silently discarded on every Create. Fixed: added
  `FlowDefinitionOptions`/`HumanLoopConfig` backend types mirroring the real shapes field-for-field
  (`HumanTaskUiArn`/`WorkteamArn`/`TaskTitle`/`TaskDescription`/`TaskCount` required,
  `TaskAvailabilityLifetimeInSeconds`/`TaskTimeLimitInSeconds`/`TaskKeywords` optional); all three
  now decode, store, and round-trip through Describe via a real-SDK-client test
  (`TestHandler_DescribeFlowDefinition_HumanLoopConfig_RealClient`).
  `HumanLoopConfig.PublicWorkforceTaskPrice` remains unmodeled — this backend never simulates paid
  Mechanical Turk work, disclosed, same reasoning already applied elsewhere in this campaign to
  other billing/pricing sub-structures.
- `ListFlowDefinitionsInput` (`api_op_ListFlowDefinitions.go:30-52`) was `NextToken`-only — added
  `CreationTimeAfter`/`CreationTimeBefore`/`MaxResults`/`SortOrder`. The op declares no `SortBy`
  field at all (only `SortOrder`) and its own doc states no default order either way; rather than
  invent a sort dimension (e.g. `CreationTime`) no doc supports, `SortOrder` is applied to this
  backend's own pre-existing default order (ascending by name, the accidental byproduct of its
  former name-keyed pagination) — the same conservative stance parity-22's `ListFeatureGroups` took
  on its own undocumented `SortBy`/`SortOrder` defaults.

**`handler_human_task_ui.go` — the second finding, a required Create member never read and a
required Describe member never emitted:**

- **`CreateHumanTaskUiInput.UiTemplate`** (`This member is required`,
  `api_op_CreateHumanTaskUi.go:29-46`, `types.UiTemplate{Content}`) **was entirely absent from
  decode** — a client's worker-facing Liquid template was silently discarded and Create succeeded
  anyway. Fixed: `UiTemplate.Content` now required and hashed at Create time
  (`sha256.Sum256`/`hex.EncodeToString`, same pattern already used by
  `services/codeartifact/handler_package_versions.go:555-556` for a content digest).
- **`DescribeHumanTaskUiOutput.UiTemplate`** (`This member is required`,
  `api_op_DescribeHumanTaskUi.go:60-63`, `types.UiTemplateInfo{ContentSha256, Url}`) **was
  completely absent from the response** — real AWS never returns the raw template on Describe, only
  its digest, so only the digest is retained (`HumanTaskUI.UITemplateContentSha256`, emitted nested
  under `"UiTemplate"` by `HumanTaskUI.MarshalJSON`). `UiTemplateInfo.Url` remains unpopulated,
  disclosed — this backend hosts no template-rendering URL.
- `ListHumanTaskUisInput` (`api_op_ListHumanTaskUis.go:30-53`) was `NextToken`-only — added
  `CreationTimeAfter`/`CreationTimeBefore`/`MaxResults`/`SortOrder`, same undocumented-default
  reasoning as `ListFlowDefinitions` above (they are the only two ops in this pass with a bare
  `SortOrder` and no `SortBy` at all).

**`handler_studio_lifecycle_configs.go` — the third finding, a required Create member never
validated and a List op's entire filter/sort surface silently dropped:**

- **`CreateStudioLifecycleConfigInput.StudioLifecycleConfigAppType`** (`This member is required`,
  `api_op_CreateStudioLifecycleConfig.go:28-52`) was decoded and passed through, but **never
  validated present** — a request supplying only `StudioLifecycleConfigName`/`Content` still
  succeeded with an empty `AppType`, where real AWS would reject it. Fixed: now enforced in the
  handler, matching the required-field-validation convention parity-22 established for
  `CreateFeatureGroup`.
- **`ListStudioLifecycleConfigsInput`** (`api_op_ListStudioLifecycleConfigs.go:31-75`) was
  `NextToken`-only — added `AppTypeEquals`/`CreationTimeAfter`/`CreationTimeBefore`/
  `ModifiedTimeAfter`/`ModifiedTimeBefore`/`NameContains`/`SortBy`/`SortOrder`/`MaxResults`. Unlike
  the two ops above, this op's own doc states explicit defaults (`SortBy` default `CreationTime`,
  `SortOrder` default `Descending`), so those are honored rather than the conservative
  keep-prior-order stance used elsewhere this pass. `StudioLifecycleConfigDetails.
  StudioLifecycleConfigAppType` (a real, always-present List-item field) was also missing from the
  emitted list items — added.

**The six questions, answered explicitly:**

1. **What does the handler read that AWS never sends?** Nothing found this pass — every field
   decoded by this pass's converted handlers exists on the real request type.
2. **Do request and response use the same key?** Checked separately throughout; no divergence found
   this pass.
3. **Is any required request member never read at all?** Yes, twice, both the most severe class:
   `CreateFlowDefinitionInput.OutputConfig` and `CreateHumanTaskUiInput.UiTemplate` — see above.
   `CreateStudioLifecycleConfigInput.StudioLifecycleConfigAppType` was read but never validated
   present, the adjacent-but-distinct defect class parity-22 also found on `CreateFeatureGroup`.
4. **Is any field parsed and then ignored, or worse, applied destructively?** Not found this pass —
   every gap found was accept-and-drop (never decoded at all), not decode-then-discard.
5. **Does it emit every declared member, and does any handler return a nil body where required
   members are declared?** `DescribeHumanTaskUiOutput.UiTemplate` (required) was completely absent
   from the response — see above, same bug class as parity-22's `DescribeFeatureGroup.NextToken`.
   `StudioLifecycleConfigDetails.StudioLifecycleConfigAppType` was likewise missing from
   `ListStudioLifecycleConfigs`' items. No nil-body case found.
6. **Does any status or lifecycle field ever advance?** Checked: `FlowDefinitionStatus` and
   `HumanTaskUiStatus` are each set once at Create (`Active`) and never transition — real AWS's
   `FlowDefinitionStatus` enum also declares `Initializing`/`Failed`/`Deleting`
   (`types/enums.go:3758-3766`), implying an async provisioning window this backend skips entirely.
   Not fixed this pass (an effort-budget decision, not a difficulty found) — flagged as the natural
   next step for `FlowDefinition` specifically, same bug class as parity-21/22's stuck-status
   findings on `CompilationJob`/`FeatureGroup`.

**Timestamps touched, each with its own serializer citation and a test that sets the value:**
`ListFlowDefinitionsInput.CreationTimeAfter`/`CreationTimeBefore`
(`api_op_ListFlowDefinitions.go:34-38`, decoded via `epochPtr`, asserted implicitly by
`TestHandler_ListFlowDefinitions_FilterSort_RealClient`'s ordering); `ListHumanTaskUisInput.
CreationTimeAfter`/`CreationTimeBefore` (`api_op_ListHumanTaskUis.go:34-38`, same pattern);
`ListStudioLifecycleConfigsInput.CreationTimeAfter`/`CreationTimeBefore`/`ModifiedTimeAfter`/
`ModifiedTimeBefore` (`api_op_ListStudioLifecycleConfigs.go:37-57`), all proven via
`TestHandler_ListStudioLifecycleConfigs_FilterSort_RealClient`, a real-SDK-client test. No
pre-existing timestamp-encoding bugs found this pass — all three types already had a correct
`MarshalJSON`/`UnmarshalJSON` epoch-seconds pair from the systemic-timestamp-bug fix (see Notes:
above, "Affected types (Describe path)"), which this pass extended rather than repaired.

**Enums read per op, not generalized:** `StudioLifecycleConfigAppType`
(`JupyterServer`/`KernelGateway`/`CodeEditor`/`JupyterLab` — `types/enums.go:9552-9560`);
`StudioLifecycleConfigSortKey` (`CreationTime`/`LastModifiedTime`/`Name` — confirmed independently,
not assumed from any sibling sort-key enum); `AwsManagedHumanLoopRequestSource`
(`AWS/Rekognition/DetectModerationLabels/Image/V3`/`AWS/Textract/AnalyzeDocument/Forms/V1` —
`types/enums.go:1513-1519`, both real ARN-shaped string values, not simple names); `SortOrder`
(shared `Ascending`/`Descending`, `types/enums.go:9240-9246`) used by both `ListFlowDefinitions` and
`ListHumanTaskUis`, each independently confirmed to declare no companion `SortBy` field at all.

**Disclosures (not fixed, out of this pass's scope):** `HumanLoopConfig.PublicWorkforceTaskPrice`
and `FlowDefinition`'s `Initializing`/`Failed` status transitions, both above.
`HumanLoopActivationConditions`/`AwsManagedHumanLoopRequestSource` are stored and echoed as opaque
strings/enum values respectively rather than semantically interpreted — this backend never
evaluates a human-loop activation condition or calls a real Rekognition/Textract integration,
disclosed, matching the established convention for opaque passthrough config elsewhere in this
campaign.

**Existing tests found ratifying the two missing-required-field-validation defects:**
`TestHandler_CreateFlowDefinition`/`TestHandler_DescribeFlowDefinition`/
`TestHandler_ListFlowDefinitions` created flow definitions with no `RoleArn` and no `OutputConfig`
at all — passed only because neither was enforced; `handler_create_tags_test.go`'s own
`CreateFlowDefinition` call already supplied both correctly and did not need changing.
`TestHandler_CreateHumanTaskUI`/`TestHandler_DescribeHumanTaskUI`/`TestHandler_DeleteHumanTaskUI`/
`TestHandler_ListHumanTaskUIs` created human task UIs with no `UiTemplate` at all — passed only
because it was never read. All four rewritten to supply valid required fields; new
`TestHandler_CreateFlowDefinition_RequiredFieldsEnforced`/
`TestHandler_CreateHumanTaskUI_RequiredFieldsEnforced`/
`TestHandler_CreateStudioLifecycleConfig_RequiredFieldsEnforced` table tests assert each required
field is independently rejected when absent.

**Hand-revert proof (all three fixes, one per file, each the most severe finding in that file):**

1. `CreateFlowDefinition`'s `OutputConfig`/`HumanLoop*` accept-and-drop: reverted
   `flow_definitions.go`/`handler_flow_definitions.go` to HEAD, rebuilt clean (the test file itself
   only calls the real SDK client, so it compiles unchanged against either version), ran
   `TestHandler_CreateFlowDefinition_RequiredFieldsEnforced`/
   `TestHandler_DescribeFlowDefinition_HumanLoopConfig_RealClient`/
   `TestHandler_ListFlowDefinitions_FilterSort_RealClient` — all failed exactly as predicted (200
   instead of 400 on three missing-required-field cases; `HumanLoopActivationConfig` nil after
   Describe; List returned unfiltered/unsorted results). Restored, `md5sum` byte-identical for both
   files, all tests pass again.
2. `CreateHumanTaskUI`'s `UiTemplate` never read: reverted `human_task_ui.go`/
   `handler_human_task_ui.go` to HEAD, rebuilt clean, ran
   `TestHandler_CreateHumanTaskUI_RequiredFieldsEnforced`/
   `TestHandler_DescribeHumanTaskUI_UiTemplateContentSha256_RealClient`/
   `TestHandler_ListHumanTaskUIs_SortOrder_RealClient` — all failed exactly as predicted (200
   instead of 400; `UiTemplate` nil on Describe; List unsorted). Restored, `md5sum` byte-identical
   for both files, all tests pass again.
3. `ListStudioLifecycleConfigs`' filter/sort accept-and-drop plus `AppType` unvalidated: reverted
   `studio_lifecycle_configs.go`/`handler_studio_lifecycle_configs.go` to HEAD, rebuilt clean, ran
   `TestHandler_CreateStudioLifecycleConfig_RequiredFieldsEnforced`/
   `TestHandler_ListStudioLifecycleConfigs_FilterSort_RealClient` — failed exactly as predicted (200
   instead of 400 on missing `AppType`; List returned 3 items instead of 2, ignoring
   `AppTypeEquals`). Restored, `md5sum` byte-identical for both files, all tests pass again.

Gates for this session: `go build ./...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./...`, `go vet -tags integration ./...`, `gofmt -l ./services/sagemaker/*.go`
(empty), `go test -race -count=1 ./services/sagemaker/...` (pass), and
`golangci-lint run ./services/sagemaker/...` (0 issues, after fixing: 1 `dupl` — extracted a shared
`filterSortPaginateByNameWindow` generic helper in `list_helpers.go` for the two undocumented-
SortBy-default List ops rather than leaving near-identical filter/sort/paginate bodies in both
files; 3 `goconst` — a bare `"Name"` string literal this pass's own new `switch` case added crossed
the existing `keyGenericName` constant's occurrence threshold in two other pre-existing files
(`list_helpers.go`, `pipelines.go`) that had not previously tripped it, same "pass's new code pushed
a pre-existing literal over threshold" pattern parity-22 also hit; 3 `golines`; 6 `govet`/
`fieldalignment` — each struct reordered by hand (pointer-containing fields grouped first, then
scalar fields, then string-plus-int32 config-carrier structs last), confirmed via non-`-fix`
`fieldalignment ./services/sagemaker/...` output rather than the package-wide `-fix`; 2 `revive`
var-naming — `UiTemplate`/`UiTemplateContentSha256` Go field names renamed to `UITemplate`/
`UITemplateContentSha256` per Go initialism convention (JSON tags unchanged, still `"UiTemplate"`
to match the real wire key); 2 `staticcheck` S1016 — `ListFlowDefinitionsFilter`/
`ListHumanTaskUIsFilter` converted directly to `nameWindowSortParams` via a type conversion rather
than a field-by-field literal, since their field sets are now identical; no `nolint` added). No
`fieldalignment -fix` or `golangci-lint --fix` run (both would run package-wide).

`last_audit_commit` left at its existing value (`5f91d37c7`) — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**52 of sagemaker's 362 inline structs now remain.** New boundary: a 6-file tier at 4
(`handler_transform_jobs.go`, `handler_processing_jobs.go`, `handler_pipeline_versions.go`,
`handler_edge_packaging_jobs.go`, `handler_automl.go`, `handler_ai_workload_configs.go`) — not
started this pass, purely an effort-budget stopping point.

## parity-24 (2026-08-21, gopherstack-oc9v): TransformJob/ProcessingJob/PipelineVersions/
EdgePackagingJob/AutoML/AIWorkloadConfig field audit (the six-file tier at 4, now zero)

Eighteenth pass of the gopherstack-oc9v campaign. Per parity-23's boundary note, this pass took
the entire six-file tier it left unstarted: `handler_transform_jobs.go`, `handler_processing_jobs.go`,
`handler_pipeline_versions.go`, `handler_edge_packaging_jobs.go`, `handler_automl.go`,
`handler_ai_workload_configs.go`, each verified by `grep -c 'var req struct {' <file>.go` = 4 before
starting. All 24 structs converted to named types and wire-audited field-by-field against the pinned
SDK (`v1.263.2`, confirmed from `go.mod`). **52 minus this pass's 24 leaves 28 remaining**,
confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go` summed, not arithmetic. **New
boundary: a 6-file tier at 3** (`handler_tags.go`, `handler_presigned_session.go`,
`handler_monitoring.go`, `handler_models.go`, `handler_endpoint_configs.go`,
`handler_algorithms.go`) — not started this pass, per this pass's own effort budget.

**`handler_edge_packaging_jobs.go` — the most severe finding of this pass, a required member
entirely absent from decode, storage, and Describe:**

- **`CreateEdgePackagingJobInput.OutputConfig`** (`This member is required`,
  `api_op_CreateEdgePackagingJob.go:13-52`, `types.EdgeOutputConfig{S3OutputLocation required,
  KmsKeyId/PresetDeploymentConfig/PresetDeploymentType optional}`) had no field for it anywhere —
  a client's packaging-output S3 destination was silently discarded and Create succeeded anyway.
  Fixed: required, stored (`EdgePackagingJob.OutputConfig`), and returned by Describe.
- `ModelName`/`ModelVersion`/`RoleArn`/`CompilationJobName` are each `This member is required` too,
  but only `EdgePackagingJobName` was ever validated present — the same
  accepted-but-never-validated defect class parity-22/23 found on
  `CreateFeatureGroup`/`CreateStudioLifecycleConfig`. All four now enforced.
- `ListEdgePackagingJobsInput` (`api_op_ListEdgePackagingJobs.go:30-58`) was
  `StatusEquals`/`NameContains`/`NextToken`-only — added `CreationTimeAfter`/`CreationTimeBefore`/
  `LastModifiedTimeAfter`/`LastModifiedTimeBefore`/`ModelNameContains`/`SortBy`/`SortOrder`/
  `MaxResults`. Neither `SortBy` nor `SortOrder` documents a default on this op, so an unset value
  keeps this backend's pre-existing ascending-by-name order rather than inventing one, the
  conservative stance parity-23 established for `ListFlowDefinitions`/`ListHumanTaskUis`.
  `SortBy`'s five real values (`NAME`/`MODEL_NAME`/`CREATION_TIME`/`LAST_MODIFIED_TIME`/`STATUS`,
  `types/enums.go:5332-5341`) are each handled explicitly.
- `ResourceKey` (real, optional `CreateEdgePackagingJobInput` field) was also accept-and-drop — now
  stored and returned.

**`handler_transform_jobs.go` — the second finding, a fabricated field this handler invented out
of nothing:**

- **`RoleArn`** was decoded on `CreateTransformJob`, stored on `TransformJob`, and echoed by
  `DescribeTransformJob` — but `CreateTransformJobInput`/`DescribeTransformJobOutput`
  (`api_op_CreateTransformJob.go:55-166`, `api_op_DescribeTransformJob.go`) have **no such field at
  all**. No real client would ever populate it, so the practical blast radius was small, but it is
  exactly the "handler reads what AWS never sends" class this campaign's six questions ask about —
  and it round-tripped through Describe as though it were real. Removed entirely (decode, options,
  storage field, Describe emission), proven by
  `TestHandler_CreateTransformJob_RoleArnNotPartOfWireShape`, which supplies `RoleArn` on Create and
  asserts it is absent from Describe's response.
- `TransformInput.DataSource.S3DataSource.S3Uri`/`TransformOutput.S3OutputPath`/
  `TransformResources.InstanceType`+`InstanceCount` are each required (alongside the
  already-checked `ModelName`/`TransformJobName`) but none were validated present — fixed.
- `ListTransformJobsInput` (`api_op_ListTransformJobs.go`) had `StatusEquals`/`NameContains` but no
  `CreationTimeAfter`/`CreationTimeBefore`/`LastModifiedTimeAfter`/`LastModifiedTimeBefore`/
  `SortBy`/`SortOrder`/`MaxResults` — added, `SortBy` default `CreationTime`/`SortOrder` default
  `Descending` per that op's own doc.

**`handler_processing_jobs.go` — the third finding, a request field nested under the wrong key
entirely:**

- **`CreateProcessingJobInput`'s VPC settings nest under `NetworkConfig.VpcConfig`**
  (`types.NetworkConfig{VpcConfig, EnableInterContainerTrafficEncryption,
  EnableNetworkIsolation}`) — there is **no top-level `VpcConfig` field on the real request at
  all**. This handler decoded exactly that nonexistent top-level key, so a real client's
  VPC-isolated processing job silently lost its network settings, and the accepted key was dead
  code no real client would ever send. Fixed: `NetworkConfig` now nests `VpcConfig` plus both real
  boolean flags, proven by `TestHandler_CreateProcessingJob_NetworkConfig_RealClient`, a real
  `aws-sdk-go-v2` client round-trip.
- `RoleArn`/`AppSpecification`/`ProcessingResources` are each `This member is required`
  (`api_op_CreateProcessingJob.go`) but only `ProcessingJobName` was validated — fixed (the real
  client itself rejects a missing `RoleArn` client-side, confirmed while writing the round-trip
  test above).
- `ExperimentConfig` (`ExperimentName`/`RunName`/`TrialComponentDisplayName`/`TrialName`) and
  `StoppingCondition` (`MaxRuntimeInSeconds`) were both accept-and-drop — both small flat types,
  now fully modeled rather than passthrough.
- `ListProcessingJobsInput` was `StatusEquals`/`MaxResults`-only — added `CreationTimeAfter`/
  `CreationTimeBefore`/`LastModifiedTimeAfter`/`LastModifiedTimeBefore`/`NameContains`/`SortBy`/
  `SortOrder` (`SortBy` default `CreationTime`/`SortOrder` default `Ascending`, both stated in that
  op's own doc — the first op this pass found an explicit default for both dimensions).

**`handler_automl.go` — the fourth finding, three required members accepted without validation:**

- `CreateAutoMLJobInput.RoleArn`/`InputDataConfig`/`OutputDataConfig` are each `This member is
  required`, but only `AutoMLJobName` was checked — `InputDataConfig` is validated non-empty, not
  just non-nil, since an empty slice satisfies a nil check but not AWS's real constraint.
- `ModelDeployConfig` (real, optional `CreateAutoMLJobInput` field) was never decoded on the V1
  path at all, even though the type is already modeled in this file for `CreateAutoMLJobV2`
  (`automl_v2.go`) — `SetAutoMLJobExtras` now accepts it and `DescribeAutoMLJob` returns it.
- `ListAutoMLJobsInput` was `NextToken`-only — added `CreationTimeAfter`/`CreationTimeBefore`/
  `LastModifiedTimeAfter`/`LastModifiedTimeBefore`/`NameContains`/`StatusEquals`/`SortBy`/
  `SortOrder` (`SortBy` default `Name`, `SortOrder` default `Descending` per that op's own doc).

**`handler_ai_workload_configs.go` — the fifth finding, a response field serialized in the wrong
shape entirely:**

- **`DescribeAIWorkloadConfigOutput.Tags`** (`[]types.Tag`, a JSON array of `{Key,Value}` objects)
  was being emitted from this backend's internal `map[string]string` `Tags` field directly, which
  `encoding/json` serializes as a JSON *object* — a real client's `[]types.Tag` deserializer
  rejects that outright, the same "response marshaled in the wrong shape" class as parity-22's
  `Search` timestamp bug. Fixed: `AIWorkloadConfig.MarshalJSON` now shadows `Tags` with
  `toTagObjects(c.Tags)`, proven by `TestHandler_DescribeAIWorkloadConfig_Tags_RealClient`, a real
  `aws-sdk-go-v2` client round-trip that would fail to deserialize the old shape.

**`handler_pipeline_versions.go` — the sixth finding, a List op's filter/sort surface silently
dropped:**

- `ListPipelineVersionsInput`'s `CreatedAfter`/`CreatedBefore`/`SortOrder`
  (`api_op_ListPipelineVersions.go`) were entirely absent — only `PipelineName`/`NextToken`/
  `MaxResults` were decoded. Added; the op's own doc states no default for `SortOrder`, but this
  backend's pre-existing behavior (newest-first) matches AWS's published behavior for it, so that
  default is kept and only overridden on an explicit `Ascending` request.
  `UpdatePipelineVersion`/`DescribePipelineDefinitionForExecution`/`UpdatePipelineExecution` were
  each already wire-complete — no findings there.

**A manifest claim this pass found false:** `processing_transform_job`'s existing entry claimed
`status: ok` / "No bugs found", based on an audit that covered only the *Describe response* shape.
The request surface was never checked, and both `CreateProcessingJob`'s wrong-key `VpcConfig` bug
and `CreateTransformJob`'s fabricated `RoleArn` lived on the request side. Status corrected to
`partial` with the false claim's scope noted explicitly (see families: entry).

**The six questions, answered explicitly:**

1. **What does the handler read that AWS never sends?** Yes —
   `CreateTransformJobInput.RoleArn`/`TransformJob.RoleArn` is fabricated end-to-end, this pass's
   fourth such finding in the campaign (after `CreateFeatureGroup`'s `RecordIdentifierFeatureDefinition`
   typo and two others named in earlier passes' notes).
2. **Do request and response use the same key?** `CreateProcessingJobInput`'s real key
   (`NetworkConfig.VpcConfig`) doesn't exist at any level in what this handler decoded
   (`VpcConfig` top-level) — worse than a mismatched key, a key that isn't part of the real shape
   at all. See above.
3. **Is any required request member never read?**
   `CreateEdgePackagingJobInput.OutputConfig` — yes, entirely absent, this pass's most severe
   finding. `ModelName`/`ModelVersion`/`RoleArn`/`CompilationJobName` (EdgePackagingJob),
   `RoleArn`/`AppSpecification`/`ProcessingResources` (ProcessingJob), `TransformInput`/
   `TransformOutput`/`TransformResources`' leaf fields (TransformJob), and `RoleArn`/
   `InputDataConfig`/`OutputDataConfig` (AutoMLJob) were each read but never validated present —
   the adjacent-but-distinct defect class parity-22/23 also found.
4. **Is any field parsed then ignored, or applied destructively?** Not found this pass — every gap
   found was accept-and-drop (never decoded) or a fabricated/misplaced key, not decode-then-discard
   or destructive overwrite.
5. **Does it emit every declared member?** `DescribeAIWorkloadConfigOutput.Tags` was emitted, but
   in the wrong JSON shape (object instead of array) — the same class as parity-22's `Search`
   timestamp bug, just on a different field type. No missing-emission (nil-body) case found this
   pass.
6. **Does any status or lifecycle field ever advance?** Checked across all six files:
   `TransformJobStatus`/`ProcessingJobStatus`/`EdgePackagingJobStatus` all already advance via
   `runDelayed` FSMs fixed in earlier passes (verified, not re-fixed). `AutoMLJobStatus` advances
   `InProgress`->`Stopped` on `StopAutoMLJob`, but skips the real enum's intermediate `Stopping`
   state entirely (`types/enums.go:1190-1198` declares it) — pre-existing, not introduced this pass,
   disclosed as a gap rather than fixed (see gaps:). `AIWorkloadConfig` has no status field at all
   in the real API, confirmed unchanged from parity-4's note.

**Timestamps touched, each with its own serializer citation and a test that sets the value:**
`ListEdgePackagingJobsInput.CreationTimeAfter`/`CreationTimeBefore`/`LastModifiedTimeAfter`/
`LastModifiedTimeBefore` (`api_op_ListEdgePackagingJobs.go:30-58`, decoded via `epochPtr`, asserted
by `TestHandler_ListEdgePackagingJobs_FilterSort`'s sort ordering); `ListTransformJobsInput` and
`ListProcessingJobsInput`'s equivalent four fields (same pattern, asserted by
`TestHandler_ListProcessingJobs_FilterSort`/existing `ListTransformJobs` tests); `ListAutoMLJobsInput`'s
equivalent four fields (asserted by `TestHandler_ListAutoMLJobs_FilterSort`); `ListPipelineVersionsInput.CreatedAfter`/`CreatedBefore`
(decoded via `epochPtr`, asserted by `TestHandler_PipelineVersions_Lifecycle`'s new
future/past-bound assertions, which prove the window actually excludes results rather than just
parsing them). No pre-existing timestamp-encoding
bugs found this pass — all types already had correct `MarshalJSON`/`UnmarshalJSON` pairs, checked
rather than assumed.

**Enums checked per op, confirming source over prose:** `TransformJobStatus`/`ProcessingJobStatus`/
`EdgePackagingJobStatus`/`AutoMLJobStatus`/`AutoMLJobSecondaryStatus` (`types/enums.go`) all matched
this backend's existing status-string constants exactly — no mismatch found. Generic `SortBy`/
`SortOrder` (`"Name"`/`"CreationTime"`/`"Status"`, `"Ascending"`/`"Descending"`) used by
ProcessingJob/TransformJob List ops, `AutoMLSortBy`/`AutoMLSortOrder` (same values, distinct type)
used by AutoMLJob's, and `ListEdgePackagingJobsSortBy` (`"NAME"`/`"MODEL_NAME"`/`"CREATION_TIME"`/
`"LAST_MODIFIED_TIME"`/`"STATUS"` — upper-snake-case, a distinct family from the mixed-case generic
`SortBy`) were each read from `types/enums.go` directly rather than assumed from doc prose, and two
new shared constants (`sortByCreationTime`/`sortByStatus`, mixed-case) were added alongside the
pre-existing upper-case `sortByName`/`sortByLastModifiedTime` specifically because the two enum
families are spelled differently and must not be conflated.

**Test-trap check:** No pre-existing test in any of the six files asserted a wrong shape or a
missing field silently — `processing_transform_job`'s stale `status: ok` was a doc claim, not a
ratifying assertion. This pass's own new required-field-validation tests
(`TestHandler_CreateEdgePackagingJob_RequiredFields`, `TestHandler_CreateAutoMLJob_RequiredFields`)
replace two pre-existing tests that *had* implicitly ratified the missing validation
(`TestHandler_EdgePackagingJobLifecycle`/`_ReachesCompleted` previously created jobs with no
`OutputConfig`; `TestHandler_CreateAutoMLJob_InputDataConfigDefaultsToEmptyList` previously asserted
that omitting `InputDataConfig` was fine) — both rewritten to supply the required fields and to
assert the new 400 instead.

**Hand-revert:** each of the six most severe fixes (`EdgePackagingJob.OutputConfig`,
`TransformJob.RoleArn` removal, `ProcessingJob.NetworkConfig`, `CreateAutoMLJob` required-field
validation, `AIWorkloadConfig.Tags` shape, `ListPipelineVersions` filter surface) was reverted one
at a time against a copy of the pre-pass file (`git show HEAD:<path>`), the predicted symptom
reproduced (missing `OutputConfig` in Describe; `RoleArn` echoed on Describe; the real-client
`NetworkConfig` test failing to deserialize; validation-missing test passing instead of failing;
the real-client `Tags` test failing to deserialize; the `SortOrder` test returning unsorted default
order), then restored and `md5sum`-verified byte-identical to the pre-revert working tree.

**Gates:** `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./...`, `go vet -tags integration ./...` (all three clean for
`services/sagemaker`; a concurrent, unrelated session's in-progress `services/timestreamquery`
changes fail the whole-repo `go build ./...`/`go vet ./...` — confirmed via `git status` that those
files were already modified before this pass began and are untouched by it), `gofmt -l
services/sagemaker/` (clean), `go test -race ./services/sagemaker/...` (pass, 3.2s),
`golangci-lint run ./services/sagemaker/...` (0 issues after fixing 1 `cyclop` — extracted
`edgePackagingJobMatchesFilter` — 1 `funlen` — extracted `processingInputsFromRequest`/
`processingOutputsFromRequest`/three `toBackend()` methods from `handleCreateProcessingJob` — 3
`goconst` "OutputConfig" — added shared `keyOutputConfig` constant, used across
`handler_edge_packaging_jobs.go`/`handler_labeling.go`/`handler_modelcard_export.go` — 2 `golines`,
1 `govet`/`fieldalignment` — hand-reordered, confirmed via non-`-fix` output, not the package-wide
`-fix` — and 1 `modernize` — `slices.Backward` replacing a hand-rolled reverse loop in
`pipeline_versions.go`; no `nolint` added).

`last_audit_commit` left at its existing value (`5f91d37c7`) — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**28 of sagemaker's 362 inline structs now remain.** New boundary: a 6-file tier at 3
(`handler_tags.go`, `handler_presigned_session.go`, `handler_monitoring.go`, `handler_models.go`,
`handler_endpoint_configs.go`, `handler_algorithms.go`) — not started this pass, purely an
effort-budget stopping point.

## parity-25 (2026-08-21, gopherstack-oc9v): Tags/PresignedSession/MonitoringAlert/
Models/EndpointConfigs/Algorithms field audit (the six-file tier at 3, now zero)

Nineteenth pass of the gopherstack-oc9v campaign. Per parity-24's boundary note, this pass took the
entire six-file tier it left unstarted: `handler_tags.go`, `handler_presigned_session.go`,
`handler_monitoring.go`, `handler_models.go`, `handler_endpoint_configs.go`,
`handler_algorithms.go`, each verified by `grep -c 'var req struct {' <file>.go` = 3 before starting.
All 18 structs converted to named types and wire-audited field-by-field against the pinned SDK
(`v1.263.2`, confirmed from `go.mod` and the module cache path). **28 minus this pass's 18 leaves
10 remaining**, confirmed by `grep -rc 'var req struct {' services/sagemaker/*.go` summed, not
arithmetic. **New boundary: a 5-file tier at 2**
(`handler_automl_v2.go`, `handler_feature_metadata.go`, `handler_modelcard_export.go`,
`handler_monitoring_job_definitions.go`, `handler_training_plans.go`) — not started this pass, per
this pass's own effort budget.

**`handler_models.go` — the most severe finding of this pass, an existing test that had ratified an
over-validation bug (a real client's valid request was being rejected):**

- **`CreateModelInput.ExecutionRoleArn` is *not* required.** `CreateModelInput`
  (`api_op_CreateModel.go:50-90`) marks only `ModelName` `This member is required` —
  `ExecutionRoleArn` carries no such tag. This handler rejected any `CreateModel` request missing
  it with a 400, and a pre-existing test, `TestCreateModel_RequiresExecutionRoleArn`, asserted this
  behavior explicitly ("Real AWS requires this field on all CreateModel calls" — a claim the SDK's
  own struct tags contradict). Fixed by removing the validation; the test was rewritten as
  `TestCreateModel_ExecutionRoleArnOptional`, asserting the corrected behavior (missing/empty
  `ExecutionRoleArn` accepted, missing `ModelName` still rejected).
- `ListModelsInput`'s `CreationTimeAfter`/`CreationTimeBefore`/`NameContains`/`SortBy`/`SortOrder`
  (`api_op_ListModels.go`) were entirely dropped (`NextToken`-only) — added, `SortBy` default
  `CreationTime`/`SortOrder` default `Descending` per that op's own doc, proven by
  `TestHandler_ListModels_FilterSort`.

**`handler_algorithms.go` — the second finding, a required member never validated, with a
consequence on the required response side too:**

- **`CreateAlgorithmInput.TrainingSpecification`** (`This member is required`,
  `api_op_CreateAlgorithm.go`) — only `AlgorithmName` was ever checked. A request omitting it
  silently succeeded with an empty spec, and `DescribeAlgorithmOutput.TrainingSpecification`
  (itself `This member is required`, `api_op_DescribeAlgorithm.go`) carried an incorrect
  `omitempty` tag that would have silently dropped the field from the response even after this fix
  if left uncorrected — both fixed together. 5 pre-existing tests across this file created
  algorithms with no `TrainingSpecification` at all (the test-trap class this campaign keeps
  finding); all rewritten to supply a minimal structurally-valid fixture
  (`minimalTrainingSpecification()`, `TrainingImage`/`SupportedTrainingInstanceTypes`/
  `TrainingChannels` — the type's own three required members,
  `types/types.go:22937-22954`), plus a new subtest asserting the 400.
- `ListAlgorithmsInput`'s `CreationTimeAfter`/`CreationTimeBefore`/`NameContains`/`SortBy`/
  `SortOrder` were dropped — added, `SortBy` default `CreationTime`/`SortOrder` default
  `Ascending` per `api_op_ListAlgorithms.go` (the one op in this pass's List trio whose real
  default `SortOrder` is Ascending, not Descending — checked per-op rather than assumed uniform),
  proven by `TestHandler_ListAlgorithms_FilterSort`.

**`handler_tags.go` — the third finding, a required response member never emitted (this pass's
sixth false-verdict-narrower-than-it-reads case), plus two required request members never
validated:**

- **`AddTagsOutput.Tags`** (`[]types.Tag`, "A list of tags associated with the SageMaker resource")
  was never emitted — `handleAddTags` returned a bare `{}` on every call. The manifest's `tags:`
  entry claimed `status: ok` based on not-found error-code mapping alone, never checked against the
  response shape. Fixed: `AddTags` now returns the resource's current tag set (a `ListTags` call
  after the write, reusing the existing `toTagObjects` shape — the same `[]{Key,Value}` array shape
  Q5 asks about, and exactly the class parity-24 found on `DescribeAIWorkloadConfig`). A
  pre-existing test, `TestHandler_Tags`, round-tripped `AddTags` without ever asserting on its
  response body — the "missing assertion that would have caught the bug" class this campaign's
  brief calls out; a new assertion was added to that same test rather than only to a new one.
- `AddTagsInput.Tags` and `DeleteTagsInput.TagKeys` are both `This member is required` but neither
  was checked non-empty — both now enforced (`TestHandler_AddTags_RequiresTags`/
  `TestHandler_DeleteTags_RequiresTagKeys`).
- `ListTagsInput.MaxResults` (`api_op_ListTags.go`, default 100) was decoded nowhere — `ListTags`
  paginated at a fixed `sagemakerDefaultPageSize` regardless of what a client requested. Now
  honored via the existing `paginateSlice` helper, proven by `TestHandler_ListTags_MaxResults`.

**`handler_monitoring.go` — the fourth finding, two required request members never validated, plus
a `MaxResults` field with no plumbing anywhere in the stack:**

- `UpdateMonitoringAlertInput.DatapointsToAlert`/`EvaluationPeriod` are both `This member is
  required` but neither was checked (`== 0`, the same convention this campaign already established
  for `TransformResources.InstanceCount` — a non-pointer required numeric field can't distinguish
  "absent" from a real zero, so an explicit zero-check is the established stance) — both now
  enforced.
- `ListMonitoringAlertsInput.MaxResults` (`api_op_ListMonitoringAlerts.go`, default 100) was decoded
  nowhere, and the backend helper it would have needed — `sagemakerListKeyPagedMap` — had no
  `maxResults` parameter at all, always paging at the fixed `sagemakerDefaultPageSize`. Both the
  handler and the helper (its one call site) now thread it through, proven by
  `TestHandler_ListMonitoringAlerts_MaxResults`.
- `ListMonitoringAlertHistoryInput`'s full field set was already real (checked, not assumed, since
  it looked suspiciously complete already) — no gap found there.

**`handler_presigned_session.go` — the fifth finding, a required member never validated:**

- `RenderUiTemplateInput.Task` is `This member is required`, and `types.RenderableTask.Input`
  (`types/types.go:19548`) is itself `This member is required` — neither was checked. A request
  with no `Task.Input` silently rendered the template unchanged (the existing empty-string
  early-return in `renderUITemplateContent`) instead of being rejected. Now enforced, proven by
  `TestHandler_RenderUiTemplate_MissingTaskInput`. `StartSessionInput`/`Output` and
  `RenderUiTemplateInput`'s other members already matched the real shape exactly — no other gaps
  found in this file.

**`handler_endpoint_configs.go` — the sixth finding, two real optional fields absent from decode
entirely:**

- `CreateEndpointConfigInput.ExplainerConfig`/`MetricsConfig` (both real, optional fields,
  `api_op_CreateEndpointConfig.go`) had no field for them anywhere in the request struct — accepted
  and silently dropped. Now stored+echoed as opaque `json.RawMessage` passthrough (the same
  convention this file already uses for `algorithm`'s `TrainingSpecification` and others), proven
  via a real `aws-sdk-go-v2` client round-trip test
  (`TestHandler_CreateEndpointConfig_ExplainerAndMetricsConfig_RealClient`) that constructs a
  structurally valid `ClarifyExplainerConfig`/`MetricsConfig` and confirms both survive Create ->
  Describe through the real client's own (de)serializer.
- `ListEndpointConfigsInput`'s `CreationTimeAfter`/`CreationTimeBefore`/`NameContains`/`SortBy`/
  `SortOrder` were dropped — added, same `CreationTime`/`Descending` defaults as `ListModels`,
  proven by `TestHandler_ListEndpointConfigs_FilterSort`.

**Dedup note:** `ListModels`/`ListEndpointConfigs`/`ListAlgorithms` share an identical filter shape
(`NameContains`/creation-time window/`SortBy`/`SortOrder`/`MaxResults`), so rather than three
near-copies of the same decode-filter-sort-paginate logic (which `golangci-lint`'s `dupl` correctly
flagged on the first pass at this), a single shared `nameTimeListRequest`/`nameTimeFilter`/
`filterSortPaginateByName[T]` in `list_helpers.go` now serves all three — each op still gets its
own named request type identity for handler-level clarity, but the request struct itself, the
filter struct, and the filter/sort/paginate algorithm are shared. `sagemakerListKeyPaged` (the
old `ListModels`/`ListAlgorithms` pagination helper, superseded by the filtered rewrite) had no
remaining callers and was deleted rather than left dead.

**A manifest claim this pass found false or narrower than it reads:** two, both corrected inline
(see `families:` entries) rather than silently replaced — `model_endpoint_config_crud`'s `ok`
verdict covered ARN/timestamp/error/persistence plumbing only, not request-field completeness, and
`tags`'s `ok` verdict covered not-found error-code mapping only, not the response/request field
surface. Seventh and eighth false verdicts this campaign has found, third and fourth inside
sagemaker's own records.

**The six questions, answered explicitly:**

1. **What does the handler read that AWS never sends?** None found this pass — every finding was
   either a required member never validated, a required member never emitted, or an optional
   member never decoded. Unlike parity-24 (which found two fabricated/misplaced fields), this
   pass's over-validation finding (`ExecutionRoleArn`) is the mirror-image defect: the handler
   rejected something AWS *does* accept, not something AWS never sends.
2. **Do request and response use the same key?** Checked across all six files — no mismatch found
   this pass.
3. **Is any required request member never read?**
   `CreateAlgorithmInput.TrainingSpecification`, `UpdateMonitoringAlertInput.DatapointsToAlert`/
   `EvaluationPeriod`, `RenderUiTemplateInput.Task.Input`, `AddTagsInput.Tags`,
   `DeleteTagsInput.TagKeys` — five instances this pass, the most of any single pass in this
   campaign so far.
4. **Is any field parsed then ignored, or applied destructively?** Not found this pass — every gap
   found was accept-and-drop (never decoded) or read-but-never-validated, not decode-then-discard
   or destructive overwrite.
5. **Does it emit every declared member, in the right JSON kind?** `AddTagsOutput.Tags` was not
   emitted at all (nil-body case, distinct from parity-22/24's wrong-JSON-*kind* cases) — fixed as
   the array-of-`{Key,Value}` shape Q5 asks about, per the task brief's specific flag on
   `handler_tags.go`. No wrong-kind case found this pass.
6. **Does any status or lifecycle field ever advance?** None of the six files' resources have a
   status/lifecycle field at all — `Model`/`EndpointConfig`/`Algorithm` are static CRUD resources
   (confirmed against `DescribeModelOutput`/`DescribeEndpointConfigOutput`/`DescribeAlgorithmOutput`
   — `Algorithm` does have `AlgorithmStatus`, but it is stamped `Completed` at Create and this
   backend does not simulate the real async validation/scan pipeline that would otherwise advance
   it, a pre-existing depth limit unchanged by this pass), `MonitoringAlert.AlertStatus` is set
   once at Create and never advances (`AWS` has no API to transition it directly either — it is
   driven by monitoring-execution outcomes this backend does not simulate), and presigned-session
   resources are stateless per-call tokens with no lifecycle at all. Not applicable this pass.

**Timestamps touched:** none this pass required a *fix* — `ListModelsInput`/`ListEndpointConfigsInput`/
`ListAlgorithmsInput`'s new `CreationTimeAfter`/`CreationTimeBefore` fields use the established
`*float64`/`epochPtr` pattern (never a bare `*time.Time`), matching every other List op's filter
fields in this file; no test needed to catch a decode failure because none was introduced. Report
negatives, per the task brief: no timestamp bug found or introduced this pass.

**Enums touched:** `types.ModelSortKey` (`Name`/`CreationTime`, `types/enums.go:6155-6161`),
`types.EndpointConfigSortKey` (`Name`/`CreationTime`, `types/enums.go:3346-3352`), and
`types.AlgorithmSortBy` (`Name`/`CreationTime`, `types/enums.go:358-364`) were each read from
`types/enums.go` directly, confirming all three really do share the same two-value shape before
routing them through the shared `filterSortPaginateByName` helper — `types.OrderKey`
(`Ascending`/`Descending`, `types/enums.go:6798-6804`) likewise confirmed shared by `ListModels`/
`ListEndpointConfigs`, while `ListAlgorithms`'s own doc-stated default (`Ascending`) was verified to
differ from the other two's (`Descending`) rather than assumed uniform — the one place this pass's
dedup could have silently introduced a real behavioral bug if the default had been hard-coded
instead of passed as a parameter.

**Test-trap check:** two pre-existing tests ratified real defects this pass fixed —
`TestCreateModel_RequiresExecutionRoleArn` asserted the over-validation bug directly (rewritten to
`TestCreateModel_ExecutionRoleArnOptional`, asserting the corrected behavior), and 5 `CreateAlgorithm`
tests across `handler_algorithms_test.go` created algorithms with no `TrainingSpecification`
(rewritten to supply `minimalTrainingSpecification()`). `TestHandler_Tags` was the subtler
"missing assertion" form the brief warns about: it called `AddTags` and asserted only the HTTP
status, never reading the response body, so the missing-`Tags`-field bug produced no test failure
for as long as that assertion was absent — a new assertion was added to the *existing* test rather
than routed only through a new one, so the coverage gap itself is closed, not just the bug.

**Hand-revert:** five of this pass's fixes were reverted individually against the post-conversion
working tree (a literal `git show HEAD:<path>` whole-file revert was not viable for these six files,
since `list_helpers.go`/`interfaces.go` changed in the same pass and a HEAD-only handler file no
longer matches the new shared-helper signatures — each revert instead removed or restored just the
specific validation/storage lines the fix added, per this pass's actual diff): removing
`CreateAlgorithm`'s `TrainingSpecification` check (predicted and reproduced: the `missing
TrainingSpecification` subtest now passes when it should fail — i.e. wrongly accepts); restoring
`CreateModel`'s `ExecutionRoleArn` check (predicted and reproduced: `TestCreateModel_ExecutionRoleArnOptional`
fails, rejecting valid requests again); removing `AddTags`'s `ListTags`-and-return call (predicted and
reproduced: `TestHandler_Tags` fails on the new response-body assertion); removing
`SetEndpointConfigExtras`'s `ExplainerConfig`/`MetricsConfig` storage (predicted and reproduced: the
real-client round-trip test fails on a nil `Expected value not to be nil`); and removing
`UpdateMonitoringAlert`'s two required-field checks (predicted and reproduced: both subtests of
`TestHandler_UpdateMonitoringAlert_RequiresDatapointsAndPeriod` fail, wrongly accepting). Each was
then restored and `md5sum`-verified byte-identical to the pre-revert working tree.

**Gates:** `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`
(all clean), `gofmt -l services/sagemaker/` (clean), `go test -race ./services/sagemaker/...`
(pass, ~3.2s), `golangci-lint run ./services/sagemaker/...` (0 issues after fixing 3 `goconst` —
three `"Name"` sort-by-literal call sites switched to the pre-existing shared `keyGenericName`
constant — 1 `unused` — deleted the now-dead `sagemakerListKeyPaged` — 4 `dupl` — the
`nameTimeListRequest`/`nameTimeFilter`/`filterSortPaginateByName[T]` dedup described above — and 1
`govet`/`fieldalignment` in a new test — hand-reordered two struct fields, confirmed via the
non-`-fix` lint output, not the package-wide `-fix`; no `nolint` added). A concurrent, unrelated
session's in-progress `services/timestreamquery` changes are visible in `git status` but untouched
by and unrelated to this pass — confirmed dirty before this pass began.

`last_audit_commit` left at its existing value (`5f91d37c7`) — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**10 of sagemaker's 362 inline structs now remain.** New boundary: a 5-file tier at 2
(`handler_automl_v2.go`, `handler_feature_metadata.go`, `handler_modelcard_export.go`,
`handler_monitoring_job_definitions.go`, `handler_training_plans.go`) — not started this pass,
purely an effort-budget stopping point. This is the smallest remaining tier in the campaign's
history; a single further pass could plausibly close out the rest of sagemaker's inline-struct
count entirely.

## parity-26 (2026-08-21, gopherstack-oc9v): AutoMLJobV2/FeatureMetadata/
ModelCardExportJob/MonitoringJobDefinitions/TrainingPlan field audit
(the five-file tier at 2, sagemaker's inline-struct count now zero)

Twentieth pass of the gopherstack-oc9v campaign. Per parity-25's boundary note, this pass took the
entire five-file tier it left unstarted: `handler_automl_v2.go`, `handler_feature_metadata.go`,
`handler_modelcard_export.go`, `handler_monitoring_job_definitions.go`, `handler_training_plans.go`,
each verified by `grep -c 'var req struct {' <file>.go` = 2 before starting (confirmed via
`bd show gopherstack-oc9v` and this file's own parity-25 section, not re-derived). All 10 structs
converted to named types and wire-audited field-by-field against the pinned SDK (`v1.263.2`,
confirmed from `go.mod`). **10 minus this pass's 10 leaves 0 remaining**, confirmed by
`grep -rc 'var req struct {' services/sagemaker/*.go` summed to `0`, not arithmetic.
**Sagemaker's inline-struct count is now zero — the campaign's original target (gopherstack-oc9v,
"sagemaker 362, by far the largest") is closed out entirely.**

**`handler_automl_v2.go` — the most severe finding of this pass, another over-validation-mirror
case (the class parity-25 introduced), this time in the opposite direction from parity-25's
`CreateModel`:**

- **`CreateAutoMLJobV2Input`'s `RoleArn`/`AutoMLJobInputDataConfig`/`AutoMLProblemTypeConfig`/
  `OutputDataConfig`** (`api_op_CreateAutoMLJobV2.go:72-166`) are each `This member is required`
  alongside `AutoMLJobName` — only `AutoMLJobName` was ever validated. A request missing any of
  the other four silently succeeded with an empty role and no data/problem/output config. A
  pre-existing test, `TestHandler_CreateAutoMLJobV2_RoundTrip`'s `"minimal"` case, exercised
  exactly this gap: it supplied only `AutoMLJobName`/`RoleArn` and asserted a 200 — the
  missing-assertion/incomplete-fixture test-trap shape, not a directly-pinned assertion this time.
  All four now enforced; the `"minimal"` case no longer represents a valid distinct scenario (every
  valid request needs all five members) and was removed from the round-trip table, replaced by a
  new `TestHandler_CreateAutoMLJobV2_RequiresAllRequiredMembers` covering all four omissions.

**`handler_feature_metadata.go` — the second finding, an accept-and-drop optional field plus a
required response field hardcoded to the wrong source:**

- **`UpdateFeatureMetadataInput.ParameterRemovals`** (`[]string`, real, optional,
  `api_op_UpdateFeatureMetadata.go:32-52`) was entirely absent from decode — a real client removing
  a parameter key had the removal silently dropped. `UpdateFeatureMetadata` (`feature_store.go`)
  gained a `parameterRemovals []string` parameter, deleting each key from the stored map; wired
  through the handler, proven by `TestHandler_UpdateFeatureMetadata_ParameterRemovals`.
- **`DescribeFeatureMetadataOutput.LastModifiedTime`** (`This member is required`) was hardcoded to
  `epochSeconds(fg.CreationTime)` — the owning feature group's creation time — on every call,
  never advancing regardless of how many times `UpdateFeatureMetadata` had been called.
  `FeatureMetadata` gained its own `LastModifiedTime time.Time` field, set by `UpdateFeatureMetadata`
  on every successful call; `Describe` now falls back to the group's `CreationTime` only when a
  feature has never been updated. Proven by `TestBackend_FeatureMetadata_LastModifiedTimeAdvances`
  (asserted on the backend's `time.Time` field directly — two calls in one fast unit test can
  otherwise land in the same whole second once truncated through the wire's `epochSeconds`) and
  `TestHandler_DescribeFeatureMetadata_LastModifiedTimeDefaultsToGroupCreation` (the wire-level
  fallback case). No pre-existing test asserted `LastModifiedTime` at all — the "asserting nothing"
  test-trap shape.

**`handler_modelcard_export.go` — no gaps found.** `CreateModelCardExportJobInput`'s
`ModelCardExportJobName`/`ModelCardName`/`OutputConfig.S3OutputPath` (all required) are validated
in the backend (`CreateModelCardExportJob`, `modelcard_export.go`) rather than the handler, but are
validated; every `DescribeModelCardExportJobOutput` member (required and optional) was already
correctly emitted, checked field-by-field against `api_op_DescribeModelCardExportJob.go`. Both
structs converted to named types with no behavioral change — the file's own hygiene, not a defect.

**`handler_monitoring_job_definitions.go` — the third finding, the widest-scope test-trap instance
this campaign has found (three of four sibling operation families, plus shared List-family test
setup):**

- **`RoleArn`** (`This member is required` on all four `Create*JobDefinitionInput` types) was
  decoded into `jobDefRequest.RoleArn` but never validated present.
- **`JobResources`** and the type's own `AppSpecification`/`JobOutputConfig` (all `This member is
  required`) were accept-and-drop with no presence check at all — kept only inside the opaque
  `Config` passthrough map alongside genuinely-optional fields like `NetworkConfig`/
  `StoppingCondition`, so a request omitting any of the five silently succeeded with an incomplete
  job definition. A new `validateJobDefRequest` helper (extracted from `parseJobDefRequest` to keep
  `cyclop` under its limit) checks all five, keyed off the type's name prefix derived from
  `jobInputKey` (e.g. `"DataQualityJobInput"` -> `"DataQuality"`, giving
  `"DataQualityAppSpecification"`/`"DataQualityJobOutputConfig"`).
- Three of the four sibling Create tests (`TestHandler_CreateModelBiasJobDefinition`,
  `TestHandler_CreateModelQualityJobDefinition`, `TestHandler_CreateModelExplainabilityJobDefinition`)
  plus their matching Delete-fixture-Creates, plus three List-family setup calls in
  `handler_modelmonitor_test.go` (`createDataQualityJobDef`,
  `TestHandler_ListModelQualityJobDefinitions_ReturnsCreated`,
  `TestHandler_ListModelExplainabilityJobDefinitions_ReturnsCreated`), all supplied only
  `JobDefinitionName` and asserted 200 — DataQuality's own Create/Describe tests were the sole
  exception, already supplying every required field. All rewritten via a new shared
  `minimalJobDefinitionFixture(name, typePrefix string)` helper; a new
  `TestHandler_CreateDataQualityJobDefinition_RequiresAllRequiredMembers` covers all five
  omissions.

**`handler_training_plans.go` — the fourth finding, an over-validation-mirror case pinned directly
by an existing test, plus a Describe/List field-emission asymmetry:**

- **`CreateTrainingPlanInput.TrainingPlanOfferingId`** (`This member is required` alongside
  `TrainingPlanName`, `api_op_CreateTrainingPlan.go`) was decoded but never validated present. A
  request naming no offering fell through `CreateTrainingPlan`'s `findTrainingPlanOffering` miss
  path and silently created a minimal `Active` plan with no backing reserved capacity, instead of
  being rejected. A pre-existing test, `TestHandler_CreateTrainingPlan_WithoutOffering_StaysMinimal`,
  asserted this directly (200, `Status: "Active"`, no `ReservedCapacitySummaries`) — pinning the
  over-validation gap the same way parity-25's `TestCreateModel_RequiresExecutionRoleArn` pinned its
  mirror-image bug. Rewritten as `TestHandler_CreateTrainingPlan_RequiresTrainingPlanOfferingId`,
  asserting the corrected 400.
- Separately, **`TrainingPlan.TargetResources`/`TotalInstanceCount`/`UpfrontFee`** (all real,
  optional `DescribeTrainingPlanOutput` members) were tagged `json:"-"` on the backend struct in
  `training_plans.go` — so `handleDescribeTrainingPlan`'s direct `json.Marshal(result)` silently
  omitted all three from every `DescribeTrainingPlan` response, even though
  `ListTrainingPlans`' summary builder (`trainingPlanSummaryJSON`, `handler_training_plan.go`) had
  already been projecting the same three fields into `List` responses the whole time — a
  Describe-vs-List same-field asymmetry, this campaign's Q2 question applied across two ops sharing
  one resource rather than within a single op. Fixed by correcting the three tags to their real
  wire names with `omitempty`; proven by new assertions in `TestHandler_DescribeTrainingPlan`.

**Dedup note:** none needed this pass — unlike parity-25's `ListModels`/`ListEndpointConfigs`/
`ListAlgorithms` triple, none of this pass's five files share a duplicable List/filter shape with
each other (the four job-definition types already shared `parseJobDefinitionListRequest`/
`buildJobDefinitionListResponse` before this pass, untouched here beyond the struct-name
conversion).

**No manifest claim this pass found false or narrower than it reads.** `feature_metadata` and
`modelcard_export` and `monitoring_job_definitions` had no prior `families:` entry at all (first
wire audit of each); `automl_job` and `training_plan`'s existing entries were both accurate as far
as they went, just silent on the request-validation gaps this pass found in the newer V2/Create ops
each family had not previously wire-audited.

**The six questions, plus the over-validation mirror, answered explicitly:**

1. **What does the handler read that AWS never sends?** None found this pass — every finding was
   either a required member decoded but never validated, a required member never emitted, or (in
   `CreateTrainingPlan`'s case) a required member's *absence* silently tolerated rather than
   rejected.
2. **Do request and response use the same key?** Checked across all five files — no
   request/response key mismatch found. The `TrainingPlan` Describe/List field-emission asymmetry
   above is a different shape: same key, but present on one op's response and silently dropped by
   the other's, from a struct-tag bug rather than a naming mismatch.
3. **Is any required request member never read?** All the members below were *read* (decoded into
   the request struct or a backend parameter) but never *validated* required — the same
   read-but-unchecked shape parity-25 emphasized, not accept-and-drop:
   `CreateAutoMLJobV2Input.RoleArn`/`AutoMLJobInputDataConfig`/`AutoMLProblemTypeConfig`/
   `OutputDataConfig`; `Create*JobDefinitionInput.RoleArn`/`JobResources`/`AppSpecification`/
   `JobOutputConfig` (four types); `CreateTrainingPlanInput.TrainingPlanOfferingId`. Nine instances
   this pass, the widest of any pass so far.
4. **Is any field parsed then ignored, or applied destructively?** Not found this pass — every gap
   was either read-but-never-validated (above), fully accept-and-drop
   (`UpdateFeatureMetadataInput.ParameterRemovals`, never decoded at all), or a response-side
   emission bug (`TrainingPlan`'s three tags). No destructive overwrite found.
5. **Does it emit every declared member, in the right JSON kind?**
   `DescribeTrainingPlanOutput.TargetResources`/`TotalInstanceCount`/`UpfrontFee` were not emitted
   at all (the nil-body-field case, like parity-25's `AddTagsOutput.Tags`) despite being populated
   on the backend struct the whole time. No wrong-JSON-*kind* case found this pass.
6. **Does any status or lifecycle field ever advance?**
   `DescribeFeatureMetadataOutput.LastModifiedTime` now does (see above) — the sixth instance found
   this campaign, and the first inside the feature-store family specifically.
   `ModelCardExportJob.Status`/`TrainingPlan.Status`/`JobDefinition` (no status field on any of the
   four monitoring job definition types, confirmed against
   `Describe{DataQuality,ModelBias,ModelQuality,ModelExplainability}JobDefinitionOutput`) were all
   checked and found to either have no status field (job definitions) or advance via existing,
   previously-audited FSMs (`ModelCardExportJob` stamped `Completed` at Create with no async
   pipeline simulated, `TrainingPlan`'s `Active`->`Scheduled` transition on a matched offering,
   both pre-existing and unchanged this pass) — not applicable/no new finding.

**The over-validation mirror, checked on every op this pass touched:**
`CreateAutoMLJobV2`/`Create*JobDefinition` (four types)/`CreateTrainingPlan` were all checked in
both directions — required-but-unvalidated (found, five ops) vs. validated-but-not-actually-required
(not found this pass, unlike parity-25's `CreateModel.ExecutionRoleArn`). `CreateTrainingPlan`'s
`TrainingPlanOfferingId` finding is itself the over-validation mirror's *own* mirror: the bug was
gopherstack **under**-validating (accepting what AWS requires), the same direction as this pass's
other four required-member findings, not the `CreateModel` direction — flagged explicitly because
the brief specifically asked to check both directions on every op, and this confirms the pass did.

**Timestamps touched:** `FeatureMetadata.LastModifiedTime` is new this pass — a plain `time.Time`
on the backend struct (this type is never marshaled directly to a wire response; `handler.go`'s
comment on `FeatureMetadata.GroupName` already establishes that convention for this struct), read
out via `epochSeconds` in the handler exactly like every sibling field. No `*time.Time`-as-epoch
decode was introduced or touched, so no epoch-number-into-time.Time class of bug was at risk here;
report negatives per the task brief — none found or introduced this pass.

**Enums touched:** none this pass introduced or modified an enum-backed field — `TrainingPlanStatus`/
`ModelCardExportJobStatus`/`AutoMLJobSecondaryStatus` were all read (to confirm Q6's answer above)
but not written to or defaulted anywhere in this pass's diff.

**Test-trap check:** two pre-existing tests directly pinned incorrect behavior this pass corrected
(`TestHandler_CreateAutoMLJobV2_RoundTrip`'s `"minimal"` case, `TestHandler_CreateTrainingPlan_
WithoutOffering_StaysMinimal`), and one asserted nothing on the field this pass fixed
(`TestHandler_UpdateAndDescribeFeatureMetadata` never checked `LastModifiedTime`). The widest
instance was `handler_monitoring_job_definitions_test.go`/`handler_modelmonitor_test.go`'s six
combined test-trap fixtures (three Create tests, two Delete-setup Creates, one List-family setup)
supplying only `JobDefinitionName` — the single largest missing-fixture-field cluster found in one
pass of this campaign. All rewritten to supply structurally-valid fixtures via new shared helpers
(`minimalJobDefinitionFixture`, reused across both test files) rather than only adding new tests
alongside the old, uncorrected ones.

**Hand-revert:** four of this pass's five files were reverted individually via `git show
HEAD:<path>` (HEAD, `d2f30feb6`, predates every edit in this pass, so it is byte-identical to each
file's pre-fix state — confirmed via `git status --short services/sagemaker/` showing only this
pass's own files dirty before any revert began): reverting `handler_automl_v2.go` reproduced the
predicted symptom (`TestHandler_CreateAutoMLJobV2_RequiresAllRequiredMembers`'s four subtests all
wrongly return 200); reverting `handler_feature_metadata.go` and `feature_store.go` together caused
a compile-time failure in `handler_feature_metadata_test.go` (`before.LastModifiedTime undefined`) —
a stronger proof than a runtime assertion failure, since the pre-fix type cannot even satisfy the
new test's shape; reverting `handler_training_plans.go` and `training_plans.go` together reproduced
both predicted symptoms at once (`TestHandler_CreateTrainingPlan_RequiresTrainingPlanOfferingId`
wrongly returns 200; `TestHandler_DescribeTrainingPlan`'s new assertions fail with `TargetResources`/
`UpfrontFee` absent and `TotalInstanceCount` un-marshalable as numeric); reverting
`handler_monitoring_job_definitions.go` reproduced all five subtests of
`TestHandler_CreateDataQualityJobDefinition_RequiresAllRequiredMembers` wrongly returning 200.
`handler_modelcard_export.go` was not hand-reverted — this pass found no bug in that file, only a
struct-name conversion with no behavioral change, so there was no predicted symptom to reproduce.
Each reverted file was then restored from its pre-revert copy and `md5sum`-verified byte-identical.

**Gates:** `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go vet -tags e2e ./services/sagemaker/...`, `go vet -tags integration ./services/sagemaker/...`
(all clean), `gofmt -l services/sagemaker/` (clean), `go test -race ./services/sagemaker/...`
(pass, ~3.2s), `golangci-lint run ./services/sagemaker/...` (0 issues after fixing 1 `cyclop` —
`parseJobDefRequest`'s required-field checks extracted into a new `validateJobDefRequest` helper
rather than a `//nolint`, per the campaign's standing ban on cyclop/gocyclo/gocognit/funlen
suppressions — 2 `golines` — wrapped two over-length lines — 2 `govet`/shadow — renamed shadowed
`ok` variables in the extracted helper to `hasResources`/`hasJobInput`/`hasAppSpec`/
`hasOutputConfig` — and 1 `modernize` — dropped a no-op `omitempty` from a non-pointer
`time.Time` field, since `encoding/json` never treats a zero-value struct as empty; no `nolint`
added). `go build ./...` (whole repo) also confirmed clean. `git status --short
services/sagemaker/` showed only this pass's own 13 files dirty throughout — no concurrent
session's changes touched this service.

`last_audit_commit` left at its existing value (`5f91d37c7`) — not updated this pass, per the
campaign's standing instruction never to write `pending` or otherwise touch it casually.

**0 of sagemaker's 362 inline structs remain**, confirmed by `grep -rc 'var req struct {'
services/sagemaker/*.go` summed to `0`, not arithmetic. **Sagemaker's inline-struct-conversion
campaign (gopherstack-oc9v) is closed out.** No further tiers remain in this service; the blind
spot's other services (`cleanrooms` 97, `iot` 79, `ssoadmin` 77, and the rest of the ranked list in
gopherstack-oc9v's own notes) are unaffected by this pass and remain open work for a future session.

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, 3 unbumped shape changes

`sagemakerSnapshotVersion` bumped 1 -> 2. Three registered-table shape changes landed on
this branch (parity-25/26, `ddcf7c3dc`, `22491c31e`) without a version bump, each unsafe for
a v1 snapshot: `ProcessingJob.VpcConfig` moved to `NetworkConfig.VpcConfig` (old top-level key
silently dropped), `TransformJob.RoleArn` removed entirely (old key silently dropped), and
`TrainingPlanExtension`'s three timestamp fields switched from RFC3339-string to
epoch-seconds-float encoding (old strings fail the new `UnmarshalJSON`'s float64 decode
outright, erroring the whole `trainingPlans` table restore).

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration, not just fields declared directly on a
`*Snapshot`-suffixed struct — the previous scan was blind to exactly this class, since
`Tables map[string]json.RawMessage` erases the real domain type from view.

**Proof:** `TestInMemoryBackend_RestoreV1SnapshotDiscarded` (persistence_test.go) builds a
v1-shaped snapshot with `ProcessingJob.VpcConfig` and `TransformJob.RoleArn` populated and
asserts both jobs are absent after restore (discarded, not silently decoded with those
fields gone). Hand-reverted to version 1: the same test then fails with both jobs present
and the old fields dropped, confirming the symptom; restored and `md5sum`-verified
byte-identical.

**Gates:** `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`, `gofmt -l
services/sagemaker/` (clean), `go test -race ./services/sagemaker/...` (pass).

## 2026-08-22 (gopherstack-zquj, keycheck sweep): two dropped-value key bugs fixed, one high-severity gap flagged

`cmd/keycheck` swept all 399 non-internal ops. Two confirmed dropped-required-value bugs,
same class as wafv2's `CheckCapacity` (b97408b98):

- **`AddAssociation`** wrote a single invented `"AssociationArn"` key. The real
  `AddAssociationOutput` (`aws-sdk-go-v2/service/sagemaker@v1.263.2`
  `api_op_AddAssociation.go:66-78`) has no such member at all -- it echoes back
  `SourceArn`/`DestinationArn` from the request. Every real client got both fields nil on
  every call. Fixed in `handler_lineage.go`'s `handleAddAssociation` to marshal
  `{"SourceArn": req.SourceArn, "DestinationArn": req.DestinationArn}`.
  `TestHandler_AddAssociation`'s raw-body assertion (`handler_lineage_test.go`), which had
  ratified the wrong key by asserting `resp["AssociationArn"]`, now asserts both real keys.
  Proof: `TestAddAssociation_EchoesSourceAndDestinationArn`
  (`wire_association_notebook_fixes_test.go`) drives the real SDK client and asserts both
  members decode correctly; confirmed failing (`expected: ..., actual: ""`) against the
  pre-fix key via hand-revert (`git show HEAD:services/sagemaker/handler_lineage.go`,
  restored, md5sum-verified byte-identical after re-fixing).

- **`DescribeNotebookInstance`** wrote its VPC security groups under `"SecurityGroupIds"`.
  The real `DescribeNotebookInstanceOutput` member is `"SecurityGroups"`
  (`api_op_DescribeNotebookInstance.go:142`) -- `SecurityGroupIds` is only the
  `CreateNotebookInstanceInput` field name for the same data, a name collision across
  request/response that doesn't hold on this op. Fixed in `handler_notebook_instances.go`'s
  `addNotebookOptionalFields`. Proof:
  `TestDescribeNotebookInstance_SecurityGroupsDecodesNonEmpty`
  (`wire_association_notebook_fixes_test.go`) creates a notebook instance with
  `SecurityGroupIds` via the real client, describes it, and asserts `out.SecurityGroups`
  decodes non-empty; confirmed failing against the pre-fix key via the same hand-revert
  procedure, byte-identical restore confirmed.

**NOT fixed -- flagged as a high-severity follow-up (no bd issue filed yet):**
`BatchAddClusterNodes`, `BatchDeleteClusterNodes`, `BatchRebootClusterNodes`, and
`BatchReplaceClusterNodes` all share `batchClusterNodesWithFailures` (`handler_cluster.go:773`),
which writes an invented top-level `"ClusterArn"` plus a flat `[]string` under `"Failures"`
(`"Errors"` for Delete) and **never writes `"Successful"` at all**. The real outputs
(`api_op_Batch{Add,Delete,Reboot,Replace}ClusterNodes.go`) all require `Failed` (a list of
per-op-typed error structs -- `types.Batch{Add,Delete,Reboot,Replace}ClusterNodesError`, each
with different fields, e.g. `BatchAddClusterNodesError` keys errors by
`InstanceGroupName`+`FailedCount`, the other three key by node/instance ID) and `Successful`
(`[]string` for Delete/Reboot/Replace, `[]types.NodeAdditionResult` for Add). Net effect: a
real client calling any of these four ops always gets `Failed == nil` and
`Successful == nil`, regardless of what actually happened -- the same severity as the
`CheckCapacity` bug, but across 4 ops, and not a same-shape fix: it needs per-type error
structs (not a renamed string list) and, for `BatchAddClusterNodes`, a backend signature
change to actually return which nodes succeeded (today it returns failures only). Deferred
rather than half-fixed given this campaign's "fix keys, don't restructure" constraint --
building the four distinct `Failed` item shapes is itself a struct-introduction, not a
key rename.

Other `keycheck` hits this pass, hand-verified as harmless extra/invented keys (the real
required members are present and correct elsewhere in the same response; a real client's
typed struct simply has no slot for the extra key) -- not fixed, no severity: `CreateEdgePackagingJob`
(`"EdgePackagingJobArn"`, a confirmed-empty-output op), `DescribeEdgePackagingJob`
(`"FailureReason"`), `DescribePipelineExecution` (`"PipelineParameters"`, `"StartTime"`),
`DescribeTransformJob` (`"LastModifiedTime"` -- real `CreationTime`/`TransformStartTime`/
`TransformEndTime` are all separately present and correct), `ListInferenceExperiments`
(`"Arn"`), `ListSpaces` (`"SpaceArn"`, `"SpaceStatus"`), `Search` (`"PipelineDefinition"`).

**Gates:** `go build`, `go vet`, `gofmt -l`, `go test -race`, `golangci-lint run`, all clean
for `services/sagemaker/...`.

## 2026-08-22 (gopherstack-f31u): Batch{Add,Delete,Reboot,Replace}ClusterNodes output shape fixed

Fixed the four ops the zquj sweep deferred above. Each of the three claims verified
independently against `aws-sdk-go-v2/service/sagemaker@v1.263.2`, per op:

- **Invented `ClusterArn`**: confirmed for all four. None of
  `BatchAddClusterNodesOutput`/`BatchDeleteClusterNodesOutput`/
  `BatchRebootClusterNodesOutput`/`BatchReplaceClusterNodesOutput`
  (`api_op_Batch{Add,Delete,Reboot,Replace}ClusterNodes.go`) declare a `ClusterArn` member,
  and none of the four `awsAwsjson11_deserializeOpDocumentBatch*Output` case lists
  (`deserializers.go`) has a `"ClusterArn"` case — an exact-match deserializer drops it
  silently. Removed from all four responses.
- **Wrong-shape flat `Failures`/`Errors`**: confirmed, but the *starting point* differed by
  op from the zquj note (verify-every-claim caught this): gopherstack's `Failures` was
  correct for Add/Replace but Delete already used `"Errors"` (not `"Failures"`) and both
  Delete/Reboot's `Failed`-equivalent lists were flat `[]string`, never the real per-op
  typed struct. Real shapes: `Failed []types.BatchAddClusterNodesError`
  (`api_op_BatchAddClusterNodes.go:60-66`, keyed by `InstanceGroupName`+`FailedCount`, both
  `This member is required` along with `ErrorCode`), `Failed []types.BatchDeleteClusterNodesError`
  (`api_op_BatchDeleteClusterNodes.go`, `Code`/`Message`/`NodeId`, `types/types.go:3254-3269`,
  all three required), `Failed []types.BatchRebootClusterNodesError`
  (`ErrorCode`/`Message`/`NodeId`, `types/types.go:3376-3391`, all required),
  `Failed []types.BatchReplaceClusterNodesError` (same three fields,
  `types/types.go:3448-3463`, all required). Note Delete's error-code field is JSON key
  `"Code"`; Reboot/Replace's is `"ErrorCode"` — not interchangeable despite otherwise
  identical shape. Deserializer case name for all four is `"Failed"`
  (`deserializers.go:awsAwsjson11_deserializeOpDocumentBatch*Output`), not `"Failures"`/`"Errors"`.
- **`Successful` never written**: confirmed for Add and Replace only — **refuting** the zquj
  note's claim that this held for all four. Delete and Reboot's handlers already wrote a
  `"Successful"` key with correct flat-`[]string` content (`api_op_BatchDeleteClusterNodes.go`'s
  `Successful []string`, `api_op_BatchRebootClusterNodes.go`'s `Successful []string` both
  match gopherstack's pre-existing flat shape); only their `Failed` shape was broken. Add and
  Replace's shared `batchClusterNodesWithFailures` helper genuinely never returned or wrote
  a `Successful` value at all. Add's real `Successful` type is `[]types.NodeAdditionResult`
  (`InstanceGroupName`/`NodeLogicalId`/`Status`, all required, `types/types.go:16094-16118`);
  Replace's is flat `[]string` like Delete/Reboot.

**The four ops do not share one output shape.** Delete and Reboot are structurally identical
to each other (flat `Successful []string` + typed `Failed`, differing only in the error
struct's code-field JSON key) and are now served by one shared handler,
`handleBatchNodeIDsOp`. Add is genuinely different: `Successful` is a list of structs
(`NodeAdditionResult`), and `Failed` is keyed by `InstanceGroupName`+`FailedCount` (an
aggregate over possibly many nodes), not by individual node ID like the other three. Replace
is flat-`Successful` like Delete/Reboot but was left on its own path since its request shape
(see below) already diverged.

**Backend signature changes** (the deferred sizing said this was needed for Add only — it
also turned out to be needed for Replace, since its shared helper with Add never returned a
successful list either): `InMemoryBackend.BatchAddClusterNodes` now returns
`(string, []ClusterNode, []ClusterNode, error)` (arn, successful, failed) instead of
`(string, []string, error)` (arn, failures); `BatchReplaceClusterNodes` now returns
`(string, []string, []string, error)` (arn, failed, successful) instead of
`(string, []string, error)` (arn, failures). Delete/Reboot's backend signatures were already
correct and untouched. New wire types in `handler_cluster.go`:
`batchAddClusterNodesError`/`nodeAdditionResult`/`batchDeleteClusterNodesError`/
`batchRebootClusterNodesError`/`batchReplaceClusterNodesError`. `batchClusterNodesWithFailures`
(the old shared helper) is gone, replaced by per-op construction plus the new
`handleBatchNodeIDsOp` shared by Delete/Reboot only.

**Sizing check: bigger than estimated, in a way the deferred note didn't anticipate.**
Writing the real-client proof test surfaced a **second, separate bug**, not covered by
gopherstack-f31u or the zquj sweep: the *request* side of `BatchAddClusterNodes` and
`BatchReplaceClusterNodes` is also silently broken, by the identical exact-match-decode-drops-
unknown-key mechanism, just on gopherstack's inbound `json.Unmarshal` instead of the SDK's
outbound deserializer. The real SDK client serializes `BatchAddClusterNodesInput` under
`"NodesToAdd"` (`serializers.go:39038`, a `[]types.AddClusterNodeSpecification` of
`InstanceGroupName`+`IncrementTargetCountBy`, an entirely different shape from node IDs) and
`BatchReplaceClusterNodesInput` under `"NodeIds"` (`serializers.go:39123`, flat `[]string`,
no per-node `InstanceType`) — gopherstack's request structs read `"NodeConfigs"` and
`"Nodes"` (`[]{NodeId,InstanceType}`) respectively, neither of which the real client ever
sends. **Both ops are currently a complete no-op via any real AWS SDK client**: the request
body decodes to an empty node list every time, regardless of what the caller asked for. Delete
and Reboot already read the correct `"NodeIds"` key and are unaffected. Add's fix would require
a genuine request-model redesign (server-generated `NodeLogicalId`s, per-instance-group count
tracking) rather than a rename; Replace's fix is a bounded rename+retype
(`Nodes []clusterNodeRequest` -> `NodeIds []string`, dropping the never-real per-node
`InstanceType`). Neither is fixed here — both are out of scope for f31u's output-shape fix and
are flagged here as a new, not-yet-filed follow-up, same severity class as this pass (silently
wrong data, not an error).

**Proof:** `TestHandler_BatchDeleteClusterNodes_RealClient` and
`TestHandler_BatchRebootClusterNodes_RealClient` (`handler_cluster_test.go`) drive the real
SDK client end-to-end (seed a node via the backend, `NodeIds: [seeded, missing]`) and assert
`out.Successful == [seeded]`, `out.Failed[0].{Code,ErrorCode}`/`NodeId`/`Message` are correct
and non-empty. Confirmed failing pre-fix via hand-revert: `out.Failed` decoded to **0 items**
(the real client silently dropped `"Failures"`/`"Errors"`, the invented key) even though a
missing node ID was requested. Add and Replace can't get the same real-client-driven
non-empty proof without also fixing the request-decode bug above, so they're proven two ways
instead: `TestHandler_BatchAddClusterNodes_RealClient_ResponseShape` /
`TestHandler_BatchReplaceClusterNodes_RealClient_ResponseShape` drive the real client through
an empty-effect call and assert the response typed-decodes with no `ClusterArn` member and
correctly-typed (empty) `Successful`/`Failed`; `TestHandler_BatchAddClusterNodes_DuplicateNodeFails`
/ `TestHandler_BatchReplaceClusterNodes_MissingNodeFails` drive gopherstack's own accepted raw
wire format directly and assert the actual non-empty structured `Failed` entries (`ErrorCode`,
`InstanceGroupName`, `FailedCount` for Add; `ErrorCode`, `NodeId`, `Message` for Replace).
Confirmed failing pre-fix: `DuplicateNodeFails` got a response containing the invented
`"ClusterArn"` key plus flat `"Failures":["node-1"]`; `MissingNodeFails` got `"ClusterArn"`
plus flat `"Failures":["missing-node"]`. All four hand-reverted via
`cp` to scratch + `git show HEAD:<path>`, restored, `md5sum` byte-identical after re-fixing.

**Tests that ratified the invented shape, now corrected:** `TestHandler_BatchAddClusterNodes`
asserted `resp["Failures"]` was `[]any{}`-typed and never checked `Successful` at all — now
asserts `Failed`/`Successful` (the real keys) and that `ClusterArn` is absent.
`TestHandler_BatchDeleteClusterNodes`/`TestHandler_BatchRebootClusterNodes`/
`TestHandler_BatchReplaceClusterNodes` all asserted `resp["ClusterArn"]` was a valid ARN
(a field that does not exist in the real output) and Reboot/Replace additionally asserted
`Contains(resp, "Failures")` (the invented key) — all four now assert `ClusterArn` is absent
and `Failed`/`Successful` are present. `TestHandler_BatchAddClusterNodes_DuplicateNodeFails`
directly asserted the invented flat shape — `failures, _ := resp["Failures"].([]any); ...
assert.Equal(t, "node-1", failures[0])` — the clearest ratification found this pass: it
encoded the bug as the expected behavior. Now asserts the real key and structured entry
(`InstanceGroupName`, `FailedCount`, `ErrorCode`).

**Persistence golden:** not touched. No persisted model's field set changed — only handler
wire (response-construction) types and `InMemoryBackend` in-memory method signatures changed;
`ClusterNode`/`Cluster` (the persisted structs) are unmodified. `go test ./pkgs/persistence/...`
confirmed clean without a `-update` run.

**Gates:** `go build ./...`, `go vet ./...`, `gofmt -l`, `go test -race ./services/sagemaker/...`,
`go test ./pkgs/persistence/...`, `golangci-lint run ./services/sagemaker/...`, and
`make build-check` (`go build ./...` + `go vet -tags e2e,integration ./...`) all clean. No
`//nolint` added; the `dupl` finding on Delete/Reboot's near-identical handlers was fixed by
extracting `handleBatchNodeIDsOp` rather than suppressed.

## 2026-08-22 (gopherstack-g4mx): BatchAddClusterNodes/BatchReplaceClusterNodes request binding fixed; NodeLogicalIds alternate identifier added

Fixed the request-decode bug the previous entry (f31u) surfaced but deferred, confirmed
directly against `aws-sdk-go-v2/service/sagemaker@v1.263.2`:

- **`BatchAddClusterNodesInput`** (`api_op_BatchAddClusterNodes.go:37-56`) declares
  `ClusterName`, `NodesToAdd []types.AddClusterNodeSpecification`, `ClientToken` — the real
  client serializes the list under `"NodesToAdd"` (`serializers.go:39024-39038`, each entry
  `IncrementTargetCountBy`+`InstanceGroupName`+optional `AvailabilityZones`/`InstanceTypes`,
  `serializers.go:24655-24682`, `types/types.go:79-99`). gopherstack decoded `"NodeConfigs"`
  (a `[]{NodeId,InstanceType}` shape AWS's input doesn't have at all) — every real request's
  `NodesToAdd` was silently dropped, decoding to an empty spec list every time.
- **`BatchReplaceClusterNodesInput`** (`api_op_BatchReplaceClusterNodes.go:52-84`) declares
  `ClusterName`, flat `NodeIds []string`, flat `NodeLogicalIds []string` — the real client
  serializes both under those exact flat-array keys (`serializers.go:39100-39123`).
  gopherstack decoded `"Nodes"` (again `[]{NodeId,InstanceType}`, a shape
  `BatchReplaceClusterNodesInput` does not have), so `NodeIds` was silently dropped too.

**Both ops were complete no-ops through any real AWS SDK client**: the request body decoded
to an empty node/spec list regardless of what the caller sent, and the handler returned 200
having done nothing. Confirmed pre-fix via hand-revert (`git show HEAD:<path>` restored,
`cp`'d to scratch first, `md5sum` byte-identical after re-fixing): the pre-fix
`TestHandler_BatchAddClusterNodes_RealClient_ResponseShape` /
`TestHandler_BatchReplaceClusterNodes_RealClient_ResponseShape` tests (this campaign's own
prior proof, see the f31u entry above) both re-passed with `Successful`/`Failed` empty even
when a real client sent a populated `NodesToAdd`/`NodeIds` — the defining symptom of a
never-bound field.

**Add's fix is a genuine request-model redesign, not a rename** (as the f31u entry's sizing
note anticipated): `NodesToAdd` targets *instance groups*, not individual node IDs, so
`InMemoryBackend.BatchAddClusterNodes` now takes `[]AddClusterNodeSpec`
(`InstanceGroupName`+`IncrementTargetCountBy`, `cluster.go`) instead of `[]ClusterNode`, looks
up the named instance group on the cluster, and provisions `IncrementTargetCountBy` new
running nodes via the existing `newClusterNode` helper (the same one `CreateCluster` already
uses) — reusing established provisioning logic rather than inventing new node-creation code.
A spec naming an instance group that doesn't exist on the cluster now fails with the real
`InstanceGroupNotFound` error code (`types.BatchAddClusterNodesErrorCodeInstanceGroupNotFound`,
`enums.go:1537`) instead of the old handler's invented `InvalidInstanceGroupStatus`
approximation for a since-removed "duplicate node ID" scenario that doesn't exist under the
real per-instance-group model. `AvailabilityZones`/`InstanceTypes` on each spec are decoded
for wire fidelity but are a disclosed no-op, matching this backend's existing convention of
placing every node using its instance group's own `InstanceType` (same as `CreateCluster`).

**Replace's fix is the bounded rename+retype the f31u entry predicted**:
`BatchReplaceClusterNodes` now takes `nodeIDs []string` instead of `[]ClusterNode`, dropping
the never-real per-node `InstanceType` the old `"Nodes"` shape invented.

**Proof (real-client, proving nodes actually land, not just that the response shape
decodes):** `TestHandler_BatchAddClusterNodes_RealClient` creates a cluster with a real
`"workers"` instance group via the real client, calls `BatchAddClusterNodes` with
`NodesToAdd: [{InstanceGroupName: "workers", IncrementTargetCountBy: 2}]`, asserts
`Successful` has 2 entries (`InstanceGroupName`/`NodeLogicalId`/`Status: Running`), and then
makes a **second** real-client call, `ListClusterNodes`, confirming 3 nodes now exist on the
cluster (1 from `CreateCluster` + 2 just added) — proving the nodes didn't just appear in the
response, they're actually in backend state. A second case in the same test drives
`InstanceGroupNotFound` against a real client. `TestHandler_BatchReplaceClusterNodes_RealClient`
seeds a node via the new `AddClusterNodeInternal` backend helper, then a real client's
`NodeIds: [seeded, missing]` returns `Successful: [seeded]` and a typed `Failed` entry for
`missing`. Confirmed failing pre-fix as described above.

**Placeholder tests replaced, as gopherstack-g4mx asked for by name:**
`TestHandler_BatchAddClusterNodes_RealClient_ResponseShape` and
`TestHandler_BatchReplaceClusterNodes_RealClient_ResponseShape` — the two real-client tests
that could previously only assert an always-empty result, since the request never bound — are
now `TestHandler_BatchAddClusterNodes_RealClient` / `TestHandler_BatchReplaceClusterNodes_RealClient`
above, exercising non-empty success and failure. `TestHandler_BatchAddClusterNodes_DuplicateNodeFails`
and `TestHandler_BatchReplaceClusterNodes_MissingNodeFails` — the two tests that drove
gopherstack's own invented `"NodeConfigs"`/`"Nodes"` wire format directly (a self-consistent,
unfalsifiable test: it asserts the shape the handler happened to write, the same trap found in
glue's tag-pair tests, gopherstack-v4a4) — are deleted; their non-empty-`Failed` coverage is
now subsumed by the real-client tests above, which drive the *correct* wire keys.

**`FailedNodeLogicalIds`/`SuccessfulNodeLogicalIds` decision: emit for Delete/Reboot/Replace,
not Add.** Checked each of the four `Batch*Output` types individually rather than assuming a
shared shape (per f31u's own lesson about this family): `BatchDeleteClusterNodesOutput`
(`api_op_BatchDeleteClusterNodes.go:60-82`), `BatchRebootClusterNodesOutput`
(`api_op_BatchRebootClusterNodes.go:75-104`), and `BatchReplaceClusterNodesOutput`
(`api_op_BatchReplaceClusterNodes.go:93-122`) each declare both
`FailedNodeLogicalIds []types.Batch*ClusterNodeLogicalIdsError` and
`SuccessfulNodeLogicalIds []string`, with deserializer cases at e.g.
`deserializers.go:105261-105272` (Delete), `:105353-105364` (Reboot), `:105404-105415`
(Replace). **`BatchAddClusterNodesOutput` (`api_op_BatchAddClusterNodes.go:60-77`) has no such
fields at all** — grepping the whole SDK module for `FailedNodeLogicalIds`/
`SuccessfulNodeLogicalIds` turns up only the three Delete/Reboot/Replace output types, never
Add's. Implemented via `handler_cluster.go`'s `handleBatchNodeIDsOp` (shared by all three, now
generalized from f31u's Delete/Reboot-only helper to also serve Replace): new
`batchDeleteClusterNodeLogicalIDsError`/`batchRebootClusterNodeLogicalIDsError`/
`batchReplaceClusterNodeLogicalIDsError` wire types mirror the real per-op error shapes
(`Code`/`ErrorCode` field naming split the same way the base `Failed` errors already split,
keyed by `NodeLogicalId` instead of `NodeId`/`InstanceId`). The two new keys are only emitted
when the request actually carried `NodeLogicalIds`, matching the real API's doc text ("This
field is only present when NodeLogicalIds were provided in the request.").

**`NodeLogicalIds` as an alternate identifier: real, and gopherstack now accepts it — not the
2wvq over-validation shape, because gopherstack never rejected the alternate arm in the first
place.** Doc prose confirmed on Reboot and Replace verbatim: "Either NodeIds or NodeLogicalIds
must be provided (or both), but at least one is required" (`api_op_BatchRebootClusterNodes.go:
44-72`, `api_op_BatchReplaceClusterNodes.go:52-84`); Delete's doc text says the same thing
using stale "InstanceIds" wording for what is actually the `NodeIds` field
(`api_op_BatchDeleteClusterNodes.go:55-61`) — neither `NodeIds` nor `NodeLogicalIds` is
individually `"This member is required"` on any of the three Go input structs, confirming
they're true alternates, not one primary plus a decorative extra. Before this fix, gopherstack
decoded `NodeIds` only; a request carrying only `NodeLogicalIds` wasn't *rejected* (there was
no "NodeIds required" check to trigger 2wvq's over-validation bug) — it was silently accepted
and treated as an empty batch, the same silent-no-op class as this issue's headline bug, just
on the alternate-identifier arm instead of the primary one. Fixed by decoding
`NodeLogicalIds` on the shared `batchNodeIDsRequest` (now used by Delete, Reboot, *and*
Replace) and resolving each entry as a node ID: this backend assigns every node's `NodeId`
synchronously at creation and never tracks a separate logical identifier for it, the same
disclosed-no-op convention `DescribeClusterNode`'s `NodeLogicalId` handling already
established. Deliberately did **not** add a "NodeIds or NodeLogicalIds required" validation
error for an all-empty request — that would be new under-validation gopherstack-g4mx didn't
ask for and isn't the 2wvq shape either way (2wvq is about wrongly *rejecting* valid alternate
input, not under-*enforcing* a required-together rule); an all-empty request now behaves
exactly as an empty `NodeIds` list already did before `NodeLogicalIds` existed here (accepted,
processes nothing). Left as a possible small follow-up, not filed as its own issue.

**Backend/interface signature changes:** `BatchAddClusterNodes(ctx, clusterName string,
specs []AddClusterNodeSpec) (string, []ClusterNode, []BatchAddClusterNodesFailure, error)`
(was `(string, []ClusterNode, nodeConfigs []ClusterNode) (string, []ClusterNode, []ClusterNode,
error)`); `BatchReplaceClusterNodes(ctx, clusterName string, nodeIDs []string) (string,
[]string, []string, error)` (was `nodes []ClusterNode`). New backend seeding helper
`AddClusterNodeInternal` (mirrors the existing `AddClusterInternal`) replaces three tests'
prior seeding-via-`BatchAddClusterNodes` calls, which broke once that signature changed to
take instance-group specs instead of raw nodes. No persisted model changed —
`ClusterNode`/`Cluster`/`ClusterInstanceGroup` (`models.go`) are untouched; only in-memory
backend method signatures and handler wire types changed. `go test ./pkgs/persistence/...`
confirmed clean for sagemaker (one unrelated pre-existing failure in `glue`'s
`backendSnapshot`, from a concurrent, uncommitted edit to `services/glue/models.go` by another
session — out of this issue's `services/sagemaker/`-only scope, not touched).

**Fixtures corrected (beyond the two replaced placeholder tests above):**
`TestHandler_BatchAddClusterNodes`'s body changed from the invented `"NodeConfigs"` key to
`"NodesToAdd"`, its "success" case now seeds a real instance group via `CreateCluster` and
asserts 2 real `Successful` entries instead of only type-checking an always-vacuous result, and
gained a "missing NodesToAdd" case for the now-required-member validation.
`TestHandler_BatchReplaceClusterNodes`'s body changed from `"Nodes": [{NodeId,InstanceType}]`
to flat `"NodeIds": [...]`, matching the real wire shape. `TestBatchRebootClusterNodes_PartialSuccess`,
`TestHandler_BatchDeleteClusterNodes_RealClient`, and `TestHandler_BatchRebootClusterNodes_RealClient`
all seeded their node via `h.Backend.BatchAddClusterNodes(ctx, name, []ClusterNode{...})`, a
call shape the signature change above broke; switched to `AddClusterNodeInternal`.

**Not fixed, out of scope, named here so a future pass doesn't have to re-derive it:**
`ClusterInstanceGroup.InstanceCount` (the response's `CurrentCount`/`TargetCount`) is set
directly from the request/`IncrementTargetCountBy` math, not from an actual live count of
`c.Nodes` entries in that group — a pre-existing inconsistency (predates this fix, visible
already in `fromClusterInstanceGroups`) this pass didn't touch since `BatchAddClusterNodes`
was already the narrowest change that made the count and the node set agree for the one op
being fixed.

**Gates:** `go build ./...`, `go vet ./...`, `gofmt -l services/sagemaker/`,
`go test -race ./services/sagemaker/...`, `go test ./pkgs/persistence/...` (sagemaker clean,
see glue note above), `golangci-lint run ./services/sagemaker/...` (0 issues after fixing a
`gocheckcompilerdirectives` false-positive from a `// go:` line-wrap, a `gocognit` finding on
`handleBatchNodeIDsOp` resolved by extracting `newNodeIdentifierArms`/`partitionByArm[T]`
rather than suppressed, a `goconst` finding resolved via `errCodeInstanceIDNotFound`, and 3
`revive` var-naming findings renaming `...LogicalIdsError` types to `...LogicalIDsError`), and
`make build-check` (`go build ./...` + `go vet -tags e2e ./...` + `go vet -tags integration
./...`) all clean. No `//nolint` added.

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
sagemaker is awsjson1.1 (confirmed from `sagemaker@v1.263.2`
deserializers.go's `awsAwsjson11_deserializeOpError*` prefix); its
per-operation deserializer still calls `restjson.GetErrorInfo` to
JSON-decode `__type`/`message`, so plain text doesn't decode -- a real
client got `*json.SyntaxError`, not even `UnknownError`.

`sagemaker@v1.263.2` `types/errors.go` models exactly 4 exceptions
(`ConflictException`, `ResourceInUse`, `ResourceLimitExceeded`,
`ResourceNotFound`) -- no generic internal-failure shape exists to reuse
for this framework-level path, unlike redshiftdata/resiliencehub/scheduler
above. Per DO-NOT-INVENT-AN-EXCEPTION-TYPE, this does not fabricate a new
named Go exception; it sends the standard unmodeled `"InternalFailure"`
wire code in a new `writeInternalServerError` helper. Since
`"InternalFailure"` matches none of the 4 modeled names, the client-side
deserializer's per-op switch falls through to its generic
`smithy.GenericAPIError` branch with the wire code intact -- satisfying
`ErrorCode() != "UnknownError"` without claiming a modeled exception this
service doesn't have.

CONFIRMED AND LEFT ALONE (the documented "left untyped" gap named in
gopherstack-o7gx for this service): `handleError`'s own
`errInvalidRequest`/`errUnknownAction`/syntax/type-error catch-all (line
~937) and its `default:` fallback (line ~960) are themselves untyped
(`map[string]string{keyMessageField: err.Error()}`, no `__type`) -- a
single shared branch spanning every operation's malformed-input and
internal-error cases, for which no single wire exception is safe to assume
across the whole service. This is a different, pre-existing gap from the
ReadBody-failure path this fix addresses (a distinct call site earlier in
`Handler()`, before dispatch is even reached) and is untouched here.

Proven with a real `aws-sdk-go-v2/service/sagemaker` client's
`CreateModelPackageGroup`, whose `ModelPackageGroupDescription` field alone
exceeds `httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandler_OversizedBodySurfacesInternalFailure`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalFailure"`; confirmed it fails pre-fix with `*json.SyntaxError`
(hand-reverted, byte-identical restore after).

## parity-27 (2026-08-23): never-named-in-PARITY sweep — AttachClusterNodeVolume wire shape, Space "Status" key

Diffed all 403 ops `GetSupportedOperations()` declares against every op name mentioned
anywhere in this file: 87 ops were never named by any prior audit pass, 56 of those have a
real response body (not void). Read 71 of the 87 against `sagemaker@v1.263.2`'s pinned SDK
source (types.go/api_op_*.go/deserializers.go); found and fixed two real bugs, both regressions
introduced when the ops were first stubbed in, never touched by parity-4..26 or gopherstack-oc9v
since neither op was named there.

**Bug 1 — AttachClusterNodeVolume: wrong request AND response shape, hard-fails every real call.**
`AttachClusterNodeVolumeInput` (`api_op_AttachClusterNodeVolume.go:33-51`) is `{ClusterArn,
NodeId, VolumeId}` — all three required, no other members. gopherstack's handler decoded
`{ClusterName, NodeId, VolumeConfig: {VolumeName, SizeInGB}}`, an entirely invented shape: a
real SDK client's request carries `"ClusterArn"`, which the handler's `ClusterName` field never
matches, so `req.ClusterName` decodes empty and the call 400s with `"ClusterName is required"`
before ever reaching the backend — every real client call failed. The response was also short:
`AttachClusterNodeVolumeOutput` requires `AttachTime`/`ClusterArn`/`DeviceName`/`NodeId`/
`Status`/`VolumeId` (`api_op_AttachClusterNodeVolume.go:60-93`); gopherstack returned only
`ClusterArn`/`NodeId`. The sibling `DetachClusterNodeVolume` (already correct, untouched here)
proved the backend already has everything needed: it resolves the cluster by ARN via
`resolveClusterLocked`, matches volumes by the client-supplied identifier stored as
`ClusterNodeVolume.VolumeName` (`cluster.go`'s existing "this emulator does not mint a separate
immutable VolumeId at attach time" convention), and synthesizes `DeviceName`/`Status`/
`AttachTime`. Fixed `AttachClusterNodeVolume`'s request struct to `{ClusterArn, NodeId,
VolumeId}`, its backend method to resolve by ARN via `resolveClusterLocked` (mirroring Detach)
and return a new `AttachedVolume` struct with all six fields, and the handler response to
surface all of them. `AttachClusterNodeVolume`'s `StorageBackend` interface signature changed
accordingly (`services/sagemaker/interfaces.go`).

Proven with a real `aws-sdk-go-v2/service/sagemaker` client
(`TestHandler_AttachDetachClusterNodeVolume_RealClient`,
`handler_cluster_space_realclient_test.go`): pre-fix, `client.AttachClusterNodeVolume` with a
`ClusterArn`/`NodeId`/`VolumeId` request (the only shape the real SDK ever sends) fails with
`operation error SageMaker: AttachClusterNodeVolume, ... 400, api error UnknownError: invalid
request: ClusterName is required` — confirmed by hand-revert (`cp` of the pre-fix
`cluster.go`/`handler_cluster.go`/`interfaces.go`, md5-identical restore after). Post-fix,
Attach/Detach round-trip cleanly with all six response fields populated.

**Bug 2 — DescribeSpace/ListSpaces: wire key `"SpaceStatus"`, real key is `"Status"`.**
`DescribeSpaceOutput`'s status member deserializes on wire key `"Status"`
(`deserializers.go:118584`, `awsAwsjson11_deserializeOpDocumentDescribeSpaceOutput`'s `case
"Status":`), not `"SpaceStatus"` — confirmed `"SpaceStatus"` appears as a deserializer `case`
key nowhere in the entire pinned SDK (`grep -c 'case "SpaceStatus"' deserializers.go` = 0).
`ListSpaces`' `SpaceDetails` summary type uses the same real key, `"Status"`
(`deserializers.go`'s `awsAwsjson11_deserializeDocumentSpaceDetails`). gopherstack's `Space`
struct (`spaces.go`) tagged the field `json:"SpaceStatus"`, and `handleListSpaces`
(`handler_spaces.go`) built its summary map with literal key `"SpaceStatus"` — both silently
unrecognized by the real client's deserializer (falls into its generic `default:` case,
discarded), so `DescribeSpaceOutput.Status`/`SpaceDetails.Status` always decoded as the empty
string on the client side regardless of the space's actual status. Not a hard-fail: the request
succeeds, the field is just silently missing. Fixed both to wire key `"Status"`.

`Space.MarshalJSON`/`UnmarshalJSON` (`spaces.go`) is read by both the wire response *and* the
snapshot restore path (its own doc comment says so), so this tag change also changes the
persisted-snapshot format: a v2 snapshot's `"SpaceStatus"` key would silently fail to populate
`Status` on restore. Bumped `sagemakerSnapshotVersion` 2 -> 3 (`persistence.go`) and regenerated
`pkgs/persistence/testdata/snapshot_inventory.json` via `go test ./pkgs/persistence/... -run
TestSnapshotVersionGuard -update` (2-line diff: the one field's tag, and the version number).
`AttachClusterNodeVolume`'s fix did **not** need a bump: `ClusterNodeVolume`'s struct tags are
unchanged, only what value gets stored in the existing `VolumeName` field changed.

Proven with a real client (`TestHandler_DescribeSpace_Status_RealClient`,
`handler_cluster_space_realclient_test.go`): pre-fix, `DescribeSpace`/`ListSpaces` on a freshly
created (default-InService) space both decode `Status`/`Spaces[0].Status` as `""` instead of
`"InService"` — confirmed by hand-revert (`cp` of pre-fix `spaces.go`/`handler_spaces.go`,
md5-identical restore after).

**Modelling gaps found while reading the 71, disclosed rather than fixed:** `DescribeExperiment`
(`CreatedBy`/`LastModifiedBy`/`Source`), `DescribeSpace` (`FailureReason`/`HomeEfsFileSystemUid`/
`Url`), `DescribeImage` (`FailureReason`), `DescribeModelPackageGroup` (`CreatedBy`) — all
optional SDK members with no backing state anywhere in this backend's models (no `CreatedBy`/
`UserContext` concept exists in this single-tenant emulator, matching the same disclosed
reasoning already applied to lineage/search/inference-experiment elsewhere in this file); not
synthesized per DO-NOT-INVENT-A-VALUE.

**Ops read clean, no bug (of the 71 reached):** `AssociateTrialComponent`,
`DetachClusterNodeVolume`, `CreateSpace`, `DeleteSpace`, `DescribeExperiment`,
`DeleteExperiment`, `UpdateTrial`, `DescribeTrial`, `DeleteTrial`, `DeleteTrialComponent`,
`DescribeWorkforce`, `DeleteWorkforce`, `DescribeWorkteam`, `DeleteWorkteam`,
`DescribeUserProfile`, `DeleteUserProfile`, `DescribeImage`, `DeleteImage`,
`DescribeModelPackageGroup`, `DeleteModelPackageGroup`, `DeleteModelPackageGroupPolicy`,
`DeleteModelPackage`, `GetModelPackageGroupPolicy`, `PutModelPackageGroupPolicy`,
`DescribeMonitoringSchedule` (already correctly surfaces the top-level `MonitoringType`
member via a hand-written `MarshalJSON`), `DeleteMonitoringSchedule`, `StopMonitoringSchedule`,
the DataQuality/ModelBias/ModelQuality/ModelExplainability JobDefinition family's
Describe/Delete/List (12 ops, shared `buildJobDefinitionResponse`/`buildJobDefinitionListResponse`
helpers, already SDK-cited), `GetDeviceFleetReport`, `DeleteDeviceFleet`, `DeregisterDevices`,
`GetSearchSuggestions`, `ListModelMetadata`, `ListCandidatesForAutoMLJob`,
`ListInferenceRecommendationsJobSteps` (legitimately empty — neither `Steps` nor `NextToken`
is a required output member), `StartInferenceExperiment`, `UpdateInferenceExperiment`,
`UpdateInferenceComponentRuntimeConfig`, `DeleteInferenceComponent`,
`UpdateClusterSchedulerConfig`, `DeleteClusterSchedulerConfig`, `UpdateComputeQuota`,
`DeleteComputeQuota`, `DeleteCluster`, `DeleteDomain`, `DeleteFlowDefinition`,
`DeleteFeatureGroup`, `DeleteProcessingJob`, `DeleteProject`, `DeleteStudioLifecycleConfig`,
`DeleteNotebookInstanceLifecycleConfig`, `DeleteEdgeDeploymentPlan`,
`DeleteEdgeDeploymentStage`, `DeleteHumanTaskUi`, `CreateEdgeDeploymentPlan`,
`CreateMlflowApp`, `EnableSagemakerServicecatalogPortfolio`,
`DisableSagemakerServicecatalogPortfolio`, `GetSagemakerServicecatalogPortfolioStatus`,
`ListResourceCatalogs`.

**Not reached, named so a future pass doesn't have to re-derive the queue:**
`CreateEdgeDeploymentStage`, `CreateNotebookInstanceLifecycleConfig`, `CreateUserProfile`,
`DescribeNotebookInstanceLifecycleConfig`, `DescribeStudioLifecycleConfig`,
`DescribeTrainingPlanExtensionHistory`, `DisassociateTrialComponent`,
`StartEdgeDeploymentStage`, `StartMonitoringSchedule`, `StopEdgeDeploymentStage`,
`StopProcessingJob`, `StopTransformJob`, `UpdateDevices`, `UpdateProject`, `UpdateSpace`,
`UpdateUserProfile`.

**Fixtures corrected:** `TestHandler_ClusterLifecycle`'s Attach/Detach round-trip and
`TestHandler_AttachClusterNodeVolume`'s table (`handler_cluster_test.go`) changed from the
invented `ClusterName`/`VolumeConfig` body to `ClusterArn`/`VolumeId`, with new assertions on
the previously-missing `Status`/`DeviceName`/`AttachTime` response fields.
`TestHandler_DescribeSpace` (`handler_spaces_test.go`) asserts `resp["Status"]`, not
`resp["SpaceStatus"]`.

**New tests:** `TestHandler_AttachDetachClusterNodeVolume_RealClient`,
`TestHandler_DescribeSpace_Status_RealClient` (`handler_cluster_space_realclient_test.go`) —
both real-`aws-sdk-go-v2` round-trips per the proof standard above.

**Gates:** `go build ./...`, `go vet ./services/sagemaker/...`, `gofmt -l services/sagemaker/`,
`go test -race ./services/sagemaker/... ./pkgs/persistence/...`, `golangci-lint run
./services/sagemaker/...` (0 issues after retagging three pre-existing `"Status"` map-key
literals — two touched by this pass, one pre-existing and unrelated in
`handler_training_jobs.go` — to the existing `keyStatus` constant once this pass's additions
crossed `goconst`'s duplicate threshold). No `//nolint` added.

## parity-28 (2026-08-23): the 16 "not reached" ops closed out — UpdateSpace/UpdateUserProfile drop their payload

Audited all 16 ops parity-27 left as "not reached, named": `CreateEdgeDeploymentStage`,
`CreateNotebookInstanceLifecycleConfig`, `CreateUserProfile`,
`DescribeNotebookInstanceLifecycleConfig`, `DescribeStudioLifecycleConfig`,
`DescribeTrainingPlanExtensionHistory`, `DisassociateTrialComponent`,
`StartEdgeDeploymentStage`, `StartMonitoringSchedule`, `StopEdgeDeploymentStage`,
`StopProcessingJob`, `StopTransformJob`, `UpdateDevices`, `UpdateProject`, `UpdateSpace`,
`UpdateUserProfile`. Re-derived the queue from this file's own "Not reached, named" list
(87 never-named minus the 71 parity-27 read = 16); it still holds. Found and fixed two real
bugs, both siblings of the same shape.

**Bug 1 — UpdateSpace silently drops `SpaceDisplayName`/`SpaceSettings`.**
`UpdateSpaceInput` (`api_op_UpdateSpace.go:27-42`) carries `DomainId`/`SpaceName` (required)
plus optional `SpaceDisplayName`/`SpaceSettings`. gopherstack's `updateSpaceInput`
(`handler_spaces.go`) declared only `DomainId`/`SpaceName` — a real client's
`SpaceDisplayName`/`SpaceSettings` were parsed into no field, so `json.Unmarshal` silently
dropped them, and `InMemoryBackend.UpdateSpace` (`spaces.go`) took no parameter for either and
only bumped `LastModifiedTime`. Every real `UpdateSpace` call returned `200 OK` with the ARN,
looking successful, while the space's `SpaceDisplayName`/`SpaceSettings` never changed — the
entire substance of the operation was a no-op. `CreateSpace` already threads both fields
correctly (`CreateSpaceOptions`), so this was specifically an Update-path gap, not a modelling
gap. Fixed by adding `UpdateSpaceOptions{SpaceDisplayName, SpaceSettings}`, decoding both in
the handler, and applying each in the backend only when the client actually sent it
(`SpaceDisplayName != ""`, `len(SpaceSettings) > 0` — the zero state is reachable on this
endpoint since neither member is required, so `omitempty`-style partial application here is
correct, not a bug).

**Bug 2 — UpdateUserProfile silently drops `UserSettings`.** Identical shape.
`UpdateUserProfileInput` (`api_op_UpdateUserProfile.go:26-38`) carries `DomainId`/
`UserProfileName` (required) plus optional `UserSettings`. gopherstack's
`updateUserProfileInput` (`handler_user_profiles.go`) declared only the two required fields;
`InMemoryBackend.UpdateUserProfile` (`user_profiles.go`) took no `UserSettings` parameter and
only bumped `LastModifiedTime`. Same 200-OK-but-nothing-changed symptom. `CreateUserProfile`
already threads `UserSettings` correctly. Fixed by adding
`UpdateUserProfileOptions{UserSettings}`, decoding it in the handler, and applying it in the
backend only when `len(UserSettings) > 0`.

Proven with a real `aws-sdk-go-v2/service/sagemaker` client
(`TestHandler_UpdateSpace_RealClient`, `TestHandler_UpdateUserProfile_RealClient`,
`handler_cluster_space_realclient_test.go`): each test creates a resource, calls Update with a
changed `SpaceDisplayName`/`SpaceSettings.AppType` (or `UserSettings.ExecutionRole`), then
Describes and asserts the new value stuck. Pre-fix (hand-revert via `cp` of the four pre-fix
files, md5-identical restore after), both tests fail — Describe still shows the *original*
value (`"Original Name"`/`AppTypeJupyterLab`/`.../role/original`) even though Update returned no
error, confirming the update was silently discarded rather than rejected.

**Sibling check — all 16, not just the two with bugs.** `CreateEdgeDeploymentStage`/
`StartEdgeDeploymentStage`/`StopEdgeDeploymentStage`/`UpdateDevices` (`edge_deployment.go`,
`device_fleets.go`) request shapes verified against `types.DeploymentStage`/`types.Device` —
correct. `CreateNotebookInstanceLifecycleConfig`/`DescribeNotebookInstanceLifecycleConfig`,
`CreateUserProfile`, `DescribeTrainingPlanExtensionHistory`, `DisassociateTrialComponent`,
`StartMonitoringSchedule`, `UpdateProject` all read clean against their `api_op_*.go` shapes —
`CreateUserProfile`/`UpdateProject` in particular are the create-side and update-adjacent
siblings of the two bugs above, and both correctly thread their optional fields, confirming
the Update-path drop was specific to `UpdateSpace`/`UpdateUserProfile`, not a repo-wide pattern.

**Sibling inconsistency noted, not fixed (unprovable).** `StopTransformJob`
(`transform_jobs.go`) rejects stopping a job that isn't `InProgress`; four other `Stop*` ops in
this file share that same explicit "AWS rejects stopping..." guard
(`StopCompilationJob`/`StopAutoMLJob`/`StopLabelingJob`/`StopMonitoringSchedule`). This queue's
`StopProcessingJob`/`StopEdgeDeploymentStage` (and, outside the queue, `StopTrainingJob`/
`StopHyperParameterTuningJob`/`StopOptimizationJob`/`StopInferenceRecommendationsJob`/
`StopEdgePackagingJob`) have no such guard — calling them on an already-terminal job silently
re-transitions it to `Stopping`→`Stopped`, overwriting a `Completed`/`Failed` status. Checked
both ops' real `Errors` sections (`docs.aws.amazon.com/sagemaker/latest/APIReference/
API_StopProcessingJob.html`, `.../API_StopTransformJob.html`): both document only
`ResourceNotFound`, no state-conflict error, for either op — so this is a real internal
inconsistency between siblings, but not provable as a wire-shape or documented-error bug from
the pinned SDK the way the two fixes above are. Left as-is; flagging for whoever next touches
`Stop*` job lifecycle so the guard convention gets applied (or deliberately rejected)
uniformly rather than op-by-op.

**Modelling looseness noted, not fixed.** `DescribeStudioLifecycleConfig`
(`handler_studio_lifecycle_configs.go`) marshals the full `StudioLifecycleConfig` struct
including `Tags`, but the real `DescribeStudioLifecycleConfigOutput`
(`api_op_DescribeStudioLifecycleConfig.go`) has no `Tags` member at all. This is gopherstack
emitting an extra, unmodeled field rather than dropping a modeled one — real clients ignore
unrecognized response keys, so this doesn't reproduce as a client-visible bug and wasn't fixed.

No modelling gaps found (no optional SDK member read had zero backing state).

Snapshot bump: not needed. `Space`/`UserProfile`'s persisted struct tags/types are unchanged —
`SpaceDisplayName`/`SpaceSettings`/`UserSettings` were already present fields (populated by
Create, previously just unreachable from Update); only the Update path's plumbing changed.
`go test ./pkgs/persistence/...` passes unchanged.

**New tests:** `TestHandler_UpdateSpace_RealClient`, `TestHandler_UpdateUserProfile_RealClient`
(`handler_cluster_space_realclient_test.go`).

**Gates:** `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`, `gofmt -l
services/sagemaker/` (clean), `go test -race ./services/sagemaker/...`, `go test
./pkgs/persistence/...`, `make build-check` (exported signatures of `InMemoryBackend.UpdateSpace`/
`UpdateUserProfile` changed). Work left uncommitted per instructions.

**Ops not reached:** none — all 16 from the parity-27 queue were read. No further never-named
ops from the original 87 remain outside this file: 71 (parity-27) + 16 (this pass) = 87.

## 2026-08-29 error-path sweep (wrong-code bug hunt, ERROR path only)

Audited sagemaker's not-found error-sentinel choices against each op's own
`awsAwsjson11_deserializeOpError<Op>` switch (sagemaker@v1.263.2 deserializers.go) — not the
service's general error-type list. This service's `handleError` maps two "families" of not-found:
the generic `awserr.ErrNotFound` -> `ValidationException` (the majority of "older" CRUD ops,
whose relevant Describe/Delete ops model no not-found-shaped exception at all — for these,
`ValidationException` matches real, documented SageMaker behavior and is correct as-is, e.g.
Algorithm/Endpoint/EndpointConfig/Model/NotebookInstance/CodeRepository/InferenceComponent/
ModelPackage/ModelPackageGroup/Project all confirmed empty-switch on their Describe/Delete ops),
and the special-cased `ErrResourceNotFound` -> `ResourceNotFound` (checked ahead of the generic
branch), previously documented as covering only the AIBenchmarkJob/AIRecommendationJob/
AIWorkloadConfig/generic-Job families.

**That "only" claim was wrong.** Extracting the modeled-code set for all 403 ops showed 218 of
them (54%) model `ResourceNotFound` — including nearly every classic `Describe*`/`Delete*`/
`Stop*`/`Update*` op for many long-standing resource families this service already had CRUD
support for well before the Job families existed. Cross-referencing against actual call sites
found 8 more resource families whose "not found" sentinel was still wired to the generic
`ValidationException` branch despite their own Describe/Stop/Delete/Update ops modeling
`ResourceNotFound` exclusively (an unmodeled `ValidationException` for these ops falls through to
a generic `smithy.GenericAPIError` for a real client — `errors.As` against neither
`*types.ValidationException` nor `*types.ResourceNotFound` succeeds):

- `TrainingJob` (`DescribeTrainingJob`/`StopTrainingJob`/`DeleteTrainingJob`/`UpdateTrainingJob`)
- `TransformJob` (`DescribeTransformJob`/`StopTransformJob`)
- `HyperParameterTuningJob` (`DescribeHyperParameterTuningJob`/`StopHyperParameterTuningJob`)
- `DeviceFleet` (`DescribeDeviceFleet`/`UpdateDeviceFleet`)
- `Device` (`DescribeDevice`)
- `EdgeDeploymentPlan` (`DescribeEdgeDeploymentPlan`)
- `InferenceRecommendationsJob` (`DescribeInferenceRecommendationsJob`/
  `StopInferenceRecommendationsJob`)
- `EdgePackagingJob` (`DescribeEdgePackagingJob`)

Fixed by redefining each family's `Err<X>NotFound` sentinel from
`awserr.New("ValidationException", awserr.ErrNotFound)` to
`awserr.New("ResourceNotFound", ErrResourceNotFound)` — the same shared special-case sentinel the
Job families already used, now with an updated doc comment listing all covered families instead
of the narrower (inaccurate) original claim. No call-site regression risk: sibling ops on the same
resource that don't model `ResourceNotFound` (e.g. `DeleteHyperParameterTuningJob`, an empty
switch) get an equally-unmodeled code either way.

**Deliberately not chased further this pass**: a parallel `ConflictException`-vs-`ResourceInUse`
mismatch exists for a comparable-sized set of `Update*`/`Delete*` ops (e.g. `DeleteAlgorithm`,
`DeleteCluster`, `UpdateCodeRepository`, `UpdateTrial`, ~25 more model `ConflictException` per
their own deserializer switch), but a spot-check (`DeleteAlgorithm`) found no "in use" guard
implemented in the backend at all for that op — a missing check, not a wrong sentinel at an
existing call site, and therefore a different (parity-gap, not wire-shape) class of work outside
this pass's scope. Left for a follow-up.

New tests, real typed `aws-sdk-go-v2` client, `errors.As` against `*types.ResourceNotFound`, all
9 hand-verified to fail against the pre-fix code first (asserted a
`*smithy.GenericAPIError`/`ValidationException`, not the typed exception):
`services/sagemaker/wire_error_code_not_modeled_test.go`. No pre-existing tests asserted the wrong
code for these 8 families (none checked the specific `__type`/error text, only HTTP status), so
none needed correcting.

Gates: `go build ./services/sagemaker/...` (clean), `go vet ./...` (repo-wide, clean — no
signature changes), `go test -race -count=1 ./services/sagemaker/...` (pass), `golangci-lint run
--fix ./services/sagemaker/...` (0 issues). Work left uncommitted per this pass's instructions.

## 2026-08-29 pagination arithmetic sweep

Audited every List* pagination path in sagemaker for the five known
gopherstack pagination-arithmetic bug classes (panic on stale offset,
infinite loop on stale equality-matched cursor, guarded-but-unused index,
encoder/decoder disagreement, unsorted collection). Census: every pagination
site in this service (~90+ List ops) funnels through one of six shared
helpers in `list_helpers.go` (`paginateSlice`, `sagemakerListPagedSlice`/
`sagemakerListPaged`, `sagemakerListKeyPagedMap`, `sagemakerListKeyPagedN`,
`filterSortPaginateByName`, `filterSortPaginateByNameWindow`,
`filterSortPaginateByNameOrTime`) plus one hand-rolled implementation
(`hub.go`'s `ListHubs`/`ListHubContents`/`ListHubContentVersions`, which
already breaks sort ties on `HubName` correctly). No inline
`for i, x := range all { if x.ID == token { start = i } }` site exists
outside `list_helpers.go` itself. Found and fixed two real bugs:

- **Class B (infinite loop).** `sagemakerListKeyPagedMap` (used by
  `ListMonitoringAlerts`) and `sagemakerListKeyPagedN` (used by
  `ListPartnerApps`) matched the token against keys by equality and left
  `start` at its zero value on a miss — a client whose cursor names an
  alert/app deleted since it was issued gets served page one forever
  instead of an empty final page. Fixed by defaulting the miss to
  `len(keys)`/`len(items)` (glacier's "default to end of collection"
  pattern), matching the shape already used correctly elsewhere in this
  repo. `paginateSlice`/`sagemakerListPagedSlice` (offset tokens, ~90+
  call sites) were already safe — clamped, no equality search.
- **Sixth class, not A-E: tied sort key re-sorted from an unspecified input
  order across two separate calls.** `filterSortPaginateByName` (6 call
  sites: ListEndpointConfigs/ListAlgorithms/ListModels) and
  `filterSortPaginateByNameOrTime` (ListContexts/ListActions) build `all`
  fresh from `store.Table.All()` (iteration order explicitly unspecified)
  and re-sort with `sort.Slice` (not stable) on every call. When two items
  tie on the active sort key (CreationTime is the default sort for both;
  ties are plausible under time-resolution collisions), the tied items'
  relative order is not guaranteed identical between the call that issued
  page N's token and the call serving page N+1 — each rebuilds and re-sorts
  from a differently-ordered map read. Proven with a probe that sorts the
  same tied-CreationTime item set from two different input orderings and
  shows a concatenated two-page walk duplicates items. This is a *different*
  failure than Class E (E has no sort at all): here the code does sort, but
  the comparator lacks a deterministic tiebreak, so two honest,
  independently-correct calls can still disagree. Fixed by adding a
  `nameOf`-based tiebreak whenever the primary key compares equal (also
  used to make the `desc`/`!less` flip well-defined on ties, which was
  otherwise an invalid `sort.Interface.Less` for both orderings).
  `filterSortPaginateByNameWindow`'s existing name-only sort needed no
  change (name is already the sole/unique key there).

All seven checks (non-dividing boundary walk, exact division, single page,
final page, empty collection, cursor round trip, stale cursor) pass for
`paginateSlice`, `sagemakerListPagedSlice`, `sagemakerListKeyPagedMap`,
`sagemakerListKeyPagedN` post-fix; both new tests failed against the
pre-fix code first, confirmed by the stale-cursor and tied-key assertions.

New tests: `services/sagemaker/pagination_arithmetic_test.go`.

Gates: `go build ./services/sagemaker/...` (clean), `go vet ./services/sagemaker/...`
(clean, no signature changes), `go test -race -count=1 ./services/sagemaker/...`
(pass). Work left uncommitted per this pass's instructions.

## 2026-08-30 filter-semantics sweep (gopherstack-uox6): Search, and CreationTimeAfter boundary

Audited for the class this issue tracks: a filter field that is read and
applied but implements the WRONG semantics for what the SDK documents —
invisible to every shape/enum/field-coverage sweep this campaign has run,
since the field exists, is read, and the value is a legal enum member.

**`Search` (the richest target: `types.SearchExpression`'s `Filters`,
`NestedFilters`, `Operator`, `SubExpressions`).** Two real bugs, both
under-matching turning into over-accepting once combined with the empty-list
default:

- `handler_automl_search.go`'s `searchInput.SearchExpression` decoded only
  `Operator` and `Filters` — `NestedFilters` and `SubExpressions` were never
  read from the wire at all. Since `matchesSearchExpression` returned `true`
  for an empty filter list, a request expressed purely via `NestedFilters`
  or `SubExpressions` (no top-level `Filters`) matched **every** resource of
  the requested type instead of the ones the caller asked for — an
  over-accept masking an under-match. Fixed by decoding both into a proper
  recursive `SearchExpression`/`SearchNestedFilter` domain type
  (`automl_search.go`) and combining every condition across all three lists
  by `SearchExpression`'s single documented `Operator` (`api_op_Search.go`:
  "every conditional statement in all lists ... The default value is And"),
  not per-list. `NestedFilters` is evaluated per its own doc and the SDK's
  `API_NestedFilters.html` worked example: satisfied if a single object in
  the `NestedPropertyName` list satisfies every one of its `Filters`, whose
  `Name` carries the FULL dotted path including the `NestedPropertyName`
  prefix (e.g. `InputDataConfig.DataSource.S3DataSource.S3Uri`) — verified
  against `TrainingJob.InputDataConfig`, the one nested list-of-objects field
  this backend's Search view actually exposes.
- `matchesSearchFilter` (`automl_search.go`) implemented only 5 of
  `types.Operator`'s 10 documented values (`Equals`/`NotEquals`/`Contains`/
  `Exists`/`NotExists`) and matched **unconditionally** (`return true`) for
  any of the other 5 (`GreaterThan`, `GreaterThanOrEqualTo`, `LessThan`,
  `LessThanOrEqualTo`, `In`) — over-accepting exactly this campaign's SNS
  shape (an operator outside the documented behaviour matches everything
  instead of nothing). Fixed by implementing all five, and changing the
  default case to reject (no match) rather than accept, matching the
  established fix pattern for this shape.
- **Self-inconsistency found and fixed alongside the above**: the response's
  `CreationTime`/`LastModifiedTime`/`TrainingStartTime`/`TrainingEndTime`
  are emitted as epoch-seconds numbers (correct for the JSON protocol,
  `awstime.Epoch`-equivalent), but `Filter.Value`'s own doc states timestamp
  properties compare as ISO 8601 strings
  (`YYYY-mm-dd'T'HH:MM:SS`) — a filter built in the documented format could
  never match this API's own emitted timestamp. Fixed by detecting the four
  timestamp field names and converting both sides to epoch-seconds before
  comparing, for `Equals`/`NotEquals` and the four range operators.
- **Confirmed correct, not fabricated**: the pre-existing default-`Operator`
  (empty string → `And`) already matched
  `SearchExpression.Operator`'s documented default exactly; left unchanged.

**Other hand-rolled matchers audited**: `list_helpers.go`'s
`nameTimeFilter`/`filterSortPaginateByName` (shared by `ListModels`,
`ListEndpointConfigs`, `ListAlgorithms`, `ListMonitoringExecutions`) treated
every `CreationTimeAfter` as a strict exclusive (`>`) bound. Checked each
consuming operation's own SDK doc text individually rather than assuming a
uniform rule: `ListModelsInput`/`ListEndpointConfigsInput` document
`CreationTimeAfter` as "**greater than or equal to** the specified time"
(inclusive), while `ListAlgorithmsInput`/`ListMonitoringExecutionsInput` say
plain "created after" (exclusive) — a real inconsistency in AWS's own
generated doc text across sibling operations sharing this emulator's one
helper. Fixed narrowly: added `nameTimeFilter.AfterInclusive`, set only by
`ListModels`/`ListEndpointConfigs` (the two call sites whose own doc is
explicit), leaving `ListAlgorithms` and every other `timeWindowOK`/
`filterSortPaginateByName*` consumer (~20 other files) untouched and
unaudited for the same wording variance — named here rather than implied,
since checking each of the ~12 further sagemaker `List*` operations whose
pinned-SDK doc also says "greater than or equal"/"on or after"
(`ListActions`, `ListArtifacts`, `ListContexts`, `ListAssociations`,
`ListAppImageConfigs`, `ListMonitoringAlertHistory`, `ListEndpoints`,
`ListHumanTaskUis`, `ListImageVersions`, `ListImages`,
`ListStudioLifecycleConfigs`) was out of scope for this pass.

**Gap recorded, not guessed**: `NestedFilters`' own SDK type doc gives one
concrete worked example (`InputDataConfig`/`S3Uri`) but does not state
whether `Filter.Name`'s dotted path is always exactly
`NestedPropertyName + "." + <path-within-the-nested-object>` for every
possible `NestedPropertyName`, or whether some nested properties use a
different addressing convention. Implemented for the one case both the SDK
doc and TrainingJob's own field shape confirm; not extended beyond it.

New/changed tests (all confirmed to fail against unmodified code first, 0
existing assertions weakened or dropped):
`handler_automl_search_test.go` (58→86 assertions, +28),
`handler_models_test.go` (41→47, +6), `handler_endpoint_configs_test.go`
(54→60, +6), `handler_algorithms_test.go` (41→44, +3).
`export_test.go` gained `SeedModelCreationTime`/`SeedEndpointConfigCreationTime`/
`SeedAlgorithmCreationTime` — the epoch-seconds wire round trip floors a
resource's true CreationTime, so a wire-level test can't reliably land on
the exact boundary second an inclusive-vs-exclusive test needs.

Gates: `go build ./services/sagemaker/...`, `go vet ./services/sagemaker/...`,
`go test -race -count=1 ./services/sagemaker/...`, `golangci-lint run
./services/sagemaker/...` all clean. `go vet ./...` (repo-wide, since
`SearchParams`/`nameTimeFilter` signatures changed) also clean; no external
callers of either type exist outside this package. Work left uncommitted
per this pass's instructions.

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD.
`cmd/reqfielddiff`: findings ranged 102-115 across the 5 old runs (96 at
HEAD), 26 op.field keys moving, all present in some old run and absent at
HEAD (over-reporting direction); zero keys at HEAD absent from every old
run.

The collision is `Url`/`URL` (and `Ui`/`UI`) casing:
`CreateHumanTaskUi`/`CreateHumanTaskUI`, `DeleteHumanTaskUi`/`DeleteHumanTaskUI`,
`ListHumanTaskUis`/`ListHumanTaskUIs`, `CreateHubContentPresignedUrls`/
`CreateHubContentPresignedURLs`, `CreatePresignedDomainUrl`/`CreatePresignedDomainURL`,
`CreatePartnerAppPresignedUrl`/`CreatePartnerAppPresignedURL`,
`CreatePresignedMlflowAppUrl`/`CreatePresignedMlflowAppURL`,
`CreatePresignedMlflowTrackingServerUrl`/`CreatePresignedMlflowTrackingServerURL`,
`CreatePresignedNotebookInstanceUrl`/`CreatePresignedNotebookInstanceURL` each
have a same-named exported `*InMemoryBackend` method the fallback
name-reconstruction could land on instead of the real handler. Read all 26
handler bodies (`handler_human_task_ui.go`, `handler_hub.go`,
`handler_presigned_session.go`, `handler_partner_apps.go`,
`handler_mlflow.go`, `handler_notebook_instances.go`): every field is
genuinely decoded off the JSON body. The `ExpiresInSeconds`/
`SessionExpirationDurationInSeconds`/`LandingUri` fields on the
presigned-URL family are declared and decoded but deliberately not applied
-- a pre-existing, already-documented no-op (this backend's synthetic URLs
have no verified TTL/query-parameter format to encode them into, per the
comments already in `handler_presigned_session.go`, `handler_mlflow.go`,
and `handler_notebook_instances.go`), unrelated to and unmoved by this
defect. No new bugs found; no code changed.

## PARITY-gap targeting: 18 ops never named in this file (2026-08-31, gopherstack-6flj/21my)

Queue computed by diffing every List/Describe/Get op in the pinned SDK
(sagemaker@v1.263.2, 172 such ops) against literal-word occurrence in this
file. 18 never appear by name (the base resource area was often audited
under a sibling op, but not this exact name): `DescribeDataQualityJobDefinition`,
`DescribeFlowDefinition`, `DescribeHumanTaskUi`, `DescribeInferenceExperiment`,
`DescribeModelBiasJobDefinition`, `DescribeModelCard`,
`DescribeModelExplainabilityJobDefinition`, `DescribeModelQualityJobDefinition`,
`DescribeOptimizationJob`, `DescribePartnerApp`, `DescribeReservedCapacity`,
`DescribeTrialComponent`, `ListEdgePackagingJobs`,
`ListInferenceRecommendationsJobs`, `ListLabelingJobsForWorkteam`,
`ListModelCardExportJobs`, `ListModelCardVersions`, `ListWorkforces`. All 18
covered this pass. Protocol confirmed from the deserializer directly:
awsAwsjson11 (JSON RPC 1.1, case-sensitive) throughout.

TWO SIBLING-SHAPE BUGS FOUND AND FIXED, both the Get-right/List-wrong
pattern (highest-yield heuristic in this campaign):

1. `ListEdgePackagingJobs`'s per-item summary omitted `CompilationJobName`
   entirely (`types.EdgePackagingJobSummary.CompilationJobName`,
   optional but backend-tracked) even though `DescribeEdgePackagingJob`
   already surfaces the same backend field. Wrapper key
   `EdgePackagingJobSummaries` was already correct. Fixed:
   `edgePackagingJobSummary` struct and the summary-building loop in
   `handler_edge_packaging_jobs.go:161-169,205-216`.

2. `ListInferenceRecommendationsJobs`'s per-item summary omitted
   `JobDescription` and `RoleArn` -- both members `types.InferenceRecommendationsJob`
   marks REQUIRED -- even though `DescribeInferenceRecommendationsJob`
   already surfaces both from the same backend fields. Wrapper key
   `InferenceRecommendationsJobs` was already correct. Fixed:
   `inferenceRecommendationsJobSummary` struct and the summary-building
   loop in `handler_inference_recommendations_jobs.go:125-132,176-186`.

Both fixed under a real-client test added first (confirmed failing against
unmodified code with the field decoding empty), then confirmed passing:
`handler_edge_packaging_jobs_realclient_test.go`,
`handler_inference_recommendations_jobs_realclient_test.go`. Both drive
the real `aws-sdk-go-v2/service/sagemaker` client (`newTestSageMakerClient`,
shared from `handler_create_tags_test.go`) and assert on the decoded typed
response, not a raw body.

SIXTEEN OPS CLEAN, no wrapper-key mismatch, no transposition, no hard
decode error found in any of them:

- `DescribeDataQualityJobDefinition`/`DescribeModelBiasJobDefinition`/
  `DescribeModelExplainabilityJobDefinition`/`DescribeModelQualityJobDefinition`:
  all four share `buildJobDefinitionResponse` (handler_monitoring_job_definitions.go),
  which stores each type-specific block (AppSpecification/JobInput/JobOutputConfig/
  BaselineConfig) verbatim from the Create request body and replays it
  unchanged -- wire-shape-faithful by construction. Their `List*` siblings
  (already in the "SWEPT" list, all four verified again here) share the
  generic `MonitoringJobDefinitionSummary` type and wrapper key
  `JobDefinitionSummaries` correctly.
- `DescribeFlowDefinition`: wrapper fields and nested `HumanLoopConfig`/
  `HumanLoopActivationConfig`/`OutputConfig` match `types.FlowDefinition`'s
  real names via a hand-built `MarshalJSON`. `FailureReason` and
  `PublicWorkforceTaskPrice` are real optional members not modeled --
  disclosed, no async failure/pricing state exists to source them.
- `DescribeHumanTaskUi` (Go-cased `handleDescribeHumanTaskUI`): correct
  nesting of `UiTemplate.ContentSha256` under `UiTemplate`, epoch-seconds
  `CreationTime` via `MarshalJSON`. `UiTemplate.Url` already disclosed
  as unpopulated in an existing comment.
- `DescribeInferenceExperiment`: `ModelVariants` correctly overrides the
  persisted `ModelVariantConfigs` json tag to the real wire name
  `ModelVariants` via a field-shadowing `MarshalJSON` alias; `EndpointMetadata`
  always built. `CompletionTime` (optional) not modeled -- no completion
  event distinct from `Status` tracked.
- `DescribeModelCard`: `CreatedBy`/`LastModifiedBy`/`ModelCardProcessingStatus`
  (all optional) not modeled -- no user-context or async-processing
  simulation exists.
- `DescribeOptimizationJob`: `optimizationJobResponseMap` fields all
  correct. `FailureReason`/`OptimizationOutput`/`OptimizationStartTime`/
  `OptimizationEndTime` not modeled -- jobs are created already
  `COMPLETED` with no state-machine transition, so there is never a
  distinct start/end/output/failure to report. Disclosed, not fixed
  (fabricating optimization-output content is a different axis).
- `DescribePartnerApp`: exhaustively disclosed in its own doc comment
  (`AvailableUpgrade`/`CurrentVersionEolDate`/`Version`/`Error` all
  confirmed genuinely unbacked); nothing new found.
- `DescribeReservedCapacity`: `UltraServerSummary` correctly synthesized
  via `ultraServerSummary()`, epoch-seconds `StartTime`/`EndTime` via
  `MarshalJSON`. Already disclosed limitations (single UltraServer per
  capacity, no unhealthy simulation) hold.
- `DescribeTrialComponent`: all populated fields verified against
  `types.TrialComponentParameterValue`'s real `NumberValue`/`StringValue`
  names. `Metrics`/`Source`/`Sources`/`CreatedBy`/`LastModifiedBy`
  (all optional) not modeled -- no metric or lineage-source tracking
  exists on this backend's `TrialComponent`.
- `ListLabelingJobsForWorkteam`: wrapper key `LabelingJobSummaryList` and
  every `LabelCountersForWorkteam` sub-field verified against the
  deserializer's own case list.
- `ListModelCardExportJobs`/`ListModelCardVersions`: both wrapper keys
  (`ModelCardExportJobSummaries`/`ModelCardVersionSummaryList`) and every
  per-item field verified against `types.ModelCardExportJobSummary`/
  `types.ModelCardVersionSummary`.
- `ListWorkforces`: wrapper key `Workforces` correct; reuses the same
  `workforceResponseMap` as `DescribeWorkforce`, so no sibling
  disagreement. `FailureReason` (optional) not modeled -- no async
  workforce-creation failure exists on this backend.

Tests: 2 new real-client tests added (`handler_edge_packaging_jobs_realclient_test.go`,
`handler_inference_recommendations_jobs_realclient_test.go`), 2 items each
with distinguishable non-zero values, both confirmed failing against
unmodified code before the fix. No existing test assertions changed or
dropped. Neither struct touched is persisted (`edgePackagingJobSummary`/
`inferenceRecommendationsJobSummary` are response-only DTOs built fresh
per request, not part of `models.go` or the persisted backend state) --
`TestSnapshotVersionGuard` run anyway as a precaution, unaffected.

Gates: `go build ./services/sagemaker/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/sagemaker/...`, `golangci-lint
run ./services/sagemaker/...` all clean.


## 2026-09-07 (gopherstack-tdg0): lifecycle_test.go migrated to testing/synctest; surfaced a real Start/Stop pipeline-execution race

`lifecycle_test.go`'s two async-transition waits (`assert.Eventually` in
`TestPipelineExecutionTransitionsFire`, `time.Sleep(150ms)` in
`TestShutdownCancelsPendingTransitions`) converted to `testing/synctest`
(`synctest.Test` + a fake-clock `time.Sleep` + `synctest.Wait`), following
`pipeline_execution_start_test.go`'s precedent (gopherstack-z5hj). The
scheduler both sites wait on (`runDelayed`, `lifecycle.go:115`) is
bubble-compatible: its goroutine is spawned via `b.wg.Go` synchronously
from the calling goroutine, so a backend constructed and driven entirely
inside `synctest.Test` gets a timer that lives in the same bubble --
confirmed by a passing, deterministic migration for two of the three
tests.

The migration did not just pass cleanly -- it surfaced a genuine,
previously-masked bug. `TestPipelineExecutionTransitionsFire/stop_transitions_to_Stopped`
now fails deterministically (20/20 runs under `-race`, left failing and
documented in-line rather than patched around). `StopPipelineExecution`
(pipeline_executions.go:138) and `StartPipelineExecution`/
`StartPipelineExecutionFull`'s delayed Executing->Succeeded callbacks
(pipelines.go:335-339, 567-571) both schedule against the same execution
ARN when Stop targets an execution still mid-Start. The Start callbacks
have no guard on current status before overwriting -- unlike
`StopProcessingJob`'s (processing_jobs.go:267-271), which checks
`ProcessingJobStatus == notebookStatusStopping` first -- so at
startTransitionDelay (200ms) they unconditionally clobber the execution
back to `Succeeded`, overwriting the `Stopped` that `StopPipelineExecution`
already landed at stopTransitionDelay (100ms). `assert.Eventually`'s
poll-until-first-match semantics previously hid this entirely: it observed
`Stopped` around 100ms and returned before the 200ms clobber ever ran. No
retry, tolerance, or production fix was added to make this pass -- it is
reported here for a follow-up bug, per this session's instructions.

Not migrated: `TestShutdownRespectsContextDeadline` (no Sleep/Eventually,
out of scope). 20 further `time.Sleep`/`assert.Eventually`/`require.Eventually`
sites remain elsewhere in `services/sagemaker/*_test.go`
(handler_notebook_instances_test.go, handler_labeling_test.go,
handler_endpoints_test.go, handler_processing_jobs_test.go,
handler_transform_jobs_test.go, handler_edge_packaging_jobs_test.go,
handler_inference_recommendations_jobs_test.go, handler_training_jobs_test.go,
handler_hp_tuning_jobs_test.go, handler_inference_components_test.go,
handler_compilation_jobs_test.go) -- out of scope for this issue, which
named `lifecycle_test.go` specifically.

Gates: `gofmt -l services/sagemaker/lifecycle_test.go` clean, `go vet
./services/sagemaker/...` clean, `golangci-lint run
./services/sagemaker/...` 0 issues. `go test -race -count=1
./services/sagemaker/...` fails on the one subtest above, everything else
in the package passes. `-count=20` on just the three migrated/reviewed
tests: the newly-failing subtest fails 20/20 identically (deterministic,
not flaky), the other two pass 20/20. Wall time for those three tests:
count=1 ~1.29s before (real sleeps) -> ~0.08s after; count=20 ~6.4s before
-> ~1.3s after.


## 2026-09-07 (gopherstack-7lrq): fixed the Start/Stop pipeline-execution clobber

Fixed the bug gopherstack-tdg0's entry above surfaced and left failing.
`StartPipelineExecution` (pipelines.go:335-341), `StartPipelineExecutionFull`
(pipelines.go:567-573), and `RetryPipelineExecution`
(pipeline_executions.go:106-114) each schedule a delayed
Executing->Succeeded callback — the first two at `startTransitionDelay`
(200ms), the last at `retryTransitionDelay` (200ms, both
pipeline_executions.go:29). None guarded on the execution's current
status before overwriting it, so `StopPipelineExecution`'s own delayed
callback (pipeline_executions.go:139-146, `stopTransitionDelay` = 100ms)
landing `Stopped` first was silently clobbered back to `Succeeded` once
the longer 200ms delay elapsed. All three now guard on
`exec.PipelineExecutionStatus == pipelineStatusExecuting`, matching
`StopProcessingJob`'s established guard pattern
(processing_jobs.go:267-271).

`RetryPipelineExecution` was initially reported but deliberately left
unfixed pending the issue filer's decision, since it mints its own new
execution ARN (`newArn`, pipeline_executions.go:93) rather than reusing
the ARN it was called with — folding it in required a dedicated
regression test that stops the *retried* ARN, not the original, since
stopping the original does not reach the vulnerable callback at all (it
only ever writes `newArn`). The filer asked for it to be folded in, so
it now carries the same guard and its own test,
`TestRetryPipelineExecution_StopClobbersToSucceeded`
(pipeline_execution_start_test.go), independently neutered before
landing: guard removed -> compiles, test fails with `status after delay
= "Succeeded", want "Stopped"`; guard restored -> passes.

Swept every `runDelayed` call site in `services/sagemaker/` for the same
unguarded-clobber shape. All other job-family Start/Stop pairs
(compilation, edge-packaging, HP-tuning, inference-recommendations,
labeling, processing, training, transform, and the generic AI-job
family in `jobs.go`/`lifecycle.go`) already guard their Stop-side
callback on the expected `*Stopping` status before writing `*Stopped`.
Two other unguarded sites remain and are intentionally left unfixed —
the issue filer is tracking them as a separate, lower-severity issue:

- `scheduleEndpointTransition` (endpoints.go:430-451) and
  `scheduleInferenceComponentTransition` (inference_components.go:216-236)
  unconditionally overwrite status on every delayed firing. Both only
  ever schedule `Creating/Updating -> InService` (no Stop/Delete-via-status
  path was found; `DeleteEndpoint`/`DeleteInferenceComponent` remove the
  record outright, so a completed delete does not get resurrected). The
  only observed risk is a transient reordering when two transitions
  overlap (e.g. `UpdateEndpoint` called again before a prior transition's
  callback fires), not a permanent wrong-terminal-status clobber like the
  pipeline-execution bug.

`PipelineExecutionStatus` is set, in this package, to only four values
(pipeline_executions.go:35-38): `Executing`, `Succeeded`, `Stopping`,
`Stopped`. A Start callback should advance only from `Executing`:
`StopPipelineExecution` sets `Stopping` synchronously under `b.mu` the
moment Stop is called (pipeline_executions.go:134), before its own
delayed callback ever runs, so by the time a Start callback's guard
could observe `Stopping`, the status is never `Executing` -- guarding on
`== Executing` alone is sufficient, an explicit `Stopping` exclusion
would be redundant.

Added `TestStartPipelineExecutionFull_StopClobbersToSucceeded` and
`TestRetryPipelineExecution_StopClobbersToSucceeded`
(pipeline_execution_start_test.go) -- the `StartPipelineExecutionFull`
and `RetryPipelineExecution` equivalents of `lifecycle_test.go`'s
now-passing `stop_transitions_to_Stopped` subtest; no prior test covered
either race. The Retry test starts an execution, lets it settle, retries
it, then stops the *retried* execution's own ARN before asserting
`Stopped` survives past `retryTransitionDelay`. The positive path (an
untouched execution still reaches `Succeeded`) was already covered by
`TestStartPipelineExecution_TransitionsThroughExecuting` (Start/StartFull)
and `lifecycle_test.go`'s `retry_transitions_to_Succeeded` subtest (Retry).

Each guard was independently neutered (removed, confirmed the package
still compiles, confirmed a test then fails) and restored:
`StartPipelineExecution`'s guard removal fails
`lifecycle_test.go`'s `TestPipelineExecutionTransitionsFire/stop_transitions_to_Stopped`;
`StartPipelineExecutionFull`'s guard removal fails
`TestStartPipelineExecutionFull_StopClobbersToSucceeded`;
`RetryPipelineExecution`'s guard removal fails
`TestRetryPipelineExecution_StopClobbersToSucceeded`.

Gates: `go build ./services/sagemaker/...` clean, `go test -race
-count=1 ./services/sagemaker/...` fully green (previously-failing
subtest now passes), `go test -race -count=20 -run
'TestPipelineExecutionTransitionsFire|TestStartPipelineExecutionFull_StopClobbersToSucceeded|TestRetryPipelineExecution_StopClobbersToSucceeded'
./services/sagemaker/` 20/20, `golangci-lint run services/sagemaker/...`
0 issues.

## 2026-09-07 (gopherstack-rh77): guarded scheduleEndpointTransition/scheduleInferenceComponentTransition; confirmed no permanent-clobber, P3 stands

Follow-up to the two sites gopherstack-7lrq's entry above named and left
unfixed. `DeleteEndpoint` (endpoints.go:307-324) and
`DeleteInferenceComponent` (inference_components.go:453-467) both call
`store.Delete(name)` outright — neither ever writes a `Deleting` (or any
other) status before removing the record — so the filer's reasoning
that a completed delete cannot be resurrected by a stale callback was
confirmed correct: `scheduleEndpointTransition`/
`scheduleInferenceComponentTransition` both check `store.Get(name)` and
no-op when the record is gone.

`types.EndpointStatus` (enums.go): `OutOfService`, `Creating`,
`Updating`, `SystemUpdating`, `RollingBack`, `InService`, `Deleting`,
`Failed`, `UpdateRollbackFailed`. `types.InferenceComponentStatus`:
`InService`, `Creating`, `Updating`, `Failed`, `Deleting`. This backend
only ever *writes* three of each: `Creating`, `Updating`, `InService`
(confirmed by grepping every `EndpointStatus =`/`InferenceComponentStatus =`
assignment in the package) — `Failed`/`Deleting`/`RollingBack`/
`OutOfService`/`SystemUpdating`/`UpdateRollbackFailed` are never
simulated. That fact is what makes the guard's fromStatus a single value
per call site rather than a set: `CreateEndpointFSM`/
`CreateInferenceComponent` synchronously set `Creating` then schedule a
transition guarded on `== Creating`; `UpdateEndpointFSM`,
`UpdateEndpointWeightsAndCapacitiesFull`, `UpdateInferenceComponent`, and
`UpdateInferenceComponentRuntimeConfig` all set `Updating` then schedule
a transition guarded on `== Updating`. Each call site already knows,
synchronously, which status it just set — there is never a case where
one call site's scheduled transition legitimately needs to advance from
either of two different statuses, so a set was unnecessary; a guard
requiring `InService` instead of `Creating`/`Updating` would have been
the too-narrow mistake the issue warned about, and would break the
normal path outright (nothing would ever leave `Creating`/`Updating`).

Interleaving analysis: because every scheduled transition in both files
targets the same terminal status (`InService`) and nothing in this
backend ever schedules a transition to `Failed`/`Deleting`/anything
else, no interleaving of overlapping Create/Update calls can leave a
*permanently* wrong status — every pending callback, whenever it
eventually fires (guarded or not), either no-ops (record deleted) or
writes the same `InService` value the record was always going to reach.
Concrete interleaving: `CreateEndpointFSM` at t=0 sets `Creating`,
schedules `Creating->InService` at `endpointCreatingToInService` (300ms,
fires t=300); an immediate `UpdateEndpointFSM` at t=0 (endpoints.go has
no precondition that an endpoint be `InService` before it can be
updated) sets `Updating`, schedules `Updating->InService` at
`endpointUpdatingToInService` (250ms, fires t=250). Without the guard,
Update's callback fires at t=250 and correctly lands `InService`; then
Create's stale callback fires at t=300 and unconditionally re-writes
`EndpointStatus = InService` (same value, so not visibly wrong) but also
bumps `LastModifiedTime` to t=300 and re-syncs `ProductionVariants`
Current* from Desired* — a phantom "changed at t=300" when nothing
actually changed since t=250. That is the real, if narrow, defect: not
a wrong terminal status (this was correctly filed P3, not re-prioritised
to P2 like 7lrq — 7lrq's bug replaced a legitimate, distinct terminal
value, `Stopped`, with a different, wrong one, `Succeeded`, and nothing
ever corrected it), but a spurious `LastModifiedTime`/variant-resync
touch after a later, overlapping transition already finished the
record. The `fromStatus` guard fixes exactly this: a stale callback
that no longer finds the status it expects (because a later transition
already advanced past it) now no-ops instead of re-touching the record.

Same interleaving applies symmetrically to
`scheduleInferenceComponentTransition` (`CreateInferenceComponent` +
`UpdateInferenceComponent`/`UpdateInferenceComponentRuntimeConfig`,
`inferenceComponentCreatingToInService`/`inferenceComponentUpdatingToInService`,
both 300ms/250ms).

Files changed: `endpoints.go` (`scheduleEndpointTransition` gained a
`fromStatus` parameter and guard; its three call sites pass
`statusCreating`/`statusUpdating`); `inference_components.go` (same
shape for `scheduleInferenceComponentTransition` and its three call
sites); `endpoint_inference_component_transition_test.go` (new,
white-box, `synctest`-based); `handler_endpoints_test.go` and
`handler_inference_components_test.go` (pre-existing `assert.Eventually`/
`require.Eventually` polls on these exact transitions converted to
`synctest.Test` + `synctest.Wait`, per this package's ban on
Eventually — masked exactly this class of race in gopherstack-7lrq).

New tests: `TestEndpointTransitions_ReachInService` and
`TestInferenceComponentTransitions_ReachInService` (table-driven over
create/update/weights-and-capacities and create/update/update-runtime-
config respectively) pin the normal path — each still reaches
`InService` after its own delay, so a too-narrow `fromStatus` would be
caught here. `TestEndpointTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate`
and `TestInferenceComponentTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate`
pin the race — explicitly an ordering test, not a clobber test, per the
interleaving analysis above: they assert `LastModifiedTime` is
unchanged by Create's stale callback after Update has already settled
the record on `InService`, not that the status differs.

Three pre-existing tests were corrected because they poll these same
transitions and would otherwise timeout-fail (or silently races-mask a
future regression) under the ban on `Eventually`:

- `TestHandler_ListEndpoints_Filters` (handler_endpoints_test.go): the
  `require.Eventually` waiting for both endpoints to leave `Creating`
  was replaced with `synctest.Test` around the two `CreateEndpoint`
  calls plus `time.Sleep`/`synctest.Wait`. `future`/`past` (used by the
  `CreationTimeAfter`/`LastModifiedTimeBefore` subtests) had to move
  inside the same bubble and be captured into closure variables — a
  synctest bubble's fake clock does not track the real wall clock, so
  computing them with a real `time.Now()` after the bubble closed
  produced nonsensical comparisons against the bubble-clock-stamped
  `CreationTime`/`LastModifiedTime` (this was caught by the gate run,
  not anticipated — two subtests failed with off-by-decades comparisons
  until fixed).
- `TestHandler_DescribeEndpoint_EventuallyInService`
  (handler_endpoints_test.go): same `Eventually`->`synctest` conversion,
  no other change needed (no post-bubble time comparisons).
- `TestHandler_InferenceComponentLifecycle`
  (handler_inference_components_test.go): same conversion, but the
  *entire* test body had to move inside one `synctest.Test` bubble, not
  just the Create-to-InService wait — `runDelayed`'s `b.wg.Go` call
  panics ("WaitGroup.Add called from inside and outside synctest
  bubble") if the same backend's WaitGroup is touched by a
  bubble-spawned goroutine and then again by a call made outside any
  bubble, and this test's later `UpdateInferenceComponentRuntimeConfig`/
  `UpdateInferenceComponent` calls each schedule their own delayed
  transition. A final `time.Sleep`+`synctest.Wait` was added at the end
  (after Delete) to drain those two calls' still-pending timers before
  the bubble exits — synctest fatals with "deadlock: main bubble
  goroutine has exited but blocked goroutines remain" otherwise.

Neuter results (`scheduleEndpointTransition`/
`scheduleInferenceComponentTransition`, guard reduced to the bare
`!ok` existence check, `// NEUTERED for gopherstack-rh77 coverage
check`): both compile; endpoint guard removal fails
`TestEndpointTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate`
(`LastModifiedTime changed from ...0.25... to ...0.3...`), leaves
`TestEndpointTransitions_ReachInService` passing (as expected — only the
race test is sensitive to this guard); inference-component guard
removal fails
`TestInferenceComponentTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate`
the same way, leaves `TestInferenceComponentTransitions_ReachInService`
passing. Both restored and diffed byte-identical against the pre-neuter
version.

Gates: `go build ./services/sagemaker/...` clean; `go test -race
-count=1 ./services/sagemaker/...` fully green; `go test -race
-count=20 -run
'TestEndpointTransitions_ReachInService|TestEndpointTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate|TestInferenceComponentTransitions_ReachInService|TestInferenceComponentTransition_StaleCreateCallbackDoesNotRetouchAfterUpdate'
./services/sagemaker/` 20/20; `golangci-lint run services/sagemaker/...`
0 issues.

## 2026-09-07 (gopherstack-iook): ten more `Eventually` polls converted to `synctest`; one site left, confirmed unmigratable

Follow-up to gopherstack-7lrq/rh77's own conversions (above): eleven
remaining `assert.Eventually`/`require.Eventually` sites across
`handler_compilation_jobs_test.go`, `handler_labeling_test.go`,
`handler_edge_packaging_jobs_test.go`, `handler_hp_tuning_jobs_test.go`,
and `handler_inference_recommendations_jobs_test.go` polled a
`runDelayed` FSM transition the same way the 7lrq bug was hiding in.
Before converting, each transition's guard was read directly:
`compilation_jobs.go` (Stop and auto-complete), `labeling.go` (both
legs of `scheduleLabelingJobCompletion`, plus `StopLabelingJob`),
`edge_packaging_jobs.go` (`scheduleEdgePackagingJobCompletion`,
`StopEdgePackagingJob`), `hp_tuning_jobs.go`
(`scheduleHPTuningJobCompletion`, `StopHyperParameterTuningJob`), and
`inference_recommendations_jobs.go` (`scheduleInferenceRecommendationsJobCompletion`,
`StopInferenceRecommendationsJob`) — all ten callbacks already carry a
current-status guard matching `StopProcessingJob`'s shape
(`processing_jobs.go:264-273`), and a same-delay overlap check (e.g.
labeling's Initializing->InProgress and Stop's own Stopping->Stopped
both scheduled at 150ms) confirmed no callback can clobber a later
state. No new bug found; ten sites converted clean, matching the
`lifecycle_test.go`/`pipeline_execution_start_test.go`/
`handler_endpoints_test.go`/`handler_inference_components_test.go`
pattern: `synctest.Test` wrapping the whole body (every one of these
ten schedules its `runDelayed` goroutine synchronously inside
`Create*`, so the whole body — not just the final wait — has to share
one bubble), a `time.Sleep(time.Second)` + `synctest.Wait()` in place
of the poll, then a single `assert.Equal`/`require.Equal` on the same
final condition the `Eventually` checked (every intermediate assertion
the original body made — e.g. the `STOPPING` check before the wait —
was kept as-is; nothing was weakened).

Eleventh site,
`TestHandler_CompilationJob_ReachesCompleted_RealClient`
(`handler_compilation_jobs_test.go`), left unconverted:  this test
drives the backend through `newTestSageMakerClient`'s real SDK client
over an `httptest.NewServer` HTTP round trip
(`handler_create_tags_test.go:25`), not the direct in-process
`doSageMakerRequest` the other ten use. `synctest.Test` bubble
membership follows the goroutine tree from `go` statement time: the
server's Accept-loop goroutine (and everything it spawns per
connection, including the goroutine that actually calls
`CreateCompilationJob`/`runDelayed`) is a descendant of whichever
goroutine created the `httptest.Server` — so it, and the fake clock
governing the scheduled transition, only join the bubble if the server
itself is constructed inside `synctest.Test`. Confirmed empirically:
wrapping the whole test (client construction included) in
`synctest.Test` deadlocks — 20 iterations of `-race` were not
attempted because a single run hung with zero test output and near-zero
CPU for 452s (manually killed; go test's own 10-minute default would
have eventually fatal'd it). This is a `testing/synctest` /
real-network-I/O incompatibility, not a production bug, and not fixable
within this test file alone — `newTestSageMakerClient` is a shared
helper used by many other `_RealClient` tests across this package, so
giving it a synctest-safe in-process transport is a separate,
broader change. Left as `require.Eventually` (`handler_compilation_jobs_test.go:578`
after the other edits shifted line numbers).

Files changed: `handler_compilation_jobs_test.go`,
`handler_labeling_test.go`, `handler_edge_packaging_jobs_test.go`,
`handler_hp_tuning_jobs_test.go`,
`handler_inference_recommendations_jobs_test.go` (all ten
`Eventually`->`synctest` conversions); each gained a
`"testing/synctest"` import.

Timing: the ten migrated sites, run individually against the
pre-migration code, took 0.15-0.31s each real time (`go test -race
-count=1 -v -run '<the ten names>' ./services/sagemaker/...` totalled
2.361s); the same run against the migrated code reports 0.00s per site
(1.603s total, dominated by the still-real eleventh site's 0.31s) —
the fake clock resolves each `time.Sleep(time.Second)` instantly
instead of waiting out the real 150-450ms transition delay.

Gates: `go test -race -count=1 ./services/sagemaker/...` — 2.522s,
fully green. `go test -race -count=20 ./services/sagemaker/...` —
29.674s, 20/20, no flakes. `golangci-lint run services/sagemaker/...`
0 issues. `grep -rn 'assert\.Eventually\|require\.Eventually'
services/sagemaker/*_test.go` returns exactly two lines: the live
eleventh site above, and `lifecycle_test.go:68`, which is prose inside
a comment (`// ... assert.Eventually hid that by returning the ...`),
not a call.
