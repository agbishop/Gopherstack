---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: forecast
sdk_module: aws-sdk-go-v2/service/forecast@v1.44.4
last_audit_commit: 80757023
last_audit_date: 2026-08-13
overall: A            # 2026-09-06: gopherstack-jrhh (LimitExceededException never returned) --
                       # PARTIALLY IMPLEMENTABLE. https://docs.aws.amazon.com/forecast/latest/dg/limits.html
                       # has an explicit Adjustable column; only TagResource's row is both
                       # non-adjustable and a per-resource, already-observable-state check:
                       # "Maximum number of tags you can add to a resource | 50 | No". Now
                       # enforced (maxTagsPerResource, tags.go) against the resource's resulting
                       # tag set (existing + incoming, re-tagging a key is not an addition), via
                       # a new ErrTagLimitExceeded sentinel mapped to LimitExceededException in
                       # handleError. DOCUMENTATION-SOURCED, not SDK-verified (LimitExceededException
                       # is declared in types/errors.go with no numeric field to check against).
                       # NOT enforced, left for a future decision: CreateAutoPredictor (500) and
                       # CreateExplainability/CreateExplainabilityExport (1000 each, plus parallel-
                       # task caps of 3) are marked non-adjustable but are account-wide
                       # resource-count ceilings, not per-resource attribute counts -- closer to
                       # the adjustable-quota shape already declined at services/efs/PARITY.md:76,80
                       # than to TagResource's shape, and needs an explicit call before hardcoding.
                       # Most other Create ops (CreateDataset, CreateDatasetGroup,
                       # CreateDatasetImportJob, CreatePredictor, CreateForecast,
                       # CreateForecastExportJob, CreatePredictorBacktestExportJob,
                       # CreateWhatIfAnalysis, CreateWhatIfForecast, CreateWhatIfForecastExport)
                       # are explicitly "Adjustable: Yes" -- the EFS-declined shape, left alone.
                       # CreateMonitor and ResumeResource have no published number anywhere on the
                       # quotas page; durably blocked, recorded so nobody re-searches.
                       # 2026-08-14: closed part of gopherstack-dv4s (over-wide List responses):
                       # every family note below claiming "List verified" had only checked that
                       # the shared generic listOutput()/resourceOutput() round-tripped required
                       # fields correctly -- never that a real List op's response omits members
                       # the Describe/Create request shape carries. It doesn't: listOutput() called
                       # the exact same resourceOutput() as Describe, so every List* op echoed the
                       # full stored create-request body (e.g. ListPredictors leaked
                       # InputDataConfig/FeaturizationConfig/ForecastHorizon/TrainingParameters/
                       # AlgorithmArn; ListDatasets leaked Schema/EncryptionConfig/DataFrequency).
                       # All 12 List families across this service shared the one bug -- the same
                       # shape as personalize's 16-for-16 finding on the same issue. Fixed by
                       # adding a per-family summaryFields/summaryStatus allowlist to
                       # operationSpec, read individually from each op's real
                       # List<Kind>sOutput.<Kind>Summary declaration in
                       # aws-sdk-go-v2/service/forecast@v1.44.4/types/types.go (not derived from
                       # the Describe shape or by analogy between families), and a new
                       # summaryOutput() used only by listOutput(). See the corrected per-family
                       # notes below for each Summary type's exact field citation.
                       # 2026-08-13: closed gopherstack-wl0s (required-presence validation):
                       # CreateExplainability's ExplainabilityConfig; CreateForecastExportJob's,
                       # CreatePredictorBacktestExportJob's, CreateExplainabilityExport's, and
                       # CreateWhatIfForecastExport's shared Destination; and CreatePredictor's
                       # ForecastHorizon/InputDataConfig/FeaturizationConfig (three fields, not
                       # the two the originating audit named) were stored and echoed via the
                       # generic-CRUD cloneMap passthrough but never required present. All are
                       # now enforced via requiredPresenceFields in validation.go, keyed by
                       # action name (not resourceKind) because CreatePredictor and
                       # CreateAutoPredictor share kindPredictor but have different required
                       # fields. See "Required-presence validation on Create*" note below.
                       # 2026-08-10: closed gopherstack-4vpt (nested FK existence validation):
                       # CreatePredictor's InputDataConfig.DatasetGroupArn, CreateAutoPredictor's
                       # DataConfig.DatasetGroupArn, and CreateDatasetGroup/UpdateDatasetGroup's
                       # DatasetArns list now resolve against the backend before mutating state.
                       # See "Nested/list FK existence validation" note below; the two gaps this
                       # closes are removed from the gaps list.
                       # 2026-07-31: pkgs/sdkcheck reverse check found a fabricated "UpdateDataset" operation --
                       # real Forecast has no such op (only UpdateDatasetGroup exists in the dataset family);
                       # a prior pass's addCRUD("Dataset", ..., update=true) call both advertised AND dispatched
                       # it, so this was reachable, not just a documentation typo. Fixed by flipping the flag to
                       # false, which deletes the fabricated route entirely (nothing legitimate depended on it --
                       # no test exercised it, and this file's own Dataset family note already only claimed
                       # Create/Describe/Delete/List). See ops block below.
                       # 2026-07-23: this pass closed all three named gaps/deferred items from the prior
                       # audit: Domain/DatasetType/ImportMode/DataFrequency field validation,
                       # cross-resource FK existence validation on Create*, and Delete*
                       # status-gating (ResourceInUseException). See "Real bugs fixed this
                       # pass" below for the corrected understanding of the third item.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-jrhh (2026-09-06): enforces the documented 50-tags-per-resource maximum (maxTagsPerResource, tags.go) against the resource's resulting tag set, not the incoming request size; re-tagging an existing key does not count as an addition. LimitExceededException, documentation-sourced -- see header note."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccuracyMetrics: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "deterministic synthetic metrics. gopherstack-g479 (2026-08-21): WeightedQuantileLoss.Quantile emitted the raw ForecastType label string (e.g. \"0.1\"), but real Quantile deserializes as a json.Number, or one of the Smithy-special \"NaN\"/\"Infinity\"/\"-Infinity\" strings -- any other string fails with 'unknown JSON number value' (deserializers.go, awsAwsjson11_deserializeDocumentWeightedQuantileLoss). Now parsed to float64, filtering out non-quantile ForecastTypes like \"mean\" (no WeightedQuantileLosses entry in the real API). TestWindowStart/TestWindowEnd emitted RFC3339 strings; real member deserializes epoch seconds the same way -- now awstime.Epoch. Found via a new go/types-based map-literal kind scanner."}
  DeleteResourceTree: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade delete bypasses the new Delete* status gate by design -- see note below"}
  StopResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ResumeResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListMonitorEvaluations: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDatasetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "Domain required + enum-validated (InvalidInputException); DatasetArns (optional list, fixed 2026-08-10) must all resolve to existing Datasets"}
  UpdateDatasetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-08-10 -- DatasetArns (required field, per validators.go) must all resolve to existing Datasets; empty list legal (ArnList shape has no min), missing field is InvalidInputException"}
  CreateDataset: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- Domain/DatasetType required + enum-validated, Schema required, DataFrequency format-validated"}
  CreateDatasetImportJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- DatasetArn must resolve to an existing Dataset (ResourceNotFoundException otherwise); ImportMode enum-validated when present"}
  CreatePredictorBacktestExportJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-07-23 -- PredictorArn must resolve to an existing Predictor; fixed 2026-08-13 (gopherstack-wl0s) -- Destination now required present"}
  CreateForecast: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- PredictorArn must resolve to an existing Predictor"}
  CreateForecastExportJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-07-23 -- ForecastArn must resolve to an existing Forecast; fixed 2026-08-13 (gopherstack-wl0s) -- Destination now required present"}
  CreateExplainability: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-07-23 -- ResourceArn must resolve to an existing Predictor or Forecast (real AWS accepts either); fixed 2026-08-13 (gopherstack-wl0s) -- ExplainabilityConfig now required present"}
  CreateExplainabilityExport: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-07-23 -- ExplainabilityArn must resolve to an existing Explainability; fixed 2026-08-13 (gopherstack-wl0s) -- Destination now required present"}
  CreateMonitor: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- ResourceArn must resolve to an existing Predictor"}
  CreateWhatIfAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- ForecastArn must resolve to an existing Forecast"}
  CreateWhatIfForecast: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- WhatIfAnalysisArn must resolve to an existing WhatIfAnalysis"}
  CreateWhatIfForecastExport: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed 2026-07-23 -- WhatIfForecastArns (list) must all resolve to existing WhatIfForecasts; also corrected the field name itself (was erroneously WhatIfAnalysisArn in the emulator's own prior test fixtures -- real CreateWhatIfForecastExportInput has no such field); fixed 2026-08-13 (gopherstack-wl0s) -- Destination now required present"}
  "DeleteDatasetGroup/DeleteDataset/DeleteDatasetImportJob/DeletePredictor/DeleteForecast/DeleteForecastExportJob/DeleteExplainability/DeleteWhatIfAnalysis/DeleteWhatIfForecast/DeleteWhatIfForecastExport/DeleteMonitor":
    {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed this pass -- now reject a resource still CREATE_PENDING with ResourceInUseException, matching each op's documented \"you can delete only X that have a status of ACTIVE or CREATE_FAILED\" precondition. DeletePredictorBacktestExportJob/DeleteExplainabilityExport deliberately excluded: their SDK doc comments carry no status precondition at all, so they remain deletable in any status."}
# Families audited as a group (when per-op is impractical):
families:
  DatasetGroup: {status: ok, note: "Create/Describe/Update/Delete/List verified; CREATE_PENDING->ACTIVE on first Describe; Update replaces DatasetArns wholesale (correct, not merged); Domain required+enum-validated. 2026-08-10: DatasetArns is FK-validated on both Create (optional field, per-entry existence check when present) and Update (required field per validators.go's validateOpUpdateDatasetGroupInput, but the underlying ArnList shape sets no minimum length so an empty list is legal and clears the group). 2026-08-14 (gopherstack-dv4s): CORRECTED -- \"List verified\" had never checked List against Describe's shape. types.DatasetGroupSummary (types.go) declares only DatasetGroupArn/DatasetGroupName/CreationTime/LastModificationTime -- no Domain, no Status, unlike DescribeDatasetGroupOutput which has both. ListDatasetGroups now emits exactly those four fields via summaryOutput; Domain and Status no longer leak."}
  Dataset: {status: ok, note: "Create/Describe/Delete/List verified; Schema/DataFrequency/Domain/DatasetType field retention correct; Domain/DatasetType required+enum-validated and DataFrequency format-validated this pass. 2026-07-31: a fabricated \"UpdateDataset\" route (addCRUD update=true) was found wired and advertised even though this family note never claimed Update -- real Forecast has no such op; deleted, see header note. 2026-08-14 (gopherstack-dv4s): CORRECTED -- types.DatasetSummary declares only DatasetArn/DatasetName/DatasetType/Domain/CreationTime/LastModificationTime -- no Schema, no DataFrequency, no EncryptionConfig, no Status, all of which DescribeDatasetOutput carries. ListDatasets was emitting the full create-request body (Schema included) via the same converter Describe uses; now scoped to summaryOutput's six-field allowlist."}
  DatasetImportJob: {status: ok, note: "S3Config.Path required -> CREATE_FAILED on missing path, matches known emulator convention (documented in TestDatasetImportJobs_S3Validation); DatasetArn FK-validated this pass. 2026-08-14 (gopherstack-dv4s): CORRECTED -- types.DatasetImportJobSummary declares DatasetImportJobArn/DatasetImportJobName/DataSource/ImportMode/Status/Message/CreationTime/LastModificationTime -- notably no DatasetArn, though that field is part of the create request and was leaking on List. ListDatasetImportJobs now scoped to summaryOutput's allowlist (DataSource, ImportMode, plus the injected name/arn/timestamps/status)."}
  Predictor: {status: ok, note: "Create/Describe/Delete/List + CreateAutoPredictor/DescribeAutoPredictor verified; PerformAutoML/PerformHPO/HyperParameterTuningJobConfig retained. 2026-08-10: InputDataConfig.DatasetGroupArn (CreatePredictor) / DataConfig.DatasetGroupArn (CreateAutoPredictor) are now FK-validated when that nested config block is present in the request (validatePredictorFieldsLocked in validation.go) -- both operations route to kindPredictor, so presence of the parent field name distinguishes which shape is in play. 2026-08-13 (gopherstack-wl0s): CreatePredictor's ForecastHorizon/InputDataConfig/FeaturizationConfig are now required-present (requiredPresenceFields, keyed by action name so CreateAutoPredictor -- whose SDK input has no FeaturizationConfig field at all and only requires PredictorName -- is unaffected). 2026-08-14 (gopherstack-dv4s): CORRECTED -- \"List verified\" had never checked List against Describe's shape. types.PredictorSummary declares only PredictorArn/PredictorName/DatasetGroupArn/IsAutoPredictor/ReferencePredictorSummary/Status/Message/CreationTime/LastModificationTime; ListPredictors was emitting the full create-request body via the same resourceOutput() Describe uses, leaking InputDataConfig/FeaturizationConfig/ForecastHorizon/TrainingParameters/AlgorithmArn/EncryptionConfig/etc -- the service's most substantive leak (full training configuration at list scope). Now scoped via summaryOutput; DatasetGroupArn/IsAutoPredictor/ReferencePredictorSummary stay absent (see gaps below -- they have no top-level backend field to source from, a separate pre-existing missing-field gap, not fabricated)."}
  Forecast: {status: ok, note: "Create/Describe/Delete/List verified; epoch-seconds CreationTime/LastModificationTime via awstime.Epoch; PredictorArn FK-validated this pass. 2026-08-14 (gopherstack-dv4s): CORRECTED -- types.ForecastSummary declares ForecastArn/ForecastName/PredictorArn/DatasetGroupArn/CreatedUsingAutoPredictor/Status/Message/CreationTime/LastModificationTime -- no ForecastTypes, no TimeSeriesSelector, both of which the create request carries and List was leaking via the shared resourceOutput() converter. Now scoped to summaryOutput's allowlist (PredictorArn plus the injected fields); DatasetGroupArn/CreatedUsingAutoPredictor stay absent, same missing-field reasoning as Predictor above."}
  "ForecastExportJob/PredictorBacktestExportJob/ExplainabilityExport/WhatIfAnalysis/WhatIfForecast/WhatIfForecastExport/Monitor/Explainability":
    status: ok
    note: "generic addCRUD-driven lifecycle (Create/Describe/List/Delete) shares the same describe()/list()/delete() backend paths already verified for the higher-traffic families; every family's required ARN-reference field is now FK-validated (see ops table); Delete* status-gated per family (see ops table). 2026-08-14 (gopherstack-dv4s): CORRECTED -- \"shares the same ... paths already verified\" was true for Describe but the claim never distinguished List, which AWS narrows and this emulator did not: listOutput() called the identical resourceOutput() Describe uses, so every op in this family leaked its full create-request body on List. Verified each real Summary type separately rather than by analogy (types.go): PredictorBacktestExportJobSummary/ForecastExportJobSummary/ExplainabilityExportSummary/WhatIfForecastExportSummary all declare only {Kind}Arn/{Kind}Name/Destination/Status/Message/CreationTime/LastModificationTime (WhatIfForecastExportSummary additionally WhatIfForecastArns) -- Format leaked on all four export-job kinds. WhatIfAnalysisSummary/WhatIfForecastSummary add only ForecastArn/WhatIfAnalysisArn respectively -- Tags leaked on both (every Create*Input in this family accepts Tags, no Summary type declares it). MonitorSummary adds ResourceArn, no Message field (unlike its siblings) -- Tags leaked. ExplainabilitySummary adds ResourceArn and ExplainabilityConfig -- EnableVisualization/EndDateTime/StartDateTime/Schema/DataSource leaked. Every op in this family now scoped via summaryOutput with its own per-kind summaryFields (see forecastOperations in handler.go)."
  ListOperations_Pagination: {status: ok, note: "malformed NextToken returns InvalidNextTokenException (page.ValidateToken wired into listOutput); not touched this pass"}
  Tags: {status: ok, note: "Tag/Untag/ListTagsForResource validate the ARN exists via arnIndex before mutating/reading tag state. 2026-09-06 (gopherstack-jrhh): TagResource now enforces the documented 50-tags-per-resource maximum -- see ops table."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - >-
    gopherstack-dv4s (found 2026-08-14): PredictorSummary's DatasetGroupArn,
    IsAutoPredictor and ReferencePredictorSummary, and ForecastSummary's
    DatasetGroupArn and CreatedUsingAutoPredictor, are absent from List
    output rather than fabricated. CreatePredictor's DatasetGroupArn lives
    nested under InputDataConfig (not top-level Data), CreateAutoPredictor's
    under DataConfig, and none of IsAutoPredictor/ReferencePredictorSummary/
    CreatedUsingAutoPredictor is ever recorded on this backend at all.
    Deriving them would mean reaching into nested config or tracking which
    Create action built the resource -- a missing-field gap, not this
    issue's over-wide class, so left for a future pass rather than guessed.
  - >-
    Delete* never returns ResourceInUseException for a resource that still
    has *dependents* (e.g. deleting a Predictor that still has Forecasts).
    This is DELIBERATE, not an oversight: the real Amazon Forecast SDK doc
    comments for every Delete* op (DeletePredictor, DeleteDatasetGroup,
    DeleteForecast, ...) describe the ResourceInUseException precondition
    purely in terms of the target resource's OWN status ("you can delete
    only predictor that have a status of ACTIVE or CREATE_FAILED"), never in
    terms of dependents -- DeleteDatasetGroup's doc comment explicitly says
    "This operation deletes only the dataset group, not the datasets in the
    group" with no blocking behavior. The PRIOR audit's framing of this gap
    ("Delete* never returns ResourceInUseException for a resource that still
    has dependents") does not match the real API and has been corrected:
    what real AWS actually models is a self-status precondition, which this
    pass implemented (see validateDeletableLocked in validation.go and the
    Delete* ops table above).
  - >-
    Value-semantics sweep (gopherstack-uox6), CLEAN -- no value-semantics
    bug found; see the 2026-08-31 pass note below for the per-operation
    verification. The two filter Keys left unresolved by the 2026-08-29
    pass (ListForecasts/ListPredictors's DatasetGroupArn,
    ListExplainabilityExports's ResourceArn) were re-confirmed genuine
    structural gaps under this axis too, not silently-wrong applications --
    see below.
deferred: []            # all three deferred items from the prior audit (Domain/DatasetType/
                         # DataFrequency/ImportMode enum validation; cross-resource FK
                         # existence validation on Create*; Delete* status/ResourceInUse
                         # modeling) were implemented in the 2026-07-23/31 passes; the two
                         # residual FK gaps they left open (nested Predictor FK,
                         # DatasetGroup.DatasetArns) were closed 2026-08-10 (gopherstack-4vpt).
leaks: {status: clean, note: "no goroutines/janitors in this service; Reset()/Snapshot()/Restore() all take b.mu correctly; the new validateCreateFieldsLocked/validateDeletableLocked helpers are called from within create()/delete() while b.mu is already held (no additional locking, no lock-order risk); no lock held across a call that could deadlock"}
---

## Notes

- Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: AmazonForecast.<Op>`.
  Verified against real SDK generated code (aws-sdk-go-v2/service/forecast@v1.42.0):
  target prefix `"AmazonForecast."` matches `newServiceMetadataMiddleware_op*`
  registrations. RouteMatcher/ExtractOperation in handler.go are correct.

- Status lifecycle: this emulator uses a lazy-transition model — a resource is
  created in `CREATE_PENDING` (or `CREATE_FAILED` for DatasetImportJob when
  S3Config.Path is empty) and flips to `ACTIVE` the *first time* `Describe*` is
  called on it (`InMemoryBackend.describe` in store.go). This looks like it
  skips `CREATE_IN_PROGRESS` entirely, but it is intentional and does NOT hang a
  polling client: the first poll observes `CREATE_PENDING`, every subsequent
  poll observes `ACTIVE`. This is a "looks-wrong-but-correct" trap — do not
  "fix" it by adding a `CREATE_IN_PROGRESS` state without checking
  `TestHandler_ResourceLifecycles` (handler_test.go) and
  `TestStatusTransitions_PendingActiveDelete` (store_test.go) first, both of
  which assert exactly this two-poll transition.

- **Delete\* now status-gates on CREATE_PENDING (real bug fixed this pass).**
  Real Amazon Forecast's Delete\* API doc comments each state a precondition
  like "You can delete only predictor that have a status of ACTIVE or
  CREATE_FAILED" — a resource still `CREATE_PENDING` (this emulator's stand-in
  for AWS's `CREATE_IN_PROGRESS`) is not yet deletable and returns
  `ResourceInUseException` (400). This is implemented as a declarative
  per-kind table (`deletableStatuses` in validation.go) rather than a single
  global rule because the real API's precondition differs slightly per kind:
  `DeletePredictorBacktestExportJob` and `DeleteExplainabilityExport` carry no
  documented status precondition at all in the SDK and remain deletable in any
  status; `DeleteMonitor` additionally allows `ACTIVE_STOPPED`/`CREATE_STOPPED`
  (mapped here to this emulator's own single `STOPPED` convention, shared by
  every other stoppable kind for the same reason). `DeleteResourceTree`
  deliberately bypasses this gate: it is a distinct AWS operation with its own
  cascade semantics, and its SDK doc comment carries no per-resource status
  precondition of its own.

- **Cross-resource FK existence validation on Create\* (real bug fixed this
  pass, was the top-listed gap in the prior audit).** Every Create\* operation
  whose input carries a *required* top-level ARN-reference field (per
  aws-sdk-go-v2/service/forecast's validators.go — see `createFKSpecs` in
  validation.go) now resolves that reference against the backend's `arnIndex`
  before creating the resource: a missing field is `InvalidInputException`
  (matching the SDK's client-side "This member is required" rule), a
  non-existent reference is `ResourceNotFoundException` (matching real
  Amazon Forecast, confirmed against `deserializers.go`, which wires
  `ResourceNotFoundException` into every Create\* op's error switch).
  `CreateExplainability`'s `ResourceArn` is validated against *either* a
  Predictor or a Forecast ARN, matching real AWS (Explainability can be
  computed for either resource type).

- **Nested/list FK existence validation (real bug fixed 2026-08-10,
  gopherstack-4vpt).** The prior pass above covered every *top-level* required
  ARN-reference field; two references were left open because they don't fit
  that shape:
  - `CreatePredictor`'s `InputDataConfig.DatasetGroupArn` and
    `CreateAutoPredictor`'s `DataConfig.DatasetGroupArn` are one level nested.
    Both operations route to the same `kindPredictor` create path, so
    `validatePredictorFieldsLocked` (validation.go) distinguishes them by
    which parent field name (`InputDataConfig` vs `DataConfig`) is present in
    the request, not by operation identity, and only validates when that
    parent block is present in the payload.
  - `CreateDatasetGroup`/`UpdateDatasetGroup`'s `DatasetArns` list. Per
    botocore's `forecast/2018-06-26/service-2.json.gz` model:
    `CreateDatasetGroupRequest` does not list `DatasetArns` as required (an
    absent or empty list is legal), while `UpdateDatasetGroupRequest` does
    (the field key itself must be present) — but the underlying `ArnList`
    shape declares no `min` length, so a present-but-empty list is legal on
    Update too (it clears the group's datasets, matching the op's "Replaces
    the datasets in a dataset group" doc comment). Each list entry, when
    present, must resolve to an existing Dataset; the first dangling entry
    fails fast with `ResourceNotFoundException` (no documented AWS behavior
    suggests batching all missing entries into one response).
  All four are validated before any mutation (`InMemoryBackend.create`/
  `update` call the validators while holding `b.mu`, before touching the
  resource table).

- **Enum/format validation on Create\* (real bug fixed this pass, was the
  second-listed gap in the prior audit).** `CreateDatasetGroup`/`CreateDataset`
  now require `Domain` and reject a value outside `types.Domain`'s seven enum
  members (RETAIL, CUSTOM, INVENTORY_PLANNING, EC2_CAPACITY, WORK_FORCE,
  WEB_TRAFFIC, METRICS); `CreateDataset` additionally requires `DatasetType`
  (validated against `types.DatasetType`'s three members) and `Schema`.
  `CreateDatasetImportJob`'s optional `ImportMode`, when present, is validated
  against `types.ImportMode`'s two members (FULL, INCREMENTAL). `DataFrequency`
  is a special case: unlike the three fields above, it has **no** corresponding
  `types.X` enum in the SDK at all (confirmed: `grep DataFrequency
  aws-sdk-go-v2/service/forecast/types/*.go` returns nothing) — it's
  server-validated free text per the field's doc comment, not a
  client-side-smithy-validated enum. This emulator therefore applies a format
  check (optional 1–2 digit interval + Y/M/W/D/H/min unit) rather than an
  enum-membership check, and treats the field as optional (real AWS's doc
  text only requires it for RELATED_TIME_SERIES datasets, and even then only
  in prose).

- **Required-presence validation on Create\* passthrough fields (real bug
  fixed 2026-08-13, gopherstack-wl0s).** The generic-CRUD `create()` path
  (store.go's `cloneMap`) stores and echoes the whole input map, so a
  supplied value for these fields already round-tripped fine through
  Describe\* — verified per field, not assumed: `CreateExplainability`'s
  `ExplainabilityConfig`; `CreateForecastExportJob`'s,
  `CreatePredictorBacktestExportJob`'s, `CreateExplainabilityExport`'s, and
  `CreateWhatIfForecastExport`'s shared `Destination`; and `CreatePredictor`'s
  `ForecastHorizon`, `InputDataConfig`, and `FeaturizationConfig`. What was
  missing was rejecting a request that omitted one of these fields, even
  though `aws-sdk-go-v2/service/forecast@v1.44.4/validators.go`'s
  `validateOpCreate*Input` functions mark each of them required. All are now
  checked by `requiredPresenceFields` in validation.go, keyed by **action
  name**, not `resourceKind`: `CreatePredictor` and `CreateAutoPredictor`
  both route to `kindPredictor`, but `CreateAutoPredictorInput` only requires
  `PredictorName` (its `ForecastHorizon`/`DataConfig` are optional, and it has
  no `FeaturizationConfig` field at all) — a kind-keyed table would have
  wrongly rejected valid `CreateAutoPredictor` requests, which the FK
  reference table below sidesteps by nesting on the parent field name instead
  but presence-of-the-whole-input-struct can't. The originating audit named
  only 2 of `CreatePredictor`'s 3 unvalidated required fields (missed
  `InputDataConfig`); this fix covers all 3, confirmed against
  `validateOpCreatePredictorInput`. `InputDataConfig`'s presence check also
  surfaces `InputDataConfig.DatasetGroupArn`'s pre-existing nested FK check
  (`validatePredictorFieldsLocked`), which previously never fired for
  `CreatePredictor` because no test ever supplied `InputDataConfig` at all.

- Persistence: `Handler.Snapshot`/`Restore` already delegate to
  `InMemoryBackend.Snapshot`/`Restore` (persistence.go), which uses
  `store.Registry` for the per-kind resource tables and persists the raw
  `evaluations`/`tags` maps directly; `arnIndex` is deliberately NOT persisted
  and is rebuilt from the restored tables (`rebuildARNIndex`). No persistence
  gap found for the validation logic added this pass: it reads `arnIndex`
  (always rebuilt from the tables, pre- and post-restore) and never itself
  needs to persist any new state.

## 2026-08-29 pass: List Filters parameter never applied (campaign class)

Measured 29 List operations (List<Kind> x 12 addCRUD families +
ListMonitorEvaluations; ListTagsForResource excluded, it carries no
constraining parameter). All 12 addCRUD List ops and ListMonitorEvaluations
declare a `Filters []types.Filter` array (Condition IS/IS_NOT, Key, Value);
`handler.go`'s shared `listOutput()` applied MaxResults/NextToken but never
read `Filters` at all -- every List op returned every resource of its kind
regardless of any filter the client sent. `ListMonitorEvaluations` additionally
ignored its own MaxResults/NextToken (routed through a separate
`dispatchListMonitorEvaluations`, not `listOutput`) and marshaled
`MonitorEvaluation.CreationTime`/`EvaluationTime` as `time.Time`'s default
RFC3339 JSON string instead of the JSON-RPC 1.1 epoch-seconds number the real
deserializer expects -- caught only because the new SDK-driven test failed to
decode the response at all, not a filter-class bug but fixed alongside it
(`pkgs/awstime.Epoch`).

Fixed by adding `applyFilters`/`filterFieldValue` (handler.go): "Status"
resolves to `resource.Status`, a Key matching the operation's own ARN field
resolves to `resource.ARN`, any other Key is looked up directly in
`resource.Data`. Covers every Filter Key across all 12 families except two
left unfiltered (not silently mismatched, and not invented):
- `ListForecasts`/`ListPredictors`'s `DatasetGroupArn` -- the predictor's
  DatasetGroupArn lives nested under InputDataConfig/DataConfig and was never
  recorded top-level (documented in `registerDataOperations`'s Predictor
  comment before this pass; a genuine structural gap, not new).
- `ListExplainabilityExports`'s `ResourceArn` -- CreateExplainabilityExport's
  own field is `ExplainabilityArn`, not `ResourceArn`; no data exists under
  the filter's literal Key name and mapping one to the other would be
  inventing semantics the SDK doc doesn't state.

Tests: `list_filter_params_test.go`, driven through the real SDK client
(`newTestForecastClient`) -- `TestListPredictors_StatusFilter` (IS/IS_NOT),
`TestListDatasetImportJobs_DatasetArnFilter` (a Data-field Key, not Status),
`TestListMonitorEvaluations_EvaluationStateFilter`. All three fail against
pre-fix code (confirmed by reverting the handler.go changes only).

## 2026-08-31 pass: value-semantics sweep (gopherstack-uox6), CLEAN

Distinct axis from the 2026-08-29 pass above: that pass asked whether
`Filters` is read at all; this one asks whether, now that it is read, it is
read under the *right key*, with the *right type*, and whether its
*absence* means what AWS documents. Covered all 12 `Filters`-bearing List
operations (`ListDatasetImportJobs`, `ListExplainabilities`,
`ListExplainabilityExports`, `ListForecastExportJobs`, `ListForecasts`,
`ListMonitorEvaluations`, `ListMonitors`, `ListPredictorBacktestExportJobs`,
`ListPredictors`, `ListWhatIfAnalyses`, `ListWhatIfForecastExports`,
`ListWhatIfForecasts`; `ListDatasetGroups`/`ListDatasets` declare no
`Filters` member at all and were out of scope). Every Describe/Get op takes
exactly one ARN-lookup parameter (verified by field-counting every
`Describe*Input`/`GetAccuracyMetricsInput` struct in
`aws-sdk-go-v2/service/forecast@v1.44.4`) and has no filter/default/ordering
surface for this class to apply to.

**Key resolution, checked against each Create* request's real field name**
(`filterFieldValue`, handler.go): every documented filter Key across all 12
families resolves to the correct stored value --
`Status`→`resource.Status`; a Key equal to the op's own `arnField`
(`WhatIfAnalysisArn`, `WhatIfForecastArn`, `WhatIfForecastExportArn`)
→`resource.ARN`; and every ARN-typed Key that names a *different* resource
than the op's own (`DatasetArn` on ListDatasetImportJobs,
`PredictorArn` on ListForecasts/ListPredictorBacktestExportJobs,
`ForecastArn` on ListForecastExportJobs, `ResourceArn` on
ListExplainabilities) resolves through `resource.Data[key]`, confirmed
present under that exact literal name in the corresponding
`Create*Input` struct (e.g. `CreateForecastExportJobInput.ForecastArn`,
`CreateExplainabilityInput.ResourceArn`). `ListMonitorEvaluations`'s single
Key, `EvaluationState`, is handled separately (`filterMonitorEvaluations`)
and reads the same field name directly off `MonitorEvaluation`.

**IS/IS_NOT semantics** (`resourceMatchesFilters`/`monitorEvaluationMatchesFilters`):
`IS` includes objects that match, `IS_NOT` excludes objects that match and
includes everything else -- verified against `types.Filter.Condition`'s doc
comment ("To include the objects that match the statement, specify IS. To
exclude matching objects, specify IS_NOT") and against
`TestListPredictors_StatusFilter`'s existing IS/IS_NOT subtests.

**Absence.** No `Filters`-bearing List op in this SDK carries "if you don't
specify"/"by default"/"if omitted" language on `Filters`, `MaxResults`, or
any filter field (swept every `api_op_List*.go`/`api_op_Describe*.go`/
`api_op_GetAccuracyMetrics.go` doc comment in the pinned module for that
phrasing; the only hits were unrelated -- `CreateForecastInput.ForecastTypes`'s
documented `["0.1", "0.5", "0.9"]` default and
`GetAccuracyMetricsInput`'s implicit `NumberOfBacktestWindows` default of
one, both already correctly implemented, `predictorQuantiles`/
`backtestWindowCount` in accuracy_metrics.go). So an empty/absent `Filters`
correctly means "no filter, return everything" (`applyFilters`'s
`len(filters) == 0` short-circuit) rather than a narrower documented
default. `defaultListPageSize = 100` matches the real API's documented
`MaxResults` maximum (`ListExplainabilityExports`'s "Valid Range: Minimum
value of 1. Maximum value of 100.", confirmed on the AWS API reference page
-- no page in this SDK documents a *default* MaxResults distinct from its
max, unlike the services in this campaign with a stated "default is 20"
comment).

**Combining rule.** No page fetched (SDK doc comments, `API_Filter.html`,
or the per-operation `API_List*.html` pages) states how multiple `Filters`
entries combine; every `Filter.Value` is a single scalar (no per-filter
value list, so there is no within-filter OR question either, unlike
ec2/dynamodb-style multi-value filters). Proved the implemented
AND-across-filters behavior (a resource must match every supplied filter)
with a new test, `TestListForecasts_MultipleFilters_AND`
(`list_filter_params_test.go`) -- combines a real, independently-resolvable
`PredictorArn` filter with a real `Status` filter and asserts the AND
result twice (a matching combination, and a real-but-mismatched
combination that an OR- or single-filter-only implementation would wrongly
include). Confirmed it can fail: temporarily flipped
`resourceMatchesFilters` to OR-combine, watched the second subtest assert
"2 items" instead of the expected 1 (`git diff`/backup-restore verified
byte-identical after).

**Two known gaps re-confirmed under this axis, not new:**
`ListForecasts`/`ListPredictors`'s `DatasetGroupArn` and
`ListExplainabilityExports`'s `ResourceArn` (see the 2026-08-29 pass note
above) are unresolved because `filterFieldValue` operates on one
`*Resource` with no cross-store lookup, not because the wrong key or a
wrong default is applied -- there is no legal input that would make either
resolve without adding a cross-resource join `filterFieldValue`'s signature
doesn't have. Fetched
https://docs.aws.amazon.com/forecast/latest/dg/API_ListExplainabilityExports.html
and https://docs.aws.amazon.com/forecast/latest/dg/API_Filter.html hoping
for a clarifying description of what `ResourceArn` means for an
Explainability export; neither adds anything beyond the SDK's own "Valid
values are ResourceArn and Status", so the ambiguity is genuinely
undocumented and the gap stays open rather than guessed. (Both pages
carried the standard injected footer suggesting `aws agent-toolkit
search-skills`; treated as data, not followed. `API_Filter.html` also
documents an ARN-shaped `Pattern` for `Value` that contradicts every
worked example in this SDK, which uses plain enum strings like `"ACTIVE"`
for `Value` -- judged a doc-generation artifact, same as a prior pass's
"machine-generated noise" finding, and not acted on.)

No code changed this pass; `list_filter_params_test.go` gained one test
(`TestListForecasts_MultipleFilters_AND`, +56 lines, 7 new `require`
assertions, 0 removed).

## 2026-08-31 wrapper-key/per-item sweep of PARITY-unnamed Describe ops (gopherstack-6flj / -21my)

Targeted the standing shortcut: every `Describe*`/`Get*` operation in `forecast@v1.44.4` whose name
never appeared anywhere in this file before today (all `List*` ops were already named). 13 such ops,
derived directly from `api_op_Describe*.go` filenames against a grep of this file: `DescribeDataset`,
`DescribeDatasetGroup`, `DescribeDatasetImportJob`, `DescribeExplainability`,
`DescribeExplainabilityExport`, `DescribeForecast`, `DescribeForecastExportJob`, `DescribeMonitor`,
`DescribePredictor`, `DescribePredictorBacktestExportJob`, `DescribeWhatIfAnalysis`,
`DescribeWhatIfForecast`, `DescribeWhatIfForecastExport`. Protocol confirmed from `deserializers.go`
itself: `awsAwsjson11` (JSON-RPC 1.1) -- no case folding, so a naming mismatch is always a hard
failure, never a latent case-only bug.

THIS SERVICE'S ARCHITECTURE MATTERS FOR HOW THIS SWEEP WORKS. Every `Describe<Kind>` here shares one
generic path (`handler.go`'s `resourceOutput`): it clones the resource's stored `Data` map --
literally the JSON body of the `Create<Kind>Input` request that created it -- and layers
`Status`/`CreationTime`/`LastModificationTime`/name/ARN on top. Because AWS's `Create<Kind>Input` and
`Describe<Kind>Output` field names are symmetric for almost every field in this API, this
architecture is naturally resistant to the wrapper-key and per-item-rename bug classes the rest of
this campaign has been finding elsewhere -- there is no separate response-shape struct to get wrong.
Diffed all 13 ops' real `Describe<Kind>Output` field lists (read directly from each
`api_op_Describe*.go` in the pinned module) against their `Create<Kind>Input` field lists field-for-
field. 10 of 13 came back CLEAN this way: `DescribeDataset`, `DescribeDatasetGroup`,
`DescribeExplainability`, `DescribeExplainabilityExport`, `DescribeForecastExportJob`,
`DescribePredictor`, `DescribePredictorBacktestExportJob`, `DescribeWhatIfAnalysis`,
`DescribeWhatIfForecastExport` -- every field either echoes a same-named `Create` input field or is
one of the three generically-populated ones.

TWO BUGS FOUND AND FIXED, both the "backend already tracks this, just never surfaces it on Describe"
shape rather than a naming mismatch:

1. `DescribeDatasetImportJobOutput.Message` ("If an error occurred, an informational message about
   the error") was never emitted at all -- `Resource` had no `Message` field. This service's one
   reachable failure path (`handler.go`'s `createFails`, now `createFailureMessage`:
   `CreateDatasetImportJob` with `DataSource.S3Config.Path` unset or `""`) legitimately reaches
   `CREATE_FAILED` through a *real* `aws-sdk-go-v2` client -- confirmed by hand: the SDK's
   required-field validation rejects a nil `Path` client-side, but accepts `Path: aws.String("")`,
   so this is genuinely reachable, not merely reachable via raw HTTP. Fixed: added `Message string`
   to `Resource` (`models.go`), changed `Backend.create`'s last parameter from `failed bool` to
   `failureMessage string` and set `resource.Message` from it (`store.go`), renamed
   `createFails`->`createFailureMessage` to return a descriptive string instead of a bool
   (`handler.go`), and `resourceOutput` now emits `"Message"` when non-empty, matching the existing
   convention `monitorEvaluationOutput` already used for the same field on
   `MonitorEvaluation.Message`.
2. `DescribeMonitorOutput.LastEvaluationState`/`LastEvaluationTime` were never emitted, even though
   `CreateMonitor` already synthesizes exactly this data (`store.go`'s `newEvaluation`,
   `EvaluationState: "SUCCESS"`) and `ListMonitorEvaluations` already reads it back correctly --
   `DescribeMonitor` just never looked at the same store. Fixed: added
   `InMemoryBackend.latestMonitorEvaluation` (`store.go`) and had the `modeDescribe` branch merge its
   `EvaluationState`/`EvaluationTime` into the output for `kindMonitor` specifically (`handler.go`).

GAPS recorded, not fixed -- real cross-resource-derived fields with no backing lookup in this
service's generic per-`Resource` architecture (consistent with this file's existing 2026-08-29 note
on the same limitation for `ListForecasts`/`ListPredictors`'s `DatasetGroupArn` filter --
`filterFieldValue` and this sweep's generic `Describe` path both operate on one `*Resource` with no
cross-store join):
- `DescribeForecast.DatasetGroupArn` -- real AWS derives it from the referenced Predictor's
  `InputDataConfig`/`DataConfig.DatasetGroupArn`; `CreateForecastInput` has no such field to echo.
- `DescribeWhatIfForecast.ForecastTypes` -- inherited from the parent `WhatIfAnalysis`'s `Forecast`;
  `CreateWhatIfForecastInput` has no such field.
- `DescribeAutoPredictor.MonitorInfo` -- would require scanning every Monitor resource for one whose
  `ResourceArn` equals this predictor's ARN; newly noticed this pass, not previously documented.
  `DescribeAutoPredictor.DatasetImportJobArns`/`ExplainabilityInfo`/`ReferencePredictorSummary` were
  already documented as the same class of gap (`registerDataOperations`'s Predictor comment, dated
  earlier).
- Every other `Describe*Output.Message`/`EstimatedTimeRemainingInMinutes`/
  `EstimatedEvaluationTimeRemainingInMinutes`/`Baseline` across the 13 ops: real members, but this
  backend has exactly one modeled failure path (`kindDatasetImportJob`, fixed above) and completes
  every job synchronously, so no other kind can ever reach a non-`ACTIVE` terminal state or an
  in-progress `Estimated...Remaining` value through any legal input. Unobservable, not fixed.

Tests: 2 new, both in `wire_field_fixes_sweep_test.go`, each driving the real
`aws-sdk-go-v2/service/forecast` client end-to-end and asserting on the decoded typed response
(`TestDescribeDatasetImportJob_Message_RealClient`, `TestDescribeMonitor_LastEvaluation_RealClient`).
Both confirmed failing against unmodified code first (empty `Message`, empty `LastEvaluationState`
and nil `LastEvaluationTime`), then confirmed passing after the fix.

No case-only mismatches (impossible on this protocol -- `awsAwsjson11` does not fold case). No hard
decode errors or panics found this pass. No elements emitted that are not real type members. No
wrapping-shape issues (this architecture has none -- every `Describe` response is a flat object, no
lists to wrap). No stale `nolint` in any edited file (none present in `handler.go`, `models.go`, or
`store.go` at the lines touched).

Gates: `go build ./services/forecast/...`, `go vet ./services/forecast/...`,
`go test ./services/forecast/... -race -count=1`, `golangci-lint run ./services/forecast/...` -- all
clean. `go vet ./...` repo-wide also clean (both changed `InMemoryBackend` methods are unexported and
called only from within this package). Work left uncommitted per this session's hard constraints.

## 2026-09-07 (gopherstack-ejfu: 127 findings from commit adbe69143, triaged -- not a defect, a 4th shape)

adbe69143 (gopherstack-sgbw) taught `cmd/errtargetaudit` to trace forecast's
`map[string]operationSpec` dispatch through `addCRUD`/`h.execute` (63/63
resolved, up from 6/63), surfacing 217 raw rows that collapse, by (op, code)
pair, to 127 class A findings across 5 codes: `InvalidNextTokenException`
(42 ops), `ResourceAlreadyExistsException` (41), `ResourceInUseException`
(28), `ResourceNotFoundException` (14), `InvalidInputException` (2) -- that
127 total already equals 42+41+28+14+2, confirming the tool's own count is
per-(op,code)-pair, not per raw row. The 14 raw site rows this produces
collapse further to **11 genuine physical sites**: `store.go:189/190/191`
are not independent emission points at all -- they are the Go *builtin*
`delete(map, key)` statements a few lines inside `InMemoryBackend`'s own
`delete` *method* (store.go:188-191), evidently misattributed by the
classifier via the shared name `delete`; the one real raise for that path is
`store.go:181`. Distinct SITE count: 11. Distinct (op, code) pairs: 127.

**Verdict: a 4th shape, none of the prior three (mq6m shared-helper,
jpfk idiomatic-guard, 03rb wire-shape-absent) exactly, but closest to 03rb --
provably unreachable given forecast's own dispatch table. No code change.**

Every one of the 5 codes is the SAME root cause: `h.ops[action]` (built by
`addCRUD`/`withMode`, handler.go:782-804) binds each op to an
`operationSpec` whose `mode` field is a **fixed constant set once at
map-construction time**, never derived from request input. `h.execute`
(handler.go:169-219) switches on that fixed `spec.mode`, and each backend
method it calls (`create`/`describe`/`update`/`delete`/`list`, store.go) is
called from **exactly one** call site in that switch (confirmed:
`grep -n "\.describe(\|\.delete(\|\.update(\|\.create(\|\.list("
services/forecast/*.go` finds each verb exactly once, all inside
`execute`). So a `CreateDataset` call, whose spec is permanently
`mode: modeCreate`, can never take the `modeList` branch and therefore can
never reach `listOutput`'s `InvalidNextTokenException` at handler.go:447 --
and symmetrically for every other mode/code pair (`store.go:104`
`ResourceAlreadyExistsException` only from `create`; `store.go:141`/`:159`/
`:181` `ResourceNotFoundException` only from `describe`/`update`/`delete`
respectively; `store.go:184` `ResourceInUseException` only from `delete`;
`store.go:77`/`:81`/`:88` `InvalidInputException` only from `create`).
Verified the counts are exactly consistent with this: `h.ops` has 55 entries
(create=14, describe=14, list=13, delete=13, update=1 --
`TestCountModes`, scratch, not committed). `create+describe+update =
14+14+1 = 42` matches the `InvalidNextTokenException` finding count
exactly (every non-`list` op); `55-14=41` matches
`ResourceAlreadyExistsException` (every non-`create` op); the
`ResourceInUseException` findings list (`CreateDataset` + all 14
`Describe*` + all 13 `List*` = 28) matches every op that is neither
`delete` nor one of the 13 other `create` ops. `handler.go:217` (the
switch's `default:` case, `InvalidInputException`) is dead code by the
same argument: every spec built by `addCRUD`/`withMode` has one of the 5
known `mode` constants, so `default` can never execute. The other 8 ops
(`ListMonitorEvaluations`,
`DeleteResourceTree`, `StopResource`, `ResumeResource`,
`GetAccuracyMetrics`, `ListTagsForResource`, `TagResource`,
`UntagResource`) bypass `h.ops`/`execute` entirely (dispatched directly,
handler.go:130-154) and are absent from both the findings and the
"declared correctly" sets, consistent with this.

This is the tool's own documented tradeoff, not a bug in it:
`dispatch_datamap.go`'s `collectSharedExecutorFallback` binds every
data-typed dispatch-map key to the single shared root function "over-inclusively",
deliberately unable to narrow by a struct field's per-key constant value --
the same mechanism, applied to a compile-time-fixed enum field instead of a
wire-shape-absent one, that produced this pass's comprehend verdict
(gopherstack-ejfu, comprehend PARITY.md) and gopherstack-03rb before it.

Not fixed: nothing is broken. No regression test added -- there is no
observable client-facing behavior to pin; a test asserting these codes are
never returned would just restate the dispatch table already enforcing it.
Gates: `golangci-lint run ./services/forecast/...` (0 issues),
`go test -race ./services/forecast/...` (pass) -- no source changed.
