---
service: rekognition
sdk_module: aws-sdk-go-v2/service/rekognition@v1.58.0   # version audited against (was stale at v1.51.26; go.mod pins v1.54.4 -- corrected this sweep)
last_audit_commit: 903d74b67                       # HEAD when this manifest was written
last_audit_date: 2026-08-29
overall: A            # field-completeness follow-up sweep (see Notes #6): shallow CreateProjectVersion/StartProjectVersion/CopyProjectVersion fields and async-video Get* JobTag/Video/SelectedSegmentTypes/GetRequestMetadata now modeled; deep Custom Labels manifests and post-training fields stay deliberately deferred
                       # 2026-08-29 (gopherstack wrapper-key/constraint-parameter sweep): three constraint
                       # parameters found never applied -- DescribeProjects.Features (never plumbed, and its
                       # documented default changes real behavior), ListDatasetEntries' four filters
                       # (ContainsLabels/Labeled/SourceRefContains/HasErrors, none read at all), ListFaces'
                       # FaceIds/UserId (never read). See the three rows' notes below.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateCollection: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteCollection: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades: deletes all faces in the collection + its tags"}
  DescribeCollection: {wire: ok, errors: ok, state: ok, persist: ok, note: "UserCount field omitted from response (optional, client-side nil-safe — not a bug)"}
  ListCollections: {wire: ok, errors: ok, state: ok, persist: ok}
  IndexFaces: {wire: partial, errors: ok, state: ok, persist: ok, note: "real face storage; deterministic per-identity Confidence (not canned) — see backend.go faceConfidence. FaceDetail/BoundingBox/IndexFacesModelVersion/UserId fields on Face are omitted (optional pointer fields on the real SDK type, zero-value-safe on decode). GAP found 2026-09-06 (gopherstack-eshx): indexFacesReq has no Image field at all -- IndexFacesInput's required Image member is never parsed, so IndexFaces cannot be given the InvalidS3ObjectException check this pass added to every other Image-taking op (see gaps)."}
  DeleteFaces: {wire: ok, errors: ok, state: ok, persist: ok}
  ListFaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "real pagination via facesByCollection index. FIXED (gopherstack wrapper-key sweep, 2026-08-29): FaceIds and UserId filters (own doc comments, api_op_ListFaces.go) were read by nothing at all -- listFacesReq had no such fields, so every call returned every face in the collection regardless of what was requested. UserId now resolved against the associating user's storedUser.FaceIDs (see AssociateFaces)."}
  SearchFaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "deterministic per-identity similarity (same ExternalImageId => 100.0), not canned — see faceSimilarity"}
  SearchFacesByImage: {wire: ok, errors: ok, state: ok, persist: ok, note: "similarity varies per imageKey (S3 path or byte length) via FNV-1a seed, not canned. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired. 2026-09-06 (gopherstack-qlqz): QualityFilter now parsed and enum-validated (was declared on no field at all -- see Notes #7) -- see Notes #9."}
  CreateUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: duplicate UserId now returns ConflictException (was ResourceAlreadyExistsException) — see Notes #2"}
  DeleteUser: {wire: ok, errors: ok, state: ok, persist: ok}
  ListUsers: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateFaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "real FaceId membership check against the collection; unknown faces reported in UnsuccessfulFaceAssociations with FACE_NOT_FOUND"}
  DisassociateFaces: {wire: ok, errors: ok, state: ok, persist: ok}
  SearchUsers: {wire: ok, errors: fixed, state: ok, persist: ok, note: "gopherstack-2wvq (2026-08-21): handler unconditionally required UserId, but SearchUsersInput marks only CollectionId required (rekognition@v1.54.4 api_op_SearchUsers.go) -- 'The request must be provided with either FaceId or UserId... If a FaceId is provided, UserId isn't required to be present in the Collection.' Added SearchUsersByFace, reusing the existing facesByCollection index SearchFaces already uses (faces.go) rather than a new one, so a FaceId-only request now resolves (and errors ResourceNotFoundException if the face itself doesn't exist); UserId-absent-and-FaceId-absent still rejects. Response now emits SearchedFace (not SearchedUser) when searched by FaceId, matching the real SearchUsersOutput having both as distinct optional members."}
  SearchUsersByImage: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  CreateStreamProcessor: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (2026-07-23): Input/Output/Settings/RegionsOfInterest/NotificationChannel/KmsKeyId/DataSharingPreference are now parsed from the request and stored (see Notes #5). Also FIXED prior sweep: duplicate Name now returns ResourceInUseException (was ResourceAlreadyExistsException) — see Notes #2"}
  DeleteStreamProcessor: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeStreamProcessor: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (2026-07-23): now returns Input/Output/Settings/RegionsOfInterest/NotificationChannel/KmsKeyId/DataSharingPreference/LastUpdateTimestamp/StatusMessage, all routed through epochSeconds() for the two timestamp fields — see Notes #5"}
  ListStreamProcessors: {wire: ok, errors: ok, state: ok, persist: ok}
  StartStreamProcessor: {wire: ok, errors: ok, state: ok, persist: ok}
  StopStreamProcessor: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateStreamProcessor: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (2026-07-23): DataSharingPreferenceForUpdate/ParametersToDelete/RegionsOfInterestForUpdate/SettingsForUpdate.ConnectedHomeForUpdate now actually mutate the stored stream processor (was a pure existence-check no-op) — see Notes #5"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: resourceExists() now also recognizes ProjectVersion ARNs (the 'Custom Labels model' AWS's TagResource doc says is taggable, alongside collections/stream processors) — was previously always ResourceNotFoundException for a real, existing ProjectVersion — see Notes #3"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateProject: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: duplicate name now returns ResourceInUseException (was ResourceAlreadyExistsException) — see Notes #2"}
  DeleteProject: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeProjects: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: CreationTimestamp was an ISO8601 string ('2006-01-02T15:04:05.000Z' Format()) — real awsjson1.1 wire shape is an epoch-seconds JSON number; SDK deserializer errors with 'expected DateTime to be a JSON Number, got string instead'. Now epochSeconds() — see Notes #1. FIXED (gopherstack wrapper-key sweep, 2026-08-29): Features filter (api_op_DescribeProjects.go: 'Specifies the type of customization to filter projects by. If no value is specified, CUSTOM_LABELS is used as a default.') was never plumbed through the call chain -- describeProjectsReq had no such field. Worse than a missing filter: the documented default silently changed real behavior too, since an absent Features now excludes CONTENT_MODERATION projects (previously every DescribeProjects call, filtered or not, returned every project regardless of feature)."}
  CreateProjectVersion: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (2026-08-10, Notes #6): OutputConfig is now enforced as required (was silently optional -- more permissive than the real validator); FeatureConfig.ContentModeration.ConfidenceThreshold now parsed/stored/echoed (shallow, 2 levels, no unions); TrainingData/TestingData now cross-validated (both-or-neither) though their contents stay opaque -- see gaps. Prior sweep (2026-07-23): Tags/OutputConfig/KmsKeyId/VersionDescription parsed, stored, echoed — see Notes #5. Duplicate (ProjectArn,VersionName) returns ResourceInUseException — see Notes #2"}
  DeleteProjectVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeProjectVersions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (Notes #6): now also echoes FeatureConfig/MaxInferenceUnits/MinInferenceUnits/SourceProjectVersionArn (previously stored by Start/CopyProjectVersion but never serialized here). Prior sweep: CreationTimestamp string->epoch-seconds — see Notes #1"}
  CopyProjectVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (Notes #6): now stores SourceProjectVersionArn on the destination version (echoed by DescribeProjectVersions)"}
  StartProjectVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (Notes #6): now accepts and stores the optional MaxInferenceUnits (StartProjectVersionInput member; was parsed nowhere, so MinInferenceUnits was the only value ever recorded)"}
  StopProjectVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  ListProjectPolicies: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: CreationTimestamp + LastUpdatedTimestamp string->epoch-seconds — see Notes #1. FIXED 2026-08-31 (gopherstack-uox6): MaxResults omission default was 100 (this service's general default/cap), but this op's own doc comment states 'The largest value you can specify is 5 ... The default value is 5' — the only List/Describe op in this service with a 5-item default instead of 100. See Notes #7."}
  PutProjectPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteProjectPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDataset: {wire: partial, errors: ok, state: ok, persist: ok, note: "FIXED this sweep (2026-07-23): now rejects a duplicate (ProjectArn,DatasetType) pair with ResourceAlreadyExistsException (via an explicit b.datasets.Range scan, since datasetARN is still always uuid-suffixed so the table key itself never collides) — see Notes #5. GAP found 2026-09-06 (gopherstack-eshx): createDatasetReq never parses DatasetSource/DatasetSource.GroundTruthManifest.S3Object at all (CreateDataset's only optional Image-shaped input, used for a MANUAL-type dataset's seed manifest) -- CreateDataset therefore is not given the InvalidS3ObjectException check this pass added elsewhere (see gaps)."}
  DeleteDataset: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDataset: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: CreationTimestamp + LastUpdatedTimestamp string->epoch-seconds — see Notes #1"}
  ListDatasetEntries: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack wrapper-key sweep, 2026-08-29): ContainsLabels/Labeled/SourceRefContains/HasErrors (all four own doc comments, api_op_ListDatasetEntries.go) were read by nothing at all -- listDatasetEntriesReq had none of these fields. ContainsLabels/Labeled/SourceRefContains now parse the stored JSON-lines manifest entries (source-ref, *-metadata blocks) via entryLabels/entrySourceRef. HasErrors is honoured structurally, not fabricated: this backend has no entry-level error concept (see computeDatasetStats' ErrorEntries note), so HasErrors=true now correctly returns an empty result rather than inventing error entries."}
  ListDatasetLabels: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDatasetEntries: {wire: ok, errors: ok, state: ok, persist: ok}
  DistributeDatasetEntries: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateFaceLivenessSession: {wire: ok, errors: ok, state: ok, persist: ok}
  GetFaceLivenessSessionResults: {wire: ok, errors: ok, state: ok, persist: ok}
  StartMediaAnalysisJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced on Input.S3Object when S3 is wired -- see Notes."}
  GetMediaAnalysisJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: CreationTimestamp string->epoch-seconds — see Notes #1"}
  ListMediaAnalysisJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this sweep: CreationTimestamp string->epoch-seconds — see Notes #1"}
families:
  detect_and_recognize: {status: ok, note: "CompareFaces/DetectFaces/DetectLabels/DetectText/DetectCustomLabels/DetectModerationLabels/DetectProtectiveEquipment/RecognizeCelebrities/GetCelebrityInfo — inherently-ML ops, correctly deterministic mocks per parity-principles.md rule 4 (not flagged as bugs); DetectLabels' plausibleLabels() genuinely varies with MinConfidence/MaxLabels, CompareFaces/DetectFaces/RecognizeCelebrities always return an empty/fixed-shape result regardless of input — acceptable, these are stateless single-shot image ops with no backing resource to fake statefulness against. FIXED 2026-08-31 (gopherstack-uox6): DetectLabels' omitted-MinConfidence default was 50.0 but the op's own doc comment states 'The default is 55%.' — had zero observable effect against the current 7-entry synthetic label set (lowest confidence 55.4, above both values) but is now correct at the source (resolveMinConfidence, handler_labels.go) for any future addition to that set. See Notes #7. FIXED 2026-09-06 (gopherstack-eshx): CompareFaces (SourceImage+TargetImage)/DetectFaces/DetectLabels/DetectText/DetectCustomLabels/DetectModerationLabels/DetectProtectiveEquipment/RecognizeCelebrities all declare InvalidS3ObjectException and none checked Image.S3Object against real S3 state at all -- now enforced when S3 is wired, see Notes. AUDITED 2026-09-06 (gopherstack-qlqz): MinConfidence/QualityFilter/Attributes enum validation swept across all eight stateless detection ops. FIXED: DetectFaces.Attributes (was []string, unvalidated against types.Attribute) and CompareFaces/SearchFacesByImage.QualityFilter (declared on no field at all -- see Notes #7); both now enum-validated against the real SDK enum via types.Attribute(\"\").Values()/types.QualityFilter(\"\").Values(). MinConfidence left unvalidated by design, confirmed correct -- see Notes #9. FIXED 2026-09-06 (gopherstack-duj0): DetectProtectiveEquipment.SummarizationAttributes .MinConfidence/.RequiredEquipmentTypes are both required-when-SummarizationAttributes-set (validators.go, ProtectiveEquipmentSummarizationAttributes) but were float32/[]string with no way to distinguish omitted from zero/nil -- retyped to *float32/*[]string and enforced. See Notes #10; corrects the MinConfidence required-ness verdict in Notes #9."
  async_video_jobs: {status: ok, note: "Start*/Get* (CelebrityRecognition, ContentModeration, FaceDetection, FaceSearch, LabelDetection, PersonTracking, SegmentDetection, TextDetection) — real StartAsyncJob/GetAsyncJob state machine (IN_PROGRESS -> SUCCEEDED on 2nd poll, PollCount persisted). FIXED this sweep (Notes #6): JobTag and Video (S3 reference) were parsed from every Start* request and then discarded -- both are real GetXxxOutput members, now stored and echoed back. GetSegmentDetection.SelectedSegmentTypes now echoes the Type values from StartSegmentDetection's SegmentTypes (ModelVersion omitted, no legitimate source). GetLabelDetection/GetContentModeration now return GetRequestMetadata (SortBy/AggregateBy echo). Detection-result arrays (Celebrities/ModerationLabels/Faces/Labels/Persons/Segments/TextDetections) remain synthesized-empty — acceptable mock, ML-inherent-op exemption, see gaps/deferred. FIXED 2026-09-06 (gopherstack-eshx): every Start* op declares InvalidS3ObjectException and none checked Video.S3Object against real S3 state at all -- now enforced when S3 is wired, see Notes."}
routing: {status: ok, note: "single X-Amz-Target: RekognitionService.<Op> POST endpoint (awsjson1.1), verified every op in the dispatch map (buildOps + appendixAOps) against a real op name in aws-sdk-go-v2/service/rekognition; no name mismatches found"}
gaps:
  - CreateProjectVersion still drops TrainingData/TestingData contents (Custom Labels external-manifest structures: TrainingData/TestingData -> []Asset -> GroundTruthManifest -> S3Object, 3-4 levels, no unions, structurally simple but pointless to store -- the only place they'd resurface is TrainingDataResult/TestingDataResult, which requires a training-completion lifecycle this backend never reaches; both-or-neither presence is still cross-validated) — see Notes #6
  - "2026-09-06 (gopherstack-eshx): IndexFaces never parses IndexFacesInput.Image at all (indexFacesReq has CollectionId/ExternalImageId only) -- a required member of a real IndexFaces request is silently dropped, not just unchecked against S3. Structural gap, out of this pass's scope (adding S3Object existence checking, not adding a missing wire field); IndexFaces is therefore excluded from this pass's InvalidS3ObjectException enforcement. Needs its own fix."
  - "2026-09-06 (gopherstack-eshx): CreateDataset never parses CreateDatasetInput.DatasetSource (createDatasetReq has ProjectArn/DatasetType only) -- DatasetSource.GroundTruthManifest.S3Object, the one Image-shaped field this op accepts, is silently dropped. Same structural-gap reasoning as IndexFaces above; excluded from this pass's InvalidS3ObjectException enforcement."
deferred:
  - ProjectVersionDescription's BaseModelVersion (needs data this emulator cannot have: an AWS-internal base-model-catalog string, not derivable or user-supplied) and BillableTrainingTimeInSeconds/TrainingEndTimestamp/EvaluationResult/ManifestSummary/TestingDataResult/TrainingDataResult (needs a lifecycle that does not exist: all are documented as populated only once training completes, and this backend's Status never advances past TRAINING_IN_PROGRESS; EvaluationResult additionally requires a fabricated F1 score, which the no-fabrication rule forbids outright) — see Notes #6
  - ProjectVersionDescription.Feature / DescribeProjects' Feature (large mechanical surface deferred for size: Feature is set at CreateProject time, which does not currently accept or store it at all; modeling ProjectVersionDescription.Feature honestly requires a CreateProject signature change cascading through DescribeProjects too, a separate op family from this sweep's CreateProjectVersion/StartProjectVersion/CopyProjectVersion scope) — see Notes #6
  - SegmentTypeInfo.ModelVersion (needs data this emulator cannot have: AWS-internal segment-detection model build string) — Type is modeled, ModelVersion is not, see Notes #6
  - Detection-result arrays (Celebrities/ModerationLabels/Faces/Labels/Persons/Segments/TextDetections) stay synthesized-empty; acceptable per the ML-mock exemption, not individually wire-diffed field-by-field this sweep (this sweep's scope was CreateProjectVersion/ProjectVersionDescription/async-video envelope fields, not the ML detection payloads themselves)
leaks: {status: clean, note: "no goroutines/janitors in this service; lockmetrics.RWMutex coarse lock verified around every backend mutation; Snapshot/Restore delegation (Handler->Backend) verified wired (persistence.go)"}
---

## Notes

1. **Timestamp wire shape (the main bug class this sweep).** awsjson1.1 (which
   Rekognition uses) always serializes `time.Time` fields as epoch-seconds JSON
   *numbers*, never ISO8601 strings — confirmed by reading
   `aws-sdk-go-v2/service/rekognition@v1.54.4/deserializers.go`'s
   `case "CreationTimestamp": ... case json.Number: ... default: return
   fmt.Errorf("expected DateTime to be a JSON Number, got %T instead", value)`.
   8 fields across 5 response types in `handler_appendixa.go` were rendering
   `time.Time.Format("2006-01-02T15:04:05.000Z")` (a string) instead: those
   responses would fail to decode in a real SDK client with "expected DateTime
   to be a JSON Number, got string instead". Fixed by switching every
   affected field from `string` to `float64` and rendering via the existing
   `epochSeconds()` helper (already used correctly by `handleDescribeCollection`
   in `handler.go`): `projectDescription.CreationTimestamp`,
   `projectVersionDescription.CreationTimestamp`,
   `projectPolicyEntry.{CreationTimestamp,LastUpdatedTimestamp}`,
   `datasetDescription.{CreationTimestamp,LastUpdatedTimestamp}`,
   `mediaAnalysisJobDescription.CreationTimestamp` /
   `getMediaAnalysisJobResp.CreationTimestamp`. `DescribeStreamProcessor`'s
   `CreationTimestamp` was already a number (`float64(t.Unix())`) — correct,
   just not routed through `epochSeconds()`; left alone (not a bug, no
   fractional-second loss matters here since `time.Now()` sub-second precision
   isn't asserted anywhere).

2. **"Already exists" exception type varies per operation — do not generically
   dispatch on the AlreadyExists sentinel.** `aws-sdk-go-v2/service/rekognition`'s
   generated `deserializers.go` gives each `Create*` op its OWN switch of
   recognized exception-type strings; an exception type not in that op's
   switch deserializes as an untyped `smithy.GenericAPIError` instead of the
   typed exception, breaking SDK-side `errors.As(&typedErr)` matching. Verified
   per-op via each op's `awsAwsjson11_deserializeOpError<Op>` switch:
   - `CreateCollection`, `CreateDataset` → `ResourceAlreadyExistsException`
   - `CreateStreamProcessor`, `CreateProject`, `CreateProjectVersion` →
     `ResourceInUseException`
   - `CreateUser` → `ConflictException`
   Before this sweep, `handleError` (`handler.go`) had a single generic
   `errors.Is(err, awserr.ErrAlreadyExists)` case hardcoded to
   `ResourceAlreadyExistsException`, so only `CreateCollection`/`CreateDataset`
   were actually correct — the other 3 create ops silently emitted the wrong
   `__type`. Fixed by introducing two new local sentinels in `backend.go`
   (`ErrNameInUse` → `ResourceInUseException`, `ErrUserConflict` →
   `ConflictException`), routing `CreateStreamProcessor`/`CreateProject`/
   `CreateProjectVersion`/`CreateUser`'s duplicate-checks through them, and
   adding matching cases to `handleError` ahead of the generic
   `ErrAlreadyExists` case. `handler_audit1_test.go`'s
   "CreateStreamProcessor duplicate returns error" test asserted the old
   (wrong) `ResourceAlreadyExistsException` value — updated to
   `ResourceInUseException`.

3. **`resourceExists()` (backend.go, gates TagResource/UntagResource/
   ListTagsForResource) only recognized collection and stream-processor
   ARNs.** Per `aws-sdk-go-v2/service/rekognition@v1.54.4/api_op_TagResource.go`'s
   doc comment, tagging also applies to "an Amazon Rekognition ... Custom
   Labels model" — i.e. a ProjectVersion ARN. Before this sweep, tagging a
   real, just-created ProjectVersion always failed with
   `ResourceNotFoundException`. Fixed by adding a `b.projectVersions.All()`
   scan to `resourceExists()`. (Project ARNs themselves are deliberately NOT
   included — AWS's TagResource doc explicitly scopes to
   collection/stream-processor/model, not project.)

4. **False leads ruled out** (documented so the next audit doesn't re-flag):
   - `CompareFaces`/`DetectFaces`/`RecognizeCelebrities` always return an
     empty/fixed result regardless of input image — this is the accepted
     ML-mock exemption (parity-principles.md rule 4), not a disguised no-op:
     there's no backing *resource* (collection/face/user) being silently
     dropped, just a stateless single-shot detection call with no real vision
     model behind it.
   - `IndexFaces`/`SearchFaces`/`SearchFacesByImage`/`SearchUsers`/
     `SearchUsersByImage` confidence/similarity scores are deterministic
     hashes of stored identity (FaceID/ExternalImageId/UserID), not canned
     constants — genuinely varies per input and is stable across repeated
     calls, matching how a real client test would expect determinism.
   - `AsyncJob`/`MediaAnalysisJob` Start/Get lifecycle (`PollCount`-driven
     IN_PROGRESS → SUCCEEDED state transition) is real, persisted state, not a
     stub — verified `GetAsyncJob` mutates and returns based on
     `storedAsyncJob.PollCount`.

5. **2026-07-23 sweep: closed every remaining `gaps:` item from the prior
   audit except the CreateProjectVersion TrainingData/TestingData/
   FeatureConfig one (kept as a gap, see above — deliberately deferred, not
   an oversight).**
   - **Stream processor config fields.** `CreateStreamProcessor` previously
     accepted `Input`/`Output`/`Settings`/`RegionsOfInterest`/
     `NotificationChannel`/`KmsKeyId`/`DataSharingPreference` but discarded
     them; `DescribeStreamProcessor` always returned them absent. Added
     `StreamProcessorInput`/`StreamProcessorOutput`/`StreamProcessorSettings`/
     `RegionOfInterest`/`BoundingBox`/`Point`/
     `StreamProcessorNotificationChannel`/`StreamProcessorDataSharingPreference`
     domain types (`interfaces.go`) mirroring the real SDK's nested
     `types.*` shapes field-for-field (verified against
     `aws-sdk-go-v2/service/rekognition@v1.54.4/types/types.go` and the
     `awsAwsjson11_serialize/deserializeDocument*` functions for exact JSON
     key names/nesting), threaded through a `CreateStreamProcessorParams`
     struct (avoids an unbounded positional-parameter CreateStreamProcessor
     signature), stored on `storedStreamProcessor`, and echoed back by
     `DescribeStreamProcessor` (`handler_stream_processors.go`'s
     `*Wire` request/response types + `*FromDomain`/`.toDomain()`
     converters). Optional pointer wire fields use `omitempty` so an unset
     field is *absent* from the JSON (matching the real serializer's
     `if v.X != nil { ... }` guards), not present-as-`null`.
   - **`UpdateStreamProcessor` was a pure existence-check no-op.** Now
     applies `DataSharingPreferenceForUpdate`,
     `SettingsForUpdate.ConnectedHomeForUpdate.{Labels,MinConfidence}`, and
     `RegionsOfInterestForUpdate` (wholesale replace, not merge), with
     `ParametersToDelete` (`RegionsOfInterest` / `ConnectedHomeMinConfidence`)
     applied last so a delete always wins over a same-request set — matches
     AWS's documented apply-then-delete order. Presence/absence of each
     update field is signaled the same way the AWS wire shape does: Go's
     `encoding/json` leaves an absent key's pointer/slice field `nil` and a
     present-but-empty JSON array as a non-nil empty slice, so no extra
     `*Set bool` sidecar fields were needed.
   - **`CreateDataset` never rejected a duplicate `(ProjectArn,DatasetType)`
     pair.** `datasetARN` is still always uuid-suffixed (so the table key
     itself never collides — left as-is, this is how dataset identity is
     modeled here), so the check is now explicit: a `b.datasets.Range` scan
     for an existing dataset with the same `(ProjectARN, DatasetType)`
     before insert, returning the new `ErrDatasetAlreadyExists` sentinel
     (→ `ResourceAlreadyExistsException`, verified against
     `CreateDataset`'s own error-deserializer switch — same exception type
     as `CreateCollection`, not `ResourceInUseException`).
   - **`CreateProjectVersion` dropped `Tags`/`OutputConfig`/`KmsKeyId`/
     `VersionDescription`.** These four are now parsed, stored on
     `storedProjectVersion`, and (for `OutputConfig`/`KmsKeyId`/
     `VersionDescription`) echoed back by `DescribeProjectVersions`; initial
     `Tags` are applied to the ProjectVersion ARN's tag-store entry the same
     way `CreateStreamProcessor` applies its initial tags (ProjectVersion
     ARNs are already confirmed taggable — see Notes #3). `TrainingData`/
     `TestingData`/`FeatureConfig` remain a deliberate gap (see `gaps:`):
     each describes a nested Custom Labels training-manifest structure
     (`GroundTruthManifest`/`Asset`/feature-variant unions) with no backing
     resource this in-memory backend can meaningfully simulate, and are
     lower-traffic than the four fields fixed this sweep.
   - Added `fieldalignment`-optimal struct field ordering (via
     `fieldalignment -fix`) to every struct touched this sweep
     (`storedStreamProcessor`, `StreamProcessor`, `StreamProcessorSettings`,
     `CreateStreamProcessorParams`) to keep `golangci-lint`'s `govet`
     fieldalignment check at 0 issues; field order in those structs carries
     no semantic meaning beyond that.

6. **2026-08-10 field-completeness follow-up (gopherstack-3tzd).** SDK pin
   was stale (`v1.51.26` in this file vs `v1.54.4` pinned in `go.mod`) —
   corrected here and in every inline citation above. Depth measurement of
   the recorded gaps, read directly from
   `aws-sdk-go-v2/service/rekognition@v1.54.4/types/types.go`:
   - `TrainingData`/`TestingData` -> `[]Asset` -> `Asset.GroundTruthManifest`
     -> `GroundTruthManifest.S3Object` -> `S3Object{Bucket,Name,Version}`: 4
     struct levels, no unions, every level 1-3 fields. Structurally shallow,
     but each level's only content is an S3 pointer to a manifest this
     backend never trains against — there is no training-completion
     lifecycle here for `TrainingDataResult`/`TestingDataResult` (the only
     place a stored copy would resurface) to ever populate. Left opaque; only
     the documented "both or neither" cross-field requirement
     (`api_op_CreateProjectVersion.go` doc comment) is enforced, since that's
     cheaply checkable without modeling the contents — this closes a
     more-permissive-than-real gap (gopherstack previously accepted either
     field alone).
   - `FeatureConfig` -> `CustomizationFeatureConfig.ContentModeration` ->
     `CustomizationFeatureContentModerationConfig.ConfidenceThreshold`: 2
     levels, single member each, no unions — the prior audit's "genuinely
     complex... feature-variant unions" characterization (see the Notes #5
     entry above) was wrong; verified by reading `types.go:486,495` directly.
     Modeled and echoed verbatim (no fabrication: it's the client's own
     training-job config, not an inference result).
   - **`CreateProjectVersion` was more permissive than the real service**:
     `OutputConfig` is a required `CreateProjectVersionInput` member
     (`validateOpCreateProjectVersionInput`, `validators.go:2107`) but
     gopherstack never checked for it. Fixed with a failing-first test
     (`TestProjectVersions/CreateProjectVersion_missing_OutputConfig_returns_error`);
     three existing tests asserted the old (wrong) permissive behavior by
     omitting `OutputConfig` and expecting 200 — fixed to send it.
   - **`StartProjectVersion`/`CopyProjectVersion`/`DescribeProjectVersions`
     dropped fields the backend already had, or could trivially have,
     but never serialized**: `MaxInferenceUnits` (a real
     `StartProjectVersionInput` member, parsed nowhere before this sweep);
     `SourceProjectVersionArn` (never stored by `CopyProjectVersion`); and
     `MinInferenceUnits` itself (stored since a prior sweep via
     `StartProjectVersion`, but never echoed by `DescribeProjectVersions` —
     a pure serialization gap). All three fixed.
   - **Async-video `Get*` responses: `JobTag` and `Video` are real
     `GetXxxOutput` members** (verified against every `api_op_GetXxx.go` in
     this family) that every `Start*` handler already parsed into its
     request struct and then discarded (`_ *startXxxReq`). Now threaded
     through `StartAsyncJobParams` -> `storedAsyncJob` -> the shared
     `getJobBase` helper, so all seven `Get*` responses
     (LabelDetection/ContentModeration/CelebrityRecognition/FaceDetection/
     FaceSearch/TextDetection/PersonTracking) echo them. `getJobBase`'s
     signature grew a second return value (the raw `*AsyncJob`) so
     `GetSegmentDetection` doesn't have to call `GetAsyncJob` a second time
     to read `SegmentTypes` — doing so would have double-advanced the
     `PollCount` IN_PROGRESS->SUCCEEDED state machine per client-visible
     call, a real bug caught before it shipped.
   - **`GetSegmentDetection.SelectedSegmentTypes`** now echoes the `Type` of
     each `SegmentTypes` entry from the matching `StartSegmentDetection`
     call (previously always `[]`, despite the value being sitting right
     there in the discarded request). `ModelVersion` is deliberately left
     off `segmentTypeInfoWire` — it names the internal Rekognition model
     build and there is no legitimate source for that string.
   - **`GetLabelDetection`/`GetContentModeration.GetRequestMetadata`** now
     echo the current call's `SortBy`/`AggregateBy`, applying the documented
     default when omitted (`LabelDetectionSortBy`/`ContentModerationSortBy`
     default to `TIMESTAMP`, `ContentModerationAggregateBy` defaults to
     `TIMESTAMPS` — all three defaults are stated in their respective
     `api_op_GetXxx.go` doc comments). `LabelDetectionAggregateBy` has no
     documented default, so it is only reported when the caller supplies
     one — inventing a default here would be the same class of mistake as a
     fabricated confidence score.
   - **Deliberately not modeled, with reasons** (see `deferred:`):
     `BaseModelVersion` (needs data this emulator cannot have — AWS-internal
     base-model catalog); `BillableTrainingTimeInSeconds`,
     `TrainingEndTimestamp`, `EvaluationResult`, `ManifestSummary`,
     `TestingDataResult`, `TrainingDataResult` (needs a lifecycle that does
     not exist — this backend's `Status` never advances past
     `TRAINING_IN_PROGRESS`, and `EvaluationResult.F1Score` additionally
     can't be computed without a real model); `Feature` on
     `ProjectVersionDescription`/`DescribeProjects` (large mechanical
     surface deferred for size — `CreateProject` doesn't accept or store
     `Feature` at all yet, so modeling it honestly is a separate
     `CreateProject`+`DescribeProjects` change, not a `CreateProjectVersion`
     one); `SegmentTypeInfo.ModelVersion` (needs data this emulator cannot
     have, see above).
   - All new struct fields use `omitempty` and are additive — no
     `rekognitionSnapshotVersion` bump; round-trip verified by
     `TestSnapshotRestore_ProjectVersionAndAsyncJobNewFields`
     (`persistence_test.go`).

**2026-08-30 (gopherstack request-field re-scan, `cmd/reqfieldscan`)**:
`cmd/reqfieldscan` (added `aa4ec0ad2`) against this service's request fields.
Coverage: 75/75 dispatch-table ops (100%) resolved via `service.WrapOp`, no
unresolved ops, no `wrapAccuracy`-style local-wrapper blind spot (unlike
cognitoidp). 35 fields originally flagged; 3 fixed this pass (see below), 32
remain, all hand-verified and sorted below.

**Real bug, fixed: `CompareFaces` discarded its entire request** (`_
*compareFacesReq` -- `SourceImage`/`TargetImage`/`SimilarityThreshold` all
three unreadable) and always returned the same hardcoded match regardless of
`SimilarityThreshold`, so a client asking for a 99.99% threshold got the
identical fabricated "match" as a client asking for 1%
(api_op_CompareFaces.go's documented default is 80%). Fixed to bind `req`,
default an unset/zero threshold to 80 per that doc, and gate a synthetic
match on `SimilarityThreshold` using a deterministic similarity derived from
whether `SourceImage`/`TargetImage` are the same reference (`imageRefKey`,
this file's existing convention for stateless-mock similarity, already used
by `SearchFacesByImage`) -- identical images score 100, distinct ones a
lower plausible score, so the threshold has an observable, testable effect.
Proof: `TestCompareFaces_SimilarityThreshold` (`wire_field_fixes_test.go`),
real typed SDK client, confirmed failing (returned a match at a 99.99
threshold against two distinct images) against the unfixed code.

**Real bugs found, hand-verified, explicitly NOT fixed this pass (shape:
"a handler discarding its whole request body") -- four more handlers share
`CompareFaces`'s pre-fix shape, declared with a blank `_ *reqType`
parameter, structurally unable to read anything:**

- **`DetectFaces`** (`Image`, `Attributes` both unreadable). No sibling
  precedent to safely generalize from: `Attributes` selects which optional
  facial-attribute sub-objects (age range, emotions, landmarks, ...) appear
  in the response, and `faceDetailEntry` has no fields for any of them --
  wiring it in without inventing new response shape isn't a narrow fix.
- **`DetectCustomLabels`** (`ProjectVersionArn`, `Image`, `MaxResults`,
  `MinConfidence` all unreadable). Distinct from the others: this service
  *does* track custom-labels project versions as real state
  (`project_versions.go`'s `InMemoryBackend.projectVersions`, with a real
  `RUNNING`/`TRAINING_IN_PROGRESS`/`STOPPED` status lifecycle via
  `StartProjectVersion`/`StopProjectVersion`), so this is also a **missing
  existence check**: real AWS requires the named `ProjectVersionArn` to
  exist and be `RUNNING` (`ResourceNotReadyException` otherwise), and
  gopherstack currently accepts any string, running or not, without
  looking it up. `projectVersions` is only reachable through the unexported
  `InMemoryBackend` field, not the `StorageBackend` interface `Handler`
  holds, so a correct fix needs a new interface method -- a layer-boundary
  change, reported rather than made. Fabricating specific custom-label
  *names* for a nonexistent customer-trained model would additionally risk
  inventing capability that isn't real (the class of bug this campaign
  explicitly flags as "fix deletes rather than adds"), so even the
  existence-check-only version of this fix was left for a dedicated pass.
- **`DetectProtectiveEquipment`** (`SummarizationAttributes`, `Image` both
  unreadable). Same "no safe sibling pattern" reasoning as `DetectFaces`.
- **`RecognizeCelebrities`** (`Image` unreadable, its only field). No
  celebrity database or confidence-threshold analog exists to gate a
  synthetic result on, unlike `CompareFaces`'s `SimilarityThreshold`.

**Verified, not bugs -- established sibling convention, not a gap:**

- **`DetectLabels.Image` / `DetectModerationLabels.Image`.** Both handlers
  *do* bind `req` and use its non-image fields (`MinConfidence`/`MaxLabels`
  for `DetectLabels`'s `plausibleLabels`; `MinConfidence` gating
  `DetectModerationLabels`'s "clean by default, `Suggestive` only below a
  low explicit threshold" synthetic result) -- this service's established,
  disclosed pattern ("stateless mock results", per `handler_faces.go`'s own
  section comment) for ops with no real CV backing is to shape a synthetic
  response from confidence/count parameters without decoding the image
  itself. `Image` being unread here is consistent with that established
  convention, not a stub.

**Verified, structural (whole capability class not implemented anywhere in
this service), not fixed:**

- **`ClientRequestToken`** on all ten `Start*`/`CreateFaceLivenessSession`
  ops (`CreateFaceLivenessSession`, `StartCelebrityRecognition`,
  `StartContentModeration`, `StartFaceDetection`, `StartFaceSearch`,
  `StartLabelDetection`, `StartMediaAnalysisJob`, `StartPersonTracking`,
  `StartSegmentDetection`, `StartTextDetection`). No idempotency-token dedup
  pattern exists anywhere in this service (or, per the same pass's ecs
  finding, in ecs either) -- a systemic gap, not an isolated one.
- **`DetectText.Filters`.** Declared as `*struct{}` -- a Go empty-struct
  type with zero members, so the *sub-fields* real AWS's `DetectTextFilters`
  actually carries (`WordFilter.MinConfidence`, region-of-interest boxes)
  were never modeled in the first place; there is nothing for a field read
  to reach.
- **`getJobReq.NextToken`/`.MaxResults`** (shared by `GetCelebrityRecognition`
  /`GetFaceDetection`/`GetFaceSearch`/`GetPersonTracking`/
  `GetSegmentDetection`/`GetTextDetection`), **`getContentModerationReq
  .NextToken`/`.MaxResults`**, **`getLabelDetectionReq.NextToken`/
  `.MaxResults`.** Every one of these six ops' result-list field is typed
  as `[]struct{}` (`Faces`, `Persons`, `ModerationLabels`, `Labels`, etc.) --
  literally incapable of carrying data regardless of how the handler is
  written, since this backend does no real video analysis. Pagination
  parameters are moot when the collection being paginated can never hold
  anything; distinguishing this from a silently-broken listing per this
  campaign's own guidance, this is an honestly-empty design, not a bug.
- **`startContentModerationReq.MinConfidence`, `startFaceDetectionReq
  .FaceAttributes`, `startFaceSearchReq.FaceMatchThreshold`,
  `startLabelDetectionReq.MinConfidence`.** These exist to shape their
  matching `Get*` op's results -- moot for the same reason as the
  pagination fields above: the `Get*` responses they would shape are
  structurally always empty.

Gates: `go build ./services/rekognition/...`, `go build ./...` (repo-wide,
clean), `go vet ./services/rekognition/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/rekognition/...` (pass),
`golangci-lint run ./services/rekognition/...` (0 issues). Work left
uncommitted per this pass's instructions.

7. **2026-08-31 (gopherstack-uox6, value-semantics sweep): two wrong-default
   bugs, both a right-shaped value at the wrong number.** Swept the pinned
   SDK (`aws-sdk-go-v2/service/rekognition@v1.54.4`) for omission-default
   language ("If you do not specify ... the operation defaults to").
   - `DetectLabels.MinConfidence`: doc "The default is 55%." Code
     (`handler_labels.go`) applied 50.0 when omitted. Extracted into
     `resolveMinConfidence` and exposed via `export_test.go` for a direct
     regression test (`TestDetectLabels_MinConfidenceDefault`, confirmed
     failing at 50 against unmodified code); no request-behavior test could
     observe the difference because every synthetic label in
     `plausibleLabels`'s 7-entry set sits at or above 55.4, so both the wrong
     and correct default returned every label — fixed anyway since the value
     is objectively wrong per the doc, and now correct for any future label
     added in the 50–55 range.
   - `ListProjectPolicies.MaxResults`: doc "The largest value you can specify
     is 5 ... The default value is 5" — the only List/Describe op in this
     service with a default other than 100 (verified `ListDatasetLabels`,
     `ListMediaAnalysisJobs`, `DescribeProjects`, `DescribeProjectVersions`,
     `ListDatasetEntries` all correctly use 100, matching their own doc
     comments). `projects.go`'s `ListProjectPolicies` used `maxPerPage = 100`
     for both the default and the cap — a 20x-too-wide page. Fixed to 5.
     Regression test `TestListProjectPolicies_DefaultPageSize`
     (`omission_defaults_test.go`) creates 6 policies and asserts a 5-item
     page with a non-empty `NextToken`; failed against unmodified code with a
     6-item page and empty token.

   **Checked and confirmed correct, not fixed:** `ListDatasetEntries`'
   `ContainsLabels` (OR-across-values, matching "the response includes an
   entry only if one or more of the labels ... exist" — `matchesDatasetEntryFilter`,
   `datasets.go`), its `HasErrors`/`Labeled`/`SourceRefContains` filters, and
   its `MaxResults` default (100, matching doc).

   **Recorded as the other axis (never read), not fixed here:**
   `QualityFilter` on `IndexFaces`/`CompareFaces`/`SearchFacesByImage` is
   declared on no request struct in this backend at all — not a wrong
   algorithm, a field with no code path at all. (`CompareFaces`/
   `SearchFacesByImage` fixed 2026-09-06, gopherstack-qlqz — see Notes #9;
   `IndexFaces` out of scope there, it's not a stateless detection op.)

   No web pages fetched this pass; everything resolved from the pinned
   module cache.

8. **2026-09-06: InvalidS3ObjectException wiring (gopherstack-eshx).** A large set of ops
   declare `InvalidS3ObjectException` in their real `deserializeOpError<Op>` switch (verified
   via the digit-safe `awk`+`grep -oE '"[A-Za-z0-9]+"'` extraction against
   `rekognition@v1.54.4/deserializers.go`, not the earlier `[A-Za-z]+` pattern that silently
   drops S3-named codes — see gopherstack-jkpi): `CompareFaces`, `CreateDataset`,
   `DetectCustomLabels`, `DetectFaces`, `DetectLabels`, `DetectModerationLabels`,
   `DetectProtectiveEquipment`, `DetectText`, `IndexFaces`, `RecognizeCelebrities`,
   `SearchFacesByImage`, `SearchUsersByImage`, and every `Start<X>` async video/media-analysis
   job (`StartCelebrityRecognition`, `StartContentModeration`, `StartFaceDetection`,
   `StartFaceSearch`, `StartLabelDetection`, `StartMediaAnalysisJob`, `StartPersonTracking`,
   `StartSegmentDetection`, `StartTextDetection`). Doc comment
   (`types/errors.go:344`): "Amazon Rekognition is unable to access the S3 object specified in
   the request." None of these ops previously checked an Image/Video/Input S3Object against
   real S3 state at all.

   Enforced for 19 of the 21: every op above except `IndexFaces` and `CreateDataset`, both of
   which turned out to never parse the request's Image-shaped field at all (see `gaps`) — a
   pre-existing structural gap, not something this pass's S3-check could reach without
   fabricating a field this backend has never read.

   Followed the `services/cloudtrail` `S3Backend`/`SetS3Backend`/`wireXxxS3` precedent
   (gopherstack-g9b4): `S3Backend` interface (`interfaces.go`, `HeadObject` only) implemented
   directly by `s3.InMemoryBackend` (no adapter), `InMemoryBackend.SetS3Backend` setter
   (`store.go`), `Handler.checkS3Object`/`checkImageRef`/`checkVideoRef` (`handler.go`,
   sharing the existing `imageRef`/`videoRef` wire types and `imageRefKey`/`videoRefS3`
   helpers) calling it, wired from `cli.go`'s `wireRekognitionS3` in
   `wireStorageAndSecretsIntegrations`. Unwired (no `SetS3Backend` call) or Backend not
   `*InMemoryBackend`: no-op, matching this repo's unwired-hook-stays-permissive convention.
   `Image.Bytes` (inline bytes, the real alternative to `Image.S3Object` on the `Image` union)
   is never checked. Only existence is checked (`s3.InMemoryBackend.HeadObject`, which itself
   distinguishes a missing bucket from a missing key) — "unreadable" or "wrong format" would
   require decoding the object, which this mock does not and should not simulate.

   Regression tests: `services/rekognition/s3_object_test.go` —
   `TestImageOps_S3ObjectValidation` (table: the 7 single-Image sync ops),
   `TestCompareFaces_S3ObjectValidation` (both SourceImage and TargetImage),
   `TestSearchByImageOps_S3ObjectValidation` (SearchFacesByImage/SearchUsersByImage, against a
   real created collection), `TestStartVideoOps_S3ObjectValidation` (table: all 8 Start* video
   ops), `TestStartMediaAnalysisJob_S3ObjectValidation` (Input.S3Object) — each asserting a
   missing bucket/key is rejected (400, `InvalidS3ObjectException`) and an existing one
   succeeds (200, proving the check does not reject everything).
   `TestDetectLabels_UnwiredS3StaysPermissive` / `TestStartLabelDetection_UnwiredS3StaysPermissive`
   prove the unwired path stays permissive. `TestDetectLabels_InlineBytesUnaffected` proves
   inline `Bytes` is never checked. All fail against a neutered `checkS3Object` (HeadObject
   called but its result discarded) and pass against the fix.
   `cli_textract_rekognition_s3_wiring_test.go` (root package) drives the real
   `initializeServices` composition root end-to-end.

   Gates: `go build ./...`, `go test -race -count=1 ./services/rekognition/...` and `.` (root),
   `golangci-lint run ./ services/rekognition/... services/textract/...` — all clean.

9. **2026-09-06 (gopherstack-qlqz): MinConfidence/QualityFilter/Attributes enum
   validation on the stateless detection ops.** Filed title-only, empty
   description — an audit-coverage issue ("this area was never examined"),
   not a known bug. Scope: the eight stateless image-analysis ops (take an
   image, return detections, persist nothing) — `DetectFaces`,
   `DetectLabels`, `DetectModerationLabels`, `DetectText`,
   `DetectProtectiveEquipment`, `RecognizeCelebrities`, `CompareFaces`,
   `SearchFacesByImage`. (`DetectCustomLabels` also matches the shape but
   isn't in the issue's named list, so left alone; `IndexFaces` persists
   real face state, out of scope.)

   Which of the three parameters each op actually has (`rekognition@v1.54.4`
   `api_op_Detect*.go`/`api_op_CompareFaces.go`/`api_op_SearchFacesByImage.go`/
   `api_op_RecognizeCelebrities.go`):

   | Op | MinConfidence | QualityFilter | Attributes |
   |---|---|---|---|
   | DetectFaces | — | — | `[]types.Attribute` |
   | DetectLabels | `*float32` | — | — |
   | DetectModerationLabels | `*float32` | — | — |
   | DetectText | — | — | — |
   | DetectProtectiveEquipment | `*float32` (nested, `SummarizationAttributes.MinConfidence`, required if `SummarizationAttributes` set) | — | — |
   | RecognizeCelebrities | — | — | — (only field is `Image`) |
   | CompareFaces | — | `types.QualityFilter` | — |
   | SearchFacesByImage | — | `types.QualityFilter` | — |

   **MinConfidence has two distinct axes — range and required-ness. Audited
   both; the range verdict below is correct, the required-ness verdict
   originally recorded here was wrong and was corrected 2026-09-06
   (gopherstack-duj0) — see that entry for the fix. Left as a single
   narrative here so the two verdicts stay next to their shared evidence.**

   *Range (is an out-of-bounds value rejected): confirmed correct, not
   fixed.* `validators.go`'s `validateOpDetect{Labels,ModerationLabels,
   ProtectiveEquipment}Input` (and every other op's validator in this
   service) never range-checks a numeric field, for any op, anywhere in this
   SDK. Rekognition's modeled `types/errors.go` has no `ValidationException`
   type at all (unlike REST-JSON AWS services that auto-reject Smithy
   `@range` violations that way) — the only relevant declared error is
   `InvalidParameterException`, which carries no evidence tying it
   specifically to MinConfidence range violations. Decisive first-party
   evidence instead points the other way:
   `ProtectiveEquipmentSummarizationAttributes.MinConfidence`'s own doc
   comment (`types/types.go:2111`) states plainly: "If you specify a value
   that is less than 50%, the results are the same as specifying a value of
   50%" — i.e. AWS **clamps/filters**, it does not **reject** an in-range
   type but out-of-bounds value. This backend's existing behavior already
   matches that: `detectLabelsReq` (`handler_labels.go`) treats an
   omitted/non-positive value as the 55% default and otherwise uses the raw
   value as a filter threshold (no labels above it), never erroring;
   `detectModerationLabelsReq` and
   `detectProtectiveEquipmentReq.SummarizationAttributes.MinConfidence` are
   likewise threshold/gate values, never a rejection condition. No fix
   applied here — inventing an `InvalidParameterException` rejection for an
   out-of-range-but-present value would be adding a rejection AWS's own SDK
   doc says doesn't happen.

   *Required-ness (is a missing value rejected): audit originally missed
   this, corrected as gopherstack-duj0.* `validators.go`'s
   `validateProtectiveEquipmentSummarizationAttributes` (lines 1914-1925) is
   exactly the same evidence class this entry used to justify the
   QualityFilter/Attributes fixes below — a real generated client-side
   validator — and it checks `v.MinConfidence == nil` /
   `v.RequiredEquipmentTypes == nil`, both `smithy.NewErrParamRequired`. That
   makes both members required *whenever `SummarizationAttributes` itself is
   supplied* (the outer field stays optional). This backend's
   `detectProtectiveEquipmentReq.SummarizationAttributes` had a plain
   `float32`/`[]string` pair — omitted vs. explicit-zero/nil were
   indistinguishable, so nothing could ever be rejected. See the
   gopherstack-duj0 entry (Notes #10) for the fix.

   **QualityFilter: real gap, fixed for `CompareFaces`/
   `SearchFacesByImage`.** `types.QualityFilter.Values()`
   (`types/enums.go:761`) is a closed 5-member enum (`NONE`, `AUTO`, `LOW`,
   `MEDIUM`, `HIGH`); `InvalidParameterException` is declared in both ops'
   `deserializeOpError` switch (raw extraction below). This backend's
   `compareFacesReq`/`searchFacesByImageReq` (`handler_faces.go`) didn't
   declare a `QualityFilter` field *at all* before this fix — already
   recorded as a known-but-unfixed gap by the prior 2026-08-31 sweep (Notes
   #7: "a field with no code path at all"). Fixed: both structs now have
   `QualityFilter string`, validated via `isValidQualityFilter` (empty
   string, meaning omitted, also accepted — matching the documented `NONE`
   default) against `sdktypes.QualityFilter("").Values()` — derived from the
   live SDK enum, not hand-copied, following this repo's
   `isValidVaultEvent` (`services/backup/vault_policies.go`) precedent so
   the check can't drift from the real enum. An invalid value now returns
   `InvalidParameterException` via the existing `ErrValidation` sentinel
   (`errors.go`), which `handler.go`'s `errors.Is(err,
   awserr.ErrInvalidParameter)` branch already maps to that wire type. No
   filtering *behavior* was added for `QualityFilter` (this mock has no face
   "quality" signal to filter on) — only enum validation, matching the
   issue's scope.

   **Attributes: real gap, fixed for `DetectFaces`.** `types.Attribute
   .Values()` (`types/enums.go:29`) is a closed 14-member enum (`DEFAULT`,
   `ALL`, `AGE_RANGE`, `BEARD`, `EMOTIONS`, `EYE_DIRECTION`, `EYEGLASSES`,
   `EYES_OPEN`, `GENDER`, `MOUTH_OPEN`, `MUSTACHE`, `FACE_OCCLUDED`,
   `SMILE`, `SUNGLASSES`); `InvalidParameterException` is declared in
   `DetectFaces`'s `deserializeOpError` switch. `detectFacesReq.Attributes`
   (`[]string`) accepted any string, validated against nothing — the
   response (`FaceDetails: []faceDetailEntry{}`, always empty regardless of
   input) never even read the field. Fixed: each element now validated via
   `isValidFaceAttribute`, likewise derived from `sdktypes.Attribute("")
   .Values()`; an invalid element returns `InvalidParameterException`.

   Raw error extractions (`awk "/^func awsAwsjson11_deserializeOpError<Op>\(/,/^}/"
   deserializers.go | grep -oE '"[A-Za-z0-9]+"'`, `rekognition@v1.54.4`):
   - `DetectFaces`: `AccessDeniedException InternalServerError
     InvalidImageFormatException InvalidParameterException
     InvalidS3ObjectException ImageTooLargeException
     ProvisionedThroughputExceededException ThrottlingException
     UnknownError`
   - `CompareFaces`: same set as `DetectFaces`.
   - `SearchFacesByImage`: same set as `DetectFaces`, plus
     `ResourceNotFoundException`.
   - `DetectLabels`/`DetectText`/`DetectProtectiveEquipment`/
     `RecognizeCelebrities`: same set as `DetectFaces`.
   - `DetectModerationLabels`: same set as `DetectFaces`, plus
     `HumanLoopQuotaExceededException`/`ResourceNotFoundException`/
     `ResourceNotReadyException`.

   Regression tests (`handler_faces_test.go`):
   `TestImageAnalysis_EnumValidation` (table: `DetectFaces` with a bogus
   `Attributes` value, `CompareFaces`/`SearchFacesByImage` with a bogus
   `QualityFilter` value — all three asserted `400`/`InvalidParameterException`)
   and `TestImageAnalysis_EnumValidation_AcceptsValidValues` (sanity: `ALL`
   attribute, `HIGH` quality filter both `200`). Confirmed failing against
   unmodified code (reverted `handler_faces.go` to `HEAD`, kept the new
   tests, ran `go test -run TestImageAnalysis_EnumValidation`): `DetectFaces`
   and `CompareFaces` returned `200` instead of `400`;
   `SearchFacesByImage` returned `400`/`ResourceNotFoundException`
   (rejecting the never-created `sfbi-enum-coll` collection before ever
   reaching `QualityFilter`) instead of `400`/`InvalidParameterException` —
   confirming the fix, not just the collection lookup, is what the test
   exercises. Restored the fix; all three then pass.

   **Left for its own issue, not expanded into here:** `QualityFilter`
   is validated but not *applied* — this mock never simulates a face
   "quality" signal to filter on, for `CompareFaces`/`SearchFacesByImage`
   or the still-unwired `IndexFaces`. Implementing actual quality-based
   filtering behavior (as opposed to enum validation) is a feature gap,
   not a validation gap, and out of this issue's scope.

   Gates: `GOTOOLCHAIN=go1.26.6 go test -race ./services/rekognition/...`
   (pass), `GOTOOLCHAIN=go1.26.6 golangci-lint run
   services/rekognition/...` (`0 issues.`).

10. **2026-09-06 (gopherstack-duj0): DetectProtectiveEquipment.
    SummarizationAttributes required-ness, correcting Notes #9.** Notes #9
    audited MinConfidence only on the range axis (is an in-range-type,
    out-of-bounds *value* rejected — correctly "no", see that entry) and
    missed the required-ness axis (is a *missing* member rejected). Caught
    on review: `validators.go`'s `validateProtectiveEquipmentSummarizationAttributes`
    (lines 1914-1925) —

    ```go
    if v.MinConfidence == nil {
        invalidParams.Add(smithy.NewErrParamRequired("MinConfidence"))
    }
    if v.RequiredEquipmentTypes == nil {
        invalidParams.Add(smithy.NewErrParamRequired("RequiredEquipmentTypes"))
    }
    ```

    — is a real generated client-side validator, the same evidence class
    Notes #9 used to justify the QualityFilter/Attributes fixes. Both
    members are required *whenever `SummarizationAttributes` itself is
    supplied* (the outer field stays optional — `DetectProtectiveEquipmentInput
    .SummarizationAttributes` itself is not required, only checked with
    `if v.SummarizationAttributes != nil` in `validateOpDetectProtectiveEquipmentInput`).

    **The nil-vs-empty question (settled before implementing, as asked):**
    AWS's check is `RequiredEquipmentTypes == nil`, a nil check, not
    `len(...) == 0`. Standard Smithy list-member "required" semantics: must
    be *set*, not must be *non-empty*. Mirrored faithfully rather than
    treating both as missing — an explicitly-empty array satisfies the
    requirement, matching what the real client-side validator itself
    accepts. `TestDetectProtectiveEquipment_SummarizationAttributesValidation
    /explicit_empty_RequiredEquipmentTypes_array_is_not_the_same_as_missing`
    asserts this directly (200, not 400).

    Fixed (`handler_moderation.go`): `detectProtectiveEquipmentReq
    .SummarizationAttributes` was `struct { RequiredEquipmentTypes []string;
    MinConfidence float32 }` — a plain slice and float32 can't distinguish
    "omitted" from "nil/zero", so nothing could ever be rejected. Retyped to
    `*[]string`/`*float32` (same absent-vs-zero fix pattern as
    gopherstack-7bxb's mediaconvert `ConcurrentJobs`): unmarshaling a JSON
    array (even `[]`) into `*[]string` yields a non-nil pointer, while an
    absent key or explicit `null` leaves it nil; likewise `*float32`
    distinguishes an omitted `MinConfidence` from an explicit `0`.
    `handleDetectProtectiveEquipment` now checks, only when
    `SummarizationAttributes != nil`: `MinConfidence == nil` and
    `RequiredEquipmentTypes == nil`, each returning `InvalidParameterException`
    via the existing `ErrValidation` sentinel. No behavior change to the
    (always-empty) `Persons` response — this is a required-ness check only,
    not new filtering logic.

    No pre-existing test sent a partial `SummarizationAttributes` (the one
    existing `DetectProtectiveEquipment` test, `TestModeration_ImageAnalysis
    /DetectProtectiveEquipment_returns_empty_list`, omits
    `SummarizationAttributes` entirely and still expects 200 — unaffected,
    the outer field stays optional) — nothing to correct.

    Regression test (`handler_moderation_test.go`):
    `TestDetectProtectiveEquipment_SummarizationAttributesValidation` —
    missing `MinConfidence` (400/`InvalidParameterException`), missing
    `RequiredEquipmentTypes` (400/`InvalidParameterException`), both present
    (200), `RequiredEquipmentTypes: []` present-but-empty (200), and
    `SummarizationAttributes` omitted entirely (200). Confirmed failing
    against unmodified code (reverted `handler_moderation.go` to `HEAD`,
    kept the new tests): both required-field subtests returned `200` instead
    of `400`; the empty-array and omitted-struct subtests passed either way,
    as expected (they're not exercising the fix). Restored the fix; all five
    then pass.

    Snapshot guard (`pkgs/persistence/...` `TestSnapshotVersionGuard`) run
    read-only (no `-update`) after the retype: `detectProtectiveEquipmentReq`
    is a request DTO, not a `*Snapshot`-suffixed struct or a `store.Register`ed
    type, so it's outside the guard's scan surface — passed clean, no golden
    diff, as expected.

    Gates: `GOTOOLCHAIN=go1.26.6 go test -race ./services/rekognition/...`
    (pass), `GOTOOLCHAIN=go1.26.6 golangci-lint run
    services/rekognition/...` (`0 issues.` — required reordering the new
    test's table struct fields, `fieldalignment` flagged the first attempt).
